package store

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// Channel 4's deletion pass, against a REAL MIGRATED DATABASE with a populated
// corpus. Every assertion here is on the row the sweep was supposed to move, by
// identity — `WHERE remote_id = '41'`, `WHERE id = <the library>` — and never on
// a count of rows in a state. A count passes over its own deleted subject: a
// sweep that tombstoned the wrong two items leaves the same "2 tombstoned" that
// a correct one does.

var sweepLater = testNow.Add(72 * time.Hour)

// linkState is one link's tombstone and the tombstone of the work under it,
// looked up BY IDENTITY.
type linkState struct {
	linkDeleted sql.NullString
	workDeleted sql.NullString
	workID      int64
}

func readLink(t *testing.T, s *Store, inst int64, remoteKind, remoteID string) linkState {
	t.Helper()
	var st linkState
	err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT l.deleted_at, w.deleted_at, w.id
		  FROM service_item_link l JOIN work w ON w.id = l.work_id
		 WHERE l.service_instance_id = ? AND l.remote_kind = ? AND l.remote_id = ?`,
		inst, remoteKind, remoteID).Scan(&st.linkDeleted, &st.workDeleted, &st.workID)
	if err != nil {
		t.Fatalf("read link %s/%s: %v", remoteKind, remoteID, err)
	}
	return st
}

func nullStr(t *testing.T, s *Store, q string, args ...any) sql.NullString {
	t.Helper()
	var v sql.NullString
	if err := s.db.Read().QueryRowContext(t.Context(), q, args...).Scan(&v); err != nil {
		t.Fatalf("read %q: %v", q, err)
	}
	return v
}

// sweepFixture is one instance, two bound containers and three items, applied
// through the shipped write path rather than by hand — so the links carry the
// remote_identity_hash step 7 writes, which is the value guard 1 compares.
func sweepFixture(t *testing.T, s *Store) (inst int64, binds map[string]CatalogueBinding) {
	t.Helper()
	inst = fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Manga"),
		{RemoteID: "2", Name: "Ebooks", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	items := []CatalogueItem{
		item("41", "1", "comic", "Frieren"),
		item("42", "1", "comic", "Berserk"),
		item("77", "2", "book", "The Hobbit",
			ExternalIdentifier{Source: "hardcover_book", Value: "445", Confidence: 1.0}),
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, items, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	return inst, binds
}

func allSeen() []LinkRef {
	return []LinkRef{
		{RemoteKind: "series", RemoteID: "41"},
		{RemoteKind: "series", RemoteID: "42"},
		{RemoteKind: "series", RemoteID: "77"},
	}
}

// ── the item half ───────────────────────────────────────────────────────────

func TestSweepTombstonesWhatTheReadDidNotSeeAndSparesWhatItDid(t *testing.T) {
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	// 41 is gone upstream; 42 and 77 are still there.
	res, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}, {RemoteKind: "series", RemoteID: "77"}},
		[]string{"1", "2"}, sweepLater)
	if err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}

	gone := readLink(t, s, inst, "series", "41")
	if !gone.linkDeleted.Valid {
		t.Error("the absent item's link was not tombstoned")
	}
	if !gone.workDeleted.Valid {
		t.Error("the absent item's WORK was not tombstoned, so it is still on every screen")
	}
	if gone.linkDeleted.String != FormatTime(sweepLater) {
		t.Errorf("tombstoned at %q, want the sweep's own instant %q",
			gone.linkDeleted.String, FormatTime(sweepLater))
	}

	for _, id := range []string{"42", "77"} {
		st := readLink(t, s, inst, "series", id)
		if st.linkDeleted.Valid || st.workDeleted.Valid {
			t.Errorf("item %s was still in the read and was tombstoned anyway: link=%v work=%v",
				id, st.linkDeleted, st.workDeleted)
		}
	}

	// NOTHING IS HARD-DELETED. The row is what carries the user's tags and
	// requests through a temporarily-empty upstream (§7.4).
	if n := count(t, s, `SELECT COUNT(*) FROM service_item_link WHERE service_instance_id = ?`, inst); n != 3 {
		t.Errorf("service_item_link rows = %d, want 3: the sweep deletes nothing", n)
	}
	if res.LinksTombstoned != 1 || res.WorksTombstoned != 1 || res.LinksLive != 3 {
		t.Errorf("result = %+v, want 1 link and 1 work moved out of 3 live", res)
	}
}

func TestSweepSparesAWorkASecondInstanceStillReports(t *testing.T) {
	// THE `NOT EXISTS` IS NOT INSTANCE-SCOPED, and this is why. Two instances
	// report the same identified book; one stops. The link on the quiet instance
	// is tombstoned and the WORK is not, because it is still being reported.
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	other := fixtureInstance(t, s, "kavita-two")
	binds, _, err := s.BindContainers(t.Context(), other, SystemUserID,
		[]CatalogueContainer{{RemoteID: "9", Name: "Ebooks Two", Kind: "book"}})
	if err != nil {
		t.Fatalf("BindContainers other: %v", err)
	}
	// The SAME hardcover id, so §6.4 tier 1 resolves both links onto one work.
	shared := item("500", "9", "book", "The Hobbit",
		ExternalIdentifier{Source: "hardcover_book", Value: "445", Confidence: 1.0})
	if _, err := s.ApplyCatalogueBatch(t.Context(), other, binds, []CatalogueItem{shared}, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch other: %v", err)
	}
	if a, b := readLink(t, s, inst, "series", "77").workID, readLink(t, s, other, "series", "500").workID; a != b {
		t.Fatalf("the fixture did not share a work: %d vs %d — this test would prove nothing", a, b)
	}

	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "41"}, {RemoteKind: "series", RemoteID: "42"}},
		[]string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}

	st := readLink(t, s, inst, "series", "77")
	if !st.linkDeleted.Valid {
		t.Error("the quiet instance's link was not tombstoned")
	}
	if st.workDeleted.Valid {
		t.Error("the work was tombstoned while another instance still reports it")
	}
}

func TestSweepDistinguishesTwoKindsSharingOneRemoteID(t *testing.T) {
	// `remote_kind` IN THE TOMBSTONE'S WHERE CLAUSE IS CORRECTNESS, NOT ONLY A
	// QUERY PLAN. ux_sil admits (instance, 'book', '9') and (instance, 'series',
	// '9') as two rows, and ADR-0068's adapter writes both kinds — so an UPDATE
	// that matched on the instance and the id alone would tombstone a live item
	// because its NEIGHBOUR went away, with no error and no plan change to show
	// for it.
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	child := item("9", "1", "comic_issue", "Berserk 9")
	child.RemoteKind = "book"
	child.NumberText = sql.NullString{String: "9", Valid: true}
	child.Parent = &CatalogueParent{
		RemoteKind: "series", RemoteID: "9", Kind: "comic",
		Title: "Berserk", SortTitle: "Berserk", NormalizedTitle: "berserk", NormVersion: 1,
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{child}, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	// The SERIES is gone; the issue under it is still reported. Contrived on
	// purpose — it is the minimal input that separates the two rows.
	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "book", RemoteID: "9"}}, []string{"1"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}

	if !readLink(t, s, inst, "series", "9").linkDeleted.Valid {
		t.Error("the absent series link was not tombstoned")
	}
	if readLink(t, s, inst, "book", "9").linkDeleted.Valid {
		t.Error("the issue link was tombstoned because a DIFFERENT kind sharing its remote id went away")
	}
}

func TestSweepIsIdempotentOverItsOwnTombstones(t *testing.T) {
	// The second sweep must not re-stamp the first sweep's tombstone: the
	// 7-day window is measured from when the item went away, and a sweep that
	// restamped would push its own expiry out forever.
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	seen := []LinkRef{{RemoteKind: "series", RemoteID: "42"}, {RemoteKind: "series", RemoteID: "77"}}
	if _, err := s.SweepDeletions(t.Context(), inst, seen, []string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("first SweepDeletions: %v", err)
	}
	first := readLink(t, s, inst, "series", "41").linkDeleted

	later := sweepLater.Add(48 * time.Hour)
	res, err := s.SweepDeletions(t.Context(), inst, seen, []string{"1", "2"}, later)
	if err != nil {
		t.Fatalf("second SweepDeletions: %v", err)
	}
	again := readLink(t, s, inst, "series", "41").linkDeleted
	if again.String != first.String {
		t.Errorf("the tombstone moved from %q to %q on a second sweep", first.String, again.String)
	}
	if res.LinksTombstoned != 0 || res.WorksTombstoned != 0 {
		t.Errorf("the second sweep reported %+v, want no transitions", res)
	}
	if res.LinksLive != 2 {
		t.Errorf("LinksLive = %d, want 2: the tombstoned link is not live any more", res.LinksLive)
	}
}

// ── the container half: the ROADMAP acceptance ──────────────────────────────

func TestSweepStampsMissingSinceAndOrphanedAt(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	manga := binds["1"].LibraryID
	ebooks := binds["2"].LibraryID
	if manga == 0 || ebooks == 0 || manga == ebooks {
		t.Fatalf("the fixture bound %d and %d; this test needs two distinct libraries", manga, ebooks)
	}

	// Container "1" is gone upstream. Container "2" is still reported.
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), []string{"2"}, sweepLater)
	if err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}

	missing := nullStr(t, s, `
		SELECT missing_since FROM library_source
		 WHERE service_instance_id = ? AND container_kind = 'remote_library' AND container_ref = ?`,
		inst, "1")
	if !missing.Valid {
		t.Error("the absent container's library_source has no missing_since")
	}
	if missing.String != FormatTime(sweepLater) {
		t.Errorf("missing_since = %q, want %q", missing.String, FormatTime(sweepLater))
	}
	stillHere := nullStr(t, s, `
		SELECT missing_since FROM library_source
		 WHERE service_instance_id = ? AND container_kind = 'remote_library' AND container_ref = ?`,
		inst, "2")
	if stillHere.Valid {
		t.Errorf("a container the read reported was stamped missing at %q", stillHere.String)
	}

	orphaned := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, manga)
	if !orphaned.Valid {
		t.Error("the library whose only source went missing has no orphaned_at")
	}
	fed := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, ebooks)
	if fed.Valid {
		t.Errorf("a library with a live source was orphaned at %q", fed.String)
	}
	// ⚠️ library 0 is reserved and has no sources by construction; an orphan
	// rule that tested it would mark the one library that must always be there.
	if unfiled := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, UnfiledLibraryID); unfiled.Valid {
		t.Errorf("the reserved Unfiled library was orphaned at %q", unfiled.String)
	}
	if res.SourcesMissing != 1 || res.LibrariesOrphaned != 1 {
		t.Errorf("result = %+v, want one source missing and one library orphaned", res)
	}
}

func TestSweepStampsAbsenceOnceAndNeverRestampsIt(t *testing.T) {
	// FIRST OBSERVED ABSENT, not last. Migration 0005 calls missing_since the
	// instant "the upstream stopped reporting this container"; a sweep that
	// restamped would turn "missing since Tuesday" into "missing since just now"
	// on every run and destroy the only fact the column carries.
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	manga := binds["1"].LibraryID

	if _, err := s.SweepDeletions(t.Context(), inst, allSeen(), []string{"2"}, sweepLater); err != nil {
		t.Fatalf("first SweepDeletions: %v", err)
	}
	firstMissing := nullStr(t, s, `
		SELECT missing_since FROM library_source
		 WHERE service_instance_id = ? AND container_ref = ?`, inst, "1")
	firstOrphan := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, manga)
	if !firstMissing.Valid || !firstOrphan.Valid {
		t.Fatalf("the first sweep stamped nothing (%v / %v); the re-stamp assertion below would be vacuous",
			firstMissing, firstOrphan)
	}

	later := sweepLater.Add(96 * time.Hour)
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), []string{"2"}, later)
	if err != nil {
		t.Fatalf("second SweepDeletions: %v", err)
	}
	secondMissing := nullStr(t, s, `
		SELECT missing_since FROM library_source
		 WHERE service_instance_id = ? AND container_ref = ?`, inst, "1")
	secondOrphan := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, manga)

	if secondMissing.String != firstMissing.String {
		t.Errorf("missing_since moved from %q to %q", firstMissing.String, secondMissing.String)
	}
	if secondOrphan.String != firstOrphan.String {
		t.Errorf("orphaned_at moved from %q to %q", firstOrphan.String, secondOrphan.String)
	}
	if res.SourcesMissing != 0 || res.LibrariesOrphaned != 0 {
		t.Errorf("the second sweep reported %+v, want no transitions", res)
	}
}

func TestSweepClearsOrphanedAtWhenTheContainerComesBack(t *testing.T) {
	// A flag that can only be set is a library permanently marked orphaned the
	// moment its upstream blinks. bindOneContainer clears missing_since on the
	// way back in; orphaned_at has no other clearer anywhere, so the sweep owns
	// both directions.
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	manga := binds["1"].LibraryID

	if _, err := s.SweepDeletions(t.Context(), inst, allSeen(), []string{"2"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if !nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, manga).Valid {
		t.Fatal("the library was never orphaned; the restore assertion would be vacuous")
	}

	// The container is back, which is the ordinary bind path clearing
	// missing_since — the same statement a real import runs.
	if _, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{comicContainer("1", "Manga")}); err != nil {
		t.Fatalf("re-bind: %v", err)
	}
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), []string{"1", "2"}, sweepLater.Add(time.Hour))
	if err != nil {
		t.Fatalf("second SweepDeletions: %v", err)
	}
	if o := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, manga); o.Valid {
		t.Errorf("the library is still orphaned at %q with its source reported again", o.String)
	}
	if res.LibrariesRestored != 1 {
		t.Errorf("result = %+v, want one library restored", res)
	}
}

func TestSweepDoesNotStampADeclinedContainerAsMissing(t *testing.T) {
	// "UsArr has no kind for this" and "the upstream has stopped reporting this"
	// are different facts. bindPhase passes every container the upstream named,
	// declined ones included, and this asserts the distinction survives.
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	if _, err := s.SweepDeletions(t.Context(), inst, allSeen(), []string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	for _, ref := range []string{"1", "2"} {
		if m := nullStr(t, s, `
			SELECT missing_since FROM library_source
			 WHERE service_instance_id = ? AND container_ref = ?`, inst, ref); m.Valid {
			t.Errorf("container %s was reported and stamped missing at %q", ref, m.String)
		}
	}
}

// ── guard 1: id resurrection ────────────────────────────────────────────────

func TestATombstonedIDComingBackWithTheSameIdentityRevivesItsWork(t *testing.T) {
	// ⚠️ THE UNIDENTIFIED ITEM IS THE SUBJECT AND THAT IS DELIBERATE. An item
	// with a strong external id is recovered by §6.4 tier 1 whether the guard
	// resurrects its link or hard-deletes it, so it CANNOT tell a working guard
	// from a deleted comparison — both land on the same work and the test is
	// green either way. An item with no external ids has nothing to resolve
	// through: if the comparison stops running, the link is hard-deleted, tier 1
	// finds nothing, and the item comes back as a DUPLICATE work with the
	// original still tombstoned beside it. On a source whose ordinary state is
	// unidentified (§6.4, ADR-0035 §1) that is the whole catalogue.
	//
	// Item 41 carries no external ids; item 77 does, and is asserted alongside it
	// so the identified path is covered too.
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	unidentified := readLink(t, s, inst, "series", "41").workID
	identified := readLink(t, s, inst, "series", "77").workID

	// Both go away.
	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}}, []string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	for _, id := range []string{"41", "77"} {
		if !readLink(t, s, inst, "series", id).workDeleted.Valid {
			t.Fatalf("%s was not tombstoned; the resurrection below would not be one", id)
		}
	}

	// Both come back UNCHANGED: same remote ids, same identities.
	back := []CatalogueItem{
		item("41", "1", "comic", "Frieren"),
		item("77", "2", "book", "The Hobbit",
			ExternalIdentifier{Source: "hardcover_book", Value: "445", Confidence: 1.0}),
	}
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, back, sweepLater)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	if got := readLink(t, s, inst, "series", "41").workID; got != unidentified {
		t.Errorf("the unidentified item came back on work %d, want the original %d — "+
			"it was duplicated rather than revived", got, unidentified)
	}
	if got := readLink(t, s, inst, "series", "77").workID; got != identified {
		t.Errorf("the identified item came back on work %d, want the original %d", got, identified)
	}
	for _, id := range []string{"41", "77"} {
		st := readLink(t, s, inst, "series", id)
		if st.linkDeleted.Valid || st.workDeleted.Valid {
			t.Errorf("%s: the tombstone was not cleared for an item whose identity did not change", id)
		}
	}
	// AND NOTHING WAS LEFT BEHIND. A hard-delete-and-rebuild leaves the old work
	// tombstoned beside the new one; a revival leaves no such row.
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE deleted_at IS NOT NULL AND title IN ('Frieren','The Hobbit')`); n != 0 {
		t.Errorf("%d tombstoned works remain under the revived titles; they were duplicated", n)
	}
	if res.IDsReused != 0 {
		t.Errorf("guard 1 fired on an unchanged identity: %+v", res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportIDReused); n != 0 {
		t.Errorf("%d id_reused rows for an unchanged identity", n)
	}
}

