package ssrf

import (
	"net/url"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in       string
		mustHave []string
		mustNot  []string
	}{
		{
			in:       "https://api.themoviedb.org/3/movie/550?api_key=deadbeefdeadbeef&language=en",
			mustHave: []string{"api_key=REDACTED", "language=en"},
			mustNot:  []string{"deadbeef"},
		},
		{
			in:       "http://sonarr.lan:8989/api/v3/series?apiKey=0123456789abcdef",
			mustHave: []string{"REDACTED"},
			mustNot:  []string{"0123456789abcdef"},
		},
		{
			in:       "https://navidrome.lan/rest/ping?u=joe&t=abcd1234&s=salty&v=1.16.1",
			mustHave: []string{"u=joe", "v=1.16.1"},
			mustNot:  []string{"abcd1234", "salty"},
		},
		{
			in:       "https://joe:hunter2@komga.lan/api/v1/books",
			mustHave: []string{"komga.lan"},
			mustNot:  []string{"hunter2", "joe:"},
		},
		{
			in:       "https://img.example/cover.jpg?access_token=xyz&sig=abc&width=500",
			mustHave: []string{"width=500"},
			mustNot:  []string{"xyz", "sig=abc"},
		},
	}

	for _, tc := range cases {
		u, err := url.Parse(tc.in)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.in, err)
		}
		got := RedactURL(u)
		for _, want := range tc.mustHave {
			if !strings.Contains(got, want) {
				t.Errorf("RedactURL(%q) = %q, want it to contain %q", tc.in, got, want)
			}
		}
		for _, bad := range tc.mustNot {
			if strings.Contains(got, bad) {
				t.Errorf("RedactURL(%q) = %q, must not leak %q", tc.in, got, bad)
			}
		}
		// Redacting must not mutate the caller's URL.
		if u.String() != tc.in && !strings.Contains(tc.in, "@") {
			t.Errorf("RedactURL mutated its argument: %q became %q", tc.in, u.String())
		}
	}
}

func TestRedactRawURLUnparseable(t *testing.T) {
	t.Parallel()

	// "It did not parse" is not evidence that it holds no secret.
	if got := RedactRawURL("http://[::1"); strings.Contains(got, "::1") {
		t.Errorf("unparseable url leaked through: %q", got)
	}
}

func TestStripCredentials(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://cdn.example/a.jpg?apikey=secret&token=t&sig=s&keep=1")
	if err != nil {
		t.Fatal(err)
	}
	stripCredentials(u)
	q := u.Query()
	for _, name := range []string{"apikey", "token", "sig"} {
		if q.Has(name) {
			t.Errorf("%s survived stripCredentials", name)
		}
	}
	if q.Get("keep") != "1" {
		t.Error("stripCredentials dropped a non-credential parameter")
	}
}
