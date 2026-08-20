package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// THE ARRIVALS CURSOR — channel 3b's durable state, and the ONE column in this
// schema whose stored instant belongs to an upstream's clock rather than to
// UsArr's.
//
// # A POSITION IS NOT A TIME, and that sentence is the whole file
//
// Every other timestamp column here is a CLAIM ABOUT FRESHNESS: it answers "how
// stale is the replica", it is UsArr's own clock, and it is rendered. This one
// answers "where did the last walk get to", it is BOOKORBIT'S clock, and it is
// never rendered. The two facts are separated because merging them puts an
// upstream clock reading behind a "28 minutes ago" sub-line computed against a
// browser's now — which puts the two machines' clock difference directly onto
// the one number whose entire job is to be trustworthy. The freshness instant a
// screen renders for a delta comes from the latest `delta_walk` sync_report
// row's created_at, and needs no column at all.
//
// # ⚠️ last_delta_sync_at — WHY 3b DELIBERATELY DOES NOT WRITE IT
//
// `service_instance.last_delta_sync_at` (migration 00001) is the seam this
// channel was expected to land in, and 3b does not use it. Stated at the seam
// rather than in a design document, on internal/store/libraries.go's precedent
// for `orphaned_at`:
//
//   - ⚠️ NOTHING WRITES IT AND NOTHING READS IT. Measured over the tree:
//     `last_delta_sync_at` and any Go spelling of it occur in migration 00001,
//     in the generated schema snapshot and in docs/reference/schema.md, and
//     nowhere in Go. It has no writer and no reader, test or otherwise.
//
//   - ⚠️ NO CHANNEL CLAIMS IT. docs/ARCHITECTURE.md §7.3 and
//     docs/reference/sync.md §3 specify a *reader* for it — channel 3, the *Arr
//     history walk, which computes `last_delta_sync_at − overlap` and expects
//     store.ParseTime's layout — but channel 3 is v0.2's and no *Arr adapter
//     exists. A specified reader is not a writer, so today the column is claimed
//     by a channel that is not built.
//
//   - 3b IS NOT THAT WRITER, and the reason is the heading above rather than
//     tidiness. This channel stores a CURSOR POSITION that deliberately lags the
//     maximum it observed (the walk's per-page mitigation advances to the
//     largest value strictly below the last row's), in an upstream's clock, in a
//     millisecond layout ParseTime refuses. `last_delta_sync_at` names a
//     freshness fact in UsArr's own second-resolution layout. Writing one into
//     the other would be silent in the worst way: services.go's three
//     `if t, err := ParseTime(...); err == nil` blocks SWALLOW the parse error,
//     so the first screen to render it would read `never` — no error, no log.
//
//   - ITS INTENDED USE IS UNSETTLED. This comment does not say who will
//     eventually write it, because the honest answer is that nobody has decided.
//     §7.3 makes it channel 3's, ADR-0035 §2a's Kavita watermark is a different
//     shape again, and the column may end up meaning neither. It is retained
//     because a column with a specified reader is a real seam and dropping it
//     would need a migration to put back; it is left alone because a third
//     confident story about its owner is exactly what produced the first two.
//
// # The type boundary, and what it is for
//
// ArrivalsWatermark is a struct rather than a time.Time or a string, and the
// accessors below take and return only it. So a caller CANNOT obtain a
// time.Time from the store for this column, which is what stops the next person
// reaching for FormatTime — the instinct this codebase actively teaches, since
// service_item_link.remote_updated_at already floors a wire-millisecond value to
// the second through exactly that call. See ArrivalsWatermarkLayout for what
// flooring would cost here.

