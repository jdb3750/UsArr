package store

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// ── the corpus these tests search ───────────────────────────────────────────
//
// It is built through ApplyCatalogueBatch — the real writer — rather than by
// INSERTing into search_doc directly, and that is the point rather than
// convenience. search_doc.rowid is THE allocator for search_fts and search_trgm,
// RRF fuses on it, and LS-01/G3 records that dropping the explicit rowid insert
// passed three tests before a fourth caught it. A test that hand-rolled its own
// documents would be fusing rowids it allocated itself and would never see that
// class of bug at all.

type searchFixture struct {
	store     *Store
	instance  int64
	libraries map[string]int64
}

// newSearchFixture builds a small multi-library, multi-kind corpus.
func newSearchFixture(t *testing.T) *searchFixture {
	t.Helper()
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		{RemoteID: "1", Name: "Manga", Kind: "comic"},
		{RemoteID: "2", Name: "Novels", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

	f := &searchFixture{store: s, instance: inst, libraries: map[string]int64{}}
	for remote, b := range binds {
		switch remote {
		case "1":
			f.libraries["manga"] = b.LibraryID
		case "2":
			f.libraries["novels"] = b.LibraryID
		}
	}
	return f
}

// searchItem builds one CatalogueItem with a REAL normalised title, because the
// re-rank compares the typed query against norm_title and the shared `item`
// helper lowercases instead of normalising.
func searchItem(remoteID, container, kind, title string) CatalogueItem {
	it := item(remoteID, container, kind, title)
	it.NormalizedTitle = NormalizeForSearch(title)
	return it
}

func (f *searchFixture) apply(t *testing.T, items ...CatalogueItem) {
	t.Helper()
	binds, _, err := f.store.BindContainers(t.Context(), f.instance, SystemUserID,
		[]CatalogueContainer{
			{RemoteID: "1", Name: "Manga", Kind: "comic"},
			{RemoteID: "2", Name: "Novels", Kind: "book"},
		})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if _, err := f.store.ApplyCatalogueBatch(t.Context(), f.instance, binds, items, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
}

func hitTitles(hits []SearchHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Title
	}
	return out
}

// execTest runs one throwaway statement on the writer, for the tests that have
// to put the database into a state the writer cannot reach on its own — a
// disabled library, a soft-deleted work, a stranded document.
func execTest(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	if err := s.write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	}); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func mustSearch(t *testing.T, s *Store, scope Scope, q string) SearchResults {
	t.Helper()
	res, err := s.SearchLibrary(t.Context(), scope, q, SearchDefaultLimit)
	if err != nil {
		t.Fatalf("SearchLibrary(%q): %v", q, err)
	}
	return res
}

// TestSearchLibraryFindsWhatWasImported is the end-to-end sanity floor: a work
// the real writer put in the corpus comes back for the text of its own title.
//
// It also covers the case that used to be a 500 rather than a miss —
// `Fallout: New Vegas`, where an unescaped colon is an FTS5 column filter. See
// TestEveryConstructedQueryIsAcceptedByFTS5 for the isolated form.
func TestSearchLibraryFindsWhatWasImported(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t,
		searchItem("1", "1", "comic", "Frieren: Beyond Journey's End"),
		searchItem("2", "1", "comic", "Chainsaw Man"),
		searchItem("3", "2", "book", "Fallout: New Vegas"),
	)
	scope := OwnerScope(SystemUserID)

	for _, tc := range []struct{ query, want string }{
		{"frieren", "Frieren: Beyond Journey's End"},
		{"chainsaw", "Chainsaw Man"},
		{"Fallout: New Vegas", "Fallout: New Vegas"},
		{"fallout new", "Fallout: New Vegas"},
	} {
		res := mustSearch(t, f.store, scope, tc.query)
		if len(res.Hits) == 0 {
			t.Errorf("%q found nothing", tc.query)
			continue
		}
		if res.Hits[0].Title != tc.want {
			t.Errorf("%q ranked %q first, want %q (all: %q)",
				tc.query, res.Hits[0].Title, tc.want, hitTitles(res.Hits))
		}
	}
}

// TestUnfiledIsSearchableAndIsNotAChip is DECISION 1, and it is the trap this
// whole feature has.
//
// Migration 0005 reserves library 0, "Unfiled", as the landing place for a work
// that belongs to no user library, precisely so that §7 invariant 5 holds and
// no document is visible through nothing. Its own comment also says the row is
// "never offered in the scope chip", and listLibrariesSQL therefore excludes it
// in the WHERE clause.
//
// Those two facts point in opposite directions, and the wrong resolution is
// invisible: if this query derived its library list from the CHIP-OFFERABLE set
// — the obvious thing to reuse, since listLibrariesSQL is right there — library
// 0 would be excluded from search too, and every unfiled document would silently
// vanish. No error, no empty state; works that simply cannot be found. That is
// the exact disappearance the Unfiled row exists to prevent, reintroduced
// through the query instead of through the data.
//
// So: unfiled documents ARE searchable, and this test is what says so.
func TestUnfiledIsSearchableAndIsNotAChip(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t, searchItem("1", "1", "comic", "Berserk"))
	scope := OwnerScope(SystemUserID)

	// Move the document into Unfiled the way a library delete would: strip its
	// membership and re-file it. writeSearchDoc's own invariant-5 branch does
	// this, so this is the state the writer produces, not an invented one.
	execTest(t, f.store,
		`DELETE FROM search_doc_library WHERE library_id <> ?`, UnfiledLibraryID)
	execTest(t, f.store,
		`INSERT OR IGNORE INTO search_doc_library (library_id, doc_rowid)
		   SELECT ?, rowid FROM search_doc`, UnfiledLibraryID)

	res := mustSearch(t, f.store, scope, "berserk")
	if len(res.Hits) != 1 {
		t.Fatalf("an UNFILED document was not found: hits = %q.\n"+
			"This is the disappearance library 0 exists to prevent. If the "+
			"searchable-library list was derived from the chip-offerable set "+
			"(listLibrariesSQL excludes library 0 by design), every unfiled "+
			"work is now unfindable and nothing says so.", hitTitles(res.Hits))
	}

	// And the searchable set genuinely contains library 0, which is the
	// mechanism rather than the symptom.
	ids, err := f.store.searchableLibraries(t.Context(), scope)
	if err != nil {
		t.Fatalf("searchableLibraries: %v", err)
	}
	var hasUnfiled bool
	for _, id := range ids {
		if id == UnfiledLibraryID {
			hasUnfiled = true
		}
	}
	if !hasUnfiled {
		t.Errorf("searchableLibraries = %v, which omits library %d (Unfiled)", ids, UnfiledLibraryID)
	}
}

