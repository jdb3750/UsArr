package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Search over your own library — ARCHITECTURE.md §8.2, docs/reference/search.md
// §4. This is the read behind GET /api/v1/search.
//
// IT IS A LOCAL READ AND NOTHING ELSE (principle 1). Two SQLite statements
// against the local file: one that resolves which libraries the caller can
// search, and one that retrieves, fuses and hydrates. No upstream call, no
// metadata provider, no image fetch. ARCHITECTURE.md §13 budgets this
// endpoint at p50 < 15 ms and p99 < 50 ms, which is the reason the Prowlarr
// fan-out was moved off /api/v1/search and onto /api/v1/releases/search: that
// one cannot meet this budget by construction, and this one has to.
//
// WHAT IT DOES NOT DO, said here so nobody infers it from silence:
//
//   - No grouping. Linked works render as separate rows. §4's grouped card is
//     derived from work_relation edges and is a presentation layer above this
//     one. ⚠️ work_relation IS DESIGNED TO CARRY the confidence and evidence
//     columns that card needs — the DDL is docs/reference/schema.md §11
//     "Cross-media edges · v0.3", which is the design of record — AND THE TABLE
//     DOES NOT EXIST. It is deferred to v0.3, no shipped migration creates it,
//     and TestDeferredTablesAreAbsent (internal/db/migrate_test.go) fails the
//     build if one does. This bullet used to read "work_relation carries the
//     confidence and evidence columns for it and nothing reads them yet", which
//     is a comment in the search path claiming a table the search path could
//     not see: there is nothing to read, not merely no reader.
//   - No "not in your library" section. That is the release search, on its own
//     endpoint, over a different corpus.
//   - No paging. The candidate set is capped at retrievalLimit by construction
//     and the re-rank is over the whole of it, so there is no keyset position a
//     cursor could name. A caller asks for up to SearchMaxLimit rows and that is
//     every row there is.

// The bm25 column weights. THIS COMMIT OWNS THEM: catalogue.go's creditedNames
// says in as many words that "search.md §4's fusion query can down-weight it
// when retrieval lands; today that query passes no weights at all because
// nothing queries either FTS table yet". This is retrieval landing.
//
// WHAT A WEIGHT DOES AND DOES NOT DO. fts5's bm25(tbl, w1…wN) takes one weight
// per column and scales that column's contribution; the result is negative, and
// more negative is a better match, which is why every leg orders ASCENDING.
// The weights therefore decide the ORDER WITHIN ONE LEG and nothing else — RRF
// (fuseRRF) throws the magnitudes away and keeps only each row's position, so a
// weight can never leak across legs or outvote the other engine. Getting one
// wrong reorders a leg; it cannot corrupt the fusion.
//
// THE FIVE, AND THE ARGUMENT FOR EACH:
//
//	title          10  The field the user is almost always typing. Everything
//	                   else is relative to it.
//	original_title  6  A real title of the same work in another language or
//	                   script, so it is title-grade evidence — below `title`
//	                   only because `title` is what the result row renders, so a
//	                   tie should resolve toward the string the user will see.
//	alt_titles      4  Also real titles, but the column is a newline-joined BLOB
//	                   of every alternate a source reported. Length dilutes bm25
//	                   term frequency on its own; the lower weight says the
//	                   evidence is genuinely weaker, not just longer.
//	people          2  "Susanna Clarke" → Piranesi is a query this column exists
//	                   to answer (LS-100), and it must retrieve. But it holds
//	                   EVERY credited name across all eighteen roles — on a
//	                   comics-first library that is pencillers, letterers and
//	                   cover artists — so a hit here is much weaker evidence of
//	                   "this is the thing I typed" than a hit on a title. This
//	                   is the down-weight creditedNames asked for by name.
//	overview      0.5  A plot synopsis that happens to contain your word is the
//	                   weakest evidence in the document and the longest field.
//
// ⚠️ THE overview WEIGHT IS UNTESTABLE AGAINST REAL DATA TODAY. Kavita is the
// only catalogue adapter and work.overview is empty for everything it writes,
// so the column is uniformly ” in the corpus this project can actually build.
// The number is reasoned, not measured, and it is marked so rather than left to
// look like it was tuned.
const (
	bm25WeightTitle         = 10.0
	bm25WeightOriginalTitle = 6.0
	bm25WeightAltTitles     = 4.0
	bm25WeightPeople        = 2.0
	bm25WeightOverview      = 0.5
)

// rrfK is §4's Reciprocal Rank Fusion constant, k = 60 — the conventional value
// from Cormack, Clarke and Buettcher (2009), and the one §4 names.
//
// RRF fuses on RANK, never on score, and that is the load-bearing property: bm25
// over a unicode61 index and bm25 over a trigram index are not on a common
// scale, so adding or comparing them directly would be arithmetic on two
// different units. k damps the head — the difference between rank 1 and rank 2
// is 1/61 − 1/62, small — so a document both legs merely liked outranks one a
// single leg loved, which is the behaviour a hybrid retriever is for.
const rrfK = 60.0

