package libsync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// fakeSource is the adapter seam, used where a cassette would only add noise:
// batching boundaries, a stream that dies mid-array, and a container list that
// fails. Every test that exercises the KAVITA MAPPING uses a cassette instead.
type fakeSource struct {
	containers []store.CatalogueContainer
	items      []store.CatalogueItem

	// failAfter aborts the stream after this many elements have been handed
	// over, reproducing StreamSeries's partial-delivery contract.
	failAfter int
	streamErr error
	listErr   error
}

func (f *fakeSource) Containers(context.Context) ([]store.CatalogueContainer, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.containers, nil
}

func (f *fakeSource) StreamItems(_ context.Context, fn func(store.CatalogueItem) error) (int, error) {
	n := 0
	for _, it := range f.items {
		if f.streamErr != nil && n == f.failAfter {
			return n, f.streamErr
		}
		if err := fn(it); err != nil {
			return n, err
		}
		n++
	}
	if f.streamErr != nil {
		return n, f.streamErr
	}
	return n, nil
}

func genItems(n int, container, kind string) []store.CatalogueItem {
	out := make([]store.CatalogueItem, 0, n)
	for i := range n {
		title := fmt.Sprintf("Title %04d", i)
		out = append(out, store.CatalogueItem{
			RemoteID: fmt.Sprint(i), RemoteKind: "series", ContainerID: container, Kind: kind,
			Title: title, SortTitle: title, NormalizedTitle: NormalizeTitle(title),
			NormVersion: NormVersion, AddedAt: testNow, RemoteUpdatedAt: testNow, HasFile: true,
		})
	}
	return out
}

func newImporter(t *testing.T, s *store.Store, src Source) *Importer {
	t.Helper()
	return &Importer{
		Store: s, Source: src, UserID: store.SystemUserID,
		Now: func() time.Time { return testNow },
	}
}

