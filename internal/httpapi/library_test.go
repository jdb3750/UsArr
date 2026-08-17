package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// seedLibraryCorpus writes the same shape internal/store's recent_test.go uses,
// through raw SQL, because the endpoint's job is to render whatever the
// catalogue writer put there and the writer is a different commit's subject.
//
// A REAL MIGRATED DATABASE WITH ROWS IN IT. Every assertion below is about
// ordering, exclusion, field selection or paging, and not one of those is
// observable at zero rows.
func seedLibraryCorpus(t *testing.T, s *Server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'kavita', 'library', 'Kavita', 'http://kavita.example', X'00')`,
		`INSERT INTO library (id, user_id, name, slug, kind) VALUES (1, 0, 'Manga', 'manga', 'comic')`,
		`INSERT INTO library (id, user_id, name, slug, kind) VALUES (2, 0, 'Books', 'books', 'book')`,
	}
	rows := []struct {
		id      int
		kind    string
		title   string
		added   string
		deleted string
		lib     int
	}{
		{1, "comic", "Berserk", "2026-08-10 10:00:00", "", 1},
		{2, "comic", "Vinland Saga", "2026-08-09 10:00:00", "", 1},
		{3, "book", "Piranesi", "2026-08-08 10:00:00", "", 2},
		{4, "book", "Project Hail Mary", "2026-08-07 10:00:00", "", 2},
		// Interleaved, so the exclusions have to hold in the MIDDLE of a page.
		{5, "comic", "A Soft-Deleted Comic", "2026-08-06 10:00:00", "2026-08-16 09:00:00", 1},
		{6, "person", "Kentaro Miura", "2026-08-05 10:00:00", "", 1},
		{7, "comic_issue", "Berserk #1", "2026-08-04 10:00:00", "", 1},
		{8, "movie", "Train Dreams", "2026-08-03 10:00:00", "", 1},
		// The undated tail — Kavita reporting no `created`.
		{9, "comic", "An Undated Manga", "", "", 1},
	}
	nullable := func(v string) string {
		if v == "" {
			return "NULL"
		}
		return "'" + v + "'"
	}
	for _, r := range rows {
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO work
			  (id, kind, title, sort_title, normalized_title, year, added_at, deleted_at,
			   have_count, want_count, availability)
			VALUES (%d, '%s', '%s', '%s', '%s', 2020, %s, %s, 43, 17,
			        '{"k":"count","have":43,"total":null,"total_source":null,"missing":["7","12"]}')`,
			r.id, r.kind, r.title, strings.ToLower(r.title), strings.ToLower(r.title),
			nullable(r.added), nullable(r.deleted)))
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			 VALUES (1, %d, 'r%d', 'series', '2026-08-16 12:00:00')`, r.id, r.id))
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (%d, 'w%02d', %d)`,
			r.lib, r.id, r.id))
	}
	// Editions: Piranesi is an ebook, Project Hail Mary an audiobook. This is
	// the split §17.2 says a client cannot make for itself.
	stmts = append(stmts,
		`INSERT INTO edition (id, work_id, format) VALUES (1, 3, 'ebook')`,
		`INSERT INTO edition (id, work_id, format) VALUES (2, 4, 'audiobook')`)

	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

