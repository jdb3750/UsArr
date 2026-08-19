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
	// The alias is DERIVED from the correlated column (scopeLinkAlias, LS-379),
	// so the assertion derives it too rather than hard-coding a name that a
	// literal `sil` would go on matching by prefix while covering nothing.
	wantSil := "SEARCH " + scopeLinkAlias("m.work_id") + " EXISTS USING INDEX ix_sil_work"
	if !strings.Contains(scoped, wantSil) {
		t.Errorf("the count's scope EXISTS is not a seek on ix_sil_work.\n  got:  %s\n  want: %s",
			scoped, wantSil)
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
		// ⚠️ dropIndexesAndConfirm, never a bare DROP: listLibrariesPlan below
		// EXPLAINs on the READ pool, and MEASURED on this tree, a read-pool
		// connection that has already planned this statement keeps answering
		// with the PRE-DROP plan indefinitely — repeating the EXPLAIN does not
		// shake it loose. This arm survives a bare drop today only because
		// nothing plans before it; that is an accident of ordering, and one seed
		// refactor that plans on its way past turns this arm into a measurement
		// of a schema that no longer exists. The confirming read inside is both
		// the proof and the cure. Undocumented behaviour, characterised
		// empirically — the helper in store_test.go carries the measurement and
		// its caveats.
		dropIndexesAndConfirm(t, s, "ux_libmem_identity")

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

// ─────────────────────────────────────────────────────────────────────────────
// The THIRD statement's plan guard — libraryCompletenessSQL.
//
// It was the one statement 1bc400a added without one, and its author flagged the
// gap themselves. What follows is what MEASURING it produced, which is not quite
// what that flag guessed.
//
// MEASURED — SQLite 3.53.4 (github.com/ncruces/go-sqlite3), this schema, the
// corpus below, NO ANALYZE. Owner scope, indented by EXPLAIN QUERY PLAN's parent
// column:
//
//	SEARCH ls USING COVERING INDEX sqlite_autoindex_library_source_1 (library_id=?)
//	SEARCH si USING INTEGER PRIMARY KEY (rowid=?)
//	SEARCH r USING INTEGER PRIMARY KEY (rowid=?)
//	CORRELATED SCALAR SUBQUERY 1
//	  SEARCH r2 USING INDEX ix_sync_report_instance (service_instance_id=?)
//	  USE TEMP B-TREE FOR ORDER BY
//	USE TEMP B-TREE FOR LAST TERM OF ORDER BY
//
// Instance scope, which is the multi-user plan, differs in three places only: si
// leads the join, ls picks up its second key column
// (`library_id=? AND service_instance_id=?`), and the outer sort is a full
// `USE TEMP B-TREE FOR ORDER BY` rather than a last-term one.
//
// ⚠️ THERE ARE TWO TEMP B-TREES, AND ONE OF THEM IS INSIDE THE CORRELATED
// SUBQUERY. That is worse than "it seeks the instance and then filters kind and
// remote_id within those rows", which is how the gap was flagged.
// ix_sync_report_instance is (service_instance_id, created_at DESC) and the
// subquery orders by `r2.id DESC`, so the newest-verdict pick cannot come off
// the index at all: for every library_source row, SQLite walks that instance's
// sync_report rows, filters kind/remote_kind/remote_id with no index help, and
// SORTS what survives to take one row. sync_report is append-only and this kind
// writes one row per container per import, so the walked set grows with import
// COUNT forever — it is not bounded by the size of the library.
//
// ⚠️ AND THE INDEX IS WHAT COSTS THE SORT. With ix_sync_report_instance DROPPED
// the subquery plans as a bare `SCAN r2` and the sort DISAPPEARS — a table scan
// visits rows in rowid order, which is `r2.id` order, so `ORDER BY r2.id DESC`
// comes free. Arm 1 of TestLibraryCompletenessPlanGuardFires prints exactly
// that. So the two shapes trade a bounded walk for a sort, and neither is the
// one this read wants; whoever scopes the index above should know that dropping
// the existing one is not the alternative.
//
// This was PINNED, NOT FIXED when it was first measured. The fix named there was
// an index leading with (service_instance_id, kind, remote_kind, remote_id) and
// trailing id — "a new migration and a scoping decision that is not a test's to
// take". What the guard bought meanwhile was that the plan could not get WORSE
// than the paragraphs above without a test going red.
//
// ─── RESOLVED by migration 0011, 2026-08-19 ─────────────────────────────────
//
// Everything above is the finding as it was measured, and it is kept verbatim so
// the history stays readable: what was measured, what was done about it, and
// when. What follows is what was done.
//
// internal/db/migrations/00011_sync_report_container_latest_index.sql creates
// exactly the index the paragraph above scoped:
//
//	CREATE INDEX ix_sync_report_container_latest
//	  ON sync_report(service_instance_id, kind, remote_kind, remote_id, id);
//
// RE-MEASURED, same engine, same schema, same corpus, still NO ANALYZE. Owner
// scope:
//
//	SEARCH ls USING COVERING INDEX sqlite_autoindex_library_source_1 (library_id=?)
//	SEARCH si USING INTEGER PRIMARY KEY (rowid=?)
//	SEARCH r USING INTEGER PRIMARY KEY (rowid=?)
//	CORRELATED SCALAR SUBQUERY 1
//	  SEARCH r2 USING COVERING INDEX ix_sync_report_container_latest (service_instance_id=? AND kind=? AND remote_kind=? AND remote_id=?)
//	USE TEMP B-TREE FOR LAST TERM OF ORDER BY
//
// Instance scope differs in the same three places it always did: si leads, ls
// picks up its second key column, and the outer sort is a full
// `USE TEMP B-TREE FOR ORDER BY`.
//
// ✅ THE SUBQUERY'S SORT IS GONE, AND SO IS ITS ROW FETCH. Four columns
// constrained instead of one, a COVERING seek instead of a seek plus a fetch,
// and no temp b-tree under CORRELATED SCALAR SUBQUERY 1 at all. The
// walked-and-sorted set that grew with import COUNT is now a single index seek
// straight to the newest row. ONE temp b-tree remains and it is the outer
// `ORDER BY ls.library_id, ls.id`, which is bounded by the number of library
// sources on the screen and was never the problem.
//
// ⚠️ THE COUNT ASSERTION THEREFORE FLIPPED FROM 2 TO 1, DELIBERATELY. It was not
// weakened to accommodate whatever plan appeared: the new plan was measured
// first and is now asserted exactly, positionally, and the "any SCAN anywhere is
// a fault" clause is unchanged. A count of 2 now means the subquery started
// sorting again — which is what arm 1 below produces on purpose.
//
// 🔍 AND THE OLD LEVER IS NOW MEASURABLY IRRELEVANT TO THIS READ. Dropping
// ix_sync_report_instance used to change this plan (it is what the ⚠️ paragraph
// above is about); with 0011 in place it changes NOTHING here, because the
// subquery no longer touches that index. That is asserted below rather than
// merely claimed — it is what killed the original arm 1, and the assertion is
// where the dead arm went.
//
// WHAT IS STILL DELIBERATELY NOT ASSERTED: the absence of the ONE remaining
// sort, because it is really there and forbidding it would fail on the first
// honest run — the same posture TestSearchLibraryPlanIsSeeks takes for its two.
// It is asserted as EXPECTED and counted, so if it disappears the plan changed
// shape and this comment must be re-read rather than left silently wrong.
// ─────────────────────────────────────────────────────────────────────────────

// seedCompletenessReports gives sync_report rows for every source in the corpus,
// several imports deep, plus rows of another kind on the same containers so that
// a subquery which stopped filtering on kind would pick one of those instead.
//
// Rows rather than an empty table, because a correlated subquery over nothing is
// not the read that ships. It does not change the PLAN — measured identical at 0
// rows, at 240 rows, and at 240 rows after `ANALYZE`, since with no sqlite_stat1
// the planner chooses from the schema, and this schema offers it no alternative
// to choose. That null result is recorded here so nobody re-derives it.
//
// ✅ RE-CONFIRMED AFTER MIGRATION 0011, which changed the plan but not this: the
// post-0011 plan is likewise identical over an empty sync_report, over the
// corpus below, and over the corpus after `ANALYZE`, and it is identical whether
// the index is created before the rows or after them. These guards pin the
// NO-ANALYZE planner by convention (internal/store/browse.go:244) and that
// convention costs nothing here, because there is nothing for statistics to
// change their mind about.
func seedCompletenessReports(t *testing.T, s *Store) {
	t.Helper()
	sources := []struct {
		instance int
		ref      string
	}{{1, "11"}, {1, "12"}, {2, "21"}, {2, "22"}}

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for run := range 3 {
			for _, src := range sources {
				at := fmt.Sprintf("2026-08-%02d 00:00:00", run+1)
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO sync_report
					   (service_instance_id, kind, remote_kind, remote_id, detail, created_at)
					 VALUES (?, ?, 'library', ?, ?, ?)`,
					src.instance, SyncReportContentCompleteness, src.ref,
					`{"state":"complete","container_identity":"c","expected":10,"visible":10}`,
					at); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO sync_report
					   (service_instance_id, kind, remote_kind, remote_id, detail, created_at)
					 VALUES (?, 'items_skipped', 'library', ?, '{}', ?)`,
					src.instance, src.ref, at); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed sync_report: %v", err)
	}
}

// libraryCompletenessPlan renders the SHIPPED statement through
// libraryCompletenessSQL — the same function attachLibraryCompleteness calls,
// per docs/DEVELOPMENT.md §11 rule 1: a guard that asserts against a hand-copied
// lookalike is probing a proxy.
func libraryCompletenessPlan(t *testing.T, s *Store, scope Scope) []string {
	t.Helper()
	query, args := libraryCompletenessSQL(scope, []int64{1, 2, 3})
	return planStepsOf(t, s, query, args)
}

// planStepsOf is planOf's ordered sibling. It returns the steps as a slice
// rather than one joined string because the step that matters most here is a
// temp b-tree whose meaning depends entirely on which parent it hangs off:
// `USE TEMP B-TREE FOR ORDER BY` at the top level is the harmless outer
// `ORDER BY ls.library_id, ls.id`, and the same words directly under
// `CORRELATED SCALAR SUBQUERY 1` are the per-source sort described above.
// EXPLAIN QUERY PLAN emits children immediately after their parent, and
// db.QueryPlan drops the parent column, so POSITION is what tells them apart.
func planStepsOf(t *testing.T, s *Store, query string, args []any) []string {
	t.Helper()
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return plan
}

// libraryCompletenessPlanFaults is the assertion the guard and its firing both
// run, so neither can drift from the other.
func libraryCompletenessPlanFaults(plan []string) []string {
	joined := strings.Join(plan, " | ")
	var faults []string

	if !strings.Contains(joined,
		"SEARCH ls USING COVERING INDEX sqlite_autoindex_library_source_1 (library_id=?") {
		faults = append(faults,
			"the library_source leg is not a covering seek on the unique key's leading "+
				"library_id, so the IN list walks every source in the install")
	}
	if !strings.Contains(joined, "SEARCH si USING INTEGER PRIMARY KEY") {
		faults = append(faults, "the service_instance join is not a rowid lookup")
	}
	if !strings.Contains(joined, "SEARCH r USING INTEGER PRIMARY KEY (rowid=?)") {
		faults = append(faults,
			"the verdict row is not fetched by rowid: `r.id = (subquery)` no longer "+
				"resolves to an integer-primary-key seek, so the outer join walks sync_report")
	}
	if strings.Contains(joined, "SCAN ") {
		faults = append(faults, "the plan contains a SCAN; every leg of this read is a seek")
	}

	// The subquery, BY POSITION: its sync_report leg must be the covering seek on
	// migration 0011's index, and it must be the subquery's ONLY step — the sort
	// that used to hang under it is what 0011 exists to remove.
	i := indexOfPlanStep(plan, "CORRELATED SCALAR SUBQUERY 1")
	switch {
	case i < 0:
		faults = append(faults,
			"the newest-verdict pick is no longer a correlated scalar subquery, so this "+
				"guard is asserting against a statement it does not describe")
	case i+1 >= len(plan):
		faults = append(faults, "the correlated subquery has no step under it at all")
	default:
		const wantSeek = "SEARCH r2 USING COVERING INDEX ix_sync_report_container_latest " +
			"(service_instance_id=? AND kind=? AND remote_kind=? AND remote_id=?)"
		if plan[i+1] != wantSeek {
			faults = append(faults, fmt.Sprintf(
				"the subquery's sync_report leg is %q, not %q — migration 0011 exists to "+
					"make this a four-column covering seek, and without it the newest-verdict "+
					"pick either walks one instance's whole report history and SORTS it "+
					"(ix_sync_report_instance) or reads the entire table (no index), once per "+
					"library source", plan[i+1], wantSeek))
		}
		// ⚠️ POSITIONALLY, the seek must be the LAST step before the single outer
		// sort. db.QueryPlan drops the parent column, so "the subquery has no sort
		// under it" is expressible only as "nothing sits between its seek and the
		// end except the one top-level ORDER BY".
		switch {
		case len(plan) != i+3:
			faults = append(faults, fmt.Sprintf(
				"the correlated subquery has %d step(s) under it, want exactly 1 (its "+
					"covering seek). A second step here is the per-library_source SORT that "+
					"migration 0011 removed, coming back: %q",
				len(plan)-i-1, strings.Join(plan[i+1:], " | ")))
		case !strings.HasPrefix(plan[i+2], "USE TEMP B-TREE"):
			faults = append(faults, fmt.Sprintf(
				"the step after the subquery is %q, not the outer `ORDER BY "+
					"ls.library_id, ls.id`", plan[i+2]))
		}
	}

	// Exactly ONE, and it is accounted for: the outer `ORDER BY ls.library_id,
	// ls.id`. It was TWO before migration 0011, when the subquery sorted per
	// library_source row; the flip from 2 to 1 is the measured improvement, not a
	// relaxation. A second temp b-tree is that sort returning.
	if n := strings.Count(joined, "USE TEMP B-TREE"); n != 1 {
		faults = append(faults, fmt.Sprintf(
			"the plan has %d temp b-trees, want exactly 1 (the outer `ORDER BY "+
				"ls.library_id, ls.id`). Before migration 0011 there were 2, the second "+
				"being the subquery's `ORDER BY r2.id DESC` — if that one is back, "+
				"ix_sync_report_container_latest is gone or unusable", n))
	}
	return faults
}

func indexOfPlanStep(plan []string, step string) int {
	for i, s := range plan {
		if s == step {
			return i
		}
	}
	return -1
}

func TestLibraryCompletenessPlanIsSeeksAndOneSort(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)
	seedCompletenessReports(t, s)

	owner := libraryCompletenessPlan(t, s, OwnerScope(0))
	if faults := libraryCompletenessPlanFaults(owner); len(faults) > 0 {
		t.Errorf("the owner plan is wrong:\n  plan: %s\n  %s",
			strings.Join(owner, "\n        "), strings.Join(faults, "\n  "))
	}
	// The outer sort in owner scope is a LAST TERM one: the IN list is walked in
	// library_id order off sqlite_autoindex_library_source_1, so only ls.id is
	// left to sort.
	if last := owner[len(owner)-1]; last != "USE TEMP B-TREE FOR LAST TERM OF ORDER BY" {
		t.Errorf("the owner plan's outer sort is %q, want a LAST TERM sort:\n  %s\n"+
			"A full sort here means library_id stopped arriving in order from the "+
			"library_source index.", last, strings.Join(owner, "\n  "))
	}

	// The SCOPED plan is the one a multi-user install runs, and it reverses the
	// join order — si leads, ls picks up its second key column. Neither may cost
	// a seek or add a sort.
	scoped := libraryCompletenessPlan(t, s, Scope{UserID: 0, InstanceIDs: []int64{1, 2}})
	if faults := libraryCompletenessPlanFaults(scoped); len(faults) > 0 {
		t.Errorf("the scoped plan is wrong:\n  plan: %s\n  %s",
			strings.Join(scoped, "\n        "), strings.Join(faults, "\n  "))
	}
	const wantScopedSource = "SEARCH ls USING COVERING INDEX " +
		"sqlite_autoindex_library_source_1 (library_id=? AND service_instance_id=?)"
	if !strings.Contains(strings.Join(scoped, " | "), wantScopedSource) {
		t.Errorf("the scoped source leg does not constrain both key columns.\n"+
			"  got:  %s\n  want: %s\n"+
			"Constraining only library_id would make the instance scope a post-filter, "+
			"and a visibility predicate that filters after the join is the shape §14 "+
			"does not allow.", strings.Join(scoped, "\n        "), wantScopedSource)
	}
}

