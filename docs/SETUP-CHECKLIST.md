# UsArr — Owner Setup Checklist

**For: Joe.** What you personally need to obtain, decide, or supply before development can proceed.
Ranked strictly by whether it blocks work. No filler.

**Read this first, because it changes the urgency of everything below:** almost none of this blocks
the *first* few thousand lines of code. `internal/config`, `internal/db`, `internal/crypto`, the
migration harness, the tag schema, and the SvelteKit shell all need zero credentials. What credentials
buy is the ability to **record HTTP cassettes** (`docs/DEVELOPMENT.md §7.2`) — and once a cassette is
committed, that integration is testable forever by anyone with no stack at all. So the practical
sequence is: decide the four things in §0, then get *Arr + Prowlarr + Jellyfin + TMDB, record, and
everything else can trail.

---

## 0. Decisions only you can make — BLOCKING, and they cost nothing but a choice

These block the **first commit**, not a feature. Nothing can be written until they are fixed.

| # | Decision | Why it blocks | Notes |
|---|---|---|---|
| 0.1 | **Go module path** — e.g. `github.com/joe/UsArr` | `go mod init` needs it, and every import in the repo bakes it in. Changing it later is a repo-wide rewrite. | Must match the eventual GitHub owner/repo exactly. Decide the repo name and owner *now*, even if the repo is private for a while. |
| 0.2 | **License** | It is an open-source project; the LICENSE file is expected in the first commit, and contributors will ask. | AGPL-3.0 is the self-hosted-media norm (Jellyfin) and prevents a hosted-SaaS fork; MIT/Apache-2.0 is friendlier to embedding. Pick one; do not defer. |
| 0.3 | **Public project name and URL for attribution** | TMDB attribution requires the TMDB logo be *less prominent than yours*, so a name/mark has to exist. MusicBrainz and Open Library require a `User-Agent` carrying a contact URL or email. | The contact can be a GitHub URL — it does not have to be your personal email. |
| 0.4 | **Default HTTP port** — proposed `8484` | It goes in the docs, the compose file, the Dockerfile `EXPOSE`, and every example. Cheap now, annoying later. | 8484 was chosen to avoid every ecosystem default (8989/7878/8686/8787/9696/6969/5299/8096/32400/8080/6789/9091/8112/6767/5055/7700). Confirm or replace. |

---

## 1. BLOCKING credentials — needed to build and test the core

### 1.1 Master encryption key — 1 minute, free, no signup

```bash
openssl rand -base64 32
```

Put it in your local `.env` as `USARR_SECRET_KEY`. **Blocking** in the sense that UsArr refuses to
start without one, but it costs you a single command. Store the real deployment key in a password
manager — losing it means re-pasting every service API key. It is not the same secret as your dev
key; do not reuse.

### 1.2 The *Arr stack — BLOCKING for requests, cross-media search, and source tagging

Requests are a first-class feature: a user asks for something not in the library and UsArr routes it
to the right *Arr by media type. **Without these, the request feature has nowhere to send anything**,
and the `usenet` vs `torrent` source tag — one of UsArr's differentiators — has no data source.

For each app you run, supply the **base URL as UsArr will reach it** (container name inside Docker,
IP or hostname otherwise) and the **API key**.

| App | Default port | Where the key lives | Blocking for |
|---|---|---|---|
| **Prowlarr** | 9696 | Settings → General → Security → API Key | **Highest priority.** Cross-indexer search, the `protocol: usenet\|torrent` tag, indexer privacy/category metadata. Nothing else provides these. |
| **Sonarr** | 8989 | Settings → General → Security → API Key | TV library + TV requests |
| **Radarr** | 7878 | Settings → General → Security → API Key | Film library + film requests |
| **Lidarr** | 8686 | Settings → General → Security → API Key | Music library + music requests |
| **LazyLibrarian** | 5299 | Config → Interface → API. Key must be **exactly 32 characters**. | Books/magazines/comics requests. Give UsArr the **read-only** key unless you want it queueing books. |

Notes that will save you time:

* *Arr API keys are 32 hex characters and are **full admin** on that instance — they can delete your
  library. Treat them accordingly; never paste one into an issue or a chat.
* You can also read them from each app's `config.xml` (`<ApiKey>`) if UsArr shares the volume.
* If you run **Readarr**: it is retired upstream (archived 2025-06-27, metadata backend gone). Its
  `/api/v1` still responds, so UsArr can read an existing library, but do not plan around it.
* If you use a `UrlBase` on any app (e.g. `/sonarr`), say so — every path shifts, including
  `/sonarr/api/v3/...` and `/sonarr/ping`, and the connection wizard has to handle it.

