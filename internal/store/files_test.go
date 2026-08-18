package store

import (
	"database/sql"
	"testing"
	"time"
)

// Every test in this file runs against a REAL MIGRATED DATABASE and POPULATES
// BEFORE IT ASSERTS, on catalogue_test.go's rule.

// fileFixture imports one comic and returns the instance and the work id, so
// that ApplyFiles has a service_item_link to resolve — the same reason
// creditFixture imports rather than inserting a work row.
func fileFixture(t *testing.T, s *Store) (inst, workID int64) {
	t.Helper()
	inst = fixtureInstance(t, s, "kavita-files")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	workID = importOne(t, s, inst, binds, item("41", "1", "comic", "Frieren"))
	return inst, workID
}

func mediaFile(path string, size int64) MediaFileRow {
	return MediaFileRow{
		Path: path, RemoteFileID: path, SizeBytes: size,
		DateAdded: time.Date(2026, 2, 11, 18, 22, 5, 0, time.UTC),
	}
}

func countFiles(t *testing.T, s *Store, workID int64) int {
	t.Helper()
	var n int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM media_file WHERE work_id = ?`, workID).Scan(&n); err != nil {
		t.Fatalf("count files: %v", err)
	}
	return n
}

// TestAFileWalkWritesRowsAndMarksTheWorkDirty is the happy path, and the dirty
// mark is the half that matters: media_file rows and have_count are written by
// two different writers (REVIEW-LOG.md LS-211.1), and this flag is the only
// thing connecting them.
func TestAFileWalkWritesRowsAndMarksTheWorkDirty(t *testing.T) {
	s := newTestStore(t)
	inst, workID := fileFixture(t, s)

	res, err := s.ApplyFiles(t.Context(), inst, []FileSet{{
		RemoteKind: "series", RemoteID: "41",
		Files: []MediaFileRow{
			mediaFile("kavita:mangafile:1", 62914560),
			mediaFile("kavita:mangafile:2", 60817408),
		},
		EditionFormat: sql.NullString{String: "cbz", Valid: true},
	}})
	if err != nil {
		t.Fatalf("ApplyFiles: %v", err)
	}
	if res.FilesWritten != 2 || res.EditionsCreated != 1 || res.WorksMarkedDirty != 1 {
		t.Errorf("result = %+v; want 2 files, 1 edition created, 1 work marked dirty", res)
	}
	if n := countFiles(t, s, workID); n != 2 {
		t.Errorf("media_file rows = %d, want 2", n)
	}

	var dirty int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT rollup_dirty FROM work WHERE id = ?`, workID).Scan(&dirty); err != nil {
		t.Fatalf("read rollup_dirty: %v", err)
	}
	if dirty != 1 {
		t.Errorf("work %d has rollup_dirty = %d; the flush reads ix_work_dirty and would never "+
			"see this work", workID, dirty)
	}

	// Every file points at the primary edition the same call created.
	var linked int
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT COUNT(*) FROM media_file f
		  JOIN edition e ON e.id = f.edition_id
		 WHERE f.work_id = ? AND e.is_primary = 1 AND e.format = 'cbz'`, workID).Scan(&linked); err != nil {
		t.Fatalf("read edition links: %v", err)
	}
	if linked != 2 {
		t.Errorf("%d of 2 files point at the primary cbz edition", linked)
	}
}

// TestFilesNotSeenThisPassAreDeleted fires the within-series reconciliation.
//
// IT IS THE GUARD AGAINST A COUNT THAT ONLY EVER GOES UP. Channel 4's sweep
// does not exist (internal/libsync/doc.go), so nothing else in the tree would
// ever remove the row of a chapter deleted upstream — and the rollup counts
// these rows, so a stale row is a permanently inflated have_count on a library
// that is shrinking.
func TestFilesNotSeenThisPassAreDeleted(t *testing.T) {
	s := newTestStore(t)
	inst, workID := fileFixture(t, s)

	if _, err := s.ApplyFiles(t.Context(), inst, []FileSet{{
		RemoteKind: "series", RemoteID: "41",
		Files: []MediaFileRow{
			mediaFile("kavita:mangafile:1", 10),
			mediaFile("kavita:mangafile:2", 20),
			mediaFile("kavita:mangafile:3", 30),
		},
	}}); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if n := countFiles(t, s, workID); n != 3 {
		t.Fatalf("first pass wrote %d files, want 3", n)
	}

	// The operator deleted chapter 3 in Kavita. The next walk reports two files.
	res, err := s.ApplyFiles(t.Context(), inst, []FileSet{{
		RemoteKind: "series", RemoteID: "41",
		Files: []MediaFileRow{
			mediaFile("kavita:mangafile:1", 10),
			mediaFile("kavita:mangafile:2", 20),
		},
	}})
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if res.FilesRemoved != 1 {
		t.Errorf("FilesRemoved = %d, want 1", res.FilesRemoved)
	}
	if n := countFiles(t, s, workID); n != 2 {
		t.Errorf("media_file rows = %d after the deletion, want 2", n)
	}
	var gone int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM media_file WHERE path = 'kavita:mangafile:3'`).Scan(&gone); err != nil {
		t.Fatalf("read the deleted row: %v", err)
	}
	if gone != 0 {
		t.Error("the file the walk did not report is still in the replica")
	}

	// And an EMPTY set removes the rest. "This series has no files" is a real
	// statement, not a reason to skip the set.
	res, err = s.ApplyFiles(t.Context(), inst, []FileSet{{RemoteKind: "series", RemoteID: "41"}})
	if err != nil {
		t.Fatalf("third pass: %v", err)
	}
	if res.FilesRemoved != 2 || countFiles(t, s, workID) != 0 {
		t.Errorf("an empty file set left %d rows (removed %d)", countFiles(t, s, workID), res.FilesRemoved)
	}
}

