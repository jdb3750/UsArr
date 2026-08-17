package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
const latestSchemaVersion int64 = 5

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
// when it leaves the six unbuilt subtype tables out.
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

	// Deferred, with the milestone that owns each. The six work_* subtypes wait
	// for the catalogue source that writes them (ADR-0035, ADR-0036); the rest
	// is schema.md's "later tables" appendix.
	deferred := []string{
		// music — lands with Navidrome (§16.1 position 1 or 2)
		"work_album", "work_track", "work_credit",
		// books and comics — lands with Kavita (§16.1 position 1 or 2)
		"work_book", "work_comic", "work_comic_issue",
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
				"see 00005's header for which milestone owns it", name)
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
	}
	for _, name := range want {
		if !present[name] {
			t.Errorf("%s is missing; it should be created by migrations 0001-0005", name)
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
