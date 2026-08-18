package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// The corpus for §17.8's Libraries list.
//
// A REAL MIGRATED DATABASE WITH ROWS IN IT, for recent_test.go's reason: the
// ordering, the Unfiled exclusion, the per-library counts and the scope are all
// invisible at zero rows.
//
// The shape, chosen so that no assertion below can pass by accident:
//
//	library 0  Unfiled     reserved, seeded by migration 0005     (must never appear)
//	library 1  Manga       source on instance 1                   2 members
//	library 2  Books       sources on instances 1 AND 2           3 members
//	library 3  Films       source on instance 2 only              1 member
//	library 4  Loose Ends  NO sources at all — §6.5 rule 5's orphan, 0 members
//
// Instance 1 and instance 2 therefore split the libraries three ways — one each
// and one shared — so an instance scope that silently did nothing would still
// look plausible against a corpus where every library named every instance.
//
// sort_order is deliberately NOT id order: Films sorts first. An ordering
// assertion against a list that is already in id order proves nothing.
// ─────────────────────────────────────────────────────────────────────────────

func seedLibrariesCorpus(t *testing.T, s *Store) {
	t.Helper()
	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'kavita', 'library', 'Kavita Manga', 'http://kavita.example', X'00')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (2, 'kavita', 'library', 'Kavita Books', 'http://kavita2.example', X'00')`,

		`INSERT INTO library (id, user_id, name, slug, kind, sort_order, enabled, include_in_search)
		   VALUES (1, 0, 'Manga', 'manga', 'comic', 10, 1, 1)`,
		`INSERT INTO library (id, user_id, name, slug, kind, sort_order, enabled, include_in_search)
		   VALUES (2, 0, 'Books', 'books', 'book', 20, 1, 0)`,
		`INSERT INTO library (id, user_id, name, slug, kind, sort_order, enabled, include_in_search)
		   VALUES (3, 0, 'Films', 'films', 'movie', 5, 0, 1)`,
		`INSERT INTO library (id, user_id, name, slug, kind, sort_order, enabled, include_in_search)
		   VALUES (4, 0, 'Loose Ends', 'loose-ends', 'book', 30, 1, 1)`,

		`INSERT INTO library_source
		   (id, library_id, service_instance_id, container_kind, container_ref,
		    container_identity, is_metadata_authority)
		 VALUES (1, 1, 1, 'remote_library', '11', 'Manga', 1)`,
		`INSERT INTO library_source
		   (id, library_id, service_instance_id, container_kind, container_ref,
		    container_identity, is_metadata_authority)
		 VALUES (2, 2, 1, 'remote_library', '12', 'Books', 1)`,
		`INSERT INTO library_source
		   (id, library_id, service_instance_id, container_kind, container_ref,
		    container_identity, is_metadata_authority)
		 VALUES (3, 2, 2, 'remote_library', '21', 'More Books', 0)`,
		`INSERT INTO library_source
		   (id, library_id, service_instance_id, container_kind, container_ref,
		    container_identity, is_metadata_authority)
		 VALUES (4, 3, 2, 'remote_library', '22', 'Films', 1)`,
	}

	// Works, their links and their membership. Work n lives on the instance its
	// library's first source names, so the scope has something consistent to
	// filter.
	members := []struct {
		work int
		kind string
		lib  int
		inst int
	}{
		{1, "comic", 1, 1},
		{2, "comic", 1, 1},
		{3, "book", 2, 1},
		{4, "book", 2, 1},
		{5, "book", 2, 2},
		{6, "movie", 3, 2},
	}
	for _, m := range members {
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO work
			  (id, kind, title, sort_title, normalized_title, added_at)
			VALUES (%d, '%s', 'w%02d', 'w%02d', 'w%02d', '2026-08-10 10:00:00')`,
			m.work, m.kind, m.work, m.work, m.work))
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO service_item_link
			   (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			 VALUES (%d, %d, 'r%d', 'series', '2026-08-16 12:00:00')`, m.inst, m.work, m.work))
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (%d, 'w%02d', %d)`,
			m.lib, m.work, m.work))
	}

	// And one member row in the RESERVED library, because the derivation files
	// stranded works there. If the Unfiled exclusion ever leaked, this is the
	// row that would make it visible as a count rather than only as a name.
	stmts = append(stmts,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (0, 'w01', 1)`)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

func libraryNames(ls []Library) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Name)
	}
	return out
}

func libraryByName(t *testing.T, ls []Library, name string) Library {
	t.Helper()
	for _, l := range ls {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("library %q is not in %v", name, libraryNames(ls))
	return Library{}
}

// The list itself: what §17.8's row view needs, in sort_order.
func TestListLibrariesRendersTheRowView(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	got, err := s.ListLibraries(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}

	// sort_order, not id order: Films is 5, Manga 10, Books 20, Loose Ends 30.
	want := []string{"Films", "Manga", "Books", "Loose Ends"}
	if strings.Join(libraryNames(got), " | ") != strings.Join(want, " | ") {
		t.Fatalf("libraries are not in sort_order:\n  got:  %v\n  want: %v",
			libraryNames(got), want)
	}

	manga := libraryByName(t, got, "Manga")
	if manga.Slug != "manga" || manga.Kind != "comic" {
		t.Errorf("Manga's identity is wrong: %+v", manga)
	}
	if !manga.Enabled || !manga.IncludeInSearch {
		t.Errorf("Manga's visibility flags are wrong: %+v", manga)
	}
	if manga.ItemCount != 2 {
		t.Errorf("Manga's item count = %d, want 2", manga.ItemCount)
	}

	// The two flags are read from the two columns and not from each other:
	// Books is enabled and OUT of search, Films is disabled and IN it.
	books := libraryByName(t, got, "Books")
	if !books.Enabled || books.IncludeInSearch {
		t.Errorf("Books: enabled=%v include_in_search=%v, want true/false",
			books.Enabled, books.IncludeInSearch)
	}
	films := libraryByName(t, got, "Films")
	if films.Enabled || !films.IncludeInSearch {
		t.Errorf("Films: enabled=%v include_in_search=%v, want false/true",
			films.Enabled, films.IncludeInSearch)
	}
	if books.ItemCount != 3 || films.ItemCount != 1 {
		t.Errorf("counts: Books=%d (want 3), Films=%d (want 1)", books.ItemCount, films.ItemCount)
	}

	// §6.5 rule 5's retained orphan: no sources, no members, still listed.
	loose := libraryByName(t, got, "Loose Ends")
	if len(loose.Sources) != 0 || loose.ItemCount != 0 {
		t.Errorf("the source-less library did not come back empty: %+v", loose)
	}
}

// THE RESERVED ROW. Migration 0005: library 0, "Unfiled", is "never listed on
// the Libraries screen, never offered in the scope chip, never proposed". This
// read is the Libraries screen.
func TestListLibrariesNeverReturnsUnfiled(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	// The row is really there, so the exclusion has something to exclude —
	// otherwise this test passes against a database that simply lacks it.
	var name string
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT name FROM library WHERE id = ?`, UnfiledLibraryID).Scan(&name); err != nil {
		t.Fatalf("the reserved row is not in the database, so this test asserts nothing: %v", err)
	}
	if name != "Unfiled" {
		t.Fatalf("library %d is %q, not the reserved row", UnfiledLibraryID, name)
	}

	for _, scope := range []Scope{
		OwnerScope(0),
		{UserID: 0, InstanceIDs: []int64{1, 2}},
	} {
		got, err := s.ListLibraries(t.Context(), scope)
		if err != nil {
			t.Fatalf("ListLibraries: %v", err)
		}
		for _, l := range got {
			if l.ID == UnfiledLibraryID || l.Name == "Unfiled" {
				t.Errorf("the reserved Unfiled row reached the Libraries list: %+v", l)
			}
		}
	}
}

