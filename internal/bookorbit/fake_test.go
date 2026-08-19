package bookorbit

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ⚠️ THIS IS A LOCAL FAKE, NOT A BOOKORBIT.
//
// No live BookOrbit is available to this repository and none was contacted. What
// the fake is built from is BookOrbit's own source at
// bookorbit/bookorbit@73b7877d — the routes from auth.controller.ts,
// health.controller.ts and app-info.controller.ts, the response shapes from
// AuthService.buildUserResponse, @nestjs/terminus's HealthCheckResult and
// packages/types/src/app-info.ts, and the error envelope from
// common/filters/http-exception.filter.ts.
//
// SAY IT PLAINLY, because it is the whole value of the convention
// internal/kavita's vcr_test.go established: A FAKE PROVES THIS CLIENT'S
// BEHAVIOUR, NOT THE SERVER'S. It cannot discover that a controller enforces
// something its source does not appear to, that a middleware rewrites a header,
// or that a field is always null in practice. Those are what a live probe is
// for, and ADR-0052 records that BookOrbit has had none.

// testMagicLink is the fixture magic-link token.
//
// It is the repository's shared sequential-hex fixture value, which
// .gitleaks.toml already allowlists BY VALUE. A realistic-looking token here
// would trip the generic-api-key rule, and .gitleaks.toml's own instruction for
// that case is explicit: "make it obviously fake rather than extending this
// list".
//
// Nothing about this value is BookOrbit-shaped and nothing in this package cares
// — MagicLinkLoginDto accepts any string of 1..512 characters.
const testMagicLink = "0123456789abcdef0123456789abcdef"

const testBaseURL = "http://bookorbit.test:3000"

// freshJWT builds a structurally real JWT — three unpadded base64url segments —
// with the given expiry, generated fresh on every run.
//
// Fresh-per-run for the two reasons internal/vcrscrub's fixtureJWT is: a
// constant would be a credential-shaped literal in the repository, and a probe
// that is the same every run cannot show that a leak is THIS run's value.
// Unpadded is load-bearing: a `=` inside the value would let the scrubber's
// queryPairRe see a name=value pair inside the token and rewrite it for the
// wrong reason.
func freshJWT(exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"sub": 7,
		"ver": 1,
		"iat": exp.Add(-15 * time.Minute).Unix(),
		"exp": exp.Unix(),
	})
	if err != nil {
		panic("encoding fixture JWT claims: " + err.Error())
	}
	sig := make([]byte, 32)
	if _, err := rand.Read(sig); err != nil {
		panic("no entropy for the fixture JWT: " + err.Error())
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

// seenRequest is one request the fake answered.
type seenRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   string
}

// fake is a local stand-in for one BookOrbit.
type fake struct {
	t   *testing.T
	srv *httptest.Server

	mu     sync.Mutex
	seen   []seenRequest
	mints  int
	issued []string

	// Knobs. Each is consulted before the default behaviour; a nil handler means
	// "behave like BookOrbit".
	healthHandler  http.HandlerFunc
	loginHandler   http.HandlerFunc
	appInfoHandler http.HandlerFunc

	// user is what the login response reports. Zero value is the correctly
	// scoped shared account: not a superuser, provisioned 'shared', no
	// permissions.
	user userDTO

	// tokenTTL is how long each issued access token claims to live.
	tokenTTL time.Duration

	// acceptOnlyLatest makes app-info 401 any bearer that is not the most
	// recently issued token — which is what an expired JWT looks like from the
	// client's side.
	acceptOnlyLatest bool

	// ── the catalogue routes (slice 1) ──────────────────────────────────────

	// libraries is what GET /api/v1/libraries answers. The shape is
	// LibraryRepository.findAllForUser's projection — the NON-superuser one,
	// which is what UsArr's shared viewer account actually receives.
	libraries []map[string]any

	// books are the BookCards, keyed by library id.
	books map[int64][]map[string]any

	// booksHandler replaces the paging behaviour wholesale.
	booksHandler http.HandlerFunc

	// covers is what GET /api/v1/books/{id}/cover answers, keyed by book id. A
	// book that is NOT in the map answers 404 with BookController.getCover's own
	// NotFoundException message, which is what a book with no cover file does
	// upstream (book.controller.ts:344) — so "absent from the map" and "has no
	// cover" are the same thing here on purpose.
	covers map[int64]fakeCover

	// coverHandler replaces the cover route wholesale.
	coverHandler http.HandlerFunc
}

