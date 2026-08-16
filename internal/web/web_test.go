package web

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
)

// requireBuilt skips a test when no frontend build is embedded. That is the
// state of a fresh clone before `make web-build` has run; it is not a failure,
// and a bare `go test ./...` must not turn it into one. Everything below that
// asserts on build *content* goes through here.
//
// USARR_REQUIRE_WEB_BUILD=1 turns the skip into a failure. `make test-go` sets
// it, because it depends on `web-build` and has therefore just produced the
// build these tests need. Without that, the skip was load-bearing in the wrong
// direction: the gate ran in a tree with nothing embedded, so
// TestEmbeddedFSCarriesAppDir — the only thing that catches a lost `all:`
// prefix, which silently drops the entire application bundle — never executed,
// and a commit reintroducing the trap would have gone green.
func requireBuilt(t *testing.T) fs.FS {
	t.Helper()
	if !Built() {
		if os.Getenv("USARR_REQUIRE_WEB_BUILD") == "1" {
			t.Fatal("USARR_REQUIRE_WEB_BUILD=1 but no frontend build is embedded: " +
				"web/scripts/sync-embed.mjs did not populate internal/web/spa. " +
				"This test must not be allowed to skip inside the gate — it is the " +
				"regression test for the //go:embed `all:` prefix.")
		}
		t.Skip("no frontend build embedded — run `make web-build` (see docs/DEVELOPMENT.md §3)")
	}
	return FS()
}

// TestEmbeddedFSCarriesAppDir is the regression test for ADR-0024 §6's first
// trap. Go's embed excludes names beginning with '_' unless the pattern carries
// the `all:` prefix, and SvelteKit's appDir is `_app`. Dropping `all:` still
// embeds index.html, so nothing errors — the app is simply gone and the browser
// shows a blank page. This test is the only thing that catches that.
func TestEmbeddedFSCarriesAppDir(t *testing.T) {
	files := requireBuilt(t)

	info, err := fs.Stat(files, "_app")
	if err != nil {
		t.Fatalf("_app is missing from the embedded FS: %v\n"+
			"the //go:embed directive in web.go has almost certainly lost its `all:` prefix; "+
			"without it Go silently drops every path beginning with '_' or '.'", err)
	}
	if !info.IsDir() {
		t.Fatalf("_app is embedded but is not a directory")
	}

	var scripts int
	err = fs.WalkDir(files, "_app", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && path.Ext(p) == ".js" {
			scripts++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking _app: %v", err)
	}
	if scripts == 0 {
		t.Fatalf("_app is embedded but contains no JavaScript — the bundle did not make it in")
	}
}

// TestFallbackAssetPathsAreRootAbsolute is the empirical answer to ADR-0024
// §6's fourth trap, which recorded `paths.relative: false` as an untested
// inference. The property that actually has to hold is this one: the fallback
// document's asset URLs must not be relative, because the same document is
// served at every depth. Whatever svelte.config.js says, this is what breaks
// deep routes if it regresses.
func TestFallbackAssetPathsAreRootAbsolute(t *testing.T) {
	files := requireBuilt(t)

	index := readAll(t, files, indexFile)
	if !strings.Contains(index, `"/_app/`) {
		t.Fatalf("index.html references no root-absolute /_app/ asset:\n%s", index)
	}
	for _, bad := range []string{`"./_app/`, `"../_app/`, `"_app/`} {
		if strings.Contains(index, bad) {
			t.Errorf("index.html contains a relative asset reference %q; served at /search it would "+
				"resolve to /search/_app/... and 404. Set kit.paths.relative = false.", bad)
		}
	}
}

func TestHandlerServesRoot(t *testing.T) {
	requireBuilt(t)
	res := get(t, "/")

	if res.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", res.Code)
	}
	if ct := res.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}
	if body := res.Body.String(); !strings.Contains(body, "<!doctype html>") {
		t.Errorf("GET / did not return the SPA document:\n%s", body)
	}
	if cc := res.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("GET / Cache-Control = %q, want no-cache", cc)
	}
}

// TestHandlerServesDeepRoute is the SPA fallback: /search is a client-side
// route with no file behind it and must still get the document, at status 200.
func TestHandlerServesDeepRoute(t *testing.T) {
	requireBuilt(t)

	for _, deep := range []string{"/search", "/search?query=dune", "/library/movies/12345"} {
		res := get(t, deep)
		if res.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", deep, res.Code)
			continue
		}
		if ct := res.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s Content-Type = %q, want text/html", deep, ct)
		}
		if !strings.Contains(res.Body.String(), "<!doctype html>") {
			t.Errorf("GET %s did not return the SPA document", deep)
		}
	}
}

// TestDeepRouteAssetsResolve walks the whole path a browser walks: fetch
// /search, pull an asset URL out of the returned document, resolve it against
// the request URL exactly as a browser would, and fetch that. If the asset
// references were relative this is the test that fails.
func TestDeepRouteAssetsResolve(t *testing.T) {
	requireBuilt(t)

	const deep = "/search"
	doc := get(t, deep).Body.String()

	assets := extractAppRefs(doc)
	if len(assets) == 0 {
		t.Fatalf("no _app asset references found in the document served at %s", deep)
	}

	base, err := url.Parse("http://usarr.test" + deep)
	if err != nil {
		t.Fatalf("parsing base: %v", err)
	}

	for _, ref := range assets {
		resolved, err := base.Parse(ref)
		if err != nil {
			t.Errorf("asset %q does not parse: %v", ref, err)
			continue
		}
		res := get(t, resolved.Path)
		if res.Code != http.StatusOK {
			t.Errorf("asset %q referenced from %s resolved to %s and returned %d, want 200",
				ref, deep, resolved.Path, res.Code)
			continue
		}
		ct := res.Header().Get("Content-Type")
		switch path.Ext(resolved.Path) {
		case ".js":
			if !strings.Contains(ct, "javascript") {
				t.Errorf("asset %s Content-Type = %q, want a JavaScript type", resolved.Path, ct)
			}
		case ".css":
			if !strings.HasPrefix(ct, "text/css") {
				t.Errorf("asset %s Content-Type = %q, want text/css", resolved.Path, ct)
			}
		}
		if cc := res.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Errorf("hashed asset %s Cache-Control = %q, want an immutable policy", resolved.Path, cc)
		}
	}
}

