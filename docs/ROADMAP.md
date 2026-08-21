# UsArr — Roadmap working view

> **This file is NOT authoritative for scope.** [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16 is, and
> it wins over every line here. This is a *working view* of what v0.1 still needs, distilled from
> §16 plus the ADRs, kept short enough to re-read on a phone.
>
> **It is not authoritative for status either.** Status is read off the tree —
> `web/src/routes`, `internal/`, `internal/db/migrations`, `git log` — never off this page. Every
> item below carries **a check you can run**, so the list is re-derived rather than trusted. If a
> check disagrees with the box beside it, the check is right and the box is stale.
>
> **No dates and no estimates appear here, ever**, at the owner's standing instruction. Where a line
> is inference rather than something read off §16, an ADR or the tree, it is marked 🔍.
>
> **Citation policy, applied 2026-08-19 to §2's per-type-grid item and the two items after it —
> AND TO NOTHING ELSE:** prefer function and symbol names over `file:<n>` line citations for any
> file that moves, Go and Svelte especially. A wrong line number still resolves to a plausible
> line, so it fails invisibly and reads as checked. Line citations elsewhere in this file were
> **neither swept nor vetted** by that pass — no sweep happened.
>
> **Extended 2026-08-19 by the Kavita-sunset pass, to exactly TWO further items and to nothing
> else:** §2's **image-pipeline** item, whose `ARCHITECTURE.md:2649-2651` citation was **~40 lines
> off** and is a section reference now rather than a corrected number, and §2's **output-codec**
> item, whose `00005_library_sync.sql:219-236` citation named a schema fact that migration `00008`
> had already falsified. That pass rewrote §1, §2's image / codec / covers items, §3's sequencing
> table and its BookOrbit entry, and §4. **It read nothing else for line drift, and no sweep
> happened this time either.** Every other line citation in this file remains unvetted.
>
> **Extended 2026-08-19 by the ADR-0052/0053 consolidation pass, to the items it actually touched
> and to nothing else:** §1's objective bullet · §2's opening ⚠️ · §2's **image-pipeline** item, its
> Q4 hedge and its freeze paragraph ONLY · §2's **per-type grid** item · §2's **facet-counts /
> sidebar** item · the **three items that pass added** · §3's second-adapter row, its BookOrbit
> section and adapter box, and its sequencing recommendation · §3's new deferred row · §4's BookOrbit
> bullet. **No other line moved, no sweep happened, and every other line citation in this file is
> still unvetted.**
>
> **Extended 2026-08-19 by the ADR-0054 / stale-count pass, to exactly the lines it touched and to
> NOTHING ELSE:** the baseline block's **ADRs** bullet · §1's objective bullet, its
> *"highest on `main`"* clause **only** · §2's **relevance score on the wire** item · the **two items
> that pass added** · §3's mockup-re-keying row, its *"no ADR number is cited"* clause **only**.
> **No other line moved, no sweep happened, and every other line citation in this file is still
> unvetted.** That pass also removed the last three places where an **ADR number stood in as a count
> or a maximum**; numbers used to **cite a specific ADR** are untouched and stay.
>
> **Absence rule, added 2026-08-19 after this file wrongly recorded a probe as *"never run"*:**
> a missing artefact **in the repo** does not establish that something never happened — a result can
> live in a thread, on the owner's box, or nowhere. A claim of the form *"X was never done"* needs a
> source that **would have recorded X**, not merely a place where X is not.
>
> **Done-when rule, added 2026-08-19, and scoped to *Done when* clauses in this file — it is not a
> repo-wide rule and must not be read as one:** a *Done when* that a text editor alone can satisfy is
> not a *Done when*. An acceptance criterion has to name an observation only the running system can
> produce — a byte through the path, a guard fired, a command that exits zero on a real input.
> *"The file exists"*, *"the function is written"*, *"the column is there"* are every one of them true
> of a tree nothing has ever run. Where the running observation genuinely cannot be taken here — an
> absent prerequisite such as a Docker daemon, or a live upstream this environment has no access to —
> an item may still be ticked against its written criterion, on **two** conditions: the unfired
> obligation is recorded immediately beneath the box, and the specific missing prerequisite is named.
> A tick without both is a claim that something works.
> This is the roadmap-shaped member of the same family as the repo's drill discipline — presence is
> not being-called, a guard is proven by firing, a validator nothing calls mitigates nothing. Kin,
> not an extension: it amends nothing in `docs/DEVELOPMENT.md`.
> **It records what was already done rather than imposing something new.** Both of the cases that
> prompted it were handled this way on the day: §2's **Docker / backup** item is ticked against
> *"a `Dockerfile` exists"* — `deploy/Dockerfile`, content commit `000ac52` — with the unbuilt image
> and the missing daemon written out beneath the box; and §2's **image-pipeline** item's third leg
> stays **open** although its writer and its call site landed as code (content commits `7e5934d` and
> `c4a3277`), because no real cover has been through the path yet.
>
> **Extended 2026-08-19 by the shipped-state batch pass, to exactly the items it touched and to
> NOTHING ELSE:** §2's **Libraries-row link** item · §2's **all-types scoped view** item · §2's
> **`?lib=` chip** item · §2's **facet-counts** item, the action-text rider on it **only** · the
> **one item that pass added** · §3's **BookOrbit adapter** item and the ADR subsection it added at
> the end of §3. **No other line moved, no sweep happened, and every other line citation
> in this file is still unvetted.**
>
> **Extended 2026-08-19 by the facet / image / BookOrbit catch-up pass, to exactly the items it
> touched and to NOTHING ELSE:** §2's **facet-read** item, rewritten whole, and the
> `ErrUnservableSort` rider carried on it · §2's **image-pipeline** item, its headline and its three
> done-when legs **only** — the rest of that long item, including every LS-260 paragraph, was **not
> re-read** · §2's **`search.md` §4 re-rank** item · §2's **Docker / backup** item, the observation
> appended under it **only** · §3's **BookOrbit adapter** box. **No other line moved, no sweep
> happened, and every other line citation in this file is still unvetted.**
>
> **Extended 2026-08-19 by the two-stale-opens pass, to exactly FOUR places and to NOTHING ELSE:**
> §2's **image-pipeline fetch-half** item — its headline, done-when legs 2 and 3, the obligations
> block's opening and Obligations 1 and 3, the tint paragraph's rider, and the *what is still owed*
> block it added; **Obligation 2 and every LS-260 paragraph were NOT re-read** · §2's closed
> **image-SERVING-half** item, the two paragraphs on it that asserted no writer exists **and nothing
> else on that box** · §2's **Docker / backup** item, rewritten whole and ticked against its own
> *Done when* · §3's **ADR-0039 `write_queue` validator** row. **No other line moved, no sweep
> happened, and every other line citation in this file is still unvetted.** This pass cited symbols and section numbers throughout
> and added no `file:<n>` citation of its own.
>
> **Extended 2026-08-20 by the comics-landed / stale-defect pass, to exactly FIVE places and to
> NOTHING ELSE:** the baseline block, advanced and re-fired · §2's **per-series volume and chapter
> walk** item, its *Done when* **only**, which is split into its two legs · §2's **"not identified"
> badge** item, its opening clause and its *Done when* **only** · the **one item that pass added**,
> recording the comics import · §3's 🔴 **open-defect** block on the BookOrbit adapter, struck in
> place together with the three-hop chain beneath it and everything that reasoned from it, down to
> and including the 🔍 facet-count inference. **The *"Verified facts"* bullets that follow that block
> were NOT re-read** — they are about BookOrbit's own source, not about UsArr's, and no line of them
> moved.
> **No other line moved, no sweep happened, and every other line citation in this file is still
> unvetted.** This pass cited symbols and distinctive strings throughout and added no `file:<n>`
> citation of its own.
>
> **Extended 2026-08-20 by the Block-A / facet-consumer pass, to exactly FOUR places and to NOTHING
> ELSE:** this paragraph · the baseline block, advanced and re-fired · §2's **Home Block A** item,
> ticked with its scope and its unfired obligation written out · §2's **facet-consumer** item, in
> **two places only** — its ⚠️ *"DO NOT TICK"* bullet list and its *Done when*, the latter now split
> into its two legs with **leg 2 left open**. **Every other paragraph of that long item — the
> ADR-0053 / ADR-0059 argument, the empty-state paragraph, the 🔍 budget inference and the
> shared-action-string rider — was NOT re-read, and no line of it moved.** **No other line moved, no
> sweep happened, and every other line citation in this file is still unvetted.** This pass cited
> symbols and distinctive strings throughout and added no `file:<n>` citation of its own.

**Last re-derived against:** `origin/main` `7c8cb1b` (2026-08-21).
⚠️ **`7c8cb1b` IS THE TIP THE GATED TREE IS ACTUALLY BASED ON, NOT WHATEVER `main` WAS WHEN THIS WAS
WRITTEN.** It is `git rev-parse HEAD~1` of the landing commit — the tree `make check` ran against.
**A baseline naming a tip nobody verified at is worse than a stale one**, because it launders unread
commits as attested.
⚠️ **AND `origin/main` MOVED AGAIN WHILE THE GATE RAN — TO `9694447`, SEVEN NON-MERGE COMMITS — SO
IT WAS MERGED FORWARD (NOT REBASED) AND THE GATE RE-RUN ON THE MERGED TREE.** Those seven touch
`docs/DECISIONS.md`, `docs/FUTURE.md`, `docs/REVIEW-LOG.md` and `docs/reference/schema.md` and no
other file. **Every claim in this pass that cites one of those two first files was re-fired on the
merged tree** — the ADR-0070 / 0052 / 0068 / 0072 / 0074 quotes, ADR-0001 Correction 3, the two
*"nullable column and the badge"* sites, the `GOARCH=arm64` check and its `amd64` control, and
`reference/schema.md`'s `work` block — and every one still holds. **`7c8cb1b` stays the baseline
because it is the tip this pass actually READ every box against**; the seven merged commits are
attested only on the narrow re-fire just named, and **nothing else in them was read.**
⚠️ **Advanced from `6533f1c` (2026-08-20), which was the Block-A / facet-consumer pass's baseline**,
and **the range is large and is emphatically not documentation-only** — **114 non-merge commits**
(172 including merges, 99 files) carrying `web/src/` (37 files), `cmd/usarr` (11), `internal/store`
(10), `internal/libsync` (9), `internal/httpapi` (7), `docs/` (6), `internal/db` (5),
`docs/design/` (5), `internal/bookorbit` (4), `docs/reference/` (3), plus `Makefile` and `CLAUDE.md`
— so **read every box this pass does not name as attested at `6533f1c`**, not at the baseline line
above. ⚠️ **That figure was measured, not inherited:** `git log --no-merges --oneline
6533f1c..7c8cb1b | wc -l` → `114`, run on a clone unshallowed for the purpose, because the
environment the previous pass ran in was shallow and could not resolve `6533f1c` at all.

**FIRED at `7c8cb1b` by this pass, and this is the whole list:**
- **The `?lib=` write census, and its INSTRUMENT re-fired beside it:** every `lib` query-parameter
  write under `web/src/`, enumerated as six sites and listed rather than counted; then the struck
  sentence's own recipe — `set('lib'`/`delete('lib')` under `web/src/` — re-fired to show it returns
  **five** and walks structurally past `libraryScopeHref`'s `new URLSearchParams({ lib: slug })`.
- **The scope select, wired and GUARDED IN BOTH DIRECTIONS:** the five-symbol `$lib/scopeselect`
  import, the `$derived` trio and the `<select id="library-scope">` / `<select id="grid-scope">`
  blocks in both grid routes, read in the files; then `pnpm vitest run scopeselect` fired green
  (31/31) **and fired failing** with `onchange={onScope}` removed from `/library`'s `<select>`.
- **Channel 3b's reachability and its negative control:** `grep -n 'sync/delta'
  internal/httpapi/server.go` non-empty at this baseline, and the same grep **empty at `5069c91`**,
  a tree where `DeltaSync` already existed.
- **Channel 4's own checks, re-fired rather than trusted:** `grep -n 'startReconciler('
  cmd/usarr/main.go` non-empty here and **empty at `f87aef44`**; `reconcileInterval = 6 * time.Hour`
  and `reconcileTick = 30 * time.Minute` in `cmd/usarr/reconcile.go`; `FullImport`'s unfiltered
  `streamAndApplyCredits` and its `StampFullSync`, both read inside `FullImport`'s own span.
- **The arm64 criterion's replacement and its control:** `grep -n 'GOARCH=arm64' docs/DECISIONS.md`
  **empty**, against `GOARCH=amd64` returning ADR-0001 Correction 3's hardware line.
- **The badge's absent column and absent renderer:** all thirteen files in `internal/db/migrations/`
  searched for an identifier-shaped column on `work` (none; no `ALTER TABLE work` at all),
  `reference/schema.md`'s `work` block inventoried to its `) STRICT;`, every non-test consumer of
  `store.BatchResult.Unidentified` traced forward (three log fields plus the merge), and
  `grep -rniE 'not identified|not_identified|notIdentified|unidentified' web/src/` **empty**.
- **The cover-art timeline:** `163f608`, `a34d87f`, `7e5934d`, `c4a3277`, `db9b028` and `ad821a06`
  each resolved to their author date and message locally on the unshallowed clone, plus the
  `<img>` census at this baseline (one rendered element, `$lib/PosterGrid`).
- **The `ok` state's withdrawn citation and the home guard, fired both ways:**
  `grep -n 'that citation is withdrawn' web/src/lib/home.ts` non-empty against the negative control
  `that citation is restored` (empty); `pnpm vitest run home` green at 64/64 **and failing 2 of 64**
  with `ok: ''`. Banner SHAs `712db17` / `019775b` re-derived, and the mis-cited `ed3a948` read.
- **§16's silence on a `System` screen:** the `## 16. Roadmap` → `## 17. Screens` awk range measured
  at 895 lines, `System` filtered of `System tags` **empty**, `sidebar` zero; against the shell's
  `NAV_GROUPS`, the `'Settings'` control, and the three design sources that draw `System`. **And the
  nav guard fired failing** — `/library`'s `NAV_GROUPS` entry deleted, `pnpm vitest run librarygrid
  libraryscreen` fails 2 of 86; restored, 86/86.

**NOT fired by this pass, and therefore inherited:** no `REVIEW-LOG.md` read · **no Go test was
fired as EVIDENCE for any claim here** — the pre-commit gate's Go suite ran on this tree, but a
green gate is not evidence for a box, and the four `web` vitest files named above are the whole of
what was fired at a claim · **no live service touched**, so every *"on a real instance"* leg in this
file is untouched · **no §2 box this pass does not name** · **no line-citation sweep, again**, and
this pass added no `file:<n>` citation of its own, citing symbols and distinctive strings instead.
⚠️ **THIS PASS TICKED ONE BOX AND DE-BOXED ONE, AND DECLINED TO TICK A THIRD WHOSE WRITTEN LEGS ARE
BOTH MET** — channel 3b, held open on §1's *"proves the replica thesis on real data"*, marked so a reviewer
can overturn it. ⚠️ **AND IT REFUSED HALF OF ONE FINDING IT WAS HANDED:** the credits-only drift gap
was briefed as having an *invisible remedy*, and the tree falsifies that — ADR-0076's reconciler
calls `FullImport`, which re-applies credits unfiltered, so `Last successful sync` is a truthful
marker of it. Only the narrow residual was written.

**INHERITED from the `6533f1c` baseline. Its own attestation follows, unchanged and NOT re-fired:**
⚠️ **That baseline advanced from `0a5d66e` by the Block-A / facet-consumer pass, which re-derived
THREE things and NOTHING ELSE**, and **the range is large and is emphatically not
documentation-only** — 41 non-merge commits carrying `internal/repofiles` (a package that did not
exist at the previous baseline), `internal/libsync`, `internal/store`, `internal/httpapi`,
`internal/db`, `internal/bookorbit`, `cmd/usarr`, `web/src/`, `docs/`, `docs/design/`,
`docs/reference/`, `README.md` and `CLAUDE.md` — so **read every box THAT pass does not name as
attested at `0a5d66e`**, not at its baseline line.

**FIRED at `6533f1c` by THAT pass — inherited verbatim, and *"this pass"* below means the Block-A /
facet-consumer pass, not the one that advanced the baseline:**
- **Home's block map and its facet call:** the ADR-0028 block map in `web/src/routes/+page.svelte`,
  read in the file; the `fetchLibraryFacets` import and its call site; `librarySummary(facets,
  health)` feeding `summaryRows`. Content commits `51a9e68` and `da33aa7`, attributed by `git log -S`
  on `fetchLibraryFacets` and on the block-map string *"media-type summary    ≤6 rows           DRAWN"*
  rather than by subject, and each confirmed **single-parent** before being cited.
- **What Home does NOT draw:** the same file's `BLOCK A's COLUMNS, AND THE TWO §17.2 NAMES THAT ARE
  DELIBERATELY ABSENT` paragraph and its `WHAT IS STILL NOT DRAWN` paragraph, both read whole; and
  the read's own refusal — the declaration comment above `handleLibraryFacets` and the
  `toMediaTypeCounts` allowlist in `internal/httpapi/facets.go`.
- **The sidebar predicate:** `TYPE_NAV` in `web/src/routes/+layout.svelte`, read as an expression and
  not as a claim about one, together with its two call sites; plus every `facet` mention under
  `web/src` by `git grep -l` (nine files), with the four this box named opened on their §7.1
  citations.
- **The harness claim, fired rather than assumed:** `git grep -l 'render(' -- 'web/src/**/*.test.ts'`
  (no hits) and the import header of `web/src/lib/home.test.ts`.

**NOT fired by this pass, and therefore inherited:** no `ARCHITECTURE.md` read · no `REVIEW-LOG.md`
read · **no migration read at all** · **no ADR text read at all**, so every ADR-0053 / ADR-0059
sentence on the facet box is inherited unread · no Go outside `internal/httpapi/facets.go` · **no §2
box this pass does not name** · **no line-citation sweep, again.**
⚠️ **THE INHERITED ATTESTATION BELOW IS STALE IN FILES THIS PASS DID NOT RE-FIRE, AND THE RANGE SAYS
WHICH.** `0a5d66e..6533f1c` touches `internal/libsync/bookorbit.go`,
`internal/libsync/bookorbitfiles.go`, `internal/libsync/doc.go`, `internal/store/catalogue.go` and
`docs/DECISIONS.md` — five files the `0a5d66e` pass fired on and this one did not re-open. **Read its
five checks as attested at `0a5d66e`, not here.**
⚠️ **This pass was handed three claims and all three held**, but **one was handed incomplete**: leg 1
of the facet box is false as the brief says, and the 🔍 inference riding on it — that four `web/src`
comments were stale — had **already been discharged in the tree**, which the brief did not say. That
is recorded on the box rather than dropped. **Two corrections were also handed over and then found
already landed on `origin/main`** — the zero-external-providers annotation (content `8cdf399`) and
the box-is-a-bookmark preamble clause — and **neither was written a second time.**
⚠️ **One stale sentence was SEEN AND DELIBERATELY LEFT**, because it is another lane's: §2's opening
⚠️ still lists the *"zero-external-providers evidence clause"* among items *"None of them is
re-pointed line by line here"*, and `8cdf399` re-pointed exactly that one. **Not this pass's to fix,
and flagged rather than quietly corrected.**

**INHERITED from the `0a5d66e` baseline. Its own attestation follows, unchanged and NOT re-fired:**
⚠️ **That baseline advanced from `4d95d36` by the comics-landed / stale-defect pass, which
re-derived FIVE things and NOTHING ELSE**, and **the range is large and is emphatically not
documentation-only** — 36 non-merge commits carrying `internal/libsync`, `internal/store`,
`internal/bookorbit`, `cmd/`, `web/src/`, `testdata/` and `CLAUDE.md` — so **read every box that pass
does not name as attested at `4d95d36`**, not at its baseline line.

**FIRED at `0a5d66e` by THAT pass — inherited verbatim, and *"this pass"* below means the
comics-landed / stale-defect pass, not the one that advanced the baseline:**
- **The BookOrbit file and credit sources:** `func (s *BookOrbitSource) StreamFiles`
  (`internal/libsync/bookorbitfiles.go`) and `func (s *BookOrbitSource) StreamCredits`
  (`internal/libsync/bookorbitcredits.go`), found by searching every non-`_test.go` file in
  `internal/libsync` for both method names; the `FileSource` and `CreditSource` type assertions in
  `internal/libsync/importer.go`. Content commit `373df3f`, read as a diff.
- **The comics import:** the `bookorbit.MediaKindComic` arm of the `MediaKind()` switch in
  `internal/libsync/bookorbit.go`, and `mapComic`. Content commits `1c35d18` (the import slice),
  `10444a4`, `ff13582`, `04d1620` and `0a5d66e`, each read as a diff rather than as a subject and
  each confirmed **single-parent** before being cited.
- **The `work_comic_issue` writer:** `INSERT INTO work_comic_issue` across all Go outside `_test.go`
  — one hit, the `case "comic_issue"` arm of `internal/store/catalogue.go` — plus every
  non-`_test.go` file that names the table at all (six).
- **The per-series walk's other leg:** `SeriesVolumes` on the source interface in
  `internal/libsync/kavita.go`, and `func (s *KavitaSource) StreamFiles`
  (`internal/libsync/files.go`), read against ADR-0052's sunset clause.
- **[ADR-0068](./DECISIONS.md#adr-0068)** — its status block and its four numbered done-checks — and
  **[ADR-0052](./DECISIONS.md#adr-0052)**'s status block, both read directly in `DECISIONS.md`.

**NOT fired by this pass, and therefore inherited:** no `ARCHITECTURE.md` read · no `REVIEW-LOG.md`
read, so **every LS-260 paragraph on the image item is still inherited unread** · **no migration
read at all**, so nothing about the schema was re-verified · no `web/src/` read · no Go outside the
files named above · **no ADR text beyond ADR-0052 and ADR-0068** · **no §2 box this pass does not
name** — the image-pipeline item, channel 3b, channel 4 and the zero-external-providers clause were
**not re-read**, and the first three still reason from Kavita · **no line-citation sweep, again.**
⚠️ **This pass was handed five defects to verify and found its brief wrong on one of them**: the
brief named `ff13582` as the comics *import* slice, and it is not — `ff13582` names a sibling
library for its kind. The import slice is `1c35d18`, and the corrected chain is written on the box
rather than the one handed over.
⚠️ **One of the five had been stale since before the previous baseline and was not caught then.**
`373df3f` (2026-08-19 13:47Z) is an **ancestor of `4d95d36`** (18:55Z), so §3's open-defect block was
already false at the moment it was attested. That is why it is **struck in place rather than
deleted** — the record of an alarming box having been wrong is worth more than a clean page.

**INHERITED from the `4d95d36` baseline. Its own attestation follows, unchanged and NOT re-fired:**
⚠️ **That baseline advanced from `a51d3c3` by the two-stale-opens pass, which re-derived FIVE things
and NOTHING ELSE**, and **the range is large and is emphatically not documentation-only** — it carries
`internal/imagepipeline` (a whole new package), `internal/store`, `internal/libsync`, `cmd/` and
`deploy/` — so **read every box that pass does not name as attested at `a51d3c3`**, not at its
baseline line.

**FIRED at `4d95d36` by THAT pass — inherited verbatim, and *"this pass"* below means the
two-stale-opens pass, not the one that advanced the baseline:**
- **The image writer:** `INSERT INTO image_asset` across all Go outside `_test.go` (one hit,
  `internal/store/imagewrite.go`); `PosterAsset.validate`'s calls to `ValidImageFormat` and
  `checkImageSourceURL`; `PutPosterAsset`'s `INSERT` column list; `ssrf.IsCredentialParam`.
- **The call site:** `Importer.fetchCovers` (`internal/libsync/covers.go`), its one call site in
  `internal/libsync/importer.go`, `PosteredItems` (`internal/store/posterpresence.go`), and
  `coverPipeline` (`cmd/usarr/import.go`). Content commits `7e5934d` and `c4a3277`, each read as a
  diff rather than as a subject.
- **What is NOT written:** `dominant_color`, `thumbhash`, `etag`, `last_modified` and `expires_at`
  across the whole tree, against `00005_library_sync.sql`'s `image_asset` block and
  `ARCHITECTURE.md` §4.4.1, read directly.
- **`deploy/`:** its file list; `deploy/Dockerfile`'s header; the `docker` recipe in `Makefile`;
  `docs/DEVELOPMENT.md` §4's target table, §12's opening and §12's known-gaps bullet; `000ac52` and
  `ea7c855` as diffs; and the newest **non-merge** commit per area (`deploy/` · `internal/` · `cmd/`
  · `docs/` · `web/src/`).
- **The `write_queue` validator:** `internal/store/writequeue.go`, every non-`_test.go` reference to
  `ValidWriteQueueState` (two files), `internal/db/spike/fixture.go`'s build tag, and
  `writequeuelint_test.go`'s header.

**NOT fired by this pass, and therefore inherited:** no `ARCHITECTURE.md` read **except §4.4.1** · no
`REVIEW-LOG.md` read, so **every LS-260 paragraph on the image item is inherited unread** · no
`DECISIONS.md` read, so **no ADR text was re-verified** · **no migration read except `00005`'s
`image_asset` block** · no `web/src/` read · no Go outside the files named above · **no §2 box this
pass does not name** · **no line-citation sweep, again.** ⚠️ **This pass was handed two hypotheses
and told to treat them as hypotheses; it verified both and found a third thing neither named** —
`internal/httpapi/images.go`'s package header is itself now stale, and is flagged on the image item
rather than fixed, because that file is not this pass's to edit.

**INHERITED from the `a51d3c3` baseline. Its own attestation follows, unchanged and NOT re-fired:**
⚠️ **That baseline advanced from `0085676` by the facet / image / BookOrbit catch-up pass, which
re-derived SEVEN things and NOTHING ELSE**, and **the range is large and is emphatically not
documentation-only** — it carries a migration, `internal/`, `cmd/` and `web/src/` — so **read every
box that pass does not name as attested at `0085676`**, not at its baseline line.

**FIRED at `a51d3c3` by THAT pass — inherited verbatim, and *"this pass"* below means the facet /
image / BookOrbit catch-up pass, not the one that advanced the baseline:**
- **The facet read:** `internal/httpapi/facets.go` and its registration in
  `internal/httpapi/server.go`; `TYPE_NAV` in `web/src/routes/+layout.svelte`; every `facet` mention
  under `web/src/`; [ADR-0059](./DECISIONS.md#adr-0059), read directly in `DECISIONS.md`.
- **The unservable-sort action:** `browseUnservableSortAction` and the `store.ErrUnservableSort` arm
  that calls it, in `internal/httpapi/library.go`.
- **The re-rank prose:** `reference/search.md` §4's signal table and the paragraph above it.
- **The image pipeline's three legs, and only those:** the `GET /img/{key}` registration in
  `internal/httpapi/server.go`, and `INSERT INTO image_asset` across all Go outside `_test.go`.
- **The completeness work:** [ADR-0061](./DECISIONS.md#adr-0061), read directly, and
  `internal/db/migrations/00011_sync_report_container_latest_index.sql`, read whole.
- **BookOrbit's real state:** `internal/bookorbit/`'s file list; `BookOrbitSource`'s method set and
  `StreamItems` in `internal/libsync/bookorbit.go`; the `FileSource` type assertion in
  `internal/libsync/importer.go`; `mediaTypeOf` (`internal/store/recent.go`) and
  `browseAudiobookPredicate` (`internal/store/browse.go`); the `bookorbit` arms of `serviceKinds`
  (`internal/httpapi/services.go`), `cmd/usarr/services.go` and `SERVICE_KINDS`
  (`web/src/lib/api.ts`).
- **The `deploy/` stall:** the newest **non-merge** commit touching `deploy/`, measured against the
  newest touching each of `internal/`, `cmd/`, `docs/` and `web/src/`.

**NOT fired by this pass, and therefore inherited:** no `ARCHITECTURE.md` read · no `REVIEW-LOG.md`
read · **no migration read except `00011`** · no Go outside the files named above · **no §2 box this
pass does not name, and no part of the image item beyond its headline and its legs** · **no
line-citation sweep, again.**

⚠️ **A METHOD NOTE THAT COST THIS PASS REAL TIME, RECORDED BECAUSE THE NEXT PASS WILL HIT IT.** The
working checkout was **shallow**, and in a shallow clone `git log -- deploy/` answers with a **merge**
at the graft boundary — a date nobody should trust, and a SHA the citation rule below forbids citing
anyway. The non-merge answer only appeared after `git fetch --deepen`. **The general rule, which is
the header's absence rule in a second costume: a history search in a shallow clone can report a
confident FALSE ABSENCE.** Decide whether something exists from **current file content**, never from
`git log -S`.

**COMMITS LANDED MID-PASS, and the baseline is deliberately NOT advanced past what was read.**
`a51d3c3..f33639f` is two commits — `476e0e8` (`docs/FUTURE.md`, `docs/REVIEW-LOG.md`,
`reference/sync.md`) and `f33639f` (`cmd/usarr/main.go`, `internal/config/`) — and **it touches no
file any of the seven checks above fired on**: no `internal/httpapi`, no `internal/store`, no
`internal/libsync`, no `internal/bookorbit`, no migration, no `web/src/`, no `deploy/`, no
`reference/search.md` and no `DECISIONS.md`. **So none of the seven is stale by it.** Neither commit
was read for anything else.

**INHERITED from the `0085676` baseline. Its own attestation follows, unchanged and NOT re-fired:**
⚠️ **That baseline advanced from `13878f2` by the shipped-state batch pass, which re-derived SEVEN
things and NOTHING ELSE**, and **the range is not documentation-only** — it carries `web/src/`,
`internal/` and a whole new package — so read every box that pass does not name as attested at
`13878f2`, not at its baseline line.

**FIRED at `0085676` by this pass, and this is the whole list:**
- **The Libraries-row link:** `libraryScopeHref` and the row anchor it feeds in
  `web/src/routes/libraries/+page.svelte`.
- **The all-types scoped view:** the browse read in `web/src/routes/library/+page.svelte`, and
  `browseKindCount` / `browseSortsFor` / `browseSortNote` / `BROWSE_AZ_UNAVAILABLE` in
  `web/src/lib/librarygrid.ts`.
- **Who WRITES `?lib=`:** every `set('lib'` and `delete('lib')` under `web/src/`.
- **The shared action text:** the `store.ErrUnservableSort` arm of the browse error mapping in
  `internal/httpapi/library.go`.
- **BookOrbit slice 0:** `internal/bookorbit/`'s file list, `scope.go`'s permission vocabulary and
  `classifyScope`, and the guards in `scope_test.go`.
- **The absent service kind:** `serviceKinds` (`internal/httpapi/services.go`), the kind switch in
  `cmd/usarr/services.go`, and `SERVICE_KINDS` (`web/src/lib/api.ts`).
- **The breaker lift and the ADR ledger:** `internal/breaker/` with the wrappers in
  `internal/kavita/breaker.go` and `internal/servarr/breaker.go`; and `grep -n '^## ADR-005[5-8]'
  docs/DECISIONS.md`, which returned nothing **at this baseline and does not any more** — see the
  mid-pass note below and §3's closing subsection.

**NOT fired by this pass, and therefore inherited:** no migration and no schema read · no
`ARCHITECTURE.md` read · no `REVIEW-LOG.md` read · no Go outside the files named above · **no §2 box
this pass does not name** · **no line-citation sweep, again.** ⚠️ **This pass was deliberately
SMALL and a deeper re-derivation is expected after it** — it recorded state it was handed and
verified, and it did not go looking for state it was not handed.

**COMMITS LANDED MID-PASS, and the baseline is deliberately NOT advanced past what was read.**
`0085676..79d96f8` is `docs/DECISIONS.md` and `docs/REVIEW-LOG.md` only — **no Go, no migration, no
`web/src/`** — so none of the seven checks above is stale by it. **One of those commits falsified a
line this pass had already written**, and the correction is at the end of §3 rather than swapped in
silently. `REVIEW-LOG.md` was not read.

**INHERITED from the `13878f2` baseline. Its own attestation follows, unchanged and NOT re-fired:**
⚠️ **That baseline advanced from `c7d9ed3` by the ADR-0054 / stale-count pass, which re-derived FIVE
things and NOTHING ELSE.** Everything else here is **inherited across that range, and the range is not
documentation-only** — `13878f2` carries Go, `web/src/` and `docs/reference/` changes — so read every
box this pass does not name as attested at `c7d9ed3`, not at the baseline line above.

**FIRED at `13878f2` by this pass, and this is the whole list:**
- [ADR-0054](./DECISIONS.md#adr-0054), read directly in `DECISIONS.md`.
- **The score's wire path:** `searchItem.Score` and the store-order comment in
  `internal/httpapi/librarysearch.go`; `SearchItem` in `web/src/lib/search.ts`;
  `reference/http-api.md` §6.2.1.
- **The re-rank:** `rerankWeightRRF` / `rerankWeightJW` / `rerankWeightRecency` and `rerank` in
  `internal/store/searchlibrary.go`, against `reference/search.md` §4's signal table and
  `REVIEW-LOG.md` LS-191 and LS-192.
- **Test-citation checks:** `git grep TestSearchOrderIsTheServersAndIsNotScoreOrder`, and every test
  citation in `web/src/` non-test sources.
- `grep -rn 'SD-08' docs/` → **no hits**. The review round's own label is not in the tree yet, which
  is why §2's two new items cite the evidence rather than the finding id.

**NOT fired by this pass, and therefore inherited:** no migration and no schema read · no
`ARCHITECTURE.md` read · no Go outside the two files named above · no §2 box except the score item and
the two items it added · no §3 read except the one clause named in the citation note · **no
line-citation sweep, again.**

**COMMITS LANDED MID-PASS, and the baseline is deliberately NOT advanced past what was read.**
`13878f2..81108d9` adds `internal/bookorbit` (slice 0), `internal/breaker`, `librarygrid.ts`'s Music
A-to-Z notice and three `docs/` catch-ups. **It touches no file this pass fired on**, and the four
checks above were **re-run against `81108d9`** and still hold: `search.md` §4 still marks
Jaro-Winkler *"primary"*, the three `rerankWeight*` constants are unchanged,
`TestSearchOrderIsTheServersAndIsNotScoreOrder` still has no `func`, and every `web/src/` test
citation still resolves. **Nothing else in that range was read.**

**INHERITED from the `c7d9ed3` baseline. Its own attestation follows, unchanged and NOT re-fired:**
⚠️ **That baseline moved from `7bd45e9` by the ADR-0052/0053 consolidation pass, and what that pass
fired is listed rather than summarised**, because the intervening range is large — it carries Go,
`web/src/` and `docs/` changes alike, so **inheritance is not safe here the way it was for the two
documentation-only passes before it**.
**FIRED at `c7d9ed3`:**
- **ADRs**, read directly: [ADR-0052](./DECISIONS.md#adr-0052) and [ADR-0053](./DECISIONS.md#adr-0053).
  ⚠️ **NO HIGHEST-ADR NUMBER IS RECORDED HERE ANY MORE, AND REMOVING IT WAS NOT HOUSEKEEPING.** This
  bullet used to end `grep -o '^## ADR-[0-9]*' docs/DECISIONS.md | tail -3` →
  *"~~`0053` is the highest on `main`~~"*. **An agent read that line, believed it, and self-allocated
  ADR-0054 from it.** The number stands — ADR-0054 is merged and has in-tree references — but a
  stale count in a file that is **not authoritative for the count** drove a real allocation decision.
  [`DECISIONS.md`](./DECISIONS.md) is authoritative for ADR numbering; this file is not, so it now
  **points instead of carrying a copy**: run `grep -o '^## ADR-[0-9]*' docs/DECISIONS.md | tail -1`
  when you need the number. **The evidence is recorded rather than deleted because it is the best
  worked example in this file for the bias the whole page is written under** — state what is *owed*,
  and point at the authority for what *is*.
- **Scope**, read directly: `ARCHITECTURE.md` §16's v0.1 entry and §16.1's sequencing table.
- **Screen checks:** `web/src/routes/` and `web/src/routes/library/`; `TYPE_NAV` and `NAV_GROUPS` in
  `web/src/routes/+layout.svelte`; `MEDIA_TYPES` in `web/src/lib/library.ts`; the exports of
  `web/src/lib/librarygrid.ts`; `Library.slug` in `web/src/lib/libraries.ts`; every `href` in
  `web/src/routes/libraries/+page.svelte`; the scope rendering in
  `web/src/routes/library/[type]/+page.svelte`.
- **Go checks:** `handleBrowseWorks`'s `media_type` / `sort` / `lib` parsing and its
  `ErrUnservableSort` action text (`internal/httpapi/library.go`); `browseWorksSQL`'s library
  predicate and its `sort == WorksSortTitle && len(kinds) != 1` refusal (`internal/store/browse.go`);
  `seedLibraryCorpus` (`internal/httpapi/library_test.go`) and
  `TestBrowseEndpointScopesByLibrarySlug` (`internal/httpapi/library_browse_test.go`).
- **`REVIEW-LOG.md`:** LS-260's closure section and the cassette-scrubber entry (LS-340…LS-345).
**THREE COMMITS LANDED MID-PASS AND THAT PASS DELIBERATELY DID NOT ADVANCE PAST WHAT IT READ.**
`adf8960`, `a6bf6c1` and `cad0563` touch `docs/DEVELOPMENT.md`, `docs/design/DESIGN-DIRECTION.md` and
one blockquote in `docs/DECISIONS.md` — **no Go, no migration, no `web/src/`** — so the checks above
are not stale by them. `cad0563` was read directly, because it marks ADR-0052's mockup re-draw
discharged and §3's new deferred row turns on exactly what that mark does and does not cover.
**NOT fired by THAT pass, and therefore inherited:** **no schema check and no migration read** —
nothing below that rests on `internal/db/migrations` was re-derived by it — **no `make check` line
citation was swept**, and **every §2 box it does not name is inherited, not re-fired.**
⚠️ **Kept from the earlier riders, because the lines they attest to are unchanged:** the
probe-correction pass re-derived exactly ONE fact — that `kavita-cover-probe.sh` did run — and fired
NO grep and NO check on this tree. The Go, schema and `ARCHITECTURE.md` reads below were **fired**,
not inherited, at `d5ab034` — the tip when the Kavita-sunset pass started. `fc2b7c4` and `2ce8ed9`
landed mid-pass; both touch **`docs/reference/` only**, change no Go and no migration, and both were
read directly for what §2's obligation 3 says about them.

---

## 1. The objective

1. **v0.1 proves the replica thesis on real data, and its catalogue source is BookOrbit.**
   [ADR-0052](./DECISIONS.md#adr-0052) is **Accepted, owner-decided 2026-08-19**: it amends
   [ADR-0041](./DECISIONS.md#adr-0041) clause 1 and `ARCHITECTURE.md` §16's v0.1 entry, which now
   reads *"the sync core, with one Tier 0 Go adapter in front of it: **BookOrbit**"*. **Kavita is
   sunset — investment stops, the adapter stays in the tree** (§3). The owner's words are in the ADR;
   this file does not re-argue them.

   ⚠️ **THIS BULLET USED TO READ *"~~SUPERSEDED … pending an ADR that is NOT YET WRITTEN~~"* AND
   *"~~until that ADR lands, v0.1's proven source is UNDECIDED — it is not 'BookOrbit' yet~~"*.**
   Both are false now: the ADR landed (`6749365`), was reviewed against BookOrbit's source
   (`6601dce`) and closed out (`f4cc386`). The companion line *"~~`0051` is the highest on `main`~~"*
   is dropped rather than corrected — **and no replacement maximum is written in its place**, for the
   reason the baseline block above now records at length: a maximum in this file mis-allocated an ADR
   once already. [`DECISIONS.md`](./DECISIONS.md) is where the current highest lives. ADR-0053 is
   §2's sidebar decision, and citing a specific ADR by number is not what went wrong.

   **What survives is the rule**, which no re-sequencing has ever moved: **one source, proven on
   real data, before a second adapter**. **What v0.1 now OWES against that rule is the run** — a
   BookOrbit adapter exercised against the owner's own library. ADR-0052 **ships no code by design**,
   so the source is named and the proof is outstanding; naming a source is not proving one.

   ⚠️ **ADR-0041 clause 4's channel list does NOT carry over, and this is the part most easily read
   as settled.** ADR-0052 **reopens** it rather than re-answering it, because clause 4 was earned by
   [ADR-0035](./DECISIONS.md#adr-0035) §2a's probe against a live **Kavita** and BookOrbit has had no
   equivalent run. §16's v0.1 entry now states the narrower shape a source read produced instead:
   channels **1 and 4** for everything, **3b for `work_book` only**, with comics and manga on §7.1a's
   documented **reconciliation-only** fallback. §3 carries why, and it is a constraint rather than a
   gap.
2. Alongside it, **Prowlarr Search-and-Grab** is v0.1's one write path — the request surface for all
   six media types (§8.5, §16).
3. Five screens ship: Home, Services, Libraries, Search, Requests (`CLAUDE.md`, §17).

---

## 2. v0.1 remaining work

Ordered roughly by what the rest depends on, not by size.

⚠️ **EVERY ITEM BELOW THAT NAMES KAVITA IS WRITTEN AGAINST A SOURCE v0.1 NO LONGER TAKES (§1)** —
channel 3b, the per-series volume/chapter walk, the *"not identified"* badge, and the
zero-external-providers evidence clause. **The work each names is real and source-shaped**, and what
each owes is that same work pointed at BookOrbit. **None of them is re-pointed line by line here**,
and the reason has changed: this used to say *"~~re-pointing them is the unwritten ADR's job~~"*, and
the ADR landed — [ADR-0052](./DECISIONS.md#adr-0052) **deliberately declined to re-answer** ADR-0041
clause 4's channel list, reopening it instead. ⚠️ **THE ZERO-EXTERNAL-PROVIDERS EVIDENCE CLAUSE IS
RE-POINTED, 2026-08-20.** Its box below reads *"for BookOrbit"* and its body names BookOrbit's
payloads. **CITE `8cdf399`, NOT EITHER MERGE.** Only the **source name** moved, which is a different
defect from the per-item **rewrite** this sentence defers — the evidence obligation is still owed and
its box is still open. **The rest are untouched by that pass, and this sentence still holds of
them.** ⚠️ **AND THE INVENTORY IS NO LONGER A COUNT.** It read *"channel 3b, **channel 4**, the
per-series volume/chapter walk …"* and *"ONE OF THE FIVE … The other four"*; the channel-4 item was
rewritten on 2026-08-21 and no longer frames its weight in terms of Kavita — the only Kavita left in
it is inside a dated rider quoting what it used to say, which is history rather than staleness
([`DEVELOPMENT.md`](./DEVELOPMENT.md) §11). It is dropped from the list rather than the list
re-numbered, because a count maintained by a different act than the thing it counts is what went
stale here. So the per-item rewrite is owed by whoever writes the adapter, against §16's narrower
channel sentence (§1), and **channel 3b's item in particular cannot simply be re-pointed** — for
`work_comic` there is nothing at the far end to point it at (§3).
Items marked 🛑 **STOPPED** are the different case: those are stopped by the decision itself.

- [ ] **Channel 3b — the ordered page-walk delta, ~~for Kavita~~ for BookOrbit.** ~~The watermark
      walk with an overlap window and a client-side stop, so an import is not the only way the
      replica moves.~~
      ⚠️ **TITLE AND DESCRIPTION BOTH CORRECTED 2026-08-21, AND THEY ARE A DECISION MARK RATHER
      THAN A REFRESH — [ADR-0070](./DECISIONS.md#adr-0070) DECIDED AGAINST BOTH MECHANISMS THIS
      BOX NAMED.** The source moved off Kavita ([ADR-0052](./DECISIONS.md#adr-0052)), and the two
      mechanisms in the struck description are not merely unused: ADR-0070 records **the
      `updatedAt` client-side-stop shape is *"REFUSED, not merely unused"*** and §7.1a's **overlap
      formula *"RETIRED"* for this source**, leaving *"the surviving **5 minutes** recorded as a
      NEW, UNMEASURED constant safe in the large direction"*. **What shipped is arrivals only,
      server-side filtered on `books.addedAt`** — the code says the same at
      `internal/libsync/doc.go`, which scopes §7.1a's client-side stop to *"a source that CANNOT
      express a since-filter, and BookOrbit can (ADR-0070)"*. **Correcting this box's criterion
      while leaving a false title in the same box is not defensible, so both move together.**
      *Authority:* §7.1a **as amended by ADR-0070 for this source**, §16 v0.1 entry,
      [ADR-0041](./DECISIONS.md#adr-0041), [ADR-0070](./DECISIONS.md#adr-0070),
      [ADR-0073](./DECISIONS.md#adr-0073).
      *Done when:* ⚠️ **THE SECOND LEG IS STRUCK 2026-08-21, AND IT WAS A PADLOCK RATHER THAN A
      CRITERION.** It read *"~~`internal/libsync/doc.go` stops listing channel 3b under "NOT
      HERE"~~"*, written when 3b was one thing. ADR-0070 **scoped 3b to BookOrbit** and left every
      other source's 3b unanswered, so `doc.go` now says both at once — *"ALSO HERE, AND
      REACHABLE: channel 3b for BookOrbit"* beside *"Channel 3b for every source that is NOT
      BookOrbit"* under `NOT HERE`. **A grep over the NOT-HERE list still hits**, and goes on
      hitting until Navidrome (v0.4), Audiobookshelf, Kavita and Komga (v1.0) all ship one — **so
      the leg could not come out TRUE inside v0.1 however completely 3b shipped**, while on the
      charitable reading it came out true the moment a comment was edited. It discriminated in
      neither direction, and `doc.go` is a doc comment besides.
      1. ✅ **`internal/libsync` has a delta path.** `internal/libsync/delta.go`'s
         `func (im *Importer) DeltaSync`, declared *"DeltaSync is channel 3b"* in the same file.
      2. ✅ **THE REPLACEMENT, AND IT ASSERTS 3b IS REACHABLE RATHER THAN MERELY BUILT — WHICH IS
         THE STATE THE OLD LEG EXISTED TO CATCH:** `grep -n 'sync/delta' internal/httpapi/server.go`
         must be NON-EMPTY. **Fired 2026-08-21 at this file's baseline:** it returns
         `296:	mux.Handle("POST /api/v1/services/{id}/sync/delta", …)`. **Negative control fired
         rather than assumed:** `git show 5069c91:internal/httpapi/server.go | grep -n 'sync/delta'`
         is **EMPTY, exit 1** — and at `5069c91` `DeltaSync` already existed, so that tree is leg 1
         true and leg 2 false, which is [ADR-0073](./DECISIONS.md#adr-0073)'s *"BUILT, TESTED AND
         UNREACHABLE"* exactly. The stronger form, which presses the route and asserts the walk ran
         and was journalled, is `cmd/usarr/delta_route_e2e_test.go`'s
         `TestPressingTheDeltaRouteRunsAnArrivalsWalkAndRecordsIt`.
      ❗ **BOTH LEGS ARE MET AND THIS BOX IS DELIBERATELY NOT TICKED. WHAT HOLDS IT OPEN IS THE
      RESIDUAL: NO DELTA HAS EVER WALKED A REAL BookOrbit.** Every check above is over fixtures, and
      §1's first objective is *"v0.1 proves the replica thesis **on real data**, and its catalogue
      source is BookOrbit"*, and §3's gate spells out the same rule — **one source, proven on real
      data, before a second adapter** (§16.0, §16.1, [ADR-0036](./DECISIONS.md#adr-0036)). **A delta
      path that has only ever walked a recorded fixture has proven nothing on real data.** The
      missing prerequisite is a live instance, which is §4's. ⚠️ **THIS IS A JUDGEMENT AND
      IT IS MARKED SO A REVIEWER CAN OVERTURN IT:** a reader who holds that a box closes on its own
      written *Done when* alone should tick it and move the residual to §4, and nothing here argues
      that reading is wrong — only that closing on fixtures would make this file say 3b is proven
      when §1 says proving it takes real data.

- [ ] **Channel 4 — reconciliation. The DELETION HALF and GUARD 1 have landed; the drift step, guard
      2, the scheduler and the tombstone reaper have not.** ⚠️ **THE SCHEDULER HAS SINCE LANDED
      (2026-08-21, [ADR-0076](./DECISIONS.md#adr-0076)) and this opening no longer describes the
      tree.** `cmd/usarr/reconcile.go`'s `startReconciler` is the six-hourly timer and `main.go`
      starts, cancels and waits on it. What is still missing is the drift step, guard 2 for the
      \*Arrs, and the tombstone reaper — and the reaper is **decided absent**, not merely unbuilt:
      ADR-0076 Decision 4 rules the seven days a restoration window, and a retention limit is a joint
      decision with guard 1 that nobody has taken. It carries more weight for a catalogue
      source than for an \*Arr: a page walk cannot observe a deletion, and BookOrbit's arrivals filter
      sees no edit at all. ⚠️ **This item read *"with both sweep guards and 7-day tombstones"* and
      framed the weight in terms of Kavita**; [ADR-0074](./DECISIONS.md#adr-0074) ships guard 1
      **wired** and **defers guard 2 for BookOrbit** on a measured void premise, leaving named gaps
      with no guard. Guard 2 for the \*Arrs is unbuilt and re-sequenced with their adapters, not cut.
      ⚠️ **AND THE DRIFT STEP COULD NOT HAVE CAUGHT THE CLASS IT IS MOST OFTEN WANTED FOR —
      RECORDED 2026-08-21, POINTING AT THE ADR RATHER THAN RE-DECIDING IT.**
      `internal/store/catalogue.go`'s `remoteHash` hashes **nine** values and `store.CatalogueItem`
      carries **no credit field of any kind**, so a credits-only upstream edit — an author or a
      narrator corrected and nothing else — moves no hash.
      [ADR-0074](./DECISIONS.md#adr-0074) is the argument and it closes the obvious fix: widening
      the hash is refused, because a gate that cannot see credits, *"placed anywhere upstream of
      the credit pass … **actively suppresses the one pass that could have corrected the row**"*.
      **The residual is CLOSED by unconditional re-apply, not by the hash**, and this rider
      restates none of that — read the ADR.
      ⚠️ **AND THE OBVIOUS READING OF THAT — *"so the class goes unrepaired"* — IS FALSE, WHICH IS
      WHY IT IS MEASURED HERE RATHER THAN INFERRED.** Channel 3b cannot see it:
      `internal/libsync/delta.go`'s `streamAndApplyCredits` runs over the **arrivals** set, and an
      already-imported row is not in it. **Channel 4 CAN**: `internal/libsync/importer.go`'s
      `FullImport` calls `streamAndApplyCredits` over every non-child item with **no filter on the
      chain**, and since [ADR-0076](./DECISIONS.md#adr-0076) `cmd/usarr/reconcile.go`'s
      `startReconciler` calls `FullImport` on `reconcileInterval = 6 * time.Hour`, asked at
      `reconcileTick = 30 * time.Minute`. **So the class is repaired unattended, bounded at 6 h
      30 m** — provided the process is up and `reconcileDue` finds `last_full_sync_at` set, which
      it does not for an instance that has never completed one.
      ⚠️ **AND THE REMEDY IS NOT INVISIBLE. THAT WAS CHECKED, NOT ASSUMED.** The Services screen's
      `Last successful sync` column renders `last_full_sync_at` (`web/src/lib/services.ts`'s
      `syncCell`), and `StampFullSync` writes that field **inside the same pass that re-applies the
      credits** — so the column is a truthful marker of when they were last re-applied, and it
      advances on its own. **What is genuinely unstated is narrower, and is all this rider
      claims:** nothing user-facing names the six-hourly clock — `grep -rniE 'six.hour|every 6
      hour|6 hours' web/src/ docs/CONFIGURATION.md` is **EMPTY, exit 1**, and the interval is a
      constant with no configuration key — and **no screen carries a per-channel freshness at
      all**: `describeSync()` in `$lib/services` has **no caller** outside its own tests, and
      `POST /api/v1/services/{id}/sync/delta` has **no `web/src` caller** either, its only mention
      under `web/src` being a comment. **Both against the control that the full-sync route IS
      called** — `$lib/api`'s `syncService` is imported and invoked by
      `web/src/routes/services/+page.svelte` — so the emptiness is these two surfaces' absence and
      not a grep that cannot see a caller.
      *Authority:* §7.4, §16 v0.1 entry, [ADR-0074](./DECISIONS.md#adr-0074),
      [ADR-0076](./DECISIONS.md#adr-0076).
      *Done when:* ⚠️ **THE OLD CHECK NOW PASSES AND PROVES ALMOST NOTHING** — it was
      `grep -rn 'missing_since\|orphaned_at' --include=*.go internal/` showing a statement that
      **sets** a non-NULL value, on the premise that *"today every one clears it"*, and
      `internal/store/reconcile.go`'s `sweepContainers` and `sweepOrphans` now set **both**. What is
      left is the greps that still come back empty: a `SELECT` in non-test Go naming `remote_hash`
      (the drift step — there is none, for any source); and a reader of `identity_fingerprint` or
      `max_remote_id_seen` (guard 2).
      ⚠️ **A THIRD CHECK IS STRUCK 2026-08-21 RATHER THAN DELETED, BECAUSE IT IS A FALSE NEGATIVE BY
      CONSTRUCTION:** ~~and any caller of `SweepDeletions` other than `FullImport` (there is no
      scheduler, and nothing hard-deletes a tombstone after seven days)~~. **The scheduler that
      shipped calls `FullImport`** — deliberately, because `SweepDeletions`' precondition is
      satisfiable only by a real upstream read — so that grep stays empty *while a scheduler exists*.
      Measured at both ends: at `f87aef44` and at this branch's tip the only non-test call of
      `SweepDeletions` is `internal/libsync/importer.go`'s, inside `FullImport`. **The second half of
      the struck parenthetical is still TRUE** — nothing hard-deletes a tombstone after seven days
      (ADR-0076 Decision 4) — and it is carried in the opening above rather than in a check that
      cannot see it.
      **The check that replaces it, and it asserts the loop is WIRED rather than merely defined:**
      `grep -n 'startReconciler(' cmd/usarr/main.go` must be NON-EMPTY. Verified to discriminate:
      empty at `f87aef44`, and `204:	reconcilerDone := a.registry.startReconciler(reconcilerCtx)` at
      this branch's tip. The stronger form of the same assertion, which also pins the shutdown wait,
      is `cmd/usarr/reconcile_loop_test.go`'s `TestTheReconcilerIsStartedFromRun`.

- [x] **Search over your own library — the read path AND the SCREEN.** Both landed 2026-08-18.
      `GET /api/v1/search` answers a flat ranked list off the local corpus at `04a28a4` — the handler
      is `internal/httpapi/librarysearch.go:125` `handleLibrarySearch`, which calls
      `store.SearchLibrary` (`internal/store/searchlibrary.go:490`) and reaches `search_fts` through
      `keywordLeg` (`:204`, `WHERE search_fts MATCH ?`) plus a trigram leg — two SQLite statements, no
      \*Arr call, no provider, no image fetch. Contract in
      [`reference/http-api.md`](./reference/http-api.md) §6. The screen landed at `cbf82bc` (merged
      `5035f4c`) and Home's search box was moved off Requests onto it at `23369ee` (merged
      `0c89420`). `GET /api/v1/releases/search` is the Prowlarr indexer fan-out, moved there at
      `4a51bd4` — a different thing over a different corpus, and the Search screen does not call it.
      *Authority:* §8.2, §17.4, §4.5, §16 v0.1 entry.
      *Was done when:* `web/src/routes/search/+page.svelte` stopped being the gap notice — it is now a
      551-line screen whose only read is `$lib/search.fetchSearch` over `LIBRARY_SEARCH_URL =
      '/api/v1/search'` (`web/src/lib/search.ts:44`), registered at
      `internal/httpapi/server.go:300`.
      *This box does NOT cover, and neither ships:* §4's grouped card — `grep -rn work_relation
      --include=*.go internal/` finds comments only, no reader of the edges — and the tier-1 client
      prefix index. Both are named in `librarysearch.go`'s and the screen's headers rather than
      quietly omitted.

- [x] **DRAWN 2026-08-20 — ~~Home Block A — the media-type summary. Blocks B and C are drawn; A is
      not~~.** ⚠️ **THE TICK IS SCOPED TO WHAT IS DRAWN, AND IT IS NOT THE WHOLE OF §17.2's ROW. Read
      the two riders under it before quoting this box.** `web/src/routes/+page.svelte`'s ADR-0028
      block map now reads *"Block A · media-type summary · ≤6 rows · DRAWN in `library` mode"*,
      beside Block B's *"DRAWN"* and Block C's *"DRAWN in `library` mode"*; the screen imports
      `fetchLibraryFacets`, calls it, and feeds `librarySummary(facets, health)` into `summaryRows`.
      The six rows are drawn and ~~each carries one of §17.7's states~~ each carries a state.
      ⚠️ **"ONE OF §17.7's STATES" IS FALSE AS OF 2026-08-20 AND IS STRUCK HERE RATHER THAN
      QUIETLY DROPPED.** Two of the three ARE specified and **neither is §17.7's**: `unconfigured`
      and `importing` are **§17.2's**. The third, `ok`, has **no located design source at all** —
      `web/src/lib/home.ts`'s `SUMMARY_STATE` note **withdrew** the §17.7 citation rather than
      re-pointing it, *"because no replacement was located"*. **The `*Authority:*` line's `§17.7`
      below is stale in the same way, and is left standing because §17.7 does govern the rest of
      this box.** **See the new entry below this one, which is where that gap is carried.**
      Content commits `51a9e68`
      (*"feat(home): Block A draws the six media types off the facet read"*) and `da33aa7`
      (*"fix(home): Block A's counts and its reach are read off the install, not the build"*),
      attributed by `git log -S` on `fetchLibraryFacets` and on the block-map string
      *"media-type summary    ≤6 rows           DRAWN"* rather than by subject, and each confirmed
      **single-parent** before being cited.

      ⚠️ **`Have` AND `Synced` ARE NOT DRAWN, AND THAT IS SCOPE RATHER THAN OMISSION.** §17.2's row
      is `name · count · availability rollup · last import · see all`, and the refusal sits
      **upstream of the screen**: `internal/httpapi/facets.go` answers only the first two and says so
      at its own declaration — *"the rollup and the import time are their own aggregates and their
      own commit"* — and `toMediaTypeCounts` is a hand-written allowlist in which *"every field is
      named"*. `+page.svelte` says the same in terms, under `BLOCK A's COLUMNS, AND THE TWO §17.2
      NAMES THAT ARE DELIBERATELY ABSENT`: a value in either column *"would be invented"*, and `Have`
      in particular is a specified figure. **A screen cannot draw a column its read refuses to
      answer**, so this is owed by those aggregates, not by Home.
      ⚠️ **§17.7's `stale` state is not drawn either.** The same file's `WHAT IS STILL NOT DRAWN`
      paragraph names it. `partial` **is** drawn — as *"first import running"* with no number beside
      it. **So this tick says Block A is drawn; it does NOT say Block A is complete.**
      *Authority:* §17.2 as amended by [ADR-0028](./DECISIONS.md#adr-0028), §16 v0.1 entry, §17.7.
      *Done when:* ~~`web/src/routes/+page.svelte`'s block map stops saying `Block A … NOT DRAWN`~~
      — **that criterion is satisfiable by a text editor alone, which is exactly what this file's
      done-when rule refuses.** It is ticked against it anyway, on that rule's two conditions.
      **The unfired obligation:** Home has never been rendered against a running UsArr over a
      populated catalogue — nothing here has watched a count arrive on a screen.
      **The missing prerequisites, named:** `web` has **no component-render harness at all**
      (`git grep -l 'render(' -- 'web/src/**/*.test.ts'` returns nothing; `web/src/lib/home.test.ts`
      is an `environment: 'node'` vitest over pure functions plus a copy guard that reads
      `+page.svelte?raw` **as text**), and this environment has no live catalogue source to populate
      the six counts from. **The running-system criterion this box now owes, and which nothing here
      can take:** Home in `library` mode draws six rows whose counts equal
      `GET /api/v1/library/facets` on a real install.

- [ ] **A per-type `ok` STATE SHIPS ON HOME AND NO DESIGN DOCUMENT SPECIFIES IT. This is a DECISION
      owed by §17's owner, not code owed by a builder** — nothing here says the state is wrong, and
      **the box must not be read as asking for it to be removed.** Block A gives every media-type
      row a `Status` word, because a `Status` column that is blank on a healthy row reads as
      *status unknown* rather than *status fine* — and the word for the healthy row is
      `catalogued`, keyed `ok` in `web/src/lib/home.ts`'s `SUMMARY_STATE`. **Its two neighbours are
      sourced:** `unconfigured` and `importing` are both **§17.2's**. `ok` is not.
      ⚠️ **THE TREE WITHDREW THE CITATION RATHER THAN RE-POINTING IT, AND THE SEARCH BEHIND THE
      WITHDRAWAL IS BOUNDED — QUOTE THE BOUND WITH THE CLAIM.** `home.ts`'s `SUMMARY_STATE` note
      records that `docs/ARCHITECTURE.md` (§17.7 **and** §17.2) and
      `docs/design/DESIGN-DIRECTION.md` were searched, that §10's required-state table carries
      `empty`, `filtered-empty`, `scope-empty`, `partial`, `stale`, `error` and `unconfigured` and
      **no `ok` row**, and — in its own words — *"Nothing here says the state is invented,
      unspecified or wrong — only that a source for it was not found in those two files."*
      DESIGN-DIRECTION §3.2 does define a status **colour role** named `ok`; the note neither rules
      it in nor out as the origin, and **neither does this box.**
      ⚠️ **`catalogued` IS DELIBERATELY NOT `Up to date`, AND THAT REASON HAS ITSELF EXPIRED —
      FLAGGED, NOT RE-DECIDED.** `home.ts` gives as its reason *"There is no periodic re-sync in
      this build — `cmd/usarr/import.go` runs at most one import per instance per database, on
      connect, with no timer behind it"*. **That sentence is FALSE since
      [ADR-0076](./DECISIONS.md#adr-0076)**: `cmd/usarr/reconcile.go`'s `startReconciler` is a
      six-hourly timer over `FullImport`. Whether a freshness word is now measurable is part of
      what §17's owner is being asked, and **nothing is chosen here.** ⚠️ The stale sentence is in
      the tree, not in this file, and **fixing it is not this box's** — it is named so the next
      reader of `home.ts` is not misled by it.
      ⚠️ **NOT A BANNER GAP AND NOT AN EMPTY-STATE GAP — both were checked and both are clean**, so
      do not widen this box into them. §17.7's degraded banner IS on `/library` and on the per-type
      grids, landed at content commit `712db17` (*"feat: show §17.7's degraded-backend banner on the
      catalogue screens"*) and guarded at `019775b` (*"test: guard the degraded banner in both
      directions"*); and the per-type empties are fully differentiated by `browseEmptyState` over
      `recentEmptyState`'s four cases. **Both SHAs were re-derived rather than inherited: an earlier
      relay of this finding cited `ed3a948` for the banner, and that commit is *"test: the
      corpus-floor figures re-measured on the tree they describe"* and touches
      `web/src/lib/designrules.test.ts` only.**
      *Authority:* §17.2 as amended by [ADR-0028](./DECISIONS.md#adr-0028) for the two sourced
      states; **§17 owns whether a third one exists, and §16 is silent** — this is a design
      decision, not a milestone question.
      *Done when:* **two legs, and leg 1 can be satisfied in either direction.**
      1. Either §17 / DESIGN-DIRECTION §10's required-state table names a state for a counted
         per-type row, **or** `SUMMARY_STATE.ok` is retired — and either way
         `grep -n 'that citation is withdrawn' web/src/lib/home.ts` comes back **EMPTY**. **Fired
         2026-08-21: NON-EMPTY**, `web/src/lib/home.ts:638`. **Negative control fired rather than
         assumed:** the same grep for `that citation is restored` is **EMPTY, exit 1**, so the
         shape can come out empty and the non-emptiness above is the note's presence.
      2. **The running leg, and it is a guard that has been TRIGGERED rather than inferred from its
         presence:** `cd web && pnpm vitest run home` stays green on whatever word the row then
         carries, and **FAILS** when that word is blanked — the blank-`Status` regression the state
         exists to prevent. **Fired in both directions 2026-08-21: 64 passed; with `ok: ''`, 2 of
         64 fail.**

- [ ] **Libraries — the auto-proposal flow and its Accept step.** The row view over
      `GET /api/v1/libraries` is drawn; the proposal step is not, and its storage question is now
      answered: a proposal is **not** a `library` row, and a row is created only on Accept.
      *Authority:* §17.8, [ADR-0026](./DECISIONS.md#adr-0026), [ADR-0048](./DECISIONS.md#adr-0048).
      *Done when:* an Accept path exists that creates a `library` row, and the proposal has a home
      that is not that table.

- [ ] **The per-series volume and chapter walk, and the rows it writes.** Phase A is served; the walk
      that fills `work_comic_issue` and `media_file` is not fetched and is not faked.
      *Authority:* §7.2, `internal/libsync/doc.go`.
      *Done when — TWO LEGS, AND THEY HAVE COME APART. THE BOX STAYS OPEN.* The clause used to read
      *"`internal/libsync/kavita.go` performs the walk **and** `work_comic_issue` has a writer in
      non-test Go"*, as one conjunction. It is split here because one leg closed and the other lost
      its subject, and ticking the box on the first would have claimed the second.
      1. ✅ **`work_comic_issue` has a writer in non-test Go, and it is reached.** `INSERT INTO
         work_comic_issue` has exactly one hit outside `_test.go` — the `case "comic_issue"` arm of
         `internal/store/catalogue.go` — and the comics import reaches it (the box below). **This
         leg closed silently**: nothing on this page moved when it did.
         ⚠️ **Ticked against the written criterion, not against a run.** The criterion is
         *"has a writer"*, which a text editor can satisfy, so this file's own Done-when rule applies
         — **the unfired obligation is that no `work_comic_issue` row has been observed from a real
         import**, and the missing prerequisite is a live BookOrbit instance, which is §4's and which
         no test in this repo can stand in for.
      2. ❌ **`internal/libsync/kavita.go` performs the walk** — and this leg names **a source v0.1
         no longer takes** ([ADR-0052](./DECISIONS.md#adr-0052)). The Kavita walk is genuinely absent
         as written: `kavita.go` declares `SeriesVolumes` on its source interface, and the file walk
         that consumes it lives in `internal/libsync/files.go`, not in `kavita.go`. **But re-pointing
         the leg at BookOrbit is not a rename** — BookOrbit has no volume/chapter level to walk, so
         what this leg owes against the current source is **undecided**, and §2's opening ⚠️ already
         says whoever writes that decision owns it. **Nothing here decides it.**
         ⚠️ **DATED 2026-08-21, AND THE UNDECIDED VERDICT IS NOW CITED RATHER THAN ASSERTED —
         NOTHING BELOW DECIDES ANYTHING EITHER.** The paragraph above was true when written and
         carried no date, so a later reader cannot tell whether it predates
         [ADR-0068](./DECISIONS.md#adr-0068). It does not. **Re-pointing has no target, on two
         measurements that meet:** ADR-0052 records *"The series half is not an open question.
         There is no series-level ordered read in BookOrbit at all"*, measured against the clone
         at `73b7877d` and re-checked at `4a420a04`, identical at both; and ADR-0068 rules **a
         BookOrbit comic is an issue** — *"issues are **minted under series works**"* — so a comic
         **is** one file and there is no volume or chapter level beneath it to walk.
         ⚠️ **THAT SECOND HALF IS LOAD-BEARING, BECAUSE ADR-0052's OWN TABLE NAMES A ROUTE THAT
         LOOKS LIKE THE RE-POINT TARGET AND IS NOT:** `GET /series/:seriesId/books` exists — the
         series controller exposes *"exactly two"* routes and that is one of them. It is not the
         walk. `internal/bookorbit`'s catalogue doc excludes the series endpoints **on a
         measurement rather than on a budget** — *"there is no series watermark to walk, and every
         fact they carry rides the book stream already"* — and the BookOrbit analogue of the walk,
         `internal/libsync/bookorbitfiles.go`'s `StreamFiles`, **issues no GET at all** and reads
         the card `bookorbit.go`'s `keepCard` already holds. **So what this leg owes is not a
         rename and not a new endpoint: it is a decision about whether anything remains owed once
         channel 1 delivers the issue and its parent in one pass, and NOTHING HERE TAKES IT.**
         ⚠️ **`internal/libsync/kavita.go` IS NOT A DANGLING PATH** — ADR-0052 keeps it
         deliberately, *""sunset" explicitly does NOT mean delete"*, so it still exists and still
         builds; what went stale is the source slot, not the file.

- [x] **Comics import as ISSUES under series works — LANDED, AND UNVERIFIED AGAINST REAL DATA.**
      `internal/libsync/bookorbit.go` **no longer early-returns on `MediaKindComic`**: the
      `bookorbit.MediaKindComic` arm of its `MediaKind()` switch calls `mapComic`, and
      `internal/store/catalogue.go` writes the parent series work and the child issue in **one
      transaction**, so `parent_work_id` is never null on a child. This is
      [ADR-0068](./DECISIONS.md#adr-0068) — *a BookOrbit comic is one file, so it is an issue* — and
      **ADR-0066 decision 5 activated with it**: a `comic` series is filed into a `comic` library
      minted lazily over the same `library_source` container ref, so one container ref may
      legitimately name two libraries.
      *Authority:* [ADR-0068](./DECISIONS.md#adr-0068), [ADR-0066](./DECISIONS.md#adr-0066)
      decision 5, [ADR-0030](./DECISIONS.md#adr-0030)'s series/issue model, §16 v0.1 entry.
      *The chain, content commits only, each confirmed single-parent before being cited:* `1c35d18`
      **the import slice** · `10444a4` keys the parent cache by container as well as upstream id ·
      `ff13582` names the sibling library for its kind and records a kind change · `04d1620` makes a
      container's libraries follow what the walk actually found · `0a5d66e` stops a mixed container's
      names encoding traversal order. ⚠️ **`ff13582` is NOT the import slice**, and was handed to
      this pass as though it were; it is the fourth commit in the chain, and `1c35d18` is the first.
      *Ticked against the written criterion:* `internal/libsync/bookorbit.go` maps comics rather than
      skipping them, and `INSERT INTO work_comic_issue` has a non-test writer that path reaches.
      **No migration, no column, no DDL, no new wire field** — the acquisition cost was four fields
      on the existing allowlist and **zero extra HTTP**.

      ❗ **THE UNFIRED OBLIGATION, STATED BENEATH THE BOX BECAUSE THE TICK IS NOT A CLAIM THAT THIS
      WORKS. NOTHING HERE IS VERIFIED AGAINST REAL DATA.** Every check behind the tick is against
      **fixtures** — recorded cassettes and Go tests — and the owner's own import is the **first real
      contact**. **The missing prerequisite is a live BookOrbit instance with comics in it**, which
      is §4's and which no test in this repo can perform. ADR-0068 states its own done-check as
      *"after a live import against a real BookOrbit, all four must hold"*, and **none of the four
      has been run**:
      1. `work.kind = 'comic_issue'` rows exist, and **zero** have `parent_work_id IS NULL`.
      2. `work.kind = 'comic'` rows exist and are **strictly and substantially fewer** than the issue
         rows. ⚠️ **Parity here is a FAILURE, not a near-miss** — it is the per-row implementation
         passing itself off as the accepted one, and it is the only outcome worse than not shipping.
      3. `/library/comics` renders **series, not issues**, with a non-zero facet count.
      4. The latest `items_skipped` row's `Comics` field reads **0**.
      Only check 2 can tell the accepted shape from the refused one. **Until the import runs, this
      box records that the code exists and is gated — not that a comic has ever reached a screen.**

- [ ] **The "not identified" badge and ~~the column under it~~ THE DERIVATION under it.**
      ~~Free Kavita returns null identifier
      fields, so this is v0.1's ordinary case, not an edge one.~~ The badge is v0.1; **the remedy is
      not** — see §3.
      ⚠️ **STRUCK 2026-08-20: THE ARGUMENT, NOT THE ITEM.** That clause reasons from **Kavita** as
      v0.1's source, and [ADR-0052](./DECISIONS.md#adr-0052) moved v0.1's source to **BookOrbit**.
      **What replaces it is not written here, because this pass did not measure it** — how often a
      BookOrbit card carries no external identifier is a question about BookOrbit's payloads, and
      nobody has run the count. §3's *"Verified facts"* bullets are the nearest thing on this page to
      an answer and they are narrower than one: `comicvineId` **exists**, and MangaUpdates, AniList
      and MyAnimeList ids are **absent**. **Neither settles the prose case.** ⚠️ **Do not read the
      strike as *"the ordinary case is now identified"*** — it says only that the sentence's
      evidence is about a sunset source. **The badge is owed either way**, which is why the box is
      untouched.
      *Authority:* §6.4, §16 v0.1 entry, [ADR-0035](./DECISIONS.md#adr-0035) §1.
      *Done when:* a work whose ~~identifier column is null~~ **renders the badge in the browser**,
      ~~off a column that exists in `internal/db/migrations`~~, on a library imported from a real
      instance.
      ⚠️ **The clause used to read *"~~the state is rendered in `web/src/routes` off a column that
      exists in `internal/db/migrations`~~"*, which a text editor alone can satisfy** — a `.svelte`
      file and a migration both being present is true of a tree nothing has run. **Strengthened
      under this file's own Done-when rule**, and deliberately **not** ticked: the run it now names
      has not been taken.
      ⚠️ **DATED RIDER 2026-08-21 — THERE IS NO SUCH COLUMN, AND THE TWO CLAUSES THAT ASSERT ONE
      ARE STRUCK IN PLACE ABOVE RATHER THAN DELETED.** **`work` has no identifier column and no
      migration creates one.** Measured across all **thirteen** files in `internal/db/migrations/`:
      `work`'s DDL in `00005_library_sync.sql` names none, **no migration ALTERs `work` at all**,
      and the schema's only identifier-shaped column is `service_instance.needs_reidentification`
      (`00001_initial.sql`) — a column about re-identifying a **service**, named here so it is not
      mistaken for this one twice. `reference/schema.md`'s own `work` block agrees.
      ⚠️ **AND THIS BOX NAMES NO COLUMN, SO NONE IS SUPPLIED HERE.** It asserts a column's
      **existence and shape** — *"a work whose identifier column is null"*, *"a column that exists
      in `internal/db/migrations`"* — and never a name. Its authority, ARCHITECTURE §6.4, names none
      either: it says *"the nullable column belongs on `work` from the migration that creates it"*
      and *"It costs one nullable column and one badge"*. **Inventing a name for a column nobody
      specified is how a second wrong claim gets written to repair the first, so the clauses are
      retired rather than corrected.**
      🛑 **DO NOT READ THAT AS *"THE DONE-WHEN IS NEARLY MET"*. NOTHING RENDERS THIS STATE.** The
      tree does compute an identity state — `internal/store/catalogue.go` reads
      `SELECT EXISTS (SELECT 1 FROM external_id WHERE work_id = ?)` and the code says of itself that
      *"§6.4 describes it as costing 'one nullable column', and migration 0005 shipped no such
      column"* — **but that `EXISTS` is not the badge's source and must not be written down as
      though it were.** It feeds exactly one field, `store.BatchResult.Unidentified`, whose only
      consumers are **three structured-log fields** on the **WRITE** path (`cmd/usarr/import.go`
      twice and `internal/libsync/importer.go` once) plus the merge that sums them. **No read path
      publishes the state, no response field carries it, and `web/src` contains no "not identified"
      badge at all** — `grep -rniE 'not identified|not_identified|notIdentified|unidentified'
      web/src/` is **EMPTY, exit 1**, and the only `badge` hits under `web/src` belong to the
      **severity** badge. **It is an import-report counter, not a rendering. The item stays open;
      what changed is only what would discharge it.**
      ***Done when*, RESTATED:** a work with **no `external_id` row** renders the badge in the
      browser, **off a state a read path publishes**, on a library imported from a real instance.
      **The migration-column clause is RETIRED, not weakened** — this is still a criterion only a
      running system can satisfy, and the run it names has still not been taken.
      ⚠️ **AND THE DESIGN HAS NOT MOVED TO MATCH. THAT DIVERGENCE IS RECORDED, NOT CLOSED, AND IT
      IS NOT THIS FILE'S TO CLOSE.** ARCHITECTURE §6.4 still reads *"the nullable column belongs on
      `work` from the migration that creates it"*, and `DECISIONS.md` still repeats *"The nullable
      column and the badge"* in two places. **No ADR decides the derived shape**; the tree took it
      in a code comment. §16 is scope authority and the tree is status authority, so this rider
      records the divergence, **allocates no ADR number and guesses none** —
      [`DECISIONS.md`](./DECISIONS.md) is authoritative for the next free one.

- [ ] **The image pipeline's FETCH HALF — NARROWER AGAIN. The writer, the renderer and the import
      call site all landed; what is left is §4.4.1's cold-start plan and a first run against a real
      cover.**
      ⚠️ **THE HEADLINE USED TO READ *"~~`image_asset` is in the schema; nothing in `internal/` or
      `cmd/` writes or serves it~~"*, AND THE *SERVES* CLAUSE WENT FALSE FIRST.** `GET /img/{key}` is
      registered in `internal/httpapi/server.go` and handled by `internal/httpapi/images.go`
      (`34a277f`), and `ffebec7` puts `poster_key` on **every response that renders a work row**.
      ⚠️ **FALSIFIED 2026-08-19 — THE *WRITES* CLAUSE IS NOW FALSE TOO, AND SO IS EVERY
      CONSEQUENCE THIS ITEM DREW FROM IT.** The old text is kept visible because it is what a reader
      would otherwise still believe: *"~~The **writes** clause still holds … nothing outside
      `_test.go` writes an `image_asset` row, so nothing fetches, decodes or stores a cover — every
      `/img` request answers `not_cached` and every row's `poster_key` is absent on every real
      install.~~"* **Every clause of that is wrong on the baseline above**, and the checks that
      falsified it are leg 2 and Obligations 1 and 3.
      **What actually exists, read off the tree rather than off a commit subject:** `PutPosterAsset`
      (`internal/store/imagewrite.go`) is a **non-test** `INSERT INTO image_asset`, and it is called
      by `Pipeline.Poster` (`internal/imagepipeline/pipeline.go`), which fetches the cover, decodes
      it, renders one JPEG per allowlisted width (`renderAll`, `internal/imagepipeline/render.go`)
      and writes the bytes through `internal/imagecache` before recording the row. **Content commit
      `7e5934d`**, cited rather than the merge that carried it, per this file's standing rule.
      **And it has a production caller.** `Importer.fetchCovers` (`internal/libsync/covers.go`) is
      **phase D** of a full import — a bounded loop over `Poster` that runs **between committed
      batches** and never inside one, skipping items `store.PosteredItems` says already carry
      artwork — called from `FullImport` in `internal/libsync/importer.go`; and
      `cmd/usarr/import.go`'s `coverPipeline` builds the fetcher from the instance's **own**
      BookOrbit client, returning `nil` (pass disabled) for any other kind or with no image-cache
      directory. **Content commit `c4a3277`.**
      🛑 **DO NOT READ THAT AS *"COVERS WORK"*. NOTHING HAS EVER PUT A REAL COVER THROUGH IT**,
      and the package says so about itself rather than letting a green test suite be the only signal
      — `internal/imagepipeline`'s package doc: *"This pipeline has been TESTED AGAINST A FAKE
      FETCHER AND NEVER AGAINST A REAL COVER. Every image it has ever processed was fabricated by its
      own tests; no byte from a running BookOrbit has been through it."* It draws the comparison
      itself, and this file endorses it: **that is the same register as `deploy/Dockerfile`'s
      written-not-built.**
      ✅ **THE STALE COMMENT THIS ITEM RECORDED IS NOW FIXED.** `internal/httpapi/images.go`'s
      package header said *"What no code does is CALL it during an import"* — written at `7e5934d`,
      when it was true, made false by `c4a3277`, and not revisited until 2026-08-19. The header now
      names `c4a3277` as the falsifier and leaves the original claim legible. Four further copies
      were corrected in the same commit: `internal/httpapi/server.go`, `internal/imagecache`,
      `internal/httpapi/library_test.go`, and `reference/http-api.md` §1 and §9.4. **Its
      neighbouring sentence — that the pipeline has never run against a cover from a running
      service — is still correct, and is kept as a separate claim rather than folded in.**
      🔻 **FALSIFIED 2026-08-19 by the owner's report that the library grid shows cover art.**
      Both own-voice claims above — *"NOTHING HAS EVER PUT A REAL COVER THROUGH IT"* and *"its
      neighbouring sentence … is still correct"* — were true when written and are not true now; they
      are left legible rather than rewritten. The report establishes only that the path ran **once**,
      end to end, against **BookOrbit**: fetch, decode, `renderAll`, `PutPosterAsset`, `/img` serving
      from cache. ⚠️ **It is an install fact with no falsifier in this repository** — no test reaches
      that machine — and it narrows **none** of the three gaps: coverage is unmeasured
      (`items_skipped` unanswered for covers), **§4.4.1's cold start is untouched** (`thumbhash`,
      `dominant_color`, `etag`, `last_modified`, `expires_at` still have no writer — `PutPosterAsset`'s
      INSERT names none of the five), and `coverGate` has never been known to be contended.
      `internal/imagepipeline`'s package doc carries the same correction and is the site of record.
      ⚠️ **DATED RIDER 2026-08-21 — THE REPORT STANDS; THE FALSIFICATION DRAWN FROM IT DOES NOT.**
      The mark above is struck in place and kept legible — *"~~🔻 FALSIFIED 2026-08-19 by the
      owner's report that the library grid shows cover art~~"* — and with it the finding beneath
      it: *"~~The report establishes only that the path ran **once**, end to end, against BookOrbit:
      fetch, decode, `renderAll`, `PutPosterAsset`, `/img` serving from cache~~"*. **That chain
      needs the library grid to have been drawing `/img`'s output on 2026-08-19, and the dates say
      it was not.**
      **THE DATES ARE THE LOAD-BEARING FACT, AND THEY DO NOT DEPEND ON WHICH TREE YOU READ THIS
      AT.** The report is dated **2026-08-19**. **The first cover rendered ANYWHERE in this
      repository is `163f608`, 2026-08-21 08:02 UTC** — *"draw covers on Home's Block C behind a
      Posters view toggle"* — whose own message opens *"`posterUrl` has had no caller since it was
      written. **This is the first one**"*. **The first cover in the LIBRARY GRID is `a34d87f`,
      2026-08-21 13:59 UTC** — *"a posters view on /library and the per-type grids"*. **Both are
      two days AFTER the report.** And on the day itself the catalogue screen said it about
      itself: at `db9b028` (2026-08-19 20:31 UTC, the last commit of that day to touch it),
      `web/src/routes/library/+page.svelte`'s header read *"COVERS ARE ABSENT FROM THIS SCREEN …
      what is missing is the poster **VIEW** — this screen draws rows and no artwork"*; and
      `ad821a06` (2026-08-19 04:44 UTC), the commit that built the per-type grid, says *"the
      poster view is unbuilt and the route header says so."*
      ⚠️ **THE GRID DOES DRAW COVERS NOW, AND THAT IS NOT A REBUTTAL — IT IS WHY THE DATES ARE
      QUOTED INSTEAD OF A CENSUS.** As of this file's baseline the web tree has **exactly one
      rendered `<img>`** — `web/src/lib/PosterGrid.svelte` — reached from Home, `/library` and
      `/library/[type]`. **A reader measuring today finds covers on the grid and could mistake that
      for confirmation of a 2026-08-19 report.** It is not: the element arrived after the report,
      so a census of the current tree cannot corroborate it and the timeline is the only evidence
      that bears.
      ⚠️ **WHAT IS *NOT* RETRACTED, because the narrow claim is the whole point.** The backend links
      were there: `7e5934d` (2026-08-19 15:22 UTC), the pipeline writer, and `c4a3277`
      (2026-08-19 16:49 UTC), the import call site, both predate the report. **Nothing here says
      the pipeline could not have run.** It says only that **the screen the report names could not
      have shown its output on that date**, so the report cannot be the evidence that it did. The
      inference fails at the rendering, not at the pipeline.
      ❓ **WHAT THE OWNER ACTUALLY SAW IS OPEN, AND IT IS JOE'S TO ANSWER. NO HYPOTHESIS IS
      SUBSTITUTED HERE** — a second guess dressed as a finding is precisely how the first one came
      to be written, and one is not repaired by another. **The question, addressed to him and left
      for him:** on which screen, at what address, and on which build was the cover art seen?
      Until he answers, the two own-voice claims above are **restored rather than struck**, **with
      no repository falsifier** — which is exactly what the struck mark itself conceded when it
      said *"it is an install fact with no falsifier in this repository."*
      ⚠️ **`internal/imagepipeline`'s PACKAGE DOC IS THE SITE OF RECORD AND CARRIES THE SAME
      UNSOUND STEP, SO A COMPANION CORRECTION IS OWED THERE — AND IS NOT DONE.** The sentence
      immediately above calls that doc the site of record, so correcting only this file leaves the
      falsified claim standing where the next reader meets it. `internal/imagepipeline/pipeline.go`
      **retired** the *"NEVER AGAINST A REAL COVER"* admission on this inference, states the step
      verbatim — *"A grid rendering covers is not reachable with any of those links broken, which
      is the entire strength of the evidence"* — and draws *"AND THAT CALLER HAS NOW RUN"* from it.
      **This pass is docs-only and changed no Go file. This is a pointer to work that is OWED, not
      a claim that it is done.**
      ⚠️ **THE THREE GAPS ARE UNAFFECTED EITHER WAY**, and the struck paragraph scoped them
      correctly: coverage unmeasured, §4.4.1's cold start untouched, `coverGate` never known to be
      contended. **The box stays open on those alone**, and would have stayed open on them under
      either reading.
      ⚠️ **§16 puts this and the library grid in ONE line item** — the sentence in **§16's v0.1
      entry** reading *"Library grid with "Load more" + `content-visibility` on grid rows carrying
      explicit ARIA roles (§4.5), keyset pagination, image pipeline **including the §4.4.1
      cold-start plan**"*. This item and the grid item below are two halves of that one sentence,
      not two independent lines.
      ⚠️ **That citation used to read `ARCHITECTURE.md:2649-2651` and was ~40 lines off** — the
      sentence sits at 2689-2691 on the baseline above. It is **a section reference now, not a
      corrected line number**, per the header's citation policy: a number in a file that moves
      fails invisibly and reads as checked.
      🛑 **THE KAVITA-SPECIFIC HALF IS STOPPED BY DECISION (§1) — not abandoned, and not a gap.**
      That is the cover **fetch path** against `GET /api/Image/series-cover`, and the four facts
      `kavita-cover-probe.sh` was written to answer (`REVIEW-LOG.md` LS-260).
      ⚠️ **CORRECTED 2026-08-19. This item used to read *"~~There is no probe result to carry: the
      probe was never run.~~"* — FALSE.** It was written from a repo search that found no result
      artefact, which is precisely the trap the header's **absence rule** now names: the repo is not
      a source that would have recorded the run. **The probe WAS run, 2026-08-19, by the owner
      against his own live Kavita**; he pasted the full raw output into the library-sync thread
      (04:23:04Z) and **deliberately committed nothing**, which is why the tree holds no artefact.
      **What it measured:** 5 of 5 covers `image/png`, ~165 KiB median; `ETag` on **0 of 5**,
      `Last-Modified` on **5 of 5**, and an `If-Modified-Since` re-request earned a **304** — so
      **the timestamp is the revalidation key, not the entity tag**; `primaryColor` present as a hex
      colour on **5 of 5**, **all distinct**.
      **Against LS-260's four questions that is *answered in part*, NOT *satisfied*:** it answers
      **Q3 in full** — content type, size, and the validator **fired** rather than reported, which is
      the standard LS-260 set for itself — and it meets **Q2's stated-in-advance criterion**,
      *present **and** varied*, so `USABLE` rather than `POPULATED BUT USELESS`.
      ⚠️ **Q1 — the header-vs-query auth gate, which LS-260 calls *the* gate — IS ANSWERED, AND IT
      IS THE FAIL CASE.** This file used to record it as *"~~NOT in the results summarised here~~"*.
      The owner's raw probe output, pasted in the library-sync thread 2026-08-19, carries this line
      verbatim:
      `x-api-key header only 400 / ?apiKey= query only (no header) 200 / neither 401`
      — so on `GET /api/Image/series-cover` **header-only auth FAILS and query auth SUCCEEDS.** That
      is exactly LS-260's *header-fail-query-succeed* outcome, which its own criterion marks a **fail
      with consequences**: the credential lands in the upstream's access logs, scrubbing obligations
      follow, and **no `go-vcr` fixture may be recorded that keeps it.**
      ✅ **Q4 IS ANSWERED — as *genuinely unobservable on that instance*, which is an answer rather
      than a hedge. This item used to read *"~~nobody has read the full paste on that point~~"*;
      somebody has, and `REVIEW-LOG.md`'s LS-260 closure is the record.** The probe scanned **all 151
      series on the page** — not the 5 sampled — for a null-or-empty `coverImage` and found **0**, so
      in its own words *"all 151 series on the scanned page have a coverImage, so there was no
      coverless series to ask about. Not guessed."* A **nonexistent** series id is a different
      question, measured separately and labelled so (**404**, 162 bytes), and the log refuses to
      conflate the two.
      ⚠️ **WHAT THAT LEAVES OWED, stated as an obligation rather than as a state:** whether a series
      that **exists but has no cover** answers `404` or a `200` placeholder is **unmeasured**, so
      **whether a failed cover fetch is retried or cached as permanently absent is undecided** — and
      **LS-260 is still open on Q4 alone, by its own words**: *"Q1, Q2 and Q3 are answered … Q4 is
      not, so LS-260 remains open on Q4 alone and is not closed."*
      ⚠️ **This pass was handed *"LS-260 is now fully closed"*. The log says the opposite in terms,
      and the log wins.** What is genuinely closed is the *question* Q4 asked of **this** library —
      it holds no coverless series to observe. Answering it at all needs a Kavita library that does,
      which ADR-0052 makes unlikely to be arranged, and the log calls that an acceptable place to
      leave it because the question gates a fetch path §1 stops. **This box is a pointer to LS-260's
      closure, never a second opinion about it.**
      ✅ **The freeze arc in one line, because it is the model: imposed on suspicion, justified by
      measurement, LIFTED on a drilled guard.** Recording cassettes against a live instance was
      frozen on the suspicion that a cover fetch would write a full-admin credential into a committed
      cassette; the owner's probe **measured** it (`?apiKey=` 200, header 400); and the freeze's own
      stated condition — *"every other thread was told to record nothing new until this landed"* — is
      **discharged**. `internal/vcrscrub` is in the tree (merged `36d7f71`), it is the **only**
      cassette opener, it **installs** the `BeforeSave` hook rather than offering it as an option,
      `USARR_RECORD` finally has an implementation behind the five places that documented it, and the
      **credential-in-path drill was fired in both directions** — LS-344's table measures `gitleaks`
      catching `?apiKey=<guid>` and **missing the same GUID as a bare path segment**, which is the
      shape Kavita's OPDS routes use, so the scrubber is green exactly where the gate is not.
      **The freeze outlives Kavita as a convention, and its evidence travels with it.**
      🛑 **DO NOT read the fail as newly-urgent work — nobody should now build query-auth
      scrubbing for Kavita.** That cover path is stopped by the owner's decision (§1). The finding's
      forward value is **as a template**: BookOrbit's cover auth must be asked the same question, and
      the read at BookOrbit HEAD `73b7877` (§3) found its `/api/v1` covers **header-authenticated**,
      with the HMAC-cover-token-in-the-query-string shape confined to **OPDS** — which is why an
      adapter is confined to `/api/v1`.
      The script and its stated-in-advance criterion sit at the repo root; **nothing further is owed
      against them — because §1 stops the source, not because the probe never ran.** What is *not*
      stopped is everything source-independent — the encoder, the seven-width allowlist, the cache and
      the route.
      *Authority:* §4.4, §16's v0.1 entry.

      *Done when — **ALL THREE** legs. The first two run from a clean checkout:*
      1. **A registered image route exists.** ✅ **DISCHARGED 2026-08-19** by `34a277f`, and recorded
         on the serving-half item below, which is a separate line and is now closed.
         `grep -nE 'mux\.Handle(Func)?\("[A-Z]+ /(api/v1/)?(img|image|cover)' internal/httpapi/server.go`
         returns `mux.Handle("GET /img/{key}", …)`. ⚠️ **This leg used to read "Fired on the
         baseline tree: exit 1, no output — RED today", and that is no longer the tree.**
      2. **A non-test writer stores a REAL format, not NULL.** ✅ **DISCHARGED 2026-08-19** by
         `7e5934d`. `grep -rn 'INSERT INTO image_asset' --include=*.go internal/ cmd/ | grep -v
         _test.go` returns `internal/store/imagewrite.go:296`, and that same file **calls**
         `ValidImageFormat` — in `PosterAsset.validate`, on the value actually being stored, not
         merely mentioning it in a comment. ⚠️ **This leg used to read *"~~RE-FIRED at the baseline
         above: exit 1 on the first half — STILL RED … Every file in the tree matching `INSERT INTO
         image_asset` is a `_test.go`~~"*, and that is no longer the tree.**
      3. **Bytes actually come back.** Against a running instance,
         `curl -sS -o /dev/null -w '%{http_code} %{content_type} %{size_download}\n' '<base>/img/<key>?w=342'`
         answers `200`, an `image/*` content type and a **non-zero** size. (This container has no
         `sqlite3` CLI, so this leg is deliberately a request rather than a query.)
         🔴 **STILL RED — AND NOW RED FOR ITS OWN REASON, WHICH IS THE POINT OF THE SPLIT.** It used
         to be red *for leg 2's reason*: there was no `<key>` to put in the URL because nothing wrote
         a row. **That blocker is gone and the leg did not go green with it.** Nothing has run the
         pipeline against a real service — `internal/imagepipeline`'s own package doc says so — so
         no install has a rendered cover to request, and **this leg is unfired rather than failed**:
         it needs a host with a BookOrbit and a completed `usarr import`, which the agent container
         is not. **It stays open until somebody fires it and records what came back.**
         ⚠️ **A green `make check` does not touch this leg and must not be read as touching it.**
         Every image the suite has ever put through the pipeline was fabricated by the suite.

      ✅ **SO THE THREE-LEG SPLIT NOW READS TWO GREEN, ONE RED — AND IT HAS NOW EARNED THE SPLIT
      TWICE.** Leg 1 went green on a commit that **fetches nothing**. Leg 2 then went green on a
      commit that fetches, decodes and renders — but **still on fabricated bytes only**. Had this
      item carried a single done-check, either commit would have closed it. **Leg 3 is the one that
      cannot be satisfied by writing code**, and it is what holds this item open.

      **The clause that stops this being weakened back:** the three legs exist because **a writer
      that fetches nothing, decodes nothing and serves nothing satisfies a bare SQL grep exactly** —
      a colour-only or state-only `INSERT` is indistinguishable from a working pipeline to leg 2
      alone, and legs 1 and 3 are what refuse it.
      ⚠️ **THIS IS THE SECOND FALSELY-GREENABLE CHECK ON THIS ONE ITEM, AND RECORDING THAT IS THE
      POINT.** The **first** read `grep -rln image_asset --include=*.go internal/ cmd/` and matched
      five files — three comment-only mentions in `internal/ssrf` (`policy.go`, `ssrf.go`,
      `redact.go`) and two tests. Its replacement,
      `grep -rn 'INSERT INTO image_asset' … outside _test.go`, was **also** falsely greenable, for
      the reason in the clause above. Two misses on one line is a pattern, not luck: **a done-check
      for a pipeline has to name the pipeline's OUTPUT, never one of its INSERTs.**

      **SO WHAT IS ACTUALLY STILL OWED ON THIS ITEM. Four things, and §4.4.1 is three of them.**
      Fired against the tree at the baseline above.
      1. **THE FIRST REAL RUN — leg 3, and it is the only one that cannot be closed by writing
         code.** Covered above; it needs a host with a BookOrbit and a completed `usarr import`.
      2. **§4.4.1's COLD START IS UNBUILT IN FULL, and the columns for it have been in the schema
         since `00005`.** `image_asset.thumbhash` and `image_asset.dominant_color` are declared at
         `00005_library_sync.sql`'s `image_asset` block — the `dominant_color` line carries the
         comment *"available BEFORE thumbhash; see ARCHITECTURE §4.4.1"* — and **nothing writes
         either.** `PutPosterAsset`'s `INSERT` names ten columns (`source_url`, `origin_class`,
         `origin_service_instance_id`, `role`, `width`, `height`, `cache_key`, `format`,
         `fetched_at`, `state`) and neither of those two is among them. Outside the migration and
         `internal/db/testdata/schema.sql`, the only mention of either token anywhere in the tree is
         `internal/store/imageassets.go`'s comment saying they are deliberately **absent from the
         serving read**. So, leg by leg against §4.4.1's four rules: **rule 1 (viewport-prioritised
         fetching)** — no priority queue and no client hint endpoint; `fetchCovers` walks the
         import's own item order behind a flat permit gate. **Rule 2 (smallest size first)** —
         `renderAll` encodes every allowlisted width in one pass, so there is no 92px-first stage to
         hang a ThumbHash off. **Rule 3 (`dominant_color`, and the contrast rule)** — unwritten, and
         with it the assertion §4.4.1 says in terms is **owed to `make check`**: *"a test that
         recomputes the ratio for every `(dominant_color, foreground)` pair the pipeline emits and
         fails below 4.5:1."* **Rule 4 (progressive rendering)** is the grid's half and is not this
         item's to close.
      3. **REVALIDATION AND EXPIRY HAVE COLUMNS AND NO WRITER.** `etag`, `last_modified` and
         `expires_at` are all on `image_asset` in `00005`, and `grep` over non-test Go finds only
         `imageassets.go`'s comment — no `If-None-Match`, no `If-Modified-Since`, no sweep. ⚠️ **The
         cover pass does not need them to be correct today** — `store.PosteredItems` makes it skip
         any work that already carries a poster, so a re-import re-fetches nothing — **but that is
         *never refresh* rather than *revalidate*, and a cover that changes upstream is never
         picked up.** `ix_img_state(state, expires_at)` therefore still has no reader, which
         `00005` already flags against itself.
      4. **THE TINT, which is the design decision below and is unbuilt.** `store.PosterAsset` carries
         no colour field of any kind.
      🔍 **Inference, labelled: one further gap that is a shape question rather than a missing
      line.** The pass's only production trigger is `FullImport` — `grep -n fetchCovers
      internal/libsync/importer.go` finds one call site, in phase D. There is no per-work trigger and
      no standalone backfill command, so on an install whose catalogue was imported before `c4a3277`
      the way to get artwork is to **re-run the import**, which `PosteredItems` makes cheap for the
      works that already have it. Whether that is sufficient or whether a backfill deserves its own
      entry point is **not a question this file should answer for the owner** — `internal/imagepipeline`'s
      package doc already names the per-work trigger as a shape it deliberately left to its caller.

      **WHAT THE FIRST WRITER OWED: THREE OBLIGATIONS AND ONE DESIGN DECISION — AND THE WRITER
      HAS SINCE ARRIVED AND DISCHARGED TWO OF THEM.** ⚠️ **This block used to open *"~~WHAT THE FIRST
      WRITER OWES … Every one was verified against the tree at the baseline above~~"*, in the future
      tense throughout, because no writer existed. `7e5934d` is that writer.** The list is kept whole
      rather than pruned, because **which obligations a writer met, and how, is the thing a second
      writer needs** — the tint pass and any second catalogue source will each write this table
      again. Every verdict below was re-fired against the tree at the baseline above.

      **Obligation 1 — the format vocabulary. ✅ DISCHARGED, AND BY A CALL RATHER THAN BY A
      MENTION.** `PosterAsset.validate` (`internal/store/imagewrite.go`) **calls**
      `ValidImageFormat` on the value being stored and refuses the row with `ErrInvalidImageAsset`,
      and the file's own header says why the distinction matters: the lint below *"can only see the
      reference, so the reference alone would satisfy a test and not the rule."* The obligation and
      its mechanism are kept here because **the mechanism is what binds the NEXT writer**, and it is
      no longer vacuous.
      - **Any future `image_asset` writer must reference `store.ValidImageFormat` (or
        `store.ImageFormatJPEG`), or `make check` goes RED.**
        `TestImageWritesValidateTheFormatVocabulary` (`internal/store/imagelint_test.go`) is an AST
        walk over non-test code that matches `INSERT` / `INSERT OR IGNORE` / `REPLACE` / `UPDATE`
        against `image_asset`, including quoted, backticked and `main.`-qualified spellings, and it
        **fires its own matcher against known strings before trusting it**. ⚠️ **It used to be
        *"~~vacuous today because no writer exists~~"*; it is not vacuous any more** — it has a real
        writer under it, and it still flips for the next one, **including a writer that stores only
        NULL.** [ADR-0050](./DECISIONS.md#adr-0050) and
        `internal/db/migrations/00008_image_asset_format.sql` both name it as the thing that keeps
        ADR-0039's never-written validator from repeating.

      **Obligation 2 — the wire reaches more screens than it looks like it does.**
      - **`store.RecentWork` reaches TWO registered endpoints, not one** — `GET
        /api/v1/library/recent` (`handleRecentWorks`) and `GET /api/v1/library`
        (`handleBrowseWorks`), which share `recentWorkResponse` and `toRecentWorkResponse` in
        `internal/httpapi/library.go`. **So a colour field added to `RecentWork` lands on the
        library grid as well as on Home's recently-added table — and the grid is the screen tinted
        tiles are for.** That is a property of the shape, not an accident.
        ⚠️ **It is NOT three, and the tree is explicit about why.** `/api/v1/search` returns
        `store.SearchHit` through its own allowlist in `internal/httpapi/librarysearch.go`;
        `internal/store/searchlibrary.go`'s doc comment says it in terms — *"THE FIELDS ARE
        RecentWork'S FIELDS, ON PURPOSE … Nothing is shared in the type system yet"*. Giving search
        the same field is **a third, separate edit**, not a consequence of the first two.
        ⚠️ **This was handed to this pass as *"THREE endpoints"*. The tree says two**, and the tree
        wins: two registered handlers share the row type today, and search is the separate edit
        above. The point being made survives the correction intact — **the colour field still lands
        on the library grid, which is the screen tinted tiles are for**; only the count was wrong.

      **Obligation 3 — REJECT A `source_url` THAT STILL CARRIES A CREDENTIAL. ✅ DISCHARGED
      2026-08-19 by `7e5934d`. ⚠️ IT SPENT A WHILE DOCUMENTED AS SHIPPED WHEN IT WAS NOT, WHICH IS
      WHY IT IS ON THIS LIST AND WHY THE WHOLE ARC IS KEPT.**
      - **It exists now, as `checkImageSourceURL` and `ErrCredentialInSourceURL`
        (`internal/store/imagewrite.go`)**, run from `PosterAsset.validate` **before** anything
        reaches a prepared statement's arguments, and it consults `internal/ssrf`'s one deny-list
        through `ssrf.IsCredentialParam` rather than copying the names. It refuses `userinfo` in the
        URL as well as a credential query parameter.
        ⚠️ **IT REFUSES; IT DOES NOT STRIP — WHICH IS THE OPPOSITE OF WHAT THIS ITEM ASKED FOR, AND
        DELIBERATELY SO.** The bullet below still reads *"strip credential parameters"*, kept
        visible because the divergence is the interesting part. `imagewrite.go`'s own header argues
        the case: *"Stripping silently would store a correct row and leave the caller still
        constructing credentialed URLs — into log lines, into an HTTP cache key, into the next table
        that has no such check. A refusal surfaces it once, at the moment it is introduced."*
        **`security.md` §5 asked for an assertion, and an assertion is what landed.**
      - ⚠️ **This bullet used to read *"~~It does not exist. There is no image pipeline, and
        `source_url` appears in non-test Go exactly once, in a comment — `internal/ssrf/redact.go:14`
        …~~"*, and both halves are now false** — there is a pipeline, and `SourceURL` is a field on
        `store.PosterAsset`.
      - **`docs/reference/security.md` §5 and `docs/reference/schema.md` §12 BOTH ASSERTED IT IN THE
        PRESENT TENSE** — *"an ingest assertion rejects writing a `source_url` …"* — so **a reader
        who checked `security.md` yesterday would have believed UsArr already had this guard.** It
        has none. That is the whole reason the obligation belongs on the roadmap and not only in a
        reference file: this is where someone looks to find out what is owed.
      - **Cite the corrections, never the original claim — and they are TWO commits, not one.**
        `fc2b7c4` (*"the credential deny-list has one home, and it is not these files"*) removed a
        **contradictory fourth deny-list** from both sites — `api_key`, `apikey`, `token`, `key=` —
        and pointed them at **`internal/ssrf/redact.go`'s `credentialParams`**, whose own header
        calls it *"the ONE deny-list"*. ⚠️ **`fc2b7c4` fixed the NAMES and left the TENSE**; this
        pass read it and said so. **`2ce8ed9`** (*"the credential-free `source_url` rule is owed,
        not implemented"*) is what fixed the tense, landing while this pass was running — §5 now
        reads *"no row may be written whose `source_url` still carries a credential parameter — the
        ingest path that writes these rows **owes** that assertion"*, and §12 the same. **Both
        reference files are correct as of the baseline above; the obligation is still unmet in
        code.**
      - **The obligation as it was written, kept verbatim so the divergence above is legible:**
        *"strip credential parameters before the row is written, with the names taken from
        `credentialParams` and **never restated locally** — and note `cache_key =
        sha256(source_url)[:16]`, so getting this wrong does not merely leak, it makes a
        provider-key rotation silently invalidate the whole image cache."* **The deny-list clause
        held exactly** — `ssrf.IsCredentialParam` is consulted, nothing is restated — and the
        `cache_key` consequence is quoted back in `ErrCredentialInSourceURL`'s own header. **Only
        *strip* became *refuse*.**
      - **It binds the NEXT writer too, and by construction rather than by this paragraph:**
        `PutPosterAsset` is the only non-test path to an `image_asset` row, so a second catalogue
        source that goes through it inherits the assertion. One that does not go through it inherits
        nothing — which is what makes `TestImageWritesValidateTheFormatVocabulary` (Obligation 1)
        the load-bearing half of the pair.

      **The design decision — and it is the tint.**
      ⚠️ **The *"zero-fetch tinted placeholder as a real first slice"* framing DIED WITH THE KAVITA
      SUNSET**, and the reason is written here so nobody reconstructs it from the old text. It was
      zero-fetch **only** because Kavita hands out a precomputed per-series colour —
      `SeriesDto.primaryColor` / `secondaryColor`, declared on four DTOs in
      `internal/kavita/resources.go` and forwarded by `internal/kavita/redact.go`; **nothing in
      `internal/libsync` ever read either, so the slice was unbuilt, not half-built.**
      **VERIFIED at BookOrbit HEAD `73b7877`: BookOrbit exposes NO precomputed cover colour.** No
      colour column on books, book metadata, series or any cover table — the only colour anywhere in
      its Drizzle schema is `annotations.color`, a highlight colour. `sharp` is its only image
      library and its cover path merely resizes and re-encodes; `sharp`'s `stats().dominant` is
      never called, and there is no blurhash or thumbhash anywhere. Kavita is the unusual one here.
      🔍 **Inference on top of those verified facts — labelled as inference:** with no colour to
      read, a tint needs a cover **fetched and decoded**, which does not make the work *bigger* so
      much as **not independent**. This pipeline already fetches, decodes and downscales every
      cover, so **averaging a colour during a decode that is happening anyway is a small rider on
      this item — one extra field written during that decode**, not a separate slice.
      ✅ **THE PREMISE OF THAT INFERENCE IS NOW A FACT RATHER THAN A PLAN.** `7e5934d`'s decode is
      real: `renderAll` (`internal/imagepipeline/render.go`) already holds the decoded source image
      in memory to downscale it. **The rider is still unwritten** — `store.PosterAsset` has no
      colour field and `PutPosterAsset`'s `INSERT` names no colour column — so what changed is that
      the decode it was waiting on exists, not that anything of the tint was built.
      **Sync's measurement stands and is carried, not re-derived:** ~**90%** of the tinted-tile
      design is adapter-independent — the writer, the credential-free URL discipline, idempotency,
      the wire field and the guards — and **survives a backend switch untouched.** It simply lands
      *inside* the pipeline rather than ahead of it.
      ⚠️ **An option seen and NOT taken, recorded so it is not re-derived as a discovery:** compute
      the tint **in the browser** from an already-decoded `<img>`, which is exactly what BookOrbit's
      own UI does (`client/src/features/book/lib/cover-tint.ts`, canvas hue-binning, persisting
      nothing). It works, and it is **declined by default under principle 1**: it puts work on the
      render path. Taking it would need an explicit argument, not a preference.
      ⚠️ Not the same question, and already settled the other way: `REVIEW-LOG.md` **V-15 deleted**
      the averaged-colour machinery from *poster titles* — title and year sit below the tile on the
      chrome's own ground. It **narrowed rather than withdrew** the contrast rule for a **row-level
      tint**, *"where the ground is known"*, which is this.

- [x] **FALSIFIED 2026-08-19 — ~~The image pipeline's OUTPUT CODEC is undecided, and no encoder can
      be written until it is~~. It is decided, and the schema already carries the column this item
      said was missing.** The old claim is kept visible; the checks that falsified it are below.
      [ADR-0050](./DECISIONS.md#adr-0050) is **Accepted, 2026-08-19**: **stdlib JPEG is the base
      output format**, **AVIF is deferred with its seam kept** (reopening condition named), and
      *"one codec per row"* is an explicit invariant — `orig` included, so **there is no passthrough
      width**.
      ⚠️ **The claim *"`image_asset` has **no format column**"* IS FALSE ON THIS TREE.**
      `internal/db/migrations/00008_image_asset_format.sql` adds `image_asset.format` — nullable
      `TEXT`, no default, **no `CHECK`** on ADR-0039's reasoning — where **NULL deliberately means
      *"no encoded bytes exist for this row yet"***. The citation
      `00005_library_sync.sql:219-236` described the schema **before 0008** and is **dropped rather
      than corrected**, per the header's citation policy.
      ⚠️ The item's *own* earlier falsification stands and is not re-litigated: *"AVIF is not
      buildable under `CGO_ENABLED=0`"* is false — `gen2brain/avif` v0.6.0 is MIT and cgo-free.
      ⚠️ The 🔍 **recommendation this item carried** — *"name stdlib JPEG as the base format now and
      defer AVIF, keeping the seam"* — **is precisely what ADR-0050 decided.** It is no longer a
      recommendation and no longer belongs to any thread.
      🔍 **One live consequence of §1's sunset — flagged as inference, and NOT acted on here.**
      ADR-0050 names its likeliest reopening trigger as **input decode**, on the stated grounds that
      *"Kavita is v0.1's catalogue source"* and its *Save Media As* setting can emit AVIF that this
      binary cannot decode. **If Kavita stops being the source, that reasoning loses its subject.**
      Whether the trigger survives against a different backend is the ADR lane's question, not this
      file's, and nothing is re-planned around it here.
      *Authority:* [ADR-0050](./DECISIONS.md#adr-0050),
      `internal/db/migrations/00008_image_asset_format.sql`, §4.4.
      *Was done when:* an ADR named the base format **and** the schema carried whatever column it
      needed. Both happened, on the same day this item said neither had.

- [x] **FALSIFIED 2026-08-19 — ~~Library grid: "Load more", keyset pagination,
      `content-visibility` on grid rows with explicit ARIA roles~~.** All three primitives ship.
      Home's Block C walks keyset pages of 200 — `LOAD_MORE_PAGE_SIZE`
      (`web/src/lib/list.ts:434`) against `RecentWorksMaxLimit` (`internal/store/recent.go:82-83`),
      driven from `loadRecent` (`web/src/routes/+page.svelte:798`, read at `5642d16`) — with the
      stop rule tested: a short *or* empty page that still carries a cursor does not stop the walk
      (`web/src/lib/library.test.ts:453-466`). `web/src/lib/List.svelte` carries
      `content-visibility` with `role="table"`, `aria-rowcount` and `aria-rowindex`.
      ⚠️ The item read *"`GET /api/v1/library/recent` is the only catalogue read on the wire"* and
      inferred a gap from the literal 100. **That 100 is `store.SearchMaxLimit`
      (`internal/store/searchlibrary.go:109`) and binds only on `/api/v1/search`, where it is a
      documented structural refusal rather than an omission:** `SearchLibrary` fuses at most 200
      candidates (`retrievalLimit`, `internal/store/searchlibrary.go:99`) and re-ranks *the whole
      set* in Go, so there is no keyset position a cursor could name — `reference/http-api.md` §6.5
      publishes exactly that, and `web/src/lib/search.test.ts:74-82` asserts no second page exists.
      **Lifting that cap would be a store redesign contradicting a published contract, and needs an
      ADR first. It is not a missing feature.** What the falsification did surface is the grid item
      that follows.

- [x] **SHIPPED 2026-08-19 — ~~The PER-TYPE library grid, `/library/{type}` — the SCREEN~~.**
      `web/src/routes/library/[type]/+page.svelte` renders it over `GET /api/v1/library` at
      `ad821a0`, off `$lib/librarygrid`'s `LIBRARY_BROWSE_URL = '/api/v1/library'`, and the sidebar
      links all six types to it ([ADR-0053](./DECISIONS.md#adr-0053); see the facet item below).
      Not one all-types screen: navigation is §17.2's **six-value media-type enum**, and item routes
      are already `/library/{type}/{id}` — named
      in the `RecentItem.id` doc comment in `web/src/lib/library.ts`. §16 puts the grid in v0.1 **in
      the same sentence as the image pipeline** (§16's v0.1 entry), so it is that line's other half.
      The §4.5 primitives ship (see the falsified item above).
      ✅ **THE BROWSE READ SHIPPED** — `f80097f`, merged as `1c13afd`. `GET /api/v1/library` is a
      registered route, served by **`handleBrowseWorks`** (`internal/httpapi/library.go`) over
      **`store.ListWorks`** / `browseWorksSQL` (`internal/store/browse.go`). It takes `media_type`,
      `lib`, `sort`, `limit` and `cursor` (`reference/http-api.md` §7.1); an unrecognised value of
      any of them is a `400`, never a silently unfiltered page, and `?lib=` slugs resolve through
      **`resolveBrowseLibraries`**. Three orders are live in **`browseSorts`** — `added_at`,
      `sort_title`, `popularity` — with `year` refused and never substituted;
      [ADR-0051](./DECISIONS.md#adr-0051)'s 2026-08-19 amendment owns that gap.
      ⚠️ **THE FILTER PARAMETER IS `media_type`, NOT `kind`, and the two were separated on
      purpose.** `kind` is a real column with twelve members that ships on this wire **in every row
      under its own name**, beside `media_type`; the nav enum has six. Two of the six (**Ebooks**
      and **Audiobooks**) are the *same* kind split by `edition.format`. §13's budget rows and
      `reference/http-api.md` §7.2 both spell the parameter `media_type`, and ARCHITECTURE §13
      carries a dated ⚠️ recording that its own `?kind=movie` row was the same mistake.
      ⚠️ **THREE CLAIMS HERE WERE FALSIFIED BY THE BROWSE MERGE and are corrected above.** This item
      read *"BACKEND-BLOCKED"*, *"neither is a registered route"*, and that
      `internal/httpapi/server.go` *"registers `GET /api/v1/library/recent` and that is the only
      library read there"*. The mux registers **both** reads today. It also read that the interim
      `/library` table was *"not on `origin/main`"* — it is: `web/src/routes/library/+page.svelte`.
      ⚠️ **THE FOURTH FALSIFIED CLAIM, and it is this item's own: *"~~What is still missing is the
      FRONTEND … there is no `/library/{type}` route and no grid~~"*.** There is:
      `web/src/routes/library/[type]/` exists, and `web/src/routes/library/+page.svelte` — the
      unified all-types table — is **kept beside it rather than replaced**, because §17.2 keeps both
      and `reference/http-api.md` §7 calls the browse read *"a different endpoint from §1, not a
      superset of it"*.
      ⚠️ **THAT CLAUSE NAMED AN ENDPOINT UNTIL 2026-08-21 — it read *"~~the unified newest-first
      table over `/api/v1/library/recent`~~"* — AND THE BOX ALREADY CONTRADICTED ITSELF FURTHER
      DOWN, IN THIS SAME ITEM**, where it records that the screen *"moved **off**
      `GET /api/v1/library/recent` **onto** `GET /api/v1/library`"*. **No replacement endpoint is
      written here, deliberately: a fresher endpoint name is exactly the kind of status this file
      must not carry, and this one has moved once already inside this single item.** **Read
      `web/src/routes/library/+page.svelte`'s own header for what the screen reads, and
      `internal/httpapi/server.go`'s `mux.Handle` route table for what is registered.**
      ⚠️ **AND `GET /api/v1/library/recent` WAS NOT RETIRED — do not read this correction as saying
      so.** It is a live route with a live consumer: it is registered in
      `internal/httpapi/server.go` and Home reads it through `$lib/library`'s `fetchRecentPage`.
      **The fact that changed is which screen calls it, not whether it exists.**
      ⚠️ **THE `sort_title` (A–Z) DEFERRAL NOW HAS TWO BLOCKED CONSUMERS, NOT ONE, AND ITS
      SINGLE-CONSUMER PREMISE HAS EXPIRED — FLAGGED HERE, NOT RE-DECIDED.**
      [ADR-0051](./DECISIONS.md#adr-0051)'s amendment defers the index behind `sort_title` over more
      than one kind and names **the Music grid** as the consumer, with two honest fixes — *"an index
      that does not lead with `kind`, or splitting the Music grid into Artists and Albums"* — both
      §17.2's to choose. The store's condition is a **kind COUNT**,
      `sort == WorksSortTitle && len(kinds) != 1` in `browseWorksSQL`, so **the all-types scoped view
      below is blocked by the same line**, at six kinds rather than two. Which fix wins is still
      §17.2's, and nothing is chosen here.
      ⚠️ **AND THE *"interim table"* FRAMING IS RETIRED WITH IT.** This item used to call `/library`
      *"~~a SLICE of this line item and NEVER a tick~~"*, whose missing type filter and sort control
      were *"~~simply not wired yet~~"*. **`/library` is not an interim anything**: it is ADR-0028's
      Block C at full length.
      ⚠️ **THE REASON THIS BOX GAVE FOR THAT WAS A PHANTOM CITATION AND IS STRUCK.** It read
      *"~~§17.2 closes it at **one table, one order and no filters** on purpose, so wiring a filter
      onto it would contradict the section rather than finish it~~"*, and §17.2 says the opposite.
      §17.2 closes the **shape** — one table rather than one strip per type, so *"a sixth type adds
      rows to an existing list rather than a sixth region to scan"* — and of that table requires, in
      the same sentence, that *"it sorts, it filters, it Ctrl+Fs (§4.5)"*, while
      [ADR-0028](./DECISIONS.md#adr-0028) puts Block C's *"scope … from the `?lib=` chip"*.
      **No document forbids a filter on this screen**, so nothing here can be ticked or blocked on
      one being forbidden.
      ⚠️ **THE STRUCK SENTENCE'S SUPPORTING TRAP HAS EXPIRED TOO — FLAGGED, NOT RE-ARGUED.** It said
      the screen holds *"~~a keyset **prefix** of the catalogue, so a client-side control would
      present itself as operating on the library while operating on whatever has been loaded~~"*,
      and *"~~the filters and the sort control belong to the per-type grid, and that is where they
      shipped~~"*. `web/src/routes/library/+page.svelte`'s own header now records that the screen
      moved **off** `GET /api/v1/library/recent` **onto** `GET /api/v1/library` for exactly that
      reason, so the sort control and the `?lib=` scope select are applied **server-side over the
      whole table** and are on this screen today. What it still omits is `media_type` — that is what
      makes it the all-types view. **Read the route, not this box.**
      *Authority:* §16's v0.1 entry, §17.2, §17, §13's budget table, §4.5,
      `reference/http-api.md` §7.
      *Was done when:* a `/library/{type}` route exists under `web/src/routes/` and renders over
      `GET /api/v1/library`. **Both hold.**
      *What this box does NOT cover, and each is its own line below:* the covers half of §16's grid
      sentence — ⚠️ **the parenthetical here used to read *"there is still no image route"* and is
      false: `GET /img/{key}` is routed and `poster_key` is on this response; what is missing is the
      bytes** — the **all-types scoped view** a Libraries row should open, the **link** from a
      Libraries row into it, and the **`?lib=` chip** that writes scope.

- [ ] **The facet read SHIPPED. What is owed is a CONSUMER — and a sidebar predicate these counts
      CANNOT supply.**
      ⚠️ **THIS ITEM'S HEADLINE USED TO READ *"~~A facet read — until there is one, per-type hiding
      cannot come back and Home's Block A has no source~~"*, AND IT IS FALSE.**
      `GET /api/v1/library/facets` is registered in `internal/httpapi/server.go` and handled by
      `internal/httpapi/facets.go` (landed `2711926`). It answers all six of §17.2's navigation types
      with a count each, from **two local SQLite statements** with plans pinned in
      `internal/store/facets_test.go` — six equality seeks on `ix_work_kind_sort`, no sort, and a
      covering probe on `ix_edition_format` for the book split — so principle 1 holds: no \*Arr call,
      no metadata provider, no image fetch. It reads **no query parameter at all**, deliberately: no
      `?lib=`, no `?media_type=`, no paging. The wire contract is `reference/http-api.md` §8.

      ⚠️ **DO NOT TICK THIS BOX. Its *Done when* has two legs, and the one still open is the SECOND
      — which is not the leg this box was opened for.**
      - ✅ **A documented read exists**, and it is its own endpoint rather than a rider on the browse
        response.
      - ✅ **A `web/src` consumer reads it — ~~NOTHING UNDER `web/src` READS IT~~, and that clause is
        now FALSE.** `web/src/routes/+page.svelte` imports `fetchLibraryFacets`, calls it, and feeds
        `librarySummary(facets, health)` into `summaryRows`; §17.2's **Block A** is drawn off it (the
        Block A box above, content commits `51a9e68` and `da33aa7`). **This leg closed and this page
        did not move when it did.**
        ⚠️ **The 🔍 inference this leg carried was ALREADY DISCHARGED, and is struck rather than
        inherited.** It read *"~~those four comments are now stale in exactly the way this item's own
        headline was~~"* — of `web/src/lib/home.ts`, `web/src/routes/+layout.svelte`,
        `web/src/routes/library/[type]/+page.svelte` and `web/src/lib/librarygrid.test.ts`. They are
        not stale: `home.ts` now says *"`GET /api/v1/library/facets` ships and Block A is"* drawn,
        and `+layout.svelte` says *"WHICH IS NO LONGER TRUE — Home's Block A is drawn off
        `GET /api/v1/library/facets`"*. What those files still cite is `http-api.md` §7.1's *"no
        facet counts beside the chips"*, **which is a claim about CHIPS and is still true** — it is
        not this leg's subject and never was.
      - 🔴 **`TYPE_NAV` still has no predicate — UNMOVED, and this is what holds the box open.**
        `web/src/routes/+layout.svelte`'s `TYPE_NAV` is a bare `MEDIA_TYPES.map` over the six-member
        `as const` in `web/src/lib/library.ts`, with no filter, every entry resolving to
        `/library/[type]`.

      ⚠️ **AND THE SIDEBAR LEG IS NOT WAITING ON A CONSUMER. IT IS WAITING ON A DIFFERENT READ —
      READING THIS RESIDUE AS AN OVERSIGHT IS THE MISTAKE THIS PARAGRAPH EXISTS TO PREVENT.** The PM
      ruled that **[ADR-0053](./DECISIONS.md#adr-0053) is NOT to be amended**, and the ground is
      arithmetic rather than caution.
      [ADR-0059](./DECISIONS.md#adr-0059)'s counting semantics make the Ebooks/Audiobooks split an
      **ASSIGNMENT, not two independent tests**: every `book` work lands in **exactly one** bucket,
      and **a book held as both an EPUB and an M4B counts under Ebooks only** — reached by calling
      `mediaTypeOf`, the same function that renders every Type cell, rather than by restating its
      rule. A count has to do that to stay a count: overlap stops the column summing to the library,
      and *"a number that double-counts is not a smaller error than a number that under-counts; it is
      a different kind of object."*
      **ADR-0053's reopening condition asks a different question** — *does this type have content* —
      which is §17.2 rows 4–5's **independent `EXISTS` over `edition.format`**, and **presence is
      monotone and may legitimately overlap**, because two true answers about one work are not in
      conflict.
      **So hiding a type on these counts would hide a type the user HAS content in.** A library whose
      only audiobooks are second editions of ebooks reports `audiobooks: 0` while row 5 says that
      type has content — ADR-0059 states that consequence in terms and names the shape: *"the failure
      mode this project keeps refusing: hiding Audiobooks from someone who has audiobooks."* Worse
      than the version ADR-0053 already rejected, because it would arrive through **a count somebody
      did measure**, and would therefore have looked like compliance.
      ✅ **What was done instead: ADR-0053's condition is REFINED, NOT DISCHARGED.** It now names the
      existence predicate over `edition.format` — which `ix_edition_format` (migration `00009`)
      already serves — **precisely so these counts cannot be re-read as satisfying it on a later
      pass. This box is a later pass, and it does not.** The amendment was **considered and
      declined**; the nav stays all-six-always for v0.1. **What is owed is one more predicate, not
      one more argument.**

      **Where the honesty lives meanwhile, unchanged by any of the above:** the sidebar cannot carry
      it, so the empty state does. `browseEmptyState` (`web/src/lib/librarygrid.ts`) separates *no
      library-bearing service is connected* from *this type has no rows yet* from *the scope excludes
      everything*. ADR-0053 amends [ADR-0027](./DECISIONS.md#adr-0027)'s *"a type with zero items is
      not rendered anywhere"* **for the sidebar and for nothing else**, leaving it intact for Block A
      and for search groups.
      🔍 **Inference, and NARROWED by what shipped.** §13 budgets *"1 keyset page + 6 sidebar
      `COUNT(*)`"* at < 15 ms p50, so the cost was priced — and the open half of that inference,
      *"nothing decides whether the counts ride the browse response or take their own endpoint"*, is
      **settled: they took their own endpoint.** What is **not** settled is the shape of the
      *presence* read, and ADR-0059 says explicitly that the decision about what eventually hides an
      empty type **is not taken there**.

      ✅ **THE SHARED-ACTION-STRING RIDER CARRIED ON THIS ITEM IS DISCHARGED — and it was never part
      of this item's done-when, so closing it closes nothing else.** `internal/httpapi/library.go`
      used to map `store.ErrUnservableSort` to **one shared action string** naming both the music
      refusal and the missing `year` index, so a caller who asked about neither was told about both.
      `d9812da` (*"fix(httpapi): address the unservable-sort 400 to the caller who hit it"*) replaced
      it with **`browseUnservableSortAction(filter)`**, which picks per request: a non-`sort_title`
      order gets a **defensive** *added_at-or-popularity* answer rather than an unreachable-panic;
      the all-types grid is told to **add a `media_type`**, the narrowing that makes alphabetical
      work; and a multi-kind media type is told **by shape rather than by literal** — *"this media
      type covers more than one kind of work"* — so the sentence stays true if a second multi-kind
      type is ever added, **and it never names a media type the caller did not choose.** The old
      rider's own framing was that this was owed **to the wire, not to the screens**, since
      `BROWSE_AZ_UNAVAILABLE` / `BROWSE_AZ_UNAVAILABLE_MULTI_KIND` keep the option off the control
      entirely; the wire is where it was paid.

      *Authority:* `reference/http-api.md` §8 (the read that shipped) and §7.1 (no counts beside the
      chips), [ADR-0059](./DECISIONS.md#adr-0059), [ADR-0053](./DECISIONS.md#adr-0053),
      `design/DESIGN-DIRECTION.md` §8.1, §17.2, §13.
      *Done when — TWO LEGS, AND THEY HAVE COME APART. THE BOX STAYS OPEN.* The clause used to read
      *"a `web/src` consumer renders §17.2's Block A off `GET /api/v1/library/facets`, **and**
      `TYPE_NAV` grows a predicate fed by an **existence** read over `edition.format`"*, as one
      conjunction. It is split here because one leg closed and the other did not move at all, and
      ticking the box on the first would have claimed the second.
      1. ✅ **A `web/src` consumer renders §17.2's Block A off `GET /api/v1/library/facets`.**
         `+page.svelte` calls `fetchLibraryFacets` and draws the six rows (the Block A box above).
         ⚠️ **Ticked against the written criterion, not against a run**, on the same terms as that
         box: **the unfired obligation is that no facet count has been observed arriving on a
         rendered screen**, and the missing prerequisites are the absent component-render harness
         under `web` and a live catalogue source, both named in full on the Block A box.
      2. ❌ **`TYPE_NAV` grows a predicate fed by an existence read over `edition.format` — not by
         these counts.** Unmoved: still a bare `MEDIA_TYPES.map`, no filter. **The existence read it
         is waiting on does not exist either**, so this leg is waiting on a DIFFERENT READ and not on
         a consumer — the ⚠️ paragraph above is there to stop that being read as an oversight.
         **An ADR recording a different answer also closes this leg. Amending ADR-0053 to accept the
         counts does NOT, and that is the PM's ruling rather than this file's inference.**

- [x] **SHIPPED 2026-08-19 — ~~The ALL-TYPES scoped view a Libraries row opens — DECIDED, MEASURED
      ON THE WIRE, unbuilt~~.** `web/src/routes/library/+page.svelte` is it, at `d0215fb`: the screen
      was switched off `GET /api/v1/library/recent` — which parses `limit` and `cursor` and nothing
      else — onto `GET /api/v1/library`, the read that accepts `?lib=` and a `sort`. It sends **no
      `media_type`**, and its sort control is `browseSortsFor(undefined)`, which is `added_at` and
      `popularity` with `sort_title` filtered out; `browseSortNote` prints
      `BROWSE_AZ_UNAVAILABLE` beside it so the absence is stated rather than discovered. **The A–Z
      gate is `browseKindCount`** — the client's copy of the store's `len(kinds) != 1` — so it is
      derived from the kind COUNT and never from the library scope, which is the flag this item
      carried and which the implementation took.
      **The decision this box was written to protect is unchanged, and it is the kind that gets
      silently re-decided:** a
      Libraries row opens an **all-types** scoped view, **not** a per-type grid. **A library spans
      media types** — §17.8's flagship shape is one upstream library offered as **Ebooks *and*
      Audiobooks** — so picking a type on the user's behalf silently drops most of what they clicked.
      ✅ **The wire already serves it, and this is measured rather than reasoned.** `?lib=` **without**
      `media_type` is accepted and correctly scoped. On `seedLibraryCorpus`
      (`internal/httpapi/library_test.go`): the whole catalogue is **6** items; `?lib=manga` is **4**,
      spanning **two** media types (three comics and a movie); `?lib=books` is **2**; and the two are
      a **disjoint partition** of the six. Measured on the wire at `dd88a67`, and it is a **standing**
      check rather than a one-off reading — `TestBrowseEndpointScopesByLibrarySlug`
      (`internal/httpapi/library_browse_test.go`) sends `?lib=books` with no `media_type` and pins
      the titles it comes back with.
      ✅ **The STRUCTURAL reason a silently-unfiltered page cannot happen here, which is worth stating
      because it is a property rather than a promise somebody has to keep.** `media_type` is optional
      in `handleBrowseWorks`, `?lib=` resolves through `resolveBrowseLibraries`
      (`internal/httpapi/library.go`), and in `internal/store/browse.go` the library predicate is
      **spliced into the one `browseWorksSQL` template** as ADR-0051's work-driven `EXISTS`. **There
      is no separate unfiltered path** for an all-types query to fall down. Widening would take
      deleting the splice, not forgetting a branch.
      ✅ **A–Z IS A `400` ON AN ALL-TYPES QUERY, AND THE SORT CONTROL NOW *KNOWS* THAT RATHER THAN
      DISCOVERING IT AS AN ERROR.** The refusal is `sort == WorksSortTitle && len(kinds) != 1` — **a
      kind count, not a library-scope rule** — and the unfiltered case is six kinds. This item read
      that `$lib/librarygrid`'s `browseKindCount`, `browseSortsFor` and `browseSortAvailable` were
      keyed on a **required** `MediaType` with no all-types arm, and that giving them one was part of
      this item. **They have one:** `browseKindCount(undefined)` returns `ALL_TYPES_KIND_COUNT`, a
      literal `6` whose comment says why it is not `MEDIA_TYPES.length` — six kinds over six media
      types is a coincidence of digits, not an identity.
      ➡️ **THE `ErrUnservableSort` ACTION-TEXT WRINKLE HAS MOVED OFF THIS ITEM.** It is
      **server-side text**, not screen work, and it now rides on the **facet-counts item above** and
      behind it. This box no longer carries it.
      *Authority:* §17.8, §17.2, [ADR-0051](./DECISIONS.md#adr-0051), `reference/http-api.md` §7.1
      and §7.3.
      *Was done when:* a route under `web/src/routes/` renders `GET /api/v1/library` with `?lib=` and
      **no** `media_type`, and its sort control offers `added_at` and `popularity` without offering
      A–Z. **All three hold.**

- [x] **SHIPPED 2026-08-19 — ~~A path from a Libraries row INTO its scoped view — nothing links
      one~~.** `web/src/routes/libraries/+page.svelte` now carries `libraryScopeHref`, and each row's
      name is a real `<a href>` built from it: `/library?lib=<slug>`, assembled with
      `URLSearchParams` rather than a template literal, with a row whose slug did not parse rendering
      **no link at all** rather than a link to a `400`. The destination is the **all-types** scoped
      view — the item above, which landed in the same commit — and not a per-type grid, which is the
      decision that item exists to protect.
      ⚠️ **CITE `d0215fb`, NOT THE MERGE.** This was relayed as *"merged in `80be22d`"*;
      `80be22d`'s diff is four `docs/` files, and `d0215fb`
      (*"feat(web): /library becomes the all-types scoped view, and Libraries rows link into it"*) is
      where `libraryScopeHref` appears. The kind-count-derived Music A–Z note relayed alongside it is
      `BROWSE_AZ_UNAVAILABLE_MULTI_KIND` in `web/src/lib/librarygrid.ts`, from `b811616`.
      ⚠️ **THIS ITEM WAS SEQUENCED AND WRITTEN AS OPEN ABOUT FORTY MINUTES AFTER IT HAD ALREADY
      MERGED, and it is the SECOND instance today of this file being correct when written and stale
      within the hour.** The first is the sidebar claim on the facet item below, which carries the
      worked note; it is not restated here. **Point at it rather than re-learning it.**
      *Authority:* §17.8, `reference/http-api.md` §7.3.
      *Was done when:* a Libraries row is a link whose target carries `?lib=<slug>`. **It is.**

- [x] **SHIPPED 2026-08-21 — ~~The `?lib=` chip — RE-MEASURE WHAT IT STILL OWES, AND REPORT
      BEFORE BUILDING~~. THE RE-MEASURE IS WRITTEN DOWN, AND LEG 1 OF THIS BOX'S OWN *Done when* IS
      DISCHARGED IN CODE.**
      ⚠️ **CLOSING THIS BOX BUILDS NOTHING OF §8.1's CHIP, AND THE CHIP IS STILL UNBUILT.** The
      residual is written out below rather than left to close with the box, because a box that
      closes on a subset is exactly how a specification goes quietly missing.
      `design/DESIGN-DIRECTION.md` §8.1 pins it **above the nav** — *"a library is a **scope**, not a
      place — a multi-select chip pinned above the nav"* — has it render nothing at 0 or 1 library,
      and hoists it into the top bar at narrow widths because the drawer must never be the only
      statement of an active scope.
      **The read end of the loop is complete, and that part is unchanged:** `readLibraryScope` and
      `MAX_LIBRARY_SLUGS` bound a scope at 32 before anything is sent; `browseParams` **deletes** the
      parameter rather than emptying it, because an empty `?lib=` is a `400` and not "no scope"; the
      server echoes the slugs it resolved; and `web/src/routes/library/[type]/+page.svelte` renders
      *"Scoped to …"* with an address that clears it.
      ⚠️ **THIS ITEM'S OWN PREMISE IS FALSIFIED.** It read *"~~the control that WRITES scope …
      **The write end is empty:** nothing in `web/src/routes` puts a slug into `?lib=`~~"*. Something
      does: `libraryScopeHref` in `web/src/routes/libraries/+page.svelte` (the item above) authors a
      scope on every row.
      ⚠️ **AND ITS OWN MEASUREMENT WAS FALSIFIED IN TURN — STRUCK IN PLACE 2026-08-21, NOT
      OVERWRITTEN.** It read *"~~**What ships is *arriving* at a scope and *clearing* one** — fired
      at the baseline above, the only `lib` writes under `web/src/` are `browseParams`'
      `params.set('lib', …)` in `$lib/librarygrid`, which serialises a scope the screen already
      holds, and the two `params.delete('lib')` calls in `/library` and `/library/[type]`. **So a
      grid can drop a scope and cannot change one.**~~"* **There are SIX `lib` write sites under
      `web/src/`, and the sentence names three. Listed rather than counted, so the next pass can
      re-fire them one by one:** `$lib/librarygrid`'s `browseParams` — `params.set('lib', …)` ·
      `$lib/scopeselect`'s `scopeSelectSearch` — `params.set('lib', …)` · the same function's
      `params.delete('lib')` · `/library`'s `params.delete('lib')` · `/library/[type]`'s
      `params.delete('lib')` · and `web/src/routes/libraries/+page.svelte`'s `libraryScopeHref` —
      `new URLSearchParams({ lib: slug })`. The sentence fails three separate ways: **(1)**
      `scopeSelectSearch` is a **second** `set('lib', …)`, so the write is not `browseParams`'
      alone; **(2)** the same function's delete is a **third** delete, not one of two; and **(3)**
      ⚠️ **THE ONE THAT WAS NEVER TRUE:** `libraryScopeHref` writes the parameter as an object key,
      and **the sentence immediately above names that function as a writer while this one denied
      it, in the same paragraph.** (3) is error and needs no history.
      **THE INSTRUMENT IS THE DURABLE FINDING, AND IT IS WHY (3) HAPPENED.** The baseline block
      fired *"every `set('lib'` and `delete('lib')` under `web/src/`"* — and **no grep of that
      shape can structurally see a parameter written as an object key.** Re-fired 2026-08-21: that
      shape returns the five call-form sites and walks straight past `libraryScopeHref`. A
      two-shape grep was reported as an exhaustive census. **A census of a query parameter needs
      the constructor shapes too**, `new URLSearchParams({ … })` above all.
      🔍 Inference, labelled: whether **(1)** and **(2)** existed at the baseline the struck
      sentence was fired at is not established here, so those two may be staleness rather than
      error. **Only (3) is claimed as error.**
      **WHAT THE SCOPE SELECT IS: `$lib/scopeselect` plus a `<select>` in BOTH browse toolbars.**
      `scopeSelectSearch` writes the parameter and **deletes rather than empties** it; the two grid
      routes — `web/src/routes/library/+page.svelte` and `web/src/routes/library/[type]/+page.svelte`
      — each import the same five symbols, derive `scopeOptions` / `scopeValue` / `showScopeSelect`,
      and navigate with `pushState` from `onScope`. `scopeSelectWorthShowing` keeps a control that
      can do nothing off the screen at zero libraries. **A grid can now CHANGE a scope, which is
      exactly what the struck sentence said it could not do.**
      ⚠️ **AND IT IS A STRICT SUBSET OF §8.1, WHICH THE CODE SAYS ABOUT ITSELF IN THREE PLACES.**
      `$lib/scopeselect`'s header opens *"THIS IS NOT DESIGN-DIRECTION §8.1's SCOPE CHIP, AND §8.1's
      CHIP REMAINS UNBUILT"*; `web/src/routes/+layout.svelte`'s header says **THE SCOPE CHIP IS
      STILL NOT HERE**; and `scopeselect.test.ts` fails if either route's `{#if showScopeSelect}`
      block stops carrying the literal **§8.1**, *"so a reader can mistake this subset for the
      control that is still unbuilt"*. The name is `scopeSelect` throughout **precisely so a search
      for the chip does not find it**.
      **THE §8.1 RESIDUAL, WHICH THIS BOX DOES NOT DISCHARGE AND WHICH SURVIVES ITS CLOSING —
      SHELL-LEVEL, EVERY ITEM OF IT:** the chip **above the nav** · the **top-bar hoist** at narrow
      widths · **MULTI-select** across more than one slug, which neither a link nor a single-select
      can express · the popover with its keyboard behaviours and live region · scope **propagated
      across navigation** so the sidebar and every screen agree · the `scope-empty` state · and
      §8.1's unmet wire precondition, **DESIGN-DIRECTION §9.7**. **What dissolved is `arrive` and
      `clear`, and now `change` on the two browse screens. What did not dissolve is the shell.**
      🔍 Inference, labelled, and NOT decided here: the residual is now **smaller and entirely
      shell-shaped**, which is the condition §8.1 was written before. **Whether that is a narrower
      item or an amendment to §8.1 is a decision for a pass that reads §8.1 whole** — this box
      measured, and measuring is all it claims.
      ⚠️ **THE BOX'S TWO REMAINING OWN-VOICE PARAGRAPHS ARE STRUCK RATHER THAN DELETED, 2026-08-21,
      BECAUSE ONE IS DISCHARGED AND THE OTHER IS FALSIFIED.** They read: *"~~**WHAT IS OWED IS
      THEREFORE A RE-MEASURE OF §8.1, NOT A CHIP:** subtract *arrive* and *clear* from what §8.1
      specifies, and **report the remainder before building any of it.**~~"* — **DISCHARGED**; the
      remainder is the shell-level list above, and it was reported before anything was built. And:
      *"~~🔍 Inference, labelled: what looks left is **changing** a scope without returning to
      Libraries, plus §8.1's **multi-select** across more than one slug, which no link can express.
      **If that remainder mostly dissolves, the honest outcome is a narrower item or an amendment to
      §8.1** — not a chip built to a specification written before the entry point existed. Which of
      those it is, is the measurement's answer and is not pre-empted here.~~"* — **half FALSIFIED**:
      *changing* has shipped and is no longer left, while **multi-select survives untouched** and the
      narrower-item-or-amendment question is re-asked, undecided, in the paragraph above.
      *Authority:* `design/DESIGN-DIRECTION.md` §8.1, §17.2, `reference/http-api.md` §7.3.
      *Was done when:* the re-measure is written down — **either** a control in `web/src/routes`
      that sets `?lib=` to a slug the screen did not arrive with, **or** a recorded finding that
      §8.1's remainder is smaller than the chip it specifies. **Leg 1 holds:** `<select
      id="library-scope">` and `<select id="grid-scope">`, with their shared `onScope` handler, are
      in the two route files, and `scopeSelectSearch` sets `?lib=` to any library in the list.
      **Leg 2 is the residual paragraph above.** ✅ **AND THE GUARD WAS FIRED, IN BOTH DIRECTIONS,
      2026-08-21:** `cd web && pnpm vitest run scopeselect` is **31 passed**; with `onchange={onScope}`
      deleted from `/library`'s `<select>` it fails **1 of 31**, *"routes/library/+page.svelte's
      select does not write the address"*. **This tick rests on a guard that was made to fail, not
      on one inferred from its presence.**

- [x] **The COVERS / POSTER half of §16's grid line — the SERVING half.**
      **Landed 2026-08-19.** `GET /img/{key}` is registered in `internal/httpapi/server.go`
      (`images.go` serves it), and both browse responses carry `poster_key` — the
      `image_asset.cache_key` the route is keyed on — which `$lib/library`'s `posterUrl` turns into
      a URL. `reference/http-api.md` §9 is the wire contract. The done-when is discharged as
      written: a route in the mux, and a key on the browse response that resolves through it,
      proven end to end by `TestThePosterKeyOnTheBrowseResponseResolvesThroughImg`.
      What landed with it: migration `00010_image_serving_indexes.sql` (three indexes — the route
      key, and one per arm of the owning-work EXISTS), `store.LookupImageAsset` authorized against
      the owning item per `reference/security.md` §4, `Content-Type` derived from
      `image_asset.format` and never sniffed, `Cache-Control: private` as a constant no
      `origin_class` branches on, and `internal/imagecache` for the width allowlist and the on-disk
      layout.
      ⚠️ **THE FETCH HALF IS STILL OPEN and is the item above — but NOT for the reason written
      here, and the reason is where this paragraph went stale.** It used to read *"~~Nothing writes
      `image_asset`, so every `/img` request answers `not_cached` and every row's `poster_key` is
      absent on every real install~~"*; `7e5934d` and `c4a3277` falsified that, and the item above
      carries the whole correction — **this is a pointer to it, not a second opinion.** What is still
      true is the sentence after: the serving half was deliberately ahead of the bytes, and that bet
      paid — **the fetcher did land as a small piece behind it.**
      ⚠️ **`reference/http-api.md` §7.1's sentence — shipping `poster_asset_id` *"would be an id the
      client cannot turn into anything"* — IS NOW FALSE and is corrected in place**, along with the
      two Svelte route comments and `$lib/library`'s header that had copied the same explanation.
      ⚠️ **The key is the `cache_key`, not `image_asset.id`.** §4.1 and §13 both spell the route
      `/img/{cache_key}`; a row id would be an id that resolves through nothing.
      ⚠️ **`/img/public/*` is deliberately NOT registered.** §4 wants the split structural rather
      than conditional, and the structural half is that publicness is not expressible on the private
      route. Nothing produces provider artwork, and an unauthenticated route with nothing behind it
      is a hole waiting for content.
      ⚠️ **NO `image_asset` WRITER WAS INTRODUCED — TRUE OF THIS COMMIT, AND NO LONGER TRUE OF THE
      TREE.** This paragraph used to continue *"~~`store.ValidImageFormat`'s AST lint is still
      vacuous by its own `_test.go` exemption, and the credential-stripped `source_url` assertion
      (`security.md` §5) is still owed by nothing that exists~~"*. **Both obligations travelled with
      the fetch half exactly as this line said they would, and `7e5934d` discharged both** — see
      Obligations 1 and 3 on the item above. The lint is no longer vacuous and the assertion exists
      as `ErrCredentialInSourceURL`.
      *Authority:* §16's v0.1 entry, §4.4, §13's budget table, `reference/http-api.md` §9.

- [x] **A relevance score on the wire.**
      **Landed 2026-08-19.** Resolved the first of the three ways this item named — a published
      score — by [ADR-0054](./DECISIONS.md#adr-0054), which also records why the other two lost:
      server-side grouping needs `work_relation`, a table v0.3 owns and no migration creates, and
      amending §17.4 rule 2 to a fixed type order deletes the finding the rule was written from.
      `items[].score` is `store.SearchHit.Score` — the re-rank's own output, three signals
      normalised over that answer's candidate set — which the store already computed per hit and
      **discarded at the boundary**; no migration, no column and no new query.
      ⚠️ **The order is still the contract and is NOT score order**: diversity injection promotes a
      row without re-scoring it, so a client sorting by the number gets a worse list than the one it
      was handed. `reference/http-api.md` §6.2.1 is the semantics — two permitted uses, five
      forbidden ones, each with its mechanism — and the guards are the `TestSearchScore*` family in
      `internal/store` and `internal/httpapi`.
      ✅ **THE PREREQUISITE IS DISCHARGED. §17.4's GROUPED SCREEN IS NOT BUILT, AND THIS BOX DOES NOT
      CLAIM IT IS.** Both halves were checked against the tree rather than taken from the ADR.
      **On the wire:** `searchItem` in `internal/httpapi/librarysearch.go` carries
      ``Score float64 `json:"score"` `` and fills it from `store.SearchHit.Score`, unrounded.
      **Semantics documented:** `reference/http-api.md` **§6.2.1** — *"`score` — what it is, and the
      five things it must not be used for"* — states the formula, what the number is **not**, a
      seven-row comparability table, the **two** permitted uses and the **five** forbidden ones, each
      with its mechanism. That was the standing condition on this item and it is **met**.
      ⚠️ **Nothing reads the field yet, and that is the shape of what remains.**
      `web/src/lib/search.ts` declares `export type SearchItem = RecentItem`, a type with no `score`
      key, so the number crosses the wire and the client drops it. §17.4 **rule 2**'s group ordering
      and **rule 4**'s cross-media placement are both unbuilt, and rule 4 additionally waits on
      `work_relation`, which v0.3 owns and no migration creates. **Unblocked is not built.**
      *Authority:* §17.4 rule 2, `reference/http-api.md` §6.2, §6.2.1 and §6.6, §16 v0.1 entry.

- [x] **FIXED 2026-08-19 — ~~`reference/search.md` §4 never absorbed LS-191's re-rank
      divergence~~.** The fix is `f2548ea` (*"docs: search.md §4 carries the re-rank the code
      actually runs"*), and the primary half of the done-when is discharged **as written**: §4's
      signal table now marks **Jaro-Winkler on `norm_title`** as *"live"* — never *"primary"* — and a
      paragraph above it states the correction in terms, *"Jaro-Winkler is not the primary signal,
      and this table used to say it was"*, with LS-191's reason attached: JW sees `norm_title` and
      nothing else, so a primary weight buries every hit retrieved through `people`, `alt_titles` or
      `original_title` — *"a search for 'Susanna Clarke' scores JW ≈ 0 against Piranesi."* The table
      now reads **RRF ratio live and heaviest, JW live, recency live and smallest**, with the other
      three rows marked **dead** and each carrying why.
      ⚠️ **THE SECOND HALF OF THE DONE-WHEN — *"names the three live weights"* — WAS ANSWERED BY
      REFUSING IT, AND THAT IS THE FIX RATHER THAN A SHORTFALL IN IT.** §4 deliberately prints **no
      numbers**: *"The weights are in the code, and this document does not keep a second copy of
      them … `internal/store/searchlibrary.go`'s `rerankWeightRRF` / `rerankWeightJW` /
      `rerankWeightRecency` const block is authoritative; when it and any prose disagree, **it wins
      and the prose is stale**."* **A second copy of a number is a third place for it to go stale**,
      which is the failure this whole round was about — so the done-when as this file wrote it was
      the weaker instruction, and the tree took the stronger one. What §4 keeps is the **ordering**
      those weights encode — *retrieval evidence above string shape above recency* — plus the
      statement that they are **chosen, not tuned**, there being no relevance-judgement set behind
      them. `reference/http-api.md` §6.2.1 still prints the formula.
      **This item's own record of its provenance stands and is not re-fired:** it was one of two
      findings from a search-docs review round whose label `SD-08` was not in the tree, which is why
      it cited evidence rather than a finding id. **The sibling finding — the source comment citing
      `TestSearchOrderIsTheServersAndIsNotScoreOrder`, which is defined nowhere — is a SEPARATE box
      below and is NOT closed by this commit.**
      *Authority:* `REVIEW-LOG.md` LS-191 and LS-192, `reference/http-api.md` §6.2.1,
      `reference/search.md` §4.

- [ ] **A source comment cites a test that exists nowhere in the tree.** **RECORDED HERE, NOT FIXED**,
      same round and same queue as the item above. `internal/httpapi/librarysearch.go`'s store-order
      comment ends *"…`TestSearchOrderIsTheServersAndIsNotScoreOrder` holds it here"*, and
      `git grep TestSearchOrderIsTheServersAndIsNotScoreOrder` returns **that comment and nothing
      else** — no such function is defined anywhere. The guard it means is
      **`TestSearchOrderIsNotScoreOrder`** in `internal/store/searchlibrary_test.go`: a different
      name in a different package, so the citation is wrong about **where** the order is held as well
      as **what** holds it. A citation that resolves to nothing fails invisibly, which is the same
      failure mode the header's citation policy was written against.
      ⚠️ **This was briefed to this pass as a WEB-code comment. The tree says otherwise — it is Go**,
      and the brief is recorded as falsified rather than quietly adjusted. `web/src/` was checked for
      the same fault and is clean: the five `Test*` names cited from non-test sources
      (`TestListLibrariesShipsNoCredentialOrAddress`, `TestBrowseWorksUnfilteredIsBlockCsCorpus`,
      `TestUnrecognisedQueryParametersAreIgnoredNotRefused`,
      `TestBrowseEnvelopeOmitsLibOnlyWhenNoScopeWasApplied`, `TestSearchResponseKeysAreTheAllowlist`)
      all resolve to a `func`, and every `*.test.ts` filename cited resolves to a file.
      *Authority:* the tree; `CLAUDE.md`'s *verify, don't assert*.
      *Done when:* `git grep TestSearchOrderIsTheServersAndIsNotScoreOrder` finds a `func`, or the
      comment names the guard that exists.

- [x] **DONE — BookOrbit's `packages/types` IS VENDORED, WITH A DRIFT CHECK.** Landed 2026-08-19 at
      `api/specs/bookorbit-types/` (68 files, upstream `packages/types` at
      `73b7877d`, git tree `4cb990a3…`), with `api/specs/bookorbit-types.manifest` beside it.
      ⚠️ **The `docs/reference/` in this item's original heading was the INFERENCE it asked to have
      settled, and it was settled the other way** — `api/specs/` is where this tree keeps vendored
      upstream artefacts and where `SOURCES.md` registers them; `docs/reference/` holds hand-written
      Markdown and no vendored artefact. The reasoning is in `api/specs/SOURCES.md` and in
      `internal/bookorbit/vendoredtypes_test.go`'s `vendoredTypesRoot`. Guards: three offline tests
      in `make check` (tree identity, manifest currency, and a comment-blind **declaration digest**
      over the five transcribed files) plus `TestSpecDriftBookOrbitTypesStillMatchUpstream` in
      `make spec-drift`, which is network-only and **runs only when a person types it** — there is
      no CI, and the item's *"a check fails when it diverges"* is true of the check, not of any
      schedule. What none of it covers is listed under *What this does NOT cover* in
      `api/specs/SOURCES.md`. **Original item kept below for its reasoning.**

- ~~**NEW OBLIGATION — VENDOR BookOrbit's `packages/types` UNDER `docs/reference/`, WITH A DRIFT
  CHECK. Rated ABOVE ordinary hygiene, and the reason is the item.**~~ **(SUPERSEDED by the row
  above, and DE-BOXED 2026-08-21.)** ⚠️ **IT WAS A STRUCK HEADING ON AN OPEN `- [ ]`, so it counted
  as outstanding work while its own text said it was discharged** — the only such box in this file.
  **Both legs of its *Done when* are met**: the vendored copy is `api/specs/bookorbit-types/` at a
  named upstream commit, and `internal/bookorbit/vendoredtypes_test.go` plus
  `TestSpecDriftBookOrbitTypesStillMatchUpstream` are the offline pin and the network drift check.
  **The checkbox is gone; the reasoning below is kept deliberately and is why this is de-boxed
  rather than deleted.** ⚠️ **NO COUNT IS ADDED AND NONE IS STRUCK** — this file states no open
  tally anywhere, and the miscount this repairs is the one a reader forms from `- [ ]` markers.
      **Why it is not hygiene: BookOrbit is now v0.1's ONLY catalogue source (§1), so upstream drift
      is a single point of failure for the WHOLE library** — not the degradation of one source among
      several, which is what the same bug would have been while more than one adapter was in play. A
      vocabulary that moves upstream and goes unnoticed downstream mis-grades a credential or
      mis-reads a payload for everything UsArr holds.
      **The obligation:** BookOrbit's `packages/types` vendored at a **named upstream commit**, plus
      a check that fails when the vendored copy and upstream disagree — the shape
      [ADR-0046](./DECISIONS.md#adr-0046) and [ADR-0047](./DECISIONS.md#adr-0047) already set for the
      Kavita and Prowlarr specs (an offline identity pin, plus a network drift check), applied to a
      source that is TypeScript rather than an OpenAPI document. ⚠️ **BookOrbit builds its OpenAPI
      document at RUNTIME and mounts it only under `SWAGGER_ENABLED`, so there is no served spec to
      pin instead** — `internal/bookorbit/scope_test.go`'s header records that, and it is why the
      vendored source is the substitute rather than a second-best.
      **Why it is owed on top of the guard that exists — a property of that guard, not a fault in
      it.** `TestEveryBookOrbitPermissionIsClassified` and `TestPermissionVocabularyMatchesTheSource`
      both range over a list **this build carries**. An addition made upstream is invisible to them;
      it reaches this binary only as an unrecognised string, and only on a live credential that
      happens to hold it — **a guard that sees only what it happens to encounter.** Grading the
      unknown as ELEVATED makes that safe; it does not make it **observable**, and observability is
      what this obligation buys. ⚠️ **[ADR-0058](./DECISIONS.md#adr-0058) leans on that test** —
      *"`TestEveryBookOrbitPermissionIsClassified` notices a 24th permission upstream where a
      paragraph could not"* — and it also calls the grading *"a **maintenance obligation**, not a
      self-maintaining one"*. **This item is that obligation's mechanism**, and the reach the first
      quote claims is what a vendored copy plus a drift check would actually supply.
      🔍 Inference, labelled, and this item should SETTLE it rather than inherit it: **`docs/reference/`
      holds prose Markdown only today**, while the vendored upstream documents live in `api/specs/`
      (`kavita-develop.json`, `kavita-v0.9.0.2.json`, `prowlarr.json`). Which of the two is the right
      home for vendored TypeScript is a choice to make explicitly, not one to take from the brief
      that raised the obligation.
      *Authority:* §1, [ADR-0046](./DECISIONS.md#adr-0046), [ADR-0047](./DECISIONS.md#adr-0047),
      `CLAUDE.md`'s *verify, don't assert*.
      *Done when:* a vendored copy of BookOrbit's `packages/types` exists at a named upstream commit
      **and** a check fails when it diverges from upstream — offline for the identity pin, on the
      network for the drift, so `make check-offline` still runs.

- [ ] **System tags `type:`, `format:`, `source:`, `quality:`, `indexer:` with the `downloadId`
      provenance join.**
      *Authority:* §10, §16 v0.1 entry.
      *Done when:* a tag vocabulary has a writer and a filter path in `internal/`.

- [x] **`usarr key rotate`, working, on top of key versioning and AAD.**
      **Landed.** `cmd/usarr/keyrotate.go` is the subcommand: refuse under `USARR_SECRET_KEY`/
      `USARR_SECRET_KEY_FILE` → resume or generate `keys/secret.key.new` → register both keys →
      re-wrap in keyset-paginated batches, tombstones included → prove every row unwraps under the
      new key → `rename(2)` the file into place. Key ids are content-derived (**ADR-0049**), so a
      key file names its own id and an interrupted rotation is readable from the two files alone.
      *Authority:* §14, `reference/security.md` §1.5, §16 v0.1 entry.
      *Done when:* `grep -rn 'rotate' cmd/usarr/*.go` finds a subcommand, not nothing. ✅

- [x] **LS-170 — lift `httpapi.redactText` into `internal/ssrf`, and the three fixes around it.**
      **All four steps landed** (`dff0fa7`, `44b9354`, `a13bf6f`, `3fe94aa`): `ssrf.RedactText` is
      defined in `internal/ssrf/redact.go` and called from `internal/kavita`, `internal/httpapi` (via
      a one-line shim, so its 38 call sites are unchanged) and `cmd/usarr`; `last_error` is redacted
      before the row; every branch of `parseErrorBody` is redacted **and** bounded, the
      `problemDetails` branch included; and `cmd/usarr`'s slog handlers are wrapped rather than the
      three log sites hand-fixed. The step-4 guard was **fired against the unfixed code** before it
      was trusted.
      *Authority:* `REVIEW-LOG.md` LS-170 § *Applied*, `reference/security.md` §5. **No ADR** — it
      applies rules those documents already state, and closes off no alternative.
      ⚠️ `docs/reference/http-api.md:774-801` still describes this gap as open and is now stale; the
      thread that owns that file is to correct it.

- [x] **FALSIFIED 2026-08-19 — ~~The Docker image, and `VACUUM INTO` backups as a shipped path.
      `cmd/usarr/backup.go` exists; there is no `Dockerfile` anywhere in the tree.~~ THIS ITEM'S OWN
      *Done when* IS DISCHARGED, AND THE BOX IS TICKED AGAINST THAT CHECK AND AGAINST NOTHING
      WIDER.** The old text is kept visible because a reader who trusted it would go looking for a
      file that is there.
      *Authority:* §15, §16 v0.1 entry.
      *Done when:* a `Dockerfile` exists. ✅ **`deploy/Dockerfile`, from `000ac52`** (*"feat: add
      deploy/Dockerfile — distroless, non-root, static binary"*) — content commit, cited rather than
      the merge that carried it. The same commit added `.dockerignore`, a `make docker` target that
      **refuses a `BASE_IMAGE` that is not digest-pinned**, and the `README`/`DEVELOPMENT.md` text
      around them.
      ✅ **The `VACUUM INTO` half is shipped twice over.** `backupBeforeMigrate`
      (`cmd/usarr/backup.go`) has taken an automatic pre-migration snapshot since `3cde773`, and
      **`ea7c855`** (*"feat(cli): add `usarr backup` …"*) is what made it **a shipped path a person
      can invoke** — `cmd/usarr/backupcmd.go`, registered in `cmd/usarr/main.go`. Both go through the
      same `VACUUM INTO` helper in `backup.go`, which is a `VACUUM INTO` and not a `cp` for the WAL
      reason the file states.
      🛑 **THE IMAGE IS WRITTEN, NOT BUILT — AND THIS TICK MUST NOT BE READ AS *"THE IMAGE
      WORKS"*. THE PREREQUISITE IS A DOCKER DAEMON, WHICH THIS ENVIRONMENT DOES NOT HAVE.** Recorded
      here as an **outstanding obligation** rather than left to be inferred from the box:
      - **What is owed:** one successful `make docker` on a host with a daemon, and somebody
        recording what it produced. Until then the container path is **unverified**, and a `docker`
        target that has never run is indistinguishable from one that cannot.
      - **The tree already says so in four places, and they agree** — this item is a pointer to them,
        never a second opinion. `deploy/Dockerfile`'s own header carries an **HONESTY NOTE**: *"This
        file has NOT been built in the environment it was written in: the agent container carries no
        Docker daemon (docs/DEVELOPMENT.md §8), so `make docker` cannot run here … Treat a green
        build as unverified until then."* `docs/DEVELOPMENT.md` **§4**'s target table says the image
        is *"unbuilt and unverified"*; **§12**'s opening paragraph says the container path *"stays
        unverified"* and flags the README's Compose block **illustrative only**; and §12's
        known-gaps list says **`make docker` has not been made to succeed on any checkout**. The
        `Makefile`'s own `docker` recipe fails closed on a missing daemon and points at §8.
      - ⚠️ **THE SAME SHAPE AS THIS FILE'S OTHER OPEN LEG, AND WORTH NOTICING TWICE IN ONE PASS.**
        §2's image-pipeline item has a leg that **cannot be satisfied by writing code** — a first run
        against a real cover — and `internal/imagepipeline`'s package doc names
        `deploy/Dockerfile`'s written-not-built as its own comparison. **A *Done when* that a text
        editor can satisfy is the failure mode both of them found**, and this box is ticked knowing
        that its check was the weak kind.
      📉 **THE *`deploy/` HAS STALLED* OBSERVATION UNDER THIS ITEM IS FALSIFIED TOO, AND IT WAS
      FALSIFIED BY THIS ITEM'S OWN COMMIT.** It used to read: *"~~`deploy/` has not moved since
      2026-08-17. The newest commit whose diff contains a `deploy/` change is `3b951cf` … the gap is
      close to two days against an otherwise hourly tree~~"*, with the labelled inference that a
      directory sitting still *"is more likely **unallocated** than **finished**."* **`000ac52`
      landed 2026-08-19 14:38Z**, so `deploy/` now holds three files — `Dockerfile`, `update.sh`,
      `status.sh` — and on the baseline above the newest non-merge commit per area is `deploy/`
      14:38Z, `cmd/` and `web/src/` 16:49Z, `internal/` 18:10Z, `docs/` 18:51Z. **Hours, not two
      days.** ⚠️ **The inference was not wrong so much as answered:** the directory was unallocated,
      it has since been allocated, and **an observation about attention has a shelf life measured in
      hours on this tree** — which is the reason it was written as an observation and not as an item.

- [ ] **The arm64 RSS spike.** §16 calls it a day-one spike. `internal/db/spike/` exists; whether the
      arm64 measurement was taken is not readable from the tree.
      ⚠️ **DATED RIDER, 2026-08-21 — THE FIRST SENTENCE CITES A FRAMING §16 HAS ITSELF STRUCK, AND
      §16 NAMES THIS BOX WHILE DOING IT.** §16's v0.1 entry now opens *"One day-one spike, **and its
      deadline passed unmet**"*, records that the *"before the schema is written"* clause is what it
      **used to** read, and rules: *"an arm64 `make bench-rss` gates **claiming arm64 support**, not
      v0.1. v0.1 therefore owes no arm64 measurement, and this entry no longer holds one over the
      schema"* — ratified by [ADR-0072](./DECISIONS.md#adr-0072), 2026-08-20. §16 then names this
      very box: *"It is blind to every other document — `docs/ROADMAP.md`'s arm64 item still cites
      this entry for the old deadline, and is not rewritten here."* **This rider is that rewrite.**
      ⚠️ **THE GATE MOVED; THE OBLIGATION DID NOT** — ADR-0072's limit clause is *"not optional"*:
      *"the arm64 run remains owed before any claim of arm64 support. This moves the gate; it does
      not discharge the obligation, and nothing in it says arm64 works or that the x86-64 figures
      transfer."* Page size and core count both move these numbers, so an arm64 result is a **second
      row** in ADR-0001 and never a replacement. **So this box is not v0.1 work and is not cut
      either** — it gates the arm64 support claim.
      *Authority:* §13, §16 v0.1 entry **as corrected above**,
      [ADR-0072](./DECISIONS.md#adr-0072).
      *Done when:* ⚠️ **THE OLD CRITERION IS ALREADY TRUE TODAY AND THEREFORE PROVES NOTHING —
      STRUCK IN PLACE 2026-08-21.** It read *"~~a recorded measurement exists in `docs/`~~"*, and
      one does: `docs/DECISIONS.md`'s **ADR-0001, Correction, revision 3**, headed *"the memory
      numbers are measured now **(x86-64 only)**"*, carrying a nine-cell pragma table and the
      hardware line `GOOS=linux GOARCH=amd64`. **A criterion an x86-64 run already satisfies cannot
      record whether an arm64 run happened**, which is the same non-discriminating defect this file
      repairs elsewhere, in the permanently-true direction. **The replacement names the tool, the
      architecture and the destination:** `make bench-rss` has been run **on arm64 hardware** and
      its output is recorded as a **second row** beside the x86-64 row in ADR-0001 Correction 3 —
      checked by `grep -n 'GOARCH=arm64' docs/DECISIONS.md`, which must be NON-EMPTY. **Fired
      2026-08-21: EMPTY, exit 1.** ⚠️ **And the shape was proved to find such a row rather than
      assumed to** — the same grep for `GOARCH=amd64` returns Correction 3's own hardware line, so
      the emptiness is the arm64 run's absence and not a grep that cannot see hardware lines. And
      **no `arm64` hit in any of the four documents that could carry the figure holds one** —
      `grep -rn -i arm64 docs/DECISIONS.md docs/ARCHITECTURE.md docs/DEVELOPMENT.md
      docs/REVIEW-LOG.md | grep -iE 'MiB|GiB'` is **EMPTY, exit 1**, against the control that the
      same shape over `amd64` returns Correction 3's `15.7 GiB RAM` hardware line. ⚠️ **The range is
      four named files rather than `docs/` because a `docs/`-wide form would match THIS SENTENCE and
      stop discriminating** — which is the exact defect this box repairs.

- [ ] **The zero-external-providers evidence clause for BookOrbit.** §16 says v0.1 needs no TMDB
      account because the source carries its own metadata. That claim was evidenced against Radarr's
      `MovieResource` and Sonarr's `SeriesResource`, and **neither is in v0.1 any more**; the
      equivalent primary-source check against BookOrbit's payloads is **owed and undischarged in the
      repo.** ⚠️ **The source name here read *"Kavita"*** until
      [ADR-0052](./DECISIONS.md#adr-0052) moved v0.1's catalogue source; only the subject changed,
      and the obligation, its evidence standard and the *Done when* below are untouched — that
      `grep` is a dated record of a check fired against the superseded subject, and re-firing it at
      BookOrbit is part of what this box still owes. ⚠️ **That is NOT the claim that the check was
      never made** — see the header's **absence rule**: a check can have been run and never written
      up, exactly as the cover probe was. What is readable is only the repo's silence, which is the
      weaker claim, and an undischarged obligation is still owed, so this box stays open.
      *Authority:* §16 v0.1 entry, which flags it against itself.
      *Done when:* `docs/RESEARCH.md` carries the citation. Fired at the baseline above:
      `grep -n -i kavita docs/RESEARCH.md` returns 9 lines — the API surface, the auth scheme, the
      `sortByLastModified` finding, and *"identifier matching is a paid subscription feature"* — and
      **none of them evidences this clause** the way `MovieResource` and `SeriesResource` did.

- [ ] **THE SHELL RENDERS NO `System` ENTRY AND EVERY DESIGN SOURCE SPECIFIES ONE — AND §16 ASSIGNS
      THE SCREEN TO NO MILESTONE. The §16 read is the finding; the sidebar is only how it was
      noticed.** ⚠️ **This is NOT a claim that a `System` screen is owed in v0.1.** It is the claim
      that nothing in the repo says **which milestone owns it**, while three documents draw it.
      **What the shell renders**, read as an expression in `web/src/routes/+layout.svelte`'s
      `NAV_GROUPS` and mirrored in its `TITLES` map: `Home` · the six `TYPE_NAV` types · `Library` ·
      `Search` · `Requests` · `Services` · `Libraries` · `Settings`. **Seven fixed entries, and
      `System` is not among them** — `grep -rn "'System'" web/src/` is **EMPTY, exit 1**, against
      the control that `grep -n "'Settings'" web/src/routes/+layout.svelte` returns **two** hits in
      that same file, so the shape can find a nav label and the emptiness is `System`'s absence.
      **What the design sources say**, all three: `docs/design/mockups/index.html` carries a
      `System` row and calls the set *"Seven fixed entries, against a stated budget of eight"*;
      `docs/design/DESIGN-DIRECTION.md` §8.1 writes *"`Settings` and `System` always the last two
      entries, in that order"*, draws `System` in its ASCII sidebar and counts it in its row budget;
      and `ARCHITECTURE.md` §17 says the sidebar *"is already committed to Home · Search · Requests
      · Services · Settings · System"*.
      ⚠️ **THE `Library` / `Libraries` HALF IS A DIFFERENT CASE AND IS NOT AN OPEN DEFECT** —
      checked, so this box is not read as covering it. `Library` is the all-types route;
      `+layout.svelte` records the collision with `Libraries` **and accepts it** — *"It is accepted
      rather than resolved"* — and `routes/library/+page.svelte` records that *"§17 NAMES NO
      `/library` ROUTE … a gap in §17's coverage rather than a verdict against the screen."* **Two
      documented deviations. `System` is the undocumented one.**
      ⚠️ **AND §17's OWN INVENTORY DISAGREES WITH ITSELF — FLAGGED, NOT RE-DECIDED HERE.** The
      committed-list sentence omits `Libraries`, while §17.3 rules *"Services and Libraries are
      top-level screens and are not also items inside a Settings navigation"*. Whoever answers §16
      answers that too.
      ⚠️ **NO GATE CAN SEE THIS.** `docs/design/check.mjs` reads `tokens.css`, the mockup assets and
      §17's copy corpus; **nothing under `web/src` is in its file list** — the one mention of that
      path is a comment describing what would change *"When this check is later pointed at
      web/src/\*\* the import arm carries the rule instead"*, a plan and not a read. And
      `make design` is required by no document.
      *Authority:* ⚠️ **§16 IS SILENT, AND THAT SILENCE IS THE ENTRY.** §16 runs from
      `## 16. Roadmap` to `## 17. Screens`, and a `grep` for `System` across those 895 lines returns
      **one** hit — *"System tags `type:`, `format:`, `source:`, `quality:`, `indexer:`"*, the
      **search tag vocabulary, not a screen**; `sidebar` returns none. So the screen is
      **unassigned in §16**, which by §16's own rule — *"A milestone label is scope, not status: it
      says which milestone owns a thing, never that the thing exists"* — means **nobody may infer a
      milestone for it** from this file or from the design documents, and **this box infers none.**
      *Done when:* **two legs, and leg 1 is the decision this box actually asks for.**
      1. §16 names the milestone that owns a `System` screen — **or** the design sources stop
         drawing one. Runnable, and it discriminates:
         `awk '/^## 16\. Roadmap/,/^## 17\./' docs/ARCHITECTURE.md | grep -n 'System' |
         grep -v 'System tags'` must be **NON-EMPTY** for the first arm. **Fired 2026-08-21:
         EMPTY, exit 1.** ⚠️ **And the range and the shape were both proved rather than assumed** —
         the same `awk` emits **895** lines bounded by `## 16. Roadmap` and `## 17. Screens`, and
         the same `grep` **without** the `System tags` filter returns a hit, so an empty result is
         §16's silence and not an awk range that selected nothing. For the second arm,
         `grep -c 'System' docs/design/mockups/index.html` and DESIGN-DIRECTION §8.1's budget line
         both drop to zero; **fired 2026-08-21: the mockup grep returns 3, and §8.1's budget line
         still reads *"Home, Search, Requests, Services, Settings, System = 6"*.**
      2. **The running leg, and it applies ONLY if §16 assigns the screen to v0.1 and the entry is
         built:** the shell's nav guard covers it the way it already covers `/library` —
         `librarygrid.test.ts` and `libraryscreen.test.ts` read `routes/+layout.svelte?raw` and fail
         by name when an entry goes missing, so a `/system` entry gets the same guard.
         ⚠️ **THAT MECHANISM WAS FIRED RATHER THAN ASSERTED, 2026-08-21:** with
         `{ id: '/library', label: 'Library', … }` deleted from `NAV_GROUPS`,
         `cd web && pnpm vitest run librarygrid libraryscreen` fails **2 of 86**, on
         *"application links to the screen"* / `toContain("id: '/library'")`; restored, it is 86/86.
         **So the guard exists and bites — what it does not yet cover is `System`, because there is
         no entry to guard.** **If §16 assigns it past v0.1, leg 2 does not apply and this box
         closes on leg 1 alone** — say which, on the tick.

**Already discharged, listed so nobody re-opens them:** the Kavita `LastChapterAdded` watermark probe
(ran 2026-08-17 against the owner's live instance and passed — [ADR-0035](./DECISIONS.md#adr-0035)
§2a); the three Kavita subtype tables (`00006_kavita_subtypes.sql`); `work_credit`
(`00007_work_credit.sql`, [ADR-0044](./DECISIONS.md#adr-0044)).

---

## 3. Blocked and sequenced

| Item | Blocked on / sequenced behind |
|---|---|
| ~~Wiring Kavita's `PluginVersion`, or any second Kavita endpoint taking a credential in a query or path~~ | **UNBLOCKED.** All four LS-170 steps landed (`REVIEW-LOG.md` LS-170 § *Applied*), so the ordering constraint that gated this is discharged. `PluginVersion` remains unwired: nothing calls it, and whether to wire it is a separate decision that LS-170 no longer gates. |
| A second catalogue adapter (Navidrome, then Audiobookshelf, then Komga) | **v0.1's BookOrbit adapter, landed and run against the owner's real library.** ⚠️ **This row used to read *"~~THE SEQUENCE IS NOW CONTRADICTED BY AN OWNER DECISION AND CANNOT BE FIXED FROM THIS FILE~~"*, on the ground that §16.1 still called Kavita v0.1's source and an ADR was owed. Both are done.** [ADR-0052](./DECISIONS.md#adr-0052) landed, and §16.1 now records that Kavita left **v0.1** as well as the table, that the three entries **shift up by one without reordering** — no source refused, no order changed between them — and that the table is therefore *"the sequence after v0.1's own source"*. **The gate is re-pointed, not absent**, and the rule it enforces never moved: **one source, proven on real data, before a second adapter** (§16.0, §16.1, [ADR-0036](./DECISIONS.md#adr-0036)). |
| **The Kavita adapter code itself — RE-SEQUENCED, NOT CUT** | **Nothing.** It **STAYS IN THE TREE**: `internal/kavita`, `internal/libsync`'s Kavita path, `00006_kavita_subtypes.sql` and the recorded fixtures all remain, because **other people run Kavita** and principle 3 (*pluggable by default*) is the reason the adapter exists at all. **What the sunset stops is INVESTMENT, not the code** — no deletion, no deprecation notice, no migration. Read any Kavita item in §2 as *"unfunded, still standing"*, never as *"dead"*. |
| **Re-keying the design mockups' v0.1 DATA off BookOrbit** — a real design change, and a different thing from the label swap | **The owner's mixed-library answer, and the two ADRs the sync lane has pre-allocated for its structural findings.** ⚠️ **Both gates are relayed from other threads and are NOT readable in this tree** — recorded as sequencing, not as status. **The label swap already landed and is not this**: `a1995f9` moved the install switcher on all five screens to *"v0.1: BookOrbit, Prowlarr"*, while `mockups/README.md`'s v0.1 figures were **deliberately left un-re-keyed** — they are arithmetically derived from the Kavita-era install, and that section now **states its provenance where a reader meets the numbers** rather than letting a BookOrbit label sit over Kavita-shaped arithmetic. Re-deriving them changes what the drawings assert about a real install, which is why it is a decision rather than a rename. ⚠️ **[ADR-0052](./DECISIONS.md#adr-0052) marks its own mockup re-draw DISCHARGED by that commit** (`cad0563`), and that mark is about **the rendered label**, not the figures under it — reading it as covering both is the mistake this row exists to prevent. ⚠️ **No ADR number is cited: none is allocated** — and **none is to be guessed from a maximum written here**, because reading one out of *this* file is exactly what mis-allocated an ADR once already (see the baseline block). [`DECISIONS.md`](./DECISIONS.md) is authoritative for the next free number. ⚠️ **The design thread is CLOSED, so this has NO OWNER at slotting time**, and naming one is part of slotting it: **a closed thread's sections do not pass to whoever next touches them.** |
| The minimal write path — `monitor`, `unmonitor`, `delete`, `add`, the queue worker and its settlement loop | **v0.2**, with the first \*Arr adapter ([ADR-0042](./DECISIONS.md#adr-0042), [ADR-0045](./DECISIONS.md#adr-0045)). `write_queue` stays in the schema with **no writer for the whole of v0.1** — that is the seam, and it costs no migration ([ADR-0039](./DECISIONS.md#adr-0039)). |
| ~~[ADR-0039](./DECISIONS.md#adr-0039)'s outstanding Go `state`-vocabulary declaration and validation~~ — **the DECLARATION landed; what is sequenced is now only its USE** | **The validator EXISTS**: `internal/store/writequeue.go` declares the six states and `ValidWriteQueueState`, landed `007e58e` (content commit, not the merge). ⚠️ **BUT *"has a non-test caller"* AND *"is in use in production"* DIVERGE HERE, AND THE DISTINCTION IS THE WHOLE ENTRY.** Its only caller outside `_test.go` is `internal/db/spike/fixture.go`, whose first line is `//go:build bench` — **a bench fixture, not a runtime path**; `internal/httpapi/grabs.go` names it in a comment only. A grep for a non-test caller therefore comes back green over a symbol **no shipped binary ever calls**. **The durable claim, which does not go stale either way: the first production `write_queue` writer cannot be written without validating** — `writequeuelint_test.go` is an AST walk that fires `make check` RED on any non-test `INSERT` / `REPLACE` / `UPDATE` against `write_queue` in a file that does not reference the vocabulary, and it re-measures its own matcher against the five spellings that walked past `imagelint_test.go`'s first version. That first writer is still **v0.2's**, and `write_queue` still has **no** non-test writer. |
| The minimal match-correction UI — the remedy for the badge in §2 | **v0.2** ([ADR-0043](./DECISIONS.md#adr-0043), [ADR-0045](./DECISIONS.md#adr-0045)). v0.1 ships the defect's badge without its remedy for a whole milestone, and §16 states that cost rather than burying it. |
| A request destination on a library binding | A service that advertises `Add` under §8.3's capability filter. **No service v0.1 connects does** — Prowlarr's grab path posts to Prowlarr's own download client — so §17.8 drops the column for v0.1. It returns with Sonarr and Radarr at v0.2. |
| The queue-state column on Requests' `Recent grabs` block | The first `write_queue` writer — a v0.2 addition, not a v0.1 gap. |
| Knowing whether an *already connected* source covers a media type | One capability array on the health row, derived at ingest. **Build neither it nor §8.3's `Caps.MediaKinds` now** — the seam is [`FUTURE.md`](./FUTURE.md) §20. Naming *which source will populate a type* is unblocked and is a constant derived from §16. |

### v0.2 is settled — not an open question

**Decided 2026-08-17, and closed.** [ADR-0045](./DECISIONS.md#adr-0045) (Accepted, owner-delegated)
slots the commitments [ADR-0042](./DECISIONS.md#adr-0042) and [ADR-0043](./DECISIONS.md#adr-0043) each
left without a milestone — **the Sonarr and Radarr adapters, the minimal write path, and the minimal
match-correction UI** — into **v0.2**. ADR-0045 counts them as three commitments; they are four work
items, because Sonarr and Radarr are two adapters.

*Authority:* ADR-0045 and §16's v0.2 entry, which carries it. Read §16 for what v0.2 now contains and
in what order; it is not restated here. **No review should report any of these as awaiting a milestone
decision — that question is not open.**

### 🔍 Sequencing recommendation — a RECOMMENDATION, not a decision

**No ADR backs this. §16 does not say it. Nothing is planned around it.** It is inference from facts
already on record, offered to whoever picks v0.2 up.

Across the v0.2 window, take the **minimal match-correction UI** and the **Navidrome adapter** first
— both run against services the owner actually operates, so both can be **proven on real data**, which
is the rule ADR-0036 set and [ADR-0041](./DECISIONS.md#adr-0041) clause 2 kept: *"prove the replica
thesis on real data, on one source, before a second adapter is written"*. **Sonarr, Radarr and the
write path cannot be proven on his stack at all** — §16 records that the owner runs neither Sonarr nor
Radarr. Two lines already point the same way: §16 says of the correction UI that *"it is the part of
this milestone that can land first, and it should"*, and §16.1 puts **Navidrome at #1** in the
post-v0.1 catalogue sequence, *"numbered by order, not by version"*, with *"Navidrome must precede
v0.4"* as its only version pin.

Two caveats this does not paper over: **Navidrome is not a member of v0.2** — §16 pins no catalogue
source to it and has #1 landing *"before or alongside"* — and Navidrome is **sequenced behind v0.1's
own source running on a real library**, per the table above.
⚠️ **THE SECOND CAVEAT IS THE ONE §1 MOVED, AND IT IS NO LONGER VOID — IT IS RE-POINTED.** It read
*"~~sequenced behind v0.1's **Kavita** adapter running on a real library~~"*, and for a while this
file recorded the ordering premise as void because no replacement was decided. **§16.1 has since
answered it**: Navidrome sits behind **BookOrbit**, at #1 of a table that shifted up by one without
reordering. The recommendation's *reasoning* is unchanged and if anything strengthened — Navidrome
runs on the owner's own stack and can therefore be **proven on real data**, which is the rule that
outlived the source.

**Wording this into §16 or an ADR belongs to the implementation thread, not to this file.**

### BookOrbit — OWNER-DECIDED, NOW ADR-BACKED, and what the adapter owes

⚠️ **THIS SECTION USED TO BE HEADED *"Open decision — BookOrbit as a books backend"* AND IS NO LONGER
OPEN.** §1 carries the owner's words. ⚠️ **This paragraph used to add *"~~Two of that entry's
three Against findings are also falsified below~~"*, and the 2026-08-19 watermark downgrade made that
count wrong.** As it stands below: the **no-inbound-API-key** finding is **falsified**, the
**no-manga-or-comic-external-ids** finding is **narrowed**, and the **`updatedAt` watermark** finding
is **substantially UPHELD** — it is the bullet that overwrote it that was wrong. The old text is kept
visible so the reversal is legible:

> **~~Tracking, not a decision.~~** ~~Joe is standing up a BookOrbit instance and is **leaning
> toward** migrating his books backend off Kavita (2026-08-18: *"in my heart i kind of want to
> migrate to book orbit… it doesn't have a paid tier"*).~~ ⚠️ ~~**Against:** **no inbound API key** —
> headless auth needs the account password, which is worse than UsArr's Kavita credential model; an
> `updatedAt` watermark that **misses tag, genre and author edits**; and **no manga or comic external
> ids**.~~ ⚠️ ~~**The standing recommendation from that evaluation is: do NOT switch UsArr's first
> adapter off Kavita.**~~

⚠️ **THE STANDING RECOMMENDATION IN THAT LAST STRUCK LINE IS SUPERSEDED — say so explicitly, because
striking it through was never the same as reconciling it.** *"Do not switch UsArr's first adapter off
Kavita"* is **reversed by the owner's decision (§1)**, which is not this file's to re-argue. **It is
kept visible as the record of what was believed and why**, not as advice still in force.
⚠️ **This paragraph used to end *"~~an ADR is pending … until it lands, v0.1's proven source is
UNDECIDED~~"*. It landed:** [ADR-0052](./DECISIONS.md#adr-0052), and it names the successor the dead
recommendation could not. **Two of the three findings that produced that recommendation were
re-measured against BookOrbit and are FALSE** — headless auth needs no password, and comics are
covered by a shipped ComicVine provider — leaving manga and anime identifiers as the residue below.

That evaluation ran at HEAD **`4a420a04`** (2026-08-17). ⚠️ **The line *"~~§16 still assigns BookOrbit
nothing, and no ADR backs any of this~~"* is false now:** §16's v0.1 entry names BookOrbit as the one
Tier 0 adapter, and ADR-0052 backs it.

- [ ] **The BookOrbit catalogue adapter — UNGATED. [ADR-0052](./DECISIONS.md#adr-0052) discharged the
      gate this box used to carry**, and what remains is a constraint to build against rather than a
      question to answer first. ⚠️ **This line used to read *"~~GATED ON THE UNWRITTEN ADR … do not
      start it before that ADR exists~~"*.**
      🚩 **THE GATING UNKNOWN IS ANSWERED, AND BADLY — RECORD IT AS A CONSTRAINT, NOT AS A STATE.**
      This box used to read *"~~UNVERIFIED: nobody has checked whether a SERIES-level ordered read
      exists at all~~"*. Somebody has, against BookOrbit's own source at HEAD **`73b7877`**
      (re-checked at the evaluation's `4a420a04`, identical at both), and **there is no series-level
      ordered read in BookOrbit at all**:
      - the series controller exposes **exactly two** routes, `GET /series` and
        `GET /series/:seriesId/books` — **no `POST …/query` counterpart** to `POST /books/query`;
      - `SERIES_LIST_SORTS` is `name`, `bookCount`, `lastAddedAt`, `readProgress` — **no `updatedAt`**
        — and an `@IsIn` validator **rejects** one with a `400` rather than ignoring it;
      - `book_series.updated_at` **exists** as a column with `$onUpdateFn` and **is never selected**:
        the series projection is `id, name, bookCount, readCount, lastAddedAt`, and `lastAddedAt` is
        `max(books.added_at)` — an **added**-time aggregate that cannot observe an edit;
      - `collapseSeries` does not supply one indirectly. It picks a **representative row** per series
        and orders on **that row's** `updated_at` rather than a `MAX()` over the group, so **editing
        a later volume moves nothing.**

      **THE CONSTRAINT, WHICH IS THE THING TO CARRY FORWARD: change tracking on BookOrbit is
      available per BOOK, and UsArr's catalogue unit is the SERIES.** `work.kind`'s `'comic'` **is**
      the series — which is why the shipped Kavita adapter walks `POST /api/Series/all-v2` — so the
      verified ordered read, `POST /books/query` with `sort: [{field:"updatedAt"}]`, `page`/`size`
      paging and a deterministic `books.id` tiebreaker, is at the **wrong grain** for `work_comic`: a
      rename or an `expectedBookCount` change on a series moves **no** book's `updated_at`. **So the
      adapter must derive series-level change from book-level movement, or not derive it at all** —
      and §7.1a's **reconciliation-only** fallback is the documented outcome for comics and manga,
      which §16 now states as the **expected** state rather than a contingency. ⚠️ **A live probe
      cannot lift this.** It can confirm the traced write paths and cover the ones not traced; it
      **cannot produce a read that does not exist**.
      🔍 Inference, labelled: the book half is the residue that still works, so a first slice that
      delta-syncs `work_book` and reconciles `work_comic` is buildable today without waiting on
      anything — but which of the two the adapter starts with is not decided here.

      ✅ **SLICE 0 SHIPPED — AND IT IS A SLICE, NEVER A TICK. THE ADAPTER IS NOT DONE, SO THIS BOX
      STAYS OPEN.** `c324cbf` (*"feat(bookorbit): slice 0 — the client and the credential path"*,
      merged `568ddbc`) adds `internal/bookorbit`: the client, the credential path, and `scope.go`,
      which **grades all 23 members of BookOrbit's permission vocabulary** — transcribed from
      `packages/types/src/permissions.ts@73b7877d` and pinned against that transcription by
      `TestPermissionVocabularyMatchesTheSource` — with **an unrecognised permission graded
      ELEVATED**, on the stated ground that a build cannot judge what a name it has never heard of
      grants. **The verdict costs no extra requests:** it is computed from the `user.permissions`
      array the mint already returns (`TestScopeIsPopulatedByTheMintAtNoExtraCost`), exposed as
      `Client.Scope` and **logged at WARN rather than refused**, so principle 3's *"says what is
      missing and why"* survives an over-scoped credential instead of being replaced by a silent
      refusal to connect. **What slice 0 does NOT ship is any catalogue read** — no `StreamItems`, no
      `internal/libsync` path, nothing against `POST /books/query`.
      ✅ **THE SERVICE KIND IS REGISTERED — THIS PARAGRAPH'S PREMISE IS FALSIFIED.** It used to
      read *"~~SLICE 1'S OPENING MOVE … THERE IS NO `bookorbit` SERVICE KIND ANYWHERE, so no
      BookOrbit credential can be stored yet~~"*, fired on a grep that returned nothing. `e1a3837`
      (*"feat(bookorbit): register the kind, so a credential can actually be stored"*) landed it in
      **all three** registries that old text named: `serviceKinds` (`internal/httpapi/services.go`,
      where `"bookorbit"` maps to the `library` role), the per-instance client switch in
      `cmd/usarr/services.go`, and `SERVICE_KINDS` (`web/src/lib/api.ts`, now
      `['prowlarr', 'kavita', 'bookorbit']`). ⚠️ **The old text warned that one of the three is
      `web/`, ANOTHER LANE'S TERRITORY; the edit crossed it, as that warning said it would have to.**

      ✅ **SLICE 1 SHIPPED — PROSE BOOKS, END TO END. STILL NOT A TICK.** `862a0ca`
      (*"feat(bookorbit): slice 1 — one library's prose books, end to end"*) adds
      `internal/libsync/bookorbit.go`: `BookOrbitSource` with `Containers`, `StreamItems`, a `gate`
      that runs the §14 scope verdict **before any read**, and per-container `Skipped` tallies. It
      feeds **the same channel-1 importer the Kavita adapter feeds**, so it is a translation rather
      than a second write path — its own file says *"read `kavita.go` first; this then reads as a
      translation rather than an invention."*
      ⚠️ **THE COMMIT SUBJECT SAYS *"one library's"* AND THE CODE WALKS EVERY LIBRARY. Recorded
      rather than smoothed over, because this file's job is to be right about the tree.**
      `StreamItems` loops over everything `Containers` returned and walks each one's books page by
      page, so the shipped scope is **every library the credential can see**. **Cite the behaviour,
      not the subject line.**
      **What slice 1 deliberately does NOT do, in its own words:** **comics are SKIPPED AND COUNTED,
      never guessed at** — the unit-of-work question for comics is open, BookOrbit series have no
      library and a book can belong to several, and *"a wrong `work.kind` is written once at ingest
      and can never be merged away"*
      ⚠️ **THAT CLAUSE IS DISCHARGED AND THE QUOTATION NO LONGER EXISTS IN THE FILE.**
      [ADR-0068](./DECISIONS.md#adr-0068) gave comics a unit of work — one file is an ISSUE, minted
      under a series work — and the adapter maps them. The §6.4 caution that made the clause right
      still stands and is why the parent binding rests on a measurement of BookOrbit's source. **Read
      the current behaviour off `internal/libsync/bookorbit.go`, not off this paragraph**, which
      records what the slice-1 commit did.
      Also not in slice 1: **no `CreditSource` and no `FileSource`**; **no `work.year`**,
      because `store.CatalogueItem` has no `Year` field even though BookOrbit puts `publishedYear`
      right on the card; and **no channel 3b, no channel 4, no cover fetch, and no migration.**

      ✅ **THE COMPLETENESS DETECTION SHIPPED — ITS OWN ADR, AND ITS OWN MIGRATION.** `1bc400a`
      (*"feat: detect and surface a BookOrbit content-filter shortfall"*) is the code;
      [ADR-0061](./DECISIONS.md#adr-0061) (Accepted 2026-08-19) is the decision.
      **The defect it answers is UPSTREAM'S, and it is the nastiest shape a replica can have.** A
      BookOrbit account's `contentFilters` land in the books `LEFT JOIN … ON` condition rather than
      in a `WHERE`, so a filter **shorts each library's `bookCount` without dropping a library row**:
      the library appears, the counts look plausible, and a slice of the books is simply absent with
      nothing anywhere saying so. UsArr subtracts the listing's `bookCount` from
      `GET /api/v1/libraries/{id}/stats`'s `totalBooks` — **one request per library per import**,
      pinned by `TestTheStatsProbeIsMadeOncePerLibraryAndNotPerBook`.
      **The verdict is THREE-VALUED and `unverified` is a first-class member:** `complete` ·
      `shortfall` · `unverified`, with `Total = -1` rather than `0` under `unverified`, **because `0`
      is a legal total for an empty container.** ⚠️ **A boolean is rejected explicitly**, and the
      ground is that the stats route is **unguarded upstream and nobody promised it would stay that
      way** — collapsing *"no shortfall"* into *"not checked"* would report every library complete on
      the day UsArr stopped being able to tell, which is the original defect rebuilt inside its own
      fix. It **never blocks or refuses a sync** (*"a partial replica that says it is partial beats no
      replica"*), **every container gets a row** — `sync_report.kind = 'content_completeness'`,
      because an absent skip row means nothing was skipped while an absent completeness row means
      nothing was **asked** — and each row carries `covers` / `does_not_cover`, since whether **whole
      libraries** are hidden from UsArr's account is **unanswerable from a read-only account**: the
      upstream guard throws a byte-identical refusal for *"no access"* and *"no such library"*. So
      `complete` is **not** a claim that UsArr can see everything, and the row says so to anyone
      reading it out of the database.
      ⚠️ **ADR-0061's Consequences say *"No migration"* AND A MIGRATION LANDED. BOTH ARE TRUE, AND
      THE DISTINCTION IS THE POINT.** The ADR means no **schema** change — `sync_report` carries no
      `CHECK` over `kind` (migration `00005`), so the vocabulary grew without DDL and `detail` was
      already JSON. What landed is an **index**:
      **`internal/db/migrations/00011_sync_report_container_latest_index.sql`** adds
      `ix_sync_report_container_latest` on
      `(service_instance_id, kind, remote_kind, remote_id, id)` — **exactly one index and nothing
      else, the third index-only migration in a row after `00009` and `00010`.**
      **Why the read needs it:** `internal/store/libraries.go`'s `libraryCompletenessSQL` picks the
      newest verdict per `(instance, container_ref)` with a correlated subquery carrying **four
      equalities and a descending `id` pick**, and `00005`'s `ix_sync_report_instance` —
      `(service_instance_id, created_at DESC)` — serves the first predicate, none of the other three,
      and sorts on the wrong column. Measured before: `USE TEMP B-TREE FOR ORDER BY`, **per
      `library_source` row**. After: a four-column `COVERING` seek with **no row fetch and the sort
      gone**. `sync_report` is append-only and this kind writes one row per container per import, so
      **the walked-and-sorted set grew with IMPORT COUNT, forever, on a render path** — principle 1
      says every user-facing read renders from local SQLite and says nothing about that read being
      allowed to get slower every time an import runs.
      ⚠️ **Dropping `ix_sync_report_instance` is NOT the alternative, and that was measured too**:
      with it gone the subquery plans as a bare `SCAN` and the temp b-tree **disappears**, because a
      scan visits rows in rowid order — trading a bounded walk for a full scan, and **neither shape
      is what the read wants.** **Both indexes stay**; they serve different reads and neither
      subsumes the other, and `TestLibraryCompletenessPlanGuardFires` **drops this one and watches
      the sort come back**, so at least that half is executed rather than asserted.
      **It is surfaced on the Libraries screen from local SQLite** — the comparison is at import, the
      render is a `SELECT` — and **`complete` renders nothing**, which keeps that screen's standing
      invariant that nothing on it renders a positive health claim, and is why `unverified` has to be
      loud. **The Kavita adapter is untouched and serves no verdict**, which renders as an absent
      key: that is the seam, not an omission.

      ✅ **CLOSED — STRUCK 2026-08-20, AND KEPT IN PLACE ON PURPOSE. THE BLOCK BELOW WAS ALREADY
      FALSE WHEN IT WAS ATTESTED.** `BookOrbitSource` implements **both** interfaces:
      `func (s *BookOrbitSource) StreamFiles` is in `internal/libsync/bookorbitfiles.go` and
      `func (s *BookOrbitSource) StreamCredits` is in `internal/libsync/bookorbitcredits.go`, closed
      by content commit `373df3f` (*"BookOrbit credits, files and year — an audiobook renders as
      one"*). So hop 1 fails, hops 2 and 3 never start, and the user-visible wrong answer this box
      announced does not exist.
      ⚠️ **`373df3f` IS AN ANCESTOR OF `4d95d36`, THE BASELINE THIS BOX CITES AS ITS OWN
      EVIDENCE** — 13:47Z against 18:55Z on 2026-08-19. **A re-derivation pass read this page,
      advanced the baseline past the fix, and left the box standing.** That is the failure worth
      keeping: *"verified against the tree at the baseline above"* was written over a tree that
      already contradicted it, and nothing on the page could tell the reader so. **This is the
      loudest box on the page, so its wrongness is deleted last, not first.**
      ❗ **What is NOT claimed by this strike.** The interfaces exist and the assertion in
      `internal/libsync/importer.go` now succeeds — that is a reading, and readings are what this
      file's own Done-when rule refuses on their own. **No `edition` row has been observed for a real
      BookOrbit audiobook**, and no Type cell or `/library/audiobooks` grid has been rendered off
      one. **The missing prerequisite is the same one §4 owes: an import against the owner's own
      instance.** Until then the correct statement is *"the cause this box named is gone"*, not
      *"audiobooks render correctly"*.

      ~~🔴 **OPEN DEFECT — EVERY BOOKORBIT BOOK RENDERS AS AN EBOOK, AND `/library/audiobooks` RETURNS
      NONE OF THEM. Verified against the tree at the baseline above.** This is a **user-visible wrong
      answer**, not a missing feature, and it is recorded here because slice 1's *"does not"* list
      makes it read as a scoped omission.~~
      ~~**The cause is one missing interface, and it is a three-hop chain:**~~
      1. ~~**`BookOrbitSource` implements no `FileSource`.** `internal/libsync/importer.go` asserts
         `src, ok := im.Source.(FileSource)` and treats a failed assertion as **not an error** —
         by design, per its own comment — so the assertion **silently returns false** and nothing
         anywhere reports it.~~
      2. ~~**So no `edition` row is ever written for a BookOrbit book.** `internal/libsync/files.go`'s
         `FileSource` is the only thing that produces them, and slice 1's file says so in terms:
         *"CREDITS AND EDITIONS … authors, narrators and `files[]` all ride the card this walk
         already reads"*, costing **zero extra HTTP** — which is precisely why it was held back as a
         slice of its own rather than taken for free.~~
      3. ~~**And both sides of the media-type answer read `edition.format`.** `mediaTypeOf`
         (`internal/store/recent.go`) returns `MediaTypeAudiobooks` for `kind = 'book'` **only** when
         the `allAudiobook` aggregate is `1`, and its own comment states the fallback: *"A book with
         NO editions is Ebooks … an absent edition is not evidence of an audiobook."* The grid filter
         agrees by construction — `browseAudiobookPredicate` (`internal/store/browse.go`) opens with
         `EXISTS (SELECT 1 FROM edition ea WHERE ea.format = ? AND ea.work_id = w.id)`, which
         **cannot be satisfied by a work with no edition rows at all.**~~
      ~~**So a BookOrbit audiobook is filed under Ebooks in its Type cell AND excluded from the
      Audiobooks grid — consistently.** ⚠️ **The consistency is NOT a mitigation.** `mediaTypeOf` and
      the predicate agreeing is exactly what `TestBrowseFilterAgreesWithMediaTypeOf` exists to hold,
      and here **they agree on the wrong answer, because both are reading an absence.** A guard that
      pins two readers to one rule cannot notice that the rule's input is missing.~~
      ~~⚠️ **BookOrbit'S OWN `MediaKind()` ALREADY DISTINGUISHES THEM, AND THE INFORMATION IS DISCARDED
      ONE LAYER UP.** `StreamItems` switches on `bookorbit.MediaKindEbook` and
      `bookorbit.MediaKindAudiobook` — it can already tell an M4B from an EPUB — and maps **both**
      through the same `mapBook`, because `store.CatalogueItem` has no format field to carry it on.
      **The fix is the deferred `FileSource` slice, not a new signal:** the adapter already knows.~~
      ~~🔍 **Inference, labelled, and NOT separately measured by this pass:** the same absence should
      also zero the `audiobooks` **facet count** for a BookOrbit-only install, since ADR-0059's split
      binds this same `browseAudiobookPredicate`. If so it is §2's facet consequence arriving from the
      other direction — there a count under-reports a type whose content is real, here the
      **editions themselves** are absent, so nothing downstream has anything to under-report from.~~
      ⚠️ **The struck inference is struck with its premise and is NOT thereby answered.** Whether a
      BookOrbit-only install's `audiobooks` facet count is right is now an ordinary open question
      about a shipped path, and it is **§2's facet item's**, not this box's.
      **Verified facts, read off BookOrbit's own source at HEAD `73b7877`, release `v2.6.0`** — carry
      these into the ADR rather than re-deriving them:
      - ✅ **§14 IS SATISFIED, and this falsifies the *"no inbound API key"* finding above.**
        **Magic-link tokens** give a **storable, revocable, optionally-expiring** credential that is
        **NOT the account password.** The credential model is therefore no worse than the Kavita one
        it was said to lose to — it is better on revocation.
      - ✅ **Covers on `/api/v1` are HEADER-authenticated, with no credential in the URL.**
        ⚠️ **The OPDS surface is different and must not be used:** it puts an **HMAC cover token in
        the query string**, so **an adapter must go through `/api/v1`.** This is the exact shape of
        the question `REVIEW-LOG.md` LS-260 had to write a probe for against Kavita, and here it is
        answerable from source.
      - ✅ **Comics are real: CBZ / CBR / CB7**, with a **`comic_metadata` table** and a
        **`comicvineId`** field. This narrows — it does not fully falsify — the *"no manga or comic
        external ids"* finding above.
      - ✅ **Licence AGPL-3.0.** Compatible; no licence question to answer.
      - ⚠️ **DOWNGRADED 2026-08-19 — THE WATERMARK IS INCOMPLETE, NOT ABSENT.** This bullet used to
        read *"~~THE ONE CONFIRMED OBSTACLE … THERE IS NO CHANGE WATERMARK OF ANY KIND … So an
        adapter must full-resync or diff locally … It is not partial; it is absent, and that is the
        harder claim.~~"* **That was a three-hop compression** — *"misses some edits"* → *"no
        watermark"* → *"full resync only"* — and every hop hardened a claim the evidence does not
        carry. ⚠️ **It also contradicted this same section**, whose struck evaluation block above
        reads *"an `updatedAt` watermark that misses tag, genre and author edits"*. **The struck
        version was the closer one**; this correction restores it rather than the bullet that
        overwrote it.
        **What the evidence supports:** there is **no `since`-style FILTER parameter and no changes
        feed** — but **`updatedAt` exists, and BookOrbit's API admits it as a SORT KEY with paging**,
        which is **an ordered page walk**: the incremental shape channel 3b is already built around
        (§7.1a). **Do not assert full-resync-only.**
        ❓ **What is genuinely unsettled is `updatedAt`'s COMPLETENESS.** It is set
        **application-side, with no DB trigger**, and **authors, tags and genres are not columns on
        the book row** — so a metadata-only edit can fail to move it. **What `updatedAt` does and
        does not cover is a probe's answer, not a reading's**: mutate a tag, a genre and an author on
        a known book, re-read `updatedAt`, report which moved it. **Commissioning that probe is the
        ADR's**, the same way LS-200's and LS-260's were commissioned.
      - ❓ **OPEN QUESTION for manga identity, read at that same HEAD `73b7877`: MangaUpdates,
        AniList and MyAnimeList ids are ABSENT.** `comicvineId` does not cover manga. Whether that is answered by the owner's own
        **MangaBaka sidecar** (§1, his words), by the *"official support"* he expects, or by
        something in UsArr is **undecided and belongs in the ADR.**
      *Authority:* §1's owner decision, [ADR-0052](./DECISIONS.md#adr-0052), **§16's v0.1 entry,
      which now assigns this adapter to v0.1.**
      *Was gated on:* an ADR that names v0.1's source, **answers the series-vs-books question**, and
      states **what the incremental read is** rather than presuming it. **ADR-0052 does all three** —
      BookOrbit; no series-ordered read; 3b for `work_book` and reconciliation-only for `work_comic`.
      ⚠️ **This line used to demand *"~~the full-resync-or-local-diff decision~~"*, which presumed
      the absence claim corrected below**, and then *"~~until then this item is a specification, not
      work in progress~~"*. **It is work now.**
      *Done when:* `internal/libsync` carries a BookOrbit adapter whose channel-1 import has been run
      against the owner's own instance — the rule in §1, which naming a source does not satisfy.
      ⚠️ **ITS FIRST HALF IS NOW MET AND THE SECOND IS THE WHOLE CONDITION.**
      `internal/libsync/bookorbit.go` exists (slices 1 and the completeness detection, above), so
      *"`internal/libsync` carries a BookOrbit adapter"* is no longer outstanding. **What remains is
      THE RUN, against the owner's own instance**, and it is §4's rather than this file's — **no
      commit can discharge it**, which is exactly why §1 states the rule as *one source, proven on
      real data* rather than *one source, written*.

**The importer, stream and UI plumbing is source-agnostic, and the Kavita adapter stays either way** —
see the blocked table above, where that is now its own row.

### The two ADRs behind slice 0 — WRITTEN, and they landed while this pass was running

⚠️ **No maximum and no next-free number is written here**, and none is to be inferred from this
list: reading a maximum out of *this* file mis-allocated an ADR once already (see the baseline
block). [`DECISIONS.md`](./DECISIONS.md) is authoritative both for what exists and for what is free.
Specific numbers are cited below because citing a specific ADR is not what went wrong.

- **[ADR-0057](./DECISIONS.md#adr-0057) — the `internal/breaker` lift.** The state machine moved out of
  `internal/servarr/breaker.go`, and `internal/kavita`'s deliberate copy collapsed onto it. What
  remains in both packages is a **thin wrapper, not a re-export**, and the distinction is the whole
  design: each `Allow()` must return **its own package's** `ErrBreakerOpen`, because
  `internal/libsync` and `internal/releases` match on those sentinels with `errors.Is` and a Kavita
  failure must not read `servarr: circuit breaker open`. The sentinel became an argument to
  `breaker.New` instead of a reason to keep a second state machine. The lift's own trigger was
  written into the copy in advance — *"worth taking the first time a THIRD client needs one"* — and
  `internal/bookorbit` is that third client.
- **[ADR-0058](./DECISIONS.md#adr-0058) — the credential-scope grading**, which
  `internal/bookorbit/scope.go` implements (the slice-0 record above), and which
  **[ADR-0052](./DECISIONS.md#adr-0052)'s §14 scope gate cites in a dated inline discharge note.**
- ⚠️ **THIS SUBSECTION WAS WRITTEN AS *"~~allocated … neither ADR is written, and that note is not
  there yet~~"*, AND WAS FALSE BEFORE IT WAS PUSHED.** It was fired at the baseline above, where
  `grep -n '^## ADR-005[5-8]' docs/DECISIONS.md` returned nothing — and `24c4a4d`
  (*"docs: ADR-0057 and ADR-0058 record what slice 0 decided"*) landed in the same window. **Both
  ADRs are written**, and **ADR-0052 carries its dated inline discharge note pointing at ADR-0058**,
  which states in terms that *"a discharge is not an amendment"* and strikes nothing in the gate's
  own paragraph. **This is the THIRD instance today of this file being correct when written and
  stale within the hour** — see the two on §2's Libraries-row link and facet items. The correction
  is recorded rather than the claim quietly swapped, because the pattern is the finding.

---

## 4. Joe's manual steps

Things no agent in this repo can do. Nothing here is blocked on code.

- **Run `deploy/update.sh`** on the server to pull and restart. `deploy/status.sh` reports what is
  running.
- 🛑 ~~**Run a full sync on the Kavita instance** so the library the importer reads is current.~~
  **STOPPED BY DECISION (§1)** — nothing is owed against an instance that is being sunset. The same
  goes for `kavita-cover-probe.sh`. ⚠️ **CORRECTED 2026-08-19: this line used to say the probe was
  *"~~written and never run~~"*. It WAS run** — by the owner, against his live instance, 2026-08-19,
  results pasted into the library-sync thread and deliberately not committed; §2's image-pipeline
  item carries what it measured and which LS-260 questions that does and does not answer. The script
  stays at the repo root with its criterion intact, and is not a task any more.
- ✅ ~~**Confirm or drop the BookOrbit direction** once the instance is up (§3).~~ **DONE — he
  confirmed it, 2026-08-19 (§1).** ⚠️ **This bullet used to close *"~~an ADR is owed~~"*; it landed
  as [ADR-0052](./DECISIONS.md#adr-0052)**, so nothing here is outstanding against the owner.
  **What v0.1 still owes him is the run, not a decision** (§1): the BookOrbit adapter exercised
  against his own library is what proves the replica thesis, and no document can discharge it.
- **Verify Symfonium's `apiKeyAuthentication` support against a live client** before any gateway code
  is written. Far out — it gates v0.4, not v0.1 — but it is unverified and the whole v0.4 success
  criterion rests on it (§16 v0.4 entry).

---

## 5. Out of scope

Not restated here, deliberately — one list, one owner:

- **Deferred, wanted later, each with the seam that keeps it cheap:** [`FUTURE.md`](./FUTURE.md) —
  where a heading reading *Declined* means closed rather than deferred.
- **Permanent refusals:** [`ARCHITECTURE.md`](./ARCHITECTURE.md) §1.4 and §16's *Explicitly never*.
  Do not propose them and do not reopen them.
- **Which milestone owns a thing:** §16. Not this file.