// TestLibraryCompletenessDoesNotDependOnTheInstanceIndex is where the ORIGINAL
// arm 1 went, and it is the assertion that arm turned into.
//
// Before migration 0011, dropping ix_sync_report_instance changed this plan — it
// was the whole subject of the ⚠️ paragraph in the block comment above, and
// TestLibraryCompletenessPlanGuardFires arm 1 dropped it and watched the guard
// go red. With 0011 in place that arm is DEAD: the subquery seeks on
// ix_sync_report_container_latest and never looks at the instance index, so
// dropping it produces a byte-identical plan and the guard stays green. An arm
// that cannot fire is not an arm.
//
// Rather than delete the coverage, it is inverted. The migration header claims
// the two indexes serve different reads and that neither subsumes the other;
// this is the half of that claim visible from internal/store — that the
// completeness read does not depend on ix_sync_report_instance AT ALL. The other
// half (that the Services screen's read still needs it, and still does not sort)
// is TestMigrate0011KeepsTheInstanceIndex in internal/db.
//
// Together they are what stops a later "two indexes on one small table" tidy-up
// from dropping the wrong one silently.
func TestLibraryCompletenessDoesNotDependOnTheInstanceIndex(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)
	seedCompletenessReports(t, s)

	// ⚠️ THIS TEST IS THE ONE THE READ-POOL STALENESS HAZARD WOULD DESTROY, and
	// it is the reason dropIndexesAndConfirm exists. Every other arm asserts that
	// something CHANGED, so a stale plan makes it fail loudly. This one asserts
	// that nothing changed — so a stale plan is indistinguishable from the result
	// it is looking for, and it would pass while measuring absolutely nothing.
	// dropIndexesAndConfirm proves on the READ pool that the drop landed before
	// any EXPLAIN runs here.
	dropIndexesAndConfirm(t, s, "ix_sync_report_instance")

	for _, tc := range []struct {
		name  string
		scope Scope
	}{
		{"owner", OwnerScope(0)},
		{"instance", Scope{UserID: 0, InstanceIDs: []int64{1, 2}}},
	} {
		plan := libraryCompletenessPlan(t, s, tc.scope)
		if faults := libraryCompletenessPlanFaults(plan); len(faults) > 0 {
			t.Errorf("the %s plan degraded when ix_sync_report_instance was dropped:\n"+
				"  plan: %s\n  %s\n"+
				"Migration 0011's header states this read does not touch that index. If "+
				"that is no longer true, the header is wrong and so is "+
				"TestMigrate0011KeepsTheInstanceIndex's reasoning.",
				tc.name, strings.Join(plan, "\n        "), strings.Join(faults, "\n  "))
		}
	}
}

