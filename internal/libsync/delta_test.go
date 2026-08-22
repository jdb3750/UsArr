package libsync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// Channel 3b's tests, at the layer where the ARTEFACTS live.
//
// The walk's own arithmetic — page 0 on every request, the tie mitigation, the
// wedge, the short-page stop — is pinned in internal/bookorbit against that
// package's HTTP seam, where the request BODY can be asserted. What is asserted
// here is everything a person can afterwards SEE: the stored cursor position,
// the sync_report row, whether the failure reached the caller, and whether the
// Libraries screen's verdicts survived.

func bookAt(id int64, title string, at time.Time) bookorbit.Book {
	b := proseBook(id, title)
	b.AddedAt = at
	return b
}

func at(mins int) time.Time {
	return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(mins) * time.Minute)
}

// deltaFixture is one instance, one library, and the books in it.
type deltaFixture struct {
	st       *store.Store
	instance int64
	reader   *fakeBookOrbitReader
	src      *BookOrbitSource
}

func newDeltaFixture(t *testing.T, libs []bookorbit.Library, books map[int64][]bookorbit.Book) *deltaFixture {
	t.Helper()
	r := &fakeBookOrbitReader{libs: libs, books: books}
	f := &deltaFixture{
		st:     newTestStore(t),
		reader: r,
		src:    NewBookOrbitSource(r),
	}
	f.instance = fixtureBookOrbitInstance(t, f.st)
	return f
}

func (f *deltaFixture) importer() *Importer {
	return &Importer{Store: f.st, Source: f.src, UserID: store.SystemUserID}
}

// reopen gives the fixture a FRESH adapter over the same reader and database,
// which is what a second run in a live process gets: a new BookOrbitSource is
// built per run, so no fold, tally or verdict survives from the last one.
func (f *deltaFixture) reopen() {
	f.src = NewBookOrbitSource(f.reader)
}

func (f *deltaFixture) watermark(t *testing.T) store.ArrivalsWatermark {
	t.Helper()
	w, err := f.st.ReadArrivalsWatermark(t.Context(), f.instance)
	if err != nil {
		t.Fatalf("ReadArrivalsWatermark: %v", err)
	}
	return w
}

// deltaWalkRows reads back every delta_walk report this instance produced.
func (f *deltaFixture) deltaWalkRows(t *testing.T) []map[string]any {
	t.Helper()
	rows, err := f.st.DB().Read().QueryContext(t.Context(),
		`SELECT detail FROM sync_report WHERE service_instance_id = ? AND kind = ? ORDER BY id`,
		f.instance, SyncReportDeltaWalk)
	if err != nil {
		t.Fatalf("read delta_walk rows: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []map[string]any
	for rows.Next() {
		var detail string
		if err := rows.Scan(&detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var d map[string]any
		if err := json.Unmarshal([]byte(detail), &d); err != nil {
			t.Fatalf("decode delta_walk detail %q: %v", detail, err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// syncReportKindCount counts one kind's rows for this instance.
func (f *deltaFixture) syncReportKindCount(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := f.st.DB().Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		f.instance, kind).Scan(&n); err != nil {
		t.Fatalf("count %s rows: %v", kind, err)
	}
	return n
}

// ─── the bootstrap ───────────────────────────────────────────────────────────

// ⚠️ WITHOUT THIS THE CHANNEL NEVER RUNS ONCE. A keyset's request 1 REQUIRES a
// filter value, so an unbootstrapped instance leaves the implementer reaching
// for UsArr's own clock — which is the one move that can push a row across the
// boundary permanently.
func TestAFullImportMintsTheFirstArrivalsCursor(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 2}},
		map[int64][]bookorbit.Book{1: {
			bookAt(1, "Early", at(0)),
			bookAt(2, "Late", at(30)),
		}})

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}

	w := f.watermark(t)
	if !w.Valid {
		t.Fatal("the full import minted no arrivals cursor, so no delta can ever run")
	}
	if got, want := w.Value, bookorbit.FormatArrivalsInstant(at(30)); got != want {
		t.Errorf("watermark = %q, want %q — the MAXIMUM observed, in BookOrbit's clock, "+
			"and never the run's own start", got, want)
	}
}

// A stamp UsArr cannot read makes the maximum untrustworthy rather than wrong,
// so the mint is DECLINED. On the pinned upstream this cannot happen —
// books.added_at is NOT NULL and toISOString() is the only producer — which is
// exactly why it is fired here deliberately instead of trusted.
func TestAnUnreadableStampDeclinesTheMint(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 2}},
		map[int64][]bookorbit.Book{1: {
			bookAt(1, "Readable", at(0)),
			bookAt(2, "Unreadable", time.Time{}), // what parseISO returns for a value it cannot read
		}})

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if w := f.watermark(t); w.Valid {
		t.Errorf("watermark = %+v, want none: a maximum computed over a set with unreadable "+
			"members is not known to be a maximum, and defaulting it would be a cursor "+
			"position nothing observed", w)
	}
	// The rows themselves still landed: refusing the page over one bad stamp
	// would cost the catalogue, which is the trade parseISO already made.
	works, err := f.st.WorkCountsByInstance(t.Context(), store.OwnerScope(store.SystemUserID))
	if err != nil {
		t.Fatalf("WorkCountsByInstance: %v", err)
	}
	if works[f.instance] != 2 {
		t.Errorf("works = %d, want 2 — both books are applied; only the CURSOR is withheld",
			works[f.instance])
	}
}

