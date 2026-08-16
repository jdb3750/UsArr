package config

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MasterKeyLen is the master key's exact length. 31 or 33 bytes is an error,
// never something to pad or truncate.
const MasterKeyLen = 32

// KEKSaltLen is the length of the per-install HKDF salt.
const KEKSaltLen = 32

// placeholderKeys is the hardcoded reject-list: every placeholder string ever
// shipped in a released .env.example. A value published in a git repository is
// not a key, so matching one is fatal and the error names where it came from.
//
// Append to this list; never remove from it. A placeholder retired from
// .env.example is still sitting in someone's checked-out copy.
var placeholderKeys = []string{
	"REPLACE_ME_WITH_OUTPUT_OF_openssl_rand_base64_32",
}

// KeySource records where the master key came from, for the startup log line.
type KeySource string

const (
	// KeySourceEnv is USARR_SECRET_KEY.
	KeySourceEnv KeySource = "USARR_SECRET_KEY"
	// KeySourceFileVar is the file named by USARR_SECRET_KEY_FILE.
	KeySourceFileVar KeySource = "USARR_SECRET_KEY_FILE"
	// KeySourceKeyFile is $USARR_CONFIG_DIR/keys/secret.key.
	KeySourceKeyFile KeySource = "key file"
	// KeySourceGenerated means GenerateMasterKey wrote a new key.
	KeySourceGenerated KeySource = "generated"
)

// MasterKey is the resolved master key and where it came from.
type MasterKey struct {
	// Key is exactly MasterKeyLen bytes.
	Key []byte
	// Source names the channel it arrived on.
	Source KeySource
	// Path is the file it was read from or written to, empty for KeySourceEnv.
	Path string
	// IgnoredKeyFile is set when an environment-supplied key took precedence
	// over an existing key file. The startup path logs this: an operator who
	// has both and does not know which won is one restart from a surprise.
	IgnoredKeyFile string
}

// ErrKeyAbsent means the master key was not supplied and no key file exists.
//
// This is not yet an error condition: it is the genuine-first-run candidate.
// The caller must open the database, ask whether it already holds encrypted
// rows, and then either call GenerateMasterKey (empty database) or fail with
// ErrMissingKeyForExistingData (rows present). Never start half-decrypted.
var ErrKeyAbsent = errors.New("config: master key not supplied and no key file present")

// ResolveMasterKey implements the startup ladder in docs/CONFIGURATION.md §3.2.
//
// Every fatal case names the variable or file the bad value came from. There is
// no lenient fallback, no hashing-to-length, no truncation and no shipped
// default anywhere in this function.
func (c *Config) ResolveMasterKey() (*MasterKey, error) {
	envSet := c.SecretKeySet
	fileVarSet := c.SecretKeyFile != ""

	// Both channels set. No guessing.
	if envSet && fileVarSet {
		return nil, errors.New(
			"USARR_SECRET_KEY and USARR_SECRET_KEY_FILE are both set: they are mutually " +
				"exclusive, and UsArr does not guess which you meant. Unset one")
	}

	switch {
	case envSet:
		// Empty is not "absent". The usual way to get an empty value is an
		// unset shell variable interpolated into a compose file
		// (USARR_SECRET_KEY=${USARR_SECRET_KEY}); silently generating a key
		// there would produce a key the operator does not know exists and will
		// never back up.
		if strings.TrimSpace(c.SecretKey) == "" {
			return nil, errors.New(
				"USARR_SECRET_KEY is set to an empty value. Empty is not the same as unset: " +
					"unset the variable entirely to let UsArr generate a key on first run")
		}
		key, err := ValidateMasterKey(c.SecretKey, "USARR_SECRET_KEY")
		if err != nil {
			return nil, err
		}
		mk := &MasterKey{Key: key, Source: KeySourceEnv}
		if _, statErr := os.Stat(c.SecretKeyPath()); statErr == nil {
			mk.IgnoredKeyFile = c.SecretKeyPath()
		}
		return mk, nil

	case fileVarSet:
		key, err := readKeyFile(c.SecretKeyFile, "USARR_SECRET_KEY_FILE")
		if err != nil {
			return nil, err
		}
		mk := &MasterKey{Key: key, Source: KeySourceFileVar, Path: c.SecretKeyFile}
		if abs, statErr := os.Stat(c.SecretKeyPath()); statErr == nil && abs != nil {
			if filepath.Clean(c.SecretKeyFile) != c.SecretKeyPath() {
				mk.IgnoredKeyFile = c.SecretKeyPath()
			}
		}
		return mk, nil
	}

	// Neither channel set: fall back to the key file on the config volume.
	// This is the normal, recommended path after the first run.
	path := c.SecretKeyPath()
	switch _, err := os.Stat(path); {
	case err == nil:
		key, err := readKeyFile(path, path)
		if err != nil {
			return nil, err
		}
		return &MasterKey{Key: key, Source: KeySourceKeyFile, Path: path}, nil
	case errors.Is(err, os.ErrNotExist):
		return nil, ErrKeyAbsent
	default:
		return nil, fmt.Errorf("stat master key file %s: %w", path, err)
	}
}

// ErrMissingKeyForExistingData is the fatal error for the one ladder branch
// that cannot be decided until the database is open: no key was supplied, no
// key file exists, but the database already holds encrypted rows.
//
// The startup path calls this after ResolveMasterKey returns ErrKeyAbsent and
// the store reports encrypted rows present. Generating a fresh key here would
// leave every stored credential permanently unreadable while the process
// happily continued.
func (c *Config) ErrMissingKeyForExistingData(encryptedRows int64) error {
	return fmt.Errorf(
		"the database holds %d encrypted credential(s) but no master key was supplied: "+
			"expected USARR_SECRET_KEY, USARR_SECRET_KEY_FILE, or the key file at %s. "+
			"Refusing to start half-decrypted. If the key is lost, see docs/CONFIGURATION.md §3.5 "+
			"for the re-enter-your-credentials recovery flow",
		encryptedRows, c.SecretKeyPath())
}

