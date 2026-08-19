// Package crypto implements UsArr's credential encryption at rest.
//
// UsArr is a credential vault: one *Arr API key is full admin on that instance
// — delete the library, change root folders, restart the process — and it is a
// single opaque string with no scoping whatsoever. Everything here follows from
// that.
//
// The scheme is envelope encryption. Each secret gets a fresh random
// data-encryption key (DEK) and a fresh nonce; the payload is sealed with
// AES-256-GCM under the DEK; the DEK is wrapped under a key-encryption key
// (KEK) derived from the master key. Only the wrapped DEK, the key id and the
// ciphertext reach the database. The indirection is what makes rotation cheap:
// rotating re-wraps DEKs, it does not re-encrypt secrets.
//
// See docs/reference/security.md §1.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
)

// Envelope layout, docs/reference/security.md §1.1:
//
//	kek_id (uint32 BE) || nonce (12 B) || wrapped_dek (40 B) || ciphertext || tag (16 B)
//
// The trailing tag is GCM's, appended to the ciphertext by Seal.
const (
	kekIDLen      = 4
	nonceLen      = 12
	dekLen        = 32
	wrappedDEKLen = dekLen + 8 // RFC 3394 wraps n blocks into n+1
	tagLen        = 16

	nonceOffset      = kekIDLen
	wrappedDEKOffset = nonceOffset + nonceLen
	payloadOffset    = wrappedDEKOffset + wrappedDEKLen

	// HeaderLen is everything before the ciphertext.
	HeaderLen = payloadOffset

	// MinEnvelopeLen is the shortest well-formed envelope: header plus a
	// zero-length plaintext's GCM tag.
	MinEnvelopeLen = HeaderLen + tagLen
)

// ErrMalformed means the stored bytes are not a well-formed envelope. This is
// distinct from ErrDecrypt: it is a structural failure, detectable without any
// key at all.
var ErrMalformed = errors.New("crypto: malformed envelope")

// ErrDecrypt means authenticated decryption failed under a KEK the caller
// supplied.
//
// docs/reference/security.md §1.2: a decryption failure with a valid KEK means
// tampering. It is a loud, audit-logged failure, never a silent skip. Callers
// must not swallow this error and must not fall back to treating the row as
// having no credential — see Keyring.Open's contract.
var ErrDecrypt = errors.New("crypto: authenticated decryption failed")

// ErrUnknownKEK means the envelope names a key id the keyring does not hold.
// During a rotation both the old and new ids are held, so this means the
// keyring is wrong, not that the rotation is mid-flight.
var ErrUnknownKEK = errors.New("crypto: envelope names a key id not in the keyring")

// Keyring holds the KEKs that can open stored envelopes.
//
// It holds more than one on purpose. Rotation registers both the old and the
// new key as active before any row is touched, so a crash, OOM or container
// restart mid-rotation leaves a database that is still fully readable. Without
// a key id in the envelope, an interrupted rotation leaves records under two
// keys and nothing able to tell them apart, because AEAD makes a wrong-key
// decrypt indistinguishable from corruption.
type Keyring struct {
	keys      map[uint32][]byte
	primaryID uint32
}

// NewKeyring builds a keyring whose primary key is primaryID. Additional keys
// are decrypt-only: they open existing records but never seal new ones.
func NewKeyring(primaryID uint32, primaryKEK []byte) (*Keyring, error) {
	if len(primaryKEK) != DerivedKeyLen {
		return nil, fmt.Errorf("crypto: KEK %d is %d bytes, want %d", primaryID, len(primaryKEK), DerivedKeyLen)
	}
	k := &Keyring{keys: map[uint32][]byte{primaryID: primaryKEK}, primaryID: primaryID}
	return k, nil
}

