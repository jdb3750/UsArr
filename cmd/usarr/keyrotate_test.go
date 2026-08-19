package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/store"
)

// rotateOn runs the real `usarr key rotate` against an existing config
// directory, through the same parser the binary uses, and returns what the
// command printed.
func rotateOn(t *testing.T, dir string, env map[string]string) (string, error) {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	cfg, err := config.Load(config.Options{
		Args: []string{"key", "rotate", "--config-dir", dir}, Env: env, Version: "0.0.0-test",
	})
	if !errors.Is(err, config.ErrKeyRotateRequested) {
		t.Fatalf("`key rotate` did not resolve to a rotation: %v", err)
	}
	log := slog.New(slog.NewTextHandler(&syncBuffer{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var out bytes.Buffer
	rerr := runKeyRotate(context.Background(), cfg, log, httpapi.BuildInfo{
		Version: "0.0.0-test", Commit: "test", GoVersion: goVersion(),
	}, &out)
	return out.String(), rerr
}

// sealExtraInstance adds another sealed credential straight through the store,
// under whatever key the running app holds. It exists so a test can have more
// than one row — including a tombstoned one — without repeating the one-time
// first-run setup flow that sealOneCredential performs.
func sealExtraInstance(t *testing.T, a *app, name, baseURL, apiKey string) int64 {
	t.Helper()
	id, err := a.store.CreateServiceInstanceSealed(context.Background(), store.ServiceInstance{
		Kind: "radarr", Name: name, BaseURL: baseURL, Enabled: true, VerifyTLS: true,
	}, func(id int64) ([]byte, uint32, error) {
		aad, err := crypto.ServiceInstanceAAD(id, baseURL)
		if err != nil {
			return nil, 0, err
		}
		env, err := a.keyring.Seal([]byte(apiKey), aad)
		return env, a.keyring.PrimaryID(), err
	})
	if err != nil {
		t.Fatalf("seal %s: %v", name, err)
	}
	return id
}

// openStored opens one row's credential through the running app's keyring,
// tombstoned rows included. registry.openCredential cannot be used for those:
// the scoped read never returns them.
func openStored(t *testing.T, a *app, id int64) string {
	t.Helper()
	rows, err := a.store.ListCredentialEnvelopesIncludingDeleted(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	for _, c := range rows {
		if c.ID != id {
			continue
		}
		aad, err := crypto.ServiceInstanceAAD(c.ID, c.BaseURL)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := a.keyring.Open(c.Envelope, aad)
		if err != nil {
			t.Fatalf("service_instance %d did not open: %v", id, err)
		}
		return string(plain)
	}
	t.Fatalf("service_instance %d is not in the table", id)
	return ""
}

func kekIDOf(t *testing.T, a *app, id int64) uint32 {
	t.Helper()
	rows, err := a.store.ListCredentialEnvelopesIncludingDeleted(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	for _, c := range rows {
		if c.ID == id {
			return c.KEKID
		}
	}
	t.Fatalf("service_instance %d is not in the table", id)
	return 0
}

// TestKeyRotateKeepsEveryCredentialOpenable is the whole point of the command:
// after a completed rotation and a restart, the exact credential that was
// sealed under the old key still opens under the new one.
//
// It rotates a live row and a TOMBSTONED one together. A tombstone still holds
// its ciphertext, and a rotation that skipped it would replace the key file
// while that row stayed wrapped under the old key — permanently unopenable
// ciphertext produced by the procedure whose purpose is not to produce any.
// See REVIEW-LOG.md RK-01.
func TestKeyRotateKeepsEveryCredentialOpenable(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	const deletedKey = "prowlarrKEY0011223344556677889900"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	instanceID := sealOneCredential(t, dir, prowlarr.URL(), apiKey)

	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("restart before rotating: %v", err)
	}
	deletedID := sealExtraInstance(t, a, "Retired Radarr", "http://radarr.lan:7878", deletedKey)
	if err := a.store.SoftDeleteServiceInstance(context.Background(), deletedID, time.Now()); err != nil {
		t.Fatalf("SoftDeleteServiceInstance: %v", err)
	}
	beforeID := kekIDOf(t, a, instanceID)
	keyBefore, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("close before rotating: %v", err)
	}

	out, err := rotateOn(t, dir, nil)
	if err != nil {
		t.Fatalf("key rotate: %v\n%s", err, out)
	}
	t.Logf("key rotate output:\n%s", out)
	for _, want := range []string{"prepare:", "rewrap:", "verify:", "promote:", "rotation complete"} {
		if !strings.Contains(out, want) {
			t.Errorf("the command output does not report the %q phase:\n%s", want, out)
		}
	}
	// No secret value may ever appear in what the command prints.
	for _, secret := range []string{apiKey, deletedKey, string(keyBefore)} {
		if strings.Contains(out, strings.TrimSpace(secret)) {
			t.Fatal("the command printed a secret value")
		}
	}

	// The key file really changed, and the mid-rotation file is gone.
	keyAfter, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("secret.key is unchanged after a rotation")
	}
	if _, err := os.Stat(filepath.Join(dir, "keys", "secret.key.new")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("secret.key.new survived a completed rotation: %v", err)
	}

	// Restart on the new key alone. This is the assertion that matters.
	after, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("restart after rotating: %v", err)
	}
	si, err := after.store.GetServiceInstance(context.Background(), store.Scope{AllInstances: true}, instanceID)
	if err != nil {
		t.Fatalf("read service instance: %v", err)
	}
	opened, err := after.registry.openCredential(si)
	if err != nil {
		t.Fatalf("the credential did not survive the rotation: %v", err)
	}
	if opened != apiKey {
		t.Fatal("the credential opened to the wrong value after rotation")
	}
	if got := openStored(t, after, deletedID); got != deletedKey {
		t.Fatal("the tombstoned row's credential did not survive the rotation")
	}

	// Every row moved to the new id, and the new id is the content-derived one.
	afterKEKID := kekIDOf(t, after, instanceID)
	if afterKEKID == beforeID {
		t.Fatal("kek_id is unchanged after a rotation")
	}
	if afterKEKID != after.keyring.PrimaryID() {
		t.Errorf("kek_id = %d but the primary key is %d", afterKEKID, after.keyring.PrimaryID())
	}
	if n, err := after.store.CountCredentialsOutsideKEKIDIncludingDeleted(
		context.Background(), after.keyring.PrimaryID()); err != nil || n != 0 {
		t.Errorf("%d row(s) still at another kek id (err %v)", n, err)
	}

	// One audit row per phase, and none of them carries anything but counts
	// and ids.
	assertRotationAudit(t, after, 1)
}

// TestInterruptedKeyRotationStartsAndResumes is the crash window the two-phase
// design exists to close: secret.key.new on disk, some rows re-wrapped and some
// not. Every credential must still open, the server must still start, and the
// next `key rotate` must resume rather than start over.
func TestInterruptedKeyRotationStartsAndResumes(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	const secondKey = "prowlarrKEY0011223344556677889900"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	instanceID := sealOneCredential(t, dir, prowlarr.URL(), apiKey)

	// Interrupt a rotation exactly where a crash would: the new key file is
	// written and registered, ONE row has moved to it, the other has not.
	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	secondID := sealExtraInstance(t, a, "Radarr", "http://radarr.lan:7878", secondKey)
	newMaster, err := a.cfg.GenerateNewSecretKey()
	if err != nil {
		t.Fatalf("GenerateNewSecretKey: %v", err)
	}
	newKEK, err := crypto.DeriveKEK(newMaster, a.kekSalt)
	if err != nil {
		t.Fatal(err)
	}
	newID := crypto.KeyID(newKEK)
	if err := a.keyring.Add(newID, newKEK); err != nil {
		t.Fatal(err)
	}
	rows, err := a.store.ListCredentialEnvelopesIncludingDeleted(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two sealed rows, got %d", len(rows))
	}
	moved, err := a.keyring.Rewrap(rows[0].Envelope, newID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.RewrapCredentialsIncludingDeleted(context.Background(),
		[]store.CredentialRewrite{{ID: rows[0].ID, Envelope: moved, KEKID: newID}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Restart. This must be an ordinary start, not a refusal, and BOTH rows
	// must open — the one that moved and the one that did not.
	half, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("an install with an interrupted rotation must start, not refuse: %v", err)
	}
	if got := openStored(t, half, instanceID); got != apiKey {
		t.Error("the first credential did not open mid-rotation")
	}
	if got := openStored(t, half, secondID); got != secondKey {
		t.Error("the second credential did not open mid-rotation")
	}
	// The server must NOT have rotated on its own: primary stays on secret.key.
	if half.keyring.PrimaryID() == newID {
		t.Error("startup promoted the pending key; rotation is an operator action, never a startup one")
	}
	if len(half.keyring.IDs()) < 3 {
		t.Errorf("mid-rotation keyring holds %v; it must hold the old key, its legacy id and the pending key",
			half.keyring.IDs())
	}
	if err := half.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Resume. It must adopt the EXISTING secret.key.new, not generate a third
	// key — generating one would strand the row already wrapped under it.
	pendingBefore, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key.new"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := rotateOn(t, dir, nil)
	if err != nil {
		t.Fatalf("resume: %v\n%s", err, out)
	}
	t.Logf("resumed rotation output:\n%s", out)
	if !strings.Contains(out, "resuming") {
		t.Errorf("the command did not report that it was resuming:\n%s", out)
	}

	promoted, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(promoted, pendingBefore) {
		t.Fatal("the promoted key is not the pending one; the resume generated fresh material " +
			"and stranded the rows already re-wrapped")
	}

	done, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("restart after the resumed rotation: %v", err)
	}
	if got := openStored(t, done, instanceID); got != apiKey {
		t.Error("the first credential did not survive the resumed rotation")
	}
	if got := openStored(t, done, secondID); got != secondKey {
		t.Error("the second credential did not survive the resumed rotation")
	}
	if done.keyring.PrimaryID() != newID {
		t.Errorf("primary kek id = %d, want the resumed key's %d", done.keyring.PrimaryID(), newID)
	}
	assertRotationAudit(t, done, 1)
}

// TestKeyRotateRefusesAnEnvironmentSuppliedKey: the command can only manage
// keys/secret.key. A key that arrives through the environment lives somewhere
// UsArr does not own, and "rename a replacement into place" has no meaning
// there — rotating anyway would re-wrap every row under material the next start
// cannot find.
func TestKeyRotateRefusesAnEnvironmentSuppliedKey(t *testing.T) {
	dir := t.TempDir()
	// A key that is obviously a fixture: 32 bytes of 0x00 would be rejected as
	// all-zero, so this is 31 zero bytes and a trailing 1.
	const suppliedKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "USARR_SECRET_KEY",
			env:  map[string]string{"USARR_SECRET_KEY": suppliedKey},
			want: "USARR_SECRET_KEY is set",
		},
		{
			name: "USARR_SECRET_KEY_FILE",
			env:  map[string]string{"USARR_SECRET_KEY_FILE": filepath.Join(dir, "elsewhere.key")},
			want: "USARR_SECRET_KEY_FILE is set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := rotateOn(t, t.TempDir(), tc.env)
			if err == nil {
				t.Fatalf("key rotate SUCCEEDED under %s; it can only manage keys/secret.key\n%s", tc.name, out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not name %s: %v", tc.name, err)
			}
			if !strings.Contains(err.Error(), "secret.key") {
				t.Errorf("the refusal does not say what it CAN rotate: %v", err)
			}
			t.Logf("refused with: %v", err)
		})
	}
}