// ArrivalsWatermarkLayout is the ONLY layout arrivals_watermark is ever written
// in: fixed-width milliseconds, applied after .UTC().
//
// ⚠️ THREE PROPERTIES, AND EACH ONE RULES OUT A TEMPTING ALTERNATIVE.
//
//  1. IT KEEPS MILLISECONDS. store.FormatTime's layout ("2006-01-02 15:04:05")
//     has no fractional-second field and FLOORS — measured: .001, .499, .500,
//     .999 and .999999 all render :33. The walk's filter is a strict `>`, so a
//     watermark floored to the second leaves every row in [W, W+1s) satisfying
//     `added_at > W` on EVERY poll until an arrival lands in a strictly later
//     second. That is the permanent duplicate ADR-0070 refused `>=` to avoid,
//     reintroduced where nobody would look for it because the operator is still
//     demonstrably `>`.
//
//  2. IT IS FIXED WIDTH, so values are lexicographically ordered and one instant
//     has one spelling. time.RFC3339Nano is not: it drops trailing zeros
//     (".980Z" → ".98Z", ".000Z" → no fraction at all), so it is variable width
//     and gives one instant two spellings.
//
//  3. THE ZONE FIELD IS `Z07:00` AND NOT A LITERAL `Z`. In Go's reference layout
//     a bare "Z" at the end is a LITERAL: the shorter, tempting
//     "2006-01-02T15:04:05.000Z" would stamp a Z onto a non-UTC instant and lie
//     about the zone. With Z07:00 a UTC value renders "Z" and anything else
//     renders "+02:00" — which is also why FormatArrivalsWatermark applies
//     .UTC() first, since a value formatted without it is not lexicographically
//     comparable with one that was.
const ArrivalsWatermarkLayout = "2006-01-02T15:04:05.000Z07:00"

// ArrivalsWatermark is one instance's arrivals cursor position, or the absence
// of one.
//
// ⚠️ IT IS OPAQUE TO THE IMPORTER BY CONSTRUCTION, NOT BY CONVENTION. Value is
// upstream's own instant rendered in ArrivalsWatermarkLayout; nothing here
// parses it, and the pipeline hands it to an adapter without looking inside. The
// adapter that MINTED the format is the only code that reads it back, subtracts
// a lookbehind from it and formats it again — because that adapter is the only
// code that knows whose clock it is in.
//
// Valid false is a real, ordinary state with three distinct causes the delta's
// preconditions read differently: never fully imported; the last full import
// declined to mint; and the last full import measured the catalogue empty.
//
// ⚠️ IT IS NOT THE CURSOR. The cursor is the filter value of the NEXT request:
// a local variable inside one walk, discarded when the walk ends, and under the
// tie mitigation deliberately one tie-group BEHIND what the walk actually saw.
// Persisting the cursor here is the obvious "resume where we left off" instinct
// and it breaks two things at once — a mid-walk failure would leave a stored
// value claiming progress the run's other phases never made, and the stored
// value would be permanently short of the maximum observed, so the same group is
// re-read forever without anyone being able to see that it is wrong. The two are
// separate vocabularies (docs/DEVELOPMENT.md §11) and they never share a term.
type ArrivalsWatermark struct {
	// Value is the rendered instant. It is meaningful only when Valid.
	Value string

	// Valid is false for SQL NULL, i.e. no cursor position is stored.
	Valid bool
}

// FormatArrivalsWatermark renders an observed upstream instant as a watermark.
//
// A ZERO TIME IS AN INVALID WATERMARK AND NEVER A FORMATTED ONE. The adapter's
// timestamp parser returns the zero time for a value it cannot read, so a zero
// arriving here means "no maximum was observed" — and formatting it would write
// year 1 into the column and then filter every future walk against it, which
// would deliver the entire catalogue on every poll rather than nothing.
func FormatArrivalsWatermark(t time.Time) ArrivalsWatermark {
	if t.IsZero() {
		return ArrivalsWatermark{}
	}
	return ArrivalsWatermark{Value: t.UTC().Format(ArrivalsWatermarkLayout), Valid: true}
}

// Time parses a watermark back into the instant it was minted from.
//
// It is on the TYPE rather than on the Store deliberately: the store surface
// hands out no time.Time for this column at all (see the file header), and the
// adapter that needs one for its lookbehind arithmetic asks the value it was
// given, not the database.
func (w ArrivalsWatermark) Time() (time.Time, error) {
	if !w.Valid {
		return time.Time{}, errors.New("store: arrivals watermark is not set")
	}
	t, err := time.Parse(ArrivalsWatermarkLayout, w.Value)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse arrivals watermark %q: %w", w.Value, err)
	}
	return t, nil
}

