// Package web serves UsArr's single-page frontend out of the binary.
//
// The frontend is built by `make web-build` (SvelteKit + adapter-static, output
// in web/build) and mirrored into this package's spa/ directory by
// web/scripts/sync-embed.mjs, because //go:embed cannot reach outside the
// embedding package's own directory. See docs/DEVELOPMENT.md §2 and ADR-0025 §6.
//
// A binary built without running the frontend build still compiles — spa/ is
// tracked in git with nothing but a .gitkeep — and reports that honestly rather
// than serving a blank page. Principle 3: degrade honestly, never render an
// empty screen that looks broken.
package web

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// The `all:` prefix is mandatory and is the single most important character
// sequence in this file. SvelteKit's appDir defaults to `_app`, and go:embed
// excludes files whose names begin with '.' or '_' unless the pattern is
// prefixed with `all:` (https://pkg.go.dev/embed). Without it this embeds
// index.html and the favicon, drops the entire application bundle, and the
// result is a blank page with no error anywhere. TestEmbeddedFSCarriesAppDir
// fails if that regression is ever reintroduced.
//
//go:embed all:spa
var embedded embed.FS

// indexFile is the SPA fallback document. Every route that is not a real file
// resolves to it and is then routed client-side.
const indexFile = "index.html"

// buildDir is the subdirectory of the embedded FS holding the frontend build.
const buildDir = "spa"

// FS returns the embedded frontend build, rooted so that "index.html" and
// "_app/..." are top-level entries. It is exported for tests and for any future
// handler that wants to serve the same bytes differently.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, buildDir)
	if err != nil {
		// Unreachable: buildDir is a compile-time constant that go:embed
		// already resolved, so fs.Sub cannot fail on it.
		panic("internal/web: embedded build directory is unusable: " + err.Error())
	}
	return sub
}

// Built reports whether a frontend build is actually embedded. It is false in a
// tree where `make web-build` has never run.
func Built() bool {
	_, err := fs.Stat(FS(), indexFile)
	return err == nil
}

// Handler serves the embedded SPA.
//
// Routing, in order:
//
//   - a request whose path names a real embedded file gets that file;
//   - a request whose path has a file extension but names nothing gets 404,
//     so a missing asset fails loudly instead of being handed HTML;
//   - anything else — /, /search, /library/movies/12345 — gets index.html with
//     status 200, which is what makes deep links work in an SPA.
//
// The handler is read-only and stateless; one instance can serve every request.
func Handler() http.Handler {
	return &handler{files: FS(), built: Built()}
}

type handler struct {
	files fs.FS
	built bool
}

// modTime is fixed rather than time.Now(): the embedded bytes never change for a
// given binary, so a stable Last-Modified lets conditional requests work and
// keeps responses byte-identical across restarts.
var modTime = time.Time{}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.built {
		// Honest, specific, and actionable — see docs/DEVELOPMENT.md §3: a bare
		// `go build ./cmd/usarr` produces exactly this binary.
		http.Error(w,
			"the UsArr frontend is not embedded in this binary: build it with `make build`, which runs `make web-build` first",
			http.StatusNotFound)
		return
	}

	name := resolve(r.URL.Path)

	if name != "" {
		if h.serveFile(w, r, name) {
			return
		}
		// A request for something that looks like a file must not be answered
		// with the SPA document. Handing HTML to a <script src> is how a broken
		// asset path turns into an unexplainable syntax error in the console.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}

	h.serveIndex(w, r)
}

// resolve maps a request path onto a name inside the embedded FS, or "" for the
// root. It rejects anything that escapes the tree.
func resolve(urlPath string) string {
	cleaned := path.Clean("/" + urlPath)
	trimmed := strings.TrimPrefix(cleaned, "/")
	if trimmed == "" || trimmed == "." {
		return ""
	}
	if !fs.ValidPath(trimmed) {
		return ""
	}
	return trimmed
}

// serveFile writes the named embedded file and reports whether it did.
func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	file, err := h.files.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return false
	}

	setCacheHeaders(w, name)
	// ServeContent derives Content-Type from the extension and handles Range and
	// If-Modified-Since for us.
	http.ServeContent(w, r, path.Base(name), modTime, seeker)
	return true
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	file, err := h.files.Open(indexFile)
	if err != nil {
		http.Error(w, "the UsArr frontend is not embedded in this binary", http.StatusNotFound)
		return
	}
	defer func() { _ = file.Close() }()

	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		http.Error(w, "the embedded frontend document is unreadable", http.StatusInternalServerError)
		return
	}

	setCacheHeaders(w, indexFile)
	http.ServeContent(w, r, indexFile, modTime, seeker)
}

func setCacheHeaders(w http.ResponseWriter, name string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// SvelteKit puts a content hash in every filename under _app/immutable, so
	// those are safe to cache forever. Everything else — index.html above all —
	// must be revalidated, or a deployed update never reaches an open tab.
	if strings.HasPrefix(name, "_app/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

// ErrNotBuilt is returned by AssertBuilt when no frontend build is embedded. It
// lets a caller fail loudly at startup instead of at first request.
var ErrNotBuilt = errors.New("internal/web: no frontend build is embedded")

// AssertBuilt reports ErrNotBuilt when the binary carries no frontend build.
func AssertBuilt() error {
	if !Built() {
		return ErrNotBuilt
	}
	return nil
}
