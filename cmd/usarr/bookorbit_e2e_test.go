package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// boTestBody is the connection-test response, with `action` on it.
//
// It is a local shape rather than an extra field on the shared testBody,
// because Action is what every assertion in this file turns on — §17.3 requires
// the ONE button that fixes the problem, and a test that read the message and
// not the action would pass on a result that told the user nothing to do.
type boTestBody struct {
	OK             bool   `json:"ok"`
	Reachable      bool   `json:"reachable"`
	KeyProvenValid bool   `json:"key_proven_valid"`
	AppName        string `json:"app_name"`
	AppVersion     string `json:"app_version"`
	APIVersion     string `json:"api_version"`
	Message        string `json:"message"`
	Action         string `json:"action"`
}

// boMagicLink is the fixture magic-link token.
//
// Sequential hex, which .gitleaks.toml allowlists BY VALUE, and the same
// discipline the Kavita fixture keeps: a realistic-looking token would trip the
// generic-api-key rule, and the file's own instruction for that case is to make
// the fixture obviously fake rather than to extend the allowlist.
const boMagicLink = "0123456789abcdef0123456789abcdef"

// boSetUp builds an app with a signed-in owner. Every test here needs it and
// none of them is about it.
func boSetUp(t *testing.T) *testEnv {
	t.Helper()
	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)
	return env
}