// FIRING THE PLAN GUARD. Four arms, each breaking a different thing it protects,
// each running the SAME libraryCompletenessPlanFaults the test above runs.
//
// ⚠️ ARM 1 IS NOT THE ARM IT USED TO BE. It dropped ix_sync_report_instance,
// which was the regression that mattered before migration 0011. With 0011 in
// place that drop changes nothing about this plan, so the arm went dead and was
// REPOINTED at ix_sync_report_container_latest — the index this read now
// actually depends on. The fact it used to assert is not lost: it became
// TestLibraryCompletenessDoesNotDependOnTheInstanceIndex above, where "dropping
// it changes nothing" is the point rather than the failure.
func TestLibraryCompletenessPlanGuardFires(t *testing.T) {
	// Arm 1 — ix_sync_report_container_latest is dropped outright. This is
	// EXACTLY migration 0011's Down block, so the arm is a live rehearsal of the
	// rollback, and it is the regression that matters most: the subquery falls
	// back to ix_sync_report_instance and the per-library_source SORT returns,
	// which is the pre-0011 plan the block comment above records verbatim.
	// sync_report is append-only and this kind writes one row per container per
	// import, so that sorted set grows with import count forever, on a screen
	// that renders from local SQLite precisely so that it never waits.
	t.Run("ix_sync_report_container_latest dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)
		seedCompletenessReports(t, s)
		dropIndexesAndConfirm(t, s, "ix_sync_report_container_latest")

		plan := libraryCompletenessPlan(t, s, OwnerScope(0))
		faults := libraryCompletenessPlanFaults(plan)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan with ix_sync_report_container_latest "+
				"dropped:\n  %s", strings.Join(plan, "\n  "))
		}
		// The sort must be what came back, not merely "something differed".
		if n := strings.Count(strings.Join(plan, " | "), "USE TEMP B-TREE"); n != 2 {
			t.Errorf("dropping 0011's index produced %d temp b-trees, want the 2 of the "+
				"pre-0011 plan:\n  %s", n, strings.Join(plan, "\n  "))
		}
		t.Logf("guard fired with ix_sync_report_container_latest dropped:\n"+
			"  plan:   %s\n  faults: %v", strings.Join(plan, "\n          "), faults)
	})

	// Arm 2 — BOTH indexes dropped, so the subquery has nothing at all and plans
	// as a bare `SCAN r2`.
	//
	// ⚠️ THIS ARM EXISTS BECAUSE THE TEMP-B-TREE COUNT CANNOT CATCH IT. A full
	// table scan visits rows in rowid order, which is `id` order, so
	// `ORDER BY r2.id DESC` comes free off it and the count stays at 1 — the
	// healthy number. This is the trap the block comment's ⚠️ paragraph records:
	// the worst plan available and the best plan available have the same sort
	// count. It is the "any SCAN anywhere is a fault" clause that catches it, and
	// this arm is that clause being executed rather than written down.
	t.Run("both sync_report indexes dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)
		seedCompletenessReports(t, s)
		dropIndexesAndConfirm(t, s,
			"ix_sync_report_container_latest", "ix_sync_report_instance")

		plan := libraryCompletenessPlan(t, s, OwnerScope(0))
		joined := strings.Join(plan, " | ")
		if !strings.Contains(joined, "SCAN r2") {
			t.Fatalf("dropping both indexes did not produce the bare `SCAN r2` this arm "+
				"is built on:\n  %s", strings.Join(plan, "\n  "))
		}
		if n := strings.Count(joined, "USE TEMP B-TREE"); n != 1 {
			t.Errorf("the scan plan has %d temp b-trees, want 1 — the premise of this arm "+
				"is that a scan sorts for FREE and so the count alone cannot catch it:\n  %s",
				n, strings.Join(plan, "\n  "))
		}
		faults := libraryCompletenessPlanFaults(plan)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan that SCANS sync_report once per library "+
				"source:\n  %s", strings.Join(plan, "\n  "))
		}
		t.Logf("guard fired on a bare SCAN of sync_report:\n  plan:   %s\n  faults: %v",
			strings.Join(plan, "\n          "), faults)
	})

	// Arm 3 — both indexes survive but the subquery is disqualified from using
	// either, with SQLite's unary plus (<https://sqlite.org/optoverview.html>) on
	// its leading equality. That is the shape a rewritten predicate would take,
	// and it degrades to the same `SCAN r2` as arm 2 by a completely different
	// route: the schema is intact and the STATEMENT is what broke. This arm
	// proves the guard reads the plan rather than merely noticing that the table
	// has the indexes it expects.
	t.Run("the subquery loses its index", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)
		seedCompletenessReports(t, s)

		query, args := libraryCompletenessSQL(OwnerScope(0), []int64{1, 2, 3})
		broken := strings.Replace(query,
			"WHERE r2.service_instance_id = ls.service_instance_id",
			"WHERE +r2.service_instance_id = ls.service_instance_id", 1)
		if broken == query {
			t.Fatalf("the shipped statement no longer contains the subquery's instance "+
				"predicate, so this arm is asserting against nothing:\n%s", query)
		}
		plan := planStepsOf(t, s, broken, args)
		faults := libraryCompletenessPlanFaults(plan)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan whose subquery scans sync_report:\n  %s",
				strings.Join(plan, "\n  "))
		}
		t.Logf("guard fired with the subquery's index disqualified:\n  plan:   %s\n  faults: %v",
			strings.Join(plan, "\n          "), faults)
	})

	// Arm 4 — the outer library_source seek is disqualified, so the read walks
	// every source in the install and runs the subquery against each. This is the
	// one arm whose fault is OUTSIDE the subquery: the sync_report leg stays a
	// healthy covering seek throughout, so it proves the guard is still watching
	// the join it was watching before 0011 and did not narrow to the index.
	t.Run("the source leg degrades to a scan", func(t *testing.T) {
		s := newTestStore(t)
		seedLibrariesCorpus(t, s)
		seedCompletenessReports(t, s)

		query, args := libraryCompletenessSQL(OwnerScope(0), []int64{1, 2, 3})
		broken := strings.Replace(query, "WHERE ls.library_id IN (", "WHERE +ls.library_id IN (", 1)
		if broken == query {
			t.Fatalf("the shipped statement no longer contains the library IN list this arm "+
				"rewrites:\n%s", query)
		}
		plan := planStepsOf(t, s, broken, args)
		faults := libraryCompletenessPlanFaults(plan)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan that scans library_source:\n  %s",
				strings.Join(plan, "\n  "))
		}
		t.Logf("guard fired on a scan of library_source:\n  plan:   %s\n  faults: %v",
			strings.Join(plan, "\n          "), faults)
	})
}

