package releases

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
)

func newTestService(t *testing.T, c *fakeClient, st Store) *Service {
	t.Helper()
	svc, err := NewService(Config{
		InstanceID:        testInstanceID,
		Client:            c,
		Store:             st,
		PerIndexerTimeout: 200 * time.Millisecond,
		OverallTimeout:    5 * time.Second,
		Concurrency:       4,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestSearchRefusesOutOfScopeInstance(t *testing.T) {
	svc := newTestService(t, &fakeClient{}, newFakeStore())
	_, err := svc.Search(context.Background(), Scope{UserID: 2, InstanceIDs: []int64{99}}, Query{Text: "dune"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

// One request per indexer, not one aggregate request. That is the only way to get
// progressive results out of Prowlarr, which gives no per-indexer progress.
func TestSearchFansOutOnePerIndexer(t *testing.T) {
	c := &fakeClient{
		indexers: []servarr.IndexerResource{
			indexer(1, "Alpha", 10, servarr.ProtocolTorrent),
			indexer(2, "Beta", 25, servarr.ProtocolUsenet),
		},
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("g1", "A.2024.1080p", 1, "Alpha", servarr.ProtocolTorrent, 2000)},
			2: {release("g2", "B.2024.1080p", 2, "Beta", servarr.ProtocolUsenet, 2000)},
		},
	}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune", Limit: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, outcomes, report := collect(ch)

	if len(c.calls()) != 2 {
		t.Fatalf("made %d search calls, want one per indexer", len(c.calls()))
	}
	for _, call := range c.calls() {
		if len(call.indexerIDs) != 1 {
			t.Errorf("a leg addressed %v; the fan-out must address one indexer per request", call.indexerIDs)
		}
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if report == nil {
		t.Fatal("no Done event")
	}
	if report.Answered != 2 || report.Failed != 0 || report.Skipped != 0 {
		t.Errorf("report = %+v", report)
	}
	if report.Degraded() {
		t.Error("a fully successful search must not be reported as degraded")
	}
	if len(outcomes) != 2 {
		t.Errorf("got %d outcomes", len(outcomes))
	}

	// Every result carries its derived tags immediately, with no library behind
	// them. That is the whole point of Search-and-Grab mode.
	var sawSource, sawType, sawIndexer bool
	for _, tag := range results[0].Tags {
		switch tag.Namespace {
		case "source":
			sawSource = true
		case "type":
			sawType = true
		case "indexer":
			sawIndexer = true
		}
	}
	if !sawSource || !sawType || !sawIndexer {
		t.Errorf("tags = %v; want source:, type: and indexer:", results[0].Tags)
	}
}

// The failure mode this whole package exists to prevent: Prowlarr returns 200
// with [] when everything is down, and a naive client renders an empty screen.
func TestSearchDistinguishesNoResultsFromEverythingBroken(t *testing.T) {
	c := &fakeClient{
		indexers: []servarr.IndexerResource{
			indexer(1, "Alpha", 10, servarr.ProtocolTorrent),
			indexer(2, "Beta", 25, servarr.ProtocolUsenet),
			indexer(4, "Flaky", 30, servarr.ProtocolTorrent),
		},
		statuses: []servarr.IndexerStatusResource{
			{ID: 1, IndexerID: 4, DisabledTill: ptrTime(time.Unix(1_800_000_000, 0).UTC())},
		},
		// Alpha genuinely has nothing; Beta is broken.
		searchByIndexer: map[int32][]servarr.ReleaseResource{1: nil},
		searchErrs:      map[int32]error{2: &servarr.APIError{Op: "Search", Err: servarr.ErrServer, Message: "upstream 502"}},
	}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, outcomes, report := collect(ch)

	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
	if report == nil {
		t.Fatal("no Done event")
	}
	if !report.Degraded() {
		t.Fatal("a search where one indexer failed and one is blocked is degraded")
	}
	if report.TotalIndexers != 3 || report.Answered != 1 || report.Failed != 1 || report.Skipped != 1 {
		t.Errorf("report = %+v; want 1 of 3 answered, 1 failed, 1 skipped", report)
	}

	// The blocked indexer must be SKIPPED, not queried: it is blocked precisely
	// because it has been timing out.
	for _, call := range c.calls() {
		if len(call.indexerIDs) == 1 && call.indexerIDs[0] == 4 {
			t.Error("a blocked indexer was queried anyway")
		}
	}
	blocked, ok := outcomeFor(outcomes, 4)
	if !ok || blocked.Status != OutcomeBlocked {
		t.Fatalf("indexer 4 outcome = %+v, want blocked", blocked)
	}
	if blocked.BlockedUntil == nil || !strings.Contains(blocked.Reason, "blocked until") {
		t.Errorf("blocked reason = %q; the UI needs to be able to say when", blocked.Reason)
	}

	// The one that answered honestly with nothing is distinguishable from the one
	// that failed.
	alpha, _ := outcomeFor(outcomes, 1)
	if alpha.Status != OutcomeOK || !strings.Contains(alpha.Reason, "no matches") {
		t.Errorf("indexer 1 outcome = %+v, want ok with a stated no-matches reason", alpha)
	}
	beta, _ := outcomeFor(outcomes, 2)
	if beta.Status != OutcomeFailed {
		t.Errorf("indexer 2 outcome = %+v, want failed", beta)
	}
}

func TestSearchSkipsDisabledAndUnsupportedIndexersWithReasons(t *testing.T) {
	rssOnly := indexer(2, "RSS Only", 20, servarr.ProtocolTorrent)
	rssOnly.SupportsSearch = false

	disabled := indexer(3, "Retired", 50, servarr.ProtocolTorrent)
	disabled.Enable = false

	wrongCats := indexer(5, "Books Only", 15, servarr.ProtocolTorrent)
	wrongCats.Capabilities = caps(50, 7000, 7020)

	noMovieMode := indexer(6, "No Movie Mode", 12, servarr.ProtocolTorrent)
	noMovieMode.Capabilities.MovieSearchParams = nil

	c := &fakeClient{
		indexers: []servarr.IndexerResource{
			indexer(1, "Alpha", 10, servarr.ProtocolTorrent), rssOnly, disabled, wrongCats, noMovieMode,
		},
		searchByIndexer: map[int32][]servarr.ReleaseResource{1: nil},
	}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{
		Text: "dune", Type: servarr.SearchTypeMovie, Categories: []int32{2000},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	_, outcomes, report := collect(ch)

	if report.Skipped != 4 {
		t.Errorf("skipped = %d, want 4", report.Skipped)
	}
	for id, wantStatus := range map[int32]OutcomeStatus{
		2: OutcomeUnsupported, 3: OutcomeDisabled, 5: OutcomeUnsupported, 6: OutcomeUnsupported,
	} {
		o, ok := outcomeFor(outcomes, id)
		if !ok {
			t.Errorf("indexer %d has no outcome", id)
			continue
		}
		if o.Status != wantStatus {
			t.Errorf("indexer %d status = %s, want %s", id, o.Status, wantStatus)
		}
		if o.Reason == "" {
			t.Errorf("indexer %d was skipped with no stated reason", id)
		}
	}
}

// Prowlarr's own rule: dedupe by guid, keep the lowest-priority-value indexer's
// copy. Because results stream, a better copy can arrive after a worse one has
// already been emitted, so the stream carries an explicit supersede.
func TestSearchDedupesAcrossIndexersAndSupersedes(t *testing.T) {
	slow := indexer(1, "Better", 5, servarr.ProtocolTorrent)
	fast := indexer(2, "Worse", 40, servarr.ProtocolTorrent)

	c := &fakeClient{
		indexers: []servarr.IndexerResource{slow, fast},
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("shared-guid", "From.Better", 1, "Better", servarr.ProtocolTorrent, 2000)},
			2: {release("shared-guid", "From.Worse", 2, "Worse", servarr.ProtocolTorrent, 2000)},
		},
		// Make the better indexer answer last, which is the case the supersede
		// mechanism exists for.
		searchDelays: map[int32]time.Duration{1: 60 * time.Millisecond},
	}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, _, report := collect(ch)

	if len(results) != 2 {
		t.Fatalf("got %d results; the worse copy is emitted then superseded, so both cross the stream", len(results))
	}
	last := results[len(results)-1]
	if last.Title != "From.Better" {
		t.Errorf("final result = %q, want the priority-5 indexer's copy", last.Title)
	}
	if last.SupersedesCandidateID != results[0].CandidateID {
		t.Errorf("supersedes = %d, want %d", last.SupersedesCandidateID, results[0].CandidateID)
	}
	_ = report
}

func TestSearchDoesNotSupersedeWithAWorseCopy(t *testing.T) {
	c := &fakeClient{
		indexers: []servarr.IndexerResource{
			indexer(1, "Better", 5, servarr.ProtocolTorrent),
			indexer(2, "Worse", 40, servarr.ProtocolTorrent),
		},
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("shared-guid", "From.Better", 1, "Better", servarr.ProtocolTorrent, 2000)},
			2: {release("shared-guid", "From.Worse", 2, "Worse", servarr.ProtocolTorrent, 2000)},
		},
		searchDelays: map[int32]time.Duration{2: 60 * time.Millisecond},
	}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, _, _ := collect(ch)
	if len(results) != 1 || results[0].Title != "From.Better" {
		t.Fatalf("got %+v; the worse copy must be dropped, not emitted", titles(results))
	}
}

func TestSearchTimesOutOneIndexerWithoutStallingTheRest(t *testing.T) {
	c := &fakeClient{
		indexers: []servarr.IndexerResource{
			indexer(1, "Alpha", 10, servarr.ProtocolTorrent),
			indexer(2, "Molasses", 20, servarr.ProtocolTorrent),
		},
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)},
		},
		// Longer than the 200 ms per-indexer deadline, shorter than the overall one.
		searchDelays: map[int32]time.Duration{2: 2 * time.Second},
	}
	svc := newTestService(t, c, newFakeStore())

	start := time.Now()
	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, outcomes, report := collect(ch)
	elapsed := time.Since(start)

	if elapsed > 1500*time.Millisecond {
		t.Errorf("search took %s; the per-indexer deadline must cut the slow leg", elapsed)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want the fast indexer's one", len(results))
	}
	slow, _ := outcomeFor(outcomes, 2)
	if slow.Status != OutcomeTimedOut {
		t.Errorf("slow indexer outcome = %+v, want timed_out", slow)
	}
	if !report.Degraded() {
		t.Error("a timed-out indexer makes the result set incomplete")
	}
}

