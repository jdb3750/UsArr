# UsArr

> **Status: pre-alpha. Nothing is implemented yet.**
> This repository currently contains a design and its evidence base — no code, no binary, no
> container. There is nothing to install. If you are here to run something, come back later.

**One unified, searchable catalogue over everything you own and everything you might want —
that plugs into the players you already use.**

---

## What it is

UsArr is a self-hosted **aggregation gateway** for the whole media-acquisition ecosystem. It sits
over Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian and the post-Readarr book tools for
acquisition, and over Jellyfin, Navidrome, Audiobookshelf and Komga/Kavita for libraries — and
presents all of it as **one library, one search box, one tag vocabulary, one request flow, one
credential.**

**It coexists with your stack; it does not replace it.** Sonarr keeps doing acquisition, Jellyfin
keeps doing playback, Navidrome keeps being an excellent music server. UsArr is the layer that makes
them one catalogue.

Two things make it different from what already exists:

**It is a replica, not a proxy.** Every screen renders from a local SQLite database that UsArr owns.
No page load ever waits on an upstream API call. Once synced, if every service in your stack is
offline, UsArr still browses, searches, sorts and filters your entire library at full speed. (Two
honest caveats: that holds *after* a first sync completes, and posters not yet in the local cache
show their placeholder until a service returns.) This is a direct response to how comparable tools
fail: one puts a modern UI on top of a 1 req/s external API and takes ~50 minutes to scan a 10k-album
library; another's dominant complaint is that *"all the data has to load before anything shows."*

**It is cross-media.** Searching *Train Dreams* returns the 2025 film **and** the Denis Johnson
novella, joined by a real, evidence-carrying relationship — *"based on the novella by Denis Johnson"*
— sourced from Wikidata and shipped offline. Every 1,000+ star project in this space is
movies-and-TV-only.

---

## Feature status

Nothing below is built. Status reflects **planned milestone**, not progress.