// Add registers an additional KEK for decryption. It is used by rotation, in
// both directions: the new key is added before re-wrapping starts, and the old
// key stays until zero rows remain under it.
func (k *Keyring) Add(id uint32, kek []byte) error {
	if len(kek) != DerivedKeyLen {
		return fmt.Errorf("crypto: KEK %d is %d bytes, want %d", id, len(kek), DerivedKeyLen)
	}
	if existing, ok := k.keys[id]; ok && string(existing) != string(kek) {
		return fmt.Errorf("crypto: KEK %d is already registered with different bytes", id)
	}
	k.keys[id] = kek
	return nil
}

// PrimaryID is the key id new records are sealed under.
func (k *Keyring) PrimaryID() uint32 { return k.primaryID }

// SetPrimary promotes an already-registered key to primary. Rotation calls this
// only after the new key file is durable on disk.
func (k *Keyring) SetPrimary(id uint32) error {
	if _, ok := k.keys[id]; !ok {
		return fmt.Errorf("%w: %d", ErrUnknownKEK, id)
	}
	k.primaryID = id
	return nil
}

// Drop removes a key from the ring. Rotation calls this only when
// `SELECT count(*) WHERE kek_id = :old` is zero.
func (k *Keyring) Drop(id uint32) error {
	if id == k.primaryID {
		return fmt.Errorf("crypto: refusing to drop KEK %d: it is the primary key", id)
	}
	delete(k.keys, id)
	return nil
}

// IDs reports every registered key id.
func (k *Keyring) IDs() []uint32 {
	out := make([]uint32, 0, len(k.keys))
	for id := range k.keys {
		out = append(out, id)
	}
	return out
}

func (k *Keyring) kek(id uint32) ([]byte, error) {
	kek, ok := k.keys[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKEK, id)
	}
	return kek, nil
}

// Seal encrypts plaintext under a fresh DEK and nonce, wrapping the DEK with
// the primary KEK, and returns the stored envelope.
//
// aad binds the result to its table, column, row and instance host:port.
// Opening it anywhere else fails.
func (k *Keyring) Seal(plaintext []byte, aad AAD) ([]byte, error) {
	kek, err := k.kek(k.primaryID)
	if err != nil {
		return nil, err
	}

	dek := make([]byte, dekLen)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("crypto: generate DEK: %w", err)
	}
	defer zero(dek)

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	wrapped, err := wrapKey(kek, dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: wrap DEK: %w", err)
	}

	gcm, err := newGCM(dek)
	if err != nil {
		return nil, err
	}

	out := make([]byte, payloadOffset, payloadOffset+len(plaintext)+tagLen)
	binary.BigEndian.PutUint32(out[:kekIDLen], k.primaryID)
	copy(out[nonceOffset:], nonce)
	copy(out[wrappedDEKOffset:], wrapped)

	return gcm.Seal(out, nonce, plaintext, aad.Bytes()), nil
}

// Open decrypts a stored envelope.
//
// A returned ErrDecrypt under a KEK the keyring holds means the record was
// tampered with — the ciphertext was moved to a different row, the instance's
// base_url was changed underneath it, or the bytes were edited. It is
// audit-worthy and must never be treated as "this row has no credential".
func (k *Keyring) Open(envelope []byte, aad AAD) ([]byte, error) {
	kekID, nonce, wrapped, payload, err := parseEnvelope(envelope)
	if err != nil {
		return nil, err
	}

	kek, err := k.kek(kekID)
	if err != nil {
		return nil, err
	}

	dek, err := unwrapKey(kek, wrapped)
	if err != nil {
		// The wrapped DEK failed its RFC 3394 integrity check under a key we
		// hold. Same class as a GCM failure, so it gets the same error.
		return nil, fmt.Errorf("%w: kek_id=%d: %w", ErrDecrypt, kekID, err)
	}
	defer zero(dek)

	gcm, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, payload, aad.Bytes())
	if err != nil {
		return nil, fmt.Errorf("%w: kek_id=%d: %s", ErrDecrypt, kekID, aad.location())
	}
	return plaintext, nil
}

