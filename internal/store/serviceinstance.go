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

	// IndexersFetchedAt is when UsArr last successfully replicated this
	// instance's indexer list into indexer_catalog (migration 0004). INVALID
	// means never — which is a different state from "fetched, and this service
	// has zero indexers", and the indexers endpoint says a different sentence
	// for each. Written only on success, so it never claims a freshness the
	// replica does not have.
	IndexersFetchedAt sql.NullString

	// LastFullSyncAt is when this instance's last SUCCESSFULLY COMPLETED full
	// catalogue import BEGAN READING the upstream (migration 0005). INVALID
	// means NEVER, and it is a different fact from "synced, and the library was
	// empty" — the Services screen says a different sentence for each, exactly
	// as it does for IndexersFetchedAt.
	//
	// ⚠️ IT IS THE RUN'S START, NOT ITS FINISH, and not the instant any row was
	// written locally — that one is service_item_link.synced_at, which is per
	// row and moves per batch. The distinction is invisible on a healthy system
	// and is the whole content of the field on a degraded one, which is the only
	// time ARCHITECTURE.md §17.7's "showing cached data from 14:02" banner
	// renders. docs/reference/http-api.md §3.5 is the contract; StampFullSync
	// carries the argument for the choice.
	//
	// It is written ON SUCCESS ONLY, by StampFullSync, so it never claims a
	// freshness the replica does not have: a partial import leaves its
	// committed batches standing and this column untouched.
	LastFullSyncAt sql.NullString

	DeletedAt sql.NullString

	// ── THE FOUR GUARD-2 COLUMNS ARE DELIBERATELY ABSENT FROM THIS STRUCT, AND
	// THAT IS A RECORDED STATE RATHER THAN AN OVERSIGHT.
	//
	// service_instance carries identity_fingerprint, identity_epoch,
	// needs_reidentification and max_remote_id_seen (00001_initial.sql:155-158,
	// under its own "-- identity generation guard (sync.md §4)" banner). None of
	// them is selected here or named in serviceInstanceColumns below.
	//
	// ⚠️ NOTHING WRITES OR READS ANY OF THEM. Measured over the tree at d9a3f37:
	// identity_fingerprint, identity_epoch and max_remote_id_seen have ZERO Go
	// references of any kind. needs_reidentification has two and neither touches
	// the column — httpapi's healthState COMPARES health_state against the STRING
	// "needs_reidentification" (and against stateReID, "needs re-identification"),
	// and the other is a TEST FIXTURE assigning that same string to HealthState.
	// The state travels through health_state; the column named for it is dead.
	//
	// THE SEAM IS KEPT BECAUSE ITS PREMISE IS ONLY VOID FOR ONE SOURCE, AND THIS
	// IS THE PART A LATER READER MUST NOT GENERALISE. Guard 2 answers an id space
	// that moves BACKWARDS, which sync.md §4 grounds on SQLite's rowid allocator —
	// "ids are reused after deletion". That premise is LIVE for every *Arr and is
	// measured VOID for BookOrbit: books.id and libraries.id are PostgreSQL
	// serial, and setval(, SQL TRUNCATE and RESTART IDENTITY are all absent from
	// server/src at bookorbit/bookorbit@73b7877d2fed. So ADR-0074 (2026-08-21)
	// DEFERS guard 2 FOR BOOKORBIT ONLY and keeps these columns for the *Arr
	// adapters, which are re-sequenced rather than cut (ADR-0042).
	//
	// ⚠️ AND WHAT SURVIVES THE VOID PREMISE IS A NAMED GAP WITH NO GUARD, not a
	// hazard some other channel absorbs: an older pg_dump restored out of band
	// rewinds the sequence, and the instance can be repointed at a different or
	// rebuilt server. BookOrbit exposes no instance identity to fingerprint —
	// a four-term search of server/src for
	// instanceId|installationId|serverUuid|instance_uuid returns zero files, and
	// the same search shape over four terms that ARE present returns 93 files, so
	// the search was working rather than broken. The only repair for either case
	// is the manual full import.
	//
	// So guard 1 carries this source alone, which is why ADR-0074 required it
	// WIRED — applyOneItem's step 1a now compares remote_identity_hash, and a
	// value nothing compares is not a guard. ⚠️ ITS REACH IS STILL BOUNDED: the
	// comparison is over the item's external ids, so an item with none hashes
	// like every other item with none and the guard passes a reused id through.
	// That is ADR-0074's third named gap, and it has no guard either.
	//
	// THE FORM OF THIS ANNOTATION IS libraries.go's Library.OrphanedAt, NOT ITS
	// PLACEMENT: that one annotates a field that IS selected, and what is being
	// annotated here is these four columns' absence from the struct.
}

const serviceInstanceColumns = `
  id, kind, role, name, base_url, url_base, api_key_enc, kek_id,
  api_version, verify_tls, tls_spki_pin, enabled, priority, managed_by,
  health_state, breaker_state, breaker_until, consecutive_failures,
  last_ok_at, last_error, indexers_fetched_at, last_full_sync_at, deleted_at`

func scanServiceInstance(sc interface{ Scan(...any) error }) (ServiceInstance, error) {
	var si ServiceInstance
	var apiVersion sql.NullString
	err := sc.Scan(
		&si.ID, &si.Kind, &si.Role, &si.Name, &si.BaseURL, &si.URLBase, &si.APIKeyEnc, &si.KEKID,
		&apiVersion, &si.VerifyTLS, &si.TLSSPKIPin, &si.Enabled, &si.Priority, &si.ManagedBy,
		&si.HealthState, &si.BreakerState, &si.BreakerUntil, &si.ConsecutiveFailures,
		&si.LastOKAt, &si.LastError, &si.IndexersFetchedAt, &si.LastFullSyncAt, &si.DeletedAt,
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
	return getServiceInstance(ctx, s.db.Read(), scope, id)
}

// getServiceInstance is GetServiceInstance's body, over any querier, so a caller
// that must see this row in the same snapshot as something else can pass its own
// read transaction. See ReadIndexerCatalog.
func getServiceInstance(ctx context.Context, q querier, scope Scope, id int64) (ServiceInstance, error) {
	pred, args := scope.instancePredicate("id")
	query := `SELECT` + serviceInstanceColumns + `
		FROM service_instance
		WHERE id = ? AND deleted_at IS NULL AND ` + pred
	row := q.QueryRowContext(ctx, query, append([]any{id}, args...)...)

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
	return listServiceInstances(ctx, s.db.Read(), scope)
}

// listServiceInstances is ListServiceInstances's body, over any querier. See
// getServiceInstance.
func listServiceInstances(ctx context.Context, q querier, scope Scope) ([]ServiceInstance, error) {
	pred, args := scope.instancePredicate("id")
	query := `SELECT` + serviceInstanceColumns + `
		FROM service_instance
		WHERE deleted_at IS NULL AND ` + pred + `
		ORDER BY priority DESC, name ASC`

	rows, err := q.QueryContext(ctx, query, args...)
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
