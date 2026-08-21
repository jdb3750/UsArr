package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// POST /api/v1/services/{id}/sync, at the handler boundary. The wiring and the
// real concurrency guard are exercised in cmd/usarr/import_rerun_test.go; what
// is here is the contract this package owns — which outcome gets which status
// and which code, and what the body may contain.

// fakeImports records what it was asked to start and answers with err.
//
// The two methods carry SEPARATE recorders and SEPARATE errors, because half of
// what the tests below assert is that a press on one route does not reach the
// other port method — a shared field would make the two routes indistinguishable
// in exactly the dimension the tests are about.
type fakeImports struct {
	err     error
	started []int64

	deltaErr     error
	deltaStarted []int64
}

func (f *fakeImports) StartImport(instanceID int64) error {
	f.started = append(f.started, instanceID)
	return f.err
}

func (f *fakeImports) StartDeltaSync(instanceID int64) error {
	f.deltaStarted = append(f.deltaStarted, instanceID)
	return f.deltaErr
}

func seedKavita(t *testing.T, s *Server, name string, enabled bool) int64 {
	t.Helper()
	id, err := s.store.CreateServiceInstance(context.Background(), store.ServiceInstance{
		Kind: "kavita", Role: "library", Name: name,
		BaseURL: "http://kavita.example:5000", APIKeyEnc: []byte{0},
		Enabled: enabled, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("seed kavita: %v", err)
	}
	return id
}

func TestSyncStartsAnImportAndSaysSoWithoutWaiting(t *testing.T) {
	imports := &fakeImports{}
	s := newTestServer(t, func(c *Config) { c.Imports = imports })
	id := seedKavita(t, s, "Kavita", true)

	code, body := asOwner(t, s, s.handleSyncService, http.MethodPost, "/api/v1/services/1/sync", "", id)
	if code != http.StatusAccepted {
		t.Fatalf("POST sync = %d, want 202: %s", code, body)
	}
	if len(imports.started) != 1 || imports.started[0] != id {
		t.Fatalf("StartImport calls = %v, want exactly [%d]", imports.started, id)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	// THE ALLOWLIST. Four fields, and any fifth is a leak waiting to happen —
	// the row this is built from holds an encrypted full-admin credential.
	allowed := map[string]bool{"status": true, "instance_id": true, "kind": true, "name": true}
	for k := range got {
		if !allowed[k] {
			t.Errorf("the 202 body carries %q, which is not allowlisted: %s", k, body)
		}
	}
	if got["status"] != "started" {
		t.Errorf("status = %v, want %q", got["status"], "started")
	}
	if got["kind"] != "kavita" || got["name"] != "Kavita" {
		t.Errorf("unexpected identity fields: %s", body)
	}
	// No progress field of any kind: nothing has been read when this is written,
	// so any count would be invented.
	for _, forbidden := range []string{"progress", "percent", "items", "total", "eta", "base_url", "api_key"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the 202 body mentions %q: %s", forbidden, body)
		}
	}
}

// The four outcomes a caller has to tell apart, each with its own status and its
// own code. A client that cannot separate them shows the wrong sentence.
func TestSyncOutcomesAreDistinguishableOnTheWire(t *testing.T) {
	cases := []struct {
		name     string
		portErr  error
		seed     func(t *testing.T, s *Server) int64
		wantCode int
		wantErr  string
		// wantStarted is whether the port should have been reached at all.
		wantStarted bool
	}{
		{
			name:    "already running",
			portErr: ErrImportInProgress,
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Kavita", true)
			},
			wantCode: http.StatusConflict, wantErr: "import_in_progress", wantStarted: true,
		},
		{
			name: "unknown instance",
			seed: func(t *testing.T, s *Server) int64 { return 9999 },
			// Not started: an id nobody owns never reaches the importer.
			wantCode: http.StatusNotFound, wantErr: "not_found",
		},
		{
			name: "not a catalogue source",
			seed: func(t *testing.T, s *Server) int64 { return seedInstance(t, s, "") },
			// Refused from SQLite alone, so the port is never called.
			wantCode: http.StatusConflict, wantErr: "not_a_catalogue_source",
		},
		{
			name: "disabled",
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Off", false)
			},
			wantCode: http.StatusConflict, wantErr: "service_disabled",
		},
		{
			name:    "the port refuses a kind the boundary let through",
			portErr: ErrNotCatalogueSource,
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Kavita", true)
			},
			wantCode: http.StatusConflict, wantErr: "not_a_catalogue_source", wantStarted: true,
		},
		{
			name:    "anything else is a 500 that names the reason",
			portErr: errors.New("the stored API key for \"Kavita\" cannot be opened"),
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Kavita", true)
			},
			wantCode: http.StatusInternalServerError, wantErr: "internal", wantStarted: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			imports := &fakeImports{err: tc.portErr}
			s := newTestServer(t, func(c *Config) { c.Imports = imports })
			id := tc.seed(t, s)

			code, body := asOwner(t, s, s.handleSyncService, http.MethodPost,
				"/api/v1/services/1/sync", "", id)
			if code != tc.wantCode || !strings.Contains(body, `"error":"`+tc.wantErr+`"`) {
				t.Fatalf("= %d %s, want %d %s", code, body, tc.wantCode, tc.wantErr)
			}
			if started := len(imports.started) > 0; started != tc.wantStarted {
				t.Errorf("StartImport reached = %v, want %v — %v", started, tc.wantStarted, imports.started)
			}
		})
	}
}

