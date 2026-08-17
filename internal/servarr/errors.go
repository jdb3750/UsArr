package servarr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Callers match with errors.Is; every transport-level failure is
// wrapped in *APIError, which unwraps to one of these.
var (
	// ErrUnauthorized is a 401. *Arr apps never return 403 for a bad key — the
	// authentication handler returns NoResult(), which ASP.NET turns into a 401
	// with an EMPTY BODY. Do not try to parse it.
	ErrUnauthorized = errors.New("servarr: unauthorized (bad or missing API key)")

	// ErrNotFound is a 404. On POST /api/v1/search it means the release fell out
	// of Prowlarr's in-process grab cache (non-rolling 30 min TTL, keyed
	// "{indexerId}_{guid}", populated only by a prior GET /api/v1/search, and lost
	// on process restart). The recovery is a re-search followed by a re-grab.
	ErrNotFound = errors.New("servarr: not found")

	// ErrConflict is a 409. On grab it is "Getting release from indexer failed".
	ErrConflict = errors.New("servarr: conflict")

	// ErrNoDownloadClient is a 500 whose message says no enabled download client
	// matches the release protocol. It is a configuration problem on the Prowlarr
	// side, not a UsArr bug, and it must reach the user as an instruction rather
	// than as a .NET stack trace.
	ErrNoDownloadClient = errors.New("servarr: no enabled download client for this protocol")

	// ErrValidation is a 400. Two body shapes reach here: a bare array of
	// FluentValidation failures, and ASP.NET's ValidationProblemDetails.
	ErrValidation = errors.New("servarr: request rejected by validation")

	// ErrServer is any other 5xx.
	ErrServer = errors.New("servarr: upstream server error")

	// ErrUnexpectedStatus is any other non-2xx.
	ErrUnexpectedStatus = errors.New("servarr: unexpected status")

	// ErrBreakerOpen means the per-instance circuit breaker refused the call. The
	// request never left the process, so this is not evidence about the upstream.
	ErrBreakerOpen = errors.New("servarr: circuit breaker open")

	// ErrTimeout means the call exceeded its deadline. Prowlarr's /api/v1/search
	// dispatches every indexer in parallel and answers only when the slowest has
	// answered or failed (15 s per-indexer HTTP timeout, 2 Polly retries), so
	// 45-60 s is a normal worst case and not a bug.
	ErrTimeout = errors.New("servarr: deadline exceeded")

	// ErrDecode means the body was not the JSON this endpoint promises.
	ErrDecode = errors.New("servarr: cannot decode response")

	// ErrResponseTooLarge means the body ran past the client's byte bound. It is
	// deliberately distinct from ErrDecode: the bound FAILS rather than truncating,
	// because a truncated *Arr list is indistinguishable from content deleted
	// upstream and would drive a destructive reconciliation sweep. On the buffered
	// path it also means "this endpoint belongs on StreamList".
	ErrResponseTooLarge = errors.New("servarr: response body too large")

	// ErrWrongService means system/status.appName was not the expected app. Sonarr
	// and Radarr answer the identical SystemResource shape on their own paths, so
	// appName is the only handshake that distinguishes them.
	ErrWrongService = errors.New("servarr: instance is not the expected application")

	// ErrUnsupportedAPIVersion means GET /api reported a `current` other than the
	// one this client speaks. Never derive the path from the app version: Prowlarr
	// 2.x still serves /api/v1.
	ErrUnsupportedAPIVersion = errors.New("servarr: unsupported API version")

	// ErrInvalidRequest is a client-side rejection: a search type that is not one
	// of the five Prowlarr accepts (an unrecognised value silently degrades to a
	// basic search server-side, which is worse than an error), or a typed criterion
	// that is not valid for the requested type.
	ErrInvalidRequest = errors.New("servarr: invalid request")
)

// ValidationFailure is one entry of the bare FluentValidation array returned on
// 400. Note the envelope is a JSON *array*, not the usual object.
type ValidationFailure struct {
	PropertyName              string `json:"propertyName"`
	ErrorMessage              string `json:"errorMessage"`
	AttemptedValue            any    `json:"attemptedValue,omitempty"`
	Severity                  any    `json:"severity,omitempty"`
	ErrorCode                 string `json:"errorCode,omitempty"`
	FormattedMessageArguments []any  `json:"formattedMessageArguments,omitempty"`
	IsWarning                 bool   `json:"isWarning,omitempty"`
}