func TestATombstonedIDReusedForDifferentContentDoesNotResurrectTheOldWork(t *testing.T) {
	// §7.4 guard 1 and reference/sync.md §4's pseudocode, literally: the
	// tombstoned link is hard-deleted, the id resolves as a fresh item, and the
	// old work KEEPS ITS TOMBSTONE rather than being silently rebound to content
	// that is not its own.
	//
	// The tombstone cannot be kept alongside a second row: ux_sil is a plain
	// UNIQUE index over (service_instance_id, remote_kind, remote_id), so those
	// three columns admit exactly one row whatever its deleted_at says.
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	old := readLink(t, s, inst, "series", "77").workID

	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "41"}, {RemoteKind: "series", RemoteID: "42"}},
		[]string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}

	// Remote id 77 is now a DIFFERENT book.
	reused := item("77", "2", "book", "Dune",
		ExternalIdentifier{Source: "hardcover_book", Value: "999", Confidence: 1.0})
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{reused}, sweepLater)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	after := readLink(t, s, inst, "series", "77")
	if after.workID == old {
		t.Fatalf("the reused id rebound onto work %d — the old book now renders as Dune", old)
	}
	if after.linkDeleted.Valid {
		t.Error("the fresh link was born tombstoned")
	}
	title := nullStr(t, s, `SELECT title FROM work WHERE id = ?`, after.workID)
	if title.String != "Dune" {
		t.Errorf("the new link points at %q, want Dune", title.String)
	}
	// THE OLD WORK IS ASSERTED BY IDENTITY, not by a count: it keeps its own
	// title and its own tombstone.
	oldTitle := nullStr(t, s, `SELECT title FROM work WHERE id = ?`, old)
	oldDeleted := nullStr(t, s, `SELECT deleted_at FROM work WHERE id = ?`, old)
	if oldTitle.String != "The Hobbit" {
		t.Errorf("the abandoned work is now titled %q; it was rewritten after all", oldTitle.String)
	}
	if !oldDeleted.Valid {
		t.Error("the abandoned work lost its tombstone")
	}
	if res.IDsReused != 1 {
		t.Errorf("guard 1 did not fire: %+v", res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportIDReused); n != 1 {
		t.Errorf("%d id_reused sync_report rows, want 1", n)
	}
}

