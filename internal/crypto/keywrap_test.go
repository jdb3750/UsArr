package crypto

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

// RFC 3394 §4.6 — wrap 256 bits of key data with a 256-bit KEK.
//
// This is the only external check on the key-wrap implementation, which was
// written from the RFC's pseudocode. A round-trip test would pass against a
// consistently wrong implementation; this vector would not.
func TestKeyWrapRFC3394Vector(t *testing.T) {
	kek := mustHex(t, "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	key := mustHex(t, "00112233445566778899AABBCCDDEEFF000102030405060708090A0B0C0D0E0F")
	want := mustHex(t, "28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21")

	got, err := wrapKey(kek, key)
	if err != nil {
		t.Fatalf("wrapKey: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("wrapKey =\n  %X\nwant\n  %X", got, want)
	}
	// The wrapped 256-bit key is exactly 40 bytes, which is what the stored
	// envelope format in docs/reference/security.md §1.1 reserves for it.
	if len(got) != wrappedDEKLen {
		t.Fatalf("wrapped length = %d, want %d", len(got), wrappedDEKLen)
	}

	back, err := unwrapKey(kek, got)
	if err != nil {
		t.Fatalf("unwrapKey: %v", err)
	}
	if !bytes.Equal(back, key) {
		t.Fatalf("unwrapKey = %X, want %X", back, key)
	}
}

func TestKeyWrapRejectsWrongKEK(t *testing.T) {
	kek := bytes.Repeat([]byte{0x11}, 32)
	other := bytes.Repeat([]byte{0x22}, 32)
	key := bytes.Repeat([]byte{0x33}, 32)

	wrapped, err := wrapKey(kek, key)
	if err != nil {
		t.Fatalf("wrapKey: %v", err)
	}
	if _, err := unwrapKey(other, wrapped); !errors.Is(err, ErrUnwrap) {
		t.Fatalf("unwrapKey under the wrong KEK = %v, want ErrUnwrap", err)
	}
}

// Every single-bit change must fail the integrity check. Without this the
// unwrap would return 32 bytes of garbage that would then be used as a key.
func TestKeyWrapRejectsTampering(t *testing.T) {
	kek := bytes.Repeat([]byte{0x11}, 32)
	key := bytes.Repeat([]byte{0x33}, 32)
	wrapped, err := wrapKey(kek, key)
	if err != nil {
		t.Fatalf("wrapKey: %v", err)
	}

	for i := range wrapped {
		tampered := append([]byte(nil), wrapped...)
		tampered[i] ^= 0x01
		if _, err := unwrapKey(kek, tampered); !errors.Is(err, ErrUnwrap) {
			t.Fatalf("flipping bit 0 of byte %d was accepted: %v", i, err)
		}
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex fixture: %v", err)
	}
	return b
}