// TestARealPathIsRefused fires validateReplicaPath.
//
// The owner decided on 2026-08-18 that media_file.path is an opaque surrogate
// and that a host filesystem path never enters the replica. This is that rule
// as code rather than as a comment: the guard lives in the writer every adapter
// passes through, so the next adapter cannot forget it.
func TestARealPathIsRefused(t *testing.T) {
	s := newTestStore(t)
	inst, workID := fileFixture(t, s)

	res, err := s.ApplyFiles(t.Context(), inst, []FileSet{{
		RemoteKind: "series", RemoteID: "41",
		Files: []MediaFileRow{
			mediaFile("/mnt/user/media/manga/Frieren/Vol. 1.cbz", 10),
			mediaFile(`C:\media\manga\Frieren\Vol. 2.cbz`, 20),
			mediaFile("", 30),
			mediaFile("kavita:mangafile:9", 40),
		},
	}})
	if err != nil {
		t.Fatalf("ApplyFiles: %v", err)
	}
	if res.FilesRejected != 3 {
		t.Errorf("FilesRejected = %d, want 3 (a POSIX path, a Windows path and an empty string)",
			res.FilesRejected)
	}
	if res.FilesWritten != 1 {
		t.Errorf("FilesWritten = %d, want 1 (only the surrogate)", res.FilesWritten)
	}
	if n := countFiles(t, s, workID); n != 1 {
		t.Fatalf("media_file rows = %d, want 1", n)
	}
	var path string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT path FROM media_file WHERE work_id = ?`, workID).Scan(&path); err != nil {
		t.Fatalf("read the surviving path: %v", err)
	}
	if path != "kavita:mangafile:9" {
		t.Errorf("media_file.path = %q; a host filesystem path reached the replica", path)
	}

	// A rejected file is NOT counted as seen, so it cannot spare a stale row
	// from the reconciliation either.
	var total int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM media_file`).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 1 {
		t.Errorf("media_file has %d rows in total, want 1", total)
	}
}

