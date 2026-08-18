package kavita

import (
	"strings"
	"testing"
)

// LS-170 step 4: the guard the finding says did not exist.
//
// assertNoCredential's four existing call sites all cover the REQUEST side — the
// base URL, the redacted PluginVersion URL, a transport error, and the cassette's
// request fields. None covered a response body, so a change that widened what
// reaches APIError.Message passed the suite green. This one drives the response
// side.
//
// The body is the shape the named live route produces: GET /api/Plugin/version
// takes the Auth Key as ?apiKey=, so a Kavita error that quotes the URL it failed
// on quotes the key with it. APIError.Error() renders Message AND Validation, and
// that string reaches slog and service_instance.last_error.
func TestParseErrorBodyRedactsTheAuthKey(t *testing.T) {
	failingURL := testBaseURL + "/api/Plugin/version?apiKey=" + testAuthKey

	body := `{` +
		`"type":"https://tools.ietf.org/html/rfc7231#section-6.6.1",` +
		`"title":"An error occurred",` +
		`"status":500,` +
		`"detail":"Get \"` + failingURL + `\": upstream refused",` +
		`"errors":{"apiKey":["rejected for ` + failingURL + `"]}` +
		`}`

	err := parseErrorBody("PluginVersion", "GET", "/api/Plugin/version", 500, []byte(body))

	assertNoCredential(t, "APIError.Error()", err.Error())
	assertNoCredential(t, "APIError.Message", err.Message)
	for _, v := range err.Validation {
		assertNoCredential(t, "APIError.Validation PropertyName", v.PropertyName)
		assertNoCredential(t, "APIError.Validation ErrorMessage", v.ErrorMessage)
	}

	// Redaction must not cost the diagnostic: the sentence and the host survive.
	for _, want := range []string{"An error occurred", "upstream refused", "kavita.test:5000"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("redaction destroyed the diagnostic: %q missing from %s", want, err.Error())
		}
	}
}

// The four truncate branches are the ones a fix aimed only at `truncate` would
// have covered. They are asserted too, so neither half can regress alone.
func TestParseErrorBodyRedactsEveryBranch(t *testing.T) {
	failingURL := testBaseURL + "/api/Plugin/version?apiKey=" + testAuthKey

	cases := map[string]string{
		"{-shaped body that fails the problemDetails decode": `{"unrecognised":"` + failingURL + `"}`,
		"bare JSON string body":                              `"failed on ` + failingURL + `"`,
		`"-shaped body that fails the string decode`:         `"failed on ` + failingURL,
		"anything else":                                      "failed on " + failingURL,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			err := parseErrorBody("PluginVersion", "GET", "/api/Plugin/version", 500, []byte(body))
			assertNoCredential(t, "APIError.Error()", err.Error())
		})
	}
}

// The bound the problemDetails branch did not have. A body that carries no
// credential can still be a 2 MB proxy error page, and Message reaches a log line.
func TestParseErrorBodyBoundsTheProblemDetailsBranch(t *testing.T) {
	body := `{"title":"boom","detail":"` + strings.Repeat("x", 4096) + `"}`

	err := parseErrorBody("Libraries", "GET", "/api/Library/libraries", 500, []byte(body))

	if len(err.Message) > 600 {
		t.Fatalf("problemDetails Message is unbounded: %d bytes", len(err.Message))
	}
}
