package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// The not-sent arm of Recent grabs reads failed grabs out of audit_log. Every
// test here is against the read that serves it: it must be scoped, it must be
// filterable, and it must use the index migration 0002 added for it.

// appendGrab writes one grab audit row for a user.
func appendGrab(t *testing.T, s *Store, actor int64, at time.Time, action, result, meta string) int64 {
	t.Helper()
	id, err := s.AppendAudit(context.Background(), AuditEntry{
		TS:           at,
		ActorUserID:  sql.NullInt64{Int64: actor, Valid: true},
		ActorIP:      "192.168.1.2",
		Action:       action,
		TargetType:   "release_candidate",
		TargetID:     "7",
		Result:       result,
		MetadataJSON: meta,
	})
	if err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	return id
}

// THE ONE THIS PARAMETER EXISTS FOR.
//
// Before ListAuditLog took a scope, any caller could read every row in the
// table. On the grab path that is one user's release titles, their indexers and
// their failures handed to another user — the same class of leak
// GetProvenanceByDownloadID was fixed for, on the other arm of the same union.
//
// The assertion is deliberately two-sided: the second user sees none of the
// first's rows AND still sees their own. A read that returns nothing at all
// would pass a one-sided test while being just as broken.
func TestAuditReadDoesNotLeakOneUsersFailedGrabsToAnother(t *testing.T) {
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

	appendGrab(t, s, alice, testNow, "release.grab", "fail",
		`{"outcome":"not_sent","release_title":"Alice.Private.Release.2026.1080p"}`)
	appendGrab(t, s, bob, testNow.Add(time.Second), "release.grab", "fail",
		`{"outcome":"not_sent","release_title":"Bob.Own.Release.2026.1080p"}`)

	q := AuditQuery{Actions: []string{"release.grab"}, Results: []string{"fail"}, Limit: 50}

	got, err := s.ListAuditLog(ctx, Scope{UserID: bob}, q)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("bob sees %d rows, want exactly his own 1", len(got))
	}
	if !got[0].ActorUserID.Valid || got[0].ActorUserID.Int64 != bob {
		t.Errorf("bob's read returned actor %+v, want %d", got[0].ActorUserID, bob)
	}
	if strings.Contains(got[0].MetadataJSON, "Alice.Private.Release") {
		t.Errorf("bob can read alice's release title through the audit log: %q", got[0].MetadataJSON)
	}

	// And the scope is a filter, not a mute button: alice still sees hers.
	got, err = s.ListAuditLog(ctx, Scope{UserID: alice}, q)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(got) != 1 || !strings.Contains(got[0].MetadataJSON, "Alice.Private.Release") {
		t.Fatalf("alice cannot read her own failed grab: %+v", got)
	}
}

// The filters the not-sent arm needs. Without them the caller reads the whole
// log and sorts it in Go, which is the read this index was added to stop.
func TestAuditReadFiltersOnActionAndResult(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	actor, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local", IsOwner: true})
	if err != nil {
		t.Fatal(err)
	}

	appendGrab(t, s, actor, testNow, "release.grab", "fail", `{"outcome":"not_sent"}`)
	appendGrab(t, s, actor, testNow.Add(1*time.Second), "release.grab", "warn", `{"outcome":"sent_unknown"}`)
	appendGrab(t, s, actor, testNow.Add(2*time.Second), "release.grab", "ok", `{"outcome":"sent_confirmed"}`)
	appendGrab(t, s, actor, testNow.Add(3*time.Second), "user.login", "fail", `{}`)

	scope := OwnerScope(actor)

	all, err := s.ListAuditLog(ctx, scope, AuditQuery{})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("unfiltered read returned %d rows, want 4", len(all))
	}
	// Newest first, and the tiebreak is not exercised here because the four
	// timestamps differ.
	if all[0].Action != "user.login" {
		t.Errorf("first row = %q, want the newest", all[0].Action)
	}

	// The not-sent arm's actual query.
	notSent, err := s.ListAuditLog(ctx, scope, AuditQuery{
		Actions: []string{"release.grab"},
		Results: []string{"fail"},
	})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(notSent) != 1 {
		t.Fatalf("the not-sent read returned %d rows, want 1", len(notSent))
	}
	if !strings.Contains(notSent[0].MetadataJSON, "not_sent") {
		t.Errorf("the not-sent read returned the wrong row: %q", notSent[0].MetadataJSON)
	}

	// Both handed-over states in one read, which is the other half of the union.
	sent, err := s.ListAuditLog(ctx, scope, AuditQuery{
		Actions: []string{"release.grab"},
		Results: []string{"ok", "warn"},
	})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("the sent read returned %d rows, want 2", len(sent))
	}

	// A filter that matches nothing returns nothing rather than everything.
	none, err := s.ListAuditLog(ctx, scope, AuditQuery{Actions: []string{"nothing.happened"}})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("an unmatched action filter returned %d rows, want 0", len(none))
	}
}

