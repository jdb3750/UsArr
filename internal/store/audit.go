package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AuditEntry is one appended audit row.
//
// MetadataJSON must never contain a secret VALUE. Names, hosts and ids are
// fine; an API key, a password or a token is not. The redaction middleware
// strips credentials before any log line, audit row, error string or support
// bundle exists — this struct is downstream of that, not a second chance.
type AuditEntry struct {
	ID          int64
	TS          time.Time
	ActorUserID sql.NullInt64
	ActorIP     string
	Action      string
	TargetType  string
	TargetID    string

	// Result is the outcome: "ok", "warn", "fail". Not a CHECK constraint in
	// the schema, so it is not enforced here either.
	//
	// "warn" is for an action whose outcome UsArr genuinely does not know, and
	// it is not decoration: the grab path uses it for a release that WAS sent to
	// Prowlarr and whose fate is unknown, which must not be filed next to a grab
	// that provably never left the process. See internal/httpapi/grab.go.
	Result       string
	MetadataJSON string
	PrevHash     string
}

// AppendAudit appends one audit row and returns its id.
//
// There is no update and no delete: migration 0001 installs BEFORE UPDATE and
// BEFORE DELETE triggers that RAISE(ABORT). The chain makes tampering EVIDENT,
// not impossible — someone with the database file can rewrite the whole chain,
// but they cannot edit one row and leave the rest verifying.
//
// The read of the previous row and the insert are in one BEGIN IMMEDIATE
// transaction, so two concurrent appends cannot both chain off the same
// predecessor. With a single writer connection that is already true, but the
// transaction is what makes it true rather than the pool size.
func (s *Store) AppendAudit(ctx context.Context, e AuditEntry) (int64, error) {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	var id int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var prevHash sql.NullString
		err := tx.QueryRowContext(ctx,
			`SELECT prev_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prevHash)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("append audit %q: read chain head: %w", e.Action, err)
		}

		e.PrevHash = auditHash(prevHash.String, e)
		res, err := tx.ExecContext(ctx, `
			INSERT INTO audit_log (
			  ts, actor_user_id, actor_ip, action, target_type, target_id,
			  result, metadata_json, prev_hash
			) VALUES (?,?,?,?,?,?,?,?,?)`,
			FormatTime(e.TS), e.ActorUserID, nullString(e.ActorIP), e.Action,
			nullString(e.TargetType), nullString(e.TargetID), e.Result,
			nullString(e.MetadataJSON), e.PrevHash)
		if err != nil {
			return fmt.Errorf("append audit %q: %w", e.Action, err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("append audit %q: last insert id: %w", e.Action, err)
		}
		return nil
	})
	return id, err
}

// auditHash computes the rolling chain value: sha256(prev_hash || row).
//
// The row serialisation is defined here because the schema says
// "sha256(prev_hash || row)" without fixing one. Fields are joined with a
// separator that cannot appear in a field (0x1f, unit separator), so no
// combination of field values can produce the same digest as a different
// combination. Changing this function invalidates every existing chain, so it
// needs a migration that re-chains, not an edit.
func auditHash(prev string, e AuditEntry) string {
	h := sha256.New()
	h.Write([]byte(prev))
	fields := []string{
		FormatTime(e.TS),
		actorString(e.ActorUserID),
		e.ActorIP,
		e.Action,
		e.TargetType,
		e.TargetID,
		e.Result,
		e.MetadataJSON,
	}
	h.Write([]byte{0x1f})
	h.Write([]byte(strings.Join(fields, "\x1f")))
	return hex.EncodeToString(h.Sum(nil))
}

func actorString(v sql.NullInt64) string {
	if !v.Valid {
		return ""
	}
	return strconv.FormatInt(v.Int64, 10)
}

// ListAuditLog reads the most recent audit rows, newest first. Driven by
// ix_audit_ts.
func (s *Store) ListAuditLog(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT id, ts, actor_user_id, actor_ip, action, target_type, target_id,
		       result, metadata_json, prev_hash
		  FROM audit_log
		 ORDER BY ts DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit_log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts string
		var actorIP, targetType, targetID, metadata, prevHash sql.NullString
		if err := rows.Scan(&e.ID, &ts, &e.ActorUserID, &actorIP, &e.Action,
			&targetType, &targetID, &e.Result, &metadata, &prevHash); err != nil {
			return nil, fmt.Errorf("list audit_log: scan: %w", err)
		}
		if e.TS, err = ParseTime(ts); err != nil {
			return nil, err
		}
		e.ActorIP, e.TargetType, e.TargetID = actorIP.String, targetType.String, targetID.String
		e.MetadataJSON, e.PrevHash = metadata.String, prevHash.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list audit_log: %w", err)
	}
	return out, nil
}

// VerifyAuditChain recomputes the chain from the oldest row forward and
// reports the id of the first row whose hash does not match, or 0 if the chain
// is intact.
func (s *Store) VerifyAuditChain(ctx context.Context) (int64, error) {
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT id, ts, actor_user_id, actor_ip, action, target_type, target_id,
		       result, metadata_json, prev_hash
		  FROM audit_log
		 ORDER BY id ASC`)
	if err != nil {
		return 0, fmt.Errorf("verify audit chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var prev string
	for rows.Next() {
		var e AuditEntry
		var ts string
		var actorIP, targetType, targetID, metadata, stored sql.NullString
		if err := rows.Scan(&e.ID, &ts, &e.ActorUserID, &actorIP, &e.Action,
			&targetType, &targetID, &e.Result, &metadata, &stored); err != nil {
			return 0, fmt.Errorf("verify audit chain: scan: %w", err)
		}
		if e.TS, err = ParseTime(ts); err != nil {
			return 0, err
		}
		e.ActorIP, e.TargetType, e.TargetID = actorIP.String, targetType.String, targetID.String
		e.MetadataJSON = metadata.String

		if want := auditHash(prev, e); want != stored.String {
			return e.ID, nil
		}
		prev = stored.String
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("verify audit chain: %w", err)
	}
	return 0, nil
}
