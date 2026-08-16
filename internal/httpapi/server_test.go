package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/db"
	"github.com/jdb3750/UsArr/internal/store"
)

func newTestServer(t *testing.T, mutate func(*Config)) *Server {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(ctx, filepath.Join(t.TempDir(), "usarr.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	kek := make([]byte, 32)
	if _, err := rand.Read(kek); err != nil {
		t.Fatalf("kek: %v", err)
	}
	keyring, err := crypto.NewKeyring(1, kek)
	if err != nil {
		t.Fatalf("keyring: %v", err)
	}

	cfg := Config{
		Store:         store.New(database),
		Keyring:       keyring,
		GrabRowIDKey:  testGrabRowIDKey(t),
		AuditRowIDKey: testAuditRowIDKey(t),
		SchemaVersion: 1,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// Readiness is exactly two things. This nails down the half that a well-meaning
// change would otherwise extend, because "ready should mean the library has
// synced" sounds reasonable right up until Docker's start-period=0s /retries=3
// defaults turn a first run into a restart loop that looks like a crash.
func TestReadyIsMigrationsPlusListenerAndNothingElse(t *testing.T) {
	s := newTestServer(t, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// live answers before the listener is even advertised: it is the process
	// liveness probe and must not fail for any other reason.
	if code, body := get(t, srv.URL+"/api/health/live"); code != http.StatusOK {
		t.Fatalf("live before MarkListening = %d: %s", code, body)
	}

	code, body := get(t, srv.URL+"/api/health/ready")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("ready before MarkListening = %d, want 503: %s", code, body)
	}
	var ready readyResponse
	mustJSON(t, body, &ready)
	if !ready.MigrationsApplied || ready.ListenerAccepting {
		t.Fatalf("unexpected readiness detail: %+v", ready)
	}

	s.MarkListening()
	code, body = get(t, srv.URL+"/api/health/ready")
	if code != http.StatusOK {
		t.Fatalf("ready after MarkListening = %d: %s", code, body)
	}
	mustJSON(t, body, &ready)
	if ready.SchemaVersion != 1 || !ready.ListenerAccepting {
		t.Fatalf("unexpected readiness detail: %+v", ready)
	}

	// Draining flips it back off so anything in front stops sending work while
	// in-flight requests still complete.
	s.Draining()
	if code, _ := get(t, srv.URL+"/api/health/ready"); code != http.StatusServiceUnavailable {
		t.Fatalf("ready while draining = %d, want 503", code)
	}
}

// USARR_URL_BASE has to work on EVERY route, not only the SPA's asset paths: a
// reverse proxy mounted at /usarr rewrites nothing.
func TestURLBaseAppliesToEveryRoute(t *testing.T) {
	s := newTestServer(t, func(c *Config) { c.URLBase = "/usarr" })
	s.MarkListening()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	if code, _ := get(t, srv.URL+"/usarr/api/health/live"); code != http.StatusOK {
		t.Errorf("/usarr/api/health/live = %d, want 200", code)
	}
	if code, _ := get(t, srv.URL+"/api/health/live"); code == http.StatusOK {
		t.Error("/api/health/live must not answer when a url base is configured")
	}

	// A bare /usarr is the link people paste.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/usarr", nil)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /usarr: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/usarr/" {
		t.Errorf("GET /usarr = %d Location=%q, want 301 -> /usarr/", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestURLBaseIsValidated(t *testing.T) {
	for _, bad := range []string{"usarr", "/usarr/"} {
		if _, err := New(Config{
			Store:         newTestServer(t, nil).store,
			Keyring:       newTestServer(t, nil).keyring,
			GrabRowIDKey:  testGrabRowIDKey(t),
			AuditRowIDKey: testAuditRowIDKey(t),
			URLBase:       bad,
		}); err == nil {
			t.Errorf("URLBase %q was accepted", bad)
		}
	}
}

// The two row-id keys must be DIFFERENT BYTES, and this is the only thing that
// checks it, because nothing in the repo ever constructs a Config where they are
// equal — main wires two distinct derivations and every test helper does the
// same, so the guard in New could be deleted with the whole gate staying green.
//
// ⚠️ IT IS NOT THE HKDF-LABEL CHECK WEARING A DIFFERENT HAT. A copy-pasted
// label is caught in internal/crypto, where the two derivations are asserted to
// disagree. What only this test can see is the other mistake: two CORRECT keys,
// each 32 bytes and properly derived, wired into both fields at the call site.
// That compiles, runs, and silently collapses the provenance and audit id
// domains into one keyspace — and one response carries both arms, so the client
// would key two unrelated rows on one id (review finding RG-01.3).
//
// The second case is not padding. A regression that rejected every Config would
// satisfy the first case happily, and a guard that says no to everything is not
// this guard.
func TestRowIDKeysMustBeDistinctBytes(t *testing.T) {
	base := newTestServer(t, nil)
	cfg := func(grab, audit []byte) Config {
		return Config{
			Store:         base.store,
			Keyring:       base.keyring,
			GrabRowIDKey:  grab,
			AuditRowIDKey: audit,
		}
	}

	// Cloned rather than the same slice header, so what is under test is
	// bytes.Equal and not slice identity.
	key := testGrabRowIDKey(t)
	if s, err := New(cfg(key, bytes.Clone(key))); err == nil {
		t.Errorf("New accepted GrabRowIDKey == AuditRowIDKey and returned %v", s != nil)
	}

	if _, err := New(cfg(testGrabRowIDKey(t), testAuditRowIDKey(t))); err != nil {
		t.Errorf("New rejected correct, distinct keys: %v", err)
	}
}

// A forwarded header is believed ONLY from inside a configured CIDR. Empty
// trusts nothing, which is the default: GHSA-qcmf-gmhm-rfv9 is exactly the bug
// where a spoofable client IP made an attacker look local.
func TestForwardedHeadersAreOnlyBelievedFromTrustedProxies(t *testing.T) {
	capture := func(trusted []netip.Prefix) string {
		s := newTestServer(t, func(c *Config) { c.TrustedProxies = trusted })
		var got string
		h := s.clientIPMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = ClientIP(r.Context())
			if r.Header.Get("X-Forwarded-For") != "" {
				t.Error("X-Forwarded-For reached the handler; it must be stripped")
			}
		}))
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/health/live", nil)
		r.RemoteAddr = "192.0.2.10:5000"
		r.Header.Set("X-Forwarded-For", "203.0.113.99, 192.0.2.10")
		h.ServeHTTP(httptest.NewRecorder(), r)
		return got
	}

	if got := capture(nil); got != "192.0.2.10" {
		t.Errorf("with nothing trusted, client ip = %q, want the peer address", got)
	}
	if got := capture([]netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}); got != "203.0.113.99" {
		t.Errorf("from a trusted proxy, client ip = %q, want the forwarded address", got)
	}
	// A trusted range that does not contain the peer must not be believed.
	if got := capture([]netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}); got != "192.0.2.10" {
		t.Errorf("from an untrusted peer, client ip = %q, want the peer address", got)
	}
}

func TestUnknownAPIPathIs404JSONNotTheSPA(t *testing.T) {
	s := newTestServer(t, nil)
	s.MarkListening()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	code, body := get(t, srv.URL+"/api/v1/nope")
	if code != http.StatusNotFound {
		t.Fatalf("unknown /api path = %d, want 404", code)
	}
	if !strings.Contains(body, `"error":"not_found"`) {
		t.Fatalf("unknown /api path must answer as JSON, got: %s", body)
	}
}

func TestStateChangingRoutesRequireAuth(t *testing.T) {
	s := newTestServer(t, nil)
	s.MarkListening()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range []string{
		"/api/v1/services", "/api/v1/services/health", "/api/v1/search?query=x",
		"/api/events", "/api/v1/system/status",
	} {
		if code, _ := get(t, srv.URL+path); code != http.StatusUnauthorized {
			t.Errorf("GET %s unauthenticated = %d, want 401", path, code)
		}
	}
}

// The cookie path and any bearer path stay strictly separate, so a browser
// cannot authenticate an endpoint with an ambient credential.
func TestBearerCredentialsAreRefusedOnTheCookiePath(t *testing.T) {
	s := newTestServer(t, nil)
	s.MarkListening()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, header := range []string{"Authorization", "X-Api-Key"} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/v1/services", nil)
		req.Header.Set(header, "some-credential")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", header, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), "session cookie only") {
			t.Errorf("%s = %d: %s", header, resp.StatusCode, body)
		}
	}
}

