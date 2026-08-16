package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// seedNotSentGrab writes the audit row a definite failure leaves behind, exactly
// as auditNotSent writes it — through grabAuditJSON rather than a hand-typed
// JSON literal, so a change to the metadata shape reaches these tests instead of
// being papered over by a fixture that still spells the old field names.
func seedNotSentGrab(t *testing.T, s *Server, userID int64, at time.Time, meta grabAuditMeta) int64 {
	t.Helper()
	meta.Outcome = outcomeNotSent
	id, err := s.store.AppendAudit(context.Background(), store.AuditEntry{
		TS:           at,
		ActorUserID:  sql.NullInt64{Int64: userID, Valid: true},
		ActorIP:      "192.168.1.2",
		Action:       auditGrabAction,
		TargetType:   "release_candidate",
		TargetID:     "7",
		Result:       store.AuditResultFail,
		MetadataJSON: grabAuditJSON(meta),
	})
	if err != nil {
		t.Fatalf("seed audit row: %v", err)
	}
	return id
}

// seedSentUnknownGrab writes BOTH rows a sent-unknown grab leaves: the
// unconfirmed provenance row internal/releases writes to keep the download id,
// and the result="warn" audit row internal/httpapi writes beside it. It is one
// grab with two rows, which is exactly the shape the union must not double-count.
func seedSentUnknownGrab(t *testing.T, s *Server, userID int64, title string, at time.Time) int64 {
	t.Helper()
	provID := seedGrabRow(t, s, userID, title, at, store.AcquisitionUnconfirmed)
	if _, err := s.store.AppendAudit(context.Background(), store.AuditEntry{
		TS:          at,
		ActorUserID: sql.NullInt64{Int64: userID, Valid: true},
		Action:      auditGrabAction,
		TargetType:  "release_candidate",
		TargetID:    "7",
		Result:      store.AuditResultWarn,
		MetadataJSON: grabAuditJSON(grabAuditMeta{
			InstanceID:   1,
			Outcome:      outcomeSentUnknown,
			Error:        string(CodeGrabOutcomeUnknown),
			ReleaseTitle: title,
			Indexer:      "an-indexer",
			Protocol:     "torrent",
			ProvenanceID: provID,
		}),
	}); err != nil {
		t.Fatalf("seed warn audit row: %v", err)
	}
	return provID
}

