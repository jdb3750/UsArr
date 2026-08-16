package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

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

// ErrPasswordMismatch means the password did not match the stored hash. It
// carries no detail on purpose.
var ErrPasswordMismatch = errors.New("crypto: password does not match")

// HashPassword returns a full PHC string for storage in user.password_hash.
//
// The parameters are encoded in the string rather than assumed at verify time,
// so raising the cost later does not invalidate existing hashes.
func HashPassword(password string) (string, error) {
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
// It returns ErrPasswordMismatch for a wrong password and a different error for
// a malformed or unsupported hash, because those need different operator
// responses: one is a failed login, the other is a corrupt row.
func VerifyPassword(phc, password string) error {
	params, salt, want, err := parsePHC(phc)
	if err != nil {
		return err
	}
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
