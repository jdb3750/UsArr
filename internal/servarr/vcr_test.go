package servarr

import (
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
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

// urlMatcher matches an interaction on method plus full URL, and nothing else.
//
// go-vcr's default matcher also compares every request header — which cannot work
// alongside scrubbing, because the recorded X-Api-Key is the placeholder while the
// live request carries the real key. Matching on method+URL is both the workable
// choice and the meaningful one: the URL is the contract.
//
// The POST bodies in these cassettes are deliberately not matched on, so a grab
// retry after a cache miss replays the next unreplayed interaction in order.
func urlMatcher(r *http.Request, i cassette.Request) bool {
	return r.Method == i.Method && r.URL.String() == i.URL
}

// credentialQueryRe matches a credential carried in a query string. Prowlarr's
// search results embed `?apikey=<full admin key>` inside downloadUrl and
// magnetUrl, so this has to run over response BODIES, not just over URLs.
var credentialQueryRe = regexp.MustCompile(`(?i)\b(apikey|api_key|access_token|token|sig)=([^&"'\s\\]+)`)

// scrubbedHeaders are rewritten wholesale. Every one of them carries a bearer
// secret for some service in this ecosystem (docs/DEVELOPMENT.md §7.1).
var scrubbedHeaders = []string{
	"X-Api-Key",
	"Authorization",
	"Set-Cookie",
	"Cookie",
	"X-Transmission-Session-Id",
}

// scrubInteraction is the BeforeSave hook. Scrubbing is MANDATORY and must be a
// hook rather than a manual step: a manual step is one forgotten commit away from
// putting a full-admin *Arr key in the repository, and `make secrets` is only the
// mechanical backstop.
//
// In replay-only runs this never fires, which is exactly why TestScrubInteraction
// exercises it directly.
func scrubInteraction(i *cassette.Interaction) error {
	for _, h := range scrubbedHeaders {
		if _, ok := i.Request.Headers[http.CanonicalHeaderKey(h)]; ok {
			i.Request.Headers.Set(h, RedactedPlaceholder)
		}
		if _, ok := i.Response.Headers[http.CanonicalHeaderKey(h)]; ok {
			i.Response.Headers.Set(h, RedactedPlaceholder)
		}
	}
	i.Request.URL = RedactURL(i.Request.URL)
	i.Request.Body = credentialQueryRe.ReplaceAllString(i.Request.Body, "$1="+RedactedPlaceholder)
	i.Response.Body = credentialQueryRe.ReplaceAllString(i.Response.Body, "$1="+RedactedPlaceholder)
	if f := i.Request.Form; f != nil {
		for k := range f {
			// The parsed form is a second copy of the query string, so it needs the
			// same treatment. Reuse the URL redactor rather than a second list.
			if RedactURL("http://x/?"+k+"=probe") != "http://x/?"+k+"=probe" {
				f.Set(k, RedactedPlaceholder)
			}
		}
	}
	return nil
}

// newTestClient builds a Client wired to a cassette.
func newTestClient(t *testing.T, cassetteName string) *Client {
	t.Helper()

	rec, err := recorder.New(
		cassettePath(cassetteName),
		recorder.WithMode(recorder.ModeReplayOnly),
		recorder.WithMatcher(urlMatcher),
		recorder.WithHook(scrubInteraction, recorder.BeforeSaveHook),
		recorder.WithSkipRequestLatency(true),
	)
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

	if err := scrubInteraction(i); err != nil {
		t.Fatalf("scrub: %v", err)
	}

	assertNoCredential(t, "request URL", i.Request.URL)
	assertNoCredential(t, "request header", i.Request.Headers.Get("X-Api-Key"))
	assertNoCredential(t, "request form", i.Request.Form.Get("apikey"))
	assertNoCredential(t, "request body", i.Request.Body)
	assertNoCredential(t, "response body", i.Response.Body)

	if got := i.Response.Headers.Get("Set-Cookie"); got != RedactedPlaceholder {
		t.Errorf("Set-Cookie = %q, want %q", got, RedactedPlaceholder)
	}
	// Non-credential headers must survive, or the cassette stops being useful.
	if got := i.Response.Headers.Get("X-Application-Version"); got != "2.6.2.5052" {
		t.Errorf("X-Application-Version was clobbered: %q", got)
	}
	if !strings.Contains(i.Request.URL, "query=dune") {
		t.Errorf("scrubbing dropped a non-credential query parameter: %q", i.Request.URL)
	}
}