// TestAddingABookOrbitInstance is the whole of this slice, end to end: a
// BookOrbit can be ADDED through the real HTTP API, with the real
// server-enforced connection test, and comes out as a `library`-role instance
// with an encrypted credential and a real health row.
//
// That is the thing that was impossible before. internal/bookorbit shipped a
// client, headless magic-link authentication and a scope grader at 568ddbc, and
// none of it was reachable: `bookorbit` was in no service-kind map, so the
// credential could not be stored, so the client could never be built. This test
// is the gate on the three places that had to learn the kind:
//
//  1. internal/httpapi's serviceKinds map, which decides the kind AND the role.
//  2. cmd/usarr's registry.entry(), which builds the client stack — and which the
//     BACKGROUND PROBER reaches, unlike the connection test.
//  3. cmd/usarr's registry.Test(), whose default arm refused every unknown kind.
func TestAddingABookOrbitInstance(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	env := boSetUp(t)

	// ── the connection test is mandatory and it PROVES THE TOKEN ────────────
	//
	// GET /api/v1/health is @Public() and answers 200 to anybody, so a test that
	// stopped at reachability would store a dead credential behind a green tick.
	code, body := env.raw(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "Bad Token", "base_url": bo.URL(),
		"api_key": "0000000000000000000000000000dead",
	})
	if code != http.StatusBadGateway {
		t.Fatalf("a BookOrbit with a bad magic-link token must be refused with 502, got %d: %s", code, body)
	}
	if !strings.Contains(strings.ToLower(body), "magic link") &&
		!strings.Contains(strings.ToLower(body), "magic-link") {
		t.Errorf("the refusal must name the credential and how to replace it: %s", body)
	}
	t.Logf("bad token refused: %d %s", code, strings.TrimSpace(body))

	// ── the happy path ──────────────────────────────────────────────────────
	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)

	if created.Kind != "bookorbit" {
		t.Errorf("kind = %q", created.Kind)
	}
	// THE ROLE DECISION, asserted rather than commented. BookOrbit is a
	// catalogue source: replicated FROM, never commanded. `acquisition` would
	// promise a write path that does not exist.
	if created.Role != "library" {
		t.Errorf("role = %q, want library — a BookOrbit is a catalogue source, not an indexer "+
			"and not an acquisition app", created.Role)
	}
	if !created.HasCredential {
		t.Error("the stored instance reports no credential: the magic-link token was not sealed")
	}

	// ── THE CREDENTIAL NEVER COMES BACK, IN ANY SHAPE ───────────────────────
	//
	// Two secrets, not one, and the second is the reason this assertion is not a
	// copy of the Kavita one. BookOrbit hands an ACCESS TOKEN back in a response
	// BODY — the direction a request-side redactor was never written for — and
	// that bearer is as good as the magic link for fifteen minutes. Neither may
	// reach a browser, and neither may reach the row a browser reads.
	_, listBody := env.raw(t, "GET", "/api/v1/services", nil)
	if strings.Contains(listBody, boMagicLink) {
		t.Fatal("the services list returned the magic-link token")
	}
	for i, tok := range bo.issuedTokens() {
		if strings.Contains(listBody, tok) {
			t.Fatalf("the services list returned issued access token %d", i)
		}
	}
	// AccountView is an allowlist and these two are the fields it deliberately
	// leaves off: an operator's address, and an open settings blob.
	if strings.Contains(listBody, "shared@example.invalid") {
		t.Error("the BookOrbit account's email address reached the services list")
	}
	if strings.Contains(listBody, `"theme"`) {
		t.Error("the BookOrbit account's settings blob reached the services list")
	}

	// The connection test's OWN response is the third browser-facing surface,
	// and the one where a credential is actually in scope in the handler that
	// writes it — testBookOrbit holds the token in a local variable while it
	// builds the message a browser will render.
	_, retestBody := env.raw(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", created.ID),
		map[string]any{"base_url": bo.URL()})
	if strings.Contains(retestBody, boMagicLink) {
		t.Fatal("the connection-test response returned the magic-link token")
	}
	for i, tok := range bo.issuedTokens() {
		if strings.Contains(retestBody, tok) {
			t.Fatalf("the connection-test response returned issued access token %d", i)
		}
	}

	_, healthBody := env.raw(t, "GET", "/api/v1/services/health", nil)
	if strings.Contains(healthBody, boMagicLink) {
		t.Fatal("the services health screen returned the magic-link token")
	}
	for i, tok := range bo.issuedTokens() {
		if strings.Contains(healthBody, tok) {
			t.Fatalf("the services health screen returned issued access token %d", i)
		}
	}

	// ── the wire discipline, from the SERVER's side ─────────────────────────
	//
	// ADR-0052's constraint is that the credential never reaches a URL. This is
	// the half that can only be checked from the other end of the socket.
	reqs := bo.requests()
	sawHealth, sawLogin, sawAppInfo, tokenBodies := false, false, false, 0
	for _, r := range reqs {
		if strings.Contains(r.Query, boMagicLink) || strings.Contains(r.Path, boMagicLink) {
			t.Errorf("%s %s carried the magic-link token in the URL: %q %q", r.Method, r.Path, r.Path, r.Query)
		}
		if strings.Contains(r.Auth, boMagicLink) {
			t.Errorf("%s %s put the magic-link token in the Authorization header; only the "+
				"short-lived bearer belongs there", r.Method, r.Path)
		}
		if strings.Contains(r.Body, boMagicLink) {
			tokenBodies++
		}
		switch r.Path {
		case "/api/v1/health":
			sawHealth = true
			if r.Auth != "" {
				t.Errorf("the @Public() reachability probe carried a credential: %q", r.Auth)
			}
		case "/api/v1/auth/magic-links/login":
			sawLogin = true
		case "/api/v1/app-info":
			sawAppInfo = true
			if !strings.HasPrefix(r.Auth, "Bearer ") {
				t.Errorf("the authenticated read carried %q, not a bearer", r.Auth)
			}
		default:
			t.Errorf("the connection test called %s %s, which is outside the /api/v1 routes "+
				"ADR-0052 constrains this adapter to", r.Method, r.Path)
		}
	}
	if !sawHealth {
		t.Error("the test never probed /api/v1/health, so it cannot tell 'host down' from 'token wrong'")
	}
	if !sawLogin {
		t.Error("the test never called the magic-link login route, so it never PROVED the token")
	}
	if !sawAppInfo {
		t.Error("the test never made an authenticated read, so it never proved the bearer is accepted")
	}
	if tokenBodies == 0 {
		t.Error("the magic-link token appeared in no request body at all, which cannot be right")
	}
	t.Logf("bookorbit wire: %d requests, token in %d bodies, 0 URLs, 0 headers", len(reqs), tokenBodies)

	// ── the BACKGROUND PROBER produces a real health row ────────────────────
	//
	// This is the half the connection test cannot reach: Test() builds a
	// THROWAWAY client stack, so only the prober goes through registry.entry().
	// A kind registered in httpapi and not in the registry would be accepted,
	// stored, and then reported permanently broken by a goroutine nobody watches.
	//
	// The predicate waits on every field asserted below, for the reason the
	// Kavita test spells out: the snapshot and the row settle at different times.
	var row serviceHealthBody
	for range 100 {
		var health servicesHealthBody
		env.do(t, "GET", "/api/v1/services/health", nil, &health)
		if len(health.Services) != 1 {
			t.Fatalf("expected one service row, got %d", len(health.Services))
		}
		row = health.Services[0]
		if !row.Stale && row.State == "healthy" && row.AppVersion != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("services screen: %s state=%s stale=%v app_version=%s problem=%q",
		row.Name, row.State, row.Stale, row.AppVersion, row.Problem)
	if row.Stale {
		t.Error("the row is stale: the background prober never produced a snapshot for a BookOrbit")
	}
	if row.State != "healthy" {
		t.Errorf("state = %q problem = %q; a working BookOrbit must not read as broken", row.State, row.Problem)
	}
	if row.AppVersion != "v1.4.2" {
		t.Errorf("app_version = %q; the prober reads it from the authenticated app-info read", row.AppVersion)
	}
	if len(row.Warnings) != 0 {
		t.Errorf("a correctly scoped shared account must raise no warning: %+v", row.Warnings)
	}

	// ── the catalogue-source gate now PASSES for this kind ──────────────────
	//
	// ⚠️ THIS BLOCK USED TO ASSERT THE OPPOSITE, and the assertion was correct
	// when it was written: "a catalogue sync from a BookOrbit must not claim to
	// have started … error = not_a_catalogue_source", because internal/libsync
	// had no BookOrbit source. It has one now, so the refusal it pinned would be
	// a bug rather than a property.
	//
	// This env deliberately does not arm imports (armBootstrapImport is the
	// switch, and only the import tests throw it), so the honest assertion here
	// is the NARROW one: whatever answer comes back, it is no longer
	// "not_a_catalogue_source". The import itself is proved in
	// TestAddingABookOrbitProducesACatalogue, against a real database.
	code, body = env.raw(t, "POST", fmt.Sprintf("/api/v1/services/%d/sync", created.ID), map[string]any{})
	if code == http.StatusNotFound {
		t.Fatalf("the sync endpoint moved; this assertion is now testing nothing: %d %s", code, body)
	}
	if code != http.StatusOK && code != http.StatusAccepted {
		var syncErr errorBodyShape
		mustUnmarshal(t, body, &syncErr)
		if syncErr.Error == "not_a_catalogue_source" {
			t.Errorf("a BookOrbit was refused as not a catalogue source, which stopped being true "+
				"when internal/libsync/bookorbit.go landed: %s", body)
		}
		// And whatever the refusal is, it must never tell a user their books do
		// not exist.
		if strings.Contains(syncErr.Message, "carries no catalogue") {
			t.Errorf("the refusal claims the service has no catalogue, which is false of a BookOrbit: %q",
				syncErr.Message)
		}
	}
	t.Logf("sync from a BookOrbit: %d %s", code, strings.TrimSpace(body))
}