// THE ONE THAT KEEPS THE UNION HONEST.
//
// A sent-unknown grab writes a provenance row AND an audit row. If the audit arm
// widened from result="fail" to "anything that looks like a failure", that one
// grab would render twice — once as "sent, outcome unknown" and once as a
// failure — which is the double-grab invitation the three-state model exists to
// prevent, printed twice on the same screen.
//
// The assertion counts occurrences rather than checking a flag, because the bug
// this guards against produces two well-formed rows, not a malformed one.
func TestRecentGrabsRendersASentUnknownGrabExactlyOnce(t *testing.T) {
	s := newTestServer(t, nil)
	seedSentUnknownGrab(t, s, 1, "Ambiguous.Release-GROUP", grabTestNow)

	code, body := callRecentGrabs(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var got recentGrabsResponse
	mustJSON(t, body, &got)

	var seen []recentGrabResponse
	for _, g := range got.Grabs {
		if g.ReleaseTitle == "Ambiguous.Release-GROUP" {
			seen = append(seen, g)
		}
	}
	if len(seen) != 1 {
		t.Fatalf("one sent-unknown grab rendered as %d rows; the audit arm is picking up the "+
			"warn row that already has a provenance row behind it:\n%s", len(seen), body)
	}
	// And it is the HANDED-OVER rendering that survived, not the failure one. A
	// deduplication that kept the wrong copy would pass the count above while
	// telling the user nothing was sent — the more dangerous of the two errors,
	// because it is the one that invites the second grab.
	if seen[0].Outcome != wireOutcomeSentUnknown {
		t.Errorf("outcome = %q, want %q: this grab WAS handed over and its row must not "+
			"read as a failure", seen[0].Outcome, wireOutcomeSentUnknown)
	}
	if seen[0].AcquisitionState != store.AcquisitionUnconfirmed {
		t.Errorf("acquisition_state = %q, want the provenance row's own column",
			seen[0].AcquisitionState)
	}
	if seen[0].DownloadID == "" {
		t.Error("the surviving row has no download_id, which is the only thing an ambiguous " +
			"row can honestly point the user at")
	}
}

// The third state reaching the wire, with everything it can and cannot carry.
func TestRecentGrabsRendersDefiniteFailures(t *testing.T) {
	s := newTestServer(t, nil)
	seedNotSentGrab(t, s, 1, grabTestNow, grabAuditMeta{
		InstanceID:   1,
		Error:        string(CodeNoDownloadClient),
		Message:      "Prowlarr has no download client enabled",
		ReleaseTitle: "Never.Sent-GROUP",
		Indexer:      "an-indexer",
		Protocol:     "torrent",
	})

	code, body := callRecentGrabs(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var got recentGrabsResponse
	mustJSON(t, body, &got)
	if len(got.Grabs) != 1 {
		t.Fatalf("got %d rows, want the one failure: %s", len(got.Grabs), body)
	}
	row := got.Grabs[0]

	if row.Outcome != wireOutcomeNotSent {
		t.Errorf("outcome = %q, want %q", row.Outcome, wireOutcomeNotSent)
	}
	// The one outcome that may be worded as "it did not happen". The other two
	// begin "sent" precisely so nothing keys on a prefix and gets this wrong.
	if strings.HasPrefix(row.Outcome, "sent") {
		t.Errorf("outcome %q reads as sent; §17.5's third state is the one UsArr knows "+
			"never left the process", row.Outcome)
	}
	if row.ErrorCode != string(CodeNoDownloadClient) {
		t.Errorf("error_code = %q, want %q — the row cannot offer the right action without it",
			row.ErrorCode, CodeNoDownloadClient)
	}
	if row.ReleaseTitle != "Never.Sent-GROUP" || row.IndexerName != "an-indexer" || row.Protocol != "torrent" {
		t.Errorf("the row lost fields the audit metadata carries: %+v", row)
	}
	if row.GrabbedAt == nil || !row.GrabbedAt.Equal(grabTestNow) {
		t.Errorf("grabbed_at = %v, want the audit row's ts", row.GrabbedAt)
	}

	// WHAT IT MUST NOT CARRY, asserted rather than left to omitempty. Each of
	// these would be a claim about a file that does not exist.
	if row.SizeBytes != nil {
		t.Errorf("size_bytes = %v on a grab that was never sent", *row.SizeBytes)
	}
	if row.DownloadID != "" {
		t.Errorf("download_id = %q on a grab no download client ever saw", row.DownloadID)
	}
	if row.AcquisitionState != "" {
		t.Errorf("acquisition_state = %q, but there is no provenance row to have one",
			row.AcquisitionState)
	}
	// The key must be ABSENT, not empty: absent is what tells the client there is
	// no provenance row, and "" would read as an unnamed state.
	if strings.Contains(body, "acquisition_state") {
		t.Errorf("acquisition_state is present on a not-sent row; its absence is the signal:\n%s", body)
	}
}

// The union is ONE list in ONE order, which is the whole reason the two reads are
// merged in the handler rather than shipped as two arrays.
func TestRecentGrabsMergesBothArmsNewestFirst(t *testing.T) {
	s := newTestServer(t, nil)

	seedGrabRow(t, s, 1, "sent-oldest", grabTestNow.Add(-3*time.Hour), store.AcquisitionConfirmed)
	seedNotSentGrab(t, s, 1, grabTestNow.Add(-2*time.Hour), grabAuditMeta{ReleaseTitle: "failed-middle"})
	seedGrabRow(t, s, 1, "sent-newer", grabTestNow.Add(-time.Hour), store.AcquisitionUnconfirmed)
	seedNotSentGrab(t, s, 1, grabTestNow, grabAuditMeta{ReleaseTitle: "failed-newest"})

	_, body := callRecentGrabs(t, s, "")
	var got recentGrabsResponse
	mustJSON(t, body, &got)

	titles := make([]string, 0, len(got.Grabs))
	for _, g := range got.Grabs {
		titles = append(titles, g.ReleaseTitle)
	}
	want := "failed-newest,sent-newer,failed-middle,sent-oldest"
	if strings.Join(titles, ",") != want {
		t.Errorf("order = %s\nwant   = %s\nThe arms are interleaved by time, not concatenated.",
			strings.Join(titles, ","), want)
	}
}

// The limit is the union's, not each arm's. Ten rows on each side must not
// produce twenty.
func TestRecentGrabsLimitsTheUnionAndNotEachArm(t *testing.T) {
	s := newTestServer(t, nil)
	for i := range store.RecentProvenanceDefaultLimit {
		at := grabTestNow.Add(time.Duration(i) * time.Minute)
		seedGrabRow(t, s, 1, "sent-"+strconv.Itoa(i), at, store.AcquisitionConfirmed)
		seedNotSentGrab(t, s, 1, at, grabAuditMeta{ReleaseTitle: "failed-" + strconv.Itoa(i)})
	}

	_, body := callRecentGrabs(t, s, "")
	var got recentGrabsResponse
	mustJSON(t, body, &got)
	if len(got.Grabs) != store.RecentProvenanceDefaultLimit {
		t.Fatalf("got %d rows for limit %d; the clamp is applied to the merged list, not to "+
			"each arm separately", len(got.Grabs), got.Limit)
	}

	// And the rows kept are the newest of BOTH arms, not the newest of one. This
	// is the assertion an arm-by-arm truncation fails: with the limit split, the
	// oldest rows of each arm would survive alongside the newest.
	var sawFailure bool
	for _, g := range got.Grabs {
		if g.Outcome == wireOutcomeNotSent {
			sawFailure = true
		}
		if strings.HasSuffix(g.ReleaseTitle, "-0") {
			t.Errorf("the oldest row of an arm survived the merge: %q", g.ReleaseTitle)
		}
	}
	if !sawFailure {
		t.Error("no failure survived the merge, so this proves nothing about the union")
	}
}

// The access scope reaches the new arm too. audit_log holds every user's actions,
// not only their grabs, so an unscoped read here is a wider leak than the
// provenance one.
func TestRecentGrabsShowsOnlyTheCallersFailures(t *testing.T) {
	s := newTestServer(t, nil)
	seedNotSentGrab(t, s, 1, grabTestNow.Add(-time.Hour), grabAuditMeta{ReleaseTitle: "Mine.Failed-GROUP"})
	seedNotSentGrab(t, s, 2, grabTestNow, grabAuditMeta{ReleaseTitle: "Someone.Elses.Failed-GROUP"})

	_, body := callRecentGrabs(t, s, "")
	if !strings.Contains(body, "Mine.Failed-GROUP") {
		t.Fatalf("the caller cannot see their own failure, so this proves nothing: %s", body)
	}
	if strings.Contains(body, "Someone.Elses") {
		t.Fatalf("another user's failed grab crossed to the client: %s", body)
	}
}

// The audit-derived id gets the same treatment as the provenance one, and the
// raw audit rowid must not be recoverable from the response.
//
// audit_log.id is the WIDER oracle of the two: provenance grows once per
// acquisition, audit_log grows on every audited action by every user, so its
// sequence measures far more than grab volume.
func TestNotSentRowShipsAnOpaqueIDNotTheAuditRowid(t *testing.T) {
	s := newTestServer(t, nil)
	auditID := seedNotSentGrab(t, s, 1, grabTestNow, grabAuditMeta{ReleaseTitle: "Never.Sent-GROUP"})

	_, body := callRecentGrabs(t, s, "")
	if !strings.Contains(body, "Never.Sent-GROUP") {
		t.Fatalf("the response holds no failure, so it proves nothing: %s", body)
	}
	var got recentGrabsResponse
	mustJSON(t, body, &got)
	if len(got.Grabs) != 1 {
		t.Fatalf("got %d rows, want 1", len(got.Grabs))
	}

	raw := strconv.FormatInt(auditID, 10)
	if strings.Contains(body, `"id":`+raw) || strings.Contains(body, `"id":"`+raw+`"`) {
		t.Errorf("the response ships the raw audit rowid %s; stringifying it would keep the "+
			"leak too:\n%s", raw, body)
	}
	if _, err := strconv.ParseInt(got.Grabs[0].ID, 10, 64); err == nil {
		t.Errorf("the wire id %q parses as an integer; a client will read order into it",
			got.Grabs[0].ID)
	}
	if got.Grabs[0].ID != auditRowID(s.cfg.AuditRowIDKey, 1, auditID) {
		t.Errorf("the wire id is not the keyed hash of (actor_user_id, audit rowid): %q",
			got.Grabs[0].ID)
	}

	// Stable across requests, or identity-keyed focus and hover rebuild on every
	// poll — the same property the provenance arm is pinned for.
	_, second := callRecentGrabs(t, s, "")
	var again recentGrabsResponse
	mustJSON(t, second, &again)
	if again.Grabs[0].ID != got.Grabs[0].ID {
		t.Errorf("two calls gave %q then %q; the not-sent id is not stable",
			got.Grabs[0].ID, again.Grabs[0].ID)
	}
}

// THE COLLISION THE SECOND HKDF LABEL EXISTS TO PREVENT.
//
// provenance.id and audit_log.id are two independent sequences, so (user 3, row
// 41) names a different row in each — and one response carries both arms. Under
// a shared key the two would hash to one token and the client would key two rows
// on one id.
func TestTheTwoRowIDFamiliesCannotCollide(t *testing.T) {
	grabKey, auditKey := testGrabRowIDKey(t), testAuditRowIDKey(t)

	// The keys themselves must differ, or nothing below means anything.
	if string(grabKey) == string(auditKey) {
		t.Fatal("the two derived keys are identical; the info labels are not separating them")
	}
	for _, n := range []int64{1, 41, 4242} {
		for _, u := range []int64{0, 1, 7} {
			if grabRowID(grabKey, u, n) == auditRowID(auditKey, u, n) {
				t.Errorf("provenance row %d and audit row %d hash to the same id for user %d; "+
					"the two id families share a keyspace", n, n, u)
			}
		}
	}

	// And the audit id keeps the properties the grab id has, because it is the
	// same construction. The fixed-width input is the one worth re-asserting:
	// decimal or variable-width concatenation makes (user 1, row 23) and
	// (user 12, row 3) collide, which is a cross-user id collision.
	if auditRowID(auditKey, 1, 23) == auditRowID(auditKey, 12, 3) {
		t.Error("(user 1, row 23) and (user 12, row 3) collide; the HMAC inputs are not fixed-width")
	}
	if auditRowID(auditKey, 1, 7) == auditRowID(auditKey, 2, 7) {
		t.Error("the same audit row gives the same id to two users; actor_user_id is not in the input")
	}
}

// A NULL actor must never reach the id function.
//
// The scoped predicate `actor_user_id IN (0, :uid)` already makes such a row
// unreadable — `NULL IN (0, 1)` is unknown, not true — so this cannot happen
// today. It is asserted anyway because the failure mode if the predicate ever
// changes is silent and worse than an error: sql.NullInt64's zero value would
// feed user 0 into the HMAC and mint a stable, plausible id in the WRONG domain,
// colliding with the shared/system sentinel's rows. A row that disappears is a
// bug someone notices.
func TestNotSentRowWithNoActorIsRefusedRatherThanHashedAsUserZero(t *testing.T) {
	s := newTestServer(t, nil)

	if _, ok := s.toNotSentGrabResponse(store.AuditEntry{
		ID:           41,
		TS:           grabTestNow,
		Action:       auditGrabAction,
		Result:       store.AuditResultFail,
		MetadataJSON: grabAuditJSON(grabAuditMeta{ReleaseTitle: "Actorless-GROUP"}),
	}); ok {
		t.Fatal("an audit row with no actor was rendered; it would have been hashed in user 0's " +
			"domain, which is the shared/system sentinel")
	}

	// The row is genuinely unreachable through the endpoint as well, which is
	// what makes the check above a belt rather than the only strap.
	if _, err := s.store.AppendAudit(context.Background(), store.AuditEntry{
		TS:           grabTestNow,
		Action:       auditGrabAction,
		Result:       store.AuditResultFail,
		MetadataJSON: grabAuditJSON(grabAuditMeta{ReleaseTitle: "Actorless-GROUP"}),
	}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	_, body := callRecentGrabs(t, s, "")
	if strings.Contains(body, "Actorless-GROUP") {
		t.Errorf("an actorless audit row reached the wire:\n%s", body)
	}
}

// The leak guard, run against the ARM THAT DID NOT EXIST when it was written.
//
// The allowlist in grabs_test.go governs the whole response, so it covers this
// arm automatically — which is the point of an allowlist. What it cannot cover is
// a URL smuggled into an allowed key's VALUE, and the audit metadata is the new
// way one could get there: Message is assembled from an upstream error string.
// That is exactly why the wire shape carries error_code and not the prose.
func TestNotSentRowNeverShipsAURLOrACredential(t *testing.T) {
	s := newTestServer(t, nil)
	seedNotSentGrab(t, s, 1, grabTestNow, grabAuditMeta{
		InstanceID:   1,
		Error:        string(CodeGrabFailed),
		Message:      "the grab did not go through: Post \"http://prowlarr.lan:9696/api/v1/search?apikey=deadbeef\": refused",
		ReleaseTitle: "Never.Sent-GROUP",
		Indexer:      "an-indexer",
		Protocol:     "torrent",
	})

	_, body := callRecentGrabs(t, s, "")
	if !strings.Contains(body, "Never.Sent-GROUP") {
		t.Fatalf("the response holds no failure, so it proves nothing about leaks: %s", body)
	}

	var decoded any
	mustJSON(t, body, &decoded)
	for _, key := range jsonKeysAtEveryDepth(decoded) {
		if !recentGrabsWireKeys[key] {
			t.Errorf("the not-sent row carries the key %q, which is not on the allowlist:\n%s", key, body)
		}
	}
	for _, forbidden := range []string{"passkey", "apikey", "deadbeef", "magnet:", "http://", "https://"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("the not-sent row carries %q in a value; the error PROSE is withheld from "+
				"this surface precisely so a URL in an upstream error cannot ride out on it:\n%s",
				forbidden, body)
		}
	}
}

// RG-01.3's second half: the grab response's provenance_id.
//
// The property that has to hold is CORRELATION AND NOTHING ELSE — a caller can
// match the grab they just made to its row in Recent grabs, and learns nothing
// about anyone else's rows from either. So the assertion is equality between the
// two surfaces, not merely "it is opaque": an opaque token that disagreed with
// the list would be safe and useless.
func TestGrabResponseProvenanceIDMatchesItsRecentGrabsRow(t *testing.T) {
	s := newTestServer(t, nil)
	rowID := seedGrabRow(t, s, 1, "Just.Grabbed-GROUP", grabTestNow, store.AcquisitionConfirmed)
	a := authSession{User: store.User{ID: 1, Username: "joe", IsOwner: true}}

	token := s.provenanceRowID(a, rowID)
	if token == "" {
		t.Fatal("the grab response carries no provenance id at all")
	}
	if token == strconv.FormatInt(rowID, 10) {
		t.Errorf("provenance_id is the raw rowid %q; that is the cross-user volume oracle "+
			"RG-01.3 closed on the list read, reachable one grab at a time", token)
	}
	if _, err := strconv.ParseInt(token, 10, 64); err == nil {
		t.Errorf("provenance_id %q parses as an integer", token)
	}

	_, body := callRecentGrabs(t, s, "")
	var got recentGrabsResponse
	mustJSON(t, body, &got)
	if len(got.Grabs) != 1 {
		t.Fatalf("got %d rows, want 1", len(got.Grabs))
	}
	if got.Grabs[0].ID != token {
		t.Errorf("the grab response says %q and Recent grabs says %q for the same row; "+
			"the caller cannot correlate their own grab with its history",
			token, got.Grabs[0].ID)
	}

	// No row means no token. Hashing rowid 0 would mint a stable, plausible id
	// for a row that does not exist, and omitempty could not then drop it.
	if got := s.provenanceRowID(a, 0); got != "" {
		t.Errorf("provenanceRowID(0) = %q, want an empty string", got)
	}
}

// The handler must actually USE the opaque token, which the test above cannot
// see: it exercises provenanceRowID directly, so reverting the response field to
// the rowid would leave it green. The wire TYPE is what carries the property —
// an int64 there is the rowid by construction, whatever builds it — so that is
// what is pinned.
func TestGrabResponseProvenanceIDIsAStringOnTheWire(t *testing.T) {
	f, ok := reflect.TypeOf(grabResponse{}).FieldByName("ProvenanceID")
	if !ok {
		t.Fatal("grabResponse has no ProvenanceID field")
	}
	if f.Type.Kind() != reflect.String {
		t.Errorf("provenance_id is %s on the wire; an integer there is the raw rowid however "+
			"it was built, which is RG-01.3 reopened one grab at a time", f.Type)
	}
}
