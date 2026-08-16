package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/db"
	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/store"
	"github.com/jdb3750/UsArr/internal/web"
)

// app is everything the process owns between startup and shutdown.
type app struct {
	cfg      *config.Config
	log      *slog.Logger
	db       *db.DB
	store    *store.Store
	registry *registry
	server   *httpapi.Server
}

// buildApp runs the startup sequence, in the order CONFIGURATION.md §3 and
// ARCHITECTURE.md §15 require:
//
//	create the directories
//	→ the master-key ladder
//	→ pre-migration backup
//	→ open SQLite with the pragmas and apply migrations
//	→ the "encrypted rows exist but no key" check
//	→ build the router
//
// Every fatal error names the variable or file the bad value came from. There is
// no lenient branch anywhere in here: half-configured is not a state UsArr runs
// in.
func buildApp(ctx context.Context, cfg *config.Config, log *slog.Logger, build httpapi.BuildInfo) (*app, error) {
	if err := ensureDirs(cfg); err != nil {
		return nil, err
	}

	// The ladder. ErrKeyAbsent is NOT yet an error: it is the genuine-first-run
	// candidate, and it cannot be resolved until the database says whether it
	// already holds encrypted rows. Generating a key over existing ciphertext
	// would leave every stored credential permanently unreadable while the
	// process happily continued.
	masterKey, keyErr := cfg.ResolveMasterKey()
	if keyErr != nil && !errors.Is(keyErr, config.ErrKeyAbsent) {
		return nil, keyErr
	}

	backup, err := backupBeforeMigrate(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pre-migration backup: %w", err)
	}
	if backup != "" {
		log.Info("pre-migration backup written", "path", backup)
	}

	// db.Open sets the pragmas on EVERY connection in both pools and then
	// applies pending migrations on the single writer, each in its own
	// transaction.
	database, err := db.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite creates the database and its sidecars with the process umask, and
	// CONFIGURATION.md §5 pins them at 0600 explicitly and independent of it.
	// The file holds every wrapped DEK, every password hash and the whole audit
	// log, so "whatever umask happened to be" is not an acceptable answer.
	if err := restrictDatabaseModes(cfg.DatabasePath()); err != nil {
		_ = database.Close()
		return nil, err
	}
	schemaVersion, err := database.Version(ctx)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	log.Info("database ready", "path", cfg.DatabasePath(), "schema_version", schemaVersion)

	st := store.New(database)

	// The branch the ladder could not decide before the database was open.
	if errors.Is(keyErr, config.ErrKeyAbsent) {
		encrypted, err := st.CountEncryptedCredentials(ctx)
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		if encrypted > 0 {
			_ = database.Close()
			return nil, cfg.ErrMissingKeyForExistingData(encrypted)
		}
		if masterKey, err = cfg.GenerateMasterKey(); err != nil {
			_ = database.Close()
			return nil, err
		}
		// One line, loud, naming the real path. Losing this key means every
		// stored service credential has to be re-entered by hand.
		log.Warn(masterKey.BackupNotice())
	}
	log.Info("master key loaded", "source", string(masterKey.Source), "path", masterKey.Path)
	if masterKey.IgnoredKeyFile != "" {
		log.Warn("a key file exists but was ignored because the environment supplied a key",
			"ignored", masterKey.IgnoredKeyFile, "source", string(masterKey.Source))
	}

	// The KEK is derived, never used raw: distinct HKDF info labels keep the
	// credential KEK and the URL-signing key independently rotatable.
	salt, err := cfg.ResolveKEKSalt()
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	kek, err := crypto.DeriveKEK(masterKey.Key, salt)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	keyring, err := crypto.NewKeyring(1, kek)
	if err != nil {
		_ = database.Close()
		return nil, err
	}

	for _, warning := range cfg.TrustedProxyWarnings() {
		log.Warn(warning)
	}
	if cfg.TrustsNothing() {
		log.Info("USARR_TRUSTED_PROXIES is empty: no forwarded header is believed and the peer IP is used as-is")
	}

	reg := newRegistry(st, keyring, log, build.Version)

	var spa http.Handler
	if web.Built() {
		spa = web.Handler()
	} else {
		// Honest, and actionable: a bare `go build ./cmd/usarr` produces exactly
		// this binary. Serving a blank page instead would be the worst outcome.
		log.Warn("no frontend build is embedded in this binary; " +
			"the API works and / returns a 404 explaining how to build it")
		spa = web.Handler()
	}

	server, err := httpapi.New(httpapi.Config{
		Store:          st,
		Keyring:        keyring,
		SchemaVersion:  schemaVersion,
		URLBase:        cfg.URLBase,
		TrustedProxies: cfg.TrustedProxies,
		Build:          build,
		SPA:            spa,
		Releases:       reg,
		Tester:         reg,
		Probes:         reg,
		Logger:         log,
	})
	if err != nil {
		_ = database.Close()
		return nil, err
	}

	return &app{cfg: cfg, log: log, db: database, store: st, registry: reg, server: server}, nil
}

// Close releases the database. The HTTP server is drained by the caller first.
func (a *app) Close() error {
	a.server.Close()
	return a.db.Close()
}

// ensureDirs creates the on-disk layout with explicit modes.
//
// UsArr sets these itself rather than inheriting a process umask, so the tree in
// CONFIGURATION.md §5 holds regardless of how the process was started — which
// matters because the container has no shell and cannot chown anything.
func ensureDirs(cfg *config.Config) error {
	dirs := []string{
		cfg.ConfigDir,
		cfg.DataDir,
		cfg.KeysDir(),
		cfg.BackupsDir(),
		cfg.ProvidersDir(),
		cfg.LogsDir(),
	}
	for _, dir := range dirs {
		mode := config.DirMode
		if dir == cfg.KeysDir() {
			mode = config.SecretDirMode
		}
		if err := os.MkdirAll(dir, mode); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		// MkdirAll leaves an existing directory's mode alone and the volume may
		// have been created by hand.
		if err := os.Chmod(dir, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", dir, err)
		}
	}
	return nil
}

// restrictDatabaseModes forces 0600 on the database and its WAL sidecars.
//
// A sidecar that does not exist yet is not an error: -wal and -shm appear on
// the first write, and they are re-chmodded on the next start. They carry
// committed transactions, so they are exactly as sensitive as the main file.
func restrictDatabaseModes(dbPath string) error {
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Chmod(p, config.FileMode); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("chmod %s: %w", p, err)
		}
	}
	return nil
}

func goVersion() string { return runtime.Version() }
