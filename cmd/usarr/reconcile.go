package main

import (
	"context"
	"errors"
	"time"

	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/store"
)

// Channel 4's SCHEDULE: ARCHITECTURE.md §7.4's "every 6 h plus on demand",
// finally on a clock.
//
// # What it adds, which is ONLY the clock
//
// §7.4's pass is "fetch the full entity list per instance; left-anti-join
// `service_item_link`; soft-delete with a 7-day tombstone; compare `remote_hash`;
// emit a `sync_report`". That is a full import, exactly — internal/libsync's
// FullImport is the fetch and the hash comparison, and it calls
// store.SweepDeletions on its success path as the left-anti-join. So the pass
// was already built and already reachable; what was missing was something to
// start it without a person.
//
// ⚠️ AND §7.4's "PLUS ON DEMAND" WAS ALREADY BUILT TOO, which is why there is no
// request channel here and this loop is shaped like startCandidateSweeper rather
// than like RunProber. The Services screen's "Run full sync now" reaches
// StartImport through POST /api/v1/services/{id}/sync (§17.3), and StartImport
// runs the identical pass through the identical guard. A second on-demand door
// into one behaviour would be a trigger with no caller, and import.go's header
// enumerates the triggers explicitly — adding an unreachable fourth entry to
// that list would make the list wrong in the direction it is hardest to notice.
//
// # It calls FullImport and not SweepDeletions, and that is not a shortcut
//
// SweepDeletions' precondition is that the seen-set it is handed is the
// upstream's WHOLE list, and it says in its own doc comment that it cannot check
// that. A timer holds no such list. The only way to satisfy the precondition is
// to go and read the upstream, which is what FullImport does — so a scheduler
// that reached for the sweep directly would have to synthesise a seen-set it did
// not observe, and the difference between that set and the truth is tombstoned.
// That is the whole library on a partial read.
//
// # The cost, measured rather than assumed
//
// The recurring cost of this loop is a NO-OP sweep — the shape a healthy
// instance produces, where the read reports everything the replica already has.
// Measured at this tip on x86-64 (4 cores, go1.25.13): 50-57 ms of single-writer
// hold over 20,000 live links. The deletion-heavy figures REVIEW-LOG C-7
// recorded are re-measured with it (ADR-0076), and they describe an event — a
// genuine library-scale upstream deletion — not this loop's ordinary tick.
//
// Two facts keep that off principle 1. Reads do not queue behind it: internal/db
// runs WAL with a read pool and one writer, so a write transaction blocks other
// WRITES and never a render path. And the import's own writes are already
// chunked — ApplyCatalogueBatch is sized to min(2000 rows, 100 ms) — so the
// sweep's single transaction is the one unbounded hold in the pass, which is the
// trade C-7 accepted on atomicity and ADR-0076 re-accepts for an unattended run.
//
// ⚠️ §7.4's "run at low priority with a bounded rate" IS NOT FULLY HONOURED, and
// naming the gap is the point. What is honoured is serialisation: instances are
// reconciled one at a time (see reconcileOnce), so N instances never read N
// upstreams at once. There is no priority class and no request-rate limiter; the
// bound is the adapter's own paging and the 6-hour period.
const (
	// reconcileInterval is how stale an instance's last completed full sync may
	// get before this loop reads its upstream again — §7.4's "every 6 h".
	//
	// It is a constant, not a configuration key, on maintenance.go's standing
	// ruling for candidateSweepInterval: CONFIGURATION.md §1 keeps level-1
	// configuration to what an operator genuinely has to decide. ⚠️ sync.md §4
	// writes it "every 6 h (configurable)" and that parenthesis is NOT honoured
	// here — deliberately. An operator asked to pick this number has to know how
	// long their upstream takes to walk and how fast their catalogue churns, and
	// the honest default is the one §7.4 already chose for them.
	reconcileInterval = 6 * time.Hour

	// reconcileTick is how often the loop ASKS whether anything is due. It is
	// not the period; reconcileInterval is.
	//
	// WHY THE TWO ARE DIFFERENT NUMBERS. A bare 6-hour ticker measures from
	// process start, so a box that restarts on every update — the ordinary life
	// of a self-hosted binary — can go indefinitely without ever reconciling,
	// and the loop's own liveness would depend on uptime nobody promised. Due-ness
	// is therefore read off last_full_sync_at, which is durable, and the ticker
	// only sets the resolution at which that column is consulted. Thirty minutes
	// bounds the overshoot at 6 h 30 m rather than at the 12 h a same-period
	// ticker would give, and costs one indexed read per instance per half hour.
	reconcileTick = 30 * time.Minute
)