// retrievalLimit is §4's LIMIT 200 per leg, and also the cap on the fused
// candidate set. It is what bounds the OR-expansion in §3 step 3: a two-token OR
// query can match a large fraction of the corpus, and the re-rank is only
// sub-millisecond because it sees at most this many short strings.
const retrievalLimit = 200

// Bounds on SearchLibrary's page.
//
// There is no paging (see the file comment), so SearchMaxLimit is the largest
// answer this endpoint can give, not the largest first page. 25 is the default
// because a search result list is read top-down and abandoned early, unlike
// Home's Block C inventory table whose default is 50.
const (
	SearchDefaultLimit = 25
	SearchMaxLimit     = 100
)

// SearchHit is one row of the library search result.
//
// It is deliberately NOT a whole work record, for §4.5's reason ("never load
// full item records") and for the same allowlist reason RecentWork is not:
// overview, remote_path, content_hash and size_on_disk are absent, and
// remote_path above all is a filesystem path on somebody else's box.
//
// THE RENDERING FIELDS ARE RecentWork'S FIELDS, ON PURPOSE. A search result row
// and a recently-added row render the same way — Type, title, year, the Have
// grammar — so a client can reuse one row component. Nothing is shared in the
// type system yet because the two reads have different keys and a premature
// common struct would fix the wrong thing.
//
// Score is the ONE field RecentWork has no analogue for, and it cannot have one:
// Home's list is ordered by date, so there is no relevance for it to carry.
type SearchHit struct {
	ID int64

	// Kind is work.kind, and MediaType is §17.2's six-value navigation enum
	// derived from it.
	//
	// ⚠️ IT IS work.kind, NOT search_doc.kind, AND THE TWO CAN DIVERGE.
	// catalogue.go's readSearchDocText records the divergence and its cause: the
	// item pass writes the container's kind into search_doc but its UPDATE of an
	// existing work does not set work.kind, so a container an operator retyped
	// upstream leaves the two disagreeing. work.kind is the one taken here for
	// the same reason readSearchDocText takes it — it is what the REST of the
	// replica believes (the subtype tables, recent.go's queries, the item
	// route), so a result row that named the other one would link to a screen
	// built on this one. search_doc.kind stays what it is: the corpus's own
	// record of what it indexed, and the writer's exclusion list is enforced
	// against it.
	Kind      string
	MediaType string

	Title   string
	Year    sql.NullInt64
	AddedAt sql.NullString

	HaveCount    int64
	WantCount    int64
	Availability sql.NullString

	// Score is rerank's output for this hit: the weighted sum of the three live
	// signals, in (0,1]. It is written by rerank and by nothing else, so a hit
	// that never went through the re-rank carries the zero value.
	//
	// IT IS A PROPERTY OF ONE ANSWER, NOT OF THE WORK. Two of its three
	// components are normalised over THE CANDIDATE SET — `rrf` against the best
	// rrf in the set, recency against the set's added_at order — so the same
	// work scores differently under a different query, and under the same query
	// against a different corpus or a different access scope. Nothing may
	// persist it, cache it across queries, or compare two of them that did not
	// come out of the same call. docs/reference/http-api.md §6.2.1 is the wire
	// contract that says this to a consumer, which is where a consumer will
	// look.
	//
	// ⚠️ IT IS NOT THE ORDER. rerank sorts by it and then diversify PROMOTES
	// rows past better-scoring ones, so the returned slice is deliberately not
	// score-descending. The order is the answer; this number explains it and
	// does not reproduce it.
	Score float64
}

// SearchResults is one answer, plus the one fact about the query the caller has
// to be able to tell the user.
type SearchResults struct {
	Hits []SearchHit

	// Truncated says §3 step 1's twelve-token cap fired, so the results answer
	// a prefix of what was typed. §3 requires a UI note; a note the server never
	// sends cannot be rendered.
	Truncated bool
}

// retrievalLeg is ONE retrieval engine's contribution: a ranked list of
// search_doc rowids.
//
// THIS IS THE SEAM ARCHITECTURE.md §8.2 REQUIRES, and the requirement is stated
// as a negative: ranking must never learn which engine produced a candidate. So
// the structure is N legs in, one fused candidate list out, and fuseRRF is the
// ONLY function that ever sees the association between a rowid and the leg it
// came from — it consumes provenance and does not emit it. rerank takes
// []searchCandidate, which has no field naming a leg and no field a leg could
// smuggle one into.
//
// Why the negative matters rather than being tidiness: the moment ranking can
// ask "did the trigram leg find this?", every future weight becomes a weight on
// an ENGINE instead of on the evidence, the two legs stop being interchangeable,
// and adding a third (an embedding leg, an external index — docs/FUTURE.md's
// Meilisearch) stops being a matter of appending to this slice. CLAUDE.md's rule
// is that the seam ships and the feature does not: today there are exactly two
// legs, both fts5, and nothing speculative sits on top of this.
//
// `name` exists for the query-plan guard and for error messages ONLY. It is
// never an input to fusion or to ranking, and TestRankingIsBlindToProvenance is
// what holds that.
type retrievalLeg struct {
	name string

	// cte is the leg's SQL body: it must project exactly (doc_rowid, rnk), with
	// rnk a 1-based rank, best first.
	cte string

	// args are the leg's bound parameters, in the order cte names them.
	args []any
}

