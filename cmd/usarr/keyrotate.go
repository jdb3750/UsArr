package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/store"
)

// rotateExtraPasses bounds how many times the re-wrap pass is repeated because
// rows appeared at another key id while it ran.
//
// Rows CAN appear underneath a rotation: there is no single-instance lock
// (REVIEW-LOG.md FI-06), so a server may be running against the same config
// directory and re-sealing a credential under ITS primary — the old key —
// between this pass and the count that follows it. One extra pass catches the
// realistic case; an unbounded retry against a busy server would spin forever,
// so after this many the command stops and says the true remedy out loud, which
// is to stop the server. It never promotes on an unfinished re-wrap.
const rotateExtraPasses = 3

// say writes one line of the command's progress report.
//
// The write error is discarded for printVersion's reason: the only real caller
// writes to stdout, and a closed stdout is the caller's problem to notice.
// Aborting a rotation halfway because a pipe closed would be strictly worse
// than finishing it and saying nothing — the key files and the database are
// what matter here, not the transcript.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// runKeyRotate is `usarr key rotate`: the two-phase, resumable master-key
// rotation from docs/CONFIGURATION.md §3.4 and docs/reference/security.md §1.5.
//
// The sequence fails closed at every step, and the ORDER is the safety
// property, not an implementation detail:
//
//	refuse if the key is not ours to manage
//	→ ordinary startup, so the salt ladder refuses exactly as it always does
//	→ resume from secret.key.new, or generate fresh material through the
//	  never-clobbering O_EXCL writer
//	→ register both keys, primary on the new one
//	→ re-wrap in keyset-paginated batches, one bounded transaction each
//	→ prove EVERY row now names the new id and unwraps under it
//	→ only then promote the key file and drop the superseded key
//
// Nothing before the promote step touches a key file that an existing row
// depends on, so every intermediate state is one where both keys are on disk
// and every row is openable.
func runKeyRotate(ctx context.Context, cfg *config.Config, log *slog.Logger, build httpapi.BuildInfo, out io.Writer) error {
	// It can only manage keys/secret.key. A key supplied through the
	// environment lives somewhere UsArr does not own — a Docker secret, a
	// systemd credential, a password manager — and "generate a replacement and
	// rename it into place" has no meaning there. Rotating anyway would re-wrap
	// every row under material the next start cannot find.
	switch {
	case cfg.SecretKeySet:
		return fmt.Errorf("USARR_SECRET_KEY is set, so the master key does not live in a file UsArr owns "+
			"and `key rotate` cannot manage it. It can only rotate %s. "+
			"To rotate an environment-supplied key: unset USARR_SECRET_KEY and let the key file take over, "+
			"or replace the value yourself and follow docs/CONFIGURATION.md §3.5", cfg.SecretKeyPath())
	case cfg.SecretKeyFile != "":
		// NAME THE SETTING THE VALUE ACTUALLY CAME FROM. The path has a flag
		// twin and an environment twin that resolve into one field, and this
		// message used to hardcode the variable — so an operator who passed
		// --secret-key-file was told to go and unset a variable they had never
		// set, while holding an install that would not rotate. The remedy is
		// the same either way; the thing to change is not.
		source := "USARR_SECRET_KEY_FILE is set"
		if cfg.SecretKeyFileFromFlag {
			source = "--secret-key-file was passed"
		}
		return fmt.Errorf("%s (%s), so the master key lives in a file UsArr does not "+
			"own and `key rotate` cannot manage it. It can only rotate %s. "+
			"To rotate a file supplied that way, replace its contents yourself and follow "+
			"docs/CONFIGURATION.md §3.5", source, cfg.SecretKeyFile, cfg.SecretKeyPath())
	}

	// An empty config directory has no key to rotate, and buildApp below would
	// happily make one: its ladder GENERATES a master key when the file is
	// absent and nothing is sealed, which is exactly right for a first server
	// start and exactly wrong here. Left alone, `usarr key rotate` on a fresh
	// directory printed "a new master key was generated — BACK IT UP NOW" and
	// then rotated that key away in the same run, so the notice named material
	// that was already superseded by the time the operator read it.
	//
	// os.Stat rather than ResolveMasterKey: the only question is whether the
	// file this command manages exists at all. A file that exists but holds a
	// bad value is buildApp's error to report, in buildApp's words.
	if _, err := os.Stat(cfg.SecretKeyPath()); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("there is no master key to rotate: %s does not exist. "+
			"UsArr generates one on its first start, so run the server once before rotating",
			cfg.SecretKeyPath())
	}

	// The ordinary startup sequence, deliberately: it applies migrations and it
	// runs the same fail-closed ladder a server start runs, so a missing
	// kek.salt with sealed rows present refuses HERE too, with the same error.
	// A rotation that invented its own lighter startup would be a second copy
	// of that ladder and would drift from it.
	a, err := buildApp(ctx, cfg, log, build)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := a.Close(); cerr != nil {
			log.Error("key rotate: database did not close cleanly", "err", cerr)
		}
	}()

	say(out, "usarr key rotate\n")
	say(out, "  config dir:   %s\n", cfg.ConfigDir)
	say(out, "  key file:     %s\n", cfg.SecretKeyPath())
	say(out, "  new key file: %s\n\n", cfg.NewSecretKeyPath())

	oldID := a.keyring.PrimaryID()
	oldKEK, err := crypto.DeriveKEK(a.masterKey.Key, a.kekSalt)
	if err != nil {
		return err
	}

	newID, resumed, err := prepareRotation(cfg, a.keyring, a.kekSalt, oldKEK, out)
	if err != nil {
		return err
	}
	if err := auditRotation(ctx, a.store, store.AuditActionKeyRotatePrepare, map[string]any{
		"old_kek_id": oldID, "new_kek_id": newID, "resumed": resumed,
	}); err != nil {
		return err
	}

	moved, tombstoned, err := rewrapEverything(ctx, a.store, a.keyring, newID, out)
	if err != nil {
		return err
	}
	if err := auditRotation(ctx, a.store, store.AuditActionKeyRotateRewrap, map[string]any{
		"old_kek_id": oldID, "new_kek_id": newID,
		"rewrapped": moved, "rewrapped_tombstoned": tombstoned,
	}); err != nil {
		return err
	}

	verified, err := verifyEverything(ctx, a.store, a.keyring, newID, out)
	if err != nil {
		return err
	}

	dropped, err := promoteRotation(cfg, a.keyring, newID, out)
	if err != nil {
		return err
	}

	if err := auditRotation(ctx, a.store, store.AuditActionKeyRotatePromote, map[string]any{
		"old_kek_id": oldID, "new_kek_id": newID,
		"verified": verified, "dropped_kek_ids": dropped,
	}); err != nil {
		return err
	}

	// The last window, and the one the verify pass cannot cover: a row sealed
	// under the old key AFTER verifyEverything read the table. See the
	// post-promote note in strandedAfterPromote.
	//
	// It runs after the promote audit row rather than before it, and the row
	// deliberately does not carry the number. That row records what PROMOTION
	// did — the rename, and which ids left the ring — and all of that was true
	// the moment promoteRotation returned. What this count finds is a different
	// actor's writes, observed afterwards; folding it into the promote row would
	// make one entry answer two questions. The durable record of a stranded row
	// is the row itself: service_instance.kek_id names an id no key derives, and
	// startup reports it on every start (warnStrandedCredentials).
	stranded, err := a.store.ListCredentialsOutsideKEKIDsIncludingDeleted(ctx, []uint32{newID})
	if err != nil {
		return err
	}
	if len(stranded) > 0 {
		return strandedAfterPromote(cfg, newID, stranded, out)
	}

	say(out, "\nrotation complete. Back up %s now — the previous key no longer opens anything.\n",
		cfg.SecretKeyPath())
	return nil
}

