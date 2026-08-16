package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fixtureKey is a fixture key. It is structurally obvious as a fixture — 32 bytes
// of 0xAB — so it can never be mistaken for a real credential by a reader or by
// `make secrets`.
var fixtureKey = base64.StdEncoding.EncodeToString(fixtureKeyBytes())

func fixtureKeyBytes() []byte {
	b := make([]byte, MasterKeyLen)
	for i := range b {
		b[i] = 0xAB
	}
	return b
}

func testConfig(t *testing.T, env map[string]string) *Config {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	if _, ok := env["USARR_CONFIG_DIR"]; !ok {
		env["USARR_CONFIG_DIR"] = t.TempDir()
	}
	c, err := Load(Options{Env: env})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

// TestValidateMasterKey covers every validation rule in
// docs/CONFIGURATION.md §3.2. All four failures are fatal and named; there is
// no best-effort branch, no hashing-to-length fallback and no truncation.
func TestValidateMasterKey(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "valid padded", raw: fixtureKey},
		{name: "valid unpadded", raw: strings.TrimRight(fixtureKey, "=")},
		{name: "surrounding whitespace tolerated", raw: "  " + fixtureKey + "\n"},

		{name: "not base64", raw: "this is not base64 !!!", wantErr: "not valid base64"},
		{
			name:    "31 bytes",
			raw:     base64.StdEncoding.EncodeToString(make([]byte, 31)),
			wantErr: "decodes to 31 bytes",
		},
		{
			name:    "33 bytes",
			raw:     base64.StdEncoding.EncodeToString(make([]byte, 33)),
			wantErr: "decodes to 33 bytes",
		},
		{
			name:    "all zero",
			raw:     base64.StdEncoding.EncodeToString(make([]byte, MasterKeyLen)),
			wantErr: "zero bytes",
		},
		{
			name:    "shipped placeholder",
			raw:     "REPLACE_ME_WITH_OUTPUT_OF_openssl_rand_base64_32",
			wantErr: "placeholder value shipped in .env.example",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, err := ValidateMasterKey(tc.raw, "USARR_SECRET_KEY")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateMasterKey: %v", err)
				}
				if len(key) != MasterKeyLen {
					t.Fatalf("key length = %d, want %d", len(key), MasterKeyLen)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateMasterKey = nil error, want %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
			// Every fatal case names the variable it came from.
			if !strings.Contains(err.Error(), "USARR_SECRET_KEY") {
				t.Errorf("error %q does not name the variable", err)
			}
		})
	}
}

