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
// The corpus is recent_test.go's — deliberately, because this read and Block C
// are two windows over ONE corpus and the tests that matter most here compare
// the two. seedBrowseCorpus adds only what the browse read can see and Block C
// cannot: a third library so the §17.8 two-libraries-over-one-kind topology is
// real, a work that is a member of TWO of them, distinct popularity values, and
// the one edition shape the Audiobooks predicate can get wrong.
//
// ⚠️ A NOTE FOR ANYONE DOING PLAN WORK ON THIS SCHEMA. Build the scratch
// database with THIS package's engine — github.com/ncruces/go-sqlite3 with
// ext/fts5 registered, which is what internal/db.Open does and what these tests
// run against; it reports SQLite 3.53.4. The SYSTEM `sqlite3` CLI cannot build
// this schema at all: migration 0005 contains `RAISE(ABORT, 'a' || 'b')` and the
// CLI rejects it, so a plan measured by piping the migrations into `sqlite3` is
// not a plan for this schema — it is a plan for whatever subset loaded before
// the error. `go test ./internal/store -run TestBrowseWorks` is the cheap way in.
// ─────────────────────────────────────────────────────────────────────────────

func seedBrowseCorpus(t *testing.T, s *Store) {
	t.Helper()
	seedRecentCorpus(t, s)

	stmts := []string{
		// §17.8's flagship topology: two libraries over ONE kind. Library 2 is
		// the corpus's `book` library; 3 is the second one, and work 3 is in
		// BOTH. That work is the reason the library scope is an EXISTS rather
		// than a join — a join returns it once per matching membership row, one
		// row per library it is filed in, while a browse row is work-keyed.
		//
		// ⚠️ `edition_id` IS THE `0` SENTINEL ON EVERY ROW, because that is the
		// only value the tree can actually produce: catalogue.go's item pass is
		// the only production writer and hardcodes it (REVIEW-LOG.md LS-213).
		// The duplicate this fixture creates is therefore per-LIBRARY and does
		// not depend on edition grain ever arriving.
		`INSERT INTO library (id, user_id, name, slug, kind) VALUES (3, 0, 'Audiobooks', 'audiobooks', 'book')`,
		`INSERT INTO library_member (library_id, sort_title, work_id, edition_id) VALUES (2, 'piranesi', 3, 0)`,
		`INSERT INTO library_member (library_id, sort_title, work_id, edition_id) VALUES (3, 'piranesi', 3, 0)`,
		`INSERT INTO library_member (library_id, sort_title, work_id, edition_id) VALUES (3, 'dune', 5, 0)`,

		// A book with an audiobook edition AND an edition whose format the
		// upstream did not report. Under `MIN(format = 'audiobook')` — the
		// expression the Type column is rendered from — the NULL is ignored and
		// this work is an AUDIOBOOK. It is the row that separates the shipped
		// `format IS NOT NULL AND format <> ?` predicate from the tidier-looking
		// `format IS NOT ?`, which would drop it out of the Audiobooks grid
		// while its own Type cell still said Audiobooks.
		work(20, "book", "A Part-Catalogued Audiobook", "2026-07-29 10:00:00", ""),
		`INSERT INTO edition (id, work_id, format) VALUES (20, 20, 'audiobook')`,
		`INSERT INTO edition (id, work_id, format) VALUES (21, 20, NULL)`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (2, 20, 'r20', 'series', '2026-08-16 12:00:00')`,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (2, 'w20', 20)`,

		// Distinct popularity, so the popularity order is an order and not a
		// tie broken by id. Left at the column default (0) everywhere else,
		// which is itself worth having: most of a real catalogue is unscored.
		`UPDATE work SET popularity = 9.5 WHERE id = 1`,
		`UPDATE work SET popularity = 7.25 WHERE id = 3`,
		`UPDATE work SET popularity = 7.25 WHERE id = 5`,
		`UPDATE work SET popularity = 0.5 WHERE id = 10`,
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

// browseAll walks every page and returns the titles in order. Walking rather
// than reading one page is the point: a keyset bug is invisible on page one.
func browseAll(t *testing.T, s *Store, scope Scope, f WorksFilter, limit int) []string {
	t.Helper()
	var out []string
	cur := WorksCursor{Sort: f.sort()}
	for page := 0; ; page++ {
		if page > 50 {
			t.Fatalf("the walk did not terminate after 50 pages: %v", out)
		}
		rows, next, err := s.ListWorks(t.Context(), scope, f, cur, limit)
		if err != nil {
			t.Fatalf("ListWorks: %v", err)
		}
		out = append(out, titles(rows)...)
		if !next.Value.Valid && !next.NullTail {
			return out
		}
		// Round-trip the cursor through the wire codec on every page, so the
		// encoding is exercised by the paging tests rather than only by its own.
		decoded, err := DecodeWorksCursor(EncodeWorksCursor(next))
		if err != nil {
			t.Fatalf("cursor round-trip: %v", err)
		}
		cur = decoded
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// What the read returns.
// ─────────────────────────────────────────────────────────────────────────────

// With no filter at all this is Block C's corpus in Block C's order, plus the
// one work seedBrowseCorpus adds. That equality is the contract: the grid and
// the recently-added table must not disagree about what is in the library.
func TestBrowseWorksUnfilteredIsBlockCsCorpus(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	want := append(append([]string{}, recentCorpusOrder[:9]...),
		"A Part-Catalogued Audiobook")
	want = append(want, recentCorpusOrder[9:]...)

	if got := browseAll(t, s, OwnerScope(1), WorksFilter{}, 3); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the unfiltered browse order is not Block C's.\n got: %v\nwant: %v", got, want)
	}
}

// ⚠️ THE TEST THIS FILE EXISTS FOR. The media-type FILTER (browseKinds plus the
// audiobook predicate) and the media-type RENDERER (mediaTypeOf, over the MIN
// subquery) are two expressions of §17.2's six-row table, in two languages, and
// nothing but this test makes them agree. A filter that admits a work whose Type
// cell then says something else looks like a rendering bug and is a query bug.
//
// Executed against the whole corpus, both directions, for all six types.
func TestBrowseFilterAgreesWithMediaTypeOf(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	// Block C's own answer, which is mediaTypeOf's, for every visible work.
	all, _, err := ListWorksHelperRecent(t, s)
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string][]string{}
	for _, w := range all {
		byType[w.MediaType] = append(byType[w.MediaType], w.Title)
	}

	seen := 0
	for _, mt := range []string{
		MediaTypeMovies, MediaTypeTV, MediaTypeMusic,
		MediaTypeEbooks, MediaTypeAudiobooks, MediaTypeComics,
	} {
		t.Run(mt, func(t *testing.T) {
			got := browseAll(t, s, OwnerScope(1), WorksFilter{MediaType: mt}, 3)
			want := byType[mt]
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("media_type=%s disagrees with the Type column mediaTypeOf renders.\n"+
					" filter says: %v\n renderer says: %v", mt, got, want)
			}
			// And every row the filter returned renders as the type asked for.
			rows, _, err := s.ListWorks(t.Context(), OwnerScope(1), WorksFilter{MediaType: mt}, WorksCursor{}, 100)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				if r.MediaType != mt {
					t.Errorf("media_type=%s returned %q, whose Type cell says %q", mt, r.Title, r.MediaType)
				}
			}
		})
		seen += len(byType[mt])
	}
	if seen != len(all) {
		t.Errorf("the six media types cover %d of %d visible works", seen, len(all))
	}

	// The specific row the two predicates can disagree on, named so a failure
	// above is diagnosable: work 20 has an audiobook edition and one whose
	// format is NULL, and MIN() ignores NULLs.
	found := false
	for _, w := range byType[MediaTypeAudiobooks] {
		if w == "A Part-Catalogued Audiobook" {
			found = true
		}
	}
	if !found {
		t.Error("the NULL-format fixture is not an audiobook to mediaTypeOf, so this test " +
			"is no longer covering the `format IS NOT NULL AND format <> ?` distinction")
	}
}

