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
	// THE LIBRARIES EXIST BECAUSE SOMEBODY ACCEPTED THEM (ADR-0048): the bind
	// path creates none, and the sweep's whole subject — missing_since on a
	// source, orphaned_at on a library — needs sources to exist.
	binds = acceptedBind(t, s, inst,
		comicContainer("1", "Manga"),
		CatalogueContainer{RemoteID: "2", Name: "Ebooks", Kind: "book"},
	)
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

// bothContainers is the fixture's whole container list, reported AND observed:
// two containers the read named and delivered items from. It is spelled once
// because SweepScope's two fields are the same type and the risk it exists to
// answer is a transposition nothing else would catch.
func bothContainers() SweepScope {
	return SweepScope{Reported: []string{"1", "2"}, Observed: []string{"1", "2"}}
}

// oneContainer is a read that named and delivered exactly one of the fixture's
// two containers — the other is not reported at all, which is what a container
// that went away looks like.
func oneContainer(ref string) SweepScope {
	return SweepScope{Reported: []string{ref}, Observed: []string{ref}}
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

	res, err := s.SweepDeletions(t.Context(), inst, nil, SweepScope{}, sweepLater)
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
		bothContainers(), sweepLater); err != nil {
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

	res, err := s.SweepDeletions(t.Context(), inst, nil, SweepScope{}, sweepLater)
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
// a test rather than only as a comment: INSIDE ONE OBSERVED CONTAINER, a read
// that reports one of its two items sweeps the other.
//
// ⚠️ THAT IS A LARGE PROPORTIONAL DROP AND IT IS DELIBERATELY UNGUARDED. Refusing
// it needs a percentage somebody has to defend and tune, and no ruling sets one.
// This test is here so a later reader adding a threshold has to delete an
// assertion that says the threshold was declined, rather than filling a silence.
//
// ⚠️ THE SCOPE IT PASSES CHANGED WITH THE PER-CONTAINER RULE, and the change is
// what keeps the test honest rather than what weakens it. It used to hand the
// sweep BOTH containers as observed while reporting an item from only one of
// them — a scope no real read produces, and one that made the assertion below
// count a container's protection as a threshold decision. Container 1 is
// observed and its unreported item goes; container 2 reported nothing and its
// item stays, which is TestOneDarkContainerDoesNotTombstoneItsItems' subject and
// not this one's.
func TestTheZeroReadRefusalIsNotAThreshold(t *testing.T) {
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	res, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}},
		SweepScope{Reported: []string{"1", "2"}, Observed: []string{"1"}}, sweepLater)
	if err != nil {
		t.Fatalf("a one-item read was refused; the rule is zero, not a proportion: %v", err)
	}
	// BY IDENTITY: 41 shared container 1 with the item that WAS reported, and
	// half of that container going away is not a refusal.
	if !readLink(t, s, inst, "series", "41").linkDeleted.Valid {
		t.Error("item 41 was in an observed container, was not reported, and survived: " +
			"a proportional drop inside one container is not guarded")
	}
	if res.LinksTombstoned != 1 {
		t.Errorf("LinksTombstoned = %d, want 1", res.LinksTombstoned)
	}
}

// TestOneDarkContainerDoesNotTombstoneItsItems is the per-container half of the
// zero-read rule, and it is the failure the instance-wide refusal structurally
// cannot see.
//
// ⚠️ THE FAILURE IS CONTAINER-SHAPED AND THE OLD GUARD WAS INSTANCE-SHAPED. Every
// mode ErrSweepRefusedEmptyRead names — a credential that lost its scope, an
// unmounted share, a library the operator renamed, a body that parsed to an
// empty array — happens PER LIBRARY on a source that is a list of libraries, and
// BookOrbit's walk is literally one StreamBooks call per library. One of those
// libraries answering with nothing while the others answer normally produced a
// non-empty read, no refusal, and every work in the dark container tombstoned —
// the container stamped missing AND its items deleted on the same event, which is
// how an unmounted share becomes a deleted library.
//
// The rule needs no threshold and no percentage: it is the same "zero from a
// source the replica knows had content" rule, evaluated per container.
func TestOneDarkContainerDoesNotTombstoneItsItems(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	manga := binds["1"].LibraryID

	// Container 1 went dark: it is not reported at all and delivered nothing.
	// Container 2 answered normally and reported its one item.
	res, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "77"}}, oneContainer("2"), sweepLater)
	if err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}

	// THE ITEMS IN THE DARK CONTAINER SURVIVE, asserted per row by identity.
	for _, id := range []string{"41", "42"} {
		st := readLink(t, s, inst, "series", id)
		if st.linkDeleted.Valid || st.workDeleted.Valid {
			t.Errorf("item %s lives in the container that went dark and was tombstoned "+
				"anyway: link=%v work=%v — one unmounted share deleted a library",
				id, st.linkDeleted, st.workDeleted)
		}
	}
	// AND THE CONTAINER STAMP STILL HAPPENS. The two facts are independent: the
	// upstream really has stopped naming this container, and that is what the
	// Libraries screen renders. What must not follow from it is the item
	// tombstone.
	if m := nullStr(t, s, `SELECT missing_since FROM library_source
		 WHERE service_instance_id = ? AND container_ref = ?`, inst, "1"); !m.Valid {
		t.Error("the dark container was not stamped missing; the container half stopped " +
			"running instead of the item half being scoped")
	}
	if o := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, manga); !o.Valid {
		t.Error("the library whose only source went dark was not orphaned")
	}

	// AND IT IS COUNTED, because a link that was protected looks exactly like a
	// link that was reported once the sweep has finished.
	if res.LinksUnobserved != 2 {
		t.Errorf("LinksUnobserved = %d, want 2: the protection left no number behind (%+v)",
			res.LinksUnobserved, res)
	}
	if res.LinksTombstoned != 0 {
		t.Errorf("LinksTombstoned = %d, want 0", res.LinksTombstoned)
	}
	d := nullStr(t, s, `SELECT detail FROM sync_report
		 WHERE service_instance_id = ? AND kind = ?`, inst, SyncReportDeletionSweep)
	if !strings.Contains(d.String, `"links_unobserved":2`) {
		t.Errorf("the sweep's own row does not carry what it declined to touch: %s", d.String)
	}
}

