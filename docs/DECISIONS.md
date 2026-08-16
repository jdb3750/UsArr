# UsArr — Decision Log (ADRs)

Lightweight architecture decision records. One entry per significant, hard-to-reverse choice.

**Status values:** `Accepted` · `Superseded by ADR-NNNN` · `Proposed` · `Deprecated`.
Evidence for every claim below lives in [`RESEARCH.md`](./RESEARCH.md); the design that follows
from it lives in [`ARCHITECTURE.md`](./ARCHITECTURE.md).

**All ADRs dated 2026-08-16 unless noted. Nothing here is implemented yet** — these are
decisions taken before the first line of code, on the basis of five research tracks.

**Revision 2 (2026-08-16, after a three-way adversarial review).** Seven ADRs changed. Three
distinctions now matter and are used consistently below:

- **Accepted** — the decision stands.
- **Deferred** — the idea is sound and is *not* being built now. Every deferred ADR names **the
  seam** in the v0.1 design that keeps it cheap to add later, and **a revisit trigger**. The full
  set lives in [`FUTURE.md`](./FUTURE.md). A deferral is not a rejection, and it must not be
  restated as one.
- **Superseded / Reversed** — the decision was wrong, or its evidence was. The reversal records what
  changed and why, rather than silently rewriting the original.