// The source chips: the two service_instance columns §17.8 renders, the
// container the upstream reported, and the per-source health field.
func TestListLibrariesCarriesItsSources(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	got, err := s.ListLibraries(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}

	// The flagship shape: ONE library, TWO sources on two instances. §17.8's
	// "two Radarrs → one Movies library with two sources" is what makes the
	// per-source badge render at all.
	books := libraryByName(t, got, "Books")
	if len(books.Sources) != 2 {
		t.Fatalf("Books has %d sources, want 2: %+v", len(books.Sources), books.Sources)
	}
	first, second := books.Sources[0], books.Sources[1]
	if first.ServiceName != "Kavita Manga" || second.ServiceName != "Kavita Books" {
		t.Errorf("source instances are wrong: %q, %q", first.ServiceName, second.ServiceName)
	}
	if first.ServiceKind != "kavita" {
		t.Errorf("service kind = %q, want kavita", first.ServiceKind)
	}
	if first.ContainerKind != "remote_library" || first.ContainerRef != "12" {
		t.Errorf("container is wrong: %+v", first)
	}
	if !first.ContainerIdentity.Valid || first.ContainerIdentity.String != "Books" {
		t.Errorf("container identity is wrong: %+v", first.ContainerIdentity)
	}
	if !first.IsMetadataAuthority || second.IsMetadataAuthority {
		t.Errorf("metadata authority landed on the wrong source: %v, %v",
			first.IsMetadataAuthority, second.IsMetadataAuthority)
	}
	if first.MissingSince.Valid {
		t.Errorf("missing_since is set on a healthy source: %+v", first.MissingSince)
	}
}