// GenerateMasterKey creates the master key on a genuine first run: 32 bytes
// from crypto/rand, keys/ at 0700, secret.key at 0600.
//
// The caller must have established that the database holds no encrypted rows.
// It is an error if the key file already exists — that would mean the ladder
// was not followed, and overwriting it destroys every stored credential.
func (c *Config) GenerateMasterKey() (*MasterKey, error) {
	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		return nil, fmt.Errorf("create %s: %w", c.KeysDir(), err)
	}
	// MkdirAll leaves an existing directory's mode alone, and the config
	// volume may have been created by hand.
	if err := os.Chmod(c.KeysDir(), SecretDirMode); err != nil {
		return nil, fmt.Errorf("chmod %s: %w", c.KeysDir(), err)
	}

	key := make([]byte, MasterKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}

	path := c.SecretKeyPath()
	if err := writeSecretFile(path, base64.StdEncoding.EncodeToString(key)+"\n"); err != nil {
		return nil, err
	}
	return &MasterKey{Key: key, Source: KeySourceGenerated, Path: path}, nil
}

// writeSecretFile creates a new 0600 file and fsyncs it.
//
// O_EXCL is the point: it never clobbers an existing file. Overwriting a master
// key destroys every stored credential, so "create, and fail if it is already
// there" is the only correct mode.
func writeSecretFile(path, content string) (err error) {
	// #nosec G304 -- the path is derived from USARR_CONFIG_DIR, which is
	// operator-supplied level-1 configuration. Choosing where its own state
	// lives is the entire purpose of that variable; there is no untrusted
	// input on this path.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, SecretMode)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() {
		cerr := f.Close()
		if err == nil && cerr != nil {
			err = fmt.Errorf("close %s: %w", path, cerr)
		}
	}()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync %s: %w", path, err)
	}
	return nil
}

// BackupNotice is the one-line notice logged after a key is generated. It is a
// method rather than a constant so the path is always the real one.
func (m *MasterKey) BackupNotice() string {
	return fmt.Sprintf(
		"a new master key was generated at %s — BACK IT UP NOW, separately from the database. "+
			"Lose it and every stored service credential must be re-entered by hand. "+
			"See docs/CONFIGURATION.md §3.5",
		m.Path)
}

// ResolveKEKSalt reads the per-install HKDF salt, creating it on first run.
//
// docs/reference/security.md §1.3 requires a stored per-install salt for KEK
// derivation but does not say where it lives. It goes beside the key: the two
// share a fate, and the schema has no settings table to hold it.
func (c *Config) ResolveKEKSalt() ([]byte, error) {
	path := c.KEKSaltPath()
	// #nosec G304 -- path is derived from USARR_CONFIG_DIR; see writeSecretFile.
	switch raw, err := os.ReadFile(path); {
	case err == nil:
		salt, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if decErr != nil {
			return nil, fmt.Errorf("%s: not valid base64: %w", path, decErr)
		}
		if len(salt) != KEKSaltLen {
			return nil, fmt.Errorf("%s: expected %d bytes, got %d", path, KEKSaltLen, len(salt))
		}
		return salt, nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		return nil, fmt.Errorf("create %s: %w", c.KeysDir(), err)
	}
	salt := make([]byte, KEKSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate KEK salt: %w", err)
	}
	if err := writeSecretFile(path, base64.StdEncoding.EncodeToString(salt)+"\n"); err != nil {
		return nil, err
	}
	return salt, nil
}

// ValidateMasterKey decodes and validates a supplied master key. origin names
// the variable or file the value came from and appears in every error.
//
// Validation order differs from the order the four rules are listed in
// docs/CONFIGURATION.md §3.2: the placeholder check runs first, against the raw
// string, so that a shipped placeholder produces "this is the example value"
// rather than "this is not valid base64". The set of accepted keys is
// identical either way.
func ValidateMasterKey(raw, origin string) ([]byte, error) {
	v := strings.TrimSpace(raw)

	for _, p := range placeholderKeys {
		if subtle.ConstantTimeCompare([]byte(v), []byte(p)) == 1 {
			return nil, fmt.Errorf(
				"%s is the placeholder value shipped in .env.example. A key published in a git "+
					"repository is not a key. Generate one with: openssl rand -base64 32", origin)
		}
	}

	// Padding optional: accept both the padded and raw standard alphabets.
	key, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(v)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: not valid base64 (standard alphabet, padding optional): %w", origin, err)
	}

	if len(key) != MasterKeyLen {
		return nil, fmt.Errorf(
			"%s: decodes to %d bytes, expected exactly %d. UsArr does not pad or truncate a key. "+
				"Generate one with: openssl rand -base64 32", origin, len(key), MasterKeyLen)
	}

	var acc byte
	for _, b := range key {
		acc |= b
	}
	if acc == 0 {
		return nil, fmt.Errorf("%s: decodes to %d zero bytes, which is not a key", origin, MasterKeyLen)
	}

	return key, nil
}

func readKeyFile(path, origin string) ([]byte, error) {
	// #nosec G304 -- path comes from USARR_SECRET_KEY_FILE or USARR_CONFIG_DIR,
	// both operator-supplied level-1 configuration. Pointing UsArr at its own
	// key file is the documented purpose of the variable.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s: %w", origin, path, err)
	}
	return ValidateMasterKey(string(raw), origin+" ("+path+")")
}
