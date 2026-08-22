package libsync

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// Channel 4's deletion pass as it is actually reached: from FullImport's success
// path, on the seen-set the item stream collected. internal/store's
// reconcile_test.go covers the sweep's own behaviour; this file covers the two
// facts only the importer can get wrong — WHICH links the read is judged to have
// seen, and WHICH channel is allowed to reconcile at all.

// issueItem is one child item and its parent, the shape ADR-0068 gives a
// BookOrbit comic: a `comic_issue` under a `comic` series, each with its OWN
// service_item_link.
func issueItem(remoteID, container, seriesID, title string) store.CatalogueItem {
	it := store.CatalogueItem{
		RemoteID: remoteID, RemoteKind: "book", ContainerID: container, Kind: "comic_issue",
		Title: title, SortTitle: title, NormalizedTitle: strings.ToLower(title), NormVersion: 1,
		AddedAt: testNow, RemoteUpdatedAt: testNow, HasFile: true,
		NumberText: sql.NullString{String: "1", Valid: true},
		Parent: &store.CatalogueParent{
			RemoteKind: "series", RemoteID: seriesID, Kind: "comic",
			Title: "Series " + seriesID, SortTitle: "Series " + seriesID,
			NormalizedTitle: "series " + seriesID, NormVersion: 1,
		},
	}
	return it
}

func linkDeletedAt(t *testing.T, s *store.Store, inst int64, remoteKind, remoteID string) sql.NullString {
	t.Helper()
	var v sql.NullString
	err := s.DB().Read().QueryRowContext(t.Context(), `
		SELECT deleted_at FROM service_item_link
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?`,
		inst, remoteKind, remoteID).Scan(&v)
	if err != nil {
		t.Fatalf("read link %s/%s: %v", remoteKind, remoteID, err)
	}
	return v
}

// TestTheDeletionPassSpansChildrenAndSynthesisedParents is the trap this wiring
// exists to avoid, and it is not hypothetical.
//
// FullImport already carries a per-item list — []ImportedItem — that
// DELIBERATELY EXCLUDES CHILDREN, because the cover pass issues one HTTP request
// per entry and ARCHITECTURE §13 sizes the child side at ~90,000 rows. A
// deletion pass fed from that list would find every `comic_issue` link absent
// from the read and tombstone the entire child side of the catalogue on the
// first sweep of a healthy instance. The seen-set is collected separately for
// exactly this reason, and this test is the reason written down.
func TestTheDeletionPassSpansChildrenAndSynthesisedParents(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := &fakeSource{
		containers: []store.CatalogueContainer{{RemoteID: "1", Name: "Comics", Kind: "comic"}},
		items: []store.CatalogueItem{
			issueItem("i1", "1", "s1", "Berserk 1"),
			issueItem("i2", "1", "s1", "Berserk 2"),
		},
	}
	rep, err := newImporter(t, s, src).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if !rep.Completed {
		t.Fatal("the seeding import did not complete")
	}
	// The fixture must really have written links for both grains, or the
	// assertion below is about a table with nothing in it to get wrong.
	for _, k := range [][2]string{{"book", "i1"}, {"book", "i2"}, {"series", "s1"}} {
		if linkDeletedAt(t, s, inst, k[0], k[1]).Valid {
			t.Fatalf("link %s/%s was born tombstoned", k[0], k[1])
		}
	}
	if rep.Deletions != (store.SweepResult{LinksLive: 3}) {
		t.Errorf("the first import's sweep moved something: %+v", rep.Deletions)
	}

	// A SECOND, IDENTICAL import. Nothing changed upstream, so nothing may be
	// tombstoned — child, parent or otherwise.
	again, err := newImporter(t, s, src).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("second FullImport: %v", err)
	}
	for _, k := range [][2]string{{"book", "i1"}, {"book", "i2"}, {"series", "s1"}} {
		if d := linkDeletedAt(t, s, inst, k[0], k[1]).Valid; d {
			t.Errorf("link %s/%s was tombstoned by an import that read it", k[0], k[1])
		}
	}
	if again.Deletions.LinksTombstoned != 0 || again.Deletions.WorksTombstoned != 0 {
		t.Errorf("an unchanged re-import tombstoned something: %+v", again.Deletions)
	}
}

