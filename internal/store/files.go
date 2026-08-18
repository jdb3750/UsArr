package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The file write path: how one upstream item's files become media_file rows,
// what an `edition` row costs, and the two rules that make the pass safe to run
// on every import.
//
// # media_file.path is an OPAQUE SURROGATE, not a host filesystem path
//
// ⚠️ OWNER DECISION, 2026-08-18. The column is `NOT NULL` and it is half of
// `ux_file_path(service_instance_id, path)`, so every row needs a value that is
// stable and unique per instance. The obvious candidate — the upstream's own
// `filePath` — is a HOST FILESYSTEM PATH on the media server's box, and this
// project already refuses that class of value in three other places
// (kavita.SeriesView drops folderPath, kavita.LibraryView drops folders,
// internal/servarr's SystemResource skip-list refuses "host filesystem paths
// UsArr must not store"). So the adapters mint a surrogate keyed on the
// upstream's own file id instead — `kavita:mangafile:1187` — and the real path
// never enters the replica at all.
//
// NOTHING IS LOST BY IT, and that is checked rather than assumed. The one
// argument for storing real paths was cross-instance dedup, and
// docs/reference/schema.md §3 has already WITHDRAWN it: *"Two observations of
// one physical file are reconciled by `content_key`, once it exists … Container
// mount points differ per service, so `path` equality is not a substitute."*
// The surrogate satisfies both things the column is actually load-bearing for —
// `NOT NULL`, and per-instance identity under `ux_file_path` — and nothing on
// any v0.1 screen renders a file path.
//
// validateReplicaPath is the guard, and it is in the WRITER rather than in each
// adapter on purpose: a rule enforced once at the boundary every adapter passes
// through cannot be forgotten by the next adapter.
//
// # Within-series reconciliation happens HERE, because channel 4 does not exist
//
// internal/libsync/doc.go: channel 4, the reconciliation sweep, is not built —
// "nothing here tombstones, deletes or detects drift". That is survivable for
// works, whose rows are upserted by identity. It is NOT survivable for files:
// a chapter deleted upstream leaves its media_file row behind forever, and the
// rollup that counts those rows then ratchets have_count upward on every import
// for a library that is shrinking. So this pass deletes the rows it did not see
// — scoped to the (work, instance) pairs it actually walked, and to nothing
// else. It is not a sweep and it does not pretend to be one: it reconciles only
// what it read.
//
// It is a replication write and takes no Scope, on catalogue.go's header's
// reasoning: it is driven by a background importer with no acting user.

// MediaFileRow is one file, as an adapter projected it. Like CatalogueItem and
// Credit it is NOT an upstream DTO — internal/store never sees a
// kavita.MangaFileDto.
type MediaFileRow struct {
	// Path is the OPAQUE SURROGATE — see the file header. It is validated, and
	// a value that looks like a real filesystem path is rejected rather than
	// stored.
	Path string

	// RemoteFileID is the upstream's own file id, verbatim
	// (media_file.remote_file_id). It is the value Path is derived from, kept
	// in its own column so a later reader does not have to parse the surrogate
	// back apart.
	RemoteFileID string

	// SizeBytes lands on media_file.size_bytes.
	SizeBytes int64

	// DateAdded lands on media_file.date_added. A zero time is stored as NULL:
	// the upstream not reporting a date is not a date.
	DateAdded time.Time
}

// FileSet is every file one already-replicated item carries.
//
// It is keyed by the UPSTREAM's identifiers rather than by a work id, for
// CreditSet's reason: the walk runs after the item stream has closed and
// re-resolves through service_item_link, so no work id is carried across two
// transactions.
type FileSet struct {
	RemoteKind string
	RemoteID   string

	// Files is the COMPLETE set this item currently has. An empty slice is a
	// real statement — "this item has no files" — and it reconciles the
	// item's rows away, exactly as an empty credit set removes credits.
	Files []MediaFileRow

	// EditionFormat is a member of edition.format's CHECK, or INVALID when the
	// upstream's own format has no honest member. NULL is legal in that column
	// and the CHECK still bites on a misspelling (migration 0005), so an
	// unknown format is stored as unknown rather than guessed.
	EditionFormat sql.NullString
}