// TestStartupLadder walks every row of the table in docs/CONFIGURATION.md §3.2.
func TestStartupLadder(t *testing.T) {
	tests := []struct {
		name string
		// env, minus USARR_CONFIG_DIR which the harness supplies.
		env map[string]string
		// writeKeyFile writes $CONFIG_DIR/keys/secret.key before resolving.
		writeKeyFile string
		// writeExternal writes a file and points USARR_SECRET_KEY_FILE at it.
		writeExternal string

		wantSource  KeySource
		wantAbsent  bool
		wantErr     string
		wantIgnored bool
	}{
		{
			name:       "unset with no key file is the first-run candidate",
			wantAbsent: true,
		},
		{
			name:         "unset with a key file reads it",
			writeKeyFile: fixtureKey,
			wantSource:   KeySourceKeyFile,
		},
		{
			name:       "variable set is used",
			env:        map[string]string{"USARR_SECRET_KEY": fixtureKey},
			wantSource: KeySourceEnv,
		},
		{
			name:         "variable set ignores the key file, and says so",
			env:          map[string]string{"USARR_SECRET_KEY": fixtureKey},
			writeKeyFile: fixtureKey,
			wantSource:   KeySourceEnv,
			wantIgnored:  true,
		},
		{
			name:          "file variable is read",
			writeExternal: fixtureKey,
			wantSource:    KeySourceFileVar,
		},
		{
			// Empty is not "absent". A compose file passing ${USARR_SECRET_KEY}
			// from an unset host variable resolves to empty, and silently
			// generating a key there produces one the operator never backs up.
			name:    "empty string refuses to start",
			env:     map[string]string{"USARR_SECRET_KEY": ""},
			wantErr: "empty value",
		},
		{
			name:    "whitespace-only is also empty",
			env:     map[string]string{"USARR_SECRET_KEY": "   "},
			wantErr: "empty value",
		},
		{
			name:          "both channels set refuses to start",
			env:           map[string]string{"USARR_SECRET_KEY": fixtureKey},
			writeExternal: fixtureKey,
			wantErr:       "mutually exclusive",
		},
		{
			name:    "invalid key in the variable refuses to start",
			env:     map[string]string{"USARR_SECRET_KEY": "short"},
			wantErr: "USARR_SECRET_KEY",
		},
		{
			name:         "invalid key in the key file refuses to start, naming the file",
			writeKeyFile: "not base64 at all !!",
			wantErr:      "secret.key",
		},
		{
			name:          "placeholder in an external file names that file",
			writeExternal: "REPLACE_ME_WITH_OUTPUT_OF_openssl_rand_base64_32",
			wantErr:       "placeholder",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			env := map[string]string{"USARR_CONFIG_DIR": dir}
			for k, v := range tc.env {
				env[k] = v
			}
			if tc.writeExternal != "" {
				p := filepath.Join(dir, "external.key")
				if err := os.WriteFile(p, []byte(tc.writeExternal), 0o600); err != nil {
					t.Fatal(err)
				}
				env["USARR_SECRET_KEY_FILE"] = p
			}

			c := testConfig(t, env)
			if tc.writeKeyFile != "" {
				if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(c.SecretKeyPath(), []byte(tc.writeKeyFile), SecretMode); err != nil {
					t.Fatal(err)
				}
			}

			mk, err := c.ResolveMasterKey()

			switch {
			case tc.wantAbsent:
				if !errors.Is(err, ErrKeyAbsent) {
					t.Fatalf("ResolveMasterKey = (%v, %v), want ErrKeyAbsent", mk, err)
				}
			case tc.wantErr != "":
				if err == nil {
					t.Fatalf("ResolveMasterKey = nil error, want %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err, tc.wantErr)
				}
			default:
				if err != nil {
					t.Fatalf("ResolveMasterKey: %v", err)
				}
				if mk.Source != tc.wantSource {
					t.Errorf("Source = %q, want %q", mk.Source, tc.wantSource)
				}
				if len(mk.Key) != MasterKeyLen {
					t.Errorf("key length = %d, want %d", len(mk.Key), MasterKeyLen)
				}
				if tc.wantIgnored && mk.IgnoredKeyFile == "" {
					t.Error("IgnoredKeyFile is empty; the operator is not told the file was ignored")
				}
			}
		})
	}
}