// keywordLeg is the unicode61 leg: search_fts, five columns, diacritics folded.
//
// ⚠️ THE TWO FTS TABLES TOKENISE DIFFERENTLY ON PURPOSE AND THAT IS NOT A BUG TO
// FIX. search_fts is `unicode61 remove_diacritics 2`, so `amelie` finds `Amélie`
// here; search_trgm is `trigram`, which folds nothing, so the same query misses
// there. The legs disagree, RRF fuses what each found, and the union is a better
// answer than either — which is the entire argument for running two.
func keywordLeg(match string, scopeSQL string, scopeArgs []any) retrievalLeg {
	args := []any{match}
	args = append(args, scopeArgs...)
	return retrievalLeg{
		name: "search_fts",
		cte: `
      SELECT f.rowid AS doc_rowid,
             ROW_NUMBER() OVER (ORDER BY rank) AS rnk
        FROM search_fts f
       WHERE search_fts MATCH ?
         AND rank MATCH '` + bm25Rank(
			bm25WeightTitle, bm25WeightOriginalTitle, bm25WeightAltTitles,
			bm25WeightPeople, bm25WeightOverview) + `'
         AND ` + scopeSQL + `
       ORDER BY rank
       LIMIT ` + strconv.Itoa(retrievalLimit),
		args: args,
	}
}

// trigramLeg is the substring leg: search_trgm, two columns, no folding, and a
// three-character floor its tokenizer imposes (§3 step 5).
func trigramLeg(match string, scopeSQL string, scopeArgs []any) retrievalLeg {
	args := []any{match}
	args = append(args, scopeArgs...)
	return retrievalLeg{
		name: "search_trgm",
		cte: `
      SELECT t.rowid AS doc_rowid,
             ROW_NUMBER() OVER (ORDER BY rank) AS rnk
        FROM search_trgm t
       WHERE search_trgm MATCH ?
         AND rank MATCH '` + bm25Rank(bm25WeightTitle, bm25WeightAltTitles) + `'
         AND ` + scopeSQL + `
       ORDER BY rank
       LIMIT ` + strconv.Itoa(retrievalLimit),
		args: args,
	}
}

// bm25Rank renders the `bm25(w1, w2, …)` string that configures a leg's ranking
// function, for fts5's `rank MATCH` form.
//
// # WHY `rank MATCH 'bm25(…)'` AND NOT `ORDER BY bm25(search_fts, …)`
//
// The two compute the same order. Only one of them lets fts5 do it. MEASURED on
// this schema, SQLite 3.53.4, through EXPLAIN QUERY PLAN on the shipped
// statement:
//
//	ORDER BY bm25(search_fts, 10, 6, 4, 2, 0.5)
//	  →  SCAN f VIRTUAL TABLE INDEX 0:M5
//	     USE TEMP B-TREE FOR ORDER BY
//
//	rank MATCH 'bm25(10, 6, 4, 2, 0.5)' … ORDER BY rank
//	  →  SCAN f VIRTUAL TABLE INDEX 32:rM5
//
// The first materialises and sorts EVERY row the MATCH found before the LIMIT
// can cut it, and §3 step 3's OR operator is what makes that set large on
// purpose. The second pushes the ordering into the virtual table, which ranks
// internally and streams — the `r` in `32:rM5` is fts5 reporting that it, not
// SQLite, is producing the order. On a p50 < 15 ms budget that is the difference
// between sorting the match set and sorting nothing.
//
// THE WEIGHTS ARE FORMATTED INTO SQL RATHER THAN BOUND, and that is forced:
// fts5 parses the rank string itself, so a bound parameter inside it is text
// fts5 never sees. They are compile-time float constants declared in this file
// and nothing user-supplied reaches this function, so there is no injection
// surface; the MATCH argument, which IS user-supplied, is bound as a parameter
// and is quoted by ftsQuote besides.
func bm25Rank(weights ...float64) string {
	var b strings.Builder
	b.WriteString("bm25(")
	for i, w := range weights {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.FormatFloat(w, 'f', -1, 64))
	}
	b.WriteString(")")
	return b.String()
}

