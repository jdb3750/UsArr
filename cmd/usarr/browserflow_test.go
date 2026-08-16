package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/httpapi"
)

// The end-to-end test next door drives the HTTP API directly, building each
// request by hand. That is exactly why the embedded SPA could not sign in, could
// not open the event stream and could not grab anything while every API test
// stayed green: nothing here was shaped like a browser.
//
// This file is. Everything below goes through browserClient, which is a
// transcription of web/src/lib/api.ts — one cookie jar, the CSRF token read back
// out of the jar the way the SPA reads it out of document.cookie, and the exact
// header set and body that postJson() puts on the wire. If the client and the
// server disagree about authentication, content type, the double-submit token or
// the request body, this test fails and web/src/lib/auth.test.ts fails with it.

// browserClient is what the SPA is, reduced to what it puts on the wire.
type browserClient struct {
	t    *testing.T
	base string
	jar  http.CookieJar
	http *http.Client
	root *url.URL

	// transcript is every response body this client has been handed. The SPA
	// can render anything in here, so "the API key never reaches the browser"
	// is one assertion over the whole of it rather than one per endpoint
	// somebody has to remember to add.
	transcript []string
}

// everythingSeen is every byte the SPA could possibly have rendered.
func (b *browserClient) everythingSeen() string { return strings.Join(b.transcript, "\n") }

func newBrowserClient(t *testing.T, serverURL, urlBase string) *browserClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	root, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	return &browserClient{
		t:    t,
		base: serverURL + urlBase,
		jar:  jar,
		root: root,
		http: &http.Client{
			Jar: jar,
			// A browser follows redirects, but a test wants to see them.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// csrfToken is document.cookie, transcribed. The server sets usarr_csrf without
// HttpOnly precisely so the SPA can do this; if that ever changes, the double
// submit stops working and this returns "".
func (b *browserClient) csrfToken() string {
	for _, c := range b.jar.Cookies(b.root) {
		if c.Name == "usarr_csrf" {
			return c.Value
		}
	}
	return ""
}

// browserResponse is everything the SPA can see of a reply: the status, the
// body, and the cookies the browser just filed away. The *http.Response itself
// is deliberately not carried out of send() — it is already drained and closed,
// and handing it around only invites a use-after-close.
type browserResponse struct {
	path    string
	status  int
	body    string
	cookies []*http.Cookie
}

// get is requestJson() with no init: the jar's cookies and nothing else.
func (b *browserClient) get(path string) browserResponse {
	b.t.Helper()
	return b.send(b.request(http.MethodGet, path, nil, func(r *http.Request) {
		r.Header.Set("Accept", "application/json")
	}))
}

// postJSON is postJson(), transcribed exactly: the JSON content type
// csrfProtected demands, the token it compares against the cookie, and a real
// body because decodeJSON answers 400 on an empty one.
func (b *browserClient) postJSON(path string, body any, opts ...func(*http.Request)) browserResponse {
	b.t.Helper()
	return b.writeJSON(http.MethodPost, path, body, opts...)
}

// writeJSON is sendJson(), transcribed: the SPA routes POST, PATCH and DELETE
// through ONE helper, because csrfProtected wraps all three identically in
// server.go and checks the content type before the token. A DELETE issued as a
// bare fetch answers 415, not 403, which reads like a routing bug — so the
// header set here is the same for every method, DELETE's unused body included.
func (b *browserClient) writeJSON(method, path string, body any, opts ...func(*http.Request)) browserResponse {
	b.t.Helper()
	blob, err := json.Marshal(body)
	if err != nil {
		b.t.Fatalf("marshal %s body: %v", path, err)
	}
	prep := []func(*http.Request){func(r *http.Request) {
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", b.csrfToken())
	}}
	return b.send(b.request(method, path, blob, append(prep, opts...)...))
}

func (b *browserClient) patchJSON(path string, body any, opts ...func(*http.Request)) browserResponse {
	b.t.Helper()
	return b.writeJSON(http.MethodPatch, path, body, opts...)
}

func (b *browserClient) deleteJSON(path string, opts ...func(*http.Request)) browserResponse {
	b.t.Helper()
	return b.writeJSON(http.MethodDelete, path, map[string]any{}, opts...)
}

func (b *browserClient) request(method, path string, body []byte, opts ...func(*http.Request)) *http.Request {
	b.t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, b.base+path, reader)
	if err != nil {
		b.t.Fatalf("build %s %s: %v", method, path, err)
	}
	for _, opt := range opts {
		opt(req)
	}
	return req
}

func (b *browserClient) send(req *http.Request) browserResponse {
	b.t.Helper()
	resp, err := b.http.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		b.t.Fatalf("read body: %v", err)
	}
	b.transcript = append(b.transcript,
		fmt.Sprintf("%s %s → %d\n%s", req.Method, req.URL.Path, resp.StatusCode, blob))
	return browserResponse{
		path:    req.URL.Path,
		status:  resp.StatusCode,
		body:    string(blob),
		cookies: resp.Cookies(),
	}
}

