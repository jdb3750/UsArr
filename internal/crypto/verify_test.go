package crypto

import (
	"bytes"
	"errors"
	"testing"
)

// TestVerifyProvesAKeyOpensARowWithoutTheAAD is the reason Verify exists.
//
// Rotation's last step before it replaces the master key file is "prove the new
// material opens every row". Open cannot answer that question, because the AAD
// binds base_url and a row whose URL was edited without the credential being
// re-entered (security.md §1.6) fails Open for a reason that has nothing to do
// with the key. Verify separates the two.
func TestVerifyProvesAKeyOpensARowWithoutTheAAD(t *testing.T) {
	kr := testKeyring(t)
	sealedAAD := testAAD(t)

	envelope, err := kr.Seal([]byte(apiKeyFixture), sealedAAD)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if err := kr.Verify(envelope); err != nil {
		t.Fatalf("Verify on a freshly sealed envelope: %v", err)
	}

	// The row's base_url was edited: same instance, different port. Open must
	// fail — that is §1.2's whole point — and Verify must still pass, because
	// the key is fine and the credential simply needs re-entering.
	moved, err := ServiceInstanceAAD(42, "http://radarr.lan:7879")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Open(envelope, moved); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Open under a changed origin = %v, want ErrDecrypt", err)
	}
	if err := kr.Verify(envelope); err != nil {
		t.Fatalf("Verify must not fail because base_url changed; a pre-existing §1.6 state "+
			"would then block key rotation forever: %v", err)
	}
}

// TestVerifyRejectsAKeyThatCannotUnwrapTheDEK: the failure Verify is actually
// looking for.
func TestVerifyRejectsAKeyThatCannotUnwrapTheDEK(t *testing.T) {
	kr := testKeyring(t)
	envelope, err := kr.Seal([]byte(apiKeyFixture), testAAD(t))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Corrupt the wrapped DEK. RFC 3394's integrity check is what catches it,
	// and it is exactly the layer a re-wrap touches.
	tampered := append([]byte(nil), envelope...)
	tampered[wrappedDEKOffset] ^= 0xff
	if err := kr.Verify(tampered); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Verify on a corrupted wrapped DEK = %v, want ErrDecrypt", err)
	}

	// An id the ring does not hold is a different failure and says so: during a
	// rotation both ids are registered, so this means the keyring is wrong.
	unknown := append([]byte(nil), envelope...)
	unknown[0], unknown[1], unknown[2], unknown[3] = 0xde, 0xad, 0xbe, 0xef
	if err := kr.Verify(unknown); !errors.Is(err, ErrUnknownKEK) {
		t.Fatalf("Verify on an unheld key id = %v, want ErrUnknownKEK", err)
	}

	if err := kr.Verify([]byte{1, 2, 3}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("Verify on a short envelope = %v, want ErrMalformed", err)
	}
}

// TestVerifyStopsAtTheDEKWrap states the limit out loud so nothing mistakes
// Verify for a full integrity check. It answers "does this key work", not "is
// this row intact"; a corrupted PAYLOAD is Open's business and rotation never
// touches the payload.
func TestVerifyStopsAtTheDEKWrap(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)
	envelope, err := kr.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	tampered := append([]byte(nil), envelope...)
	tampered[len(tampered)-1] ^= 0xff
	if err := kr.Verify(tampered); err != nil {
		t.Fatalf("Verify inspects the DEK wrap only, so a payload edit is not its failure: %v", err)
	}
	if _, err := kr.Open(tampered, aad); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Open on a tampered payload = %v, want ErrDecrypt", err)
	}
}

// TestRotationUnderContentDerivedIDs walks the whole crypto half of a rotation
// with the ids the binary actually uses — KeyID(kek), not hand-picked 1 and 2.
//
// It is the shape `usarr key rotate` performs: register both, re-wrap, verify
// under the new id, promote, drop the old.
func TestRotationUnderContentDerivedIDs(t *testing.T) {
	oldKEK := deriveTestKEK(t, 0xAB, 0xCD)
	newKEK := deriveTestKEK(t, 0x5A, 0xCD) // same salt, new master key
	oldID, newID := KeyID(oldKEK), KeyID(newKEK)
	if oldID == newID {
		t.Fatal("the fixture keys collided on one id")
	}

	kr, err := NewKeyring(oldID, oldKEK)
	if err != nil {
		t.Fatal(err)
	}
	// Startup also registers the same key at the legacy id, so rows written by
	// an older binary keep opening.
	if err := kr.Add(LegacyKEKID, oldKEK); err != nil {
		t.Fatal(err)
	}

	aad := testAAD(t)
	current, err := kr.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := KEKID(current); id != oldID {
		t.Fatalf("a new envelope was sealed at id %d, want the content-derived %d", id, oldID)
	}
	// And a row an older binary wrote, still at the legacy id.
	legacy, err := kr.Rewrap(current, LegacyKEKID)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare: both keys live, primary on the new one.
	if err := kr.Add(newID, newKEK); err != nil {
		t.Fatal(err)
	}
	if err := kr.SetPrimary(newID); err != nil {
		t.Fatal(err)
	}
	for _, e := range [][]byte{current, legacy} {
		if err := kr.Verify(e); err != nil {
			t.Fatalf("mid-rotation, every row must still be openable: %v", err)
		}
	}

	// Re-wrap.
	rotated := make([][]byte, 0, 2)
	for _, e := range [][]byte{current, legacy} {
		r, err := kr.Rewrap(e, newID)
		if err != nil {
			t.Fatalf("Rewrap: %v", err)
		}
		if !bytes.Equal(e[payloadOffset:], r[payloadOffset:]) {
			t.Fatal("Rewrap re-encrypted the payload; rotation must only re-wrap the DEK")
		}
		rotated = append(rotated, r)
	}

	// Verify before promoting, which is the order that makes promotion safe.
	for _, r := range rotated {
		if id, _ := KEKID(r); id != newID {
			t.Fatalf("a re-wrapped envelope names id %d, want %d", id, newID)
		}
		if err := kr.Verify(r); err != nil {
			t.Fatalf("Verify before promote: %v", err)
		}
	}

	// Promote: drop everything that is not the new id.
	for _, id := range []uint32{oldID, LegacyKEKID} {
		if err := kr.Drop(id); err != nil {
			t.Fatalf("Drop %d: %v", id, err)
		}
	}
	for _, r := range rotated {
		got, err := kr.Open(r, aad)
		if err != nil {
			t.Fatalf("Open after promote: %v", err)
		}
		if string(got) != apiKeyFixture {
			t.Fatal("the credential changed across the rotation")
		}
	}
	// And an un-rotated row is now unopenable, which is why the verify pass has
	// to reach every row before anything is dropped.
	if err := kr.Verify(current); !errors.Is(err, ErrUnknownKEK) {
		t.Fatalf("Verify on an un-rotated envelope after promote = %v, want ErrUnknownKEK", err)
	}
}
