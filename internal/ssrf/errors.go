package ssrf

import (
	"errors"
	"fmt"
	"net/netip"
)

// Sentinel errors. Callers match with errors.Is; the http.Client wraps everything
// below in *url.Error, which unwraps cleanly.
var (
	// ErrInvalidOptions means the caller built an Options that cannot express a
	// safe policy — e.g. ClassConfigured with no AllowedHostPort. Failing here is
	// deliberate: a "sensible default" for a missing allowlist is a wide-open one.
	ErrInvalidOptions = errors.New("ssrf: invalid options")

	// ErrScheme is returned for anything that is not http or https.
	ErrScheme = errors.New("ssrf: scheme not permitted")

	// ErrHostNotAllowed means the request's host:port is not the one this client
	// was built for, and the fallback policy for the class refuses it.
	ErrHostNotAllowed = errors.New("ssrf: host not allowed for this request class")

	// ErrBlockedAddress means a resolved IP failed the address policy.
	ErrBlockedAddress = errors.New("ssrf: address blocked by egress policy")

	// ErrNoAddress means the hostname resolved to nothing usable.
	ErrNoAddress = errors.New("ssrf: hostname resolved to no address")

	// ErrTooManyRedirects means the hop budget was exhausted.
	ErrTooManyRedirects = errors.New("ssrf: too many redirects")

	// ErrRedirectScheme covers cross-scheme hops, including any downgrade to http.
	ErrRedirectScheme = errors.New("ssrf: redirect changes scheme")

	// ErrResponseTooLarge means the body exceeded the cap. It is returned from
	// Read so the caller sees a failure rather than a silently truncated body.
	ErrResponseTooLarge = errors.New("ssrf: response exceeds size cap")

	// ErrPinMismatch means the peer's SPKI did not match service_instance.tls_spki_pin.
	ErrPinMismatch = errors.New("ssrf: TLS SPKI pin mismatch")
)

// PolicyError is every refusal that came from UsArr's own egress policy rather
// than from the network. Callers distinguish the two with IsPolicyError: a policy
// refusal is a configuration or provenance bug and must not trip a circuit
// breaker or be retried, whereas a dial failure is ordinary upstream flakiness.
type PolicyError struct {
	Op    string     // "validate", "dial", "redirect", "tls", "body"
	Class Class      // policy class in force
	Host  string     // hostname or host:port, never a full URL (URLs carry credentials)
	Addr  netip.Addr // resolved address that failed, if any
	Err   error      // one of the sentinels above
}

func (e *PolicyError) Error() string {
	var where string
	switch {
	case e.Addr.IsValid() && e.Host != "":
		where = fmt.Sprintf(" (%s -> %s)", e.Host, e.Addr)
	case e.Host != "":
		where = fmt.Sprintf(" (%s)", e.Host)
	case e.Addr.IsValid():
		where = fmt.Sprintf(" (%s)", e.Addr)
	}
	return fmt.Sprintf("ssrf %s [%s]: %v%s", e.Op, e.Class, e.Err, where)
}

func (e *PolicyError) Unwrap() error { return e.Err }

// IsPolicyError reports whether err was UsArr refusing to make the request,
// as opposed to the request being made and failing.
func IsPolicyError(err error) bool {
	var pe *PolicyError
	return errors.As(err, &pe)
}

func policyErr(op string, class Class, host string, addr netip.Addr, err error) *PolicyError {
	return &PolicyError{Op: op, Class: class, Host: host, Addr: addr, Err: err}
}
