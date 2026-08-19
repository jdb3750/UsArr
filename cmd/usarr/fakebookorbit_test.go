package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBookOrbit is a BookOrbit-shaped test double for the cmd-level gate tests.
//
// ⚠️ IT IS A FAKE, NOT A BOOKORBIT, and internal/bookorbit's fake_test.go says
// why that distinction has to be stated rather than assumed: no live BookOrbit
// is available to this repository, so a fake proves THIS BUILD'S behaviour and
// never the server's. The routes and shapes are taken from BookOrbit's own
// source at bookorbit/bookorbit@73b7877d — auth.controller.ts,
// health.controller.ts, app-info.controller.ts, AuthService.buildUserResponse,
// @nestjs/terminus's HealthCheckResult and common/filters/http-exception.filter.ts.
//
// It is a SECOND fake rather than internal/bookorbit's, because that one lives
// in package bookorbit and these tests are package main. What it models is the
// three things that decide whether the connection test tells the truth, none of
// which the client's own tests can reach from here:
//
//  1. GET /api/v1/health is @Public(): it answers with NO credential, and it
//     will happily answer for a bad magic-link token. A test that stopped there
//     would store a broken credential and report success.
//  2. POST /api/v1/auth/magic-links/login takes the token in the BODY and 401s
//     an unknown one. It is the only thing that can prove the credential.
//  3. GET /api/v1/app-info needs a bearer and no permission at all, so it is
//     where "the token minted but the account cannot make an ordinary call"
//     becomes visible.
type fakeBookOrbit struct {
	t     *testing.T
	token string

	srv *httptest.Server

	mu     sync.Mutex
	seen   []boRequest
	issued []string

	// user is what the login response reports. The zero-value account built by
	// newFakeBookOrbit is the CORRECTLY SCOPED one: not a superuser, provisioned
	// 'shared', no permissions at all. A test that wants the §14 warning
	// mutates it before the instance is added.
	user map[string]any

	// health, when non-nil, replaces the Terminus report. Set before the
	// instance is added.
	health map[string]any
	// healthStatus is the status code served with it. 503 is what Terminus
	// answers when an indicator is down, and the BODY is the same shape.
	healthStatus int

	// appInfoStatus, when non-zero, makes app-info answer that status instead of
	// the app info — the "valid token, refused bearer" case.
	appInfoStatus int
	// appInfoMessage rides with it.
	appInfoMessage string

	// version is what a successful app-info reports.
	version string
}

type boRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   string
}

func newFakeBookOrbit(t *testing.T, token string) *fakeBookOrbit {
	t.Helper()
	f := &fakeBookOrbit{
		t: t, token: token, healthStatus: http.StatusOK, version: "v1.4.2",
		user: map[string]any{
			"id": 7, "username": "usarr", "name": "UsArr service account",
			"email": "shared@example.invalid", "active": true,
			"isSuperuser": false, "isDefaultPassword": false,
			"settings": map[string]any{"theme": "dark"}, "avatarUrl": nil,
			"provisioningMethod": "shared", "permissions": []string{},
		},
	}

	mux := http.NewServeMux()

	// @Public() at class level: registered deliberately WITHOUT any credential
	// check, because that is the property the reachability probe depends on.
	mux.HandleFunc("GET /api/v1/health", f.record(func(w http.ResponseWriter, r *http.Request) {
		body := f.health
		if body == nil {
			body = map[string]any{
				"status":  "ok",
				"info":    map[string]any{"database": map[string]any{"status": "up"}},
				"error":   map[string]any{},
				"details": map[string]any{"database": map[string]any{"status": "up"}},
			}
		}
		boWriteJSON(w, f.healthStatus, body)
	}))

	// @Public() and @Throttle({limit:10, ttl:60_000}). The token arrives in the
	// BODY and nowhere else.
	mux.HandleFunc("POST /api/v1/auth/magic-links/login", f.record(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		blob, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(blob, &req); err != nil || req.Token == "" {
			boNestError(w, r, http.StatusBadRequest, "token must be a string")
			return
		}
		if req.Token != f.token {
			// MagicLinkService.loginWithToken collapses unknown, revoked,
			// deactivated, expired and orphaned into ONE message on purpose.
			boNestError(w, r, http.StatusUnauthorized, "Invalid or expired magic link")
			return
		}
		tok := boFreshJWT(time.Now().Add(15 * time.Minute))
		f.mu.Lock()
		f.issued = append(f.issued, tok)
		user := f.user
		f.mu.Unlock()
		boWriteJSON(w, http.StatusOK, map[string]any{"accessToken": tok, "user": user})
	}))

	// Behind the global JwtAuthGuard, with no @RequirePermission of its own.
	mux.HandleFunc("GET /api/v1/app-info", f.record(func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			boNestError(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		if f.appInfoStatus != 0 {
			boNestError(w, r, f.appInfoStatus, f.appInfoMessage)
			return
		}
		boWriteJSON(w, http.StatusOK, map[string]any{
			"version": f.version, "updateAvailable": false,
			"latestVersion": f.version, "maxUploadSizeMb": 200,
		})
	}))

	mux.HandleFunc("/", f.record(func(w http.ResponseWriter, r *http.Request) {
		boNestError(w, r, http.StatusNotFound, "Cannot "+r.Method+" "+r.URL.Path)
	}))

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBookOrbit) URL() string { return f.srv.URL }

// record captures every request BEFORE handing it on, including the body. The
// body is the point: it is what lets a test prove the magic-link token travelled
// in one place and one place only.
func (f *fakeBookOrbit) record(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		blob, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		f.mu.Lock()
		f.seen = append(f.seen, boRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Auth: r.Header.Get("Authorization"), Body: string(blob),
		})
		f.mu.Unlock()
		r.Body = io.NopCloser(strings.NewReader(string(blob)))
		next(w, r)
	}
}

func (f *fakeBookOrbit) requests() []boRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]boRequest(nil), f.seen...)
}

func (f *fakeBookOrbit) issuedTokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.issued...)
}

func (f *fakeBookOrbit) bearerOK(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || got == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.issued {
		if t == got {
			return true
		}
	}
	return false
}

// boFreshJWT builds a structurally real JWT — three unpadded base64url segments
// — generated fresh on every run, for the reason internal/bookorbit's fake
// gives: a constant would be a credential-shaped literal in the repository, and
// a probe that is identical every run cannot show that a leak is THIS run's
// value.
func boFreshJWT(exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"sub": 7, "ver": 1, "iat": exp.Add(-15 * time.Minute).Unix(), "exp": exp.Unix(),
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

func boWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// boNestError is GlobalExceptionFilter's envelope.
func boNestError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	boWriteJSON(w, status, map[string]any{
		"statusCode": status, "message": msg,
		"path": r.URL.RequestURI(), "timestamp": "2026-08-19T00:00:00.000Z",
		"requestId": "req-1",
	})
}
