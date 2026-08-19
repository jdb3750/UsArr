// Package vcrscrub is the ONE credential scrubber for go-vcr cassettes, and the
// only supported way to open one.
//
// # Why this is a package and not a hook copied into each _test.go
//
// It was three copies — internal/servarr, internal/kavita and internal/releases
// each carried a `scrubInteraction` and its own
// `regexp.MustCompile("(?i)\\b(apikey|api_key|access_token|token|sig)=…")`, and
// internal/libsync's cassette harness carried NO hook at all. Those five-name
// regexes were a SECOND DENY-LIST beside internal/ssrf's, and it had already
// drifted: `signature`, `secret`, `secret_key`, `auth_token`, `passkey`,
// `torrent_pass`, `torrentpass`, `rsskey`, `authkey`, `apipasskey` and the
// OpenSubsonic `p`/`t`/`s` triple were all redacted by the logging path and NOT
// by the recording path. internal/ssrf/redact.go states the rule this violated
// — "This is the ONE deny-list. There is deliberately no second copy anywhere" —
// and a cassette is the worst possible place to lose that argument, because a
// cassette is committed to git.
//
// So: no name list lives here. Scrub asks ssrf.IsCredentialParam.
//
// The objection recorded in the old internal/releases copy — that sharing this
// "would put a go-vcr dependency in the shipped binary" — is answered by
// TestBinaryDoesNotLinkTheRecorder, which asks the go tool what
// ./cmd/usarr actually links. Nothing outside a _test.go imports this package,
// so the linker never reaches it; go.mod already carries go-vcr either way.
//
// # Why the recording path is where the fix belongs
//
// Kavita's /api/Image/* routes take the Auth Key as `?apiKey=` and REFUSE the
// `x-api-key` header (measured: header-only 400, query-only 200 — see
// internal/kavita's doc.go). So the moment a cover fetch is recorded, the
// request URL itself is a full-admin credential.
//
// `make secrets` is a HALF backstop here, and which half it is matters. Drilled
// against this repo's own .gitleaks.toml with a freshly generated uuid4, one
// probe per position, clean baseline in between:
//
//	url: …/api/Image/series-cover?seriesId=1&apiKey=<guid>   → exit 1, "leaks found: 1"
//	url: …/api/Opds/<the same guid>/series                   → exit 0, "no leaks found"
//
// The gate fires on the adjacent `apiKey=` keyword, not on the credential. It
// closes the LABELLED half of the class and misses the UNLABELLED half — and the
// unlabelled half is a shape Kavita actually uses, because its OPDS routes carry
// the Auth Key as a path segment. That is why this package strips the key in
// BOTH positions and why the on-disk scan has a tier that does not look at names
// at all; a guard that were green exactly where the gate is green would be two
// defences failing on the same half.
//
// A cassette is written once and read forever, so the only place the credential
// can be stopped is before the bytes hit disk.
package vcrscrub

