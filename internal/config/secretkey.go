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
//
// "Back up the key file" is correct advice again now that the KEK salt sits
// beside the database rather than in keys/, where the documented
// `--exclude='keys'` was silently dropping it. The salt is carried by an
// ordinary database backup, so the operator has exactly one thing to do here.
func (m *MasterKey) BackupNotice() string {
	return fmt.Sprintf(
		"a new master key was generated at %s — BACK IT UP NOW, separately from the database. "+
			"Lose it and every stored service credential must be re-entered by hand "+
			"(the library, users and history are unaffected). "+
			"See docs/CONFIGURATION.md §3.5",
		m.Path)
}

// ErrKEKSaltAbsent means no KEK salt file exists at either the current path or
// the legacy keys/ one.
//
// Like ErrKeyAbsent, this is NOT yet an error: it is the genuine-first-run
// candidate. The caller must open the database, ask whether it already holds
// encrypted rows, and then either call GenerateKEKSalt (empty database) or fail
// with ErrMissingSaltForExistingData (rows present).
//
// The salt is not secret, but it is exactly as unrecoverable as the master key:
// the KEK is HKDF-SHA256(master key, salt=<this file>, info="usarr/kek/v1"), so
// a different salt is a different KEK and every stored envelope becomes noise.
// Generating a fresh one over existing ciphertext turns a recoverable mistake —
// "restore the file from the backup you already have" — into a permanent one,
// which is the wrong
// default under ARCHITECTURE.md §14's threat model.
var ErrKEKSaltAbsent = errors.New("config: no KEK salt file present")

// errSaltAlreadyExists reports that a salt appeared at the destination while we
// were writing one. It is internal and never reaches an operator: it means
// another process won a race, and the correct response is always to adopt the
// salt on disk rather than to replace it or to fail.
var errSaltAlreadyExists = errors.New("config: a KEK salt already exists at the destination")

// ResolveKEKSalt reads the per-install HKDF salt, COPYING a pre-existing one
// forward from the legacy keys/ location. It never creates one, and it never
// deletes one.
//
// The salt lives beside the database (see KEKSaltPath for why it is not in
// keys/). Installs predating that keep it at LegacyKEKSaltPath, and this
// function carries them forward: reading only the new path would report the salt
// missing on every existing install and destroy exactly the credentials the
// change exists to protect.
//
// # Why this is a copy and not a move — do not "tidy" it into a rename
//
// Nothing stops two UsArr processes starting against the same config directory:
// there is no lock file, and both will happily reach this code at the same
// instant. A MOVE has a window in which neither path holds the salt. Lose the
// process there — a crash, an OOM kill, a container restart mid-upgrade — and
// the key-derivation input is gone for good, which is strictly worse than the
// bug this whole change fixes: unopenable-until-restored becomes unopenable
// forever. A copy has no such window. The legacy file is not a secret, a
// duplicate costs one 45-byte file, and leaving it means anyone whose backup
// captures keys/ still holds a working salt — the old advice degrades to
// redundant instead of wrong.
//
// Concurrency: reads are of complete files only, because every write here lands
// via writeSaltFile's temp-plus-link(2) — NOT temp-plus-rename, and the
// difference is what makes this race safe. rename(2) would replace the
// destination, so a second process could clobber the salt a first one had
// already started sealing credentials under. link(2) refuses when the
// destination exists, so the loser of the race is told it lost and adopts the
// winner's file instead of overwriting it. The function is idempotent.
//
// It refuses rather than regenerating when there is genuinely no salt anywhere;
// see ErrKEKSaltAbsent.
func (c *Config) ResolveKEKSalt() ([]byte, error) {
	path := c.KEKSaltPath()
	salt, err := c.readKEKSalt(path)
	switch {
	case err == nil:
		return salt, nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}

	legacy, err := c.readKEKSalt(c.LegacyKEKSaltPath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, ErrKEKSaltAbsent
	case err != nil:
		return nil, err
	}

	// Copy it forward. The legacy file stays exactly where it is, forever.
	switch err := c.writeSaltFile(path, legacy); {
	case err == nil:
		return legacy, nil
	case errors.Is(err, errSaltAlreadyExists):
		// Another process copied it first. Adopt what is on disk rather than
		// assuming it matches: identical here in practice, but "read the file"
		// is the only answer that stays correct if it ever is not.
		return c.readKEKSalt(path)
	default:
		return nil, fmt.Errorf("copying the KEK salt from %s to %s: %w. "+
			"The salt at %s is untouched and still authoritative — nothing has been lost, "+
			"and UsArr will keep reading it until the copy succeeds",
			c.LegacyKEKSaltPath(), path, err, c.LegacyKEKSaltPath())
	}
}

