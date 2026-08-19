package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncruces/go-sqlite3/driver"

	"github.com/jdb3750/UsArr/internal/config"
)

// seedDatabase writes a real SQLite file with one recognisable row, so a
// restored snapshot can be proved to hold the data rather than merely to exist.
func seedDatabase(t *testing.T, cfg *config.Config) {
	t.Helper()
	ctx := context.Background()
	conn, err := driver.Open(bareDSN(cfg.DatabasePath()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`CREATE TABLE legacy (id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO legacy (note) VALUES ('irreplaceable')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), config.DirMode); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, config.FileMode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// backupSet returns the .db and .kek.salt files runBackup wrote, so a test can
// assert on the pair without knowing the timestamp in the name.
func backupSet(t *testing.T, cfg *config.Config) (dbFiles, saltFiles []string) {
	t.Helper()
	entries, err := os.ReadDir(cfg.BackupsDir())
	if err != nil {
		t.Fatalf("read %s: %v", cfg.BackupsDir(), err)
	}
	for _, e := range entries {
		p := filepath.Join(cfg.BackupsDir(), e.Name())
		switch {
		case strings.HasSuffix(e.Name(), ".kek.salt"):
			saltFiles = append(saltFiles, p)
		case strings.HasSuffix(e.Name(), ".db"):
			dbFiles = append(dbFiles, p)
		}
	}
	return dbFiles, saltFiles
}

// A .db on its own is not a restorable backup of a UsArr install: the KEK is
// derived from the master key AND the per-install salt, so a snapshot without
// the salt decrypts nothing even when the operator has kept the key perfectly.
// That is not hypothetical — it is the 2026-08-16 defect, and this is the guard
// that keeps `usarr backup` from reintroducing it one level up.
func TestBackupWritesTheDatabaseAndTheSalt(t *testing.T) {
	cfg := testConfig(t)
	seedDatabase(t, cfg)
	salt := bytes.Repeat([]byte{0xA5}, config.KEKSaltLen)
	writeFile(t, cfg.KEKSaltPath(), salt)

	var out bytes.Buffer
	if err := runBackup(context.Background(), cfg, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	dbFiles, saltFiles := backupSet(t, cfg)
	if len(dbFiles) != 1 || len(saltFiles) != 1 {
		t.Fatalf("backup wrote %d db and %d salt files; want one of each\n%s",
			len(dbFiles), len(saltFiles), out.String())
	}

	if dirInfo, err := os.Stat(cfg.BackupsDir()); err != nil {
		t.Fatalf("stat backups dir: %v", err)
	} else if dirInfo.Mode().Perm() != config.DirMode {
		t.Errorf("backups dir mode = %04o, want %04o", dirInfo.Mode().Perm(), config.DirMode)
	}
	for _, p := range append(append([]string{}, dbFiles...), saltFiles...) {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm() != config.FileMode {
			t.Errorf("%s mode = %04o, want %04o", p, info.Mode().Perm(), config.FileMode)
		}
	}

	// The pair must share a stamp, or a directory listing cannot tell which
	// salt belongs to which snapshot.
	if strings.TrimSuffix(filepath.Base(dbFiles[0]), ".db") !=
		strings.TrimSuffix(filepath.Base(saltFiles[0]), ".kek.salt") {
		t.Errorf("the pair does not share a stamp: %s and %s",
			filepath.Base(dbFiles[0]), filepath.Base(saltFiles[0]))
	}

	got, err := os.ReadFile(saltFiles[0])
	if err != nil {
		t.Fatalf("read salt copy: %v", err)
	}
	if !bytes.Equal(got, salt) {
		t.Error("the copied salt does not match the original byte for byte")
	}

	// A backup that does not restore is not a backup.
	restored, err := driver.Open(bareDSN(dbFiles[0]))
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = restored.Close() }()
	var note string
	if err := restored.QueryRowContext(context.Background(),
		`SELECT note FROM legacy`).Scan(&note); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if note != "irreplaceable" {
		t.Errorf("snapshot content = %q, want %q", note, "irreplaceable")
	}

	// The key must never be inside anything the backup writes.
	if _, err := os.Stat(filepath.Join(cfg.BackupsDir(), "keys")); !os.IsNotExist(err) {
		t.Error("keys/ must never appear under backups/")
	}
}

// The hard constraint: a backup that quietly omits the master key is worse than
// no command, because it turns "I should back up properly" into "I have a
// backup". The exclusion has to be in the OUTPUT — the operator is reading a
// terminal, not docs/CONFIGURATION.md.
func TestBackupOutputNamesWhatItDoesNotCover(t *testing.T) {
	cfg := testConfig(t)
	seedDatabase(t, cfg)
	writeFile(t, cfg.KEKSaltPath(), bytes.Repeat([]byte{0x11}, config.KEKSaltLen))

	var out bytes.Buffer
	if err := runBackup(context.Background(), cfg, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	for _, want := range []string{
		"NOT IN THIS BACKUP",
		"the master key",
		cfg.SecretKeyPath(), // where it lives on THIS install
		"password manager",  // where the operator is told to put it
		"On restore",        // and what they then do with it
		"providers/*.yaml",  // the other things this does not carry
		"USARR_DATA_DIR",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("backup output does not mention %q:\n%s", want, out.String())
		}
	}
}

// A command that deliberately reads key-derivation material has to be held to
// its own rule rather than to the redacting log handler's, which it never goes
// through: everything it says is written straight to a writer.
//
// It watches the process's real stdout and stderr as well as the report writer,
// and that is not belt-and-braces. The first version of this test asserted on
// the report only, and a deliberate `fmt.Printf` of the salt bytes inside
// copyKEKSalt passed it — a stray Printf or a debugging line is exactly the
// shape a leak takes here, and it lands on the file descriptor rather than in
// the buffer.
func TestBackupNeverPrintsKeyMaterial(t *testing.T) {
	cfg := testConfig(t)
	seedDatabase(t, cfg)

	const keyText = "TOPSECRETMASTERKEYVALUE0000000000000000000="
	saltText := strings.Repeat("S", config.KEKSaltLen)
	writeFile(t, cfg.SecretKeyPath(), []byte(keyText))
	writeFile(t, cfg.KEKSaltPath(), []byte(saltText))

	var out bytes.Buffer
	console := captureConsole(t, func() {
		if err := runBackup(context.Background(), cfg, &out); err != nil {
			t.Errorf("runBackup: %v", err)
		}
	})

	for _, secret := range []string{keyText, saltText} {
		if strings.Contains(out.String(), secret) {
			t.Errorf("the report leaked material from a key file:\n%s", out.String())
		}
		if strings.Contains(console, secret) {
			t.Errorf("stdout/stderr leaked material from a key file:\n%s", console)
		}
	}
}

// captureConsole runs fn with the process's stdout and stderr redirected, and
// returns everything they were given. The reader runs on its own goroutine
// because a pipe whose buffer fills with nobody draining it deadlocks the
// writer, and a test that hangs is worse than one that fails.
func captureConsole(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	defer func() {
		os.Stdout, os.Stderr = stdout, stderr
		_ = w.Close()
		_ = r.Close()
	}()
	fn()
	os.Stdout, os.Stderr = stdout, stderr
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	return <-done
}

// No salt is a real state — a fresh install has never sealed anything — and it
// is also what a half-restored config directory looks like. Either way the
// report says so rather than writing a pair that is silently a single.
func TestBackupSaysWhenThereIsNoSalt(t *testing.T) {
	cfg := testConfig(t)
	seedDatabase(t, cfg)

	var out bytes.Buffer
	if err := runBackup(context.Background(), cfg, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	if _, saltFiles := backupSet(t, cfg); len(saltFiles) != 0 {
		t.Errorf("a salt file was written with no salt on disk: %v", saltFiles)
	}
	for _, want := range []string{"NOT WRITTEN", cfg.KEKSaltPath(), cfg.LegacyKEKSaltPath()} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("backup output does not mention %q:\n%s", want, out.String())
		}
	}
}

// Installs predating the salt's move still keep it in keys/. The backup reads
// that path too — and does NOT copy it forward into the config directory, the
// way ResolveKEKSalt does, because a backup must not write to the install it is
// reading.
func TestBackupCopiesTheLegacySaltWithoutMovingIt(t *testing.T) {
	cfg := testConfig(t)
	seedDatabase(t, cfg)
	salt := bytes.Repeat([]byte{0x5A}, config.KEKSaltLen)
	writeFile(t, cfg.LegacyKEKSaltPath(), salt)

	var out bytes.Buffer
	if err := runBackup(context.Background(), cfg, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	_, saltFiles := backupSet(t, cfg)
	if len(saltFiles) != 1 {
		t.Fatalf("legacy salt produced %d salt copies; want 1\n%s", len(saltFiles), out.String())
	}
	if got, err := os.ReadFile(saltFiles[0]); err != nil || !bytes.Equal(got, salt) {
		t.Errorf("the legacy salt was not copied byte for byte (err %v)", err)
	}
	if _, err := os.Stat(cfg.LegacyKEKSaltPath()); err != nil {
		t.Errorf("the legacy salt must stay where it is: %v", err)
	}
	if _, err := os.Stat(cfg.KEKSaltPath()); !os.IsNotExist(err) {
		t.Error("a backup must not write to the install it is reading")
	}
	if !strings.Contains(out.String(), cfg.LegacyKEKSaltPath()) {
		t.Errorf("the report does not say the salt came from the legacy path:\n%s", out.String())
	}
}

// An env-supplied key is not in the config directory at all, so "keep a copy of
// keys/secret.key" would be an instruction the operator cannot follow. The
// report has to name the channel this install actually uses.
func TestBackupNamesTheEnvironmentKeyChannel(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(config.Options{
		Args: []string{"--config-dir", dir},
		Env:  map[string]string{"USARR_SECRET_KEY": "not-read-by-this-command"},
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	seedDatabase(t, cfg)

	var out bytes.Buffer
	if err := runBackup(context.Background(), cfg, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	if !strings.Contains(out.String(), "USARR_SECRET_KEY") {
		t.Errorf("the report does not name the environment key channel:\n%s", out.String())
	}
	// And it must not name the other one. `cat /config/keys/secret.key` is an
	// instruction that cannot be followed on this install — the file is not
	// there — so printing it sends the operator looking for a key they will
	// never find and would not need. The mention above is not enough on its
	// own: the restore steps name the variable too, so a location line that had
	// regressed to the file path would still satisfy it.
	if strings.Contains(out.String(), cfg.SecretKeyPath()) {
		t.Errorf("the report points at %s on an install whose key comes from the environment:\n%s",
			cfg.SecretKeyPath(), out.String())
	}
	if strings.Contains(out.String(), "not-read-by-this-command") {
		t.Errorf("the report printed the key value:\n%s", out.String())
	}
}

// `usarr backup` on an install that has never run has nothing to snapshot, and
// an empty backups/ full of zero-byte files is the kind of artefact that reads
// as a backup later.
func TestBackupRefusesWithNoDatabase(t *testing.T) {
	cfg := testConfig(t)
	err := runBackup(context.Background(), cfg, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runBackup on an empty config directory succeeded")
	}
	if !strings.Contains(err.Error(), cfg.DatabasePath()) {
		t.Errorf("the refusal does not name the database: %v", err)
	}
}

// vacuumInto deletes a partial snapshot on failure. It must delete only one it
// created: SQLite refuses to VACUUM INTO a file that already exists, so the
// commonest way to reach that error path is with somebody else's backup sitting
// at the target, and eating it would be a data-loss bug inside the command
// whose whole job is not losing data.
func TestVacuumIntoKeepsAPreexistingTarget(t *testing.T) {
	cfg := testConfig(t)
	seedDatabase(t, cfg)

	target := filepath.Join(t.TempDir(), "someone-elses-backup.db")
	writeFile(t, target, []byte("do not delete me"))

	if err := vacuumInto(context.Background(), cfg.DatabasePath(), target); err == nil {
		t.Fatal("VACUUM INTO an existing file succeeded; the guard below proves nothing")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("the pre-existing target was deleted: %v", err)
	}
	if string(got) != "do not delete me" {
		t.Errorf("the pre-existing target was overwritten: %q", got)
	}
}