// missing_since is read, even though nothing in the tree SETS it — see
// LibrarySource's own comment. This writes the column directly, which is the
// only way to reach the state today, so that the field is proven to travel
// rather than assumed to.
func TestListLibrariesReportsAMissingSource(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE library_source SET missing_since = '2026-08-17 09:00:00' WHERE id = 3`)
		return err
	}); err != nil {
		t.Fatalf("mark missing: %v", err)
	}

	got, err := s.ListLibraries(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	books := libraryByName(t, got, "Books")
	if len(books.Sources) != 2 {
		t.Fatalf("Books has %d sources, want 2", len(books.Sources))
	}
	if books.Sources[1].MissingSince.String != "2026-08-17 09:00:00" {
		t.Errorf("the missing source did not report its date: %+v", books.Sources[1])
	}
	if books.Sources[0].MissingSince.Valid {
		t.Errorf("the healthy source in the same library reports missing: %+v", books.Sources[0])
	}
}

// A soft-deleted instance drops out of the source list, matching
// listServiceInstances. Recorded as behaviour rather than left to be discovered
// — see librarySourcesSQL for the consequence.
func TestListLibrariesDropsSourcesOnASoftDeletedInstance(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET deleted_at = '2026-08-17 09:00:00' WHERE id = 2`)
		return err
	}); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	got, err := s.ListLibraries(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	books := libraryByName(t, got, "Books")
	if len(books.Sources) != 1 || books.Sources[0].ServiceInstanceID != 1 {
		t.Errorf("the deleted instance is still a source of Books: %+v", books.Sources)
	}
	// Films' only source was on instance 2, so it now lists with none. The
	// library itself is still returned: §6.5 rule 5 never auto-deletes.
	films := libraryByName(t, got, "Films")
	if len(films.Sources) != 0 {
		t.Errorf("Films kept a source on a deleted instance: %+v", films.Sources)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// THE ACCESS SCOPE. store.go rule 2, and the reason this read carries a Scope at
// all: a library row NAMES the service instances behind it, so listing one whose
// sources are all invisible publishes the topology of the install.
// ─────────────────────────────────────────────────────────────────────────────

func TestListLibrariesScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)
	ctx := t.Context()

	all, err := s.ListLibraries(ctx, OwnerScope(0))
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("the owner sees %d libraries, want 4: %v", len(all), libraryNames(all))
	}

	// Instance 1 only. Films' one source is on instance 2, so Films is gone.
	// Loose Ends has no sources at all and stays — it names no instance.
	one, err := s.ListLibraries(ctx, Scope{UserID: 0, InstanceIDs: []int64{1}})
	if err != nil {
		t.Fatalf("instance 1: %v", err)
	}
	wantOne := []string{"Manga", "Books", "Loose Ends"}
	if strings.Join(libraryNames(one), " | ") != strings.Join(wantOne, " | ") {
		t.Fatalf("instance 1's scope:\n  got:  %v\n  want: %v", libraryNames(one), wantOne)
	}
	if len(one) == len(all) {
		t.Fatalf("the scope parameter changed nothing: %d libraries either way", len(one))
	}

	// And the hidden instance is not named INSIDE a library that stayed. Books
	// is visible through instance 1; its instance-2 source must not travel.
	books := libraryByName(t, one, "Books")
	if len(books.Sources) != 1 || books.Sources[0].ServiceInstanceID != 1 {
		t.Fatalf("a source on an invisible instance was published anyway: %+v", books.Sources)
	}
	for _, l := range one {
		for _, src := range l.Sources {
			if src.ServiceInstanceID == 2 || src.ServiceName == "Kavita Books" {
				t.Errorf("library %q names instance 2, which this scope hides: %+v", l.Name, src)
			}
		}
	}

	// The ITEM COUNT is scoped too. Books holds works 3, 4 (instance 1) and 5
	// (instance 2); under instance 1 it must count 2, not 3. A count that
	// included work 5 would be a rollup over an instance the caller cannot see.
	if books.ItemCount != 2 {
		t.Errorf("Books' item count under instance 1 = %d, want 2 (work 5 is on instance 2)",
			books.ItemCount)
	}

	// The complement. Manga's only source is on instance 1, so it goes.
	two, err := s.ListLibraries(ctx, Scope{UserID: 0, InstanceIDs: []int64{2}})
	if err != nil {
		t.Fatalf("instance 2: %v", err)
	}
	wantTwo := []string{"Films", "Books", "Loose Ends"}
	if strings.Join(libraryNames(two), " | ") != strings.Join(wantTwo, " | ") {
		t.Fatalf("instance 2's scope:\n  got:  %v\n  want: %v", libraryNames(two), wantTwo)
	}
	if b := libraryByName(t, two, "Books"); b.ItemCount != 1 {
		t.Errorf("Books' item count under instance 2 = %d, want 1", b.ItemCount)
	}

	// FAIL CLOSED. No visible instances means no rows, never all rows — and
	// that includes the source-less library, because the whole read is refused
	// rather than the source join alone.
	none, err := s.ListLibraries(ctx, Scope{UserID: 0})
	if err != nil {
		t.Fatalf("empty scope: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("an empty visible set returned %d libraries: %v", len(none), libraryNames(none))
	}
}