// TestBookOrbitHostEditForcesCredentialReentry is reference/security.md §1.6 at
// the level a user meets it, on the kind this commit adds.
//
// The attack it stops: point base_url at a listener you control, press Test, and
// read the credential off your own log. It is enforced SERVER-SIDE — a UI that
// merely masks the field is cosmetic against it — and it holds even though the
// stored ciphertext could not be opened for the new host anyway, because the
// failure mode of trying is sending a live credential to whatever the user just
// typed.
func TestBookOrbitHostEditForcesCredentialReentry(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	evil := newFakeBookOrbit(t, "0000000000000000000000000000dead")
	env := boSetUp(t)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)

	before := len(evil.requests())

	// (a) A SAVE that moves the host with no credential in the body.
	code, body := env.raw(t, "PATCH", fmt.Sprintf("/api/v1/services/%d", created.ID),
		map[string]any{"base_url": evil.URL()})
	if code != http.StatusBadRequest {
		// Errorf, not Fatalf: the assertion that MATTERS is the one below, that
		// the attacker-controlled host was never contacted. Stopping here would
		// hide the leak behind the status code that was supposed to prevent it.
		t.Errorf("moving base_url with no re-entered token must be refused with 400, got %d: %s", code, body)
	}
	var errBody errorBodyShape
	mustUnmarshal(t, body, &errBody)
	if errBody.Error != "credential_reentry_required" {
		t.Errorf("error = %q, want credential_reentry_required; the SPA branches on this code", errBody.Error)
	}
	t.Logf("host edit refused: %d error=%s action=%q", code, errBody.Error, errBody.Action)

	// (b) A TEST against the moved host with no credential in the body. This is
	// the endpoint the attack actually uses: it is reachable before any save.
	code, body = env.raw(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", created.ID),
		map[string]any{"base_url": evil.URL()})
	if code != http.StatusBadRequest {
		t.Errorf("testing a moved host with no re-entered token must be refused with 400, got %d: %s", code, body)
	}
	// A FRESH struct, not the one above. mustUnmarshal leaves absent keys at
	// their previous value, so reusing it would let a 200 with no `error` key
	// inherit the refusal code from the assertion before it — the guard would
	// read green while reporting a leak two lines further down.
	var testErr errorBodyShape
	mustUnmarshal(t, body, &testErr)
	if testErr.Error != "credential_reentry_required" {
		t.Errorf("error = %q, want credential_reentry_required: %s", testErr.Error, body)
	}
	t.Logf("host-edit test refused: %d error=%s action=%q", code, testErr.Error, testErr.Action)

	// THE POINT OF THE RULE, asserted rather than trusted: the host the user
	// just typed was never contacted at all, so it never had a chance to see a
	// credential.
	for _, r := range evil.requests()[before:] {
		leaked := ""
		if strings.Contains(r.Body, boMagicLink) {
			leaked = " AND IT CARRIED THE STORED MAGIC-LINK TOKEN"
		}
		t.Errorf("the attacker-controlled host was contacted%s: %s %s auth=%q",
			leaked, r.Method, r.Path, r.Auth)
	}

	// And the instance is unchanged: a refused edit must not half-apply.
	var still serviceBody
	env.do(t, "GET", fmt.Sprintf("/api/v1/services/%d", created.ID), nil, &still)
	if still.BaseURL != created.BaseURL {
		t.Errorf("base_url moved to %q despite the refusal", still.BaseURL)
	}
}

