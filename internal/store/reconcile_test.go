package store

import (
	"database/sql"
	"errors"
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

// TestTheSweepRefusesAZeroReadWhileTheInstanceHasLiveLinks is the zero-read
// refusal, and it is the arm that fires it.
//
// A read that returns success and an empty list is what a lost credential scope,
// an unmounted share and a renamed container all look like from here. Handed to
// the sweep unguarded, it is an absence for EVERY live link on the instance, and
// the pass tombstones the entire library.
func TestTheSweepRefusesAZeroReadWhileTheInstanceHasLiveLinks(t *testing.T) {
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	res, err := s.SweepDeletions(t.Context(), inst, nil, nil, sweepLater)
	// ⚠️ Errorf, NOT Fatalf, AND THE REASON IS THE MUTATION DRILL. Fatalf here
	// short-circuits every assertion below, so a tree with the refusal DELETED
	// reported only "the error is missing" — an assertion on the error's shape,
	// passing over its own subject. The damage the guard exists to prevent is the
	// tombstoned library, and this test must name it when it happens.
	if !errors.Is(err, ErrSweepRefusedEmptyRead) {
		t.Errorf("SweepDeletions err = %v, want ErrSweepRefusedEmptyRead", err)
	}

	// NOTHING MOVED. Asserted per row, by identity, and not as a count of rows
	// in a state — a count passes over its own deleted subject.
	for _, id := range []string{"41", "42", "77"} {
		st := readLink(t, s, inst, "series", id)
		if st.linkDeleted.Valid || st.workDeleted.Valid {
			t.Errorf("item %s was tombstoned by a refused sweep: link=%v work=%v",
				id, st.linkDeleted, st.workDeleted)
		}
	}
	// THE CONTAINER HALF IS REFUSED TOO. One failed read produced both seen-sets,
	// so a refusal that swept containers anyway would orphan every library on the
	// instance while claiming to have protected the items.
	for _, ref := range []string{"1", "2"} {
		if m := nullStr(t, s, `SELECT missing_since FROM library_source
			 WHERE service_instance_id = ? AND container_ref = ?`, inst, ref); m.Valid {
			t.Errorf("container %s was stamped missing by a refused sweep: %v", ref, m)
		}
	}

	// OBSERVABLE, NOT A SILENT NO-OP. The report row is written and COMMITTED —
	// returning the refusal as an error inside the transaction would have rolled
	// back the only durable record that the sweep declined.
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportDeletionSweepRefused); n != 1 {
		t.Errorf("%d deletion_sweep_refused rows, want 1: the refusal left no record", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportDeletionSweep); n != 0 {
		t.Errorf("%d deletion_sweep rows for a pass that did not run", n)
	}
	// The denominator rides the result and the row, so a reader can re-derive
	// the decision rather than take it on trust.
	if res.LinksLive != 3 {
		t.Errorf("LinksLive = %d, want 3", res.LinksLive)
	}
	d := nullStr(t, s, `SELECT detail FROM sync_report
		 WHERE service_instance_id = ? AND kind = ?`, inst, SyncReportDeletionSweepRefused)
	if !strings.Contains(d.String, `"links_live":3`) {
		t.Errorf("the refusal detail does not carry the live count: %s", d.String)
	}
}

// TestAGenuinelyEmptyInstanceStillSweeps is the OTHER half of the same rule, and
// without it the refusal is untested in the direction that matters most.
//
// Zero read against zero live links is not a failed read — it is an instance
// whose content really has gone, and the pass has real work to do in the
// container half. A guard that refused here would be a guard that never lets a
// library be orphaned, which is the state §6.5 rule 5 exists to record.
func TestAGenuinelyEmptyInstanceStillSweeps(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	manga := binds["1"].LibraryID
	ebooks := binds["2"].LibraryID
	if manga == 0 || ebooks == 0 || manga == ebooks {
		t.Fatalf("the fixture bound %d and %d; this test needs two distinct libraries", manga, ebooks)
	}

	// Empty the ITEM half first, through the ordinary pass. The seed read is
	// NON-EMPTY — one ref for an item that does not exist — so it is not itself a
	// refusal, and it reports none of the three real items, which tombstones all
	// of them.
	//
	// ⚠️ IT REPORTS BOTH CONTAINERS, and that is what makes the assertions below
	// measure something. A seed that passed nil containers would stamp
	// missing_since and orphan both libraries itself, leaving the sweep under test
	// with no transition left to make — and the test would then be green whether
	// the container half ran or not.
	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "no-such-item"}},
		[]string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("seed sweep: %v", err)
	}
	// Now every link is tombstoned, so the instance has no LIVE links at all.
	if n := count(t, s, `SELECT COUNT(*) FROM service_item_link
		 WHERE service_instance_id = ? AND deleted_at IS NULL`, inst); n != 0 {
		t.Fatalf("%d live links remain; this test's premise does not hold", n)
	}
	for _, ref := range []string{"1", "2"} {
		if m := nullStr(t, s, `SELECT missing_since FROM library_source
			 WHERE service_instance_id = ? AND container_ref = ?`, inst, ref); m.Valid {
			t.Fatalf("the seed already stamped container %s missing; "+
				"the sweep under test has no work left to prove it did", ref)
		}
	}

	res, err := s.SweepDeletions(t.Context(), inst, nil, nil, sweepLater)
	if err != nil {
		t.Errorf("a genuinely empty instance was refused: %v", err)
	}

	// ⚠️ THE ASSERTION IS THE CONTAINER HALF'S WORK, NOT THE ABSENCE OF AN ERROR.
	// "Still sweeps" is a claim about what the pass DID, and a test that only
	// checked `err == nil` and a sync_report count passed over its own subject:
	// it stayed green for a refusal made unconditional right up until the error
	// changed, and said nothing at all about the stamping this instance needs.
	// This is how a library whose last container went away gets orphaned.
	for _, ref := range []string{"1", "2"} {
		m := nullStr(t, s, `SELECT missing_since FROM library_source
			 WHERE service_instance_id = ? AND container_ref = ?`, inst, ref)
		if !m.Valid {
			t.Errorf("container %s was not stamped missing; the container half did not run", ref)
			continue
		}
		if m.String != FormatTime(sweepLater) {
			t.Errorf("container %s missing_since = %q, want %q", ref, m.String, FormatTime(sweepLater))
		}
	}
	for what, id := range map[string]int64{"manga": manga, "ebooks": ebooks} {
		if o := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, id); !o.Valid {
			t.Errorf("library %s (%d) lost its last source and was not orphaned", what, id)
		}
	}
	if res.SourcesMissing != 2 || res.LibrariesOrphaned != 2 {
		t.Errorf("result = %+v, want two sources missing and two libraries orphaned", res)
	}
	if res.LinksLive != 0 {
		t.Errorf("LinksLive = %d, want 0", res.LinksLive)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportDeletionSweepRefused); n != 0 {
		t.Errorf("%d deletion_sweep_refused rows for an instance that had nothing to protect", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportDeletionSweep); n == 0 {
		t.Error("the pass ran but wrote no deletion_sweep row")
	}
}

