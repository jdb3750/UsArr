package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// callBrowseWorks runs the handler with a session already resolved, which is
// what the authenticated middleware does before it.
func callBrowseWorks(t *testing.T, s *Server, query string) (int, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/library"+query, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: true},
	}))
	w := httptest.NewRecorder()
	if err := s.handleBrowseWorks(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

func browseTitles(t *testing.T, body string) []string {
	t.Helper()
	var got struct {
		Items []struct {
			Title     string `json:"title"`
			MediaType string `json:"media_type"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	out := make([]string, 0, len(got.Items))
	for _, i := range got.Items {
		out = append(out, i.Title)
	}
	return out
}

// With no parameters at all the grid is Block C's corpus in Block C's order.
// The two endpoints must not disagree about what is in the library.
func TestBrowseEndpointUnfilteredMatchesBlockC(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	code, body := callBrowseWorks(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	_, recent := callRecentWorks(t, s, "")
	if strings.Join(browseTitles(t, body), "|") != strings.Join(browseTitles(t, recent), "|") {
		t.Errorf("the unfiltered grid and Block C disagree:\n grid:   %v\n recent: %v",
			browseTitles(t, body), browseTitles(t, recent))
	}
}

// ⚠️ THE PARAMETER IS `media_type` AND IT TAKES THE NAVIGATION ENUM. `kind` is a
// real column that ships on this wire under its own name, so a parameter named
// `kind` that rejected `kind` values would be two vocabularies wearing one name.
// This pins both halves: the six media types are served, and the `kind` spellings
// that collide with them are refused.
func TestBrowseEndpointFiltersByMediaTypeAndNotByKind(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	code, body := callBrowseWorks(t, s, "?media_type=comics")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	want := []string{"Berserk", "Vinland Saga", "An Undated Manga"}
	if got := browseTitles(t, body); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("media_type=comics\n got: %v\nwant: %v", got, want)
	}

	// The Ebooks/Audiobooks split is resolved SERVER-SIDE, which is the whole
	// reason this parameter is not `kind`: both are kind='book' and what
	// separates them is edition.format, which is not on this wire at all.
	_, body = callBrowseWorks(t, s, "?media_type=audiobooks")
	if got := browseTitles(t, body); strings.Join(got, "|") != "Project Hail Mary" {
		t.Errorf("media_type=audiobooks = %v, want [Project Hail Mary]", got)
	}
	_, body = callBrowseWorks(t, s, "?media_type=ebooks")
	if got := browseTitles(t, body); strings.Join(got, "|") != "Piranesi" {
		t.Errorf("media_type=ebooks = %v, want [Piranesi]", got)
	}

	// `?kind=` is not a parameter this endpoint reads at all, so sending it is
	// an unfiltered page rather than a filtered one — which is why the
	// vocabulary check below matters.
	_, kindBody := callBrowseWorks(t, s, "?kind=comic")
	_, plain := callBrowseWorks(t, s, "")
	if strings.Join(browseTitles(t, kindBody), "|") != strings.Join(browseTitles(t, plain), "|") {
		t.Error("?kind= filtered something; this endpoint's filter is ?media_type=")
	}
}

// ⚠️ AN UNRECOGNISED media_type IS A 400 AND NEVER A SILENTLY UNFILTERED PAGE.
// Widening on a typo turns "show me my comics" into "show me everything", which
// looks like it worked. Includes the two kind spellings a caller is most likely
// to reach for, because they are the ones that would look plausible.
func TestBrowseEndpointRejectsAnUnknownMediaType(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	for _, v := range []string{"moovies", "movie", "series", "book", "Movies", "person", "games"} {
		code, body := callBrowseWorks(t, s, "?media_type="+v)
		if code != http.StatusBadRequest {
			t.Errorf("media_type=%s returned %d, want 400: %s", v, code, body)
			continue
		}
		if !strings.Contains(body, `"bad_request"`) || !strings.Contains(body, `"action"`) {
			t.Errorf("media_type=%s: the 400 carries no action: %s", v, body)
		}
	}
	// The caller's text is never echoed into a response body. Asserted on a
	// value that cannot appear inside the vocabulary the action lists.
	for _, v := range []string{"moovies", "<script>", "person"} {
		_, body := callBrowseWorks(t, s, "?media_type="+v)
		if strings.Contains(body, v) {
			t.Errorf("media_type=%s echoes the value back: %s", v, body)
		}
	}
}

// `?lib=` takes SLUGS, resolved through ux_library_slug, and an unknown one is a
// 400 rather than a wider page.
func TestBrowseEndpointScopesByLibrarySlug(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	code, body := callBrowseWorks(t, s, "?lib=books")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	want := []string{"Piranesi", "Project Hail Mary"}
	if got := browseTitles(t, body); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("?lib=books\n got: %v\nwant: %v", got, want)
	}

	// Multi-value, which is the shape §17.2's chip is a multi-select for.
	code, body = callBrowseWorks(t, s, "?lib=books,manga")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if len(browseTitles(t, body)) < 5 {
		t.Errorf("?lib=books,manga returned %v, which is not both libraries", browseTitles(t, body))
	}

	// ⚠️ AN UNKNOWN SLUG IS A 400. Dropping it silently widens the page, and
	// dropping every slug removes the scope entirely — so a bookmark to a
	// deleted library would answer with the whole catalogue.
	for _, q := range []string{"?lib=nope", "?lib=books,nope", "?lib=,", "?lib=unfiled"} {
		if code, body := callBrowseWorks(t, s, q); code != http.StatusBadRequest {
			t.Errorf("%s returned %d, want 400: %s", q, code, body)
		}
	}

	// ⚠️ AN EMPTY `?lib=` IS THE SAME REFUSAL, and it is the one that is easy to
	// get wrong: `?lib=,` reaches the split and is refused, while `?lib=` and
	// `?lib=%20` are refused only if the emptiness test runs BELOW the split.
	// Above it they read as "the parameter was never sent" and answer 200 with
	// the WHOLE CATALOGUE — the silent widening the endpoint exists to prevent,
	// and the worst version of it, because a bookmark to a deleted library
	// renders every work the caller can see instead of saying it is gone.
	code, body = callBrowseWorks(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("the unscoped browse returned %d: %s", code, body)
	}
	everything := len(browseTitles(t, body))
	for _, q := range []string{"?lib=", "?lib=%20", "?lib=%20%20"} {
		code, body := callBrowseWorks(t, s, q)
		if code != http.StatusBadRequest {
			t.Errorf("%s returned %d, want 400: %s", q, code, body)
		}
		if code == http.StatusOK && len(browseTitles(t, body)) == everything {
			t.Errorf("%s answered with the whole catalogue (%d works)", q, everything)
		}
	}

	// The reserved Unfiled library is never offered in the scope chip
	// (migration 0005), which is why its slug is among the refusals above.
	_, body = callBrowseWorks(t, s, "?lib=unfiled")
	if strings.Contains(body, "Unfiled") {
		t.Errorf("the refusal names the reserved library back: %s", body)
	}

	// The bound on how many slugs one request may carry is a REFUSAL, not a
	// clamp: silently dropping slugs widens the page.
	many := make([]string, 0, browseMaxLibrarySlugs+1)
	for i := 0; i <= browseMaxLibrarySlugs; i++ {
		many = append(many, "books")
	}
	if code, _ := callBrowseWorks(t, s, "?lib="+strings.Join(many, ",")); code != http.StatusBadRequest {
		t.Errorf("%d slugs returned %d, want 400", len(many), code)
	}
}

// The three orders are served under `library.default_sort`'s own vocabulary, and
// nothing else is.
func TestBrowseEndpointSortVocabularyIsTheSchemasOwn(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	for _, q := range []string{"?sort=added_at", "?sort=popularity", "?sort=sort_title&media_type=comics"} {
		if code, body := callBrowseWorks(t, s, q); code != http.StatusOK {
			t.Errorf("%s returned %d: %s", q, code, body)
		}
	}
	// `year` is legal in library.default_sort and unservable here — work.year
	// has no index — so it is a 400 that says so rather than a slow page.
	code, body := callBrowseWorks(t, s, "?sort=year")
	if code != http.StatusBadRequest {
		t.Errorf("sort=year returned %d, want 400: %s", code, body)
	}
	if !strings.Contains(body, "year") {
		t.Errorf("the sort=year refusal does not name the reason: %s", body)
	}
	// sort_title needs a media type of exactly one kind. `music` is two.
	if code, _ := callBrowseWorks(t, s, "?sort=sort_title&media_type=music"); code != http.StatusBadRequest {
		t.Errorf("sort_title over music returned %d, want 400", code)
	}
	if code, _ := callBrowseWorks(t, s, "?sort=sort_title"); code != http.StatusBadRequest {
		t.Errorf("sort_title with no media type returned %d, want 400", code)
	}
	// And a spelling nobody defined is refused rather than silently served as
	// the default order.
	for _, v := range []string{"newest", "az", "title", "added", "" + "SORT_TITLE"} {
		if code, _ := callBrowseWorks(t, s, "?sort="+v); code != http.StatusBadRequest {
			t.Errorf("sort=%s returned %d, want 400", v, code)
		}
	}
}

// Paging walks every page through the server's own cursor, and the cursor is
// refused when replayed under a different order — which is a state a client
// reaches by changing the sort chip without clearing the cursor.
func TestBrowseEndpointPagesAndRefusesACrossSortCursor(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	var all []string
	cursor := ""
	for page := 0; page < 20; page++ {
		q := "?limit=2"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		code, body := callBrowseWorks(t, s, q)
		if code != http.StatusOK {
			t.Fatalf("page %d: status %d: %s", page, code, body)
		}
		all = append(all, browseTitles(t, body)...)
		var env struct {
			Limit      int    `json:"limit"`
			NextCursor string `json:"next_cursor"`
			MediaType  string `json:"media_type"`
			Sort       string `json:"sort"`
		}
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatal(err)
		}
		if env.Limit != 2 {
			t.Fatalf("the server echoed limit=%d, not the 2 it applied", env.Limit)
		}
		if env.NextCursor == "" {
			break
		}
		cursor = env.NextCursor
	}
	seen := map[string]int{}
	for _, ti := range all {
		seen[ti]++
		if seen[ti] > 1 {
			t.Fatalf("the walk repeated %q: %v", ti, all)
		}
	}
	// It reached the undated tail, which is the boundary a plain row-value
	// keyset cannot cross.
	if !strings.Contains(strings.Join(all, "|"), "An Undated Manga") {
		t.Errorf("the walk never reached the undated tail: %v", all)
	}

	// A cursor minted under added_at, replayed under popularity: it decodes,
	// and it must still be refused rather than paging from a position that
	// means something else in the requested order.
	_, body := callBrowseWorks(t, s, "?limit=2")
	var env struct {
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatal(err)
	}
	if env.NextCursor == "" {
		t.Fatal("the fixture produced no second page")
	}
	if code, b := callBrowseWorks(t, s, "?sort=popularity&limit=2&cursor="+env.NextCursor); code != http.StatusBadRequest {
		t.Errorf("a cross-sort cursor replay returned %d, want 400: %s", code, b)
	}
	// And a token this endpoint never issued is a 400, never a silent reset to
	// page one — that turns a stale bookmark into a Load-more loop.
	if code, _ := callBrowseWorks(t, s, "?cursor=not-a-cursor"); code != http.StatusBadRequest {
		t.Errorf("a malformed cursor returned %d, want 400", code)
	}
	// Block C's tokens are not this endpoint's tokens.
	_, recent := callRecentWorks(t, s, "?limit=2")
	if err := json.Unmarshal([]byte(recent), &env); err != nil {
		t.Fatal(err)
	}
	if env.NextCursor != "" {
		if code, _ := callBrowseWorks(t, s, "?cursor="+env.NextCursor); code != http.StatusBadRequest {
			t.Errorf("a Block C cursor was accepted by the grid: %d", code)
		}
	}
}

// The envelope echoes the query the cursor belongs to, and omits both keys when
// the request named neither — so "no filter" stays distinguishable from
// "movies", and "no sort" from "added_at".
func TestBrowseEndpointEchoesTheQuery(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	envelope := func(query string) map[string]any {
		t.Helper()
		_, body := callBrowseWorks(t, s, query)
		// A FRESH map every time: json.Unmarshal MERGES into a non-empty map,
		// so reusing one would make an omitted key look present.
		env := map[string]any{}
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return env
	}

	env := envelope("?media_type=comics&sort=sort_title&lib=manga")
	if env["media_type"] != "comics" || env["sort"] != "sort_title" {
		t.Errorf("the envelope does not echo the query: %v", env)
	}
	// `lib` is a LIST on the wire, because the chip is a multi-select and a
	// comma-joined string would make the client re-parse what it just sent.
	if got, want := env["lib"], []any{"manga"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the envelope does not echo lib: got %#v, want %#v\n%v", got, want, env)
	}

	// Echoed in the order sent and trimmed to the slugs that were resolved, so a
	// client restoring a deep link can light the chips straight from it.
	env = envelope("?lib=%20books%20,manga")
	if got, want := env["lib"], []any{"books", "manga"}; !reflect.DeepEqual(got, want) {
		t.Errorf("multi-select lib echo: got %#v, want %#v", got, want)
	}

	env = envelope("")
	body := "the unfiltered envelope"
	if _, ok := env["media_type"]; ok {
		t.Errorf("an unfiltered page echoes a media_type: %s", body)
	}
	if _, ok := env["sort"]; ok {
		t.Errorf("a page with no sort echoes one: %s", body)
	}
	if _, ok := env["lib"]; ok {
		t.Errorf("a page with no library scope echoes one: %s", body)
	}
}

// The row is Block C's row, built by the same allowlist — asserted key for key
// so that growing the grid's row is a deliberate edit in one place rather than
// two structs drifting.
func TestBrowseEndpointRowIsBlockCsRow(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	keys := func(query string, call func(*testing.T, *Server, string) (int, string)) map[string]bool {
		_, body := call(t, s, query)
		var env struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatal(err)
		}
		if len(env.Items) == 0 {
			t.Fatalf("no rows to compare: %s", body)
		}
		out := map[string]bool{}
		for k := range env.Items[0] {
			out[k] = true
		}
		return out
	}
	grid := keys("", callBrowseWorks)
	block := keys("", callRecentWorks)
	for k := range grid {
		if !block[k] {
			t.Errorf("the grid row has %q and Block C's does not", k)
		}
	}
	for k := range block {
		if !grid[k] {
			t.Errorf("Block C's row has %q and the grid's does not", k)
		}
	}
	// Nothing service-side, on either.
	_, body := callBrowseWorks(t, s, "")
	for _, forbidden := range []string{"remote_path", "api_key", "service_instance", "content_hash"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the grid response carries %q: %s", forbidden, body)
		}
	}
}

// No session, no page. The route is behind s.authenticated, and the handler
// refuses on its own as well, so a future route edit cannot open it silently.
func TestBrowseEndpointNeedsASession(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/library", nil)
	w := httptest.NewRecorder()
	if err := s.handleBrowseWorks(w, r); err != nil {
		s.writeError(w, r, err)
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401: %s", w.Code, w.Body.String())
	}
}

// browseEnvelope decodes one browse response into a bare map, so that an
// OMITTED key is distinguishable from a zero-valued one. A struct with
// `omitempty` cannot tell those apart on the way back in, and the whole point of
// the echo is that absence carries a meaning.
func browseEnvelope(t *testing.T, s *Server, query string) (int, map[string]any) {
	t.Helper()
	code, body := callBrowseWorks(t, s, query)
	// A FRESH map every time: json.Unmarshal MERGES into a non-empty map, so
	// reusing one would make an omitted key look present.
	env := map[string]any{}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("%s: %v\n%s", query, err, body)
	}
	return code, env
}

// ⚠️ AN OMITTED `lib` MEANS "NO LIBRARY SCOPE WAS APPLIED", WITH NO SECOND
// READING — and this test pins the property the echo DEPENDS on rather than the
// echo itself.
//
// The echo is only worth having because every way of naming a scope that the
// server will not apply is a 400 (resolveBrowseLibraries: an unknown slug, a
// slug the caller cannot see, the reserved `unfiled`, an empty `?lib=`, and more
// than browseMaxLibrarySlugs of them). If any one of those ever became a silent
// drop instead, this endpoint would answer 200 with `lib` omitted over the WHOLE
// catalogue — and the omission a client reads as "you asked for no scope" would
// have come to also mean "you asked for a scope and did not get it". The echo
// exists to remove that ambiguity, so it must not be the thing that introduces
// it.
//
// The invariant, stated once and asserted over every spelling: FOR ANY REQUEST
// THAT CARRIED `lib`, THE ANSWER IS EITHER A 400 OR A 200 WHOSE ENVELOPE ECHOES
// `lib`. There is no third outcome, and in particular no 200 with the key
// missing.
func TestBrowseEnvelopeOmitsLibOnlyWhenNoScopeWasApplied(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	tooMany := make([]string, 0, browseMaxLibrarySlugs+1)
	for i := 0; i <= browseMaxLibrarySlugs; i++ {
		tooMany = append(tooMany, "books")
	}

	// Every spelling of `lib` the endpoint knows, servable and refused alike.
	// The refused half is the half that matters: each one is a request that
	// named a scope and must not be answered over the whole catalogue.
	for _, q := range []string{
		"?lib=books",
		"?lib=books,manga",
		"?lib=%20books%20,%20manga%20",
		"?lib=nope",
		"?lib=books,nope",
		"?lib=unfiled",
		"?lib=",
		"?lib=%20",
		"?lib=,",
		"?lib=,,",
		"?lib=" + strings.Join(tooMany, ","),
	} {
		code, env := browseEnvelope(t, s, q)
		switch code {
		case http.StatusBadRequest:
			// A refusal. The scope was not applied and the caller was told so.
		case http.StatusOK:
			if _, ok := env["lib"]; !ok {
				t.Errorf("%s answered 200 with no `lib` in the envelope, which a client "+
					"reads as `no scope was asked for` — the ambiguity the echo removes: %v",
					q, env)
			}
		default:
			t.Errorf("%s returned %d, which is neither the refusal nor a page", q, code)
		}
	}

	// The other half of the invariant: a request that carried no `lib` at all is
	// the ONLY thing that omits it.
	code, env := browseEnvelope(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("the unscoped browse returned %d", code)
	}
	if _, ok := env["lib"]; ok {
		t.Errorf("a request that named no library echoed one: %v", env)
	}
	// …and it really is unscoped, so "omitted" is not quietly covering a scope
	// that was applied from somewhere other than the query.
	everything := len(browseTitles(t, mustBody(t, s, "")))
	scoped := len(browseTitles(t, mustBody(t, s, "?lib=books")))
	if scoped >= everything {
		t.Errorf("?lib=books returned %d works and the unscoped page returned %d, so the "+
			"echoed scope is not filtering anything", scoped, everything)
	}
}

// mustBody is browse's body for a query that must answer 200.
func mustBody(t *testing.T, s *Server, query string) string {
	t.Helper()
	code, body := callBrowseWorks(t, s, query)
	if code != http.StatusOK {
		t.Fatalf("%s returned %d: %s", query, code, body)
	}
	return body
}
