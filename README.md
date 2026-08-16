# UsArr

> **Status: pre-alpha. Nothing is implemented yet.**
> This repository currently contains a design and its evidence base — no code, no binary, no
> container. There is nothing to install. If you are here to run something, come back later.

**One unified, searchable catalogue over everything you own and everything you might want —
that plugs into the players you already use.**

---

## What it is

UsArr is a self-hosted **aggregation gateway** for the whole media-acquisition ecosystem. It
sits over Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian and the post-Readarr book tools for
acquisition, and over Jellyfin, Navidrome, Audiobookshelf and Komga/Kavita for libraries — and
presents all of it as **one library, one search box, one tag vocabulary, one request flow, one
credential.** Point Symfonium, KOReader or your Jellyfin TV app at UsArr instead of at five
different server URLs, and it routes each request to whichever backend can actually serve it.

Two things make it different from what already exists:

**It is a replica, not a proxy.** Every screen renders from a local SQLite database that UsArr
owns. No page load ever waits on an upstream API call. If every service in your stack is
offline, UsArr still browses, searches, sorts and filters your entire library at full speed.
This is a direct response to the way comparable tools fail: one puts a modern UI on top of a
1 req/s external API and takes ~50 minutes to scan a 10k-album library; another's dominant
complaint is that *"all the data has to load before anything shows."* UsArr's design refuses
that shape.

**It is cross-media.** Searching *Train Dreams* returns the 2025 film **and** the Denis Johnson
novella, joined by a real, evidence-carrying relationship — *"based on the novella by Denis
Johnson"* — sourced from Wikidata and shipped offline. Every 1,000+ star project in this space
is movies-and-TV-only.

---

## Feature status

Nothing below is built. Status reflects **planned milestone**, not progress.

### Core

| Feature | Status |
|---|---|
| Unified library across movies / TV / music / books / audiobooks / comics | 📋 Planned — v0.1 |
| Local-first reads; no upstream call on any render path | 📋 Planned — v0.1 |
| Sonarr, Radarr, Prowlarr integration | 📋 Planned — v0.1 |
| Instant search (client prefix index → FTS5 hybrid with typo tolerance) | 📋 Planned — v0.1 |
| Source tagging: usenet / torrent / irc / direct, first-class and filterable | 📋 Planned — v0.1 |
| **"1080p ✓ / 4K ✗"** — one poster across two Radarr instances | 📋 Planned — v0.1 |
| Image pipeline: proxy, downscale, ThumbHash placeholders | 📋 Planned — v0.1 |

### Gateway

| Feature | Status |
|---|---|
| **OpenSubsonic server** (`apiKeyAuthentication` only — never salt/token) | 📋 Planned — v0.2 |
| **OPDS 2.0 catalogue** (+1.2 fallback) | 📋 Planned — v0.2 |
| Navidrome, Jellyfin, Audiobookshelf, Komga aggregation | 📋 Planned — v0.2 |
| Stable opaque IDs so client playlists survive re-syncs | 📋 Planned — v0.2 |
| Favourites / ratings / scrobbles written back to the right backend | 📋 Planned — v0.2 |
| Jellyfin-compatible surface | 🤔 Unproven — v2 at earliest |

### Requests

| Feature | Status |
|---|---|
| One search box spanning owned **and** unowned | 📋 Planned — v0.3 |
| One Add action that routes to whichever backend can service it | 📋 Planned — v0.3 |
| Approval workflow, per-user quotas, per-media-type grants | 📋 Planned — v1.0 |

### Later

| Feature | Status |
|---|---|
| Cross-media linking via a prebuilt Wikidata CC0 subset + review inbox | 📋 Planned — v1.0 |
| Namespaced tags: aliases, virtual parents, rule engine, saved filters | 📋 Planned — v1.0 |
| Multi-user: roles, named permissions, library visibility, user import | 📋 Planned — v1.0 |
| Lidarr, Whisparr, Kavita, Calibre-Web, download-client visibility | 📋 Planned — v1.0 |
| Tailscale `tsnet` embedded listener + identity | 📋 Planned — v1.0 |
| Declarative YAML service manifests (add any service without code) | 📋 Planned — v0.3 → v1.0 |
| WASM plugins (Extism/wazero, sandboxed) | 📋 Planned — v1.x |
| OIDC / PKCE, passkeys, TOTP | 📋 Planned — v1.x |
| Optional Meilisearch search backend | 📋 Planned — v1.x |