// FileResult is what one ApplyFiles call did.
type FileResult struct {
	// FilesWritten counts rows inserted or updated. It is NOT "new files": a
	// re-import of an unchanged library rewrites every row and reports them
	// all, because ON CONFLICT DO UPDATE cannot distinguish the two and an
	// invented distinction would be worse than a plain count of work done.
	FilesWritten int

	// FilesRemoved counts rows deleted because this pass did not see them —
	// the within-series reconciliation the file header describes.
	FilesRemoved int

	// FilesRejected counts files refused by validateReplicaPath. On a healthy
	// adapter it is 0 forever; a non-zero value means an adapter tried to store
	// a host filesystem path, which is a code bug and is surfaced rather than
	// silently dropped.
	FilesRejected int

	// EditionsCreated and EditionsReused split the primary-edition resolution.
	// `edition` has NO unique index, so a writer that inserted unconditionally
	// would add a row per import forever; EditionsReused being equal to the
	// item count on a second import is what proves it does not.
	EditionsCreated int
	EditionsReused  int

	// ItemsUnresolved counts sets whose remote item has no service_item_link,
	// on CreditResult.ItemsUnresolved's terms.
	ItemsUnresolved int

	// WorksMarkedDirty counts works this call set rollup_dirty on. Those works
	// are what the next rollup flush re-aggregates; nothing here computes a
	// count (ARCHITECTURE.md §6.3: never re-aggregate per child write).
	WorksMarkedDirty int
}

func (r *FileResult) add(o FileResult) {
	r.FilesWritten += o.FilesWritten
	r.FilesRemoved += o.FilesRemoved
	r.FilesRejected += o.FilesRejected
	r.EditionsCreated += o.EditionsCreated
	r.EditionsReused += o.EditionsReused
	r.ItemsUnresolved += o.ItemsUnresolved
	r.WorksMarkedDirty += o.WorksMarkedDirty
}

// Add merges another result into r, so an importer can accumulate across
// batches without reimplementing the sum.
func (r *FileResult) Add(o FileResult) { r.add(o) }

