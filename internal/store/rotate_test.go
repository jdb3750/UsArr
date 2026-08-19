package store

import (
	"testing"

	"github.com/jdb3750/UsArr/internal/crypto"
)

// sealInstance creates one service instance with a real sealed credential and
// returns its id.
func sealInstance(t *testing.T, s *Store, kr *crypto.Keyring, name, baseURL string) int64 {
	t.Helper()
	id, err := s.CreateServiceInstanceSealed(t.Context(), ServiceInstance{
		Kind: "radarr", Name: name, BaseURL: baseURL, Enabled: true, VerifyTLS: true,
	}, func(id int64) ([]byte, uint32, error) {
		aad, err := crypto.ServiceInstanceAAD(id, baseURL)
		if err != nil {
			return nil, 0, err
		}
		env, err := kr.Seal([]byte(fixtureAPIKey), aad)
		return env, kr.PrimaryID(), err
	})
	if err != nil {
		t.Fatalf("seal %s: %v", name, err)
	}
	return id
}

// TestRotationReachesSoftDeletedRows is the regression for a defect the
// rotation helpers were written to avoid, recorded as RK-01 in
// docs/REVIEW-LOG.md.
//
// A tombstoned service_instance still holds its ciphertext — that is what
// "the id stays burned" means — and CountEncryptedCredentials already counts
// tombstones. But every credential UPDATE in serviceinstance.go carries
// `AND deleted_at IS NULL`, because every one of them is a user action on a row
// the user can see. A rotation built on those helpers would report zero rows
// remaining at the old key id while a tombstone sat there still wrapped under a
// key whose file had just been replaced: permanently unopenable ciphertext,
// produced by the one procedure whose entire purpose is not to produce any.
func TestRotationReachesSoftDeletedRows(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	kr := newTestKeyring(t)
	oldID := kr.PrimaryID()

	live := sealInstance(t, s, kr, "Radarr", "http://radarr.lan:7878")
	tombstoned := sealInstance(t, s, kr, "Old Sonarr", "http://sonarr.lan:8989")
	if err := s.SoftDeleteServiceInstance(ctx, tombstoned, testNow); err != nil {
		t.Fatalf("SoftDeleteServiceInstance: %v", err)
	}

	// The visible-row helper cannot even see it. This is the assertion that
	// makes the rest of the test mean something: it pins the gap rather than
	// assuming it.
	if err := s.UpdateServiceInstanceCredential(ctx, tombstoned, []byte("unused"), 99); err == nil {
		t.Fatal("UpdateServiceInstanceCredential touched a tombstoned row; " +
			"this test's whole premise is that it does not")
	}

	// Register the new key and re-wrap everything the rotation can see.
	newKEK, err := crypto.DeriveKEK([]byte("new master key, 32 bytes exactly"), []byte("salt, also 32 bytes exactly ...."))
	if err != nil {
		t.Fatal(err)
	}
	newID := crypto.KeyID(newKEK)
	if err := kr.Add(newID, newKEK); err != nil {
		t.Fatal(err)
	}

	page, err := s.ListCredentialEnvelopesIncludingDeleted(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListCredentialEnvelopesIncludingDeleted: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("the rotation read saw %d rows, want both the live one and the tombstone", len(page))
	}
	var sawLive, sawTombstone bool
	var batch []CredentialRewrite
	for _, c := range page {
		switch c.ID {
		case live:
			sawLive = true
			if c.Deleted {
				t.Error("the live row is reported as deleted")
			}
		case tombstoned:
			sawTombstone = true
			if !c.Deleted {
				t.Error("the tombstoned row is not reported as deleted")
			}
		}
		env, err := kr.Rewrap(c.Envelope, newID)
		if err != nil {
			t.Fatalf("Rewrap %d: %v", c.ID, err)
		}
		batch = append(batch, CredentialRewrite{ID: c.ID, Envelope: env, KEKID: newID})
	}
	if !sawLive || !sawTombstone {
		t.Fatalf("the rotation read returned live=%v tombstoned=%v; it must return both", sawLive, sawTombstone)
	}
	if err := s.RewrapCredentialsIncludingDeleted(ctx, batch); err != nil {
		t.Fatalf("RewrapCredentialsIncludingDeleted: %v", err)
	}

	// The termination condition must now be genuinely satisfied — including the
	// tombstone. Before these helpers existed it would have read zero here with
	// the tombstone still at the old id.
	remaining, err := s.CountCredentialsOutsideKEKIDIncludingDeleted(ctx, newID)
	if err != nil {
		t.Fatalf("CountCredentialsOutsideKEKIDIncludingDeleted: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("%d row(s) still at another kek id after the rotation pass", remaining)
	}
	if n, err := s.CountCredentialsOutsideKEKIDIncludingDeleted(ctx, oldID); err != nil || n != 2 {
		t.Fatalf("count outside the OLD id = %d (err %v), want both rows moved", n, err)
	}

	// And the tombstone's credential really opens under the new key alone,
	// which is what "the rotation reached it" has to mean.
	if err := kr.SetPrimary(newID); err != nil {
		t.Fatal(err)
	}
	if err := kr.Drop(oldID); err != nil {
		t.Fatal(err)
	}
	after, err := s.ListCredentialEnvelopesIncludingDeleted(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range after {
		if c.KEKID != newID {
			t.Errorf("service_instance %d is still at kek id %d", c.ID, c.KEKID)
		}
		if err := kr.Verify(c.Envelope); err != nil {
			t.Errorf("service_instance %d does not unwrap under the new key alone: %v", c.ID, err)
		}
		aad, err := crypto.ServiceInstanceAAD(c.ID, c.BaseURL)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := kr.Open(c.Envelope, aad)
		if err != nil {
			t.Fatalf("service_instance %d: Open after rotation: %v", c.ID, err)
		}
		if string(plain) != fixtureAPIKey {
			t.Errorf("service_instance %d decrypted to the wrong value", c.ID)
		}
	}
}

// TestRotationReadSkipsNonEnvelopes: the placeholder CreateServiceInstanceSealed
// writes between its insert and its seal is a single byte, and a value crypto
// could only ever reject is not a credential to rotate. The read and the count
// must agree about that, or the rotation loop never terminates.
func TestRotationReadSkipsNonEnvelopes(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	id, err := s.CreateServiceInstance(ctx, ServiceInstance{
		Kind: "radarr", Name: "No credential", BaseURL: "http://radarr.lan:7878",
		APIKeyEnc: []byte{0}, KEKID: 0,
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}

	page, err := s.ListCredentialEnvelopesIncludingDeleted(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 0 {
		t.Fatalf("the rotation read returned %d row(s) for a placeholder blob", len(page))
	}
	// Any id at all: with nothing rotatable in the table, the remaining-work
	// count must be zero or the loop spins forever on a row it cannot rotate.
	if n, err := s.CountCredentialsOutsideKEKIDIncludingDeleted(ctx, 12345); err != nil || n != 0 {
		t.Fatalf("count = %d (err %v), want 0: the read and the count disagree about what a credential is", n, err)
	}
	// The row itself is still there; it is the CREDENTIAL that is absent.
	if _, err := s.GetServiceInstance(ctx, OwnerScope(1), id); err != nil {
		t.Fatalf("the placeholder row was skipped by rotation but must still exist: %v", err)
	}
}

// TestStrandedListSeesTombstonesAndSkipsHeldKeys drills
// ListCredentialsOutsideKEKIDsIncludingDeleted, which is what both the
// post-promote report and the startup diagnostic read.
//
// The tombstone is the case that matters and is the reason this function is in
// the deleted_at-ignoring group with the other three: a soft-deleted row keeps
// its ciphertext forever, so it can be stranded exactly like a live one, and a
// version of this built on the visible-row helpers would report an install as
// healthy while a tombstone sat under a key nothing holds. See REVIEW-LOG.md
// RK-01 for why that grouping exists and RK-02 for what reads it.
func TestStrandedListSeesTombstonesAndSkipsHeldKeys(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	kr := newTestKeyring(t)
	heldID := kr.PrimaryID()

	held := sealInstance(t, s, kr, "Radarr", "http://radarr.lan:7878")
	strandedLive := sealInstance(t, s, kr, "Sonarr", "http://sonarr.lan:8989")
	strandedDead := sealInstance(t, s, kr, "Old Lidarr", "http://lidarr.lan:8686")
	if err := s.SoftDeleteServiceInstance(ctx, strandedDead, testNow); err != nil {
		t.Fatalf("SoftDeleteServiceInstance: %v", err)
	}

	// Move two rows to an id the ring does not hold. The envelope is left alone
	// on purpose: this function reads the kek_id column, which is what makes it
	// usable at startup before anything has been opened.
	const goneID uint32 = 424242
	for _, id := range []int64{strandedLive, strandedDead} {
		env := envelopeFor(t, s, id)
		if err := s.RewrapCredentialsIncludingDeleted(ctx,
			[]CredentialRewrite{{ID: id, Envelope: env, KEKID: goneID}}); err != nil {
			t.Fatalf("move service_instance %d: %v", id, err)
		}
	}

	got, err := s.ListCredentialsOutsideKEKIDsIncludingDeleted(ctx, []uint32{heldID})
	if err != nil {
		t.Fatalf("ListCredentialsOutsideKEKIDsIncludingDeleted: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d stranded row(s), want the live one and the tombstone: %+v", len(got), got)
	}
	seen := map[int64]StrandedCredential{}
	for _, c := range got {
		seen[c.ID] = c
	}
	if _, ok := seen[held]; ok {
		t.Error("a row at a held key id was reported as stranded")
	}
	if c, ok := seen[strandedLive]; !ok || c.Deleted || c.Name != "Sonarr" || c.KEKID != goneID {
		t.Errorf("the live stranded row is wrong or missing: %+v (present=%v)", c, ok)
	}
	if c, ok := seen[strandedDead]; !ok || !c.Deleted || c.Name != "Old Lidarr" {
		t.Errorf("the TOMBSTONED stranded row is wrong or missing: %+v (present=%v)", c, ok)
	}

	// Holding both ids leaves nothing stranded, which is the healthy install.
	rest, err := s.ListCredentialsOutsideKEKIDsIncludingDeleted(ctx, []uint32{heldID, goneID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 0 {
		t.Errorf("rows reported stranded while every id they name is held: %+v", rest)
	}

	// An empty held set means nothing is holdable, so everything is stranded.
	// It is the honest reading and no caller passes it; asserting it stops a
	// future caller from discovering it means "nothing is wrong" instead.
	all, err := s.ListCredentialsOutsideKEKIDsIncludingDeleted(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("an empty held set reported %d stranded row(s), want all 3", len(all))
	}
}

// envelopeFor reads one row's stored envelope, tombstones included.
func envelopeFor(t *testing.T, s *Store, id int64) []byte {
	t.Helper()
	rows, err := s.ListCredentialEnvelopesIncludingDeleted(t.Context(), 0, 0)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	for _, c := range rows {
		if c.ID == id {
			return c.Envelope
		}
	}
	t.Fatalf("service_instance %d is not in the table", id)
	return nil
}
