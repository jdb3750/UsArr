package store

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// seedGrab writes one provenance row the way the grab path writes it: a title, a
// grab time, and the acquisition state on the row itself.
func seedGrab(t *testing.T, s *Store, userID int64, title string, at time.Time, state string) int64 {
	t.Helper()
	id, err := s.InsertProvenance(t.Context(), Provenance{
		UserID:            userID,
		Protocol:          "torrent",
		IndexerName:       "an-indexer",
		IndexerCategories: `[2040,2045]`,
		DownloadID:        "hash-" + title,
		ReleaseTitle:      title,
		SizeBytes:         sql.NullInt64{Int64: 8 << 30, Valid: true},
		GrabbedAt:         sql.NullString{String: FormatTime(at), Valid: true},
		SourceSystem:      "prowlarr",
		AcquisitionState:  state,
	})
	if err != nil {
		t.Fatalf("seed provenance %q: %v", title, err)
	}
	return id
}

func titlesOf(rows []Provenance) []string {
	out := make([]string, 0, len(rows))
	for _, p := range rows {
		out = append(out, p.ReleaseTitle)
	}
	return out
}

func TestListRecentProvenanceIsNewestFirst(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	owner := OwnerScope(1)

	seedGrab(t, s, 1, "oldest", testNow.Add(-2*time.Hour), AcquisitionConfirmed)
	seedGrab(t, s, 1, "newest", testNow, AcquisitionConfirmed)
	seedGrab(t, s, 1, "middle", testNow.Add(-time.Hour), AcquisitionUnconfirmed)

	got, err := s.ListRecentProvenance(ctx, owner, 0)
	if err != nil {
		t.Fatalf("ListRecentProvenance: %v", err)
	}
	want := []string{"newest", "middle", "oldest"}
	if strings.Join(titlesOf(got), ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", titlesOf(got), want)
	}

	// The discriminator has to survive the read. Without it a reader that starts
	// from provenance — which is all this read is — cannot tell an acquisition
	// UsArr confirmed from one it merely dispatched, and the row silently reads
	// as a success.
	if got[1].AcquisitionState != AcquisitionUnconfirmed {
		t.Errorf("acquisition_state = %q, want %q", got[1].AcquisitionState, AcquisitionUnconfirmed)
	}
	if got[0].AcquisitionState != AcquisitionConfirmed {
		t.Errorf("acquisition_state = %q, want %q", got[0].AcquisitionState, AcquisitionConfirmed)
	}

	// Fields the block renders must arrive populated, not merely non-erroring.
	if got[0].IndexerName != "an-indexer" || !got[0].SizeBytes.Valid || got[0].DownloadID == "" {
		t.Errorf("row is missing fields §17.5 renders: %+v", got[0])
	}
	if got[0].IndexerCategories != `[2040,2045]` {
		t.Errorf("categories = %q, want the raw Newznab array", got[0].IndexerCategories)
	}
}

// Two grabs inside the same second must still come out in a stable, correct
// order. grabbed_at is stored at one-second resolution, so id is the tiebreak —
// the same reason ix_prov_user_grabbed trails id after grabbed_at.
func TestListRecentProvenanceBreaksTiesById(t *testing.T) {
	s := newTestStore(t)
	seedGrab(t, s, 1, "first", testNow, AcquisitionConfirmed)
	seedGrab(t, s, 1, "second", testNow, AcquisitionConfirmed)

	got, err := s.ListRecentProvenance(t.Context(), OwnerScope(1), 0)
	if err != nil {
		t.Fatalf("ListRecentProvenance: %v", err)
	}
	if len(got) != 2 || got[0].ReleaseTitle != "second" {
		t.Fatalf("order = %v, want the later-inserted row first", titlesOf(got))
	}
}

