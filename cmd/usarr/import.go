package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/libsync"
	"github.com/jdb3750/UsArr/internal/store"
)

// The catalogue full import (channel 1), wired.
//
// It lives beside RunProber because it is the same kind of thing: outbound HTTP
// driven by a background worker, in the one package that is allowed to hold a
// client (ARCHITECTURE.md §2.3 rule 1). It is NOT the prober: the prober runs on
// a ticker, and this deliberately does not.
//
// # What triggers it, and what deliberately does not
//
// THREE TRIGGERS, all explicit:
//
//   - ON CONNECT. bootstrapImport runs once when a Kavita client stack is built
//     for an instance that has never completed a full sync. That is the
//     first-run bootstrap: the user adds a Kavita, and a catalogue appears
//     without them having to find a button.
//   - ON DEMAND, from the Services screen's "Run full sync now" action
//     (ARCHITECTURE.md §17.3), which reaches StartImport through
//     POST /api/v1/services/{id}/sync.
//   - IN PROCESS, by calling FullImport directly.
//
// WHY THE THIRD ONE HAD TO EXIST. bootstrapImport is gated on last_full_sync_at
// and that gate is correct for what it guards — a restart must not re-read the
// whole library — but it left the process with NO WAY AT ALL to ask for a second
// import. Every fix that changes what an import WRITES (the credit pass writing
// work.year is the one that forced this) is unreachable for rows already
// imported, for the life of the database, because the only trigger declines to
// run again. A re-import is therefore not a feature of the sync core: it is what
// makes the sync core's own fixes deliverable.
//
// NO TIMER. There is no periodic re-import here and adding one would be the
// wrong shape anyway: a re-read of the whole library every N hours is channel
// 4's job (the reconciliation sweep, which compares hashes and touches <1% of
// rows), not channel 1's. Neither channel 4 nor channel 3b is built here — which
// is also why this is a FULL re-import rather than a delta: internal/libsync has
// exactly one channel, and a delta walk over ADR-0035 §2a's watermark would
// revisit only what changed upstream and so could never repair a row UsArr
// itself wrote wrongly.

// FullImport runs channel 1 against one instance. It is the manual trigger.
//
// It blocks for the length of the import, which for a large library is minutes.
// Every caller in this process therefore runs it on its own goroutine; the
// contract is stated here rather than hidden behind an internal goroutine,
// because a function that returns before its work is done cannot report what the
// work did.
func (g *registry) FullImport(ctx context.Context, instanceID int64) (libsync.Report, error) {
	// THE MUTUAL-EXCLUSION POINT, and it is here rather than in StartImport
	// because there are three callers and only one of them is StartImport. A
	// bootstrap racing a hand-pressed "Run full sync now" is the case a guard
	// inside the handler would miss entirely.
	if !g.beginImport(instanceID) {
		return libsync.Report{}, fmt.Errorf("%w for service instance %d", httpapi.ErrImportInProgress, instanceID)
	}
	defer g.endImport(instanceID)
	return g.fullImportLocked(ctx, instanceID)
}

// fullImportLocked is FullImport's body, with the guard ALREADY HELD by the
// caller. Splitting it is what lets StartImport claim the guard synchronously —
// so its caller gets a truthful "started" or "already running" — and release it
// from the goroutine that actually finishes the work.
func (g *registry) fullImportLocked(ctx context.Context, instanceID int64) (libsync.Report, error) {
	rep, err := g.runImport(ctx, instanceID)
	if err != nil {
		// THE TERMINAL FRAME FOR A RUN THAT STOPPED, and this wrapper is the
		// only site that can publish it. Two of the failures below happen
		// BEFORE an Importer exists — a service that is not a Kavita, and a
		// stored credential that will not open — so libsync.FullImport cannot
		// see them and a publish inside it would leave those two silent, which
		// is exactly half of the defect (REVIEW-LOG LS-152, LS-180).
		//
		// It is deliberately NOT published for the refusals decided before this
		// function is entered — an import already running (FullImport), and
		// StartImport's own kind/armed checks. Those callers get a synchronous
		// non-2xx and http-api.md §4.4 is explicit that it means NO IMPORT
		// STARTED for that press; a terminal frame there would claim a run that
		// never existed.
		g.publishImportStopped(instanceID, rep)
	}
	return rep, err
}

// runImport is fullImportLocked's body: everything that can fail, with nothing
// between it and the publish above.
func (g *registry) runImport(ctx context.Context, instanceID int64) (libsync.Report, error) {
	entry, err := g.entry(ctx, instanceID)
	if err != nil {
		return libsync.Report{}, err
	}
	if entry.kavita == nil {
		return libsync.Report{}, fmt.Errorf(
			"%q has kind %q; v0.1 imports a catalogue from kavita and from nothing else (ADR-0041)",
			entry.instance.Name, entry.instance.Kind)
	}

	log := g.log.With("instance_id", instanceID, "instance", entry.instance.Name)
	// The adapter gets its OWN logger because it is where a refused identity
	// claim is reported, and the importer never sees one — the mapping has
	// already dropped it by the time a CatalogueItem exists.
	src := libsync.NewKavitaSource(entry.kavita)
	src.Log = log

	im := &libsync.Importer{
		Store:  g.st,
		Source: src,
		Log:    log,
		// v0.1's single owner. The parameter exists so multi-user is a
		// behaviour change rather than a redesign (§1.3 rule 1).
		UserID:   store.SystemUserID,
		Progress: g.importProgress(),
	}
	return im.FullImport(ctx, instanceID)
}

