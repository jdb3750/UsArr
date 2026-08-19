package bookorbit

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
	// ErrUnauthorized is the ONLY 401 sentinel in this package. parseErrorBody's
	// `case status == 401` arm branches on nothing, so every 401 from every route
	// unwraps to this one value. That mapping is UsArr's own and is the part of
	// this comment a reader can check here. (Cited by name and not by line: this
	// comment has already grown once and would take the line number with it.)
	//
	// The 403 arm a few lines below is the contrast worth reading beside it: 403
	// DOES discriminate, splitting one status into ErrForbidden and
	// ErrPasswordChangeRequired on an exact message match. The mechanism for
	// splitting one status by its message therefore exists in this same function,
	// and is not applied to 401.
	//
	// Collapsing costs less than it looks, because the upstream wire text survives
	// even though the sentinel does not vary: parseErrorBody fills APIError.Message
	// from the response body BEFORE this switch runs, and Error() renders it. The
	// one route where that is untrue is the magic-link login, where auth.go
	// OVERWRITES Message with a sentence of its own — so upstream's words reach a
	// caller everywhere except the one route this comment used to be about.
	//
	// WHAT UPSTREAM DOES IS NOT CHECKABLE FROM THIS TREE. This comment used to say
	// that MagicLinkService.loginWithToken collapses five conditions into one
	// message "deliberately"; that was its word and it is not adopted here.
	// BookOrbit's server source is not vendored — only packages/types is — so the
	// five-condition enumeration rests entirely on doc.go's reading of
	// bookorbit@73b7877d, and upstream's INTENT rests on nothing at all and is
	// recorded as UNKNOWN. Note that the message below names three conditions
	// rather than five, so it is not that enumeration's source either.
	//
	// On any other route it means the cached access token is stale, which this
	// client handles by re-minting once rather than by surfacing (see auth.go).
	// A 401 that survives the re-mint is the stored magic-link token failing.
	ErrUnauthorized = errors.New("bookorbit: unauthorized (bad, revoked or expired magic-link token)")

	// ErrForbidden is a 403: the JWT is valid and the account lacks the reach.
	// Distinct from ErrUnauthorized because the fix is different — a 401 says
	// re-enter the credential, a 403 says re-scope the account.
	ErrForbidden = errors.New("bookorbit: forbidden (the token is valid but its account lacks the permission)")

	// ErrPasswordChangeRequired is the ONE 403 that is neither of those, and it
	// gets its own sentinel because the fix is a third thing again.
	//
	// JwtAuthGuard.handleRequest throws ForbiddenException('Password change
	// required') when user.isDefaultPassword, on EVERY route that is not
	// @AllowDefaultPassword. At 73b7877d it cannot reach this client at all:
	// only UserService.createUser sets isDefaultPassword:true, nothing sets it
	// true again afterwards, and MagicLinkService.createToken will not mint
	// against a target whose provisioningMethod is not 'shared' — which
	// createUser's accounts never become. The sentinel exists so that an upstream
	// invariant BREAKING does not arrive as a generic auth failure: if this ever
	// fires, the account is in a state the mint path cannot produce, and the fix
	// is in BookOrbit rather than in the stored token.
	ErrPasswordChangeRequired = errors.New("bookorbit: the account must change its password before any API call succeeds")

	// ErrNotFound is a 404.
	ErrNotFound = errors.New("bookorbit: not found")

	// ErrValidation is a 400. Nest's global ValidationPipe runs with
	// whitelist:true and forbidNonWhitelisted:true (server/src/main.ts), so an
	// UNKNOWN body key is rejected rather than ignored — a request body here must
	// carry exactly the fields the DTO declares and nothing else.
	ErrValidation = errors.New("bookorbit: request rejected by validation")

	// ErrThrottled is a 429, and it has a sentinel because BookOrbit throttles the
	// route this client depends on most: POST /api/v1/auth/magic-links/login is
	// @Throttle({limit:10, ttl:60_000}) (auth.controller.ts). Ten mints a minute
	// is ample for a 15-minute token, but a caller that re-mints per request
	// would hit it — so this must be legible rather than collapsed into "5xx-ish
	// upstream trouble". It is a 4xx and does NOT trip the breaker.
	ErrThrottled = errors.New("bookorbit: rate limited by the upstream throttler")

	// ErrServer is any 5xx.
	ErrServer = errors.New("bookorbit: upstream server error")

	// ErrUnhealthy means GET /api/v1/health answered and reported a failing
	// check. It is NOT ErrServer even though the status code is 503: the host is
	// up, BookOrbit is running, and it has told us which indicator is down. That
	// is a diagnosis, and it must reach the Services screen as one.
	ErrUnhealthy = errors.New("bookorbit: instance reports a failing health check")

	// ErrUnexpectedStatus is any other non-2xx.
	ErrUnexpectedStatus = errors.New("bookorbit: unexpected status")

	// ErrBreakerOpen means the per-instance circuit breaker refused the call. The
	// request never left the process, so this is not evidence about the upstream.
	ErrBreakerOpen = errors.New("bookorbit: circuit breaker open")

	// ErrTimeout means the call exceeded its deadline.
	ErrTimeout = errors.New("bookorbit: deadline exceeded")

	// ErrDecode means the body was not the JSON this endpoint promises.
	ErrDecode = errors.New("bookorbit: cannot decode response")

	// ErrResponseTooLarge means the body ran past the client's byte bound. The
	// bound FAILS rather than truncating, for the reason internal/kavita gives:
	// a truncated list is indistinguishable from content deleted upstream and
	// would drive a destructive reconciliation sweep.
	ErrResponseTooLarge = errors.New("bookorbit: response body too large")

	// ErrWrongService means the host answered but is not a BookOrbit. A 200 on
	// /api/v1/health proves a Terminus health controller is there and nothing
	// more; the handshake that proves the rest is a successful magic-link mint.
	ErrWrongService = errors.New("bookorbit: instance is not a BookOrbit")

	// ErrNoCredential means no magic-link token is configured. Every /api/v1
	// route except health is behind the global JwtAuthGuard, so there is no
	// useful anonymous surface to fall back to.
	ErrNoCredential = errors.New("bookorbit: no magic-link token is configured")

	// ErrInvalidRequest is a client-side rejection: a bad BaseURL, a missing
	// HTTPClient, a body that could not be built.
	ErrInvalidRequest = errors.New("bookorbit: invalid request")
)

