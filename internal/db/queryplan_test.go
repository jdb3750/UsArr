package db

import (
	"strings"
	"testing"
)

// Query-plan assertions. docs/DEVELOPMENT.md §5 puts these in CI because they
// are deterministic and hardware-independent: ~30 lines that catch an index
// regression forever. Wall-clock budgets live in `make bench` and are never a
// merge gate.
//
// WHAT IS ASSERTED, AND WHAT IS DELIBERATELY NOT: each case asserts that the
// step is a SEARCH (not a SCAN) and that the expected index is named. It does
// NOT assert COVERING. Per docs/reference/schema.md §1, whether SQLite can
// serve a query from the index alone depends on the SELECT list, so pinning
// COVERING makes the test fail the moment a caller selects one more column —
// which is a change to the query, not an index regression.
func TestQueryPlans(t *testing.T) {
	ctx := t.Context()
	d := openTestDB(t)

	tests := []struct {
		name string
		// index is the index the plan must name.
		index string
		query string
		args  []any
		// scan marks a query whose plan is an ordered SCAN over an index
		// rather than a SEARCH. That is the right plan for "newest N": the
		// index supplies the order, so there is no temp b-tree.
		scan bool
	}{
		{
			// The canonical tag filter. user_id is IN the index because the
			// predicate always carries it; without it the covering property is
			// lost and a common tag on 400k items becomes 400k random reads.
			name:  "tag filter is user-scoped and indexed",
			index: "ix_ta_tag",
			query: `SELECT work_id FROM tag_assignment WHERE tag_id = ? AND user_id IN (0, ?)`,
			args:  []any{1, 1},
		},
		{
			name:  "tags of one work",
			index: "ix_ta_work",
			query: `SELECT tag_id FROM tag_assignment WHERE work_id = ?`,
			args:  []any{1},
		},
		{
			name:  "tags of one service instance",
			index: "ix_ta_inst_lookup",
			query: `SELECT id FROM tag_assignment WHERE service_instance_id = ? AND tag_id = ?`,
			args:  []any{1, 1},
		},
		{
			// The northbound auth path: one indexed lookup, then one HMAC and
			// one constant-time compare. This is what keeps
			// GET /rest/ping?apiKey=garbage from costing anything.
			name:  "api key lookup by prefix",
			index: "ux_cc_prefix",
			query: `SELECT id, user_id, key_hash FROM client_credential WHERE key_prefix = ?`,
			args:  []any{"abcdefgh"},
		},
		{
			name:  "live credentials for one user",
			index: "ix_cc_user",
			query: `SELECT id FROM client_credential WHERE user_id = ? AND revoked_at IS NULL`,
			args:  []any{1},
		},
		{
			name:  "live sessions for one user",
			index: "ix_session_user",
			query: `SELECT id FROM session WHERE user_id = ? AND revoked_at IS NULL`,
			args:  []any{1},
		},
		{
			name:  "newest audit rows",
			index: "ix_audit_ts",
			query: `SELECT id, action FROM audit_log ORDER BY ts DESC LIMIT 100`,
			scan:  true,
		},
		{
			// The TTL sweep. Prowlarr drops grabbable releases after 30
			// minutes, so this runs often and must not scan the table.
			name:  "expired release candidates",
			index: "ix_rel_expiry",
			query: `SELECT id FROM release_candidate WHERE expires_at <= ?`,
			args:  []any{"2026-08-16 00:00:00"},
		},
		{
			// downloadId is THE join key reattaching an import event to the
			// grab that produced it.
			name:  "provenance by download id",
			index: "ix_prov_dlid",
			query: `SELECT id FROM provenance WHERE download_id = ?`,
			args:  []any{"nzo_1"},
		},
		{
			name:  "provenance by indexer",
			index: "ix_prov_indexer",
			query: `SELECT id FROM provenance WHERE indexer_name = ?`,
			args:  []any{"an-indexer"},
		},
		{
			name:  "provenance by protocol",
			index: "ix_prov_protocol",
			query: `SELECT id FROM provenance WHERE protocol = ?`,
			args:  []any{"torrent"},
		},
		{
			// Idempotency is (user_id, key), never a bare unique on the key: a
			// globally unique client-supplied key means a replay can return
			// another user's payload.
			name:  "write queue idempotency lookup",
			index: "ux_wq_idem",
			query: `SELECT id FROM write_queue WHERE user_id = ? AND idempotency_key = ?`,
			args:  []any{0, "01J000000000000000000000"},
		},
		{
			// Also the reconciliation guard's index: the sweep skips any
			// work_id with a row in pending, inflight or verifying.
			name:  "runnable write queue entries",
			index: "ix_wq_runnable",
			query: `SELECT id FROM write_queue
			         WHERE state IN ('pending','inflight','verifying') AND next_attempt_at <= ?`,
			args: []any{"2026-08-16 00:00:00"},
		},
		{
			name:  "write queue entries for one work",
			index: "ix_wq_work",
			query: `SELECT id FROM write_queue WHERE work_id = ? AND state = ?`,
			args:  []any{1, "pending"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := QueryPlan(ctx, d.Read(), tc.query, tc.args...)
			if err != nil {
				t.Fatalf("QueryPlan: %v", err)
			}
			joined := strings.Join(plan, " | ")

			if !strings.Contains(joined, tc.index) {
				t.Fatalf("plan does not use %s:\n  %s", tc.index, joined)
			}
			want := "SEARCH"
			if tc.scan {
				want = "SCAN"
			}
			if !strings.Contains(joined, want) {
				t.Errorf("plan is not a %s:\n  %s", want, joined)
			}
			if !tc.scan && strings.Contains(joined, "SCAN") {
				t.Errorf("plan contains a table SCAN:\n  %s", joined)
			}
			// An ordered read that still needs a sort is not using the index
			// for what it was created for.
			if strings.Contains(joined, "TEMP B-TREE") {
				t.Errorf("plan needs a temp b-tree:\n  %s", joined)
			}
		})
	}
}

// service_instance is deliberately left without an ordering index.
//
// The services list sorts by (priority, name) and plans as a scan plus a temp
// b-tree. That is correct: docs/reference/schema.md §5 specifies no such index,
// and a homelab has single-digit instances. Pinned here so the scan is a
// recorded decision rather than something a later reader "fixes" by adding an
// index to a ten-row table.
//
// The assertion below is t.Errorf, not t.Logf. It used to log, which meant the
// test could not fail: adding an ordering index left it green and silent, so it
// pinned nothing while claiming to pin a decision. Failing is the point — if
// someone adds the index, this test forces them to come here and change it
// deliberately, which is exactly the conversation the comment above wants to
// have. Removing the pin is a one-line edit; not noticing you removed it is the
// failure mode being guarded.
func TestServiceInstanceListScanIsIntentional(t *testing.T) {
	plan, err := QueryPlan(t.Context(), openTestDB(t).Read(),
		`SELECT id, name FROM service_instance WHERE deleted_at IS NULL
		  ORDER BY priority DESC, name ASC`)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	if !strings.Contains(strings.Join(plan, " | "), "SCAN service_instance") {
		t.Errorf("service_instance is no longer planned as a scan: %v\n"+
			"schema.md §5 specifies no ordering index on this table and a homelab has "+
			"single-digit instances. If adding one is intended, say so in schema.md §5 "+
			"and update this test in the same change.", plan)
	}
}