// callRecentWorks runs the handler with a session already resolved, which is
// what the authenticated middleware does before it.
func callRecentWorks(t *testing.T, s *Server, query string) (int, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/library/recent"+query, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: true},
	}))
	w := httptest.NewRecorder()
	if err := s.handleRecentWorks(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

func TestRecentWorksEndpointRendersOneUnifiedTable(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	code, body := callRecentWorks(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/library/recent = %d: %s", code, body)
	}
	var got recentWorksResponse
	mustJSON(t, body, &got)

	if got.Limit != store.RecentWorksDefaultLimit {
		t.Errorf("limit = %d, want the default %d", got.Limit, store.RecentWorksDefaultLimit)
	}
	want := []string{
		"Berserk", "Vinland Saga", "Piranesi", "Project Hail Mary",
		"Train Dreams", "An Undated Manga",
	}
	var titles []string
	for _, item := range got.Items {
		titles = append(titles, item.Title)
	}
	if strings.Join(titles, " | ") != strings.Join(want, " | ") {
		t.Fatalf("Block C is not one unified table in added_at order:\n  got:  %v\n  want: %v",
			titles, want)
	}
	if got.NextCursor != "" {
		t.Errorf("a page carrying the whole corpus still offered a next cursor: %q", got.NextCursor)
	}

	// FOUR media types in ONE table, which is the shape ADR-0028 replaced six
	// strips with.
	seen := map[string]bool{}
	for _, item := range got.Items {
		seen[item.MediaType] = true
	}
	for _, mt := range []string{
		store.MediaTypeComics, store.MediaTypeEbooks,
		store.MediaTypeAudiobooks, store.MediaTypeMovies,
	} {
		if !seen[mt] {
			t.Errorf("media type %q is missing from the unified table: %s", mt, body)
		}
	}
}

// The exclusions, asserted at the HTTP boundary rather than only at the store,
// because the boundary is where the owner's browser reads them.
func TestRecentWorksEndpointExcludesPeopleTombstonesAndChildKinds(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	_, body := callRecentWorks(t, s, "")
	for _, forbidden := range []string{
		"Kentaro Miura", "person", "A Soft-Deleted Comic",
		"Berserk #1", "comic_issue",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%q reached the browser: %s", forbidden, body)
		}
	}
}

// THE ALLOWLIST, pinned as a key set. The response struct is the allowlist and
// this is what makes growing it deliberate: a column added to store.RecentWork
// and copied into the response by a later change fails here first.
func TestRecentWorksResponseKeysAreTheAllowlist(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	_, body := callRecentWorks(t, s, "")
	var envelope map[string]json.RawMessage
	mustJSON(t, body, &envelope)

	wantEnvelope := []string{"items", "limit"} // next_cursor is omitempty and absent here
	gotEnvelope := keysOf(envelope)
	if strings.Join(gotEnvelope, ",") != strings.Join(wantEnvelope, ",") {
		t.Errorf("envelope keys = %v, want %v", gotEnvelope, wantEnvelope)
	}

	var items []map[string]json.RawMessage
	mustJSON(t, string(envelope["items"]), &items)
	if len(items) == 0 {
		t.Fatal("no items to inspect")
	}
	// Berserk is a comic with a year, an added_at and an availability blob, so
	// every optional key is present on it. Every OTHER row is a subset of this.
	want := []string{
		"added_at", "availability", "have_count", "id", "kind",
		"media_type", "title", "want_count", "year",
	}
	if got := keysOf(items[0]); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("row keys = %v\n         want %v", got, want)
	}
	for i, item := range items {
		for key := range item {
			if !contains(want, key) {
				t.Errorf("row %d carries %q, which is not on the allowlist", i, key)
			}
		}
	}
}

