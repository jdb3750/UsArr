package crypto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// apiKeyFixture stands in for an *Arr API key, which is a 32-hex-character
// string. It is deliberately mostly zeroes so it can never be mistaken for a
// real credential.
const apiKeyFixture = "0000000000000000000000000000dead"

func testKeyring(t *testing.T) *Keyring {
	t.Helper()
	secret := bytes.Repeat([]byte{0xAB}, 32)
	salt := bytes.Repeat([]byte{0xCD}, 32)
	kek, err := DeriveKEK(secret, salt)
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	kr, err := NewKeyring(1, kek)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	return kr
}

func testAAD(t *testing.T) AAD {
	t.Helper()
	aad, err := ServiceInstanceAAD(42, "http://radarr.lan:7878")
	if err != nil {
		t.Fatalf("ServiceInstanceAAD: %v", err)
	}
	return aad
}

func TestSealOpenRoundTrip(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)

	envelope, err := kr.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := kr.Open(envelope, aad)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != apiKeyFixture {
		t.Fatalf("Open = %q, want %q", got, apiKeyFixture)
	}

	// The plaintext must not appear anywhere in the stored bytes.
	if bytes.Contains(envelope, []byte(apiKeyFixture)) {
		t.Fatal("the envelope contains the plaintext credential")
	}
}

// The stored layout is a contract: a byte offset that shifts silently makes
// every existing row unreadable. docs/reference/security.md §1.1.
func TestEnvelopeLayoutIsByteExact(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)
	plaintext := []byte(apiKeyFixture)

	envelope, err := kr.Seal(plaintext, aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// kek_id (uint32 BE) || nonce (12 B) || wrapped_dek (40 B) || ciphertext || tag (16 B)
	if got, want := len(envelope), 4+12+40+len(plaintext)+16; got != want {
		t.Fatalf("envelope length = %d, want %d", got, want)
	}
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"kek_id length", kekIDLen, 4},
		{"nonce length", nonceLen, 12},
		{"wrapped_dek length", wrappedDEKLen, 40},
		{"tag length", tagLen, 16},
		{"nonce offset", nonceOffset, 4},
		{"wrapped_dek offset", wrappedDEKOffset, 16},
		{"payload offset", payloadOffset, 56},
		{"header length", HeaderLen, 56},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}

	if got := binary.BigEndian.Uint32(envelope[:4]); got != 1 {
		t.Errorf("kek_id = %d, want 1", got)
	}
	id, err := KEKID(envelope)
	if err != nil {
		t.Fatalf("KEKID: %v", err)
	}
	if id != 1 {
		t.Errorf("KEKID = %d, want 1", id)
	}
}

// Each seal must use a fresh DEK and a fresh nonce. Reusing either across
// records would let one leaked DEK open every row.
func TestSealIsRandomised(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)

	nonces := map[string]bool{}
	deks := map[string]bool{}
	for range 16 {
		envelope, err := kr.Seal([]byte(apiKeyFixture), aad)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		nonce := string(envelope[nonceOffset:wrappedDEKOffset])
		dek := string(envelope[wrappedDEKOffset:payloadOffset])
		if nonces[nonce] {
			t.Fatal("nonce reused across seals")
		}
		if deks[dek] {
			t.Fatal("wrapped DEK reused across seals")
		}
		nonces[nonce], deks[dek] = true, true
	}
}

// Moving a ciphertext to a different row, column, table or host must fail.
// This is the property that stops someone with database write access copying
// the Radarr row's ciphertext into a row whose base_url they control.
func TestOpenRejectsWrongAAD(t *testing.T) {
	kr := testKeyring(t)
	base := testAAD(t)

	envelope, err := kr.Seal([]byte(apiKeyFixture), base)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	mutations := []struct {
		name string
		aad  AAD
	}{
		{"different row", AAD{base.Table, base.Column, "43", base.HostPort}},
		{"different table", AAD{"other_table", base.Column, base.PrimaryKey, base.HostPort}},
		{"different column", AAD{base.Table, "other_column", base.PrimaryKey, base.HostPort}},
		{"different host", AAD{base.Table, base.Column, base.PrimaryKey, "attacker.example:7878"}},
		{"different port", AAD{base.Table, base.Column, base.PrimaryKey, "radarr.lan:8989"}},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := kr.Open(envelope, tc.aad); !errors.Is(err, ErrDecrypt) {
				t.Fatalf("Open with %s = %v, want ErrDecrypt", tc.name, err)
			}
		})
	}
}

// Changing an instance's base_url host or port makes the stored ciphertext fail
// to open rather than silently succeed — the cryptographic half of the rule in
// docs/reference/security.md §1.6.
func TestChangingBaseURLInvalidatesTheCredential(t *testing.T) {
	kr := testKeyring(t)

	original, err := ServiceInstanceAAD(7, "http://sonarr.lan:8989")
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := kr.Seal([]byte(apiKeyFixture), original)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	moved, err := ServiceInstanceAAD(7, "http://attacker.example:8989")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Open(envelope, moved); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Open after a base_url change = %v, want ErrDecrypt", err)
	}

	// An equivalent URL — same host and port, tidier spelling — still opens.
	same, err := ServiceInstanceAAD(7, "HTTP://Sonarr.LAN.:8989/some/path?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Open(envelope, same); err != nil {
		t.Fatalf("Open with an equivalently-spelled URL failed: %v", err)
	}
}

