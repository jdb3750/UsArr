package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// The corpus is browse_test.go's, which is recent_test.go's — deliberately,
// because the facet count's central claim is that it counts EXACTLY what the
// library grid pages, and a claim about two reads is only testable over one
// corpus.
//
// seedFacetCorpus adds the four rows neither of those needs and this one cannot
// be tested without:
//
//   - a work in NO library at all (the "unfiled" state the tree actually
//     produces),
//   - a work filed ONLY into the reserved `Unfiled` library 0 (the state the
//     tree cannot currently produce — see facets.go's Unfiled block),
//   - an `artist`, so the Music bucket is fed by BOTH of its kinds and not just
//     by `album`,
//   - a second audiobook on the OTHER service instance, so neither instance
//     scope makes the audiobook statement's own scope predicate a no-op.
// ─────────────────────────────────────────────────────────────────────────────

func seedFacetCorpus(t *testing.T, s *Store) {
	t.Helper()
	seedBrowseCorpus(t, s)

	stmts := []string{
		// 30 — a movie in no library at all. This is what "unfiled" means in
		// this tree: no library_member row, NOT a member of library 0.
		work(30, "movie", "An Unfiled Film", "2026-07-01 10:00:00", ""),
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 30, 'r30', 'movie', '2026-08-16 12:00:00')`,

		// 31 — an ebook filed ONLY into the reserved Unfiled library. Nothing in
		// non-test Go writes this row; it is here so the decision that the
		// reserved library cannot change these numbers is PINNED rather than
		// merely true by accident of nobody writing it.
		work(31, "book", "A Book Filed Nowhere", "2026-06-01 10:00:00", ""),
		`INSERT INTO edition (id, work_id, format) VALUES (31, 31, 'ebook')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (2, 31, 'r31', 'book', '2026-08-16 12:00:00')`,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (0, 'w31', 31)`,

		// 32 — an audiobook on instance 1. The base corpus puts both of its
		// audiobooks (works 4 and 20) on instance 2, which would let the
		// audiobook statement's scope predicate be deleted without any
		// single-instance assertion noticing.
		work(32, "book", "An Audiobook On The Other Box", "2026-05-01 10:00:00", ""),
		`INSERT INTO edition (id, work_id, format) VALUES (32, 32, 'audiobook')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 32, 'r32', 'book', '2026-08-16 12:00:00')`,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (2, 'w32', 32)`,

		// 33 — an artist, so Music is two kinds folded into one bucket.
		work(33, "artist", "Radiohead", "2026-04-01 10:00:00", ""),
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 33, 'r33', 'artist', '2026-08-16 12:00:00')`,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (1, 'w33', 33)`,
	}
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// facetCorpusTotals is the whole visible corpus, by media type, under the
// owner's scope. Every number is arrived at by naming rows in the comment beside
// it, so a fixture change surfaces as a named difference rather than as an
// integer that moved.
var facetCorpusTotals = MediaTypeCounts{
	Movies:     2, // 10 Train Dreams, 30 An Unfiled Film
	TV:         1, // 11 Severance
	Music:      2, // 12 OK Computer (album), 33 Radiohead (artist)
	Ebooks:     4, // 3 Piranesi, 5 Dune (MIXED), 6 no editions, 31 unfiled
	Audiobooks: 3, // 4 Project Hail Mary, 20 part-catalogued, 32 other box
	Comics:     4, // 1 Berserk, 2 Vinland Saga, 13 + 14 undated
}

func countsString(c MediaTypeCounts) string {
	return fmt.Sprintf("movies=%d tv=%d music=%d ebooks=%d audiobooks=%d comics=%d",
		c.Movies, c.TV, c.Music, c.Ebooks, c.Audiobooks, c.Comics)
}

// ─────────────────────────────────────────────────────────────────────────────
// What the read returns.
// ─────────────────────────────────────────────────────────────────────────────

func TestMediaTypeCountsOverTheCorpus(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)

	got, err := s.CountWorksByMediaType(t.Context(), OwnerScope(1))
	if err != nil {
		t.Fatalf("CountWorksByMediaType: %v", err)
	}
	if got != facetCorpusTotals {
		t.Errorf("counts:\n got:  %s\n want: %s", countsString(got), countsString(facetCorpusTotals))
	}
}

// The exclusions, asserted as exclusions rather than inferred from a total that
// happens to match. Each row named here is in the corpus and must not be in any
// bucket, and the arithmetic that would catch it is stated: the corpus holds one
// soft-deleted comic, one person, one comic_issue, and two more comics that ARE
// counted — so a failure of any single exclusion shows up in Comics or in a
// bucket that should not exist at all.
func TestMediaTypeCountsExcludeTombstonesPeopleAndChildKinds(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)

	got, err := s.CountWorksByMediaType(t.Context(), OwnerScope(1))
	if err != nil {
		t.Fatalf("CountWorksByMediaType: %v", err)
	}

	// work 7 "A Soft-Deleted Comic" and work 9 "Berserk #1" (comic_issue) both
	// carry kind='comic'/'comic_issue' and would land in Comics.
	if got.Comics != 4 {
		t.Errorf("comics = %d, want 4 — a soft-deleted comic (work 7) or the "+
			"comic_issue (work 9) is being counted", got.Comics)
	}
	// work 8 "Kentaro Miura" is a person. mediaTypeOf maps `person` to nothing,
	// so if it ever reached the scan the read errors rather than miscounting —
	// which is the assertion. The count above is the second half.
	total := got.Movies + got.TV + got.Music + got.Ebooks + got.Audiobooks + got.Comics
	if total != 16 {
		t.Errorf("the six buckets total %d, want 16 (18 non-child visible works "+
			"less the soft-deleted comic and the person)", total)
	}
}

// ⚠️ THE Unfiled DECISION, PINNED FROM BOTH READINGS. facets.go states it:
// unfiled works ARE counted, and "unfiled" in this tree means a work with no
// `library_member` row at all rather than a member of the reserved library 0 —
// because nothing writes a member row into library 0.
//
// Both are asserted, including the one the tree cannot currently produce, so a
// future writer that starts filing into library 0 meets an assertion instead of
// a silent change of meaning.
func TestMediaTypeCountsCountUnfiledWorks(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)
	ctx := t.Context()

	// The premise: work 30 really is in no library, and work 31 really is in
	// library 0 and nowhere else. A fixture that quietly stopped being true
	// would make the rest of this test assert nothing.
	for _, tc := range []struct {
		workID int64
		want   int64
		what   string
	}{
		{30, 0, "work 30 must be in no library at all"},
		{31, 1, "work 31 must be in exactly one library"},
	} {
		var n int64
		if err := s.DB().Read().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM library_member WHERE work_id = ?`, tc.workID).Scan(&n); err != nil {
			t.Fatalf("membership of %d: %v", tc.workID, err)
		}
		if n != tc.want {
			t.Fatalf("%s: it is in %d", tc.what, n)
		}
	}
	var unfiledLib int64
	if err := s.DB().Read().QueryRowContext(ctx,
		`SELECT library_id FROM library_member WHERE work_id = 31`).Scan(&unfiledLib); err != nil {
		t.Fatalf("library of work 31: %v", err)
	}
	if unfiledLib != UnfiledLibraryID {
		t.Fatalf("work 31 is in library %d, want the reserved %d", unfiledLib, UnfiledLibraryID)
	}

	// The decision itself: both are counted.
	got, err := s.CountWorksByMediaType(ctx, OwnerScope(1))
	if err != nil {
		t.Fatalf("CountWorksByMediaType: %v", err)
	}
	if got.Movies != 2 {
		t.Errorf("movies = %d, want 2 — work 30 is in no library and MUST still be "+
			"counted (facets.go, the Unfiled block)", got.Movies)
	}
	if got.Ebooks != 4 {
		t.Errorf("ebooks = %d, want 4 — work 31 is in the reserved Unfiled library "+
			"and MUST still be counted (facets.go, the Unfiled block)", got.Ebooks)
	}
}