// ListWorksHelperRecent reads Block C whole, so the test above has mediaTypeOf's
// answer for every visible work without reimplementing it.
func ListWorksHelperRecent(t *testing.T, s *Store) ([]RecentWork, RecentWorksCursor, error) {
	t.Helper()
	return s.ListRecentWorks(t.Context(), OwnerScope(1), RecentWorksCursor{}, RecentWorksMaxLimit)
}

// An unrecognised media type is an ERROR, never a silently unfiltered page. A
// filter that widens on a typo turns "show me my comics" into "show me
// everything", which looks like it worked.
func TestBrowseWorksUnknownMediaTypeIsAnError(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	for _, mt := range []string{"moovies", "MOVIES", "book", "series", "tv ", "person", "game"} {
		if _, _, err := s.ListWorks(t.Context(), OwnerScope(1),
			WorksFilter{MediaType: mt}, WorksCursor{}, 10); err == nil {
			t.Errorf("media_type=%q was accepted", mt)
		}
	}
	// `kind` values are NOT media types, and two of them collide with real
	// media-type spellings in the obvious wrong direction. This is the
	// two-vocabularies-one-name failure the parameter is named `media_type` to
	// avoid: 'series' is a work.kind and the media type is 'tv'.
	if _, _, err := s.ListWorks(t.Context(), OwnerScope(1),
		WorksFilter{MediaType: "series"}, WorksCursor{}, 10); err == nil {
		t.Error("the work.kind 'series' was accepted as a media type")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The library scope — ADR-0051's EXISTS.
// ─────────────────────────────────────────────────────────────────────────────

// One library returns its members and nothing else, in the global order.
func TestBrowseWorksLibraryScopeSelectsMembersOnly(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	got := browseAll(t, s, OwnerScope(1), WorksFilter{LibraryIDs: []int64{1}}, 3)
	// Library 1 holds the odd-numbered works (the corpus files by instance),
	// minus the ones Block C's kind allowlist excludes: 7 is soft-deleted, 9 is
	// a comic_issue.
	want := []string{"Piranesi", "Dune", "Severance", "An Undated Manga"}
	want = append([]string{"Berserk"}, want...)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("library 1 browse.\n got: %v\nwant: %v", got, want)
	}
}

// ⚠️ THE PROPERTY A JOIN CANNOT PROMISE. Work 3 is a member of library 2 AND of
// library 3 — §17.8's Ebooks/Audiobooks split, one upstream library offered
// twice — so it carries TWO membership rows, one per library, both at the `0`
// edition sentinel every production write uses. A browse row is work-keyed, so
// under a join that work comes back TWICE on one page — and the duplicate is
// silent, because both copies are correct rows. Under the EXISTS it comes back
// once.
func TestBrowseWorksMultiLibraryReturnsAWorkOnce(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	got := browseAll(t, s, OwnerScope(1), WorksFilter{LibraryIDs: []int64{2, 3}}, 3)
	seen := map[string]int{}
	for _, ti := range got {
		seen[ti]++
	}
	for ti, n := range seen {
		if n != 1 {
			t.Errorf("%q came back %d times from a two-library scope; the shape is a join, "+
				"not the EXISTS ADR-0051 decided on", ti, n)
		}
	}
	// And it really is in both libraries, so the test is not passing vacuously.
	var n int
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT count(*) FROM library_member WHERE work_id = 3 AND library_id IN (2, 3)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("the fixture work is in %d of the two libraries, want 2", n)
	}
	if !strings.Contains(strings.Join(got, "|"), "Piranesi") {
		t.Error("the doubly-filed work is missing from the two-library page entirely")
	}
}

