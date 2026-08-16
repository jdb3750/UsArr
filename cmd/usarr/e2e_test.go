package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/httpapi"
)

// TestEndToEndSearchAndGrab drives the ENTIRE Search-and-Grab path through
// UsArr's own HTTP API against a Prowlarr-shaped double:
//
//	setup → login → add the service (with its mandatory connection test)
//	→ test the connection again → open the SSE stream → start a search
//	→ read results off the stream → grab one → read the Services screen.
//
// It is one test rather than several because the interesting failures are in the
// seams: a search id that never reaches the stream, a candidate id that does not
// survive the round trip, a credential that leaks on the way out.
func TestEndToEndSearchAndGrab(t *testing.T) {
	// Deliberately unlike any other hex in the fixture: the infoHash is
	// 0123456789ab…, and a key that shares a prefix with it would make the
	// leak assertions below fire on the hash and pass for the wrong reason.
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"

	prowlarr := newFakeProwlarr(t, apiKey)
	env := newTestApp(t)

	// ── 1. first run: no owner account exists ───────────────────────────────
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	if !sess.SetupRequired {
		t.Fatal("a fresh install must report setup_required")
	}
	if sess.CSRFToken == "" {
		t.Fatal("the bootstrap call must hand out a CSRF token")
	}

	// CSRF is enforced, not decorative.
	if code, body := env.raw(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, withoutCSRF); code != http.StatusForbidden {
		t.Fatalf("POST without a CSRF header must be 403, got %d: %s", code, body)
	}
	// So is the JSON content type.
	if code, body := env.raw(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, asFormPost); code != http.StatusUnsupportedMediaType {
		t.Fatalf("POST as a form must be 415, got %d: %s", code, body)
	}

	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)
	if !sess.Authenticated {
		t.Fatal("setup must sign the owner in")
	}
	t.Logf("owner created: user_id=%d sudo_until=%v", sess.UserID, sess.SudoUntil)

	// ── 2. add the Prowlarr instance ────────────────────────────────────────
	//
	// The connection test is mandatory and runs server-side; a bad key must not
	// be storable.
	if code, body := env.raw(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Bad Key", "base_url": prowlarr.URL(), "api_key": "wrong-key",
	}); code != http.StatusBadGateway {
		t.Fatalf("a service with a bad key must be refused with 502, got %d: %s", code, body)
	}

	var svc serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prowlarr.URL(), "api_key": apiKey,
	}, &svc)
	if svc.ID == 0 || svc.Role != "indexer" {
		t.Fatalf("unexpected service row: %+v", svc)
	}
	t.Logf("service created: id=%d role=%s has_credential=%v api_version=%s",
		svc.ID, svc.Role, svc.HasCredential, svc.APIVersion)

	// ── 3. the connection test endpoint ─────────────────────────────────────
	var test testBody
	env.do(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", svc.ID), map[string]any{}, &test)
	if !test.OK || !test.Reachable {
		t.Fatalf("connection test failed: %+v", test)
	}
	t.Logf("connection test: ok=%v key_proven_valid=%v %s", test.OK, test.KeyProvenValid, test.Message)

	// Changing the host without re-entering the key must be refused
	// server-side (reference/security.md §1.6).
	code, body := env.raw(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", svc.ID),
		map[string]any{"base_url": "http://attacker.example:9696"})
	if code != http.StatusBadRequest || !strings.Contains(body, "credential_reentry_required") {
		t.Fatalf("a test against a changed host must demand re-entry, got %d: %s", code, body)
	}
	t.Logf("changed-host test refused as required: %s", strings.TrimSpace(body))

	// ── 4. the SSE stream, opened BEFORE the search ─────────────────────────
	stream := env.openStream(t)
	defer stream.close()

	// ── 5. start a search ───────────────────────────────────────────────────
	prowlarr.blockIndexer(2) // so the report has something honest to say

	var accepted searchAcceptedBody
	env.do(t, "GET", "/api/v1/search?query=Test+Release&type=search&category=2000", nil, &accepted)
	if accepted.SearchID == "" {
		t.Fatal("the search must return a search id immediately")
	}
	t.Logf("search accepted: id=%s instance=%d events_url=%s",
		accepted.SearchID, accepted.InstanceID, accepted.EventsURL)

	// ── 6. read the stream ──────────────────────────────────────────────────
	var (
		results  []releaseBody
		report   reportBody
		outcomes []string
	)
	deadline := time.After(30 * time.Second)
	for report.Summary == "" {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for search.done; got %d results so far", len(results))
		case ev := <-stream.events:
			t.Logf("SSE id=%s event=%s %s", ev.id, ev.name, truncateForLog(ev.data))
			switch ev.name {
			case httpapi.EventSearchResults:
				var payload struct {
					Results []releaseBody `json:"results"`
				}
				mustUnmarshal(t, ev.data, &payload)
				results = append(results, payload.Results...)
			case httpapi.EventSearchIndexer:
				var payload struct {
					Phase   string `json:"phase"`
					Indexer struct {
						Name   string `json:"name"`
						Status string `json:"status"`
						Reason string `json:"reason"`
					} `json:"indexer"`
				}
				mustUnmarshal(t, ev.data, &payload)
				if payload.Phase == "indexer_done" {
					outcomes = append(outcomes,
						fmt.Sprintf("%s=%s (%s)", payload.Indexer.Name, payload.Indexer.Status, payload.Indexer.Reason))
				}
			case httpapi.EventSearchDone:
				var payload struct {
					Report reportBody `json:"report"`
				}
				mustUnmarshal(t, ev.data, &payload)
				report = payload.Report
			}
		}
	}

	t.Logf("indexer outcomes: %s", strings.Join(outcomes, " | "))
	t.Logf("report: %s (degraded=%v answered=%d failed=%d skipped=%d results=%d)",
		report.Summary, report.Degraded, report.Answered, report.Failed, report.Skipped, report.Results)

	if len(results) == 0 {
		t.Fatal("no results arrived on the stream")
	}
	if !report.Degraded {
		t.Error("with one blocked and one disabled indexer the report must be degraded: " +
			"an empty 200 from Prowlarr is indistinguishable from no-results without it")
	}
	if counts := prowlarr.searchCounts(); len(counts) != 1 || counts["1"] != 1 {
		t.Errorf("expected one search against indexer 1 only (blocked and disabled indexers "+
			"must be skipped, not re-timed-out on), got %v", counts)
	}

	// ── 7. no credential may have crossed the boundary ──────────────────────
	assertNoSecret(t, "SSE stream", stream.dump(), apiKey, "indexer-passkey-should-not-leak")
	for _, res := range results {
		// The indexer's infoUrl carried ?apikey=<the real key>; the deny-list
		// must have replaced the value, and the link must still work as a link.
		if !strings.Contains(res.InfoURL, "apikey=REDACTED") {
			t.Errorf("info_url was not redacted: %s", res.InfoURL)
		}
	}
	t.Logf("info_url as served: %s", results[0].InfoURL)

	// ── 8. grab one ─────────────────────────────────────────────────────────
	candidate := results[0]
	t.Logf("grabbing candidate_id=%d %q (expires %s)", candidate.CandidateID, candidate.Title, candidate.ExpiresAt)

	var grab grabBody
	env.do(t, "POST", fmt.Sprintf("/api/v1/releases/%d/grab", candidate.CandidateID), map[string]any{}, &grab)
	if grab.CandidateID != candidate.CandidateID {
		t.Fatalf("grab returned candidate %d, wanted %d", grab.CandidateID, candidate.CandidateID)
	}
	t.Logf("grab: title=%q protocol=%s indexer=%s provenance_id=%d re_searched=%v — %s",
		grab.ReleaseTitle, grab.Protocol, grab.IndexerName, grab.ProvenanceID, grab.ReSearched, grab.Message)

	grabs := prowlarr.grabs()
	if len(grabs) != 1 {
		t.Fatalf("expected exactly one grab upstream, got %d", len(grabs))
	}
	t.Logf("upstream received grab body: %v", grabs[0])
	if grabs[0]["guid"] != candidate.GUID {
		t.Errorf("grab body guid = %v, wanted %s", grabs[0]["guid"], candidate.GUID)
	}
	// GrabBody sends the minimum: sending the whole resource back would echo
	// the embedded admin key over the wire for nothing — and, as the live 400
	// proved, every extra field is one the far end's model binder gets a vote on.
	// Prowlarr reads guid, indexerId and downloadClientId; nothing else may appear.
	allowed := map[string]bool{"guid": true, "indexerId": true, "downloadClientId": true}
	for k, v := range grabs[0] {
		if !allowed[k] {
			t.Errorf("the grab body sends %q=%#v; Prowlarr reads only guid, indexerId and "+
				"downloadClientId, and a zero-valued extra field is what broke this path", k, v)
		}
	}

	// ── 9. the Services screen ──────────────────────────────────────────────
	//
	// The screen renders from the LAST PROBE, never from an upstream call, so
	// the test has to make a probe happen and then wait for it — which is the
	// property under test, not an inconvenience. A successful connection test
	// asks for one out of band.
	env.do(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", svc.ID), map[string]any{}, &test)

	var health servicesHealthBody
	var row serviceHealthBody
	for range 100 {
		health = servicesHealthBody{}
		env.do(t, "GET", "/api/v1/services/health", nil, &health)
		if len(health.Services) != 1 {
			t.Fatalf("expected one service row, got %d", len(health.Services))
		}
		row = health.Services[0]
		if !row.Stale && len(row.BlockedIndexers) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("services screen: %s state=%s stale=%v app_version=%s warnings=%d blocked=%d problem=%q action=%q",
		row.Name, row.State, row.Stale, row.AppVersion, len(row.Warnings), len(row.BlockedIndexers), row.Problem, row.Action)
	if row.Stale {
		t.Error("the row is stale: the background prober never ran")
	}
	if len(row.Warnings) == 0 {
		t.Error("Prowlarr's own /api/v1/health warnings must be surfaced on this screen")
	}
	if len(row.BlockedIndexers) == 0 {
		t.Error("blocked indexers from /api/v1/indexerstatus must be surfaced on this screen")
	}

	// ── 10. nothing anywhere in the logs ────────────────────────────────────
	assertNoSecret(t, "process log", env.logs(), apiKey, "indexer-passkey-should-not-leak")

	// The API key never appears in any service representation either.
	var listed struct {
		Services []serviceBody `json:"services"`
	}
	env.do(t, "GET", "/api/v1/services", nil, &listed)
	blob, _ := json.Marshal(listed)
	assertNoSecret(t, "GET /api/v1/services", string(blob), apiKey)
	t.Logf("GET /api/v1/services: %s", blob)
}

// TestSearchRequiresIndexerService proves the honest-degradation path: with
// nothing configured, search says what is missing and names the fix rather than
// returning an empty result set.
func TestSearchRequiresIndexerService(t *testing.T) {
	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	code, body := env.raw(t, "GET", "/api/v1/search?query=anything", nil)
	if code != http.StatusConflict || !strings.Contains(body, "no_indexer_service") {
		t.Fatalf("search with no indexer must be a 409 that says so, got %d: %s", code, body)
	}
	t.Logf("no-indexer search: %d %s", code, strings.TrimSpace(body))

	var health servicesHealthBody
	env.do(t, "GET", "/api/v1/services/health", nil, &health)
	if !health.SetupRequired {
		t.Error("the Services screen must report setup_required rather than an empty table")
	}
}

// TestUnauthenticatedAndURLBase covers the two routing properties that are easy
// to get wrong and invisible until a deployment breaks: authentication on every
// /api/v1 route, and USARR_URL_BASE applying to all of them.
func TestUnauthenticatedAndURLBase(t *testing.T) {
	env := newTestAppWith(t, "/usarr")

	for _, path := range []string{
		"/usarr/api/v1/services",
		"/usarr/api/v1/services/health",
		"/usarr/api/v1/search?query=x",
		"/usarr/api/events",
		"/usarr/api/v1/system/status",
	} {
		if code, _ := env.raw(t, "GET", path, nil, withoutSession); code != http.StatusUnauthorized {
			t.Errorf("%s unauthenticated = %d, want 401", path, code)
		}
	}
	// Health is deliberately open: it is the container's healthcheck and must
	// answer before there is a session.
	for _, path := range []string{"/usarr/api/health/live", "/usarr/api/health/ready"} {
		if code, _ := env.raw(t, "GET", path, nil, withoutSession); code != http.StatusOK {
			t.Errorf("%s = %d, want 200", path, code)
		}
	}
	// And the same routes are NOT served off the base.
	if code, _ := env.raw(t, "GET", "/api/health/live", nil, withoutSession); code == http.StatusOK {
		t.Error("with USARR_URL_BASE set, /api/health/live must not answer off the base")
	}
	// The SPA still resolves deep routes under the base.
	if code, _ := env.raw(t, "GET", "/usarr/search", nil, withoutSession); code != http.StatusOK {
		t.Errorf("/usarr/search = %d, want the SPA document", code)
	}
	t.Log("every route honours USARR_URL_BASE=/usarr")
}

// ── harness ─────────────────────────────────────────────────────────────────

type testEnv struct {
	app     *app
	srv     *httptest.Server
	cookies map[string]string
	csrf    string
	logBuf  *syncBuffer
	base    string
}

func newTestApp(t *testing.T) *testEnv { return newTestAppWith(t, "") }

func newTestAppWith(t *testing.T, urlBase string) *testEnv {
	t.Helper()
	dir := t.TempDir()

	args := []string{"--config-dir", dir}
	if urlBase != "" {
		args = append(args, "--url-base", urlBase)
	}
	cfg, err := config.Load(config.Options{Args: args, Env: map[string]string{}, Version: "0.0.0-test"})
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	buf := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := context.Background()
	a, err := buildApp(ctx, cfg, log, httpapi.BuildInfo{Version: "0.0.0-test", Commit: "test", GoVersion: goVersion()})
	if err != nil {
		t.Fatalf("buildApp: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	// The prober is a background goroutine in production too; the test runs it
	// so the Services screen has real snapshots rather than a stub.
	proberCtx, stopProber := context.WithCancel(context.Background())
	t.Cleanup(stopProber)
	go a.registry.RunProber(proberCtx)

	srv := httptest.NewServer(a.server.Handler())
	t.Cleanup(srv.Close)
	a.server.MarkListening()

	// The key file must exist at 0600 inside a 0700 directory on a first run.
	assertMode(t, filepath.Join(dir, "keys"), 0o700)
	assertMode(t, filepath.Join(dir, "keys", "secret.key"), 0o600)

	return &testEnv{app: a, srv: srv, cookies: map[string]string{}, logBuf: buf, base: urlBase}
}

func (e *testEnv) logs() string { return e.logBuf.String() }

type reqOpt func(*http.Request)

func withoutCSRF(r *http.Request) { r.Header.Del("X-CSRF-Token") }
func withoutSession(r *http.Request) {
	r.Header.Del("Cookie")
}

func asFormPost(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }

// raw issues a request and returns the status and body.
func (e *testEnv) raw(t *testing.T, method, path string, body any, opts ...reqOpt) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		blob, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(blob)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, e.srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	e.applyCookies(req)
	if e.csrf != "" {
		req.Header.Set("X-CSRF-Token", e.csrf)
	}
	for _, opt := range opts {
		opt(req)
	}

	// Redirects are not followed: a 301 to the url-base is a result the test
	// wants to see rather than transparently chase.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	e.captureCookies(resp)

	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(blob)
}

// do issues a request, requires 2xx, and decodes into out.
func (e *testEnv) do(t *testing.T, method, path string, body, out any) {
	t.Helper()
	code, blob := e.raw(t, method, path, body)
	if code < 200 || code > 299 {
		t.Fatalf("%s %s = %d: %s", method, path, code, blob)
	}
	if out != nil && strings.TrimSpace(blob) != "" {
		mustUnmarshal(t, blob, out)
	}
}

// Cookies are tracked by hand rather than with net/http/cookiejar, because the
// jar refuses to return a Secure cookie over plain http and the test needs to
// exercise both the Secure and non-Secure paths without arguing with it.
func (e *testEnv) applyCookies(r *http.Request) {
	for name, value := range e.cookies {
		r.AddCookie(&http.Cookie{Name: name, Value: value})
	}
}

func (e *testEnv) captureCookies(resp *http.Response) {
	for _, c := range resp.Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(e.cookies, c.Name)
			continue
		}
		e.cookies[c.Name] = c.Value
		if c.Name == "usarr_csrf" {
			e.csrf = c.Value
		}
	}
}

