package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// Query-plan assertions. docs/DEVELOPMENT.md §5 puts these in CI because they
// are deterministic and hardware-independent: ~30 lines that catch an index
// regression forever. Wall-clock budgets live in `make bench` and are never a
// merge gate.
//
// WHAT IS ASSERTED, AND WHAT IS DELIBERATELY NOT: each case asserts that the
// step is a SEARCH (not a SCAN) and that the expected index is named. It does
// NOT assert COVERING. Per docs/reference/schema.md §1, whether SQLite can
// serve a query from the index alone depends on the SELECT list, so pinning
// COVERING makes the test fail the moment a caller selects one more column —
// which is a change to the query, not an index regression.
func TestQueryPlans(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	tests := []struct {
		name string
		// index is the index the plan must name.
		index string
		query string
		args  []any
		// scan marks a query whose plan is an ordered SCAN over an index
		// rather than a SEARCH. That is the right plan for "newest N": the
		// index supplies the order, so there is no temp b-tree.
		scan bool
	}{
		{
			// The canonical tag filter. user_id is IN the index because the
			// predicate always carries it; without it the covering property is
			// lost and a common tag on 400k items becomes 400k random reads.
			name:  "tag filter is user-scoped and indexed",
			index: "ix_ta_tag",
			query: `SELECT work_id FROM tag_assignment WHERE tag_id = ? AND user_id IN (0, ?)`,
			args:  []any{1, 1},
		},
		{
			name:  "tags of one work",
			index: "ix_ta_work",
			query: `SELECT tag_id FROM tag_assignment WHERE work_id = ?`,
			args:  []any{1},
		},
		{
			name:  "tags of one service instance",
			index: "ix_ta_inst_lookup",
			query: `SELECT id FROM tag_assignment WHERE service_instance_id = ? AND tag_id = ?`,
			args:  []any{1, 1},
		},
		{
			// The northbound auth path: one indexed lookup, then one HMAC and
			// one constant-time compare. This is what keeps
			// GET /rest/ping?apiKey=garbage from costing anything.
			name:  "api key lookup by prefix",
			index: "ux_cc_prefix",
			query: `SELECT id, user_id, key_hash FROM client_credential WHERE key_prefix = ?`,
			args:  []any{"abcdefgh"},
		},
		{
			name:  "live credentials for one user",
			index: "ix_cc_user",
			query: `SELECT id FROM client_credential WHERE user_id = ? AND revoked_at IS NULL`,
			args:  []any{1},
		},
		{
			name:  "live sessions for one user",
			index: "ix_session_user",
			query: `SELECT id FROM session WHERE user_id = ? AND revoked_at IS NULL`,
			args:  []any{1},
		},
		{
			name:  "newest audit rows",
			index: "ix_audit_ts",
			query: `SELECT id, action FROM audit_log ORDER BY ts DESC LIMIT 100`,
			scan:  true,
		},
		{
			// The TTL sweep. Prowlarr drops grabbable releases after 30
			// minutes, so this runs often and must not scan the table.
			name:  "expired release candidates",
			index: "ix_rel_expiry",
			query: `SELECT id FROM release_candidate WHERE expires_at <= ?`,
			args:  []any{"2026-08-16 00:00:00"},
		},
		{
			// downloadId is THE join key reattaching an import event to the
			// grab that produced it.
			name:  "provenance by download id",
			index: "ix_prov_dlid",
			query: `SELECT id FROM provenance WHERE download_id = ?`,
			args:  []any{"nzo_1"},
		},
		{
			name:  "provenance by indexer",
			index: "ix_prov_indexer",
			query: `SELECT id FROM provenance WHERE indexer_name = ?`,
			args:  []any{"an-indexer"},
		},
		{
			name:  "provenance by protocol",
			index: "ix_prov_protocol",
			query: `SELECT id FROM provenance WHERE protocol = ?`,
			args:  []any{"torrent"},
		},
		{
			// Migration 0002's index. The Recent-grabs block on Home orders by
			// grab time, and 0001's three provenance indexes are all equality
			// lookups, so this read sorted the whole table.
			//
			// The SELECT list is the one the block renders, which is why the
			// plan is SEARCH and not COVERING — schema.md §1's warning: pinning
			// COVERING makes the test fail the moment a caller selects one more
			// column, which is a change to the query, not an index regression.
			name:  "recent grabs for one user",
			index: "ix_prov_user_grabbed",
			query: `SELECT id, release_title, indexer_name, size_bytes, grabbed_at
			          FROM provenance
			         WHERE user_id = ?
			         ORDER BY grabbed_at DESC, id DESC
			         LIMIT 50`,
			args: []any{1},
		},
		{
			// Keyset paging over the same index. id breaks ties because
			// grabbed_at has one-second resolution, and two grabs in the same
			// second would otherwise make a page repeat a row.
			name:  "recent grabs, next page",
			index: "ix_prov_user_grabbed",
			query: `SELECT id, release_title, grabbed_at
			          FROM provenance
			         WHERE user_id = ? AND (grabbed_at, id) < (?, ?)
			         ORDER BY grabbed_at DESC, id DESC
			         LIMIT 50`,
			args: []any{1, "2026-08-16 00:00:00", 9999},
		},
		{
			// Migration 0002's other index: "this user's failed grabs", the
			// attention block. Filtering one actor's rows by action used to scan
			// a table that grows forever by design.
			name:  "one user's grab failures",
			index: "ix_audit_actor_action",
			query: `SELECT id, ts, target_id, result, metadata_json
			          FROM audit_log
			         WHERE actor_user_id = ? AND action = ?
			         ORDER BY ts DESC
			         LIMIT 50`,
			args: []any{1, "grab"},
		},
		{
			// Idempotency is (user_id, key), never a bare unique on the key: a
			// globally unique client-supplied key means a replay can return
			// another user's payload.
			name:  "write queue idempotency lookup",
			index: "ux_wq_idem",
			query: `SELECT id FROM write_queue WHERE user_id = ? AND idempotency_key = ?`,
			args:  []any{0, "01J000000000000000000000"},
		},
		{
			// Also the reconciliation guard's index: the sweep skips any
			// work_id with a row in pending, inflight or verifying.
			name:  "runnable write queue entries",
			index: "ix_wq_runnable",
			query: `SELECT id FROM write_queue
			         WHERE state IN ('pending','inflight','verifying') AND next_attempt_at <= ?`,
			args: []any{"2026-08-16 00:00:00"},
		},
		{
			name:  "write queue entries for one work",
			index: "ix_wq_work",
			query: `SELECT id FROM write_queue WHERE work_id = ? AND state = ?`,
			args:  []any{1, "pending"},
		},

		// ─── Migration 0005 ──────────────────────────────────────────────

		{
			// schema.md §1's NORMATIVE plan, and the one the library grid is
			// built on. The exact fragment is asserted by
			// TestWorkGridKeysetPlanIsNormative below, which is a separate test
			// because the fragment — not just the index name — is what §1 pins.
			name:  "library grid keyset by kind",
			index: "ix_work_kind_sort",
			query: `SELECT title, year, poster_asset_id, have_count, want_count, availability
			          FROM work
			         WHERE kind = ? AND deleted_at IS NULL AND (sort_title, id) > (?, ?)
			         ORDER BY sort_title, id
			         LIMIT 100`,
			args: []any{"movie", "", 0},
		},
		{
			// Home's unified recently-added table (ADR-0028 block C). The
			// partial predicate is what keeps the 7-day tombstone window from
			// degrading exactly this view.
			name:  "recently added, all kinds",
			index: "ix_work_added",
			query: `SELECT id, title, kind FROM work WHERE deleted_at IS NULL
			         ORDER BY added_at DESC, id DESC LIMIT 50`,
			scan: true,
		},
		{
			name:  "children of a work",
			index: "ix_work_parent",
			query: `SELECT id FROM work WHERE parent_work_id = ? AND kind = ?`,
			args:  []any{1, "episode"},
		},
		{
			// Identity tier 1: normalised title + year + kind.
			name:  "work by normalised title",
			index: "ix_work_norm",
			query: `SELECT id FROM work WHERE normalized_title = ? AND year = ? AND kind = ?`,
			args:  []any{"train dreams", 2025, "movie"},
		},
		{
			// The 250 ms rollup flush picks up its batch here. A full index on
			// rollup_dirty would be a second index over every work row for a
			// column that is 0 almost always.
			name:  "dirty rollups awaiting the flush",
			index: "ix_work_dirty",
			query: `SELECT id FROM work WHERE rollup_dirty = 1`,
		},
		{
			name:  "files of one work",
			index: "ix_file_work",
			query: `SELECT id, path, size_bytes FROM media_file WHERE work_id = ?`,
			args:  []any{1},
		},
		{
			// (service_instance_id, path), never path alone: media_file rows are
			// per-instance observations, and two services indexing one volume
			// is the normal case.
			name:  "file by instance and path",
			index: "ux_file_path",
			query: `SELECT id FROM media_file WHERE service_instance_id = ? AND path = ?`,
			args:  []any{1, "/media/movies/x.mkv"},
		},
		{
			// ix_extid_lookup(source, value) is deliberately absent; ux_extid
			// serves the equality as its leftmost prefix. This is the assertion
			// that says so, so nobody adds a second B-tree to the importer's
			// hottest insert path without a measurement.
			name:  "external id lookup",
			index: "ux_extid",
			query: `SELECT work_id, edition_id FROM external_id WHERE source = ? AND value = ?`,
			args:  []any{"tmdb_movie", "1017298"},
		},
		{
			name:  "external ids of one work",
			index: "ix_extid_work",
			query: `SELECT source, value FROM external_id WHERE work_id = ?`,
			args:  []any{1},
		},
		{
			// ux_sil is only usable IN FULL when remote_kind is known, which is
			// why the northbound id encodes it (ARCHITECTURE §5.3). Without it
			// this is a range scan over every link on the instance — ~400k rows
			// for a 2k-series Sonarr, on every stream resolve and getCoverArt.
			name:  "service link by remote identity",
			index: "ux_sil",
			query: `SELECT id, work_id FROM service_item_link
			         WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?`,
			args: []any{1, "movie", "842"},
		},
		{
			name:  "live links of one work",
			index: "ix_sil_work",
			query: `SELECT id FROM service_item_link WHERE work_id = ? AND deleted_at IS NULL`,
			args:  []any{1},
		},
		{
			// The membership derivation walks links per instance per container.
			// Without this index it is a full scan of every link on the instance
			// for every library the instance feeds.
			name:  "links in one upstream container",
			index: "ix_sil_container",
			query: `SELECT id, work_id FROM service_item_link
			         WHERE service_instance_id = ? AND remote_library_id = ? AND deleted_at IS NULL`,
			args: []any{1, "lib-7"},
		},
		{
			// Old northbound ids stay resolvable when a pinned instance is
			// deleted, and the resolve is on the hot stream path.
			name:  "northbound alias resolve",
			index: "service_item_alias",
			query: `SELECT link_id FROM service_item_alias
			         WHERE old_instance_id = ? AND old_remote_kind = ? AND old_remote_id = ?`,
			args: []any{1, "movie", "842"},
		},
		{
			name:  "editions of one work",
			index: "ix_edition_work",
			query: `SELECT id, label, format FROM edition WHERE work_id = ? ORDER BY is_primary DESC`,
			args:  []any{1},
		},
		{
			name:  "alt titles of one work",
			index: "ix_alt_work",
			query: `SELECT title FROM work_alt_title WHERE work_id = ?`,
			args:  []any{1},
		},
		{
			name:  "enabled libraries of one kind",
			index: "ix_library_kind",
			query: `SELECT id, name FROM library WHERE user_id = ? AND kind = ? AND enabled = 1`,
			args:  []any{0, "movie"},
		},
		{
			// "which libraries is this work in" — the item detail page, and the
			// search_doc_library rebuild.
			name:  "libraries containing one work",
			index: "ix_libmem_work",
			query: `SELECT library_id FROM library_member WHERE work_id = ?`,
			args:  []any{1},
		},
		{
			name:  "sources feeding one instance",
			index: "ix_libsrc_instance",
			query: `SELECT id, library_id FROM library_source WHERE service_instance_id = ?`,
			args:  []any{1},
		},
		{
			// §6.5 rule 1: a correction survives a tombstone expiry and an id
			// resurrection, and target_identity_hash is how it re-attaches.
			// That re-attachment runs per resurrected row on the sync path.
			name:  "override re-attach by identity hash",
			index: "ix_ovr_hash",
			query: `SELECT id, verb FROM library_override WHERE target_identity_hash = ?`,
			args:  []any{"abc"},
		},
		{
			name:  "search doc of one work",
			index: "ix_sd_work",
			query: `SELECT rowid FROM search_doc WHERE work_id = ?`,
			args:  []any{1},
		},
		{
			// The reverse of the junction's primary key: deleting a search_doc
			// row, and "which libraries can see this".
			name:  "libraries a search doc is visible through",
			index: "ix_sdl_doc",
			query: `SELECT library_id FROM search_doc_library WHERE doc_rowid = ?`,
			args:  []any{1},
		},
		{
			name:  "image cache sweep",
			index: "ix_img_state",
			query: `SELECT id FROM image_asset WHERE state = ? AND expires_at <= ?`,
			args:  []any{"ready", "2026-08-16 00:00:00"},
		},
		{
			// Migration 0006's one index, and schema.md §1.1 declares it
			// normative: it is what makes the contiguity report cheap —
			// "43 issues · #7, #12 and #30-32 missing", computed locally with no
			// upstream help, and per ARCHITECTURE §6.1 the only always-honest
			// completeness number in the domain. A range over number_sort is the
			// shape that report reads in; without the index it is a scan of
			// every issue in the library, which for a manga library is the
			// biggest table in the schema after `work` itself.
			name:  "issues in a number range",
			index: "ix_comic_issue_sort",
			query: `SELECT work_id, number_text FROM work_comic_issue
			         WHERE number_sort >= ? AND number_sort < ? ORDER BY number_sort`,
			args: []any{1.0, 60.0},
		},
		{
			name:  "recent sync reports for one instance",
			index: "ix_sync_report_instance",
			query: `SELECT id, kind, remote_id, created_at FROM sync_report
			         WHERE service_instance_id = ? ORDER BY created_at DESC LIMIT 20`,
			args: []any{1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := QueryPlan(ctx, d.Read(), tc.query, tc.args...)
			if err != nil {
				t.Fatalf("QueryPlan: %v", err)
			}
			joined := strings.Join(plan, " | ")

			if !strings.Contains(joined, tc.index) {
				t.Fatalf("plan does not use %s:\n  %s", tc.index, joined)
			}
			want := "SEARCH"
			if tc.scan {
				want = "SCAN"
			}
			if !strings.Contains(joined, want) {
				t.Errorf("plan is not a %s:\n  %s", want, joined)
			}
			if !tc.scan && strings.Contains(joined, "SCAN") {
				t.Errorf("plan contains a table SCAN:\n  %s", joined)
			}
			// An ordered read that still needs a sort is not using the index
			// for what it was created for.
			if strings.Contains(joined, "TEMP B-TREE") {
				t.Errorf("plan needs a temp b-tree:\n  %s", joined)
			}
		})
	}
}