func TestAFullImportTombstonesTheItemThatStoppedBeingReported(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	containers := []store.CatalogueContainer{{RemoteID: "1", Name: "Comics", Kind: "comic"}}

	first := &fakeSource{containers: containers, items: genItems(3, "1", "comic")}
	if _, err := newImporter(t, s, first).FullImport(t.Context(), inst); err != nil {
		t.Fatalf("first FullImport: %v", err)
	}

	// Item "2" is gone upstream.
	second := &fakeSource{containers: containers, items: genItems(3, "1", "comic")[:2]}
	rep, err := newImporter(t, s, second).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("second FullImport: %v", err)
	}

	if !linkDeletedAt(t, s, inst, "series", "2").Valid {
		t.Error("the item the second read did not report is still live")
	}
	for _, id := range []string{"0", "1"} {
		if linkDeletedAt(t, s, inst, "series", id).Valid {
			t.Errorf("item %s was in the read and was tombstoned anyway", id)
		}
	}
	// The work is what browse and search filter on, so the tombstone has to
	// reach it or the pass is invisible.
	var workDeleted sql.NullString
	if err := s.DB().Read().QueryRowContext(t.Context(), `
		SELECT w.deleted_at FROM work w
		  JOIN service_item_link l ON l.work_id = w.id
		 WHERE l.service_instance_id = ? AND l.remote_kind = 'series' AND l.remote_id = '2'`,
		inst).Scan(&workDeleted); err != nil {
		t.Fatalf("read work: %v", err)
	}
	if !workDeleted.Valid {
		t.Error("the absent item's work was not tombstoned")
	}
	if rep.Deletions.LinksTombstoned != 1 || rep.Deletions.WorksTombstoned != 1 {
		t.Errorf("Deletions = %+v, want one link and one work", rep.Deletions)
	}
}

// TestAPartialImportNeverSweeps is §7.4's nightmare bug, guarded one layer above
// the tombstone: a stream that died mid-array has not observed an absence, it has
// observed nothing further. Every earlier return in FullImport is an error
// return, so the sweep is unreachable on that path — and this asserts the
// unreachability by its EFFECT rather than by reading the code.
func TestAPartialImportNeverSweeps(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	containers := []store.CatalogueContainer{{RemoteID: "1", Name: "Comics", Kind: "comic"}}

	if _, err := newImporter(t, s, &fakeSource{
		containers: containers, items: genItems(3, "1", "comic"),
	}).FullImport(t.Context(), inst); err != nil {
		t.Fatalf("seeding FullImport: %v", err)
	}

	// The stream dies after one element. The other two items are unobserved, not
	// absent.
	broken := &fakeSource{
		containers: containers, items: genItems(3, "1", "comic"),
		failAfter: 1, streamErr: errors.New("stream ended mid-array"),
	}
	im := newImporter(t, s, broken)
	im.BatchRows = 1
	rep, err := im.FullImport(t.Context(), inst)
	if err == nil {
		t.Fatal("the broken stream did not fail the import")
	}
	if rep.Completed {
		t.Fatal("a partial import reported itself complete")
	}
	if rep.Deletions != (store.SweepResult{}) {
		t.Errorf("a partial import swept: %+v", rep.Deletions)
	}
	for _, id := range []string{"0", "1", "2"} {
		if linkDeletedAt(t, s, inst, "series", id).Valid {
			t.Errorf("item %s was tombstoned by an import that never finished reading", id)
		}
	}
}