// ─── THE SIBLING AXIS: what the import read and did not map ──────────────────

// seedSkipReports writes what an import that WALKED Manga's and both of Books'
// containers would leave behind (ADR-0063): a non-zero tally on Manga's, a
// ZERO-COUNT row on each of Books', and no skip row at all for Films'.
//
// ⚠️ FILMS' CONTAINER GETS A COMPLETENESS VERDICT AND NO SKIP ROW ON PURPOSE,
// and it is the control that proves the coupling is retired. Under ADR-0061 §5
// that exact pair was the evidence for `none`; Films must now read nil, because
// a completeness verdict says a container was ASKED ABOUT, not that it was
// walked. Loose Ends has no source at all and stays nil either way.
//
// TWO IMPORTS DEEP ON MANGA'S SKIP ROW, so the newest-row pick is doing work: if the
// correlated subquery stopped ordering by id the older tally would win and the
// count would be wrong rather than absent, which is the failure that looks like
// data.
func seedSkipReports(t *testing.T, s *Store) {
	t.Helper()
	type row struct {
		instance int
		kind     string
		ref      string
		detail   string
		at       string
	}
	rows := []row{
		// Manga, first import: a bigger tally that must LOSE to the newer one.
		{
			1, SyncReportItemsSkipped, "11",
			`{"name":"Manga","skipped_comics":90,"skipped_unknown":9,"reason":"stale"}`,
			"2026-08-01 00:00:00",
		},
		{
			1, SyncReportItemsSkipped, "11",
			`{"name":"Manga","skipped_comics":2,"skipped_unknown":1,` +
				`"reason":"UsArr maps prose books only","effect":"no work row"}`,
			"2026-08-02 00:00:00",
		},
		// Books' two containers: WALKED, and each recorded a clean zero. No
		// reason and no effect, because there is no skip to explain.
		{
			1, SyncReportItemsSkipped, "12",
			`{"name":"Books","skipped_comics":0,"skipped_unknown":0}`,
			"2026-08-02 00:00:00",
		},
		{
			2, SyncReportItemsSkipped, "21",
			`{"name":"More Books","skipped_comics":0,"skipped_unknown":0}`,
			"2026-08-03 00:00:00",
		},
		// Every container that was ASKED ABOUT — which now includes Films' '22',
		// the one with no skip row above.
		{1, SyncReportContentCompleteness, "11", `{"state":"complete"}`, "2026-08-02 00:00:00"},
		{1, SyncReportContentCompleteness, "12", `{"state":"complete"}`, "2026-08-02 00:00:00"},
		{2, SyncReportContentCompleteness, "21", `{"state":"complete"}`, "2026-08-03 00:00:00"},
		{2, SyncReportContentCompleteness, "22", `{"state":"complete"}`, "2026-08-03 00:00:00"},
	}
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO sync_report
				   (service_instance_id, kind, remote_kind, remote_id, detail, created_at)
				 VALUES (?, ?, 'library', ?, ?, ?)`,
				r.instance, r.kind, r.ref, r.detail, r.at); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed skip reports: %v", err)
	}
}

// THE ASSERTION THE WHOLE FEATURE TURNS ON: "nothing was skipped" and "nothing
// walked this library" are DIFFERENT STORED VALUES, not one absence with two
// meanings.
//
// ⚠️ THE DISTINCTION SURVIVED ADR-0063; ITS EVIDENCE DID NOT, AND THIS TEST IS
// RENAMED FOR THAT RATHER THAN REWRITTEN. It used to assert that the read pairs
// the items_skipped table with the COMPLETENESS row, because a skip row existed
// only where something had been skipped (ADR-0061 §5) and the table alone could
// not tell the two apart. Now every container an import walked has a skip row,
// zero or not, so the answer comes off those rows alone.
//
// ⚠️ AND THE OLD CONTROL IS INVERTED RATHER THAN DROPPED, which is the whole
// value of keeping it: Films' container carries a completeness verdict and NO
// skip row — the exact pair that used to mean `none` — and it must now read nil.
// If somebody reinstates the completeness fallback, Films goes to `none` and
// this goes red naming it. Books is the other half: two containers, two
// zero-count rows, and a measured `none` that rests on nothing but its own rows.
func TestLibrarySkipsTellsNothingSkippedFromNothingWalked(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)
	seedSkipReports(t, s)

	got, err := s.ListLibraries(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}

	manga := libraryByName(t, got, "Manga").Skips
	if manga == nil || manga.State != SkipsLeftOut {
		t.Fatalf("Manga's skip verdict = %+v, want a left_out verdict", manga)
	}
	if manga.Items != 3 {
		t.Errorf("Manga left out %d items, want 3 (2 comics + 1 unknown, from the NEWEST "+
			"row — 99 means the older import's tally won and the newest-row pick is broken)",
			manga.Items)
	}
	if manga.Reason != "UsArr maps prose books only" {
		t.Errorf("Manga's reason = %q, want the newest row's own words", manga.Reason)
	}
	if manga.RecordedAt == "" {
		t.Error("Manga's verdict carries no timestamp, so a reader cannot tell how old it is")
	}

	books := libraryByName(t, got, "Books").Skips
	if books == nil || books.State != SkipsNone {
		t.Fatalf("Books' skip verdict = %+v, want `none` — both of its containers were "+
			"walked and each recorded a ZERO-COUNT row, which is a MEASURED negative and "+
			"must not read as the same thing as nobody having looked", books)
	}
	if books.Items != 0 || books.Reason != "" {
		t.Errorf("the `none` verdict published a count or a reason: %+v — there is nothing "+
			"to count there, and a zero under that label is a claim the label does not make",
			books)
	}
	if books.Containers != 2 {
		t.Errorf("Books folded %d container observations, want 2", books.Containers)
	}

	// ⚠️ THE CONTROLS, AND THE FIRST OF THEM IS THE INVERTED ONE. Films'
	// container has a COMPLETENESS verdict and no skip row — under ADR-0061 §5
	// that pair WAS the evidence for `none`, and this test asserted it. ADR-0063
	// retired the pairing, so nil is now the right answer and `none` is the
	// regression. Loose Ends has no source at all and is nil under either rule.
	if films := libraryByName(t, got, "Films").Skips; films != nil {
		t.Errorf("Films carries %+v — nothing ever WALKED its container, only asked "+
			"about it, and publishing a verdict off the completeness row is the "+
			"coupling ADR-0063 removed", films)
	}
	if loose := libraryByName(t, got, "Loose Ends").Skips; loose != nil {
		t.Errorf("Loose Ends has no source and carries %+v", loose)
	}
}

// A detail blob that will not decode is DROPPED, and with it the only evidence
// that anything walked that container — so the library reads nil.
//
// ⚠️ THIS EXPECTATION IS INVERTED BY ADR-0063 RATHER THAN DELETED, and the old
// one is worth stating because it is why the test exists: the drop used to fall
// through to the SkipsNone pass, so an unreadable row on an OBSERVED container
// still produced `none`. That pass is gone. nil is a step further into the same
// safe direction the drop was chosen for — it understates what is known and
// never overstates it — and it is the honest answer here, because what could not
// be read is precisely the record of the walk.
func TestAnUnreadableSkipRowIsDroppedAndPublishesNothing(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)
	seedSkipReports(t, s)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sync_report (service_instance_id, kind, remote_kind, remote_id, detail, created_at)
			 VALUES (1, ?, 'library', '11', 'not json at all', '2026-08-09 00:00:00')`,
			SyncReportItemsSkipped)
		return err
	}); err != nil {
		t.Fatalf("seed the unreadable row: %v", err)
	}

	got, err := s.ListLibraries(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if manga := libraryByName(t, got, "Manga").Skips; manga != nil {
		t.Fatalf("Manga's verdict = %+v, want NIL: its newest skip row is unreadable, so "+
			"the one record of what the walk left out cannot be read and nothing may be "+
			"published off it — least of all a measured negative", manga)
	}
}

