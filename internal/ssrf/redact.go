package ssrf

import (
	"net/url"
	"strings"
)

// credentialParams are query parameters that carry a secret. TMDB v3, Fanart.tv
// and Comic Vine all authenticate by query parameter, and OpenSubsonic puts the
// salt/token/password triple in "s"/"t"/"p" — so the short, generic-looking names
// are the dangerous ones, not an over-reach.
//
// These are stripped in two places, and both matter: from any URL that is logged
// or stored (image_asset.source_url, the HTTP cache key), and from any redirect
// hop, because a redirect hands the whole query string to whatever the upstream
// nominated. Deriving cache keys from the stripped URL also means rotating a
// provider key does not invalidate the entire image cache.
//
// This is the ONE deny-list. There is deliberately no second copy anywhere: a
// drifting duplicate is how a parameter ends up redacted on one code path and
// not the other. ARCHITECTURE.md §14.5 item 5 and security.md §5 document it and
// must be updated together with it.
//
// `refresh_token` / `refreshtoken` are here because of a RESPONSE BODY, not a
// request: Kavita's UserDto carries `token`, `refreshToken` and `apiKey`
// together, and it is what POST /api/Plugin/authenticate returns
// (api/specs/kavita-develop.json, components.schemas.UserDto). Two of those
// three names were already covered and the third was not — found while working
// out what a go-vcr cassette could capture coming BACK from a server, which is
// the direction this list was not originally written for.
//
// PRIVATE-TRACKER PASSKEYS are in this list, and they are not hypothetical.
// Prowlarr's ReleaseResource.infoUrl and .commentUrl are indexer-supplied and
// are surfaced to the browser as info_url; a private tracker's announce and
// details URLs routinely carry the user's personal passkey as a query parameter
// under one of these names. Without them, UsArr would ship a tracker credential
// straight to the client — and a leaked passkey on a private tracker means
// account termination, because it is what the tracker uses to attribute traffic.
// Prowlarr's own indexer definitions name these fields, which is where the list
// comes from.
//
// Adding a name here also removes it from redirect targets (see
// stripCredentials). That is the reason to prefer long, specific names: a short
// generic one like "t" or "s" is a legitimate cache-buster or size parameter on
// plenty of CDNs, so widening this list with short names has a functional cost
// that the tracker-specific names do not.
var credentialParams = map[string]struct{}{
	// Provider / generic.
	"apikey":        {},
	"api_key":       {},
	"token":         {},
	"access_token":  {},
	"auth_token":    {},
	"refresh_token": {},
	"refreshtoken":  {},
	"sig":           {},
	"signature":     {},
	"secret":        {},
	"secret_key":    {},

	// OpenSubsonic salt/token/password.
	"p": {},
	"t": {},
	"s": {},

	// Private-tracker passkeys, as named by Prowlarr's indexer definitions.
	"passkey":      {},
	"torrent_pass": {},
	"torrentpass":  {},
	"rsskey":       {},
	"authkey":      {},
	"apipasskey":   {},
	"cookie":       {},
}

// redactedValue is what replaces a credential in a logged URL. It is not empty,
// so a reader can tell "the key was present" from "there was no key".
//
// It is deliberately a short, all-letter, digit-free token, which makes
// looksLikeCredential reject it and so makes redaction IDEMPOTENT: redacting an
// already-redacted URL is a no-op. The one-time provenance repair pass relies on
// that, and so does the double redaction on the grab path.
const redactedValue = "REDACTED"