// mustPost fails unless the request succeeded, and decodes into out.
func (b *browserClient) mustPost(path string, body, out any) browserResponse {
	b.t.Helper()
	res := b.postJSON(path, body)
	if res.status < 200 || res.status > 299 {
		b.t.Fatalf("POST %s = %d: %s", path, res.status, res.body)
	}
	res.decode(b.t, out)
	return res
}

func (b *browserClient) mustGet(path string, out any) browserResponse {
	b.t.Helper()
	res := b.get(path)
	if res.status < 200 || res.status > 299 {
		b.t.Fatalf("GET %s = %d: %s", path, res.status, res.body)
	}
	res.decode(b.t, out)
	return res
}

func (r browserResponse) decode(t *testing.T, out any) {
	t.Helper()
	if out != nil && strings.TrimSpace(r.body) != "" {
		mustUnmarshal(t, r.body, out)
	}
}

// ── the flow ────────────────────────────────────────────────────────────────

// TestBrowserFlowSetupLoginSearchAndGrab drives the whole SPA path through the
// real server the way a browser does it:
//
//	bootstrap → first-run setup → sign out → sign back in → open the SSE stream
//	on the session cookie → search → read results off the stream → grab.
//
// The cookie jar is real, so a cookie the server sets with the wrong path,
// scope or flags fails this test rather than working by accident.
func TestBrowserFlowSetupLoginSearchAndGrab(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	const password = "correct horse battery"

	prowlarr := newFakeProwlarr(t, apiKey)
	env := newTestApp(t)
	b := newBrowserClient(t, env.srv.URL, env.base)

	// ── 1. the bootstrap call the layout makes before anything else ─────────
	//
	// It is unauthenticated by design: there is no session yet and there may be
	// no owner either, and the SPA cannot ask for either without a CSRF token.
	var boot sessionBody
	bootstrap := b.mustGet("/api/v1/auth/session", &boot)
	if !boot.SetupRequired {
		t.Fatal("a fresh install must report setup_required so the shell shows the setup form")
	}
	if boot.Authenticated {
		t.Fatal("nobody is signed in yet")
	}

	// The two cookie flags the whole design rests on. usarr_csrf MUST be
	// readable from script — that is what makes the double submit possible and
	// it is the only reason the SPA can talk to a state-changing endpoint at
	// all. usarr_session must NOT be.
	assertCookieFlags(t, bootstrap, "usarr_csrf", false)
	if b.csrfToken() == "" {
		t.Fatal("the jar holds no usarr_csrf cookie, so the SPA has nothing to echo in X-CSRF-Token")
	}
	if b.csrfToken() != boot.CSRFToken {
		t.Fatalf("the cookie (%q) and the JSON csrf_token (%q) disagree; the double submit compares "+
			"the header against the COOKIE, so the SPA must be able to use either", b.csrfToken(), boot.CSRFToken)
	}
	t.Logf("bootstrap: setup_required=%v csrf cookie readable from script, %d bytes",
		boot.SetupRequired, len(b.csrfToken()))

	// ── 2. first-run setup, exactly as the sign-in screen posts it ──────────
	var afterSetup sessionBody
	created := b.mustPost("/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": password}, &afterSetup)
	if !afterSetup.Authenticated {
		t.Fatal("setup must sign the new owner in")
	}
	assertCookieFlags(t, created, "usarr_session", true)
	assertCookieFlags(t, created, "usarr_csrf", false)

	// Setup closes permanently. The sign-in screen has to handle this, because a
	// second tab left open on the setup form will hit it.
	again := b.postJSON("/api/v1/auth/setup", map[string]any{"username": "someone", "password": password})
	if again.status != http.StatusConflict {
		t.Fatalf("a second setup must be 409 already_setup, got %d: %s", again.status, again.body)
	}
	if !strings.Contains(again.body, "already_setup") {
		t.Fatalf("the 409 must name already_setup so the screen can switch to signing in: %s", again.body)
	}
	t.Log("setup closed after the first owner, as the sign-in screen expects")

	// ── 3. sign out and back in, so login is exercised too ──────────────────
	b.mustPost("/api/v1/auth/logout", map[string]any{}, nil)
	if res := b.get("/api/v1/services"); res.status != http.StatusUnauthorized {
		t.Fatalf("after sign-out an authenticated route must 401, got %d", res.status)
	}

	// Signing back in needs a fresh CSRF token, because logout cleared the
	// session and the SPA re-bootstraps on the sign-in page.
	b.mustGet("/api/v1/auth/session", &boot)
	if boot.SetupRequired {
		t.Fatal("setup_required must be false once an owner exists; the screen would show the wrong form")
	}
	var afterLogin sessionBody
	b.mustPost("/api/v1/auth/login", map[string]any{"username": "joe", "password": password}, &afterLogin)
	if !afterLogin.Authenticated {
		t.Fatal("login must authenticate")
	}
	// Login regenerates both credentials; a jar holding the pre-login token
	// would now fail every write, which is the bug this assertion guards.
	if b.csrfToken() == "" {
		t.Fatal("login left no usarr_csrf cookie in the jar")
	}
	t.Logf("signed in as user_id=%d", afterLogin.UserID)

	// ── 4. add Prowlarr, through the same client ────────────────────────────
	//
	// Sudo is open because signing in is itself a fresh authentication.
	var svc serviceBody
	b.mustPost("/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prowlarr.URL(), "api_key": apiKey,
	}, &svc)
	if svc.ID == 0 {
		t.Fatalf("unexpected service row: %+v", svc)
	}

	// ── 5. the SSE stream, opened on the session cookie alone ───────────────
	//
	// This is the request that used to 401 in the browser. EventSource cannot
	// set a header and cannot report a status, so a 401 here is invisible from
	// the client: the stream retries forever and the search screen stays empty.
	stream := b.openStream(t)
	defer stream.close()

	// ── 6. search and read the stream ───────────────────────────────────────
	prowlarr.blockIndexer(2)

	var accepted searchAcceptedBody
	b.mustGet("/api/v1/search?query=Test+Release&type=search&category=2000", &accepted)
	if accepted.SearchID == "" {
		t.Fatal("the search must return a search id immediately")
	}

	var (
		results []releaseBody
		report  reportBody
	)
	deadline := time.After(30 * time.Second)
	for report.Summary == "" {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for search.done; %d results so far", len(results))
		case ev := <-stream.events:
			switch ev.name {
			case httpapi.EventSearchResults:
				var payload struct {
					Results []releaseBody `json:"results"`
				}
				mustUnmarshal(t, ev.data, &payload)
				results = append(results, payload.Results...)
			case httpapi.EventSearchDone:
				var payload struct {
					Report reportBody `json:"report"`
				}
				mustUnmarshal(t, ev.data, &payload)
				report = payload.Report
			}
		}
	}
	if len(results) == 0 {
		t.Fatal("no results reached the browser over SSE")
	}
	t.Logf("SSE delivered %d results to the browser: %s", len(results), report.Summary)

	candidate := results[0]

	// ── 7. the three ways grab used to fail, one at a time ──────────────────
	//
	// The old client sent a bare POST: no content type, no token, no body. Each
	// of these is one of the three, and each is refused on its own.
	grabPath := fmt.Sprintf("/api/v1/releases/%d/grab", candidate.CandidateID)
	t.Run("grab without the JSON content type is 415", func(t *testing.T) {
		res := b.postJSON(grabPath, map[string]any{}, func(r *http.Request) { r.Header.Del("Content-Type") })
		if res.status != http.StatusUnsupportedMediaType {
			t.Fatalf("= %d, want 415: %s", res.status, res.body)
		}
	})
	t.Run("grab without the CSRF header is 403", func(t *testing.T) {
		res := b.postJSON(grabPath, map[string]any{}, func(r *http.Request) { r.Header.Del("X-CSRF-Token") })
		if res.status != http.StatusForbidden {
			t.Fatalf("= %d, want 403: %s", res.status, res.body)
		}
	})
	t.Run("grab with an empty body is 400", func(t *testing.T) {
		res := b.send(b.request(http.MethodPost, grabPath, nil, func(r *http.Request) {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-CSRF-Token", b.csrfToken())
		}))
		if res.status != http.StatusBadRequest {
			t.Fatalf("= %d, want 400 from decodeJSON: %s", res.status, res.body)
		}
	})

	// ── 8. the grab the client actually builds ──────────────────────────────
	var grabbed grabBody
	b.mustPost(grabPath, map[string]any{}, &grabbed)
	if grabbed.CandidateID != candidate.CandidateID {
		t.Fatalf("grab returned candidate %d, wanted %d", grabbed.CandidateID, candidate.CandidateID)
	}
	if len(prowlarr.grabs()) != 1 {
		t.Fatalf("expected exactly one grab upstream, got %d", len(prowlarr.grabs()))
	}
	t.Logf("grab from the browser-shaped client: %q — %s", grabbed.ReleaseTitle, grabbed.Message)

	// ── 9. nothing leaked to the browser on the way ─────────────────────────
	assertNoSecret(t, "SSE stream as the browser saw it", stream.dump(), apiKey)
}