// THE PLAN GUARD IS SHARED, AND THIS IS THE ASSERTION THAT KEEPS IT SHARED.
//
// librarySkipsSQL and libraryCompletenessSQL both delegate to
// containerReportSQL, so the two statements are the same text with a different
// bound kind. That is what lets TestLibraryCompletenessPlanIsSeeksAndOneSort and
// its four firing arms cover this read without a second copy of any of it — and
// the day somebody hand-writes a second statement, this goes red and says which
// coverage just lapsed.
//
// ⚠️ RE-EXAMINED AND KEPT UNDER ADR-0063, which changed neither statement: the
// zero-count skip row is a WRITE-side change, so containerReportSQL, the four
// equalities, the `ORDER BY r2.id` and ix_sync_report_container_latest are all
// untouched. What did change is that both kinds now file a row per walked
// container, which makes the shared statement more obviously the right shape
// rather than a coincidence worth un-sharing.
func TestTheSkipStatementIsTheCompletenessStatement(t *testing.T) {
	skipSQL, skipArgs := librarySkipsSQL(OwnerScope(0), []int64{1, 2, 3})
	compSQL, compArgs := libraryCompletenessSQL(OwnerScope(0), []int64{1, 2, 3})

	if skipSQL != compSQL {
		t.Fatalf("the two statements have diverged, so the completeness plan guard no "+
			"longer covers the skip read and this read has NO plan assertion at all:\n"+
			"skips:\n%s\ncompleteness:\n%s", skipSQL, compSQL)
	}
	if len(skipArgs) != len(compArgs) {
		t.Fatalf("skip args %v, completeness args %v", skipArgs, compArgs)
	}
	if skipArgs[0] != SyncReportItemsSkipped {
		t.Errorf("the skip statement binds kind %v, want %q — the kind is the FIRST "+
			"parameter because the correlated subquery is earlier in the text than the "+
			"IN list", skipArgs[0], SyncReportItemsSkipped)
	}
	if compArgs[0] != SyncReportContentCompleteness {
		t.Errorf("the completeness statement binds kind %v", compArgs[0])
	}
}