// A build with no importer says so, rather than answering 202 for something that
// will never run.
func TestSyncWithNoImporterWiredInSaysSo(t *testing.T) {
	s := newTestServer(t, nil)
	id := seedKavita(t, s, "Kavita", true)

	code, body := asOwner(t, s, s.handleSyncService, http.MethodPost, "/api/v1/services/1/sync", "", id)
	if code != http.StatusNotImplemented || !strings.Contains(body, `"error":"not_configured"`) {
		t.Fatalf("= %d %s, want 501 not_configured", code, body)
	}
}

// The route is gated exactly like the five service writes beside it: CSRF, a
// session, and the sudo window. Driven through the REAL middleware chain with a
// real session, because the middleware order is the subject and calling the
// handler directly bypasses every layer of it.
//
// The sudo case is the one that needs a clock, and it is the one worth having:
// a route that lost s.sudo() would still pass every other test in this file.
func TestSyncIsGatedLikeEveryOtherServiceWrite(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	s := newTestServer(t, func(c *Config) {
		c.Imports = &fakeImports{}
		c.Now = func() time.Time { return now }
	})
	id := seedKavita(t, s, "Kavita", true)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	path := srv.URL + "/api/v1/services/" + strconv.FormatInt(id, 10) + "/sync"

	c := &cookieClient{t: t, jar: map[string]string{}}

	// 1. No session and no token. csrfProtected is outermost, so this is what a
	//    cross-site POST meets first.
	if code, body := c.post(path, false); code != http.StatusForbidden ||
		!strings.Contains(body, `"error":"csrf"`) {
		t.Fatalf("a POST with no CSRF token = %d %s, want 403 csrf", code, body)
	}

	// 2. A real owner session with a fresh sudo window: 202.
	c.get(srv.URL + "/api/v1/auth/session")
	if code, body := c.postJSON(srv.URL+"/api/v1/auth/setup",
		`{"username":"joe","password":"correct horse battery"}`); code != http.StatusOK {
		t.Fatalf("setup = %d: %s", code, body)
	}
	if code, body := c.post(path, true); code != http.StatusAccepted {
		t.Fatalf("a signed-in owner inside the sudo window = %d, want 202: %s", code, body)
	}

	// 3. The same session, past the sudo window. Nothing else changes.
	now = now.Add(6 * time.Minute)
	code, body := c.post(path, true)
	if code != http.StatusForbidden || !strings.Contains(body, `"error":"sudo_required"`) {
		t.Fatalf("a sync outside the sudo window = %d %s, want 403 sudo_required — this write "+
			"is gated like the five beside it", code, body)
	}
}

// cookieClient is the minimum browser: a cookie map and the double-submit token
// read back out of it. A net/http/cookiejar is not used for the same reason
// cmd/usarr's harness does not use one — it argues about Secure cookies over
// plain http.
type cookieClient struct {
	t   *testing.T
	jar map[string]string
}