// ⚠️ THE EXCLUSION BOUNDARY, FROM THE CONSUMER'S SIDE. The counts are of what
// you OWN and carry no `?lib=` scope, so the number a facet reports and the
// number a LIBRARY-SCOPED grid page returns are different questions with
// different answers — and the difference is exactly the unfiled work.
//
// This is asserted by CALLING the grid, not by re-deriving what the grid would
// do: the failure this guards against is a facet count that silently agrees
// with whatever query its own author wrote.
func TestMediaTypeCountsAreNotLibraryScoped(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)
	ctx := t.Context()

	got, err := s.CountWorksByMediaType(ctx, OwnerScope(1))
	if err != nil {
		t.Fatalf("CountWorksByMediaType: %v", err)
	}

	// Every library there is, as the grid's own resolver sees them. The union of
	// all of them is still missing work 30, which is in none.
	all, err := s.ListLibraries(ctx, OwnerScope(1))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	ids := make([]int64, 0, len(all))
	for _, l := range all {
		ids = append(ids, l.ID)
	}
	scoped := browseAll(t, s, OwnerScope(1),
		WorksFilter{MediaType: MediaTypeMovies, LibraryIDs: ids}, 50)

	if int64(len(scoped)) >= got.Movies {
		t.Fatalf("the movie facet (%d) is not larger than a grid page scoped to every "+
			"library (%d) — either the fixture lost its unfiled work, or the facet "+
			"count has grown a library predicate it must not have",
			got.Movies, len(scoped))
	}
	for _, title := range scoped {
		if title == "An Unfiled Film" {
			t.Errorf("the unfiled film came back from a library-scoped page: %v", scoped)
		}
	}
}

