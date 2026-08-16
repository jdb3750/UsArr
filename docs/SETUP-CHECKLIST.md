# UsArr — Owner Setup Checklist

**For: Joe.** What you personally need to decide, obtain or stand up, ranked by whether it actually
blocks work. Ranked against `docs/ARCHITECTURE.md` §16, which owns the roadmap:

> **v0.1** unified library + search · **v0.2** requests · **v0.3** cross-media · **v0.4** gateway ·
> **v1.0** breadth

**Read this first, because it changes the urgency of everything below.** Almost nothing here blocks
the first few thousand lines of code. `internal/config`, `internal/db`, `internal/crypto`, the
migration harness, the search schema and the SvelteKit shell need zero credentials, and the *Arr
clients are developed against **committed HTTP cassettes and the vendored OpenAPI specs**
(`docs/DEVELOPMENT.md` §7), not against a live stack.

What credentials buy is the ability to **record** a cassette. Once one is committed, that integration
is testable forever by anyone with no stack at all — including you, today.

**Your actual starting position:** you run **no Sonarr, no Radarr, no Lidarr**, and you run
**Navidrome**. That is an unusual starting point for this project and this checklist is written for
it rather than for the imaginary user who already has a full stack.

---

## 1. Decisions only you can make — these block the first commit

Nothing can be written until these are fixed. They cost a choice, not money.

| # | Decision | Why it blocks | Notes |
|---|---|---|---|
| 1.1 | **Go module path** — e.g. `github.com/joe/UsArr` | `go mod init` needs it and every import bakes it in. Changing it later is a repo-wide rewrite. | Must match the eventual GitHub owner/repo exactly. Decide the owner and repo name now, even if the repo stays private for a while. This also settles the public project name. |
| 1.2 | **License** | Contributions into a repo with no LICENSE have undefined terms, and retro-licensing means chasing every contributor. The README agrees: confirmation is needed **before the first PR is accepted**, not before the first release. | AGPL-3.0 is the self-hosted-media norm (Jellyfin) and blocks a hosted-SaaS fork; MIT/Apache-2.0 is friendlier to embedding. Either is fine. Deferring is not. **Put the file in the first commit.** |
| 1.3 | **Default HTTP port** — proposed `8484` | Goes in the docs, the compose file, the Dockerfile `EXPOSE` and every example. Cheap now, annoying later. | Chosen to avoid every ecosystem default (8989/7878/8686/8787/9696/6969/5299/8096/32400/8080/6789/9091/8112/6767/5055). Confirm or replace. |

That is the whole blocking list. Note what is **not** on it: no API keys, no accounts, no tailnet, no
master key.

---

## 2. The one thing v0.1 needs from your homelab

v0.1 renders a unified library. **A library needs a library-bearing service**, and Prowlarr is not
one — it indexes indexers, it holds no movies or series. Running UsArr against Prowlarr alone
produces a working install with nothing in it, which is the worst possible first impression. So:

### 2.1 Stand up Prowlarr first — 9696

Not because UsArr needs it first, but because **you** do. Every *Arr you add afterwards gets its
indexers from Prowlarr in one place instead of being configured five times, and Prowlarr is the
smallest, fastest container in the stack to get right. It is also the only source of the
`usenet` vs `torrent` protocol tag and of indexer/category metadata, which is a genuine UsArr
differentiator.

Key: Settings → General → Security → API Key. 32 hex characters, **full admin** on that instance.

⚠️ One correction to earlier notes: Prowlarr's `/history/since` returns **indexer query telemetry**
(`indexerQuery`, `indexerRss`, `indexerAuth`), not library changes, and its `HistoryResource` has no
`movieId`/`seriesId`. UsArr uses Prowlarr for search and provenance, never as a delta-sync source.

### 2.2 Then one *Arr — Radarr (7878) or Sonarr (8989)

This is the thing that makes v0.1 render. One is enough to build and demo against; two of the same
kind at different quality profiles is what makes the "1080p ✓ / 4K ✗" per-instance view interesting,
but that is a bonus, not a requirement.

**Radarr is the easier one to seed** — movies are single rows, whereas 2 000 series means ~400 000
episode rows and one `/api/v3/episode?seriesId=N` call per series on first import.

Key: Settings → General → Security → API Key, same shape as Prowlarr's.

### 2.3 What I need from you, concretely

For each app you stand up: one line of `kind | url | api key | urlbase (if any)`. **Through a secret
channel, not chat.**

