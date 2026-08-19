package libsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/imagepipeline"
	"github.com/jdb3750/UsArr/internal/store"
)

// TestTheRealPipelineSatisfiesPosterFetcher is a COMPILE-TIME proof and has no
// body worth running.
//
// *imagepipeline.Pipeline satisfies PosterFetcher structurally, with no adapter
// between them — the same relationship imagepipeline.CoverSource has with
// *bookorbit.Client, and asserted the same way, so the day either signature
// moves the build says so rather than a wrapper silently absorbing it.
var _ PosterFetcher = (*imagepipeline.Pipeline)(nil)

// fakeCovers is the PosterFetcher seam.
//
// ⚠️ IT COUNTS ITS CALLS, and every drill below asserts the count rather than
// only the outcome. A bound drill that asserts "no more than N ran at once"
// passes trivially when the answer is zero — and zero is exactly what a pass
// that silently skipped everything produces — so "the fetcher was reached, this
// many times, with these items" is the assertion that makes the rest mean
// anything.
type fakeCovers struct {
	mu    sync.Mutex
	calls []coverRequest

	// answer is the error to return per remote id. A missing entry succeeds.
	answer map[string]error

	// bytes is CachedBytes on a successful Result.
	bytes int

	// entered receives one value as each call BEGINS, before it can return. It
	// is what lets a test observe a fetch that is in flight without waiting a
	// wall-clock interval for one.
	entered chan coverRequest

	// release, when non-nil, holds every call open until it is closed.
	release chan struct{}

	// during runs inside the call. It is how the "no open write transaction"
	// drill performs a database write at the moment a cover is being fetched.
	during func(coverRequest)
}

func (f *fakeCovers) Poster(
	_ context.Context, _ int64, remoteKind, remoteID string,
) (imagepipeline.Result, error) {
	r := coverRequest{RemoteKind: remoteKind, RemoteID: remoteID}
	f.mu.Lock()
	f.calls = append(f.calls, r)
	err := f.answer[remoteID]
	f.mu.Unlock()

	if f.entered != nil {
		f.entered <- r
	}
	if f.during != nil {
		f.during(r)
	}
	if f.release != nil {
		<-f.release
	}
	if err != nil {
		return imagepipeline.Result{}, err
	}
	return imagepipeline.Result{
		AssetID: 1, CacheKey: "0123456789abcdef", Format: store.ImageFormatJPEG,
		SourceWidth: 500, SourceHeight: 750, Renditions: 7, CachedBytes: f.bytes,
	}, nil
}

func (f *fakeCovers) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeCovers) called(remoteID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.RemoteID == remoteID {
			return true
		}
	}
	return false
}

// coverFixture is one import's worth of DISTINCT items, imported once so their
// service_item_link rows exist, with the cover pass wired but not yet run.
//
// ⚠️ EVERY ITEM IS DISTINCT AND NONE HAS A POSTER. Repeating one work would make
// every bound drill below vacuous: the pass skips an item whose work already
// carries a poster, so N+1 calls against one work is N+1 skips and zero fetches,
// and a limiter would read as respected while nothing had been asked of it.
//
// ⚠️ THE REMOTE IDS ARE "1".."N", POSITIVE AND NUMERIC, and that is a deliberate
// departure from genItems (which starts at 0). It costs nothing here — the fake
// never parses one — and it keeps the fixture VALID FOR THE REAL PIPELINE, whose
// only pre-fetch early return is parseBookID refusing a non-positive or
// non-numeric remote id. A fixture carrying "0" or "not-a-book" would make every
// bound drill below pass for the parser's reason rather than the gate's, the
// moment anyone swapped the fake for the real thing.
func coverFixture(t *testing.T, n int) (*store.Store, int64, *fakeSource) {
	t.Helper()
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	items := make([]store.CatalogueItem, 0, n)
	for i := 1; i <= n; i++ {
		title := fmt.Sprintf("Title %04d", i)
		items = append(items, store.CatalogueItem{
			RemoteID: fmt.Sprint(i), RemoteKind: "series", ContainerID: "1", Kind: "comic",
			Title: title, SortTitle: title, NormalizedTitle: NormalizeTitle(title),
			NormVersion: NormVersion, AddedAt: testNow, RemoteUpdatedAt: testNow, HasFile: true,
		})
	}
	src := &fakeSource{
		containers: []store.CatalogueContainer{{RemoteID: "1", Name: "Manga", Kind: "comic"}},
		items:      items,
	}
	return s, inst, src
}

