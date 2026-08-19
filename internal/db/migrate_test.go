package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// schemaSnapshotPath is the checked-in expected schema. Regenerate with:
//
//	go test ./internal/db -run TestMigrationRoundTrip -update-schema
const schemaSnapshotPath = "testdata/schema.sql"

// updateSchema regenerates the snapshot instead of asserting against it.
var updateSchema = flagBool("update-schema", "rewrite testdata/schema.sql from the migrations")

// TestMigrationRoundTrip runs every migration against a fresh database and
// asserts the result matches the checked-in snapshot.
//
// This catches "works on my dev DB because it was created three migrations ago"
// drift: a migration edited after it shipped, or a column added straight to a
// dev database and never written into a migration, both show up here as a diff.
func TestMigrationRoundTrip(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	got, err := dumpSchema(ctx, d.Read())
	if err != nil {
		t.Fatalf("dump schema: %v", err)
	}

	if *updateSchema {
		if err := os.MkdirAll(filepath.Dir(schemaSnapshotPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(schemaSnapshotPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", schemaSnapshotPath)
		return
	}

	want, err := os.ReadFile(schemaSnapshotPath)
	if err != nil {
		t.Fatalf("read %s (regenerate with -update-schema): %v", schemaSnapshotPath, err)
	}
	if got != string(want) {
		t.Errorf("schema does not match %s.\n\ngot:\n%s\n\nwant:\n%s",
			schemaSnapshotPath, got, want)
	}
}

// latestSchemaVersion is the highest migration number in migrations/. It is
// asserted rather than derived so that adding a migration is a deliberate edit
// here too — a version the tests do not know about is a migration nobody
// round-tripped.
const latestSchemaVersion int64 = 11

// Down is not a supported user path, but it must work, because it is the
// cheapest way to test a migration locally. A Down that leaves objects behind
// makes the next Up fail in a way that only shows up on a developer's machine.
//
// MigrateDown rolls back exactly ONE migration, so this walks the whole stack
// down rather than calling it once: with two migrations, one Down leaves 0001's
// tables standing and would have asserted nothing about 0002's rollback.
func TestMigrationDownLeavesNothingBehind(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	v, err := d.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v != latestSchemaVersion {
		t.Fatalf("schema version = %d, want %d", v, latestSchemaVersion)
	}

	for v > 0 {
		if err := d.MigrateDown(ctx); err != nil {
			t.Fatalf("MigrateDown from %d: %v", v, err)
		}
		next, err := d.Version(ctx)
		if err != nil {
			t.Fatalf("Version: %v", err)
		}
		if next >= v {
			t.Fatalf("MigrateDown left the version at %d", next)
		}
		v = next
	}

	names, err := userObjects(ctx, d.Read())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("after Down, these objects remain: %v", names)
	}

	// And Up works again from the emptied database.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate after Down: %v", err)
	}
	if v, err = d.Version(ctx); err != nil || v != latestSchemaVersion {
		t.Fatalf("version after re-Up = %d (err %v), want %d", v, err, latestSchemaVersion)
	}
}

// TestMigration0002NeedsNoRebuild pins the observations 0002's header rests on,
// so that "no table rebuild is needed" stays a checked claim.
//
// It matters because the alternative — the 12-step rebuild SQLite requires for
// anything ADD COLUMN cannot express — would have to copy every provenance row,
// and provenance is the one table whose contents are permanent.
func TestMigration0002NeedsNoRebuild(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// 1. Foreign keys really are on. Everything below is only interesting under
	//    that pragma; with it off, the FK column is decoration.
	var fk int
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk)
	}); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d on the writer; migration 0002's FK reasoning assumes 1", fk)
	}

	// 2. Both tables are still STRICT after ADD COLUMN, and both new columns are
	//    NOT NULL with the sentinel default. TestAllTablesAreStrict covers the
	//    first for every table; this asserts the column shape.
	for _, table := range []string{"provenance", "release_candidate"} {
		var notNull int
		var dflt sql.NullString
		err := d.Read().QueryRowContext(ctx,
			`SELECT "notnull", dflt_value FROM pragma_table_info(?) WHERE name = 'user_id'`,
			table).Scan(&notNull, &dflt)
		if err != nil {
			t.Fatalf("%s has no user_id column: %v", table, err)
		}
		if notNull != 1 {
			t.Errorf("%s.user_id is nullable; the invariant is that every user-scoped row carries one", table)
		}
		if dflt.String != "0" {
			t.Errorf("%s.user_id defaults to %q, want the system sentinel 0", table, dflt.String)
		}
	}

	// 3. release_candidate.user_id's foreign key is enforced, and provenance's
	//    deliberately is not. This is the asymmetry the header argues for, and
	//    it is the kind of claim that rots silently.
	err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO service_instance (id, kind, name, base_url, api_key_enc)
			VALUES (1, 'prowlarr', 'p', 'http://p.lan', x'00')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	insertCandidate := func(userID int64) error {
		return d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO release_candidate (
				  service_instance_id, guid, title, raw_release_json, fetched_at, expires_at, user_id
				) VALUES (1, 'g', 't', '{}', '2026-08-16 00:00:00', '2026-08-16 00:25:00', ?)`, userID)
			return err
		})
	}
	if err := insertCandidate(0); err != nil {
		t.Errorf("release_candidate rejected the sentinel user 0: %v", err)
	}
	if err := insertCandidate(9999); err == nil {
		t.Error("release_candidate accepted a dangling user_id; its REFERENCES clause is not enforced")
	}

	// provenance takes a historical id that no longer resolves, exactly like
	// audit_log.actor_user_id. If this starts failing, someone added a foreign
	// key and made deleting a user who ever grabbed anything impossible.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO provenance (protocol, release_title, source_system, user_id)
			VALUES ('torrent', 'Some.Release', 'prowlarr', 9999)`)
		return err
	}); err != nil {
		t.Errorf("provenance refused a historical user id: %v\n"+
			"provenance.user_id must carry NO foreign key — it records who acquired the file, and "+
			"that must survive the user's deletion. See migration 0002's header.", err)
	}
}

// TestMigration0003NeedsNoRebuild pins 0003's header the same way, for a TEXT
// column rather than an INTEGER one, and checks the two things that make the
// column safe to rely on: the backfill really is the default, and the partial
// index really is partial.
//
// The rebuild it avoids would copy every provenance row, and provenance is the
// one table whose contents are permanent.
func TestMigration0003NeedsNoRebuild(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// 1. The column exists, is NOT NULL, and defaults to 'confirmed'. The
	//    default IS the backfill: SQLite rewrites the stored DDL and leaves the
	//    rows alone, which is what makes the migration free.
	var notNull int
	var dflt sql.NullString
	err := d.Read().QueryRowContext(ctx,
		`SELECT "notnull", dflt_value FROM pragma_table_info('provenance')
		  WHERE name = 'acquisition_state'`).Scan(&notNull, &dflt)
	if err != nil {
		t.Fatalf("provenance has no acquisition_state column: %v", err)
	}
	if notNull != 1 {
		t.Error("provenance.acquisition_state is nullable; a NULL state is neither confirmed nor not")
	}
	if dflt.String != "'confirmed'" {
		t.Errorf("acquisition_state defaults to %q, want 'confirmed' — the honest value for every "+
			"row written before 0003, all of which followed a 2xx", dflt.String)
	}

	// 2. provenance is still STRICT after the ADD COLUMN. TestAllTablesAreStrict
	//    covers every table; this pins the one 0003 touched, because losing
	//    STRICT here would silently accept an integer state.
	var strict int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT strict FROM pragma_table_list WHERE name = 'provenance'`).Scan(&strict); err != nil {
		t.Fatalf("reading pragma_table_list for provenance: %v", err)
	}
	if strict != 1 {
		t.Error("provenance is no longer STRICT after ADD COLUMN")
	}

	// 3. An insert that names no state gets 'confirmed', and one that names
	//    'unconfirmed' keeps it. There is deliberately NO CHECK constraint —
	//    SQLite cannot ALTER one and 0001's audit_log FK is what that costs —
	//    so the vocabulary is enforced in internal/store, not here.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO provenance (id, protocol, release_title, source_system, user_id)
			VALUES (1, 'torrent', 'Confirmed.Release', 'prowlarr', 0)`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO provenance (id, protocol, release_title, source_system, user_id, acquisition_state)
			VALUES (2, 'torrent', 'Unconfirmed.Release', 'prowlarr', 0, 'unconfirmed')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var state string
	if err := d.Read().QueryRowContext(ctx,
		`SELECT acquisition_state FROM provenance WHERE id = 1`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "confirmed" {
		t.Errorf("a row inserted without a state got %q, want confirmed", state)
	}

	// 4. The index is partial on exactly the unconfirmed rows. A non-partial
	//    index here would be a second full-size index on the table that grows
	//    forever, for a block that only ever shows the rare rows.
	var indexed int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_index_info('ix_prov_unconfirmed')`).Scan(&indexed); err != nil {
		t.Fatalf("ix_prov_unconfirmed does not exist: %v", err)
	}
	var partial bool
	if err := d.Read().QueryRowContext(ctx,
		`SELECT partial FROM pragma_index_list('provenance') WHERE name = 'ix_prov_unconfirmed'`).
		Scan(&partial); err != nil {
		t.Fatal(err)
	}
	if !partial {
		t.Error("ix_prov_unconfirmed is not partial; it would index every confirmed grab forever")
	}

	// And the planner actually reaches for it, rather than scanning provenance
	// to build the attention block.
	var plan string
	if err := d.Read().QueryRowContext(ctx,
		`EXPLAIN QUERY PLAN
		 SELECT id FROM provenance
		  WHERE user_id = 0 AND acquisition_state <> 'confirmed'
		  ORDER BY grabbed_at DESC, id DESC`).Scan(new(int), new(int), new(int), &plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "ix_prov_unconfirmed") {
		t.Errorf("the attention-block query does not use ix_prov_unconfirmed: %s", plan)
	}
}

// Deleting a user must stay possible. audit_log's comment in 0001 records that
// a foreign key on the actor made it impossible; 0002 adds two more user_id
// columns and this is the test that they did not reintroduce the problem.
func TestDeletingAUserStillWorks(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, auth_source) VALUES (7, 'joe', 'local')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO service_instance (id, kind, name, base_url, api_key_enc)
			VALUES (1, 'prowlarr', 'p', 'http://p.lan', x'00')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO release_candidate (
			  service_instance_id, guid, title, raw_release_json, fetched_at, expires_at, user_id
			) VALUES (1, 'g', 't', '{}', '2026-08-16 00:00:00', '2026-08-16 00:25:00', 7)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO provenance (protocol, release_title, source_system, user_id)
			VALUES ('torrent', 'Some.Release', 'prowlarr', 7)`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO audit_log (actor_user_id, action, result) VALUES (7, 'grab', 'ok')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user WHERE id = 7`)
		return err
	}); err != nil {
		t.Fatalf("deleting a user who had grabbed something failed: %v\n"+
			"this is the failure mode audit_log's comment in migration 0001 documents; "+
			"check the foreign keys added by 0002", err)
	}

	// The candidate cascaded away; the provenance row and the audit row stayed,
	// still naming the user who did it.
	var candidates, provenances, audits int
	row := d.Read().QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM release_candidate WHERE user_id = 7),
		       (SELECT count(*) FROM provenance        WHERE user_id = 7),
		       (SELECT count(*) FROM audit_log         WHERE actor_user_id = 7)`)
	if err := row.Scan(&candidates, &provenances, &audits); err != nil {
		t.Fatal(err)
	}
	if candidates != 0 {
		t.Error("release_candidate rows survived the user's deletion; ON DELETE CASCADE is not doing its job")
	}
	if provenances != 1 {
		t.Error("the provenance row was destroyed with the user. Acquisition history is the one thing " +
			"that table exists to keep — see migration 0002's header.")
	}
	if audits != 1 {
		t.Error("the audit row lost its actor")
	}
}

// Migration 0001 must be idempotent at the provider level: running it twice is
// what every restart does.
func TestMigrateIsIdempotent(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	before, err := dumpSchema(ctx, d.Read())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	after, err := dumpSchema(ctx, d.Read())
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Error("running the migrations twice changed the schema")
	}
}

// The system sentinel must exist from migration 0001: tag_assignment.user_id is
// NOT NULL DEFAULT 0 REFERENCES user(id), so without the row no shared tag can
// be written at all.
func TestSystemSentinelUserExists(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	var username string
	var disabled bool
	err := d.Read().QueryRowContext(ctx,
		`SELECT username, is_disabled FROM user WHERE id = 0`).Scan(&username, &disabled)
	if err != nil {
		t.Fatalf("the id-0 sentinel row is missing: %v", err)
	}
	if !disabled {
		t.Error("the sentinel user is not disabled; it must never be a login target")
	}

	// A shared tag assignment resolves its foreign key against it.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag (id, namespace, value) VALUES (1, 'system', 'shared')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tag_assignment (tag_id, work_id, source) VALUES (1, 100, 'system')`)
		return err
	}); err != nil {
		t.Fatalf("shared tag assignment against the sentinel failed: %v", err)
	}
}