// ReadArrivalsWatermark reports one instance's arrivals cursor position, or an
// invalid ArrivalsWatermark for "no walk has ever completed one".
//
// It takes no Scope, and that is a deliberate placement rather than an oversight
// — the same paragraph LastFullSyncAt owes and for the same reason, restated
// here rather than cross-referenced because the two functions can be read apart.
// This is not a user-facing read. It reads ONE NAMED INSTANCE's control-plane
// column so the replication worker can decide whether it may run a delta at all,
// exactly as CountEncryptedCredentials reads a control-plane count for the
// key-rotation path. The rule store.go states is about reads that AGGREGATE
// ACROSS INSTANCES, which is where an existence oracle lives. There is no
// user-facing read of this column to carry a Scope: nothing renders it, it is
// absent from serviceInstanceColumns, and it never reaches internal/httpapi.
func (s *Store) ReadArrivalsWatermark(ctx context.Context, instanceID int64) (ArrivalsWatermark, error) {
	var v sql.NullString
	err := s.db.Read().QueryRowContext(ctx,
		`SELECT arrivals_watermark FROM service_instance WHERE id = ? AND deleted_at IS NULL`,
		instanceID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return ArrivalsWatermark{}, fmt.Errorf("service_instance %d not found", instanceID)
	}
	if err != nil {
		return ArrivalsWatermark{}, fmt.Errorf("read arrivals_watermark for service_instance %d: %w", instanceID, err)
	}
	return ArrivalsWatermark{Value: v.String, Valid: v.Valid}, nil
}

// StampArrivalsWatermark advances one instance's arrivals cursor position.
//
// ⚠️ AN INVALID WATERMARK IS REFUSED RATHER THAN WRITTEN, AND THAT REFUSAL IS
// THE POINT OF THE FUNCTION. Writing an invalid one would NULL the column, and
// the run that most wants to call this function with an invalid value is the
// ordinary one: a completed delta that observed no new arrivals, which is the
// MODAL outcome of an arrivals channel. A quiet BookOrbit would then destroy its
// own cursor and re-read from nothing — intermittently, because a run inside the
// lookbehind window usually does re-observe something, and on exactly the quiet
// instances nobody is watching. A completed run that observed nothing advances
// nothing and leaves the stored value BYTE-FOR-BYTE unchanged; that is not an
// error and not a failure.
//
// ON A WHOLLY SUCCESSFUL RUN ONLY, and the caller is what enforces that, exactly
// as it enforces StampFullSync's. This is called at most once per run, after
// every phase has succeeded for every library. A library that failed, a walk
// that wedged on a tie group, and any row whose arrival stamp could not be read
// each leave the column untouched with a reason recorded in the run's report.
//
// It is a replication write and takes no Scope; see ReadArrivalsWatermark.
func (s *Store) StampArrivalsWatermark(ctx context.Context, instanceID int64, w ArrivalsWatermark) error {
	if !w.Valid {
		return fmt.Errorf("stamp arrivals_watermark on service_instance %d: %w", instanceID, ErrNoWatermarkToStamp)
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET arrivals_watermark = ? WHERE id = ? AND deleted_at IS NULL`,
			w.Value, instanceID)
		if err != nil {
			return fmt.Errorf("stamp arrivals_watermark on service_instance %d: %w", instanceID, err)
		}
		return expectOneRow(res, "service_instance", instanceID)
	})
}

// ErrNoWatermarkToStamp is what StampArrivalsWatermark refuses an invalid
// watermark with.
//
// It is a sentinel rather than a plain error so a caller that reaches this by
// accident FAILS ITS RUN rather than logging a line: the only correct response
// to "there is no maximum to store" is not to call this function.
var ErrNoWatermarkToStamp = errors.New("store: refusing to null arrivals_watermark; a run that observed no maximum advances nothing")
