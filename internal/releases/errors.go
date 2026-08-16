package releases

import (
	"errors"

	"github.com/jdb3750/UsArr/internal/servarr"
)

var (
	// ErrNotConfigured means no Prowlarr client was supplied.
	ErrNotConfigured = errors.New("releases: no indexer service configured")

	// ErrForbidden means the caller's access scope does not cover the service
	// instance the request names. Authorization is enforced server-side from the
	// first commit and is never bolted on later (CLAUDE.md principle 4).
	ErrForbidden = errors.New("releases: instance not in caller's access scope")

	// ErrCandidateNotFound means no release_candidate row with that id is visible
	// to the caller. It is deliberately indistinguishable from "exists but is out
	// of scope", so a caller cannot probe for the existence of other users' rows.
	ErrCandidateNotFound = errors.New("releases: release candidate not found")

	// ErrCandidateExpired means the candidate is past expires_at. Prowlarr's grab
	// cache has already dropped it, or is about to; re-run the search.
	ErrCandidateExpired = errors.New("releases: release candidate expired, search again")

	// ErrNoDownloadClient means Prowlarr has no ENABLED download client whose
	// protocol matches this release. This is caught by preflight so the user gets
	// an instruction instead of a 500 with a .NET stack trace.
	ErrNoDownloadClient = errors.New("releases: no enabled Prowlarr download client for this protocol")

	// ErrGrabCacheMiss means the release was gone from Prowlarr's cache and the
	// transparent re-search RAN AND CAME BACK WITHOUT IT. Retrying will not help;
	// the release is no longer offered by that indexer.
	//
	// It is not the error for a re-search that failed to complete — that is
	// ErrReSearchFailed, and conflating the two told users a release had been
	// withdrawn when all that had happened was a 30-second timeout.
	ErrGrabCacheMiss = errors.New("releases: release is no longer available from that indexer")

	// ErrReSearchFailed means the grab missed Prowlarr's cache and the
	// transparent re-search that would have recovered it did not complete: a
	// timeout, an open breaker, a bad credential.
	//
	// Nothing was dispatched — the cache lookup and its 404 both precede
	// SendReportToClient — so this asserts failure honestly. What it must NOT
	// say is that the indexer no longer offers the release, which is a claim
	// about the indexer that a failed search is no evidence for.
	ErrReSearchFailed = errors.New("releases: the search that would have recovered this release did not complete")

	// ErrGrabOutcomeUnknown means the grab REACHED Prowlarr and the answer says
	// nothing about whether the release was added: a 5xx, a timeout, or a
	// cancellation after the POST left the process.
	//
	// This is the one outcome that is neither success nor failure, and it exists
	// because Prowlarr cannot tell the two apart either. It adds the release to
	// the download client FIRST and applies its configuration SECOND, with no
	// rollback, so a throw in the tail — Deluge's Label plugin rejecting a label
	// that was never created, which is the DEFAULT configuration — leaves the
	// torrent running and still returns a bare 500. A 200 is the only
	// confirmation the API offers. See docs/reference/search.md and
	// docs/reference/arr-apis.md for the upstream code.
	//
	// Reporting it as a failure is not a cosmetic error: it invites the user to
	// grab the same release twice, which on a private tracker costs a duplicate
	// download and a ratio hit.
	ErrGrabOutcomeUnknown = errors.New("releases: the release was sent and its outcome is unknown")

	// ErrNoIndexers means the instance has no enabled indexer that could serve the
	// query. The UI must say which of "none configured", "all disabled" and "all
	// blocked" applies — read the Report.
	ErrNoIndexers = errors.New("releases: no eligible indexer")

	// ErrInvalidQuery is a client-side rejection before anything leaves the process.
	ErrInvalidQuery = errors.New("releases: invalid query")

	// ErrRequestRejected means the *Arr refused the request UsArr constructed —
	// a 400 from its model binder or its validators.
	//
	// This is NOT an upstream failure and must not be reported as one. The service
	// is up, the credential works, the user did nothing wrong: UsArr sent a body
	// the other end would not bind. Mapping it to 502 tells the user their Prowlarr
	// is broken and offers them a "Test connection" button for a connection that is
	// working, which sends them looking in the one place the fault is not. That is
	// what happened when the grab body carried `"protocol":""`.
	ErrRequestRejected = errors.New("releases: the service rejected the request UsArr sent")
)

// UpstreamMessage returns the *Arr's OWN message from err, or "" when the error
// never carried one.
//
// It exists because internal/httpapi must not import internal/servarr (that
// package's doc.go), and a message that says "the download client reported an
// error" is useless without the error the download client reported.
//
// Two things it deliberately does not return. APIError.Description, which is a
// full .NET stack trace and is debug-logging only. And anything at all for a
// transport failure — a timeout or a cancellation has Status 0 and an
// UsArr-authored message like "deadline exceeded", and attributing that to
// Prowlarr would put words in its mouth. An empty result means "we have no
// answer from the service", which is a different sentence for a caller to write.
func UpstreamMessage(err error) string {
	var ae *servarr.APIError
	if errors.As(err, &ae) && ae.Status != 0 {
		return ae.Message
	}
	return ""
}
