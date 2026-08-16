package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(t.Context(), filepath.Join(t.TempDir(), "usarr.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return d
}

// wantPragmas is every pragma from docs/reference/sync.md §6 that reads back as
// an integer.
var wantPragmas = map[string]int64{
	"foreign_keys":       1,
	"busy_timeout":       5000,
	"synchronous":        1, // NORMAL
	"temp_store":         2, // MEMORY
	"wal_autocheckpoint": 1000,
	"cache_size":         -8000,
}

// foreign_keys is off by default in SQLite and is PER-CONNECTION. Setting it
// once at startup would leave every other pooled connection running without
// referential integrity, so this holds every read-pool connection open at once
// and checks each one, rather than sampling the same recycled connection.
func TestPragmasOnEveryConnection(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	readers := d.Read().Stats().MaxOpenConnections
	if readers < 2 {
		t.Fatalf("read pool max connections = %d, want NumCPU*2", readers)
	}

	var wg sync.WaitGroup
	release := make(chan struct{})
	errs := make(chan error, readers)

	for i := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := d.Read().Conn(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer func() { _ = conn.Close() }()

			for name, want := range wantPragmas {
				got, err := PragmaInt(ctx, conn, name)
				if err != nil {
					errs <- err
					return
				}
				if got != want {
					t.Errorf("read connection %d: PRAGMA %s = %d, want %d", i, name, got, want)
				}
			}
			// Hold the connection until every goroutine has one, so the pool
			// really did hand out `readers` distinct connections.
			<-release
		}()
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("read pool connection: %v", err)
	}

	// The single writer connection gets them too.
	for name, want := range wantPragmas {
		got, err := PragmaInt(ctx, d.Writer(), name)
		if err != nil {
			t.Fatalf("write pool: PRAGMA %s: %v", name, err)
		}
		if got != want {
			t.Errorf("write pool: PRAGMA %s = %d, want %d", name, got, want)
		}
	}

	// journal_mode reads back as text, and is a database property rather than a
	// per-connection one.
	var mode string
	if err := d.Read().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

// mmap_size was removed from the pragma list because this driver compiles
// memory-mapped I/O out (ADR-0001 amendment): every value requested reads back
// as 0. This guards the removal from both sides — the pragma is not in the list,
// and setting it would not work anyway — so anyone re-adding it as a tuning knob
// fails here instead of shipping configuration that measures nothing.
func TestMmapSizeIsCompiledOut(t *testing.T) {
	ctx := t.Context()

	for _, p := range pragmas {
		if strings.HasPrefix(p, "mmap_size(") {
			t.Errorf("mmap_size is back in the pragma list as %q; it is inert under this "+
				"driver (MAX_MMAP_SIZE=0). Re-run `make bench-rss SPIKE_FLAGS='-mmap=...'` "+
				"and only re-add it with a number behind it. See ADR-0001.", p)
		}
	}

	d := openTestDB(t)
	conn, err := d.Read().Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Ask for 128 MiB the way the old pragma line did. A driver that grew mmap
	// support would read back what it was told, and that is the signal to
	// revisit the decision rather than a failure of this package.
	if _, err := conn.ExecContext(ctx, "PRAGMA mmap_size = 134217728"); err != nil {
		t.Fatalf("set mmap_size: %v", err)
	}
	got, err := PragmaInt(ctx, conn, "mmap_size")
	if err != nil {
		t.Fatalf("read mmap_size: %v", err)
	}
	if got != 0 {
		t.Errorf("mmap_size read back %d, want 0: this driver now supports mmap, so "+
			"ADR-0001's removal of the pragma is worth revisiting with a fresh measurement", got)
	}
}

// foreign_keys=ON is only worth setting if a violation actually fails. This is
// the end-to-end proof that the pragma reached the connection doing the write.
func TestForeignKeysAreEnforced(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO session (id, user_id, kind, created_at, last_seen_at,
			                     idle_expires_at, absolute_expires_at)
			VALUES ('sess', 999999, 'web', '2026-08-16 00:00:00', '2026-08-16 00:00:00',
			        '2026-08-19 00:00:00', '2026-09-15 00:00:00')`)
		return err
	})
	if err == nil {
		t.Fatal("inserted a session referencing a non-existent user; foreign_keys is not ON")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("error = %v, want a foreign key constraint failure", err)
	}
}

func TestWritePoolIsSingleConnection(t *testing.T) {
	d := openTestDB(t)
	if got := d.Writer().Stats().MaxOpenConnections; got != 1 {
		t.Errorf("write pool max connections = %d, want exactly 1", got)
	}
}

// TestReadPoolRefusesWrites makes the single-writer discipline structural rather
// than conventional.
//
// The read pool used to open read-WRITE, so "never start a write transaction on
// it" was a comment and nothing more. A stray write through DB.Read() therefore
// succeeded on a developer's idle machine and only misbehaved in production,
// where the deferred transaction it starts has to upgrade to a write lock — the
// one path busy_timeout does NOT cover (sync.md §6 rule 2), so it surfaces as an
// intermittent "database is locked" arbitrarily far from the statement at fault.
//
// mode=ro turns that into an immediate, deterministic failure at the offending
// line. Both entry points are covered: a bare statement, and an explicit
// transaction, because a caller can reach the pool either way.
func TestReadPoolRefusesWrites(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// Reads still work, so this is a read-only pool and not a broken one.
	var n int
	if err := d.Read().QueryRowContext(ctx, `SELECT count(*) FROM service_instance`).Scan(&n); err != nil {
		t.Fatalf("the read pool cannot read: %v", err)
	}

	const wantErr = "readonly"

	_, err := d.Read().ExecContext(ctx, `CREATE TABLE stray (id INTEGER PRIMARY KEY)`)
	if err == nil {
		t.Fatal("a DDL write through the read pool succeeded; the read pool is not mode=ro, " +
			"so a stray write races the real writer instead of failing at the line that wrote it")
	}
	if !strings.Contains(strings.ToLower(err.Error()), wantErr) {
		t.Errorf("read-pool write error = %v, want it to name a readonly database", err)
	}

	err = d.ReadTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO service_instance (kind, role, name, base_url, api_key_enc, kek_id)
			 VALUES ('prowlarr','indexer','x','http://x.test:9696',x'01',1)`)
		return err
	})
	if err == nil {
		t.Fatal("an INSERT inside a read-pool transaction succeeded")
	}
	if !strings.Contains(strings.ToLower(err.Error()), wantErr) {
		t.Errorf("read-pool transaction write error = %v, want it to name a readonly database", err)
	}

	// And the write pool is unaffected — the fix must not have made the process
	// read-only.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE allowed (id INTEGER PRIMARY KEY)`)
		return err
	}); err != nil {
		t.Fatalf("the write pool can no longer write: %v", err)
	}
}

