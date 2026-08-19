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

	// The key material the run resolved. `usarr key rotate` needs all three —
	// the ring to add a key to, and the master key plus salt to derive the new
	// KEK from the same salt the old one used. They are held here rather than
	// re-resolved by the rotation, because re-resolving would be a second copy
	// of the startup ladder and the two would drift.
	keyring   *crypto.Keyring
	masterKey *config.MasterKey
	kekSalt   []byte
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

	keyring, err := buildKeyring(ctx, cfg, st, masterKey, salt, log)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	log.Info("keyring ready", "primary_kek_id", keyring.PrimaryID(), "active_kek_ids", keyring.IDs())
	if err := warnStrandedCredentials(ctx, st, keyring, log); err != nil {
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
		ImageCacheDir:  cfg.ImageCacheDir(),
		TrustedProxies: cfg.TrustedProxies,
		Build:          build,
		SPA:            spa,
		Releases:       reg,
		Tester:         reg,
		Probes:         reg,
		Imports:        reg,
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

	return &app{
		cfg: cfg, log: log, db: database, store: st, registry: reg, server: server,
		keyring: keyring, masterKey: masterKey, kekSalt: salt,
	}, nil
}

// buildKeyring registers every KEK this process must be able to open a row
// under, and decides which one new rows are sealed with.
//
// Three registrations, and each one closes a specific failure:
//
//   - KeyID(kek), the PRIMARY. Key ids are content-derived (ADR-0049), so the
//     key file names its own id and no second piece of state has to stay
//     consistent with it across a crash.
//
//   - crypto.LegacyKEKID, the SAME key under the fixed id 1. Every envelope
//     sealed before ids were content-derived carries 1, and this is what keeps
//     those rows opening with no migration and no rewrite. It is decrypt-only:
//     the primary is the derived id, so nothing new is ever sealed at 1.
//
//   - keys/secret.key.new, when it exists, as an additional decrypt-only key.
//     Its presence means a rotation was interrupted, so rows are split across
//     two ids; registering both is what makes EVERY row openable in that state
//     rather than only the ones the rotation had not reached. Primary stays on
//     secret.key, because secret.key is still the file the operator's backup
//     matches and the process must not start sealing under material that a
//     `usarr key rotate` has not yet promoted.
//
// The server NEVER rotates on its own. It logs a warning and continues, because
// a rotation is an operator action with an audit trail, and a startup path that
// silently re-wrapped every row would do it unattended, unlogged by the
// operator, and on a restart loop.
//
// It takes the store only for the fatal branch below: an unreadable
// secret.key.new has two remedies with opposite consequences, and which one is
// safe depends on how many rows are wrapped under the key in that file.
// warnStrandedCredentials, not this function, reports rows no key opens — that
// is a diagnostic about the ring this returns, so it runs after it.
func buildKeyring(
	ctx context.Context, cfg *config.Config, st *store.Store,
	masterKey *config.MasterKey, salt []byte, log *slog.Logger,
) (*crypto.Keyring, error) {
	kek, err := crypto.DeriveKEK(masterKey.Key, salt)
	if err != nil {
		return nil, err
	}
	primaryID := crypto.KeyID(kek)
	keyring, err := crypto.NewKeyring(primaryID, kek)
	if err != nil {
		return nil, err
	}
	// Add is idempotent for identical bytes, so the 2^-32 case where KeyID
	// lands on LegacyKEKID needs no special handling here.
	if err := keyring.Add(crypto.LegacyKEKID, kek); err != nil {
		return nil, err
	}

	pending, err := cfg.ReadNewSecretKey()
	switch {
	case errors.Is(err, os.ErrNotExist):
		return keyring, nil
	case err != nil:
		// A secret.key.new that exists but cannot be read is fatal. Continuing
		// would leave whichever rows the interrupted rotation already re-wrapped
		// unopenable, and report nothing worse than a red connection test.
		//
		// The REMEDY is not one sentence, and the shorter version of it was a
		// data-loss instruction. This used to say "restore or remove it"
		// unconditionally, and on a rotation interrupted after its re-wrap pass
		// had moved rows, removing the file destroys every one of them
		// permanently — the file is the only remaining source of the key those
		// rows name. Which of the two states this is cannot be guessed, so it is
		// counted: rows outside the live key's ids are rows that need the
		// unreadable file.
		//
		// A failure to count is not allowed to soften the message. The generic
		// advice is what gets printed only when the count says zero rows are at
		// stake; when the count itself is unavailable the operator gets the
		// cautious form, because "remove it" is unrecoverable and "restore it"
		// is not.
		stranded, countErr := st.ListCredentialsOutsideKEKIDsIncludingDeleted(ctx, keyring.IDs())
		if countErr == nil && len(stranded) == 0 {
			return nil, fmt.Errorf("an interrupted key rotation left %s in place and it could not be read: %w. "+
				"No stored credential is wrapped under it, so restoring or removing it are both safe; "+
				"see docs/CONFIGURATION.md §3.4", cfg.NewSecretKeyPath(), err)
		}
		count := "an unknown number of"
		if countErr == nil {
			count = fmt.Sprintf("%d", len(stranded))
		}
		return nil, fmt.Errorf("an interrupted key rotation left %s in place and it could not be read: %w. "+
			"%s stored credential(s) are wrapped under the key in that file and nothing else on disk can "+
			"open them, so RESTORING IT IS THE ONLY SAFE ROUTE — removing it destroys those credentials "+
			"permanently and they can only be re-entered by hand. "+
			"See docs/CONFIGURATION.md §3.4", cfg.NewSecretKeyPath(), err, count)
	}

	pendingKEK, err := crypto.DeriveKEK(pending, salt)
	if err != nil {
		return nil, err
	}
	pendingID := crypto.KeyID(pendingKEK)
	if err := keyring.Add(pendingID, pendingKEK); err != nil {
		return nil, fmt.Errorf("registering the pending key from %s: %w", cfg.NewSecretKeyPath(), err)
	}
	log.Warn("an interrupted key rotation was found and both keys are active for decryption; "+
		"no row is unopenable, but the rotation is not finished — re-run `usarr key rotate` to resume it",
		"pending_key_file", cfg.NewSecretKeyPath(),
		"active_kek_ids", keyring.IDs(), "primary_kek_id", keyring.PrimaryID())
	return keyring, nil
}