// ─── the quiet run ───────────────────────────────────────────────────────────

// 🚩 THE MODAL OUTCOME OF AN ARRIVALS CHANNEL, and the rule whose absence was a
// data-loss bug: a completed run that observed nothing SUCCEEDS, so "write the
// watermark on success" would have said WRITE IT — and writing an invalid
// watermark NULLS THE COLUMN. A BookOrbit that has simply been quiet would
// destroy its own cursor.
func TestAQuietDeltaLeavesTheCursorByteForByteUnchanged(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "Only", at(0))}})

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	before := f.watermark(t)

	// Nothing new arrives, and the lookbehind window is empty too: the fixture's
	// one book sits an hour below the seed.
	f.reader.books[1] = nil
	f.reopen()
	rep, err := f.importer().DeltaSync(t.Context(), f.instance)
	if err != nil {
		t.Fatalf("DeltaSync: %v — a quiet run is not a failure", err)
	}

	after := f.watermark(t)
	if after != before {
		t.Fatalf("watermark moved from %+v to %+v on a run that observed nothing", before, after)
	}
	if rep.Advanced || rep.DeclineReason != declineNoRows {
		t.Errorf("Advanced = %v, reason = %q, want false and %q",
			rep.Advanced, rep.DeclineReason, declineNoRows)
	}
	rows := f.deltaWalkRows(t)
	if len(rows) != 1 || rows[0]["advance_declined_reason"] != declineNoRows {
		t.Errorf("delta_walk rows = %+v, want one recording %q", rows, declineNoRows)
	}
}

// ─── the ordinary run ────────────────────────────────────────────────────────