// The tables deferred past the library-sync migration must NOT be here, and the
// ones it ships must. A migration that creates a table nothing queries is a
// schema claim nobody has tested — 00001's own rule, and the one 00005 quotes
// when it leaves the unbuilt subtype tables out. 0006 moved three of the six
// across, because Kavita — the source that writes them — became v0.1's first
// adapter (ADR-0041); the same rule decided both.
//
// BOTH LISTS ARE EXHAUSTIVE ON PURPOSE, and they were not before 0005: the
// deferred list named neither the music/books/comics subtype tables nor the
// v0.2/v1.0 appendix tables, and the present list omitted indexer_catalog,
// search_trgm, search_doc_library and all four library tables — so it asserted
// nothing about the objects most likely to be forgotten. A table that belongs
// to neither list is caught by the final loop rather than passing silently.
func TestDeferredTablesAreAbsent(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	names, err := userObjects(ctx, d.Read())
	if err != nil {
		t.Fatal(err)
	}
	present := map[string]bool{}
	for _, n := range names {
		present[n] = true
	}

	// Deferred, with the milestone that owns each. The work_* subtypes wait for
	// the catalogue source that writes them (ADR-0040); the rest is schema.md's
	// "later tables" appendix.
	//
	// THREE OF THE SIX MOVED OUT OF THIS LIST IN 0006 AND THE LIST ITSELF IS THE
	// RECORD OF WHY: ADR-0041 made Kavita v0.1's first adapter, Kavita writes
	// books and comics/manga, and ADR-0040's rule — the landing point is the
	// source, not the date — put work_book, work_comic and work_comic_issue in
	// `want` below. The music three did not move: Navidrome still has no
	// adapter in internal/.
	deferred := []string{
		// music — lands with Navidrome (§16.1 position 1 or 2). work_credit was
		// on this line until 0007 and is NOT any more: ADR-0044 applied
		// ADR-0040's own rule — the landing point is the source that writes the
		// table — and noticed that Kavita writes credits (writers, cover
		// artists, pencillers, inkers, colorists, letterers, editors,
		// translators) while Navidrome writes albums and tracks. So work_credit
		// moved to `want`; work_album and work_track did not move, because an
		// album and a track are still music objects with no v0.1 writer.
		"work_album", "work_track",
		// v0.2
		"request", "request_quota",
		// v0.3
		"work_relation",
		// v1.0
		"work_merge", "tag_rule", "tag_alias", "tag_implies", "saved_filter",
		"playback_state", "play_history", "playlist", "playlist_item",
		"role", "role_permission", "user_role", "user_permission", "user_library_access",
	}
	for _, name := range deferred {
		if present[name] {
			t.Errorf("%s exists, but no shipped migration should create it; "+
				"see 00005's and 00006's headers for which source owns it", name)
		}
	}

	// Present, by the migration that creates it.
	want := []string{
		// 0001
		"user", "session", "client_credential", "audit_log", "service_instance",
		"release_candidate", "provenance", "write_queue", "tag", "tag_assignment",
		// 0004
		"indexer_catalog",
		// 0005
		"image_asset", "work", "work_movie", "work_series", "work_episode",
		"work_alt_title", "edition", "media_file", "external_id",
		"service_item_link", "service_item_alias",
		"library", "library_source", "library_member", "library_override",
		"search_doc", "search_fts", "search_trgm", "search_doc_library",
		"sync_report",
		// 0006 — the subtype tables Kavita writes (ADR-0040 + ADR-0041)
		"work_book", "work_comic", "work_comic_issue",
		// 0007 — the credit table Kavita also writes (ADR-0040 + ADR-0044)
		"work_credit",
	}
	for _, name := range want {
		if !present[name] {
			t.Errorf("%s is missing; it should be created by migrations 0001-0007", name)
		}
	}

	// Neither list, and therefore unasserted. Indexes, triggers and FTS5's own
	// shadow tables are not tables anyone declared, so they are excluded by
	// asking pragma_table_list for the type rather than by matching on a name.
	//
	// `type IN ('table','virtual')` and not `type = 'table'`: search_fts and
	// search_trgm are VIRTUAL, and with the narrower filter this loop skipped
	// them — which was checked by firing it, not assumed. Deleting search_trgm
	// from `want` left the whole test green, so the two objects most easily
	// forgotten were the two it could not see. That is the proxy-instead-of-
	// condition failure DEVELOPMENT.md §11 rule 1 describes, caught by rule 3.
	known := map[string]bool{}
	for _, n := range append(append([]string{}, deferred...), want...) {
		known[n] = true
	}
	rows, err := d.Read().QueryContext(ctx, `
		SELECT name FROM pragma_table_list
		 WHERE type IN ('table', 'virtual')
		   AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if !known[name] {
			t.Errorf("table %s is in neither list in this test, so nothing here asserts "+
				"whether it should exist. Add it to `want` (and say which migration) or to "+
				"`deferred` (and say which milestone).", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// Every ordinary table is STRICT. A non-STRICT table accepts a string into an
// INTEGER column, which is how a type bug reaches disk.
//
// This asks pragma_table_list for each object's TYPE rather than string-matching
// sqlite_schema, because migration 0005 introduces two kinds of object for which
// "does the DDL contain STRICT" is the wrong question and would have been
// answered wrong: `search_fts` and `search_trgm` are VIRTUAL tables, and SQLite
// has no STRICT for one; and FTS5 creates four `shadow` tables of its own per
// virtual table, whose DDL this project does not write. Both appear in
// sqlite_schema as type='table'. Excluding them by name pattern would have been
// the proxy-instead-of-condition mistake DEVELOPMENT.md §11 rule 1 describes —
// pragma_table_list reports the distinction directly.
func TestAllTablesAreStrict(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	rows, err := d.Read().QueryContext(ctx, `
		SELECT name, type, strict FROM pragma_table_list
		 WHERE name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	var ordinary int
	for rows.Next() {
		var name, objType string
		var strict int
		if err := rows.Scan(&name, &objType, &strict); err != nil {
			t.Fatal(err)
		}
		if objType != "table" {
			continue // virtual, shadow or view
		}
		ordinary++
		if strict != 1 {
			t.Errorf("table %s is not STRICT", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	// Guard the guard: if pragma_table_list ever stops reporting `type`, or the
	// filter above turns into a no-op, this loop would silently assert nothing.
	if ordinary < 20 {
		t.Errorf("only %d ordinary tables were examined; the schema has more than that, "+
			"so this test is filtering out rows it should be checking", ordinary)
	}
}

// FTS5 is a REGISTERED EXTENSION in this driver build, not a built-in, and
// registration is per-connection exactly as the pragmas are. Measured, not
// assumed: on ncruces/go-sqlite3 v0.35.3 a connection opened without
// ext/fts5.Register answers `CREATE VIRTUAL TABLE … USING fts5(…)` with "no
// such module: fts5".
//
// Migration 0005 creates search_fts and search_trgm, so a pool that lost the
// registration could not migrate at all — but a pool that lost it on only SOME
// connections would fail intermittently, at query time, arbitrarily far from
// the cause. That is the same failure shape TestPragmasOnEveryConnection exists
// for, so this is checked the same way: across every connection of the read
// pool, held open simultaneously so the pool really hands out distinct ones.
func TestFTS5IsAvailableOnEveryConnection(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	readers := d.Read().Stats().MaxOpenConnections
	if readers < 2 {
		t.Fatalf("read pool max connections = %d, want NumCPU*2", readers)
	}

	var wg sync.WaitGroup
	release := make(chan struct{})
	errs := make(chan error, readers+1)

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

			// A MATCH against the contentless table is the cheapest statement
			// that cannot be planned at all without the fts5 module loaded.
			var n int
			if err := conn.QueryRowContext(ctx,
				`SELECT count(*) FROM search_fts WHERE search_fts MATCH 'usarr'`).Scan(&n); err != nil {
				errs <- fmt.Errorf("read connection %d has no fts5: %w", i, err)
				return
			}
			<-release
		}()
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("%v\nfts5 must be registered on every connection of BOTH pools — see "+
			"internal/db.Open. Without it search_fts and search_trgm are unreadable.", err)
	}

	// contentless_delete=1 needs SQLite >= 3.43.0. The DELETE is the whole
	// point of it: a plain contentless FTS5 table answers "cannot DELETE from
	// contentless fts5 table", and without a working DELETE §7 invariant 2
	// (count(search_fts) == count(search_trgm) == count(search_doc)) cannot be
	// maintained across a work delete or a title change.
	var version string
	if err := d.Read().QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO search_fts (rowid, title) VALUES (1, 'Train Dreams')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO search_trgm (rowid, title) VALUES (1, 'Train Dreams')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_fts WHERE rowid = 1`); err != nil {
			return fmt.Errorf("search_fts: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_trgm WHERE rowid = 1`); err != nil {
			return fmt.Errorf("search_trgm: %w", err)
		}
		return nil
	}); err != nil {
		t.Errorf("contentless_delete=1 is not in effect on SQLite %s: %v", version, err)
	}
}

// TestMigrate0005WriteQueueRebuild proves the 12-step rebuild did what its
// header claims, by reading the result back rather than trusting that the
// CREATE statements ran.
//
// Everything here is a claim that rots silently: an index that a rebuilt table
// does not inherit, a partial predicate quietly recreated as a full one (which
// changes which rows the reconciliation sweep sees), or a foreign-key violation
// left behind by the copy.
func TestMigrate0005WriteQueueRebuild(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// 1. Still STRICT after the rebuild.
	var strict int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT strict FROM pragma_table_list WHERE name = 'write_queue'`).Scan(&strict); err != nil {
		t.Fatalf("reading pragma_table_list for write_queue: %v", err)
	}
	if strict != 1 {
		t.Error("write_queue is not STRICT after the rebuild")
	}

	// 2. ALL THREE indexes exist, and ix_wq_runnable is still PARTIAL on
	//    exactly ('pending','inflight','verifying'). A rebuilt table inherits
	//    no indexes; a partial index silently recreated as a full one changes
	//    which rows the sweep and the reconciliation guard see.
	ddl := func() map[string]string {
		out := map[string]string{}
		rows, err := d.Read().QueryContext(ctx, `
			SELECT name, sql FROM sqlite_schema WHERE type = 'index' AND tbl_name = 'write_queue'`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var name string
			var s sql.NullString
			if err := rows.Scan(&name, &s); err != nil {
				t.Fatal(err)
			}
			out[name] = s.String
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}()

	for _, name := range []string{"ux_wq_idem", "ix_wq_work", "ix_wq_runnable"} {
		if _, ok := ddl[name]; !ok {
			t.Errorf("%s did not survive the rebuild; a rebuilt table inherits no indexes", name)
		}
	}
	if got := ddl["ix_wq_runnable"]; !strings.Contains(got, "WHERE state IN ('pending','inflight','verifying')") {
		t.Errorf("ix_wq_runnable's partial predicate did not survive the rebuild:\n  %s\n"+
			"It must stay byte-identical to 00001's. 'awaiting_choice' is deliberately NOT in "+
			"it — a row waiting on a person is not runnable, and listing it here would expose "+
			"it to the retry sweep and the verify_until TTL. See 00005's header, decision (b).", got)
	}
	var partial int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT partial FROM pragma_index_list('write_queue') WHERE name = 'ix_wq_runnable'`).
		Scan(&partial); err != nil {
		t.Fatal(err)
	}
	if partial != 1 {
		t.Error("ix_wq_runnable is no longer partial")
	}

	// 3. The `state` CHECK is GONE (00005 decision (a)) and `fail_reason`'s is
	//    KEPT. Both halves matter: dropping fail_reason's too would delete the
	//    DB-01 regression witness.
	var table string
	if err := d.Read().QueryRowContext(ctx,
		`SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'write_queue'`).
		Scan(&table); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(table, "CHECK (state IN") {
		t.Error("write_queue.state still carries a CHECK. 00005 drops it deliberately: the " +
			"lifecycle vocabulary is still growing, SQLite cannot ALTER a CHECK, and " +
			"audit_log.result and provenance.acquisition_state are the shipped precedent " +
			"for enforcing that class of vocabulary in Go.")
	}
	if !strings.Contains(table, "fail_reason IS NULL OR fail_reason IN") {
		t.Error("write_queue.fail_reason lost its CHECK, or lost the `IS NULL OR` form. " +
			"That vocabulary is closed, and this column is DB-01's regression witness.")
	}

	// And the behaviour, not just the DDL: a state 00001 would have rejected is
	// now accepted, because the enforcement moved to Go.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO write_queue (idempotency_key, kind, payload, state)
			VALUES ('k-awaiting', 'grab', '{}', 'awaiting_choice')`)
		return err
	}); err != nil {
		t.Errorf("write_queue rejected state='awaiting_choice': %v\n"+
			"That is the state a two-phase sink parks in while a person decides "+
			"(FUTURE.md §11). 00005 drops the CHECK so it needs no further migration.", err)
	}

	// 4. Step 10 of the 12-step procedure, run where its result can fail a
	//    build. The migration deliberately does not write it: goose discards a
	//    PRAGMA's rows, so it would have been decoration.
	fk, err := d.Read().QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fk.Close() }()
	var violations int
	for fk.Next() {
		violations++
	}
	if err := fk.Err(); err != nil {
		t.Fatal(err)
	}
	if violations != 0 {
		t.Errorf("PRAGMA foreign_key_check reports %d violations after the rebuild", violations)
	}
}

// TestMigrate0005WorkIDForeignKey proves decision (d) by execution rather than
// by argument: write_queue.work_id gets `REFERENCES work(id) ON DELETE CASCADE`
// back, the reference 00001 dropped with a comment naming this migration.
//
// The reason it is proven rather than reasoned about is that the argument
// against it — the audit_log precedent, where an ON DELETE action is an
// implicit write that a trigger aborts, making a user undeletable — is exactly
// the kind of thing that looks fine in a comment and fails at runtime. So the
// last assertion re-runs that failure mode directly.
func TestMigrate0005WorkIDForeignKey(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// 1. A dangling work_id is now rejected. Before the rebuild it was a bare
	//    INTEGER and this insert succeeded, leaving ix_wq_work returning rows
	//    for a work that does not exist.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO write_queue (idempotency_key, kind, work_id, payload)
			VALUES ('k-dangling', 'monitor', 9999, '{}')`)
		return err
	}); err == nil {
		t.Error("write_queue accepted a work_id with no work row; the restored REFERENCES " +
			"clause is not enforced")
	}

	// 2. A real work_id is accepted, and deleting the work takes the queued
	//    command with it. This is the behaviour the header argues for and the
	//    one a reader is most likely to be surprised by, so it is asserted
	//    rather than described.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title)
			VALUES (42, 'movie', 'Train Dreams', 'train dreams', 'train dreams')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO write_queue (idempotency_key, kind, work_id, payload)
			VALUES ('k-real', 'monitor', 42, '{}')`)
		return err
	}); err != nil {
		t.Fatalf("enqueueing against a real work failed: %v", err)
	}

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM work WHERE id = 42`)
		return err
	}); err != nil {
		t.Fatalf("deleting a work with a queued command failed: %v\n"+
			"ON DELETE CASCADE was chosen over RESTRICT precisely so this stays possible: "+
			"a tombstone expiry that cannot delete is a sweep that stalls forever.", err)
	}

	var queued int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM write_queue WHERE idempotency_key = 'k-real'`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Error("the queued command outlived its work; ON DELETE CASCADE is not doing its job")
	}

	// 3. The audit_log failure mode, re-run against the rebuilt table. A user
	//    who has queued something must still be deletable — the new foreign key
	//    is on work_id, but the rebuild also re-declares user_id's, and a
	//    rebuild is exactly where a clause gets copied wrong.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user (id, username, auth_source) VALUES (7, 'joe', 'local')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO write_queue (idempotency_key, user_id, kind, payload)
			VALUES ('k-user', 7, 'grab', '{}')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO audit_log (actor_user_id, action, result) VALUES (7, 'grab', 'ok')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user WHERE id = 7`)
		return err
	}); err != nil {
		t.Fatalf("deleting a user with a queued command failed after the rebuild: %v", err)
	}
}

