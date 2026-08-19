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
>
> **Extended 2026-08-19 by the Kavita-sunset pass, to exactly TWO further items and to nothing
> else:** §2's **image-pipeline** item, whose `ARCHITECTURE.md:2649-2651` citation was **~40 lines
> off** and is a section reference now rather than a corrected number, and §2's **output-codec**
> item, whose `00005_library_sync.sql:219-236` citation named a schema fact that migration `00008`
> had already falsified. That pass rewrote §1, §2's image / codec / covers items, §3's sequencing
> table and its BookOrbit entry, and §4. **It read nothing else for line drift, and no sweep
> happened this time either.** Every other line citation in this file remains unvetted.

**Last re-derived against:** `origin/main` `3c88b2e` (2026-08-19). Every check quoted below was
**fired**, not inherited. Precisely: the Go, schema and `ARCHITECTURE.md` reads were fired at
`d5ab034`, the tip when the Kavita-sunset pass started. `fc2b7c4` and `2ce8ed9` landed mid-pass —
both touch **`docs/reference/` only**, change no Go and no migration, and both were read directly for
what §2's obligation 3 says about them.

---

## 1. The objective

1. ⚠️ **SUPERSEDED 2026-08-19 BY AN OWNER DECISION — pending an ADR that is NOT YET WRITTEN.**
   The objective read, and the old text is kept visible rather than deleted:
   *"~~v0.1 proves the **replica thesis on real data**: the owner's own Kavita, imported,
   delta-synced, reconciled, searched and rendered from local SQLite
   ([ADR-0041](./DECISIONS.md#adr-0041), §16).~~"*

   **Joe is sunsetting Kavita. OWNER-DECIDED, not delegated**, 2026-08-19, in his own words:
   > *"i'm gonna sunset kavita. bookorbit is phenomenal. atm i have a sidecar approach of pulling
   > data from mangabaka. but i think there's going to be official support in the near future."*

   **What survives is the rule**, which no re-sequencing has ever moved: **one source, proven on
   real data, before a second adapter**. **What is falsified is the claim that Kavita is that
   source** — the owner will not be running it, so a replica proven against it would be proven on
   data nobody keeps, which is the one thing the criterion was written to prevent.
   [ADR-0041](./DECISIONS.md#adr-0041) states the position this supersedes; **it is not amended
   here, because this file cannot amend an ADR** — see §3.
   ⚠️ **No ADR number is cited, on purpose: none is allocated.** `0051` is the highest on `main` at
   the baseline above. Do not invent the next one; writing it belongs to the lane that owns
   `DECISIONS.md`.
   **Until that ADR lands, v0.1's proven source is UNDECIDED — it is not "BookOrbit" yet.** The
   owner named a direction, not an adapter assignment (§3).
2. Alongside it, **Prowlarr Search-and-Grab** is v0.1's one write path — the request surface for all
   six media types (§8.5, §16).
3. Five screens ship: Home, Services, Libraries, Search, Requests (`CLAUDE.md`, §17).

---

## 2. v0.1 remaining work

Ordered roughly by what the rest depends on, not by size.

⚠️ **EVERY ITEM BELOW THAT NAMES KAVITA INHERITS §1's SUPERSEDED FRAMING** — channel 3b, channel 4,
the per-series volume/chapter walk, the *"not identified"* badge, and the zero-external-providers
evidence clause. **The work each names is real and source-shaped; which source it is written against
is the open question**, and not one of them is re-pointed here, because re-pointing them is the
unwritten ADR's job, not this file's. Items marked 🛑 **STOPPED** are the different case: those are
stopped by the decision itself, not merely re-pointed by it.

- [ ] **Channel 3b — the ordered page-walk delta, for Kavita.** The watermark walk with an overlap
      window and a client-side stop, so an import is not the only way the replica moves.
      *Authority:* §7.1a, §16 v0.1 entry, [ADR-0041](./DECISIONS.md#adr-0041).
      *Done when:* `internal/libsync` has a delta path and `internal/libsync/doc.go` stops listing
      channel 3b under "NOT HERE".

- [ ] **Channel 4 — reconciliation, with both sweep guards and 7-day tombstones.** It carries more
      weight for Kavita than for an \*Arr: a page walk cannot observe a deletion, and Kavita's
      watermark moves on a chapter *add* only.
      *Authority:* §7.4, §16 v0.1 entry ([ADR-0035](./DECISIONS.md#adr-0035) §2a).
      *Done when:* `grep -rn 'missing_since\|orphaned_at' --include=*.go internal/` shows a statement
      that **sets** a non-NULL value. Today every one clears it.

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

- [ ] **Home Block A — the media-type summary.** Blocks B and C are drawn; A is not.
      *Authority:* §17.2 as amended by [ADR-0028](./DECISIONS.md#adr-0028), §16 v0.1 entry.
      *Done when:* `web/src/routes/+page.svelte`'s block map stops saying `Block A … NOT DRAWN`.

- [ ] **Libraries — the auto-proposal flow and its Accept step.** The row view over
      `GET /api/v1/libraries` is drawn; the proposal step is not, and its storage question is now
      answered: a proposal is **not** a `library` row, and a row is created only on Accept.
      *Authority:* §17.8, [ADR-0026](./DECISIONS.md#adr-0026), [ADR-0048](./DECISIONS.md#adr-0048).
      *Done when:* an Accept path exists that creates a `library` row, and the proposal has a home
      that is not that table.

- [ ] **The per-series volume and chapter walk, and the rows it writes.** Phase A is served; the walk
      that fills `work_comic_issue` and `media_file` is not fetched and is not faked.
      *Authority:* §7.2, `internal/libsync/doc.go`.
      *Done when:* `internal/libsync/kavita.go` performs the walk and `work_comic_issue` has a writer
      in non-test Go.

- [ ] **The "not identified" badge and the column under it.** Free Kavita returns null identifier
      fields, so this is v0.1's ordinary case, not an edge one. The badge is v0.1; **the remedy is
      not** — see §3.
      *Authority:* §6.4, §16 v0.1 entry, [ADR-0035](./DECISIONS.md#adr-0035) §1.
      *Done when:* the state is rendered in `web/src/routes` off a column that exists in
      `internal/db/migrations`.

- [ ] **The image pipeline, including the §4.4.1 cold-start plan.** `image_asset` is in the schema;
      nothing in `internal/` or `cmd/` writes or serves it.
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
      `kavita-cover-probe.sh` was written to answer (`REVIEW-LOG.md` LS-260). **There is no probe
      result to carry: the probe was never run.** The script and its stated-in-advance criterion sit
      at the repo root; **nothing is owed against them now.** What is *not* stopped is everything
      source-independent — the encoder, the seven-width allowlist, the cache and the route.
      *Authority:* §4.4, §16's v0.1 entry.

      *Done when — **ALL THREE** legs. The first two run from a clean checkout:*
      1. **A registered image route exists.**
         `grep -nE 'mux\.Handle(Func)?\("[A-Z]+ /(api/v1/)?(img|image|cover)' internal/httpapi/server.go`
         returns a hit. **Fired on the baseline tree: exit 1, no output — RED today.** The mux
         registers no route under `img`, `image`, `cover`, `poster` or `thumb`.
      2. **A non-test writer stores a REAL format, not NULL.**
         `grep -rn 'INSERT INTO image_asset' --include=*.go internal/ cmd/ | grep -v _test.go`
         returns a hit, **and that same file references** `store.ValidImageFormat` or
         `store.ImageFormatJPEG`. **Fired: exit 1 on the first half — RED today**; the only non-test
         references to the format vocabulary anywhere in the tree are its own declarations in
         `internal/store/images.go`.
      3. **Bytes actually come back.** Against a running instance,
         `curl -sS -o /dev/null -w '%{http_code} %{content_type} %{size_download}\n' '<base>/img/<key>?w=342'`
         answers `200`, an `image/*` content type and a **non-zero** size. (This container has no
         `sqlite3` CLI, so this leg is deliberately a request rather than a query.)

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

      **WHAT THE FIRST WRITER OWES: THREE OBLIGATIONS AND ONE DESIGN DECISION.** They are kept
      together here rather than scattered, because this file is where someone will look to find out
      what is actually owed. Every one was verified against the tree at the baseline above.

      **Obligation 1 — the format vocabulary.**
      - **Any future `image_asset` writer must reference `store.ValidImageFormat` (or
        `store.ImageFormatJPEG`), or `make check` goes RED.**
        `TestImageWritesValidateTheFormatVocabulary` (`internal/store/imagelint_test.go`) is an AST
        walk over non-test code that matches `INSERT` / `INSERT OR IGNORE` / `REPLACE` / `UPDATE`
        against `image_asset`, including quoted, backticked and `main.`-qualified spellings, and it
        **fires its own matcher against known strings before trusting it**. It is **vacuous today**
        because no writer exists, and **it flips the moment one does — including a writer that
        stores only NULL.** [ADR-0050](./DECISIONS.md#adr-0050) and
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

      **Obligation 3 — REJECT A `source_url` THAT STILL CARRIES A CREDENTIAL. ⚠️ THIS WAS
      DOCUMENTED AS SHIPPED WHEN IT IS NOT, AND THAT IS WHY IT IS ON THIS LIST.**
      - **It does not exist.** There is no image pipeline, and **`source_url` appears in non-test Go
        exactly once, in a comment** — `internal/ssrf/redact.go:14`. Fired on the baseline tree:
        `grep -rn 'source_url\|SourceURL' --include=*.go internal/ cmd/ | grep -v _test.go` returns
        that one line and nothing else.
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
      - **The obligation on the writer:** strip credential parameters before the row is written,
        with the names taken from `credentialParams` and **never restated locally** — and note
        `cache_key = sha256(source_url)[:16]`, so getting this wrong does not merely leak, it makes
        a provider-key rotation silently invalidate the whole image cache.

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
      driven from `web/src/routes/+page.svelte:474` — with the stop rule tested: a short *or* empty
      page that still carries a cursor does not stop the walk
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

- [ ] **The PER-TYPE library grid, `/library/{type}` — the SCREEN. It is no longer backend-blocked.**
      Not one all-types screen: navigation is §17.2's **six-value media-type enum**, one sidebar
      entry per type *that has content*, and item routes are already `/library/{type}/{id}` — named
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
      **What is still missing is the FRONTEND.** `web/src/routes/` holds `library/` — one unified
      newest-first table that still reads **only** `/api/v1/library/recent` — plus `libraries/`,
      which is §17.8's row view. **There is no `/library/{type}` route and no grid.**
      ⚠️ **The interim `/library` table is a SLICE of this line item and NEVER a tick.** Its missing
      type filter and sort control were justified here by *"an endpoint with no kind facet and one
      fixed order"* — **that premise has expired**: the facet and three orders now exist. The
      controls are simply not wired yet. 🔍 Inference: the §17 honesty rule that justified their
      absence no longer applies once a control can act on the whole table, so the next move is to
      wire them, not to re-argue them.
      *Authority:* §16's v0.1 entry, §17.2, §17, §13's budget table, §4.5,
      `reference/http-api.md` §7.
      *Done when:* a `/library/{type}` route exists under `web/src/routes/` and renders over
      `GET /api/v1/library`.

- [ ] **A facet-counts read — until there is one, a data-driven sidebar has NO honest source.**
      `reference/http-api.md` §7.1 closes the wire question: *"There are **no facet counts** beside
      the chips; each is its own aggregate and its own read."* `design/DESIGN-DIRECTION.md` §8.1
      decided the opposite shape — *"one sidebar entry per type **that has content**"* — and §17.2's
      nav table repeats it. The sidebar the design asks for cannot be built over the wire as it is.
      ⚠️ **The frontend's interim answer is to ship NO media-type entries — not six placeholders —
      and that is DELIBERATE, not a gap.** `web/src/routes/+layout.svelte`'s file header records the
      reason: §17.2's hard rule is that a type the user does not have is *not shown AT ALL*, so six
      placeholder rows would break that rule in the one place it is most visible. `NAV_GROUPS` in
      that file therefore ships the fixed entry set only, and labels `/library` **"Recently added"**
      rather than "Library" so the row does not promise the screen §16 specifies.
      🔍 Inference: §13 already budgets **"1 keyset page + 6 sidebar `COUNT(*)`"** at < 15 ms p50, so
      the cost is priced — but nothing decides whether the counts ride the browse response or their
      own endpoint, and that closes off an alternative, so it needs an ADR.
      *Authority:* `reference/http-api.md` §7.1, `design/DESIGN-DIRECTION.md` §8.1, §17.2, §13.
      *Done when:* per-media-type counts reach the client from a documented read **and** `NAV_GROUPS`
      is driven from them — **or** an ADR records the alternative.

- [ ] **The COVERS / POSTER half of §16's grid line — there is NO image route at all.**
      The browse merge covered only the row-grid half of §16's v0.1 sentence. **Searched
      `internal/httpapi/server.go` for `img`, `image`, `cover`, `poster`, `thumb` and `MediaCover`:
      the only hits are two comment lines in the middleware chain, and nothing registers a route
      under any of them.** No handler in `internal/httpapi/` matches `handle*(Image|Cover|Poster|Img)`
      either. §13 budgets `GET /img/{k}?w=342` at < 3 ms p50 on a cache hit; that route does not
      exist.
      The schema is ahead of the wire — `image_asset`, `work.poster_asset_id` and
      `work.backdrop_asset_id` are in `internal/db/migrations/00005_library_sync.sql` — which is why
      `reference/http-api.md` §7.1 says shipping `poster_asset_id` today *"would be an id the client
      cannot turn into anything"*, and why the browse response does not ship it.
      ⚠️ **This is the SERVING half only.** Producing the bytes is the image-pipeline item above.
      ⚠️ **The third check on this §16 line is now a FALSIFIED box, not an open one:** the *"undecided
      output codec"* item above is closed by [ADR-0050](./DECISIONS.md#adr-0050), so the encoder is
      no longer blocked on a decision. One §16 line, three separate checks, one of them discharged.
      🛑 **Nothing in THIS item is stopped by §1's sunset.** The route, its key, the `Content-Type`
      derived from `image_asset.format` and the id on the browse response are all
      source-independent. What §1 stopped is the Kavita **fetch** half, which is the item above.
      *Authority:* §16's v0.1 entry, §4.4, §13's budget table, `reference/http-api.md` §7.1.
      *Done when:* an image route is registered in `internal/httpapi/server.go`'s mux **and** the
      browse response carries an id that resolves through it.

- [ ] **A relevance score on the wire.** §17.4 rule 2 orders grouped results *"by the group's
      best-scoring hit, descending"*, and §6.2 publishes no score — *"**`items` is ordered by
      relevance and the ORDER IS THE CONTRACT.** No score is published"* — deliberately, because it
      is normalised per query and publishing it would freeze §6.6's ranking. §17.4's grouped card
      therefore cannot be built over the response as specified. 🔍 Inference: which way that
      resolves — a published score, server-side grouping, or an amended §17.4 — is decided nowhere,
      so it closes off an alternative and needs an ADR.
      *Authority:* §17.4 rule 2, `reference/http-api.md` §6.2 and §6.6, §16 v0.1 entry.
      *Done when:* the search response carries a field §6.2 documents for grouping, **or** an ADR
      records the alternative.

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

- [ ] **The Docker image, and `VACUUM INTO` backups as a shipped path.** `cmd/usarr/backup.go` exists;
      there is no `Dockerfile` anywhere in the tree.
      *Authority:* §15, §16 v0.1 entry.
      *Done when:* a `Dockerfile` exists.

- [ ] **The arm64 RSS spike.** §16 calls it a day-one spike. `internal/db/spike/` exists; whether the
      arm64 measurement was taken is not readable from the tree.
      *Authority:* §13, §16 v0.1 entry.
      *Done when:* a recorded measurement exists in `docs/`.

- [ ] **The zero-external-providers evidence clause for Kavita.** §16 says v0.1 needs no TMDB account
      because the source carries its own metadata. That claim was evidenced against Radarr's
      `MovieResource` and Sonarr's `SeriesResource`, and **neither is in v0.1 any more**; the
      equivalent primary-source check against Kavita's payloads is owed and has not been made.
      *Authority:* §16 v0.1 entry, which flags it against itself.
      *Done when:* `docs/RESEARCH.md` carries the citation.

**Already discharged, listed so nobody re-opens them:** the Kavita `LastChapterAdded` watermark probe
(ran 2026-08-17 against the owner's live instance and passed — [ADR-0035](./DECISIONS.md#adr-0035)
§2a); the three Kavita subtype tables (`00006_kavita_subtypes.sql`); `work_credit`
(`00007_work_credit.sql`, [ADR-0044](./DECISIONS.md#adr-0044)).

---

## 3. Blocked and sequenced

| Item | Blocked on / sequenced behind |
|---|---|
| ~~Wiring Kavita's `PluginVersion`, or any second Kavita endpoint taking a credential in a query or path~~ | **UNBLOCKED.** All four LS-170 steps landed (`REVIEW-LOG.md` LS-170 § *Applied*), so the ordering constraint that gated this is discharged. `PluginVersion` remains unwired: nothing calls it, and whether to wire it is a separate decision that LS-170 no longer gates. |
| A second catalogue adapter (Navidrome, then Audiobookshelf, then Komga) | ⚠️ **THE SEQUENCE IS NOW CONTRADICTED BY AN OWNER DECISION (§1) AND CANNOT BE FIXED FROM THIS FILE.** It read: *"~~v0.1's Kavita adapter landing and running against a real library~~"*. **ARCHITECTURE §16.1 sequences the post-v0.1 sources and §16.1 is NOT this file's to edit** — it still says *"Kavita is no longer in this table because it moved INTO v0.1"*, still calls Kavita *"v0.1's one catalogue source and the sync core's first adapter"*, and still rests the table's order on that. **§16.1 AND AN ADR MUST BOTH BE UPDATED BY THE LANES THAT OWN THEM.** This row is the pointer, deliberately, and not a fresher claim: no replacement gate is written here, because writing one would be inventing the decision the ADR owes. **The rule itself is untouched and survives every re-sequencing: one source, proven on real data, before a second adapter** (§16.0, §16.1, [ADR-0036](./DECISIONS.md#adr-0036)). |
| **The Kavita adapter code itself — RE-SEQUENCED, NOT CUT** | **Nothing.** It **STAYS IN THE TREE**: `internal/kavita`, `internal/libsync`'s Kavita path, `00006_kavita_subtypes.sql` and the recorded fixtures all remain, because **other people run Kavita** and principle 3 (*pluggable by default*) is the reason the adapter exists at all. **What the sunset stops is INVESTMENT, not the code** — no deletion, no deprecation notice, no migration. Read any Kavita item in §2 as *"unfunded, still standing"*, never as *"dead"*. |
| The minimal write path — `monitor`, `unmonitor`, `delete`, `add`, the queue worker and its settlement loop | **v0.2**, with the first \*Arr adapter ([ADR-0042](./DECISIONS.md#adr-0042), [ADR-0045](./DECISIONS.md#adr-0045)). `write_queue` stays in the schema with **no writer for the whole of v0.1** — that is the seam, and it costs no migration ([ADR-0039](./DECISIONS.md#adr-0039)). |
| [ADR-0039](./DECISIONS.md#adr-0039)'s outstanding Go `state`-vocabulary declaration and validation | The first `write_queue` writer, which is v0.2's. |
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
source to it and has #1 landing *"before or alongside"* — and Navidrome was **sequenced behind v0.1's
Kavita adapter running on a real library**, per the table above.
⚠️ **THAT SECOND CAVEAT IS THE ONE §1 BROKE**, and the recommendation is left standing rather than
rewritten, because rewriting it here would be inventing the ADR's answer. Its *reasoning* is if
anything strengthened — Navidrome runs on the owner's own stack and can therefore be **proven on real
data**, which is the rule that outlived the source. Its *ordering premise* is void: there is no
settled v0.1 Kavita adapter for it to sit behind any more. **Which source now precedes it is §16.1's
and the ADR's to say.**

**Wording this into §16 or an ADR belongs to the implementation thread, not to this file.**

### BookOrbit — the direction is OWNER-DECIDED; the ADAPTER is gated on an unwritten ADR

⚠️ **THIS SECTION USED TO BE HEADED *"Open decision — BookOrbit as a books backend"* AND IS NO LONGER
OPEN.** §1 carries the owner's words. Two of that entry's three *"Against"* findings are also
falsified below. The old text is kept visible so the reversal is legible:

> **~~Tracking, not a decision.~~** ~~Joe is standing up a BookOrbit instance and is **leaning
> toward** migrating his books backend off Kavita (2026-08-18: *"in my heart i kind of want to
> migrate to book orbit… it doesn't have a paid tier"*).~~ ⚠️ ~~**Against:** **no inbound API key** —
> headless auth needs the account password, which is worse than UsArr's Kavita credential model; an
> `updatedAt` watermark that **misses tag, genre and author edits**; and **no manga or comic external
> ids**.~~ ⚠️ ~~**The standing recommendation from that evaluation is: do NOT switch UsArr's first
> adapter off Kavita.**~~

That evaluation ran at HEAD **`4a420a04`** (2026-08-17). **§16 still assigns BookOrbit nothing, and
no ADR backs any of this.**

- [ ] **The BookOrbit catalogue adapter — GATED ON THE UNWRITTEN ADR (§1), and on nothing else.**
      **Do not start it before that ADR exists.** No number is cited because none is allocated.
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
      - 🛑 **THE ONE CONFIRMED OBSTACLE, and the ADR must decide it explicitly: THERE IS NO CHANGE
        WATERMARK OF ANY KIND.** No `updatedSince`, no changes feed, and **`updatedAt` is *provably*
        not bumped by tag, genre or author edits.** So **an adapter must full-resync or diff
        locally**, and **which of those it does is a design decision the ADR owes** — it is not an
        implementation detail to be discovered later. ⚠️ Note the shape: the earlier evaluation
        called this *"an `updatedAt` watermark that misses tag, genre and author edits"*, which reads
        as a partial watermark. **It is not partial; it is absent**, and that is the harder claim.
      - ❓ **OPEN QUESTION for manga identity: MangaUpdates, AniList and MyAnimeList ids are
        ABSENT.** `comicvineId` does not cover manga. Whether that is answered by the owner's own
        **MangaBaka sidecar** (§1, his words), by the *"official support"* he expects, or by
        something in UsArr is **undecided and belongs in the ADR.**
      *Authority:* §1's owner decision. **§16 assigns this nothing; this box is not a milestone
      claim.**
      *Done when:* the ADR exists, names a source for v0.1, and states the full-resync-or-local-diff
      decision. **Until then this item is a specification, not work in progress.**

**The importer, stream and UI plumbing is source-agnostic, and the Kavita adapter stays either way** —
see the blocked table above, where that is now its own row.

---

## 4. Joe's manual steps

Things no agent in this repo can do. Nothing here is blocked on code.

- **Run `deploy/update.sh`** on the server to pull and restart. `deploy/status.sh` reports what is
  running.
- 🛑 ~~**Run a full sync on the Kavita instance** so the library the importer reads is current.~~
  **STOPPED BY DECISION (§1)** — nothing is owed against an instance that is being sunset. The same
  goes for `kavita-cover-probe.sh`, which was written and **never run**; it stays at the repo root
  with its criterion intact, and is not a task any more.
- ✅ ~~**Confirm or drop the BookOrbit direction** once the instance is up (§3).~~ **DONE — he
  confirmed it, 2026-08-19 (§1).** What replaces it is not his: **an ADR is owed**, and writing it
  belongs to the lane that owns `DECISIONS.md`, not to this list.
- **Verify Symfonium's `apiKeyAuthentication` support against a live client** before any gateway code
  is written. Far out — it gates v0.4, not v0.1 — but it is unverified and the whole v0.4 success
  criterion rests on it (§16 v0.4 entry).

---

## 5. Out of scope

Not restated here, deliberately — one list, one owner:

- **Deferred, wanted later, each with the seam that keeps it cheap:** [`FUTURE.md`](./FUTURE.md).
- **Permanent refusals:** [`ARCHITECTURE.md`](./ARCHITECTURE.md) §1.4 and §16's *Explicitly never*.
  Do not propose them and do not reopen them.
- **Which milestone owns a thing:** §16. Not this file.
