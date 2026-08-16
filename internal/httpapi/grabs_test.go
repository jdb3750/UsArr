package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"math/bits"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/store"
)

var grabTestNow = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

// testGrabRowIDKey is the Config.GrabRowIDKey every test server gets. It is
// derived through the SHIPPING derivation from a FIXED secret and salt, not
// generated randomly, and both halves of that matter:
//
//   - Fixed, because the published row id must be stable across restarts. Two
//     newTestServer calls stand in for two runs of the process, and a random key
//     would make them disagree — which is precisely the bug
//     TestGrabRowIDIsStableAcrossServerInstances exists to catch.
//   - Through crypto.DeriveGrabRowIDKey rather than 32 hand-written bytes, so
//     the tests exercise the real key path and a change to the info label shows
//     up here rather than only in production.
func testGrabRowIDKey(t *testing.T) []byte {
	t.Helper()
	key, err := crypto.DeriveGrabRowIDKey(
		bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32))
	if err != nil {
		t.Fatalf("derive grab row id key: %v", err)
	}
	return key
}

// testAuditRowIDKey is the same story for the not-sent arm: the SHIPPING
// derivation over the SAME fixed secret and salt, so the two keys differ only by
// their info label — which is the property the whole domain separation rests on,
// and the one a hand-written constant here would quietly fake.
func testAuditRowIDKey(t *testing.T) []byte {
	t.Helper()
	key, err := crypto.DeriveAuditRowIDKey(
		bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32))
	if err != nil {
		t.Fatalf("derive audit row id key: %v", err)
	}
	return key
}