// ⚠️ THE CENTRAL CLAIM: the count equals the list the user reaches by clicking
// it. For each of the six types, the facet number and an exhausted, UNSCOPED
// grid walk must be the same number.
//
// It goes through ListWorks — the caller's read — rather than through this
// package's query, because the failure being guarded against is precisely a
// count that agrees with its own author's SQL and with nothing a user sees.
func TestMediaTypeCountsAgreeWithTheBrowseRead(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)

	got, err := s.CountWorksByMediaType(t.Context(), OwnerScope(1))
	if err != nil {
		t.Fatalf("CountWorksByMediaType: %v", err)
	}

	for _, tc := range []struct {
		mediaType string
		facet     int64
	}{
		{MediaTypeMovies, got.Movies},
		{MediaTypeTV, got.TV},
		{MediaTypeMusic, got.Music},
		{MediaTypeEbooks, got.Ebooks},
		{MediaTypeAudiobooks, got.Audiobooks},
		{MediaTypeComics, got.Comics},
	} {
		t.Run(tc.mediaType, func(t *testing.T) {
			// A page size of 2 so the walk really pages: a count that matched
			// only the first page would be a count of the page size.
			rows := browseAll(t, s, OwnerScope(1), WorksFilter{MediaType: tc.mediaType}, 2)
			if int64(len(rows)) != tc.facet {
				t.Errorf("facet says %d, the grid returns %d: %v", tc.facet, len(rows), rows)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The access scope.
// ─────────────────────────────────────────────────────────────────────────────

// ⚠️ THE SCOPE IS BROKEN AND AN ASSERTION CATCHES IT, rather than the parameter
// merely being threaded. store.go rule 2 is at its most literal here: six
// numbers whose entire content is how much exists.
//
// Both statements are covered. The instance-1 and instance-2 scopes each hold
// audiobooks, so deleting the scope predicate from EITHER statement moves a
// number in this test.
func TestMediaTypeCountsScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)
	ctx := t.Context()

	one, err := s.CountWorksByMediaType(ctx, Scope{UserID: 1, InstanceIDs: []int64{1}})
	if err != nil {
		t.Fatalf("instance 1: %v", err)
	}
	wantOne := MediaTypeCounts{
		Movies:     1, // 30 An Unfiled Film
		TV:         1, // 11 Severance
		Music:      1, // 33 Radiohead
		Ebooks:     2, // 3 Piranesi, 5 Dune
		Audiobooks: 1, // 32 An Audiobook On The Other Box
		Comics:     2, // 1 Berserk, 13 An Undated Manga
	}
	if one != wantOne {
		t.Errorf("instance 1:\n got:  %s\n want: %s", countsString(one), countsString(wantOne))
	}

	two, err := s.CountWorksByMediaType(ctx, Scope{UserID: 1, InstanceIDs: []int64{2}})
	if err != nil {
		t.Fatalf("instance 2: %v", err)
	}
	wantTwo := MediaTypeCounts{
		Movies:     1, // 10 Train Dreams
		TV:         0,
		Music:      1, // 12 OK Computer
		Ebooks:     2, // 6 A Book With No Edition Rows, 31 A Book Filed Nowhere
		Audiobooks: 2, // 4 Project Hail Mary, 20 A Part-Catalogued Audiobook
		Comics:     2, // 2 Vinland Saga, 14 Another Undated Manga
	}
	if two != wantTwo {
		t.Errorf("instance 2:\n got:  %s\n want: %s", countsString(two), countsString(wantTwo))
	}

	// The two scopes partition the corpus. This is the assertion that catches a
	// scope predicate deleted from EITHER statement: without it each half
	// reports the whole, and the sums double.
	sum := MediaTypeCounts{
		Movies:     one.Movies + two.Movies,
		TV:         one.TV + two.TV,
		Music:      one.Music + two.Music,
		Ebooks:     one.Ebooks + two.Ebooks,
		Audiobooks: one.Audiobooks + two.Audiobooks,
		Comics:     one.Comics + two.Comics,
	}
	if sum != facetCorpusTotals {
		t.Errorf("the two instance scopes do not partition the corpus:\n got:  %s\n want: %s",
			countsString(sum), countsString(facetCorpusTotals))
	}
}

// FAIL CLOSED, and it is the whole point of this read carrying a scope at all.
// "No visible instances" must mean zero everywhere, never the whole catalogue.
//
// httpapi's storeScope hands exactly this scope to every non-owner, and there
// are no non-owner accounts in v0.1, so this is the assertion that decides what
// happens the day one appears.
func TestMediaTypeCountsFailClosedOnAnEmptyScope(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)

	got, err := s.CountWorksByMediaType(t.Context(), Scope{UserID: 7})
	if err != nil {
		t.Fatalf("CountWorksByMediaType: %v", err)
	}
	if got != (MediaTypeCounts{}) {
		t.Errorf("an empty scope saw %s over a seeded corpus; it must see nothing",
			countsString(got))
	}
}

// ⚠️ ZERO AND INVISIBLE ARE THE SAME ANSWER, IN ONE DIRECTION. A caller who can
// see nothing and a caller looking at an empty database get BYTE-IDENTICAL
// answers — not merely equivalent ones — so nothing on this read can be used to
// learn that content exists behind a scope.
//
// The direction matters and only one of the two is a leak: a caller who can see
// everything learning that a type is empty is the answer, not a disclosure.
func TestMediaTypeCountsHideRestrictionInsideZero(t *testing.T) {
	full := newTestStore(t)
	seedFacetCorpus(t, full)
	restricted, err := full.CountWorksByMediaType(t.Context(), Scope{UserID: 7})
	if err != nil {
		t.Fatalf("restricted: %v", err)
	}

	empty := newTestStore(t)
	genuine, err := empty.CountWorksByMediaType(t.Context(), OwnerScope(1))
	if err != nil {
		t.Fatalf("empty database: %v", err)
	}

	if restricted != genuine {
		t.Errorf("a restricted caller can tell itself apart from an empty library:"+
			"\n restricted: %s\n empty:      %s", countsString(restricted), countsString(genuine))
	}
}

// A seventh media type is an edit to MediaTypeCounts, to recentWorkKinds and to
// mediaTypeOf, and this is what makes forgetting one of the three loud. Every
// media type the enum publishes must be a bucket `add` accepts, and `add` must
// accept nothing else.
func TestMediaTypeCountsCoverEveryMediaType(t *testing.T) {
	for _, mt := range []string{
		MediaTypeMovies, MediaTypeTV, MediaTypeMusic,
		MediaTypeEbooks, MediaTypeAudiobooks, MediaTypeComics,
	} {
		var c MediaTypeCounts
		if err := c.add(mt, 1); err != nil {
			t.Errorf("media type %q has no bucket: %v", mt, err)
		}
		if c == (MediaTypeCounts{}) {
			t.Errorf("media type %q was accepted and counted nothing", mt)
		}
	}
	var c MediaTypeCounts
	if err := c.add("games", 1); err == nil {
		t.Error("a media type nobody defined was accepted into a bucket")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The plans. Both statements, on the render path of the first screen.
//
// ⚠️ THESE ARE THE NO-ANALYZE PLANNER'S, exactly as browse_test.go's are: a
// fresh test database has no `sqlite_stat1`, while the binary usually runs WITH
// statistics because the importer runs ANALYZE after a full import. A plan that
// holds without statistics holds with them, so this pins the conservative one.
// `make bench` is where the with-statistics wall clock lives.
// ─────────────────────────────────────────────────────────────────────────────

func facetPlan(t *testing.T, s *Store, query string, args []any) string {
	t.Helper()
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// The kind count must be an equality SEEK per kind on ix_work_kind_sort, with
// the GROUP BY supplied by the index rather than by a temp b-tree. Both halves
// are what keep this read off the "six aggregates on the render path" objection
// ADR-0053 rejected as premature: six seeks and no sort, whatever the size of
// the catalogue.
//
// ⚠️ IT PINS `SEARCH … USING INDEX` AND NOT `COVERING INDEX`, per
// reference/schema.md §1's standing rule for this index — covering-ness depends
// on the SELECT list, so pinning it fails the moment one grows a column. It is
// also what SQLite actually reports here; see mediaTypeCountsSQL for the
// measurement and for the prediction it falsified.
func TestMediaTypeCountsPlanIsPerKindSeeks(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)

	query, args := mediaTypeCountsSQL(OwnerScope(1))
	plan := facetPlan(t, s, query, args)

	const want = "SEARCH w USING INDEX ix_work_kind_sort (kind=?)"
	if !strings.Contains(plan, want) {
		t.Errorf("the kind count is not a per-kind seek.\n  got:  %s\n  want: … %s …", plan, want)
	}
	if strings.Contains(plan, "SCAN w") {
		t.Errorf("the kind count degraded to a scan of the work table: %s", plan)
	}
	if strings.Contains(plan, "TEMP B-TREE") {
		t.Errorf("the kind count needs a temp b-tree — the index no longer supplies "+
			"the GROUP BY: %s", plan)
	}
}

// The audiobook half. migration 0009 exists so the SELECTIVE predicate runs
// first and short-circuits: `format = 'audiobook' AND work_id = ?` is a
// two-column covering probe, so a book with no audiobook edition is rejected
// without an edition row ever being fetched. Pinning it here is what stops the
// second, uncovered probe from becoming the driver.
func TestMediaTypeCountsAudiobookPlanSeeksTheFormatIndex(t *testing.T) {
	s := newTestStore(t)
	seedFacetCorpus(t, s)

	query, args := audiobookCountSQL(OwnerScope(1))
	plan := facetPlan(t, s, query, args)

	const wantFormat = "COVERING INDEX ix_edition_format (format=? AND work_id=?)"
	if !strings.Contains(plan, wantFormat) {
		t.Errorf("the format probe does not seek on ix_edition_format.\n"+
			"  got:  %s\n  want: … %s …", plan, wantFormat)
	}
	// The "not every edition is an audiobook" half is a work_id seek on
	// ix_edition_work, stated rather than hidden: no index can seek on
	// `format IS NOT NULL AND format <> ?`.
	if !strings.Contains(plan, "en USING INDEX ix_edition_work (work_id=?)") {
		t.Errorf("the NOT EXISTS half is not a work_id seek:\n  %s", plan)
	}
	if !strings.Contains(plan, "SEARCH w USING INDEX ix_work_kind_sort (kind=?)") {
		t.Errorf("the audiobook count does not drive off ix_work_kind_sort: %s", plan)
	}
	if strings.Contains(plan, "SCAN ea") || strings.Contains(plan, "SCAN en") ||
		strings.Contains(plan, "SCAN w") {
		t.Errorf("the audiobook count scans a table instead of probing it: %s", plan)
	}
	if strings.Contains(plan, "TEMP B-TREE") {
		t.Errorf("the audiobook count needs a temp b-tree: %s", plan)
	}
}
