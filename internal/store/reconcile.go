package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Channel 4's DELETION PASS: what a replica does about a row the upstream has
// stopped reporting (ARCHITECTURE.md §7.4, reference/sync.md §4 steps 2 and 3).
//
// It is only the deletion half. Drift detection — §7.4 step 4's `remote_hash`
// comparison and the refetch it triggers — is not here, and neither is guard 2's
// instance-identity generation check; both are named in §7.4 and neither is
// reachable from this file.
//
// # The set difference is computed in Go, never in SQL
//
// The obvious statement is `… WHERE remote_id NOT IN (SELECT value FROM
// json_each(?))` with the whole read serialised into one parameter. It is
// rejected structurally rather than by preference: the JSON payload is bounded
// by SQLITE_MAX_LENGTH and the statement by SQLITE_MAX_SQL_LENGTH, and a
// catalogue is exactly the thing that grows past a bound like that on somebody
// else's install and not on the author's. Reading the live rows and diffing
// against a Go map has no ceiling, and the read it costs is one the sweep has
// to do anyway to know which works lost their last link.
//
// # Nothing here hard-deletes
//
// §7.4's tombstone is not a formality, it is the mitigation for the nightmare
// bug the section names: an upstream that is temporarily empty — an unmounted
// share, a credential that lost its scope, a container the operator renamed —
// must not take the user's tags, requests and playback state with it. Every
// absence recorded here is a stamped column on a retained row, and every one of
// them is cleared again by the ordinary write path when the thing comes back.
//
// The one hard delete in channel 4 is guard 1's, and it is in catalogue.go where
// the resurrection it answers is detected.
//
// It is a replication write driven by a background worker with no acting user,
// so it takes no Scope — catalogue.go's header carries the full argument.

// SyncReportDeletionSweep is sync_report.kind for one completed deletion pass:
// reference/sync.md §4 step 5's "emit a sync_report row", carrying what the
// sweep changed.
//
// It is a CONSTANT for SyncReportFileWalkFailed's reason — sync_report.kind
// carries no CHECK, so a typo is a row that silently never matches — and it is
// spelled to collide with nothing already in that vocabulary: `delta_walk`,
// `identity_conflict`, `container_declined`, `container_bind_failed`,
// `container_kind_changed`, `items_skipped`, `content_completeness`,
// `file_walk_failed`.
const SyncReportDeletionSweep = "deletion_sweep"

// LinkRef identifies one service_item_link row within one service instance.
//
// BOTH FIELDS, ALWAYS. `ux_sil` is UNIQUE on (service_instance_id, remote_kind,
// remote_id) and migration 0005's own comment says it "is only usable IN FULL
// when remote_kind is known at lookup time" — a ref that carried the id alone
// would make every lookup built from it a range scan over the instance, and
// would make two different kinds sharing an upstream id indistinguishable.
type LinkRef struct {
	RemoteKind string
	RemoteID   string
}

// SweepResult is what one deletion pass changed. Every field is a count of rows
// this call MOVED, never of rows that were already in the state.
type SweepResult struct {
	// LinksLive is how many live links the instance had when the sweep read
	// them — the denominator the other item figures are against.
	LinksLive int

	// LinksTombstoned is links that were live and are now inside the 7-day
	// window.
	LinksTombstoned int

	// WorksTombstoned is works that lost their LAST live link in this sweep. It
	// is smaller than LinksTombstoned whenever a second instance still reports
	// the same work, which is the whole reason it is counted separately.
	WorksTombstoned int

	// SourcesMissing is library_source rows newly stamped missing_since.
	SourcesMissing int

	// LibrariesOrphaned and LibrariesRestored are §6.5 rule 5's retained-orphan
	// state entered and left. A library is orphaned when no source of it is
	// still being reported by anyone.
	LibrariesOrphaned int
	LibrariesRestored int
}

