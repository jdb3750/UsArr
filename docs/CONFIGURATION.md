# UsArr Configuration Reference

> **Status: pre-alpha, zero application code.** This is the *specification* the implementation is
> being built against, not a description of shipped behaviour. Design decisions not yet exercised
> against real services are marked **(proposed)**; facts nobody verified from a primary source are
> marked **⚠️ unverified**.
>
> **Scope authority:** `docs/ARCHITECTURE.md` §16 owns the roadmap. Where this document states that
> something is available at a milestone, §16 wins. **§5 of this document (the directory layout) is
> authoritative for the whole project** — `ARCHITECTURE.md` conforms to it, not the other way round.

Roadmap shorthand used throughout: **v0.1** unified library + search · **v0.2** requests ·
**v0.3** cross-media · **v0.4** gateway · **v1.0** breadth.

---

## 0. How UsArr is reached

**v0.1 has exactly one network story: UsArr binds a TCP port.** You either publish it on a trusted
LAN, or you put a reverse proxy in front of it.

| | Direct port (simplest) | Reverse proxy (Traefik / Caddy / nginx) |
|---|---|---|
| Config | `USARR_BIND_ADDRESS`, `USARR_PORT` | the above, plus `USARR_URL_BASE` and `USARR_TRUSTED_PROXIES` |
| TLS | **none — plain HTTP** | terminated at the proxy |
| Suitable for | a home LAN or a private overlay you already trust | anything else, including internet exposure |

UsArr does not terminate TLS itself and there is no `USARR_TLS_*` variable: a second-rate TLS stack
inside a credential vault is worse than the solved problem sitting next to it.

An embedded Tailscale node (`tsnet`) is a **later milestone**. It is designed in §9 so the shape is
settled, but nothing in v0.1 depends on it and the setup checklist does not ask you for a tailnet.

**v0.1 is single-user** — one owner account. Roles, quotas, per-user library visibility, signup,
OIDC, passkeys, TOTP and forward-auth are not deferred, they are **cut from the project**
(`ARCHITECTURE.md` §16). Nothing here configures them.

**UsArr does not stream or transcode media.** No FFmpeg, no transcode configuration, no media-path
tuning — there is none in the product.

---

## 1. Configuration philosophy: two levels, and only two

### Level 1 — Bootstrap. Environment and flags. Never the database, never the UI.

Everything the process needs *before it can open its database or trust a request*: where the data
lives, what socket to bind, how to log, the master key, and which proxies may be believed.

Never stored in the database, never editable from the UI, restart to change. The reason is a security
property, not an aesthetic one: the master key decrypts the database and the trusted-proxy allowlist
decides whether a forwarded header is believed. If either were editable through the surface it
protects, a UI authorization bug would be a full compromise. Jellyfin shipped exactly this class of
bug —
[GHSA-qcmf-gmhm-rfv9](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-qcmf-gmhm-rfv9),
where spoofable client IPs let an unauthenticated attacker look like LAN and restart the server.

**Precedence within level 1 — first match wins:**

```
1. command-line flag              --port 8484
2. environment variable           USARR_PORT=8484
3. USARR_SECRET_KEY_FILE          (the one variable with a file twin; see §3)
4. built-in default
```

There is no `config.yaml`. A file that duplicates the environment is a third place for a setting to
hide and a fourth precedence rule to explain.

### Level 2 — Services. The database, written by the UI. Nothing else.

Service instances (Sonarr, Radarr, Prowlarr, Jellyfin, Navidrome, …), their credentials, tags, saved
filters, metadata-provider keys and per-instance TLS settings live in SQLite and are created, edited
and deleted **only through the UI**, behind a wizard that runs a live connection test before saving.

**There is no environment channel for service configuration, and no provisioning file.** No
`USARR_SONARR_URL`, no `USARR_SONARR_2_API_KEY`, no `services.yaml`, no `managed_by`, no
"managed by environment" badge, no prune semantics. This removes the precedence contradiction rather
than arbitrating it: with one writer there is nothing to arbitrate. The cost is one manual step on a
fresh headless install. The benefit is that "where did this setting come from?" has exactly one
answer, forever.

### `*_FILE` twins

Supported for **`USARR_SECRET_KEY` only** — it is the sole secret in the environment. Setting both
the variable and its twin is a startup error, not a fallback; UsArr does not guess which you meant.

### What is deliberately *not* configurable

* **The *Arr API version per app.** A property of the app, not a preference: Sonarr, Radarr and
  Whisparr are `/api/v3`; Lidarr, Readarr and Prowlarr are `/api/v1`. UsArr derives it from
  `system/status.appName`, never from user input.
* **Whether service credentials are encrypted at rest.** They always are (§3).
* **Whether forwarded headers are believed with `USARR_TRUSTED_PROXIES` empty.** They are not. Ever.
* **Session lifetimes.** Fixed at 72 h idle / 720 h absolute. Sensitive operations — adding or
  changing a service credential, changing an instance's `base_url`, downloading a backup, rotating
  the master key — additionally require re-authentication inside a 5-minute window ("sudo mode",
  `ARCHITECTURE.md` §12.1). A stolen cookie must not be a 30-day key to the vault.
* **The password hashing scheme.** Argon2id with a per-hash salt. **There is no password pepper**;
  earlier drafts referenced one twice and specified it nowhere, which is worse than not having one —
  a pepper silently present on one deploy and absent on another locks every user out.

---

## 2. Environment variable reference

Fourteen variables, of which two are development-only. `TZ` is unprefixed by container convention.

### 2.1 Core process