### Explicit non-goals — not "later", **no**

| | |
|---|---|
| ❌ | **Video transcoding.** No FFmpeg command lines built from user input, ever. Jellyfin — mature and well-staffed — still ships High-severity argument-injection CVEs in exactly this surface. |
| ❌ | **An in-app media player.** UsArr routes you to your player; it does not become one. |
| ❌ | **Serving media bytes** as a core capability. Streams are a `302` to the backend. |
| ❌ | **Native TV or mobile apps.** ~40 Subsonic clients and Jellyfin's whole client matrix already exist. UsArr speaks their protocols instead. |
| ❌ | **Reimplementing the *Arr download/import engines.** |
| ❌ | **Any required sidecar** — no Postgres, no Redis, no search server. |

---

## Quickstart

> ⚠️ **Placeholder — this does not work yet.** No image is published. Shown so the intended
> shape is reviewable.

```yaml
# docker-compose.yml — illustrative only
services:
  usarr:
    image: ghcr.io/<org>/usarr:latest     # not published
    container_name: usarr
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
      - USARR_SECRET_KEY=${USARR_SECRET_KEY}   # generated on first run; BACK THIS UP
    volumes:
      - ./config:/config
    ports:
      - 8484:8484
    restart: unless-stopped
```

Then open `http://localhost:8484`, complete the setup wizard, and add your first service
(base URL + API key, with a mandatory connection test).

**Intended deployment** is a Tailscale tailnet — on which UsArr can run as its own tailnet
device via embedded `tsnet`, with no published port at all. Internet exposure is supported but
is the hardened secondary mode, not the default.

**Everything in `/config` is yours.** `usarr.db` is the data; `cache.db` and `cache/images/` are
disposable — deleting them costs a re-sync, not data. Restore is: stop, replace the file, start.

---

## Architecture in five bullets

1. **Replica, not proxy.** Every user-facing read is a local SQLite query. The *Arrs and media
   servers are replication sources and command sinks, never request-time dependencies. External
   metadata providers (TMDB, MusicBrainz, Wikidata, Open Library) are **ingest-time only**.
2. **Two-sided gateway.** Northbound protocol surfaces (OpenSubsonic, OPDS) that existing client
   apps already speak; southbound aggregation of your actual backends. One endpoint, one
   credential, and UsArr routes each item — backend credentials never reach a client.
3. **`work` / `edition` / `file`, with typed relationship edges.** The *Train Dreams* film and
   the *Train Dreams* novella are two distinct works joined by a `based_on` edge — not one work
   with two editions. Following Open Library and Wikidata, both of which model adaptation as a
   link between works. One work maps to rows in **many** service instances at once, which is
   where the "1080p ✓ / 4K ✗" badge comes from.
4. **Sync in four channels.** Full import → SignalR push → `/history/since` delta poll →
   periodic reconciliation. The delta poll is the correctness guarantee; SignalR is only an
   optimisation, because it frequently fails behind a reverse proxy. Writes go through an intent
   log with optimistic acknowledgement and real rollback.
5. **Go + SQLite + SvelteKit, one static binary.** `CGO_ENABLED=0`, `ncruces/go-sqlite3` (real
   upstream SQLite compiled to WASM under wazero), SQLite + WAL with a single-writer discipline,
   and a SvelteKit `adapter-static` SPA embedded via `embed.FS`. One container, one `/config`
   volume, multi-arch, no sidecars.

---

## Documentation

| Document | What it is |
|---|---|
| **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** | The authoritative design. System diagram, component breakdown, full SQL DDL for the core schema, the sync engine, search and requests, cross-media linking, the tag taxonomy, the plugin model, identity, the performance budget, the security model, deployment, and the roadmap. |
| **[docs/RESEARCH.md](docs/RESEARCH.md)** | The evidence base. Five research tracks condensed, with sources. API gotchas, rate limits, licensing constraints, the prior-art comparison, and — preserved deliberately — every ✅ verified / ⚠️ unverified distinction the researchers drew. **Read this before trusting any factual claim in the architecture.** |
| **[docs/DECISIONS.md](docs/DECISIONS.md)** | 21 ADRs. Context, decision, consequences, and **what was rejected and why** — including the good arguments for the roads not taken (Rust, Elixir, React, always-proxy). |
| **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)** | Every environment variable, default, and data-directory path. **Authoritative for configuration** — where it and the architecture doc differ on a name or a default, this one wins. |
| **[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)** | Intended toolchain, repository layout, make targets, testing strategy, and how to work on integrations without owning an *Arr stack. |
| **[docs/SETUP-CHECKLIST.md](docs/SETUP-CHECKLIST.md)** | What the project owner needs to obtain or decide before development proceeds, ranked by what actually blocks work. |

