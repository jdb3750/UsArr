package store

import (
	"context"
	"database/sql"
	"fmt"
)

// redactBatchSize bounds how many rewrites are held in memory and applied per
// transaction. Small enough that the first pass over a large provenance table
// does not build one enormous statement batch, large enough that a homelab's
// whole table is a single round trip.
const redactBatchSize = 500

// RedactStoredProvenanceURLs rewrites every provenance.nzb_info_url through
// redact and returns how many rows changed.
//
// WHY THIS EXISTS. provenance.nzb_info_url is indexer-supplied and permanent by
// design: the host and the release path are the record of where a file came
// from, and provenance rows are never overwritten. Rows written before the grab
// path started redacting can therefore hold a private tracker's passkey
// verbatim, forever, in the database file and in every backup taken from it.
// release_candidate heals itself — its rows expire in 25 minutes and the sweeper
// deletes them — but provenance has no such mechanism, so the repair has to be
// explicit.
//
// WHY IT TAKES THE REDACTOR AS A PARAMETER. The caller passes the SAME helper
// the write path uses (servarr.RedactURL, which is ssrf.RedactRawURL). Injecting
// it keeps internal/store free of URL policy, and — the point — makes it
// impossible for this pass to drift from the write path: there is exactly one
// deny-list and one path heuristic in the codebase, and a stored value and a
// served value can never disagree about what counts as a secret. Doing this in
// SQL instead would have meant reimplementing both in `replace()` chains, and
// SQLite has no regular expressions at all, so the path heuristic could not have
// been expressed there even badly.
//
// IDEMPOTENT, by construction. Only rows whose redacted form DIFFERS from what
// is stored are updated, and the redactor is itself idempotent — the REDACTED
// marker is short, all-letter and digit-free, so it can never look like a
// credential to the next pass. Re-running this changes nothing, which is what
// lets it be a startup pass rather than a once-ever migration whose failure mode
// is "the process died halfway and it never runs again".
//
// LIMIT, and it must be said out loud: this repairs the LIVE database only. A
// passkey already copied into a `VACUUM INTO` backup, a filesystem snapshot or a
// support bundle is still in that copy, and nothing UsArr does can reach it. The
// only remedy there is to rotate the passkey at the tracker and delete the old
// backups. docs/reference/security.md §5 says the same thing.
func (s *Store) RedactStoredProvenanceURLs(ctx context.Context, redact func(string) string) (int64, error) {
	if redact == nil {
		return 0, fmt.Errorf("store: RedactStoredProvenanceURLs needs a redactor")
	}

	var changed int64
	// Keyset cursor rather than OFFSET: the pass updates rows as it goes, and
	// OFFSET over a table being written under you skips rows.
	var afterID int64
	for {
		batch, seen, lastID, err := s.scanProvenanceURLs(ctx, afterID, redact)
		if err != nil {
			return changed, err
		}
		if seen > 0 {
			afterID = lastID
		}

		if len(batch) > 0 {
			err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				stmt, err := tx.PrepareContext(ctx,
					`UPDATE provenance SET nzb_info_url = ? WHERE id = ?`)
				if err != nil {
					return fmt.Errorf("redact provenance url: prepare: %w", err)
				}
				defer func() { _ = stmt.Close() }()
				for _, r := range batch {
					if _, err := stmt.ExecContext(ctx, r.url, r.id); err != nil {
						return fmt.Errorf("redact provenance %d url: %w", r.id, err)
					}
				}
				return nil
			})
			if err != nil {
				return changed, err
			}
			changed += int64(len(batch))
		}

		if seen < redactBatchSize {
			return changed, nil
		}
	}
}

// provenanceRewrite is one row the pass will update.
type provenanceRewrite struct {
	id  int64
	url string
}

// scanProvenanceURLs reads one keyset page and returns the rows whose redacted
// form differs, how many rows the page held, and the last id it saw.
//
// It is a separate function purely so the rows can be closed with defer. Closing
// a *sql.Rows by hand at three exits inside a loop is how one exit ends up
// missing it, which on the single-writer pool is a leaked read connection.
func (s *Store) scanProvenanceURLs(ctx context.Context, afterID int64, redact func(string) string) ([]provenanceRewrite, int, int64, error) {
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT id, nzb_info_url
		  FROM provenance
		 WHERE id > ? AND nzb_info_url IS NOT NULL AND nzb_info_url <> ''
		 ORDER BY id ASC
		 LIMIT ?`, afterID, redactBatchSize)
	if err != nil {
		return nil, 0, afterID, fmt.Errorf("scan provenance for stored credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var batch []provenanceRewrite
	seen := 0
	lastID := afterID
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, seen, lastID, fmt.Errorf("scan provenance for stored credentials: %w", err)
		}
		seen++
		lastID = id
		// Only rows that actually change are collected. A row with no
		// credential is left byte-identical: rewriting a clean permanent record
		// buys nothing and risks everything.
		if safe := redact(raw); safe != raw {
			batch = append(batch, provenanceRewrite{id: id, url: safe})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, seen, lastID, fmt.Errorf("scan provenance for stored credentials: %w", err)
	}
	return batch, seen, lastID, nil
}