* The URL must be **as UsArr will reach it** — container name inside Docker (`http://radarr:7878`),
  IP or hostname otherwise. `http://localhost:7878` is the classic wrong answer from inside a
  container.
* If you set a `UrlBase` (e.g. `/radarr`), say so — every path shifts, including `/radarr/api/v3/…`
  and `/radarr/ping`, and the connection wizard has to handle it.
* You can also read the key from each app's `config.xml` (`<ApiKey>`) if you share the volume.
* Never paste one into an issue or a chat. It can delete the library.

### 2.4 Also useful, and free: how big is your library going to be?

Rough series / film / album / book counts. It changes real decisions — `/series` and `/movie` are
**not paged** in the *Arr APIs and serialise the entire object graph on every call, so a 2 000-series
library and a 50-series library imply different import and caching strategies.

### 2.5 Not needed: a master key

Earlier drafts told you to run `openssl rand -base64 32` and put it in `.env`. Don't. UsArr generates
a real key on first run into `$USARR_CONFIG_DIR/keys/secret.key` at mode `0600` and logs a line
telling you to back it up. That path has more entropy than anything you would paste, and it cannot
accidentally be the placeholder from a public file. Copy the generated key into your password manager
**once**, and keep it out of the same archive as the database (`docs/CONFIGURATION.md` §6).

---

## 3. Navidrome — what you already run

You have it; it is worth knowing where it lands.

* **As a library source** (music appearing in your unified library): **v1.0**.
* **As a client target** (UsArr speaking OpenSubsonic *to* Symfonium and friends): **v0.4**.
* Neither blocks anything now.