// The one ladder branch that cannot be decided until the database is open.
func TestMissingKeyWithEncryptedRowsIsFatal(t *testing.T) {
	c := testConfig(t, nil)

	if _, err := c.ResolveMasterKey(); !errors.Is(err, ErrKeyAbsent) {
		t.Fatalf("ResolveMasterKey = %v, want ErrKeyAbsent", err)
	}

	err := c.ErrMissingKeyForExistingData(3)
	for _, want := range []string{"3 encrypted", "USARR_SECRET_KEY", c.SecretKeyPath(), "Refusing to start"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestGenerateMasterKey(t *testing.T) {
	c := testConfig(t, nil)

	mk, err := c.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey: %v", err)
	}
	if len(mk.Key) != MasterKeyLen {
		t.Fatalf("key length = %d, want %d", len(mk.Key), MasterKeyLen)
	}
	if mk.Source != KeySourceGenerated {
		t.Errorf("Source = %q, want %q", mk.Source, KeySourceGenerated)
	}
	// "Back up the key file" is correct advice only because the KEK salt now
	// sits beside the database, where an ordinary backup catches it. If the salt
	// ever moves back under keys/, this notice becomes a trap again.
	notice := mk.BackupNotice()
	if !strings.Contains(notice, "BACK IT UP") {
		t.Errorf("BackupNotice() = %q, want a back-it-up line", notice)
	}
	if strings.HasPrefix(c.KEKSaltPath(), c.KeysDir()+string(os.PathSeparator)) {
		t.Errorf("the KEK salt is inside %s, which the documented backup excludes; "+
			"BackupNotice would then be telling the operator to save half of what "+
			"they need", c.KeysDir())
	}

	// keys/ is 0700 and secret.key is 0600, set by UsArr regardless of umask.
	di, err := os.Stat(c.KeysDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := di.Mode().Perm(); got != SecretDirMode.Perm() {
		t.Errorf("keys/ mode = %#o, want %#o", got, SecretDirMode.Perm())
	}
	fi, err := os.Stat(c.SecretKeyPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != SecretMode.Perm() {
		t.Errorf("secret.key mode = %#o, want %#o", got, SecretMode.Perm())
	}

	// The written key round-trips through the ladder.
	got, err := c.ResolveMasterKey()
	if err != nil {
		t.Fatalf("ResolveMasterKey after generate: %v", err)
	}
	if string(got.Key) != string(mk.Key) {
		t.Error("key read back from disk differs from the generated key")
	}
	if got.Source != KeySourceKeyFile {
		t.Errorf("Source = %q, want %q", got.Source, KeySourceKeyFile)
	}

	// Never clobber an existing key: that would destroy every credential.
	if _, err := c.GenerateMasterKey(); err == nil {
		t.Error("GenerateMasterKey overwrote an existing key file")
	}
}

// Two runs must not produce the same key. This is a smoke test for a wired-up
// crypto/rand, not a statistical test.
func TestGenerateMasterKeyIsRandom(t *testing.T) {
	seen := map[string]bool{}
	for range 8 {
		c := testConfig(t, nil)
		mk, err := c.GenerateMasterKey()
		if err != nil {
			t.Fatalf("GenerateMasterKey: %v", err)
		}
		k := string(mk.Key)
		if seen[k] {
			t.Fatal("GenerateMasterKey returned a repeated key")
		}
		seen[k] = true
	}
}

func TestResolveKEKSalt(t *testing.T) {
	c := testConfig(t, nil)

	salt, err := c.GenerateKEKSalt()
	if err != nil {
		t.Fatalf("GenerateKEKSalt: %v", err)
	}
	if len(salt) != KEKSaltLen {
		t.Fatalf("salt length = %d, want %d", len(salt), KEKSaltLen)
	}

	// Stable across calls: a changing salt would change the KEK and orphan
	// every stored credential.
	again, err := c.ResolveKEKSalt()
	if err != nil {
		t.Fatalf("ResolveKEKSalt: %v", err)
	}
	if string(again) != string(salt) {
		t.Error("ResolveKEKSalt returned a different salt on the second call")
	}

	fi, err := os.Stat(c.KEKSaltPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != SecretMode.Perm() {
		t.Errorf("kek.salt mode = %#o, want %#o", got, SecretMode.Perm())
	}
}

// TestResolveKEKSaltNeverCreates is the unit-level half of the data-loss
// regression. A missing salt is reported, never invented: the caller has to ask
// the database whether anything is sealed before it may generate one.
func TestResolveKEKSaltNeverCreates(t *testing.T) {
	c := testConfig(t, nil)

	salt, err := c.ResolveKEKSalt()
	if !errors.Is(err, ErrKEKSaltAbsent) {
		t.Fatalf("ResolveKEKSalt = (%v, %v), want ErrKEKSaltAbsent", salt, err)
	}
	if _, statErr := os.Stat(c.KEKSaltPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("ResolveKEKSalt created %s; it must never write a salt. "+
			"A regenerated salt derives a different KEK and permanently destroys "+
			"every stored credential", c.KEKSaltPath())
	}
}

// TestGenerateKEKSaltNeverClobbers: an existing salt is adopted, never
// replaced. Overwriting one is exactly as destructive as overwriting the master
// key, so a caller that reaches this by mistake — or a second process that
// raced here — must end up with the salt already on disk.
func TestGenerateKEKSaltNeverClobbers(t *testing.T) {
	c := testConfig(t, nil)

	first, err := c.GenerateKEKSalt()
	if err != nil {
		t.Fatalf("GenerateKEKSalt: %v", err)
	}
	second, err := c.GenerateKEKSalt()
	if err != nil {
		t.Fatalf("a second GenerateKEKSalt must adopt the existing salt: %v", err)
	}
	if !bytes.Equal(second, first) {
		t.Fatal("GenerateKEKSalt returned a different salt than the one already on disk")
	}

	after, err := c.ResolveKEKSalt()
	if err != nil {
		t.Fatalf("ResolveKEKSalt: %v", err)
	}
	if !bytes.Equal(after, first) {
		t.Error("the salt on disk changed after a second GenerateKEKSalt")
	}
}

// TestGenerateKEKSaltAdoptsALegacySalt is the upgrade-path corner the row count
// cannot see: a pre-existing install whose database happens to hold no sealed
// credential yet still has a salt in keys/, and it must be adopted rather than
// replaced — anything sealed under it later would otherwise be unopenable.
func TestGenerateKEKSaltAdoptsALegacySalt(t *testing.T) {
	c := testConfig(t, nil)
	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{0x2B}, KEKSaltLen)
	blob := base64.StdEncoding.EncodeToString(want) + "\n"
	if err := os.WriteFile(c.LegacyKEKSaltPath(), []byte(blob), SecretMode); err != nil {
		t.Fatal(err)
	}

	got, err := c.GenerateKEKSalt()
	if err != nil {
		t.Fatalf("GenerateKEKSalt: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("GenerateKEKSalt invented a salt while a legacy one existed")
	}
}

// TestErrMissingSaltForExistingDataNamesTheSalt pins the one thing the old
// message got wrong: it must name kek.salt, not send an operator who is already
// holding the master key off to restore the master key. It must also name the
// legacy path, since an old backup keeps the salt there.
func TestErrMissingSaltForExistingDataNamesTheSalt(t *testing.T) {
	c := testConfig(t, nil)
	msg := c.ErrMissingSaltForExistingData(4).Error()

	for _, want := range []string{
		c.KEKSaltPath(), c.LegacyKEKSaltPath(), "kek.salt", "4 encrypted credential",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrMissingSaltForExistingData does not mention %q: %s", want, msg)
		}
	}
}

// TestKEKSaltLivesOutsideKeysDir is the structural guard behind the whole fix.
// keys/ is excluded from the documented backup; a non-secret that the database
// cannot be opened without must not be in there, or correct backup advice
// silently produces an unrecoverable archive.
func TestKEKSaltLivesOutsideKeysDir(t *testing.T) {
	c := testConfig(t, nil)

	if dir := filepath.Dir(c.KEKSaltPath()); dir != c.ConfigDir {
		t.Errorf("KEKSaltPath is in %s, want it beside usarr.db in %s", dir, c.ConfigDir)
	}
	if filepath.Dir(c.KEKSaltPath()) == c.KeysDir() {
		t.Error("the KEK salt is back inside keys/, the directory the documented " +
			"backup excludes. That is the data-loss bug this test exists to prevent")
	}
	if c.LegacyKEKSaltPath() != filepath.Join(c.KeysDir(), "kek.salt") {
		t.Errorf("LegacyKEKSaltPath = %s, want the pre-move keys/kek.salt so existing "+
			"installs are migrated rather than read as empty", c.LegacyKEKSaltPath())
	}
}

// TestResolveKEKSaltRelocatesLegacySalt is the unit-level upgrade path: an
// install created before the move must have its salt carried forward BYTE FOR
// BYTE, because a different salt is a different KEK.
func TestResolveKEKSaltRelocatesLegacySalt(t *testing.T) {
	c := testConfig(t, nil)
	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{0x5A}, KEKSaltLen)
	legacyBlob := base64.StdEncoding.EncodeToString(want) + "\n"
	if err := os.WriteFile(c.LegacyKEKSaltPath(), []byte(legacyBlob), SecretMode); err != nil {
		t.Fatal(err)
	}

	got, err := c.ResolveKEKSalt()
	if err != nil {
		t.Fatalf("ResolveKEKSalt with a legacy salt present: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the relocated salt is not the legacy salt; every credential would be lost")
	}

	again, err := c.readKEKSalt(c.KEKSaltPath())
	if err != nil {
		t.Fatalf("read relocated salt: %v", err)
	}
	if !bytes.Equal(again, want) {
		t.Fatal("the file at the new path does not hold the legacy salt")
	}

	// The legacy file MUST still be there. This is a copy, never a move: a move
	// has an instant in which neither path holds the salt, and a process killed
	// in that instant loses the key-derivation input permanently. Two processes
	// can start against one config directory, so that instant is reachable.
	surviving, err := c.readKEKSalt(c.LegacyKEKSaltPath())
	if err != nil {
		t.Fatalf("the legacy salt was deleted; this must be a copy, not a move: %v", err)
	}
	if !bytes.Equal(surviving, want) {
		t.Fatal("the legacy salt was modified")
	}

	// Idempotent: a second start is an ordinary read.
	third, err := c.ResolveKEKSalt()
	if err != nil || !bytes.Equal(third, want) {
		t.Fatalf("ResolveKEKSalt after relocation = (%x, %v), want the same salt", third, err)
	}
}

// TestResolveKEKSaltKeepsLegacyOnRelocationFailure: if the new file cannot be
// written, the legacy salt must survive untouched and stay authoritative.
// Losing it while trying to protect it would be the worst possible outcome of
// this change.
func TestResolveKEKSaltKeepsLegacyOnRelocationFailure(t *testing.T) {
	c := testConfig(t, nil)
	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{0x77}, KEKSaltLen)
	blob := base64.StdEncoding.EncodeToString(want) + "\n"
	if err := os.WriteFile(c.LegacyKEKSaltPath(), []byte(blob), SecretMode); err != nil {
		t.Fatal(err)
	}
	// Block the write by putting a DIRECTORY where the new file must go.
	if err := os.MkdirAll(c.KEKSaltPath(), DirMode); err != nil {
		t.Fatal(err)
	}

	if _, err := c.ResolveKEKSalt(); err == nil {
		t.Fatal("ResolveKEKSalt reported success when it could not write the relocated salt")
	}
	survived, err := c.readKEKSalt(c.LegacyKEKSaltPath())
	if err != nil {
		t.Fatalf("the legacy salt did not survive a failed relocation: %v", err)
	}
	if !bytes.Equal(survived, want) {
		t.Fatal("the legacy salt was altered by a failed relocation")
	}
}

func TestResolveKEKSaltRejectsWrongLength(t *testing.T) {
	c := testConfig(t, nil)
	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		t.Fatal(err)
	}
	short := base64.StdEncoding.EncodeToString(make([]byte, 8))
	if err := os.WriteFile(c.KEKSaltPath(), []byte(short), SecretMode); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveKEKSalt(); err == nil {
		t.Error("ResolveKEKSalt accepted an 8-byte salt")
	}
}

