package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// The corpus.
//
// EVERY TEST BELOW RUNS AGAINST A REAL MIGRATED DATABASE WITH ROWS IN IT. A read
// asserted over an empty database proves the statement parses and nothing about
// what it returns — the ordering, the exclusions and the keyset handoff are all
// invisible at zero rows, and all three are what this read is.
//
// The mix is Kavita's, because Kavita is v0.1's first catalogue adapter
// (ADR-0041) and comics and books are what it writes: comics and books of all
// three edition shapes, one soft-deleted work, one `person` work created the way
// migration 0007's credit applier creates them, one `comic_issue`, works split
// across two service instances AND two user-defined libraries, and two works
// with no added_at at all — the state Kavita reaches when a series carries no
// `created`.
// ─────────────────────────────────────────────────────────────────────────────

// seedRecentCorpus writes the fixture and returns nothing: every test names the
// rows it expects by title, so a change to the fixture surfaces as a named
// difference rather than as an index that moved.
func seedRecentCorpus(t *testing.T, s *Store) {
	t.Helper()
	stmts := []string{
		// Two instances, so the access scope has something to separate. The
		// api_key_enc placeholder is never opened by any read here.
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'kavita', 'library', 'Kavita Manga', 'http://kavita.example', X'00')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (2, 'kavita', 'library', 'Kavita Books', 'http://kavita2.example', X'00')`,

		// Two user-defined libraries over the two instances. Nothing in this
		// read joins them yet — the `?lib=` chip is a later commit — and
		// TestRecentWorksIsLibraryAgnosticToday pins that as a stated gap
		// rather than leaving it to be discovered.
		`INSERT INTO library (id, user_id, name, slug, kind) VALUES (1, 0, 'Manga', 'manga', 'comic')`,
		`INSERT INTO library (id, user_id, name, slug, kind) VALUES (2, 0, 'Books', 'books', 'book')`,

		// ── visible, dated ──────────────────────────────────────────────────
		work(1, "comic", "Berserk", "2026-08-10 10:00:00", ""),
		work(2, "comic", "Vinland Saga", "2026-08-09 10:00:00", ""),
		work(3, "book", "Piranesi", "2026-08-08 10:00:00", ""),
		work(4, "book", "Project Hail Mary", "2026-08-07 10:00:00", ""),
		work(5, "book", "Dune", "2026-08-06 10:00:00", ""),
		work(6, "book", "A Book With No Edition Rows", "2026-08-05 10:00:00", ""),

		// ── must never appear, and each one is INTERLEAVED with the visible
		//    rows by added_at on purpose: an exclusion that only ever had to
		//    hold at the end of the list is not tested by a list that ends
		//    before it. 7, 8 and 9 sit between 6 and 10.
		work(7, "comic", "A Soft-Deleted Comic", "2026-08-04 10:00:00", "2026-08-16 09:00:00"),
		work(8, "person", "Kentaro Miura", "2026-08-03 10:00:00", ""),
		work(9, "comic_issue", "Berserk #1", "2026-08-02 10:00:00", ""),

		// ── visible, dated, the other three media types ─────────────────────
		work(10, "movie", "Train Dreams", "2026-08-01 10:00:00", ""),
		work(11, "series", "Severance", "2026-07-31 10:00:00", ""),
		work(12, "album", "OK Computer", "2026-07-30 10:00:00", ""),

		// ── visible, UNDATED: the tail ix_work_added sorts last ─────────────
		workNullAdded(13, "comic", "An Undated Manga"),
		workNullAdded(14, "comic", "Another Undated Manga"),

		// Editions, which are what split Ebooks from Audiobooks (§17.2 rows 4
		// and 5). Work 6 deliberately has none.
		`INSERT INTO edition (id, work_id, format) VALUES (1, 3, 'ebook')`,
		`INSERT INTO edition (id, work_id, format) VALUES (2, 4, 'audiobook')`,
		`INSERT INTO edition (id, work_id, format) VALUES (3, 5, 'ebook')`,
		`INSERT INTO edition (id, work_id, format) VALUES (4, 5, 'audiobook')`,
	}
	// Instance 1 holds the odd works, instance 2 the even ones, so neither
	// scope is a prefix of the ordering — a scope filter that silently did
	// nothing would still look plausible against a contiguous split.
	for id := 1; id <= 14; id++ {
		inst := 2 - id%2
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO service_item_link
			   (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			 VALUES (%d, %d, 'r%d', 'series', '2026-08-16 12:00:00')`, inst, id, id))
		lib := 1
		if inst == 2 {
			lib = 2
		}
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (%d, 'w%02d', %d)`,
			lib, id, id))
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

func work(id int, kind, title, added, deleted string) string {
	nullable := func(v string) string {
		if v == "" {
			return "NULL"
		}
		return "'" + v + "'"
	}
	return fmt.Sprintf(`INSERT INTO work
		  (id, kind, title, sort_title, normalized_title, year, added_at, deleted_at,
		   have_count, want_count, availability)
		VALUES (%d, '%s', '%s', '%s', '%s', 2020, %s, %s, 3, 1,
		        '{"k":"count","have":3,"total":null,"total_source":null,"missing":[]}')`,
		id, kind, title, strings.ToLower(title), strings.ToLower(title),
		nullable(added), nullable(deleted))
}

func workNullAdded(id int, kind, title string) string {
	return work(id, kind, title, "", "")
}

func titles(rows []RecentWork) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}

// recentCorpusOrder is the whole visible corpus in the order Block C must render
// it: added_at DESC, then id DESC, with the undated tail last.
var recentCorpusOrder = []string{
	"Berserk", "Vinland Saga", "Piranesi", "Project Hail Mary", "Dune",
	"A Book With No Edition Rows", "Train Dreams", "Severance", "OK Computer",
	"Another Undated Manga", "An Undated Manga",
}

// ─────────────────────────────────────────────────────────────────────────────
// What the read returns.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecentWorksIsNewestFirstAcrossEveryMediaType(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	got, next, err := s.ListRecentWorks(t.Context(), OwnerScope(1), RecentWorksCursor{}, 50)
	if err != nil {
		t.Fatalf("ListRecentWorks: %v", err)
	}
	if strings.Join(titles(got), " | ") != strings.Join(recentCorpusOrder, " | ") {
		t.Fatalf("order:\n  got:  %v\n  want: %v", titles(got), recentCorpusOrder)
	}
	if next.AddedAt.Valid || next.NullTail {
		t.Errorf("a page that returned the whole corpus still minted a next cursor: %+v", next)
	}
}

// The Type column, which is §17.2's six-value enum and NOT work.kind. The
// ebooks/audiobooks pair is the half that cannot be derived in the browser.
func TestRecentWorksResolvesTheSixMediaTypes(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	got, _, err := s.ListRecentWorks(t.Context(), OwnerScope(1), RecentWorksCursor{}, 50)
	if err != nil {
		t.Fatalf("ListRecentWorks: %v", err)
	}
	byTitle := map[string]RecentWork{}
	for _, r := range got {
		byTitle[r.Title] = r
	}
	want := map[string]string{
		"Berserk":                     MediaTypeComics,
		"Train Dreams":                MediaTypeMovies,
		"Severance":                   MediaTypeTV,
		"OK Computer":                 MediaTypeMusic,
		"Piranesi":                    MediaTypeEbooks,     // one ebook edition
		"Project Hail Mary":           MediaTypeAudiobooks, // audiobook only
		"Dune":                        MediaTypeEbooks,     // MIXED — see mediaTypeOf
		"A Book With No Edition Rows": MediaTypeEbooks,     // no editions at all
	}
	for title, wantType := range want {
		row, ok := byTitle[title]
		if !ok {
			t.Errorf("%q is missing from the page", title)
			continue
		}
		if row.MediaType != wantType {
			t.Errorf("%q media_type = %q, want %q", title, row.MediaType, wantType)
		}
	}
}

// ADR-0033 and §17.2: `person` maps to no media type, has no screen in any
// milestone, and must never be a row here. Migration 0007 makes this reachable
// rather than theoretical — one Kavita import creates hundreds of person works,
// every one with an added_at from that import.
func TestRecentWorksNeverShowsAPerson(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	// Paged deliberately, so the assertion is not accidentally satisfied by a
	// page that stopped before the person row. Work 8 sits between works 6 and
	// 10 in the ordering.
	for _, page := range allRecentPages(t, s, OwnerScope(1), 3) {
		for _, row := range page {
			if row.Kind == "person" {
				t.Fatalf("a person work reached Block C: %+v", row)
			}
			if row.Title == "Kentaro Miura" {
				t.Fatalf("the person fixture reached Block C under kind %q", row.Kind)
			}
		}
	}
}

// The other four excluded kinds, and the soft-delete tombstone. Both are
// exclusions the partial index ix_work_added is built around (schema.md §1).
func TestRecentWorksNeverShowsAChildKindOrATombstone(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	for _, page := range allRecentPages(t, s, OwnerScope(1), 3) {
		for _, row := range page {
			switch row.Kind {
			case "season", "episode", "track", "comic_issue", "game":
				t.Errorf("a %s work reached Block C: %+v", row.Kind, row)
			}
			if row.Title == "A Soft-Deleted Comic" {
				t.Errorf("a soft-deleted work reached Block C: %+v", row)
			}
			if row.Title == "Berserk #1" {
				t.Errorf("a comic_issue reached Block C: %+v", row)
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The access scope.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecentWorksScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)
	ctx := t.Context()

	// The owner sees everything.
	all, _, err := s.ListRecentWorks(ctx, OwnerScope(1), RecentWorksCursor{}, 50)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	if len(all) != len(recentCorpusOrder) {
		t.Fatalf("owner scope returned %d rows, want %d", len(all), len(recentCorpusOrder))
	}

	// One visible instance sees only its own works. Instance 1 holds the ODD
	// work ids, which is deliberately not a prefix of the ordering.
	one, _, err := s.ListRecentWorks(ctx, Scope{UserID: 1, InstanceIDs: []int64{1}}, RecentWorksCursor{}, 50)
	if err != nil {
		t.Fatalf("one instance: %v", err)
	}
	// Works 1, 3, 5, 11 and 13. Work 7 is soft-deleted and work 9 is a
	// comic_issue, so both are excluded on top of the scope.
	wantOne := []string{"Berserk", "Piranesi", "Dune", "Severance", "An Undated Manga"}
	if strings.Join(titles(one), " | ") != strings.Join(wantOne, " | ") {
		t.Fatalf("instance 1 scope:\n  got:  %v\n  want: %v", titles(one), wantOne)
	}
	if len(one) == len(all) {
		t.Fatalf("the scope parameter changed nothing: %d rows either way", len(one))
	}

	// The other instance sees the complement, and the two partition the corpus.
	two, _, err := s.ListRecentWorks(ctx, Scope{UserID: 1, InstanceIDs: []int64{2}}, RecentWorksCursor{}, 50)
	if err != nil {
		t.Fatalf("other instance: %v", err)
	}
	if len(one)+len(two) != len(all) {
		t.Errorf("the two instance scopes do not partition the corpus: %d + %d != %d",
			len(one), len(two), len(all))
	}
	for _, row := range two {
		if row.Title == "Berserk" {
			t.Errorf("instance 2's scope returned instance 1's work: %+v", row)
		}
	}
}

// FAIL CLOSED. store.go rule 2: "no visible instances" must mean no rows, never
// all rows — a rollup computed across instances a user cannot see is an
// existence oracle, and Block C is a rollup of exactly that shape.
//
// httpapi's storeScope hands this scope to every non-owner, and there are no
// non-owner accounts in v0.1, so this is the assertion that decides what happens
// the day one appears.
func TestRecentWorksFailsClosedOnAnEmptyScope(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	got, next, err := s.ListRecentWorks(t.Context(), Scope{UserID: 7}, RecentWorksCursor{}, 50)
	if err != nil {
		t.Fatalf("ListRecentWorks: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("an empty visible instance set returned %d rows: %v", len(got), titles(got))
	}
	if next.AddedAt.Valid || next.NullTail {
		t.Errorf("an empty scope minted a next cursor: %+v", next)
	}
}

// The read is library-AGNOSTIC today, and that is a stated gap rather than an
// oversight: §17.2's `?lib=` chip is a multi-select over library_member, whose
// key leads with sort_title rather than added_at, so it is a different plan and
// a different commit. This pins the current behaviour so the day it changes,
// it changes deliberately.
func TestRecentWorksIsLibraryAgnosticToday(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	got, _, err := s.ListRecentWorks(t.Context(), OwnerScope(1), RecentWorksCursor{}, 50)
	if err != nil {
		t.Fatalf("ListRecentWorks: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range got {
		seen[r.Title] = true
	}
	// Berserk is in library 1, Piranesi in library 2. Both are here.
	if !seen["Berserk"] || !seen["Piranesi"] {
		t.Fatalf("a work from one of the two libraries is missing: %v", titles(got))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Keyset paging, including the undated tail.
// ─────────────────────────────────────────────────────────────────────────────

// allRecentPages walks the whole corpus one page at a time and returns the
// pages. It fails rather than looping for ever if the cursor stops advancing.
func allRecentPages(t *testing.T, s *Store, scope Scope, limit int) [][]RecentWork {
	t.Helper()
	var pages [][]RecentWork
	cur := RecentWorksCursor{}
	for i := 0; ; i++ {
		if i > 100 {
			t.Fatalf("the cursor did not terminate after 100 pages")
		}
		page, next, err := s.ListRecentWorks(t.Context(), scope, cur, limit)
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		pages = append(pages, page)
		if !next.AddedAt.Valid && !next.NullTail {
			return pages
		}
		cur = next
	}
}

func TestRecentWorksPagesTheWholeCorpusExactlyOnce(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	for _, limit := range []int{1, 2, 3, 4, 5, 11, 12} {
		var walked []string
		for _, page := range allRecentPages(t, s, OwnerScope(1), limit) {
			if len(page) > limit {
				t.Errorf("limit %d: a page carried %d rows", limit, len(page))
			}
			walked = append(walked, titles(page)...)
		}
		if strings.Join(walked, " | ") != strings.Join(recentCorpusOrder, " | ") {
			t.Errorf("limit %d walked the corpus wrongly:\n  got:  %v\n  want: %v",
				limit, walked, recentCorpusOrder)
		}
	}
}

// THE UNDATED TAIL, on its own, because it is the failure a plain
// `(added_at, id) < (?, ?)` keyset walk produces silently: NULL compares to
// nothing, so the rows Kavita reported with no `created` would be reachable on
// the first page and on no other. With limit 2 the first page cannot reach them.
func TestRecentWorksReachesTheUndatedTailThroughTheCursor(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	var walked []string
	for _, page := range allRecentPages(t, s, OwnerScope(1), 2) {
		walked = append(walked, titles(page)...)
	}
	for _, want := range []string{"An Undated Manga", "Another Undated Manga"} {
		found := false
		for _, got := range walked {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is unreachable through the cursor: the undated tail was not paged into.\n"+
				"  walked: %v", want, walked)
		}
	}
	// And they are LAST, never first: an absent date must not claim the top of
	// a table whose whole purpose is recency.
	if walked[len(walked)-1] != "An Undated Manga" || walked[len(walked)-2] != "Another Undated Manga" {
		t.Errorf("the undated rows are not the tail: %v", walked)
	}
}

func TestRecentWorksCursorRoundTrips(t *testing.T) {
	cases := []RecentWorksCursor{
		{AddedAt: sql.NullString{String: "2026-08-10 10:00:00", Valid: true}, ID: 42},
		{NullTail: true, ID: 7},
	}
	for _, c := range cases {
		got, err := DecodeRecentWorksCursor(EncodeRecentWorksCursor(c))
		if err != nil {
			t.Fatalf("decode(encode(%+v)): %v", c, err)
		}
		if got != c {
			t.Errorf("round trip: got %+v, want %+v", got, c)
		}
	}
	if got, err := DecodeRecentWorksCursor(""); err != nil || got != (RecentWorksCursor{}) {
		t.Errorf("the empty cursor must decode to the first page, got %+v, %v", got, err)
	}
	// The token has to survive a query string unescaped, which is why it is
	// base64url at all: the payload carries store.go's timeLayout, and that
	// layout has a SPACE in it because SQLite's datetime() does.
	tok := EncodeRecentWorksCursor(cases[0])
	if strings.ContainsAny(tok, " +/=&?#%") {
		t.Errorf("cursor %q carries a character a query string cannot hold unescaped", tok)
	}

	// STRICT. A cursor that will not parse is an error, never a silent reset to
	// page one — that turns a stale bookmark into a Load-more loop over page one.
	bad := []string{"not base64!!", "***"}
	for _, payload := range []string{
		"x", "1", "1d", "1dabc.2026-08-10 10:00:00", "1d5", "1d5.", "1n", "1nx", "2d5.x", "1d0.x",
	} {
		bad = append(bad, base64.RawURLEncoding.EncodeToString([]byte(payload)))
	}
	for _, b := range bad {
		if _, err := DecodeRecentWorksCursor(b); err == nil {
			t.Errorf("cursor %q was accepted", b)
		}
	}

	// And the message must not echo the caller's text back — it is rendered by
	// httpapi into a response body.
	_, err := DecodeRecentWorksCursor(base64.RawURLEncoding.EncodeToString([]byte("9<script>")))
	if err == nil {
		t.Fatal("a junk cursor was accepted")
	}
	if strings.Contains(err.Error(), "script") {
		t.Errorf("the cursor error echoes the caller's token back: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The query plan. schema.md §1 is what these pin, and they EXPLAIN the statement
// that ships — recentWorksSQL itself, not a lookalike pasted into a test
// (docs/DEVELOPMENT.md §11 rule 1).
// ─────────────────────────────────────────────────────────────────────────────

// recentWorksPlanFaults is the guard, factored out so the tests that fire it
// deliberately run the SAME predicate the passing test runs. A guard asserted
// one way and fired against a different condition has proved nothing.
func recentWorksPlanFaults(plan string) []string {
	var faults []string
	// planHas, not strings.Contains: `ix_work_added` is a prefix of
	// `ix_work_added_at`, and a substring match would survive that rename while
	// pinning nothing (planassert_test.go).
	if !planHas(plan, "USING INDEX ix_work_added") {
		faults = append(faults, "does not use ix_work_added")
	}
	if strings.Contains(plan, "ix_work_kind_sort") {
		faults = append(faults,
			"fell back to ix_work_kind_sort, which serves the kind filter and not the order")
	}
	// A table scan, as distinct from the ordered INDEX scan the first page is.
	if strings.Contains(plan, "SCAN work") && !strings.Contains(plan, "SCAN work USING INDEX") {
		faults = append(faults, "degraded to a scan of the work table")
	}
	// The whole point of ix_work_added is that the index supplies the order. A
	// sort here is Home paying for the entire catalogue on every page.
	if strings.Contains(plan, "TEMP B-TREE") {
		faults = append(faults, "needs a temp b-tree for the ordering")
	}
	// schema.md §1: assert SEARCH … USING INDEX, never COVERING INDEX. The
	// SELECT list carries title, year, have_count, want_count and availability,
	// so a covering plan means the query changed, not that the index improved.
	if strings.Contains(plan, "COVERING INDEX ix_work_added") {
		faults = append(faults, "is a COVERING INDEX, so this is not the shipped SELECT list")
	}
	// PosterKeyExpr is a correlated subquery that runs ONCE PER ROW of the page,
	// so it has to be a rowid seek and nothing else. It is written as
	// `ia.id = w.poster_asset_id` against an INTEGER PRIMARY KEY, which SQLite
	// answers with `SEARCH ia USING INTEGER PRIMARY KEY (rowid=?)`; anything
	// else — a scan, or a seek on some other index — means the expression was
	// rewritten, and fifty full scans of image_asset per page is what that
	// costs.
	if strings.Contains(plan, "ia ") || strings.Contains(plan, "SCAN ia") {
		if !strings.Contains(plan, "SEARCH ia USING INTEGER PRIMARY KEY (rowid=?)") {
			faults = append(faults, "the poster-key subquery is not a rowid seek on image_asset")
		}
	}
	return faults
}

func recentWorksPlan(t *testing.T, s *Store, scope Scope, cur RecentWorksCursor) string {
	t.Helper()
	query, args := recentWorksSQL(scope, cur, RecentWorksDefaultLimit)
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// The keyset continuation, which is the plan schema.md §1's partial index exists
// for and the one every page after the first uses:
//
//	SEARCH work USING INDEX ix_work_added (added_at<?)
//
// SEARCH … USING INDEX, deliberately NOT COVERING INDEX, for §1's stated reason.
func TestRecentWorksKeysetPlanIsNormative(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	joined := recentWorksPlan(t, s, OwnerScope(1), RecentWorksCursor{
		AddedAt: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4,
	})
	const want = "SEARCH w USING INDEX ix_work_added (added_at<?)"
	if !strings.Contains(joined, want) {
		t.Errorf("the recently-added keyset plan is not the normative one.\n"+
			"  got:  %s\n  want: %s", joined, want)
	}
	if faults := recentWorksPlanFaults(joined); len(faults) > 0 {
		t.Errorf("the keyset plan is wrong:\n  plan: %s\n  %s", joined, strings.Join(faults, "\n  "))
	}
}

// The first page has no cursor predicate, so it is an ordered INDEX SCAN with no
// sort — which is the right plan for "newest N" and is what internal/db's
// "recently added, all kinds" case already records. It still has to be
// ix_work_added, and it still must not need a temp b-tree.
func TestRecentWorksFirstPagePlanIsAnOrderedIndexWalk(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	joined := recentWorksPlan(t, s, OwnerScope(1), RecentWorksCursor{})
	if !planHas(joined, "SCAN w USING INDEX ix_work_added") {
		t.Errorf("the first page is not an ordered walk of ix_work_added: %s", joined)
	}
	if faults := recentWorksPlanFaults(joined); len(faults) > 0 {
		t.Errorf("the first page plan is wrong:\n  plan: %s\n  %s", joined, strings.Join(faults, "\n  "))
	}
}

// The undated tail. `added_at IS NULL` reads as an equality on the index's
// leading column, so this stays a seek rather than becoming a walk from the top
// of the index on every tail page.
func TestRecentWorksNullTailPlanIsASeek(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	joined := recentWorksPlan(t, s, OwnerScope(1), RecentWorksCursor{NullTail: true, ID: 14})
	const want = "SEARCH w USING INDEX ix_work_added (added_at=? AND id<?)"
	if !strings.Contains(joined, want) {
		t.Errorf("the undated tail is not a seek.\n  got:  %s\n  want: %s", joined, want)
	}
	if faults := recentWorksPlanFaults(joined); len(faults) > 0 {
		t.Errorf("the tail plan is wrong:\n  plan: %s\n  %s", joined, strings.Join(faults, "\n  "))
	}
}

// The SCOPED plan, which is the one a multi-user install runs. The EXISTS over
// service_item_link must be an inner seek on ix_sil_work and must not displace
// ix_work_added as the driver.
func TestRecentWorksScopedPlanKeepsTheIndex(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)

	joined := recentWorksPlan(t, s, Scope{UserID: 1, InstanceIDs: []int64{1, 2}}, RecentWorksCursor{
		AddedAt: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4,
	})
	if faults := recentWorksPlanFaults(joined); len(faults) > 0 {
		t.Errorf("the scoped plan is wrong:\n  plan: %s\n  %s", joined, strings.Join(faults, "\n  "))
	}
	// The inner alias is derived from the correlated column (scopeLinkAlias),
	// so this derives it too: a literal `sil` is a prefix of every derived alias
	// and would keep matching without covering anything.
	if !planHas(joined, "SEARCH "+scopeLinkAlias("w.id")) ||
		!planHas(joined, "ix_sil_work") {
		t.Errorf("the scope EXISTS is not a seek on ix_sil_work: %s", joined)
	}
	if strings.Contains(joined, "SCAN sil") {
		t.Errorf("the scope EXISTS degraded to a scan of every service link: %s", joined)
	}
}

// FIRING THE GUARD. Three arms, each breaking a different thing the guard
// protects and each running the SAME recentWorksPlanFaults the tests above run.
func TestRecentWorksPlanGuardFires(t *testing.T) {
	// Arm 1 — the index goes away. This is the regression the assertion exists
	// for: someone drops or renames ix_work_added and Home silently starts
	// sorting the whole catalogue on every page.
	t.Run("index dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedRecentCorpus(t, s)
		// ⚠️ dropIndexesAndConfirm, never a bare DROP: recentWorksPlan below
		// EXPLAINs on the READ pool, and MEASURED on this tree, a read-pool
		// connection that has already planned this statement keeps answering
		// with the PRE-DROP plan indefinitely — repeating the EXPLAIN does not
		// shake it loose. This arm survives a bare drop today only because
		// nothing plans before it; that is an accident of ordering, and one seed
		// refactor that plans on its way past turns this arm into a measurement
		// of a schema that no longer exists. The confirming read inside is both
		// the proof and the cure. Undocumented behaviour, characterised
		// empirically — the helper in store_test.go carries the measurement and
		// its caveats.
		dropIndexesAndConfirm(t, s, "ix_work_added")

		joined := recentWorksPlan(t, s, OwnerScope(1), RecentWorksCursor{
			AddedAt: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4,
		})
		faults := recentWorksPlanFaults(joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan with ix_work_added dropped:\n  %s", joined)
		}
		t.Logf("guard fired with ix_work_added dropped:\n  plan:   %s\n  faults: %v", joined, faults)
	})

	// Arm 2 — the unary plus on `+w.kind` removed. It is the single character
	// standing between this read and a full sort of every non-deleted work of
	// the six kinds, and it is exactly the character a later reader "cleans up".
	t.Run("unary plus removed", func(t *testing.T) {
		s := newTestStore(t)
		seedRecentCorpus(t, s)

		query, args := recentWorksSQL(OwnerScope(1), RecentWorksCursor{
			AddedAt: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4,
		}, RecentWorksDefaultLimit)
		broken := strings.Replace(query, "+w.kind IN", "w.kind IN", 1)
		if broken == query {
			t.Fatalf("the shipped statement no longer contains `+w.kind IN`, so this arm "+
				"is asserting against nothing:\n%s", query)
		}
		plan, err := db.QueryPlan(t.Context(), s.DB().Read(), broken, args...)
		if err != nil {
			t.Fatalf("QueryPlan: %v", err)
		}
		joined := strings.Join(plan, " | ")
		faults := recentWorksPlanFaults(joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed the plan without the unary plus:\n  %s", joined)
		}
		t.Logf("guard fired without the unary plus:\n  plan:   %s\n  faults: %v", joined, faults)
	})

	// Arm 3 — a real temp b-tree, so the ordering half of the guard is executed
	// rather than merely written down.
	t.Run("ordering not served by the index", func(t *testing.T) {
		s := newTestStore(t)
		seedRecentCorpus(t, s)
		plan, err := db.QueryPlan(t.Context(), s.DB().Read(),
			`SELECT id, title FROM work WHERE deleted_at IS NULL ORDER BY title LIMIT 50`)
		if err != nil {
			t.Fatalf("QueryPlan: %v", err)
		}
		joined := strings.Join(plan, " | ")
		faults := recentWorksPlanFaults(joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan that sorts:\n  %s", joined)
		}
		t.Logf("guard fired on an unindexed ordering:\n  plan:   %s\n  faults: %v", joined, faults)
	})
}

// The kind allowlist in the SQL and the switch in mediaTypeOf are two lists that
// must stay equal — a kind the query admits and the switch does not map renders
// a blank Type cell. recentWorksPage turns that into an error; this proves the
// two lists agree today, which is what keeps that error unreachable.
func TestRecentWorkKindsAllMapToAMediaType(t *testing.T) {
	for _, kind := range recentWorkKinds {
		if mediaTypeOf(kind, sql.NullInt64{}) == "" {
			t.Errorf("recentWorkKinds admits %q and mediaTypeOf does not map it", kind)
		}
	}
	// And the excluded ones stay excluded, named individually so removing one
	// from the allowlist is not silently symmetrical.
	for _, kind := range []string{"person", "season", "episode", "track", "comic_issue", "game"} {
		for _, admitted := range recentWorkKinds {
			if kind == admitted {
				t.Errorf("%q is in recentWorkKinds and must not be (ADR-0033 / ADR-0030 / §17.2)", kind)
			}
		}
	}
}
