package servarr

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"

	"github.com/jdb3750/UsArr/internal/vcrscrub"
)

// The cassettes in testdata/cassettes are SYNTHETIC. They were hand-authored
// from Prowlarr's shipped OpenAPI document (api/specs/prowlarr.json) and from its
// controller behaviour, because there is no public demo instance of any Servarr
// app with API access and the project owner runs no *Arr stack. Re-record them
// against a real Prowlarr when one is available: run with USARR_RECORD=1, which
// switches the recorder to ModeRecordOnce and lets the scrubbing hook below do
// its job for real.
//
// A synthetic cassette proves the client's parsing, not the server's behaviour.
// That is why the contract test against the vendored spec exists alongside it:
// between them they pin the shapes, and the drift job pins the spec.

const (
	testBaseURL = "http://prowlarr.test:9696"

	// testAPIKey is a fake 32-hex key with the same shape as a real one. It is
	// what the scrubbing assertions look for: no test may let this string reach a
	// cassette, an error string or a log line.
	testAPIKey = "0123456789abcdef0123456789abcdef"
)

func cassettePath(name string) string {
	return filepath.Join("..", "..", "testdata", "cassettes", name)
}

// The matcher, the scrubbing hook and the recorder wiring all moved to
// internal/vcrscrub. This package used to carry its own copy of each, and the
// copy included a five-name `credentialQueryRe` that was a SECOND deny-list
// beside internal/ssrf's — one which had already drifted past `signature`,
// `secret`, `auth_token` and every private-tracker passkey name. That is the
// duplication internal/ssrf/redact.go forbids in as many words, and a cassette
// is the worst place to lose the argument, because a cassette is committed.

// newTestClient builds a Client wired to a cassette.
func newTestClient(t *testing.T, cassetteName string) *Client {
	t.Helper()

	rec, err := vcrscrub.New(cassettePath(cassetteName))
	if err != nil {
		t.Fatalf("opening cassette %s: %v", cassetteName, err)
	}
	t.Cleanup(func() {
		if err := rec.Stop(); err != nil {
			t.Errorf("stopping recorder: %v", err)
		}
	})

	c, err := NewProwlarr(Options{
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

// assertNoCredential fails if s contains the API key anywhere.
func assertNoCredential(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, testAPIKey) {
		t.Fatalf("%s leaked the API key: %q", what, s)
	}
}

// TestScrubInteraction keeps this package's own shapes — Prowlarr's admin key
// spliced into downloadUrl and magnetUrl by SearchController.MapReleases — under
// test against the shared scrubber. internal/vcrscrub owns the generic
// behaviour; this owns the claim that PROWLARR's leak shape is covered.
func TestScrubInteraction(t *testing.T) {
	i := &cassette.Interaction{
		Request: cassette.Request{
			Method:  http.MethodGet,
			URL:     testBaseURL + "/api/v1/search?apikey=" + testAPIKey + "&query=dune",
			Headers: http.Header{"X-Api-Key": []string{testAPIKey}, "Accept": []string{"application/json"}},
			Form:    url.Values{"apikey": []string{testAPIKey}},
			Body:    "access_token=" + testAPIKey,
		},
		Response: cassette.Response{
			Headers: http.Header{
				"Set-Cookie":            []string{"SID=abc123; Path=/"},
				"X-Application-Version": []string{"2.6.2.5052"},
			},
			// The shape that actually matters: a search result body with the admin
			// key embedded in downloadUrl by SearchController.MapReleases.
			Body: `{"downloadUrl":"http://prowlarr.test:9696/1/download?apikey=` + testAPIKey + `&link=aGk"}`,
		},
	}

	if err := vcrscrub.Scrub(i); err != nil {
		t.Fatalf("scrub: %v", err)
	}

	assertNoCredential(t, "request URL", i.Request.URL)
	assertNoCredential(t, "request header", i.Request.Headers.Get("X-Api-Key"))
	assertNoCredential(t, "request form", i.Request.Form.Get("apikey"))
	assertNoCredential(t, "request body", i.Request.Body)
	assertNoCredential(t, "response body", i.Response.Body)

	if got := i.Response.Headers.Get("Set-Cookie"); got != vcrscrub.Placeholder {
		t.Errorf("Set-Cookie = %q, want %q", got, vcrscrub.Placeholder)
	}
	// Non-credential headers must survive, or the cassette stops being useful.
	if got := i.Response.Headers.Get("X-Application-Version"); got != "2.6.2.5052" {
		t.Errorf("X-Application-Version was clobbered: %q", got)
	}
	if !strings.Contains(i.Request.URL, "query=dune") {
		t.Errorf("scrubbing dropped a non-credential query parameter: %q", i.Request.URL)
	}
}
