package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
)

func TestUserCreateAndRead(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	phc, err := crypto.HashPassword(t.Context(), "a good long passphrase")
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateUser(ctx, User{
		Username: "joe", DisplayName: "Joe", AuthSource: "local",
		PasswordHash: phc, IsOwner: true,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id == SystemUserID {
		t.Fatal("a real account was given the sentinel id 0")
	}

	u, err := s.GetUserByUsername(ctx, "joe")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if u.ID != id || !u.IsOwner || u.AuthSource != "local" {
		t.Errorf("round trip wrong: %+v", u)
	}
	if err := crypto.VerifyPassword(t.Context(), u.PasswordHash, "a good long passphrase"); err != nil {
		t.Errorf("stored password hash does not verify: %v", err)
	}

	owner, err := s.Owner(ctx)
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owner.ID != id {
		t.Errorf("Owner = %d, want %d", owner.ID, id)
	}

	if err := s.TouchUserLogin(ctx, id, testNow); err != nil {
		t.Fatalf("TouchUserLogin: %v", err)
	}
	if u, err = s.GetUser(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !u.LastLoginAt.Valid {
		t.Error("last_login_at not recorded")
	}

	if _, err := s.GetUserByUsername(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUserByUsername on a missing user = %v, want ErrNotFound", err)
	}
}

// The id-0 sentinel is a foreign-key anchor for shared rows, not an account. It
// must never be reachable as a login target.
func TestSystemSentinelIsNotALoginTarget(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	if _, err := s.GetUserByUsername(ctx, "_system"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUserByUsername(\"_system\") = %v, want ErrNotFound", err)
	}
	if _, err := s.Owner(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("Owner() with no real account = %v, want ErrNotFound", err)
	}

	// It does exist, and it has no password.
	u, err := s.GetUser(ctx, SystemUserID)
	if err != nil {
		t.Fatalf("the sentinel row is missing: %v", err)
	}
	if u.PasswordHash != "" || !u.IsDisabled {
		t.Errorf("sentinel is not inert: %+v", u)
	}
}

func TestUsernameIsUnique(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	if _, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local"}); err == nil {
		t.Error("a duplicate username was accepted")
	}
}

func TestAuthSourceIsConstrained(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	for _, ok := range []string{"local", "jellyfin", "plex", "tailscale"} {
		if _, err := s.CreateUser(ctx, User{Username: "u-" + ok, AuthSource: ok}); err != nil {
			t.Errorf("auth_source %q rejected: %v", ok, err)
		}
	}
	if _, err := s.CreateUser(ctx, User{Username: "bad", AuthSource: "ldap"}); err == nil {
		t.Error("the CHECK constraint accepted an unknown auth_source")
	}
}

func newSession(userID int64, id string, now time.Time) Session {
	return Session{
		ID: id, UserID: userID, Kind: "web",
		CreatedAt: now, LastSeenAt: now,
		IdleExpiresAt:     now.Add(SessionIdleTimeout),
		AbsoluteExpiresAt: now.Add(SessionAbsoluteTimeout),
	}
}

func TestSessionLifecycle(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	userID, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local", IsOwner: true})
	if err != nil {
		t.Fatal(err)
	}

	// The stored id is a hash of the cookie, never the cookie itself.
	sess := newSession(userID, "hash-of-cookie", testNow)
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSession(ctx, sess.ID, testNow)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.UserID != userID || got.Kind != "web" {
		t.Errorf("round trip wrong: %+v", got)
	}

	// Idle expiry.
	if _, err := s.GetSession(ctx, sess.ID, testNow.Add(SessionIdleTimeout+time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Errorf("an idle-expired session was returned: %v", err)
	}

	// Touching slides the idle window forward.
	later := testNow.Add(time.Hour)
	if err := s.TouchSession(ctx, sess.ID, later); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	if _, err := s.GetSession(ctx, sess.ID, later.Add(SessionIdleTimeout-time.Minute)); err != nil {
		t.Errorf("session expired despite being touched: %v", err)
	}

	// But the absolute expiry is never extended: a stolen cookie must not be a
	// 30-day key to the vault.
	beyond := testNow.Add(SessionAbsoluteTimeout + time.Hour)
	if err := s.TouchSession(ctx, sess.ID, beyond); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, sess.ID, beyond); !errors.Is(err, ErrNotFound) {
		t.Error("a session past its absolute expiry was returned")
	}
}

func TestSessionSudoWindow(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	userID, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local"})
	if err != nil {
		t.Fatal(err)
	}
	sess := newSession(userID, "sid", testNow)
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSession(ctx, sess.ID, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if got.SudoUntil.Valid {
		t.Error("a new session already has sudo")
	}

	if err := s.GrantSudo(ctx, sess.ID, testNow); err != nil {
		t.Fatalf("GrantSudo: %v", err)
	}
	got, err = s.GetSession(ctx, sess.ID, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SudoUntil.Valid {
		t.Fatal("GrantSudo did not set sudo_until")
	}
	until, err := ParseTime(got.SudoUntil.String)
	if err != nil {
		t.Fatal(err)
	}
	if want := testNow.Add(SudoWindow); !until.Equal(want) {
		t.Errorf("sudo_until = %v, want %v (a 5-minute window)", until, want)
	}
}

func TestSessionRevocation(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	userID, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		if err := s.CreateSession(ctx, newSession(userID, id, testNow)); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.RevokeSession(ctx, "a", testNow); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, err := s.GetSession(ctx, "a", testNow); !errors.Is(err, ErrNotFound) {
		t.Errorf("a revoked session was returned: %v", err)
	}

	n, err := s.RevokeUserSessions(ctx, userID, testNow)
	if err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}
	if n != 2 {
		t.Errorf("revoked %d sessions, want the 2 still live", n)
	}
	for _, id := range []string{"b", "c"} {
		if _, err := s.GetSession(ctx, id, testNow); !errors.Is(err, ErrNotFound) {
			t.Errorf("session %s survived a bulk revoke", id)
		}
	}
}

// Deleting a user cascades to their sessions.
func TestSessionCascadesOnUserDelete(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	userID, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, newSession(userID, "sid", testNow)); err != nil {
		t.Fatal(err)
	}

	if err := s.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user WHERE id = ?`, userID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, "sid", testNow); !errors.Is(err, ErrNotFound) {
		t.Error("the session survived its user being deleted; ON DELETE CASCADE did not fire")
	}
}

// TestUserDeleteSucceedsWhenTheUserHasAuditRows is the regression test for DB-02.
//
// TestSessionCascadesOnUserDelete above deletes a user who has sessions but no
// audit rows, which is why it never caught this. audit_log.actor_user_id used to
// read `REFERENCES user(id) ON DELETE SET NULL`; SET NULL performs an implicit
// UPDATE on audit_log, trg_audit_no_update RAISE(ABORT)s it, and the whole
// DELETE FROM user fails with "audit_log is append-only". Logging in writes an
// audit row, so no user who had ever logged in could be deleted — baked into
// migration 0001.
//
// Note that ON DELETE NO ACTION does not fix this either: it leaves the FK
// violated instead of firing the trigger, so the delete still fails, only with a
// different message. The column carries no reference at all, and this test locks
// in both halves of that: the delete succeeds, AND the audit row still names who
// acted. Nulling the actor would destroy exactly the record the log exists for.
func TestUserDeleteSucceedsWhenTheUserHasAuditRows(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	actor, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local", IsOwner: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAudit(ctx, AuditEntry{
		TS:          testNow,
		ActorUserID: sql.NullInt64{Int64: actor, Valid: true},
		ActorIP:     "192.168.1.2",
		Action:      "user.delete",
		TargetType:  "user",
		TargetID:    "9",
		Result:      "ok",
	}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}

	if err := s.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user WHERE id = ?`, actor)
		return err
	}); err != nil {
		t.Fatalf("deleting a user who has audit rows failed: %v\n"+
			"audit_log.actor_user_id must not carry a foreign key to user(id) — any "+
			"ON DELETE action fights the append-only trigger and makes deletion impossible", err)
	}

	if _, err := s.GetUser(ctx, actor); !errors.Is(err, ErrNotFound) {
		t.Errorf("the user survived the delete: %v", err)
	}

	// The audit row must survive intact, still naming the (now deleted) actor.
	entries, err := s.ListAuditLog(ctx, OwnerScope(actor), AuditQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d audit entries, want 1", len(entries))
	}
	got := entries[0]
	if !got.ActorUserID.Valid || got.ActorUserID.Int64 != actor {
		t.Errorf("actor_user_id = %+v, want %d — the log must still record who acted "+
			"after that user is gone; that is what the log is for", got.ActorUserID, actor)
	}
	if got.Action != "user.delete" {
		t.Errorf("action = %q", got.Action)
	}

	// The hash chain must still verify: nothing rewrote the row underneath it.
	if _, err := s.VerifyAuditChain(ctx); err != nil {
		t.Errorf("audit chain broken after a user delete: %v", err)
	}
}

