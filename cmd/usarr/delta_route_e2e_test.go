package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/libsync"
	"github.com/jdb3750/UsArr/internal/store"
)

// POST /api/v1/services/{id}/sync/delta, end to end through the real binary.
//
// WHAT ONLY THIS FILE CAN SHOW. internal/httpapi's table drives the handler over
// a fake port, which proves the mapper and nothing about the wiring: a fake that
// returns ErrNotDeltaSource proves a 409 is produced, never that anything in the
// real process ever produces that sentinel. The capability answer lives at a
// DeltaSource type assertion inside internal/libsync, three packages down, and
// the whole point of the pre-flight in registry.StartDeltaSync is to reach that
// answer BEFORE a 202 is written. Only a request against a real adapter over a
// real client stack shows that it does.

// deltaSyncNow presses "run a delta sync" for one instance and returns the status
// and body, without failing on a non-2xx — most tests here are about which
// non-2xx it is.
func deltaSyncNow(t *testing.T, env *testEnv, instanceID int64) (int, string) {
	t.Helper()
	return env.raw(t, http.MethodPost,
		fmt.Sprintf("/api/v1/services/%d/sync/delta", instanceID), map[string]any{})
}

// 🚩 THE 409 IS REAL, AND THIS IS THE PROOF. A Kavita has a catalogue UsArr
// imports today and no delta channel: its own 3b would be a client-side stop over
// a sorted list, because Kavita's filter vocabulary carries no timestamp member,
// so the adapter declines to implement the delta half rather than implement a
// lie.
//
// Both halves are asserted together on ONE instance, and they have to be:
//
//   - The ENGINE refuses with libsync.ErrNoDeltaChannel, which is the assertion
//     inside Importer.DeltaSync answering. That is the condition the wire code
//     stands for.
//   - The ROUTE refuses with 409 not_a_delta_source, synchronously, WITHOUT
//     having written a 202 first — which is only possible because the pre-flight
//     asks the same question before it launches anything.
//
// Assert only the first and the route could still answer 202 and refuse into a
// log line; assert only the second and the pre-flight could be answering a
// question the engine no longer asks.
func TestADeltaSyncOfASourceWithNoDeltaChannelIsRefusedByItsOwnCode(t *testing.T) {
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
	waitForImport(t, env, created.ID)

	// The engine's own answer, taken first so a failure here is not misread as a
	// failure of the route.
	_, err := env.app.registry.DeltaSync(t.Context(), created.ID)
	if !errors.Is(err, libsync.ErrNoDeltaChannel) {
		t.Fatalf("the engine's delta of a Kavita = %v, want libsync.ErrNoDeltaChannel — "+
			"if this changed, the wire code below stands for nothing", err)
	}

	code, body := deltaSyncNow(t, env, created.ID)
	if code != http.StatusConflict || !strings.Contains(body, `"error":"not_a_delta_source"`) {
		t.Fatalf("a delta sync of a Kavita = %d: %s\nwant 409 not_a_delta_source — a 202 here "+
			"means the refusal was left to the walk, where nobody can read it", code, body)
	}
	// It must name the way out. The service is FINE; a full sync of this very row
	// works, and that is what the user should press.
	if !strings.Contains(body, `"action":"Run a full sync instead"`) {
		t.Errorf("the refusal names no way out: %s", body)
	}
	// ⚠️ AND THE STORAGE VOCABULARY MUST NOT BE ON THE WIRE. internal/libsync's
	// errorClass DECLARES "no_delta_channel" for this same condition — declares,
	// not writes: DeltaSync returns at its DeltaSource assertion before any
	// recordDeltaWalk, so nothing lands that class in sync_report.detail today.
	// The string is one edit from being written, DEVELOPMENT.md §11 forbids the
	// two sets sharing a term, and this is the only place a collision could
	// surface — so the guard is kept ahead of the collision rather than after it.
	if strings.Contains(body, "no_delta_channel") {
		t.Errorf("the wire body carries internal/libsync's STORED class: %s", body)
	}
}

// A Prowlarr has no catalogue at all, and that is refused from SQLite alone —
// before the port, before a client stack, before anything upstream.
func TestADeltaSyncOfANonCatalogueServiceIsRefusedByItsOwnCode(t *testing.T) {
	prow := newFakeProwlarr(t, importProwlarrKey)
	env := newTestApp(t)
	setUpOwner(t, env)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prow.URL(),
		"api_key": importProwlarrKey,
	}, &created)

	code, body := deltaSyncNow(t, env, created.ID)
	if code != http.StatusConflict || !strings.Contains(body, `"error":"not_a_catalogue_source"`) {
		t.Fatalf("a delta sync of a Prowlarr = %d: %s\nwant 409 not_a_catalogue_source", code, body)
	}
}

