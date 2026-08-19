package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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

	// ONE POSTER, ON BERSERK. Written here rather than by the import because no
	// import CALLS the fetch half — internal/imagepipeline exists and nothing
	// invokes it — so without this every row's poster_key is legitimately absent
	// and every assertion about it would pass VACUOUSLY, which is the
	// empty-green the seed above already has a paragraph about.
	//
	// ⚠️ THIS INSERT IS TEST-ONLY. internal/store's format lint exempts
	// `_test.go`, so this does not satisfy it; the ValidImageFormat call and the
	// credential-stripped `source_url` assertion (security.md §5) attach to the
	// first PRODUCTION writer, which is not this.
	//
	// Berserk deliberately, because it is the row every key-set assertion here
	// inspects: it is newest, so it is items[0] on Block C and on the grid's
	// default order, and it already carries every other optional key.
	stmts = append(stmts,
		`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format, state)
		   VALUES (1, 'http://kavita.example/cover/1', 'configured', 'poster',
		           'aaaaaaaaaaaaaaaa', 'jpeg', 'ready')`,
		`UPDATE work SET poster_asset_id = 1 WHERE id = 1`)

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
		"media_type", "poster_key", "title", "want_count", "year",
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

// `limit` IS A CLAMP, NOT A VALIDATED RANGE, and the whole table is pinned by
// execution rather than by the sentence above recentWorksLimit — a rule with an
// undocumented exception is what sent a client into modelling `limit` as the
// number it sent. The 2^31 row is the exception this closes: it 400ed while
// 2147483647 clamped, so "too big is served short" was false at exactly one
// boundary and true either side of it.
func TestRecentWorksLimitIsAClampNotAValidatedRange(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	for _, tc := range []struct {
		query string
		code  int
		limit int // the echoed limit, when code is 200
	}{
		{"", http.StatusOK, store.RecentWorksDefaultLimit},
		{"?limit=", http.StatusOK, store.RecentWorksDefaultLimit},
		{"?limit=0", http.StatusOK, store.RecentWorksDefaultLimit},
		{"?limit=1", http.StatusOK, 1},
		{"?limit=50", http.StatusOK, 50},
		{"?limit=200", http.StatusOK, store.RecentWorksMaxLimit},
		{"?limit=201", http.StatusOK, store.RecentWorksMaxLimit},
		{"?limit=100000", http.StatusOK, store.RecentWorksMaxLimit},
		// int32's ceiling, and one past it. Both are page sizes that came out
		// too big, so both are clamped; neither is a different kind of request.
		{"?limit=2147483647", http.StatusOK, store.RecentWorksMaxLimit},
		{"?limit=2147483648", http.StatusOK, store.RecentWorksMaxLimit},
		{"?limit=9223372036854775808", http.StatusOK, store.RecentWorksMaxLimit},
		// Not page sizes at all.
		{"?limit=-1", http.StatusBadRequest, 0},
		{"?limit=-9223372036854775809", http.StatusBadRequest, 0},
		{"?limit=abc", http.StatusBadRequest, 0},
		{"?limit=1.5", http.StatusBadRequest, 0},
		{"?limit=0x10", http.StatusBadRequest, 0},
		// A bare "+" is NOT here on purpose: a query string decodes it to a
		// space, so it arrives as the empty parameter and takes the default.
		// That is URL semantics, not this parser's, and pinning it as a 400
		// would pin the wrong layer.
	} {
		code, body := callRecentWorks(t, s, tc.query)
		if code != tc.code {
			t.Errorf("GET /api/v1/library/recent%s = %d, want %d: %s", tc.query, code, tc.code, body)
			continue
		}
		if code != http.StatusOK {
			var errBody errorBody
			mustJSON(t, body, &errBody)
			if errBody.Error != CodeBadRequest {
				t.Errorf("%s: error code = %q, want %q", tc.query, errBody.Error, CodeBadRequest)
			}
			if errBody.Action == "" {
				t.Errorf("%s: the 400 names no action: %s", tc.query, body)
			}
			continue
		}
		var got recentWorksResponse
		mustJSON(t, body, &got)
		if got.Limit != tc.limit {
			t.Errorf("GET /api/v1/library/recent%s echoed limit %d, want %d",
				tc.query, got.Limit, tc.limit)
		}
		// The echo is AUTHORITATIVE, so it must actually bound the page. A
		// number a client pages against and the server does not honour is worse
		// than no number.
		if len(got.Items) > got.Limit {
			t.Errorf("%s: %d items against an echoed limit of %d",
				tc.query, len(got.Items), got.Limit)
		}
	}
}

