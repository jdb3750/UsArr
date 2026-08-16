package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

var grabTestNow = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

func seedGrabRow(t *testing.T, s *Server, userID int64, title string, at time.Time, state string) {
	t.Helper()
	_, err := s.store.InsertProvenance(context.Background(), store.Provenance{
		UserID:            userID,
		Protocol:          "torrent",
		IndexerName:       "an-indexer",
		IndexerCategories: `[2040,2045]`,
		DownloadID:        "infohash-" + title,
		// A stored URL that would leak a passkey if this shape had anywhere to
		// put it. It must not appear in the response.
		NZBInfoURL:       "http://tracker.example/details?passkey=deadbeef",
		ReleaseTitle:     title,
		SizeBytes:        sql.NullInt64{Int64: 8 << 30, Valid: true},
		GrabbedAt:        sql.NullString{String: store.FormatTime(at), Valid: true},
		SourceSystem:     "prowlarr",
		AcquisitionState: state,
	})
	if err != nil {
		t.Fatalf("seed provenance %q: %v", title, err)
	}
}

// callRecentGrabs runs the handler with a session already resolved, which is
// what the authenticated middleware does before it.
func callRecentGrabs(t *testing.T, s *Server, query string) (int, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/grabs/recent"+query, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: true},
	}))
	w := httptest.NewRecorder()
	if err := s.handleRecentGrabs(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

func TestRecentGrabsIsNewestFirstAndCarriesTheOutcome(t *testing.T) {
	s := newTestServer(t, nil)
	seedGrabRow(t, s, 1, "Older.Release-GROUP", grabTestNow.Add(-time.Hour), store.AcquisitionConfirmed)
	seedGrabRow(t, s, 1, "Ambiguous.Release-GROUP", grabTestNow, store.AcquisitionUnconfirmed)

	code, body := callRecentGrabs(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/grabs/recent = %d: %s", code, body)
	}

	var got recentGrabsResponse
	mustJSON(t, body, &got)
	if got.Limit != store.RecentProvenanceDefaultLimit {
		t.Errorf("limit = %d, want the §17.5 default of %d", got.Limit, store.RecentProvenanceDefaultLimit)
	}
	if len(got.Grabs) != 2 {
		t.Fatalf("got %d grabs, want 2: %s", len(got.Grabs), body)
	}
	if got.Grabs[0].ReleaseTitle != "Ambiguous.Release-GROUP" {
		t.Errorf("first row is %q, want the newest", got.Grabs[0].ReleaseTitle)
	}

	// The whole point of the endpoint: an unconfirmed grab reads as SENT, next
	// to the confirmed one, not as a failure. Prowlarr adds the release to the
	// download client before it configures it and never rolls back, so a row
	// worded as "it did not happen" invites a second grab of the same
	// multi-gigabyte release.
	first := got.Grabs[0]
	if first.AcquisitionState != store.AcquisitionUnconfirmed {
		t.Errorf("acquisition_state = %q, want it verbatim from the column", first.AcquisitionState)
	}
	if first.Outcome != wireOutcomeSentUnknown {
		t.Errorf("outcome = %q, want %q", first.Outcome, wireOutcomeSentUnknown)
	}
	if !strings.HasPrefix(first.Outcome, "sent") {
		t.Errorf("outcome %q does not read as sent; §17.5 puts the ambiguous state beside the "+
			"confirmed one, not beside a failure", first.Outcome)
	}
	if got.Grabs[1].Outcome != wireOutcomeSent {
		t.Errorf("confirmed outcome = %q, want %q", got.Grabs[1].Outcome, wireOutcomeSent)
	}

	// Everything §17.5 renders that this row can actually supply.
	if first.SizeBytes == nil || *first.SizeBytes != 8<<30 {
		t.Errorf("size_bytes = %v", first.SizeBytes)
	}
	if first.GrabbedAt == nil || !first.GrabbedAt.Equal(grabTestNow) {
		t.Errorf("grabbed_at = %v, want %v", first.GrabbedAt, grabTestNow)
	}
	if first.IndexerName != "an-indexer" || first.Protocol != "torrent" {
		t.Errorf("indexer/protocol = %q/%q", first.IndexerName, first.Protocol)
	}
	if len(first.Categories) != 2 || first.Categories[0] != 2040 {
		t.Errorf("categories = %v, want the raw Newznab ints", first.Categories)
	}
	// The string that lets the user find the torrent in their own download
	// client, which is the only action an ambiguous row can honestly offer.
	if first.DownloadID != "infohash-Ambiguous.Release-GROUP" {
		t.Errorf("download_id = %q", first.DownloadID)
	}
}

// An unrecognised state must not be renamed into one of the two known ones. The
// column ships with NO CHECK on purpose — v0.2's request path may add "pending"
// — so this is a reachable case, and guessing here would put an unknown state
// beside "sent".
func TestRecentGrabsDoesNotInventAnOutcomeForAnUnknownState(t *testing.T) {
	if got := outcomeFor("pending"); got != wireOutcomeStateUnknown {
		t.Errorf("outcomeFor(%q) = %q, want %q", "pending", got, wireOutcomeStateUnknown)
	}
	if got := outcomeFor(""); got != wireOutcomeStateUnknown {
		t.Errorf("outcomeFor(%q) = %q, want %q", "", got, wireOutcomeStateUnknown)
	}
}

// The scope is applied by the store, and this asserts it survives the whole way
// out to the wire — the arm a handler test can check that a store test cannot.
func TestRecentGrabsShowsOnlyTheCallersGrabs(t *testing.T) {
	s := newTestServer(t, nil)
	seedGrabRow(t, s, 1, "Mine-GROUP", grabTestNow.Add(-time.Hour), store.AcquisitionConfirmed)
	seedGrabRow(t, s, 2, "Someone.Elses-GROUP", grabTestNow, store.AcquisitionConfirmed)

	code, body := callRecentGrabs(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if strings.Contains(body, "Someone.Elses") {
		t.Fatalf("another user's acquisition crossed to the client: %s", body)
	}
}

// The one hard rule: nothing on this path may hand a client a download URL or a
// credential. provenance.download_url is never written and this shape has
// nowhere to put one, but the assertion is on the BYTES rather than on the
// struct, so adding a field later cannot quietly pass.
func TestRecentGrabsNeverShipsAURLOrACredential(t *testing.T) {
	s := newTestServer(t, nil)
	seedGrabRow(t, s, 1, "Some.Release-GROUP", grabTestNow, store.AcquisitionConfirmed)

	_, body := callRecentGrabs(t, s, "")
	// "Found nothing" and "looked at nothing" must not produce the same verdict
	// (docs/DEVELOPMENT.md §11 rule 4): assert the floor first, so an empty
	// response can never make this test green.
	if !strings.Contains(body, "Some.Release-GROUP") {
		t.Fatalf("the response holds no grab, so it proves nothing about leaks: %s", body)
	}
	for _, forbidden := range []string{
		"download_url", "downloadUrl", "magnet", "apikey", "api_key", "apiKey",
		"passkey", "nzb_info_url", "info_url", "http://", "https://",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the response carries %q, which must never reach a browser:\n%s", forbidden, body)
		}
	}
}

func TestRecentGrabsBoundsTheLimit(t *testing.T) {
	s := newTestServer(t, nil)
	for i := range store.RecentProvenanceDefaultLimit + 3 {
		seedGrabRow(t, s, 1, "R"+string(rune('a'+i)), grabTestNow.Add(time.Duration(i)*time.Minute),
			store.AcquisitionConfirmed)
	}

	code, body := callRecentGrabs(t, s, "?limit=3")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var got recentGrabsResponse
	mustJSON(t, body, &got)
	if len(got.Grabs) != 3 || got.Limit != 3 {
		t.Errorf("limit=3 returned %d rows with limit %d", len(got.Grabs), got.Limit)
	}

	// Clamped, and the clamp is reported. A client that asked for 10000 and got
	// 200 rows would otherwise read the short answer as "that is all there is".
	_, body = callRecentGrabs(t, s, "?limit=10000")
	mustJSON(t, body, &got)
	if got.Limit != store.RecentProvenanceMaxLimit {
		t.Errorf("limit = %d, want it clamped to %d", got.Limit, store.RecentProvenanceMaxLimit)
	}

	// A garbage limit is a 400 that says so, not a silent default.
	code, body = callRecentGrabs(t, s, "?limit=-1")
	if code != http.StatusBadRequest {
		t.Errorf("limit=-1 returned %d: %s", code, body)
	}
}

// The route is behind the session, and a bearer or API-key credential is refused
// rather than ignored: v0.1 has no bearer path at all.
func TestRecentGrabsRequiresASessionCookie(t *testing.T) {
	s := newTestServer(t, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	code, body := get(t, srv.URL+"/api/v1/grabs/recent")
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET = %d, want 401: %s", code, body)
	}
	if strings.Contains(body, "release_title") {
		t.Fatalf("an unauthenticated caller saw acquisition history: %s", body)
	}
}