func (c *cookieClient) do(req *http.Request) (int, string) {
	c.t.Helper()
	for name, value := range c.jar {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	for _, ck := range resp.Cookies() {
		if ck.MaxAge < 0 || ck.Value == "" {
			delete(c.jar, ck.Name)
			continue
		}
		c.jar[ck.Name] = ck.Value
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

func (c *cookieClient) get(url string) (int, string) {
	c.t.Helper()
	req, err := http.NewRequestWithContext(c.t.Context(), http.MethodGet, url, nil)
	if err != nil {
		c.t.Fatalf("request: %v", err)
	}
	return c.do(req)
}

// post sends the empty JSON object every write endpoint here accepts, with or
// without the CSRF header.
func (c *cookieClient) post(url string, withToken bool) (int, string) {
	c.t.Helper()
	req, err := http.NewRequestWithContext(c.t.Context(), http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		c.t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if withToken {
		req.Header.Set("X-CSRF-Token", c.jar["usarr_csrf"])
	}
	return c.do(req)
}

func (c *cookieClient) postJSON(url, body string) (int, string) {
	c.t.Helper()
	req, err := http.NewRequestWithContext(c.t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		c.t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.jar["usarr_csrf"])
	return c.do(req)
}

// ── POST /api/v1/services/{id}/sync/delta ───────────────────────────────────
//
// Same two altitudes as the full sync above: which outcome gets which status and
// which code, and then the middleware chain. The engine's own refusals, and the
// proof that `not_a_delta_source` is reachable from a REAL adapter rather than
// only from the fake below, are cmd/usarr/delta_route_e2e_test.go's — a table
// that only ever exercises a fake proves the mapper, never the wiring.

// The five outcomes a caller has to tell apart, plus the one the delta adds. The
// added one is the whole reason this route is not a flag on /sync:
// `not_a_delta_source` means the service is FINE and a full sync of it works.
func TestDeltaSyncOutcomesAreDistinguishableOnTheWire(t *testing.T) {
	cases := []struct {
		name     string
		portErr  error
		seed     func(t *testing.T, s *Server) int64
		wantCode int
		wantErr  string
		// wantStarted is whether the port should have been reached at all.
		wantStarted bool
		// wantAction, when set, is the sentence the client shows next. It is
		// asserted for the new code because a 409 with no action is a dead end
		// on a screen whose whole job is saying how to fix things (§17.3).
		wantAction string
	}{
		{
			name:    "the source has a catalogue and no change feed",
			portErr: ErrNotDeltaSource,
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Kavita", true)
			},
			wantCode: http.StatusConflict, wantErr: "not_a_delta_source", wantStarted: true,
			wantAction: "Run a full sync instead",
		},
		{
			name:    "already running — one guard covers both reads",
			portErr: ErrImportInProgress,
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Kavita", true)
			},
			wantCode: http.StatusConflict, wantErr: "import_in_progress", wantStarted: true,
		},
		{
			name: "disabled",
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Off", false)
			},
			wantCode: http.StatusConflict, wantErr: "service_disabled",
		},
		{
			name: "not a catalogue source",
			seed: func(t *testing.T, s *Server) int64 { return seedInstance(t, s, "") },
			// Refused from SQLite alone, so the port is never called.
			wantCode: http.StatusConflict, wantErr: "not_a_catalogue_source",
		},
		{
			name:     "unknown instance",
			seed:     func(t *testing.T, s *Server) int64 { return 9999 },
			wantCode: http.StatusNotFound, wantErr: "not_found",
		},
		{
			name:    "anything else is a 500 that names the reason",
			portErr: errors.New("the stored API key for \"Kavita\" cannot be opened"),
			seed: func(t *testing.T, s *Server) int64 {
				return seedKavita(t, s, "Kavita", true)
			},
			wantCode: http.StatusInternalServerError, wantErr: "internal", wantStarted: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			imports := &fakeImports{deltaErr: tc.portErr}
			s := newTestServer(t, func(c *Config) { c.Imports = imports })
			id := tc.seed(t, s)

			code, body := asOwner(t, s, s.handleDeltaSyncService, http.MethodPost,
				"/api/v1/services/1/sync/delta", "", id)
			if code != tc.wantCode || !strings.Contains(body, `"error":"`+tc.wantErr+`"`) {
				t.Fatalf("= %d %s, want %d %s", code, body, tc.wantCode, tc.wantErr)
			}
			if tc.wantAction != "" && !strings.Contains(body, `"action":"`+tc.wantAction+`"`) {
				t.Errorf("the refusal names no way out: %s\nwant action %q", body, tc.wantAction)
			}
			if started := len(imports.deltaStarted) > 0; started != tc.wantStarted {
				t.Errorf("StartDeltaSync reached = %v, want %v — %v",
					started, tc.wantStarted, imports.deltaStarted)
			}
			// AND THE OTHER PORT METHOD IS NEVER TOUCHED. A delta press that fell
			// through to StartImport would re-read the whole library — minutes of
			// upstream traffic for a button that promised a five-minute window.
			if len(imports.started) != 0 {
				t.Errorf("a delta press reached StartImport: %v", imports.started)
			}
		})
	}
}

// The happy path: 202, the same four allowlisted fields the full sync answers
// with, and the delta port reached exactly once.
func TestDeltaSyncStartsAWalkAndSaysSoWithoutWaiting(t *testing.T) {
	imports := &fakeImports{}
	s := newTestServer(t, func(c *Config) { c.Imports = imports })
	id := seedKavita(t, s, "Kavita", true)

	code, body := asOwner(t, s, s.handleDeltaSyncService, http.MethodPost,
		"/api/v1/services/1/sync/delta", "", id)
	if code != http.StatusAccepted {
		t.Fatalf("POST sync/delta = %d, want 202: %s", code, body)
	}
	if len(imports.deltaStarted) != 1 || imports.deltaStarted[0] != id {
		t.Fatalf("StartDeltaSync calls = %v, want exactly [%d]", imports.deltaStarted, id)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	allowed := map[string]bool{"status": true, "instance_id": true, "kind": true, "name": true}
	for k := range got {
		if !allowed[k] {
			t.Errorf("the 202 body carries %q, which is not allowlisted: %s", k, body)
		}
	}
	if got["status"] != "started" {
		t.Errorf("status = %v, want %q — inherited from POST /sync, whose body this one "+
			"matches field for field", got["status"], "started")
	}
	// ⚠️ NO ESCALATION FIELD, AND THERE CANNOT BE ONE. Whether the walk escalates
	// to a full import is decided after this body is written; a field here would
	// be a guess, and a client that branched on it would branch on nothing.
	for _, forbidden := range []string{"escalat", "progress", "items", "window", "watermark", "base_url", "api_key"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the 202 body mentions %q: %s", forbidden, body)
		}
	}
}