// Nothing service-side and nothing credential-shaped leaves, asserted against
// the RESPONSE BYTES rather than against the struct, so a field added by any
// route — including one added by embedding — is caught.
func TestRecentWorksShipsNothingServiceSide(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	// Give the work a remote path and a hostname on its link, the two
	// server-side strings closest to this read.
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE service_item_link SET remote_path = '/mnt/tank/manga/Berserk' WHERE work_id = 1`)
		return err
	}); err != nil {
		t.Fatalf("seed remote path: %v", err)
	}

	_, body := callRecentWorks(t, s, "")
	for _, forbidden := range []string{
		"/mnt/tank", "remote_path", "api_key", "apikey", "kavita.example",
		"service_instance", "Kavita", "http://", "content_hash",
	} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
			t.Errorf("the response carries %q: %s", forbidden, body)
		}
	}
}

// The Ebooks/Audiobooks split is server-side only in v0.1 (§17.2), so it has to
// be ON THE WIRE. If media_type were derived in the browser from kind, two of
// the six chips would be one chip.
func TestRecentWorksSplitsEbooksFromAudiobooksOnTheWire(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	_, body := callRecentWorks(t, s, "")
	var got recentWorksResponse
	mustJSON(t, body, &got)

	byTitle := map[string]recentWorkResponse{}
	for _, item := range got.Items {
		byTitle[item.Title] = item
	}
	if byTitle["Piranesi"].MediaType != store.MediaTypeEbooks {
		t.Errorf("Piranesi media_type = %q, want %q", byTitle["Piranesi"].MediaType, store.MediaTypeEbooks)
	}
	if byTitle["Project Hail Mary"].MediaType != store.MediaTypeAudiobooks {
		t.Errorf("Project Hail Mary media_type = %q, want %q",
			byTitle["Project Hail Mary"].MediaType, store.MediaTypeAudiobooks)
	}
	// Both are kind=book, so the two rows are separated by nothing else.
	if byTitle["Piranesi"].Kind != "book" || byTitle["Project Hail Mary"].Kind != "book" {
		t.Fatalf("the fixture no longer proves the split: both rows must be kind=book")
	}
}

func TestRecentWorksPagesThroughTheCursor(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	var walked []string
	query := "?limit=2"
	for i := 0; ; i++ {
		if i > 50 {
			t.Fatalf("the cursor did not terminate")
		}
		code, body := callRecentWorks(t, s, query)
		if code != http.StatusOK {
			t.Fatalf("page %d = %d: %s", i, code, body)
		}
		var page recentWorksResponse
		mustJSON(t, body, &page)
		if len(page.Items) > 2 {
			t.Fatalf("page %d carried %d items against limit=2", i, len(page.Items))
		}
		for _, item := range page.Items {
			walked = append(walked, item.Title)
		}
		if page.NextCursor == "" {
			break
		}
		query = "?limit=2&cursor=" + page.NextCursor
	}

	want := []string{
		"Berserk", "Vinland Saga", "Piranesi", "Project Hail Mary",
		"Train Dreams", "An Undated Manga",
	}
	if strings.Join(walked, " | ") != strings.Join(want, " | ") {
		t.Fatalf("the walk did not cover the corpus exactly once:\n  got:  %v\n  want: %v", walked, want)
	}
	// The undated row is reachable and it is LAST. A plain row-value keyset
	// predicate would make it unreachable on every page but the first, silently.
	if walked[len(walked)-1] != "An Undated Manga" {
		t.Errorf("the undated row is not the tail: %v", walked)
	}
}

// A cursor that will not parse is a 400 with an action, never a silent reset to
// page one — a reset turns a stale bookmark into a Load-more loop that re-serves
// the first page for ever and reads as a stuck list.
func TestRecentWorksRejectsAMalformedCursor(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	code, body := callRecentWorks(t, s, "?cursor=not-a-cursor")
	if code != http.StatusBadRequest {
		t.Fatalf("a malformed cursor gave %d, want 400: %s", code, body)
	}
	var errBody errorBody
	mustJSON(t, body, &errBody)
	if errBody.Error != CodeBadRequest {
		t.Errorf("error code = %q, want %q", errBody.Error, CodeBadRequest)
	}
	if errBody.Action == "" {
		t.Errorf("the 400 names no action: %s", body)
	}
}

func TestRecentWorksClampsTheLimit(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	_, body := callRecentWorks(t, s, "?limit=100000")
	var got recentWorksResponse
	mustJSON(t, body, &got)
	if got.Limit != store.RecentWorksMaxLimit {
		t.Errorf("limit = %d, want the cap %d echoed back", got.Limit, store.RecentWorksMaxLimit)
	}
}

// A session is required. The route is mounted behind s.authenticated, and this
// asserts the handler itself refuses rather than relying on the router — a
// handler that trusts its middleware is one route registration away from being
// public.
func TestRecentWorksNeedsASession(t *testing.T) {
	s := newTestServer(t, nil)
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/library/recent", nil)
	w := httptest.NewRecorder()
	err := s.handleRecentWorks(w, r)
	if err == nil {
		t.Fatal("a request with no session was served")
	}
	if !strings.Contains(err.Error(), string(CodeUnauthorized)) {
		t.Errorf("error = %v, want an unauthorized", err)
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