**✅ Verified 2026-08-16: Navidrome does not support the OpenSubsonic `apiKeyAuthentication`
extension** — not in v0.63.2, not on `master`. Two PRs are open and unmerged
([#4022](https://github.com/navidrome/navidrome/pull/4022),
[#5731](https://github.com/navidrome/navidrome/pull/5731)) and the extensions tracker
([#2695](https://github.com/navidrome/navidrome/issues/2695)) has the box unchecked. So the earlier
instruction — "tell me whether your version exposes it" — was asking a question whose answer is
already known, and the "if it lacks it, that integration is blocked on upgrading Navidrome" line was
simply wrong. There is nothing to upgrade to.

**Confirm it yourself in one command, no credentials needed** (`getOpenSubsonicExtensions` is the only
route outside Navidrome's auth group):

```bash
curl -s 'https://<your-navidrome>/rest/getOpenSubsonicExtensions?v=1.16.1&c=UsArr&f=json'
```

**What UsArr will do instead**, and it does not store a recoverable password: you supply the URL,
username and password **once** in the wizard; UsArr calls `POST /auth/login`, keeps the
`subsonicSalt` + `subsonicToken` pair it gets back (encrypted, like every credential), and discards
the password. Details in `docs/CONFIGURATION.md` §7.3.

**Nothing for you to do today.** When v1.0 approaches, one pairing in the UI.

---

## 4. Later, by milestone — nothing here is blocking

Supply these when the milestone that uses them is being built, not before.

| Item | Needed at | Cost | Notes |
|---|---|---|---|
| **Lidarr** (8686, `/api/v1`) | v1.0 | free | Only if you want music from the acquisition side rather than from Navidrome. |
| **LazyLibrarian** (5299) | v1.0 | free | Books/magazines/comics. Key must be **exactly 32 characters** or it returns `503 Invalid API key`. Prefer its read-only key. |
| **Jellyfin** (8096) | v1.0 | free | Video library aggregation and the "open in Jellyfin" handoff. **Not blocking, and never was** — v0.1 sources 100% of its metadata and poster URLs from the *Arrs. When it lands, give the wizard admin credentials once and let UsArr mint its own key via `/Auth/Keys` rather than storing a password; tell me the server version, since 10.11 changed which auth header the server accepts. |
| **Audiobookshelf** (13378) | v1.0 | free | Audiobooks; ABS stays the source of truth for listening position, UsArr mirrors it. |
| **Komga** (25600) / **Kavita** (5000) | v1.0 | free | Comics and ebooks. ⚠️ Kavita's REST auth scheme is unconfirmed from primary sources. |
| **Bazarr** (6767) | v1.0 | free | Subtitle status. Joins via the *Arr instance's local IDs, not external IDs. |
| **TMDB key** | v0.2 | free, non-commercial | **Not blocking, and never was.** Radarr's `MovieResource` and Sonarr's `SeriesResource` already carry title, overview, year, runtime, genres, certification, ratings, `tmdbId`, `imdbId` and `images[].remoteUrl` — every field v0.1 renders. TMDB enters only with discovery search for things you don't own. When it does, you accept: mandatory in-UI attribution with the TMDB logo less prominent than UsArr's; a **6-month cache cap**; and no sublicensing, which is why every user brings their own key. |
| **Metadata contact string** | v0.3 | free | The value of `USARR_METADATA_USER_AGENT`, e.g. `UsArr/0.1 (+https://github.com/joe/UsArr)`. MusicBrainz **requires** an identifying UA (1 req/s, 503 above); Open Library raises you 1 → 3 req/s for one. A project URL is fine; it does not have to be your email, and ⚠️ whatever you put is transmitted to those providers. |
| **Metron account** (comics) | v1.0 | free | Preferred over Comic Vine — friendlier limits, `If-Modified-Since` and incremental sync. |
| **Fanart.tv / Trakt / OMDb / Google Books / Hardcover / AniList keys** | v1.0 | free tiers | Enrichment only. Skip until something visibly lacks artwork or ratings. |
| **A tailnet + auth key** | v1.0 | free tier | `tsnet` is a later milestone, deliberately (`docs/CONFIGURATION.md` §9). **v0.1 binds a port**: publish it on your LAN, or put a reverse proxy in front. Do not create a tailnet auth key for this project yet — an unused secret is a liability, not preparation. |
| **GHCR / package publishing** | first release | free for public | Not needed until there is something to release. |

---

## 5. Explicitly NOT needed — nothing here is anything for you to obtain

So nobody chases them. Some are permanent non-goals; the ones marked **deferred** are wanted later
but are on no milestone and need nothing from you now (`docs/FUTURE.md`).

* **TheTVDB v4 key.** There is no free path: either a negotiated commercial contract or a
  "user-supported" key requiring *every end user* to hold a paid subscription and supply a PIN
  ([FAQ 81](https://support.thetvdb.com/kb/faq.php?id=81)). The default TV path is TMDB + TVmaze, both
  free.
* **Meilisearch.** **Deferred** (`docs/FUTURE.md` §2), on no milestone. SQLite FTS5 is the search
  engine, not a fallback — the library is ~50k short strings, not 50M documents. Nothing to install.
* **WASM plugins.** **Deferred** (`docs/FUTURE.md` §1), on no milestone. No `/plugins` directory, no
  sandbox, nothing to install.
* **OIDC, passkeys/WebAuthn, TOTP, forward-auth.** **Deferred** (`docs/FUTURE.md` §4). v0.1
  authentication is one local account with Argon2id and a server-side session cookie.
* **Goodreads API** — dead. Stopped issuing keys 2020-12-08 and revoked existing ones; this is why
  Readarr was archived (2025-06-27).
* **IMDb API** — does not exist publicly. `tt…` IDs come free via TMDB `external_ids` or Wikidata
  `P345`.
* **Wikidata credentials** — none. CC0, no key, no attribution, no cache limit.
* **TVmaze, Cover Art Archive, Open Library keys** — none required.
* **FFmpeg, transcoding hardware, `/dev/dri` passthrough, a GPU** — UsArr never transcodes anything,
  and never runs FFmpeg. (It does copy audio and ebook bytes on its own OpenSubsonic/OPDS surfaces,
  which needs no codec and no hardware.) If a conversation heads toward hardware acceleration,
  something has gone wrong architecturally.
* **PUID/PGID.** The container is distroless and runs as a fixed non-root `65532:65532`. Pre-chown
  your volumes, or set `user:` in compose. `docs/CONFIGURATION.md` §2.4.
* **User management.** v0.1 is single-user; there is no signup, and multi-user is v1.0.
* **A public domain name or TLS certificate** — only if you choose to expose UsArr to the internet,
  and then it is your reverse proxy's job.

---

## 6. The short version

**Three things and development is unblocked:**

1. **Decide the module path, the license and the port** (§1). Costs ten minutes, blocks the first
   commit.
2. **Stand up Prowlarr** (§2.1) and send me `kind | url | api key` through a secret channel.
3. **Stand up one *Arr — Radarr is easiest** (§2.2) and send the same. This is the thing that makes
   v0.1 have anything to show.

That is it. No TMDB account, no Jellyfin, no tailnet, no master key.

Everything in §4 arrives per-milestone as those integrations get built. Everything in §5 needs
nothing from you — permanently for the non-goals, and for now for the deferred items.