// TestScopedProvenanceOrderNeedsASort records what the CANONICAL scope
// predicate costs, so the gap between the query the index was designed for and
// the query internal/store actually issues is written down rather than
// discovered.
//
// store.Scope.userPredicate renders `user_id IN (0, :uid)` — 0 being the
// shared/system sentinel that migration 0002 backfills provenance to, so the
// owner keeps seeing their own pre-0002 history. SQLite cannot supply ORDER BY
// from an index whose LEADING column is constrained by IN: it plans a SEARCH on
// ix_prov_user_grabbed and then a temp b-tree. The equality form above does come
// out ordered.
//
// So the index is doing half its job on that read: it still turns a full-table
// scan into a scan of just the readable rows, and the sort is over that bounded
// set with a LIMIT on it. This is pinned rather than fixed because the fix is a
// data change — attributing the backfilled rows to a real user in a later
// migration — not an index change, and it is not worth doing while v0.1 has one
// user and the two row sets are the same rows.
//
// t.Errorf, not t.Logf: if SQLite (or a rewritten predicate) ever makes this
// ordered, that is good news and this comment is then wrong, so it must fail and
// force the edit.
func TestScopedProvenanceOrderNeedsASort(t *testing.T) {
	plan, err := QueryPlan(t.Context(), openTestDB(t).Read(),
		`SELECT id, release_title, grabbed_at FROM provenance
		  WHERE user_id IN (?, ?) ORDER BY grabbed_at DESC, id DESC LIMIT 50`, 0, 1)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")
	if !strings.Contains(joined, "ix_prov_user_grabbed") {
		t.Errorf("the scoped read no longer uses ix_prov_user_grabbed at all: %s", joined)
	}
	if !strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("the scoped read is now ordered by the index: %s\n"+
			"That is an improvement, and it makes the note on store.Scope.userPredicate and on this "+
			"test wrong. Update both in the same change.", joined)
	}
}

