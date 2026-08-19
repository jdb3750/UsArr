package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// newImageServer builds a server with an image cache directory and one work
// carrying a poster, one carrying a backdrop, and one pending asset.
//
// ⚠️ THE image_asset INSERTS ARE TEST-ONLY. internal/store's format lint exempts
// `_test.go`, so these do not satisfy it. The obligation to call
// store.ValidImageFormat — and to refuse a `source_url` still carrying a
// credential (security.md §5) — attaches to the PRODUCTION writer, which is now
// internal/store's PutPosterAsset and discharges both there. Nothing here is a
// model for one.
func newImageServer(t *testing.T) (*Server, string) {
	t.Helper()
	cacheDir := t.TempDir()
	s := newTestServer(t, func(c *Config) { c.ImageCacheDir = cacheDir })

	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'kavita', 'library', 'Kavita', 'http://kavita.example', X'00')`,
		`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format, state)
		   VALUES (1, 'http://kavita.example/c/1', 'configured', 'poster',
		           'aaaaaaaaaaaaaaaa', 'jpeg', 'ready')`,
		`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format, state)
		   VALUES (2, 'http://kavita.example/c/2', 'configured', 'backdrop',
		           'bbbbbbbbbbbbbbbb', 'jpeg', 'ready')`,
		// Pending: owned, visible, and with no bytes and no format.
		`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, state)
		   VALUES (3, 'http://kavita.example/c/3', 'configured', 'poster',
		           'cccccccccccccccc', 'pending')`,
		// A row naming a codec this binary has no media type for.
		`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format, state)
		   VALUES (4, 'http://kavita.example/c/4', 'configured', 'poster',
		           'dddddddddddddddd', 'avif', 'ready')`,
		`INSERT INTO work (id, kind, title, sort_title, normalized_title,
		                   poster_asset_id, backdrop_asset_id)
		   VALUES (1, 'comic', 'Berserk', 'berserk', 'berserk', 1, 2)`,
		`INSERT INTO work (id, kind, title, sort_title, normalized_title, poster_asset_id)
		   VALUES (2, 'comic', 'Vagabond', 'vagabond', 'vagabond', 3)`,
		`INSERT INTO work (id, kind, title, sort_title, normalized_title, poster_asset_id)
		   VALUES (3, 'comic', 'Blame', 'blame', 'blame', 4)`,
	}
	for id := 1; id <= 3; id++ {
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			   VALUES (1, %d, 'r%d', 'series', '2026-08-16 12:00:00')`, id, id))
	}

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
	return s, cacheDir
}

// writeCachedImage puts bytes where the serving path looks for them, through the
// same package the serving path derives its layout from — never through a path
// spelled out here. A hand-written path is the one thing this fixture must not
// have: it would make the layout agree with itself and disagree with production.
func writeCachedImage(t *testing.T, root, key, width string, body []byte) {
	t.Helper()
	dir := filepath.Join(root, key[:2], key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, width), body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// callImage runs the handler with a session already resolved and the path value
// set, which is what the mux and the authenticated middleware do before it.
func callImage(t *testing.T, s *Server, key, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/img/"+key+query, nil)
	r.SetPathValue("key", key)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: true},
	}))
	w := httptest.NewRecorder()
	if err := s.handleImage(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w
}

// The happy path: bytes on disk, served with the Content-Type the ROW names.
func TestImageServesCachedBytesWithTheStoredFormatsMediaType(t *testing.T) {
	s, cacheDir := newImageServer(t)
	body := []byte("not really a jpeg, and that is the point")
	writeCachedImage(t, cacheDir, "aaaaaaaaaaaaaaaa", imagecache.Width342, body)

	w := callImage(t, s, "aaaaaaaaaaaaaaaa", "?w=342")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != string(body) {
		t.Errorf("body = %q, want the cached bytes", got)
	}
	if got := w.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
}