// The reserved `library.id = 0` row, "Unfiled", must exist from the migration
// that creates the table.
//
// It is what upholds schema.md §7 invariant 5: every search_doc row is visible
// through at least one library, because a work the membership derivation would
// otherwise place in NO library is a member of Unfiled. Without the row that
// invariant has no mechanism, and a work visible through no library matches no
// scope — it disappears from search for everyone, including its owner.
func TestUnfiledLibraryExists(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	var name, slug, managedBy string
	var userID int64
	if err := d.Read().QueryRowContext(ctx,
		`SELECT name, slug, managed_by, user_id FROM library WHERE id = 0`).
		Scan(&name, &slug, &managedBy, &userID); err != nil {
		t.Fatalf("the reserved library.id = 0 row is missing: %v", err)
	}
	if name != "Unfiled" || slug != "unfiled" {
		t.Errorf("library 0 is %q/%q, want Unfiled/unfiled", name, slug)
	}
	if userID != 0 {
		t.Errorf("library 0 belongs to user %d, want the sentinel 0", userID)
	}
	if managedBy != "auto" {
		t.Errorf("library 0 is managed_by %q, want auto", managedBy)
	}

	// It has to be usable as a membership target and as a search scope, which
	// is the only thing it is for.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title)
			VALUES (1, 'movie', 'Orphan', 'orphan', 'orphan')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO library_member (library_id, sort_title, work_id) VALUES (0, 'orphan', 1)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO search_doc (rowid, work_id, kind, norm_title) VALUES (1, 1, 'movie', 'orphan')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO search_doc_library (library_id, doc_rowid) VALUES (0, 1)`)
		return err
	}); err != nil {
		t.Fatalf("filing a work into Unfiled failed: %v", err)
	}

	// §7 invariant 5, asserted on the fixture just written.
	var invisible int
	if err := d.Read().QueryRowContext(ctx, `
		SELECT count(*) FROM search_doc sd
		 WHERE NOT EXISTS (SELECT 1 FROM search_doc_library sdl WHERE sdl.doc_rowid = sd.rowid)`).
		Scan(&invisible); err != nil {
		t.Fatal(err)
	}
	if invisible != 0 {
		t.Errorf("%d search_doc rows are visible through no library", invisible)
	}
}

// TestNullableCheckConstraintsActuallyConstrain is the regression test for DB-01.
//
// `write_queue.fail_reason` read `CHECK (fail_reason IN (NULL,'rejected',...))`,
// which enforced nothing: `x IN (NULL, 'a')` evaluates to NULL — not FALSE —
// when x matches no entry, and a CHECK passes when its expression is NULL. One
// NULL in the list makes every value legal. The constraint read as enforcement
// and enforced nothing, in a migration that is never edited once merged.
//
// `library_override.field_name` is the SECOND instance of the same defect,
// found while writing migration 0005: schema.md §13.4 specified it as
// `CHECK (field_name IN (NULL,'title','sort_title','year','cover'))`, which is
// DB-01 character for character. 0005 creates it in the `IS NULL OR …` form and
// this test is what says so.
//
// The control used to be `write_queue.state` — the identical pattern WITHOUT
// NULL in the list. Migration 0005 drops that CHECK deliberately (see its
// header, decision (a)), so the control moved to `provenance.protocol`, which
// is the same shape on a NOT NULL column. If only the control passes, the
// nullable form has regressed; if the control fails, CHECK enforcement is
// broken generally and nothing else in this test means anything.
func TestNullableCheckConstraintsActuallyConstrain(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	queue := func(t *testing.T, id int, col, val string) error {
		t.Helper()
		q := fmt.Sprintf(
			`INSERT INTO write_queue (id, idempotency_key, kind, payload, %s) VALUES (?, ?, 'grab', '{}', %s)`,
			col, val)
		return d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, q, id, fmt.Sprintf("k%d", id))
			return err
		})
	}

	// The legal values, including NULL, must still be accepted.
	for i, v := range []string{"'rejected'", "'unknown'", "'exhausted'", "NULL"} {
		if err := queue(t, 10+i, "fail_reason", v); err != nil {
			t.Errorf("fail_reason=%s was rejected but is legal: %v", v, err)
		}
	}

	// Garbage must not be.
	if err := queue(t, 20, "fail_reason", "'TOTAL-GARBAGE'"); err == nil {
		t.Error("fail_reason='TOTAL-GARBAGE' was ACCEPTED — the CHECK is a no-op.\n" +
			"A nullable column must use `CHECK (col IS NULL OR col IN (...))`; " +
			"putting NULL inside the IN list poisons the comparison and accepts everything.")
	}

	// The same defect class on migration 0005's own nullable CHECK. verb='field'
	// is required by the adjacent CHECK, so the rows below differ only in
	// field_name.
	override := func(t *testing.T, id int, fieldName string) error {
		t.Helper()
		q := fmt.Sprintf(`
			INSERT INTO library_override (id, verb, work_id, target_identity_hash, field_name)
			VALUES (?, 'field', 1, 'h', %s)`, fieldName)
		return d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, q, id)
			return err
		})
	}
	for i, v := range []string{"'title'", "'sort_title'", "'year'", "'cover'"} {
		if err := override(t, 30+i, v); err != nil {
			t.Errorf("library_override.field_name=%s was rejected but is legal: %v", v, err)
		}
	}
	if err := override(t, 40, "'TOTAL-GARBAGE'"); err == nil {
		t.Error("library_override.field_name='TOTAL-GARBAGE' was ACCEPTED — the CHECK is a no-op.\n" +
			"docs/reference/schema.md §13.4 specified this column as " +
			"`CHECK (field_name IN (NULL,'title',...))`, which is DB-01 verbatim. " +
			"Migration 0005 writes the `IS NULL OR ... IN (...)` form; if this fires, " +
			"someone copied the spec back in.")
	}
	// NULL must still be legal for the two global verbs, which is the whole
	// reason the column is nullable.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO library_override (id, verb, library_id, work_id, target_identity_hash)
			VALUES (41, 'exclude', 0, 1, 'h')`)
		return err
	}); err != nil {
		t.Errorf("an exclude override with field_name NULL was rejected: %v", err)
	}

	// Control: the same pattern on a NOT NULL column, with no NULL in the list.
	// If this fails, CHECK constraints are not being enforced at all and every
	// assertion above is vacuous.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO provenance (protocol, release_title, source_system)
			VALUES ('TOTAL-GARBAGE', 'Some.Release', 'prowlarr')`)
		return err
	}); err == nil {
		t.Error("provenance.protocol='TOTAL-GARBAGE' was accepted; CHECK constraints are not " +
			"being enforced at all")
	}
}

// dumpSchema renders the schema in a stable, diffable order.
func dumpSchema(ctx context.Context, q *sql.DB) (string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT type, name, sql FROM sqlite_schema
		 WHERE sql IS NOT NULL
		   AND name NOT LIKE 'sqlite_%'
		   AND name NOT LIKE 'goose_%'
		 ORDER BY type, name`)
	if err != nil {
		return "", fmt.Errorf("read sqlite_schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var b strings.Builder
	for rows.Next() {
		var objType, name, ddl string
		if err := rows.Scan(&objType, &name, &ddl); err != nil {
			return "", fmt.Errorf("read sqlite_schema: scan: %w", err)
		}
		fmt.Fprintf(&b, "-- %s %s\n%s;\n\n", objType, name, strings.TrimSpace(ddl))
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read sqlite_schema: %w", err)
	}
	return b.String(), nil
}

// schemaDump is a schema dump together with the migration version it was read
// at.
//
// The version travels with the text because the round-trips below compare two
// dumps and a bare string does not say which version it is: comparing a dump
// read at 10 with one read at 11 produces a difference that reads as "the Down
// block is broken" when the real fault is that unlike things were compared.
// sameAs turns that into its own, loud failure.
type schemaDump struct {
	version int64
	ddl     string
}

// dumpSchemaAt steps the database down to `version` and dumps the schema there.
//
// The version is a REQUIRED argument and the step down is the helper's job
// rather than the caller's, because the failure being removed is an OMISSION.
// openTestDB migrates to LATEST, so a dump taken without stepping back to the
// migration under test captures whatever landed after it, and the "removed or
// changed an object it did not create" comparison then blames THIS migration's
// Down block for rolling back a later one. It is green when written and wrong
// the moment a stranger lands the next migration.
//
// That is measured history, not a hypothetical: 0009's round-trip lacked the
// step and 0010 broke it on all three of 0010's indexes. A COMMENT DID NOT STOP
// THE REPEAT — 0010's author wrote out "the line 0011 will need" in so many
// words and 0011 still landed without it. So the step is no longer something a
// caller can forget to write: there is no way to take a dump without naming the
// version it is taken at.
//
// Stepping is idempotent, so a caller that has already walked down to `version`
// for its own assertions may still dump through this helper; migrateDownTo
// fails loudly if the database is BELOW the version asked for.
func dumpSchemaAt(t *testing.T, ctx context.Context, d *DB, version int64) schemaDump {
	t.Helper()
	migrateDownTo(t, ctx, d, version)
	ddl, err := dumpSchema(ctx, d.Read())
	if err != nil {
		t.Fatal(err)
	}
	return schemaDump{version: version, ddl: ddl}
}

// sameAs reports whether two dumps hold identical DDL, and FAILS THE TEST
// OUTRIGHT if they were read at different versions rather than reporting them
// as unequal.
//
// The helper above removes one of this comparison's two failure modes — a dump
// taken at head instead of at the migration under test. It cannot remove the
// other, which is naming two different versions across the two calls, because
// each call is individually well-formed. A version mismatch is never a
// meaningful comparison, so it is not reported as a schema difference.
//
// Deliberate cross-version comparisons — "everything at N that this Down block
// did not create is still there at N-1" — read .ddl directly and do not come
// through here.
func (s schemaDump) sameAs(t *testing.T, other schemaDump) bool {
	t.Helper()
	if s.version != other.version {
		t.Fatalf("comparing a schema dump read at version %d with one read at version %d: "+
			"these are not comparable, and the difference between them says nothing about "+
			"any Down block", s.version, other.version)
	}
	return s.ddl == other.ddl
}

// TestMigration0004NeedsNoRebuild pins 0004's header, and pins the one thing
// about indexer_catalog that is a security property rather than a performance
// one: ITS COLUMN LIST IS AN ALLOWLIST.
//
// Prowlarr's IndexerResource carries fields[] — a private tracker's passkey,
// RSS key, API key and session cookie. UsArr closes that leak class
// structurally, by never having anywhere to put one, and the cheapest place for
// that to rot is a well-meaning "let's also keep the raw resource so we don't
// have to re-fetch it" column. This test is what makes adding one a deliberate
// act rather than an oversight.
func TestMigration0004NeedsNoRebuild(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// 1. The exact column set of indexer_catalog. Not "these columns exist" —
	//    the WHOLE set, so a new one fails here and has to be argued for.
	want := []string{
		"service_instance_id", "indexer_id", "name", "protocol", "privacy",
		"enabled", "priority", "supports_search", "supports_rss",
		"supports_pagination", "search_types", "limits_max", "limits_default",
		"categories", "fetched_at",
	}
	got := func() []string {
		rows, err := d.Read().QueryContext(ctx,
			`SELECT name FROM pragma_table_info('indexer_catalog') ORDER BY cid`)
		if err != nil {
			t.Fatalf("indexer_catalog does not exist: %v", err)
		}
		defer func() { _ = rows.Close() }()

		var names []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return names
	}()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("indexer_catalog columns = %v\nwant %v\n"+
			"This table is an ALLOWLIST: a credential from Prowlarr's fields[] can only reach "+
			"SQLite if a named column puts it there. Adding a column — above all a raw-resource "+
			"or JSON-blob column — reopens the leak class migration 0004 closed. If the addition "+
			"is genuinely wanted, say so in 0004's successor and update this list in the same "+
			"change.", got, want)
	}

	// 2. indexer_catalog is STRICT, so a passkey cannot arrive as an untyped
	//    blob in an INTEGER column either.
	var strict int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT strict FROM pragma_table_list WHERE name = 'indexer_catalog'`).Scan(&strict); err != nil {
		t.Fatalf("reading pragma_table_list for indexer_catalog: %v", err)
	}
	if strict != 1 {
		t.Error("indexer_catalog is not STRICT")
	}

	// 3. service_instance survived ADD COLUMN as a STRICT table — the claim
	//    0004's header rests on, checked rather than remembered.
	if err := d.Read().QueryRowContext(ctx,
		`SELECT strict FROM pragma_table_list WHERE name = 'service_instance'`).Scan(&strict); err != nil {
		t.Fatalf("reading pragma_table_list for service_instance: %v", err)
	}
	if strict != 1 {
		t.Error("service_instance is no longer STRICT after ADD COLUMN")
	}

	// 4. indexers_fetched_at is NULLABLE with no default, and that is the whole
	//    mechanism: NULL means "never successfully read", which is a different
	//    state from "read, and this service has zero indexers". A NOT NULL
	//    column with a default would collapse the two and make the endpoint
	//    say "you have no indexers" to someone whose Prowlarr UsArr has simply
	//    never reached.
	var notNull int
	var dflt sql.NullString
	if err := d.Read().QueryRowContext(ctx,
		`SELECT "notnull", dflt_value FROM pragma_table_info('service_instance')
		  WHERE name = 'indexers_fetched_at'`).Scan(&notNull, &dflt); err != nil {
		t.Fatalf("service_instance has no indexers_fetched_at column: %v", err)
	}
	if notNull != 0 {
		t.Error("indexers_fetched_at is NOT NULL; there would then be no way to say 'never read'")
	}
	if dflt.Valid {
		t.Errorf("indexers_fetched_at defaults to %q; a default is a claim of a fetch that did not happen", dflt.String)
	}

	// 5. The foreign key is enforced, so a catalogue row cannot outlive the
	//    service it replicates. Foreign keys really are on — 0002 pins that —
	//    and this is the insert that would silently succeed if they were not.
	err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO indexer_catalog (service_instance_id, indexer_id, name, fetched_at)
			VALUES (999, 1, 'Orphan', '2026-08-16 00:00:00')`)
		return err
	})
	if err == nil {
		t.Error("a catalogue row for a non-existent service was accepted")
	}
}

// userObjects lists every non-internal schema object.
func userObjects(ctx context.Context, q *sql.DB) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT name FROM sqlite_schema
		 WHERE name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'
		 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite_schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("read sqlite_schema: scan: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sqlite_schema: %w", err)
	}
	return out, nil
}

// ─── The write_queue COPY, which no test above exercises ─────────────────────
//
// TestMigrate0005WriteQueueRebuild proves the rebuilt SHAPE. It cannot prove
// anything about the COPY, and that is the deeper defect it hid: it runs
// against openTestDB, which migrates 1→5 over a fresh EMPTY database, so the
// one step of the 12 that can lose data — `INSERT … SELECT … FROM write_queue`
// — moves zero rows in every test in this file.
//
// The two tests below migrate to 0004 first, put rows in, and then migrate.
//
// They are TWO tests rather than one because the resolution of the finding
// makes a single "copy everything and diff every column" test unwritable: a
// non-NULL work_id is dangling by construction (0005 creates `work` empty), and
// 0005 now ABORTS on it rather than passing it through an EXISTS guard that
// silently substitutes NULL. So the lossless-copy assertion covers every column
// for rows the migration is willing to move, and the abort assertion covers the
// rows it is not.