// TestSearchHonoursEnabledAndIncludeInSearch is DECISION 2. Both columns have
// been served since migration 0005 and nothing has ever filtered on them; this
// is the first query that does, and `include_in_search` has no other possible
// reader — it names this query.
//
// It also pins the half of decision 1 that had to be CHECKED rather than
// assumed: the two decisions do not conflict, because migration 0005 seeds
// library 0 with enabled = 1 AND include_in_search = 1, so honouring the flags
// keeps Unfiled in.
func TestSearchHonoursEnabledAndIncludeInSearch(t *testing.T) {
	scope := OwnerScope(SystemUserID)

	for _, tc := range []struct {
		name   string
		column string
	}{
		{"a disabled library is not searched", "enabled"},
		{"include_in_search = 0 is honoured", "include_in_search"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSearchFixture(t)
			f.apply(t, searchItem("1", "1", "comic", "Vinland Saga"))

			if got := len(mustSearch(t, f.store, scope, "vinland").Hits); got != 1 {
				t.Fatalf("baseline: %d hits, want 1", got)
			}
			//nolint:gosec // G202: tc.column is one of two literals in this table, never input.
			execTest(t, f.store, `UPDATE library SET `+tc.column+` = 0 WHERE id <> ?`, UnfiledLibraryID)
			if got := mustSearch(t, f.store, scope, "vinland").Hits; len(got) != 0 {
				t.Errorf("after %s = 0 the work is still searchable: %q", tc.column, hitTitles(got))
			}
		})
	}

	t.Run("the two flags do not exclude Unfiled", func(t *testing.T) {
		f := newSearchFixture(t)
		var enabled, include int
		if err := f.store.db.Read().QueryRowContext(t.Context(),
			`SELECT enabled, include_in_search FROM library WHERE id = ?`,
			UnfiledLibraryID).Scan(&enabled, &include); err != nil {
			t.Fatalf("read library 0: %v", err)
		}
		if enabled != 1 || include != 1 {
			t.Fatalf("library %d has enabled=%d include_in_search=%d.\n"+
				"Decisions 1 and 2 now CONFLICT: honouring the flags excludes "+
				"Unfiled, which makes every unfiled document unsearchable. Stop "+
				"and re-decide rather than picking one.", UnfiledLibraryID, enabled, include)
		}
	})
}

// TestSearchLibraryScopeActuallyFilters breaks EACH of the two scoping
// mechanisms in turn and watches an assertion catch it.
//
// A scope a query accepts and never filters on is indistinguishable from no
// scope at all. This query is the only one in the package that carries BOTH
// mechanisms — the search_doc_library semi-join for LIBRARY visibility and the
// service_item_link EXISTS for INSTANCE visibility — so it is the only one where
// "the scope is threaded" can be true of one half and false of the other.
func TestSearchLibraryScopeActuallyFilters(t *testing.T) {
	t.Run("the library half fails closed on an empty set", func(t *testing.T) {
		pred, args := searchScopePredicate("f.rowid", nil)
		if pred != "1=0" {
			t.Errorf("an empty searchable-library set renders %q, want %q — "+
				"\"no visible libraries\" must mean no rows, never all rows", pred, "1=0")
		}
		if len(args) != 0 {
			t.Errorf("the fail-closed predicate binds %d args, want none", len(args))
		}
	})

	t.Run("the library half is in the SQL, not only in the signature", func(t *testing.T) {
		pred, args := searchScopePredicate("f.rowid", []int64{0, 7})
		if !strings.Contains(pred, "search_doc_library") ||
			!strings.Contains(pred, "sdl.library_id IN (") {
			t.Errorf("the library predicate does not constrain search_doc_library:\n  %s", pred)
		}
		if len(args) != 2 {
			t.Errorf("the predicate binds %d args for two libraries", len(args))
		}
	})

	t.Run("the instance half renders all three branches", func(t *testing.T) {
		legs := []retrievalLeg{keywordLeg(`"x"*`, "1=1", nil)}
		for _, tc := range []struct {
			name  string
			scope Scope
			want  string
		}{
			{"owner", OwnerScope(SystemUserID), "1=1"},
			{"nothing visible", Scope{UserID: 1}, "1=0"},
			{"partial", Scope{UserID: 1, InstanceIDs: []int64{4}}, "service_item_link"},
		} {
			sqlText, _ := searchLibrarySQL(tc.scope, legs, retrievalLimit)
			if !strings.Contains(sqlText, tc.want) {
				t.Errorf("%s scope: the statement does not contain %q:\n%s", tc.name, tc.want, sqlText)
			}
		}
	})

	t.Run("a scope that sees no instance sees no results", func(t *testing.T) {
		f := newSearchFixture(t)
		f.apply(t, searchItem("1", "1", "comic", "Blame!"))

		if got := len(mustSearch(t, f.store, OwnerScope(SystemUserID), "blame").Hits); got != 1 {
			t.Fatalf("baseline: %d hits, want 1", got)
		}
		// AllInstances false with an empty visible set: sees nothing.
		if got := mustSearch(t, f.store, Scope{UserID: 9}, "blame").Hits; len(got) != 0 {
			t.Errorf("a scope with no visible instances found %q", hitTitles(got))
		}
	})
}

