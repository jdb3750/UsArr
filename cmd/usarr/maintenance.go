package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/store"
)

// candidateSweepInterval is how often expired release_candidate rows are
// deleted.
//
// It is a constant, not a configuration key. CONFIGURATION.md §1 keeps level-1
// configuration to what an operator genuinely has to decide, and nobody has to
// decide this: the row's own lifetime is releases.CandidateTTL (25 minutes,
// pinned below Prowlarr's non-rolling 30-minute grab cache), so the interval
// only controls how long a dead row lingers past it. Five minutes bounds that
// at TTL + 5 and costs one indexed DELETE — `ix_rel_expiry` covers
// `expires_at`, so the statement touches only rows it is going to remove.
const candidateSweepInterval = 5 * time.Minute

// startCandidateSweeper runs the release_candidate eviction loop and returns a
// channel that closes when the loop has stopped.
//
// WHY THIS EXISTS AT ALL. release_candidate deliberately has no uniqueness on
// (service_instance_id, guid): two searches for the same term inside the TTL
// window insert two full copies of every release. reference/schema.md §6 argues
// that is safe, and the argument is explicitly "the bound on that duplication is
// the TTL … swept via ix_rel_expiry, so the table's size is governed by search
// volume within a 25-minute window, not by uptime". That sentence was only true
// on paper: Store.ExpireReleaseCandidates and Service.EvictExpired both existed,
// and nothing in the binary ever called either, so the table grew without bound
// for the life of the process — hundreds of rows per search, each carrying a
// full ReleaseResource blob, forever.
//
// It sweeps through the store rather than per releases.Service: the DELETE is
// keyed on expires_at alone and is not instance-scoped, so running it once per
// registered instance would re-run the same statement N times for one result.
//
// ORDERING. The caller starts this only after buildApp has returned, and
// buildApp does not return until db.Open has applied every pending migration —
// so the sweep can never run against a database without release_candidate in it.
//
// The first sweep runs immediately. A process that was down for an hour comes
// back to a table full of rows that expired while it was gone, and waiting a
// full interval to clear them is pointless.
func (a *app) startCandidateSweeper(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(candidateSweepInterval)
		defer ticker.Stop()

		for {
			a.sweepCandidatesOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

// sweepCandidatesOnce deletes what has expired and logs the outcome.
//
// A failure is logged and never returned: this is housekeeping, and an
// unsweepable table is not a reason to stop serving. It is logged at warn rather
// than swallowed because a sweep that keeps failing means the table really is
// growing without bound, which is the whole thing this loop prevents.
func (a *app) sweepCandidatesOnce(ctx context.Context) {
	n, err := a.store.ExpireReleaseCandidates(ctx, time.Now())
	switch {
	case ctx.Err() != nil:
		// Shutdown cancelled the sweep mid-statement. Not a fault.
	case err != nil:
		a.log.Warn("could not sweep expired release candidates", "err", err)
	case n > 0:
		a.log.Debug("swept expired release candidates", "rows", n)
	}
}

// redactStoredCredentials repairs credentials already written to disk, once per
// start, before the server exists.
//
// THE PROBLEM IT FIXES. provenance.nzb_info_url is indexer-supplied and
// permanent by design: it records which tracker and which release page a grab
// came from, and provenance rows are never overwritten. Rows written before the
// grab path started redacting hold a private tracker's PASSKEY verbatim, and on
// a private tracker a leaked passkey means account termination, because it is
// what the tracker attributes traffic by. release_candidate heals itself — a
// 25-minute TTL and the sweeper above — and provenance does not, which is why
// this exists and the sweeper is not enough.
//
// IT USES THE WRITE PATH'S OWN REDACTOR. servarr.RedactURL is what
// releases.recordProvenance calls, and it is ssrf.RedactRawURL underneath: one
// deny-list, one path heuristic, no second copy. A stored value and a served
// value therefore cannot disagree about what counts as a secret, which is the
// property that makes this a repair rather than a second opinion.
//
// A FAILURE IS FATAL, unlike the sweep. The sweep is housekeeping and a failure
// costs disk; this is a credential still readable through the API, so refusing
// to start is the correct answer and buildApp propagates the error.
//
// WHAT IT CANNOT REACH. Only the live database. A passkey already copied into a
// VACUUM INTO backup, a filesystem snapshot or a support bundle is still in that
// copy and nothing here can touch it — the remedy is to rotate the passkey at
// the tracker and delete the old copies. Said again in reference/security.md §5
// so an operator meets it somewhere other than a source comment.
func redactStoredCredentials(ctx context.Context, st *store.Store, log *slog.Logger) error {
	n, err := st.RedactStoredProvenanceURLs(ctx, servarr.RedactURL)
	if err != nil {
		return fmt.Errorf("redacting credentials already stored in provenance: %w", err)
	}
	if n > 0 {
		// Warn, not info. This says a credential WAS on disk, which an operator
		// has to act on: the rows are clean now, but any backup taken before
		// this run still holds the passkey verbatim.
		log.Warn("redacted credentials found in stored provenance URLs; "+
			"these rows are clean now, but any backup taken before this run still holds them — "+
			"rotate the passkey at the tracker and delete those backups",
			"rows", n)
	}
	return nil
}