// SweepDeletions is the deletion pass over one service instance, after a read
// that saw the upstream's WHOLE list.
//
// ⚠️ THE PRECONDITION IS THE WHOLE CORRECTNESS OF THIS FUNCTION AND IT CANNOT BE
// CHECKED HERE. `seenItems` must be every link the caller's read observed and
// `seenContainers` every container it observed; anything absent from them is
// treated as absent upstream. A partial read — a stream that died mid-array, a
// delta walk that by construction returns only what changed — passed to this
// function tombstones the difference, which is the entire library. The caller is
// what enforces it: internal/libsync calls this from FullImport's success path
// only, and its delta channel collects no seen-set at all.
//
// now is injectable so the stamps a test asserts are the stamps it chose.
func (s *Store) SweepDeletions(
	ctx context.Context, instanceID int64,
	seenItems []LinkRef, seenContainers []string, now time.Time,
) (SweepResult, error) {
	seen := make(map[LinkRef]struct{}, len(seenItems))
	for _, k := range seenItems {
		seen[k] = struct{}{}
	}
	containers := make(map[string]struct{}, len(seenContainers))
	for _, c := range seenContainers {
		containers[c] = struct{}{}
	}

	var res SweepResult
	nowStr := FormatTime(now)
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := sweepItems(ctx, tx, instanceID, seen, nowStr, &res); err != nil {
			return err
		}
		if err := sweepContainers(ctx, tx, instanceID, containers, nowStr, &res); err != nil {
			return err
		}
		return recordSyncReport(ctx, tx, instanceID, SyncReportDeletionSweep, "", "",
			res.detailJSON())
	})
	if err != nil {
		return SweepResult{}, fmt.Errorf("deletion sweep of service_instance %d: %w", instanceID, err)
	}
	return res, nil
}

// detailJSON is the sweep's sync_report.detail. It is hand-rolled rather than
// marshalled because every value in it is an int this function produced, so
// there is no encoding error to handle and no upstream text to redact.
func (r SweepResult) detailJSON() string {
	return fmt.Sprintf(
		`{"links_live":%d,"links_tombstoned":%d,"works_tombstoned":%d,`+
			`"sources_missing":%d,"libraries_orphaned":%d,"libraries_restored":%d}`,
		r.LinksLive, r.LinksTombstoned, r.WorksTombstoned,
		r.SourcesMissing, r.LibrariesOrphaned, r.LibrariesRestored)
}

// absentLink is one live link the read did not report.
type absentLink struct {
	ref    LinkRef
	workID int64
}

