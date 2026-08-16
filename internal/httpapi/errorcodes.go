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
	// CodeServiceDisabled answers a request that NAMED a service the user has
	// turned off. It is distinct from not_configured (nothing is set up) and
	// from no_indexer_service (nothing enabled to fall back to): here the
	// service exists and the one thing wrong is its Enabled flag, so the client
	// can offer to flip it.
	CodeServiceDisabled    ErrorCode = "service_disabled"
	CodeServiceUnavailable ErrorCode = "service_unavailable"

	// Search and grab.
	CodeExpired          ErrorCode = "expired"
	CodeGrabFailed       ErrorCode = "grab_failed"
	CodeInstanceMismatch ErrorCode = "instance_mismatch"
	CodeNoDownloadClient ErrorCode = "no_download_client"
	CodeNoIndexerService ErrorCode = "no_indexer_service"
	CodeNoIndexers       ErrorCode = "no_indexers"
	CodeNoLongerOffered  ErrorCode = "no_longer_offered"
	CodeSearchFailed     ErrorCode = "search_failed"
)

// errorCodes is the authoritative set. A map rather than a slice because every
// consumer either asks "is this one of them?" or enumerates, and a map answers
// the first in one line without a linear scan on every fixture.
var errorCodes = map[ErrorCode]struct{}{
	CodeAlreadySetup:              {},
	CodeBadRequest:                {},
	CodeConflict:                  {},
	CodeConnectionTestFailed:      {},
	CodeCredentialReentryRequired: {},
	CodeCSRF:                      {},
	CodeEncode:                    {},
	CodeExpired:                   {},
	CodeForbidden:                 {},
	CodeGrabFailed:                {},
	CodeInstanceMismatch:          {},
	CodeInternal:                  {},
	CodeInvalidURLBase:            {},
	CodeNoDownloadClient:          {},
	CodeNoIndexerService:          {},
	CodeNoIndexers:                {},
	CodeNoLongerOffered:           {},
	CodeNotConfigured:             {},
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