// A build with no importer says so, rather than answering 202 for a walk that
// will never run.
func TestDeltaSyncWithNoImporterWiredInSaysSo(t *testing.T) {
	s := newTestServer(t, nil)
	id := seedKavita(t, s, "Kavita", true)

	code, body := asOwner(t, s, s.handleDeltaSyncService, http.MethodPost,
		"/api/v1/services/1/sync/delta", "", id)
	if code != http.StatusNotImplemented || !strings.Contains(body, `"error":"not_configured"`) {
		t.Fatalf("= %d %s, want 501 not_configured", code, body)
	}
}

// 🚩 THE DELTA'S OWN CODE MUST NOT LEAK ONTO THE FULL SYNC'S ROUTE. The two
// mappers share four outcomes and the delta adds one; if that one were added to
// importStartError instead of wrapped around it, POST /sync would be free to
// answer a code its documented error table (reference/http-api.md §4.4) does not
// carry and no client of it branches on.
func TestTheFullSyncRouteNeverAnswersTheDeltasOwnCode(t *testing.T) {
	imports := &fakeImports{err: ErrNotDeltaSource}
	s := newTestServer(t, func(c *Config) { c.Imports = imports })
	id := seedKavita(t, s, "Kavita", true)

	code, body := asOwner(t, s, s.handleSyncService, http.MethodPost, "/api/v1/services/1/sync", "", id)
	if strings.Contains(body, string(CodeNotDeltaSource)) {
		t.Fatalf("POST /sync answered the delta route's code: %d %s", code, body)
	}
	if code != http.StatusInternalServerError || !strings.Contains(body, `"error":"internal"`) {
		t.Fatalf("= %d %s, want 500 internal — an unmapped sentinel is the generic arm", code, body)
	}
}

// The delta route is gated exactly like every other service write beside it:
// CSRF, a session, and the sudo window. (Deliberately not counted — see
// server.go's note where the two sync routes are registered.) Driven through the REAL middleware chain, because
// the middleware order is the subject and calling the handler directly bypasses
// every layer of it.
//
// A route registered with one wrapper missing would still pass every other test
// in this file.
func TestDeltaSyncIsGatedLikeEveryOtherServiceWrite(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	s := newTestServer(t, func(c *Config) {
		c.Imports = &fakeImports{}
		c.Now = func() time.Time { return now }
	})
	id := seedKavita(t, s, "Kavita", true)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	path := srv.URL + "/api/v1/services/" + strconv.FormatInt(id, 10) + "/sync/delta"

	c := &cookieClient{t: t, jar: map[string]string{}}

	// 1. No session and no token. csrfProtected is outermost, so this is what a
	//    cross-site POST meets first.
	if code, body := c.post(path, false); code != http.StatusForbidden ||
		!strings.Contains(body, `"error":"csrf"`) {
		t.Fatalf("a POST with no CSRF token = %d %s, want 403 csrf", code, body)
	}

	// 2. A real owner session with a fresh sudo window: 202. This is also the
	//    proof the route is REGISTERED — a missing mux entry answers 404 here and
	//    every handler-level test above would still pass.
	c.get(srv.URL + "/api/v1/auth/session")
	if code, body := c.postJSON(srv.URL+"/api/v1/auth/setup",
		`{"username":"joe","password":"correct horse battery"}`); code != http.StatusOK {
		t.Fatalf("setup = %d: %s", code, body)
	}
	if code, body := c.post(path, true); code != http.StatusAccepted {
		t.Fatalf("a signed-in owner inside the sudo window = %d, want 202: %s", code, body)
	}

	// 3. The same session, past the sudo window. Nothing else changes.
	now = now.Add(6 * time.Minute)
	code, body := c.post(path, true)
	if code != http.StatusForbidden || !strings.Contains(body, `"error":"sudo_required"`) {
		t.Fatalf("a delta sync outside the sudo window = %d %s, want 403 sudo_required — this "+
			"write is gated like the six beside it", code, body)
	}
}