func TestGuardOneDoesNotFireOnALiveLinkWhoseIdentityChanged(t *testing.T) {
	// THE GUARD IS SCOPED TO THE RESURRECTION PATH AND MUST STAY THERE. An
	// operator matching a book in the upstream changes its external ids on a link
	// that was never tombstoned; that is an ordinary update, not an id reuse, and
	// hard-deleting the link for it would discard the work on every re-match.
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	before := readLink(t, s, inst, "series", "41").workID

	matched := item("41", "1", "comic", "Frieren",
		ExternalIdentifier{Source: "hardcover_book", Value: "1234", Confidence: 1.0})
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{matched}, sweepLater)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	if got := readLink(t, s, inst, "series", "41").workID; got != before {
		t.Errorf("a live link's work moved from %d to %d on a metadata match", before, got)
	}
	if res.IDsReused != 0 {
		t.Errorf("guard 1 fired outside the resurrection path: %+v", res)
	}
}

// ── the query plans ─────────────────────────────────────────────────────────

// resurrectionPlanFaults is the judgement, extracted from the assertion so a
// firing arm can run it against a plan the shipped tree never produces —
// planlint_test.go's own construction, for its reason.
//
// THE NEEDLE IS THE WHOLE THREE-COLUMN CONSTRAINT AND NOT THE INDEX NAME. An
// assertion that only asked for `ux_sil` would still pass after `remote_kind`
// was dropped from the WHERE clause: SQLite keeps using the index on its leading
// column alone and reports a one-column seek that is a range scan over every
// link on the instance. Naming the constraint is what makes the guard
// discriminate between the seek that ships and the skip-scan that does not.
func resurrectionPlanFaults(plan string) []string {
	var faults []string
	if !strings.Contains(plan, "USING INDEX ux_sil (") {
		faults = append(faults, "the lookup does not ride ux_sil")
	}
	if !strings.Contains(plan, "(service_instance_id=? AND remote_kind=? AND remote_id=?)") {
		faults = append(faults, "ux_sil is not constrained on all three columns")
	}
	return faults
}

