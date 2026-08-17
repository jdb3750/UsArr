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
//	→ the "encrypted rows exist but no KEK salt" check
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
	// Whether this run created the master key, so the back-it-up notice fires
	// once, after the salt exists too.
	generatedKey := false

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

	// Repair credentials already on disk, before anything can serve them.
	//
	// It runs HERE, in Go, and not inside a migration. The repair has to use the
	// exact deny-list and path heuristic the write path uses, and both live in
	// internal/ssrf; reimplementing either in SQL would be the second, drifting
	// copy that internal/ssrf's own comment forbids, and SQLite has no regular
	// expressions, so the path heuristic could not be expressed there at all.
	// Doing it after migrations rather than as one keeps internal/db free of a
	// dependency on URL policy, which is the wrong direction for that package.
	//
	// It runs on EVERY start, not once. It is idempotent by construction, the
	// scan touches only rows that hold a URL at all, and after the first pass it
	// changes nothing. A run-once gate would have the wrong failure mode for a
	// security repair: a process killed mid-pass would leave the remaining rows
	// unredacted with the gate already satisfied.
	if err := redactStoredCredentials(ctx, st, log); err != nil {
		_ = database.Close()
		return nil, err
	}

	// The two branches the ladder could not decide before the database was
	// open. Both ask the same question — does the database already hold
	// ciphertext? — so it is asked once, lazily, and shared.
	//
	// keys/secret.key and keys/kek.salt are the SAME class of secret with the
	// SAME consequence: the KEK is HKDF(master key, salt), so a missing salt
	// makes every stored envelope exactly as unopenable as a missing key. They
	// therefore follow the same rule. Creating either one over existing
	// ciphertext is permanent, silent data loss, so neither is ever created
	// except on a genuine first run with nothing sealed anywhere.
	countEncrypted := func() (int64, error) { return st.CountEncryptedCredentials(ctx) }

	if errors.Is(keyErr, config.ErrKeyAbsent) {
		encrypted, err := countEncrypted()
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
		generatedKey = true
	}
	log.Info("master key loaded", "source", string(masterKey.Source), "path", masterKey.Path)
	if masterKey.IgnoredKeyFile != "" {
		log.Warn("a key file exists but was ignored because the environment supplied a key",
			"ignored", masterKey.IgnoredKeyFile, "source", string(masterKey.Source))
	}

	// The KEK is derived, never used raw: distinct HKDF info labels keep the
	// credential KEK and the URL-signing key independently rotatable.
	salt, saltErr := cfg.ResolveKEKSalt()
	if saltErr != nil && !errors.Is(saltErr, config.ErrKEKSaltAbsent) {
		_ = database.Close()
		return nil, saltErr
	}
	if errors.Is(saltErr, config.ErrKEKSaltAbsent) {
		encrypted, err := countEncrypted()
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		if encrypted > 0 {
			// The restore-shaped failure: secret.key came back, kek.salt did
			// not. Regenerating here derives a different KEK and destroys every
			// credential permanently, while the process reports nothing worse
			// than a red connection test.
			_ = database.Close()
			return nil, cfg.ErrMissingSaltForExistingData(encrypted)
		}
		if salt, err = cfg.GenerateKEKSalt(); err != nil {
			_ = database.Close()
			return nil, err
		}
	}
	if generatedKey {
		// One line, loud, after BOTH artifacts exist — it tells the operator to
		// back up the whole directory, so it must not fire while half of it is
		// still missing.
		log.Warn(masterKey.BackupNotice())
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

	// Derived from the same master key and salt as the KEK, so the opaque grab
	// row ids the API publishes are the same after a restart. They key rows in
	// the client; a key that changed per process would rebuild every row on
	// every poll.
	grabRowIDKey, err := crypto.DeriveGrabRowIDKey(masterKey.Key, salt)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	// The definite-failure arm of the same block publishes audit_log rowids,
	// which are a second monotonic sequence and therefore the same oracle. Its
	// own HKDF label keeps the two id domains from colliding.
	auditRowIDKey, err := crypto.DeriveAuditRowIDKey(masterKey.Key, salt)
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
		GrabRowIDKey:   grabRowIDKey,
		AuditRowIDKey:  auditRowIDKey,
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

	// The registry is built before the server because httpapi.New takes it, so
	// the stream is attached afterwards rather than injected. One assignment,
	// during startup, before anything can publish.
	reg.attachEvents(server.Events())

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