// A failing indexer must not stop the others, and after enough failures it must
// be skipped outright rather than re-timed-out on every search.
func TestPerIndexerBreakersAreIndependentAndSkipAfterRepeatedFailure(t *testing.T) {
	c := &fakeClient{
		indexers: []servarr.IndexerResource{
			indexer(1, "Alpha", 10, servarr.ProtocolTorrent),
			indexer(2, "Broken", 20, servarr.ProtocolTorrent),
		},
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)},
		},
		searchErrs: map[int32]error{2: &servarr.APIError{Op: "Search", Err: servarr.ErrServer}},
	}
	svc := newTestService(t, c, newFakeStore())

	for range 5 {
		ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		results, _, _ := collect(ch)
		if len(results) != 1 {
			t.Fatalf("the healthy indexer stopped answering: %d results", len(results))
		}
	}

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	_, outcomes, _ := collect(ch)
	broken, _ := outcomeFor(outcomes, 2)
	if broken.Status != OutcomeBreakerOpen {
		t.Fatalf("after five failures the outcome = %+v, want breaker_open", broken)
	}
	if !strings.Contains(broken.Reason, "next retry") {
		t.Errorf("reason = %q; the UI needs to say when it will be tried again", broken.Reason)
	}
}

func TestSearchPersistsCandidatesWithTheGrabWindow(t *testing.T) {
	store := newFakeStore()
	c := &fakeClient{
		indexers: []servarr.IndexerResource{indexer(1, "Alpha", 10, servarr.ProtocolTorrent)},
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("g1", "A.2024", 1, "Alpha", servarr.ProtocolTorrent, 2000, 2045)},
		},
	}
	fixed := time.Unix(1_755_000_000, 0).UTC()
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, _, _ := collect(ch)
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}

	const guardGUID = "g1"

	cand, err := store.Candidate(context.Background(), ownerScope, results[0].CandidateID)
	if err != nil {
		t.Fatalf("Candidate: %v", err)
	}
	// 25 minutes against Prowlarr's non-rolling 30-minute cache.
	if got := cand.ExpiresAt.Sub(cand.FetchedAt); got != CandidateTTL {
		t.Errorf("TTL = %s, want %s", got, CandidateTTL)
	}
	if results[0].ExpiresAt != cand.ExpiresAt {
		t.Error("the client-facing result must carry the same expiry the row does")
	}
	// raw_release_json is stored SANITISED. This assertion used to be inverted —
	// it required the credential to be present, on the belief that the grab
	// echoes the resource back verbatim. It does not: Grab sends GrabBody(),
	// which is guid + indexerId + downloadClientId. See TestPersistedBlobNeverCarriesTheAPIKey.
	if strings.Contains(string(cand.RawReleaseJSON), "SECRETKEY") {
		t.Error("raw_release_json must not carry Prowlarr's API key")
	}
	// The blob still has to be usable: the provenance fields are what it is for.
	if !strings.Contains(string(cand.RawReleaseJSON), guardGUID) {
		t.Error("raw_release_json lost the guid the grab is resolved by")
	}
	// It must also never reach the client-facing type. Result embeds
	// mapping.Release, which has no field for it.
	if strings.Contains(resultDebug(results[0]), "SECRETKEY") {
		t.Fatal("a credential reached the client-facing Result")
	}
	// Raw categories survive intact.
	if len(cand.Categories) != 2 || cand.Categories[1] != 2045 {
		t.Errorf("categories = %v, want the raw array", cand.Categories)
	}
}