func coverImporter(t *testing.T, s *store.Store, src Source, covers PosterFetcher) *Importer {
	t.Helper()
	im := newImporter(t, s, src)
	im.Covers = covers
	return im
}

// shortenCoverWait makes the gate's refusal cost 20 ms instead of coverMaxWait.
// It is the only reason coverMaxWait is a var, exactly as kdfMaxWait is.
func shortenCoverWait(t *testing.T) {
	t.Helper()
	restore := coverMaxWait
	coverMaxWait = 20 * time.Millisecond
	t.Cleanup(func() { coverMaxWait = restore })
}

// fillCoverGate takes every permit and returns them on cleanup.
func fillCoverGate(t *testing.T) []func() {
	t.Helper()
	releases := make([]func(), 0, CoverPermits())
	for i := range CoverPermits() {
		release, err := AcquireCover(t.Context())
		if err != nil {
			t.Fatalf("permit %d of %d: %v", i, CoverPermits(), err)
		}
		// ONCE-GUARDED: a test may return its permits early, and the cleanup
		// still runs. A second release would take a token nobody put back and
		// park the cleanup goroutine for ever.
		var once sync.Once
		releases = append(releases, func() { once.Do(release) })
	}
	t.Cleanup(func() {
		for _, release := range releases {
			release()
		}
	})
	return releases
}

func TestCoverPermitsIsTheDeclaredBound(t *testing.T) {
	want := runtime.NumCPU()
	if want > maxCoverConcurrency {
		want = maxCoverConcurrency
	}
	if got := CoverPermits(); got != want {
		t.Fatalf("CoverPermits() = %d, want min(NumCPU=%d, %d) = %d",
			got, runtime.NumCPU(), maxCoverConcurrency, want)
	}
}

// TestAFullGateRefusesEveryCoverFetchAndTheImportStillSucceeds is THE DRILL for
// the bound, and it is two-sided on purpose.
//
// ⚠️ ONE SIDE ALONE WOULD BE A GREEN PROTECTING NOTHING. "The gate was full and
// nothing was fetched" is also what a pass that was never wired up produces, so
// the SAME fixture is run again with the permits returned and the fetcher must
// then be reached exactly N+1 times, on N+1 DISTINCT items with no poster
// between them. The first half proves the gate refuses; the second proves there
// was something real to refuse.
//
// The gate is filled through AcquireCover rather than by racing goroutines,
// because a limiter drilled against a stopwatch becomes a green the moment the
// machine gets faster. internal/crypto's TestEveryKDFCallIsBounded is the
// precedent and this is the same shape.
func TestAFullGateRefusesEveryCoverFetchAndTheImportStillSucceeds(t *testing.T) {
	shortenCoverWait(t)
	n := CoverPermits() + 1

	// ── Side one: every permit held, so nothing may be fetched. ──────────────
	s, inst, src := coverFixture(t, n)
	covers := &fakeCovers{}
	releases := fillCoverGate(t)

	rep, err := coverImporter(t, s, src, covers).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport with a full cover gate = %v; a refused permit must never fail an import", err)
	}
	if got := covers.callCount(); got != 0 {
		t.Errorf("the fetcher was called %d times with every permit held, want 0: "+
			"the pass is not taking its permits from coverGate, so nothing bounds it", got)
	}
	if rep.Covers.Deferred != n {
		t.Errorf("Covers.Deferred = %d, want %d — every item must be accounted for as "+
			"NOT ATTEMPTED rather than vanishing", rep.Covers.Deferred, n)
	}
	if rep.Covers.Fetched != 0 || rep.Covers.Failed != 0 {
		t.Errorf("Covers = %+v, want no fetches and no failures: a refused permit is neither", rep.Covers)
	}
	if !rep.Completed {
		t.Error("Completed = false; artwork must not decide whether a catalogue is complete")
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM work`); got != n {
		t.Errorf("work rows = %d, want %d: the catalogue is the part that must not be affected", got, n)
	}

	// The refusal is DURABLE, not only in the returned Report.
	var detail string
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT detail FROM sync_report WHERE kind = ?`, syncReportCoverPassIncomplete,
	).Scan(&detail); err != nil {
		t.Fatalf("no %q row was written for a pass that got no artwork at all: %v",
			syncReportCoverPassIncomplete, err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(detail), &got); err != nil {
		t.Fatalf("sync_report detail is not JSON: %v", err)
	}
	if got["deferred"] != float64(n) {
		t.Errorf("sync_report detail deferred = %v, want %d", got["deferred"], n)
	}

	// ── Side two: the permits come back and the very same fixture fetches. ───
	for _, release := range releases {
		release()
	}
	s2, inst2, src2 := coverFixture(t, n)
	covers2 := &fakeCovers{bytes: 11}
	rep2, err := coverImporter(t, s2, src2, covers2).FullImport(t.Context(), inst2)
	if err != nil {
		t.Fatalf("FullImport with the gate free: %v", err)
	}
	if got := covers2.callCount(); got != n {
		t.Fatalf("the fetcher was called %d times with the gate free, want %d — "+
			"without this the refusal above proves nothing, because a pass that "+
			"never calls the fetcher at all produces the same zero", got, n)
	}
	if rep2.Covers.Fetched != n || rep2.Covers.Deferred != 0 {
		t.Errorf("Covers = %+v, want %d fetched and nothing deferred", rep2.Covers, n)
	}
	if rep2.Covers.Bytes != n*11 {
		t.Errorf("Covers.Bytes = %d, want %d", rep2.Covers.Bytes, n*11)
	}
}