**What I need from you concretely:** for each app you run, one line of
`kind | url | api key | urlbase (if any)`. Send them through a secret channel, not chat.

### 1.3 Jellyfin — BLOCKING for playback handoff

UsArr does **not** stream or transcode. Every "Play" hands off to a backend, and Jellyfin is the
primary one.

* **URL** (default port 8096) and an **API key** from Dashboard → API Keys.
* Alternatively give the setup wizard admin credentials **once** and let UsArr mint its own key via
  `/Auth/Keys` — that way no admin password is ever stored. This is what Jellyseerr does and is the
  better path.
* Tell me the **server version**. Jellyfin 10.11 changed which auth header the server accepts and
  broke clients that only sent the old one; the adapter needs to know what it is targeting.

### 1.4 TMDB API key — BLOCKING for film/TV metadata

* **Sign up:** <https://www.themoviedb.org/signup> then request a key at
  <https://www.themoviedb.org/settings/api>
* **Cost:** free for non-commercial use.
* **Obligations you are accepting** (these are product requirements, not fine print):
  * The TMDB logo plus the exact sentence *"This product uses TMDB and the TMDB APIs but is not
    endorsed, certified, or otherwise approved by TMDB"* must appear in the UI, with TMDB's logo
    **less prominent than UsArr's own** — which is why §0.3 exists.
  * **Cached TMDB data may not be kept longer than 6 months.** This is a schema constraint, built in
    from day one.
  * No sublicensing — which is why **UsArr will not ship a shared key** and every user brings their
    own. Radarr and Sonarr ship one *by arrangement with TMDB*.
* **Optional follow-up, not blocking:** email `tmdb@themoviedb.org` asking whether a self-hosted OSS
  project may bundle a key. Until they say yes in writing, users bring their own.

### 1.5 Metadata contact string — free, 2 minutes, blocks compliance not code

Decide the value of `USARR_METADATA_USER_AGENT`, e.g. `UsArr/0.1 ( https://github.com/joe/UsArr )`.

* **MusicBrainz requires** a descriptive `User-Agent` with contact info and enforces **1 request per
  second** (503 above it).
* **Open Library** raises you from 1 to 3 req/s if you send one, and its terms explicitly forbid bulk
  downloading via the API — use their monthly dumps instead.

No signup for either. This is a decision, not a credential.

---

## 2. BLOCKING for the default deployment model — Tailscale

The assumed deployment is a **private tailnet**, not the public internet. This blocks the deployment
path and the identity design, but **not** local development (set `USARR_TSNET_ENABLED=false` and work
on a normal port).

| Item | Where | Cost | Notes |
|---|---|---|---|
| A tailnet | <https://login.tailscale.com/start> | Free tier is ample for a homelab | You probably already have one — confirm. |
| Auth key | <https://login.tailscale.com/admin/settings/keys> | Free | **This is a secret** — it can add a device to your tailnet. Generate it **non-reusable**, **not ephemeral** (the node must survive restarts), pre-authorized, tagged (e.g. `tag:usarr`), short expiry. Needed only for first registration; delete it from the environment afterwards. |
| MagicDNS + HTTPS certs enabled | Tailscale admin → DNS | Free | Needed for `usarr.<tailnet>.ts.net` with automatic TLS. Confirm both are on. |
| Your tailnet login | — | — | Goes in `USARR_TSNET_AUTH_ALLOWED_LOGINS`. Leaving it empty means *any* tailnet user is admitted — fine on a solo tailnet, wrong on a shared one. Tell me which yours is. |

**Decide also:** embedded node (`tsnet`, recommended — no published ports, cryptographic identity) vs
Tailscale Serve in front of a loopback listener (weaker: header-based identity, only safe when bound
to `127.0.0.1`). See `docs/CONFIGURATION.md §6`.

---

## 3. HIGH — blocks one subsystem each, supply for whatever you actually run

Only needed for backends you run. Each missing one means that media type is absent from the unified
library; nothing else breaks.

| Backend | Default port | Where to get the key | Blocks |
|---|---|---|---|
| **Navidrome** (music) | 4533 | Settings → your user. **Tell me whether your version exposes the OpenSubsonic `apiKeyAuthentication` extension** — UsArr will not implement the legacy Subsonic salt/token scheme, because it requires the server to store a recoverable password. If your version lacks it, that integration is blocked on upgrading Navidrome. | Music aggregation and playback handoff |
| **Audiobookshelf** | 13378 | Settings → Users → API token (or a token from `/login`) | Audiobooks; listening-position sync |
| **Komga** (comics/manga) | 25600 | Account settings → API Keys (`X-API-Key`) | Comics/manga aggregation, OPDS |
| **Kavita** (comics/ebooks) | 5000 | User settings → API Key | Ebook/comic aggregation |
| **Bazarr** (subtitles) | 6767 | Settings → General → Security (`X-API-KEY`) | Subtitle status in the unified view |
| **Plex** | 32400 | `X-Plex-Token` — see <https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/> | Secondary/migration source only. Plex paywalled *remote* playback of personal media in 2025, so it is not a viable primary. |