// wqFixture is the row set both tests load at 0004. It covers every `state`
// 0001's CHECK permits, every `fail_reason` including NULL, unicode and control
// characters in three free-text columns, NULL and non-NULL for every nullable
// column, and an id at the top of the rowid range.
//
// work_id is a parameter because it is the column the two tests disagree about.
func wqFixture(t *testing.T, ctx context.Context, d *DB, workIDs map[int64]any) {
	t.Helper()

	// Parents first: a non-NULL user_id and service_instance_id have to point
	// somewhere, and a rebuild is exactly where those two clauses get copied
	// wrong.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user (id, username, auth_source) VALUES (7, 'joe', 'local')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO service_instance (id, kind, name, base_url, api_key_enc)
			VALUES (3, 'sonarr', 'Sonarr', 'http://sonarr:8989', x'00')`)
		return err
	}); err != nil {
		t.Fatalf("fixture parents: %v", err)
	}

	// Unicode outside the BMP, combining marks, and C0 control characters —
	// the three things a hand-written copy of a TEXT column is most likely to
	// mangle, and none of which any other test in this package writes.
	weird := "Ünïcødé \u200b 🎬 \x01\x02\x1f\t\n\r déjà-vu"

	rows := []struct {
		id       int64
		key      string
		userID   int64
		kind     string
		instance any
		payload  string
		state    string
		fail     any
		attempts int64
		maxAtt   int64
		next     any
		verify   any
		lastErr  any
		created  string
		settled  any
	}{
		{1, "k-pending", 0, "add", nil, `{"a":1}`, "pending", nil, 0, 6, "2026-08-17 07:00:00", nil, nil, "2026-08-17 06:00:00", nil},
		{2, "k-inflight", 7, "monitor", int64(3), `{"b":"` + weird + `"}`, "inflight", nil, 1, 6, "2026-08-17 07:00:01", "2026-08-17 07:15:00", weird, "2026-08-17 06:00:01", nil},
		{3, "k-verifying", 7, "grab", int64(3), `{}`, "verifying", nil, 2, 3, nil, "2026-08-17 07:15:00", nil, "2026-08-17 06:00:02", nil},
		{4, "k-done", 0, "unmonitor", nil, `{}`, "done", nil, 1, 6, nil, nil, nil, "2026-08-17 06:00:03", "2026-08-17 06:30:00"},
		{5, "k-failed-rejected", 0, "tag_add", int64(3), `{}`, "failed", "rejected", 6, 6, nil, nil, "rejected upstream", "2026-08-17 06:00:04", "2026-08-17 06:30:01"},
		{6, "k-failed-unknown", 7, "refresh", nil, `{}`, "failed", "unknown", 6, 6, nil, nil, weird, "2026-08-17 06:00:05", "2026-08-17 06:30:02"},
		{7, "k-failed-exhausted", 7, "delete", nil, `{}`, "failed", "exhausted", 6, 6, nil, nil, nil, "2026-08-17 06:00:06", "2026-08-17 06:30:03"},
		// The unicode key, and the top of the rowid range. Both go in last:
		// after max int64 no implicit rowid can be allocated.
		{8, "k-" + weird, 0, "grab", nil, `{}`, "pending", nil, 0, 6, nil, nil, nil, "2026-08-17 06:00:07", nil},
		{9223372036854775807, "k-max-rowid", 0, "grab", nil, `{}`, "pending", nil, 0, 6, nil, nil, nil, "2026-08-17 06:00:08", nil},
	}

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO write_queue (
					id, idempotency_key, user_id, kind, work_id, service_instance_id,
					payload, state, fail_reason, attempts, max_attempts,
					next_attempt_at, verify_until, last_error, created_at, settled_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				r.id, r.key, r.userID, r.kind, workIDs[r.id], r.instance,
				r.payload, r.state, r.fail, r.attempts, r.maxAtt,
				r.next, r.verify, r.lastErr, r.created, r.settled); err != nil {
				return fmt.Errorf("row %d: %w", r.id, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture rows: %v", err)
	}
}

// dumpWriteQueue renders every column of every row, typed, so that a value that
// changed type (1 vs '1') or became NULL is a diff rather than a match. The
// column list comes from the result set, so a column added by a later migration
// is compared too instead of being silently skipped.
func dumpWriteQueue(ctx context.Context, q *sql.DB) (string, error) {
	rows, err := q.QueryContext(ctx, `SELECT * FROM write_queue ORDER BY id`)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		for i, c := range cols {
			fmt.Fprintf(&b, "%s=%T:%#v\n", c, cells[i], cells[i])
		}
		b.WriteString("---\n")
	}
	return b.String(), rows.Err()
}

// migrateTo0004 walks a fresh database up and then one step back down, which is
// the only route this package has to "the schema as 0004 left it".
func migrateTo0004(t *testing.T, ctx context.Context) *DB {
	t.Helper()
	d := openTestDB(t)
	migrateDownTo(t, ctx, d, 4)
	return d
}

// migrateDownTo steps a database down one migration at a time until it is at
// `target`, checking the version after each step.
//
// It is a loop and not a single MigrateDown because it used to be the latter,
// and that made "step down to the migration under test" mean "step down once" —
// a coincidence that held only while 0005 was the head. Adding 0006 broke both
// callers with `schema version = 5, want 4`, which is the failure mode worth
// removing rather than re-hardcoding: the next migration would break them again.
func migrateDownTo(t *testing.T, ctx context.Context, d *DB, target int64) {
	t.Helper()
	for {
		v, err := d.Version(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if v == target {
			return
		}
		if v < target {
			t.Fatalf("schema version = %d, below the target %d", v, target)
		}
		if err := d.MigrateDown(ctx); err != nil {
			t.Fatalf("MigrateDown %d→%d: %v", v, v-1, err)
		}
	}
}

// TestMigrate0005WriteQueueCopyIsLossless is the test the rebuild never had.
// Every column of every row must survive the 12-step rebuild byte for byte.
func TestMigrate0005WriteQueueCopyIsLossless(t *testing.T) {
	ctx := t.Context()
	d := migrateTo0004(t, ctx)

	wqFixture(t, ctx, d, nil) // every work_id NULL: see the abort test for the rest

	before, err := dumpWriteQueue(ctx, d.Read())
	if err != nil {
		t.Fatalf("dump before: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate 4→5: %v", err)
	}

	after, err := dumpWriteQueue(ctx, d.Read())
	if err != nil {
		t.Fatalf("dump after: %v", err)
	}
	if before != after {
		t.Errorf("the 0005 write_queue copy is not lossless.\n\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestMigrate0005AbortsRatherThanDroppingWorkID pins the resolution of the
// silent-NULL finding.
//
// `work` is created EMPTY by this migration, so every pre-existing non-NULL
// work_id is dangling by construction and the restored foreign key cannot
// accept it. There are only two honest outcomes — abort, or keep the value —
// and keeping it is not available. What is NOT acceptable is the third one the
// migration used to take: pass the column through
// `CASE WHEN EXISTS (SELECT 1 FROM work …) THEN work_id END`, which converts
// every such value to NULL with no error, no log and no sync_report row.
//
// So: the migration must fail, the message must say how many rows and what to
// do about them, and the database must be left at 4 with the rows untouched so
// the operator can act on them.
func TestMigrate0005AbortsRatherThanDroppingWorkID(t *testing.T) {
	ctx := t.Context()
	d := migrateTo0004(t, ctx)

	wqFixture(t, ctx, d, map[int64]any{2: int64(4412), 5: int64(9999)})

	before, err := dumpWriteQueue(ctx, d.Read())
	if err != nil {
		t.Fatalf("dump before: %v", err)
	}

	err = d.Migrate(ctx)
	if err == nil {
		t.Fatal("migration 0005 SUCCEEDED against write_queue rows carrying a work_id.\n" +
			"Every one of them is dangling — 0005 creates `work` empty — so the copy has " +
			"either silently NULLed the value or silently dropped the row. Neither is " +
			"acceptable: the value is the only record of which work the queued command was " +
			"for, and losing it is unrecoverable and invisible.")
	}
	// The message has to be actionable: the count, and the column.
	if !strings.Contains(err.Error(), "work_id") || !strings.Contains(err.Error(), "2") {
		t.Errorf("the abort message names neither the column nor the row count: %v", err)
	}

	// goose runs each migration in one transaction, so a failed 0005 must leave
	// the database exactly as 0004 left it — including the rows themselves.
	v, err := d.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != 4 {
		t.Errorf("schema version = %d after the abort, want 4", v)
	}
	after, err := dumpWriteQueue(ctx, d.Read())
	if err != nil {
		t.Fatalf("dump after: %v", err)
	}
	if before != after {
		t.Errorf("the aborted migration changed write_queue.\n\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestImageAssetExpirySweepEvicts proves the change from the DEFAULTED
// `ON DELETE NO ACTION` on work.poster_asset_id / work.backdrop_asset_id to
// `ON DELETE SET NULL`.
//
// `ix_img_state(state, expires_at)` has no plausible reader but an expiry
// sweep. Under NO ACTION that sweep's DELETE fails with "FOREIGN KEY constraint
// failed" for every asset any work points at — which is every poster on the
// Home screen, i.e. exactly the set the index exists to find. The index was
// therefore serving a query that could never succeed.
func TestImageAssetExpirySweepEvicts(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for i, role := range []string{"poster", "backdrop"} {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, state, expires_at)
				VALUES (?, ?, 'provider', ?, 'k', 'ready', '2020-01-01 00:00:00')`,
				i+1, fmt.Sprintf("https://example.invalid/%s.jpg", role), role); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title,
			                  poster_asset_id, backdrop_asset_id)
			VALUES (1, 'movie', 'Train Dreams', 'train dreams', 'train dreams', 1, 2)`)
		return err
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	// The sweep, spelled the way ix_img_state is shaped for.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`DELETE FROM image_asset WHERE state = 'ready' AND expires_at <= '2026-08-17 00:00:00'`)
		return err
	}); err != nil {
		t.Fatalf("the image expiry sweep cannot evict an asset a work references: %v\n"+
			"That is what a defaulted ON DELETE NO ACTION on work.poster_asset_id / "+
			"work.backdrop_asset_id costs: ix_img_state's only plausible reader can never "+
			"succeed on a referenced row.", err)
	}

	var poster, backdrop sql.NullInt64
	if err := d.Read().QueryRowContext(ctx,
		`SELECT poster_asset_id, backdrop_asset_id FROM work WHERE id = 1`).
		Scan(&poster, &backdrop); err != nil {
		t.Fatal(err)
	}
	if poster.Valid || backdrop.Valid {
		t.Errorf("the work still points at evicted assets (poster=%v backdrop=%v); "+
			"ON DELETE SET NULL is not in effect", poster, backdrop)
	}
	// The work itself must survive: SET NULL, never CASCADE. A cover expiring
	// must not delete the movie.
	var works int
	if err := d.Read().QueryRowContext(ctx, `SELECT count(*) FROM work`).Scan(&works); err != nil {
		t.Fatal(err)
	}
	if works != 1 {
		t.Errorf("evicting a cover deleted %d work rows; the clause is CASCADE, not SET NULL", 1-works)
	}
}

// TestUnfiledLibraryIsProtected covers the second half of the reserved row:
// before this, "reserved" was a comment and `DELETE FROM library WHERE id = 0`
// simply succeeded, leaving the membership derivation with nowhere to file an
// unfiled work for the life of the database.
//
// The trigger that fixes it is the shape that has already bitten this project
// once — audit_log's append-only triggers turned an ON DELETE action into an
// undeletable user (schema.md §9) — so the interaction is tested, not argued.
func TestUnfiledLibraryIsProtected(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// 1. The direct delete is refused.
	err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM library WHERE id = 0`)
		return err
	})
	if err == nil {
		t.Error("DELETE FROM library WHERE id = 0 succeeded; the reserved row is unprotected")
	} else if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("library 0 is protected by something other than its own message: %v", err)
	}
	var unfiled int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM library WHERE id = 0`).Scan(&unfiled); err != nil {
		t.Fatal(err)
	}
	if unfiled != 1 {
		t.Fatal("library 0 is gone")
	}

	// 2. Any OTHER library still deletes. A guard that blocks the whole table
	//    would be a different bug with the same green test.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO library (id, user_id, name, slug, kind)
			VALUES (5, 0, 'Films', 'films', 'movie')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM library WHERE id = 5`)
		return err
	}); err != nil {
		t.Errorf("an ordinary library is no longer deletable: %v", err)
	}

	// 3. THE audit_log FAILURE MODE. A user with a row in every table that
	//    references user(id) — library included — must still be deletable, or
	//    the trigger has repeated audit_log's defect one table over.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmts := []string{
			`INSERT INTO user (id, username, auth_source) VALUES (9, 'ada', 'local')`,
			`INSERT INTO library (id, user_id, name, slug, kind) VALUES (6, 9, 'Ada', 'ada', 'movie')`,
			`INSERT INTO library_override (id, user_id, verb, library_id, work_id, target_identity_hash)
			   VALUES (1, 9, 'exclude', 6, 1, 'h')`,
			`INSERT INTO session (id, user_id, kind, created_at, last_seen_at,
			                      idle_expires_at, absolute_expires_at)
			   VALUES ('s', 9, 'web', '2026-08-17 00:00:00', '2026-08-17 00:00:00',
			           '2026-08-18 00:00:00', '2026-08-24 00:00:00')`,
			`INSERT INTO client_credential (id, user_id, label, protocol, key_prefix, key_hash)
			   VALUES (1, 9, 'l', 'native', 'pfx', x'00')`,
			`INSERT INTO tag (id, value) VALUES (1, 't')`,
			`INSERT INTO tag_assignment (tag_id, work_id, user_id, source) VALUES (1, 1, 9, 'user')`,
			`INSERT INTO write_queue (idempotency_key, user_id, kind, payload)
			   VALUES ('k9', 9, 'grab', '{}')`,
			`INSERT INTO service_instance (id, kind, name, base_url, api_key_enc)
			   VALUES (2, 'prowlarr', 'Prowlarr', 'http://p:9696', x'00')`,
			`INSERT INTO release_candidate (id, user_id, service_instance_id, guid, title,
			                                raw_release_json, fetched_at, expires_at)
			   VALUES (1, 9, 2, 'g', 't', '{}', '2026-08-17 00:00:00', '2026-08-17 00:25:00')`,
			`INSERT INTO audit_log (actor_user_id, action, result) VALUES (9, 'login', 'ok')`,
		}
		for _, s := range stmts {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user WHERE id = 9`)
		return err
	}); err != nil {
		t.Fatalf("a user with rows in every table is no longer deletable: %v\n"+
			"That is audit_log's defect reproduced: a trigger that aborts the implicit write "+
			"an ON DELETE action performs makes some row undeletable forever.", err)
	}

	// 4. …and the one row that IS now undeletable, pinned deliberately rather
	//    than discovered. user 0 is the shared/system sentinel and its cascade
	//    reaches library 0. Deleting it used to succeed and took library 0,
	//    every shared tag_assignment, every shared write_queue row and every
	//    shared release_candidate with it. It now fails.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user WHERE id = 0`)
		return err
	}); err == nil {
		t.Error("DELETE FROM user WHERE id = 0 succeeded. The sentinel user's cascade reaches " +
			"library 0, so it must not: a sentinel that can be deleted is not a sentinel.")
	}
}

