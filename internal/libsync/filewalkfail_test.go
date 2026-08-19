package libsync

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// The guards for a file walk that FAILED on one item.
//
// Three properties, and they are three because breaking any one of them is a
// different bug:
//
//  1. the failure is COUNTED, RECORDED and LOGGED, and the walk carries on;
//  2. the failure DELETES NOTHING — the deliberate half, which nothing guarded
//     until this file existed;
//  3. nothing the upstream SAID reaches the durable row or the log line.
//
// ⚠️ THE WALK UNDER TEST IS THE REAL ONE. These are importer-level tests and the
// obvious shortcut would be a fake FileSource that reports a failure directly —
// but every one of the three properties lives in KavitaSource.StreamFiles, so a
// fake would leave the production code free to break with the tests green. Only
// Containers and StreamItems are faked; the file phase runs the adapter over a
// fake HTTP client.

// planted is a secret-shaped string put where a real Kavita would put a message
// it parsed out of a response body. R-08 is why it is worth a test: a Mylar3
// returns indexer API keys in a response BODY, and ssrf.RedactText finds
// credentials inside URLs — its own doc says "a bare secret that is not inside a
// URL passes through untouched", so a redactor is NOT what makes this safe.
// walkFailureReason throwing the text away is.
const planted = "0123456789abcdefdeadbeefcafef00d"

// realWalkSource fakes the container list and the item stream and runs the REAL
// KavitaSource for the file phase. Composition rather than embedding, because
// both halves declare Containers and StreamItems and an embedded pair would be
// an ambiguous selector rather than an override.
type realWalkSource struct {
	fakeSource
	kav *KavitaSource
}

func (r *realWalkSource) StreamFiles(
	ctx context.Context, reqs []FileRequest, fn func(store.FileSet) error,
) (int, []FileWalkFailure, error) {
	return r.kav.StreamFiles(ctx, reqs, fn)
}

// threeSeriesWalk is three importable items whose walks all succeed, except for
// the ids named in fail.
func threeSeriesWalk(fail map[int32]error, log *slog.Logger) *realWalkSource {
	vols := map[int32][]kavita.VolumeDto{}
	for i := int32(0); i < 3; i++ {
		// A distinct file id per series: media_file.path is the surrogate minted
		// from it, and ux_file_path is unique per instance.
		vols[i] = oneFileVolume(100+i, ".cbz")
	}
	kav := NewKavitaSource(&fakeVolumeClient{vols: vols, fail: fail})
	kav.Log = log
	return &realWalkSource{
		fakeSource: fakeSource{
			containers: []store.CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}},
			// genItems numbers remote ids from 0, which is what the walk parses
			// back into a Kavita series id.
			items: genItems(3, "1", "comic"),
		},
		kav: kav,
	}
}

// filesOfRemote counts the media_file rows behind one upstream item id.
func filesOfRemote(t *testing.T, s *store.Store, inst int64, remoteID string) int {
	t.Helper()
	var n int
	err := s.DB().Read().QueryRowContext(t.Context(), `
		SELECT COUNT(*)
		  FROM media_file f
		  JOIN service_item_link l ON l.work_id = f.work_id
		 WHERE l.service_instance_id = ? AND l.remote_id = ? AND l.deleted_at IS NULL`,
		inst, remoteID).Scan(&n)
	if err != nil {
		t.Fatalf("count files of remote %s: %v", remoteID, err)
	}
	return n
}

// TestAFailedFileWalkIsCountedRecordedAndSurvived is the whole of property 1.
//
// ⚠️ IT REPLACES A SILENCE, NOT A WRONG NUMBER. StreamFiles used to drop a failed
// series on a bare `continue`: no counter, no sync_report row, no log line. The
// consequence was on a screen rather than in a log — the item ends up with no
// rollup and renders http-api.md §1.4.1's "not counted yet", which is the SAME
// rendering as an item that walked clean and holds nothing.
func TestAFailedFileWalkIsCountedRecordedAndSurvived(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)

	var logbuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logbuf, nil))
	src := threeSeriesWalk(map[int32]error{1: &kavita.APIError{
		Op: "SeriesVolumes", Method: "GET", Path: "/api/Series/volumes",
		Status: 503, Err: kavita.ErrServer,
	}}, log)

	im := newImporter(t, s, src)
	im.Log = log

	rep, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("one failed walk failed the whole import: %v", err)
	}
	if !rep.Completed {
		t.Error("one failed walk stopped the import completing; it is not an import failure")
	}

	// Counted.
	if rep.FileReadFailures != 1 {
		t.Errorf("Report.FileReadFailures = %d, want 1", rep.FileReadFailures)
	}
	// And the walk carried on past it: the other two items still delivered.
	if rep.FileItemsRead != 2 {
		t.Errorf("FileItemsRead = %d, want 2 — the walk stopped at the failure", rep.FileItemsRead)
	}
	for _, id := range []string{"0", "2"} {
		if got := filesOfRemote(t, s, inst, id); got != 1 {
			t.Errorf("item %s has %d files, want 1 — the walk did not carry on past the failure",
				id, got)
		}
	}

	// Recorded, durably, because the Report has no reader on a background sync.
	var remoteKind, remoteID, detail string
	err = s.DB().Read().QueryRowContext(t.Context(), `
		SELECT remote_kind, remote_id, detail FROM sync_report WHERE kind = ?`,
		store.SyncReportFileWalkFailed).Scan(&remoteKind, &remoteID, &detail)
	if err != nil {
		t.Fatalf("no %s row was written: %v", store.SyncReportFileWalkFailed, err)
	}
	if remoteKind != "series" || remoteID != "1" {
		t.Errorf("row names %s/%s, want series/1", remoteKind, remoteID)
	}
	for _, want := range []string{`"reason":"server_error"`, `"status":503`, `"phase":"files"`} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %s does not carry %s", detail, want)
		}
	}

	// Logged — the adapter's per-series line, and the import's own summary.
	out := logbuf.String()
	if !strings.Contains(out, `msg="kavita: a series' file walk failed and was dropped"`) ||
		!strings.Contains(out, "remote_id=1") {
		t.Errorf("the dropped series was not logged:\n%s", out)
	}
	if !strings.Contains(out, "file_read_failures=1") {
		t.Errorf("the import's own summary line does not carry the count:\n%s", out)
	}
}