// service_instance is deliberately left without an ordering index.
//
// The services list sorts by (priority, name) and plans as a scan plus a temp
// b-tree. That is correct: docs/reference/schema.md §5 specifies no such index,
// and a homelab has single-digit instances. Pinned here so the scan is a
// recorded decision rather than something a later reader "fixes" by adding an
// index to a ten-row table.
//
// The assertion below is t.Errorf, not t.Logf. It used to log, which meant the
// test could not fail: adding an ordering index left it green and silent, so it
// pinned nothing while claiming to pin a decision. Failing is the point — if
// someone adds the index, this test forces them to come here and change it
// deliberately, which is exactly the conversation the comment above wants to
// have. Removing the pin is a one-line edit; not noticing you removed it is the
// failure mode being guarded.
func TestServiceInstanceListScanIsIntentional(t *testing.T) {
	plan, err := QueryPlan(t.Context(), openTestDB(t).Read(),
		`SELECT id, name FROM service_instance WHERE deleted_at IS NULL
		  ORDER BY priority DESC, name ASC`)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	if !strings.Contains(strings.Join(plan, " | "), "SCAN service_instance") {
		t.Errorf("service_instance is no longer planned as a scan: %v\n"+
			"schema.md §5 specifies no ordering index on this table and a homelab has "+
			"single-digit instances. If adding one is intended, say so in schema.md §5 "+
			"and update this test in the same change.", plan)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The three plans docs/reference/schema.md declares NORMATIVE, asserted on the
// exact fragment rather than on the index name, because the fragment is what
// those sections pin.
// ─────────────────────────────────────────────────────────────────────────────

// TestWorkGridKeysetPlanIsNormative pins schema.md §1's sentence in full:
//
//	SEARCH work USING INDEX ix_work_kind_sort (kind=? AND sort_title>?)
//
// SEARCH … USING INDEX, deliberately NOT COVERING INDEX. §1 says so explicitly:
// ix_work_kind_sort serves the keyset query, it does not cover it, because the
// SELECT list carries title, year, poster_asset_id, have_count, want_count and
// availability. 100 row lookups per page is fine; asserting COVERING would fail
// the moment a caller selects one more column, which is a change to the query
// and not an index regression.
//
// The `(kind=? AND sort_title>?)` half is the part worth pinning on its own: an
// index named in a plan that constrains only its leading column is a range scan
// over every movie, which is the same words and a different query.
func TestWorkGridKeysetPlanIsNormative(t *testing.T) {
	plan, err := QueryPlan(t.Context(), openTestDB(t).Read(),
		`SELECT title, year, poster_asset_id, have_count, want_count, availability
		   FROM work
		  WHERE kind = ? AND deleted_at IS NULL AND (sort_title, id) > (?, ?)
		  ORDER BY sort_title, id
		  LIMIT 100`, "movie", "", 0)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")

	const want = "SEARCH work USING INDEX ix_work_kind_sort (kind=? AND sort_title>?)"
	if !strings.Contains(joined, want) {
		t.Errorf("the library grid keyset plan is not schema.md §1's normative plan.\n"+
			"  got:  %s\n  want: %s", joined, want)
	}
	if strings.Contains(joined, "COVERING INDEX") {
		t.Errorf("the plan is now a COVERING index: %s\n"+
			"schema.md §1 states this query is served, not covered. If the SELECT list "+
			"shrank, change §1 in the same commit; do not pin COVERING here.", joined)
	}
	if strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("the keyset page needs a sort: %s", joined)
	}
}

// TestLibraryScopedKeysetIsASeek pins schema.md §13.3, which asks for the plan
// to be asserted in BOTH library topologies — one library per kind, and two
// libraries over one kind — "because only the second is the interesting one".
//
// §6.5's stated mitigation for scoped paging was that a library is single-kind
// and the default topology is one library per kind, so the common case is
// `work.kind = ?` with membership as a one-row lookup. That is false in the
// design's own flagship example: the reference install has seven libraries over
// six kinds, including two `book` libraries (the Audiobookshelf ebook/audiobook
// split §17.8 exists to demonstrate) and two `comic` libraries. Denormalising
// sort_title into library_member's key is what makes the scoped page a single
// covered seek regardless of selectivity, and this is the assertion that the
// key order still delivers it.
//
// ⚠️ Both subtests are built with real rows, and both produce the same plan —
// which is the honest result and is recorded rather than dressed up.
// EXPLAIN QUERY PLAN chooses from the schema, not from the data: with no
// ANALYZE statistics the topology cannot change the plan, only its selectivity.
// So what CI can assert is that the seek is on the primary key in both shapes;
// what it CANNOT assert is the row-count difference §13.3 is really worried
// about, and that belongs to `make bench`, which is not a merge gate.
func TestLibraryScopedKeysetIsASeek(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// Topology A: one library per kind. Topology B: two libraries over one
	// kind, sharing every work between them.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range []string{
			`INSERT INTO library (id, name, slug, kind) VALUES (1, 'Films', 'films', 'movie')`,
			`INSERT INTO library (id, name, slug, kind) VALUES (2, 'Ebooks', 'ebooks', 'book')`,
			`INSERT INTO library (id, name, slug, kind) VALUES (3, 'Audiobooks', 'audiobooks', 'book')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (1, 'movie', 'Train Dreams', 'train dreams', 'train dreams')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (2, 'book', 'Piranesi', 'piranesi', 'piranesi')`,
			`INSERT INTO edition (id, work_id, format) VALUES (1, 2, 'ebook')`,
			`INSERT INTO edition (id, work_id, format) VALUES (2, 2, 'audiobook')`,
			// Kind with no format axis: edition_id 0, the sentinel.
			`INSERT INTO library_member (library_id, sort_title, work_id, edition_id)
			   VALUES (1, 'train dreams', 1, 0)`,
			// One work, two libraries over one kind, one edition each.
			`INSERT INTO library_member (library_id, sort_title, work_id, edition_id)
			   VALUES (2, 'piranesi', 2, 1)`,
			`INSERT INTO library_member (library_id, sort_title, work_id, edition_id)
			   VALUES (3, 'piranesi', 2, 2)`,
		} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		libraryID int
	}{
		{"one library per kind", 1},
		{"two libraries over one kind", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := QueryPlan(ctx, d.Read(),
				`SELECT m.work_id, m.edition_id FROM library_member m
				  WHERE m.library_id = ? AND (m.sort_title, m.work_id) > (?, ?)
				  ORDER BY m.sort_title, m.work_id
				  LIMIT 100`, tc.libraryID, "", 0)
			if err != nil {
				t.Fatalf("QueryPlan: %v", err)
			}
			joined := strings.Join(plan, " | ")

			// schema.md §13.3 writes the expected plan as
			// `SEARCH m USING PRIMARY KEY (library_id=? AND sort_title>?)`.
			// MEASURED, the row-value comparison is reported as
			// `(library_id=? AND (sort_title,work_id)>(?,?))`. Same seek, and
			// §13.3 now says so; asserting the two halves separately is what
			// keeps this from breaking on a cosmetic change to SQLite's plan
			// text while still catching a leading-column-only scan.
			if !strings.Contains(joined, "SEARCH m USING PRIMARY KEY") {
				t.Errorf("the scoped keyset page is not a primary-key seek:\n  %s\n"+
					"sort_title is denormalised into library_member's key precisely so that "+
					"this stays a single covered seek regardless of the library's "+
					"selectivity. See schema.md §13.3.", joined)
			}
			if !strings.Contains(joined, "library_id=?") || !strings.Contains(joined, "sort_title") {
				t.Errorf("the seek does not constrain both library_id and sort_title:\n  %s\n"+
					"Constraining only the leading column is a range scan over the whole "+
					"library wearing the same plan text.", joined)
			}
			if strings.Contains(joined, "SCAN") {
				t.Errorf("the scoped keyset page contains a SCAN:\n  %s", joined)
			}
			if strings.Contains(joined, "TEMP B-TREE") {
				t.Errorf("the scoped keyset page needs a sort:\n  %s\n"+
					"That is the failure the denormalised sort_title exists to prevent: "+
					"without it the page must fetch and sort every member before the "+
					"window can be cut.", joined)
			}
		})
	}
}