// ─── Path-segment credentials ────────────────────────────────────────────────
//
// The deny-list above matches query PARAMETERS. Plenty of private trackers put
// the passkey in a PATH SEGMENT instead, so `https://tracker.example/rss/<passkey>/`
// sailed through untouched — the same defect with a different URL layout.
//
// There is no name to match on in a path, so this is a heuristic, and a
// heuristic that fires wrongly SILENTLY CORRUPTS PROVENANCE — those rows are
// immutable and permanent, and a mangled details URL cannot be recovered from
// them. So the rule below is deliberately biased towards missing a passkey
// rather than eating a real path segment, and every condition is a narrowing
// one.
//
// WHERE THE NUMBERS COME FROM. Prowlarr's own log scrubber,
// src/NzbDrone.Common/Instrumentation/CleanseLogMessage.cs on `develop`, is the
// closest thing to a census of real tracker URL shapes, because upstream had to
// enumerate them to stop leaking them into its logs. Its path-segment rules:
//
//	torrentleech\.org/rss/download/[0-9]+/(?<secret>[0-9a-z]+)
//	rss(24h)?\.torrentleech\.org/(?!rss)(?<secret>[0-9a-z]+)
//	/fetch/[a-z0-9]{32}/(?<secret>[a-z0-9]{32})
//	(?:sharewood)\.[a-z]{2,3}/api/(?<secret>[a-z0-9]{16,})/
//	(?<=beyond-hd\.[a-z]+/api/torrents/)(?<secret>[^&=][a-z0-9]+)
//	(?<=[a-z0-9-]+\.[a-z]+/torrent/download/\d+\.)(?<secret>[^&=][a-z0-9]+)   -- UNIT3D
//	(?:avistaz|exoticaz|cinemaz|privatehd)\.[a-z]{2,3}/rss/download/(?<secret>…)/(?<secret>…)\.torrent
//	announce(\.php)?(/|%2f|%3fpasskey%3d)(?<secret>[a-z0-9]{16,})
//
// Three things follow, and all three are load-bearing here:
//
//  1. Every one of them is an UNBROKEN run of [a-z0-9] — no hyphens, no
//     underscores. Human-readable path segments of any length essentially always
//     carry a separator (`the-shawshank-redemption`, `api_v1`), so requiring an
//     unbroken alphanumeric run does most of the discrimination on its own.
//  2. Where upstream has no host to anchor against it uses **16 or more**
//     characters (sharewood, announce). Those rules ARE anchored to a path
//     prefix, though, and this one is not — it runs on every segment of every
//     URL — so 16 is raised to 20. 20 still covers the shortest concrete key in
//     the list: TorrentLeech's RSS key is 20 alphanumerics.
//  3. UNIT3D's key sits after a dot inside a segment (`…/download/12345.<rsskey>`),
//     so segments are split on `.` and each part judged separately. That also
//     makes release-name segments safe for free: every dot-separated part of
//     `Some.Movie.2019.1080p.BluRay.x264-GRP` is far under 20.
//
// The rest of the rule is about what must NOT be touched. A part must hold at
// least one letter AND at least one digit, which saves numeric ids and word-run
// slugs (`theshawshankredemption`) at a cost of roughly 1e-7 for a 32-character
// hex key. And a part that reads like words survives regardless — see
// looksLikeWords.
//
// NOT applied to stripCredentials. Removing a path segment from a redirect
// target changes which resource is being asked for; redaction is for values that
// are logged, stored or served, and a redirect is neither.
const (
	// credentialSegmentMinLen is the shortest path part treated as a possible
	// credential. See the citation above: upstream's unanchored floor is 16, and
	// the shortest real key it names is TorrentLeech's 20.
	credentialSegmentMinLen = 20

	// wordVowelRatio and wordMaxConsonantRun define "this reads like words".
	// English prose sits near 0.38 vowels per character and rarely runs more
	// than five non-vowels together; hex sits near 0.125, because only `a` and
	// `e` of the sixteen hex digits are vowels. The thresholds are set well
	// inside the readable side of that gap so the veto errs towards leaving a
	// segment alone.
	wordVowelRatio      = 0.25
	wordMaxConsonantRun = 6
)

// redactPathSegments replaces credential-looking parts of an escaped URL path
// and reports whether anything changed.
//
// It works on the ESCAPED path, not on url.URL.Path. A percent-encoded `/`
// inside a segment decodes to a plain `/` in Path, so splitting the decoded form
// would mis-segment the URL and re-encoding it would change which resource it
// names. A percent-encoded part is not an unbroken alphanumeric run, so it is
// simply left alone, which is the conservative outcome.
func redactPathSegments(escapedPath string) (string, bool) {
	if escapedPath == "" {
		return escapedPath, false
	}
	changed := false
	segments := strings.Split(escapedPath, "/")
	for i, seg := range segments {
		if len(seg) < credentialSegmentMinLen {
			// Cheap reject: no dot-separated part can be longer than the whole.
			continue
		}
		parts := strings.Split(seg, ".")
		for j, part := range parts {
			if looksLikeCredential(part) {
				parts[j] = redactedValue
				changed = true
			}
		}
		if changed {
			segments[i] = strings.Join(parts, ".")
		}
	}
	if !changed {
		return escapedPath, false
	}
	return strings.Join(segments, "/"), true
}