> **This table is generated from [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §16. If they
> disagree, §16 wins.** Ideas that are deliberately deferred are not listed here at all — they live
> in [`docs/FUTURE.md`](docs/FUTURE.md) with the reason, the cost, and the seam that keeps each one
> cheap to add later.

### v0.1 — it reads your library, it is fast, and you can act on it

| Feature | Status |
|---|---|
| Unified library across **movies and TV** (Sonarr + Radarr) | 📋 Planned — v0.1 |
| Local-first reads; no upstream call on any render path | 📋 Planned — v0.1 |
| **Search-and-Grab mode** — free-text indexer search and grab via Prowlarr, with no library | 📋 Planned — v0.1 |
| Instant search: client prefix index → FTS5 hybrid (prefix + substring; **no typo tolerance**) | 📋 Planned — v0.1 |
| Source tagging: **usenet / torrent**, first-class and filterable | 📋 Planned — v0.1 |
| Minimal write path — monitor, unmonitor, delete, add — on a durable command queue | 📋 Planned — v0.1 |
| Sync: full import + `/history/since` delta + **reconciliation with 7-day tombstones** | 📋 Planned — v0.1 |
| **Services health screen** — what is broken, why, and the button that fixes it | 📋 Planned — v0.1 |
| Image pipeline: proxy, downscale, ThumbHash, **viewport-prioritised cold start** | 📋 Planned — v0.1 |
| **"1080p ✓ / 4K ✗"** — one poster across two Radarr instances | 📋 Planned — v0.1 |
| Encrypted credentials with key versioning, AAD and a working rotate command | 📋 Planned — v0.1 |

### v0.2 – v1.0

| Feature | Status |
|---|---|
| Requests: one Add that routes, availability states, per-season TV, approval workflow | 📋 Planned — v0.2 |
| One search box spanning owned **and** unowned (out-of-band provider search, streamed) | 📋 Planned — v0.2 |
| **Cross-media linking** via a prebuilt Wikidata CC0 edge artifact — *Train Dreams* end to end | 📋 Planned — v0.3 |
| Declarative YAML service manifests (add a service without code) | 📋 Planned — v0.3 |
| **OpenSubsonic server** — 13-endpoint read-only subset, `apiKeyAuthentication` only, one Navidrome | 📋 Planned — v0.4 |
| Stable IDs so client playlists survive re-syncs | 📋 Planned — v0.4 |
| **OPDS 2.0 catalogue** (+1.2 fallback) | 📋 Planned — v1.0 |
| Multi-instance aggregation; favourites / ratings / scrobbles written back | 📋 Planned — v1.0 |
| Music, books, audiobooks and comics: Lidarr, Kavita, Calibre-Web, Audiobookshelf, Komga, Jellyfin | 📋 Planned — v1.0 |
| Source tagging: **irc / direct** (LazyLibrarian) | 📋 Planned — v1.0 |
| Namespaced tags: aliases, virtual parents, rule engine, saved filters | 📋 Planned — v1.0 |
| Multi-user: roles, named permissions, library visibility, user import | 📋 Planned — v1.0 |
| Tailscale `tsnet` embedded listener + identity | 📋 Planned — v1.0 |

### Deferred — good ideas, not now

WASM plugins · an optional external search engine · typo tolerance · OIDC / passkeys / TOTP /
forward-auth · the cross-media fuzzy ladder and review inbox · a Jellyfin-compatible surface ·
video byte-proxying · **release calendars across every subscription** · **per-user watch/listen/read
statistics**. Each is in **[`docs/FUTURE.md`](docs/FUTURE.md)** with why it was deferred, what it
would cost, and the specific seam in the v0.1 design that keeps it cheap to add — because the base
is being built with intentional space for them.

### Explicit non-goals — not "later", **no**

| | |
|---|---|
| ❌ | **Video transcoding, and any FFmpeg dependency at all.** No FFmpeg command lines built from user input, ever. Jellyfin — mature and well-staffed — still ships High-severity argument-injection CVEs in exactly this surface. |
| ❌ | **An in-app media player.** UsArr routes you to your player; it does not become one. |
| ❌ | **Reimplementing the \*Arr download/import engines.** |
| ❌ | **Any required sidecar** — no Postgres, no Redis, no search server. |
| ❌ | **Native TV or mobile apps.** ~40 Subsonic clients and Jellyfin's whole client matrix already exist. UsArr speaks their protocols instead. |

On bytes, precisely: **UsArr never carries video** — a video item links out to your Jellyfin's own
client. It *does* proxy audio and ebook bytes for its own OpenSubsonic and OPDS surfaces, because
those protocols have no way to hand a client off safely. That proxy is a plain copy with correct
HTTP `Range` handling and **no transcoding, ever**.

---

## Quickstart

> ⚠️ **Placeholder — this does not work yet.** No image is published. Shown so the intended shape is
> reviewable.

```yaml
# docker-compose.yml — illustrative only
services:
  usarr:
    image: ghcr.io/<org>/usarr:latest     # not published
    container_name: usarr
    environment:
      - TZ=Etc/UTC
      # USARR_SECRET_KEY is NOT set here. It is generated on first run into the config
      # volume at mode 0600. BACK IT UP — it decrypts your stored service credentials.
    volumes:
      - ./config:/config
    ports:
      - 8484:8484
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "/usarr", "healthcheck"]
      start_period: 60s
```

The container runs **non-root as UID 65532** on a distroless base, so `chown 65532:65532 ./config`
before the first start. Then open `http://localhost:8484`, complete the setup wizard, and add your
first service (base URL + API key, with a mandatory connection test).

**Intended deployment** is a Tailscale tailnet — on which UsArr can run as its own tailnet device via
embedded `tsnet`, with no published port at all. Internet exposure is supported but is the hardened
secondary mode, not the default.

**Everything in `/config` is yours.** `usarr.db` is the data; `cache.db` and the image cache are
disposable — deleting them costs a re-sync, not data. Restore is: stop, replace the file, start —
and the key file is a **separate** step, because a key stored in the same archive as the ciphertext
defeats the encryption. See [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md) §5 for the authoritative
layout.

---

## Architecture in five bullets

1. **Replica, not proxy.** Every user-facing read is a local SQLite query. The \*Arrs and media
   servers are replication sources and command sinks, never request-time dependencies. External
   metadata providers are ingest-time only — the single exception is unowned search, which runs
   out-of-band and streams in, never blocking a page.
2. **Two-sided gateway.** Northbound protocol surfaces (OpenSubsonic, OPDS) that existing client apps
   already speak; southbound aggregation of your actual backends. One endpoint, one credential —
   and **backend credentials never reach a client**, which is why the stream path proxies rather
   than redirects.
3. **`work` / `edition` / `file`, with typed relationship edges.** The *Train Dreams* film and the
   novella are two distinct works joined by a `based_on` edge — not one work with two editions —
   following Open Library and Wikidata. An **audiobook is an `edition` of a book work**, which is
   what makes ebook-vs-audiobook routing a schema property. One work maps to rows in **many** service
   instances at once, which is where the "1080p ✓ / 4K ✗" badge comes from.
4. **Sync in three channels, plus a write queue.** Full import → `/history/since` delta poll →
   periodic reconciliation with tombstones. The delta poll is the correctness guarantee;
   reconciliation is the safety net. Writes go through a **durable command queue** with honest
   pending and failed states — no optimistic apply, no silent reverts.
5. **Go + SQLite + SvelteKit, one static binary.** `CGO_ENABLED=0`, `ncruces/go-sqlite3` (upstream
   SQLite compiled to WebAssembly and translated to Go by `wasm2go` — no CGO, and as of 2026 no WASM
   runtime dependency either), SQLite + WAL with a single-writer discipline and a priority scheduler,
   and a SvelteKit `adapter-static` SPA embedded via `embed.FS`. One container, one `/config` volume,
   multi-arch, no sidecars.

---

## Documentation

| Document | What it is |
|---|---|
| **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** | The authoritative design: principles, components, the gateway, the data model, sync, search and requests, cross-media, tags, providers, security, deployment, **the roadmap (§16 is authoritative for scope)** and **the screens**. |
| **[docs/reference/](docs/reference/)** | The load-bearing detail the architecture links to: full DDL, sync mechanics, search query construction, the gateway endpoint map, cross-media, tags, providers, \*Arr API facts, and the security model. |
| **[docs/FUTURE.md](docs/FUTURE.md)** | Deferred features — what each is, why it waited, what it would cost, and **the seam in the current design that keeps it cheap**. |
| **[docs/DECISIONS.md](docs/DECISIONS.md)** | 24 ADRs. Context, decision, consequences, and **what was rejected and why** — including reversals, which record what falsified the original rather than quietly rewriting it. |
| **[docs/REVIEW-LOG.md](docs/REVIEW-LOG.md)** | Every finding from the adversarial review: applied, or rebutted with reasoning and evidence. |
| **[docs/RESEARCH.md](docs/RESEARCH.md)** | The evidence base. Five research tracks with sources, and every ✅ verified / ⚠️ unverified distinction preserved. **Read this before trusting any factual claim.** |
| **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)** | Every environment variable, default and data-directory path. **Authoritative for configuration.** |
| **[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)** | Toolchain, repository layout, make targets, testing, and how to work on integrations without owning an \*Arr stack. |
| **[docs/SETUP-CHECKLIST.md](docs/SETUP-CHECKLIST.md)** | What the owner needs to obtain or decide, ranked by what actually blocks work. |