// TestARefusedSweepFailsTheImportRatherThanCompletingIt is
// ErrSweepRefusedEmptyRead's THIRD observability, and it is the one that had no
// guard: the doc claims the refusal is visible three ways — a sync_report row,
// the error reaching the importer, and *"the import that would have swept fails
// rather than reporting a completed run that quietly skipped its deletion pass"*
// — and only the first was asserted anywhere. Replacing this call site's
// `if err != nil` arm with `_ = err` survived the whole suite, so an importer
// change that swallowed the refusal would have shipped green and the refusal
// would have become a silent no-op at exactly the layer that is supposed to
// surface it.
//
// ⚠️ IT IS ASSERTED BY EFFECT, NOT BY THE ERROR ALONE. `rep.Completed` and
// `last_full_sync_at` are what a swallowed refusal actually produces: a run that
// tells the Services screen the replica is current, having skipped the pass that
// was going to make it current. Both are read here, and the freshness stamp is
// read against a DIFFERENT clock instant so "did not move" is a real question.
func TestARefusedSweepFailsTheImportRatherThanCompletingIt(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	containers := []store.CatalogueContainer{{RemoteID: "1", Name: "Comics", Kind: "comic"}}

	seed, err := newImporter(t, s, &fakeSource{
		containers: containers, items: genItems(3, "1", "comic"),
	}).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("seeding FullImport: %v", err)
	}
	if !seed.Completed {
		t.Fatal("the seeding import did not complete, so there are no live links to refuse over")
	}
	seeded, err := s.LastFullSyncAt(t.Context(), inst)
	if err != nil {
		t.Fatalf("LastFullSyncAt: %v", err)
	}
	if !seeded.Valid {
		t.Fatal("the seeding import stamped no freshness; the assertion below would be vacuous")
	}

	// THE READ COMES BACK EMPTY while the instance still has live links — a lost
	// credential scope, an unmounted share, a renamed container. A LATER clock, so
	// a stamp written by this run is distinguishable from the seeding run's.
	later := testNow.Add(24 * time.Hour)
	im := &Importer{
		Store: s, Source: &fakeSource{containers: containers}, UserID: store.SystemUserID,
		Now: func() time.Time { return later },
	}
	rep, err := im.FullImport(t.Context(), inst)

	if rep.Completed {
		t.Error("the import reported itself COMPLETE over a refused deletion pass: it claims to " +
			"have replaced what the replica knows about this source, having skipped the half " +
			"that removes what the source stopped reporting")
	}
	if !errors.Is(err, store.ErrSweepRefusedEmptyRead) {
		t.Errorf("FullImport err = %v, want ErrSweepRefusedEmptyRead; the refusal did not "+
			"reach the caller", err)
	}
	after, err := s.LastFullSyncAt(t.Context(), inst)
	if err != nil {
		t.Fatalf("LastFullSyncAt: %v", err)
	}
	if after.String != seeded.String {
		t.Errorf("last_full_sync_at moved from %q to %q: the Services screen now renders a "+
			"freshness this run did not earn", seeded.String, after.String)
	}
	// AND NOTHING WAS TOMBSTONED, which is the refusal doing its own job — the
	// assertions above would also pass for a sweep that ran and emptied the
	// library before failing.
	for _, id := range []string{"0", "1", "2"} {
		if linkDeletedAt(t, s, inst, "series", id).Valid {
			t.Errorf("item %s was tombstoned by the read the sweep refused to act on", id)
		}
	}
}

// TestADeltaSyncNeverSweeps is the same rule on the other channel, and it is the
// one that matters most: a delta walk returns only what CHANGED, so the set
// difference against it is the whole library. streamAndApply takes the seen-set
// as a NILABLE PARAMETER precisely so channel 3b cannot collect one, and this
// asserts the consequence against the real BookOrbit walk rather than a stub.
func TestADeltaSyncNeverSweeps(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 2}},
		map[int64][]bookorbit.Book{1: {
			bookAt(1, "Early", at(0)),
			bookAt(2, "Late", at(30)),
		}})

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	for _, id := range []string{"1", "2"} {
		if linkDeletedAt(t, f.st, f.instance, "book", id).Valid {
			t.Fatalf("book %s was tombstoned by the seeding import", id)
		}
	}

	// A LATER ARRIVAL AND NOTHING ELSE. The walk's arrivals filter returns book
	// 3 alone; books 1 and 2 are simply not in this read, which is the exact
	// input a set difference would read as "both deleted upstream".
	f.reader.books[1] = append(f.reader.books[1], bookAt(3, "Newest", at(60)))
	f.reopen()
	rep, err := f.importer().DeltaSync(t.Context(), f.instance)
	if err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if rep.DeclineReason != "" {
		t.Fatalf("the delta declined (%q) and escalated, so it never ran the walk this test is about",
			rep.DeclineReason)
	}
	// FEWER THAN THE THREE BOOKS THAT EXIST. The exact number is the lookbehind
	// window's business, not this test's — what matters is that at least one
	// book upstream is absent from this read, or there is no difference for a
	// sweep to have got wrong. Book 1 is the one: it is the oldest arrival and
	// no lookbehind reaches it.
	if rep.ItemsRead >= 3 {
		t.Fatalf("the delta read %d of 3 books, so it was not a partial read and this test proves nothing",
			rep.ItemsRead)
	}

	for _, id := range []string{"1", "2"} {
		if linkDeletedAt(t, f.st, f.instance, "book", id).Valid {
			t.Errorf("book %s was tombstoned by a DELTA walk that never claimed to have read it", id)
		}
	}
	if rep.Deletions != (store.SweepResult{}) {
		t.Errorf("a delta swept: %+v", rep.Deletions)
	}
}