// looksLikeCredential reports whether one dot-separated path part is long
// enough, unstructured enough and mixed enough to be a tracker passkey.
//
// Every clause narrows. Adding one makes the redactor miss more passkeys and eat
// fewer real paths; removing one does the reverse. Given that provenance is
// permanent, missing is the cheaper mistake.
func looksLikeCredential(s string) bool {
	if len(s) < credentialSegmentMinLen {
		return false
	}
	var letters, digits int
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letters++
		default:
			// Any separator or non-ASCII byte at all: a hyphen, an underscore,
			// a percent escape, a space. Real keys are unbroken; slugs are not.
			return false
		}
	}
	// A pure-digit part is an id. A pure-letter part is a word run. Both are
	// things people put in paths; a hex or base32 key is neither, with
	// probability that rounds to one at this length.
	if letters == 0 || digits == 0 {
		return false
	}
	return !looksLikeWords(s)
}

// looksLikeWords vetoes redaction for a part that reads like human text.
//
// It is the guard for the one shape the clauses above let through: a
// separator-free CamelCase or lowercase run with a number in it, of the
// `2160pRemuxCollection` kind. Two independent signals have to agree that it is
// readable, and the veto is generous on purpose — a false veto loses a passkey
// from a log line, a false redaction destroys a provenance row.
func looksLikeWords(s string) bool {
	vowels, run, longestRun := 0, 0, 0
	for i := range len(s) {
		if isVowel(s[i]) {
			vowels++
			run = 0
			continue
		}
		run++
		if run > longestRun {
			longestRun = run
		}
	}
	if longestRun > wordMaxConsonantRun {
		return false
	}
	return float64(vowels)/float64(len(s)) >= wordVowelRatio
}

// isVowel counts digits as non-vowels: they break up words, and hex keys are
// three-fifths digits, which is what makes the ratio test discriminate at all.
func isVowel(c byte) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
		return true
	}
	return false
}

func isCredentialParam(name string) bool {
	// Case-insensitive: apiKey, ApiKey and APIKEY are all the same parameter to
	// the services that accept them.
	_, ok := credentialParams[strings.ToLower(name)]
	return ok
}

// IsCredentialParam reports whether name is a credential-bearing parameter under
// the ONE deny-list above. Case-insensitive.
//
// It is exported for callers that redact something that is not a URL and so
// cannot go through RedactURL — the go-vcr cassette scrubber
// (internal/vcrscrub) is the reason it exists. That scrubber has to redact a
// credential carried in a request BODY, in a form encoding and as a JSON object
// KEY, none of which url.URL can parse. Exporting the predicate is what lets it
// consult this list instead of growing its own: a second, drifting copy is how a
// parameter ends up redacted on one code path and not the other, and a cassette
// is the code path where the drifting copy gets committed to git.
func IsCredentialParam(name string) bool {
	return isCredentialParam(name)
}

// RedactURL renders a URL safe to log: userinfo removed, credential query
// parameter values replaced, credential-looking path segments replaced. Every
// log line and every error message that names a URL goes through this — the
// logging middleware included. An *Arr API key is a full-admin credential and it
// is routinely in the URL you are tempted to print.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	clone.User = nil

	if esc, changed := redactPathSegments(clone.EscapedPath()); changed {
		// Set both halves the way url.URL.setPath would. RawPath is consulted
		// only when it is a valid encoding of Path, so a mismatch degrades to
		// re-escaping Path rather than to a wrong URL; setting both keeps the
		// original escaping of the segments that were not touched.
		if decoded, err := url.PathUnescape(esc); err == nil {
			clone.Path, clone.RawPath = decoded, esc
		}
	}

	if clone.RawQuery != "" {
		q := clone.Query()
		for name, values := range q {
			if !isCredentialParam(name) {
				continue
			}
			for i := range values {
				values[i] = redactedValue
			}
		}
		clone.RawQuery = q.Encode()
	}
	return clone.String()
}

