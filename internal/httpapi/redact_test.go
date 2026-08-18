package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The deny-list is fixed by ARCHITECTURE.md §14.5 and CONFIGURATION.md §2.1.
// This is the case-by-case proof that the middleware actually applies it — and
// in particular that the short, generic-looking names are covered, because those
// are the OpenSubsonic ones and they are the dangerous half.
func TestRedactRequestLineCoversTheDenyList(t *testing.T) {
	for _, param := range []string{
		"apiKey", "api_key", "apikey", "token", "access_token", "sig", "p", "t", "s",
	} {
		t.Run(param, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/rest/ping?"+param+"=SUPERSECRET&f=json", nil)
			got := redactRequestLine(r)
			if strings.Contains(got, "SUPERSECRET") {
				t.Fatalf("%s was not redacted: %s", param, got)
			}
			if !strings.Contains(got, "f=json") {
				t.Fatalf("a non-credential parameter was lost: %s", got)
			}
		})
	}
}

func TestRedactMiddlewareRunsBeforeTheHandler(t *testing.T) {
	var seen string
	h := redactMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestLine(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/search?query=x&apikey=LEAK", nil))

	if seen == "" {
		t.Fatal("the handler saw no redacted request line at all")
	}
	if strings.Contains(seen, "LEAK") {
		t.Fatalf("the redacted request line still carries the key: %s", seen)
	}
	if !strings.HasPrefix(seen, "GET ") {
		t.Fatalf("the request line lost its method: %s", seen)
	}
}

func TestRedactedHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("X-Api-Key", "full-admin-credential")
	h.Set("Authorization", "Bearer full-admin-credential")
	h.Set("Cookie", "usarr_session=abc")
	h.Set("User-Agent", "curl/8")

	got := RedactedHeaders(h)
	for _, name := range []string{"X-Api-Key", "Authorization", "Cookie"} {
		if got[name] != RedactedPlaceholder {
			t.Errorf("%s = %q, want %q", name, got[name], RedactedPlaceholder)
		}
	}
	if got["User-Agent"] != "curl/8" {
		t.Errorf("User-Agent was mangled: %q", got["User-Agent"])
	}
}

// The shim over ssrf.RedactText. The scanner's own cases live in internal/ssrf;
// this proves the shim still redacts, so no call site in this package silently
// lost its redaction when the implementation moved.
func TestRedactTextShimStillRedacts(t *testing.T) {
	in := `Get "http://prowlarr:9696/api/v1/indexer?apikey=abc123def456": dial tcp 10.0.0.5:9696: connect: connection refused`
	got := redactText(in)

	if strings.Contains(got, "abc123def456") {
		t.Fatalf("the key survived redaction: %s", got)
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("redaction destroyed the diagnostic: %s", got)
	}
}