// Unknown to this user is 404, and it is the SCOPED read that decides.
func TestADeltaSyncOfAnUnknownInstanceIs404(t *testing.T) {
	env := newTestApp(t)
	setUpOwner(t, env)

	code, body := deltaSyncNow(t, env, 4242)
	if code != http.StatusNotFound || !strings.Contains(body, `"error":"not_found"`) {
		t.Fatalf("a delta sync of an unknown instance = %d: %s\nwant 404 not_found", code, body)
	}
}

// The deliverable: pressing the route runs channel 3b's arrivals walk against a
// real BookOrbit, and the walk leaves the durable record the 202 deliberately
// does not carry.
//
// THE `delta_walk` ROW IS THE OBSERVATION, not a log line and not the response.
// §4a is explicit that a 202 says started and no more; what a delta actually did
// is a sync_report row, and a route that answered 202 without ever producing one
// would be indistinguishable from this one on the wire.
func TestPressingTheDeltaRouteRunsAnArrivalsWalkAndRecordsIt(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 1)}
	bo.books = map[int][]map[string]any{
		1: {boBook(101, "The Hobbit", nil)},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, nil)
	id := boInstanceID(t, env)
	waitForImport(t, env, id)

	walksBefore := countIn(t, env,
		`SELECT COUNT(*) FROM sync_report WHERE kind = '`+libsync.SyncReportDeltaWalk+`'`)
	if walksBefore != 0 {
		t.Fatalf("delta_walk rows before any press = %d, want 0 — a full import must not "+
			"write one, or this test proves nothing", walksBefore)
	}
	// The cursor the bootstrap minted. It is what the walk below must ask from,
	// and reading it here rather than hardcoding the fixture's stamp is what
	// makes the assertion about the SEED rather than about a constant.
	watermark := stringIn(t, env,
		`SELECT COALESCE(arrivals_watermark, '') FROM service_instance WHERE id = ?`, id)
	if watermark == "" {
		t.Fatalf("the full import minted no arrivals cursor, so there is no seed for the walk to use")
	}
	queriesBefore := len(bo.bookQueries())

	code, body := deltaSyncNow(t, env, id)
	if code != http.StatusAccepted {
		t.Fatalf("POST sync/delta = %d, want 202: %s", code, body)
	}
	if !strings.Contains(body, `"status":"started"`) {
		t.Errorf("the 202 body must say what happened: %s", body)
	}

	waitFor(t, "the delta walk to leave its durable record", func() bool {
		return countIn(t, env,
			`SELECT COUNT(*) FROM sync_report WHERE kind = '`+libsync.SyncReportDeltaWalk+`'`) == 1
	})

	// 🚩 AND IT WAS AN ARRIVALS WALK, WHICH NOTHING ABOVE CAN TELL. Every other
	// assertion in this test — the 202, the delta_walk row, the released guard —
	// is satisfied identically by a walk that sent NO filter and re-read the
	// whole library, which is channel 1's request wearing channel 3b's name. The
	// one place the two differ on the wire is the filter, so the filter is the
	// subject: exactly one `after(addedAt, …)` rule, seeded from the stored
	// watermark and not from anywhere else.
	asked := bo.bookQueries()[queriesBefore:]
	if len(asked) == 0 {
		t.Fatalf("the press read no books at all; queries seen: %+v", bo.bookQueries())
	}
	for _, q := range asked {
		if q.Filter == nil {
			t.Fatalf("the walk asked for library %d with NO filter — that is a full page walk, "+
				"not an arrivals walk: %+v", q.LibraryID, q)
		}
		if q.Filter.Field != "addedAt" || q.Filter.Operator != "after" {
			t.Errorf("the walk filtered on %s(%s), want after(addedAt)", q.Filter.Operator, q.Filter.Field)
		}
	}
	// The SEED is the first request's filter value, and only the first: a keyset
	// walk advances its cursor page by page, so a later request carrying a larger
	// value is the walk working, not the walk losing the watermark.
	//
	// ⚠️ IT IS THE STORED WATERMARK MOVED BACK, NOT THE STORED WATERMARK. The
	// exact step is internal/libsync's arrivalsLookbehind, which is unexported
	// and pinned by that package's own tests against the constant; naming the
	// number here would copy a constant into a second place and let the two
	// drift. What THIS layer can assert is the shape: the seed is derived from
	// the stored cursor — a small step BELOW it, never above it, never the zero
	// value, never a wall-clock reading.
	seed := asked[0].Filter.Value
	seedAt, err := time.Parse(store.ArrivalsWatermarkLayout, seed)
	if err != nil {
		t.Fatalf("the walk's seed %q does not parse as an arrivals cursor: %v", seed, err)
	}
	markAt, err := time.Parse(store.ArrivalsWatermarkLayout, watermark)
	if err != nil {
		t.Fatalf("the stored watermark %q does not parse: %v", watermark, err)
	}
	back := markAt.Sub(seedAt)
	if back <= 0 {
		t.Errorf("the walk asked from %q, at or AFTER the stored watermark %q — the lookbehind "+
			"is what covers BookOrbit's commit-visibility lag, and asking forward of the cursor "+
			"loses exactly the rows it exists for", seed, watermark)
	}
	if back > time.Hour {
		t.Errorf("the walk asked from %q, %v below the stored watermark %q — that is not a "+
			"lookbehind off this cursor, it is a cursor from somewhere else", seed, back, watermark)
	}

	// AND THE CLAIM COMES BACK. The delta holds the SAME guard a full import
	// holds, released by the goroutine that started it; a leak there would make
	// this the last sync of any kind this process accepts for the instance.
	waitFor(t, "the claim from the requested delta to be released", func() bool {
		code, _ := syncNow(t, env, id)
		return code == http.StatusAccepted
	})
}