// TestScopedSearchIsASeekNotAScan pins schema.md §7 invariant 6 — "the scoped
// search plan is a seek, not a scan".
//
// ⚠️ THE INVARIANT'S LITERAL TEXT IS NOT WHAT ANY REAL QUERY PRODUCES, and that
// is measured rather than worked around. §7 invariant 6 says the plan "must
// contain `SEARCH sdl USING PRIMARY KEY (library_id=? AND doc_rowid=?)`". No
// shape of the query yields that string, because which of the two indexes
// SQLite reaches for depends on which side drives the join:
//
//	doc set known (the RRF fusion — candidate rowids come from search_fts, and
//	the scope is applied per candidate):
//	  SEARCH sdl USING COVERING INDEX ix_sdl_doc (doc_rowid=? AND library_id=?)
//
//	scope leading (browse a scope with no query text):
//	  SEARCH sdl USING PRIMARY KEY (library_id=?)
//
// Both are seeks and neither is a scan, which is the invariant's substance and
// the whole reason the junction table replaced a JSON `library_scope` column:
// permission filtering must happen IN the index join, because a post-filter
// breaks keyset page sizes and leaks existence through result counts and
// ranking positions. §7 invariant 6 has been corrected to the two measured
// plans; this test asserts the substance in both directions so it cannot
// silently regress to a scan whichever way a future query is written.
func TestScopedSearchIsASeekNotAScan(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	for _, tc := range []struct {
		name  string
		query string
		args  []any
	}{
		{
			name: "doc set known — the fusion path",
			query: `SELECT sd.work_id, sd.popularity FROM search_doc sd
			          JOIN search_doc_library sdl
			            ON sdl.doc_rowid = sd.rowid AND sdl.library_id IN (?, ?)
			         WHERE sd.rowid IN (?, ?)`,
			args: []any{0, 1, 1, 2},
		},
		{
			name: "scope leading — browse with no query text",
			query: `SELECT sd.work_id FROM search_doc_library sdl
			          JOIN search_doc sd ON sd.rowid = sdl.doc_rowid
			         WHERE sdl.library_id IN (?, ?)
			         LIMIT 50`,
			args: []any{0, 1},
		},
		{
			name: "full RRF arm — FTS candidates, scoped in the join",
			query: `SELECT sd.work_id FROM search_fts f
			          JOIN search_doc sd ON sd.rowid = f.rowid
			          JOIN search_doc_library sdl
			            ON sdl.doc_rowid = sd.rowid AND sdl.library_id IN (?, ?)
			         WHERE search_fts MATCH ?
			         ORDER BY rank
			         LIMIT 50`,
			args: []any{0, 1, "train"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := QueryPlan(ctx, d.Read(), tc.query, tc.args...)
			if err != nil {
				t.Fatalf("QueryPlan: %v", err)
			}
			joined := strings.Join(plan, " | ")

			if !strings.Contains(joined, "SEARCH sdl") {
				t.Errorf("search_doc_library is not seeked:\n  %s", joined)
			}
			if strings.Contains(joined, "SCAN sdl") ||
				strings.Contains(joined, "SCAN search_doc_library") {
				t.Errorf("the scoped search SCANS search_doc_library:\n  %s\n"+
					"This is the regression the junction table exists to make impossible. "+
					"A scan here means permission filtering has moved out of the index "+
					"join, which breaks keyset page sizes and leaks existence through "+
					"result counts and ranking positions. schema.md §7 invariant 6.", joined)
			}
			if !strings.Contains(joined, "library_id=?") {
				t.Errorf("the seek does not constrain library_id:\n  %s", joined)
			}
		})
	}
}

