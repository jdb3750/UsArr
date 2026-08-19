package libsync

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
	"github.com/jdb3750/UsArr/internal/vcrscrub"
)

// ⚠️ EVERY CASSETTE THIS PACKAGE REPLAYS IS SYNTHETIC, and each one says so in
// its own header comment along with the Kavita version it claims. Two are
// internal/kavita's (kavita_libraries.yaml, kavita_series_all_v2.yaml) and two
// are this commit's (kavita_libraries_all_types.yaml,
// kavita_series_all_v2_identified.yaml). None was captured off a wire.
//
// What that buys and what it does not, stated the same way internal/kavita's
// vcr_test.go states it: A SYNTHETIC CASSETTE PROVES THIS MAPPING, NOT THE
// SERVER'S BEHAVIOUR. It cannot discover that a field is always null in
// practice, that a controller enforces something the schema does not, or that a
// middleware header arrives in a different case. The one live measurement this
// project has is ADR-0035 §2a's channel-3b probe against the owner's Kavita
// 0.9.0.2, and it covers ordering and resumability — not these mappings.
//
// THE DATABASE SIDE IS NOT SYNTHETIC. Every test here that touches SQL runs
// against a real migrated database opened through internal/db, and populates it
// before it asserts.

const (
	testBaseURL = "http://kavita.test:5000"
	// The repository's shared obviously-fake credential fixture. It is not
	// GUID-shaped on purpose — see internal/kavita/vcr_test.go.
	testAuthKey = "0123456789abcdef0123456789abcdef"
)

var testNow = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

// The matcher and the scrubbing hook come from internal/vcrscrub.
//
// ⚠️ THIS HARNESS PREVIOUSLY HAD NO BeforeSave HOOK AT ALL — it wired
// recorder.New by hand and installed only a matcher. It never fired, because
// the mode was hard-coded ModeReplayOnly, but it was the one cassette opener in
// the tree that would have written a credential straight to disk the moment
// anybody enabled recording. vcrscrub.New installs the hook rather than offering
// it, so there is nothing left to forget.

func newCassetteClient(t *testing.T, names ...string) *kavita.Client {
	t.Helper()
	if len(names) == 0 {
		t.Fatal("newCassetteClient needs at least one cassette")
	}

	// One recorder per cassette, chained: the libraries call and the series call
	// live in different files, and a Client takes one HTTPDoer.
	var doer kavita.HTTPDoer = chainedDoer(t, names)
	c, err := kavita.New(kavita.Options{
		BaseURL:    testBaseURL,
		APIKey:     testAuthKey,
		HTTPClient: doer,
		AppVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

// chainedDoer tries each cassette in turn and returns the first that has the
// interaction. It fails the test rather than the request when none does: a
// silent miss would make an assertion about "what the adapter read" an
// assertion about an empty stream.
type chained struct {
	t     *testing.T
	doers []*http.Client
	names []string
}

func chainedDoer(t *testing.T, names []string) *chained {
	t.Helper()
	c := &chained{t: t, names: names}
	for _, n := range names {
		rec, err := vcrscrub.New(
			filepath.Join("..", "..", "testdata", "cassettes", strings.TrimSuffix(n, ".yaml")),
		)
		if err != nil {
			t.Fatalf("opening cassette %s: %v", n, err)
		}
		t.Cleanup(func() {
			if err := rec.Stop(); err != nil {
				t.Errorf("stopping recorder %s: %v", n, err)
			}
		})
		c.doers = append(c.doers, rec.GetDefaultClient())
	}
	return c
}

func (c *chained) Do(req *http.Request) (*http.Response, error) {
	var lastErr error
	for _, d := range c.doers {
		// Each attempt needs its own body: a replayed POST consumes it.
		clone := req.Clone(req.Context())
		if req.GetBody != nil {
			b, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			clone.Body = b
		}
		resp, err := d.Do(clone)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "usarr.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("db.Close: %v", err)
		}
	})
	return store.New(d)
}

func fixtureInstance(t *testing.T, s *store.Store) int64 {
	t.Helper()
	id, err := s.CreateServiceInstance(t.Context(), store.ServiceInstance{
		Kind: "kavita", Role: "library", Name: "kavita",
		BaseURL: testBaseURL, APIKeyEnc: []byte{1, 2, 3}, KEKID: 1,
		Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	return id
}
