package servarr

import (
	"math/rand/v2"
	"sync"
	"time"
)

// BreakerState is the circuit-breaker state per ARCHITECTURE.md §7.5.
type BreakerState int

const (
	// BreakerClosed passes every call through.
	BreakerClosed BreakerState = iota
	// BreakerOpen refuses every call until the cooldown expires.
	BreakerOpen
	// BreakerHalfOpen lets exactly one probe through.
	BreakerHalfOpen
)

func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "closed"
	case BreakerOpen:
		return "open"
	case BreakerHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// BreakerConfig is the tuning for one instance's breaker. The defaults are
// ARCHITECTURE.md §7.5: 5 failures to open, 5 s → 15 m capped backoff, ±20% jitter.
type BreakerConfig struct {
	FailureThreshold int
	BaseCooldown     time.Duration
	MaxCooldown      time.Duration
	JitterFraction   float64
}

func (c BreakerConfig) withDefaults() BreakerConfig {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.BaseCooldown <= 0 {
		c.BaseCooldown = 5 * time.Second
	}
	if c.MaxCooldown <= 0 {
		c.MaxCooldown = 15 * time.Minute
	}
	if c.JitterFraction <= 0 {
		c.JitterFraction = 0.2
	}
	return c
}

// Breaker is a per-instance circuit breaker.
//
// It is per-instance and NEVER global: Radarr being down must not stop Sonarr
// syncing, and in this package one dead Prowlarr indexer fan-out leg must not trip
// the legs to the other indexers. internal/releases gives each indexer its own
// Breaker for exactly that reason.
type Breaker struct {
	cfg  BreakerConfig
	now  func() time.Time
	rand func() float64

	mu            sync.Mutex
	state         BreakerState
	failures      int
	openings      int // consecutive open transitions, drives the exponential backoff
	retryAt       time.Time
	probeInFlight bool
}

// NewBreaker builds a breaker. now and rnd may be nil; rnd must return [0,1).
func NewBreaker(cfg BreakerConfig, now func() time.Time, rnd func() float64) *Breaker {
	if now == nil {
		now = time.Now
	}
	if rnd == nil {
		rnd = rand.Float64
	}
	return &Breaker{cfg: cfg.withDefaults(), now: now, rand: rnd, state: BreakerClosed}
}

// Allow reports whether a call may proceed. It returns ErrBreakerOpen when the
// circuit is open, which callers must NOT treat as evidence about the upstream:
// the request never left the process.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case BreakerClosed:
		return nil
	case BreakerOpen:
		if b.now().Before(b.retryAt) {
			return ErrBreakerOpen
		}
		b.state = BreakerHalfOpen
		b.probeInFlight = true
		return nil
	case BreakerHalfOpen:
		// Exactly one probe at a time. A second concurrent caller waits for the
		// cooldown rather than piling onto a host that has been failing.
		if b.probeInFlight {
			return ErrBreakerOpen
		}
		b.probeInFlight = true
		return nil
	}
	return nil
}

// Success records a completed call. It closes a half-open circuit and resets the
// backoff, so a recovered instance does not stay on a 15-minute cooldown.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = BreakerClosed
	b.failures = 0
	b.openings = 0
	b.probeInFlight = false
	b.retryAt = time.Time{}
}

// Failure records a failed call. Only failures that are evidence about the
// upstream belong here: a 4xx is UsArr's own bug and an SSRF policy refusal never
// reached the network, so neither should trip the breaker. The Client filters
// those out before calling this.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.probeInFlight = false
	if b.state == BreakerHalfOpen {
		// The probe failed: straight back to open with the next backoff step.
		b.trip()
		return
	}
	b.failures++
	if b.failures >= b.cfg.FailureThreshold {
		b.trip()
	}
}

// trip must be called with b.mu held.
func (b *Breaker) trip() {
	b.state = BreakerOpen
	b.failures = 0
	b.openings++
	b.retryAt = b.now().Add(b.cooldown())
}

// cooldown is base * 2^(openings-1), capped, with ±JitterFraction jitter. The
// jitter matters because a user with six *Arrs behind one flaky reverse proxy
// would otherwise retry all six on the same tick, forever.
func (b *Breaker) cooldown() time.Duration {
	d := b.cfg.BaseCooldown
	for i := 1; i < b.openings && d < b.cfg.MaxCooldown; i++ {
		d *= 2
	}
	if d > b.cfg.MaxCooldown {
		d = b.cfg.MaxCooldown
	}
	jitter := 1 + b.cfg.JitterFraction*(2*b.rand()-1)
	return time.Duration(float64(d) * jitter)
}

// State reports the current state, transitioning open → half-open if the cooldown
// has elapsed. Health UI reads this; it does not consume the half-open probe slot.
func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == BreakerOpen && !b.now().Before(b.retryAt) {
		return BreakerHalfOpen
	}
	return b.state
}

// RetryAt reports when an open breaker will next admit a probe. Zero when closed.
func (b *Breaker) RetryAt() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == BreakerClosed {
		return time.Time{}
	}
	return b.retryAt
}
