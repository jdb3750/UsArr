package kavita

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// Sentinel errors. Callers match with errors.Is; every failure this package
// returns is an *APIError, which unwraps to one of these.
var (
	// ErrUnauthorized is a 401: the Auth Key is missing, wrong, or expired.
	// Kavita's Auth Keys can carry an expiry (GET /api/Plugin/authkey-expires),
	// so "it worked yesterday" is not evidence that it works now.
	ErrUnauthorized = errors.New("kavita: unauthorized (bad, missing or expired Auth Key)")

	// ErrForbidden is a 403, and it is NOT the same failure as a 401 — which is
	// why this package has a sentinel the *Arr client does not need.
	//
	// Kavita authorizes per policy: ServerController is
	// [Authorize(PolicyGroups.AdminPolicy)] at class level, so a PERFECTLY VALID
	// non-admin Auth Key gets 403 from GET /api/Server/server-info-slim while
	// every series and library call it needs succeeds. Collapsing that into
	// ErrUnauthorized would tell the user to re-enter a key that is fine.
	ErrForbidden = errors.New("kavita: forbidden (the Auth Key is valid but its account lacks the role)")

	// ErrNotFound is a 404.
	ErrNotFound = errors.New("kavita: not found")

	// ErrValidation is a 400. Kavita binds request bodies with System.Text.Json
	// and validates enum members with [EnumDataType], so a body carrying an
	// integer that is not a member of the C# enum is rejected here rather than
	// silently defaulted. See SeriesFilter.
	ErrValidation = errors.New("kavita: request rejected by validation")

	// ErrServer is any 5xx.
	ErrServer = errors.New("kavita: upstream server error")

	// ErrUnexpectedStatus is any other non-2xx.
	ErrUnexpectedStatus = errors.New("kavita: unexpected status")

	// ErrBreakerOpen means the per-instance circuit breaker refused the call. The
	// request never left the process, so this is not evidence about the upstream.
	ErrBreakerOpen = errors.New("kavita: circuit breaker open")

	// ErrTimeout means the call exceeded its deadline.
	ErrTimeout = errors.New("kavita: deadline exceeded")

	// ErrDecode means the body was not the JSON this endpoint promises.
	ErrDecode = errors.New("kavita: cannot decode response")

	// ErrResponseTooLarge means the body ran past the client's byte bound. It is
	// deliberately distinct from ErrDecode: the bound FAILS rather than
	// truncating, because a truncated series list is indistinguishable from
	// content deleted upstream and would drive a destructive reconciliation
	// sweep. On the buffered path it also means "this endpoint needs the
	// streaming path".
	ErrResponseTooLarge = errors.New("kavita: response body too large")

	// ErrWrongService means the host answered but is not a Kavita. Nothing about
	// "200 on /api/Health" proves what is on the other end — the body is the bare
	// string Ok — so the handshake that proves it is server-info-slim or
	// Library/libraries returning a shape only Kavita returns.
	ErrWrongService = errors.New("kavita: instance is not a Kavita")

	// ErrInvalidRequest is a client-side rejection: a bad BaseURL, a missing
	// HTTPClient, a nil callback, or a filter body that could not be built.
	ErrInvalidRequest = errors.New("kavita: invalid request")
)

// APIError is every non-2xx answer from a Kavita.
//
// Error() never carries a URL: a Kavita URL can carry ?apiKey=. Path is the
// request path with no query string for the same reason.
type APIError struct {
	Op      string // "Health", "ServerInfo", "SeriesList", …
	Method  string
	Path    string // path only, never a full URL
	Status  int
	Message string

	// Validation carries ASP.NET's ValidationProblemDetails.errors, flattened.
	Validation []ValidationFailure

	Err error // one of the sentinels above
}

// ValidationFailure is one field-level complaint out of a 400.
type ValidationFailure struct {
	PropertyName string
	ErrorMessage string
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "kavita %s: %s %s -> %d", e.Op, e.Method, e.Path, e.Status)
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

// problemDetails is ASP.NET's ValidationProblemDetails, which is what Kavita's
// model binder emits on a 400. Kavita's own handlers more often return
// `BadRequest(string)` — a bare, localised, quoted JSON string — so both shapes
// have to survive parseErrorBody.
type problemDetails struct {
	Type   string              `json:"type"`
	Title  string              `json:"title"`
	Status int                 `json:"status"`
	Detail string              `json:"detail"`
	Errors map[string][]string `json:"errors"`
}

// parseErrorBody turns a non-2xx response into an *APIError.
//
// Status first, then sniff the body, because the shapes are not mutually
// decodable and because a 401 must not be parsed at all.
func parseErrorBody(op, method, path string, status int, body []byte) *APIError {
	e := &APIError{Op: op, Method: method, Path: path, Status: status}

	// A 401 from ASP.NET's authentication handler has an empty body. Parsing it
	// yields "unexpected end of JSON input", which hides the real cause.
	if status == 401 {
		e.Err = ErrUnauthorized
		e.Message = "bad, missing or expired Auth Key"
		return e
	}

	trimmed := strings.TrimSpace(string(body))
	switch {
	case trimmed == "":
		// nothing to say beyond the status
	case trimmed[0] == '{':
		var pd problemDetails
		if err := json.Unmarshal(body, &pd); err == nil && (pd.Title != "" || len(pd.Errors) > 0 || pd.Detail != "") {
			e.Message = pd.Title
			if pd.Detail != "" {
				if e.Message == "" {
					e.Message = pd.Detail
				} else {
					e.Message = pd.Title + ": " + pd.Detail
				}
			}
			e.Message = clean(e.Message)
			for field, msgs := range pd.Errors {
				for _, m := range msgs {
					e.Validation = append(e.Validation, ValidationFailure{
						PropertyName: clean(field),
						ErrorMessage: clean(m),
					})
				}
			}
			break
		}
		e.Message = clean(trimmed)
	case trimmed[0] == '"':
		// BadRequest(await localizationService.TranslateAsync(...)) — a localised
		// message serialised as a bare JSON string. Unquote it so the user sees
		// the sentence and not the quotes.
		var s string
		if err := json.Unmarshal(body, &s); err == nil {
			e.Message = clean(s)
			break
		}
		e.Message = clean(trimmed)
	default:
		e.Message = clean(trimmed)
	}

	switch {
	case status == 400:
		e.Err = ErrValidation
	case status == 403:
		e.Err = ErrForbidden
	case status == 404:
		e.Err = ErrNotFound
	case status >= 500:
		e.Err = ErrServer
	default:
		e.Err = ErrUnexpectedStatus
	}
	return e
}

// clean is what every byte of upstream response-body text passes through before
// it reaches APIError, and there is deliberately no branch of parseErrorBody that
// skips it.
//
// Two separate jobs, in this order. Redaction first, because APIError.Error()
// renders Message AND Validation into a string that reaches slog and
// service_instance.last_error, and a Kavita error that quotes the URL it failed on
// quotes ?apiKey= with it (GET /api/Plugin/version takes the Auth Key in the
// query). Truncation second, so the bound applies to the redacted form.
//
// The redactor is ssrf.RedactText and NOT a copy of it: internal/httpapi's
// redact.go states the rule — two deny-lists drift, and the one that drifts is the
// one that leaks. Its limit carries over unchanged: a bare secret outside a URL is
// not caught by shape, which is why the request-side guards in vcr_test.go exist
// as well.
func clean(s string) string { return truncate(ssrf.RedactText(s)) }

// truncate bounds an unrecognised body so a 2 MB HTML error page from a reverse
// proxy cannot end up inside an error string or a log line.
func truncate(s string) string {
	const max = 512
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
