package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/httpapi"
)

// Re-running a catalogue import, end to end through the real binary.
//
// THE BUG THESE COVER. The import had one trigger — a bootstrap gated on
// last_full_sync_at — so an instance that had imported once could never import
// again for the life of the database. Every later fix to what an import WRITES
// was therefore undeliverable to rows already imported, which is not a
// hypothetical: work.year is written by the credit pass, and 151 real series
// were sitting on a NULL that nothing could ever fill.

// syncNow presses "Run full sync now" for one instance and returns the status
// and body, without failing on a non-2xx — every test here is about which
// non-2xx it is.
func syncNow(t *testing.T, env *testEnv, instanceID int64) (int, string) {
	t.Helper()
	return env.raw(t, http.MethodPost, fmt.Sprintf("/api/v1/services/%d/sync", instanceID), map[string]any{})
}

func setUpOwner(t *testing.T, env *testEnv) {
	t.Helper()
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)
}

// waitFor polls until cond holds, and fails with why rather than hanging.
func waitFor(t *testing.T, why string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", why)
}

// The whole deliverable, in the shape the bug had: an instance that has ALREADY
// completed an import is imported again on request, and the second import
// rewrites the rows the first one wrote.
//
// The year is the payload rather than the subject. The first import runs against
// an upstream with no releaseYear, exactly as the real one did; the second runs
// after the upstream grew one, and the row has to move. A re-import that read the
// library again and left the row alone would pass a "was it called twice" test
// and fail this one.
func TestACompletedImportCanBeRunAgainAndRewritesItsRows(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = []map[string]any{kavitaSeries(41, 1, "Frieren", nil)}
	// No releaseYear: the state the 151 real rows were in.
	kav.metadata = map[int]map[string]any{
		41: kavitaMetadata(41, map[string][]string{"writers": {"Kanehito Yamada"}}),
	}

	env := newTestApp(t)
	setUpOwner(t, env)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)
	waitForImport(t, env, created.ID)

	if got := yearOf(t, env, "Frieren"); got.Valid {
		t.Fatalf("year = %v before the fix landed upstream, want NULL", got.Int64)
	}
	// The upstream — or UsArr's mapping of it — now has the year.
	kav.setMetadata(map[int]map[string]any{
		41: kavitaMetadata(41, map[string][]string{"writers": {"Kanehito Yamada"}},
			map[string]any{"releaseYear": 2016}),
	})

	code, body := syncNow(t, env, created.ID)
	if code != http.StatusAccepted {
		t.Fatalf("POST sync = %d, want 202: %s", code, body)
	}
	if !strings.Contains(body, `"status":"started"`) {
		t.Errorf("the 202 body must say what happened: %s", body)
	}

	// The EFFECT is what is waited on, not a timestamp: last_full_sync_at is
	// second-resolution, and two fixture imports inside one second write the
	// same string.
	waitFor(t, "the re-import to repair work.year", func() bool {
		got := yearOf(t, env, "Frieren")
		return got.Valid && got.Int64 == 2016
	})
	if n := kav.countPath("/api/Series/all-v2"); n != 2 {
		t.Errorf("the series list was read %d times across two imports, want 2", n)
	}
	// One work, not two: a re-import updates, it does not duplicate.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind <> 'person'`); n != 1 {
		t.Errorf("catalogue works after two imports = %d, want 1", n)
	}

	// AND THE CLAIM COMES BACK. An import started through this endpoint holds
	// the guard for its whole run, so the release is the endpoint's own code
	// path rather than FullImport's defer — and a leak there would make the
	// second press the last press this process ever accepts. Pressing until it
	// answers 202 is the only way to observe it; a 409 here just means the
	// second import has not finished yet.
	waitFor(t, "the claim from the requested import to be released", func() bool {
		code, _ := syncNow(t, env, created.ID)
		return code == http.StatusAccepted
	})
}

// THE CONCURRENCY GUARD, watched failing rather than asserted.
//
// The upstream is parked inside the series read, so the first import is provably
// still running when the second is asked for. That is the double-click, the
// impatient retry, and a scheduled call landing on a running import — all three
// are this.
func TestASecondImportIsRefusedWhileTheFirstIsStillRunning(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = []map[string]any{kavitaSeries(41, 1, "Frieren", nil)}
	kav.holdSeriesList()

	env := newTestApp(t)
	setUpOwner(t, env)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)

	// The bootstrap import is now parked inside the series read.
	waitFor(t, "the first import to reach the series list", func() bool {
		return kav.countPath("/api/Series/all-v2") == 1
	})

	code, body := syncNow(t, env, created.ID)
	if code != http.StatusConflict {
		t.Fatalf("a second import while one is running = %d, want 409: %s", code, body)
	}
	if !strings.Contains(body, `"error":"import_in_progress"`) {
		t.Errorf("the refusal must be legible on the wire as its own code, not a generic "+
			"conflict and not a false success: %s", body)
	}
	// It refused, and it refused WITHOUT touching the upstream a second time.
	if n := kav.countPath("/api/Series/all-v2"); n != 1 {
		t.Errorf("the series list was read %d times while one import was running, want 1", n)
	}

	kav.releaseSeriesList()
	waitForImport(t, env, created.ID)

	// And the claim is RELEASED: the same press now starts an import.
	code, body = syncNow(t, env, created.ID)
	if code != http.StatusAccepted {
		t.Fatalf("a sync after the running import finished = %d, want 202: %s", code, body)
	}
	waitFor(t, "the second import to reach the series list", func() bool {
		return kav.countPath("/api/Series/all-v2") == 2
	})
}

