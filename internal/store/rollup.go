package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// The rollup flush: how media_file rows become work.have_count and the
// `availability` blob, and the one rule that keeps the blob honest.
//
// # Dirty-mark plus a flush, never per child write
//
// ARCHITECTURE.md §6.3 and reference/sync.md both say it in the same words:
// "a child write marks its ancestor dirty in the same transaction, and ancestors
// are re-aggregated once per 250 ms flush batch … Never re-aggregate per child
// write — a series has up to ~300 children and an import generates ~400k child
// events". internal/store/files.go sets the flag; this file is the flush that
// reads it. Nothing in the tree re-aggregates inside a child write, and the
// partial index ix_work_dirty is what makes picking up the batch a seek.
//
// ⚠️ THERE IS NO 250 ms TIMER, and the number in those documents describes a
// cadence this milestone does not have. internal/libsync/doc.go lists "any
// timer" among the things deliberately not built: an import is called on connect
// or on demand, so the flush runs at the end of one. The cadence belongs to the
// interactive write path — §7.6's verbs, which have no target in v0.1 — and the
// seam for it is FlushRollups itself, which is already batch-bounded and takes
// no import context.
//
// # THE RULE THAT MATTERS: never aggregate over zero children
//
// 🚩 A work with no file rows gets NO blob (REVIEW-LOG.md LS-211.2 rule 2). Not
// `have: 0` — no key at all. The failure it prevents is specific and was
// reasoned out before this code existed: the front end takes the
// `availability.k === 'count'` arm and renders `0 / N · N missing` on a healthy
// library, which is a denominator with no numerator arriving through a column
// that now LOOKS computed. That is worse than the blank column it replaced,
// because a blank column visibly says nothing while a computed-looking zero
// asserts emptiness.
//
// IT IS ENFORCED HERE RATHER THAN BY SEQUENCING, and LS-211.2 says why in as
// many words: "Sequencing is a property of a plan and plans slip; a writer that
// will not aggregate over zero children is a property of the code." A work whose
// children have not been walked YET is the steady state during every subsequent
// sync, not a one-off first-run condition.
//
// # The absence contract this must not break
//
// reference/http-api.md §1.4.1: an ABSENT `availability` key means NO COUNT HAS
// EVER BEEN COMPUTED, and a consumer must not render it as zero, as "none", or
// as any glyph asserting emptiness. Two consequences for this writer:
//
//  1. It never moves have_count without also writing the blob. §1.4.1 rests on
//     that ("there is no specified path that moves have_count without also
//     writing the blob"), so a work this flush cannot produce a blob for is left
//     with BOTH columns untouched — see rollupKindUnsupported below.
//  2. When a work's basis goes away — every file deleted upstream, so the walk
//     reconciled its rows to nothing — the blob is WITHDRAWN rather than
//     rewritten as `have: 0`. Withdrawing returns the row to "not counted",
//     which is the honest state when there is nothing left to count: with no
//     file rows, "never walked" and "walked, and empty" are indistinguishable
//     from here, and only one of them may be rendered as emptiness.

// Work statuses this package recognises, and the three that unlock a
// denominator.
//
// THE VOCABULARY IS DEFINED HERE AND PRODUCED BY THE ADAPTERS, on
// work_credit.role's pattern: internal/libsync maps Kavita's integer
// PublicationStatus onto these strings, and the gate below is the only reader.
// work.status carries no CHECK, so a constant that no adapter emits is dead
// rather than rejected — which is why the adapter references these constants
// instead of spelling the strings again.
const (
	WorkStatusOngoing   = "ongoing"
	WorkStatusHiatus    = "hiatus"
	WorkStatusCompleted = "completed"
	WorkStatusCancelled = "cancelled"
	WorkStatusEnded     = "ended"
)

// statusAllowsATotal is ARCHITECTURE.md §6.1's gate, verbatim: show a fraction
// "only when the series status is ENDED/Completed/Cancelled AND a total is
// declared", and otherwise show the count with no denominator.
//
// The reason is not caution, it is that every total in this domain is a
// DECLARATION: Komga's totalBookCount and Kavita's totalCount both derive from
// ComicInfo's `Count`, whose own specification concedes it "could be different
// on each book in a series". An ongoing series' declared total is a plan, not a
// denominator, and rendering `43 / 60` against it invents a completeness claim.
func statusAllowsATotal(status string) bool {
	switch status {
	case WorkStatusCompleted, WorkStatusCancelled, WorkStatusEnded:
		return true
	}
	return false
}

