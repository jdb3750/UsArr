package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
)

// Per-app API keys (client_credential) verify with a fast keyed hash, NOT
// Argon2id. This is deliberate and it is a security decision, not a shortcut.
//
// Argon2id is a password KDF: it is expensive on purpose because a human-chosen
// password has perhaps 30 bits of entropy and the cost is what stands between a
// stolen hash and an offline guess. An API key generated here has 256 bits of
// entropy from crypto/rand, so there is nothing to guess and no work factor to
// buy.
//
// Running Argon2id on every request would instead be a remote
// memory-exhaustion vector: at m=19456 KiB per verification, a few dozen
// concurrent requests carrying a garbage key would allocate hundreds of
// megabytes on a machine that is often a Raspberry Pi. Northbound OpenSubsonic
// clients poll; an unauthenticated `GET /rest/ping?apiKey=garbage` must cost
// almost nothing.
//
// The cheap pre-check is key_prefix: ux_cc_prefix makes the auth path a single
// indexed lookup followed by one HMAC and one constant-time compare.

// KeyPrefixLen is the number of leading characters stored in
// client_credential.key_prefix. It is the lookup key AND the form that appears
// in logs, so it must never be long enough to be useful to an attacker.
const KeyPrefixLen = 8

// clientKeyBytes is the entropy behind a generated API key.
const clientKeyBytes = 32

// ErrKeyMismatch means the presented API key did not match the stored hash.
var ErrKeyMismatch = errors.New("crypto: client credential does not match")

// ClientKey is a freshly minted per-app API key.
type ClientKey struct {
	// Secret is the full key. It is shown to the user exactly once and is
	// never stored, never logged and never returned by the API again.
	Secret string
	// Prefix is the first KeyPrefixLen characters of Secret, stored in
	// client_credential.key_prefix.
	Prefix string
	// Hash is HMAC-SHA256(serverKey, Secret), stored in
	// client_credential.key_hash.
	Hash []byte
}

// GenerateClientKey mints a per-app API key under the server key derived by
// DeriveClientCredentialKey.
//
// The key is base64url without padding so it survives a query string
// unescaped: northbound OpenSubsonic credentials travel in the query string by
// specification.
func GenerateClientKey(serverKey []byte) (ClientKey, error) {
	if len(serverKey) != DerivedKeyLen {
		return ClientKey{}, fmt.Errorf("crypto: client credential server key is %d bytes, want %d",
			len(serverKey), DerivedKeyLen)
	}
	raw := make([]byte, clientKeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return ClientKey{}, fmt.Errorf("crypto: generate client key: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	return ClientKey{
		Secret: secret,
		Prefix: secret[:KeyPrefixLen],
		Hash:   HashClientKey(serverKey, secret),
	}, nil
}

// HashClientKey computes the stored hash for an API key.
func HashClientKey(serverKey []byte, secret string) []byte {
	mac := hmac.New(sha256.New, serverKey)
	mac.Write([]byte(secret))
	return mac.Sum(nil)
}

// ClientKeyPrefix returns the lookup prefix for a presented key, and whether
// the key is long enough to have one.
//
// A key shorter than KeyPrefixLen cannot match anything stored, and slicing it
// would panic; returning false lets the auth path reject it before it touches
// the database.
func ClientKeyPrefix(secret string) (string, bool) {
	if len(secret) < KeyPrefixLen {
		return "", false
	}
	return secret[:KeyPrefixLen], true
}

// VerifyClientKey checks a presented key against a stored hash in constant
// time.
func VerifyClientKey(serverKey []byte, secret string, storedHash []byte) error {
	if subtle.ConstantTimeCompare(HashClientKey(serverKey, secret), storedHash) != 1 {
		return ErrKeyMismatch
	}
	return nil
}