func countRows(t *testing.T, s *store.Store, q string) int {
	t.Helper()
	var n int
	if err := s.DB().Read().QueryRowContext(t.Context(), q).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

func TestFullImportFromTheCassettesEndToEnd(t *testing.T) {
	// THE WHOLE PATH, against a real migrated database and the synthetic
	// cassettes: two Kavita libraries, three series, no external ids anywhere —
	// which is the ORDINARY case on a free instance.
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := NewKavitaSource(newCassetteClient(t, "kavita_libraries.yaml", "kavita_series_all_v2.yaml"))

	var progress []Progress
	im := newImporter(t, s, src)
	im.Progress = func(p Progress) { progress = append(progress, p) }

	rep, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if !rep.Completed {
		t.Fatal("the import did not report itself complete")
	}
	if rep.ContainersSeen != 2 || rep.LibrariesCreated != 2 || len(rep.DeclinedContainers) != 0 {
		t.Errorf("containers: seen %d created %d declined %d",
			rep.ContainersSeen, rep.LibrariesCreated, len(rep.DeclinedContainers))
	}
	if rep.ItemsRead != 3 || rep.ItemsApplied != 3 || rep.WorksCreated != 3 {
		t.Errorf("items: read %d applied %d created %d, want 3/3/3",
			rep.ItemsRead, rep.ItemsApplied, rep.WorksCreated)
	}
	// Degraded identity, the ordinary case: three works, none identified, all
	// kept and all searchable.
	if rep.Unidentified != 3 || rep.ExternalIDsWritten != 0 {
		t.Errorf("identity: unidentified %d external ids %d, want 3 and 0 on a free instance",
			rep.Unidentified, rep.ExternalIDsWritten)
	}
	if rep.SearchDocs != 3 {
		t.Errorf("SearchDocs = %d, want 3", rep.SearchDocs)
	}

	if n := countRows(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'comic'`); n != 2 {
		t.Errorf("comic works = %d, want 2 (both from the Manga library)", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'book'`); n != 1 {
		t.Errorf("book works = %d, want 1", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM work_comic`); n != 2 {
		t.Errorf("work_comic rows = %d, want 2", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM work_book`); n != 1 {
		t.Errorf("work_book rows = %d, want 1", n)
	}
	// Nothing writes work_comic_issue or media_file in this commit, and the
	// reason is stated rather than left implicit: Kavita's series list carries
	// no chapter and no file facts at all. See kavita.go and doc.go.
	if n := countRows(t, s, `SELECT COUNT(*) FROM work_comic_issue`); n != 0 {
		t.Errorf("work_comic_issue rows = %d; channel 1's series read has no chapter facts to write", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM media_file`); n != 0 {
		t.Errorf("media_file rows = %d; POST /api/Series/all-v2 reports no file, no size and no path "+
			"to a file — only a series FOLDER, which is not a file", n)
	}

	// §7 invariants 2 and 5, over a POPULATED corpus.
	docs := countRows(t, s, `SELECT COUNT(*) FROM search_doc`)
	fts := countRows(t, s, `SELECT COUNT(*) FROM search_fts`)
	trgm := countRows(t, s, `SELECT COUNT(*) FROM search_trgm`)
	if docs != 3 || fts != 3 || trgm != 3 {
		t.Errorf("corpus = doc %d / fts %d / trgm %d, want 3/3/3", docs, fts, trgm)
	}
	unscoped := countRows(t, s, `
		SELECT COUNT(*) FROM search_doc d
		 WHERE NOT EXISTS (SELECT 1 FROM search_doc_library l WHERE l.doc_rowid = d.rowid)`)
	if unscoped != 0 {
		t.Errorf("%d docs have no library scope", unscoped)
	}

	// last_full_sync_at is written, and ANALYZE ran.
	at, err := s.LastFullSyncAt(t.Context(), inst)
	if err != nil {
		t.Fatalf("LastFullSyncAt: %v", err)
	}
	if !at.Valid {
		t.Error("a completed import left last_full_sync_at unset")
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM sqlite_schema WHERE name = 'sqlite_stat1'`); n != 1 {
		t.Error("ANALYZE did not run after the bulk import")
	}

	// Progress reached the callback with real counts and a terminal frame.
	if len(progress) < 2 {
		t.Fatalf("progress frames = %d, want at least a containers frame and a done frame", len(progress))
	}
	last := progress[len(progress)-1]
	if last.Phase != "done" || last.Applied != 3 {
		t.Errorf("last progress frame = %+v", last)
	}
}

func TestFullImportDeclinesTheImageLibraryAndSaysWhy(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := NewKavitaSource(newCassetteClient(t,
		"kavita_libraries_all_types.yaml", "kavita_series_all_v2_identified.yaml"))

	rep, err := newImporter(t, s, src).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if len(rep.DeclinedContainers) != 1 || rep.DeclinedContainers[0].RemoteID != "4" {
		t.Fatalf("declined = %+v, want exactly the Image library", rep.DeclinedContainers)
	}
	if rep.DeclinedContainers[0].Reason == "" {
		t.Error("§17.8 requires a declined container to carry its reason")
	}
	if rep.LibrariesCreated != 5 {
		t.Errorf("LibrariesCreated = %d, want 5 (six containers, one declined)", rep.LibrariesCreated)
	}
	// The decline is REPORTED to the operator, not only returned: an import
	// started by a background connect has no caller left to read the Report.
	var kind, remoteID, detail string
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT kind, remote_id, detail FROM sync_report WHERE kind = 'container_declined'`).
		Scan(&kind, &remoteID, &detail); err != nil {
		t.Fatalf("read sync_report: %v", err)
	}
	if remoteID != "4" || detail == "" {
		t.Errorf("sync_report row = (%q, %q, %q)", kind, remoteID, detail)
	}

	// No work was created for the declined library's series.
	if n := countRows(t, s, `SELECT COUNT(*) FROM work WHERE title = 'Desktop dump'`); n != 0 {
		t.Error("a series from the declined Image library became a work")
	}
	if rep.ItemsApplied != 5 {
		t.Errorf("ItemsApplied = %d, want 5", rep.ItemsApplied)
	}
	// Two series claim the same AniList id: tier 1 resolves them onto one work.
	if rep.WorksCreated != 4 || rep.WorksReused != 1 {
		t.Errorf("works: created %d reused %d, want 4 and 1 — the two Berserk rows are one work",
			rep.WorksCreated, rep.WorksReused)
	}
	if len(rep.IdentityConflicts) != 0 {
		t.Errorf("IdentityConflicts = %+v; tier 1 absorbs the in-batch case", rep.IdentityConflicts)
	}
}

func TestBatchesCommitAtTheRowBound(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := &fakeSource{
		containers: []store.CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}},
		items:      genItems(250, "1", "comic"),
	}
	im := newImporter(t, s, src)
	im.BatchRows = 100
	// A frozen clock: Now never advances, so the WALL-CLOCK bound can never
	// fire and this test measures the ROW bound alone.
	rep, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if rep.Batches != 3 {
		t.Errorf("Batches = %d, want 3 (100 + 100 + a 50-row tail)", rep.Batches)
	}
	if rep.ItemsApplied != 250 {
		t.Errorf("ItemsApplied = %d, want 250", rep.ItemsApplied)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM work`); n != 250 {
		t.Errorf("work rows = %d, want 250", n)
	}
}

func TestBatchesCommitAtTheWallClockBound(t *testing.T) {
	// reference/sync.md §6 rule 3 sizes a batch at min(2000 rows, 100 ms) and
	// the wall-clock half is the one that keeps the interactive lane moving. A
	// clock that jumps past the window on every element must produce one batch
	// per element even though the row bound is nowhere near.
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := &fakeSource{
		containers: []store.CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}},
		items:      genItems(5, "1", "comic"),
	}
	clock := testNow
	im := &Importer{
		Store: s, Source: src, UserID: store.SystemUserID,
		BatchRows:   2000,
		BatchWindow: 100 * time.Millisecond,
		Now: func() time.Time {
			clock = clock.Add(200 * time.Millisecond)
			return clock
		},
	}
	rep, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if rep.Batches != 5 {
		t.Errorf("Batches = %d, want 5: the wall-clock bound never fired, so a slow disk "+
			"would hold the single writer for the whole import", rep.Batches)
	}
}

