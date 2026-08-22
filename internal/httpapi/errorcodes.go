package httpapi

import "sort"

// ErrorCode is the machine-readable `error` field of every JSON error body.
//
// It is a named type, not a bare string, so the compiler can point at the one
// place codes are defined. That is NOT the whole guard — an untyped string
// constant still converts implicitly, so `errStatus(400, "typo", …)` would
// compile — which is why TestErrorCodesAreConstants parses this package's own
// source and fails on any call site that passes a literal instead of one of the
// constants below.
//
// WHY THE SET IS EXPORTED. The frontend asserts that every error fixture it
// ships names a code a handler can actually emit. A fixture naming
// `sudo_requird` is a test that passes forever while the screen it stands for is
// broken, and the only authority on the real spelling is the Go side. So the set
// is enumerable from outside the package, and the two tests below keep it honest
// in both directions: no constant is missing from ErrorCodes(), and no handler
// emits a code that is not a constant.
type ErrorCode string

// The codes handlers emit. Keep this list and errorCodes below in step; the
// tests fail if they drift.
//
// These are a wire contract. Renaming one is a breaking change for any client
// that branches on it, so a rename needs the frontend changed in the same commit
// — which is precisely what having one list makes possible to check.
const (
	// Generic request-shape and outcome codes.
	CodeBadRequest           ErrorCode = "bad_request"
	CodeConflict             ErrorCode = "conflict"
	CodeInternal             ErrorCode = "internal"
	CodeNotFound             ErrorCode = "not_found"
	CodeUnsupportedMediaType ErrorCode = "unsupported_media_type"

	// CodeEncode is the last-resort body written when the real response could
	// not be marshalled. It never travels through errStatus, because by then
	// encoding is exactly what has failed — see writeJSON.
	CodeEncode ErrorCode = "encode"

	// Authentication, authorization and the sudo window.
	CodeAlreadySetup ErrorCode = "already_setup"
	// CodeBusy answers a request shed by the Argon2id concurrency bound
	// (internal/crypto's maxKDFConcurrency), with 503. It is deliberately NOT
	// service_unavailable, which means an upstream *Arr did not answer: this one
	// is UsArr itself declining to start another password hash right now, and
	// the fix — wait a moment — is nothing like "check the service".
	CodeBusy         ErrorCode = "busy"
	CodeCSRF         ErrorCode = "csrf"
	CodeForbidden    ErrorCode = "forbidden"
	CodeSudoRequired ErrorCode = "sudo_required"
	CodeUnauthorized ErrorCode = "unauthorized"

	// Service configuration and health.
	CodeConnectionTestFailed ErrorCode = "connection_test_failed"
	// gosec G101 fires on the word "credential" in a string constant. This is
	// the opposite of a hardcoded credential: it is the code that tells the
	// browser a credential must be RE-ENTERED because UsArr refuses to reuse a
	// stored one against a host the user has just edited.
	CodeCredentialReentryRequired ErrorCode = "credential_reentry_required" //nolint:gosec // G101: an error code, not a secret
	CodeInvalidURLBase            ErrorCode = "invalid_url_base"
	CodeNotConfigured             ErrorCode = "not_configured"
	// CodeImportInProgress refuses a SECOND catalogue import for an instance
	// that already has one running. It is not a failure of the request: the
	// work the caller asked for is in flight, and the honest word for that is
	// "already running", never a false success.
	CodeImportInProgress ErrorCode = "import_in_progress"
	// CodeNotCatalogueSource answers a sync aimed at a service that has no
	// library to read — a Prowlarr is an indexer (ADR-0041). Distinct from
	// import_in_progress because the fixes are opposite.
	CodeNotCatalogueSource ErrorCode = "not_a_catalogue_source"
	// CodeNotDeltaSource answers a DELTA sync aimed at a service that has a
	// catalogue UsArr can read and no change feed to read it incrementally —
	// today, a Kavita. Distinct from not_a_catalogue_source because the service
	// is not the wrong kind: a full sync of it works, and that is the fix.
	//
	// ⚠️ THE SPELLING DIVERGES FROM internal/libsync's STORED CLASS ON PURPOSE,
	// AND THE NEXT READER'S INSTINCT WILL BE TO "FIX" IT.
	// internal/libsync/delta.go's errorClass DECLARES the literal
	// "no_delta_channel" for this same condition, against ErrNoDeltaChannel.
	// That is the STORAGE vocabulary; this is the WIRE vocabulary, and
	// DEVELOPMENT.md §11 ("a wire vocabulary and a storage vocabulary never
	// share a term") is explicit that the repair for a collision is distinct
	// spellings, never making the two values agree — because two vocabularies
	// that match today are free to diverge tomorrow, and one shared identifier
	// turns a change to a durable record into a change on the wire. So: distinct
	// by construction, and spelled to match its sibling not_a_catalogue_source
	// rather than to match the journal.
	//
	// 🚩 THAT errorClass ARM IS UNREACHABLE TODAY, AND THE DIVERGENCE STILL HAS
	// TO HOLD. Importer.DeltaSync returns ErrNoDeltaChannel at its DeltaSource
	// type assertion, which is ahead of every recordDeltaWalk call site, so no
	// sync_report.detail row carries "no_delta_channel" and the collision cannot
	// be observed in the database. It is declared rather than written — one edit
	// to where that assertion sits makes it written — and a vocabulary rule that
	// only took effect once the two values were both live would be a rule that
	// arrives after the identifier it governs has already been chosen.
	CodeNotDeltaSource ErrorCode = "not_a_delta_source"
	// CodeServiceDisabled answers a request that NAMED a service the user has
	// turned off. It is distinct from not_configured (nothing is set up) and
	// from no_indexer_service (nothing enabled to fall back to): here the
	// service exists and the one thing wrong is its Enabled flag, so the client
	// can offer to flip it.
	CodeServiceDisabled    ErrorCode = "service_disabled"
	CodeServiceUnavailable ErrorCode = "service_unavailable"

	// Libraries.
	//
	// CodeLibraryNameTaken refuses an Accept whose name this user cannot have,
	// with 409. It is its own code rather than the generic `conflict` for the
	// reason the two sync 409s are two codes: the fix is specific and typeable
	// — pick another name — and a client switching on `conflict` alone would
	// have to guess which sentence to show. ⚠️ It covers THREE store conditions
	// whose fix is the same one: the name is held at another kind, it is held by
	// the reserved `Unfiled` library that nothing may join, or the name is free
	// and the slug it reduces to is not. See acceptLibrariesError.
	CodeLibraryNameTaken ErrorCode = "library_name_taken"

	// CodeContainerAlreadyBound refuses an Accept that would bind one upstream
	// container into a SECOND library of the SAME kind, with 409. It is its own
	// code rather than CodeLibraryNameTaken because the fix is a different one:
	// the name is fine and changing it would not help — the container is the
	// thing already spoken for. ADR-0066 decision 5 licenses two libraries over
	// one container ref only at DIFFERENT kinds; see
	// store.ErrContainerBoundAtSameKind.
	CodeContainerAlreadyBound ErrorCode = "container_already_bound"

	// Search and grab.
	CodeExpired ErrorCode = "expired"
	// CodeGrabFailed asserts that the release did NOT reach the download
	// client. It used to be the unclassified remainder and carried three
	// different outcomes; the ones UsArr cannot make that assertion about now
	// have their own code below.
	CodeGrabFailed ErrorCode = "grab_failed"
	// CodeGrabOutcomeUnknown is the third outcome, and it is ADDITIVE rather
	// than a rename of grab_failed, so no client that branches on the old code
	// breaks. It means the release WAS sent to Prowlarr and neither UsArr nor
	// Prowlarr can say what the download client did with it.
	CodeGrabOutcomeUnknown ErrorCode = "grab_outcome_unknown"
	CodeInstanceMismatch   ErrorCode = "instance_mismatch"
	CodeNoDownloadClient   ErrorCode = "no_download_client"
	CodeNoIndexerService   ErrorCode = "no_indexer_service"
	CodeNoIndexers         ErrorCode = "no_indexers"
	CodeNoLongerOffered    ErrorCode = "no_longer_offered"
	CodeSearchFailed       ErrorCode = "search_failed"

	// Images.
	//
	// CodeNotCached answers a request for an image the caller IS entitled to
	// see and this server does not hold the bytes for. It is additive to
	// not_found rather than a narrowing of it, and the split is what lets a
	// client render §4.4.1's cold start honestly: a placeholder for a cover
	// that has not been fetched yet, and a broken-link state for a key that
	// names nothing. It discloses nothing extra — the caller got the key from a
	// browse response it was entitled to read, so "this asset exists" is not
	// news to it.
	CodeNotCached ErrorCode = "not_cached"
)

