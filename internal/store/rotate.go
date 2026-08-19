package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdb3750/UsArr/internal/crypto"
)

// rotateBatchSize bounds how many envelopes one rotation transaction rewrites.
// Same reasoning as redactBatchSize: small enough that no single transaction
// holds the writer for long, large enough that a homelab's whole table is one
// round trip.
const rotateBatchSize = 500

// CredentialEnvelope is one row's stored ciphertext, as rotation sees it.
//
// BaseURL is carried because it is what the AAD binds; rotation itself never
// needs it — Rewrap does not take an AAD — but a diagnostic that says which
// instance failed is useless without a name for the instance.
type CredentialEnvelope struct {
	ID       int64
	Name     string
	BaseURL  string
	Envelope []byte
	KEKID    uint32

	// Deleted reports that this row is tombstoned. Rotation rewrites it anyway;
	// the field exists so the operator-facing count can say how many of the
	// rows it touched were tombstones.
	Deleted bool
}

// The three functions below are the ONLY credential reads and writes in this
// package that ignore deleted_at, and the omission is the point rather than an
// oversight.
//
// WHY. A soft-deleted service_instance still holds its ciphertext — that is
// exactly what SoftDeleteServiceInstance's "the id stays burned" means, and
// CountEncryptedCredentials already counts tombstones for it, which is what
// makes startup refuse when the salt is missing on an install whose only sealed
// rows are deleted ones. Every OTHER credential UPDATE in this package carries
// `AND deleted_at IS NULL`, because every other one is a user action on a row
// the user can see. A rotation is not a user action on a row: it re-wraps key
// material, and a tombstoned row's key material is wrapped under the same KEK
// as everything else. Built on the visible-row helpers, a rotation would report
// "zero rows remain at the old id" while tombstones sat there still wrapped
// under a key whose file has just been replaced — permanently unopenable
// ciphertext, produced by the procedure whose entire purpose is not to produce
// any.
//
// Anything else that needs a credential must use the deleted_at-filtered
// helpers in serviceinstance.go. If a second caller ever appears here, it needs
// the same argument written down before it does.

// ListCredentialEnvelopesIncludingDeleted reads one keyset page of stored
// credentials, TOMBSTONES INCLUDED. See the note above.
//
// Rows whose api_key_enc is shorter than a well-formed envelope are skipped:
// CreateServiceInstanceSealed inserts a single-byte placeholder that the same
// transaction replaces, and a value that crypto could only ever reject is not a
// credential to rotate. The count below applies the identical filter, so the
// two can never disagree about what "remaining work" means.
func (s *Store) ListCredentialEnvelopesIncludingDeleted(ctx context.Context, afterID int64, limit int) ([]CredentialEnvelope, error) {
	if limit <= 0 {
		limit = rotateBatchSize
	}
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT id, name, base_url, api_key_enc, kek_id, deleted_at IS NOT NULL
		  FROM service_instance
		 WHERE id > ? AND length(api_key_enc) >= ?
		 ORDER BY id ASC
		 LIMIT ?`, afterID, crypto.MinEnvelopeLen, limit)
	if err != nil {
		return nil, fmt.Errorf("read credentials for rotation: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []CredentialEnvelope
	for rows.Next() {
		var c CredentialEnvelope
		if err := rows.Scan(&c.ID, &c.Name, &c.BaseURL, &c.Envelope, &c.KEKID, &c.Deleted); err != nil {
			return nil, fmt.Errorf("read credentials for rotation: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read credentials for rotation: %w", err)
	}
	return out, nil
}

// CredentialRewrite is one re-wrapped envelope waiting to be written.
type CredentialRewrite struct {
	ID       int64
	Envelope []byte
	KEKID    uint32
}

// RewrapCredentialsIncludingDeleted writes a batch of re-wrapped envelopes in
// ONE bounded transaction, TOMBSTONES INCLUDED. See the note above.
//
// One transaction per batch, not one per row and not one for the whole table: a
// crash between batches leaves every row readable under the two-key set, which
// is the property that makes rotation resumable (security.md §1.5 step 3).
func (s *Store) RewrapCredentialsIncludingDeleted(ctx context.Context, batch []CredentialRewrite) error {
	if len(batch) == 0 {
		return nil
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`UPDATE service_instance SET api_key_enc = ?, kek_id = ? WHERE id = ?`)
		if err != nil {
			return fmt.Errorf("rewrap credentials: prepare: %w", err)
		}
		defer func() { _ = stmt.Close() }()
		for _, r := range batch {
			res, err := stmt.ExecContext(ctx, r.Envelope, r.KEKID, r.ID)
			if err != nil {
				return fmt.Errorf("rewrap service_instance %d credential: %w", r.ID, err)
			}
			if err := expectOneRow(res, "service_instance", r.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// CountCredentialsOutsideKEKIDIncludingDeleted reports how many stored
// credentials are wrapped under any key id other than keepID, TOMBSTONES
// INCLUDED. See the note above.
//
// This is rotation's remaining-work number and its termination condition. It is
// deliberately "not at the new id" rather than "at the old id": a row wrapped
// under some third id — an interrupted rotation from a previous attempt, a
// restored row from an older backup — is work too, and counting only the known
// old id would call the rotation finished with that row still unaccounted for.
func (s *Store) CountCredentialsOutsideKEKIDIncludingDeleted(ctx context.Context, keepID uint32) (int64, error) {
	var n int64
	err := s.db.Read().QueryRowContext(ctx, `
		SELECT count(*) FROM service_instance
		 WHERE length(api_key_enc) >= ? AND kek_id <> ?`, crypto.MinEnvelopeLen, keepID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count credentials outside kek_id %d: %w", keepID, err)
	}
	return n, nil
}
