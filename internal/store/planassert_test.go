package store

import (
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// PREFIX ROT, and the assertion shape that survives it.
//
// A query-plan guard asserts on identifiers — index names, table names, aliases
// — with strings.Contains, and the plan is one long string. That is fine until
// an identifier is RENAMED to something that begins with the old name:
// `ix_work_added` → `ix_work_added_at`, `sil` → `sil_w`, `sdl` → `sdl_f`. The
// old needle still matches the new plan, so the guard stays GREEN while pinning
// nothing at all — and there is nothing in the symptom to point at the cause,
// because a guard that passes by accident looks exactly like one that passes
// correctly.
//
// This is not hypothetical. LS-379's fix derived three inner aliases from their
// correlated columns; ONE plan guard went red and told us so, and TWO went on
// passing on `sil_w` by prefix while asserting nothing.
//
// planHas is the fix in one function: the same substring match, plus the
// requirement that the match END AT A TOKEN BOUNDARY. A longer identifier that
// merely starts with the expected one no longer satisfies it.
//
// WHERE THE IDENTIFIER IS DERIVED IN PRODUCTION CODE (the scope aliases), the
// guards call the SAME derivation rather than a literal — a test that computes
// what the code computes cannot drift from it. planHas is for the identifiers
// that must stay literal, which is every index and table name: those live in a
// migration, and the test has nothing to derive them from.
// ─────────────────────────────────────────────────────────────────────────────

// planHas is db.PlanHas under this package's original name, so the fifteen call
// sites below and in the sibling files read as they did when the sweep landed.
//
// THE IMPLEMENTATION MOVED, it was not copied. internal/db/*_test.go has plan
// guards with the same rot, and internal/db cannot import internal/store — so
// the one implementation lives beside QueryPlan in internal/db/sqlite.go, which
// both packages already depend on. See db.PlanHas for why the match is
// one-directional.
func planHas(plan, want string) bool { return db.PlanHas(plan, want) }

// PROVING THE SHAPE, on the real plans rather than on invented strings.
//
// Each case takes a plan the shipped guard passes on, RENAMES the identifier the
// guard names to `<old>_x` — the exact rot this file exists to catch — and
// asserts two things: that the OLD substring assertion is still satisfied (so
// the hazard is real and this test is not describing a problem that does not
// exist), and that the guard AS WRITTEN NOW rejects it.
//
// If a guard listed here is ever rewritten back to a bare strings.Contains, the
// second assertion fails and says so.
func TestPlanGuardsRejectAPrefixRename(t *testing.T) {
	s := newTestStore(t)
	seedRecentCorpus(t, s)
	recentPlan := recentWorksPlan(t, s, OwnerScope(1), RecentWorksCursor{})

	b := newTestStore(t)
	seedBrowseCorpus(t, b)
	browsePlan := browseWorksPlan(t, b, OwnerScope(1), WorksFilter{LibraryIDs: []int64{2, 3}}, WorksCursor{})

	for _, tc := range []struct {
		name    string
		plan    string
		ident   string
		needle  string // the pre-sweep substring needle, kept as the hazard's proof
		faultsA func(string) []string
	}{
		{
			name:    "recentWorksPlanFaults / ix_work_added",
			plan:    recentPlan,
			ident:   "ix_work_added",
			needle:  "USING INDEX ix_work_added",
			faultsA: recentWorksPlanFaults,
		},
		{
			name:   "browseMembershipProbeFaults / ix_libmem_work",
			plan:   browsePlan,
			ident:  "ix_libmem_work",
			needle: "SEARCH lm EXISTS USING COVERING INDEX ix_libmem_work",
			faultsA: func(plan string) []string {
				return browseMembershipProbeFaults(plan)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if faults := tc.faultsA(tc.plan); len(faults) > 0 {
				t.Fatalf("the guard does not pass on the real plan, so this case is "+
					"measuring nothing:\n  plan: %s\n  %s", tc.plan, strings.Join(faults, "\n  "))
			}

			renamed := strings.ReplaceAll(tc.plan, tc.ident, tc.ident+"_x")
			if renamed == tc.plan {
				t.Fatalf("the plan does not name %q, so the rename is a no-op and this "+
					"case tests nothing:\n%s", tc.ident, tc.plan)
			}

			// The hazard, demonstrated rather than asserted: the pre-sweep needle
			// is STILL satisfied by the renamed plan.
			if !strings.Contains(renamed, tc.needle) {
				t.Fatalf("strings.Contains(%q) no longer matches the renamed plan, so "+
					"this case is not demonstrating prefix rot any more:\n%s",
					tc.needle, renamed)
			}

			if faults := tc.faultsA(renamed); len(faults) == 0 {
				t.Fatalf("the guard PASSES on a plan where %s has been renamed to %s_x. "+
					"It is matching by prefix and pinning nothing — use planHas:\n%s",
					tc.ident, tc.ident, renamed)
			}
		})
	}
}

// The helper's own property test MOVED WITH THE IMPLEMENTATION: it is
// TestPlanHasMatchesWholeTokensOnly in internal/db/queryplan_test.go, beside
// PlanHas. Keeping a second copy of the same table here would be the
// duplication this consolidation removed.