// RedactRawURL is RedactURL for a string that may not parse. An unparseable URL
// is returned as a fixed placeholder rather than verbatim, because "it did not
// parse" is not evidence that it holds no secret.
func RedactRawURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}
	return RedactURL(u)
}

// stripCredentials removes credential query parameters outright (not redacted —
// removed) and drops userinfo. Used on redirect hops, where the goal is that the
// next host never sees the secret at all.
//
// KNOWN OVER-BREADTH, deliberately not fixed yet. This shares credentialParams
// with the redactor, so the OpenSubsonic short names `p`, `t` and `s` are
// stripped from redirect targets too. Those are correct for redaction and
// over-broad here: an upstream that redirects to `…?t=1699999999` (cache-buster),
// `?s=small` (size) or `?p=2` (page) has that parameter silently removed and
// serves a different resource, or a 400. Internet Archive and CDN redirects
// commonly use short names.
//
// It is left as-is for now because over-stripping fails closed and nothing in
// v0.1 follows redirects on a path where this bites — the image/artwork pipeline
// that fetches CDN URLs is a later milestone. Whoever lands that pipeline should
// split the lists: keep the full list for redaction, and use a narrower one here
// (apikey, api_key, access_token, auth_token, token, sig, signature, secret,
// secret_key, and the tracker passkey names — all long enough to be unambiguous),
// with a comment saying why the two differ. Splitting them now would create a
// second deny-list with no consumer to validate it against.
func stripCredentials(u *url.URL) {
	u.User = nil
	if u.RawQuery == "" {
		return
	}
	q := u.Query()
	var dropped bool
	for name := range q {
		if isCredentialParam(name) {
			q.Del(name)
			dropped = true
		}
	}
	if dropped {
		u.RawQuery = q.Encode()
	}
}

// RedactText strips credentials out of free-form text that may embed a URL.
//
// Upstream error strings are shown VERBATIM on the Services screen (§17.3), ride
// the SSE stream, reach slog and are stored in service_instance.last_error, and
// an upstream error routinely quotes the URL it failed on — which for Prowlarr
// carries ?apikey= and for Kavita's GET /api/Plugin/version carries ?apiKey=.
// Verbatim means "the actual error, not a placeholder"; it does not mean
// "including the key".
//
// It lives here rather than in a caller because of the one-implementation rule
// this file states above: the deny-list has exactly one copy, and so does the
// text scanner that feeds it. internal/httpapi and internal/kavita both call
// this; neither has its own.
//
// LIMIT, stated so no caller mistakes it for a general secret scrubber: a bare
// secret that is not inside a URL passes through untouched. The URL rewriting is
// RedactRawURL's; this function only finds the substrings to hand it.
func RedactText(s string) string {
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	if !strings.Contains(lower, "http://") && !strings.Contains(lower, "https://") {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		start := indexScheme(s[i:])
		if start < 0 {
			b.WriteString(s[i:])
			break
		}
		start += i
		b.WriteString(s[i:start])
		end := start
		for end < len(s) && !isURLTerminator(s[end]) {
			end++
		}
		// Trailing punctuation belongs to the sentence, not to the URL.
		for end > start && strings.ContainsRune(".,;:!?)]}'\"", rune(s[end-1])) {
			end--
		}
		b.WriteString(RedactRawURL(s[start:end]))
		i = end
	}
	return b.String()
}

func indexScheme(s string) int {
	lower := strings.ToLower(s)
	h := strings.Index(lower, "http://")
	s2 := strings.Index(lower, "https://")
	switch {
	case h < 0:
		return s2
	case s2 < 0:
		return h
	case h < s2:
		return h
	default:
		return s2
	}
}

func isURLTerminator(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '"' || c == '<' || c == '>'
}
