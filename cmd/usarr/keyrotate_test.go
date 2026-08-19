package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	return rotateOnWith(t, dir, env)
}

// rotateOnWith is rotateOn with extra command-line arguments appended, for the
// cases where the SETTING'S SOURCE is what is under test rather than its value.
func rotateOnWith(t *testing.T, dir string, env map[string]string, extra ...string) (string, error) {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	cfg, err := config.Load(config.Options{
		Args: append([]string{"key", "rotate", "--config-dir", dir}, extra...),
		Env:  env, Version: "0.0.0-test",
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
	opened, err := after.registry.openCredential(context.Background(), si)
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

// TestKeyRotateRefusesAKeyItDoesNotOwn: the command can only manage
// keys/secret.key. A key supplied from outside lives somewhere UsArr does not
// own, and "rename a replacement into place" has no meaning there — rotating
// anyway would re-wrap every row under material the next start cannot find.
//
// ⚠️ EACH CASE ALSO CHECKS THAT THE REFUSAL NAMES THE SETTING THE OPERATOR
// ACTUALLY USED, and the third case is why. The path has a flag twin and an
// environment twin that resolve into one config field, and this refusal used to
// hardcode the variable's name for both — so `usarr key rotate
// --secret-key-file …` sent an operator holding a broken install off to unset a
// variable they had never set. It is the only diagnostic in this command that a
// PERSON reads while something is wrong, so naming the wrong thing costs real
// time. `notWant` is the half that has teeth: a message that named both would
// satisfy `want` alone.
func TestKeyRotateRefusesAKeyItDoesNotOwn(t *testing.T) {
	dir := t.TempDir()
	// A key that is obviously a fixture: 32 bytes of 0x00 would be rejected as
	// all-zero, so this is 31 zero bytes and a trailing 1.
	const suppliedKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

	elsewhere := filepath.Join(dir, "elsewhere.key")

	for _, tc := range []struct {
		name string
		env  map[string]string
		args []string
		want string
		// notWant is the setting the operator did NOT use. A refusal that names
		// it is sending someone to change the wrong thing.
		notWant string
	}{
		{
			name: "USARR_SECRET_KEY",
			env:  map[string]string{"USARR_SECRET_KEY": suppliedKey},
			want: "USARR_SECRET_KEY is set",
		},
		{
			name:    "USARR_SECRET_KEY_FILE",
			env:     map[string]string{"USARR_SECRET_KEY_FILE": elsewhere},
			want:    "USARR_SECRET_KEY_FILE is set",
			notWant: "--secret-key-file",
		},
		{
			name:    "--secret-key-file",
			args:    []string{"--secret-key-file", elsewhere},
			want:    "--secret-key-file was passed",
			notWant: "USARR_SECRET_KEY_FILE",
		},
		{
			// Both set, flag wins — so the refusal has to name the flag. This
			// is the case where "just print both" would still be wrong: the
			// variable's value is not the one that broke the rotation.
			name:    "--secret-key-file over USARR_SECRET_KEY_FILE",
			env:     map[string]string{"USARR_SECRET_KEY_FILE": filepath.Join(dir, "ignored.key")},
			args:    []string{"--secret-key-file", elsewhere},
			want:    "--secret-key-file was passed",
			notWant: "ignored.key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := rotateOnWith(t, t.TempDir(), tc.env, tc.args...)
			if err == nil {
				t.Fatalf("key rotate SUCCEEDED under %s; it can only manage keys/secret.key\n%s", tc.name, out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not name %s: %v", tc.name, err)
			}
			if tc.notWant != "" && strings.Contains(err.Error(), tc.notWant) {
				t.Errorf("the refusal blames %s, which this operator did not use: %v", tc.notWant, err)
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

// registerSecondKey generates the rotation's new key material on a RUNNING app,
// registers it for decryption, and returns its id.
//
// It is the state prepareRotation leaves behind, reproduced without running the
// command, so a test can drive one later phase in isolation.
func registerSecondKey(t *testing.T, a *app) uint32 {
	t.Helper()
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
	return newID
}

func envelopeOf(t *testing.T, a *app, id int64) []byte {
	t.Helper()
	rows, err := a.store.ListCredentialEnvelopesIncludingDeleted(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	for _, c := range rows {
		if c.ID == id {
			return append([]byte(nil), c.Envelope...)
		}
	}
	t.Fatalf("service_instance %d is not in the table", id)
	return nil
}

func writeEnvelope(t *testing.T, a *app, id int64, envelope []byte, kekID uint32) {
	t.Helper()
	if err := a.store.RewrapCredentialsIncludingDeleted(context.Background(),
		[]store.CredentialRewrite{{ID: id, Envelope: envelope, KEKID: kekID}}); err != nil {
		t.Fatalf("write envelope for service_instance %d: %v", id, err)
	}
}

// TestVerifyRefusesToPromoteOnEachWayARowCanLie drills verifyEverything's three
// per-row checks INDIVIDUALLY.
//
// They are the only evidence that the key about to become authoritative works,
// and a rotation that promotes past any one of them replaces the key file over
// ciphertext nothing can open. Each case below is constructed so that exactly
// ONE of the three catches it and the other two pass, which is what makes the
// test fail when that check alone is neutered — a case that trips two checks
// would still go green with either of them gone. See REVIEW-LOG.md RK-04.
func TestVerifyRefusesToPromoteOnEachWayARowCanLie(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"

	// Each case returns the substring the refusal must contain. The row is set
	// up against a keyring that holds BOTH keys, exactly as it does mid-rotation
	// — which is why the other two checks pass in every case: the old key is
	// still there to make an old-id envelope verify.
	for _, tc := range []struct {
		name string
		// derange puts the row into the shape this case is about.
		derange func(t *testing.T, a *app, id int64, newID uint32)
		want    string
		why     string
	}{
		{
			name: "the kek_id column never moved",
			derange: func(*testing.T, *app, int64, uint32) {
				// Nothing: the row is left at the old id, envelope and column
				// agreeing, and the old key still in the ring. Only the
				// column-names-the-new-id check can see this.
			},
			want: "is still at kek id",
			why:  "a row the re-wrap pass never reached",
		},
		{
			name: "the column says new but the envelope still says old",
			derange: func(t *testing.T, a *app, id int64, newID uint32) {
				// The column is advanced WITHOUT re-wrapping. The envelope is
				// intact and opens under the old key, so crypto.Verify passes;
				// only the column-versus-envelope check disagrees.
				writeEnvelope(t, a, id, envelopeOf(t, a, id), newID)
			},
			want: "but the envelope says",
			why:  "a half-applied re-wrap: metadata moved, ciphertext did not",
		},
		{
			name: "the wrapped DEK does not unwrap",
			derange: func(t *testing.T, a *app, id int64, newID uint32) {
				// A correctly re-wrapped envelope with one byte of the wrapped
				// DEK flipped. Both id checks pass — the header and the column
				// agree on the new id — and only the RFC 3394 integrity check
				// on the DEK notices.
				env, err := a.keyring.Rewrap(envelopeOf(t, a, id), newID)
				if err != nil {
					t.Fatal(err)
				}
				env[crypto.HeaderLen-1] ^= 0xff
				writeEnvelope(t, a, id, env, newID)
			},
			want: "authenticated decryption failed",
			why:  "an envelope whose key material was corrupted in flight",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prowlarr := newFakeProwlarr(t, apiKey)
			dir := t.TempDir()
			instanceID := sealOneCredential(t, dir, prowlarr.URL(), apiKey)
			a, err := startOn(t, dir)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			newID := registerSecondKey(t, a)
			tc.derange(t, a, instanceID, newID)

			var out bytes.Buffer
			verified, err := verifyEverything(context.Background(), a.store, a.keyring, newID, &out)
			if err == nil {
				t.Fatalf("verify PASSED on %s (%d row(s) verified); "+
					"promotion would have replaced the key file over a row it had not proven", tc.why, verified)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not name the problem (%s): %v", tc.want, err)
			}
			// Every refusal must say the same thing about the key files, or an
			// operator reading it cannot tell a recoverable state from a lost one.
			if !strings.Contains(err.Error(), "every key file is unchanged") {
				t.Errorf("the refusal does not say the key files are untouched: %v", err)
			}
			if !strings.Contains(err.Error(), "service_instance") {
				t.Errorf("the refusal does not name the row: %v", err)
			}
			t.Logf("refused with: %v", err)
		})
	}
}

// TestKeyRotateRefusesAPendingKeyIdenticalToTheLiveOne drills prepareRotation's
// same-key refusal.
//
// The realistic way in is an operator who `cp`s secret.key to secret.key.new by
// hand. Without the check the command runs to completion and reports a
// successful rotation that changed nothing — which is the worst possible
// outcome, because the reason to rotate is that the old key is compromised and
// the operator now believes it is not. See REVIEW-LOG.md RK-04.
func TestKeyRotateRefusesAPendingKeyIdenticalToTheLiveOne(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()
	sealOneCredential(t, dir, prowlarr.URL(), apiKey)

	live, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(dir, "keys", "secret.key.new")
	if err := os.WriteFile(pending, live, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := rotateOn(t, dir, nil)
	if err == nil {
		t.Fatalf("key rotate SUCCEEDED on a pending key identical to the live one; "+
			"it reported a rotation that changed nothing\n%s", out)
	}
	if !strings.Contains(err.Error(), "same key") {
		t.Errorf("the refusal does not say the two files hold the same key: %v", err)
	}
	if !strings.Contains(err.Error(), "secret.key.new") {
		t.Errorf("the refusal does not name the file to remove: %v", err)
	}
	// A refusal that deleted the operator's file would be a different bug.
	if _, statErr := os.Stat(pending); statErr != nil {
		t.Errorf("the refusal removed %s: %v", pending, statErr)
	}
	after, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(live, after) {
		t.Error("the refusal still replaced secret.key")
	}
	t.Logf("refused with: %v", err)
}

// execOnWriter runs one statement on the app's writer connection. Only the
// tests below use it, and only to install a trigger that simulates a SECOND
// process writing to service_instance while the rotation runs — which is the
// condition there is no lock against (REVIEW-LOG.md FI-06) and the one thing a
// single-process test otherwise cannot produce deterministically.
func execOnWriter(t *testing.T, a *app, stmt string) {
	t.Helper()
	if err := a.db.Write(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, stmt)
		return err
	}); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

// TestKeyRotateGivesUpWhenRowsKeepAppearingAtTheOldKey drills the
// rotateExtraPasses exhaustion branch.
//
// The trigger below is a stand-in for the running server FI-06 describes: every
// time the re-wrap pass advances one row to the new id, another row goes back to
// the old one, so the remaining-work count is never zero. The branch under test
// is what stops that from being an infinite loop AND what stops it from being a
// promotion over rows still at the old key. See REVIEW-LOG.md RK-04.
func TestKeyRotateGivesUpWhenRowsKeepAppearingAtTheOldKey(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	const secondKey = "prowlarrKEY0011223344556677889900"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	sealOneCredential(t, dir, prowlarr.URL(), apiKey)
	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	sealExtraInstance(t, a, "Radarr", "http://radarr.lan:7878", secondKey)
	oldID := a.keyring.PrimaryID()

	// Recursive triggers are off by default in SQLite, so the UPDATE inside this
	// one does not re-fire it: each outer row update drags exactly one other row
	// back, and the two rows ping-pong for as long as the loop keeps passing.
	execOnWriter(t, a, fmt.Sprintf(`
		CREATE TRIGGER rot_test_reseal AFTER UPDATE OF kek_id ON service_instance
		BEGIN
		  UPDATE service_instance SET kek_id = %d WHERE id <> NEW.id;
		END`, oldID))
	keyBefore, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out, err := rotateOn(t, dir, nil)
	if err == nil {
		t.Fatalf("key rotate SUCCEEDED while rows kept returning to the old key id; "+
			"it promoted over ciphertext the new key cannot open\n%s", out)
	}
	if !strings.Contains(err.Error(), "still wrapped under another key id") {
		t.Errorf("the refusal does not say what is wrong: %v", err)
	}
	if !strings.Contains(err.Error(), "Stop the UsArr server") {
		t.Errorf("the refusal does not name the real remedy: %v", err)
	}
	// The point of failing closed: everything is still recoverable.
	keyAfter, err := os.ReadFile(filepath.Join(dir, "keys", "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("secret.key was replaced by a rotation that never finished re-wrapping")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "keys", "secret.key.new")); statErr != nil {
		t.Errorf("the pending key was discarded, so the rotation cannot be resumed: %v", statErr)
	}
	t.Logf("gave up with: %v", err)
}

// TestKeyRotateReportsCredentialsSealedWhileItRan is defect RK-02: a rotation
// that completes while a server is up leaves that server's newly sealed rows
// under the retired key, and the command used to exit 0 saying "rotation
// complete".
//
// The trigger fires on the promote audit row, which is the last write the
// command makes before it re-counts — so it lands in exactly the window the
// verify pass cannot cover, deterministically and with no goroutine race.
func TestKeyRotateReportsCredentialsSealedWhileItRan(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	const secondKey = "prowlarrKEY0011223344556677889900"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	sealOneCredential(t, dir, prowlarr.URL(), apiKey)
	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	strandedID := sealExtraInstance(t, a, "Radarr", "http://radarr.lan:7878", secondKey)
	oldID := a.keyring.PrimaryID()
	execOnWriter(t, a, fmt.Sprintf(`
		CREATE TRIGGER rot_test_late_seal AFTER INSERT ON audit_log
		WHEN NEW.action = 'key.rotate.promote'
		BEGIN
		  UPDATE service_instance SET kek_id = %d WHERE id = %d;
		END`, oldID, strandedID))
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out, err := rotateOn(t, dir, nil)
	if err == nil {
		t.Fatalf("key rotate reported success with a credential stranded under the retired key\n%s", out)
	}
	t.Logf("key rotate output:\n%s\nerror: %v", out, err)

	// The report must name the instance: a bare count is not actionable.
	if !strings.Contains(out, "Radarr") {
		t.Errorf("the report does not name the affected instance:\n%s", out)
	}
	if !strings.Contains(err.Error(), "re-enter the API key") {
		t.Errorf("the error does not say what the operator must do: %v", err)
	}
	// And it must not read as a failed rotation, because the rotation is done.
	if !strings.Contains(err.Error(), "rotation itself is complete") {
		t.Errorf("the error does not say the rotation stands: %v", err)
	}
	if strings.Contains(out, "rotation complete. Back up") {
		t.Errorf("the command still printed its unqualified success line:\n%s", out)
	}

	// The promotion really happened — this is a report, not a rollback.
	if _, statErr := os.Stat(filepath.Join(dir, "keys", "secret.key.new")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("secret.key.new survived: the rotation did not actually promote: %v", statErr)
	}
}

// TestStartupReportsStrandedCredentials is the other half of RK-02: once a row
// is stranded, nothing said so until somebody used that credential and got a red
// connection test. It must be reported at every start, by name.
//
// It WARNS rather than refusing, and the assertion below pins that: the repair
// is to re-enter the API key through the UI, which needs the server up. See
// warnStrandedCredentials.
func TestStartupReportsStrandedCredentials(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	const secondKey = "prowlarrKEY0011223344556677889900"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	sealOneCredential(t, dir, prowlarr.URL(), apiKey)
	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	strandedID := sealExtraInstance(t, a, "Stranded Radarr", "http://radarr.lan:7878", secondKey)
	// A key id nothing on disk derives — what a row sealed under a key that has
	// since been rotated away looks like on the next start.
	execOnWriter(t, a, fmt.Sprintf(
		`UPDATE service_instance SET kek_id = 424242 WHERE id = %d`, strandedID))
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	logs := &syncBuffer{}
	cfg, err := config.Load(config.Options{
		Args: []string{"--config-dir", dir}, Env: map[string]string{}, Version: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	after, err := buildApp(context.Background(), cfg,
		slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		httpapi.BuildInfo{Version: "0.0.0-test", Commit: "test", GoVersion: goVersion()})
	if after != nil {
		t.Cleanup(func() { _ = after.Close() })
	}
	// It WARNS. A refusal here would make the one action that repairs the row
	// — re-entering the API key in the UI — impossible.
	if err != nil {
		t.Fatalf("a stranded credential must not stop the server starting: %v", err)
	}
	got := logs.String()
	if !strings.Contains(got, "Stranded Radarr") {
		t.Errorf("startup did not name the stranded instance:\n%s", got)
	}
	if !strings.Contains(got, "424242") {
		t.Errorf("startup did not report the key id nothing holds:\n%s", got)
	}
	// ADR-0049 says the key id is logged at startup. It is, on every start.
	if !strings.Contains(got, "keyring ready") {
		t.Errorf("startup did not log the active key ids:\n%s", got)
	}
}

// TestUnreadablePendingKeyRefusesToAdviseRemoval is defect RK-03. The startup
// error for an unreadable secret.key.new used to say "restore or remove it" in
// every case, and on a rotation interrupted mid-re-wrap the second half of that
// advice is permanent, silent destruction of every row already moved to the
// pending key.
func TestUnreadablePendingKeyRefusesToAdviseRemoval(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	prowlarr := newFakeProwlarr(t, apiKey)

	t.Run("rows depend on it", func(t *testing.T) {
		dir := t.TempDir()
		instanceID := sealOneCredential(t, dir, prowlarr.URL(), apiKey)
		a, err := startOn(t, dir)
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		// A genuine partial rotation: pending key registered, one row moved.
		newID := registerSecondKey(t, a)
		moved, err := a.keyring.Rewrap(envelopeOf(t, a, instanceID), newID)
		if err != nil {
			t.Fatal(err)
		}
		writeEnvelope(t, a, instanceID, moved, newID)
		if err := a.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		// Now make the pending key unreadable, the way a truncating write or a
		// half-restored backup does.
		if err := os.WriteFile(filepath.Join(dir, "keys", "secret.key.new"), []byte("nonsense"), 0o600); err != nil {
			t.Fatal(err)
		}

		_, err = startOn(t, dir)
		if err == nil {
			t.Fatal("startup accepted an unreadable secret.key.new")
		}
		if !strings.Contains(err.Error(), "ONLY SAFE ROUTE") {
			t.Errorf("the error does not say restoring is the only safe route: %v", err)
		}
		if !strings.Contains(err.Error(), "1 stored credential") {
			t.Errorf("the error does not say how many credentials are at stake: %v", err)
		}
		if !strings.Contains(err.Error(), "destroys those credentials") {
			t.Errorf("the error does not say what removal costs: %v", err)
		}
		t.Logf("refused with: %v", err)
	})

	t.Run("nothing depends on it", func(t *testing.T) {
		dir := t.TempDir()
		sealOneCredential(t, dir, prowlarr.URL(), apiKey)
		// A rotation that died in prepare: the file exists, no row moved to it.
		if err := os.WriteFile(filepath.Join(dir, "keys", "secret.key.new"), []byte("nonsense"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := startOn(t, dir)
		if err == nil {
			t.Fatal("startup accepted an unreadable secret.key.new")
		}
		if !strings.Contains(err.Error(), "both safe") {
			t.Errorf("the error withholds the safe remedy when removal really is safe: %v", err)
		}
		t.Logf("refused with: %v", err)
	})
}

// TestKeyRotateRefusesAnEmptyConfigDirectory: with no secret.key, the command
// used to fall through to the ordinary startup ladder, which GENERATES a master
// key and logs "back it up now" — and then rotated that key away in the same
// run, leaving the notice naming material already superseded.
func TestKeyRotateRefusesAnEmptyConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	out, err := rotateOn(t, dir, nil)
	if err == nil {
		t.Fatalf("key rotate SUCCEEDED on an empty config directory\n%s", out)
	}
	if !strings.Contains(err.Error(), "no master key to rotate") {
		t.Errorf("the refusal does not say there is nothing to rotate: %v", err)
	}
	if !strings.Contains(err.Error(), "run the server once") {
		t.Errorf("the refusal does not say how to get a key: %v", err)
	}
	// And it must not have made one on the way past.
	if _, statErr := os.Stat(filepath.Join(dir, "keys", "secret.key")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the refusal generated a master key anyway: %v", statErr)
	}
	t.Logf("refused with: %v", err)
}