// TestSearchLibraryPlanIsSeeks is the query-plan guard, and it EXPLAINS THE
// STATEMENT THAT SHIPS — rendered through searchLibrarySQL, the same function
// SearchLibrary calls (docs/DEVELOPMENT.md §11 rule 1: a guard that asserts
// against a hand-copied lookalike is probing a proxy).
//
// ⚠️ IT LIVES HERE RATHER THAN IN internal/db, AND THAT IS FORCED. The brief for
// this commit asked for internal/db's TestScopedSearchIsASeekNotAScan to be
// extended to explain the shipped statement. It cannot be: internal/store
// imports internal/db, so internal/db cannot call searchLibrarySQL without an
// import cycle. internal/db's test keeps doing what it does — pinning the bare
// index property against hand-written shapes — and carries a pointer to this
// one, which is exactly the split the "recently added" case in that file already
// documents for recentWorksSQL.
//
// WHAT IS PINNED, AND WHAT IS DELIBERATELY NOT:
//
//   - The permission filter is a SEEK on search_doc_library with BOTH columns
//     constrained. That is schema.md §7 invariant 6's substance: filtering
//     happens IN the index join, because a post-filter breaks keyset page sizes
//     and leaks existence through result counts and ranking positions.
//   - Neither FTS leg sorts. `rank MATCH 'bm25(…)'` pushes the ordering into the
//     virtual table; `ORDER BY bm25(tbl, …)` would not, and would materialise
//     and sort every row the MATCH found before the LIMIT could cut it. §3 step
//     3's OR operator is what makes that set large on purpose. See bm25Rank.
//   - The hydration reaches search_doc and work by INTEGER PRIMARY KEY.
//   - The two temp B-trees that REMAIN are asserted as expected rather than
//     forbidden: the GROUP BY that performs the fusion, and the ORDER BY on the
//     fused score. Both are over at most retrievalLimit × len(legs) rows, and
//     neither can be indexed — `rrf` is computed. Forbidding them would be
//     asserting something untrue and would fail on the first correct change.
func TestSearchLibraryPlanIsSeeks(t *testing.T) {
	d := openStorePlanDB(t)

	q := buildFTSQueries("train dreams")
	kwScope, kwArgs := searchScopePredicate("f.rowid", []int64{0, 1})
	tgScope, tgArgs := searchScopePredicate("t.rowid", []int64{0, 1})
	legs := []retrievalLeg{
		keywordLeg(q.prefix, kwScope, kwArgs),
		trigramLeg(q.raw, tgScope, tgArgs),
	}
	query, args := searchLibrarySQL(OwnerScope(SystemUserID), legs, retrievalLimit)

	plan, err := db.QueryPlan(t.Context(), d.Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")

	// Both legs seek the junction, on both of its key columns.
	if n := strings.Count(joined, "SEARCH sdl EXISTS USING COVERING INDEX ix_sdl_doc "+
		"(doc_rowid=? AND library_id=?)"); n != len(legs) {
		t.Errorf("the permission filter is a covering seek on %d of %d legs:\n  %s\n"+
			"schema.md §7 invariant 6: permission filtering must happen IN the "+
			"index join. A scan here leaks existence through result counts and "+
			"ranking positions.", n, len(legs), strings.Join(plan, "\n  "))
	}
	if strings.Contains(joined, "SCAN sdl") || strings.Contains(joined, "SCAN search_doc_library") {
		t.Errorf("the scoped search SCANS search_doc_library:\n  %s", strings.Join(plan, "\n  "))
	}

	// Neither FTS leg sorts: fts5 produces the order itself. The `r` in
	// `32:rM5` is fts5 reporting that.
	for _, want := range []string{
		"SCAN f VIRTUAL TABLE INDEX 32:r",
		"SCAN t VIRTUAL TABLE INDEX 32:r",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("a retrieval leg is not ranked inside fts5 (%q absent):\n  %s\n"+
				"That is the `rank MATCH 'bm25(…)'` form regressing to `ORDER BY "+
				"bm25(tbl, …)`, which materialises and sorts the whole match set "+
				"before LIMIT. See bm25Rank for the measurement.", want, strings.Join(plan, "\n  "))
		}
	}

	for _, want := range []string{
		"SEARCH sd USING INTEGER PRIMARY KEY (rowid=?)",
		"SEARCH w USING INTEGER PRIMARY KEY (rowid=?)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("hydration is not a primary-key seek (%q absent):\n  %s",
				want, strings.Join(plan, "\n  "))
		}
	}

	// The two sorts that are EXPECTED, asserted as expected. If either
	// disappears the query changed shape and this guard should be re-read
	// rather than silently keep passing.
	if n := strings.Count(joined, "USE TEMP B-TREE"); n != 2 {
		t.Errorf("the plan has %d temp B-trees, want exactly 2 (the fusion GROUP BY "+
			"and the ORDER BY on the computed rrf, both over at most %d rows):\n  %s",
			n, retrievalLimit*len(legs), strings.Join(plan, "\n  "))
	}
}

// TestSearchableLibrariesPlanIsASeek pins the second statement. It is small —
// a homelab has single-digit libraries — but it runs on every search, and
// ix_library_kind is partial on `enabled = 1`, which is exactly the flag
// decision 2 filters on.
func TestSearchableLibrariesPlanIsASeek(t *testing.T) {
	d := openStorePlanDB(t)
	query, args := searchableLibrariesSQL(OwnerScope(SystemUserID))
	plan, err := db.QueryPlan(t.Context(), d.Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")
	if !strings.Contains(joined, "SEARCH library USING INDEX ix_library_kind") {
		t.Errorf("the searchable-library read does not seek ix_library_kind:\n  %s",
			strings.Join(plan, "\n  "))
	}
}

func openStorePlanDB(t *testing.T) *db.DB {
	t.Helper()
	return newTestStore(t).db
}

// TestRRFFusesOnRowidAndTheRowidsAgree is the LS-01/G3 trap, asserted for the
// two legs this commit ships.
//
// search_doc.rowid is THE allocator for search_fts and search_trgm, and RRF
// fuses on it. If a writer ever drops an explicit rowid insert, SQLite allocates
// its own and the fusion silently joins UNRELATED documents — plausible-looking
// wrong results, the hardest class of bug to diagnose. LS-01/G3 records that
// exactly this passed three tests before a fourth caught it.
//
// THE RULE FOR ANYONE ADDING A LEG: add its rowid assertion here. A third
// retrieval engine that allocates its own ids fuses garbage on day one.
func TestRRFFusesOnRowidAndTheRowidsAgree(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t,
		searchItem("1", "1", "comic", "Frieren"),
		searchItem("2", "1", "comic", "Berserk"),
		searchItem("3", "2", "book", "Piranesi"),
	)

	// Every rowid in each FTS table is a search_doc rowid, and the counts agree
	// — schema.md §7 invariant 2, restated from the READER's side because that
	// is the side the fusion is on.
	for _, table := range []string{"search_fts", "search_trgm"} {
		orphans := count(t, f.store, `SELECT COUNT(*) FROM `+table+` x
		     WHERE NOT EXISTS (SELECT 1 FROM search_doc d WHERE d.rowid = x.rowid)`)
		if orphans != 0 {
			t.Errorf("%s holds %d rowid(s) that are not search_doc rowids. "+
				"RRF fuses on rowid, so these fuse UNRELATED documents.", table, orphans)
		}
	}

	// And the fused result names the right work: a query that matches on both
	// legs must return the work whose document matched, not a neighbour.
	res := mustSearch(t, f.store, OwnerScope(SystemUserID), "piranesi")
	if len(res.Hits) != 1 || res.Hits[0].Title != "Piranesi" {
		t.Errorf("fusion returned %q, want exactly [Piranesi]. A wrong title here "+
			"with the right shape is the rowid-drift signature.", hitTitles(res.Hits))
	}
}

