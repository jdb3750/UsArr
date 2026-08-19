package kavita

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"

	"github.com/jdb3750/UsArr/internal/vcrscrub"
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

// The matcher, the scrubbing hook and the recorder wiring all moved to
// internal/vcrscrub, which now owns them for every cassette in the tree.
//
// 🚩 THE REDACTION ON BOTH SIDES OF THE MATCH IS NOT COSMETIC and it is why the
// shared matcher had to be the redacting one rather than this package's being
// the odd one out. Kavita's GET /api/Plugin/version carries the Auth Key IN THE
// URL, so the saved interaction's URL is scrubbed to `?apiKey=REDACTED` while
// the live request carries the real key. Comparing raw URLs would mean a
// cassette could only match by STORING the credential — precisely what the
// scrubbing hook exists to prevent.
//
// This package also used to carry a five-name `credentialQueryRe`, a SECOND
// deny-list beside internal/ssrf's. It is gone; vcrscrub asks
// ssrf.IsCredentialParam.

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

// TestScrubInteraction keeps KAVITA's shapes under test against the shared
// scrubber: the Auth Key in the URL query, and the Pagination header that must
// survive. internal/vcrscrub owns the generic behaviour.
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

	if err := vcrscrub.Scrub(i); err != nil {
		t.Fatalf("scrub: %v", err)
	}

	assertNoCredential(t, "request URL", i.Request.URL)
	assertNoCredential(t, "request header", i.Request.Headers.Get("X-Api-Key"))
	assertNoCredential(t, "request form", i.Request.Form.Get("apiKey"))
	assertNoCredential(t, "request body", i.Request.Body)

	if got := i.Response.Headers.Get("Set-Cookie"); got != vcrscrub.Placeholder {
		t.Errorf("Set-Cookie = %q, want %q", got, vcrscrub.Placeholder)
	}
	// The Pagination header must survive: it is the only custom response header
	// Kavita sets on an /api/* response, it is not in the OpenAPI document, and a
	// scrubber that ate it would make every cassette useless for the one thing
	// only a capture can show.
	if got := i.Response.Headers.Get("Pagination"); !strings.Contains(got, "totalItems") {
		t.Errorf("scrubbing clobbered the Pagination header: %q", got)
	}
}
