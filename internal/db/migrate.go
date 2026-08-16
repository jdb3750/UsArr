package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// migrations are embedded so a binary can always bring its own database
// forward without an external tool. `make migrate` runs the same files through
// the goose CLI for authoring; the binary does not depend on it existing.
//
//go:embed migrations/*.sql
var migrations embed.FS

// MigrationsFS exposes the embedded migrations for tests and tooling.
func MigrationsFS() embed.FS { return migrations }

// Migrate applies every pending migration on the single writer connection.
//
// goose wraps each migration in its own transaction, so a migration that fails
// halfway leaves the database at the previous version rather than partly
// upgraded. Running on the writer pool keeps DDL inside the single-writer
// discipline: a migration and an interactive write must never contend.
func Migrate(ctx context.Context, d *DB) error {
	provider, err := newProvider(d)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations to %s: %w", d.path, err)
	}
	return nil
}

// Migrate applies every pending migration.
func (d *DB) Migrate(ctx context.Context) error { return Migrate(ctx, d) }

// MigrateDown rolls back exactly one migration. Downgrades are not a supported
// user path; this exists for the migration round-trip test.
func (d *DB) MigrateDown(ctx context.Context) error {
	provider, err := newProvider(d)
	if err != nil {
		return err
	}
	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("roll back migration on %s: %w", d.path, err)
	}
	return nil
}

// Version reports the current schema version.
func (d *DB) Version(ctx context.Context) (int64, error) {
	provider, err := newProvider(d)
	if err != nil {
		return 0, err
	}
	v, err := provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read schema version of %s: %w", d.path, err)
	}
	return v, nil
}

func newProvider(d *DB) (*goose.Provider, error) {
	// goose looks for *.sql at the root of the filesystem it is given, so the
	// embedded tree has to be re-rooted at migrations/.
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("prepare migrations: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, d.write, sub)
	if err != nil {
		return nil, fmt.Errorf("prepare migrations: %w", err)
	}
	return p, nil
}