// ⚠️ THE CONTENT-TYPE IS DERIVED FROM image_asset.format AND FROM NOTHING ELSE.
// The cached bytes here are PNG magic while the row says jpeg, so a handler that
// sniffed the bytes — or that let http.ServeContent sniff them, which is what
// ServeContent does when Content-Type is unset — answers image/png and fails.
// A test written with real JPEG bytes could not tell the two implementations
// apart.
func TestImageContentTypeIsNotSniffedFromTheBytes(t *testing.T) {
	s, cacheDir := newImageServer(t)
	// PNG's eight-byte signature, which is exactly what net/http's sniffer
	// keys on.
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
		make([]byte, 600)...)
	writeCachedImage(t, cacheDir, "aaaaaaaaaaaaaaaa", imagecache.WidthOrig, png)

	w := callImage(t, s, "aaaaaaaaaaaaaaaa", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg.\n"+
			"The bytes on disk are a PNG and the row says jpeg. The row wins: the "+
			"bytes are produced by UsArr's own encoder in the codec the column names "+
			"(ADR-0050), so the column is the only party that knows the encoding, "+
			"and a sniff is a guess made from content.", got)
	}
	// The global nosniff header is what stops the BROWSER re-deciding, and it
	// is only safe because the value above is derived rather than observed.
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	if got := headerThrough(t, srv, "/img/aaaaaaaaaaaaaaaa", "X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

// ⚠️ `Cache-Control: private`, FOR EVERY ROW, INCLUDING PROVIDER ARTWORK.
// A content-derived key justifies immutability, not publicness. `public` here
// would tell a shared cache it may re-serve one user's authorized response to
// another user, which converts the authorization check into no check at all for
// every request after the first. The directive is a constant, so this walks
// every origin_class the schema admits and asserts one answer.
func TestImageCacheControlIsAlwaysPrivate(t *testing.T) {
	s, cacheDir := newImageServer(t)

	// origin_class's CHECK admits exactly these three.
	classes := []string{"configured", "derived", "provider"}
	keys := map[string]string{
		"configured": "aaaaaaaaaaaaaaaa",
		"derived":    "eeeeeeeeeeeeeeee",
		"provider":   "ffffffffffffffff",
	}
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		id := int64(10)
		for _, class := range classes[1:] {
			for _, q := range []string{
				fmt.Sprintf(`INSERT INTO image_asset (id, source_url, origin_class, role,
				                                      cache_key, format, state)
				   VALUES (%d, 'http://p/%d', '%s', 'poster', '%s', 'jpeg', 'ready')`,
					id, id, class, keys[class]),
				fmt.Sprintf(`INSERT INTO work (id, kind, title, sort_title, normalized_title,
				                               poster_asset_id)
				   VALUES (%d, 'comic', 'W%d', 'w%d', 'w%d', %d)`, id, id, id, id, id),
				fmt.Sprintf(`INSERT INTO service_item_link
				               (service_instance_id, work_id, remote_id, remote_kind, synced_at)
				   VALUES (1, %d, 'r%d', 'series', '2026-08-16 12:00:00')`, id, id),
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return fmt.Errorf("%s: %w", q, err)
				}
			}
			id++
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, class := range classes {
		t.Run(class, func(t *testing.T) {
			key := keys[class]
			writeCachedImage(t, cacheDir, key, imagecache.WidthOrig, []byte("bytes"))
			w := callImage(t, s, key, "")
			if w.Code != http.StatusOK {
				t.Fatalf("status %d, want 200: %s", w.Code, w.Body.String())
			}
			got := w.Header().Get("Cache-Control")
			if !strings.HasPrefix(got, "private") {
				t.Errorf("Cache-Control = %q for origin_class %q.\n"+
					"Every /img response is private, whatever the artwork's origin. "+
					"`public` on a per-user-authorized resource lets a shared cache "+
					"re-serve it across users (security.md §4).", got, class)
			}
			if strings.Contains(got, "public") {
				t.Errorf("Cache-Control = %q carries `public`", got)
			}
			if !strings.Contains(got, "immutable") {
				t.Errorf("Cache-Control = %q drops `immutable`", got)
			}
		})
	}
}

// The width allowlist is a REFUSAL, not a clamp. An arbitrary `?w=` mints an
// unbounded number of cache entries, each costing a decode, a downscale, an
// encode and a file — the resource the cache exists to protect (§4.4,
// GHSA-rrr6-mvwg-9pg9).
func TestImageWidthIsAnAllowlistedRefusal(t *testing.T) {
	s, cacheDir := newImageServer(t)
	for _, width := range imagecache.Widths() {
		writeCachedImage(t, cacheDir, "aaaaaaaaaaaaaaaa", width, []byte(width))
	}

	for _, width := range imagecache.Widths() {
		w := callImage(t, s, "aaaaaaaaaaaaaaaa", "?w="+width)
		if w.Code != http.StatusOK {
			t.Errorf("?w=%s = %d, want 200: %s", width, w.Code, w.Body.String())
		}
		if got := w.Body.String(); got != width {
			t.Errorf("?w=%s served the bytes for %q — the width is not part of the "+
				"cache path", width, got)
		}
	}

	for _, width := range []string{
		"343", "0", "-1", "99999", "1e9", "342px", " 342 x", "orig ", " orig", "92 ",
		"../../etc/passwd", "%2e%2e", "342;", "ORIG", "<script>",
	} {
		w := callImage(t, s, "aaaaaaaaaaaaaaaa", "?w="+url.QueryEscape(width))
		if w.Code != http.StatusBadRequest {
			t.Errorf("?w=%s = %d, want 400 — an unlisted width is refused, never "+
				"clamped to a neighbour", width, w.Code)
			continue
		}
		// The MESSAGE, not the whole body: the action deliberately echoes the
		// ALLOWLIST, and a rejected value that is a substring of it ("0" inside
		// "200") would make a whole-body check assert nothing.
		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("error body is not JSON: %s", w.Body.String())
		}
		if strings.Contains(body.Message, strings.TrimSpace(width)) && strings.TrimSpace(width) != "" {
			t.Errorf("?w=%s is echoed back into the message: %q", width, body.Message)
		}
	}

	// No `?w=` at all is `orig`, not a refusal and not a silent thumbnail.
	if w := callImage(t, s, "aaaaaaaaaaaaaaaa", ""); w.Code != http.StatusOK ||
		w.Body.String() != imagecache.WidthOrig {
		t.Errorf("no ?w= = %d %q, want 200 and the orig bytes", w.Code, w.Body.String())
	}
}