// searchScopePredicate renders the LIBRARY half of the access scope: a semi-join
// from a doc rowid to the libraries this caller may search.
//
// # DECISION 1 — UNFILED (library.id = 0) IS INCLUDED, AND IS NEVER A CHIP
//
// This is the trap in this feature and it is worth the paragraph. §17.2's scope
// chip is a multi-select over the user's own libraries, and migration 0005 says
// of library 0: "Never listed on the Libraries screen, never offered in the
// scope chip, never proposed" — listLibrariesSQL excludes it in the WHERE
// clause for exactly that reason. If THIS query derived its library list from
// that same chip-offerable set, library 0 would be excluded from it too, and
// every unfiled document would silently vanish from search: no error, no empty
// state, just works that cannot be found. That is precisely the disappearance
// the Unfiled row was created to prevent — migration 0005's own ⚠️ reads "a row
// visible through no library matches no scope and disappears from search for
// every user including the owner". Re-introducing it through the query rather
// than through the data would be the same bug with a better hiding place.
//
// So the list is derived from the `library` TABLE, filtered on the two columns
// below, and it is NOT filtered on `id <> 0`. The exclusion belongs to the
// Libraries screen, which renders libraries; it does not belong to search,
// which searches documents.
//
// # DECISION 2 — library.enabled AND library.include_in_search ARE HONOURED
//
// Both columns have been selected and served since migration 0005 and NOTHING
// has ever filtered on them; this is the first query that should, and
// include_in_search has no other possible reader — it names this query.
//
// THE TWO DECISIONS DO NOT CONFLICT, WHICH WAS CHECKED RATHER THAN ASSUMED.
// Migration 0005 seeds the reserved row as
// `INSERT INTO library (…, enabled, include_in_search) VALUES (0, 0, 'Unfiled',
// 'unfiled', 'movie', 'auto', 1, 1)` — both flags are 1 — so filtering on them
// keeps library 0 in. The two would only collide if an operator could turn
// Unfiled off, and no code path offers that: the row is not on the Libraries
// screen, so there is no toggle for it.
//
// # WHY THE USER FILTER IS userPredicate AND NOT `user_id = ?`
//
// library 0 carries user_id 0, the system sentinel. Scope.userPredicate renders
// `user_id IN (0, :uid)`, which is the canonical shape in this package and is
// what admits the sentinel row whatever id the owner account happens to have.
// A plain equality here would have excluded library 0 from every search the day
// the owner stopped being user 0 — the same disappearance as decision 1, by a
// different route.
//
// # IT IS A SEMI-JOIN, NOT A JOIN
//
// EXISTS rather than `JOIN search_doc_library … library_id IN (…)`, because a
// document filed in three visible libraries would otherwise appear three times
// inside the leg, inflating ROW_NUMBER() and handing the same rowid three
// different ranks to fuse. EXISTS stops at the first match. The plan is
// `SEARCH sdl USING COVERING INDEX ix_sdl_doc (doc_rowid=? AND library_id=?)`,
// which is schema.md §7 invariant 6's "doc set known" form: permission
// filtering INSIDE the index join, never after it, because a post-filter leaks
// existence through result counts and ranking positions.
func searchScopePredicate(docRowidColumn string, libraryIDs []int64) (string, []any) {
	// An empty visible set FAILS CLOSED. "No searchable libraries" must mean no
	// rows, never all rows — the same third branch workVisibilityPredicate and
	// libraryVisibilityPredicate have, for the same reason. It is reachable: a
	// user who disabled every library, or turned include_in_search off
	// everywhere, has genuinely asked for this.
	if len(libraryIDs) == 0 {
		return "1=0", nil
	}
	args := make([]any, 0, len(libraryIDs))
	for _, id := range libraryIDs {
		args = append(args, id)
	}
	return `EXISTS (SELECT 1 FROM search_doc_library sdl
	                 WHERE sdl.doc_rowid = ` + docRowidColumn + `
	                   AND sdl.library_id IN (` + placeholders(len(args)) + `))`, args
}

// searchableLibrariesSQL is the statement that resolves decision 1 and decision
// 2 into a list of ids. See searchScopePredicate for the whole argument.
//
// It is a SEPARATE statement rather than a subquery inside the fusion because
// the subquery form costs the plan: SQLite materialises it and the seek on
// search_doc_library loses its `library_id=?` constraint, degrading the
// permission filter from a covering index seek to a check applied per candidate.
// Two statements keep both plans flat and separately assertable, which is the
// same trade ListLibraries makes.
func searchableLibrariesSQL(scope Scope) (string, []any) {
	userPred, args := scope.userPredicate("user_id")
	return `SELECT id FROM library
	         WHERE ` + userPred + `
	           AND enabled = 1
	           AND include_in_search = 1
	         ORDER BY id`, args
}