// TestAnInFlightCoverIsHOLDINGAPermit closes the loop the two-sided drill leaves
// open: it proves the permit is held FOR THE DURATION OF THE FETCH, not taken
// and dropped around it.
//
// ⚠️ IT IS DETERMINISTIC AND NOT A STOPWATCH. The test blocks until the fake
// reports a call has BEGUN, and only then asks the gate for the one remaining
// permit. If the in-flight fetch holds it, that ask must be refused; if the pass
// released it before calling, the ask succeeds and this fails. No interval is
// waited on and no assumption is made about how long a fetch takes.
func TestAnInFlightCoverIsHOLDINGAPermit(t *testing.T) {
	shortenCoverWait(t)
	if CoverPermits() < 1 {
		t.Fatalf("CoverPermits() = %d", CoverPermits())
	}

	// Hold all but one permit, so exactly one fetch may be in flight.
	held := make([]func(), 0, CoverPermits()-1)
	for i := 0; i < CoverPermits()-1; i++ {
		release, err := AcquireCover(t.Context())
		if err != nil {
			t.Fatalf("permit %d: %v", i, err)
		}
		held = append(held, release)
	}
	defer func() {
		for _, release := range held {
			release()
		}
	}()

	// ONE item, so the pass runs ONE worker and the only permit in play is the
	// one that fetch is holding. More items would be refused by the gate the
	// moment they asked — the pass halts on the first refusal, by design — and
	// this drill would then be measuring that rule instead of this one.
	s, inst, src := coverFixture(t, 1)
	covers := &fakeCovers{
		entered: make(chan coverRequest, 1),
		release: make(chan struct{}),
	}

	done := make(chan error, 1)
	go func() {
		_, err := coverImporter(t, s, src, covers).FullImport(context.Background(), inst)
		done <- err
	}()

	<-covers.entered // a fetch has begun and has not returned

	release, err := acquireCover(context.Background())
	if !errors.Is(err, ErrCoverFetchBusy) {
		if release != nil {
			release()
		}
		t.Fatalf("acquireCover while a cover fetch was in flight = %v, want ErrCoverFetchBusy — "+
			"the pass is not holding a permit across the fetch, so the gate caps nothing", err)
	}
	// THE REFUSAL, QUOTED. It must name this process rather than the upstream: a
	// saturated gate is not a fact about BookOrbit.
	if want := "libsync: no cover-fetch permit came free"; err.Error() != want {
		t.Errorf("refusal text = %q, want %q", err.Error(), want)
	}

	close(covers.release)
	if err := <-done; err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if got := covers.callCount(); got != 1 {
		t.Errorf("the fetcher was called %d times, want 1", got)
	}
	// AND THE PERMIT CAME BACK. A gate that acquires and never releases caps the
	// first import and refuses every one after it, for the life of the process —
	// which passes every assertion above.
	back, err := acquireCover(context.Background())
	if err != nil {
		t.Fatalf("acquireCover after the pass finished = %v; the permit was leaked", err)
	}
	back()
}

