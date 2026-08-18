package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAddingAKavitaInstance drives ADR-0041's first requirement end to end: a
// Kavita can be added through the real HTTP API, with the real server-enforced
// connection test, and comes out the other side as a `library`-role instance
// with a real health row.
//
// It is a gate test, not a client test — internal/kavita has those. What it
// proves is that the THREE PLACES that refused everything but Prowlarr no longer
// do, and that each of them refuses for the right reason instead:
//
//  1. internal/httpapi's serviceKinds map, which decided the kind AND the role.
//  2. cmd/usarr's `si.Role != "indexer"` refusal in registry.entry(), which used
//     to block a Kavita before a client was ever built — including for the
//     background prober, which would have painted a permanent error row.
//  3. cmd/usarr's `kind != "prowlarr"` refusal in registry.Test().
func TestAddingAKavitaInstance(t *testing.T) {
	// Sequential hex, so `make secrets` stays green without a new waiver, and
	// deliberately different from the Prowlarr fixture key in the other tests.
	const authKey = "0123456789abcdef0123456789abcdef"

	kav := newFakeKavita(t, authKey)
	env := newTestApp(t)

	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	// ── the connection test is mandatory and it PROVES THE KEY ──────────────
	//
	// GET /api/Health answers `Ok` to anybody, so a test that stopped there
	// would store a bad credential and report success. It must not.
	code, body := env.raw(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Bad Key", "base_url": kav.URL(), "api_key": "wrong-key",
	})
	if code != http.StatusBadGateway {
		t.Fatalf("a Kavita with a bad Auth Key must be refused with 502, got %d: %s", code, body)
	}
	if !strings.Contains(body, "Auth Key") {
		t.Errorf("the refusal must name the credential and where to fix it: %s", body)
	}
	t.Logf("bad key refused: %d %s", code, strings.TrimSpace(body))

	// ── the happy path ──────────────────────────────────────────────────────
	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": authKey,
	}, &created)

	if created.Kind != "kavita" {
		t.Errorf("kind = %q", created.Kind)
	}
	// THE ROLE DECISION, asserted rather than commented. Kavita is a catalogue
	// source: replicated FROM, never commanded. `acquisition` would promise a
	// write path that does not exist (ADR-0032).
	if created.Role != "library" {
		t.Errorf("role = %q, want library — a Kavita is a catalogue source, not an indexer "+
			"and not an acquisition app", created.Role)
	}
	if !created.HasCredential {
		t.Error("the stored instance reports no credential")
	}

	// ── the credential never comes back, in any shape ───────────────────────
	_, listBody := env.raw(t, "GET", "/api/v1/services", nil)
	if strings.Contains(listBody, authKey) {
		t.Fatal("the services list returned the Auth Key")
	}
	if strings.Contains(listBody, "/mnt/user") {
		t.Error("a Kavita host filesystem path reached the services list")
	}

	// ── the wire discipline, from the server's side ─────────────────────────
	keys, queries, paths := kav.seen()
	sawHealth, sawLibraries := false, false
	for i, p := range paths {
		switch p {
		case "/api/Health":
			sawHealth = true
			if keys[i] != "" {
				t.Errorf("the anonymous probe carried a key; it must be answerable without one")
			}
		case "/api/Library/libraries":
			sawLibraries = true
		}
		if strings.Contains(queries[i], authKey) {
			t.Errorf("%s carried the Auth Key in the query string: %q", p, queries[i])
		}
		if p == "/api/Plugin/version" {
			t.Error("the connection test called Plugin/version, which puts the credential in a URL")
		}
	}
	if !sawHealth {
		t.Error("the connection test never probed /api/Health, so it cannot tell 'host down' from 'key wrong'")
	}
	if !sawLibraries {
		t.Error("the connection test never called /api/Library/libraries, so it never PROVED the key")
	}

	// ── the BACKGROUND PROBER produces a real health row ────────────────────
	//
	// This is the half of gate (2) that the connection test cannot reach: the
	// wizard's test builds a THROWAWAY client stack, so only the prober goes
	// through registry.entry(), which is where the `si.Role != "indexer"` refusal
	// used to live. With that gate still in place this row would be permanently
	// red with "only indexer services can be searched today" — an instance that
	// was accepted, stored, and then reported broken forever.
	//
	// The screen renders from the LAST PROBE and never from an upstream call, so
	// the test asks for one and waits. A successful connection test asks for one
	// out of band.
	//
	// The predicate WAITS ON EVERY FIELD THE ASSERTIONS BELOW READ, and State is
	// the one that makes that necessary rather than tidy. A health row is
	// assembled from TWO stores that settle at different times: Stale and
	// AppVersion come from registry.probes, the in-memory snapshot, while State
	// is derived from the service_instances row. recordProbe writes the snapshot
	// under probeMu and only THEN issues UpdateServiceInstanceHealth, so between
	// those two writes the screen honestly renders a fresh version on a row whose
	// health_state is still the insert-time "unknown". Waiting on the snapshot
	// half and asserting on the database half broke into that window at ~2% under
	// concurrent -race load: `state=unknown stale=false app_version=0.9.0.20`.
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
		t.Error("the row is stale: the background prober never produced a snapshot for a Kavita")
	}
	if row.State != "healthy" {
		t.Errorf("state = %q problem = %q; a working Kavita must not read as broken", row.State, row.Problem)
	}
	if row.AppVersion != "0.9.0.20" {
		t.Errorf("app_version = %q; the prober reads it from server-info-slim", row.AppVersion)
	}

	// ── searching a library-only install is still refused, and says why ─────
	//
	// The role gate moved from registry.entry() to registry.For(); it did not
	// disappear. This install never reaches either, because internal/httpapi
	// answers first with no_indexer_service — which is the better refusal and is
	// the point: a Kavita is now a storable, probeable instance that simply is
	// not searchable, rather than a row nothing can open.
	code, body = env.raw(t, "GET", "/api/v1/search?query=anything", nil)
	if code == http.StatusOK {
		t.Fatalf("search against a library-only install must not succeed: %s", body)
	}
	t.Logf("search with only a Kavita: %d %s", code, strings.TrimSpace(body))
}

