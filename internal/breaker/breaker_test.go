package breaker_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/breaker"
)

var errOpen = errors.New("test: circuit breaker open")

// TestAllowReturnsTheInjectedSentinel is the whole reason this package takes one.
//
// Before the lift, internal/servarr and internal/kavita each carried a copy of
// the state machine SOLELY so that Allow() could return their own error —
// internal/libsync and internal/releases both match on it with errors.Is, and a
// Kavita failure reading `servarr: circuit breaker open` was the outcome nobody
// wanted. If this test ever passes with a shared sentinel, the copies were
// removed for nothing.
func TestAllowReturnsTheInjectedSentinel(t *testing.T) {
	b := breaker.New(breaker.Config{FailureThreshold: 1}, errOpen, time.Now, func() float64 { return 0.5 })
	b.Failure()

	err := b.Allow()
	if !errors.Is(err, errOpen) {
		t.Fatalf("Allow() = %v, want the injected sentinel", err)
	}
}

// TestNewRefusesANilSentinel: a nil there would make an open breaker
// indistinguishable from a closed one at every call site, silently.
func TestNewRefusesANilSentinel(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New accepted a nil sentinel; an open breaker would then return nil and pass every gate")
		}
	}()
	breaker.New(breaker.Config{}, nil, nil, nil)
}

func TestDefaultsAreTheArchitectureNumbers(t *testing.T) {
	cfg := breaker.Config{}.WithDefaults()
	if cfg.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5 (ARCHITECTURE.md §7.5)", cfg.FailureThreshold)
	}
	if cfg.BaseCooldown != 5*time.Second {
		t.Errorf("BaseCooldown = %v, want 5s", cfg.BaseCooldown)
	}
	if cfg.MaxCooldown != 15*time.Minute {
		t.Errorf("MaxCooldown = %v, want 15m", cfg.MaxCooldown)
	}
	if cfg.JitterFraction != 0.2 {
		t.Errorf("JitterFraction = %v, want 0.2", cfg.JitterFraction)
	}
}

// TestHalfOpenAdmitsExactlyOneProbe. A second concurrent caller must wait rather
// than pile onto a host that has been failing.
func TestHalfOpenAdmitsExactlyOneProbe(t *testing.T) {
	now := time.Now()
	b := breaker.New(breaker.Config{FailureThreshold: 1, BaseCooldown: time.Second}, errOpen,
		func() time.Time { return now }, func() float64 { return 0.5 })

	b.Failure()
	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("State = %v, want open", got)
	}
	now = now.Add(2 * time.Second)

	if err := b.Allow(); err != nil {
		t.Fatalf("the probe was refused after the cooldown: %v", err)
	}
	if err := b.Allow(); !errors.Is(err, errOpen) {
		t.Errorf("a SECOND caller got through the half-open gate: %v", err)
	}

	b.Success()
	if got := b.State(); got != breaker.StateClosed {
		t.Errorf("State = %v after a successful probe, want closed", got)
	}
	if !b.RetryAt().IsZero() {
		t.Errorf("RetryAt = %v on a closed breaker, want zero", b.RetryAt())
	}
}

// TestBackoffDoublesAndCaps, with jitter pinned out.
func TestBackoffDoublesAndCaps(t *testing.T) {
	now := time.Now()
	b := breaker.New(breaker.Config{FailureThreshold: 1, BaseCooldown: time.Second, MaxCooldown: 4 * time.Second},
		errOpen, func() time.Time { return now }, func() float64 { return 0.5 })

	for _, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second} {
		b.Failure()
		if got := b.RetryAt().Sub(now); got != want {
			t.Errorf("cooldown = %v, want %v", got, want)
		}
		now = now.Add(want)
		if err := b.Allow(); err != nil { // consume the probe
			t.Fatalf("probe refused: %v", err)
		}
	}
}
