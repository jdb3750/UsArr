package crypto

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	const pw = "correct horse battery staple"

	phc, err := HashPassword(t.Context(), pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := VerifyPassword(t.Context(), phc, pw); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if err := VerifyPassword(t.Context(), phc, pw+"!"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("VerifyPassword with a wrong password = %v, want ErrPasswordMismatch", err)
	}
	if strings.Contains(phc, pw) {
		t.Fatal("the PHC string contains the password")
	}
}

// The stored form is a full PHC string carrying the parameters, so raising the
// cost later does not invalidate existing hashes.
func TestPasswordPHCFormat(t *testing.T) {
	phc, err := HashPassword(t.Context(), "hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("PHC string = %q, want the OWASP m=19456,t=2,p=1 parameters", phc)
	}
	if got := len(strings.Split(phc, "$")); got != 6 {
		t.Fatalf("PHC string has %d $-separated fields, want 6", got)
	}
}

// A per-hash salt means two users with the same password get different hashes.
func TestPasswordSaltIsPerHash(t *testing.T) {
	a, err := HashPassword(t.Context(), "same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword(t.Context(), "same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two hashes of the same password are identical; the salt is not per-hash")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	tests := []struct {
		name string
		phc  string
	}{
		{name: "empty", phc: ""},
		{name: "not PHC", phc: "plaintext"},
		{name: "wrong algorithm", phc: "$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"},
		{name: "wrong version", phc: "$argon2id$v=16$m=19456,t=2,p=1$c2FsdA$aGFzaA"},
		{name: "missing fields", phc: "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA"},
		{name: "unknown parameter", phc: "$argon2id$v=19$m=19456,t=2,p=1,x=9$c2FsdA$aGFzaA"},
		{name: "zero memory", phc: "$argon2id$v=19$m=0,t=2,p=1$c2FsdA$aGFzaA"},
		{name: "bad base64 salt", phc: "$argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyPassword(t.Context(), tc.phc, "anything")
			if err == nil {
				t.Fatal("VerifyPassword accepted a malformed hash")
			}
			// A corrupt row is a different operator problem from a failed
			// login, so it must not masquerade as one.
			if errors.Is(err, ErrPasswordMismatch) {
				t.Errorf("malformed hash reported as ErrPasswordMismatch: %v", err)
			}
		})
	}
}

func TestNeedsRehash(t *testing.T) {
	current, err := HashPassword(t.Context(), "x")
	if err != nil {
		t.Fatal(err)
	}
	need, err := NeedsRehash(current)
	if err != nil {
		t.Fatal(err)
	}
	if need {
		t.Error("a freshly created hash wants a rehash")
	}

	weak := "$argon2id$v=19$m=4096,t=1,p=1$c2FsdHNhbHRzYWx0$aGFzaGhhc2hoYXNoaGFzaA"
	need, err = NeedsRehash(weak)
	if err != nil {
		t.Fatal(err)
	}
	if !need {
		t.Error("a hash weaker than the current parameters does not want a rehash")
	}
}

// ─── Per-app API keys: HMAC, deliberately not Argon2id ───────────────────────

func TestClientKeyRoundTrip(t *testing.T) {
	serverKey := bytes.Repeat([]byte{0x5A}, DerivedKeyLen)

	ck, err := GenerateClientKey(serverKey)
	if err != nil {
		t.Fatalf("GenerateClientKey: %v", err)
	}
	if len(ck.Prefix) != KeyPrefixLen {
		t.Fatalf("prefix length = %d, want %d", len(ck.Prefix), KeyPrefixLen)
	}
	if !strings.HasPrefix(ck.Secret, ck.Prefix) {
		t.Fatal("prefix is not the leading characters of the secret")
	}
	if len(ck.Hash) != 32 {
		t.Fatalf("hash length = %d, want 32 (HMAC-SHA256)", len(ck.Hash))
	}

	if err := VerifyClientKey(serverKey, ck.Secret, ck.Hash); err != nil {
		t.Fatalf("VerifyClientKey: %v", err)
	}
	if err := VerifyClientKey(serverKey, ck.Secret+"x", ck.Hash); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("VerifyClientKey with a wrong key = %v, want ErrKeyMismatch", err)
	}

	// A different server key must not verify: the hash is keyed, so a stolen
	// database alone does not let an attacker mint a working key.
	other := bytes.Repeat([]byte{0xA5}, DerivedKeyLen)
	if err := VerifyClientKey(other, ck.Secret, ck.Hash); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("VerifyClientKey under a different server key = %v, want ErrKeyMismatch", err)
	}
}

// The key travels in a query string by OpenSubsonic specification, so it must
// survive one unescaped.
func TestClientKeyIsQuerySafe(t *testing.T) {
	serverKey := bytes.Repeat([]byte{0x5A}, DerivedKeyLen)
	for range 32 {
		ck, err := GenerateClientKey(serverKey)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range ck.Secret {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_'
			if !ok {
				t.Fatalf("key %q contains %q, which needs escaping in a query string", ck.Secret, r)
			}
		}
	}
}

func TestGenerateClientKeyIsRandom(t *testing.T) {
	serverKey := bytes.Repeat([]byte{0x5A}, DerivedKeyLen)
	seen := map[string]bool{}
	for range 32 {
		ck, err := GenerateClientKey(serverKey)
		if err != nil {
			t.Fatal(err)
		}
		if seen[ck.Secret] {
			t.Fatal("GenerateClientKey returned a repeated key")
		}
		seen[ck.Secret] = true
	}
}

func TestClientKeyPrefix(t *testing.T) {
	if _, ok := ClientKeyPrefix("short"); ok {
		t.Error("ClientKeyPrefix accepted a key shorter than the prefix length")
	}
	got, ok := ClientKeyPrefix("abcdefghijklmnop")
	if !ok || got != "abcdefgh" {
		t.Errorf("ClientKeyPrefix = (%q, %v), want (\"abcdefgh\", true)", got, ok)
	}
}

func TestGenerateClientKeyRejectsShortServerKey(t *testing.T) {
	if _, err := GenerateClientKey([]byte("short")); err == nil {
		t.Error("GenerateClientKey accepted a short server key")
	}
}