// warnStrandedCredentials reports, at startup, every stored credential whose
// kek_id no key in the ring holds.
//
// WHY IT EXISTS. Such a row is unopenable, and until this ran the only way to
// find that out was to use the credential: a connection test or a search went
// red with `crypto: envelope names a key id not in the keyring`, one instance at
// a time, with nothing saying how many others were in the same state. The two
// ways to get there are both real and both silent — a `usarr key rotate` that
// ran while a server was up (see strandedAfterPromote), and a secret.key.new
// deleted rather than restored after an interrupted rotation.
//
// WHY IT WARNS RATHER THAN REFUSING. A stranded row is a per-row condition, not
// a broken install: every other credential still works, and the fix is to
// re-enter the affected API keys through the UI — which requires the server to
// be running. Refusing would make the one action that repairs the condition
// impossible, and it would do so on a state that can be entirely stale, since
// service_instance never hard-deletes and a tombstoned row keeps its ciphertext
// forever. A row deleted two years ago would then hold the process down. The
// missing-key and missing-salt ladders in buildApp refuse instead, and the
// difference is deliberate: there the WHOLE keyring is absent and continuing
// would seal new rows under material that opens nothing, which is not
// recoverable through the UI or through anything else.
//
// A failure to run the query IS fatal, because it is a database error rather
// than a key one, and a startup that cannot read service_instance has a problem
// this diagnostic is not the right place to swallow.
func warnStrandedCredentials(ctx context.Context, st *store.Store, keyring *crypto.Keyring, log *slog.Logger) error {
	stranded, err := st.ListCredentialsOutsideKEKIDsIncludingDeleted(ctx, keyring.IDs())
	if err != nil {
		return err
	}
	if len(stranded) == 0 {
		return nil
	}
	for _, c := range stranded {
		log.Warn("this service's stored credential cannot be opened by any key this process holds; "+
			"re-enter its API key to repair it",
			"service_instance", c.ID, "name", c.Name, "kek_id", c.KEKID, "deleted", c.Deleted)
	}
	log.Warn("stored credentials name a key id no key on disk derives — they were sealed under a key "+
		"that has since been rotated away or deleted, and re-entering each API key is the only repair",
		"count", len(stranded), "active_kek_ids", keyring.IDs())
	return nil
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
