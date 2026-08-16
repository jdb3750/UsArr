package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/db"
)

// fixtureAPIKey stands in for an *Arr API key. Mostly zeroes so it can never be
// mistaken for a real credential.
const fixtureAPIKey = "0000000000000000000000000000beef"

var testNow = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "usarr.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return New(d)
}

func newTestKeyring(t *testing.T) *crypto.Keyring {
	t.Helper()
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = 0xAB
	}
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = 0xCD
	}
	kek, err := crypto.DeriveKEK(secret, salt)
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	kr, err := crypto.NewKeyring(1, kek)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	return kr
}

func TestTimeRoundTripMatchesSQLiteFormat(t *testing.T) {
	// Go-written timestamps must be byte-identical in shape to what
	// datetime('now') produces, or lexicographic ORDER BY on these TEXT
	// columns silently breaks.
	got := FormatTime(testNow)
	if got != "2026-08-16 12:00:00" {
		t.Fatalf("FormatTime = %q, want SQLite's datetime() shape", got)
	}
	back, err := ParseTime(got)
	if err != nil {
		t.Fatalf("ParseTime: %v", err)
	}
	if !back.Equal(testNow) {
		t.Errorf("ParseTime = %v, want %v", back, testNow)
	}
	if _, err := ParseTime("2026-08-16T12:00:00Z"); err == nil {
		t.Error("ParseTime accepted RFC 3339; the stored format is SQLite's datetime()")
	}
}