// ── SSE client ──────────────────────────────────────────────────────────────

type sseEvent struct {
	id   string
	name string
	data string
}

type sseStream struct {
	events chan sseEvent
	cancel context.CancelFunc

	mu  sync.Mutex
	raw strings.Builder
}

func (s *sseStream) close() { s.cancel() }

func (s *sseStream) dump() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.raw.String()
}

// openStream connects to GET /api/events and parses frames off it.
func (e *testEnv) openStream(t *testing.T) *sseStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.srv.URL+e.base+"/api/events", nil)
	if err != nil {
		cancel()
		t.Fatalf("build SSE request: %v", err)
	}
	e.applyCookies(req)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed by the reader goroutine
	if err != nil {
		cancel()
		t.Fatalf("open SSE: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("SSE returned %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("SSE Content-Type = %q", ct)
	}

	// readSSE (browserflow_test.go) is the one framing parser both harnesses use,
	// so a change to the wire format cannot be fixed in one of them and missed in
	// the other.
	stream := &sseStream{events: make(chan sseEvent, 64), cancel: cancel}
	go readSSE(ctx, resp, stream)
	return stream
}

// ── assertions and small helpers ────────────────────────────────────────────

// assertNoSecret is the defence-in-depth check the task exists for. Prowlarr
// puts its full admin API key in every search result's downloadUrl and
// magnetUrl; this asserts that none of it reached the given surface.
func assertNoSecret(t *testing.T, where, haystack string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(haystack, secret) {
			idx := strings.Index(haystack, secret)
			lo, hi := max(0, idx-120), min(len(haystack), idx+len(secret)+120)
			t.Errorf("%s leaked %q:\n…%s…", where, secret, haystack[lo:hi])
		}
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %04o, want %04o", path, got, want)
	}
}

