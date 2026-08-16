# UsArr Configuration Reference

> **Status: pre-alpha.** UsArr has no released code yet. This document is the *specification* the
> implementation is being built against, not a description of shipped behaviour. Where a value is a
> design decision that has not yet been exercised against real services, it is marked
> **(proposed)**. Where a fact comes from research that the researcher could not verify from a
> primary source, it is marked **⚠️ unverified**.

---

## 0. Deployment model: tailnet first

**The assumed deployment for UsArr is a private [Tailscale](https://tailscale.com) tailnet.**
Internet exposure is a supported but *secondary*, hardened mode. This is a real architectural
assumption and it changes several defaults:

| | Tailnet (default) | Internet-exposed (secondary) |
|---|---|---|
| Reachability | Only tailnet members. There is no public listener. | Anyone who resolves the hostname. |
| Identity | Cryptographic, from the tailnet connection itself (§6). | Local accounts / OIDC / forward-auth. |
| TLS | Automatic via Tailscale certs (MagicDNS name). | Your reverse proxy's job. |
| `USARR_TRUSTED_PROXIES` | Irrelevant in `tsnet` mode — there is no proxy. Still required for the Tailscale Serve and reverse-proxy paths. | **Mandatory** before any forwarded header is believed. |
| Brute-force surface | Effectively none. | Real; rate limiting and lockout are load-bearing. |

Three concrete topologies, in the order they are recommended:

1. **Embedded tailnet node (`tsnet`)** — UsArr joins the tailnet *itself*, as its own device. No
   `tailscaled`, no published ports, no reverse proxy. Best isolation, least configuration. §6.
2. **Tailscale Serve in front of a loopback listener** — UsArr binds `127.0.0.1`; the host's
   `tailscaled` proxies to it and injects identity headers. §6.3.
3. **Reverse proxy (Traefik/Caddy/nginx), tailnet or public** — the conventional path. Everything in
   §2.2 about `USARR_TRUSTED_PROXIES` applies with full force. §7.3.

Anything about "internet-exposed" hardening in this document is written for topology 3 with a public
address. If you are on a tailnet, most of it is defence in depth rather than the primary control.

**UsArr does not stream or transcode media.** It routes clients to the backend that owns the bytes —
Jellyfin for video, Navidrome for music, Audiobookshelf for audiobooks, Komga/Kavita for
comics and ebooks. There is no FFmpeg, no transcode configuration, and no media-path tuning in this
document, because there is none in the product. UsArr owns identity, the unified library, search,
tagging, and requests; playback surfaces are borrowed.

---

## 1. Configuration philosophy

UsArr splits configuration into two tiers with **different storage, different lifecycles, and
different audiences**.

### Tier 1 — Bootstrap settings (environment / file / flags only)

Everything the process needs *before it can open its database or trust a request*:

| Setting class | Examples |
|---|---|
| Filesystem | config dir, data dir |
| Network identity | bind address, port, URL base, TLS |
| Observability | log level, log format |
| Cryptography | master encryption key |
| Trust boundary | trusted-proxy CIDR allowlist |
| Onboarding | whether public signup is open |

These are **never stored in the database**, are **never editable from the UI**, and **require a
restart** to change. The reason is a security property, not an aesthetic one: the master key
decrypts the database, and the trusted-proxy allowlist decides whether a forwarded auth header is
believed. If either were editable through the surface they protect, a UI-level authorization bug
would become a full compromise. Jellyfin shipped exactly this class of bug —
[GHSA-qcmf-gmhm-rfv9](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-qcmf-gmhm-rfv9),
where spoofable client IPs let an unauthenticated attacker look like LAN and restart the server.

### Tier 2 — Domain settings (database, managed through the UI)

Service instances (Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian, download clients, Jellyfin),
their API keys, users, roles, quotas, tags, tag rules, saved filters, notification targets, and
metadata-provider preferences all live in SQLite and are managed through the UI.

This is deliberate. A self-hoster adds a Sonarr by pasting a URL and an API key into a wizard that
runs a live connection test and shows `appName`/`version` back from
`GET /api/v3/system/status` — not by editing YAML and restarting. Every *Arr response carries an
`X-Application-Version` header and `/ping` is unauthenticated, so the wizard can genuinely validate
before saving.

### Tier 2b — Declarative provisioning, for headless and IaC users

The UI-first model breaks down for people who manage their homelab from a git repo. UsArr therefore
supports **provisioning**: an optional `services.yaml` whose entries are reconciled into the
database at startup.

Rules **(proposed)**:

* Entries created from `services.yaml` are stored in the DB with `managed_by = "file"`.
* File-managed entries are **read-only in the UI**, shown with a "managed by services.yaml" badge.
* Removing an entry from the file does **not** delete it by default; set
  `provisioning.prune: true` to make the file fully authoritative.
* A secret in the file may be given inline, as `${ENV_VAR}`, or as `*_file:` pointing at a path
  (Docker secrets). Inline secrets are still encrypted at rest when written to the DB, but the file
  itself is plaintext — document this to users, and prefer `${ENV_VAR}` / `*_file:`.

### Precedence

**Bootstrap settings** — first match wins:

```
1. command-line flag        --port 8484
2. environment variable     USARR_PORT=8484
3. *_FILE indirection       USARR_SECRET_KEY_FILE=/run/secrets/usarr_key
4. config file              $USARR_CONFIG_DIR/config.yaml
5. built-in default
```

**Domain settings** — the database is always the runtime source of truth:

```
1. database row (written by the UI, or reconciled from services.yaml)
2. services.yaml               reconciled into the DB at each startup
3. USARR_<SERVICE>_URL/_API_KEY env vars
     → create-if-absent ONLY. Applied on a run where no instance of that
       kind exists yet. Never overwrites an existing row, never re-creates a
       row the admin deleted. These exist so `docker run -e USARR_SONARR_URL=...`
       lands you in a working app, not as a config channel.
```

`*_FILE` indirection is supported for **every** variable whose value is a secret
(`USARR_SECRET_KEY_FILE`, `USARR_TMDB_API_KEY_FILE`, `USARR_SONARR_API_KEY_FILE`, …). If both the
variable and its `_FILE` twin are set, UsArr **fails to start** rather than guessing.

### What is deliberately *not* configurable

* The *Arr API version per app. It is a property of the app, not a preference:
  Sonarr/Radarr/Whisparr are `/api/v3`; Lidarr/Readarr/Prowlarr are `/api/v1`. UsArr derives it from
  `system/status.appName`, never from user input.
* Whether *Arr credentials are encrypted at rest. They always are.
* Whether trusted-header auth is on when `USARR_TRUSTED_PROXIES` is empty. It is off. Always.

---

## 2. Environment variable reference

Prefix is `USARR_` for everything UsArr itself reads. `PUID`/`PGID`/`TZ` are unprefixed by
LinuxServer.io convention and are consumed by the **container entrypoint**, not by the Go binary.

### 2.1 Core process

| Variable | Purpose | Default | Required | Example |
|---|---|---|---|---|
| `USARR_CONFIG_DIR` | Directory for `config.yaml`, `services.yaml`, the SQLite DB, and the key file. The one path that must be a persistent volume. | `/config` | No | `/config` |
| `USARR_DATA_DIR` | Directory for regenerable state: image cache, metadata cache, logs, plugins. Safe to delete; UsArr rebuilds it. | value of `USARR_CONFIG_DIR` | No | `/data` |
| `USARR_CONFIG_FILE` | Explicit path to the config file, overriding `$USARR_CONFIG_DIR/config.yaml`. | *(unset)* | No | `/etc/usarr/config.yaml` |
| `USARR_SERVICES_FILE` | Path to the declarative provisioning file (§1, Tier 2b). | `$USARR_CONFIG_DIR/services.yaml` if present | No | `/config/services.yaml` |
| `USARR_BIND_ADDRESS` | Interface to listen on. Use `127.0.0.1` when a reverse proxy is on the same host. | `0.0.0.0` | No | `127.0.0.1` |
| `USARR_PORT` | HTTP listen port. **(proposed)** 8484 was picked to avoid every default port in the ecosystem UsArr talks to (8989/7878/8686/8787/9696/6969/5299/8096/32400/8080/6789/9091/8112/6767/5055/7700). | `8484` | No | `8484` |
| `USARR_URL_BASE` | Sub-path when reverse-proxied at e.g. `https://home.example/usarr`. Leading slash, no trailing slash. Must match the proxy exactly or the SPA's asset paths break. | *(empty)* | No | `/usarr` |
| `USARR_LOG_LEVEL` | `trace`, `debug`, `info`, `warn`, `error`. | `info` | No | `debug` |
| `USARR_LOG_FORMAT` | `auto`, `text`, `json`. `auto` = text on a TTY, JSON otherwise. | `auto` | No | `json` |
| `USARR_LOG_MAX_SIZE_MB` / `USARR_LOG_MAX_FILES` | Log rotation for `$USARR_DATA_DIR/logs/`. | `10` / `5` | No | `50` / `10` |

### 2.2 Security

| Variable | Purpose | Default | Required | Example |
|---|---|---|---|---|
| `USARR_SECRET_KEY` | **Master key.** Wraps the per-record data-encryption keys that protect every stored service credential (*Arr API keys, Jellyfin tokens, download-client passwords) and TOTP secrets. 32 random bytes, base64. See §3. | *(none)* | **Yes** — UsArr refuses to start without it (see §3.1 for the first-run generation path) | `USARR_SECRET_KEY=<base64-32-bytes>` |
| `USARR_SECRET_KEY_FILE` | Path to a file containing the key. Preferred in Docker/Kubernetes. Mutually exclusive with `USARR_SECRET_KEY`. | *(unset)* | No | `/run/secrets/usarr_key` |
| `USARR_TRUSTED_PROXIES` | Comma-separated CIDRs whose `X-Forwarded-For` / `X-Forwarded-Proto` / forwarded auth headers are believed. **Empty means trust nothing** — the peer IP is used as-is and forward-auth is refused. Mandatory before `USARR_FORWARD_AUTH_*` has any effect. | *(empty)* | No, but **mandatory for forwarded-header auth** | `172.18.0.0/16,10.0.0.0/8` |
| `USARR_FORWARD_AUTH_ENABLED` | Enable Authelia/Authentik/Traefik forward-auth. Refuses to enable if `USARR_TRUSTED_PROXIES` is empty. | `false` | No | `true` |
| `USARR_FORWARD_AUTH_USER_HEADER` | Header carrying the username. No default and no guessing — you must name it. | *(none)* | Only if forward-auth on | `Remote-User` |
| `USARR_FORWARD_AUTH_GROUPS_HEADER` | Header carrying groups. | *(none)* | No | `Remote-Groups` |
| `USARR_FORWARD_AUTH_EMAIL_HEADER` | Header carrying email. | *(none)* | No | `Remote-Email` |
| `USARR_ENABLE_SIGNUP` | Allow self-registration at `/signup`. **Inert in v0.1, which is single-user** — the first-run wizard creates the owner account and no second account can be created. The variable is defined now so the name is stable when multi-user lands; setting it to `true` in v0.1 logs a warning and does nothing. | `false` | No | `true` |
| `USARR_SESSION_IDLE_TIMEOUT` | Idle session expiry (Go duration). | `168h` | No | `24h` |
| `USARR_SESSION_ABSOLUTE_TIMEOUT` | Hard session expiry regardless of activity. | `720h` | No | `168h` |
| `USARR_TLS_CERT_FILE` / `USARR_TLS_KEY_FILE` | Serve HTTPS directly. Both or neither. Recommended only when there is no reverse proxy. | *(unset)* | No | `/config/tls/cert.pem` |
| `USARR_ALLOW_INSECURE_TLS_HOSTS` | Comma-separated `host:port` list whose self-signed certificates are accepted on **outbound** calls. *Per-host*, never a global insecure switch — *Arr apps serve HTTPS on a self-signed cert on port 9898 by default. | *(empty)* | No | `sonarr.lan:9898` |

`USARR_TRUSTED_PROXIES` is load-bearing in three places: rate-limit bucketing by client IP, audit-log
`actor_ip`, and forward-auth. Getting it wrong in the permissive direction means anyone who can reach
the app port directly is any user, including the owner — this is why the default is empty and why
forward-auth hard-fails rather than silently degrading.

### 2.3 Metadata providers

All optional. Absent keys disable the corresponding provider; they never cause a startup failure.
See §4 for licensing and rate limits before setting any of these.

| Variable | Purpose | Default | Required | Example |
|---|---|---|---|---|
| `USARR_TMDB_API_KEY` | TMDB v3 API key, for film/TV metadata and artwork. Bring your own — see §4.1. | *(unset)* | No (film/TV metadata degrades to TVmaze + Wikidata without it) | `<tmdb-v3-key>` |
| `USARR_TMDB_READ_TOKEN` | TMDB v4 read access token, if you prefer bearer auth over the v3 key. | *(unset)* | No | `<tmdb-v4-read-token>` |
| `USARR_TVDB_API_KEY` | TheTVDB v4 API key. **There is no free tier** — see §4.2 before enabling. | *(unset)* | No | `<tvdb-v4-key>` |
| `USARR_TVDB_PIN` | Subscriber PIN for TheTVDB "user-supported" keys. Required alongside the key on that plan. | *(unset)* | Only with a user-supported TVDB key | `<tvdb-subscriber-pin>` |
| `USARR_METADATA_USER_AGENT` | Contact string appended to UsArr's outbound `User-Agent`. **MusicBrainz requires an identifying UA with contact info**, and Open Library raises you from 1 to 3 req/s if you send one. Set this if you use either. | `UsArr/<version> (+https://github.com/<owner>/UsArr)` | No, but strongly recommended | `UsArr/0.1 ( admin@example.com )` |
| `USARR_TMDB_CACHE_TTL` | Max age of cached TMDB rows. **Capped at 6 months by TMDB's terms** (§4.1) — values above `4320h` are clamped and logged. | `720h` | No | `2160h` |

### 2.4 Search backend

| Variable | Purpose | Default | Required | Example |
|---|---|---|---|---|
| `USARR_MEILISEARCH_URL` | Meilisearch base URL. When unset, UsArr uses its built-in SQLite FTS5 index — which is the supported default, not a fallback. Meilisearch is an optional plugin for large libraries. | *(unset)* | No | `http://meilisearch:7700` |
| `USARR_MEILISEARCH_KEY` | Meilisearch API key. Use a key scoped to UsArr's indexes, not the master key. | *(unset)* | Only if `_URL` is set and the instance requires auth | `<meili-key>` |
| `USARR_MEILISEARCH_INDEX_PREFIX` | Prefix for UsArr's indexes, so one Meilisearch can serve several apps. | `usarr` | No | `usarr_prod` |

### 2.5 Service bootstrap (headless first run)

These are **create-if-absent only** (§1, precedence). They exist so a single `docker run` produces a
working install; they are not a configuration channel. For real IaC use `services.yaml`.

For a second instance of the same kind, append `_2`, `_3`, … (`USARR_SONARR_2_URL`).
Every `*_API_KEY` also accepts a `*_API_KEY_FILE` twin.

| Variable | Purpose | Default | Required | Example |
|---|---|---|---|---|
| `USARR_JELLYFIN_URL` | Jellyfin base URL — the primary playback and identity backend. | *(unset)* | No | `http://jellyfin:8096` |
| `USARR_JELLYFIN_API_KEY` | Jellyfin server API key (Dashboard → API Keys). If omitted while `_URL` is set, UsArr can mint its own key at setup via `/Auth/Keys` given admin credentials in the wizard — the approach Jellyseerr uses, so an admin password is never stored. | *(unset)* | No | `<jellyfin-api-key>` |
| `USARR_SONARR_URL` / `USARR_SONARR_API_KEY` | Sonarr (TV). Default port 8989, API base `/api/v3`. Key is 32 hex chars, from Settings → General → Security. | *(unset)* | No | `http://sonarr:8989` |
| `USARR_RADARR_URL` / `USARR_RADARR_API_KEY` | Radarr (movies). Port 7878, `/api/v3`. | *(unset)* | No | `http://radarr:7878` |
| `USARR_LIDARR_URL` / `USARR_LIDARR_API_KEY` | Lidarr (music). Port 8686, **`/api/v1`**. | *(unset)* | No | `http://lidarr:8686` |
| `USARR_PROWLARR_URL` / `USARR_PROWLARR_API_KEY` | Prowlarr (indexers). Port 9696, **`/api/v1`**. The source of the `usenet`/`torrent` protocol tag. | *(unset)* | No | `http://prowlarr:9696` |
| `USARR_LAZYLIBRARIAN_URL` / `USARR_LAZYLIBRARIAN_API_KEY` | LazyLibrarian (books/magazines/comics). Port 5299. Key must be **exactly 32 characters** or LazyLibrarian returns `503 Invalid API key`. Prefer its **read-only** key (`API_RO_KEY`) unless you need UsArr to queue books. | *(unset)* | No | `http://lazylibrarian:5299` |
| `USARR_READARR_URL` / `USARR_READARR_API_KEY` | Readarr. **Retired upstream** — the repo was archived 2025-06-27 and its metadata backend is gone. The `/api/v1` surface still responds, so UsArr can read an existing library. Do not build a new library on it. | *(unset)* | No | `http://readarr:8787` |
| `USARR_WHISPARR_URL` / `USARR_WHISPARR_API_KEY` | Whisparr. Port 6969, `/api/v3`, Sonarr-shaped. | *(unset)* | No | `http://whisparr:6969` |
| `USARR_BAZARR_URL` / `USARR_BAZARR_API_KEY` | Bazarr (subtitles). Port 6767, header `X-API-KEY`. Rows join to UsArr via the *Arr instance's local IDs, not external IDs. | *(unset)* | No | `http://bazarr:6767` |

⚠️ **Unverified:** whether the *Arr apps accept their own API key via an env var such as
`SONARR__AUTH__APIKEY`. The options-binding code exists in the Servarr source but the researcher
could not confirm the configuration key prefix. Don't rely on it in compose examples until tested.

### 2.6 Container conventions (LinuxServer.io)

Read by the container entrypoint before it drops privileges. The Go binary never reads them.

| Variable | Purpose | Default | Required | Example |
|---|---|---|---|---|
| `PUID` | UID the process runs as. Must own `$USARR_CONFIG_DIR` and `$USARR_DATA_DIR`. | `1000` | No | `1000` |
| `PGID` | GID the process runs as. Match your media group so UsArr can read mounted libraries. | `1000` | No | `1000` |
| `TZ` | IANA timezone for logs, schedules, and calendar rendering. | `Etc/UTC` | No | `Europe/Lisbon` |
| `UMASK` | umask for files UsArr creates. | `022` | No | `002` |

---

## 3. The master key: `USARR_SECRET_KEY`

### 3.1 What it protects and how

UsArr is a credential vault. A single *Arr API key grants full admin on that instance — delete the
library, rewrite root folders, restart the app. Those keys, Jellyfin tokens, download-client
passwords, and TOTP secrets are stored with **envelope encryption**:

* Each secret gets a random data-encryption key (DEK) and a unique nonce; the value is sealed with
  **AES-256-GCM**, so tampered ciphertext is rejected rather than silently decrypted.
* Each DEK is wrapped with the key-encryption key derived from `USARR_SECRET_KEY`.
* Only the wrapped DEK and the ciphertext go in the database.

This indirection is what makes key rotation cheap: rotating re-wraps DEKs, it does not re-encrypt
every secret.

**Startup behaviour (proposed):**

* Key present → start.
* Key absent, `$USARR_CONFIG_DIR/secret.key` present → use it.
* Key absent, no key file, **and the database has no encrypted rows** (first run) → generate 32
  random bytes, write `$USARR_CONFIG_DIR/secret.key` with mode `0600`, print the key once with a
  loud "back this up now" warning, and continue.
* Key absent, no key file, **but the database contains encrypted rows** → **refuse to start** with a
  message naming the missing variable. Never start half-decrypted.

### 3.2 Generating a key

```bash
# Any of these produce 32 random bytes, base64-encoded.
openssl rand -base64 32
head -c 32 /dev/urandom | base64
python3 -c 'import os,base64; print(base64.b64encode(os.urandom(32)).decode())'

# Or, once the binary exists:
usarr keygen
```

Put it somewhere your password manager or secrets store will keep. **If you lose it, every stored
service credential is unrecoverable** — the fix is deleting the credentials and re-pasting every API
key. Losing the key does not lose your library data.

### 3.3 Rotating

```bash
usarr key rotate --new-key-file /run/secrets/usarr_key.new    # (proposed CLI)
```

Rotation stops the HTTP listener, re-wraps every DEK under the new KEK inside one transaction, and
writes a rotation entry to the audit log. Rotate when: the key was ever committed to a repo or
pasted into a chat, a machine holding it was compromised, or a person with access to it left.
Rotating the master key does **not** invalidate user sessions or force password resets — the
password *pepper* is a separate value, and rotating that one does force resets.

### 3.4 Handling rules

* Never in a `docker-compose.yml` that is committed. Use `env_file:` with a gitignored `.env`, or a
  Docker/Kubernetes secret plus `USARR_SECRET_KEY_FILE`.
* Never returned by the API. Stored credentials are never sent to the client either — the UI shows
  `••••••1a2b` and offers a server-side "Test connection" button.
* Redacted from logs, from error messages, and from any support-bundle export.

---

## 4. Metadata providers

Attribution and rate limits are **product requirements**, not optional politeness. The table below
is the state of the research as of 2026-08-16; verification level is marked per row.

| Provider | Key needed? | How to get one | Free-tier limits | Rate limit | Licensing constraints for a self-hosted OSS app |
|---|---|---|---|---|---|
| **TMDB** (film/TV) | Yes | Create an account, then request a key at <https://www.themoviedb.org/settings/api> | Free for non-commercial use | ⚠️ **Not published.** The "~40 req/s per IP" figure circulating in forums is not in the terms; the terms say only "no excessive bandwidth". Implement adaptive backoff on 429, not a hard-coded rate. | ✅ Read from the [terms of use](https://www.themoviedb.org/api-terms-of-use): non-commercial only (commercial needs a signed agreement); **mandatory attribution** — the TMDB logo plus the exact sentence *"This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB"*, with TMDB's logo less prominent than UsArr's; **caching capped at 6 months**; must not be used as an image host; **no sublicensing** of keys, and no "cloaking or concealing your identity". The terms also name *training or validating an ML system* as commercial use. |
| **TheTVDB v4** | Yes, **and it is never free** | <https://thetvdb.com/api-information> | **None.** ✅ Per [TVDB FAQ 81](https://support.thetvdb.com/kb/faq.php?id=81) there are exactly two paths: a negotiated commercial contract, or a "user-supported" key where **every one of your end users** holds a paid TheTVDB subscription and supplies a PIN. | ⚠️ Unpublished | Off by default in UsArr, PIN-based, never a hard dependency. ⚠️ The ~$12/yr/user subscription price is secondhand. |
| **TVmaze** (TV) | No | — | Free | 📄 ≥20 calls / 10 s per IP; edge-cached hits don't count | Free; user-contributed data is ⚠️ reportedly CC BY-SA — verify before redistributing a derived dataset. Carries `externals.thetvdb` and `externals.imdb`, which makes it the practical free replacement for TVDB. |
| **MusicBrainz** (music) | No | — | Free | ✅ **1 req/s per IP.** Exceeding it returns HTTP 503. | Core data is **CC0**. A **descriptive `User-Agent` with contact info is mandatory** — see [rate-limiting docs](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting). Set `USARR_METADATA_USER_AGENT`. For library-scale scanning, 1 req/s is not viable; the escape hatch is the [Live Data Feed](https://musicbrainz.org/doc/Live_Data_Feed) local mirror. ⚠️ Its access terms for an OSS project were not verified. **Do not route through `api.lidarr.audio`** — it is a Servarr-run cache with documented propagation lag and repeated breakage. |
| **Cover Art Archive** | No | — | Free | ✅📄 No rate limiting currently in place | Internet-Archive-backed, keyed by MusicBrainz release/release-group MBID. |
| **Open Library** (books) | No | — | Free | ✅ **1 req/s anonymous, 3 req/s with an identifying `User-Agent`** | ✅ Terms explicitly say **"do not use our APIs for bulk download"** — use the monthly dumps. ⚠️ The exact data license was not read in full; check <https://openlibrary.org/developers/licensing> before redistributing. The Work/Edition model is what UsArr's book identity layer is built on. |
| **Hardcover** (books) | Yes, per user | <https://hardcover.app> account → API token, docs at <https://docs.hardcover.app/api/getting-started/> | 📄 60 req/min, 30 s timeout, max GraphQL query depth 3 | 📄 Same figures — secondhand; the docs site returned 403 to automated fetch | ⚠️ Commercial/redistribution terms for a third-party self-hosted app **not verified**. Designed as a per-user token: each UsArr user supplies their own. |
| **Google Books** | Yes | <https://console.cloud.google.com/apis/library/books.googleapis.com> | ⚠️ "Fair use", per-project quota, exact figure unverified | ⚠️ Unverified | Good ISBN gap-fill; volume IDs are not works, so it is enrichment only. |
| **Wikidata** | No | — | Free | ✅ `Special:EntityData/{QID}.json` and `query.wikidata.org/sparql` were reliable under test; ⚠️ `www.wikidata.org/w/api.php` rate-limited the researcher within a handful of sequential requests — **do not build the hot path on `api.php`**. SPARQL timeout 📄 60 s. | **CC0.** No attribution obligation, no commercial restriction, no cache limit. The only provider in this table with genuinely unencumbered terms, which is why it is UsArr's cross-media spine. |
| **AniList** (anime) | No for public GraphQL | <https://anilist.co/settings/developer> for OAuth | Free | 📄 90 req/min, `X-RateLimit-*` headers, 429 + 1-min timeout on breach | Free. Best anime source. Pair with vendored ID-mapping files (Fribb/anime-lists) rather than solving anime numbering yourself. |
| **Fanart.tv** (artwork) | Yes (project key; personal key optional) | <https://fanart.tv/get-an-api-key/> | Free project key | 📄 "Rarely applied"; personal keys get fresher data | Artwork only. |
| **Trakt** | Yes (OAuth client) | <https://trakt.tv/oauth/applications> | Free | 📄 ~1 write/s, ~1000 GET / 5 min | Good for watch state; it mirrors TMDB for metadata, so it is not a base layer. |
| **Metron** (comics) | Yes (HTTP Basic) | <https://metron.cloud/> account | Free | ✅📄 20 req/min burst, 5 000 req/day; supports `If-Modified-Since` → 304 and `modified_gt` incremental sync | Community-run and far friendlier than Comic Vine. ⚠️ Limits were reduced in early 2025 — re-check before launch. |
| **Comic Vine** (comics) | Yes | <https://comicvine.gamespot.com/api/> | Free with registration | 📄 200 req/resource/hour, plus velocity detection → temporary blocks | ⚠️ ToU for caching/redistribution in a self-hosted app **not read**. Fallback only. |
| **OMDb** | Yes | <https://www.omdbapi.com/apikey.aspx> | 📄 1 000 req/day free | 📄 As above | Third-party IMDb scrape; ratings only. |
| **IMDb** | — | — | — | — | **No free public API.** `tt…`/`nm…` IDs are used as a lingua franca only, obtained via TMDB `external_ids` or Wikidata `P345`. The bulk datasets at `datasets.imdbws.com` are ⚠️ non-commercial-use-only. |

**Verification legend:** ✅ verified live during research · 📄 from vendor docs or a reputable
secondary source, not independently exercised · ⚠️ unverified or volatile.

### 4.1 TMDB, stated plainly

Three concrete obligations fall on UsArr, not on the user:

1. **Attribution** must be rendered in the UI wherever TMDB data appears, with the required sentence
   verbatim and the TMDB logo less prominent than UsArr's own.
2. **The 6-month cache cap is a schema requirement.** TMDB-derived rows carry `provider` and
   `fetched_at`, and a refresh job evicts anything older. `USARR_TMDB_CACHE_TTL` is clamped to
   4320h for this reason.
3. **UsArr will not ship a bundled TMDB key.** The terms forbid sublicensing and forbid concealing
   identity. Radarr and Sonarr ship a shared key *by arrangement with TMDB*. Until someone emails
   `tmdb@themoviedb.org` and gets written permission, users bring their own key. ⚠️ Open item.

Where `watch/providers` (JustWatch) data is displayed, **per-item JustWatch attribution is
mandatory** and TMDB revokes API access for non-compliance. ✅

### 4.2 Why TVDB is optional and off

There is no free path. Making TVDB a hard dependency would mean every UsArr user needs a paid
TheTVDB subscription. The default TV path is therefore **TMDB + TVmaze**, with AniList and the
vendored anime mapping files for anime, and TVDB available for users who already have a key.

---

## 5. Data directory layout

```
$USARR_CONFIG_DIR/            # /config — PERSIST THIS. Irreplaceable.
├── config.yaml               # bootstrap settings (optional; env vars override)
├── services.yaml             # optional declarative provisioning (§1 Tier 2b)
├── secret.key                # mode 0600 — only if USARR_SECRET_KEY is not set
├── usarr.db                  # SQLite, WAL mode
├── usarr.db-wal              # write-ahead log      ┐ transient, but see §6:
├── usarr.db-shm              # shared-memory index  ┘ never copy these by hand
└── backups/
    └── usarr-2026-08-16T03-00-00Z.db   # output of the scheduled backup job

$USARR_DATA_DIR/              # /data — regenerable. Safe to delete; UsArr rebuilds it.
├── cache/
│   ├── images/               # proxied *Arr MediaCover + provider artwork.
│   │                         #   *Arr cover URLs are instance-relative and require the API key,
│   │                         #   so UsArr must proxy and cache them; provider images are fetched
│   │                         #   under the SSRF egress policy (no private ranges).
│   └── metadata/             # provider response cache, TTL-bounded (TMDB: ≤6 months, §4.1)
├── logs/
│   └── usarr.log             # + rotated usarr.log.1 … per USARR_LOG_MAX_FILES
├── plugins/
│   ├── available/*.wasm      # WASM plugins (wazero/Extism host)
│   └── data/<plugin-id>/     # per-plugin scratch, isolated from the host filesystem
└── tmp/                      # in-progress downloads/imports; cleared at startup
```

**Single-volume Docker installs** leave `USARR_DATA_DIR` unset, so everything lands under `/config`.
**NAS installs** should split them: `/config` on SSD (SQLite hates spinning rust and network
filesystems), `/data` wherever the cache fits.

⚠️ **Do not put `usarr.db` on NFS, SMB, or any network filesystem.** SQLite's locking is unreliable
there and WAL mode requires shared memory the remote filesystem does not provide. This is the single
most common corruption cause for SQLite-backed self-hosted apps.

---

## 6. Backup and restore

### What must be backed up

| Item | Why | Recoverable without it? |
|---|---|---|
| `usarr.db` | Library index, users, service configs, tags, links, request history | No |
| `USARR_SECRET_KEY` (or `secret.key`) | Decrypts every stored credential in that DB | The DB opens, but every service credential is garbage — re-paste every API key |
| `config.yaml` / `services.yaml` | Bootstrap + provisioning | Yes, rewrite by hand |
| `$USARR_DATA_DIR` | Caches, logs | Yes — UsArr rebuilds it |

**The key and the database are two halves of one backup.** Storing them together in the same
archive defeats encryption at rest; storing only one defeats recovery. Keep the DB in your normal
backup rotation and the key in a password manager or secrets store.

### Taking a backup

```bash
# Correct: an atomic, consistent copy while UsArr is running.
sqlite3 /config/usarr.db "VACUUM INTO '/config/backups/usarr-$(date -u +%FT%TZ).db'"

# Also correct: stop UsArr, then copy the .db AND -wal AND -shm together.
```

**Wrong:** `cp usarr.db backup.db` on a running instance. In WAL mode the newest committed
transactions live in `usarr.db-wal`, so a bare copy of the main file is a torn, older database that
may not even open. `VACUUM INTO` uses SQLite's own machinery and yields a single consistent file.

UsArr's own scheduled backup job **(proposed)** writes to `$USARR_CONFIG_DIR/backups/` using the same
mechanism, retains N files, and never includes the master key in the archive.

### Restoring

1. Stop UsArr.
2. Move the damaged `usarr.db`, `usarr.db-wal`, `usarr.db-shm` aside — do not delete them until the
   restore is confirmed good.
3. Copy the backup into place as `usarr.db`. There is no `-wal`/`-shm` to restore; SQLite recreates
   them.
4. Restore `USARR_SECRET_KEY` to the same value it had when the backup was taken.
5. Start UsArr. It runs any pending migrations forward automatically. Migrations are **forward-only
   (proposed)** — restoring a newer backup into an older binary is not supported.
6. Open Settings → Services and confirm each instance shows a green connection test. A red test with
   a decryption error means the key does not match the backup.

### Moving to a new host

Copy `$USARR_CONFIG_DIR` (or just the `VACUUM INTO` output), set `USARR_SECRET_KEY` to the same
value, point `PUID`/`PGID` at a user that owns the volume, and start. Service URLs pointing at
`localhost` will need editing if the topology changed — this is a common gotcha when moving from a
bare-metal install to Docker, where `http://localhost:8989` must become `http://sonarr:8989`.

---

## 7. Worked examples

### 7.1 Minimal Docker Compose

```yaml
services:
  usarr:
    image: ghcr.io/<owner>/usarr:latest      # not yet published — pre-alpha
    container_name: usarr
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Europe/Lisbon
      - USARR_SECRET_KEY_FILE=/run/secrets/usarr_key
      - USARR_TRUSTED_PROXIES=172.18.0.0/16   # your proxy's docker network
    volumes:
      - ./config:/config
      - ./data:/data
    ports:
      - "8484:8484"
    secrets:
      - usarr_key
    restart: unless-stopped

secrets:
  usarr_key:
    file: ./secrets/usarr_key            # contains ONLY the base64 key, no newline needed
```

### 7.2 Headless / IaC, with provisioning

`services.yaml` **(proposed schema)**:

```yaml
version: 1
provisioning:
  prune: false        # true makes this file fully authoritative

services:
  - kind: sonarr
    name: "Sonarr (TV)"
    url: http://sonarr:8989
    api_key: ${SONARR_API_KEY}         # resolved from the environment at reconcile time
  - kind: prowlarr
    name: "Prowlarr"
    url: http://prowlarr:9696
    api_key_file: /run/secrets/prowlarr_key
  - kind: jellyfin
    name: "Jellyfin"
    url: http://jellyfin:8096
    api_key: ${JELLYFIN_API_KEY}
  - kind: lazylibrarian
    name: "LazyLibrarian"
    url: http://lazylibrarian:5299
    api_key: ${LL_RO_KEY}              # read-only key is enough for browsing
```

### 7.3 Behind Traefik at a sub-path with forward auth

```yaml
environment:
  - USARR_URL_BASE=/usarr
  - USARR_BIND_ADDRESS=0.0.0.0
  - USARR_TRUSTED_PROXIES=172.18.0.0/16
  - USARR_FORWARD_AUTH_ENABLED=true
  - USARR_FORWARD_AUTH_USER_HEADER=Remote-User
  - USARR_FORWARD_AUTH_GROUPS_HEADER=Remote-Groups
  - USARR_FORWARD_AUTH_EMAIL_HEADER=Remote-Email
```

`USARR_TRUSTED_PROXIES` must contain the network Traefik reaches UsArr from, and **nothing wider**.
UsArr strips the configured auth headers from every inbound request before processing and only
re-reads them when the peer is inside the allowlist. If the app port is also published to your LAN,
anyone on the LAN bypasses Traefik — publish only to the proxy network, or bind to `127.0.0.1`.

Forward-auth headers are never honoured on API-key endpoints. Those are separate auth paths on
purpose, so a browser cannot accidentally authenticate a machine endpoint.

---

## 8. Open configuration questions

Tracked here so they are not silently forgotten:

* Whether TMDB will grant UsArr a bundled key, or users must always bring their own (§4.1).
* Whether the *Arr apps accept `SONARR__AUTH__APIKEY`-style env injection (§2.5) — this would let
  the compose bundle pre-seed keys instead of asking users to copy them.
* MusicBrainz Live Data Feed access terms for an OSS project (§4).
* Hardcover's terms for third-party self-hosted apps (§4).
* Open Library's exact data license (§4).
* Whether `USARR_PORT` default 8484 collides with anything common in the self-hosting ecosystem.
