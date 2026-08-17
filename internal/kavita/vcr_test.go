package kavita

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

// ⚠️ THE KAVITA CASSETTES IN testdata/cassettes ARE SYNTHETIC. Every one of them
// was hand-authored from the vendored OpenAPI document (api/specs/kavita-develop.json,
// develop @ 9c3e540, info.version 0.9.0.20) and from Kavita's controller source
// at that same commit. None was captured off a wire.
//
// Say it plainly, because the distinction is the whole value of the convention
// internal/servarr established: A SYNTHETIC CASSETTE PROVES THIS CLIENT'S
// PARSING, NOT THE SERVER'S BEHAVIOUR. It cannot discover that a field is always
// null in practice, that a controller enforces something the schema does not, or
// that a middleware header arrives in a different case. Those are what a live
// capture is for.
//
// What stands behind them instead, and what does not:
//
//   - The contract tests in this package pin the shapes against the vendored
//     spec, and the drift job pins the spec.
//   - The behavioural facts this client is built on were read from Kavita's
//     CONTROLLERS at the pinned commit, not from the schema — because
//     DEVELOPMENT.md §5's rule is that the controller wins.
//   - ONE live measurement exists and it is not this suite's: ADR-0035 §2a ran
//     the channel-3b watermark probe against the owner's Kavita 0.9.0.2, 151
//     series, 2026-08-17. That is the only evidence in this project about a real
//     Kavita, and it covers ordering and resumability — not these endpoints'
//     error shapes.
//
// The version each cassette claims is in its own header comment, because a
// capture from 0.9.0.2 and one from 0.9.0.20 are different evidence and must not
// be indistinguishable in this directory. Kavita sends NO version response
// header, so unlike the Prowlarr cassettes that fact is not free to capture — it
// has to be written down by hand at record time.
//
// Re-record against a real Kavita with USARR_RECORD=1, which switches the
// recorder to ModeRecordOnce and lets the scrubbing hook below do its job for
// real.

const (
	testBaseURL = "http://kavita.test:5000"

	// testAuthKey is the fixture Auth Key. It is what the scrubbing assertions
	// look for: no test may let this string reach a cassette, an error string or a
	// log line.
	//
	// It is DELIBERATELY NOT GUID-SHAPED, although a real Kavita Auth Key is a
	// GUID (inference from the `x-api-key` fixtures in Kavita's own docs; not
	// read from the generator). A GUID-shaped fixture — even
	// `0123456789abcdef0123456789abcdef` — trips gitleaks' generic-api-key
	// rule, and `.gitleaks.toml`'s own instruction for that case is explicit:
	// "make it obviously fake rather than extending this list". So this reuses the
	// repository's existing sequential-hex fixture value, which the shared
	// allowlist already covers, and nothing in this package cares about the
	// dashes.
	testAuthKey = "0123456789abcdef0123456789abcdef"

	// testKavitaVersion is what the synthetic cassettes claim to be.
	testKavitaVersion = "0.9.0.20"
)

func cassettePath(name string) string {
	return filepath.Join("..", "..", "testdata", "cassettes", name)
}

// urlMatcher matches an interaction on method plus REDACTED URL, and nothing
// else.
//
// go-vcr's default matcher also compares every request header — which cannot work
// alongside scrubbing, because the recorded x-api-key is the placeholder while the
// live request carries the real key.
//
// 🚩 THE REDACTION ON BOTH SIDES IS NOT COSMETIC, and it is the one place this
// package's matcher has to differ from internal/servarr's. Kavita's
// GET /api/Plugin/version carries the Auth Key IN THE URL, so the saved
// interaction's URL is scrubbed to `?apiKey=REDACTED` while the live request
// carries the real key. Comparing raw URLs would mean a cassette could only match
// by storing the credential — which is precisely what the scrubbing hook exists
// to prevent. Redacting the incoming URL too makes the two agree, and it is a
// no-op for every URL that carries no credential.
//
// The POST body is deliberately not matched on either: SeriesFilter's exact bytes
// are pinned by TestOutboundBodiesAreSpecLegal against the SPEC, which is a
// stronger check than pinning them against a fixture this repo also wrote.
func urlMatcher(r *http.Request, i cassette.Request) bool {
	return r.Method == i.Method && RedactURL(r.URL.String()) == RedactURL(i.URL)
}

// credentialQueryRe matches a credential carried in a query string. Kavita's
// GET /api/Plugin/version takes the Auth Key as ?apiKey= because the controller
// reads it from the query and offers no header path, so this has to run over URLs
// and over bodies.
var credentialQueryRe = regexp.MustCompile(`(?i)\b(apikey|api_key|access_token|token|sig)=([^&"'\s\\]+)`)

// scrubbedHeaders are rewritten wholesale.
var scrubbedHeaders = []string{
	"x-api-key",
	"Authorization",
	"Set-Cookie",
	"Cookie",
	// Kavita's own outbound-to-Kavita+ credentials, in case a capture ever picks
	// one up from a proxied response.
	"x-license-key",
	"x-anilist-token",
}

// scrubInteraction is the BeforeSave hook. Scrubbing is MANDATORY and must be a
// hook rather than a manual step: a manual step is one forgotten commit away from
// putting a full-admin Auth Key in the repository, and `make secrets` is only the
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

	c, err := New(Options{
		BaseURL:    testBaseURL,
		APIKey:     testAuthKey,
		HTTPClient: rec.GetDefaultClient(),
		AppVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

// assertNoCredential fails if s contains the Auth Key anywhere.
func assertNoCredential(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, testAuthKey) {
		t.Fatalf("%s leaked the Auth Key: %q", what, s)
	}
}

func TestScrubInteraction(t *testing.T) {
	i := &cassette.Interaction{
		Request: cassette.Request{
			Method:  http.MethodGet,
			URL:     testBaseURL + "/api/Plugin/version?apiKey=" + testAuthKey,
			Headers: http.Header{"X-Api-Key": []string{testAuthKey}, "Accept": []string{"application/json"}},
			Form:    url.Values{"apiKey": []string{testAuthKey}},
			Body:    "access_token=" + testAuthKey,
		},
		Response: cassette.Response{
			Headers: http.Header{
				"Set-Cookie": []string{"SID=abc123; Path=/"},
				"Pagination": []string{`{"currentPage":1,"itemsPerPage":2,"totalItems":151,"totalPages":76}`},
			},
			Body: `"` + testKavitaVersion + `"`,
		},
	}

	if err := scrubInteraction(i); err != nil {
		t.Fatalf("scrub: %v", err)
	}

	assertNoCredential(t, "request URL", i.Request.URL)
	assertNoCredential(t, "request header", i.Request.Headers.Get("X-Api-Key"))
	assertNoCredential(t, "request form", i.Request.Form.Get("apiKey"))
	assertNoCredential(t, "request body", i.Request.Body)

	if got := i.Response.Headers.Get("Set-Cookie"); got != RedactedPlaceholder {
		t.Errorf("Set-Cookie = %q, want %q", got, RedactedPlaceholder)
	}
	// The Pagination header must survive: it is the only custom response header
	// Kavita sets on an /api/* response, it is not in the OpenAPI document, and a
	// scrubber that ate it would make every cassette useless for the one thing
	// only a capture can show.
	if got := i.Response.Headers.Get("Pagination"); !strings.Contains(got, "totalItems") {
		t.Errorf("scrubbing clobbered the Pagination header: %q", got)
	}
}