// TestTheTwoLegsTokeniseDifferentlyOnPurpose pins behaviour that looks like a
// bug and is not.
//
// search_fts is `unicode61 remove_diacritics 2` and folds accents; search_trgm
// is `trigram` and folds nothing. So `amelie` finds `Amélie` on the keyword leg
// and misses it on the trigram leg. That disagreement is the whole argument for
// running two engines and fusing them — do not "fix" it by making the tokenizers
// match, which would delete one of the two legs' reason to exist.
func TestTheTwoLegsTokeniseDifferentlyOnPurpose(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t, searchItem("1", "2", "book", "Amélie"))

	var fts, trgm int
	if err := f.store.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM search_fts WHERE search_fts MATCH ?`, `"amelie"*`).Scan(&fts); err != nil {
		t.Fatalf("search_fts: %v", err)
	}
	if err := f.store.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM search_trgm WHERE search_trgm MATCH ?`, `"amelie"`).Scan(&trgm); err != nil {
		t.Fatalf("search_trgm: %v", err)
	}
	if fts != 1 {
		t.Errorf("search_fts found %d for `amelie` against `Amélie`, want 1: "+
			"remove_diacritics 2 is what folds the accent", fts)
	}
	if trgm != 0 {
		t.Errorf("search_trgm found %d for `amelie` against `Amélie`, want 0: "+
			"the trigram tokenizer folds nothing, and that asymmetry is why "+
			"two legs are fused rather than one being enough", trgm)
	}

	// And the fused answer is the union: the accent-blind leg carries it.
	if got := mustSearch(t, f.store, OwnerScope(SystemUserID), "amelie").Hits; len(got) != 1 {
		t.Errorf("the fused search found %q for `amelie`, want [Amélie]", hitTitles(got))
	}
}

// TestSoftDeletedWorksAreNotSearchable pins the join to `work`.
//
// search_doc has no tombstone column and nothing deletes a document when a work
// is SOFT deleted — the foreign key cascades on a hard delete only. Without
// `w.deleted_at IS NULL` the seven-day tombstone window is fully searchable,
// which is the same degradation schema.md §1 says the partial indexes exist to
// prevent.
func TestSoftDeletedWorksAreNotSearchable(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t, searchItem("1", "1", "comic", "Dorohedoro"))
	scope := OwnerScope(SystemUserID)

	if got := len(mustSearch(t, f.store, scope, "dorohedoro").Hits); got != 1 {
		t.Fatalf("baseline: %d hits, want 1", got)
	}
	execTest(t, f.store, `UPDATE work SET deleted_at = datetime('now')`)
	if got := mustSearch(t, f.store, scope, "dorohedoro").Hits; len(got) != 0 {
		t.Errorf("a soft-deleted work is still searchable: %q.\n"+
			"search_doc carries no tombstone, so the join to work is the only "+
			"thing that can know.", hitTitles(got))
	}
}

// TestSearchLibraryTruncatesAndSaysSo: §3 step 1's cap has to reach the wire,
// because §3 requires a UI note and a note the server never sends cannot be
// rendered.
func TestSearchLibraryTruncatesAndSaysSo(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t, searchItem("1", "1", "comic", "Frieren"))

	short := mustSearch(t, f.store, OwnerScope(SystemUserID), "frieren")
	if short.Truncated {
		t.Error("a one-token query reported Truncated")
	}
	long := mustSearch(t, f.store, OwnerScope(SystemUserID), strings.Repeat("frieren ", 20))
	if !long.Truncated {
		t.Error("a twenty-token query did not report Truncated")
	}
}

// TestRankingIsBlindToProvenance is the §8.2 seam, asserted as the NEGATIVE the
// requirement is stated as: ranking must never learn which engine produced a
// candidate.
//
// It is a structural assertion rather than a behavioural one, on purpose. The
// behaviour "ranking ignores the leg" cannot be observed from outside, because a
// ranker that secretly used provenance would still produce SOME order. What can
// be observed is that the type ranking consumes has nowhere to put it: rerank
// takes []searchCandidate, and searchCandidate's fields are a SearchHit, a
// float and a string. Adding a leg name to it is what this test forbids, and it
// is the edit that would make every future weight a weight on an ENGINE rather
// than on evidence — after which a third leg stops being an append to a slice.
func TestRankingIsBlindToProvenance(t *testing.T) {
	// rerank's whole input, constructed by hand: two candidates that a caller
	// could only distinguish by their leg if the type carried one.
	got := rerank([]string{"frieren"}, []searchCandidate{
		{hit: SearchHit{ID: 2, Title: "Frieren", MediaType: MediaTypeComics}, rrf: 0.03, normTitle: "frieren"},
		{hit: SearchHit{ID: 1, Title: "Fried Green", MediaType: MediaTypeComics}, rrf: 0.02, normTitle: "fried green"},
	})
	if len(got) != 2 || got[0].ID != 2 {
		t.Errorf("rerank put %q first; the stronger fused candidate with the "+
			"closer title must lead", hitTitles(got))
	}

	// The structural half. searchCandidate is the ONLY thing that crosses from
	// fusion into ranking, and it must carry no leg.
	for _, banned := range []string{"leg", "engine", "source", "table"} {
		if fieldNamesOfSearchCandidate(banned) {
			t.Errorf("searchCandidate has a field naming %q. Ranking must never "+
				"learn which engine produced a candidate (ARCHITECTURE.md §8.2).", banned)
		}
	}
}

