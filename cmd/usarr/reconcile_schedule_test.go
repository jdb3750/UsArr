package main

import (
	"testing"
	"time"
)

// The §7.4 schedule, through the real binary: which instances a timed pass
// enumerates, and — the point of the file — which ones it must not.
//
// THE PREDICATE UNDER TEST IS `deleted_at IS NULL AND enabled = 1`, and no test
// here reads it back. Two of them assert a STAMP that has to survive the pass,
// and one asserts a stamp that has to appear; a test that asserted the predicate's
// text, or counted the instances enumerated, would pass against a pass that
// enumerated correctly and then did the wrong thing to what it found.

// linkDeletedAt reports whether one remote item's link is tombstoned.
func linkTombstoned(t *testing.T, env *testEnv, instanceID int64, remoteID string) bool {
	t.Helper()
	return countIn(t, env,
		`SELECT count(*) FROM service_item_link
		  WHERE service_instance_id = ? AND remote_id = ? AND deleted_at IS NOT NULL`,
		instanceID, remoteID) == 1
}

// libraryOrphaned reports whether the instance's library carries orphaned_at.
func libraryOrphaned(t *testing.T, env *testEnv, instanceID int64) bool {
	t.Helper()
	return countIn(t, env,
		`SELECT count(*) FROM library l
		   JOIN library_source ls ON ls.library_id = l.id
		  WHERE ls.service_instance_id = ? AND l.orphaned_at IS NOT NULL`,
		instanceID) > 0
}

// importedKavita is the shared arrangement: one Kavita reporting two series in
// one library, imported once, with its instance id returned.
func importedKavita(t *testing.T, env *testEnv, kav *fakeKavita) int64 {
	t.Helper()
	setUpOwner(t, env)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)
	waitForImport(t, env, created.ID)
	return created.ID
}

// twoSeries is the upstream before anything is deleted from it.
func twoSeries() []map[string]any {
	return []map[string]any{
		kavitaSeries(41, 1, "Frieren", nil),
		kavitaSeries(42, 1, "Berserk", nil),
	}
}

// aged is a clock far enough past the import that reconcileInterval has elapsed
// several times over. It is passed explicitly so "due" is a fact the test states
// rather than one it waits for.
func aged() time.Time { return time.Now().Add(4 * reconcileInterval) }

// THE POSITIVE CONTROL, and it is first because the two refusals below are
// worthless without it: an empty reconcileOnce would satisfy both of them.
//
// It also pins the due-check in the one direction a stubbed clock can prove —
// an instance whose last full sync has aged past reconcileInterval is re-read,
// and the re-read runs the DELETION PASS, not merely a fetch.
func TestTheScheduleReReadsAnAgedInstanceAndSweepsWhatVanished(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = twoSeries()

	env := newTestApp(t)
	id := importedKavita(t, env, kav)

	if linkTombstoned(t, env, id, "42") {
		t.Fatal("Berserk is tombstoned before the upstream ever stopped reporting it")
	}

	// The upstream loses a series between the two reads. Nothing local changes.
	kav.setSeries([]map[string]any{kavitaSeries(41, 1, "Frieren", nil)})

	env.app.registry.reconcileOnce(t.Context(), aged())

	if !linkTombstoned(t, env, id, "42") {
		t.Errorf("the scheduled pass did not tombstone a series the upstream stopped reporting.\nProcess log:\n%s",
			env.logs())
	}
	if linkTombstoned(t, env, id, "41") {
		t.Error("the scheduled pass tombstoned a series the upstream still reports")
	}
}

// THE OTHER HALF OF THE DUE-CHECK: an instance that synced a moment ago is not
// re-read. Without this, reconcileInterval could be zero and every test above
// would still pass.
func TestTheScheduleLeavesAFreshlySyncedInstanceAlone(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = twoSeries()

	env := newTestApp(t)
	id := importedKavita(t, env, kav)
	kav.setSeries([]map[string]any{kavitaSeries(41, 1, "Frieren", nil)})

	// The clock has NOT moved past reconcileInterval since the import.
	env.app.registry.reconcileOnce(t.Context(), time.Now())

	if linkTombstoned(t, env, id, "42") {
		t.Error("a pass ran against an instance whose last full sync had not aged past reconcileInterval")
	}
}

// ── the enumeration drill ───────────────────────────────────────────────────