// strandedAfterPromote reports credentials that appeared at another key id
// after the verify pass had already read the table, and returns the non-zero
// exit that goes with them.
//
// WHY THIS RUNS AFTER PROMOTION RATHER THAN BLOCKING IT. verifyEverything closes
// the window for every row that existed when it ran, but there is no
// single-instance lock (REVIEW-LOG.md FI-06), so a server still running against
// this config directory can seal a credential under ITS primary — the old key —
// between the verify pass and the rename, or at any point after it. Those rows
// name a key id that is no longer on disk once the old file is replaced.
//
// Refusing before promotion would be the wrong trade and is deliberately not
// what happens: at this point the rotation itself is sound and complete, every
// pre-existing row is re-wrapped and verified, and the key file is the one the
// operator must now back up. Rolling that back to protect a row written by a
// process that was told to be stopped would put the whole install back on a key
// the operator rotated because it was compromised.
//
// So the rotation stands and the command fails LOUDLY instead: the affected
// instances are named, because "n credentials are unopenable" is not something
// an operator can act on, and the exit is non-zero, because a rotation that
// stranded a credential is not a success and must not look like one to a
// script.
func strandedAfterPromote(cfg *config.Config, newID uint32, stranded []store.StrandedCredential, out io.Writer) error {
	say(out, "\n!! the rotation completed, but %d credential(s) were written under another key "+
		"WHILE it ran\n", len(stranded))
	for _, c := range stranded {
		label := ""
		if c.Deleted {
			label = " (deleted)"
		}
		say(out, "   service_instance %d %q%s is at kek id %d, not %d\n", c.ID, c.Name, label, c.KEKID, newID)
	}
	return fmt.Errorf("%d credential(s) were sealed under a superseded key while the rotation ran and "+
		"cannot be opened by %s — a UsArr server was running against %s. "+
		"The rotation itself is complete and every other credential survived it: back up %s. "+
		"Then stop the server, start it again, and re-enter the API key for the instance(s) listed above",
		len(stranded), cfg.SecretKeyPath(), cfg.ConfigDir, cfg.SecretKeyPath())
}

