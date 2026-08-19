package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jdb3750/UsArr/internal/config"
)

// runBackup is `usarr backup`: the operator-invocable half of the backup story
// docs/CONFIGURATION.md §6 describes.
//
// The automatic pre-migration snapshot in backup.go already did the hard
// mechanical part — VACUUM INTO rather than cp, 0600 in a 0700 directory — but
// it fires only when a migration is pending, which means the answer to "take a
// backup now" was "make a schema change first". This command shares that code
// path exactly, and adds the two things an on-demand backup needs and an
// automatic one does not.
//
// # The first: it copies the KEK salt beside the snapshot
//
// A lone .db file is NOT a restorable backup of a UsArr install, and the way it
// fails is silent. Every stored credential is sealed under
// KEK = HKDF-SHA256(secret.key, salt=kek.salt, info="usarr/kek/v1"), so a
// restore needs three things and the snapshot carries one. The salt is not a
// secret — its value protects nothing, it only has to be per-install and stable
// — so it can travel with the ciphertext, and it must, because it is the input
// nobody remembers to keep. This project has already shipped the other choice
// once: the documented procedure excluded the directory the salt then lived in,
// and a restore with a byte-identical master key decrypted nothing.
//
// # The second: it says what it does not cover, in its own output
//
// The master key is deliberately NOT copied — see ADR-0062 — and a command that
// made that choice quietly would be worse than no command, because it converts
// "I should back up properly" into "I have a backup". So the exclusion is
// printed every run, in the terminal where the operator actually is, naming
// where the key lives on THIS install and what restoring it takes.
func runBackup(ctx context.Context, cfg *config.Config, out io.Writer) error {
	dbPath := cfg.DatabasePath()
	info, err := os.Stat(dbPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("there is nothing to back up: %s does not exist. "+
			"UsArr creates the database on its first start, so run the server once before backing up", dbPath)
	case err != nil:
		return fmt.Errorf("stat %s: %w", dbPath, err)
	case info.Size() == 0:
		return fmt.Errorf("there is nothing to back up: %s exists but is empty. "+
			"A zero-byte file is not a database; run the server once before backing up", dbPath)
	}

	// Same two calls as the pre-migration path, for the same reason: MkdirAll
	// leaves an existing directory's mode alone, and the volume may have been
	// created by hand or by an older build.
	if err := os.MkdirAll(cfg.BackupsDir(), config.DirMode); err != nil {
		return fmt.Errorf("create %s: %w", cfg.BackupsDir(), err)
	}
	if err := os.Chmod(cfg.BackupsDir(), config.DirMode); err != nil {
		return fmt.Errorf("chmod %s: %w", cfg.BackupsDir(), err)
	}

	// One stamp for both files, so the pair is obvious in a directory listing
	// and `ls` sorts them together. The format is the pre-migration path's,
	// which is filename-safe on every filesystem UsArr supports — a literal
	// RFC 3339 string is not, because of the colons.
	stamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	dbTarget := filepath.Join(cfg.BackupsDir(), "usarr-"+stamp+".db")
	saltTarget := filepath.Join(cfg.BackupsDir(), "usarr-"+stamp+".kek.salt")

	if err := vacuumInto(ctx, dbPath, dbTarget); err != nil {
		return err
	}
	saltSource, err := copyKEKSalt(cfg, saltTarget)
	if err != nil {
		// The snapshot is good and staying: deleting it because the salt copy
		// failed would throw away the expensive half over the cheap one. The
		// report below says plainly that the pair is incomplete.
		return fmt.Errorf("copying the KEK salt beside %s: %w", dbTarget, err)
	}

	reportBackup(out, cfg, dbTarget, saltTarget, saltSource)
	return nil
}