| Variable | Purpose | Default | Required |
|---|---|---|---|
| `USARR_CONFIG_DIR` | Irreplaceable state: the SQLite database, the key file, provider manifests, backups. Must be a persistent volume. See §5. | `/config` | No |
| `USARR_DATA_DIR` | Regenerable state: image cache, HTTP cache, logs, temp. Safe to delete. | value of `USARR_CONFIG_DIR` | No |
| `USARR_BIND_ADDRESS` | Interface to listen on. Use `127.0.0.1` when a reverse proxy runs on the same host. | `0.0.0.0` | No |
| `USARR_PORT` | HTTP listen port. **(proposed)** 8484 avoids every default port in the ecosystem UsArr talks to (8989/7878/8686/8787/9696/6969/5299/8096/32400/8080/6789/9091/8112/6767/5055). | `8484` | No |
| `USARR_URL_BASE` | Sub-path when reverse-proxied at e.g. `https://home.example/usarr`. Leading slash, no trailing slash. Must match the proxy exactly or the SPA's asset paths break. | *(empty)* | No |
| `USARR_LOG_LEVEL` | `trace`, `debug`, `info`, `warn`, `error`. | `info` | No |
| `USARR_LOG_FORMAT` | `auto`, `text`, `json`. `auto` = text on a TTY, JSON otherwise. | `auto` | No |

Log rotation is fixed at 10 MB × 5 files in `$USARR_DATA_DIR/logs/`.