func TestAPartialStreamKeepsWhatCommittedAndRefusesToClaimFreshness(t *testing.T) {
	// StreamSeries's partial-delivery contract, carried all the way up. A body
	// cut mid-array leaves the committed batches in place — rolling back a 60 MB
	// import over element 41,000 would mean nothing is ever imported from a
	// flaky instance — but last_full_sync_at MUST NOT be written, because the
	// Services screen renders it as "this replica is current".
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	boom := errors.New("stream ended mid-array")
	src := &fakeSource{
		containers: []store.CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}},
		items:      genItems(250, "1", "comic"),
		failAfter:  150,
		streamErr:  boom,
	}
	im := newImporter(t, s, src)
	im.BatchRows = 100

	rep, err := im.FullImport(t.Context(), inst)
	if err == nil {
		t.Fatal("a truncated stream returned no error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want the stream's own error wrapped", err)
	}
	if rep.Completed {
		t.Error("a partial import reported itself complete")
	}
	if rep.ItemsRead != 150 {
		t.Errorf("ItemsRead = %d, want 150: the count is how far the READ got", rep.ItemsRead)
	}
	if rep.ItemsApplied != 100 {
		t.Errorf("ItemsApplied = %d, want 100: one full batch committed, the 50-row tail did not",
			rep.ItemsApplied)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM work`); n != 100 {
		t.Errorf("work rows = %d, want 100 — committed batches stand", n)
	}
	at, err := s.LastFullSyncAt(t.Context(), inst)
	if err != nil {
		t.Fatalf("LastFullSyncAt: %v", err)
	}
	if at.Valid {
		t.Error("a PARTIAL import wrote last_full_sync_at, which claims a freshness the replica does not have")
	}
}

func TestAFailedContainerListStopsBeforeAnythingIsWritten(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	boom := errors.New("401 unauthorized")
	src := &fakeSource{listErr: boom}

	rep, err := newImporter(t, s, src).FullImport(t.Context(), inst)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the container list's own error", err)
	}
	if rep.Completed {
		t.Error("a failed import reported itself complete")
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM library WHERE id <> 0`); n != 0 {
		t.Errorf("%d libraries were created before the container list failed", n)
	}
}

func TestReImportIsIdempotentAcrossTheWholePath(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := &fakeSource{
		containers: []store.CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}},
		items:      genItems(40, "1", "comic"),
	}
	im := newImporter(t, s, src)
	for run := 1; run <= 3; run++ {
		if _, err := im.FullImport(t.Context(), inst); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}
	for _, tc := range []struct {
		q    string
		want int
	}{
		{`SELECT COUNT(*) FROM library WHERE id <> 0`, 1},
		{`SELECT COUNT(*) FROM library_source`, 1},
		{`SELECT COUNT(*) FROM work`, 40},
		{`SELECT COUNT(*) FROM library_member`, 40},
		{`SELECT COUNT(*) FROM service_item_link`, 40},
		{`SELECT COUNT(*) FROM search_doc`, 40},
		{`SELECT COUNT(*) FROM search_fts`, 40},
		{`SELECT COUNT(*) FROM search_trgm`, 40},
		{`SELECT COUNT(*) FROM search_doc_library`, 40},
	} {
		if n := countRows(t, s, tc.q); n != tc.want {
			t.Errorf("after three imports, %s = %d, want %d", tc.q, n, tc.want)
		}
	}
}

func TestReportDurationAndProgressShape(t *testing.T) {
	r := Report{StartedAt: testNow}
	if r.Duration() != 0 {
		t.Error("an unfinished report reported a duration")
	}
	r.FinishedAt = testNow.Add(time.Second)
	if r.Duration() != time.Second {
		t.Errorf("Duration = %v, want 1s", r.Duration())
	}
}