// TestMissingAssetIs404 guards the rule that a request that looks like a file
// never gets the SPA document. Handing HTML back for a missing .js turns a
// broken path into an unexplainable console error.
func TestMissingAssetIs404(t *testing.T) {
	requireBuilt(t)

	for _, p := range []string{"/_app/immutable/nope.js", "/favicon.ico", "/robots.txt"} {
		if res := get(t, p); res.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, res.Code)
		}
	}
}

func TestPathTraversalIsRefused(t *testing.T) {
	requireBuilt(t)

	// path.Clean collapses these to /index.html or /, so the worst case is the
	// SPA document. What must never happen is a file from outside the tree.
	for _, p := range []string{"/../web.go", "/_app/../../web.go", "//etc/passwd"} {
		res := get(t, p)
		if strings.Contains(res.Body.String(), "package web") {
			t.Errorf("GET %s escaped the embedded tree", p)
		}
	}
}

func TestNonReadMethodsRejected(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/", nil)
		res := httptest.NewRecorder()
		Handler().ServeHTTP(res, req)
		if res.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s / = %d, want 405", method, res.Code)
		}
		if allow := res.Header().Get("Allow"); allow != "GET, HEAD" {
			t.Errorf("%s / Allow = %q, want \"GET, HEAD\"", method, allow)
		}
	}
}

// TestUnbuiltHandlerFailsHonestly covers the fresh-clone binary: it must say
// what is wrong and how to fix it, not serve a blank page.
func TestUnbuiltHandlerFailsHonestly(t *testing.T) {
	h := &handler{files: FS(), built: false}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("unbuilt GET / = %d, want 404", res.Code)
	}
	if !strings.Contains(res.Body.String(), "make build") {
		t.Errorf("the unbuilt response does not name the fix:\n%s", res.Body.String())
	}
}

// TestOverRealHTTPServer runs the same three requests over an actual socket
// rather than through a ResponseRecorder, so anything that only shows up with a
// real net/http server in front of the handler shows up here.
func TestOverRealHTTPServer(t *testing.T) {
	files := requireBuilt(t)

	server := httptest.NewServer(Handler())
	t.Cleanup(server.Close)

	asset := firstAppScript(t, files)

	cases := []struct {
		path        string
		wantType    string
		wantContent string
	}{
		{"/", "text/html", "<!doctype html>"},
		{"/search", "text/html", "<!doctype html>"},
		{"/" + asset, "javascript", ""},
	}

	for _, tc := range cases {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+tc.path, nil)
		if err != nil {
			t.Fatalf("building request for %s: %v", tc.path, err)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Errorf("GET %s: %v", tc.path, err)
			continue
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", tc.path, res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, tc.wantType) {
			t.Errorf("GET %s Content-Type = %q, want it to contain %q", tc.path, ct, tc.wantType)
		}
		if tc.wantContent != "" && !strings.Contains(string(body), tc.wantContent) {
			t.Errorf("GET %s body does not contain %q", tc.path, tc.wantContent)
		}
		if len(body) == 0 {
			t.Errorf("GET %s returned an empty body", tc.path)
		}
	}
}

// --- helpers ---------------------------------------------------------------

// firstAppScript returns the path of some JavaScript file under _app, in
// deterministic order so a failure names the same file every run.
func firstAppScript(t *testing.T, files fs.FS) string {
	t.Helper()
	var found string
	err := fs.WalkDir(files, "_app", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found == "" && !d.IsDir() && path.Ext(p) == ".js" {
			found = p
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking _app: %v", err)
	}
	if found == "" {
		t.Fatal("no JavaScript found under _app")
	}
	return found
}

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	res := httptest.NewRecorder()
	Handler().ServeHTTP(res, req)
	return res
}

func readAll(t *testing.T, files fs.FS, name string) string {
	t.Helper()
	f, err := files.Open(name)
	if err != nil {
		t.Fatalf("opening %s: %v", name, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(b)
}

// extractAppRefs pulls every quoted reference containing "_app/" out of the
// document — href=, src=, and the dynamic import() calls in the bootstrap
// script all use the same quoting.
func extractAppRefs(doc string) []string {
	var refs []string
	seen := map[string]bool{}
	for _, quote := range []byte{'"', '\''} {
		rest := doc
		for {
			start := strings.IndexByte(rest, quote)
			if start < 0 {
				break
			}
			end := strings.IndexByte(rest[start+1:], quote)
			if end < 0 {
				break
			}
			candidate := rest[start+1 : start+1+end]
			rest = rest[start+1+end+1:]
			if strings.Contains(candidate, "_app/") && !seen[candidate] {
				seen[candidate] = true
				refs = append(refs, candidate)
			}
		}
	}
	return refs
}
