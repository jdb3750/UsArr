package servarr

import (
	"github.com/jdb3750/UsArr/internal/ssrf"
)

// RedactedPlaceholder is what a stripped credential is replaced with in a HEADER
// or a request body. It is a visible marker rather than an empty string so a
// redacted value still reads as "there was a secret here".
//
// URLs are redacted by ssrf.RedactRawURL and carry that package's placeholder;
// this constant deliberately does not try to unify the two, because the URL
// redactor is shared with the logging middleware and owns its own vocabulary.
const RedactedPlaceholder = "<redacted>"

// RedactURL strips credential-bearing query parameters and any userinfo from a
// URL so it is safe to log, store or return in an error.
//
// This exists because Prowlarr's SearchController.MapReleases rewrites every
// result's downloadUrl and magnetUrl into
// `{serverUrl}{urlBase}/{indexerId}/download?apikey={ApiKey}&link=…&file=…`,
// embedding Prowlarr's FULL ADMIN API KEY in plaintext in two fields of every
// search result. A single log line at info level would leak the vault.
//
// It delegates to internal/ssrf so there is exactly one deny-list of credential
// parameter names in the codebase. A second, drifting copy is how the OpenSubsonic
// salt/token/password triple ends up redacted in one code path and not the other.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}
	return ssrf.RedactRawURL(raw)
}

// SanitizeRelease returns a copy of r with every credential-bearing field removed.
//
// Call this on the boundary toward anything that is not the grab path: the HTTP
// API, SSE payloads, logs, support bundles — and, above all, PERSISTENCE. A
// credential written to SQLite is in every VACUUM INTO backup and every support
// bundle forever, so the store is the strictest of those boundaries, not the
// most relaxed one.
//
// Two different treatments, because the fields are two different things:
//
//   - DownloadURL and MagnetURL are DROPPED. They are Prowlarr proxy links into
//     which SearchController.MapReleases splices Prowlarr's own FULL ADMIN API
//     KEY. A redacted proxy link is useless to a client anyway, and keeping the
//     shape invites someone to "fix" the redaction.
//   - InfoURL, CommentURL and PosterURL are REDACTED, not dropped. These are
//     indexer-supplied, and a private tracker routinely carries the user's
//     personal PASSKEY in them as a query parameter — a leaked passkey means
//     account termination, because it is what the tracker attributes traffic by.
//     They are still what tells a human which tracker and which release page a
//     grab came from, and provenance keeps that forever, so the host and path
//     are worth keeping and only the secret has to go.
//
// The redaction is ssrf.RedactRawURL: the ONE deny-list (apikey, passkey,
// torrent_pass, rsskey, authkey, …), shared with the logging middleware and the
// HTTP boundary, so a stored URL and a served URL can never disagree about what
// counts as a secret. Do not add a second list here.
//
// LIMIT, deliberately: the deny-list matches query PARAMETERS. A tracker that
// embeds a passkey in a path segment (…/rss/<passkey>/…) is not covered by it,
// here or anywhere else in UsArr. Fixing that belongs in internal/ssrf, once,
// not in a private copy in this function.
//
// MagnetURL is NOT a magnet: URI. It is an http(s) Prowlarr proxy link, so it
// cannot be handed to a torrent client. Use InfoHash to build a real magnet.
//
// PosterURL survives redaction, but it is an indexer-supplied URL read out of a
// response body: SSRF class `derived` (security.md §2). Never fetch it with the
// `configured` policy.
func SanitizeRelease(r ReleaseResource) ReleaseResource {
	out := r
	out.DownloadURL = nil
	out.MagnetURL = nil
	out.InfoURL = RedactURL(r.InfoURL)
	out.CommentURL = RedactURL(r.CommentURL)
	out.PosterURL = RedactURL(r.PosterURL)
	return out
}

// SanitizeIndexer returns a copy of ix with fields[] removed.
//
// IndexerResource.fields carries the indexer's own credentials (cookies, passkeys,
// API keys) under Field.privacy ∈ {password, apiKey, userName}. It must never
// reach the browser. The rest of the resource is safe.
func SanitizeIndexer(ix IndexerResource) IndexerResource {
	out := ix
	out.Fields = nil
	out.Presets = nil // presets are IndexerResource values and carry fields too
	return out
}

// SanitizeDownloadClient does the same for a download client's fields[].
func SanitizeDownloadClient(dc DownloadClientResource) DownloadClientResource {
	out := dc
	out.Fields = nil
	out.Presets = nil
	return out
}