// APIError is every non-2xx answer from an *Arr.
//
// Error() deliberately omits Description and never carries a URL: Description is a
// full .NET stack trace (log at debug only, never show a user) and a URL can carry
// ?apikey=. Path is the request path with no query string for the same reason.
type APIError struct {
	Op          string // "Search", "Grab", …
	Method      string
	Path        string // path only, never a full URL
	Status      int
	Message     string
	Description string // .NET stack trace — debug logging ONLY, never user-visible
	Validation  []ValidationFailure
	Err         error // one of the sentinels above
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "servarr %s: %s %s -> %d", e.Op, e.Method, e.Path, e.Status)
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	for i, v := range e.Validation {
		if i == 0 {
			b.WriteString(": ")
		} else {
			b.WriteString("; ")
		}
		if v.PropertyName != "" {
			fmt.Fprintf(&b, "%s %s", v.PropertyName, v.ErrorMessage)
		} else {
			b.WriteString(v.ErrorMessage)
		}
	}
	return b.String()
}

func (e *APIError) Unwrap() error { return e.Err }

// arrErrorEnvelope is envelope shape (1): the ordinary *Arr error object.
type arrErrorEnvelope struct {
	Message     string `json:"message"`
	Description string `json:"description"`
	Content     any    `json:"content"`
}

// problemDetails is envelope shape (3): ASP.NET model-binding failures.
type problemDetails struct {
	Type   string              `json:"type"`
	Title  string              `json:"title"`
	Status int                 `json:"status"`
	Detail string              `json:"detail"`
	Errors map[string][]string `json:"errors"`
}

// downloadClientMissingMarkers are the exact substrings Prowlarr's
// DownloadService emits when no enabled client matches the release protocol. They
// arrive as a 500 with a stack trace in `description`, which is useless to a user;
// matching them lets UsArr say "configure a torrent client in Prowlarr" instead.
var downloadClientMissingMarkers = []string{
	"Torrent Download client isn't configured yet",
	"Usenet Download client isn't configured yet",
	"Indexer specified download client is not available",
}

// parseErrorBody turns a non-2xx response into an *APIError.
//
// Decode order is status-first, then sniff, because the three envelopes are not
// mutually decodable: a bare array will not unmarshal into a struct, and
// ValidationProblemDetails and the *Arr error object are both JSON objects that
// share no required key.
func parseErrorBody(op, method, path string, status int, body []byte) *APIError {
	e := &APIError{Op: op, Method: method, Path: path, Status: status}

	// A 401 has an empty body. Parsing it produces a confusing "unexpected end of
	// JSON input" that hides the real cause, so short-circuit before any decode.
	if status == 401 {
		e.Err = ErrUnauthorized
		e.Message = "bad or missing API key"
		return e
	}

	trimmed := strings.TrimSpace(string(body))
	switch {
	case trimmed == "":
		// nothing to say beyond the status
	case trimmed[0] == '[':
		var vs []ValidationFailure
		if err := json.Unmarshal(body, &vs); err == nil && len(vs) > 0 {
			e.Validation = vs
			e.Message = vs[0].ErrorMessage
			break
		}
		e.Message = truncate(trimmed)
	case trimmed[0] == '{':
		var env arrErrorEnvelope
		if err := json.Unmarshal(body, &env); err == nil && (env.Message != "" || env.Description != "") {
			e.Message = env.Message
			e.Description = env.Description
			break
		}
		var pd problemDetails
		if err := json.Unmarshal(body, &pd); err == nil && (pd.Title != "" || len(pd.Errors) > 0) {
			e.Message = pd.Title
			if pd.Detail != "" {
				e.Message = pd.Title + ": " + pd.Detail
			}
			for field, msgs := range pd.Errors {
				for _, m := range msgs {
					e.Validation = append(e.Validation, ValidationFailure{PropertyName: field, ErrorMessage: m})
				}
			}
			break
		}
		e.Message = truncate(trimmed)
	default:
		e.Message = truncate(trimmed)
	}

	switch {
	case status == 400:
		e.Err = ErrValidation
	case status == 404:
		e.Err = ErrNotFound
	case status == 409:
		e.Err = ErrConflict
	case status >= 500:
		e.Err = ErrServer
		haystack := e.Message + "\n" + e.Description
		for _, marker := range downloadClientMissingMarkers {
			if strings.Contains(haystack, marker) {
				e.Err = ErrNoDownloadClient
				// Replace the message so nothing downstream is tempted to render the
				// stack trace that came with it.
				e.Message = marker
				e.Description = ""
				break
			}
		}
	default:
		e.Err = ErrUnexpectedStatus
	}
	return e
}

// truncate bounds an unrecognised body so a 2 MB HTML error page from a reverse
// proxy cannot end up inside an error string or a log line.
func truncate(s string) string {
	const max = 512
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