// copyKEKSalt copies the per-install HKDF salt next to the snapshot, and reports
// which path it came from. An empty source means no salt exists anywhere.
//
// It reads the legacy keys/kek.salt too. Config.ResolveKEKSalt does that as
// well, and this deliberately does not call it: ResolveKEKSalt COPIES a legacy
// salt forward into the config directory, which is a write, and a backup
// command must not modify the install it is reading. Reading both paths here
// costs three lines and leaves the config directory untouched.
//
// The bytes are never logged, never printed and never returned. They go from
// one file to another and nowhere else — the salt is not a secret, but a backup
// command is exactly the wrong place to start being relaxed about key-derivation
// material.
func copyKEKSalt(cfg *config.Config, target string) (string, error) {
	for _, source := range []string{cfg.KEKSaltPath(), cfg.LegacyKEKSaltPath()} {
		salt, err := os.ReadFile(source) // #nosec G304 -- path built from USARR_CONFIG_DIR, not from a request.
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue
		case err != nil:
			return "", fmt.Errorf("read %s: %w", source, err)
		}
		// WriteFile's mode is masked by the process umask, so the explicit
		// chmod is what actually holds 0600 — the same reason ensureDirs
		// chmods after MkdirAll. The copy is never more permissive than the
		// original, which is 0600 by the same rule.
		// #nosec G703 -- target is filepath.Join(cfg.BackupsDir(), …) with a
		// name runBackup formats from a UTC timestamp. No request input reaches
		// it; the config directory is the operator's own level-1 setting, the
		// same provenance the VACUUM INTO literal in backup.go relies on.
		if err := os.WriteFile(target, salt, config.FileMode); err != nil {
			return "", fmt.Errorf("write %s: %w", target, err)
		}
		if err := os.Chmod(target, config.FileMode); err != nil {
			return "", fmt.Errorf("chmod %s: %w", target, err)
		}
		return source, nil
	}
	return "", nil
}

// reportBackup prints what was written and, at greater length, what was not.
//
// The proportion is deliberate. The files are one line each because the
// operator can see them with ls; the exclusion is the part they cannot see, and
// it is the part that decides whether this backup restores.
func reportBackup(out io.Writer, cfg *config.Config, dbTarget, saltTarget, saltSource string) {
	say(out, "usarr backup\n\n")
	say(out, "Wrote to %s (mode %04o):\n", cfg.BackupsDir(), config.DirMode)
	say(out, "  database  %s  (%s, mode %04o)\n", dbTarget, fileSize(dbTarget), config.FileMode)
	if saltSource != "" {
		say(out, "  kek salt  %s  (%s, mode %04o)\n", saltTarget, fileSize(saltTarget), config.FileMode)
		if saltSource == cfg.LegacyKEKSaltPath() {
			say(out, "            copied from the legacy location %s\n", saltSource)
		}
	} else {
		say(out, "  kek salt  NOT WRITTEN — no salt file exists at %s or %s.\n",
			cfg.KEKSaltPath(), cfg.LegacyKEKSaltPath())
		say(out, "            UsArr creates one the first time it seals a credential, so an\n")
		say(out, "            install that has never stored one has nothing to lose here. If\n")
		say(out, "            this install HAS stored service credentials, that salt is missing\n")
		say(out, "            and they cannot be recovered from this snapshot.\n")
	}
	say(out, "\nSafe to run while UsArr is running: VACUUM INTO writes a consistent snapshot of\n")
	say(out, "the committed database, WAL contents included. `cp usarr.db` does not.\n")

	say(out, "\nNOT IN THIS BACKUP, and not an oversight:\n\n")
	say(out, "  the master key — %s\n\n", masterKeyLocation(cfg))
	say(out, "  Every service credential in the snapshot is sealed under a key derived from\n")
	say(out, "  that key and the salt above. Restore without it and the database still opens:\n")
	say(out, "  the library, the users and the audit log are intact, and every stored *Arr\n")
	say(out, "  credential is noise that can only be re-entered by hand.\n\n")
	say(out, "  It is left out because one file holding both the ciphertext and the key that\n")
	say(out, "  opens it is a single-file compromise — the property encryption at rest exists\n")
	say(out, "  to prevent. Restoring it is a separate, manual step (ADR-0062), and this is\n")
	say(out, "  the whole of it:\n\n")
	for _, line := range masterKeyRestoreSteps(cfg) {
		say(out, "    %s\n", line)
	}
	if _, err := os.Stat(cfg.NewSecretKeyPath()); err == nil {
		say(out, "\n  A key rotation is unfinished: %s exists. Rows in this snapshot may be\n", cfg.NewSecretKeyPath())
		say(out, "  sealed under EITHER key, so keep BOTH files — restoring only one of them\n")
		say(out, "  leaves part of the credentials unreadable.\n")
	}

	say(out, "\nAlso not covered: providers/*.yaml (hand-written service manifests, if you use\n")
	say(out, "them) and $USARR_DATA_DIR (caches and logs, rebuilt automatically).\n")

	say(out, "\nTo restore (docs/CONFIGURATION.md §6.3): stop UsArr, move the damaged database\n")
	say(out, "aside, copy the snapshot to %s", cfg.DatabasePath())
	if saltSource != "" {
		say(out, "\nand the salt to %s", cfg.KEKSaltPath())
	}
	say(out, ", then install the master key\nas above. The first two restore the library; the third is what makes the stored\ncredentials readable again.\n")
}

