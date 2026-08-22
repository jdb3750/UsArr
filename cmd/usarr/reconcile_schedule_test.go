package main

import (
	"fmt"
	"strings"
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
	// ⚠️ AND THE LIBRARY IS ACCEPTED (ADR-0048), because an import creates none.
	// Every caller of this helper is about what happens to a LIBRARY — orphaned,
	// restored, swept — and none of those states exist without a library_source
	// row, which only Accept writes.
	acceptLibraries(t, env, created.ID, acceptSpec{ref: "1", name: "Library 1", kind: "comic"})
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
// ⚠️ THIS COMMENT SAID *"ENFORCED FOUR TIMES OVER AND NOT ONCE IN reconcile.go"*
// AND BOTH HALVES ARE FALSE (re-measured 2026-08-21, REVIEW-LOG LS-394.10). The
// sites are listed in reconcileOnce's own doc comment, which is where a reader
// looking for the predicate goes; what belongs here is the drill that fired this
// test, in the order it was applied:
//
//  1. reconcileOnce's ListServiceInstances call replaced with raw
//     `SELECT id, enabled FROM service_instance`. GREEN.
//  2. registry.entry's disabled refusal, and store.getServiceInstance's own
//     `deleted_at IS NULL`. GREEN.
//  3. sweepOrphans' RESTORE-arm `si_orphan.deleted_at IS NULL` (REVIEW-LOG C-6)
//     — the restore arm only; the stamp arm's copy is the mechanism that orphans
//     the library in the first place, and stripping it kills this test at its own
//     vacuity check instead. GREEN.
//  4. store.LastFullSyncAt's `deleted_at IS NULL`. STILL GREEN — and this is
//     where the old account stopped and declared the red.
//  5. reconcileOnce's own `!si.Enabled` continue. RED: "a scheduled pass cleared
//     the orphaned_at stamp SoftDeleteServiceInstance set."
//
// STEP 5 IS THE CORRECTION. SoftDeleteServiceInstance clears `enabled` in the
// same statement that stamps `deleted_at`, so the loop's `enabled` filter is a
// protection on the TOMBSTONE half too — one of the strips is in reconcile.go
// after all. Of the store-side clauses, LastFullSyncAt's and sweepOrphans'
// restore arm are each INDIVIDUALLY DECISIVE: from the step-5 red, restoring
// either one alone turns this green again.
//
// SO IT IS A REGRESSION GUARD ON A CONJUNCTION, and the honest reading is that no
// single plausible edit can break it today. It is kept because the thing it
// protects is not local: SoftDeleteServiceInstance stamps library.orphaned_at in
// the delete's own transaction precisely because no sweep can ever reach a
// deleted instance's library again, and a schedule is the first thing in this
// binary that could falsify that premise without anyone pressing a button. The
// assertion is the stamp's survival, so it holds whichever layer is doing the
// work and notices when the last one goes.
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

// NEVER SYNCED IS NOT DUE — reconcileDue's `if !at.Valid { return false }`, the
// arm its own doc comment calls load-bearing and the one nothing was firing.
//
// ⚠️ FIRED. Inverting that arm to `return true` turns this red:
// "a scheduled pass found an instance that has never completed a full sync DUE."
// Before this test existed, the same inversion left the whole cmd/usarr suite
// green — every other test of the schedule arranges an instance that HAS synced,
// so the unset column was a state none of them was ever standing in.
//
// # A PROWLARR IS THE SUBJECT, because it is the case that never heals
//
// last_full_sync_at is written on success only, and a Prowlarr has no catalogue
// and will never write it — so under the opposite reading it is permanently
// overdue and produces a kind refusal on every tick for the life of the process.
// The other two cases the arm covers reach the same clause by a different route:
// a catalogue source awaiting its bootstrap, and one whose bootstrap keeps
// failing. This one is chosen because it cannot be argued away as a transient.
//
// ⚠️ THE CONTROL AND THE SUBJECT ARE THE SAME SENTENCE ABOUT TWO INSTANCES, read
// out of one process log after one pass. reconcileInstance logs "due a re-read"
// with the instance id before it touches an upstream, so due-ness is asserted
// where the arm decides it rather than three layers downstream. The Kavita
// beside the Prowlarr IS due, so a reconcileOnce that enumerated nothing — or a
// log this test failed to read, or a message that has since been reworded —
// fails on the control before the refusal is asserted at all.
func TestTheScheduleNeverReadsAServiceThatHasNeverCompletedAFullSync(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = twoSeries()
	prow := newFakeProwlarr(t, importProwlarrKey)

	env := newTestApp(t)
	kavitaID := importedKavita(t, env, kav)

	var prowlarr serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prow.URL(),
		"api_key": importProwlarrKey,
	}, &prowlarr)

	// A Prowlarr never completes a full sync, so nothing ever stamps the column.
	if at, err := env.app.store.LastFullSyncAt(t.Context(), prowlarr.ID); err != nil {
		t.Fatalf("read last_full_sync_at for the Prowlarr: %v", err)
	} else if at.Valid {
		t.Fatalf("the Prowlarr's last_full_sync_at is set to %q; the arm under test is "+
			"about an UNSET column and this test would be drilling something else", at.String)
	}

	env.app.registry.reconcileOnce(t.Context(), aged())
	logs := env.logs()

	// THE CONTROL, first: the pass ran, it found the Kavita due, and it said so
	// in the words this test is about to search for. Without it the refusal below
	// is satisfied by an empty pass and by a message that was renamed.
	if !strings.Contains(logs, dueLine(kavitaID)) {
		t.Fatalf("the scheduled pass never found the Kavita due, though its last full sync has "+
			"aged past reconcileInterval at this clock. Either no pass ran or reconcileInstance no "+
			"longer logs %q — and the refusal asserted below is vacuous either way.\n"+
			"Process log:\n%s", dueLine(kavitaID), logs)
	}

	// THE SUBJECT.
	if strings.Contains(logs, dueLine(prowlarr.ID)) {
		t.Errorf("a scheduled pass found an instance that has never completed a full sync DUE. "+
			"A Prowlarr will never write last_full_sync_at, so treating unset as due makes it "+
			"permanently overdue: a kind refusal on every tick for the life of the process.\n"+
			"Process log:\n%s", logs)
	}
}

// dueLine is reconcileInstance's "this instance is due" sentence for one
// instance, as it lands in the process log.
//
// The trailing space matters: without it instance_id=2 is a prefix of
// instance_id=20, and this test's whole subject is WHICH instance the pass
// decided about.
func dueLine(instanceID int64) string {
	return fmt.Sprintf("catalogue is due a re-read\" instance_id=%d ", instanceID)
}
