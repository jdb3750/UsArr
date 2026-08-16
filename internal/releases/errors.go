package releases

import "errors"

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
	// transparent re-search did not find it again either. Retrying will not help;
	// the release is no longer offered by that indexer.
	ErrGrabCacheMiss = errors.New("releases: release is no longer available from that indexer")

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
