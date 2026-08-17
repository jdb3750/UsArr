# UsArr — Decision Log (ADRs)

Lightweight architecture decision records. One entry per significant, hard-to-reverse choice.

**Status values:** `Accepted` · `Superseded by ADR-NNNN` · `Proposed` · `Deprecated`.
Evidence for every claim below lives in [`RESEARCH.md`](./RESEARCH.md); the design that follows
from it lives in [`ARCHITECTURE.md`](./ARCHITECTURE.md).

**All ADRs dated 2026-08-16 unless noted.** These were taken before the first line of code, on the
basis of five research tracks. **Most remain unimplemented**; the first slice of code has since
landed, and `ARCHITECTURE.md` §16 is authoritative for which parts of it exist.

**Revision 2 (2026-08-16, after a three-way adversarial review).** Seven ADRs changed. Three
distinctions now matter and are used consistently below:

- **Accepted** — the decision stands.
- **Deferred** — the idea is sound and is *not* being built now. Every deferred ADR names **the
  seam** in the v0.1 design that keeps it cheap to add later, and **a revisit trigger**. The full
  set lives in [`FUTURE.md`](./FUTURE.md). A deferral is not a rejection, and it must not be
  restated as one.
- **Superseded / Reversed** — the decision was wrong, or its evidence was. The reversal records what
  changed and why, rather than silently rewriting the original.

### How an ADR is amended when the world moves under it

**An ADR body is a dated record of a decision as taken, so it is annotated, never rewritten.** The
original text stays standing — struck with `~~` where it is plainly wrong, left as-is where it is
merely overtaken — and the correction is *added* beside it. **Three marks are always owed**, and a
fourth where it is needed:

1. **The index row above** gains `⚠️ amended <date> by ADR-NNNN`, with one line on what moved.
2. **The `Status:` line** gains the same flag, so a reader arriving at the anchor is warned before
   the first section rather than after it.
3. **A dated block directly under the `Status:` line** — a `### ⚠️ Amendment, <date>` section or a
   `> ⚠️ **AMENDED …**` blockquote — naming *which* claims below no longer hold and which survive.
4. **The falsified sentence itself** keeps a dated inline flag wherever a reader could otherwise
   take it as live, since anchors and search land people mid-document.

**The decision lives in the superseding ADR; the amendment note points at it and does not re-argue
it.** This is the rule [`DEVELOPMENT.md`](./DEVELOPMENT.md) §11 already states for `REVIEW-LOG.md` —
*"a citation inside a dated record is history, not staleness"* — and it is deliberately the opposite
of how the design documents are handled: [`ARCHITECTURE.md`](./ARCHITECTURE.md), `README.md` and
`SETUP-CHECKLIST.md` describe the design **as it stands now**, so they are corrected in place, quoting
what they used to say. An ADR records what was decided **then**.

⚠️ **Where this was not followed it cost something, which is why it is written down.** `162dca5`
rewrote ADR-0035's title, its §1 heading and its §1 prose in place instead of only annotating them.
The sentence that rewrite introduced — *"Not in v0.1, which draws no comics or books library at
all"* — was falsified by [ADR-0041](#adr-0041) the next day, and it traces to **no ADR at all**,
because no ADR ever decided it. Annotating leaves that failure mode nowhere to hide.