// prepareRotation establishes the new key material and registers it, returning
// its id and whether it came from an interrupted rotation.
//
// Resuming is preferred over generating, always. An existing secret.key.new is
// a rotation in flight, and rows are already wrapped under the key it holds;
// generating over it would strand exactly those rows. The O_EXCL writer would
// refuse anyway — this checks first so the operator gets "resuming" rather than
// a file-exists error.
func prepareRotation(
	cfg *config.Config, keyring *crypto.Keyring, salt, oldKEK []byte, out io.Writer,
) (uint32, bool, error) {
	var newMaster []byte
	resumed := true
	newMaster, err := cfg.ReadNewSecretKey()
	if errors.Is(err, os.ErrNotExist) {
		resumed = false
		newMaster, err = cfg.GenerateNewSecretKey()
	}
	if err != nil {
		return 0, false, err
	}

	newKEK, err := crypto.DeriveKEK(newMaster, salt)
	if err != nil {
		return 0, false, err
	}
	if subtle.ConstantTimeCompare(newKEK, oldKEK) == 1 {
		// Same material under a different name. Almost always an operator who
		// copied secret.key to secret.key.new by hand. Promoting it would report
		// a completed rotation that changed nothing, which is worse than
		// refusing: the reason to rotate is that the old key is compromised.
		return 0, false, fmt.Errorf("%s holds the same key as %s, so rotating it would change nothing. "+
			"Remove %s and re-run to generate fresh material",
			cfg.NewSecretKeyPath(), cfg.SecretKeyPath(), cfg.NewSecretKeyPath())
	}

	newID := crypto.KeyID(newKEK)
	if err := keyring.Add(newID, newKEK); err != nil {
		// The 2^-32 collision: the new key's content-derived id is one the ring
		// already holds with different bytes. Fresh material fixes it, and the
		// freshly written file is removed so the operator is not left with a
		// phantom rotation in flight. A RESUMED file is never removed — it may
		// have rows behind it.
		if !resumed {
			_ = os.Remove(cfg.NewSecretKeyPath())
			return 0, false, fmt.Errorf("the new key's id collides with a key already in the keyring "+
				"(1 chance in 2^32); the generated file was discarded — re-run the command: %w", err)
		}
		return 0, false, fmt.Errorf("the pending key in %s has an id that collides with a key already in "+
			"the keyring: %w", cfg.NewSecretKeyPath(), err)
	}
	if err := keyring.SetPrimary(newID); err != nil {
		return 0, false, err
	}

	if resumed {
		say(out, "prepare: resuming the rotation already in progress (%s)\n", cfg.NewSecretKeyPath())
	} else {
		say(out, "prepare: new key material written to %s\n", cfg.NewSecretKeyPath())
	}
	say(out, "         active kek ids %v, primary %d — every row is openable from here on\n",
		keyring.IDs(), keyring.PrimaryID())
	return newID, resumed, nil
}