// TestBrowserFirstRunConfiguresAServiceAndSearches is the WHOLE first-run path,
// from a container that has never been started to a grabbed release, with every
// request shaped the way web/src/lib/api.ts shapes it.
//
// It exists because that path had a dead end in the middle. The shell could sign
// you in and could search, and between those two there was nothing: a fresh
// install has no service, so search answers 409 no_indexer_service, and the only
// way to fix it was to POST /api/v1/services from a terminal. The step this test
// adds over its neighbour above is everything the /services route does —
// listing, testing, the re-entry rule, the edit and the removal — asserted
// against the real server rather than against a stub of it.
//
// The ordering is the user's, not the API's: look, fail, fix, succeed.
func TestBrowserFirstRunConfiguresAServiceAndSearches(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	const password = "correct horse battery"

	prowlarr := newFakeProwlarr(t, apiKey)
	env := newTestApp(t)
	b := newBrowserClient(t, env.srv.URL, env.base)

	// ── 1. a container that has never been started ──────────────────────────
	var boot sessionBody
	b.mustGet("/api/v1/auth/session", &boot)
	if !boot.SetupRequired {
		t.Fatal("a first run must report setup_required")
	}
	b.mustPost("/api/v1/auth/setup", map[string]any{"username": "joe", "password": password}, &boot)

	// Sign out and back in, so the path under test is the one a returning user
	// walks and not just the one setup leaves behind.
	b.mustPost("/api/v1/auth/logout", map[string]any{}, nil)
	b.mustGet("/api/v1/auth/session", &boot)
	b.mustPost("/api/v1/auth/login", map[string]any{"username": "joe", "password": password}, &boot)

	// ── 2. the services screen, with nothing configured ─────────────────────
	var listed servicesListBody
	b.mustGet("/api/v1/services", &listed)
	if len(listed.Services) != 0 {
		t.Fatalf("a fresh install has no services, got %d", len(listed.Services))
	}
	var health servicesHealthBody
	b.mustGet("/api/v1/services/health", &health)
	if !health.SetupRequired {
		t.Fatal("with nothing configured the screen must say setup_required, " +
			"not render an empty table that looks broken")
	}

	// ── 3. the dead end this test exists for ────────────────────────────────
	//
	// The 409 must NAME the fix, because the search screen turns `error` into
	// the link to /services. A 409 whose code the client cannot match on is a
	// banner with nothing to click, which is where a new install used to stop.
	deadEnd := b.get("/api/v1/search?query=Test+Release")
	if deadEnd.status != http.StatusConflict {
		t.Fatalf("search with nothing configured = %d, want 409: %s", deadEnd.status, deadEnd.body)
	}
	var failure errorBodyShape
	deadEnd.decode(t, &failure)
	if failure.Error != "no_indexer_service" {
		t.Fatalf("the 409 must carry error=no_indexer_service so the SPA can link to the fix, got %q", failure.Error)
	}
	t.Logf("empty install: search → 409 %s (%q, action %q)", failure.Error, failure.Message, failure.Action)

	// ── 4. test before saving, the way the add form's second button does ────
	var probe testBody
	b.mustPost("/api/v1/services/test", map[string]any{
		"kind": "prowlarr", "base_url": prowlarr.URL(), "api_key": "wrong-key",
	}, &probe)
	if probe.OK {
		t.Fatal("a wrong key must not test OK")
	}
	if strings.TrimSpace(probe.Message) == "" {
		t.Fatal("a failed test must carry the upstream's own words; a blank message is the " +
			"'an error occurred' §17.3 refuses")
	}
	t.Logf("unsaved test with a wrong key: ok=%v reachable=%v %q", probe.OK, probe.Reachable, probe.Message)

	b.mustPost("/api/v1/services/test", map[string]any{
		"kind": "prowlarr", "base_url": prowlarr.URL(), "api_key": apiKey,
	}, &probe)
	if !probe.OK {
		t.Fatalf("the unsaved test with the right key must pass: %+v", probe)
	}

	// ── 5. add it, with the body createService() builds ─────────────────────
	var svc serviceBody
	created := b.postJSON("/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prowlarr.URL(), "api_key": apiKey,
	})
	if created.status != http.StatusCreated {
		t.Fatalf("POST /api/v1/services = %d, want 201: %s", created.status, created.body)
	}
	created.decode(t, &svc)
	if svc.ID == 0 || svc.Role != "indexer" {
		t.Fatalf("unexpected service row: %+v", svc)
	}

	// ── 6. what the list gives the screen back ──────────────────────────────
	b.mustGet("/api/v1/services", &listed)
	if len(listed.Services) != 1 {
		t.Fatalf("expected one configured service, got %d", len(listed.Services))
	}
	if !listed.Services[0].HasCredential {
		t.Error("has_credential must be true: it is the ONLY thing the screen can say about a " +
			"stored key, because the key itself is never returned in any form")
	}

	// ── 7. re-test the saved instance ───────────────────────────────────────
	var retest testBody
	b.mustPost(fmt.Sprintf("/api/v1/services/%d/test", svc.ID), map[string]any{}, &retest)
	if !retest.OK {
		t.Fatalf("re-testing the saved instance failed: %+v", retest)
	}
	t.Logf("saved-instance test: ok=%v key_proven_valid=%v %q", retest.OK, retest.KeyProvenValid, retest.Message)

	// ── 8. the rule the edit form has to surface ────────────────────────────
	//
	// requiresCredentialReentry() in web/src/lib/api.ts refuses this edit before
	// it is sent. The server refuses it too, and this is the assertion that the
	// two agree — if the client ever stopped checking, the user would land here.
	moved := b.patchJSON(fmt.Sprintf("/api/v1/services/%d", svc.ID),
		map[string]any{"base_url": "http://attacker.example:9696"})
	if moved.status != http.StatusBadRequest {
		t.Fatalf("moving base_url with no key = %d, want 400: %s", moved.status, moved.body)
	}
	moved.decode(t, &failure)
	if failure.Error != "credential_reentry_required" {
		t.Fatalf("the 400 must carry error=credential_reentry_required so the form can explain "+
			"itself instead of showing a bare 400, got %q", failure.Error)
	}
	t.Logf("changed-host PATCH refused: %s — %s", failure.Message, failure.Action)

	// A rename touches no credential and goes through.
	var renamed serviceBody
	patched := b.patchJSON(fmt.Sprintf("/api/v1/services/%d", svc.ID), map[string]any{"name": "Prowlarr (LAN)"})
	if patched.status != http.StatusOK {
		t.Fatalf("PATCH name = %d, want 200: %s", patched.status, patched.body)
	}
	patched.decode(t, &renamed)
	if renamed.Name != "Prowlarr (LAN)" {
		t.Fatalf("rename did not stick: %+v", renamed)
	}

	// ── 9. and now the thing the user came for ──────────────────────────────
	stream := b.openStream(t)
	defer stream.close()

	prowlarr.blockIndexer(2)

	var accepted searchAcceptedBody
	b.mustGet("/api/v1/search?query=Test+Release&type=search&category=2000", &accepted)
	if accepted.SearchID == "" {
		t.Fatal("the search must return a search id immediately")
	}

	var (
		results []releaseBody
		report  reportBody
	)
	deadline := time.After(30 * time.Second)
	for report.Summary == "" {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for search.done; %d results so far", len(results))
		case ev := <-stream.events:
			switch ev.name {
			case httpapi.EventSearchResults:
				var payload struct {
					Results []releaseBody `json:"results"`
				}
				mustUnmarshal(t, ev.data, &payload)
				results = append(results, payload.Results...)
			case httpapi.EventSearchDone:
				var payload struct {
					Report reportBody `json:"report"`
				}
				mustUnmarshal(t, ev.data, &payload)
				report = payload.Report
			}
		}
	}
	if len(results) == 0 {
		t.Fatal("no results reached the browser over SSE")
	}

	var grabbed grabBody
	b.mustPost(fmt.Sprintf("/api/v1/releases/%d/grab", results[0].CandidateID), map[string]any{}, &grabbed)
	if len(prowlarr.grabs()) != 1 {
		t.Fatalf("expected exactly one grab upstream, got %d", len(prowlarr.grabs()))
	}
	t.Logf("first run finished: configured a service in the browser and grabbed %q", grabbed.ReleaseTitle)

	// ── 10. removal puts the install back where it started ──────────────────
	removed := b.deleteJSON(fmt.Sprintf("/api/v1/services/%d", svc.ID))
	if removed.status != http.StatusNoContent {
		t.Fatalf("DELETE /api/v1/services/%d = %d, want 204: %s", svc.ID, removed.status, removed.body)
	}
	b.mustGet("/api/v1/services", &listed)
	if len(listed.Services) != 0 {
		t.Fatalf("the removed service is still listed: %+v", listed.Services)
	}
	if again := b.get("/api/v1/search?query=Test+Release"); again.status != http.StatusConflict {
		t.Fatalf("with the only indexer removed, search must be 409 again, got %d: %s", again.status, again.body)
	}

	// The three write methods, each refused on its own if it drops a header
	// csrfProtected demands. DELETE is the one that would be hand-rolled without
	// a content type, and it answers 415 rather than anything that reads like a
	// routing problem.
	t.Run("DELETE without the JSON content type is 415", func(t *testing.T) {
		res := b.deleteJSON("/api/v1/services/999999", func(r *http.Request) { r.Header.Del("Content-Type") })
		if res.status != http.StatusUnsupportedMediaType {
			t.Fatalf("= %d, want 415: %s", res.status, res.body)
		}
	})
	t.Run("PATCH without the CSRF header is 403", func(t *testing.T) {
		res := b.patchJSON("/api/v1/services/999999", map[string]any{"name": "x"},
			func(r *http.Request) { r.Header.Del("X-CSRF-Token") })
		if res.status != http.StatusForbidden {
			t.Fatalf("= %d, want 403: %s", res.status, res.body)
		}
	})

	// ── 11. nothing the browser was handed carried the credential ───────────
	//
	// Every response body of every request above, plus the SSE stream. The key
	// went UP on create and on two tests; it must never have come back down.
	assertNoSecret(t, "everything this browser was served", b.everythingSeen(), apiKey)
	assertNoSecret(t, "SSE stream as the browser saw it", stream.dump(), apiKey)
	assertNoSecret(t, "process log", env.logs(), apiKey)
}