// TestAContainerMeasuredShortDoesNotHaveItsItemsSwept is the same protection
// reached the other way: the container ANSWERED, and the answer is known to be
// incomplete.
//
// The store cannot measure that — the completeness verdict lives in the adapter —
// so what is asserted here is that the mechanism honours a caller that reports a
// container as reported-but-not-observed. internal/libsync's
// TestAShortContainerIsNotSwept drives the same rule through the real
// BookOrbit completeness pass.
func TestAContainerMeasuredShortDoesNotHaveItsItemsSwept(t *testing.T) {
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	// Container 1 is still named by the upstream and delivered item 42; the
	// caller has measured its read as short, so it is not in Observed.
	res, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}, {RemoteKind: "series", RemoteID: "77"}},
		SweepScope{Reported: []string{"1", "2"}, Observed: []string{"2"}}, sweepLater)
	if err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if st := readLink(t, s, inst, "series", "41"); st.linkDeleted.Valid {
		t.Error("item 41 was tombstoned on a read its own caller measured as short")
	}
	if m := nullStr(t, s, `SELECT missing_since FROM library_source
		 WHERE service_instance_id = ? AND container_ref = ?`, inst, "1"); m.Valid {
		t.Errorf("a container the read REPORTED was stamped missing at %q because its items "+
			"were not vouched for; the two fields answer different questions", m.String)
	}
	if res.LinksUnobserved != 1 {
		t.Errorf("LinksUnobserved = %d, want 1 (%+v)", res.LinksUnobserved, res)
	}
}

func TestSweepTombstonesWhatTheReadDidNotSeeAndSparesWhatItDid(t *testing.T) {
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	// 41 is gone upstream; 42 and 77 are still there.
	res, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}, {RemoteKind: "series", RemoteID: "77"}},
		bothContainers(), sweepLater)
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
		bothContainers(), sweepLater); err != nil {
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
		[]LinkRef{{RemoteKind: "book", RemoteID: "9"}}, oneContainer("1"), sweepLater); err != nil {
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
	if _, err := s.SweepDeletions(t.Context(), inst, seen, bothContainers(), sweepLater); err != nil {
		t.Fatalf("first SweepDeletions: %v", err)
	}
	first := readLink(t, s, inst, "series", "41").linkDeleted

	later := sweepLater.Add(48 * time.Hour)
	res, err := s.SweepDeletions(t.Context(), inst, seen, bothContainers(), later)
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
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), oneContainer("2"), sweepLater)
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

	if _, err := s.SweepDeletions(t.Context(), inst, allSeen(), oneContainer("2"), sweepLater); err != nil {
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
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), oneContainer("2"), later)
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

	if _, err := s.SweepDeletions(t.Context(), inst, allSeen(), oneContainer("2"), sweepLater); err != nil {
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
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), bothContainers(), sweepLater.Add(time.Hour))
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

// secondSourceOn attaches an existing library to a second service instance, the
// way a library fed by two upstreams looks in the row. It is written directly
// because BindContainers mints a library per container and this fixture needs
// two instances pointing at ONE.
func secondSourceOn(t *testing.T, s *Store, libraryID, inst int64, ref string) {
	t.Helper()
	if _, err := s.db.Writer().ExecContext(t.Context(), `
		INSERT INTO library_source
		  (library_id, service_instance_id, container_kind, container_ref, container_identity)
		VALUES (?, ?, 'remote_library', ?, ?)`,
		libraryID, inst, ref, "container "+ref); err != nil {
		t.Fatalf("seed second source: %v", err)
	}
}