// librarySkipsPlan renders the SHIPPED statement through librarySkipsSQL — the
// same function attachLibrarySkips calls, per docs/DEVELOPMENT.md §11 rule 1.
func librarySkipsPlan(t *testing.T, s *Store, scope Scope) []string {
	t.Helper()
	query, args := librarySkipsSQL(scope, []int64{1, 2, 3})
	return planStepsOf(t, s, query, args)
}

// The skip read plans exactly as the completeness read does, and it is MEASURED
// rather than inferred from the shared text.
//
// The two statements are byte-identical, but they bind different kinds, and a
// bound value is something the planner can in principle take an interest in. It
// does not here — with no sqlite_stat1 the planner chooses from the schema —
// and that null result is worth executing rather than assuming, because the
// whole claim that migration 0011's index serves this read rests on it.
func TestLibrarySkipsPlanIsSeeksAndOneSort(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)
	seedCompletenessReports(t, s)

	for _, tc := range []struct {
		name  string
		scope Scope
	}{
		{"owner", OwnerScope(0)},
		{"instance", Scope{UserID: 0, InstanceIDs: []int64{1, 2}}},
	} {
		plan := librarySkipsPlan(t, s, tc.scope)
		if faults := libraryCompletenessPlanFaults(plan); len(faults) > 0 {
			t.Errorf("the %s skip plan is wrong:\n  plan: %s\n  %s",
				tc.name, strings.Join(plan, "\n        "), strings.Join(faults, "\n  "))
		}
	}
}