// TestABookTheWalkReadButCouldNotMapIsNotTombstoned is the seen-set's OTHER
// definition, and the one that made the sweep disagree with the report on the
// same page.
//
// ⚠️ THE SEEN-SET USED TO BE WHAT THIS IMPORTER APPLIED, NOT WHAT THE UPSTREAM
// REPORTED. streamAndApply collects it from the batch, so a book the adapter
// read and declined to map never entered it — and BookOrbit declines exactly one
// class of book, the one whose MediaKind is unknown, which is what a book with
// no primary file reads as. A book still being processed therefore vanished from
// the replica, took its work with it, and was COUNTED in ItemsRead on the very
// report that recorded its deletion.
//
// It runs through the real BookOrbitSource and the real FullImport, because the
// defect lives in the seam between the adapter's classification and the
// importer's collection, and a fake source cannot have that seam.
func TestABookTheWalkReadButCouldNotMapIsNotTombstoned(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 2}},
		map[int64][]bookorbit.Book{1: {
			proseBook(1, "Kept"),
			proseBook(2, "Reprocessing"),
		}})

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("seeding FullImport: %v", err)
	}
	for _, id := range []string{"1", "2"} {
		if linkDeletedAt(t, f.st, f.instance, "book", id).Valid {
			t.Fatalf("book %s was tombstoned by the seeding import", id)
		}
	}

	// Book 2 loses its files — BookOrbit still lists it, and still counts it, but
	// classifies it as media kind unknown, which UsArr has no mapping for.
	unclassified := proseBook(2, "Reprocessing")
	unclassified.Files = nil
	if got := unclassified.MediaKind(); got != bookorbit.MediaKindUnknown {
		t.Fatalf("the fixture book classifies as %q, not unknown; this test's premise has moved", got)
	}
	f.reader.books[1] = []bookorbit.Book{proseBook(1, "Kept"), unclassified}
	f.reopen()

	rep, err := f.importer().FullImport(t.Context(), f.instance)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if rep.ItemsRead != 2 {
		t.Fatalf("the walk read %d books, want 2: the upstream must be REPORTING the book "+
			"this test says is not deleted", rep.ItemsRead)
	}

	// THE EFFECT, by identity: the book BookOrbit reported in this very read is
	// still live, link and work alike.
	if d := linkDeletedAt(t, f.st, f.instance, "book", "2"); d.Valid {
		t.Errorf("book 2 was tombstoned at %q by a read that reported it — ItemsRead says 2 "+
			"on the same report", d.String)
	}
	var workDeleted sql.NullString
	if err := f.st.DB().Read().QueryRowContext(t.Context(), `
		SELECT w.deleted_at FROM work w
		  JOIN service_item_link l ON l.work_id = w.id
		 WHERE l.service_instance_id = ? AND l.remote_kind = 'book' AND l.remote_id = '2'`,
		f.instance).Scan(&workDeleted); err != nil {
		t.Fatalf("read work: %v", err)
	}
	if workDeleted.Valid {
		t.Errorf("book 2's WORK was tombstoned at %q, so it is off every screen", workDeleted.String)
	}
	if rep.Deletions.LinksTombstoned != 0 || rep.Deletions.WorksTombstoned != 0 {
		t.Errorf("Deletions = %+v, want nothing moved", rep.Deletions)
	}
	// AND THE CONTAINER IS STILL VOUCHED FOR. The unmappable book is what kept
	// the container observed, so a fix that only widened the seen-set — without
	// carrying the container — would leave the whole library unswept forever and
	// pass every assertion above.
	if rep.Deletions.LinksUnobserved != 0 {
		t.Errorf("LinksUnobserved = %d, want 0: the container was walked and answered, so "+
			"its absences are still evidence", rep.Deletions.LinksUnobserved)
	}
}