func TestADeltaSeedsFromTheWatermarkMinusTheLookbehindAndThenAdvances(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "First", at(0))}})

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}

	// One arrival lands above the watermark.
	f.reader.books[1] = append(f.reader.books[1], bookAt(2, "Second", at(60)))
	f.reopen()
	rep, err := f.importer().DeltaSync(t.Context(), f.instance)
	if err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}

	// THE SEED IS THE WATERMARK MINUS THE WINDOW, APPLIED ONCE. Applying it per
	// page instead is a non-terminating walk.
	if len(f.reader.cursorsSeen) != 1 {
		t.Fatalf("cursors sent = %d, want 1 (one library, one walk)", len(f.reader.cursorsSeen))
	}
	c := f.reader.cursorsSeen[0]
	want := bookorbit.FormatArrivalsInstant(at(0).Add(-arrivalsLookbehind))
	if !c.Set || c.Value != want {
		t.Errorf("cursor = %+v, want %q", c, want)
	}

	// The window re-delivered the first book; the upsert absorbed it, and the
	// cursor advanced to the new maximum.
	if !rep.Advanced {
		t.Fatal("the watermark did not advance on a completed run that observed a new maximum")
	}
	if got, want := f.watermark(t).Value, bookorbit.FormatArrivalsInstant(at(60)); got != want {
		t.Errorf("watermark = %q, want %q", got, want)
	}
	rows := f.deltaWalkRows(t)
	if len(rows) != 1 {
		t.Fatalf("delta_walk rows = %d, want 1", len(rows))
	}
	if rows[0]["advanced"] != true || rows[0]["advance_declined_reason"] != "" {
		t.Errorf("row = %+v, want advanced with no decline reason", rows[0])
	}
	// The lookbehind's own measurement, collected while the lookbehind exists.
	if got, want := rows[0]["min_delivered_added_at"], bookorbit.FormatArrivalsInstant(at(0)); got != want {
		t.Errorf("min_delivered_added_at = %v, want %v — it is half of what would MEASURE the "+
			"five-minute window instead of assuming it", got, want)
	}
}

// A row the adapter declines to MAP is still an arrival. A cursor that advanced
// only past the rows this adapter understood would re-read every unmappable book
// on every poll, forever.
func TestAnUnmappableArrivalStillMovesTheCursor(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "First", at(0))}})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}

	// A media kind BookOrbit itself classifies as unknown: read, counted, and
	// never mapped.
	f.reader.books[1] = append(f.reader.books[1], withFormat(bookAt(2, "Odd", at(60)), "xyz"))
	f.reopen()
	if _, err := f.importer().DeltaSync(t.Context(), f.instance); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if got, want := f.watermark(t).Value, bookorbit.FormatArrivalsInstant(at(60)); got != want {
		t.Errorf("watermark = %q, want %q", got, want)
	}
}

// ─── THE WEDGE DRILL, libsync half ───────────────────────────────────────────

// The bookorbit half fires the wedge against the real HTTP seam and asserts the
// walk stops. This half asserts the ARTEFACTS — the things a person can see —
// because asserting only that a function returned an error would test the
// intention rather than the noise.
func TestAWedgeLeavesTheCursorUnchangedAndSaysSoWhereItCanBeSeen(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "First", at(0))}})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	before := f.watermark(t)

	const midnight = "2026-06-01T00:00:00.000Z"
	f.reader.wedgeAt = map[int64]string{1: midnight}
	f.reader.books[1] = append(f.reader.books[1], bookAt(2, "Backdated", at(60)))
	f.reopen()

	rep, err := f.importer().DeltaSync(t.Context(), f.instance)

	// 1. THE ERROR REACHES THE CALLER. A wedge is an outage above that instant
	//    until it clears, and a person is standing there.
	if err == nil {
		t.Fatal("the run succeeded; a wedge must fail to its caller rather than return a note")
	}
	// 2. THE STORED CURSOR IS BYTE-FOR-BYTE UNCHANGED.
	if after := f.watermark(t); after != before {
		t.Fatalf("watermark moved from %+v to %+v on a wedged run — that IS the 70-row loss, "+
			"reintroduced deliberately", before, after)
	}
	// 3. A delta_walk ROW EXISTS, NAMES THE REASON, AND CARRIES THE INSTANT.
	rows := f.deltaWalkRows(t)
	if len(rows) != 1 {
		t.Fatalf("delta_walk rows = %d, want 1 — a wedge nobody can find afterwards is a quiet "+
			"stall, which is the failure mode this whole design exists to avoid", len(rows))
	}
	if rows[0]["advance_declined_reason"] != declineTieWedge {
		t.Errorf("advance_declined_reason = %v, want %q", rows[0]["advance_declined_reason"], declineTieWedge)
	}
	if rows[0]["wedge_at"] != midnight {
		t.Errorf("wedge_at = %v, want %q — the instant is how a person finds the rows",
			rows[0]["wedge_at"], midnight)
	}
	if rows[0]["error_class"] != "walk_failed" {
		t.Errorf("error_class = %v, want a UsArr-authored class", rows[0]["error_class"])
	}
	// 4. THE RECORD CARRIES NO UPSTREAM PROSE. sync_report.detail reaches a
	//    browser, and reference/security.md keeps it free of response bodies.
	blob, _ := json.Marshal(rows[0])
	if strings.Contains(string(blob), "bookorbit: library") {
		t.Errorf("the report carries the upstream's own error text: %s", blob)
	}
	if rep.Advanced {
		t.Error("the report claims the watermark advanced")
	}
}