---

## Where this stands, honestly

**Nobody has built this.** The closest attempts each cover about 60% and stop: Seerr owns discovery
and requests but is movies+TV only; arr-dashboard owns operations — and already proves the
poll-and-cache architecture works — but has thin discovery and no gateway; MediaManager and Mydia own
single-app simplicity but are movies+TV; Reiverr had the unified-with-playback vision and stalled in
a rewrite; SeerrNG is the only cross-media request app and it has 19 stars.

That is the opportunity and also the warning. **The replica architecture is not a moat** —
arr-dashboard already does it. What has to be earned is the cross-media catalogue, the gateway
routing, and speed that is felt in the first 300 ms rather than read off a feature matrix. That is
why the roadmap now puts **requests second and cross-media third**, before the gateway: the earliest
milestone that is not already available elsewhere is the one that must ship soonest.

Known-risky things, called out rather than buried:

- **Cross-media coverage is thin in the long tail.** Wikidata has ~15k film adaptations — excellent
  for *Dune* and *Train Dreams*, poor for mid-list titles. UsArr's answer is to **claim nothing it
  cannot evidence**: no fuzzy guessing, no review queue, and a missing link rather than a wrong one.
- **Search has no typo tolerance.** FTS5's trigram tokenizer does substring matching, not fuzzy
  matching. Tier 2 gives prefix and substring search, and the UI says so.