// Not-found is ONE answer for "no such key" and for "not yours". The asset key
// on a browse response is a property of a row the caller can already see, and
// this is the other half of keeping it that way: a caller who guesses a key
// learns nothing a browse response would not have told them.
func TestImageDoesNotDiscloseWhatTheCallerMayNotSee(t *testing.T) {
	s, cacheDir := newImageServer(t)
	writeCachedImage(t, cacheDir, "aaaaaaaaaaaaaaaa", imagecache.WidthOrig, []byte("bytes"))

	// A non-owner: storeScope gives an empty visible set, which the store
	// renders as `1=0`. This is v0.1's only non-owner shape and it is the one
	// that must fail closed.
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/img/aaaaaaaaaaaaaaaa", nil)
	r.SetPathValue("key", "aaaaaaaaaaaaaaaa")
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 2, Username: "kid", IsOwner: false},
	}))
	w := httptest.NewRecorder()
	if err := s.handleImage(w, r); err != nil {
		s.writeError(w, r, err)
	}
	hidden := w.Body.String()
	if w.Code != http.StatusNotFound {
		t.Fatalf("a caller with an empty scope got %d for an existing image: %s", w.Code, hidden)
	}

	// A key that names nothing at all. Byte-identical body, so nothing in the
	// response separates the two.
	absent := callImage(t, s, "9999999999999999", "")
	if absent.Code != http.StatusNotFound {
		t.Fatalf("an unknown key = %d, want 404", absent.Code)
	}
	if hidden != absent.Body.String() {
		t.Errorf("the two 404s differ, which makes the pair an existence oracle:\n"+
			"  hidden: %s\n  absent: %s", hidden, absent.Body.String())
	}

	// The positive control: the OWNER does get it, so the refusal above is the
	// scope and not an empty fixture.
	if ok := callImage(t, s, "aaaaaaaaaaaaaaaa", ""); ok.Code != http.StatusOK {
		t.Fatalf("the owner cannot fetch the same image (%d), so the assertions "+
			"above are vacuous: %s", ok.Code, ok.Body.String())
	}
}

// A malformed key never reaches the filesystem, and the traversal it would be is
// blocked by the alphabet rather than by a cleaning step.
func TestImageRefusesAKeyThatIsNotOne(t *testing.T) {
	s, _ := newImageServer(t)
	for _, key := range []string{
		"", "..", "../../etc/passwd", "aaaaaaaaaaaaaaaa/../../x",
		"AAAAAAAAAAAAAAAA", "aaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaa",
		"aaaaaaaaaaaaaaa/", "aaaaaaaaaaaa%00", "aaaaaaaaaaaaaaa'",
	} {
		w := callImage(t, s, key, "")
		if w.Code != http.StatusNotFound {
			t.Errorf("key %q = %d, want 404: %s", key, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), key) && key != "" {
			t.Errorf("key %q is echoed into the response: %s", key, w.Body.String())
		}
	}
}