// seedGrabRow inserts one provenance row and returns its RAW rowid — the value
// that must never reach the wire. Tests need it to assert its own absence.
func seedGrabRow(t *testing.T, s *Server, userID int64, title string, at time.Time, state string) int64 {
	t.Helper()
	id, err := s.store.InsertProvenance(context.Background(), store.Provenance{
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
	return id
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

// recentGrabsWireKeys is every JSON key GET /api/v1/grabs/recent is allowed to
// emit, at any depth. It is an ALLOWLIST, and that is the whole point of it: a
// denylist passes everything it does not enumerate, and provenance carries four
// columns — download_url, nzb_info_url, release_guid, torrent_info_hash — that a
// name-based ban list had no entry for. A Newznab `guid` is frequently the
// download URL itself and therefore passkey-bearing, so "we did not think to ban
// it" is exactly how one ships.
//
// Adding a field to recentGrabResponse must fail this test until the name is
// added here deliberately. That is the review step, expressed as a constant.
var recentGrabsWireKeys = map[string]bool{
	// envelope
	"grabs": true,
	"limit": true,
	// one grab
	"id":                true,
	"release_title":     true,
	"protocol":          true,
	"indexer_name":      true,
	"categories":        true,
	"size_bytes":        true,
	"grabbed_at":        true,
	"download_id":       true,
	"source_system":     true,
	"acquisition_state": true,
	"outcome":           true,
	// The not-sent arm's only new key. It is a CLOSED vocabulary — errorcodes.go's
	// constants — which is why it is admissible here at all: the error PROSE it
	// replaces is assembled from an upstream error string and passes through
	// redactText, which strips credentials from a URL but leaves scheme://host
	// behind, so admitting the sentence would mean loosening the value check
	// below to accommodate a field.
	"error_code": true,
}

// The one hard rule: nothing on this path may hand a client a download URL or a
// credential. provenance.download_url is never written and this shape has
// nowhere to put one, but the assertion is on the MARSHALLED JSON rather than on
// the struct, so adding a field later cannot quietly pass.
//
// Same idiom as mapping.TestCatalogProjectionCannotCarryAnIndexerCredential,
// deliberately rather than a second one: that path settled on an allowlist for
// the same reason, and two shapes of the same guard is how one of them rots.
func TestRecentGrabsNeverShipsAURLOrACredential(t *testing.T) {
	s := newTestServer(t, nil)
	rowID := seedGrabRow(t, s, 1, "Some.Release-GROUP", grabTestNow, store.AcquisitionConfirmed)

	_, body := callRecentGrabs(t, s, "")
	// "Found nothing" and "looked at nothing" must not produce the same verdict
	// (docs/DEVELOPMENT.md §11 rule 4): assert the floor first, so an empty
	// response can never make this test green.
	if !strings.Contains(body, "Some.Release-GROUP") {
		t.Fatalf("the response holds no grab, so it proves nothing about leaks: %s", body)
	}

	var decoded any
	mustJSON(t, body, &decoded)
	for _, key := range jsonKeysAtEveryDepth(decoded) {
		if !recentGrabsWireKeys[key] {
			t.Errorf("the response carries the key %q, which is not on the allowlist "+
				"— add it to recentGrabsWireKeys only after checking it cannot carry a "+
				"credential:\n%s", key, body)
		}
	}

	// The allowlist governs KEYS; a URL smuggled into an allowed key's VALUE
	// would still slip past it. The seeded row's nzb_info_url is a passkey-
	// bearing tracker URL precisely so this half has something real to catch.
	for _, forbidden := range []string{"passkey", "magnet:", "http://", "https://"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("the response carries %q in a value, which must never reach a browser:\n%s", forbidden, body)
		}
	}

	// RG-01.3, ASSERTED HERE RATHER THAN IN A TEST OF ITS OWN. The raw rowid
	// leaks through a key the allowlist ALLOWS — `id` is on the list and must
	// stay there, since the client needs something to key rows on — so the
	// allowlist above cannot catch this and a value-level assertion is the only
	// one that can. It belongs beside the other value check rather than in a
	// second "nothing leaks" test, which is how one of the two rots.
	//
	// provenance.id is a globally monotonic sequence shared across users, so
	// shipping it hands a caller a volume oracle over other people's rows that
	// the access scope filter cannot close.
	raw := strconv.FormatInt(rowID, 10)
	if strings.Contains(body, `"id":`+raw) {
		t.Errorf("the response ships the raw provenance rowid %s as a number, which is the "+
			"cross-user volume oracle RG-01.3 removed:\n%s", raw, body)
	}
	if strings.Contains(body, `"id":"`+raw+`"`) {
		t.Errorf("the response ships the raw provenance rowid %s as a string; stringifying it "+
			"changes the type and keeps the leak:\n%s", raw, body)
	}

	var got recentGrabsResponse
	mustJSON(t, body, &got)
	if len(got.Grabs) != 1 {
		t.Fatalf("got %d grabs, want 1", len(got.Grabs))
	}
	if _, err := strconv.ParseInt(got.Grabs[0].ID, 10, 64); err == nil {
		t.Errorf("the wire id %q parses as an integer; an opaque row key must not, "+
			"or a client will read order into it", got.Grabs[0].ID)
	}
	if got.Grabs[0].ID != grabRowID(s.cfg.GrabRowIDKey, 1, rowID) {
		t.Errorf("the wire id is not the keyed hash of (user_id, rowid): %q", got.Grabs[0].ID)
	}
}

// STABILITY, the first of the two properties the opaque id must have. The client
// keys rows by identity for focus and hover, so an id that changes between polls
// — or across a restart — rebuilds the block under the user's cursor.
//
// The second server stands in for a restart: a fresh process, a fresh database,
// the same derived key. It is the arm that catches the obvious wrong
// implementation, which is a random token minted per response.
func TestGrabRowIDIsStableAcrossServerInstances(t *testing.T) {
	first := newTestServer(t, nil)
	rowID := seedGrabRow(t, first, 1, "Same.Release-GROUP", grabTestNow, store.AcquisitionConfirmed)

	idOf := func(s *Server) string {
		t.Helper()
		_, body := callRecentGrabs(t, s, "")
		var got recentGrabsResponse
		mustJSON(t, body, &got)
		if len(got.Grabs) != 1 {
			t.Fatalf("got %d grabs, want 1: %s", len(got.Grabs), body)
		}
		if got.Grabs[0].ID == "" {
			t.Fatal("the row shipped an empty id, so this test proves nothing")
		}
		return got.Grabs[0].ID
	}

	a, b := idOf(first), idOf(first)
	if a != b {
		t.Errorf("two calls to the same server gave %q then %q; the id is not stable", a, b)
	}

	second := newTestServer(t, nil)
	if got := seedGrabRow(t, second, 1, "Same.Release-GROUP", grabTestNow, store.AcquisitionConfirmed); got != rowID {
		t.Fatalf("the second instance seeded rowid %d, not %d; the two are not the same row", got, rowID)
	}
	if c := idOf(second); c != a {
		t.Errorf("a fresh server on the same derived key gave %q, want %q; the id does not "+
			"survive a restart", c, a)
	}
}

// NO ORDER AND NO VOLUME, the second property, and the one RG-01.3 is actually
// about. An attacker holding two of their own ids must learn nothing about what
// was written between them.
//
// These assertions are on the OUTPUT's statistical shape, not on HMAC-SHA256: a
// truncated hash, a different hash, or a per-user sequence with a random base
// would all pass, and a rowid dressed up in hex or base64 — the tempting cheap
// "fix" — would fail every one of them.
func TestGrabRowIDCarriesNoOrderOrVolume(t *testing.T) {
	key := testGrabRowIDKey(t)

	const rows = 64
	ids := make([]string, 0, rows)
	raw := make([][]byte, 0, rows)
	seen := map[string]int64{}
	for i := int64(1); i <= rows; i++ {
		id := grabRowID(key, 1, i)
		if prev, dup := seen[id]; dup {
			t.Fatalf("rowids %d and %d produced the same id %q", prev, i, id)
		}
		seen[id] = i
		decoded, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil {
			t.Fatalf("id %q is not base64url: %v", id, err)
		}
		if len(decoded) != grabRowIDBytes {
			t.Fatalf("id %q decodes to %d bytes, want %d", id, len(decoded), grabRowIDBytes)
		}
		ids = append(ids, id)
		raw = append(raw, decoded)
	}

	// Sorting the ids must not recover the rowid order. A monotonic encoding —
	// zero-padded decimal, big-endian hex, anything order-preserving — is
	// exactly what this rejects, and it is the shape a well-meaning "just make
	// it a string" change produces.
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	if slices.Equal(sorted, ids) {
		t.Error("sorting the ids reproduces rowid order, so the id still ranks rows")
	}

	// Adjacent rowids must give unrelated ids. Half the bits differ on average
	// for any decent PRF; the band is wide enough that a correct implementation
	// will not flake and narrow enough that a counter with a constant offset —
	// which flips very few bits — cannot pass.
	const bits = grabRowIDBytes * 8
	for i := 1; i < len(raw); i++ {
		d := hammingDistance(raw[i-1], raw[i])
		if d < bits/4 || d > bits*3/4 {
			t.Errorf("rowids %d and %d differ in %d of %d bits; adjacent rows are related",
				i, i+1, d, bits)
		}
		if ids[i-1][:4] == ids[i][:4] {
			t.Errorf("rowids %d and %d share a leading prefix: %q, %q", i, i+1, ids[i-1], ids[i])
		}
	}

	// The GAP between two rowids must not be recoverable. A near neighbour and a
	// distant one have to look equally unrelated, or the difference itself
	// measures how many rows other users wrote in between — which is the oracle
	// RG-01.3 names.
	near := hammingDistance(raw[0], raw[1])
	far := hammingDistance(raw[0], raw[rows-1])
	if near < bits/4 || near > bits*3/4 || far < bits/4 || far > bits*3/4 {
		t.Errorf("distance to the neighbour (%d) and to row %d (%d) are not both ~half of %d bits",
			near, rows, far, bits)
	}

	// Per-user domain separation: the same row seen by two users hashes to two
	// unrelated ids, so a row that is ever visible to both cannot correlate them.
	if grabRowID(key, 1, 7) == grabRowID(key, 2, 7) {
		t.Error("the same rowid gives the same id to two users; user_id is not in the input")
	}
	// The two inputs are fixed-width, so no (user, row) pair can collide with
	// another by running the fields together.
	if grabRowID(key, 1, 23) == grabRowID(key, 12, 3) {
		t.Error("(user 1, row 23) and (user 12, row 3) collide; the inputs are not delimited")
	}

	// A different install key gives different ids for the same row: the id is
	// per-install, so one leaked id says nothing about another deployment.
	other, err := crypto.DeriveGrabRowIDKey(bytes.Repeat([]byte{0x33}, 32), bytes.Repeat([]byte{0x44}, 32))
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if grabRowID(other, 1, 1) == ids[0] {
		t.Error("the id does not depend on the derived key")
	}
}

func hammingDistance(a, b []byte) int {
	n := 0
	for i := range a {
		n += bits.OnesCount8(a[i] ^ b[i])
	}
	return n
}

// jsonKeysAtEveryDepth walks a decoded JSON document and returns every object
// key it finds, however deeply nested. Nesting is the reason it recurses rather
// than reading the top level: a leak inside `grabs[]` is still a leak.
func jsonKeysAtEveryDepth(v any) []string {
	switch node := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(node))
		for key, child := range node {
			keys = append(keys, key)
			keys = append(keys, jsonKeysAtEveryDepth(child)...)
		}
		return keys
	case []any:
		var keys []string
		for _, child := range node {
			keys = append(keys, jsonKeysAtEveryDepth(child)...)
		}
		return keys
	default:
		return nil
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