// sweepPlan is libraries_test.go's planOf plus the emptiness check every
// assertion below depends on: EXPLAIN QUERY PLAN returning no rows makes a
// `!strings.Contains` guard pass on the empty string.
// planOfShipped EXPLAINs a statement whose PLACEHOLDER COUNT is not a constant of
// this test, which is what lets it be pointed at the shipped `linkLookupSQL`
// identifier rather than at a copy: an edit that removes a predicate removes a
// placeholder with it, and an arm that hard-coded three arguments would then die
// on an argument-count error instead of reporting the plan it was written to
// judge. Positional values fill as far as they go; the rest bind NULL, which
// SQLite plans identically for an equality on an indexed column.
func planOfShipped(t *testing.T, s *Store, q string, inst int64) string {
	t.Helper()
	want := strings.Count(q, "?")
	have := []any{inst, "series", "77"}
	args := make([]any, want)
	for i := range args {
		if i < len(have) {
			args[i] = have[i]
		}
	}
	return sweepPlan(t, s, q, args...)
}

func sweepPlan(t *testing.T, s *Store, q string, args ...any) string {
	t.Helper()
	plan := planOf(t, s, q, args)
	if strings.TrimSpace(plan) == "" {
		t.Fatal("EXPLAIN QUERY PLAN returned nothing; every assertion below would be vacuous")
	}
	return plan
}

