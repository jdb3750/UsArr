package crypto

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters, OWASP's second recommended configuration.
//
// These apply to USER PASSWORDS ONLY. See clientcred.go for why per-app API
// keys deliberately do not go through this function.
//
// There is no pepper. Earlier drafts referenced one twice and specified it
// nowhere, which is worse than not having one: a pepper silently present on one
// deploy and absent on another locks every user out with no diagnosis path.
const (
	argonMemoryKiB = 19456 // 19 MiB
	argonTime      = 2
	argonThreads   = 1
	argonSaltLen   = 16
	argonKeyLen    = 32
)

// ── the KDF concurrency bound ───────────────────────────────────────────────

// maxKDFConcurrency and kdfMaxWait bound how much Argon2id can be in flight at
// once. This is a CONCURRENCY bound, not a rate limit — the two solve different
// problems and only one of them is solved here.
//
// THE PROBLEM. Every login attempt runs Argon2id at m = 19 MiB, including for a
// username that does not exist: handleLogin burns a dummy hash on the
// unknown-account branch deliberately, so response time does not answer "does
// this account exist" (reference/security.md §6). That equaliser is not
// negotiable — trading a user-enumeration oracle for a memory bound is a bad
// swap — so the cheapest request an attacker can send, needing no valid
// username, allocates 19 MiB. Request volume therefore converts directly into
// memory pressure, and on a Pi or a small VM that is an OOM rather than a slow
// page.
//
// WHY A SEMAPHORE RATHER THAN A LIMITER. A limiter needs state (per-IP, per
// username), a policy, somewhere for the state to live, and a decision about
// what to do when it fills. A semaphore of N permits caps peak KDF memory at
// N × 19 MiB regardless of arrival rate, with none of that. It is not a
// substitute for a rate limiter — credential stuffing is still unthrottled, and
// FUTURE.md carries that half — it is the half that can be built with no state
// at all.
//
// SIZING, WRITTEN DOWN SO A READER CAN CHECK IT RATHER THAN RECOMPUTE IT. The
// bound is min(runtime.NumCPU(), 8) permits, and each permit covers exactly one
// argon2.IDKey call at m = 19456 KiB, so peak KDF memory is at most
// 8 × 19 MiB = 152 MiB on a box with 8 or more cores, and NumCPU × 19 MiB below
// that. That is deliberately loose. Real concurrency here is one person signing
// in, perceived speed is the owner's stated first requirement, and a bound tight
// enough to be felt in ordinary use would be a self-inflicted latency bug; a
// handful of permits is tens of megabytes on a box with gigabytes. The precedent
// is ARCHITECTURE.md §4.4, which puts all image transcoding behind a
// min(NumCPU, 4) semaphore citing jellyfin#9795 — that is a design statement for
// a pipeline that is not built yet, so it is the shape being reused, not shipped
// code being copied.
//
// PERMITS ARE PER CALL, NOT PER REQUEST, and a login can run the KDF twice: a
// successful login may rehash when the cost parameters have risen. Those two
// calls are SEQUENTIAL, so one request holds at most one permit at a time and
// the cap above is unaffected — a login that also rehashes acquires deliberately
// twice, one after the other, rather than holding a single permit across both.
// (The rehash arm does not exist today: NeedsRehash has no caller anywhere in
// the tree. When it gains one, a refused permit on the SECOND acquire must be
// logged and ignored, never turned into a failed login — the password was
// already verified, and the rehash is an optimisation.)
//
// THE WAIT IS BOUNDED, which is the difference between shedding load and
// converting a memory problem into an unbounded-goroutine one. A caller that
// cannot get a permit within kdfMaxWait gets ErrKDFBusy, which the HTTP layer
// renders as 503 rather than parking the request.
//
// The trade, stated rather than hidden: under load, login gets slow instead of
// the box falling over — degraded availability beats an OOM, but it is a trade.
// And a queue long enough to shed load sheds legitimate logins first, because an
// attacker retries and a person gives up. That is why the wait is seconds and
// not minutes.
const maxKDFConcurrency = 8

// kdfMaxWait is how long a caller waits for a permit before being shed. It is a
// var rather than a const for exactly one reason: internal/crypto's own drill
// shortens it so the test does not cost two seconds per assertion. Nothing in
// production writes it.
var kdfMaxWait = 2 * time.Second

// ErrKDFBusy means no Argon2id permit came free within kdfMaxWait.
//
// It is deliberately distinct from ErrPasswordMismatch: one is "the password is
// wrong", the other is "this server will not start another hash right now", and
// a caller that conflated them would report a bad password to a user whose
// password is fine.
var ErrKDFBusy = errors.New("crypto: the password hasher is saturated")

// kdfGate is the semaphore. A buffered channel rather than
// golang.org/x/sync/semaphore because x/sync is an indirect dependency today and
// a weighted semaphore buys nothing here: every acquisition is one unit.
var kdfGate = make(chan struct{}, kdfPermits())

func kdfPermits() int {
	if n := runtime.NumCPU(); n < maxKDFConcurrency {
		return n
	}
	return maxKDFConcurrency
}

// KDFPermits reports how many Argon2id hashes may run at once.
//
// Exported so the bound is inspectable rather than folklore — it is a number an
// operator sizing a container wants (permits × 19 MiB), and a test that wants to
// fill the gate needs it too.
func KDFPermits() int { return cap(kdfGate) }