// TestAFailedFileWalkDeletesNothing is property 2 — THE DELIBERATE HALF.
//
// The store reconciles a work's files against what the pass reported, so
// delivering a failed series as an empty set would drive its file count to zero
// and its availability blob to "0 held" on a series the user demonstrably has on
// disk. StreamFiles drops it instead, and this test exists because the drop is
// now surrounded by new code that could plausibly start "reporting" the failure
// by handing an empty set downstream.
func TestAFailedFileWalkDeletesNothing(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	discard := slog.New(slog.DiscardHandler)

	// Import one: every walk succeeds, so all three items have a file row.
	im := newImporter(t, s, threeSeriesWalk(nil, discard))
	if _, err := im.FullImport(t.Context(), inst); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if got := filesOfRemote(t, s, inst, "1"); got != 1 {
		t.Fatalf("setup: item 1 has %d files after a clean import, want 1", got)
	}

	// Import two: item 1's walk fails. Its row must still be there.
	im2 := newImporter(t, s, threeSeriesWalk(map[int32]error{1: kavita.ErrNotFound}, discard))
	rep, err := im2.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if rep.FileReadFailures != 1 {
		t.Fatalf("setup: the second import reported %d failures, want 1", rep.FileReadFailures)
	}
	if got := filesOfRemote(t, s, inst, "1"); got != 1 {
		t.Errorf("a failed read removed the item's file rows: %d left, want 1", got)
	}
	if rep.Files.FilesRemoved != 0 {
		t.Errorf("a failed read removed %d file rows, want 0", rep.Files.FilesRemoved)
	}
}

// TestWalkFailureCarriesNoUpstreamText is property 3, end to end: what the
// adapter classifies, what it logs, and what lands in sync_report.detail.
func TestWalkFailureCarriesNoUpstreamText(t *testing.T) {
	// What a Kavita hands back: a message parsed out of a response body, which
	// this fixture makes secret-shaped on purpose.
	upstream := &kavita.APIError{
		Op: "SeriesVolumes", Method: "GET", Path: "/api/Series/volumes",
		Status: 500, Message: "backend rejected key " + planted,
		Err: kavita.ErrServer,
	}
	if !strings.Contains(upstream.Error(), planted) {
		t.Fatalf("the fixture does not actually carry the secret: %v", upstream)
	}

	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	var logbuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logbuf, nil))

	im := newImporter(t, s, threeSeriesWalk(map[int32]error{1: upstream}, log))
	im.Log = log
	rep, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if rep.FileReadFailures != 1 {
		t.Fatalf("setup: reported %d failures, want 1", rep.FileReadFailures)
	}

	var detail sql.NullString
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT detail FROM sync_report WHERE kind = ?`,
		store.SyncReportFileWalkFailed).Scan(&detail); err != nil {
		t.Fatalf("read the row back: %v", err)
	}
	if !strings.Contains(detail.String, `"reason":"server_error"`) {
		t.Errorf("detail %s does not carry the classified reason", detail.String)
	}

	// The sweep: the durable row, and every line the run logged.
	assertNoPlantedSecret(t, "sync_report.detail", detail.String)
	assertNoPlantedSecret(t, "the log", logbuf.String())
}

func assertNoPlantedSecret(t *testing.T, where, got string) {
	t.Helper()
	if strings.Contains(got, planted) {
		t.Errorf("%s carries the planted secret: %s", where, got)
	}
	// The message it was embedded in must not survive either: a redactor that
	// only removed the key would leave the body text, and the next body will
	// carry its secret in a shape no deny-list knows.
	if strings.Contains(got, "backend rejected key") {
		t.Errorf("%s carries upstream response text: %s", where, got)
	}
}
