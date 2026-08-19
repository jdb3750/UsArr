package crypto

import (
	"bytes"
	"testing"
)

// TestKeyIDIsDeterministicAndContentDerived pins the property the whole scheme
// rests on: the same key file always produces the same id, on every process and
// after every restart. If it did not, an envelope's kek_id would name a key
// nothing could find.
func TestKeyIDIsDeterministicAndContentDerived(t *testing.T) {
	kek := deriveTestKEK(t, 0x11, 0x22)

	first := KeyID(kek)
	for i := 0; i < 8; i++ {
		if got := KeyID(kek); got != first {
			t.Fatalf("KeyID is not deterministic: %d then %d", first, got)
		}
	}
	// And it does not mutate its input; the caller goes on to seal with it.
	if !bytes.Equal(kek, deriveTestKEK(t, 0x11, 0x22)) {
		t.Fatal("KeyID modified the key it was given")
	}
}

// TestKeyIDSeparatesDifferentKeys is the reason rotation works at all: the old
// and new keys must land on different ids, or a row cannot say which one wrapped
// it.
func TestKeyIDSeparatesDifferentKeys(t *testing.T) {
	seen := map[uint32][]byte{}
	// A spread of key material, including two that differ in one byte and two
	// that differ only in the salt they were derived under.
	for _, pair := range [][2]byte{
		{0x11, 0x22}, {0x12, 0x22}, {0x11, 0x23}, {0x00, 0x01}, {0xff, 0xfe}, {0x7f, 0x80},
	} {
		kek := deriveTestKEK(t, pair[0], pair[1])
		id := KeyID(kek)
		if id == 0 {
			t.Fatalf("KeyID returned 0 for %v; 0 is the kek_id placeholder and must never be a real id", pair)
		}
		if prev, ok := seen[id]; ok && !bytes.Equal(prev, kek) {
			t.Fatalf("two distinct keys collided on id %d", id)
		}
		seen[id] = kek
	}
	if len(seen) != 6 {
		t.Fatalf("six distinct keys produced %d distinct ids", len(seen))
	}
}

// TestKeyIDDependsOnEveryByte: a truncated or padded read of the key file must
// not produce the id of the correct key, or a corrupt key file would claim to be
// the good one.
func TestKeyIDDependsOnEveryByte(t *testing.T) {
	kek := deriveTestKEK(t, 0x11, 0x22)
	base := KeyID(kek)

	for i := range kek {
		flipped := append([]byte(nil), kek...)
		flipped[i] ^= 0x01
		if KeyID(flipped) == base {
			t.Fatalf("flipping byte %d did not change the key id", i)
		}
	}
	if KeyID(kek[:len(kek)-1]) == base {
		t.Fatal("a truncated key produced the full key's id")
	}
}

// TestKeyIDIsDomainSeparated: the id is a public identifier and must not be a
// prefix of anything derived from the same key for another purpose. Sharing a
// digest across purposes is how a public value starts leaking a secret one.
func TestKeyIDIsDomainSeparated(t *testing.T) {
	secret := bytes.Repeat([]byte{0x11}, 32)
	salt := bytes.Repeat([]byte{0x22}, 32)
	kek, err := DeriveKEK(secret, salt)
	if err != nil {
		t.Fatal(err)
	}
	id := KeyID(kek)

	// Nothing else derived from the same inputs may collide with the id's
	// digest. These are the five frozen HKDF labels' outputs.
	for _, derive := range []func(a, b []byte) ([]byte, error){
		DeriveKEK, DeriveStreamTokenKey, DeriveClientCredentialKey, DeriveGrabRowIDKey, DeriveAuditRowIDKey,
	} {
		other, err := derive(secret, salt)
		if err != nil {
			t.Fatal(err)
		}
		if KeyID(other) == id && !bytes.Equal(other, kek) {
			t.Fatal("a key derived under a different label collided with the KEK's id")
		}
	}
}

// TestLegacyKEKIDIsOne pins the constant. Every envelope sealed before ids were
// content-derived carries this value, and changing it makes those rows name an
// id the keyring does not hold.
func TestLegacyKEKIDIsOne(t *testing.T) {
	if LegacyKEKID != 1 {
		t.Fatalf("LegacyKEKID = %d, want 1: rows sealed by an older binary carry 1", LegacyKEKID)
	}
}

func deriveTestKEK(t *testing.T, secretByte, saltByte byte) []byte {
	t.Helper()
	kek, err := DeriveKEK(bytes.Repeat([]byte{secretByte}, 32), bytes.Repeat([]byte{saltByte}, 32))
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	return kek
}