// TestACancelledCallerIsNotReportedAsSaturation keeps the two apart, on
// crypto/password.go's line: the server is fine, the caller left, and filing
// both under one error makes a disconnect look like an overload.
func TestACancelledCallerIsNotReportedAsSaturation(t *testing.T) {
	shortenCoverWait(t)
	fillCoverGate(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release, err := acquireCover(ctx)
	if release != nil {
		release()
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("acquireCover with a cancelled caller = %v, want context.Canceled", err)
	}
}

// TestTheCoverWaitIsBounded proves the refusal RETURNS rather than parking a
// goroutine for ever. Converting one problem into an unbounded-goroutine problem
// is not a fix.
func TestTheCoverWaitIsBounded(t *testing.T) {
	shortenCoverWait(t)
	fillCoverGate(t)

	start := time.Now()
	release, err := acquireCover(context.Background())
	if release != nil {
		release()
	}
	if !errors.Is(err, ErrCoverFetchBusy) {
		t.Fatalf("acquireCover on a full gate = %v, want ErrCoverFetchBusy", err)
	}
	// A ceiling three orders of magnitude above the shortened wait: this asserts
	// that a bound EXISTS, and is not a calibration of how fast the box is.
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("a refused acquire took %v; the wait is not bounded", elapsed)
	}
}

