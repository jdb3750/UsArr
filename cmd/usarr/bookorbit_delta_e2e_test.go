package main

import (
	"errors"
	"testing"

	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/libsync"
	"github.com/jdb3750/UsArr/internal/store"
)

// Channel 3b's WIRING, end to end through the real binary's startup path.
//
// internal/libsync tests DeltaSync against a hand-built adapter and a real
// database. What is untested there, and is tested here, is the two things that
// live in this package and can only fail in a running process: which recorders a
// delta path runs, and whether the escalation deadlocks against the import guard
// it is holding.

// ⚠️ THE ESCALATION MUST CALL THE LOCKED PATH, AND THE WRONG CALL COMPILES. A
// delta claims the mutual-exclusion guard, discovers it needs a full import, and
// calls it — if it calls the guard-CLAIMING entry point it gets "an import is
// already in progress", so NEITHER runs and the person who pressed the button is
// told a sync is running that is not. It fails only on a run that actually
// escalates, which is exactly the run nobody exercises by hand.
func TestADeltaThatEscalatesDoesNotDeadlockAgainstItsOwnImportGuard(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 1)}
	bo.books = map[int][]map[string]any{
		1: {boBook(101, "The Hobbit", nil)},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)
	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, nil)
	env.do(t, "GET", "/api/v1/services", nil, nil)
	created.ID = boInstanceID(t, env)
	waitForImport(t, env, created.ID)

	// A library the credential was not granted before. A container UsArr has
	// never imported is not a delta question — the arrivals filter would return
	// none of its back catalogue.
	bo.libraries = append(bo.libraries, boLibrary(2, "Audio", 1))
	bo.books[2] = []map[string]any{boBook(201, "Project Hail Mary", nil)}

	rep, err := env.app.registry.DeltaSync(t.Context(), created.ID)
	if errors.Is(err, httpapi.ErrImportInProgress) {
		t.Fatal("the escalation collided with the guard the delta was already holding, so " +
			"NEITHER ran — this is the failure that only appears on a new-container run")
	}
	if err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if !rep.Escalated {
		t.Fatalf("the run did not escalate although a container was newly bound: %+v", rep)
	}
	// The escalation is not a refusal: the full import RAN, and the new library's
	// book is in the catalogue.
	if !rep.Completed {
		t.Errorf("the escalated full import did not complete: %+v", rep.Report)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE id <> 0 AND managed_by = 'auto'`); n != 2 {
		t.Errorf("auto libraries = %d, want 2 — the escalation is what imports the new one", n)
	}
}

// 🚩 A DELTA MUST NOT OVERWRITE THE LIBRARIES SCREEN'S PER-CONTAINER VERDICTS.
// recordCompleteness runs, because it is measured against the WHOLE library and
// stays true; recordSkippedItems and recordComicResidue do not, because they are
// tallies of what THIS WALK read and their vocabulary is a claim about the
// container, under a latest-wins read.
func TestADeltaRecordsCompletenessAndNoPerContainerSkipVerdict(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 2)}
	bo.books = map[int][]map[string]any{
		1: {
			boBook(101, "The Hobbit", nil),
			// A file BookOrbit itself cannot classify: read, skipped, counted —
			// so the full import's skip verdict for this container is non-zero.
			boBook(102, "Mystery Blob", map[string]any{
				"files": []map[string]any{boFile(1021, "", "primary")},
			}),
		},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, nil)
	id := boInstanceID(t, env)
	waitForImport(t, env, id)

	skipsAfterImport := countIn(t, env,
		`SELECT COUNT(*) FROM sync_report WHERE kind = '`+store.SyncReportItemsSkipped+`'`)
	completenessAfterImport := countIn(t, env,
		`SELECT COUNT(*) FROM sync_report WHERE kind = '`+store.SyncReportContentCompleteness+`'`)
	if skipsAfterImport == 0 {
		t.Fatalf("the full import wrote no skip verdict, so this test cannot show one surviving")
	}

	if _, err := env.app.registry.DeltaSync(t.Context(), id); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}

	if got := countIn(t, env,
		`SELECT COUNT(*) FROM sync_report WHERE kind = '`+store.SyncReportItemsSkipped+`'`); got != skipsAfterImport {
		t.Errorf("items_skipped rows = %d, want the full import's %d unchanged: the screen's read "+
			"is latest-wins, so a window-scoped zero row IS the wrong answer on it",
			got, skipsAfterImport)
	}
	if got := countIn(t, env,
		`SELECT COUNT(*) FROM sync_report WHERE kind = '`+store.SyncReportContentCompleteness+`'`); got <= completenessAfterImport {
		t.Errorf("content_completeness rows = %d, want more than the import's %d: the verdict is "+
			"measured against the WHOLE library, so it is window-independent and must keep running",
			got, completenessAfterImport)
	}
	// And the delta left its own durable record.
	if got := countIn(t, env,
		`SELECT COUNT(*) FROM sync_report WHERE kind = '`+libsync.SyncReportDeltaWalk+`'`); got != 1 {
		t.Errorf("delta_walk rows = %d, want 1", got)
	}
}

// boInstanceID reads back the one BookOrbit instance's id.
func boInstanceID(t *testing.T, env *testEnv) int64 {
	t.Helper()
	var id int64
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(),
		`SELECT id FROM service_instance WHERE kind = 'bookorbit' AND deleted_at IS NULL`).Scan(&id); err != nil {
		t.Fatalf("read the bookorbit instance id: %v", err)
	}
	return id
}