// ─── declines to advance ─────────────────────────────────────────────────────

func TestAnUnreadableStampDeclinesTheAdvance(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "First", at(0))}})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	before := f.watermark(t)

	f.reader.books[1] = append(f.reader.books[1],
		bookAt(2, "Later", at(60)), bookAt(3, "Unreadable", time.Time{}))
	f.reopen()
	rep, err := f.importer().DeltaSync(t.Context(), f.instance)
	if err != nil {
		t.Fatalf("DeltaSync: %v — the rows still apply; only the cursor is withheld", err)
	}
	if after := f.watermark(t); after != before {
		t.Errorf("watermark moved to %+v despite an unreadable stamp", after)
	}
	if rep.DeclineReason != declineUnreadableStamp {
		t.Errorf("reason = %q, want %q", rep.DeclineReason, declineUnreadableStamp)
	}
}

// The all-or-nothing rule: one library short means the instance's cursor does
// not move at all, and the next run re-reads every library's window.
func TestAFailedLibraryLeavesTheCursorUnmovedAndTheNextRunReReadsTheWindow(t *testing.T) {
	boom := errors.New("bookorbit refused library 2")
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}, {ID: 2, Name: "Audio", BookCount: 1}},
		map[int64][]bookorbit.Book{
			1: {bookAt(1, "One", at(0))},
			2: {bookAt(2, "Two", at(1))},
		})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	before := f.watermark(t)

	f.reader.books[1] = append(f.reader.books[1], bookAt(3, "New in one", at(60)))
	f.reader.walkErr = map[int64]error{2: boom}
	f.reopen()

	if _, err := f.importer().DeltaSync(t.Context(), f.instance); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the library failure", err)
	}
	if after := f.watermark(t); after != before {
		t.Fatalf("watermark moved from %+v to %+v although library 2 never completed — the next "+
			"run would then never re-read library 2's window", before, after)
	}
	// RESUMPTION: the next run asks from the SAME unmoved watermark, so the whole
	// window is re-read and the already-applied rows re-apply as no-ops.
	f.reader.walkErr = nil
	f.reopen()
	if _, err := f.importer().DeltaSync(t.Context(), f.instance); err != nil {
		t.Fatalf("second DeltaSync: %v", err)
	}
	prev, err := before.Time()
	if err != nil {
		t.Fatalf("the stored cursor does not parse: %v", err)
	}
	for _, c := range f.reader.cursorsSeen {
		want := bookorbit.FormatArrivalsInstant(prev.Add(-arrivalsLookbehind))
		if c.Value != want {
			t.Errorf("resumed cursor = %q, want %q — nothing records a resume point, and the "+
				"whole window is re-read from the unmoved watermark", c.Value, want)
		}
	}
	if got, want := f.watermark(t).Value, bookorbit.FormatArrivalsInstant(at(60)); got != want {
		t.Errorf("after the repaired run the watermark is %q, want %q", got, want)
	}
}

// ─── the preconditions ───────────────────────────────────────────────────────