// fieldNamesOfSearchCandidate reports whether searchCandidate has a field whose
// name contains s. It is spelled out rather than reflected over because the
// struct has three fields and a compile-time list is the clearer assertion: a
// fourth field added without touching this list is caught by the compiler at
// the switch below, not silently admitted.
func fieldNamesOfSearchCandidate(s string) bool {
	c := searchCandidate{}
	// The exhaustive destructure. Adding a field to searchCandidate breaks this
	// line, which is the point.
	_, _, _ = c.hit, c.rrf, c.normTitle
	for _, name := range []string{"hit", "rrf", "normTitle"} {
		if strings.Contains(name, s) {
			return true
		}
	}
	return false
}

// TestDiversifyPromotesAnAbsentMediaType is §4's media-type diversity injection:
// "guarantee the top 10 contains at least one item per media type that scored
// above a floor".
//
// §4 calls it the thing that makes the Train Dreams case work — a film and a
// novella of the same name, where the film's richer text sweeps the list and the
// novella never appears.
//
// ⚠️ IT IS A NO-OP ON THE CORPUS THIS PROJECT CAN BUILD TODAY. Kavita is the one
// catalogue adapter and it writes comics, so every real candidate has the same
// media type. This test is synthetic for that reason, and says so rather than
// implying the behaviour has been seen on an import.
func TestDiversifyPromotesAnAbsentMediaType(t *testing.T) {
	hits := make([]SearchHit, 0, 15)
	for i := 1; i <= 14; i++ {
		hits = append(hits, SearchHit{ID: int64(i), Title: "comic", MediaType: MediaTypeComics})
	}
	hits = append(hits, SearchHit{ID: 99, Title: "the novella", MediaType: MediaTypeEbooks})

	got := diversify(hits)
	if len(got) != len(hits) {
		t.Fatalf("diversify changed the list length: %d → %d. It promotes, it "+
			"does not drop.", len(hits), len(got))
	}
	var found bool
	for _, h := range got[:diversifyWindow] {
		if h.MediaType == MediaTypeEbooks {
			found = true
		}
	}
	if !found {
		t.Errorf("the only ebook did not reach the top %d:\n  %q\n"+
			"Without the promotion, whichever medium has better text statistics "+
			"sweeps the list and the novella never appears — the failure §4 says "+
			"the feature exists to prevent.", diversifyWindow, hitTitles(got[:diversifyWindow]))
	}

	// ⚠️ AND rerank ACTUALLY CALLS IT. The first version of this test exercised
	// diversify() directly, so deleting the call from rerank left it green — an
	// ABSORBED guard, which is the failure mode this area has a recorded history
	// of (LS-01, four of them). This subtest goes through rerank, which is the
	// only path SearchLibrary uses.
	t.Run("rerank applies it, not just the function existing", func(t *testing.T) {
		candidates := make([]searchCandidate, 0, 15)
		for i := 1; i <= 14; i++ {
			candidates = append(candidates, searchCandidate{
				hit:       SearchHit{ID: int64(i), Title: "comic", MediaType: MediaTypeComics},
				rrf:       0.03,
				normTitle: "comic",
			})
		}
		// Deliberately the WEAKEST candidate on every signal, so nothing but
		// the promotion can lift it into the window.
		candidates = append(candidates, searchCandidate{
			hit:       SearchHit{ID: 99, Title: "the novella", MediaType: MediaTypeEbooks},
			rrf:       0.001,
			normTitle: "zzzzzzzzz",
		})

		got := rerank([]string{"comic"}, candidates)
		var found bool
		for _, h := range got[:diversifyWindow] {
			if h.MediaType == MediaTypeEbooks {
				found = true
			}
		}
		if !found {
			t.Errorf("rerank did not diversify: the only ebook is absent from the "+
				"top %d:\n  %q\nThe function existing is not the same as rerank "+
				"calling it.", diversifyWindow, hitTitles(got[:diversifyWindow]))
		}
	})

	t.Run("a single-media-type list is untouched", func(t *testing.T) {
		same := diversify(hits[:14])
		for i := range same {
			if same[i].ID != hits[i].ID {
				t.Fatalf("diversify reordered a list with one media type at %d", i)
			}
		}
	})
}

// TestSearchLibraryOrderIsDeterministic. An unstable order across identical
// requests makes every screenshot and every test flaky, and a user reads it as
// the list moving under them.
func TestSearchLibraryOrderIsDeterministic(t *testing.T) {
	f := newSearchFixture(t)
	items := make([]CatalogueItem, 0, 8)
	for i := range 8 {
		items = append(items, searchItem(string(rune('a'+i)), "1", "comic", "Test Volume"))
	}
	f.apply(t, items...)

	first := hitTitles(mustSearch(t, f.store, OwnerScope(SystemUserID), "test").Hits)
	for range 5 {
		again := hitTitles(mustSearch(t, f.store, OwnerScope(SystemUserID), "test").Hits)
		if strings.Join(first, "|") != strings.Join(again, "|") {
			t.Fatalf("two identical searches disagreed:\n  %q\n  %q", first, again)
		}
	}
}

// TestJaroWinkler pins the metric against its published definition, because a
// similarity function that is subtly wrong produces a plausible ranking and
// never errors.
func TestJaroWinkler(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want float64
	}{
		{"", "", 1},
		{"a", "", 0},
		{"martha", "marhta", 0.961},
		{"dixon", "dicksonx", 0.813},
		{"dwayne", "duane", 0.840},
		{"frieren", "frieren", 1},
		{"abc", "xyz", 0},
	} {
		got := jaroWinkler(tc.a, tc.b)
		if got < tc.want-0.001 || got > tc.want+0.001 {
			t.Errorf("jaroWinkler(%q,%q) = %.4f, want %.3f", tc.a, tc.b, got, tc.want)
		}
	}

	t.Run("it is prefix weighted, which is the reason §4 chose it", func(t *testing.T) {
		// The Winkler half is exactly a boost over Jaro for a SHARED PREFIX, so
		// the property is asserted against Jaro itself rather than against
		// another pair — two pairs can differ for reasons that have nothing to
		// do with the prefix (a deletion scores very high on Jaro whatever it
		// deletes, which is how the first version of this test fooled itself).
		withPrefix := []string{"dwayne", "duane"}
		noPrefix := []string{"martha", "arthma"}

		base := jaro([]rune(withPrefix[0]), []rune(withPrefix[1]))
		boosted := jaroWinkler(withPrefix[0], withPrefix[1])
		if boosted <= base {
			t.Errorf("a shared prefix was not boosted: jaro=%.4f jaroWinkler=%.4f.\n"+
				"§4 chose this metric because people get the beginning of a "+
				"title right; without the boost it is plain Jaro.", base, boosted)
		}

		flat := jaro([]rune(noPrefix[0]), []rune(noPrefix[1]))
		if got := jaroWinkler(noPrefix[0], noPrefix[1]); got != flat {
			t.Errorf("a pair with NO shared prefix was boosted: jaro=%.4f "+
				"jaroWinkler=%.4f", flat, got)
		}

		// And the property that matters to a search box: a typed prefix of a
		// real title scores very high.
		if got := jaroWinkler("the lord of the r", "the lord of the rings"); got <= 0.9 {
			t.Errorf("a typed prefix of a title scored %.3f, want > 0.9", got)
		}
	})

	t.Run("it operates on runes, not bytes", func(t *testing.T) {
		// Two strings differing in one MULTI-BYTE character. A byte-wise
		// implementation sees three differences here, not one.
		if got := jaroWinkler("葬送のフリーレン", "葬送のフリーレソ"); got < 0.9 {
			t.Errorf("jaroWinkler on two 8-rune strings differing in one rune = %.3f, want > 0.9", got)
		}
	})
}