// TestKavitaNonAdminKeyIsNotReportedAsABadCredential is the distinction the
// whole ErrForbidden sentinel exists for, asserted at the level a user meets it.
//
// ServerController is admin-only, so a valid non-admin Auth Key gets a 403 from
// the version endpoint while every read the sync needs succeeds. Failing the
// connection test on that would tell the user to re-enter a key that is fine —
// the single most confusing thing this screen could do.
func TestKavitaNonAdminKeyIsNotReportedAsABadCredential(t *testing.T) {
	const authKey = "0123456789abcdef0123456789abcdef"

	kav := newFakeKavita(t, authKey)
	kav.admin = false

	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": authKey,
	}, &created)

	// It was accepted. Now the message has to be honest about what it could not
	// read, rather than silently reporting a version it never got.
	var res testBody
	env.do(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", created.ID), map[string]any{
		"base_url": kav.URL(),
	}, &res)

	if !res.OK {
		t.Fatalf("a valid non-admin Auth Key must pass the connection test: %+v", res)
	}
	if !res.KeyProvenValid {
		t.Error("the key WAS proven: /api/Library/libraries answered 200, which a bad key cannot do")
	}
	if res.AppVersion != "" {
		t.Errorf("app_version = %q, but server-info-slim answered 403 — a version must not be invented",
			res.AppVersion)
	}
	if !strings.Contains(res.Message, "admin") {
		t.Errorf("the message must say WHY the version is missing, so nobody debugs the key: %q", res.Message)
	}
	t.Logf("non-admin connection test: ok=%v message=%q", res.OK, res.Message)
}

// TestKavitaEmptyLibraryListIsSurfaced covers CLAUDE.md's third principle at the
// one place it is easiest to violate: reachable, authenticated, and nothing to
// replicate must not render as plain success.
func TestKavitaEmptyLibraryListIsSurfaced(t *testing.T) {
	const authKey = "0123456789abcdef0123456789abcdef"

	kav := newFakeKavita(t, authKey)
	kav.libraries = 0

	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": authKey,
	}, &created)

	var res testBody
	env.do(t, "POST", fmt.Sprintf("/api/v1/services/%d/test", created.ID), map[string]any{
		"base_url": kav.URL(),
	}, &res)
	if !strings.Contains(res.Message, "no libraries") && !strings.Contains(res.Message, "NO libraries") {
		t.Errorf("an account that can see nothing must say so rather than reading as success: %q", res.Message)
	}
	t.Logf("empty-library connection test: %q", res.Message)
}

// TestUnknownServiceKindIsStillRefused pins the other half of widening
// serviceKinds: the map got a second entry, not a hole.
func TestUnknownServiceKindIsStillRefused(t *testing.T) {
	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	for _, kind := range []string{"sonarr", "radarr", "komga", "navidrome", ""} {
		code, body := env.raw(t, "POST", "/api/v1/services", map[string]any{
			"kind": kind, "name": "X-" + kind, "base_url": "http://127.0.0.1:9/", "api_key": "k",
		})
		if code != http.StatusBadRequest {
			t.Errorf("kind %q = %d, want 400: %s", kind, code, body)
		}
		// The supported list must be stable across calls: it is rendered from a
		// map, and an error message that reorders itself is one a user cannot
		// search for.
		if !strings.Contains(body, "kavita, prowlarr") {
			t.Errorf("kind %q: the supported list is missing or unsorted: %s", kind, body)
		}
	}
}
