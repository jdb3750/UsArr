package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// The two catalogue-freshness fields on GET /api/v1/services/health:
// `last_full_sync_at` and `work_count`.
//
// Every test here runs against a REAL MIGRATED DATABASE with rows in it. The
// whole subject is a distinction between three states — never synced, synced and
// empty, synced with a catalogue — and none of them is observable at zero rows.

// seedSyncCorpus writes three instances that sit in exactly those three states,
// plus the rows that make the count non-trivial: a link into a soft-deleted
// work, two links onto ONE work (the §6.4 tier-1 merge), and a 'person' work
// with no link at all.
func seedSyncCorpus(t *testing.T, s *Server) {
	t.Helper()
	stmts := []string{
		// 1 — synced, and holding a catalogue.
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc, last_full_sync_at)
		   VALUES (1, 'kavita', 'library', 'Kavita', 'http://kavita.example', X'00',
		           '2026-08-16 12:00:00')`,
		// 2 — synced, and the library was EMPTY. Not the same fact as 3.
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc, last_full_sync_at)
		   VALUES (2, 'kavita', 'library', 'Empty Kavita', 'http://empty.example', X'00',
		           '2026-08-16 13:00:00')`,
		// 3 — NEVER synced. last_full_sync_at is NULL.
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (3, 'kavita', 'library', 'Fresh Kavita', 'http://fresh.example', X'00')`,
	}
	works := []struct {
		id      int
		kind    string
		title   string
		deleted string
	}{
		{1, "comic", "Berserk", ""},
		{2, "comic", "Berserk (Deluxe)", ""},
		{3, "book", "Piranesi", ""},
		{4, "comic", "A Soft-Deleted Comic", "2026-08-16 09:00:00"},
		// A credited author. It is a `work` row and it is NOT part of any
		// instance's contribution, because the credit pass writes no link for
		// one — asserted rather than assumed by the count below.
		{5, "person", "Kentaro Miura", ""},
	}
	for _, w := range works {
		del := "NULL"
		if w.deleted != "" {
			del = "'" + w.deleted + "'"
		}
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO work
			  (id, kind, title, sort_title, normalized_title, deleted_at)
			VALUES (%d, '%s', '%s', '%s', '%s', %s)`,
			w.id, w.kind, w.title, strings.ToLower(w.title), strings.ToLower(w.title), del))
	}
	links := []struct {
		remote  string
		work    int
		deleted string
	}{
		{"r1", 1, ""},
		// TWO remote items on ONE work. A link count would say 4 here; the
		// honest answer is 3.
		{"r2", 1, ""},
		{"r3", 2, ""},
		{"r4", 3, ""},
		// Into a soft-deleted work — outside the count.
		{"r5", 4, ""},
		// A tombstoned link onto a live work — also outside it.
		{"r6", 2, "2026-08-16 09:00:00"},
	}
	for _, l := range links {
		del := "NULL"
		if l.deleted != "" {
			del = "'" + l.deleted + "'"
		}
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO service_item_link
			  (service_instance_id, work_id, remote_id, remote_kind, synced_at, deleted_at)
			VALUES (1, %d, '%s', 'series', '2026-08-16 12:00:00', %s)`, l.work, l.remote, del))
	}

	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func callServicesHealth(t *testing.T, s *Server) (int, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/services/health", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: true},
	}))
	w := httptest.NewRecorder()
	if err := s.handleServicesHealth(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

// healthRows decodes the response as RAW JSON per row, so `null` and an ABSENT
// key are distinguishable. Decoding into a *time.Time would collapse both to
// nil, which is exactly the confusion this endpoint exists to remove.
func healthRows(t *testing.T, body string) map[string]map[string]json.RawMessage {
	t.Helper()
	var envelope struct {
		Services []map[string]json.RawMessage `json:"services"`
	}
	mustJSON(t, body, &envelope)
	out := map[string]map[string]json.RawMessage{}
	for _, row := range envelope.Services {
		var name string
		mustJSON(t, string(row["name"]), &name)
		out[name] = row
	}
	return out
}

// TestServicesHealthDistinguishesNeverSyncedFromSyncedAndEmpty is the whole
// point of the pair of fields. A single number cannot carry it: "0 items" is
// the answer for a service that has never run and for one that ran and found an
// empty library, and those need different sentences on the screen.
func TestServicesHealthDistinguishesNeverSyncedFromSyncedAndEmpty(t *testing.T) {
	s := newTestServer(t, nil)
	seedSyncCorpus(t, s)

	code, body := callServicesHealth(t, s)
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/services/health = %d: %s", code, body)
	}
	rows := healthRows(t, body)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}

	for _, tc := range []struct{ name, wantSync, wantCount string }{
		// Three works: Berserk (two links, one work), Berserk (Deluxe), Piranesi.
		// NOT five — the soft-deleted work, the tombstoned link, the duplicate
		// link and the person are all out.
		{"Kavita", `"2026-08-16T12:00:00Z"`, `3`},
		{"Empty Kavita", `"2026-08-16T13:00:00Z"`, `0`},
		// NULL, EXPLICITLY. Not an absent key: the browser must be able to tell
		// "never synced" from "this build does not send the field".
		{"Fresh Kavita", `null`, `0`},
	} {
		row, ok := rows[tc.name]
		if !ok {
			t.Fatalf("no row named %q", tc.name)
		}
		raw, present := row["last_full_sync_at"]
		if !present {
			t.Errorf("%s: last_full_sync_at is ABSENT; it must always be sent, "+
				"as a value or as an explicit null", tc.name)
		} else if string(raw) != tc.wantSync {
			t.Errorf("%s: last_full_sync_at = %s, want %s", tc.name, raw, tc.wantSync)
		}
		rawCount, present := row["work_count"]
		if !present {
			t.Errorf("%s: work_count is ABSENT; 0 is a real answer and must be sent", tc.name)
		} else if string(rawCount) != tc.wantCount {
			t.Errorf("%s: work_count = %s, want %s", tc.name, rawCount, tc.wantCount)
		}
	}
}

// TestServicesHealthKeysAreTheAllowlist pins the response shape whole. This row
// is built from a service_instance record that also holds an ENCRYPTED
// FULL-ADMIN CREDENTIAL, so a field added by a later change — including one
// added by widening the row rather than naming a column — fails here first.
func TestServicesHealthKeysAreTheAllowlist(t *testing.T) {
	s := newTestServer(t, nil)
	seedSyncCorpus(t, s)

	_, body := callServicesHealth(t, s)
	rows := healthRows(t, body)

	// Kavita is the row with every non-omitempty key populated. warnings,
	// blocked_indexers, observed_at, problem and app_version are omitempty and
	// need a probe snapshot, which this test deliberately does not have.
	want := []string{
		"action", "base_url", "breaker_state", "consecutive_failures", "enabled",
		// Added deliberately, and named here rather than allowed through by
		// widening: http-api.md §3.4. It is a COUNT and carries no reason and no
		// upstream text — the reason lives in sync_report.detail, which the
		// browser never sees.
		"file_read_failures",
		"id", "kind", "last_full_sync_at", "name", "role", "stale", "state",
		"work_count",
	}
	got := keysOf(rows["Kavita"])
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("row keys = %v\n         want %v", got, want)
	}

	// And nothing credential-shaped, asserted against the RESPONSE BYTES rather
	// than the struct.
	for _, banned := range []string{"api_key", "apiKey", "api_key_enc", "kek_id", "tls_spki_pin"} {
		if strings.Contains(body, banned) {
			t.Errorf("the health response contains %q", banned)
		}
	}
}

// TestWorkCountIsScoped keeps the aggregate honest about visibility. It groups
// across instances, so without the scope predicate a user who can see one
// service learns the catalogue size of every other one.
func TestWorkCountIsScoped(t *testing.T) {
	s := newTestServer(t, nil)
	seedSyncCorpus(t, s)

	counts, err := s.store.WorkCountsByInstance(t.Context(), store.Scope{UserID: 1})
	if err != nil {
		t.Fatalf("WorkCountsByInstance: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("a scope with no visible instances got counts for %v — it must fail closed", counts)
	}

	counts, err = s.store.WorkCountsByInstance(t.Context(),
		store.Scope{UserID: 1, InstanceIDs: []int64{2}})
	if err != nil {
		t.Fatalf("WorkCountsByInstance: %v", err)
	}
	if _, leaked := counts[1]; leaked {
		t.Errorf("a scope naming only instance 2 was told instance 1's count: %v", counts)
	}
}

// seedWalkFailures appends `file_walk_failed` rows to the corpus.
func seedWalkFailures(t *testing.T, s *Server, rows ...[3]string) {
	t.Helper()
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO sync_report
				  (service_instance_id, kind, remote_kind, remote_id, detail, created_at)
				VALUES (?, ?, 'series', ?, '{"phase":"files","reason":"server_error"}', ?)`,
				r[0], store.SyncReportFileWalkFailed, r[1], r[2]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed sync_report: %v", err)
	}
}