// TestACoverFetchFailureNeverFailsTheImport is the behaviour the whole pass is
// shaped around: a partial catalogue that says so beats no catalogue, and a
// complete catalogue with partial artwork beats both.
func TestACoverFetchFailureNeverFailsTheImport(t *testing.T) {
	s, inst, src := coverFixture(t, 4)
	boom := errors.New("bookorbit fell over")
	covers := &fakeCovers{answer: map[string]error{"1": boom, "2": boom, "3": boom, "4": boom}}

	rep, err := coverImporter(t, s, src, covers).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport = %v; every cover failing must not fail an import", err)
	}
	if !rep.Completed {
		t.Error("Completed = false: last_full_sync_at must still be stamped")
	}
	if rep.Covers.Failed != 4 {
		t.Errorf("Covers.Failed = %d, want 4", rep.Covers.Failed)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM work`); got != 4 {
		t.Errorf("work rows = %d, want 4", got)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM sync_report WHERE kind = 'cover_pass_incomplete'`); got != 1 {
		t.Errorf("cover_pass_incomplete rows = %d, want exactly 1 — one row per PASS, "+
			"never one per item, or a library the credential half-sees writes tens of "+
			"thousands of them", got)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM service_instance WHERE last_full_sync_at IS NOT NULL`); got != 1 {
		t.Errorf("stamped instances = %d, want 1", got)
	}
}

// TestA404IsAbsentForThisCredentialAndIsNeverCachedAsAVerdict is the 404 rule.
//
// ⚠️ BookOrbit throws the SAME NotFoundException for a book with no cover file,
// a book that does not exist, and a book the credential's content filters HIDE.
// So a 404 is not evidence about a file, and anything durable written from one
// permanently mislabels every book the service account cannot see. The
// assertion is therefore about what is NOT in the database, and about the next
// import asking again.
func TestA404IsAbsentForThisCredentialAndIsNeverCachedAsAVerdict(t *testing.T) {
	s, inst, src := coverFixture(t, 3)
	notFound := fmt.Errorf("imagepipeline: fetch cover: %w", imagepipeline.ErrCoverNotFound)
	covers := &fakeCovers{answer: map[string]error{"1": notFound, "2": notFound, "3": notFound}}
	im := coverImporter(t, s, src, covers)

	rep, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if rep.Covers.Absent != 3 || rep.Covers.Failed != 0 {
		t.Errorf("Covers = %+v, want 3 absent and 0 failed: a 404 is an ANSWER, not a failure", rep.Covers)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM image_asset`); got != 0 {
		t.Errorf("image_asset rows = %d, want 0: nothing may be recorded from a 404", got)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM work WHERE poster_asset_id IS NOT NULL`); got != 0 {
		t.Errorf("works with a poster = %d, want 0", got)
	}
	// ⚠️ AND NO sync_report ROW. A library the credential only partly sees
	// answers 404 for the rest, on EVERY import, for ever; a row each time is a
	// permanent red mark on a service behaving exactly as configured.
	if got := countRows(t, s, `SELECT COUNT(*) FROM sync_report WHERE kind = 'cover_pass_incomplete'`); got != 0 {
		t.Errorf("cover_pass_incomplete rows = %d, want 0 — a 404 alone is not a shortfall", got)
	}

	// THE RETRY, which is the half a counter cannot show. The same items come
	// back on the next import because nothing was written to say they had none.
	covers.mu.Lock()
	covers.calls = nil
	covers.answer = nil
	covers.mu.Unlock()

	rep2, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("second FullImport: %v", err)
	}
	if got := covers.callCount(); got != 3 {
		t.Fatalf("the fetcher was called %d times on the re-import, want 3: a 404 was "+
			"cached as a permanent verdict and every book the credential cannot see "+
			"is now blank for ever", got)
	}
	if rep2.Covers.Fetched != 3 {
		t.Errorf("Covers.Fetched = %d on the re-import, want 3", rep2.Covers.Fetched)
	}
}

// TestAWorkThatAlreadyHasAPosterIsNotReFetched is idempotence, and it is the
// pass's own job rather than the pipeline's.
//
// ⚠️ MEASURED, NOT ASSUMED: imagepipeline.Pipeline.Poster has NO short circuit.
// Its body is parseBookID → build source URL → source.Cover, with no store read
// before the fetch; the only idempotence it owns is the store upsert AFTER the
// bytes are already on the wire (`ON CONFLICT(source_url) DO UPDATE`), which
// saves a row rewrite and not a round trip. So a re-import that did not skip
// here would re-fetch every cover in the library, every time.
func TestAWorkThatAlreadyHasAPosterIsNotReFetched(t *testing.T) {
	s, inst, src := coverFixture(t, 3)
	// The fake writes the row the real pipeline would, so the second import sees
	// a genuinely postered work rather than a hand-placed marker.
	covers := &fakeCovers{}
	covers.during = func(r coverRequest) {
		url := "http://bookorbit.test/api/v1/books/" + r.RemoteID + "/cover"
		key, err := imagecache.Key(url)
		if err != nil {
			t.Errorf("imagecache.Key: %v", err)
			return
		}
		if _, err := s.PutPosterAsset(context.Background(), store.PosterAsset{
			RemoteKind: r.RemoteKind, RemoteID: r.RemoteID,
			SourceURL: url, CacheKey: key, Format: store.ImageFormatJPEG,
			OriginClass: store.ImageOriginConfigured, ServiceInstanceID: inst,
			Width: 500, Height: 750, FetchedAt: testNow,
		}); err != nil {
			t.Errorf("PutPosterAsset: %v", err)
		}
	}
	im := coverImporter(t, s, src, covers)

	if _, err := im.FullImport(t.Context(), inst); err != nil {
		t.Fatalf("first FullImport: %v", err)
	}
	if got := covers.callCount(); got != 3 {
		t.Fatalf("first import called the fetcher %d times, want 3", got)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM work WHERE poster_asset_id IS NOT NULL`); got != 3 {
		t.Fatalf("works with a poster after the first import = %d, want 3", got)
	}

	covers.mu.Lock()
	covers.calls = nil
	covers.mu.Unlock()

	rep, err := im.FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("second FullImport: %v", err)
	}
	if got := covers.callCount(); got != 0 {
		t.Errorf("the re-import called the fetcher %d times, want 0: every cover in the "+
			"library is being re-fetched on every import", got)
	}
	if rep.Covers.AlreadyHeld != 3 || rep.Covers.Fetched != 0 {
		t.Errorf("Covers = %+v, want 3 already held and 0 fetched", rep.Covers)
	}
}

