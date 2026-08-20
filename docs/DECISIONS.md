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
| [0025](#adr-0025) | Styling and typography: Tailwind v4 with the default theme deleted, Bits UI, Tabler, self-hosted IBM Plex | **Accepted** — ⚠️ **amended 2026-08-16**: **Tailwind is not used**; the styling layer is hand-written CSS in `web/src/app.css`. Bits UI, Tabler and IBM Plex are unaffected; ⚠️ **amended 2026-08-19**: §6's `paths.relative` bullet — the setting stays pinned to `false`, but the reason given for it is corrected: SvelteKit skips the relative-path rewrite for the SPA fallback document, so it is **belt-and-braces, not load-bearing**. The decision itself, and all four traps' settings, are untouched |
| [0026](#adr-0026) | A library is a user-owned binding to upstream containers, with a correction layer | **Accepted** — refines ADR-0004, extends ADR-0014; ⚠️ **amended 2026-08-17 by [ADR-0043](#adr-0043)** — the binding, the four verbs and the storage are untouched, but this ADR's consequence capping the correction **UI** at v0.3 rested on *"§6.4 already establishes that tier 1 resolves essentially 100% of the v0.1 identity problem"*, which [ADR-0041](#adr-0041) falsified for v0.1's actual source: **the minimal *"fix this match"* case moves earlier**, and the full four-verb surface stays at v0.3 |
| [0027](#adr-0027) | Two axes: media type is navigation, a library is scope | **Accepted** — settles §17.2's open question; ⚠️ **amended 2026-08-19 by [ADR-0053](#adr-0053)** — the two axes, the scope chip, the pins and the row budget all stand, but the **data-driven sidebar** does not: *"showing only types that have content"* needs a read answering which types have rows, and `reference/http-api.md` §7.1 says no facet count is on the wire, so **all six entries always render**; the zero-items rule survives for **home and for search groups**, which ADR-0053 leaves untouched |
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
| [0039](#adr-0039) | `write_queue.state` loses its `CHECK`; `work_id` gets its foreign key back | **Accepted** — 2026-08-17; **supersedes** `reference/schema.md` §10 step 1 and the seam in `FUTURE.md` §11 / §11.1; closes `REVIEW-LOG.md` WQ-05; ⚠️ **amended 2026-08-17** — decision 3's ground 1 is **struck**, on a misquotation of `reference/sync.md` §4 that dropped the words *toward the \*Arr*: the decision stands on grounds 2 and 3, which are independent of it; ⚠️ **corrected 2026-08-17** — decision 1 and the first rejected alternative wrote the Go `state` validation as **done**; it is **owed by the first `write_queue` writer** and nothing validates the vocabulary today (`REVIEW-LOG.md` M5-25); ⚠️ **amended 2026-08-19** — the *declaring* half is **discharged in code**: `007e58e` landed `internal/store/writequeue.go`, whose `ValidWriteQueueState` is the vocabulary's **single Go home**, and the tree's only `write_queue` writer — the **bench-tagged** fixture `internal/db/spike/fixture.go`, behind `//go:build bench` — routes through it; **but there is still NO PRODUCTION WRITER**, so at runtime nothing validates anything because nothing writes anything, and *"nothing validates the vocabulary today"* is false about the declaration and still true about the runtime; **the claim worth recording is stronger than either** — an AST guard (`TestWriteQueueWritesValidateTheStateVocabulary`) fails `make check` if anything writes `write_queue` while nothing references the validator, so **the first production writer cannot be written without validating**, and the guard **fired unplanted** against the bench writer the moment it landed |
| [0040](#adr-0040) | The six subtype tables land with the catalogue source that writes each | **Accepted** — 2026-08-17; records as a decision what `00005_library_sync.sql` did; **in tension with** ARCHITECTURE §16's enumerated v0.1 schema line, which is left to the thread that owns §16; ⚠️ **amended 2026-08-17 by [ADR-0044](#adr-0044)** — the RULE is confirmed and **applied**, not overridden, and one table moves under it: `work_credit` lands with **Kavita** rather than Navidrome, because Kavita is the source that writes credits. `work_album` and `work_track` are unaffected and this ADR's decision clause 1 is otherwise untouched |
| [0041](#adr-0041) | The sync core ships with **Kavita** as its first adapter; Sonarr and Radarr re-sequence behind it | **Accepted** — owner-decided 2026-08-17; **amends [ADR-0036](#adr-0036)** (*"No catalogue source ships in v0.1"*) and **amends** ARCHITECTURE §16, whose replacement text is proposed here and routed to the thread that owns §16; **re-sequences, rejects nothing** — Sonarr and Radarr still arrive; confirms [ADR-0035](#adr-0035) and [ADR-0040](#adr-0040); ⚠️ **amended 2026-08-17 by [ADR-0042](#adr-0042)** — the write-path question this ADR flagged as *"NOT decided here"* is now answered: the minimal write path **re-sequences with the \*Arr adapters**, so its consequence bullet and the ⚠️ clause in its proposed §16 text are both settled · ⚠️ **amended 2026-08-19 by [ADR-0052](#adr-0052)** — clause 1's source is now **BookOrbit**, on the owner's decision to sunset Kavita; clauses 2 and 3 stand, and **clause 4's channel list is reopened**, not re-answered |
| [0042](#adr-0042) | v0.1's minimal write path re-sequences with the \*Arr adapters; Sonarr and Radarr stay on the roadmap | **Accepted** — owner-decided 2026-08-17; **answers the question [ADR-0041](#adr-0041) flagged and refused**; **amends** ARCHITECTURE §16, [ADR-0041](#adr-0041), [ADR-0036](#adr-0036) and [ADR-0012a](#adr-0012a); **re-sequences, rejects nothing** — [ADR-0012a](#adr-0012a)'s queue design is untouched, the seam costs **no migration**, and Sonarr and Radarr stay on the roadmap at the owner's explicit condition; raises one open question it does not close (neither \*Arr has a milestone); ⚠️ **amended 2026-08-17 by [ADR-0045](#adr-0045)** — that open question is **closed**: the owner delegated the call and **Sonarr, Radarr and this write path all land in v0.2**. The decision, the measurement and the seam are untouched; only *"this ADR does not assign them a milestone"* (clause 5), alternative (e) and the open question itself are overtaken |
| [0043](#adr-0043) | A **minimal** match-correction UI moves earlier than v0.3; the full correction surface stays there | **Accepted** — owner-decided 2026-08-17; **answers the scope call ARCHITECTURE §6.4 flagged and routed**; **amends [ADR-0026](#adr-0026)**'s correction-UI consequence and **amends** ARCHITECTURE §16.0, §16.1's v0.1 and v0.3 entries; **re-sequences, rejects nothing** — ADR-0026's model, verbs, tables and scoping are untouched and `library_override` is already in the tree; **source-independent** — it turns on v0.1 having a weak-identity catalogue source, not on that source being Kavita; **assigns no milestone**, which is carried as an open question; ⚠️ **amended 2026-08-17 by [ADR-0045](#adr-0045)** — that open question is **closed**: the owner delegated the call and the minimal case lands in **v0.2**, chosen **by elimination** (the only slot both earlier than v0.3 and not v0.1) with its cost recorded — v0.1 ships the *"not identified"* badge without its remedy for one milestone. The *"minimal"* boundary, the source-independence and the v0.3 cap on the full surface are all untouched |
| [0044](#adr-0044) | Author and creator credits ship in v0.1; `work_credit` lands with **Kavita**, not Navidrome | **Accepted** — owner-decided 2026-08-17; **applies [ADR-0040](#adr-0040)'s rule rather than overriding it** — the landing point is the source that writes the table, and Kavita writes credits; **amends [ADR-0040](#adr-0040)** for `work_credit` **only** — `work_album` and `work_track` still wait for Navidrome; **confirms [ADR-0033](#adr-0033)**, whose `person` kind finally gets a writer; **costs a ROW, not a column** — an author is a `work` of kind `person`, which nothing in v0.1 created before; leaves [ADR-0035](#adr-0035), [ADR-0041](#adr-0041) and [ADR-0042](#adr-0042) untouched; raises one open question it does not close (nothing collects an uncredited person) |
| [0045](#adr-0045) | The three unslotted commitments land in **v0.2**: Sonarr and Radarr, the minimal write path, and the minimal match-correction UI | **Accepted** — **owner-delegated 2026-08-17** (*"whatever you think is best"*); **closes the open questions [ADR-0042](#adr-0042) and [ADR-0043](#adr-0043) each raised and refused**; **amends** ARCHITECTURE §16 (§16.0, §16.1, and the v0.1, **v0.2** and v0.3 entries) and **amends [ADR-0042](#adr-0042)** and **[ADR-0043](#adr-0043)** at their no-milestone clauses; **assigns milestones and nothing else** — no scope moves, no design reopens, no ADR is reversed; for the two \*Arr items this **writes down a dependency v0.2 already had** (§8.3's `Add` capability filter), for the correction UI it is **by elimination** and the cost is recorded rather than argued away; raises one open question it does not close (whether v0.2 should be split); ⚠️ **amended 2026-08-18** — the order **inside** v0.2 is now fixed, resolving this ADR's own *"correction UI first or last as convenient"*: **minimal match-correction UI first** (gated on nothing), **Navidrome adapter next** (next in §16.1's sequence, provable on the owner's real services), then **Sonarr, Radarr and the write path** (no real \*Arr data exists to prove them against — the owner runs neither). Membership is untouched; only the order is added |
| [0046](#adr-0046) | Kavita's contract tests pin **TWO** specs: the release the owner runs is the **floor**, `develop` is the **ceiling** | **Accepted** — 2026-08-17; **implements a policy `api/specs/SOURCES.md` already stated and the tree had not acted on** (*"a green contract test here is evidence about `develop`"*); **changes what a green means, not what the code does** — no adapter field, request or migration changes; **renames** `api/specs/kavita.json` → `kavita-develop.json` and **adds** `kavita-v0.9.0.2.json`; every spec-reading test runs against **both**, named per file, with enum coverage **equal** to the ceiling and a **superset** of the floor; `ceilingOnlyProperties` machine-checks the five modelled properties that **decode to nothing on the owner's server**; the `cbr` external_id is **unreachable** on the stable line and now says so; raises three open questions it does not close (Prowlarr has the same gap, `'cbr'` is an unenumerated `external_id.source`, and the floor's re-pin cadence) — **question 2 settled 2026-08-17** by the rename to `comicbookroundup` (`LS-73`); ⚠️ **amended 2026-08-17** — the decision stands unchanged **for Kavita**, but it is **not a template**: it rests on the unstated assumption that **upstream regenerates its spec per release**, which Prowlarr does not — its `openapi.json` is the **same git blob** at tag `v2.5.2.5491` and at `develop`, so open question 1's *"the same shape of gap"* is wrong about the shape and the two-spec structure there would manufacture the false green this ADR abolishes. The remedy is chosen **per upstream** and is [ADR-0047](#adr-0047); and where a spec self-reports a placeholder `info.version`, identity is by **blob SHA** |
| [0047](#adr-0047) | Prowlarr pins **ONE** spec — floor and ceiling are the same git blob — guarded by an offline blob-identity pin in `check` plus a network drift check outside it | **Accepted** — 2026-08-17; **is the per-upstream remedy [ADR-0046](#adr-0046)'s 2026-08-17 amendment (`LS-53`, `cf5fab5`) points at**, and **answers [ADR-0046](#adr-0046)'s open question 1** (*"`prowlarr.json` has the same shape of gap"*) by **correcting its premise** — measured independently by both threads, `src/Prowlarr.Api.V1/openapi.json` is the **same blob `134d31d7…`** at `v2.5.2.5491` and `develop`, not *"develop, a minor version ahead"*; **the two-spec split is vacuous here, not impossible** — nothing stops two byte-identical copies being vendored, and the second would prove exactly what the first already proves (the Context paragraph's wording, which this row previously overstated); **changes what a green means, not what the code does** — no adapter field, request or migration changes, no file added or renamed; the one file is stale (last regenerated **2025-06-07**, 33 releases ago) and describes **neither ref reliably**; `TestVendoredSpecIsThePinnedBlob` pins the blob **offline, in `check`**, `TestSpecDriftRefsStillShareThePinnedBlob` catches upstream regenerating **on the network, in `make spec-drift`, never in `check`**, and `knownSpecDivergences` machine-checks the `Limit`/`Offset` `int?` gap (PR #2654, `v2.3.6.5351`) as **still live**; `info.version` **`1.0.0`** is Swashbuckle's placeholder and is pinned to by nothing; leaves [ADR-0035](#adr-0035), [ADR-0041](#adr-0041) and [ADR-0046](#adr-0046) untouched; the floor `v2.5.2.5491` is **owner-confirmed 2026-08-17**; raises two open questions it does not close (the floor drifts when the owner's auto-updating box does, and `make spec-drift` is unautomated) |
| [0048](#adr-0048) | A library **proposal** is not a row in `library`; a row is created only on Accept | **Accepted** — 2026-08-17; **refines [ADR-0026](#adr-0026)** (its binding model, four verbs, single-kind rule and four tables are untouched — what is decided is *when a `library` row comes into existence*, which ADR-0026 did not say); **applies [ADR-0004](#adr-0004)** rather than excepting it — the connect probe is a **setup** action, not a render path; **answers the open decision `web/src/routes/libraries/+page.svelte` records at `78660a4`**, which named three candidates and picked none — this takes the first, *a proposal stops being a row until it is accepted*, and **rejects the other two in writing**; **closes off a third `managed_by` state and any `proposed` flag on `library`**; **costs no migration, no data change and no new state** — ⚠️ **and not for the reason it first appears**: `managed_by` **cannot** express "proposed" and never could, which is a fact *for* this decision rather than against it, because after it the unaccepted state has no persistent representation to record; existing `managed_by = 'auto'` rows are **declared** accepted on upgrade rather than read as accepted, since the column cannot tell an accepted library from one the user has never been shown; states plainly that **it describes unbuilt behaviour on two counts** — `'user'` has never been written by any code path, so §17.8's one-way door is specified and unimplemented, and today's import **creates rows unconditionally**, so implementing Accept **removes** creation from the import path rather than adding a screen to it; **that removal is not done here** — it belongs to the library thread that builds §17.8 |
| [0049](#adr-0049) | Key ids are **derived from the key material**; there is no counter and no settings row | **Accepted** — 2026-08-19; **enables `usarr key rotate`** rather than being asked for by it; `crypto.KeyID(kek)` is the first four bytes big-endian of `sha256("usarr/kek-id/v1" || kek)`, forced nonzero, so **a key file names its own id** and no second artifact has to stay consistent with the key material across a crash — which is exactly the window rotation exists to survive, since the SQLite transaction and the key-file write are not one atomic unit; **closes off a monotonic counter and a `key_id` row in a settings table**, both of which reintroduce that window (and the settings row puts key identity *inside* the thing being rotated); startup registers the live key under both `KeyID(kek)` and the legacy id `1`, so **every existing row keeps opening with no migration** and the first rotation retires `1`; **costs no migration** — `service_instance.kek_id` is already `INTEGER`; **adds no HKDF label** and does not touch `derive.go`'s five frozen ones; ⚠️ **publishes a 32-bit hash of the KEK in every stored row**, accepted in writing because RFC 3394 key-wrap **already** gives an offline attacker an *exact* per-row oracle for the same question, so a 32-bit filter grants no capability the ciphertext did not |
| [0050](#adr-0050) | The image pipeline's base output format is **stdlib JPEG**; **AVIF is deferred** with its seam kept | **Accepted** — 2026-08-19; **amends** ARCHITECTURE §4.4 and §4.4.1, which named AVIF as the only output codec and named **no base format at all** — a spec missing its base case, which is why this ADR was owed; the reason for stdlib is **zero new dependencies in a static binary** (UsArr has **five** direct dependencies; `image/jpeg` adds none), **not** "JPEG is good enough" — the ADR records the ledger it is traded against, roughly **2–3× larger** than AVIF on photographic content, so a future reader can weigh it; **AVIF is buildable here** (`gen2brain/avif` v0.6.0, MIT, cgo-free, libaom-as-WASM) and is deferred on a **measured trade with a named reopening condition**, not rejected — one MIT dependency plus a **second** WASM runtime, since `wazero` is **verified absent** from this module graph after `ncruces/go-sqlite3` moved to `wasm2go`, and the **binary-size delta is recorded as UNMEASURED rather than estimated**; **reopens when** someone measures the binary delta and the per-width encode cost and decides the bytes are worth it (an ADR amendment plus one map entry, **no migration**), or when an upstream is found serving a format the stdlib cannot decode — ⚠️ **measured, not assumed, and the first draft's assumption was wrong: Kavita is v0.1's catalogue source and its *Save Media As* setting writes covers as PNG (default), WebP or AVIF**, so one admin checkbox on the owner's own server produces input this binary cannot decode (`x/image/webp` is decode-only; there is no pure-Go AVIF decoder in `x/image`), which relocates the likeliest revisit from output size to **input decode**; **one codec per row is an explicit invariant** — clause 1 puts `orig` inside UsArr's encoder rather than leaving it a passthrough, because §4.4 stores **seven widths per asset** and the column is **one per row**, so per-`role` variation stays expressible and per-**width** variation is foreclosed in writing; the seam is **`image_asset.format`** (migration `00008_image_asset_format.sql`) — nullable `TEXT`, no default, **no `CHECK`** on [ADR-0039](#adr-0039)'s reasoning; ⚠️ **unlike ADR-0039 the Go validation SHIPPED WITH THE COLUMN** (`internal/store/images.go`, plus an AST-walk guard that fails `check` if a writer lands without it), because ADR-0039's promised validator was never written and repeating that would be worse than a `CHECK` — ⚠️ **true when written and the contrast still holds; the premise was discharged hours later on 2026-08-19 by `007e58e`**, which gave `write_queue.state` the same treatment (`internal/store/writequeue.go` plus its own AST guard), so **this ADR's reasoning is unchanged and is now the pattern rather than the exception** — the ordering it argues from, validation shipping *with* the column rather than being promised after it, is exactly why ADR-0039 needed a second commit to catch up; see [ADR-0039](#adr-0039)'s 2026-08-19 amendment; ⚠️ **described a pipeline that did not exist** — when this ADR landed nothing wrote `image_asset`, so what shipped was the decision, the column and the guard; 🔻 **`7e5934d` built the pipeline** (`internal/imagepipeline` + `store.PutPosterAsset`) to clause 1, discharging two of the three owed items, leaving the AVIF deferral untouched, and **still never run against a real cover** |
| [0051](#adr-0051) | The library-scoped grid is a **work-driven `EXISTS`**, not a join to `library_member` | **Accepted** — 2026-08-19; **supersedes [ADR-0026](#adr-0026)'s materialisation as read by ARCHITECTURE §6.5 for the `added_at` order ONLY** — §6.5's denormalised `(library_id, sort_title, work_id, edition_id)` key stands, and `TestLibraryScopedKeysetIsASeek` still pins it, but it serves the **`sort_title`** order and **only** that one: measured on the real schema (`ncruces/go-sqlite3`, SQLite 3.53.4), a library-scoped page ordered by `added_at` gets `USE TEMP B-TREE FOR ORDER BY` in **both** topologies §6.5 names, **with and without `ANALYZE`**; the work-driven `EXISTS` over `ix_libmem_work` keeps `SEARCH w USING INDEX ix_work_added (added_at<?)` in **every** configuration measured, **including the multi-value `?lib=a,b` case** — which was a hypothesis until the plan was read, because an `IN` on the leading key column destroys the ordered index in every *member-driven* shape; it is also **the only shape that cannot return one work twice**, since a work filed in two of the named libraries carries **one membership row per library** and a browse row is work-keyed — ⚠️ per-**library**, not per-edition: `library_member`'s key carries `edition_id`, but the only production writer hardcodes the `0` sentinel, so membership is **not** edition-grained in the tree today (`REVIEW-LOG.md` LS-213), and the ADR body's argument, which is about two libraries over one work, is unaffected; **costs one migration** — `00009_edition_format_index.sql`, `ix_edition_format ON edition(format, work_id)`, for the Audiobooks filter and not for the scope; **`ix_libmem_added` is explicitly NOT owed** and must not be added on this ADR's authority; ⚠️ **reopens on `make bench` over a NARROW library** — the `EXISTS` walks the *global* `added_at` order and discards non-members, which suits a broad library and not a narrow one, so a 1%-selective library over a 25k-row kind is the measurement that would send this back to a member-driven shape with a new index; ⚠️ **amended 2026-08-19** — the *"says nothing about `year`"* non-decision gains the shape of the gap it leaves: `default_sort`'s CHECK admits four orders and this read serves three, `?sort=year` is **refused and never substituted**, the endpoint never reads `default_sort` at all, and nothing in the tree writes the column yet — so the trap arms the day §17.8's DETAIL view offers the choice. The decision is untouched |
| [0052](#adr-0052) | v0.1's catalogue source is **BookOrbit**; **Kavita is sunset** and its adapter **stays in the tree** | **Accepted** — **owner-decided 2026-08-19**; **amends [ADR-0041](#adr-0041)** clause 1 (*"v0.1's catalogue source is **Kavita**"*) — ADR-0041's clauses 2 and 3 are **confirmed**, and its clause 4 (channels **1, 3b and 4**) is **REOPENED as an open question, not re-answered**, because BookOrbit has had no equivalent of [ADR-0035](#adr-0035) §2a's live probe; **amends ARCHITECTURE §16.1's v0.1 entry**, edited in the same change because §16 is scope authority; the decision is **the owner's, not an agent's** — he is sunsetting Kavita entirely, BookOrbit takes everything, his word is **"phenomenal"**, and the repo's own one-day-older record of the same direction is `ROADMAP.md` §3's *"in my heart i kind of want to migrate to book orbit"*; ⚠️ **it REVERSES `ROADMAP.md` §3's standing recommendation** *"do NOT switch UsArr's first adapter off Kavita"*, because **two of the three findings that produced it were re-measured on 2026-08-19 against BookOrbit `main` and are FALSE** — headless auth needs **no password** (`server/src/modules/auth/magic-link.service.ts`; SHA-256-hashed token, `loginWithToken()` validates no password), and **comics ARE covered** (a shipped ComicVine provider), leaving only **manga and anime** identifiers absent (zero hits for `mangabaka`/`anilist`/`myanimelist` repo-wide); 🚩 **and a third claim reached the ADR in relay and was REFUSED on primary source** — *"no watermark, so full resync with no delta channel"* is **false in its strong form**, since `packages/types/src/query.ts` admits `"updatedAt"` as a sort key with page/size paging, which is exactly channel 3b's shape, so writing it in would have foreclosed v0.1 work on a premise the source refutes; what is **genuinely unsettled** is whether that timestamp moves on tag, genre and author edits, since `$onUpdateFn` is **application-level, not a DB trigger** and those live off the book row — §7.1a's **reconciliation-only** fallback is the named failure branch, **not** this ADR's decision; **"sunset" explicitly does NOT mean delete** — `internal/kavita`, `internal/libsync/kavita.go`, both vendored specs and [ADR-0046](#adr-0046)'s contract guard stay and stay green, investment stops, and **no milestone for further Kavita work is invented**, on [ADR-0042](#adr-0042)'s refusal-to-number precedent; **MangaBaka is NOT a dependency** — the owner's *"in the near future"* is **his expectation, nobody's commitment**, native support is an **open PR with no maintainer signoff**, and the adapter is designed against what BookOrbit ships **today**; ⚠️ **MangaBaka data may be fetched at runtime and NEVER vendored, shipped or cached as a dump** — **CC BY-NC-SA 4.0**, verified at `mangabaka.org/data/database`, is **not AGPL-3.0-compatible**, and the dump additionally carries third-party terms it does not license; ✅ **identity needs NO migration and NO new mechanism, which INVERTS this ADR's own first draft** — the draft called BookOrbit's series-level identity a structural degradation and warned of a migration, and a schema check against the tree falsified it: `external_id`'s `source` is plain `TEXT` with **no `CHECK`** and it carries `confidence` (`00005_library_sync.sql:444`), a series **IS a work row** (`work.kind` admits `'series'` and `'comic'`, `:242`) so `work_id` **already is** the series-level column, and `kavitaExternalIDs` **already writes seven series-level ids** including **`mangabaka` at 0.90** (`internal/libsync/kavita.go`, `weblinkid.go:111,162`) — because Kavita's own series ids are **weblink-parsed from what the user tagged**, exactly as BookOrbit's would come from a user-populated custom field, so the two are **the same arrangement** and 0.90 is already the right grade; the one recorded wrinkle is that BookOrbit's custom fields are **book-scoped** (`custom-metadata.ts`, `bookId` FK, no series variant) so the id needs a hoist — **which Kavita also needs and does lossily**; 🚫 **`work_relation` is cited nowhere and must not be added** — it is **absent from the tree** and `internal/db/migrate_test.go` fails if it appears; ⚠️ **ships NO code by design** — it gates the adapter |
| [0053](#adr-0053) | All six media types are **always** in the sidebar; per-type hiding is closed until a facet read exists | **Accepted** — 2026-08-19; **amends [ADR-0027](#adr-0027)** for its sidebar clause **only** — that ADR's *"a type with zero items is not rendered anywhere"* stands for Block A and for search groups, both of which are unaffected — and amends ARCHITECTURE §17.2 and `design/DESIGN-DIRECTION.md` §8.1 to match the shipped shell; the data-driven sidebar those two specified **cannot be built over the wire UsArr serves**, because `reference/http-api.md` §7.1 states there are *"no facet counts beside the chips; each is its own aggregate and its own read"* and no read answers per-type presence at all; so **all six render unconditionally, no row carries a count**, and the honesty moves to the per-type screen, where `browseEmptyState` names which of three reasons the grid is empty; ⚠️ **the rejected alternative that looks like compliance is hiding a type on a count nobody measured** — it fails silently and removes the very row that would have explained the absence; **adds no endpoint, no migration and no backend change**; ⚠️ **reopens on exactly one condition** — a read answering which of the six types have rows under the current scope, in one statement — at which point the seam is one predicate on `TYPE_NAV`, and §13 has already priced the shape at *"1 keyset page + 6 sidebar `COUNT(*)`"* < 15 ms p50 without deciding whether it rides the browse response or its own endpoint; ⚠️ **reopening condition REFINED 2026-08-19, citing [ADR-0059](#adr-0059) — refined, not discharged.** `GET /api/v1/library/facets` shipped and looks like the named read; it is not. It answers *how many works are bucketed to each type*, and every book is bucketed exactly once, so discharging the condition with it would hide **Audiobooks** from a user who has audiobooks held as second editions of ebooks. The condition now reads as a predicate answering **whether a type has content** — the independent `EXISTS` over `edition.format` of §17.2 rows 4–5, which `ix_edition_format` already serves — **and not a read returning per-type counts**. The decision itself is untouched: the nav stays all-six-always for v0.1 |
| [0054](#adr-0054) | The search response publishes a per-hit relevance `score`, and the ORDER stays the contract | **Accepted** — 2026-08-19; **amends [`reference/http-api.md`](./reference/http-api.md) §6.2**, which said *"No score is published"*; **unblocks ARCHITECTURE §17.4 rule 2**, whose grouped results are ordered *"by the group's best-scoring hit"* — a comparison **no ordering can answer**, because a row's ordinal position says nothing about the distance between two groups' best rows; the withheld field was stopping the screen from being built, not stopping the misuse it was withheld for, so the misuse is **documented and tested against instead**: §6.2.1 lists two permitted uses and five forbidden ones, each with its mechanism; ⚠️ **the order is still authoritative and is NOT score order** — the media-type diversity injection promotes a row without re-scoring it, so a client re-sorting by the published number silently produces a **worse** list than the one it was given; **adds no migration, no column and no query** — the value was already computed per hit and discarded at the boundary |
| [0057](#adr-0057) | The circuit breaker is **one package with an injected open sentinel**, not a copy per client | **Accepted** — 2026-08-19; **taken at the trigger the tree itself named** — `internal/kavita/breaker.go` carried the standing instruction *"worth taking the first time a THIRD client needs one: lift this file into `internal/breaker` with an injected sentinel … two copies is cheaper than a package that exists to serve two callers; three is not"*, and `internal/bookorbit` is that third client; **closes off each client keeping its own copy of the state machine**, which was the live alternative and would have made three; the open sentinel is a **constructor argument**, which is the one reason the copy ever existed — `errors.Is(err, kavita.ErrBreakerOpen)` and `errors.Is(err, servarr.ErrBreakerOpen)` keep meaning exactly what they meant, and a Kavita failure never reads `servarr: circuit breaker open`; **`internal/kavita` and `internal/servarr` keep their exported names as type aliases** (`BreakerState`, `BreakerConfig`, `Breaker`, the three state constants, `NewBreaker`), so **`internal/releases` and `internal/libsync` are untouched** — verified, neither appears in `c324cbf`'s diff at all; **the sha moved from `568ddbc` because that is the merge that carried slice 0, not the commit that contains it** — `568ddbc`'s own diff is seventeen files of library-screen, search and docs work with no breaker file in it, and `c324cbf` is its first parent, *"feat(bookorbit): slice 0 — the client and the credential path"*, so the verification re-runs on `c324cbf` and nobody restores the merge sha; **the only test change outside the new package is one line**, `BreakerConfig{}.withDefaults()` → `.WithDefaults()` in `internal/kavita/client_test.go`, and `internal/servarr/breaker_test.go` was not touched; **the §7.5 tuning does not move** (5 failures to open, 5 s → 15 m capped, ±20% jitter) and is pinned in **three** packages — `internal/breaker`'s own `TestDefaultsAreTheArchitectureNumbers`, plus a client-side assertion in `internal/kavita/client_test.go` and `internal/bookorbit/client_test.go`, while `internal/servarr` pins the same numbers **behaviourally** through its five untouched breaker tests — and the client-side assertions deliberately stay in the client packages, which is why `withDefaults` becomes the exported `WithDefaults`; ⚠️ **the line ledger is a deduplication, not a net saving, and this ADR states the measured numbers rather than the headline** — the commit deletes **347** lines (346 of them the two copied state machines, the 347th that one test line), the two client files go from **202** and **192** lines to **52** and **49** (101 lines of alias wrapper, **53** of them newly written), and the new package costs **245** lines plus a **103**-line test; what is bought is **one** state machine instead of two, not a smaller tree; **adds no dependency, no migration, no behaviour change and no exported-API break** |
| [0058](#adr-0058) | UsArr **grades the scope a stored service credential actually carries**, and **reports rather than refuses** | **Accepted** — 2026-08-19; **discharges [ADR-0052](#adr-0052)'s §14 credential-scope gate** — ⚠️ a **discharge is not an amendment**, so ADR-0052 gains a dated inline note pointing here and **nothing in its text is struck**; **closes off assuming a service account is minimal** because it was created as a service account — the grading is done **in code, not in prose**, so `TestEveryBookOrbitPermissionIsClassified` notices a 24th permission upstream where a paragraph could not; all **23** members of BookOrbit's `Permission` enum are classified **elevated** (14: write or admin reach beyond a catalogue read) or **unneeded** (9: harmless but more than UsArr uses), with the superuser flag (elevated), a non-`shared` `provisioningMethod` (unneeded) and an inactive account (unneeded) as **separate findings** rather than permissions; ⚠️ **an unrecognised permission grades ELEVATED, never harmless** — chosen, not fallen into, so the verdict gets *more* conservative on its own when BookOrbit's vocabulary grows, at the named cost that a genuinely harmless upstream addition is over-flagged until someone classifies it (the grading is a **maintenance obligation**, not a self-maintaining one); **costs ZERO extra requests** — `AuthService.buildUserResponse` ships `permissions`, `isSuperuser` and `provisioningMethod` in the same body as the `accessToken`, which is why the verdict can be recomputed on **every** mint; **the client REPORTS and WARNS, it does not REFUSE**, because refusing leaves an operator with a service that will not talk to them and no visible reason — the opposite of principle 3, and the §14 finding would be **less** visible, not more; ⚠️ **what ships is the mechanism, not the gate's enforcement** — ADR-0052's condition is on the **catalogue read**, so slice 1 must consult `ScopeVerdict.Elevated()` at **both `Containers()` and `StreamItems()`** — a container list **is** catalogue data (it is what the Libraries screen renders and what `BindContainers` writes rows from) and `Containers()` runs first, so gating the item walk alone would bind a library list under an ungraded credential; one `sync.Once` makes the wider gate free — and slice 0 ships the thing both reads will consult; ✅ **that obligation is DISCHARGED, 2026-08-19, at `862a0ca`** — `BookOrbitSource.gate()` consults the verdict from both entry points; ⚠️ **discharged is not vacated**: the condition is on the *catalogue read*, so a future read path that lands without consulting it makes the obligation live again; **adds no migration, no column, no crypto and no new HKDF label** — the secret rides the existing versioned AAD-bound `service_instance` envelope |
| [0059](#adr-0059) | The per-media-type facet count is an **assignment**: every work lands in **exactly one** bucket | **Accepted** — 2026-08-19; **records the counting semantics that shipped with `GET /api/v1/library/facets`** (`2711926`), whose wire contract is [`reference/http-api.md`](./reference/http-api.md) §8 and whose reasoning is `internal/store/facets.go` — **the numbers and the predicates live there and this ADR does not copy them**; a count cannot put one work in two buckets and remain a count, so the column would stop summing to the library and the same work would be reported twice in a summary whose whole question is *"what do I have?"*; the assignment is **`mediaTypeOf`'s**, the one that already renders every Block C and grid row's Type cell, so **a book held as both an EPUB and an M4B counts under Ebooks** and the count therefore equals the list a user reaches by clicking it — that equality is the decision's whole argument, and `TestMediaTypeCountsAgreeWithTheBrowseRead` pins it from the consumer's side; ⚠️ **the consequence, stated rather than buried: a library whose only audiobooks are second editions of ebooks reports `audiobooks: 0`**, while ARCHITECTURE §17.2's row-5 predicate — an independent `EXISTS` over `edition.format` — says that type has content; **the two disagree by design**, and this ADR is where a reader is meant to meet that; **closes off §17.2's independent-`EXISTS` shape as the counting rule** — it is the right shape for a *presence* test and the wrong one for a *count*, and the two are not interchangeable; **adds no endpoint, no migration, no column and no UI change** — the read, the index (`ix_edition_format`, migration `00009`) and the tests were already in the tree when this was written; ⚠️ **it is a live input to [ADR-0053](#adr-0053)**, whose reopening condition these counts appear to satisfy and do not — see that ADR's 2026-08-19 refinement, which this ADR is cited by |
| [0060](#adr-0060) | A stored credential can be **readable upstream**: BookOrbit's `raw_token` is bounded by **service-account privilege, not rotation** | **Accepted** — 2026-08-19; **records a measured property of the service v0.1 replicates from** ([ADR-0052](#adr-0052)) rather than a design UsArr chose — BookOrbit at `73b7877` stores the magic-link token in **plaintext beside its hash** (`magic_access_tokens.raw_token`, written by `MagicLinkRepository.create`, which is **not** what `loginWithToken` authenticates on) and **returns every token's plaintext to any superuser** through `findAll` behind `GET /api/v1/auth/magic-links`; **the two axes are deliberately NOT collapsed, which is this ADR's whole value** — *plaintext-at-rest is the class norm, which **Kavita confirms** (measured at tag `v0.9.0.2` / `6bcd568`, **not `main`**, which is frozen at v0.7.8 and has planted a wrong fact in these docs before: `AppUserAuthKey.Key` is plaintext with **no hash column at all**, plus the legacy `AppUser.ApiKey` the v0.8.9 migration copied verbatim); admin-retrievable-over-the-API is **not**, which **Kavita refutes** (no route returns another user's key — `MemberDto` carries no key field, every read resolves `UserId` from the caller's own claims, and rotate/delete each assert `authKey.AppUserId == UserId`)*; so the listing is **a step down the threat model names as such, not the ecosystem baseline**, and it is a fact about **the switch**, not a defect *of BookOrbit*; ⚠️ **because the credential is retrievable and rotation does not change that, the privilege of the account UsArr authenticates as is the ONLY control that bounds the consequence** — not one mitigation among several but the whole of UsArr's leverage, and it is already built ([ADR-0058](#adr-0058)'s grading); 🚫 **periodic re-minting is REJECTED, explicitly so nobody re-proposes it** — a re-minted token lands in the same plaintext column and the same listing, so rotation stays available for an **incident** and is **not advised on a schedule**; the difference the retrievability axis actually makes is **attribution** — minting emits `AuditAction.MagicLinkCreate`, reading emits nothing, and later use is counted against UsArr's own row; ⚠️ **the Trace-logging hedge qualifies RETRIEVABILITY ONLY and leaves plaintext-at-rest untouched** — a Kavita admin can reach live keys indirectly (`AuthKeyService.UpdateLastAccessedAsync` logs a **valid** key at Trace, the level is an admin-writable setting, `GET /api/server/logs` downloads the zip), so *"not admin-retrievable"* is true of the API surface and true by default, **not absolute** — a deliberate multi-step through a config change and a wait, **not a read**, and **not exercised on any live instance**; **amends ARCHITECTURE §14** with one clause that **generalises and names no service** — a credential UsArr stores may be readable by the service that issued it, so **UsArr's protections are necessary and not sufficient** and the model says which side of the boundary each protection sits on, a question **every future adapter** gets asked; ✅ **CLOSES** — rather than standing forever over a mutable upstream — when BookOrbit drops `raw_token` (both axes) or the listing stops returning plaintext (retrievability), which doubles as the concrete ask any upstream report carries; **ships no code, no migration, no column and no schema change** |
| [0061](#adr-0061) | Catalogue completeness is **measured and three-valued**: "not checked" is a state, not a zero | **Accepted** — 2026-08-19; **records the completeness verdict that shipped with the BookOrbit adapter's content-filter check** (`internal/bookorbit/stats.go`, `internal/libsync/bookorbitcompleteness.go`), whose wire contract is [`reference/http-api.md`](./reference/http-api.md) §2.6 and whose render is ARCHITECTURE §17.8; **the defect it answers is measured, not guessed** — BookOrbit at `73b7877d` puts a user's `contentFilters` in the books `LEFT JOIN … ON` condition with **no `.where()` at all** (`library.repository.ts:30-51`), so a filter **shorts each library's `bookCount` without dropping a library row**, while `getStats` (`library.repository.ts:150-178`) takes neither user nor filter, making `totalBooks − bookCount` the exact number of present books the credential was not shown; ⚠️ **a boolean `complete` is REJECTED explicitly, and that rejection is this ADR's whole point** — *"no shortfall"* and *"not checked"* are different facts, so the verdict is three-valued (`complete` · `shortfall` · `unverified`) with **`unverified` a first-class member**, enforced one level down as `Total` = **−1** rather than `0`, because `0` is a legal total for an empty container; ⚠️ **the degradation condition is named rather than assumed** — the measurement rests on an **unguarded upstream route** nobody promised UsArr, so if BookOrbit adds a `@RequirePermission` to `GET /libraries/:id/stats` every probe answers 403 and **every verdict becomes `unverified`**, which is the original defect recreated inside its own fix, and is why it is drilled rather than argued — `TestAGuardedStatsRouteRecordsUnverifiedRatherThanComplete` serves the 403 and asserts `unverified` with `Total = -1`; **it never blocks or refuses a sync** — a partial replica that says it is partial beats no replica, and the items that did import are correct either way; **every container gets a row**, `sync_report.kind = 'content_completeness'`, deliberately the **opposite** of the neighbouring `items_skipped` rule, because an absent skip row means nothing was skipped while an absent completeness row means nothing was **asked**, and those two absences must not look alike; ⚠️ **the claim's SCOPE travels in the row** (`covers` / `does_not_cover`) because a second axis is **unanswerable from a read-only account** — `LibraryAccessGuard` throws a byte-identical `ForbiddenException('No library access')` for *"the library exists and this account has no access row"* and for *"there is no such library"* — so `complete` on every library UsArr can see is **not** a statement that UsArr can see every library; 🚫 **probing library ids to find the hidden ones is REJECTED** — it is enumeration against the operator's own service and it does not even work, the guard's two refusals being identical; **`complete` renders nothing** on §17.8, which keeps that screen's standing invariant that nothing on it renders a positive health claim, and is why `unverified` has to be loud; **adds no migration, no column and no DDL** — `sync_report` carries no `CHECK` over `kind` (migration `00005`) and `detail` is already JSON, so the verdict's three sides share one declaration in `internal/store/completeness.go` and a typo is a silently missing verdict rather than a constraint violation; **the Kavita adapter is untouched and serves no verdict**, which renders as an absent key and is the seam: an adapter that can make the comparison implements the same shape and the screen needs no change; ⚠️ **superseded in part 2026-08-19 by [ADR-0063](#adr-0063)** — the `content_completeness` rule this ADR decides stands **unchanged**, and what falls is the `items_skipped` semantics §5 contrasts itself against: that kind now follows the **same** row-per-walked-container rule, so *"an absent skip row means nothing was skipped"* is no longer true and the two absences no longer differ |
| [0062](#adr-0062) | `usarr backup` captures the database **and** the KEK salt, and leaves the master key out — loudly | **Accepted** — 2026-08-19 |
| [0063](#adr-0063) | A walked container records a **zero-count skip row**; "none skipped" stops being an absence | **Accepted** — 2026-08-19; **supersedes [ADR-0061](#adr-0061) §5's absent-row semantics for `items_skipped`**, leaving ADR-0061's text standing and unreworded with an inline flag on §5, on the pattern [ADR-0052](#adr-0052) used against [ADR-0041](#adr-0041) clause 1; **the problem is a coupling, not a bug** — `42246c0` built a reader that needed three states from a table that offered two (*items left out* · *walked, nothing left out* · *nothing walked it*), and could only separate the last two by joining against the **completeness** row, which measures a **different axis** and was quietly the only per-container record in the schema that an import had gone near a container, so `cmd/usarr` recorded in as many words that *"stop writing a row for the clean containers and every one of those collapses back into silence"*; ⚠️ **and the two adjacent readers had OPPOSITE absence conventions** one §17.8 column apart — absent completeness meant *nothing was asked*, absent skip meant *nothing was found* — which is a hazard on its own; **the same-screen compensating control was ruled INSUFFICIENT by the owner**, in his words *"cross-referencing is what people stop doing"*; **the decision**: every container an import **walked** gets a row, zero or not, and a container nothing walked gets none; ⚠️ **the three states survive and only the EVIDENCE moves** — `left_out` is a non-zero row, `none` is a zero row, absent is no row, and the wire contract, the `SkipState` vocabulary and the §17.8 render are all unchanged (`none` still renders nothing, keeping that screen's standing invariant that it paints no positive health claim); **the completeness derivation is RETIRED rather than left as a fallback**, taking `ListLibraries`'s ordering constraint with it, because a dead cleverness kept just-in-case is a second code path nobody exercises; ⚠️ **the change belongs in the ADAPTER and not in `cmd/usarr`, which is load-bearing** — the tally map is populated by `tallyFor` at the top of one container's iteration **inside** the walk and is never pre-seeded, so the row set is the set of containers the walk **reached**; ✅ **the before-the-walk imprecision therefore genuinely DIES** — completeness is measured in `Containers()` before the walk, so an aborted import writes verdicts for containers it never reached, and those used to read *"nothing was skipped"* when the truth was *"not observed"*; ⚠️ **and what is STILL OPEN is stated rather than implied** — the one container the walk died **inside** carries a row from what it had read, so a clean partial read is indistinguishable from a clean complete one (at most one per import), not closed because withholding it loses genuine observed skips and contradicts the invariant, and marking it means a fourth state the screen cannot render; 🚫 **synthesising the zero rows in `cmd/usarr` from the container list is REJECTED and was fired deliberately** — the only list available there is `Containers()`'s, and seeding from it produces a clean zero for a container named `Never Reached`, which **moves** the imprecision into the field just decoupled from it; **a zero row carries no `reason` and no `effect`** (they explain a skip, and on a zero they assert a cause for a non-event) but keeps `covers`/`does_not_cover`, because a skip count is not a completeness verdict at any count; **the `StreamItems` LOG keeps its zero gate** — nobody infers "walked clean" from an absent log line, so the record is the row; **adds no migration, no column and no DDL** — same `kind`, and `sync_report.kind` carries no `CHECK` (migration `00005`, verified in the DDL); **no SQL and no plan change** — `librarySkipsSQL` and `libraryCompletenessSQL` still share one `containerReportSQL`, and `TestTheSkipStatementIsTheCompletenessStatement` was re-examined and **kept**; **the Libraries screen does not change**, since `skipMarks` and `skippedNote` already keyed on `left_out` and the absent key; ⚠️ **one reader behaviour inverts in the safe direction** — a library whose only skip row is undecodable now reads absent rather than `none`; **four existing assertions are INVERTED rather than deleted**, because an inverted assertion records that the decision changed and a deleted one is silence |
| [0064](#adr-0064) | BookOrbit's wire vocabulary is pinned by vendoring `packages/types` under `api/specs/`, guarded by an offline git-tree pin plus a network drift check | **Accepted** — 2026-08-19; **extends [ADR-0047](#adr-0047)'s two-half guard shape from one file to a directory** and leaves [ADR-0046](#adr-0046), [ADR-0047](#adr-0047) and [ADR-0052](#adr-0052) standing and unreworded — the change carrying it edits no ADR at all; **the problem is that [ADR-0052](#adr-0052) made BookOrbit the SOLE catalogue source** while every wire shape in `internal/bookorbit` is **hand-transcribed** from BookOrbit's TypeScript citing `73b7877d`, so an upstream rename or retyped field was not the degradation of one adapter among several but a single point of failure for the whole library, **invisible until an import broke against a real server**; ⚠️ **there is nothing else to vendor** — BookOrbit commits no OpenAPI document, `server/src/swagger.ts` builds one at **runtime** and `main.ts` mounts it only under a default-false `SWAGGER_ENABLED`, so [ADR-0046](#adr-0046)'s floor/ceiling split has nothing to bite on and no "fetch it from the running instance" recipe exists either; **the decision**: vendor `packages/types` **verbatim and whole** — all **68 files** at `73b7877d` — to `api/specs/bookorbit-types/`; **`api/specs/` rather than `docs/reference/`, DECIDED rather than inherited**, settling an inference `docs/ROADMAP.md` explicitly flagged as one to settle — `api/specs/SOURCES.md` opens *"vendored verbatim, never fetched at build or test time"* and carries a provenance table contract tests read, while `docs/reference/` holds hand-written Markdown and **not one vendored artefact**; **the identity is a git TREE name** (`4cb990a36b8325845abb79eb4b7a4445e6df679b`) and not a SHA-256 of our own devising, which buys two things a home-grown hash would not — upstream is **comparable with no download**, since a blobless fetch resolves a path to its tree name out of the tree objects alone, and the value is **upstream's own name**, so nobody must trust that we hashed the right bytes in the right order; ⚠️ **nothing may be added inside the vendored directory** — one extra file changes the tree and destroys that identity, which is why the manifest lives **beside** it and never in it; **two authorities and one diagnosis**: the tree name moves for **any** byte (right offline, where the bytes are frozen), a comment-blind **declaration digest** moves only when a type, field, enum member or literal union changes (so an upstream comment rewrite does not read the same as a rename — **an alarm that is usually noise is an alarm nobody reads**), and the per-file manifest is *"a diagnosis and never an authority"*, read only to turn *"the tree hash differs"* into *"these files differ"*; ⚠️ **the digest is a LEXER THAT REFUSES RATHER THAN GUESSES, which is why five files are digested and not sixty-eight** — `stripTypeScript` cannot tell a regex literal from a division, and `src/pattern-resolver.ts:71` declares a character class **containing a double quote** that would trap the scanner in a string state it never leaves, so it **asserts it finished in code state** and errors otherwise rather than pinning a file to nonsense; **the guard is split across the network line** on [ADR-0047](#adr-0047)'s reason — three offline checks in **`make check`**, the upstream comparison behind the `upstream` tag in **`make spec-drift`** only, because `make check` makes exactly two network calls and a third would let a GitHub outage redden an unrelated commit; ⚠️ **the network half asks TWO questions and they are not the same news** — the tree at the **pinned commit** not matching is **our** bug and invalidates every transcription, while the tree at **`main`** having moved is **upstream news**, graded per file by whether `internal/bookorbit` transcribes it; ✅ **`SPEC_DRIFT_FLOOR` was raised 1 → 2 in the same change, and that is what makes the drift check non-vacuous** — `go test -run` matching nothing exits 0, so a floor is the only thing standing between a green and a green over zero checks; 🚫 **rejected**: vendoring only the five transcribed files (silently un-covers the next slice, and a guard that quietly shrank is indistinguishable from one that held), a plain SHA-256 over a concatenation (not upstream-comparable, and demands trust), the tree hash as the **only** signal (cries wolf on comment churn and on a Prettier rewrap), [ADR-0046](#adr-0046)'s floor/ceiling (no committed document and no release-tag line to split), and vendoring the NestJS server; ⚠️ **the honest limit is stated rather than implied** — it pins **the file we vendored, not the server the owner runs**; it sees **types, not behaviour**; it does **not** check whether our transcription reads them correctly (`TestMediaKindVocabularyMatchesTheSource` and `TestPermissionVocabularyMatchesTheSource` stay necessary); the server's own **NestJS DTOs are unpinned by anything**, the largest uncovered surface; and **nothing runs the network half on a schedule** — there is no CI and `make spec-drift` is a thing a person types, so this ADR claims no cadence |
| [0065](#adr-0065) | The BookOrbit cover fetch runs **inside the import, between committed batches** — not on a render path, and not inside `applyOneItem`'s transaction | **Accepted** — 2026-08-19; **the SHAPE is a restatement of ARCHITECTURE §4.4.1 rule 4 rather than a novelty** — *"the grid paints from `work` rows as import phase A commits; images fill in behind, and the grid is never blocked on the image queue"* — so covers are fetched **during the import** and there is **no per-work on-demand entry point**, a render path being disqualified by principle 1 and [ADR-0004](#adr-0004) because a synchronous upstream call on a render is exactly what they refuse; ⚠️ **the LEVEL named in the first ruling was WRONG, and the correction is MEASURED** — `applyOneItem` runs inside `ApplyCatalogueBatch`'s single `BEGIN IMMEDIATE` transaction, which spans a whole batch at `min(2000 rows, 100 ms)` on a writer pool hard-capped to **one connection** (`write.SetMaxOpenConns(1)`, *"Exactly one. This is the entire single-writer discipline."*), so a fetch there would hold the process's only writer across a network round trip **per book** and serialise the entire import behind upstream latency — which `internal/db/sqlite.go`'s `Write` doc already forbids in writing: *"fn must not call Write, and must not hold the transaction across a network call: the whole process shares one writer connection"*; **so the pass sits in `internal/libsync`, between committed batches**, keyed by **remote id** with the work re-resolved inside the store's own transaction, because `ImportedItem` carries **no work id by design** and `PosterAsset`'s header names the hazard — an id read before a network round trip is an id read arbitrarily long ago, and `work.id` being `INTEGER PRIMARY KEY` turns a reused id into **the wrong book's cover**, silent and visible only as art that does not match its title; **two in-tree precedents, both built before this question was asked** — credits were moved OUT of the stream callback (`FullImport` phase 3: issuing per-item GETs from inside it *"would hold the streaming connection open across all of them"*) and `PutPosterAsset` opens *"The bytes are already on disk when this runs"*, a sentence a fetch inside the transaction would falsify; **a cover-fetch failure NEVER fails the import**, a partial catalogue that says it is partial beating no catalogue, on `FileReadFailures`' existing *"NOT an import failure"* shape; ⚠️ **concurrency is bounded and the bound is BUILT rather than INHERITED** — §4.4's `min(NumCPU, 4)` is **prose** and is about **transcoding**, so this decision's own first constraint was wrong and is carried verbatim rather than quietly fixed: *"§4.4's semaphore is scoped to transcoding… fetch concurrency needs its own bound, stated in its own terms, not borrowed from a limit that exists for a different resource"*; 🔍 **and one supporting claim was CHECKED AND IS FALSE, so it is not leaned on** — `internal/imagepipeline/pipeline.go`'s *"the tree's only one is the Argon2id gate"* does not hold, `internal/releases/search.go` having bounded its indexer fan-out at `DefaultConcurrency = 6` since `dd15d95`, which is a bound over concurrent **network legs** and therefore the closest precedent rather than a counter-argument; **a 404 is "absent for this credential" and never terminal** — BookOrbit throws the same `NotFoundException` for a missing cover file, a missing book, and a book the credential's content filters hide, so a 404 is **not evidence about a file** and the next import retries rather than inheriting a permanent verdict, which deliberately NARROWS `cover.go`'s own *"caching absence on a 404 is sound"*; ⚠️ **what it does NOT decide is stated rather than implied** — whether whole libraries are hidden from the account remains **unanswerable read-only** ([ADR-0061](#adr-0061)), the permit count and the pass's exact slot in `FullImport` are implementation (though `PutPosterAsset` sets no `rollup_dirty` — verified — so a cover pass does not compete with the file walk for the last-before-flush slot), and within-import retry is left open; **the pipeline's *"NEVER AGAINST A REAL COVER"* caveat is untouched and stays exactly as loud**; **adds no migration, no column and no DDL, and edits no other ADR's reasoning** — two mechanical anchor/rule repairs elsewhere in this file ride along in the same change and touch no decision · ⚠️ **amended 2026-08-19** — decision 5's *"a fetch bound, stated in fetch terms"* is less precise than the bound that shipped: one permit is held across fetch, decode **and** render, so it caps concurrent transcodes and caps connections only incidentally, and **N permits is not N concurrent requests**; **the decision itself — bounded, built rather than inherited, own constant and own refusal test — is unchanged** |
| [0066](#adr-0066) | A wholly skipped BookOrbit library is **bound with an honest zero**, never declined; and the whole-library sentence says **what happened**, not just how many | **Accepted** — 2026-08-19; **builds on [ADR-0063](#adr-0063)** — the zero-count skip row is what makes an all-skipped library representable, so this ADR is about **what the screen says**, not about the schema — and **amends no ADR's reasoning**, editing no other entry in this file; **it is the ADR `internal/libsync/bookorbit.go` asked for by name**: *"Splitting one container into two libraries is a deviation from §17.8 that needs an ADR, and it is comics' slice to ask for"*; **the case is not hypothetical** — comics are skipped inside the walk today (`StreamItems`: `case bookorbit.MediaKindComic: tally.Comics++; return nil`, on the unit-of-work gap its package doc names, BookOrbit's series having no library to bind a comic's series work to) while **no container is declined at all** (`Containers()`: *"NOTHING IS DECLINED HERE… BookOrbit says nothing, so there is no answer to decline on"*, its `libraries` table carrying no type/kind/mediaType column), and **the owner's own BookOrbit keeps comics and prose in SEPARATE libraries** — his words, 2026-08-19, *"libraries are split up"* — so on the install v0.1 is proven against a comics library **walks, reports and yields nothing**, which is a whole library on his screen rather than an edge case; **the decision**: such a container is **BOUND** with an item count of zero and a sentence, and **no adapter declines one for being wholly skipped** — ratifying what the code already does, precisely because the tempting change is the one that makes the row disappear; ⚠️ **and the whole-library case gets its OWN sentence, because the count is not the message** — *"42 items were read and not mapped"* is a report **on the remainder** and reads as *some of this arrived*, so when the 42 IS the library the identical sentence reads as *this is broken*; the specified pair is *"Nothing in this library is imported yet"* over *"All 412 items were read. UsArr does not import comics yet."*, with **three acceptance criteria rather than taste** — it asserts the read succeeded with the count as evidence, it names the **media type** and the reason, and **it must be impossible to mistake for a connection failure**; **the condition is DERIVED and no wire field is added** — `item_count == 0` ∧ `skipped.state == "left_out"` ∧ `skipped.items > 0`, both fields shipping today, with the **empty upstream library** staying distinct for free since it walks clean to `state: "none"` with `items` absent; 🔍 **`skipEffect`'s remainder clause is measured FALSE on this shape and recorded as false rather than left to be found on a screen** — *"every other book in the library was imported"*, when there is no other book (it travels to `sync_report` and not to the cell, on §9.1's split, which bounds the blast radius and not the wrongness); **the MIXED container is decided too, and it becomes TWO library records** — `library.kind` is *"Exactly one, required"* under a `CHECK` that **already permits `'comic'`**, and `library_source`'s uniqueness is per `(library_id, …, container_ref)`, so two libraries may name one container and **no migration is needed**; until comics import, a mixed container binds exactly as today, one `book` library with its comics counted, which is the partial case and renders the partial sentence; ⚠️ **that half is DESIGNED AND UNTESTED, stated plainly so untested does not read as unsupported** — the owner's split libraries mean **his data will never exercise it**, and the precedent for the distinction is this project's own **synthetic-cassette rule** (`internal/bookorbit/vcr_test.go`: *"A SYNTHETIC CASSETTE PROVES THIS CLIENT'S PARSING, NOT THE SERVER'S BEHAVIOUR"*), where a green is **spec evidence only** and the tests are kept and trusted for exactly what they cover; 🚫 **DECLINING THE LIBRARY IS THE ALTERNATIVE THIS ADR CLOSES**, and it loses on three counts — invisible reads as broken, since §17.8 renders `library` joined to `library_source` and a declined container has neither, leaving a green Services row, a library the owner can see upstream and **no row at all**, which is principle 3's *"empty screen that looks broken"* exactly; the decline lands in `container_declined`, which is **measured to have no reader** in `internal/store`, `internal/httpapi` or `web/`; and it makes the state **unstable across a release**, a library materialising out of nowhere with a full catalogue the day comics import, where binding now turns that day into a zero becoming a number on a row the user has already seen; 🚫 **also rejected**: rendering nothing (`none` is the **measured negative** meaning *nothing was left out*, so using it where **everything** was would make the column's one honest silence say the opposite of the truth), reusing the partial sentence, a `whole_library` wire field or a third `SkipState` member (**"and also"** — derivable from two shipping fields, and a second thing that can be absent or disagree, on ADR-0063's own ground for declining a fourth state), and giving the row `kind = 'comic'` today (it could only be inferred **from the tally**, i.e. from what the walk happened to read, and §6.4's cascade makes a wrong `work.kind` unmergeable); ⚠️ **one residual is recorded rather than closed** — the row's kind is `book` while its contents are comics, mitigated only by `kind` being *"EDITABLE (§6.5 rule 4)"*, and closed when comics get a unit of work; **adds no migration, no column, no DDL and no wire field**, owes the implementing slice exactly **two strings**, and **nothing here is built** — it rules behaviour the §17.8 screen and the comics slice implement → **2026-08-19: decision 5 is built** — the comics import, content `10444a4`, with tonight's rulings on it in `ff13582`; read it off `internal/store/catalogue.go`'s `parentBinding` and `bindOneContainer`, where a MIXED container's `comic` sibling is minted lazily on the first comic reached and named `Fiction (Comics)` for its kind. **Decision 1 needed nothing built** — it *"ratifies what the code already does"* in its own words, and `bindOneContainer` still binds a wholly skipped container. **Decisions 2–4 measured unbuilt the same day**: the specified word and detail (*"Nothing in this library is imported yet"*) appear nowhere under `web/`, decision 3's derived condition — `item_count == 0` AND `left_out` AND `items > 0` — is written nowhere, and `skipEffect`'s remainder clause still reads *"every other book in the library was imported"* at `cmd/usarr/import.go`. ⚠️ **No status mark, because no decision changed.** The text above stands unreworded and this is a dated pointer at the tree, not a fresher status claim — and the tree, not this row, is what is authoritative for any of it. |
| [0067](#adr-0067) | A pasted BookOrbit **magic link is accepted and reduced to its token**; the refusal becomes the fallback | **Accepted** — 2026-08-19; **reverses a ruling taken the same morning and records both**, because the first one was correct reasoning on a premise that turned out to be false — `ab9e0f3` refused a pasted magic-link URL on the belief that BookOrbit's copy button *"yields a URL, while POST /api/v1/auth/magic-links/login wants the bare token"*, read as *an artefact its own API cannot consume*; **reading the consumer falsified it** — `client/src/router/index.ts` declares a public `/magic` route, `MagicLinkLoginView.vue` takes `route.query.token` and strips it from history, and `useAuth.loginWithMagicLink` POSTs `{"token": raw}`, so **URL in / bare token out is an adapter BookOrbit already implements**, and `MagicLinksSettings.vue` offers the operator nothing else (the table renders the label, the account, the expiry and the use count, never the raw value); **measured at `73b7877d2fede2221b0ca360af9bfced7c3797f3`, cited as a commit because the tag `v2.6.0` was NOT verified to point at it**; **found by a live failure on the owner's install**, not by review; **leaves [ADR-0060](#adr-0060) standing and unreworded**; the price is named rather than buried — the accept rule is a **whitelist**, so an upstream token-format change would have UsArr refuse a valid credential |
| [0068](#adr-0068) | A BookOrbit comic is an **issue**, and issues are **minted under series works**; `seriesId` null synthesizes a one-shot series, extra memberships are **recorded, not resolved** | **Accepted** — 2026-08-19; **this is the "unit of work" [ADR-0066](#adr-0066) decision 5 was waiting for** — *"The kind stays `book` until comics have a unit of work"* — so it activates that decision's two-library split rather than reopening it; **[ADR-0030](#adr-0030)'s model is applied, not amended**: `comic` is the series, `comic_issue` the issue, verified at migration `00005_library_sync.sql:256` (*"'comic' is the SERIES, 'comic_issue' the issue or chapter"*) and `00006_kavita_subtypes.sql`'s header; **the parent binding is MEASURED, not inferred** — `BookCard.seriesId` is not an arbitrary `memberships[0]` and is not null under multi-membership, it is BookOrbit's own maintained **primary** (`series-membership.service.ts`, `displayOrder = 0`, round-tripped by `syncPrimaryMetadata` and `syncPrimaryFromMetadata`, at commit `73b7877d2fede2221b0ca360af9bfced7c3797f3`); **`seriesMemberships[]` beyond the primary is RECORDED and not acted on**, on [ADR-0063](#adr-0063)'s precedent, the fuzzy tier that would resolve it staying v0.3 via `work_relation`; **`is_oneshot` is WRITTEN rather than merely tolerated** — *"a column with a DEFAULT 0 and no writer is a deaf column"*; **both residue defaults emit a `sync_report` row**, so sizing comes from instrumentation rather than from estimates; **no migration, no column, no DDL and no new wire field** — `sync_report.kind` carries no `CHECK` by design and `library.kind` already permits `'comic'`; ⚠️ **the done-check FAILS if series count equals issue count**, because that is the per-row shape [ADR-0066](#adr-0066) already pre-emptively refused |

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
  not claim one. **The fuzzy ladder is deferred, not rejected** — `work_relation` ~~already carries~~
  **is designed to carry** the `confidence` and `evidence` columns it would populate, so adding it
  later is writing lower-confidence rows plus a surface, with no change to how edges are read,
  grouped or rendered ([`FUTURE.md`](./FUTURE.md) §5).
  **[Tense corrected 2026-08-19; the decision is untouched.]** The DDL is
  [`reference/schema.md`](./reference/schema.md) §11 *Cross-media edges · **v0.3***, which is the
  design of record. ⚠️ **The table is deferred to v0.3 and no shipped migration creates it** —
  `TestDeferredTablesAreAbsent` (`internal/db/migrate_test.go`) fails the build if one does — so the
  seam is a designed shape to build to, not a column pair sitting in the schema waiting.
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
- ~~`work_relation` carries **`evidence` (JSON) and `status`**, not just a confidence float,
  because the review inbox is unusable without being able to explain *why* two things linked.~~
  **`work_relation` is designed to carry `evidence` (JSON)**, not just a confidence float, for that
  reason.
  **[Corrected 2026-08-19 on two counts, neither of which reopens this ADR's decision.]**
  **(1) Tense.** The DDL is [`reference/schema.md`](./reference/schema.md) §11
  *Cross-media edges · **v0.3***, which is the design of record; ⚠️ **the table is deferred to v0.3
  and no shipped migration creates it**, and `TestDeferredTablesAreAbsent`
  (`internal/db/migrate_test.go`) fails the build if one does.
  **(2) `status` is no longer part of the design at all** — [ADR-0007](#adr-0007) revision 2 removed
  the review inbox (see [ADR-0019](#adr-0019)'s user-scoped list, which strikes its verdicts for the
  same reason), and §11 says in as many words that `status`, `reviewed_by` and `reviewed_at` are
  **not** in this table. The stated reason — the inbox being unusable without an explanation — is the
  reason for `evidence`, which survives; the inbox it named does not.

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
   documents. Both are now invariants asserted by `make check`.

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
  **[Corrected 2026-08-19; the decision is untouched and `paths.relative: false` stays
  pinned.]** The 🔍 marker's own condition — *"it must be tested empirically before the build config
  is frozen"* — has been met, and the inference did not survive it. What is corrected is
  *"`paths.relative` defaults to `true` and **likely breaks assets under a deep-route SPA
  fallback**"*, and with it *"so `paths.relative: false` is the expected setting"*: it is not
  what makes deep routes work. [`REVIEW-LOG.md`](./REVIEW-LOG.md)'s **B-01** built the SPA both ways
  and diffed the output. SvelteKit already special-cases the fallback document and skips the
  relative-path rewrite for it — `@sveltejs/kit` 2.70.2, `src/runtime/server/page/render.js`,
  `if (paths.relative) { if (!state.prerendering?.fallback) {` — so
  `index.html` emits root-absolute `/_app/…` with the setting either on or off, and `relative`
  governs only genuinely prerendered pages, of which this SPA has none. **The setting is
  belt-and-braces, not load-bearing.** It stays pinned to `false` because explicit beats relying on
  a special case and it costs nothing, not because anything breaks without it; the property that
  actually has to hold is guarded by `TestFallbackAssetPathsAreRootAbsolute`
  (`internal/web/web_test.go`), which asserts the fallback's asset URLs are root-absolute whatever
  `svelte.config.js` says. ⚠️ **Nothing is re-measured here** — B-01 is the measurement and this is
  it being applied. **Why it is applied now.** B-01's disposition was *"Correction recorded here
  rather than applied"*, because the styling ADR was on another branch when the finding was
  written; §5's *Left for the owner* bullet was re-confirmed on 2026-08-19 as *"The bullet stands
  as the round's record"*, which flips the other half of that line and explicitly does **not**
  re-verdict this one; and **LS-380** ⏭️ recorded *"Correcting ADR-0025 §6 itself is deliberately
  NOT done here … Routed, not assumed done."* This is that routing arriving, under LS-380's id
  rather than a new one. What makes it due now is `web/svelte.config.js:38`, which carries a
  **live instruction** — *"but it is NOT load-bearing. ADR-0025 §6 should be corrected."* Until the
  content commit `ce17389` (2026-08-19) that sentence read `ADR-0024 §6`, an ADR with no numbered
  sections at all, so it was unfollowable and inert; `ce17389` aimed it at a section that exists.
  **Decision point 6 stands as decided.** The `all:` prefix, `precompress` and its handler,
  `paths.base: ''` and the `paths.relative: false` value are all unchanged. Only the
  reason given for the fourth trap is corrected.

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
- **Library membership is never an input to identity**, to be enforced by an assertion in
  `make check` that no query in the identity path references `library_member`, `library_source`
  **or `library_override`** — owed, not written: there is no identity cascade and no correction
  applier yet for it to read (`reference/schema.md` §13.5).
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
  denormalise the sort key onto `library_member`. It is an `EXPLAIN QUERY PLAN` assertion in
  `make check` and a `make bench` line, not an assumption.

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
`design/DESIGN-DIRECTION.md` OQ-2 in the sidebar's favour · ⚠️ **Amended 2026-08-19 by
[ADR-0053](#adr-0053)** — the data-driven **sidebar** clause only; see the block below.

> ⚠️ **AMENDED 2026-08-19 by [ADR-0053](#adr-0053): the sidebar renders all six media types
> unconditionally, and the sidebar is the only thing that moves.** **What no longer holds:** the
> Decision's *"showing only types that have content"*, and the **sidebar** third of the consequence
> *"A type with zero items is not rendered anywhere — sidebar, home, or search group"*. Both need a
> read answering which of the six types have rows, and there is none:
> [`reference/http-api.md`](./reference/http-api.md) §7.1 states there are *"no facet counts beside
> the chips; each is its own aggregate and its own read"*, and nothing UsArr serves answers per-type
> presence either. Each falsified clause carries an inline flag at its own site.
>
> **What still holds — including two thirds of that consequence.** The two axes and the rule that
> they are never merged, media type as the navigation axis, a library as scope, the `?lib=` URL
> state, the `LibrarySelector` chip and its three owed keyboard behaviours, pinning, the row budget
> (whose arithmetic already sized the sidebar at all six types, so it is unaffected — its *"≤6
> types"* is now simply exactly six), and every rejected alternative. **Home and search groups keep
> the zero-items rule**: ADR-0053 amends the sidebar clause **and no other**, because neither of the
> other two is contradicted — Block A's rollup is blocked on the same missing read and draws nothing
> at all, and a search group's count is a property of a response the screen already has.
>
> **This is one clause narrowed on a wire fact, not a reopening of the navigation model**, and the
> rule is *closed rather than abandoned*: ADR-0053 reopens it on one named condition. That decision
> lives there; this note points at it and does not re-argue it.

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
> comics — rendered as sidebar entries, ~~showing only types that have content~~** ⚠️ **all six
> render unconditionally — amended 2026-08-19, [ADR-0053](#adr-0053)**. **A library is a *scope*,
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
- A type with zero items is not rendered anywhere — ~~sidebar,~~ home, or search group. 🚩 **The
  sidebar third is struck 2026-08-19 by [ADR-0053](#adr-0053), and struck rather than deleted
  because it is one of the two clauses the amendment at the top of this ADR exists for.** All six
  media-type entries render unconditionally, in one fixed order, on every install, and no sidebar row
  carries a count — because no read answers which types have rows, and `reference/http-api.md` §7.1
  publishes *"no facet counts"*. The honesty this row used to carry moves to the per-type screen,
  which says which of three reasons made it empty — rather than the row vanishing from the nav and
  taking the only explanation with it. **The other two thirds stand exactly as written**, and ADR-0053 says so
  in its own consequences: home and search groups are not contradicted by anything on the wire.

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
  for fixing it already exists: `work_relation` ~~carries~~ **is designed to carry** typed edges with
  `confidence` and `evidence` (ADR-0009), so a `same_person` edge is an added edge type rather than a
  new table.
  **[Corrected 2026-08-19; the decision is untouched.]** The DDL is
  [`reference/schema.md`](./reference/schema.md) §11 *Cross-media edges · **v0.3***. ⚠️ **The table
  is deferred to v0.3 and no shipped migration creates it** — `TestDeferredTablesAreAbsent`
  (`internal/db/migrate_test.go`) fails the build if one does — so this seam is a designed shape, and
  "the seam for fixing it **already exists**" is true of the design and not of the schema. "Rather
  than a schema change" is narrowed to "rather than a new table" for a second reason found while
  checking the first: §11's `rel_type` is a `CHECK` over thirteen named values and `same_person` is
  not among them, so the edge type arrives in whichever migration creates the table, or in a later
  one that rewrites the `CHECK`.
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
carries the same reasoning next to the SQL. · ⚠️ **amended 2026-08-19** — see the block below:
decision 1's Go declaration is **discharged** by `007e58e`, its runtime validation is **not**, and
the obligation on *"whoever writes the first `write_queue` writer"* is **not released but made
unavoidable**. **No decision changes**; the `CHECK` is still dropped and `work_id` still gains its
key.

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
> ⚠️ **Amended 2026-08-19, and clause by clause because the sentence is half true.** The
> **declaring** half is **discharged**: `internal/store/writequeue.go` (`007e58e`) is the
> vocabulary's single Go home. *"Nothing validates it today"* is therefore **false about the
> declaration** and **still true about the runtime** — there is **no production writer**, so nothing
> validates because nothing writes. The obligation on *"whoever writes the first writer"* is **not
> released; it is made unavoidable.** See the amendment block below.
>
> **2. `'awaiting_choice'` is excluded from `ix_wq_runnable`'s partial predicate**, which stays
> byte-identical to `00001`'s `WHERE state IN ('pending','inflight','verifying')`, with the reason
> written beside it in the SQL.
>
> **3. `write_queue.work_id` regains `REFERENCES work(id) ON DELETE CASCADE`.**
>
> **4. `fail_reason`'s `CHECK` is kept**, in its `IS NULL OR … IN (…)` form.

### ⚠️ Correction, 2026-08-17 — decision 1's Go validation is **owed**, and this ADR twice wrote it as done

> ⚠️ **This section is true of its own date and is superseded in part on 2026-08-19.** Everything
> it measures held on `49dfa6c`. What it calls *owed* is now **half discharged** — see the
> amendment at the end of this section. It is kept verbatim because a retraction kept in place is
> this file's convention and a deleted assertion is silence.

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
⚠️ **Amended 2026-08-19, splitting that sentence.** *"It is not there yet"* is now **false** —
`internal/store/writequeue.go` is there, and it compiles into the binary under no build tag. *"Nowhere
that runs"* is **false about the declaration** and **still true about any validation actually
happening**, because no production writer calls it. Of the two things the first writer owed, the
**declaration has landed** and only the **call** remains.

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

### ⚠️ Amendment, 2026-08-19 — the validator shipped, there is still no production writer, and the first one now cannot skip it

**Why an amendment section here, when the 2026-08-17 correction was made in place.** This ADR's own
📌 note sets the rule: a correction goes in place when *"the sentence was wrong the moment it was
written"* and nothing in the world changed, and an `⚠️ Amendment` section is for **a fact about the
world changing under a standing decision** — ADR-0035 §2a's precedent. This is the second case. The
2026-08-17 text was true of its date; `007e58e` changed the tree on 2026-08-19. So the original stays
verbatim and the new fact is appended beside it.

**The honest statement needs three clauses, and any two of them mislead.**

1. **A validator exists, and it is the single Go home for the vocabulary.**
   `internal/store/writequeue.go` declares the six states as constants — `pending` · `inflight` ·
   `verifying` · `awaiting_choice` · `done` · `failed` — and `ValidWriteQueueState` at `:91` answers
   membership. The six agree with decision 1's list and with
   `00005_library_sync.sql:906-913`'s column comment; all three were checked against each other
   rather than assumed.
2. **The tree's only `write_queue` writer routes through it — and that writer is a BENCH FIXTURE.**
   `internal/db/spike/fixture.go` is behind `//go:build bench`, so `go build ./...` never compiles
   it. It validates every state it can produce before the `INSERT`, exhaustively rather than by
   sample.
3. **There is still no production writer.** So **at runtime nothing validates anything, because
   nothing writes anything.** `internal/httpapi/grabs.go` states the same thing in the tree, and is
   cited rather than paraphrased: *"Nothing in v0.1 writes `write_queue`; the one writer in the tree
   is the bench fixture behind `//go:build bench`."*

⚠️ **So the obvious wording — *"the vocabulary is now validated in Go"* — is FALSE, and falsely in
the quiet way.** It is the sentence a reader writes after seeing clause 1 and stopping. It would
repeat this ADR's original defect in the opposite direction: 2026-08-17 claimed a validator that did
not exist, and this wording would claim an enforcement that does not run.

**The claim actually worth recording is stronger than either *"never written"* or *"now validated"*.**
It is this: **the first production writer cannot be written without validating, and the guard proving
that has already caught a real writer.** `TestWriteQueueWritesValidateTheStateVocabulary` walks the
AST of production sources and fails the build if anything writes `write_queue` while nothing
references the validator. It **did not start vacuous**, unlike its `image_asset` sibling: it went
**RED unplanted** against the bench fixture the moment it landed, and turning it green required
**routing that code through the validator** rather than adjusting the test. That is the in-tree
record, in the guard's own words — *"it went RED on that writer before the writer was routed through
`ValidWriteQueueState`"*.

**This is what discharges the obligation, and note the form.** Decision 1 put the duty on *"whoever
writes the first `write_queue` writer"*. A duty stated in prose is exactly what this ADR already lost
once — the 2026-08-17 correction exists because *"the promise lived only in prose"*. What shipped is
not a reminder but the thing that makes the duty **unskippable**. The guard is a **floor, not a
proof**, and it says so: it checks that the validator is **referenced**, not that it is called on the
right value at the right moment, and a query assembled by `fmt.Sprintf` where verb and table never
share a string literal still walks past. **What it removes is the silent-skip path**, which is how
the last one was lost.

**What this amendment does NOT change.** Decisions 1–4 stand exactly as taken. `write_queue.state`
still carries **no `CHECK`** — the Go validator is not a schema constraint, SQLite will still accept
`'pendign'` from any writer that bypasses it, and that cost is still the one decision 1 accepted
knowingly. `'awaiting_choice'` is still **excluded** from `ix_wq_runnable`'s predicate (decision 2);
`work_id` still gains its foreign key (decision 3); `fail_reason`'s `CHECK` is still **kept**
(decision 4). Being a **legal** value and being a **runnable** one remain different questions, and
`ValidWriteQueueState` answers only the first.

📌 **One known inaccuracy is recorded rather than fixed, because it is unfixable.**
`internal/db/migrations/00008_image_asset_format.sql:149-153` still states the pre-`007e58e` position
in the present tense. A **merged migration is never edited** (`CLAUDE.md`), so it stays as it is and
this block is where a reader learns it is stale. The four Go comment sites that carried the same claim
— `internal/store/images.go`, `internal/store/imagewrite.go`, `internal/store/imagelint_test.go` and
`internal/httpapi/grabs.go` — were **all discharged by `007e58e`** and are **not** outstanding; each
keeps its old wording inside a past-tense retrospective and then cancels it, so **a grep match there
is a correction, not a defect**.

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
  ⚠️ **Amended 2026-08-19 — the first clause is corrected and the second is NOT.** *"Does not
  exist"* is **false**: `ValidWriteQueueState` exists. *"The only mitigation running today is
  `ix_wq_runnable`'s predicate"* is **still exactly true**, because a validator nothing calls
  mitigates nothing at runtime, and nothing calls it — there is no production writer. What changed is
  not today's mitigation but tomorrow's: the AST guard makes the Go check **unskippable** by the first
  writer, so the missing half is now scheduled rather than merely owed.
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
  ⚠️ **Amended 2026-08-19: that code exists.** `internal/store/writequeue.go` (`007e58e`) is the
  one home, so the bullet's headline — *"exactly one home, and it is Go"* — is now **literally true**
  rather than aspirational, and the closing sentence's gap is **closed for the declaration**. The
  worker still owns the **calling**, and until it exists nothing validates at runtime.
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
see the flag below. · ⚠️ **Amended 2026-08-19 by [ADR-0052](#adr-0052)** — clause 1's
source is no longer Kavita; see the second flag below.

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

> ⚠️ **AMENDED 2026-08-19 by [ADR-0052](#adr-0052): clause 1's source is now BookOrbit, and
> clause 4 is REOPENED rather than re-answered.** The owner is sunsetting Kavita entirely and moving
> his books, comics and manga to BookOrbit — his decision, his word for it *"phenomenal"*. **What
> falls:** clause 1's *"v0.1's catalogue source is **Kavita**"*, and clause 4's *"v0.1's sync channels
> for its catalogue source are 1, 3b and 4"*, which rested on [ADR-0035](#adr-0035) §2a's live probe
> of **Kavita** and has no BookOrbit equivalent — ADR-0052 carries it as an open question with the
> probe named. **What stands, unchanged:** clause 2 (*the count is unchanged at one*), clause 3 (the
> \*Arrs re-sequenced not cut), the whole *"the sync core is source-agnostic"* argument — which is
> what ADR-0052 spends a second time — and the three subtype tables, which are not Kavita-specific.
> ⚠️ **The Kavita adapter is NOT deleted**: ADR-0052 clause 2 keeps `internal/kavita`,
> `internal/libsync/kavita.go`, both vendored specs and [ADR-0046](#adr-0046)'s contract guard in the
> tree and green. This ADR's own *"re-sequenced, not cut"* framing is what ADR-0052 extends to it.
> **No sentence of this ADR's body is reworded**; the two falsified clauses carry inline flags.

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
> 🚩 **STRUCK 2026-08-19 by [ADR-0052](#adr-0052)** — v0.1's catalogue source is **BookOrbit**.
> *"The sync machinery is built now"* and *"its first adapter"* both survive; only the **name** of
> the source changes, for the second time and for the same reason — it is the one the owner runs.
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
>
> 🚩 **REOPENED 2026-08-19 by [ADR-0052](#adr-0052)** — this clause is about **Kavita**, and it was
> earned by [ADR-0035](#adr-0035) §2a's live run. **BookOrbit has had no such run**, so channel 3b's
> applicability to it is an **open question**, not an inherited answer. What is known: `updatedAt` is
> an allowed sort key with page/size paging, so the walk is *expressible*. What is not: whether that
> timestamp moves on a tag, genre or author edit. §7.1a's **reconciliation-only** fallback is the
> named failure branch. **Channel 3 remains inapplicable either way** — BookOrbit has no
> `/history/since` any more than Kavita does.

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
> a table with all three of its indexes and no writer. ⚠️ **Still true of production as of
> 2026-08-19, with one narrowing**: `007e58e` added a **bench-tagged** writer
> (`internal/db/spike/fixture.go`, `//go:build bench`), so *"no writer"* now means **no production
> writer**. This clause's argument is untouched — the vocabulary is still unconstrained in the schema
> and a writer still costs no migration. See [ADR-0039](#adr-0039)'s 2026-08-19 amendment. [ADR-0039](#adr-0039) left its `state`
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
  ⚠️ **Amended 2026-08-19 — the assignment stands, its premise is half discharged.** `007e58e`
  landed the **declaration** (`internal/store/writequeue.go`), so *"nothing validates it today"* is no
  longer accurate about the vocabulary's Go home; it remains accurate about the **runtime**, there
  being no production writer. What this bullet assigns — the **calling** of the validator, owed by the
  first \*Arr adapter's writer — is unchanged and is now enforced by an AST guard rather than by
  prose. See [ADR-0039](#adr-0039)'s 2026-08-19 amendment.

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
else** — no scope moves, no design is reopened, and no ADR is reversed. · ⚠️ **amended 2026-08-18**
— see the block directly below: the order **inside** v0.2 is now fixed, which this ADR left as
*"first or last as convenient"*; membership is untouched.

### ⚠️ Amendment, 2026-08-18 — the order inside v0.2 is fixed, and it is a sequencing call, not a scope change

**This ADR set v0.2's membership and left its internal order loose.** Its Consequences named an
ordering — *"adapters and write path before the request flow; correction UI first or last"* — and its
`ARCHITECTURE.md` §16 v0.2 entry wrote the same thing down as *"the correction UI is independent and
comes first or last as convenient."* That *"first or last"* is the only thing this amendment closes.
Nothing enters or leaves v0.2; the three commitments this ADR slotted are exactly the three it slotted.

**The order is now:**

1. **The minimal match-correction UI first.** §16's v0.2 entry already records that it is gated on
   nothing — *"It depends on nothing else in v0.2 and nothing in v0.2 depends on it … Because nothing
   in v0.2 gates it, it is the part of this milestone that can land first."* Being unblocked, it goes
   first.
2. **The Navidrome adapter next.** Navidrome is already #1 in §16.1's post-v0.1 catalogue sequence,
   which §16.1 says is *"expected to interleave with v0.2."* The owner runs Navidrome, so it is
   provable against real data the moment it lands.
3. **Sonarr and Radarr with the minimal write path last.** The owner runs neither \*Arr (*"thats
   gonna have to be future"*, 2026-08-17, quoted in [ADR-0041](#adr-0041)), so there is **no real
   \*Arr data to prove them against yet**, and this project's rule — kept unchanged through every
   re-sequencing — is *"one source, proven on real data, before a second adapter"* (`CLAUDE.md`;
   §16.0). The request flow that routes through them (§8.3's `Add`) comes after them, exactly as this
   ADR's Consequences already said (*"the adapters and the write path come before the request flow
   that routes through them"*), so ordering the \*Arrs last orders the request flow last with them.

**This is a real dependency only between (3)'s halves, not across the three.** The \*Arr adapters and
write path must precede the request flow, which this ADR already stated. Between (1), (2) and (3) there
is **no technical dependency** — the order is chosen on *provability against real data*, not on a build
chain. The match UI acts on `library_override`, already in the tree; nothing it builds is needed by the
write path, and nothing the write path builds is needed by it (this ADR, decision clause 3, and §16's
v0.2 bullet 3). If a hard dependency between them is ever discovered, it would reopen this order, not
merely reshuffle it.

**Who decided.** The owner **delegated the milestone-and-sequencing call** and stated no preference —
verbatim in the PM thread, 2026-08-18: *"i genuinely dont care which milestone they go to. you can
decide whats best."* **That is a delegation, not an approval:** he supplied the authority to choose and
no choice of his own, so this order is **not owner-approved and must not be recorded as such** — it is
the PM's decision, taken under that delegation, and it stands on the repository's own logic above rather
than on the owner having endorsed it. This is the same care [ADR-0045](#adr-0045) took with the same
owner's *"whatever you think is best"*, and the precedent it names is real: a prior relay called a
delegation an approval and it had to be corrected.

**Where it is authoritative.** `ARCHITECTURE.md` §16 is scope authority, so the order becomes
authoritative in its v0.2 entry, which is amended in the same pass; this block is the decision record
that entry points back to. `docs/ROADMAP.md` carries the same order as narrative; membership is
identical across all three.

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
> ⚠️ **Amended 2026-08-19:** of the two things named here, the **declaration** shipped early, in
> v0.1, as `internal/store/writequeue.go` (`007e58e`); what remains **v0.2's** is the **validation
> call** at the first production writer, now guarded in `check` rather than owed in prose. The clause
> is otherwise unchanged, and it still costs no migration. See [ADR-0039](#adr-0039)'s 2026-08-19
> amendment.

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
  ⚠️ **Amended 2026-08-19:** the obligation **split** rather than moved. Its **declaration** half
  was discharged in v0.1 by `007e58e`; only the **validation call** is still v0.2's. See
  [ADR-0039](#adr-0039)'s 2026-08-19 amendment.

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
**Leaves [ADR-0035](#adr-0035) and [ADR-0041](#adr-0041) untouched.** · ⚠️ **Amended 2026-08-17** —
the decision stands unchanged for Kavita; what is corrected is its **reach**. See the block below.

> ⚠️ **AMENDED 2026-08-17: this decision stands unchanged FOR KAVITA. What is corrected is its
> reach — it is not a template, and open question 1 was wrong about why.** Nothing in the Decision,
> the Alternatives or the Consequences is reopened; no file, test or runtime behaviour moves.
>
> **The assumption it turned out to rest on, named plainly: that upstream regenerates its OpenAPI
> spec per release.** Every clause above needs a tag and a branch tip to be *two documents* — that
> is what makes two pins two points. **Kavita satisfies it** (462 paths at the tag against 488 on
> develop). **Prowlarr does not.** `src/Prowlarr.Api.V1/openapi.json` is the **same git blob** at
> tag `v2.5.2.5491` and at `develop` — `134d31d7df5e80714c454a6224e7449df512c55e`, which is also
> exactly what `api/specs/prowlarr.json` already holds; confirmed by `git ls-tree` at both refs on
> 2026-08-17. Upstream last regenerated it on **2025-06-07** (`60740fa25`, *"Automated API Docs
> update"*), 33 release tags back. Building the floor/ceiling structure there would vendor the same
> 145 KB twice under two names and print two subtest names over one document — **exactly the false
> green this ADR exists to abolish.**
>
> **So the remedy is chosen per upstream, not applied as a template.** *Floor and ceiling* is the
> answer where the two pins are two documents; where they are one document it is theatre, and the
> honest remedy is a different one. **Prowlarr is the counter-example**, and it has its own
> measurement and its own ADR from the thread that took it — **pointed at here, not retold.** That
> the staleness is real and not merely cosmetic is worth one line: `SearchResource.Limit`/`Offset`
> became `int?` upstream in PR #2654 (`c687bdb1f`, first shipped `v2.3.6.5351`, present on both
> lines) while the vendored document still declares them non-nullable `int32` — **no contract test
> over that document could ever have caught it, because the document never moved.** UsArr is
> independently correct (`internal/servarr/search.go` types both `*int32` and omits on nil), so
> there is no live bug; there is a test that cannot see.
>
> **And identity is by BLOB SHA where a spec declares a placeholder version.** This ADR tells its
> two files apart by `info.version` — 0.9.0.0 against 0.9.0.20 — and
> `TestBothSpecsAreTheDocumentsSOURCESSays` pins exactly that. Prowlarr's spec self-reports
> `info.version` **`"1.0.0"`**, the Swashbuckle placeholder: it carries **no** version information
> rather than stale information, so a version assertion there would assert nothing while looking
> like it asserted something. That is a **harder** case than Kavita's floor, which at least declares
> a stale-but-real 0.9.0.0. Where the document names no version, its identity is its blob SHA.

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

1. **`prowlarr.json` has the same shape of gap and is not fixed here.**
   ⚠️ **2026-08-17: the gap is real, the *shape* is not the same, and this paragraph's implied
   remedy does not apply.** Prowlarr's `openapi.json` is one blob at both refs, so there is no
   second document to pin — see the amendment block under `Status:`. The final sentence of this
   paragraph is the part that survives, and it survives more strongly than written.
   It tracks `develop` at v2.6.2
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

---

<a id="adr-0047"></a>
## ADR-0047 — Prowlarr's contract tests pin ONE spec, because floor and ceiling are the same git blob; the guard is an offline blob-identity pin plus a network drift check

**Status:** Accepted · 2026-08-17 · **Is the per-upstream remedy [ADR-0046](#adr-0046)'s own
2026-08-17 amendment points at** — that amendment (`cf5fab5`, review-log `LS-53`) records that
ADR-0046 stands for Kavita and is **not a template**, and defers the Prowlarr remedy to *"its own ADR
from the thread that took it"*; this is that ADR, and the reference runs both ways · **Answers the
first open question [ADR-0046](#adr-0046) raised and refused** (*"`prowlarr.json` has the same shape
of gap and is not fixed here"*) · **Corrects that question's own premise** — ADR-0046 described
`prowlarr.json` as *"tracks `develop` at v2.6.2 while the only known deployment runs stable
2.5.2.5491, a minor version behind"*; measured, it tracks neither by version and is byte-identical at
both refs, which the amendment now flags inline · **Changes what a green means; changes no runtime
behaviour** — no adapter field is added or removed, no request changes, no migration · **Adds no
vendored file** and renames none · **Leaves [ADR-0035](#adr-0035), [ADR-0041](#adr-0041) and
[ADR-0046](#adr-0046) untouched.**

### Context

**[ADR-0046](#adr-0046) gave Kavita a floor and a ceiling because Kavita regenerates its checked-in
`openapi.json` most releases, so the tag's copy and `develop`'s copy are genuinely different
documents.** The obvious next step was to port that two-spec split to Prowlarr, which ADR-0046's own
open question 1 flagged as owed. **It does not port, and the reason is measured, not assumed.**

Everything below was measured on 2026-08-17 against Prowlarr's git objects — via `git hash-object` on
the vendored file and a raw fetch of each ref through the proxy, hashed with `git hash-object` — and,
where the spec is silent, against Prowlarr's own C# source at a tag. No running instance was
required for any of it.

| Question | Answer, measured 2026-08-17 |
| --- | --- |
| Blob of `src/Prowlarr.Api.V1/openapi.json` at tag `v2.5.2.5491`? | `134d31d7df5e80714c454a6224e7449df512c55e` |
| …at `develop` (`1f7db1e`)? | `134d31d7df5e80714c454a6224e7449df512c55e` — **the same git blob** |
| …and `api/specs/prowlarr.json`? | the same blob a third time |
| When was it last regenerated upstream? | **2025-06-07**, commit `60740fa25` (*"Automated API Docs update"*) |
| Release tags shipped since (`git tag --contains 60740fa25`)? | **33**, none pointing at it |
| Highest released tag as of today? | `v2.6.1.5509` |
| What does the file say its version is? | `info.version` **`1.0.0`** — **Swashbuckle's placeholder, carrying no release information whatsoever, so nobody pins to it** |

**Floor and ceiling are one file. Splitting it would vendor 145 KB twice and produce a green that
proves exactly what one copy already proves.** So Prowlarr is vendored ONCE — and that is a decision,
not the omission ADR-0046's open question read it as.

**The honest consequence is worse than "one file, not two": the one file describes NEITHER ref
reliably.** Regenerated ten months and 33 releases ago, it is evidence about the API's *shape*, and
**Prowlarr's source at a tag is the evidence about a release**. This is not hypothetical — a grab
failure came from exactly this gap in another form, where a test fake was written from the spec and
inherited the spec's silence, validating nothing.

**It already misdescribes both refs, and the divergence is machine-checked rather than trusted to a
comment.** `SearchResource.Limit` and `.Offset` were `int` and became `int?` in `c687bdb1f` (*"Fixed:
Don't send limit=0 to Newznab indexers"*, PR #2654), a commit that tag `v2.3.6.5351` sits directly
on — so nullable on the owner's `v2.5.2.5491` and on `develop` alike. The vendored spec, regenerated
before that commit, still declares them non-nullable `int32`. UsArr's own code is **independently
correct** — `SearchRequest.{Limit,Offset}` are `*int32` and `Values()` omits them when nil, matching
upstream's `Limit is > 0` guard — so this is a documented divergence, not a live bug.

**The floor version, `v2.5.2.5491`, is owner-confirmed 2026-08-17** — the owner stated it directly,
in his own words, of his actual install, so the floor is checked against a human's box rather than
inferred from a changelog. That distinction is worth keeping visible: the next person to touch this
pin should know it was confirmed against a real deployment. It is still not the same thing as UsArr
reading `/api/v1/system/status` itself, so it bounds what a green proves — a green here is evidence
about a document, and a real instance is the only evidence about a real instance. The floor is
labelled everywhere it appears (SOURCES.md, `contract_test.go`, `specdrift_upstream_test.go`), and
because the owner's Prowlarr auto-updates it is kept as a one-line pin so re-confirming it later is a
one-line change.

### Decision

**Vendor one spec, and split the guard across the network line into a deterministic offline pin in
the gate and a network drift check outside it.**

1. **One vendored `api/specs/prowlarr.json`.** SOURCES.md and `contract_test.go` say, in words, that
   it describes neither the owner's release nor `develop`, and why (Prowlarr regenerates rarely). It
   is treated as evidence about the API's shape; the source at a tag is the evidence about a release.
2. **`TestVendoredSpecIsThePinnedBlob` — offline, IN `make check`.** It pins the git blob name
   `134d31d7…` as a constant and asserts the vendored bytes hash to it, reimplementing git's
   `blob <len>\0` + SHA-1 so it needs no git checkout and no network. It catches the vendored file
   changing under the suite — a re-vendor, a hand-edit, a bad merge — deterministically. **No third
   network call is added to the gate**, which makes exactly two (both vulnerability databases) and
   whose `check-offline` is defined as those two removed.
3. **`TestSpecDriftRefsStillShareThePinnedBlob` — network, OUTSIDE `make check`.** Behind the
   `//go:build upstream` tag and `make spec-drift`, refusing without `USARR_SPEC_DRIFT=1`, exactly as
   `make test-integration` refuses without `USARR_INTEGRATION=1`. It resolves both refs to their blob
   name with a blobless `git fetch` (trees, no file contents) and fails the day upstream regenerates.
   A failure there is **news, not a broken build**: an upstream outage, a rate limit or a plane must
   never redden a commit that touched no spec, which is precisely why it is not in the gate.
4. **`knownSpecDivergences` + `TestKnownSpecDivergencesStillHold` — offline, IN `make check`.** The
   `Limit`/`Offset` nullability gap is written out by hand against upstream source and machine-checked
   in the only direction a single stale file can be: the divergence must **still be there**. An entry
   that stops holding means upstream finally regenerated, which is news — and news is what a comment
   cannot deliver, because a comment that has gone false looks exactly like one that has not.
5. **Every failure message clears the 2am bar with three named outcomes.** A reader must be able to
   tell from the message alone whether **their Prowlarr is too old**, **our expectation is too new**,
   or **the vendored document is simply stale** — three different fixes. `specSkewAdvice` names all
   three; the drift check's message names its own three (local mistake, upstream regenerated, one ref
   moved). Shortening either to one line restores the problem it exists to solve.

**No runtime behaviour changes.** `SearchRequest`'s pointers, `Values()`'s omit-when-nil and the
`Limit is > 0` guard all stay exactly as they were; what changes is that the tree now *says* the
vendored spec is stale and *proves* the divergence is still live, instead of implying the contract
tests attest something about the server the owner runs.

### Why ADR-0046's pattern does not port

ADR-0046 turned on a property Prowlarr lacks: **that the tag and `develop` carry different
documents.** Kavita regenerates its spec most releases; Prowlarr has not regenerated its since
2025-06-07. Where Kavita's `ceilingOnlyProperties` can be *recomputed* from two files, Prowlarr's
delta cannot — there is only one file — so it is written against source and checked for persistence
instead. The floor/ceiling split here is **vacuous, not impossible** — that wording overstated it,
and the Context section above puts it correctly. Nothing prevents vendoring the same blob under two
filenames; what it would buy is nothing, because the second copy proves exactly what the first
already proves while costing another 145 KB and a second file to keep in step. When `make spec-drift`
fires, that premise has changed and the split becomes worth revisiting — which is the whole reason
that check exists.

**Two threads reached this independently, which is corroboration rather than a shared assumption.**
The library thread that owns ADR-0046 measured the same blob identity from its own side (`git ls-tree`
at both refs) and amended ADR-0046 to say the pattern is not a template (`cf5fab5`, `LS-53`); this
thread measured it with `git hash-object` on a raw fetch of each ref and pins the result in code. The
same fact arrived twice by two methods, and each ADR points at the other so neither reference dangles.

### The deciding question

**What a green means to a reader who has not read this ADR.** One vendored spec, a blob pinned in the
gate, a drift check kept out of it, and three-outcome failure messages are all chosen against the
same test ADR-0046 set: a green must not be mistaken for evidence it does not carry, and a red must
tell its reader which of three different things to do.

### Open questions this ADR does not close

1. **The floor drifts when the owner's box does.** `v2.5.2.5491` is owner-confirmed 2026-08-17, but
   his Prowlarr auto-updates, so the confirmation has a shelf life. Nothing here re-confirms it; the
   pin is deliberately one line so doing so later is one line. (What this ADR does *not* claim is that
   UsArr read the version off `/api/v1/system/status` — it did not; the floor is a human's word about
   a real install, which is stronger than a changelog and weaker than a live read.)
2. **`make spec-drift` is unautomated.** It is a target a person or an agent must type; this repo has
   no CI to run it on a schedule, and SOURCES.md says as much. The seam is a build tag and a target,
   which is all a future CI would need.

---

<a id="adr-0048"></a>
## ADR-0048 — A library proposal is not a row in `library`; a row is created only on Accept

**Status:** Accepted · 2026-08-17 · **Refines [ADR-0026](#adr-0026)** — its binding model, its
single-kind rule, its four verbs and its four tables are untouched; what is decided here is *when a
`library` row comes into existence*, which ADR-0026 did not say · **Applies [ADR-0004](#adr-0004)
rather than excepting it** — the connect probe is a setup action, not a render path · **Answers an
open decision the tree already records** in `web/src/routes/libraries/+page.svelte` (`78660a4`),
which named three candidates and deliberately picked none · **Closes off a third `managed_by` state,
and any `proposed` flag on `library`** · **Costs no migration, no data change and no new state** ·
**Describes behaviour that is not built**, on two counts named in the Decision, and it says so rather
than implying it · **Removes nothing from the import path here** — that removal belongs to the
library thread that builds §17.8.

### Context

**`ARCHITECTURE.md` §17.8 specifies a proposal step and does not say where a proposal lives.** On a
successful connect and capability probe, UsArr *proposes* libraries as one pre-checked Accept step,
one per container the connected service itself reports, each editable inline, with a stated
case-insensitive whitespace-trimmed merge key and a one-way door on editing. §17.8 says all of that
about the user's experience and nothing about storage, and the gap is the kind that gets filled by
whoever implements it first.

**It was already filled, silently, and the tree has since said so.** `web/src/routes/libraries/+page.svelte`
carried a comment reasoning that the schema was ready for the Accept step; another thread corrected
it at **`78660a4`** — comment-only, 25 insertions, 5 deletions — and the corrected text is the
starting point for this ADR rather than a footnote to it. Three findings, in its words:

1. **The column cannot express what the flow needs.** `managed_by` has *"exactly TWO values, and
   NEITHER means 'proposed'"*, so *"a library the user accepted and one they have never been shown
   are therefore the same row, indistinguishable."*
2. **The row already exists at proposal time, which inverts the screen's safety.** *"If a proposal is
   already a row, then Accept is a no-op and Decline is a DELETE — backwards from what a pre-checked
   confirmation screen implies, where saying no is supposed to be the cheap direction."*
3. **It is an open decision with three candidates and no pick** — *"a proposal stops being a row
   until it is accepted; or a third `managed_by` state; or §17.8 is renamed to a review of what has
   already been created"* — and *"until one is picked, the Accept step is not buildable on this
   schema."*

**This ADR takes the first and rejects the other two.** Finding 2 is the sharpest argument for it and
it was measured rather than reasoned: a confirmation screen whose *No* is the destructive direction is
not a confirmation screen.

#### The precedent: `library` already holds one row that is not a library

Migration `00005_library_sync.sql` seeds a reserved row at `id = 0`, `Unfiled`
(`internal/db/migrations/00005_library_sync.sql:597-598`). Its own comment states what it is for and
what it is not: it is the landing place the membership derivation needs so that a work bound to no
other library still matches a scope, and it is *"never listed on the Libraries screen, never offered
in the scope chip, never proposed"* (same file, comment block at lines 579-596). It is protected by a
`BEFORE DELETE` trigger (lines 625-632), which exists because *"reserved" was previously a comment and
nothing else*.

**And the way the codebase keeps it out of user-facing results is a hand-written identity comparison,
not a predicate meaning "not a real library".** `internal/store/catalogue.go` declares
`UnfiledLibraryID int64 = 0` (line 41) and excludes it in Go, inside the scan loop:

```go
if r.id != UnfiledLibraryID {
	out.joinable[libraryNameKey(name)] = r
}
```

— `internal/store/catalogue.go:546`, in `userLibraries`, the name-collision lookup the binding uses to
decide whether to join an existing library or create a new one. The same constant is used in the
other direction at line 1163, where `rebuildSearchDoc` files a stranded document *into* library 0.

⚠️ **An earlier draft of this ADR put that exclusion in the SQL, and it is not there.** It cited
`SELECT id, name, kind FROM library WHERE user_id = ? AND id <> ?` at `:395`, in a function called
`userLibrariesByNameKey`. **The query, the line and the function name are all stale**: the statement
is now `SELECT id, name, slug, kind FROM library WHERE user_id = ?` (`:532`) with no exclusion in it
at all, and the reserved row is dropped afterwards, in Go. The correction **strengthens** the
argument rather than weakening it, on two counts. First, an exclusion that is an `if` inside a scan
loop is even less likely to be inherited by the next author's query than one written into a `WHERE`
clause, because it is not in the query to copy. Second, the one site shows the reserved row costs
**two** decisions rather than one: `out.names` deliberately KEEPS `Unfiled`, so no new library can
take the name, while `out.joinable` drops it, so no proposal can join it. A second not-really-a-library
class would have to answer both questions, and nothing in the schema asks either.

⚠️ **The exact size of that precedent is stated rather than inflated, because it is smaller than the
argument wants it to be and saying so is the point.** On the tree read there is **exactly one**
exclusion of library 0 in non-test Go — line 546 — plus one in `internal/store/catalogue_test.go:437`,
two in `internal/libsync/importer_test.go:358` and `:380`, and two end-to-end in
`cmd/usarr/import_e2e_test.go:190` (which excludes it *and* filters `managed_by = 'auto'`) and `:193`.
**There is only one because there is almost nothing that reads the table** — see the Decision's second
unbuilt fact. The claim that a second not-really-a-library class would cost a second hand-written
exclusion **at every future read site** is therefore *inference*, not measurement: it is the pattern
the one existing site demonstrates, applied to sites that do not exist yet. What is measured is the
pattern, and the pattern is exclusion by identity comparison.

**The failure mode is silent, which is what makes the inference worth acting on.** Search scope is
`search_doc_library` membership (§7 invariant 5, quoted in the migration's comment above the reserved
row); an availability rollup is §6.3's. Neither has a natural place to learn about a second excluded
class. A proposal nobody accepted appearing in a search scope or a rollup does not throw, does not log
and does not render an error: it renders a number that is wrong.

### Decision

> **A library proposal lives in the connect probe's response. It is never persisted. A `library` row
> is created only when the user accepts one.**

Five clauses, each with what it rests on:

1. **The proposal set is a value computed by the probe, not a table.** It is a function of two inputs
   — what the connected instance reports as its containers, and what is already bound in
   `library_source` for that user — recomputed on every probe. There is nothing to migrate, nothing
   to garbage-collect and nothing that can be stale, because a proposal that is a function of its
   inputs is current by construction. **A persisted proposal has the opposite property**: rename or
   delete an upstream library between proposal and acceptance and the stored row now describes
   something that is not there, with no event to tell it. *(Inference from the two inputs. UsArr has
   no container-level change feed from Kavita: `ARCHITECTURE.md` §7.1a and [ADR-0035](#adr-0035) §2a
   establish a usable item delta and no changed-since endpoint, and neither covers a library being
   renamed out from under a stored proposal.)*

2. **This makes Decline the cheap direction again.** With no row until Accept, declining a proposal
   writes nothing and accepting it writes once — which is the order a pre-checked confirmation screen
   implies. It is the direct answer to the inversion `78660a4` recorded, and it is the clause that
   makes §17.8's Accept step buildable at all.

3. **`managed_by` keeps exactly two states, and no `proposed` flag is added. ⚠️ Not because the
   column can record an acceptance — it cannot, and this ADR must not be read as claiming it can.**
   The column is
   `TEXT NOT NULL DEFAULT 'auto' CHECK (managed_by IN ('auto','user'))`
   (`internal/db/migrations/00005_library_sync.sql:565`), and its comment on the three lines below
   defines *"'auto' = created by the proposal flow and still tracking its source; 'user' = the user
   edited it, so the proposal flow never rewrites it again."* Both halves describe a library that
   exists. **Neither distinguishes an accepted library from one the user has never been shown**,
   exactly as `78660a4` found. **What clause 1 changes is that the distinction stops needing to be
   recorded:** once no row exists before Accept, every row is an accepted row by construction, and a
   column that cannot express "proposed" is not being asked to. The schema is adequate because the
   state was removed, not because the state was already covered.

4. **Existing `managed_by = 'auto'` rows are DECLARED accepted on upgrade.** This is a decision, not
   a reading — the column cannot tell the applying agent which they are, per clause 3. It is the
   right decision because it matches the only history those rows can have: the sole Go writer is one
   `INSERT` with the literal `'auto'` (`internal/store/catalogue.go:487-489`), there is no
   `UPDATE library` anywhere in non-test Go, so no row has ever left the state it was inserted in, and
   every one of them was created by the binding on a first connect — the proposal flow's own path.
   **No migration file, no `UPDATE`, no backfill.** The honest cost is stated rather than hidden: an
   install that auto-created a library the owner would have declined keeps it, and the remedy is the
   Libraries screen's own delete once §17.8 ships.

5. **The connect probe may call upstream, and that is not an exception to replica-not-proxy. This is
   stated here so nobody flags it later.** `CLAUDE.md` principle 1 and `ARCHITECTURE.md` §2 bind
   *"every user-facing read"* and *"no browser request ever awaits an outbound call"*;
   [ADR-0004](#adr-0004) enforces it as *"the `api` package imports `store`, never `provider`"*. **A
   connect probe is not a read of the library; it is the act of configuring one**, and §17.8 already
   draws this line for the neighbouring case in its own words: the Requests panel's quality profile,
   root folder and tags are fetched live *"when the panel opens — a settings screen may block on an
   upstream call, a render path may not"*. The proposal is the same shape. It needs no argument to
   survive review, and it gets none here beyond naming the rule it is already inside.

#### Two facts about what is not built, stated plainly

**Neither is implied, softened or left to be inferred, because this ADR specifies a flow whose two
halves are in opposite states: one is unwritten, the other is written and wrong for this design.**

**Fact 1 — `managed_by = 'user'` has never been written by any code path, so §17.8's one-way door is
specified and unimplemented.** §17.8 states it as permanent helper text: *"Editing any proposal marks
that library user-managed. After that, a later connect can only offer to add sources. It can never
reshape the library."* **Nothing implements it.** Measured by grep over the tree: `library.managed_by`
has **one** writer in Go — the `INSERT` at `internal/store/catalogue.go:487-489`, writing the literal
`'auto'` — and **zero** readers in non-test Go. The only other writer of the column anywhere is the
seed `INSERT` for library 0 at `00005_library_sync.sql:597`, also `'auto'`. The only readers are two
tests (`internal/db/migrate_test.go:909`, asserting library 0 is `'auto'`, and
`cmd/usarr/import_e2e_test.go:190`, counting `'auto'` rows). **No code path has ever written
`'user'`** — that, and not a count of occurrences, is the claim this argument needs. ⚠️ **An earlier
draft of this ADR said `'user'` "appears in the tree in exactly one place: the `CHECK` constraint
that permits it", and that was false.** The literal also appears in the migration's own adjoining
comment (`00005_library_sync.sql:567`) and twice in the generated schema mirror
(`internal/db/testdata/schema.sql:373` and `:375`), besides `docs/reference/schema.md:1458,1460` and
the `+page.svelte` comment block. None of those is a writer, which is why the corrected claim is about
writes rather than about occurrences. The door is a design commitment with no hinge.

**Fact 2 — the import creates rows unconditionally and silently, so Accept is a removal, and the
removal is not done here.** The path is: a Kavita client stack is built for an instance →
`bootstrapImport` fires once, gated on `last_full_sync_at` being unset (`cmd/usarr/services.go:219-221`,
`cmd/usarr/import.go:102-111`) → `FullImport` → `store.BindContainers` → `bindOneContainer`, which
joins an existing library on the name key or **creates one** (`internal/store/catalogue.go:488`).
There is no prompt, no confirmation and no screen; `internal/libsync`'s report counts
`LibrariesCreated` and `LibrariesJoined` into a log line. **So Accept is not a new screen bolted onto
an existing flow — it is the removal of creation from `bindOneContainer`'s reach on the bootstrap
path**, a change to code that works today. ⚠️ **This ADR does not make that change and does not
schedule it.** It belongs to the library thread that builds §17.8, together with the first open
question below; what this ADR fixes is which of `78660a4`'s three candidates that thread implements.

### The deciding question

**Which class of row can be added to `library` without a reader having to know about it?** Every
answer to *"should a proposal be a row"* reduces to that, because the cost of a not-really-a-library
row is not paid at insert. It is paid at every future `SELECT` by an author who did not write the
insert, and library 0 is the tree's own demonstration of what that costs: a reserved row, a comment
saying it is reserved, a trigger added later because *the comment was not enough*, and an identity
comparison every new read site has to remember. **One such class is a documented cost. Two is a
convention, and a convention nobody wrote down is how a proposal ends up in an availability rollup.**

### Alternatives considered

**(a) A third `managed_by` state — `'proposed'`.** This is `78660a4`'s second candidate. Rejected, and
it is the strongest of the alternatives because it is *cheap and honest about the lifecycle*. The
state machine `proposed → auto → user` is exactly the sequence §17.8 describes, in the column §17.8's
own comment was written for; a proposal is genuinely a stage in a library's life rather than a
different kind of object, and modelling a lifecycle as a state on the row is ordinary. It would also
give a half-finished connect a durable place to sit: a user who closes the tab mid-wizard could come
back to the same proposals. **What defeats it is that the state is not free where the rows are read,
and the column is not where the reads are.** `library` is filtered on `user_id`, `kind`, `enabled` and
`include_in_search`, and the last two are `INTEGER NOT NULL DEFAULT 1`
(`00005_library_sync.sql:552-553`), under `ux_library_slug`, `ux_library_name` and
`ix_library_kind … WHERE enabled = 1` (`:576-578`). A `'proposed'` row satisfies `enabled = 1` and
`include_in_search = 1` on its defaults and sits in `ix_library_kind` like any other, so **every read
that does not name `managed_by` sees it** — and no read names `managed_by`, because there are zero
readers. The second cost is the unique indexes: `ux_library_name` and `ux_library_slug` are per user
over all rows, so **a proposal would reserve a name before the user accepted it**, which breaks
§17.8's stated merge behaviour — typing an existing name into a proposal is specified to *join* that
library, and it cannot join a name its own unaccepted row is holding. Third, it does not fix the
inversion: Decline still deletes a row. None of these says the state machine is wrong; all three say
the row is the wrong place to keep it.

**(b) A `proposed` boolean, or a nullable `accepted_at`, on `library`.** Rejected on everything in
(a), and worse on the one axis where (a) was better: a boolean carries no lifecycle, so the
`managed_by` question returns the moment a proposal is edited. It also needs a migration, which (a)
and the chosen answer do not.

**(c) A separate `library_proposal` table.** Rejected. It keeps `library` clean, which is the real
objection to (a) and (b), and it would be right if proposals had to survive a restart. **They do not,
and paying a migration, a table, a cleanup rule and a staleness problem for data whose whole content
is recomputable from the probe is buying durability for something that is worth less when it is
durable** — a stored proposal can describe a container the upstream has since renamed or deleted; a
recomputed one cannot. `CLAUDE.md`'s *"cut before you add"* arrives at the same place from the other
direction: this adds a table and removes nothing.

**(d) Rename §17.8's step to a review of what has already been created.** This is `78660a4`'s third
candidate, and it deserves better than the summary dismissal it could get, because **it is the only
alternative that describes what the code does today and it costs nothing to build.** Its argument is
real: the first import already produces a catalogue without the user finding a button, which
`cmd/usarr/import.go`'s own comment names as the point of the on-connect trigger; a review screen is
honest about that, needs no removal from a working path, and keeps a first connect useful even if the
user never opens Libraries. **It is rejected on two grounds.** First, it does not repair the
inversion — it *ratifies* it. Every row on that screen exists already, so the only actions available
are keep and delete, and a user who wanted none of them has to delete each one; §17.8's pre-checked
Accept, whose whole safety property is that saying no is free, would be a screen that cannot be
implemented as specified and would have to be re-specified around the storage rather than the other
way round. Second, it writes before consent as a matter of design rather than as an accident, and the
rows it writes are indistinguishable from accepted ones forever, per clause 3 — so the product would
have no way, at any later date, to tell a library the user chose from one that appeared. **This
alternative is the one to revisit if clause 1 turns out to be expensive to build**, and revisiting it
means re-specifying §17.8's step, not just renaming it.

**(e) Leave it unspecified and let the implementation choose.** Rejected — it is what the tree already
did, and the result is Fact 2 plus a comment block that had to be corrected. The import creates rows
because nothing said it should not, and the cost of reversing that is now a change to a working path
rather than a line in a spec.

### Consequences

* **`library` keeps exactly one class of row that is not a library.** The identity comparison at
  `internal/store/catalogue.go:546` stays the only exclusion of its kind, and the next read site has
  one thing to remember rather than two.
* **§17.8's Accept step becomes buildable**, which `78660a4` recorded that it was not. That comment
  block is corrected in the same commit as this ADR, because leaving it would put both claims on
  `main` at once — it was a correct record of an open decision as of its date, and the decision
  closing is what changes.
* **No migration, no data change, no new state** — but see clause 3: the reason is that the state was
  removed from the design, not that the column already covered it. The `CHECK` and its comment are
  unchanged, and a merged migration is never edited anyway (`CLAUDE.md`, Conventions).
* **Implementing Accept is a subtraction from a working path, and it is the harder half.** Removing
  unconditional creation from the bootstrap import means a first connect no longer produces a
  catalogue on its own — the behaviour `cmd/usarr/import.go` describes as the point of the on-connect
  trigger: *"the user adds a Kavita, and a catalogue appears without them having to find a button"*.
  **That trade is named here and not decided here.** It is the library thread's, at §17.8 build time.
* **A proposal cannot be resumed across a restart**, by construction. Closing the wizard discards the
  proposal set and the next probe recomputes it. The loss is real and small: the recomputed set is the
  same set unless the upstream changed, in which case the recomputed one is correct and the resumed
  one would have been wrong.
* **§17.8's one-way door remains unimplemented and is now recorded as such** rather than reading as a
  description of behaviour. The §17.8 rewrite in this same commit carries that statement in the
  section itself.
* **This ADR does not settle LS-06.** `internal/store/catalogue.go`'s `BindContainers` comment states
  that its `container_kind = 'remote_library'` decision *"IS OWED AN ADR AND DOES NOT HAVE ONE"*
  (`:307-314`, recorded as `LS-06` in [`REVIEW-LOG.md`](./REVIEW-LOG.md)). That is a different
  decision — *which container a source binds to* — and it stays owed.

### 🚩 Open questions this ADR raises and does not answer

1. **Whether the first import runs before Accept or after it.** Clause 1 makes creation conditional;
   what replaces the bootstrap's current behaviour is not decided here and belongs to the library
   thread. Two shapes are live — probe, propose, accept, then import; or import into a holding state
   and confirm afterwards — and the second reintroduces exactly the row-that-is-not-yet-a-library this
   ADR rejects, so it is not free. It needs the owner.
2. **What the probe proposes on an install that already has bound libraries.** §17.8's rule is that a
   later connect *"can only offer to add sources"* for a user-managed library, but every existing row
   is `'auto'` and no code has ever read the column to tell the two apart. The rule is well-specified
   and has never been exercised.
3. **Whether a declined container should be remembered.** `BindContainers` skips a container whose
   kind UsArr has no `work.kind` for and returns it to the caller to report — the rule is stated at
   `internal/store/catalogue.go:315-317` and executed at `:344-346` (`if c.Kind == "" { continue }`),
   with the downstream skip recorded at `:640`; §17.8 requires it be *declined with a reason* rather
   than silently dropped, in a `Decision` column. If a decline is not persisted either, it is
   recomputed on every probe and the user re-declines it every connect. That is a proposal-lifetime
   question this ADR's answer makes sharper without answering.

---

<a id="adr-0049"></a>
## ADR-0049 — Key ids are derived from the key material; there is no counter and no settings row

**Status:** Accepted · 2026-08-19 · **Enables `usarr key rotate`** (`docs/ROADMAP.md`,
`reference/security.md` §1.5, `CONFIGURATION.md` §3.4) rather than being asked for by it ·
**Closes off two alternatives**: a monotonic counter, and a key-id row in a settings table ·
**Costs no migration** — `service_instance.kek_id` is already `INTEGER` and already carries the id ·
**Does not touch `internal/crypto/derive.go`'s five frozen HKDF labels**, and adds none · **Publishes
a 32-bit hash of the KEK in every stored row**, which is analysed below and accepted.

### Context

Every stored envelope has carried its wrapping key's id since the first commit
(`reference/security.md` §1.1: `kek_id (uint32 BE) || nonce || wrapped_dek || ciphertext || tag`),
and `service_instance.kek_id` duplicates it as a plain column so a rotation can find the remaining
work without opening anything. Both were built for rotation. Neither said **where the number comes
from**.

What the tree actually did was hard-code it: `cmd/usarr/app.go` called `crypto.NewKeyring(1, kek)`,
so every row ever sealed says `1`, and **nothing anywhere persisted which key material the id `1`
corresponds to**. That is fine while there is exactly one key forever. It stops being fine the moment
there are two, which is the entire content of a rotation.

### The decision

**`crypto.KeyID(kek)` — the first four bytes, big endian, of
`sha256("usarr/kek-id/v1" || kek)`, forced nonzero — is the id of a key.** A key file therefore names
its own id, and startup registers the key derived from `keys/secret.key` under both `KeyID(kek)` and
the legacy id `1`, primary on the former.

Three consequences, in the order they matter:

1. **There is no second piece of state to keep consistent across a crash.** The rotation's whole
   safety argument is that the SQLite transaction and the key-file write are not one atomic unit
   (§1.5). Anything that stores "the current key id" separately from the key sits in exactly that
   window: promote the file and die before the counter is written, and every row names an id nothing
   holds; write the counter and die before the file, and the id names material that no longer exists.
   A derived id cannot disagree with the key, because it *is* the key, hashed.
2. **An interrupted rotation is fully readable from the two files alone.** `keys/secret.key` and
   `keys/secret.key.new` are the keyring. Startup derives both ids, registers both keys, and every
   row opens — with no bookkeeping, no recovery step, and nothing to reconcile.
3. **The legacy id keeps every existing row opening with no migration.** The same key registers at
   `1` as a decrypt-only entry. New rows seal under the derived id; a rotation moves the old ones off
   `1` like any other id, and the first rotation on an existing install retires it for good.

`0` is never a real id: it is the placeholder `CreateServiceInstanceSealed` writes into `kek_id`
between its insert and its seal. When the hash lands on `0` — one run in 2^32 — the id becomes `1`,
the id the same key is registered at anyway, so nothing special-cases it.

### The alternatives, and why they lose

**A monotonic counter (`1`, `2`, `3`, …), persisted somewhere.** The obvious design, and it is the
one that fails at the exact moment rotation exists to survive. It needs a home — a settings table, a
sidecar file, a line inside the key file — and every home reintroduces the two-artifact consistency
problem in clause 1. It also needs a reader on the startup path that must never disagree with what
the key files hold, which is a second ladder to keep in step with the one in `config.ResolveMasterKey`.
Nothing about a counter is *wrong*; it just buys ordering, which nothing here needs, at the price of
the one property that matters.

**A `key_id` row in a settings table.** Worse than the counter on the same axis and worse again on
another: the database is the thing being rotated. Deciding which key opens the database from a row
*inside* the database is fine only while the row is plaintext and the schema is reachable — which it
is today — but it puts key identity behind migrations, backups and restores. A restored database with
a stale settings row would claim material that the config volume no longer holds, and the failure
would be a decrypt error rather than a named refusal.

**Keeping the hard-coded `1` and rotating "in place".** Rejected outright: two different keys both
claiming id `1` is precisely the state AEAD cannot distinguish from corruption, which is the argument
`crypto.Keyring`'s own doc comment already makes.

### What this publishes, and why it is nothing

**Every stored row now contains a 32-bit hash of the KEK**, in `service_instance.kek_id` and in the
envelope header. An offline attacker holding a database file can compute `KeyID` of any candidate key
and compare.

This grants them nothing they did not already have. RFC 3394 key-wrap **already** gives an exact,
per-row oracle for the same question: unwrap the row's DEK under a candidate KEK and the integrity
check tells you, without ambiguity, whether the candidate is right. That check is exact where this one
is 32 bits wide, it is available on every row, and it has been there since the first commit. A
candidate filter that is strictly weaker than a test the attacker can already run adds no capability.
What it costs the attacker is one SHA-256 instead of one AES key-unwrap per guess — and the guessing
itself is bounded by the KEK being HKDF output over 32 bytes of `crypto/rand`, which is not
brute-forceable at any hash speed.

The id is **not** secret and is not treated as one: it is printed by `usarr key rotate`, logged at
startup (`cmd/usarr/app.go`, the `keyring ready` line, with the whole active set beside it), and
written into audit metadata. It must never become an authentication input. ⚠️ **The startup half of
that sentence was aspirational when this ADR landed** and was made true afterwards — until then the
id reached a log only on the interrupted-rotation path, which is the one start where it is *least*
representative. See `REVIEW-LOG.md` RK-06.

### Consequences

* **`crypto.KeyID` is now load-bearing and its definition is frozen in the same sense the HKDF labels
  are.** Changing the constant, the digest or the truncation makes every stored row name an id the
  keyring no longer derives. It lives in `internal/crypto/keyid.go` rather than beside the HKDF
  labels in `derive.go` precisely so the two categories are not confused: the labels are bound into
  ciphertext as domain-separation inputs, this one produces a public identifier.
* **Key ids are no longer small integers, and operator-facing output shows them.** `1` became
  `4205839355`. Anything that assumed a low id — a hand-written test fixture, a log grep — is wrong,
  and `service_instance.kek_id` is `INTEGER`, so nothing in the schema notices.
* **`LegacyKEKID = 1` is permanent.** It is registered on every start for as long as any install can
  still hold a row sealed before this change, which is forever in the absence of a migration that
  rewrites them. It costs one map entry.
* **A rotation's re-wrap loop terminates on `kek_id <> new`, not `kek_id = old`.** With derived ids a
  row can legitimately sit at a *third* id — an earlier interrupted attempt, a row restored from an
  older backup — and "not at the new id" counts it as work instead of walking past it.

---

<a id="adr-0050"></a>
## ADR-0050 — The image pipeline's base output format is **stdlib JPEG**; AVIF is deferred with its seam kept

**Status:** Accepted · **2026-08-19** · **Amends** [`ARCHITECTURE.md`](./ARCHITECTURE.md) §4.4 and
§4.4.1, which named AVIF as the only output codec and named **no base format at all** · **Applies**
[ADR-0039](#adr-0039)'s no-`CHECK` reasoning to a second column, and **discharges in code** the
Go-validation obligation ADR-0039 promised and never wrote — ⚠️ **accurate on the day this ADR
landed, and superseded in one word since**: ADR-0039's obligation was itself discharged later the
same day, 2026-08-19, by `007e58e`, so *"never wrote"* should now read *"had not yet written"*. **This
ADR's argument is untouched** and is strengthened: shipping the validation **with** the column is what
kept `image_asset` from needing a catch-up commit at all. See [ADR-0039](#adr-0039)'s 2026-08-19
amendment · Lands with migration
`00008_image_asset_format.sql`, whose header carries the schema half of the reasoning next to the
SQL.

### Context

**§4.4 specified an output codec and never specified a base format. That is the gap, and it is a
real defect in a spec rather than a wording nit.** The section says images are *"AVIF encoded lazily
off the request path"* and mentions no other format anywhere — not as a fallback, not as what the
lazy encode is lazy *relative to*, not as what a row holds before the AVIF backfill reaches it.
§4.4.1's cold-start plan then says *"defer larger widths and AVIF"*, which only parses if something
non-AVIF exists at 92px, and never says what. So the pipeline as specified had:

- no declared encoding for the bytes it stores before the AVIF pass runs;
- no answer for what `/img` puts in a `Content-Type`;
- no column to record an encoding in — `image_asset` (migration `00005_library_sync.sql`) carries
  `width`, `height`, `thumbhash`, `dominant_color`, `etag`, `state` and no format or mime column.

**A spec that names a lazy optimisation without naming what it optimises *from* is not
under-specified at the margin; it is missing its base case.** This ADR is owed because of that, and
saying so plainly is the point — the alternative was to quietly pick a format in an implementation
PR and leave §4.4 reading as though it had always been complete.

**Serving upstream bytes untouched is not available as an escape hatch.** §4.4 mandates ingest-time
downscale to a seven-width allowlist (`92, 154, 200, 342, 500, 780, orig`) because arbitrary `?w=`
is a cache-poisoning DoS (GHSA-rrr6-mvwg-9pg9). Downscaling is decode-and-re-encode. So UsArr
*already* owns an encoder on six of the seven widths, and the only open question was which one.
**The codec is negotiable; the re-encode is not.** ⚠️ The seventh width, `orig`, is the one §4.4
leaves open — it is in the allowlist but it is not a downscale, so nothing in the spec said whether
its bytes are UsArr's or the upstream's. **Decision 1 closes that**, because the column is one per
row and cannot describe two codecs behind one asset.

**AVIF is buildable here, so this is a trade and not a limitation.** `gen2brain/avif` v0.6.0 is MIT
and cgo-free: per its own README it is *"based on libavif with dav1d (decode) and aom (encode)
compiled to WASM and used with wazero runtime (CGo-free)"*, trying a shared library through `purego`
first and falling back to the WASM build ([pkg.go.dev](https://pkg.go.dev/github.com/gen2brain/avif),
read 2026-08-19). The `CGO_ENABLED=0` single-static-binary constraint does **not** rule AVIF out, and
any argument that starts by claiming it does is wrong.

### Decision

> **1. The image pipeline's base output format is JPEG, encoded with the standard library's
> `image/jpeg`, and EVERY rendition UsArr stores for an asset — all seven widths, `orig`
> included — is produced by UsArr's own encoder in the codec `image_asset.format` names. There is
> no passthrough width.** `image_asset.format` holds the token `jpeg`.
>
> This clause is not decoration; it is the invariant `image_asset.format` is only correct under, and
> it is discharged **by definition rather than by code**, because there is no pipeline yet to
> enforce it in. §4.4 stores **up to seven widths per asset** and the column is **one per row**, so
> without it the schema cannot express a state the rest of the design would then require.
> - **Per-`role` codec variation stays open** — PNG for logos, floated under *Alternatives* below,
>   is per-row because `role` is a column on the same row.
> - **Per-width codec variation is foreclosed.** Serving `?w=orig` as the upstream's untouched bytes
>   would put two codecs behind one row. If that is ever wanted it is an amendment to this ADR plus
>   a **second** column for the passthrough codec — measured at one more one-line
>   `ALTER TABLE ADD COLUMN` in `00008`'s header, not a rebuild.
> - **The first writer is the code that owes this.** Until then the guard in clause 4 is what stops
>   a writer landing without meeting it.
>
> **2. AVIF is deferred, not rejected, and the deferral has a named reopening condition** (below).
> §4.4's *lazy, off-the-request-path, behind a `min(NumCPU,4)` semaphore* shape is unaffected and
> stands — what changes is which codec that machinery emits.
>
> **3. The seam is `image_asset.format`**, added by migration `00008_image_asset_format.sql`: a
> nullable `TEXT` column holding a lowercase codec token, with **no `CHECK`**, so adding `avif` costs
> one map entry in Go and no migration at all.
>
> **4. The format vocabulary is declared and validated in Go, in this commit, in
> `internal/store/images.go`** — `ImageFormatJPEG`, `ValidImageFormat`, unit-tested, plus an AST-walk
> guard (`TestImageWritesValidateTheFormatVocabulary`) that fails `make check` if a writer against
> `image_asset` ever lands without referencing the validator.
>
> **5. `ARCHITECTURE.md` §4.4 and §4.4.1 are amended to point here**, with the AVIF mentions struck
> rather than deleted, per this file's own amendment rule.

### Why stdlib — and the reason is **not** "JPEG is good enough"

**The reason is zero new dependencies in a static binary, and that is a stated project value rather
than a taste.** `CLAUDE.md` puts *"single static binary"* in the stack conventions, requires an
AGPL-compatibility check on every new dependency, and makes *"cut before you add"* a working
practice. `docs/DEVELOPMENT.md`'s gate runs `modverify` and `govulncheck` over the module graph.

The measurement that makes this concrete: **UsArr has five direct dependencies today.**

```
github.com/ncruces/go-sqlite3   github.com/pressly/goose/v3
golang.org/x/crypto             golang.org/x/text             gopkg.in/dnaeon/go-vcr.v4
```

`image/jpeg` adds nothing to that list. It is in the standard library, it encodes as well as
decodes, its vulnerability surface is already tracked by the `govulncheck` run that sets this
project's Go floor, and it needs no AGPL check because there is no third party to check.

**A future reader should be able to weigh this rather than obey it, so here is the honest ledger.**
JPEG is a 1992 codec. Against AVIF at equal perceptual quality it is **commonly reported at roughly
2–3× larger** on photographic content — ⚠️ **that ratio is recollection, not a citation and not a
measurement on this project's own posters**, and it is marked as such because the rest of this ADR
is careful to mark its unmeasured numbers; it is an order-of-magnitude sanity figure and nothing
should be decided on its third significant digit. Posters are photographic content — on §4.4's own worked example (a 60-item
viewport at ~5–9 MB) that difference is megabytes per screenful, on the exact axis the owner named
as requirement number one. **This decision spends bytes on the wire to buy a dependency-free
binary.** If a future reader measures the two costs below and concludes the bytes matter more, they
are not overturning a principle — they are supplying the number this ADR did not have.

Three smaller properties, none of them decisive on their own:

- **Universal decode.** Every browser and every OS preview pane renders JPEG. AVIF's browser support
  is broad in 2026 but is not the same statement.
- **Encode speed.** §4.4 already records AVIF as *~10–20× slower than WebP*, which is why it was put
  off the request path. JPEG encode is fast enough that the semaphore is a safety rail rather than a
  necessity.
- **Decode coverage is a genuine residual risk, and it is now MEASURED rather than assumed — the
  assumption an earlier draft made here was wrong.** The stdlib decodes JPEG, PNG and GIF. It does
  **not** decode WebP; `golang.org/x/image/webp`'s package synopsis is *"Package webp implements a
  decoder for WEBP images"* and it exports only `Decode` and `DecodeConfig`
  ([pkg.go.dev](https://pkg.go.dev/golang.org/x/image/webp), re-verified 2026-08-19). There is **no
  pure-Go AVIF decoder in `x/image` at all**.
  ⚠️ **That draft said *"no v0.1 source is known to serve WebP `MediaCover`"*. v0.1's catalogue
  source is Kavita (§16.1, [ADR-0041](#adr-0041)), and Kavita's *Admin settings → Media → **Save
  Media As*** selects the format it writes covers, thumbnails and bookmarks in: **PNG (the default),
  WebP, or AVIF** ([wiki.kavitareader.com](https://wiki.kavitareader.com/guides/admin-settings/media/),
  read 2026-08-19). So the shipped adapter serves a stdlib-decodable format **by default and by
  default only** — one admin toggle produces covers this binary cannot decode at all, and the AVIF
  setting produces them in the very codec this ADR defers.
  **This does not change the decision** — the *output* codec is still JPEG and still free — but it
  relocates the risk: the thing most likely to force a dependency is **input**, not output, and it
  is a checkbox on the owner's own Kavita rather than a hypothetical upstream. The pipeline that
  gets built owes a **decode-failure path that says which format it could not read**, so this
  surfaces as a nameable fault rather than a permanently grey card.

### Why AVIF is deferred — the measured trade, stated as a trade

**AVIF's cost here is one MIT dependency plus a second WebAssembly runtime, and the second half is
the part that is easy to get wrong.**

- **`gen2brain/avif` v0.6.0 — MIT, cgo-free, ships libaom as WASM run under `tetratelabs/wazero`.**
  MIT is AGPL-compatible, so licensing is not the objection.
- **wazero would be a *second* WASM runtime in this binary, not a shared one.** `CLAUDE.md` records
  that `ncruces/go-sqlite3` **moved off wazero to `wasm2go` on 2026-03-05**, and explicitly warns
  against repeating the "shared runtime with the plugin host" argument because it no longer holds.
  **Verified on this tree, not taken from the note:** `grep wazero go.mod go.sum` returns nothing and
  `go list -deps ./...` returns **zero** packages matching `wazero`. So the runtime is not already
  paid for — adding AVIF adds it from scratch.
- **⚠️ The binary-size delta is UNMEASURED.** A WASM runtime plus a WASM-compiled libaom is
  plausibly several megabytes on a single static binary, and this ADR does **not** know the number.
  It is recorded as unknown rather than estimated, because an estimate here would read as evidence.
- **`gen2brain/gav1d` is days old with negligible adoption, and nothing should depend on it.**
  Naming it is not an endorsement — it is the opposite: it is in this record so that a future reader
  finding it does not mistake proximity to `gen2brain/avif` for maturity.

**What reopens this — the condition, stated so it can be met:**

> **Someone measures (a) the binary-size delta from adding `gen2brain/avif` and wazero to this
> build, and (b) the wall-clock encode cost per image at the seven allowlisted widths on the owner's
> hardware — and decides, with both numbers in front of them, that the bytes saved on the wire are
> worth them.** That is an amendment to this ADR plus one entry in `internal/store/images.go`'s
> `imageFormats` map. **No migration**, because `00008` deliberately left the column unconstrained.
>
> A second, independent trigger: **an upstream is found to serve cover art in a format the stdlib
> cannot decode** (WebP being the realistic case). That forces a dependency decision regardless of
> the byte-size argument, and it should be taken here rather than in the PR that hits it.

### Alternatives considered and rejected

- **AVIF now, as §4.4 originally said.** Rejected on the trade above: an unmeasured multi-megabyte
  binary cost and a second WASM runtime, taken *before* anyone has a single real poster in a real
  cache to measure the saving against. **`CLAUDE.md`'s "cut before you add" points this way and the
  replica thesis is unproven on real data yet** — the moment to pay for a codec is after the
  pipeline exists, not in the migration that gives it a column.
- **WebP as the base.** Rejected on a measured fact: `golang.org/x/image/webp` is **decode-only**,
  so WebP costs a third-party *encoder* — the same dependency objection as AVIF, for a codec that
  saves markedly less than AVIF does. It buys the worst of both.
- **PNG as the base** (also stdlib, also zero dependencies). Rejected: PNG is lossless and is
  dramatically *larger* than JPEG on photographic content, which inverts the one axis that matters.
  PNG remains the right choice for logos if the pipeline ever distinguishes them by `role`; that is
  not decided here and the unconstrained column leaves it open.
- **Store the upstream's bytes untouched and skip the encode.** Foreclosed by §4.4's own width
  allowlist, as set out in Context. Recorded because it is the first thing a reader will propose.
- **Add the column later, when the encoder is written.** Rejected on cost asymmetry. Nothing writes
  `image_asset` today, so the column is a one-line `ALTER` now; after a populated image cache it is
  a 12-step rebuild of a table that `work.poster_asset_id` and `work.backdrop_asset_id` both
  reference. **The seam ships, the feature does not** — which is exactly `CLAUDE.md`'s rule.
  🔻 **The premise dated 2026-08-19; the rejection is not weakened by it but vindicated.**
  *"Nothing writes `image_asset` today"* was true when written and was falsified by `7e5934d` —
  the same falsifier the Consequences rider below records, and the whole correction lives there
  rather than being restated here. The encoder arrived **after** this ADR, exactly as this bullet
  assumed it would, so the cheap `ALTER` was taken at the only moment it was cheap and the 12-step
  rebuild was never owed. The argument is unchanged and the alternative stays rejected.
- **Name the format only in Go and leave the schema alone.** Rejected — ⚠️ **but not on the
  grounds an earlier draft gave.** That draft said the encoding of a stored row would be
  "unrecoverable"; under clause 1's one-codec-per-row invariant it is perfectly recoverable, because
  every row written before a second codec lands is JPEG by construction and
  `UPDATE image_asset SET format='jpeg' WHERE state='ready'` after a later `ADD COLUMN` would be
  exactly right. **The real ground is that the invariant is a promise and the column is a record.**
  Clause 1 itself contemplates PNG for logos, a per-`role` split that would break the backfill and
  break it *silently*, on rows nobody would think to re-examine. A column costs one nullable `TEXT`
  and no index; discovering after the fact that the promise was not kept costs the cache.

### Consequences

- **`ARCHITECTURE.md` §4.4 and §4.4.1 no longer claim AVIF ships.** The AVIF mentions are struck in
  place with a pointer here, per this file's amendment rule.
- **`image_asset` gains one nullable column and no index.** `reference/schema.md` §12 is updated;
  `internal/db/testdata/schema.sql` is regenerated.
- **The `/img` surface owes a token→media-type lookup**, one line, when it is built. `jpeg` →
  `image/jpeg`. It reads the token for **every** allowlisted width including `orig`, which is only
  sound because clause 1 forbids a passthrough width.
- **The pipeline owes a decode-failure path that names the format it could not read**, because
  clause 1 governs what UsArr *writes* and says nothing about what an upstream *serves* — and the
  measurement above shows the shipped adapter can be configured to serve WebP or AVIF.
- **⚠️ This ADR describes a pipeline that does not exist.** Nothing in Go writes `image_asset`,
  there is no fetcher, no downscaler and no encoder. What ships here is the *decision*, the column
  and the vocabulary guard — not the image pipeline. Stating that is required by `CLAUDE.md`'s "no
  invented status", and it is also the honest scope: the reopening condition above is written for
  whoever builds the pipeline, who will be better placed to measure than this thread was.
  🔻 **Superseded 2026-08-19 by `7e5934d`, which built it.** `internal/imagepipeline` fetches,
  decodes, downscales to all seven of §4.4's widths and encodes every one of them — `orig`
  included — through stdlib `image/jpeg`, exactly as clause 1 requires; `internal/store`'s
  `PutPosterAsset` writes the row and calls `ValidImageFormat` on the value it stores, so the
  vocabulary guard is no longer vacuous. **Two of this ADR's three owed items are discharged with
  it**: the decode-failure path names the format it could not read (`ErrUnsupportedFormat`, which
  quotes the upstream's declared `Content-Type` as the diagnostic), and `x/image`'s WebP decoder is
  registered — so the *Save Media As* hazard measured above now costs an honest refusal instead of
  a silent skip for WebP, and still costs one for AVIF. The `/img` token→media-type lookup was
  already built. **The deferral itself is untouched**: AVIF is still deferred on the same measured
  trade and the same reopening condition, and nothing here reopens it. ⚠️ **What `7e5934d` did NOT
  do is meet a real cover** — the pipeline is tested only against images its own tests fabricate,
  and no import calls it yet.

---

<a id="adr-0051"></a>
## ADR-0051 — The library-scoped grid is a work-driven `EXISTS`, not a join to `library_member`

**Status:** Accepted · **2026-08-19** · **Supersedes [ADR-0026](#adr-0026)'s materialisation as
[`ARCHITECTURE.md`](./ARCHITECTURE.md) §6.5 reads it — for the `added_at` order and no other** ·
**Costs one migration**, `00009_edition_format_index.sql`, which is for the *media-type* filter and
not for the scope · **Adds no column, no table and no rewrite of `library_member`** · **Explicitly
does NOT add `ix_libmem_added`** · **Reopens on a `make bench` measurement over a narrow library**

### Context

`GET /api/v1/library` is §17.2's per-type grid and §17.8's library scope chip. Its default order is
`added_at DESC, id DESC` — the same order Home's Block C walks — and its scope is a multi-select over
user-defined libraries, so the query is "the newest works that are members of any of these
libraries".

§6.5 had already answered how a library-scoped page is served, and answered it well for the question
it asked. Its ⚠️ block records that the naive shape "either probes membership per candidate (≈18
index rows scanned per row returned, so ~1,800 probes for a 100-row page) or fetches every member and
sorts before the window can be cut", and concludes that **the denormalisation that was the fallback
is now the default**: `library_member`'s primary key becomes
`(library_id, sort_title, work_id, edition_id)` `WITHOUT ROWID`, "which makes the scoped keyset a
single covered seek at any selectivity".

**That sentence is true and it is about `sort_title`.** The key leads with `sort_title` immediately
after `library_id`, so it supplies the alphabetical order and nothing else. §6.5 does not say which
order it means, because at the time it was written the grid had one order in mind. The grid that
actually shipped has three, and its default is not that one.

### The measurement

Taken on the real migrated schema with this repo's own engine — `github.com/ncruces/go-sqlite3` with
`ext/fts5` registered, reporting **SQLite 3.53.4** — over §17.8's flagship topology, in both of the
two library shapes §6.5 asks for by name.

> ⚠️ **The system `sqlite3` CLI cannot build this schema at all**, so a plan measured by piping the
> migrations into it is a plan for whatever subset loaded before the error: migration 0005 contains
> `RAISE(ABORT, 'a' || 'b')` and the CLI rejects it. `internal/store/browse_test.go` carries this
> note where someone doing plan work will meet it.

**The member-driven shape, `added_at` order, one library and two libraries over one kind:**

```
SEARCH lm USING COVERING INDEX ux_libmem_identity (library_id=?)
  | SEARCH w USING INTEGER PRIMARY KEY (rowid=?)
  | USE TEMP B-TREE FOR ORDER BY
```

Identical in both topologies, **with and without `ANALYZE`**. The temp b-tree is the whole point: the
membership key orders by `sort_title`, the query orders by `added_at`, and there is no index that
holds both. A 1%-selective library is not a mitigation — the sort is over the *members*, so it grows
with the library, and it is paid on **every** page rather than once.

**The work-driven `EXISTS`, same order, same two topologies, all three page shapes:**

```
SEARCH w USING INDEX ix_work_added (added_at<?)
  | SEARCH lm EXISTS USING COVERING INDEX ix_libmem_work (work_id=? AND library_id=?)
```

with `SCAN w USING INDEX ix_work_added` on the first page (an ordered walk, no sort) and
`SEARCH w USING INDEX ix_work_added (added_at=? AND id<?)` in the undated tail. **No temp b-tree in
any of them, and none in the scoped-install variant either**, where the access-scope `EXISTS` over
`service_item_link` joins the plan as a third seek on `ix_sil_work`.

**The multi-value case was a hypothesis and is now a measurement.** `?lib=a,b` puts an `IN` on
`library_member.library_id`, which is that table's *leading* key column — and an `IN` on a leading
column is exactly what stops SQLite supplying `ORDER BY` from an index (the property
`store.Scope.userPredicate` already records for `ix_prov_user_grabbed`). In every **member-driven**
shape that is fatal. Under the `EXISTS` the ordering comes off `work` and the `IN` lands on a probe,
so the plan above is unchanged from the single-library one. That is what the plan output says, and it
is asserted in both topologies rather than argued.

**The probe is covering for free.** `library_member` is `WITHOUT ROWID`, so a secondary index carries
the primary key in its own entries — `ix_libmem_work(work_id)` is really `(work_id, library_id,
sort_title, edition_id)`. The membership test therefore constrains both columns and reads no table
row at all.

⚠️ **What `ANALYZE` DOES move, reported because it was measured and is not being claimed as a
win.** With `sqlite_stat1` present on the *fixture*, the two correlated subqueries over `edition` —
the media-type `MIN()` in the `SELECT` list and the Audiobooks `NOT EXISTS` — reshuffle among
`ix_edition_work` and `ix_edition_format`, and a `BLOOM FILTER` appears on the Audiobooks probe. That
is a **small-corpus artifact**: the fixture has single-digit `edition` rows, so a covering index scan
really is cheaper there than a seek, and nothing follows from it about 58,500 rows. **The half that
is stable across both planners is the half this ADR is about** — the driving table, the ordering
index, and the absence of a temp b-tree are identical with statistics and without, in both
topologies, for all three orders.

### The decision

**A library-scoped page in the `added_at` or `popularity` order is work-driven, with an `EXISTS` over
`library_member`. The member-driven shape is the right one for `sort_title` and keeps it.**

Three consequences, in the order they matter:

1. **The ordering index is never displaced.** `ix_work_added` and `ix_work_pop` are both
   `WHERE deleted_at IS NULL` partial indexes on `work`, and the scope becomes a filter *inside* the
   walk rather than a different driving table. Adding a library chip therefore cannot change which
   index the page comes off, which is the property that keeps §13's `< 8 ms` budget a budget about
   one thing.
2. **A work cannot come back twice, and no `DISTINCT` is needed to promise it.** §17.8's flagship
   case is one Audiobookshelf library offered as Ebooks *and* Audiobooks, so a book with an EPUB and
   an M4B is **one `work` row with two membership rows**. Under a join, `?lib=ebooks,audiobooks`
   returns it once per matching membership row — and both copies are *correct rows*, so the
   duplicate is silent. Under the `EXISTS` it is returned once by construction. A `DISTINCT` would
   fix the count and reintroduce a sort.
3. **`ix_libmem_added` is not owed, and must not be added on this ADR's authority.** The obvious
   reflex on reading "the added_at order needs an index" is
   `library_member(library_id, added_at)`. It would serve the *member-driven* shape this ADR is
   choosing against, so under the chosen shape it is an index with no reader — and
   `library_member.added_at` is the date the work joined the LIBRARY, which is not `work.added_at`
   and is not what the grid orders by. Two dates that look alike is a worse trap than a missing
   index.

### What this ADR does **not** decide

- **It does not touch `library_member`'s key or the denormalised `sort_title`.** §6.5's argument for
  that column is untouched and `TestLibraryScopedKeysetIsASeek` still pins it, in both topologies.
- **It does not change Block C.** `GET /api/v1/library/recent` has no scope and gets none here.
- **It says nothing about `year`.** `library.default_sort` legally takes `'year'` and `work.year` has
  **no index at all** — six indexes on `work` and not one leads with it — so a year-ordered page is a
  temp b-tree over the whole filtered corpus, which is the failure this ADR exists to refuse. The
  read returns `ErrUnservableSort` and the endpoint renders a `400` naming the missing index.

  ⚠️ **Amended 2026-08-19 — the column can legally store a value the API refuses, and that is
  written down here rather than left in a comment.** The decision above is unchanged; this records
  the shape of the gap where the next person to touch `default_sort` or the sort vocabulary will
  look for it.

  **The two vocabularies, spelled out.** Migration 0005 admits **four**:
  `CHECK (default_sort IN ('sort_title','added_at','year','popularity'))`. The browse read serves
  **three** — `added_at`, `sort_title`, `popularity` (`browseSorts`, `internal/httpapi/library.go`;
  `WorksSort`, `internal/store/browse.go`). The gap is exactly one member, `year`, for the reason
  the bullet gives: nothing indexes `work.year`, so the order cannot come off an index and the page
  would sort the whole filtered corpus every time.

  **What the code actually does with it — checked, not assumed.** `?sort=year` is **refused, never
  substituted**. `browseWorksSQL` names `year` in its own `case` arm and returns `ErrUnservableSort`
  rather than letting it fall into the unknown-sort arm; `handleBrowseWorks` matches it against
  `browseUnservableSort` and renders a `400` whose message says *there is no index* and whose action
  names the three servable orders. It is not a `500`, and it is **not** a silent fall-back to
  `added_at` — a fall-back is the failure mode this endpoint's whole refusal discipline exists
  against, because a page that quietly served a different order would look like the parameter worked.

  ⚠️ **The endpoint never reads `library.default_sort`.** With `?sort=` absent the read defaults to
  `added_at` whatever the library's stored default says, so a library configured with
  `default_sort = 'year'` does not *itself* produce a refusal — the refusal happens when a **client**
  reads that stored default back to `?sort=`, which is precisely the caller that deserves the real
  reason and is why `year` gets a message of its own rather than "not an order this library serves".

  ⚠️ **Today the trap is armed but unreachable, which is why it is easy to miss.** No Go code reads
  or writes the column: `default_sort` appears in `internal/` only inside comments and tests, in no
  `SELECT`, `INSERT` or `UPDATE`. Every `library` row therefore carries the schema default
  `'sort_title'`, and `'year'` can only get in through hand-written SQL. **It becomes reachable the
  day §17.8's DETAIL view lets a user choose a library's default sort** — that screen must either
  offer three values rather than four, or ship the index. Closing the gap the other way is a **new
  migration** (an `ix_work_kind_year`, or a `(year, id)` index if the unfiltered case is ever
  wanted), never an edit to the allowlist: adding `year` to `browseSorts` without an index behind it
  puts the temp b-tree this ADR refuses onto a render path.
  Serving it is a new migration and a decision §17.2 owns; **it is not fixed here and it is not
  forgotten.**
- **It says nothing about `sort_title` over more than one kind.** `ix_work_kind_sort` is
  `(kind, sort_title, id)` and `media_type=music` is two kinds, so that combination is refused for
  the same reason `year` is. The two honest fixes — an index that does not lead with `kind`, or
  splitting the Music grid into Artists and Albums — are §17.2's.

### The alternatives, and why they lose

**The member-driven join with the denormalised key (§6.5's default).** The incumbent, and it loses on
one measured fact rather than on taste: it sorts, in both topologies, with and without statistics,
for the order the grid actually defaults to. Its own justification is explicitly about making "the
scoped keyset a single covered seek", and it still is — for `sort_title`, which is why it is kept
there rather than replaced.

**The join plus `ix_libmem_added` on `(library_id, added_at, work_id)`.** This *would* order without
a sort, for one library. It fails on three counts. The `IN` for `?lib=a,b` puts the multi-select back
into a temp b-tree, and the multi-select is the chip's normal state. It orders by the wrong date —
`library_member.added_at` is when membership was materialised, `work.added_at` is when the item
arrived — so it answers a different question that would render identically and be wrong. And it costs
a second date column on the hot membership table for a shape the `EXISTS` already serves.

**The join with `DISTINCT` or `GROUP BY w.id`.** Fixes clause 2's duplicate and reintroduces a sort
to do it, which is the cost the whole exercise is about.

**A materialised per-(library, order) view.** Rejected as "and also": it is a fourth table and a
fourth writer for a query that already plans as a single ordered walk.

### The reopening condition

**`make bench`, over a NARROW library, on the `added_at` order.** This is not a formality and the
mechanism is worth stating plainly: the `EXISTS` walks the **global** `added_at` order and **discards
non-members as it goes**. That is excellent when the scope is most of the catalogue and it degrades
linearly as the scope narrows — a library holding 1% of a 25k-row kind means roughly 100 candidate
works read and rejected for every row returned, and §13's own budget row is written for exactly that
shape (*"1%-selective library over a 25k-row kind"*). A plan assertion cannot see this: `EXPLAIN
QUERY PLAN` chooses from the schema and not from the data, which is the same limitation
`TestLibraryScopedKeysetIsASeek` already records about itself. **So the plan guards are not evidence
that this is fast on a narrow library, and they are not offered as such** — they are evidence that it
does not sort. The wall clock belongs to `make bench`, which is not and must not become a merge gate.

If that measurement comes back badly, the fix is a member-driven shape with an ordering index on
`library_member` — the one clause 3 declines to add today — plus a plan for the multi-library `IN`,
and this ADR is superseded rather than amended.

### What is built

`internal/store/browse.go` (`ListWorks`, `browseWorksSQL`), the route in
`internal/httpapi/server.go`, `handleBrowseWorks` in `internal/httpapi/library.go`, and migration
`00009_edition_format_index.sql`. The plan assertions are in `internal/store/browse_test.go` — both
topologies, all three page shapes, all three orders, the scoped install, and four guard-firing arms.

⚠️ **Every plan guard in that file pins the NO-`ANALYZE` planner**, which is the one the test suite
sees and **not** the one the binary usually runs: a test database is fresh and has no `sqlite_stat1`,
while a production database has one because the importer runs `ANALYZE` after a full import. The
difference is not cosmetic — the unary `+` on `+w.kind` changes the plan **only** when statistics are
absent. A plan that holds without statistics holds with them, so the guards are conservative and
correct; they are simply not a measurement of production, and each one says so where it is written.

---

<a id="adr-0052"></a>
## ADR-0052 — v0.1's catalogue source is **BookOrbit**; Kavita is sunset, and its adapter stays in the tree

**Status:** Accepted · **owner-decided 2026-08-19** · **Amends [ADR-0041](#adr-0041)**, whose decision
clause 1 — *"v0.1's catalogue source is **Kavita**, not Sonarr and Radarr"* — is the sentence this
changes; ADR-0041's clauses 2 and 3 are **confirmed, not reopened**, and its clause 4 is **reopened as
an open question rather than answered** (see *What this ADR does not settle*) ·
**Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16**, which remains authoritative for scope; the §16.1
edit lands **in this same change** · **Re-sequences, rejects nothing** — `internal/libsync/kavita.go`
and `internal/kavita` **stay in the tree** · **Confirms [ADR-0043](#adr-0043)** and
[ADR-0044](#adr-0044), both of which were written to turn on a property of the source rather than on
its name · **Needs no migration**, and the schema check that established that is recorded below ·
⚠️ **Reverses a standing recommendation**, [`ROADMAP.md`](./ROADMAP.md) §3's *"do NOT switch UsArr's
first adapter off Kavita"*, on evidence that was re-measured after that recommendation was written

### Why this is **Accepted** and not **Proposed**, asked because it removes an evidentiary basis

**The question was put in adversarial review and is answered here rather than left implicit:**
[ADR-0041](#adr-0041)'s strongest sentence was that Kavita's delta had been *"verified against a live
instance"*, and this ADR removes it. **Should it stay Proposed until a probe runs?**

**No, and the reason is that the two things are not on the same axis.** What is **decided** is
*which stack v0.1 is built against*, and that is the owner's call about his own server — it does not
become more or less true when a probe runs, and no agent may hold it Proposed pending evidence the
owner did not ask for. What is **evidence** is *which sync channels that stack can support*, and this
ADR does not decide it: [ADR-0041](#adr-0041) clause 4 does not carry over, and open question 1 below
now settles the series half **from source** and leaves a narrowed probe owed.

⚠️ **The honest cost is stated rather than absorbed: v0.1's channel list is weaker than ADR-0041's,
and §16.1 says so in the same change.** A `Proposed` status would misrepresent that as indecision
about the source. **What would justify reverting to Proposed** is the owner disputing that he is
moving off Kavita — not a probe result, which is already scoped to change the channel list and not
the source.

### Context

#### 1. The decision is the owner's, and it is a decision rather than a delegation

This distinction has cost this project before — `DECISIONS.md`'s own preamble records the `162dca5`
post-mortem, where a sentence that no ADR ever decided was written into an ADR body as though one
had. So it is stated exactly.

**The owner is sunsetting Kavita entirely and moving his books, comics and manga to BookOrbit.**
BookOrbit takes everything Kavita held. His word for it, verbatim, is **"phenomenal"**. He runs a
sidecar of his own that pulls data from MangaBaka into BookOrbit today, and he expects official
BookOrbit support for MangaBaka **"in the near future"**.

🔍 **Provenance, marked because it is not first-hand to this ADR.** Those words reached this record
through the coordinating thread rather than from the owner directly in it. The repository's own
verbatim record of the same direction is one day older and points the same way —
[`ROADMAP.md`](./ROADMAP.md) §3, dated **2026-08-18**: *"in my heart i kind of want to migrate to book
orbit… it doesn't have a paid tier"*. What that entry called *"tracking, not a decision"* is now the
decision, and this ADR is what turns it into one. **Nothing here was decided by an agent**; what an
agent did is the re-measurement in §2, and that is evidence, not the choice.

#### 2. The evidence moved, and two of the three findings against BookOrbit are false

[`ROADMAP.md`](./ROADMAP.md) §3 records a **2026-08-17 evaluation at BookOrbit HEAD `4a420a04`**. Its
three *"against"* findings are what produced the standing *"do NOT switch"* recommendation. **All
three were re-measured against BookOrbit `main` on 2026-08-19, from its own source, and the table
below is that run.**

🚩 **The first draft of this ADR could not check out `4a420a04` and therefore recorded each
falsification as *"the claim is false today"* rather than *"the evaluation was wrong"*, choosing the
charitable reading. `4a420a04` has since been checked out, and the charitable reading does not
survive it.** The commit is real (`4a420a04a271948219f1f2853a05cb73e3c64e9e`, 2026-08-17 00:45 -0600),
and against it:

- `git diff --stat 4a420a04 HEAD -- server/src/modules/auth/magic-link.service.ts` → **empty**. The
  magic-link path is **byte-identical** at the evaluated commit and at `main`.
- `git diff --stat 4a420a04 HEAD -- server/src/modules/metadata-fetch/providers/comicvine/` →
  **empty**. All seven ComicVine files existed, unchanged.

**So both falsified findings were already false at the commit the evaluation examined.** The evidence
did not move; **the evaluation was wrong when it was written.** ✅ That *strengthens* the case for the
swap — the standing *"do NOT switch"* recommendation was never well-founded rather than merely
outlived. ⚠️ **And it carries a caution the charitable reading hid:** the third finding, the
watermark, comes from that same evaluation, so it earned no presumption of accuracy either. It is
re-measured from source in the table below rather than inherited.

| 2026-08-17 finding ([`ROADMAP.md`](./ROADMAP.md) §3) | 2026-08-19 verdict | Primary source |
|---|---|---|
| *"**no inbound API key** — headless auth needs the account password, which is worse than UsArr's Kavita credential model"* | 🚩 **FALSIFIED.** Headless auth exists and takes no password. `server/src/modules/auth/magic-link.service.ts` implements a first-class magic-link module: a raw token is issued, only its **SHA-256 hash** is stored, and `loginWithToken()` checks existence, `isActive`, not-revoked and not-expired, then calls `issueTokensForUser()`. **No password is validated anywhere in that flow.** | `bookorbit/bookorbit@main`, `server/src/modules/auth/magic-link.service.ts` |
| *"**no manga or comic external ids**"* | 🚩 **HALF FALSIFIED, and the surviving half is the narrower one.** **Comics are covered:** ComicVine ships as a metadata provider — `server/src/modules/metadata-fetch/providers/comicvine/` holds `.client.ts`, `.mapper.ts`, `.provider.ts` and `.types.ts`, and the README names *"ComicVine for comics"* among fourteen providers. **Manga and anime are not:** a repository-wide code search for `mangabaka`, `anilist` and `myanimelist` returns **zero hits**. | `bookorbit/bookorbit@main` README and `server/src/modules/metadata-fetch/providers/` |
| *"an `updatedAt` watermark that **misses tag, genre and author edits**"* | 🚩 **CONFIRMED, and it is worse than the finding says — measured from source on 2026-08-19, not deferred to a probe.** The sort key is real but it is **book-scoped**: the type carrying it is named `BookQuery` (`packages/types/src/query.ts:380`), served by `POST /books/query`, and `SORT_FIELD_MAP` binds `updatedAt` to **`books.updatedAt`** (`book-sort-builder.service.ts:35`). Tracing the tag path end to end — `POST /books/bulk-update-tags` → `BookService.bulkUpdateTags` (`book.service.ts:2362`) → `MetadataService.replaceTags` → `replaceTagsInExecutor` (`metadata.service.ts:513`) — the executor does `delete(bookTags)` + `insert(bookTags)` **and nothing else**; the follow-up `triggerPostMetadataUpdateEffects` schedules a file write/rename and recomputes the metadata score, and **none of those writes `books.updatedAt`**. `replaceGenresInExecutor` (`:480`) and `replaceAuthorsInExecutor` (`:369`) have the same shape. There is **no SQL trigger anywhere in the repo** (`grep -rli "create trigger" server/ --include=*.sql` → no match), so nothing compensates. **A tag, genre or author edit therefore does not move `books.updated_at`.** ✅ The one thing that *does* hold: core metadata edits are covered by an **explicit** touch — `updateMetadataFields` and `bulkUpdateMetadataFields` end with `update(books).set({updatedAt})` (`book.repository.ts:1960, 2012`). | `bookorbit/bookorbit@main`, `packages/types/src/query.ts:380`, `book-sort-builder.service.ts:35`, `metadata.service.ts:369,480,513`, `book.repository.ts:1960,2012` |

> ⚠️ **Citations pinned and one corrected, 2026-08-19 — the verdict above is untouched.** The row's
> 🚩 **CONFIRMED**, its end-to-end trace and its ✅ counter-example all stand. What changes is where a
> later reader is sent, and two of the additions make the verdict **harder** to argue with, not softer.
>
> **The source is pinned.** The Primary-source cell cites `bookorbit/bookorbit@main`, a moving ref that
> names a different tree every time upstream merges. The read was taken at **`73b7877`**
> (`73b7877d2fede2221b0ca360af9bfced7c3797f3`), `main`'s tip at the time. That commit is corroborated
> inside this repository — [`api/specs/SOURCES.md`](../api/specs/SOURCES.md)'s BookOrbit provenance
> table carries it, dated **2026-08-19**, with the 68-file `packages/types` tree `4cb990a3…` beside it
> — so the **ref** is pinned here even though the line numbers below are not.
>
> **One correction.** `replaceAuthorsInExecutor` is at
> `server/src/modules/metadata/metadata.service.ts:370`, not `:369`. The wrong number appears twice in
> the row, once in the prose and once in the Primary-source cell, and both mean `:370`. Nothing else in
> the trace moves: the executor's shape, `replaceGenresInExecutor`, `replaceTagsInExecutor` and the
> absence of any SQL trigger are all as written.
>
> **Strengthener 1 — the three executors are outliers, not house style.** `replaceCommunityRatings`
> (`book.repository.ts:1996`) is a delete-and-insert over a child table of exactly the same shape, and
> it **does** bump the parent's `updatedAt`. That closes off the charitable reading the row leaves
> open — that a delete-and-insert simply cannot carry a parent touch, making the gap a property of the
> write shape and expensive upstream to fix. It is not: an identically shaped write in the same
> codebase carries it. The missed touch is a **per-call-site omission**, and on balance that is worse
> for UsArr rather than better. It can be closed upstream in three lines with no type, route or
> `packages/types` file moving, so [ADR-0064](#adr-0064)'s drift check would not see it either way;
> and by the same token it can spread to the next relation somebody adds.
>
> **Strengthener 2 — the guard that lets a tags-only edit reach the gap.** The ✅ half of the row is
> narrower than it reads. The explicit `update(books).set({updatedAt})` touch sits behind
> `book.service.ts:1743`'s **`if (scalarFieldCount > 0)`**, and the relation calls sit *after* that
> guard, at `:1769`, `:1783` and `:1788`. So an edit changing no scalar field — a tags-only edit, the
> exact case the finding names — does not merely miss a touch inside an executor, it **skips the only
> touch on the path**. The honest boundary is therefore: core metadata edits are covered when at least
> one scalar field changed, and not otherwise.
>
> **Provenance, stated plainly because it is unusual.** All four pins above were measured **first-hand
> by the BookOrbit report-drafting lane on 2026-08-19**, in its own clone of `bookorbit/bookorbit` at
> `73b7877`, `main`'s tip when it cloned. The measurement and its date are the citation; there is no
> filed artefact to point at, and none is claimed. ⚠️ **No falsifier for them exists in this
> repository, and none ever will.** BookOrbit's server source is not vendored — `api/specs/bookorbit-types/`
> is `packages/types` and nothing else, 68 files — and `SOURCES.md` states the consequence in its own
> words: *"the server's own DTOs are not in `packages/types` … the controller, service and repository
> citations in `internal/bookorbit` are **unpinned by anything**. That is the largest uncovered
> surface."* Neither `make check` nor `make spec-drift` has ever read `book.repository.ts`,
> `book.service.ts` or `metadata.service.ts`, and neither ever will. A later reader wanting these four
> numbers re-checked has exactly one route, and it is a fresh clone at `73b7877`.

🚩 **One claim reached this ADR in relay and is REFUSED, because the primary source contradicts it.**
The drafting brief stated that BookOrbit has *"no watermark — no reliable 'changed since' signal — so
the sync design is full resync, with no delta channel."* **The strong form of that is false.**
`sort: "updatedAt"` with page-and-size paging is exactly the shape §7.1a's channel 3b requires — an
ordered walk on the source's own last-modified field with a **client-side** stop — and it is the same
shape Kavita's `FilterV2Dto.SortOptions` on `POST /api/Series/all-v2` presents. Writing *"no delta
channel"* into this ADR would have **foreclosed channel 3b on a premise the source refutes**, and the
repository's own record never said it either: [`ROADMAP.md`](./ROADMAP.md) §3 says the watermark is
**incomplete**, not absent. The honest position is the open question below, not a decision.

#### 3. Why the swap is affordable, which is ADR-0041's argument reused rather than a new one

[ADR-0041](#adr-0041) established that *"the sync core is source-agnostic"* — *"the streaming import
and its two-phase rendering, the batched writes and the single-writer discipline, the upsert and
identity path, the search-document builder, library membership derivation, and the reconciliation
sweep with both its guards are the same work whichever adapter feeds them."* **That claim is what is
being spent here, and this is the second time it has paid.** ADR-0041 swapped Sonarr and Radarr for
Kavita on it; this swaps Kavita for BookOrbit on it. The media types are unchanged — books, comics
and manga — so `work_book`, `work_comic` and `work_comic_issue` (migration `00006_kavita_subtypes.sql`)
and `work_credit` ([ADR-0044](#adr-0044)) are all still the right tables, written by a different
adapter.

⚠️ **What is NOT reused is ADR-0041's strongest sentence.** Its case for Kavita was *"the only one
whose delta has been verified against a live instance"* ([ADR-0035](#adr-0035) §2a — Kavita 0.9.0.2,
151 series, page size 10, clause by clause). **BookOrbit has had no such run.** That is a real
regression in evidence and it is the price of following the owner's stack, which is the same trade
ADR-0041 itself made in the opposite direction when it refused a throwaway Radarr: *"it buys real HTTP
and not real data."* The difference is that the owner **runs** BookOrbit, so the probe is available
rather than hypothetical — it has simply not been run.

### Decision

> **1. v0.1's catalogue source is BookOrbit.** It replaces Kavita in that slot, on the owner's
> decision. The media types v0.1 catalogues are unchanged: books, comics and manga.
>
> **2. Kavita is SUNSET, NOT DELETED.** `internal/libsync/kavita.go`, `internal/kavita`, the vendored
> specs `api/specs/kavita-v0.9.0.2.json` and `api/specs/kavita-develop.json`, and the contract tests
> [ADR-0046](#adr-0046) pins **all stay in the tree and stay green**. What stops is **investment**: no
> new Kavita feature work is scheduled by this ADR, and no Kavita gap is a v0.1 blocker. **Nothing is
> refused** — a user running Kavita keeps a working adapter, which is `CLAUDE.md` principle 3 applied
> to a user who is not the owner, exactly as [ADR-0043](#adr-0043) applied it.
>
> **3. This ADR invents no milestone for further Kavita work, and that refusal is deliberate.** The
> owner said he is moving off it; **he did not say what happens to it afterwards**. [ADR-0042](#adr-0042)
> refused to number Sonarr and Radarr on precisely this ground — *"Picking a number here would be
> inventing a commitment nobody made"* — and the same refusal applies. "Sunset" in clause 2 means
> *investment stops*; it does **not** mean *delete*, and a later reader must not read it as one.
>
> **4. MangaBaka is NOT a dependency of the BookOrbit adapter.** The adapter is designed against what
> BookOrbit ships **today**. The owner's expectation of official support *"in the near future"* is
> **his expectation and nobody's commitment**; no line of the adapter may assume it, and no schedule
> may be built on it. If native support lands, this ADR gains an amendment.
>
> **5. MangaBaka data may be fetched at runtime and MUST NEVER be vendored, shipped or cached as a
> dump.** See the licence section below. This is a distribution constraint on UsArr itself, not a
> style preference.
>
> **6. Identity needs no migration and no new mechanism.** A source-supplied series id lands in
> `external_id` keyed on `work_id`, at a `confidence` that grades how much the source is trusted —
> **the pattern `internal/libsync/kavita.go` already uses for seven ids**, `mangabaka` among them.
> This ADR **does not design the mapping**, which a separate pass owes; it records that the mechanism
> exists so nobody budgets a schema change for it.

### The MangaBaka licence constraint, stated as a constraint

**Verified from the primary source on 2026-08-19.** `https://mangabaka.org/data/database` states:
*"MangaBaka-original data in this download is licensed under CC BY-NC-SA 4.0 — free for personal and
non-commercial use with attribution,"* and, separately, *"Third-party data (from AniList, MAL, etc.)
remains subject to each provider's own terms."*

Two consequences, and the second is the one a later implementer will trip over:

- **CC BY-NC-SA 4.0 is not AGPL-3.0-compatible.** `CLAUDE.md` requires an AGPL-compatibility check on
  anything new, and names MIT, BSD and Apache-2.0 as fine. A **NonCommercial** and **ShareAlike** term
  is neither. Vendoring MangaBaka-original data into this repository would change UsArr's own
  distribution terms.
- 🚩 **Caching a dump is the temptation, and it is foreclosed here by name.** A nightly dump is
  published, it is far cheaper to walk than an API, and reaching for it is the obvious engineering
  move. **It is redistribution.** Fetching at runtime, on the user's own server, on the user's own
  behalf, is a different act from shipping the bytes; only the first is available to UsArr.
  ⚠️ And the dump is not even uniformly under that licence — the third-party sentence above means a
  cached dump carries AniList's and MAL's terms with it, unexamined.

### What this ADR does not settle — and one thing it turns out to settle after all

**Two open questions and one correction. Each open question names what would close it; the
correction names what falsified the draft that preceded it.**

🚩 **1. Whether channel 3b applies to BookOrbit — and the series half of that is now CLOSED, in the
negative, from source.** [ADR-0041](#adr-0041) clause 4 fixed v0.1's channels at **1, 3b and 4** for
Kavita, on a probe run in advance ([ADR-0035](#adr-0035) §2). **BookOrbit has had no equivalent run,
so clause 4 does not carry over.** But *"pending a probe"* overstates what is still open, so the two
halves are separated.

🚩 **The series half is not an open question. There is no series-level ordered read in BookOrbit at
all**, measured 2026-08-19 against the clone at `73b7877d` and re-checked at the evaluation's own
commit `4a420a04`, identical at both:

| Measured | Result | Where |
|---|---|---|
| Every route the series controller exposes | **Exactly two**, both `GET`: `/series` and `/series/:seriesId/books`. No `POST …/query` counterpart to `POST /books/query`. | `server/src/modules/series/series.controller.ts:13,18` |
| The sort keys `GET /series` admits | `SERIES_LIST_SORTS = ['name', 'bookCount', 'lastAddedAt', 'readProgress']` — **no `updatedAt`**, and `@IsIn(SERIES_LIST_SORTS)` **rejects** one with a 400 rather than ignoring it. | `dto/list-series.dto.ts:4,34` |
| Whether a series watermark is even readable | **No.** `book_series.updated_at` **exists** as a column with `$onUpdateFn`, but the series repository never selects it — the projection is `id, name, bookCount, readCount, lastAddedAt`. And `lastAddedAt` is `max(books.added_at)`, an **added**-time aggregate that cannot observe an edit. | `db/schema/series.ts:16`; `series.repository.ts:84,100,120` |
| Whether `collapseSeries` supplies one indirectly | **No.** The collapsed query picks one representative per series by `series_index ASC NULLS LAST, added_at ASC, id ASC` and orders on **that row's** `updated_at` — not a `MAX()` over the group. Editing volume 7 moves nothing. | `book.repository.ts:142-145,976` |

**Why that is decisive rather than a detail: UsArr's unit of work for comics and manga is the
series, not the book.** `work.kind`'s `'comic'` **is the series** (`00006_kavita_subtypes.sql`;
`00005_library_sync.sql:242` — *"'comic' is the SERIES, 'comic_issue' the issue or chapter"*), and the
shipped adapter walks `POST /api/Series/all-v2` for exactly that reason
(`internal/libsync/kavita.go:210`). So the ordered read BookOrbit **does** offer — `POST /books/query`,
whose type is named `BookQuery` (`packages/types/src/query.ts:380`) — is at the wrong grain for
`work_comic`, and a rename or an `expectedBookCount` change on a BookOrbit series moves **no** book's
`updated_at` and is invisible to it.

✅ **The book half survives, and is the honest residue.** For `work_book`, `POST /books/query` with
`sort: [{field:"updatedAt"}]` and `pagination: {page,size}` is a real ordered walk with a deterministic
tiebreaker (`books.id ASC`, `book-sort-builder.service.ts:52`) — the same POST-with-sort-body shape as
Kavita's `all-v2`. **What §2's table then removes is its soundness for edits**: a tag, genre or author
change does not move `books.updated_at`, traced through the write paths there.

**What a live probe is still worth**, therefore, is narrower than *"does 3b apply"*: it would confirm
the traced write paths against a running instance and check the paths not traced. **It cannot restore
a series-ordered read, because none exists to find.**
ℹ️ **The failure branch is already specified and costs no new design**: §7.1a says a source without
the ordering guarantee *"falls back to **reconciliation only**"*, with the Services row (§17.3) reading
`no change feed — full compare at 09:12` in place of a delta time. **On the measurement above that is
the expected outcome for the comics and manga half, not a contingency** — and it is a documented
degradation rather than a new one.

✅ **2. Identity needs no new design and no migration — which is the opposite of what this ADR's
first draft said, and the correction is the more useful record.**

**The draft carried a framing that a schema check falsified.** It said BookOrbit's series-level
identity was a *structural degradation* against §6.2's *"External IDs are the **only** reliable
cross-instance join key"*, and that closing it might need UsArr-side storage — a migration. **Three
citations were checked against the tree on 2026-08-19, all three hold, and together they invert the
conclusion:**

| Checked | What the tree says | Where |
|---|---|---|
| `external_id`'s shape | `source TEXT NOT NULL` with **no `CHECK`**, `confidence REAL NOT NULL DEFAULT 1.0`, `CHECK ((work_id IS NULL) != (edition_id IS NULL))`, unique on `(source, value, COALESCE(work_id,-1), COALESCE(edition_id,-1))` | `internal/db/migrations/00005_library_sync.sql:444` |
| A series **is a work row** | `work.kind`'s twelve-member `CHECK` admits `'series'` and `'comic'`, and the migration's own comment says *"'comic' is the SERIES, 'comic_issue' the issue or chapter"*. So **`external_id.work_id` already IS the series-level column** | `00005_library_sync.sql:242` |
| UsArr already writes series-level ids from user-recorded data | `kavitaExternalIDs` writes seven: `hardcover_book` at 1.0; `metron_series` and `comicbookroundup` through `editableIdentity`; `anilist`, `mal_manga` and **`mangabaka`** through `webLinkIdentity` at `WebLinkConfidence = 0.90`; ComicVine through `comicVineIdentity` | `internal/libsync/kavita.go:432-490`, `internal/libsync/weblinkid.go:111,162` |

🚩 **So the honest reading is that BookOrbit is NOT a degradation relative to Kavita, because Kavita
was never the thing the draft implicitly compared it to.** Kavita's series table supplies no
first-class series identifiers either — `internal/libsync/kavita.go`'s own comment records that
`AniListId`, `MalId` and `MangaBakaId` are **weblink-parsed** from a `<Web>` element the user's tagger
wrote, or typed into the Edit Series dialog, and that *"MangaBaka has no provider writer at all"*.
That is exactly why they are capped at 0.90 rather than trusted at 1.0. **BookOrbit's admin-defined
custom metadata field is the same arrangement under a different name** — user-populated,
API-readable. What both assessments actually show is that **series identity in this domain comes from
what the user records, not from what the server models**, and UsArr was built for precisely that, with
`confidence` grading the trust.

**Concretely: reading a MangaBaka id out of a BookOrbit custom field and writing `source='mangabaka'`
at 0.90 uses the existing pattern, the existing source string and the existing confidence grade.** No
new column, no new table, no new confidence tier, **no migration**. `source` carries no `CHECK`, so a
new source string is a naming decision rather than a schema change.

ℹ️ **The one real wrinkle, recorded because the adapter thread will meet it and it is still not a
migration.** BookOrbit's custom metadata values are **book-scoped** — verified 2026-08-19 against
`server/src/db/schema/custom-metadata.ts`, whose `bookCustomMetadataValues` carries
`bookId: integer('book_id').notNull().references(() => books.id, …)` and which declares **no
series-scoped variant**. UsArr's series is a **work** row, so a series id recorded on a book has to be
hoisted a level. ⚠️ **Kavita has the identical hoist and it is documented as lossy** — its weblink ids
are read off a *chapter's* `ComicInfo.xml` and erased to `0` when the first chapter carries no link
for that site ([`REVIEW-LOG.md`](./REVIEW-LOG.md) LS-38/LS-39). **Same shape, same hoist, same 0.90.**
How UsArr resolves disagreement between two books of one series is an adapter question for the
implementation thread — not a schema question, and not this ADR's.

⚠️ **What this section does NOT do.** It does not design the mapping, name the custom field, or
enumerate MangaBaka's identifiers — a separate pass owns that input, and this ADR ships no adapter
code. It records that **the mechanism those inputs will land in already exists**, so that nobody
budgets a migration for it.

🚫 **`work_relation` is deliberately cited nowhere in this ADR, and this line exists so a later reader
does not add it.** It **does not exist in the tree**: `internal/db/migrations` creates no such table
and `internal/db/migrate_test.go`'s `deferred` list **fails the suite if it appears**. It is a **v0.3
cross-media linking** seam whose DDL lives only in a reference document — not the identity mechanism,
which is `external_id`.

ℹ️ **Context for the above, deliberately not a design.** MangaBaka is an id hub whose series endpoint
returns a source block spanning several manga databases and which models merges explicitly. A
BookOrbit issue and an open pull request exist for native MangaBaka support — **uncommitted, with no
maintainer signoff**. Worth an amendment if it lands; **never a dependency** (decision clause 4).

🚩 **3. The credential model, which improved but did not become free.** Magic-link auth removes the
password, which is the whole of the falsification in §2 — but §14's rules do not relax. A magic link
is **reusable, optionally non-expiring, capped at 25 per user, superuser-created, and creatable only
for *shared* accounts** (`magic-link.service.ts`). So UsArr would store a **long-lived bearer secret
scoped to an account that is not the owner's personal one**, which is better than a password and is
still a §14 credential: encrypted at rest under the versioned AAD-bound scheme, never logged, never
sent to the browser. **What a shared account can see, and whether that is the right scope for a
catalogue read, is not decided here.**
⚠️ **Deferring it is acceptable only because this ADR ships no adapter and no credential store
reaches production on it — but it is a §14 question, so it is given a named gate rather than an
open end.** *"Not decided here"* must not become *"not decided anywhere"*: **the BookOrbit adapter
may not read a catalogue under a shared-account credential until the scope that account grants has
been enumerated against §14** — specifically whether it confers write or admin reach beyond the
catalogue read UsArr needs, since §14 treats an over-scoped stored credential the same way it treats
an \*Arr API key. **Closes in the adapter thread, before the first credential is stored**, not in a
later pass.

✅ **Discharged 2026-08-19 by [ADR-0058](#adr-0058).** `internal/bookorbit/scope.go` (landed at
`c324cbf`) enumerates BookOrbit's 23-member permission vocabulary against §14 and computes the
verdict on every credential mint at **zero extra cost**, because the login response already carries
`permissions`, `isSuperuser` and `provisioningMethod`. **This is a note and not an amendment:
nothing in the paragraph above is falsified.** The gate named a thing that had to happen before a
catalogue read; the thing happened. A discharged gate is a task finished, not a claim overturned, so
the text above stands exactly as written and is now *satisfied* rather than *outstanding*.
⚠️ **Discharged is not vacated.** The condition ADR-0052 states is on the **catalogue read**, which
belongs to slice 1: slice 0 ships the mechanism slice 1 must consult (`ScopeVerdict.Elevated()`),
not a refusal to connect. If a catalogue read ever lands without consulting it, this gate is open
again.

✅ **The slice-1 half discharged 2026-08-19 at `862a0ca`.** `BookOrbitSource.gate()`
(`internal/libsync/bookorbit.go`) consults `ScopeVerdict.Elevated()` before the first catalogue
read, from **`Containers()` as well as `StreamItems()`**, behind one `sync.Once` — so the library
list is graded before it is read or bound, and not only the item walk. **This is a note and not an
amendment**, on the same principle as the note above: the gate named a thing that had to happen
before a catalogue read, and it has now happened at both entry points. Nothing above is struck.
⚠️ **Discharged is still not vacated.** The obligation is that the catalogue read consults the
verdict — it is not a statement about these two methods. If a future read path lands without
consulting it, this gate is open again.

### Alternatives considered

- **(a) Keep Kavita as v0.1's source and treat BookOrbit as a later adapter.** ⚠️ **The strongest
  alternative, and it was the repository's own standing recommendation** ([`ROADMAP.md`](./ROADMAP.md)
  §3). It has real weight: Kavita's delta is the **only** one this project has verified against a live
  instance, the adapter is built, and [ADR-0046](#adr-0046)'s two-spec contract guard is running. ⚠️
  **Stated at its strongest, because an ADR owes that to what it rejects, and the asymmetry is
  larger than the draft admitted:** against Kavita's *built, probed and contract-guarded* adapter,
  **BookOrbit's is zero lines** — no adapter, no vendored spec, no contract guard, no fixture — and
  the measurements in open question 1 below make the gap worse, not better, since **BookOrbit offers
  no series-ordered read at all** where Kavita's `all-v2` is the very thing ADR-0035 §2a probed
  successfully. A variant of this alternative is sharper still and deserves naming: **ship v0.1 on
  the proven adapter and swap after the replica thesis is proven** — which would cost the owner
  nothing but keeping one container running for one milestone. **It loses on the same sentence
  ADR-0041 turned on: *on real data*.** The owner is sunsetting Kavita, so
  the instance that produced ADR-0035 §2a's run is going away. A v0.1 whose success criterion is
  "proves the replica thesis on the owner's real library" cannot be met against a server he no longer
  runs — which is, precisely, the argument that removed Sonarr and Radarr. ℹ️ And two thirds of the
  evaluation that produced the recommendation is now false (§2), so the recommendation is not being
  overridden so much as outlived.
- **(b) Ship both adapters in v0.1.** Rejected on ADR-0036's rule as [ADR-0041](#adr-0041) clause 2
  kept it — *"prove the replica thesis on real data, on one source, before a second adapter is
  written"*, **count unchanged at one**. This is ADR-0041's own rejected alternative (e) at a different
  size and its reason is unchanged: a milestone that tests two things at once learns nothing clean
  from a failure. `CLAUDE.md`'s *"cut before you add"* points the same way.
- **(c) Wait for BookOrbit's native MangaBaka support, then decide.** Rejected for
  [ADR-0043](#adr-0043) alternative (e)'s reason, and for a sharper one: the support is an **open pull
  request with no maintainer signoff**, so this defers v0.1's largest subsystem behind an event with
  no date and no owner. `CLAUDE.md` names never shipping as this project's biggest risk.
- **(d) Delete the Kavita adapter, since the owner is sunsetting it.** Rejected. It saves nothing —
  the code is written, the contract tests are green and unattended, and `internal/servarr` is the
  standing precedent that displaced work stays ([ADR-0041](#adr-0041) clause 3). It also costs a
  working experience for any user who runs Kavita, which principle 3 exists to protect. **Deletion is
  the one reading of "sunset" this ADR forbids** (clause 2).
- **(e) Write "full resync, no delta channel" into the decision, as briefed.** Rejected because the
  primary source refutes it (§2's 🚩). It would have foreclosed channel 3b — v0.1 work under
  [ADR-0041](#adr-0041) — on a false premise, and a foreclosure is far more expensive to undo than an
  open question is to close.

### Consequences

**What changes:**

- **[`ARCHITECTURE.md`](./ARCHITECTURE.md) §16.1's v0.1 entry names BookOrbit**, and is edited in this
  same change, because §16 is scope authority and must not disagree with an accepted ADR.
- **[ADR-0041](#adr-0041) is annotated** per the preamble's four marks — index row, `Status:` line, a
  dated block, and an inline flag at clause 1. **No sentence of its body is reworded.**
- **§7.1a gains a BookOrbit row when the probe runs**, and not before. Kavita's row is a **dated
  record** and is not touched; `DEVELOPMENT.md` §11 is explicit that a citation inside a dated record
  is history rather than staleness.

- 🚩 **[ADR-0050](#adr-0050)'s second reopening trigger loses its stated ground and is RE-DERIVED
  here rather than silently inherited. ADR-0050 is not reopened; this is an acknowledgement.** That
  ADR (stdlib JPEG base, AVIF deferred) relocated its risk from *output* to *input* on a ground it
  names explicitly: *"v0.1's catalogue source is Kavita (§16.1, ADR-0041), and Kavita's Admin settings
  → Media → **Save Media As** selects the format it writes covers … PNG (the default), WebP, or
  AVIF"*, concluding *"the shipped adapter serves a stdlib-decodable format **by default and by
  default only** — one admin toggle produces covers this binary cannot decode."* **Decision clause 1
  voids that ground.** Re-derived against BookOrbit on 2026-08-19, **the trigger does not merely
  survive — it fires harder, and the mitigating half of ADR-0050's sentence is gone.** BookOrbit
  stores a book's full-size cover under the extension its **own magic-byte sniffer** returns —
  `imageExt()` recognises `png`, `jpg`, `gif`, **`webp`** and **`bmp`**
  (`server/src/modules/metadata/lib/cover.ts:17-36`), and `cover.service.ts:258` writes the cover as
  `${COVER_CUSTOM_FILE_PREFIX}${ext}` — so the served format is **whatever was embedded in the user's
  EPUB or CBZ, preserved verbatim**. `image/gif` and `image/png` are stdlib; **WebP and BMP are not**
  (`x/image/webp` is decode-only, `x/image/bmp` is a dependency either way). ⚠️ **So where Kavita's
  risk was an admin checkbox that defaults to safe, BookOrbit's is not a setting at all** — a single
  CBZ carrying a WebP cover produces a cover this binary cannot decode, with no toggle that prevents
  it. ✅ **Thumbnails are unaffected**: `generateThumbnail()` always re-encodes to JPEG q90
  (`cover.ts:38-40`). **The decision ADR-0050 made still stands** — the output codec is still JPEG and
  still free — and what it already owes is unchanged and now more clearly owed: *"a decode-failure
  path that says which format it could not read."* **No codec is re-decided here.**

- ⚠️ **The adapter reads covers from BookOrbit's `/api/v1` routes only, and that constraint is
  load-bearing rather than stylistic:** those routes sit behind the global `JwtAuthGuard`
  (`server/src/app.module.ts:164`), whose strategy takes the credential from the `Authorization`
  header or a cookie and **never from the query string**
  (`server/src/modules/auth/jwt.strategy.ts:15`); the `?t=` they accept is a **cache-buster** that
  only selects a `Cache-Control` value (`server/src/modules/book/book.controller.ts:335-348`); and
  the HMAC-token-in-URL shape is confined to the **OPDS** surface the adapter does not use —
  `createHmac` has exactly one non-test call site in the repo,
  `server/src/modules/opds/opds-auth.guard.ts:24`, and it is that guard, not the `/api/v1` one, that
  reads a credential out of `?t=`. **That is the escape from the exact class LS-260's question 1
  measured on Kavita**, where the header arm returned 400 and only `?apiKey=` in the query returned
  200 — forcing a full-admin credential into the URL and thence into every upstream access log.
  (BookOrbit `73b7877`.)

**What does not change:**

- **The schema.** Six-type from migration 0001, three subtype tables from `00006_kavita_subtypes.sql`.
  ⚠️ **The migration keeps its name**, because a merged migration is never edited and renaming one
  would be worse than a stale filename. The tables it creates are `work_book`, `work_comic` and
  `work_comic_issue` — none of them Kavita-specific.
- **Prowlarr Search-and-Grab**, which is v0.1's request path for all six media types and touches no
  catalogue source.
- **[ADR-0043](#adr-0043) and [ADR-0044](#adr-0044).** Both were written to turn on a *property* of
  v0.1's source rather than on its name — ADR-0043 on *"v0.1's catalogue source has weak identity"* —
  which is exactly why they survive a source swap. ADR-0043's stated reopening condition is *"a v0.1
  source that supplies strong external ids for the ordinary user"*, and BookOrbit is **not** that at
  the series level: what it supplies there is what the user recorded, which UsArr writes at 0.90 and
  never at 1.0 — the same grade Kavita's series ids earn (open question 2). **Neither is reopened**,
  and the *"minimal match-correction UI"* ADR-0043 moved earlier is, if anything, better justified.
- **[ADR-0045](#adr-0045)'s v0.2**, and §16.1's post-v0.1 sequence — Navidrome, Audiobookshelf, Komga,
  in that order. This ADR touches none of it.

**Documents that now assert a stale fact, listed rather than swept.** ⚠️ **This ADR fixes §16 only.**
The rest are named here so the next pass does not have to re-derive them, and because `CLAUDE.md`'s
*"no invented status"* cuts both ways — pretending they are already fixed would be the same fault:
🚩 **`README.md`, and its entry is corrected here because the draft's was wrong in a way that
would have caused it to be skipped.** The draft read *"its status tables are generated from §16 and
will regenerate"*. **Nothing regenerates them.** There is no `README` make target, no `scripts/`
directory, and no file in the tree references `README.md` programmatically
(`grep -rln "README.md" --include=*.go --include=*.mjs --include=*.js --include=Makefile .` → no
match). `CLAUDE.md`'s *"generated from"* describes **provenance, not a mechanism**. The tables are
hand-written, they name Kavita as v0.1's source in **at least eight places** (`README.md:7, 8, 22, 69,
70, 72, 80, 92`), and after this change the repo's **front door** contradicts its own scope authority
until someone edits it by hand, [`ROADMAP.md`](./ROADMAP.md)
§1, §3 and §4, [`FUTURE.md`](./FUTURE.md), [`DEVELOPMENT.md`](./DEVELOPMENT.md) **§2**'s tree comment
(at line 105 — the draft cited *"§105"*, which reads as a section number and there is no §105; the
file has twelve sections),
[`design/DESIGN-DIRECTION.md`](./design/DESIGN-DIRECTION.md) and `design/mockups/README.md` — the last
two carry `v0.1: Kavita, Prowlarr` as a rendered string in the mockups, which is a design-asset change
rather than a prose one and is the largest of them.

> ✅ **DISCHARGED 2026-08-19 by `a1995f9`** — the sentence directly above **stands as written** and is
> not struck: it was true the day it was written, nothing it claims has been falsified, and only its
> **status** moved, from owed to done. The re-draw landed `v0.1: BookOrbit, Prowlarr` across the five
> screen pages, the generated `prototype.html` and `design/mockups/README.md`; `DESIGN-DIRECTION.md` §9.6, whose quotation
> deliberately tracked the asset rather than leading it, followed. Recorded because this paragraph's
> own reason for existing — `CLAUDE.md`'s *"no invented status"* cutting both ways — makes a list
> still saying *owed* after the work landed the same fault as one saying *done* before it did. The
> rest of the list is untouched and stays owed.
>
> **Only this mark is owed, and the preamble is why.** *"How an ADR is amended when the world moves
> under it"* owes three marks always and a fourth where needed, but all four are shaped for a claim
> that **stopped holding**: marks 1 and 2 flag the index row and the `Status:` line *"so a reader
> arriving at the anchor is warned before the first section"*, and mark 3 names *"which claims below
> no longer hold and which survive"*. **No claim of this ADR stopped holding**, so those three would
> announce an amendment a reader would then hunt for and not find — and each is written to carry
> *"amended … by ADR-NNNN"*, while *"the decision lives in the superseding ADR"* and **no ADR
> supersedes anything here**; a commit discharging listed follow-up work is not one. What is owed is
> mark 4, *"a dated inline flag wherever a reader could otherwise take it as live, since anchors and
> search land people mid-document"* — a reader landing on that sentence would take a design-asset
> change as still outstanding. That is this blockquote, and it is the whole amendment.

**What would reverse this.** The owner changing his mind about his own stack, which is his to change;
or the §2 probe failing so badly that BookOrbit cannot support even reconciliation-only honestly —
which §7.1a says is a **surfaced** state rather than a blocking one, so this bar is high.

### What is built

**Nothing.** This ADR ships no code by design: it **gates** the adapter rather than being written
alongside one, so that the decision is reviewable before an implementation makes it expensive to
revisit. What lands with it is this record and the §16.1 edit that keeps scope authority consistent
with it.

---

<a id="adr-0053"></a>
## ADR-0053 — All six media types are always in the sidebar; per-type hiding is closed until a facet read exists

**Status:** Accepted · **2026-08-19** · **Amends [ADR-0027](#adr-0027)** — its final consequence,
*"A type with zero items is not rendered anywhere — sidebar, home, or search group"* — **for the
sidebar and for nothing else** · **Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §17.2** and
`design/DESIGN-DIRECTION.md` §8.1 · **Adds no endpoint, no migration, no column and no backend
change** · **Reopens on exactly one condition: a per-type facet read on
[`reference/http-api.md`](./reference/http-api.md)** · ⚠️ **reopening condition refined 2026-08-19,
citing [ADR-0059](#adr-0059)** — see the block below

> ### ⚠️ Refinement, 2026-08-19 — the condition is REFINED, not discharged
>
> **The decision is untouched. All six media-type entries still render unconditionally, no row
> carries a count, and `TYPE_NAV` still maps six with no predicate.** What moves is the reopening
> condition below, which was about to be discharged by the wrong read.
>
> `GET /api/v1/library/facets` shipped ([ADR-0059](#adr-0059), `reference/http-api.md` §8), and it
> looks like the read this ADR named. **It is not, and the gap is invisible until a two-format
> book.** The facets read answers *how many works are **bucketed** to each type*; every book lands
> in exactly one bucket, so a book held as both an EPUB and an M4B counts under **Ebooks** and under
> nothing else. Discharging this condition with it would hide **Audiobooks** from a user who has
> audiobooks — the silent drop this ADR's own rejected alternative is written against, arriving
> through a read that looks like compliance.
>
> **So the condition now reads:** a predicate answering **whether each of the six types has
> content** — the independent `EXISTS` over `edition.format` of ARCHITECTURE §17.2 rows 4–5, which
> `ix_edition_format` (migration `00009`) already serves — **and not a read returning per-type
> counts.** A count is not that predicate and cannot be substituted for it.
>
> Nothing else in this ADR moves: the Alternatives, the Consequences and the seam (*"one predicate
> on `TYPE_NAV`"*) all stand as written.

### Context

ADR-0027 made media type the navigation axis and specified the entries as **data-driven**. §17.2's
axis table says *"one sidebar entry per type **that has content**"*; its hard rule says a type the
user does not have is *"**not shown at all** — not in Block A, not in the sidebar, not as a search
group"*; and `design/DESIGN-DIRECTION.md` §8.1 repeated both, drew a sidebar mock with a per-type
count beside every row, and added *"Nothing hard-codes a type"*.

**Nothing UsArr serves can answer "which types have content".** `reference/http-api.md` §7.1 is
explicit — *"There are **no facet counts** beside the chips; each is its own aggregate and its own
read"*. `GET /api/v1/library` takes a `media_type` and returns a keyset page, which answers the
question only for the one type asked and only by fetching rows; `GET /api/v1/library/recent` returns
`{items, limit, next_cursor}` and carries no per-type facet; `GET /api/v1/services/health` carries a
`role` whose CHECK admits four values — `library|acquisition|indexer|download_client` — and no
media-kind array. There is no read that answers presence, and none that answers a count.

So the rule as written specifies a screen the wire cannot serve — and the sidebar still has to render
something on every install today.

### Decision

> **All six media-type entries render in the sidebar, unconditionally, in one fixed order, on every
> install — including the types this install has nothing in. No sidebar row carries a count. A type
> that is empty says so on its own screen, in words that name which of the reasons it is.**
>
> **Per-type hiding is closed, not abandoned, and it reopens on one named condition: a read that
> answers which of the six types have rows — under the current scope — in one statement.** ⚠️ **This
> sentence is REFINED as of 2026-08-19 — see the Refinement block under `Status:`, and read it before
> treating any read as satisfying this.** *"Have rows"* means **has content**, an existence question
> over `edition.format`; the per-type **counts** on `GET /api/v1/library/facets` are a different
> answer and do not discharge it ([ADR-0059](#adr-0059)). Until such
> a read exists on `reference/http-api.md`, re-specifying *"one entry per type that has content"* is
> specifying a screen the wire cannot serve, and this ADR is the reason a future pass should not
> "restore" it as though it had been dropped by accident.

### Alternatives rejected

- **Probe six counts on every navigation.** Six aggregates on the render path is what principle 1
  exists to refuse. ⚠️ It is **not** rejected as unaffordable: §13 already prices this shape, budgeting
  a scope-chip toggle of *"1 keyset page + 6 sidebar `COUNT(*)` over `library_member ⋈ work`"* at
  **< 15 ms p50**. It is rejected as **premature** — the budget prices a read nobody has designed, and
  nothing yet decides whether the counts ride the browse response or take their own endpoint. That is
  the decision the facet read will have to make, and it is deliberately left to it rather than
  pre-empted here.
- **Hide a type on a count nobody measured** — let the sidebar infer presence from whatever rows it
  happens to have loaded. This is the worst of the four, and it is worth naming because it is the one
  that looks like compliance. Its failure is **silent**, and it lands in exactly the place a user
  looks for the missing thing: a library that is really there becomes unreachable from the nav, with
  no empty state to explain it, because the row that would have carried the explanation is the row
  that was removed. A wrong row costs one click; a missing row is not visible at all.
- **Ship no media-type entries until the read exists** — the tree's own earlier answer, still
  recorded in `web/src/lib/home.ts`'s header. It honours the letter of the hard rule, since nothing
  hidden is wrongly shown, at the cost of §16's entire v0.1 navigation: six real routes exist under
  `/library/[type]` and nothing in the shell links to them. *"Not shown at all"* was written to stop
  an empty row promising content it does not have. It was not written to stop the application having
  navigation.
- **Keep the count column and fill it in lazily.** A column blank on first paint that fills per row is
  the six aggregates again, with layout shift added.

### Consequences

- **The empty state carries the honesty the sidebar can no longer carry**, and it is a real state
  rather than a blank table. `browseEmptyState` (`web/src/lib/librarygrid.ts`) words it from the
  services read, separating *"no library-bearing service is connected"* from *"this type has no rows
  yet"* from *"the library scope excludes everything"* — so the user who clicks an empty type is told
  which of those is true, which is strictly more than a hidden row tells them.
- **The hard rule survives untouched for Block A and for search groups.** Neither is contradicted:
  Block A's per-type rollup is not built at all, and is blocked on this same missing read; a search
  group's row count is a property of the response the screen already has, so §17.4 rule 1 stays
  buildable. This ADR amends the sidebar clause and no other.
- **§17.2's scope-vs-shape paragraph keeps its answer and loses its mechanism.** Scoping to Comics
  still does not remove the `Movies` row — but *"only the counts narrow"* no longer describes how,
  because there are no counts. The shape half is now unconditional, which is the simpler half of what
  it said.
- **The seam is one line wide.** `TYPE_NAV` in `web/src/routes/+layout.svelte` is a `MEDIA_TYPES.map`
  with no predicate; the day a facet read lands, adding the predicate is that one line. The order,
  the labels and the six routes all stay. Reversing this decision is cheap **by construction**, which
  is why closing it now costs little.

### What is built

`web/src/routes/+layout.svelte`: `TYPE_NAV` maps `$lib/library`'s `MEDIA_TYPES` — the six-member
`as const` that is §17.2's own order — with no predicate, and the file header carries this argument
where an implementer meets it rather than only here. The empty states are `browseEmptyState` in
`web/src/lib/librarygrid.ts`, rendered by `web/src/routes/library/[type]/+page.svelte`. **There is no
backend half:** this ADR removes a requirement from the wire, and adds none.

---

<a id="adr-0054"></a>
## ADR-0054 — The search response publishes a per-hit relevance `score`, and the ORDER stays the contract

**Status:** Accepted · **2026-08-19** · **Amends [`reference/http-api.md`](./reference/http-api.md)
§6.2** — its *"No score is published"* clause, and nothing else on that endpoint ·
**Unblocks [`ARCHITECTURE.md`](./ARCHITECTURE.md) §17.4 rule 2 and rule 4** · **Adds no migration,
no column, no query and no upstream call** · **Does not change the ranking, the retrieval, or the
order any caller receives**

### Context

§17.4 rule 2 specifies the grouped Search screen: *"Group order is by the group's best-scoring hit,
descending — not a fixed type order — and it is computed once per query and then frozen."* Rule 4
puts a cross-media linked work *"in the group of its highest-scoring medium"*. Both are comparisons
between the **best hit of one group and the best hit of another**.

`reference/http-api.md` §6.2 answered with the opposite: *"**`items` is ordered by relevance and the
ORDER IS THE CONTRACT.** No score is published. The score is normalised per query, so a client
comparing one across two queries would be comparing nothing; publishing it would also freeze §6.6's
ranking, which is expected to change."* `internal/httpapi/librarysearch.go` carried the same
sentence in its own words.

**Neither rule can be built over an ordering alone, and the reason is not a limitation of effort.**
An ordinal position is a *rank*, and a rank carries no magnitude: `items[3]` being the best hit of
the Movies group and `items[7]` being the best hit of the Ebooks group tells a client that Movies
led, and nothing whatever about whether it led by a hair or by a mile. A grouped screen ordered on
that has no way to distinguish *"the album hit is unambiguous and the two film rows are thin"* —
§17.4's own worked example, and the case it says the six-type grouping exists for — from the reverse.
`ROADMAP.md` recorded this as an open item and noted that resolving it *"closes off an alternative
and needs an ADR"*.

The store had the number all along. `rerank` (`internal/store/searchlibrary.go`) computes a score per
candidate, sorts on it, and then **threw it away** when it copied the hits out. Nothing had to be
computed, queried or measured to publish it; it had to be *specified*.

### Decision

> **The search response publishes `score` on every item: the re-rank's own output, a weighted sum of
> three signals each normalised over the candidate set of that one answer, in `(0, 1]`.**
>
> **The ORDER remains the contract, and the order is deliberately NOT score order.** The media-type
> diversity injection is a promotion and not a re-score, so a promoted row sits above better-scoring
> rows carrying the lower number it earned. A client that re-sorts by `score` undoes that guarantee
> and produces a worse list than the one it was handed.
>
> **The misuse the field was withheld to prevent is answered by documentation and by tests instead
> of by absence.** `reference/http-api.md` §6.2.1 states the formula, a comparability table with
> seven rows, **two** permitted uses and **five** forbidden ones, each with the mechanism that makes
> it wrong rather than a general caution.

### Alternatives rejected

- **Keep withholding it, and group server-side.** The honest alternative, and it was close. It fails
  on scope and on sequencing: server-side grouping needs `work_relation`, whose table **does not
  exist** — it is v0.3, no shipped migration creates it, and `TestDeferredTablesAreAbsent` fails the
  build if one appears. So this alternative does not defer the screen by a commit, it defers it by
  two milestones, and it would replace a flat read that meets §13's `p50 < 15 ms` budget with a
  grouped one that has never been priced. **Cut before you add** points the same way: this ADR adds
  a field to a struct, that one adds a subsystem.
- **Amend §17.4 rule 2 to a fixed type order.** Cheapest of all, and it deletes the finding the rule
  was written from: with six types a fixed order *"buries an unambiguous album hit under two thin
  film rows"*. It also re-opens the ranking-bias problem §17.4 cites Sushmita et al. for — the whole
  argument for grouping is that relevance across a film and an audiobook is not comparable, and a
  fixed order does not avoid making the comparison, it makes it badly and invisibly.
- **Publish a coarse bucket** — `high` / `medium` / `low`, or a 1–5 star. Strictly worse than the
  number on both counts. It reads as an absolute judgement of match quality, which is the one thing
  the value is not, and it destroys exactly the resolution rule 2 needs: the ordering of two groups
  whose best hits fall in the same bucket becomes arbitrary again.
- **Publish the RRF value, or the BM25 rank, instead.** Both are further from what the screen needs
  and closer to what a consumer must not have. The RRF sum is dominated by whether *both* retrieval
  legs matched a row, so publishing it puts a leg's fingerprint on the wire — the association
  ARCHITECTURE §8.2 states as a negative and keeps inside fusion. A BM25 magnitude is not on a
  common scale between the `unicode61` and `trigram` legs at all.
- **Round it, to discourage over-reading.** Rounding manufactures ties the server did not have, and
  the single comparison §6.2.1 permits — one hit against another, in one response — is precisely the
  comparison a tie destroys. The digits are noise; they are not *harmful* noise.

### Consequences

- **The item key set is no longer identical to Home's.** §6.2 said the keys were §1.3's keys so one
  row component renders both screens; it now says *"§1.3's item keys plus `score`"*. The row
  component is unaffected — it ignores the field, which is read by the grouping layer above it.
- **`score` is a security-relevant field, and is documented as one.** Because it is normalised over
  the candidate set, a document that could enter that set from outside the caller's access scope
  would make the number an **existence oracle**: a client could watch a visible work's score move and
  learn that something it may not see matched its query. Scope is enforced *inside* the retrieval
  legs, before ranking, which is what makes the field safe — and
  `TestSearchScoreIsBlindToWhatTheCallerCannotSee` fires when the filter is moved to a post-filter,
  a refactor that returns the same rows and the wrong numbers.
- **§6.6's ranking is not frozen by this.** §6.2 feared that publishing a score would fix the
  algorithm. What is published is a *within-answer relative* figure with an explicit "not comparable
  across versions" row in its table, so changing the weights changes no documented promise. What
  would be frozen is a score a client was invited to threshold or persist — which is why those two
  are forbidden uses rather than discouraged ones.
- **The three signals behind it are still three, not five.** `popularity`, `title_idf` and
  `in_library` remain hardcoded `0`/`1` in the writer with no reader (§6.6). Publishing the blend
  does not imply the blend is finished, and §6.2.1 says the weights are *chosen, not tuned*.
- **The grouped screen is now buildable and is not built here.** This ADR ships the wire piece only.

### What is built

`internal/store/searchlibrary.go`: `SearchHit.Score`, written by `rerank` onto the hit itself so it
rides through the sort and through `diversify`'s promotion. `internal/httpapi/librarysearch.go`:
`searchHitResponse.Score`, on the hand-built allowlist, forwarded unrounded and unconditionally.
`reference/http-api.md` §6.2 and the new §6.2.1 are the contract. The guards are
`TestSearchScoreIsTheWeightedSumOfTheLiveSignals`, `TestSearchScoreIsBoundedAndPositive`,
`TestSearchScoreDoesNotDependOnTheLimit`, `TestSearchScoreIsBlindToWhatTheCallerCannotSee` and
`TestSearchOrderIsNotScoreOrder` in `internal/store`, and
`TestSearchScoreCrossesTheWireOnTheRightRow`,
`TestSearchScoreDoesNotDescribeAnythingButTheHit` and the amended
`TestSearchResponseKeysAreTheAllowlist` in `internal/httpapi` — each one fired by neutering what it
protects and watching it go red.

---

<a id="adr-0057"></a>
## ADR-0057 — The circuit breaker is one package with an injected open sentinel, not a copy per client

**Status:** Accepted · **2026-08-19** · Landed with the BookOrbit adapter's slice 0 at `c324cbf` ·
**Executes a standing instruction the tree already carried**, in `internal/kavita/breaker.go`, at
exactly the condition that instruction named · **Closes off each client keeping its own copy of the
state machine** · **Adds no dependency, no migration, no configuration key and no behaviour change**
· **Breaks no exported API** — `internal/kavita` and `internal/servarr` keep every exported name ·
**Does not touch ARCHITECTURE §7.5's tuning**, which is the thing a shared breaker most plausibly
puts at risk

### Context

`internal/servarr` wrote the per-instance circuit breaker first. `internal/kavita` needed one and
**copied it verbatim** rather than importing it, for one specific reason: `Allow()` returns the
package's own `ErrBreakerOpen`, and callers match on it with `errors.Is`. Importing
`internal/servarr` from `internal/kavita` would have put the string `servarr: circuit breaker open`
inside a Kavita error, in a package whose `doc.go` declares it *"UsArr's single client for the \*Arr
family"* — and Kavita is not in that family.

That copy was made deliberately and it **wrote down the condition for undoing itself**, verbatim in
`internal/kavita/breaker.go`:

> *"The seam for removing the copy is clean and is worth taking the first time a **THIRD** client
> needs one: lift this file into `internal/breaker` with an injected sentinel, and have both
> packages wrap it. **Two copies is cheaper than a package that exists to serve two callers; three
> is not.**"*

`internal/bookorbit` is the third client. So the question this ADR answers is not *"should the
breaker be shared"* in the abstract — the tree answered that in advance, with a trigger — but
whether the trigger has fired and whether the lift is taken now or one commit later.

### Decision

> **The state machine lives once, in `internal/breaker`, and the open sentinel is a constructor
> argument.** `breaker.New(cfg, openErr, now, rnd)` takes the error each client wants `Allow()` to
> return, so `errors.Is(err, kavita.ErrBreakerOpen)`, `errors.Is(err, servarr.ErrBreakerOpen)` and
> `errors.Is(err, bookorbit.ErrBreakerOpen)` all keep meaning what they meant. A nil sentinel
> **panics** at construction, because a nil there would make an open breaker indistinguishable from
> a closed one at every call site.
>
> **`internal/kavita` and `internal/servarr` keep their exported names as type aliases** —
> `BreakerState`, `BreakerConfig`, `Breaker`, `BreakerClosed`/`BreakerOpen`/`BreakerHalfOpen`, and
> `NewBreaker` — so nothing outside those two packages changes. **The clients that assert the
> ARCHITECTURE §7.5 defaults keep that assertion in their own package** rather than delegating it to
> the shared suite, which is why `withDefaults` becomes the exported `Config.WithDefaults`.
>
> **The lift is taken at the trigger, not after it.** Writing a third verbatim copy and lifting
> later is the alternative this closes.

### Alternatives rejected

- **Write a third verbatim copy in `internal/bookorbit`.** This is the alternative the decision
  exists to close, and it is not a straw man — it is the *cheapest single commit* of the three, and
  it is exactly what the first two clients did. It loses on the condition the tree already set: the
  drift risk is not linear in the number of copies, because the copies are held consistent by
  **three separate defaults tests** that a reviewer must notice are meant to agree. Two copies were
  priced and accepted; a third was priced in advance and refused.
- **Have `internal/bookorbit` import `internal/servarr`'s breaker.** The same error-identity problem
  that produced the first copy, one package further along: `bookorbit` failures would report as
  `servarr:` errors, and `internal/libsync` matches `kavita.ErrBreakerOpen` with `errors.Is` in
  production (`credits.go`, `files.go`) to distinguish "the request never left the process" from a
  real upstream failure — so the two families would become confusable in the place the distinction
  is actually read.
- **Return a shared `breaker.ErrOpen`, and have each client wrap it in its own sentinel.** This is
  the closest rejected alternative and it *nearly* works. It fails on two counts. First, the error
  message goes service-agnostic — `breaker: circuit breaker open` names no service, and the whole
  reason the copy existed was that a Kavita failure must not read like a `servarr` one. Second, it
  preserves per-client `errors.Is` **only if every client remembers to wrap**, and a client that
  forgets produces an error that matches nothing and reads as an ordinary upstream failure. The
  constructor argument makes the sentinel impossible to forget: `New` panics without one.
- **Lift the breaker and delete the per-client defaults tests**, keeping one in `internal/breaker`.
  Cheaper and worse. The §7.5 numbers are the thing a shared implementation most easily drifts on,
  and the value of three suites failing rather than one is that a change made for one client's
  convenience cannot land quietly.

### Consequences

- **The measured ledger is a deduplication, not a net line saving, and this ADR states the numbers
  rather than the headline.** The numbers below are `c324cbf`'s, slice 0's content commit,
  measured with `git show --numstat c324cbf` and not with the merge `568ddbc` that carried it.
  `c324cbf` deletes **347** lines: **177** from
  `internal/kavita/breaker.go`, **169** from `internal/servarr/breaker.go` — that is the duplicated
  state machine, twice — and **one** from `internal/kavita/client_test.go`. The two client files go
  from **202** and **192** lines to **52** and **49**: 394 lines of duplicated state machine become
  101 lines of alias wrapper, **53** of them newly written and most of those the comments explaining
  why the wrapper exists. Against that, the new package costs **245** lines plus a **103**-line test.
  Counted honestly, **the tree is not smaller**. What is bought is **one** state machine to reason
  about, review and fix instead of two — which is what the original instruction asked for, and it
  never claimed a line saving.
- **`internal/releases` and `internal/libsync` were untouched, and that is the aliases' whole
  return.** Both consume breakers: `internal/releases` builds a per-indexer breaker through
  `servarr.NewBreaker` (`service.go`), and `internal/libsync` matches `kavita.ErrBreakerOpen` with
  `errors.Is` (`credits.go`, `files.go`). ⚠️ **One inherited comment overstates this and is worth
  correcting here rather than repeating:** `internal/servarr/breaker.go` says *"`internal/releases`
  matches on it with `errors.Is` to render `OutcomeBreakerOpen`"*, and it does not — `runLeg` treats
  any non-nil `Allow()` as breaker-open without inspecting the sentinel, and the only `errors.Is`
  against `servarr.ErrBreakerOpen` in that package is in `grab_outcome_test.go`. The sentinel
  argument still holds on `internal/libsync`, which does match in production. Neither package appears
  in the commit's diff. Verified against the tree:
  `git show --numstat c324cbf` — slice 0's content commit, not the merge `568ddbc` that carried
  it — shows five Go files touched outside `internal/bookorbit`, all of them in
  `internal/breaker`, `internal/kavita` or `internal/servarr`.
- **The only test change outside the new package is one line.** `internal/kavita/client_test.go`'s
  `TestBreakerDefaultsMatchTheArchitectureNumbers` calls `BreakerConfig{}.WithDefaults()` where it
  called `.withDefaults()`; three comment lines were added beside it saying why the method is
  exported now. `internal/servarr/breaker_test.go` — five tests including the jitter bound and the
  independence check — was **not touched at all** and passes unchanged against the shared
  implementation, which is the strongest available evidence that the lift is behaviour-preserving.
- **`Config.WithDefaults` is exported for a reason that is not aesthetic.** The §7.5 numbers must be
  pinned from the client packages, and an unexported method is unreachable from there. The export is
  the price of keeping three suites honest rather than one.
- **`internal/breaker` is not a general-purpose resilience package and must not grow into one.** It
  is a per-service-instance circuit breaker with one tuning, ARCHITECTURE §7.5's. Retries, hedging,
  rate limiting and bulkheads are not owed a home here on this ADR's authority.
- **A breaker is per instance and never global**, and centralising the implementation does not
  centralise the *instances*. One Kavita being down must not stop a Prowlarr grab; a second Kavita
  counts its own failures; each Prowlarr indexer fan-out leg keeps its own. Nothing in this change
  creates a shared instance, and a future package-level singleton would be a reversal of this ADR,
  not an extension of it.

### What is built

`internal/breaker/breaker.go` — `State`, `Config`, `Config.WithDefaults`, `Breaker`, `New`, `Allow`,
`Success`, `Failure`, `State()`, `RetryAt()`. `internal/breaker/breaker_test.go` —
`TestAllowReturnsTheInjectedSentinel`, `TestNewRefusesANilSentinel`,
`TestDefaultsAreTheArchitectureNumbers`, `TestHalfOpenAdmitsExactlyOneProbe`,
`TestBackoffDoublesAndCaps`. `internal/kavita/breaker.go` and `internal/servarr/breaker.go` are now
alias wrappers whose file comments carry this argument where an implementer meets it.
`internal/bookorbit` constructs its breaker through the same package with its own sentinel. The
pre-existing `internal/servarr/breaker_test.go` and `internal/kavita/client_test.go` breaker tests
are the regression evidence, and they were run against the shared implementation unchanged but for
the one line above.

---

<a id="adr-0058"></a>
## ADR-0058 — UsArr grades the scope a stored service credential actually carries, and reports rather than refuses

**Status:** Accepted · **2026-08-19** · Landed with the BookOrbit adapter's slice 0 at `c324cbf` ·
**Discharges [ADR-0052](#adr-0052)'s §14 credential-scope gate** — ⚠️ **a discharge is not an
amendment**: ADR-0052 gains a dated inline note pointing here, **nothing in its text is struck**,
and none of the three amendment marks is owed · **Closes off assuming a service account is minimal
because it was created as a service account** · **Adds no migration, no column, no crypto and no new
HKDF label** — the stored secret rides the existing versioned AAD-bound `service_instance` envelope
· **Costs zero extra requests** · **Ships the mechanism the gate will be enforced with, not the
enforcement** — that is slice 1's · ✅ **Slice 1's enforcement LANDED 2026-08-19 at `862a0ca`** —
`BookOrbitSource.gate()` consults `ScopeVerdict.Elevated()` from **`Containers()` as well as
`StreamItems()`**, behind one `sync.Once`; **this is a note and not an amendment**, on the same
principle as the discharge notes below — the clause before it describes what *this* ADR shipped and
stays true as written — and ⚠️ **discharged is not vacated**: the obligation is on the catalogue
read, so a future read path landing without consulting the verdict makes it live again

### Context

ADR-0052 left the credential question as a **named gate** rather than an open end: the BookOrbit
adapter *"may not read a catalogue under a shared-account credential until the scope that account
grants has been enumerated against §14 — specifically whether it confers write or admin reach beyond
the catalogue read UsArr needs"*, closing *"in the adapter thread, before the first credential is
stored"*. This ADR is the answer to it.

The §14 rule the gate invokes is the one the project applies to \*Arr API keys: an over-scoped
stored credential is treated as a full-admin credential, because that is what it is. BookOrbit's
magic-link auth removes the *password* from the picture, which is real, but it does not tell UsArr
what the account behind the link can **do**.

**The enumeration was done against source** (`bookorbit/bookorbit@73b7877d`), and it found that the
correct credential is a shared account with an **empty** permission set and an explicit `libraryIds`
grant: authorization is three ordered `APP_GUARD`s, permissions are additive and default to none,
and every route this adapter needs declares no `@RequirePermission`. That is a good answer — and it
is exactly the kind of answer that rots. **A paragraph saying "an empty permission set is
sufficient" cannot notice when BookOrbit adds a 24th permission.**

### Decision

> **UsArr grades the permissions its stored credential actually carries. It does not assume a
> service account is minimal because someone created it as one.** All **23** members of BookOrbit's
> `Permission` enum are classified in code as either **elevated** — write or admin reach beyond a
> catalogue read, which is the exact test §14 applies and the exact words ADR-0052's gate uses — or
> **unneeded**: harmless, but more than a catalogue replica uses. The superuser flag, a
> `provisioningMethod` other than `shared`, and an inactive account are **separate findings**, not
> permissions.
>
> **An unrecognised permission grades ELEVATED. Never harmless.**
>
> **The client reports and warns. It does not refuse.** The verdict is computed on every credential
> mint, exposed through `Client.Scope()`, returned by `Client.Authenticate`, and logged at WARN with
> the account name and the findings — never the token.
>
> **The gate ADR-0052 states is on the catalogue read**, so slice 1 must consult
> `ScopeVerdict.Elevated()` before it reads — at **`Containers()` as well as `StreamItems()`**, not
> at the item walk alone. `Containers()` performs `GET /api/v1/libraries` and it runs **first**, and
> **a container list is catalogue data, not plumbing**: it is what the Libraries screen renders and
> what `BindContainers` writes rows from. Gating only the item walk would read and bind a library
> list under a credential whose scope had never been graded, which is the exact thing the gate
> exists to prevent. One `sync.Once` makes the wider gate free — one `Authenticate` per source
> either way. Slice 0 ships the thing both reads will consult.

### Alternatives rejected

- **Document the required scope and trust the operator to configure it.** This is the option the
  enumeration itself produced, and it is why the decision is worth writing down: the finding
  *"an empty permission set is sufficient"* is true and is not self-checking. It fails silently in
  both directions — an operator who pastes a superuser token gets no signal, and an upstream that
  grows a permission gets no signal either.
- **Refuse to connect on an elevated credential.** Superficially the stricter, safer choice, and it
  is the one this ADR most deliberately turns down. Refusing leaves the operator with a service that
  will not talk to them and **no visible reason why** — the exact opposite of principle 3's *"says
  what is missing and why, rather than rendering an empty screen that looks broken"*. It also makes
  the §14 finding **less** visible, not more: a connection that fails is indistinguishable from ten
  other causes, whereas a connection that works and carries a warning puts the finding in front of
  the person who can fix it. And it would be enforcement in the wrong place — ADR-0052's condition
  is on the catalogue read, not on the handshake.
- **Spend a request on `GET /users/me` (or equivalent) to enumerate scope.** Unnecessary. The
  magic-link login response already carries `permissions`, `isSuperuser` and `provisioningMethod` in
  the same body as the `accessToken`, so the verdict is free. A verdict that costs a request would
  have to be computed rarely, and one computed rarely goes stale against an account whose
  permissions changed.
- **Grade only the permissions that are obviously dangerous, and ignore the rest.** This collapses
  into "unknown is harmless" for everything the classifier has not thought about, which is the
  default the next decision closes.
- **Treat an unrecognised permission as harmless (`unneeded`) until someone classifies it.** The
  quiet default, and the one that looks like nothing happening. See the Consequences.

### Consequences

- **`unrecognised → ELEVATED` is a decision, and it is where the design pays for itself.** It was
  **chosen**, not fallen into. The alternative — treating a permission this build has never heard of
  as harmless — is the one that produces no output, no test failure and no log line on the day
  BookOrbit ships a 24th permission, which is precisely the day the classification was built for.
  The rule **fails toward refusing rather than permitting**: when the vocabulary grows upstream,
  UsArr's verdict gets *more* conservative on its own, with no code change and without anyone having
  noticed the vocabulary moved. ⚠️ **The cost is named rather than argued away: a genuinely harmless
  upstream addition will still grade elevated until a human classifies it.** So the grading is a
  **maintenance obligation, not a self-maintaining mechanism** — an operator may see an elevated
  verdict that says only *"this build cannot judge what it grants"*, and the remedy is a person
  adding a line to `elevatedPermissions` or `unneededPermissions`, not a smarter default. Anyone
  weighing a change to this default should weigh those two paragraphs against each other, in that
  order.
- **The vocabulary is machine-checked, which is the reason it is in code at all.**
  `TestEveryBookOrbitPermissionIsClassified` iterates all 23 and fails if one is unclassified;
  `TestPermissionVocabularyMatchesTheSource` pins the transcription. A 24th permission upstream turns
  the suite red — and until it does, `unrecognised → ELEVATED` is what protects the runtime.
- **The bias is towards flagging, and two entries are broader than the scoping note that preceded
  the code.** `library_download` is graded elevated *even though downloading is a read*, because a
  stored credential that can pull every byte of every visible library is exfiltration reach and
  UsArr does not need it — covers come from `/books/:id/cover` and `/books/:id/thumbnail`, neither
  of which declares a permission. `manage_icons` is elevated because it is an app-settings write
  like its siblings. A false flag costs the operator one sentence on the Services screen; a missed
  one is a full-admin-equivalent credential stored under a scheme that says it is not.
- **The three non-permission findings are separate on purpose, and are not all elevated.** The
  superuser flag is **elevated** — it is every permission there is, and since `MagicLinkService`
  only mints links for shared accounts it means the account was promoted after its link was created.
  A non-`shared` `provisioningMethod` and an inactive account are **unneeded**: both say the account
  changed under the link, neither is extra reach, and the link still works. Grading them by severity
  rather than by category is what keeps `Elevated()` meaning one thing.
- **`Elevated()` is the predicate the gate turns on, and slice 1 owes the call.** ADR-0052's
  condition is on the *catalogue read*. If a `StreamItems` lands without consulting the verdict, the
  gate is open again and this ADR does not cover it.

  ✅ **Discharged 2026-08-19 at `862a0ca`.** Slice 1 paid the call: `BookOrbitSource.gate()` in
  `internal/libsync/bookorbit.go` consults `ScopeVerdict.Elevated()` before the first catalogue
  read, from **`Containers()` as well as `StreamItems()`**, behind one `sync.Once` — a container
  list is catalogue data, `Containers()` runs first, and the widening costs one `Authenticate` per
  source either way. **This is a note and not an amendment:** the bullet above named work that was
  owed, the work is done, and nothing in it is struck.
  ⚠️ **Discharged is not vacated.** The obligation is that the catalogue read consults the verdict.
  If a future read path lands without consulting it, the obligation is live again.
- **A superuser is reported once, not twice.** `buildUserResponse` emits both `isSuperuser` and the
  literal `["*"]` in the permission array; reporting both would read as two problems. The wildcard
  *without* the flag is still reported, because that combination is not one BookOrbit produces.
- **This is a pattern, not a BookOrbit special case, and it is not retrofitted here.** The same
  question — *what does the credential we stored actually let us do* — applies to every service UsArr
  holds a key for, and §14 already treats an \*Arr API key as full-admin. Nothing in this ADR grades
  an \*Arr key, and no such work is scheduled by it.

### What is built

`internal/bookorbit/scope.go`: the `Permission` vocabulary (23 constants, transcribed with their
wire string values and their source's grouping), `SuperuserPermission` for the literal `"*"`,
`elevatedPermissions` (14) and `unneededPermissions` (9), `ScopeSeverity`, `ScopeFinding`,
`ScopeVerdict` with `Elevated()` and `Minimal()`, and `classifyScope`, which never mutates and never
calls out. `internal/bookorbit/auth.go`: the verdict is computed on every mint, held on the cached
session, logged at WARN when non-minimal, and read back through `Client.Scope()` and
`Client.Authenticate`. The guards are `TestEveryBookOrbitPermissionIsClassified`,
`TestPermissionVocabularyMatchesTheSource`, `TestTheCorrectCredentialIsMinimal`,
`TestScopeClassification`, `TestSuperuserIsReportedOnceNotTwice`,
`TestScopeIsPopulatedByTheMintAtNoExtraCost` and `TestAccountViewIsAnAllowlist` in
`internal/bookorbit/scope_test.go`. **There is no UI half and no schema half:** nothing renders the
verdict yet, and nothing persists it — it is recomputed from the login response on every mint.

---

<a id="adr-0059"></a>
## ADR-0059 — The per-media-type facet count is an assignment: every work lands in exactly one bucket

**Status:** Accepted · **2026-08-19** · Landed with `GET /api/v1/library/facets` at `2711926` ·
**Records the counting semantics that shipped**, which had nowhere decision-shaped to live ·
**Closes off** ARCHITECTURE §17.2's independent-`EXISTS` shape *as the counting rule* — it stays
correct as a **presence** test · **Adds no endpoint, no migration, no column and no UI change** —
the read, `ix_edition_format` (migration `00009`) and the tests were all in the tree when this was
written · ⚠️ **A live input to [ADR-0053](#adr-0053)**, whose reopening condition is refined rather
than discharged on the strength of this

### Context

`GET /api/v1/library/facets` answers §17.2's six navigation types with a count each. The wire
contract is [`reference/http-api.md`](./reference/http-api.md) §8 and the reasoning behind every
predicate is `internal/store/facets.go`. **Those two are authoritative and this ADR copies neither**
— it exists for the one thing neither is shaped to hold: the decision that was taken when a work
could plausibly have been counted twice, and the consequence that follows from taking it.

The question is forced by `book`. It is the one `work.kind` that two media types share (§17.2 rows 4
and 5), separated by `edition.format` — and a book can carry both an EPUB and an M4B at once. Every
other type is a `work.kind` equality and has no such case, which is why this decision is about one
kind and reads as though it were about all six.

### Decision

> **The Ebooks/Audiobooks split is an ASSIGNMENT, not two independent tests. Every `book` work lands
> in exactly one of the two buckets, and a book with both an ebook edition and an audiobook edition
> lands in Ebooks.**
>
> The assignment is `mediaTypeOf`'s — **the same function that renders the Type cell of every Block C
> row and every library-grid row** — reached by calling it rather than by restating its rule.

**A count cannot do otherwise and remain a count.** If a work could occupy two buckets, the column
would stop summing to the library, and the same work would be reported twice inside a summary whose
entire question is *"what do I have?"*. A number that double-counts is not a smaller error than a
number that under-counts; it is a different kind of object.

**And the choice of *which* bucket is what makes the count equal the list.** The count's consumer is
a row a user clicks, and the click lands on `GET /api/v1/library?media_type=X`. That endpoint filters
Audiobooks on `browseAudiobookPredicate` and Ebooks on its complement — the same predicate the facet
read binds, shared by reference rather than retyped. So the count is not merely *a* consistent
bucketing; it is the bucketing the grid already performs, and the two agree row for row.
`TestMediaTypeCountsAgreeWithTheBrowseRead` pins that equality **from the consumer's side**, by
paging the grid and counting what comes back rather than by re-deriving it from the facet query.
That equality is the argument for the decision, not a pleasant side effect of it.

### Alternatives rejected

- **§17.2's independent-`EXISTS` shape, used as the counting rule.** Rows 4 and 5 give each type its
  own `EXISTS` over `edition.format`, under which a book with an EPUB *and* an M4B makes **both**
  types "have content". **That is the right answer to the question §17.2 is asking and the wrong
  answer to this one**, and the distinction is worth stating rather than assuming: *presence* is
  monotone and may overlap — two true answers about one work are not in conflict, because neither
  claims exclusivity — while a *count* is a partition and overlap breaks it. Adopting the `EXISTS`
  shape here would produce two columns that each look right and together exceed the library.
- **Count a mixed book under Audiobooks instead.** Symmetric on the arithmetic and worse on the
  equality above: `mediaTypeOf` answers Ebooks for a book with no edition rows at all, so making the
  mixed case Audiobooks would put the two arms of one function on different rules and re-open the
  drift the shared call exists to prevent.
- **A third bucket for mixed books.** §17.2's enum is closed at six *by construction* and the whole
  navigation model rests on that; a seventh value is a change to the axis, not to this read.

### Consequences

- ⚠️ **A library whose only audiobooks are second editions of ebooks reports `audiobooks: 0`, while
  §17.2's row-5 predicate says that type has content. The two disagree by design.** This is the
  sentence this ADR exists to put somewhere a reader will meet it: it is stated in
  `reference/http-api.md` §8.4 and in `internal/store/facets.go`, and neither is where someone
  reasoning about *whether to trust a count* will look.
- **The disagreement is only a defect if something reads the count as a presence test.** Nothing
  does today. It becomes one the day something hides a type on this number — which is
  [ADR-0053](#adr-0053)'s territory, below.
- `ebooks + audiobooks` is exactly the number of `book` works, with no work in both and none in
  neither. The Ebooks half is the complement rather than its own statement, so no third plan has to
  be kept honest to confirm arithmetic.
- **This ADR does not own the numbers or the predicates.** `internal/store/facets.go` owns the
  reasoning, `reference/http-api.md` §8 owns the wire contract, and the code is right about
  behaviour whenever the two disagree. Restating a predicate here would make this a third place it
  lives, which is the failure the search weights were just fixed for.

### Its relation to ADR-0053, which is not this ADR's decision to take

[ADR-0053](#adr-0053) closes per-type hiding in the sidebar and reopens *"on exactly one condition:
a read answering which of the six types have rows under the current scope, in one statement"*. The
facets read is one statement per half of one read, under scope, per type — **and it does not satisfy
that condition**, for the reason this whole ADR is about: it answers *how many works are bucketed to
each type*, and the condition needs *whether a type has content*.

The gap is invisible until a two-format book, and then it is the failure mode this project keeps
refusing: **hiding Audiobooks from someone who has audiobooks.** ADR-0053's own rejected
alternatives already name that shape — *"hide a type on a count nobody measured"* — and it would
have arrived here through a count somebody did measure, which is worse, because it would have looked
like compliance.

**The amendment was considered and declined.** The nav stays all-six-always for v0.1. What was done
instead is recorded on ADR-0053: its reopening condition is **refined, not discharged**, so that it
now names an existence predicate over `edition.format` — the independent `EXISTS` of §17.2 rows 4–5,
which `ix_edition_format` already serves — and cannot be re-discharged by these counts on a later
pass. **The decision about what eventually hides an empty type is not taken here**, and whoever
takes it inherits the question this ADR is the answer to: what to do about a book whose audiobook
edition is invisible to the count.

### What is built

`internal/httpapi/facets.go`, `internal/store/facets.go` and `internal/store/facets_test.go`, at
`2711926`; the wire contract at `reference/http-api.md` §8. Nothing rendered it when this was
written — §17.2's Block A is still undrawn, and `internal/httpapi/library.go` says so — so the
decision recorded here is currently load-bearing for exactly one consumer, the next one.

---

<a id="adr-0060"></a>
## ADR-0060 — A stored credential can be readable upstream: BookOrbit's `raw_token` is bounded by the service account's privilege, not by rotation

**Status:** Accepted · **2026-08-19** · **Records a measured property of the service v0.1 replicates
from** ([ADR-0052](#adr-0052)) rather than a design UsArr chose · **Measured, not recalled** —
BookOrbit at `73b7877`, and Kavita at tag **`v0.9.0.2`** (`6bcd568`) for the class comparison, which
is the release the owner runs and the commit [`api/specs/SOURCES.md`](../api/specs/SOURCES.md)
records · **Amends [`ARCHITECTURE.md`](./ARCHITECTURE.md) §14** with one clause that **generalises
and names no service**; the edit lands in this same change · **Keeps two axes apart** — *plaintext at
rest* and *retrievable over the API* — because collapsing them turns a class norm into a scandal in
one direction and a real step down into a shrug in the other · **Closes off periodic re-minting as a
mitigation** · **Names a CLOSING condition, not only reopening ones** · **Ships no code, no
migration, no column and no schema change** — the one control this ADR names already exists
([ADR-0058](#adr-0058))

### Context

UsArr authenticates to BookOrbit with a magic-link token it stores encrypted, AAD-bound and
never-to-the-browser, per §14. **That is a statement about UsArr's side of the boundary and it says
nothing about the other side.** This ADR is what happens when the other side is measured.

#### 1. What BookOrbit does with the token it issued, measured at `73b7877`

Three facts, each read from source at that commit rather than inferred from the module's shape:

- **The plaintext is a column.** `server/src/db/schema/auth.ts` declares `magic_access_tokens` with
  **both** `tokenHash: varchar('token_hash', {length: 255}).notNull().unique()` **and**
  `rawToken: varchar('raw_token', {length: 255}).notNull()`. The hash is not a replacement for the
  secret; it sits beside it.
- **The write puts it there.** `MagicLinkRepository.create` (`magic-link.repository.ts`) generates
  `randomBytes(32).toString('hex')`, computes `sha256(rawToken)`, and inserts **both** values.
  Authentication itself is hash-based — `loginWithToken` hashes the presented token and calls
  `findByTokenHash` — so the plaintext column is **not load-bearing for login**. It is stored so it
  can be shown again.
- **A superuser can read every one of them back.** `MagicLinkRepository.findAll` selects
  `rawToken` for **every** row, joined to the owning and creating usernames, behind
  `GET /api/v1/auth/magic-links` → `AuthController.listMagicLinks`, which throws
  `ForbiddenException('Only superusers can view magic links')` unless `user.isSuperuser`. ⚠️ **That
  check is the whole of the authorization for this read, and it lives in the controller alone** —
  `MagicLinkService.listTokens()` is two lines with no guard of its own, where `createToken`,
  `setActive` and `revokeToken` each re-assert `actor.isSuperuser` in the service. This is an
  observation about **where** the gate sits, **not** a claim that the gate is missing: on the code
  as measured, the route is superuser-only.

✅ **None of that is a mis-implementation of the surrounding design, and this ADR says so before it
says anything else.** Minting is superuser-only and restricted to `shared` accounts, issuance and
revocation emit audit events, and expiry, revocation, an active flag and a 25-token-per-user cap are
all enforced on the login path. The `raw_token` column is a deliberate product decision — *"show me
the link I made"* — with a foreseeable cost, not an accident.

#### 2. What the comparison is worth, and the ref it was taken at

The obvious defence of the finding above is that this is simply what the class does. **That is worth
testing rather than assuming, so it was tested against the other catalogue source this repo has a
measured adapter for.**

🚩 **The ref matters, and is named for a reason this repo has already paid for.** Kavita's `main` is
frozen at v0.7.8 and has planted a wrong fact in these docs before; the measurement below was taken
at **tag `v0.9.0.2`, commit `6bcd568`** — the release the owner runs and the same commit
[`api/specs/SOURCES.md`](../api/specs/SOURCES.md) pins the vendored spec to. Reading `main` for this
would have described a build nobody in this project runs.

| Question | Kavita, at tag `v0.9.0.2` / `6bcd568` | Source |
|---|---|---|
| Is the auth key hashed at rest? | **No — and there is no hash column at all.** `AppUserAuthKey.Key` is a plain `string` under `[Index(nameof(Key), IsUnique = true)]`, and authentication is **raw-value equality**: the handler resolves the presented key against that column directly. BookOrbit at least *has* a `token_hash` and authenticates on it. | `Kavita.Models/Entities/User/AppUserAuthKey.cs` |
| Is there a second plaintext copy? | **Yes, a lingering legacy one.** `AppUser.ApiKey` is `[Obsolete("Migrated to AuthKey in v0.8.9")]`, and the migration that moved it copied it **verbatim** — `Key = user.ApiKey` — without clearing the old column. | `Kavita.Models/Entities/User/AppUser.cs`, `Kavita.Server/ManualMigrations/v0.8.9/MigrateToAuthKeys.cs` |
| Can an admin read **another user's** key over the API? | **No route does — admin-gated or otherwise.** `MemberDto` carries **no key field at all**. `GET /api/account/auth-keys` resolves `GetAuthKeysForUserId(UserId)` from the caller's own claims; `ConstructUserDto` fills `ApiKey`/`AuthKeys` from the user the response is *about*, on that user's own flows. Rotate and delete each fetch by id and then assert **`if (authKey?.AppUserId != UserId) return BadRequest();`**. | `Kavita.Models/DTOs/Account/MemberDto.cs`, `Kavita.Server/Controllers/AccountController.cs` |

**So the class norm splits into two different answers, and that split is the reason this ADR
exists.**

#### 3. The two axes, and why keeping them apart is the whole value

- **Plaintext at rest.** BookOrbit stores it; Kavita stores it and has no hash to store instead. On
  this axis BookOrbit is **not** an outlier, and is if anything ahead — it authenticates on a hash,
  which Kavita cannot.
- **Retrievable over the API.** BookOrbit returns every token's plaintext to any superuser on one
  documented route; Kavita returns no user's key to anybody but that user. On this axis BookOrbit is
  behind, and the gap is **a route that exists** versus **a route that does not**.

⚠️ **Collapsing the two is how this gets reported wrong in both directions.** Collapsed one way, a
class-normal storage choice reads as BookOrbit's scandal. Collapsed the other, a real widening of
the boundary reads as *"everyone does this"* and disappears. The axes are held apart in the decision
below, and every later note is labelled with the axis it qualifies.

### Decision

> **Plaintext-at-rest is the class norm, which Kavita confirms; admin-retrievable-over-the-API is
> not, which Kavita refutes — BookOrbit's superuser `rawToken` listing is a step down the threat
> model names as such, not as the ecosystem baseline.**
>
> **This is not recorded as a defect *of BookOrbit*. It is recorded as a fact about THE SWITCH**
> ([ADR-0052](#adr-0052)): moving v0.1's catalogue source from Kavita to BookOrbit **widened the
> boundary on the retrievability axis**, and left the storage axis where it already was. The switch
> stands; what changes is that its price is written down where the next reader meets it.
>
> **Because the credential is retrievable, and because rotation does not change that, the privilege
> of the account UsArr authenticates as is the ONLY control that bounds the consequence.** It is not
> one mitigation among several. It is the whole of UsArr's leverage — and it is already built:
> [ADR-0058](#adr-0058)'s scope grading is what keeps the retrievable thing worth as little as
> possible.
>
> **Rotation is available and is NOT advised on a schedule.** A re-minted token is written to the
> same plaintext column and returned by the same listing. See the Alternatives.

### Why privilege is the only control, stated as the link rather than left adjacent

A reader who takes the finding and the recommendation as two neighbouring paragraphs has missed the
argument, so it is made in one line: **UsArr cannot make the token unreadable, so the only variable
left is what reading it is worth.**

Everything §14 gives UsArr — AES-256-GCM with a key-version prefix, AAD bound to the row and to the
instance's `host:port`, redaction before every log line, never sending the secret to the browser —
protects the copy **UsArr** holds. None of it touches the copy **BookOrbit** holds, and the copy
BookOrbit holds is the one on the API. So the residual question is not *"can the token leak"* but
*"what does a leaked token authorize"*, and that has exactly one answer-shaped input: the
permissions on the shared account. [ADR-0058](#adr-0058) grades those on **every** mint and warns
when they are not minimal. It was written for the opposite question — *what does the credential we
stored let **us** do* — and it is load-bearing here for this one.

⚠️ **The concrete difference the retrievability axis makes is ATTRIBUTION, and it is named because
*"a superuser could just mint their own link anyway"* is the reply this finding attracts.** They
could — `createToken` is superuser-only and **emits an audit event** (`AuditAction.MagicLinkCreate`).
Reading the existing token emits **nothing**: `listTokens()` performs no audit write, and use of the
token afterwards increments `useCount`/`lastUsedAt` on **UsArr's own row**. Minting a second link is
a recorded act by a named actor; reading UsArr's is an unrecorded one whose consequences are
attributed to UsArr. That is the step down, precisely.

### Alternatives rejected

- **Re-mint the token periodically, on a schedule.** 🚫 **Rejected, and rejected explicitly so
  nobody re-proposes it as an easy win.** It is the reflex answer to a credential-exposure finding
  and it is **inert** here: the new token is written to `raw_token` exactly as the old one was, and
  listed by `findAll` exactly as the old one was, so the window it shortens is not the one that is
  open. It buys **churn**, a sync that fails whenever a re-mint and a walk interleave, and an
  operator who believes a control is in place. **Rotation stays available for what it is actually
  for** — an **incident**, where a specific token is believed compromised and a superuser revokes it
  — and that use is untouched here.
- **Refuse to connect to BookOrbit until the plaintext column is gone.** This refuses v0.1's
  catalogue source over a property the whole class shares on one of the two axes, and leaves the
  operator with a service that will not talk to them — the same trade [ADR-0058](#adr-0058) turned
  down for the same reason, and principle 3 says which way it goes.
- **Encrypt UsArr's copy harder.** Already done, and it addresses the wrong side of the boundary. A
  stronger scheme on UsArr's disk does nothing about a route that returns the same secret in a JSON
  body.
- **Record it in [`RESEARCH.md`](./RESEARCH.md) as an ecosystem finding and leave it there.** Half
  right — the measurement *is* research, and belongs there too — but the finding closes off an
  alternative (scheduled rotation) and names one control as the only one, which is decision-shaped.
  `CLAUDE.md`'s rule for that case is an ADR.
- **Report it upstream instead of writing an ADR.** Not either/or, and the ADR is what makes the
  report possible: an upstream ask needs a stated acceptance condition — the closing condition below
  — rather than a complaint that a design is uncomfortable.

### Consequences

- **The §14 amendment generalises, and names no service.** What lands in
  [`ARCHITECTURE.md`](./ARCHITECTURE.md) §14 is clause **8**, in full:

  > 8. **A credential UsArr stores may be readable by the service that issued it** (ADR-0060).
  >    Encryption at rest, the AAD binding, redaction and never-to-the-browser are protections on
  >    **UsArr's** side of the boundary; **none of them constrains what the issuing service does
  >    with its own copy of the same secret.** **UsArr's protections are therefore necessary and not sufficient**, and
  >    this model says which side of the boundary each protection sits on. Every adapter is asked
  >    the same two questions before its credential path is designed — *how does this service store
  >    the credential it issues us*, and *what can be read back over its API, and by whom* — and the
  >    two are kept apart, because plaintext at rest and retrievable-over-the-API are different
  >    exposures with different remedies. Where the answer is bad and cannot be fixed from here, the
  >    only control left is **the privilege of the account the credential authenticates as**, which
  >    is why that privilege is graded rather than assumed ([ADR-0058](#adr-0058)).

  It names no service because the question is not BookOrbit's. **Every future adapter gets asked
  it**, and a clause that named one service would read as settled the moment that service was
  replaced — which, on this repo's record, is a matter of months.
- ⚠️ **This ADR does not make the operator's BookOrbit safer, and does not pretend to.** Everything
  measured here is upstream of UsArr and unreachable from it. What changes is that the residual risk
  is **named**, is **attached to the control that bounds it**, and has a condition under which it
  stops being true.
- **A superuser is already the trust boundary, and this narrows what that sentence excuses.** Anyone
  who can read the listing already administers the service UsArr replicates. The finding is not that
  a stranger can read the token; it is that the boundary is **wider than the credential's own
  scope** and that the widening **leaves no trace**, so *"they were already trusted"* stops being a
  complete answer.
- **The storage axis keeps its own consequence, which is not about the API at all.** Anything that
  reads BookOrbit's database — a backup, a dump, a restore onto another host, an operator with
  `psql` — yields **live** credentials with no cracking step. That is true of Kavita too, which is
  what makes it the class norm, and it is why the axis is recorded rather than waved through.
- **Nothing in the adapter changes.** No code, no migration, no column, no new HKDF label, no extra
  request. The control this ADR names is `internal/bookorbit/scope.go`, which shipped with slice 0.

### The Trace-logging hedge, which qualifies ONE axis

⚠️ **"Not admin-retrievable" is true of Kavita's API surface and true by default. It is not
absolute**, and the qualification is recorded here rather than discovered later by someone who
concludes this ADR overstated its comparison.

At tag `v0.9.0.2` / `6bcd568`, `AuthKeyService.UpdateLastAccessedAsync` opens with
`logger.LogTrace("Updating last accessed Auth key:  {AuthKey}", authKey)` — and it is reached only
from `AuthKeyAuthenticationHandler`, **after** the presented key has resolved to a real user, so
what it logs is a **valid** key. `Trace` is one of the five levels `GET /api/settings/log-levels`
offers, the level is written through the admin-gated settings update (`SettingsService` →
`SwitchLogLevel`), and `GET /api/server/logs` — on a controller carrying
`[Authorize(PolicyGroups.AdminPolicy)]` — zips the log files and returns them. **So a Kavita admin
can reach live keys indirectly.**

**What that costs, stated so the hedge is not mistaken for equivalence:** it is a **deliberate
multi-step through a configuration change and then a wait** — lower the level, wait for someone
else's key to actually be *used*, download the zip, read a log line — against **one authorized `GET`
that returns every token, at any time, whether or not it has ever been used.** A capability that
requires reconfiguring the server and waiting on a victim is a different object from a read.

🚫 **Not exercised on any live instance.** This is a source reading at a pinned commit. Nobody on
this project may turn a log level up on the owner's server to confirm it.

⚠️ **And the axis label, because a later reader will otherwise use this paragraph to blur the two
back together: the hedge qualifies RETRIEVABILITY ONLY.** It refines that axis from an absolute
*"no route"* to *"no route, and admin-reachable only by deliberate reconfiguration"*. **It leaves
plaintext-at-rest exactly where it was** — Kavita still stores the key in the clear with no hash
column, BookOrbit still stores `raw_token` beside its hash, and nothing in this paragraph touches
either. The hedge is **not** a finding that *"Kavita is just as bad after all"*, and it must not be
cited as one.

### The closing condition, and what reopens this

**A permanent statement about a mutable upstream rots into a rumour**, and a year from now nobody
would know whether this still holds. So it closes as well as reopens.

✅ **CLOSES — this ADR becomes historical, and the §14 clause stays — when EITHER holds at a named
BookOrbit commit, measured rather than assumed:**

1. `magic_access_tokens` no longer carries `raw_token` — the column is dropped, and the migration
   that drops it does not move the value somewhere else; **or**
2. `GET /api/v1/auth/magic-links` no longer returns token plaintext to anybody — `findAll` stops
   selecting `rawToken`, or the field leaves the response — while **creation may still show the
   secret once, to its creator**, which is the pattern this ADR has no argument with.

Either one discharges the **retrievability** finding; **the first discharges both axes.** Whoever
verifies it records the commit and adds a dated note here, in the shape [ADR-0052](#adr-0052) and
[ADR-0058](#adr-0058) use for their discharges — **a note, not an amendment**, because a condition
met is a fact arriving rather than a claim falsified.

📤 **Those two clauses are also the concrete ask any upstream report carries**, which is the second
reason for writing them out: they are a change request with an acceptance test, not a complaint that
a design is uncomfortable.

⚠️ **REOPENS, and gets worse rather than better, if:** the listing loses its superuser gate or gains
a non-superuser caller — the controller check is the only one, per Context §1; the plaintext appears
on a second route or in an export; or UsArr's own account stops being a minimal shared account, at
which point [ADR-0058](#adr-0058)'s verdict is the thing that says so, and this ADR is why that
warning matters.

### What is built

**Nothing, by design.** One clause in [`ARCHITECTURE.md`](./ARCHITECTURE.md) §14, quoted in full
above and landed in this same change, and this record. The control the decision names — the scope
grading that keeps a retrievable credential worth as little as possible — is
`internal/bookorbit/scope.go` and `internal/libsync/bookorbit.go`'s `gate()`, both already in the
tree under [ADR-0058](#adr-0058). **No test is owed here and none is claimed:** the facts in Context
§1 and §2 are readings of two upstream repositories at pinned commits, which UsArr's suite cannot
assert against and must not pretend to.

---

<a id="adr-0061"></a>
## ADR-0061 — Catalogue completeness is **measured and three-valued**: "not checked" is a state, not a zero

**Status:** Accepted — 2026-08-19

### Context

#### 1 · The defect, verified at `bookorbit/bookorbit@73b7877d`

A BookOrbit account can carry `contentFilters`. `LibraryRepository.findAllForUser`
(`server/src/modules/library/library.repository.ts:30-51`) turns them into `filterClauses` and puts
them in **`bookJoinOn` — the books `LEFT JOIN … ON` condition** — and the statement has **no
`.where()` at all**. Library rows come from `.from(libraries)` INNER JOINed to `userLibraryAccess`
(line 46), which the filter never touches.

**So a content filter shorts each library's `bookCount` without dropping a library row.** That is
the nastiest shape a replica can have: the library appears, the counts look plausible, and a slice
of the books is simply absent, with nothing anywhere saying so.

#### 2 · And the shortfall is a subtraction, not a guess

`LibraryRepository.getStats(libraryId)` (`library.repository.ts:150-178`) takes **neither a user nor
a filter set**; both of its selects are `where(libraryId AND status = 'present')`, and
`LibraryService.getStats` (`library.service.ts:298-309`) passes it nothing else. The route is
`@Get(':id/stats')` with `@RequireLibraryAccess('viewer')` and **no `@RequirePermission`**
(`library.controller.ts:108-112`), so UsArr's shared viewer account with an empty permission set
reaches it.

Both sides of the pair therefore carry `status = 'present'`, applied server-side, and differ in
exactly one term — the content filter. `totalBooks − bookCount` **is** the number of present books
the filter hid.

⚠️ **The paged read is NOT the other side of this pair, and pairing it would be wrong.**
`book-query-builder.service.ts:55-90` builds **no status predicate at all** unless the caller
supplies an explicit `isPresent` status rule (`statusRuleToSql:789`). A shortfall computed against
`POST /libraries/{id}/books`'s `total` without that rule would be subtracting a
filtered-and-unstatused count from an unfiltered-and-statused one. The listing's `bookCount` is used
precisely because there is no predicate for a caller to forget.

#### 3 · The dependency nobody promised us

Fact §2 rests on an **unguarded upstream route**. That is a property of somebody else's service at
one commit, not a contract with UsArr, and BookOrbit may add a `@RequirePermission` to it at any
time. A design that did not plan for that would report every library as complete on the day it
stopped being able to tell — which is the original defect, recreated inside its own fix.

#### 4 · And there is a second axis this cannot reach

Whether **whole libraries** are hidden from UsArr's account is **unanswerable from a read-only
account**. `LibraryAccessGuard` (`common/guards/library-access.guard.ts`) throws an identical
`ForbiddenException('No library access')` for *"the library exists and this account has no access
row"* and for *"there is no such library"*. Probing ids would be enumeration against the operator's
own service and would still not distinguish the two.

### Decision

**1 · UsArr measures catalogue completeness per container, at import, and records it.** The BookOrbit
adapter calls `GET /api/v1/libraries/{id}/stats` once per library per import
(`internal/bookorbit/stats.go`) and subtracts the listing's `bookCount` from its `totalBooks`
(`internal/libsync/bookorbitcompleteness.go`). One extra request per library per import is the whole
cost.

**2 · The verdict is THREE-VALUED, and `unverified` is a first-class member.**

| State | Means |
| --- | --- |
| `complete` | Measured; the two counts agree. |
| `shortfall` | Measured; the credential was shown fewer items than the container holds. |
| `unverified` | **Not measured.** Must never render as either of the other two. |

⚠️ **A boolean is rejected explicitly.** "No shortfall" and "not checked" are different facts, and
Context §3 is why: collapsing them makes an instance whose probes have all started failing
indistinguishable from an instance with nothing wrong. The same rule is enforced one level down —
`Total` is **−1**, never `0`, under `unverified`, because `0` is a legal total for an empty container.

**3 · The named degradation condition is written down: if BookOrbit guards
`GET /libraries/:id/stats`, every probe answers 403 and every verdict becomes `unverified`.** Any
error at all — a 403, a timeout, an open breaker, a moved route, a body that is not a count — reads
as `unverified` with a reason. This is stated in `internal/bookorbit/stats.go`, in
`internal/libsync/bookorbitcompleteness.go`, in `reference/http-api.md` §2.6, and here.

**4 · It never blocks or refuses a sync.** Not one path in the check can fail an import. **A partial
replica that says it is partial beats no replica**: refusing would turn a reporting improvement into
an outage, and the items that did import are correct either way.

**5 · Every container gets a row, including the ones that were fine.** `sync_report.kind =
'content_completeness'`, one row per container per import. This is the opposite of the neighbouring
`items_skipped` rule and is deliberate: an absent skip row means nothing was skipped, an absent
completeness row means nothing was **asked**, and the two absences must not look alike.

> 🚩 **SUPERSEDED IN PART 2026-08-19 by [ADR-0063](#adr-0063)** — **this clause's own rule stands
> unchanged; what falls is the `items_skipped` semantics it contrasts itself against.** That kind now
> follows the SAME rule — a row per container an import walked, zero or not — so *"an absent skip row
> means nothing was skipped"* is no longer true, and the two absences no longer differ. The reason
> was that they were being read one column apart on the same §17.8 row with opposite meanings, and
> that the reader built on this clause had to borrow the completeness row as its evidence that a
> container had been walked at all. **Every other word of this clause survives**, including its
> conclusion for `content_completeness`: an absent verdict here still means nothing was **asked**.

**6 · The claim's SCOPE travels in the row.** Every row carries `covers` and `does_not_cover` in its
own detail blob. The second is load-bearing: Context §4's axis is unanswerable, so `complete` on
every library UsArr can see is **not** a statement that UsArr can see every library, and the row says
so to anyone reading it out of the database or off the wire.

**7 · It is surfaced on the Libraries screen (§17.8), from local SQLite.** The comparison is at
import; the render is a `SELECT`. A `shortfall` row says *"Some books are hidden — this library holds
412 books; the service account can see 389"* with the age of the measurement, and one sentence above
the table names the fix, which is off this screen: the filter is on the service account. An
`unverified` row says *"Completeness unverified"* in grey with the reason. **`complete` renders
nothing**, which keeps this column's standing invariant — nothing on the Libraries screen renders a
positive health claim — and is why `unverified` has to be loud.

### Consequences

- **One extra HTTP request per library per import.** Not per page and not per item; pinned by
  `TestTheStatsProbeIsMadeOncePerLibraryAndNotPerBook`.
- **No migration.** `sync_report` carries no `CHECK` over `kind` (migration `00005`), so the
  vocabulary grows without DDL, and `detail` is already a JSON column. The verdict's three sides —
  the adapter that measures it, `cmd/usarr` that writes it, the store read that folds it — share one
  declaration in `internal/store/completeness.go` for exactly that reason: a typo would be a silently
  missing verdict rather than a constraint violation.
- **A multi-container library loses precision, deliberately.** The fold puts `unverified` **above**
  `shortfall`, because `total_items` and `visible_items` are library-level once folded and an
  unmeasured container makes both wrong. The shortfall on the other container is not lost — it is in
  the log and in its own `sync_report` row.
- **The Kavita adapter is untouched and serves no verdict**, which renders as an absent key. It is
  the seam: an adapter that can make the comparison implements the same shape and the screen needs no
  change.

### Alternatives rejected

**A boolean `complete`.** Rejected under Context §3 — see Decision 2.

**Refuse the sync on a shortfall.** Rejected under Decision 4. A partial catalogue that says so is
strictly better than none, and the failure mode is an outage rather than a report.

**Compare against the paged walk's `total` instead of the listing's `bookCount`.** Rejected under
Context §2: the paged read has no default status predicate, so the two sides would sit on different
predicates unless every caller remembered to send an `isPresent` rule. Correctness that depends on a
caller remembering is not correctness.

**Probe library ids to find hidden libraries.** Rejected under Context §4. It is enumeration against
the operator's own service and it does not work: the guard's two refusals are byte-identical.

**Record only the shortfalls.** Rejected under Decision 5.

### What is built

`internal/bookorbit/stats.go` · `internal/libsync/bookorbitcompleteness.go` ·
`internal/store/completeness.go` · `internal/store/libraries.go` (the third statement and the fold) ·
`cmd/usarr/import.go` (`recordCompleteness`) · `internal/httpapi/libraries.go`
(`libraryCompletenessResponse`) · `web/src/lib/libraries.ts` (`toCompleteness`, `completenessMarks`,
`completenessNote`) · `web/src/routes/libraries/+page.svelte`. Wire contract in
[`reference/http-api.md`](./reference/http-api.md) §2.6.

**Guards:** `internal/libsync/bookorbitcompleteness_test.go`,
`cmd/usarr/bookorbit_completeness_e2e_test.go`, `internal/httpapi/libraries_test.go` and
`web/src/lib/completeness.test.ts`. The one that matters most is the guard-later drill —
`TestAGuardedStatsRouteRecordsUnverifiedRatherThanComplete` — which serves a 403 from the fake
BookOrbit's stats route and asserts the recorded verdict is `unverified` with `Total = -1`.

---

<a id="adr-0062"></a>
## ADR-0062 — `usarr backup` captures the database **and** the KEK salt, and leaves the master key out — loudly

**Status:** Accepted — 2026-08-19

### Context

#### 1 · The question is not "does a backup include secrets", it is "which failure do we choose"

Both answers are bad in a specific, nameable way, and picking one without saying so is how this
project already lost credentials once.

**Key material in the archive** makes a single file that holds the ciphertext and the key that opens
it. That file is a complete compromise on its own: whoever gets the backup gets every stored \*Arr
API key, and every \*Arr API key is a full-admin credential (`ARCHITECTURE.md` §14). Backups are the
copy that leaves the host — to a NAS, to object storage, to a cloud sync folder — so it is also the
copy least likely to be protected the way `/config` is.

**Key material out of the archive** makes a backup that restores a library and cannot open a single
credential. Worse, it fails *quietly*: the database opens, the screens render, and only a connection
test says anything is wrong.

#### 2 · The layout already chose, and the choice is load-bearing

`internal/config/config.go`'s `KeysDir` states it outright — `keys/` "is excluded from every backup,
so that 'back up this volume' and 'never store the key with the ciphertext' are not the same
instruction" — and `CONFIGURATION.md` §6.1 says the same thing at the operator's end: *"`tar -czf
backup.tgz /config` is not a backup. It is a compromise."* A new command that reversed that would
not be a fresh decision; it would be a second, contradictory answer living beside the first, and the
weaker one would win by being the convenient one.

#### 3 · But the 2026-08-16 failure was not the exclusion — it was the silence about what the exclusion then required

`kek.salt` lived inside `keys/`. The documented procedure excluded `keys/`. Both halves were
individually defensible; nothing anywhere said that a restore therefore needed a second file, so the
gap surfaced only when it was already unrecoverable — a byte-identical master key that decrypted
nothing, because `KEK = HKDF-SHA256(secret.key, salt=kek.salt, info="usarr/kek/v1")` and a different
salt is a different KEK. The salt has since moved to `$USARR_CONFIG_DIR/kek.salt`, beside the
database.

**That fix repairs the volume-level archive and does nothing for a snapshot.** `VACUUM INTO` copies
one SQLite file. The salt sits next to that file and is not in it. So a lone
`backups/usarr-*.db` reproduces the 2026-08-16 defect exactly, one level down, with the same
symptom and the same recoverability.

### Decision

**`usarr backup` writes a pair, and prints what it is not writing.**

1. **In: the database.** `VACUUM INTO`, mode `0600` in a `0700` directory, shared code with the
   automatic pre-migration backup (`vacuumInto` in `cmd/usarr/backup.go`).
2. **In: `kek.salt`,** copied byte for byte beside the snapshot as `usarr-<stamp>.kek.salt`, from
   `$USARR_CONFIG_DIR/kek.salt` or, on an install that predates the move, from the legacy
   `keys/kek.salt`. The salt is **not secret** — its value protects nothing, it only has to be
   per-install and stable — so it costs nothing to the compromise argument, and it is the input
   nobody thinks to keep.
3. **Out: the master key,** whichever channel supplies it. Reason: §1's first cost is the one that
   is *unbounded*. A missing key costs the operator a re-entry of each \*Arr API key — tedious,
   bounded, and repairable by hand. A leaked key costs every credential the backup ever held, with
   no repair short of rotating each one at each service.
4. **Said out loud, every run, in the command's own output** — not only in this ADR and not only in
   `CONFIGURATION.md` §6. The person who needs the sentence is reading a terminal.

### The restore path for the half it excludes

This section is the point of the ADR. Recording the choice without recording its consequence is
precisely what happened on 2026-08-16.

`usarr backup` prints the sequence for **this install's** key channel, because the channel changes
what the operator does and naming the wrong one makes the instruction unfollowable:

| Channel, as resolved by `internal/config` | What the backup says to do now | What the restore then is |
|---|---|---|
| `keys/secret.key` (the default) | `cat $USARR_CONFIG_DIR/keys/secret.key`, store that value in a password manager or secrets store, **once**, and not in `backups/` | Write it back to `keys/secret.key`, mode `0600`, inside a `0700` `keys/`, before starting UsArr |
| `USARR_SECRET_KEY` | The value in your compose file / systemd unit / secrets store **is** the key; UsArr stores nothing on this host to recover it from | Set the same variable to the same value before starting |
| `--secret-key-file` / `USARR_SECRET_KEY_FILE` | Back up that file to somewhere that is neither this archive nor this host | Put it back at the path the flag or variable points at, mode `0600` |

Restoring the pair is then: stop UsArr, move the damaged database aside, copy the `.db` to
`$USARR_CONFIG_DIR/usarr.db` and the `.kek.salt` to `$USARR_CONFIG_DIR/kek.salt`, install the key as
above, start. The first two restore the library, the users and the audit log; the third is what makes
the stored credentials readable. `CONFIGURATION.md` §6.3 is the long form.

**If a rotation is unfinished** — `keys/secret.key.new` exists — the report says so and says to keep
**both** key files, because rows in that snapshot may be sealed under either.

### Consequences

- The one artefact this command can produce that would be worse than nothing — a `.db` that looks
  like a backup and cannot be restored — is now impossible without the operator having read a
  paragraph saying so.
- `backups/` gains a second file type. It still contains **no key**: a salt is not one, and the
  compromise property of `backups/` is unchanged.
- The command still does not cover `providers/*.yaml` or `$USARR_DATA_DIR`, and says so.
- It writes nothing into the install it is reading. In particular it does **not** call
  `Config.ResolveKEKSalt`, which would copy a legacy salt forward — a backup that mutates the thing
  it is protecting is a backup with a failure mode of its own.
- Retention is **not** part of this. `CONFIGURATION.md` §6.1's "retains N files" belongs to the
  scheduled job, which does not exist; adding a pruner alongside an operator-invoked command would
  mean a command that deletes backups nobody asked it to delete.

### Alternatives rejected

**Include `keys/` behind a `--with-key` flag.** The flag would be typed by exactly the people who
should not type it, and its output would be indistinguishable from an ordinary backup afterwards.
Nothing in the file says which kind it is.

**Encrypt the snapshot under a passphrase and include the key.** A second secret to lose, in a
project whose stated failure mode is losing exactly one. It also puts a crypto surface in a command
whose value is that it is boring.

**Print nothing and rely on `CONFIGURATION.md` §6.** That is the 2026-08-16 shape: correct
documentation, no sentence where the person actually is.

### What is built

`cmd/usarr/backupcmd.go` (`runBackup`, `copyKEKSalt`, `reportBackup`, `masterKeyLocation`,
`masterKeyRestoreSteps`); `vacuumInto` extracted in `cmd/usarr/backup.go` and shared with the
pre-migration path; `backup` registered in the one parser's `subcommands` table
(`internal/config/flags.go`) and dispatched from `cmd/usarr/main.go` on `config.ErrBackupRequested`.

**Guards:** `cmd/usarr/backupcmd_test.go`. The two that carry the decision are
`TestBackupWritesTheDatabaseAndTheSalt` — which fails with "1 db and 0 salt files" the moment the
salt copy is dropped — and `TestBackupOutputNamesWhatItDoesNotCover`, which fails if the exclusion
paragraph stops naming the master key, its location on this install, or what to do about it.
`TestBackupNeverPrintsKeyMaterial` watches the process's real stdout and stderr as well as the
report writer, because the first version of it passed a deliberate `fmt.Printf` of the salt bytes.

---

<a id="adr-0063"></a>
## ADR-0063 — A walked container records a **zero-count skip row**; "none skipped" stops being an absence

**Status:** Accepted — 2026-08-19. **Supersedes [ADR-0061](#adr-0061) §5's absent-row semantics for
`items_skipped`.** ADR-0061's text is left standing and unreworded; its §5 carries an inline flag
pointing here, on the pattern [ADR-0052](#adr-0052) used against [ADR-0041](#adr-0041) clause 1.

### Context

#### 1 · What shipped, and the state it could not name

`sync_report.kind = 'items_skipped'` records what an adapter READ inside a container and
deliberately did not map — v0.1's case is a comic, for which UsArr has no settled unit of work.
ADR-0061 §5 stated the write rule as a decision: **a row only when something was skipped**, so an
absent row meant nothing was skipped.

`42246c0` then built the reader and the §17.8 render on top of that, and immediately met three
states where the table offered two:

| Reading | Wanted |
| --- | --- |
| Items were left out | `state: "left_out"` |
| Walked, and nothing was left out | `state: "none"` — a **measured negative** |
| Nothing walked this library | the key **absent** |

The last two are the same absence in an `items_skipped`-only table. The reader separated them by
joining against the **`content_completeness`** row, which ADR-0061 §5 does write for every container
observed — so "a library with a completeness verdict and no skip row" became the stand-in for
"walked, nothing left out".

#### 2 · Which made the neighbour load-bearing for this field's silence

That is the defect this ADR answers, and it is a coupling rather than a bug: the completeness row is
a measurement on a **different axis** — *did the credential SEE everything the container holds* —
and it was quietly doing double duty as the only per-container record in the schema that an import
had gone near a container at all. `cmd/usarr/import.go` said so out loud: *"Stop writing a row for
the clean containers and every one of those collapses back into silence."* A change to the
completeness pass, made for completeness reasons, would have silently changed what the skip column
says.

#### 3 · And two adjacent readers had opposite absence conventions

`completeness` and `skipped` render one column apart on the same §17.8 row. An absent completeness
row meant *nothing was asked*; an absent skip row meant *nothing was found*. Two opposite meanings
for the same shape of silence, side by side, is a hazard independent of the coupling above — for the
next adapter author, for anyone reading `sync_report` out of the database, and for anyone reasoning
about the screen.

The compensating control was on the same screen and was **ruled insufficient**: the reader could
cross-reference the completeness column to work out what the skip column's silence meant.
Cross-referencing is what people stop doing.

### Decision

**1 · Every container an import WALKED gets an `items_skipped` row, zero or not. A container nothing
walked gets none.** One row per container per import, mirroring `content_completeness`. Both readers
now mean the same thing by an absence: **nothing looked**.

**2 · The three states survive; only the evidence moves.** `left_out` is a row with a non-zero
total, `none` is a row with a zero total, absent is no row. The wire contract
(`reference/http-api.md` §2.6a), the store's `SkipState` vocabulary and the §17.8 render are
**unchanged** — `none` still renders nothing, which keeps that screen's standing invariant that it
paints no positive health claim.

**3 · The completeness derivation is RETIRED, not left in as a fallback.** `attachLibrarySkips`'s
second pass is deleted, and with it `ListLibraries`'s constraint that the skip read must run after
the completeness read. A dead cleverness kept "just in case" is a second code path nobody exercises
and a second explanation of the same field.

**4 · The change belongs in the ADAPTER, not in `cmd/usarr`.** `BookOrbitSource.Skipped` stops
filtering zero tallies. `recordSkippedItems` is untouched in that respect — it already wrote a row
per element it was handed. This is the load-bearing half of the placement: the tally map is
populated by `tallyFor` at the top of **one container's iteration inside the walk**, and nothing
pre-seeds it, so the set of rows is the set of containers the walk **reached**. Synthesising the
zero rows in `cmd/usarr` would mean taking a container list from `Containers()` — see Decision 5.

**5 · A zero row carries no `reason` and no `effect`.** Both sentences explain a skip; on a row
saying nothing was skipped they assert a cause for a non-event, to an operator reading `sync_report`
and to the fold that lifts `reason` onto the wire. `covers` / `does_not_cover` **do** stay on every
row (ADR-0061 §6), because *"a skip count is not a completeness verdict"* is as true of a zero as of
a thousand.

**6 · The `StreamItems` LOG keeps its zero gate.** A log line is not a record: nobody infers "this
container was walked clean" from the absence of a line in a process log, so a line per clean library
would be noise on every import and would buy the honesty nothing. The record is the row.

### The imprecision: what dies, and what is still open

Stated in both directions on purpose, because an ADR that omits the claim reads, later, as an ADR
that made it.

✅ **RETIRED.** The completeness verdict is measured in `Containers()`, **before** the walk, so an
import that dies part-way writes one for containers it never reached — and while `none` was derived
from that verdict, those containers read *"nothing was skipped"* when the truth was *"not
observed"*. Skip rows are raised from tallies created **during** the walk, so a container the walk
never reached has no row and reads as absent. The skip read no longer inherits the completeness
pass's reach. This is a property of **where the tallies come from**, so it is asserted rather than
argued: `TestSkippedNamesOnlyTheContainersTheWalkReached` walks two of three containers and fails if
the third reports.

⚠️ **STILL OPEN, and not closed by this change.** The one container the walk died **inside** does
get a row, from what it had read so far — so a container that had skipped nothing before failing
reads `none` for a partial read. At most one per import: `StreamItems` returns on the first
container error. **Why it could not be closed here.** Withholding that container's row loses genuine
skips the walk did observe and contradicts Decision 1's own invariant, since a partially walked
container *was* walked; and marking it would mean a fourth "partially observed" concept in a read
this change exists to simplify, for a state the screen has no distinct rendering for. The
compensating control is unchanged and is not this field's: an instance whose import did not finish
renders *"An import did not finish · this count may be short"* on every one of its libraries
(`web/src/lib/libraries.ts`). It is asserted, so it cannot quietly become something else — the same
test pins the dead container's row and says in words that this is the residual.

### Consequences

- **One extra `sync_report` row per container per import.** An append-only row with a small JSON
  blob, on the import path, once per container. Nothing.
- **No migration, and no new vocabulary.** The `kind` is the one that already exists;
  `sync_report.kind` carries **no `CHECK`** (migration `00005`, verified in the DDL rather than
  assumed) and `detail` is already JSON. Nothing in the schema changes.
- **No SQL and no plan changes.** `librarySkipsSQL` and `libraryCompletenessSQL` still delegate to
  one `containerReportSQL`, the four equality predicates and the `ORDER BY r2.id` are untouched, and
  `ix_sync_report_container_latest` (migration `0011`) still covers the pick.
  `TestTheSkipStatementIsTheCompletenessStatement` was **re-examined and kept** rather than assumed
  still valid: byte-identity is what lets one plan guard and its firing arms cover both reads.
- **The Libraries screen does not change.** `skipMarks` already rendered only `left_out` and
  `skippedNote` already counted only `left_out` and the absent key, so all three states landed
  identically before and after. What changed on that side is prose that had gone false.
- **One reader behaviour inverts, in the safe direction.** With the second pass gone, a library
  whose only skip row is **undecodable** now reads as absent rather than as `none`. That is a step
  further into the direction the drop was already chosen for — nil understates what is known and
  never overstates it — and it is the honest answer, because what could not be read is precisely the
  record of the walk.
- **The next adapter has one rule to learn, not two.** "A row per container you walked" is now the
  answer for both fields.

### Alternatives rejected

**Keep the coupling and rely on the same-screen compensating control.** Rejected by the owner: the
reader can already cross-reference the completeness column to decode the skip column's silence, and
*cross-referencing is what people stop doing*. It also leaves a measurement on one axis
load-bearing for a different axis's silence, which is a coupling nobody would design deliberately.

**Synthesise the zero rows in `cmd/usarr` from the container list.** This is the tempting shape,
because `recordSkippedItems` is where the rows are written — and it is exactly wrong. The only
container list available there is `Containers()`'s, i.e. **before the walk**, so every container an
aborted import never reached would get a row saying UsArr left nothing out of a container it never
opened. That does not retire the before-the-walk imprecision, it **moves it** into the field that
had just been decoupled from it. Fired deliberately during implementation: seeding the tally map
from `s.containers` produces `RemoteID:3 Name:"Never Reached"` with a clean zero, and the guard
names it.

**A fourth state for "partially walked".** Rejected for now. It is the only thing that would close
the residual above, and it buys a distinction §17.8 has no separate rendering for, at the cost of a
fourth member in a vocabulary that exists to keep three readings apart. The residual is recorded
instead. If a screen ever wants to say it, this is the seam: the writer already knows which
container the walk died in.

**Widen `sync_report` with a `walked` column or a new `container_walked` kind.** Rejected as "and
also" — a second record for a fact the existing row can carry, and a second thing that can be
missing.

### What is built

Write side: `internal/libsync/bookorbit.go` — `Skipped()` returns every tally including the zeroes;
`Containers()` gains a comment pinning that the map is **never** seeded from the container list;
`StreamItems`'s log gate keeps its zero filter and says why. `cmd/usarr/import.go` —
`recordSkippedItems` omits `reason` / `effect` on a zero row, and `recordCompleteness`'s doc records
that its row **used** to be load-bearing for the neighbour and no longer is.

Read side: `internal/store/libraries.go` — `attachLibrarySkips` loses its second pass, `foldSkips`
derives the state from the totals (`SkipsNone` until a non-zero total moves it, so row order cannot
change the answer), and `ListLibraries` loses the ordering constraint. Vocabulary corrected in
`internal/store/skips.go`, `internal/store/completeness.go`,
`internal/libsync/bookorbitcompleteness.go`, `internal/httpapi/libraries.go`,
`web/src/lib/libraries.ts` and `reference/http-api.md` §2.6a.

**Guards.** `TestSkippedNamesOnlyTheContainersTheWalkReached` is the new one and carries both halves
of the invariant plus the named residual. Four existing assertions are **inverted rather than
deleted**, because an inverted assertion records that the decision changed and a deleted one is
silence: `TestAddingABookOrbitProducesACatalogue`'s *"library 2 skipped nothing and still got %d
notes"* now demands the zero row and additionally asserts that no row exists for a container the
walk never reached; `TestLibrarySkipsTellsNothingSkippedFromNothingObserved` is renamed
`…FromNothingWalked` and its Films control — a completeness verdict with no skip row, the exact pair
that used to mean `none` — must now read nil; `TestAnUnreadableSkipRowFallsBackToTheObservation`
becomes `TestAnUnreadableSkipRowIsDroppedAndPublishesNothing`; and both `Skipped()` assertions in
`internal/libsync/bookorbit_test.go` now want a tally for every walked container. Each was fired
red before being trusted, against the code line and not a comment.

---

<a id="adr-0064"></a>
## ADR-0064 — BookOrbit's wire vocabulary is pinned by vendoring `packages/types` under `api/specs/`, guarded by an offline git-tree pin plus a network drift check

**Status:** Accepted — 2026-08-19. **Extends [ADR-0047](#adr-0047)'s two-half guard shape from one
file to a directory.** [ADR-0046](#adr-0046), [ADR-0047](#adr-0047) and [ADR-0052](#adr-0052) are
left standing and unreworded — the change carrying this decision edits no ADR at all.

### Context

#### 1 · A hand-transcription against a sole source was the whole library's single point of failure

[ADR-0052](#adr-0052) made BookOrbit v0.1's **sole** catalogue source. Every wire shape in
`internal/bookorbit` — `catalogue.go`, `scope.go`, `stats.go`, `resources.go` — is
**hand-transcribed** from BookOrbit's TypeScript, citing commit
`73b7877d2fede2221b0ca360af9bfced7c3797f3`. With one source rather than several, an upstream rename,
a retyped field or a dropped key is not the degradation of one adapter among many; it is a failure of
the **entire library**, and until this landed it was invisible until an import broke against a real
server.

#### 2 · There is no OpenAPI document to vendor, so ADR-0046's shape has nothing to bite on

BookOrbit commits no spec: `server/src/swagger.ts` builds one at **runtime**, and `main.ts` mounts it
only when `SWAGGER_ENABLED` is true, which `parseBooleanFlag` defaults to **false**. So there is
nothing checked in to vendor and nothing to fetch from a running instance either.
[ADR-0046](#adr-0046)'s floor/ceiling split assumes a committed document at two refs and does **not**
transfer; [ADR-0047](#adr-0047)'s blob-identity pin assumes a committed document at one ref and
**does** transfer, one level up the object graph. `packages/types` is the only machine-readable
statement of the wire vocabulary upstream publishes — not a second-best, the only candidate.

⚠️ §2 is a reading of an upstream repository at a pinned commit, which UsArr's own suite cannot
assert against and must not pretend to. `api/specs/SOURCES.md` carries the same caveat.

### Decision

1. **Vendor `packages/types` verbatim and whole** — all **68 files**, at `73b7877d`, to
   `api/specs/bookorbit-types/`.

2. **`api/specs/`, not `docs/reference/` — decided rather than inherited.** `docs/ROADMAP.md` raised
   this obligation under a `docs/reference/` heading and then explicitly labelled that heading an
   inference to settle. The tree settles it: `api/specs/SOURCES.md` opens *"These files are vendored
   verbatim, never fetched at build or test time"* and carries a provenance table contract tests
   read, while `docs/reference/` holds hand-written Markdown and **not one vendored artefact**.
   Filing it there would make that directory two things at once and leave `SOURCES.md` — the register
   that exists precisely to answer *which bytes belong there* — not knowing about the most
   consequential vendored artefact in the tree.

3. **The identity is a git TREE name**, `4cb990a36b8325845abb79eb4b7a4445e6df679b`
   (`git rev-parse 73b7877d:packages/types`), not a SHA-256 over a concatenation of our own devising.
   Two things follow that a home-grown hash would not buy: upstream is **comparable with no
   download**, because a blobless fetch resolves a path to its tree name out of the tree objects
   alone; and the value is **upstream's own name** for the directory, so nobody has to trust that we
   hashed the right bytes in the right order. `gitTreeName` reimplements git's naming rather than
   shelling out, so the gate does not require a git checkout to be present, and it **refuses**
   executables and non-regular files rather than guessing a mode.

4. ⚠️ **Nothing may be added inside `api/specs/bookorbit-types/`.** One extra file changes the tree
   and destroys that identity. `api/specs/bookorbit-types.manifest` therefore lives **beside** the
   directory, never in it.

5. **Two authorities and one diagnosis**, because they answer different questions. The **tree name**
   moves for *any* byte — offline that is exactly right, since those bytes are frozen and must not
   move for any reason at all. The **declaration digest** (SHA-256 over the file with comments
   stripped and whitespace outside string literals collapsed) moves only when a type, field, enum
   member or literal union changes, so an upstream comment rewrite does not read the same as an
   upstream rename; without it every reworded doc comment arrives as a full-volume alarm, and **an
   alarm that is usually noise is an alarm nobody reads** — the same failure as no alarm, reached by
   a longer road. The **manifest** — one `<git-blob>  <path>` row per file — is neither:
   `TestVendoredBookOrbitTypesAreTheUpstreamTree` reads it only to turn *"the tree hash differs"*
   into *"these files differ"*, and it is in its own words *"a diagnosis and never an authority"*, so
   editing it to match a tampered file changes no verdict.

6. **The digest is a lexer that refuses rather than guesses — which is why five files are digested
   and not sixty-eight.** `stripTypeScript` is a lexer over six states, not a parser, and cannot tell
   a regex literal from a division. `api/specs/bookorbit-types/src/pattern-resolver.ts:71` declares
   `INVALID_SEGMENT_CHARS_REGEX` as a character class that **contains a double quote**, which would
   send the scanner into a string state it never leaves and pin that file to nonsense. So the lexer
   **asserts it finished in code state** and errors otherwise, and `dependedOnDeclarations` covers
   exactly the five files `internal/bookorbit` transcribes: `src/book.ts`, `src/query.ts`,
   `src/permissions.ts`, `src/library.ts`, `src/app-info.ts`. The limit is **measured, not cautious**
   — a file that would silently corrupt its own digest fails loudly instead of being pinned to a lie.

7. **The guard is split across the network line**, on [ADR-0047](#adr-0047)'s reason: `make check`
   makes exactly two network calls and `make check-offline` is defined as `check` minus them, so a
   third would let a GitHub outage redden an unrelated commit.

   | Guard | Where | Network | Answers |
   | --- | --- | --- | --- |
   | `TestVendoredBookOrbitTypesAreTheUpstreamTree` | `internal/bookorbit`, **in `make check`** | no | have the vendored bytes moved under us? |
   | `TestVendoredBookOrbitTypesManifestIsCurrent` | `internal/bookorbit`, **in `make check`** | no | is the per-file diagnosis still accurate? |
   | `TestDependedOnTypeFilesCarryThePinnedDeclarationDigest` | `internal/bookorbit`, **in `make check`** | no | have the five transcribed files' **declarations** moved? |
   | `TestSpecDriftBookOrbitTypesStillMatchUpstream` | `internal/bookorbit`, **`make spec-drift` only** | **yes** | two questions, graded differently |

   ⚠️ **The network half asks two questions, and they are not the same news.** The tree at the
   **pinned commit** must equal what we vendored — a mismatch there is *our* bug, and it invalidates
   everything else the package asserts, since every transcription cites that commit. The tree at
   **`main`** differing is *upstream news*, and the target grades each changed file by whether
   `internal/bookorbit` transcribes it. The test emits a `SPEC_DRIFT_VERDICT:` line — `drift`,
   `unreached`, `path-moved` — as a contract with the `Makefile`, so an outage or a local git failure
   is not reported as upstream movement.

8. **`SPEC_DRIFT_FLOOR` is raised 1 → 2 in the same change, and that is what makes the network half
   non-vacuous.** `make spec-drift` counts `--- PASS:`/`--- FAIL:` lines matching the reserved
   `TestSpecDrift` prefix and fails if fewer than the floor **actually ran**, because `go test -run`
   that matches nothing exits 0 and a green over zero checks is not a clean bill of health. Adding a
   drift test without raising the floor leaves the new check free to vanish silently — hence
   `docs/DEVELOPMENT.md` §11 rule 4's requirement that the two move together, honoured here.

### Rejected

- **Vendor only the five files a transcription reads today.** Cheaper and smaller, and it silently
  un-covers the next slice: the moment somebody transcribes a sixth file the guard says nothing about
  it and nobody notices, because a guard that quietly shrank is indistinguishable from one that held.
- **A plain SHA-256 over a concatenation of the files.** Not comparable to upstream without
  downloading every byte, and it asks a reader to trust that we hashed the right bytes in the right
  order. The tree name needs neither.
- **The whole-tree hash as the *only* signal.** Correct offline, where the bytes are frozen; useless
  as the only input to the network half, for the alarm-fatigue reason in decision 5 — it fires on a
  fixed typo in a JSDoc line and on a rewrap by a newer Prettier.
- **[ADR-0046](#adr-0046)'s floor/ceiling split.** It needs a committed document at a release tag and
  at a branch tip. BookOrbit has neither a committed document nor a release-tag line to split on.
- **Fetching the spec from a running instance.** `SWAGGER_ENABLED` defaults false, so on a default
  install there is no served document at all.
- **Vendoring the NestJS server to close gap 4 below.** Not a proportionate answer to it.

### ⚠️ What this does NOT cover

1. **It pins the file we vendored, not the server the owner runs.** `packages/types` is what
   BookOrbit's own frontend compiles against at one commit. The owner's instance may be older, newer,
   or a fork. **A real instance remains the only evidence about a real instance.**
2. **It sees types, not behaviour.** A handler that stops populating a field, a route that gains a
   `@RequirePermission`, a query that silently narrows — none of those touch `packages/types`, and
   every one of them breaks the adapter.
3. **It sees what upstream declares, not whether we read it right.** A green says the vocabulary has
   not moved; it says nothing about whether the transcription was correct in the first place.
   `TestMediaKindVocabularyMatchesTheSource` and `TestPermissionVocabularyMatchesTheSource` are what
   check that, and they stay necessary.
4. **The server's own NestJS DTOs are in no file here.** Where a controller returns a class rather
   than a shared type, nothing in `packages/types` describes it, and the controller, service and
   repository citations in `internal/bookorbit` are **unpinned by anything**. That is the largest
   uncovered surface, and it is uncovered deliberately.
5. **Nothing runs the network half on a schedule.** There is no CI; `make spec-drift` is a thing a
   person types, behind `USARR_SPEC_DRIFT=1` and the `upstream` build tag. This ADR should not be
   read as claiming a cadence. The vendored copy and the offline pins stand on their own; *"we will
   hear when upstream moves"* is only as true as somebody's habit of running that target.

### Consequences

**Re-vendoring becomes a deliberate multi-part change, by design.** A newer BookOrbit commit means
updating, in one change: `vendoredTypesCommit`, `vendoredTypesTree`, the five
`dependedOnDeclarations` digests, `api/specs/bookorbit-types.manifest`, and the BookOrbit rows in
`api/specs/SOURCES.md` — and then **re-reading** the transcriptions that feed off whichever files
changed, because a moved type is exactly when a hand-transcription stops being true. The failure
messages spell that sequence out rather than inviting an observed hash to be pasted over the
constant.

**The gate grows three offline checks and no network calls**, so `make check` keeps its exactly-two
and `make check-offline` its zero. **`api/specs/` is now the settled home for vendored upstream
artefacts of any format**, not only OpenAPI JSON; `SOURCES.md` gained a BookOrbit section carrying
the provenance, the tree name, the guard split and the five uncovered surfaces above.

---

<a id="adr-0065"></a>
## ADR-0065 — The BookOrbit cover fetch runs **inside the import, between committed batches** — not on a render path, and not inside `applyOneItem`'s transaction

**Status:** Accepted — 2026-08-19. **The shape is what ARCHITECTURE.md §4.4.1 rule 4 already
specified**, and this ADR records it as such rather than as a novelty. **The LEVEL named in the
first ruling was wrong and is corrected here on a measurement**, not on taste. **No other ADR's
reasoning is edited** — [ADR-0050](#adr-0050) and [ADR-0052](#adr-0052) stand unreworded — though
the change carrying this ADR also makes two **mechanical, additive** repairs elsewhere in this file:
ADR-0050 gains the `<a id="adr-0050"></a>` anchor its existing index row was already pointing at,
and ADR-0063 gains the `---` rule above its anchor that every other ADR carries. Neither touches a
word of either decision. · ⚠️ **amended 2026-08-19** — see the block directly below: decision 5's
phrase *"a fetch bound, stated in fetch terms"* is less precise than the bound that shipped. **The
decision is unchanged**; what moves is what a reader may conclude the number means.

### ⚠️ Amendment, 2026-08-19 — the shipped bound is wider than "a fetch bound", and N permits is not N concurrent requests

**What this amendment does not touch.** Decision 5 stands as taken: concurrency **is** bounded, and
the bound **is built rather than inherited** — `internal/libsync/covers.go` carries its own constant
(`maxCoverConcurrency`), its own sentinel (`ErrCoverFetchBusy`) and its own refusal test, reached
through the deliberately exported `AcquireCover` *"because a limiter nobody has watched refuse is
indistinguishable from no limiter."* Nothing in decisions 1–4 or 6 is affected. What is imprecise is
three words: *"stated in fetch terms."*

**What shipped.** One permit is held across the whole `imagepipeline.Poster` call, not across its
network leg. The file says so in its own words, and they are quoted rather than paraphrased:

> WHAT IT ACTUALLY CAPS, stated because a bound whose subject is vague is not one — AND IT IS WIDER
> THAN "FETCHES", DELIBERATELY. One permit covers one `imagepipeline.Poster` call end to end, and
> that call is a fetch PLUS a decode PLUS a seven-width render PLUS seven cache writes. Poster's
> duration is in fact dominated by the CPU half, under `internal/imagepipeline`'s 32 MiB / 64 MP
> ceiling. So this gate bounds concurrent TRANSCODES — which is what §4.4 asks for and what
> jellyfin#9795 is about — and bounds concurrent connections only as a consequence of the two being
> one call.

**The consequence worth recording, in the file's own capitals:** *"SO N PERMITS IS NOT N CONCURRENT
REQUESTS, and a reader sizing this against an upstream must know it."* The same header names why the
gap is not merely notional — `internal/bookorbit`'s client *"re-mints its access token and RETRIES
ONCE on a 401, so a single Poster can be up to three wire exchanges — mint, request, re-minted
request — none of which this gate can see."* **Bounding the upstream's request rate is therefore a
different job with a different instrument**, and it belongs in the client that issues the requests,
not in this loop.

**What decision 5 should have said:** *the bound is the import loop's own, with its own constant and
its own refusal test, and it caps concurrent `Poster` calls — which is primarily a transcode bound
and only incidentally a connection bound.* That wording would have left the "Where this decision's
own stated constraint was wrong" section below reading exactly as it does; the correction it records
— that a bound may not be **borrowed** from §4.4's prose — is untouched and still holds. ⚠️ **One
thing that section could mislead a reader about is worth stating plainly**: the shipped **number** is
`min(NumCPU, 4)`, the same figure §4.4 names, and the file records it as *"§4.4's own figure adopted
deliberately rather than copied"* — adopted on the transcode citation (jellyfin#9795, *"and Poster
transcodes"*), which is the resource the permit actually covers, rather than borrowed as a fetch
limit. The Rejected bullet *"Borrowing §4.4's `min(NumCPU, 4)`"* refuses the **derivation**, not the
digit.

**No decision text below is rewritten**, per this file's own rule at the top: the original sentence
in decision 5 stays readable and carries a dated inline flag.

### Context

#### 1 · A complete pipeline with no caller, waiting on one shape decision

`internal/imagepipeline` fetches one cover, decodes it, renders every width on §4.4's allowlist,
writes the bytes through `internal/imagecache` and records the row through
`internal/store.PutPosterAsset`. Its package doc states its own honest end state: *"complete,
tested, and waiting on ONE call site that is a shape decision about UsArr rather than either lane's
to pick."* Nothing in the tree calls it — measured, not assumed: `imagepipeline.` appears in no file
outside the package. This ADR is that decision, and nothing else.

#### 2 · A render path is disqualified by principle 1, and §4.4.1 rule 4 already said where it goes

Covers are the bottleneck the architecture is written around — §4.4 measures a 60-item viewport at
~5–9 MB per screenful against ~30 KB of JSON, and §4.4.1 budgets a cold start of 10k posters. A
per-work on-demand entry point would put a synchronous BookOrbit round trip on a render, which is
what CLAUDE.md's first principle refuses in as many words and what [ADR-0004](#adr-0004) exists to
prevent. **The design already answered this**, in §4.4.1 rule 4:

> **Progressive rendering.** The grid paints from `work` rows as import phase A commits (§7.2);
> images fill in behind, and the grid is never blocked on the image queue.

"Images fill in behind" is a producer running in the import. So the fetch belongs to the import, and
that half of this decision is a **restatement**, not a ruling — the shape the design specified.

**Two in-tree precedents put it in the same place, and both were built before this question was
asked.** They matter because they are the shape already load-bearing in this tree rather than an
argument from first principles:

- **Credits were moved OUT of the stream callback**, deliberately, and `FullImport`'s phase 3 says
  why: *"it is after the stream rather than inside it because Kavita reports no creator on the
  series list, so credits cost one HTTP GET per series and issuing those from inside the stream
  callback would hold the streaming connection open across all of them."* A per-item HTTP read does
  not belong inside the machinery that is streaming the items.
- **`PutPosterAsset` opens with *"The bytes are already on disk when this runs"***
  (`internal/store/imagewrite.go`). The store write was designed on the assumption that the fetch,
  the decode and the render all happened **before** the transaction opened. Putting the fetch inside
  a transaction would falsify that first sentence.

#### 3 · ⚠️ The level was wrong in the first ruling, and the correction is MEASURED

The original ruling named **`applyOneItem`** as the call site. That is where the work↔book-id pairing
lives, which is why it looked right — `internal/imagepipeline`'s own package doc says the pairing
*"exists in exactly ONE lexical scope in this tree — applyOneItem."* It is wrong anyway, and the
numbers are in the tree rather than in a judgement:

`applyOneItem` runs inside `ApplyCatalogueBatch`'s **single `BEGIN IMMEDIATE` transaction**, which
spans a whole batch — `min(2000 rows, 100 ms)` (`reference/sync.md` §6 rule 3, restated on
`ApplyCatalogueBatch` itself). The writer pool is hard-capped at **one connection**
(`internal/db/sqlite.go`: `write.SetMaxOpenConns(1)`, commented *"Exactly one. This is the entire
single-writer discipline."*). A fetch there would hold **the process's only writer across a network
round trip, per book**, serialising the entire import — and every interactive write in the process —
behind BookOrbit's latency. The prohibition is already written down, one line above the function
that would have hosted it:

> `fn` must not call `Write`, and must not hold the transaction across a network call: the whole
> process shares one writer connection.

**The pairing is not needed, which is what dissolves the problem rather than trading it away.**
`store.PosterAsset` is keyed by `RemoteKind` + `RemoteID` and **re-resolves the work through
`service_item_link` inside its own transaction** — and its header already gives this exact reason,
quoting `CreditSet` verbatim and then sharpening it:

> ⚠️ IT IS KEYED THIS WAY AND NOT BY A WORK ID, FOR `CreditSet`'s REASON VERBATIM (`credits.go`):
> "Carrying a work id across the two passes would mean trusting an id read in a different
> transaction, and an item deleted between the two passes would then have its credits written onto
> whatever now holds that id." The same hazard is worse here, because the gap between resolving the
> id and writing the row is not another local pass — it is a COVER FETCH, a network round trip
> against a service that may be slow.

`work.id` is `INTEGER PRIMARY KEY`, so SQLite may reuse a deleted row's id; a stale id is therefore
not "no such work" but **the wrong book's cover**, silent and visible only as art that does not
match its title. So a call site that has a work id in hand is not an advantage here — it is the
hazard.

### Decision

1. **The cover fetch runs during the IMPORT.** There is no per-work on-demand entry point in v0.1.
   §4.4.1 rule 4 is the authority; principle 1 and [ADR-0004](#adr-0004) are why.

2. **It runs in `internal/libsync`, between committed batches — never inside a store transaction.**
   `ImportedItem` is the carrier that already exists, and it already carries exactly what is needed
   and nothing more: `RemoteKind`, `RemoteID`, `Kind`, `RemoteSubtype`, and **no work id, by
   design**. `FullImport` already runs two per-item passes after the stream closes on precisely this
   shape — credits (phase B) and the file walk (phase C) — and a cover pass is a fourth of the same
   kind, not a new mechanism.

3. **Keyed by remote id, with the work re-resolved inside the store's own transaction.** No work id
   crosses the fetch. `PutPosterAsset` already does this and needs no change.

4. **A cover-fetch failure NEVER fails the import.** A partial catalogue that says it is partial
   beats no catalogue — the rule [ADR-0061](#adr-0061) and [ADR-0063](#adr-0063) already apply to
   completeness and skips, applied here. The items that imported are correct either way, and a
   missing poster degrades to §4.4.1's own fallback (title and year over a colour fill) rather than
   to a failed sync. `Report` already has the shape for this: `FileReadFailures` is documented as
   *"NOT an import failure"* and does not stop `last_full_sync_at` being stamped.

5. **Concurrency is BOUNDED, and the bound is BUILT rather than inherited.** `imagepipeline.Poster`
   is one synchronous fetch that holds no lock and no pool; its doc says *"Whoever writes the loop
   owns the bound, and owes it a test that N+1 is refused."* This ADR accepts that debt on the
   import's behalf. The bound is a fetch bound, stated in fetch terms, with its own constant and its
   own refusal test. ⚠️ **Imprecise as of 2026-08-19** — the shipped permit is held across fetch,
   decode **and** render, so N permits is not N concurrent requests; see the amendment block above.
   The rest of this clause holds exactly.

6. **A 404 is "absent for this credential", never a terminal verdict.** `bookorbit/cover.go`
   enumerates four conditions that all produce one `NotFoundException` — no cover file; no such book
   id; **a book the credential's content filters hide**, deliberately made indistinguishable from an
   absent one; and the catch-all. So a 404 is **not evidence about a file**, and the next import
   retries rather than inheriting a permanent verdict. ⚠️ This deliberately NARROWS `cover.go`'s own
   sentence *"caching absence on a 404 is sound"*: that reasoning holds for the three book-shaped
   conditions and is defeated by the content-filter one, which is a fact about the **credential**
   and can change without the book changing. [ADR-0061](#adr-0061) already established that a
   content filter makes an upstream absence a statement about scope rather than about content; this
   is the same finding one route along.

### ⚠️ Where this decision's own stated constraint was wrong

**Recorded rather than quietly corrected, because a record that shows where its own reasoning failed
is worth more than one that ships only the fixed version.** The bound in decision 5 was first
justified by pointing at §4.4's existing semaphore. That was wrong, in the author's own words:

> "§4.4's semaphore is scoped to transcoding… fetch concurrency needs its own bound, stated in its
> own terms, not borrowed from a limit that exists for a different resource."

And on the level, from the same correction:

> "the level was the part of my answer I was least entitled to; the shape is the part I was arguing
> for."

§4.4's `min(NumCPU, 4)` is **prose**, and it is about **transcoding**: *"all transcoding behind a
`min(NumCPU, 4)` semaphore (jellyfin#9795 is exactly this failure)"*. `internal/crypto/password.go`
says the same of it from the other side — *"that is a design statement for a pipeline that is not
built yet, so it is the shape being reused, not shipped code being copied."* A fetch bound and a
transcode bound cap different resources (sockets and upstream load; peak decode memory and CPU), and
one number serving both is a number serving neither.

⚠️ **One supporting claim was checked and is FALSE, so it is not repeated here.**
`internal/imagepipeline/pipeline.go` states that *"no such semaphore is implemented anywhere — the
tree's only one is the Argon2id gate."* The **first** clause is true; the **second** is not.
`internal/releases/search.go` bounds its indexer fan-out with `sem := make(chan struct{},
s.cfg.Concurrency)` at `DefaultConcurrency = 6`, landed in `dd15d95` — **before** that comment was
written. It is a bound over concurrent **network legs**, which makes it the closest precedent this
tree has for decision 5 and the opposite of an argument against it. The stale comment is left for
its owning lane; this ADR simply does not lean on it.

### Rejected

- **A per-work on-demand fetch, triggered by a render or by a work-detail route.** It is the shape
  §4.4.1 rule 4 and principle 1 both refuse. `imagepipeline.Poster` stays per-work so that a future
  trigger is not blocked — the seam ships, the feature does not — but nothing renders through it.
- **The fetch inside `applyOneItem`.** The original ruling. Refused on the measurement in context 3:
  one writer connection held across a network round trip per book.
- **The fetch inside the `StreamItems` callback.** Refused for `FullImport` phase 3's reason
  verbatim: it holds the streaming connection open across every per-item HTTP read.
- **Carrying a work id from the import into the fetch.** It is what `applyOneItem` appeared to
  offer, and it is the hazard `PosterAsset`'s header names — an id read before a network round trip
  is an id read arbitrarily long ago, and `INTEGER PRIMARY KEY` reuse turns that into the wrong
  book's cover.
- **Failing the import on a cover error**, and **caching a 404 as permanent**. Decisions 4 and 6.
- **Borrowing §4.4's `min(NumCPU, 4)`.** The self-correction above.

### ⚠️ What this does NOT decide

1. **Whether whole libraries are hidden from the account.** [ADR-0061](#adr-0061) recorded that
   `LibraryAccessGuard` throws a byte-identical `ForbiddenException('No library access')` for *"the
   library exists and this account has no access row"* and for *"there is no such library"*, so the
   question is **unanswerable from a read-only account**. A cover 404 does not move it either way,
   and decision 6 is careful to claim only that a 404 is not terminal — not that the missing set is
   knowable.
2. **The exact permit count, and where in `FullImport` the pass sits.** Both are implementation, and
   both are constrained rather than free: the rollup flush must stay last among the writers
   (ARCHITECTURE §6.3), and the file walk must stay the last pass that touches a work before it,
   because it sets `work.rollup_dirty`. **`PutPosterAsset` sets no `rollup_dirty`** — verified in
   `internal/store/imagewrite.go`, which writes `image_asset` and `work.poster_asset_id` and nothing
   else — so a cover pass does not compete for that slot.
3. **Whether the fetch is retried within one import.** Decision 6 says the *next* import retries; it
   says nothing about a second attempt inside the current one.
4. **Anything about a second catalogue source.** `imagepipeline.CoverSource` names `bookorbit.Cover`
   and is BookOrbit-shaped on purpose (its own doc records that trade against
   [ADR-0052](#adr-0052)). This ADR inherits that scope and does not widen it.

### Consequences

**`internal/libsync` gains an import of `internal/imagepipeline`, and that direction is acyclic** —
`imagepipeline` imports `internal/bookorbit`, `internal/imagecache` and `internal/store`, and not
`libsync`.

**The pipeline's package doc stops being true in one clause and will need amending by whoever lands
the caller**: *"AND THERE IS NO CALLER"* is a measured statement with a date on it, and the same doc
already carries a standing note that a sentence in it has been falsified three times. Its harder
caveat is **untouched by this ADR and stays exactly as loud**: this pipeline *"has been TESTED
AGAINST A FAKE FETCHER AND NEVER AGAINST A REAL COVER."* Deciding where the fetch runs changes
nothing about that, and nothing here should be read as claiming a byte from a running BookOrbit has
been through it.

**The cold-start budget in §4.4.1 is now owed a producer that respects it.** Viewport-prioritised
fetching and smallest-size-first (rules 1 and 2) are not satisfied by an import-time pass alone;
this ADR decides where the fetch runs, and the priority queue those rules describe remains unbuilt
and unclaimed.

---

<a id="adr-0066"></a>
## ADR-0066 — A wholly skipped BookOrbit library is **bound with an honest zero**, never declined; and the whole-library sentence says **what happened**, not just how many

**Status:** Accepted — 2026-08-19. **Builds on [ADR-0063](#adr-0063)** — the zero-count skip row is
what makes the empty library representable at all — and **amends no ADR's reasoning**: ADR-0061,
ADR-0063 and ADR-0065 stand unreworded, and this change edits no other entry in this file.
**It is the ADR `internal/libsync/bookorbit.go` asked for**, in the `bookKind` doc's own words:
*"Splitting one container into two libraries is a deviation from §17.8 that needs an ADR, and it is
comics' slice to ask for."* ⚠️ **Nothing here is built.** This ADR rules behaviour a later slice
implements — the §17.8 screen and the comics slice — and every sentence below describing what a user
sees is a specification of intent, on §17.8's own standing warning.

### Context

#### 1 · Comics are skipped during the import today, and the container is not

Measured, not recalled. `internal/libsync/bookorbit.go`'s `StreamItems` switches on the book's media
kind and a comic returns without reaching `fn`:

```go
case bookorbit.MediaKindComic:
    tally.Comics++
    return nil
```

The package doc states it as a standing gap — *"COMICS. A comic-format book is SKIPPED AND COUNTED,
never guessed at"* — and gives the reason, which is a unit-of-work question rather than a parsing
one: BookOrbit's series have no library and a book can belong to several of them, so the series work
a comic would hang under **has no container to bind to**, and a wrong `work.kind` is written once at
ingest and can never be merged away (§6.4's cascade).

**The container is a different matter, and nothing declines one.** `Containers()` reads
`GET /api/v1/libraries` and gives every library the `book` kind, and its own doc says why that is not
a Kavita-style judgement: *"NOTHING IS DECLINED HERE… mapLibraryType declines Kavita's Image type
because Kavita SAYS what a library holds and UsArr has no kind for that answer. BookOrbit says
nothing, so there is no answer to decline on."* BookOrbit's `libraries` table has no `type`, `kind`
or `mediaType` column. So the judgement is **per book, inside the walk**, and every container
`Containers()` reports reaches `store.BindContainers` → `bindOneContainer` and becomes a library row
regardless of what the walk later makes of its contents.

#### 2 · The record for the empty case already exists, so this is a copy and a policy question

[ADR-0063](#adr-0063) gives **every container an import walked** an `items_skipped` row, zero or not,
and reserves the absence for *nothing walked this container*. A library whose every item is skipped
therefore already produces a `left_out` row whose total equals everything the walk read. **No new
kind, no new column and no new state is needed to represent it** — which is why this ADR can be about
what the screen says rather than about what the schema holds.

#### 3 · On the owner's install this is a whole library, not an edge case

**The owner's BookOrbit keeps comics and prose in separate libraries** — his words, 2026-08-19:
*"libraries are split up."* Recorded as a statement about **his install**, not as a property of
BookOrbit, which declares no container type at all (context 1) and permits a mixed one. So on the
install v0.1 is proven against, a comics library **walks, reports, and yields nothing**. It is not a
corner the design may leave rough: it is one of his libraries, on the screen, every day.

#### 4 · And the copy that ships today is written for the partial case, in two places

Both are measured, and the first is plainly false rather than merely imprecise:

- **`skipEffect` asserts a remainder that does not exist.** `cmd/usarr/import.go`'s constant ends
  *"…and this library's item count is short by that many; **every other book in the library was
  imported**"*. On a wholly skipped container there is no other book. It reaches an operator reading
  `sync_report` rather than a browser — `reason` is the half that travels to the cell, on §9.1's
  split — which bounds the blast radius and does nothing about the wrongness.
- **The rendered mark is a count under a "some" word.** `skipMarks`
  (`web/src/lib/libraries.ts`) draws the word *"Some items were left out"* over the detail
  `` `${items} items were read and not mapped` `` plus the wire `reason`, and ARCHITECTURE §17.8
  specifies exactly that sentence as the tenth state's rendering: *"Some items were left out — 42
  items were read and not mapped; UsArr maps prose books only"*. **Every word of it is right for a
  partial library.** *"Some"* is wrong when it was all, and a bare count with no explanation of the
  whole is the shape a reader has to guess at.

### Decision

**1 · A container whose every item is skipped is BOUND, and no adapter declines one for being
wholly skipped.** `Containers()` keeps reporting it, `bindOneContainer` keeps binding it, and the
library renders on §17.8 with an item count of zero and a sentence. This **ratifies what the code
already does** rather than changing it — which is the point of recording it: the tempting change is
the one that makes the row go away, and it is now closed rather than merely unbuilt.

**2 · The whole-library case gets its OWN sentence, and that sentence says what happened.**
Specification, in `skipMarks`'s existing word/detail shape (§9.1's split: the word is the state, the
detail is the evidence and the why):

- **word** — *"Nothing in this library is imported yet"*
- **detail** — *"All 412 items were read. UsArr does not import comics yet."*

**Three properties are acceptance criteria rather than taste**, and an implementation that keeps the
meaning may move the wording:

  1. **It asserts the read succeeded, with the count as evidence.** *"All N items were read"* is a
     positive statement about the connection, which is the half a bare shortfall cannot make.
  2. **It names the media type and the reason.** Not *"not mapped"* — *comics*, and *not imported
     yet*. A reader who does not know what UsArr maps learns it here.
  3. **It must be impossible to mistake for a connection failure.** That is the test to apply to any
     rewording, and it is the whole reason this state is not left to the partial sentence.

§13's ban on the mid-sentence em-dash beat in UI microcopy applies here as to every string in §17,
which is why the detail is two plain sentences.

**3 · The condition is DERIVED from fields that already ship. No wire field is added.**
`item_count == 0` **and** `skipped.state == "left_out"` **and** `skipped.items > 0`.
`libraryResponse.ItemCount` and `librarySkipsResponse.Items` both exist today
(`internal/httpapi/libraries.go`; `reference/http-api.md` §2 and §2.6a). **The empty upstream library
stays distinct and needs no care**: it walks clean, so its row is `state: "none"` with `items`
absent — the wire contract's *"Absent under `none`"* — and it renders nothing, which keeps §17.8's
standing invariant that no positive health claim is painted on that column.

**4 · `skipEffect`'s remainder clause is FALSE on this shape and is recorded as false here**, rather
than left to be discovered on a screen. Fixing it belongs to the slice that implements decision 2;
this ADR's job is that nobody later reads that sentence as a decision somebody took about the whole
case.

**5 · A MIXED container becomes TWO library records on UsArr's side, and that shape is decided
here.** `library.kind` is `TEXT NOT NULL CHECK (kind IN ('movie','series','artist','album','book',
'comic','game'))` — *"Exactly one, required"* (migration `00005_library_sync.sql`) — so once comics
have a unit of work, one upstream container holding prose and comics **cannot** be one UsArr record.
It becomes a `book` library and a `comic` library over the same `library_source` container ref.
**This needs no migration**: `'comic'` is already a permitted kind and `library_source`'s uniqueness
is `(library_id, service_instance_id, container_kind, container_ref)`, so two libraries may name the
same container. **Until comics import, a mixed container binds exactly as it does today** — one
`book` library, its comics counted as skipped — which is decision 2's *partial* case and renders the
partial sentence, correctly.

**6 · Decision 5 is DESIGNED AND UNTESTED, and that is not the same as unsupported.** The owner's
libraries are split (context 3), so **his data will never exercise it** and no import against his
server can confirm or refute it. Stated plainly because the alternative is that a later reader finds
no evidence for it and concludes it does not work. **The precedent for the distinction is this
project's own synthetic-cassette rule** — `internal/bookorbit/vcr_test.go`: *"A SYNTHETIC CASSETTE
PROVES THIS CLIENT'S PARSING, NOT THE SERVER'S BEHAVIOUR"* — where a green is evidence **about the
spec we transcribed** and never about the running server, and the tests are kept and trusted for
exactly what they cover. Decision 5 has the same grade: buildable, fixture-testable, and standing on
no byte of real data.

### Why

**Declining makes it invisible, and invisible reads as broken.** A declined container produces no
`library` row, so §17.8 — which renders `library` joined to `library_source` — shows **nothing at
all**. The owner then sees a BookOrbit that connected green on Services, a library he knows exists
upstream, and no row for it, with nothing on the screen to tell that apart from a connection that
half-failed. Principle 3 is written against exactly this: *every feature degrades honestly when a
service is absent — it says what is missing and why, rather than rendering an empty screen that looks
broken.* A zero-count row with one sentence is that honest degradation; an absence is the empty
screen.

**And the decline would not even be recorded where anyone looks.** `container_declined` rows are
written (`internal/libsync/importer.go`) and **read by nothing** — measured: the string appears in no
file under `internal/store`, `internal/httpapi` or `web/`, its one other occurrence being a comment
in `internal/store/skips.go` distinguishing it from this kind. So declining moves the fact into a
table with no reader, which is the opposite of the direction ADR-0063 just moved this field.

**The count is not the message, and the same sentence changes meaning at the boundary.** *"42 items
were read and not mapped"* is a report **on the remainder**, and it is right precisely because there
is a remainder — it reads as *some of this arrived*. When 42 is the entire library there is no
remainder for it to be about, and the identical sentence reads as *this is broken*. That is a
property of the sentence and not of the reader, which is why decision 2 is a second string rather
than a threshold on the first.

**It is also where every neighbouring verdict already sits.** The ninth state (a content-filter
shortfall) and the tenth (items left out) both render **on a bound library's row**, greyed or ambered
by whether anything is actionable. A skip is UsArr doing what it was built to do — grey, nothing to
go and fix — and a whole-library skip is the same fact at a different size. Putting it anywhere other
than that row would need an argument this ADR does not have.

### Alternatives rejected

**Decline the container, so no library is created.** ⚠️ **This is the alternative this ADR exists to
close**, and it is tempting for a real reason: a library with nothing in it looks like clutter, and
Kavita's adapter already declines containers it has no kind for, so the machinery is right there. It
loses on three counts. **(a)** Invisible reads as broken — the Why above, and principle 3.
**(b)** The decline lands in `container_declined`, which nothing reads, so the user is not told;
UsArr would be silently dropping a library the owner can see upstream. **(c)** It makes the state
**unstable across a release**: the day comics import, a library appears out of nowhere carrying a
full catalogue, and nothing on the screen ever said it was there. Binding it now means that day
changes a zero to a number on a row the user has already seen — which is a catalogue growing, not a
library materialising.

**Bind it, but render nothing** — treat "everything skipped" as the `none` silence. Rejected, and it
is backwards rather than merely thin: `none` is §17.8's *measured negative*, the silence that means
**nothing was left out**, and using it for a library where **everything** was left out would make the
column's one honest silence say the opposite of the truth. It also collides with the invariant that
that column paints no positive claim, by making its silence carry one.

**Reuse the partial sentence and let the count speak.** Decision 2, and the second Why paragraph:
the sentence is a report on a remainder, and at this boundary there is none.

**Add a wire field — `whole_library: true`, or a third `SkipState` member.** Rejected as "and also".
The condition is derivable from two fields that already ship (decision 3), and a second field is a
second thing that can be absent, can disagree with the first, and must be kept true by every future
adapter. ADR-0063 declined a fourth state on the same ground and this is the same vocabulary.

**Give the wholly skipped comics library `kind = 'comic'` today.** Rejected. `bookKind` is a constant
*"BECAUSE BOOKORBIT SUPPLIES NO CONTAINER-LEVEL KIND AT ALL"*, so a `comic` kind here could only be
inferred **from the tally** — from what the walk happened to read — and §6.4's cascade means a wrong
`work.kind` written at ingest can never be merged away. The kind stays `book` until comics have a
unit of work; decision 5 is what changes it, on evidence rather than on a guess.

### Consequences

- **No migration, no column, no DDL and no new wire field.** The row, the state vocabulary and both
  read fields already exist; what this ADR adds is a rendering rule and a policy.
- **Two strings are owed by the implementing slice**, and only two: §17.8's tenth state gains a
  whole-library arm alongside the partial sentence it already specifies, and `skipEffect` loses the
  remainder clause decision 4 marks false. Both are copy in files that already exist.
- **The owner's Libraries screen gains a row it would not otherwise have had** — his comics library,
  at zero, saying why. That is the entire user-visible effect of this ADR on his install, and it is
  the intended one.
- **One residual, recorded rather than closed: the row's kind is `book` and its contents are
  comics.** Decision 5's rejected alternative explains why no better kind is available today, and
  `library.kind` is *"EDITABLE (§6.5 rule 4)"* in the schema's own comment, so a user is not stuck
  with it. It closes when comics get a unit of work.
- **Decision 5 ships as a decision and not as code**, and carries decision 6's grade permanently: any
  future claim that the two-record split "works" must name the fixture it was proven against, never
  an import.
- **The next adapter inherits one rule**, and it is short: *a container you walked is a library you
  bind, whatever the walk made of its contents.* The counting rule (ADR-0063) and the binding rule
  now say the same thing about a container the import reached.
  🔻 **2026-08-19 — that sentence is ruled to mean BIND VERSUS DECLINE, and nothing wider.** It is
  ambiguous between *"you never decline a container over what was in it"* and a broad ban on a
  library's **count or kind** depending on observed contents, and the narrow reading is the one that
  governs. ⚠️ **No status mark, and no decision of this ADR changed** — this records which of two
  readings was already the ratified rule, it does not amend one.
  **(a) The broad reading would make this ADR contradict itself.** Decision 5's own implementation
  mints the `comic` sibling **conditionally on observed contents**, and argues that it must:
  `internal/store/catalogue.go:1382-1389` — *"Minting the comic library at bind time would therefore
  give EVERY prose-only library a permanently empty comic sibling on the Libraries screen, which is
  the 'empty screen that looks broken' principle 3 exists to prevent."* Under the broad reading,
  decision 5 violates this sentence. A reading that leaves the document coherent beats one that makes
  it self-contradictory.
  **(b) This is a Consequence, not a Decision.** Consequences describe what follows and hand rules to
  the next adapter; they do not legislate a constraint the Decision section never states, and no
  decision above says a library's count or kind is fixed before the walk.
  **(c) The sentence's evident purpose survives intact.** Do not drop a container on the floor because
  you disliked its contents — under the narrow reading nothing is declined, every walked container
  still gets a row, and decision 1 is untouched.
  **(d) This ADR named its own expiry and we have reached it.** *"The kind stays `book` until comics
  have a unit of work"* (`:9532-9533`), and the book-over-comics residual *"closes when comics get a
  unit of work"* (`:9548`). [ADR-0068](#adr-0068) discharges that condition in those exact words —
  *"and **this ADR is that unit of work**"* (`:9951`) — so the interval in which contents could not
  inform a kind is over.

---

<a id="adr-0067"></a>
## ADR-0067 — A pasted BookOrbit magic link is **accepted and reduced to its token**; the refusal is the fallback, not the rule

**Status:** Accepted · **2026-08-19** · **Reverses a ruling taken the same morning, on a premise that
was measured and found false — BOTH rulings are recorded below**, because a reader who sees only the
second one cannot tell whether the first was careless or correct-on-bad-evidence, and it was the
second · **Found by a live failure on the owner's install**, not by review — the owner added a
BookOrbit and the mandatory connection test came back 401 · **Ships code**: `serviceCredential` in
[`internal/httpapi/services.go`](../internal/httpapi/services.go) and its guards in
`services_credential_test.go` · **No migration, no schema change, no new configuration key** ·
**Leaves [ADR-0060](#adr-0060) standing and unreworded** — that ADR measured what BookOrbit does with
a token it has issued, and nothing here touches it · **Measurements cite a COMMIT, not a tag**:
everything below was read from `bookorbit/bookorbit` at
`73b7877d2fede2221b0ca360af9bfced7c3797f3`, cloned and read on 2026-08-19. ⚠️ **The tag `v2.6.0` was
NOT verified to point at that commit** and is deliberately not named as if it did.

### Context

#### 1 · The occasion is a 401 nobody could act on

Adding a BookOrbit failed the mandatory connection test (ARCHITECTURE §11.3, §17.7) with a 401 from
`POST /api/v1/auth/magic-links/login`. The credential pasted was the whole magic link. On the wire
that is indistinguishable from a revoked, expired, deactivated or simply unknown token — the login
route hashes what it is given and looks the hash up, so a *wrong-shaped* value and a *dead* value
produce the same status and the same body. `bookOrbitTestAction`'s advice for that status was
therefore aimed at the wrong fault, and the operator was told to go and change something that was
already correct.

**The distinction is available in exactly one place: before the value is sent.** After the send,
nothing downstream can recover it. That is the whole argument for a check at the API boundary, and
it is common to both rulings below — what the two rulings disagree about is what the check should
then DO.

#### 2 · The first ruling, and the premise under it

The first ruling, shipped in `ab9e0f3`, was **refuse and name which half to paste**: a BookOrbit
credential containing `://` or `token=` was rejected with a message telling the operator to paste
only the value after `token=`. Its stated reason was that guessing which half of a pasted string is
the secret is *"the inventing configuration on the user's behalf that `normalizeServiceURLBase`
already declines"* — and behind that sat a premise, which the code comment stated as fact:

> the copy button on the screen the user is told to use yields a URL, while
> `POST /api/v1/auth/magic-links/login` wants the bare token

Read as *"BookOrbit hands its operator an artefact its own API cannot consume"*, that premise makes
the URL an upstream oversight, a pasted URL a user error, and refusal the honest answer. **It is
false.**

#### 3 · What falsified it: `/magic` is an implemented adapter, not a dead link

Read at `73b7877d2fede2221b0ca360af9bfced7c3797f3`:

- **`client/src/router/index.ts`** declares `{ path: '/magic', name: 'magic-link-login', meta: {
  public: true } }`. It is a real, public, named route.
- **`client/src/features/auth/MagicLinkLoginView.vue`** reads `route.query.token`, immediately
  `router.replace`s it away — its own comment: *"Strip token from URL immediately to prevent leaks
  via browser history/referrer"* — and hands the value to `loginWithMagicLink`.
- **`client/src/features/auth/composables/useAuth.ts`** posts it: `fetch('/api/v1/auth/magic-links/login', { method: 'POST', body: JSON.stringify({ token }) })`.

So **URL in, bare token out is a reduction BookOrbit already implements**, on the route its own copy
button's URL points at. The link is not a mis-copied token; it is the artefact the product means to
hand an operator, and the token is the payload it carries.

**One adjacent claim was checked at the same time and holds**: there is **no header-borne magic-link
credential anywhere in `server/src`**. `AuthController.loginWithMagicLink` takes
`@Body() dto: MagicLinkLoginDto`, whose only field is `token: string` (`@IsString`, `@MinLength(1)`,
`@MaxLength(512)`), and a search of `server/src` for a magic-token header name returns nothing. Any
wording that says or implies the login route reads a header would be wrong; none was found in this
tree (see Consequences).

#### 4 · The settings screen offers the operator nothing else

This is what turns "the URL is legitimate" into "refusing it is hostile". In
`client/src/features/settings/MagicLinksSettings.vue`:

- `getMagicUrl(rawToken)` returns `` `${window.location.origin}/magic?token=${rawToken}` ``, and
  `copyMagicUrl` is what the row's only credential-shaped button calls.
- **The raw token is never rendered.** The table's columns are the label, the account, the creator,
  the expiry and the use count. `rawToken` arrives in the JSON and is passed to the copy handler; no
  cell prints it.
- **Not even at creation.** `handleCreate` awaits `createToken` and closes the dialog. There is no
  show-once panel.

So an operator following BookOrbit's UI **cannot obtain a bare token at all** — only the link. A
refusal therefore instructs them to hand-edit the only artefact the upstream will give them, before
pasting it into a field that could have done the edit itself. *Explicit over clever* is a good rule
and it loses here, because the "user error" it would be policing is the upstream's designed
workflow.

#### 5 · Where the accepted shape comes from

`MagicLinkRepository.create` mints `randomBytes(32).toString('hex')` — **64 lowercase hex
characters**, Node's hex encoder being lowercase — and stores `sha256(rawToken)` alongside it.
That single line is the only definition of the shape; nothing in this decision infers it.

### Decision

1. **Two shapes are accepted for a `bookorbit` credential, and only two**: a bare token that is 64
   lowercase hex characters, or a URL whose `token` query value is one. The second is reduced to the
   first, and everything downstream sees only the first.

2. **Everything else is refused, with a message naming both shapes that work.** The refusal built by
   the first ruling is not deleted — it becomes the fallback the accept path leans on. Without it a
   mistyped credential still reaches the login route and still comes back as an unactionable 401.

3. **Only the extracted token is sealed and sent.** Guarded on all four credential-bearing endpoints
   — create, PATCH, test-unsaved, test-saved — and asserted against the *decrypted* envelope, not
   only against the connection tester, because sealing a URL is invisible afterwards: the credential
   is never returned to the browser, so no screen could ever show that the wrong half is inside it.

4. **The URL is PARSED with `net/url`, never cut with string surgery**, so the query value is
   percent-decoded once, by the rules a browser would apply, rather than by whatever an index-of
   would have implied. A scheme is not required — a link pasted without one still parses.

5. **Two different `token` values in one paste is a refusal, not a first-wins guess.** This is the
   one part of the first ruling's reasoning that survives on its own terms: where a paste has no
   single right answer, UsArr does not invent one.

6. **The submitted value is never echoed back** — not in the refusal message, not in its action, not
   in an audit row, not in a log attribute. A pasted magic link carries two things worth keeping out
   of a log: the token, and the operator's own BookOrbit hostname. The message says what was
   expected and says nothing about what arrived.

7. **The rule is BookOrbit's alone.** A Prowlarr or Kavita key is opaque to UsArr, so nothing here
   may refuse one and nothing here may rewrite one; a key that happens to contain `token=` reaches
   the wire byte for byte. `TestTheMagicLinkShapeRuleIsBookOrbitOnly` pins that property, and it now
   pins both halves of it — no refusal leaks, and no extraction leaks either.

8. **The whitespace trim from `ab9e0f3` stands and runs FIRST**, before shape detection: a paste's
   surrounding whitespace is not part of its shape.

### ⚠️ What this costs: the acceptance rule is a WHITELIST

Stated plainly because it is the real price and it is not the one the first ruling paid. Before this
change, an unrecognised BookOrbit credential was passed through and failed upstream. Now anything
that is neither accepted shape is refused **locally**, so a BookOrbit that changed its token format —
longer, base64, prefixed — would have UsArr refuse a *valid* credential and make adding the service
impossible, where the old code would merely have produced a confusing 401.

That trade is taken deliberately: an actionable refusal today beats an indistinguishable 401 today,
and the failure mode it introduces is loud, immediate and names its own cause. **The falsifier is
one line upstream** — `randomBytes(32).toString('hex')` in `magic-link.repository.ts` — and it is
cited at the constant in the code so the next reader meets it there rather than here.

⚠️ **A consequence inside this repo, worth stating because it fired immediately:** the `cmd/usarr`
BookOrbit end-to-end fixtures used 32-hex tokens and were refused at the API boundary before they
reached the fake BookOrbit at all. They are now 64-hex. A fixture that no longer resembles the real
credential is a test that has stopped testing the boundary, and this is the boundary moving, not a
test to relax.

### The ruling this reverses, and why it was reasonable

**Refuse-don't-parse was correct reasoning on a wrong premise, and it is recorded that way rather
than as an error.** Given *"BookOrbit hands out something its own API cannot consume"*, refusal is
the better answer on every axis the first ruling weighed: it declines to guess at a secret, it keeps
one rule instead of two, and it matches `normalizeServiceURLBase`'s existing refusal to invent
configuration. Nothing about that argument was sloppy. **What was missing was one read** — the SPA
router — and the premise did not survive it.

**What generalises, and it is not "check harder":** the first ruling described an upstream's UI
behaviour (*the copy button emits a URL*) and then inferred an upstream's intent (*so the URL is a
mistake*). The behaviour was measured; the intent was assumed and written in the same voice. **An
inferred intent should never be the load-bearing premise of a refusal**, because a refusal is where
UsArr overrides what the user did — and the evidence that settles intent is usually one file away,
in the consumer of the artefact rather than in its producer.

### Rejected

- **Refusing a pasted magic link** — the first ruling, and the reason above.
- **Accepting a URL and passing it upstream unchanged**, letting BookOrbit fail it. That is the
  original defect: it seals a credential that can never work into an AAD-bound envelope nothing can
  read back, and reports it as a 401 that reads as *"your token was revoked"*.
- **Cutting on `strings.Index(v, "token=")`.** One line, and it hands percent-escapes to the login
  route verbatim while silently taking the first of two conflicting values. Decision 4 and 5.
- **Folding case on the token.** Uppercase hex is not the string BookOrbit hashed, so accepting it
  would convert a clear local refusal into a remote 401 — the failure this ADR exists to remove.
- **Consulting the host inside the pasted link.** The service's `base_url` is configured separately
  and is the only thing that decides where requests go (`internal/ssrf` owns that). The link's host
  is read by the parser and discarded; making it authoritative would be a new SSRF entry point
  (§14) for no gain.
- **Doing the reduction in the SPA.** The rule would then be advisory: the API is a supported
  surface and `cmd/usarr` and any script are clients of it too. The SPA sends what the operator
  typed, trimmed, and the server owns the shape.

### ⚠️ What this does NOT decide

1. **Anything about `rawToken`'s retrievability.** [ADR-0060](#adr-0060) owns that measurement and is
   untouched. The two ADRs meet only in the user-facing strings that lean on both.
2. **Whether BookOrbit should keep returning `rawToken` on its list route.** That is upstream's call;
   this repo's dependence on it is now annotated at each site that leans on it, with what would
   falsify it.
3. **Anything about other credential kinds.** Decision 7 is a scope limit, not a promise that a
   similar reduction is right for Kavita or an *Arr.
4. **Whether the connection test should be able to distinguish a wrong token from a revoked one.** It
   still cannot, and no upstream signal makes it possible; this ADR only removes one *category* of
   value from ever reaching that ambiguity.

### Consequences

**The user-facing copy inverts.** The Services screen's placeholder and API-key help, and
`credentialAction`'s string, now say a pasted magic link is fine and name the bare token as the
alternative. Wording that framed the copy button as a BookOrbit mistake is removed: it is a real
feature serving BookOrbit's own login page, and describing it as a trap would have been wrong in the
same direction the first ruling was.

**A search of `cmd/`, `internal/` and `web/src/` for wording claiming the magic-link token travels in
a HEADER found none** — `internal/bookorbit/doc.go` already documents `POST
/api/v1/auth/magic-links/login with body {"token": "<raw>"}`, and
`TestCredentialTravelsInHeadersAndOneBodyOnly` (`internal/bookorbit/client_test.go`) plus
`cmd/usarr/bookorbit_e2e_test.go` already assert the body-not-header split on the wire. The
mechanism was never mis-stated in this tree; only the intent behind the copy button was.

**The refusal path gained a guard it did not have**: that a refused credential is quoted back in
neither the response, the log, nor the audit row. Nothing was found leaking — the create path's audit
metadata carries the instance name and a reason, and `errorBody`'s `message` and `action` both pass
through `redactText` — so the guard pins existing behaviour rather than fixing a breach. It was fired
against a deliberately-broken build first: with the submitted value interpolated into the refusal
message, it fails.

---

<a id="adr-0068"></a>
## ADR-0068 — A BookOrbit comic is an **issue**, and issues are **minted under series works**

**Status:** Accepted — 2026-08-19 · **This is the "unit of work" [ADR-0066](#adr-0066) decision 5 was
waiting for.** That ADR closed with *"The kind stays `book` until comics have a unit of work; decision
5 is what changes it, on evidence rather than on a guess"* — so this entry **activates** decision 5's
two-library split rather than reopening it, and ADR-0066 stands unreworded ·
**[ADR-0030](#adr-0030)'s model is APPLIED, NOT AMENDED** — `comic` is the series, `comic_issue` the
issue beneath it · **Builds on [ADR-0063](#adr-0063)** for the record-what-you-declined rule ·
~~**Ships no code**: this ADR governs a later slice, and **nothing here is built**~~ ·
⚠️ **THAT CLAUSE WAS TRUE WHEN WRITTEN AND IS NOW STALE — the implementing slice has landed, and
this is a POINTER rather than a fresher status claim** (`CLAUDE.md`: *"When you find a status claim
that has gone stale, do not write a fresher one; write the pointer"*). What exists is read off the
tree: `internal/libsync/bookorbit.go`'s `mapComic` and its two parent builders for decisions 1–3,
`internal/store/catalogue.go`'s `applyOneItem` step 0 and `parentBinding` for the two-level write and
decision 5, and `cmd/usarr/import.go`'s `recordComicResidue` for decision 4. **The ADR's own
done-check has NOT been run** — it needs a live import against a real BookOrbit, which no test in
this repo can perform ·
**No migration, no column, no DDL and no new wire field** ·
⚠️ **Measurements of BookOrbit cite a COMMIT, not a tag**: everything below was read from
`bookorbit/bookorbit` at `73b7877d2fede2221b0ca360af9bfced7c3797f3`, the same commit
[ADR-0067](#adr-0067) pinned.

### Context

#### 1 · A BookOrbit comic is one file, so it is an issue — not a series

`MediaKindComic` is *"one of cbz, cbr, cb7, cbx"*
([`internal/bookorbit/catalogue.go:434-435`](../internal/bookorbit/catalogue.go)), and
`MediaKindOf` returns it from a single primary file's format token (`:534-543`). **One BookOrbit book
is one comic file.** A `.cbz` is an issue or a chapter. It is not a series, and nothing upstream makes
it one.

#### 2 · The model it lands in is already settled, and was verified rather than assumed

Three independent statements of the same rule, each read at this tip:

- **[ADR-0030](#adr-0030)** — *"`comic` is the series — top-level, in the grid, in the search corpus,
  `kind_byte` 9. `comic_issue` is the issue or chapter — a child, excluded from the corpus"*, with
  `work_comic` split into a series table and `work_comic_issue`.
- **Migration `00005_library_sync.sql:256`** — the `work.kind` `CHECK` comment, verbatim:
  *"'comic' is the SERIES, 'comic_issue' the issue or chapter, excluded from the search corpus exactly
  as episode/track are (ADR-0030)."* `parent_work_id` is at `:264`, `ix_work_parent` at `:312`.
- **Migration `00006_kavita_subtypes.sql`** — its header creates *"work_book · work_comic ·
  work_comic_issue"*, and the table at `:180-194` carries `number_text`, `number_sort`, `volume_label`,
  `volume_sort`, `is_special`, `is_oneshot`, `special_version`, `page_count`.

ARCHITECTURE §5.3/§6 carry the same split (`docs/ARCHITECTURE.md:733`, `:764-784`, `:867-876`), and
§8.2's corpus exclusion at `:1570`. **This ADR adds nothing to that model. It decides how rows enter
it.**

#### 3 · The screen is already built and already reads zero

`/library/comics` is live and wired end to end: the route exists
([`web/src/routes/library/[type]/+page.svelte`](../web/src/routes/library/%5Btype%5D/+page.svelte)),
`comics` is a member of `MEDIA_TYPES` ([`web/src/lib/library.ts:61`](../web/src/lib/library.ts)), and
`browseKinds("comics")` returns `[]string{"comic"}`
([`internal/store/browse.go:153`](../internal/store/browse.go)). **It renders zero rows for exactly
one reason: nothing writes a comic work.**

#### 4 · The series data costs zero extra HTTP

`BookCard` already carries `seriesId`, `seriesName`, `seriesIndex` and `seriesMemberships[]`
([`api/specs/bookorbit-types/src/book.ts:130-133`](../api/specs/bookorbit-types/src/book.ts), inside
the `BookCard` declared at `:124`; `BookSeriesMembership` at `:115-122`). UsArr's Go `Book` — *"the
allowlisted projection of one BookCard"*
([`internal/bookorbit/catalogue.go:368`](../internal/bookorbit/catalogue.go)) — **decodes none of
them**, and `bookCardDTO` is *"NARROWER THAN THE WIRE SHAPE ON PURPOSE"* (`:236-250`). The bytes are
already on the wire in the walk that ships. **Widening the allowlist is the whole acquisition cost;
there is no second request, and no new endpoint.** `GET /series` stays excluded on its existing
measurement (`:30-34`) — *"there is no series watermark to walk, and every fact they carry rides the
book stream already"* — which this decision now depends on rather than merely inherits.

### Decision

**1 · Every imported comic file becomes a `work.kind = 'comic_issue'` row, and every one of them is
minted under a `work.kind = 'comic'` parent.** `parent_work_id` is never null on a `comic_issue` row
this importer writes. The series work is the row the grid, the corpus and the facet count see; the
issue is the child, exactly as `episode` sits under `series`.

**2 · `seriesId` null → a SYNTHESIZED SINGLE-ISSUE SERIES, with `is_oneshot = 1` written.** A comic
whose card reports no series is **never silently dropped**, and is **never promoted to a `comic` work
in its own right**. It is ingested as an issue under a synthesized series work named for the book, and
the issue row sets `work_comic_issue.is_oneshot = 1`.

> **The flag is WRITTEN, not merely tolerated.** *"A column with a `DEFAULT 0` and no writer is a deaf
> column, and this project has found several. If the one-shot case is real enough to have a flag, the
> import writes the flag."* `special_version`'s enumerated values already include `one-shot`
> (`00006_kavita_subtypes.sql:188`), so the vocabulary for this case was allocated before this ADR and
> had no writer either.

⚠️ **This was cross-checked against ADR-0030 before being relied on, because the one-shot reading
began as a reading of the schema rather than a measurement of intent.** ADR-0030 **does not
contradict it**: it allocates `is_oneshot` and `is_special` as columns of `work_comic_issue` and
defines neither's writer, and its only adjacent ruling — that a TPB *"is its own `comic_issue` with
`special_version='tpb'`"* — is the same shape as this one, an unusual issue modelled as an issue.
**ADR-0030 in fact strengthens the parent rule**: it explicitly rejected *"putting series and issues in
one kind and distinguishing on `parent_work_id IS NULL`"*, so a parentless issue is a shape that ADR
already refused to make meaningful.

**3 · Bind on the scalar `seriesId`. RECORD the additional memberships; do not resolve them.** A
BookOrbit book may belong to several series. The parent binding uses the scalar `seriesId` and nothing
else. Memberships beyond that one are **recorded** — the ADR-0063 precedent, *record what you declined
to act on* — and are never used to mint a second parent, a second membership, or a `work_relation`
edge.

> ⚠️ **The premise here was a stop-condition, and it was MEASURED rather than inferred.** *"Bind on
> BookOrbit's own primary"* was originally an inference from the wire shape alone. It was checked
> against BookOrbit's own source, at the commit pinned in the Status line above, in
> `server/src/common/services/series-membership.service.ts`:
>
> - `bookMetadata.seriesId` is a **real scalar column**, and the card takes it from that column —
>   `seriesId: bookMetadata.seriesId` (`book.repository.ts:552`), surfaced as
>   `seriesId: row.seriesId ?? null` (`utils/assemble-book-cards.ts:194`), on a path **independent of**
>   the `seriesMemberships` join (`:197-203`).
> - `displayOrder = 0` is a **designated primary slot**, not an accident of ordering.
>   `syncPrimaryFromMetadata` rebuilds the list as `[primary, ...rest]` where
>   `rest = current.filter(m => m.displayOrder !== 0)` (`:59-89`), and `backfillFromMetadata` seeds the
>   scalar into `display_order = 0` (`:135-148`).
> - The invariant is **maintained in both directions**: `replaceForBook` calls `syncPrimaryMetadata` on
>   every membership write (`:55`), which sets `bookMetadata.seriesId = memberships[0].seriesId`
>   (`:154-166`). BookOrbit's own test names it — *"replaces memberships with normalized unique series
>   and **syncs primary scalar metadata**"* — with fixtures literally named `Primary` and `Rest`.
>
> **So both failure branches are FALSE.** `seriesId` is **not** an arbitrary `memberships[0]`: it is
> the `displayOrder = 0` primary, a slot BookOrbit names *primary* in three method names and keeps
> true on every write. And it is **not** null when there are several memberships — `syncPrimaryMetadata`
> writes `primary?.seriesId ?? null`, which is null only when the membership list is **empty**.
>
> **The strengthening this buys:** binding on the scalar and binding on `memberships[displayOrder = 0]`
> are, by construction, **the same binding**. UsArr does not need `seriesMemberships[]` to find the
> parent at all — which is precisely why recording the remainder is a pure record and can never
> silently become a resolution.

**4 · Both residue defaults emit a `sync_report` row.** The synthesized-series case and the
extra-membership case each write one, so the **first real import against the owner's library measures
how often each occurs**. Sizing comes from instrumentation, not from estimates, and **not from asking
the owner to run SQL**. This needs no migration: `sync_report.kind` deliberately carries **no `CHECK`**
— *"this vocabulary is the newest and least settled in the schema"* (`00005_library_sync.sql:864-867`)
— and `detail` is JSON that is redacted on the way in (`:875-878`).

**5 · The library the series belongs to is [ADR-0066](#adr-0066) decision 5's answer, and this ADR is
what makes it live.** A BookOrbit series is **not** library-scoped upstream, so the container question
is answered on UsArr's side, not BookOrbit's: **a synthesized or bound `comic` series work belongs to
the `comic` library minted over the `library_source` container ref the issue's book was walked from.**
Decision 5 already ruled the mechanism — a mixed container *"becomes a `book` library and a `comic`
library over the same `library_source` container ref"*, needing no migration because `'comic'` is
already a permitted `library.kind` and `library_source`'s uniqueness is
`(library_id, service_instance_id, container_kind, container_ref)`. Its own condition was *"once comics
have a unit of work"*, and **this ADR is that unit of work**, so the sentence *"until comics import, a
mixed container binds exactly as it does today"* stops applying at the slice that implements this.
**No series work is ever minted into no library at all.**

### Why

**Because the parent is the only part of this that cannot be fixed later.** ARCHITECTURE §6.4's cascade
means a wrong `work.kind` written at ingest can never be merged away — the rule ADR-0066 cited when it
refused to guess a container kind. Everything else here is recoverable; the parent binding is not.
That is why the binding rests on a measurement of BookOrbit's source and not on the shape of its JSON.

**Because the alternative to a parent is a screen that lies.** `comic_issue` is excluded in three
independent places, and this is the whole argument against orphan issues — verified in the tree:

1. **`recentWorkKinds`** is `{"movie","series","artist","album","book","comic"}`
   ([`internal/store/recent.go:71`](../internal/store/recent.go)), with `comic_issue` named in the
   doc as excluded for ADR-0030's reason.
2. **`browseKinds`** maps `MediaTypeComics` to `[]string{"comic"}`
   ([`internal/store/browse.go:153`](../internal/store/browse.go)).
3. **The FTS corpus refuses it outright.** `corpusExcludedKinds` contains `"comic_issue": true`
   ([`internal/store/catalogue.go:1065-1067`](../internal/store/catalogue.go)) and `writeSearchDoc`
   **returns an error** on it (`:1099-1104`) rather than merely skipping it.

That third one is stronger than "excluded", and it changes the argument: an importer that wrote
parentless issues through the top-level item path would not quietly render zero — it would **fail**.
The guard's own doc says so and names this exact slice: *"NOTHING SHIPPED REACHES IT … today this is a
guard against a future caller — the phase-B `comic_issue` walk is the obvious one — routing a child
kind through the top-level item path"* (`:1060-1064`). **This ADR is that caller, and it is the reason
the guard was written.**

**Because the one-shot is a real shape and the schema already conceded it.** Two columns and one
`special_version` value were allocated for it and none has a writer. Synthesizing a single-issue series
is the shape that keeps a one-shot visible on `/library/comics` — where the user will look for it —
without inventing a kind, a third level, or a nullable parent.

### Alternatives rejected

- **Orphan `comic_issue` works with no parent.** Refused: *"a deaf guard with a progress bar on it —
  worse than today, because today the emptiness is honest."* The import would report success and every
  screen would still read zero, for the three exclusions verified above; and on the corpus path it
  would not even do that, because `writeSearchDoc` errors. **A `comic_issue` with no parent is not a
  degraded comic; it is a row no shipped read can see.** ADR-0030 had already refused to give
  `parent_work_id IS NULL` meaning on this kind.

- **One `comic` work per comic file** — the one-line version. Refused: it mints a **series** work per
  issue, and §6.4's cascade makes a wrong `work.kind` at ingest **permanently unmergeable**.
  ⚠️ **[ADR-0066](#adr-0066) already rejected this pre-emptively**, in terms — *"The kind stays `book`
  until comics have a unit of work; decision 5 is what changes it, on evidence rather than on a
  guess"* — refusing even to infer a container's kind from a tally for the same cascade reason. Doing
  it per row is that refusal at a finer grain. ARCHITECTURE §13's own sizing shows the shape it would
  destroy: *"`comic` | 3,000 | ~90,000 `comic_issue` rows behind them"* (`docs/ARCHITECTURE.md:2120`)
  — a thirty-to-one ratio flattened to one-to-one.

- **Defer comics until the parent seam exists.** Refused **as stated**: the seam is `parent_work_id`
  plus `ix_work_parent`, and it has existed since `00005_library_sync.sql:264` and `:312` — so
  "until the seam exists" describes no future state. **No owner builds a seam that already exists for
  a feature nobody is importing**, which makes this deferral a **scope cut** rather than a sequencing
  choice. A scope cut is **the repo owner's call and would have to be put to him in those words**. It
  was not chosen, and it is recorded here so that a later reader can see it was considered rather than
  overlooked.

- **Resolve `seriesMemberships[]` into `work_relation` edges now.** Refused: the fuzzy-match tier that
  would adjudicate multi-series membership is **v0.3** (`docs/FUTURE.md`, and `work_relation`'s
  confidence and evidence columns are its seam). **The seam ships; the feature does not.** Recording
  the memberships is what keeps the later tier cheap without building toward it early.

- **Promote a `seriesId`-null comic to a `comic` work in its own right.** Refused: it is the per-row
  alternative wearing a smaller hat, and it makes the two residue cases produce two different kinds
  from one file format — so the `/library/comics` count would silently mean two different things
  depending on upstream metadata quality.

### Consequences

- **No migration, no column, no DDL, no new configuration key and no new wire field.** Every object
  this needs exists: `work.kind`'s `comic_issue` member, `parent_work_id`, `ix_work_parent`,
  `work_comic`, `work_comic_issue`, `library.kind`'s `'comic'`, and a `sync_report.kind` vocabulary
  with no `CHECK` on it.
- **`internal/bookorbit`'s allowlist widens by four fields, and that is the acquisition cost in full**
  — `seriesId`, `seriesName`, `seriesIndex`, `seriesMemberships[]` into `bookCardDTO` and the `Book`
  projection. **Zero extra HTTP.**
- **`SkipTally.Comics` and `SkipNote.Comics` stop being the comics story.** They exist today because
  comics are skipped ([`internal/libsync/bookorbit.go:120-121`](../internal/libsync/bookorbit.go),
  [`internal/store/skips.go:121`](../internal/store/skips.go)); after this, the field stays and its
  expected value becomes **0**, which is the fourth done-check below.
- **`writeSearchDoc`'s guard gets its first real caller and must keep refusing.** The implementing
  slice routes `comic_issue` rows around the top-level item path; it does not relax
  `corpusExcludedKinds`.
- **The owner's Libraries screen changes shape**, via ADR-0066 decision 5: a mixed container that binds
  as one `book` library today will bind as a `book` library **and** a `comic` library. ⚠️ **The owner's
  own libraries are split**, so — as ADR-0066 decision 6 already recorded — **his data will not
  exercise the mixed-container split**, and this ADR inherits that limitation unchanged rather than
  claiming coverage it does not have.

#### The done-check

After a live import against a real BookOrbit, all four must hold. **The middle clause does the work:**

1. **`work.kind = 'comic_issue'` rows exist, and every one resolves to a parent series work.** Zero
   rows with `parent_work_id IS NULL`.
2. **`work.kind = 'comic'` rows exist and are STRICTLY AND SUBSTANTIALLY FEWER than the issue rows.**
   ⚠️ **If the series count EQUALS the issue count, the per-row implementation shipped and this check
   MUST FAIL.** A green on (1), (3) and (4) with parity on (2) is precisely the rejected alternative
   passing itself off as the accepted one, and it is the only outcome here that is worse than not
   shipping.
3. **`/library/comics` renders SERIES, NOT ISSUES**, with a non-zero facet count.
4. **The latest `items_skipped` row's `Comics` field reads 0.**

Checks (1) and (4) prove the rows arrived. Check (3) proves they arrived in the layer the user reads.
**Only check (2) can tell the accepted shape from the refused one**, which is why it is stated as a
failure condition rather than as a number to look at.