// searchLibrarySQL renders the ONE statement that retrieves, fuses and hydrates.
//
// It is a function rather than a literal so the query-plan guard can EXPLAIN THE
// STATEMENT THAT SHIPS (docs/DEVELOPMENT.md §11 rule 1: a guard that asserts
// against a hand-copied lookalike is probing a proxy). TestSearchLibraryPlanIsSeeks
// renders it through here.
//
// # THE FUSION IS UNION ALL + GROUP BY, NOT §4'S FULL OUTER JOIN
//
// §4 writes the fusion as `FROM kw FULL OUTER JOIN tg USING (rowid)`. That is
// the same arithmetic — `COALESCE(1.0/(60+kw.rnk),0) + COALESCE(1.0/(60+tg.rnk),0)`
// is `SUM(1.0/(60+rnk))` over the union — but it hardcodes EXACTLY TWO legs into
// the SQL's shape, which is the one thing §8.2's seam forbids: a third leg would
// need a second FULL OUTER JOIN and a third COALESCE term, i.e. a rewrite rather
// than an append. UNION ALL over a generated list of CTEs is N-leg generic, so
// retrievalLeg is a slice and this function iterates it. The result set and the
// scores are identical; only the shape generalises.
//
// # BOTH SCOPING MECHANISMS ARE HERE, AND NO OTHER QUERY IN THIS PACKAGE CARRIES
// # BOTH
//
// Every other scoped read needs one of the two. This one needs both, because the
// two answer different questions:
//
//   - LIBRARY visibility (searchScopePredicate, inside each leg) is what
//     schema.md §7 requires: filter in the index join, per §8.2, via
//     search_doc_library. It answers "may this caller see this DOCUMENT".
//   - INSTANCE visibility (workVisibilityPredicate, on the hydration join) is
//     store.go rule 2's: a work is reachable through the instances that
//     replicated it, and a result computed across instances the caller cannot
//     see is an existence oracle. It answers "may this caller see this WORK".
//
// A library can be a subset of an instance (ADR-0026) and an instance can hold
// works in no library at all, so neither predicate implies the other and
// dropping either one opens a different hole. Under v0.1's owner scope the
// instance half renders `1=1` and costs nothing, which is why it is cheap to
// keep correct.
//
// # WHY THE HYDRATION JOINS `work`
//
// search_doc has no tombstone column and nothing deletes a doc when a work is
// SOFT deleted — the foreign key cascades on a hard delete only. Without
// `w.deleted_at IS NULL` the seven-day tombstone window would be searchable,
// which is the same degradation schema.md §1 says the partial indexes exist to
// prevent. The join is also where title, year and the Have-column rollups come
// from: search_doc stores only norm_title, because the FTS tables are
// contentless and cannot be read back.
func searchLibrarySQL(scope Scope, legs []retrievalLeg, limit int) (string, []any) {
	if limit <= 0 || limit > retrievalLimit {
		limit = retrievalLimit
	}

	args := make([]any, 0, 8)
	var ctes, unions []string
	for i, leg := range legs {
		name := "leg" + strconv.Itoa(i)
		ctes = append(ctes, name+" AS ("+leg.cte+"\n    )")
		unions = append(unions, "SELECT doc_rowid, rnk FROM "+name)
		args = append(args, leg.args...)
	}

	// The Ebooks/Audiobooks split, resolved server-side for §17.2's reason: the
	// Tier 1 client index ships no format, so a client cannot separate them.
	// Same correlated MIN() recentWorksSQL uses, so one row renders identically
	// on Home and in a search result.
	args = append(args, editionFormatAudiobook)

	instPred, instArgs := scope.workVisibilityPredicate("sd.work_id")
	args = append(args, instArgs...)
	args = append(args, limit)

	return `
    WITH ` + strings.Join(ctes, ",\n    ") + `,
    fused AS (
      SELECT doc_rowid, SUM(1.0 / (` + strconv.FormatFloat(rrfK, 'f', -1, 64) + ` + rnk)) AS rrf
        FROM (` + strings.Join(unions, "\n              UNION ALL ") + `)
       GROUP BY doc_rowid
    )
    SELECT sd.work_id, w.kind, w.title, w.year, w.added_at,
           w.have_count, w.want_count, w.availability,
           (SELECT MIN(e.format = ?) FROM edition e WHERE e.work_id = sd.work_id),
           sd.norm_title, fused.rrf
      FROM fused
      JOIN search_doc sd ON sd.rowid = fused.doc_rowid
      JOIN work w ON w.id = sd.work_id
     WHERE w.deleted_at IS NULL
       AND ` + instPred + `
     ORDER BY fused.rrf DESC, sd.work_id
     LIMIT ?`, args
}

// searchCandidate is one fused candidate as the re-rank sees it.
//
// ⚠️ THERE IS NO FIELD NAMING A LEG, AND THAT IS THE POINT — see retrievalLeg.
// `rrf` is the only trace of retrieval that survives fusion, and it is a single
// number that cannot be decomposed back into which engine contributed it.
type searchCandidate struct {
	hit       SearchHit
	rrf       float64
	normTitle string
}

