package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// The not-sent arm of Recent grabs renders from audit_log, so a grab that
// failed before dispatch has to LEAVE A ROW and that row has to carry enough to
// display. handleGrab has seven early returns ahead of the dispatch and every
// one of them used to return silently, which made the whole arm empty.
//
// These tests pin both halves of the judgement: which four write a row, and
// which three deliberately do not.

// grabTestFixture builds a server with one user, one Prowlarr and one stored
// release candidate, and returns the request factory bound to that user.
type grabTestFixture struct {
	server      *Server
	userID      int64
	instanceID  int64
	candidateID int64
}

func newGrabFixture(t *testing.T) grabTestFixture {
	t.Helper()
	ctx := t.Context()
	s := newTestServer(t, nil)

	userID, err := s.store.CreateUser(ctx, store.User{
		Username: "joe", AuthSource: "local", IsOwner: true,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	instanceID, err := s.store.CreateServiceInstance(ctx, store.ServiceInstance{
		Kind: "prowlarr", Role: "indexer", Name: "prowlarr-1",
		BaseURL: "http://prowlarr.internal:9696", Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	ids, err := s.store.InsertReleaseCandidates(ctx, []store.ReleaseCandidate{{
		UserID:            userID,
		ServiceInstanceID: instanceID,
		GUID:              "guid-1",
		Title:             "Some.Release.2026.1080p.WEB-DL-GRP",
		Indexer:           "an-indexer",
		Protocol:          "torrent",
		RawReleaseJSON:    `{"title":"Some.Release.2026.1080p.WEB-DL-GRP"}`,
		FetchedAt:         time.Now(),
		ExpiresAt:         time.Now().Add(25 * time.Minute),
	}})
	if err != nil || len(ids) != 1 {
		t.Fatalf("InsertReleaseCandidates: %v", err)
	}
	return grabTestFixture{server: s, userID: userID, instanceID: instanceID, candidateID: ids[0]}
}

// grab issues one POST /api/v1/releases/{id}/grab straight at the handler.
// pathID is a string so a malformed id is expressible.
func (f grabTestFixture) grab(t *testing.T, pathID, body string, withSession bool) error {
	t.Helper()
	ctx := t.Context()
	if withSession {
		user, err := f.server.store.GetUser(ctx, f.userID)
		if err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		ctx = context.WithValue(ctx, ctxKeySession, authSession{User: user})
	}
	r := httptest.NewRequestWithContext(ctx, "POST",
		"/api/v1/releases/"+pathID+"/grab", strings.NewReader(body))
	r.SetPathValue("id", pathID)
	return f.server.handleGrab(httptest.NewRecorder(), r)
}

// grabRows reads every release.grab audit row this user can see.
func (f grabTestFixture) grabRows(t *testing.T) []store.AuditEntry {
	t.Helper()
	rows, err := f.server.store.ListAuditLog(t.Context(), store.OwnerScope(f.userID), store.AuditQuery{
		Actions: []string{"release.grab"},
	})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	return rows
}

func decodeGrabMeta(t *testing.T, raw string) grabAuditMeta {
	t.Helper()
	var m grabAuditMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("audit metadata is not JSON (%v): %q\n"+
			"A row the reader cannot parse is a row that never appears in Recent grabs, "+
			"which is the failure this metadata exists to fix.", err, raw)
	}
	return m
}

// A grab of a candidate that is gone: a real attempt by a real user that never
// dispatched, and before this change the only trace was a 404 in the request
// log.
func TestGrabOfAMissingCandidateLeavesAnAuditRow(t *testing.T) {
	f := newGrabFixture(t)

	if err := f.grab(t, "999999", `{}`, true); err == nil {
		t.Fatal("grabbing a candidate that does not exist returned no error")
	}

	rows := f.grabRows(t)
	if len(rows) != 1 {
		t.Fatalf("got %d audit rows, want 1 — a grab attempt that failed before dispatch "+
			"must still be readable, or the not-sent arm of Recent grabs is empty by "+
			"construction", len(rows))
	}
	if rows[0].Result != "fail" {
		t.Errorf("result = %q, want %q: nothing was sent and UsArr knows it", rows[0].Result, "fail")
	}
	if rows[0].TargetID != "999999" {
		t.Errorf("target_id = %q, want the candidate the user actually asked for", rows[0].TargetID)
	}
	m := decodeGrabMeta(t, rows[0].MetadataJSON)
	if m.Outcome != outcomeNotSent {
		t.Errorf("outcome = %q, want %q", m.Outcome, outcomeNotSent)
	}
	if m.Error != string(CodeNotFound) {
		t.Errorf("error = %q, want %q", m.Error, CodeNotFound)
	}
	// The title is genuinely unknown here — the listing was swept — and the row
	// says so by omitting it rather than by inventing one.
	if m.ReleaseTitle != "" {
		t.Errorf("release_title = %q, but there was no candidate to take a title from", m.ReleaseTitle)
	}
	if m.Message == "" {
		t.Error("the row carries no error text, so it renders as a failure with no reason")
	}
}

// The best-furnished pre-dispatch failure: the candidate resolved, so the row
// can carry everything Recent grabs renders.
func TestGrabInstanceMismatchLeavesAReadableAuditRow(t *testing.T) {
	f := newGrabFixture(t)

	err := f.grab(t, strconv.FormatInt(f.candidateID, 10), `{"instance_id":`+strconv.FormatInt(f.instanceID+1, 10)+`}`, true)
	if err == nil {
		t.Fatal("an instance mismatch returned no error")
	}

	rows := f.grabRows(t)
	if len(rows) != 1 {
		t.Fatalf("got %d audit rows, want 1", len(rows))
	}
	m := decodeGrabMeta(t, rows[0].MetadataJSON)

	// THE THREE FIELDS THAT MAKE A ROW RENDERABLE. Before this change the
	// metadata was instance_id, outcome, an error CODE and a provenance id — so
	// even a row that existed had nothing to display but an enum.
	if m.ReleaseTitle != "Some.Release.2026.1080p.WEB-DL-GRP" {
		t.Errorf("release_title = %q; a Recent-grabs row with no release name is not a row", m.ReleaseTitle)
	}
	if m.Indexer != "an-indexer" {
		t.Errorf("indexer = %q, want the indexer the candidate came from", m.Indexer)
	}
	if !strings.Contains(m.Message, "different service instance") {
		t.Errorf("message = %q, want the error text the user was shown", m.Message)
	}

	if m.Error != string(CodeInstanceMismatch) {
		t.Errorf("error = %q, want %q", m.Error, CodeInstanceMismatch)
	}
	if m.Outcome != outcomeNotSent {
		t.Errorf("outcome = %q, want %q", m.Outcome, outcomeNotSent)
	}
	if m.InstanceID != f.instanceID {
		t.Errorf("instance_id = %d, want the candidate's own instance %d — the candidate is "+
			"authoritative, not the id the client asserted", m.InstanceID, f.instanceID)
	}
	if m.Protocol != "torrent" {
		t.Errorf("protocol = %q", m.Protocol)
	}
}

// No release-search service is wired into the test server, so searcherFor is
// the branch that fails. The candidate resolved first, so this row is fully
// furnished: "your stack is broken and nothing was sent" is exactly what the
// not-sent arm is for.
func TestGrabWithNoIndexerServiceLeavesAnAuditRow(t *testing.T) {
	f := newGrabFixture(t)

	if err := f.grab(t, strconv.FormatInt(f.candidateID, 10), `{}`, true); err == nil {
		t.Fatal("grabbing with no release service wired in returned no error")
	}

	rows := f.grabRows(t)
	if len(rows) != 1 {
		t.Fatalf("got %d audit rows, want 1", len(rows))
	}
	m := decodeGrabMeta(t, rows[0].MetadataJSON)
	if m.Outcome != outcomeNotSent {
		t.Errorf("outcome = %q, want %q", m.Outcome, outcomeNotSent)
	}
	if m.ReleaseTitle != "Some.Release.2026.1080p.WEB-DL-GRP" {
		t.Errorf("release_title = %q", m.ReleaseTitle)
	}
	if m.Message == "" {
		t.Error("the row carries no error text")
	}
}

// THE OTHER HALF OF THE JUDGEMENT, and the reason this is a test rather than a
// comment: writing all seven early returns would fill an append-only table with
// scanner traffic and client bugs.
//
// None of these three named a release. The no-session case additionally has no
// actor at all, so the row it would write is one the scoped read can never
// return (store.TestAuditReadReturnsNoRowThatNamesNoActor) — and it is the one
// branch reachable without credentials, which makes it an unauthenticated way to
// grow a table nothing can delete.
func TestGrabWritesNoAuditRowForARequestThatNamedNoRelease(t *testing.T) {
	for _, tc := range []struct {
		name        string
		pathID      string
		body        string
		withSession bool
	}{
		{"no session at all", "1", `{}`, false},
		{"a path id that is not an id", "wat", `{}`, true},
		{"a body that will not decode", "1", `{"instance_id":`, true},
		{"a body with a field this endpoint does not take", "1", `{"nope":1}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrabFixture(t)
			if err := f.grab(t, tc.pathID, tc.body, tc.withSession); err == nil {
				t.Fatal("the request was accepted")
			}
			if rows := f.grabRows(t); len(rows) != 0 {
				t.Errorf("got %d audit rows, want 0: %+v\n"+
					"This request named no release, so the row it wrote can carry no title, "+
					"no indexer and no instance — it renders as a failed grab of nothing. "+
					"If including it is now intended, say why in auditNotSent's comment.",
					len(rows), rows)
			}
			// Not even an unattributable one: the scoped read hides those, so a
			// count over the whole table is the only honest check.
			if total, err := f.server.store.ListAuditLog(t.Context(),
				store.OwnerScope(0), store.AuditQuery{}); err != nil {
				t.Fatalf("ListAuditLog: %v", err)
			} else if len(total) != 0 {
				t.Errorf("an unattributable row was appended anyway: %+v", total)
			}
		})
	}
}

// The metadata is built with encoding/json, not fmt.Sprintf with %q. Go quoting
// and JSON quoting agree on ASCII and diverge past it, and a release title is
// exactly the field carrying an em dash, a quote or a backslash straight from an
// indexer. A row whose metadata will not parse never reaches the screen.
func TestGrabAuditMetadataSurvivesAHostileReleaseTitle(t *testing.T) {
	hostile := `Some "Release" \ 2026 — 1080p	x265` + "\x00" + `日本語`
	raw := grabAuditJSON(grabAuditMeta{
		InstanceID:   3,
		Outcome:      outcomeNotSent,
		Error:        string(CodeGrabFailed),
		ReleaseTitle: hostile,
		Indexer:      `a "quoted" indexer`,
	})

	m := decodeGrabMeta(t, raw)
	if m.ReleaseTitle != hostile {
		t.Errorf("release_title round-tripped to %q, want %q", m.ReleaseTitle, hostile)
	}
	if m.Indexer != `a "quoted" indexer` {
		t.Errorf("indexer round-tripped to %q", m.Indexer)
	}
}