// The same guard under simultaneous pressure, which is the case a sequential
// test cannot reach: N callers arriving at once, none of them ordered behind
// another. Exactly one may start.
func TestOnlyOneOfManySimultaneousImportsStarts(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = []map[string]any{kavitaSeries(41, 1, "Frieren", nil)}

	env := newTestApp(t)
	setUpOwner(t, env)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)
	// Let the bootstrap finish first, so the only imports racing below are the
	// ones this test starts.
	waitForImport(t, env, created.ID)
	before := kav.countPath("/api/Series/all-v2")
	kav.holdSeriesList()

	const callers = 8
	var wg sync.WaitGroup
	results := make([]error, callers)
	start := make(chan struct{})
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = env.app.registry.StartImport(created.ID)
		}()
	}
	close(start)
	wg.Wait()

	started, refused := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			started++
		case errors.Is(err, httpapi.ErrImportInProgress):
			refused++
		default:
			t.Fatalf("StartImport returned an unexpected error: %v", err)
		}
	}
	if started != 1 || refused != callers-1 {
		t.Fatalf("%d of %d simultaneous imports started (%d refused), want exactly 1 — "+
			"two writers over one instance is what the guard exists to prevent",
			started, callers, refused)
	}

	waitFor(t, "the one winner to reach the series list", func() bool {
		return kav.countPath("/api/Series/all-v2") == before+1
	})
	kav.releaseSeriesList()
	waitForImport(t, env, created.ID)
	if n := kav.countPath("/api/Series/all-v2") - before; n != 1 {
		t.Errorf("%d series-list reads followed %d simultaneous presses, want 1", n, callers)
	}
}

// A Prowlarr has no catalogue, and the answer says so BEFORE anything is
// started — a caller that pressed the wrong row must not be told "started".
func TestSyncingANonCatalogueServiceIsRefusedByItsOwnCode(t *testing.T) {
	prow := newFakeProwlarr(t, importProwlarrKey)
	env := newTestApp(t)
	setUpOwner(t, env)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prow.URL(),
		"api_key": importProwlarrKey,
	}, &created)

	code, body := syncNow(t, env, created.ID)
	if code != http.StatusConflict || !strings.Contains(body, `"error":"not_a_catalogue_source"`) {
		t.Fatalf("sync of a Prowlarr = %d: %s\nwant 409 not_a_catalogue_source", code, body)
	}
}

// Unknown to this user is 404, and it is the SCOPED read that decides — not a
// bare existence check.
func TestSyncingAnUnknownInstanceIs404(t *testing.T) {
	env := newTestApp(t)
	setUpOwner(t, env)

	code, body := syncNow(t, env, 4242)
	if code != http.StatusNotFound || !strings.Contains(body, `"error":"not_found"`) {
		t.Fatalf("sync of an unknown instance = %d: %s\nwant 404 not_found", code, body)
	}
}

// The 202 body is an allowlist. service_instance carries an encrypted
// full-admin credential and a base URL; neither is in the response, and neither
// is anything else that was not chosen field by field.
func TestTheSyncResponseCarriesOnlyItsFourChosenFields(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1

	env := newTestApp(t)
	setUpOwner(t, env)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)
	waitForImport(t, env, created.ID)

	code, body := syncNow(t, env, created.ID)
	if code != http.StatusAccepted {
		t.Fatalf("POST sync = %d, want 202: %s", code, body)
	}
	var got map[string]any
	mustUnmarshal(t, body, &got)
	want := map[string]bool{"status": true, "instance_id": true, "kind": true, "name": true}
	for k := range got {
		if !want[k] {
			t.Errorf("the 202 body carries %q, which is not one of the four allowlisted fields: %s", k, body)
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("the 202 body is missing %q: %s", k, body)
		}
	}
	assertNoSecret(t, "sync response", body, importAuthKey, kav.URL())
}

// yearOf reads one work's year, keeping NULL distinguishable from 0.
func yearOf(t *testing.T, env *testEnv, title string) sql.NullInt64 {
	t.Helper()
	var year sql.NullInt64
	err := env.app.store.DB().Read().
		QueryRowContext(t.Context(), `SELECT year FROM work WHERE title = ?`, title).Scan(&year)
	if err != nil {
		t.Fatalf("read year for %q: %v", title, err)
	}
	return year
}