// TestSearchDocVisibilityIsACodeInvariant executes schema.md §7's invariants 5
// and 2 against the paths that break them, so that "asserted, not enforced" is
// a measurement rather than a claim.
//
// Neither is declarable in SQLite. Invariant 5 ("every search_doc row has at
// least one search_doc_library row") has no "at least one child" constraint and
// no trigger position that can express it: the doc must exist before its
// junction row, and the two ways it breaks are cascades. Invariant 2
// (count(search_fts) == count(search_trgm) == count(search_doc)) cannot be
// expressed at all — the FTS5 tables are virtual, so they can carry neither a
// constraint nor a trigger.
//
// This test therefore pins THREE things: each invariant's own query, the fact
// that the schema does not uphold either, and — by firing them — that the
// queries detect what they exist to detect.
func TestSearchDocVisibilityIsACodeInvariant(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	orphans := func() int {
		t.Helper()
		var n int
		if err := d.Read().QueryRowContext(ctx, `
			SELECT count(*) FROM search_doc sd
			 WHERE NOT EXISTS (SELECT 1 FROM search_doc_library sdl WHERE sdl.doc_rowid = sd.rowid)`).
			Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	counts := func() (int, int, int) {
		t.Helper()
		var doc, fts, trgm int
		row := func(q string, into *int) {
			if err := d.Read().QueryRowContext(ctx, q).Scan(into); err != nil {
				t.Fatal(err)
			}
		}
		row(`SELECT count(*) FROM search_doc`, &doc)
		row(`SELECT count(*) FROM search_fts`, &fts)
		row(`SELECT count(*) FROM search_trgm`, &trgm)
		return doc, fts, trgm
	}

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmts := []string{
			`INSERT INTO user (id, username, auth_source) VALUES (9, 'ada', 'local')`,
			`INSERT INTO library (id, user_id, name, slug, kind) VALUES (5, 9, 'Ada', 'ada', 'movie')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (1, 'movie', 'Train Dreams', 'train dreams', 'train dreams')`,
			`INSERT INTO search_doc (rowid, work_id, kind, norm_title) VALUES (1, 1, 'movie', 'train dreams')`,
			`INSERT INTO search_fts (rowid, title) VALUES (1, 'Train Dreams')`,
			`INSERT INTO search_trgm (rowid, title) VALUES (1, 'Train Dreams')`,
			`INSERT INTO search_doc_library (library_id, doc_rowid) VALUES (5, 1)`,
		}
		for _, s := range stmts {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if got := orphans(); got != 0 {
		t.Fatalf("the fixture already violates invariant 5: %d orphans", got)
	}

	// INVARIANT 5, BREAKING PATH 1: delete the library. The junction row
	// cascades; the doc does not, and is now visible to no scope at all —
	// including Unfiled.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM library WHERE id = 5`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := orphans(); got != 1 {
		t.Errorf("deleting a library left %d orphaned search_doc rows, want 1 — "+
			"either the schema now enforces invariant 5 (say so beside search_doc_library "+
			"in 00005 and delete this arm) or the invariant query no longer detects it", got)
	}

	// INVARIANT 5, BREAKING PATH 2: delete the OWNER. It reaches the junction
	// two cascades deep, through library.user_id, which is the path a reader
	// looking only at search_doc_library's own foreign keys will not see.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmts := []string{
			`INSERT INTO library (id, user_id, name, slug, kind) VALUES (6, 9, 'Ada 2', 'ada-2', 'movie')`,
			`INSERT INTO search_doc_library (library_id, doc_rowid) VALUES (6, 1)`,
		}
		for _, s := range stmts {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := orphans(); got != 0 {
		t.Fatalf("re-filing the doc did not clear the orphan: %d", got)
	}
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user WHERE id = 9`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := orphans(); got != 1 {
		t.Errorf("deleting the owning user left %d orphaned search_doc rows, want 1", got)
	}

	// INVARIANT 2. Deleting the work cascades search_doc away and leaves both
	// FTS tables holding a document that no longer exists — they are virtual,
	// so no foreign key and no trigger can reach them.
	if doc, fts, trgm := counts(); doc != 1 || fts != 1 || trgm != 1 {
		t.Fatalf("fixture counts are %d/%d/%d, want 1/1/1", doc, fts, trgm)
	}
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM work WHERE id = 1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	doc, fts, trgm := counts()
	if doc != 0 {
		t.Errorf("search_doc did not cascade with its work: %d rows", doc)
	}
	if fts != 1 || trgm != 1 {
		t.Errorf("the FTS tables followed the work delete (%d/%d) — if SQLite can now reach a "+
			"virtual table from a foreign key, invariant 2 is enforceable and the note beside "+
			"search_doc_library in 00005 is wrong", fts, trgm)
	}
}

// TestMigration0005DownPreservesEveryRow covers the Down block, which used to
// filter its copy with `WHERE state IN (…)` and therefore DELETED — silently,
// with no count — every row in the one state 0005 exists to permit.
//
// Down is not a supported user path; it is how a migration is tested locally,
// and a Down that eats rows makes the round trip untestable, which is the whole
// job of the block.
func TestMigration0005DownPreservesEveryRow(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows := []struct{ key, state, lastErr string }{
			{"k-pending", "pending", ""},
			{"k-awaiting", "awaiting_choice", ""},
			{"k-awaiting-2", "awaiting_choice", "an earlier error"},
			{"k-done", "done", ""},
		}
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO write_queue (idempotency_key, kind, payload, state, last_error)
				VALUES (?, 'grab', '{}', ?, NULLIF(?, ''))`, r.key, r.state, r.lastErr); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	// Down to 4, which is 0005's Down block — 0006 sits above it now and its own
	// Down touches nothing here.
	migrateDownTo(t, ctx, d, 4)

	var n int
	if err := d.Read().QueryRowContext(ctx, `SELECT count(*) FROM write_queue`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("the Down block kept %d of 4 rows. It must not delete a row whose state "+
			"0001's CHECK cannot hold — the state is remapped to 'failed' and recorded in "+
			"last_error instead, because a row that vanishes is unrecoverable and a row whose "+
			"state changed is still there to look at.", n)
	}

	// The remap is visible in the data, not just implied by the row count.
	rows, err := d.Read().QueryContext(ctx,
		`SELECT idempotency_key, state, last_error FROM write_queue ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	remapped := 0
	for rows.Next() {
		var key, state string
		var lastErr sql.NullString
		if err := rows.Scan(&key, &state, &lastErr); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(key, "k-awaiting") {
			continue
		}
		remapped++
		if state != "failed" {
			t.Errorf("%s survived Down as state=%q; 0001's CHECK cannot hold it", key, state)
		}
		if !strings.Contains(lastErr.String, "awaiting_choice") {
			t.Errorf("%s lost the record of its original state: last_error = %q", key, lastErr.String)
		}
		if key == "k-awaiting-2" && !strings.Contains(lastErr.String, "an earlier error") {
			t.Errorf("%s's pre-existing last_error was overwritten: %q", key, lastErr.String)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if remapped != 2 {
		t.Errorf("found %d remapped rows, want 2", remapped)
	}
}

// TestFTS5TokenizerBehaviour tests what the two virtual tables were CONFIGURED
// for, which no test did: TestFTS5IsAvailableOnEveryConnection only MATCHes an
// empty table (and never asserts the count), so every tokenizer option in the
// DDL — `remove_diacritics 2`, `prefix='2 3 4'`, `trigram` — was a string
// nothing exercised.
//
// WHICH MUTATION EACH ASSERTION CATCHES WAS MEASURED, NOT ASSUMED, because
// three of the four options turn out to be much harder to detect than they
// look. Each option was gutted in the migration in turn and the suite re-run:
//
//	tokenize=trigram → unicode61       the two substring cases go red
//	remove_diacritics 2 → 0            the two folding cases go red
//	remove_diacritics 2 → 1            ONLY the pinyin case goes red — see below
//	prefix='2 3 4' removed             NOTHING goes red — see below
//
// The 2-versus-1 discriminator is not what one would guess. Twenty accented
// characters were tried under both settings and eighteen behaved identically;
// what separates them is characters carrying TWO diacritics, which "1" leaves
// alone and "2" folds (ǚ, ǻ, ǭ, ȱ, ṩ). ǚ is the one that matters here rather
// than being a curiosity: it is pinyin, so "nǚ" is a romanised Chinese title,
// and under `remove_diacritics 1` a search for "nu" would not find it.
func TestFTS5TokenizerBehaviour(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		docs := []struct {
			rowid int
			title string
		}{
			{1, "Amélie"},        // precomposed é
			{2, "Trainspotting"}, //
			{3, "Amélie Two"},   // decomposed: e + combining acute
			{4, "Nǚ Shu"},        // pinyin ǚ — u with diaeresis AND caron
		}
		for _, doc := range docs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO search_fts (rowid, title) VALUES (?, ?)`, doc.rowid, doc.title); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO search_trgm (rowid, title) VALUES (?, ?)`, doc.rowid, doc.title); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	match := func(t *testing.T, table, expr string) []int {
		t.Helper()
		rows, err := d.Read().QueryContext(ctx,
			fmt.Sprintf(`SELECT rowid FROM %s WHERE %s MATCH ? ORDER BY rowid`, table, table), expr)
		if err != nil {
			t.Fatalf("MATCH %q against %s: %v", expr, table, err)
		}
		defer func() { _ = rows.Close() }()
		var out []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			out = append(out, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	same := func(got []int, want []int) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	cases := []struct {
		name  string
		table string
		expr  string
		want  []int
		why   string
	}{
		{
			name: "exact term", table: "search_fts", expr: `trainspotting`, want: []int{2},
			why: "the baseline: unicode61 indexing whole words, case-folded",
		},
		{
			name: "prefix query", table: "search_fts", expr: `trai*`, want: []int{2},
			why: "a prefix query must return the document. NOTE (measured): removing " +
				"prefix='2 3 4' from the migration leaves this GREEN — FTS5 answers a " +
				"prefix query from the main term list either way, and the declared prefix " +
				"index is what makes it a seek rather than a term-list scan. The DDL " +
				"assertion below is what pins the option; this pins the behaviour",
		},
		{
			name: "diacritics folded, precomposed", table: "search_fts", expr: `amelie`, want: []int{1, 3},
			why: "remove_diacritics folds é so a keyboard without it still matches",
		},
		{
			name: "diacritics folded, decomposed", table: "search_fts", expr: `amelie two`, want: []int{3},
			why: "and it folds a combining acute the same way as a precomposed é",
		},
		{
			name: "diacritics folded, query accented", table: "search_fts", expr: "Am\u00e9lie", want: []int{1, 3},
			why: "the fold is applied to the QUERY as well as to the document",
		},
		{
			name: "the double diacritic only option 2 folds", table: "search_fts", expr: `nu`, want: []int{4},
			why: "ǚ carries two diacritics. `remove_diacritics 1` leaves it alone and this " +
				"case goes red under it; 2 folds it. Measured, and it is the ONLY case in " +
				"this test that separates the two settings",
		},
		{
			name: "substring, mid-word", table: "search_trgm", expr: `spotti`, want: []int{2},
			why: "the trigram table exists for exactly this: unicode61 cannot match inside a word",
		},
		{
			// ⚠️ MEASURED ASYMMETRY, pinned rather than fixed. The trigram
			// tokenizer's own remove_diacritics defaults to 0 and the
			// migration does not set it, so `melie` — which the unicode61
			// table above WOULD fold to a match — returns nothing here, and
			// the accented spelling is the only one that hits. That is a
			// substantive difference between the two arms of the same search
			// and it belongs to whoever writes the retriever; this case
			// records which behaviour ships today.
			name: "substring, mid-word, accent must be typed", table: "search_trgm",
			expr: "\u00e9lie", want: []int{1},
			why: "the trigram arm is diacritic-SENSITIVE: tokenize='trigram' with no " +
				"remove_diacritics option, which defaults to 0",
		},
		{
			name: "substring, mid-word, unaccented misses", table: "search_trgm",
			expr: `melie`, want: nil,
			why: "the other half of the same measurement. If this ever returns a row, " +
				"the trigram table has gained diacritic folding and the case above " +
				"plus the note beside it are wrong",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := match(t, tc.table, tc.expr)
			if !same(got, tc.want) {
				t.Errorf("%s MATCH %q = %v, want %v\n  %s", tc.table, tc.expr, got, tc.want, tc.why)
			}
		})
	}

	// The negative control. Without it every assertion above is satisfied by a
	// configuration that matches everything.
	if got := match(t, "search_fts", `nosuchterm`); len(got) != 0 {
		t.Errorf("search_fts MATCH 'nosuchterm' = %v, want nothing", got)
	}

	// The one option with no behavioural signature, pinned on the DDL instead —
	// and said out loud rather than left as a green test that proves nothing.
	// prefix='2 3 4' is what makes a 2-, 3- or 4-character as-you-type query an
	// index read; ARCHITECTURE §8.2's search-as-you-type budget is written
	// against it.
	var ddl string
	if err := d.Read().QueryRowContext(ctx,
		`SELECT sql FROM sqlite_schema WHERE name = 'search_fts'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "prefix='2 3 4'") {
		t.Errorf("search_fts no longer declares prefix='2 3 4':\n  %s", ddl)
	}
	if !strings.Contains(ddl, "remove_diacritics 2") {
		t.Errorf("search_fts no longer declares remove_diacritics 2:\n  %s", ddl)
	}
}

// ─── Migration 0006 · the three subtype tables Kavita writes ─────────────────
//
// ⚠️ THERE IS NO POPULATED-FIXTURE TEST FOR 0006, AND THAT IS STATED HERE
// RATHER THAN LEFT FOR A READER TO WONDER ABOUT. The lesson 0005 paid for is
// that a migration test which runs 1→N against an EMPTY database proves the
// shape and nothing about data — which is why 0005 has wqFixture,
// TestMigrate0005WriteQueueCopyIsLossless and
// TestMigrate0005AbortsRatherThanDroppingWorkID beside its round trip. That
// lesson does not apply here, and the reason is structural rather than a
// judgement call: 0006 creates three BRAND-NEW tables and touches no existing
// one. There is no copy step, no ALTER, no rebuild and no backfill, so there is
// no row anywhere that this migration could drop, truncate or silently rewrite
// — including on the way back down, where the Down block's three DROPs remove
// only tables the Up block created. A fixture would populate tables that do not
// exist until the migration under test has already run, which measures the
// tables and not the migration.
//
// What CAN go wrong here is shape, and shape is what these tests execute
// against: the declared types (ADR-0030 says any INTEGER issue-number column is
// wrong), the foreign keys and their ON DELETE actions, the NOT NULL defaults,
// and the down/up round trip.

// TestMigrate0006SubtypeTablesExist pins the exact column set and declared type
// of each of the three tables.
//
// The DECLARED TYPE is asserted and not just the name, because under STRICT it
// is the only thing standing between '1.MU' and a rejected insert:
// work_comic_issue.number_sort must be REAL and number_text must be TEXT.
// ADR-0030 puts it in terms — "Any integer column is wrong" — after Komga and
// Kavita were both checked, and a well-meaning INTEGER here is the single most
// likely future edit to this table.
func TestMigrate0006SubtypeTablesExist(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	want := map[string][][2]string{
		"work_book": {
			{"work_id", "INTEGER"},
			{"page_count", "INTEGER"},
			{"series_name", "TEXT"},
			{"series_position", "REAL"},
		},
		"work_comic": {
			{"work_id", "INTEGER"},
			{"volume_label", "TEXT"},
			{"volume_year", "INTEGER"},
			{"reading_direction", "TEXT"},
			{"publisher", "TEXT"},
			{"total_issues_declared", "INTEGER"},
			{"total_issues_source", "TEXT"},
		},
		"work_comic_issue": {
			{"work_id", "INTEGER"},
			{"number_text", "TEXT"},
			{"number_sort", "REAL"},
			{"volume_label", "TEXT"},
			{"volume_sort", "REAL"},
			{"is_special", "INTEGER"},
			{"is_oneshot", "INTEGER"},
			{"special_version", "TEXT"},
			{"page_count", "INTEGER"},
		},
	}

	for table, cols := range want {
		t.Run(table, func(t *testing.T) {
			rows, err := d.Read().QueryContext(ctx,
				`SELECT name, type FROM pragma_table_info(?) ORDER BY cid`, table)
			if err != nil {
				t.Fatalf("%s does not exist: %v", table, err)
			}
			defer func() { _ = rows.Close() }()

			var got [][2]string
			for rows.Next() {
				var name, typ string
				if err := rows.Scan(&name, &typ); err != nil {
					t.Fatal(err)
				}
				got = append(got, [2]string{name, typ})
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(got) != fmt.Sprint(cols) {
				t.Errorf("%s columns = %v\nwant %v\n"+
					"docs/reference/schema.md §1.1 is authoritative for the shape of these "+
					"three tables. If a column is genuinely wanted, add it in 0006's "+
					"successor — 0006 is merged — and update this list in the same change.",
					table, got, cols)
			}
		})
	}
}

// TestMigrate0006IssueNumbersRoundTrip is the by-execution half of the type
// assertion above: the five issue numbers ADR-0030 names as real — '1.MU',
// '-1', '0', 'Annual 1', '1A' — are inserted, read back and ordered.
//
// STRICT is what makes this test able to fail. In a non-STRICT table a TEXT
// value lands in an INTEGER column by affinity and this whole test stays green
// over a schema that has silently lost the distinction.
func TestMigrate0006IssueNumbersRoundTrip(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	type issue struct {
		id   int
		text string
		sort float64
	}
	issues := []issue{
		{1, "-1", -1},
		{2, "0", 0},
		{3, "1", 1},
		{4, "1.MU", 1.5},
		{5, "1A", 1.75},
		{6, "Annual 1", 900},
	}
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title)
			VALUES (1, 'comic', 'Saga', 'saga', 'saga')`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_comic (work_id, volume_label, volume_year, reading_direction,
			                        publisher, total_issues_declared, total_issues_source)
			VALUES (1, 'Vol. 1', 2012, 'ltr', 'Image', 60, 'comicinfo')`); err != nil {
			return err
		}
		for _, is := range issues {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO work (id, kind, parent_work_id, title, sort_title, normalized_title)
				VALUES (?, 'comic_issue', 1, ?, ?, ?)`,
				100+is.id, is.text, is.text, is.text); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO work_comic_issue (work_id, number_text, number_sort)
				VALUES (?, ?, ?)`, 100+is.id, is.text, is.sort); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	rows, err := d.Read().QueryContext(ctx, `
		SELECT number_text, number_sort FROM work_comic_issue ORDER BY number_sort`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var order []string
	for rows.Next() {
		var text string
		var sort float64
		if err := rows.Scan(&text, &sort); err != nil {
			t.Fatal(err)
		}
		order = append(order, text)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(order, ","), "-1,0,1,1.MU,1A,Annual 1"; got != want {
		t.Errorf("issues ordered by number_sort = %s, want %s", got, want)
	}

	// A fractional sort key survives as a fraction. On an INTEGER column a
	// STRICT table rejects it outright, and a non-STRICT one truncates it to 1 —
	// which is the failure that puts '1.MU' and '1' at the same position.
	var frac float64
	if err := d.Read().QueryRowContext(ctx,
		`SELECT number_sort FROM work_comic_issue WHERE number_text = '1.MU'`).Scan(&frac); err != nil {
		t.Fatal(err)
	}
	if frac != 1.5 {
		t.Errorf("number_sort for '1.MU' read back as %v, want 1.5", frac)
	}

	// And the type really is enforced: a text issue number in the SORT column is
	// rejected. Without this the REAL declaration above is a claim nobody tested.
	//
	// Against a work of its own, so the row that fails fails on its TYPE. Reusing
	// an existing work_id would trip the primary key first and leave this
	// assertion green over a table that had lost STRICT entirely.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, parent_work_id, title, sort_title, normalized_title)
			VALUES (200, 'comic_issue', 1, 'x', 'x', 'x')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work_comic_issue (work_id, number_text, number_sort)
			VALUES (200, 'x', 'Annual 1')`)
		return err
	}); err == nil {
		t.Error("number_sort accepted the string 'Annual 1'; the table is not STRICT, " +
			"so a parse failure now reaches disk as data instead of as an error")
	}

	// The two boolean flags default to 0 rather than to NULL, so "not a special"
	// is a value and not an unknown.
	var special, oneshot int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT is_special, is_oneshot FROM work_comic_issue WHERE work_id = 101`).
		Scan(&special, &oneshot); err != nil {
		t.Fatal(err)
	}
	if special != 0 || oneshot != 0 {
		t.Errorf("is_special/is_oneshot defaulted to %d/%d, want 0/0", special, oneshot)
	}
}