// fakeCover is one stored cover: the bytes and the Content-Type BookOrbit would
// derive from the file's extension (imageContentTypeFromPath).
type fakeCover struct {
	contentType string
	body        []byte
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{
		t:        t,
		tokenTTL: 15 * time.Minute,
		user: userDTO{
			ID: 7, Username: "usarr", Name: "UsArr service account",
			Active: true, ProvisioningMethod: provisioningShared,
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc(apiPrefix+"/health", f.record(func(w http.ResponseWriter, r *http.Request) {
		if f.healthHandler != nil {
			f.healthHandler(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"info":    map[string]any{"database": map[string]any{"status": "up"}},
			"error":   map[string]any{},
			"details": map[string]any{"database": map[string]any{"status": "up"}},
		})
	}))
	mux.HandleFunc(magicLinkLoginPath, f.record(func(w http.ResponseWriter, r *http.Request) {
		if f.loginHandler != nil {
			f.loginHandler(w, r)
			return
		}
		f.defaultLogin(w, r)
	}))
	mux.HandleFunc(apiPrefix+"/app-info", f.record(func(w http.ResponseWriter, r *http.Request) {
		if f.appInfoHandler != nil {
			f.appInfoHandler(w, r)
			return
		}
		f.defaultAppInfo(w, r)
	}))
	mux.HandleFunc("GET "+apiPrefix+"/libraries", f.record(func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			nestErrorText(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		out := f.libraries
		if out == nil {
			out = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, out)
	}))

	mux.HandleFunc("POST "+apiPrefix+"/libraries/{id}/books", f.record(func(w http.ResponseWriter, r *http.Request) {
		if f.booksHandler != nil {
			f.booksHandler(w, r)
			return
		}
		f.defaultBooks(w, r)
	}))

	mux.HandleFunc("GET "+apiPrefix+"/books/{id}/cover", f.record(func(w http.ResponseWriter, r *http.Request) {
		if f.coverHandler != nil {
			f.coverHandler(w, r)
			return
		}
		f.defaultCover(w, r)
	}))

	mux.HandleFunc("/", f.record(func(w http.ResponseWriter, r *http.Request) {
		nestError404(w, r)
	}))

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// record captures every request before handing it on. It reads the BODY, which
// is what lets a test assert that the magic-link token travelled in one and only
// one place.
func (f *fake) record(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		f.mu.Lock()
		f.seen = append(f.seen, seenRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Auth: r.Header.Get(authHeader), Body: string(body),
		})
		f.mu.Unlock()
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		next(w, r)
	}
}

func (f *fake) defaultLogin(w http.ResponseWriter, r *http.Request) {
	var req magicLinkLoginRequest
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil || req.Token == "" {
		nestErrorJSON(w, r, http.StatusBadRequest, []string{"token must be a string", "token must be longer than or equal to 1 characters"})
		return
	}
	if req.Token != testMagicLink {
		nestErrorText(w, r, http.StatusUnauthorized, "Invalid or expired magic link")
		return
	}
	tok := freshJWT(time.Now().Add(f.tokenTTL))
	f.mu.Lock()
	f.mints++
	f.issued = append(f.issued, tok)
	user := f.user
	f.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"accessToken": tok, "user": user})
}

func (f *fake) defaultAppInfo(w http.ResponseWriter, r *http.Request) {
	if !f.bearerOK(r) {
		nestErrorText(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": "v1.4.2", "updateAvailable": false, "latestVersion": "v1.4.2", "maxUploadSizeMb": 200,
	})
}

// defaultBooks is POST /api/v1/libraries/{id}/books, paging honestly.
//
// ⚠️ IT SERVES EXACTLY THE REQUESTED SIZE UNTIL THE LAST PAGE, because that is
// what a real BookOrbit does and because the walk's stop condition is a SHORT
// page. A fake that shrank pages of its own accord would let a broken stop
// condition pass here and then spin against a real instance.
func (f *fake) defaultBooks(w http.ResponseWriter, r *http.Request) {
	if !f.bearerOK(r) {
		nestErrorText(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}
	libID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		nestErrorText(w, r, http.StatusBadRequest, "Validation failed (numeric string is expected)")
		return
	}
	var q bookQueryRequest
	blob, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(blob, &q); err != nil {
		nestErrorText(w, r, http.StatusBadRequest, "Unexpected token in JSON")
		return
	}
	// BookQueryPipe's zod bounds, enforced rather than assumed.
	if q.Pagination.Size < 1 || q.Pagination.Size > 200 {
		nestErrorJSON(w, r, http.StatusBadRequest,
			[]string{"pagination.size: Number must be less than or equal to 200"})
		return
	}
	if q.Pagination.Page < 0 {
		nestErrorJSON(w, r, http.StatusBadRequest,
			[]string{"pagination.page: Number must be greater than or equal to 0"})
		return
	}

	all := f.books[libID]
	start := min(q.Pagination.Page*q.Pagination.Size, len(all))
	end := min(start+q.Pagination.Size, len(all))
	items := all[start:end]
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(all), "page": q.Pagination.Page, "size": q.Pagination.Size,
	})
}

