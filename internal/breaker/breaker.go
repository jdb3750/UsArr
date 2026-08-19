// Package breaker is the per-service-instance circuit breaker every UsArr
// client uses. One implementation, three callers.
//
// # Why this package exists, and why it did not exist sooner
//
// internal/servarr wrote it first. internal/kavita copied it verbatim and said
// so in a comment that also named the condition for undoing the copy: "The seam
// for removing the copy is clean and is worth taking the first time a THIRD
// client needs one: lift this file into internal/breaker with an injected
// sentinel, and have both packages wrap it. Two copies is cheaper than a package
// that exists to serve two callers; three is not."
//
// internal/bookorbit is the third client. This is that lift, taken at the
// trigger the tree named rather than one commit later — and it is a CUT: two
// copies of the state machine become one, and the two clients keep their own
// exported names as aliases so nothing outside them changes.
//
// # The injected sentinel is the whole reason it was ever copied
//
// Neither client could import the other's breaker, because Allow() returns that
// package's ErrBreakerOpen and callers match on it with errors.Is. Importing
// internal/servarr would have put `servarr: circuit breaker open` inside a
// Kavita error, in a package whose own doc.go declares it "UsArr's single client
// for the *Arr family". So the sentinel is a CONSTRUCTOR ARGUMENT here: each
// client passes its own, and its callers' errors.Is checks are unchanged.
//
// # The tuning is ARCHITECTURE.md §7.5 and is not a knob to taste
//
// 5 failures to open, 5 s doubling to a 15 m cap, ±20% jitter. The jitter is not
// decoration: a user with six *Arrs behind one flaky reverse proxy would
// otherwise retry all six on the same tick, forever. Each client pins these
// numbers with its own test, so a change here fails three suites rather than
// drifting silently in one.
//
// A breaker is PER INSTANCE and NEVER global. One Kavita being down must not
// stop a Prowlarr grab; a second Kavita must count its own failures; and
// internal/releases gives each Prowlarr indexer fan-out leg its own, so one dead
// indexer does not trip the legs to the others.
package breaker

import (
	"math/rand/v2"
	"sync"
	"time"
)

// State is the circuit-breaker state per ARCHITECTURE.md §7.5.
type State int

const (
	// StateClosed passes every call through.
	StateClosed State = iota
	// StateOpen refuses every call until the cooldown expires.
	StateOpen
	// StateHalfOpen lets exactly one probe through.
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config is the tuning for one instance's breaker. The defaults are
// ARCHITECTURE.md §7.5: 5 failures to open, 5 s → 15 m capped backoff, ±20%
// jitter.
type Config struct {
	FailureThreshold int
	BaseCooldown     time.Duration
	MaxCooldown      time.Duration
	JitterFraction   float64
}

// WithDefaults fills in the §7.5 numbers for every zero field.
//
// It is EXPORTED, unlike the unexported `withDefaults` the two copies carried,
// because each client's "the defaults still match the architecture" test lives
// in that client's own package and can no longer reach an unexported method
// here. Those tests are the reason the numbers cannot drift, so they had to keep
// working.
func (c Config) WithDefaults() Config {
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
type Breaker struct {
	cfg     Config
	openErr error
	now     func() time.Time
	rand    func() float64

	mu            sync.Mutex
	state         State
	failures      int
	openings      int // consecutive open transitions, drives the exponential backoff
	retryAt       time.Time
	probeInFlight bool
}

// New builds a breaker.
//
// openErr is what Allow returns when the circuit is open — each client passes
// its OWN sentinel so that `errors.Is(err, kavita.ErrBreakerOpen)` and
// `errors.Is(err, bookorbit.ErrBreakerOpen)` keep meaning what they meant. It
// must not be nil: a nil there would make an open breaker indistinguishable from
// a closed one at every call site.
//
// now and rnd may be nil; rnd must return [0,1).
func New(cfg Config, openErr error, now func() time.Time, rnd func() float64) *Breaker {
	if openErr == nil {
		panic("breaker: New requires a non-nil open sentinel; see the doc comment")
	}
	if now == nil {
		now = time.Now
	}
	if rnd == nil {
		rnd = rand.Float64
	}
	return &Breaker{cfg: cfg.WithDefaults(), openErr: openErr, now: now, rand: rnd, state: StateClosed}
}

// Allow reports whether a call may proceed. It returns the client's open
// sentinel when the circuit is open, which callers must NOT treat as evidence
// about the upstream: the request never left the process.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return nil
	case StateOpen:
		if b.now().Before(b.retryAt) {
			return b.openErr
		}
		b.state = StateHalfOpen
		b.probeInFlight = true
		return nil
	case StateHalfOpen:
		// Exactly one probe at a time. A second concurrent caller waits for the
		// cooldown rather than piling onto a host that has been failing.
		if b.probeInFlight {
			return b.openErr
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
	b.state = StateClosed
	b.failures = 0
	b.openings = 0
	b.probeInFlight = false
	b.retryAt = time.Time{}
}

// Failure records a failed call. Only failures that are evidence about the
// upstream belong here: a 4xx is UsArr sending a bad request and an SSRF policy
// refusal never reached the network, so neither should trip the breaker. Each
// client filters those out before calling this.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.probeInFlight = false
	if b.state == StateHalfOpen {
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
	b.state = StateOpen
	b.failures = 0
	b.openings++
	b.retryAt = b.now().Add(b.cooldown())
}

// cooldown is base * 2^(openings-1), capped, with ±JitterFraction jitter.
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

// State reports the current state, transitioning open → half-open if the
// cooldown has elapsed. Health UI reads this; it does not consume the half-open
// probe slot.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == StateOpen && !b.now().Before(b.retryAt) {
		return StateHalfOpen
	}
	return b.state
}

// RetryAt reports when an open breaker will next admit a probe. Zero when
// closed.
func (b *Breaker) RetryAt() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == StateClosed {
		return time.Time{}
	}
	return b.retryAt
}
