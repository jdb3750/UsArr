package bookorbit

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