// ─── Audit log ───────────────────────────────────────────────────────────────

func TestAuditAppendAndChain(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	// actor_user_id carries no foreign key (see TestUserDeleteSucceedsWhenTheUserHasAuditRows),
	// but a real actor row is what the realistic case looks like.
	actor, err := s.CreateUser(ctx, User{Username: "joe", AuthSource: "local", IsOwner: true})
	if err != nil {
		t.Fatal(err)
	}

	for i, action := range []string{"service.created", "service.base_url.changed", "key.rotated"} {
		if _, err := s.AppendAudit(ctx, AuditEntry{
			TS:          testNow.Add(time.Duration(i) * time.Second),
			ActorUserID: sql.NullInt64{Int64: actor, Valid: true},
			ActorIP:     "192.168.1.2",
			Action:      action,
			TargetType:  "service_instance",
			TargetID:    "1",
			Result:      "ok",
		}); err != nil {
			t.Fatalf("AppendAudit(%s): %v", action, err)
		}
	}

	entries, err := s.ListAuditLog(ctx, OwnerScope(actor), AuditQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	// Newest first.
	if entries[0].Action != "key.rotated" {
		t.Errorf("first entry = %q, want the newest", entries[0].Action)
	}
	// Every row is chained.
	for _, e := range entries {
		if len(e.PrevHash) != 64 {
			t.Errorf("entry %d prev_hash = %q, want a 64-char sha256 hex", e.ID, e.PrevHash)
		}
	}

	bad, err := s.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if bad != 0 {
		t.Errorf("VerifyAuditChain flagged row %d on an untampered log", bad)
	}
}

// The chain makes tampering EVIDENT, not impossible.
func TestAuditChainDetectsTampering(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	for i := range 3 {
		if _, err := s.AppendAudit(ctx, AuditEntry{
			TS: testNow.Add(time.Duration(i) * time.Second), Action: "a", Result: "ok",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// The triggers block UPDATE, so tampering means dropping them first —
	// which is exactly what someone with the database file would do.
	if err := s.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DROP TRIGGER trg_audit_no_update`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE audit_log SET result = 'tampered' WHERE id = 2`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	bad, err := s.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if bad != 2 {
		t.Errorf("VerifyAuditChain = %d, want the tampered row 2", bad)
	}
}

// The triggers are the mechanism, not a convention: a caller reaching for
// UPDATE or DELETE gets an abort.
func TestAuditLogIsAppendOnly(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	if _, err := s.AppendAudit(ctx, AuditEntry{TS: testNow, Action: "a", Result: "ok"}); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`UPDATE audit_log SET result = 'x'`,
		`DELETE FROM audit_log`,
	} {
		err := s.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt)
			return err
		})
		if err == nil {
			t.Errorf("%q succeeded against an append-only table", stmt)
			continue
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%q failed with %v, want the trigger message", stmt, err)
		}
	}
}
