package crypto

import (
	"crypto/aes"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
)

// AES Key Wrap, RFC 3394.
//
// Why this and not AES-GCM for the DEK: the stored envelope format in
// docs/reference/security.md §1.1 fixes wrapped_dek at exactly 40 bytes. RFC
// 3394 wraps an n-block key into n+1 blocks, so a 32-byte DEK wraps to exactly
// 40 bytes. No AEAD construction produces 40 bytes for a 32-byte plaintext:
// GCM would need 48 (32 + a 16-byte tag) before counting a nonce. The 40-byte
// field in the spec is therefore AES-KW, and this is the derivation of that
// reading — the document states the length, not the algorithm.
//
// AES-KW is deterministic (no nonce to manage, so a wrapped DEK is stable
// across re-wraps of the same key) and carries its own integrity check: the
// fixed IV must reappear on unwrap, which is what makes a wrong-KEK unwrap a
// clean error rather than 32 bytes of garbage that would then be used as a key.

// keyWrapIV is the default initial value from RFC 3394 §2.2.3.1.
var keyWrapIV = [8]byte{0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6}

// ErrUnwrap means the wrapped key failed its integrity check. Under a KEK the
// caller believes is correct, this means the record was tampered with.
var ErrUnwrap = errors.New("crypto: key unwrap failed integrity check")

// wrapKey wraps plaintext (a multiple of 8 bytes, at least 16) under kek,
// returning len(plaintext)+8 bytes.
func wrapKey(kek, plaintext []byte) ([]byte, error) {
	if len(plaintext) < 16 || len(plaintext)%8 != 0 {
		return nil, fmt.Errorf("crypto: key to wrap is %d bytes; must be a multiple of 8 and at least 16", len(plaintext))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("crypto: kek: %w", err)
	}

	n := len(plaintext) / 8
	a := keyWrapIV
	r := make([]byte, len(plaintext))
	copy(r, plaintext)

	// RFC 3394 defines the round constant as t = (n*j)+i. Because the loops run
	// j ascending and i ascending within it, that expression is simply the
	// 1-based step number, so a running counter computes it exactly and avoids
	// converting a signed product on every round.
	var t uint64
	var buf [16]byte
	for range 6 {
		for i := 1; i <= n; i++ {
			t++
			copy(buf[:8], a[:])
			copy(buf[8:], r[(i-1)*8:i*8])
			block.Encrypt(buf[:], buf[:])

			copy(a[:], buf[:8])
			binary.BigEndian.PutUint64(a[:], binary.BigEndian.Uint64(a[:])^t)
			copy(r[(i-1)*8:i*8], buf[8:])
		}
	}

	out := make([]byte, 8+len(r))
	copy(out[:8], a[:])
	copy(out[8:], r)
	return out, nil
}

// unwrapKey reverses wrapKey. It returns ErrUnwrap if the integrity check
// fails, which is the case for a wrong KEK and for a tampered wrapped key
// alike — the two are indistinguishable by design.
func unwrapKey(kek, wrapped []byte) ([]byte, error) {
	if len(wrapped) < 24 || len(wrapped)%8 != 0 {
		return nil, fmt.Errorf("%w: wrapped key is %d bytes", ErrUnwrap, len(wrapped))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("crypto: kek: %w", err)
	}

	n := len(wrapped)/8 - 1
	var a [8]byte
	copy(a[:], wrapped[:8])
	r := make([]byte, len(wrapped)-8)
	copy(r, wrapped[8:])

	// Unwrap walks the same t sequence backwards, from 6n down to 1. n is
	// (len(wrapped)/8 - 1) and len(wrapped) >= 24 was checked above, so n is at
	// least 2 and the product cannot overflow.
	t := 6 * uint64(n) //#nosec G115 -- n is a positive slice-derived block count
	var buf [16]byte
	for range 6 {
		for i := n; i >= 1; i-- {
			binary.BigEndian.PutUint64(buf[:8], binary.BigEndian.Uint64(a[:])^t)
			copy(buf[8:], r[(i-1)*8:i*8])
			block.Decrypt(buf[:], buf[:])
			t--

			copy(a[:], buf[:8])
			copy(r[(i-1)*8:i*8], buf[8:])
		}
	}

	if subtle.ConstantTimeCompare(a[:], keyWrapIV[:]) != 1 {
		return nil, ErrUnwrap
	}
	return r, nil
}

// zero overwrites b. Go's GC may already have copied the bytes elsewhere, so
// this is best-effort hygiene on the DEK, not a guarantee.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
