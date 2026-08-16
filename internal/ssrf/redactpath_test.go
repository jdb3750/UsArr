package ssrf

import (
	"net/url"
	"strings"
	"testing"
)

// The path-segment half of the passkey problem. The deny-list matches query
// parameters, so `https://tracker.example/rss/<passkey>/torrents` was untouched
// — the same leak with a different URL layout.
//
// Both directions are tested hard, and the second one matters more: a false
// positive here silently corrupts provenance, whose rows are immutable and
// permanent, so a mangled details URL can never be recovered. The shapes below
// are taken from Prowlarr's own log scrubber
// (src/NzbDrone.Common/Instrumentation/CleanseLogMessage.cs, `develop`), which
// is the closest thing to a census of real tracker URLs — see the citation on
// redactPathSegments.

func TestPathSegmentPasskeysAreRedacted(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		raw    string
		secret string
	}{
		{
			// The shape named in the task, and the generic private-tracker RSS
			// feed: a 32-hex passkey as a whole path segment.
			name:   "rss feed passkey segment",
			raw:    "https://tracker.example/rss/3f5a9c1e7b2d4086af13c95e6d20b874/torrents",
			secret: "3f5a9c1e7b2d4086af13c95e6d20b874",
		},
		{
			// TorrentLeech: /rss/download/<id>/<20-char key>/<name>.torrent.
			// 20 characters is the shortest real key upstream names, and is why
			// credentialSegmentMinLen is 20 and not 24.
			name:   "torrentleech rss download key",
			raw:    "https://www.torrentleech.org/rss/download/1234567/a1b2c3d4e5f6g7h8i9j0/Some.Release.torrent",
			secret: "a1b2c3d4e5f6g7h8i9j0",
		},
		{
			// UNIT3D: the rsskey is appended after a dot INSIDE the segment, so
			// the numeric id must survive and only the key go.
			name:   "unit3d torrent download rsskey",
			raw:    "https://blutopia.example/torrent/download/98765.9d4f2a7c1b8e60355fa1c2d7e9b04836",
			secret: "9d4f2a7c1b8e60355fa1c2d7e9b04836",
		},
		{
			name:   "announce url passkey",
			raw:    "https://tracker.example:2096/announce/7c1f9b23ae0546d8b1f3902e6ad4c857",
			secret: "7c1f9b23ae0546d8b1f3902e6ad4c857",
		},
		{
			name:   "api key as a path segment",
			raw:    "https://sharewood.example/api/x7k29fq4mz10rb85tvn3/last-torrents",
			secret: "x7k29fq4mz10rb85tvn3",
		},
		{
			// Uppercase base32, as RFC 4648 alphabets and several trackers emit.
			name:   "base32 uppercase key",
			raw:    "https://tracker.example/feed/MFRGGZDFMZTWQ2LKNNWG23TP/all",
			secret: "MFRGGZDFMZTWQ2LKNNWG23TP",
		},
		{
			// Mixed-case alphanumeric, the UNIT3D and Gazelle rsskey shape.
			name:   "mixed case alnum key",
			raw:    "https://tracker.example/rss/aB3xZ9qL7mR2tK5vN8pW/movies",
			secret: "aB3xZ9qL7mR2tK5vN8pW",
		},
		{
			// Both halves of the two-secret shape upstream matches with
			// /fetch/<32>/<32>.
			name:   "two adjacent key segments",
			raw:    "https://tracker.example/fetch/0f1e2d3c4b5a69788796a5b4c3d2e1f0/a0b1c2d3e4f506172839a4b5c6d7e8f9",
			secret: "a0b1c2d3e4f506172839a4b5c6d7e8f9",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RedactRawURL(tc.raw)
			if strings.Contains(got, tc.secret) {
				t.Errorf("RedactRawURL(%q) = %q\nleaked the path-segment passkey %q; on a private "+
					"tracker that means account termination", tc.raw, got, tc.secret)
			}
			if !strings.Contains(got, redactedValue) {
				t.Errorf("RedactRawURL(%q) = %q, want a %s marker so a reader can tell the key was there",
					tc.raw, got, redactedValue)
			}
			// The host must survive: which tracker a grab came from is the whole
			// reason provenance keeps the URL at all.
			u, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, u.Hostname()) {
				t.Errorf("RedactRawURL(%q) = %q, lost the host", tc.raw, got)
			}
		})
	}
}