func TestOpenRejectsWrongKEK(t *testing.T) {
	aad := testAAD(t)

	good := testKeyring(t)
	envelope, err := good.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	otherKEK, err := DeriveKEK(bytes.Repeat([]byte{0x01}, 32), bytes.Repeat([]byte{0x02}, 32))
	if err != nil {
		t.Fatal(err)
	}
	bad, err := NewKeyring(1, otherKEK)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bad.Open(envelope, aad); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Open under the wrong KEK = %v, want ErrDecrypt", err)
	}
}

// An envelope naming a key id the ring does not hold is a distinct condition
// from a failed decrypt: it means the keyring is wrong, not the data.
func TestOpenRejectsUnknownKEKID(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)
	envelope, err := kr.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	binary.BigEndian.PutUint32(envelope[:4], 99)
	if _, err := kr.Open(envelope, aad); !errors.Is(err, ErrUnknownKEK) {
		t.Fatalf("Open with an unknown kek_id = %v, want ErrUnknownKEK", err)
	}
}

// Every byte of the envelope is authenticated one way or another: the wrapped
// DEK by AES-KW's integrity check, the ciphertext and tag by GCM. The nonce is
// not authenticated directly, but changing it changes the decryption and so
// fails the tag.
func TestOpenRejectsTamperedBytes(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)
	envelope, err := kr.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	for i := kekIDLen; i < len(envelope); i++ {
		tampered := append([]byte(nil), envelope...)
		tampered[i] ^= 0x01
		if _, err := kr.Open(tampered, aad); err == nil {
			t.Fatalf("flipping a bit in byte %d was accepted", i)
		}
	}
}

func TestOpenRejectsMalformed(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)
	for _, n := range []int{0, 1, HeaderLen, MinEnvelopeLen - 1} {
		if _, err := kr.Open(make([]byte, n), aad); !errors.Is(err, ErrMalformed) {
			t.Errorf("Open on a %d-byte envelope = %v, want ErrMalformed", n, err)
		}
	}
}

// Rotation re-wraps the DEK and leaves the ciphertext alone. That is what makes
// it cheap, and what makes it resumable at any row boundary.
func TestRewrap(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)

	envelope, err := kr.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	newKEK, err := DeriveKEK(bytes.Repeat([]byte{0x77}, 32), bytes.Repeat([]byte{0x88}, 32))
	if err != nil {
		t.Fatal(err)
	}
	// Both keys active before any row is touched: from here either can open
	// any row, which is what removes the crash window.
	if err := kr.Add(2, newKEK); err != nil {
		t.Fatalf("Add: %v", err)
	}

	rotated, err := kr.Rewrap(envelope, 2)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}

	if id, _ := KEKID(rotated); id != 2 {
		t.Errorf("kek_id after Rewrap = %d, want 2", id)
	}
	// The nonce and ciphertext are untouched: only the DEK wrap changed.
	if !bytes.Equal(envelope[nonceOffset:wrappedDEKOffset], rotated[nonceOffset:wrappedDEKOffset]) {
		t.Error("Rewrap changed the nonce")
	}
	if !bytes.Equal(envelope[payloadOffset:], rotated[payloadOffset:]) {
		t.Error("Rewrap changed the ciphertext")
	}
	if bytes.Equal(envelope[wrappedDEKOffset:payloadOffset], rotated[wrappedDEKOffset:payloadOffset]) {
		t.Error("Rewrap did not change the wrapped DEK")
	}

	got, err := kr.Open(rotated, aad)
	if err != nil {
		t.Fatalf("Open after Rewrap: %v", err)
	}
	if string(got) != apiKeyFixture {
		t.Fatalf("Open after Rewrap = %q, want %q", got, apiKeyFixture)
	}

	// Promote, then drop the old key: the row still opens under the new one
	// alone, which is the end state of a completed rotation.
	if err := kr.SetPrimary(2); err != nil {
		t.Fatalf("SetPrimary: %v", err)
	}
	if err := kr.Drop(1); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if _, err := kr.Open(rotated, aad); err != nil {
		t.Fatalf("Open after dropping the old key: %v", err)
	}
	// And the un-rotated envelope is now unreadable, which is why rotation
	// must reach zero rows at the old id before dropping it.
	if _, err := kr.Open(envelope, aad); !errors.Is(err, ErrUnknownKEK) {
		t.Fatalf("Open of an un-rotated envelope = %v, want ErrUnknownKEK", err)
	}
}

func TestKeyringGuards(t *testing.T) {
	kr := testKeyring(t)

	if err := kr.Add(2, []byte("too short")); err == nil {
		t.Error("Add accepted a short KEK")
	}
	if err := kr.Drop(kr.PrimaryID()); err == nil {
		t.Error("Drop removed the primary key")
	}
	if err := kr.SetPrimary(99); !errors.Is(err, ErrUnknownKEK) {
		t.Errorf("SetPrimary(99) = %v, want ErrUnknownKEK", err)
	}
	if _, err := NewKeyring(1, []byte("short")); err == nil {
		t.Error("NewKeyring accepted a short KEK")
	}
}

// A decryption failure must say where it happened so it can be audit-logged,
// and must not leak the credential.
func TestDecryptErrorIsLoudAndClean(t *testing.T) {
	kr := testKeyring(t)
	aad := testAAD(t)
	envelope, err := kr.Seal([]byte(apiKeyFixture), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	wrong := AAD{aad.Table, aad.Column, "999", aad.HostPort}

	_, err = kr.Open(envelope, wrong)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"service_instance", "api_key_enc", "999"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %q", msg, want)
		}
	}
	if strings.Contains(msg, apiKeyFixture) {
		t.Error("the error message contains the credential")
	}
}