// seedAvailability puts the four blob states on four real rows of a real
// migrated database: NULL, valid, malformed text, and an object whose "k" is a
// value nobody defined. Ids match seedLibraryCorpus's.
func seedAvailability(t *testing.T, s *Server) {
	t.Helper()
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range []string{
			// 1 Berserk: valid, k=tier.
			`UPDATE work SET availability = '{"k":"tier","1080p":{"have":1,"total":2}}' WHERE id = 1`,
			// 2 Vinland Saga: NULL — a work that honestly has no blob.
			`UPDATE work SET availability = NULL WHERE id = 2`,
			// 3 Piranesi: truncated garbage. This is the row the frontend could
			// not tell apart from row 2.
			`UPDATE work SET availability = '{"k":"count","have":' WHERE id = 3`,
			// 4 Project Hail Mary: valid JSON, object, no discriminator at all.
			`UPDATE work SET availability = '{"have":1,"total":2}' WHERE id = 4`,
			// 8 Train Dreams: a fourth "k". §1 names three.
			`UPDATE work SET availability = '{"k":"sardine","have":1}' WHERE id = 8`,
			// 9 An Undated Manga: valid JSON that is not an object.
			`UPDATE work SET availability = '[1,2,3]' WHERE id = 9`,
		} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed availability: %v", err)
	}
}

// `availability` IS OPTIONAL ON THE WIRE, AND ITS TWO REASONS FOR BEING ABSENT
// ARE NOT THE SAME REASON. Absent-because-NULL is the truth about a work and is
// silent. Absent-because-corrupt is a bug in whatever wrote the row, and used to
// be byte-identical to the first — which is the failure shape this endpoint's
// own comments spend a page refusing everywhere else (an unparseable added_at is
// dropped, and grabs.go logs an actorless audit row rather than swallowing it).
func TestRecentWorksAvailabilityAbsenceIsHonestAndCorruptionIsLogged(t *testing.T) {
	var logs bytes.Buffer
	s := newTestServer(t, func(c *Config) {
		c.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	seedLibraryCorpus(t, s)
	seedAvailability(t, s)

	_, body := callRecentWorks(t, s, "")
	var got recentWorksResponse
	mustJSON(t, body, &got)

	byID := map[int64]recentWorkResponse{}
	for _, item := range got.Items {
		byID[item.ID] = item
	}
	if len(byID) != 6 {
		t.Fatalf("the fixture no longer covers every case: %d rows", len(byID))
	}

	// The valid blob crosses VERBATIM. It is polymorphic by medium and the
	// renderer switches on "k", so nothing here flattens it.
	if want := `{"k":"tier","1080p":{"have":1,"total":2}}`; string(byID[1].Availability) != want {
		t.Errorf("work 1 availability = %s, want %s", byID[1].Availability, want)
	}
	// Every corrupt case is absent from the wire — a bad blob must not fail the
	// whole block, and the consumer is not asked to parse garbage.
	for _, id := range []int64{2, 3, 4, 8, 9} {
		if byID[id].Availability != nil {
			t.Errorf("work %d put %s on the wire", id, byID[id].Availability)
		}
	}
	if strings.Contains(body, "sardine") {
		t.Errorf("an unrecognised discriminator reached the browser: %s", body)
	}

	// …and every corrupt case is OBSERVABLE, by work id, which is what makes the
	// row findable: SELECT availability FROM work WHERE id = ?.
	out := logs.String()
	for _, want := range []string{
		`work.availability will not decode and was dropped from the response" work_id=3`,
		`work.availability will not decode and was dropped from the response" work_id=9`,
		`work.availability has no \"k\" discriminator and was dropped from the response" work_id=4`,
		`work.availability has an unrecognised \"k\" discriminator and was dropped from the response" work_id=8 k=sardine`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the log does not carry %q:\n%s", want, out)
		}
	}
	// THE NULL ROW IS NOT A FAULT AND MUST NOT BE LOGGED. A warning per
	// honestly-empty work would fire on most of a fresh catalogue and the log
	// would stop being worth reading — which is the same as no signal.
	if strings.Contains(out, "work_id=2") {
		t.Errorf("a NULL availability was logged as a fault:\n%s", out)
	}
	// The blob's own text stays out of the log: it came from the database and
	// has no length bound.
	if strings.Contains(out, `"have":`) {
		t.Errorf("the log echoed the stored blob:\n%s", out)
	}
}

// The "k" vocabulary is closed, and this pins the Go-side copy against
// schema.md — the document that defines it — so a fourth shape cannot be added
// in one place only. Nothing in internal/ writes an availability blob yet, so
// today these are the only two statements of the vocabulary that exist.
func TestAvailabilityKindsMatchSchemaMd(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "schema.md"))
	if err != nil {
		t.Fatalf("read schema.md: %v", err)
	}
	found := map[string]bool{}
	for _, m := range regexp.MustCompile(`"k"\s*:\s*"([a-z_]+)"`).FindAllStringSubmatch(string(raw), -1) {
		found[m[1]] = true
	}
	if len(found) == 0 {
		t.Fatal(`schema.md declares no "k" discriminators; this guard has stopped measuring anything`)
	}
	for k := range found {
		if !availabilityKinds[k] {
			t.Errorf("schema.md defines availability shape %q and availabilityKinds does not carry it", k)
		}
	}
	for k := range availabilityKinds {
		if !found[k] {
			t.Errorf("availabilityKinds carries %q and schema.md defines no such shape", k)
		}
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