// TestSearchLibraryLimits: the store clamps its own page, so a caller that
// forgets to cannot walk the whole candidate set.
func TestSearchLibraryLimits(t *testing.T) {
	f := newSearchFixture(t)
	items := make([]CatalogueItem, 0, 30)
	for i := range 30 {
		items = append(items, searchItem(strconv.Itoa(i), "1", "comic", "Volume "+strconv.Itoa(i)))
	}
	f.apply(t, items...)
	scope := OwnerScope(SystemUserID)

	for _, tc := range []struct{ ask, want int }{
		{0, SearchDefaultLimit},
		{5, 5},
		{10_000, SearchMaxLimit},
	} {
		res, err := f.store.SearchLibrary(t.Context(), scope, "volume", tc.ask)
		if err != nil {
			t.Fatalf("SearchLibrary: %v", err)
		}
		if got := len(res.Hits); got > tc.want {
			t.Errorf("limit=%d returned %d hits, want at most %d", tc.ask, got, tc.want)
		}
	}
}

// TestSearchLibraryEmptyQueryReturnsNothing. An empty query is the caller's to
// reject; the store must never read it as "everything".
func TestSearchLibraryEmptyQueryReturnsNothing(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t, searchItem("1", "1", "comic", "Frieren"))
	for _, q := range []string{"", "   ", "\t\n"} {
		res, err := f.store.SearchLibrary(t.Context(), OwnerScope(SystemUserID), q, 0)
		if err != nil {
			t.Fatalf("SearchLibrary(%q): %v", q, err)
		}
		if len(res.Hits) != 0 {
			t.Errorf("SearchLibrary(%q) returned %d hits; an empty query must "+
				"never read as everything", q, len(res.Hits))
		}
	}
}

// TestTheThreeDeadSignalsAreStillDead is the guard on decision 3's honesty.
//
// §4 names five ranking signals and three of them are constants on real data:
// writeSearchDoc hardcodes popularity = 0, title_idf = 0 and in_library = 1 for
// every row. The re-rank therefore implements two, and says so. THIS TEST FAILS
// WHEN THAT STOPS BEING TRUE — which is the moment to add the missing term, in
// the same commit as the writer that started computing it.
func TestTheThreeDeadSignalsAreStillDead(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t,
		searchItem("1", "1", "comic", "Frieren"),
		searchItem("2", "2", "book", "Piranesi"),
	)
	for _, tc := range []struct {
		column string
		want   string
	}{
		{"popularity", "0"},
		{"title_idf", "0"},
		{"in_library", "1"},
	} {
		n := count(t, f.store,
			`SELECT COUNT(*) FROM search_doc WHERE `+tc.column+` <> `+tc.want)
		if n != 0 {
			t.Errorf("%d search_doc row(s) have %s <> %s. That signal is no longer "+
				"constant, so the re-rank should now use it — see the weight block "+
				"in searchlibrary.go, which documents all three as inert.", n, tc.column, tc.want)
		}
	}
}

// ── SearchHit.Score, the number the wire publishes as §6.2.1's `score` ───────
//
// docs/reference/http-api.md §6.2.1 makes four claims to a consumer, and the
// four tests below are those claims. They are written against the SCORE and not
// against the order, because the order already has TestSearchLibraryOrderIs-
// Deterministic and TestDiversifyPromotesAnAbsentMediaType: what is new here is
// a number a client can read, and every property §6.2.1 promises of it has to be
// a property something breaks when it stops holding.

// TestSearchScoreIsTheWeightedSumOfTheLiveSignals pins WHAT the number is.
//
// §6.2.1 says it is "the weighted sum of three normalised signals" and not a raw
// bm25 rank, not the RRF value, and not a percentage. Those are four different
// numbers with the same shape — a small positive float — so an implementation
// that published any of the other three would look correct in every ordering
// test in this file. This one computes the documented value independently and
// insists on it.
//
// The corpus is ONE work on purpose: with a single candidate, two of the three
// components are pinned by construction (rrf/bestRRF = 1 because the candidate
// IS the best, recency = 1 because one dated candidate is both newest and
// oldest), which leaves Jaro-Winkler as the only term that has to be computed —
// and it is computed here from the same inputs the ranker gets rather than
// copied as a literal, so the test states the FORMULA rather than a number
// somebody once observed.
func TestSearchScoreIsTheWeightedSumOfTheLiveSignals(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t, searchItem("1", "1", "comic", "Vinland Saga"))

	hits := mustSearch(t, f.store, OwnerScope(SystemUserID), "vinland").Hits
	if len(hits) != 1 {
		t.Fatalf("%d hits, want 1: %q", len(hits), hitTitles(hits))
	}

	q := buildFTSQueries("vinland")
	jw := jaroWinkler(
		normalizeForCompare(strings.Join(q.tokens, " ")),
		normalizeForCompare(NormalizeForSearch("Vinland Saga")))
	want := rerankWeightRRF*1.0 + rerankWeightJW*jw + rerankWeightRecency*1.0

	if got := hits[0].Score; math.Abs(got-want) > 1e-12 {
		t.Errorf("Score = %v, want %v (%.2f·rrf/best + %.2f·jw(%v) + %.2f·recency).\n"+
			"The published number is the RE-RANK score, not the fusion value and "+
			"not a bm25 rank — a consumer reading §6.2.1 is told it is the first "+
			"of those three.",
			got, want, rerankWeightRRF, rerankWeightJW, jw, rerankWeightRecency)
	}
}