// validateReplicaPath is the guard that keeps a host filesystem path out of
// media_file.path.
//
// THE RULE IT ENFORCES IS "NO PATH SEPARATOR", and what that does and does not
// catch is worth stating. It catches every shape the thing being guarded
// against actually has: Kavita's `filePath` is the scanner's absolute path, so
// it is `/mnt/user/media/manga/Frieren/Vol. 1.cbz` on Linux and
// `C:\media\manga\…` on Windows, and both are rejected on their separators. It
// does NOT catch a bare filename, and it is not meant to be a general "is this a
// path" oracle — that is not decidable from a string.
//
// ⚠️ THE MILESTONE THAT ADDS AN ADAPTER REPORTING REAL PATHS MUST CHANGE THIS
// DELIBERATELY. No adapter in the tree reports one today; Radarr and Jellyfin
// will, and schema.md §3's own example row is a real path. This guard is what
// makes that a decision somebody takes rather than a property that erodes.
func validateReplicaPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("media_file.path is empty; every row needs a surrogate " +
			"(the column is NOT NULL and half of ux_file_path)")
	}
	if strings.ContainsAny(path, `/\`) {
		return fmt.Errorf("media_file.path %q contains a path separator: v0.1 stores an OPAQUE "+
			"SURROGATE keyed on the upstream's file id, never a host filesystem path "+
			"(internal/store/files.go's header carries the owner decision)", path)
	}
	return nil
}

// ApplyFiles writes one batch of file sets in ONE BEGIN IMMEDIATE transaction,
// on ApplyCredits's sizing and for its reasons.
//
// # Why reconciliation is deferred to the end of the batch
//
// The delete is per (work_id, service_instance_id) — the pair the header
// explains — and the set of paths it spares is accumulated across the WHOLE
// call rather than applied per set. That matters in exactly one case and it is
// a reachable one: two upstream items in ONE instance can resolve onto ONE work
// (tier 1 matches them on the same strong external id), and a per-set delete
// would then have the second item's walk remove the first item's rows. Batching
// the reconciliation makes them additive.
//
// ⚠️ IT IS BATCH-SCOPED, NOT IMPORT-SCOPED, and the residue is recorded rather
// than hidden: two such items split across two batches still fight, and the
// later batch wins. The fix for that is the same one everything else here is
// waiting on — channel 4's sweep, which reconciles against the upstream rather
// than against one batch's memory.
// ⚠️ IT TAKES NO `now`, unlike ApplyCatalogueBatch and ApplyCredits, and the
// asymmetry is deliberate rather than an omission. Every timestamp this pass
// writes is the UPSTREAM's: media_file.date_added is the file's own `created`,
// and a file the upstream reports no date for is stored as NULL rather than as
// "now" — "when UsArr noticed it" is not when it was added. Nothing else here
// has a clock: the dirty mark is a flag, and work.updated_at is deliberately not
// bumped, because a re-import that rewrites identical rows must not churn it
// (credits.go's `year IS NOT ?` guard exists for the same reason).
func (s *Store) ApplyFiles(
	ctx context.Context, instanceID int64, sets []FileSet,
) (FileResult, error) {
	var res FileResult
	if len(sets) == 0 {
		return res, nil
	}
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res = FileResult{}

		// seen is (work id → the paths this CALL wrote for it), and order keeps
		// the reconciliation deterministic. A map alone would delete in map
		// order, which makes a failure's error message depend on the run.
		seen := map[int64]map[string]bool{}
		var order []int64

		for _, set := range sets {
			one, workID, err := applyOneFileSet(ctx, tx, instanceID, set)
			if err != nil {
				return fmt.Errorf("apply files for %s %s from service_instance %d: %w",
					set.RemoteKind, set.RemoteID, instanceID, err)
			}
			res.add(one)
			if workID == 0 {
				continue
			}
			if _, ok := seen[workID]; !ok {
				seen[workID] = map[string]bool{}
				order = append(order, workID)
			}
			for _, f := range set.Files {
				if validateReplicaPath(f.Path) == nil {
					seen[workID][f.Path] = true
				}
			}
		}

		for _, workID := range order {
			removed, err := reconcileFiles(ctx, tx, instanceID, workID, seen[workID])
			if err != nil {
				return err
			}
			res.FilesRemoved += removed

			// THE DIRTY MARK, in the same transaction as the child writes —
			// ARCHITECTURE.md §6.3's mechanism, and reference/sync.md's rule
			// that a child write never re-aggregates. It is set for every work
			// this call touched, including one whose files did not change:
			// "did anything change" is not knowable from an upsert's row count,
			// and a flush over a clean work is cheap while a missed dirty mark
			// is a count that stays wrong until the next import.
			r, err := tx.ExecContext(ctx, `UPDATE work SET rollup_dirty = 1 WHERE id = ?`, workID)
			if err != nil {
				return fmt.Errorf("mark work %d dirty: %w", workID, err)
			}
			if n, err := r.RowsAffected(); err == nil && n > 0 {
				res.WorksMarkedDirty++
			}
		}
		return nil
	})
	return res, err
}

// applyOneFileSet writes one item's files and returns the work they landed on.
// A returned work id of 0 means the item resolved to nothing.
func applyOneFileSet(
	ctx context.Context, tx *sql.Tx, instanceID int64, set FileSet,
) (FileResult, int64, error) {
	var res FileResult

	// ── 1. Re-resolve the work through the link the item pass wrote.
	var workID int64
	err := tx.QueryRowContext(ctx, `
		SELECT work_id FROM service_item_link
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?
		   AND deleted_at IS NULL`,
		instanceID, set.RemoteKind, set.RemoteID).Scan(&workID)
	if errors.Is(err, sql.ErrNoRows) {
		res.ItemsUnresolved++
		return res, 0, nil
	}
	if err != nil {
		return res, 0, fmt.Errorf("look up link: %w", err)
	}

	// ── 2. The primary edition, resolved rather than reallocated. An item with
	// no files gets no edition: an edition row whose only purpose is to be the
	// parent of files nobody has is a row about nothing.
	var editionID sql.NullInt64
	if len(set.Files) > 0 {
		id, created, err := primaryEditionID(ctx, tx, workID, set.EditionFormat)
		if err != nil {
			return res, workID, err
		}
		editionID = sql.NullInt64{Int64: id, Valid: true}
		if created {
			res.EditionsCreated++
		} else {
			res.EditionsReused++
		}
	}

	// ── 3. The files.
	//
	// content_key stays NULL. It is defined as hex(size) || ':' || sha256 of
	// the first 64 KiB (schema.md §3), which is a READ OF THE FILE'S BYTES, and
	// ADR-0026 decides UsArr never touches a filesystem. Kavita's own
	// koreaderHash is a different algorithm over a different input and writing
	// it here would make two instances' rows for one file compare unequal while
	// looking comparable.
	//
	// provenance_id stays NULL. A row replicated from a media server has no
	// acquisition history — provenance is what the *Arr grab path writes. The
	// column's NO ACTION foreign key is DELIBERATE (migration 0005 explains it
	// at length) and is not to be "fixed" to SET NULL.
	for _, f := range set.Files {
		if err := validateReplicaPath(f.Path); err != nil {
			res.FilesRejected++
			continue
		}
		r, execErr := tx.ExecContext(ctx, `
			INSERT INTO media_file (
			  work_id, edition_id, service_instance_id, remote_file_id, path,
			  size_bytes, date_added)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT (service_instance_id, path) DO UPDATE SET
			  work_id = excluded.work_id,
			  edition_id = excluded.edition_id,
			  remote_file_id = excluded.remote_file_id,
			  size_bytes = excluded.size_bytes,
			  date_added = excluded.date_added`,
			workID, editionID, instanceID, nullString(f.RemoteFileID), f.Path,
			f.SizeBytes, nullTime(f.DateAdded))
		if execErr != nil {
			return res, workID, fmt.Errorf("upsert file %q: %w", f.Path, execErr)
		}
		if n, err := r.RowsAffected(); err == nil && n > 0 {
			res.FilesWritten++
		}
	}
	return res, workID, nil
}

// reconcileFiles deletes this (work, instance)'s rows that the pass did not see.
func reconcileFiles(
	ctx context.Context, tx *sql.Tx, instanceID, workID int64, keep map[string]bool,
) (int, error) {
	args := make([]any, 0, len(keep)+2)
	args = append(args, workID, instanceID)
	for p := range keep {
		args = append(args, p)
	}
	r, err := tx.ExecContext(ctx, reconcileFilesSQL(len(keep)), args...)
	if err != nil {
		return 0, fmt.Errorf("reconcile files of work %d on service_instance %d: %w", workID, instanceID, err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reconcile files of work %d on service_instance %d: %w", workID, instanceID, err)
	}
	return int(n), nil
}

// reconcileFilesSQL renders the reconciliation statement for n spared paths.
//
// It is a function rather than a string built in place, on recentWorksSQL's
// rule: the statement that SHIPS is the one a test can EXPLAIN, and the only
// variable part is the count of `?` placeholders — never a value.
func reconcileFilesSQL(n int) string {
	if n == 0 {
		return `DELETE FROM media_file WHERE work_id = ? AND service_instance_id = ?`
	}
	return `DELETE FROM media_file
		 WHERE work_id = ? AND service_instance_id = ?
		   AND path NOT IN (` + placeholders(n) + `)`
}

// primaryEditionID resolves the work's primary edition, creating one only when
// none exists.
//
// ⚠️ `edition` HAS NO UNIQUE INDEX. Migration 0005 gives it
// `ix_edition_work(work_id, is_primary DESC)` and nothing more, so a naive
// INSERT duplicates the row on every re-import — a library of 151 series would
// grow 151 edition rows a sync, forever, and every media_file would point at
// whichever duplicate was newest. RESOLVE, DON'T REALLOCATE is the precedent
// REVIEW-LOG.md C-18 records for exactly this shape, and personWorkID in
// credits.go is the same pattern against the same absence.
//
// The lookup key is (work_id, is_primary = 1) — the "this is the edition the
// work is normally read as" row, which is what a media server reports and the
// only one a v0.1 adapter can name. A source that reports several editions
// (Navidrome's remasters, ADR-0031) resolves them by their own identifiers and
// is a different function.
//
// The format is UPDATED when it changes, because a library re-scanned from CBZ
// to CBR is a real event and the alternative is a row that says the wrong thing
// forever. `format IS NOT ?` — never `<>` — for the reason credits.go's year
// update spells out: SQLite's IS NOT compares NULLs as values, so writing the
// first real format onto a NULL is not skipped.
func primaryEditionID(
	ctx context.Context, tx *sql.Tx, workID int64, format sql.NullString,
) (id int64, created bool, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM edition
		 WHERE work_id = ? AND is_primary = 1
		 ORDER BY id LIMIT 1`, workID).Scan(&id)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx, `
			UPDATE edition SET format = ? WHERE id = ? AND format IS NOT ?`,
			format, id, format); err != nil {
			return 0, false, fmt.Errorf("update edition %d of work %d: %w", id, workID, err)
		}
		return id, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, false, fmt.Errorf("look up primary edition of work %d: %w", workID, err)
	}

	r, execErr := tx.ExecContext(ctx, `
		INSERT INTO edition (work_id, format, is_primary) VALUES (?,?,1)`, workID, format)
	if execErr != nil {
		return 0, false, fmt.Errorf("create primary edition of work %d: %w", workID, execErr)
	}
	id, execErr = r.LastInsertId()
	if execErr != nil {
		return 0, false, fmt.Errorf("create primary edition of work %d: id: %w", workID, execErr)
	}
	return id, true, nil
}

// ⚠️ WHAT THIS FILE DELIBERATELY DOES NOT TOUCH: library_member.
//
// The table's key is edition-grained — `(library_id, sort_title, work_id,
// edition_id)` — and catalogue.go's item pass hardcodes `edition_id = 0`, the
// "whole work, format-independent" sentinel. internal/store/libraries.go:253-259
// says in its own comment what happens if that changes: *"When an
// edition-grained writer lands, a work with two editions in one library will
// count twice."*
//
// ✅ MEMBERSHIP STAYS AT THE SENTINEL (REVIEW-LOG.md LS-213). An edition writer
// landing is NOT a reason to make membership edition-grained, for two reasons:
// the Libraries screen's item_count would silently change meaning — 151 today,
// some larger number tomorrow, with nothing in the diff touching that screen —
// and the obvious repair (COUNT(DISTINCT work_id)) costs the covering seek
// TestListLibrariesPlanIsSeeks pins. The seam stays open in both keys; it is
// simply not spent here.