// startReconciler runs the §7.4 schedule and returns a channel that closes when
// the loop has stopped.
//
// THE FIRST PASS RUNS IMMEDIATELY, and it is safe here for the reason it is safe
// in startCandidateSweeper and would NOT have been safe on a bare ticker: due-ness
// is a question about last_full_sync_at, so a process that has just restarted
// finds nothing due and reads no upstream. A process that was down for a day
// finds everything due and catches up at once, which is the behaviour a restart
// should have and the one a bare ticker cannot produce.
func (g *registry) startReconciler(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(reconcileTick)
		defer ticker.Stop()

		for {
			g.reconcileOnce(ctx, time.Now())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

// reconcileOnce reconciles every instance that is due, one at a time.
//
// # The enumeration predicate is `deleted_at IS NULL AND enabled = 1`
//
// ⚠️ NEITHER HALF OF IT DEPENDS ON THIS FUNCTION, and that was measured rather
// than assumed — the loop below states the predicate, and every layer under it
// states it again. A reader who takes the two clauses here for the only thing
// standing between a timer and a deleted service has it backwards, and a reader
// who therefore deletes them has removed the cheapest copy, not the load-bearing
// one. What each half really rests on, at this tip:
//
//   - `deleted_at IS NULL`: FOUR independent filters, none of them here.
//     store.listServiceInstances hard-codes `WHERE deleted_at IS NULL` into the
//     statement ahead of the Scope predicate, so Scope{AllInstances: true} is
//     `1=1` over the LIVE rows and cannot widen onto the dead ones — Scope
//     decides which visible instance a caller may see, never whether a tombstoned
//     one is visible at all. store.LastFullSyncAt carries the same clause, so a
//     tombstoned instance can never be due. store.getServiceInstance carries it a
//     third time, so registry.entry cannot build a client for one. And
//     sweepOrphans' restore arm requires `si_orphan.deleted_at IS NULL` on both
//     arms (REVIEW-LOG C-6), so the stamp survives even a pass that reached it.
//     All four had to be removed together before the guard in
//     reconcile_schedule_test.go could be made to fail.
//
//   - `enabled = 1`: the filter below, plus registry.entry's own refusal
//     (services.go: "%q is disabled"), which is on FullImport's path through
//     runImport. Either one alone is enough to stop the pass.
//
// SO WHY WRITE EITHER CLAUSE HERE. Because a refusal that arrives at entry() has
// already cost a row read and a credential open, and it arrives as an ERROR —
// which this loop logs at warn, once per disabled instance per tick, for the life
// of the process. A log that cries every half hour about a state the operator
// chose deliberately is a log an operator learns to skip. The clauses are
// cheapness and quiet, and the correctness is layered underneath them.
//
// WHAT THE TOMBSTONE HALF IS PROTECTING, since it is the non-obvious one.
// SoftDeleteServiceInstance stamps library.orphaned_at in the same transaction
// that tombstones the instance (REVIEW-LOG C-6), because a deleted instance never
// imports again and no sweep can ever reach the library it abandoned. A scheduler
// that enumerated tombstoned rows would break that premise on a timer. The guard
// asserts the EFFECT — the stamp still set after a pass — rather than the
// predicate's text, because the predicate is spread across five files and a test
// that read it back would only be reading itself.
//
// SEQUENTIAL, and that is §7.4's "bounded rate" as far as it is honoured here: at
// 03:00 every catalogue instance comes due in the same tick, and reconciling them
// concurrently would put N full library walks on the wire at once.
func (g *registry) reconcileOnce(ctx context.Context, now time.Time) {
	instances, err := g.st.ListServiceInstances(ctx, store.Scope{AllInstances: true})
	if err != nil {
		g.log.Warn("reconciliation sweep: services could not be listed", "err", err)
		return
	}
	for _, si := range instances {
		if ctx.Err() != nil {
			return
		}
		if !si.Enabled {
			continue
		}
		if !g.reconcileDue(ctx, si.ID, now) {
			continue
		}
		g.reconcileInstance(ctx, si.ID)
	}
}

// reconcileDue answers whether one instance's last completed full sync is older
// than reconcileInterval.
//
// ⚠️ NEVER SYNCED IS NOT DUE, which reads backwards and is the load-bearing arm.
// last_full_sync_at is written on SUCCESS ONLY, so an unset column means one of
// three things and this loop must decline all three. A catalogue source nobody
// has imported yet belongs to bootstrapImport, which is gated on the same column
// and will run on the next connect. A catalogue source whose bootstrap keeps
// FAILING would, under the opposite reading, be re-read every half hour forever
// against an upstream that is already broken. And a Prowlarr — which has no
// catalogue and will never write this column — would be permanently overdue,
// producing a kind-refusal warning on every tick for the life of the process.
// Treating unset as not-due answers all three with one clause and needs no kind
// check to do it.
//
// A read failure is not-due. It is one SELECT against the local database; if it
// cannot be answered, starting minutes of upstream traffic on the strength of the
// failure is the wrong direction to err in.
func (g *registry) reconcileDue(ctx context.Context, instanceID int64, now time.Time) bool {
	at, err := g.st.LastFullSyncAt(ctx, instanceID)
	if err != nil {
		if ctx.Err() == nil {
			g.log.Warn("reconciliation sweep: cannot tell when this service last synced; skipping it this pass",
				"instance_id", instanceID, "err", err)
		}
		return false
	}
	if !at.Valid {
		return false
	}
	last, err := store.ParseTime(at.String)
	if err != nil {
		g.log.Warn("reconciliation sweep: this service's last_full_sync_at is unreadable; skipping it this pass",
			"instance_id", instanceID, "err", err)
		return false
	}
	return now.Sub(last) >= reconcileInterval
}

// reconcileInstance runs one instance's pass and logs the outcome.
//
// IT TAKES THE SHARED IMPORT GUARD by calling FullImport rather than
// fullImportLocked, so a scheduled pass that lands while the operator is watching
// a hand-pressed sync stands down instead of colliding with it. That refusal is
// bootstrapImport's case exactly, and it is logged the same way: not a failure,
// because the work this pass wanted done is already being done.
//
// A FAILURE IS LOGGED AND NEVER RETURNED. This is unattended background
// replication with no caller left to tell; the rows the run committed stand,
// last_full_sync_at is unwritten, and the instance is simply still due on the
// next tick. The Services screen's health is the prober's answer, not this one's.
func (g *registry) reconcileInstance(ctx context.Context, instanceID int64) {
	g.log.Info("reconciliation sweep: this service's catalogue is due a re-read",
		"instance_id", instanceID, "interval", reconcileInterval.String())

	rep, err := g.FullImport(ctx, instanceID)
	switch {
	case ctx.Err() != nil:
		// Shutdown cancelled the pass. Not a fault, and not worth a warning that
		// would fire on every clean stop that happened to catch a run.
	case errors.Is(err, httpapi.ErrImportInProgress):
		g.log.Info("an import for this service is already running; the reconciliation sweep stands down",
			"instance_id", instanceID)
	case err != nil:
		g.log.Warn("the reconciliation sweep did not finish; the rows it wrote stand and it stays due",
			"instance_id", instanceID,
			"items_read", rep.ItemsRead, "items_applied", rep.ItemsApplied, "err", err)
	default:
		g.log.Info("reconciliation sweep complete",
			"instance_id", instanceID,
			"items", rep.ItemsApplied,
			"links_tombstoned", rep.Deletions.LinksTombstoned,
			"works_tombstoned", rep.Deletions.WorksTombstoned,
			"libraries_orphaned", rep.Deletions.LibrariesOrphaned)
	}
}