// TestOrdinaryPathsSurviveRedaction is the half that protects provenance.
//
// Every entry here is a path a real service serves. If one of them comes back
// changed, the heuristic is eating data, and the correct response is to narrow
// the rule (or drop it and document the limit), never to delete the case.
func TestOrdinaryPathsSurviveRedaction(t *testing.T) {
	t.Parallel()

	untouched := []string{
		// The three named in the brief.
		"https://tracker.example/api/v1/search",
		"https://tracker.example/details/12345",
		"https://tracker.example/rss/movies/1080p",

		// UsArr's own routes, which go through the logging middleware on every
		// single request.
		"http://usarr.lan:7878/api/v1/services/12",
		"http://usarr.lan:7878/api/v1/releases/5/grab",
		"http://usarr.lan:7878/api/v1/services/health",
		"http://usarr.lan:7878/api/events",

		// *Arr and media-server routes.
		"http://sonarr.lan:8989/api/v3/series/lookup",
		"http://prowlarr.lan:9696/1/download",
		"http://navidrome.lan:4533/rest/getAlbumList2.view",
		"http://jellyfin.lan:8096/Items/a1b2c3/Images/Primary",

		// Human-readable slugs, including long ones with digits in them.
		"https://tracker.example/torrents/the-shawshank-redemption-1994",
		"https://tracker.example/browse/science-fiction-and-fantasy",
		"https://openlibrary.org/books/OL7353617M/The_Left_Hand_of_Darkness",
		"https://tracker.example/collection/2160pRemuxCollection",
		"https://tracker.example/user/averylongusernameindeed",

		// Release names, the single most common long path segment in this
		// ecosystem. Dot-separated, so every part is far under the floor.
		"https://tracker.example/torrent/Some.Movie.2019.1080p.BluRay.x264-GROUP",
		"https://tracker.example/t/The.Series.S01E01.2160p.WEB-DL.DDP5.1.H.265-CTRLHD",

		// Dates, ids and hashes that are SHORT. A 40-hex git sha would be
		// redacted, and that is accepted: nothing in this ecosystem puts one in
		// a URL a user reads.
		"https://tracker.example/2026/08/16/announcement",
		"https://tracker.example/torrent/9876543210",

		// Already redacted. Idempotence is what the provenance repair pass in
		// migration 0002's Go companion depends on.
		"https://tracker.example/rss/REDACTED/torrents",
	}

	for _, raw := range untouched {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := RedactURL(u)
			if got != raw {
				t.Errorf("RedactURL(%q) = %q\nan ordinary path was rewritten. A false positive here "+
					"silently corrupts provenance, which is immutable — narrow the heuristic in "+
					"redact.go rather than deleting this case.", raw, got)
			}
		})
	}
}

// TestOpaqueIDsInPathsAreRedacted records a KNOWN, ACCEPTED limit rather than a
// bug, so the next reader meets it here instead of in a confusing log line.
//
// A ULID, a UUID without hyphens, a git sha or any other opaque high-entropy
// identifier sitting in a path segment is redacted, because it is
// indistinguishable BY SHAPE from a passkey — that is the whole reason this has
// to be a heuristic. The cost is bounded: UsArr's own routes carry no path
// segment anywhere near 20 characters (its search ids ride the query string and
// the JSON body, not the path), and the direction of the error is the safe one.
// If a future route does put an opaque id in a path and the redaction hurts
// debugging, the fix is to log the id as its own structured field, not to widen
// this rule.
func TestOpaqueIDsInPathsAreRedacted(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://example.test/stream/01J8ZK3QW9YB4N7C2F6MXAT0RD",     // ULID
		"https://example.test/commit/9f2c1b7ae04d5386bb1f39026ad4c8", // git sha
	} {
		if got := RedactRawURL(raw); !strings.Contains(got, redactedValue) {
			t.Errorf("RedactRawURL(%q) = %q; the documented limit above says this IS redacted, so "+
				"either the heuristic changed or this comment is now stale — reconcile them", raw, got)
		}
	}
}