// TestTheEditionIsResolvedNotReallocated fires the resolve-don't-reallocate
// rule across two imports.
//
// `edition` HAS NO UNIQUE INDEX (migration 0005 gives it only
// ix_edition_work), so an unconditional INSERT would add a row per import
// forever and leave every earlier media_file pointing at an orphaned duplicate.
// Two passes is the smallest test that can see it; one pass cannot.
func TestTheEditionIsResolvedNotReallocated(t *testing.T) {
	s := newTestStore(t)
	inst, workID := fileFixture(t, s)

	set := FileSet{
		RemoteKind: "series", RemoteID: "41",
		Files:         []MediaFileRow{mediaFile("kavita:mangafile:1", 10)},
		EditionFormat: sql.NullString{String: "cbz", Valid: true},
	}
	first, err := s.ApplyFiles(t.Context(), inst, []FileSet{set})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := s.ApplyFiles(t.Context(), inst, []FileSet{set})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if first.EditionsCreated != 1 || first.EditionsReused != 0 {
		t.Errorf("first import: created %d reused %d, want 1/0", first.EditionsCreated, first.EditionsReused)
	}
	if second.EditionsCreated != 0 || second.EditionsReused != 1 {
		t.Errorf("second import: created %d reused %d, want 0/1 — the edition was REALLOCATED",
			second.EditionsCreated, second.EditionsReused)
	}
	var n int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM edition WHERE work_id = ?`, workID).Scan(&n); err != nil {
		t.Fatalf("count editions: %v", err)
	}
	if n != 1 {
		t.Errorf("work %d has %d edition rows after two imports, want 1", workID, n)
	}

	// A format the upstream re-scanned is UPDATED in place rather than added
	// beside the old one.
	set.EditionFormat = sql.NullString{String: "cbr", Valid: true}
	if _, err := s.ApplyFiles(t.Context(), inst, []FileSet{set}); err != nil {
		t.Fatalf("third import: %v", err)
	}
	var format sql.NullString
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT format FROM edition WHERE work_id = ?`, workID).Scan(&format); err != nil {
		t.Fatalf("read format: %v", err)
	}
	if format.String != "cbr" {
		t.Errorf("edition.format = %v after a re-scan, want cbr", format)
	}
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM edition WHERE work_id = ?`, workID).Scan(&n); err != nil {
		t.Fatalf("count editions: %v", err)
	}
	if n != 1 {
		t.Errorf("a format change left %d edition rows, want 1", n)
	}
}

// TestMembershipStaysAtTheWholeWorkSentinel pins REVIEW-LOG.md LS-213.
//
// The edition writer's known trap: library_member's key is edition-grained and
// catalogue.go's item pass hardcodes edition_id = 0. Making membership
// edition-grained here would change what the Libraries screen's item_count
// MEANS, with nothing in the diff touching that screen.
func TestMembershipStaysAtTheWholeWorkSentinel(t *testing.T) {
	s := newTestStore(t)
	inst, workID := fileFixture(t, s)

	if _, err := s.ApplyFiles(t.Context(), inst, []FileSet{{
		RemoteKind: "series", RemoteID: "41",
		Files:         []MediaFileRow{mediaFile("kavita:mangafile:1", 10)},
		EditionFormat: sql.NullString{String: "cbz", Valid: true},
	}}); err != nil {
		t.Fatalf("ApplyFiles: %v", err)
	}

	var rows, sentinel int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*), COALESCE(SUM(edition_id = 0), 0) FROM library_member WHERE work_id = ?`,
		workID).Scan(&rows, &sentinel); err != nil {
		t.Fatalf("read membership: %v", err)
	}
	if rows != 1 || sentinel != 1 {
		t.Errorf("work %d has %d library_member rows, %d at the whole-work sentinel; an edition "+
			"writer must not make membership edition-grained (LS-213)", workID, rows, sentinel)
	}
}

// TestAnUnresolvedFileSetIsCountedNotFatal covers the item-stream-died case: a
// set whose remote item never reached a committed batch has nothing to attach
// to, and one such set must not cost the rest of the batch its files.
func TestAnUnresolvedFileSetIsCountedNotFatal(t *testing.T) {
	s := newTestStore(t)
	inst, workID := fileFixture(t, s)

	res, err := s.ApplyFiles(t.Context(), inst, []FileSet{
		{RemoteKind: "series", RemoteID: "9999", Files: []MediaFileRow{mediaFile("kavita:mangafile:8", 1)}},
		{RemoteKind: "series", RemoteID: "41", Files: []MediaFileRow{mediaFile("kavita:mangafile:1", 1)}},
	})
	if err != nil {
		t.Fatalf("ApplyFiles: %v", err)
	}
	if res.ItemsUnresolved != 1 {
		t.Errorf("ItemsUnresolved = %d, want 1", res.ItemsUnresolved)
	}
	if res.FilesWritten != 1 || countFiles(t, s, workID) != 1 {
		t.Errorf("the resolvable set lost its files: %+v", res)
	}
}

// TestTwoItemsOnOneWorkDoNotDeleteEachOthersFiles fires the batch-scoped
// reconciliation.
//
// Two upstream items in ONE instance CAN resolve onto one work — tier 1 matches
// them on the same strong external id — and a per-set delete would then have the
// second item's walk remove the first item's rows. This is why ApplyFiles
// accumulates the seen set across the whole call before it deletes anything.
func TestTwoItemsOnOneWorkDoNotDeleteEachOthersFiles(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita-shared-work")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	// The same strong identifier on both items, which is what makes tier 1
	// resolve the second onto the first's work.
	id := ExternalIdentifier{Source: "comicvine", Value: "12345", Confidence: 1.0}
	workA := importOne(t, s, inst, binds, item("41", "1", "comic", "Frieren", id))
	workB := importOne(t, s, inst, binds, item("42", "1", "comic", "Frieren (2020)", id))
	if workA != workB {
		t.Fatalf("the fixture did not produce one shared work: %d and %d", workA, workB)
	}

	res, err := s.ApplyFiles(t.Context(), inst, []FileSet{
		{RemoteKind: "series", RemoteID: "41", Files: []MediaFileRow{mediaFile("kavita:mangafile:1", 1)}},
		{RemoteKind: "series", RemoteID: "42", Files: []MediaFileRow{mediaFile("kavita:mangafile:2", 2)}},
	})
	if err != nil {
		t.Fatalf("ApplyFiles: %v", err)
	}
	if res.FilesRemoved != 0 {
		t.Errorf("FilesRemoved = %d; the two items' walks deleted each other's rows", res.FilesRemoved)
	}
	if n := countFiles(t, s, workA); n != 2 {
		t.Errorf("the shared work has %d files, want 2", n)
	}
}
