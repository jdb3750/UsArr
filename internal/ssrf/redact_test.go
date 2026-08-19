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

// TestPrivateTrackerPasskeysAreRedacted is the regression test for the passkey
// leak.
//
// Prowlarr's ReleaseResource.infoUrl and .commentUrl are indexer-supplied and
// are surfaced to the browser as info_url via httpapi's redactURLField, which
// delegates here. Private trackers put the user's personal passkey in the query
// string of exactly those URLs. The deny-list covered apikey/token/sig/p/t/s but
// none of the tracker-specific names, so a passkey shipped straight to the
// client — and on a private tracker a leaked passkey means account termination,
// because it is what the tracker attributes traffic by.
//
// Both entry points are asserted: RedactURL (logging, storage, API responses)
// and stripCredentials (redirect hops), because they share this one list and a
// name must never be covered by only one of them.
func TestPrivateTrackerPasskeysAreRedacted(t *testing.T) {
	t.Parallel()

	const secret = "PASSKEYVALUE0123456789"
	names := []string{"passkey", "torrent_pass", "torrentpass", "rsskey", "authkey", "apipasskey", "cookie"}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw := "https://tracker.example/details.php?id=42&" + name + "=" + secret

			got := RedactRawURL(raw)
			if strings.Contains(got, secret) {
				t.Errorf("RedactRawURL leaked %s: %q\n"+
					"this URL reaches the browser as info_url on every search result", name, got)
			}
			if !strings.Contains(got, "id=42") {
				t.Errorf("RedactRawURL dropped a non-credential parameter: %q", got)
			}

			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			stripCredentials(u)
			if u.Query().Has(name) {
				t.Errorf("stripCredentials left %s on a redirect target: %q", name, u.String())
			}
			if u.Query().Get("id") != "42" {
				t.Errorf("stripCredentials dropped a non-credential parameter: %q", u.String())
			}
		})
	}

	// Case-insensitivity: trackers spell these every which way.
	for _, spelling := range []string{"PassKey", "TORRENT_PASS", "RSSKey"} {
		got := RedactRawURL("https://tracker.example/rss?" + spelling + "=" + secret)
		if strings.Contains(got, secret) {
			t.Errorf("RedactRawURL is case-sensitive on %q: %q", spelling, got)
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

// RedactText's own cases. These moved here from internal/httpapi when the scanner
// was lifted (LS-170 step 2); the strings and the assertions are unchanged, so a
// weakening of what stays redacted still fails here.

// An upstream error routinely quotes the URL it failed on, and for Prowlarr that
// URL carries ?apikey=. Verbatim means "the actual error", not "including the
// key" — this is the line between the two.
func TestRedactTextKeepsTheErrorAndDropsTheKey(t *testing.T) {
	in := `Get "http://prowlarr:9696/api/v1/indexer?apikey=abc123def456": dial tcp 10.0.0.5:9696: connect: connection refused`
	got := RedactText(in)

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
	if got := RedactText(in); got != in {
		t.Fatalf("RedactText(%q) = %q, want it unchanged", in, got)
	}
}

func TestRedactTextHandlesTrailingPunctuation(t *testing.T) {
	got := RedactText("failed on http://host:9696/x?token=SECRET.")
	if strings.Contains(got, "SECRET") {
		t.Fatalf("key survived: %s", got)
	}
	if !strings.HasSuffix(got, ".") {
		t.Fatalf("the sentence lost its full stop: %s", got)
	}
}

// TestRefreshTokenIsRedacted covers the name added when the cassette scrubber
// asked what a RESPONSE could carry back rather than what a request sends out.
//
// Kavita's UserDto — the body of POST /api/Plugin/authenticate — carries
// `token`, `refreshToken` and `apiKey` as siblings. Two of the three were
// already on the deny-list and the third was not.
func TestRefreshTokenIsRedacted(t *testing.T) {
	t.Parallel()

	const secret = "REFRESHTOKENVALUE0123456789"
	for _, spelling := range []string{"refresh_token", "refreshToken", "REFRESHTOKEN"} {
		if got := RedactRawURL("https://kavita.example/x?" + spelling + "=" + secret); strings.Contains(got, secret) {
			t.Errorf("RedactRawURL leaked %s: %q", spelling, got)
		}
		if !IsCredentialParam(spelling) {
			t.Errorf("IsCredentialParam(%q) = false", spelling)
		}
	}
	// The exported predicate is the ONE deny-list seen from outside; it must
	// agree with the unexported one it wraps and must not be a fresh copy.
	for _, name := range []string{"apikey", "PASSKEY", "p", "t", "s"} {
		if !IsCredentialParam(name) {
			t.Errorf("IsCredentialParam(%q) = false, but the deny-list holds it", name)
		}
	}
	for _, name := range []string{"query", "seriesId", "link", "file", "id"} {
		if IsCredentialParam(name) {
			t.Errorf("IsCredentialParam(%q) = true — over-broad", name)
		}
	}
}

// TestUnderscoredNamesCarryTheirCamelCaseTwin pins the STRUCTURAL invariant
// behind the accessToken gap, rather than re-listing the names by hand.
//
// isCredentialParam folds case and nothing else, so `accessToken` lowercases to
// `accesstoken` and never to `access_token`. An underscored entry therefore
// covers only the wire spelling that uses an underscore — which is the one a
// JSON API is LEAST likely to emit. Three entries were missing their twin
// (`access_token`, `auth_token`, `secret_key`) while three others had theirs, so
// the list looked complete and was not.
//
// Asserting over the map means a name added in future fails HERE, at the moment
// it is added, instead of when someone re-derives the spellings by hand or a
// cassette records the missing one. That is the whole difference between fixing
// the reported name and closing the class.
func TestUnderscoredNamesCarryTheirCamelCaseTwin(t *testing.T) {
	t.Parallel()

	for name := range credentialParams {
		twin := strings.ReplaceAll(name, "_", "")
		if twin == name {
			continue
		}
		if _, ok := credentialParams[twin]; !ok {
			t.Errorf("credentialParams has %q but not %q — a JSON API spelling it %q "+
				"in camelCase lowercases to %q and would NOT be redacted",
				name, twin, name, twin)
		}
	}
}

// TestAccessTokenIsRedacted covers the name the drill found, in the spellings
// that reach it.
//
// BookOrbit — v0.1's catalogue source under ADR-0052 — returns its JWT as
// `{"accessToken": …}` from `login()` and `issueTokensForUser()`
// (bookorbit/bookorbit@main, server/src/modules/auth/auth.service.ts), which is
// what POST auth/login, auth/refresh and auth/magic-links/login all resolve to.
// The shape predates that choice: Kavita's vendored spec requires `accessToken`
// on MalUserInfoDto (api/specs/kavita-v0.9.0.2.json), a MyAnimeList OAuth token.
func TestAccessTokenIsRedacted(t *testing.T) {
	t.Parallel()

	const secret = "ACCESSTOKENVALUE0123456789"
	for _, spelling := range []string{"access_token", "accessToken", "ACCESSTOKEN", "accesstoken"} {
		if got := RedactRawURL("https://bookorbit.example/x?" + spelling + "=" + secret); strings.Contains(got, secret) {
			t.Errorf("RedactRawURL leaked %s: %q", spelling, got)
		}
		if !IsCredentialParam(spelling) {
			t.Errorf("IsCredentialParam(%q) = false", spelling)
		}
	}
	// The two other twins added with it, in the spelling that was missing.
	for _, name := range []string{"authToken", "secretKey"} {
		if !IsCredentialParam(name) {
			t.Errorf("IsCredentialParam(%q) = false", name)
		}
	}
	// Widening must not have reached a benign neighbour. `accessLevel` and
	// `tokenCount` share a prefix with the names added and carry no secret.
	for _, name := range []string{"accessLevel", "tokenCount", "secretary", "author"} {
		if IsCredentialParam(name) {
			t.Errorf("IsCredentialParam(%q) = true — over-broad", name)
		}
	}
}