// TestResolveKEKSaltIsConcurrencySafe: nothing stops two UsArr processes
// starting against one config directory, so the copy-forward must be safe to
// run simultaneously with itself. Every caller must get the same salt, and no
// caller may ever observe a partially written file.
func TestResolveKEKSaltIsConcurrencySafe(t *testing.T) {
	c := testConfig(t, nil)
	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{0x3C}, KEKSaltLen)
	blob := base64.StdEncoding.EncodeToString(want) + "\n"
	if err := os.WriteFile(c.LegacyKEKSaltPath(), []byte(blob), SecretMode); err != nil {
		t.Fatal(err)
	}

	const racers = 16
	var wg sync.WaitGroup
	got := make([][]byte, racers)
	errs := make([]error, racers)
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got[i], errs[i] = c.ResolveKEKSalt()
		}()
	}
	close(start)
	wg.Wait()

	for i := range racers {
		if errs[i] != nil {
			t.Fatalf("racer %d: %v", i, errs[i])
		}
		if !bytes.Equal(got[i], want) {
			t.Fatalf("racer %d resolved a different salt; credentials sealed by one "+
				"process would be unopenable by the other", i)
		}
	}
	// Both copies intact, neither partial.
	for _, p := range []string{c.KEKSaltPath(), c.LegacyKEKSaltPath()} {
		salt, err := c.readKEKSalt(p)
		if err != nil {
			t.Fatalf("%s after the race: %v", p, err)
		}
		if !bytes.Equal(salt, want) {
			t.Errorf("%s holds the wrong salt after the race", p)
		}
	}
	// No temp files left behind.
	entries, err := os.ReadDir(c.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".kek.salt.tmp-") {
			t.Errorf("a temp file survived the race: %s", e.Name())
		}
	}
}

