package servarr

import (
	"net/url"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// The shape Prowlarr actually produces in every search result.
			name: "prowlarr download proxy link",
			in:   "http://prowlarr.test:9696/1/download?apikey=" + testAPIKey + "&link=aGk&file=x",
			want: "http://prowlarr.test:9696/1/download?apikey=REDACTED&file=x&link=aGk",
		},
		{
			name: "opensubsonic token triple",
			in:   "http://usarr.test/rest/ping?u=joe&t=abc123&s=deadbeef&p=plaintext",
			want: "http://usarr.test/rest/ping?p=REDACTED&s=REDACTED&t=REDACTED&u=joe",
		},
		{
			// Userinfo is REMOVED, not redacted: the redactor is shared with the
			// logging middleware, and a stored URL should not keep the shape of a
			// credential it no longer has.
			name: "userinfo",
			in:   "https://user:hunter2@prowlarr.test:9696/api/v1/indexer",
			want: "https://prowlarr.test:9696/api/v1/indexer",
		},
		{
			name: "no credential is left alone",
			in:   "https://prowlarr.test:9696/api/v1/search?query=dune&type=search",
			want: "https://prowlarr.test:9696/api/v1/search?query=dune&type=search",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			// An unparseable string may still contain a key, so nothing of it is
			// returned. internal/ssrf owns this placeholder.
			name: "unparseable",
			in:   "http://[::1]:namedport/x",
			want: "<unparseable url>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactURL(tc.in)
			if got != tc.want {
				t.Errorf("RedactURL(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
			assertNoCredential(t, "redacted URL", got)
		})
	}
}

func TestRedactURLIsCaseInsensitive(t *testing.T) {
	got := RedactURL("https://tmdb.test/3/movie/1?API_KEY=" + testAPIKey)
	assertNoCredential(t, "redacted URL", got)
	if !strings.Contains(got, "API_KEY=") {
		t.Errorf("parameter name should survive: %q", got)
	}
}

func TestRedactErrStripsTheURLFromTransportErrors(t *testing.T) {
	// net/http wraps every transport failure in *url.Error, whose Error() prints
	// the full request URL — which for this package can carry a credential.
	err := &url.Error{
		Op:  "Get",
		URL: "http://p.test/api/v1/search?apikey=" + testAPIKey,
		Err: &testErr{"connection refused"},
	}
	got := redactErr(err)
	assertNoCredential(t, "redacted transport error", got)
	if !strings.Contains(got, "connection refused") {
		t.Errorf("the cause must survive: %q", got)
	}
}

type testErr struct{ s string }

func (e *testErr) Error() string { return e.s }
