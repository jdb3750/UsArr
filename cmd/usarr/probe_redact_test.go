package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// TestProbeErrorIsRedactedBeforeItIsStored is the guard for LS-170 step 1.
//
// The health prober is the one path on which an UPSTREAM ERROR STRING becomes a
// stored row: registry.probe writes health.Error into the in-memory snapshot the
// Services screen renders and into service_instance.last_error via
// UpdateServiceInstanceHealth. internal/servarr deliberately keeps URLs out of
// its own error envelope (APIError.Path is a path, transportError reconstructs
// *url.Error), but it copies the *Arr's error MESSAGE through verbatim — and that
// message is upstream free text which routinely quotes the indexer URL the *Arr
// failed on, passkey and all. So the redaction has to happen in cmd/usarr, before
// the value reaches the row.
//
// WHY THIS MATTERS MORE THAN A LOG LINE: last_error is persisted, so a credential
// that lands there survives in every VACUUM INTO backup taken afterwards.
// Redacting on read-out does not reach the bytes already on disk.
//
// The test drives the REAL probe against the Prowlarr double over real HTTP —
// registry.probe is the function that performs the assignment — rather than
// asserting on a string it built itself.
//
// It fires on the `entry.prowlarr.SystemStatus` branch. The two sibling
// assignments in the same file (the entry() failure above it, and probeKavita's)
// are the same one-line rule; the Kavita one is defence in depth behind
// internal/kavita's own clean() and cannot be made to leak from here.
func TestProbeErrorIsRedactedBeforeItIsStored(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"

	prowlarr := newFakeProwlarr(t, apiKey)
	env := newTestApp(t)

	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	var svc serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prowlarr.URL(), "api_key": apiKey,
	}, &svc)
	if svc.ID == 0 {
		t.Fatalf("no service row was created: %+v", svc)
	}

	// What a real *Arr says when one of its indexers refuses it: its own message,
	// quoting the tracker URL with the user's personal passkey in the query.
	upstreamMessage := fmt.Sprintf(
		"Indexer 'Working Tracker' failed: https://tracker.example/rss?passkey=%s&limit=100 returned 403",
		fixtureIndexerPasskey)
	prowlarr.failStatusWith(upstreamMessage)

	env.app.registry.probe(context.Background(), svc.ID)

	snap, ok := env.app.registry.Snapshot(svc.ID)
	if !ok {
		t.Fatal("the probe recorded no snapshot at all")
	}
	if snap.Error == "" {
		t.Fatalf("the probe reported no error, so nothing was under test: %+v", snap)
	}
	// Not vacuous: the upstream message really did reach the value being
	// asserted on, and the user still gets a usable error rather than a
	// placeholder (ARCHITECTURE.md §17.3 renders this string).
	if !strings.Contains(snap.Error, "tracker.example") || !strings.Contains(snap.Error, "403") {
		t.Fatalf("the upstream message did not reach health.Error, so this test proves nothing: %q", snap.Error)
	}
	if !strings.Contains(snap.Error, "REDACTED") {
		t.Errorf("health.Error kept a credential-bearing URL unredacted: %q", snap.Error)
	}
	assertNoSecret(t, "the probe snapshot", snap.Error, fixtureIndexerPasskey)

	// And the persisted row, which is the one that outlives the process.
	si, err := env.app.store.GetServiceInstance(context.Background(),
		store.Scope{AllInstances: true}, svc.ID)
	if err != nil {
		t.Fatalf("re-reading the service row: %v", err)
	}
	if !si.LastError.Valid || si.LastError.String == "" {
		t.Fatalf("service_instance.last_error was not written, so nothing was persisted to check")
	}
	assertNoSecret(t, "service_instance.last_error", si.LastError.String, fixtureIndexerPasskey)

	// The same string on the way to the browser, which is where a user reads it.
	var health servicesHealthBody
	env.do(t, "GET", "/api/v1/services/health", nil, &health)
	for _, row := range health.Services {
		assertNoSecret(t, "GET /api/v1/services/health", row.Problem+" "+row.Action, fixtureIndexerPasskey)
	}
	t.Logf("stored last_error: %s", si.LastError.String)
}
