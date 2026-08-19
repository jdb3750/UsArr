package store

import (
	"context"
	"database/sql"
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

// THE CONSTRUCTION, for the WHOLE FAMILY. All three predicates that carry a
// correlated subquery derive their inner alias through derivedInnerAlias, and
// the property that closes the leak is the same one in each: the derived alias
// never equals the outer qualifier it must not shadow. Asserting it per helper
// is what stops a future fourth predicate from being added with an empty prefix,
// which is the one way the construction could regain a fixed point.
func TestDerivedInnerAliasesNeverEqualTheOuterQualifier(t *testing.T) {
	for _, fn := range []struct {
		name    string
		derive  func(string) string
		columns []string
	}{
		{"scopeLinkAlias", scopeLinkAlias, []string{"w.id", "m.work_id", "sil.work_id"}},
		{"librarySourceAlias", librarySourceAlias, []string{"l.id", "ls.library_id", "ls.id"}},
		{"searchDocLibraryAlias", searchDocLibraryAlias, []string{"f.rowid", "t.rowid", "sdl.doc_rowid"}},
	} {
		for _, column := range fn.columns {
			alias := fn.derive(column)
			outer, _, _ := strings.Cut(column, ".")
			if alias == outer || len(alias) <= len(outer) {
				t.Errorf("%s(%q) = %q, which does not strictly grow past the outer "+
					"qualifier %q: the collision is representable again and the "+
					"correlation can be shadowed", fn.name, column, alias, outer)
			}
			// And the derivation of the derivation, which is the fixed point a
			// caller reaches by aliasing its own table after the alias.
			if again := fn.derive(alias + ".x"); again == alias {
				t.Errorf("%s has a fixed point at %q", fn.name, alias)
			}
		}
	}
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

// ─────────────────────────────────────────────────────────────────────────────
// libraryVisibilityPredicate — the same defect, in a predicate with TWO arms.
// ─────────────────────────────────────────────────────────────────────────────

// visibleLibraryIDsViaSourceTable answers "which libraries can this caller see?"
// FROM library_source ITSELF, whose alias everywhere in this package is `ls`.
// That outer alias is the whole point of the fixture, not incidental.
func visibleLibraryIDsViaSourceTable(t *testing.T, s *Store, pred string, args []any) []int64 {
	t.Helper()
	return queryIDs(t, s, `SELECT DISTINCT ls.library_id
	                         FROM library_source ls
	                        WHERE `+pred+`
	                        ORDER BY ls.library_id`, args)
}

// visibleLibraryIDsViaLibraryTable answers the same question from `library`,
// which is the ONLY outer that can show the orphan arm: a library with no
// sources at all has no row in library_source to select. It is aliased `ls` for
// the same reason — the shadowing is by NAME, so any outer that happens to
// choose that name collides, whatever table it names.
//
// The reserved Unfiled row is excluded the way listLibrariesSQL excludes it, so
// the assertions below read as §17.8's library list rather than as one row of
// migration 0005's seed.
func visibleLibraryIDsViaLibraryTable(t *testing.T, s *Store, pred string, args []any) []int64 {
	t.Helper()
	return queryIDs(t, s, `SELECT ls.id
	                         FROM library ls
	                        WHERE ls.id <> `+strconv.FormatInt(UnfiledLibraryID, 10)+`
	                          AND `+pred+`
	                        ORDER BY ls.id`, args)
}

func queryIDs(t *testing.T, s *Store, query string, args []any) []int64 {
	t.Helper()
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

// seedUnboundInstance adds a service instance that no library_source names — a
// newly configured service nobody has bound a container to yet. A scope that
// sees ONLY that instance is what separates the orphan arm from the EXISTS arm:
// under it the EXISTS arm is legitimately false for every library, so the
// orphan arm is the only thing left holding library 4 in the result.
func seedUnboundInstance(t *testing.T, s *Store) int64 {
	t.Helper()
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
			   VALUES (3, 'kavita', 'library', 'Kavita Unbound', 'http://kavita3.example', X'00')`)
		return err
	}); err != nil {
		t.Fatalf("seed unbound instance: %v", err)
	}
	return 3
}

// THE LEAK ARM, BOTH HALVES. libraryVisibilityPredicate correlates TWO inner
// library_source subqueries to the outer library-id column by NAME, so an outer
// aliased `ls` shadows the caller's table in BOTH of them and the tie to the
// caller's row is gone. What is left depends on the column name: over an outer
// `library_source ls` the correlation is the literal tautology
// `ls.library_id = ls.library_id`; over an outer `library ls` it is
// `ls.library_id = ls.id`, an unrelated comparison between two columns of the
// INNER table. Either way the subquery no longer says anything about the
// caller's row, and the two arms then degenerate in OPPOSITE directions:
//
//   - the EXISTS arm becomes "is ANY library sourced on a visible instance",
//     which is true as soon as one is — so every library LEAKS;
//   - the NOT EXISTS orphan arm becomes a question about library_source alone,
//     false as soon as it holds a matching row — so the source-less libraries
//     it exists to admit DISAPPEAR.
//
// The second failure is normally masked by the first, because they sit either
// side of an OR. It is visible under a scope naming an instance that holds no
// sources, where the EXISTS arm is legitimately false. See
// docs/REVIEW-LOG.md LS-379.
func TestLibraryVisibilityPredicateSurvivesAnOuterLsAlias(t *testing.T) {
	// The EXISTS arm: library 3 is sourced ONLY on instance 2, which this scope
	// cannot see, so it must not come back.
	t.Run("the EXISTS arm", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)

		narrow := Scope{UserID: 0, InstanceIDs: []int64{1}}
		pred, args := narrow.libraryVisibilityPredicate("ls.library_id")

		got := visibleLibraryIDsViaSourceTable(t, s, pred, args)
		want := []int64{1, 2}
		if formatIDs(got) != formatIDs(want) {
			t.Errorf("SCOPE LEAK: visible libraries = %s, want %s. Library 3 is "+
				"sourced only on instance 2, which this scope cannot see.\n  predicate: %s",
				formatIDs(got), formatIDs(want), pred)
		}
	})

	// The NOT EXISTS orphan arm: library 4 has NO sources — §6.5 rule 5's
	// retained orphan — so it names no instance and stays visible under every
	// non-empty scope, including one whose instance holds no sources at all.
	t.Run("the NOT EXISTS orphan arm", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)
		unbound := seedUnboundInstance(t, s)

		narrow := Scope{UserID: 0, InstanceIDs: []int64{unbound}}
		pred, args := narrow.libraryVisibilityPredicate("ls.id")

		got := visibleLibraryIDsViaLibraryTable(t, s, pred, args)
		want := []int64{4}
		if formatIDs(got) != formatIDs(want) {
			t.Errorf("THE ORPHAN ARM DEGENERATED: visible libraries = %s, want %s. "+
				"Library 4 has no sources, so nothing about it is scoped and it must "+
				"stay visible; libraries 1-3 are sourced only on instances this scope "+
				"cannot see.\n  predicate: %s",
				formatIDs(got), formatIDs(want), pred)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// searchScopePredicate — the same defect over search_doc_library.
// ─────────────────────────────────────────────────────────────────────────────

// visibleDocTitlesViaJunction answers "which documents are in a library this
// caller may search?" FROM the junction itself, aliased `sdl` — the alias
// searchScopePredicate's own subquery uses, and the one searchlibrary.go's plan
// guard names.
func visibleDocTitlesViaJunction(t *testing.T, s *Store, pred string, args []any) []string {
	t.Helper()
	query := `SELECT DISTINCT sd.norm_title
	            FROM search_doc_library sdl
	            JOIN search_doc sd ON sd.rowid = sdl.doc_rowid
	           WHERE ` + pred + `
	           ORDER BY sd.norm_title`

	rows, err := s.DB().Read().QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("QueryContext:\n%s\n%v", query, err)
	}
	defer func() { _ = rows.Close() }()

	var titles []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return titles
}

// THE LEAK ARM. Same shape as the two above: `sdl.doc_rowid = sdl.doc_rowid` is
// a tautology, the semi-join stops filtering, and a search returns documents
// from libraries the caller cannot search — including, in a multi-user install,
// another user's. See docs/REVIEW-LOG.md LS-379.
func TestSearchScopePredicateSurvivesAnOuterSdlAlias(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t,
		searchItem("1", "1", "comic", "Frieren"),
		searchItem("2", "2", "book", "Piranesi"),
	)

	// Only the manga library is searchable.
	pred, args := searchScopePredicate("sdl.doc_rowid", []int64{f.libraries["manga"]})

	got := visibleDocTitlesViaJunction(t, f.store, pred, args)
	want := []string{"frieren"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("SCOPE LEAK: searchable documents = %v, want %v. `Piranesi` is "+
			"filed only in Novels, which this scope does not include.\n  predicate: %s",
			got, want, pred)
	}
}

// FIRING THE GUARD, BOTH ARMS. The two arms above only prove something if they
// are capable of going red, so this renders the shipped predicate, puts the
// hard-coded `ls` back — and nothing else — and watches each assertion catch its
// own failure. It is the pre-fix statement byte for byte, which makes it a
// mutation of the thing that ships rather than a hand-copied lookalike
// (docs/DEVELOPMENT.md §11 rule 1).
func TestLibraryVisibilityPredicateOuterAliasGuardFires(t *testing.T) {
	t.Run("the EXISTS arm", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)

		narrow := Scope{UserID: 0, InstanceIDs: []int64{1}}
		pred, args := narrow.libraryVisibilityPredicate("ls.library_id")
		broken := hardCodeAlias(t, pred, librarySourceAlias("ls.library_id"), "ls")

		got := visibleLibraryIDsViaSourceTable(t, s, broken, args)
		if formatIDs(got) == formatIDs([]int64{1, 2}) {
			t.Fatalf("with the inner alias forced back to `ls` the read still hid "+
				"library 3. The EXISTS arm above is then passing for some other "+
				"reason and is not testing the shadowing defect:\n%s", broken)
		}
		t.Logf("MUTATION CONFIRMED: the hard-coded alias leaks library 3 — "+
			"visible libraries %s, the scoped answer is [1 2]", formatIDs(got))
	})

	t.Run("the NOT EXISTS orphan arm", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)
		unbound := seedUnboundInstance(t, s)

		narrow := Scope{UserID: 0, InstanceIDs: []int64{unbound}}
		pred, args := narrow.libraryVisibilityPredicate("ls.id")
		broken := hardCodeAlias(t, pred, librarySourceAlias("ls.id"), "ls")

		got := visibleLibraryIDsViaLibraryTable(t, s, broken, args)
		if formatIDs(got) == formatIDs([]int64{4}) {
			t.Fatalf("with the inner alias forced back to `ls` the read still kept "+
				"source-less library 4. The orphan arm above is then passing for some "+
				"other reason and is not testing the shadowing defect:\n%s", broken)
		}
		t.Logf("MUTATION CONFIRMED: the hard-coded alias loses library 4 — "+
			"visible libraries %s, the scoped answer is [4]", formatIDs(got))
	})
}

// FIRING THE GUARD. Same mutation, same reason, over search_doc_library.
func TestSearchScopePredicateOuterAliasGuardFires(t *testing.T) {
	f := newSearchFixture(t)
	f.apply(t,
		searchItem("1", "1", "comic", "Frieren"),
		searchItem("2", "2", "book", "Piranesi"),
	)

	pred, args := searchScopePredicate("sdl.doc_rowid", []int64{f.libraries["manga"]})
	broken := hardCodeAlias(t, pred, searchDocLibraryAlias("sdl.doc_rowid"), "sdl")

	got := visibleDocTitlesViaJunction(t, f.store, broken, args)
	if strings.Join(got, ",") == "frieren" {
		t.Fatalf("with the inner alias forced back to `sdl` the read still hid "+
			"`Piranesi`. The arm above is then passing for some other reason and is "+
			"not testing the shadowing defect:\n%s", broken)
	}
	t.Logf("MUTATION CONFIRMED: the hard-coded alias leaks the Novels document — "+
		"searchable documents %v, the scoped answer is [frieren]", got)
}

// hardCodeAlias puts the pre-fix constant back into a rendered predicate, and
// FAILS if the derived alias is not there to replace — a mutation that silently
// matched nothing would leave a guard that can never fire, which is the failure
// mode the whole family exists to prevent.
func hardCodeAlias(t *testing.T, pred, derived, hardCoded string) string {
	t.Helper()
	broken := strings.ReplaceAll(pred, derived, hardCoded)
	if broken == pred {
		t.Fatalf("the derived alias %q is not in the shipped predicate, so this "+
			"mutation tests nothing:\n%s", derived, pred)
	}
	return broken
}