// TestTheCoverPassHoldsNoWriteTransaction is the LOAD-BEARING structural drill.
//
// internal/db/sqlite.go's Write says it in these words: "fn must not call Write,
// and must not hold the transaction across a network call: the whole process
// shares one writer connection." A fetch issued from inside ApplyCatalogueBatch
// would hold the process's ONLY writer across a network round trip per book.
//
// ⚠️ IT IS DRILLED BY DOING A REAL WRITE FROM INSIDE A FETCH, on a short
// deadline. If the pass ever moves inside an open transaction, that write cannot
// take BEGIN IMMEDIATE — the single writer connection is already taken by the
// caller — and this goes red on a deadline rather than passing quietly.
func TestTheCoverPassHoldsNoWriteTransaction(t *testing.T) {
	s, inst, src := coverFixture(t, 2)
	var writeErr error
	var once sync.Once
	covers := &fakeCovers{}
	covers.during = func(r coverRequest) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := s.RecordSyncReport(ctx, inst, "drill", r.RemoteKind, r.RemoteID, `{"drill":true}`)
		if err != nil {
			once.Do(func() { writeErr = err })
		}
	}

	if _, err := coverImporter(t, s, src, covers).FullImport(t.Context(), inst); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if writeErr != nil {
		t.Fatalf("a write issued DURING a cover fetch failed: %v — the cover pass is "+
			"holding the single writer connection across a network call, which is "+
			"exactly what internal/db/sqlite.go's Write forbids", writeErr)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM sync_report WHERE kind = 'drill'`); got != 2 {
		t.Errorf("drill rows = %d, want 2", got)
	}
}

// TestEveryImportedItemLandsInExactlyOneCoverCounter pins the invariant
// CoverResult's header claims. An item that is in no counter is one that
// disappeared silently, and "how many of my books got artwork" then has no
// answer that can be checked against "how many books were there".
func TestEveryImportedItemLandsInExactlyOneCoverCounter(t *testing.T) {
	s, inst, src := coverFixture(t, 4)
	covers := &fakeCovers{answer: map[string]error{
		"2": fmt.Errorf("wrapped: %w", imagepipeline.ErrCoverNotFound),
		"3": fmt.Errorf("wrapped: %w", imagepipeline.ErrCoverForbidden),
		"4": errors.New("a transient upstream"),
	}}

	rep, err := coverImporter(t, s, src, covers).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	got := rep.Covers
	if got.Fetched != 1 || got.Absent != 1 || got.Forbidden != 1 || got.Failed != 1 {
		t.Errorf("Covers = %+v, want one of each of fetched/absent/forbidden/failed", got)
	}
	if got.Total() != 4 {
		t.Errorf("Covers.Total() = %d, want 4 (the items the pass was handed)", got.Total())
	}
	// A 403 is NOT folded in with the 404: one is a scope to widen, the other is
	// a cover to ask for again, and they take different fixes.
	if !covers.called("3") {
		t.Error("the forbidden item was never asked for")
	}
}

// TestNoPosterFetcherIsNotAFailure is principle 3 applied to a pass. Every
// adapter with no cover route wired — Kavita today, the *Arrs later — imports
// exactly as before.
func TestNoPosterFetcherIsNotAFailure(t *testing.T) {
	s, inst, src := coverFixture(t, 3)
	rep, err := newImporter(t, s, src).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport with no Covers: %v", err)
	}
	if rep.Covers != (CoverResult{}) {
		t.Errorf("Covers = %+v, want the zero value", rep.Covers)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM sync_report`); got != 0 {
		t.Errorf("sync_report rows = %d, want 0: a pass that did not run has nothing to report", got)
	}
}

// TestTheCoverPassPublishesItsOwnPhase pins the SSE frame. web/src/lib/api.test.ts
// reads the phase set off this package and CLIENT_PHASES has to match it; this
// is the server-side half, asserting the frame is actually emitted rather than
// only declared.
func TestTheCoverPassPublishesItsOwnPhase(t *testing.T) {
	s, inst, src := coverFixture(t, 2)
	var frames []Progress
	im := coverImporter(t, s, src, &fakeCovers{})
	im.Progress = func(p Progress) { frames = append(frames, p) }

	if _, err := im.FullImport(t.Context(), inst); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	var covers []Progress
	for _, f := range frames {
		if f.Phase == "covers" {
			covers = append(covers, f)
		}
	}
	if len(covers) == 0 {
		t.Fatalf("no `covers` frame was published; phases seen: %v", phaseNames(frames))
	}
	last := covers[len(covers)-1]
	if last.ItemsRead != 2 || last.Applied != 2 || last.Total != 2 {
		t.Errorf("last covers frame = %+v, want items_read 2, applied 2, total 2", last)
	}
	// The terminal frame is still `done`, and it still comes last: covers run
	// before it, so a client waiting on `done` is not told the run finished
	// while artwork is still being fetched.
	if frames[len(frames)-1].Phase != "done" {
		t.Errorf("last frame is %q, want done", frames[len(frames)-1].Phase)
	}
}

func phaseNames(frames []Progress) []string {
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		out = append(out, f.Phase)
	}
	return out
}
