package crypto

import (
	"crypto/sha256"
	"encoding/binary"
)

// keyIDInfo domain-separates the key-id digest from every other use of the KEK.
//
// It is NOT one of derive.go's HKDF info labels and deliberately does not live
// beside them. Those five are bound into stored ciphertext, issued API keys and
// published row ids, so that list is frozen; this constant feeds a plain
// SHA-256 that produces a public identifier, never key material. Keeping it
// here keeps the two categories from being confused for each other, and keeps
// the frozen list frozen.
//
// #nosec G101 -- a domain-separation constant, not a credential. It is public
// by design, exactly like the HKDF labels in derive.go.
const keyIDInfo = "usarr/kek-id/v1"

// LegacyKEKID is the fixed id every envelope sealed before key ids became
// content-derived carries.
//
// Startup registers the key derived from secret.key under BOTH KeyID(kek) and
// this id, so rows written by an older binary keep opening with no migration
// and no rewrite of any row. It is never a primary id: new envelopes are sealed
// under KeyID(kek).
const LegacyKEKID uint32 = 1

// KeyID derives a key's id from the key itself: the first four bytes, big
// endian, of sha256("usarr/kek-id/v1" || kek), forced nonzero.
//
// WHY CONTENT-DERIVED. The id's whole job is to let a row say which key wrapped
// it, and a key file that names its own id is the only version of that which
// cannot drift. A counter or a settings-table row would be a SECOND piece of
// state that has to stay consistent with the key material across a crash, and
// the crash window rotation exists to close is exactly where those two would
// disagree — a promoted key file with an un-promoted counter makes every row
// name an id nothing holds. See ADR-0049.
//
// The id is not a secret and is not a strength claim. It is published in
// service_instance.kek_id and in every envelope header, which means an offline
// attacker holding the database learns a 32-bit hash of the KEK. That grants
// them nothing: RFC 3394 key-wrap already gives them a per-row integrity check
// they can test a candidate key against, and that check is exact where this one
// is 32 bits wide. See ADR-0049.
//
// Forced nonzero because 0 is the placeholder CreateServiceInstanceSealed
// writes into kek_id between the insert and the seal, so a real key must never
// claim it. The substitute is LegacyKEKID rather than an arbitrary value: on the
// 2^-32 occasion it fires, the key registers at the id it would already have
// been registered at anyway, and nothing special-cases it.
func KeyID(kek []byte) uint32 {
	h := sha256.New()
	h.Write([]byte(keyIDInfo))
	h.Write(kek)
	sum := h.Sum(nil)
	id := binary.BigEndian.Uint32(sum[:4])
	if id == 0 {
		return LegacyKEKID
	}
	return id
}
