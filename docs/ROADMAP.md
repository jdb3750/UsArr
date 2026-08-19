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
> **Absence rule, added 2026-08-19 after this file wrongly recorded a probe as *"never run"*:**
> a missing artefact **in the repo** does not establish that something never happened — a result can
> live in a thread, on the owner's box, or nowhere. A claim of the form *"X was never done"* needs a
> source that **would have recorded X**, not merely a place where X is not.

**Last re-derived against:** `origin/main` `c7d9ed3` (2026-08-19).
⚠️ **The baseline moved from `7bd45e9` by the ADR-0052/0053 consolidation pass, and what that pass
fired is listed rather than summarised**, because the intervening range is large — it carries Go,
`web/src/` and `docs/` changes alike, so **inheritance is not safe here the way it was for the two
documentation-only passes before it**.
**FIRED on this tree, at the baseline above:**
- **ADRs**, read directly: [ADR-0052](./DECISIONS.md#adr-0052) and [ADR-0053](./DECISIONS.md#adr-0053).
  `grep -o '^## ADR-[0-9]*' docs/DECISIONS.md | tail -3` → **`0053` is the highest on `main`**.
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
**THREE COMMITS LANDED MID-PASS and the baseline is deliberately NOT advanced past what was read.**
`adf8960`, `a6bf6c1` and `cad0563` touch `docs/DEVELOPMENT.md`, `docs/design/DESIGN-DIRECTION.md` and
one blockquote in `docs/DECISIONS.md` — **no Go, no migration, no `web/src/`** — so the checks above
are not stale by them. `cad0563` was read directly, because it marks ADR-0052's mockup re-draw
discharged and §3's new deferred row turns on exactly what that mark does and does not cover.
**NOT fired, and therefore inherited:** **no schema check and no migration read** — nothing below
that rests on `internal/db/migrations` was re-derived by this pass — **no `make check` line citation
was swept**, and **every §2 box this pass does not name is inherited, not re-fired.**
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
   is dropped rather than corrected — **`0053` is**, and ADR-0053 is §2's sidebar decision.

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
channel 3b, channel 4, the per-series volume/chapter walk, the *"not identified"* badge, and the
zero-external-providers evidence clause. **The work each names is real and source-shaped**, and what
each owes is that same work pointed at BookOrbit. **None of them is re-pointed line by line here**,
and the reason has changed: this used to say *"~~re-pointing them is the unwritten ADR's job~~"*, and
the ADR landed — [ADR-0052](./DECISIONS.md#adr-0052) **deliberately declined to re-answer** ADR-0041
clause 4's channel list, reopening it instead. So the per-item rewrite is owed by whoever writes the
adapter, against §16's narrower channel sentence (§1), and **channel 3b's item in particular cannot
simply be re-pointed** — for `work_comic` there is nothing at the far end to point it at (§3).
Items marked 🛑 **STOPPED** are the different case: those are stopped by the decision itself.

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
      unified newest-first table over `/api/v1/library/recent` — is **kept beside it rather than
      replaced**, because §17.2 keeps both and `reference/http-api.md` §7 calls the browse read *"a
      different endpoint from §1, not a superset of it"*.
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
      Block C at full length, and §17.2 closes it at **one table, one order and no filters** on
      purpose, so wiring a filter onto it would contradict the section rather than finish it. The
      screen's own header says so, and names the trap — what it holds is a keyset **prefix** of the
      catalogue, so a client-side control would present itself as operating on the library while
      operating on whatever has been loaded. **The filters and the sort control belong to the
      per-type grid, and that is where they shipped.**
      *Authority:* §16's v0.1 entry, §17.2, §17, §13's budget table, §4.5,
      `reference/http-api.md` §7.
      *Was done when:* a `/library/{type}` route exists under `web/src/routes/` and renders over
      `GET /api/v1/library`. **Both hold.**
      *What this box does NOT cover, and each is its own line below:* the covers half of §16's grid
      sentence (there is still no image route), the **all-types scoped view** a Libraries row should
      open, the **link** from a Libraries row into it, and the **`?lib=` chip** that writes scope.

- [ ] **A facet read — until there is one, per-type hiding cannot come back and Home's Block A has
      no source.** `reference/http-api.md` §7.1 closes the wire question: *"There are **no facet
      counts** beside the chips; each is its own aggregate and its own read."*
      ⚠️ **THE SIDEBAR CLAIM ON THIS ITEM WAS STALE AND IS CORRECTED. It used to read *"~~the
      frontend's interim answer is to ship NO media-type entries — not six placeholders — and that is
      DELIBERATE~~"*.** That was true when it was written and is not now: the grid work at `ad821a0`
      ships **all six, unconditionally**. `TYPE_NAV` in `web/src/routes/+layout.svelte` is a
      `MEDIA_TYPES.map` with **no predicate** over the six-member `as const` in
      `web/src/lib/library.ts`, and every entry resolves to `/library/[type]`.
      [ADR-0053](./DECISIONS.md#adr-0053) (Accepted 2026-08-19) is the decision behind it: it amends
      [ADR-0027](./DECISIONS.md#adr-0027)'s *"a type with zero items is not rendered anywhere"*
      consequence **for the sidebar and for nothing else**, leaving it intact for Block A and for
      search groups. **The honesty the sidebar can no longer carry moved to the empty state** —
      `browseEmptyState` (`web/src/lib/librarygrid.ts`) separates *no library-bearing service is
      connected* from *this type has no rows yet* from *the scope excludes everything*.
      **SO WHAT IS OWED IS NOT A SIDEBAR — IT IS THE READ THAT LETS PER-TYPE HIDING RETURN.** Hiding
      is **closed, not abandoned**, on one named reopening condition: a documented read that answers
      **which of the six types have rows under the current scope, in one statement**. **No read on
      the wire answers per-type presence or per-type count today** — `GET /api/v1/library` answers
      only for the one `media_type` asked and only by fetching its rows, `GET /api/v1/library/recent`
      carries no facet at all, and `GET /api/v1/services/health` carries a `role`, not a media-kind
      array. Block A above is blocked on this same missing read.
      🔍 Inference, unchanged by ADR-0053: §13 already budgets **"1 keyset page + 6 sidebar
      `COUNT(*)`"** at < 15 ms p50, so the cost is priced — but nothing decides whether the counts
      ride the browse response or take their own endpoint. ADR-0053 leaves that to the facet read
      rather than pre-empting it, and it closes off an alternative, so it needs an ADR.
      *Authority:* `reference/http-api.md` §7.1, [ADR-0053](./DECISIONS.md#adr-0053),
      `design/DESIGN-DIRECTION.md` §8.1, §17.2, §13.
      *Done when:* per-media-type presence reaches the client from a documented read **and**
      `TYPE_NAV` grows the predicate ADR-0053 calls one line wide — **or** an ADR records the
      alternative.

- [ ] **The ALL-TYPES scoped view a Libraries row opens — DECIDED, MEASURED ON THE WIRE, unbuilt.**
      **The decision, recorded here because it is the kind that gets silently re-decided:** a
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
      ⚠️ **A–Z IS A `400` ON AN ALL-TYPES QUERY, AND THE SORT CONTROL MUST *KNOW* THAT RATHER THAN
      DISCOVER IT AS AN ERROR.** The refusal is `sort == WorksSortTitle && len(kinds) != 1` — **a
      kind count, not a library-scope rule** — and the unfiltered case is six kinds. **So this view
      ships `added_at` and `popularity` only.** `$lib/librarygrid`'s `browseKindCount`,
      `browseSortsFor` and `browseSortAvailable` are keyed on a **required** `MediaType` and have no
      all-types arm; giving them one is part of this item. See the A–Z flag on the grid item above.
      ⚠️ **A small real wrinkle for whoever is next in `internal/httpapi/library.go` — a one-line
      fix.** `ErrUnservableSort` renders **one shared action**, *"sort_title needs a media_type of one
      kind — not music — and there is no index behind year at all"*, so **a user arriving from an
      all-types scoped view is told about a media type they never chose** and about a sort they never
      asked for. The refusal is right; the copy names the wrong cause for this caller.
      *Authority:* §17.8, §17.2, [ADR-0051](./DECISIONS.md#adr-0051), `reference/http-api.md` §7.1
      and §7.3.
      *Done when:* a route under `web/src/routes/` renders `GET /api/v1/library` with `?lib=` and
      **no** `media_type`, and its sort control offers `added_at` and `popularity` without offering
      A–Z.

- [ ] **A path from a Libraries row INTO its scoped view — nothing links one.** `Library.slug` is
      parsed (`web/src/lib/libraries.ts`, `slug: str(value.slug)`) and its own doc comment says what
      it is for — *"it is carried because it is the chip's `?lib=` value"* — while
      `web/src/routes/libraries/+page.svelte` renders **no link that uses it**: its `href`s go to the
      login screen and to Services, and nowhere else. **The one screen that knows every library's
      slug is the one screen that cannot open one.**
      🔍 Inference, and it is why this sits **before** the chip below rather than after it: a row
      that opens its own scope discharges most of what the chip is for — *arriving* at a scope —
      leaving the chip the part a link cannot do, which is **changing** one.
      *Authority:* §17.8, `reference/http-api.md` §7.3.
      *Done when:* a Libraries row is a link whose target carries `?lib=<slug>`.

- [ ] **The `?lib=` scope chip — the control that WRITES scope. Everything that READS scope ships;
      nothing authors it.** `design/DESIGN-DIRECTION.md` §8.1 pins it **above the nav** — *"a library
      is a **scope**, not a place — a multi-select chip pinned above the nav"* — has it render
      nothing at 0 or 1 library, and hoists it into the top bar at narrow widths because the drawer
      must never be the only statement of an active scope.
      **The read end of the loop is complete:** `readLibraryScope` and `MAX_LIBRARY_SLUGS` bound a
      scope at 32 before anything is sent; `browseParams` **deletes** the parameter rather than
      emptying it, because an empty `?lib=` is a `400` and not "no scope"; the server echoes the
      slugs it resolved; and `web/src/routes/library/[type]/+page.svelte` renders *"Scoped to …"*
      with an address that clears it. **The write end is empty:** nothing in `web/src/routes` puts a
      slug into `?lib=`.
      *Authority:* `design/DESIGN-DIRECTION.md` §8.1, §17.2, `reference/http-api.md` §7.3.
      *Done when:* a control in `web/src/routes` sets `?lib=` and removes it again.

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
      *Authority:* §17.4 rule 2, `reference/http-api.md` §6.2 and §6.6, §16 v0.1 entry.

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
      equivalent primary-source check against Kavita's payloads is **owed and undischarged in the
      repo.** ⚠️ **That is NOT the claim that the check was never made** — see the header's
      **absence rule**: a check can have been run and never written up, exactly as the cover probe
      was. What is readable is only the repo's silence, which is the weaker claim, and an
      undischarged obligation is still owed, so this box stays open.
      *Authority:* §16 v0.1 entry, which flags it against itself.
      *Done when:* `docs/RESEARCH.md` carries the citation. Fired at the baseline above:
      `grep -n -i kavita docs/RESEARCH.md` returns 9 lines — the API surface, the auth scheme, the
      `sortByLastModified` finding, and *"identifier matching is a paid subscription feature"* — and
      **none of them evidences this clause** the way `MovieResource` and `SeriesResource` did.

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
| **Re-keying the design mockups' v0.1 DATA off BookOrbit** — a real design change, and a different thing from the label swap | **The owner's mixed-library answer, and the two ADRs the sync lane has pre-allocated for its structural findings.** ⚠️ **Both gates are relayed from other threads and are NOT readable in this tree** — recorded as sequencing, not as status. **The label swap already landed and is not this**: `a1995f9` moved the install switcher on all five screens to *"v0.1: BookOrbit, Prowlarr"*, while `mockups/README.md`'s v0.1 figures were **deliberately left un-re-keyed** — they are arithmetically derived from the Kavita-era install, and that section now **states its provenance where a reader meets the numbers** rather than letting a BookOrbit label sit over Kavita-shaped arithmetic. Re-deriving them changes what the drawings assert about a real install, which is why it is a decision rather than a rename. ⚠️ **[ADR-0052](./DECISIONS.md#adr-0052) marks its own mockup re-draw DISCHARGED by that commit** (`cad0563`), and that mark is about **the rendered label**, not the figures under it — reading it as covering both is the mistake this row exists to prevent. ⚠️ **No ADR number is cited: none is allocated** — `0053` is the highest on `main`. ⚠️ **The design thread is CLOSED, so this has NO OWNER at slotting time**, and naming one is part of slotting it: **a closed thread's sections do not pass to whoever next touches them.** |
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

**The importer, stream and UI plumbing is source-agnostic, and the Kavita adapter stays either way** —
see the blocked table above, where that is now its own row.

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

- **Deferred, wanted later, each with the seam that keeps it cheap:** [`FUTURE.md`](./FUTURE.md).
- **Permanent refusals:** [`ARCHITECTURE.md`](./ARCHITECTURE.md) §1.4 and §16's *Explicitly never*.
  Do not propose them and do not reopen them.
- **Which milestone owns a thing:** §16. Not this file.