// TestGenerateKEKSaltConvergesUnderRace: two genuine first runs at the same
// instant must agree on one salt. If they diverged, a credential sealed by one
// process would be unopenable by the other, forever.
func TestGenerateKEKSaltConvergesUnderRace(t *testing.T) {
	c := testConfig(t, nil)

	const racers = 8
	var wg sync.WaitGroup
	got := make([][]byte, racers)
	errs := make([]error, racers)
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got[i], errs[i] = c.GenerateKEKSalt()
		}()
	}
	close(start)
	wg.Wait()

	onDisk, err := c.readKEKSalt(c.KEKSaltPath())
	if err != nil {
		t.Fatalf("read the generated salt: %v", err)
	}
	for i := range racers {
		if errs[i] != nil {
			t.Fatalf("racer %d: %v", i, errs[i])
		}
		if !bytes.Equal(got[i], onDisk) {
			t.Fatalf("racer %d returned a salt that is not the one on disk; it would "+
				"seal credentials under a KEK nothing else can derive", i)
		}
	}
}

// TestWriteSaltFileNeverReplaces pins the property the whole concurrency story
// rests on. rename(2) would silently replace, and a salt that replaces a
// different salt is unrecoverable: credentials sealed microseconds earlier can
// never be opened again.
func TestWriteSaltFileNeverReplaces(t *testing.T) {
	c := testConfig(t, nil)
	first := bytes.Repeat([]byte{0x11}, KEKSaltLen)
	if err := c.writeSaltFile(c.KEKSaltPath(), first); err != nil {
		t.Fatalf("writeSaltFile: %v", err)
	}

	second := bytes.Repeat([]byte{0x22}, KEKSaltLen)
	err := c.writeSaltFile(c.KEKSaltPath(), second)
	if !errors.Is(err, errSaltAlreadyExists) {
		t.Fatalf("writeSaltFile over an existing salt = %v, want errSaltAlreadyExists", err)
	}

	onDisk, err := c.readKEKSalt(c.KEKSaltPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, first) {
		t.Fatal("an existing salt was replaced; every credential sealed under it is now lost")
	}

	// No temp files left behind on either the success or the refusal path.
	entries, err := os.ReadDir(c.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".kek.salt.tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
