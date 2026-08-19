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

// The grab audit vocabulary, spelled ONCE for both sides of it.
//
// internal/httpapi writes these values and this package reads them back, and
// until now each side spelled them itself. That is a silent failure by
// construction: a row written with a different action string is not a wrong row
// in the Recent-grabs list, it is a row that never appears in it at all, and
// nothing fails. Exporting them from the side that does the reading makes the
// writer import the reader's spelling.
//
// The three results are audit_log.result's values. There is no CHECK constraint
// behind them, deliberately, so these are the vocabulary and not the schema.
const (
	AuditActionGrab = "release.grab"

	// AuditResultOK is a grab the upstream confirmed.
	AuditResultOK = "ok"

	// AuditResultWarn is a grab that WAS handed over and whose outcome UsArr
	// does not know. It is not a lesser failure: a warn row has a provenance row
	// behind it, which is why the Recent-grabs union reads warn rows from
	// provenance and must NOT also read them from here.
	AuditResultWarn = "warn"

	// AuditResultFail is a grab that provably never left the process. These rows
	// have no provenance row anywhere, so this table is the only record they
	// have, and they are the entire audit arm of the union.
	AuditResultFail = "fail"
)

// The key-rotation vocabulary, one action per phase of `usarr key rotate`.
//
// Three actions rather than one with a phase field, because CONFIGURATION.md
// §3.4 step 5 asks for one entry per phase and the phases are what an operator
// reconstructing an interrupted rotation needs to see in order.
//
// What each phase's row does and does NOT tell you, stated exactly, because the
// looser version of this comment was wrong and an operator would have acted on
// it: a prepare with no rewrap after it says the new key file was written; it
// says NOTHING about how many rows were re-wrapped afterwards, because the
// rewrap row is appended once, after the whole pass, so a run killed part-way
// through leaves rows already moved to the new id and no audit row naming a
// single one of them. Reconstruct that from service_instance.kek_id, not from
// here. A rewrap with no promote is the stronger reading and does hold: the key
// files were never touched, since promotion and its row are adjacent.
//
// A single action name would put all three in one bucket and make even that
// much a matter of parsing metadata.
//
// Metadata on these rows carries COUNTS AND KEY IDS ONLY. A key id is public —
// it is already in every envelope header and in service_instance.kek_id — and
// nothing else about a key belongs in a table that is append-only by design.
const (
	// AuditActionKeyRotatePrepare records the new key material being written
	// and registered. Both ids are live for decryption from this row onwards.
	AuditActionKeyRotatePrepare = "key.rotate.prepare"

	// AuditActionKeyRotateRewrap records the re-wrap pass and how many rows it
	// moved to the new id.
	AuditActionKeyRotateRewrap = "key.rotate.rewrap"

	// AuditActionKeyRotatePromote records secret.key.new becoming secret.key
	// and the superseded key leaving the keyring.
	AuditActionKeyRotatePromote = "key.rotate.promote"
)

// AuditQuery narrows a scoped audit read.
//
// Both filters are OR-within, AND-between: an empty slice means "any". They
// exist because the read that needs them — Recent grabs' not-sent arm, which is
// `action = 'release.grab' AND result = 'fail'` — is otherwise a full walk of a
// table that grows forever by design.
type AuditQuery struct {
	// Actions restricts to these action names. Leading column of the filter and
	// the second column of ix_audit_actor_action.
	Actions []string

	// Results restricts to these result values: "ok", "warn", "fail". Not
	// indexed — it narrows an already-indexed set.
	Results []string

	// Limit caps the rows returned. Zero or less means 100.
	Limit int
}

