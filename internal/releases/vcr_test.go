package releases

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/vcrscrub"
)

// One end-to-end test that drives the real servarr.Client over a cassette rather
// than the fake, because the cache-miss recovery is the one flow whose correctness
// depends on the exact upstream status codes and body shapes.
//
// The cassette is SYNTHETIC — hand-authored from Prowlarr's shipped OpenAPI
// document and controller behaviour. There is no public Prowlarr demo instance.

const (
	testBaseURL = "http://prowlarr.test:9696"
	testAPIKey  = "0123456789abcdef0123456789abcdef"
)

// The scrubbing hook, the matcher and the recorder wiring live in
// internal/vcrscrub. This file used to carry a third copy of all three,
// justified in a comment that said sharing them "would put a go-vcr dependency
// in the shipped binary" — an objection now answered by execution rather than by
// duplication: vcrscrub's TestBinaryDoesNotLinkTheRecorder asks the go tool what
// ./cmd/usarr actually links.

func newCassetteClient(t *testing.T, name string) *servarr.Client {
	t.Helper()
	rec, err := vcrscrub.New(filepath.Join("..", "..", "testdata", "cassettes", name))
	if err != nil {
		t.Fatalf("opening cassette %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := rec.Stop(); err != nil {
			t.Errorf("stopping recorder: %v", err)
		}
	})

	c, err := servarr.NewProwlarr(servarr.Options{
		BaseURL:    testBaseURL,
		APIKey:     testAPIKey,
		HTTPClient: rec.GetDefaultClient(),
		AppVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

func TestGrabCacheMissRecoveryAgainstCassette(t *testing.T) {
	client := newCassetteClient(t, "prowlarr_grab_cache_miss_recovered")

	// The candidate as the search would have stored it: the verbatim
	// ReleaseResource, credential-bearing downloadUrl and all.
	raw, err := json.Marshal(servarr.ReleaseResource{
		GUID:             "https://fake-torrents.test/details/1001",
		Title:            "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC-FraMeSToR",
		IndexerID:        1,
		Indexer:          "Fake Torrents (API)",
		Protocol:         servarr.ProtocolTorrent,
		DownloadClientID: servarr.Int32(2),
		InfoHash:         strPtr("d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0"),
		Categories:       []servarr.IndexerCategory{{ID: 2000}, {ID: 2045}},
	})
	if err != nil {
		t.Fatalf("marshalling fixture: %v", err)
	}

	now := time.Unix(1_755_000_000, 0).UTC()
	store := newFakeStore()
	id := store.put(Candidate{
		ServiceInstanceID: testInstanceID,
		GUID:              "https://fake-torrents.test/details/1001",
		Title:             "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC-FraMeSToR",
		Indexer:           "Fake Torrents (API)",
		IndexerID:         1,
		Protocol:          "torrent",
		DownloadClientID:  servarr.Int32(2),
		RawReleaseJSON:    raw,
		FetchedAt:         now,
		ExpiresAt:         now.Add(CandidateTTL),
	})

	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: client, Store: store,
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
		t.Error("the cassette scripts a 404 first; the recovery should be reported")
	}
	if got.ProvenanceID == 0 || len(store.provenance) != 1 {
		t.Fatal("provenance was not recorded")
	}
	p := store.provenance[0]
	if p.ReleaseTitle == "" || p.ReleaseGUID == "" {
		t.Errorf("provenance = %+v", p)
	}
	// Nothing anywhere in the recorded provenance may carry the credential.
	blob, _ := json.Marshal(p)
	if strings.Contains(string(blob), "apikey=") || strings.Contains(string(blob), testAPIKey) {
		t.Fatalf("provenance carries a credential: %s", blob)
	}
}

func strPtr(s string) *string { return &s }