// FIRING THAT GUARD ON THE SKIP STATEMENT SPECIFICALLY.
//
// ⚠️ THE DROP GOES THROUGH dropIndexesAndConfirm, NEVER A BARE DROP. A read-pool
// connection that has already planned a statement keeps serving the pre-drop
// plan indefinitely, so a bare drop can make this arm measure nothing at all
// while looking green — the hazard store_test.go's helper exists for.
func TestLibrarySkipsPlanGuardFires(t *testing.T) {
	s := newTestStore(t)
	seedLibrariesCorpus(t, s)
	seedCompletenessReports(t, s)
	dropIndexesAndConfirm(t, s, "ix_sync_report_container_latest")

	plan := librarySkipsPlan(t, s, OwnerScope(0))
	faults := libraryCompletenessPlanFaults(plan)
	if len(faults) == 0 {
		t.Fatalf("the guard passed a skip plan with ix_sync_report_container_latest "+
			"dropped:\n  %s", strings.Join(plan, "\n  "))
	}
	if n := strings.Count(strings.Join(plan, " | "), "USE TEMP B-TREE"); n != 2 {
		t.Errorf("dropping 0011's index produced %d temp b-trees in the skip plan, want "+
			"the 2 of the pre-0011 shape:\n  %s", n, strings.Join(plan, "\n  "))
	}
	t.Logf("skip guard fired with ix_sync_report_container_latest dropped:\n"+
		"  plan:   %s\n  faults: %v", strings.Join(plan, "\n          "), faults)
}