// TestRedactURLIsIdempotent pins the property the one-time provenance repair
// pass is built on: running the redactor over an already-redacted value changes
// nothing, so the pass can be re-run and can be applied to rows written after
// the write path was fixed.
func TestRedactURLIsIdempotent(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://tracker.example/rss/3f5a9c1e7b2d4086af13c95e6d20b874/torrents",
		"https://tracker.example/details.php?id=42&passkey=3f5a9c1e7b2d4086af13c95e6d20b874",
		"https://tracker.example/torrent/download/98765.9d4f2a7c1b8e60355fa1c2d7e9b04836",
		"https://joe:hunter2@tracker.example/rss/3f5a9c1e7b2d4086af13c95e6d20b874",
	} {
		once := RedactRawURL(raw)
		twice := RedactRawURL(once)
		if once != twice {
			t.Errorf("redaction is not idempotent for %q:\n once: %q\ntwice: %q", raw, once, twice)
		}
	}
}

// TestRedactedPathPreservesEncoding checks the escaped-path handling. Splitting
// the DECODED path would mis-segment a URL whose segment contains %2F and
// re-encoding would then change which resource it names.
func TestRedactedPathPreservesEncoding(t *testing.T) {
	t.Parallel()

	// %2F is a literal slash inside one segment, not a separator.
	const raw = "https://tracker.example/browse/a%2Fb/3f5a9c1e7b2d4086af13c95e6d20b874"
	got := RedactRawURL(raw)
	if strings.Contains(got, "3f5a9c1e") {
		t.Errorf("RedactRawURL(%q) = %q, leaked the key", raw, got)
	}
	if !strings.Contains(got, "a%2Fb") {
		t.Errorf("RedactRawURL(%q) = %q, re-encoded the escaped segment; the URL now names a "+
			"different resource", raw, got)
	}

	// A percent-escaped segment is never itself a credential candidate: it is
	// not an unbroken alphanumeric run.
	const encoded = "https://tracker.example/browse/%D0%9F%D1%80%D0%B8%D0%B2%D0%B5%D1%82%D0%9C%D0%B8%D1%80"
	if got := RedactRawURL(encoded); got != encoded {
		t.Errorf("RedactRawURL rewrote a percent-encoded segment: %q -> %q", encoded, got)
	}
}

// TestLooksLikeCredentialBoundaries pins the individual clauses, so a later
// change to one of them fails here with a name rather than silently in a URL.
func TestLooksLikeCredentialBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
		why  string
	}{
		{"3f5a9c1e7b2d4086af13c95e6d20b874", true, "32-hex passkey"},
		{"a1b2c3d4e5f6g7h8i9j0", true, "20 alnum, the shortest real key shape"},
		{"a1b2c3d4e5f6g7h8i9j", false, "19 chars is under the floor"},
		{"12345678901234567890123456789012", false, "all digits: an id, not a key"},
		{"abcdefghijklmnopqrstuvwxyzabcdef", false, "all letters: a word run, not a key"},
		{"3f5a9c1e-7b2d-4086-af13-c95e6d20", false, "a UUID carries hyphens; separators mean structure"},
		{"a1b2c3d4_e5f6g7h8_i9j0k1l2", false, "underscores are structure too"},
		{"REDACTED", false, "the marker itself, so redaction is idempotent"},
		{"2160pRemuxCollection", false, "readable: the word veto holds"},
		{"Season1Episode12Extras", false, "readable"},
		{"", false, "empty"},
	}

	for _, tc := range cases {
		if got := looksLikeCredential(tc.in); got != tc.want {
			t.Errorf("looksLikeCredential(%q) = %v, want %v (%s)", tc.in, got, tc.want, tc.why)
		}
	}
}

// TestStripCredentialsLeavesPathsAlone. Redaction is for values that are logged,
// stored or served. A redirect hop is none of those: removing a path segment
// changes which resource is being requested, so the next hop would 404.
func TestStripCredentialsLeavesPathsAlone(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://cdn.example/fetch/3f5a9c1e7b2d4086af13c95e6d20b874/cover.jpg?apikey=s")
	if err != nil {
		t.Fatal(err)
	}
	stripCredentials(u)
	if !strings.Contains(u.Path, "3f5a9c1e7b2d4086af13c95e6d20b874") {
		t.Errorf("stripCredentials rewrote a redirect target's path: %q", u.String())
	}
}