// APIError is every non-2xx answer from a BookOrbit.
//
// Error() never carries a URL and Path is the request path with no query string.
// That is belt-and-braces here rather than load-bearing as it is for Kavita —
// ADR-0052's whole point is that a BookOrbit credential never reaches a URL —
// but the discipline is identical because the day it stops being true is not
// announced.
type APIError struct {
	Op      string // "Health", "MagicLinkLogin", "AppInfo", …
	Method  string
	Path    string // path only, never a full URL
	Status  int
	Message string

	// ErrorCode is Nest's optional machine-readable code. GlobalExceptionFilter
	// passes it through when the thrown exception's response object carries one
	// — LoginErrorCode.ACCOUNT_LOCKED and the OidcErrorCode family are the
	// producers in the tree at 73b7877d.
	ErrorCode string

	// RetryAfter is the optional retryAfterSeconds hint from the same envelope.
	RetryAfter int

	// Validation carries the ValidationPipe's message ARRAY, flattened. Nest
	// sends `message` as a string for a hand-thrown exception and as a string[]
	// for a class-validator failure; both shapes have to survive parseErrorBody.
	Validation []string

	Err error // one of the sentinels above
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "bookorbit %s: %s %s -> %d", e.Op, e.Method, e.Path, e.Status)
	if e.ErrorCode != "" {
		fmt.Fprintf(&b, " [%s]", e.ErrorCode)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	for i, v := range e.Validation {
		if i == 0 {
			b.WriteString(": ")
		} else {
			b.WriteString("; ")
		}
		b.WriteString(v)
	}
	return b.String()
}

func (e *APIError) Unwrap() error { return e.Err }