// TestSoftDeletingAnInstanceOrphansTheLibrariesItWasTheLastSourceOf is the
// orphan state's OTHER writer, and until it existed the state was unreachable
// for the case that produces it most obviously.
//
// A soft-deleted instance keeps its library_source rows — the FK cascade fires
// on a HARD delete only — and librarySourcesSQL hides them, so the library
// rendered with no sources at all. It was orphaned in fact and un-orphaned in the
// data, permanently: no sweep runs for a deleted instance, so nothing was ever
// going to stamp it.
//
// ⚠️ ASSERTED BY IDENTITY, NOT BY A COUNT. Two libraries, told apart by which
// instance feeds them: the one whose last source went with the deleted instance
// is stamped, and the one another live instance still feeds is not. A count would
// pass for a delete that orphaned everything.
func TestSoftDeletingAnInstanceOrphansTheLibrariesItWasTheLastSourceOf(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	manga := binds["1"].LibraryID
	ebooks := binds["2"].LibraryID
	if manga == 0 || ebooks == 0 || manga == ebooks {
		t.Fatalf("the fixture bound %d and %d; this test needs two distinct libraries", manga, ebooks)
	}
	// The Ebooks library is ALSO fed by a second instance that is not being
	// deleted, which is what makes the assertion below about the last source
	// rather than about the delete having stamped every library it touched.
	survivor := fixtureInstance(t, s, "kavita-two")
	secondSourceOn(t, s, ebooks, survivor, "9")

	if err := s.SoftDeleteServiceInstance(t.Context(), inst, sweepLater); err != nil {
		t.Fatalf("SoftDeleteServiceInstance: %v", err)
	}

	orphaned := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, manga)
	if !orphaned.Valid {
		t.Error("the library whose only source was on the deleted instance is not orphaned; it " +
			"renders with no sources and no reason, and no sweep will ever run to stamp it")
	}
	if orphaned.Valid && orphaned.String != FormatTime(sweepLater) {
		t.Errorf("orphaned_at = %q, want the instant of the delete %q",
			orphaned.String, FormatTime(sweepLater))
	}
	if fed := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, ebooks); fed.Valid {
		t.Errorf("a library another live instance still feeds was orphaned at %q", fed.String)
	}
	// ⚠️ library 0 is reserved and has no sources by construction, on
	// fedLibraries' rule; a delete that stamped it would mark the one library
	// that must always be there.
	if unfiled := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`,
		UnfiledLibraryID); unfiled.Valid {
		t.Errorf("the reserved Unfiled library was orphaned at %q", unfiled.String)
	}
}

// TestTheSweepDoesNotCountASourceOnASoftDeletedInstance is the same disagreement
// one step further out, and it is the shape that survived the fix above on its
// own: the deleted instance is not the LAST source, so nothing is stamped at the
// delete — and the retained row then keeps the surviving instance's sweep from
// ever stamping it either.
//
// The sweep's NOT EXISTS joined nothing, so any library_source row with
// missing_since IS NULL satisfied it, including one belonging to an instance the
// owner deleted and the screen no longer shows. The sweep and the read now use
// ONE definition of a source that counts.
func TestTheSweepDoesNotCountASourceOnASoftDeletedInstance(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	ebooks := binds["2"].LibraryID
	gone := fixtureInstance(t, s, "kavita-deleted")
	secondSourceOn(t, s, ebooks, gone, "9")

	// The owner deletes the SECOND instance. The library still has a live source
	// on the first, so it is not orphaned yet and must not be.
	if err := s.SoftDeleteServiceInstance(t.Context(), gone, sweepLater); err != nil {
		t.Fatalf("SoftDeleteServiceInstance: %v", err)
	}
	if o := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, ebooks); o.Valid {
		t.Fatalf("the library was orphaned at %q while a live instance still reported a source "+
			"of it; the assertion below would be vacuous", o.String)
	}

	// The FIRST instance now stops reporting container "2" as well. Every source
	// of this library is now either missing or on a deleted instance.
	later := sweepLater.Add(time.Hour)
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), oneContainer("1"), later)
	if err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	orphaned := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`, ebooks)
	if !orphaned.Valid {
		t.Error("the library has no source the Libraries screen can see and is not orphaned: a " +
			"row on a soft-deleted instance is keeping the sweep's NOT EXISTS satisfied")
	}
	if orphaned.Valid && orphaned.String != FormatTime(later) {
		t.Errorf("orphaned_at = %q, want the sweep's instant %q", orphaned.String, FormatTime(later))
	}
	// The Manga library's container was still reported, so it stays.
	if o := nullStr(t, s, `SELECT orphaned_at FROM library WHERE id = ?`,
		binds["1"].LibraryID); o.Valid {
		t.Errorf("the library whose container the read DID report was orphaned at %q", o.String)
	}
	if res.LibrariesOrphaned != 1 {
		t.Errorf("result = %+v, want one library orphaned", res)
	}
}