// An empty LibraryIDs is "every library", not "no library" — the chip's cleared
// state is the whole catalogue. Stated as a test because the opposite reading is
// the one that fails closed on a screen the user did not scope.
func TestBrowseWorksEmptyLibraryFilterIsEveryLibrary(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	with := browseAll(t, s, OwnerScope(1), WorksFilter{LibraryIDs: []int64{}}, 5)
	without := browseAll(t, s, OwnerScope(1), WorksFilter{}, 5)
	if strings.Join(with, "|") != strings.Join(without, "|") {
		t.Errorf("an empty library filter is not the same as no filter:\n %v\n %v", with, without)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Access scope. store.go rule 2.
// ─────────────────────────────────────────────────────────────────────────────

// A scope a query accepts and never filters on is indistinguishable from no
// scope at all, so this breaks the predicate and watches the assertion catch it
// rather than trusting that the parameter is threaded.
func TestBrowseWorksScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	scope := Scope{UserID: 1, InstanceIDs: []int64{1}}
	rows, _, err := s.ListWorks(t.Context(), scope, WorksFilter{}, WorksCursor{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := titles(rows)
	if len(got) == 0 {
		t.Fatal("instance 1 alone returned nothing; the fixture is wrong")
	}
	for _, ti := range got {
		if ti == "Vinland Saga" {
			t.Fatal("a work replicated only by instance 2 came back under a scope of instance 1")
		}
	}

	// FIRING THE GUARD: the same page under the full scope must be strictly
	// larger. Without that, an assertion over a filtered list would pass
	// against a predicate that returns nothing at all.
	full, _, err := s.ListWorks(t.Context(), OwnerScope(1), WorksFilter{}, WorksCursor{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) <= len(got) {
		t.Errorf("the scoped page (%d) is not smaller than the unscoped one (%d)", len(got), len(full))
	}
}

// "No visible instances" must mean no rows, never all rows.
func TestBrowseWorksFailsClosedOnAnEmptyScope(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	for _, f := range []WorksFilter{
		{},
		{MediaType: MediaTypeComics},
		{LibraryIDs: []int64{1, 2, 3}},
		{Sort: WorksSortPopularity},
	} {
		rows, next, err := s.ListWorks(t.Context(), Scope{UserID: 9}, f, WorksCursor{}, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Errorf("a scope with no visible instances returned %d rows for %+v", len(rows), f)
		}
		if next.Value.Valid || next.NullTail {
			t.Errorf("a scope with no visible instances minted a next cursor for %+v", f)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The three orders, and their keyset walks.
// ─────────────────────────────────────────────────────────────────────────────

// Every order pages the whole filtered corpus exactly once, at a page size that
// forces a keyset handoff on every page — including, for added_at, the handoff
// across the undated tail.
func TestBrowseWorksPagesEachOrderExactlyOnce(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	full, _, err := s.ListWorks(t.Context(), OwnerScope(1), WorksFilter{}, WorksCursor{}, 200)
	if err != nil {
		t.Fatal(err)
	}
	want := len(full)

	for _, tc := range []struct {
		name string
		f    WorksFilter
	}{
		{"added_at", WorksFilter{Sort: WorksSortAdded}},
		{"popularity", WorksFilter{Sort: WorksSortPopularity}},
		{"sort_title, one kind", WorksFilter{Sort: WorksSortTitle, MediaType: MediaTypeComics}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, limit := range []int{1, 2, 3, 5, 100} {
				got := browseAll(t, s, OwnerScope(1), tc.f, limit)
				seen := map[string]int{}
				for _, ti := range got {
					seen[ti]++
					if seen[ti] > 1 {
						t.Fatalf("limit=%d repeated %q: %v", limit, ti, got)
					}
				}
				if tc.f.MediaType == "" && len(got) != want {
					t.Fatalf("limit=%d walked %d works, want %d: %v", limit, len(got), want, got)
				}
			}
		})
	}
}

// The orders are the orders they claim to be, read off the rows rather than off
// the SQL.
func TestBrowseWorksOrdersAreTheOrders(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	// popularity DESC, id DESC. Work 1 (9.5) leads; 3 and 5 tie at 7.25 and are
	// separated by id DESC, so 5 comes before 3; 10 (0.5) then the 0s.
	got := browseAll(t, s, OwnerScope(1), WorksFilter{Sort: WorksSortPopularity}, 4)
	if len(got) < 4 || got[0] != "Berserk" || got[1] != "Dune" || got[2] != "Piranesi" ||
		got[3] != "Train Dreams" {
		t.Errorf("the popularity order is wrong at the head: %v", got[:min(4, len(got))])
	}

	// sort_title ASC within one kind. seedRecentCorpus lower-cases the title
	// into sort_title, so this is alphabetical.
	got = browseAll(t, s, OwnerScope(1),
		WorksFilter{Sort: WorksSortTitle, MediaType: MediaTypeComics}, 2)
	want := []string{"An Undated Manga", "Another Undated Manga", "Berserk", "Vinland Saga"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the sort_title order is wrong.\n got: %v\nwant: %v", got, want)
	}
}

// ⚠️ THE ONE HOLE, PINNED AS A HOLE. ix_work_kind_sort is (kind, sort_title, id)
// and SQLite cannot supply ORDER BY from an index whose leading column is
// constrained by IN, so an alphabetical page over more than one kind has no
// index behind it. media_type=music is two kinds and no media type at all is
// six. ListWorks refuses both rather than serving a temp b-tree over the
// catalogue on a render path.
//
// t.Errorf and not t.Logf: if this ever starts working — a new index, or a
// planner that can merge — that is good news and makes ListWorks's own comment,
// ADR-0051 and http-api.md wrong, so it must fail and force the edit.
func TestBrowseWorksTitleSortNeedsExactlyOneKind(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	for _, mt := range []string{"", MediaTypeMusic} {
		_, _, err := s.ListWorks(t.Context(), OwnerScope(1),
			WorksFilter{Sort: WorksSortTitle, MediaType: mt}, WorksCursor{}, 10)
		if err == nil {
			t.Errorf("sort_title over media_type=%q was served. If an index now supports it, "+
				"update ListWorks, ADR-0051 and docs/reference/http-api.md in the same change.", mt)
		}
	}
	// The single-kind media types are all served.
	for _, mt := range []string{
		MediaTypeMovies, MediaTypeTV, MediaTypeComics, MediaTypeEbooks, MediaTypeAudiobooks,
	} {
		if _, _, err := s.ListWorks(t.Context(), OwnerScope(1),
			WorksFilter{Sort: WorksSortTitle, MediaType: mt}, WorksCursor{}, 10); err != nil {
			t.Errorf("sort_title over media_type=%s: %v", mt, err)
		}
	}
	// And `year`, which library.default_sort legally holds, is not a WorksSort
	// at all — it has no index on `work` and is refused before any statement is
	// rendered.
	if _, _, err := s.ListWorks(t.Context(), OwnerScope(1),
		WorksFilter{Sort: WorksSort("year")}, WorksCursor{}, 10); err == nil {
		t.Error("sort=year was served, and work.year has no index")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The cursor codec.
// ─────────────────────────────────────────────────────────────────────────────

func TestBrowseWorksCursorRoundTrips(t *testing.T) {
	cases := []WorksCursor{
		{Sort: WorksSortAdded, Value: sql.NullString{String: "2026-08-10 10:00:00", Valid: true}, ID: 7},
		{Sort: WorksSortAdded, NullTail: true, ID: 14},
		{Sort: WorksSortTitle, Value: sql.NullString{String: "a title. with dots. in it", Valid: true}, ID: 3},
		{Sort: WorksSortPopularity, Value: sql.NullString{String: "7.25", Valid: true}, ID: 5},
	}
	for _, c := range cases {
		got, err := DecodeWorksCursor(EncodeWorksCursor(c))
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if got != c {
			t.Errorf("round trip: got %+v want %+v", got, c)
		}
	}

	// The token must survive a query string unescaped: the added_at payload
	// carries store.go's timeLayout, which has a SPACE in it because SQLite's
	// datetime() does.
	if tok := EncodeWorksCursor(cases[0]); strings.ContainsAny(tok, " +/=&?#%") {
		t.Errorf("cursor %q carries a character a query string cannot hold unescaped", tok)
	}

	// ⚠️ A CURSOR MINTED UNDER ONE SORT MUST NOT DECODE AS ANOTHER. All three
	// tokens parse; what stops the replay is the discriminator, and the store
	// refuses a mismatch rather than paging from a position that means
	// something else in the requested order.
	for _, c := range cases {
		got, err := DecodeWorksCursor(EncodeWorksCursor(c))
		if err != nil {
			t.Fatal(err)
		}
		if got.Sort != c.Sort {
			t.Errorf("cursor lost its sort: %+v", got)
		}
	}

	// Block C's tokens are NOT this endpoint's tokens, in both directions.
	if _, err := DecodeWorksCursor(EncodeRecentWorksCursor(
		RecentWorksCursor{AddedAt: sql.NullString{String: "2026-08-10 10:00:00", Valid: true}, ID: 7},
	)); err == nil {
		t.Error("a Block C cursor was accepted by the browse decoder")
	}
	if _, err := DecodeRecentWorksCursor(EncodeWorksCursor(cases[0])); err == nil {
		t.Error("a browse cursor was accepted by Block C's decoder")
	}

	// STRICT. A cursor that will not parse is an error, never a silent reset to
	// page one — that turns a stale bookmark into a Load-more loop over page one.
	bad := []string{"not base64!!", "***"}
	for _, payload := range []string{
		"x", "1", "1a", "1av", "1zv5.x", "1avabc.x", "1av0.x", "1av5", "1av5.",
		"1an", "1anx", "1an0", "1tn5", "1pn5", "2av5.x", "1ax5.x",
	} {
		bad = append(bad, base64.RawURLEncoding.EncodeToString([]byte(payload)))
	}
	for _, b := range bad {
		if _, err := DecodeWorksCursor(b); err == nil {
			t.Errorf("cursor %q was accepted", b)
		}
	}

	// The message must not echo the caller's text back — httpapi renders it
	// into a response body.
	_, err := DecodeWorksCursor(base64.RawURLEncoding.EncodeToString([]byte("9<script>")))
	if err == nil {
		t.Fatal("a junk cursor was accepted")
	}
	if strings.Contains(err.Error(), "script") {
		t.Errorf("the cursor error echoes the caller's token back: %v", err)
	}
}

// The replay is refused at the STORE, not only at the boundary — a popularity
// position replayed on the added_at order decodes cleanly and would mis-page in
// silence.
func TestBrowseWorksRefusesACursorFromAnotherSort(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	_, next, err := s.ListWorks(t.Context(), OwnerScope(1),
		WorksFilter{Sort: WorksSortPopularity}, WorksCursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Value.Valid {
		t.Fatal("the fixture produced no second page")
	}
	if _, _, err := s.ListWorks(t.Context(), OwnerScope(1),
		WorksFilter{Sort: WorksSortAdded}, next, 2); err == nil {
		t.Error("a popularity cursor was replayed on the added_at order")
	}
	// The undated tail belongs to added_at alone.
	if _, _, err := s.ListWorks(t.Context(), OwnerScope(1),
		WorksFilter{Sort: WorksSortPopularity},
		WorksCursor{Sort: WorksSortPopularity, NullTail: true, ID: 5}, 2); err == nil {
		t.Error("a tail cursor was accepted under the popularity order")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Slug resolution.
// ─────────────────────────────────────────────────────────────────────────────

func TestLibraryIDsBySlug(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	ids, err := s.LibraryIDsBySlug(t.Context(), OwnerScope(0), []string{"books", "audiobooks"})
	if err != nil {
		t.Fatalf("resolving two real slugs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("resolved %v, want two ids", ids)
	}

	// An unknown slug is an error, never a dropped filter — dropping it widens
	// the page, and dropping all of them removes the scope entirely.
	if _, err := s.LibraryIDsBySlug(t.Context(), OwnerScope(0), []string{"books", "nope"}); err == nil {
		t.Error("an unknown slug was ignored")
	}
	if _, err := s.LibraryIDsBySlug(t.Context(), OwnerScope(0), []string{"nope"}); err == nil {
		t.Error("an entirely unknown slug set resolved to no filter at all")
	}
	// And the error does not name it back: a caller must not be able to probe
	// library names by watching the message change.
	_, err = s.LibraryIDsBySlug(t.Context(), OwnerScope(0), []string{"secret-library"})
	if err == nil || strings.Contains(err.Error(), "secret-library") {
		t.Errorf("the error echoes the slug back: %v", err)
	}

	// The reserved Unfiled row is never offered in the scope chip (migration
	// 0005), so its slug resolves to nothing.
	if _, err := s.LibraryIDsBySlug(t.Context(), OwnerScope(0), []string{"unfiled"}); err == nil {
		t.Error("the reserved Unfiled library was resolvable through ?lib=")
	}

	// Nothing asked for, nothing resolved, no error: an absent chip is not an
	// empty chip.
	ids, err = s.LibraryIDsBySlug(t.Context(), OwnerScope(0), nil)
	if err != nil || len(ids) != 0 {
		t.Errorf("no slugs = %v, %v; want no ids and no error", ids, err)
	}

	// Scope. A caller who can see no instance can see no library that names one.
	if _, err := s.LibraryIDsBySlug(t.Context(),
		Scope{UserID: 0}, []string{"books"}); err == nil {
		t.Error("a slug resolved under a scope with no visible instances")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The query plans. These are the acceptance evidence for ADR-0051, and they
// EXPLAIN the statement that ships — browseWorksSQL itself, not a lookalike
// pasted into a test (docs/DEVELOPMENT.md §11 rule 1).
//
// ⚠️ EVERY GUARD BELOW PINS THE NO-STATISTICS PLANNER, WHICH IS THE ONE THIS
// SUITE SEES AND NOT THE ONE THE BINARY USUALLY RUNS. A test database is fresh
// and has no `sqlite_stat1`; a production database has one, because the importer
// runs ANALYZE after a full import. The difference is not cosmetic — the unary
// plus on `+w.kind` changes the plan ONLY when statistics are absent — so these
// assertions are a claim about the conservative planner and never a measurement
// of production. A plan that holds with no statistics holds with them; the
// wall clock, with statistics and at real row counts, is `make bench`'s.
//
// ⚠️ A GUARD MUST MUTATE BEFORE IT MEASURES, NEVER AFTER — THE PLAN IS CACHED.
// Every arm below builds a fresh store, breaks something, and only then
// EXPLAINs, and that order is load-bearing rather than stylistic. EXPLAIN the
// same statement twice with a DDL change in between and the second answer is
// the FIRST plan: measured here at 64 of 64 repeats, a plan taken before a
// `DROP INDEX ix_work_added` and taken again after it said
// `SEARCH w USING INDEX ix_work_added` both times, naming an index that no
// longer exists. (browseWorksPlan reads through the read pool while the drop
// goes to the single writer, so the connection answering holds a statement
// compiled against the pre-DDL schema; the exact layer that caches it does not
// matter, the observable does.)
//
// So every assertion made after a mutation on an already-measured statement is
// an assertion about the schema as it was BEFORE the mutation. The loud version
// of that mistake is a firing arm that reports "the guard passed a plan with
// ix_work_added dropped" when the guard was never shown the dropped index at
// all. The quiet version is the one to fear: a POSITIVE assertion — measure,
// change something, assert the plan still seeks the way it did — is green by
// construction and pins nothing, because it re-reads its own first answer.
// Every arm here is already on the safe side; this note is so the next one is
// too. Measure once, on a store that is already in the state under test.
//
// ⚠️ ORDERING IS NO LONGER THE ONLY DEFENCE, AND MUST NOT BE. Every drop below
// goes through dropIndexesAndConfirm (store_test.go), which proves ON THE READ
// POOL that the drop landed before any EXPLAIN runs. Ordering alone is what made
// these arms correct by ACCIDENT: measured, a pool that has never planned the
// statement sees the drop immediately, so it is PRIMING that arms the trap — and
// any refactor that plans the shipped statement before the drop, a seed helper
// that grows an assertion say, re-breaks all of them at once without touching
// this file.
// ─────────────────────────────────────────────────────────────────────────────

// browseWorksPlanFaults is the guard, factored out so the tests that fire it
// deliberately run the SAME predicate the passing tests run. A guard asserted
// one way and fired against a different condition has proved nothing.
//
// index is the ordering index the shape under test must walk.
func browseWorksPlanFaults(index, plan string) []string {
	var faults []string
	if !strings.Contains(plan, "USING INDEX "+index) {
		faults = append(faults, "does not use "+index)
	}
	// A table scan, as distinct from the ordered INDEX scan a first page is.
	if strings.Contains(plan, "SCAN w") && !strings.Contains(plan, "SCAN w USING INDEX") {
		faults = append(faults, "degraded to a scan of the work table")
	}
	// The whole point of the ordering index is that it supplies the order. A
	// sort here is the grid paying for the entire filtered corpus on every page.
	if strings.Contains(plan, "TEMP B-TREE") {
		faults = append(faults, "needs a temp b-tree for the ordering")
	}
	// ADR-0051: the library scope is a PROBE, never the driver. A scan of
	// library_member means the shape turned back into the member-driven join.
	if strings.Contains(plan, "SCAN lm") {
		faults = append(faults, "scans library_member instead of probing it")
	}
	// schema.md §1: assert SEARCH … USING INDEX, never COVERING INDEX on the
	// driver. The SELECT list carries title, year, have_count, want_count and
	// availability, so a covering plan means the query changed.
	if strings.Contains(plan, "COVERING INDEX "+index) {
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

// browseMembershipProbeFaults is ADR-0051's own guard: the library scope must
// be a COVERING two-column probe on ix_libmem_work and never the driver.
//
// It is covering for free — library_member is WITHOUT ROWID, so a secondary
// index carries the primary key, library_id included, in its own entries — and
// asserting only `work_id=?` would stay green if the library filter moved out of
// the index and became a row-by-row check. It is factored out so the arm that
// fires it deliberately runs the same predicate the passing test runs.
func browseMembershipProbeFaults(plan string) []string {
	var faults []string
	if !strings.Contains(plan, "SEARCH lm EXISTS USING COVERING INDEX ix_libmem_work") {
		faults = append(faults, "the library scope is not a covering probe on ix_libmem_work")
	}
	if !strings.Contains(plan, "work_id=?") || !strings.Contains(plan, "library_id=?") {
		faults = append(faults,
			"the membership probe does not constrain both work_id and library_id, so it "+
				"fetches every membership row of every candidate work and filters afterwards")
	}
	if strings.Contains(plan, "SCAN lm") {
		faults = append(faults, "library_member is scanned rather than probed")
	}
	return faults
}

func browseWorksPlan(t *testing.T, s *Store, scope Scope, f WorksFilter, cur WorksCursor) string {
	t.Helper()
	query, args, err := browseWorksSQL(scope, f, cur, BrowseWorksDefaultLimit)
	if err != nil {
		t.Fatalf("browseWorksSQL: %v", err)
	}
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// ⚠️ THE ASSERTION ADR-0051 IS MADE OF, and the one §6.5 asks for by name:
// the plan in BOTH library topologies — one library per kind, and two libraries
// over one kind — for the added_at order. §6.5 says "only the second is the
// interesting one", and it is: a multi-value `?lib=a,b` puts an IN on
// library_member's leading key column, which destroys the ordered index in every
// member-driven shape. Under the work-driven EXISTS the ordering comes off
// `work` and the IN lands on a probe, which is a hypothesis until the plan says
// so — and this is where it says so.
func TestBrowseWorksLibraryScopedPlanKeepsTheAddedOrder(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	dated := WorksCursor{Value: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4}

	for _, tc := range []struct {
		name string
		libs []int64
	}{
		{"one library per kind", []int64{1}},
		{"two libraries over one kind", []int64{2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, page := range []struct {
				name string
				cur  WorksCursor
			}{
				{"first page", WorksCursor{}},
				{"keyset", dated},
				{"undated tail", WorksCursor{NullTail: true, ID: recentWorksTailStart}},
			} {
				t.Run(page.name, func(t *testing.T) {
					joined := browseWorksPlan(t, s, OwnerScope(1),
						WorksFilter{LibraryIDs: tc.libs}, page.cur)

					if faults := browseWorksPlanFaults("ix_work_added", joined); len(faults) > 0 {
						t.Errorf("the library-scoped added_at plan is wrong:\n  plan: %s\n  %s",
							joined, strings.Join(faults, "\n  "))
					}
					if faults := browseMembershipProbeFaults(joined); len(faults) > 0 {
						t.Errorf("the library scope is not ADR-0051's probe:\n  plan: %s\n  %s",
							joined, strings.Join(faults, "\n  "))
					}
					t.Logf("%s / %s: %s", tc.name, page.name, joined)
				})
			}
		})
	}
}

// The unscoped and media-type-filtered shapes, one per index, so a regression in
// any of the three orders is caught by name.
func TestBrowseWorksPlansPerOrder(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	for _, tc := range []struct {
		name  string
		index string
		f     WorksFilter
		cur   WorksCursor
		want  string
	}{
		{
			name: "added_at, no filter, keyset", index: "ix_work_added",
			f:    WorksFilter{},
			cur:  WorksCursor{Value: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4},
			want: "SEARCH w USING INDEX ix_work_added (added_at<?)",
		},
		{
			name: "added_at, undated tail", index: "ix_work_added",
			f:    WorksFilter{},
			cur:  WorksCursor{NullTail: true, ID: 14},
			want: "SEARCH w USING INDEX ix_work_added (added_at=? AND id<?)",
		},
		{
			name: "added_at, media_type=comics", index: "ix_work_added",
			f:    WorksFilter{MediaType: MediaTypeComics},
			cur:  WorksCursor{Value: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4},
			want: "SEARCH w USING INDEX ix_work_added (added_at<?)",
		},
		{
			name: "sort_title, one kind", index: "ix_work_kind_sort",
			f:    WorksFilter{Sort: WorksSortTitle, MediaType: MediaTypeComics},
			cur:  WorksCursor{Sort: WorksSortTitle, Value: sql.NullString{String: "b", Valid: true}, ID: 1},
			want: "SEARCH w USING INDEX ix_work_kind_sort (kind=? AND sort_title>?)",
		},
		{
			name: "popularity", index: "ix_work_pop",
			f:    WorksFilter{Sort: WorksSortPopularity},
			cur:  WorksCursor{Sort: WorksSortPopularity, Value: sql.NullString{String: "7.25", Valid: true}, ID: 5},
			want: "SEARCH w USING INDEX ix_work_pop (popularity<?)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			joined := browseWorksPlan(t, s, OwnerScope(1), tc.f, tc.cur)
			if !strings.Contains(joined, tc.want) {
				t.Errorf("plan is not the normative one.\n  got:  %s\n  want: %s", joined, tc.want)
			}
			if faults := browseWorksPlanFaults(tc.index, joined); len(faults) > 0 {
				t.Errorf("plan is wrong:\n  plan: %s\n  %s", joined, strings.Join(faults, "\n  "))
			}
			t.Logf("%s: %s", tc.name, joined)
		})
	}
}

// ⚠️ MIGRATION 0009's INDEX HAS A READER, AND THIS IS IT. The Audiobooks filter's
// selective half is `format = ? AND work_id = ?`, which is a two-column COVERING
// seek on ix_edition_format. Without the index it is a one-column seek on
// ix_edition_work plus a row fetch per edition, for every candidate book, on
// every page. TestBrowseWorksPlanGuardFires drops the index and watches this
// assertion catch it.
func TestBrowseAudiobookFilterSeeksOnTheFormatIndex(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	for _, mt := range []string{MediaTypeAudiobooks, MediaTypeEbooks} {
		t.Run(mt, func(t *testing.T) {
			joined := browseWorksPlan(t, s, OwnerScope(1), WorksFilter{MediaType: mt},
				WorksCursor{Value: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4})
			const want = "COVERING INDEX ix_edition_format (format=? AND work_id=?)"
			if !strings.Contains(joined, want) {
				t.Errorf("the format probe does not seek on ix_edition_format.\n"+
					"  got:  %s\n  want: … %s …", joined, want)
			}
			// The "not every edition is an audiobook" half is a work_id seek on
			// ix_edition_work, which is stated rather than hidden: no index can
			// seek on `format IS NOT NULL AND format <> ?`.
			if !strings.Contains(joined, "en USING INDEX ix_edition_work (work_id=?)") {
				t.Errorf("the NOT EXISTS half is not a work_id seek:\n  %s", joined)
			}
			if faults := browseWorksPlanFaults("ix_work_added", joined); len(faults) > 0 {
				t.Errorf("the format filter displaced the ordering index:\n  plan: %s\n  %s",
					joined, strings.Join(faults, "\n  "))
			}
			t.Logf("%s: %s", mt, joined)
		})
	}
}

// The SCOPED plan, which is the one a multi-user install runs. The EXISTS over
// service_item_link must be an inner seek on ix_sil_work and must not displace
// the ordering index — and it must coexist with the library EXISTS, since a
// scoped install is exactly where both are present at once.
func TestBrowseWorksScopedPlanKeepsTheIndex(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	joined := browseWorksPlan(t, s, Scope{UserID: 1, InstanceIDs: []int64{1, 2}},
		WorksFilter{LibraryIDs: []int64{2, 3}},
		WorksCursor{Value: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4})

	if faults := browseWorksPlanFaults("ix_work_added", joined); len(faults) > 0 {
		t.Errorf("the scoped plan is wrong:\n  plan: %s\n  %s", joined, strings.Join(faults, "\n  "))
	}
	// Derived, not literal — see scopeLinkAlias and recent_test.go's note.
	if !strings.Contains(joined, "SEARCH "+scopeLinkAlias("w.id")) ||
		!strings.Contains(joined, "ix_sil_work") {
		t.Errorf("the scope EXISTS is not a seek on ix_sil_work: %s", joined)
	}
	if strings.Contains(joined, "SCAN sil") {
		t.Errorf("the scope EXISTS degraded to a scan of every service link: %s", joined)
	}
	t.Logf("scoped + two libraries: %s", joined)
}

// ⚠️ THE PLAN §6.5's MEMBER-DRIVEN DEFAULT PRODUCES, MEASURED, which is the
// evidence ADR-0051 supersedes it on. §6.5's denormalised
// (library_id, sort_title, work_id, edition_id) key serves the sort_title order
// and ONLY that one — TestLibraryScopedKeysetIsASeek in internal/db pins that it
// still does — and an added_at page over the same join sorts.
//
// t.Errorf and not t.Logf: if this ever stops needing a sort, ADR-0051's
// argument has changed and the ADR must be amended rather than left standing on
// a measurement that no longer holds.
func TestMemberDrivenAddedOrderStillNeedsASort(t *testing.T) {
	s := newTestStore(t)
	seedBrowseCorpus(t, s)

	for _, tc := range []struct {
		name string
		q    string
		args []any
	}{
		{"one library", `SELECT w.id, w.title FROM library_member lm JOIN work w ON w.id = lm.work_id
			  WHERE lm.library_id = ? AND w.deleted_at IS NULL
			  ORDER BY w.added_at DESC, w.id DESC LIMIT 100`, []any{2}},
		{"two libraries over one kind", `SELECT w.id, w.title FROM library_member lm JOIN work w ON w.id = lm.work_id
			  WHERE lm.library_id IN (?, ?) AND w.deleted_at IS NULL
			  ORDER BY w.added_at DESC, w.id DESC LIMIT 100`, []any{2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := db.QueryPlan(t.Context(), s.DB().Read(), tc.q, tc.args...)
			if err != nil {
				t.Fatalf("QueryPlan: %v", err)
			}
			joined := strings.Join(plan, " | ")
			if !strings.Contains(joined, "TEMP B-TREE") {
				t.Errorf("the member-driven added_at page no longer sorts: %s\n"+
					"That is an improvement, and it makes ADR-0051's measurement wrong. "+
					"Amend the ADR in the same change.", joined)
			}
			t.Logf("REJECTED shape, %s: %s", tc.name, joined)
		})
	}
}

// FIRING THE GUARD. Four arms, each breaking a different thing the guard
// protects and each running the SAME browseWorksPlanFaults the tests above run.
func TestBrowseWorksPlanGuardFires(t *testing.T) {
	dated := WorksCursor{Value: sql.NullString{String: "2026-08-07 10:00:00", Valid: true}, ID: 4}

	// Arm 1 — the ordering index goes away. The regression the assertion exists
	// for: someone drops or renames ix_work_added and the grid silently starts
	// sorting the whole catalogue on every page.
	t.Run("ordering index dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedBrowseCorpus(t, s)
		// ⚠️ dropIndexesAndConfirm, never a bare DROP: browseWorksPlan below
		// EXPLAINs on the READ pool, and MEASURED on this tree, a read-pool
		// connection that has already planned this statement keeps answering with
		// the PRE-DROP plan indefinitely — repeating the EXPLAIN does not shake it
		// loose, though sqlite_master on that same connection is correct
		// throughout. The confirming read inside is therefore both the proof and
		// the cure. Undocumented behaviour, characterised empirically — the helper
		// in store_test.go carries the measurement and its caveats.
		dropIndexesAndConfirm(t, s, "ix_work_added")

		joined := browseWorksPlan(t, s, OwnerScope(1), WorksFilter{LibraryIDs: []int64{2, 3}}, dated)
		faults := browseWorksPlanFaults("ix_work_added", joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan with ix_work_added dropped:\n  %s", joined)
		}
		t.Logf("guard fired with ix_work_added dropped:\n  plan:   %s\n  faults: %v", joined, faults)
	})

	// Arm 2 — the membership index goes away, which is what turns the EXISTS
	// probe into a scan of every membership row per candidate work.
	t.Run("membership index dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedBrowseCorpus(t, s)
		// ⚠️ dropIndexesAndConfirm, never a bare DROP — MEASURED, the read pool
		// this arm EXPLAINs on goes on serving the stale PRE-DROP plan once it has
		// planned the statement, so the confirming read inside is the arm's
		// correctness and not tidiness. Undocumented behaviour, characterised
		// empirically; store_test.go carries the measurement.
		dropIndexesAndConfirm(t, s, "ix_libmem_work")

		joined := browseWorksPlan(t, s, OwnerScope(1), WorksFilter{LibraryIDs: []int64{2, 3}}, dated)
		// ⚠️ browseWorksPlanFaults does NOT fire here, and that is measured
		// rather than papered over: with ix_libmem_work gone SQLite falls back
		// to ux_libmem_identity, which is a different covering seek and still
		// no scan and no sort. What the loss actually costs is the probe's
		// shape, so the arm fires ADR-0051's own guard — the one the passing
		// topology test runs — and not the ordering guard.
		faults := browseMembershipProbeFaults(joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan with ix_libmem_work dropped:\n  %s", joined)
		}
		t.Logf("guard fired with ix_libmem_work dropped:\n  plan:   %s\n  faults: %v", joined, faults)
	})

	// Arm 3 — the unary plus on `+w.kind` removed. It is the single character
	// standing between the added_at order and a full sort of every non-deleted
	// work of the six kinds, and it is exactly the character a later reader
	// "cleans up". ⚠️ It changes the plan only when sqlite_stat1 is ABSENT,
	// which is the planner this suite sees; see the section header.
	t.Run("unary plus removed", func(t *testing.T) {
		s := newTestStore(t)
		seedBrowseCorpus(t, s)

		query, args, err := browseWorksSQL(OwnerScope(1), WorksFilter{}, dated, BrowseWorksDefaultLimit)
		if err != nil {
			t.Fatal(err)
		}
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
		faults := browseWorksPlanFaults("ix_work_added", joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed the plan without the unary plus:\n  %s", joined)
		}
		t.Logf("guard fired without the unary plus:\n  plan:   %s\n  faults: %v", joined, faults)
	})

	// Arm 4 — migration 0009's index goes away, fired against the assertion
	// that names it. Without this arm, TestBrowseAudiobookFilterSeeksOnTheFormat
	// Index would be a sentence about an index nobody had watched fail.
	t.Run("ix_edition_format dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedBrowseCorpus(t, s)
		// ⚠️ dropIndexesAndConfirm, never a bare DROP — MEASURED, the read pool
		// this arm EXPLAINs on goes on serving the stale PRE-DROP plan once it has
		// planned the statement, so the confirming read inside is the arm's
		// correctness and not tidiness. Undocumented behaviour, characterised
		// empirically; store_test.go carries the measurement.
		dropIndexesAndConfirm(t, s, "ix_edition_format")

		joined := browseWorksPlan(t, s, OwnerScope(1), WorksFilter{MediaType: MediaTypeAudiobooks}, dated)
		if strings.Contains(joined, "ix_edition_format") {
			t.Fatalf("the plan still names a dropped index:\n  %s", joined)
		}
		if !strings.Contains(joined, "ea EXISTS USING INDEX ix_edition_work") {
			t.Errorf("without ix_edition_format the probe fell back to something other than "+
				"the work_id seek the migration header names:\n  %s", joined)
		}
		t.Logf("guard fired with ix_edition_format dropped:\n  plan: %s", joined)
	})
}

// browseKinds and mediaTypeOf are inverses, asserted directly as well as by
// execution: every kind browseKinds can return must map to the media type that
// asked for it, and the six media types must between them name every kind
// Block C admits.
func TestBrowseKindsAndMediaTypeOfAreInverses(t *testing.T) {
	covered := map[string]bool{}
	for _, mt := range []string{
		MediaTypeMovies, MediaTypeTV, MediaTypeMusic,
		MediaTypeEbooks, MediaTypeAudiobooks, MediaTypeComics,
	} {
		kinds, ok := browseKinds(mt)
		if !ok {
			t.Fatalf("browseKinds does not know %q", mt)
		}
		for _, k := range kinds {
			covered[k] = true
			// mediaTypeOf's answer depends on the editions for `book`, which is
			// exactly why the two book media types share one kind; every other
			// kind must map straight back.
			if k != "book" && mediaTypeOf(k, sql.NullInt64{}) != mt {
				t.Errorf("browseKinds(%q) admits %q, which mediaTypeOf calls %q",
					mt, k, mediaTypeOf(k, sql.NullInt64{}))
			}
		}
	}
	for _, k := range recentWorkKinds {
		if !covered[k] {
			t.Errorf("no media type admits kind %q, so it is browsable from nowhere", k)
		}
	}
	if _, ok := browseKinds(""); !ok {
		t.Error(`browseKinds("") must be the unfiltered corpus, not an unknown media type`)
	}
}
