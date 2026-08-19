package crypto

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

// TestEveryKDFCallIsBounded is the drill for the concurrency cap.
//
// The bound is a property of the PRIMITIVE, not of the login handler, and this
// is the test that says so: it fills the gate and then checks that BOTH
// entry points — HashPassword, which first-run setup and any future password
// change use, and VerifyPassword, which login and sudo use — shed rather than
// allocating another 19 MiB. A bound that lived in the caller would pass a test
// aimed at login and silently miss the next caller someone adds.
func TestEveryKDFCallIsBounded(t *testing.T) {
	want := runtime.NumCPU()
	if want > maxKDFConcurrency {
		want = maxKDFConcurrency
	}
	if got := KDFPermits(); got != want {
		t.Fatalf("KDFPermits() = %d, want min(NumCPU=%d, %d) = %d",
			got, runtime.NumCPU(), maxKDFConcurrency, want)
	}

	// A short wait so the drill does not cost kdfMaxWait per assertion. This is
	// the only reason kdfMaxWait is a var rather than a const.
	restore := kdfMaxWait
	kdfMaxWait = 20 * time.Millisecond
	t.Cleanup(func() { kdfMaxWait = restore })

	releases := make([]func(), 0, KDFPermits())
	for i := 0; i < KDFPermits(); i++ {
		release, err := AcquireKDF(t.Context())
		if err != nil {
			t.Fatalf("permit %d of %d: %v", i, KDFPermits(), err)
		}
		releases = append(releases, release)
	}

	// Seeded through the ungated primitive: the test has deliberately given away
	// every permit, so it cannot ask for one to build its own fixture.
	phc, err := hashPassword("a good long passphrase")
	if err != nil {
		t.Fatalf("seed hash: %v", err)
	}

	if _, err := HashPassword(t.Context(), "a good long passphrase"); !errors.Is(err, ErrKDFBusy) {
		t.Errorf("HashPassword with the gate full = %v, want ErrKDFBusy — first-run setup and "+
			"any password change must inherit the cap, not sit outside it", err)
	}
	if err := VerifyPassword(t.Context(), phc, "a good long passphrase"); !errors.Is(err, ErrKDFBusy) {
		t.Errorf("VerifyPassword with the gate full = %v, want ErrKDFBusy", err)
	}

	// The wait really is bounded: the call above returned, rather than parking a
	// goroutine until something else finished. Converting a memory problem into
	// an unbounded-goroutine problem is not a fix.
	start := time.Now()
	_ = VerifyPassword(t.Context(), phc, "a good long passphrase")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("a shed call took %v; the wait is not bounded", elapsed)
	}

	// A cancelled caller is NOT reported as saturation: the server is fine, the
	// client left, and filing the two under one error makes a disconnect look
	// like an overload.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := VerifyPassword(cancelled, phc, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled caller got %v, want context.Canceled", err)
	}

	// Give one permit back and the very next call succeeds. Without this the
	// test would pass just as well against a gate that is permanently shut.
	releases[0]()
	if err := VerifyPassword(t.Context(), phc, "a good long passphrase"); err != nil {
		t.Errorf("VerifyPassword after one permit was released = %v, want success", err)
	}
	for _, release := range releases[1:] {
		release()
	}
}