// masterKeyLocation names where THIS install's master key comes from, without
// reading it. The channel is a level-1 setting, so the answer is already in the
// resolved config and no file has to be opened to give it.
func masterKeyLocation(cfg *config.Config) string {
	switch {
	case cfg.SecretKeySet:
		return "supplied through USARR_SECRET_KEY, so UsArr does not store it on this host at all"
	case cfg.SecretKeyFile != "" && cfg.SecretKeyFileFromFlag:
		// Name the setting the value actually came from, for the reason
		// keyrotate.go names it: the flag and the variable resolve into one
		// field, and telling an operator to go and change the one they never
		// set is how a correct instruction becomes an unfollowable one.
		return "read from " + cfg.SecretKeyFile + " (--secret-key-file)"
	case cfg.SecretKeyFile != "":
		return "read from " + cfg.SecretKeyFile + " (USARR_SECRET_KEY_FILE)"
	default:
		return cfg.SecretKeyPath()
	}
}

// masterKeyRestoreSteps is the actual sequence for this install's key channel —
// not the gesture "back it up separately". Which channel it is changes what the
// operator does, and getting the wrong one is how a restore stalls.
func masterKeyRestoreSteps(cfg *config.Config) []string {
	switch {
	case cfg.SecretKeySet:
		return []string{
			"The value of USARR_SECRET_KEY is the whole key. It lives in your compose",
			"file, systemd unit or secrets store — not in this config directory, so",
			"there is nothing here to recover it from later.",
			"On restore: set USARR_SECRET_KEY to the SAME value before starting UsArr.",
		}
	case cfg.SecretKeyFile != "":
		return []string{
			"Back up " + cfg.SecretKeyFile + " to your password manager or secrets",
			"store — somewhere that is not this backup and not this host.",
			"On restore: put that file back at the path USARR_SECRET_KEY_FILE or",
			"--secret-key-file points at, mode 0600, before starting UsArr.",
		}
	default:
		return []string{
			"cat " + cfg.SecretKeyPath(),
			"→ store that value in your password manager or secrets store, once. Do",
			"  not put it in this backups directory; that would undo the split.",
			"On restore: write it back to " + cfg.SecretKeyPath() + " as mode 0600,",
			"inside a 0700 " + cfg.KeysDir() + ", before starting UsArr.",
		}
	}
}

// fileSize renders a size for the report. A file that cannot be stat'd right
// after being written is not worth failing the whole backup over — the path is
// printed either way and the operator can look — so this degrades to a word.
func fileSize(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "size unknown"
	}
	n := info.Size()
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