// An action with no actor is attributable to nobody, so it is readable by
// nobody: `NULL IN (0, :uid)` is unknown, not true.
//
// This is pinned rather than left implicit because it is the load-bearing half
// of a decision made in internal/httpapi/grab.go — the no-session rejection
// writes no audit row precisely because this read could never return it, and a
// change that made anonymous rows visible here would quietly turn that into a
// missing feature instead of a deliberate omission.
func TestAuditReadReturnsNoRowThatNamesNoActor(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	actor, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local", IsOwner: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAudit(ctx, AuditEntry{
		TS:      testNow,
		Action:  "release.grab",
		Result:  "fail",
		ActorIP: "10.0.0.9",
	}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}

	got, err := s.ListAuditLog(ctx, OwnerScope(actor), AuditQuery{})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("an actorless row is readable through the scoped read: %+v\n"+
			"If that is now intended, internal/httpapi/grab.go's no-session branch should "+
			"start writing a row and this test should say so.", got)
	}

	// The chain check is a different job and still sees everything.
	if bad, err := s.VerifyAuditChain(ctx); err != nil || bad != 0 {
		t.Errorf("VerifyAuditChain(%d, %v): the chain must be verified whole", bad, err)
	}
}

// ix_audit_actor_action was added by migration 0002 for exactly this read and,
// until the filters landed, was used by no query in the codebase at all.
//
// The plan is EXPLAINed from auditListSQL, so this asserts the statement
// ListAuditLog actually issues rather than a copy pasted into a test.
//
// It asserts SEARCH … USING INDEX and never COVERING INDEX, per
// docs/reference/schema.md §1: whether SQLite can serve the read from the index
// alone depends on the SELECT list, so pinning COVERING fails the moment a
// caller selects one more column — a change to the query, not an index
// regression.
func TestAuditReadUsesTheActorActionIndex(t *testing.T) {
	s := newTestStore(t)
	query, args := auditListSQL(OwnerScope(1), AuditQuery{
		Actions: []string{"release.grab"},
		Results: []string{"fail"},
		Limit:   50,
	})

	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")

	if !strings.Contains(joined, "ix_audit_actor_action") {
		t.Fatalf("the not-sent read does not use ix_audit_actor_action:\n  %s\n"+
			"Migration 0002 added that index for this read and nothing else uses it.", joined)
	}
	if !strings.Contains(joined, "SEARCH") {
		t.Errorf("the plan is not a SEARCH:\n  %s", joined)
	}
	if strings.Contains(joined, "SCAN audit_log") {
		t.Errorf("the plan scans a table that grows forever by design:\n  %s", joined)
	}
	if strings.Contains(joined, "COVERING INDEX") {
		t.Errorf("the plan is COVERING:\n  %s\n"+
			"schema.md §1 says assert SEARCH … USING INDEX and never COVERING — a covering "+
			"plan here means the SELECT list shrank, and pinning it makes the next added "+
			"column look like an index regression.", joined)
	}
}

// TestScopedAuditOrderNeedsASort records what the CANONICAL scope predicate
// costs on this read, exactly as TestScopedProvenanceOrderNeedsASort does for
// the other arm of the union.
//
// `actor_user_id IN (0, :uid)` constrains the LEADING column of
// ix_audit_actor_action with IN, and SQLite cannot supply ORDER BY from such an
// index — it plans the SEARCH and then a temp b-tree. The index is still doing
// most of its job: the walk is one actor's rows for one action rather than every
// row ever appended, and the sort is over that bounded set under a LIMIT.
//
// t.Errorf, not t.Logf: if SQLite or a rewritten predicate ever makes this
// ordered, that is good news and it makes the note on ListAuditLog wrong, so it
// must fail and force the edit.
func TestScopedAuditOrderNeedsASort(t *testing.T) {
	s := newTestStore(t)
	query, args := auditListSQL(OwnerScope(1), AuditQuery{
		Actions: []string{"release.grab"},
		Results: []string{"fail"},
		Limit:   50,
	})
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")
	if !strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("the scoped audit read is now ordered by the index: %s\n"+
			"That is an improvement, and it makes the note on ListAuditLog and on this test "+
			"wrong. Update both in the same change.", joined)
	}
}

// The scope must be applied IN THE SQL, not merely accepted in the signature.
// This project has already found a parameter that was taken and ignored twice,
// so the rendered statement is asserted directly: a read whose WHERE clause does
// not constrain actor_user_id is unscoped however good its signature looks.
func TestAuditListSQLConstrainsTheActor(t *testing.T) {
	query, args := auditListSQL(Scope{UserID: 42}, AuditQuery{Limit: 10})
	if !strings.Contains(query, "actor_user_id IN (?, ?)") {
		t.Fatalf("the audit read does not constrain actor_user_id:\n%s", query)
	}
	if len(args) != 3 || args[0] != SystemUserID || args[1] != int64(42) || args[2] != 10 {
		t.Fatalf("args = %#v, want [SystemUserID 42 limit]", args)
	}
}
