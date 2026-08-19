package store

import (
	"sort"
	"strconv"
	"strings"
	"testing"
)

// visibleWorkIDsViaLinkTable runs the read a future caller would naturally
// write: "which works can this caller see?", answered FROM THE LINK TABLE
// ITSELF, whose obvious alias is `sil` — the same alias every other query in
// this package gives service_item_link.
//
// The outer alias is the whole point of the fixture and is not incidental: it is
// the collision LS-379 found latent in workVisibilityPredicate.
func visibleWorkIDsViaLinkTable(t *testing.T, s *Store, pred string, args []any) []int64 {
	t.Helper()

	query := `SELECT DISTINCT sil.work_id
	            FROM service_item_link sil
	           WHERE sil.deleted_at IS NULL
	             AND ` + pred + `
	           ORDER BY sil.work_id`

	rows, err := s.DB().Read().QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("QueryContext:\n%s\n%v", query, err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func formatIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// THE LEAK ARM. workVisibilityPredicate correlates its inner service_item_link
// to the outer work-id column by NAME. If the inner alias can equal the outer
// one, the inner shadows the outer, `sil.work_id = sil.work_id` is a tautology,
// and the EXISTS stops filtering — the read returns every row, including rows on
// instances outside the caller's scope. Silently: no error, no empty result, no
// syntax fault. See docs/REVIEW-LOG.md LS-379.
//
// The corpus is worklinks_test.go's: work 4 lives ONLY on instance 3, which the
// narrowed scope below cannot see.
func TestWorkVisibilityPredicateSurvivesAnOuterSilAlias(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)

	narrow := Scope{UserID: 2, InstanceIDs: []int64{1, 2}}
	pred, args := narrow.workVisibilityPredicate("sil.work_id")

	got := visibleWorkIDsViaLinkTable(t, s, pred, args)

	// Works 1, 2, 3 and 5 all carry a live link on instance 1 or 2. Work 4 does
	// not — it is reachable only through instance 3 — and work 6 has no links.
	want := []int64{1, 2, 3, 5}
	for _, id := range got {
		if id == 4 {
			t.Errorf("SCOPE LEAK: work 4 came back under a scope that cannot see "+
				"instance 3, its only instance.\n  got:  %s\n  want: %s\n  predicate: %s",
				formatIDs(got), formatIDs(want), pred)
		}
	}
	if formatIDs(got) != formatIDs(want) {
		t.Errorf("visible works = %s, want %s.\n  predicate: %s",
			formatIDs(got), formatIDs(want), pred)
	}
}

// FIRING THE GUARD. The arm above only proves something if it is capable of
// going red, and a scope test that passes against the DEFECT is not a test. So
// this one renders the shipped predicate, puts the hard-coded `sil` back — and
// nothing else — and watches the same assertion catch the leak.
//
// It is the pre-fix statement byte for byte, which is what makes it a mutation
// of the thing that ships rather than a hand-copied lookalike
// (docs/DEVELOPMENT.md §11 rule 1).
func TestWorkVisibilityPredicateOuterAliasGuardFires(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)

	narrow := Scope{UserID: 2, InstanceIDs: []int64{1, 2}}
	pred, args := narrow.workVisibilityPredicate("sil.work_id")

	broken := strings.ReplaceAll(pred, scopeLinkAlias("sil.work_id"), "sil")
	if broken == pred {
		t.Fatalf("the derived alias is not in the shipped predicate, so this "+
			"mutation tests nothing:\n%s", pred)
	}

	got := visibleWorkIDsViaLinkTable(t, s, broken, args)
	if formatIDs(got) == formatIDs([]int64{1, 2, 3, 5}) {
		t.Fatalf("with the inner alias forced back to `sil` the read still hid "+
			"work 4. The arm above is then passing for some other reason and is "+
			"not testing the shadowing defect:\n%s", broken)
	}
	t.Logf("MUTATION CONFIRMED: the hard-coded alias leaks work 4 — visible works %s",
		formatIDs(got))
}

// THE CONSTRUCTION, not the rendering. scopeLinkAlias closes the leak by making
// the collision unrepresentable rather than by guarding against one value, so
// what is asserted is the property: the derived alias never equals the outer
// qualifier it must not shadow — including for the alias that caused LS-379, for
// an alias that tries to be the derivation's own fixed point, and for an
// unqualified column.
func TestScopeLinkAliasNeverEqualsTheOuterQualifier(t *testing.T) {
	for _, column := range []string{
		"w.id",       // recent, browse, facets, imageassets
		"m.work_id",  // libraries
		"sd.work_id", // searchlibrary
		"sil.work_id",
		"sil_sil.work_id",     // the derivation's would-be fixed point
		"sil_sil_sil.work_id", // and the next one
		"work_id",             // unqualified: nothing to shadow
	} {
		alias := scopeLinkAlias(column)
		outer, _, _ := strings.Cut(column, ".")
		if alias == outer {
			t.Errorf("scopeLinkAlias(%q) = %q, which SHADOWS the outer qualifier %q: "+
				"the correlation collapses to a tautology and the scope stops filtering",
				column, alias, outer)
		}
		if len(alias) <= len(outer) {
			t.Errorf("scopeLinkAlias(%q) = %q is not strictly longer than the outer "+
				"qualifier %q. Strict growth is what makes equality impossible; without "+
				"it the construction has a fixed point and the collision is representable "+
				"again", column, alias, outer)
		}
	}
}