// TestMigrate0006ForeignKeysCascadeFromWork proves the one integrity property
// these tables have: each row's lifetime is its parent work's.
//
// It matters because of the 7-day tombstone. A work is soft-deleted first and
// hard-deleted later, and a subtype row that outlived its work would be a row no
// query can reach and no sweep can find.
func TestMigrate0006ForeignKeysCascadeFromWork(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	for _, table := range []string{"work_book", "work_comic", "work_comic_issue"} {
		t.Run(table, func(t *testing.T) {
			// A dangling parent is rejected. Foreign keys really are on — 0002
			// pins that — and this is the insert that would silently succeed if
			// they were not.
			if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx,
					fmt.Sprintf(`INSERT INTO %s (work_id) VALUES (99999)`, table))
				return err
			}); err == nil {
				t.Errorf("%s accepted a row for a work that does not exist", table)
			}

			// And a real parent's delete takes the subtype row with it.
			if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO work (id, kind, title, sort_title, normalized_title)
					VALUES (7, 'book', 'Piranesi', 'piranesi', 'piranesi')`); err != nil {
					return err
				}
				_, err := tx.ExecContext(ctx,
					fmt.Sprintf(`INSERT INTO %s (work_id) VALUES (7)`, table))
				return err
			}); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `DELETE FROM work WHERE id = 7`)
				return err
			}); err != nil {
				t.Fatalf("deleting the parent work: %v", err)
			}
			var n int
			if err := d.Read().QueryRowContext(ctx,
				fmt.Sprintf(`SELECT count(*) FROM %s WHERE work_id = 7`, table)).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("%s kept %d row(s) after its parent work was deleted", table, n)
			}
		})
	}
}

// TestMigrate0006DownAndUp round-trips the migration.
//
// The Down block is three DROPs and this is what checks the header's claim that
// nothing else is owed: after Down the three tables are gone and NOTHING ELSE
// changed — the assertion is over the whole schema dump, not over the three
// names, because a Down that also took an index or a trigger with it would pass
// a three-name check. PRAGMA foreign_key_check runs afterwards, which is where
// a DROP that stranded a reference would show up.
func TestMigrate0006DownAndUp(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	created := []string{"work_book", "work_comic", "work_comic_issue"}

	// Later migrations sit on top of 0006, and MigrateDown rolls back exactly
	// one migration, so this walks down to 0006 first and back up past it at
	// the end. Every assertion below is still about 0006's own block.
	//
	// ⚠️ THE WALK DOWN IS dumpSchemaAt'S JOB, NOT A HAND-COUNTED RUN OF
	// MigrateDown CALLS. This used to be one explicit step per migration
	// stacked above, with a comment saying "each new migration adds one step
	// here. That is the maintenance a migration on top of this one owes". 0008
	// and 0009 landed in the same week and both paid it; migrateDownTo retired
	// that bill, and dumpSchemaAt retires the one after it — see its doc for
	// why the version is an argument and not a line a caller can forget.
	at6 := dumpSchemaAt(t, ctx, d, 6)

	migrateDownTo(t, ctx, d, 5)
	for _, table := range created {
		var n int
		if err := d.Read().QueryRowContext(ctx,
			`SELECT count(*) FROM pragma_table_list WHERE name = ?`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s survived the Down block", table)
		}
	}

	at5 := dumpSchemaAt(t, ctx, d, 5)
	// Everything 0006 did NOT create must be byte-identical across the Down.
	// This one IS a cross-version comparison, so it reads .ddl rather than
	// going through sameAs.
	for _, line := range strings.Split(at6.ddl, "\n\n") {
		if line == "" {
			continue
		}
		var mine bool
		for _, table := range created {
			if strings.Contains(line, table) {
				mine = true
			}
		}
		if !mine && !strings.Contains(at5.ddl, line) {
			t.Errorf("0006's Down block removed or changed an object it did not create:\n%s", line)
		}
	}

	rows, err := d.Read().QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		t.Error("PRAGMA foreign_key_check reports a violation after 0006's Down")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// Migrate goes all the way to head, so the dump below has to come back down
	// to 6 before it is comparable with at6. That is the argument.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate 5 to latest: %v", err)
	}
	again := dumpSchemaAt(t, ctx, d, 6)
	if !again.sameAs(t, at6) {
		t.Error("a Down followed by an Up did not reproduce the schema 0006 creates")
	}
}

// ─── Migration 0007 · work_credit ───────────────────────────────────────────

// creditRoleVocabulary is migration 0007's `role` CHECK, written out once.
//
// It is the vocabulary docs/reference/schema.md §1.1 declares, in §1.1's own
// order, and it is shared by every assertion below so that a test which inserts
// a legal role and a test which pins the CHECK cannot drift apart.
var creditRoleVocabulary = []string{
	// music — no v0.1 writer; they are here because SQLite cannot ALTER a CHECK
	"primary", "featured", "composer", "conductor", "performer", "remixer", "producer",
	// books
	"author", "translator", "editor", "illustrator", "narrator",
	// comics
	"writer", "penciller", "inker", "colorist", "letterer", "cover_artist",
}

// TestMigrate0007WorkCreditShape pins the column set, the declared types and the
// primary-key ORDER.
//
// The order is the part worth asserting rather than the membership. schema.md
// §1.1 states the reason — "the PK leads with (work_id, role, position) because
// the only hot read is 'give me this work's credits in billing order', which is
// then a single covered range scan" — and a PK that spelled the same four
// columns in a different order would satisfy a set comparison, keep every
// uniqueness property, and quietly turn that range scan into a full scan of the
// table. On a WITHOUT ROWID table the primary key IS the storage order, so this
// is a physical-layout assertion, not a style one.
func TestMigrate0007WorkCreditShape(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	wantCols := [][2]string{
		{"work_id", "INTEGER"},
		{"creator_work_id", "INTEGER"},
		{"role", "TEXT"},
		{"position", "INTEGER"},
		{"credited_as", "TEXT"},
	}
	gotCols := tableInfo(t, d, "work_credit")
	if fmt.Sprint(gotCols) != fmt.Sprint(wantCols) {
		t.Errorf("work_credit columns = %v\nwant %v\n"+
			"docs/reference/schema.md §1.1 is authoritative for this table's shape. "+
			"0007 is merged; a genuinely wanted column goes in its successor.",
			gotCols, wantCols)
	}

	// The primary key, in its declared order. pragma_table_info.pk is the
	// 1-based position within the key, 0 for a non-key column.
	pkRows, err := d.Read().QueryContext(ctx,
		`SELECT name FROM pragma_table_info('work_credit') WHERE pk > 0 ORDER BY pk`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pkRows.Close() }()
	var pk []string
	for pkRows.Next() {
		var name string
		if err := pkRows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		pk = append(pk, name)
	}
	if err := pkRows.Err(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(pk, ","), "work_id,role,position,creator_work_id"; got != want {
		t.Errorf("work_credit PRIMARY KEY = (%s), want (%s).\n"+
			"The order is load-bearing: this is a WITHOUT ROWID table, so the key IS the "+
			"storage order, and the hot read is one work's credits in billing order.", got, want)
	}

	// WITHOUT ROWID, asserted by executing the thing it forbids rather than by
	// reading the DDL text: a WITHOUT ROWID table has no rowid column.
	var ignored int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT rowid FROM work_credit LIMIT 1`).Scan(&ignored); !errorMentions(err, "no such column") {
		t.Errorf("SELECT rowid FROM work_credit gave %v, want a \"no such column: rowid\" error. "+
			"work_credit must be WITHOUT ROWID: with a rowid the PRIMARY KEY becomes a secondary "+
			"index and the covered range scan it exists for is gone.", err)
	}
}

// errorMentions is true when err is non-nil and its message contains want. It
// exists because SQLite reports "no such column" as a plain error string and
// there is no sentinel to compare against.
func errorMentions(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}

// tableInfo reads a table's (name, declared type) pairs in declaration order.
func tableInfo(t *testing.T, d *DB, table string) [][2]string {
	t.Helper()
	rows, err := d.Read().QueryContext(t.Context(),
		`SELECT name, type FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		t.Fatalf("%s does not exist: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	var out [][2]string
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			t.Fatal(err)
		}
		out = append(out, [2]string{name, typ})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestMigrate0007RoleCheckIsEnforced fires the CHECK.
//
// Every one of the eighteen legal roles is inserted and must be accepted; then
// a nineteenth that is not a member must be rejected. Both halves are needed:
// a CHECK that rejects everything passes the negative test alone, and DB-01 —
// the `IN (NULL, …)` defect this project shipped twice — passes the positive
// test alone.
func TestMigrate0007RoleCheckIsEnforced(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title)
			VALUES (1, 'comic', 'Saga', 'saga', 'saga')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title)
			VALUES (2, 'person', 'Brian K. Vaughan', 'vaughan, brian k.', 'brian k vaughan')`)
		return err
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	for i, role := range creditRoleVocabulary {
		if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO work_credit (work_id, creator_work_id, role, position)
				VALUES (1, 2, ?, ?)`, role, i)
			return err
		}); err != nil {
			t.Errorf("role %q was rejected but is a member of schema.md §1.1's vocabulary: %v", role, err)
		}
	}

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work_credit (work_id, creator_work_id, role, position)
			VALUES (1, 2, 'TOTAL-GARBAGE', 99)`)
		return err
	}); err == nil {
		t.Error("work_credit.role accepted 'TOTAL-GARBAGE' — the CHECK is a no-op.\n" +
			"role is NOT NULL, so the correct form is the bare `role IN (...)`; if someone " +
			"rewrote it as `role IS NULL OR role IN (...)` that is harmless, but if a NULL " +
			"reached the IN list itself this is DB-01 for the third time.")
	}

	// A NULL role is rejected by NOT NULL, which is what makes the bare IN list
	// the correct form rather than an oversight.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work_credit (work_id, creator_work_id, role, position)
			VALUES (1, 2, NULL, 98)`)
		return err
	}); err == nil {
		t.Error("work_credit.role accepted NULL; it is declared NOT NULL")
	}
}

// TestMigrate0007CoCreditsAndBillingOrder is the by-execution proof that the
// primary key models what ADR-0031 says it models.
//
// Three things must all be true at once, and a scalar author column breaks all
// three: one work carries several people in one role at different positions;
// two people share one role AND one position (a co-credit); and the same person
// carries two different roles on one work (the writer who also letters).
func TestMigrate0007CoCreditsAndBillingOrder(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title)
			VALUES (1, 'comic', 'Saga', 'saga', 'saga')`); err != nil {
			return err
		}
		for id, name := range map[int]string{
			2: "Brian K. Vaughan", 3: "Fiona Staples", 4: "Fonografiks",
		} {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO work (id, kind, title, sort_title, normalized_title)
				VALUES (?, 'person', ?, ?, ?)`, id, name, name, name); err != nil {
				return err
			}
		}
		for _, c := range []struct {
			creator  int
			role     string
			position int
		}{
			{2, "writer", 0},
			{2, "letterer", 0}, // the same person, a second role
			{3, "cover_artist", 0},
			{4, "letterer", 1}, // a second letterer, later in the billing
			{3, "writer", 0},   // a CO-credit: same work, same role, same position
		} {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO work_credit (work_id, creator_work_id, role, position)
				VALUES (1, ?, ?, ?)`, c.creator, c.role, c.position); err != nil {
				return fmt.Errorf("credit %d/%s/%d: %w", c.creator, c.role, c.position, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM work_credit WHERE work_id = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("work 1 has %d credits, want 5 — a co-credit or a second role was swallowed", n)
	}

	// The exact row is rejected on a re-insert: the PK is uniqueness, not just
	// ordering.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work_credit (work_id, creator_work_id, role, position)
			VALUES (1, 2, 'writer', 0)`)
		return err
	}); err == nil {
		t.Error("the identical credit was inserted twice; the PRIMARY KEY is not unique")
	}

	// Billing order comes out of the index, in the key's own order.
	rows, err := d.Read().QueryContext(ctx, `
		SELECT role, position, creator_work_id FROM work_credit
		 WHERE work_id = 1 AND role = 'letterer' ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var order []string
	for rows.Next() {
		var role string
		var pos, creator int
		if err := rows.Scan(&role, &pos, &creator); err != nil {
			t.Fatal(err)
		}
		order = append(order, fmt.Sprintf("%d:%d", pos, creator))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(order, ","), "0:2,1:4"; got != want {
		t.Errorf("letterers in billing order = %s, want %s", got, want)
	}
}

// TestMigrate0007CascadesFromBothParents is the integrity property this table
// has that no other subtype table has: TWO foreign keys into `work`, and a
// credit must die with either end.
//
// The creator half is the one worth executing. work_id's cascade is the
// familiar subtype-row lifetime; creator_work_id's is what stops a deleted
// person leaving credits pointing at a work id that has since been reallocated
// to a film.
func TestMigrate0007CascadesFromBothParents(t *testing.T) {
	ctx := t.Context()

	for _, tc := range []struct{ name, deleteID string }{
		{"the credited work is deleted", "1"},
		{"the creator is deleted", "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := openTestDB(t)
			if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO work (id, kind, title, sort_title, normalized_title)
					VALUES (1, 'book', 'Piranesi', 'piranesi', 'piranesi')`); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO work (id, kind, title, sort_title, normalized_title)
					VALUES (2, 'person', 'Susanna Clarke', 'clarke, susanna', 'susanna clarke')`); err != nil {
					return err
				}
				_, err := tx.ExecContext(ctx, `
					INSERT INTO work_credit (work_id, creator_work_id, role, position)
					VALUES (1, 2, 'author', 0)`)
				return err
			}); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `DELETE FROM work WHERE id = `+tc.deleteID)
				return err
			}); err != nil {
				t.Fatalf("deleting work %s: %v", tc.deleteID, err)
			}
			var n int
			if err := d.Read().QueryRowContext(ctx,
				`SELECT count(*) FROM work_credit`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("%d credit(s) survived the delete of work %s", n, tc.deleteID)
			}
		})
	}

	// And a dangling creator is rejected outright. Foreign keys really are on —
	// 0002 pins that — and this is the insert that would silently succeed if
	// they were not.
	d := openTestDB(t)
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, title, sort_title, normalized_title)
			VALUES (1, 'book', 'Piranesi', 'piranesi', 'piranesi')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work_credit (work_id, creator_work_id, role, position)
			VALUES (1, 99999, 'author', 0)`)
		return err
	}); err == nil {
		t.Error("work_credit accepted a creator_work_id for a work that does not exist")
	}
}

// TestMigrate0007NoWorkPersonTable pins a deliberate ABSENCE, which is the kind
// of decision that otherwise gets "fixed" by the next reader.
//
// schema.md §1.1: every kind has a subtype table or an explicit justification
// for not having one, and 'person' has the justification — a credited human is a
// name, some external_id rows and the credits pointing at it, all of which
// `work`, `external_id` and `work_credit` already carry. A work_person table
// would hold a birth year and a biography, which no v0.1 source reports.
func TestMigrate0007NoWorkPersonTable(t *testing.T) {
	var n int
	if err := openTestDB(t).Read().QueryRowContext(t.Context(),
		`SELECT count(*) FROM pragma_table_list WHERE name = 'work_person'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("work_person exists. 'person' has NO subtype table by design " +
			"(docs/reference/schema.md §1.1); if that is being reversed it needs an ADR, " +
			"not a migration.")
	}
}

// TestMigrate0007DownAndUp round-trips the migration.
//
// One DROP, and this is what checks the header's claim that nothing else is
// owed: after Down work_credit is gone and NOTHING ELSE changed — asserted over
// the whole schema dump rather than over the one name, because a Down that also
// took an index with it would pass a one-name check.
func TestMigrate0007DownAndUp(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// Later migrations sit on top of 0007; dumpSchemaAt walks down to it first.
	// See TestMigrate0006DownAndUp on why that is a loop and not a hand-counted
	// run of MigrateDown calls.
	at7 := dumpSchemaAt(t, ctx, d, 7)

	migrateDownTo(t, ctx, d, 6)
	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM pragma_table_list WHERE name = 'work_credit'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("work_credit survived the Down block")
	}

	at6 := dumpSchemaAt(t, ctx, d, 6)
	for _, obj := range strings.Split(at7.ddl, "\n\n") {
		if obj == "" || strings.Contains(obj, "work_credit") {
			continue
		}
		if !strings.Contains(at6.ddl, obj) {
			t.Errorf("0007's Down block removed or changed an object it did not create:\n%s", obj)
		}
	}

	rows, err := d.Read().QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		t.Error("PRAGMA foreign_key_check reports a violation after 0007's Down")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// Migrate goes all the way to head, so the dump below comes back down to 7.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate 6 to latest: %v", err)
	}
	again := dumpSchemaAt(t, ctx, d, 7)
	if !again.sameAs(t, at7) {
		t.Error("a Down followed by an Up did not reproduce the schema 0007 creates")
	}
}