---

## Where this stands, honestly

**Nobody has built this.** The closest attempts each cover about 60% and stop: Seerr owns
discovery and requests but is movies+TV only; arr-dashboard owns operations — and already
proves the poll-and-cache architecture works — but has thin discovery and no gateway;
MediaManager and Mydia own single-app simplicity but are movies+TV; Reiverr had the
unified-with-playback vision and stalled in a rewrite; SeerrNG is the only cross-media request
app and it has 19 stars. The intersection UsArr targets is unoccupied.

That is the opportunity and also the warning. **The replica architecture is not a moat** —
arr-dashboard already does it. What has to be earned is the cross-media catalogue, the gateway
routing, and speed that is felt in the first 300 ms rather than read off a feature matrix.

A few things in the design are known-risky and are called out rather than buried:

- **The write path is the biggest schedule risk.** Everything else is mechanical; the intent
  log's failure and reconciliation semantics are where subtle bugs will live.
- **Cross-media coverage is thin in the long tail.** Wikidata has ~15k film adaptations —
  excellent for *Dune* and *Train Dreams*, poor for mid-list titles. The fuzzy scorer and the
  human review inbox are load-bearing, not decoration.
- **Delegating playback means users run a second service.** That is the real objection to the
  whole approach, and the mitigation is a good wizard, not a rebuttal.
- **Several upstream facts remain unverified** and are marked ⚠️ throughout the docs — TMDB's
  actual rate limit, `/history/since` behaviour parity across all five *Arrs, Tailscale's exact
  identity-header contract, and whether every Subsonic client tolerates our ID format.

---

## Contributing

The project is at the design stage, so **the most valuable contribution right now is a serious
critique of the documents** — particularly if you have run any of these services at scale and
can confirm or refute something marked ⚠️ in `docs/RESEARCH.md`.

Before opening an issue or a PR:

1. Read **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** and
   **[docs/DECISIONS.md](docs/DECISIONS.md)**. If you want to reopen a decision, the ADR
   probably already records the argument for your position and why it did not win — engage with
   that rather than restating it.
2. Factual claims need a source. The documents distinguish **verified** from **unverified**
   deliberately, and that distinction is worth protecting.
3. New service integrations should be a **YAML manifest** (`/config/providers/`) unless they
   genuinely need protocol work. That is what the manifest tier is for.

`CONTRIBUTING.md`, a code of conduct, a security policy and issue templates will land alongside
the first code.

---

## License

Not yet chosen. It will be an OSI-approved open-source license, decided before the first
release. *(Relevant context: the surveyed field is split roughly between MIT, AGPL-3.0 and
GPL-2.0.)*

---

## Prior art and acknowledgements

UsArr is designed on top of a lot of other people's work, and several projects were studied
closely enough that the debt should be explicit:

**Servarr** (Sonarr, Radarr, Lidarr, Prowlarr, Whisparr) for the ecosystem this exists to
serve · **arr-dashboard** for proving poll-and-cache works · **Seerr** for setting the
discovery and request-UX bar · **Homarr** for the integration-registry pattern and for
documenting the all-or-nothing-page-load anti-pattern so thoroughly · **DroppedNeedle** for the
protocol-emulation idea and for being candid in its own docs about where the time goes ·
**Streamarr** for the authenticated-internal-proxy escape hatch · **Mydia** for demonstrating
what "fast on weak hardware" actually looks like · **Navidrome** for being the existence proof
that Go + embedded SQLite idles at ~50 MB · **Jellyfin** for the client matrix UsArr borrows,
and for a security-advisory history that is genuinely instructive · **Wikidata**, **Open
Library** and **MusicBrainz** for CC0 data that makes cross-media linking possible at all.