func TestADeltaEscalatesOnAnInstanceNeverFullyImported(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "One", at(0))}})

	rep, err := f.importer().DeltaSync(t.Context(), f.instance)
	if !errors.Is(err, ErrEscalateToFullImport) {
		t.Fatalf("err = %v, want ErrEscalateToFullImport", err)
	}
	if !rep.Escalated || !strings.Contains(rep.EscalationReason, "never completed a full sync") {
		t.Errorf("report = %+v, want the reason named", rep)
	}
	// ANSWERED FROM SQLITE ALONE: finding out that there is nothing to be a delta
	// of must not cost a request.
	if len(f.reader.walkedSinceAt) != 0 || f.reader.auths != 0 {
		t.Errorf("the refusal cost %d walks and %d authentications, want none",
			len(f.reader.walkedSinceAt), f.reader.auths)
	}
}

// 🚩 THE PERMISSION-VERSUS-EMPTY GUARD. A full import that observed zero books
// mints no cursor, and "the catalogue held no books" is only ONE cause of that
// observation — this project's record already names another at the source, where
// LibraryAccessGuard throws an IDENTICAL ForbiddenException for "exists, no
// access" and for "no such library", so FORBIDDEN IS INDISTINGUISHABLE FROM
// ABSENT, and a content filter shorts a count without dropping the library.
//
// So the guard does not ask WHY the zero happened. It asks whether an
// independent, unfiltered measurement AGREES that there is nothing there — which
// is answered correctly for causes nobody has thought of, where a guard scoped to
// one cause is blind to every other cause of the same observation.
func TestADeltaEscalatesWhenAZeroBookObservationIsNotAMeasuredEmptyCatalogue(t *testing.T) {
	// The credential was shown nothing; BookOrbit's own unfiltered count says the
	// library holds 900 books. Nothing here is a permission test.
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 0}},
		map[int64][]bookorbit.Book{})
	f.reader.stats = map[int64]int64{1: 900}

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if w := f.watermark(t); w.Valid {
		t.Fatalf("the full import observed no books and still minted %+v", w)
	}

	f.reopen()
	rep, err := f.importer().DeltaSync(t.Context(), f.instance)
	if !errors.Is(err, ErrEscalateToFullImport) {
		t.Fatalf("err = %v, want ErrEscalateToFullImport — an unfiltered 'delta' over an instance "+
			"that is NOT known to be empty is a full read wearing a delta's report", err)
	}
	if !strings.Contains(rep.EscalationReason, "900") {
		t.Errorf("reason = %q, want the independent measurement quoted", rep.EscalationReason)
	}
	if len(f.reader.walkedSinceAt) != 0 {
		t.Errorf("the walk ran anyway")
	}
}

// The same guard refuses an UNVERIFIED measurement as loudly as a non-zero one:
// a measurement that did not happen is not agreement.
func TestADeltaEscalatesWhenTheEmptinessCouldNotBeMeasured(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 0}},
		map[int64][]bookorbit.Book{})
	f.reader.statsErr = map[int64]error{1: bookorbit.ErrForbidden}

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	f.reopen()
	rep, err := f.importer().DeltaSync(t.Context(), f.instance)
	if !errors.Is(err, ErrEscalateToFullImport) {
		t.Fatalf("err = %v, want ErrEscalateToFullImport", err)
	}
	if !strings.Contains(rep.EscalationReason, "could not be measured") {
		t.Errorf("reason = %q, want the unmeasured state named", rep.EscalationReason)
	}
}

// The narrow branch the guard exists to keep narrow: a catalogue MEASURED empty
// runs unfiltered, because escalating there costs the same read AND re-runs
// ANALYZE AND re-stamps a freshness claim that did not change.
func TestAMeasuredEmptyCatalogueRunsUnfiltered(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 0}},
		map[int64][]bookorbit.Book{})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}

	// Still empty at delta time, which is the state this branch was narrowed to.
	f.reopen()
	if _, err := f.importer().DeltaSync(t.Context(), f.instance); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if len(f.reader.cursorsSeen) != 1 || f.reader.cursorsSeen[0].Set {
		t.Errorf("cursors = %+v, want exactly one UNSET cursor: request 1 carries no filter key "+
			"at all, and never a filter on the zero time", f.reader.cursorsSeen)
	}
	if w := f.watermark(t); w.Valid {
		t.Errorf("watermark = %+v, want none: a run that observed nothing advances nothing", w)
	}
}