// TestBookOrbitOverScopedAccountIsWarnedAboutAndStILLStored is the judgement
// this slice makes deliberately, pinned so it cannot drift into either extreme.
//
// The credential is a magic-link token minted against a shared account, and the
// grader can find that the account is a superuser or holds write permissions.
// The client's standing decision is to REPORT AND WARN rather than refuse
// (internal/bookorbit/doc.go), and this test says the connection test agrees:
// the service is stored and usable, and the user is told, in the panel, with the
// one action that narrows it.
//
// Both halves matter. A refusal would leave a user with a working token and no
// way to use it, for a fix entirely on BookOrbit's side. A silent pass would
// store a full-admin credential with no one told.
func TestBookOrbitOverScopedAccountIsWarnedAboutAndStillStored(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.user["isSuperuser"] = true
	bo.user["permissions"] = []string{"users:manage", "libraries:manage"}

	env := boSetUp(t)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)
	if !created.HasCredential {
		t.Fatal("an over-scoped account must still be STORED: refusing a working credential fixes nothing")
	}

	var res boTestBody
	env.do(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", created.ID),
		map[string]any{"base_url": bo.URL()}, &res)

	if !res.OK {
		t.Fatalf("an over-scoped but working credential must PASS the test: %+v", res)
	}
	if !res.KeyProvenValid {
		t.Error("the token WAS proven: the login route accepted it, which a bad token cannot do")
	}
	if !strings.Contains(res.Message, "OVER-SCOPED") {
		t.Errorf("the verdict must reach the user, not just the client: %q", res.Message)
	}
	if !strings.Contains(strings.ToLower(res.Action), "no permissions") {
		t.Errorf("§17.3 wants the ONE action that fixes it; got %q", res.Action)
	}
	// The findings are BookOrbit's vocabulary and UsArr's prose. Nothing that
	// names the human behind the account may ride along.
	if strings.Contains(res.Message, "usarr") && strings.Contains(res.Message, "username") {
		t.Errorf("the scope note named the account: %q", res.Message)
	}
	t.Logf("over-scoped connection test: ok=%v action=%q\n  message=%s", res.OK, res.Action, res.Message)

	// It is repeated on the Services screen too, so it does not evaporate with
	// the panel the user closed ten seconds later.
	var warned bool
	for range 100 {
		var health servicesHealthBody
		env.do(t, "GET", "/api/v1/services/health", nil, &health)
		if len(health.Services) == 1 && !health.Services[0].Stale {
			for _, w := range health.Services[0].Warnings {
				if strings.Contains(w.Message, "OVER-SCOPED") {
					warned = true
					t.Logf("services screen warning: [%s] %s", w.Source, w.Message)
				}
			}
			if warned || health.Services[0].State != "unknown" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !warned {
		t.Error("the Services screen never repeated the §14 scope warning")
	}
}

// TestBookOrbitRefusedBearerFailsTheTestWithoutBlamingTheToken is the honesty
// requirement at its sharpest.
//
// A magic-link token can be perfectly valid and the account behind it still
// unable to make one ordinary call: JwtAuthGuard 403s every route that is not
// @AllowDefaultPassword when user.isDefaultPassword, and the login route is
// @Public() so the guard never ran there. A connection test that stopped at the
// successful mint would show a green tick over an adapter that can do nothing.
//
// So it FAILS — and it still reports key_proven_valid, because the stored token
// is not the thing that is wrong and telling the user to re-paste it would send
// them in exactly the wrong direction.
func TestBookOrbitRefusedBearerFailsTheTestWithoutBlamingTheToken(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.appInfoStatus = http.StatusForbidden
	bo.appInfoMessage = "Password change required"

	env := boSetUp(t)

	code, body := env.raw(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	})
	if code != http.StatusBadGateway {
		t.Fatalf("a bearer refused on app-info must fail the connection test, got %d: %s", code, body)
	}
	var errBody errorBodyShape
	mustUnmarshal(t, body, &errBody)
	if !strings.Contains(errBody.Message, "app-info") {
		t.Errorf("the message must name the call that was refused: %q", errBody.Message)
	}
	if !strings.Contains(strings.ToLower(errBody.Action), "shared") {
		t.Errorf("the action must point at the account, not at the token: %q", errBody.Action)
	}
	if strings.Contains(strings.ToLower(errBody.Action), "mint a new magic-link token") {
		t.Errorf("the action blamed the token, which is the one thing that is fine: %q", errBody.Action)
	}
	t.Logf("refused bearer: %d\n  message=%s\n  action=%s", code, errBody.Message, errBody.Action)
}