- **Delegating playback means users run a second service.** That is the real objection to the whole
  approach, and the mitigation is a good wizard, not a rebuttal.
- **The idle-memory budget is unmeasured.** It rested on a citation to Navidrome, which uses a
  *cgo* SQLite driver that UsArr does not. A day-one spike on arm64 sets the real number before the
  schema is written.
- **Several upstream facts remain unverified** and are marked ⚠️ throughout — TMDB's and AniList's
  actual rate limits, `/history/since` behaviour parity, Tailscale's exact identity-header contract,
  and whether every Subsonic client tolerates our ID format.

---

## Contributing

The project is at the design stage, so **the most valuable contribution right now is a serious
critique of the documents** — particularly if you have run any of these services at scale and can
confirm or refute something marked ⚠️.

1. Read **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)**. It is written to be read start to finish;
   the reference files are there when you need the detail, not before.
2. If you want to reopen a decision, **[docs/DECISIONS.md](docs/DECISIONS.md)** probably already
   records the argument for your position and why it did not win — engage with that rather than
   restating it. If you think something was deferred wrongly, **[docs/FUTURE.md](docs/FUTURE.md)**
   names the trigger that would reopen it.
3. Factual claims need a source. The documents distinguish **verified** from **unverified**
   deliberately, and that distinction is worth protecting.
4. New service integrations should be a **YAML manifest** unless they genuinely need protocol work.
   Note what a manifest is: a server-side request generator that runs with your instance's stored
   credential — **not** a sandbox.

**One process rule, adopted deliberately: no new design document until there is a running binary.**
Further design goes in the code and in ADRs. `CONTRIBUTING.md`, a code of conduct, a security policy
and issue templates land alongside the first code.

---

## License

**Recommendation: AGPL-3.0** — pending the owner's confirmation, which is needed **before the first
PR is accepted**, not before the first release. Accepting contributions into a repository with no
`LICENSE` leaves their terms undefined, and retro-licensing means chasing every contributor.

AGPL-3.0 is the default answer given the project's positioning against hosted forks and the
self-hosted-media norm; MIT is the alternative if embedding matters more. Either is fine. Deferring
is not.

---

## Prior art and acknowledgements

**Servarr** (Sonarr, Radarr, Lidarr, Prowlarr, Whisparr) for the ecosystem this exists to serve ·
**arr-dashboard** for proving poll-and-cache works · **Seerr** for setting the discovery and
request-UX bar · **Homarr** for the integration-registry pattern and for documenting the
all-or-nothing-page-load anti-pattern so thoroughly · **DroppedNeedle** for the protocol-emulation
idea and for being candid about where the time goes · **Streamarr** for the authenticated-internal-
proxy escape hatch · **Mydia** for demonstrating what "fast on weak hardware" actually looks like ·
**Navidrome** for proving that a Go media server with embedded SQLite can idle small, and for being
the reference point for a UI that is bare-bones and exactly right · **Jellyfin** for the client
matrix UsArr borrows, and for a security-advisory history that is genuinely instructive ·
**Wikidata**, **Open Library** and **MusicBrainz** for CC0 data that makes cross-media linking
possible at all.