// ── the Services screen's state machine ─────────────────────────────────────

func TestHealthStateAndAction(t *testing.T) {
	past := time.Now().Add(-time.Minute)

	cases := []struct {
		name       string
		si         store.ServiceInstance
		row        serviceHealthResponse
		wantState  string
		wantAction string
	}{
		{
			name:       "never probed",
			si:         store.ServiceInstance{Enabled: true, HealthState: "unknown"},
			row:        serviceHealthResponse{BreakerState: "closed"},
			wantState:  stateUnknown,
			wantAction: "Test connection",
		},
		{
			name:       "healthy",
			si:         store.ServiceInstance{Enabled: true, HealthState: "healthy"},
			row:        serviceHealthResponse{BreakerState: "closed", LastOKAt: &past},
			wantState:  stateHealthy,
			wantAction: "",
		},
		{
			name:       "breaker open is down",
			si:         store.ServiceInstance{Enabled: true, HealthState: "down"},
			row:        serviceHealthResponse{BreakerState: "open", Problem: "dial tcp: connection refused"},
			wantState:  stateDown,
			wantAction: "Test connection",
		},
		{
			name:       "a 401 names the credential fix, not a retry",
			si:         store.ServiceInstance{Enabled: true, HealthState: "down"},
			row:        serviceHealthResponse{BreakerState: "closed", Problem: "servarr: unauthorized (bad or missing API key)"},
			wantState:  stateDown,
			wantAction: "Update API key",
		},
		{
			name:       "a changed pin names the fingerprint review",
			si:         store.ServiceInstance{Enabled: true, HealthState: "degraded"},
			row:        serviceHealthResponse{BreakerState: "closed", Problem: "tls: certificate pin mismatch"},
			wantState:  stateDegraded,
			wantAction: "Review the TLS fingerprint",
		},
		{
			name:       "needs re-identification is loud and specific",
			si:         store.ServiceInstance{Enabled: true, HealthState: "needs_reidentification"},
			row:        serviceHealthResponse{BreakerState: "closed"},
			wantState:  stateReID,
			wantAction: "Re-link this instance",
		},
		{
			name:       "disabled",
			si:         store.ServiceInstance{Enabled: false, HealthState: "healthy"},
			row:        serviceHealthResponse{BreakerState: "closed"},
			wantState:  stateUnknown,
			wantAction: "Enable this service",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := tc.row
			row.Enabled = tc.si.Enabled
			row.State = healthState(tc.si, row)
			if row.State != tc.wantState {
				t.Fatalf("state = %q, want %q", row.State, tc.wantState)
			}
			if got := healthAction(row); got != tc.wantAction {
				t.Errorf("action = %q, want %q", got, tc.wantAction)
			}
		})
	}
}

// The Problem column carries the ACTUAL error text (§17.3 is explicit that "an
// error occurred" is not acceptable) — minus any credential in it.
func TestHealthRowKeepsTheErrorVerbatimAndDropsTheKey(t *testing.T) {
	s := newTestServer(t, nil)
	row := s.healthRow(store.ServiceInstance{
		ID: 1, Name: "Prowlarr", Enabled: true, BaseURL: "http://prowlarr:9696",
		HealthState: "down", BreakerState: "open",
		LastError: sql.NullString{Valid: true, String: `Get "http://prowlarr:9696/api/v1/indexer?apikey=REALKEY": connection refused`},
	})
	if strings.Contains(row.Problem, "REALKEY") {
		t.Fatalf("the Problem column leaked the key: %s", row.Problem)
	}
	if !strings.Contains(row.Problem, "connection refused") {
		t.Fatalf("the Problem column lost the actual error: %s", row.Problem)
	}
	if row.State != stateDown || row.Action != "Test connection" {
		t.Errorf("state=%q action=%q", row.State, row.Action)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

func mustJSON(t *testing.T, blob string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(blob), out); err != nil {
		t.Fatalf("decode %q: %v", blob, err)
	}
}
