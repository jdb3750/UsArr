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

// Down is not a supported user path, but it must work, because it is the
// cheapest way to test a migration locally. A Down that leaves objects behind
// makes the next Up fail in a way that only shows up on a developer's machine.
func TestMigrationDownLeavesNothingBehind(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	v, err := d.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v != 1 {
		t.Fatalf("schema version = %d, want 1", v)
	}

	if err := d.MigrateDown(ctx); err != nil {
		t.Fatalf("MigrateDown: %v", err)
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
	if v, err = d.Version(ctx); err != nil || v != 1 {
		t.Fatalf("version after re-Up = %d (err %v), want 1", v, err)
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
