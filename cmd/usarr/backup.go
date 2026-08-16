package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	// The driver is imported here, and only here outside internal/db, for the
	// pre-migration backup: it needs a connection to the database BEFORE
	// db.Open runs the migrations, and db.Open deliberately does both in one
	// call so no caller can open an unmigrated database by accident.
	"github.com/ncruces/go-sqlite3/driver"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/db"
)

// backupBeforeMigrate takes a VACUUM INTO snapshot when, and only when, a
// migration is about to run against an existing database.
//
// ARCHITECTURE.md §15 requires an automatic pre-migration backup. Two details
// decide whether it is useful or merely noisy:
//
//   - It must be VACUUM INTO, not `cp`. In WAL mode the newest committed
//     transactions live in usarr.db-wal, so a bare copy of the main file is a
//     torn, older database that may not even open (CONFIGURATION.md §6.2).
//   - It must not run on every start. A fresh database has nothing to lose, and
//     an up-to-date one has nothing to protect against, so the check is "is the
//     applied goose version behind the newest embedded migration".
//
// The backup goes to $USARR_CONFIG_DIR/backups at mode 0600 in a 0700
// directory. It holds ciphertext and never a key: keys/ is a sibling, outside
// everything the backup and host-migration instructions copy.
func backupBeforeMigrate(ctx context.Context, cfg *config.Config) (string, error) {
	dbPath := cfg.DatabasePath()
	info, err := os.Stat(dbPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", nil // first run: nothing to back up
	case err != nil:
		return "", fmt.Errorf("stat %s: %w", dbPath, err)
	case info.Size() == 0:
		return "", nil
	}

	applied, err := appliedSchemaVersion(ctx, dbPath)
	if err != nil {
		return "", err
	}
	newest, err := newestEmbeddedMigration()
	if err != nil {
		return "", err
	}
	if applied >= newest {
		return "", nil
	}

	if err := os.MkdirAll(cfg.BackupsDir(), config.DirMode); err != nil {
		return "", fmt.Errorf("create %s: %w", cfg.BackupsDir(), err)
	}
	if err := os.Chmod(cfg.BackupsDir(), config.DirMode); err != nil {
		return "", fmt.Errorf("chmod %s: %w", cfg.BackupsDir(), err)
	}

	name := fmt.Sprintf("usarr-pre-migration-%s-v%d.db",
		time.Now().UTC().Format("2006-01-02T15-04-05Z"), applied)
	target := filepath.Join(cfg.BackupsDir(), name)

	conn, err := driver.Open(bareDSN(dbPath))
	if err != nil {
		return "", fmt.Errorf("open %s for backup: %w", dbPath, err)
	}
	defer conn.Close()

	// VACUUM INTO takes a literal, not a bound parameter, so the path is
	// escaped by doubling single quotes. The path comes from the operator's own
	// configuration, not from a request, but escaping it costs one line.
	literal := "'" + strings.ReplaceAll(target, "'", "''") + "'"
	if _, err := conn.ExecContext(ctx, "VACUUM INTO "+literal); err != nil {
		return "", fmt.Errorf("VACUUM INTO %s: %w", target, err)
	}
	if err := os.Chmod(target, config.FileMode); err != nil {
		return "", fmt.Errorf("chmod %s: %w", target, err)
	}
	return target, nil
}

// appliedSchemaVersion reads goose's own version table without migrating.
//
// A database that predates goose, or one written by a much older build, has no
// such table; that is version 0 and not an error.
func appliedSchemaVersion(ctx context.Context, dbPath string) (int64, error) {
	conn, err := driver.Open(bareDSN(dbPath))
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer conn.Close()

	var version sql.NullInt64
	err = conn.QueryRowContext(ctx,
		`SELECT max(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, fmt.Errorf("read schema version of %s: %w", dbPath, err)
	}
	return version.Int64, nil
}

// bareDSN opens the database with nothing but a busy timeout.
//
// It deliberately does NOT repeat internal/db's pragma set: this connection
// exists for one read and one VACUUM INTO and is closed immediately, and a
// second copy of the tuned DSN would be a second place for it to drift.
func bareDSN(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)"
}

var migrationVersionRE = regexp.MustCompile(`^(\d+)_`)

// newestEmbeddedMigration is the highest version this binary carries.
func newestEmbeddedMigration() (int64, error) {
	entries, err := fs.Glob(db.MigrationsFS(), "migrations/*.sql")
	if err != nil {
		return 0, fmt.Errorf("list embedded migrations: %w", err)
	}
	var newest int64
	for _, e := range entries {
		m := migrationVersionRE.FindStringSubmatch(filepath.Base(e))
		if m == nil {
			continue
		}
		v, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		if v > newest {
			newest = v
		}
	}
	return newest, nil
}
