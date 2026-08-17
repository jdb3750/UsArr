//go:build bench

package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// The fixture is 500,000 rows across the tables migration 0001 actually
// creates. ARCHITECTURE §13 asks for "a 500k-row fixture"; it does not say which
// tables, and it cannot, because the spike is a PREREQUISITE to the schema work.
//
// ⚠️ MIGRATION 0005 HAS SINCE LANDED, and this fixture has NOT been rebuilt for
// it. work / edition / media_file / search_doc / search_fts / search_trgm all
// exist now, and the composition below still ignores them, so what this harness
// measures is a subset of the real schema. Rebuilding it is owed to whoever
// next runs `make bench-rss` against a claim about the library tables.
//
// One change was forced rather than chosen: migration 0005 restores
// `write_queue.work_id REFERENCES work(id) ON DELETE CASCADE`, so the 8,000
// queue rows below need real `work` rows to point at or the fixture build fails
// on the foreign key. seedDimensions creates exactly that many, narrow and
// title-only. They are DIMENSION rows, not part of the 500k plan — the same
// treatment tags and service instances already get — because inventing a
// work-table share of the budget would silently change what every previously
// recorded RSS number means.
//
// So the composition below is chosen for page-and-index pressure comparable to
// the reference library (10k movies / 2k series / ~400k episodes), using rows
// the shipped schema can hold:
//
//	tag_assignment    400,000  narrow rows, SIX indexes — the index-page pressure
//	                           the episode table will produce
//	provenance         60,000  wide rows (verbatim release titles, JSON arrays) —
//	                           table-page volume
//	audit_log          30,000  the one ordered-scan index (ix_audit_ts)
//	write_queue         8,000  partial index + unique idempotency index
//	release_candidate   2,000  ~1 KiB raw_release_json each — overflow pages
//
// What this therefore does NOT measure, and the report says so: FTS5 memory.
// Both search tables are contentless with a trigram sibling (§8.2) and they will
// have their own profile. Re-run this harness after the migration that adds
// them.
const (
	fracTagAssignment    = 0.800
	fracProvenance       = 0.120
	fracAuditLog         = 0.060
	fracWriteQueue       = 0.016
	fracReleaseCandidate = 0.004

	// Tags in the fixture. Small on purpose: a homelab has tens of tags over
	// hundreds of thousands of items, which is what makes "all items with tag
	// X" a wide index range rather than a point lookup.
	fixtureTags = 200

	// Service instances. A homelab has single digits (schema.md §5).
	fixtureInstances = 4

	// `work` rows seeded so write_queue.work_id has something to reference
	// (migration 0005 restored that foreign key). The queue is 1.6% of the
	// plan, so this covers every work_id it generates at the default 500k and
	// is checked at run time rather than assumed — see buildFixture.
	seedWorks = 8000

	// §7.7 rule 3: import batches commit at min(2000 rows, 100 ms). Row count
	// is the deterministic half, and it is what the fixture uses.
	importBatch = 2000
)

// fixtureCounts is the row plan for a given total.
type fixtureCounts struct {
	TagAssignment    int
	Provenance       int
	AuditLog         int
	WriteQueue       int
	ReleaseCandidate int
}

func (c fixtureCounts) total() int {
	return c.TagAssignment + c.Provenance + c.AuditLog + c.WriteQueue + c.ReleaseCandidate
}

func planFixture(rows int) fixtureCounts {
	c := fixtureCounts{
		TagAssignment: int(float64(rows) * fracTagAssignment),
		Provenance:    int(float64(rows) * fracProvenance),
		AuditLog:      int(float64(rows) * fracAuditLog),
		WriteQueue:    int(float64(rows) * fracWriteQueue),
	}
	// The remainder lands on release_candidate so the total is exact.
	c.ReleaseCandidate = rows - c.TagAssignment - c.Provenance - c.AuditLog - c.WriteQueue
	return c
}

