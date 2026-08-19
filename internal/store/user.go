package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Session lifetimes are fixed, not configurable (docs/CONFIGURATION.md §1).
// A stolen cookie must not be a 30-day key to the vault.
const (
	SessionIdleTimeout     = 72 * time.Hour
	SessionAbsoluteTimeout = 720 * time.Hour

	// SudoWindow is how long a re-authentication covers a sensitive operation:
	// adding or changing a service credential, changing an instance's base_url,
	// downloading a backup, rotating the master key.
	SudoWindow = 5 * time.Minute
)

// User is an account. v0.1 ships one owner; the table carries everything
// multi-user needs from migration 0001 because retrofitting it touches every
// query, and hiding a UI is free.
type User struct {
	ID          int64
	Username    string
	DisplayName string
	Email       string

	// AuthSource is local|jellyfin|plex|tailscale. ExternalID is set for
	// everything but local.
	AuthSource string
	ExternalID string

	// PasswordHash is the full Argon2id PHC string, empty for external users.
	PasswordHash string

	IsOwner     bool
	IsDisabled  bool
	CreatedAt   time.Time
	LastLoginAt sql.NullString
}

// CreateUser inserts an account and returns its id.
func (s *Store) CreateUser(ctx context.Context, u User) (int64, error) {
	var id int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO user (username, display_name, email, auth_source, external_id,
			                  password_hash, is_owner, is_disabled)
			VALUES (?,?,?,?,?,?,?,?)`,
			u.Username, nullString(u.DisplayName), nullString(u.Email),
			defaultString(u.AuthSource, "local"), nullString(u.ExternalID),
			nullString(u.PasswordHash), u.IsOwner, u.IsDisabled)
		if err != nil {
			return fmt.Errorf("insert user %q: %w", u.Username, err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("insert user %q: last insert id: %w", u.Username, err)
		}
		return nil
	})
	return id, err
}

const userColumns = `id, username, display_name, email, auth_source, external_id,
                     password_hash, is_owner, is_disabled, created_at, last_login_at`

func scanUser(sc interface{ Scan(...any) error }) (User, error) {
	var u User
	var displayName, email, externalID, passwordHash sql.NullString
	var createdAt string
	if err := sc.Scan(&u.ID, &u.Username, &displayName, &email, &u.AuthSource, &externalID,
		&passwordHash, &u.IsOwner, &u.IsDisabled, &createdAt, &u.LastLoginAt); err != nil {
		return User{}, err
	}
	u.DisplayName, u.Email = displayName.String, email.String
	u.ExternalID, u.PasswordHash = externalID.String, passwordHash.String

	var err error
	if u.CreatedAt, err = ParseTime(createdAt); err != nil {
		return User{}, err
	}
	return u, nil
}

// GetUserByUsername reads an account by username.
//
// The system sentinel (id 0) is excluded: it is a foreign-key anchor for shared
// rows, not an account, and it must never be a login target.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	row := s.db.Read().QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM user WHERE username = ? AND id != ?`,
		username, SystemUserID)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("user %q: %w", username, ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("read user %q: %w", username, err)
	}
	return u, nil
}

// GetUser reads an account by id.
func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	row := s.db.Read().QueryRowContext(ctx, `SELECT `+userColumns+` FROM user WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("user %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("read user %d: %w", id, err)
	}
	return u, nil
}

// Owner reads the single v0.1 owner account.
func (s *Store) Owner(ctx context.Context) (User, error) {
	row := s.db.Read().QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM user WHERE is_owner = 1 AND id != ? ORDER BY id ASC LIMIT 1`,
		SystemUserID)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("owner account: %w", ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("read owner account: %w", err)
	}
	return u, nil
}

