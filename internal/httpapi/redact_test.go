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
			r := httptest.NewRequest(http.MethodGet, "/rest/ping?"+param+"=SUPERSECRET&f=json", nil)
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
		httptest.NewRequest(http.MethodGet, "/api/v1/search?query=x&apikey=LEAK", nil))

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

// An upstream error routinely quotes the URL it failed on, and for Prowlarr that
// URL carries ?apikey=. Verbatim means "the actual error", not "including the
// key" — this is the line between the two.
func TestRedactTextKeepsTheErrorAndDropsTheKey(t *testing.T) {
	in := `Get "http://prowlarr:9696/api/v1/indexer?apikey=abc123def456": dial tcp 10.0.0.5:9696: connect: connection refused`
	got := redactText(in)

	if strings.Contains(got, "abc123def456") {
		t.Fatalf("the key survived redaction: %s", got)
	}
	for _, want := range []string{"connection refused", "prowlarr:9696", "/api/v1/indexer"} {
		if !strings.Contains(got, want) {
			t.Errorf("redaction destroyed the diagnostic: %q missing from %s", want, got)
		}
	}
}

func TestRedactTextLeavesPlainTextAlone(t *testing.T) {
	in := "Prowlarr rejected UsArr's API key"
	if got := redactText(in); got != in {
		t.Fatalf("redactText(%q) = %q, want it unchanged", in, got)
	}
}

func TestRedactTextHandlesTrailingPunctuation(t *testing.T) {
	got := redactText("failed on http://host:9696/x?token=SECRET.")
	if strings.Contains(got, "SECRET") {
		t.Fatalf("key survived: %s", got)
	}
	if !strings.HasSuffix(got, ".") {
		t.Fatalf("the sentence lost its full stop: %s", got)
	}
}