// assertRotationAudit checks that one row per phase was appended and that no
// row carries anything but counts and ids.
func assertRotationAudit(t *testing.T, a *app, rotations int) {
	t.Helper()
	entries, err := a.store.ListAuditLog(context.Background(), store.OwnerScope(1), store.AuditQuery{
		Actions: []string{
			store.AuditActionKeyRotatePrepare,
			store.AuditActionKeyRotateRewrap,
			store.AuditActionKeyRotatePromote,
		},
	})
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Action]++
		if e.TargetType != "master_key" {
			t.Errorf("audit %s target_type = %q, want master_key", e.Action, e.TargetType)
		}
		// Metadata is counts and ids only. A path or a key would be a secret
		// value in an append-only table nothing can correct.
		if strings.Contains(e.MetadataJSON, "/") || strings.Contains(e.MetadataJSON, "secret") {
			t.Errorf("audit %s metadata looks like more than counts and ids: %s", e.Action, e.MetadataJSON)
		}
	}
	for _, action := range []string{
		store.AuditActionKeyRotatePrepare,
		store.AuditActionKeyRotateRewrap,
		store.AuditActionKeyRotatePromote,
	} {
		if counts[action] != rotations {
			t.Errorf("audit rows for %s = %d, want %d", action, counts[action], rotations)
		}
	}
}
