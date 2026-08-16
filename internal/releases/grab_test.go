package releases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
)

func storedCandidate(t *testing.T, store *fakeStore, rel servarr.ReleaseResource, now time.Time) int64 {
	t.Helper()
	raw, err := json.Marshal(rel)
	if err != nil {
		t.Fatalf("marshalling fixture: %v", err)
	}
	return store.put(Candidate{
		ServiceInstanceID: testInstanceID,
		GUID:              rel.GUID,
		Title:             rel.Title,
		Indexer:           rel.Indexer,
		IndexerID:         rel.IndexerID,
		Protocol:          string(rel.Protocol),
		DownloadClientID:  rel.DownloadClientID,
		RawReleaseJSON:    raw,
		FetchedAt:         now,
		ExpiresAt:         now.Add(CandidateTTL),
	})
}

func enabledClients() []servarr.DownloadClientResource {
	return []servarr.DownloadClientResource{
		{ID: 1, Name: "sabnzbd", Enable: true, Protocol: servarr.ProtocolUsenet},
		{ID: 2, Name: "qbittorrent", Enable: true, Protocol: servarr.ProtocolTorrent},
	}
}

func TestGrabHappyPathRecordsProvenance(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "Dune.Part.Two.2024.2160p", 1, "Alpha", servarr.ProtocolTorrent, 2000, 2045)
	hash := "d1e2f3"
	rel.InfoHash = &hash
	rel.IndexerFlags = []string{"freeleech"}
	rel.DownloadClientID = servarr.Int32(2)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{clients: enabledClients()}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.Grab(context.Background(), ownerScope, id)
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}
	if got.ReSearched {
		t.Error("no re-search should have been needed")
	}
	if got.ProvenanceID == 0 {
		t.Fatal("no provenance row was recorded")
	}
	if len(store.provenance) != 1 {
		t.Fatalf("got %d provenance rows", len(store.provenance))
	}

	p := store.provenance[0]
	// The raw scene/P2P name, verbatim, forever.
	if p.ReleaseTitle != rel.Title {
		t.Errorf("release_title = %q, want the verbatim name %q", p.ReleaseTitle, rel.Title)
	}
	if p.Protocol != "torrent" || p.SourceSystem != "prowlarr" {
		t.Errorf("provenance = %+v", p)
	}
	// Prowlarr's grab returns no download id, so the torrent infohash IS the join key.
	if p.DownloadID != hash || p.TorrentInfoHash != hash {
		t.Errorf("downloadId/infoHash = %q/%q, want %q", p.DownloadID, p.TorrentInfoHash, hash)
	}
	if len(p.IndexerCategories) != 2 {
		t.Errorf("categories = %v; the raw array must be kept", p.IndexerCategories)
	}
	if p.SourceRecordID != "1_g1" {
		t.Errorf("sourceRecordId = %q, want the Prowlarr cache key shape", p.SourceRecordID)
	}
	if p.PublishedAt == nil {
		t.Error("publishedAt should be carried over")
	}
}

func TestGrabRefusesAnExpiredCandidate(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, store, rel, now.Add(-CandidateTTL-time.Minute))

	c := &fakeClient{clients: enabledClients()}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Grab(context.Background(), ownerScope, id)
	if !errors.Is(err, ErrCandidateExpired) {
		t.Fatalf("err = %v, want ErrCandidateExpired", err)
	}
	// It must not even be attempted: Prowlarr's cache has already dropped it, and a
	// pointless POST would surface as a confusing 404.
	if c.grabCalls != 0 {
		t.Errorf("made %d grab calls for an expired candidate", c.grabCalls)
	}
}

