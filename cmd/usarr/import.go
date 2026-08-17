package main

import (
	"context"
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
// TWO TRIGGERS, both explicit:
//
//   - ON CONNECT. bootstrapImport runs once when a Kavita client stack is built
//     for an instance that has never completed a full sync. That is the
//     first-run bootstrap: the user adds a Kavita, and a catalogue appears
//     without them having to find a button.
//   - MANUALLY, by calling FullImport.
//
// NO TIMER. There is no periodic re-import in this commit and adding one would
// be the wrong shape anyway: a re-read of the whole library every N hours is
// channel 4's job (the reconciliation sweep, which compares hashes and touches
// <1% of rows), not channel 1's. Neither channel 4 nor channel 3b is built here.

// FullImport runs channel 1 against one instance. It is the manual trigger.
//
// It blocks for the length of the import, which for a large library is minutes.
// Every caller in this process therefore runs it on its own goroutine; the
// contract is stated here rather than hidden behind an internal goroutine,
// because a function that returns before its work is done cannot report what the
// work did.
func (g *registry) FullImport(ctx context.Context, instanceID int64) (libsync.Report, error) {
	entry, err := g.entry(ctx, instanceID)
	if err != nil {
		return libsync.Report{}, err
	}
	if entry.kavita == nil {
		return libsync.Report{}, fmt.Errorf(
			"%q has kind %q; v0.1 imports a catalogue from kavita and from nothing else (ADR-0041)",
			entry.instance.Name, entry.instance.Kind)
	}

	im := &libsync.Importer{
		Store:  g.st,
		Source: libsync.NewKavitaSource(entry.kavita),
		Log:    g.log.With("instance_id", instanceID, "instance", entry.instance.Name),
		// v0.1's single owner. The parameter exists so multi-user is a
		// behaviour change rather than a redesign (§1.3 rule 1).
		UserID:   store.SystemUserID,
		Progress: g.importProgress(),
	}
	return im.FullImport(ctx, instanceID)
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
	if err != nil {
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