// absentLinks is THE UNQUALIFIED INSTANCE SWEEP and the Go-side set difference.
//
// It is a function of its own so its cursor is CLOSED BEFORE THE FIRST UPDATE —
// the sweep never has a read open on service_item_link while it writes to it.
// 🔍 That the interleaving would actually misbehave is INFERENCE and is not
// measured here; what is certain is that nothing is bought by it, because the
// whole difference is in memory the moment the last row is scanned.
//
// The read has no predicate SQLite can seek on beyond the instance itself, and
// after ANALYZE that is correctly a SCAN — on a single-instance install the
// predicate selects the whole table, so an index seek would only add a lookup
// per row. There is no index to add here and none is wanted;
// TestTheInstanceSweepIsAllowedToScan records the acceptance.
func absentLinks(
	ctx context.Context, tx *sql.Tx, instanceID int64, seen map[LinkRef]struct{},
) ([]absentLink, int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT remote_kind, remote_id, work_id
		  FROM service_item_link
		 WHERE service_instance_id = ? AND deleted_at IS NULL`, instanceID)
	if err != nil {
		return nil, 0, fmt.Errorf("read live links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var absent []absentLink
	var live int
	for rows.Next() {
		var a absentLink
		if err := rows.Scan(&a.ref.RemoteKind, &a.ref.RemoteID, &a.workID); err != nil {
			return nil, 0, fmt.Errorf("scan live link: %w", err)
		}
		live++
		if _, ok := seen[a.ref]; !ok {
			absent = append(absent, a)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read live links: %w", err)
	}
	return absent, live, nil
}

// absentSources is the container half's set difference: the library_source rows
// on this instance that are not stamped missing and were not in the read. Its
// cursor is closed before any write, on absentLinks' rule.
//
// 'remote_library' is the only container_kind a catalogue source produces —
// bindOneContainer writes that literal and reads it back — so naming it here
// keeps the sweep off the root_folder and tag rows an *Arr sync will own.
func absentSources(
	ctx context.Context, tx *sql.Tx, instanceID int64, seen map[string]struct{},
) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, container_ref
		  FROM library_source
		 WHERE service_instance_id = ? AND container_kind = 'remote_library'
		   AND missing_since IS NULL`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("read sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var absent []int64
	for rows.Next() {
		var id int64
		var ref string
		if err := rows.Scan(&id, &ref); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		if _, ok := seen[ref]; !ok {
			absent = append(absent, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sources: %w", err)
	}
	return absent, nil
}

// fedLibraries is every library this instance is a source of, except the
// reserved Unfiled row. Its cursor is closed before any write, on absentLinks'
// rule.
//
// ⚠️ library 0 IS EXCLUDED. "Unfiled" has no sources by construction (see
// UnfiledLibraryID), so an orphan rule that tested it would mark the one library
// whose job is to always be there.
func fedLibraries(ctx context.Context, tx *sql.Tx, instanceID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT library_id FROM library_source
		 WHERE service_instance_id = ? AND library_id <> ?`, instanceID, UnfiledLibraryID)
	if err != nil {
		return nil, fmt.Errorf("read fed libraries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var libs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan fed library: %w", err)
		}
		libs = append(libs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read fed libraries: %w", err)
	}
	return libs, nil
}

// sweepItems tombstones the links the read did not see, and then the works that
// lost their last live link with them.
//
// THE WORK TOMBSTONE IS NOT A SECOND FEATURE. `work.deleted_at` is what
// browse.go, searchlibrary.go and recent.go all filter on; `service_item_link.
// deleted_at` is not read by any of the three. A pass that tombstoned only the
// link would move a column and change nothing a user can see, and would be
// measurable only by reading the column it wrote — which is the shape of a check
// that passes over its own deleted subject.
func sweepItems(
	ctx context.Context, tx *sql.Tx, instanceID int64,
	seen map[LinkRef]struct{}, nowStr string, res *SweepResult,
) error {
	absent, live, err := absentLinks(ctx, tx, instanceID, seen)
	if err != nil {
		return err
	}
	res.LinksLive = live

	for _, a := range absent {
		// remote_kind IS NAMED, and it is CORRECTNESS before it is a query plan.
		// ux_sil admits (instance, 'book', '9') and (instance, 'series', '9') as
		// two different rows, and ADR-0068's adapter writes both kinds — so an
		// UPDATE matching on the instance and the id alone tombstones a live item
		// because its neighbour went away, with no error to show for it. That it
		// is also the difference between a three-column seek and a skip-scan over
		// every link on the instance is the second reason. See LinkRef.
		r, err := tx.ExecContext(ctx, `
			UPDATE service_item_link SET deleted_at = ?
			 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?
			   AND deleted_at IS NULL`,
			nowStr, instanceID, a.ref.RemoteKind, a.ref.RemoteID)
		if err != nil {
			return fmt.Errorf("tombstone link %s %s: %w", a.ref.RemoteKind, a.ref.RemoteID, err)
		}
		n, err := r.RowsAffected()
		if err != nil {
			return fmt.Errorf("tombstone link %s %s: %w", a.ref.RemoteKind, a.ref.RemoteID, err)
		}
		res.LinksTombstoned += int(n)
	}

	// The work half, once per work rather than once per link: two links from the
	// same instance onto one work (§6.4 tier 1 resolves them together) would
	// otherwise ask the same question twice.
	done := make(map[int64]struct{}, len(absent))
	for _, a := range absent {
		if a.workID == 0 {
			continue
		}
		if _, ok := done[a.workID]; ok {
			continue
		}
		done[a.workID] = struct{}{}

		// ⚠️ THE NOT EXISTS IS THE POINT AND IT IS NOT INSTANCE-SCOPED. A work
		// reported by TWO instances loses one link and stays visible; only the
		// last live link anywhere makes the work itself gone. The subquery names
		// its table `sil_absent` rather than the obvious `l`, and the work id is
		// bound TWICE rather than correlated by name, so there is no alias for a
		// future edit to collide with — recent.go's scopeLinkAlias carries the
		// full argument for why that collision is silent when it happens.
		r, err := tx.ExecContext(ctx, `
			UPDATE work SET deleted_at = ?, updated_at = ?
			 WHERE id = ? AND deleted_at IS NULL
			   AND NOT EXISTS (SELECT 1 FROM service_item_link sil_absent
			                    WHERE sil_absent.work_id = ? AND sil_absent.deleted_at IS NULL)`,
			nowStr, nowStr, a.workID, a.workID)
		if err != nil {
			return fmt.Errorf("tombstone work %d: %w", a.workID, err)
		}
		n, err := r.RowsAffected()
		if err != nil {
			return fmt.Errorf("tombstone work %d: %w", a.workID, err)
		}
		res.WorksTombstoned += int(n)
	}
	return nil
}

// sweepContainers stamps library_source.missing_since for containers the read
// did not report, and then library.orphaned_at for libraries that no longer have
// a single source anybody still reports.
//
// # First observed absent, and why that is structural rather than a Go branch
//
// Migration 0005 says missing_since is when "the upstream stopped reporting this
// container" and orphaned_at is "set when the last library_source goes away" —
// both are the instant an absence BEGAN, so a sweep that re-stamped on every run
// would turn "missing since Tuesday" into "missing since thirty seconds ago" and
// destroy the only fact the column carries. `AND missing_since IS NULL` /
// `AND orphaned_at IS NULL` in the UPDATE is what enforces it: a caller cannot
// re-stamp by calling twice, because the second statement matches no row. The
// RowsAffected counts are therefore counts of TRANSITIONS, not of missing things.
//
// # The clearing arm
//
// bindOneContainer already clears missing_since when a container comes back, so
// there is no clearing arm for it here. There is one for orphaned_at, because
// nothing anywhere else clears that column and a flag that can only be set is a
// library permanently marked orphaned the moment its upstream blinks.
func sweepContainers(
	ctx context.Context, tx *sql.Tx, instanceID int64,
	seen map[string]struct{}, nowStr string, res *SweepResult,
) error {
	absent, err := absentSources(ctx, tx, instanceID, seen)
	if err != nil {
		return err
	}

	for _, id := range absent {
		r, err := tx.ExecContext(ctx, `
			UPDATE library_source SET missing_since = ?
			 WHERE id = ? AND missing_since IS NULL`, nowStr, id)
		if err != nil {
			return fmt.Errorf("stamp library_source %d missing: %w", id, err)
		}
		n, err := r.RowsAffected()
		if err != nil {
			return fmt.Errorf("stamp library_source %d missing: %w", id, err)
		}
		res.SourcesMissing += int(n)
	}

	return sweepOrphans(ctx, tx, instanceID, nowStr, res)
}

// sweepOrphans moves library.orphaned_at both ways for the libraries this
// instance feeds.
func sweepOrphans(
	ctx context.Context, tx *sql.Tx, instanceID int64, nowStr string, res *SweepResult,
) error {
	libs, err := fedLibraries(ctx, tx, instanceID)
	if err != nil {
		return err
	}

	for _, id := range libs {
		// NOT INSTANCE-SCOPED, on the work tombstone's rule: a library fed by
		// two instances is not orphaned because one of them went quiet. The
		// subquery's `ls_orphan` alias and the twice-bound id are there for the
		// reason sweepItems states.
		r, err := tx.ExecContext(ctx, `
			UPDATE library SET orphaned_at = ?, updated_at = ?
			 WHERE id = ? AND orphaned_at IS NULL
			   AND NOT EXISTS (SELECT 1 FROM library_source ls_orphan
			                    WHERE ls_orphan.library_id = ? AND ls_orphan.missing_since IS NULL)`,
			nowStr, nowStr, id, id)
		if err != nil {
			return fmt.Errorf("stamp library %d orphaned: %w", id, err)
		}
		n, err := r.RowsAffected()
		if err != nil {
			return fmt.Errorf("stamp library %d orphaned: %w", id, err)
		}
		res.LibrariesOrphaned += int(n)
		if n > 0 {
			continue
		}

		r, err = tx.ExecContext(ctx, `
			UPDATE library SET orphaned_at = NULL, updated_at = ?
			 WHERE id = ? AND orphaned_at IS NOT NULL
			   AND EXISTS (SELECT 1 FROM library_source ls_orphan
			                WHERE ls_orphan.library_id = ? AND ls_orphan.missing_since IS NULL)`,
			nowStr, id, id)
		if err != nil {
			return fmt.Errorf("clear library %d orphaned: %w", id, err)
		}
		n, err = r.RowsAffected()
		if err != nil {
			return fmt.Errorf("clear library %d orphaned: %w", id, err)
		}
		res.LibrariesRestored += int(n)
	}
	return nil
}