// 🚩 libsync.ErrEscalateToFullImport MUST NEVER REACH THE WIRE. It is a sentinel
// the run handles for itself, on the path that already holds the guard — and it
// is raised on exactly the run nobody exercises by hand, so the failure mode is a
// 500 (or a 409) shown to a user whose delta was in fact supposed to become a
// full import and succeed.
//
// The stimulus is a container bound since the last walk: an arrivals filter over
// a library UsArr has never imported would return none of its back catalogue, so
// the honest answer is to read the library.
func TestADeltaStartedFromTheWireThatEscalatesNeverPutsItsSentinelOnTheWire(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 1)}
	bo.books = map[int][]map[string]any{
		1: {boBook(101, "The Hobbit", nil)},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, nil)
	id := boInstanceID(t, env)
	waitForImport(t, env, id)

	// A library the credential was not granted before.
	bo.libraries = append(bo.libraries, boLibrary(2, "Audio", 1))
	bo.books[2] = []map[string]any{boBook(201, "Project Hail Mary", nil)}

	code, body := deltaSyncNow(t, env, id)
	if code != http.StatusAccepted {
		t.Fatalf("a delta that will escalate = %d: %s\nwant 202 — the escalation is decided "+
			"inside the run, after this body is written, so it cannot be an error status", code, body)
	}
	for _, leaked := range []string{"escalat", "full import", "internal", "conflict"} {
		if strings.Contains(strings.ToLower(body), leaked) {
			t.Errorf("the 202 body carries %q: %s", leaked, body)
		}
	}

	// The escalation is not a refusal: the full import RAN, and the newly bound
	// library's BOOK is in the catalogue.
	//
	// ⚠️ THE SUBJECT IS THE `work` ROWS AND IT CANNOT BE THE `library` ROWS. This
	// assertion counted auto libraries until 2026-08-21, and that count is
	// satisfied by the delta itself: bindPhase runs INSIDE DeltaSync, ahead of
	// deltaPreconditions, so the second library row exists before the escalation
	// has even been decided — the test passed with the whole escalation branch in
	// deltaSyncLocked deleted. `delta_walk`.`escalated_to_full_import` is no
	// better: libsync writes it when it DECIDES to escalate, not when cmd/usarr
	// carries the decision out. Only rows that exclusively a full import lands
	// distinguish the two, and the newly bound library's book is one.
	waitFor(t, "the escalated full import to land the newly bound library's book", func() bool {
		return countIn(t, env, `SELECT COUNT(*) FROM work`) == 2
	})
}

// The port's own refusals, asked of the registry directly. The route above
// cannot reach this one: it is the answer for a process that never armed its
// import context, i.e. a build or a test, and saying "started" there would be a
// lie about work nothing will ever run.
func TestStartDeltaSyncOnAnUnarmedProcessRefusesRatherThanClaimingItStarted(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 1)}
	bo.books = map[int][]map[string]any{1: {boBook(101, "The Hobbit", nil)}}

	env := boSetUp(t)
	// Deliberately NOT armed.
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, nil)
	id := boInstanceID(t, env)

	err := env.app.registry.StartDeltaSync(id)
	if err == nil {
		t.Fatal("StartDeltaSync on an unarmed process returned nil, claiming a walk that " +
			"has no context to run under")
	}
	// It is NOT one of the two sentinels a client branches on — those mean
	// something else entirely, and mapping this onto one of them would tell the
	// user to wait for a walk, or to press a different service.
	if errors.Is(err, httpapi.ErrNotDeltaSource) || errors.Is(err, httpapi.ErrImportInProgress) {
		t.Errorf("an unarmed process answered with a client-facing sentinel: %v", err)
	}
}