// FIRING THE SCOPE GUARD. A scope check that has never been watched failing is
// indistinguishable from no scope check, so each of the three predicates is made
// INEFFECTIVE in turn — by rewriting the shipped statement, not a lookalike —
// and the assertion above is re-run against the result.
func TestListLibrariesScopeGuardFires(t *testing.T) {
	// Arm 1 — the LIBRARY visibility predicate is neutralised, so a library
	// whose only source is on a hidden instance comes back anyway.
	t.Run("library visibility predicate neutralised", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)

		scope := Scope{UserID: 0, InstanceIDs: []int64{1}}
		query, args := listLibrariesSQL(scope)
		broken, brokenArgs := neutraliseLibraryVisibility(t, scope, query, args)

		names := libraryNamesFrom(t, s, broken, brokenArgs)
		if !contains(names, "Films") {
			t.Fatalf("the break was a no-op: instance 1's scope still hides Films (%v)", names)
		}
		t.Logf("guard fired with the library-visibility predicate neutralised: %v", names)
	})

	// Arm 2 — the SOURCE predicate is neutralised, so a library that is
	// legitimately visible publishes a source on an instance the scope hides.
	t.Run("source instance predicate neutralised", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)

		scope := Scope{UserID: 0, InstanceIDs: []int64{1}}
		query, _ := librarySourcesSQL(scope, []int64{2})
		broken := strings.Replace(query,
			"AND ls.service_instance_id IN (?)", "AND 1=1", 1)
		if broken == query {
			t.Fatalf("the shipped source statement no longer contains the instance "+
				"predicate this arm rewrites, so it asserts nothing:\n%s", query)
		}
		// The instance argument goes with the predicate that consumed it.
		rows, err := s.DB().Read().QueryContext(t.Context(), broken, int64(2))
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer func() { _ = rows.Close() }()
		var leaked []string
		for rows.Next() {
			var libraryID, srcID, instID int64
			var name, kind, ck, ref string
			var identity, missing sql.NullString
			var authority bool
			if err := rows.Scan(&libraryID, &srcID, &instID, &name, &kind,
				&ck, &ref, &identity, &authority, &missing); err != nil {
				t.Fatalf("scan: %v", err)
			}
			if instID == 2 {
				leaked = append(leaked, name)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		if len(leaked) == 0 {
			t.Fatal("the break was a no-op: no hidden instance came back")
		}
		t.Logf("guard fired with the source instance predicate neutralised: %v", leaked)
	})

	// Arm 3 — the COUNT's scope is neutralised, so the item count silently
	// rolls up works from an instance the caller cannot see. This is the arm
	// that matters most, because a wrong number is the failure that renders
	// without looking wrong.
	t.Run("item count scope neutralised", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)

		scope := Scope{UserID: 0, InstanceIDs: []int64{1}}
		query, args := listLibrariesSQL(scope)
		// The count's EXISTS is the first scope fragment in the statement.
		const countExists = `WHERE m.library_id = l.id AND EXISTS`
		if !strings.Contains(query, countExists) {
			t.Fatalf("the shipped statement no longer scopes the count the way this arm "+
				"rewrites, so it asserts nothing:\n%s", query)
		}
		broken := strings.Replace(query, countExists,
			`WHERE m.library_id = l.id AND 1=1 AND NOT EXISTS`, 1)
		if broken == query {
			t.Fatal("the break was a no-op: the statement is unchanged")
		}
		rows, err := s.DB().Read().QueryContext(t.Context(), broken, args...)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		got, err := scanLibraries(rows)
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		books := libraryByName(t, got, "Books")
		if books.ItemCount == 2 {
			t.Fatal("the break was a no-op: Books still counts 2 under instance 1")
		}
		t.Logf("guard fired with the count's scope neutralised: Books counts %d, "+
			"the scoped answer is 2", books.ItemCount)
	})
}