// ⚠️ AND THE SAME MEASUREMENT ESCALATES ONCE THE INSTANCE HAS FILLED UP, which
// is a decision rather than a side effect. A keyset walk of the whole set would
// be a correct walk, but a catalogue that just gained its entire contents is
// exactly where ANALYZE and a re-stamped freshness claim are earned.
func TestAnInstanceThatWasEmptyAndHasFilledUpEscalates(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 0}},
		map[int64][]bookorbit.Book{})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}

	f.reader.libs[0].BookCount = 2
	f.reader.books[1] = []bookorbit.Book{bookAt(1, "One", at(0)), bookAt(2, "Two", at(5))}
	f.reopen()
	if _, err := f.importer().DeltaSync(t.Context(), f.instance); !errors.Is(err, ErrEscalateToFullImport) {
		t.Fatalf("err = %v, want ErrEscalateToFullImport", err)
	}
	if len(f.reader.walkedSinceAt) != 0 {
		t.Errorf("the arrivals walk ran anyway")
	}
}

// 🚩 THE ESCALATION `CatalogueBinding.Created` MISSES. A container new to UsArr
// whose trimmed name matches an existing library of the same kind JOINS it and
// writes a brand-new binding row — Created is false there. An arrivals filter
// would return none of that container's back catalogue, and a whole library
// would be silently missing content.
func TestADeltaEscalatesWhenAContainerIsBoundIntoAnExistingLibrary(t *testing.T) {
	// Instance A's library "Fiction" exists because it was ACCEPTED — since
	// ADR-0048 an import creates none, and the join this test is about needs a
	// library to join.
	other := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 9, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{9: {bookAt(91, "A book", at(0))}})
	acceptContainers(t, other.st, other.instance,
		store.CatalogueContainer{RemoteID: "9", Name: "Fiction", Kind: "book"})
	if _, err := other.importer().FullImport(t.Context(), other.instance); err != nil {
		t.Fatalf("first instance FullImport: %v", err)
	}

	// Instance B, on the SAME database and the same user, starts with only
	// "Audio" and is fully imported.
	r := &fakeBookOrbitReader{
		libs:  []bookorbit.Library{{ID: 1, Name: "Audio", BookCount: 1}},
		books: map[int64][]bookorbit.Book{1: {bookAt(1, "Audio one", at(0))}},
	}
	b := &deltaFixture{st: other.st, reader: r, src: NewBookOrbitSource(r)}
	secondID, err := other.st.CreateServiceInstance(t.Context(), store.ServiceInstance{
		Kind: "bookorbit", Role: "library", Name: "bookorbit-two",
		BaseURL: "https://books.example:9999", APIKeyEnc: []byte{4, 5, 6}, KEKID: 1,
		Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	b.instance = secondID
	acceptContainers(t, b.st, b.instance,
		store.CatalogueContainer{RemoteID: "1", Name: "Audio", Kind: "book"})
	if _, err := b.importer().FullImport(t.Context(), b.instance); err != nil {
		t.Fatalf("second instance FullImport: %v", err)
	}

	// The credential is granted "Fiction" as well. It JOINS instance A's library.
	r.libs = append(r.libs, bookorbit.Library{ID: 2, Name: "Fiction", BookCount: 50})
	r.books[2] = []bookorbit.Book{bookAt(2, "Old backlog", at(-6000))}
	b.reopen()

	rep, err := b.importer().DeltaSync(t.Context(), b.instance)
	if !errors.Is(err, ErrEscalateToFullImport) {
		t.Fatalf("err = %v, want ErrEscalateToFullImport", err)
	}
	if !strings.Contains(rep.EscalationReason, "bound to this service for the first time") {
		t.Errorf("reason = %q", rep.EscalationReason)
	}
	// ⚠️ THE PROOF THAT `Created` WOULD HAVE MISSED IT: no library was created on
	// this run. Every binding joined, so a detector keyed on Created sees nothing.
	if rep.LibrariesCreated != 0 {
		t.Errorf("LibrariesCreated = %d, want 0 — if a library WAS created then this test is not "+
			"exercising the case Created misses, and the escalation is proving nothing new",
			rep.LibrariesCreated)
	}
	if rep.LibrariesJoined != 2 {
		t.Errorf("LibrariesJoined = %d, want 2", rep.LibrariesJoined)
	}
}

// ─── what a delta must not do ────────────────────────────────────────────────

// 🚩 A WINDOW-SCOPED VERDICT MUST NOT OVERWRITE A CONTAINER-SCOPED ONE. The skip
// vocabulary is a claim about a CONTAINER and the Libraries screen's read is
// latest-wins, so one quiet delta poll writing a zero-count row would leave that
// screen reading "nothing left out" while a full import's 300 unmapped comics
// are still not imported.
func TestADeltaWritesNoPerContainerSkipVerdict(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "One", at(0))}})
	// The library exists because it was accepted (ADR-0048): this test reads the
	// Libraries screen, and there is no library row without an Accept.
	acceptContainers(t, f.st, f.instance,
		store.CatalogueContainer{RemoteID: "1", Name: "Fiction", Kind: "book"})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}

	// The full import's verdict, as cmd/usarr would have written it.
	detail, err := json.Marshal(store.SkipNote{Name: "Fiction", Comics: 300})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.RecordSyncReport(t.Context(), f.instance,
		store.SyncReportItemsSkipped, "library", "1", string(detail)); err != nil {
		t.Fatalf("RecordSyncReport: %v", err)
	}

	f.reader.books[1] = append(f.reader.books[1], bookAt(2, "Two", at(60)))
	f.reopen()
	if _, err := f.importer().DeltaSync(t.Context(), f.instance); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}

	if skipRows := f.syncReportKindCount(t, store.SyncReportItemsSkipped); skipRows != 1 {
		t.Fatalf("items_skipped rows = %d, want the full import's 1 and nothing from the delta: "+
			"latest-wins means a second row IS the wrong answer on a shipped screen", skipRows)
	}
	// And the screen still reads the truth.
	libs, err := f.st.ListLibraries(t.Context(), store.OwnerScope(store.SystemUserID))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	var found bool
	for _, l := range libs {
		if l.Skips != nil && l.Skips.Items == 300 {
			found = true
		}
	}
	if !found {
		t.Errorf("the Libraries screen no longer reports the 300 left out: %+v", libs)
	}
}