// TestMigration0008NeedsNoRebuild is 0003's precedent test written for 0008,
// and it pins the four claims 0008's header makes about ALTER TABLE rather than
// inheriting them:
//
//  1. the column exists, is TEXT, is NULLABLE and has NO default;
//  2. image_asset is still STRICT after the ADD COLUMN;
//  3. NULL and 'jpeg' both round-trip, and NULL is what an insert that names no
//     format gets — so "not encoded yet" is representable and distinguishable;
//  4. there is deliberately NO CHECK, which is checked by inserting a value the
//     vocabulary does not contain and requiring the database to ACCEPT it.
//
// Claim 4 is the one worth having a test for. "No CHECK" is otherwise an absence,
// and an absence is exactly what nobody notices being reversed.
//
// The rebuild this avoids would copy every image_asset row and, because
// work.poster_asset_id and work.backdrop_asset_id both reference the table, would
// drag the 12-step procedure across a parent table.
func TestMigration0008NeedsNoRebuild(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// 1. The column's declared shape. NULLABLE WITH NO DEFAULT is the decision,
	//    not an oversight: an image_asset row is created in state 'pending',
	//    before any encoder has run, so at INSERT time the format is genuinely
	//    unknown. A NOT NULL DEFAULT 'jpeg' would assert an encoding for bytes
	//    that do not exist and leave no value meaning "not encoded yet" — the
	//    have_count NOT NULL DEFAULT 0 shape this project has been bitten by.
	var typ string
	var notNull int
	var dflt sql.NullString
	err := d.Read().QueryRowContext(ctx,
		`SELECT type, "notnull", dflt_value FROM pragma_table_info('image_asset')
		  WHERE name = 'format'`).Scan(&typ, &notNull, &dflt)
	if err != nil {
		t.Fatalf("image_asset has no format column: %v", err)
	}
	if typ != "TEXT" {
		t.Errorf("image_asset.format is %q, want TEXT", typ)
	}
	if notNull != 0 {
		t.Error("image_asset.format is NOT NULL; NULL is how the column says " +
			"'no encoded bytes exist for this row yet', and without it 'encoded as " +
			"JPEG' and 'never encoded' are the same value. See 00008's header.")
	}
	if dflt.Valid {
		t.Errorf("image_asset.format defaults to %q; it must have NO default, so that "+
			"a row created before its encode runs carries NULL rather than a positive "+
			"claim about bytes that do not exist", dflt.String)
	}

	// 2. image_asset is still STRICT after the ADD COLUMN. TestAllTablesAreStrict
	//    covers every table; this pins the one 0008 touched, and it is the claim
	//    that makes the migration a one-liner rather than a rebuild.
	var strict int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT strict FROM pragma_table_list WHERE name = 'image_asset'`).Scan(&strict); err != nil {
		t.Fatalf("reading pragma_table_list for image_asset: %v", err)
	}
	if strict != 1 {
		t.Error("image_asset is no longer STRICT after ADD COLUMN")
	}

	// 3. An insert that names no format gets NULL; one that names 'jpeg' keeps it.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO image_asset (id, source_url, origin_class, role, cache_key)
			VALUES (1, 'https://example.invalid/a.jpg', 'provider', 'poster', 'aaaa')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format)
			VALUES (2, 'https://example.invalid/b.jpg', 'provider', 'poster', 'bbbb', 'jpeg')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var pending sql.NullString
	if err := d.Read().QueryRowContext(ctx,
		`SELECT format FROM image_asset WHERE id = 1`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending.Valid {
		t.Errorf("a row inserted without a format got %q, want NULL", pending.String)
	}
	var encoded string
	if err := d.Read().QueryRowContext(ctx,
		`SELECT format FROM image_asset WHERE id = 2`).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	if encoded != "jpeg" {
		t.Errorf("format round-tripped as %q, want jpeg", encoded)
	}

	// 4. There is NO CHECK constraint, and this asserts the absence by exercising
	//    it. The vocabulary is expected to grow — ADR-0050 defers AVIF with a
	//    named reopening condition rather than rejecting it — and SQLite cannot
	//    ALTER an existing CHECK, so a constraint here would schedule a 12-step
	//    rebuild of a table two `work` columns reference. ADR-0039 made the same
	//    call for write_queue.state.
	//
	//    ⚠️ A CHECK was genuinely available and was declined, rather than being
	//    impossible. Measured on this tree's SQLite 3.53.4: ADD COLUMN accepts a
	//    column CHECK on a STRICT table and enforces it. 00003's header sentence
	//    calling a CHECK "the one thing ALTER TABLE cannot express" is imprecise —
	//    what ALTER TABLE cannot do is ALTER or DROP an EXISTING one.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format)
			VALUES (3, 'https://example.invalid/c.bin', 'provider', 'poster', 'cccc', 'not-a-codec')`)
		return err
	}); err != nil {
		t.Errorf("the database REJECTED an unknown format value: %v\n"+
			"image_asset.format must carry no CHECK constraint — the codec vocabulary "+
			"grows (jpeg today, avif on ADR-0050's reopening, possibly webp) and SQLite "+
			"cannot ALTER a CHECK. The vocabulary is enforced in internal/store/images.go "+
			"and guarded by TestImageWritesValidateTheFormatVocabulary.", err)
	}

	// 5. STRICT buys almost NOTHING for this column, and that is measured here
	//    rather than assumed — because assuming it is how a reader talks
	//    themselves out of the Go validator.
	//
	//    A STRICT TEXT column converts a value "if and only if the conversion is
	//    lossless and reversible", so INTEGER 7 and REAL 1.5 are ACCEPTED and
	//    stored as '7' and '1.5'. Measured on this tree's SQLite 3.53.4. Only a
	//    BLOB is refused. An earlier draft of this test asserted that STRICT
	//    rejects an integer here and the test itself falsified it.
	//
	//    ⚠️ The same measurement falsifies a sentence in this file: item 2 of
	//    TestMigration0003NeedsNoRebuild says "losing STRICT here would silently
	//    accept an integer state". STRICT accepts an integer state either way.
	//    That comment is 0003's and is left alone by this migration; recorded as
	//    LS-287 rather than fixed here.
	//
	//    So the only thing standing between image_asset.format and a garbage
	//    value is internal/store/images.go. That is the argument for shipping the
	//    validator with the column instead of promising it.
	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format)
			VALUES (4, 'https://example.invalid/d.bin', 'provider', 'poster', 'dddd', 7)`)
		return err
	}); err != nil {
		t.Errorf("a STRICT TEXT column refused the integer 7: %v\n"+
			"If this starts failing, SQLite's STRICT coercion rules changed and the "+
			"reasoning above — that STRICT protects this column from nothing but a BLOB — "+
			"needs re-measuring.", err)
	}
	var coerced string
	if err := d.Read().QueryRowContext(ctx,
		`SELECT format FROM image_asset WHERE id = 4`).Scan(&coerced); err != nil {
		t.Fatal(err)
	}
	if coerced != "7" {
		t.Errorf("the integer 7 stored as %q, want the string \"7\"", coerced)
	}
}

// TestMigrate0008DownAndUp mirrors 0007's round-trip check for a column rather
// than a table: the Down block must remove exactly `format` and leave every other
// schema object byte-identical, and a following Up must reproduce 0008 exactly.
//
// Unlike 00003's Down, this one drops no index first — 0008 creates none, and
// SQLite's refusal to DROP COLUMN on an indexed column is therefore not in play.
// If someone later adds an index over format without amending the Down block,
// this test is what fails.
func TestMigrate0008DownAndUp(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	// Later migrations sit on top of 0008; dumpSchemaAt walks down to it first.
	// See TestMigrate0006DownAndUp on why that is a loop and not a hand-counted
	// run of MigrateDown calls.
	at8 := dumpSchemaAt(t, ctx, d, 8)

	migrateDownTo(t, ctx, d, 7)
	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM pragma_table_info('image_asset') WHERE name = 'format'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("image_asset.format survived the Down block")
	}

	at7 := dumpSchemaAt(t, ctx, d, 7)
	for _, obj := range strings.Split(at8.ddl, "\n\n") {
		if obj == "" || strings.Contains(obj, "image_asset") {
			continue
		}
		if !strings.Contains(at7.ddl, obj) {
			t.Errorf("0008's Down block removed or changed an object it did not create:\n%s", obj)
		}
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate 7 to latest: %v", err)
	}
	again := dumpSchemaAt(t, ctx, d, 8)
	if !again.sameAs(t, at8) {
		t.Error("a Down followed by an Up did not reproduce the schema 0008 creates")
	}
}

// TestMigrate0009IndexShape pins the index this migration exists to create —
// its name, its table, its column order and its uniqueness — read back out of
// SQLite rather than out of the migration file it was written in.
//
// THE COLUMN ORDER IS THE WHOLE DECISION and is asserted as an ordered list.
// (format, work_id) makes the browse read's Audiobooks probe a two-column
// covered seek; (work_id, format) would duplicate ix_edition_work's leading
// column and serve the probe no better than ix_edition_work already does. A
// test that asserted only "an index named ix_edition_format exists on edition"
// would stay green through exactly that reversal.
func TestMigrate0009IndexShape(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	var unique int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT "unique" FROM pragma_index_list('edition') WHERE name = 'ix_edition_format'`,
	).Scan(&unique); err != nil {
		t.Fatalf("ix_edition_format is not an index on edition: %v", err)
	}
	if unique != 0 {
		t.Error("ix_edition_format is UNIQUE. A work may legitimately hold two editions " +
			"of one format — two ebook releases of the same book — so uniqueness here " +
			"would reject real rows.")
	}

	rows, err := d.Read().QueryContext(ctx,
		`SELECT name FROM pragma_index_info('ix_edition_format') ORDER BY seqno`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(cols, ","), "format,work_id"; got != want {
		t.Errorf("ix_edition_format is on (%s), want (%s). The order is the decision — see "+
			"the migration header and ADR-0051.", got, want)
	}

	// NOT PARTIAL. `edition` has no soft-delete column, so there is no
	// tombstone predicate to make this partial on — and a partial index would
	// silently drop the NULL-format rows the Ebooks side of §17.2's split has
	// to be able to see.
	var ddl string
	if err := d.Read().QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'ix_edition_format'`,
	).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(ddl), " WHERE ") {
		t.Errorf("ix_edition_format is partial: %s\n"+
			"It must index every edition row, NULL formats included.", ddl)
	}
}

// TestMigrate0009NullFormatsAreIndexed executes the claim above rather than
// asserting the absence of a WHERE clause and calling it proof. A NULL
// `edition.format` is a first-class state — migration 0005's CHECK note says so
// — and `NOT EXISTS (… format IS NOT 'audiobook')`, which is half of the
// Audiobooks filter, has to see those rows or a book with one unset edition
// files as an audiobook.
func TestMigrate0009NullFormatsAreIndexed(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range []string{
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (1, 'book', 'Piranesi', 'piranesi', 'piranesi')`,
			`INSERT INTO edition (id, work_id, format) VALUES (1, 1, 'audiobook')`,
			`INSERT INTO edition (id, work_id, format) VALUES (2, 1, NULL)`,
		} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM edition INDEXED BY ix_edition_format WHERE format IS NULL`,
	).Scan(&n); err != nil {
		t.Fatalf("reading NULL formats through the index: %v", err)
	}
	if n != 1 {
		t.Errorf("the index reports %d NULL-format editions, want 1", n)
	}
}

// TestMigrate0009DownAndUp round-trips the migration.
//
// One DROP INDEX, and this checks the header's claim that nothing else is owed:
// after Down ix_edition_format is gone and NOTHING ELSE changed — asserted over
// the whole schema dump rather than over the one name, because a Down that also
// took a table with it would pass a one-name check.
//
// ⚠️ An index Down is the one rollback in this directory that cannot lose data,
// which the migration header states and this test is the execution of: the row
// count over `edition` is taken before and after, and it does not move.
func TestMigrate0009DownAndUp(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range []string{
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (1, 'book', 'Piranesi', 'piranesi', 'piranesi')`,
			`INSERT INTO edition (id, work_id, format) VALUES (1, 1, 'ebook')`,
			`INSERT INTO edition (id, work_id, format) VALUES (2, 1, 'audiobook')`,
		} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	// ⚠️ THE 9 IS THE POINT. openTestDB migrates to LATEST, so a dump that did
	// not step back to 9 would capture whatever landed after 0009, and the
	// "removed an object it did not create" comparison below would then blame
	// 0009's Down block for rolling back a later migration. Measured: it did,
	// on all three of 0010's indexes, the moment 0010 landed. dumpSchemaAt is
	// why that line can no longer be left out — its doc carries the history.
	at9 := dumpSchemaAt(t, ctx, d, 9)

	migrateDownTo(t, ctx, d, 8)
	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'ix_edition_format'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("ix_edition_format survived the Down block")
	}
	if err := d.Read().QueryRowContext(ctx, `SELECT count(*) FROM edition`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("%d edition rows after the Down, want 2 — an index rollback must not "+
			"touch a row", n)
	}

	at8 := dumpSchemaAt(t, ctx, d, 8)
	for _, obj := range strings.Split(at9.ddl, "\n\n") {
		if obj == "" || strings.Contains(obj, "ix_edition_format") {
			continue
		}
		if !strings.Contains(at8.ddl, obj) {
			t.Errorf("0009's Down block removed or changed an object it did not create:\n%s", obj)
		}
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate 8 to latest: %v", err)
	}
	// Back to 9 for the same reason the first dump stepped back to it: `again`
	// has to be the schema at the version this test is about. sameAs enforces
	// that the two dumps name the SAME version, which the 9 above and the 9
	// here would otherwise only agree on by inspection.
	again := dumpSchemaAt(t, ctx, d, 9)
	if !again.sameAs(t, at9) {
		t.Error("a Down followed by an Up did not reproduce the schema 0009 creates")
	}
	if err := d.Read().QueryRowContext(ctx, `SELECT count(*) FROM edition`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("%d edition rows after the re-Up, want 2", n)
	}
}