// TestAContainerMeasuredShortIsNotSwept is the arm the completeness pass already
// had the answer to and nobody asked.
//
// bookorbitcompleteness.go computes, per container, whether UsArr's credential
// sees fewer books than the container holds. cmd/usarr read that verdict AFTER
// FullImport returned — that is, after the sweep had already deleted the
// difference. A container whose verdict is `shortfall` is by definition one
// whose read violated the sweep's precondition.
func TestAContainerMeasuredShortIsNotSwept(t *testing.T) {
	f := newDeltaFixture(t,
		[]bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 2}},
		map[int64][]bookorbit.Book{1: {proseBook(1, "One"), proseBook(2, "Two")}})

	if _, err := f.importer().FullImport(t.Context(), f.instance); err != nil {
		t.Fatalf("seeding FullImport: %v", err)
	}

	// The credential's view narrows: BookOrbit's own statistics say the library
	// holds 2 books, the listing shows UsArr only 1, and the walk delivers that
	// one. Without the completeness arm that is an ordinary absence and book 2 is
	// deleted.
	f.reader.libs = []bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 1}}
	f.reader.stats = map[int64]int64{1: 2}
	f.reader.books[1] = []bookorbit.Book{proseBook(1, "One")}
	f.reopen()

	rep, err := f.importer().FullImport(t.Context(), f.instance)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	// The premise: the adapter really did measure a shortfall.
	var short bool
	for _, c := range f.src.Completeness() {
		if c.State == store.CompletenessShortfall {
			short = true
		}
	}
	if !short {
		t.Fatalf("no container was measured short (%+v); this test would prove nothing",
			f.src.Completeness())
	}

	if d := linkDeletedAt(t, f.st, f.instance, "book", "2"); d.Valid {
		t.Errorf("book 2 was tombstoned at %q on a read UsArr had already measured as short — "+
			"the credential stopped seeing it, which is not the same as it being deleted", d.String)
	}
	if rep.Deletions.LinksUnobserved != 1 {
		t.Errorf("LinksUnobserved = %d, want 1 (%+v)", rep.Deletions.LinksUnobserved, rep.Deletions)
	}
	if rep.Deletions.LinksTombstoned != 0 {
		t.Errorf("LinksTombstoned = %d, want 0", rep.Deletions.LinksTombstoned)
	}
}

// TestADeclinedContainerReachesTheSweepAsReported is bindPhase's own rule,
// asserted through the path that applies it.
//
// ⚠️ THE STORE-SIDE TEST OF THIS NAME PASSES THE CONTAINER LIST TO SweepDeletions
// DIRECTLY and never runs bindPhase, so it cannot see the rule it is named for:
// making bindPhase skip declined containers when it builds `refs` is invisible
// to it. This one drives FullImport, so the list under test is the list bindPhase
// actually produced.
//
// The container must have been bound ONCE for the assertion to mean anything — a
// container declined from the first import has no library_source row to stamp,
// and a test over a table with no row in it passes over its own subject. So:
// bound on the first import, declined on the second, which is exactly the
// "an adapter's mapping changed" case the rule exists for.
func TestADeclinedContainerReachesTheSweepAsReported(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)

	bound := []store.CatalogueContainer{{RemoteID: "1", Name: "Comics", Kind: "comic"}}
	// ACCEPTED, because the library_source row is the thing the sweep could
	// wrongly stamp and since ADR-0048 an import writes none.
	acceptContainers(t, s, inst, bound...)
	items := genItems(2, "1", "comic")
	if _, err := newImporter(t, s, &fakeSource{containers: bound, items: items}).
		FullImport(t.Context(), inst); err != nil {
		t.Fatalf("seeding FullImport: %v", err)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM library_source`); n != 1 {
		t.Fatalf("%d library_source rows after the seeding import, want 1: there is nothing "+
			"for the sweep to wrongly stamp", n)
	}

	// The same container, now DECLINED: UsArr has no kind for it any more.
	declined := []store.CatalogueContainer{
		{RemoteID: "1", Name: "Comics", DeclineReason: "no kind for this container"},
	}
	rep, err := newImporter(t, s, &fakeSource{containers: declined, items: items}).
		FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if len(rep.DeclinedContainers) != 1 {
		t.Fatalf("the second import declined %d containers, want 1", len(rep.DeclinedContainers))
	}

	var missing sql.NullString
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT missing_since FROM library_source WHERE service_instance_id = ? AND container_ref = ?`,
		inst, "1").Scan(&missing); err != nil {
		t.Fatalf("read library_source: %v", err)
	}
	if missing.Valid {
		t.Errorf("the declined container was stamped missing at %q: an adapter that stopped "+
			"mapping a container is not an upstream that stopped reporting it", missing.String)
	}
	if rep.Deletions.SourcesMissing != 0 {
		t.Errorf("SourcesMissing = %d, want 0 (%+v)", rep.Deletions.SourcesMissing, rep.Deletions)
	}
}