// The SSE contract: a delta publishes nothing at all, so the Services screen
// never renders an import in progress for a channel that shows nothing new.
func TestADeltaPublishesNoProgress(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "One", at(0))}})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	f.reader.books[1] = append(f.reader.books[1], bookAt(2, "Two", at(60)))
	f.reopen()

	var frames []Progress
	im := f.importer()
	im.Progress = func(p Progress) { frames = append(frames, p) }
	if _, err := im.DeltaSync(t.Context(), f.instance); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	// ⚠️ The importer this test builds DOES carry a Progress hook, deliberately:
	// nil-ing it in cmd/usarr is what ships, and this asserts the pipeline still
	// publishes when one is supplied — so the silence in production is a caller's
	// decision that can be read at the call site rather than a behaviour buried
	// here. What must never happen is a NEW phase name the browser does not pin.
	for _, p := range frames {
		switch p.Phase {
		case "containers", "items", "credits", "files", "covers", "done":
		default:
			t.Errorf("the delta published phase %q, which the browser's CLIENT_PHASES does not pin",
				p.Phase)
		}
	}
}

// A source with no delta half is refused with a reason rather than told "202
// started" while nothing runs.
func TestASourceWithNoDeltaChannelIsRefusedWithAReason(t *testing.T) {
	st := newTestStore(t)
	id := fixtureInstance(t, st)
	im := &Importer{Store: st, Source: &staticSource{}, UserID: store.SystemUserID}

	_, err := im.DeltaSync(t.Context(), id)
	if !errors.Is(err, ErrNoDeltaChannel) {
		t.Fatalf("err = %v, want ErrNoDeltaChannel", err)
	}
	if !strings.Contains(err.Error(), "full sync") {
		t.Errorf("the refusal does not name what the source DOES offer: %v", err)
	}
}