// availabilityCount is schema.md §1's k="count" blob.
//
// THE FIELD ORDER IS THE WIRE ORDER AND `k` IS FIRST, which schema.md requires
// ("The blob carries a `"k"` discriminator as its first key and a renderer
// switches on it"). encoding/json emits struct fields in declaration order, so
// the order here IS the guarantee — a map would emit them sorted and put `have`
// first.
//
// Total and TotalSource are POINTERS WITHOUT omitempty, so an undeclared total
// marshals as an explicit `"total":null`. That is not cosmetic: schema.md and
// http-api.md §1.4 both state that `total: null` is not `total: 0` — the first
// means nobody honestly knows, the second means the series is empty — and §6.3's
// render rule (`have == total && total > 0` → ✓) must never fire on the first.
// Omitting the key would leave a reader to guess which of the two it was.
//
// ⚠️ THERE IS NO `missing` KEY, and its absence is a consequence rather than an
// oversight. `missing` is contiguity — "43 issues · #7, #12 and #30-32 missing"
// — computed locally from work_comic_issue.number_sort, and nothing in the tree
// writes work_comic_issue (internal/libsync/files.go's header says why). It is
// the one always-honest completeness number in the domain, and it is owed by the
// slice that lands the issue level.
type availabilityCount struct {
	K           string  `json:"k"`
	Have        int64   `json:"have"`
	Total       *int64  `json:"total"`
	TotalSource *string `json:"total_source"`
}

// RollupResult is what one FlushRollups call did.
type RollupResult struct {
	// WorksFlushed counts works whose dirty flag was cleared, whatever else
	// happened to them.
	WorksFlushed int

	// BlobsWritten counts works that got a k="count" availability blob.
	BlobsWritten int

	// BlobsWithheld counts works the LS-211.2 rule refused to write a blob for
	// because they had no file rows in scope. It is NOT an error count: on a
	// steady-state library it is the number of works whose files have not been
	// walked yet, which during a first import is most of them.
	BlobsWithheld int

	// TotalsOffered counts blobs that carry a non-null `total`. The gap between
	// this and BlobsWritten is the honest one: it is how many series have no
	// denominator anybody can stand behind, which on a comics library is most
	// of them.
	TotalsOffered int

	// KindsUnsupported counts dirty works whose kind has no blob shape in this
	// writer — video is k="tier" and music is k="edition", and neither has a
	// source that writes files yet. Their have_count is left ALONE rather than
	// written without a blob, because http-api.md §1.4.1 rests on there being no
	// path that moves one without the other.
	KindsUnsupported int
}

func (r *RollupResult) add(o RollupResult) {
	r.WorksFlushed += o.WorksFlushed
	r.BlobsWritten += o.BlobsWritten
	r.BlobsWithheld += o.BlobsWithheld
	r.TotalsOffered += o.TotalsOffered
	r.KindsUnsupported += o.KindsUnsupported
}

// Add merges another result into r.
func (r *RollupResult) Add(o RollupResult) { r.add(o) }

// DefaultRollupBatch is how many dirty works one flush transaction takes.
//
// It bounds the transaction rather than the work: the flush shares one writer
// connection with every interactive write (reference/sync.md §6 rule 3), so the
// thing to keep short is the transaction, and FlushRollups simply repeats until
// the dirty set is empty.
const DefaultRollupBatch = 500

// dirtyWorksSQL is the statement that picks up a flush batch.
//
// It is a function so the query-plan guard can EXPLAIN THE STATEMENT THAT SHIPS
// rather than a hand-copied lookalike (docs/DEVELOPMENT.md §11 rule 1). The
// index it must use is ix_work_dirty, the partial index migration 0005 created
// for exactly this read; internal/db's own plan test asserts the same index
// against a simpler spelling, and that test had never had a writer to be
// green about.
func dirtyWorksSQL() string {
	return `SELECT id, kind FROM work WHERE rollup_dirty = 1 ORDER BY id LIMIT ?`
}