// TestMigrate0010IndexShapes pins the three indexes this migration exists to
// create — name, table, column, uniqueness and partiality — read back out of
// SQLite rather than out of the migration file they were written in.
//
// THE PARTIALITY IS THE DECISION on the two `work` indexes and is asserted
// rather than described. Both columns are NULL for every work with no artwork,
// which during §4.4.1's cold start is most of the library and on this tree is
// all of it; a full index over them would hold one entry per work, nearly all
// NULL and none of them ever sought. A test that asserted only "an index named
// ix_work_poster exists on work" would stay green through the loss of the WHERE
// clause.
func TestMigrate0010IndexShapes(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	for _, tc := range []struct {
		index   string
		table   string
		column  string
		partial string // "" means the index must NOT be partial
	}{
		// The route key. NOT partial: cache_key is TEXT NOT NULL, so there is
		// no NULL half to exclude. NOT unique either — the read fails closed on
		// a duplicate instead, because the writer that would owe a constraint
		// is not built.
		{"ix_img_cache_key", "image_asset", "cache_key", ""},
		{"ix_work_poster", "work", "poster_asset_id", "poster_asset_id   IS NOT NULL"},
		{"ix_work_backdrop", "work", "backdrop_asset_id", "backdrop_asset_id IS NOT NULL"},
	} {
		t.Run(tc.index, func(t *testing.T) {
			var unique int
			if err := d.Read().QueryRowContext(ctx,
				`SELECT "unique" FROM pragma_index_list(?) WHERE name = ?`,
				tc.table, tc.index,
			).Scan(&unique); err != nil {
				t.Fatalf("%s is not an index on %s: %v", tc.index, tc.table, err)
			}
			if unique != 0 {
				t.Errorf("%s is UNIQUE. Migration 00010's header states why none of the "+
					"three is: the writer that would owe the constraint is not built, and "+
					"internal/store.LookupImageAsset refuses an ambiguous key instead.",
					tc.index)
			}

			cols := indexColumns(t, ctx, d, tc.index)
			if got, want := strings.Join(cols, ","), tc.column; got != want {
				t.Errorf("%s is on (%s), want (%s)", tc.index, got, want)
			}

			var ddl string
			if err := d.Read().QueryRowContext(ctx,
				`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, tc.index,
			).Scan(&ddl); err != nil {
				t.Fatal(err)
			}
			switch tc.partial {
			case "":
				if strings.Contains(strings.ToUpper(ddl), " WHERE ") {
					t.Errorf("%s is partial and must not be: %s", tc.index, ddl)
				}
			default:
				if !strings.Contains(ddl, "WHERE "+tc.partial) {
					t.Errorf("%s is not partial on %q: %s\n"+
						"A full index over a column that is NULL for most works holds one "+
						"entry per work, almost none of them ever sought.",
						tc.index, tc.partial, ddl)
				}
			}
		})
	}
}

// TestMigrate0010PartialIndexesAreUsable executes the claim the partiality rests
// on rather than asserting the presence of a WHERE clause and calling it proof.
//
// SQLite will only use a partial index when the query's WHERE clause IMPLIES the
// index's. `poster_asset_id = ?` implies `poster_asset_id IS NOT NULL` because
// `=` is never true of NULL — but that implication is SQLite's to make, not this
// migration's to assert, and if it ever stopped making it the two indexes would
// be dead weight that no plan mentions and no test noticed. `INDEXED BY` forces
// the question: SQLite raises "no query solution" if the index is unusable for
// the statement, so this either runs or fails loudly.
func TestMigrate0010PartialIndexesAreUsable(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range []string{
			`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, state)
			   VALUES (1, 'http://k/1', 'configured', 'poster', 'aaaaaaaaaaaaaaaa', 'ready')`,
			`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, state)
			   VALUES (2, 'http://k/2', 'configured', 'backdrop', 'bbbbbbbbbbbbbbbb', 'ready')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title,
			                   poster_asset_id, backdrop_asset_id)
			   VALUES (1, 'comic', 'Berserk', 'berserk', 'berserk', 1, 2)`,
			// A work with NO artwork: the row the partial predicate excludes.
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (2, 'comic', 'Vagabond', 'vagabond', 'vagabond')`,
		} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	for _, tc := range []struct {
		index  string
		column string
		asset  int64
	}{
		{"ix_work_poster", "poster_asset_id", 1},
		{"ix_work_backdrop", "backdrop_asset_id", 2},
	} {
		var n int
		q := `SELECT count(*) FROM work INDEXED BY ` + tc.index + ` WHERE ` + tc.column + ` = ?`
		if err := d.Read().QueryRowContext(ctx, q, tc.asset).Scan(&n); err != nil {
			t.Fatalf("%s is not usable for `%s = ?`: %v\n"+
				"The partial predicate is `%s IS NOT NULL` and the query's equality is "+
				"supposed to imply it; if SQLite disagrees, the index is dead weight.",
				tc.index, tc.column, err, tc.column)
		}
		if n != 1 {
			t.Errorf("%s reports %d rows through the index, want 1", tc.index, n)
		}
	}

	// And the route key, which is not partial and must find its row.
	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM image_asset INDEXED BY ix_img_cache_key WHERE cache_key = ?`,
		"aaaaaaaaaaaaaaaa").Scan(&n); err != nil {
		t.Fatalf("ix_img_cache_key is not usable for `cache_key = ?`: %v", err)
	}
	if n != 1 {
		t.Errorf("ix_img_cache_key reports %d rows, want 1", n)
	}
}

// TestMigrate0010DownAndUp round-trips the migration.
//
// Three DROP INDEXes, and this checks the header's claim that nothing else is
// owed: after Down the three are gone and NOTHING ELSE changed — asserted over
// the whole schema dump rather than over three names, because a Down that also
// took a table with it would pass a name check.
//
// ⚠️ An index Down cannot lose data, which the header claims and this executes:
// the row counts over `work` and `image_asset` are taken before and after and do
// not move. It cannot widen an answer either — the access-scope predicate
// LookupImageAsset carries is in the SQL, and dropping an index cannot remove a
// WHERE clause.
func TestMigrate0010DownAndUp(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range []string{
			`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, state)
			   VALUES (1, 'http://k/1', 'configured', 'poster', 'aaaaaaaaaaaaaaaa', 'ready')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title, poster_asset_id)
			   VALUES (1, 'comic', 'Berserk', 'berserk', 'berserk', 1)`,
		} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	created := []string{"ix_img_cache_key", "ix_work_poster", "ix_work_backdrop"}

	// ⚠️ THE 10 IS THE POINT — openTestDB migrates to LATEST, so a dump that
	// did not step back to 10 would capture whatever landed after this
	// migration, and the "removed an object it did not create" comparison would
	// then blame THIS Down block for rolling back a later one. 0009's
	// round-trip lacked that step and 0010 broke it on all three indexes. The
	// warning this comment used to carry — "the line 0011 will need" — did not
	// stop 0011 landing without it, which is why the step now lives inside
	// dumpSchemaAt rather than in a comment.
	at10 := dumpSchemaAt(t, ctx, d, 10)

	migrateDownTo(t, ctx, d, 9)

	for _, index := range created {
		var n int
		if err := d.Read().QueryRowContext(ctx,
			`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s survived the Down block", index)
		}
	}
	for _, table := range []string{"work", "image_asset"} {
		var n int
		if err := d.Read().QueryRowContext(ctx,
			`SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%d %s rows after the Down, want 1 — an index rollback must not "+
				"touch a row", n, table)
		}
	}

	at9 := dumpSchemaAt(t, ctx, d, 9)
	for _, obj := range strings.Split(at10.ddl, "\n\n") {
		if obj == "" {
			continue
		}
		if slices.ContainsFunc(created, func(name string) bool { return strings.Contains(obj, name) }) {
			continue
		}
		if !strings.Contains(at9.ddl, obj) {
			t.Errorf("0010's Down block removed or changed an object it did not create:\n%s", obj)
		}
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate 9 to latest: %v", err)
	}
	again := dumpSchemaAt(t, ctx, d, 10)
	if !again.sameAs(t, at10) {
		t.Error("a Down followed by an Up did not reproduce the schema 0010 creates")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Migration 0011 — ix_sync_report_container_latest.
//
// The index that takes the per-library_source sort out of
// libraryCompletenessSQL's newest-verdict subquery. What it buys is a PLAN, and
// a plan is not a schema fact, so it is pinned where it can be measured against
// the shipped statement: internal/store's
// TestLibraryCompletenessPlanIsSeeksAndOneSort asserts the plan and
// TestLibraryCompletenessPlanGuardFires drops this index and watches the sort
// come back. What THESE tests own is the schema half — that the index exists,
// that it is on the columns the plan depends on in the order it depends on, that
// the index it sits BESIDE is still there, and that the Down block takes nothing
// else with it.
// ─────────────────────────────────────────────────────────────────────────────

// TestMigrate0011IndexShape pins all five columns AND their order.
//
// The order is the decision, the same way it was for 0009 and 0010: the first
// four are the subquery's equality predicates and the fifth is the `ORDER BY
// r2.id DESC LIMIT 1` the temp b-tree used to serve. A test that asserted only
// "an index named ix_sync_report_container_latest exists on sync_report" would
// stay green through a reordering that moved `id` into the middle — and that
// reordering was tried while writing this: it drops the seek from four
// constrained columns to two, which is the regression this pins.
func TestMigrate0011IndexShape(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	var unique int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT "unique" FROM pragma_index_list('sync_report')
		  WHERE name = 'ix_sync_report_container_latest'`,
	).Scan(&unique); err != nil {
		t.Fatalf("ix_sync_report_container_latest is not an index on sync_report: %v", err)
	}
	if unique != 0 {
		t.Error("ix_sync_report_container_latest is UNIQUE. sync_report is APPEND-ONLY and " +
			"every import writes a fresh row for the same (instance, kind, remote_kind, " +
			"remote_id) tuple — uniqueness here would reject the second import, which is " +
			"the exact history the newest-verdict subquery reads.")
	}

	const wantCols = "service_instance_id,kind,remote_kind,remote_id,id"
	got := strings.Join(indexColumns(t, ctx, d, "ix_sync_report_container_latest"), ",")
	if got != wantCols {
		t.Errorf("ix_sync_report_container_latest is on (%s), want (%s).\n"+
			"The first four are libraryCompletenessSQL's equality predicates and the "+
			"fifth is its ORDER BY. See the migration header.", got, wantCols)
	}

	// 🔍 THE TRAILING `id` IS REDUNDANT AND EXPLICIT ON PURPOSE — `id` is INTEGER
	// PRIMARY KEY, so SQLite appends it as the rowid anyway, and the four-column
	// index measures a byte-identical plan, `COVERING` and all. It is pinned
	// above so the ordering guarantee lives in the DDL rather than resting on
	// that unstated identity. This asserts the identity the redundancy depends
	// on, so a future rebuild that breaks it fails HERE, naming the reason,
	// rather than as a mystery sort in a store-package plan guard.
	var typ string
	var pk int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT type, pk FROM pragma_table_info('sync_report') WHERE name = 'id'`,
	).Scan(&typ, &pk); err != nil {
		t.Fatal(err)
	}
	if typ != "INTEGER" || pk != 1 {
		t.Errorf("sync_report.id is %s (pk=%d), not the INTEGER PRIMARY KEY the migration "+
			"header reasons from. The trailing `id` column stops being redundant the "+
			"moment that changes — re-read the header before touching the index.", typ, pk)
	}

	// NOT PARTIAL — and unlike 0010's two `work` indexes, that is the right call
	// here rather than an oversight: sync_report has no soft-delete column, and
	// remote_kind / remote_id are both nullable, so a report naming no container
	// is a real row that must stay indexed.
	var ddl string
	if err := d.Read().QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master
		  WHERE type = 'index' AND name = 'ix_sync_report_container_latest'`,
	).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(ddl), " WHERE ") {
		t.Errorf("ix_sync_report_container_latest is partial: %s\n"+
			"It must index every sync_report row, NULL remote_kind/remote_id included.", ddl)
	}
}

// TestMigrate0011KeepsTheInstanceIndex executes the migration header's loudest
// claim: 0011 ADDS an index and removes NOTHING.
//
// ix_sync_report_instance is (service_instance_id, created_at DESC) and serves
// 00005's stated reader — the Services screen's "this instance's recent reports,
// newest first" — which ix_sync_report_container_latest cannot serve at all,
// because created_at is not in it and three equality columns sit between the
// instance and any ordering. A later tidy-up that reads "two indexes on one
// small table" and drops either one is a regression in one of the two reads, and
// only one of those two halves is visible from internal/store. This is the other
// half; internal/store's TestLibraryCompletenessDoesNotDependOnTheInstanceIndex
// is its counterpart.
func TestMigrate0011KeepsTheInstanceIndex(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	names := map[string]bool{}
	rows, err := d.Read().QueryContext(ctx, `SELECT name FROM pragma_index_list('sync_report')`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names[n] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ix_sync_report_instance", "ix_sync_report_container_latest"} {
		if !names[want] {
			t.Errorf("sync_report has no %s. BOTH indexes ship: the 0011 header says which "+
				"read each one serves and why neither subsumes the other.", want)
		}
	}

	// And the Services read still seeks, and still does not sort. This is the
	// half 0011's own plan guard cannot see, because that guard EXPLAINs a
	// different statement.
	plan, err := QueryPlan(ctx, d.Read(),
		`SELECT id, kind, detail, created_at FROM sync_report
		  WHERE service_instance_id = ? ORDER BY created_at DESC LIMIT 20`, 1)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, " | ")
	if !strings.Contains(joined, "USING INDEX ix_sync_report_instance") {
		t.Errorf("the Services screen's recent-reports read no longer seeks on "+
			"ix_sync_report_instance:\n  %s\n"+
			"0011 adds an index BESIDE that one and must not have changed this plan.",
			strings.Join(plan, "\n  "))
	}
	if strings.Contains(joined, "USE TEMP B-TREE") {
		t.Errorf("the Services screen's recent-reports read now SORTS:\n  %s\n"+
			"ix_sync_report_instance's trailing `created_at DESC` exists precisely so "+
			"that it does not.", strings.Join(plan, "\n  "))
	}
}

// TestMigrate0011DownAndUp round-trips the migration.
//
// One DROP INDEX, and this checks the header's claim that nothing else is owed:
// after Down ix_sync_report_container_latest is gone and NOTHING ELSE changed —
// asserted over the whole schema dump rather than over the one name, because a
// Down that also took ix_sync_report_instance with it would pass a one-name
// check, and dropping that OTHER index is the specific mistake the header warns
// about.
//
// ⚠️ An index Down cannot lose data, which the migration header states and this
// test is the execution of: the sync_report row count is taken before and after,
// and it does not move.
func TestMigrate0011DownAndUp(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	if err := d.Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range []string{
			`INSERT INTO service_instance (id, kind, name, base_url, api_key_enc)
			   VALUES (1, 'sonarr', 'Sonarr', 'http://sonarr:8989', x'00')`,
			`INSERT INTO sync_report (id, service_instance_id, kind, remote_kind, remote_id, detail)
			   VALUES (1, 1, 'content_completeness', 'library', '11', '{}')`,
			// A row with NULL remote_kind/remote_id — the not-partial claim, made
			// executable below.
			`INSERT INTO sync_report (id, service_instance_id, kind, detail)
			   VALUES (2, 1, 'items_skipped', '{}')`,
		} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	// NOT PARTIAL, executed rather than inferred from the absence of a WHERE
	// clause: forcing the index must still find the NULL-container row.
	var n int
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM sync_report INDEXED BY ix_sync_report_container_latest
		  WHERE service_instance_id = 1 AND kind = 'items_skipped'`).Scan(&n); err != nil {
		t.Fatalf("reading NULL-container reports through the index: %v", err)
	}
	if n != 1 {
		t.Errorf("the index reports %d NULL-container rows, want 1 — a report that names "+
			"no container is a real row and 0011 must index it", n)
	}

	// ⚠️ THE 11 IS THE POINT, and it is armed the moment 0012 lands. openTestDB
	// migrates to LATEST, so a dump that did not step back to 11 would capture
	// whatever landed AFTER 0011, and the "removed an object it did not create"
	// comparison would then blame THIS Down block for rolling back a later one.
	// 0009's round-trip lacked that step and 0010 broke it. This test is the
	// reason the step now lives inside dumpSchemaAt: 0010's comment spelled out
	// the line 0011 would need, and 0011 landed without it anyway.
	at11 := dumpSchemaAt(t, ctx, d, 11)

	migrateDownTo(t, ctx, d, 10)
	if err := d.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master
		  WHERE type = 'index' AND name = 'ix_sync_report_container_latest'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("ix_sync_report_container_latest survived the Down block")
	}
	if err := d.Read().QueryRowContext(ctx, `SELECT count(*) FROM sync_report`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("%d sync_report rows after the Down, want 2 — an index rollback must not "+
			"touch a row", n)
	}

	at10 := dumpSchemaAt(t, ctx, d, 10)
	for _, obj := range strings.Split(at11.ddl, "\n\n") {
		if obj == "" || strings.Contains(obj, "ix_sync_report_container_latest") {
			continue
		}
		if !strings.Contains(at10.ddl, obj) {
			t.Errorf("0011's Down block removed or changed an object it did not create:\n%s", obj)
		}
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate 10 to latest: %v", err)
	}
	// TO VERSION 11 AGAIN, for the same reason: Migrate returns the DB to head,
	// and `again` must be read at 0011 to be comparable with at11. sameAs is
	// what makes "the same version" checked rather than assumed.
	again := dumpSchemaAt(t, ctx, d, 11)
	if !again.sameAs(t, at11) {
		t.Error("a Down followed by an Up did not reproduce the schema 0011 creates")
	}
	if err := d.Read().QueryRowContext(ctx, `SELECT count(*) FROM sync_report`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("%d sync_report rows after the re-Up, want 2", n)
	}
}

// indexColumns reads an index's columns in seqno order.
func indexColumns(t *testing.T, ctx context.Context, d *DB, index string) []string {
	t.Helper()
	rows, err := d.Read().QueryContext(ctx,
		`SELECT name FROM pragma_index_info(?) ORDER BY seqno`, index)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return cols
}
