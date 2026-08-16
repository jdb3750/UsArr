package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a row does not exist, or exists but is outside
// the caller's scope. The two are deliberately indistinguishable: telling a
// user that a row they cannot see exists is an existence oracle.
var ErrNotFound = errors.New("store: not found")

// ServiceInstance is one configured *Arr or media backend.
//
// APIKeyEnc holds the versioned envelope produced by internal/crypto — never a
// plaintext key. KEKID duplicates the key id inside that envelope as a plain
// column so a rotation can `SELECT … WHERE kek_id = :old` and resume; the two
// must always agree.
type ServiceInstance struct {
	ID        int64
	Kind      string
	Role      string
	Name      string
	BaseURL   string
	URLBase   string
	APIKeyEnc []byte
	KEKID     uint32

	APIVersion string
	VerifyTLS  bool
	TLSSPKIPin []byte
	Enabled    bool
	Priority   int64
	ManagedBy  string

	HealthState         string
	BreakerState        string
	BreakerUntil        sql.NullString
	ConsecutiveFailures int64
	LastOKAt            sql.NullString
	LastError           sql.NullString

	DeletedAt sql.NullString
}

const serviceInstanceColumns = `
  id, kind, role, name, base_url, url_base, api_key_enc, kek_id,
  api_version, verify_tls, tls_spki_pin, enabled, priority, managed_by,
  health_state, breaker_state, breaker_until, consecutive_failures,
  last_ok_at, last_error, deleted_at`

func scanServiceInstance(sc interface{ Scan(...any) error }) (ServiceInstance, error) {
	var si ServiceInstance
	var apiVersion sql.NullString
	err := sc.Scan(
		&si.ID, &si.Kind, &si.Role, &si.Name, &si.BaseURL, &si.URLBase, &si.APIKeyEnc, &si.KEKID,
		&apiVersion, &si.VerifyTLS, &si.TLSSPKIPin, &si.Enabled, &si.Priority, &si.ManagedBy,
		&si.HealthState, &si.BreakerState, &si.BreakerUntil, &si.ConsecutiveFailures,
		&si.LastOKAt, &si.LastError, &si.DeletedAt,
	)
	if err != nil {
		return ServiceInstance{}, err
	}
	si.APIVersion = apiVersion.String
	return si, nil
}

// CreateServiceInstance inserts an instance and returns its id.
//
// The caller must have sealed the API key already: this function never sees a
// plaintext credential. Note the ordering problem it creates — the AAD binds
// the ciphertext to the row's primary key, which does not exist until the
// insert. Use CreateServiceInstanceSealed, which does both halves in one
// transaction.
func (s *Store) CreateServiceInstance(ctx context.Context, si ServiceInstance) (int64, error) {
	var id int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		id, err = insertServiceInstance(ctx, tx, si)
		return err
	})
	return id, err
}