// The access scope is APPLIED, not merely accepted. This project has already
// shipped reads that took a scope parameter and never put it in the WHERE
// clause, which is indistinguishable from having no scope at all — so the
// assertion is that another user's row is absent, not that the parameter exists.
func TestListRecentProvenanceAppliesScope(t *testing.T) {
	s := newTestStore(t)

	seedGrab(t, s, 1, "mine", testNow, AcquisitionConfirmed)
	seedGrab(t, s, 2, "someone-elses", testNow.Add(time.Minute), AcquisitionConfirmed)
	// The shared/system sentinel: every row migration 0002 backfilled carries
	// it, and it is the owner's own pre-attribution history. Filtering it out
	// would hide the user's past from them.
	seedGrab(t, s, SystemUserID, "pre-attribution", testNow.Add(-time.Minute), AcquisitionConfirmed)

	got, err := s.ListRecentProvenance(t.Context(), OwnerScope(1), 0)
	if err != nil {
		t.Fatalf("ListRecentProvenance: %v", err)
	}
	for _, p := range got {
		if p.ReleaseTitle == "someone-elses" {
			t.Fatalf("another user's acquisition is readable: %v", titlesOf(got))
		}
	}
	want := []string{"mine", "pre-attribution"}
	if strings.Join(titlesOf(got), ",") != strings.Join(want, ",") {
		t.Fatalf("rows = %v, want %v", titlesOf(got), want)
	}

	// And a scope that can see nothing sees nothing. OwnerScope is the only
	// scope v0.1 constructs, so this is the arm that has never run in
	// production and is therefore the one worth firing deliberately.
	got, err = s.ListRecentProvenance(t.Context(), Scope{UserID: 999}, 0)
	if err != nil {
		t.Fatalf("ListRecentProvenance: %v", err)
	}
	if len(got) != 1 || got[0].ReleaseTitle != "pre-attribution" {
		t.Fatalf("unknown user sees %v, want only the shared sentinel row", titlesOf(got))
	}
}