// FlushRollups re-aggregates every dirty work and clears the flag.
//
// # Scope is in the signature from day one, and here is what it degenerates to
//
// docs/reference/security.md: an availability figure computed across instances
// the viewer cannot see is an EXISTENCE ORACLE. So the numerator is counted over
// the media_file rows the scope can see, through the same three branches
// Scope.instancePredicate has — AllInstances is `1=1`, an empty visible set
// FAILS CLOSED at `1=0`, and otherwise it is an IN over the visible instances.
// (recent.go needs an EXISTS for the same job because `work` carries no instance
// column; media_file does, so the predicate sits directly on the row.)
//
// ⚠️ A DENORMALISED COLUMN IS SCOPE-FREE BY CONSTRUCTION, and pretending
// otherwise would be the real bug. work.have_count and work.availability hold
// ONE answer, so they hold the answer for ONE scope — whichever scope last
// flushed them. v0.1's degenerate case is that there is exactly one such scope:
// the single owner's, OwnerScope, whose predicate is `1=1`, so the stored
// columns and the true answer coincide for the only user who exists.
//
// THE SEAM FOR MULTI-SCOPE IS THIS SIGNATURE PLUS THAT FACT, not a second
// column. When a second user lands, the choice is between (a) keeping these
// columns as the OWNER's answer and computing a narrower user's figure at read
// time from media_file — the read is already indexed by ix_file_work — or
// (b) a per-scope rollup table. Both start from a flush that already knows what
// scope it computed, which is what this parameter buys and what adding it later
// would have cost.
func (s *Store) FlushRollups(ctx context.Context, scope Scope, batch int) (RollupResult, error) {
	if batch <= 0 {
		batch = DefaultRollupBatch
	}
	var res RollupResult
	for {
		one, n, err := s.flushRollupBatch(ctx, scope, batch)
		res.add(one)
		if err != nil {
			return res, err
		}
		if n == 0 {
			return res, nil
		}
	}
}

// flushRollupBatch does one transaction's worth and reports how many dirty works
// it picked up.
func (s *Store) flushRollupBatch(
	ctx context.Context, scope Scope, batch int,
) (RollupResult, int, error) {
	var res RollupResult
	var picked int

	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res = RollupResult{}
		picked = 0

		type dirtyWork struct {
			id   int64
			kind string
		}
		var works []dirtyWork
		rows, err := tx.QueryContext(ctx, dirtyWorksSQL(), batch)
		if err != nil {
			return fmt.Errorf("read the dirty set: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var w dirtyWork
			if err := rows.Scan(&w.id, &w.kind); err != nil {
				return fmt.Errorf("read the dirty set: %w", err)
			}
			works = append(works, w)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read the dirty set: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("read the dirty set: %w", err)
		}
		picked = len(works)

		for _, w := range works {
			one, err := flushOneWork(ctx, tx, scope, w.id, w.kind)
			if err != nil {
				return fmt.Errorf("flush rollup of work %d: %w", w.id, err)
			}
			res.add(one)
			// CLEARED WHATEVER HAPPENED, including for a kind this writer has
			// no shape for. A flag left set on a work the flush cannot help is
			// a work every later flush picks up again, forever.
			if _, err := tx.ExecContext(ctx,
				`UPDATE work SET rollup_dirty = 0 WHERE id = ?`, w.id); err != nil {
				return fmt.Errorf("clear the dirty flag on work %d: %w", w.id, err)
			}
			res.WorksFlushed++
		}
		return nil
	})
	return res, picked, err
}

// rollupKindHasCountShape reports whether a work's kind is one this writer can
// produce a blob for.
//
// v0.1's catalogue source is Kavita, so the kinds with files are 'comic' and
// 'book' — both of them k="count" (schema.md §1: "comics, and anything else with
// no honest denominator"). Video is k="tier" and music is k="edition", and
// neither has a source that writes a media_file row yet, so a dirty work of
// those kinds is not reachable today; the switch has an arm for them anyway
// because "unreachable" is a fact about the current tree and not about the
// column.
func rollupKindHasCountShape(kind string) bool {
	switch kind {
	case "book", "comic", "comic_issue":
		return true
	}
	return false
}