// AcquireKDF reserves one Argon2id permit for a caller that will run several
// hashes back to back and wants them to count as one against the bound. The
// returned function releases it and must be called exactly once.
//
// ⚠️ DO NOT call HashPassword or VerifyPassword while holding a permit from
// this function. They acquire their own, so a caller holding the last permit
// would wait kdfMaxWait on itself and get ErrKDFBusy. The two styles are
// alternatives, not layers.
//
// NO PRODUCTION CALLER USES THIS TODAY, and that is stated rather than implied:
// every KDF call in the tree is a single hash, so each one acquires per call.
// It exists because the compound case is real and near — a login that rehashes
// on raised cost parameters runs the KDF twice — and because it is what lets a
// test fill the gate deterministically instead of racing a stopwatch.
func AcquireKDF(ctx context.Context) (func(), error) { return acquireKDF(ctx) }

// acquireKDF takes one permit, waiting at most kdfMaxWait.
//
// The uncontended case does not touch a timer at all: the first select's
// default arm makes a permit that is already free cost one channel send, so the
// semaphore is invisible when nothing is competing for it — which, on a
// single-user install, is essentially always.
func acquireKDF(ctx context.Context) (func(), error) {
	release := func() { <-kdfGate }
	select {
	case kdfGate <- struct{}{}:
		return release, nil
	default:
	}

	timer := time.NewTimer(kdfMaxWait)
	defer timer.Stop()
	select {
	case kdfGate <- struct{}{}:
		return release, nil
	case <-timer.C:
		return nil, ErrKDFBusy
	case <-ctx.Done():
		// A caller that has gone away frees the queue immediately. This is
		// ctx.Err(), not ErrKDFBusy: the server is not saturated, the client
		// left, and filing the two under one error would make a disconnect look
		// like an overload in the logs.
		return nil, ctx.Err()
	}
}

// ErrPasswordMismatch means the password did not match the stored hash. It
// carries no detail on purpose.
var ErrPasswordMismatch = errors.New("crypto: password does not match")

// HashPassword returns a full PHC string for storage in user.password_hash.
//
// The parameters are encoded in the string rather than assumed at verify time,
// so raising the cost later does not invalidate existing hashes.
//
// It goes through the same concurrency bound as VerifyPassword — see
// maxKDFConcurrency — so first-run setup and any future password change inherit
// the cap without their author having to know it exists. It returns ErrKDFBusy
// if no permit comes free within kdfMaxWait.
func HashPassword(ctx context.Context, password string) (string, error) {
	release, err := acquireKDF(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return hashPassword(password)
}

// hashPassword is HashPassword without the permit. It is split out so the gate
// wraps exactly the expensive line and nothing else, and so the bound has one
// place rather than being interleaved with the PHC formatting.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("crypto: generate password salt: %w", err)
	}
	sum := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

// VerifyPassword checks a password against a stored PHC string.
//
// It returns ErrPasswordMismatch for a wrong password, ErrKDFBusy if no permit
// came free within kdfMaxWait, and a different error again for a malformed or
// unsupported hash — because those need different responses: one is a failed
// login, one is a 503, and one is a corrupt row.
func VerifyPassword(ctx context.Context, phc, password string) error {
	params, salt, want, err := parsePHC(phc)
	if err != nil {
		return err
	}

	// The permit is taken AFTER parsing and BEFORE the only expensive line, so a
	// malformed row does not consume capacity. Parsing is bytes in memory and
	// costs nothing worth bounding.
	release, err := acquireKDF(ctx)
	if err != nil {
		return err
	}
	defer release()

	//#nosec G115 -- want is a decoded digest, bounded by the stored hash length.
	keyLen := uint32(len(want))
	got := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, keyLen)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// NeedsRehash reports whether a stored hash was produced with weaker
// parameters than the current constants, so a successful login can transparently
// upgrade it.
func NeedsRehash(phc string) (bool, error) {
	params, _, _, err := parsePHC(phc)
	if err != nil {
		return false, err
	}
	return params.memory < argonMemoryKiB || params.time < argonTime, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func parsePHC(phc string) (argonParams, []byte, []byte, error) {
	var p argonParams
	parts := strings.Split(phc, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash
	if len(parts) != 6 || parts[0] != "" {
		return p, nil, nil, fmt.Errorf("crypto: password hash is not a PHC string")
	}
	if parts[1] != "argon2id" {
		return p, nil, nil, fmt.Errorf("crypto: password hash algorithm %q is not argon2id", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return p, nil, nil, fmt.Errorf("crypto: password hash version field %q: %w", parts[2], err)
	}
	if version != argon2.Version {
		return p, nil, nil, fmt.Errorf("crypto: password hash version %d, want %d", version, argon2.Version)
	}

	var memory, timeCost, threads uint64
	for _, kv := range strings.Split(parts[3], ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return p, nil, nil, fmt.Errorf("crypto: password hash parameter %q", kv)
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return p, nil, nil, fmt.Errorf("crypto: password hash parameter %q: %w", kv, err)
		}
		switch k {
		case "m":
			memory = n
		case "t":
			timeCost = n
		case "p":
			threads = n
		default:
			return p, nil, nil, fmt.Errorf("crypto: unknown password hash parameter %q", k)
		}
	}
	if memory == 0 || timeCost == 0 || threads == 0 || threads > 255 {
		return p, nil, nil, fmt.Errorf("crypto: password hash parameters out of range")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, fmt.Errorf("crypto: password hash salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return p, nil, nil, fmt.Errorf("crypto: password hash digest: %w", err)
	}
	if len(salt) == 0 || len(want) == 0 {
		return p, nil, nil, fmt.Errorf("crypto: password hash has an empty salt or digest")
	}

	// Each value was parsed with ParseUint(..., 32) and range-checked above, so
	// the narrowing conversions cannot truncate.
	//#nosec G115 -- bounds enforced by ParseUint bitSize and the checks above.
	p = argonParams{memory: uint32(memory), time: uint32(timeCost), threads: uint8(threads)}
	return p, salt, want, nil
}