func rawField(t *testing.T, row map[string]json.RawMessage, key string) string {
	t.Helper()
	raw, present := row[key]
	if !present {
		t.Fatalf("%s is ABSENT; 0 is a real answer and must be sent", key)
	}
	return string(raw)
}

// TestServicesHealthCountsItemsTheFileWalkCouldNotRead is what makes an
// uncounted item explicable.
//
// ⚠️ THE READ IS sync_report's ONLY ONE. The importer wrote `file_walk_failed`
// rows and nothing read them, which surfaces exactly as much as the silent
// `continue` those rows replaced. The four cases below are the whole contract of
// http-api.md §3.4.
func TestServicesHealthCountsItemsTheFileWalkCouldNotRead(t *testing.T) {
	s := newTestServer(t, nil)
	seedSyncCorpus(t, s)
	seedWalkFailures(t, s,
		// Instance 1 completed a run at 12:00. Two series failed DURING it…
		[3]string{"1", "r1", "2026-08-16 12:00:30"},
		[3]string{"1", "r3", "2026-08-16 12:00:31"},
		// …the same series failed twice, which is one series, not two…
		[3]string{"1", "r3", "2026-08-16 12:00:45"},
		// …and one failed in an EARLIER run that the completed one superseded.
		[3]string{"1", "r4", "2026-08-15 08:00:00"},
		// Instance 3 has NEVER completed a run, so there is no earlier run to
		// exclude and its one partial run's failure counts.
		[3]string{"3", "r9", "2026-08-16 07:00:00"},
	)

	code, body := callServicesHealth(t, s)
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/services/health = %d: %s", code, body)
	}
	rows := healthRows(t, body)

	for _, tc := range []struct{ name, want, why string }{
		{"Kavita", "2", "two DISTINCT series failed since the last completed run started"},
		{"Empty Kavita", "0", "an instance with no rows sends 0, not an absent key"},
		{"Fresh Kavita", "1", "an instance that never completed a run counts what it has"},
	} {
		if got := rawField(t, rows[tc.name], "file_read_failures"); got != tc.want {
			t.Errorf("%s: file_read_failures = %s, want %s — %s", tc.name, got, tc.want, tc.why)
		}
	}

	// And the response says nothing about WHY, on §5.5.5's rule: the classified
	// reason is in sync_report.detail, for an operator with database access.
	if strings.Contains(body, "server_error") || strings.Contains(body, "reason") {
		t.Errorf("the health response carries a failure reason: %s", body)
	}
}