**Also useful, no credential needed:** tell me roughly **how big your library is** (series count,
film count, track count, book count). It changes real decisions — `/series` and `/movie` are *not
paged* in the *Arr APIs and serialise the entire object graph on every call, so a 2 000-series
library and a 50-series library imply different caching strategies.

---

## 4. OPTIONAL — nothing breaks without these

| Item | Where | Cost | Skip unless |
|---|---|---|---|
| **TheTVDB v4 key** | <https://thetvdb.com/api-information> | **Not free.** Either a negotiated commercial contract, or a "user-supported" key requiring *every end user* to hold a paid subscription and supply a PIN. | **Recommend: skip entirely.** The default TV path is TMDB + TVmaze (free, no key) plus AniList for anime. Only relevant if you already pay for TVDB. |
| **Hardcover token** (books) | <https://hardcover.app> → account → API | Free, ~60 req/min | You want book covers/series/ratings beyond Open Library. Per-user token by design. ⚠️ Their terms for third-party self-hosted apps are unverified. |
| **Google Books key** (books) | <https://console.cloud.google.com/apis/library/books.googleapis.com> | Free tier | ISBN gap-filling becomes a problem |
| **Metron account** (comics) | <https://metron.cloud/> | Free, HTTP Basic | You have a comics library. Preferred over Comic Vine — friendlier limits, `If-Modified-Since` and incremental sync. |
| **Comic Vine key** | <https://comicvine.gamespot.com/api/> | Free with registration | Metron's coverage falls short. ⚠️ Its caching/redistribution terms were never read. |
| **Fanart.tv key** | <https://fanart.tv/get-an-api-key/> | Free project key | You want better artwork than TMDB provides |
| **Trakt OAuth client** | <https://trakt.tv/oauth/applications> | Free | You want watch-state sync. It mirrors TMDB for metadata, so it is not a metadata source. |
| **OMDb key** | <https://www.omdbapi.com/apikey.aspx> | Free tier 1 000 req/day | You specifically want IMDb-derived ratings |
| **AniList OAuth client** | <https://anilist.co/settings/developer> | Free | You need per-user anime lists. Public metadata needs no key. |
| **Meilisearch instance + key** | self-hosted, port 7700 | Free | Your library outgrows SQLite FTS5. FTS5 is the **supported default**, not a fallback — do not stand up Meilisearch preemptively. |
| **Download client credentials** (SABnzbd, qBittorrent, …) | Each client's own settings | Free | You want queue/history detail beyond what the *Arr APIs already expose. The *Arrs surface most of it. |
| **GitHub Container Registry / package publishing** | GitHub settings | Free for public | You want `ghcr.io/<owner>/usarr` images published. Not needed until there is something to release. |

---

## 5. Explicitly NOT needed

So nobody wastes time chasing them:

* **Goodreads API** — dead. Stopped issuing keys 2020-12-08; existing keys revoked. This is why
  Readarr was archived.
* **IMDb API** — does not exist publicly. `tt…` IDs come free via TMDB `external_ids` or Wikidata.
* **Wikidata credentials** — none. CC0, no key, no attribution requirement, no cache limit. It is
  the cross-media spine precisely because it has no strings attached.
* **TVmaze, Cover Art Archive, Open Library keys** — none required.
* **FFmpeg, any transcoding hardware, `/dev/dri` passthrough, GPU** — UsArr does not touch media
  bytes. Playback is delegated. If a conversation starts heading toward hardware acceleration,
  something has gone wrong architecturally.
* **User-management setup** — v0.1 is single-user. `USARR_ENABLE_SIGNUP` exists and is inert.
  Nothing about roles, quotas, or per-user library visibility blocks v0.1.
* **A public domain name, TLS certificate, or reverse proxy** — not needed on a tailnet. Tailscale
  provides the name and the certificate.

---

## 6. The short version

**Do these four things and development is unblocked:**

1. Fix the module path, license, project name/URL, and default port (§0).
2. Send me `kind | url | api key` for **Prowlarr + whichever *Arrs you run** and for **Jellyfin**
   (§1.2, §1.3) — through a secret channel, not chat.
3. Register a free **TMDB** key (§1.4).
4. Confirm the **tailnet** exists with MagicDNS + HTTPS on, and give me your tailnet login (§2).

Everything in §3 can arrive per-backend as those integrations get built. Everything in §4 is a
someday. Everything in §5, ignore.
