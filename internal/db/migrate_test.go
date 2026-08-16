package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
const latestSchemaVersion int64 = 4

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

// The tables deferred to the library-sync migration must NOT be here. A
// migration that creates a table nothing queries is a schema claim nobody has
// tested.
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

	for _, deferred := range []string{
		"work", "work_movie", "work_series", "work_episode", "edition", "media_file",
		"external_id", "service_item_link", "service_item_alias",
		"search_doc", "search_fts", "image_asset", "sync_report",
		"tag_rule", "tag_alias", "tag_implies", "saved_filter", "work_relation",
	} {
		if present[deferred] {
			t.Errorf("%s is in migration 0001; it belongs in the library-sync migration", deferred)
		}
	}

	for _, want := range []string{
		"user", "session", "client_credential", "audit_log", "service_instance",
		"release_candidate", "provenance", "write_queue", "tag", "tag_assignment",
	} {
		if !present[want] {
			t.Errorf("%s is missing from migration 0001", want)
		}
	}
}

// Every table in migration 0001 is STRICT. A non-STRICT table accepts a string
// into an INTEGER column, which is how a type bug reaches disk.
func TestAllTablesAreStrict(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	rows, err := d.Read().QueryContext(ctx, `
		SELECT name, sql FROM sqlite_schema
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ToUpper(ddl), "STRICT") {
			t.Errorf("table %s is not STRICT", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
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
// `state` is the control: the identical pattern without NULL in the list. If
// only the state assertions pass, the nullable form has regressed.
func TestNullableCheckConstraintsActuallyConstrain(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	insert := func(t *testing.T, id int, col, val string) error {
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
		if err := insert(t, 10+i, "fail_reason", v); err != nil {
			t.Errorf("fail_reason=%s was rejected but is legal: %v", v, err)
		}
	}

	// Garbage must not be.
	if err := insert(t, 20, "fail_reason", "'TOTAL-GARBAGE'"); err == nil {
		t.Error("fail_reason='TOTAL-GARBAGE' was ACCEPTED — the CHECK is a no-op.\n" +
			"A nullable column must use `CHECK (col IS NULL OR col IN (...))`; " +
			"putting NULL inside the IN list poisons the comparison and accepts everything.")
	}

	// Control: the same pattern without NULL in the list rejects correctly. If
	// this fails, something broader is wrong with CHECK enforcement.
	if err := insert(t, 21, "state", "'TOTAL-GARBAGE'"); err == nil {
		t.Error("state='TOTAL-GARBAGE' was accepted; CHECK constraints are not being enforced at all")
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