// The 500 + .NET stack trace this preflight exists to prevent.
func TestGrabPreflightsDownloadClients(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{
		// Only a usenet client is enabled, and the torrent one is disabled.
		clients: []servarr.DownloadClientResource{
			{ID: 1, Name: "sabnzbd", Enable: true, Protocol: servarr.ProtocolUsenet},
			{ID: 2, Name: "qbittorrent", Enable: false, Protocol: servarr.ProtocolTorrent},
		},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Grab(context.Background(), ownerScope, id)
	if !errors.Is(err, ErrNoDownloadClient) {
		t.Fatalf("err = %v, want ErrNoDownloadClient", err)
	}
	if c.grabCalls != 0 {
		t.Error("the grab was attempted despite the preflight failing")
	}
	// The message has to be actionable, not a stack trace.
	if got := err.Error(); got == "" || !strings.Contains(got, "Prowlarr") {
		t.Errorf("error = %q; it must tell the user what to do", got)
	}
}

// Prowlarr failures are soft: a flaky secondary probe must not block a grab that
// would otherwise work.
// TestGrabPreflightRoutesByDownloadClientIDNotProtocol is the regression test
// for GRAB-01, in both directions.
//
// Prowlarr routes a grab by downloadClientId when the release names one, and
// consults the protocol ONLY on the other branch (DownloadService.cs:
// `downloadClientId.HasValue ? _downloadClientProvider.Get(id) : GetDownloadClient(protocol, indexerId)`).
// The preflight checked the protocol unconditionally, so it was wrong twice:
// it passed releases whose named client no longer exists — where Prowlarr's
// `.Single()` throws InvalidOperationException, which is NOT one of the three
// DownloadClientUnavailableException messages errors.go maps, so the user got a
// bare 500 with a raw .NET message — and it refused releases whose named client
// was perfectly fine, merely because nothing matched the protocol.
func TestGrabPreflightRoutesByDownloadClientIDNotProtocol(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()

	t.Run("refuses a stale download client id the protocol check would have passed", func(t *testing.T) {
		store := newFakeStore()
		rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
		// The release names client 9, which Prowlarr no longer has. A torrent
		// client IS enabled, so the old protocol-only preflight passed this
		// straight through to a 500.
		rel.DownloadClientID = servarr.Int32(9)
		id := storedCandidate(t, store, rel, now)

		c := &fakeClient{clients: enabledClients()}
		svc, err := NewService(Config{
			InstanceID: testInstanceID, Client: c, Store: store,
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		_, err = svc.Grab(context.Background(), ownerScope, id)
		if !errors.Is(err, ErrNoDownloadClient) {
			t.Fatalf("err = %v, want ErrNoDownloadClient — the named client is gone, and "+
				"Prowlarr would answer with a generic 500", err)
		}
		if c.grabCalls != 0 {
			t.Error("the grab was attempted against a download client Prowlarr does not have")
		}
		if got := err.Error(); !strings.Contains(got, "search again") {
			t.Errorf("error = %q; re-searching is the actual recovery, so say so", got)
		}
	})

	t.Run("refuses a named client that exists but is disabled", func(t *testing.T) {
		store := newFakeStore()
		rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
		rel.DownloadClientID = servarr.Int32(2)
		id := storedCandidate(t, store, rel, now)

		c := &fakeClient{clients: []servarr.DownloadClientResource{
			{ID: 1, Name: "sabnzbd", Enable: true, Protocol: servarr.ProtocolUsenet},
			{ID: 2, Name: "qbittorrent", Enable: false, Protocol: servarr.ProtocolTorrent},
		}}
		svc, err := NewService(Config{
			InstanceID: testInstanceID, Client: c, Store: store,
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		if _, err := svc.Grab(context.Background(), ownerScope, id); !errors.Is(err, ErrNoDownloadClient) {
			t.Fatalf("err = %v, want ErrNoDownloadClient", err)
		}
	})

	t.Run("allows a named client even when no client matches the protocol", func(t *testing.T) {
		store := newFakeStore()
		rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
		// Names client 1, which is enabled. No TORRENT client is enabled, so the
		// old protocol-only preflight refused this grab — but Prowlarr never
		// looks at the protocol on the by-id branch, so it would have worked.
		rel.DownloadClientID = servarr.Int32(1)
		id := storedCandidate(t, store, rel, now)

		c := &fakeClient{clients: []servarr.DownloadClientResource{
			{ID: 1, Name: "sabnzbd", Enable: true, Protocol: servarr.ProtocolUsenet},
		}}
		svc, err := NewService(Config{
			InstanceID: testInstanceID, Client: c, Store: store,
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		if _, err := svc.Grab(context.Background(), ownerScope, id); err != nil {
			t.Fatalf("Grab refused a release naming an enabled client: %v", err)
		}
		if c.grabCalls != 1 {
			t.Errorf("grabCalls = %d, want 1", c.grabCalls)
		}
	})
}

func TestGrabProceedsWhenTheDownloadClientProbeFails(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{clientsErr: errBoom}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.Grab(context.Background(), ownerScope, id); err != nil {
		t.Fatalf("Grab: %v", err)
	}
}

// The cache-miss recovery. A Prowlarr restart is enough to lose the entry, and
// re-running the search is what puts it back.
func TestGrabRecoversFromCacheMissByReSearching(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "Dune.Part.Two.2024.1080p", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	rel.DownloadClientID = servarr.Int32(2)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{
		clients:         enabledClients(),
		grabResponses:   []error{&servarr.APIError{Op: "Grab", Status: 404, Err: servarr.ErrNotFound}},
		searchByIndexer: map[int32][]servarr.ReleaseResource{1: {rel}},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.Grab(context.Background(), ownerScope, id)
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}
	if !got.ReSearched {
		t.Error("the recovery should be reported")
	}
	if c.grabCalls != 2 {
		t.Errorf("made %d grab calls, want 2 (the retry)", c.grabCalls)
	}

	calls := c.calls()
	if len(calls) != 1 {
		t.Fatalf("made %d searches, want exactly one re-search", len(calls))
	}
	// The re-search is by title, pinned to the one indexer.
	if calls[0].query != rel.Title {
		t.Errorf("re-search query = %q, want the release title", calls[0].query)
	}
	if len(calls[0].indexerIDs) != 1 || calls[0].indexerIDs[0] != 1 {
		t.Errorf("re-search indexerIds = %v, want just the owning indexer", calls[0].indexerIDs)
	}
	if len(store.provenance) != 1 {
		t.Error("provenance should still be recorded after a recovered grab")
	}
}

// Exactly one retry. If the release is genuinely gone, repeating will not help.
func TestGrabGivesUpWhenTheReSearchDoesNotFindIt(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{
		clients:       enabledClients(),
		grabResponses: []error{&servarr.APIError{Op: "Grab", Status: 404, Err: servarr.ErrNotFound}},
		// The re-search comes back with a different release.
		searchByIndexer: map[int32][]servarr.ReleaseResource{
			1: {release("some-other-guid", "B", 1, "Alpha", servarr.ProtocolTorrent, 2000)},
		},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Grab(context.Background(), ownerScope, id)
	if !errors.Is(err, ErrGrabCacheMiss) {
		t.Fatalf("err = %v, want ErrGrabCacheMiss", err)
	}
	if c.grabCalls != 1 {
		t.Errorf("made %d grab calls; there must be no second retry", c.grabCalls)
	}
	if len(store.provenance) != 0 {
		t.Error("a failed grab must not record provenance")
	}
}

func TestGrabMapsUpstreamNoDownloadClientError(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{
		clients: enabledClients(), // the preflight passes, the grab still 500s
		grabResponses: []error{&servarr.APIError{
			Op: "Grab", Status: 500, Err: servarr.ErrNoDownloadClient,
			Message: "Torrent Download client isn't configured yet",
		}},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.Grab(context.Background(), ownerScope, id); !errors.Is(err, ErrNoDownloadClient) {
		t.Fatalf("err = %v, want ErrNoDownloadClient", err)
	}
}

// TestGrabClassifiesAValidationRejectionAsOurBug pins whose fault a 400 is.
//
// Nothing a user types reaches the grab body — it is built server-side from a
// stored release — so a 400 from the model binder is always UsArr's own defect.
// Reporting it as a bad gateway (which is what happened when the body carried
// `"protocol":""`) blames a Prowlarr that answered correctly and offers a
// "Test connection" button for a connection that works.
func TestGrabClassifiesAValidationRejectionAsOurBug(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{
		clients: enabledClients(),
		grabResponses: []error{&servarr.APIError{
			Op: "Grab", Method: "POST", Path: "/api/v1/search", Status: 400, Err: servarr.ErrValidation,
			Message: "One or more validation errors occurred.",
			Validation: []servarr.ValidationFailure{{
				PropertyName: "$.protocol",
				ErrorMessage: "The JSON value could not be converted to NzbDrone.Core.Indexers.DownloadProtocol.",
			}},
		}},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = svc.Grab(context.Background(), ownerScope, id)
	if !errors.Is(err, ErrRequestRejected) {
		t.Fatalf("err = %v, want ErrRequestRejected", err)
	}
	// The upstream text survives: it names the property, which is the only part
	// of the message that says where to look.
	if !strings.Contains(err.Error(), "$.protocol") {
		t.Errorf("the upstream detail was dropped: %v", err)
	}
}

func TestGrabEnforcesTheAccessScope(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, store, rel, now)

	c := &fakeClient{clients: enabledClients()}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	other := Scope{UserID: 2, InstanceIDs: []int64{999}}
	if _, err := svc.Grab(context.Background(), other, id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if c.grabCalls != 0 {
		t.Error("an out-of-scope grab reached the upstream")
	}
}

func TestGrabRejectsACandidateFromAnotherInstance(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	raw, _ := json.Marshal(rel)
	id := store.put(Candidate{
		ServiceInstanceID: 42, // a different Prowlarr
		GUID:              rel.GUID, Title: rel.Title, IndexerID: 1,
		RawReleaseJSON: raw, FetchedAt: now, ExpiresAt: now.Add(CandidateTTL),
	})

	c := &fakeClient{clients: enabledClients()}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: store,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	// The scope covers instance 7 and 42, but this Service fronts 7.
	scope := Scope{UserID: 1, InstanceIDs: []int64{testInstanceID, 42}}
	if _, err := svc.Grab(context.Background(), scope, id); !errors.Is(err, ErrCandidateNotFound) {
		t.Fatalf("err = %v, want ErrCandidateNotFound", err)
	}
}