// writeSaltFile writes a salt atomically: a temp file in the DESTINATION
// directory, fsynced, then link(2) within that same directory.
//
// It is link(2), not rename(2). Both are atomic to a reader, but rename replaces
// the destination unconditionally and link refuses when it exists — see the long
// comment at the call below for why "never clobber" is the property that matters
// on a file whose loss is unrecoverable.
//
// Atomicity is not decoration here either. A reader that catches a salt
// half-written in place sees a short or truncated value; validation would reject
// it and startup would refuse — a false alarm telling an operator their
// credentials are at risk when they are not. link(2) publishes the complete file
// under its real name in one step, so a concurrent reader sees either nothing or
// the whole thing and never a partial. The temp file must be in the same
// directory as the destination, because USARR_CONFIG_DIR and USARR_DATA_DIR may
// be different filesystems and a cross-device link fails with EXDEV.
//
// It returns errSaltAlreadyExists — never an overwrite — when the destination
// appeared underneath it. Callers treat that as "another process won the race"
// and adopt whatever is on disk.
func (c *Config) writeSaltFile(path string, salt []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".kek.salt.tmp-*")
	if err != nil {
		return fmt.Errorf("create a temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// The temp file must never be left behind, on success or failure. On
		// success the link has already given the content its real name and this
		// only drops the extra directory entry.
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(SecretMode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if _, err = tmp.WriteString(base64.StdEncoding.EncodeToString(salt) + "\n"); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("fsync %s: %w", tmpName, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	// Link, NOT rename. rename(2) replaces the destination unconditionally, and
	// a salt that replaces a different salt is unrecoverable data loss — two
	// processes reaching a genuine first run together would each generate one,
	// each overwrite the other, and each go on to seal credentials under a KEK
	// the file on disk can no longer derive. link(2) is equally atomic for a
	// reader and refuses when the destination exists, which turns that race into
	// "one writer wins, the other adopts the winner's salt".
	//
	// This function therefore NEVER replaces an existing salt. Both callers are
	// built on that: the copy-forward has already established the destination
	// was absent, and the generator re-reads on EEXIST.
	if err = os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errSaltAlreadyExists
		}
		// Hard links are unavailable on a few exotic filesystems. Fall back to
		// an O_EXCL create, which keeps the property that actually matters —
		// never clobber — at the cost of a window in which a concurrent reader
		// could see a partial file. That reader fails validation and startup
		// refuses loudly; it never silently derives the wrong KEK.
		if ferr := writeSecretFile(path, base64.StdEncoding.EncodeToString(salt)+"\n"); ferr != nil {
			if errors.Is(ferr, os.ErrExist) {
				err = errSaltAlreadyExists
				return err
			}
			err = fmt.Errorf("link %s to %s: %w; and the O_EXCL fallback failed: %w",
				tmpName, path, err, ferr)
			return err
		}
		err = nil
	}
	_ = os.Remove(tmpName)
	// fsync the directory so the new link itself survives a power cut. Best
	// effort: some filesystems refuse to open a directory for sync, and the
	// directory entry is already durable on every mainstream one.
	//
	// #nosec G304 -- dir is filepath.Dir of a path this package built from
	// USARR_CONFIG_DIR, and it is opened read-only purely to fsync the
	// directory entry. No caller-supplied or network-supplied string reaches it,
	// and nothing is read from the handle.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// readKEKSalt reads and validates one salt file. A missing file is reported as
// os.ErrNotExist for the caller to interpret; every other failure is fatal and
// names the path, because a salt that is present but unreadable must never be
// mistaken for a first run.
func (c *Config) readKEKSalt(path string) ([]byte, error) {
	// #nosec G304 -- path is derived from USARR_CONFIG_DIR; see writeSecretFile.
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	salt, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s: not valid base64: %w", path, err)
	}
	if len(salt) != KEKSaltLen {
		return nil, fmt.Errorf("%s: expected %d bytes, got %d", path, KEKSaltLen, len(salt))
	}
	return salt, nil
}

// ErrMissingSaltForExistingData is the fatal error for the salt branch that
// cannot be decided until the database is open: keys/kek.salt is missing but
// the database already holds encrypted rows.
//
// It names kek.salt specifically, because the operator who hits this has almost
// always just restored a backup and IS holding the master key — telling them to
// restore the master key would send them to look for the one artifact they
// already have.
//
// It also names the legacy path, because an install created before the salt
// moved beside the database keeps it in keys/, and an operator restoring such a
// backup needs to know both places to look.
func (c *Config) ErrMissingSaltForExistingData(encryptedRows int64) error {
	return fmt.Errorf(
		"the database holds %d encrypted credential(s) but the KEK salt file %s is missing: "+
			"the master key alone cannot open them, because the key that wraps every credential is "+
			"derived from the master key AND this per-install salt. Refusing to start: generating a "+
			"fresh salt here would silently and permanently destroy every stored credential. "+
			"Restore %s from a backup of the config volume — it sits beside usarr.db and an ordinary "+
			"backup contains it. On installs created before the salt moved, it is at %s instead; "+
			"put it back at either path and UsArr will pick it up. "+
			"If the salt is genuinely lost, see docs/CONFIGURATION.md §3.5: the database, library, "+
			"users and audit log are unaffected, but every service API key must be re-entered",
		encryptedRows, c.KEKSaltPath(), c.KEKSaltPath(), c.LegacyKEKSaltPath())
}

// GenerateKEKSalt creates the per-install HKDF salt on a genuine first run.
//
// The caller must have established that the database holds no encrypted rows.
// writeSecretFile is O_EXCL, so this can never clobber an existing salt even if
// a caller reaches it by mistake — overwriting one destroys every stored
// credential just as surely as overwriting the master key does.
func (c *Config) GenerateKEKSalt() ([]byte, error) {
	if err := os.MkdirAll(c.ConfigDir, DirMode); err != nil {
		return nil, fmt.Errorf("create %s: %w", c.ConfigDir, err)
	}

	// A salt that appeared since the caller last looked wins. Two processes can
	// start against one config directory — there is no lock — and both can reach
	// a genuine first run at the same instant. Whoever writes first is
	// authoritative; the loser must adopt that salt rather than overwrite it,
	// because a salt swapped out from under a credential sealed microseconds
	// earlier is unopenable forever.
	switch existing, err := c.ResolveKEKSalt(); {
	case err == nil:
		return existing, nil
	case !errors.Is(err, ErrKEKSaltAbsent):
		return nil, err
	}

	salt := make([]byte, KEKSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate KEK salt: %w", err)
	}
	if err := c.writeSaltFile(c.KEKSaltPath(), salt); err != nil && !errors.Is(err, errSaltAlreadyExists) {
		return nil, err
	}

	// Always read back; never trust the salt we just generated. writeSaltFile
	// refuses to replace an existing file, so if another process won the race
	// the file holds ITS salt — and that is the one every credential from here
	// on must be sealed under. This read is what makes two racing first runs
	// converge instead of each sealing under a KEK the other cannot derive.
	final, err := c.readKEKSalt(c.KEKSaltPath())
	if err != nil {
		return nil, fmt.Errorf("re-reading the KEK salt at %s: %w", c.KEKSaltPath(), err)
	}
	return final, nil
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

// ReadNewSecretKey reads and validates keys/secret.key.new, the file that
// exists only while a rotation is in flight.
//
// A missing file is reported as os.ErrNotExist for the caller to interpret:
// startup treats it as "no rotation in flight", and `usarr key rotate` treats it
// as "nothing to resume, generate fresh material". Every other failure is fatal
// and names the path, because a half-rotated install whose new key is present
// but unreadable must never be mistaken for either of those.
func (c *Config) ReadNewSecretKey() ([]byte, error) {
	path := c.NewSecretKeyPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	return readKeyFile(path, path)
}

// GenerateNewSecretKey writes fresh master-key material to keys/secret.key.new
// and returns it.
//
// It goes through writeSecretFile, so it is O_EXCL: it never clobbers an
// existing secret.key.new, because that file is a rotation in flight and
// overwriting it strands every row already re-wrapped under the key it holds.
// The caller resumes from an existing one instead of calling this.
//
// The directory is fsynced as well as the file. A key file whose CONTENT is
// durable but whose directory ENTRY is not is a file that vanishes on a power
// cut, and rotation is precisely the procedure where the crash window matters:
// the whole two-phase design assumes that once this returns, both keys can be
// found again.
func (c *Config) GenerateNewSecretKey() ([]byte, error) {
	if err := os.MkdirAll(c.KeysDir(), SecretDirMode); err != nil {
		return nil, fmt.Errorf("create %s: %w", c.KeysDir(), err)
	}
	if err := os.Chmod(c.KeysDir(), SecretDirMode); err != nil {
		return nil, fmt.Errorf("chmod %s: %w", c.KeysDir(), err)
	}

	key := make([]byte, MasterKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate a new master key: %w", err)
	}
	path := c.NewSecretKeyPath()
	if err := writeSecretFile(path, base64.StdEncoding.EncodeToString(key)+"\n"); err != nil {
		return nil, err
	}
	if err := syncDir(c.KeysDir()); err != nil {
		return nil, err
	}
	return key, nil
}

// PromoteNewSecretKey makes keys/secret.key.new the master key: fsync it,
// rename(2) it over keys/secret.key, fsync the directory.
//
// It is rename(2), and this is the ONE place in this package where replace
// semantics are correct. writeSaltFile deliberately uses link(2) so that it can
// never clobber, because two racing first runs must converge on one salt; here
// the destination is a file the caller has just PROVEN is superseded — every
// stored row has been re-wrapped under the new key and verified — and replacing
// it is the entire operation. Reusing writeSaltFile would refuse, leave
// secret.key.new in place forever, and turn a completed rotation into one that
// can never finish.
//
// rename(2) is atomic: a reader sees either the old key or the new one, never a
// partial file and never neither. That is what leaves the window closed if the
// process dies inside this function.
func (c *Config) PromoteNewSecretKey() error {
	newPath := c.NewSecretKeyPath()
	// Sync the content again before publishing it under the live name. It was
	// synced when written, but a resumed rotation did not write it — this run
	// only read it — and one extra fsync costs nothing next to the failure it
	// rules out.
	// #nosec G304 -- the path is built from USARR_CONFIG_DIR; see writeSecretFile.
	f, err := os.Open(newPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", newPath, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync %s: %w", newPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", newPath, err)
	}

	if err := os.Rename(newPath, c.SecretKeyPath()); err != nil {
		return fmt.Errorf("rename %s to %s: %w", newPath, c.SecretKeyPath(), err)
	}
	return syncDir(c.KeysDir())
}

// syncDir fsyncs a directory so a rename or create is durable, not just the
// bytes inside the file.
//
// Best effort on the error from Sync itself: a few filesystems refuse to sync a
// directory handle at all, and failing a completed rotation over that would be
// worse than the risk it covers. A directory that cannot even be OPENED is a
// different matter and is reported.
func syncDir(dir string) error {
	// #nosec G304 -- dir is built from USARR_CONFIG_DIR and is opened read-only
	// purely to fsync the directory entry. Nothing is read from the handle.
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open %s to fsync it: %w", dir, err)
	}
	_ = d.Sync()
	return d.Close()
}
