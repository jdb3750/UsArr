package store

import (
	"strings"
	"testing"
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

// planHas reports whether plan contains want AS A WHOLE TOKEN — the match must
// not be followed by another identifier character, so `ix_work_added` does not
// match a plan that names `ix_work_added_at`.
//
// It is deliberately one-directional. The needles here always END with the
// identifier under test and begin with plan keywords (`SEARCH `, `USING INDEX `)
// or with the identifier itself, and a plan identifier is never preceded by
// another identifier character, so only the trailing edge can rot.
func planHas(plan, want string) bool {
	for i := 0; i+len(want) <= len(plan); {
		j := strings.Index(plan[i:], want)
		if j < 0 {
			return false
		}
		end := i + j + len(want)
		if end == len(plan) || !isIdentByte(plan[end]) {
			return true
		}
		i += j + 1
	}
	return false
}

func isIdentByte(b byte) bool {
	return b == '_' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}

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

// The helper's own property, because the two cases above exercise it through
// two guards and would not notice a planHas that returned true for everything.
func TestPlanHasMatchesWholeTokensOnly(t *testing.T) {
	const plan = "SEARCH w USING INDEX ix_work_added_at (added_at<?) | SCAN lm"
	for _, tc := range []struct {
		want string
		ok   bool
	}{
		{"USING INDEX ix_work_added", false}, // the rot this file exists to catch
		{"USING INDEX ix_work_added_at", true},
		{"SCAN lm", true},  // at the very end of the string
		{"SCAN l", false},  // a prefix of an alias
		{"SEARCH w", true}, // followed by a space
	} {
		if got := planHas(plan, tc.want); got != tc.ok {
			t.Errorf("planHas(plan, %q) = %v, want %v", tc.want, got, tc.ok)
		}
	}
}