func flushOneWork(
	ctx context.Context, tx *sql.Tx, scope Scope, workID int64, kind string,
) (RollupResult, error) {
	var res RollupResult

	if !rollupKindHasCountShape(kind) {
		// NEITHER COLUMN IS TOUCHED. Writing have_count here and withholding
		// the blob would break http-api.md §1.4.1's premise — that no path
		// moves the count without writing the blob — and a count under an
		// absent key is exactly the state §1.4.1 tells consumers cannot happen.
		res.KindsUnsupported++
		return res, nil
	}

	// ── 1. The numerator, scoped.
	pred, args := scope.instancePredicate("f.service_instance_id")
	q := `SELECT COUNT(*) FROM media_file f WHERE f.work_id = ? AND ` + pred
	var files int64
	if err := tx.QueryRowContext(ctx, q, append([]any{workID}, args...)...).Scan(&files); err != nil {
		return res, fmt.Errorf("count files: %w", err)
	}

	// `have` and the BASIS are the same number today and they are two variables
	// on purpose. have is "how many of the thing does the user hold"; the basis
	// is "is there anything to count at all", which is what LS-211.2's rule
	// keys on. They diverge the moment work_comic_issue lands and `have` counts
	// ISSUES HELD rather than files — at which point a work can honestly hold 0
	// issues while having file rows, and the refusal must still key on the
	// basis. Collapsing them into one variable now is what would make that a
	// rewrite instead of an edit.
	have := files
	basis := files

	if basis == 0 {
		// THE REFUSAL. See the file header: no blob, and the previous one is
		// WITHDRAWN rather than left standing or rewritten as have: 0.
		//
		// have_count goes to 0 alongside it, which does not violate §1.4.1's
		// premise: that paragraph's own words are "have_count: 0 is not
		// evidence of anything on its own", and 0 is also the column default,
		// so the pair (0, absent) is precisely the "never counted" state a
		// consumer must render as not-counted.
		if _, err := tx.ExecContext(ctx,
			`UPDATE work SET have_count = 0, availability = NULL
			  WHERE id = ? AND (have_count <> 0 OR availability IS NOT NULL)`,
			workID); err != nil {
			return res, fmt.Errorf("withdraw the blob: %w", err)
		}
		res.BlobsWithheld++
		return res, nil
	}

	// ── 2. The denominator, which is usually absent and says so.
	blob := availabilityCount{K: "count", Have: have}
	total, source, err := declaredTotal(ctx, tx, workID)
	if err != nil {
		return res, err
	}
	if total.Valid {
		t := total.Int64
		blob.Total = &t
		if source.Valid {
			src := source.String
			blob.TotalSource = &src
		}
		res.TotalsOffered++
	}

	encoded, err := json.Marshal(blob)
	if err != nil {
		return res, fmt.Errorf("encode the availability blob: %w", err)
	}

	// ── 3. One UPDATE for both, because they are one statement about the work
	// and a reader that saw one without the other would see a state the rollup
	// never computed.
	if _, err := tx.ExecContext(ctx,
		`UPDATE work SET have_count = ?, availability = ? WHERE id = ?`,
		have, string(encoded), workID); err != nil {
		return res, fmt.Errorf("write the rollup: %w", err)
	}
	res.BlobsWritten++
	return res, nil
}

// declaredTotal reads the work's declared total and applies §6.1's status gate.
//
// It returns INVALID — which becomes `"total":null` — in every case but one:
// the series has a status that means it has STOPPED, and a positive total was
// declared. `total_issues_declared` lives on work_comic and is written by the
// credit pass from the same metadata read that carries the credits, so the gate
// costs no HTTP at all (kavita.SeriesMetadataDto carries publicationStatus and
// totalCount beside the person arrays).
//
// ⚠️ A WORK WITH NO work_comic ROW HAS NO DECLARED TOTAL, AND THAT IS THE BOOKS
// CASE RATHER THAN AN ERROR. work_book has no total column — a book is not a
// series of issues — so every 'book' work takes the sql.ErrNoRows branch and
// gets `total: null`, which is the honest answer for it.
func declaredTotal(
	ctx context.Context, tx *sql.Tx, workID int64,
) (total sql.NullInt64, source sql.NullString, err error) {
	var status sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT w.status, c.total_issues_declared, c.total_issues_source
		  FROM work_comic c JOIN work w ON w.id = c.work_id
		 WHERE c.work_id = ?`, workID).Scan(&status, &total, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, sql.NullString{}, nil
	}
	if err != nil {
		return sql.NullInt64{}, sql.NullString{}, fmt.Errorf("read the declared total: %w", err)
	}
	if !statusAllowsATotal(status.String) || !total.Valid || total.Int64 <= 0 {
		// ⚠️ `total: 0` IS NEVER EMITTED. A declared zero is not a series of
		// zero issues, it is an upstream that declared nothing — Kavita's
		// totalCount is a plain int32 whose unset value is 0 — and §6.3's
		// render rule (`have == total && total > 0` → ✓) is written the way it
		// is because a 0 total would otherwise mark an empty series complete.
		return sql.NullInt64{}, sql.NullString{}, nil
	}
	return total, source, nil
}