// neutraliseLibraryVisibility rewrites the shipped statement's library
// visibility predicate to a tautology, and FAILS if the fragment it looks for is
// no longer there — a break that silently matched nothing would make the arm a
// permanent pass.
func neutraliseLibraryVisibility(
	t *testing.T, scope Scope, query string, args []any,
) (string, []any) {
	t.Helper()
	pred, predArgs := scope.libraryVisibilityPredicate("l.id")
	if !strings.Contains(query, pred) {
		t.Fatalf("the shipped statement does not contain the visibility predicate:\n%s", query)
	}
	broken := strings.Replace(query, pred, "1=1", 1)
	// Drop the arguments the removed predicate consumed. They are the LAST
	// ones, because listLibrariesSQL appends them last.
	return broken, args[:len(args)-len(predArgs)]
}

func libraryNamesFrom(t *testing.T, s *Store, query string, args []any) []string {
	t.Helper()
	rows, err := s.DB().Read().QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	got, err := scanLibraries(rows)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return libraryNames(got)
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// QUERY PLANS. docs/DEVELOPMENT.md §5: deterministic, hardware-independent, and
// a merge gate. Wall-clock budgets are `make bench`'s and are not.
// ─────────────────────────────────────────────────────────────────────────────

func listLibrariesPlan(t *testing.T, s *Store, scope Scope) string {
	t.Helper()
	query, args := listLibrariesSQL(scope)
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

func librarySourcesPlan(t *testing.T, s *Store, scope Scope) string {
	t.Helper()
	query, args := librarySourcesSQL(scope, []int64{1, 2, 3})
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// listLibrariesPlanFaults is the assertion both plan tests run, so the guard and
// its firing execute the same code.
//
// WHAT IT ASSERTS: the two seeks that scale. The item count is the one leg whose
// cost grows with the catalogue — library_member holds one row per item — so a
// scan there is the regression this exists to catch, and the library table's own
// leg must stay a seek on a user_id index rather than a table scan.
//
// WHAT IT DELIBERATELY DOES NOT ASSERT: the absence of a temp b-tree. The
// ordering is `sort_order, name` and migration 0005 declares no index leading
// with sort_order, so the sort is expected — see listLibrariesSQL and
// TestListLibrariesOrderingSortIsIntentional, which records it as a decision
// rather than letting it pass unremarked. It also does not assert COVERING, per
// schema.md §1.
func listLibrariesPlanFaults(plan string) []string {
	var faults []string
	if strings.Contains(plan, "SCAN library") && !strings.Contains(plan, "SCAN library USING") {
		faults = append(faults, "degraded to a table scan of library")
	}
	if !strings.Contains(plan, "SEARCH l USING INDEX") {
		faults = append(faults, "the library leg is not an index seek")
	}
	if !strings.Contains(plan, "SEARCH m USING") || !strings.Contains(plan, "ux_libmem_identity") {
		faults = append(faults,
			"the item count is not a seek on ux_libmem_identity, so it walks library_member")
	}
	if !strings.Contains(plan, "(library_id=?)") {
		faults = append(faults, "the item count does not constrain library_id")
	}
	if strings.Contains(plan, "SCAN m") {
		faults = append(faults, "the item count degraded to a scan of library_member")
	}
	return faults
}

func TestListLibrariesPlanIsSeeks(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	joined := listLibrariesPlan(t, s, OwnerScope(0))
	const wantCount = "SEARCH m USING COVERING INDEX ux_libmem_identity (library_id=?)"
	if !strings.Contains(joined, wantCount) {
		t.Errorf("the item count is not the measured seek.\n  got:  %s\n  want: %s",
			joined, wantCount)
	}
	if faults := listLibrariesPlanFaults(joined); len(faults) > 0 {
		t.Errorf("the owner plan is wrong:\n  plan: %s\n  %s", joined, strings.Join(faults, "\n  "))
	}

	// The SCOPED plan, which is the one a multi-user install runs: two more
	// correlated subqueries over library_source and an EXISTS over
	// service_item_link, and none of them may displace the seeks above.
	scoped := listLibrariesPlan(t, s, Scope{UserID: 0, InstanceIDs: []int64{1, 2}})
	if faults := listLibrariesPlanFaults(scoped); len(faults) > 0 {
		t.Errorf("the scoped plan is wrong:\n  plan: %s\n  %s", scoped, strings.Join(faults, "\n  "))
	}
	if !strings.Contains(scoped, "SEARCH sil EXISTS USING INDEX ix_sil_work") {
		t.Errorf("the count's scope EXISTS is not a seek on ix_sil_work: %s", scoped)
	}
	if strings.Contains(scoped, "SCAN ls") {
		t.Errorf("the library visibility EXISTS degraded to a scan of library_source: %s", scoped)
	}
}

func TestLibrarySourcesPlanIsASeek(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	joined := librarySourcesPlan(t, s, OwnerScope(0))
	const want = "SEARCH ls USING INDEX sqlite_autoindex_library_source_1 (library_id=?)"
	if !strings.Contains(joined, want) {
		t.Errorf("the source read is not a seek on library_source's own unique key.\n"+
			"  got:  %s\n  want: %s\n"+
			"migration 0005 declares UNIQUE (library_id, service_instance_id, container_kind, "+
			"container_ref), whose implicit index leads with library_id; ix_libsrc_instance "+
			"leads with the instance and ux_libsrc_authority is partial, so neither serves "+
			"this lookup.", joined, want)
	}
	if strings.Contains(joined, "SCAN ls") {
		t.Errorf("the source read degraded to a scan of library_source: %s", joined)
	}
	if !strings.Contains(joined, "SEARCH si USING INTEGER PRIMARY KEY") {
		t.Errorf("the service_instance join is not a rowid lookup: %s", joined)
	}
}

// FIRING THE PLAN GUARD. Three arms, each breaking a different thing the guard
// protects and each running the SAME listLibrariesPlanFaults the tests above
// run.
func TestListLibrariesPlanGuardFires(t *testing.T) {
	// Arm 1 — the item count's index seek is disqualified with SQLite's unary
	// plus, which is the documented way to say "not usable as an index key"
	// (<https://sqlite.org/optoverview.html>). This is the regression that
	// matters: library_member holds one row per item, so a scan here is the
	// Libraries screen walking the whole catalogue once per library.
	t.Run("item count loses its index", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)

		query, args := listLibrariesSQL(OwnerScope(0))
		broken := strings.Replace(query, "WHERE m.library_id = l.id", "WHERE +m.library_id = l.id", 1)
		if broken == query {
			t.Fatalf("the shipped statement no longer contains `WHERE m.library_id = l.id`, "+
				"so this arm is asserting against nothing:\n%s", query)
		}
		joined := planOf(t, s, broken, args)
		faults := listLibrariesPlanFaults(joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan whose count scans library_member:\n  %s", joined)
		}
		t.Logf("guard fired with the count's index disqualified:\n  plan:   %s\n  faults: %v",
			joined, faults)
	})

	// Arm 2 — the index the count uses is dropped outright, which is the shape
	// a later migration's tidy-up would take. The PK still leads with
	// library_id, so this is the arm that proves the guard names the index it
	// means rather than merely noticing a scan.
	t.Run("ux_libmem_identity dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)
		if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DROP INDEX ux_libmem_identity`)
			return err
		}); err != nil {
			t.Fatalf("drop index: %v", err)
		}

		joined := listLibrariesPlan(t, s, OwnerScope(0))
		faults := listLibrariesPlanFaults(joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan with ux_libmem_identity dropped:\n  %s", joined)
		}
		t.Logf("guard fired with ux_libmem_identity dropped:\n  plan:   %s\n  faults: %v",
			joined, faults)
	})

	// Arm 3 — a real table scan of library, so the scan half of the guard is
	// executed rather than merely written down.
	t.Run("library leg degrades to a scan", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)

		query, args := listLibrariesSQL(OwnerScope(0))
		broken := strings.Replace(query, "AND l.user_id IN (?, ?)", "AND +l.user_id IN (?, ?)", 1)
		if broken == query {
			t.Fatalf("the shipped statement no longer contains the user predicate this arm "+
				"rewrites:\n%s", query)
		}
		joined := planOf(t, s, broken, args)
		faults := listLibrariesPlanFaults(joined)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan that scans library:\n  %s", joined)
		}
		t.Logf("guard fired on a table scan of library:\n  plan:   %s\n  faults: %v", joined, faults)
	})
}

func planOf(t *testing.T, s *Store, query string, args []any) string {
	t.Helper()
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// The ordering sort is EXPECTED, and it is recorded here so it cannot pass
// unremarked — the same posture TestServiceInstanceListScanIsIntentional takes
// for service_instance, and for the same reason: migration 0005 declares no
// index leading with sort_order, a homelab has single-digit libraries, and this
// is a settings screen. If an ordering index is ever added it is a NEW
// migration, and this test is what makes that a deliberate edit.
func TestListLibrariesOrderingSortIsIntentional(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)

	joined := listLibrariesPlan(t, s, OwnerScope(0))
	if !strings.Contains(joined, "TEMP B-TREE FOR ORDER BY") {
		t.Errorf("the `sort_order, name` ordering is no longer a sort: %s\n"+
			"If an ordering index was added, it belongs in a NEW migration (0005 is merged "+
			"and merged migrations are never edited), and this test must be updated in the "+
			"same change.", joined)
	}
}