// importPhaseStopped is the terminal failure phase, specified by
// docs/reference/http-api.md §5.5 and consumed by web/src/lib/services.ts.
//
// IT IS `stopped`, NEVER `failed`. `failed` is already taken in the SPA
// (SyncPhase, services.ts) for the refusal that means NO IMPORT STARTED, so the
// catalogue is untouched — the exact negation of this frame, which says a run
// did start, did commit batches, and those rows STAND. §5.5.2 owns that
// reasoning; do not rename it.
const importPhaseStopped = "stopped"

// publishImportStopped sends the one terminal frame a stopped run gets.
//
// IT IS THE SAME Progress STRUCT AND IT GAINS NO FIELD (§5.5.1). There is no
// cause, no reason, no status code and no upstream text — deliberately, and
// §5.5.5 owns that: reference/security.md §5 forbids upstream strings in an SSE
// payload, and the absence of a field is a stronger guarantee than a rule about
// what may be put in one. The operator reads the real error in the process log,
// in sync_report, and on the Services health row.
//
// The counters are how far the run got, not a result: Applied is the rows that
// reached a COMMITTED batch and therefore stand, because a stopped import is
// never rolled back (§5.5.4). Both are 0 for the two failures that happen
// before an Importer exists, which is honest — nothing was written.
//
// If the hub is not wired it publishes nothing, exactly as a healthy run's
// frames are not published: importProgress returns nil there, so silence on
// that build is not a claim about the import either way.
func (g *registry) publishImportStopped(instanceID int64, rep libsync.Report) {
	publish := g.importProgress()
	if publish == nil {
		return
	}
	publish(libsync.Progress{
		InstanceID: instanceID,
		Phase:      importPhaseStopped,
		ItemsRead:  rep.ItemsRead,
		Applied:    rep.ItemsApplied,
	})
}

// importProgress is the one line that puts §7.2's "progress over SSE with real
// counts" on the existing stream.
//
// It is cheap — a callback, one Publish per committed batch — which is the
// condition under which this was worth doing at all. If the hub is not wired
// (the registry is built before the server in buildApp) it returns nil and the
// importer publishes nothing, rather than the import failing over telemetry.
func (g *registry) importProgress() libsync.ProgressFunc {
	hub := g.hub
	if hub == nil {
		return nil
	}
	return func(p libsync.Progress) {
		// UserID is the owner: an import is not a user's action, and 0 on this
		// hub means the system sentinel, not "everyone".
		hub.Publish(store.SystemUserID, httpapi.EventImportProgress, p)
	}
}

// bootstrapImport is the on-connect trigger.
//
// IT RUNS AT MOST ONCE PER INSTANCE PER DATABASE, gated on last_full_sync_at
// being unset — which is a column written on SUCCESS ONLY, so a failed or
// partial import leaves the gate open and the next connect retries. A restart
// after a successful import does not re-run it.
//
// The goroutine is deliberate and so is its context: it must not block the
// caller (entry() is on the probe path and, through it, on a request path), and
// it must not inherit a request's or a probe's deadline — a 20-second probe
// timeout would kill a five-minute import halfway through. It takes the
// process-lifetime context the prober was started with.
func (g *registry) bootstrapImport(ctx context.Context, instanceID int64) {
	at, err := g.st.LastFullSyncAt(ctx, instanceID)
	if err != nil {
		g.log.Warn("cannot tell whether this service has ever been imported; skipping the bootstrap import",
			"instance_id", instanceID, "err", err)
		return
	}
	if at.Valid {
		return
	}
	g.log.Info("first connect to this catalogue source; starting a full import",
		"instance_id", instanceID)
	rep, err := g.FullImport(ctx, instanceID)
	switch {
	case errors.Is(err, httpapi.ErrImportInProgress):
		// Not a failure and not worth a warning: something else — a hand-pressed
		// "Run full sync now" that arrived first — is already doing exactly this
		// work. It can happen on a never-imported instance, because StartImport
		// builds the client stack and building it is what schedules this.
		g.log.Info("an import for this service is already running; the bootstrap stands down",
			"instance_id", instanceID)
		return
	case err != nil:
		// SOFT. A failed bootstrap must not take the service down: the rows that
		// committed stand, last_full_sync_at is unwritten, and the next connect
		// tries again. The Services screen's own health is the prober's answer,
		// not this one's.
		g.log.Warn("the first full import did not finish; the rows it wrote stand and it will be retried on the next connect",
			"instance_id", instanceID,
			"items_read", rep.ItemsRead, "items_applied", rep.ItemsApplied, "err", err)
		return
	}
	g.log.Info("first full import complete",
		"instance_id", instanceID,
		"libraries_created", rep.LibrariesCreated,
		"libraries_joined", rep.LibrariesJoined,
		"declined_containers", len(rep.DeclinedContainers),
		"items", rep.ItemsApplied,
		"unidentified", rep.Unidentified)
}