// auditListSQL renders one scoped audit read: the statement and its arguments.
//
// It is split out from ListAuditLog so TestAuditReadUsesTheActorActionIndex can
// EXPLAIN the query this function ACTUALLY issues rather than a copy of it
// pasted into a test, which is the way a query-plan assertion silently stops
// covering the query it names.
func auditListSQL(scope Scope, q AuditQuery) (string, []any) {
	// The scope is applied HERE, in the SQL, not merely accepted in the
	// signature. That distinction is the whole point of the parameter.
	userPred, args := scope.userPredicate("actor_user_id")

	var b strings.Builder
	b.WriteString(`SELECT id, ts, actor_user_id, actor_ip, action, target_type, target_id,
	       result, metadata_json, prev_hash
	  FROM audit_log
	 WHERE ` + userPred)
	if len(q.Actions) > 0 {
		b.WriteString(" AND action IN (" + placeholders(len(q.Actions)) + ")")
		for _, a := range q.Actions {
			args = append(args, a)
		}
	}
	if len(q.Results) > 0 {
		b.WriteString(" AND result IN (" + placeholders(len(q.Results)) + ")")
		for _, r := range q.Results {
			args = append(args, r)
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	// id DESC trails ts DESC because ts has one-second resolution: two rows
	// appended in the same second must still have a total order, or a paged read
	// repeats one. Same reasoning as ix_prov_user_grabbed's tiebreak.
	b.WriteString(" ORDER BY ts DESC, id DESC LIMIT ?")
	return b.String(), append(args, limit)
}

// ListAuditLog reads audit rows the given scope may see, newest first.
//
// IT TAKES THE ACCESS SCOPE IN ITS SIGNATURE AND APPLIES IT IN THE SQL, and
// both halves are load-bearing (ARCHITECTURE.md §1.3 rule 2). This used to take
// neither: any caller could read every user's actions, which for the grab path
// means one user's release titles, indexers and failures. That is the same class
// of leak GetProvenanceByDownloadID was fixed for, on the other arm of the same
// Recent-grabs union.
//
// A ROW WITH NO ACTOR IS INVISIBLE HERE, DELIBERATELY. The predicate is the
// canonical `actor_user_id IN (0, :uid)`, and audit_log.actor_user_id is
// nullable — SQL three-valued logic makes `NULL IN (0, 1)` unknown, so an
// unauthenticated action is attributable to nobody and is read by nobody. That
// is the fail-closed default, and it is why internal/httpapi's grab path
// deliberately writes NO audit row for a request that arrived with no session:
// a row nothing can ever read is cost without signal, and an unauthenticated
// endpoint that appends a row per request is a log-flood vector. Chain
// verification is a different job and reads every row — see VerifyAuditChain.
//
// ORDERING COSTS A SORT, and that is recorded rather than hidden.
// TestScopedAuditOrderNeedsASort pins it: SQLite cannot supply ORDER BY from an
// index whose LEADING column is constrained by IN, so the plan is a SEARCH on
// ix_audit_actor_action plus a temp b-tree. The index still cuts the walk from
// "every row ever appended" to "this actor's rows for this action", and the sort
// is over that bounded set under a LIMIT.
func (s *Store) ListAuditLog(ctx context.Context, scope Scope, q AuditQuery) ([]AuditEntry, error) {
	query, args := auditListSQL(scope, q)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
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

// NotSentGrabsQuery is the definite-failure arm of ARCHITECTURE.md §17.5's
// Recent-grabs block, expressed once.
//
// WHY `fail` AND NOT `warn`. The two arms of that union are not
// success-versus-failure, they are HANDED-OVER versus NEVER-HANDED-OVER. A warn
// row is a grab that reached Prowlarr and whose fate is unknown, and internal/
// releases writes a provenance row for it precisely so the download id survives
// — so a warn row is ALREADY in the provenance arm. Reading warn rows here as
// well would render that grab twice, once as "sent, outcome unknown" and once as
// a failure, which is the double-grab invitation the three-state model exists to
// prevent, printed on the same screen.
//
// The limit is clamped HERE, sharing RecentProvenance's bounds, for two reasons:
// the two arms of one union must be bounded identically or the merge is skewed
// toward whichever arm was allowed more rows, and — as with
// recentProvenanceSQL — a bound that lives in the caller can be left behind by
// the next caller.
func NotSentGrabsQuery(limit int) AuditQuery {
	switch {
	case limit <= 0:
		limit = RecentProvenanceDefaultLimit
	case limit > RecentProvenanceMaxLimit:
		limit = RecentProvenanceMaxLimit
	}
	return AuditQuery{
		Actions: []string{AuditActionGrab},
		Results: []string{AuditResultFail},
		Limit:   limit,
	}
}

// ListNotSentGrabs reads the grabs that provably never left this process, newest
// first, for the rows this scope may see.
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL, via ListAuditLog: these rows carry a
// user's release titles, their indexers and their failures, and a scope a query
// accepts but never filters on is indistinguishable from no scope at all.
//
// A ROW WITH NO ACTOR IS NOT RETURNED, which is the scoped predicate's doing
// rather than this function's: `NULL IN (0, :uid)` is unknown, not true. That is
// load-bearing downstream — internal/httpapi derives a per-user opaque id from
// actor_user_id, and a NULL actor would silently hash in the wrong domain — so
// the reader asserts it rather than trusting this sentence.
func (s *Store) ListNotSentGrabs(ctx context.Context, scope Scope, limit int) ([]AuditEntry, error) {
	return s.ListAuditLog(ctx, scope, NotSentGrabsQuery(limit))
}

// VerifyAuditChain recomputes the chain from the oldest row forward and
// reports the id of the first row whose hash does not match, or 0 if the chain
// is intact.
//
// It takes NO scope, on purpose: a chain is only verifiable whole. Skipping the
// rows one user may not read would break every link across the gap and report
// tampering that did not happen. This is an operator-level integrity check over
// the file, not a user-facing read of its contents — it returns an id and
// nothing else, so it discloses no row.
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
