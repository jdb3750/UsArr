package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ncruces/go-sqlite3/driver"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/db"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(config.Options{
		Args: []string{"--config-dir", t.TempDir()},
		Env:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg
}

// A first run has nothing to lose, so there must be no backup and no empty
// backups/ full of zero-byte files.
func TestNoBackupOnAFreshDatabase(t *testing.T) {
	cfg := testConfig(t)
	path, err := backupBeforeMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("backupBeforeMigrate: %v", err)
	}
	if path != "" {
		t.Fatalf("a fresh install took a backup: %s", path)
	}
}

// An already-current database has nothing to protect against. Backing up on
// every start would fill the config volume with identical copies.
func TestNoBackupWhenTheSchemaIsCurrent(t *testing.T) {
	cfg := testConfig(t)
	ctx := context.Background()

	database, err := db.Open(ctx, cfg.DatabasePath())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	path, err := backupBeforeMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("backupBeforeMigrate: %v", err)
	}
	if path != "" {
		t.Fatalf("an up-to-date database took a backup: %s", path)
	}
}

// A database behind the newest embedded migration gets a VACUUM INTO snapshot
// before anything touches it, at 0600 in a 0700 directory, and the snapshot
// must actually open.
func TestBackupTakenWhenAMigrationIsPending(t *testing.T) {
	cfg := testConfig(t)
	ctx := context.Background()

	// A real SQLite file with no goose table at all: schema version 0, which is
	// behind migration 00001. This is also the shape of a database written by a
	// build that predates goose.
	conn, err := driver.Open(bareDSN(cfg.DatabasePath()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE legacy (id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO legacy (note) VALUES ('irreplaceable')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if v, err := appliedSchemaVersion(ctx, cfg.DatabasePath()); err != nil || v != 0 {
		t.Fatalf("appliedSchemaVersion = %d, %v; want 0, nil", v, err)
	}
	newest, err := newestEmbeddedMigration()
	if err != nil || newest < 1 {
		t.Fatalf("newestEmbeddedMigration = %d, %v", newest, err)
	}

	path, err := backupBeforeMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("backupBeforeMigrate: %v", err)
	}
	if path == "" {
		t.Fatal("a pending migration took no backup")
	}
	if filepath.Dir(path) != cfg.BackupsDir() {
		t.Errorf("backup landed in %s, want %s", filepath.Dir(path), cfg.BackupsDir())
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Mode().Perm() != config.FileMode {
		t.Errorf("backup mode = %04o, want %04o", info.Mode().Perm(), config.FileMode)
	}
	if dirInfo, err := os.Stat(cfg.BackupsDir()); err != nil {
		t.Fatalf("stat backups dir: %v", err)
	} else if dirInfo.Mode().Perm() != config.DirMode {
		t.Errorf("backups dir mode = %04o, want %04o", dirInfo.Mode().Perm(), config.DirMode)
	}

	// The snapshot must open and hold the data. A backup that does not restore
	// is not a backup.
	restored, err := driver.Open(bareDSN(path))
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = restored.Close() }()
	var note string
	if err := restored.QueryRowContext(ctx, `SELECT note FROM legacy`).Scan(&note); err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if note != "irreplaceable" {
		t.Errorf("backup content = %q", note)
	}

	// The key must never be inside anything the backup job writes.
	if _, err := os.Stat(filepath.Join(cfg.BackupsDir(), "keys")); !os.IsNotExist(err) {
		t.Error("keys/ must never appear under backups/")
	}
}