| ADR | Decision | Status |
|---|---|---|
| [0001](#adr-0001) | Go for the backend | **Accepted, evidence corrected** (rev 2) |
| [0002](#adr-0002) | Embedded SQLite + WAL, no required sidecar | Accepted, one claim narrowed |
| [0003](#adr-0003) | SvelteKit `adapter-static` SPA embedded via `embed.FS` | Accepted |
| [0004](#adr-0004) | Replica, not proxy | Accepted |
| [0005](#adr-0005) | Delegate playback to media servers | Accepted |
| [0006](#adr-0006) | Never build a video transcoder | Accepted · permanent |
| [0007](#adr-0007) | Wikidata as the cross-media spine, shipped as a prebuilt CC0 subset | **Amended** (rev 2): edges-only artifact, per-release |
| [0008](#adr-0008) | **Two** plugin tiers now; a WASM tier deferred | **Amended** (rev 2) |
| [0009](#adr-0009) | `work` / `edition` / `file` with typed `work_relation` edges | Accepted, `audiobook` resolved |
| [0010](#adr-0010) | OpenSubsonic and OPDS as northbound server surfaces | Accepted, scope narrowed |
| [0011](#adr-0011) | Named permission strings, not a bitfield | Accepted |
| [0012](#adr-0012) | Sync channels and the write path | **Superseded in part by ADR-0012a** |
| [0012a](#adr-0012a) | A durable command queue replaces the optimistic intent log | **Accepted** (rev 2) |
| [0013](#adr-0013) | **Two-tier** search; an external engine deferred | **Amended** (rev 2) |
| [0014](#adr-0014) | `service_item_link` is many-to-many | Accepted, framing demoted |
| [0015](#adr-0015) | Namespaced tags with virtual parents | Accepted |
| [0016](#adr-0016) | Tailnet as the default deployment assumption | Accepted |
| [0017](#adr-0017) | ~~302 redirect for streams~~ → **proxy for audio/ebooks, link out for video** | **Reversed** (rev 2) |
| [0018](#adr-0018) | No in-app player; UsArr is not a byte-delivery product | Accepted, one clause corrected |
| [0019](#adr-0019) | Single-user first, multi-user schema from migration 0001 | Accepted, list reconciled |
| [0020](#adr-0020) | Requests are a first-class pillar | Accepted |
| [0021](#adr-0021) | Stable IDs address `service_item_link`, not `work` | **Amended** (rev 2): kind byte, pin, no opacity claim |
| [0022](#adr-0022) | v1 authentication is local-only; external identity deferred | **Accepted** (rev 2) |
| [0023](#adr-0023) | UsArr coexists with the ecosystem rather than replacing it | **Accepted** (rev 2) |
| [0024](#adr-0024) | Styling and typography: Tailwind v4 with the default theme deleted, Bits UI, Tabler, self-hosted IBM Plex | **Accepted** |

---

<a id="adr-0001"></a>
## ADR-0001 — Go for the backend

**Status:** Accepted

### Context
The workload is ~50k–500k rows, 1–5 users, fan-out I/O against N services, on a hardware floor
of a Raspberry Pi 4/5, an N100, or a Synology NAS with 1–2 GB of RAM available. ARM64 and AMD64
are both mandatory. This is **not** a high-QPS service; what matters is startup time, idle
memory, tail latency under a large import, distribution simplicity, and — for an OSS project
that must survive 24 months — contributor accessibility.

The one genuinely CPU-shaped task is JSON: a 10k-movie `/api/v3/movie` response is 30–80 MB.

### Decision
**Go.** `CGO_ENABLED=0`, static binary, cross-compiled by `GOOS`/`GOARCH` with no toolchain.
SQLite via `ncruces/go-sqlite3`. ~~WASM plugin host via `wazero`.~~ — struck in revision 2: the
WASM tier is deferred (ADR-0008) and the driver no longer depends on wazero (correction 1 below).

### Consequences
- `docker buildx` multi-arch images become trivially reproducible; target < 40 MB compressed.
- Goroutines + `errgroup` + a bounded semaphore is ~30 lines for the fan-out poller, with no
  function colouring.
- **`ncruces/go-sqlite3` gives cgo-free SQLite with FTS5 and the trigram tokenizer.**
- GC spikes during a large import are real (~150–300 MB peak vs Rust's ~60–120 MB flat). Budget
  for it; streaming ingest (ADR-0012) is what keeps it bounded.
- A modest cgo-free performance penalty on SQLite (~75% of `mattn/go-sqlite3` in one benchmark
  set). Irrelevant at this scale.

### Correction, revision 2 — two claims in this ADR were wrong, and one of them was load-bearing

1. **`ncruces/go-sqlite3` no longer runs on wazero.** The library migrated to the maintainer's own
   **`wasm2go`** translator: its README now states it *"wraps a Wasm build of SQLite, and uses
   wasm2go to translate it to Go"*, and that *"Go and `x/sys` are the only direct dependencies"* —
   i.e. **wazero is not a dependency at all.** Announced in discussion #361 (2026-03-05), PR #362
   ready 2026-03-09. The original text's clinching argument — *"the same wazero runtime then serves
   the Tier 2 plugin host: one runtime, two uses"* — is therefore **false**, and ADR-0008 has been
   re-argued without it. `DEVELOPMENT.md` already carried the correct position while four other
   documents asserted the opposite.
2. **"Bit-for-bit upstream behaviour" is dropped.** Under `wasm2go` the C source is compiled to Wasm
   and then *translated to Go*, which is a different claim from "runs the upstream C source" and
   would need its own evidence. What can be said without evidence is narrower and is what the text
   now says: cgo-free SQLite with FTS5 and the trigram tokenizer.

**A memory claim in this ADR is also unsupported and is now marked so.** The README's acknowledgement
of *"Navidrome, the existence proof that Go + embedded SQLite idles at ~50 MB"* does not transfer:
Navidrome uses a **cgo** driver. The `< 80 MB` idle-RSS budget therefore rests on nothing measured.
**Required before schema work starts:** a one-day spike — this driver, a 500k-row fixture, WAL, the
architecture's pragmas, idle and peak RSS measured **on arm64**. Record the number here as a
consequence and set the budget from it. If it lands materially above 80 MB, **the pragma defaults are
the first thing to tune, not the driver** — noted explicitly so a future reader does not reopen this
ADR by mistake.

### Alternatives rejected
- **Rust + Axum** — the honest runner-up. 2–3× lower memory, no GC pauses during a 500k-row
  import, and **Tantivy**, a materially better search story than anything in Go. Rejected on
  contributor accessibility (async Rust — `Pin`, `Send + Sync` bounds on trait objects, lifetime
  puzzles in a plugin registry — is a genuine filter on the homelabber who wants to add
  LazyLibrarian support) and cross-compilation friction the moment a dependency pulls in C.
  **Revisit if UsArr ever needs heavy in-process work** (local embeddings, image processing at
  scale).
- **Node/TypeScript** — tempting for type-sharing with the frontend. Killed by JSON: decoding a
  60 MB payload on the single thread stalls the event loop for hundreds of milliseconds, and the
  fix (worker threads) reintroduces the complexity you chose Node to avoid. Plus no single
  binary and a 100+ MB baseline. **arr-dashboard's documented heap-pressure history is this
  problem in the wild.**
- **Python** — the GIL serialises the parse-heavy import path; the worst packaging story of any
  option.
- **.NET 9** — genuinely tempting because the *Arrs are .NET, so their DTO contracts and SignalR
  client could be reused directly. Rejected because it inherits the exact runtime weight profile
  self-hosters complain about, and the homelab OSS contributor pool for C# is thin.
- **Elixir/Phoenix** — **the best concurrency model on the list**, and a beautiful fit for
  supervising N flaky upstream connections (circuit breakers are nearly free with OTP). Track 02
  independently identified Mydia (Elixir/LiveView) as the empirical *speed exemplar*, with a
  third-party review reporting excellent performance on an underpowered NAS, and argued that
  preemptive per-process scheduling structurally prevents the Overseerr#2030 job-blocking
  failure. **Rejected on contributor-pool size and deployment weight.** Go's preemptive scheduler
  addresses the same failure mode. **This is a real loss and is recorded as such** — it is the
  "if we had ten Elixir developers" answer.

---

<a id="adr-0002"></a>
## ADR-0002 — Embedded SQLite + WAL, no required sidecar

**Status:** Accepted

### Context
Self-hosters overwhelmingly want one container, one volume, no sidecar. Every additional
*required* service costs a meaningful fraction of the potential user base. Separately: is SQLite
fast enough? Seerr users report **82k-item libraries with ~1.5 s cold load and zero tuning** —
so yes, comfortably.

### Decision
**SQLite in-process, WAL mode, and it is not negotiable as a requirement.** Two connection
pools: a read pool of `NumCPU*2` and a **write pool of exactly one connection.** Every write
transaction uses `BEGIN IMMEDIATE`. A second, disposable `cache.db` holds high-churn data.

### Consequences
- Zero network hops on the query path — SQLite is *faster* here, not a compromise.
- **`SQLITE_BUSY` arising from concurrent writers *inside the process* is eliminated**, because all
  writes funnel through one connection. `busy_timeout` does **not** rescue a deferred read
  transaction that upgrades to a write — that path returns `SQLITE_BUSY` immediately, which is the
  classic production-only bug.
  **Revision 2 narrows this claim**, which previously said `SQLITE_BUSY` from internal contention was
  *"structurally impossible"* — overstated, and the kind of sentence that gets believed. Residual
  sources remain, all present in this design: `VACUUM INTO` and `ANALYZE` take their own locks; WAL
  checkpointing can be starved by a long-lived reader, after which `wal_autocheckpoint` silently
  fails and the WAL grows unbounded; a second process (`usarr key rotate`, a user running `sqlite3`,
  two containers on one volume); and `cache.db` if it is ever `ATTACH`ed inside a write transaction.
  Mitigations are specified in `reference/sync.md` §6.
- **The single writer is shared between the bulk importer and the interactive write path**, so it
  needs a scheduler, not just a pool size: import batches commit at `min(2000 rows, 100 ms)` and
  interactive commands preempt at batch boundaries. Without that, a 5,000-row import batch (200 ms–2 s
  on a Pi) puts every user write behind it, which is the exact failure mode ADR-0004 exists to avoid.
- Bulk writes must be batched (1000–5000 rows per transaction). This is the difference between a
  40-second import and a 40-minute one.
- `STRICT` tables require SQLite ≥ 3.37. `ncruces/go-sqlite3` bundles a recent build, so this is
  safe — **but note it if the driver ever changes.**
- ⚠️ **Write amplification on ZFS and other CoW filesystems is real** and bit Seerr users; the
  documented workaround is moving the DB to ext4. WAL + batching mitigates but does not
  eliminate it. **Document the NAS case.**
- Backups are `VACUUM INTO` — transactionally consistent, no locking, compacted output.

### Alternatives rejected
- **Postgres as a requirement** — a product failure for this audience. May exist as an optional
  backend; may never be required.
- **DuckDB** — an OLAP engine for an OLTP problem. Weak concurrent writes.
- **LibSQL/Turso** — their concurrent-writes work explicitly targets SQLite's single-writer
  limitation, which is interesting but solves a problem UsArr does not have: the single-writer
  discipline is a *design choice*, not a constraint being suffered. Adds a fork dependency for
  no benefit.

---

<a id="adr-0003"></a>
## ADR-0003 — SvelteKit `adapter-static` SPA, embedded via `embed.FS`

**Status:** Accepted

### Context
Access is LAN or reverse-proxy, desktop and mobile browsers, a handful of users, long sessions
involving heavy browsing and filtering of a 10k+ item grid.

### Decision
**SvelteKit configured as a pure client-side SPA**, built to `web/dist`, embedded in the Go
binary via `embed.FS`. **No Node process in production, ever.**

### Consequences
- The single-binary story survives. Any SSR framework would destroy it.
- Smallest baseline bundle of the realistic options; Svelte compiles to vanilla JS with
  near-zero runtime, and bundle size is the honest lever on perceived speed over phone Wi-Fi.
- Navigation between library → detail → search is **zero-network** after first load.
- Svelte 5 runes make the bespoke optimistic-write store (ADR-0012) pleasant without a mutation
  library.
- The team owns cache-coherency on the client rather than buying it.

### Alternatives rejected
- **SSR (Next.js/Nuxt/SvelteKit-with-a-server)** — the classic SSR argument is SEO plus
  first-paint on a cold, distant, high-latency connection. **None of that applies**: the app is
  private, the server is 1 ms away, and the session is long-lived so the shell amortises
  instantly. SSR is dead weight *and* it breaks the single binary.
- **React + Vite + TanStack Router/Query** — **the defensible runner-up, and this ADR would not
  be reopened in anger if a team chose it.** The strongest argument is **TanStack Query**, which
  solves — with years of edge-case hardening — precisely UsArr's problems:
  stale-while-revalidate, optimistic mutations with automatic rollback, request deduplication,
  background refetch. If the team judges client-side cache coherency to be the risky part,
  buying that maturity is worth the extra ~80 KB. Rejected on bundle size and because UsArr's
  optimistic writes are bespoke enough (a pending-intent overlay keyed by ULID) that a generic
  cache would be fought as often as used.
- **SolidStart** — best-in-class runtime perf, but a **34× smaller ecosystem** than SvelteKit
  (59k vs 2.02M weekly downloads) and most React component libraries are incompatible.
  Disqualifying for a project that needs drive-by contributors.
- **HTMX + Go templates** — genuinely tempting; it would make the Go story purer. **Rejected on
  requirements, not taste:** a 10k-item virtualized poster grid with instant client-side
  filter/sort *is* a rich client-state problem, and every HTMX interaction is a network round
  trip — the exact thing this project exists to eliminate.

---

<a id="adr-0004"></a>
## ADR-0004 — Replica, not proxy

**Status:** Accepted · **This is the load-bearing decision. Everything else is downstream.**

### Context
The stated complaint — *"it wants to feel modern but all the API calls are just not great user
experience"* — has a documented mechanism in every comparable project:

- **DroppedNeedle** puts a modern SvelteKit UI directly on MusicBrainz's hard **1 req/s** limit;
  its own docs state a **10k-album library takes ~50 minutes** to scan.
- **Homarr's** dominant complaint is that *"all the data has to load before anything shows"* —
  one slow integration blocks the entire dashboard.
- **Overseerr#2030** (open 2021 → archive, never fixed): background jobs slow the site *despite
  low CPU*.
- **Prowlarr#712**: a fan-out search waits for the deadest indexer, 30 s+.

### Decision
**Every user-facing read renders from local SQLite. The *Arrs and media servers are upstream
replication sources and command sinks, never request-time dependencies.** External metadata
providers are ingest-time only.

Enforced by package boundary: **the `api` package imports `store`, never `provider`.** A
violation is architectural, not stylistic.

### Consequences
- If every upstream service is offline, UsArr still browses, searches, sorts and filters the
  whole library at full speed. Only *new actions* degrade — into a queue with an honest label.
- **UsArr now owns a cache-coherency problem.** That is the price, and ADR-0012 is the payment.
- A conflict rule must be stated unambiguously: **the *Arr owns the truth; UsArr owns the
  cache.** When reconciliation finds divergence with no pending intent, the *Arr wins.
- Degraded ≠ blocked: stale rows are returned flagged, with a non-modal banner. Never a spinner,
  never greyed-out content.
- **The boundary must be explicit** (see ADR-0017/0018): UsArr replicates anything a screen
  renders and never replicates a byte stream.

### Alternatives rejected
- **Live proxying** — the failure mode this project exists to escape.
- **Proxy with a TTL cache** — a half-measure that keeps the tail latency (every cache miss is a
  live call, and cold misses cluster exactly when a user is browsing somewhere new) while adding
  invalidation complexity anyway.

**Honest note:** **arr-dashboard already does poll-and-cache**, in its own words *"caches and
syncs locally — it does not proxy live API calls."* So this architecture is validated and it is
**not a moat.** UsArr differentiates by pairing it with cross-media search, the gateway role,
and discovery-grade UX.

---

<a id="adr-0005"></a>
## ADR-0005 — Delegate playback to media servers

**Status:** Accepted

### Context
Where people actually watch is TVs and TV boxes, then phones, then desktop browsers. Building
that client matrix natively means Kotlin/Leanback, Swift, **BrightScript/SceneGraph** (Roku's
own language and toolchain), Tizen `.wgt` packaging, webOS `ares-*` — a multi-year, multi-team
commitment. **Roku has no usable browser at all**, so a web-only frontend cannot serve the
primary surface.

Jellyfin already has Web, Android, Android TV, Swiftfin (iOS/tvOS), Kodi, Roku, Tizen and webOS
clients, plus community clients. ~40 OpenSubsonic clients and the whole OPDS reader ecosystem
exist for music and books.

### Decision
**Delegate playback.** Jellyfin primary; Emby as a feature-flagged variant; Plex secondary;
Audiobookshelf, Navidrome, Komga/Kavita/Calibre-Web for their own domains. **UsArr owns
identity, authorization, the unified catalogue, search, requests and routing.**

### Consequences
- The answer to "how do I watch on my TV?" is *"install Jellyfin's app; UsArr manages who sees
  what and routes you there."* The answer to "how do I listen on my phone?" is *"any Subsonic
  client."*
- Users must run a second service. **This is the real objection** — mitigate with a first-class
  compose bundle, a three-field wizard with a mandatory connection test, and LAN autodiscovery.
- UsArr inherits Jellyfin's bugs and API churn: the 10.11 auth-header change broke clients, the
  10.11 OpenAPI schema is reported invalid, and `PlaybackInfo` has open correctness issues.
  Mitigate with a thin adapter, version probing, and defensive parsing. **Still an order of
  magnitude less work than owning FFmpeg.**
- ⚠️ **Only one Jellyfin access token per `DeviceId`** — UsArr must mint a per-user DeviceId
  (`hash(installId + username)`) or users silently log each other out.
- 🚩 **Plex paywalled remote playback of personal media on 29 April 2025.** Support it as an
  import/browse source; document the paywall; nudge toward Jellyfin.

### Alternatives rejected
- **Building native clients** — multi-year, multi-team, on top of ADR-0006's work.
- **One-media-server-at-a-time** (Seerr's limitation) — explicitly rejected. The whole point of
  ADR-0010 is aggregating several.

---

<a id="adr-0006"></a>
## ADR-0006 — Never build a video transcoder

**Status:** Accepted · **This is a permanent non-goal, not a "not yet".**

### Context
The protocol layer is easy — fMP4/CMAF collapsed the HLS-vs-DASH choice, and that decision takes
an afternoon. **The cost is elsewhere.**

The transcode decision matrix is combinatorial: H.264 8-bit is the only universally safe video
codec; H.264 10-bit has no hardware decoder outside Apple Silicon and Rockchip; **DTS has no
browser support at all**; AC3/EAC3 fail in Firefox; **MKV is unsupported in Firefox**; **PGS
subtitles only work in MKV, otherwise burn-in** — and Jellyfin's docs state plainly that
**"burning in subtitles is the most intensive method of transcoding,"** forcing a full re-encode.

Hardware acceleration means six backends (VAAPI, QSV, NVDEC/NVENC, AMF, VideoToolbox, RKMPP),
and **Jellyfin ships its own patched FFmpeg**, stating that other builds *"will result in partial
acceleration."* You would maintain a patched FFmpeg fork.

Honest estimate: **~7–12 engineer-months to reach "worse than Jellyfin."**

### Decision
**UsArr will never assemble an FFmpeg command line from user input.** No video transcoding, no
hardware-acceleration backends, no patched FFmpeg fork.

### Consequences
- The entire v1 budget goes to the things nobody else has built, rather than reproducing a
  solved problem.
- UsArr never inherits the CVE class below.
- Users with an exotic codec on an exotic client get whatever their media server gives them.

### Alternatives rejected
- **Building it.** The decisive argument is not effort, it is the **permanent High-severity
  vulnerability class**. Jellyfin — mature, well-staffed, years of focus on exactly this — has
  shipped: **CVE-2025-31499** (FFmpeg argument injection via `/Videos/{id}/stream` → arbitrary
  file write → RCE, and **it was a bypass of a fix shipped in 10.8.13 — they got it wrong
  twice**); a second argument injection via codec parameters; **CVE-2021-21402**
  (unauthenticated arbitrary file read, path traversal in `HlsSegmentController`);
  **CVE-2021-29490** (SSRF); an IP-spoofing DoS; and a March 2026 unauthenticated image-resize
  DoS. **A greenfield hub with a small team will do worse. This alone ends the debate.**

**The line UsArr draws:** *UsArr may move bytes; UsArr may not build FFmpeg command lines from
user input.* Per ADR-0018 byte delivery is not a product goal either: the only media bytes UsArr
moves are the untranscoded audio/ebook proxy on its own northbound surfaces (ADR-0017), and video
links out.

**Conditions that would reopen this** (both required, both **measured**, not assumed):
(1) Jellyfin's API becomes unusable or hostile — a licensing change like Emby's 2018 move, or
breaking churn making the adapter cost exceed the transcoder cost; (2) there are ≥2 engineers
who can own an FFmpeg/HWA surface indefinitely **including security response**.
Explicitly **not** justifications: *"it would be cooler if it were all one binary"* or *"I want
a nicer player UI."*

---

<a id="adr-0007"></a>
## ADR-0007 — Wikidata as the cross-media spine, shipped as a prebuilt CC0 subset

**Status:** Accepted

### Context
Requirement: searching "Train Dreams" surfaces the 2025 film *and* the Denis Johnson novella,
linked. Wikidata is **the only free source that carries both the adaptation edge (`P144 based
on` / `P4969 derivative work`) and the external IDs of every downstream provider** (`P345` IMDb,
`P4947` TMDB, `P648` Open Library, `P434`/`P436` MusicBrainz). It is also **the only provider
with genuinely unencumbered terms: CC0 — no attribution obligation, no commercial restriction,
no cache limit.** (Contrast TMDB: mandatory attribution *and* a 6-month cache limit.)

Measured coverage, from live SPARQL counts: **15,360** films with `P144`; **5,314** TV series;
**34,673** `P4969` statements total; **14,443** films with both `P144` and a TMDB ID (**94%** of
adaptations are TMDB-resolvable).

### Decision (amended, revision 2)
**Wikidata is the spine.** Ship an **edges-only CC0 SQLite artifact** (`wikidata-edges.db`,
single-digit MB) as a release asset, **generated by a committed SPARQL script and regenerated per
release**. **Live SPARQL is a cache-miss path, not a dependency.** Edges come from authoritative
sources only; **nothing below 0.85 confidence is stored**, and there is no review inbox in v1.

### Consequences
- **Cross-media search is instant and works offline.** The entire edge set is a few MB.
- Redistribution is legal because Wikidata structured data is CC0.
- ⚠️ **Coverage is excellent for famous works and poor for the long tail** (~6.1k book→film pairs
  with an OL ID on the book side). The original ADR concluded from this that *"the fuzzy Tier-3
  scorer and the review UI are not optional"*. **Revision 2 reverses that conclusion while keeping
  the measurement**: a wrong link is far worse than a missing one, the design itself concedes that
  title-similarity guessing is *"a false-positive machine"*, and a false-positive-management UI is a
  second product this project cannot staff. If Wikidata does not know about an adaptation, UsArr does
  not claim one. **The fuzzy ladder is deferred, not rejected** — `work_relation` already carries the
  `confidence` and `evidence` columns it would populate, so adding it later is writing lower-confidence
  rows plus a surface, with no change to how edges are read, grouped or rendered
  ([`FUTURE.md`](./FUTURE.md) §5).
- **The weekly dump pipeline is dropped.** It committed a one-person project to ingesting tens of GB
  of Wikidata dumps and republishing an artifact forever, unpaid, with the feature quietly rotting if
  it lapsed — and the original ADR quoted the artifact's size three incompatible ways ("a few MB", "a
  few hundred MB", "fits in the release artifact"), one of which does not fit in a < 40 MB container
  image. It is also unnecessary: the measured counts are tens of thousands of rows, retrievable by
  paginated SPARQL in minutes. One number now: **single-digit MB, per release.**
- **The inverse query is mandatory.** The adaptation edge exists **only** on the derived item;
  the novella `Q85810391` carries no `P4969` back to the film. An implementation that reads a
  book's statements finds nothing. `?x wdt:P144 wd:<book>` is the core primitive.
- Must use `Special:EntityData/{QID}.json` and `query.wikidata.org/sparql`, **not `api.php`** —
  which rate-limited the researcher within a handful of sequential requests.
- Must filter disambiguation pages (`P31 = Q4167410`) — searching "Train Dreams" returns five
  items, one of which is a disambiguation page.

### Alternatives rejected
- **TheTVDB as a base layer** — **not free.** v4 requires either a negotiated commercial contract
  or "user-supported" access where **every one of your end users** must hold a paid subscription
  and supply a PIN. Make it optional and PIN-based; default TV path is TMDB + TVmaze.
- **Goodreads** — dead. Stopped issuing keys 2020-12-08, blanket 403s by late 2025. **This is
  why Readarr was archived.** Still useful only as Wikidata property `P8383`.
- **`api.lidarr.audio`** — a Servarr-run cache proxy in front of MusicBrainz with documented
  multi-hour propagation lag and repeated schema-change breakage. **Go to MusicBrainz directly
  (1 req/s, mandatory UA), or run a local mirror via the Live Data Feed.**
- **Title-similarity inference as the primary mechanism** — a false-positive machine. *The Wire*
  (TV) vs *The Wire* (album); *It* (1927) vs *It* (1986/1990/2017). Tier 3 exists to populate a
  review queue, not to create confirmed links.
- **Live SPARQL on the hot path** — violates ADR-0004 and has a 60 s timeout.

---

<a id="adr-0008"></a>
## ADR-0008 — Two plugin tiers now; a WASM tier deferred with its seam preserved

**Status:** Accepted · **Amended in revision 2** (was "three plugin tiers")

### Context
UsArr must work with (a) Prowlarr alone, (b) a full stack, and (c) **a service nobody has
written Go code for.** The ecosystem is genuinely diverse: auth spans header-key, query-key,
basic, cookie-session, bearer and token-with-client-identity; transport spans REST, JSON-RPC,
query-string RPC and XML; **error signalling includes HTTP 200 with a `Success:false` body**
(LazyLibrarian, SABnzbd); session lifecycle includes **Transmission's mandatory
409-challenge-retry handshake**.

The insight: **90% of "add a new service" is not code, it is HTTP plumbing.**

### Decision (amended, revision 2)
**Two tiers now, over one `Provider` interface resolved from a registry.** **Tier 0** compiled-in
Go. **Tier 1** declarative YAML service manifests in `$USARR_CONFIG_DIR/providers/`. **A WASM tier
is deferred**, with the registry seam that makes it cheap preserved deliberately.

### Consequences
- A user adds Komga, Kavita, a Sonarr fork or their own homebrew service in **~40 lines of YAML
  and a reload** — no compiler, no release.
- **The registry is the seam.** Providers are resolved from a registry of `ProviderFactory`
  implementations, and the sync engine never names a concrete provider type. `RemoteItem` is the
  neutral wire type every tier produces. **A WASM host — or any future tier — is one more factory
  and changes zero code in the sync engine.** That property costs one interface today and is
  expensive to retrofit, which is exactly why it is in v0.1.
- **A manifest is not a sandbox**, and the original text calling Tier 1 *"fully sandboxed (no code
  runs)"* was the most dangerous sentence in the document set, because it would have driven an
  implementation that validates nothing. A manifest is a **server-side HTTP request generator that
  runs with the instance's stored credential**. Four normative rules now govern it: URL construction
  confined by construction (`ResolveReference` forbidden — `//evil.example/x` against
  `http://sonarr:8989` escapes the host and carries the credential); mandatory escaping filters on
  every interpolation; manifest-emitted `externalIds` capped below confidence 1.0 so a manifest can
  never write a strong identity and collapse a library into one work; and **reviewed distribution**.
- **Manifests are no longer promoted as shareable gists.** A manifest chooses which endpoint on the
  configured host receives the credential and in what form, which is a straight credential-disclosure
  primitive if the author is hostile. The 6–8 bundled manifests ship **embedded in the repo,
  reviewed**, and a manifest found in `providers/` requires explicit admin confirmation, with its
  endpoints, auth placement and target host displayed, before it is bound to a credential.
- **The manifest's scope is now stated rather than implied:** a read-mostly JSON-over-HTTP service
  with stateless auth. Session establishment (qBittorrent, Deluge), challenge-retry handshakes
  (Transmission), JSON-RPC envelopes and XML (Plex) are **out of scope** and need a Tier 0 provider.
  The original ADR listed those axes as diversity the manifest accommodates, next to a grammar that
  cannot express any of them.
- **The discipline is resisting DSL growth.** When a feature request would add control flow, the
  answer is "write a Tier 0 provider" — and with WASM deferred that is the only answer, which is a
  real cost, accepted deliberately.

### Why the WASM tier is deferred rather than rejected
Deferred on **staffing and blast radius**, not on merit: a sandbox executing third-party code inside
the process that holds every \*Arr admin key is a security-critical subsystem, and this project does
not yet have the capacity to review third-party modules or to answer for one that misbehaves.
Shipping the *design* before the base exists invites someone to build it first. The specification it
would need — a fuel/interrupt budget, a hard per-call timeout, a memory-page cap, one invocation at a
time per instance, a host-function API in which **the host attaches credentials and a plugin never
receives a decrypted key**, module-hash approval, and a bounded `kv_*` store in `cache.db` rather
than `usarr.db` — is written out in [`FUTURE.md`](./FUTURE.md) §1 so it does not have to be
rediscovered.

**Revisit trigger:** Tier 1 manifests have demonstrably hit their expressiveness ceiling for a
service people actually run, **and** there is someone who can own sandbox security response.

**One argument for WASM is now dead and must not be recycled:** *"wazero is already a dependency via
`ncruces/go-sqlite3`."* That library moved to `wasm2go` and no longer depends on wazero at all
(ADR-0001, revision 2). A WASM host would be a **new** dependency and must be argued on wazero's own
merits — pure Go, zero CGO, real sandboxing — not on shared-runtime economy.

### Alternatives rejected (still rejected)
- **gRPC (hashicorp/go-plugin)** — battle-tested in Terraform/Nomad/Vault, but unsandboxed and
  requires shipping and supervising extra processes inside the container, which is exactly the
  sidecar complexity ADR-0002 rejects. **Helm 4 explicitly ruled it out "as it wasn't as secure
  as the new Wasm plugin model"** — a recent, direct precedent from a major Go project.
- **Go `plugin` package (`.so`)** — must match the Go build version exactly. Brutal.
- **Compiled-in only** — fails requirement (c) outright, and makes every new service a PR plus a
  release.

---

<a id="adr-0009"></a>
## ADR-0009 — `work` / `edition` / `file`, with typed `work_relation` edges

**Status:** Accepted

### Context
FRBR defines four layers (Work → Expression → Manifestation → Item). **BIBFRAME collapses Work +
Expression into one.** **Wikidata went the other way**, collapsing Expression and Manifestation
into a single "edition" concept — a two-layer work/edition framework. **Open Library uses the
same two-layer model**, and states explicitly: ***"if a work has been adapted or retold, it is
considered a unique work, different from the original."***

### Decision
Three layers — **`work` (kind-scoped) / `edition` / `media_file`** — and:

> **The film *Train Dreams* and the novella *Train Dreams* are two different `work` rows joined
> by a typed `work_relation` edge — NOT one work with two editions.**

### Consequences
- The two items keep **separate monitoring state, separate service instances, and separate
  quality profiles** while being presented as connected. A merged-work model cannot do this.
- The UI can say **"Based on the novella by Denis Johnson"** with a real relationship type
  instead of silently merging two things.
- **Never auto-merge across `kind`.** The identity cascade's tiers 3–5 all require same-kind;
  cross-kind association is always a *link*, never a merge.
- Merges are recorded in `work_merge` and are **reversible.** Without this, one bad heuristic
  permanently corrupts a library and produces a bug report you cannot fix.
- The `edition` layer looks degenerate for movies and TV — but it is what makes the Portuguese
  translation *Sonhos e Comboios* the same work as *Train Dreams* (a title match would never
  find it), and it is what ADR-0014's 1080p/4K case rides on.
- `work_relation` carries **`evidence` (JSON) and `status`**, not just a confidence float,
  because the review inbox is unusable without being able to explain *why* two things linked.

### Alternatives rejected
- **One work with two editions** — produces a table where most columns are null for most rows
  (the film has a director and runtime; the novella has an author and page count), and destroys
  the ability to monitor the movie but not the book. Contradicted by two independent, mature
  bibliographic systems.
- **Full four-layer FRBR/WEMI** — Wikidata declined it *"due to boundary definition
  difficulties."* If Wikidata found the Expression/Manifestation boundary too hard to define, a
  media hub will too.
- **External IDs as a JSON blob on the item** (Track 01's sketch) — cannot be indexed for
  `(source, value)`, which is *the* hot lookup during sync.

---

<a id="adr-0010"></a>
## ADR-0010 — OpenSubsonic and OPDS as northbound server surfaces

**Status:** Accepted · **Promoted in priority by the gateway reframing (ADR-0018).**

### Context
UsArr is a two-sided gateway: point every client at one endpoint with one credential and have
the right library appear. Speaking a protocol clients already know is vastly cheaper than
building clients: **OpenSubsonic gets ~40 music apps free** (Symfonium, Amperfy, Supersonic,
play:Sub, DSub, substreamer, Nautiline, NaviBeat…); **OPDS gets KOReader, Panels, Librera and
Moon+ Reader** — the e-ink and offline crowd a web app can never serve. **DroppedNeedle already
proves the pattern**, emulating both OpenSubsonic and Jellyfin server APIs.

### Decision
Implement **OpenSubsonic** and **OPDS 2.0 (with 1.2 fallback)** as server surfaces.
**OpenSubsonic with `apiKeyAuthentication` ONLY. Never implement salt/token auth.**

### Consequences
- 🚩 **The auth rule is the whole point.** Classic Subsonic auth is `u` + `t` + `s` where
  `t = md5(password + salt)`, which **mathematically requires the server to hold the password in
  recoverable form.** Navidrome's own docs concede it — *"unable to properly hash passwords and
  thus encrypts them instead"* — with a key that by default ships in its source code. The
  `apiKeyAuthentication` extension exists to fix exactly this, and **the spec states servers
  offering API-key auth should no longer support salt/token auth.** Taking that trade lets
  **Argon2id remain the only password storage in UsArr.**
- **A minority of ancient clients will not work.** That is strictly the right trade. **Document
  it prominently; users will ask.**
- Per-user, per-app, revocable `client_credential` rows become the northbound identity, which is
  also what makes "sign out my ex-roommate's Fire Stick" possible.
- ⚠️ Client spec coverage is uneven — one source indicates **Feishin** still targets Navidrome's
  internal API and Jellyfin's API rather than full OpenSubsonic. **Test against Symfonium as the
  reference client.**
- A **Jellyfin-compatible surface is deferred and unproven** — on no milestone, with its seam
  recorded in [`FUTURE.md`](./FUTURE.md) §6. The API is large, under active change, and its
  playback-negotiation surface is precisely what UsArr does not want.

### Alternatives rejected
- **Implementing salt/token for compatibility** — would force recoverable password storage, i.e.
  reproduce Navidrome's acknowledged compromise, for a shrinking tail of old clients.
- **Building native music/reader clients** — ~40 already exist and are better than anything this
  project would ship.
- **A UsArr-proprietary API only** — every client integration would then be someone else's work
  that never happens.

---

<a id="adr-0011"></a>
## ADR-0011 — Named permission strings, not a bitfield

**Status:** Accepted

### Context
Overseerr/Jellyseerr/Seerr store permissions as a **bitfield** on the User entity with a
`hasPermission(perm | perm[], options)` helper, roles of Owner/Admin/User, per-user request
quotas, and 4K-specific permissions.

### Decision
**Named permission strings** in a `role_permission` join table — `media.video.request`,
`media.music.play`, `media.book.download`, `requests.approve`, `requests.autoapprove.movie`,
`admin.services.configure`, `admin.users.manage`. Roles as bundles, plus per-user grants and
**explicit denies where deny wins**.

### Consequences
- UsArr has **more media types (video, music, books, comics, audiobooks) × more verbs (browse,
  play, download, request, approve, manage)** than Overseerr. **A 64-bit field would run out and
  become unreadable** long before the feature set is complete.
- Permissions are greppable, self-documenting in logs and the audit trail, and extensible
  without a migration that renumbers bits.
- Per-media-type grants fall out naturally: a user can request books but not 4K movies.
- Auto-approve becomes a *permission* (`requests.autoapprove.book`) rather than a boolean flag,
  so it composes with the rest of the model — which is what makes ADR-0019's single-user mode a
  configuration rather than a code path.
- Slightly more storage and a join. Irrelevant at this scale.
- **Library visibility lives in UsArr's own `user_library_access` table** and is enforced in
  UsArr's query layer. Jellyfin's `EnabledFolders` is defence in depth, **not the boundary** —
  Jellyfin's parental controls have documented gaps (special features do not respect them).

### Alternatives rejected
- **Copying the bitfield** — compact and fast, and wrong for this domain's cardinality.
- **Full RBAC/ABAC with a policy engine** (Casbin, OPA) — overkill for a household media hub and
  a dependency users would have to understand to debug.

---

<a id="adr-0012"></a>
## ADR-0012 — Four-channel sync with an intent log for writes

**Status:** Accepted

### Context
ADR-0004 makes UsArr a replica, which means it owns cache coherency. No single sync mechanism is
sufficient: SignalR frequently fails behind a reverse proxy (Radarr's own docs say so),
`/history/since` only reports history-generating events, and a full import is always slow.

### Decision
**Four channels:** full import → SignalR push → `/history/since` delta poll → periodic
reconciliation sweep, plus a webhook receiver. **Writes go through an intent log** with
optimistic apply, three-phase settlement (`applied` → `confirmed`), inverse-patch rollback and
ULID idempotency keys.

### Consequences
- **Channel 2 is an optimisation; channel 3 is the correctness guarantee; channel 4 is the
  safety net.** Correctness must never depend on SignalR staying up.
- `/history/since` **exists in all five apps** (verified from the shipped specs) — ⚠️ but
  behaviour parity is unverified; probe at connect time.
- **`applied` ≠ `confirmed`.** A 201 from Radarr means "queued", not "exists consistently". Only
  an independent observation (SignalR or delta poll) clears the pending affordance in the UI.
- **Retry only idempotent-safe kinds.** `grab` is **not** blind-retryable (double download) — one
  attempt plus a manual button.
- **Rollback is never silent.** A poster that quietly un-monitors itself is worse than an error
  toast.
- **The 7-day tombstone on upstream deletions is a hard requirement.** *"My NAS unmounted and
  UsArr deleted everything"* is the nightmare bug, and one `deleted_at` column plus a delay
  prevents it entirely.
- **Streaming JSON ingest is mandatory**, not an optimisation — list endpoints are unpaged, a
  10k-movie response is 30–80 MB, and buffering *and* unmarshalling peaks at 200–400 MB on a
  1 GB Pi. arr-dashboard had to retrofit exactly this.
- **This is the biggest schedule risk in the project.** Everything else is mechanical; the intent
  log's failure and reconciliation semantics are where subtle bugs will live. **Budget a
  fault-injection harness that randomly 500s and times out a fake *Arr.**
  → **Revision 2 acted on this sentence rather than repeating it: see ADR-0012a.**

### Revision 2 — two corrections to the channel model

1. **The shipping order was backwards.** SignalR (channel 2) shipped before reconciliation (channel
   4), but `/history/since` provably cannot observe a movie *removed* from Radarr (removing it
   deletes its history rows), a `monitored` toggle, a quality-profile change, or a root-folder move.
   Without channel 4 the only repair for divergence is a manual full re-import, for most of the
   project's life. Reconciliation is also the *simplest* channel and is fully specified. **Channel 4
   moves into v0.1; SignalR and webhooks move out.**
2. **Prowlarr is not a channel-3 source.** Its `/history/since` exists, but its `HistoryEventType`
   enum is `unknown, releaseGrabbed, indexerQuery, indexerRss, indexerAuth, indexerInfo` — indexer
   telemetry, not entity change. `indexerQuery`/`indexerRss` fire on every RSS poll of every indexer,
   and `prowlarr.HistoryResource` has no `movieId`/`seriesId`. Channel 3 applies to library-bearing
   acquisition apps only. (Also: **six** apps expose `/history/since`, not five — Whisparr is the
   sixth.)

**Two guards are now mandatory on the sweep**, because "the \*Arr owns the truth" is dangerous when
the \*Arr is lying: **id resurrection** (the \*Arrs reuse integer ids after deletion, so a tombstoned
link can rebind a *different* movie to the old `work_id` — compare external identity before clearing
`deleted_at`) and **instance identity generation** (an \*Arr restored from an older backup moves its
id space backwards; on a fingerprint change or a backwards jump in `max(id)`, refuse to sweep).

### Alternatives rejected
- **SignalR alone** — breaks behind reverse proxies, which is the common deployment.
- **Polling alone** — either too slow to feel live or too aggressive on the upstream. The *Arrs
  have no rate limit but serialise the whole object graph on every list call.
- **Webhooks alone** — require writing config into the user's *Arr, and give no bootstrap path.
- **Synchronous writes** (block the request until the *Arr answers) — violates ADR-0004 and gives
  up the sub-10 ms ack that makes the UI feel instant. **Revision 2 partially reverses this
  judgement: see ADR-0012a.**

---

<a id="adr-0012a"></a>
## ADR-0012a — A durable command queue replaces the optimistic intent log

**Status:** Accepted (revision 2) · **Supersedes the write-path half of ADR-0012.**

### Context
ADR-0012 specified optimistic local apply, a stored `inverse_patch`, three-phase settlement where
`applied ≠ confirmed`, per-kind retry-safety analysis, and a client-side pending-intent overlay
reconciled against SSE — and then named itself *"the biggest schedule risk in the project"*.

All of that machinery exists so that a monitor toggle feels instant instead of taking one round trip.
The owner's speed complaint was about **browsing and API-driven page loads**, not about writes.
Writes in this product are rare and deliberate: request a thing, toggle monitored, delete. A
200–400 ms spinner on a button the user just pressed is normal and expected.

Three defects made the cost worse than the benefit. **The `applied` state had no timeout and no
guard**, so a write that the \*Arr accepted but that was never independently confirmed — the *normal*
case once SignalR was deferred out of v0.1 — was unguarded against the reconciliation sweep and would
be silently reverted. **Rollback was not real**: `inverse_patch` reverts UsArr's local state only, so
a timeout after Radarr committed produced a local rollback, a failure toast, and the item reappearing
hours later with no explanation. And **`idempotency_key` was globally `UNIQUE`** rather than
user-scoped, so a collision or replay returned another user's intent row.

### Decision
**A durable command queue.** States `pending → inflight → done | failed_rejected | verifying → done |
failed`. `POST` returns `202 {command_id}`; the UI shows an inline pending chip on that one item and
resolves it from SSE. **No optimistic apply, no `inverse_patch`, and therefore no rollback — because
nothing was applied locally.**

Four things are kept or added deliberately:

1. **`verifying` is the state that matters.** A timeout, transport error or 5xx means the write
   *might* have landed, so it triggers a **targeted refetch of the affected entity** rather than a
   failure report. TTL 15 minutes, then one final verification and an explicit `failed` with a
   reason. No state is unbounded. A 4xx **with a body** is `failed_rejected` and may be reported
   immediately.
2. **`UNIQUE (user_id, idempotency_key)`**, not a global unique. A key that exists under a different
   user returns `409`, never the other row.
3. **Northbound idempotency keys are derived server-side** from
   `(user_id, client_credential_id, verb, usarr_id, coarse_timestamp)`, because OpenSubsonic and OPDS
   carry no idempotency field. One rule for every surface; there is no second scheme for scrobbles.
4. **`grab` remains max one attempt** plus a manual retry, because a blind retry is a double
   download.

**The reconciliation guard covers every non-terminal state** — `pending`, `inflight` *and*
`verifying`. Guarding only the first two means every v0.1 write is reverted by the next sweep.

### Consequences
- The project's self-declared top schedule risk is removed rather than restated.
- The UI is honest instead of optimistic: a pending chip and a real error beat a value that
  silently reverts.
- **Optimistic apply is deferred, not condemned** ([`FUTURE.md`](./FUTURE.md) §10). The queue's state
  machine already distinguishes "definitely did not land" from "might have", which is the hard half;
  a future contributor with more people can add optimistic apply on top **deliberately, rather than
  by drift.**

---

<a id="adr-0013"></a>
## ADR-0013 — Two-tier search; an external engine and typo tolerance deferred

**Status:** Accepted · **Amended in revision 2** (was "three-tier … → optional Meilisearch")

### Context
The requirement is typo tolerance (`train dremas`), as-you-type prefix matching, cross-entity
ranking where a movie row and a book row compete on comparable scores, and owned/unowned in one
result set. FTS5's default tokenizers do prefix matching but **not fuzzy**; its `trigram`
tokenizer is explicitly built for generalized substring matching. Benchmarks show only
Typesense and Meilisearch handle misspellings properly — **but also that in-memory SQLite is by
far the fastest** on raw FTS latency.

### Decision (amended, revision 2)
**Tier 1:** a client-side prefix index in IndexedDB over **top-level works only**, capped at 25,000
items. **Tier 2:** two FTS5 tables (`unicode61 remove_diacritics 2` with `prefix='2 3 4'`, and
`trigram`) fused by **Reciprocal Rank Fusion (k=60)**, then a Go re-rank of ≤200 candidates by
Jaro-Winkler + popularity + `in_library` + an IDF penalty, followed by **media-type diversity
injection**. **There is no Tier 3.** An external engine is deferred behind the retained
`SearchProvider` boundary.

### Consequences
- **Tier 1 is where the "instant" feeling actually comes from** — ~80% of real queries resolve
  with literally zero network.
- **RRF is used because the two engines' scores are not comparable.** BM25 over a unicode61
  index and BM25 over a trigram index are on different scales; RRF uses them purely for
  `row_number()` ranking within each set.
- **Media-type diversity injection is what makes the Train Dreams case work.** Without it,
  whichever medium has better text statistics sweeps the list and the novella never appears.
- **`in_library` boost is the single most user-satisfying signal and everyone forgets it.**
- **Short common titles must be penalised hard** via corpus IDF, or "It"/"Her"/"Us" swamp everything.

### Revision 2 — three corrections and one deferral

1. **The typo-tolerance claim was false and is withdrawn.** This ADR's own context opened with
   *"the requirement is typo tolerance (`train dremas`)"*, and the design does not deliver it. FTS5's
   `trigram` tokenizer is a **substring** matcher, not a fuzzy one: `MATCH 'dremas'` finds rows
   literally containing `dremas`, so a transposition destroys the match and neither FTS table
   retrieves the row. The Go re-rank only reorders candidates *already retrieved*. Tier 2 gives
   **prefix and substring matching**, the UI says so, and the README no longer claims otherwise.
   **Typo tolerance is deferred** (`spellfix1`/`editdist3` or a Go-side BK-tree as a fourth retrieval
   leg), with its costs and its ⚠️ unverified extension-availability question recorded in
   [`FUTURE.md`](./FUTURE.md) §3.
2. **Tier 1's sizing was wrong by ~40×.** "~1.2 MB for 10k items" described an index "for the whole
   library", but the reference library is ~412k `work` rows once episodes are counted — 33–50 MB
   shipped on every cold load. Scoping it to top-level works (~13k rows), dropping `sort_title`,
   sending ThumbHashes as raw bytes and capping at 25,000 items makes it ~1.5–2.1 MB and *true*.
3. **Both FTS tables need `contentless_delete=1`** (SQLite ≥ 3.43) or deleted works stay in the index
   forever, and the three tables must share one rowid space or RRF silently fuses unrelated
   documents. Both are now invariants with CI assertions.

**Meilisearch is deferred, not rejected.** This ADR previously kept it as a named tier with config
variables, a checklist row and a README row — implying someone would build and support it — while
§8.4 of the architecture argued against itself (*"the library is ~50k short strings, not 50M
documents; a 200 MB search server for that is absurd"*). **The seam is kept and is the part that
matters:** retrieval is separated from ranking, the fusion takes N legs, and the re-rank never learns
which engine produced a candidate — so an external engine, or a typo-tolerant index, is **an added
leg rather than a rewrite**. The `SearchProvider` interface boundary costs one Go interface.
**Revisit trigger:** a real user with a library where FTS5 latency or recall is measurably
inadequate, measured on `make bench`.

### Alternatives rejected
- **Bleve (Go)** — its fuzzy path is not Levenshtein-automaton-backed (a long-open issue says
  so), and it costs +50–150 MB.
- **Tantivy** — best in class (edit distance ≤2, sub-millisecond queries), **Rust only.** A real
  reason to prefer Rust if the team were already fluent; not enough to overturn ADR-0001.
- **Typesense over Meilisearch** — Typesense stores indexes **entirely in memory** and requires
  ≥2 vCPUs; Meilisearch is memory-mapped (LMDB), so dataset size is not bounded by RAM. **The
  memory-mapped property is the right one for a Pi or NAS.**
- **`sqlite-vec` semantic search** — would work at this scale, but needs an embedding model:
  either a runtime dependency or a remote call, both violating zero-dependency. ⚠️ Also, TMDB's
  ToU names ML training/validation as commercial use, so embedding TMDB overviews is legally
  murky. Deferred to a phase-3 plugin over Wikidata/local text.

---

<a id="adr-0014"></a>
## ADR-0014 — `service_item_link` is many-to-many

**Status:** Accepted

### Context
The most common advanced *Arr topology is **two Radarr instances — one 1080p, one 4K — both
tracking the same films.** Also common: Sonarr plus Sonarr-Anime, and a Lidarr per quality tier.
The *Arr web UIs cannot show you a unified view of this; you check two dashboards.

### Decision
`service_item_link` is **M:N**: one canonical `work` maps to rows in many instances at once,
each with its own `remote_id`, `monitored` state, quality profile and root folder.

### Consequences
- **One poster with a "1080p ✓ / 4K ✗" badge** — **a free consequence of the M:N link and a strong
  signal to power users.** *Revision 2 demotes the framing:* it was previously called "the flagship
  differentiating feature" that "should be on the landing page", but it requires the user to run
  **two Radarr instances**, which is a power-user topology and may not even be the owner's. Calling
  it the flagship aims the headline at a narrow audience and displaces what was actually asked for.
  The landing-page claim is: *one place to search everything you have and ask for what you don't.*
  The capability stays — it costs nothing extra.
- The `availability` summary is denormalised onto `work` so the grid renders the badge without
  a join.
- Conflict rule when instances disagree on shared metadata: **highest `priority` among
  `is_authoritative` links wins; otherwise most-recently-synced.** Log divergences rather than
  flip-flopping between them.
- `remote_hash` must hash only the **synced subset** — fields like `sizeOnDisk` churn constantly
  and would defeat the delta short-circuit entirely. Done right, a reconciliation sweep touches
  <1% of rows.
- **Northbound IDs address a link, not a work** (ADR-0021), which falls out of this decision.

### Alternatives rejected
- **One instance per work (1:N)** — cannot represent the dual-Radarr case at all, which is the
  single topology most worth solving.
- **A separate `work` row per instance** — duplicates the entire library, breaks search,
  duplicates every poster, and makes cross-media linking meaningless.

---

<a id="adr-0015"></a>
## ADR-0015 — Namespaced tags with virtual parents

**Status:** Accepted

### Context
*Arr tags are `{id: int, label: string}` — **integer-keyed, per-instance labels with no
namespace**; two instances' tag `3` are unrelated. But `TagDetailsResource` reveals what tags
actually *do* upstream: they are join keys for delay profiles, import lists, notifications,
restrictions, indexers, download clients and auto-tagging rules. Meanwhile **Hydrus has the best
tag model in the space**: namespaces, siblings (aliases), and **virtual parents** —
*"parents are virtual; the parent is not actually added to every file."*

### Decision
Namespaced `namespace:value` stored as **two indexed columns**, never one string. A system/user
split (system tags are derived and undeletable), a rule/static split, **Hydrus-style
`tag_alias` siblings and `tag_implies` virtual parents resolved at query time**, and smart
filters as saved queries that are **never** tags.

### Consequences
- `type:audiobook` implies `type:book` **at query time**, with no duplicated rows and no re-tag
  when an implication changes.
- **Source provenance becomes a first-class filterable attribute** — `source:usenet`,
  `source:torrent`, `source:irc`, `source:direct` — which **no surveyed frontend exposes.**
  Small, distinctive, cheap.
- **Tags attach to config objects, not just media** (`tag_assignment.service_instance_id`),
  because that is what upstream tags are for.
- **Inheritance direction must be per-namespace and getting it wrong is a real bug:**
  `user:favorites` and `collection:*` inherit downward; `source:*`, `quality:*`, `codec:*` are
  **per-file and must not** ("this series is `source:usenet`" is meaningless when episode 4 came
  from a torrent).
- **Rule of thumb: if the answer changes when the underlying data changes, it is a filter, not a
  tag.** This prevents the classic bug where a "smart tag" goes stale and users cannot tell why
  an item is in a collection.
- The rule engine copies *Arr's `AutoTaggingSpecification` shape
  (`{implementation, fields, negate, required}` + `removeTagsAutomatically`) **almost verbatim**
  — it already exists, users already understand it, and `negate` + `required` is exactly the
  right expressiveness without becoming a query language.
- *Arr tags import into `sonarr-tag:`/`radarr-tag:` namespaces with an explicit "promote"
  action, **never silently reinterpreted**; write-back only ever from those namespaces.

### Alternatives rejected
- **Flat labels like the *Arrs** — no way to express `type:` vs `source:` vs `user:`, and
  collisions across instances are guaranteed.
- **Storing `"ns:value"` as one string** — kills the `(namespace, value)` index and forces string
  parsing on every filter.
- **A fully polymorphic `(entity_type, entity_id)` assignment table** — forfeits FK integrity and
  makes the planner guess. **Resolved as a synthesis:** one `tag_assignment` table with real
  nullable FK columns per target and a `CHECK` that exactly one is set.
- **Materializing inherited tags onto children** — doubles ingest writes and makes changing an
  implication a full re-tag.

---

<a id="adr-0016"></a>
## ADR-0016 — Tailnet as the default deployment assumption

**Status:** Accepted · **Added after the initial design pass, on the project owner's input.**

### Context
The owner's stated deployment: *"i reckon this would be on someone's tailscale network."* That
changes the threat model and, more importantly, it changes what is *reachable*: on a tailnet
every client device can already reach every backend directly.

### Decision
**Assume a Tailscale tailnet (or equivalent WireGuard overlay).** Internet-exposed deployment is
**supported but hardened-secondary**, not the default. Because the backend is Go,
`tailscale.com/tsnet` can embed a tailnet node directly in the binary.

### Consequences
- **This resolves ADR-0017** — the sole objection to 302-redirecting streams was "the client
  might not reach the backend", which is false by construction on a tailnet.
- The network is authenticated at the transport layer before UsArr sees a packet. The threat
  model becomes "people I deliberately let onto my tailnet", not "the internet".
- **tsnet is a genuinely better deployment story**: UsArr appears as its own tailnet device with
  its own MagicDNS name and ACL entry, no host `tailscaled`, no published Docker port. **This is
  only available because of ADR-0001.**
- **Tailscale identity becomes a legitimate zero-config auth source** (`auth_source='tailscale'`)
  — but it is **still trusted-header auth**, so the trusted-proxy rules apply in full. **Prefer
  `LocalClient.WhoIs` over headers**, because `WhoIs` derives identity from the actual tailnet
  connection rather than from a header a client could set.
- ⚠️ **Specifics are stated from research notes, not a live API test, and must be verified:**
  the `tsnet.Server.Listen` → `net.Listener` shape, `LocalClient().WhoIs()`, Tailscale Serve's
  automatic HTTPS, and the exact `Tailscale-User-Login` / `Tailscale-User-Name` header names and
  the conditions under which they are injected.
- **Do not make Tailscale a requirement.** A plain listener behind any reverse proxy stays fully
  supported; tsnet is an *additional* listener behind `USARR_TSNET_ENABLED=1`.
- SSRF (ADR: §14.2) and encrypted credential storage **stay top-priority** — they apply on a
  tailnet exactly as on the internet. Public-internet hardening moves to a clearly-marked
  "if you expose it" checklist.

### Alternatives rejected
- **Designing for the public internet by default** — would force TLS/HSTS/CSP/forced-setup
  friction on every user for a threat most do not have, and would have kept the byte-proxy on the
  hot path forever.
- **Requiring Tailscale** — excludes users on plain LANs, other overlays, or reverse proxies.

---

<a id="adr-0017"></a>
## ADR-0017 — Stream path: proxy audio and ebooks, link out for video

**Status:** **Reversed in revision 2.** The original decision — "302 redirect by default, byte-proxy
as an opt-in fallback" — is recorded below in full, along with what falsified it. This is a reversal,
not a rewrite: the reasoning that produced the wrong answer is worth keeping, because it was sound
given a premise that turned out to be false.

### Context
A gateway that aggregates N backends behind one client credential must decide who carries the
bytes. Proxying keeps one endpoint and one credential and hides backend URLs — but puts UsArr on
the path for gigabytes and requires correct `Range`, `Content-Range`, `206` and `If-Range`
handling, which is a real, bug-prone surface. Redirecting is nearly free and preserves the
backend's native seek behaviour — but leaks the backend URL and **fails if the client cannot
reach the backend.**

ADR-0016 removes that last objection.

### The original decision (superseded)
> **`302` redirect is the default.** A byte-proxy mode exists as an opt-in fallback
> (`USARR_STREAM_MODE=proxy`, with per-instance override). The remaining honest problem is
> credential leakage in the redirect target, solved in preference order by (1) a **backend-native
> ephemeral token** minted per request — *"Jellyfin can issue scoped access tokens; Navidrome
> supports its own API keys"* — (2) a short-TTL signed URL, (3) a per-backend proxy.

### What falsified it
**Mitigation 1 does not exist**, and the whole safety case rested on it.

- ✅ `jellyfin/jellyfin#10808` — *"Refactor 'Copy Stream URL' to not leak the user's session API
  key"* — is an **open issue proposing** per-object scoped keys, filed precisely because Jellyfin has
  no such thing today and its stream URLs carry the user's **full session token**.
- ✅ **Navidrome does not support OpenSubsonic `apiKeyAuthentication`** in any release or in `master`
  (v0.63.2 latest; PRs #4022 and #5731 open, neither merged; its advertised extensions are
  `transcodeOffset, formPost, songLyrics, indexBasedQueue, transcoding, playbackReport,
  topSongsByArtistId, sonicSimilarity`). There is no Navidrome API key to mint either.

Three further defects, each fatal on its own:

1. A Jellyfin access token **is** a backend credential. Once in a client's hands it authenticates
   arbitrary API calls against that backend for its lifetime — exactly the "replay against the
   backend outside UsArr's authorization" the credential rule forbids, and it silently un-does
   library visibility and parental controls on a tailnet whose threat model explicitly includes a
   kids account.
2. Neither backend can mint a token **bound to `(user_id, usarr_id)`**; their tokens are scoped to a
   backend *user*, not to one item. The stated binding rule was unimplementable via mitigation 1.
3. A minutes-lived token **breaks seek**, because most Subsonic clients issue a new `Range` request
   to the *same* URL rather than re-calling `stream` — the hardest failure mode to diagnose, since it
   appears mid-playback rather than at start. Cookie-session backends cannot be redirected to at all.

### Decision (revision 2)
**UsArr proxies bytes for its own OpenSubsonic and OPDS surfaces (audio, ebooks, comics), and links
out for video.** The proxy is a plain `io.Copy` with correct `Range` / `Content-Range` / `206` /
`If-Range` / `ETag` / `Accept-Ranges` handling and **no transcoding, ever**. `USARR_STREAM_MODE` is
gone; there is no redirect mode to fall back to.

### Consequences
- **No client ever receives a backend credential.** That is the property this reversal buys, and it
  is worth the cost.
- **UsArr is on the byte path for audio.** Stated plainly rather than minimised: `Range` handling is
  a genuine source of subtle bugs. Mitigations: audio is ~1–5 Mb/s rather than a 60 Mb/s 4K remux;
  there is no transcode; and the failure mode of getting it wrong is a client that cannot seek, not a
  leaked credential.
- **Video links out**, so the ruinous byte cost never arrives and the northbound surfaces advertise
  no video stream endpoint at all. Carrying video is deferred, not refused
  ([`FUTURE.md`](./FUTURE.md) §7) — and if it ever lands it is `io.Copy` and nothing else. **A
  transcoder remains a permanent non-goal (ADR-0006).**
- **A short token TTL no longer breaks seek**, because the bytes come from UsArr: a client
  re-`Range`s the same UsArr URL and UsArr re-authorizes. The token is now fully specified — numeric
  TTL (120 s default, 600 s max), a nonce, a replay cache, revocation checked against
  `client_credential.revoked_at` on **every** redemption, and a signing key derived under a distinct
  HKDF label from the credential KEK so the two rotate independently.
- **Images remain the deliberate exception in the other direction**: `/img/*` is always proxied and
  cached, never redirected — `MediaCover` requires an API key, the downscale + ThumbHash pipeline is
  the biggest perceived-speed win, and a poster is ~30 KB.
- ⚠️ The per-client redirect-tolerance question disappears with the redirect. It is replaced by a
  narrower test matrix on the proxy: seek, resume after pause, large-file range, repeated requests to
  the same URL.

### Alternatives rejected
- **Keeping the redirect and accepting the leak** — would mean documenting, loudly, that any tailnet
  client can extract a backend-user-equivalent credential. On a network whose threat model is "people
  I deliberately let on", including a kids account, that is not an acceptable default.
- **Cutting the byte path entirely and linking out for everything**, which is what one reviewer
  proposed. It does not work for the surfaces UsArr actually ships: a Subsonic client has exactly one
  `stream` verb and no way to be sent somewhere else, so "link out" for audio means the OpenSubsonic
  surface does not play music, which is the milestone's entire success criterion. Link-out is
  correct for video precisely because video has a first-class alternative — the backend's own client.

---

<a id="adr-0018"></a>
## ADR-0018 — No in-app player; UsArr serves no media bytes as a core capability

**Status:** Accepted · **Narrows ADR-0005. Added on the project owner's input.**

### Context
An earlier draft planned a minimal in-UsArr web player calling Jellyfin's `PlaybackInfo` with a
browser `DeviceProfile` — UsArr owning the chrome, Jellyfin owning the transcode. The owner
narrowed the scope: *"we don't need the capability to stream or do anything else. just have a
unified source that plugs into your favorite players."*

### Decision
**UsArr has no in-app media player, and byte delivery is not a product goal.** The northbound
protocol surfaces (ADR-0010) **remain and go up in priority** — they are how "your favourite
players" plug in — but **their job is catalogue, search and routing.**

### Consequences
- The product is a **gateway**, and the framing sharpens: northbound protocol surfaces +
  southbound aggregation + one canonical library + requests.
- ADR-0006 becomes even more clearly correct — there is now no in-house playback surface at all
  to tempt anyone toward "just a little transcoding".
- The web SPA is for browsing, searching, managing and requesting. "Play" hands off.
- Roadmap effect: the player is removed entirely (not deferred).
- **Where the replica model stops must be stated explicitly** so this does not read as a
  contradiction: UsArr replicates anything a screen renders (metadata, browse, search,
  availability, links, tags) and **never replicates a byte stream**.

**Revision 2 corrects one clause.** This ADR previously said the northbound surfaces' job is
*"catalogue, search, routing and **redirect**, not byte delivery"*. The redirect is gone (ADR-0017),
and for audio and ebooks **UsArr does carry the bytes** — because OpenSubsonic and OPDS have exactly
one acquisition verb and no way to hand a client off safely. That is not a reversal of *this* ADR's
intent: byte delivery is still not a product goal, there is still no player, and there is still no
transcoding of any kind. Video, which is where the byte cost is ruinous and where a first-class
alternative exists, **links out to the backend's own client**. ADR-0023 states the general rule:
link out where a better neighbour exists; carry bytes only where the protocol leaves no
alternative.

### Alternatives rejected
- **Shipping the minimal web player anyway** — it is genuinely cheap (an `<video>` over a
  delegated HLS URL) but it is scope the owner explicitly declined, it creates an expectation of
  playback support, and it drags in device-profile handling and `TranscodeReasons` diagnostics
  that only matter if you intend to own the experience.

---

<a id="adr-0019"></a>
## ADR-0019 — Single-user first, multi-user schema from migration 0001

**Status:** Accepted

### Context
The owner: *"maybe starts as single user only but it could open up."* Multi-tenancy is expensive
to retrofit — it touches every query, every index and every API response — while hiding a UI is
free.

### Decision
**v0.1 ships single-user** (one implicit owner, user-management UI hidden). **But every table
that should be user-scoped carries a `user_id` from migration 0001.**

Scoped from the start — **this list is authoritative; the architecture's §1.3 previously carried a
shorter one, and both claimed to be the hard rule**: `request`, `tag_assignment` (user-namespace
tags), `playback_state`, `play_history`, `playlist`, `saved_filter`, `client_credential`, `session`,
`audit_log`, `write_queue`. The review-inbox verdicts on `work_relation` are **removed** from the
list, because revision 2 removed the review inbox (ADR-0007); manually-created links are
instance-global in v0.1 and **must become user-scoped when multi-user lands**, which is recorded as
a v1.0 obligation in `reference/schema.md` §11 rather than left to be discovered.

**A second rule joins it in revision 2, and it is the one that is expensive to retrofit:** every read
path that aggregates across instances takes an **access-scope parameter** in its query signature —
the library grid, search, the client prefix index, the availability rollup and every northbound
surface. A denormalised rollup or a shipped-to-client index computed across instances a user cannot
see is an existence oracle, and bolting a filter on afterwards is exactly the cost this ADR refuses
to accept for `user_id` columns.

### Consequences
- Every query is written user-scoped from day one, so multi-user is a UI unlock plus a role
  seeding, not a migration.
- Auto-approve is a *permission* (ADR-0011), so single-user mode is the owner holding
  `requests.autoapprove.*` — a configuration, not a separate code path. The request state machine
  runs identically in both modes.
- Small cost: a `user_id` column and predicate that is trivially satisfied in single-user mode.
- Risk avoided: the class of bug where one user sees another's favourites or reading position
  because a query was written before multi-tenancy existed.

### Alternatives rejected
- **Genuinely single-user schema, migrate later** — the expensive option, and the one most
  projects regret.
- **Full multi-user UI in v0.1** — scope that delays the thing actually worth proving (that the
  replica model is fast on real data).

---

<a id="adr-0020"></a>
## ADR-0020 — Requests are a first-class pillar

**Status:** Accepted

### Context
The owner: *"you can view and search everything in your library everywhere. and if you want
something you don't have, you can do that here too."* That is one interaction, not two features.

Prior art confirms the demand and the difficulty: Seerr owns discovery and requests but is
movies+TV only; **SeerrNG is the only project shipping cross-media requests, at 19 stars**, and
names *provider matching* and *ebook-vs-audiobook service routing* as its two friction points.
Two of the three flagship request bots are archived, but the demand they served — request
without opening a web app — was real.

### Decision
Requests are a pillar equal to the unified library. **One search box returning owned and
unowned in one ranked result set** with explicit availability state, and **one Add action that
routes** to whichever backend can service it.

### Consequences
- Availability state is rendered per medium from `service_item_link`:
  `available` / `partial` (**"1080p ✓ / 4K ✗"**) / `monitored` / `requested` / `absent` /
  `unroutable`.
- **Routing is explicit and ordered**: user choice → routing rule → capability filter (probed
  `Caps.MediaKinds`) → highest `priority` among healthy instances → `unroutable` **with the
  reason surfaced.** Never silently drop a request.
- SeerrNG's two friction points are addressed **at the schema level rather than the adapter
  level**: an audiobook is an `edition` of a `book` work, routed to a different instance than the
  ebook, and provider matching is the identity cascade.
  **Revision 2 makes that real rather than asserted.** The schema previously said both things at
  once — `'audiobook'` was a member of `work.kind` *and* was described as an edition — which is
  unimplementable in either direction: as a kind it could never be matched to its own ebook (the
  cascade forbids cross-kind matching), and as an edition the kind enum and the `type:` tag
  vocabulary were wrong. **The edition reading wins** (it matches Open Library, which ADR-0009
  already cites as authority) and it propagates: `'audiobook'` leaves `work.kind`;
  `edition.format` gains it; `request` carries `(work_kind, edition_format)`; the tag vocabulary
  gains a `format:` namespace and loses `type:audiobook`; and `Caps.MediaKinds` becomes a list of
  `(kind, format)` pairs.
- Approval, quotas and auto-approve compose with ADR-0011's permissions and ADR-0019's
  single-user mode.
- **"Request in under 3 seconds from a search box" is the design target.** Keep the request API
  clean enough that a Discord/Matrix bot is a 200-line external program — but **do not build the
  bot** in-tree.

### Alternatives rejected
- **Requests as a v2 add-on** — would mean designing search, availability and routing without the
  consumer that most constrains them, and rebuilding all three later.
- **Separate "library" and "discover" surfaces** (the Seerr shape) — the owner explicitly wants
  one place. Two surfaces also make the cross-media case worse, because the film would be in one
  and the novella in the other.

---

<a id="adr-0021"></a>
## ADR-0021 — Northbound stable IDs address `service_item_link`, not `work`

**Status:** Accepted

### Context
Aggregating N backends behind one OpenSubsonic/OPDS endpoint means backend item IDs collide —
Navidrome-A's album `3f2a…` and Navidrome-B's `3f2a…` are different albums. Worse, **clients
cache these IDs indefinitely**, in playlists, favourites and offline downloads. An ID that
changes on re-sync silently corrupts user data, and the user blames UsArr.

### Decision (amended, revision 2)
```
usarr_id := crockford_base32( varint(instance_id) || kind_byte || enc_byte || native_id_bytes )
```
**It addresses a `service_item_link`, not a `work`.**

### Consequences
- Globally unique (instance prefix, and `service_instance.id` is **never reused**, even after
  deletion — deleted instances leave a tombstone).
- **The key insight: addressing a `work` would be unstable for reasons entirely internal to
  UsArr.** A canonical work can be *merged* by the identity cascade or *split* by an un-merge,
  changing its id — and **merges must not be able to corrupt a client's playlist.**
- ⚠️ **Open question:** whether every target client tolerates non-UUID, non-numeric IDs in all
  fields (`id`, `parent`, `coverArt`, `albumId`, `artistId`), and whether any imposes a length
  limit. Symfonium is the reference; **test before the surface ships** — the scheme is unchangeable
  once clients cache ids.

### Revision 2 — four corrections

1. **`kind_byte` added, `0x00` separator removed.** The old encoding carried `(instance_id,
   native_id)` only, but the sole unique index on `service_item_link` is
   `(service_instance_id, remote_kind, remote_id)`. Without the kind at lookup time SQLite can use
   only the leftmost column, so `WHERE service_instance_id=? AND remote_id=?` yields
   `SEARCH … USING INDEX ux_sil (service_instance_id=?)` — a range scan over every link on that
   instance (~400k rows for a 2k-series Sonarr) on **every** stream resolve, `getCoverArt` and
   metadata call. Adding a `(instance, remote_id)` unique index instead is not available: it asserts
   an invariant that is false for the \*Arrs, where series 42 and episode 42 both exist. The `0x00`
   separator is dropped as decodable dead weight — `varint` is self-delimiting.
2. **"Nothing UsArr computes is in it" was false, twice.** `instance_id` is assigned by UsArr, and
   *which* link is addressed for a unified item was driven by an admin-editable `priority` — so
   reordering two Navidromes would have changed every affected album's ID, which is the exact
   corruption this ADR exists to prevent, caused by a settings change with no warning. **The fix is a
   pin:** once a link has been addressed northbound it is `is_northbound_canonical` and priority
   changes do not move it; deleting a pinned instance mints an alias row so old IDs still resolve,
   and never silently rebinds. The true invariant is narrower: **the ID is stable for a fixed
   `(instance, kind, native id)`.**
3. **"Opaque to the client" is struck.** Base32 is an encoding, not a confidentiality mechanism: any
   client decodes a UsArr ID in one line, and \*Arr native ids are small sequential integers, so the
   space is enumerable in a few thousand round trips. **Authorization must never depend on ID
   secrecy** — every resolution performs a `user_library_access` + permission check *before* any
   backend call and returns the protocol-native not-found (Subsonic 70) on failure, never a 403,
   which would confirm existence.
4. **The length requirement is honest now.** Crockford base32 expands 8/5, so a verbatim 32-character
   identifier is 56 characters, over the ~48 target. `enc_byte` lets a hex or UUID identifier be
   decoded to 16 raw bytes and re-hexed on the way out (31 characters), and where an id cannot be
   compacted the document **states the resulting length** rather than pretending the bound holds.
   🔍 The per-backend id formats this depends on are inference and must be checked against live
   instances before the codec is frozen.

**"Resolvable without a database lookup" is also narrowed:** routing decodes locally, but the
gateway must still reach `work` for metadata, so the link lookup is unavoidable — which is precisely
why `kind_byte` matters.

### Alternatives rejected
- **`work.id` directly** — unstable across merges, which is the one thing an eternally-cached ID
  must not be.
- **A random surrogate key per exposed item** — requires a lookup on every request and a mapping
  table that must itself never lose a row.
- **The backend's native ID unprefixed** — collides the moment a second instance of the same kind
  is added, which is the exact topology the gateway exists to serve.

---

<a id="adr-0022"></a>
## ADR-0022 — v1 authentication is a local account and API keys; external identity is deferred

**Status:** Accepted (revision 2)

### Context
The design carried a full enterprise-adjacent authentication programme: OIDC with PKCE against three
named providers (with opaque-token introspection handling for Authelia), forward-auth with trusted
header plumbing and CIDR allowlists, WebAuthn passkeys issuing discoverable credentials, TOTP, and a
visible "security posture panel". None of it serves the stated deployment — a tailnet, a household,
one owner — and every item is a subsystem rather than a setting.

Note carefully what is **not** in scope of this ADR: credential encryption at rest and the SSRF
egress policy are correctly identified as applying on a tailnet exactly as on the internet, and they
are **v0.1 deliverables in full**. The problem is only the exposure apparatus stacked on top.

### Decision
> **v1 authentication is: a local account with Argon2id, an opaque server-side session cookie, a
> CSRF token on state-changing requests, and per-app API keys for the northbound surfaces. Nothing
> else.**

Three specifics that are decisions rather than defaults:

1. **Argon2id is for user passwords only.** Northbound `client_credential` keys are server-generated
   ≥128-bit random tokens verified with **HMAC-SHA256 and a constant-time compare**, looked up by
   `key_prefix`. Running a memory-hard KDF on a high-entropy bearer token buys nothing and costs
   everything: Subsonic clients authenticate *per request*, one poster grid is ~60 `getCoverArt`
   calls, and at m = 19 MiB that is 1.1 GB of transient allocation on a Pi — plus an unauthenticated
   OOM primitive, since verification runs before the key is known to be valid. **The reasoning is
   recorded so a future contributor does not "fix" it back.**
2. **There is no password pepper.** It appeared twice in prose and was specified nowhere — no env
   var, no storage location, no rotation procedure — and a pepper silently absent on one deploy and
   present on another locks users out. Per-hash salts are the design.
3. **Sudo mode exists in v1**: a 5-minute re-authentication window before touching the vault (adding
   or changing a service credential, changing a `base_url`, downloading a backup, issuing a
   credential, rotating the key). Without it a single stolen cookie is a month-long window.

### The seam
**Credential verification is isolated behind one `Authenticator` interface from v0.1**, and v0.1
already ships three implementations — password, HMAC'd API key, and the tailnet `WhoIs` path — so a
fourth identity source is an implementation rather than a rewrite. The interface carries a
`Surfaces()` method (`web | rest | opds`), which is the load-bearing half: it makes the rule that
**ambient and trusted-header auth never reach the OpenSubsonic or OPDS surfaces** structural rather
than something to remember. `user.auth_source` already takes another value without a migration.

**Revisit trigger:** a user who actually runs an SSO stack asks for it — and, for passkeys
specifically, a decision about what TV and native clients do instead, since WebAuthn is web-only.

### Consequences
- The v1 config surface loses `USARR_FORWARD_AUTH_*` and the worked forward-auth example.
- The audit log ships as a **plain paginated list**; the filtered audit UI and the security-posture
  panel are deferred ([`FUTURE.md`](./FUTURE.md) §10). Rate limiting and the audit log itself stay —
  they are cheap and *"who deleted this"* is a real need on a shared server.
- **In v0.1 there is exactly one account**, so a tailnet identity path that matches the allowlist
  authenticates **as the owner** and any other login is refused. UsArr **never auto-creates** in
  single-user mode, and an enabled identity path with an **empty allowlist is a startup error** —
  fail closed, not open.

---

<a id="adr-0023"></a>
## ADR-0023 — UsArr coexists with the ecosystem rather than replacing it

**Status:** Accepted (revision 2) · **This is a framing decision, and it settles a class of future
arguments.**

### Context
Every aggregation layer faces the same drift: it starts as a view over other people's services, then
absorbs one of them "because it would be simpler if it were all one binary". The design already
resists this in specific places — no transcoder, no player, no reimplementation of the \*Arr import
engines — but the *general* principle was never stated, so each instance had to be re-argued from
first principles.

### Decision
> **UsArr is the layer that makes your existing services one catalogue. Sonarr keeps doing
> acquisition, Jellyfin keeps doing playback, Navidrome keeps being an excellent music server.**

Two rules follow, and they are the useful part:

1. **A feature that moves UsArr toward *replacing* a neighbour is a non-goal**, not a deferral, and
   belongs in the "explicitly never" list rather than in [`FUTURE.md`](./FUTURE.md). Transcoding and
   an in-app player are the worked examples.
2. **A feature that makes the neighbours easier to live with is on-thesis** — the unified catalogue,
   cross-media links, one credential, one search box, aggregated release calendars, unified
   statistics. These belong in the roadmap or in `FUTURE.md` with a seam.

### Consequences
- The `FUTURE.md` / "explicitly never" split has a test, so an idea can be sorted without a debate
  about whether it is "cool".
- It explains a decision that otherwise looks inconsistent: UsArr **proxies audio bytes** for its own
  OpenSubsonic surface (there is no neighbour to hand a Subsonic client off to) but **links out for
  video** (there is — the backend's own client, which is better at it).
- It is the honest answer to "why not just fork Jellyfin": UsArr is not trying to be a better
  Jellyfin, it is trying to make five services feel like one library.
- **Users must run a second service**, and that remains the real objection to the whole approach.
  The mitigation is a good wizard, a first-class compose bundle and honest documentation — not a
  rebuttal, and not absorbing the neighbour.

---

<a id="adr-0024"></a>
## ADR-0024 — Styling and typography: Tailwind v4 with the default theme deleted, Bits UI, Tabler, self-hosted IBM Plex

**Status:** Accepted · **Depends on ADR-0003**, which fixed the framework and said nothing about the
styling layer. Full rationale, with citations, in
[`design/DESIGN-DIRECTION.md`](./design/DESIGN-DIRECTION.md); the canonical values are in
[`design/tokens.css`](./design/tokens.css).

### Context
ADR-0003 chose SvelteKit `adapter-static` embedded via `embed.FS`. It left the CSS approach, the
component library, the icon set and the typeface entirely open — as of this ADR there is no
styling decision anywhere in the repository and no frontend code to constrain one.

ARCHITECTURE §17.1 sets the brief: utilitarian over stylish, density and speed over animation,
standard patterns over novel ones, native controls, no flair that costs render time, Navidrome as
the bar. The owner separately asked for two things that pull in opposite directions: an
**established framework rather than hand-rolled CSS**, and a result that does **not** look like
generic generated output.

**The risk actually being managed is not "will it look good".** It is *"will a solo maintainer keep
it consistent across years"*. The ecosystem supplies the cautionary evidence: Portainer ships
Angular 1.8 **and** React 17 with Bootstrap 3 **and** Tailwind 3 in one manifest, which is what a
half-finished migration looks like in a dependency graph; Jellyfin's multi-year rewrite left the
app internally inconsistent for years. Design *drift* — each screen slightly different because
there is no token file — and design *sameness* — every screen identical because the tool's
defaults shipped untouched — have the same root cause: no decision was made and written down.

One further constraint is specific to this repo: `make check` is the pre-commit gate and makes
**exactly one network call** (govulncheck). Anything that adds network dependence to the build is
expensive here in a way it would not be elsewhere.

### Decision

**1. Tailwind CSS v4 (4.3.x) via `@tailwindcss/vite`, with the default theme deleted.**

```css
@import "tailwindcss";
@import "./tokens.css";
@theme { --*: initial; }
```

`--*: initial` is **mandatory and load-bearing**, not a tidy-up. Tailwind's own theme documentation
states that after a namespace reset "all of the default utilities that use that namespace *(like
`bg-red-500`)* will be removed, and only your custom values *(like `bg-midnight`)* will be
available" (<https://tailwindcss.com/docs/theme>). With the whole theme reset, **`bg-indigo-500`,
`shadow-md` and `rounded-2xl` do not exist as classes at all.** The generic look becomes
*structurally impossible* rather than a code-review convention, and the one rule density actually
depends on — every value is a multiple of one base unit — becomes tooling-enforced rather than
remembered. Arbitrary-value syntax (`[13px]`) becomes a greppable lint target.

Tailwind is a **build-time Vite plugin** that emits a plain `.css` file. Nothing Node-related
survives into the build output, so ADR-0003's "no Node process in production, ever" is untouched.

**2. Bits UI (2.18.x) for headless behaviour — and only where there is no native element.**
§17.1 requires native `<select>`, native checkboxes and radios, native focus rings and real form
submits, and forbids a bespoke dropdown that breaks keyboard navigation. Bits UI is therefore
scoped to dialog, popover/menu, tooltip and tabs — the cases where correct focus management,
escape/outside-click handling and ARIA wiring are genuinely hard and hand-rolling them is how
keyboard support breaks. It imposes no visual opinion, and its runtime dependencies (`runed`,
`esm-env`, `tabbable`, `svelte-toolbelt`, `@floating-ui/dom`) are exactly the focus-management and
positioning problems in question.

**3. Tabler icons, inlined and subsetted at build time via `unplugin-icons`, behind an explicit
import allowlist.** `unplugin-icons` resolves icons from Iconify at build time and inlines them as
components: no runtime library, no icon font, no extra request, and only imported glyphs are
bundled. Tabler (MIT, ~5,900 glyphs) is aimed at data-dense admin interfaces and is
distinguishable from the Lucide/Heroicons house style without being loud — and Lucide is itself
named as a tell, being "baked into templates, starter kits, and AI-generated components by
default". Icons render at 16px in rows and 20px in headers, in `currentColor`. **No emoji anywhere
in the chrome**, which is a correctness rule (per-platform rendering, screen-reader behaviour) as
well as a taste one.

**4. Self-hosted IBM Plex Sans (UI) and IBM Plex Mono (machine data only), SIL OFL 1.1, subset to
Latin, served from `embed.FS`, with the system stack as the fallback in the same `font-family`
declaration.** Mono is semantic — file paths, indexer names, sizes, hashes, log lines, verbatim
upstream error text — never decorative. **Never a Google Fonts `<link>`:** it is both a documented
tell and a third-party request in software whose premise is that it runs on your own hardware.

**5. All design values live in one token file**, `docs/design/tokens.css`, mapped into Tailwind
through `@theme inline`. No literal colour, size, radius or duration in any component.

**6. Serving, per ADR-0003's embedding story** — four traps that are cheap now and expensive later:

- **`//go:embed all:dist`, never `//go:embed dist`.** SvelteKit's `appDir` defaults to `_app`, and
  Go's `embed` excludes files whose names begin with `.` or `_` (<https://pkg.go.dev/embed>). Plain
  `//go:embed dist` silently drops the entire application and embeds the favicon. The `all:` prefix
  "changes [the rule] to include those files beginning with '.' or '_'".
- **`precompress: true` needs a handler that honours it.** `adapter-static` can emit `.br`/`.gz`,
  but Go's stock `http.FileServer` will not serve them. Use `statigz`
  (<https://github.com/vearutop/statigz>). Brotli on the JS bundle is the single largest first-paint
  lever available, and because compression happens at build time it costs zero CPU per request —
  which matters on the Pi-class hardware §13 targets.
- **`paths.base` is baked in at build time**, so a single binary cannot serve itself under an
  arbitrary reverse-proxy subpath chosen at runtime. Build with `base: ''`, document that UsArr
  must be proxied at a domain or subdomain root, and treat subpath support as a separate decision
  if it is ever demanded.
- **🔍 `paths.relative` defaults to `true` and likely breaks assets under a deep-route SPA
  fallback.** With one fallback document served at every depth (`/library/movies/12345`), relative
  asset URLs would resolve against the wrong directory, so `paths.relative: false` is the expected
  setting. **This is inference from the documented semantics, not a cited statement — it must be
  tested empirically before the build config is frozen.**

### Consequences
- **The generic look is unavailable rather than discouraged.** A contributor cannot reach for
  `rounded-2xl` because it is not generated. This is the whole point of the decision.
- **Node and a native binary enter the *build*, never the runtime.** The lockfile must be committed
  and `DEVELOPMENT.md` must document the offline path.
- **CSS ships small.** Tailwind's v4 material puts a representative purged production build at
  ~18 KB uncompressed; with a deliberately reduced token set, 🔍 well under 10 KB brotli is the
  expectation. CSS is not where this app's bytes go — the JS bundle and the fonts are.
- **The fonts are the real byte cost.** Two subset WOFF2 families at three faces is 🔍 an estimated
  ~120–180 KB, **unmeasured**, and it must be measured before the fonts are committed. If it lands
  materially above ~200 KB, drop a weight before dropping the family; if it still does not fit, the
  zero-webfont fallback below is the answer.
- **Native controls stay native**, so the surface Bits UI covers is small — which also keeps the
  dependency footprint small.
- **Both themes are audited independently.** Every contrast ratio in `tokens.css` is computed and
  recorded next to the pair it was measured for. A `dark:` prefix per element is not how this
  works.
- **The seam is the token file.** Tailwind consumes it; scoped Svelte `<style>` blocks read the
  same variables. Swapping the utility engine later touches the token file and the class
  attributes, **not the component logic**.

### The strongest argument against this decision, recorded rather than dismissed
Tailwind v4's engine, **`@tailwindcss/oxide`, is a native Rust binary distributed as per-platform
optional dependencies with a postinstall script that fetches the right one**, plus a WASM fallback.
There is a documented class of CI failures where npm skips the platform-specific optional
dependency (<https://github.com/tailwindlabs/tailwindcss/pull/17929>; discussions
[#18427](https://github.com/tailwindlabs/tailwindcss/discussions/18427),
[#15254](https://github.com/tailwindlabs/tailwindcss/discussions/15254)). **In a project whose
stated quality gate makes exactly one network call, a native-binary-fetching postinstall is a new
class of build fragility.** The counter-position is respectable and is not being waved away.

**The pre-agreed fallback is Open Props** — plain CSS custom properties, no engine, no postinstall,
no native binary. **Trigger condition: if the `@tailwindcss/oxide` install fails on a supported
build platform, or blocks an offline build, in a way that a committed lockfile does not fix.** The
migration cost is bounded because both approaches express the same tokens: `tokens.css` is already
plain CSS and would be consumed directly rather than through `@theme inline`; what changes is the
class attributes, not the components. Open Props supplies tokens **without enforcement**, and
enforcement is the reason Tailwind is chosen — so this is a fallback, not a preference.

### Alternatives considered
- **Open Props** (v1.7.x) — **the defensible runner-up and the recorded fallback**, per the trigger
  above. Rejected as the primary only because it gives tokens without making the wrong value
  impossible to type.
- **The system font stack with no webfont** — the recorded typographic alternative. Zero bytes,
  zero FOUT, zero layout shift, native feel. Rejected as the default because San Francisco, Segoe
  UI and Roboto have different x-heights and advance widths, so **the layout is not reproducible
  across platforms** — a title that fits a 32px row on macOS may wrap on Windows — and MDN warns
  that `system-ui` "may cause the displayed typeface to be undesirable for some users"
  (<https://developer.mozilla.org/en-US/docs/Web/CSS/font-family>). For a design built on fixed row
  heights that is not cosmetic. It remains the answer if the font budget in OQ-3 fails.
- **Inter, and its substitutes** — Inter is now the most-named typographic tell of generated UI,
  and Geist, Space Grotesk, Instrument Serif and Poppins are named as its equally-recognisable
  replacements. The fair counter-argument is that the objection is about *identity, not
  legibility*: Inter would work fine. It is simply the one choice guaranteed to read as defaulted.
- **shadcn-svelte** (1.5.x) — a CLI that copies component source into the repo, generating Bits UI
  + Tailwind components. Genuinely interesting: with `--*: initial` it is a source-code starting
  point rather than a design language. Rejected because vendored components are yours to maintain
  and drift, and because UsArr needs a handful of primitives, not a component set.
- **Flowbite Svelte** (1.33.x) — **disqualified on its dependency graph**: it declares
  **`apexcharts`** and `date-fns` as *runtime* dependencies. A charting library in the dependency
  graph of a media browser is exactly the dead weight ADR-0003 rejected SSR over. It also imposes
  a strong, recognisable look.
- **Skeleton v5** — credible and well-engineered (Zag.js state machines are a serious foundation),
  but 23 runtime packages and a plugin-driven theme system is a bigger commitment than five screens
  need, and it imposes a look.
- **Melt** — architecturally equal to Bits UI, but it declares **`jest-axe` as a runtime
  dependency**. 🔍 Almost certainly a packaging mistake rather than something that reaches the
  browser, but it is a smell, and Bits UI has no equivalent.
- **Svelte Material UI** — wrong on premise. Material is a design language with animation and
  elevation baked in, which contradicts §17.1 directly.
- **Pico.css, Water.css, Bulma** — genuinely charming, very fast to start, and **tuned for
  comfortable reading density**: generous spacing, wide default typography. That is the exact
  opposite of §17.1's "density is a feature", and the result would be fought the whole way.
- **Bootstrap 5** — mature and enormously documented, which is a real advantage for drive-by
  contributors. Rejected because it brings its own JS for interactive components (duplicating what
  Bits UI does better in Svelte) and because its look is arguably *more* dated-generic than
  Tailwind's.
- **PatternFly** — right in spirit, genuinely built for dense admin consoles, but React-first with
  the plain-CSS distribution a second-class citizen, and a strong Red Hat visual identity.
- **Hand-rolled CSS with a custom token layer** — technically the cleanest: zero build dependency,
  smallest output, total control. Rejected because it is precisely what the owner said he did not
  want, and because it puts all naming and consistency discipline on review rather than on tooling.
  🔍 For a solo maintainer that is where CSS rot starts.
- **Hugo** — **rejected for the application, and the reason is requirements rather than taste.**
  Hugo renders content through Go templates at build time; it has no client-state model, no
  reactivity and no component runtime, so a 10k-item filterable grid — ADR-0003's stated
  requirement — is not something it addresses at any level. Using it would mean hand-writing all
  the interactivity in vanilla JS, which is strictly worse than what SvelteKit already provides.
  **The instinct behind the suggestion is sound and is kept alive:** Hugo is a first-rate fit for a
  future project documentation site — a single Go binary matching the toolchain, and a `docs/` tree
  already in Markdown that it could render essentially as-is. **That is its own small ADR if a docs
  site is ever wanted, and is explicitly not part of this one.**