// nestError is the ONE error envelope BookOrbit produces.
//
// server/src/main.ts installs GlobalExceptionFilter as the only global filter
// and it catches EVERYTHING (`@Catch()` with no argument), so there is no second
// shape to sniff for — unlike Kavita, which emits ASP.NET
// ValidationProblemDetails on one path and a bare quoted string on another.
//
// `message` is json.RawMessage because it is polymorphic: a string from a
// hand-thrown HttpException, a []string from the global ValidationPipe.
type nestError struct {
	StatusCode int             `json:"statusCode"`
	Message    json.RawMessage `json:"message"`
	Path       string          `json:"path"`
	Timestamp  string          `json:"timestamp"`
	RequestID  string          `json:"requestId"`
	ErrorCode  string          `json:"errorCode"`
	RetryAfter int             `json:"retryAfterSeconds"`
}

// maxValidationMessages bounds how many of a message[] array reach the APIError.
//
// forbidNonWhitelisted:true means one wrong body can produce a complaint per
// rejected key, and the whole array ends up in service_instance.last_error and
// in a log line. Ten is enough to diagnose and short enough to read.
const maxValidationMessages = 10

// parseErrorBody turns a non-2xx response into an *APIError.
//
// Status is classified first and the body only ever ADDS detail, so an
// unparseable or empty body degrades to "the status, and nothing invented".
func parseErrorBody(op, method, path string, status int, body []byte) *APIError {
	e := &APIError{Op: op, Method: method, Path: path, Status: status}

	trimmed := strings.TrimSpace(string(body))
	if trimmed != "" && trimmed[0] == '{' {
		var ne nestError
		if err := json.Unmarshal(body, &ne); err == nil {
			e.ErrorCode = clean(ne.ErrorCode)
			e.RetryAfter = ne.RetryAfter
			e.Message, e.Validation = decodeNestMessage(ne.Message)
		} else {
			e.Message = clean(trimmed)
		}
	} else if trimmed != "" {
		e.Message = clean(trimmed)
	}

	switch {
	case status == 400:
		e.Err = ErrValidation
	case status == 401:
		e.Err = ErrUnauthorized
	case status == 403:
		// The one 403 with a specific, actionable cause. Matching on the message
		// is unavoidable — Nest attaches no code to it — so the comparison is
		// exact and case-insensitive against the literal in
		// common/guards/jwt-auth.guard.ts, and anything else stays a plain
		// ErrForbidden rather than being guessed at.
		if strings.EqualFold(e.Message, passwordChangeRequiredMessage) {
			e.Err = ErrPasswordChangeRequired
		} else {
			e.Err = ErrForbidden
		}
	case status == 404:
		e.Err = ErrNotFound
	case status == 429:
		e.Err = ErrThrottled
	case status >= 500:
		e.Err = ErrServer
	default:
		e.Err = ErrUnexpectedStatus
	}
	return e
}

// passwordChangeRequiredMessage is JwtAuthGuard.handleRequest's literal.
const passwordChangeRequiredMessage = "Password change required"

// decodeNestMessage resolves the polymorphic `message` field.
func decodeNestMessage(raw json.RawMessage) (msg string, list []string) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return clean(s), nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for i, v := range arr {
			if i >= maxValidationMessages {
				list = append(list, fmt.Sprintf("… and %d more", len(arr)-maxValidationMessages))
				break
			}
			list = append(list, clean(v))
		}
		return "", list
	}
	// Something else entirely — an object, a number. Render the raw bytes
	// through the same redaction and bound rather than dropping the only
	// diagnostic the response carried.
	return clean(string(raw)), nil
}

// clean is what every byte of upstream response-body text passes through before
// it reaches an APIError, and there is deliberately no branch of parseErrorBody
// that skips it.
//
// Two jobs, in this order. Redaction first, because APIError.Error() renders
// Message AND Validation into a string that reaches slog and
// service_instance.last_error, and BookOrbit's own envelope ECHOES THE REQUEST
// PATH back in `path` — an upstream that quotes a URL in an error message is not
// hypothetical here, it is the default shape. Truncation second, so the bound
// applies to the redacted form.
//
// The redactor is ssrf.RedactText and NOT a copy of it: internal/ssrf/redact.go
// states the rule — there is ONE deny-list, and a second copy is how a value
// ends up redacted on one code path and not the other. Its limit carries over
// unchanged: a bare secret outside a URL is not caught by shape, which is why
// the request-side guards in the tests exist as well.
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