// SearchLibrary answers one library search: docs/reference/search.md §3 and §4.
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL, both halves, and here it is in the
// SQL TWICE — see searchLibrarySQL's note on the two mechanisms. A scope a query
// accepts and never filters on is indistinguishable from no scope at all, which
// is why TestSearchLibraryScopeActuallyFilters breaks each predicate in turn and
// watches an assertion catch it.
//
// An empty or whitespace-only query is the caller's to reject; this returns no
// results for it rather than every row.
func (s *Store) SearchLibrary(
	ctx context.Context, scope Scope, query string, limit int,
) (SearchResults, error) {
	switch {
	case limit <= 0:
		limit = SearchDefaultLimit
	case limit > SearchMaxLimit:
		limit = SearchMaxLimit
	}

	q := buildFTSQueries(query)
	out := SearchResults{Hits: []SearchHit{}, Truncated: q.Truncated}
	if q.empty() {
		return out, nil
	}

	libraryIDs, err := s.searchableLibraries(ctx, scope)
	if err != nil {
		return out, err
	}
	scopeSQL, scopeArgs := searchScopePredicate("f.rowid", libraryIDs)

	legs := make([]retrievalLeg, 0, 2)
	if q.prefix != "" {
		legs = append(legs, keywordLeg(q.prefix, scopeSQL, scopeArgs))
	}
	if q.raw != "" {
		trgmSQL, trgmArgs := searchScopePredicate("t.rowid", libraryIDs)
		legs = append(legs, trigramLeg(q.raw, trgmSQL, trgmArgs))
	}
	if len(legs) == 0 {
		return out, nil
	}

	candidates, err := s.searchCandidates(ctx, scope, legs)
	if err != nil {
		return out, err
	}

	ranked := rerank(q.tokens, candidates)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out.Hits = ranked
	return out, nil
}

// searchableLibraries reads the library ids this caller may search.
func (s *Store) searchableLibraries(ctx context.Context, scope Scope) ([]int64, error) {
	query, args := searchableLibrariesSQL(scope)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search: read searchable libraries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("search: read searchable libraries: scan: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: read searchable libraries: %w", err)
	}
	return out, nil
}