// rewrapEverything re-wraps every stored envelope under newID and returns how
// many rows it moved, and how many of those were tombstones.
//
// It repeats until the "not at the new id" count is zero, because rows can
// appear underneath it — see rotateExtraPasses.
func rewrapEverything(
	ctx context.Context, st *store.Store, keyring *crypto.Keyring, newID uint32, out io.Writer,
) (moved, tombstoned int64, err error) {
	for pass := 0; ; pass++ {
		m, t, err := rewrapOnePass(ctx, st, keyring, newID)
		moved += m
		tombstoned += t
		if err != nil {
			return moved, tombstoned, err
		}
		remaining, err := st.CountCredentialsOutsideKEKIDIncludingDeleted(ctx, newID)
		if err != nil {
			return moved, tombstoned, err
		}
		if remaining == 0 {
			say(out, "rewrap:  %d credential(s) re-wrapped under kek id %d (%d tombstoned), "+
				"0 remaining at any other id\n", moved, newID, tombstoned)
			return moved, tombstoned, nil
		}
		if pass >= rotateExtraPasses {
			// Fail closed, and say the real remedy. Promoting here would leave
			// those rows wrapped under a key file that is about to be replaced.
			return moved, tombstoned, fmt.Errorf(
				"%d credential(s) are still wrapped under another key id after %d passes: "+
					"something is writing credentials while the rotation runs. "+
					"Nothing has been promoted and every row is still readable — %s is unchanged. "+
					"Stop the UsArr server and re-run `usarr key rotate`; it resumes where it left off",
				remaining, pass+1, "keys/secret.key")
		}
	}
}

// rewrapOnePass walks the table once, keyset-paginated, re-wrapping in bounded
// batches of one transaction each.
//
// Keyset rather than OFFSET, for the same reason provenance_redact.go uses one:
// the pass updates rows as it goes, and OFFSET over a table being written under
// you skips rows.
func rewrapOnePass(
	ctx context.Context, st *store.Store, keyring *crypto.Keyring, newID uint32,
) (moved, tombstoned int64, err error) {
	var afterID int64
	for {
		page, err := st.ListCredentialEnvelopesIncludingDeleted(ctx, afterID, 0)
		if err != nil {
			return moved, tombstoned, err
		}
		if len(page) == 0 {
			return moved, tombstoned, nil
		}
		afterID = page[len(page)-1].ID

		var batch []store.CredentialRewrite
		var batchTombstones int64
		for _, c := range page {
			if c.KEKID == newID {
				continue
			}
			envelope, err := keyring.Rewrap(c.Envelope, newID)
			if err != nil {
				// Fail closed on the first row that will not re-wrap. Skipping
				// it would let the rotation reach promote with a row nothing on
				// disk can open afterwards.
				return moved, tombstoned, fmt.Errorf(
					"service_instance %d (%q) could not be re-wrapped, so nothing has been promoted "+
						"and every key file is unchanged: %w", c.ID, c.Name, err)
			}
			batch = append(batch, store.CredentialRewrite{ID: c.ID, Envelope: envelope, KEKID: newID})
			if c.Deleted {
				batchTombstones++
			}
		}
		if err := st.RewrapCredentialsIncludingDeleted(ctx, batch); err != nil {
			return moved, tombstoned, err
		}
		moved += int64(len(batch))
		tombstoned += batchTombstones
	}
}

