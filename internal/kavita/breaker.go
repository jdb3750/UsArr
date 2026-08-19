package kavita

import (
	"time"

	"github.com/jdb3750/UsArr/internal/breaker"
)

// This file used to be a DELIBERATE COPY of internal/servarr/breaker.go, and its
// comment named the condition for undoing the copy: "worth taking the first time
// a THIRD client needs one: lift this file into internal/breaker with an
// injected sentinel, and have both packages wrap it. Two copies is cheaper than
// a package that exists to serve two callers; three is not."
//
// internal/bookorbit is the third client, so the lift happened. What is left
// here is the WRAPPER, and it exists for exactly the reason the copy did:
// Allow() has to return THIS package's ErrBreakerOpen, because internal/libsync
// matches on it with errors.Is and a Kavita failure must not read
// `servarr: circuit breaker open`. The sentinel is now an argument instead of a
// second state machine.
//
// The tuning is unchanged (ARCHITECTURE.md §7.5: 5 failures to open, 5 s → 15 m
// capped backoff, ±20% jitter) and TestBreakerDefaultsMatchTheArchitecture
// Numbers still pins it from inside this package, so a change in
// internal/breaker fails here too.

// BreakerState is the circuit-breaker state per ARCHITECTURE.md §7.5.
type BreakerState = breaker.State

const (
	// BreakerClosed passes every call through.
	BreakerClosed = breaker.StateClosed
	// BreakerOpen refuses every call until the cooldown expires.
	BreakerOpen = breaker.StateOpen
	// BreakerHalfOpen lets exactly one probe through.
	BreakerHalfOpen = breaker.StateHalfOpen
)

// BreakerConfig is the tuning for one instance's breaker.
type BreakerConfig = breaker.Config

// Breaker is a per-instance circuit breaker.
//
// Per-instance and NEVER global: one Kavita being down must not stop a Prowlarr
// grab, and a second Kavita must count its own failures.
type Breaker = breaker.Breaker

// NewBreaker builds a breaker whose open sentinel is this package's
// ErrBreakerOpen. now and rnd may be nil; rnd must return [0,1).
func NewBreaker(cfg BreakerConfig, now func() time.Time, rnd func() float64) *Breaker {
	return breaker.New(cfg, ErrBreakerOpen, now, rnd)
}