// Verify checks that an envelope's DEK unwraps under the key id the envelope
// itself records, and returns no plaintext.
//
// It exists for the last step before a rotation promotes the new key file: the
// only honest question at that point is "can the material I am about to make
// authoritative open this row", and Open cannot answer it. Open needs the AAD,
// which binds the row's base_url, so it fails on a row whose URL was edited
// without the credential being re-entered (security.md §1.6) — a legitimate,
// pre-existing state that has nothing to do with the rotation. Verify stops at
// the RFC 3394 integrity check on the wrapped DEK, which is the layer rotation
// actually touched, so it separates "the new key is wrong" from "this row was
// already unopenable".
//
// It returns no plaintext ON PURPOSE. Nothing about proving a key works needs a
// credential in memory, let alone in a caller that might log it.
func (k *Keyring) Verify(envelope []byte) error {
	kekID, _, wrapped, _, err := parseEnvelope(envelope)
	if err != nil {
		return err
	}
	kek, err := k.kek(kekID)
	if err != nil {
		return err
	}
	dek, err := unwrapKey(kek, wrapped)
	if err != nil {
		return fmt.Errorf("%w: kek_id=%d: %w", ErrDecrypt, kekID, err)
	}
	zero(dek)
	return nil
}

// Rewrap re-wraps an envelope's DEK under a different registered KEK without
// touching the ciphertext.
//
// This is what makes rotation cheap and what makes it resumable: the payload,
// nonce and AAD are unchanged, so a batch of Rewrap calls is a pure metadata
// update and can be interrupted at any row boundary. The AAD is not needed
// because the DEK wrap is independent of it.
func (k *Keyring) Rewrap(envelope []byte, newKEKID uint32) ([]byte, error) {
	oldID, _, wrapped, _, err := parseEnvelope(envelope)
	if err != nil {
		return nil, err
	}
	oldKEK, err := k.kek(oldID)
	if err != nil {
		return nil, err
	}
	newKEK, err := k.kek(newKEKID)
	if err != nil {
		return nil, err
	}
	if oldID == newKEKID {
		return append([]byte(nil), envelope...), nil
	}

	dek, err := unwrapKey(oldKEK, wrapped)
	if err != nil {
		return nil, fmt.Errorf("%w: kek_id=%d: %w", ErrDecrypt, oldID, err)
	}
	defer zero(dek)

	rewrapped, err := wrapKey(newKEK, dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: re-wrap DEK: %w", err)
	}

	out := append([]byte(nil), envelope...)
	binary.BigEndian.PutUint32(out[:kekIDLen], newKEKID)
	copy(out[wrappedDEKOffset:payloadOffset], rewrapped)
	return out, nil
}

// KEKID reads the key id from a stored envelope without any key.
//
// The id is also stored as a plain column (service_instance.kek_id) so a
// rotation can `SELECT … WHERE kek_id = :old` and resume; this function is how
// a consistency check compares the two.
func KEKID(envelope []byte) (uint32, error) {
	id, _, _, _, err := parseEnvelope(envelope)
	return id, err
}

func parseEnvelope(b []byte) (kekID uint32, nonce, wrapped, payload []byte, err error) {
	if len(b) < MinEnvelopeLen {
		return 0, nil, nil, nil, fmt.Errorf("%w: %d bytes, minimum %d", ErrMalformed, len(b), MinEnvelopeLen)
	}
	kekID = binary.BigEndian.Uint32(b[:kekIDLen])
	nonce = b[nonceOffset:wrappedDEKOffset]
	wrapped = b[wrappedDEKOffset:payloadOffset]
	payload = b[payloadOffset:]
	return kekID, nonce, wrapped, payload, nil
}

func newGCM(dek []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: DEK: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: GCM: %w", err)
	}
	return gcm, nil
}

// location renders the AAD's addressing fields for an error message. The origin
// hash is omitted: it is long and the three plain fields already identify the
// row. The origin itself is printed, scheme included — when a decrypt fails
// because base_url was edited, "which origin was this bound to" is the whole
// diagnosis, and the scheme is exactly the part that used to be invisible.
func (a AAD) location() string {
	return a.Table + "." + a.Column + " id=" + a.PrimaryKey + " origin=" + a.Origin
}