// TestSearchScoreIsBoundedAndPositive pins the RANGE §6.2.1 publishes, (0, 1].
//
// Both halves matter to a consumer and for different reasons. The upper bound is
// what lets a grouped card compare its groups' best hits at all; the lower one
// is why the wire field is unconditional rather than `omitempty` — a zero would
// be indistinguishable from an absent key, so a hit must never score one.
func TestSearchScoreIsBoundedAndPositive(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t,
		searchItem("1", "1", "comic", "Vinland Saga"),
		searchItem("2", "1", "comic", "Vinland Saga: Book Two"),
		searchItem("3", "2", "book", "The Long Ships"),
		searchItem("4", "2", "book", "Saga of the Volsungs"),
	)

	hits := mustSearch(t, f.store, OwnerScope(SystemUserID), "vinland saga").Hits
	if len(hits) < 3 {
		t.Fatalf("%d hits, want at least 3 to have a spread: %q", len(hits), hitTitles(hits))
	}
	for _, h := range hits {
		if !(h.Score > 0 && h.Score <= 1) {
			t.Errorf("%q scored %v, which is outside (0, 1] — the range §6.2.1 "+
				"publishes. Every component is normalised to [0,1] and the three "+
				"weights sum to 1, so a value outside it means a component escaped "+
				"its normalisation.", h.Title, h.Score)
		}
	}

	// A single distinct value across the whole answer would satisfy the bound
	// above and carry no information at all, which is the vacuous-green this
	// repo has a rule about: the number has to DISCRIMINATE.
	if hits[0].Score == hits[len(hits)-1].Score {
		t.Errorf("every hit scored %v — the field is present and says nothing:\n  %q",
			hits[0].Score, hitTitles(hits))
	}

	// THE FLOOR UNDER THE TOP HIT, which is §6.2.1's reason thresholding is a
	// forbidden use rather than a discouraged one. The rrf component is
	// normalised against the best candidate IN THE SAME ANSWER, so whichever row
	// wins that division contributes the whole rerankWeightRRF — and the first
	// row of the answer scores at least as much as it does. A query that matched
	// nothing good therefore still leads with a number above the floor, which is
	// why "hide anything under 0.5" hides everything or nothing depending on the
	// query rather than filtering by quality.
	t.Run("a weak match still leads above the floor", func(t *testing.T) {
		weak := mustSearch(t, f.store, OwnerScope(SystemUserID), "volsungs").Hits
		if len(weak) == 0 {
			t.Skip("nothing matched the weak query, so there is no leader to check")
		}
		jw := jaroWinkler(normalizeForCompare("volsungs"), normalizeForCompare(weak[0].Title))
		if jw > 0.6 {
			t.Fatalf("%q is too good a match for %q (jw=%v) for this subtest to be "+
				"about a weak one", weak[0].Title, "volsungs", jw)
		}
		if weak[0].Score < rerankWeightRRF {
			t.Errorf("the best hit for a deliberately poor query scored %v, below "+
				"the %v floor.\nThe floor is what makes a client-side threshold "+
				"meaningless, and §6.2.1 says so — if it does not hold, that "+
				"paragraph is wrong.", weak[0].Score, rerankWeightRRF)
		}
	})
}

// TestSearchScoreDoesNotDependOnTheLimit pins §6.2.1's "the score is a property
// of the query and the corpus, not of how many rows you asked for".
//
// It is a real risk rather than a theoretical one: the store retrieves a fixed
// retrievalLimit candidates, re-ranks the WHOLE of that set, and only then cuts
// to the caller's limit. Move the cut earlier — an optimisation that looks free
// — and both normalising denominators change with the page size, so the same
// work would score differently at limit=1 and limit=100 and a client caching a
// score across two calls would be comparing nothing.
func TestSearchScoreDoesNotDependOnTheLimit(t *testing.T) {
	f := newSearchFixture(t)
	items := make([]CatalogueItem, 0, 6)
	for i := range 6 {
		items = append(items, searchItem(strconv.Itoa(i+1), "1", "comic",
			"Vinland Saga Volume "+strconv.Itoa(i+1)))
	}
	f.apply(t, items...)

	scores := func(limit int) map[int64]float64 {
		t.Helper()
		res, err := f.store.SearchLibrary(t.Context(), OwnerScope(SystemUserID), "vinland", limit)
		if err != nil {
			t.Fatalf("SearchLibrary(limit=%d): %v", limit, err)
		}
		if len(res.Hits) == 0 {
			t.Fatalf("limit=%d returned nothing", limit)
		}
		out := make(map[int64]float64, len(res.Hits))
		for _, h := range res.Hits {
			out[h.ID] = h.Score
		}
		return out
	}

	// A SHORT page and the longest one. The comparison has to reach past the
	// FIRST row to mean anything: the top hit is the best candidate under any
	// cut, so its normalised rrf and its recency are 1 either way and it would
	// score the same even under a re-rank that had been narrowed. The rows
	// BELOW it are where a narrowed candidate set shows.
	const shortPage = 3
	short, full := scores(shortPage), scores(SearchMaxLimit)
	if len(full) <= shortPage {
		t.Fatalf("the corpus returned %d hits at the maximum limit, so a %d-row "+
			"page is not actually shorter", len(full), shortPage)
	}
	var compared int
	for id, got := range short {
		want, ok := full[id]
		if !ok {
			continue
		}
		compared++
		if got != want {
			t.Errorf("work %d scored %v on a %d-row page and %v on a full one.\n"+
				"§6.2.1 tells a client the score does not depend on the page size, "+
				"which is only true while the re-rank sees the whole candidate set "+
				"before the cut.", id, got, shortPage, want)
		}
	}
	if compared < 2 {
		t.Fatalf("only %d work appeared on both pages, so nothing below the top "+
			"row was compared", compared)
	}
}