// searchCandidates runs the fusion statement and hydrates its rows.
func (s *Store) searchCandidates(
	ctx context.Context, scope Scope, legs []retrievalLeg,
) ([]searchCandidate, error) {
	query, args := searchLibrarySQL(scope, legs, retrievalLimit)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		// The MATCH argument is constructed by buildFTSQueries and every token
		// in it is ftsQuote'd, so an fts5 syntax error here is a bug in that
		// function rather than in the user's typing — which is why the error is
		// wrapped plainly and the caller renders it as a 500, not a 400.
		return nil, fmt.Errorf("search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]searchCandidate, 0, retrievalLimit)
	for rows.Next() {
		var c searchCandidate
		var allAudiobook sql.NullInt64
		if err := rows.Scan(&c.hit.ID, &c.hit.Kind, &c.hit.Title, &c.hit.Year,
			&c.hit.AddedAt, &c.hit.HaveCount, &c.hit.WantCount, &c.hit.Availability,
			&allAudiobook, &c.normTitle, &c.rrf); err != nil {
			return nil, fmt.Errorf("search: scan: %w", err)
		}
		// mediaTypeOf returns "" for a kind §17.2's six-value enum has no row
		// for. Today that is 'game' alone, and nothing writes a game work — but
		// this does NOT error the way recent.go's does, because recent.go
		// constrains its own corpus to six kinds in the WHERE clause and can
		// therefore call an unmapped kind a contradiction. This query must not:
		// search.md §2 puts the corpus refusal at the WRITER
		// (corpusExcludedKinds), and a `kind IN (…)` allowlist here would be a
		// second vocabulary free to drift from it. So an unmapped kind travels
		// with an empty MediaType and the wire contract says the key is absent.
		c.hit.MediaType = mediaTypeOf(c.hit.Kind, allAudiobook)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return out, nil
}

// The re-rank weights. §4 names five signals; THREE OF THEM ARE CONSTANTS ON
// REAL DATA and this code says so instead of implying otherwise.
//
// WHAT §4 ASKS FOR, AND WHAT THE CORPUS ACTUALLY CONTAINS:
//
//	Jaro-Winkler on norm_title   VARIES.   Implemented.
//	Recency of added_at          VARIES.   Implemented.
//	popularity prior             CONSTANT. catalogue.go's writeSearchDoc inserts
//	                             `popularity` as a hardcoded 0 for every row,
//	                             with its own comment saying why: "Kavita
//	                             reports neither, and a fabricated ranking
//	                             signal is worse than an absent one". NOT
//	                             implemented — a term multiplied by a column
//	                             that is 0 everywhere is a term that does
//	                             nothing, and shipping it would make the code
//	                             read as if five signals worked.
//	title_idf penalty            CONSTANT. Same writer, same hardcoded 0, and
//	                             for a stronger reason: title_idf is a CORPUS
//	                             STATISTIC, and only a pass over the whole
//	                             corpus can compute one. Nothing computes it.
//	                             NOT implemented.
//	in_library boost             CONSTANT. writeSearchDoc inserts in_library = 1
//	                             for every row, and it cannot be otherwise:
//	                             search_doc exists only for works the replica
//	                             HAS. §4 calls it "the single most
//	                             user-satisfying signal" and it will be, on the
//	                             day the corpus also holds things you do not
//	                             own — which is the "not in your library"
//	                             section, a different endpoint over a different
//	                             corpus, not built. NOT implemented.
//
// The three columns stay in the schema and stay unread. Adding a term for one
// of them is a change HERE, in the same commit as the writer that starts
// computing it — never before, because a live-looking weight on a dead column
// is exactly the "no invented status" failure in code rather than in prose.
//
// ⚠️ JARO-WINKLER IS NOT PRIMARY HERE, AND §4 USED TO SAY IT SHOULD BE. The
// divergence was stated plainly rather than left quiet, and §4 has since
// absorbed it: that table no longer marks JW "primary", it names this const
// block as authoritative for the weights, and it carries this paragraph's
// reason in prose. This
// weights retrieval above JW, 0.55 to 0.35, because §4's original table predates
// the `people` column (LS-100 added it after §4 was written). Jaro-Winkler sees
// norm_title and NOTHING ELSE. Making it primary would therefore bury every hit
// retrieved through `people`, `alt_titles` or `original_title` — a search for
// "Susanna Clarke" scores JW≈0 against "Piranesi" — under any title that merely
// looks vaguely like the typed string. That is the exact query the people column
// was added to answer, so making JW primary would break the feature §4's own
// corpus section calls settled. RRF is the only signal that knows about the
// other four columns at all, so it leads; JW discriminates among what retrieval
// found, which is the job §4 wanted it for. REVIEW-LOG LS-191 carries this.
//
// The three numbers are CHOSEN, NOT TUNED. There is no relevance-judgement set
// to tune against and no query log, so calling them tuned would be the invented
// status this project forbids. They encode an ordering of confidence — retrieval
// evidence over string shape over recency — and that ordering is the claim.
const (
	rerankWeightRRF     = 0.55
	rerankWeightJW      = 0.35
	rerankWeightRecency = 0.10
)

// rerank scores the fused candidates and returns them best first.
//
// It is BLIND TO PROVENANCE by construction: its inputs are the typed tokens and
// []searchCandidate, and neither carries a leg. See retrievalLeg.
//
// The three components are each normalised to [0,1] over the candidate set, so
// the weights above are comparable to one another:
//
//   - rrf, divided by the best rrf in the set. Fusion scores are tiny (2/61 at
//     most) and their absolute size means nothing, so the ratio is the signal.
//   - jw, Jaro-Winkler between the typed tokens joined and norm_title. Already
//     in [0,1] by definition.
//   - recency, the candidate's position in added_at order, newest = 1. Rank
//     rather than elapsed time because it is a "mild tiebreak" (§4) and an
//     absolute decay would need a half-life nobody has measured. A candidate
//     with NO added_at scores 0: recent.go states the rule — an absent date must
//     never be able to claim the top — and it holds here for the same reason.
//
// The final sort breaks ties on work id, so the same corpus and the same query
// always produce the same order. An unstable order across identical requests
// makes every screenshot and every test flaky, and users read it as the list
// moving under them.
//
// THE SCORE IS PUBLISHED AND THE ORDER IS STILL THE ANSWER. Because every
// component is normalised over the candidate set rather than over the corpus,
// the number is meaningful ONLY among hits from one call — TestSearchScore*
// pins each half of that — and diversify then reorders past it, so the slice
// this returns is authoritative and the score is an explanation of it.
func rerank(tokens []string, candidates []searchCandidate) []SearchHit {
	if len(candidates) == 0 {
		return []SearchHit{}
	}

	typed := normalizeForCompare(strings.Join(tokens, " "))

	bestRRF := 0.0
	for _, c := range candidates {
		if c.rrf > bestRRF {
			bestRRF = c.rrf
		}
	}
	if bestRRF == 0 {
		bestRRF = 1
	}

	recency := recencyScores(candidates)

	// The score is written ONTO THE HIT rather than into a parallel struct the
	// function then throws away, because it leaves this package: SearchHit.Score
	// is what internal/httpapi publishes as §6.2.1's `score`. It rides the row
	// through the sort and through diversify's promotion, so a promoted row
	// arrives on the wire carrying the score it actually earned — which is the
	// whole reason a consumer can see that the order is not score order.
	scored := make([]SearchHit, len(candidates))
	for i, c := range candidates {
		jw := jaroWinkler(typed, normalizeForCompare(c.normTitle))
		scored[i] = c.hit
		scored[i].Score = rerankWeightRRF*(c.rrf/bestRRF) +
			rerankWeightJW*jw +
			rerankWeightRecency*recency[i]
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].ID < scored[j].ID
	})

	return diversify(scored)
}