// defaultCover is GET /api/v1/books/{id}/cover.
//
// The 404 path is transcribed rather than invented: getCoverPath returns null
// for a book whose cover directory holds no usable file, and the controller
// throws NotFoundException(`No cover for book ${id}`) on that null. THERE IS NO
// PLACEHOLDER ARM UPSTREAM AND THERE IS NONE HERE — a fake that answered 200
// with a grey rectangle would let a caller that cannot tell a placeholder from a
// cover pass.
func (f *fake) defaultCover(w http.ResponseWriter, r *http.Request) {
	if !f.bearerOK(r) {
		nestErrorText(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		nestErrorText(w, r, http.StatusBadRequest, "Validation failed (numeric string is expected)")
		return
	}
	cov, ok := f.covers[id]
	if !ok {
		nestErrorText(w, r, http.StatusNotFound, fmt.Sprintf("No cover for book %d", id))
		return
	}
	w.Header().Set("Content-Type", cov.contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", `"1755500000000"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cov.body)
}

// fakeCard builds one BookCard. Field names are packages/types/src/book.ts's.
func fakeCard(id int64, title string, extra map[string]any) map[string]any {
	c := map[string]any{
		"id": id, "status": "present", "coverAspectRatio": "book",
		"title": title, "subtitle": nil,
		"authors": []string{}, "narrators": []string{},
		"seriesId": nil, "seriesName": nil, "seriesIndex": nil, "seriesMemberships": []any{},
		"files": []map[string]any{
			{"id": id*10 + 1, "format": "epub", "role": "primary", "sizeBytes": 987654},
		},
		"publishedDate": nil, "publishedYear": nil, "language": nil,
		"genres": []string{}, "tags": []string{},
		"rating": nil, "readingProgress": nil, "readStatus": nil,
		"addedAt": "2026-08-01T10:00:00.000Z", "updatedAt": "2026-08-17T07:00:30.118Z",
		"metadataScore": nil, "hasCover": true,
		"hasMetadataLocks": false, "lockedFields": []string{},
		"publisher": nil, "pageCount": nil,
		"isbn13": nil, "hardcoverId": nil, "hardcoverEditionId": nil,
		"customMetadata": []any{},
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

func (f *fake) bearerOK(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get(authHeader), authScheme)
	if !ok || got == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.issued) == 0 {
		return false
	}
	if f.acceptOnlyLatest {
		return got == f.issued[len(f.issued)-1]
	}
	for _, t := range f.issued {
		if t == got {
			return true
		}
	}
	return false
}

func (f *fake) requests() []seenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seenRequest(nil), f.seen...)
}

func (f *fake) mintCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mints
}

func (f *fake) latestToken() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.issued) == 0 {
		return ""
	}
	return f.issued[len(f.issued)-1]
}

// ─── The exact wire shapes BookOrbit produces ────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// nestErrorText is GlobalExceptionFilter's envelope with a STRING message — what
// a hand-thrown HttpException produces.
func nestErrorText(w http.ResponseWriter, r *http.Request, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"statusCode": status,
		"message":    msg,
		"path":       r.URL.RequestURI(),
		"timestamp":  "2026-08-19T00:00:00.000Z",
		"requestId":  "req-1",
	})
}

// nestErrorJSON is the same envelope with an ARRAY message — what the global
// ValidationPipe produces, and the shape a client that assumes `message` is a
// string decodes wrongly.
func nestErrorJSON(w http.ResponseWriter, r *http.Request, status int, msgs []string) {
	writeJSON(w, status, map[string]any{
		"statusCode": status,
		"message":    msgs,
		"path":       r.URL.RequestURI(),
		"timestamp":  "2026-08-19T00:00:00.000Z",
		"requestId":  "req-1",
	})
}

func nestError404(w http.ResponseWriter, r *http.Request) {
	nestErrorText(w, r, http.StatusNotFound, fmt.Sprintf("Cannot %s %s", r.Method, r.URL.Path))
}

// newTestClient builds a Client pointed at the fake.
func (f *fake) client(t *testing.T, mutate ...func(*Options)) *Client {
	t.Helper()
	opts := Options{
		BaseURL:        f.srv.URL,
		MagicLinkToken: testMagicLink,
		HTTPClient:     f.srv.Client(),
		AppVersion:     "0.0.0-test",
	}
	for _, m := range mutate {
		m(&opts)
	}
	c, err := New(opts)
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

// assertNoCredential fails if s contains either secret.
func assertNoCredential(t *testing.T, what, s string, extra ...string) {
	t.Helper()
	for _, secret := range append([]string{testMagicLink}, extra...) {
		if secret == "" {
			continue
		}
		if strings.Contains(s, secret) {
			t.Fatalf("%s leaked a credential: %q", what, s)
		}
	}
}
