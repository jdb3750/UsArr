// Package db opens and migrates UsArr's SQLite database.
//
// Two pools, because SQLite has one writer:
//
//	reads   NumCPU*2 connections
//	writes  exactly one connection
//
// One writer eliminates SQLITE_BUSY arising from concurrent writers inside the
// process. It does not eliminate SQLITE_BUSY — VACUUM INTO, ANALYZE, WAL
// checkpoint starvation and a second process holding the file all remain, and
// claiming otherwise would be believed. See docs/reference/sync.md §6.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	// The driver pulls in the wasm2go-translated SQLite build itself; importing
	// go-sqlite3/embed alongside it is redundant and the package says so.
	"github.com/ncruces/go-sqlite3/driver"
)

// pragmas are set on EVERY connection in both pools.
//
// foreign_keys is the one that bites: it is off by default in SQLite and it is
// PER-CONNECTION, so setting it once at startup leaves every other pooled
// connection running without referential integrity. The driver applies these
// from the DSN on each new connection, and TestPragmasOnEveryConnection proves
// it across the whole read pool rather than trusting the claim.
//
// Order matters to the driver: busy timeout first.
var pragmas = []string{
	"busy_timeout(5000)",
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"foreign_keys(ON)",
	"temp_store(MEMORY)",
	"wal_autocheckpoint(1000)",

	// MEASURED 2026-08-16 on x86-64 by `make bench-rss` (internal/db/spike);
	// full result in docs/DECISIONS.md ADR-0001 correction 3. Both values were
	// previously marked unverified; neither is unverified now, and neither has
	// been changed on the strength of the measurement — that is an owner
	// decision, recorded rather than taken here.
	//
	//   cache_size is PER-CONNECTION. The process pays it once per pool
	//   connection plus the writer, so with NumCPU*2 readers -32000 measured
	//   ~235 MB peak RSS on 4 cores against ~35 MB at -2000. It costs more on a
	//   machine with more cores, not less.
	//
	//   mmap_size does NOTHING here. Every value reads back as 0 and PRAGMA
	//   compile_options reports MAX_MMAP_SIZE=0 — memory-mapped I/O is compiled
	//   out of this driver's SQLite. The line below is inert; drop it next time
	//   this list is touched.
	//
	// Unmeasured on arm64. Page size and core count both move these numbers.
	"cache_size(-32000)",   // 32 MB per connection
	"mmap_size(134217728)", // inert; see above
}

// DB holds the read and write pools.
type DB struct {
	read  *sql.DB
	write *sql.DB
	path  string
}

// Open opens the database at path, applies the pragmas to every connection and
// runs any pending migrations.
//
// The write pool is capped at one connection and opened with
// _txlock=immediate, so every transaction it starts is BEGIN IMMEDIATE.
func Open(ctx context.Context, path string) (*DB, error) {
	read, err := driver.Open(dsn(path, false))
	if err != nil {
		return nil, fmt.Errorf("open read pool %s: %w", path, err)
	}
	readers := runtime.NumCPU() * 2
	read.SetMaxOpenConns(readers)
	read.SetMaxIdleConns(readers)

	write, err := driver.Open(dsn(path, true))
	if err != nil {
		_ = read.Close()
		return nil, fmt.Errorf("open write pool %s: %w", path, err)
	}
	// Exactly one. This is the entire single-writer discipline.
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)

	d := &DB{read: read, write: write, path: path}

	// Ping the writer first: it creates the file and sets journal_mode=WAL,
	// which is a database-level property. Readers opening concurrently against
	// a not-yet-WAL database would race on that.
	if err := write.PingContext(ctx); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := read.PingContext(ctx); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	if err := d.Migrate(ctx); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

// dsn builds the connection string. Pragmas are repeated _pragma= parameters,
// which the driver applies on every new connection.
func dsn(path string, writer bool) string {
	q := make(url.Values, len(pragmas)+2)
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	// Store time.Time in SQLite's own datetime() format, matching the DDL
	// column defaults. Mixing RFC 3339 and SQLite datetime() in one TEXT column
	// breaks lexicographic ORDER BY, which is how every timestamp index here is
	// read.
	q.Set("_timefmt", "sqlite")
	if writer {
		// Rule 2, sync.md §6: busy_timeout does not rescue a deferred read
		// transaction that upgrades to a write — that path returns
		// SQLITE_BUSY immediately.
		q.Set("_txlock", "immediate")
	}
	return "file:" + path + "?" + q.Encode()
}

// Read returns the read pool. Never start a write transaction on it.
func (d *DB) Read() *sql.DB { return d.read }

// Writer returns the single-connection write pool. Prefer Write, which
// guarantees the BEGIN IMMEDIATE / rollback discipline.
func (d *DB) Writer() *sql.DB { return d.write }

// Path is the database file path.
func (d *DB) Path() string { return d.path }

// Close closes both pools.
func (d *DB) Close() error {
	var errs []error
	if d.read != nil {
		errs = append(errs, d.read.Close())
	}
	if d.write != nil {
		errs = append(errs, d.write.Close())
	}
	return errors.Join(errs...)
}

// Write runs fn inside a BEGIN IMMEDIATE transaction on the single writer.
//
// The write lock is taken at BEGIN rather than at the first write, so a
// conflicting writer is reported by busy_timeout's wait rather than by an
// immediate SQLITE_BUSY halfway through the transaction.
//
// fn must not call Write, and must not hold the transaction across a network
// call: the whole process shares one writer connection.
func (d *DB) Write(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}
	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ReadTx runs fn inside a read-only transaction on the read pool, so a
// multi-statement read sees one consistent snapshot.
//
// Keep these short. A long-lived read transaction starves the WAL checkpointer:
// wal_autocheckpoint then silently fails, the WAL grows unbounded, and an
// explicit truncate returns SQLITE_BUSY. Never hold one across an SSE send.
func (d *DB) ReadTx(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	tx, err := d.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin read: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // read-only; a rollback error tells us nothing
	return fn(ctx, tx)
}

// PragmaInt reads an integer pragma from a specific connection. Tests use it to
// prove the per-connection pragmas really are per-connection.
func PragmaInt(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, name string,
) (int64, error) {
	// Pragma names are not bindable parameters, so this is a string
	// concatenation by necessity. Reject anything that is not a bare
	// identifier rather than trusting every caller.
	if !isIdentifier(name) {
		return 0, fmt.Errorf("db: %q is not a pragma name", name)
	}
	var v int64
	if err := q.QueryRowContext(ctx, "PRAGMA "+name).Scan(&v); err != nil {
		return 0, fmt.Errorf("read pragma %s: %w", name, err)
	}
	return v, nil
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// QueryPlan returns EXPLAIN QUERY PLAN output for a query, one line per step.
//
// This exists for the CI assertions in docs/DEVELOPMENT.md §5: plan strings are
// deterministic and hardware-independent, so an index regression is caught
// forever by ~30 lines of test. Wall-clock budgets are not enforced here.
func QueryPlan(ctx context.Context, q *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		return nil, fmt.Errorf("explain query plan: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			return nil, fmt.Errorf("explain query plan: scan: %w", err)
		}
		out = append(out, strings.TrimSpace(detail))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("explain query plan: %w", err)
	}
	return out, nil
}
