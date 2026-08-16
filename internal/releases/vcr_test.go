package releases

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/jdb3750/UsArr/internal/servarr"
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

var credentialQueryRe = regexp.MustCompile(`(?i)\b(apikey|api_key|access_token|token|sig)=([^&"'\s\\]+)`)

// scrubInteraction is the mandatory BeforeSave hook. See
// internal/servarr/vcr_test.go for the full rationale; it is duplicated rather
// than shared because a scrubber that lives in non-test code would put a go-vcr
// dependency in the shipped binary.
func scrubInteraction(i *cassette.Interaction) error {
	for _, h := range []string{"X-Api-Key", "Authorization", "Set-Cookie", "Cookie", "X-Transmission-Session-Id"} {
		if _, ok := i.Request.Headers[http.CanonicalHeaderKey(h)]; ok {
			i.Request.Headers.Set(h, servarr.RedactedPlaceholder)
		}
		if _, ok := i.Response.Headers[http.CanonicalHeaderKey(h)]; ok {
			i.Response.Headers.Set(h, servarr.RedactedPlaceholder)
		}
	}
	i.Request.URL = servarr.RedactURL(i.Request.URL)
	i.Request.Body = credentialQueryRe.ReplaceAllString(i.Request.Body, "$1="+servarr.RedactedPlaceholder)
	i.Response.Body = credentialQueryRe.ReplaceAllString(i.Response.Body, "$1="+servarr.RedactedPlaceholder)
	return nil
}

func newCassetteClient(t *testing.T, name string) *servarr.Client {
	t.Helper()
	rec, err := recorder.New(
		filepath.Join("..", "..", "testdata", "cassettes", name),
		recorder.WithMode(recorder.ModeReplayOnly),
		recorder.WithMatcher(func(r *http.Request, i cassette.Request) bool {
			return r.Method == i.Method && r.URL.String() == i.URL
		}),
		recorder.WithHook(scrubInteraction, recorder.BeforeSaveHook),
		recorder.WithSkipRequestLatency(true),
	)
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
