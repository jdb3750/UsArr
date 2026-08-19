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