func TestListRecentProvenanceBoundsTheLimit(t *testing.T) {
	s := newTestStore(t)
	for i := range RecentProvenanceDefaultLimit + 5 {
		seedGrab(t, s, 1, string(rune('a'+i)), testNow.Add(time.Duration(i)*time.Minute), AcquisitionConfirmed)
	}
	owner := OwnerScope(1)

	got, err := s.ListRecentProvenance(t.Context(), owner, 0)
	if err != nil {
		t.Fatalf("ListRecentProvenance: %v", err)
	}
	if len(got) != RecentProvenanceDefaultLimit {
		t.Errorf("limit 0 returned %d rows, want the §17.5 default of %d",
			len(got), RecentProvenanceDefaultLimit)
	}

	got, err = s.ListRecentProvenance(t.Context(), owner, 3)
	if err != nil {
		t.Fatalf("ListRecentProvenance: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("limit 3 returned %d rows", len(got))
	}

	// The cap is the only thing bounding the sort, so a caller must not be able
	// to lift it. Asserted through the rendered SQL rather than the row count,
	// because a test database cannot hold enough rows to tell 200 from 10^9.
	query, args := recentProvenanceSQL(owner, 1<<30)
	_ = query
	if got := args[len(args)-1]; got != RecentProvenanceMaxLimit {
		t.Errorf("an oversized limit rendered as %v, want it clamped to %d", got, RecentProvenanceMaxLimit)
	}
}

// The plan for the statement that actually ships.
//
// WHAT IS ASSERTED. `SEARCH … USING INDEX ix_prov_user_grabbed`, and NOT
// `COVERING INDEX`: docs/reference/schema.md §1 and §7 are explicit that this
// index SERVES the Recent-grabs read rather than covering it, because the SELECT
// list carries release_title, indexer_name and size_bytes. A test that accepted
// a covering plan would be pinning a different query from the one that ships,
// and would go green on a SELECT list that no longer renders the block.
//
// WHAT IS NOT ASSERTED, AND WHY. The temp b-tree. The scoped predicate is
// `user_id IN (0, :uid)` and SQLite cannot supply ORDER BY from an index whose
// LEADING column is constrained by IN. internal/db's
// TestScopedProvenanceOrderNeedsASort owns that fact and fails if it ever stops
// being true; repeating the assertion here would only duplicate it. The index
// still restricts the sort to the readable rows, bounded by the LIMIT.
func TestListRecentProvenanceUsesTheServingIndex(t *testing.T) {
	s := newTestStore(t)
	query, args := recentProvenanceSQL(OwnerScope(1), RecentProvenanceDefaultLimit)

	plan, err := db.QueryPlan(t.Context(), s.db.Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")

	if faults := servingIndexFaults(joined); len(faults) > 0 {
		t.Errorf("the recent-grabs plan is wrong:\n  plan: %s\n  %s\n"+
			"schema.md §7 pins SEARCH … USING INDEX ix_prov_user_grabbed for this read. "+
			"If the SELECT list genuinely stopped rendering release_title, indexer_name "+
			"and size_bytes, update schema.md in the same change.",
			joined, strings.Join(faults, "\n  "))
	}
}

// servingIndexFaults is the assertion itself, lifted out of the test so its
// FAILURE BRANCH can be run on purpose. docs/DEVELOPMENT.md §11 rule 3: a
// guard's failure path is code, it only executes when something is already
// wrong, and untested code does not work.
func servingIndexFaults(plan string) []string {
	var faults []string
	if !strings.Contains(plan, "SEARCH") {
		faults = append(faults, "not a SEARCH")
	}
	if strings.Contains(plan, "SCAN provenance") {
		faults = append(faults, "degraded to a table scan")
	}
	if !planHas(plan, "USING INDEX ix_prov_user_grabbed") {
		faults = append(faults, "does not use ix_prov_user_grabbed")
	}
	// A covering plan is a DIFFERENT query from the one that ships. Accepting it
	// would let the test stay green on a SELECT list that no longer renders the
	// block — schema.md §1 and §7 both spell this out for exactly this index.
	if strings.Contains(plan, "COVERING INDEX") {
		faults = append(faults, "is a COVERING INDEX, so this is not the shipped SELECT list")
	}
	return faults
}

// Firing the guard deliberately. A plan over the same table with no indexed
// predicate must produce every fault the real assertion looks for; a guard that
// has never been triggered is indistinguishable from no guard.
func TestRecentProvenancePlanGuardFiresOnAScan(t *testing.T) {
	s := newTestStore(t)

	plan, err := db.QueryPlan(t.Context(), s.db.Read(),
		`SELECT id, release_title FROM provenance ORDER BY release_title LIMIT 10`)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")

	faults := servingIndexFaults(joined)
	if len(faults) == 0 {
		t.Fatalf("the guard passed a plan it exists to reject:\n  %s", joined)
	}
	if !strings.Contains(strings.Join(faults, " "), "table scan") {
		t.Errorf("the guard fired but not on the scan:\n  plan: %s\n  faults: %v", joined, faults)
	}

	// And a covering plan is rejected too — the arm the coordinator's rule is
	// about, which no query in this file would otherwise reach.
	covering, err := db.QueryPlan(t.Context(), s.db.Read(),
		`SELECT id FROM provenance WHERE user_id = ? ORDER BY grabbed_at DESC, id DESC LIMIT 10`, 1)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joinedCovering := strings.Join(covering, " | ")
	if !strings.Contains(joinedCovering, "COVERING INDEX") {
		t.Skipf("SQLite no longer plans the narrow SELECT list as covering, so the "+
			"COVERING arm cannot be fired here: %s", joinedCovering)
	}
	if len(servingIndexFaults(joinedCovering)) == 0 {
		t.Errorf("the guard accepted a COVERING INDEX plan:\n  %s", joinedCovering)
	}
}
