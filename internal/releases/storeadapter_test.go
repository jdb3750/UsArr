package releases

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/store"
)

// One round trip through the real SQLite store, because everything else in this
// package runs against a fake and a fake cannot tell you that the two halves
// actually compose. It is the adapter that is under test here, not the flow.
func newRealStore(t *testing.T) (*StoreAdapter, int64) {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "usarr.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	s := store.New(d)
	instanceID, err := s.CreateServiceInstance(t.Context(), store.ServiceInstance{
		Kind: "prowlarr", Role: "indexer", Name: "Prowlarr",
		BaseURL: "http://prowlarr.lan:9696", APIKeyEnc: []byte{1}, KEKID: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("seed service_instance: %v", err)
	}
	return NewStoreAdapter(s), instanceID
}

func TestStoreAdapterRoundTrip(t *testing.T) {
	ctx := t.Context()
	adapter, instanceID := newRealStore(t)
	scope := Scope{UserID: 1, AllInstances: true}

	now := time.Unix(1_755_000_000, 0).UTC()
	raw, err := json.Marshal(servarr.ReleaseResource{GUID: "guid-1", IndexerID: 3})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ids, err := adapter.InsertCandidates(ctx, []Candidate{{
		ServiceInstanceID: instanceID,
		GUID:              "guid-1",
		Title:             "Some.Movie.2026.1080p.BluRay.x264-GROUP",
		Indexer:           "an-indexer",
		IndexerID:         3,
		Protocol:          "torrent",
		Categories:        []int32{2000, 2045},
		SizeBytes:         8 << 30,
		Seeders:           servarr.Int32(42),
		// Leechers is deliberately nil: the absent/zero distinction has to survive
		// the SQL round trip or a usenet result looks like a dead torrent.
		AgeDays:          1.5,
		InfoURL:          "https://indexer.test/details/1",
		InfoHash:         "deadbeef",
		DownloadClientID: servarr.Int32(2),
		RawReleaseJSON:   raw,
		FetchedAt:        now,
		ExpiresAt:        now.Add(CandidateTTL),
	}})
	if err != nil {
		t.Fatalf("InsertCandidates: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("got %d ids", len(ids))
	}

	got, err := adapter.Candidate(ctx, scope, ids[0])
	if err != nil {
		t.Fatalf("Candidate: %v", err)
	}
	if got.GUID != "guid-1" || got.IndexerID != 3 || got.Protocol != "torrent" {
		t.Errorf("candidate = %+v", got)
	}
	if len(got.Categories) != 2 || got.Categories[1] != 2045 {
		t.Errorf("categories = %v; the raw array must survive the JSON column", got.Categories)
	}
	if got.Seeders == nil || *got.Seeders != 42 {
		t.Errorf("seeders = %v", got.Seeders)
	}
	if got.Leechers != nil {
		t.Errorf("leechers = %v, want nil (absent, not zero)", *got.Leechers)
	}
	if string(got.RawReleaseJSON) != string(raw) {
		t.Errorf("raw_release_json = %q, want it verbatim", got.RawReleaseJSON)
	}
	if !got.ExpiresAt.Equal(now.Add(CandidateTTL)) {
		t.Errorf("expiresAt = %s", got.ExpiresAt)
	}

	if _, err := adapter.Candidate(ctx, scope, 99999); !errors.Is(err, ErrCandidateNotFound) {
		t.Errorf("missing row: err = %v, want ErrCandidateNotFound", err)
	}
	// Out of scope must be indistinguishable from missing.
	narrow := Scope{UserID: 2, InstanceIDs: []int64{instanceID + 1000}}
	if _, err := adapter.Candidate(ctx, narrow, ids[0]); !errors.Is(err, ErrCandidateNotFound) {
		t.Errorf("out-of-scope row: err = %v, want ErrCandidateNotFound", err)
	}

	// Provenance, including the two fields that must NOT be written.
	published := now.Add(-72 * time.Hour)
	provID, err := adapter.InsertProvenance(ctx, Provenance{
		Protocol:          "torrent",
		IndexerName:       "an-indexer",
		IndexerID:         3,
		IndexerPrivacy:    "private",
		IndexerCategories: []int32{2000, 2045},
		IndexerFlags:      []string{"freeleech"},
		DownloadID:        "deadbeef",
		TorrentInfoHash:   "deadbeef",
		ReleaseGUID:       "guid-1",
		ReleaseTitle:      "Some.Movie.2026.1080p.BluRay.x264-GROUP",
		PublishedAt:       &published,
		GrabbedAt:         now,
		SourceSystem:      "prowlarr",
		SourceRecordID:    "3_guid-1",
		Confidence:        1,
	})
	if err != nil {
		t.Fatalf("InsertProvenance: %v", err)
	}
	if provID == 0 {
		t.Fatal("no provenance id")
	}
}

func TestStoreAdapterEvictsExpired(t *testing.T) {
	ctx := t.Context()
	adapter, instanceID := newRealStore(t)
	now := time.Unix(1_755_000_000, 0).UTC()

	if _, err := adapter.InsertCandidates(ctx, []Candidate{
		{
			ServiceInstanceID: instanceID, GUID: "old", Title: "old",
			RawReleaseJSON: []byte(`{}`), FetchedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
		},
		{
			ServiceInstanceID: instanceID, GUID: "fresh", Title: "fresh",
			RawReleaseJSON: []byte(`{}`), FetchedAt: now, ExpiresAt: now.Add(CandidateTTL),
		},
	}); err != nil {
		t.Fatalf("InsertCandidates: %v", err)
	}

	n, err := adapter.DeleteExpiredCandidates(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpiredCandidates: %v", err)
	}
	if n != 1 {
		t.Errorf("evicted %d, want 1", n)
	}
}
