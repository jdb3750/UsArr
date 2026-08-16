package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/store"
)

// startOn runs the real startup sequence against an EXISTING config directory,
// which is what makes these tests restarts rather than first runs.
func startOn(t *testing.T, dir string) (*app, error) {
	t.Helper()
	cfg, err := config.Load(config.Options{
		Args: []string{"--config-dir", dir}, Env: map[string]string{}, Version: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	log := slog.New(slog.NewTextHandler(&syncBuffer{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	a, err := buildApp(context.Background(), cfg, log,
		httpapi.BuildInfo{Version: "0.0.0-test", Commit: "test", GoVersion: goVersion()})
	if a != nil {
		t.Cleanup(func() { _ = a.Close() })
	}
	return a, err
}

// sealOneCredential brings up a fresh install in dir and stores exactly one
// sealed *Arr credential through the real API, connection test and all.
func sealOneCredential(t *testing.T, dir, prowlarrURL, apiKey string) int64 {
	t.Helper()
	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	srv := httptest.NewServer(a.server.Handler())
	t.Cleanup(srv.Close)
	a.server.MarkListening()

	env := &testEnv{app: a, srv: srv, cookies: map[string]string{}, logBuf: &syncBuffer{}}
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	var svc serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prowlarrURL, "api_key": apiKey,
	}, &svc)
	if !svc.HasCredential {
		t.Fatal("the fixture did not actually seal a credential")
	}
	return svc.ID
}

// TestRestoreWithoutKEKSaltRefusesToStart is the regression for a data-loss bug
// that permanently destroyed every stored credential.
//
// CONFIGURATION.md §6.1 used to archive /config with --exclude='keys' and keep
// only secret.key separately. Restoring that backup put secret.key back and left
// keys/kek.salt missing — and startup SILENTLY GENERATED A FRESH SALT. Since the
// KEK is HKDF-SHA256(secret.key, salt=kek.salt, info="usarr/kek/v1"), a new salt
// is a new KEK, so every sealed *Arr API key became permanently unopenable while
// the process reported nothing worse than a red connection test.
//
// The salt is the same class of secret as the master key, with the same
// consequence, so it follows the same rule the master-key ladder already
// follows: refuse, name the path, and never invent one over existing ciphertext.
func TestRestoreWithoutKEKSaltRefusesToStart(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	instanceID := sealOneCredential(t, dir, prowlarr.URL(), apiKey)

	saltPath := filepath.Join(dir, "kek.salt")
	saved, err := os.ReadFile(saltPath)
	if err != nil {
		t.Fatalf("read salt: %v", err)
	}

	// The restore: secret.key came back from the password manager, kek.salt was
	// never in the archive.
	if err := os.Remove(saltPath); err != nil {
		t.Fatalf("remove salt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keys", "secret.key")); err != nil {
		t.Fatalf("the master key must still be present for this to be the reported bug: %v", err)
	}

	a, err := startOn(t, dir)
	if err == nil {
		_ = a.Close()
		t.Fatal("startup SUCCEEDED with sealed credentials present and keys/kek.salt missing. " +
			"It must refuse: continuing here derives a different KEK and permanently " +
			"destroys every stored credential")
	}

	// The refusal has to be actionable. Naming secret.key here would be the
	// original bug's second half — it sends an operator who is already holding
	// the master key off to restore the master key.
	msg := err.Error()
	for _, want := range []string{saltPath, "kek.salt", "encrypted credential"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the startup refusal does not mention %q: %s", want, msg)
		}
	}
	t.Logf("startup refused with: %s", msg)

	// Nothing may have been written. This is the property that keeps the
	// mistake recoverable: the moment a fresh salt lands on disk, the operator's
	// good backup of the old one is the only thing standing between them and
	// permanent loss, and they have not been told to go and get it.
	if _, statErr := os.Stat(saltPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a fresh salt was written to %s despite the refusal", saltPath)
	}

	// And the refusal really was recoverable: put the salt back and the exact
	// credential opens again.
	if err := os.WriteFile(saltPath, saved, config.SecretMode); err != nil {
		t.Fatalf("restore salt: %v", err)
	}
	recovered, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("startup after restoring keys/kek.salt: %v", err)
	}
	si, err := recovered.store.GetServiceInstance(context.Background(),
		store.Scope{AllInstances: true}, instanceID)
	if err != nil {
		t.Fatalf("read service instance: %v", err)
	}
	opened, err := recovered.registry.openCredential(si)
	if err != nil {
		t.Fatalf("the credential did not survive the refuse-and-restore cycle: %v", err)
	}
	if opened != apiKey {
		t.Fatal("the recovered credential is not the one that was sealed")
	}
}

// TestLegacyKEKSaltIsRelocatedNotLost is the upgrade path for every install
// created before the salt moved out of keys/ — Joe's among them.
//
// The salt is not secret, so it does not belong in keys/, the one directory the
// documented backup deliberately excludes. Moving it beside usarr.db makes an
// ordinary config-volume backup capture it and `--exclude='keys'` correct as
// written. But the move is only safe if startup RELOCATES an existing
// keys/kek.salt rather than reading the new path and concluding the salt is
// missing — that conclusion, on an install with sealed credentials, is the exact
// data loss this change exists to prevent, caused by the fix for it.
func TestLegacyKEKSaltIsRelocatedNotLost(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	instanceID := sealOneCredential(t, dir, prowlarr.URL(), apiKey)

	current := filepath.Join(dir, "kek.salt")
	legacy := filepath.Join(dir, "keys", "kek.salt")

	// Rewind this install to the pre-move layout, exactly as it exists on disk
	// on an install created before the change.
	salt, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("read salt: %v", err)
	}
	if err := os.WriteFile(legacy, salt, config.SecretMode); err != nil {
		t.Fatalf("write legacy salt: %v", err)
	}
	if err := os.Remove(current); err != nil {
		t.Fatalf("remove current salt: %v", err)
	}

	// Restart. This must be an ordinary, quiet upgrade — not a refusal.
	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("an install with a legacy keys/kek.salt must start, not refuse: %v", err)
	}

	// The credential still opens. This is the assertion that matters: the KEK
	// derived after relocation is the same KEK.
	si, err := a.store.GetServiceInstance(context.Background(),
		store.Scope{AllInstances: true}, instanceID)
	if err != nil {
		t.Fatalf("read service instance: %v", err)
	}
	opened, err := a.registry.openCredential(si)
	if err != nil {
		t.Fatalf("the credential did not survive the relocation: %v", err)
	}
	if opened != apiKey {
		t.Fatal("the credential opened to the wrong value after relocation")
	}

	// The file was COPIED forward byte for byte — and the legacy copy is still
	// there. A move would have an instant in which neither path holds the salt,
	// and two processes can start against one config directory with no lock
	// between them; a kill in that instant loses key-derivation material for
	// good. A duplicate non-secret costs nothing and closes that window.
	copied, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("the salt was not copied to %s: %v", current, err)
	}
	if string(copied) != string(salt) {
		t.Fatal("the copied salt is not byte-identical to the legacy one")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("the legacy salt at %s was deleted; this must be a copy, not a move: %v",
			legacy, err)
	}
}