// recencyScores maps each candidate onto [0,1] by its position in added_at
// order, newest first. A candidate with no added_at scores 0.
func recencyScores(candidates []searchCandidate) []float64 {
	dated := make([]int, 0, len(candidates))
	for i, c := range candidates {
		if c.hit.AddedAt.Valid {
			dated = append(dated, i)
		}
	}
	out := make([]float64, len(candidates))
	if len(dated) == 0 {
		return out
	}
	sort.SliceStable(dated, func(a, b int) bool {
		return candidates[dated[a]].hit.AddedAt.String > candidates[dated[b]].hit.AddedAt.String
	})
	// One dated candidate is the newest AND the oldest; it scores 1 rather than
	// dividing by zero.
	span := float64(len(dated) - 1)
	for pos, idx := range dated {
		if span == 0 {
			out[idx] = 1
			continue
		}
		out[idx] = 1 - float64(pos)/span
	}
	return out
}

// diversifyWindow is the window §4's diversity injection guarantees over: "the
// top 10 contains at least one item per media type that scored above a floor".
const diversifyWindow = 10

// diversify is §4's media-type diversity injection.
//
// WHAT IT IS FOR, in §4's own words: without it "whichever medium has better
// text statistics sweeps the list and the novella never appears, which is
// precisely the failure the feature exists to prevent". The flagship case is
// Train Dreams — a film and a novella of the same name — where the film's
// richer text (alt titles, a long credit list) outranks the book on every text
// signal and a purely text-ordered list shows ten films.
//
// The mechanism is a PROMOTION, not a re-score: the ranking above is left
// exactly as it is, and for each media type absent from the first
// diversifyWindow rows, that type's best-scoring row is moved up to the window's
// edge. Nothing is dropped and nothing outside the window is reordered relative
// to itself, so a caller asking for more rows sees the same list with the
// promoted rows lifted out of it.
//
// ⚠️ "NOT A RE-SCORE" IS EXACTLY WHY A CLIENT MUST NOT SORT BY SearchHit.Score.
// A promoted row keeps the score it earned, which is by construction lower than
// that of the rows it was moved above, so re-sorting the returned slice by score
// sends every promoted row back down and undoes this guarantee in full. The
// score is published (§6.2.1); the order is what a client renders.
//
// THE FLOOR §4 NAMES IS "ABOVE A FLOOR" AND IT IS NOT A NUMBER HERE. Being a
// fused candidate at all IS the floor: a row only exists in this list because at
// least one retrieval leg matched it against the typed tokens, and both legs are
// bounded to retrievalLimit by bm25 order. Inventing a second numeric threshold
// on top of a score whose scale is already normalised per query would be a
// magic number with nothing to calibrate it against — the same reason the three
// dead signals above are absent rather than approximated.
//
// ⚠️ IT IS A NO-OP ON THE CORPUS THIS PROJECT CAN BUILD TODAY. Kavita is the one
// catalogue adapter and it writes comics, so every candidate has the same media
// type and there is nothing to promote. It ships because it is §4's answer to
// the case the feature exists for, and it is tested against synthetic
// multi-type data rather than against an import.
func diversify(hits []SearchHit) []SearchHit {
	if len(hits) <= diversifyWindow {
		return hits
	}

	inWindow := make(map[string]bool, diversifyWindow)
	for _, h := range hits[:diversifyWindow] {
		if h.MediaType != "" {
			inWindow[h.MediaType] = true
		}
	}

	// The best-scoring row of each media type that the window missed, in the
	// order they appear below the window — so the strongest omitted type is
	// promoted first and the promotion order is itself deterministic.
	var promote []int
	seen := make(map[string]bool, len(inWindow))
	for i := diversifyWindow; i < len(hits); i++ {
		mt := hits[i].MediaType
		if mt == "" || inWindow[mt] || seen[mt] {
			continue
		}
		seen[mt] = true
		promote = append(promote, i)
	}
	if len(promote) == 0 {
		return hits
	}

	promoted := make(map[int]bool, len(promote))
	for _, i := range promote {
		promoted[i] = true
	}
	out := make([]SearchHit, 0, len(hits))
	// The promoted rows enter at the window's edge: the top of the list keeps
	// the strongest matches, and the tail of the window becomes the guarantee.
	head := diversifyWindow - len(promote)
	if head < 0 {
		head = 0
	}
	out = append(out, hits[:head]...)
	for _, i := range promote {
		out = append(out, hits[i])
	}
	for i := head; i < len(hits); i++ {
		if promoted[i] {
			continue
		}
		out = append(out, hits[i])
	}
	return out
}