// TestTheZeroReadRefusalIsNotAThreshold records the boundary the ruling drew, as
// a test rather than only as a comment: ONE item read against an instance with
// three live links SWEEPS, and takes two of them with it.
//
// ⚠️ THAT IS A LARGE PROPORTIONAL DROP AND IT IS DELIBERATELY UNGUARDED. Refusing
// it needs a percentage somebody has to defend and tune, and no ruling sets one.
// This test is here so a later reader adding a threshold has to delete an
// assertion that says the threshold was declined, rather than filling a silence.
func TestTheZeroReadRefusalIsNotAThreshold(t *testing.T) {
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	res, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}}, []string{"1", "2"}, sweepLater)
	if err != nil {
		t.Fatalf("a one-item read was refused; the rule is zero, not a proportion: %v", err)
	}
	if res.LinksTombstoned != 2 {
		t.Errorf("LinksTombstoned = %d, want 2: two of three live links went", res.LinksTombstoned)
	}
}

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

// TestATombstonedIDComingBackWithTheSameIdentityRevivesItsWork is the IDENTIFIED
// half of guard 1's non-firing case: stored identity and incoming identity both
// exist and agree, which is the only state that is positive evidence of sameness.
//
// ⚠️ THIS TEST USED TO CARRY THE UNIDENTIFIED ITEM AS ITS SUBJECT AND ASSERT THAT
// IT REVIVED. That assertion is INVERTED rather than deleted — see
// TestAnUnidentifiedItemOnAReusedIDDoesNotRebindOntoTheOldWork below, which
// asserts the opposite outcome for the same input — because an inverted assertion
// records that a decision changed and a deleted one is silence. The old subject's
// premise was that an equality proved sameness; guardOneVerdict's comment carries
// why that equality held VACUOUSLY for an item with no external ids.
func TestATombstonedIDComingBackWithTheSameIdentityRevivesItsWork(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	identified := readLink(t, s, inst, "series", "77").workID

	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}}, []string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if !readLink(t, s, inst, "series", "77").workDeleted.Valid {
		t.Fatal("77 was not tombstoned; the resurrection below would not be one")
	}

	// It comes back UNCHANGED: same remote id, same identity.
	back := []CatalogueItem{
		item("77", "2", "book", "The Hobbit",
			ExternalIdentifier{Source: "hardcover_book", Value: "445", Confidence: 1.0}),
	}
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, back, sweepLater)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	if got := readLink(t, s, inst, "series", "77").workID; got != identified {
		t.Errorf("the identified item came back on work %d, want the original %d", got, identified)
	}
	st := readLink(t, s, inst, "series", "77")
	if st.linkDeleted.Valid || st.workDeleted.Valid {
		t.Error("the tombstone was not cleared for an item whose identity did not change")
	}
	// AND NOTHING WAS LEFT BEHIND. A hard-delete-and-rebuild leaves the old work
	// tombstoned beside the new one; a revival leaves no such row.
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE deleted_at IS NOT NULL AND title = 'The Hobbit'`); n != 0 {
		t.Errorf("%d tombstoned works remain under the revived title; it was duplicated", n)
	}
	if res.IDsReused != 0 {
		t.Errorf("guard 1 fired on an unchanged identity: %+v", res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportIDReused); n != 0 {
		t.Errorf("%d id_reused rows for an unchanged identity", n)
	}
}

// TestAnUnidentifiedItemOnAReusedIDDoesNotRebindOntoTheOldWork is guard 1's
// third state — guardOneNoIncomingIdentity — and it is THE ASSERTION THAT WAS
// VACUOUS BEFORE.
//
// # What was wrong
//
// identityHash over an item with no external ids is the hash of an EMPTY LIST: a
// single constant that every unidentified item on the instance shares. The guard
// compared that constant to itself, found equality, and read the equality as
// "same content". So a tombstoned link belonging to one book was revived for a
// completely different book that happened to arrive on the same reused remote id,
// and the two merged into one work — silently, with the first book's poster,
// tags, requests and northbound id now naming the second.
//
// AN EQUALITY THAT HOLDS VACUOUSLY IS NOT EVIDENCE. The subject here is the exact
// input that used to pass: item 41 carries no external ids, goes away, and comes
// back on the same remote id as DIFFERENT CONTENT.
//
// # Why a fresh link and not a revival
//
// The two errors are not symmetric. A wrong revival MERGES two books and is
// unrecoverable — nothing records what was overwritten. A wrong fresh link SPLITS
// one book into two visible rows, and the tombstoned work keeps its own
// corrections, so a later merge puts it back. The state with no evidence either
// way therefore falls to the fresh-link side, and must NOT take the same branch as
// a real match.
func TestAnUnidentifiedItemOnAReusedIDDoesNotRebindOntoTheOldWork(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	original := readLink(t, s, inst, "series", "41").workID

	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}}, []string{"1", "2"}, sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if !readLink(t, s, inst, "series", "41").workDeleted.Valid {
		t.Fatal("41 was not tombstoned; the resurrection below would not be one")
	}

	// The upstream reuses id 41 for something else entirely. It carries no
	// external ids either — which is the point: BOTH sides hash to the empty
	// list, so the old comparison saw them as the same book.
	back := []CatalogueItem{item("41", "1", "comic", "Vinland Saga")}
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, back, sweepLater)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	got := readLink(t, s, inst, "series", "41").workID
	if got == original {
		t.Errorf("the reused id rebound onto work %d, the work that belonged to Frieren — "+
			"two unrelated books are now one row, and nothing records what was overwritten", original)
	}
	if title := nullStr(t, s, `SELECT title FROM work WHERE id = ?`, got).String; title != "Vinland Saga" {
		t.Errorf("the new link points at work %d titled %q, want the new content", got, title)
	}

	// THE ABANDONED WORK KEEPS ITS TOMBSTONE AND ITS OWN CORRECTIONS. That is
	// what makes the split recoverable, which is the whole reason this branch is
	// the safe one.
	if del := nullStr(t, s, `SELECT deleted_at FROM work WHERE id = ?`, original); !del.Valid {
		t.Errorf("work %d lost its tombstone; it was adopted rather than left alone", original)
	}
	if title := nullStr(t, s, `SELECT title FROM work WHERE id = ?`, original).String; title != "Frieren" {
		t.Errorf("the abandoned work is titled %q; its content was overwritten", title)
	}

	// AND THE REFUSAL IS ON THE RECORD. A guard that declined to certify a match
	// but said nothing would be indistinguishable from one that never ran.
	if res.IDsReused != 1 {
		t.Errorf("IDsReused = %d, want 1: guard 1 did not report the firing (%+v)", res.IDsReused, res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report
		 WHERE service_instance_id = ? AND kind = ? AND detail LIKE '%"verdict":"no_incoming_identity"%'`,
		inst, SyncReportIDReused); n != 1 {
		t.Errorf("%d id_reused rows carry the no_incoming_identity verdict, want 1 — "+
			"a reader cannot tell a real id reuse from an unidentified library without it", n)
	}
}