| ADR | Decision | Status |
|---|---|---|
| [0001](#adr-0001) | Go for the backend | **Accepted, evidence corrected** (rev 2) |
| [0002](#adr-0002) | Embedded SQLite + WAL, no required sidecar | Accepted, one claim narrowed |
| [0003](#adr-0003) | SvelteKit `adapter-static` SPA embedded via `embed.FS` | Accepted — **one argument corrected by [ADR-0029](#adr-0029)** |
| [0004](#adr-0004) | Replica, not proxy | Accepted — **refined by [ADR-0026](#adr-0026)**; the conflict rule is narrowed to three axes in ARCHITECTURE §2.2 |
| [0005](#adr-0005) | Delegate playback to media servers | Accepted |
| [0006](#adr-0006) | Never build a video transcoder | Accepted · permanent |
| [0007](#adr-0007) | Wikidata as the cross-media spine, shipped as a prebuilt CC0 subset | **Amended** (rev 2): edges-only artifact, per-release |
| [0008](#adr-0008) | **Two** plugin tiers now; a WASM tier deferred | **Amended** (rev 2) |
| [0009](#adr-0009) | `work` / `edition` / `file` with typed `work_relation` edges | Accepted, `audiobook` resolved — **refined by [ADR-0030](#adr-0030), [ADR-0031](#adr-0031), [ADR-0033](#adr-0033)** |
| [0010](#adr-0010) | OpenSubsonic and OPDS as northbound server surfaces | Accepted, scope narrowed |
| [0011](#adr-0011) | Named permission strings, not a bitfield | Accepted |
| [0012](#adr-0012) | Sync channels and the write path | **Superseded in part by ADR-0012a** |
| [0012a](#adr-0012a) | A durable command queue replaces the optimistic intent log | **Accepted** (rev 2); ⚠️ **amended 2026-08-17 by [ADR-0042](#adr-0042)** — the design is untouched, but the queue's **first writer re-sequences out of v0.1** with the \*Arr adapters, so this ADR's *"every v0.1 write"* names writes no longer in v0.1 |
| [0013](#adr-0013) | **Two-tier** search; an external engine deferred | **Amended** (rev 2) |
| [0014](#adr-0014) | `service_item_link` is many-to-many | Accepted, framing demoted — **extended by [ADR-0026](#adr-0026)** |
| [0015](#adr-0015) | Namespaced tags with virtual parents | Accepted |
| [0016](#adr-0016) | Tailnet as the default deployment assumption | Accepted |
| [0017](#adr-0017) | ~~302 redirect for streams~~ → **proxy for audio/ebooks, link out for video** | **Reversed** (rev 2) |
| [0018](#adr-0018) | No in-app player; UsArr is not a byte-delivery product | Accepted, one clause corrected |
| [0019](#adr-0019) | Single-user first, multi-user schema from migration 0001 | Accepted, list reconciled |
| [0020](#adr-0020) | Requests are a first-class pillar | Accepted |
| [0021](#adr-0021) | Stable IDs address `service_item_link`, not `work` | **Amended** (rev 2): kind byte, pin, no opacity claim |
| [0022](#adr-0022) | v1 authentication is local-only; external identity deferred | **Accepted** (rev 2) |
| [0023](#adr-0023) | UsArr coexists with the ecosystem rather than replacing it | **Accepted** (rev 2) |
| [0024](#adr-0024) | AGPL-3.0 is the licence | **Accepted** — owner-confirmed 2026-08-16 |
| [0025](#adr-0025) | Styling and typography: Tailwind v4 with the default theme deleted, Bits UI, Tabler, self-hosted IBM Plex | **Accepted** — ⚠️ **amended 2026-08-16**: **Tailwind is not used**; the styling layer is hand-written CSS in `web/src/app.css`. Bits UI, Tabler and IBM Plex are unaffected |
| [0026](#adr-0026) | A library is a user-owned binding to upstream containers, with a correction layer | **Accepted** — refines ADR-0004, extends ADR-0014; ⚠️ **amended 2026-08-17 by [ADR-0043](#adr-0043)** — the binding, the four verbs and the storage are untouched, but this ADR's consequence capping the correction **UI** at v0.3 rested on *"§6.4 already establishes that tier 1 resolves essentially 100% of the v0.1 identity problem"*, which [ADR-0041](#adr-0041) falsified for v0.1's actual source: **the minimal *"fix this match"* case moves earlier**, and the full four-verb surface stays at v0.3 |
| [0027](#adr-0027) | Two axes: media type is navigation, a library is scope | **Accepted** — settles §17.2's open question |
| [0028](#adr-0028) | Home is three fixed blocks, not one strip per media type | **Accepted** — **amends** ARCHITECTURE §17.2 |
| [0029](#adr-0029) | "Load more" + `content-visibility`; virtualization is a benchmarked escalation | **Accepted** — **amends** §4.5, corrects an argument in ADR-0003; ⚠️ **amended 2026-08-16** — the required benchmark ran against the shipped primitive: the decision is unchanged, correction (c)'s arithmetic, the mitigation ranking and the row-ceiling extrapolation are corrected, the page size is **200 rows**, and two benchmark gaps are recorded; ⚠️ **amended 2026-08-17** — the 100 ms every row ceiling divided by was Tier 0's and was never this ADR's to borrow: the toggles are governed by `DESIGN-DIRECTION.md` §7.2's new **Controls** budget at **400 ms**, so every ceiling scales 4×, the worst-case residual-risk note is weakened rather than withdrawn, the shipped 200-row page gains a measured cost, and the page size is still **200 rows** |
| [0030](#adr-0030) | `work.kind` gains `comic_issue`; manga is not a separate kind | **Accepted** — refines ADR-0009 |
| [0031](#adr-0031) | Track position is edition-scoped; attribution is many-to-many | **Accepted** — refines ADR-0009 |
| [0032](#adr-0032) | Read-only catalogue sources move early; command sinks defer | **Accepted** — **amends** §16; **one member reversed by [ADR-0035](#adr-0035)** |
| [0033](#adr-0033) | `work.kind` gains `person`; a credit is not a music artist | **Accepted** — owner-decided 2026-08-16; refines ADR-0009, ADR-0031 |
| [0034](#adr-0034) | The project keeps the name UsArr | **Accepted** — owner-decided 2026-08-16; naming only, nothing in the codebase moves |
| [0035](#adr-0035) | Kavita, not Komga, is the comics-and-books catalogue source | **Accepted** — owner-decided 2026-08-16; **reverses one member of [ADR-0032](#adr-0032)**, confirms [ADR-0030](#adr-0030); ⚠️ **amended 2026-08-16** — the catalogue sources sequence **after** v0.1 ([ADR-0036](#adr-0036)), so this ADR picks *which* source, and its spike orders the post-v0.1 sequence; ✅ **§2's spike RAN 2026-08-17 and PASSED** — dated result in §2a, with one qualification (no server-side since-filter exists); ⚠️ **amended 2026-08-17 by [ADR-0041](#adr-0041)** — that spike result moved **Kavita into v0.1**, so the 2026-08-16 amendment below and §1's *"not in v0.1, which draws no comics or books library at all"* rider are both falsified. The choice of Kavita over Komga is untouched |
| [0036](#adr-0036) | No catalogue source ships in v0.1; they arrive one at a time after it | **Accepted** — owner-decided 2026-08-16; **amends** §16; **re-sequences [ADR-0032](#adr-0032) and [ADR-0035](#adr-0035)** without rejecting any source; ⚠️ **amended 2026-08-17 by [ADR-0042](#adr-0042)** — its libraries bullet justified the subsystem partly on *"the request destination v0.1's write path routes on"*, and **that write path re-sequences out of v0.1**; the subsystem stays on its other grounds; ⚠️ **amended 2026-08-17 by [ADR-0041](#adr-0041)** — the owner runs neither Sonarr nor Radarr, so this ADR's *"prove the replica thesis on real data"* criterion was unmeetable as scoped: **Kavita ships in v0.1 as the sync core's first adapter** and the \*Arr adapters re-sequence behind it. The rule — one source, proven on real data, before a second adapter — is kept unchanged |
| [0037](#adr-0037) | TOFU SPKI pin enrolment is removed, not completed; enforcement stays | **Accepted** — 2026-08-16; amends no ADR; reopening conditions stated (a pin field on the update path + the change-acceptance UI) |
| [0038](#adr-0038) | A list freezes its order while a user is aiming at it | **Accepted** — 2026-08-16; amends no ADR; the argument lives in `design/DESIGN-DIRECTION.md` §9.1a and ARCHITECTURE §17.5, this record holds the rejected alternatives |
| [0039](#adr-0039) | `write_queue.state` loses its `CHECK`; `work_id` gets its foreign key back | **Accepted** — 2026-08-17; **supersedes** `reference/schema.md` §10 step 1 and the seam in `FUTURE.md` §11 / §11.1; closes `REVIEW-LOG.md` WQ-05; ⚠️ **amended 2026-08-17** — decision 3's ground 1 is **struck**, on a misquotation of `reference/sync.md` §4 that dropped the words *toward the \*Arr*: the decision stands on grounds 2 and 3, which are independent of it; ⚠️ **corrected 2026-08-17** — decision 1 and the first rejected alternative wrote the Go `state` validation as **done**; it is **owed by the first `write_queue` writer** and nothing validates the vocabulary today (`REVIEW-LOG.md` M5-25) |
| [0040](#adr-0040) | The six subtype tables land with the catalogue source that writes each | **Accepted** — 2026-08-17; records as a decision what `00005_library_sync.sql` did; **in tension with** ARCHITECTURE §16's enumerated v0.1 schema line, which is left to the thread that owns §16; ⚠️ **amended 2026-08-17 by [ADR-0044](#adr-0044)** — the RULE is confirmed and **applied**, not overridden, and one table moves under it: `work_credit` lands with **Kavita** rather than Navidrome, because Kavita is the source that writes credits. `work_album` and `work_track` are unaffected and this ADR's decision clause 1 is otherwise untouched |
| [0041](#adr-0041) | The sync core ships with **Kavita** as its first adapter; Sonarr and Radarr re-sequence behind it | **Accepted** — owner-decided 2026-08-17; **amends [ADR-0036](#adr-0036)** (*"No catalogue source ships in v0.1"*) and **amends** ARCHITECTURE §16, whose replacement text is proposed here and routed to the thread that owns §16; **re-sequences, rejects nothing** — Sonarr and Radarr still arrive; confirms [ADR-0035](#adr-0035) and [ADR-0040](#adr-0040); ⚠️ **amended 2026-08-17 by [ADR-0042](#adr-0042)** — the write-path question this ADR flagged as *"NOT decided here"* is now answered: the minimal write path **re-sequences with the \*Arr adapters**, so its consequence bullet and the ⚠️ clause in its proposed §16 text are both settled |
| [0042](#adr-0042) | v0.1's minimal write path re-sequences with the \*Arr adapters; Sonarr and Radarr stay on the roadmap | **Accepted** — owner-decided 2026-08-17; **answers the question [ADR-0041](#adr-0041) flagged and refused**; **amends** ARCHITECTURE §16, [ADR-0041](#adr-0041), [ADR-0036](#adr-0036) and [ADR-0012a](#adr-0012a); **re-sequences, rejects nothing** — [ADR-0012a](#adr-0012a)'s queue design is untouched, the seam costs **no migration**, and Sonarr and Radarr stay on the roadmap at the owner's explicit condition; raises one open question it does not close (neither \*Arr has a milestone); ⚠️ **amended 2026-08-17 by [ADR-0045](#adr-0045)** — that open question is **closed**: the owner delegated the call and **Sonarr, Radarr and this write path all land in v0.2**. The decision, the measurement and the seam are untouched; only *"this ADR does not assign them a milestone"* (clause 5), alternative (e) and the open question itself are overtaken |
| [0043](#adr-0043) | A **minimal** match-correction UI moves earlier than v0.3; the full correction surface stays there | **Accepted** — owner-decided 2026-08-17; **answers the scope call ARCHITECTURE §6.4 flagged and routed**; **amends [ADR-0026](#adr-0026)**'s correction-UI consequence and **amends** ARCHITECTURE §16.0, §16.1's v0.1 and v0.3 entries; **re-sequences, rejects nothing** — ADR-0026's model, verbs, tables and scoping are untouched and `library_override` is already in the tree; **source-independent** — it turns on v0.1 having a weak-identity catalogue source, not on that source being Kavita; **assigns no milestone**, which is carried as an open question; ⚠️ **amended 2026-08-17 by [ADR-0045](#adr-0045)** — that open question is **closed**: the owner delegated the call and the minimal case lands in **v0.2**, chosen **by elimination** (the only slot both earlier than v0.3 and not v0.1) with its cost recorded — v0.1 ships the *"not identified"* badge without its remedy for one milestone. The *"minimal"* boundary, the source-independence and the v0.3 cap on the full surface are all untouched |
| [0044](#adr-0044) | Author and creator credits ship in v0.1; `work_credit` lands with **Kavita**, not Navidrome | **Accepted** — owner-decided 2026-08-17; **applies [ADR-0040](#adr-0040)'s rule rather than overriding it** — the landing point is the source that writes the table, and Kavita writes credits; **amends [ADR-0040](#adr-0040)** for `work_credit` **only** — `work_album` and `work_track` still wait for Navidrome; **confirms [ADR-0033](#adr-0033)**, whose `person` kind finally gets a writer; **costs a ROW, not a column** — an author is a `work` of kind `person`, which nothing in v0.1 created before; leaves [ADR-0035](#adr-0035), [ADR-0041](#adr-0041) and [ADR-0042](#adr-0042) untouched; raises one open question it does not close (nothing collects an uncredited person) |
| [0045](#adr-0045) | The three unslotted commitments land in **v0.2**: Sonarr and Radarr, the minimal write path, and the minimal match-correction UI | **Accepted** — **owner-delegated 2026-08-17** (*"whatever you think is best"*); **closes the open questions [ADR-0042](#adr-0042) and [ADR-0043](#adr-0043) each raised and refused**; **amends** ARCHITECTURE §16 (§16.0, §16.1, and the v0.1, **v0.2** and v0.3 entries) and **amends [ADR-0042](#adr-0042)** and **[ADR-0043](#adr-0043)** at their no-milestone clauses; **assigns milestones and nothing else** — no scope moves, no design reopens, no ADR is reversed; for the two \*Arr items this **writes down a dependency v0.2 already had** (§8.3's `Add` capability filter), for the correction UI it is **by elimination** and the cost is recorded rather than argued away; raises one open question it does not close (whether v0.2 should be split) |
| [0046](#adr-0046) | Kavita's contract tests pin **TWO** specs: the release the owner runs is the **floor**, `develop` is the **ceiling** | **Accepted** — 2026-08-17; **implements a policy `api/specs/SOURCES.md` already stated and the tree had not acted on** (*"a green contract test here is evidence about `develop`"*); **changes what a green means, not what the code does** — no adapter field, request or migration changes; **renames** `api/specs/kavita.json` → `kavita-develop.json` and **adds** `kavita-v0.9.0.2.json`; every spec-reading test runs against **both**, named per file, with enum coverage **equal** to the ceiling and a **superset** of the floor; `ceilingOnlyProperties` machine-checks the five modelled properties that **decode to nothing on the owner's server**; the `cbr` external_id is **unreachable** on the stable line and now says so; raises three open questions it does not close (Prowlarr has the same gap, `'cbr'` is an unenumerated `external_id.source`, and the floor's re-pin cadence) — **question 2 settled 2026-08-17** by the rename to `comicbookroundup` (`LS-73`) |

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

**A memory claim in this ADR was also unsupported.** The README's acknowledgement of *"Navidrome, the
existence proof that Go + embedded SQLite idles at ~50 MB"* does not transfer: Navidrome uses a
**cgo** driver. The `< 80 MB` idle-RSS budget therefore rested on nothing measured. **That is what
revision 3 fixes.**

### Correction, revision 3 — the memory numbers are measured now (x86-64 only)

Measured with `make bench-rss` (`internal/db/spike`, behind the `bench` build tag): a 500k-row
fixture built through the **real `internal/db` open path**, so the pragmas under test are the ones
the binary actually sets; process RSS read from `/proc/self/status` (`VmRSS`, and `VmHWM` for peak),
because `runtime.MemStats` cannot see SQLite's page cache; **one child process per pragma cell**,
because `VmHWM` never falls and nine cells in one process would give one peak and eight fictions.

**Hardware — this is an architecture-level datapoint, not a machine-level one.** `GOOS=linux
GOARCH=amd64`, 4 CPUs (so a read pool of 8 plus 1 writer), 4096 B pages, 15.7 GiB RAM, go1.25.13,
`ncruces/go-sqlite3 v0.35.3`, in a build container — **not** the owner's ThinkCentre. Core count
moves these numbers directly (see the per-connection finding), so treat the architecture as
transferable and the absolute figures as approximate.

**Fixture.** 500,000 rows — 400k `tag_assignment`, 60k `provenance`, 30k `audit_log`, 8k
`write_queue`, 2k `release_candidate` — an 80.0 MiB database of 20,477 pages, built in **11.5 s**
through the single writer in batches of 2,000. `work`/`edition`/`media_file`/`search_fts` do not
exist yet, so the composition targets comparable page and index pressure using the tables migration
0001 creates. **FTS5 memory is unmeasured**; re-run after the search tables land.

**Import peak RSS: 49.7 MiB** for that 500k-row build at the shipped pragmas — comfortably inside
§13's `< 300 MB` import budget.

Read sweep, all MiB. Each row: open+migrate (idle) → one pinned connection → all 8 pool readers
concurrently → a 10,000-row write burst.

| cache_size | mmap_size | idle | 1 reader | 8 readers | after write | peak (VmHWM) | Go heap |
|---|---|---|---|---|---|---|---|
| `-2000` (2.0 MiB) | `0` (off) | 10.0 | 17.1 | 33.4 | 34.6 | **35.0** | 0.3 |
| `-2000` | `64 MiB` | 10.0 | 16.5 | 31.1 | 34.4 | **34.7** | 0.3 |
| `-2000` | `128 MiB` | 10.0 | 17.2 | 34.5 | 34.4 | **36.0** | 0.3 |
| `-8000` (7.8 MiB) | `0` (off) | 10.0 | 22.9 | 82.6 | 84.4 | **85.4** | 0.3 |
| `-8000` | `64 MiB` | 10.0 | 22.8 | 82.7 | 84.1 | **84.6** | 0.3 |
| `-8000` | `128 MiB` | 10.1 | 23.3 | 82.6 | 83.9 | **84.8** | 0.3 |
| `-32000` (31.2 MiB) | `0` (off) | 10.0 | 41.3 | 236.7 | 238.6 | **239.4** | 0.3 |
| `-32000` | `64 MiB` | 10.0 | 41.2 | 235.1 | 238.7 | **238.7** | 0.3 |
| `-32000` ←**shipped** | `128 MiB` ←**shipped** | 10.0 | 40.3 | 235.1 | 236.4 | **237.1** | 0.3 |

**Four findings.**

1. **Idle RSS of the storage layer is 10 MB**, flat across every pragma combination. The `< 80 MB`
   budget survives — but note what is *not* in that figure: no HTTP server, no SPA, no *Arr clients,
   no sync workers. It is the floor, not the binary's idle RSS.
2. **`mmap_size` is a no-op under this driver.** Every requested value reads back as `0`, and
   `PRAGMA compile_options` on the build under test reports `MAX_MMAP_SIZE=0` and
   `DEFAULT_MMAP_SIZE=0` — mmap is compiled out. The `mmap_size` line in the DSN is inert
   configuration. This closes half of `reference/sync.md` §6's pending note.
3. **`cache_size` is per-connection**, which closes the other half and is the load-bearing result.
   One reader → eight readers added **+16 / +60 / +196 MiB** at `-2000` / `-8000` / `-32000`, i.e.
   0.89–1.19× of what a strictly per-connection cache predicts, rather than staying flat. **The
   process pays `cache_size` × (read pool + 1).** The shipped `-32000` therefore costs **237 MB peak
   on 4 cores** — and more on more cores, since the pool is `NumCPU*2`. *(The per-connection verdict
   is inference from those ratios, not a reading of driver internals.)*
4. **Go heap is 0.3 MB at every measurement point.** Anyone reasoning about UsArr's memory from
   `runtime.MemStats` would be off by three orders of magnitude. Use RSS.

**Consequence, and one deliberately unmade decision.** The `< 80 MB` idle budget stands, now on
evidence. `cache_size = -32000` is **not** confirmed as tuned: it is a ~235 MB peak against a ~35 MB
alternative, and changing a shipped default is an owner decision with a latency side this harness
does not measure. Recorded, not silently changed. **`mmap_size` should be dropped from the pragma
list** as dead configuration when someone touches it next.

### Amendment, 2026-08-16 — both pragmas settled

The decision the table above deferred, taken. **The measurements are unchanged; this is what follows
from them.** Both claims were re-confirmed by direct execution before anything moved — `PRAGMA
compile_options` re-read on the linked build, `PRAGMA mmap_size` requested and read back both via the
DSN and via a runtime `SET`, and `cache_size` read from four separately pinned pool connections and
then changed on one of them.

**1. `mmap_size` is removed from the pragma list.** Confirmed inert: `MAX_MMAP_SIZE=0` and
`DEFAULT_MMAP_SIZE=0` in `compile_options`, and `67108864` / `134217728` / `268435456` each read back
`0` whether set through the DSN or at runtime. The cause is structural rather than a flag anyone can
flip: `go-sqlite3-wasm`'s `build/build.sh` compiles the amalgamation with `--target=wasm32
-ffreestanding`, and SQLite only defaults `SQLITE_MAX_MMAP_SIZE` non-zero on platforms it recognises
as having mmap, so a wasm build gets `0`. It will stay `0` for as long as the driver targets wasm.
The line is gone because **inert configuration that looks meaningful is worse than none** — the next
person to tune it would measure nothing and conclude something. `make bench-rss
SPIKE_FLAGS='-mmap=134217728'` still sweeps it on demand, which is the re-check if the driver ever
ships an mmap-capable build.

**2. `cache_size` drops from `-32000` to `-8000`.** Per-connection confirmed directly, not only by
inference from the RSS ratios: four pinned pool connections each read back the requested value, and
setting one connection to `-1000` left the other three untouched. Taking the table's peak column at
face value, on 4 cores: `-2000` → ~35 MiB, `-8000` → ~85 MiB, `-32000` → ~237 MiB.

The reasoning for `-8000`, recorded because the number is otherwise arbitrary: **237 MB would be
harmless on the owner's own hardware** — an x86-64 ThinkCentre with RAM to spare — **but a default
has to be defensible on the small self-hosted boxes this project targets**, and this particular cost
**scales with core count** in a way that is easy to miss, since the read pool is `NumCPU*2`. A
16-core box would pay roughly four times the 4-core figure for a default nobody chose. `-8000` buys
most of the cache benefit at about a third of the footprint.

**The limit of this decision.** It is made on the **memory axis only**. `make bench-rss` measures
RSS; it does not measure query latency, and a smaller page cache is not free on that axis. Nothing
here claims `-8000` is latency-neutral — it is the defensible default until someone measures the
other side. **A latency benchmark that contradicts this is grounds to revisit it**, and that is a
change of evidence, not a reversal of judgement.

Applied in `internal/db/sqlite.go`; `reference/sync.md` §6 and ARCHITECTURE §7.7 / §13 updated to
match. arm64 remains unmeasured, and the sweep still covers `-2000` / `-8000` / `-32000` so an arm64
run produces a comparable row.

**arm64 is unmeasured, and the requirement is re-scoped.** The original text made this spike a
prerequisite to the schema work; the schema shipped and the deployment target is x86-64, so an arm64
run is now a prerequisite to **claiming arm64 support**, not to v0.1 — see ARCHITECTURE §13 and
REVIEW-LOG §R2.6. The command is `make bench-rss` on the arm64 machine; add its output as a second
row here. Page size and core count both move these figures, so it replaces nothing.

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
  requirements, not taste:** a 10k-item ~~virtualized~~ **large** poster grid with instant
  client-side filter/sort *is* a rich client-state problem, and every HTMX interaction is a network
  round trip — the exact thing this project exists to eliminate.
  > **Corrected by [ADR-0029](#adr-0029), 2026-08-16.** UsArr does not virtualize by default any
  > more, so the word "virtualized" in that sentence is no longer accurate and is struck above.
  > **The rejection stands on its own** — the client-side prefix index (ARCHITECTURE §4.5) is the
  > proof that a 10k-item grid with instant client-side filter and sort really is a rich
  > client-state problem, with or without virtualization.

---

<a id="adr-0004"></a>
## ADR-0004 — Replica, not proxy

**Status:** Accepted · **This is the load-bearing decision. Everything else is downstream.** ·
**Refined by [ADR-0026](#adr-0026):** its conflict rule — *"the \*Arr owns the truth; UsArr owns the
cache"* — is narrowed to *the \*Arr owns the truth about the \*Arr's own **state***, and the
three-axis form now lives in **`ARCHITECTURE.md` §2.2**, which is the section that owns the rule.
The conclusion is untouched.

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

**Status:** Accepted · **Refined by [ADR-0030](#adr-0030)** (`work.kind` gains `comic_issue`),
**[ADR-0031](#adr-0031)** (track position is edition-scoped; attribution is M:N) and
**[ADR-0033](#adr-0033)** (`work.kind` gains `person`). None of the three reopens the three-layer
conclusion; they add members and levels inside it.

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

**Status:** Accepted (revision 2) · **Supersedes the write-path half of ADR-0012.** ·
⚠️ **Amended 2026-08-17 by [ADR-0042](#adr-0042)** — see the flag below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0042](#adr-0042): nothing in this ADR's design is altered, and one
> sentence of its supporting prose is off by a milestone.** ADR-0042 re-sequenced the minimal write
> path (`monitor`, `unmonitor`, `delete`, `add`) out of v0.1 to land with the first \*Arr adapter,
> because [ADR-0041](#adr-0041) removed Sonarr and Radarr from v0.1 and all four verbs are Servarr
> operations. **What survives untouched is everything this ADR actually decided** — the state
> machine, `verifying` and its 15-minute TTL, `UNIQUE (user_id, idempotency_key)`, the server-derived
> northbound keys, `grab` at max one attempt, and the reconciliation guard over all three
> non-terminal states. **What is flagged below is the phrase *"every v0.1 write"***, which named the
> \*Arr writes that were v0.1's when this was written and are not now. `write_queue` ships in v0.1 as
> a table with no writer, deliberately: ADR-0042 records that adding one later costs **no migration**,
> because [ADR-0039](#adr-0039) left `state` unconstrained and `kind` never had a `CHECK`.

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
⚠️ **2026-08-17 ([ADR-0042](#adr-0042)): *"every v0.1 write"* no longer has a referent.** The writes
meant here are the \*Arr ones, and they re-sequence out of v0.1 with the adapters that receive them;
v0.1's only write path is Prowlarr Search-and-Grab, which dispatches synchronously and never enqueues.
**The guard's reasoning is unaffected and the rule stands as written** — it simply first bites at the
milestone the first queue writer lands in.

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

**Status:** Accepted · **Extended by [ADR-0026](#adr-0026)**, which binds libraries to containers this
table reports, and adds `remote_library_id` / `remote_tag_ids` / `remote_subtype` to it so three of
the five container kinds have storage.

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
## ADR-0024 — AGPL-3.0 is the licence

**Status:** Accepted · **Confirmed by the owner (Joe, `jdb3750`) on 2026-08-16.**

### Context
[`SETUP-CHECKLIST.md`](./SETUP-CHECKLIST.md) §1.2 listed the licence as a blocking decision, and the
README carried it as a recommendation pending confirmation. It blocks because contributions accepted
into a repository with no `LICENSE` have undefined terms, and retro-licensing means chasing every
contributor for permission.

The neighbourhood UsArr lives in has already converged. **Jellyfin** (GPL-2.0), **Navidrome**
(GPL-3.0) and the newer aggregation projects surveyed in [`RESEARCH.md`](./RESEARCH.md) §5 —
**MediaManager**, **Mydia**, **Reiverr**, **DroppedNeedle** — are all copyleft, and the last four are
AGPL-3.0 specifically. A permissive licence here would make UsArr the outlier, not the default.

The failure mode the licence is being chosen against is concrete: UsArr is a *server*. A hosted
service can run modified UsArr for users over a network and, under a plain GPL, owe them nothing,
because nothing is ever "distributed". That is the hosted-service loophole, and AGPL §13 is the
clause written to close it.

### Decision
> **UsArr is licensed under the GNU Affero General Public License, version 3.0** — the verbatim FSF
> text from <https://www.gnu.org/licenses/agpl-3.0.txt>, in [`LICENSE`](../LICENSE) at the repository
> root.

No per-file licence headers. The root `LICENSE` file and the README's `## License` section are the
whole declaration; headers on every source file are noise the project does not need.

### Consequences
- **Network use triggers the source obligation.** Anyone who runs a modified UsArr as a hosted
  service must offer that modified source to its users. Self-hosting for yourself, which is the
  entire point of the project, imposes no obligation on you at all.
- **Improvements flow back.** A fork that ships changes to users owes those changes to the commons,
  so downstream work stays reachable rather than disappearing into a private deployment.
- **Closed-source commercial forks are deterred**, which is the intended effect. So, unavoidably, is
  embedding UsArr inside a proprietary product — that is the price of the protection, paid
  knowingly.
- **Dependency compatibility must be checked.** AGPL-3.0 accepts MIT-, BSD- and Apache-2.0-licensed
  Go and JS dependencies, which covers the stack in [`ARCHITECTURE.md`](./ARCHITECTURE.md) §3, but a
  GPL-2.0-only dependency would be incompatible. New dependencies get a licence check.
- **It settles the blocking item** in `SETUP-CHECKLIST.md` §1.2, so contributions can be accepted.

### Alternative rejected
**MIT.** Permissive, maximally friendly to embedding, and the path of least friction for anyone who
wants to build on UsArr. It was rejected because it permits exactly the outcome the project is
positioned against: a company takes the code, reskins it, runs it as a closed hosted service, and
returns nothing — legitimately, with no obligation to publish a line. For a self-hosted tool whose
value proposition *is* that you own your own stack, that is the wrong trade. Apache-2.0 was
considered as the patent-grant variant of the same choice, and falls to the same objection.

---

<a id="adr-0025"></a>
## ADR-0025 — Styling and typography: Tailwind v4 with the default theme deleted, Bits UI, Tabler, self-hosted IBM Plex

**Status:** Accepted · **Depends on ADR-0003**, which fixed the framework and said nothing about the
styling layer. Full rationale, with citations, in
[`design/DESIGN-DIRECTION.md`](./design/DESIGN-DIRECTION.md); the canonical values are in
[`design/tokens.css`](./design/tokens.css).

### ⚠️ Amendment, 2026-08-16 — Tailwind is not used, and the enforcement it was chosen for is gone with it

**The styling layer that shipped is hand-written CSS.** `web/src/app.css` is ported from
[`design/mockups/usarr.css`](./design/mockups/usarr.css), carrying the token layer from
[`design/tokens.css`](./design/tokens.css) under its canonical names. There is no
`@tailwindcss/vite`, no `@theme`, no utility class anywhere in `web/`. Owner-confirmed on
2026-08-16: *"if custom css is what it wants that's probably fine; I only suggested tailwind for
speed"*. Decision points **1** and **5** above are replaced by this; **2** (Bits UI), **3** (Tabler)
and **4** (IBM Plex) are untouched, and **6** (serving) was always about `embed.FS` rather than about
the CSS engine.

**What is lost, stated as a loss.** Tailwind was never wanted for utility classes — the Decision
above is explicit that `@theme { --*: initial; }` is *"mandatory and load-bearing"*, and the
Consequences lead with *"the generic look is unavailable rather than discouraged"*. That is the whole
argument, and it rests on a mechanism hand-written CSS does not have: with the default theme deleted,
`bg-indigo-500` and `rounded-2xl` are **structurally nonexistent**, so a contributor cannot type them
and a reviewer cannot forget to look. The replacement is
[`design/check.mjs`](./design/check.mjs)'s §13 ban list, and it is a **weaker guarantee than "the
class does not exist"**. A grep can be evaded, and worse, it can silently match nothing while
printing `ok` — which is not hypothetical: two of `check.mjs`'s own rules were found matching zero
things and reporting a pass, which is why the file now prints what it *checked* rather than only what
failed. **This is not parity and must not be recorded as parity.** The enforcement dropped from
compile-time impossibility to a lint that has already been observed to have blind spots.

**What is gained.**

1. **The token layer is unchanged, because it never depended on Tailwind.** `tokens.css` is plain
   custom properties; it was consumed through `@theme inline` and is now consumed directly. This is
   exactly the seam the Consequences named — *"swapping the utility engine later touches the token
   file and the class attributes, not the component logic"* — being used for what it was left for.
2. **`usarr.css` was already hand-written**, so the design system ports across with its measured
   values intact — the row-height bands, the contrast-audited pairs, the density steps — instead of
   being re-derived as a utility vocabulary and re-measured. The port is a move, not a translation.
3. **The build loses a native Rust dependency whose postinstall fetches a binary.** That is the ADR's
   own recorded strongest argument against Tailwind, and it is discharged rather than mitigated. It
   is also the closest thing here to the pre-agreed Open Props fallback's trigger, though the route
   taken is the *"Hand-rolled CSS with a custom token layer"* alternative rather than Open Props.

**One correction worth recording, because it was nearly generalised into a rule.** The
`@tailwindcss/oxide` objection is real and was re-verified: it declares a fetching postinstall. It
**does not generalise to native-adjacent packages by reputation**. `playwright` was suspected of the
same shape and is not guilty — its install script was removed upstream in 1.38.0, and
`playwright-core`, the package `web/package.json` now pins at 1.56.1, declares no scripts at all and
downloads nothing on install (observed directly, not assumed). The objection applies to packages that
actually declare a fetching postinstall, and to those only.

**Bits UI and Tabler icons are unaffected** by this amendment and stand as originally decided; neither
depends on the CSS engine. **Self-hosted IBM Plex has now shipped** and is verified rendering, which
`check.mjs` asserts as a standing check rather than a one-off observation.

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
- **The fonts are the real byte cost, and it is now measured.** Three faces (Sans 400/600, Mono
  400) as WOFF2 subsets, by `Content-Length` on 2026-08-16: **103.6 KB for `latin` alone**
  (44.6 + 44.6 + 14.4) and **177.2 KB with `latin-ext`**. The earlier 🔍 ~120–180 KB estimate was
  correct for `latin` + `latin-ext` and pessimistic by ~40% for `latin` alone. Neither figure trips
  the ~200 KB trigger, so the decision stands on its own terms — but **the subset, not just the
  family, is the thing to decide** (OQ-3): drop `latin-ext` before dropping a weight, and a weight
  before the family.
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
  across platforms** — a title that fits a 32px row on macOS may wrap on Windows. 🔍 **That last
  claim is inference, not a citation**: it is a typographic commonplace and the drift has never
  been measured for this design, which matters because it is the sole reason the system stack lost.
  MDN separately warns
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

---

<a id="adr-0026"></a>
## ADR-0026 — A library is a user-owned binding to upstream containers, with a correction layer

**Status:** Accepted · **Refines [ADR-0004](#adr-0004)** (replica, not proxy) and **extends
[ADR-0014](#adr-0014)** (the M:N link). Neither is superseded; ADR-0004's conflict rule is *narrowed*
below, in writing, rather than quietly reinterpreted. · ⚠️ **Amended 2026-08-17 by
[ADR-0043](#adr-0043)** — see the block below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0043](#adr-0043): one consequence bullet's cap on the correction
> *UI* no longer holds whole.** The bullet *"It absorbs scope rather than adding it"* closes with
> *"The correction **UI** is capped to v0.3, where the weak catalogues actually arrive — §6.4 already
> establishes that tier 1 resolves essentially 100% of the v0.1 identity problem, so there is nothing
> to correct before then."* **Both halves of that ground were falsified by [ADR-0041](#adr-0041)**,
> which put a weak catalogue **in v0.1** and replaced the providers §6.4's percentage was measured on;
> `ARCHITECTURE.md` §6.4 has since been restated against the new source and withdraws the support
> explicitly. ADR-0043 therefore **moves the minimal *"fix this match"* case earlier than v0.3** and
> **leaves the full four-verb surface and the Corrections list at v0.3**. The bullet carries an inline
> flag at its own site.
>
> **Nothing else in this ADR moves.** The binding model, the single-kind rule, the four verbs
> `exclude` / `include` / `relink` / `field`, their `library_id` scoping and `CHECK`, the four tables,
> `target_identity_hash` and the no-foreign-key rule, the three-axis split and every rejected
> alternative all stand exactly as decided. **This is a scheduling amendment to one consequence, not a
> reopening of the design** — and the storage it re-sequences against is already in the tree.

### Context

The owner's scope moved to six media types, and with it came a requirement that does not fit the
existing model: **user-defined libraries, configured separately from services.** His stated
motivation is not storage control — he does not want UsArr reading a filesystem — it is that
**LazyLibrarian's own idea of a library is poor, and UsArr's organisation must be able to be better
than a service's.**

That is a substantiated complaint, not a taste. From LazyLibrarian's source: a file its matcher
cannot bind to a metadata-provider record has **no row written at all** — the failure lands in a
local dict that produces a debug-log line and an *"N unmatched items"* banner, and never a database
row — so its library is its *provider's* view, intersected
with what a fuzzy threshold accepted. Its own documentation sets match ratios at *"somewhere around
80% to 90%"* and warns looser matching *"will get matches against the wrong books"*. Its primary key
is borrowed from whichever provider was configured when the row was created, which is why it ships
`listAlienBooks` as a diagnostic. And its correction mechanism fails in the way that matters:
⚠️ **books marked ignored are reported to come back after an author rescan**, because the rescan
returns the book with a different provider id (LazyLibrarian GitLab issue #2407 — **no maintainer
resolution, and the reporter says they may be reading the wrong code**, so it is carried as
unverified rather than as observed behaviour).

The design also had three dangling referents that all wanted the same missing object:
`user_library_access` (ARCHITECTURE §12.2 names it, with no `library` table to point at), §8.3's
*"a routing rule"* (named, never defined), and v0.4's `getMusicFolders` (an endpoint with nothing to
return).

Three options were considered: **(A)** libraries as pure UI groupings over instances; **(B)**
libraries as bindings to containers the upstream already computed; **(C)** UsArr scanning a
filesystem. **(C) was ruled out by the owner and was also the wrong answer** — it would make UsArr a
media server (ADR-0023, ADR-0018) and would require a *scanner*, the exact component whose
misidentification failures fill this ADR's citations.

### Decision

> **A UsArr library is a user-owned, named, single-kind, format-filtered binding to containers the
> upstream services already computed — a whole instance, a root folder, an upstream library id, or an
> \*Arr tag — with materialised membership, one declared request sink, and a narrow user-owned
> override surface of exactly four verbs: `exclude`, `include`, `relink`, `field`. UsArr never
> touches a filesystem.**

Four tables in migration 0001 — `library`, `library_source`, `library_member`, `library_override` —
and one renamed column, `search_doc.instance_scope` → `library_scope`. DDL in
[`reference/schema.md`](./reference/schema.md); the model in
[`ARCHITECTURE.md`](./ARCHITECTURE.md) §6.5.

**The refinement to ADR-0004, stated explicitly because it is the one thing here that could be read
as a contradiction.** ADR-0004 says *"the \*Arr owns the truth; UsArr owns the cache."* That stands,
and is narrowed to: **the \*Arr owns the truth about the \*Arr's own state; it never owned the truth
about the user's organisation.** Three axes, and keeping them apart is the whole design — **state**
(exists, monitored, has a file, quality profile, root folder) is upstream's, always, with **no
override tier at all**; **organisation** (which library, what it is called, what kind, what feeds it,
where requests go) is UsArr's, always; **display identity** (title, sort title, year, cover, and "is
this link really this work") is upstream's by default and the user's when they say so.

A replica that can be more correct than its source is still a replica. It is one with an owned
overlay, which §2.2 already grants for tags, requests and playback position; corrections join that
list. State is excluded from the overlay deliberately: those fields are the inputs to the write queue
(§7.6), and a user-editable copy means the UI claims "monitored" while Sonarr disagrees, the queue
issues commands against a fiction, and the sweep fights the user forever.

**Catalogue source and request sink are separate bindings, and that is the normal case rather than
the books exception.** Navidrome catalogues music, Lidarr or Prowlarr takes the request.
Audiobookshelf or Calibre catalogues books, LazyLibrarian takes the request and its catalogue is
ignored — which is the owner's point made operational, because without the split, adding
LazyLibrarian to get requests would import its bad catalogue as the price. Komga and Kavita both
catalogue comics with no sink at all beyond Prowlarr free-text.

### Consequences

- **A correction is keyed to UsArr identity (`work_id`/`link_id` plus `target_identity_hash`) and is
  never cleared by a sync, a reconciliation sweep, a tombstone expiry or an id resurrection.** That
  rule exists specifically because of ⚠️ issue #2407 (unverified — see above): keying a correction
  to the upstream's id reproduces the failure by construction, which is why the rule holds whether
  or not that particular report does. Expressed in the DDL as **no foreign key on the override's
  `work_id` / `link_id`**, with `target_identity_hash` as the durable key — an `ON DELETE CASCADE`
  would let a tombstone expiry destroy the correction, which *is* the failure being designed
  against.
- **Library membership is never an input to identity**, enforced by a CI assertion that no query in
  the identity path references `library_member`, `library_source` **or `library_override`**.
  jellyfin#10985 is the counter-example — the same film in three per-language libraries collapsed
  into one item and watch state leaked across all three. The third table is named because it is the
  one library-named table that by design *does* feed identity: its `relink` verb repoints a
  `service_item_link`. So the assertion is two assertions — the identity **cascade** references none
  of the three; the correction **applier**, which runs after it, references `library_override` and
  nothing else.
- **Two of the four verbs are not library-scoped, and the schema now says so.** `relink` changes
  which `work` a link points at, which is UsArr's identity graph and global by construction;
  `field` overrides title, sort title, year or cover on a `work`, and if that work is in two
  libraries then either the override is global or the same work renders under two names. **This
  ADR's own three-axis table puts both in a row whose owner is "upstream by default, an explicit
  user correction" — with no mention of a library at all**, so the axis table and the storage
  disagreed. Resolved toward the axis table: `library_id IS NULL` for `relink` and `field`,
  `NOT NULL` for `exclude` and `include`, enforced by a `CHECK`. The visible consequence is that
  **deleting a library discards its exclusions and nothing else**, and ARCHITECTURE §17.8's
  confirmation copy now says so with the count instead of reassuring the user that nothing is
  deleted.
- **Membership is a deterministic predicate, never a similarity score**, materialised in the
  link-write transaction and dirty-marked/flushed per 250 ms batch exactly like the availability
  rollup. The only path UsArr ever compares is `root_folder_path`, as a prefix, on a value the
  upstream itself reported.
- **The library kind is required, CHECKed, and editable.** Every tool that scans disk types its
  libraries — Jellyfin's untyped mode is documented by Jellyfin as *"broken and deprecated"*, and its
  removal proposal calls the detector *"very poorly implemented"*. UsArr can additionally allow the
  type to change, precisely because nothing is parsed from a path. ⚠️ **Plex is *reported* not to
  allow this, on the strength of a community feature request rather than any official Plex
  statement**, and it has not been verified against a current build. The capability stands on its
  own; the comparison is not evidence, and an earlier draft carried it as fact in five places.
- **A library with zero sources is retained and marked orphaned, never auto-deleted.** It carries a
  user's name, corrections and access grants; destroying owned data to tidy up replicated data is the
  wrong trade.
- **It absorbs scope rather than adding it.** §8.3's undefined routing rule gets a definition,
  `user_library_access` gets a referent, and Home's per-type sections are *replaced* rather than
  extended. ~~The correction *UI* is capped to v0.3, where the weak catalogues actually arrive — §6.4
  already establishes that tier 1 resolves essentially 100% of the v0.1 identity problem, so there is
  nothing to correct before then.~~ 🚩 **STRUCK 2026-08-17 by [ADR-0043](#adr-0043), and struck rather
  than deleted because it is the clause the amendment at the top of this ADR exists for.** Both of its
  grounds went at once: [ADR-0041](#adr-0041) put a **weak catalogue in v0.1** — so the weak catalogues
  do not "actually arrive" at v0.3 — and it replaced the two providers §6.4's *"essentially 100%"* was
  measured on, which `ARCHITECTURE.md` §6.4 has since restated as *"a property of the instance, not of
  the design"*. There **is** something to correct before v0.3. **What replaces it:** the minimal
  *"fix this match"* case moves earlier, its milestone unassigned; the **full** four-verb surface and
  the Corrections list stay at v0.3. ARCHITECTURE §16.0 is authoritative for the scope.
- **UsArr now owns a bug it cannot blame upstream for.** If the derivation is wrong, the grid shows
  the wrong items and there is no \*Arr to point at. Materialised membership is a cache with an
  invalidation problem. This is the honest cost.
- ⚠️ **The library-scoped grid query is unmeasured** at the §13 reference library size. Mitigation in
  order: the common case is `work.kind = ?` with membership as a one-row lookup; failing that,
  denormalise the sort key onto `library_member`. It is a CI `EXPLAIN QUERY PLAN` assertion and a
  `make bench` line, not an assumption.

### Alternatives rejected

- **(A) Libraries as pure UI groupings over instances.** Cheaper, and it cannot express the two cases
  that motivated the feature: a subset of one instance (a root folder, a tag), and two libraries over
  one upstream library — the Ebooks/Audiobooks split that Audiobookshelf itself cannot do, because
  its `mediaType` is only `book|podcast` and the distinction lives at item level.
- **(C) Scanning a filesystem.** Refused by the owner and independently wrong: it needs a scanner,
  path access, file watching, permissions handling and a new trust surface, and it would make UsArr a
  media server. What it would genuinely buy is *files no service knows about*, and the honest answer
  is "import them into a service that owns bytes; UsArr is not that." Content-hash file identity — the
  other thing it would buy — is already answered better by the deferred `content_key`.
- **A `library_sink` table** (many sinks per library). Unnecessary because `library.formats` exists:
  "ebooks here, audiobooks there" is two libraries. *Cut before you add.*
- **Making a library a tag, or a saved filter.** Conflating the three is the mistake ADR-0015 already
  records from Komga and Kavita. A library is a single-kind container with sources and a sink; a tag
  is a cross-kind label; a saved filter is a query. Cross-kind grouping — "Kids", "Christmas" — is a
  tag, never a library.

---

<a id="adr-0027"></a>
## ADR-0027 — Two axes: media type is navigation, a library is scope

**Status:** Accepted · **Settles the navigation question ARCHITECTURE §17.2 left open**, and closes
`design/DESIGN-DIRECTION.md` OQ-2 in the sidebar's favour.

### Context

§17.2 deliberately left v0.1 navigation open between *"sections on the home page"* and *"per-type
tabs in a top navbar"*, saying "pick one on first contact with real data and do not relitigate".
DESIGN-DIRECTION §8.1 then chose a **left sidebar**, which is neither option, and recorded the
tension as OQ-2. Two things have since changed: **the owner confirmed the left sidebar**, and the
media-type count went from two to six, which is new evidence rather than a re-litigation — §17.2's
second option was drafted when "per-type" meant two, and six types plus Home · Search · Requests ·
Services · Settings · System is twelve top-level items in a horizontal strip, before Calendar and
Stats, with the persistent search input left nowhere to go.

Simultaneously, user-defined libraries (ADR-0026) are **unbounded in number**, and there is a
documented failure mode for putting unbounded things in a sidebar.

### Decision

> **Media type is the navigation axis: a closed set of six — movies, TV, music, ebooks, audiobooks,
> comics — rendered as sidebar entries, showing only types that have content. A library is a *scope*,
> not a place: a multi-select chip above the nav, reflected in the URL as `?lib=`, on the routes that
> already exist. The two axes are never merged.**

The chip is **Navidrome's `LibrarySelector` model, deliberately and in detail**: multi-select rather
than single-select; stating the current scope in words ("All libraries (4)" / "2 of 4 libraries");
**rendering nothing at all at 0 or 1 library**; and built from native checkboxes in a popover.
⚠️ **"Keyboard-operable for free" overstates what native checkboxes give**, and an implementer
following that sentence ships a list where the arrow keys do nothing: native
`<input type="checkbox">` elements are **Tab**-traversed, not arrow-navigable — only radios within a
group rove on arrows — and `Esc`-to-close is popover behaviour, not checkbox behaviour. (Navidrome's
`LibrarySelector`, cited as the model, uses MUI `Checkbox` components inside a MUI `Popover`, so it
is not evidence for the native claim either.) **What is genuinely free: Space toggles, Tab traverses,
the checked and indeterminate states are announced, and the label is the target.** What the popover
must **add**: arrow-key roving, `Esc` to close with focus returned to the chip, and closing on
`focusout` when focus leaves the popover. Three behaviours, named so they get written.

Pinning exists as a concession — **opt-in, default none, capped, in its own sidebar group, with one
`More…` overflow** — because a library used hourly should be one click away and refusing that is
dogma. A pin sets the scope and lands on Home; it is not a separate view of the app.

### Consequences

- **Zero new routes.** Six media types plus N libraries costs no new page types. Back, forward,
  reload and share work for free, per the URL-state rule.
- **Cross-library everything survives**, because the default scope is all of them.
- **Nothing is ever silently hidden**, and there is exactly one control to look at when a user asks
  "where did my music go" — labelled with the answer.
- **The sidebar has a stated row budget**: **8 fixed entries with Calendar and Stats, 6 today** +
  ≤6 types + the chip = **15 at full expansion, 13 today**, so pins are capped at
  `16 − fixed − types`, shrinking automatically as fixed entries arrive. (ARCHITECTURE §17.2 said
  "eleven fixed entries" over a list of eight and is corrected; the arithmetic here was ambiguous
  between the 6-entry and 8-entry cases, and the cap differs by two depending on which is read.) 🔍 16 is derived from the design's own 32 px sidebar row height against a 900 px viewport,
  cross-checked against Kavita's published "10 items + Home"; it is not empirical.
- A type with zero items is not rendered anywhere — sidebar, home, or search group.

### Alternatives rejected

- **Libraries as sidebar nav items.** Jellyfin's `libraryMenu.js` maps `items.map(...)` over every
  user view with **no cap, no pin, no overflow and no reorder** — the unbounded-sidebar failure in
  shipping code. Calibre-Web reached **seventeen** `SIDEBAR_*` visibility bits on *one* library, which
  is what happens when queries get promoted to places. Kavita had to retrofit "10 items + Home, rest
  under More". Every one of those is a mechanism for *managing* growth; a scope filter removes it.
- **A single-select library switcher that scopes the app.** Audiobookshelf's own documentation:
  *"Most actions in the server apply to the currently selected library, **including browsing and
  searching**"*, and an author with series in two libraries shows as **two separate author entries**.
  That makes "everything in one place" unimplementable.
- **Pin state as the only way to reach a library.** Plex's pins live per client and are lost:
  a 2026-07-09 forum thread reports *"10+ of my users have reported that their pinned libraries have
  vanished from the home screen. The 'More' option … is gone entirely"*, with four years of the same
  class behind it. Any mechanism whose default state is *hidden* eventually strands a user.
- **Per-type tabs in a top navbar** — §17.2's second option, written for two types. Twelve items and
  no home for the search input.
- **A two-level sidebar or flyout submenus.** NN/g: designs beyond two disclosure levels *"typically
  have low usability because users often get lost when moving between the levels"*.
- **A command palette as the way to reach a library.** It is the standard rescue for an unbounded nav
  list, and this decision has already deleted the unbounded nav list. Not one surveyed application
  ships one — Sonarr's entire global keyboard surface is five bindings — so it would be the single
  most novel thing in a design brief whose stated position is "standard patterns over novel ones",
  and Nielsen's heuristics make accelerators a *second* path, not the primary one. **The seam ships:
  build the `/`-focused search input so a leading `>` could later switch it into command mode, and
  keep the route table and the type/library registries as data. The palette does not.**

---

<a id="adr-0028"></a>
## ADR-0028 — Home is three fixed blocks, not one strip per media type

**Status:** Accepted · **Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §17.2**, which specified
per-type horizontal strips of recently added items. That statement is replaced, not appended to.

### Context

§17.2's home was *"one section per media type present in the library … each showing a horizontal strip
of recently added items"*. It was written for two types. At six it is a wall of carousels generated
mechanically — which is exactly what Jellyfin does: `loadRecentlyAdded` iterates `userViews` and calls
`renderLatestSection` per library, producing one horizontal carousel per library, unbounded, inside a
single home section slot, below Resume, Resume Audio, Resume Book, Live TV and Next Up. Jellyfin's
only mitigation is a set of per-library opt-out checkboxes buried in user settings, and the community
has shipped a plugin that *wholesale replaces the home screen*.

### Decision

> **Home is three fixed blocks: a ≤6-row media-type summary table; an attention block that is hidden
> entirely when empty; and one unified "Recently added" table across all types with a Type column.
> Home's height is O(1) in the number of media types. No horizontal strip appears on Home in any view
> mode.**

Choosing the Posters view renders the third block as **one wrapping grid across all types**, not six
strips. Strips remain legitimate on an item-detail page ("More from this artist", ≤5 items).

### Consequences

- **The decisive argument is UsArr's own published test, not the external research.** Six strips show
  ~16 items above the fold at 1440×900 — a 154 px poster grid fits ~8 across a ~1200 px column, a card
  plus meta is ~260 px, plus header and gap ≈ 300 px per section, so a 900 px viewport minus the
  40 px toolbar shows 2.8 sections — against the design's own 25-item floor, on the screen whose job
  is inventory. It fails before any citation is consulted.
- **The supporting evidence points the same way, restated accurately because it was not.** Runyon's
  **ND.edu** instrumentation — **28,928 tracked feature clicks over ~3 months**, mid-October 2012 to
  22 January 2013 (<https://erikrunyon.com/2013/01/carousel-interaction-stats/>) — found ~1%
  click-through with **84%** of it on the first slide. A **five-site follow-up**
  (<https://erikrunyon.com/2013/07/carousel-interaction-stats/>) found position-1 shares of
  **54.57–89.1%** and click-through of **1.07–9.41%**, and publishes **no aggregate**. Three things
  were wrong in the earlier wording and all three are worth having right: 28,928 is one property and
  not five; 84% is that property's own share and not an aggregate; and "~1%" silently picked the
  lowest of five values ranging to 9.4%. The corrected form is also the fairer one to have in an
  ADR, because the 9.4% outlier is real. **No URL was cited in any of the three places this appeared,
  in a repository whose rule is that every claim cites a primary source.** NN/g:
  *"Include 5 or fewer frames … it's unlikely users will engage with more than that"*, and people
  *"often scroll past carousels"*. Baymard: silent truncation makes users assume they have seen
  everything. jellyfin/jellyfin#16615, asking for a wrapping grid instead of horizontal rows, was
  **closed as not planned**.
- 🔍 **The honest limit, carried rather than dropped: no research exists on carousels in media
  libraries.** Every carousel finding cited measures marketing or ecommerce contexts. The transfer
  argument is that the *interaction* is identical — content reachable only by horizontal travel,
  truncated without a count — and that the content here is *weaker* than a marketing hero, since
  nobody optimises which twelve items land in "Recently added". That is reasoning, and it must not be
  quoted as a finding.
- **The attention block is the differentiator and has no equivalent in any surveyed tool**, because
  neither Jellyfin nor Plex knows what is *missing*. It is hidden when empty rather than showing an
  "all good" panel.
- **The layout improves as types are added** — a sixth type adds rows to an existing table rather than
  a sixth region to scan — and the unified table sorts, filters and Ctrl+Fs.
- **v0.1 ships no home-screen configuration at all**, with two seams pre-wired: one ordered list of
  present media types per user, driving both the sidebar order and Block A (Plex's "sidebar order is
  home order", which is one stored list and two renderers), and a per-type `show_on_home` boolean
  defaulting true. **`show_on_home` is the only home flag that survives this decision**: the
  libraries research proposed a per-library `include_on_home`, and under a three-fixed-block Home
  nothing reads it — Block A is per media *type*, Block C is unified, and the scope comes from the
  `?lib=` chip. Two overlapping flags where one has no consumer is how dead columns get into the one
  migration that can never be edited, so `include_on_home` is **cut from the `library` table**
  rather than carried (REVIEW-LOG §R2.1, corrected). Plex, Jellyfin, Emby and Kavita all ship home configuration because their default
  does not scale; this default is designed to.

### Alternatives rejected

- **Keeping per-type strips and adding an opt-out.** Jellyfin's answer, and it is seven checkboxes in
  user settings compensating for a layout that does not scale.
- **A configurable home-section list** (Jellyfin's ten `<select>` slots, Kavita's reorderable
  streams). Configuration for a problem the default should not have.
- **Auto-advancing anything.** Independently banned by the design's motion rules, and NN/g is blunt:
  auto-forwarding carousels *"annoy users and reduce visibility"*.

---

<a id="adr-0029"></a>
## ADR-0029 — "Load more" plus `content-visibility`; virtualization is a benchmarked escalation

**Status:** Accepted · **Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §4.5**, which said "virtualize
everything over ~200 rows", and **corrects a supporting argument in [ADR-0003](#adr-0003)** without
reopening its conclusion. Closes `design/DESIGN-DIRECTION.md` OQ-1, which the owner delegated. ·
⚠️ **amended 2026-08-16** — the required benchmark has been run against the shipped list primitive;
**the decision is unchanged** and three supporting numbers below are corrected by it. ·
⚠️ **amended 2026-08-17** — the 100 ms this ADR's arithmetic divides by was never this ADR's to use:
the density and theme toggles are not Tier 0, and `design/DESIGN-DIRECTION.md` §7.2 now carries a
**Controls** budget for them at **400 ms**. **The decision is again unchanged**; every row ceiling
below moves, and the page size does not.

### ⚠️ Amendment, 2026-08-17 — the budget this ADR divides by was borrowed, and it was the wrong one

**What this amendment does not touch.** The Decision stands: "Load more" over keyset pages plus
`content-visibility: auto`, virtualization as a benchmarked escalation, **and `LOAD_MORE_PAGE_SIZE`
at 200 rows, which is unchanged**. What moves is the denominator underneath every row-ceiling figure
in this ADR, and the residual-risk note that denominator produced.

**The defect, stated as the general rule it is an instance of.** This ADR, `ARCHITECTURE.md` §4.5 and
`DESIGN-DIRECTION.md` §7.4 each argued that the two toggles are *"pure-local no-data interactions"*
and concluded they were therefore **"Tier 0 by `design/DESIGN-DIRECTION.md` §7.2's own definition —
whose hard fail is 100 ms."** §7.2 defined no such category. Its Tier 0 reads *"the data is in local
SQLite. Nearly every read,"* and names a breach *"a query-plan bug"* for the `EXPLAIN QUERY PLAN`
assertions — a diagnosis that is meaningless for an interaction with no query. **A budget belongs to
the document that defines the category; a document that wants a budget must not extend someone
else's by inference.** §7.2 has now been given the category on its own authority, derived from §6's
existing anchors — Nielsen's 0.1 s instantaneous limit as the target, Doherty and Thadani's 400 ms
flow limit as the hard fail — and the derivation is deliberately independent of what the toggles
cost. **Tier 0 is unchanged at 100 ms for reads.**

**1. Every row ceiling in this ADR is the budget over a per-row cost, so all of them scale by 4×.**
No per-row figure was re-measured and none changed; only the numerator did.

| Figure, as this ADR states it | Against 100 ms | Against 400 ms |
|---|---|---|
| Shipped cost curve `0.0146 ms/row + 6.4 ms`, desktop | ≈ 6,400 rows | ≈ 27,000 rows |
| Same curve, 🔍 Pi-class at 3–5× | 930–1,840 rows | **5,000–8,700 rows** |
| Worst observed row shape, `0.214 ms/row`, desktop | ≈ 500 rows | ≈ 1,870 rows |
| Same worst case, 🔍 Pi-class at 3–5× | **100–167 rows** | **374–623 rows** |

⚠️ **The first two rows are worth less than they look, and this ADR already said why**: the linear
fit is good only to a few thousand rows because the 25,000-row point is superlinear, so ≈ 27,000 is
an extrapolation several times further past the fit's range than ≈ 6,400 was. **It is not a ceiling
anyone may build against, and raising the budget made that worse rather than better.** The operative
figures are the worst-case row.

**2. The residual-risk note is weakened, not deleted, and here is exactly how much.** §3 of the
2026-08-16 amendment reads *"on the worst-case row shape, one 200-row page is already at the Pi-class
limit (200 against 100–167)"*. Against the *Controls* budget the same worst case admits **374–623
rows**, so **a 200-row page now sits at roughly half the worst-case Pi-class ceiling instead of
over it.** The concern does not vanish — heavy row shapes still carry the least headroom on this
page size, and the required `make bench` line is still what settles whether such a list needs a
smaller page — but it is no longer a statement that the shipped default is already at the limit.
🚩 **Recorded rather than removed, because the sentence was right about the ratio and wrong only
about the threshold it was measured against**, and deleting it would hide that the risk was
overstated by a borrowed number rather than discovered to be absent.

**3. The shipped page size now has a measured cost, and it clears the budget by about 5%.**
📏 **Measured by the frontend thread, not by the thread writing this amendment.** The record is
`web/scripts/measurements/2026-08-17-density-invalidation.md`, added by **`dff20fd`**, measuring
**tree `3ff8151` plus that change** on **Chromium 141.0.7390.37 headless** (`playwright-core` 1.56.1),
**Node v22.22.2**, viewport **1440×900**, machine class **x86-64 container, 4 vCPU (Intel Xeon @
2.80 GHz), 15 GB RAM, shared host** — which that record calls *"a reasonable proxy for a ThinkCentre
under Proxmox"* and expressly ***"not a proxy for a Pi 5."*** One density change through the real
product path, five samples per cell, medians, on the with-invalidation path that holds scrollbar
error at 0.00%:

| Rows in the page | 100 | 120 | 160 | 200 |
|---|---|---|---|---|
| Density toggle, shipped path | **32.1 ms** | **37.4 ms** | **49.3 ms** | **75.7 ms** |

🔍 **At this ADR's own pessimistic 5× Pi-5 factor, the shipped 200-row page is 378.5 ms against a
400 ms hard fail — 21.5 ms of margin, about 5%.** ⚠️ **That is stated at full precision rather than
rounded to "passes", because it is thin**, and because a budget chosen to flatter a measurement would
not have landed 5% above it. The scaling is inference — the source scaled nothing to a Pi and §13
forbids quoting a Pi-derived figure as measured — while the desktop figures clear 400 ms outright.
ℹ️ **Two limits the source itself imposes**: its runner is noisy, **100 and 120 rows overlap and must
not be read apart**, and it declines to support a page-size decision on that instrument. This
amendment therefore uses the curve as a budget check and **does not revisit the page size**.

⚠️ **Note what these figures do to §3 of the previous amendment.** Its cost curve predicts about
**9.3 ms** for a 200-row density toggle; the shipped path now measures **75.7 ms** at that size. Two
contributors are visible and this amendment does not apportion between them: a different machine
class, and `dff20fd`'s forced re-measurement, which is the cost of *complying* with §7.4's
invalidation rule and did not exist when the curve was taken. **The curve is superseded for the
shipped path by a direct measurement at the size that actually ships**, and the curve's remaining
value is its shape, not its constant.

**4. What would have happened under the old framing, recorded because it is the evidence the framing
was wrong.** Tier 0's 100 ms under the same 5× factor is a **20 ms** desktop-equivalent budget — the
figure the measurement record names in its own §7 — and against it **every page size in the measured
range fails, 100 rows included at 32.1 ms**, by 1.6× at the smallest setting and 3.8× at the shipped
one. **A rule that fails at every available setting is not a strict rule, it is a misapplied one.**

### ⚠️ Amendment, 2026-08-16 — the benchmark ran; the decision stands and three of its numbers move

**The measurement this ADR made a required task now exists**, taken against the list primitive that
actually shipped — `web/src/app.css` §2.6, grid rows carrying explicit `role="table"` / `role="row"`
/ `role="columnheader"` / `role="cell"`, which is what the Decision above specifies. **Nothing here
touches the Decision.** "Load more" over keyset pages plus `content-visibility: auto` is still the
default renderer, virtualization is still an escalation, and the escalation threshold is still set
from a benchmark rather than from judgement — the rejected alternative *"picking a new threshold now,
from judgement"* is **not** reopened, because §3 below is the benchmark this ADR demanded, arriving
on the condition this ADR set. What moves is the arithmetic underneath, plus two gaps that are
written down rather than left as folklore.

**Conditions for everything below**, since a bare millisecond figure with no conditions is the class
of claim this ADR exists to correct: **desktop x86 Chromium, the shipped grid-row primitive, 5,000
rows in the DOM, density toggle**, unless a line says otherwise. The Pi-class figures are this ADR's
own 3–5× reference-hardware factor applied to those desktop numbers, and are marked 🔍 inference
wherever they appear, exactly as the original extrapolation was.

**1. Correction (c) over-counts on the shipped primitive.** It reads:

> **`contain-intrinsic-size` sizes the *content* box**, so padding and border are added on top —
> a 24 px row with `auto 28px` produced a **37 px** placeholder (28 + 8 + 1).

**The CSS fact is right; its application to the shipped row is not.** Measured on the shipped
primitive, **the row's computed padding is `0`** — padding lives on the *cell*
(`.tbl td { padding: var(--row-pad-y) var(--row-pad-x) }`), while `.tbl tbody tr` declares only
`min-height: var(--row-h)` and a 1 px `border-bottom`. So a one-line row's content box is **not**
`--row-h` plus padding plus border: the cell padding sits *inside* the row's content box rather than
outside it. The 37 px figure came from a probe that put the padding on the row, and it does not
describe what shipped.

⚠️ **This paragraph used to end "a one-line row's content box is `--row-h`", and that sentence is
withdrawn as written — it was already false when it was written.** It holds for the **`two-line`
fork before the frontend thread's `.stacksep` margin fix** and for nothing else. Measured, with
every figure carrying its box:

✅ **RE-MEASURED POST-FIX, 2026-08-16.** The `.stacksep` fix is on `main` at **`440e92d`**; the table
below is a measurement of the merged tree, with the pre-fix tree (`3ae0d44^`) measured by the same
script as a control. One-line rows, 1440×900, both forks, all three densities. *As rendered* is the
shipped `min-height: var(--row-h)`; *natural* is the same row under a forced `min-height: 0`.

| Fork | content box, as rendered | border box, as rendered | content box, natural | border box, natural | floor |
|---|---|---|---|---|---|
| `two-line`, **before** the `.stacksep` fix | **28 / 32 / 36** | 29 / 33 / 37 | 28 / 32 / 36 | 29 / 33 / 37 | **inert** |
| `two-line`, **after** it | 27 / 31 / 35 | **28 / 32 / 36** | 26 / 30 / 34 | 27 / 31 / 35 | **binds** |
| `labels` (never emitted a `.stacksep`; identical on both trees) | 27 / 31 / 35 | **28 / 32 / 36** | 26 / 30 / 34 | 27 / 31 / 35 | **binds** |

`--row-h` is **28 / 32 / 36**, so the equality with the **content** box held on exactly one fork at
one moment and holds nowhere now; post-fix `--row-h` is the **border** box on both forks, because the
floor sets it. 🚩 **Note the shape of the hazard rather than only the correction: "28 / 32 / 36" is
true on both sides of that merge — as the content box before and the border box after — so nothing
about the digits looks stale and a reader who checks them against a fresh measurement gets a match
either way.** Every row height in this ADR names its box for that reason.

✅ **The `labels` row is now settled, and `DESIGN-DIRECTION.md` §7.4's competing 26 / 30 / 34 was not
competing.** It is the **natural content box** — the floor removed — which is 2 px under the rendered
border box because it is two boxes under it, one `min-height` and one border. Three figures were in
circulation for that fork (26 / 30 / 34, 27 / 31 / 35, 28 / 32 / 36) and measurement finds all three
correct and simultaneous, describing four different quantities between them: see §7.4's resolution
table. ⚠️ **27 / 31 / 35 is the one to watch, because it is the rendered content box *and* the
natural border box** — naming the box does not disambiguate it on its own, the floor condition has to
be named too.

⚠️ **One clause above is right about the value and wrong about the cause, and the cause is what gets
reused.** "A one-line row's content box is `--row-h`" reads as *the floor sets it*. **Before the fix
it did not**: forcing `--row-h: 100px` moved every row to border box 100 px, so the `min-height` was
live — but setting `min-height: 0` left a one-line row **unchanged**, because the natural height sat
1 px over the floor. The floor was live and **slack**; it never bound, and the content box equalled
`--row-h` by **arithmetic accident** rather than by construction.

✅ **After `440e92d` the derivation is true of the BORDER box, on both forks, at all three
densities** — natural border box 27 / 31 / 35 against a floor of 28 / 32 / 36, so `min-height: 0`
now moves the row and the floor is what sets the rendered height. The clause is therefore no longer
wrong about the cause; it is wrong about the **box**. The content box is `--row-h − 1`, because the
row carries a 1 px `border-bottom` and no padding of its own. **Keep the value, name the box, and
state whether the floor binds** — those three together are what the section has been getting wrong
one at a time.

🚩 **The `--row-h: 100px` guard is only meaningful if it is applied to the right element, and the
first firing of it here was not.** `List.svelte` stamps `data-density` on the list container and the
density blocks match a bare `[data-density]` as well as `:root`, so the container re-declares
`--row-h` on itself and an inline override on `<html>` never reaches the rows — the measurement does
not move and reads as a clean null. **Fired both ways to be sure it was the element and not the
property: on the list the rows go to border box 100 px / content box 99 px on both forks at all three
densities; on `<html>` they stay at 28 / 32 / 36 px border box and the table still computes
`--row-h: 28px`.** Override on the list, not on the root. 🔗 **And note where that shadowing comes
from — it is mitigation 2 below working as designed.** List-scoping the density attribute is the
change that takes a 5,000-row density toggle from 107 ms to 80 ms; a measurement probe that has not
noticed it is overriding a token the list re-declares one level down is measuring the wrong cascade.

This is why the shipped expression —
`contain-intrinsic-size: auto calc(2 * var(--row-pad-y) + var(--row-lines, 1.1) * var(--leading-base))`
— is correct with the cell padding *added into* the placeholder rather than subtracted from it:
at compact density that is 2 × 4 px + 1.1 × 18 px ≈ 27.8 px, and the quantity it must stand in for is
the **content box**, measured post-fix at **27 px** — not `--row-h: 28px`, which is the post-fix
**border** box. **Corrections
(a) and (b) are untouched and still hold**, and so does the "measured content-box height per row
shape" rule they lead to.

**2. The three mitigations are not peers, and two of them are an order of magnitude apart.** The ADR
lists them flat:

> Three mitigations are available before any redesign: `table-layout: fixed` (never set anywhere)
> halves it for free; scoping the density attribute to the list container rather than `:root` bounds
> the invalidation; and an explicit 150 ms "applying" state on a density change is honest where a
> silent multi-second freeze is not.

**Measured, density toggle, 5,000 rows, shipped primitive, all four combinations:**

| Containment | Density attribute scope | Density toggle |
|---|---|---|
| on | list-scoped | **80 ms** |
| on | `:root` | **107 ms** |
| off | list-scoped | **701 ms** |
| off | `:root` | **911 ms** |

**Containment buys ~88 %** (911 → 107 ms at `:root` scope; 701 → 80 ms list-scoped). **Scoping buys
~25 %** (911 → 701 ms uncontained; 107 → 80 ms contained), **and at 1,000 rows scoping buys nothing
measurable at all.** Read the list accordingly: **containment is the mitigation**, scoping trims the
remainder once containment has already done the work, and the 150 ms "applying" state is an honesty
measure that makes nothing faster. `table-layout: fixed` is **moot on the shipped primitive** — the
rows are `display: grid`, so there is no table layout left to fix; the 1.5–5× it was bracketed with
was always the containment path's, and containment is what delivered it.

⚠️ **Note what the table does not say.** Contained and `:root`-scoped — the shipped configuration —
is **107 ms at 5,000 rows**, still over Tier 0's 100 ms hard fail. The mitigations do not on their
own make an unbounded list safe; the page size in §3 is what does, and it is doing real work rather
than belt-and-braces.

> ⚠️ **Amended 2026-08-17 — the threshold in that paragraph is the borrowed one.** Against the
> *Controls* budget the shipped configuration's **107 ms at 5,000 rows is comfortably inside 400 ms**,
> so this paragraph's specific claim no longer holds. ✅ **Its conclusion survives on a different
> footing and is the reason it is kept**: 5,000 rows is far outside anything "Load more" produces at
> a 200-row page, the mitigations still do not make an *unbounded* list safe, and the page size is
> still what does the work. See the 2026-08-17 amendment at the top of this ADR.

**3. The page size is 200 rows, and this is the arithmetic.** The extrapolation being sharpened:

> 🔍 **The escalation threshold this settles is a DOM-row ceiling in the hundreds, not the tens of
> thousands — inference, with the extrapolation shown.** The measured cost is 0.15–0.26 ms per row
> for a density change on desktop … which puts the 100 ms Tier-0 hard fail at roughly **100–300 rows
> in the DOM** as the markup stands, or **300–600** with `table-layout: fixed` and a working
> containment path

Measured on the shipped primitive, the density-toggle cost curve is **0.0146 ms/row + 6.4 ms fixed**,
which puts the 100 ms Tier-0 hard fail at **≈ 6,400 rows on desktop**. The **worst** per-row cost
observed across row shapes and densities is **0.214 ms/row**, i.e. a desktop hard fail at **≈ 500
rows** — inside this ADR's own **300–600** band for the containment path, reached by a separate run
on a separate harness. 🔍 **At the 3–5× Pi-5 factor that worst case is 100–167 rows on reference
hardware**, and the shipped curve at the same factor is roughly 930–1,840 rows.

> **The default page size is 200 rows**, appended one page per "Load more" press. Against the shipped
> cost curve that is **roughly six presses of headroom** on Pi-class hardware before the density
> toggle reaches Tier 0's 100 ms hard fail.

⚠️ **Two limits on that number, both stated rather than smoothed over.**

- **The linear fit is good only to a few thousand rows.** The 25,000-row data point is
  **superlinear**, so ≈ 6,400 rows is an extrapolation past the range the fit was taken over and is
  **not** a ceiling anyone should build against. The operative number is the page size; the
  extrapolated ceiling is context for it.
- **On the worst-case row shape, one 200-row page is already at the Pi-class limit** (200 against
  100–167). Heavy row shapes — the six-line services row, not the one-line search row — therefore
  carry the residual risk of this page size, and the required `make bench` line above is what settles
  whether such a list needs a smaller page. That is left to the measurement, not decided here.

> ⚠️ **Amended 2026-08-17 — every figure in §3 divides by a budget this ADR was not entitled to.**
> The toggles are not Tier 0; §7.2's *Controls* budget is **400 ms**, so each ceiling above scales by
> 4×: **≈ 6,400 → ≈ 27,000 rows** desktop on the curve, **≈ 500 → ≈ 1,870** desktop on the worst-case
> row, **100–167 → 374–623** 🔍 Pi-class on that worst case, and *"roughly six presses of headroom"*
> becomes roughly twenty-five or more. **The page size stays at 200** and no per-row cost changed —
> only the numerator. ⚠️ **The second bullet above is weakened, not withdrawn**: at 374–623 a
> 200-row page is no longer *"already at the Pi-class limit"*, it is at roughly half of it, and heavy
> row shapes still carry the least headroom on this page size. ⚠️ **The first bullet's warning gets
> stronger, not weaker** — ≈ 27,000 rows is further past the linear fit's range than ≈ 6,400 was, so
> it is even less usable as a ceiling. **And §3's arithmetic is now superseded for the shipped page
> size by a direct measurement of it — 75.7 ms at 200 rows** — in the 2026-08-17 amendment at the top
> of this ADR, which is where the current numbers live.

**4. The `<tr>` finding was independently reproduced; nothing is amended.** A separate run on a
separate harness reproduces both halves exactly: containment on `<div>` rows moves the container's
`scrollHeight` by **761,316 px** against a wrong placeholder, and moves it by **exactly zero** on
table rows. The blocker, the spec citation and the grid-row primitive all stand as written. **This is
recorded rather than left silent**, because a reproduced finding that nobody writes down is
indistinguishable from one nobody re-checked.

**5. Two gaps in the benchmark, recorded here rather than left as folklore.**

- **Arrow-key traversal at 25,000 rows was unmeasured at the time of writing**, because Chromium
  exhausted memory building that many rich rows — **2.7 GB resident at 5,000 rows** — so the harness
  could not reach the 25k point that the required `make bench` line above asks for.
- **What would close it:** a lower-memory row shape for the 25k point specifically, so the row count
  the required line names can be reached without the traversal case OOMing first.

  ✅ **Status, updated:** the harness is on `main` — `web/package.json` declares `bench:list`
  (`web/scripts/list-bench.mjs`), so `pnpm bench:list` is a command anyone can run from a clone. It
  carries the 25,000-row traversal case (`--quick` skips it, and the `--quick` subset stops at
  5,000). **What that changes is "unmeasured", not the memory ceiling itself:** whether the full run
  clears it is an observation from running it on the machine in hand, not a claim this ADR can make
  for every machine, so the row-shape mitigation above stays recorded rather than struck.

**A sixth measurement is the design thread's to record, not this one's**, and is named here in one
clause only because it would trip an assertion this ADR owns: `contain-intrinsic-size: auto`
remembers the size it measured at the *previous* density, so a node-reusing list can show a
measurably wrong scrollbar immediately after a density switch — which is the < 2 % drift gate below,
and the *"relying on `auto`'s remembered-size behaviour for the rest"* clause it guards. The finding,
its number and its fix belong to the design documents and are not restated here.

### Context

Three places in the repository committed to virtualization: §4.5's ~200-row threshold, §16's "library
grid, virtualized" line item, and — the one that matters — **ADR-0003, an accepted ADR, which rejects
HTMX partly on the grounds that "a 10k-item virtualized poster grid with instant client-side
filter/sort *is* a rich client-state problem"**. None of the three costed what virtualization takes
away, and the ~200 figure had no measurement behind it.

### Decision

> **The default list renderer is keyset-paginated "Load more" plus `content-visibility: auto` with
> `contain-intrinsic-size`, over rows that are `display: grid` elements carrying explicit ARIA
> table roles — never `<tr>`, because containment is inert on an internal table box.
> Virtualization is an escalation above a threshold set from an actual benchmark — and that
> benchmark is a required task, not a number chosen now, and it measures the density and theme
> toggles rather than scrolling.**

### Consequences

- 🚩 **`content-visibility: auto` has no effect on a `<tr>`, so the list primitive is part of this
  decision rather than an implementation detail left to whoever writes the first table.** The
  decision sentence originally named no element, and every list this design specified was an HTML
  `<table>` — §17.2's Block C is *"ONE unified table across all types"*, §17.4's six search groups
  are *"the same table the other five types use"* — which meant the accepted default renderer did
  not exist for any list in the product.

  **Primary source, CSS Containment Module Level 2** (<https://drafts.csswg.org/css-contain-2/>,
  fetched 2026-08-16): *"giving an element **size containment** has no effect if any of the
  following are true: … if its principal box is an **internal table box**"*, with the same
  exclusion stated for **layout containment** and **paint containment** *"other than table-cell"*.
  `content-visibility` is defined entirely in terms of those three containments, and a `<tr>` is
  `display: table-row` — an internal table box, and not a table-cell.

  **Measured, Chromium, 5,000 rows with `content-visibility: auto` and
  `contain-intrinsic-size: auto 28px`:** document height **120,000 px with the declaration and
  120,000 px without — identical**, against the 140,000 px a working placeholder would produce;
  the computed value reads back as `auto`, so it is applied and then ignored. The same test on
  `<div>` rows yields the expected 185,000 px, i.e. **the mechanism works, just not on a table
  row**. `<tbody>` is also an internal table box, so chunking does not rescue it; `<td>` *can* take
  containment and collapses the cells to 9 px — visible corruption. There is no element inside a
  `<table>` that `content-visibility` can usefully sit on.

  **So a UsArr list row is a `display: grid` element with `role="table"` on the container,
  `role="row"` on each row, `role="columnheader"` on the header cells and `role="cell"` on the
  rest.** The responsive stacking fork below 760 px already builds half of this, so this is
  convergence rather than a new component. **The accessibility cost is real and is now a required
  component test, not a review item**: an ARIA grid must carry by hand every association a native
  `<table>` gives for free — the roles above, header-to-cell association,
  `aria-rowcount`/`aria-colcount` where the rendered set is a window onto a larger one, and column
  names that survive the stacked view where the header row is not rendered.

  **And `make bench` asserts it rather than assuming it:** set `content-visibility: auto` on a row
  and assert the container's `scrollHeight` **differs** from the uncontained case. One assertion,
  and it would have caught this.
- **Ctrl+F keeps working, and it is *a* reason rather than *the* reason.** The WICG virtual-scroller
  explainer states the problem exactly: *"accessible landmark navigation, find in page, or
  intra-page anchor navigation are based solely on DOM structure, and virtualized content is by
  definition not in the DOM"*. `content-visibility: auto` skips rendering without removing content —
  MDN: *"the skipped contents must still be available as normal to user-agent features such as
  find-in-page, tab order navigation, etc."* (Baseline September 2024).

  **But Ctrl+F cannot carry the decision on its own, and the honest reason is recorded here rather
  than left to be discovered.** §4.5 in the same breath specifies *"keyset windows of ~100,
  prefetching ±2 pages"*, so find-in-page searches roughly 300 of a 50,000-track music library —
  and reports **not found**, not "partial". A find that silently answers "no" about an album you own
  is worse than no find, because the user believes it. UsArr also already ships a strictly better
  tool for that job: **Tier 1, the client-side prefix index over the whole top-level corpus, bound
  to `/`, budgeted at < 5 ms p50, which §8.2 calls *"where the instant feeling comes from"***. It
  beats Ctrl+F on coverage, on ranking and on the six-type case specifically.

  **The grounds the decision actually stands on**, unchanged in conclusion: a keyset window keeps
  the mounted set small either way, so virtualization buys little; it is a dependency and a
  scroll-restoration liability; and it breaks `aria-setsize`/`aria-posinset` unless set by hand and
  text selection across the list. Ctrl+F over the *loaded* window is a genuine convenience and is
  kept as a secondary benefit. The other cited losses each have a one-line answer — set the ARIA
  attributes; the design already restores scroll by row **key** rather than pixel offset (§7.3 rule
  5) — and are listed as costs of virtualization rather than as decisive.
- **ADR-0003's conclusion is untouched; one of its arguments is corrected.** A 10k-item grid with
  instant client-side filter and sort is still a rich client-state problem — the client-side prefix
  index (§4.5) is the proof — so the HTMX rejection stands on its own. The word "virtualized" in that
  sentence is no longer accurate and should be read as "large".
- **The two positions are closer than they look.** With ~100-row keyset windows and ±2 pages
  prefetched the mounted set is small either way; what differs is whether unmounted rows are absent
  from the DOM or present-but-unpainted.
- 🚩 **`make bench` gains a required line, and the earlier version of that line measured the wrong
  operation.** It specified *"frame time and scrollbar drift … while scrolling"*. **Scrolling is
  free: 0.1–0.3 ms at every size, measured.** The two operations that are O(all loaded rows) are the
  **density toggle** and the **theme toggle**, because each sets an attribute on `<html>` and
  invalidates every element that reads a custom property:

  | Rows in DOM | Density toggle, as shipped | With `table-layout: fixed` | Theme toggle |
  |---|---|---|---|
  | 1,000 | **153 ms** | 115 ms | — |
  | 5,000 | **1,199 ms** | 547 ms | — |
  | 25,000 | **6,508 ms** | 2,956 ms | **1,356 ms** (search rows) – **4,514 ms** (request rows) |

  Desktop x86 Chromium, real markup, mean of four changes. **Both are top-bar controls present on
  every screen and both are pure-local no-data interactions, so both are Tier 0 by
  `design/DESIGN-DIRECTION.md` §7.2's own definition — whose hard fail is 100 ms.**
  ⚠️ **Superseded by the 2026-08-17 amendment at the top of this ADR: §7.2 had no such definition,
  and the toggles are governed by its *Controls* budget — hard fail 400 ms — not by Tier 0.** The
  premise either side of that clause is untouched; only the tier and the number are wrong. The required
  line therefore measures, at 1k / 5k / 25k rows, in both themes and at all three densities, on the
  §13 reference hardware: **density-toggle wall clock · theme-toggle wall clock · filter and sort
  wall clock · scroll frame time · scrollbar drift**, the last as
  `|scrollHeight after full scroll − scrollHeight at load| / scrollHeight < 2%`. Frame time alone
  cannot detect drift, and no scroll test sees the first three at all.

  🔍 **The escalation threshold this settles is a DOM-row ceiling in the hundreds, not the tens of
  thousands — inference, with the extrapolation shown.** The measured cost is 0.15–0.26 ms per row
  for a density change on desktop; a Pi 5 is conservatively 3–5× slower at style recalculation and
  layout, which puts the 100 ms Tier-0 hard fail at roughly **100–300 rows in the DOM** as the
  markup stands, or **300–600** with `table-layout: fixed` and a working containment path (which
  cut the same operation by 1.5–5× in an isolated `div`-row test). ⚠️ **Superseded by §3 of the
  amendment at the top of this ADR**, which replaces this extrapolation with a measured cost curve and
  states the limit on it — the 25,000-row point is superlinear, so the linear fit is good only to a
  few thousand rows. **The ceiling is set by the
  density control, not by scrolling**, and the earlier framing implied 25,000 rows was the number
  in question. Choosing a replacement threshold here from judgement is still refused; what changed
  is *which operation* the benchmark points at. Three mitigations are available before any redesign:
  `table-layout: fixed` (never set anywhere) halves it for free; scoping the density attribute to
  the list container rather than `:root` bounds the invalidation; and an explicit 150 ms "applying"
  state on a density change is honest where a silent multi-second freeze is not.
- ✅ **`contain-intrinsic-size` has measured values, and the "no value yet" caveat this ADR carried
  is withdrawn.** 📏 **Measured by the frontend thread's `pnpm bench:list` — not by the thread that
  wrote this ADR, which did not run them.**

  | Row shape | compact | standard | relaxed |
  |---|---|---|---|
  | one-line row | **28 px** | **32 px** | **36 px** |
  | rich row (two lines, sub-line or thumbnail) | **45 px** | **49 px** | **53 px** |

  ⚠️ **THE ONE-LINE ROW OF THIS TABLE IS A TRAP, AND IT IS THE REASON THE AMENDMENT AT THE TOP OF
  THIS ADR EXISTS.** Read as **border box** — which is what the next sentence calls them — it was
  **wrong** when it was written and is **right** now, because it described the `two-line` fork whose
  border box was 29 / 33 / 37 px before `440e92d` and is 28 / 32 / 36 px after. The digits did not
  move; the fix moved underneath them. Post-fix, and on **both** forks, 28 / 32 / 36 px is the
  **border** box and 27 / 31 / 35 px is the **content** box. The rich row is unaffected and is
  **border box** 45 / 49 / 53 px, **content box** 44 / 48 / 52 px, on both trees. See the amendment
  for all four numbers of all six configurations.

  ⚠️ **The rich-row pair is a MODE, not a single height, and the distinction is load-bearing.** The
  rich row is bimodal — rows with more chips wrap — splitting at compact into content box 44 px ×
  1,308 rows and 48 px × 692 over the frontend thread's 2,000-row measurement, so the **mean** content
  box is 45.4 / 49.4 / 53.4. `45 / 49 / 53` is therefore the modal *border* box **and** the mean
  *content* box, and `RELEASE_ROW_INTRINSIC`'s `45 / 49 / 53` is the latter — correct as it stands.
  Applying the one-line correction pattern to it (border box, subtract one, 44 / 48 / 52) swaps a
  correct mean for a mode and is a regression. Name the statistic wherever the population is not
  uniform.

  Drift over a full scroll at the one-line values is **0.76 / 0.70 / 0.65%** against the 2% budget.
  These are rendered **border-box** heights; the declaration sizes the **content** box, so what ships
  is the derived expression the mockups already use —
  `auto calc(2 * var(--row-py) + var(--row-lines) * var(--lh-base))` — which tracks the density token
  instead of hard-coding three constants (§1 of the amendment above quotes the shipped app's form of
  it). Verified against the mockups rather than assumed: they render exactly 28 / 32 / 36 and
  45 / 49 / 53, with computed placeholders within 0.3% of the border-box height each stands in for.

  ✅ **Re-measured and confirmed, after the harness that produced them was found never to have
  loaded IBM Plex** — its Vite root declared no `publicDir`, so `@font-face` URLs 404'd for the
  harness's whole life. At 2,000 rows against the real `List.svelte` and `app.css`, with the face
  verified by canvas advance-width probe, **all six are byte-identical with the face served and with
  it blocked**. The reason is that `body { line-height: var(--leading-base) }` is a fixed 18 px
  *length* rather than a unitless multiplier, so glyph metrics cannot move the line box. The null
  result is load-bearing rather than lucky because the guard was fired deliberately: forcing
  `line-height: normal` *does* split the conditions (rich rows 43 / 47 / 51 served against
  39 / 43 / 47 blocked). ⚠️ **Measured on one list configuration** — `stack: 'two-line'`, the shape
  this ADR was originally measured against. ✅ **Both forks are measured now, and the `26 / 30 / 34 px`
  this bullet reported for a `stack: 'labels'` list is confirmed with its box named at last: it is the
  NATURAL CONTENT box — `min-height` forced to `0` — and the "below the floor, where `min-height`
  *would* bind" that follows it is correct.** That list's *rendered* heights are border box
  28 / 32 / 36 px and content box 27 / 31 / 35 px, unchanged across `440e92d`. See the amendment.
  **The same run's other results — the 761,316 px containment confirmation against a Δ of exactly 0
  on `display: table-row`, the ~88% / ~25% mitigation split, and the superlinear 25,000-row point that
  limits the linear fit to a few thousand rows — are recorded in the amendment above rather than
  restated here.**
- 🚩 **`contain-intrinsic-size: auto` remembers a size measured at the *previous* density — a live
  correctness bug, reported by the frontend thread's bench as new and apparently undocumented
  upstream.** This is the measurement the amendment above names in one clause and hands to the design
  documents; `design/DESIGN-DIRECTION.md` §7.4 owns it, and it is stated here only because it guards
  this ADR's own *"relying on `auto`'s remembered-size behaviour for the rest"*. A keyed `{#each}`
  reuses row nodes, so after a compact → relaxed switch the reused rows
  carry the remembered compact size and **the scrollbar is 14.57% wrong, against 0.65% when the same
  rows are rebuilt rather than reused**. Both preconditions are ordinary: density is a first-class
  control on every screen and the lists are keyed by row id. **The rule is required, not advisory:
  when row height changes, the remembered intrinsic size must be invalidated** — rebuild the affected
  nodes (key on `id + density`, or `{#key density}` around the list body) or force re-measurement of
  every mounted row before the next paint.

  ⚠️ **Not enforced yet, and it cannot be enforced by `docs/design/check.mjs`:** the condition needs
  node reuse plus thousands of rows, and that check's target is static HTML with neither, so an
  assertion there could not fire and would pass for ever. Enforcement belongs to `pnpm bench:list`,
  **fail above 2% drift** — the drift budget above, not a second number. Sequencing is fix, then
  assert, then claim: `bench:list` exits non-zero on a full run today because of a 25,000-row
  Chromium out-of-memory. If the app target later mounts large lists, moving the assertion into
  `check.mjs` is small and uncontested. `design/DESIGN-DIRECTION.md` §7.4 carries the full statement.
- **The value this ADR previously prescribed was wrong three ways**, kept because each is a way to
  arrive at a wrong placeholder again. The prescription was `contain-intrinsic-size: auto var(--row-h)`:
  1. **`--row-h` is inert on the element it describes.** `.tbl tbody tr { min-height: var(--row-h) }`
     — `min-height` does not apply to `display: table-row`. Measured: forcing `--row-h: 100px`
     leaves the row at **28.0 px**, with `getComputedStyle(tr).minHeight === "28px"`. Density works
     only through `--row-py` padding, so the ADR derived the placeholder from the one property with
     no effect on the real height. **The grid-row primitive above also fixes this**, since
     `min-height` does apply to a grid item.
  2. **It is off by ~50% even used correctly**, because rows are not one height: measured on the
     search screen at compact density there are **six distinct row heights — 28, 30, 45, 47, 59,
     62 px, mean 42.0** — and eighteen across the three densities, not the "28 / 32 / 36 plus three
     more" this ADR assumed. At the mean, estimating 25,000 rows at 28 px understates scroll height
     by ~350,000 px (33%), resolving progressively as the user scrolls: exactly the drift named.
  3. **`contain-intrinsic-size` sizes the *content* box**, so padding and border are added on top —
     a 24 px row with `auto 28px` produced a **37 px** placeholder (28 + 8 + 1).

  **What ships instead: `contain-intrinsic-size: auto <measured content-box height>` per row
  shape**, relying on `auto`'s remembered-size behaviour for the rest — **plus the invalidation rule
  above wherever row height can change, because that remembered size is the bug** — with the < 2%
  drift assertion as the gate. The measurement now exists (first bullet in this group), so this is an
  implementable rule rather than a direction.
- §16's "virtualized" line item is amended to match.

### Alternatives rejected

- **Keeping "virtualize over ~200 rows".** An unmeasured threshold buying a functional regression in
  the one interaction a library browser exists for.
- **Infinite scroll.** NN/g finds it *"can be downright harmful to usability — in particular, for
  search results"*; Baymard measured "Load more" plus lazy loading as the best-performing pattern.
  UsArr is a retrieval tool, not a discovery feed.
- **Picking a new threshold now, from judgement.** Rejected on principle: the finding against the old
  rule was that it had no measurement, and answering it with a different unmeasured number would
  concede the argument while pretending to fix it.

---

<a id="adr-0030"></a>
## ADR-0030 — `work.kind` gains `comic_issue`; manga is not a separate kind

**Status:** Accepted · **Refines [ADR-0009](#adr-0009)** (`work`/`edition`/`file`), in the same way
ADR-0009 already resolved `audiobook`. **Migration 0001 is the only cheap moment to make this
change.**

### Context

`work.kind` was `movie, series, season, episode, artist, album, track, book, comic, game`. **Every
other multi-level medium got its levels; comics got one member.** Worse, the two readings of that
member contradict each other: `work_comic`'s columns (`issue_number`, `volume`) describe an **issue**,
while the search corpus rule, the Tier 1 prefix index, the `kind_byte` map and the grid all treat
`comic` as a **top-level** work. That is the `audiobook` contradiction again, for the same reason —
nobody has had to implement it yet.

### Decision

> **`work.kind` gains `comic_issue`.** `comic` is the series — top-level, in the grid, in the search
> corpus, `kind_byte` 9. `comic_issue` is the issue or chapter — a child, excluded from the corpus,
> with its own `kind_byte` allocated **in the same commit**. `work_comic` splits into a series-level
> table (`volume_label`, `volume_year`, `reading_direction`, `publisher`, `total_issues_declared`,
> `total_issues_source`) and `work_comic_issue` (`number_text TEXT`, `number_sort REAL`,
> `volume_label`, `volume_sort`, `is_special`, `is_oneshot`, `special_version`, `page_count`).
>
> **There is no third level for Kavita's Volume, and there is no `manga` kind.**

### Consequences

- **Doing it now costs one line. Doing it later costs a CHECK-constraint change (a SQLite table
  rebuild), an FTS re-index, a rebuild of every client-side prefix index, and a change to the
  `kind_byte` codec — which ARCHITECTURE §5.3 states is "unchangeable once clients cache ids".** That
  asymmetry is the entire argument for the timing.
- **The alternative readings both fail.** One kind means either losing issues entirely — no "43 of
  60", no per-issue availability, no read progress, no readlists — or putting series and issues in one
  kind and distinguishing on `parent_work_id IS NULL`, which breaks §8.2's corpus rule, because that
  rule filters on `kind`: every chapter title would enter the FTS corpus and a large manga library
  would swamp every query, which is the 400k-episode failure the rule exists to prevent.
- **Kavita's Volume is carried as `volume_label` + `volume_sort`, a grouping attribute rather than a
  node.** Komga, Mylar3 and Kapowarr have no such level, so a third tier would be empty on four
  backends out of five and would render "Volume 1 › Chapter 1" for one of them over the same files.
  **This is a deliberate loss of fidelity against Kavita and is written down as one.**
- **Issue numbers are `number_text TEXT` plus `number_sort REAL`**, never an integer. Real issue
  numbers are `1.MU`, `-1`, `0`, `Annual 1`, `1A`. Komga models a string plus a float sort key;
  Kavita models min/max floats plus a string plus a range. Any integer column is wrong.
- **Scanlations become the `edition` layer's best case** — five translations of one chapter are five
  `edition` rows on one `comic_issue` work, `label` = group, `language`, `published_at` each. ⚠️ With
  the caveat that **no backend reports the group**: it lives in the filename. So UsArr records them as
  separate works, surfaces the condition as *"5 files report chapter 112"*, and leaves the seam in
  place. It does not guess.
- **Two shapes stay unmodelled, deliberately.** A TPB is its own `comic_issue` with
  `special_version='tpb'`, and UsArr **does not model which issues it collects** — no backend reports
  it and no metadata standard carries it, so inferring it from number ranges in titles is the
  false-positive machine ADR-0007 already refuses. Variant covers are not modelled at all: two files
  become two works, which is wrong but *visibly* wrong, and that beats a silent dedupe that hides a
  file the user owns.

### Why manga is not a second kind

1. **Neither library server models them as different types.** Komga has no manga/comic distinction at
   all — only `ReadingDirection.{RIGHT_TO_LEFT, WEBTOON}`, a *display* property. Kavita distinguishes
   them only through `LibraryType`, whose members `Manga`, `Comic (Flexible)` and `Comic (ComicVine)`
   are **filename-parsing modes over one identical entity tree**.
2. **ComicInfo carries `Manga` as a field** — `{Unknown, No, Yes, YesAndRightToLeft}` — on a book
   otherwise structurally identical to a western comic.
3. **§6.4's "never auto-merge across `kind`" would become a liability.** A series in Komga
   (undifferentiated) and in a Kavita Manga library would land in two kinds and could never be
   merged — a permanent duplicate in the grid, which is the exact bug the kind rule exists to prevent,
   inverted.
4. **The kind enum is the most expensive thing in the schema to change.** Spending a member on a
   distinction the backends do not make is a bad trade.

The real differences live where they already cost nothing: `work_comic.reading_direction`;
`external_id.source` (AniList / MAL / MangaBaka / MangaUpdates for manga, Comic Vine / Metron / GCD
for western comics); routing capabilities; and **a derived, undeletable `type:manga` system tag**, sourced in priority order from Kavita
`LibraryType ∈ {Manga, LightNovel}` → Komga `readingDirection ∈ {RIGHT_TO_LEFT, VERTICAL, WEBTOON}` →
an AniList/MAL/MangaBaka id being present → ComicInfo `Manga`. That gives a Manga filter, a Manga
home row and manga-aware sorting at zero schema cost, and it **degrades honestly** — a left-to-right
manga in Komga simply is not tagged, and the user can tag it themselves, which they cannot do to a
`kind`.

> **The Newznab category was on that list and is removed, because §8.5 denies the distinction it was
> offered as evidence for.** There is no manga category in the Newznab standard at all:
> `7030 Books/Comics` is the only comics category anywhere in the tree, `7000` is its **parent**
> rather than its sibling, and Nyaa — the dominant public manga tracker — maps its Literature
> categories to `7000`, so a search filtered on `7030` returns **zero manga**. §8.5's own rule is
> that a comics-and-manga search filters on `7000` and uses `7030` only as a ranking signal, i.e.
> both categories are used for **both** kinds of comic. "7000 vs 7030" was therefore a
> manga-vs-western-comic difference that the section it cited explicitly says does not exist. The
> four remaining items carry the argument on their own.

### Alternatives rejected

- **Leaving `work.kind` as it was and deciding later.** Refused on the cost asymmetry above.
- **A `manga` kind.** Above.
- **A third level for Volume.** Above.
- **Modelling variant covers or TPB containment.** No source of truth exists for either.

---

<a id="adr-0031"></a>
## ADR-0031 — Track position is edition-scoped; attribution is many-to-many

**Status:** Accepted · **Refines [ADR-0009](#adr-0009)** · **refined by [ADR-0033](#adr-0033)**,
which gives `work_credit` a `person` kind to point at and renames its creator column. Both changes
below are **migration 0001 or they are backfills over the largest tables in the schema.**

### Context

ARCHITECTURE §6.1 said *"`work_track` carries `disc_number` and `track_number`"*, hanging both off a
`work`. MusicBrainz's own definitions make that wrong: a **recording** is *"distinct audio"*, while a
**track** is *"the way a recording is represented on a particular release (or, more exactly, on a
particular medium)"*. The same recording is track 4 on the original CD and track 6 on the 2017 deluxe
reissue, with a different track MBID each time.

Separately, Lidarr's `AlbumResource.artistId` is singular, so an album belongs to exactly one artist
and a Various-Artists compilation lands under a synthetic "Various Artists". Navidrome went the other
way, superseding its single `ArtistID`/`AlbumArtistID` fields with a `Participants` model.

### Decision

> **1. `work_track` gains `edition_id`**, keyed `(work_id, edition_id)`. Position is a property of the
> recording-in-an-edition, not of the recording.
> **2. `work_track.track_number` is `TEXT`**, with a derived `track_position INTEGER` sort key.
> **3. Artist attribution is an M:N `work_credit(work_id, artist_work_id, role, position,
> credited_as)`.** There is no `artist_id` column on an album.
> *(Amended by [ADR-0033](#adr-0033): the column is `creator_work_id`, because for books and comics
> it points at a `person` work rather than an `artist` one. The M:N decision is unchanged.)*
> **4. `edition` gains `narrators`, `duration_seconds` and `abridged`** for audiobooks.
> **5. The availability rollup is edition-keyed for music**, which makes the `availability` blob
> polymorphic across media — worked shapes per medium in
> [`reference/schema.md`](./reference/schema.md) §1.

### Consequences

- **The seam ships in migration 0001; the multi-edition UI does not.** `edition_id` costs eight bytes
  a row now and a backfill over the largest table later. v0.x can model only the active edition, which
  is what Lidarr does.
- **`getAlbum` must order tracks correctly or the entire OpenSubsonic surface is broken**, which
  ARCHITECTURE already flags as a dependency. `TEXT` plus an integer sort key is what makes a double
  LP with `A1`, `A2`, `B1`, `B2` sort as a human expects; Lidarr ships exactly this pair.
- **The track title lives on `work_track`, not only on the track `work`.** Two releases can carry the
  same recording under different titles — "Idioteque" versus "Idioteque (Album Version)". The
  recording MBID is the identity; the title is edition-local display text.
- **`work_credit` is what makes VA compilations, collaborations and classical roles representable**,
  and it is needed for books too, where role matters: author, translator, editor, illustrator.
  Inheriting Lidarr's scalar limitation would buy nothing.
- **The audiobook columns are edition properties on every authority that models them** — Chaptarr's
  `EditionResource` (`Narrator`, `NarratorNames[]`, `DurationSeconds`, `ChapterCount`), Audiobookshelf
  (`Book.narrators`, `Book.duration`, `Book.abridged`), Audnexus. They are not work properties
  (different productions have different narrators) and not `media_file` properties (a 30-file
  audiobook has one runtime).
- **The rollup shape `{tier: {have, total}}` assumed `total` is a property of the parent work.** For
  albums it is a property of the *edition*: choosing the 2017 remaster over the 2000 original changes
  the track list, the count and the durations. Render the edition label beside the fraction, or the
  fraction is a guess — especially since Lidarr's own matcher weights media format at 1.0 against
  recording MBID at 10.0, so the "active release" is often not the release the files came from.
  **This was a normative requirement stated in two documents with no representation in the one file
  an implementer codes from**, so the blob now carries a `"k"` discriminator and one worked example
  per medium — `k:"tier"` for video, `k:"edition"` for music, `k:"count"` with `total_source` and
  contiguity for comics — beside the column in [`reference/schema.md`](./reference/schema.md) §1.
  Without a discriminator a renderer cannot tell a tier key from an edition key in the same object,
  and `getAlbum`'s correctness depends on it.
- ⚠️ **The remaster argument in ARCHITECTURE §6.1 is weaker than it read, and this rollup rests on
  it.** MusicBrainz defines no "remaster"; the quoted phrase is from the *Recording* page, and the
  step to "therefore a new edition of the same work" is UsArr's inference. More importantly the
  common real-world case takes a different path: a reissue with bonus discs and a changed title
  (*"OK Computer OKNOTOK 1997 2017"*) gets **its own release group**, which makes it a different
  album work joined by a `work_relation` edge rather than an edition of the same one. **Both paths
  must exist**, and the edition-keyed rollup describes only the first.
- **`edition.format` carries the medium, never the codec.** A 2000 UK CD release can be on disk as
  FLAC. 🔍 That separation is inference from the two models rather than a cited rule, and it is the
  one place `format` is easy to overload.
- **Two artist-level numbers must never be rendered bare.** Lidarr's `albumCount` counts albums that
  passed a *metadata profile*, which is a user setting: Radiohead has 386 primary-type albums on
  MusicBrainz, so "you have 12 of 579" is true and useless. The only defensible form names the
  profile — *"12 of 14 studio albums (Lidarr metadata profile: Studio only)"*.

### Alternatives rejected

- **Keeping `work_track` work-scoped and modelling only the active edition** (Lidarr's model). It is
  the cheaper v0.x behaviour and it is what the UI will do at first — but baking it into the schema
  makes switching editions rewrite track rows, and the retrofit is the expensive half.
- **`track_number INTEGER`.** Sorts vinyl randomly.
- **`artist_id` on the album row.** Makes VA compilations unrepresentable the moment it is written.

---

<a id="adr-0032"></a>
## ADR-0032 — Read-only catalogue sources move early; command sinks defer

**Status:** Accepted · **Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16**, which is authoritative
for scope. The README's generated status tables follow §16. · 🚩 **One member is reversed by
[ADR-0035](#adr-0035): Kavita is the comics-and-books source and Komga follows it**, because the
owner runs Kavita. · 🚩 **The milestone is re-sequenced by [ADR-0036](#adr-0036): no catalogue source
ships in v0.1.** All four still arrive, one at a time after v0.1. **This is re-sequencing, not
rejection** — the deferral of the command sinks, the honest costing, and every argument below stand
unchanged; only *when* the catalogue sources land moves. Read this ADR through both amendments.

### Context

The owner's scope moved from two media types to six: audiobooks, ebooks, comics/manga and music join
movies and TV. §16 previously put **every** non-video service in v1.0 — Lidarr, Kavita, Calibre-Web,
Audiobookshelf, Komga, Jellyfin, all in one "Breadth" bucket. Taken literally that means five of the
six media types are empty screens until the last milestone, so the product's one-sentence claim is a
claim about Sonarr and Radarr. Taken the other way — move it all forward — v0.1 acquires four
catalogue adapters *and* four write paths, and the project's stated biggest risk is never shipping.

### Decision

> **Add the read-only *catalogue sources* — Navidrome, Audiobookshelf and Komga — to v0.1, and
> **Kavita to v0.2**, which is the payment. Defer the *command sinks* — Lidarr, LazyLibrarian,
> Mylar3, Kapowarr — to v1.0.**

### The argument, since this is the "cut before you add" answer

**What enters, and what it actually costs.** An earlier revision of this ADR said all four sources
were *"the same shape … with no write path, no state machine and no new subsystem"* and priced them
as *"the marginal cost of a provider adapter, four times"*. **That was wrong in four ways this
repository's own documents contradict**, and the honest price is: **three hand-written Tier 0 Go
adapters, three auth schemes, one token lifecycle, three hierarchies, and one new delta channel they
all share.**

1. **They are Tier 0, not manifests.** ARCHITECTURE §11.2's manifest list covers Komga,
   Audiobookshelf and Kavita — but the manifest *tier* does not exist until v0.3, which ships
   LazyLibrarian as the **first** Tier 1 manifest, and Navidrome is excluded from the tier by
   §11.2's own rule against session establishment. §11.2 also states that **a manifest may never
   write a strong identity**, so even in v0.3 the manifest route cannot supply the identity a
   six-type catalogue needs. Every one of these is Go code in the milestone it ships in.
2. **Auth is three schemes and one of them expires.** Navidrome: `POST /auth/login` yields a JWT
   **plus** a `(subsonicSalt, subsonicToken)` pair. Audiobookshelf: a Bearer JWT **or** a scoped API
   key **with an expiry**. Komga: a static `X-API-Key` (Basic only on OPDS). The \*Arr machinery is
   a static header credential with **no lifecycle at all**, so login round trips, refresh and
   credential *expiry* are three new failure modes and at least one new Services state — *"this key
   expired"* — that no document previously named.
3. **None has a changed-since endpoint**, so they need **channel 3b** (ARCHITECTURE §7.1a): an
   ordered page-walk delta with a watermark, an ordering guarantee, an overlap window and a stated
   reconciliation-only fallback. That is a new subsystem, and it is now specified rather than
   assumed. Without it the replica would be up to 24 hours stale for the majority of v0.1's
   services, which does not prove the replica thesis.
4. **What genuinely is shared, and this half of the original claim holds:** `RemoteItem`, the
   provider registry, the circuit breaker, the two-phase import, the read path and the
   reconciliation sweep are all reused unchanged from Sonarr and Radarr.

And they remain exactly what makes "everything in one place" true rather than aspirational.

**What defers, and why.** A write path is per-service and expensive — routing, capability probing, an
idempotent verb mapping, queue verification semantics, and one bespoke failure mode each. The bespoke
failures are documented, not hypothetical: **Lidarr writes `artist.status = 'deleted'` into its own
database when its metadata server returns 404**, and exposes no health signal for the subsystem that
causes it, so a correct adapter needs that guard on day one or it will silently tombstone real
artists; **LazyLibrarian returns HTTP 200 with `Success: false`**; **Mylar3** has no spec, no
pagination and no delta, and has shipped no release since 2025-08-17; **Kapowarr's** API documentation
reads *"Coming Soon"*.

**And deferring them defers convenience, not capability** — **for four of the six types**: Prowlarr
free-text search-and-grab covers requesting for all six in v0.1 (books at `7020`, audiobooks at
`3030`, comics and manga at `7000`, music at `3000`). ⚠️ **That claim is materially weaker for music
and books and this ADR previously stated it unqualified.** ✅ **403 of Prowlarr's 543 shipped indexer
definitions are `type: private`**, and the trackers that actually carry music and books — Redacted,
Orpheus, DICMusic, Libble, MyAnonaMouse, Bibliotik — are **invite-only**. For a user who holds those
invites the coverage is real; for one who does not, deferring Lidarr defers *capability*, not
convenience. It is a positioning claim rather than an implementation detail, so it is **carried as an
open question rather than resolved here** — REVIEW-LOG R4 item 3, which was raised, recorded as not
the agent's to settle, and then walked past by an ADR marked Accepted. It is now a named consequence
below.

**What has to move out to pay for it: Kavita, to v0.2.** The previous answer was *"nothing is cut,
and one thing is capped"*, and the capped thing was the library correction **UI** — which the next
sentence of the same paragraph argued has no work to do in v0.1, because tier 1 resolves essentially
100% of the identity problem for Sonarr and Radarr. **A cap on a declared no-op is not a payment**,
and `CLAUDE.md`'s rule carries no exemption clause.

**Kavita earns the cut on evidence already gathered, and no media type is lost.** Komga covers
comics, so **all six media types still have a catalogue source in v0.1** — Sonarr and Radarr for film
and TV, Navidrome for music, Audiobookshelf for ebooks and audiobooks, Komga for comics. Against
that, Kavita is the weakest of the four on every axis that matters here:

- **Its identifier fields are behind a paid subscription**, so on a free instance every external id
  is null. It contributes the **least identity value** and the **most honest-gap UI** per adapter.
- **Its Series → Volume → Chapter → File hierarchy is the deepest of the four**, and it is the one
  ADR-0030 already flattens deliberately — Volume becomes `volume_label` + `volume_sort`, a grouping
  attribute rather than a node, and that is recorded as a deliberate loss of fidelity *against
  Kavita specifically*.
- **It has no catalogue delta at all.** `sortByLastModified` exists on `GET /api/Collection` and
  `POST /api/ReadingList/lists` and on **none** of the Series, Volume or Chapter endpoints, so it is
  the one source channel 3b cannot serve and it would ship reconciliation-only.

**What is kept, with the remaining cost stated rather than argued away.** The libraries subsystem and
the auto-proposal flow stay in v0.1, because they are what makes the six-type claim *usable*: without
them a six-type install is one undifferentiated grid, and the Ebooks/Audiobooks split over one
Audiobookshelf library — the demonstrable improvement over upstream's own organisation — is the whole
demonstration. The cost that remains is real and is not "nearly free": **four tables in migration
0001, materialised membership with a 250 ms dirty-flush and a denormalised sort key, a derivation
with five container predicates, an auto-proposal engine with join-vs-create defaults, and a second
first-class settings screen.** It is true that the Libraries screen *replaces* hard-coded per-type
sections rather than adding a screen; it is not true that the tables, the derivation, the proposal
engine or the `search_doc` scoping change replace anything. The correction **UI** is still capped to
v0.3, and that cap is now correctly described as a scheduling detail rather than as the payment.

### Consequences

> 🚩 **Every "v0.1" in the consequences below is re-sequenced by [ADR-0036](#adr-0036).** v0.1
> carries **no** catalogue source; the four sequence after it, one at a time. The consequences
> themselves all still hold — they attach to the milestone each source lands in rather than to v0.1.
> Left as written, per this file's convention, rather than rewritten in place.

- **v0.1** carries six media types, read-only, over **three** catalogue sources plus Sonarr and
  Radarr, with Prowlarr as the single request path for all of them, and says so honestly on every
  screen where a sink is absent. **Four of the six types have no sink in v0.1** — music, audiobooks,
  ebooks and comics — and the UI states that per library rather than implying otherwise.
- **v0.4's success criterion becomes reachable.** It requires a *populated* music replica before the
  OpenSubsonic surface exists; as previously written, that milestone contained both a new southbound
  adapter and a new northbound protocol. 🔍 This is a scheduling observation, not a verified fact.
- **v0.2 gains Kavita** as the fourth catalogue source, alongside the request model.
- **v1.0 keeps the write paths**, plus Calibre as a Tier 0 `metadata.db` adapter and the OPDS surface.
- **`work_merge` moves forward** out of v1.0 to the milestone that ships music, because MBIDs and
  OLIDs are redirect-capable and upstream renaming a key is a tier-1 problem, not a fuzzy-tier one.
- **Five honest gaps arrive with the sources and must be surfaced rather than hidden.**
  **(1)** None of the four has a "changed since" endpoint, so delta is **channel 3b** — an ordered
  page-walk, specified in ARCHITECTURE §7.1a rather than left implicit — and the reconciliation
  sweep is doing more work here than it does for the \*Arrs, because a page walk **structurally
  cannot observe a deletion**. **(2)** ⚠️ Whether Komga even accepts `sort=lastModified,desc` on its
  series list **could not be verified from the spec and needs a live probe**, because the whole Komga
  delta strategy rests on it: Spring `Pageable` sort properties are not enumerated and the DTO field
  name may not be the entity property name. **§16 now makes this a day-one spike, before the schema
  is written, and if the probe fails Komga drops to reconciliation-only and its Services row says
  so.** **(3)** Komga supplies **no external identifiers at all**, so a Komga-only library cannot be
  resolved above ~0.85 confidence. **(4)** Kavita's metadata is behind **a paid subscription**, so on
  a free Kavita every external id is null — one of the three reasons it is the source that pays for
  this amendment. **(5)** Navidrome's only delta signal is `getScanStatus.lastScan` as a change
  *trigger* plus an `updated_at`-ordered page walk, and it is the one source whose auth is a login
  round trip rather than a static header.
- ⚠️ **The "convenience, not capability" claim is weakest exactly where v0.1 has no sink.** For music
  and books the indexers that carry the content are predominantly private and invite-only — 403 of
  543 Prowlarr definitions are `type: private`, and the dedicated music trackers (Redacted, Orpheus,
  DICMusic, Libble) are all invite-only. For a user without those invites, deferring Lidarr defers
  capability. **Open: REVIEW-LOG R4 item 3.** This ADR is Accepted with that consequence named rather
  than with the question quietly closed; it is a positioning call for the owner, and it does not
  change the decision, because the alternative on offer is shipping the four write paths this
  milestone exists to avoid.

### Alternatives rejected

- **Leaving everything in v1.0.** Five of six types are empty until the last milestone.
- **Moving the sinks forward with the sources.** Four write paths, four bespoke failure modes, into
  the milestone whose one job is proving the replica thesis.
- **Cutting Navidrome as well as Kavita**, which was the reviewer's preferred payment on the grounds
  that its v0.1 placement rests on an explicitly-marked 🔍 inference about v0.4 scheduling — the
  weakest justification in the amendment — and that it brings the one credential lifecycle furthest
  from anything v0.1 otherwise builds. **Rejected because it costs a media type and Kavita does
  not.** Komga covers comics with Kavita gone; nothing covers music with Navidrome gone, so cutting
  it takes v0.1 from six types to five and makes the amendment's own headline claim false. The
  inference about v0.4 scheduling is a reason Navidrome is *convenient* in v0.1; the reason it is
  *necessary* is that it is the only music catalogue in the ecosystem UsArr reads.
- **Deferring the libraries subsystem to v0.2** and shipping one implicit auto-derived library per
  instance with no screen. A real candidate, and it is refused on what it removes: the
  Ebooks/Audiobooks split over one Audiobookshelf library is the concrete demonstration that UsArr's
  organisation can be better than upstream's, and it is unreachable without the screen and the
  proposal flow. The tables would ship either way, so the saving is the UI and the derivation —
  smaller than it looks, and it costs the amendment its best argument.
- **Moving Search-and-Grab out to v0.2 to make room.** It was the least load-bearing candidate on
  paper, and it is the *only* request path for four of the six media types under this decision.
  Cutting it would remove the thing that makes the deferral of the sinks affordable.

---

<a id="adr-0033"></a>

## ADR-0033 — `work.kind` gains `person`; a credit is not a music artist

**Status:** Accepted · **owner-decided 2026-08-16** (REVIEW-LOG round-4 §4.6 item 6, closed) ·
**Refines [ADR-0009](#adr-0009)** and **[ADR-0031](#adr-0031)**. **Migration 0001 is the only cheap
moment to make this change**, for exactly the reasons [ADR-0030](#adr-0030) gives.

### Context

[ADR-0031](#adr-0031) made attribution many-to-many: `work_credit(work_id, artist_work_id, role,
position, credited_as)`, with a `role` CHECK list that spans three media —
`primary`/`featured`/`composer`/`conductor`/`performer`/`remixer`/`producer` for music,
`author`/`translator`/`editor`/`illustrator`/`narrator` for books, and
`writer`/`penciller`/`inker`/`colorist`/`letterer`/`cover_artist` for comics. Its own words: *"it is
needed for books too, where role matters."*

**But `artist_work_id` points at a `work`, and `work.kind` had no `person`, `author` or `creator`
member.** So a book's author and a comic's penciller were stored as **`artist`-kind works** — the
same kind Navidrome and Lidarr music artists use. This was found by the round-4 disposition pass
rather than by a reviewer, recorded in `reference/schema.md` §1.1 with its cost, and **left
deliberately unresolved** on the grounds that a `kind_byte` allocation is a decision of the same size
as ADR-0030 and should be made by the owner rather than folded into a review pass. It was escalated
as REVIEW-LOG round-4 §4.6 item 6. The owner decided it on 2026-08-16.

Two consequences made it a live defect rather than a modelling preference, and neither is
speculative:

1. **Every author lands in the Music navigation type.** ARCHITECTURE §17.2's media-type enum is
   `kind`-driven and maps `('artist'|'album', *)` → **Music**. A Frank Herbert stored as an `artist`
   work is therefore a Music item, on the sidebar, in Block A's counts and in the Music search group.
2. **Every credit consumes the client prefix index's byte budget.** §4.5's Tier 1 index is over
   top-level kinds, `artist` among them, hard-capped at 25,000 items — and §13's six-type reference
   library already sits at 27,500, i.e. over the cap before a single author is counted. Adding
   ~6,000 credited people takes the overshoot from **10% to 34%**, on the mechanism the whole
   perceived-speed story rests on.

### Decision

> **`work.kind` gains `person`**, with **`kind_byte` 13** allocated in the same commit as
> `comic_issue`'s 12 and before any client caches an id.
>
> **`person` is excluded from the media-type navigation enum (ARCHITECTURE §17.2), from the Tier 1
> client prefix index (§4.5) and from the FTS corpus (§8.2).** It is not a thing the user browses;
> it is reachable as a credit link on an item (§17.6).
>
> **`work_credit.artist_work_id` is renamed `creator_work_id`** and points at a work of kind
> `artist` **or** `person`. **The rule: `artist` when a connected service models the creator as a
> top-level catalogue entity in its own right — a Navidrome or Lidarr artist, which has albums, a
> page and a library row — and `person` in every other case** (authors, translators, editors,
> illustrators, comic writers and artists, narrators reported only as a string).

### Consequences

- **Doing it now costs one CHECK member, one byte allocation and one column name. Doing it later
  costs a CHECK-constraint change (a SQLite table rebuild), an FTS re-index, a rebuild of every
  client-side prefix index, and a change to the `kind_byte` codec — which ARCHITECTURE §5.3 states is
  *"unchangeable once clients cache ids"*.** That asymmetry is the entire argument for the timing,
  and it is ADR-0030's argument reappearing at a second member of the same enum.
- **`artist` keeps its meaning and keeps the Music type.** This is not a rename of `artist` to
  `person`; it is a second kind beside it. Music artists are genuinely top-level — they have their
  own page in every music tool in the ecosystem, they carry albums as children, and §17.2 maps them
  to a navigation type. Collapsing both into `person` would cost Music its top level and would make
  `getIndexes`/`getArtists` (§16, v0.4) unimplementable without a second predicate.
- **`kind_byte` encodes the *remote* kind, not `work.kind`**, and this ADR is the occasion to write
  that down. The map in `reference/gateway.md` §3 already carries `author` (byte 10, LazyLibrarian's
  own remote kind) and `file` (11), neither of which is a `work.kind`. Remote `author` keeps 10;
  `person` takes 13 for services that report a creator entity under some other name. **Both resolve
  to `work.kind = 'person'`.**
- ⚠️ **"Find everything by this author" is unanswered in v0.1, and that is stated rather than
  implied.** Excluding `person` from the FTS corpus means a person is not a search result. The cheap
  candidate is to fold credited names into the FTS `alt_titles` of the works they are credited on, so
  the query returns the *books* — but that is a decision for whoever writes the FTS document builder,
  it is not specified anywhere, and nothing may assume it. Adding `person` to the corpus later is a
  predicate change plus a re-index; it is not a migration, which is why it can wait and the kind
  cannot.
- 🔍 **The artist-versus-person rule is inference from how the sources model their own data**, not a
  citation. Navidrome and Lidarr expose artists as first-class entities with their own endpoints;
  Audiobookshelf, Komga and Kavita expose authors and comic creators as strings or as tags on an
  item. If a future source exposes authors as first-class browsable entities — a post-Readarr book
  manager doing what Lidarr does — the rule sends them to `artist` and the Music type would be wrong,
  so **the rule is worth re-reading when a book *catalogue* source with an author endpoint is
  added**. It does not change the schema; it changes which kind the adapter writes.
- **A human who is both a music artist and a book author is two `work` rows in v0.1**, joined by
  nothing. That is a real loss, it is smaller than filing every novelist under Music, and the seam
  for fixing it already exists: `work_relation` carries typed edges with `confidence` and `evidence`
  (ADR-0009), so a `same_person` edge is an added edge type rather than a schema change.
- **§13's reference library gains a `person` row (~6,000)** and its top-level total stays 27,500
  rather than becoming ~33,500. 🔍 The figure is chosen, not measured, and it is the one row in that
  table where being wrong by 2× changes no budget, because it is excluded from every budget it could
  have pressed on.

### Alternatives rejected

- **Leave authors as `artist` works and filter them out of the Music type at query time.** This was
  the shape that existed. It needs a predicate that distinguishes them, and there is none —
  `work.kind` is the only thing the enum, the corpus rule and the prefix-index builder read, so the
  filter would have to be "does this artist have any album children", which is a join on the hot
  navigation path and is wrong for a music artist whose albums have not imported yet.
- **A separate `person` *table* rather than a `work.kind` member.** It avoids the `kind_byte`
  allocation, and it costs the thing `work_credit` was built on: `creator_work_id REFERENCES
  work(id)` becomes a polymorphic reference with no foreign key, `work_relation` cannot carry a
  `same_person` edge, and an author gains no `external_id` rows (OLIDs and Comic Vine person ids are
  real and useful) without duplicating that table too. The `work` table already models "an abstract
  entity with a title, identifiers and typed edges", which is what a person is here.
- **Rename `artist` to `person` and give music artists a subtype.** Symmetrical and cheap in the
  schema, expensive everywhere else: it renames the one kind that already appears in a navigation
  type, in the v0.4 OpenSubsonic surface, in `Caps.MediaKinds` and in every research document, to buy
  nothing the two-member answer does not.
- **Defer the decision to v0.2 and store credits as strings until then.** A string credit cannot
  carry an `external_id`, cannot be deduplicated across two sources, and cannot be the target of the
  reverse index that makes "everything this person is credited on" a seek. Upgrading strings to rows
  later is a backfill over `work_credit` *plus* the migration this ADR exists to avoid.

---

<a id="adr-0034"></a>

## ADR-0034 — The project keeps the name UsArr

**Status:** Accepted · **owner-decided 2026-08-16.** In the owner's words: *"Let's Keep Usarr. Its
the easiest and the thing my brain first went to."* This is a naming decision only. **No schema, wire
format, identifier, configuration key or credential derivation changes as a result of it** — that is
the point of it.

### Context

A rename to **Usharr** was considered, on the usher metaphor: the service ushers a request to
whichever part of the stack can satisfy it. It reads well, and it describes what UsArr does better
than a contraction of "us" and "\*Arr" does. So the name was collision-checked before anything else
was touched. Every check below was run on **2026-08-16** against GitHub's API and HTML, Docker Hub's
v2 registry API, npm, PyPI and pkg.go.dev.

**`Usharr` is already taken twice, inside this exact ecosystem.**

- <https://github.com/jaysoffian/usharr> — created 2026-05-07, last pushed 2026-06-04, **not
  archived**, 0 stars, Python/FastAPI, MIT. Its README opens: *"Usharr is a web application and API
  that complements Plex Media Server, Sonarr, Radarr, Bazarr and Tautulli."* Actively developed, and
  in adjacent conceptual territory with the same audience. **This is the material collision** — not a
  name clash in a distant field, a name clash in the neighbourhood UsArr ships into.
- <https://github.com/nicholasodonnell/usharr> — a Radarr/Tautulli movie-library pruner, TypeScript,
  GPL-3.0, 11 stars, last pushed 2026-05-09, **archived 2026-06-30**. Archived, but its image
  <https://hub.docker.com/r/nicholasodonnell/usharr> is live at **2,883 pulls** and currently owns
  the "usharr" result a self-hoster gets when they search. A repository stops being maintained when
  it is archived; it does not stop being the first hit.

`github.com/Usharr` is held by an unrelated personal account, so the organisation handle is
unavailable too. The Docker Hub namespaces `usharr` and `usarr` are both free.

⚠️ **The trademark registers could not be reached from the research environment** — Justia returned
403 and the USPTO TESS endpoint 404. That check is **unverified, not clean**, and it is unverified
for *every* name here, UsArr included. It is recorded as an open gap rather than quietly counted as a
pass.

**`UsArr` itself has zero software collisions.** A GitHub search for `usarr` returns only noise from
R's built-in `USArrests` dataset — no repository, package, image or handle contends for it.

**The cost side was measured, not estimated.** A rename touches **1,581 occurrences across 144
files**, in exactly three casings — `UsArr`, `USARR`, `usarr` — while three threads were concurrently
pushing to `main`.

### Decision

> **The project is called UsArr.** The name is not changing.
>
> The usher framing survives as prose, not as an identifier: *"UsArr ushers your requests to the
> right service"* is available as a README or docs tagline whenever it is wanted, at zero cost,
> because a tagline is not a module path.

### Consequences

- **The rename buys nothing.** A rename is worth paying for when the incumbent name collides, and
  UsArr does not collide. `Usherr`'s only advantage over `UsArr` was the metaphor — and the metaphor
  is separable from the name, so it can be had without paying for the rename.
- **Nothing in the codebase moves.** The module path, the image name, the `USARR_*` environment
  prefix, the `usarr_session` / `usarr_csrf` cookies, the HKDF info labels and the wire-visible
  `usarr_id` all stay exactly as they are. This ADR changes no behaviour and no bytes.
- **The trademark question stays open**, for UsArr as for every candidate screened. If it ever needs
  answering it must be answered against a register that actually responded, and nothing in this ADR
  may be cited as evidence that it was.

### Alternatives rejected

| Candidate | Finding on 2026-08-16 |
|---|---|
| `Usherr` | **The cleanest alternative.** Its only collision is a dormant, out-of-domain 2-star Next.js project last pushed 2022 (<https://github.com/iPanchalShubham/Usherr>); npm, PyPI, pkg.go.dev and the Docker Hub `usherr` namespace are all free. The `github.com/usherr` organisation handle is held by a dormant zero-repo account. Rejected on cost, not on availability |
| `Usherarr` | Clean everywhere, organisation handle included — and clumsy to type and clumsy to say |
| `Ushrr` | Clean on every registry, and unspellable. It reads as a typo of the taken `Usharr`, which is the worst of both |
| `Ushr` | **Blocked.** The Docker Hub `ushr` namespace is an active organisation registered to the US House of Representatives, and the GitHub username is taken |
| `Routarr`, `Portarr`, `Guidarr`, `Conductorr`, `Dispatcharr` | Already taken inside the \*Arr ecosystem. <https://github.com/Dispatcharr/Dispatcharr> is at 3,839 stars and active |

### If this is reopened

A future rename is not a find-and-replace, and these are the things it must not break. They are
recorded here so the next attempt does not rediscover them the expensive way.

- ⚠️ **The HKDF info labels `usarr/kek/v1`, `usarr/stream-token/v1` and `usarr/client-credential/v1`
  in `internal/crypto/derive.go` are cryptographic domain-separation inputs**, bound into stored
  ciphertext. Renaming them silently makes **every stored encrypted \*Arr API key undecryptable**, and
  **no test catches it** — `derive_test.go` carries no golden vectors. A rename either leaves these
  labels alone or ships a re-encryption migration.
- **The cookie names `usarr_session` and `usarr_csrf`** span `internal/httpapi/auth.go` and
  `web/src/lib/api.ts`. They must change in lockstep or every write 403s.
- **The Go module path only resolves if the GitHub repository is renamed in the same move.**
- **The scene-release fixture tag `Test.Release.2026.1080p.WEB-DL.x264-USARR` is a fabricated
  release-group name, not the product name.** It must not be swept up.
- **The wire-visible `usarr_id` and the OpenSubsonic response envelope's `type` field are protocol
  decisions** (ADR-0021, ADR-0010), not cosmetic strings. Changing them is a compatibility break,
  decided on its own merits.
- **A single case-insensitive find-and-replace is wrong**: it destroys the `USARR_*` environment
  variable casing. The three casings are distinct and each has its own call sites.

---

<a id="adr-0035"></a>

## ADR-0035 — Kavita, not Komga, is the comics-and-books catalogue source

**Status:** Accepted · **owner-decided 2026-08-16** · **Reverses one member of
[ADR-0032](#adr-0032)**, whose shape is otherwise untouched ·
**[`ARCHITECTURE.md`](./ARCHITECTURE.md) §16** remains authoritative for scope ·
**re-examines [ADR-0030](#adr-0030)** and confirms it. · 🚩 **The milestone move the amendment below
records is decided in [ADR-0036](#adr-0036)**, which carries its alternatives, its consequences and
the §16 rewrite; this ADR restates itself against it and remains the record of *which* source. ·
⚠️ **Amended 2026-08-17 by [ADR-0041](#adr-0041)** — see the flag below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0041](#adr-0041): Kavita is in v0.1, so every "not in v0.1" rider
> below is falsified — and the falsifying evidence is this ADR's own spike.** §2a ran on 2026-08-17
> against the owner's live Kavita and passed; ADR-0036's v0.1 criterion (*"a real Sonarr and a real
> Radarr, imported"*) turned out unmeetable because **the owner runs neither** (*"thats gonna have to
> be future"*, 2026-08-17); so ADR-0041 kept ADR-0036's rule — one source, proven on real data,
> before a second adapter — and changed the source to **Kavita**. **Two sites below are superseded**:
> the **2026-08-16 amendment**, whose framing (*"v0.1 has no catalogue sources at all"*, *"v0.1 is
> Sonarr, Radarr and Prowlarr"*, its clause 1 instruction to read every "v0.1" below as *"the
> milestone Kavita lands in"*, and its clause 3) is overtaken — Kavita's milestone **is** v0.1; and
> **§1's rider**, flagged again at the site. **Nothing about *which* source is chosen moves**: §1's
> identity finding, §2/§2a's probe and §3's confirmation of ADR-0030 all stand, and §1 is now
> exercised in v0.1 rather than deferred with it. ARCHITECTURE §16 and §16.1 carry the sequence.
>
> 🚩 **The 2026-08-16 amendment below is left standing and unedited**, per this file's preamble: it
> is a dated record of what was decided that day, not a description of the design today.

### ⚠️ Amendment, 2026-08-16 — the milestone moved under this ADR, and the choice inside it did not

**This ADR was written while the catalogue sources were inside v0.1, and they are not any more.** The
owner delegated the provider count — *"I'm fine with starting small… we can start with 2 services or
5"* — and the call taken is that **v0.1 has no catalogue sources at all**: v0.1 is **Sonarr, Radarr
and Prowlarr**, the \*Arr library sync proves the replica thesis on real data first, and the
catalogue sources — **Navidrome, Audiobookshelf, Kavita** — then arrive **one at a time, after
v0.1**. §16 is authoritative for which milestone each lands in, and a status note on ADR-0032 is
owed there rather than here.

**What that does and does not do to this ADR.** Every judgement below survives; only the milestone
label on them moves. Specifically:

1. **The choice of Kavita over Komga stands unchanged.** It is a choice about *which* comics-and-books
   source is built, made on the ground that Kavita is the install the owner actually runs. That ground
   is untouched by when it is built. **Read every "v0.1" in the text below as "the milestone Kavita
   lands in".**
2. **§2's spike is not wasted work, and its branch now orders a different sequence.** It no longer
   decides a build order *inside* v0.1 — there is no catalogue source in v0.1 to order. It decides the
   order of the **post-v0.1 sequence**: Kavita first if its watermark works, **Navidrome first if it
   does not**. The reasoning is unchanged and the Navidrome branch gets stronger, because de-risking
   v0.4's OpenSubsonic target early matters more when the sequence is serial and one-at-a-time.
   It is no longer a *day-one* spike; it runs before the first catalogue adapter is written.
3. **§1's rendering requirement stands and its milestone moves with it.** §17 and the mockups must
   render free-Kavita's null identifiers as the *normal* case **from the milestone Kavita lands in** —
   not in v0.1, which draws no comics library at all because it has no comics source.
4. **§3's confirmation of ADR-0030 is untouched**, and its "more expensive now that the adapter is
   v0.1" argument weakens to "expensive whenever the adapter lands", which does not change the verdict:
   a `volume` `work.kind` in migration 0001 for a single adapter is the wrong shape at any milestone.

### Context

The owner, in his own words: *"actually atm my books and comics/manga are managed by kavita."*

**ADR-0032 had the right shape and exactly one wrong member.** It paid for four early catalogue
sources by cutting one, and it cut **Kavita**, keeping Komga in v0.1. The reasoning was sound in the
abstract: Kavita's external-identifier fields are gated behind a paid subscription so it contributes
the least identity value; its Series → Volume → Chapter → MangaFile hierarchy is the deepest of the
four; and it has no catalogue delta on its Series endpoints. Every one of those observations is still
true and none of them is retracted here.

**They were the wrong criteria.** They optimise for the cleaner API rather than for the install that
exists. **The owner's stack is now confirmed directly: Navidrome for music, Audiobookshelf for
audiobooks, Kavita for books, comics and manga, on x86-64 under Proxmox.** With that on the table,
ADR-0032's other two catalogue sources map to services he actually runs and its comics source does
not — so building a Komga adapter first means building an adapter for a service nobody on this
project can point at a real library. That is the wrong order, and it is a *smaller* correction than
it sounds: one member of a four-item list changes place.

### Decision

> **Kavita replaces Komga as v0.1's comics-and-books catalogue source, because it is the install the
> owner actually runs. Komga moves to v0.2, taking Kavita's former place.**
>
> **ADR-0032's payment is preserved exactly** — one of the two still comes out, the count of v0.1
> catalogue sources is unchanged at three (Navidrome, Audiobookshelf, Kavita) plus Sonarr and Radarr,
> and **no media type loses its v0.1 source**: Kavita covers comics and manga, and covers books as
> well.
>
> **The build *order* is not fixed by this ADR. It is decided by the watermark spike in §2 below**,
> whose result is falsifiable and whose two branches are written down in advance. ⚠️ Per the
> amendment above, that order is now the order of the **post-v0.1 catalogue sequence**, not an order
> inside v0.1.

### 1. Identity gets weaker, and it is the default path rather than an edge case

**Kavita's `aniListId`, `malId`, `comicVineId` and the rest are null without Kavita+.** ADR-0032
already recorded this as one of the three reasons Kavita was the thing cut. Under this ADR it becomes
**what an ordinary user sees once Kavita lands**, not a documented edge case.

That state was already designed — the *"identifier fields are null because they are behind a paid
tier"* case — and the change is one of frequency, not of mechanism. What follows:

- **A free-Kavita comics library resolves at a lower identity tier than a Komga one would have.**
  ⚠️ Note the honest comparison: ADR-0032's own consequence (3) records that **Komga supplies no
  external identifiers at all**, so Komga was not the stronger option here either. Free Kavita and
  Komga are both title-and-metadata matching; **paid Kavita is strictly better than both**. The
  identity loss against ADR-0032's plan is therefore near zero, and the honest statement is that
  **comics has no strong-identity path in v0.1 under either choice**.
- **`ARCHITECTURE.md` §17 and the mockups must render this as the normal case — from the milestone
  Kavita lands in.** ~~⚠️ Not in v0.1, which draws no comics or books library at all, because it has no
  catalogue source for either.~~ 🚩 **STRUCK 2026-08-17 by [ADR-0041](#adr-0041), and struck rather
  than deleted because it is the clause the amendment at the top of this ADR exists for.** The
  milestone Kavita lands in **is v0.1**, so this requirement is v0.1's and is exercised there rather
  than deferred: free Kavita's null identifier fields make *"not identified"* v0.1's **ordinary**
  case (ARCHITECTURE §16.0, §16.1). The rest of this bullet is unaffected and gains force. The "not identified"
  badge and the comics gap list are not exception states on a comics library; they are what the
  screen looks like. A design that treats them as edge cases will under-serve the majority path.
- **It must never read as a defect in UsArr, and it must never read as nagware.** The screen says
  what is missing and why — the identifier fields are a paid Kavita feature and the instance has not
  supplied them — and stops there.

### 2. The delta spike changes target, and its criterion is written down before it runs

ADR-0032 funded a day-one probe of **Komga's** `sort=lastModified,desc`, on which the whole channel
3b strategy for that source rested. **That probe is retargeted to Kavita, and it is a harder
question.**

**✅ Verified from primary source, Kavita `main`, fetched 2026-08-16 — and the concern is real:**

| Fact | Source |
|---|---|
| `SortField.LastModifiedDate = 3` exists, documented *"Date entity was last modified (tag update, etc)"* | [`API/DTOs/Filtering/SortField.cs`](https://raw.githubusercontent.com/Kareadita/Kavita/main/API/DTOs/Filtering/SortField.cs) |
| `SortOptions { SortField, IsAscending }` is carried on `FilterV2Dto` | [`API/DTOs/Filtering/SortOptions.cs`](https://raw.githubusercontent.com/Kareadita/Kavita/main/API/DTOs/Filtering/SortOptions.cs) |
| `POST /api/Series/all-v2` takes `FilterV2Dto` in the body and `UserParams` paging in the query, and returns `SeriesDto` | [`API/Controllers/SeriesController.cs`](https://raw.githubusercontent.com/Kareadita/Kavita/main/API/Controllers/SeriesController.cs) L309-310 |
| **`SeriesDto` carries no `LastModified` / `LastModifiedDate` property at all.** Its complete date set is `LatestReadDate`, `LastChapterAdded`, `Created`, `LastFolderScanned` | [`API/DTOs/SeriesDto.cs`](https://raw.githubusercontent.com/Kareadita/Kavita/main/API/DTOs/SeriesDto.cs) |

**So Kavita can sort Series by a field it does not return.** Channel 3b's watermark is *"the last
value of the ordering key I saw on the previous page"*; if the ordering key is absent from every row,
**there is no watermark to carry and the page walk cannot resume** — it can only ever re-walk from
page 1, which is the reconciliation sweep with extra steps.

**Two candidate keys are both sortable and present in the DTO**, and they are what the spike is
actually testing:

| Ordering key | `SortField` | In `SeriesDto` | What it observes | What it misses |
|---|---|---|---|---|
| `LastModifiedDate` | `3` | ❌ **no** | everything, including metadata-only edits | — (unusable as a watermark) |
| `LastChapterAdded` | `4` | ✅ `LastChapterAdded` | a chapter or volume added to an existing series | a retitle, a tag edit, a cover change, a deletion |
| `CreatedDate` | `2` | ✅ `Created` | a **new** series appearing | every change to an existing series |

> **The spike, stated so its result is unambiguous.** Against a live Kavita, call
> `POST /api/Series/all-v2` with `SortOptions.SortField = LastChapterAdded, IsAscending = false` and
> `UserParams` paging. **It passes if and only if all three hold: (a)** the response is ordered by
> the `lastChapterAdded` value the rows themselves carry, **(b)** that ordering is stable enough to
> resume from — re-requesting with a filter at the last seen value returns the expected suffix, not a
> shuffled page — and **(c)** adding a chapter to an existing series moves it to the front on the
> next call. **Any one of the three failing means Kavita has no usable channel-3b watermark**, and
> the same three questions are then asked of `CreatedDate` as a narrower fallback that would detect
> new series only.

**And the spike decides the build order, which is why it is day one:**

- **If Kavita has a usable watermark** → build Kavita first *of the catalogue sources*. It is the owner's install, it is the
  source with the most types riding on it (books, comics, manga), and it is the one that can be
  tested against real data from the first commit.
- **If Kavita has no usable watermark** → **build Navidrome first.** Kavita then becomes the
  *hardest* of the three rather than the easiest — reconciliation-only, with the channel-3b
  fallback path exercised before the channel-3b happy path exists — and Navidrome de-risks two things
  at once, because it is also the service **v0.4's OpenSubsonic surface** is written against. Kavita
  still ships either way; only the order moves. ⚠️ Both branches order the **post-v0.1** sequence.

### 2a. ⚠️ Dated result — the spike was RUN, 2026-08-17, against the owner's live Kavita

**Ran by:** Joe, against his own instance. **Kavita 0.9.0.2 · 151 series · page size 10.** The
criterion above is applied clause by clause, in its own words, and nothing is graded on a curve.

| Clause | Verdict | What was observed |
|---|---|---|
| **(a)** ordered by the `lastChapterAdded` the rows themselves carry | ✅ **PASS** | all 10 rows of page 1 non-increasing on `lastChapterAddedUtc` |
| **(b)** stable enough to resume from | ✅ **PASS** | page-1 boundary value ≥ page-2 first row, **no id overlap** between the pages, and page 1 **byte-identical** across two fetches |
| **(c)** adding a chapter moves the series to the front | ⚠️ **INCONCLUSIVE live** (it needs a library change), **settled from source instead** | `UpdateLastChapterAdded()` has exactly **one** production call site — `Kavita.Services/Scanner/ProcessSeries.cs:769`, inside the `if (chapter == null)` new-chapter branch. So the field moves on a **chapter add** and does **not** move on an edit, a deletion, a retitle or a cover change |

**Verdict: the criterion is met, and Kavita has a usable channel-3b watermark** — with the loss
clause (c)'s source reading makes explicit rather than hypothetical: this watermark observes chapter
*additions* only, exactly as the key's own table row above says, so metadata edits, deletions and
retitles are reconciliation's business and not the delta channel's.

🚩 **ONE QUALIFICATION, AND IT CHANGES THE MECHANISM RATHER THAN THE VERDICT.** Clause (b) is
written as *"re-requesting with a filter at the last seen value"*, and **that is not expressible**:
`SeriesFilterField` has **no timestamp member**, so there is no server-side "since" filter to send.
Resumption is therefore a **sorted page walk with a client-side stop** — walk `LastChapterAdded`
descending and stop at the first row at or below the stored watermark. What was measured (stable
order, no overlap, repeatable page 1) is exactly what that mechanism needs, which is why the verdict
stands; but any text that says UsArr *filters* Kavita at the watermark is wrong, including
[`ARCHITECTURE.md`](./ARCHITECTURE.md) §7.1a's channel-3b wording, corrected in the same change as
this note.

ℹ️ **And the reason §7.1a's overlap window is not optional, from the owner's real data:** the
timestamps cluster on the **scan job's** clock, not on the moment each chapter appeared — three
series land within microseconds of each other at `07:00:30`. A walk that resumes *exactly* at the
watermark can therefore drop a sibling that shares the boundary timestamp, which is what the overlap
window exists to absorb.

📌 **Citation maintenance, appended rather than rewritten.** The source table in §2 above cites
`API/DTOs/Filtering/SortField.cs`, `API/DTOs/SeriesDto.cs` and friends. **Kavita's tree has since
been restructured** — `SeriesDto` is now `Kavita.Models/DTOs/SeriesDto.cs`, and the call site in
clause (c) is under `Kavita.Services/`. The 2026-08-16 citations are left exactly as they were
because a citation inside a dated record is history: they are what was read on the day, against the
tree as it then stood. Anyone re-verifying today should look under the new paths.

**This ADR deliberately does not pre-judge which branch is taken.** ⚠️ **Superseded by §2a above,
which ran it** — kept because it is the standard the result was judged against. The evidence below
says the
`LastModifiedDate` key is definitely unusable and says nothing conclusive about `LastChapterAdded` —
the ordering may be applied over the entity rather than the DTO, and that is exactly the class of
question a spec cannot answer and a probe can. ⚠️ **Recording a guess here would be the same mistake
ADR-0032 made about Komga's `sort=lastModified,desc`**, which it correctly refused to assume.

### 3. ADR-0030's flattening of Kavita's Volume still holds, and now for a better reason

[ADR-0030](#adr-0030) decided **not** to add a third `work.kind` level for Kavita's Volume, carrying
it as `volume_label` + `volume_sort` on the item instead. That decision was made while Kavita was a
v0.2 concern, so it is re-examined here rather than inherited.

**It holds, and promoting Kavita strengthens it rather than weakening it.** The reasoning:

1. **The flattening was never an argument from Kavita's *scheduling*.** ADR-0030's case is that a
   Volume is a *grouping of chapters within a series*, not an independently-identified work: it has
   no external id in any provider, it is not what a user searches for, and it is not what any other
   source in the six-type set models. None of that changes with the milestone.
2. **A third node would be the only place in the schema with a kind that one source produces.**
   Komga is two levels; Audiobookshelf, Navidrome, Sonarr and Radarr are all two or fewer below the
   top. Adding `volume` as a `work.kind` in migration 0001 — the one migration that can never be
   edited — for a single adapter is the shape `CLAUDE.md` warns about, and it would be *more*
   expensive to carry now that the adapter is v0.1 rather than less.
3. **The recorded loss of fidelity is unchanged and stays recorded.** ADR-0030 states it *against
   Kavita specifically*; that statement is now about a v0.1 source rather than a v0.2 one, which
   makes it more visible, not less true. `volume_label` and `volume_sort` are what the UI renders,
   and `LevelBar` (DESIGN-DIRECTION §9.7) already has a comics rendering.

**Verdict: confirmed and unchanged.** ⚠️ The amendment above weakens only the *milestone* half of point 2 — "more expensive now that the adapter is v0.1" becomes "expensive
whenever the adapter lands" — which does not move the verdict.

### Consequences

- **The three non-\*Arr catalogue sources are Navidrome, Audiobookshelf and Kavita**, and ⚠️ per the
  amendment above **none of them is in v0.1** — they sequence after it, one at a time. Komga follows
  Kavita, in the slot Kavita vacated. The count, the payment and the "every media type has a v0.1
  source" claim are all preserved exactly.
- **Comics and books have no strong-identity path in v0.1 on a free instance**, and the screens
  render that as the normal case. This is *not* a regression against ADR-0032, because Komga supplies
  no external identifiers at all — see §1.
- **The spike is Kavita's `LastChapterAdded` ordering, with the criterion in §2**, and its result
  sets the order of the post-v0.1 catalogue sequence. §16's Komga probe line is superseded by it,
  and the spike is no longer a day-one item — it runs before the first catalogue adapter is written.
- **v0.4's single-Navidrome assumption is now checked against a real install rather than assumed.**
  The owner runs exactly one Navidrome, which is what `reference/gateway.md`'s narrowed OpenSubsonic
  scope was written against. 🔍 It remains an assumption about *other* users' installs; it is no
  longer an assumption about the one the milestone will be demonstrated on.
- **The reference hardware is a floor, not the owner's machine.** §13 budgets against a Raspberry
  Pi 5 and several arguments extrapolate to it (the density-toggle cost in DESIGN-DIRECTION §7.4, the
  Argon2id memory argument, the streaming-import peak). **The owner runs x86-64 under Proxmox**, so
  every one of those figures is a **conservative floor** rather than a description of the machine
  this will first run on. Nothing about the budgets changes — a floor is the right thing to design
  against — but a Pi figure must not be quoted as *"what the owner will see"*.
- **No schema change.** `work.kind` is untouched (§3), `service_instance` is untouched, and the
  provider registry absorbs the swap by construction — which is the registry doing its job.

### Alternatives rejected

- **Keeping Komga in v0.1 and adding Kavita beside it.** That is "and also", not a plan: it takes the
  payment back out and puts two comics adapters into the milestone whose one job is proving the
  replica thesis.
- **Keeping ADR-0032 as written and asking the owner to install Komga.** It inverts the relationship
  between the software and the install. The product exists to sit in front of the stack a
  self-hoster already runs.
- **Editing ADR-0032 in place.** The file's convention is a superseding or refining entry with the
  reversal stated, which is what ADR-0012a and ADR-0033 both do. Editing it in place would delete the
  reasoning that made the original choice defensible, and that reasoning is still correct about
  everything except which install exists.
- **Deferring the swap until the spike has run.** Tempting, because the spike could make Kavita the
  hardest source — but the spike decides *order*, not *membership*, and membership is decided by
  which service the owner runs. Holding the ADR open would leave §16 naming a source nobody can test
  against.

---

<a id="adr-0036"></a>

## ADR-0036 — No catalogue source ships in v0.1; they arrive one at a time after it

**Status:** Accepted · **owner-decided 2026-08-16** · **Amends
[`ARCHITECTURE.md`](./ARCHITECTURE.md) §16**, which remains authoritative for scope ·
**Re-sequences [ADR-0032](#adr-0032) and [ADR-0035](#adr-0035)** · **Rejects nothing** — every source
either of them names still arrives. · ⚠️ **Amended 2026-08-17 by [ADR-0041](#adr-0041)** and
⚠️ **by [ADR-0042](#adr-0042)** — see the two flags below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0042](#adr-0042): one ground under the libraries bullet below is
> gone.** That bullet keeps the libraries subsystem in v0.1 partly because *"a library binding carries
> the request destination v0.1's write path routes on"*. **That write path — the four \*Arr verbs on
> the durable command queue — re-sequences out of v0.1** with the adapters that receive them, so the
> clause names something v0.1 no longer contains. **The bullet's conclusion is unchanged and the
> subsystem stays**, on the two grounds that never depended on it: its four tables are owed by v0.1
> either way, and its screen is one of the five `CLAUDE.md` calls essential. **The request destination
> has no v0.1 use left at all**, which is further than this decision alone would have taken it: §17.8
> establishes that **no service v0.1 connects can be a library's request sink**, so the destination
> cannot even be set and its column does not render for the milestone. Sequencing, not a cut — it
> returns with the first service that can be a destination.
> **The bullet's closing examples were already dead** by [ADR-0041](#adr-0041): an Anime
> library on a Sonarr tag and a Films library spanning two Radarrs both need services v0.1 no longer
> has, and §16.0 records the Kavita binding that replaces them.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0041](#adr-0041): the decision line *"No catalogue source ships in
> v0.1"* no longer holds, and the reason is a fact this ADR never checked.** It scoped v0.1 to the
> \*Arr library sync *"because it is the thing that proves the replica thesis on real data — a real
> Sonarr and a real Radarr, imported"*. **The owner runs neither** (his words, 2026-08-17:
> *"I don't run sonarr or radarr just yet … thats gonna have to be future"*), so that success
> criterion was unmeetable on the hardware the milestone targets. **ADR-0041 keeps this ADR's rule —
> one source, proven on real data, before a second adapter is written — and changes which source:
> Kavita ships in v0.1 as the sync core's first adapter, and Sonarr and Radarr re-sequence behind
> it.** The count stays at one, nothing is cut, and everything below about *why* a one-source
> milestone beats a three-source one is unaffected and still correct. **Two specific consequences
> below are superseded**: the channel-3b bullet's *"Nothing about it is implemented in v0.1"* (3b is
> now v0.1 work, because Kavita has no channel 3), and *"The catalogue is film and TV"* (it is books
> and comics/manga). Read ADR-0041 for both.

> **Read alongside [ADR-0035](#adr-0035)'s own amendment, which records the same call from the other
> side.** The two were written in parallel by the design and implementation threads and they agree;
> this ADR carries the decision, the alternatives and the §16 rewrite, and ADR-0035's amendment
> carries what the move does to *that* ADR's four findings clause by clause. Neither restates the
> other. Where they overlap — the owner's words, the sequence, the spike's new role — they say the
> same thing, and ADR-0035's amendment is the finer-grained account of its own consequences.

### Context

The owner was asked whether v0.1 should grow from two providers to five, and handed the decision
back: *"I'm fine with starting small, it's honestly sort of whatever to me."* That is a real answer
and it points one way. "Starting small" is the constraint; the question is what small means when two
Accepted ADRs have already argued the other direction.

**ADR-0032 moved four catalogue sources out of the v1.0 "Breadth" bucket and put three of them into
v0.1. ADR-0035 changed which three.** Neither changed the count. Both were right about the
*roadmap* — leaving five of six media types empty until the last milestone makes the product's
one-sentence claim a claim about Sonarr and Radarr — and neither asked the separate question of
whether v0.1 specifically can carry three of them at once.

**§16.0 already priced them, correctly, and then did not act on the price:** four hand-written Tier 0
Go adapters, four auth schemes, one with a lifecycle, four hierarchies, and **one entirely new sync
channel** (channel 3b, §7.1a) that no \*Arr needs. Against that, **what is actually built today is
the Prowlarr search-and-grab path** — the \*Arr *library* sync, the schema tables it writes, the
grid, the image pipeline and local search are all still design. So ADR-0032's v0.1 asked for three
adapters to be written on top of shared read machinery that has never imported anything, and for a
new delta channel to be proven at the same time as the delta channel it sits beside.

**And the repository had begun to disagree with itself about it**, which is the practical cost:
`ARCHITECTURE.md` §6.4 still justified a v0.1 rule by *"v0.1 ships Komga"* after ADR-0035 said it did
not, §7.1a's Kavita row still read *"no delta at all"* after ADR-0035 verified the sortable keys, and
`SETUP-CHECKLIST.md` still filed Audiobookshelf and Komga under v1.0. Every one of those is drift
from a scope line that had been amended twice without the documents following.

### Decision

> **v0.1 ships Sonarr, Radarr and Prowlarr. No catalogue source ships in v0.1.**
>
> **The \*Arr library sync lands first**, because it is the thing that proves the replica thesis on
> real data — a real Sonarr and a real Radarr, imported, delta-synced, reconciled, searched and
> rendered fast.
>
> **The catalogue sources then arrive one at a time, after v0.1** — Navidrome, Audiobookshelf and
> Kavita, then Komga — **each behind a milestone that ships on its own**, with its own success
> criterion: *this source's library appears in the grid, is searchable, delta-syncs, and its Services
> row is honest about what it cannot do.*
>
> **The order within that sequence is decided by the [ADR-0035](#adr-0035) §2 watermark probe, not
> fixed here: Kavita first if it passes, Navidrome first if it fails.** Komga is last regardless.
> **Navidrome must precede v0.4**, whose success criterion needs a populated music replica.
>
> **This is re-sequencing, not rejection.** Every source ADR-0032 named still arrives. Nothing is
> cut; the count of milestones goes up and the size of each goes down.

### Why this and not the alternatives

**1. It is the only reading of the owner's answer that is actually smaller.** He declined to pick,
which puts the burden on the design thread to choose the option that fails least badly if the
estimate is wrong. Three adapters in v0.1 fails by not shipping; four milestones of one adapter each
fails by shipping later than hoped, with working software at every step.

**2. `CLAUDE.md`'s stated biggest risk is never shipping, and this is the shape of that risk.**
A milestone whose job is *"prove the replica thesis"* and which simultaneously introduces three new
services, three auth schemes and a new sync channel is not proving a thesis; it is testing four
things at once and learning nothing clean from a failure.

**3. Shared machinery has to exist before it can be shared.** §16.0's honest bullet is that
`RemoteItem`, the registry, the circuit breaker, the import phasing and the reconciliation sweep are
reused unchanged by every catalogue adapter. They are reused unchanged *once they exist and have run*.
Writing the first catalogue adapter against machinery that has never imported a real library is how
the shared layer gets shaped around whichever source happened to go first.

**4. Each catalogue source is independently valuable and independently demonstrable**, which is the
test for whether a sequence is a sequence or a stalling tactic. "Navidrome ships and your music is in
the grid" is a release note. So is Kavita's. Nothing in the sequence needs the next item to be
useful.

**5. The probe is a better gate one step later than on day one.** ADR-0035 made the Kavita
`LastChapterAdded` spike day-one *because it decided build order inside v0.1*. With no catalogue
source in v0.1 it gates nothing on day one, and running it early would mean running it before there
is anything to apply the answer to. It moves to immediately before the first catalogue adapter is
written, **with its three-clause pass condition unchanged and still written down in advance** — that
was the point of writing it down, and deferring the run must not turn it back into a guess.

### Consequences

- **v0.1's six-type claim narrows, and has to be said out loud rather than implied.** The **schema**
  is six-type — it must be, migration 0001 can never be edited. **Requesting** is six-type — Prowlarr
  Search-and-Grab covers all six categories, which is already shipped. **The catalogue is film and
  TV.** §16's v0.1 entry and the README rows now say exactly that. Anything that keeps claiming
  "unified library across six media types" for v0.1 is the drift this ADR exists to stop.
- **A media type with no configured source must degrade honestly**, which is principle 3 and is the
  same path an install with no Navidrome would take anyway. This makes the honest-degradation path
  the *common* path in v0.1 rather than an edge case — which is a benefit, because it gets exercised
  from the first release instead of being discovered later.
- **Channel 3b (§7.1a) is specified now and built with the first catalogue adapter.** The
  specification stays in v0.1's documents because the watermark column, the overlap rule and the
  page-walk-stability restart are what the adapters get written against, and because the probe needs
  its pass condition on paper. **Nothing about it is implemented in v0.1.**
- **The libraries subsystem stays in v0.1, and loses its best demonstration.** Its four tables belong
  in migration 0001 regardless, its screen is one of the five `CLAUDE.md` calls essential, and a
  library binding carries the request destination v0.1's write path routes on. ⚠️ **2026-08-17
  ([ADR-0042](#adr-0042)): that third ground is gone — the write path it names re-sequences out of
  v0.1. The first two stand, and so does the conclusion.** But the
  Ebooks/Audiobooks split over one Audiobookshelf library — the concrete improvement over upstream's
  own organisation — moves with Audiobookshelf. v0.1 demonstrates it on something narrower and
  honest: an Anime library bound to a Sonarr tag, or one Films library spanning a 1080p and a 4K
  Radarr.
- **ADR-0035's §1 identity consequence is not cancelled, only rescheduled.** Free Kavita's null
  identifier fields still make *"not identified"* the ordinary case — at the milestone Kavita lands
  in. The nullable column and the badge stay in v0.1 because they cannot be retrofitted, and §6.4 now
  says that rather than justifying them with a source v0.1 does not have.
- **No schema change, and no ADR is reversed.** `work.kind` is untouched, migration 0001 is
  untouched, and the provider registry absorbs the whole sequence by construction — which is the
  registry doing exactly the job it was built for.
- **`SETUP-CHECKLIST.md` gets less useful in the short term and more honest.** The owner does not
  need to stand up anything new for v0.1 beyond what he runs. Kavita, Navidrome and Audiobookshelf
  each become "needed at the milestone that adapts them", and Komga becomes "only if you adopt it".

### Alternatives rejected

- **Keeping ADR-0035's three sources in v0.1.** It is the status quo and it is what §16 said an hour
  ago. Rejected because the price §16.0 states in its own words — four adapters, four auth schemes,
  a new sync channel — was written down and then not treated as a constraint on the milestone that
  would pay it.
- **Growing v0.1 to five providers**, which is the question the owner was actually asked. It is the
  same mistake one size larger, and "I'm fine with starting small" is not a mandate for it.
- **Cutting the catalogue sources back to v1.0 entirely.** That is ADR-0032's problem restored: five
  of six media types are empty screens until the last milestone. ADR-0032's roadmap conclusion was
  right and is preserved; only its v0.1 membership changes.
- **Fixing the order now instead of leaving it to the probe.** Tempting because it would let
  `SETUP-CHECKLIST.md` name a single next service. Rejected for the reason ADR-0035 gives: the
  evidence says `LastModifiedDate` is unusable and says nothing conclusive about `LastChapterAdded`,
  and recording a guess is the exact mistake ADR-0032 correctly refused to make about Komga.
- **Recording this only as an amendment inside ADR-0035, with no new entry.** The design thread wrote
  that amendment in parallel with this ADR and it is kept — it is the right place for *"what does the
  move do to ADR-0035's four findings"*. But it is not the right place for the decision itself: the
  call is about **v0.1's provider count**, not about which comics source, and ADR-0035's own
  amendment says a status note on ADR-0032 *"is owed there rather than here"* — a note has to point
  at something, and pointing ADR-0032 at a subsection of an ADR that reversed one of its members
  would bury a scope decision inside a source decision. `CLAUDE.md` asks for an ADR whenever a
  decision closes off an alternative, and this one closes off growing v0.1 to five providers.
- **Rewriting ADR-0032 and ADR-0035 in place.** The file's convention is a new entry plus a flag on
  the amended ADR's Status line, which is what ADR-0035 itself did to ADR-0032. Rewriting in place
  would delete reasoning that is still correct about everything except timing.

---

<a id="adr-0037"></a>

## ADR-0037 — TOFU SPKI pin enrolment is removed, not completed; enforcement stays

**Status:** Accepted · **2026-08-16** · **Amends no ADR** — per-instance TLS pinning was specified in
`docs/reference/providers.md` §4 and `CONFIGURATION.md` §7.1, never in a decision record, which is
part of why half of it shipped · **Rejects nothing permanently** — the reopening conditions are
stated below and are concrete.

### Context

`service_instance.tls_spki_pin` exists in migration 0001, and `internal/ssrf` **enforces** a pin that
is present: `ssrf.Options.SPKIPin` is compared constant-time against the peer leaf's
SubjectPublicKeyInfo on every handshake, including resumed sessions, and mismatches fail with
`ErrPinMismatch`. That half is real, tested and staying.

The other half — **enrolment**, deciding what goes in the column — was written as trust-on-first-use:
record the pin when a connection test passes. What actually existed was a line copying a `TestResult`
field that **nothing ever populated**, so the column was always `NULL` while the code read as though
enrolment worked. The choice was to finish it or delete it.

`docs/reference/providers.md` §4 and `CONFIGURATION.md` §7.1 both describe the intended flow, and it
is not bare TOFU: **show the operator the fingerprint and have them accept it**, then show it again
and have them re-accept when it changes. None of that UI exists.

### Decision

> **Delete the enrolment line. Nothing writes `tls_spki_pin`; the column stays `NULL` on every row.**
> **Keep enforcement exactly as it is** — a pin that reaches the column by any future route is
> checked on every handshake.
>
> UsArr therefore has one TLS mode today: ordinary chain verification, with `verify_tls` controlling
> whether a self-signed instance is accepted at all.

### Why this and not the alternatives

**Silent auto-capture on a passing connection test downgrades the instances that need it least.** A
pin **replaces** chain verification rather than supplementing it: `internal/ssrf` sets
`InsecureSkipVerify` on the pinned path **by design**, because the certificate the feature exists for
is a self-signed homelab one with no chain to check. Pinning every instance that passes a connection
test would therefore take an instance behind a publicly-trusted certificate — a Sonarr behind Caddy
with a real Let's Encrypt cert, say — and **drop hostname, expiry and revocation checking in exchange
for nothing it did not already have**. The feature would make the well-configured deployment less
safe and the operator would never be told.

**And there is no way back.** `store.ServiceInstanceUpdate` carries `Name`, `BaseURL`, `URLBase`,
`APIVersion`, `Enabled` and `Priority` — **no pin field**. Nothing in the API can clear a pin or
accept a new one. So a routine certificate renewal — an ACME rotation, a container regenerating its
self-signed cert on restart, both of which happen on their own schedule and neither of which is an
attack — **locks the instance out permanently** with `ErrPinMismatch`, and the only recovery is
hand-editing SQLite. A security control whose false-positive path has no supported remedy will be
worked around, and the workaround is worse than the control.

**Half-implemented is the worst of the three states.** A column that is always `NULL` behind code
that reads as though it enrols is a claim the system does not honour — the failure mode `CLAUDE.md`
calls invented status, in code rather than in docs. Deleting the line makes the state legible:
enrolment is absent, and the absence is visible at the one place a reader looks.

### Consequences

* **No behaviour changes.** The column was always `NULL`; this deletes the code that pretended
  otherwise. No migration, no schema change — `tls_spki_pin` stays exactly where it is.
* **The seam survives**, which is the point of paying for it now: the column, the enforcement path
  and its tests are all in place, so enrolment is a UI feature plus one write, not a redesign.
* **`providers.md` §4 and `CONFIGURATION.md` §7.1 now describe an unbuilt flow.** They keep the
  design; the code comment at the deletion site names them as what has to land first.
* **Self-signed instances are served by `verify_tls` alone** until enrolment ships. That is a
  coarser control and it is the honest one.

### Reopening conditions

This is deferred on evidence, not closed on principle. Both of these must land together:

1. **A pin field on the update path** — `store.ServiceInstanceUpdate` gains one, so a pin can be
   cleared and re-accepted through the API. Without it, item 2 has nothing to write to.
2. **The change-acceptance UI** that `providers.md` §4 and `CONFIGURATION.md` §7.1 already describe:
   the fingerprint shown at enrolment and accepted explicitly, and shown again and re-accepted when
   it changes. Not a silent capture, and not a silent re-capture.

Enrolment without both is the design this ADR rejects, regardless of who proposes it next.

### Alternatives rejected

- **Complete TOFU as specified, capturing silently on a passing test.** Rejected on both arguments
  above: it downgrades publicly-trusted instances, and its false-positive path is unrecoverable. The
  "first use" in trust-on-first-use has to be a moment the operator is present for, and a background
  connection test is not one.
- **Keep the dead line and file a TODO.** This is the state being left. Code that reads as a working
  feature and does nothing is worse than either the feature or its absence, because it defeats the
  reading that would find the gap.
- **Drop enforcement too, and the column with it.** Rejected — enforcement is correct, tested, and
  costs nothing while the column is `NULL`. Removing it would make the eventual enrolment feature a
  large change instead of a small one, which is exactly the seam `CLAUDE.md` asks to be preserved.
- **Pin the CA rather than the leaf.** A real option, and a different feature: it survives renewal,
  which is this ADR's main objection. It does not help the self-signed single-certificate case the
  pin exists for, and it is not what the column or the enforcement path implement. Worth raising on
  its own merits when the reopening conditions are met; not a way around them.

---

<a id="adr-0038"></a>

## ADR-0038 — A list freezes its order while a user is aiming at it

**Status:** Accepted · **2026-08-16** · **Amends no ADR** — the rule is new, and it constrains every
mutable list rather than reversing an earlier choice · **The argument is not repeated here.**
[`design/DESIGN-DIRECTION.md`](./design/DESIGN-DIRECTION.md) §9.1a carries the general component
rule and [`ARCHITECTURE.md`](./ARCHITECTURE.md) §17.5 the Requests-screen specification. This record
exists for the part neither of those keeps well — **what was rejected, and why**.

### Context

Settled 2026-08-16 in **four rounds across three threads** — design, frontend and code — over how the
release result list may reorder itself while a Prowlarr fan-out is still delivering, and closed.
Logged as **SW-21** in [`REVIEW-LOG.md`](./REVIEW-LOG.md). The frontend thread **converged on the
suspend ruling independently**, from the implementation side and without the design thread's argument
in front of it, which is why it was treated as settled rather than as one thread's preference — and
then improved it twice: **focus-within beside pointer-within**, and **collapsing the late-arrival
case into the same single control**.

### Decision

> **Instability is acceptable only while nobody is aiming at anything.**
>
> The condition keys on whether **a person is committed to a target**, never on whether the
> **application considers itself settled**. The six clauses that follow — re-sort live and freeze on
> fan-out completion; freeze while the pointer is inside the results region **or** focus is within
> it, surfacing anything that would have reordered as one `3 new results · re-sort` control;
> identity rather than position for focus, hover, selection and pending row state; 0 ms and never
> animated; sort keys in the URL — live in `DESIGN-DIRECTION.md` §9.1a, and **every mutable list in
> UsArr inherits them**.

**Why it keys on the person and not the app.** *"The fan-out finished, so the order is final"* is the
application's own readiness, and it is wrong in both directions: the user reaching for the third row
is not helped by more legs being outstanding, and the user idly reading is not harmed by a row
moving. What makes this a correctness question rather than a polish one is the affordance being
protected — **`Grab` is irreversible from UsArr's side**, since the release is handed to a download
client UsArr deliberately stops observing after handoff (§17.5, §8.5), so a mis-click cannot be
detected, cannot be reported and cannot be reversed. **Where there is no undo, prevention is the only
lever.**

### Alternatives rejected

- **Append a late arrival below the list, marked *late*, instead of counting it in the same
  control.** Rejected, and this is a rejection rather than an omission. One condition with two
  surfaces is two rendering rules and two vocabularies to maintain; the *late* marker is meaningless
  the instant the list is re-sorted; and appending **still** moves the list under an engaged user —
  the row count changes, the scroll extent changes, and a pointer resting near the foot of the list
  gets a new row under it. A late arrival is not a special case, it is another thing that would have
  reordered.
- **Animate the reorder so the movement is legible.** Rejected, and strictly worse than an instant
  re-sort for this rule: an animation *widens* the window in which the row under the pointer is
  neither where it was nor where it is going, and a click landing mid-flight is precisely the
  ambiguous case the rule exists to remove. §6's 0 ms sort is extended to say a reorder is never
  animated anywhere.
- **Key focus, hover, selection and pending row state by index.** Rejected. An index-keyed list
  reassigns all four the moment the order changes, so the highlighted row, the focused row and the
  row with a grab in flight silently become different rows. Identity keying is also what makes the
  keyboard half of the rule work at all: focus is attached to a row, so the row may move and take
  the focus with it.
- **Rely on identity keying alone and drop the pointer clause.** Rejected — the two input paths fail
  differently. Identity fully protects the keyboard user; **the physical pointer is attached to
  nothing**, sitting at a screen coordinate no amount of re-keying moves, so a row shifting under a
  resting pointer puts a *different release* under the click, with no error raised and nothing on
  screen recording the substitution. Neither half of pointer-within **or** focus-within is
  redundant.

### Consequences

* **Nothing is drawn.** The mockups are static documents with invented data: there is no fan-out to
  run, nothing to arrive late, and no honest count for the `re-sort` control to carry. §17.5 says so
  rather than illustrating it — drawing a frozen screenshot would assert a behaviour the artefact
  cannot exercise, which is `CLAUDE.md`'s invented status by illustration. This is specification for
  the implementation, verified there or not at all.
* **The scope is every mutable list**, not one screen. A future list carrying a comparably
  irreversible action inherits the rule for the same reason; a list of purely local reads does not
  need it and should not pay for it.
* **No code, no schema and no mockup changed** when this was settled. The rule lands ahead of the
  screen it governs, which is the cheapest moment at which an ordering contract can be fixed.

---

<a id="adr-0039"></a>
## ADR-0039 — `write_queue.state` loses its `CHECK`; `work_id` gets its foreign key back

**Status:** Accepted · **2026-08-17** · **Supersedes** the four-step instruction in
[`reference/schema.md`](./reference/schema.md) §10 (step 1) and the seam described in
[`FUTURE.md`](./FUTURE.md) §11 and §11.1, all three of which said the `CHECK` would *gain*
`'awaiting_choice'`. **Closes** `REVIEW-LOG.md` **WQ-05**, recorded there as "a lean, not a
decision". Both decisions landed in `internal/db/migrations/00005_library_sync.sql`, whose header
carries the same reasoning next to the SQL.

### Context

`write_queue` had to be rebuilt by the library-sync migration regardless of anything decided here.
`00001_initial.sql` created `work_id` as a bare `INTEGER` with the `REFERENCES work(id) ON DELETE
CASCADE` clause **dropped**, because `work` did not exist yet, and with a comment naming library sync
as the migration that restores it. SQLite can add neither a foreign key nor a `CHECK` to an existing
column, so restoring the reference costs a full 12-step table rebuild — one that is mandatory, not
optional. That is the fact that makes both questions below free to answer: **every option costs the
same rebuild**, so nothing here is being bought with migration effort.

Two questions arrived attached to it. `REVIEW-LOG.md` **WQ-01** established that `state` has no legal
value meaning *"waiting for a human"*: `pending` gets claimed by a worker, `inflight` asserts an
outstanding upstream request that does not exist while a person is deciding, and `verifying` carries
a 15-minute `verify_until` TTL that would settle a sleeping user's request as
`fail_reason = 'unknown'`. **WQ-05** left open whether such a state, once it existed, belonged in
`ix_wq_runnable`'s partial predicate.

### Decision

> **1. `write_queue.state`'s `CHECK` is dropped entirely, not widened.** The column is
> `TEXT NOT NULL DEFAULT 'pending'` with no constraint. The vocabulary — `pending` · `inflight` ·
> `verifying` · `awaiting_choice` · `done` · `failed` — has no home in the schema, and **declaring
> and validating it in Go is owed by whoever writes the first `write_queue` writer**. ⚠️ **Nothing
> validates it today**, in Go or anywhere else — see the dated correction immediately below.
>
> **2. `'awaiting_choice'` is excluded from `ix_wq_runnable`'s partial predicate**, which stays
> byte-identical to `00001`'s `WHERE state IN ('pending','inflight','verifying')`, with the reason
> written beside it in the SQL.
>
> **3. `write_queue.work_id` regains `REFERENCES work(id) ON DELETE CASCADE`.**
>
> **4. `fail_reason`'s `CHECK` is kept**, in its `IS NULL OR … IN (…)` form.

### ⚠️ Correction, 2026-08-17 — decision 1's Go validation is **owed**, and this ADR twice wrote it as done

**As first written, decision 1 said the vocabulary *"moves to Go and is documented and validated
there"*, and the first rejected alternative said *"the vocabulary is validated in Go on the way
in"*. Both are present tense and both were false on the day they were written.** No Go code declares
or validates `write_queue.state`. Measured 2026-08-17 on `49dfa6c`:
`grep -rn "awaiting_choice" --include='*.go' internal/ cmd/` returns **six hits, all in
`internal/db/migrate_test.go`** (`:719`, `:758`, `:761`, `:1748`, `:1749`, `:1800`) — tests that
assert what the *schema* does, not a vocabulary a writer enforces. The non-test picture is unchanged
from the one this ADR already recorded under *"Why — decision 3"*, ground 3: the only non-test
`write_queue` references under `internal/` and `cmd/` are the RSS-spike binary in
`internal/db/spike/`, which is behind `//go:build bench` and so is compiled by no step of
`go build ./...`, plus one comment at `internal/httpapi/grabs.go:58` whose own text is *"Nothing
writes `write_queue` yet."*

**So this ADR contained the refutation of its own decision-1 wording, three sections further down.**
Ground 3 is the measured claim and it is right; decision 1 and the first rejected alternative are
the claim stated as complete. The corrected reading, which is what the decision always meant:
**Go is where the vocabulary is going to live and it is not there yet.** The first `write_queue`
writer owes the declaration and the validation, and until it lands the vocabulary is documented in
`00005_library_sync.sql`'s header and nowhere that runs.

📌 **Corrected in place rather than by an amendment section or a new ADR, and the reasoning is the
file's own.** Both of the heavier mechanisms exist here for a reason that does not apply. ADR-0035's
§2a and its `⚠️ Amendment` section record a **fact about the world changing under a standing
decision** — a milestone moved, a spike ran and returned a result — so the original text stays true
of its own date and the new fact is appended beside it. ADR-0036's alternatives set the other rule:
*"the file's convention is a new entry plus a flag on the old"*, and that is scoped to **reversing a
decision**. Neither is this. **Nothing here changes, reverses or re-dates a decision** — the `CHECK`
is still dropped, `work_id` still gains its key — and nothing in the world changed either: the
sentence was wrong the moment it was written, hours ago, and preserving it verbatim would preserve
only a false status claim. `CLAUDE.md` is explicit that status is read off the tree and never
restated, so the sentence has no historical value to protect. **The one place strike-through *is*
used below is decision 3's ground 1, and the difference is instructive: that ground was an argument,
and a withdrawn argument someone may re-derive is worth showing struck.** A wrong tense is not an
argument.

⚠️ **Both sites are corrected — decision 1 above, and the first rejected alternative below — and the
consequences bullet that already said this correctly is left alone.** It reads *"Until that code
exists, the vocabulary is documented in `00005`'s header and nowhere else, which is a real gap and is
why this bullet exists"*, which was the honest sentence in the ADR all along. Recorded as
`REVIEW-LOG.md` **M5-25**.

### Why — decision 1

**The vocabulary is demonstrably still growing.** It has already been widened once in the design
(WQ-01 → `'awaiting_choice'`), and v0.2's request path has a live candidate: a request-layer
`'pending'` is not this table's `'pending'`. A `CHECK` that must be renegotiated per string literal
is a 12-step rebuild per string literal, and this one has been renegotiated once before shipping a
single row.

**There is shipped precedent, twice, for exactly this reason.** `audit_log.result` carries no
`CHECK` (`00001`). Migration `0003`'s `provenance.acquisition_state` is `TEXT NOT NULL` with no
`CHECK`, and its own header says why: *"SQLite cannot `ALTER` one and 0001's `audit_log` foreign key
is what that costs — so the vocabulary lives in `internal/store` and is enforced there."*
`write_queue.state` is the same class of column, and treating it differently was inconsistency
rather than rigour.

**`write_queue.kind` — the verb half of the same row — has never had a `CHECK`.** A constrained
lifecycle beside an unconstrained verb was already asymmetric, and it is the verb that a new sink
extends first.

### Why — decision 2

A row waiting on a person is not runnable. This partial index is what the retry sweep walks **and**
what [`reference/sync.md`](./reference/sync.md) §4's reconciliation guard tests, so a state listed in
it is a state that gets claimed by a worker and exposed to the `verify_until` TTL — which is WQ-01's
defect, reintroduced through the index. The `REVIEW-LOG.md` lean was to exclude; the objection was
that the predicate also serves the reconciliation guard and the call wanted that code in view. The
guard is still unwritten, but its specified behaviour, quoted from `reference/sync.md` §4 in full —

> **Precondition: the write-queue guard.** The sweep may correct an item **toward the \*Arr** only
> when there is **no `write_queue` row for that work in `pending`, `inflight` or `verifying`**.

— names the three states literally, so including a fourth would change the guard's meaning as well as
the sweep's, which settles it in the same direction from the side the objection came from. ⚠️ **The
scope words *toward the \*Arr* are load-bearing and an earlier revision of this ADR dropped them from
the quotation.** They do not weaken decision 2 — an extra state in the predicate widens what the
outbound guard blocks either way — but they are what makes the same sentence inapplicable to decision
3's tombstone case, which is why that ground is struck below rather than repeated here.

### Why — decision 3

**`audit_log`'s precedent was considered and does not transfer.** That precedent is not *"foreign
keys are risky"*; it is *"an `ON DELETE` action is an implicit write, so a table that **forbids**
implicit writes cannot carry one."* `audit_log` has `trg_audit_no_update` and `trg_audit_no_delete`,
which is what turned its foreign key into an undeletable user. `write_queue` has no triggers, is not
append-only, and is not history — it already carries two `ON DELETE CASCADE`s of its own, to `user`
and to `service_instance`. The precedent says *check*, and the check passes. Verified rather than
argued: `TestMigrate0005WorkIDForeignKey` deletes a user who has queued a command.

**The real objection is the tombstone one. It was first answered on three grounds; ground 1 is struck,
and the decision stands on the two that survive.** A 7-day tombstone expiry that hard-deletes a `work`
would silently take a queued command with it, where no foreign key would leave the command to fail
loudly at the \*Arr.

1. ~~`sync.md` §4's write-queue guard is normative and forbids the sweep from acting on a work with a
   row in `pending`, `inflight` or `verifying` — so the collision is one the sweep may not cause, and
   `ix_wq_runnable` is the index that guard uses.~~ 🚩 **STRUCK 2026-08-17, and struck rather than
   deleted because the error is instructive.** The ground rested on a quotation with three words
   removed. [`reference/sync.md`](./reference/sync.md) §4 reads *"The sweep may correct an item
   **toward the \*Arr** only when there is no `write_queue` row …"* — the guard is scoped to
   **outbound corrections**, writes UsArr would push at the \*Arr. The tombstone path is neither: §4
   step 3 soft-deletes **locally** a row the \*Arr has already dropped, and the hard delete seven days
   later is local too. So the guard does not forbid this collision, and there is no other precondition
   in §4 that does. **The collision is one the sweep may cause**, and this ADR is decided knowing that.
2. A work reaches hard delete only because the \*Arr itself no longer has it, so a surviving
   *"monitor this"* command can only ever fail. Keeping it produces an alarm in Home's attention
   block that the user cannot act on and cannot distinguish from a real fault. **This ground is
   independent of ground 1 and is unaffected by its withdrawal** — it argues from what the \*Arr holds,
   not from what the sweep is permitted to do, and it is the ground that answers the collision ground 1
   was wrongly said to prevent.
3. *"Fails loudly"* is aspirational either way. **Nothing outside `internal/db/spike/` reads
   `write_queue`** — measured 2026-08-17 on `b8bb500` with
   `grep -rn "write_queue" --include=*.go internal/ cmd/ | grep -v _test.go`, whose only hits are the
   standalone RSS-spike binary under `internal/db/spike/` and one comment at
   `internal/httpapi/grabs.go:58`; roots searched are `internal/` and `cmd/`, and the claim is made
   about no others. So without the foreign key the dangling row simply sits there and `ix_wq_work`
   returns rows for a `work_id` with no referent. `CASCADE` is the answer the table already gives for
   its other two parents, and it is the one SQLite can enforce today.

**Does the decision survive on 2 and 3 alone? Yes, and plainly:** the choice is between a queued
command that vanishes with its target (`CASCADE`) and one that outlives it as an uninterpretable row
pointing at nothing. Ground 2 says the outliving row could only ever have failed, and ground 3 says
nothing would notice it either way. Neither borrows anything from the sweep's permissions. The
rejected alternatives below — `RESTRICT`, `SET NULL`, no foreign key — are likewise argued without
ground 1.

### Alternatives rejected

- **Widen the `CHECK` to add `'awaiting_choice'`** — what §10, `FUTURE.md` §11 and §11.1 all
  specified. Rejected because it buys one literal and leaves the next one costing a full rebuild,
  against a vocabulary that has already moved once. It also leaves the schema inconsistent with the
  two shipped columns of the same class. **What it would have bought is real and is given up
  knowingly:** SQLite will now accept a misspelt state, and a bug that writes `'pendign'` reaches
  disk. ⚠️ **The mitigation is currently one half of what this bullet first claimed** (corrected
  2026-08-17, see the correction above): validating the vocabulary in Go on the way in is **owed by
  the first `write_queue` writer and does not exist**, so the only mitigation running today is that
  `ix_wq_runnable`'s predicate is the operational filter — a misspelt state is simply never runnable,
  which is a visible stall rather than silent wrong behaviour. That half is enough to keep a
  misspelling from being *silently wrong*, and it is not enough to keep it from being *written*.
- **Drop `fail_reason`'s `CHECK` too, for symmetry.** Rejected. That vocabulary is *closed* — it is
  the terminal taxonomy (`rejected` · `unknown` · `exhausted`), not the lifecycle one — and the
  column is DB-01's regression witness: it is the one place in the schema that proves
  `CHECK (col IS NULL OR col IN (…))` behaves differently from `CHECK (col IN (NULL, …))`. Deleting
  it would delete the test's subject.
- **`ON DELETE RESTRICT` / `NO ACTION` on `work_id`**, so a pending command blocks the delete rather
  than dying with it. Rejected: it makes a tombstone expiry fail, which stalls the reconciliation
  sweep for that work indefinitely and turns a queue bug into a sync outage.
- **`ON DELETE SET NULL` on `work_id`.** Rejected: it keeps the row and destroys the only thing that
  says what the row was *about*, leaving a command nobody can interpret. A queued verb with no target
  is worse than no row.
- **Leave `work_id` without a foreign key, as `00001` has it.** Rejected: it is the status quo only
  because `work` did not exist, `00001`'s own comment schedules the restoration, and the rebuild
  needed for it is happening anyway. Declining now would mean paying for a second rebuild later to
  get what this one could have had for nothing.
- **Restore the other four dropped foreign keys in the same migration** —
  `tag_assignment.work_id` / `.edition_id` / `.media_file_id` and `release_candidate.work_id`, whose
  `00001` comments also name library sync. Deferred, not refused: each is a further 12-step rebuild,
  neither table has a reader blocked on it (`release_candidate.work_id` is `NULL` for the whole
  Search-and-Grab path and nothing writes `tag_assignment` yet), and a rebuild is no cheaper today
  than in the migration that first writes those columns. `CLAUDE.md`'s *"cut before you add"* applies
  to migrations too.

### Consequences

* **The `state` vocabulary now has exactly one home, and it is Go.** Whoever writes the write-queue
  worker owns declaring and validating it; there is no second copy in a migration to drift from.
  Until that code exists, the vocabulary is documented in `00005`'s header and nowhere else, which is
  a real gap and is why this bullet exists.
* **`'awaiting_choice'` costs no further migration.** `FUTURE.md` §11's seam is wider than the one it
  described — and correspondingly less self-policing.
* **Deleting a `work` now deletes its queued commands.** That is new behaviour, it is silent by
  design, and `TestMigrate0005WorkIDForeignKey` is where it is written down as intended.
* **A dangling `work_id` is now impossible**, so `ix_wq_work` can no longer return rows for a work
  that does not exist, and the reconciliation guard's lookup is a key with a guaranteed referent.
* **`docs/reference/schema.md` §10's DDL block still shows the pre-rebuild shape**, deliberately: the
  reasoning under it is unchanged. `internal/db/testdata/schema.sql` is the current schema, and the
  four-step instruction above it is now a pointer to this ADR and to the migration rather than a
  second copy of what they say.

---

<a id="adr-0040"></a>
## ADR-0040 — The six subtype tables land with the catalogue source that writes each

**Status:** Accepted · **2026-08-17** · **Records as a decision** what
`internal/db/migrations/00005_library_sync.sql` did and what its header argued. Depends on
[ADR-0035](#adr-0035) and [ADR-0036](#adr-0036) for *which* source each table waits on and in what
order, and on [ADR-0030](#adr-0030), [ADR-0031](#adr-0031) and [ADR-0033](#adr-0033) for the `work.kind`
members and columns that did **not** wait. ⚠️ **In tension with ARCHITECTURE §16's enumerated v0.1
schema line — see the consequences.** · ⚠️ **Amended 2026-08-17 by
[ADR-0044](#adr-0044)** — see the flag below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0044](#adr-0044): this ADR's RULE is confirmed and APPLIED, and
> one table moves under it.** Nothing in the Why or the Alternatives is reopened; the sentence below
> that no longer holds is decision clause 1's placement of **`work_credit`** with the music tables.
>
> **What moved.** Clause 1 files `work_credit` with `work_album` and `work_track` under **Navidrome**.
> That grouping was made when the first catalogue source was assumed to be a music server and *"a
> credit"* meant a performer. [ADR-0041](#adr-0041) made **Kavita** the first adapter, and Kavita
> reports writers, cover artists, pencillers, inkers, colorists, letterers, editors and translators on
> its series metadata — eight roles, every one of them already a member of `work_credit.role`'s own
> `CHECK`. So **this ADR's rule — the landing point is the SOURCE that writes the table, not a date —
> points at Kavita for `work_credit` once that fact is on the table.** ADR-0044 applies the rule
> rather than overruling it; `work_credit` lands in `00007_work_credit.sql`.
>
> **What did NOT move, and it is most of this ADR.** `work_album` and `work_track` still wait for
> Navidrome, which still has no adapter. Decision clause 2 (what is irreversible is created anyway)
> is untouched. The rejected alternatives stay rejected — in particular *"create all six now"*, which
> ADR-0044 re-weighed and refused again. The consequence about ARCHITECTURE §16's enumerated v0.1
> schema line still stands, one table closer to agreement.

### Context

`00005_library_sync.sql` created the schema v0.1's Sonarr/Radarr sync needs and **did not create six
tables that ARCHITECTURE §16's *"Schema, enumerated"* clause names**: `work_album`, `work_track`,
`work_credit`, `work_book`, `work_comic` and `work_comic_issue`. That is a scope call, and until this
ADR it had no record of its own. The migration header carries the argument and ARCHITECTURE §16 carries
a flag paragraph, but **the two ADRs the flag paragraph cited — 0035 and 0036 — both state *"No schema
change"* in their own Consequences** (`DECISIONS.md` under ADR-0035 and ADR-0036). Neither of them
authorises a schema scope change, so the deferral was resting on citations that disclaim it. This ADR
is the missing authority, written after the fact and saying so.

The deferral is not the same question as the one ADR-0030 and ADR-0033 answered. Those two put
`comic_issue` and `person` into `work.kind`'s `CHECK` *now*, against a milestone that has no source for
either, on an irreversibility argument. The question here is whether that argument reaches the tables.

### Decision

> **1. The six subtype tables — `work_album`, `work_track`, `work_credit`, `work_book`, `work_comic`,
> `work_comic_issue` — are not created by the library-sync migration.** Each lands with the catalogue
> source that writes it, in the §16.1 sequence ADR-0036 established: the music three with **Navidrome**,
> the books-and-comics three with **Kavita** (ADR-0035).
>
> ⚠️ **The rule survives and the grouping does not (2026-08-17, [ADR-0044](#adr-0044)):
> `work_credit` lands with KAVITA**, because Kavita is the source that writes credits. *"The music
> three"* now means `work_album` and `work_track`. The rule this clause states is what moved it.
>
> **2. What is irreversible is created anyway, and was.** `work.kind` carries its **full twelve-member
> `CHECK`** in 00005, including the six kinds only those tables serve, and `edition.narrators` /
> `duration_seconds` / `abridged` are on `edition` from the day it exists.

### Why

**00001's own rule, quoted verbatim in 00005's header:** *"a migration that creates a table nothing
queries is a schema claim nobody has tested."* Nothing in v0.1 writes any of the six. A table created
against no writer is DDL that has never round-tripped a row, and this project has already shipped one
class of that defect — DB-01's `CHECK (col IN (NULL, …))`, which enforced nothing and was found twice,
the second time in this very migration's `library_override.field_name`.

**The irreversibility that forced the `CHECK` does not transfer, and the asymmetry is the whole
argument.** Adding a `work.kind` member later costs a 12-step rebuild of the **largest table in the
schema**, plus a `kind_byte` allocation that ARCHITECTURE §5.3 states is unchangeable once clients
cache northbound ids — a genuine one-way door, which is why ADR-0030 and ADR-0033 paid for it up
front. Each of the six tables is instead a **brand-new table with no dependants**: a plain later
`CREATE TABLE`, no rebuild, no backfill, no codec change, no `ALTER`. `work_track.edition_id` is the
sharpest case — ADR-0031's *"adding it later is a backfill over the largest table in the schema"* is
true of a column added to an existing `work_track` and simply **does not apply** while `work_track`
does not exist: the table will be created with the column already on it. **Nothing about deferring
these six is a one-way door.**

**Each table has an owner and a landing point, so this is a schedule and not a hole.** The music
three land with Navidrome and their command sink (Lidarr) is in no milestone before that; the
books-and-comics three land with Kavita. 00005's header names both, per table.

### Alternatives rejected

- **Create all six in 00005 as specified by §16's enumeration.** Rejected: six untested tables, six
  sets of DDL nothing exercises, and no door closes by waiting. It would also have meant creating
  `work_credit` with `creator_work_id` pointing at kinds no v0.1 source produces — a foreign key whose
  referents cannot exist.
- **Create a subset — the three that `work.kind`'s new members imply (`work_comic`,
  `work_comic_issue`, `work_credit`).** Rejected as the worst of both: it splits the six on a line
  (*"is a kind member new?"*) that has nothing to do with when a writer arrives, and leaves three
  untested tables instead of six.
- **Amend ARCHITECTURE §16's enumerated line here, so the document and the tree agree.** Rejected as
  out of this ADR's lane rather than as wrong — §16 is authoritative for scope, `DEVELOPMENT.md` §11
  requires an edit to a shared document outside the area you lead to be announced before it is pushed,
  and this thread does not own §16. §16 carries a flag paragraph pointing here instead; the amendment
  is routed to the thread that owns §16.
- **Defer `work.kind`'s six extra members too, for symmetry with the tables.** Rejected on the
  asymmetry above: the members are the one-way door and the tables are not.

### Consequences

* **⚠️ ARCHITECTURE §16's *"Schema, enumerated"* clause and the tree disagree, and the disagreement is
  recorded rather than resolved.** §16 names `work_comic` / `work_comic_issue` and `work_track` /
  `work_credit` in v0.1's schema; `internal/db/migrations` has none of them. §16 is authoritative for
  **scope**, so this ADR does not overrule it — it records that the migration read the clause's own
  qualifier (*"migration 0001 **or a backfill over the largest tables in the schema**"*) and applied
  the reason rather than the list. **Whoever owns §16 decides whether the clause is amended or the
  tables are pulled forward.** Until then the flag paragraph in §16 is the honest state of it.
* **Six tables now have a named landing point instead of a milestone.** *"With Navidrome"* and *"with
  Kavita"* are testable claims about a sequence; *"v0.1"* was a claim about a date that the migration
  had already falsified.
* **A future reader who finds `work_track` missing has an answer that is not the migration's header.**
  That was the actual gap: 00005's reasoning existed only inside a SQL comment.
* **This ADR authorises no removal from `reference/schema.md`.** The six tables keep their full DDL
  and their invariants there, marked by milestone; the file's own header is already explicit that a
  table given in full may be design-only.
* **No code and no schema changed when this was written.** The decision it records shipped in
  `00005_library_sync.sql`; what lands here is the authority the deferral did not have.

---

<a id="adr-0041"></a>
## ADR-0041 — The sync core ships with Kavita as its first adapter; Sonarr and Radarr re-sequence behind it

**Status:** Accepted · **owner-decided 2026-08-17** · **Amends [ADR-0036](#adr-0036)**, whose
decision line *"No catalogue source ships in v0.1"* is the sentence this changes ·
**Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16**, which remains authoritative for scope — the
replacement text for §16.1's v0.1 entry is drafted below and **routed to the thread that owns §16
rather than applied here** · **Re-sequences, rejects nothing** — Sonarr and Radarr both still arrive ·
**Confirms [ADR-0035](#adr-0035)** (which source) and **[ADR-0040](#adr-0040)** (when each subtype
table lands), neither of which is reopened. · ⚠️ **Amended 2026-08-17 by [ADR-0042](#adr-0042)** —
see the flag below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0042](#adr-0042): the one question this ADR deliberately left open
> is now closed, and this ADR is completed rather than corrected.** Its final consequence bullet
> reads *"v0.1's minimal write path loses its target, and that question is NOT decided here"*, and the
> ⚠️ clause in its proposed §16.1 text says the choice *"is this section's call to make"*. **§16's
> owner made it, on the owner's own answer:** the minimal write path (`monitor`, `unmonitor`,
> `delete`, `add`) on the durable command queue **re-sequences with the \*Arr adapters — specified,
> not built** — because all four verbs are Servarr operations, and because the shipped Prowlarr grab
> path was measured and **never used the queue** (`handleGrab` dispatches synchronously; `grab` is
> max one attempt). **Nothing else in this ADR moves**, and its own *"re-sequenced, not cut"* framing
> is what ADR-0042 extends to the write path, at the owner's explicit condition that Sonarr and
> Radarr stay on the roadmap. Read ADR-0042 for the decision; the two places below carry inline flags.

### Context

**ADR-0036 got the principle right and the source wrong, on a fact nobody had asked for.** Its
decision reads: *"The \*Arr library sync lands first, because it is the thing that proves the replica
thesis on real data — a real Sonarr and a real Radarr, imported, delta-synced, reconciled, searched
and rendered fast."* §16.1's v0.1 entry says the same in the roadmap's own words: **Tier 0 Go adapters
for Sonarr and Radarr**, *"which is what proves the replica thesis on real data"*.

**A scoping pass asked the question the argument had skipped — does the owner actually run these
services — and the answer was no.** In his words, 2026-08-17:

> *"I don't run sonarr or radarr just yet. eventually i want to start collecting movies and tv shows.
> but thats gonna have to be future."*

**So the milestone's stated success criterion is not merely hard, it is unmeetable on the hardware it
targets.** "A real Sonarr and a real Radarr, imported" cannot be satisfied where neither exists. This
is not a quibble about wording: *on real data* is the load-bearing half of ADR-0036's argument for
putting the \*Arr sync first, and it is the half that evaporated.

**What is left if the milestone is attempted anyway is verification against recorded fixtures and the
vendored specs — and this repository has already written down, twice, what that is worth against this
ecosystem.**

- **§7.2 and [`reference/sync.md`](./reference/sync.md) §2:** `GET /api/v3/episode` is **not** a
  bare-array endpoint — Sonarr's `EpisodeController` rejects a parameterless call with
  `BadRequestException("seriesId or episodeIds must be provided")` — while **the OpenAPI specs mark
  those parameters `required: false`**, *"which is why a spec-derived design gets it wrong: the
  constraint lives in the controller, not the schema."* A cassette recorded against the wrong shape
  records the wrong shape faithfully.
- **[ADR-0035](#adr-0035) §2a is the same lesson from the other direction**, and it is the only live
  contact this project has made: running the probe against a real instance found that
  `SeriesFilterField` carries **no timestamp member**, so the since-filter the pass condition was
  written around **is not expressible**; that `SortField.LastModifiedDate` exists while `SeriesDto`
  returns no last-modified property, so that key is unusable; and that `lastChapterAddedUtc` values
  cluster on the **scan job's** clock — three series within microseconds at `07:00:30` — which is why
  §7.1a's overlap window is not optional. **None of those three is discoverable from a fixture
  recorded against the assumptions being tested.**

**Kavita is the source the owner actually runs, and it is the one catalogue source that can be seen
working.** Its channel-3b watermark criterion — written down in advance, per ADR-0035 §2 — **ran
2026-08-17 against his live instance (Kavita 0.9.0.2, 151 series, page size 10) and passed clause by
clause** (§2a). §16.1 already re-sequenced it to slot #1 on that result, in commit `34383c9`.

**And the cost of the swap is small, because the sync core is source-agnostic.** The streaming import
and its two-phase rendering, the batched writes and the single-writer discipline, the upsert and
identity path, the search-document builder, library membership derivation, and the reconciliation
sweep with both its guards are the same work whichever adapter feeds them. §16.0 says this in its own
words — *"the read machinery is genuinely shared … `RemoteItem`, the registry, the circuit breaker,
the import phasing, the write-queue-free read path and the reconciliation sweep are reused unchanged
by every catalogue adapter"* — and then draws the conclusion that the shared layer must be shaped by
whichever source goes first. That conclusion is kept. **Only the identity of that source changes**,
to the one that can be run against real data.

### Decision

> **1. The sync machinery is built now, and Kavita is its first adapter.** v0.1's catalogue source is
> **Kavita**, not Sonarr and Radarr.
>
> **2. ADR-0036's rule survives; its membership changes.** The rule is *"prove the replica thesis on
> real data, on one source, before a second adapter is written."* ADR-0036 named the wrong source for
> it, on a fact it did not have. **The count is unchanged at one.**
>
> **3. The Sonarr and Radarr adapters are RE-SEQUENCED, NOT CUT.** The owner intends to run both.
> When they arrive they land on a core **already proven against real data**, rather than being the
> thing that has to prove it. Nothing about them is refused, and no work already done on
> `internal/servarr` is discarded — Prowlarr Search-and-Grab runs on it today.
>
> **4. v0.1's sync channels for its catalogue source are 1, 3b and 4 — not 1, 3 and 4.**

### What "first adapter" means for the sync channels, stated exactly

This is the one place the swap is **not** a like-for-like substitution, and it must not be glossed.

**Channel 3 does not apply to Kavita.** §7.1a opens by saying so — *"Channel 3 does not apply to
Navidrome, Audiobookshelf, Komga or Kavita, and none of them has a changed-since endpoint"* — and
`reference/sync.md` §1 defines channel 3 as the `/history/since` delta poll, exposed by six \*Arr
apps and by no catalogue source. Kavita has no history endpoint and no library entity feed to poll.

| | \*Arrs, as ADR-0036 scoped it | **Kavita, as this ADR scopes it** |
|---|---|---|
| Channel 1 — full import | ✅ | ✅ |
| Channel 3 — `/history/since` delta poll | ✅ | ❌ **not applicable** (§7.1a, `reference/sync.md` §1) |
| Channel 3b — ordered page walk with a client-side stop | ❌ specified, not built | ✅ **built** |
| Channel 4 — reconciliation sweep + both guards | ✅ | ✅ |

**So channel 3b, previously "specified but not built in v0.1", becomes v0.1 work.** Three documents
say the opposite today and all three are amended by this decision: §7.1a's closing paragraph
(*"specified here but built with the first catalogue adapter, not in v0.1"*), §16.1's v0.1 entry
(*"Channel 3b is specified (§7.1a) and not built here, because the sources that need it are not
here"*), and ADR-0036's own Consequences (*"Nothing about it is implemented in v0.1."*). **That is a
real addition to v0.1 and this ADR is not pretending otherwise** — but it is the channel whose pass
condition was written down in advance and then verified against the owner's live instance, which is
the strongest position any channel in this project is in.

**The delta is weaker than channel 3 would have been, and that is the honest price.** Two limits,
both already documented and neither newly introduced here:

- **A page walk cannot observe a deletion, structurally** (§7.1a). Channel 4 is the only deletion path
  for Kavita, so the sweep does more work in v0.1 than it would have for the \*Arrs.
- **Kavita's watermark moves on a chapter *add* only.** ADR-0035 §2a clause (c) settled this from
  source: `UpdateLastChapterAdded()` has exactly one production call site, in the new-chapter branch.
  Edits, retitles, deletions and cover changes are reconciliation's business, not the delta's.

Both are surfaced rather than swallowed — §7.1a's *"no change feed — full compare at 09:12"* Services
state and §17.3's per-channel freshness rule already specify how.

### Alternatives considered

- **(a) Build Radarr as scoped, and verify against the recorded fixtures plus a throwaway Radarr we
  stand up ourselves.** ⚠️ **The strongest alternative, and it is not a strawman:** it is the smallest
  possible deviation from an Accepted ADR, `internal/servarr` is genuinely part-built, and a throwaway
  instance is *real HTTP against real Radarr code* — strictly better than a cassette, and it would in
  fact have caught §7.2's controller-versus-spec divergence. **It loses on the criterion's actual
  word, which is *data*, not *HTTP*.** A throwaway instance holds a synthetic library: a handful of
  films added minutes ago, no history depth, no root-folder sprawl, no accumulated quality-profile
  mess, and nothing remotely like the **30–80 MB single response on a 1 GB Pi** that §7.2's entire
  streaming-import design exists to survive. The defects it would *not* show are precisely the ones
  this project has catalogued: `/history/since` returning a week of unbounded history in one array,
  the `Date`-header clock skew that channel 3's overlap window is derived from, and cross-app
  behavioural parity that `reference/sync.md` §1 explicitly marks *"not verified — probe at connect
  time"*. It buys real HTTP and not real data. And it is **nobody's install**: it is torn down after
  the milestone, so no regression it would have caught is ever caught twice.
- **(b) Stand up a real Radarr on the owner's box, and let real data accumulate.** Also not a
  strawman — it is the only option that satisfies ADR-0036 as literally written. It loses on the
  owner's own sentence: *"thats gonna have to be future."* A Radarr he has not chosen to fill is
  alternative (a) relocated to his hardware; making it *real* means him adopting a service and
  starting a film collection he has said is not now, and then waiting weeks for the library to reach
  a depth that tests anything. **Scope is the project's constraint to manage, not the owner's media
  habits**, and asking the user to change what he runs so that a milestone label stays true inverts
  who the product is for.
- **(c) Do nothing on sync until the owner runs an \*Arr.** It has the virtue of leaving ADR-0036
  untouched. It loses hardest of the three. The sync core is **the largest unbuilt subsystem in the
  project**, `CLAUDE.md` names never shipping as the biggest risk, and this blocks the core *and* all
  four milestones of §16.1 behind an event with no date — while the one catalogue source that can be
  demonstrated is already gate-cleared. It also wastes the probe: ADR-0035 §2's criterion was written
  down in advance and run against a live instance **in order to decide build order**, and (c) declines
  to use the answer it bought.
- **(d) Keep Sonarr and Radarr in v0.1 and weaken the success criterion to "verified against
  fixtures and the vendored specs".** Rejected because it is the failure this project has already
  been bitten by, dressed as a decision. `DEVELOPMENT.md` §11's guard rules exist because *"this repo
  shipped its opposite and stayed green"*, and §7.2's controller-versus-spec case is the concrete
  instance in this exact subsystem. Redefining the criterion to whatever is reachable is not scoping;
  it is deleting the measurement.
- **(e) Do both — \*Arrs as scoped, plus Kavita.** Two adapters in the milestone whose job is proving
  one thesis. This is ADR-0036's own rejected alternative at a smaller size, and its reason applies
  unchanged: a milestone that tests two things at once learns nothing clean from a failure.

### Consequences

**What this changes:**

- **v0.1's catalogue is books and comics/manga, not film and TV.** §16.0's *"what the six-type claim
  honestly is in v0.1"* paragraph and the README rows derived from §16 both need the noun swapped.
  The other two thirds are untouched: the **schema** is still six-type because migration 0001 can
  never be edited, and **requesting** is still six-type because Prowlarr Search-and-Grab covers all
  six categories and already ships.
- **Channel 3b moves into v0.1** — see the section above. §7.1a's closing paragraph, §16.1's v0.1
  entry and ADR-0036's channel-3b consequence bullet are each amended by this ADR.
- **`work_book`, `work_comic` and `work_comic_issue` are now due with THIS work, not later.**
  [ADR-0040](#adr-0040) is confirmed, not reopened: each subtype table still lands with the catalogue
  source that writes it, and **Kavita is what writes those three**. They arrive in a **new migration**
  — `00005_library_sync.sql` is merged and `CLAUDE.md` is unambiguous that a merged migration is never
  edited. **The music three are unaffected**: `work_album`, `work_track` and `work_credit` still wait
  for Navidrome, which still has no adapter.
- **ADR-0035 §1's identity consequence lands in v0.1 rather than being rescheduled.** Free Kavita's
  null identifier fields make *"not identified"* the ordinary case, which ADR-0036 pushed out to
  Kavita's own milestone. The nullable column and the badge were already v0.1 work because they cannot
  be retrofitted; what changes is that the path is now **exercised** in v0.1 instead of merely present.
- **The libraries subsystem gets back a real demonstration**, which ADR-0036 recorded losing. Not
  Audiobookshelf's Ebooks/Audiobooks split — that still moves with Audiobookshelf — but an Ebooks
  library and a Comics library derived from one Kavita's own containers, which is the §17.8 binding
  doing exactly its job.
- **⚠️ v0.1's minimal write path loses its target, and that question is NOT decided here.** §16.1's
  v0.1 entry names *"minimal write path (`monitor`, `unmonitor`, `delete`, `add`) on the durable
  command queue"*, and every one of those verbs is \*Arr-shaped. **Kavita is a read-only catalogue
  source with no command sink** ([ADR-0032](#adr-0032)). Whether that write path re-sequences with the
  \*Arr adapters, or stays and is exercised only by Prowlarr's grab path, is a scope call that belongs
  to whoever owns §16. It is flagged here rather than answered, because this ADR does not own §16 and
  `DEVELOPMENT.md` §11 requires an edit to a shared document outside your area to be announced before
  it is pushed. ✅ **ANSWERED 2026-08-17 by [ADR-0042](#adr-0042), owner-decided: it re-sequences with
  the \*Arr adapters — specified, not built.** The second option was refused on a measurement this
  bullet did not have: **the grab path never used the queue**, so "stays for Prowlarr's grab path
  alone" would have meant *building* v0.1's first queue writer for a path that completes inside one
  HTTP handler and is barred from retrying. Sonarr and Radarr stay on the roadmap, which is the
  owner's stated condition.

**What this does NOT change — stated explicitly, because a re-sequencing ADR is easy to over-read:**

- **[ADR-0035](#adr-0035) still picks Kavita over Komga.** That ADR answers *which* comics-and-books
  source; this one answers *when*. Its reversal of ADR-0032's Komga membership stands untouched.
- **§16.1's post-v0.1 sequence keeps its order: Navidrome, then Audiobookshelf, then Komga.** Kavita
  is lifted out of slot #1 into v0.1 and the remaining three shift up by one **without reordering**.
- **Komga is still last**, for the reason §16.0 gives and this ADR cannot improve on: nobody on this
  project can point it at a real library — which is, note, the same test this ADR has just applied to
  Sonarr and Radarr, applied consistently.
- **Navidrome must still precede v0.4**, whose success criterion needs a populated music replica. At
  #1 in the post-v0.1 sequence it satisfies that with more room than before, not less.
- **[ADR-0040](#adr-0040) stands whole.** The six subtype tables still land with the source that
  writes each; only *which milestone* three of them fall in moves, and it moves for exactly the reason
  ADR-0040 gave — the landing point is the source, not the date.
- **No ADR is reversed and no merged migration is edited.** `work.kind` keeps its full twelve-member
  `CHECK` from 00005, ADR-0030's `comic_issue` and ADR-0033's `person` were already paid for up front,
  and the provider registry absorbs the adapter swap by construction.
- **The success criterion itself is unchanged**, which is the whole point: *prove the replica thesis
  on real data.* This ADR changes the source so the criterion can be met, rather than changing the
  criterion so the source can be kept.

### 📋 Proposed replacement for ARCHITECTURE §16.1's v0.1 entry — WRITTEN HERE, NOT APPLIED

**§16 is authoritative for scope and this thread does not own it.** Per `DEVELOPMENT.md` §11 the
amendment is routed to the thread that does, exactly as [ADR-0040](#adr-0040) routed its own. The
current text is quoted first so the diff is legible; the italics, bolds and escapes are reproduced as
they appear in the source.

**Current, `ARCHITECTURE.md` §16.1, verbatim:**

```
**v0.1 — "It reads your library, it is fast, and you can act on it."**
Go binary + embedded SPA; SQLite + WAL with the §7.7 discipline; goose migrations. **Tier 0 Go
adapters** for **Sonarr and Radarr** — the \*Arr library sync, which is what proves the replica
thesis on real data — plus **Prowlarr in Search-and-Grab mode** (§8.5), which is the request path for
**all six** media types. **No catalogue source ships in v0.1** (§16.0, ADR-0036): Navidrome,
Audiobookshelf, Kavita and Komga arrive one at a time afterwards, in §16.1's sequence. **No command
sinks** — no Lidarr, no LazyLibrarian, no Mylar3, no Kapowarr.
Sync channels **1, 3 and 4**: full import for every service; **channel 3 (`/history/since`) for
Sonarr and Radarr**, which is every library-bearing service v0.1 has; **reconciliation with 7-day
tombstones and both sweep guards** for everything. **Channel 3b is specified (§7.1a) and not built
here**, because the sources that need it are not here. SignalR and webhooks are **out**. **Minimal
write path** (`monitor`, `unmonitor`, `delete`, `add`) on the durable command queue; no optimistic
apply.
```

**Proposed:**

```
**v0.1 — "It reads your library, it is fast, and you can act on it."**
Go binary + embedded SPA; SQLite + WAL with the §7.7 discipline; goose migrations. **The sync core,
with one Tier 0 Go adapter in front of it: Kavita** — which is what proves the replica thesis on real
data, because it is the source the owner runs and the only one whose delta has been verified against
a live instance ([ADR-0035](./DECISIONS.md#adr-0035) §2a) — plus **Prowlarr in Search-and-Grab mode**
(§8.5), which is the request path for **all six** media types. **Sonarr and Radarr re-sequence out of
v0.1** (ADR-0041): the owner runs neither, so *"a real Sonarr and a real Radarr, imported"* was a
criterion no v0.1 could meet. **They are re-sequenced, not cut** — they arrive onto a sync core
already proven on real data. **No command sinks** — no Lidarr, no LazyLibrarian, no Mylar3, no
Kapowarr.
Sync channels **1, 3b and 4**: full import; **channel 3b (§7.1a) — the ordered page walk with a
client-side stop — for Kavita**, which is every library-bearing service v0.1 has, and which is
therefore **built here rather than only specified** (ADR-0041). **Channel 3 (`/history/since`) is not
applicable to Kavita** (§7.1a) and lands with the first \*Arr adapter. **Reconciliation with 7-day
tombstones and both sweep guards** for everything — and it carries more weight here than it would for
an \*Arr, because a page walk cannot observe a deletion (§7.1a) and Kavita's watermark moves on a
chapter *add* only (ADR-0035 §2a clause (c)). SignalR and webhooks are **out**. ⚠️ **The minimal
write path** (`monitor`, `unmonitor`, `delete`, `add`) on the durable command queue **had only \*Arr
targets and now has none** — whether it re-sequences with them or stays for Prowlarr's grab path
alone is this section's call to make; no optimistic apply either way.
```

⚠️ **2026-08-17 — the proposed block above has been applied to §16, and its own last clause has since
been superseded there.** The block's closing ⚠️ sentence left the write path undecided —
*"whether it re-sequences with them or stays for Prowlarr's grab path alone is this section's call to
make"*. **§16 no longer reads that way:** [ADR-0042](#adr-0042) settled it, and §16's v0.1 entry now
states the answer rather than the question. The block is left standing unedited because it is a dated
record of what was proposed, not a description of what §16 says today — read §16 for that.

**Two riders for whoever applies it**, both consequences of this ADR that fall outside the v0.1 entry
itself: **(i)** §16.1's numbered table drops Kavita from row #1 and renumbers Navidrome, Audiobookshelf
and Komga to #1–#3 **without reordering them**, with a line recording that Kavita moved *into* v0.1
rather than being cut; **(ii)** §16.0's *"what the six-type claim honestly is in v0.1"* paragraph reads
**"the catalogue is film and TV"** and should read **books and comics/manga**, and its *"the honest
payment is that v0.1 ships no catalogue source at all"* paragraph is the prose this ADR amends. The
*"Schema, enumerated"* clause needs `work_book` / `work_comic` / `work_comic_issue` moved into v0.1 —
which is the same clause [ADR-0040](#adr-0040) already flagged to this section, so the two edits should
be made together.

---

<a id="adr-0042"></a>
## ADR-0042 — v0.1's minimal write path re-sequences with the \*Arr adapters; Sonarr and Radarr stay on the roadmap

**Status:** Accepted · **owner-decided 2026-08-17** · **Answers the question
[ADR-0041](#adr-0041) flagged and deliberately did not decide** ·
**Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16**, whose v0.1 entry carried the open question and
now carries this answer · **Amends [ADR-0041](#adr-0041), [ADR-0036](#adr-0036) and
[ADR-0012a](#adr-0012a)** where their bodies assume a v0.1 queue writer ·
**Re-sequences, rejects nothing** — [ADR-0012a](#adr-0012a)'s queue design is untouched and Sonarr and
Radarr both still arrive · ⚠️ **Amended 2026-08-17 by [ADR-0045](#adr-0045)** — see the block below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0045](#adr-0045): this ADR's decision, its measurement and its seam
> all stand; its refusal to name a milestone is overtaken by an owner delegation it did not have.**
> Clause 5's *"This ADR does not assign them a milestone"*, alternative **(e)**'s *"picking a number
> here would be inventing a commitment nobody made"*, and the **🚩 Open question** at the foot of this
> record were all **correct when written** — no owner statement supported a version. The owner was
> asked and answered on 2026-08-17: *"idk lets just slot them in somewhere i reckon? i don't really
> care, whatever you think is best"*. **[ADR-0045](#adr-0045) takes that delegation and puts Sonarr,
> Radarr and this write path in v0.2**, on the ground that §16's v0.2 — the \*Arr-backed request flow
> — already required them: §8.3's capability filter can only route `Add` to an instance that
> advertises `Add`, and no v0.1 service does. **Nothing below is rewritten**; the three sites carry
> inline flags. `ARCHITECTURE.md` §16 remains authoritative for milestone membership.

### Context

**[ADR-0041](#adr-0041) moved Kavita into v0.1 as its catalogue source and took Sonarr and Radarr out,
and that left one clause of the milestone with nothing behind it.** §16.1's v0.1 entry named a
*"minimal write path (`monitor`, `unmonitor`, `delete`, `add`) on the durable command queue"*. **All
four of those verbs are Servarr operations.** Kavita is a read-only catalogue source with no command
sink ([ADR-0032](#adr-0032)), so after ADR-0041 the milestone specified a write path whose every verb
addressed a service the milestone no longer contains. ADR-0041 flagged exactly this and refused to
settle it, because §16 is authoritative for scope and that ADR did not own §16:

> *"⚠️ **v0.1's minimal write path loses its target, and that question is NOT decided here.** …
> Whether that write path re-sequences with the \*Arr adapters, or stays and is exercised only by
> Prowlarr's grab path, is a scope call that belongs to whoever owns §16."*

**The tempting answer — "it stays, Prowlarr's grab path exercises it" — is false on the tree, and
this was measured rather than assumed.** The shipped grab path does not touch the queue and never did:

- **`internal/httpapi/grab.go`'s `handleGrab` dispatches synchronously.** It reads the candidate
  in scope, resolves a searcher, and calls `searcher.Grab(ctx, rscope, candidateID)` inline under a
  `grabTimeout`, then writes `audit_log` and `provenance`. There is no enqueue, no `202
  {command_id}`, and no worker.
- **`grab` is max one attempt by design**, so the queue's retry machinery is switched off for it in
  any case. §7.6: *"`grab` is max one attempt plus a manual button, because a blind retry is a double
  download"*, restated as decision 4 of [ADR-0012a](#adr-0012a).
- **Nothing anywhere writes `write_queue`.** The only non-test references under `internal/` and
  `cmd/` are the standalone bench binary in `internal/db/spike/` (behind `//go:build bench`, and it
  *reads*) and a comment at `internal/httpapi/grabs.go:58` saying so. Migration 0005's own rebuild
  guard says the same in its abort message.

So the two options ADR-0041 offered were not symmetric. "It stays for Prowlarr's grab path alone"
would not have kept an existing user of the queue alive; it would have committed v0.1 to *building*
its first queue writer, its worker and its verification loop for a path that today completes inside
one HTTP handler and is explicitly barred from retrying.

**The owner was asked and answered.** His words, verbatim, 2026-08-17:

> *"sure that's fine. we should add them though or at least the capability to add them as they're
> very popular services."*

He approves the re-sequencing and attaches one condition, which is the second half of that sentence
and is written into the Decision below rather than left to be inferred.

### Decision

> **1. The minimal write path re-sequences out of v0.1 — specified, not built.** The four verbs
> `monitor`, `unmonitor`, `delete` and `add`, the durable command queue's worker, and its
> `pending → inflight → verifying → done | failed` settlement loop **land with the first \*Arr
> adapter**, which is the milestone that first supplies a service they can address. §16 is amended
> to say so in place of the open question.
>
> **2. This removes nothing that works today.** The Prowlarr Search-and-Grab path is untouched and
> stays in v0.1: it dispatches synchronously in `handleGrab`, is max-one-attempt, and has never
> enqueued anything. **v0.1 therefore still has a write path** — grab is it, and it is the only one.
>
> **3. [ADR-0012a](#adr-0012a)'s design is not reopened.** The queue's state machine, its
> `UNIQUE (user_id, idempotency_key)` rule, the `verifying` TTL and the reconciliation guard all
> stand exactly as decided. What moves is *when the first writer is built*, not what it will do.
>
> **4. The seam is already paid for, and it costs no migration to use.** `write_queue` ships today as
> a table with all three of its indexes and no writer. [ADR-0039](#adr-0039) left its `state`
> vocabulary **unconstrained** — 00005 creates the column with an explicit *"NO CHECK"* and the
> reason beside it — and `kind`, the verb half of the same row, **has never carried a `CHECK`
> either**. A writer can therefore be added later **with no migration at all**: the table, the
> partial index `ix_wq_runnable` and the unique idempotency index are already there, and neither the
> verbs nor the states need widening. This is the seam `CLAUDE.md` asks for — the seam ships, the
> feature does not.
>
> **5. Sonarr and Radarr stay on the roadmap, and the capability to add them is preserved.** This is
> the owner's condition, in his terms: *"we should add them though or at least the capability to add
> them as they're very popular services."* **They are re-sequenced, not cut, and neither is this
> write path.** Nothing already built on `internal/servarr` is discarded — Prowlarr Search-and-Grab
> runs on it today — and no schema, index or state vocabulary that the \*Arr write path will need is
> given up here. **This ADR does not assign them a milestone**, because the owner named none and
> §16.1's table does not hold a slot for them; §16 is authoritative for when, and the gap between
> *"they arrive"* and *"they arrive at milestone N"* is recorded below as an open question rather
> than closed by invention.
> ⚠️ **2026-08-17, [ADR-0045](#adr-0045): they have a milestone now — v0.2.** Taken on an owner
> delegation this ADR did not have. The refusal above is left standing because it was right for the
> information it had; §16's v0.2 entry carries the assignment and the argument.

### Alternatives considered

- **(a) Keep the write path in v0.1 and let Prowlarr's grab path be its one user.** ⚠️ **The
  strongest alternative, because it is the one ADR-0041 named first and it preserves the milestone
  label unchanged.** It fails on measurement, not on taste. The grab path does not use the queue
  today, so this is not "keep an existing user" — it is "rewrite the one shipped write path to go
  through a queue it was deliberately built to bypass". And it would buy the least useful possible
  exercise of that queue: `grab` is barred from retrying (§7.6, ADR-0012a decision 4), so the retry
  sweep, the backoff, `next_attempt_at` and `ix_wq_runnable`'s whole reason for existing would remain
  unexercised. A queue demonstrated only on the one verb that may not retry has not been demonstrated.
- **(b) Keep the four verbs in v0.1 and point them at Kavita.** Rejected on the source. Kavita has no
  command sink at all ([ADR-0032](#adr-0032)); there is no `monitor`, no `unmonitor` and no `add` to
  address, and a `delete` against a read-only catalogue source is a deletion of the owner's files by
  a hub that principle 2 says does not own bytes. There is no honest mapping to build.
- **(c) Cut the write path rather than re-sequence it.** Rejected, and it is the reading the owner's
  answer specifically forecloses. It would also throw away a paid-for seam for no saving: the table
  and its indexes exist and are exercised by the query-plan assertions in `make test` today.
- **(d) Re-sequence the four verbs but build the worker and the retry sweep in v0.1 anyway, so the
  machinery is ready.** Rejected by *"cut before you add"* and by 00001's own rule, quoted in §16 —
  *"a migration that creates a table nothing queries is a schema claim nobody has tested"*, whose
  code equivalent is a worker that claims rows nothing enqueues. A settlement loop with no producer
  cannot be tested against anything real, which is the same defect [ADR-0041](#adr-0041) rejected
  alternative (d) for.
- **(e) Answer the scope question and also assign Sonarr and Radarr a milestone number, to make
  *"still on the roadmap"* concrete.** ⚠️ **Tempting, and refused deliberately.** The owner said they
  should be added; **he did not say when**, and §16.1's post-v0.1 table has three slots — Navidrome,
  Audiobookshelf, Komga — none of them theirs. Picking a number here would be inventing a commitment
  nobody made, in an ADR whose whole subject is a clause that outlived the fact it rested on. The gap
  is real and is recorded as an open question below, where §16's owner can close it with the owner's
  input rather than with this ADR's guess.
  ⚠️ **2026-08-17, [ADR-0045](#adr-0045): closed exactly that way — with the owner's input, not with a
  guess.** He delegated the choice, and v0.2 was picked because §16's v0.2 already required a service
  that advertises `Add` (§8.3) and no v0.1 service does. This alternative's objection stands as
  written: a number invented *here* would still have been invention.

### Consequences

**What this changes:**

- **`ARCHITECTURE.md` §16 loses its open question and gains this answer.** The v0.1 entry's ⚠️ clause
  — *"whether it re-sequences with them or stays for Prowlarr's grab path alone is this section's
  call to make"* — is replaced by the call. Four further sentences in §16 that assumed a v0.1 queue
  writer are amended in place, each quoting what it used to claim.
- **v0.1 has no command sink of any kind, and §16's list of absent sinks is no longer exhaustive as
  written.** *"No command sinks — no Lidarr, no LazyLibrarian, no Mylar3, no Kapowarr"* enumerated the
  four ADR-0032 deferred, at a time when Sonarr and Radarr were v0.1's *kept* sinks. After ADR-0041
  and this ADR the honest statement is **none at all**, and §16 now says that.
- **§16.0's libraries justification loses one of its three grounds and keeps the other two.** It read
  that a library binding *"carries the request destination that v0.1's one write path routes on"*.
  The write path it named is no longer in v0.1. The subsystem stays, on the two grounds that do not
  depend on it — its four tables are owed by v0.1 either way, and its screen is one of the five
  `CLAUDE.md` names essential. **The third ground is withdrawn outright, not narrowed**, and §17.8 has
  since taken it further than this decision alone would have: **no service v0.1 connects can be a
  library's request sink at all** — a sink must advertise `Add` under §8.3's capability filter, and
  the Prowlarr path does not, because it posts a release to Prowlarr's own download client (§8.5). So
  the destination is not merely un-routed-on in v0.1; it **cannot be set**, and §17.8 drops its column
  from the Libraries row view for the milestone. **Both are sequencing, not cuts.**
- **The `Recent grabs` cost estimate's `write_queue` half is now absent by decision, not by
  sequencing accident.** §16 already recorded that the shipped `GET /api/v1/grabs/recent` has no join
  and no queue state *"because nothing writes `write_queue` yet"*. **The *yet* is what changes**: no
  v0.1 work will write it, so the block's queue-state column is a post-v0.1 addition rather than a
  gap v0.1 still owes. The estimate itself is left standing, unretrofitted, for the reason §16
  already gives.
- **`write_queue` stays in the schema with no writer for the whole of v0.1.** That is deliberate and
  it is the seam, not an oversight. Migration 0005's rebuild guard and
  `internal/httpapi/grabs.go:58`'s comment both already document the state of affairs; nothing needs
  to change in the tree.
- **[ADR-0039](#adr-0039)'s outstanding obligation acquires an owner and a milestone.** Its corrected
  decision 1 says the Go `state` vocabulary declaration and validation are *"owed by whoever writes
  the first `write_queue` writer"* and that nothing validates it today. **That writer is now known to
  be the first \*Arr adapter's**, not v0.1's — which is a narrowing of the obligation, not a release
  from it.

**What this does NOT change — stated explicitly, because a re-sequencing ADR is easy to over-read:**

- **[ADR-0012a](#adr-0012a) stands whole.** No state, no guard, no idempotency rule and no TTL is
  altered. Only one sentence of its body is flagged, and only because it says *"every v0.1 write"* of
  writes that are no longer v0.1's.
- **The Prowlarr Search-and-Grab path is untouched**, including its synchronous dispatch, its
  max-one-attempt rule, its audit rows and the `Recent grabs` block that reads them.
- **No schema change and no migration.** `write_queue`, its three indexes, the unconstrained `state`
  and `kind` columns and the restored `work_id` foreign key all stay exactly as 00005 left them.
- **No ADR is reversed.** [ADR-0041](#adr-0041) is completed, not corrected: it asked this question
  and routed it here.
- **Sonarr and Radarr are not cut**, and neither is the write path. See decision 5 — this is the
  owner's stated condition and it is the sentence most at risk of being quietly dropped later.

### 🚩 Open question this ADR raises and does not answer

> ✅ **ANSWERED 2026-08-17 by [ADR-0045](#adr-0045), owner-delegated: Sonarr, Radarr and this write
> path all land in v0.2.** The question below is left standing as it was asked — it is what prompted
> the owner to be asked at all, and its *"two commitments pointing at a milestone that does not exist
> is a thinner position than one"* is the argument that got it closed rather than a claim that has
> gone stale.

**Sonarr and Radarr have no milestone.** §16.1's post-v0.1 table numbers three catalogue sources and
holds no slot for either; §16's prose says only that they *"arrive too, onto a core already proven"*.
This ADR now hangs a second thing off that unnumbered arrival — the minimal write path. **Two
commitments pointing at a milestone that does not exist is a thinner position than one**, and the
owner's *"we should add them"* is a reason to close the gap rather than to leave it. It is raised here
for §16's owner and the project owner to settle together; it is not closed here, because no owner
statement supports a number and this ADR will not invent one.

---

<a id="adr-0043"></a>
## ADR-0043 — A **minimal** match-correction UI moves earlier than v0.3; the full correction surface stays there

**Status:** Accepted · **owner-decided 2026-08-17** · **Answers the scope call
[`ARCHITECTURE.md`](./ARCHITECTURE.md) §6.4 flagged and deliberately routed** ·
**Amends [ADR-0026](#adr-0026)**, whose consequence bullet capped the correction UI at v0.3 on a
ground [ADR-0041](#adr-0041) falsified · **Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16.0 and
§16.1**'s v0.1 and v0.3 entries · **Re-sequences, rejects nothing** — ADR-0026's binding model, its
four verbs and its four tables are untouched · **Assigns no milestone**, which is recorded as an open
question rather than closed by a guess · ⚠️ **Amended 2026-08-17 by [ADR-0045](#adr-0045)** — see the
block below.

> ⚠️ **AMENDED 2026-08-17 by [ADR-0045](#adr-0045): every clause of this ADR's decision stands except
> its refusal to name a milestone, which is overtaken by an owner delegation it did not have.**
> Decision clause **3**'s *"No milestone is assigned, and that refusal is deliberate"*, alternative
> **(c)**'s *"Assign the minimal case to v0.2 — or to v0.1 — … Tempting and refused"*, and the
> **🚩 Open question** at the foot of this record were **correct when written**: the owner had said
> *"earlier"* and named no version. He was asked and answered on 2026-08-17 — *"idk lets just slot
> them in somewhere i reckon? i don't really care, whatever you think is best"* — and
> **[ADR-0045](#adr-0045) puts the minimal case in v0.2**, ⚠️ **by elimination rather than by fit**,
> with the cost this ADR's open question predicted written down rather than argued away: **v0.1 ships
> the badge without its remedy for one whole milestone.** **Clauses 1, 2, 4 and 5 are untouched** —
> *"minimal"* is still the boundary, the decision is still source-independent, the full four-verb
> surface is still v0.3's, and [ADR-0026](#adr-0026)'s design is still not reopened. **Nothing below
> is rewritten**; the three sites carry inline flags.

### Context

**[ADR-0026](#adr-0026) capped the correction UI at v0.3, and it gave a reason.** Its consequence
bullet reads *"The correction **UI** is capped to v0.3, where the weak catalogues actually arrive —
§6.4 already establishes that tier 1 resolves essentially 100% of the v0.1 identity problem, so there
is nothing to correct before then."* §16.0 restated the same cap and called it *"a scheduling
detail, not the payment"*.

**[ADR-0041](#adr-0041) falsified both halves of that ground on the same day, in one move.** It put
**Kavita in v0.1** as the sync core's first adapter, which does two things at once:

- **A weak catalogue now arrives in v0.1, not at v0.3.** *"where the weak catalogues actually arrive"*
  was a statement about the roadmap ADR-0026 was written against, and the roadmap moved.
- **The providers §6.4's percentage was measured on are no longer v0.1's.** *"Essentially 100%"* was
  established for **Sonarr and Radarr**, every row of which carries `tmdbId` / `imdbId` / `tvdbId`.

**The identity finding, verified in this repo rather than taken from the brief.** Both sources agree
and neither is inference:

- **[ADR-0035](#adr-0035) §1**, verbatim: *"**Kavita's `aniListId`, `malId`, `comicVineId` and the rest
  are null without Kavita+.**"* It goes on that under that ADR this becomes *"what an ordinary user
  sees once Kavita lands, not a documented edge case"*, and it requires the screens to render null
  identifiers as *"the normal case"*. The *"not in v0.1"* rider on that requirement is itself **struck
  at its own site** by ADR-0041.
- **`ARCHITECTURE.md` §6.4**, restated against Kavita after ADR-0041, checks the same fact against the
  vendored spec: `SeriesDto` in `api/specs/kavita.json` carries `aniListId`, `malId`, `hardcoverId`,
  `metronId` and `cbrId` as integers and `comicVineId` and `mangaBakaEditionId` as nullable strings,
  **all written only by the Kavita+ match path**, so a free instance returns `0`, `null` or `""` for
  every one. Its conclusion: *"How much of v0.1's identity problem tier 1 resolves is now **a property
  of the instance, not of the design** — essentially all of it on a Kavita+ install, close to none on
  a free one."*

**§6.4 then withdrew the cap's support and refused to take the call, on ownership grounds.** Its own
words: *"the restatement **withdraws the support the cap was resting on**: a v0.1 whose only source may
carry no external ids at all is a v0.1 where a user has something to correct on day one, which is not
what 'a cap on a declared no-op' describes. Whether the cap survives is a **scope** question, scope is
owned by the ADRs with §16 authoritative … **it needs an ADR and an owner decision.**"* This ADR is
that ADR, and the owner decision it needed is below.

**The owner was asked and answered. His words, verbatim, 2026-08-17:**

> *"but yeah, i guess we can do a minimal fix this match thing earlier. in case anyone is running
> kavita. but bookorbit seems to be the new meta tbh."*

**Three things are read out of that sentence and nothing more.** *(1)* **"minimal"** and *(2)*
**"earlier"** are decided, and both are load-bearing. *(3)* **"in case anyone is running kavita"** is
his **rationale**, and it is attributed to him rather than presented as a fact about the roadmap: he
does not run the case he is protecting — he is scoping for **other people's installs**, which is
principle 3 applied by the owner to a user who is not him. The third clause, *"bookorbit seems to be
the new meta"*, is **not** read as a decision; see the Decision's clause 4.

### Decision

> **1. The v0.3 cap on the correction UI is lifted for the minimal *"fix this match"* case, and holds
> for everything else.** [ADR-0026](#adr-0026)'s cap is amended, not reversed: **the full four-verb
> surface — `exclude`, `include`, `relink`, `field` — plus the Corrections list still land at v0.3.**
> What comes earlier is the narrow case a user meets on the first screen of a catalogue whose source
> supplied no external ids: an item shown as *"not identified"*, or bound to the wrong `work`, and a
> way to say so.
>
> **2. "Minimal" is the owner's word and it is a constraint on scope, not a synonym for "small".** It
> is what makes this a re-sequencing rather than an addition, and *"cut before you add"* is satisfied
> by the split itself: the part that moves is bounded by what the *"fix this match"* case needs, and
> **everything outside that boundary stays at v0.3.** 🔍 **Marked as inference, because the owner did
> not enumerate verbs:** the case he named is an identity correction, which is ADR-0026's **`relink`**
> — repointing a `service_item_link` at the right `work` — and **not** `exclude` / `include`, which
> are library **membership** and answer a different complaint. `field` sits between the two and is not
> claimed here either way. **The exact surface is settled by the milestone that takes it**, against
> this boundary; this ADR fixes the boundary, not the widget.
>
> **3. No milestone is assigned, and that refusal is deliberate.** The owner said *"earlier"*; **he
> named no version**, and `ARCHITECTURE.md` §16 is authoritative for milestone membership. Naming
> v0.2 — or v0.1 — here would be inventing a commitment nobody made, which is the exact failure
> [ADR-0042](#adr-0042) refused for Sonarr and Radarr the same day. §16.0 records that **the slot is
> not yet assigned**; the gap is carried as this ADR's open question.
> ⚠️ **2026-08-17, [ADR-0045](#adr-0045): the slot is assigned — v0.2**, on the owner's delegation.
> The refusal above is left standing because it was right for the information it had: it says a
> version invented *here* would be a commitment nobody made, and that is still true. §16's v0.2 entry
> carries the assignment, the elimination argument and the cost.
>
> **4. This decision is source-independent and survives a change of catalogue source.** What it turns
> on is that **v0.1's catalogue source has weak identity**, which is a property of the source and not
> of its name. 🔍 **`BookOrbit` is under live evaluation on this project as of 2026-08-17 and nothing
> about it is decided** — it is named only because the owner named it, no ADR has taken it, and §16
> assigns it nothing. **Whichever server ends up in that slot, clauses 1–3 stand.** The only thing
> that would reopen them is a v0.1 source that supplies **strong external ids for the ordinary,
> unpaid user** — a factual claim about a specific source, needing the same primary-source check
> against a vendored spec that Kavita's got (§6.4), and no source now under evaluation has had it.
>
> **5. Nothing about [ADR-0026](#adr-0026)'s design is reopened.** The binding model, the single-kind
> rule, the four verbs, `library_id IS NULL` for `relink` and `field` with its `CHECK`, the four
> tables, `target_identity_hash` and the deliberate absence of a foreign key on the override's
> `work_id` / `link_id` all stand exactly as decided.

### Alternatives considered

- **(a) Keep the v0.3 cap and rely on the *"not identified"* badge alone until then.** ⚠️ **The
  strongest alternative, and the one the status quo already implements.** The badge and its column are
  v0.1 work regardless — §6.4 and §16 both say they cannot be retrofitted — and *"failing to identify
  is honest"* is this project's own rule. **It fails on what it leaves the user with:** the badge
  tells a user their library is wrong and offers nothing to do about it, on a catalogue where
  ADR-0035 §1 says that state is **the majority path, not the exception**. A screen that is honest
  about a defect it forbids you to fix is a worse product than one that is silent about it, and the
  owner's answer is a direct rejection of this option.
- **(b) Move the whole ADR-0026 correction surface earlier.** Rejected by *"cut before you add"* and
  by the owner's own word. Four verbs plus a Corrections list is a subsystem; moving it into an
  earlier milestone would need a statement of what leaves to pay for it, and there is none. **The
  owner said "minimal", and this alternative is the reading that ignores the adjective.**
- **(c) Assign the minimal case to v0.2 — or to v0.1 — to make *"earlier"* concrete.** ⚠️ **Tempting
  and refused, for the reason [ADR-0042](#adr-0042) gave in the same position a day earlier.** *"He
  did not say when"* applies verbatim: he said "earlier", the milestones are §16's to allocate, and a
  number invented in an ADR whose whole subject is a clause that outlived its evidence would be the
  same mistake one turn later. Recorded as an open question instead.
  ⚠️ **2026-08-17, [ADR-0045](#adr-0045): this alternative is now the decision, taken elsewhere and on
  new information.** *"He did not say when"* stopped being true — asked directly, he delegated — and
  **v0.2 is the answer, chosen by elimination**: v0.1 had no *"cut before you add"* payment to name
  after ADR-0041 and ADR-0044. The objection above is not withdrawn; it is satisfied, because the
  number was allocated by §16 rather than invented in an ADR.
- **(d) Reverse [ADR-0026](#adr-0026)'s consequence outright rather than amend it.** Rejected on this
  file's own convention. The bullet is a **dated record of a decision as taken**, and it was right
  against the roadmap it was written against — v0.1 genuinely had strong-id sources at the time. It is
  **struck in place with the correction beside it**, per the preamble's *"How an ADR is amended when
  the world moves under it"*, which is exactly the rule the `162dca5` incident recorded there exists
  to protect.
- **(e) Wait for the BookOrbit evaluation to conclude, then decide.** Rejected because **the decision
  does not depend on its outcome** (clause 4). Blocking an owner-taken scope call on an unrelated
  live evaluation would leave the falsified cap standing in two documents for no gain, and if the
  evaluation *did* change the answer it would do so by supplying a strong-id source — which reopens
  this ADR cleanly rather than needing it withheld.

### Consequences

**What this changes:**

- **[ADR-0026](#adr-0026) gains all four marks the preamble requires** — index row, `Status:` line, a
  dated `> ⚠️ **AMENDED …**` block, and an inline `~~`-struck flag at the falsified bullet itself.
  Nothing in its body is rewritten.
- **`ARCHITECTURE.md` §16.0's kept-with-its-cost paragraph loses its open flag and gains this
  answer.** It had ended *"whether the correction UI's v0.3 cap still holds against a source with no
  ids is a live question this section flags rather than answers"*. It now records the decision, the
  three things the decision does **not** say, and the unassigned slot.
- **§16.1's v0.1 entry no longer reads *"the correction UI deferred to v0.3"* flat**, and **does not
  claim the minimal case for v0.1 either.** §16.1's v0.3 entry keeps the full surface and says what
  left it.
- **A user has something to correct on day one and, when the minimal case lands, somewhere to do it.**
  That is the whole user-visible effect.

**What this does NOT change — stated explicitly, because a re-sequencing ADR is easy to over-read:**

- **No schema change and no migration.** `library_override` and its `CHECK` are in the tree already
  (`internal/db/migrations/00005_library_sync.sql`); the correction *storage* was never the deferred
  part. **The seam ships and has shipped; this moves part of the feature.**
- **[ADR-0035](#adr-0035)'s choice of source, [ADR-0041](#adr-0041)'s milestone move and
  [ADR-0042](#adr-0042)'s write-path re-sequencing are all untouched.** This ADR consumes their
  results; it revisits none of them.
- **No ADR is reversed.** ADR-0026 is amended in one consequence.
- **`ARCHITECTURE.md` §6.4 is not edited here.** It owns the identity cascade and its restated tier-1
  claim is this ADR's evidence, not its subject; its 🚩 flag that the call *"needs an ADR and an owner
  decision"* is answered by this record and its wording is §6's thread to update.

### 🚩 Open question this ADR raises and does not answer

> ✅ **ANSWERED 2026-08-17 by [ADR-0045](#adr-0045), owner-delegated: v0.2.** The question below is
> left standing as it was asked, because **it framed the trade correctly and the answer accepts the
> half it warns about**: v0.2 does ship the badge without its remedy for one whole milestone, on a
> catalogue where the badge is the majority state. v0.1 was refused for the reason the question itself
> names — it *"needs a *cut before you add* payment named"*, and none can be. **The pattern the last
> two sentences flag — two commitments in two days on an unnumbered slot — is what got the owner
> asked**, and ADR-0045 closes both.

**The minimal match-correction case has no milestone.** §16 is authoritative for membership and this
ADR deliberately assigns none, because the owner said *"earlier"* and named no version. The candidate
answers are not equivalent — **v0.1** makes it concurrent with the source that motivates it and needs
a *"cut before you add"* payment named, while **v0.2** ships the badge without its remedy for one
whole milestone on a catalogue where the badge is the majority state. **This is the second commitment
in two days hung on an unnumbered slot** — [ADR-0042](#adr-0042) left Sonarr, Radarr and the minimal
write path in the same position — and that pattern is itself worth the owner's attention. It needs his
input, not an agent's guess.

---

<a id="adr-0044"></a>
## ADR-0044 — Author and creator credits ship in v0.1; `work_credit` lands with Kavita, not Navidrome

**Status:** Accepted · **owner-decided 2026-08-17** · **Applies [ADR-0040](#adr-0040)'s rule rather
than overriding it** — the landing point is the source that writes the table, and the source that
writes `work_credit` turned out to be Kavita · **Amends [ADR-0040](#adr-0040)** for one of its six
tables and leaves the other five exactly where they were · **Confirms [ADR-0033](#adr-0033)**, whose
`person` kind finally acquires a writer · **Changes nothing about [ADR-0035](#adr-0035),
[ADR-0041](#adr-0041) or [ADR-0042](#adr-0042).**

### Context

**A book catalogue with no authors is barely a catalogue, and that is what v0.1 had.**
[ADR-0040](#adr-0040) scheduled `work_credit` with `work_album` and `work_track` under **Navidrome**,
and that grouping was reasonable when it was made: the first catalogue source was assumed to be a
music server, ADR-0031 had introduced `work_credit` in a paragraph about Various-Artists compilations
and classical roles, and *"a credit"* therefore meant a performer.

**[ADR-0041](#adr-0041) then made Kavita the first adapter, and Kavita writes books and comics.**
The grouping did not survive that, and the migration that shipped the other three tables said so at
the time. `00006_kavita_subtypes.sql`'s header states the consequence in terms, and it is quoted here
because it is the finding this ADR answers:

> ⚠️ AND THE CONSEQUENCE OF DEFERRING work_credit IS STATED RATHER THAN DISCOVERED, because it is the
> one that bites in a comics milestone and not in a music one: work_credit is where an author, writer,
> penciller, inker, colorist, letterer or cover artist would be stored (its `role` CHECK in schema.md
> §1.1 names all of them), and it is not here. So any creator a Kavita series reports has NOWHERE TO
> LAND in v0.1 — not a lossy landing, none at all.

**Kavita does report them.** `GET /api/Series/metadata?seriesId=N` returns a `SeriesMetadataDto` with
**thirteen** person arrays — `writers`, `coverArtists`, `publishers`, `characters`, `pencillers`,
`inkers`, `imprints`, `colorists`, `letterers`, `editors`, `translators`, `teams`, `locations` — each
an array of `PersonDto`, verified against the vendored `api/specs/kavita.json` rather than recalled.
Eight of those thirteen are creators and every one of the eight has a member in `work_credit.role`'s
own `CHECK`.

**So ADR-0040's rule points at Kavita once you notice Kavita writes credits.** That is the shape of
this decision: not *"pull a deferred table forward"*, which is what ADR-0040 exists to refuse, but
*"apply ADR-0040's rule to a fact about the source that was not known when the table was filed"*.

**The owner was asked and approved it.** The cost was put to him first — see the Decision's clause 3
— and his answer was: *"yea that sounds good to me"*.

### Decision

> **1. `work_credit` lands in v0.1, with Kavita, in a new migration
> (`00007_work_credit.sql`).** ADR-0040's rule is applied, not overridden: each subtype table lands
> with the catalogue source that writes it, and Kavita is the source that writes this one.
>
> **2. `work_album` and `work_track` DO NOT MOVE. They still wait for Navidrome.** They are the two of
> the music three whose writer really is a music server — an album and a track are music objects, no
> v0.1 source produces either, and `internal/` still has no `navidrome` package. ADR-0040 stands whole
> for them, and this ADR reopens neither them nor the four-table half of ADR-0040 it does not touch.
>
> **3. THE COST IS A ROW, NOT A COLUMN, AND IT WAS PUT TO THE OWNER IN THOSE TERMS.**
> `work_credit.creator_work_id` is a foreign key into `work`, and the creator it points at is a work
> of kind **`person`** ([ADR-0033](#adr-0033)). **Nothing in v0.1 created a `person` work before this
> change.** So approving authors approves *authors as first-class rows* — the Kavita adapter now
> creates `work` rows of kind `person` as well as credit rows. That is the honest cost the owner
> accepted, and it is the reason this is an ADR rather than a migration.
>
> **4. `person` gets NO subtype table, and that absence is deliberate.**
> [`reference/schema.md`](./reference/schema.md) §6.1's rule is *"every `kind` has a subtype table or
> an explicit justification for not having one"*, and `person` has the justification: a credited human
> is a name, an optional set of `external_id` rows and the credits that point at it, all of which
> `work`, `external_id` and `work_credit` already carry. A `work_person` table would hold a birth year
> and a biography, which **no v0.1 source reports for an author**.
>
> **5. `person` stays out of the navigation enum, the Tier 1 client prefix index and the FTS corpus**,
> exactly as §6.1 already requires, and the search-document builder is what enforces it. A person hit
> would be a search result with nowhere to land, and — because every document is scoped through
> `search_doc_library` — an author would otherwise appear inside the user's library grid as an item.
>
> **6. The dedupe key is the normalized name**, under `kind = 'person'`, using the same v1 normaliser
> every other title in the schema is keyed by. Two books by one author are **one** person work. Both
> directions of error are carried openly rather than hidden: a name variant (*"A. Moore"* vs *"Alan
> Moore"*) **splits into two people**, and two humans sharing a name **merge into one**. See the
> consequences.
>
> **7. Kavita's own person id is NOT written as an `external_id`.** It is instance-local, so two Kavita
> installs would both claim person 5 and `ux_extid_work_strong` would read that collision as a **merge
> signal between two unrelated humans** — worse than the name collision it was meant to fix.

### Why the name is the key, when a stable id would be better

**There is no stable, global person identifier available to v0.1.** `PersonDto` carries `aniListId`,
`malId`, `hardcoverId` and `asin`, and all four are Kavita+ fields on the same present-and-empty
footing as the series-level ones (§6.4): a free instance returns `0`, `null` or `""`. The instance's
own `id` is disqualified by clause 7. So the alternative to the name is not *"a better key"* — it is
**a person work per credit per series**, which makes `work` a table of strings with foreign keys and
grows with credits rather than with the catalogue.

**The over-merge direction is the one that actually loses information**, and it is accepted on §6.4
rule 1's own ground: *"failing to identify is honest; merging wrongly is not"* — except that here the
merge is over a name, in a table with **no screen in any milestone**, where the visible consequence of
two John Smiths sharing a row is that a credit link (§17.6) leads somewhere slightly wrong rather than
that two catalogue items collapse. That is a materially smaller blast radius than a work merge, which
is why the same project that refuses fuzzy work merging accepts exact-normalized-name person merging.

**The seam for fixing the split direction exists and is not built.** `kavita.PersonDto` carries an
`aliases` array. Folding aliases into `work_alt_title` rows on the person work would let a later
matcher resolve *"A. Moore"* and *"Alan Moore"* to one row. That is a matcher, not a column, so it
waits — and it is cheap to add precisely because the person is already a `work`.

### Alternatives considered

- **Leave `work_credit` with Navidrome and ship v0.1 without authors.** Rejected by the owner. It is
  what ADR-0040 literally scheduled, and 00006's header already recorded that the result is *"not a
  lossy landing, none at all"* for every creator a Kavita library reports. A books-and-comics
  catalogue whose item pages cannot name an author is not the thing v0.1 exists to prove.
- **Add an `author TEXT` column to `work_book` and a `writer TEXT` to `work_comic`.** Rejected, and
  it is the tempting one because it costs no `person` rows. It reintroduces the exact defect
  [ADR-0031](#adr-0031) exists to prevent — *"the moment attribution is a scalar, Various-Artists
  compilations, collaborations and classical roles are unrepresentable"* — in the medium where it
  bites hardest: an ordinary comic has a writer AND a penciller AND an inker AND a colorist AND a
  letterer AND a cover artist, six roles that a scalar column cannot hold and that a comma-joined
  string turns into a parse. It would also have to be UNDONE, with a backfill, the day `work_credit`
  lands anyway.
- **Land all six of ADR-0040's tables now, since one is moving.** Rejected: it is the alternative
  ADR-0040 already weighed and refused, and nothing about Kavita changes the answer for `work_album`
  and `work_track`. *"Cut before you add"* is satisfied by moving exactly the table whose writer
  arrived and no others.
- **Create the `person` works but skip `work_credit`, storing the link as a `work.parent_work_id`.**
  Rejected as incoherent: a book has one parent (its series) and many creators, and the parent edge
  carries no role, no billing order and no co-credit.
- **Defer the whole thing to v0.2 with the request flow.** Rejected because it is not free to wait:
  `work_credit` is a brand-new table with no dependants *today*, which is ADR-0040's own reason such a
  table is cheap to add later — but every Kavita import that runs before it lands throws away credits
  that would then need a full re-import to recover, on a metadata endpoint that costs one HTTP GET per
  series.
- **Read `PersonDto.roles` instead of the array a person arrived in.** Rejected on a checked fact
  rather than a preference: `roles` is the set of roles that person holds **across the whole
  instance**, so a person who writes one series and colors another returns `{Writer, Colorist}` in
  both series' metadata, and reading it would credit a series' writer as its colorist.

### Consequences

* **`work` is no longer a table of catalogue items alone.** Every `SELECT … FROM work` written before
  this change may now see `person` rows, and the queries that must not — the grids, the corpus, the
  counts — need `kind <> 'person'` or an explicit kind filter. This is the sharpest consequence of the
  decision and it is stated first: `cmd/usarr`'s import end-to-end test had two assertions that
  counted `work` bare and both had to be narrowed.
* **The Kavita adapter now makes one HTTP GET per series.** `SeriesDto` carries no creator field, and
  the three bulk-shaped alternatives were checked against the vendored spec and cannot rebuild the
  mapping: `POST /api/Person/all` returns people with no series linkage, `GET
  /api/Person/series-known-for` is documented *"the top 20 series"*, and `GET
  /api/Person/chapters-by-role` is documented *"Limited to 20 results"*. The cost is budgeted the way
  [`reference/sync.md`](./reference/sync.md) §2 budgets the *Arr episode fetch — *"fetch them per
  parent, bounded"* — and it runs as a **phase-B pass after the item stream closes**, never inside its
  callback, which would hold the streaming connection open across N round trips.
* **Five of Kavita's thirteen person arrays are DROPPED, and every one of the thirteen is accounted
  for in code.** `publishers`, `imprints` and `teams` are **organisations**; `characters` and
  `locations` are not people at all. Kavita stores all five in the same table as its writers and UsArr
  does not inherit that conflation. `publishers` has a home in the schema — `work_comic.publisher`,
  which migration 0006 created and nothing writes — and this change deliberately does not write it,
  because that column belongs to the phase-B metadata backfill that also owns `summary` and
  `releaseYear`.
* **Kavita's `writers` array maps to two different roles by kind: `author` for a book, `writer` for a
  comic.** Kavita has **no** `authors` array — its Writer role is where an EPUB's `dc:creator` and a
  ComicInfo `Writer` element both land — so mapping it to `writer` everywhere would leave
  `work_credit.role`'s `author` member with no writer at all while filing every novelist under a
  comics role.
* **A work that two remote items resolve onto gets LAST-WRITER-WINS credits.** Credits are replaced
  wholesale per *work*, driven per *remote item*, so when tier 1 merges two Kavita series onto one work
  the second item's credit set replaces the first's. This is the same shape as
  [ADR-0041](#adr-0041)-era `work_comic.reading_direction` (`REVIEW-LOG.md` LS-07), it is asserted in
  a test rather than left to be rediscovered, and the alternative — accumulating across the remote
  items that share a work — is a merge, which v0.1 explicitly does not have.
* **`illustrator` and `narrator` remain members of `work_credit.role` with no writer**, alongside the
  seven music roles. They are in the `CHECK` because SQLite cannot `ALTER` one, not because something
  produces them.
* **"Find everything by this author" is still unanswered in v0.1**, and this ADR does not change it.
  `person` has no screen, so the credit is reachable as a link on an item (§17.6) and nowhere else.
  `ix_credit_creator` exists anyway and its v0.1 reader is the write path.
* **No `work.kind` member is added and no `kind_byte` changes.** `00005_library_sync.sql` paid for the
  full twelve-member `CHECK` — including `person` — precisely so that this would cost a plain
  `CREATE TABLE`. ADR-0030's and ADR-0033's up-front payment is what makes this migration cheap, which
  is the clearest vindication those two ADRs have had.

**What this does NOT change:**

- **The music three minus one.** `work_album` and `work_track` still wait for Navidrome, which still
  has no adapter. ADR-0040 is amended for `work_credit` alone.
- **[ADR-0035](#adr-0035), [ADR-0041](#adr-0041) and [ADR-0042](#adr-0042) all stand.** Kavita is
  still the comics-and-books source, still v0.1's first adapter, and the minimal write path is still
  re-sequenced with the \*Arr adapters. This ADR consumes their results and revisits none of them.
- **No merge machinery.** `work_merge` is still v1.0 and this change builds none of it; the person
  dedupe is a lookup on an indexed column, not a merge.
- **No new screen.** Rendering a credit is the item page's job and the item page is a later commit.

### 🚩 Open question this ADR raises and does not answer

**Nothing collects a `person` work that ends up credited on nothing.** A creator removed upstream
leaves its `work` row standing — harmless (no document, no library, no screen) and invisible, but
unbounded over years of re-imports on a churning library. Collecting it inside the per-work credit
replace is wrong, because the same person may be credited by another work in another batch, so the
right shape is a sweep, and a sweep is a subsystem this change deliberately does not add. It is
recorded here rather than guessed at, and `ix_credit_creator` is the index such a sweep would read.

---

<a id="adr-0045"></a>
## ADR-0045 — The three unslotted commitments land in v0.2: Sonarr and Radarr, the minimal write path, and the minimal match-correction UI

**Status:** Accepted · **owner-delegated 2026-08-17** · **Closes the open questions
[ADR-0042](#adr-0042) and [ADR-0043](#adr-0043) each raised and deliberately refused** ·
**Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16**, whose §16.0, §16.1-intro, v0.1, **v0.2** and
v0.3 entries all carried the unassigned slot · **Amends [ADR-0042](#adr-0042) and
[ADR-0043](#adr-0043)** at the clauses that record no milestone · **Assigns milestones and nothing
else** — no scope moves, no design is reopened, and no ADR is reversed.

### Context

**Two ADRs closed on the same day with the same hole in them, and both were right to leave it.**
[ADR-0042](#adr-0042) re-sequenced v0.1's minimal write path out of the milestone and recorded, in its
decision clause 5, that *"this ADR does not assign them a milestone, because the owner named none"*;
its alternative (e) refused the number outright — *"picking a number here would be inventing a
commitment nobody made"*. [ADR-0043](#adr-0043) lifted ADR-0026's v0.3 cap for the minimal *"fix this
match"* case and made the identical refusal in its decision clause 3 and its alternative (c), for the
identical reason. **Both refusals were correct at the time**: `ARCHITECTURE.md` §16 is authoritative
for milestone membership, and neither ADR had an owner statement that supported a version.

ADR-0042's own open question says why the hole could not simply be left: *"Two commitments pointing at
a milestone that does not exist is a thinner position than one."* ADR-0043's says the same one turn
later, and adds that *"this is the second commitment in two days hung on an unnumbered slot … that
pattern is itself worth the owner's attention."* Both routed the question to the owner.

**The owner was asked and answered, verbatim, 2026-08-17:**

> *"idk lets just slot them in somewhere i reckon? i don't really care, whatever you think is best"*

**That is a delegation, not an assignment, and this ADR treats it as one.** He supplies the authority
to choose and no preference to follow, so the choice has to stand on the repository's own logic and be
written down where it can be checked later — *"the owner didn't mind"* is not an argument anyone can
audit. The three things needing a slot:

1. **Sonarr and Radarr as services.** Moved out of v0.1 by [ADR-0041](#adr-0041); ADR-0042 clause 5
   records that they are **re-sequenced, not cut**, with the capability preserved, at the owner's
   explicit condition (*"we should add them though or at least the capability to add them as they're
   very popular services"*). No milestone.
2. **The minimal write path** — `monitor`, `unmonitor`, `delete`, `add`, plus the durable command
   queue's worker and its `pending → inflight → verifying → done | failed` settlement loop.
   ADR-0042 gave it a **relative** slot, *"lands with the first \*Arr adapter"*, which moves wherever
   (1) moves.
3. **The minimal match-correction UI.** ADR-0043 constrained it to *"earlier than v0.3"* and to
   *"minimal"*, both the owner's words, with [ADR-0026](#adr-0026)'s full four-verb surface staying at
   v0.3. No milestone.

### Decision

> **1. All three land in v0.2.** §16's v0.2 entry is amended to say so and to state what the
> milestone now contains.
>
> **2. For (1) and (2) this is not an addition to v0.2 — it is writing down a dependency v0.2 already
> had.** §16's v0.2 entry is *"Requests"*, and its headline is the **\*Arr-backed request flow**:
> *"Request model, routing rules, approval workflow, quotas … One Add that routes; availability
> states; per-season TV."* Three checks, each against a document rather than against intuition:
>
> * **`Add` needs a service that advertises `Add`.** §8.3's routing is *"explicit user choice → a
>   routing rule → capability filter (instances whose probed `Caps.MediaKinds` contain this
>   `(kind, format)` **and that advertise `Add`**) → highest `priority`"*. §16.0 and §17.8 already
>   record that **no service v0.1 connects passes that filter** — the Prowlarr path posts a release to
>   Prowlarr's own download client (§8.5), so §17.8 removes the request-destination column from the
>   Libraries row view for v0.1.
> * **Every other candidate sink is later than v0.2.** LazyLibrarian is v0.3 (*"as a request sink
>   only"*); Lidarr, Mylar3 and Kapowarr are v1.0 (§16.0's deferred command sinks). Sonarr and Radarr
>   are the **only** sinks the roadmap can supply at v0.2.
> * **`add` is a queue verb.** The four verbs and the settlement loop are §7.6's, and an Add that
>   routes to Radarr is a `write_queue` row, a worker and a `verifying` state. v0.2's request model
>   cannot be built without (2) either.
>
> **So *"cut before you add"* has nothing to demand here.** `CLAUDE.md`'s rule is that a proposal
> adding a subsystem must say what it removes; **this proposal adds no subsystem to v0.2** — it names
> two prerequisites the milestone already implied. A v0.2 without them is an Add button with nowhere
> to route.
>
> **3. For (3) this is by elimination, not by fit, and that is stated rather than dressed up.** The
> minimal match-correction UI has no dependency on anything in v0.2 and nothing in v0.2 depends on it.
> The owner's constraint leaves exactly two candidates, and **v0.1 is refused on this project's own
> rule**: adding a screen to v0.1 is adding a subsystem to a milestone that has just absorbed Kavita
> ([ADR-0041](#adr-0041)), the `work_book` / `work_comic` / `work_comic_issue` tables and `work_credit`
> ([ADR-0044](#adr-0044)), and there is no payment available to name. `CLAUDE.md`: *"This project's
> biggest risk is never shipping."* **v0.2 is what is left**, and it satisfies *"earlier than v0.3"*
> exactly.
>
> **4. The cost of clause 3 is recorded, not argued away.** **v0.1 ships the *"not identified"* badge
> without its remedy for a whole milestone**, on a catalogue where [ADR-0035](#adr-0035) §1 makes that
> state the majority path rather than an edge case — a v0.1 user is told their library is wrong and
> given nowhere to fix it. ADR-0043's alternative (a) rejected *"the badge alone until v0.3"*; this
> decision accepts *"the badge alone until v0.2"*, which is a shorter version of the same defect and
> is the price of refusing to grow v0.1. **Two mitigations, both real and neither a fix:** nothing in
> v0.2 gates the correction UI, so it is the part of that milestone that **can and should land
> first**; and §16.1's catalogue sequence interleaves with v0.2 rather than blocking it.
>
> **5. Nothing else moves.** No scope is added or removed, no design is reopened, and the *"minimal"*
> boundary [ADR-0043](#adr-0043) clause 2 fixed is untouched — v0.3 keeps the full four-verb surface
> and the Corrections list. [ADR-0012a](#adr-0012a)'s queue design, [ADR-0039](#adr-0039)'s
> unconstrained `state` and `kind`, [ADR-0041](#adr-0041)'s source choice and
> [ADR-0042](#adr-0042)'s clause 5 condition all stand exactly as decided.
>
> **6. One consequence follows for free and is named so it is not lost.** v0.2 is the milestone that
> writes `write_queue` first, so **[ADR-0039](#adr-0039)'s outstanding obligation** — its corrected
> decision 1, the Go `state`-vocabulary declaration and validation *"owed by whoever writes the first
> `write_queue` writer"* — is **v0.2's**. It still costs no migration.

### Alternatives considered

- **(a) Put (1) and (2) in §16.1's post-v0.1 catalogue sequence as a slot #4, ahead of or behind
  Komga.** ⚠️ **The strongest alternative, because §16.1 is where every other re-sequenced source
  went.** Rejected on two grounds. **It breaks the table's definition:** §16.1 is the **read-only
  catalogue** sequence — four Tier 0 adapters sharing channel 3b, with a per-source watermark probe in
  its `Gate` column — and the \*Arrs are library sources *and* command sinks, on channel 3
  (`/history/since`), carrying a write path. Neither the gate column nor the success criterion fits.
  **And it does not solve the problem:** slot #4 sits after Komga, which §16.1 pins last, so v0.2
  would still have an Add button with no sink. A slot ahead of Komga would be v0.2 by another name,
  with the dependency hidden in a table instead of stated in the entry that needs it.
- **(b) Put the minimal correction UI in v0.1 and (1) and (2) in v0.2.** ⚠️ **Tempting, and it is the
  reading §16.0's own sentence invites** — *"a user has something to correct on day one and now has
  somewhere to do it"* — where "day one" is the day Kavita lands. Rejected on *"cut before you add"*,
  which is a rule with a test: name what leaves v0.1 to pay for it. **Nothing can be named.** v0.1
  grew twice on 2026-08-17 already (ADR-0041, ADR-0044), its five essential screens are all
  `CLAUDE.md` commitments, and its security floor is non-negotiable. The alternative is not wrong
  about the user's experience — decision clause 4 concedes exactly that point — it is wrong about
  what the milestone can carry.
- **(c) Split the three: (1) and (2) to v0.2, (3) left unassigned pending a further owner
  conversation.** Rejected because it reproduces the defect both source ADRs flagged. ADR-0042's open
  question is that *"two commitments pointing at a milestone that does not exist is a thinner position
  than one"*; leaving one of three unslotted keeps a thin position alive for no gain, and the owner has
  already been asked once and answered.
- **(d) Create a new milestone between v0.1 and v0.2 for the \*Arr adapters and the write path.**
  Rejected by *"cut before you add"* read at the roadmap level and by §16's own ordering rationale.
  The rationale is *"the earliest milestone that is not already available elsewhere is the one that
  must ship soonest"*; a milestone whose whole content is *"connect to Sonarr and Radarr"* delivers
  the user nothing they cannot already get from Sonarr and Radarr. The work belongs to the milestone
  whose success criterion needs it, which is v0.2.
- **(e) Refuse again and route back to the owner for a version number.** Rejected because **it is
  what has already happened, twice, and he answered by delegating.** Asking a third time for a
  preference he has said he does not have converts a decision into a stall, and `CLAUDE.md`'s
  *"cut before you add"* names never shipping as this project's biggest risk. The delegation is
  honoured by choosing **and by writing the argument down**, which is what this record is.

### Consequences

**What this changes:**

- **`ARCHITECTURE.md` §16's v0.2 entry states its contents**, including the argument that (1) and (2)
  were prerequisites rather than additions, the honest size of the resulting milestone, and the
  ordering inside it (adapters and write path before the request flow; correction UI first or last).
- **Five further sites in §16 lose an unassigned slot and gain v0.2**, each quoting what it used to
  say: §16.0's *"Sonarr and Radarr arrive too"*, §16.0's *"re-sequenced, not cut"* paragraph, §16.0's
  libraries paragraph (the request destination *"returns with the first service that can be a
  destination"*), §16.0's *"It names no milestone"* bullet, §16.1's table note, and the v0.1 and v0.3
  entries' correction-UI and write-path clauses.
- **[ADR-0042](#adr-0042) and [ADR-0043](#adr-0043) each gain the four marks the preamble requires** —
  index row, `Status:` line, a dated `> ⚠️ **AMENDED …**` block, and an inline flag at every clause
  that says no milestone is assigned. **Neither body is rewritten**; both refusals were correct when
  taken, and what changed is that the owner has since answered.
- **v0.2 is now visibly the third-largest milestone in the plan**, behind v0.1 and v1.0 and ahead of
  v0.3. That is worth the owner's attention even though he delegated the choice, and it is stated in
  §16's v0.2 entry rather than left to be discovered. **Most of that size was already true and merely
  unwritten** — the two \*Arr items are v0.2's own prerequisites — but *"already true"* is not the
  same as *"already visible"*, and the split seam is named in case it has to be used.
- **[ADR-0039](#adr-0039)'s outstanding obligation acquires a version**, per decision clause 6.
  ADR-0042 narrowed it from *"whoever writes the first writer"* to *"the first \*Arr adapter's"*; this
  narrows it once more, to **v0.2**.

**What this does NOT change — stated explicitly, because a scheduling ADR is easy to over-read:**

- **No scope moves.** Nothing enters the roadmap, nothing leaves it, nothing is cut. Every one of the
  three was already committed; only the version was missing.
- **No design is reopened.** [ADR-0012a](#adr-0012a)'s queue, [ADR-0026](#adr-0026)'s correction
  model and its four verbs, [ADR-0043](#adr-0043)'s *"minimal"* boundary and
  [ADR-0041](#adr-0041)'s choice of Kavita are untouched.
- **No schema change and no migration.** `write_queue` and `library_override` are both in the tree
  already; this ADR names when their first writers land, not what they will be.
- **v0.1 is not touched.** Nothing is added to it and nothing is taken out of it. That is the point of
  decision clause 3.
- **No ADR is reversed.** [ADR-0042](#adr-0042) and [ADR-0043](#adr-0043) are **completed**, not
  corrected: each raised this question and routed it here.
- **The full correction surface stays at v0.3**, and `BookOrbit` is still assigned nothing by any
  document ([ADR-0043](#adr-0043) clause 4).

### 🚩 Open question this ADR raises and does not answer

**Whether v0.2 should be split.** Naming its real contents makes it a milestone with two independent
halves — the \*Arr adapters plus the write path, then the request model on top of them — and §16's own
ordering rationale is suspicious of large milestones, having reordered the roadmap once already so
that *"a one-to-two-person project"* would not *"survive its largest milestone before delivering
anything the owner asked for"*. **This ADR does not split it**, because a split is a scope decision
with an owner's name on it and nothing here was measured against a real \*Arr instance the owner does
not run. The seam is named in §16's v0.2 entry so that the split, if it is taken, is taken
deliberately rather than discovered halfway through.

---

<a id="adr-0046"></a>
## ADR-0046 — Kavita's contract tests pin TWO specs: the release the owner runs is the floor, `develop` is the ceiling

**Status:** Accepted · 2026-08-17 · **Corrects a claim `api/specs/SOURCES.md` had already made and
the tree had not acted on** · **Changes what a green means; changes no runtime behaviour** — no
adapter field is added or removed, no request changes, no migration · **Renames the vendored file**
`api/specs/kavita.json` → `api/specs/kavita-develop.json` and adds `api/specs/kavita-v0.9.0.2.json` ·
**Leaves [ADR-0035](#adr-0035) and [ADR-0041](#adr-0041) untouched.**

### Context

**`api/specs/kavita.json` was byte-identical to Kavita's `develop` `openapi.json`, `info.version`
`0.9.0.20`. The owner's instance runs stable v0.9.0.2** (observed 2026-08-17,
[ADR-0035](#adr-0035) §2a). Every contract test in `internal/kavita` read that one file, so **a green
was evidence about a branch nobody deploys** — including the test whose entire job is to say the
endpoints UsArr calls exist.

**SOURCES.md already said this, in terms, and nothing acted on it.** Its Kavita section carried the
sentence *"A green contract test here is evidence about `develop`, and the owner's server is two
steps away from it"*, and even recorded the measurement — the tag's copy declares `info.version`
0.9.0.0 and has 462 paths against develop's 488. **The policy existed; the tree did not implement
it.** That is the finding this ADR answers, and it is why the answer is a file rather than another
paragraph: a warning that has been read and not acted on twice is not going to work the third time.

**This is the third instance of one failure class in one day: the artefact checked is not the
artefact that ships.** Kavita's `main` branch frozen at v0.7.8 was the first; a spec declaring a
field without saying which code path fills it was the second. Each was caught by hand. Nothing in
the repo made any of them fail a test.

#### What was measured, and what it changed

Everything below was measured on 2026-08-17 against the two documents and, where it mattered,
against Kavita's own source at tag `v0.9.0.2` (`6bcd568`) — because the tag's checked-in spec
declares `0.9.0.0` and could itself have been stale.

| Question | Answer |
| --- | --- |
| Do the six endpoints the client calls exist at v0.9.0.2? | **Yes**, all six, with the same parameter names and casing |
| Do the seven enums the client pins have the same members? | **Yes** for all seven. `SeriesFilterField`, which no enum test pins, has 34 members at the tag against 35 on develop |
| Is the auth story the same? | **Yes** — one `AuthKey` apiKey-in-header scheme, one global requirement, zero per-operation overrides, on both |
| Does the whole existing suite pass unmodified against the tag's spec? | **Yes** — verified by swapping the file and running `go test ./internal/kavita/... ./internal/libsync/...` before writing a line of this ADR |
| Do `cbrId` and `mangaBakaEditionId` exist on `SeriesDto` at v0.9.0.2? | **No.** Neither is on `SeriesDto`, `ChapterDto` or `VolumeDto` at the tag. At that tag `cbrId` appears on `ExternalSeriesDetailDto` alone, fed by the Kavita+ side table `ExternalSeriesMetadata`; `mangaBakaEditionId` appears nowhere. Confirmed in source: `Kavita.Models/DTOs/SeriesDto.cs` and `Kavita.Models/Entities/Series.cs` declare `MangaBakaId` and neither of the two, and `ExternalMetadataIdHelper.SetExternalMetadataIds` writes six ids of which `CbrId` is not one |
| So which modelled properties are develop-only? | `SeriesDto.{cbrId, mangaBakaEditionId, isStandAlone, nameLocked}` and `LibraryDto.metadataProvider` |
| Does any UsArr code read them? | **Yes, one.** `libsync.kavitaExternalIDs` writes a `cbr` external_id from `SeriesDto.CbrID`. `MangaBakaEditionID` is modelled and deliberately never written (that function's decision 3). Nothing reads `isStandAlone`, `nameLocked` or `metadataProvider` outside the DTO |
| Is that a live defect? | **No, and this ADR says so plainly rather than inflating it.** An absent JSON property leaves the Go field at zero, `intID(0)` returns `""`, and `add` skips it. The row is **unreachable**, not wrong. Nothing is mis-written on the owner's box; a code path is simply dead there and did not say so |
| Was anything else stated as Kavita's that is only develop's? | **Yes.** `internal/kavita/doc.go` said `?apiKey=` appears on *"exactly 9 operations"*. True on develop. **On v0.9.0.2 it is 20**, because all twelve `/api/Image/*` cover routes still accept the key in the query string there and develop has since dropped it. Corrected in this change |

### Decision

**Vendor both, run the whole suite against each, and give each file a role in its name.**

1. **`api/specs/kavita-v0.9.0.2.json` is the FLOOR** — the release the owner runs. A green against it
   means the endpoint, enum member or property UsArr depends on exists on a server somebody actually
   runs.
2. **`api/specs/kavita-develop.json` is the CEILING** — where the API is defined. A green against it
   means nothing upstream has added or renamed has gone unmodelled. This is the old `kavita.json`,
   unchanged byte for byte and renamed.
3. **Every spec-reading test runs against both**, in a subtest named for the file, and every failure
   message names the file *and* its role in words. `TestEndpointsExistInSpec/kavita-v0.9.0.2.json` is
   a sentence; `TestEndpointsExistInSpec` was not.
4. **Two assertions are asymmetric, deliberately.** Enum coverage requires **equality** against the
   ceiling (anything upstream adds or drops is somebody's decision) and **subset** against the floor
   (Go may know members the release lacks — that is what tracking develop buys). Requiring equality
   on both would go red the moment Go was updated for a develop addition, which trains the next
   reader to weaken the test.
5. **`ceilingOnlyProperties` is written out by hand and machine-checked.**
   `TestCeilingOnlyPropertiesAreDeclared` recomputes ceiling-minus-floor from the two files and
   refuses any difference in either direction. It is the list of fields that **decode to nothing on
   the owner's server**, and the next one is a decision rather than a silent addition.
6. **`TestBothSpecsAreTheDocumentsSOURCESSays`** asserts the two files are distinct documents
   declaring the versions SOURCES.md claims — so copying one over the other cannot leave a suite that
   is green and prints two subtest names.
7. **On upgrade, the floor row is re-vendored at the new tag**, per SOURCES.md's own recipe. The
   ceiling row keeps tracking `develop`. The drift job now has two rows, checked differently: a diff
   on the branch tip is routine, a diff on an immutable tag is a supply-chain signal.

**No runtime behaviour changes.** `add("cbr", …)` stays: it is correct for anyone on develop or a
later release, and it degrades to nothing rather than to something wrong. What changes is that the
code now says it is unreachable on the stable line instead of implying it works everywhere.

### The deciding question

**What a green means to a reader who has not read this ADR.** Every clause above was chosen against
that test, and it is what settles the naming: symmetric filenames. A bare `kavita.json` sitting next
to `kavita-v0.9.0.2.json` reads as *the* Kavita spec, and it was the develop one — which is the exact
misreading this ADR exists to end. Neither file may be *the* spec. The rename costs a `git mv` and a
`sed` over comment references; it buys a directory listing that cannot be misread.

### Alternatives considered

**(a) Vendor only the tag the owner runs.** Rejected, and it is closer than it looks. It answers the
question that actually bit — *"is this on the server we talk to?"* — and it makes the vendored file
match the deployment. **But it goes blind exactly where SOURCES.md's stated purpose lies:** the
project deliberately tracks `develop` so a renamed or added field is heard from a bot rather than a
user's bug report, and pinning only a tag means an upstream rename lands unnoticed until the owner
upgrades. It also **buys less than it appears to**: the tag's `openapi.json` declares `info.version`
0.9.0.0, so it is not the deployed artefact either — it is the nearest checked-in thing to it.
Trading a spec that is honestly ahead for one that is quietly behind is not an improvement.

**(b) Vendor both, and test against each.** Chosen. Its real cost is honest and worth stating: **818
KB of vendored JSON, a second drift row, and a permanent maintenance obligation on upgrade.** Against
CLAUDE.md's *"cut before you add"* this is a file and a loop, not a subsystem, and the loop replaced
per-test bodies rather than duplicating them — the suite got one helper longer, not twice as long.
The decisive evidence was measured before the decision: **the existing suite passes unmodified
against the tag's spec**, so the second arm cost parameterisation and no new assertions to satisfy.
Had the tag arm failed, this alternative would have been a much bigger change and (a) or (c) would
have deserved another look.

**(c) Keep `develop` and state plainly in SOURCES.md and the tests what a green does and does not
attest.** Rejected — **because it is what the repo already did, and it did not work.** SOURCES.md
carried exactly that warning, in bold, with the measurement, and the tests still asserted against one
file while a struct grew four fields the owner's server has never sent. A warning nobody acts on is
indistinguishable from no warning. This is the same reasoning `DEVELOPMENT.md` §11 applies to guards:
one that has never fired is indistinguishable from no guard.

**(d) Generate a skew manifest instead of vendoring a second file.** Considered and rejected: a
derived list of "properties develop has and the release does not" is smaller than 818 KB, but it is
derived data that goes stale silently and answers only the questions someone thought to derive. It
cannot answer *"does this endpoint exist at v0.9.0.2"* for an endpoint added next month.
`ceilingOnlyProperties` is that manifest for the one question worth pinning by hand — and it is
**recomputed from the two files on every run** rather than trusted, which is only possible because
both files are present.

**(e) Test against a live instance.** Not available and not wanted. CI has no network and there is no
public Kavita demo with API access. The owner's own instance is the only real evidence there is, and
[ADR-0035](#adr-0035) §2a's live observations remain the record of it. Nothing in this ADR replaces
them.

### Consequences

* **A green now says something true about the owner's server**, for the first time in this package.
  It says it in the subtest name, so it survives being read by someone who never opens this file.
* **`api/specs/` grows to 1.86 MB** across three specs. Vendored JSON is not free, and this is the
  first time the directory has held two copies of one upstream.
* **An upgrade is now a small chore rather than nothing** — re-vendor the floor at the new tag,
  update `pinnedSpecs` and `kavitaSpecFiles`. `TestBothSpecsAreTheDocumentsSOURCESSays` fails loudly
  if the file and the table disagree, so the chore cannot be half-done.
* **Two pins are two points, not a line.** A user on v0.8 gets coverage from neither file. This ADR
  narrows the gap between *"what we test"* and *"what he runs"*; it does not close the gap between
  that and *"what everyone runs"*, and SOURCES.md now says so under its own heading.
* **The `cbr` external_id is documented as unreachable on the stable line**, in
  `internal/kavita/resources.go` and in `internal/libsync/kavita.go`, rather than being removed. If
  it is ever removed, this is the note that says what removing it costs someone on develop.
* **The naming discrepancy list in SOURCES.md gets longer, not shorter.** `ARCHITECTURE.md` §6.4 and
  §7.1a, `RESEARCH.md`, `SETUP-CHECKLIST.md`, ADR-0035, ADR-0044 and every REVIEW-LOG entry before
  this one name `kavita.json` or `kavita-openapi.json`. The pointer is recorded in SOURCES.md rather
  than by back-editing those documents — and REVIEW-LOG in particular is a historical record that
  must not be rewritten.

### 🚩 Open questions this ADR raises and does not answer

1. **`prowlarr.json` has the same shape of gap and is not fixed here.** It tracks `develop` at v2.6.2
   while the only known deployment runs stable 2.5.2.5491, and SOURCES.md's Prowlarr warning is
   word-for-word the one that failed to work for Kavita. Applying this ADR's pattern there is a
   separate change against a different suite, and doing it inside a Kavita commit would make neither
   reviewable. **Nothing here should be read as evidence that the Prowlarr contract tests attest
   anything about 2.5.2.**
2. **`external_id.source` = `'cbr'` is not enumerated anywhere as a legal source.** The column is
   free `TEXT` with a comment listing sources, and `cbr` is not in the comment — the `cbr` that does
   appear in `00005_library_sync.sql` is `edition.format`'s `CHECK`, a different thing entirely.
   Unreachable on the owner's install today, so it is recorded rather than fixed.
   ✅ **SETTLED 2026-08-17 by the rename to `comicbookroundup`** (`internal/libsync/editableid.go`,
   [`REVIEW-LOG.md`](./REVIEW-LOG.md) `LS-73`). The question is answered in the *"a different thing
   entirely"* clause's own terms: two unrelated things sharing one token inside one schema is the
   defect, and the site's own name cannot be read as a file extension. **The unreachability this ADR
   measured is what made it free** — no `cbr` row has ever been written at the floor, so there was
   nothing to migrate, and the same rename after the first `develop` user syncs would have needed
   one. The migration comment's source list is left alone: it is already stale for
   `comicvine_volume`, `mal_manga` and `hardcover_book`, a merged migration is never edited, and the
   vocabulary now lives on named Go constants that the compiler checks.
3. **Whether the floor should be re-pinned on every upstream patch release or only when the owner
   upgrades.** This change pins what he runs. Chasing every stable tag would make the floor a second
   moving target and lose the property that makes a tag diff meaningful.
