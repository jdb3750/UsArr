package servarr

import (
	"time"

	"github.com/jdb3750/UsArr/internal/breaker"
)

// This file used to hold the state machine itself. internal/kavita copied it and
// its copy named the condition for undoing the duplication — "worth taking the
// first time a THIRD client needs one: lift this file into internal/breaker with
// an injected sentinel". internal/bookorbit is that third client, so the
// implementation moved to internal/breaker and what remains here is a wrapper.
//
// Allow() returns THIS package's ErrBreakerOpen, though nothing outside the
// package matches on it today: internal/releases builds per-indexer breakers
// through NewBreaker and renders OutcomeBreakerOpen from any non-nil Allow(),
// without inspecting the sentinel. It stays because the sentinel is this
// package's published error surface (errors.go), and because the precedent is
// live next door — internal/kavita needs its own so internal/libsync can tell a
// Kavita breaker from a Servarr one. The sentinel is now an argument to
// internal/breaker.New instead of a reason to keep a second copy.

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

// BreakerConfig is the tuning for one instance's breaker. The defaults are
// ARCHITECTURE.md §7.5: 5 failures to open, 5 s → 15 m capped backoff, ±20% jitter.
type BreakerConfig = breaker.Config

// Breaker is a per-instance circuit breaker.
//
// It is per-instance and NEVER global: Radarr being down must not stop Sonarr
// syncing, and in this package one dead Prowlarr indexer fan-out leg must not
// trip the legs to the other indexers. internal/releases gives each indexer its
// own Breaker for exactly that reason.
type Breaker = breaker.Breaker

// NewBreaker builds a breaker whose open sentinel is this package's
// ErrBreakerOpen. now and rnd may be nil; rnd must return [0,1).
func NewBreaker(cfg BreakerConfig, now func() time.Time, rnd func() float64) *Breaker {
	return breaker.New(cfg, ErrBreakerOpen, now, rnd)
}