// TestSPARequestsSatisfyTheMiddleware sweeps every endpoint web/src/lib/api.ts
// calls, issued the way that file issues it, and fails if the middleware in
// front of it refuses the request.
//
// A 404 or a 409 from a HANDLER is fine and expected here — this is not testing
// the handlers. What it refuses to accept is 401 (authenticated), 403
// (csrfProtected's token check), 415 (its content-type check) and 400 (decodeJSON
// on an empty body), because every one of those means the SPA cannot reach the
// endpoint at all, which is the exact class of bug this file exists for.
func TestSPARequestsSatisfyTheMiddleware(t *testing.T) {
	env := newTestApp(t)
	b := newBrowserClient(t, env.srv.URL, env.base)

	var boot sessionBody
	b.mustGet("/api/v1/auth/session", &boot)
	b.mustPost("/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &boot)

	// Every call in web/src/lib/api.ts. A non-empty `method` means it goes
	// through sendJson() and therefore carries the content type, the token and a
	// body — for PATCH and DELETE exactly as much as for POST.
	calls := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"checkReady", "", "/api/health/ready", nil},
		{"fetchSession", "", "/api/v1/auth/session", nil},
		{"startSearch", "", "/api/v1/search?query=anything", nil},
		{"openEventStream", "", "/api/events", nil},
		{"listServices", "", "/api/v1/services", nil},
		{"fetchServicesHealth", "", "/api/v1/services/health", nil},
		// A candidate id that does not exist: the handler answers 404, which
		// proves the request got PAST csrfProtected and decodeJSON.
		{"grabRelease", http.MethodPost, "/api/v1/releases/999999/grab", map[string]any{}},
		// A host nothing is listening on: the connection test fails, the handler
		// answers 200 with ok=false or 502, and either way the request reached it.
		{"testNewService", http.MethodPost, "/api/v1/services/test", map[string]any{
			"kind": "prowlarr", "base_url": "http://127.0.0.1:1", "api_key": "not-a-real-key",
		}},
		{"createService", http.MethodPost, "/api/v1/services", map[string]any{
			"kind": "prowlarr", "name": "Nothing", "base_url": "http://127.0.0.1:1", "api_key": "not-a-real-key",
		}},
		// Ids that do not exist: 404 again, and again that is past the middleware.
		{"testService", http.MethodPost, "/api/v1/services/999999/test", map[string]any{}},
		{"updateService", http.MethodPatch, "/api/v1/services/999999", map[string]any{"name": "x"}},
		{"deleteService", http.MethodDelete, "/api/v1/services/999999", map[string]any{}},
		{"setupOwner", http.MethodPost, "/api/v1/auth/setup", map[string]any{"username": "x", "password": "correct horse battery"}},
		{"confirmSudo", http.MethodPost, "/api/v1/auth/sudo", map[string]any{"password": "correct horse battery"}},
		{"login", http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "joe", "password": "correct horse battery"}},
		// Last: it ends the session every call above depends on.
		{"logout", http.MethodPost, "/api/v1/auth/logout", map[string]any{}},
	}

	refused := map[int]string{
		http.StatusUnauthorized:         "authenticated: the SPA sent no usable session cookie",
		http.StatusForbidden:            "csrfProtected: the X-CSRF-Token header does not match the usarr_csrf cookie",
		http.StatusUnsupportedMediaType: "csrfProtected: the request is not Content-Type: application/json",
		http.StatusBadRequest:           "decodeJSON: the body is not the JSON this endpoint expects",
	}

	for _, call := range calls {
		var res browserResponse
		switch {
		case call.path == "/api/events":
			// SSE never returns; opening it and reading the status is the test.
			res = browserResponse{path: call.path, status: b.streamStatus(t)}
		case call.method != "":
			res = b.writeJSON(call.method, call.path, call.body)
		default:
			res = b.get(call.path)
		}
		code, blob := res.status, res.body
		if why, bad := refused[code]; bad {
			t.Errorf("%s → %s %d — %s\n%s", call.name, call.path, code, why, strings.TrimSpace(blob))
			continue
		}
		t.Logf("%s → %s %d (past the middleware)", call.name, call.path, code)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// assertCookieFlags checks one Set-Cookie's HttpOnly flag. The SPA's ability to
// work at all depends on these two being opposite ways round.
func assertCookieFlags(t *testing.T, res browserResponse, name string, wantHTTPOnly bool) {
	t.Helper()
	for _, c := range res.cookies {
		if c.Name != name {
			continue
		}
		if c.HttpOnly != wantHTTPOnly {
			if wantHTTPOnly {
				t.Errorf("%s is not HttpOnly; a session cookie readable from script is an XSS token grab", name)
			} else {
				t.Errorf("%s is HttpOnly, so the SPA cannot read it and the double-submit check can never pass", name)
			}
		}
		return
	}
	t.Errorf("no %s cookie was set on %s", name, res.path)
}

// openStream is EventSource, reduced: a GET carrying only the jar's cookies.
func (b *browserClient) openStream(t *testing.T) *sseStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.base+"/api/events", nil)
	if err != nil {
		cancel()
		t.Fatalf("build SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := b.http.Do(req) //nolint:bodyclose // closed by the reader goroutine
	if err != nil {
		cancel()
		t.Fatalf("open SSE: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		blob, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("GET /api/events = %d for a signed-in browser: %s\n"+
			"EventSource cannot set a header and cannot report a status, so this is invisible "+
			"from the client: the stream retries forever behind a 'not connected' banner.",
			resp.StatusCode, strings.TrimSpace(string(blob)))
	}

	stream := &sseStream{events: make(chan sseEvent, 64), cancel: cancel}
	go readSSE(ctx, resp, stream)
	return stream
}

// streamStatus opens /api/events only far enough to read the status.
func (b *browserClient) streamStatus(t *testing.T) int {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.base+"/api/events", nil)
	if err != nil {
		t.Fatalf("build SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := b.http.Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// readSSE parses frames off an open stream. It is the same framing loop the
// other harness uses; it lives here so both clients share one parser.
func readSSE(ctx context.Context, resp *http.Response, stream *sseStream) {
	defer func() { _ = resp.Body.Close() }()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var cur sseEvent
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		stream.mu.Lock()
		stream.raw.WriteString(line + "\n")
		stream.mu.Unlock()

		switch {
		case line == "":
			if cur.name != "" {
				cur.data = strings.Join(data, "\n")
				select {
				case stream.events <- cur:
				case <-ctx.Done():
					return
				}
			}
			cur, data = sseEvent{}, nil
		case strings.HasPrefix(line, ":"):
			// comment / heartbeat
		case strings.HasPrefix(line, "id: "):
			cur.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			cur.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = append(data, strings.TrimPrefix(line, "data: "))
		}
	}
}
