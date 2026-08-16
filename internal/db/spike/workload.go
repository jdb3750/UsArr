//go:build bench

package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// The read workload. An open-and-idle measurement would report an empty page
// cache and an untouched mmap region, which is precisely the number that must
// not be pasted into an ADR as "peak".
//
// Every query shape below is one the code really issues — they are the shapes
// pinned by internal/db/queryplan_test.go, which is the CI gate on this
// schema's indexes — plus the keyset pagination ARCHITECTURE §13 mandates for
// every list. Nothing here is a synthetic table scan invented to move memory.
//
// Counts are per pass and fixed, so two cells of the sweep do the same work and
// their RSS is comparable. Randomness is seeded per goroutine, so a re-run on
// the same box repeats.
const (
	opsKeysetPages  = 100   // ordered pages of 100 over ix_audit_ts
	opsTagFilter    = 300   // "all items with tag X", user-scoped
	opsWorkLookup   = 10000 // "tags of this work" — the N+1 shape, random ids
	opsProvLookup   = 3000  // provenance by download_id — random, wide rows
	opsExpirySweep  = 20    // the release TTL sweep
	opsQueueRunnabl = 20    // the writer's runnable scan
)

// querier is what a pass needs: *sql.DB (the pool) or *sql.Conn (one pinned
// connection). Pinning one connection is how the sweep separates a
// per-connection page cache from a shared one.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// readPass runs the full query set once and returns the number of statements
// executed.
func readPass(ctx context.Context, q querier, counts fixtureCounts, seed uint64) (int, error) {
	// Deterministic on purpose: two sweep cells must issue the same queries in
	// the same order, or their RSS is not comparable. Nothing here is a secret.
	rnd := rand.New(rand.NewPCG(seed, seed^0x9e3779b9)) //nolint:gosec // G404: reproducibility is the requirement
	n := 0

	// 1. Keyset pagination, newest first. §13: keyset always, never OFFSET.
	// The cursor is (ts, id) — ts alone is not unique in the fixture and is not
	// unique in production either.
	cursorTS := "9999-12-31 23:59:59"
	cursorID := int64(1 << 62)
	for range opsKeysetPages {
		got := 0
		err := query(ctx, q, &n,
			`SELECT id, ts, action FROM audit_log
			  WHERE (ts, id) < (?, ?)
			  ORDER BY ts DESC, id DESC
			  LIMIT 100`,
			[]any{cursorTS, cursorID},
			func(rows *sql.Rows) error {
				var id int64
				var ts, action string
				if err := rows.Scan(&id, &ts, &action); err != nil {
					return err
				}
				cursorTS, cursorID = ts, id
				got++
				return nil
			})
		if err != nil {
			return n, fmt.Errorf("keyset page: %w", err)
		}
		if got == 0 {
			break
		}
	}

	// 2. "All items with tag X", user-scoped — the canonical tag filter, and
	// the one query in the schema whose index was designed to cover it.
	for range opsTagFilter {
		err := query(ctx, q, &n,
			`SELECT work_id FROM tag_assignment WHERE tag_id = ? AND user_id IN (0, ?)`,
			[]any{1 + rnd.IntN(fixtureTags), 1},
			func(rows *sql.Rows) error {
				var workID sql.NullInt64
				return rows.Scan(&workID)
			})
		if err != nil {
			return n, fmt.Errorf("tag filter: %w", err)
		}
	}

	// 3. "Tags of this work", random ids across the whole key space. This is
	// the N+1 shape §13 names, and random access is what actually forces page
	// faults through the page cache.
	works := max(counts.TagAssignment/3, 1)
	for range opsWorkLookup {
		err := query(ctx, q, &n,
			`SELECT tag_id FROM tag_assignment WHERE work_id = ?`,
			[]any{1 + rnd.IntN(works)},
			func(rows *sql.Rows) error {
				var tagID int64
				return rows.Scan(&tagID)
			})
		if err != nil {
			return n, fmt.Errorf("work lookup: %w", err)
		}
	}

	// 4. Provenance by download_id — THE join key reattaching an import event
	// to the grab. Random, and it lands on wide table pages.
	if counts.Provenance > 0 {
		for range opsProvLookup {
			err := query(ctx, q, &n,
				`SELECT id, release_title, indexer_name, protocol FROM provenance
				  WHERE download_id = ?`,
				[]any{fmt.Sprintf("nzo_%08d", rnd.IntN(counts.Provenance))},
				func(rows *sql.Rows) error {
					var pid int64
					var title, indexer, proto string
					return rows.Scan(&pid, &title, &indexer, &proto)
				})
			if err != nil {
				return n, fmt.Errorf("provenance lookup: %w", err)
			}
		}
	}

	// 5. The release TTL sweep and the writer's runnable scan — both run on a
	// timer in production, so they are part of any honest idle-to-peak walk.
	now := time.Now().UTC().Format(time.DateTime)
	for range opsExpirySweep {
		err := query(ctx, q, &n,
			`SELECT id FROM release_candidate WHERE expires_at <= ?`, []any{now},
			func(rows *sql.Rows) error {
				var id int64
				return rows.Scan(&id)
			})
		if err != nil {
			return n, fmt.Errorf("expiry sweep: %w", err)
		}
	}
	for range opsQueueRunnabl {
		err := query(ctx, q, &n,
			`SELECT id, kind FROM write_queue
			  WHERE state IN ('pending','inflight','verifying') AND next_attempt_at <= ?`,
			[]any{now},
			func(rows *sql.Rows) error {
				var id int64
				var kind string
				return rows.Scan(&id, &kind)
			})
		if err != nil {
			return n, fmt.Errorf("queue scan: %w", err)
		}
	}

	return n, nil
}

// query runs one statement, hands every row to scan, and counts the statement.
// Every result set in this harness goes through here so that closing is one
// deferred call in one place rather than an error path per call site.
func query(ctx context.Context, q querier, n *int, sqlText string, args []any,
	scan func(*sql.Rows) error,
) error {
	rows, err := q.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	*n++

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// writeBurst runs a small import through the single writer at the §7.7 batch
// size, so the peak figure includes writer-side memory rather than measuring a
// read-only process and calling it peak.
//
// It writes tag_assignment rows above the fixture's key space, so it collides
// with nothing and every one of that table's six indexes takes the insert.
func writeBurst(ctx context.Context, d *db.DB, counts fixtureCounts, batches int) (int, error) {
	base := counts.TagAssignment + 1_000_000
	written := 0
	for b := range batches {
		start := base + b*importBatch
		err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			stmt, err := tx.PrepareContext(ctx,
				`INSERT INTO tag_assignment (tag_id, work_id, user_id, source, added_at)
				 VALUES (?, ?, 0, 'system', datetime('now'))`)
			if err != nil {
				return fmt.Errorf("prepare: %w", err)
			}
			defer func() { _ = stmt.Close() }()
			for i := range importBatch {
				if _, err := stmt.ExecContext(ctx, 1+(i%fixtureTags), start+i); err != nil {
					return fmt.Errorf("insert: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return written, err
		}
		written += importBatch
	}
	return written, nil
}