// The §1.1 CI assertion — "no work_track row's edition belongs to a different
// work" — is NOT here, and its absence is deliberate rather than an oversight.
//
// work_track does not exist: each subtype table lands with the catalogue source
// that writes it (ADR-0040), and work_track's is Navidrome, which has no
// adapter — so the assertion has no table to run against. Writing it now
// against a missing table is either a query that errors or one that trivially
// returns no rows, and the second is worse — a green assertion nobody ever
// exercised.
//
// The books-and-comics half of that six is no longer deferred: migration 0006
// created work_book, work_comic and work_comic_issue, because ADR-0041 made
// Kavita — the source that writes them — v0.1's first adapter. None of the
// three carries an edition-scoped invariant of work_track's kind, so nothing
// moved out of this note with them.
//
// It lands with work_track, in the migration that creates it, together with
// §13.3's sibling assertion on library_member.edition_id. That one CAN be
// written today, because both tables exist:
//
//	SELECT m.library_id, m.work_id, m.edition_id FROM library_member m
//	 WHERE m.edition_id <> 0
//	   AND NOT EXISTS (SELECT 1 FROM edition e
//	                    WHERE e.id = m.edition_id AND e.work_id = m.work_id);
//
// It is asserted in TestLibraryMemberEditionsBelongToTheirWork below.
func TestLibraryMemberEditionsBelongToTheirWork(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// library_member.edition_id is a sentinel column with no foreign key: a
	// WITHOUT ROWID table's key columns are implicitly NOT NULL, so "the whole
	// work, format-independent" has to be 0 rather than NULL. The cost is that
	// integrity for the non-zero case is the derivation's job, not SQLite's,
	// which is exactly why it is asserted here.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range []string{
			`INSERT INTO library (id, name, slug, kind) VALUES (1, 'Ebooks', 'ebooks', 'book')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (1, 'book', 'Piranesi', 'piranesi', 'piranesi')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (2, 'book', 'Other', 'other', 'other')`,
			`INSERT INTO edition (id, work_id, format) VALUES (1, 1, 'ebook')`,
			`INSERT INTO library_member (library_id, sort_title, work_id, edition_id)
			   VALUES (1, 'piranesi', 1, 1)`,
			`INSERT INTO library_member (library_id, sort_title, work_id, edition_id)
			   VALUES (1, 'other', 2, 0)`,
		} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	check := func() int {
		t.Helper()
		var n int
		if err := d.Read().QueryRowContext(ctx, `
			SELECT count(*) FROM library_member m
			 WHERE m.edition_id <> 0
			   AND NOT EXISTS (SELECT 1 FROM edition e
			                    WHERE e.id = m.edition_id AND e.work_id = m.work_id)`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := check(); got != 0 {
		t.Errorf("%d library_member rows name an edition of a different work", got)
	}

	// Fire the assertion deliberately: a member row pointing at work 2 with
	// work 1's edition must be caught. Without this the check above is a query
	// that has only ever been run against data that satisfies it.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO library_member (library_id, sort_title, work_id, edition_id)
			  VALUES (1, 'other', 2, 1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := check(); got != 1 {
		t.Errorf("the cross-work edition assertion found %d violations, want 1 — "+
			"it does not detect the thing it exists to detect", got)
	}
}

// TestWriteQueueRunnableNeedsTheVerbatimINList pins a constraint that
// `ix_wq_runnable` imposes on every future caller and that nothing states:
// the index is PARTIAL, and SQLite matches a partial index's predicate
// SYNTACTICALLY rather than deriving implication.
//
// So the obvious way to write "claim the next runnable row" —
// `WHERE state = 'pending' AND next_attempt_at <= ?` — is a logically strict
// subset of the index's predicate and reaches the index NOT AT ALL. It full
// scans a table that grows with every command UsArr ever issues.
//
// Both facts are asserted, because pinning only the good one leaves the trap
// undocumented and pinning only the bad one reads as a bug report. Nothing
// queries write_queue yet (internal/httpapi/grabs.go:58), so this test is the
// only place the constraint is written in executable form — which is exactly
// why it is here and not left to the sweep's author to rediscover.
func TestWriteQueueRunnableNeedsTheVerbatimINList(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	seeks, err := QueryPlan(ctx, d.Read(),
		`SELECT id FROM write_queue
		  WHERE state IN ('pending','inflight','verifying') AND next_attempt_at <= ?`,
		"2026-08-17 00:00:00")
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	if j := strings.Join(seeks, " | "); !strings.Contains(j, "ix_wq_runnable") {
		t.Errorf("the verbatim three-state predicate no longer reaches ix_wq_runnable:\n  %s\n"+
			"That predicate is the sweep's, and reference/sync.md §4's reconciliation guard "+
			"names the same three states literally.", j)
	}

	scans, err := QueryPlan(ctx, d.Read(),
		`SELECT id FROM write_queue WHERE state = 'pending' AND next_attempt_at <= ?`,
		"2026-08-17 00:00:00")
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	if j := strings.Join(scans, " | "); !strings.Contains(j, "SCAN write_queue") {
		t.Errorf("the single-state predicate now reaches an index:\n  %s\n"+
			"That is good news and it makes the comment beside ix_wq_runnable in "+
			"00005_library_sync.sql wrong. Update both in the same change.", j)
	}
}