// TestSearchScoreIsBlindToWhatTheCallerCannotSee is the access-scope half, and
// it is the one property of this field that is a SECURITY property rather than
// an ergonomic one.
//
// The score is normalised over the candidate set, so a document that enters that
// set moves every other document's number. If a document the caller may not see
// could enter it, the published score would be an existence oracle: a client
// could watch a visible work's score drop and learn that something it is not
// allowed to know about matched its query. The scope predicates live INSIDE each
// retrieval leg for exactly this reason, and this test is what says so.
func TestSearchScoreIsBlindToWhatTheCallerCannotSee(t *testing.T) {
	scope := OwnerScope(SystemUserID)

	f := newSearchFixture(t)
	f.apply(t,
		searchItem("1", "1", "comic", "Vinland Saga"),
		searchItem("2", "1", "comic", "Vinland Saga Volume Two"),
	)
	before := map[int64]float64{}
	for _, h := range mustSearch(t, f.store, scope, "vinland").Hits {
		before[h.ID] = h.Score
	}
	if len(before) != 2 {
		t.Fatalf("baseline: %d hits, want 2", len(before))
	}

	// TWO VISIBLE ROWS IS THE MINIMUM THIS CAN BE TESTED WITH, and the reason is
	// worth stating because a one-row version of this test passes under a real
	// leak. Every component is normalised over the visible set, so with a single
	// visible row the ratio is 1 whatever else is in the index. What a leak
	// actually moves is the RELATIVE position of two visible rows inside a leg:
	// invisible documents interleaving between them change each one's rank, and
	// therefore the rrf ratio the re-rank divides by.
	//
	// Three documents that out-rank both visible ones on the typed text — an
	// exact normalised title against two longer ones — filed in the OTHER
	// library, which is then removed from search.
	f.apply(t,
		searchItem("3", "2", "book", "Vinland"),
		searchItem("4", "2", "book", "Vinland"),
		searchItem("5", "2", "book", "Vinland"),
	)
	execTest(t, f.store, `UPDATE library SET include_in_search = 0 WHERE id = ?`,
		f.libraries["novels"])

	// THE SCORES ARE CHECKED BEFORE THE COUNT, deliberately. A leak that lets
	// the hidden rows through to the ANSWER shows in both, and the count is
	// already TestSearchHonoursEnabledAndIncludeInSearch's assertion — if this
	// test bailed on the count first it would never fire its own claim, and a
	// guard nobody has watched fail is indistinguishable from no guard.
	after := mustSearch(t, f.store, scope, "vinland").Hits
	seen := 0
	for _, h := range after {
		want, ok := before[h.ID]
		if !ok {
			continue
		}
		seen++
		if h.Score != want {
			t.Errorf("work %d scored %v alone and %v with three UNSEARCHABLE "+
				"documents in the database.\nThe score is normalised over the "+
				"candidate set and the retrieval ranks it is built from, so a row "+
				"the caller cannot see must never enter either — otherwise the "+
				"number is an existence oracle for rows outside the caller's "+
				"scope.", h.ID, want, h.Score)
		}
	}
	if seen != len(before) {
		t.Errorf("%d of the %d visible works survived the hidden library: %q",
			seen, len(before), hitTitles(after))
	}
	if len(after) != len(before) {
		t.Errorf("a library removed from search still returns %d hits: %q",
			len(after), hitTitles(after))
	}
}

// TestSearchOrderIsNotScoreOrder is the claim §6.2.1 spends most of its words on:
// the server's ORDER is authoritative, and a client re-sorting by the published
// score produces a WORSE list than the one it was handed.
//
// The mechanism is diversify. It is a promotion and not a re-score, so a
// promoted row keeps the score it earned — which is by construction lower than
// every score above it. Sorting by score therefore sends it straight back down,
// and the media-type guarantee §4 calls the reason the feature exists is gone.
//
// ⚠️ THIS IS THE TEST THAT MUST GO THROUGH rerank. Asserting it against
// diversify() alone would stay green if the call were deleted — the absorbed
// guard this area has a recorded history of (LS-01).
func TestSearchOrderIsNotScoreOrder(t *testing.T) {
	candidates := make([]searchCandidate, 0, 15)
	for i := 1; i <= 14; i++ {
		candidates = append(candidates, searchCandidate{
			hit:       SearchHit{ID: int64(i), Title: "comic", MediaType: MediaTypeComics},
			rrf:       0.03,
			normTitle: "comic",
		})
	}
	candidates = append(candidates, searchCandidate{
		hit:       SearchHit{ID: 99, Title: "the novella", MediaType: MediaTypeEbooks},
		rrf:       0.001,
		normTitle: "zzzzzzzzz",
	})

	got := rerank([]string{"comic"}, candidates)

	inWindow := func(hits []SearchHit) bool {
		for _, h := range hits[:diversifyWindow] {
			if h.ID == 99 {
				return true
			}
		}
		return false
	}
	if !inWindow(got) {
		t.Fatalf("the ebook is not in the top %d of the server's own order, so "+
			"there is no promotion for this test to be about", diversifyWindow)
	}

	// The published score of the promoted row is LOWER than the row above it.
	// This is the fact that makes a client-side sort destructive, and it is
	// asserted rather than assumed because a future re-score inside diversify
	// would silently make the rest of this test vacuous.
	var promoted SearchHit
	var pos int
	for i, h := range got {
		if h.ID == 99 {
			promoted, pos = h, i
		}
	}
	if pos == 0 || !(promoted.Score < got[pos-1].Score) {
		t.Fatalf("the promoted row scored %v against %v above it: diversify is "+
			"supposed to PROMOTE, not re-score", promoted.Score, got[pos-1].Score)
	}

	resorted := append([]SearchHit(nil), got...)
	sort.SliceStable(resorted, func(i, j int) bool { return resorted[i].Score > resorted[j].Score })
	if inWindow(resorted) {
		t.Fatalf("re-sorting by score left the ebook in the top %d, so this test "+
			"cannot show what a client-side sort costs", diversifyWindow)
	}

	// The point, stated as an assertion rather than as a comment: the two orders
	// DIFFER, and the server's is the one with the guarantee in it.
	same := true
	for i := range got {
		if got[i].ID != resorted[i].ID {
			same = false
			break
		}
	}
	if same {
		t.Errorf("the server's order and score-descending are the same list, so "+
			"§6.2.1's warning describes nothing. Sorted:\n  %v", hitIDs(resorted))
	}
}

func hitIDs(hits []SearchHit) []int64 {
	out := make([]int64, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	return out
}