// A row with no bytes answers not_cached, NOT not_found, and the split is what
// lets a client render §4.4.1's cold start as a placeholder rather than as a
// broken image.
func TestImageDistinguishesNotRenderedYetFromNoSuchImage(t *testing.T) {
	s, cacheDir := newImageServer(t)
	writeCachedImage(t, cacheDir, "aaaaaaaaaaaaaaaa", imagecache.WidthOrig, []byte("bytes"))

	code := func(w *httptest.ResponseRecorder) string {
		t.Helper()
		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("error body is not JSON: %s", w.Body.String())
		}
		return string(body.Error)
	}

	for _, tc := range []struct {
		name, key, query, want string
	}{
		// A pending row: visible, no format, no bytes.
		{"pending row", "cccccccccccccccc", "", string(CodeNotCached)},
		// Ready, formatted, and nothing on disk at that width.
		{"ready but not at this width", "aaaaaaaaaaaaaaaa", "?w=780", string(CodeNotCached)},
		// A codec this binary has no media type for. It is NOT served as
		// octet-stream and the header is not omitted.
		{"unknown codec", "dddddddddddddddd", "", string(CodeNotCached)},
		// A key naming no row at all.
		{"no such asset", "9999999999999999", "", string(CodeNotFound)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := callImage(t, s, tc.key, tc.query)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status %d, want 404: %s", w.Code, w.Body.String())
			}
			if got := code(w); got != tc.want {
				t.Errorf("error code = %q, want %q: %s", got, tc.want, w.Body.String())
			}
			if ct := w.Header().Get("Content-Type"); strings.HasPrefix(ct, "image/") {
				t.Errorf("a refusal carries Content-Type %q", ct)
			}
		})
	}
}

// A process with no image cache configured answers not_cached rather than
// reading from the working directory — principle 3's "degrades honestly", and
// the difference between an empty root and a relative path.
func TestImageWithNoCacheDirConfiguredIsHonest(t *testing.T) {
	s := newTestServer(t, nil) // ImageCacheDir stays empty
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
			   VALUES (1, 'kavita', 'library', 'Kavita', 'http://kavita.example', X'00')`,
			`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format, state)
			   VALUES (1, 'http://k/1', 'configured', 'poster', 'aaaaaaaaaaaaaaaa', 'jpeg', 'ready')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title, poster_asset_id)
			   VALUES (1, 'comic', 'Berserk', 'berserk', 'berserk', 1)`,
			`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			   VALUES (1, 1, 'r1', 'series', '2026-08-16 12:00:00')`,
		} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := callImage(t, s, "aaaaaaaaaaaaaaaa", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), string(CodeNotCached)) {
		t.Errorf("an unconfigured cache did not answer not_cached: %s", w.Body.String())
	}
}

// ── the route, through the real mux ─────────────────────────────────────────

// The route is registered, it is behind the session gate, and it is not shadowed
// by the SPA fallback at "/".
func TestImageRouteIsRegisteredAndAuthenticated(t *testing.T) {
	s, cacheDir := newImageServer(t)
	writeCachedImage(t, cacheDir, "aaaaaaaaaaaaaaaa", imagecache.WidthOrig, []byte("bytes"))

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	c := &cookieClient{t: t, jar: map[string]string{}}
	if code, body := c.get(srv.URL + "/img/aaaaaaaaaaaaaaaa"); code != http.StatusUnauthorized {
		t.Fatalf("an anonymous /img request = %d, want 401: %s", code, body)
	}

	c.get(srv.URL + "/api/v1/auth/session")
	if code, body := c.postJSON(srv.URL+"/api/v1/auth/setup",
		`{"username":"joe","password":"correct horse battery"}`); code != http.StatusOK {
		t.Fatalf("setup = %d: %s", code, body)
	}
	code, body := c.get(srv.URL + "/img/aaaaaaaaaaaaaaaa")
	if code != http.StatusOK {
		t.Fatalf("a signed-in owner = %d, want 200: %s", code, body)
	}
	if body != "bytes" {
		t.Errorf("body = %q, want the cached bytes — the SPA fallback at / may be "+
			"shadowing the route", body)
	}
}

// ⚠️ NO PUBLIC BYPASS. security.md §4 requires that genuinely public artwork get
// a DISTINCT path "so the distinction is structural, not conditional". Nothing
// produces provider artwork yet, so /img/public/* is deliberately not
// registered — and the structural half is that this route has no way to become
// it. This asserts both: the public path is not routed as an image, and no
// spelling of the private one is reachable without a session.
func TestImageRouteHasNoPublicBypass(t *testing.T) {
	s, cacheDir := newImageServer(t)
	writeCachedImage(t, cacheDir, "aaaaaaaaaaaaaaaa", imagecache.WidthOrig, []byte("bytes"))

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	c := &cookieClient{t: t, jar: map[string]string{}}

	for _, path := range []string{
		"/img/public/aaaaaaaaaaaaaaaa",
		"/img/aaaaaaaaaaaaaaaa?public=1",
		"/img/aaaaaaaaaaaaaaaa?origin_class=provider",
	} {
		code, body := c.get(srv.URL + path)
		if code == http.StatusOK {
			t.Errorf("%s served bytes without a session: %s", path, body)
		}
		if strings.Contains(body, "bytes") {
			t.Errorf("%s leaked the cached bytes", path)
		}
	}
}

func headerThrough(t *testing.T, srv *httptest.Server, path, name string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.Header.Get(name)
}

// ── the key on the browse responses ─────────────────────────────────────────

// posterKeys reads `poster_key` off every row of an items envelope, keyed by
// title so the two endpoints can be compared row for row.
func posterKeys(t *testing.T, body string) map[string]string {
	t.Helper()
	var env struct {
		Items []struct {
			Title     string `json:"title"`
			PosterKey string `json:"poster_key"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("body is not an items envelope: %s", body)
	}
	out := map[string]string{}
	for _, it := range env.Items {
		out[it.Title] = it.PosterKey
	}
	return out
}

