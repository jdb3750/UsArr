package servarr

import (
	"errors"
	"testing"
	"time"
)

// fixedClock lets the breaker's cooldowns be tested without sleeping.
type fixedClock struct{ t time.Time }

func (c *fixedClock) now() time.Time      { return c.t }
func (c *fixedClock) add(d time.Duration) { c.t = c.t.Add(d) }

// noJitter makes the ±20% jitter deterministic at exactly 1.0×.
func noJitter() float64 { return 0.5 }

func newTestBreaker(clk *fixedClock) *Breaker {
	return NewBreaker(BreakerConfig{}, clk.now, noJitter)
}

func TestBreakerOpensAfterFiveFailures(t *testing.T) {
	clk := &fixedClock{t: time.Unix(1_700_000_000, 0)}
	b := newTestBreaker(clk)

	for i := range 4 {
		b.Failure()
		if err := b.Allow(); err != nil {
			t.Fatalf("failure %d: breaker opened early: %v", i+1, err)
		}
	}
	b.Failure()
	if err := b.Allow(); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("after 5 failures: err = %v, want ErrBreakerOpen", err)
	}
	if got := b.State(); got != BreakerOpen {
		t.Errorf("state = %s, want open", got)
	}
}

func TestBreakerHalfOpenProbeAndRecovery(t *testing.T) {
	clk := &fixedClock{t: time.Unix(1_700_000_000, 0)}
	b := newTestBreaker(clk)
	for range 5 {
		b.Failure()
	}

	// First cooldown is the 5 s base.
	clk.add(4 * time.Second)
	if err := b.Allow(); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("before cooldown: err = %v, want ErrBreakerOpen", err)
	}
	clk.add(2 * time.Second)
	if err := b.Allow(); err != nil {
		t.Fatalf("after cooldown, one probe must be admitted: %v", err)
	}
	// Only one probe at a time.
	if err := b.Allow(); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("second concurrent probe: err = %v, want ErrBreakerOpen", err)
	}

	b.Success()
	if got := b.State(); got != BreakerClosed {
		t.Fatalf("state after a successful probe = %s, want closed", got)
	}
	if err := b.Allow(); err != nil {
		t.Fatalf("closed breaker refused a call: %v", err)
	}
}

func TestBreakerBackoffDoublesAndCaps(t *testing.T) {
	clk := &fixedClock{t: time.Unix(1_700_000_000, 0)}
	b := newTestBreaker(clk)

	open := func() time.Duration {
		start := clk.t
		for range 5 {
			b.Failure()
		}
		return b.RetryAt().Sub(start)
	}
	failProbe := func() time.Duration {
		start := clk.t
		if err := b.Allow(); err != nil {
			t.Fatalf("probe not admitted: %v", err)
		}
		b.Failure()
		return b.RetryAt().Sub(start)
	}

	if got := open(); got != 5*time.Second {
		t.Errorf("first cooldown = %s, want 5s", got)
	}
	clk.add(5 * time.Second)
	if got := failProbe(); got != 10*time.Second {
		t.Errorf("second cooldown = %s, want 10s", got)
	}
	// Drive it to the cap.
	for range 12 {
		clk.add(20 * time.Minute)
		failProbe()
	}
	if got := b.RetryAt().Sub(clk.t); got != 15*time.Minute {
		t.Errorf("capped cooldown = %s, want 15m", got)
	}

	// A success resets the backoff, so a recovered instance does not stay on a
	// 15-minute cooldown for the rest of the process's life.
	clk.add(20 * time.Minute)
	if err := b.Allow(); err != nil {
		t.Fatalf("probe not admitted: %v", err)
	}
	b.Success()
	if got := open(); got != 5*time.Second {
		t.Errorf("cooldown after recovery = %s, want the 5s base again", got)
	}
}

func TestBreakerJitterStaysWithinTwentyPercent(t *testing.T) {
	clk := &fixedClock{t: time.Unix(1_700_000_000, 0)}
	for _, r := range []float64{0, 0.25, 0.5, 0.75, 0.999} {
		b := NewBreaker(BreakerConfig{}, clk.now, func() float64 { return r })
		for range 5 {
			b.Failure()
		}
		got := b.RetryAt().Sub(clk.t)
		if got < 4*time.Second || got > 6*time.Second {
			t.Errorf("rand=%v gave cooldown %s, want 5s ±20%%", r, got)
		}
	}
}

func TestBreakersAreIndependent(t *testing.T) {
	clk := &fixedClock{t: time.Unix(1_700_000_000, 0)}
	a, b := newTestBreaker(clk), newTestBreaker(clk)
	for range 5 {
		a.Failure()
	}
	if err := a.Allow(); !errors.Is(err, ErrBreakerOpen) {
		t.Fatal("a should be open")
	}
	// Per-instance, never global: Radarr being down must not stop Sonarr syncing.
	if err := b.Allow(); err != nil {
		t.Fatalf("b was tripped by a's failures: %v", err)
	}
}