// CreateServiceInstanceSealed inserts an instance whose API key can only be
// sealed once the row id is known.
//
// seal is called inside the write transaction with the freshly allocated id; it
// returns the envelope and the key id it used. If seal fails the insert is
// rolled back, so a row never exists with an unsealed or absent credential.
func (s *Store) CreateServiceInstanceSealed(
	ctx context.Context,
	si ServiceInstance,
	seal func(id int64) (envelope []byte, kekID uint32, err error),
) (int64, error) {
	var id int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// api_key_enc is NOT NULL, so the insert needs a placeholder that the
		// UPDATE below replaces. A single-byte blob, never a valid envelope:
		// crypto.MinEnvelopeLen rejects it, so a crash between the two
		// statements cannot leave something that decrypts to anything.
		si.APIKeyEnc = []byte{0}
		si.KEKID = 0
		var err error
		if id, err = insertServiceInstance(ctx, tx, si); err != nil {
			return err
		}

		envelope, kekID, err := seal(id)
		if err != nil {
			return fmt.Errorf("seal api key for service_instance %d: %w", id, err)
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE service_instance SET api_key_enc = ?, kek_id = ? WHERE id = ?`,
			envelope, kekID, id)
		if err != nil {
			return fmt.Errorf("store api key for service_instance %d: %w", id, err)
		}
		return nil
	})
	return id, err
}

func insertServiceInstance(ctx context.Context, tx *sql.Tx, si ServiceInstance) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO service_instance (
		  kind, role, name, base_url, url_base, api_key_enc, kek_id,
		  api_version, verify_tls, tls_spki_pin, enabled, priority, managed_by
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		si.Kind, defaultString(si.Role, "library"), si.Name, si.BaseURL, si.URLBase,
		si.APIKeyEnc, si.KEKID, nullString(si.APIVersion), si.VerifyTLS, si.TLSSPKIPin,
		si.Enabled, si.Priority, defaultString(si.ManagedBy, "ui"),
	)
	if err != nil {
		return 0, fmt.Errorf("insert service_instance %q: %w", si.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert service_instance %q: last insert id: %w", si.Name, err)
	}
	return id, nil
}

// GetServiceInstance reads one instance within scope. Soft-deleted rows are
// excluded: the id stays burned but the row is gone as far as callers are
// concerned.
func (s *Store) GetServiceInstance(ctx context.Context, scope Scope, id int64) (ServiceInstance, error) {
	pred, args := scope.instancePredicate("id")
	query := `SELECT` + serviceInstanceColumns + `
		FROM service_instance
		WHERE id = ? AND deleted_at IS NULL AND ` + pred
	row := s.db.Read().QueryRowContext(ctx, query, append([]any{id}, args...)...)

	si, err := scanServiceInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ServiceInstance{}, fmt.Errorf("service_instance %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return ServiceInstance{}, fmt.Errorf("read service_instance %d: %w", id, err)
	}
	return si, nil
}

// ListServiceInstances reads every instance visible in scope, ordered by
// priority then name so the services screen is stable across reloads.
func (s *Store) ListServiceInstances(ctx context.Context, scope Scope) ([]ServiceInstance, error) {
	pred, args := scope.instancePredicate("id")
	query := `SELECT` + serviceInstanceColumns + `
		FROM service_instance
		WHERE deleted_at IS NULL AND ` + pred + `
		ORDER BY priority DESC, name ASC`

	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list service_instance: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ServiceInstance
	for rows.Next() {
		si, err := scanServiceInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("list service_instance: scan: %w", err)
		}
		out = append(out, si)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list service_instance: %w", err)
	}
	return out, nil
}

// UpdateServiceInstanceCredential replaces the stored envelope and key id.
//
// Rotation uses this, and so does the re-entry flow: changing base_url's
// scheme, host or port changes the AAD, so the old ciphertext can no longer be
// opened and the user must supply the key again
// (docs/reference/security.md §1.6).
func (s *Store) UpdateServiceInstanceCredential(ctx context.Context, id int64, envelope []byte, kekID uint32) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET api_key_enc = ?, kek_id = ? WHERE id = ? AND deleted_at IS NULL`,
			envelope, kekID, id)
		if err != nil {
			return fmt.Errorf("update service_instance %d credential: %w", id, err)
		}
		return expectOneRow(res, "service_instance", id)
	})
}

// UpdateServiceInstanceHealth records the outcome of a health probe.
func (s *Store) UpdateServiceInstanceHealth(
	ctx context.Context, id int64, healthState, breakerState string,
	consecutiveFailures int64, lastOK *time.Time, lastError string,
) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var lastOKAt any
		if lastOK != nil {
			lastOKAt = FormatTime(*lastOK)
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE service_instance
			   SET health_state = ?, breaker_state = ?, consecutive_failures = ?,
			       last_ok_at = COALESCE(?, last_ok_at), last_error = ?
			 WHERE id = ? AND deleted_at IS NULL`,
			healthState, breakerState, consecutiveFailures, lastOKAt, nullString(lastError), id)
		if err != nil {
			return fmt.Errorf("update service_instance %d health: %w", id, err)
		}
		return expectOneRow(res, "service_instance", id)
	})
}

// SoftDeleteServiceInstance tombstones an instance. The id stays burned and is
// never reused, so a stale northbound reference resolves to "gone" rather than
// to some other service.
func (s *Store) SoftDeleteServiceInstance(ctx context.Context, id int64, now time.Time) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET deleted_at = ?, enabled = 0 WHERE id = ? AND deleted_at IS NULL`,
			FormatTime(now), id)
		if err != nil {
			return fmt.Errorf("soft delete service_instance %d: %w", id, err)
		}
		return expectOneRow(res, "service_instance", id)
	})
}

// CountEncryptedCredentials reports how many instances hold an encrypted
// credential.
//
// This is the question the startup path asks after config.ResolveMasterKey
// returns ErrKeyAbsent: with no key supplied and rows present, UsArr refuses to
// start rather than generating a fresh key that would leave every stored
// credential permanently unreadable. Tombstoned rows count — their ciphertext
// is still there.
func (s *Store) CountEncryptedCredentials(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM service_instance WHERE length(api_key_enc) > 0`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count encrypted credentials: %w", err)
	}
	return n, nil
}

func expectOneRow(res sql.Result, table string, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update %s %d: rows affected: %w", table, id, err)
	}
	if n == 0 {
		return fmt.Errorf("%s %d: %w", table, id, ErrNotFound)
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func defaultString(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