func TestSweepDoesNotStampADeclinedContainerAsMissing(t *testing.T) {
	// "UsArr has no kind for this" and "the upstream has stopped reporting this"
	// are different facts. bindPhase passes every container the upstream named,
	// declined ones included, and this asserts the distinction survives.
	s := newTestStore(t)
	inst, _ := sweepFixture(t, s)

	if _, err := s.SweepDeletions(t.Context(), inst, allSeen(), bothContainers(), sweepLater); err != nil {
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
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}}, bothContainers(), sweepLater); err != nil {
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

// TestAnUnidentifiedItemComingBackKeepsItsWorkAndSaysItCertifiedNothing is
// guard 1's third state — guardOneNoIncomingIdentity — and it is an INVERTED
// assertion rather than a deleted one.
//
// ⚠️ THIS TEST WAS NAMED TestAnUnidentifiedItemOnAReusedIDDoesNotRebindOntoThe-
// OldWork AND ASSERTED THE OPPOSITE OUTCOME ON THIS SAME INPUT: that the
// tombstoned link was hard-deleted and the id re-minted onto a fresh work. Its
// argument was that *"a wrong revival MERGES two books and is unrecoverable …
// a wrong fresh link SPLITS one book into two visible rows … a later merge puts
// it back"*. THE LATER MERGE DOES NOT EXIST — §6.4 defers `work_merge` out of
// v0.1 — so the split was permanent too, and it was the CERTAIN outcome on the
// one source v0.1 ships, where a book carries no identifier until an operator
// matches it. guardOneVerdict carries the full reversal. An inverted assertion
// records that a decision changed; a deleted one is silence.
//
// # What is still asserted, and it is the same fact in the other direction
//
// The vacuous equality is NOT re-admitted as evidence. Two unrelated
// unidentified books still hash identically, the guard still refuses to call
// that a match, and the four states are still separate — what changed is the
// ACTION the third one takes. So the subject of this test is no longer "which
// work does it land on" but "is the absence of evidence recorded", because
// the work id alone can no longer tell a certified revival from this one.
//
// # And the input is the worst case, deliberately
//
// Item 41 goes away and comes back as DIFFERENT CONTENT on the same id — a
// genuine id reuse, the case this outcome is now wrong about. It is the input
// the reversal accepted a cost on, so it is the input the test runs: the merge
// happens, and the row that says nothing was certified is what makes it
// discoverable afterwards.
func TestAnUnidentifiedItemComingBackKeepsItsWorkAndSaysItCertifiedNothing(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	original := readLink(t, s, inst, "series", "41").workID

	// The user's own overlay on that work — the thing the tombstone exists to
	// carry through a temporarily-empty upstream (§7.4), and the thing re-minting
	// stranded on a row no screen renders.
	if _, err := s.db.Writer().ExecContext(t.Context(),
		`INSERT INTO tag (id, namespace, value) VALUES (1, 'tag', 'favourite')`); err != nil {
		t.Fatalf("seed tag: %v", err)
	}
	if _, err := s.db.Writer().ExecContext(t.Context(),
		`INSERT INTO tag_assignment (tag_id, work_id, user_id, source) VALUES (1, ?, 0, 'user')`,
		original); err != nil {
		t.Fatalf("seed tag assignment: %v", err)
	}

	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}}, bothContainers(), sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if !readLink(t, s, inst, "series", "41").workDeleted.Valid {
		t.Fatal("41 was not tombstoned; the resurrection below would not be one")
	}

	// The upstream reuses id 41 for something else entirely. It carries no
	// external ids either — which is the point: BOTH sides hash to the empty
	// list, so there is nothing to compare in either direction.
	back := []CatalogueItem{item("41", "1", "comic", "Vinland Saga")}
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, back, sweepLater)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	// THE LINK IS REVIVED ONTO ITS OWN WORK, asserted by identity.
	got := readLink(t, s, inst, "series", "41").workID
	if got != original {
		t.Errorf("the link came back on work %d, want the original %d: it was re-minted, "+
			"which strands the user's rows on a work nothing reaps", got, original)
	}
	st := readLink(t, s, inst, "series", "41")
	if st.linkDeleted.Valid || st.workDeleted.Valid {
		t.Errorf("the tombstone was not cleared: link=%v work=%v", st.linkDeleted, st.workDeleted)
	}
	// AND THE OWNED OVERLAY IS STILL ON A LIVE WORK. That is the whole of what
	// the reversal buys, so it is asserted on the row rather than inferred from
	// the work id.
	if n := count(t, s, `SELECT COUNT(*) FROM tag_assignment ta JOIN work w ON w.id = ta.work_id
		 WHERE ta.work_id = ? AND w.deleted_at IS NULL`, original); n != 1 {
		t.Errorf("%d live tag assignments on work %d, want 1: the user's tag is stranded "+
			"on a tombstoned row", n, original)
	}
	// NOTHING WAS DUPLICATED. A hard-delete-and-re-mint leaves the old work
	// tombstoned beside a NEW one, so the fixture's three works become four.
	if n := count(t, s, `SELECT COUNT(*) FROM work`); n != 3 {
		t.Errorf("%d work rows, want the fixture's 3: the id was re-minted after all "+
			"and the abandoned row is still there", n)
	}

	// ⚠️ AND THE COST IS ASSERTED TOO, RATHER THAN LEFT OUT OF THE RECORD: this
	// input WAS a genuine id reuse, and the work now carries the new content
	// under the old row's tags. That is the merge the reversal accepted.
	if title := nullStr(t, s, `SELECT title FROM work WHERE id = ?`, original).String; title != "Vinland Saga" {
		t.Errorf("work %d is titled %q; this test's premise is that the revival "+
			"rebinds new content onto the old row", original, title)
	}

	// THE RECORD IS THE ONLY THING SEPARATING THIS FROM A CERTIFIED MATCH, so it
	// is asserted on the kind, the verdict inside it, and the counter — one of
	// the three passing alone would not tell them apart.
	if res.RevivedWithoutIdentity != 1 {
		t.Errorf("RevivedWithoutIdentity = %d, want 1: the revival was indistinguishable "+
			"from a certified match (%+v)", res.RevivedWithoutIdentity, res)
	}
	if res.IDsReused != 0 {
		t.Errorf("IDsReused = %d, want 0: nothing was re-minted, so nothing may be "+
			"reported as an id reuse", res.IDsReused)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report
		 WHERE service_instance_id = ? AND kind = ? AND detail LIKE '%"verdict":"no_incoming_identity"%'`,
		inst, SyncReportRevivedWithoutIdentity); n != 1 {
		t.Errorf("%d revived_without_identity rows carry the no_incoming_identity verdict, want 1 — "+
			"without it a revival on no evidence reads exactly like one on a matching hash", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportIDReused); n != 0 {
		t.Errorf("%d id_reused rows for a link that was kept", n)
	}
}

// TestACertifiedRevivalIsNotRecordedAsAnUncertifiedOne is the other side of the
// separation, and without it the row above is decorative: an implementation that
// wrote SyncReportRevivedWithoutIdentity for EVERY revival would satisfy every
// assertion in the test above while certifying nothing.
func TestACertifiedRevivalIsNotRecordedAsAnUncertifiedOne(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)

	// 77 carries a hardcover id, so its revival IS certified by the hash.
	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}}, bothContainers(), sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if !readLink(t, s, inst, "series", "77").workDeleted.Valid {
		t.Fatal("77 was not tombstoned; the revival below would not be one")
	}

	back := []CatalogueItem{
		item("77", "2", "book", "The Hobbit",
			ExternalIdentifier{Source: "hardcover_book", Value: "445", Confidence: 1.0}),
	}
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, back, sweepLater)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	if res.RevivedWithoutIdentity != 0 {
		t.Errorf("RevivedWithoutIdentity = %d for a revival the identity hash certified: "+
			"the two outcomes are recorded identically and the record says nothing", res.RevivedWithoutIdentity)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE service_instance_id = ? AND kind = ?`,
		inst, SyncReportRevivedWithoutIdentity); n != 0 {
		t.Errorf("%d revived_without_identity rows for a certified match", n)
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

	// BOTH OUTCOME PREDICATES ARE ASSERTED FOR EVERY STATE, not just the one
	// each state is about. Four states now collapse into THREE outcomes, and a
	// table that checked only `fires` would be green again the day the third one
	// was folded back into "revive silently" — which is the exact edit
	// guardOneVerdict was introduced to make loud.
	for _, c := range []struct {
		name    string
		hash    sql.NullString
		it      CatalogueItem
		want    guardOneVerdict
		fires   bool
		records bool
	}{
		{"nothing stored", sql.NullString{}, withIDs, guardOneIdentityUnknown, false, false},
		{
			"stored, incoming carries none",
			sql.NullString{String: "abc", Valid: true},
			bare, guardOneNoIncomingIdentity, false, true,
		},
		{
			"both present, differ",
			sql.NullString{String: "abc", Valid: true},
			withIDs, guardOneIdentityChanged, true, false,
		},
		{
			"both present, agree",
			sql.NullString{String: withIDs.identityHash(), Valid: true},
			withIDs, guardOneIdentityMatches, false, false,
		},
		// THE INTERSECTION, ruled at the seam: nothing stored AND nothing
		// incoming answers "unknown", because that is the older and narrower
		// rule. Unreachable today — no shipped writer leaves the column NULL.
		{"nothing stored, incoming carries none", sql.NullString{}, bare, guardOneIdentityUnknown, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := guardOne(c.hash, c.it)
			if got != c.want {
				t.Errorf("guardOne = %s, want %s", got, c.want)
			}
			if got.firesGuard() != c.fires {
				t.Errorf("%s.firesGuard() = %v, want %v", got, got.firesGuard(), c.fires)
			}
			if got.recordsRevival() != c.records {
				t.Errorf("%s.recordsRevival() = %v, want %v", got, got.recordsRevival(), c.records)
			}
			if got.firesGuard() && got.recordsRevival() {
				t.Errorf("%s both fires and records a revival; the outcomes are meant to "+
					"be exclusive and a caller reading them in sequence would do both", got)
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
	//
	// ⚠️ THE ASSERTION IS ON THE VERDICT AND NOT ON firesGuard(), and it moved
	// there when the ruling reversed. It used to read `!v.firesGuard()` — that
	// the guard re-minted — and re-minting is no longer the outcome. What must
	// still hold, and is what this line asserts, is that the vacuous equality is
	// not read as guardOneIdentityMatches: the two books are not certified as the
	// same book, and the difference is recorded rather than silently collapsed.
	v := guardOne(sql.NullString{String: other.identityHash(), Valid: true}, bare)
	if v != guardOneNoIncomingIdentity {
		t.Errorf("guardOne returned %s for two unrelated unidentified books whose hashes are "+
			"equal; the equality is vacuous and the guard treated it as evidence", v)
	}
	if !v.recordsRevival() {
		t.Errorf("%s revives without recording it, so a revival on a vacuous equality is "+
			"indistinguishable from one the hash certified", v)
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
		bothContainers(), sweepLater); err != nil {
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

// TestTheSweepLeavesContainerKindsItDoesNotOwn is absentSources' `container_kind
// = 'remote_library'` predicate, which had no fixture and therefore no guard:
// deleting the clause was invisible to every test, because nothing in the corpus
// had a row of another kind.
//
// The kinds are migration 0005's five, and the sweep is fed by a CATALOGUE read,
// which produces exactly one of them. A root_folder row belongs to an *Arr sync
// that reports a different container list; stamping it missing because a
// catalogue read did not mention it would mark an *Arr's folders gone every time
// a Kavita import ran.
func TestTheSweepLeavesContainerKindsItDoesNotOwn(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)

	// One row of a kind this pass does not own, on the same instance and the same
	// library, with a container_ref the read will not mention either.
	if _, err := s.db.Writer().ExecContext(t.Context(), `
		INSERT INTO library_source
		  (library_id, service_instance_id, container_kind, container_ref, container_identity)
		VALUES (?, ?, 'root_folder', '/media/books', 'Books')`,
		binds["2"].LibraryID, inst); err != nil {
		t.Fatalf("seed root_folder source: %v", err)
	}

	// A read that reports NEITHER container: both remote_library rows are stamped,
	// which is what makes the assertion below about the KIND rather than about
	// the sweep having done nothing.
	res, err := s.SweepDeletions(t.Context(), inst, allSeen(), SweepScope{}, sweepLater)
	if err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	// ⚠️ THE EFFECT FIRST AND WITH Errorf, NOT THE COUNT WITH Fatalf. A premise
	// check that runs first and aborts reports "the number moved" for a tree
	// where the filter was deleted, which is an assertion on a count rather than
	// on the row the filter exists to protect — and the row is the whole subject.
	if m := nullStr(t, s, `SELECT missing_since FROM library_source
		 WHERE service_instance_id = ? AND container_kind = 'root_folder'`, inst); m.Valid {
		t.Errorf("a root_folder source was stamped missing at %q by a CATALOGUE read that "+
			"never reports root folders", m.String)
	}
	if res.SourcesMissing < 2 {
		t.Errorf("SourcesMissing = %d, want at least the fixture's 2 remote_library rows; "+
			"the assertion above passed over a sweep that stamped nothing (%+v)",
			res.SourcesMissing, res)
	}
}

// pinLink sets §5.3's northbound pin on one link. It is the OWNED BIT the
// resurrection tests below assert on, chosen deliberately: applyOneItem's step-7
// upsert does not name `is_northbound_canonical` in its ON CONFLICT list and no
// Go in the tree writes the column at all, so it survives a link that is REVIVED
// in place and is lost with a link that is HARD-DELETED and re-created. That is
// the difference between guard 1's two outcomes stated as something the owner
// can lose, rather than as the name of a branch.
func pinLink(t *testing.T, s *Store, inst int64, remoteKind, remoteID string) {
	t.Helper()
	res, err := s.db.Writer().ExecContext(t.Context(), `
		UPDATE service_item_link SET is_northbound_canonical = 1
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?`,
		inst, remoteKind, remoteID)
	if err != nil {
		t.Fatalf("pin link %s/%s: %v", remoteKind, remoteID, err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("pin link %s/%s touched %d rows (err %v); the test has nothing to lose",
			remoteKind, remoteID, n, err)
	}
}

func linkIsPinned(t *testing.T, s *Store, inst int64, remoteKind, remoteID string) bool {
	t.Helper()
	var pinned int
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT is_northbound_canonical FROM service_item_link
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?`,
		inst, remoteKind, remoteID).Scan(&pinned); err != nil {
		t.Fatalf("read pin on %s/%s: %v", remoteKind, remoteID, err)
	}
	return pinned == 1
}

// TestTheStoredIdentityHashMovesOnceFromEmptyAndNeverAgain is the rule guard 1's
// comparison rests on, at the column.
//
// ⚠️ THE RULE IS NARROWED AND THIS TEST IS THE NARROWING. It used to read
// "written at first sight and NEVER overwritten", and the fixture it asserted
// that on — link 41, first seen carrying no identifiers at all — is the exact
// case the narrowing exists for. A hash comparison cannot tell an identity
// ARRIVING from an identity CHANGING, so the unnarrowed rule froze the empty-list
// hash of every item first seen before anybody matched it, and read the ordinary
// match as a reused id ever after. Empty → present is allowed; present → anything
// is not.
//
// ⚠️ IT IS ASSERTED IN THREE DIRECTIONS, because the column is only meaningful
// relative to the one beside it and to the transition it refuses. `remote_hash` —
// the synced subset — IS refreshed on every upsert, and a test that only pinned
// the identity hash would stay green if a later edit froze both.
func TestTheStoredIdentityHashMovesOnceFromEmptyAndNeverAgain(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)

	read := func(remoteID string) (identity, hash string) {
		t.Helper()
		if err := s.db.Read().QueryRowContext(t.Context(), `
			SELECT remote_identity_hash, remote_hash FROM service_item_link
			 WHERE service_instance_id = ? AND remote_kind = 'series' AND remote_id = ?`,
			inst, remoteID).Scan(&identity, &hash); err != nil {
			t.Fatalf("read link %s: %v", remoteID, err)
		}
		return identity, hash
	}
	apply := func(it CatalogueItem) {
		t.Helper()
		if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
			[]CatalogueItem{it}, sweepLater); err != nil {
			t.Fatalf("ApplyCatalogueBatch: %v", err)
		}
	}

	// ── EMPTY → PRESENT, THE ONE TRANSITION ALLOWED. Link 41 was first seen with
	// no identifiers; the operator matches the book upstream, and its title
	// changes too so the synced subset really is different.
	emptyIdentity, firstHash := read("41")
	if emptyIdentity != emptyIdentityHash {
		t.Fatalf("link 41's stored identity is %q, want the empty-list hash %q; the fixture "+
			"no longer starts unidentified and this test is about a transition it cannot make",
			emptyIdentity, emptyIdentityHash)
	}
	matched := item("41", "1", "comic", "Frieren: Beyond Journey's End",
		ExternalIdentifier{Source: "hardcover_book", Value: "1234", Confidence: 1.0})
	apply(matched)
	gotIdentity, secondHash := read("41")
	if gotIdentity != matched.identityHash() {
		t.Errorf("remote_identity_hash stayed %q after the item was matched, want %q. An item "+
			"identified after UsArr first saw it is frozen on the hash of NOTHING, and guard 1 "+
			"reads its next resurrection as a reused id", gotIdentity, matched.identityHash())
	}
	if secondHash == firstHash {
		t.Errorf("remote_hash did not move (%q) although the item's synced subset changed; "+
			"the two columns have different refresh rules and this one is the drift detector",
			secondHash)
	}

	// ── PRESENT → ANYTHING, REFUSED. Link 77 carried an identifier at first
	// sight. The upstream now reports a different one for the same id, which is
	// the event the freeze exists for: an established identity must not follow it.
	establishedIdentity, firstHash77 := read("77")
	repointed := item("77", "2", "book", "The Hobbit: An Unexpected Retitling",
		ExternalIdentifier{Source: "hardcover_book", Value: "999", Confidence: 1.0})
	apply(repointed)
	gotIdentity77, secondHash77 := read("77")
	if gotIdentity77 != establishedIdentity {
		t.Errorf("remote_identity_hash moved from %q to %q on an item that was ALREADY "+
			"identified. Guard 1 compares against it to tell a reused id from the same item "+
			"coming back; a column that tracks the latest sighting always compares equal and "+
			"certifies nothing", establishedIdentity, gotIdentity77)
	}
	if gotIdentity77 == repointed.identityHash() {
		t.Errorf("remote_identity_hash is the INCOMING item's hash; for an item that already " +
			"had an identity it is meant to be the hash recorded at first sight")
	}
	if secondHash77 == firstHash77 {
		t.Errorf("remote_hash did not move (%q) although the item's synced subset changed",
			secondHash77)
	}
}

// TestAnIdentityMatchedAfterFirstSightSurvivesItsOwnResurrection is the
// narrowing's whole point, asserted by EFFECT: the item is revived in place and
// keeps everything the owner put on it.
//
// The item is first seen unidentified — which is what a book with no primary file
// reads as, and the ordinary state of anything the operator has not matched yet.
// It is matched. It goes away upstream and is tombstoned. It comes back carrying
// the identifiers it has carried ever since.
//
// Under the unnarrowed rule the stored identity was still the hash of NOTHING, so
// guard 1 read `identity_changed`, HARD-DELETED the link and filed an `id_reused`
// row for a reuse that never happened — on the ordinary case rather than on the
// attack. The pin is what that costs, and it is asserted rather than the branch
// name.
func TestAnIdentityMatchedAfterFirstSightSurvivesItsOwnResurrection(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)
	work := readLink(t, s, inst, "series", "41").workID

	matched := item("41", "1", "comic", "Frieren",
		ExternalIdentifier{Source: "hardcover_book", Value: "1234", Confidence: 1.0})
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{matched}, sweepLater); err != nil {
		t.Fatalf("the matching import: %v", err)
	}

	// 41 stops being reported. The sweep tombstones the link and the work with it.
	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "42"}, {RemoteKind: "series", RemoteID: "77"}},
		bothContainers(), sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if !readLink(t, s, inst, "series", "41").linkDeleted.Valid {
		t.Fatal("the sweep did not tombstone link 41, so nothing below is on the resurrection path")
	}
	pinLink(t, s, inst, "series", "41")

	// It comes back, unchanged, carrying the identifiers it was matched on.
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{matched}, sweepLater)
	if err != nil {
		t.Fatalf("the resurrecting import: %v", err)
	}

	if !linkIsPinned(t, s, inst, "series", "41") {
		t.Error("the owner's northbound pin is gone: the link was hard-deleted and re-created " +
			"rather than revived, because the identity recorded at first sight was never allowed " +
			"to become the identity the item was matched on")
	}
	after := readLink(t, s, inst, "series", "41")
	if after.workID != work {
		t.Errorf("the item came back as work %d, want %d: it was re-minted, and every tag, "+
			"request and northbound id naming %d now names nothing", after.workID, work, work)
	}
	if after.linkDeleted.Valid || after.workDeleted.Valid {
		t.Errorf("the item came back and is still tombstoned: link %v, work %v",
			after.linkDeleted, after.workDeleted)
	}
	if res.IDsReused != 0 {
		t.Errorf("IDsReused = %d: an id nobody reused was reported as reused (%+v)",
			res.IDsReused, res)
	}
}

// TestAnEstablishedIdentityRepointedUpstreamStillFiresGuardOne is the other half
// of the drill, and it is what the narrowing does NOT give up.
//
// Link 77 was identified at first sight. The upstream repoints that identifier
// while the link is live — an id reassigned, or an upstream somebody has got at —
// and the item is then tombstoned and comes back wearing the new identity. The
// stored hash must still be the one recorded at first sight, so what comes back
// does not match it and the destructive branch fires exactly as before.
//
// An unconditional overwrite in step 7's ON CONFLICT list would have made the two
// compare equal, and this is the assertion that says so: the pin SURVIVES a
// revival, so a surviving pin here means the guard did not fire.
func TestAnEstablishedIdentityRepointedUpstreamStillFiresGuardOne(t *testing.T) {
	s := newTestStore(t)
	inst, binds := sweepFixture(t, s)

	repointed := item("77", "2", "book", "The Hobbit",
		ExternalIdentifier{Source: "hardcover_book", Value: "999", Confidence: 1.0})
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{repointed}, sweepLater); err != nil {
		t.Fatalf("the repointing import: %v", err)
	}

	if _, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "41"}, {RemoteKind: "series", RemoteID: "42"}},
		bothContainers(), sweepLater); err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if !readLink(t, s, inst, "series", "77").linkDeleted.Valid {
		t.Fatal("the sweep did not tombstone link 77, so nothing below is on the resurrection path")
	}
	pinLink(t, s, inst, "series", "77")

	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{repointed}, sweepLater)
	if err != nil {
		t.Fatalf("the resurrecting import: %v", err)
	}

	if linkIsPinned(t, s, inst, "series", "77") {
		t.Error("the link was revived in place with its pin intact, so the stored identity had " +
			"followed the upstream's repoint and guard 1 compared the new identity against " +
			"itself. An established identity must not move")
	}
	if res.IDsReused != 1 {
		t.Errorf("IDsReused = %d, want 1: the identity recorded at first sight is not the one "+
			"that came back and the guard did not fire (%+v)", res.IDsReused, res)
	}
}