// staticSource implements Source and nothing else — the shape every *Arr adapter
// will have, and Kavita already has.
type staticSource struct{}

func (staticSource) Containers(context.Context) ([]store.CatalogueContainer, error) { return nil, nil }

func (staticSource) StreamItems(context.Context, func(store.CatalogueItem) error) (int, error) {
	return 0, nil
}

// ─── the stored format ───────────────────────────────────────────────────────

// ⚠️ THE COLUMN IS DELIBERATELY NOT IN THIS SCHEMA'S USUAL LAYOUT, and this is
// the negative half that says so. store.FormatTime floors to the SECOND; against
// a strict `>` that makes every row in [W, W+1s) satisfy the filter on every poll
// forever, which is the permanent duplicate `>=` was refused in order to avoid.
func TestAStoredArrivalsCursorIsNotSomethingParseTimeAccepts(t *testing.T) {
	w := store.FormatArrivalsWatermark(time.Date(2026, 8, 1, 9, 12, 33, 987*int(time.Millisecond), time.UTC))
	if w.Value != "2026-08-01T09:12:33.987Z" {
		t.Fatalf("watermark = %q", w.Value)
	}
	if _, err := store.ParseTime(w.Value); err == nil {
		t.Error("store.ParseTime accepted the cursor format; the two layouts must stay " +
			"distinguishable, because a swallowed parse error is how a screen reads `never`")
	}
	if _, err := w.Time(); err != nil {
		t.Errorf("the cursor does not round-trip through its own layout: %v", err)
	}
	// The floor, measured rather than asserted.
	if got := store.FormatTime(time.Date(2026, 8, 1, 9, 12, 33, 999999000, time.UTC)); got != "2026-08-01 09:12:33" {
		t.Errorf("FormatTime = %q; the flooring this design routes around has changed", got)
	}
}

// An invalid watermark is REFUSED rather than written, because writing one nulls
// the column.
func TestStampingAnInvalidCursorIsRefused(t *testing.T) {
	st := newTestStore(t)
	id := fixtureInstance(t, st)
	if err := st.StampArrivalsWatermark(t.Context(), id, store.ArrivalsWatermark{}); !errors.Is(err, store.ErrNoWatermarkToStamp) {
		t.Fatalf("err = %v, want ErrNoWatermarkToStamp", err)
	}
}

// A MID-WALK ARRIVAL IS DELIVERED. It is the assertion the fake's own slice
// header used to make invisible: ranging over `f.books[libraryID]` evaluates the
// map index once, so a callback that appended to the fixture changed nothing the
// loop could see and this test passed against a walk that never noticed.
func TestAMidWalkArrivalIsDelivered(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}},
		map[int64][]bookorbit.Book{1: {bookAt(1, "One", at(0))}})
	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	f.reader.books[1] = append(f.reader.books[1], bookAt(2, "Two", at(60)))
	f.reopen()

	var appended bool
	f.reader.duringWalk = func(delivered int) {
		if delivered == 1 && !appended {
			appended = true
			f.reader.books[1] = append(f.reader.books[1], bookAt(3, "Arrived mid-walk", at(90)))
		}
	}
	if _, err := f.importer().DeltaSync(t.Context(), f.instance); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if got, want := f.watermark(t).Value, bookorbit.FormatArrivalsInstant(at(90)); got != want {
		t.Errorf("watermark = %q, want %q — the book that landed mid-walk was never seen", got, want)
	}
}