// TestBothSaltCopiesPresentPrefersTheNewPath: the steady state after an upgrade
// is two identical files, because the copy never deletes the legacy one. Every
// subsequent start must resolve that quietly and identically.
func TestBothSaltCopiesPresentPrefersTheNewPath(t *testing.T) {
	dir := t.TempDir()
	if _, err := startOn(t, dir); err != nil {
		t.Fatalf("first run: %v", err)
	}

	current := filepath.Join(dir, "kek.salt")
	legacy := filepath.Join(dir, "keys", "kek.salt")
	salt, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	// The post-upgrade steady state: both files present and identical.
	if err := os.WriteFile(legacy, salt, config.SecretMode); err != nil {
		t.Fatal(err)
	}

	if _, err := startOn(t, dir); err != nil {
		t.Fatalf("two salt copies present must resolve on the next start: %v", err)
	}
	after, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(salt) {
		t.Fatal("the salt changed while both copies were present")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("the legacy copy was deleted: %v", err)
	}
}

// TestFirstRunGeneratesTheKEKSalt is the other half of the rule: refusing is
// only correct when something is actually sealed. A genuine first run must still
// create both artifacts without complaint, or the fix has broken every new
// install.
func TestFirstRunGeneratesTheKEKSalt(t *testing.T) {
	dir := t.TempDir()

	if _, err := startOn(t, dir); err != nil {
		t.Fatalf("first run must succeed on an empty config dir: %v", err)
	}
	// secret.key is a secret and lives in keys/. kek.salt is NOT a secret and
	// lives beside usarr.db, so that the documented backup — which excludes
	// keys/ — captures it.
	for _, rel := range []string{filepath.Join("keys", "secret.key"), "kek.salt"} {
		fi, err := os.Stat(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("first run did not create %s: %v", rel, err)
		}
		if got := fi.Mode().Perm(); got != config.SecretMode.Perm() {
			t.Errorf("%s mode = %#o, want %#o", rel, got, config.SecretMode.Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "keys", "kek.salt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the KEK salt must not be created inside keys/: that directory is " +
			"excluded from the documented backup, which is what lost it")
	}
}

// TestRestartKeepsTheSameKEKSalt pins the other way this can go wrong: a salt
// that is rewritten on every restart destroys credentials just as effectively
// as one regenerated after a restore, and does it to installs that never took a
// backup at all.
func TestRestartKeepsTheSameKEKSalt(t *testing.T) {
	dir := t.TempDir()
	saltPath := filepath.Join(dir, "kek.salt")

	if _, err := startOn(t, dir); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first, err := os.ReadFile(saltPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startOn(t, dir); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, err := os.ReadFile(saltPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("the KEK salt changed across a restart")
	}
}