// verifyEverything re-reads every row and proves the new material opens it,
// BEFORE any key file is touched.
//
// It checks three things per row, and each one catches a different way the
// re-wrap could have lied:
//
//   - the plain kek_id column names the new id — the loop's own termination
//     condition, re-asserted against the rows rather than against a count;
//   - the id INSIDE the envelope agrees with the column, which is the
//     consistency ServiceInstance.KEKID's contract asserts and nothing else
//     verifies;
//   - the wrapped DEK unwraps under that id (crypto.Verify), which is the only
//     evidence that the key about to become authoritative actually works.
//
// Verify rather than Open, deliberately: Open needs the AAD and fails on a row
// whose base_url was edited without the credential being re-entered
// (security.md §1.6). That row was already unopenable before the rotation
// started and refusing to promote because of it would make an unrelated,
// pre-existing condition permanently block key rotation.
func verifyEverything(
	ctx context.Context, st *store.Store, keyring *crypto.Keyring, newID uint32, out io.Writer,
) (int64, error) {
	var verified int64
	var afterID int64
	for {
		page, err := st.ListCredentialEnvelopesIncludingDeleted(ctx, afterID, 0)
		if err != nil {
			return verified, err
		}
		if len(page) == 0 {
			say(out, "verify:  %d credential(s) unwrap under kek id %d\n", verified, newID)
			return verified, nil
		}
		afterID = page[len(page)-1].ID

		for _, c := range page {
			const refused = "refusing to promote: nothing has been promoted and every key file is unchanged"
			if c.KEKID != newID {
				return verified, fmt.Errorf("service_instance %d (%q) is still at kek id %d, not %d — %s",
					c.ID, c.Name, c.KEKID, newID, refused)
			}
			inEnvelope, err := crypto.KEKID(c.Envelope)
			if err != nil {
				return verified, fmt.Errorf("service_instance %d (%q): %w — %s", c.ID, c.Name, err, refused)
			}
			if inEnvelope != c.KEKID {
				return verified, fmt.Errorf(
					"service_instance %d (%q): the kek_id column says %d but the envelope says %d — %s",
					c.ID, c.Name, c.KEKID, inEnvelope, refused)
			}
			if err := keyring.Verify(c.Envelope); err != nil {
				return verified, fmt.Errorf("service_instance %d (%q): %w — %s", c.ID, c.Name, err, refused)
			}
			verified++
		}
	}
}

// promoteRotation publishes the new key file and drops the superseded keys,
// returning the ids it dropped.
//
// This is the only step that touches a key file an existing row depends on, and
// it runs only after verifyEverything has proven no row depends on the old one.
func promoteRotation(cfg *config.Config, keyring *crypto.Keyring, newID uint32, out io.Writer) ([]uint32, error) {
	if err := cfg.PromoteNewSecretKey(); err != nil {
		return nil, err
	}
	say(out, "promote: %s is now %s\n", cfg.NewSecretKeyPath(), cfg.SecretKeyPath())

	var dropped []uint32
	for _, id := range keyring.IDs() {
		if id == newID {
			continue
		}
		if err := keyring.Drop(id); err != nil {
			return dropped, err
		}
		dropped = append(dropped, id)
	}
	say(out, "         superseded kek ids dropped: %v; active: %v\n", dropped, keyring.IDs())
	return dropped, nil
}

// auditRotation appends one audit row per phase, per CONFIGURATION.md §3.4
// step 5.
//
// actor_user_id is store.SystemUserID, the shared/system sentinel, because a
// command-line rotation has no acting user and 0 is exactly what "written by a
// path with no acting user" means — see Scope.userPredicate, whose canonical
// filter is `actor_user_id IN (0, :uid)`. NULL would have been the other
// candidate and is wrong for a practical reason: NULL matches no scope, so the
// rows would be invisible to every audit read in the codebase, which is a
// strange fate for the only record a key rotation leaves. The column carries no
// foreign key, so the sentinel needs no user row to exist.
//
// The metadata holds COUNTS AND KEY IDS ONLY. A key id is already public — it
// is in every envelope header and in service_instance.kek_id — and no key
// material, path or credential value goes anywhere near this row.
func auditRotation(ctx context.Context, st *store.Store, action string, metadata map[string]any) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("audit %s: encode metadata: %w", action, err)
	}
	_, err = st.AppendAudit(ctx, store.AuditEntry{
		ActorUserID:  sql.NullInt64{Int64: store.SystemUserID, Valid: true},
		Action:       action,
		TargetType:   "master_key",
		Result:       store.AuditResultOK,
		MetadataJSON: string(raw),
	})
	return err
}