// The access-scope parameter is in the query signature from the first commit.
// v0.1 passes OwnerScope and nothing is filtered; the point is that multi-user
// is a behaviour change rather than a redesign.
func TestScopeIsEnforcedOnReads(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	visible, err := s.CreateServiceInstance(ctx, ServiceInstance{
		Kind: "radarr", Name: "Radarr", BaseURL: "http://radarr.lan:7878",
		APIKeyEnc: []byte{1}, KEKID: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	hidden, err := s.CreateServiceInstance(ctx, ServiceInstance{
		Kind: "sonarr", Name: "Sonarr", BaseURL: "http://sonarr.lan:8989",
		APIKeyEnc: []byte{1}, KEKID: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}

	owner := OwnerScope(1)
	all, err := s.ListServiceInstances(ctx, owner)
	if err != nil {
		t.Fatalf("ListServiceInstances: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("owner sees %d instances, want 2", len(all))
	}

	// A narrowed scope sees only what it is granted.
	narrow := Scope{UserID: 2, InstanceIDs: []int64{visible}}
	got, err := s.ListServiceInstances(ctx, narrow)
	if err != nil {
		t.Fatalf("ListServiceInstances: %v", err)
	}
	if len(got) != 1 || got[0].ID != visible {
		t.Fatalf("narrow scope saw %d instances, want just %d", len(got), visible)
	}

	// And an out-of-scope row is reported as missing, not as forbidden: saying
	// "exists but denied" is an existence oracle.
	if _, err := s.GetServiceInstance(ctx, narrow, hidden); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetServiceInstance out of scope = %v, want ErrNotFound", err)
	}

	// An empty scope fails closed: no visible instances means no rows, never
	// all rows.
	empty := Scope{UserID: 3}
	got, err = s.ListServiceInstances(ctx, empty)
	if err != nil {
		t.Fatalf("ListServiceInstances: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty scope saw %d instances, want 0 (fail closed)", len(got))
	}
}

// The credential is sealed against the row's own primary key, which does not
// exist until the insert. Both halves must be in one transaction.
func TestCreateServiceInstanceSealed(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	kr := newTestKeyring(t)
	const baseURL = "http://radarr.lan:7878"

	id, err := s.CreateServiceInstanceSealed(ctx, ServiceInstance{
		Kind: "radarr", Name: "Radarr", BaseURL: baseURL, Enabled: true, VerifyTLS: true,
	}, func(id int64) ([]byte, uint32, error) {
		aad, err := crypto.ServiceInstanceAAD(id, baseURL)
		if err != nil {
			return nil, 0, err
		}
		env, err := kr.Seal([]byte(fixtureAPIKey), aad)
		return env, kr.PrimaryID(), err
	})
	if err != nil {
		t.Fatalf("CreateServiceInstanceSealed: %v", err)
	}

	si, err := s.GetServiceInstance(ctx, OwnerScope(1), id)
	if err != nil {
		t.Fatalf("GetServiceInstance: %v", err)
	}
	if si.KEKID != kr.PrimaryID() {
		t.Errorf("kek_id column = %d, want %d", si.KEKID, kr.PrimaryID())
	}
	// The plain kek_id column must agree with the id inside the envelope, or a
	// rotation's `WHERE kek_id = :old` misses rows.
	inEnvelope, err := crypto.KEKID(si.APIKeyEnc)
	if err != nil {
		t.Fatalf("crypto.KEKID: %v", err)
	}
	if inEnvelope != si.KEKID {
		t.Errorf("kek_id column %d disagrees with the envelope's %d", si.KEKID, inEnvelope)
	}

	aad, err := crypto.ServiceInstanceAAD(id, si.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := kr.Open(si.APIKeyEnc, aad)
	if err != nil {
		t.Fatalf("Open the stored credential: %v", err)
	}
	if string(plain) != fixtureAPIKey {
		t.Errorf("decrypted key = %q, want %q", plain, fixtureAPIKey)
	}
}

// If sealing fails the row must not exist: a service_instance with no usable
// credential is a broken services screen with nothing to fix it with.
func TestCreateServiceInstanceSealedRollsBack(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	sealErr := errors.New("no key available")
	_, err := s.CreateServiceInstanceSealed(ctx, ServiceInstance{
		Kind: "radarr", Name: "Radarr", BaseURL: "http://radarr.lan:7878",
	}, func(int64) ([]byte, uint32, error) {
		return nil, 0, sealErr
	})
	if !errors.Is(err, sealErr) {
		t.Fatalf("CreateServiceInstanceSealed = %v, want the seal error", err)
	}

	all, err := s.ListServiceInstances(ctx, OwnerScope(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("a row survived a failed seal: %+v", all)
	}
}

func TestServiceInstanceLifecycle(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	owner := OwnerScope(1)

	id, err := s.CreateServiceInstance(ctx, ServiceInstance{
		Kind: "prowlarr", Name: "Prowlarr", BaseURL: "http://prowlarr.lan:9696",
		APIKeyEnc: []byte{1}, KEKID: 1, Enabled: true, Role: "indexer",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}

	si, err := s.GetServiceInstance(ctx, owner, id)
	if err != nil {
		t.Fatalf("GetServiceInstance: %v", err)
	}
	if si.Role != "indexer" || si.ManagedBy != "ui" || si.HealthState != "unknown" {
		t.Errorf("defaults wrong: role=%q managed_by=%q health=%q", si.Role, si.ManagedBy, si.HealthState)
	}

	if err := s.UpdateServiceInstanceCredential(ctx, id, []byte{9, 9, 9}, 2); err != nil {
		t.Fatalf("UpdateServiceInstanceCredential: %v", err)
	}
	si, err = s.GetServiceInstance(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if si.KEKID != 2 || string(si.APIKeyEnc) != "\x09\x09\x09" {
		t.Errorf("credential not updated: kek_id=%d enc=%v", si.KEKID, si.APIKeyEnc)
	}

	ok := testNow
	if err := s.UpdateServiceInstanceHealth(ctx, id, "ok", "closed", 0, &ok, ""); err != nil {
		t.Fatalf("UpdateServiceInstanceHealth: %v", err)
	}
	si, err = s.GetServiceInstance(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if si.HealthState != "ok" || !si.LastOKAt.Valid {
		t.Errorf("health not updated: %+v", si)
	}

	// Soft delete tombstones the row; the id stays burned.
	if err := s.SoftDeleteServiceInstance(ctx, id, testNow); err != nil {
		t.Fatalf("SoftDeleteServiceInstance: %v", err)
	}
	if _, err := s.GetServiceInstance(ctx, owner, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetServiceInstance after delete = %v, want ErrNotFound", err)
	}
	// Deleting twice is not a silent success.
	if err := s.SoftDeleteServiceInstance(ctx, id, testNow); !errors.Is(err, ErrNotFound) {
		t.Errorf("second SoftDeleteServiceInstance = %v, want ErrNotFound", err)
	}
	if _, err := s.GetServiceInstance(ctx, owner, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetServiceInstance on a missing id = %v, want ErrNotFound", err)
	}
}

// The question the startup path asks after config.ResolveMasterKey returns
// ErrKeyAbsent. A tombstoned row still counts: its ciphertext is still there.
func TestCountEncryptedCredentials(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	n, err := s.CountEncryptedCredentials(ctx)
	if err != nil {
		t.Fatalf("CountEncryptedCredentials: %v", err)
	}
	if n != 0 {
		t.Fatalf("fresh database reports %d encrypted credentials, want 0", n)
	}

	id, err := s.CreateServiceInstance(ctx, ServiceInstance{
		Kind: "radarr", Name: "Radarr", BaseURL: "http://radarr.lan:7878",
		APIKeyEnc: []byte("an-envelope"), KEKID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err = s.CountEncryptedCredentials(ctx); err != nil || n != 1 {
		t.Fatalf("CountEncryptedCredentials = %d (err %v), want 1", n, err)
	}

	if err := s.SoftDeleteServiceInstance(ctx, id, testNow); err != nil {
		t.Fatal(err)
	}
	if n, err = s.CountEncryptedCredentials(ctx); err != nil || n != 1 {
		t.Fatalf("after tombstoning, CountEncryptedCredentials = %d (err %v), want 1: "+
			"the ciphertext is still in the row", n, err)
	}
}