// TestPersistedBlobNeverCarriesTheAPIKey is the regression test for SEC-01.
//
// Prowlarr's SearchController.MapReleases splices its own full-admin API key
// into downloadUrl and magnetUrl on EVERY search result. persist() used to
// marshal the resource unsanitised, so every release_candidate row wrote that
// key to SQLite in plaintext — in the same file whose service_instance.api_key_enc
// column is AES-256-GCM-enveloped precisely to stop that, and therefore in every
// VACUUM INTO backup, every support bundle and every restored volume, forever.
//
// The second half of this test is the part that matters most: it grabs the
// candidate that was persisted from the sanitised blob and asserts the grab
// still succeeds with every provenance field intact. That is what makes the fix
// provably safe rather than merely safer — the grab is resolved by
// {indexerId}_{guid} out of Prowlarr's own cache, so the two dropped fields were
// never load-bearing.
func TestPersistedBlobNeverCarriesTheAPIKey(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()

	rel := release("g1", "Dune.Part.Two.2024.2160p", 1, "Alpha", servarr.ProtocolTorrent, 2000, 2045)
	hash := "d1e2f3"
	rel.InfoHash = &hash
	rel.IndexerFlags = []string{"freeleech"}
	rel.InfoURL = "https://alpha.test/details/1"
	rel.DownloadClientID = servarr.Int32(2)
	// Both credential-bearing fields populated, as Prowlarr populates them.
	magnet := "http://prowlarr.test:9696/1/download?apikey=SECRETKEY&link=magnet"
	rel.MagnetURL = &magnet

	c := &fakeClient{
		indexers:        []servarr.IndexerResource{indexer(1, "Alpha", 10, servarr.ProtocolTorrent)},
		clients:         enabledClients(),
		searchByIndexer: map[int32][]servarr.ReleaseResource{1: {rel}},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, _, _ := collect(ch)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	cand, err := store.Candidate(context.Background(), ownerScope, results[0].CandidateID)
	if err != nil {
		t.Fatalf("Candidate: %v", err)
	}
	blob := string(cand.RawReleaseJSON)

	// The key itself, and the parameter that carries it, in either field.
	for _, forbidden := range []string{"SECRETKEY", "apikey=", "downloadUrl", "magnetUrl"} {
		if strings.Contains(blob, forbidden) {
			t.Errorf("raw_release_json contains %q — Prowlarr's admin API key is being "+
				"written to SQLite in plaintext (SEC-01). persist() must marshal "+
				"servarr.SanitizeRelease(rel), not rel.\nblob: %s", forbidden, blob)
		}
	}

	// Everything the blob is actually stored FOR must have survived.
	for _, required := range []string{"g1", "Dune.Part.Two.2024.2160p", "d1e2f3", "freeleech", "2045"} {
		if !strings.Contains(blob, required) {
			t.Errorf("raw_release_json lost %q — sanitising must drop the two credential "+
				"fields and nothing else", required)
		}
	}

	// ---- the grab still works from the sanitised blob -----------------------

	got, err := svc.Grab(context.Background(), ownerScope, cand.ID)
	if err != nil {
		t.Fatalf("Grab from a sanitised candidate failed: %v\n"+
			"this would mean the blob really is needed verbatim; it is not — "+
			"Client.Grab sends GrabBody() (guid + indexerId + downloadClientId)", err)
	}
	if got.ReleaseTitle != "Dune.Part.Two.2024.2160p" {
		t.Errorf("ReleaseTitle = %q", got.ReleaseTitle)
	}
	if got.DownloadClientID == nil || *got.DownloadClientID != 2 {
		t.Errorf("DownloadClientID = %v, want 2 — the field the grab body actually honours", got.DownloadClientID)
	}
	if got.ProvenanceID == 0 {
		t.Fatal("no provenance row was recorded")
	}

	// The three fields the grab body is built from reached the client.
	if len(c.grabbed) != 1 {
		t.Fatalf("Grab called %d times, want 1", len(c.grabbed))
	}
	sent := c.grabbed[0]
	if sent.GUID != "g1" || sent.IndexerID != 1 || sent.DownloadClientID == nil || *sent.DownloadClientID != 2 {
		t.Errorf("grab body inputs lost: guid=%q indexerId=%d clientId=%v",
			sent.GUID, sent.IndexerID, sent.DownloadClientID)
	}
	// And the credential did not survive the round trip to reappear on the wire.
	if sent.DownloadURL != nil || sent.MagnetURL != nil {
		t.Error("a decoded stored release still carries downloadUrl/magnetUrl")
	}

	// Provenance keeps every field it reads off the decoded release.
	if len(store.provenance) != 1 {
		t.Fatalf("got %d provenance rows, want 1", len(store.provenance))
	}
	p := store.provenance[0]
	if p.ReleaseGUID != "g1" || p.IndexerName != "Alpha" || p.IndexerID != 1 ||
		p.TorrentInfoHash != "d1e2f3" || p.NZBInfoURL != "https://alpha.test/details/1" ||
		len(p.IndexerCategories) != 2 || len(p.IndexerFlags) != 1 {
		t.Errorf("provenance lost a field it reads off the stored release: %+v", p)
	}
	// There is deliberately no assertion that provenance.download_url is empty:
	// releases.Provenance has no such field, so it is a compile-time guarantee
	// rather than a runtime one. Adding the field back breaks this file.
}

// TestPersistedCandidateNeverCarriesATrackerPasskey is the sibling of the test
// above, for the OTHER credential in a search result.
//
// infoUrl, commentUrl and posterUrl are INDEXER-supplied, and a private tracker
// routinely carries the user's personal passkey in them as a query parameter. A
// leaked passkey is account termination on a private tracker, because it is what
// the tracker attributes traffic by.
//
// Redaction used to happen only at the HTTP boundary (httpapi.redactURLField),
// so the API responses were clean while SQLite held the passkey verbatim — in
// release_candidate.info_url, inside raw_release_json, and from there in every
// VACUUM INTO backup and every support bundle, permanently. Persistence is the
// strictest boundary, not the most relaxed one.
//
// The assertions are about the STORE, not the wire: what the browser gets was
// already covered.
func TestPersistedCandidateNeverCarriesATrackerPasskey(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()

	const passkey = "d41d8cd98f00b204e9800998ecf8427e"
	rel := release("g1", "Dune.Part.Two.2024.2160p", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	// Every deny-listed name a tracker actually uses, on the three
	// indexer-supplied URL fields.
	rel.InfoURL = "https://alpha.test/details/1?passkey=" + passkey + "&torrent_pass=" + passkey
	rel.CommentURL = "https://alpha.test/comments/1?authkey=" + passkey + "&rsskey=" + passkey
	rel.PosterURL = "https://alpha.test/cover/1.jpg?apikey=" + passkey

	c := &fakeClient{
		indexers:        []servarr.IndexerResource{indexer(1, "Alpha", 10, servarr.ProtocolTorrent)},
		clients:         enabledClients(),
		searchByIndexer: map[int32][]servarr.ReleaseResource{1: {rel}},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, _, _ := collect(ch)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	cand, err := store.Candidate(context.Background(), ownerScope, results[0].CandidateID)
	if err != nil {
		t.Fatalf("Candidate: %v", err)
	}

	// The passkey value must appear in NOTHING that was persisted: not the
	// column, not the blob.
	persisted := map[string]string{
		"release_candidate.info_url": cand.InfoURL,
		"raw_release_json":           string(cand.RawReleaseJSON),
	}
	for what, got := range persisted {
		if strings.Contains(got, passkey) {
			t.Errorf("%s contains the tracker passkey verbatim — it is now in SQLite, "+
				"in every VACUUM INTO backup and every support bundle, permanently. "+
				"persist() must redact indexer-supplied URLs through "+
				"servarr.SanitizeRelease / servarr.RedactURL.\ngot: %s", what, got)
		}
	}

	// Redacted, not dropped: the host and the release path are the provenance
	// this column exists for, and only the secret had to go.
	if !strings.Contains(cand.InfoURL, "alpha.test/details/1") {
		t.Errorf("release_candidate.info_url lost the tracker and the release path: %q", cand.InfoURL)
	}
	if !strings.Contains(cand.InfoURL, "REDACTED") {
		t.Errorf("release_candidate.info_url = %q, want the deny-listed parameters replaced "+
			"with the shared placeholder", cand.InfoURL)
	}

	// ---- and the passkey must not reach provenance, which is FOREVER ---------

	if _, err := svc.Grab(context.Background(), ownerScope, cand.ID); err != nil {
		t.Fatalf("Grab: %v", err)
	}
	if len(store.provenance) != 1 {
		t.Fatalf("got %d provenance rows, want 1", len(store.provenance))
	}
	p := store.provenance[0]
	if strings.Contains(p.NZBInfoURL, passkey) {
		t.Errorf("provenance.nzb_info_url carries the tracker passkey: %q\n"+
			"provenance rows are immutable and permanent — this one can never be "+
			"deleted", p.NZBInfoURL)
	}
	if !strings.Contains(p.NZBInfoURL, "alpha.test/details/1") {
		t.Errorf("provenance.nzb_info_url lost the indexer and release path: %q", p.NZBInfoURL)
	}
}

func TestSearchClampsLimitToIndexerCapabilities(t *testing.T) {
	small := indexer(1, "Small", 10, servarr.ProtocolTorrent)
	small.Capabilities = caps(20, 2000)
	small.SupportsPagination = false

	big := indexer(2, "Big", 20, servarr.ProtocolTorrent)
	big.Capabilities = caps(500, 2000)
	big.SupportsPagination = true

	c := &fakeClient{indexers: []servarr.IndexerResource{small, big}}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune", Limit: 200, Offset: 100})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	collect(ch)

	for _, call := range c.calls() {
		switch call.indexerIDs[0] {
		case 1:
			if call.limit == nil || *call.limit != 20 {
				t.Errorf("limit for the limitsMax=20 indexer = %v, want 20", call.limit)
			}
			// Sending an offset to an indexer that does not advertise pagination
			// silently re-returns page 1 on some indexers.
			if call.offset != nil {
				t.Errorf("offset %v sent to an indexer with supportsPagination=false", *call.offset)
			}
		case 2:
			if call.limit == nil || *call.limit != 200 {
				t.Errorf("limit for the limitsMax=500 indexer = %v, want 200", call.limit)
			}
			if call.offset == nil || *call.offset != 100 {
				t.Errorf("offset = %v, want 100", call.offset)
			}
		}
	}
}

// A category asked for at the HTTP boundary has to survive all the way onto the
// wire, and in the ONE form Prowlarr's model binder accepts.
//
// Both halves matter. Prowlarr binds `categories` as a repeated parameter;
// comma-joining them into `categories=2000,5030` makes the binder fail or
// silently bind nothing, which downgrades a category-scoped search to an
// unfiltered one with no error anywhere. Asserting on the SearchRequest struct
// would prove neither half — hence the assertion on the encoded values.
func TestSearchSendsCategoriesAsRepeatedQueryParameters(t *testing.T) {
	c := &fakeClient{indexers: []servarr.IndexerResource{
		indexer(1, "Alpha", 10, servarr.ProtocolTorrent),
		indexer(2, "Beta", 20, servarr.ProtocolUsenet),
	}}
	svc := newTestService(t, c, newFakeStore())

	// 2000 and 5000 are both advertised by the indexer() fixture, so neither leg
	// can be skipped as category-unsupported.
	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune", Categories: []int32{2000, 5000}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	collect(ch)

	calls := c.calls()
	if len(calls) != 2 {
		t.Fatalf("made %d search calls, want one per indexer", len(calls))
	}
	for _, call := range calls {
		raw := call.values["categories"]
		if len(raw) != 2 || raw[0] != "2000" || raw[1] != "5000" {
			t.Errorf("encoded categories = %#v, want two REPEATED categories= parameters [\"2000\" \"5000\"]; "+
				"a single comma-joined value is bound as nothing by Prowlarr", raw)
		}
		if len(call.categories) != 2 || call.categories[0] != 2000 || call.categories[1] != 5000 {
			t.Errorf("decoded categories = %v, want [2000 5000]", call.categories)
		}
	}
}

func TestSearchBuildsTypedCriteriaTokens(t *testing.T) {
	c := &fakeClient{indexers: []servarr.IndexerResource{indexer(1, "Alpha", 10, servarr.ProtocolTorrent)}}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{
		Text:     "The Simpsons",
		Type:     servarr.SearchTypeTV,
		Criteria: servarr.Criteria{Season: "1", Episode: "1"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	collect(ch)

	calls := c.calls()
	if len(calls) != 1 {
		t.Fatalf("got %d calls", len(calls))
	}
	if want := "The Simpsons{Season:1}{Episode:1}"; calls[0].query != want {
		t.Errorf("query = %q, want %q", calls[0].query, want)
	}
}

func TestSearchRejectsInvalidQueryBeforeAnyRequest(t *testing.T) {
	c := &fakeClient{indexers: []servarr.IndexerResource{indexer(1, "Alpha", 10, servarr.ProtocolTorrent)}}
	svc := newTestService(t, c, newFakeStore())

	for _, q := range []Query{
		{Text: ""},
		{Text: "x", Type: servarr.SearchType("tv")},                                      // not one of the five
		{Text: "x", Type: servarr.SearchTypeTV, Criteria: servarr.Criteria{Artist: "y"}}, // wrong criterion for the type
	} {
		if _, err := svc.Search(context.Background(), ownerScope, q); err == nil {
			t.Errorf("query %+v was accepted", q)
		}
	}
	if len(c.calls()) != 0 {
		t.Errorf("an invalid query made %d requests", len(c.calls()))
	}
}

// The blocked-indexer report is a nicety; a failure to fetch it must not fail the
// search. Prowlarr failures are soft.
func TestSearchSurvivesIndexerStatusFailure(t *testing.T) {
	c := &fakeClient{
		indexers:  []servarr.IndexerResource{indexer(1, "Alpha", 10, servarr.ProtocolTorrent)},
		statusErr: errBoom,
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)},
		},
	}
	svc := newTestService(t, c, newFakeStore())
	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, _, _ := collect(ch)
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
}

func TestSearchFailsWhenIndexersCannotBeListed(t *testing.T) {
	c := &fakeClient{indexersErr: errBoom}
	svc := newTestService(t, c, newFakeStore())
	if _, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"}); err == nil {
		t.Fatal("listing indexers failed but Search returned no error")
	}
}

func TestSearchReportsPersistenceFailureRatherThanShowingUngrabbableResults(t *testing.T) {
	store := newFakeStore()
	store.insertErr = errBoom
	c := &fakeClient{
		indexers: []servarr.IndexerResource{indexer(1, "Alpha", 10, servarr.ProtocolTorrent)},
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)},
		},
	}
	svc := newTestService(t, c, store)
	ch, err := svc.Search(context.Background(), ownerScope, Query{Text: "dune"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results, outcomes, _ := collect(ch)
	if len(results) != 0 {
		t.Error("an unstored release cannot be grabbed and must not be shown as grabbable")
	}
	o, _ := outcomeFor(outcomes, 1)
	if o.Status != OutcomeFailed || !strings.Contains(o.Reason, "store") {
		t.Errorf("outcome = %+v, want a failure that says so", o)
	}
}

func TestSearchResolvesMagicIndexerIDs(t *testing.T) {
	c := &fakeClient{
		indexers: []servarr.IndexerResource{
			indexer(1, "Torrent", 10, servarr.ProtocolTorrent),
			indexer(2, "Usenet", 20, servarr.ProtocolUsenet),
		},
	}
	svc := newTestService(t, c, newFakeStore())

	ch, err := svc.Search(context.Background(), ownerScope, Query{
		Text: "dune", IndexerIDs: []int32{servarr.AllUsenetIndexers},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	collect(ch)

	calls := c.calls()
	if len(calls) != 1 || calls[0].indexerIDs[0] != 2 {
		t.Errorf("-1 (all usenet) resolved to %v, want just the usenet indexer", calls)
	}
}

func TestEvictExpired(t *testing.T) {
	store := newFakeStore()
	now := time.Unix(1_755_000_000, 0).UTC()
	store.put(Candidate{ServiceInstanceID: testInstanceID, ExpiresAt: now.Add(-time.Minute)})
	store.put(Candidate{ServiceInstanceID: testInstanceID, ExpiresAt: now.Add(time.Minute)})

	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: &fakeClient{}, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	n, err := svc.EvictExpired(context.Background())
	if err != nil {
		t.Fatalf("EvictExpired: %v", err)
	}
	if n != 1 {
		t.Errorf("evicted %d, want 1", n)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func titles(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Title
	}
	return out
}

// resultDebug renders a Result the way a careless log line or SSE encoder would.
func resultDebug(r Result) string {
	var b strings.Builder
	b.WriteString(r.GUID)
	b.WriteString(r.Title)
	b.WriteString(r.InfoURL)
	b.WriteString(r.PosterURL)
	b.WriteString(r.CommentURL)
	b.WriteString(r.InfoHash)
	return b.String()
}