// ⚠️ store.RecentWork REACHES TWO REGISTERED ENDPOINTS, and a field added to it
// lands on the library grid as well as on Home's recently-added. Both allowlist
// tests are single-sided — each compares one response against a hand-copied list
// of its own — so this asserts the RELATIONSHIP: the same work carries the same
// key on both, and the row that has no poster carries none on either.
//
// It is a VALUE assertion and not a key-set one. TestBrowseEndpointRowIsBlockCsRow
// already pins that the two rows have the same KEYS, and it would stay green with
// this field hardcoded to "" on one of the two handlers.
func TestPosterKeyCrossesBothBrowseRenderPaths(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	_, recentBody := callRecentWorks(t, s, "")
	_, gridBody := callBrowseWorks(t, s, "")
	recent := posterKeys(t, recentBody)
	grid := posterKeys(t, gridBody)

	// Berserk is the one seeded work with a poster.
	const key = "aaaaaaaaaaaaaaaa"
	if recent["Berserk"] != key {
		t.Errorf("Block C's Berserk row carries poster_key %q, want %q:\n%s",
			recent["Berserk"], key, recentBody)
	}
	if grid["Berserk"] != key {
		t.Errorf("the grid's Berserk row carries poster_key %q, want %q:\n%s",
			grid["Berserk"], key, gridBody)
	}

	// And absent, not empty, on a work with no poster — asserted on the RAW
	// row, because the struct above turns an absent key and an empty one into
	// the same string.
	var raw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(recentBody), &raw); err != nil {
		t.Fatal(err)
	}
	saw := false
	for _, item := range raw.Items {
		title := strings.Trim(string(item["title"]), `"`)
		if title == "Berserk" {
			continue
		}
		saw = true
		if _, present := item["poster_key"]; present {
			t.Errorf("%q has no poster asset and its row carries a poster_key: %s",
				title, item["poster_key"])
		}
	}
	if !saw {
		t.Fatal("no posterless rows in the fixture, so the absence assertion is vacuous")
	}

	// Every key on both responses must be a key the /img route would accept —
	// otherwise a client builds a URL that 404s on every render.
	for name, keys := range map[string]map[string]string{"recent": recent, "grid": grid} {
		for title, k := range keys {
			if k != "" && !store.ValidImageCacheKey(k) {
				t.Errorf("%s row %q ships %q, which /img refuses", name, title, k)
			}
		}
	}
}

// THE END TO END: the key a browse response publishes is the key the /img route
// serves. This is the whole claim of the serving half, and neither the browse
// tests nor the image tests can make it alone — one knows the key and not the
// route, the other knows the route and not where the key came from.
func TestThePosterKeyOnTheBrowseResponseResolvesThroughImg(t *testing.T) {
	cacheDir := t.TempDir()
	s := newTestServer(t, func(c *Config) { c.ImageCacheDir = cacheDir })
	seedLibraryCorpus(t, s)

	_, body := callBrowseWorks(t, s, "")
	key := posterKeys(t, body)["Berserk"]
	if key == "" {
		t.Fatal("the grid response carries no poster_key, so there is nothing to resolve")
	}

	// Before the bytes exist: a real answer, and the one that says "not yet".
	if w := callImage(t, s, key, "?w=342"); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), string(CodeNotCached)) {
		t.Errorf("an unrendered poster = %d %s, want 404 not_cached", w.Code, w.Body.String())
	}

	writeCachedImage(t, cacheDir, key, imagecache.Width342, []byte("the poster"))
	w := callImage(t, s, key, "?w=342")
	if w.Code != http.StatusOK {
		t.Fatalf("the published key does not resolve: %d %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "the poster" {
		t.Errorf("body = %q", w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
}