// fixtureComplete reports whether the database already holds the planned rows,
// so a second run on the same box skips the slowest step.
func fixtureComplete(ctx context.Context, d *db.DB, want fixtureCounts) bool {
	got, err := countRows(ctx, d.Read())
	if err != nil {
		return false
	}
	return got == want
}

func countRows(ctx context.Context, q *sql.DB) (fixtureCounts, error) {
	var c fixtureCounts
	for _, t := range []struct {
		table string
		into  *int
	}{
		{"tag_assignment", &c.TagAssignment},
		{"provenance", &c.Provenance},
		{"audit_log", &c.AuditLog},
		{"write_queue", &c.WriteQueue},
		{"release_candidate", &c.ReleaseCandidate},
	} {
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM "+t.table).Scan(t.into); err != nil {
			return c, fmt.Errorf("count %s: %w", t.table, err)
		}
	}
	return c, nil
}

// buildFixture fills an open, migrated database with the planned rows.
//
// Every write goes through db.Write, i.e. through the single writer connection
// and BEGIN IMMEDIATE, in batches of importBatch — the import path §7.7
// describes. Nothing here bypasses the real open path.
func buildFixture(ctx context.Context, d *db.DB, c fixtureCounts) error {
	// Checked, not assumed: write_queue.work_id is a foreign key since 0005, so
	// a plan that outgrows the seeded work rows fails halfway through the
	// slowest step with a bare "FOREIGN KEY constraint failed".
	if c.WriteQueue > seedWorks {
		return fmt.Errorf("fixture plans %d write_queue rows but only %d work rows are seeded; "+
			"raise seedWorks (write_queue.work_id references work(id) since migration 0005)",
			c.WriteQueue, seedWorks)
	}
	if err := seedDimensions(ctx, d); err != nil {
		return err
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := func(i int) string { return base.Add(time.Duration(i) * time.Second).Format(time.DateTime) }

	// tag_assignment. Each synthetic work carries either 2 or 3 tags, which is
	// what makes ix_ta_work a real multi-row lookup rather than a unique hit.
	// Tag ids are picked so (work_id, tag_id, user_id) stays unique under
	// ux_ta_work.
	if err := insertBatched(ctx, d, c.TagAssignment,
		`INSERT INTO tag_assignment (tag_id, work_id, user_id, source, added_at)
		 VALUES (?, ?, ?, ?, ?)`,
		func(i int) []any {
			work := i/3 + 1
			slot := i % 3
			tagID := 1 + (work*7+slot*137)%fixtureTags
			userID := 0
			if slot == 2 {
				userID = 1 // a user-scoped tag; the rest are shared (user 0)
			}
			source := "system"
			if slot == 1 {
				source = "user"
			}
			return []any{tagID, work, userID, source, ts(i % 86400)}
		}); err != nil {
		return fmt.Errorf("tag_assignment: %w", err)
	}

	// provenance. Wide rows, verbatim release names, JSON arrays — the shape
	// that puts bytes on table pages.
	if err := insertBatched(ctx, d, c.Provenance,
		`INSERT INTO provenance (protocol, indexer_name, indexer_id, indexer_privacy,
		    indexer_categories, indexer_flags, download_client_type, download_client_name,
		    download_id, torrent_info_hash, download_url, release_guid, release_title,
		    release_group, quality_source, quality_resolution, video_codec, audio_codec,
		    audio_channels, languages, published_at, grabbed_at, imported_at,
		    source_system, source_record_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		func(i int) []any {
			proto := "usenet"
			client := "Sabnzbd"
			if i%2 == 0 {
				proto, client = "torrent", "QBittorrent"
			}
			return []any{
				proto,
				fmt.Sprintf("indexer-%02d", i%16),
				i % 16,
				"private",
				`[2040,5040,5030]`,
				`["freeleech","internal"]`,
				client,
				strings.ToLower(client) + "-main",
				fmt.Sprintf("nzo_%08d", i),
				fmt.Sprintf("%040x", i),
				fmt.Sprintf("https://indexer-%02d.invalid/download/%d", i%16, i),
				fmt.Sprintf("guid-%016d", i),
				fmt.Sprintf("Some.Long.Release.Name.S%02dE%02d.2160p.UHD.BluRay.REMUX.HDR.DV.HEVC.TrueHD.7.1.Atmos-GROUP%04d",
					i%40, i%24, i%1000),
				fmt.Sprintf("GROUP%04d", i%1000),
				"bluray", "2160p", "x265", "truehd", "7.1", `["English","Japanese"]`,
				ts(i % 86400), ts(i % 86400), ts(i % 86400),
				"prowlarr",
				fmt.Sprintf("%d", i),
			}
		}); err != nil {
		return fmt.Errorf("provenance: %w", err)
	}

	// audit_log. Descending ts is the one ordered index in the schema, and the
	// read workload pages through it with a keyset cursor.
	if err := insertBatched(ctx, d, c.AuditLog,
		`INSERT INTO audit_log (ts, actor_user_id, actor_ip, action, target_type, target_id,
		    result, metadata_json, prev_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		func(i int) []any {
			return []any{
				ts(i), 1, "10.0.0.7",
				[]string{"login", "service.create", "release.grab", "tag.add"}[i%4],
				"service_instance", fmt.Sprintf("%d", 1+i%fixtureInstances),
				"ok", `{"detail":"synthetic fixture row"}`,
				fmt.Sprintf("%064x", i),
			}
		}); err != nil {
		return fmt.Errorf("audit_log: %w", err)
	}

	// write_queue. Most rows settled, a minority runnable — which is what
	// ix_wq_runnable's partial index is for.
	if err := insertBatched(ctx, d, c.WriteQueue,
		`INSERT INTO write_queue (idempotency_key, user_id, kind, work_id, service_instance_id,
		    payload, state, attempts, next_attempt_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		func(i int) []any {
			state := "done"
			var next any
			switch i % 20 {
			case 0:
				state, next = "pending", ts(i%86400)
			case 1:
				state, next = "inflight", ts(i%86400)
			case 2:
				state, next = "verifying", ts(i%86400)
			}
			return []any{
				fmt.Sprintf("01J%026d", i), // ULID-shaped, deliberately synthetic
				i % 2, []string{"add", "monitor", "grab", "tag_add"}[i%4],
				i + 1, 1 + i%fixtureInstances,
				`{"synthetic":true}`, state, 0, next, ts(i % 86400),
			}
		}); err != nil {
		return fmt.Errorf("write_queue: %w", err)
	}

	// release_candidate. ~1 KiB of raw_release_json each: the verbatim
	// ReleaseResource the grab needs, and the fixture's overflow-page source.
	rawJSON := `{"guid":"synthetic","title":"synthetic release","indexer":"indexer","size":123456789,` +
		strings.Repeat(`"pad":"`+strings.Repeat("x", 60)+`",`, 14) + `"protocol":"torrent"}`
	if err := insertBatched(ctx, d, c.ReleaseCandidate,
		`INSERT INTO release_candidate (work_id, service_instance_id, guid, title, indexer,
		    indexer_id, protocol, categories, size_bytes, seeders, leechers, age_days, quality,
		    download_url, info_url, info_hash, raw_release_json, fetched_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		func(i int) []any {
			return []any{
				i + 1, 1 + i%fixtureInstances,
				fmt.Sprintf("guid-rc-%08d", i),
				fmt.Sprintf("Synthetic.Release.%08d.2160p.WEB-DL", i),
				fmt.Sprintf("indexer-%02d", i%16), i % 16, "torrent",
				`[2040,5040]`, int64(i) * 1024 * 1024, i % 100, i % 20, float64(i%400) / 10.0,
				"WEBDL-2160p",
				fmt.Sprintf("https://indexer-%02d.invalid/dl/%d", i%16, i),
				fmt.Sprintf("https://indexer-%02d.invalid/info/%d", i%16, i),
				fmt.Sprintf("%040x", i),
				rawJSON,
				ts(i % 86400),
				// Half already expired, so the TTL sweep has real work.
				base.Add(time.Duration(i%2000-1000) * time.Minute).Format(time.DateTime),
			}
		}); err != nil {
		return fmt.Errorf("release_candidate: %w", err)
	}

	// §7.7 rule 5: ANALYZE after bulk import. Then truncate the WAL so the
	// fixture on disk is one file and every sweep cell starts identical.
	if _, err := d.Writer().ExecContext(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	if _, err := d.Writer().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	return nil
}

// seedDimensions writes the small tables the big ones reference.
func seedDimensions(ctx context.Context, d *db.DB) error {
	return d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// User 0 (_system) is created by migration 0001. User 1 is the owner:
		// tag_assignment.user_id references user(id), and half the fixture's
		// tags are user-scoped.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user (id, username, display_name, auth_source, is_owner)
			 VALUES (1, 'spike-owner', 'Spike Owner', 'local', 1)`); err != nil {
			return fmt.Errorf("seed user: %w", err)
		}
		for i := 1; i <= fixtureTags; i++ {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO tag (id, namespace, value, is_system) VALUES (?, ?, ?, ?)`,
				i, []string{"tag", "genre", "quality", "status"}[i%4],
				fmt.Sprintf("value-%03d", i), i%4 == 0); err != nil {
				return fmt.Errorf("seed tag: %w", err)
			}
		}
		// api_key_enc is NOT NULL and is a ciphertext envelope in production.
		// Random bytes generated at run time are the right stand-in: nothing
		// key-shaped is ever written into this repo (DEVELOPMENT.md §7.1).
		key := make([]byte, 60)
		for i := 1; i <= fixtureInstances; i++ {
			if _, err := rand.Read(key); err != nil {
				return fmt.Errorf("seed instance key: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc, api_version)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				i, []string{"radarr", "sonarr", "prowlarr", "lidarr"}[i%4],
				[]string{"library", "library", "indexer", "library"}[i%4],
				fmt.Sprintf("spike-instance-%d", i),
				fmt.Sprintf("http://127.0.0.1:%d", 7878+i),
				key, "v3"); err != nil {
				return fmt.Errorf("seed instance: %w", err)
			}
		}
		// `work` rows for write_queue.work_id to reference. Migration 0005
		// restored that foreign key; without these the write_queue insert below
		// fails on every row. Narrow and title-only: this is not an attempt to
		// model the work table's real page pressure, which is what the ⚠️ note
		// at the top of this file says is still owed.
		for i := 1; i <= seedWorks; i++ {
			title := fmt.Sprintf("Spike Work %06d", i)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO work (id, kind, title, sort_title, normalized_title)
				 VALUES (?, ?, ?, ?, ?)`,
				i, []string{"movie", "series", "episode", "album"}[i%4],
				title, strings.ToLower(title), strings.ToLower(title)); err != nil {
				return fmt.Errorf("seed work: %w", err)
			}
		}
		return nil
	})
}

// insertBatched runs n inserts through the single writer, committing every
// importBatch rows, preparing the statement once per transaction.
func insertBatched(ctx context.Context, d *db.DB, n int, query string, args func(i int) []any) error {
	for start := 0; start < n; start += importBatch {
		end := min(start+importBatch, n)
		err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			stmt, err := tx.PrepareContext(ctx, query)
			if err != nil {
				return fmt.Errorf("prepare: %w", err)
			}
			defer func() { _ = stmt.Close() }()
			for i := start; i < end; i++ {
				if _, err := stmt.ExecContext(ctx, args(i)...); err != nil {
					return fmt.Errorf("row %d: %w", i, err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
