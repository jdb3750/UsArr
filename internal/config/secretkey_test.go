package config

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if !strings.Contains(mk.BackupNotice(), "BACK IT UP") {
		t.Errorf("BackupNotice() = %q, want a back-it-up line", mk.BackupNotice())
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

	salt, err := c.ResolveKEKSalt()
	if err != nil {
		t.Fatalf("ResolveKEKSalt: %v", err)
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