// `enabled = 1`, and this is the half of the predicate reconcileOnce writes
// itself.
//
// DISABLING AN ALREADY-IMPORTED INSTANCE IS REACHABLE FROM THE PUBLIC API —
// PATCH /api/v1/services/{id} carries `enabled` (httpapi's updateServiceRequest)
// — and it leaves a row that is not deleted and whose last_full_sync_at is set,
// which is permanently due. Nothing but the filter stands between an operator's
// "leave this service alone" and a full library walk every six hours.
//
// The column is moved here with one statement rather than through the route
// because the route is sudo-protected and the subject of this test is the
// enumeration, not the re-authentication in front of it.
//
// ⚠️ FIRED, AND IT TOOK TWO SITES. Removing reconcileOnce's `!si.Enabled` filter
// alone leaves this test GREEN, because registry.entry refuses a disabled
// instance on FullImport's own path. Removing both is what turns it red
// ("the scheduled pass read an upstream the operator had disabled, and swept
// it", links_tombstoned=1). So this asserts the conjunction, which is the right
// subject: the effect has two independent protections and this is the test that
// notices when the last one goes.
func TestTheScheduleDoesNotReadAServiceTheOperatorDisabled(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = twoSeries()

	env := newTestApp(t)
	id := importedKavita(t, env, kav)

	if _, err := env.app.store.DB().Writer().ExecContext(t.Context(),
		`UPDATE service_instance SET enabled = 0 WHERE id = ?`, id); err != nil {
		t.Fatalf("disable instance: %v", err)
	}

	// The upstream loses a series. A pass that ran would tombstone it.
	kav.setSeries([]map[string]any{kavitaSeries(41, 1, "Frieren", nil)})

	env.app.registry.reconcileOnce(t.Context(), aged())

	if linkTombstoned(t, env, id, "42") {
		t.Errorf("the scheduled pass read an upstream the operator had disabled, and swept it.\nProcess log:\n%s",
			env.logs())
	}
}

// `deleted_at IS NULL`.
//
// ⚠️ THIS HALF OF THE PREDICATE IS ENFORCED FOUR TIMES OVER AND NOT ONCE IN
// reconcile.go, WHICH IS THE MEASUREMENT AND NOT AN EXCUSE FOR THE TEST. Fired
// by stripping, in order: reconcileOnce's use of ListServiceInstances (whose
// statement hard-codes `WHERE deleted_at IS NULL` ahead of the Scope predicate,
// so Scope CANNOT widen onto tombstoned rows) for raw all-instances SQL; then
// registry.entry's disabled refusal and store.getServiceInstance's own
// `deleted_at IS NULL`; then sweepOrphans' restore-arm `si_orphan.deleted_at IS
// NULL` (REVIEW-LOG C-6); and finally store.LastFullSyncAt's `deleted_at IS
// NULL`, which is what actually kept a tombstoned instance from ever being DUE.
// With all four gone it goes red: "a scheduled pass cleared the orphaned_at stamp
// SoftDeleteServiceInstance set."
//
// SO IT IS A REGRESSION GUARD ON A CONJUNCTION, and the honest reading is that no
// single plausible edit to reconcile.go can break it today. It is kept because
// the thing it protects is not local: SoftDeleteServiceInstance stamps
// library.orphaned_at in the delete's own transaction precisely because no sweep
// can ever reach a deleted instance's library again, and a schedule is the first
// thing in this binary that could falsify that premise without anyone pressing a
// button. The assertion is the stamp's survival, so it holds whichever layer is
// doing the work and notices when the last one goes.
func TestTheScheduleLeavesADeletedServicesLibraryOrphaned(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = twoSeries()

	env := newTestApp(t)
	id := importedKavita(t, env, kav)

	if libraryOrphaned(t, env, id) {
		t.Fatal("the library is orphaned while its only instance is still live")
	}
	if err := env.app.store.SoftDeleteServiceInstance(t.Context(), id, time.Now()); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if !libraryOrphaned(t, env, id) {
		t.Fatal("soft-deleting the only instance did not orphan its library; the drill below would be vacuous")
	}

	env.app.registry.reconcileOnce(t.Context(), aged())

	if !libraryOrphaned(t, env, id) {
		t.Errorf("a scheduled pass cleared the orphaned_at stamp SoftDeleteServiceInstance set.\nProcess log:\n%s",
			env.logs())
	}
}