import (
	"net/http"
	"os"
	"regexp"
	"strings"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// Placeholder is what a stripped credential becomes in a HEADER, a body or a
// form value. It matches internal/servarr's and internal/kavita's
// RedactedPlaceholder, which is what the cassettes already on disk carry.
//
// URL query values become internal/ssrf's own `REDACTED` instead, because they
// are rewritten by ssrf.RedactRawURL and that redactor owns its vocabulary. Two
// spellings is not drift — it is two redactors, and PlaceholderValues is the
// single place that knows both.
const Placeholder = "<redacted>"

// ssrfPlaceholder is what ssrf.RedactURL writes into a query value. It is
// duplicated as a literal here rather than exported from ssrf on purpose: this
// package only ever COMPARES against it (in the on-disk scan), never produces
// it, so a copy cannot cause a redaction to be missed — only a scan finding to
// be reported that a human then reads.
const ssrfPlaceholder = "REDACTED"

// PlaceholderValues are every spelling a scrubbed value may legitimately have.
// The on-disk scan (ScanDir) treats anything else in a credential position as a
// leak.
var PlaceholderValues = []string{Placeholder, ssrfPlaceholder, "<unparseable url>"}

// scrubbedHeaders are rewritten wholesale, request side and response side both.
// Each one carries a bearer secret for some service in this ecosystem
// (docs/DEVELOPMENT.md §7.1).
//
// This is a list of HEADER NAMES, which is a different thing from
// internal/ssrf's list of PARAMETER names, and it is not a second copy of it:
// ssrf redacts URLs and has no concept of a header. The rule that there is one
// deny-list is about parameter names, and Scrub honours it by calling
// ssrf.IsCredentialParam for every value that has a parameter name attached.
var scrubbedHeaders = []string{
	"X-Api-Key",
	"Authorization",
	"Proxy-Authorization",
	"Set-Cookie",
	"Cookie",
	// qBittorrent / Transmission session handshakes.
	"X-Transmission-Session-Id",
	// Kavita's own outbound credentials, in case a capture picks one up.
	"X-License-Key",
	"X-Anilist-Token",
}

// urlHeaders hold a URL rather than a secret. Replacing them wholesale would
// destroy the one thing the cassette is recording; they are REDACTED instead, so
// the hop is still legible and the query string is not.
//
// Location is the one that matters: a 302 from an authenticated endpoint hands
// the whole query string, credential included, to the next hop.
var urlHeaders = []string{"Location", "Content-Location", "Referer"}

// queryPairRe finds a `name=value` pair anywhere in a string: a URL query, an
// x-www-form-urlencoded body, or a URL embedded inside a JSON response — which
// is the shape Prowlarr's SearchController.MapReleases produces when it splices
// the full admin key into every result's downloadUrl and magnetUrl.
//
// It deliberately matches EVERY name and lets ssrf.IsCredentialParam decide.
// Baking the names into the pattern is exactly the drift this package exists to
// end.
var queryPairRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_.-]*)=([^&"'\s\;]+)`)

// jsonPairRe finds a `"name": "value"` pair. A credential can come BACK in a
// body as well as go out in one: Kavita's UserDto — what
// POST /api/Plugin/authenticate returns — carries `token`, `refreshToken` and
// `apiKey` as sibling JSON fields, and all three are now in ssrf's list.
var jsonPairRe = regexp.MustCompile(`"([A-Za-z_][A-Za-z0-9_.-]*)"(\s*:\s*)"((?:[^"\\]|\\.)*)"`)

// guidRe matches a canonical GUID. See redactGUIDSegments.
var guidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// RedactURL is the URL redactor BOTH the save hook and the replay matcher use.
//
// 🚩 That both use the same function is the whole reason a scrubbed cassette can
// still replay. The recorded URL says `?apiKey=REDACTED`; the live request
// carries the real key. Comparing raw URLs would mean an interaction could only
// match by having STORED the credential — the exact outcome this package exists
// to prevent. Redacting the incoming URL too makes the two agree by
// construction, and it is a no-op for every URL that carries no credential.
//
// Two consequences, named rather than discovered later:
//
//   - ssrf.RedactURL re-encodes the query with url.Values.Encode, which SORTS
//     parameters. Both sides go through it, so both sort identically and
//     matching is unaffected — but a recorded URL is not byte-identical to what
//     the client sent, and that is why the cassettes' `url:` lines read sorted.
//   - Two URLs that differ ONLY in a credential value collapse to the same
//     string and become interchangeable at replay. No test in this tree records
//     one endpoint twice with two different keys, and if one ever does, the
//     matcher will silently pick the first. It is a real limit of matching on a
//     redacted URL and the alternative is storing the key.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}
	return ssrf.RedactRawURL(redactGUIDSegments(raw))
}

// redactGUIDSegments replaces a GUID that appears as a whole PATH SEGMENT.
//
// internal/ssrf's path-segment heuristic does not catch this and is right not
// to: looksLikeCredential rejects anything holding a separator, so a hyphenated
// GUID falls straight through. That calibration is deliberate there — ssrf
// writes provenance rows that are immutable, so it is biased towards missing a
// passkey rather than eating a real path segment.
//
// HERE THE BIAS IS INVERTED, and that is why this lives in this package rather
// than in ssrf. A cassette is a fixture that a person authored and will read: an
// over-redaction shows up as `<redacted>` in a diff before the commit, which is
// loud and cheap. An under-redaction is a full-admin credential in git history,
// which is neither.
//
// The shape is not hypothetical. Kavita's OPDS routes take the Auth Key as a
// PATH SEGMENT — internal/kavita's doc.go names them — so `/api/opds/<guid>/…`
// has no parameter name for any deny-list to match on.
func redactGUIDSegments(raw string) string {
	if !strings.Contains(raw, "-") {
		return raw
	}
	// Split the whole string on '/' rather than parsing: this runs before
	// url.Parse and must not care whether the input parses.
	parts := strings.Split(raw, "/")
	changed := false
	for i, p := range parts {
		if guidRe.MatchString(p) {
			parts[i] = ssrfPlaceholder
			changed = true
		}
	}
	if !changed {
		return raw
	}
	return strings.Join(parts, "/")
}

// RedactBody replaces credential values in a request or response body.
//
// Both passes consult ssrf.IsCredentialParam. Neither knows any names.
func RedactBody(s string) string {
	if s == "" {
		return s
	}
	s = queryPairRe.ReplaceAllStringFunc(s, func(m string) string {
		g := queryPairRe.FindStringSubmatch(m)
		if !ssrf.IsCredentialParam(g[1]) {
			return m
		}
		return g[1] + "=" + Placeholder
	})
	s = jsonPairRe.ReplaceAllStringFunc(s, func(m string) string {
		g := jsonPairRe.FindStringSubmatch(m)
		if !ssrf.IsCredentialParam(g[1]) {
			return m
		}
		return `"` + g[1] + `"` + g[2] + `"` + Placeholder + `"`
	})
	return s
}

// Scrub is the BeforeSave hook. It is MANDATORY, and it is installed by Recorder
// rather than offered as an option, because an option is one forgotten line away
// from a full-admin key in the repository and `make secrets` is only a
// mechanical backstop that a bare-GUID Kavita key walks straight past.
//
// In replay-only runs it never fires, which is why TestScrubDrill exercises it
// against a real recording — and why TestCassettesOnDiskCarryNoCredential exists
// to catch a cassette recorded with it bypassed.
func Scrub(i *cassette.Interaction) error {
	for _, h := range scrubbedHeaders {
		ck := http.CanonicalHeaderKey(h)
		if _, ok := i.Request.Headers[ck]; ok {
			i.Request.Headers.Set(h, Placeholder)
		}
		if _, ok := i.Response.Headers[ck]; ok {
			i.Response.Headers.Set(h, Placeholder)
		}
	}
	for _, h := range urlHeaders {
		if v := i.Request.Headers.Get(h); v != "" {
			i.Request.Headers.Set(h, RedactURL(v))
		}
		if v := i.Response.Headers.Get(h); v != "" {
			i.Response.Headers.Set(h, RedactURL(v))
		}
	}

	i.Request.URL = RedactURL(i.Request.URL)
	i.Request.Body = RedactBody(i.Request.Body)
	i.Response.Body = RedactBody(i.Response.Body)

	// The parsed form is a second copy of the query string and needs the same
	// treatment. It asks the same deny-list, so it cannot disagree with the URL.
	if f := i.Request.Form; f != nil {
		for k := range f {
			if ssrf.IsCredentialParam(k) {
				f.Set(k, Placeholder)
			}
		}
	}
	return nil
}

// Matcher matches an interaction on method plus REDACTED URL, and nothing else.
//
// go-vcr's default matcher also compares every request header, which cannot work
// alongside scrubbing: the saved X-Api-Key is the placeholder while the live
// request carries the real key.
//
// Request bodies are deliberately not matched on either. Where a body's exact
// bytes matter they are pinned against the SPEC by a contract test, which is a
// stronger check than pinning them against a fixture this repo also wrote; and
// the Prowlarr grab retry after a cache miss relies on replaying the next
// unreplayed interaction in order.
func Matcher(r *http.Request, i cassette.Request) bool {
	return r.Method == i.Method && RedactURL(r.URL.String()) == RedactURL(i.URL)
}

// Mode reports which recorder mode to use. USARR_RECORD=1 selects ModeRecordOnce
// — real calls go out and a cassette is written where none exists — and anything
// else is ModeReplayOnly.
//
// ⚠️ Until this package landed, USARR_RECORD WAS READ BY internal/config AND BY
// NOTHING ELSE: every recorder in the tree hard-coded ModeReplayOnly, while four
// doc comments, docs/DEVELOPMENT.md §7.2 and docs/CONFIGURATION.md all told the
// reader to re-record with it. The documented path did not exist, which meant the
// first person to actually re-record would have wired their own recorder — and
// the hook is the thing they would have wired without.
func Mode() recorder.Mode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("USARR_RECORD"))) {
	case "1", "true", "yes", "on":
		return recorder.ModeRecordOnce
	}
	return recorder.ModeReplayOnly
}

// New opens a cassette with the scrubbing hook and the redacting matcher already
// installed. It is the only supported way to open one: every caller in the tree
// goes through it, so there is no per-package recorder wiring left to forget the
// hook.
//
// path is the cassette path WITHOUT the .yaml suffix, as go-vcr expects.
func New(path string, extra ...recorder.Option) (*recorder.Recorder, error) {
	opts := []recorder.Option{
		recorder.WithMode(Mode()),
		recorder.WithMatcher(Matcher),
		recorder.WithHook(Scrub, recorder.BeforeSaveHook),
		recorder.WithSkipRequestLatency(true),
	}
	return recorder.New(path, append(opts, extra...)...)
}
