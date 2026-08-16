package store

import (
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// ListNotSentGrabs is the definite-failure arm of ARCHITECTURE.md §17.5's
// Recent-grabs union. What it must NOT return is as load-bearing as what it
// must: a `warn` row already has a provenance row behind it and is served by the
// other arm, so returning it here renders one grab twice.
func TestListNotSentGrabsReadsOnlyDefiniteFailures(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	actor, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local", IsOwner: true})
	if err != nil {
		t.Fatal(err)
	}

	appendGrab(t, s, actor, testNow, AuditActionGrab, AuditResultFail,
		`{"outcome":"not_sent","release_title":"Never.Sent-GROUP"}`)
	appendGrab(t, s, actor, testNow.Add(time.Second), AuditActionGrab, AuditResultWarn,
		`{"outcome":"sent_unknown","release_title":"Ambiguous-GROUP"}`)
	appendGrab(t, s, actor, testNow.Add(2*time.Second), AuditActionGrab, AuditResultOK,
		`{"outcome":"sent_confirmed","release_title":"Confirmed-GROUP"}`)
	appendGrab(t, s, actor, testNow.Add(3*time.Second), "user.login", AuditResultFail, `{}`)

	got, err := s.ListNotSentGrabs(ctx, OwnerScope(actor), 0)
	if err != nil {
		t.Fatalf("ListNotSentGrabs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want only the one definite failure: %+v", len(got), got)
	}
	if !strings.Contains(got[0].MetadataJSON, "Never.Sent-GROUP") {
		t.Errorf("the wrong row came back: %q", got[0].MetadataJSON)
	}
	// Named individually, because each is a different way to get the union
	// wrong: a warn row duplicates a provenance row, an ok row duplicates it
	// too, and a login failure is not a grab at all.
	for _, unwanted := range []string{"Ambiguous-GROUP", "Confirmed-GROUP"} {
		for _, e := range got {
			if strings.Contains(e.MetadataJSON, unwanted) {
				t.Errorf("a handed-over grab (%s) is in the not-sent arm; the union will "+
					"render it twice: %q", unwanted, e.MetadataJSON)
			}
		}
	}
}

// Newest first, and one user's failures stay theirs.
func TestListNotSentGrabsIsScopedAndNewestFirst(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	alice, err := s.CreateUser(ctx, User{Username: "alice", AuthSource: "local", IsOwner: true})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.CreateUser(ctx, User{Username: "bob", AuthSource: "local"})
	if err != nil {
		t.Fatal(err)
	}

	appendGrab(t, s, alice, testNow.Add(-time.Hour), AuditActionGrab, AuditResultFail, `{"n":"older"}`)
	appendGrab(t, s, alice, testNow, AuditActionGrab, AuditResultFail, `{"n":"newer"}`)
	appendGrab(t, s, bob, testNow.Add(time.Hour), AuditActionGrab, AuditResultFail,
		`{"release_title":"Bob.Private.Release-GROUP"}`)

	got, err := s.ListNotSentGrabs(ctx, OwnerScope(alice), 0)
	if err != nil {
		t.Fatalf("ListNotSentGrabs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("alice sees %d rows, want her own 2: %+v", len(got), got)
	}
	if !strings.Contains(got[0].MetadataJSON, "newer") {
		t.Errorf("order = %q first, want newest first", got[0].MetadataJSON)
	}
	for _, e := range got {
		if strings.Contains(e.MetadataJSON, "Bob.Private.Release") {
			t.Fatalf("bob's release title is readable through alice's not-sent arm: %q", e.MetadataJSON)
		}
	}
}

// The two arms of one union must be bounded identically, or the merge is skewed
// toward whichever arm was allowed more rows.
func TestNotSentGrabsQueryClampsTheLimitLikeTheOtherArm(t *testing.T) {
	if got := NotSentGrabsQuery(0).Limit; got != RecentProvenanceDefaultLimit {
		t.Errorf("limit 0 rendered as %d, want the §17.5 default of %d", got, RecentProvenanceDefaultLimit)
	}
	if got := NotSentGrabsQuery(1 << 30).Limit; got != RecentProvenanceMaxLimit {
		t.Errorf("an oversized limit rendered as %d, want it clamped to %d", got, RecentProvenanceMaxLimit)
	}
	if got := NotSentGrabsQuery(3).Limit; got != 3 {
		t.Errorf("limit 3 rendered as %d", got)
	}
	// The filter itself, asserted rather than assumed: `warn` here is the bug
	// that renders a sent-unknown grab twice.
	q := NotSentGrabsQuery(10)
	if len(q.Results) != 1 || q.Results[0] != AuditResultFail {
		t.Errorf("results = %v, want only %q — a warn row is served by the provenance arm",
			q.Results, AuditResultFail)
	}
	if len(q.Actions) != 1 || q.Actions[0] != AuditActionGrab {
		t.Errorf("actions = %v, want only %q", q.Actions, AuditActionGrab)
	}
}

// notSentPlanFaults is the plan assertion itself, lifted out of the test so its
// FAILURE BRANCH can be executed on purpose. docs/DEVELOPMENT.md §11 rule 3: a
// guard whose failure path has never run is indistinguishable from no guard.
func notSentPlanFaults(plan string) []string {
	var faults []string
	if !strings.Contains(plan, "SEARCH") {
		faults = append(faults, "not a SEARCH")
	}
	if strings.Contains(plan, "SCAN audit_log") {
		faults = append(faults, "degraded to a scan of a table that grows forever by design")
	}
	if !strings.Contains(plan, "USING INDEX ix_audit_actor_action") {
		faults = append(faults, "does not use ix_audit_actor_action")
	}
	// A covering plan is a DIFFERENT query from the one that ships. This read
	// selects metadata_json, which carries the release title the block renders,
	// so a covering plan means the SELECT list shrank to columns the index
	// happens to hold — and the test would then be green on a query that can no
	// longer render the row. schema.md §1 says assert SEARCH … USING INDEX and
	// never COVERING, for exactly this reason.
	if strings.Contains(plan, "COVERING INDEX") {
		faults = append(faults, "is a COVERING INDEX, so this is not the shipped SELECT list")
	}
	return faults
}

// The plan for the statement ListNotSentGrabs actually issues — EXPLAINed
// through the same NotSentGrabsQuery and auditListSQL the shipping call uses,
// not a lookalike pasted into a test.
func TestNotSentGrabsUsesTheActorActionIndex(t *testing.T) {
	s := newTestStore(t)
	query, args := auditListSQL(OwnerScope(1), NotSentGrabsQuery(RecentProvenanceDefaultLimit))

	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")

	if faults := notSentPlanFaults(joined); len(faults) > 0 {
		t.Errorf("the not-sent arm's plan is wrong:\n  plan: %s\n  %s\n"+
			"Migration 0002 added ix_audit_actor_action for this read. If the SELECT list "+
			"genuinely stopped carrying metadata_json, update schema.md in the same change.",
			joined, strings.Join(faults, "\n  "))
	}
}

// Firing the guard deliberately, on both arms the coordinator's rule names.
func TestNotSentGrabsPlanGuardFires(t *testing.T) {
	s := newTestStore(t)

	// A real scan: no indexed predicate at all on the same table.
	scan, err := db.QueryPlan(t.Context(), s.DB().Read(),
		`SELECT id, metadata_json FROM audit_log ORDER BY metadata_json LIMIT 10`)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joinedScan := strings.Join(scan, " | ")
	faults := notSentPlanFaults(joinedScan)
	if len(faults) == 0 {
		t.Fatalf("the guard passed a plan it exists to reject:\n  %s", joinedScan)
	}
	if !strings.Contains(strings.Join(faults, " "), "scan") {
		t.Errorf("the guard fired but not on the scan:\n  plan: %s\n  faults: %v", joinedScan, faults)
	}

	// A real covering plan: equality on the leading column plus a SELECT list
	// the index holds outright. This is the arm no shipped query reaches, so it
	// would otherwise never be executed.
	covering, err := db.QueryPlan(t.Context(), s.DB().Read(),
		`SELECT ts FROM audit_log WHERE actor_user_id = ? AND action = ? ORDER BY ts DESC LIMIT 10`,
		1, AuditActionGrab)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joinedCovering := strings.Join(covering, " | ")
	if !strings.Contains(joinedCovering, "COVERING INDEX") {
		t.Skipf("SQLite no longer plans the narrow SELECT list as covering, so the COVERING "+
			"arm cannot be fired here: %s", joinedCovering)
	}
	if len(notSentPlanFaults(joinedCovering)) == 0 {
		t.Errorf("the guard accepted a COVERING INDEX plan:\n  %s", joinedCovering)
	}
}