// analyzedSweepCorpus is a corpus big enough that ANALYZE has something to say,
// on ONE instance — which is the shape that makes the instance predicate
// unselective and is exactly the install this sweep runs on.
func analyzedSweepCorpus(t *testing.T, s *Store) int64 {
	t.Helper()
	inst, binds := sweepFixture(t, s)
	batch := make([]CatalogueItem, 0, 400)
	for i := range 400 {
		title := "Filler " + string(rune('A'+i%26))
		it := item(FormatTime(testNow.Add(time.Duration(i)*time.Second)), "1", "comic", title)
		it.RemoteID = "f" + it.RemoteID
		batch = append(batch, it)
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, batch, testNow); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.Analyze(t.Context()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return inst
}

// TestResurrectionLookupSeeksUxSilInFull pins the plan AFTER ANALYZE, which is
// the only state a real install is ever in: FullImport runs ANALYZE at the end
// of every successful import, and a plan measured on an unanalysed schema is a
// plan no install has.
func TestResurrectionLookupSeeksUxSilInFull(t *testing.T) {
	s := newTestStore(t)
	inst := analyzedSweepCorpus(t, s)

	plan := planOfShipped(t, s, linkLookupSQL, inst)
	if faults := resurrectionPlanFaults(plan); len(faults) > 0 {
		t.Errorf("the resurrection lookup's plan is wrong:\n  plan: %s\n  %s",
			plan, strings.Join(faults, "\n  "))
	}
	t.Logf("resurrection lookup: %s", plan)
}

// FIRING THE GUARD, on the mutation it exists for: `remote_kind` dropped from
// the WHERE clause. That is the edit a reader makes when the column looks
// redundant — the adapter only ever writes two kinds — and it is invisible to
// the compiler, to the tests and to the result of the query, which is identical.
// Only the plan changes.
//
// The degraded statement is a literal here rather than an edit to catalogue.go
// for the reason planlint_test.go extracts its own judgement: an arm that can
// only fire by mutating the shipped source is an arm that never fires.
func TestResurrectionPlanGuardFiresWhenRemoteKindIsDropped(t *testing.T) {
	s := newTestStore(t)
	inst := analyzedSweepCorpus(t, s)

	const kindless = `
		SELECT work_id, deleted_at, remote_identity_hash FROM service_item_link
		 WHERE service_instance_id = ? AND remote_id = ?`

	healthy := planOfShipped(t, s, linkLookupSQL, inst)
	if faults := resurrectionPlanFaults(healthy); len(faults) > 0 {
		t.Fatalf("the shipped plan was already faulty; this arm proves nothing:\n  %s", healthy)
	}

	degraded := planOfShipped(t, s, kindless, inst)
	faults := resurrectionPlanFaults(degraded)
	if len(faults) == 0 {
		t.Fatalf("the guard accepted a lookup with remote_kind dropped:\n  plan: %s", degraded)
	}
	t.Logf("guard fired without remote_kind:\n  plan:   %s\n  faults: %v", degraded, faults)
}

// TestTheInstanceSweepIsAllowedToScan is the other half, and it is an ACCEPTANCE
// rather than a demand.
//
// The sweep's own read has no predicate beyond the instance, and on a
// single-instance install that predicate selects the whole table — so after
// ANALYZE the correct plan is a SCAN, and a guard that demanded an index here
// would be red on every real install and would provoke exactly the index this
// slice does not need. What IS asserted is that the read stays on
// service_item_link and acquires no sort: a temp B-tree over every link on the
// instance is real work the sweep never asked for.
func TestTheInstanceSweepIsAllowedToScan(t *testing.T) {
	s := newTestStore(t)
	inst := analyzedSweepCorpus(t, s)

	plan := sweepPlan(t, s, `
		SELECT remote_kind, remote_id, work_id
		  FROM service_item_link
		 WHERE service_instance_id = ? AND deleted_at IS NULL`, inst)
	if !db.PlanHas(plan, "service_item_link") {
		t.Errorf("the sweep's read left service_item_link:\n  %s", plan)
	}
	if strings.Contains(plan, "TEMP B-TREE") {
		t.Errorf("the sweep's read acquired a sort it never asked for:\n  %s", plan)
	}
	t.Logf("instance sweep: %s", plan)
}