// TestBookOrbitFailingHealthIndicatorIsCarriedNotSwallowed covers the one place
// this adapter is better off than Kavita's, and the place it is easiest to throw
// away.
//
// Terminus answers 503 with a FULLY FORMED report naming the indicator that is
// down. That is a diagnosis, not a failure to connect, so the test keeps going —
// a failing disk indicator does not stop a login — and the diagnosis is carried
// into the result instead of being replaced by "the service did not answer".
func TestBookOrbitFailingHealthIndicatorIsCarriedNotSwallowed(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.healthStatus = http.StatusServiceUnavailable
	bo.health = map[string]any{
		"status": "error",
		"info":   map[string]any{},
		"error": map[string]any{
			"storage": map[string]any{"status": "down", "message": "disk usage is above the threshold"},
		},
		"details": map[string]any{
			"storage": map[string]any{"status": "down", "message": "disk usage is above the threshold"},
		},
	}

	env := boSetUp(t)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)

	var res boTestBody
	env.do(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", created.ID),
		map[string]any{"base_url": bo.URL()}, &res)

	if !res.OK {
		t.Fatalf("a reachable BookOrbit with a failing indicator must not read as unreachable: %+v", res)
	}
	if !strings.Contains(res.Message, "storage") {
		t.Errorf("the failing indicator is the diagnosis and must survive into the message: %q", res.Message)
	}
	t.Logf("degraded-but-connected: ok=%v message=%s", res.OK, res.Message)

	// And it reaches the Services screen as a warning on a row that stays
	// otherwise honest, rather than as a red row or as nothing.
	var found string
	for range 100 {
		var health servicesHealthBody
		env.do(t, "GET", "/api/v1/services/health", nil, &health)
		if len(health.Services) == 1 && !health.Services[0].Stale {
			for _, w := range health.Services[0].Warnings {
				if strings.Contains(w.Message, "storage") {
					found = w.Message
				}
			}
			if found != "" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if found == "" {
		t.Error("the failing health indicator never reached the Services screen")
	}
	t.Logf("services screen warning: %s", found)
}