// TestGuardOneSeparatesItsFourStates is the unit half, on the decision function
// itself rather than through a whole batch apply.
//
// It exists because the four states collapse into two OUTCOMES, and a table that
// only ever checked the outcome would be green again the day two states were
// merged back — which is the defect this type was introduced to prevent.
func TestGuardOneSeparatesItsFourStates(t *testing.T) {
	withIDs := item("1", "1", "comic", "Frieren",
		ExternalIdentifier{Source: "hardcover_book", Value: "445", Confidence: 1.0})
	bare := item("1", "1", "comic", "Frieren")

	for _, c := range []struct {
		name  string
		hash  sql.NullString
		it    CatalogueItem
		want  guardOneVerdict
		fires bool
	}{
		{"nothing stored", sql.NullString{}, withIDs, guardOneIdentityUnknown, false},
		{
			"stored, incoming carries none",
			sql.NullString{String: "abc", Valid: true},
			bare, guardOneNoIncomingIdentity, true,
		},
		{
			"both present, differ",
			sql.NullString{String: "abc", Valid: true},
			withIDs, guardOneIdentityChanged, true,
		},
		{
			"both present, agree",
			sql.NullString{String: withIDs.identityHash(), Valid: true},
			withIDs, guardOneIdentityMatches, false,
		},
		// THE INTERSECTION, ruled at the seam: nothing stored AND nothing
		// incoming answers "unknown", because that is the older and narrower
		// rule. Unreachable today — no shipped writer leaves the column NULL.
		{"nothing stored, incoming carries none", sql.NullString{}, bare, guardOneIdentityUnknown, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := guardOne(c.hash, c.it)
			if got != c.want {
				t.Errorf("guardOne = %s, want %s", got, c.want)
			}
			if got.firesGuard() != c.fires {
				t.Errorf("%s.firesGuard() = %v, want %v", got, got.firesGuard(), c.fires)
			}
		})
	}

	// THE VACUOUS EQUALITY, NAMED. Two unrelated bare items hash identically —
	// that is the fact the guard used to read as proof of sameness, and it is
	// still true. What changed is that the length check runs first.
	other := item("2", "1", "comic", "Vinland Saga")
	if bare.identityHash() != other.identityHash() {
		t.Fatal("two items with no external ids no longer share a hash; " +
			"this test's whole premise, and the bug it guards, have moved")
	}
	if v := guardOne(sql.NullString{String: other.identityHash(), Valid: true}, bare); !v.firesGuard() {
		t.Errorf("guardOne returned %s for two unrelated unidentified books whose hashes are "+
			"equal; the equality is vacuous and the guard treated it as evidence", v)
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

// instanceSweepPlanFaults judges the unqualified instance sweep's plan.
//
// ⚠️ THE ACCEPTANCE IS NARROW AND IT IS SCOPED TO ONE STATEMENT. A SCAN is fine
// HERE — the read has no predicate beyond the instance and on a single-instance
// install that predicate selects the whole table, so after ANALYZE a SCAN is the
// correct plan and a guard demanding an index would be red on every real install
// while provoking an index nothing needs. It is NOT a general licence to scan
// service_item_link: linkLookupSQL's three-column seek on ux_sil stays a demand,
// judged by resurrectionPlanFaults and fired by
// TestResurrectionPlanGuardFiresWhenRemoteKindIsDropped.
//
// ⚠️ AND IT ASSERTS WHAT IT MEANS. The previous version asked only whether the
// string `service_item_link` appeared anywhere in the plan and whether a temp
// B-tree did not — which a plan that scanned service_item_link AND joined
// something else would satisfy, and which a plan that had stopped scanning
// service_item_link at all could satisfy by naming it in a subquery. Three things
// are checked instead: the plan is ONE step, that step SCANS, and the table it
// scans is service_item_link.
func instanceSweepPlanFaults(plan string) []string {
	var faults []string
	if strings.Contains(plan, " | ") {
		faults = append(faults, "the read is no longer a single step; it acquired a join or a subquery")
	}
	if !db.PlanHas(plan, "SCAN service_item_link") {
		faults = append(faults, "the read is not a SCAN of service_item_link")
	}
	if strings.Contains(plan, "TEMP B-TREE") {
		faults = append(faults, "the read acquired a sort it never asked for")
	}
	return faults
}

// TestTheInstanceSweepIsAllowedToScan is the other half of the plan gate, and it
// is an ACCEPTANCE rather than a demand. instanceSweepPlanFaults carries the
// argument for how narrow that acceptance is.
//
// It EXPLAINs absentLinksSQL — the shipped identifier, not a copy. The statement
// was an inline literal in reconcile.go with this test EXPLAINing a hand-written
// duplicate, which is the shape linkLookupSQL's own comment warns against: a test
// that EXPLAINs its own copy of a query is green while the copy is faithful and
// silent the moment it stops being. Measured: adding an ORDER BY to the shipped
// literal on the pre-fix tree left this test GREEN.
func TestTheInstanceSweepIsAllowedToScan(t *testing.T) {
	s := newTestStore(t)
	inst := analyzedSweepCorpus(t, s)

	plan := sweepPlan(t, s, absentLinksSQL, inst)
	if faults := instanceSweepPlanFaults(plan); len(faults) > 0 {
		t.Errorf("the instance sweep's plan is wrong:\n  plan: %s\n  %s",
			plan, strings.Join(faults, "\n  "))
	}
	t.Logf("instance sweep: %s", plan)
}

// FIRING THE ACCEPTANCE, on the two mutations it exists to catch. Both are edits
// a reader plausibly makes, and both were INVISIBLE to the previous assertions:
// the sorted variant still contains `service_item_link`, and the joined variant
// contains it too while doing a table's worth of extra work per sweep.
//
// The degraded statements are literals here rather than edits to reconcile.go for
// the reason planlint_test.go extracts its own judgement: an arm that can only
// fire by mutating the shipped source is an arm that never fires.
func TestTheInstanceSweepAcceptanceFiresOnASortAndOnAJoin(t *testing.T) {
	s := newTestStore(t)
	inst := analyzedSweepCorpus(t, s)

	healthy := sweepPlan(t, s, absentLinksSQL, inst)
	if faults := instanceSweepPlanFaults(healthy); len(faults) > 0 {
		t.Fatalf("the shipped plan was already faulty; this arm proves nothing:\n  %s", healthy)
	}

	for _, m := range []struct {
		what string
		sql  string
	}{
		{"an ORDER BY the sweep never asked for", `
			SELECT remote_kind, remote_id, work_id
			  FROM service_item_link
			 WHERE service_instance_id = ? AND deleted_at IS NULL
			 ORDER BY remote_id`},
		{"a join onto work", `
			SELECT l.remote_kind, l.remote_id, l.work_id
			  FROM service_item_link l JOIN work w ON w.id = l.work_id
			 WHERE l.service_instance_id = ? AND l.deleted_at IS NULL`},
	} {
		degraded := sweepPlan(t, s, m.sql, inst)
		faults := instanceSweepPlanFaults(degraded)
		if len(faults) == 0 {
			t.Errorf("the acceptance passed %s:\n  plan: %s", m.what, degraded)
			continue
		}
		t.Logf("acceptance fired on %s:\n  plan:   %s\n  faults: %v", m.what, degraded, faults)
	}
}