// TouchUserLogin records a successful login.
func (s *Store) TouchUserLogin(ctx context.Context, id int64, now time.Time) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE user SET last_login_at = ? WHERE id = ?`, FormatTime(now), id)
		if err != nil {
			return fmt.Errorf("update user %d last_login_at: %w", id, err)
		}
		return expectOneRow(res, "user", id)
	})
}

// Session is a logged-in browser or device.
//
// ID is a hash of the cookie value, never the cookie itself: a database read
// must not yield a usable session token.
type Session struct {
	ID                string
	UserID            int64
	Kind              string // web|device
	DeviceLabel       string
	UserAgent         string
	IP                string
	CreatedAt         time.Time
	LastSeenAt        time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	SudoUntil         sql.NullString
	RevokedAt         sql.NullString
}

// CreateSession inserts a session. The caller supplies the hashed id and the
// fixed lifetimes; nothing here reads a clock, so expiry is testable.
func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO session (id, user_id, kind, device_label, user_agent, ip,
			                     created_at, last_seen_at, idle_expires_at, absolute_expires_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			sess.ID, sess.UserID, sess.Kind, nullString(sess.DeviceLabel),
			nullString(sess.UserAgent), nullString(sess.IP),
			FormatTime(sess.CreatedAt), FormatTime(sess.LastSeenAt),
			FormatTime(sess.IdleExpiresAt), FormatTime(sess.AbsoluteExpiresAt))
		if err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		return nil
	})
}

// GetSession reads a live session by its hashed id.
//
// Revoked and expired sessions are reported as ErrNotFound: a caller
// authenticating a request has no use for the difference, and returning the row
// invites someone to use it anyway. now is a parameter so expiry is decided by
// the caller and testable without sleeping.
func (s *Store) GetSession(ctx context.Context, id string, now time.Time) (Session, error) {
	var sess Session
	var deviceLabel, userAgent, ip sql.NullString
	var createdAt, lastSeenAt, idleExpires, absExpires string

	err := s.db.Read().QueryRowContext(ctx, `
		SELECT id, user_id, kind, device_label, user_agent, ip,
		       created_at, last_seen_at, idle_expires_at, absolute_expires_at,
		       sudo_until, revoked_at
		  FROM session
		 WHERE id = ? AND revoked_at IS NULL
		   AND idle_expires_at > ? AND absolute_expires_at > ?`,
		id, FormatTime(now), FormatTime(now),
	).Scan(&sess.ID, &sess.UserID, &sess.Kind, &deviceLabel, &userAgent, &ip,
		&createdAt, &lastSeenAt, &idleExpires, &absExpires, &sess.SudoUntil, &sess.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, fmt.Errorf("session: %w", ErrNotFound)
	}
	if err != nil {
		return Session{}, fmt.Errorf("read session: %w", err)
	}

	sess.DeviceLabel, sess.UserAgent, sess.IP = deviceLabel.String, userAgent.String, ip.String
	for _, f := range []struct {
		dst *time.Time
		src string
	}{
		{&sess.CreatedAt, createdAt},
		{&sess.LastSeenAt, lastSeenAt},
		{&sess.IdleExpiresAt, idleExpires},
		{&sess.AbsoluteExpiresAt, absExpires},
	} {
		if *f.dst, err = ParseTime(f.src); err != nil {
			return Session{}, err
		}
	}
	return sess, nil
}

// TouchSession slides the idle window forward. The absolute expiry is never
// extended.
func (s *Store) TouchSession(ctx context.Context, id string, now time.Time) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE session SET last_seen_at = ?, idle_expires_at = ?
			 WHERE id = ? AND revoked_at IS NULL`,
			FormatTime(now), FormatTime(now.Add(SessionIdleTimeout)), id)
		if err != nil {
			return fmt.Errorf("touch session: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("touch session: rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("session: %w", ErrNotFound)
		}
		return nil
	})
}

// RegenerateSessionForSudo rewrites a session's id AND opens the sudo window,
// in one statement.
//
// reference/security.md §6 requires the session id to be regenerated on every
// privilege change. Login already satisfies that by minting a whole new row.
// Opening the sudo window is the OTHER privilege change UsArr has — it is the
// moment a session becomes able to add a service, edit a base URL or re-enter an
// \*Arr API key — and it used to be an UPDATE that left the id alone. Anyone
// holding the pre-sudo cookie value (a fixated cookie, a value captured from a
// proxy log or a shared machine) was carried across that transition with it.
//
// THE TWO WRITES ARE ONE STATEMENT ON PURPOSE. Split into "rotate the id" then
// "grant sudo" they are two writes with a window between them in which the row
// has a new id and no sudo, or — worse, in the other order — sudo under the old
// id. One UPDATE under the store's BEGIN IMMEDIATE writer has neither state.
//
// The absolute expiry is deliberately NOT extended: this is the same session
// with a new name, not a new session, and re-authenticating must not be a way to
// live past SessionAbsoluteTimeout for ever. created_at is left alone for the
// same reason.
//
// newID MUST differ from oldID, and that is checked rather than assumed. A
// caller that forgot to mint a fresh token would otherwise get a silent no-op
// that looks exactly like success — which is the bug this function exists to
// fix, reintroduced one layer down.
func (s *Store) RegenerateSessionForSudo(ctx context.Context, oldID, newID string, now time.Time) error {
	if newID == "" || newID == oldID {
		return fmt.Errorf("regenerate session: the new session id must be fresh")
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE session SET id = ?, sudo_until = ? WHERE id = ? AND revoked_at IS NULL`,
			newID, FormatTime(now.Add(SudoWindow)), oldID)
		if err != nil {
			// A PRIMARY KEY collision on newID lands here and rolls the whole
			// statement back. With 256 bits of crypto/rand behind the id that is
			// not a real event, but failing closed costs nothing.
			return fmt.Errorf("regenerate session: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("regenerate session: rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("session: %w", ErrNotFound)
		}
		return nil
	})
}

// GrantSudo opens the re-authentication window for sensitive operations.
//
// It does NOT regenerate the session id, so it is only correct where the id is
// already fresh — which today means exactly one caller, startSession, opening
// the window on a row it has just inserted. Every other privilege change must
// go through RegenerateSessionForSudo; see §6 and that function's comment.
func (s *Store) GrantSudo(ctx context.Context, id string, now time.Time) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE session SET sudo_until = ? WHERE id = ? AND revoked_at IS NULL`,
			FormatTime(now.Add(SudoWindow)), id)
		if err != nil {
			return fmt.Errorf("grant sudo: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("grant sudo: rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("session: %w", ErrNotFound)
		}
		return nil
	})
}

// RevokeSession ends one session.
func (s *Store) RevokeSession(ctx context.Context, id string, now time.Time) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE session SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
			FormatTime(now), id)
		if err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
		return nil
	})
}

// RevokeUserSessions ends every live session for one user. Driven by
// ix_session_user.
func (s *Store) RevokeUserSessions(ctx context.Context, userID int64, now time.Time) (int64, error) {
	var n int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE session SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
			FormatTime(now), userID)
		if err != nil {
			return fmt.Errorf("revoke sessions for user %d: %w", userID, err)
		}
		if n, err = res.RowsAffected(); err != nil {
			return fmt.Errorf("revoke sessions for user %d: rows affected: %w", userID, err)
		}
		return nil
	})
	return n, err
}
