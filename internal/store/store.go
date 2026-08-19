// Package store is the data-access layer over internal/db for the tables in
// migration 0001.
//
// Two rules run through every function here, both from ARCHITECTURE.md §1.3,
// and both are cheap now and expensive to retrofit:
//
//  1. Every user-scoped row carries user_id, from the first migration.
//  2. Every read that aggregates across instances takes an access-scope
//     parameter IN THE QUERY SIGNATURE — not bolted on later. v0.1 has one
//     owner, so callers pass OwnerScope and nothing is filtered out; the
//     parameter exists so multi-user is a behaviour change rather than a
//     redesign. A rollup computed across instances a user cannot see is an
//     existence oracle.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// SystemUserID is the reserved shared/system sentinel row created by migration
// 0001. Shared tag assignments carry it, and the canonical tag filter predicate
// is `user_id IN (0, :uid)`.
const SystemUserID int64 = 0

// timeLayout is SQLite's own datetime() format.
//
// docs/reference/schema.md calls timestamps "ISO-8601 UTC text" but every
// column default in the same file is `datetime('now')`, which produces
// "2006-01-02 15:04:05" with no T and no Z. Go-written timestamps must match
// the defaults byte-for-byte: these columns are compared and ordered
// lexicographically (ix_audit_ts, ix_rel_expiry, ix_wq_runnable), and two
// formats in one column silently breaks both. Correct ordering beats the
// prettier format; resolving the two is a docs question, and changing it later
// is a migration that rewrites every timestamp column.
const timeLayout = "2006-01-02 15:04:05"

// FormatTime renders a timestamp for storage.
func FormatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// ParseTime reads a stored timestamp.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse timestamp %q: %w", s, err)
	}
	return t, nil
}

// Scope is the access scope a read is evaluated under.
//
// It is a value, not an interface, because there is exactly one implementation
// today and a speculative interface would be a guess about a shape nobody has
// needed yet.
type Scope struct {
	// UserID is the acting user. Reads of user-scoped rows match
	// `user_id IN (SystemUserID, UserID)`.
	UserID int64

	// AllInstances grants visibility of every service instance. It is true for
	// the owner, which in v0.1 is everyone.
	AllInstances bool

	// InstanceIDs is the visible set when AllInstances is false. An empty slice
	// with AllInstances false means the user can see nothing, and the queries
	// below return no rows rather than every row — failing closed is the whole
	// point of carrying the parameter.
	InstanceIDs []int64
}

// OwnerScope is the full scope of the single v0.1 owner.
func OwnerScope(userID int64) Scope {
	return Scope{UserID: userID, AllInstances: true}
}

// instancePredicate renders the scope as a SQL fragment plus its arguments.
// column is the qualified service_instance id column to constrain.
func (s Scope) instancePredicate(column string) (string, []any) {
	if s.AllInstances {
		return "1=1", nil
	}
	if len(s.InstanceIDs) == 0 {
		// Fail closed. "No visible instances" must mean no rows, never all rows.
		return "1=0", nil
	}
	args := make([]any, 0, len(s.InstanceIDs))
	for _, id := range s.InstanceIDs {
		args = append(args, id)
	}
	return column + " IN (" + placeholders(len(args)) + ")", args
}

// userPredicate renders the scope's user filter plus its arguments. column is
// the qualified user_id column to constrain.
//
// The predicate is `user_id IN (0, :uid)` — the canonical one, the same shape
// tag_assignment uses. 0 is the shared/system sentinel: for provenance it is
// what rows written before per-user attribution existed carry (migration 0002
// backfills to it), and for a row written by a path with no acting user it is
// what that row means. Reading it as "not mine" would hide the owner's own
// history from them.
//
// A NOTE ON ORDER. SQLite cannot supply ORDER BY from an index whose LEADING
// column is constrained by IN — it produces `SEARCH … USING INDEX (user_id=?)`
// plus `USE TEMP B-TREE FOR ORDER BY`, verified in internal/db's query-plan
// test, which pins both forms. Equality on a single user does come out ordered.
// So a newest-first read over provenance sorts a bounded, already-index-filtered
// set rather than the table; if that ever stops being cheap, the fix is to
// attribute the backfilled rows to a real user in a later migration, not to drop
// the sentinel from the predicate.
func (s Scope) userPredicate(column string) (string, []any) {
	return column + " IN (?, ?)", []any{SystemUserID, s.UserID}
}

// derivedInnerAlias names the inner table of a CORRELATED subquery inside a
// predicate, derived from the outer column that subquery correlates to. It is
// the shared construction behind scopeLinkAlias, librarySourceAlias and
// searchDocLibraryAlias, and the full argument for it is on scopeLinkAlias.
//
// In one line: a correlated subquery's only tie to the caller's row is
// `<inner>.x = <outer>.y`, SQL resolves a qualifier to the innermost scope that
// offers it, so an inner alias equal to the outer qualifier decorrelates the
// subquery SILENTLY — no error, no syntax fault, just a scope that stops
// filtering. Deriving the inner alias from the outer qualifier makes the
// collision UNREPRESENTABLE: the result is that qualifier plus a prefix, so it
// is strictly longer than the qualifier it must never equal, and the
// construction has no fixed point.
//
// ⚠️ prefix MUST BE A NON-EMPTY CONSTANT — it is what makes the result strictly
// longer, and therefore it is the whole proof. It names the inner TABLE (`ls`
// for library_source, `sdl` for search_doc_library) rather than being chosen by
// the caller, so a query plan still reads as the table it is about. It is not a
// caller-supplied alias, and must never become one: a required alias argument
// prevents omission, not misuse. See docs/REVIEW-LOG.md LS-379.
func derivedInnerAlias(prefix, column string) string {
	outer, _, _ := strings.Cut(column, ".")
	return prefix + "_" + outer
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// Store reads and writes UsArr's tables.
type Store struct {
	db *db.DB
}

// New wraps an open database.
func New(d *db.DB) *Store { return &Store{db: d} }

// DB exposes the underlying pools for callers that need a transaction spanning
// more than one store method.
func (s *Store) DB() *db.DB { return s.db }

// write runs fn inside a BEGIN IMMEDIATE transaction on the single writer.
func (s *Store) write(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return s.db.Write(ctx, fn)
}

// querier is the read surface *sql.DB and *sql.Tx have in common.
//
// It exists so one read function serves both callers: an ordinary read straight
// off the pool, and a read that has to share ONE WAL snapshot with its
// neighbours. Two statements issued separately on the pool are two snapshots,
// and a row written between them is visible to the second and not the first —
// which is how a caller that reads a parent row and then its children renders a
// state neither the old nor the new database ever held. See ReadIndexerCatalog.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
