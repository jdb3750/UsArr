package main

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/store"
)

// TestATamperedCredentialWritesAnAuditRow is the drill for
// reference/security.md §1.2's second half.
//
// The claim is "a decryption failure with a valid KEK … is a loud,
// audit-logged failure, never a silent skip". The loud half has always been
// real. The audit half was a sentence with no code behind it, which is the
// worst kind of control: an operator reading that line would believe a tamper
// attempt against the background health prober left a durable trace, and it
// left a log line that rotates away.
//
// The tamper is the REAL one, not a stand-in. A truncated or garbage envelope
// fails structurally (crypto.ErrMalformed) and needs no key at all; flipping a
// byte of the GCM tag inside an otherwise well-formed envelope is what an
// attacker editing the database file produces, and it is the case the section
// is about — ErrDecrypt under a KEK the keyring holds.
func TestATamperedCredentialWritesAnAuditRow(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"
	prowlarr := newFakeProwlarr(t, apiKey)
	dir := t.TempDir()

	instanceID := sealOneCredential(t, dir, prowlarr.URL(), apiKey)

	a, err := startOn(t, dir)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	ctx := context.Background()

	si, err := a.store.GetServiceInstance(ctx, store.Scope{AllInstances: true}, instanceID)
	if err != nil {
		t.Fatalf("read service instance: %v", err)
	}
	// Sanity: the untampered credential opens, and does so WITHOUT writing a
	// row. Auditing every successful open would put a row a minute per instance
	// into an append-only table and bury the one row that matters.
	if opened, openErr := a.registry.openCredential(ctx, si); openErr != nil || opened != apiKey {
		t.Fatalf("the fixture's credential does not open: %v", openErr)
	}
	if rows := credentialAuditRows(t, a.store, instanceID); len(rows) != 0 {
		t.Fatalf("a SUCCESSFUL open wrote %d audit row(s); only failures are audited", len(rows))
	}

	tampered := append([]byte(nil), si.APIKeyEnc...)
	tampered[len(tampered)-1] ^= 0x01 // the last byte of the GCM tag
	if err := a.store.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx,
			`UPDATE service_instance SET api_key_enc = ? WHERE id = ?`, tampered, instanceID)
		return execErr
	}); err != nil {
		t.Fatalf("tamper with the sealed envelope: %v", err)
	}

	si, err = a.store.GetServiceInstance(ctx, store.Scope{AllInstances: true}, instanceID)
	if err != nil {
		t.Fatalf("re-read service instance: %v", err)
	}

	// The loud half.
	opened, err := a.registry.openCredential(ctx, si)
	if err == nil {
		t.Fatalf("a tampered envelope opened to %q; the AAD/GCM check did not fire", opened)
	}
	if !errors.Is(err, crypto.ErrDecrypt) && !strings.Contains(err.Error(), "cannot be opened") {
		t.Fatalf("the failure is not the tampering failure this test is about: %v", err)
	}

	// The durable half — the whole point of this test.
	rows := credentialAuditRows(t, a.store, instanceID)
	if len(rows) != 1 {
		t.Fatalf("a credential that failed to decrypt wrote %d audit rows, want exactly 1. "+
			"reference/security.md §1.2 promises a tampering attempt leaves a durable trace, "+
			"and a background path (the health prober, a bootstrap import) has no other one",
			len(rows))
	}
	row := rows[0]
	if row.Result != store.AuditResultFail {
		t.Errorf("result = %q, want %q", row.Result, store.AuditResultFail)
	}
	if row.TargetType != "service_instance" || row.TargetID != strconv.FormatInt(instanceID, 10) {
		t.Errorf("target = %s/%s, want service_instance/%d", row.TargetType, row.TargetID, instanceID)
	}
	// A NULL actor would make this row invisible to every scoped audit read —
	// `NULL IN (0, :uid)` is unknown, not true — so the row would exist and
	// nothing would ever show it. credentialAuditRows reads through the scoped
	// API precisely so that failure mode cannot pass this test silently, and
	// this asserts the sentinel it depends on.
	if !row.ActorUserID.Valid || row.ActorUserID.Int64 != store.SystemUserID {
		t.Errorf("actor = %v, want the system sentinel %d — a NULL actor is unreadable",
			row.ActorUserID, store.SystemUserID)
	}

	// No key material, no envelope, no credential. audit_log has BEFORE UPDATE
	// and BEFORE DELETE triggers that RAISE(ABORT), so anything written here is
	// written forever.
	for _, forbidden := range []string{apiKey, string(si.APIKeyEnc), si.BaseURL} {
		if forbidden != "" && strings.Contains(row.MetadataJSON, forbidden) {
			t.Errorf("the audit metadata leaks something it must not: %s", row.MetadataJSON)
		}
	}
	if !strings.Contains(row.MetadataJSON, `"kek_id"`) {
		t.Errorf("the audit metadata names no kek_id, so an operator cannot tell which key "+
			"was expected: %s", row.MetadataJSON)
	}
	t.Logf("audit row: action=%s result=%s target=%s/%s metadata=%s",
		row.Action, row.Result, row.TargetType, row.TargetID, row.MetadataJSON)
}

// credentialAuditRows reads the credential.open rows for one instance THROUGH
// THE SCOPED READ, not with a raw SELECT: a row a scoped read cannot see is not
// audit evidence, and a raw SELECT would report success for exactly the NULL
// actor bug that makes it invisible.
func credentialAuditRows(t *testing.T, st *store.Store, instanceID int64) []store.AuditEntry {
	t.Helper()
	entries, err := st.ListAuditLog(context.Background(), store.OwnerScope(store.SystemUserID),
		store.AuditQuery{Actions: []string{store.AuditActionCredentialOpen}})
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	want := strconv.FormatInt(instanceID, 10)
	var out []store.AuditEntry
	for _, e := range entries {
		if e.TargetID == want {
			out = append(out, e)
		}
	}
	return out
}
