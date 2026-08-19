# UsArr

> **Status: pre-alpha. Parts of it run; the rest is still design and its evidence base.**
> What runs today is Prowlarr Search-and-Grab: free-text indexer search, results streaming in per
> indexer, and grab — served by a Go binary with an embedded SPA shell. There is no release and no
> container image yet, so running it means building it (`make build`). A catalogue import exists as
> of `c56c8e4` — for Kavita and for nothing else, and only the first-import channel: it fires once
> when you connect a Kavita, or on demand, and there is no periodic re-sync behind it. Which
> channels the sync core has is a question for `internal/libsync` and `cmd/usarr/import.go`, whose
> package docs answer it channel by channel. The rest moves week to week, so what exists is
> whatever is in the tree (`web/src/routes`, `internal/`) rather than whatever this paragraph last
> claimed. If you are here to install something finished, come back later.

**One unified, searchable catalogue over everything you own and everything you might want —
that plugs into the players you already use.**

---

## What it is

UsArr is a self-hosted **aggregation gateway** for the whole media-acquisition ecosystem, and it
arrives in stages rather than all at once. **v0.1 aggregates BookOrbit and Prowlarr** — one catalogue
source in front of the sync core, plus free-text search-and-grab. **The remaining library servers
then arrive one at a time after it**, in the order §16.1 sets: Navidrome, then Audiobookshelf, then
Komga. **Sonarr and Radarr are re-sequenced out of v0.1 rather than cut**, and land on a sync core
already proven on real data. Lidarr, LazyLibrarian, Mylar3, Kapowarr, Jellyfin and the post-Readarr
book tools come in later milestones — [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §16 is
authoritative for which, and for where the \*Arrs land. All of it is presented as **one library, one
search box, one tag vocabulary, one request flow, one credential.**

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

Status reflects **planned milestone**, not progress, except where a row says otherwise: `📋 Planned
— v0.1` names the milestone that owns a feature, never that the feature exists. Most of this table
is not built yet, and which parts are changes faster than the table does — for that, read the tree
(`web/src/routes` for screens, `internal/` for backend surfaces), not this page.

> **This table is maintained by hand against [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §16 —
> nothing in the repo generates it, so it lags §16 and has. If they disagree, §16 wins.** Ideas that
> are deliberately deferred are not listed here at all — they live in
> [`docs/FUTURE.md`](docs/FUTURE.md) with the reason, the cost, and the seam that keeps each one
> cheap to add later.

### v0.1 — it reads your library, it is fast, and you can act on it

| Feature | Status |
|---|---|
| **Six-media-type foundation** — the schema covers movies, TV, music, ebooks, audiobooks and comics, and requesting covers all six; **the v0.1 catalogue itself is books and comics/manga**, because BookOrbit is the one catalogue source that ships as of ADR-0052 | 📋 Planned — v0.1 |
| **The sync core, with BookOrbit as its first adapter** (ADR-0052, which replaced Kavita in that slot on the owner's decision to sunset it) — full import, the ordered page walk where the source supports one, and reconciliation. It is what proves the local-first thesis on a real library, and BookOrbit carries it because it is the source the owner actually runs | 📋 Planned — v0.1 |
| **\*Arr library sync — Sonarr and Radarr** — **re-sequenced out of v0.1, not cut** (ADR-0041): the owner runs neither, so *"prove it on a real Sonarr and a real Radarr"* was a criterion no v0.1 could meet. They arrive onto a core already proven | 📋 Planned — §16 has not yet named the milestone |
| Read-only catalogue sources: **Navidrome, then Audiobookshelf, then Komga** — one at a time after v0.1, each its own milestone. ✅ The delta-watermark probe that set the order **ran 2026-08-17 and passed** — that is what moved Kavita into v0.1 ahead of them, a placement ADR-0052 has since ended, and Kavita is **not added back** to this sequence (ARCHITECTURE §16.1) | 📋 Planned — after v0.1 |
| **User-defined libraries**, configured separately from services, with a correction layer | 📋 Planned — v0.1 (correction UI v0.3) |
| **Six-type schema** — `work_credit` M:N attribution, edition-scoped `work_track`, the `comic_issue` kind, **the `person` kind**, audiobook edition columns | 📋 Planned — v0.1 (migration 0001 **or a backfill** — ADR-0030, ADR-0031, ADR-0033; the six subtype *tables* each land with the source that writes them, ADR-0040 — and ADR-0044 applied that rule to `work_credit`, which lands with v0.1's catalogue source — BookOrbit as of ADR-0052, which **confirms** ADR-0044 rather than amending it, because that ADR turns on a property of the source and not on its name) |
| Local-first reads; no upstream call on any render path | 📋 Planned — v0.1 |
| **Search-and-Grab mode** — free-text indexer search and grab via Prowlarr, **for all six types**, with no library. ⚠️ A fact about the indexer ecosystem, not about UsArr's design: 403 of the 543 indexer definitions Prowlarr ships are `type: private`, and the dedicated music and book trackers are invite-only, so on public indexers alone the results for music and books are materially thinner than for film. **A library's catalogue source and its request destination are separate bindings** (§8.3), and **Lidarr is deferred because no write-capable music service ships in v0.1** — not because music ranks lower | ✅ Shipped — v0.1 |
| Instant search: client prefix index → FTS5 hybrid (prefix + substring; **no typo tolerance**) | 📋 Planned — v0.1 |
| Source tagging: **usenet / torrent**, first-class and filterable | 🚧 Partial — v0.1: derived and served on search results; not yet filterable |
| Minimal write path — monitor, unmonitor, delete, add — on a durable command queue. ⚠️ Its \*Arr targets left v0.1 with ADR-0041, so whether it re-sequences with them or stays for Prowlarr's grab path alone is **an open question §16 flags rather than answers** | 📋 Planned — v0.1, scope open |
| Sync channels **1 and 4**, with **3b reduced to the books half at best** (ARCHITECTURE §16.1, applying ADR-0052): full import + the **ordered page-walk delta (channel 3b)** expressible for `work_book` only, because BookOrbit exposes no series-level ordered read — so comics and manga fall back to **reconciliation with 7-day tombstones**, which carries more weight here because a page walk cannot observe a deletion. `/history/since` (channel 3) does not apply to BookOrbit and lands with the first \*Arr adapter. ⚠️ This row read *"channels 1, 3b and 4 … built for Kavita"*; ADR-0041 clause 4 earned that list from a live **Kavita** probe, and ADR-0052 **reopens** it rather than re-answering it | 📋 Planned — v0.1 |
| **Services health screen** — what is broken, why, and the button that fixes it | 📋 Planned — v0.1 |
| **Recent grabs** — the last ten grabs with their state, so the acquisition loop has a memory. Not the request model, which is v0.2 | 📋 Planned — v0.1 |
| Image pipeline: proxy, downscale, ThumbHash, **viewport-prioritised cold start** | 📋 Planned — v0.1 |
| **"1080p ✓ / 4K ✗"** — one poster across two Radarr instances. A free consequence of the M:N link, which v0.1 still ships; ⚠️ **unexercised in v0.1**, because it needs two Radarrs and Radarr re-sequenced out | 📋 Planned — v0.1 (the link); demonstrated when the \*Arr adapters arrive |
| Encrypted credentials with key versioning, AAD and a working rotate command | 📋 Planned — v0.1 |

### v0.2 – v1.0

| Feature | Status |
|---|---|
| Requests: one Add that routes, availability states, per-season TV, approval workflow | 📋 Planned — v0.2 |
| **The catalogue sources, one milestone each** — **Navidrome (#1), then Audiobookshelf (#2), then Komga (#3)**. Kavita left this sequence for v0.1 and then left v0.1 too when ADR-0052 sunset it; it is **not** added back as a fourth entry, on ADR-0042's refusal-to-invent-a-milestone precedent. Each lights up its media types in the grid and search | 📋 Planned — after v0.1, ahead of v0.4 for Navidrome |
| One search box spanning owned **and** unowned (out-of-band provider search, streamed) | 📋 Planned — v0.2 |
| **Cross-media linking** via a prebuilt Wikidata CC0 edge artifact — *Train Dreams* end to end | 📋 Planned — v0.3 |
| Declarative YAML service manifests (add a service without code) | 📋 Planned — v0.3 |
| **OpenSubsonic server** — ~20-method read-only subset, `apiKeyAuthentication` only, one Navidrome | 📋 Planned — v0.4 |
| Stable IDs so client playlists survive re-syncs | 📋 Planned — v0.4 |
| **OPDS catalogue — 1.2 first**, 2.0 second (the long tail of readers is entirely 1.2; **KOReader 2026.07 shipped OPDS 2.0 basic support**, so 1.2-first is about the long tail and not about KOReader) | 📋 Planned — v1.0 |
| Multi-instance aggregation; favourites / ratings / scrobbles written back | 📋 Planned — v1.0 |
| **Request sinks**: Lidarr, LazyLibrarian, Mylar3, Kapowarr — plus Jellyfin and a Calibre `metadata.db` adapter | 📋 Planned — v1.0 |
| Source tagging: **irc / direct** (LazyLibrarian) | 📋 Planned — v1.0 |
| Namespaced tags: aliases, virtual parents, rule engine, saved filters | 📋 Planned — v1.0 |
| Multi-user: roles, named permissions, library visibility, user import | 📋 Planned — v1.0 |
| Tailscale `tsnet` embedded listener + identity | 📋 Planned — v1.0 |

### Deferred — good ideas, not now

WASM plugins · an optional external search engine · typo tolerance · OIDC / passkeys / TOTP /
forward-auth · the cross-media fuzzy ladder and review inbox · a Jellyfin-compatible surface ·
video byte-proxying · **release calendars across every subscription** · **per-user watch/listen/read
statistics** · slskd as a music sink · Suwayomi · OPDS 2.0 · Chaptarr · MangaBaka · **the
ebook↔audiobook identity pass** · per-library OPDS feeds. Each is in **[`docs/FUTURE.md`](docs/FUTURE.md)** with why it was deferred, what it
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

> ⚠️ **There is no proven container path yet.** `deploy/Dockerfile` is now in the tree, but no image
> is published and it has not been built — the environment that added it has no Docker daemon, so
> `make docker` was never run against it. The Compose file below is shown so the intended shape is
> reviewable; treat the container path as unverified until a `make docker` on a host with a daemon
> proves it.
>
> **The path that works today is building from source**: `make build` produces a single static binary
> with the SPA embedded, which you install onto `PATH` and run under systemd.
> [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) §12 has the sequence, the toolchain the build host
> needs, the three build traps, and the backup to take first.

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
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8484/api/health/live || exit 1"]
      interval: 30s
      timeout: 5s
      start_period: 60s
```

⚠️ **On the healthcheck, precisely.** Liveness is an HTTP `GET` on **`/api/health/live`** — that and
`/api/health/ready` are the endpoints that exist. There is **no `usarr healthcheck` subcommand**:
the binary takes flags only and exits 1 on any positional argument, so a `["CMD", "/usarr",
"healthcheck"]` probe would fail every time and report the container permanently unhealthy. The
`CMD-SHELL` form above needs a shell and `wget`, which a distroless *static* base does not carry —
whichever way that is settled when the image is actually built (a `busybox` layer, a second static
probe binary, or a subcommand added to `usarr`), the check itself is the HTTP GET.

The container runs **non-root as UID 65532** on a distroless base, so `chown 65532:65532 ./config`
before the first start. Then open `http://localhost:8484`, complete the setup wizard, and add your
first service (base URL + API key, with a mandatory connection test).

**Intended deployment** is a Tailscale tailnet. ⚠️ **Nothing Tailscale-specific is implemented** —
there is no embedded `tsnet` node, no `Tailscale-User-*` header handling and no `WhoIs` lookup; the
only trace of it in the code today is `tailscale` being an accepted value for a user's
`auth_source`. Until the `tsnet` listener lands (planned v1.0, see the table above and
[`docs/CONFIGURATION.md`](docs/CONFIGURATION.md) §9), UsArr is an ordinary HTTP server: put it on the
tailnet by binding it to a host that is already on one, or front it with a reverse proxy. Running it
as its own tailnet device with no published port at all is the destination, not the current state.
Internet exposure is supported but is the hardened secondary mode, not the default.

**Everything in `/config` is yours.** `usarr.db` is the data, and `kek.salt` beside it is a
non-secret the database cannot be opened without — back up both, and keep `keys/` out of that same
archive. The caches (`cache.db`, the image cache) are disposable and land under `/data`; neither is
created yet, and deleting them will cost a re-sync, not data. Restore is: stop, replace the file, start —
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
| **[docs/reference/](docs/reference/)** | The load-bearing detail the architecture links to: full DDL, sync mechanics, search query construction, the gateway endpoint map, cross-media, tags, providers, \*Arr API facts, and the security model. **[`http-api.md`](docs/reference/http-api.md) is UsArr's own northbound wire contract** — what a client may rely on, per endpoint. |
| **[docs/design/](docs/design/)** | The visual system: `DESIGN-DIRECTION.md`, `tokens.css` (the canonical values) and the v0.1 screen mockups. `ARCHITECTURE.md` §17 stays authoritative over all three. |
| **[docs/FUTURE.md](docs/FUTURE.md)** | Deferred features — what each is, why it waited, what it would cost, and **the seam in the current design that keeps it cheap**. |
| **[docs/DECISIONS.md](docs/DECISIONS.md)** | The ADRs. Context, decision, consequences, and **what was rejected and why** — including reversals, which record what falsified the original rather than quietly rewriting it. |
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

UsArr is licensed under the **GNU Affero General Public License, version 3.0** — confirmed by the
owner on 2026-08-16. The full text is in [`LICENSE`](./LICENSE).

AGPL-3.0 matches the self-hosted-media norm (Jellyfin, Navidrome, MediaManager, Mydia,
DroppedNeedle) and closes the hosted-service loophole: anyone who runs a modified UsArr as a network
service owes its users that modified source. Self-hosting for yourself carries no obligation.
The reasoning, and the rejected MIT alternative, are recorded in
[ADR-0024](./docs/DECISIONS.md#adr-0024).

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