// Write must roll back on error and commit on success, and a rollback must
// leave nothing behind.
func TestWriteTransaction(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	wantErr := errors.New("deliberate failure")
	err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag (namespace, value) VALUES ('test', 'rolled-back')`); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write = %v, want the callback's error", err)
	}

	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM tag WHERE value = 'rolled-back'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("found %d rows after a rolled-back transaction", n)
	}

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tag (namespace, value) VALUES ('test', 'committed')`)
		return err
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM tag WHERE value = 'committed'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("found %d committed rows, want 1", n)
	}
}

// The audit log is append-only, enforced by triggers rather than by convention:
// a caller that reaches for UPDATE or DELETE gets an abort, not a silent edit.
func TestAuditLogTriggersAbort(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO audit_log (action, result) VALUES ('test.action', 'ok')`)
		return err
	}); err != nil {
		t.Fatalf("insert audit row: %v", err)
	}

	for _, tc := range []struct{ name, stmt string }{
		{"update", `UPDATE audit_log SET result = 'tampered' WHERE action = 'test.action'`},
		{"delete", `DELETE FROM audit_log WHERE action = 'test.action'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, tc.stmt)
				return err
			})
			if err == nil {
				t.Fatalf("%s on audit_log succeeded; the trigger did not abort it", tc.name)
			}
			if !strings.Contains(err.Error(), "append-only") {
				t.Errorf("error = %v, want the trigger's append-only message", err)
			}
		})
	}
}

func TestPragmaIntRejectsInjection(t *testing.T) {
	d := openTestDB(t)
	for _, bad := range []string{"", "foreign_keys; DROP TABLE user", "cache_size)", "a b"} {
		if _, err := PragmaInt(t.Context(), d.Read(), bad); err == nil {
			t.Errorf("PragmaInt(%q) = nil error, want a rejection", bad)
		}
	}
}