🚩 **`trace` is not an exemption from redaction.** A fixed deny-list of query parameters (`apiKey`,
`api_key`, `apikey`, `token`, `access_token`, `sig`, `p`, `t`, `s`) plus the `Authorization` and
`X-Api-Key` headers is replaced with `<redacted>` by middleware **before** any log line, audit row,
error string or support bundle exists — at every level. Middleware, not a convention a contributor
can forget: northbound OpenSubsonic credentials travel in the query string *by specification*
([apiKeyAuthentication](https://opensubsonic.netlify.app/docs/extensions/apikeyauth/)), so an
unredacted access log *is* a credential file.

### 2.2 Security

| Variable | Purpose | Default | Required |
|---|---|---|---|
| `USARR_SECRET_KEY` | **Master key.** Wraps the per-record data-encryption keys protecting every stored service credential. 32 random bytes, base64. **Leaving it unset is the correct choice** — see §3. | *(unset)* | No. Generated on first run if absent; required thereafter |
| `USARR_SECRET_KEY_FILE` | Path to a file containing the key. Preferred in Docker/Kubernetes. Mutually exclusive with `USARR_SECRET_KEY`. | *(unset)* | No |
| `USARR_TRUSTED_PROXIES` | Comma-separated CIDRs whose `X-Forwarded-For` / `X-Forwarded-Proto` are believed. **Empty means trust nothing** — the peer IP is used as-is. | *(empty)* | Only behind a reverse proxy |

`USARR_TRUSTED_PROXIES` is load-bearing in two places: rate-limit bucketing by client IP, and the
audit log's `actor_ip`. Too permissive and every request appears to come from the proxy, so per-IP
rate limiting silently stops working. Set it to the proxy's network and **nothing wider**; never
`0.0.0.0/0`. And if the app port is also published to your LAN while a proxy fronts it, anyone on the
LAN bypasses the proxy — publish only to the proxy network, or bind `127.0.0.1`.

### 2.3 Metadata

| Variable | Purpose | Default | Required |
|---|---|---|---|
| `USARR_METADATA_USER_AGENT` | Contact string for outbound metadata requests to the providers that require or reward one. | `UsArr/<version> (+https://github.com/<owner>/UsArr)` | No |

⚠️ **This value is transmitted to third parties**, and only to the two whose terms require or reward
identification: [MusicBrainz](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting) (mandatory)
and [Open Library](https://openlibrary.org/developers/api) (1 → 3 req/s). Every other provider gets a
generic `UsArr/<version>` with no contact string. A project URL works as well as an email address.

Provider **API keys are not environment variables** — TMDB, Fanart.tv, Comic Vine and the rest are
configured in the UI and stored encrypted like every other credential. No metadata provider is
required for v0.1 at all (§4).

### 2.4 Container

| Variable | Purpose | Default |
|---|---|---|
| `TZ` | IANA timezone for logs, schedules and calendar rendering. Resolved from the `tzdata` shipped in the base image. | `Etc/UTC` |

**There is no `PUID` / `PGID` / `UMASK`.** The container runs as a fixed non-root `65532:65532` from a
[distroless static](https://github.com/GoogleContainerTools/distroless) base, which has no shell and
no `chown` and therefore cannot host an LSIO-style privilege-dropping entrypoint. Making that
entrypoint work would mean starting as **root** in the process holding every one of your *Arr admin
keys — the wrong trade for a credential vault.

The consequence is explicit and yours to handle: **pre-create and chown the volumes.**

```bash
mkdir -p ./config ./data
sudo chown -R 65532:65532 ./config ./data && sudo chmod 700 ./config ./data
```

To run as a different UID use Docker's own mechanism (`user: "1000:1000"` in compose) and chown to
match. UsArr writes its files with explicit modes (§5), so a `umask` would not change them anyway.

### 2.5 Development only

| Variable | Purpose |
|---|---|
| `USARR_INTEGRATION=1` | Opt in to tests behind the `integration` build tag. Requires a live stack; never set in CI. `docs/DEVELOPMENT.md` §8. |
| `USARR_RECORD=1` | Put go-vcr into record mode: real HTTP goes out and cassettes are written. `docs/DEVELOPMENT.md` §7.2. |

### 2.6 Deleted, and why

Roughly sixty variables were specified in earlier drafts. If you are migrating from one, these are
gone, and none returns without an ADR:

* **`USARR_SONARR_URL` / `_API_KEY` and every per-service twin (incl. `_2`, `_3`), `services.yaml`,
  `config.yaml`, `USARR_SERVICES_FILE`, `USARR_CONFIG_FILE`** — services are configured in the UI
  (§1). Three channels for one setting needed a precedence chain to arbitrate; now there is one.
* **`*_FILE` for anything but `USARR_SECRET_KEY`** — nothing else is a secret in the environment.
* **`USARR_ALLOW_INSECURE_TLS_HOSTS`** — two mechanisms for one property. Per-instance wins (§7.1).
* **`USARR_TLS_CERT_FILE` / `_KEY_FILE`** — use a reverse proxy (§0).
* **`USARR_FORWARD_AUTH_*`, `USARR_ENABLE_SIGNUP`, `USARR_SESSION_*`** — multi-user and external
  identity are cut from the project.
* **`USARR_MEILISEARCH_*`** — Meilisearch is cut; SQLite FTS5 is the engine, not a fallback.
* **`USARR_TMDB_*`, `USARR_TVDB_*`, `USARR_COMICVINE_*`, `USARR_FANART_*`, `USARR_JELLYFIN_*`,
  `USARR_NAVIDROME_*` and every other backend variable** — credentials live in the database.
* **`USARR_TSNET_*`** — later milestone (§9); documented, not shipped, not in `.env.example`.
* **`PUID`, `PGID`, `UMASK`** — incompatible with the distroless base (§2.4).
* **`USARR_LOG_MAX_SIZE_MB` / `_MAX_FILES`, `USARR_STREAM_MODE`** — fixed default; and UsArr never
  carries media bytes.

---

## 3. The master key: `USARR_SECRET_KEY`

### 3.1 What it protects

UsArr is a credential vault. A single *Arr API key is **full admin** on that instance — it can delete
the library, rewrite root folders and restart the app. Those keys and every other stored credential
get envelope encryption: a random data-encryption key (DEK) and unique nonce per secret, sealed with
**AES-256-GCM** so tampered ciphertext is rejected rather than silently decrypted; each DEK wrapped
under a key-encryption key derived from `USARR_SECRET_KEY`; only the wrapped DEK, the key id and the
ciphertext in the database.

The indirection makes rotation cheap: rotating re-wraps DEKs, it does not re-encrypt secrets. The
envelope format, key id and additional authenticated data are `ARCHITECTURE.md` §14.1's; this section
owns the *operational* contract.

### 3.2 Startup ladder — one behaviour, stated once

The `Required` column in §2.2 says "No. Generated on first run if absent; required thereafter." In
full **(proposed)**:

| State | Behaviour |
|---|---|
| Variable **unset** and `$USARR_CONFIG_DIR/keys/secret.key` **absent** and the database holds **no encrypted rows** | Generate 32 bytes from `crypto/rand`, create `keys/` mode `0700`, write `secret.key` mode `0600`, log a one-line "a new master key was generated — back it up, see §3.5" notice, continue. **This is the normal first run and the recommended path.** |
| Variable unset, key file present | Read the key file. Validate it (below). |
| Variable set | Use it. Validate it (below). Ignore any key file, and log that the file is being ignored. |
| Variable **set to the empty string** | **Refuse to start**, naming the variable. Empty is not "absent": the usual way to get an empty value is an unset shell variable interpolated into a compose file (`USARR_SECRET_KEY=${USARR_SECRET_KEY}`), and silently generating a key there would produce a key the operator does not know exists and will not back up. |
| Variable unset, key file absent, but the database **contains encrypted rows** | **Refuse to start**, naming the missing key and the file path it expected. Never start half-decrypted. |
| Both `USARR_SECRET_KEY` and `USARR_SECRET_KEY_FILE` set | **Refuse to start.** No guessing. |

**Validation, applied to every supplied key.** All four failures are fatal and named; there is no
"best effort" branch, no hashing-to-length fallback and no truncation:

1. It must be valid base64 (standard alphabet, padding optional).
2. It must decode to **exactly 32 bytes**. 31 or 33 is an error, not something to pad.
3. It must not be all-zero.
4. It must not match the **placeholder reject-list** — a hardcoded set containing every placeholder
   string ever shipped in a released `.env.example` (currently one:
   `REPLACE_ME_WITH_OUTPUT_OF_openssl_rand_base64_32`, shipped in pre-alpha). On a match, the error
   names the file the value came from. A published repository is not a key.

There is **no shipped default** and `.env.example` contains **no uncommented key line**. Leaving the
variable unset is correct and safe; the first-run path generates a real key with real entropy, which
is strictly better than any value a human copies out of a README.

### 3.3 Generating one yourself

Only needed if you want the key in a secrets manager rather than on the config volume.

```bash
openssl rand -base64 32          # or: usarr keygen   (proposed CLI, v0.1)
```

### 3.4 Rotating — atomic and resumable

```bash
usarr key rotate                 # (proposed CLI, v0.1)
```

Scheduled for **v0.1**, alongside `usarr keygen` and the "re-enter your credentials" recovery flow.
A recovery path on no milestone is a recovery path that does not exist when the first user needs it.

Two-phase, and every intermediate state is recoverable because each stored envelope carries the id of
the key that wrapped it:

1. **Prepare.** Generate the new key, write `keys/secret.key.new` (mode `0600`), and register *both*
   keys as active for decryption. Nothing is unreadable from this moment on.
2. **Re-wrap, in batches.** Re-wrap DEKs in bounded transactions, advancing each row's key id as it
   goes. Progress is visible as `SELECT count(*) … WHERE kek_id = <old>`. A crash, OOM, disk-full or
   container restart here leaves a database that is fully readable under the two-key set.
3. **Promote.** When zero rows remain at the old key id, `fsync`, then atomically `rename(2)`
   `secret.key.new` → `secret.key`, then drop the old key from the active set.
4. **Resume.** If `secret.key.new` exists at startup, rotation resumes from step 2 automatically. It
   never restarts from the beginning and never needs the operator to work out how far it got.
5. Write one audit-log entry per phase, including the count re-wrapped.

Rotate when: the key was ever committed to a repository or pasted into a chat, a machine holding it
was compromised, or a person with access to it left. Rotating does **not** invalidate sessions and
does **not** force password resets.

### 3.5 If you lose the key

**Unrecoverable:** every stored service credential — each *Arr API key, each Jellyfin/Audiobookshelf
token, each Navidrome `subsonicSalt`/`subsonicToken` pair, each metadata provider key. No recovery,
no reset, no support path. AES-256-GCM under a lost key is noise.

**Survives:** everything else — the library index, works, editions, links, tags, requests, playback
state, the audit log, user accounts and password hashes all open normally.

**The repair** is mechanical and tedious. Start with a new key; UsArr marks every instance whose
credential fails to open as `needs_credential`. Open each in the UI and paste the key again. For a
household stack that is ten minutes and a lot of tab-switching.

### 3.6 Handling rules

* Never in a committed `docker-compose.yml`. Use a gitignored `.env`, or a secret plus
  `USARR_SECRET_KEY_FILE`.
* Never returned by the API — nor are stored credentials: the UI shows `••••••1a2b` and offers a
  server-side "Test connection" button.
* Redacted from logs, error messages and support bundles (§2.1).
* **Never in the same archive as the database.** §5 and §6 make that mechanical rather than a matter
  of remembering.

---

## 4. Metadata providers

**No external metadata provider is required, at any milestone, to render your library.** Radarr's
`MovieResource` and Sonarr's `SeriesResource` already carry title, original title, overview, year,
runtime, genres, certification, ratings, `tmdbId`, `imdbId`, `tvdbId` and `images[]` with
`remoteUrl` — every field v0.1 renders. External providers enter with unowned/discovery search
(v0.2) and cross-media (v0.3). Do not register a TMDB account to see your own library.

Provider keys are configured in the UI and stored encrypted; there are no provider environment
variables (§2.3). Attribution and rate limits are **product requirements**, not optional politeness.
State of the research as of 2026-08-16, verification marked per row.

| Provider | Key? | Rate limit | Constraints that bind UsArr |
|---|---|---|---|
| **TMDB** (film/TV) | Yes, [bring your own](https://www.themoviedb.org/settings/api) | ⚠️ Not published. The "~40 req/s" forum figure is not in the terms. Back off adaptively on 429; never hard-code a rate. | ✅ [Terms](https://www.themoviedb.org/api-terms-of-use): non-commercial only; **mandatory attribution** (TMDB logo plus the exact sentence *"This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB"*, logo less prominent than UsArr's); **caching capped at 6 months**; not an image host; **no sublicensing**, so UsArr ships no bundled key. |
| **TVmaze** (TV) | No | 📄 ≥20 calls / 10 s per IP | Free. Carries `externals.thetvdb` and `externals.imdb`, making it the practical free TVDB replacement. ⚠️ Contributed data reportedly CC BY-SA — verify before redistributing a derived set. |
| **TheTVDB v4** | Yes, **never free** | ⚠️ Unpublished | ✅ [FAQ 81](https://support.thetvdb.com/kb/faq.php?id=81): a negotiated commercial contract, or a "user-supported" key where **every end user** holds a paid subscription and supplies a PIN. Off by default; never a hard dependency. |
| **MusicBrainz** | No | ✅ **1 req/s per IP**, 503 above | Core data **CC0**. A descriptive `User-Agent` with contact info is **mandatory** ([docs](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting)). 1 req/s is not viable for library-scale scanning; the escape hatch is a [Live Data Feed](https://musicbrainz.org/doc/Live_Data_Feed) mirror ⚠️ whose OSS terms were not verified. **Do not route through `api.lidarr.audio`** — Servarr-run cache with documented lag and repeated breakage. |
| **Cover Art Archive** | No | ✅📄 none currently | Internet-Archive-backed, keyed by MusicBrainz MBID. |
| **Open Library** (books) | No | ✅ 1 req/s anonymous, **3 req/s with an identifying UA** | ✅ Terms forbid bulk download via the API — use the monthly dumps. ⚠️ Exact data licence not read in full: <https://openlibrary.org/developers/licensing>. Its Work/Edition model is what UsArr's book identity layer is built on. |
| **Wikidata** | No | ✅ `Special:EntityData/{QID}.json` and `query.wikidata.org/sparql` reliable under test; ⚠️ `w/api.php` throttled the researcher within a handful of sequential requests — **do not build a hot path on `api.php`**. SPARQL timeout 📄 60 s. | **CC0.** No attribution, no commercial restriction, no cache limit. The only unencumbered provider here, which is why it is UsArr's cross-media spine (v0.3). |
| **AniList** (anime) | No, for public GraphQL | ⚠️ Commonly cited as 90 req/min with `X-RateLimit-*` headers; `docs.anilist.co` returned 403 to automated fetch, and AniList has publicly run degraded at a lower limit for extended periods. **Read `X-RateLimit-Remaining`; do not trust the constant.** | Free. Pair with vendored ID-mapping files rather than solving anime numbering yourself. |
| **Metron** (comics) | Yes, HTTP Basic | ✅📄 20 req/min burst, 5 000/day; `If-Modified-Since` → 304 and `modified_gt` incremental sync | Community-run and far friendlier than Comic Vine. ⚠️ Limits reduced in early 2025 — re-check before launch. |

**Legend:** ✅ verified live during research · 📄 vendor docs or a reputable secondary source, not
independently exercised · ⚠️ unverified or volatile.

Also available, all enrichment-only and none load-bearing:
[Fanart.tv](https://fanart.tv/get-an-api-key/) (artwork, free project key, 📄 limits "rarely
applied"); Comic Vine (📄 200 req/resource/hour plus velocity blocks, ⚠️ caching and redistribution
terms for a self-hosted app **not read** — a fallback behind Metron); Trakt, OMDb, Google Books and
Hardcover. **IMDb has no free public API** — `tt…`/`nm…` ids are a lingua franca only, obtained via
TMDB `external_ids` or Wikidata `P345`, and the bulk datasets at `datasets.imdbws.com` are
⚠️ non-commercial-use-only.

### 4.1 Provider credentials never persist in a stored URL

Normative, because it is easy to get wrong and expensive to notice. TMDB v3, Fanart.tv and Comic Vine
all authenticate by **query parameter**. Therefore:

* Every URL written to `image_asset.source_url`, to the HTTP cache in `cache.db`, or to any log or
  audit row is stored **with credential parameters stripped**. The key is attached at request time
  from the configured provider credential.
* Cache keys are derived from the **stripped** URL, so rotating a provider key does not invalidate
  the entire image cache.
* An ingest-time assertion rejects writing any URL containing `api_key`, `apikey`, `key=`,
  `token` or `access_token` to either store. It fails the write and logs; it does not sanitise
  silently, because a silent sanitiser hides the bug that produced the URL.

Without this rule the key lands in the database, in every `VACUUM INTO` backup, in every support
bundle, and in the cache index — in a column that is not encrypted and was never meant to be.

### 4.2 TMDB, stated plainly

1. **Attribution** must be rendered wherever TMDB data appears, with the sentence verbatim and the
   logo less prominent than UsArr's own.
2. **The 6-month cache cap is a schema requirement.** TMDB-derived rows carry `provider` and
   `fetched_at`; a refresh job evicts anything older. This is enforced in code, not by a setting.
3. **UsArr ships no bundled TMDB key.** The terms forbid sublicensing and forbid concealing identity.
   Radarr and Sonarr ship a shared key *by arrangement with TMDB*. Until someone emails
   `tmdb@themoviedb.org` and gets written permission, users bring their own. ⚠️ Open item.
4. Where `watch/providers` (JustWatch) data is displayed, **per-item JustWatch attribution is
   mandatory** and TMDB revokes API access for non-compliance. ✅

---

## 5. Data directory layout — authoritative

This tree is the single source of truth for on-disk layout. `ARCHITECTURE.md` conforms to it. Every
mode is explicit and is set by UsArr itself, independent of any process umask.

```
$USARR_CONFIG_DIR/                  # /config — IRREPLACEABLE. Back this up. Mode 0700.
│
├── keys/                           # 0700 — EXCLUDED FROM EVERY BACKUP. See §6.
│   ├── secret.key                  # 0600 — the master key, when not supplied by env/secret
│   └── secret.key.new              # 0600 — present only mid-rotation (§3.4)
│
├── usarr.db                        # 0600 — SQLite, WAL. Library, services, users, audit log.
├── usarr.db-wal                    # 0600 ┐ transient; never copy these by hand — see §6
├── usarr.db-shm                    # 0600 ┘
│
├── providers/                      # 0700 — Tier 1 YAML service manifests. USER DATA,
│   └── *.yaml                      #   0600.  NOT regenerable. Never place these under DATA_DIR.
│
├── backups/                        # 0700 — `VACUUM INTO` output. Contains ciphertext, no key.
│   └── usarr-2026-08-16T03-00-00Z.db   # 0600
│
└── tsnet/                          # 0700 — embedded-node identity. Only exists from the
                                    #   milestone that ships tsnet (§9); persist it or every
                                    #   restart registers a new device.

$USARR_DATA_DIR/                    # /data — REGENERABLE. Safe to delete; UsArr rebuilds it. 0700.
│
├── cache.db                        # 0600 — HTTP/metadata response cache, disposable.
├── cache.db-wal                    # 0600 ┐ never ATTACHed inside a usarr.db write transaction
├── cache.db-shm                    # 0600 ┘
├── cache/
│   └── images/                     # proxied *Arr MediaCover + provider artwork, content-keyed
├── logs/
│   └── usarr.log                   # 0600, + rotated usarr.log.1 … (10 MB × 5)
└── tmp/                            # in-progress work; cleared at startup

Shipped inside the container image, on no volume:
/usr/share/usarr/wikidata-edges.db  # read-only cross-media edge artifact (v0.3).
                                    #   Edges only — (from_qid, to_qid, rel_type, evidence) plus
                                    #   the external ids needed to resolve each side. Single-digit
                                    #   MB. Regenerated per release by a committed SPARQL script,
                                    #   not by a weekly dump pipeline. Nothing degrades if it is
                                    #   months stale.
```

Three properties this layout exists to give you, each a defect in the previous version:

1. **"Safe to delete" is literally true for `$USARR_DATA_DIR`.** Nothing user-supplied lives there.
   `providers/` moved to `CONFIG_DIR` because a hand-written manifest is not regenerable, and there
   are no plugin binaries anywhere — **WASM plugins are cut from the project**, so the directory that
   used to destroy them on a cache clear does not exist.
2. **The key is one directory you can exclude with one flag.** `keys/` is the only thing that must
   stay out of an archive of `/config`, and §6 makes that the documented procedure rather than a
   warning you are expected to remember.
3. **Every file both this document and `ARCHITECTURE.md` name is here** — `cache.db`, `providers/`
   and the Wikidata edge artifact were previously named in one and absent from the other.

**Single-volume installs** leave `USARR_DATA_DIR` unset, so everything lands under `/config`. `keys/`
is still excluded from backups; the exclusion is by path, not by volume.

**NAS installs** should split them: `/config` on SSD, `/data` wherever the cache fits.

⚠️ **Do not put `usarr.db` on NFS, SMB or any network filesystem.** SQLite's locking is unreliable
there and WAL mode needs shared memory the remote filesystem does not provide. This is the single
most common corruption cause for SQLite-backed self-hosted apps.

---

## 6. Backup, restore, migration

### 6.1 What to back up, and what must never be in the same archive

| Item | Why | Recoverable without it? |
|---|---|---|
| `usarr.db` (via `backups/*.db`) | Library index, services, users, tags, links, requests, audit log | No |
| The master key | Decrypts every stored credential in that DB | The DB opens; every service credential is noise — re-paste each one (§3.5) |
| `providers/*.yaml` | Hand-written service manifests | Only if you still have the originals |
| `tsnet/` | The node's tailnet identity | Yes — restoring without it registers a **new** device; delete the old one in the admin console |
| `$USARR_DATA_DIR` | Caches, logs | Yes — rebuilt automatically |

🚩 **`tar -czf backup.tgz /config` is not a backup. It is a compromise.** It puts the ciphertext and
the key that opens it in one file, which is exactly the property encryption at rest exists to
prevent, and it is the mistake the previous version of this document invited by telling users to copy
`$USARR_CONFIG_DIR` wholesale.

The rule, mechanically:

```bash
# Database → your normal backup rotation.
tar -czf usarr-db.tgz --exclude='keys' /config
#                     ^^^^^^^^^^^^^^^^ non-negotiable

# Key → a password manager or secrets store. A DIFFERENT PLACE. Once.
cat /config/keys/secret.key
```

UsArr's scheduled job writes `VACUUM INTO` output to `$USARR_CONFIG_DIR/backups/` at mode `0600` in a
`0700` directory, retains N files, and **never** includes `keys/`. Same for the API backup endpoint
and the UI download button, which additionally require the cookie session and sudo mode (§1) — a
northbound API key can never reach them, and every download is audit-logged with actor and IP.

Encrypt the artifact itself (`age`, `gpg`) before it leaves the host for cloud storage: the database
holds every wrapped DEK, every password hash and the full audit log.

### 6.2 Taking one by hand

```bash
# Correct: atomic and consistent while UsArr is running.
sqlite3 /config/usarr.db "VACUUM INTO '/config/backups/usarr-$(date -u +%FT%TZ).db'"
# Also correct: stop UsArr, then copy the .db AND -wal AND -shm together.
```

**Wrong:** `cp usarr.db backup.db` on a running instance. In WAL mode the newest committed
transactions live in `usarr.db-wal`, so a bare copy of the main file is a torn, older database that
may not even open.

### 6.3 Restoring

1. Stop UsArr. Move the damaged `usarr.db`, `-wal` and `-shm` aside — do not delete them until the
   restore is confirmed good.
2. Copy the backup into place as `usarr.db`. There is no `-wal`/`-shm` to restore; SQLite recreates
   them.
3. **Restore the key separately**, to the value it had when the backup was taken, as
   `keys/secret.key` (`0600`) or via `USARR_SECRET_KEY_FILE`.
4. Start. Pending migrations run forward automatically; they are **forward-only (proposed)**, so
   restoring a newer backup into an older binary is not supported.
5. Open Settings → Services and confirm each instance tests green. A red test with a decryption error
   means the key does not match the backup.

### 6.4 Moving to a new host

Two steps, deliberately separate, deliberately not one `cp -r`:

1. Copy the latest `backups/*.db` (and `providers/` if you use manifests) to the new host as
   `$USARR_CONFIG_DIR/usarr.db`. Chown to `65532:65532`, mode `0600`.
2. Transfer the master key **out of band** — from your password manager, not from the archive — and
   install it as `keys/secret.key` (`0600`, in a `0700` directory) or as a Docker secret.

Then start. Service URLs pointing at `localhost` will need editing if the topology changed; this is
the common gotcha moving from bare metal to Docker, where `http://localhost:8989` must become
`http://sonarr:8989`.

---

## 7. Talking to your services

### 7.1 Outbound TLS: one mechanism, per instance

Certificate verification is a property of a **service instance**, stored on its database row
(`service_instance.verify_tls`, default on). **There is no global environment override** — a
host:port allowlist and a per-row flag are two gates with undefined interaction, and the global one
is the one that gets left on.

🚩 **What turning verification off actually does.** UsArr then sends
`X-Api-Key: <32-hex full-admin key>` to whatever answers the connection, with no proof it is your
Sonarr. On a shared network — a Docker bridge, a LAN, an overlay network — anything that can win a
DNS race, poison a container lookup or occupy a released container IP harvests every *Arr admin key
you have configured. Scoping the flag per instance limits the blast radius; it does not authenticate
anything.

**Prefer, in this order:**

1. **Plain HTTP over a network you already trust end-to-end** — the Docker bridge to a container in
   the same compose file, or an encrypted overlay such as a tailnet. Unverified TLS is *worse* than
   plain HTTP: same protection, plus a false sense of security.
2. **Import the CA.** Mount your homelab CA into the container's trust store and leave verification
   on. One step, permanent, and it authenticates.
3. **Pin the certificate.** On first connect to an untrusted certificate UsArr shows the SPKI
   fingerprint and stores it on the instance **(proposed)**; a silently changed pin is a loud error
   thereafter. Trust-on-first-use: weak on day one, strong every day after.
4. **Disable verification** only as a temporary escape hatch, with a per-request warning naming the
   instance. Not a setting you leave on.

The pinned-IP SSRF defence (`ARCHITECTURE.md` §14.2) must keep the **original hostname** for `Host`,
SNI and certificate verification while dialling the validated IP. A naive "connect to the pinned IP"
implementation verifies the certificate against an IP, fails, and the fix a developer reaches for is
`InsecureSkipVerify` — which is this entire section, arrived at by accident.

**Correction to earlier drafts:** "*Arr apps serve HTTPS on a self-signed cert on port 9898 by
default" is wrong twice. Servarr ships `EnableSsl` **false**, so out of the box they serve plain HTTP
only; and 9898 is *Sonarr's* `SslPort` — Radarr 7879, Lidarr 8687, Readarr 8788, Prowlarr 9697. (High
confidence on `EnableSsl` and Sonarr's 9898 from the shipped `SystemResource` fields; ⚠️ medium on the
other ports — verify before quoting them.)

### 7.2 Changing an instance's URL invalidates its credential

Normative. Changing the scheme, host or port of a saved instance **requires re-entering the API
key**, and "Test connection" against a modified URL uses only the key typed into the form, never the
stored one.

Without this rule the masked-credential display is cosmetic theatre: point `base_url` at a host you
control, click Test, and read the full-admin key off your own listener. The stored ciphertext is
additionally bound to the normalised host:port through the AEAD's additional authenticated data
(`ARCHITECTURE.md` §14.1), so a host change makes it fail to open rather than silently succeed.

### 7.3 Navidrome, concretely — because it is the one that does not work the obvious way

**✅ Verified 2026-08-16, against Navidrome `master` and the v0.63.2 release: Navidrome does not
support the OpenSubsonic `apiKeyAuthentication` extension.** Not in any released version, not on
`master`. Two pull requests are open and unmerged
([#4022](https://github.com/navidrome/navidrome/pull/4022),
[#5731](https://github.com/navidrome/navidrome/pull/5731)); the extensions tracker
([#2695](https://github.com/navidrome/navidrome/issues/2695)) has the box unchecked. The advertised
extension list in `server/subsonic/opensubsonic.go` is `transcodeOffset`, `formPost`, `songLyrics`,
`indexBasedQueue`, `transcoding`, `playbackReport`, `topSongsByArtistId` and (conditionally)
`sonicSimilarity`. Any earlier UsArr document that told you to "prefer an API key" for Navidrome was
wrong.

**Check your own server in one command, no credentials needed** — `getOpenSubsonicExtensions` is the
only route registered outside Navidrome's authentication group:

```bash
curl -s 'https://navidrome.example/rest/getOpenSubsonicExtensions?v=1.16.1&c=UsArr&f=json'
```

The response carries `serverVersion` and the extension list. If `apiKeyAuthentication` is absent —
it will be — this is the flow UsArr uses instead, and it **stores no recoverable password**:

1. You supply the Navidrome URL, username and password **once**, in the connection wizard.
2. UsArr calls `POST /auth/login`.
3. The response includes **`subsonicSalt`** and **`subsonicToken`** — the classic `md5(password+salt)`
   pair.
4. UsArr stores *that pair*, encrypted like any credential, and discards the password. It never
   writes the password to disk and never sends it again.
5. Subsonic calls then authenticate with `s=<salt>&t=<token>`. The pair stays valid until you change
   your Navidrome password, at which point the instance is marked `needs_credential` and you pair
   again.

Two honest caveats. This is the salt/token scheme, which the OpenSubsonic spec *recommends* servers
drop once they offer API-key auth — moot here, since there is no API-key auth to prefer. And
Navidrome still stores a recoverable password on its own side; that is a property of Navidrome, not
something UsArr can fix, and it is why the pair is encrypted rather than treated as low-value.

Navidrome as a *southbound* library source is **v1.0**; UsArr acting as an OpenSubsonic *server* is
**v0.4**. Neither is needed to start.

---

## 8. Worked examples

### 8.1 Direct port on a trusted LAN (the v0.1 default)

```yaml
services:
  usarr:
    image: ghcr.io/<owner>/usarr:latest      # not yet published — pre-alpha
    container_name: usarr
    user: "65532:65532"                      # matches the distroless nonroot UID
    environment:
      - TZ=Europe/Lisbon
      # USARR_SECRET_KEY is deliberately absent: UsArr generates one on first
      # run into /config/keys/secret.key and tells you to back it up. See §3.2.
    volumes:
      - ./config:/config                     # pre-created and chowned 65532:65532
      - ./data:/data
    ports:
      - "8484:8484"
    restart: unless-stopped
```

```bash
mkdir -p ./config ./data && sudo chown -R 65532:65532 ./config ./data && sudo chmod 700 ./config ./data
docker compose up -d
docker compose logs usarr | grep -i 'master key'   # back this up now
```

### 8.2 Behind a reverse proxy at a sub-path

```yaml
services:
  usarr:
    image: ghcr.io/<owner>/usarr:latest
    user: "65532:65532"
    environment:
      - TZ=Europe/Lisbon
      - USARR_URL_BASE=/usarr
      - USARR_BIND_ADDRESS=0.0.0.0
      - USARR_TRUSTED_PROXIES=172.18.0.0/16   # the proxy's network, and nothing wider
      - USARR_SECRET_KEY_FILE=/run/secrets/usarr_key
    volumes:
      - ./config:/config
      - ./data:/data
    networks: [proxy]                          # NOT published to the LAN
    secrets: [usarr_key]
    restart: unless-stopped

secrets:
  usarr_key:
    file: ./secrets/usarr_key                  # contains ONLY the base64 key
```

`USARR_TRUSTED_PROXIES` must contain the network the proxy reaches UsArr from and nothing wider. Do
not also publish the port to your LAN — anyone on the LAN would bypass the proxy entirely.

Using a secret file here rather than the first-run generated key is the right call when your proxy
host already has a secrets store; it keeps the key off the config volume entirely, which makes the
§6 backup rule impossible to get wrong.

---

## 9. Later milestone: embedded Tailscale node (`tsnet`)

**Not in v0.1. Not implemented. Not in `.env.example`.** Documented so the design is settled before
anyone builds it, and so the security rules are written down while they are cheapest to get right.
All **(proposed)**.

[`tailscale.com/tsnet`](https://pkg.go.dev/tailscale.com/tsnet) embeds a full Tailscale node inside a
Go process: userspace TCP/IP stack, no `tailscaled`, no root, no system network configuration
([docs](https://tailscale.com/kb/1244/tsnet)). UsArr would appear as its own device at
`usarr.<tailnet>.ts.net`, publish no host port, and take TLS from the tailnet certificate.

Planned variables, five: `USARR_TSNET_ENABLED`, `USARR_TSNET_HOSTNAME`, `USARR_TSNET_STATE_DIR`,
`USARR_TSNET_AUTHKEY_FILE`, `USARR_TSNET_AUTH_ALLOWED_LOGINS`.

Rules settled now, because they are security-relevant:

1. 🚩 **`USARR_TSNET_AUTH_ALLOWED_LOGINS` fails closed.** Empty means **nobody** is authenticated by
   the tailnet identity path — never everybody. Enabling tailnet identity with an empty allowlist is
   a **startup error**. A tailnet is "a small trusted group", not "zero adversaries": it includes a
   contractor's laptop, a family member's phone and every tagged CI runner.
2. **A caller with no resolvable user identity is unauthenticated, full stop.** `WhoIs` on a *tagged*
   device (one authenticated by ACL tag rather than owned by a user) is expected to yield no user;
   Tailscale documents that Serve's identity headers are populated only for users, not tagged
   devices. ⚠️ Unverified against a live tailnet — and the rule is written fail-closed precisely
   because it is unverified.
3. **In single-user mode there is no auto-create.** A tsnet login matching the allowlist
   authenticates **as the owner**; anything else is refused. Never "the first tailnet user to connect
   becomes the owner".
4. **The auth key is a secret**, needed exactly once. Non-reusable, **not** ephemeral (an ephemeral
   node vanishes on restart), pre-authorized, tagged, short expiry. Delete it from the environment
   after first registration; `tsnet/` state authenticates the node from then on — and if it is not
   persisted you get a new device every restart and a tailnet filling with `usarr-1`, `usarr-2`, ….
5. **Nothing listens on localhost** in this mode, so container healthchecks break. The fix is a
   loopback listener exposing **exactly** `/api/health/live` and `/api/health/ready` — never an admin
   surface. Inside a container, "loopback" is shared with every sidecar and anything running
   `network_mode: service:usarr`, so an unauthenticated admin listener there is a privilege-escalation
   path, not a convenience.

Two things a tailnet does **not** solve, both of which apply from v0.1:

* **The master key is still required.** A private network does not encrypt your database. Anyone with
  the volume has the credentials without it.
* **SSRF policy still matters, and a tailnet makes it broader** — reachable private space gains the
  CGNAT range `100.64.0.0/10` on top of RFC1918. Note the nuance (`ARCHITECTURE.md` §14.2): artwork
  from a service instance you configured is validated against **that instance's own resolved
  host:port** and permitted there regardless of range, because on a tailnet your Komga *is* at
  `100.x.y.z`. Only URLs with no originating instance — TMDB, Fanart.tv, Cover Art Archive — get the
  blanket deny-list. "A poster URL has no business resolving to a tailnet peer" is false on this
  deployment and has been deleted.

---

## 10. Open configuration questions

Tracked so they are not silently forgotten. Resolved items have been removed rather than left as
history.

* Whether TMDB will grant UsArr a bundled key, or users must always bring their own (§4.2).
* Kavita's REST auth scheme — unconfirmed from primary sources (v1.0 provider).
* Audiobookshelf's exact play-session request schema, needed for progress mirroring (v1.0).
* What `tsnet`'s `WhoIs` returns for a tagged device (§9 rule 2). The rule is fail-closed either way;
  verification decides whether tagged devices can ever authenticate at all.
* Whether a Tailscale OAuth client is the right long-lived alternative to an auth key for unattended
  re-registration (§9).
* MusicBrainz Live Data Feed access terms for an OSS project (§4).
* Open Library's exact data licence (§4).
* Whether `USARR_PORT` default 8484 collides with anything common in the self-hosting ecosystem.

**Resolved since the last revision:** Navidrome's `apiKeyAuthentication` support (it has none — §7.3);
env-vs-database precedence for service rows (there is no env channel — §1); global vs per-instance
TLS verification (per-instance only — §7.1); the master key's startup behaviour (§3.2).