// ── the concurrency guard, and why it is this one ───────────────────────────
//
// beginImport claims the right to import one instance and reports whether it
// got it. endImport releases it, and every acquisition is paired with a defer.
//
// AN IN-PROCESS MAP, chosen over the two alternatives on the table:
//
//   - A `state` column on service_instance would survive the process, which
//     sounds like the stronger guarantee and is the weaker one: a crash or a
//     SIGKILL mid-import leaves it reading "running" with nothing running, and
//     the instance can never be imported again without a hand-edited database.
//     A lock whose holder cannot die is a lock that needs a lease, a clock and
//     a reaper, which is three subsystems for a single-binary self-hosted app.
//   - sync_report is an append-only journal (00005_library_sync.sql), not a
//     mutual-exclusion primitive. Reading "did the last row say started" is a
//     TOCTOU race by construction.
//
// The map is exactly as durable as the thing it guards: an import only exists
// inside this process, so a process that is gone is holding no import. UsArr is
// one binary against one SQLite file (ARCHITECTURE.md §2), so there is no second
// process to coordinate with — and if there ever were, the single-writer
// connection is the boundary that would need the lease, not this map.
//
// It is a SEPARATE mutex from g.mu on purpose. g.mu guards the client registry
// and is taken on the probe and request paths; holding it for the minutes an
// import runs would stall both.
func (g *registry) beginImport(instanceID int64) bool {
	g.importMu.Lock()
	defer g.importMu.Unlock()
	if g.importing[instanceID] {
		return false
	}
	g.importing[instanceID] = true
	return true
}

func (g *registry) endImport(instanceID int64) {
	g.importMu.Lock()
	defer g.importMu.Unlock()
	delete(g.importing, instanceID)
}

// ── httpapi.CatalogueImports ────────────────────────────────────────────────

// StartImport is the Services screen's "Run full sync now" (ARCHITECTURE.md
// §17.3), and it is the on-demand trigger named in this file's header.
//
// IT DOES NOT BLOCK, and that is principle 1 rather than a convenience: the
// handler behind it is a user-facing request and an import is minutes. It
// claims the guard SYNCHRONOUSLY — so the answer "already running" is decided
// before this function returns, not by a goroutine that has not been scheduled
// yet — and then hands the claim to the goroutine that does the work. Returning
// nil means an import is now running; a caller learns it finished from the
// terminal import.progress frame on the SSE stream — phase "done" for a run
// that completed, phase "stopped" for one that did not (http-api.md §5.5) — or
// by asking again and being told it may start. Neither frame is authoritative:
// it can be dropped or missed, so last_full_sync_at stays the evidence (§4.3).
//
// The context is deliberately NOT the caller's. Same reason bootstrapImport's
// is not: a request deadline measured in seconds would kill a five-minute
// import halfway through.
func (g *registry) StartImport(instanceID int64) error {
	entry, err := g.entry(context.Background(), instanceID)
	if err != nil {
		return err
	}
	// The kind check is repeated from FullImport rather than delegated to it,
	// because delegating means the caller only learns "this is a Prowlarr"
	// after a goroutine has already been launched and the HTTP response has
	// already said "started".
	if entry.kavita == nil {
		return fmt.Errorf("%w: %q has kind %q (ADR-0041)",
			httpapi.ErrNotCatalogueSource, entry.instance.Name, entry.instance.Kind)
	}
	if g.importCtx == nil {
		return errImportsNotArmed
	}
	if !g.beginImport(instanceID) {
		return fmt.Errorf("%w for service instance %d", httpapi.ErrImportInProgress, instanceID)
	}

	ctx := g.importCtx
	go func() {
		// The claim is already held; releasing it is this goroutine's job, and
		// FullImport must not try to take it a second time.
		defer g.endImport(instanceID)
		rep, err := g.fullImportLocked(ctx, instanceID)
		if err != nil {
			g.log.Warn("the requested full import did not finish; the rows it wrote stand",
				"instance_id", instanceID,
				"items_read", rep.ItemsRead, "items_applied", rep.ItemsApplied, "err", err)
			return
		}
		g.log.Info("the requested full import is complete",
			"instance_id", instanceID,
			"items", rep.ItemsApplied, "unidentified", rep.Unidentified)
	}()
	return nil
}

// errImportsNotArmed is the honest answer from a build or a test that never
// called enableBootstrapImport: there is no process-lifetime context to run an
// import under, so saying "started" would be a lie.
var errImportsNotArmed = errors.New("this process has no import context; catalogue imports are not armed")
