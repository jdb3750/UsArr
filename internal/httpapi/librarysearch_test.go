package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// seedSearchCorpus writes a corpus THROUGH THE REAL CATALOGUE WRITER rather
// than through raw INSERTs, unlike seedLibraryCorpus above it.
//
// The difference is not stylistic. search_doc.rowid is the allocator for
// search_fts and search_trgm and the fusion joins on it (LS-01/G3), so a seed
// that hand-wrote its own documents would be exercising rowids it allocated
// itself and could never see the class of bug that record is about. Block C's
// seed can use raw SQL because it renders `work` rows; this one cannot.
func seedSearchCorpus(t *testing.T, s *Server) {
	t.Helper()
	ctx := t.Context()

	inst, err := s.store.CreateServiceInstance(ctx, store.ServiceInstance{
		Kind: "kavita", Role: "library", Name: "Kavita",
		BaseURL: "http://kavita.example", APIKeyEnc: []byte{1}, KEKID: 1,
		Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	binds, _, err := s.store.BindContainers(ctx, inst, store.SystemUserID,
		[]store.CatalogueContainer{
			{RemoteID: "1", Name: "Manga", Kind: "comic"},
			{RemoteID: "2", Name: "Novels", Kind: "book"},
		})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	mk := func(remote, container, kind, title string) store.CatalogueItem {
		return store.CatalogueItem{
			RemoteID: remote, RemoteKind: "series", ContainerID: container, Kind: kind,
			Title: title, SortTitle: title,
			NormalizedTitle: store.NormalizeForSearch(title), NormVersion: 1,
			AddedAt: now, RemoteUpdatedAt: now, HasFile: true,
		}
	}
	items := []store.CatalogueItem{
		mk("1", "1", "comic", "Berserk"),
		mk("2", "1", "comic", "Vinland Saga"),
		mk("3", "2", "book", "Piranesi"),
		mk("4", "2", "book", "Fallout: New Vegas"),
	}
	if _, err := s.store.ApplyCatalogueBatch(ctx, inst, binds, items, now); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	// `year` and `availability`, written here rather than by the import because
	// NOTHING writes either yet: CatalogueItem carries no field for either one,
	// and internal/httpapi's own availabilityKinds comment says that vocabulary
	// has no producer anywhere in the tree. Left unwritten, both columns are
	// NULL for every row, both keys are legitimately absent from every response,
	// and the field-selection assertions below would pass VACUOUSLY — the
	// empty-green this repo has a rule about. The blob is the shape a writer
	// will produce (schema.md §1, "count" discriminator), applied after the
	// fact so the response has something to omit or carry.
	if err := s.store.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE work SET year = 2020,
			                 availability = '{"k":"count","have":43,"total":null}'`)
		return err
	}); err != nil {
		t.Fatalf("seed year and availability: %v", err)
	}
}

// callLibrarySearch runs the handler with a session already resolved, which is
// what the authenticated middleware does before it.
func callLibrarySearch(t *testing.T, s *Server, query string) (int, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/search"+query, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: store.SystemUserID, Username: "joe", IsOwner: true},
	}))
	w := httptest.NewRecorder()
	if err := s.handleLibrarySearch(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

func TestLibrarySearchAnswersFromTheLocalCorpus(t *testing.T) {
	s := newTestServer(t, nil)
	seedSearchCorpus(t, s)

	code, body := callLibrarySearch(t, s, "?q=berserk")
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/search = %d: %s", code, body)
	}
	var got searchResponse
	mustJSON(t, body, &got)

	if got.Query != "berserk" {
		t.Errorf("query = %q, want the text the server searched echoed back", got.Query)
	}
	if got.Limit != store.SearchDefaultLimit {
		t.Errorf("limit = %d, want the default %d", got.Limit, store.SearchDefaultLimit)
	}
	if got.Truncated {
		t.Error("a one-token query reported truncated")
	}
	if len(got.Items) != 1 || got.Items[0].Title != "Berserk" {
		t.Fatalf("items = %+v, want exactly Berserk", got.Items)
	}
	hit := got.Items[0]
	if hit.Kind != "comic" || hit.MediaType != store.MediaTypeComics {
		t.Errorf("kind=%q media_type=%q, want comic/%s", hit.Kind, hit.MediaType, store.MediaTypeComics)
	}
	if hit.ID == 0 {
		t.Error("the hit carries no work id, so nothing can be linked to")
	}
	if len(hit.Availability) == 0 {
		t.Error("availability was dropped; the Have column has nothing to render through")
	}
}

// TestLibrarySearchIsNotTheReleaseSearch pins the two endpoints apart at the
// routing layer, which is the property the endpoint move exists for.
//
// They answer different questions over different corpora, with budgets three
// orders of magnitude apart (ARCHITECTURE.md §2073 against §8.4). This asserts
// the observable difference: the library search answers 200 with results IN THE
// BODY, and the release search answers 202 with results on the SSE stream.
func TestLibrarySearchIsNotTheReleaseSearch(t *testing.T) {
	s := newTestServer(t, nil)
	seedSearchCorpus(t, s)

	code, body := callLibrarySearch(t, s, "?q=berserk")
	if code != http.StatusOK {
		t.Fatalf("the library search answered %d; it renders from SQLite and "+
			"must answer 200 with a body, not 202 with a promise: %s", code, body)
	}
	if strings.Contains(body, "search_id") || strings.Contains(body, "events_url") {
		t.Errorf("the library search answered with the release search's envelope: %s", body)
	}

	// And the route table has both, on different paths.
	mux := s.Handler()
	for _, path := range []string{"/api/v1/search?q=x", "/api/v1/releases/search?query=x"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code == http.StatusNotFound {
			t.Errorf("GET %s is not routed at all", path)
		}
	}
}

func TestLibrarySearchRefusesAnEmptyQuery(t *testing.T) {
	s := newTestServer(t, nil)
	seedSearchCorpus(t, s)

	for _, q := range []string{"", "?q=", "?q=%20%20"} {
		code, body := callLibrarySearch(t, s, q)
		if code != http.StatusBadRequest {
			t.Errorf("GET /api/v1/search%s = %d, want 400: %s", q, code, body)
		}
	}
}

// TestLibrarySearchSurvivesEveryMetacharacter is the endpoint-level form of the
// §3 escape guarantee, and the failure it catches is a 500.
//
// FTS5's MATCH argument is a query language: an unescaped `:` is a column
// filter, `*` a prefix operator, `AND`/`OR`/`NEAR` keywords, a bare `"` an
// unterminated string. Every one of those raises "fts5: syntax error near …"
// and fails the whole request — so an un-escaped implementation does not return
// zero results for `Fallout: New Vegas`, it returns 500 for it.
func TestLibrarySearchSurvivesEveryMetacharacter(t *testing.T) {
	s := newTestServer(t, nil)
	seedSearchCorpus(t, s)

	for _, raw := range []string{
		"Fallout: New Vegas", "AC/DC", `say "hello"`, "Schindler's List",
		"*", ":", "^", "-", "NEAR", "AND", "OR", "NOT", `"`,
		"title:dune", "a AND b", "(parens)", "col:umn:s", "葬送のフリーレン",
	} {
		t.Run(raw, func(t *testing.T) {
			code, body := callLibrarySearch(t, s, "?q="+url.QueryEscape(raw))
			if code != http.StatusOK {
				t.Errorf("q=%q answered %d, want 200. An fts5 syntax error is a "+
					"500 on this query, not a zero-result: %s", raw, code, body)
			}
		})
	}

	t.Run("and the colon case finds the work", func(t *testing.T) {
		_, body := callLibrarySearch(t, s, "?q="+url.QueryEscape("Fallout: New Vegas"))
		var got searchResponse
		mustJSON(t, body, &got)
		if len(got.Items) == 0 || got.Items[0].Title != "Fallout: New Vegas" {
			t.Errorf("items = %+v, want Fallout: New Vegas first", got.Items)
		}
	})
}

// TestLibrarySearchLimitIsAClamp pins §1.2's rule at this endpoint's bounds.
func TestLibrarySearchLimitIsAClamp(t *testing.T) {
	s := newTestServer(t, nil)
	seedSearchCorpus(t, s)

	for _, tc := range []struct {
		query string
		code  int
		limit int
	}{
		{"?q=a&limit=", http.StatusOK, store.SearchDefaultLimit},
		{"?q=a&limit=0", http.StatusOK, store.SearchDefaultLimit},
		{"?q=a&limit=5", http.StatusOK, 5},
		{"?q=a&limit=99999", http.StatusOK, store.SearchMaxLimit},
		// The 2^31 cliff recentWorksLimit's comment records: this must clamp,
		// not 400, because it is a page size that came out too big.
		{"?q=a&limit=2147483648", http.StatusOK, store.SearchMaxLimit},
		{"?q=a&limit=-1", http.StatusBadRequest, 0},
		{"?q=a&limit=abc", http.StatusBadRequest, 0},
	} {
		code, body := callLibrarySearch(t, s, tc.query)
		if code != tc.code {
			t.Errorf("GET %s = %d, want %d: %s", tc.query, code, tc.code, body)
			continue
		}
		if tc.code != http.StatusOK {
			continue
		}
		var got searchResponse
		mustJSON(t, body, &got)
		if got.Limit != tc.limit {
			t.Errorf("GET %s echoed limit=%d, want %d", tc.query, got.Limit, tc.limit)
		}
	}
}

// TestLibrarySearchReportsTruncation. search.md §3 requires a UI note when the
// twelve-token cap fires, and a note the server never sends cannot be rendered.
func TestLibrarySearchReportsTruncation(t *testing.T) {
	s := newTestServer(t, nil)
	seedSearchCorpus(t, s)

	_, body := callLibrarySearch(t, s, "?q="+url.QueryEscape(strings.Repeat("berserk ", 20)))
	var got searchResponse
	mustJSON(t, body, &got)
	if !got.Truncated {
		t.Errorf("a twenty-token query did not report truncated: %s", body)
	}
}

// TestSearchResponseKeysAreTheAllowlist pins the wire shape, so growing it is a
// deliberate edit rather than a struct field that leaked.
//
// It also pins the claim the doc comment makes: the keys are
// recentWorkResponse's keys, so a client reuses one row component across Home
// and Search.
func TestSearchResponseKeysAreTheAllowlist(t *testing.T) {
	s := newTestServer(t, nil)
	seedSearchCorpus(t, s)

	_, body := callLibrarySearch(t, s, "?q=berserk")
	var envelope map[string]json.RawMessage
	mustJSON(t, body, &envelope)

	wantEnvelope := []string{"items", "limit", "query", "truncated"}
	if got := sortedKeys(envelope); !reflect.DeepEqual(got, wantEnvelope) {
		t.Errorf("response keys = %v, want %v", got, wantEnvelope)
	}

	var items []map[string]json.RawMessage
	mustJSON(t, string(envelope["items"]), &items)
	if len(items) == 0 {
		t.Fatal("no items to check the allowlist against")
	}
	wantItem := []string{
		"added_at", "availability", "have_count", "id", "kind",
		"media_type", "title", "want_count", "year",
	}
	if got := sortedKeys(items[0]); !reflect.DeepEqual(got, wantItem) {
		t.Errorf("item keys = %v, want %v", got, wantItem)
	}

	// Nothing service-side reaches the wire. The instance is named in the
	// database and must never be named in a search result.
	for _, banned := range []string{"remote_path", "instance", "service", "api_key", "content_hash"} {
		if strings.Contains(body, banned) {
			t.Errorf("the response carries %q: %s", banned, body)
		}
	}
}

func sortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