func mustUnmarshal(t *testing.T, blob string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(blob), out); err != nil {
		t.Fatalf("decode %q: %v", truncateForLog(blob), err)
	}
}

func truncateForLog(s string) string {
	const max = 400
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// ── response shapes, declared here so the test reads the wire, not the code ──

type sessionBody struct {
	Authenticated bool       `json:"authenticated"`
	SetupRequired bool       `json:"setup_required"`
	CSRFToken     string     `json:"csrf_token"`
	UserID        int64      `json:"user_id"`
	SudoUntil     *time.Time `json:"sudo_until"`
}

type serviceBody struct {
	ID            int64  `json:"id"`
	Kind          string `json:"kind"`
	Role          string `json:"role"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	URLBase       string `json:"url_base"`
	APIVersion    string `json:"api_version"`
	Enabled       bool   `json:"enabled"`
	HasCredential bool   `json:"has_credential"`
}

type servicesListBody struct {
	Services []serviceBody `json:"services"`
}

// errorBodyShape is internal/httpapi's errorBody. The SPA branches on `error`
// and renders `message`/`action`, so a test that only checked the status would
// miss the half of the contract the screens actually use.
type errorBodyShape struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

type testBody struct {
	OK             bool   `json:"ok"`
	Reachable      bool   `json:"reachable"`
	KeyProvenValid bool   `json:"key_proven_valid"`
	AppName        string `json:"app_name"`
	AppVersion     string `json:"app_version"`
	Message        string `json:"message"`
}

type searchAcceptedBody struct {
	SearchID   string `json:"search_id"`
	InstanceID int64  `json:"instance_id"`
	EventsURL  string `json:"events_url"`
}

type releaseBody struct {
	CandidateID int64     `json:"candidate_id"`
	GUID        string    `json:"guid"`
	Title       string    `json:"title"`
	IndexerName string    `json:"indexer_name"`
	Protocol    string    `json:"protocol"`
	SizeBytes   int64     `json:"size_bytes"`
	InfoURL     string    `json:"info_url"`
	Tags        []string  `json:"tags"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type reportBody struct {
	Summary  string `json:"summary"`
	Degraded bool   `json:"degraded"`
	Answered int    `json:"answered"`
	Failed   int    `json:"failed"`
	Skipped  int    `json:"skipped"`
	Results  int    `json:"results"`
}

type grabBody struct {
	CandidateID  int64  `json:"candidate_id"`
	ProvenanceID int64  `json:"provenance_id"`
	ReleaseTitle string `json:"release_title"`
	Protocol     string `json:"protocol"`
	IndexerName  string `json:"indexer_name"`
	ReSearched   bool   `json:"re_searched"`
	Message      string `json:"message"`
}

type serviceHealthBody struct {
	ID              int64                     `json:"id"`
	Name            string                    `json:"name"`
	State           string                    `json:"state"`
	Problem         string                    `json:"problem"`
	Action          string                    `json:"action"`
	AppVersion      string                    `json:"app_version"`
	Warnings        []httpapi.UpstreamWarning `json:"warnings"`
	BlockedIndexers []httpapi.BlockedIndexer  `json:"blocked_indexers"`
	Stale           bool                      `json:"stale"`
}

type servicesHealthBody struct {
	Services      []serviceHealthBody `json:"services"`
	AnyUnhealthy  bool                `json:"any_unhealthy"`
	SetupRequired bool                `json:"setup_required"`
}
