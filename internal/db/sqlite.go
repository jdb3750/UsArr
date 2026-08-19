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
	"github.com/ncruces/go-sqlite3/ext/fts5"
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
	// full result and the reasoning in docs/DECISIONS.md ADR-0001 correction 3
	// and its amendment.
	//
	// cache_size is PER-CONNECTION, so the process pays it once per pool
	// connection plus the writer — with a NumCPU*2 read pool it is a multiplier
	// on RSS, not a fixed cost, and it costs MORE on a machine with more cores.
	// Measured peak RSS on 4 cores: -2000 ~35 MB, -8000 ~85 MB, -32000 ~237 MB.
	// -8000 is the default because it buys most of the cache at a third of the
	// footprint of -32000, and the small self-hosted boxes this project targets
	// are what a default has to be defensible on. This is a MEMORY-side
	// decision only: the harness measures RSS, not query latency. Revisit it if
	// a latency benchmark ever contradicts it.
	//
	// Unmeasured on arm64. Page size and core count both move these numbers.
	"cache_size(-8000)", // ~7.8 MB per connection; see above

	// mmap_size is deliberately ABSENT. It was removed after measurement
	// because this driver compiles memory-mapped I/O out: PRAGMA
	// compile_options reports MAX_MMAP_SIZE=0 and DEFAULT_MMAP_SIZE=0, and
	// every requested value reads back as 0. It follows from the wasm32 target
	// — SQLite only defaults SQLITE_MAX_MMAP_SIZE non-zero on platforms it
	// knows have mmap — so it is structural to this build, not a toggle.
	// Inert configuration that looks meaningful is worse than none: a future
	// reader would tune it and measure nothing. Re-check with `make bench-rss`
	// if the driver ever ships an mmap-capable build, and add the line back
	// only with a number behind it.
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
//
// fts5.Register is passed to BOTH pools, and it is not optional. This driver
// does NOT ship FTS5 in its default build — measured, not assumed: on
// ncruces/go-sqlite3 v0.35.3 (SQLite 3.53.4) a bare connection answers
// `CREATE VIRTUAL TABLE … USING fts5(…)` with "no such module: fts5". FTS5 is a
// separately-registered wasm extension here, registration is PER-CONNECTION
// exactly as the pragmas are, and migration 0005 creates search_fts and
// search_trgm — so a pool opened without it cannot even migrate, let alone
// read. TestFTS5IsAvailableOnEveryConnection proves it across the whole read
// pool rather than trusting this comment.
func Open(ctx context.Context, path string) (*DB, error) {
	read, err := driver.Open(dsn(path, false), fts5.Register)
	if err != nil {
		return nil, fmt.Errorf("open read pool %s: %w", path, err)
	}
	readers := runtime.NumCPU() * 2
	read.SetMaxOpenConns(readers)
	read.SetMaxIdleConns(readers)

	write, err := driver.Open(dsn(path, true), fts5.Register)
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
	} else {
		// mode=ro makes the single-writer discipline structural instead of a
		// convention. Without it the read pool is a perfectly ordinary writable
		// connection, so a stray write through DB.Read() succeeds in
		// development, and in production races the real writer: the deferred
		// transaction it starts has to upgrade to a write lock, and that is the
		// one path busy_timeout does not cover (sync.md §6 rule 2). The symptom
		// is an intermittent "database is locked" under load, arbitrarily far
		// from the offending statement.
		//
		// With mode=ro the same statement fails immediately and every time,
		// with "attempt to write a readonly database", at the line that wrote
		// it. Open() pings the writer first, which creates the file and sets
		// journal_mode=WAL, so a read-only connection never has to create
		// anything. Migrations and every other write run on the write pool.
		q.Set("mode", "ro")
	}
	return "file:" + path + "?" + q.Encode()
}

// Read returns the read pool. It is opened mode=ro, so a write attempted on it
// fails with "attempt to write a readonly database" rather than silently
// succeeding and racing the writer. Use Write for anything that mutates.
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

// PlanHas reports whether a rendered query plan contains want AS A WHOLE TOKEN
// — the match must not be followed by another identifier character, so
// `ix_work_added` does not match a plan that names `ix_work_added_at`.
//
// WHY THIS IS NOT strings.Contains. A plan guard asserts on identifiers — index
// names, table names, aliases — against one long string. That is fine until an
// identifier is RENAMED to something that BEGINS with the old name:
// `ix_work_added` → `ix_work_added_at`, `sil` → `sil_w`, `sdl` → `sdl_f`. The
// old needle still matches the new plan, so the guard stays GREEN while pinning
// nothing at all, and there is nothing in the symptom to point at the cause,
// because a guard that passes by accident looks exactly like one that passes
// correctly. LS-379's fix hit precisely this: one plan guard went red and said
// so, two went on passing on `sil_w`.
//
// It is deliberately ONE-DIRECTIONAL. The needles it is written for always END
// with the identifier under test and begin with plan keywords (`SEARCH `,
// `USING INDEX `) or with the identifier itself, and a plan identifier is never
// preceded by another identifier character, so only the trailing edge can rot.
//
// It lives beside QueryPlan, in package code rather than in a _test.go file,
// for the same reason QueryPlan does: it is machinery for the gate's plan
// assertions, and BOTH internal/db's guards and internal/store's need it.
// internal/store imports internal/db, so one exported function here is reachable
// from both; a copy in each package is the thing this is avoiding, since a
// second implementation is a second thing to get wrong.
func PlanHas(plan, want string) bool {
	for i := 0; i+len(want) <= len(plan); {
		j := strings.Index(plan[i:], want)
		if j < 0 {
			return false
		}
		end := i + j + len(want)
		if end == len(plan) || !isPlanIdentByte(plan[end]) {
			return true
		}
		i += j + 1
	}
	return false
}

func isPlanIdentByte(b byte) bool {
	return b == '_' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}

// QueryPlan returns EXPLAIN QUERY PLAN output for a query, one line per step.
//
// This exists for the query-plan assertions docs/DEVELOPMENT.md §5 puts in the
// gate: plan strings are deterministic and hardware-independent, so an index
// regression is caught forever by ~30 lines of test that `make check` runs. Wall-clock budgets are not enforced here.
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