// errorCodes is the authoritative set. A map rather than a slice because every
// consumer either asks "is this one of them?" or enumerates, and a map answers
// the first in one line without a linear scan on every fixture.
var errorCodes = map[ErrorCode]struct{}{
	CodeAlreadySetup:              {},
	CodeBadRequest:                {},
	CodeBusy:                      {},
	CodeConflict:                  {},
	CodeConnectionTestFailed:      {},
	CodeCredentialReentryRequired: {},
	CodeCSRF:                      {},
	CodeEncode:                    {},
	CodeExpired:                   {},
	CodeForbidden:                 {},
	CodeGrabFailed:                {},
	CodeGrabOutcomeUnknown:        {},
	CodeInstanceMismatch:          {},
	CodeInternal:                  {},
	CodeImportInProgress:          {},
	CodeInvalidURLBase:            {},
	CodeContainerAlreadyBound:     {},
	CodeLibraryNameTaken:          {},
	CodeNoDownloadClient:          {},
	CodeNoIndexerService:          {},
	CodeNoIndexers:                {},
	CodeNoLongerOffered:           {},
	CodeNotCached:                 {},
	CodeNotCatalogueSource:        {},
	CodeNotConfigured:             {},
	CodeNotDeltaSource:            {},
	CodeNotFound:                  {},
	CodeSearchFailed:              {},
	CodeServiceDisabled:           {},
	CodeServiceUnavailable:        {},
	CodeSudoRequired:              {},
	CodeUnauthorized:              {},
	CodeUnsupportedMediaType:      {},
}

// ErrorCodes returns every code a handler may emit, sorted, so a test or a
// generator can enumerate them deterministically.
func ErrorCodes() []ErrorCode {
	out := make([]ErrorCode, 0, len(errorCodes))
	for c := range errorCodes {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// KnownErrorCode reports whether code is one a handler emits.
func KnownErrorCode(code ErrorCode) bool {
	_, ok := errorCodes[code]
	return ok
}
