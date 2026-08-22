package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// The corpus for §17.8's Accept step.
//
// A REAL MIGRATED DATABASE WITH ROWS IN IT, and POPULATED BEFORE ANYTHING IS
// ASSERTED, for catalogue_test.go's reason: a membership derivation over an
// empty corpus files nothing and every count agrees with every other count.
//
// The shape, chosen so that a filter which did nothing at all would still look
// plausible — every exclusion this file asserts has a row that would be included
// without it:
//
//	instance 1  Kavita One
//	  c-a   work 1 comic, work 2 comic          — and work 9, whose LINK is tombstoned
//	  c-b   work 3 book                         — and work 4, whose WORK is tombstoned
//	  c-m   work 5 book, work 6 comic           — mixed, ADR-0066 decision 5's shape
//	        work 7 comic_issue under work 6     — a child kind, never a member
//	instance 2  Kavita Two
//	  c-z   work 8 comic                        — already bound to library 100
//
//	library 100  "Existing Comics" (comic), sourced on instance 2 / c-z, holding work 8
//
// So: the two instances split the containers, one container is mixed-kind, one
// holds a child, two rows are tombstoned by the two different stamps that mean
// it, and one library exists already so the join path and the already-bound half
// of a proposal have something real to find.
//
// EVERY LIVE TOP-LEVEL WORK GETS A SEARCH DOCUMENT, written by writeSearchDoc
// itself rather than by hand. That is what puts works 1-6 and 9 at library 0
// through the real invariant-5 fallback, which is the state ADR-0048's ordering
// leaves behind — the import runs BEFORE Accept — and it is the state
// rescopeSearchDocs has to repair.
// ─────────────────────────────────────────────────────────────────────────────

type proposalWork struct {
	id        int64
	kind      string
	instance  int64
	container string
	// workGone tombstones the WORK; linkGone tombstones the LINK. They are
	// different stamps meaning different things (ADR-0074) and the filing
	// statement has to honour both.
	workGone bool
	linkGone bool
}

var proposalCorpus = []proposalWork{
	{id: 1, kind: "comic", instance: 1, container: "c-a"},
	{id: 2, kind: "comic", instance: 1, container: "c-a"},
	{id: 9, kind: "comic", instance: 1, container: "c-a", linkGone: true},
	{id: 3, kind: "book", instance: 1, container: "c-b"},
	{id: 4, kind: "book", instance: 1, container: "c-b", workGone: true},
	{id: 5, kind: "book", instance: 1, container: "c-m"},
	{id: 6, kind: "comic", instance: 1, container: "c-m"},
	{id: 7, kind: "comic_issue", instance: 1, container: "c-m"},
	{id: 8, kind: "comic", instance: 2, container: "c-z"},
}

func seedProposalsCorpus(t *testing.T, s *Store) {
	t.Helper()
	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'kavita', 'library', 'Kavita One', 'http://one.example', X'00')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (2, 'kavita', 'library', 'Kavita Two', 'http://two.example', X'00')`,
	}
	for _, w := range proposalCorpus {
		deleted := "NULL"
		if w.workGone {
			deleted = `'2026-08-15 00:00:00'`
		}
		parent := "NULL"
		if w.kind == "comic_issue" {
			parent = "6"
		}
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO work
			  (id, kind, title, sort_title, normalized_title, parent_work_id, added_at, deleted_at)
			VALUES (%d, '%s', 'w%02d', 'w%02d', 'w%02d', %s, '2026-08-10 10:00:00', %s)`,
			w.id, w.kind, w.id, w.id, w.id, parent, deleted))

		linkDeleted := "NULL"
		if w.linkGone {
			linkDeleted = `'2026-08-15 00:00:00'`
		}
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO service_item_link
			  (service_instance_id, work_id, remote_id, remote_kind, remote_library_id,
			   synced_at, deleted_at)
			VALUES (%d, %d, 'r%02d', 'series', '%s', '2026-08-16 12:00:00', %s)`,
			w.instance, w.id, w.id, w.container, linkDeleted))
	}

	// The library that already exists, with its source and its member. It is
	// seeded AFTER the works its member names — library_member.work_id is a
	// foreign key — and BEFORE the search documents, so work 8's document is
	// scoped to it by the real writer rather than to library 0.
	stmts = append(stmts,
		`INSERT INTO library (id, user_id, name, slug, kind, managed_by)
		   VALUES (100, 0, 'Existing Comics', 'existing-comics', 'comic', 'auto')`,
		`INSERT INTO library_source
		   (library_id, service_instance_id, container_kind, container_ref, container_identity)
		 VALUES (100, 2, 'remote_library', 'c-z', 'Two''s comics')`,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (100, 'w08', 8)`)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		// The documents, through the shipped writer. A comic_issue is refused by
		// it on purpose (corpusExcludedKinds), which is why work 7 is skipped
		// here rather than filtered out of the corpus.
		for _, w := range proposalCorpus {
			if corpusExcludedKinds[w.kind] {
				continue
			}
			title := fmt.Sprintf("w%02d", w.id)
			if err := writeSearchDoc(ctx, tx, w.id, searchDocText{
				kind: w.kind, normTitle: title, title: title,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed the proposals corpus: %v", err)
	}
}

// ── the assertions every test below shares ──────────────────────────────────

func membersOf(t *testing.T, s *Store, libraryID int64) []int64 {
	t.Helper()
	rows, err := s.DB().Read().QueryContext(t.Context(),
		`SELECT work_id FROM library_member WHERE library_id = ? ORDER BY work_id`, libraryID)
	if err != nil {
		t.Fatalf("read members of library %d: %v", libraryID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("read members of library %d: scan: %v", libraryID, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read members of library %d: %v", libraryID, err)
	}
	return out
}

// docScopes is which libraries a work's search document is scoped to. It is the
// question §7 invariant 5 is about, asked per work.
func docScopes(t *testing.T, s *Store, workID int64) []int64 {
	t.Helper()
	rows, err := s.DB().Read().QueryContext(t.Context(), `
		SELECT sdl.library_id
		  FROM search_doc d
		  JOIN search_doc_library sdl ON sdl.doc_rowid = d.rowid
		 WHERE d.work_id = ?
		 ORDER BY sdl.library_id`, workID)
	if err != nil {
		t.Fatalf("read document scopes for work %d: %v", workID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("read document scopes for work %d: scan: %v", workID, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read document scopes for work %d: %v", workID, err)
	}
	return out
}

func acceptance(name, kind string, sources ...AcceptedSource) LibraryAcceptance {
	return LibraryAcceptance{Name: name, Kind: kind, ManagedBy: "user", Sources: sources}
}

func source(instance int64, ref string) AcceptedSource {
	return AcceptedSource{
		ServiceInstanceID: instance, ContainerKind: "remote_library",
		ContainerRef: ref, ContainerIdentity: "upstream name of " + ref,
	}
}

func proposalByRef(t *testing.T, got []ContainerProposal, ref string) ContainerProposal {
	t.Helper()
	for _, p := range got {
		if p.RemoteID == ref {
			return p
		}
	}
	t.Fatalf("no proposal for container %q in %+v", ref, got)
	return ContainerProposal{}
}

// ── DescribeContainers ──────────────────────────────────────────────────────

func TestDescribeContainersCountsAndBindings(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	got, err := s.DescribeContainers(t.Context(), OwnerScope(0), 1, []CatalogueContainer{
		{RemoteID: "c-a", Name: "Alpha", Kind: "comic"},
		{RemoteID: "c-b", Name: "Beta", Kind: "book", KindProvisional: true},
		{RemoteID: "c-m", Name: "Mixed", Kind: "book"},
		{RemoteID: "c-x", Name: "Podcasts", Kind: "", DeclineReason: "no work.kind for a podcast"},
	})
	if err != nil {
		t.Fatalf("DescribeContainers: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("described %d containers, want 4: %+v", len(got), got)
	}

	// The adapter's own answer travels verbatim, including the two fields a
	// screen renders and nothing here computes.
	if p := proposalByRef(t, got, "c-b"); p.Name != "Beta" || p.Kind != "book" || !p.KindProvisional {
		t.Errorf("the adapter's answer was not forwarded verbatim: %+v", p)
	}
	if p := proposalByRef(t, got, "c-x"); p.Kind != "" || p.DeclineReason != "no work.kind for a podcast" {
		t.Errorf("a declined container lost its reason: %+v", p)
	}

	// The counts. Every one of them would be wrong under a different exclusion:
	// c-a drops a tombstoned LINK, c-b drops a tombstoned WORK, c-m drops a
	// child kind.
	for _, want := range []struct {
		ref string
		n   int64
	}{{"c-a", 2}, {"c-b", 1}, {"c-m", 2}, {"c-x", 0}} {
		if p := proposalByRef(t, got, want.ref); p.ItemCount != want.n {
			t.Errorf("container %q counts %d items, want %d", want.ref, p.ItemCount, want.n)
		}
	}

	// Nothing on instance 1 is bound yet.
	for _, p := range got {
		if len(p.BoundTo) != 0 {
			t.Errorf("container %q reports itself bound to %+v, but nothing on instance 1 is",
				p.RemoteID, p.BoundTo)
		}
	}

	// And the container that IS bound says so, with the library's own name.
	two, err := s.DescribeContainers(t.Context(), OwnerScope(0), 2, []CatalogueContainer{
		{RemoteID: "c-z", Name: "Zed", Kind: "comic"},
	})
	if err != nil {
		t.Fatalf("DescribeContainers on instance 2: %v", err)
	}
	z := proposalByRef(t, two, "c-z")
	if z.ItemCount != 1 {
		t.Errorf("c-z counts %d items, want 1", z.ItemCount)
	}
	want := []BoundLibrary{{ID: 100, Name: "Existing Comics", Kind: "comic"}}
	if !reflect.DeepEqual(z.BoundTo, want) {
		t.Errorf("c-z's bound libraries:\n  got:  %+v\n  want: %+v", z.BoundTo, want)
	}
}

// The child-kind exclusion is DERIVED FROM childKinds, and this is what says so.
// A hand-written kind list in the SQL would satisfy every behavioural assertion
// in this file on the day it was written and drift from applyOneItem step 8 on
// the day a kind is added to the map.
func TestChildKindsExclusionIsGeneratedFromTheMap(t *testing.T) {
	pred, args := childKindsExcluded("w.kind")

	var got []string
	for _, a := range args {
		s, ok := a.(string)
		if !ok {
			t.Fatalf("argument %v is not a kind string", a)
		}
		got = append(got, s)
	}
	want := make([]string, 0, len(childKinds))
	for k := range childKinds {
		want = append(want, k)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the predicate's arguments are not childKinds:\n  got:  %v\n  want: %v", got, want)
	}
	if !strings.Contains(pred, "w.kind NOT IN (") {
		t.Errorf("the predicate is not a NOT IN over the caller's column: %q", pred)
	}
	if !childKinds["comic_issue"] {
		t.Fatal("childKinds no longer holds comic_issue, so the filing statement would file " +
			"issues into a comic library and §17.8's item count would read issues while " +
			"the library grid reads series")
	}
}

func TestDescribeContainersScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	ctx := t.Context()

	all, err := s.DescribeContainers(ctx, OwnerScope(0), 1, []CatalogueContainer{
		{RemoteID: "c-a", Name: "Alpha", Kind: "comic"},
	})
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	if all[0].ItemCount != 2 {
		t.Fatalf("the owner counts %d items in c-a, want 2 — the rest of this test measures "+
			"nothing without them", all[0].ItemCount)
	}

	// Instance 2 only. c-a's works are replicated by instance 1, so a caller who
	// cannot see instance 1 must not be told how many there are.
	hidden, err := s.DescribeContainers(ctx, Scope{UserID: 0, InstanceIDs: []int64{2}}, 1,
		[]CatalogueContainer{{RemoteID: "c-a", Name: "Alpha", Kind: "comic"}})
	if err != nil {
		t.Fatalf("instance 2's scope: %v", err)
	}
	if hidden[0].ItemCount != 0 {
		t.Errorf("c-a counts %d items under a scope that hides instance 1, want 0",
			hidden[0].ItemCount)
	}

	// The complement, on the other statement. Library 100's only source is on
	// instance 2, so a caller holding instance 1 must not learn it exists — the
	// response would carry its NAME.
	unbound, err := s.DescribeContainers(ctx, Scope{UserID: 0, InstanceIDs: []int64{1}}, 2,
		[]CatalogueContainer{{RemoteID: "c-z", Name: "Zed", Kind: "comic"}})
	if err != nil {
		t.Fatalf("instance 1's scope: %v", err)
	}
	if len(unbound[0].BoundTo) != 0 {
		t.Errorf("c-z names library %+v under a scope that hides its only instance",
			unbound[0].BoundTo)
	}

	// FAIL CLOSED. No visible instances means nothing, never everything.
	none, err := s.DescribeContainers(ctx, Scope{UserID: 0}, 2,
		[]CatalogueContainer{{RemoteID: "c-z", Name: "Zed", Kind: "comic"}})
	if err != nil {
		t.Fatalf("empty scope: %v", err)
	}
	if none[0].ItemCount != 0 || len(none[0].BoundTo) != 0 {
		t.Errorf("an empty visible set returned %d items and %+v libraries",
			none[0].ItemCount, none[0].BoundTo)
	}
}

// FIRING THE SCOPE GUARD. A scope a statement accepts and never filters on is
// indistinguishable from no scope at all, so each predicate is made INEFFECTIVE
// in the SHIPPED statement in turn and the assertion above is re-run.
func TestDescribeContainersScopeGuardFires(t *testing.T) {
	// Arm 1 — the WORK visibility predicate on the item count.
	t.Run("work visibility predicate neutralised", func(t *testing.T) {
		s := newTestStore(t)
		seedProposalsCorpus(t, s)

		scope := Scope{UserID: 0, InstanceIDs: []int64{2}}
		query, args := containerItemCountsSQL(scope, 1, []string{"c-a"})
		pred, predArgs := scope.workVisibilityPredicate("w.id")
		if !strings.Contains(query, pred) {
			t.Fatalf("the shipped count statement no longer contains the predicate this arm "+
				"rewrites, so it asserts nothing:\n%s", query)
		}
		broken := strings.Replace(query, pred, "1=1", 1)
		// The predicate's arguments are appended last, so they come off the end.
		leaked := countsFrom(t, s, broken, args[:len(args)-len(predArgs)])
		if leaked["c-a"] == 0 {
			t.Fatalf("the break was a no-op: c-a still counts nothing under instance 2's scope")
		}
		t.Logf("guard fired with the work-visibility predicate neutralised: c-a = %d",
			leaked["c-a"])
	})

	// Arm 2 — the LIBRARY visibility predicate on the already-bound half.
	t.Run("library visibility predicate neutralised", func(t *testing.T) {
		s := newTestStore(t)
		seedProposalsCorpus(t, s)

		scope := Scope{UserID: 0, InstanceIDs: []int64{1}}
		query, args := containerBindingsSQL(scope, 2, []string{"c-z"})
		pred, predArgs := scope.libraryVisibilityPredicate("l.id")
		if !strings.Contains(query, pred) {
			t.Fatalf("the shipped bindings statement no longer contains the predicate this "+
				"arm rewrites, so it asserts nothing:\n%s", query)
		}
		broken := strings.Replace(query, pred, "1=1", 1)
		rows, err := s.DB().Read().QueryContext(t.Context(), broken, args[:len(args)-len(predArgs)]...)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer func() { _ = rows.Close() }()
		var names []string
		for rows.Next() {
			var ref, name, kind string
			var id int64
			if err := rows.Scan(&ref, &id, &name, &kind); err != nil {
				t.Fatalf("scan: %v", err)
			}
			names = append(names, name)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		if len(names) == 0 {
			t.Fatalf("the break was a no-op: instance 1's scope still hides library 100")
		}
		t.Logf("guard fired with the library-visibility predicate neutralised: %v", names)
	})
}

func countsFrom(t *testing.T, s *Store, query string, args []any) map[string]int64 {
	t.Helper()
	rows, err := s.DB().Read().QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var ref string
		var n int64
		if err := rows.Scan(&ref, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[ref] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// ── AcceptLibraries: membership ─────────────────────────────────────────────

func TestAcceptFilesExactlyOneContainersTopLevelWorks(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	got, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		acceptance("Alpha Comics", "comic", source(1, "c-a")),
	})
	if err != nil {
		t.Fatalf("AcceptLibraries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("accepted %d libraries, want 1: %+v", len(got), got)
	}
	lib := got[0]
	if !lib.Created || lib.Joined {
		t.Errorf("a library over an unbound container was not created: %+v", lib)
	}
	if lib.Slug != "alpha-comics" || lib.Kind != "comic" {
		t.Errorf("the created row is not what was accepted: %+v", lib)
	}
	if lib.MembersFiled != 2 {
		t.Errorf("filed %d members, want 2", lib.MembersFiled)
	}

	// EXACTNESS. Works 1 and 2 are in c-a and land; work 9 is in c-a with a
	// tombstoned link and does not; works 3-8 are in other containers and do
	// not. Each of those is a different exclusion and each has a row.
	if want := []int64{1, 2}; !reflect.DeepEqual(membersOf(t, s, lib.ID), want) {
		t.Errorf("library %d holds %v, want %v", lib.ID, membersOf(t, s, lib.ID), want)
	}

	// A tombstoned WORK is a different stamp from a tombstoned LINK, so it gets
	// its own container and its own assertion.
	book, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		acceptance("Beta Books", "book", source(1, "c-b")),
	})
	if err != nil {
		t.Fatalf("AcceptLibraries (c-b): %v", err)
	}
	if want := []int64{3}; !reflect.DeepEqual(membersOf(t, s, book[0].ID), want) {
		t.Errorf("library %d holds %v, want %v — work 4's WORK row is tombstoned",
			book[0].ID, membersOf(t, s, book[0].ID), want)
	}
}

// ADR-0066 decision 5's shape: ONE container ref, TWO libraries, and the
// trailing `w.kind = ?` is the only thing keeping the novels out of the comic
// library and the series out of the book library.
func TestAcceptSplitsAMixedContainerByKind(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	got, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		acceptance("Mixed Prose", "book", source(1, "c-m")),
		acceptance("Mixed Comics", "comic", source(1, "c-m")),
	})
	if err != nil {
		t.Fatalf("AcceptLibraries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("accepted %d libraries, want 2: %+v", len(got), got)
	}
	prose, comics := got[0], got[1]
	if want := []int64{5}; !reflect.DeepEqual(membersOf(t, s, prose.ID), want) {
		t.Errorf("the book library holds %v, want %v — a mixed container filed everything "+
			"into both of its libraries", membersOf(t, s, prose.ID), want)
	}
	if want := []int64{6}; !reflect.DeepEqual(membersOf(t, s, comics.ID), want) {
		t.Errorf("the comic library holds %v, want %v", membersOf(t, s, comics.ID), want)
	}
	if prose.MembersFiled != 1 || comics.MembersFiled != 1 {
		t.Errorf("filed %d and %d members, want 1 and 1", prose.MembersFiled, comics.MembersFiled)
	}

	// The child kind is in neither, which is the assertion the count above
	// cannot make on its own: work 7 is a comic_issue in c-m under work 6.
	for _, lib := range got {
		for _, w := range membersOf(t, s, lib.ID) {
			if w == 7 {
				t.Errorf("library %d filed the comic_issue work 7; §17.8's item count would "+
					"then read issues while /library/comics reads series", lib.ID)
			}
		}
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member WHERE work_id = 7`); n != 0 {
		t.Errorf("work 7 has %d member rows anywhere, want 0", n)
	}
}

func TestAcceptIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	accepts := []LibraryAcceptance{acceptance("Alpha Comics", "comic", source(1, "c-a"))}
	first, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, accepts)
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}
	libraries := count(t, s, `SELECT COUNT(*) FROM library`)
	sources := count(t, s, `SELECT COUNT(*) FROM library_source`)
	members := count(t, s, `SELECT COUNT(*) FROM library_member`)
	scopes := count(t, s, `SELECT COUNT(*) FROM search_doc_library`)
	if members == 0 {
		t.Fatal("nothing was filed, so re-running proves nothing about duplication")
	}

	second, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, accepts)
	if err != nil {
		t.Fatalf("second accept: %v", err)
	}
	if second[0].ID != first[0].ID {
		t.Errorf("the second accept created library %d beside %d", second[0].ID, first[0].ID)
	}
	if second[0].Created || !second[0].Joined {
		t.Errorf("the second accept reports Created=%v Joined=%v, want a join",
			second[0].Created, second[0].Joined)
	}
	if second[0].MembersFiled != 0 {
		t.Errorf("the second accept filed %d members; MembersFiled counts works that MOVED, "+
			"not the library's size", second[0].MembersFiled)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library`); n != libraries {
		t.Errorf("library rows went %d → %d", libraries, n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_source`); n != sources {
		t.Errorf("library_source rows went %d → %d", sources, n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member`); n != members {
		t.Errorf("library_member rows went %d → %d", members, n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM search_doc_library`); n != scopes {
		t.Errorf("search_doc_library rows went %d → %d", scopes, n)
	}
}

// ── AcceptLibraries: the merge key and its refusals ─────────────────────────

func TestAcceptJoinsOnTheNameKey(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	before := count(t, s, `SELECT COUNT(*) FROM library`)
	got, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		// §17.8's key: case-insensitive, whitespace-trimmed, per user.
		acceptance("  existing COMICS  ", "comic", source(1, "c-a")),
	})
	if err != nil {
		t.Fatalf("AcceptLibraries: %v", err)
	}
	if got[0].ID != 100 || !got[0].Joined || got[0].Created {
		t.Fatalf("a rename onto an existing name did not join it: %+v", got[0])
	}
	if got[0].Name != "Existing Comics" {
		t.Errorf("the joined library was renamed to %q; a join must not reshape the row it "+
			"joins (§17.8's one-way door runs in this direction)", got[0].Name)
	}
	// ⚠️ AND THE SLUG, WHICH IS THE HALF A NAME COMPARISON MISSES. This assertion
	// came here from cmd/usarr's
	// TestASecondKindInALaterImportDoesNotRenameWhatIsAlreadyThere when ADR-0048
	// moved name derivation out of the import and into Accept: that test caught a
	// library being re-slugged to make room for a kind that arrived later, and
	// the slug is what every permalink the owner holds contains (slugify:
	// "durable by design — a rename must not change the permalink"). Accept is
	// now the only path that could rewrite one, so the guard belongs here too.
	var joinedSlug string
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT slug FROM library WHERE id = ?`, got[0].ID).Scan(&joinedSlug); err != nil {
		t.Fatalf("read the joined library's slug: %v", err)
	}
	if joinedSlug != "existing-comics" || got[0].Slug != joinedSlug {
		t.Errorf("the joined library's slug is %q (reported %q), want %q unchanged",
			joinedSlug, got[0].Slug, "existing-comics")
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library`); n != before {
		t.Errorf("library rows went %d → %d; the join created a row", before, n)
	}
	// It joined AND it filed: work 8 was already a member, works 1 and 2 arrive
	// with the new source.
	if want := []int64{1, 2, 8}; !reflect.DeepEqual(membersOf(t, s, 100), want) {
		t.Errorf("library 100 holds %v, want %v", membersOf(t, s, 100), want)
	}
	if got[0].MembersFiled != 2 {
		t.Errorf("filed %d members into the joined library, want 2", got[0].MembersFiled)
	}
}

func TestAcceptRefusesANameThatIsTakenAndCannotBeJoined(t *testing.T) {
	for _, tc := range []struct {
		name string
		acc  LibraryAcceptance
		why  string
	}{
		{
			name: "same name key, different kind",
			acc:  acceptance("Existing Comics", "book", source(1, "c-b")),
			why:  "library.kind is exactly one value, so this cannot join",
		},
		{
			name: "the reserved Unfiled row holds the name",
			acc:  acceptance("unfiled", "book", source(1, "c-b")),
			why:  "nothing may join library 0 and ux_library_name is over every row",
		},
		{
			name: "the slug is taken while the name is free",
			acc:  acceptance("Existing-Comics", "comic", source(1, "c-a")),
			why:  "slugify is lossy: two names, one permalink",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			seedProposalsCorpus(t, s)
			before := count(t, s, `SELECT COUNT(*) FROM library`)
			beforeMembers := count(t, s, `SELECT COUNT(*) FROM library_member`)

			got, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0,
				[]LibraryAcceptance{tc.acc})
			if !errors.Is(err, ErrLibraryNameTakenAtOtherKind) {
				t.Fatalf("accepting %q returned (%+v, %v), want ErrLibraryNameTakenAtOtherKind "+
					"— %s", tc.acc.Name, got, err, tc.why)
			}
			if got != nil {
				t.Errorf("a refused batch returned %+v", got)
			}
			if n := count(t, s, `SELECT COUNT(*) FROM library`); n != before {
				t.Errorf("library rows went %d → %d on a refused accept", before, n)
			}
			if n := count(t, s, `SELECT COUNT(*) FROM library_member`); n != beforeMembers {
				t.Errorf("library_member rows went %d → %d on a refused accept", beforeMembers, n)
			}
		})
	}
}

// ONE BAD ACCEPTANCE TAKES THE WHOLE BATCH, and that is the documented decision
// rather than an accident of the loop. See AcceptLibraries.
func TestAcceptIsAllOrNothing(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	before := count(t, s, `SELECT COUNT(*) FROM library`)

	_, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		acceptance("Perfectly Fine", "comic", source(1, "c-a")),
		acceptance("Existing Comics", "book", source(1, "c-b")),
	})
	if !errors.Is(err, ErrLibraryNameTakenAtOtherKind) {
		t.Fatalf("AcceptLibraries returned %v, want ErrLibraryNameTakenAtOtherKind", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library`); n != before {
		t.Errorf("library rows went %d → %d: the good acceptance ahead of the bad one was "+
			"committed", before, n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE name = 'Perfectly Fine'`); n != 0 {
		t.Errorf("the first acceptance survived a batch that failed")
	}
}

func TestAcceptRefusesASourceOutsideTheScope(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	before := count(t, s, `SELECT COUNT(*) FROM library`)

	// The caller can see instance 2 and names a container on instance 1.
	_, err := s.AcceptLibraries(t.Context(), Scope{UserID: 0, InstanceIDs: []int64{2}}, 0,
		[]LibraryAcceptance{acceptance("Alpha Comics", "comic", source(1, "c-a"))})
	if !errors.Is(err, ErrSourceOutsideScope) {
		t.Fatalf("AcceptLibraries returned %v, want ErrSourceOutsideScope", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library`); n != before {
		t.Errorf("library rows went %d → %d on a refused accept", before, n)
	}

	// FAIL CLOSED: an empty visible set admits nothing, not everything.
	if _, err := s.AcceptLibraries(t.Context(), Scope{UserID: 0}, 0,
		[]LibraryAcceptance{acceptance("Alpha Comics", "comic", source(1, "c-a"))}); !errors.Is(
		err, ErrSourceOutsideScope) {
		t.Fatalf("an empty visible set returned %v, want ErrSourceOutsideScope", err)
	}

	// And the owner, who can see both, is not refused — otherwise the two
	// assertions above would pass on a function that refuses everything.
	if _, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0,
		[]LibraryAcceptance{acceptance("Alpha Comics", "comic", source(1, "c-a"))}); err != nil {
		t.Fatalf("the owner's own scope was refused: %v", err)
	}
}

func TestAcceptValidatesWhatTheCallerDecides(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	for _, tc := range []struct {
		name string
		acc  LibraryAcceptance
	}{
		{"no name", LibraryAcceptance{Name: "   ", Kind: "comic", ManagedBy: "user"}},
		{"no kind", LibraryAcceptance{Name: "Nameless Kind", ManagedBy: "user"}},
		{"managed_by is neither", LibraryAcceptance{
			Name: "Third State", Kind: "comic", ManagedBy: "proposed",
		}},
		{"a container kind with no derivation", LibraryAcceptance{
			Name: "Rooted", Kind: "comic", ManagedBy: "user",
			Sources: []AcceptedSource{{
				ServiceInstanceID: 1, ContainerKind: "root_folder", ContainerRef: "/comics",
			}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0,
				[]LibraryAcceptance{tc.acc}); err == nil {
				t.Fatal("accepted, want an error naming what the caller got wrong")
			}
		})
	}
}

// managed_by = 'user' has never been written by any code path (ADR-0048 Fact 1),
// and this is the path that can. The column has no reader yet, so the assertion
// is on the row.
func TestAcceptWritesTheCallersManagedByAndFormats(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	got, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		{
			Name: "Ebooks", Kind: "book", Formats: []string{"ebook"}, ManagedBy: "user",
			Sources: []AcceptedSource{source(1, "c-b")},
		},
		{
			Name: "Everything Else", Kind: "comic", ManagedBy: "auto",
			Sources: []AcceptedSource{source(1, "c-a")},
		},
	})
	if err != nil {
		t.Fatalf("AcceptLibraries: %v", err)
	}
	var managed string
	var formats sql.NullString
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT managed_by, formats FROM library WHERE id = ?`, got[0].ID).
		Scan(&managed, &formats); err != nil {
		t.Fatalf("read the accepted library: %v", err)
	}
	if managed != "user" {
		t.Errorf("managed_by = %q, want 'user' — the screen decides it and the store stores it",
			managed)
	}
	if !formats.Valid || formats.String != `["ebook"]` {
		t.Errorf("formats = %v, want [\"ebook\"]", formats)
	}

	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT managed_by, formats FROM library WHERE id = ?`, got[1].ID).
		Scan(&managed, &formats); err != nil {
		t.Fatalf("read the accepted library: %v", err)
	}
	if managed != "auto" {
		t.Errorf("managed_by = %q, want 'auto'", managed)
	}
	if formats.Valid {
		t.Errorf("formats = %v, want NULL — nil means any format", formats)
	}
}

// ── AcceptLibraries: the search-document rescope ────────────────────────────

func TestAcceptRescopesSearchDocumentsAndKeepsInvariantFive(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	// The state ADR-0048's ordering leaves behind, asserted rather than assumed:
	// the import ran first, so every unfiled work's document sits at library 0.
	for _, w := range []int64{1, 2, 3, 5, 6, 9} {
		if want := []int64{UnfiledLibraryID}; !reflect.DeepEqual(docScopes(t, s, w), want) {
			t.Fatalf("work %d's document is scoped to %v before Accept, want %v — this test "+
				"measures the repair and there is nothing to repair", w, docScopes(t, s, w), want)
		}
	}

	got, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		acceptance("Alpha Comics", "comic", source(1, "c-a")),
	})
	if err != nil {
		t.Fatalf("AcceptLibraries: %v", err)
	}
	lib := got[0].ID

	// The filed works are scoped to the real library and the fallback row is
	// GONE. Leaving it would make the work findable under a scope no chip
	// offers, beside the one it now belongs to.
	for _, w := range []int64{1, 2} {
		if want := []int64{lib}; !reflect.DeepEqual(docScopes(t, s, w), want) {
			t.Errorf("work %d's document is scoped to %v, want %v", w, docScopes(t, s, w), want)
		}
	}
	// The works that stayed unfiled keep theirs. Invariant 5 is about them.
	for _, w := range []int64{3, 5, 6, 9} {
		if want := []int64{UnfiledLibraryID}; !reflect.DeepEqual(docScopes(t, s, w), want) {
			t.Errorf("work %d stayed unfiled but its document is scoped to %v, want %v",
				w, docScopes(t, s, w), want)
		}
	}
	// And work 8, which was already a member of library 100, is untouched.
	if want := []int64{100}; !reflect.DeepEqual(docScopes(t, s, 8), want) {
		t.Errorf("work 8's document is scoped to %v, want %v", docScopes(t, s, 8), want)
	}

	if n := docsWithNoScope(t, s); n != 0 {
		t.Errorf("§7 invariant 5 broken: %d documents have no scope at all", n)
	}
	// The document tables themselves are untouched by any of this: the rescope
	// writes the junction and nothing else.
	assertCorpusInvariants(t, s, 8)
}

// ── the plan guard ──────────────────────────────────────────────────────────
//
// MEASURED — SQLite 3.53.4 (github.com/ncruces/go-sqlite3), this schema, the
// corpus above, NO ANALYZE:
//
//	SEARCH sil USING INDEX ix_sil_container (service_instance_id=? AND remote_library_id=?)
//	SEARCH w USING INTEGER PRIMARY KEY (rowid=?)
//	USE TEMP B-TREE FOR DISTINCT
//
// THE THIRD STEP IS EXPECTED AND IS NOT ASSERTED AWAY, the same posture
// TestListLibrariesOrderingSortIsIntentional takes for its sort: `SELECT
// DISTINCT` over (constant, w.sort_title, w.id, constant) is not a set SQLite
// can prove unique from an index, so it sorts. It is bounded by the container
// being filed rather than by the catalogue, and it is what states the
// statement's grain — one row per WORK — rather than leaving the row count to
// depend on how many links a work happens to hold in one container. Forbidding
// it here would fail on the first honest run.
//
// DescribeContainers' own count statement was measured at the same time and
// seeks the same index on both key columns —
// `SEARCH sil USING INDEX ix_sil_container (service_instance_id=? AND
// remote_library_id=?) | SEARCH w USING INTEGER PRIMARY KEY (rowid=?) | USE TEMP
// B-TREE FOR count(DISTINCT)` — so the GROUP BY comes off the index order and
// costs no second sort.
//
// ix_sil_container is migration 0005's
// `(service_instance_id, remote_library_id) WHERE deleted_at IS NULL AND
// remote_library_id IS NOT NULL`, declared for this exact join — *"the
// membership derivation walks links per instance per container"* — and until
// this commit it had no reader in non-test Go. The filing statement is the leg
// that grows with the catalogue: without the index it is a scan of every link on
// the instance, per source, per acceptance.
//
// The statement is an INSERT … SELECT, and EXPLAIN QUERY PLAN prepares it
// without executing it, so the guard runs on the READ pool like every other plan
// guard in this package.

func fileMembersPlan(t *testing.T, s *Store) string {
	t.Helper()
	query, args := fileContainerMembersSQL(1, "comic", source(1, "c-a"), "2026-08-16 12:00:00")
	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// fileMembersPlanFaults is the assertion the guard and its firing both run, so
// neither can drift from the other.
func fileMembersPlanFaults(plan string) []string {
	var faults []string
	if !strings.Contains(plan, "SEARCH sil USING INDEX") || !planHas(plan, "ix_sil_container") {
		faults = append(faults,
			"the link leg is not a seek on ix_sil_container, so filing one container walks "+
				"every link on the instance")
	}
	if !strings.Contains(plan, "(service_instance_id=? AND remote_library_id=?)") {
		faults = append(faults,
			"the link leg does not constrain both key columns, so it seeks the instance and "+
				"filters the container")
	}
	if strings.Contains(plan, "SCAN ") {
		faults = append(faults, "the plan contains a SCAN; both legs of this write are seeks")
	}
	if !strings.Contains(plan, "SEARCH w USING INTEGER PRIMARY KEY") {
		faults = append(faults, "the work join is not a rowid lookup")
	}
	return faults
}

func TestAcceptFilingPlanIsASeek(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	plan := fileMembersPlan(t, s)
	if faults := fileMembersPlanFaults(plan); len(faults) > 0 {
		t.Errorf("the membership filing plan degraded:\n  plan:   %s\n  faults: %v", plan, faults)
	}
	t.Logf("membership filing plan: %s", plan)
}

// FIRING IT. A plan guard that has never been watched failing is
// indistinguishable from no guard, so the index it names is dropped and the
// SAME assertion is re-run.
func TestAcceptFilingPlanGuardFires(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)

	// The drop is on the write connection and the EXPLAIN is on the read pool,
	// which is stale until something else reads — see dropIndexesAndConfirm.
	dropIndexesAndConfirm(t, s, "ix_sil_container")

	plan := fileMembersPlan(t, s)
	faults := fileMembersPlanFaults(plan)
	if len(faults) == 0 {
		t.Fatalf("the guard passed a plan with ix_sil_container dropped:\n  %s", plan)
	}
	t.Logf("guard fired with ix_sil_container dropped:\n  plan:   %s\n  faults: %v", plan, faults)
}

// ─────────────────────────────────────────────────────────────────────────────
// ProposedContainers — §17.8's Accept screen read off local state.
//
// The corpus above is reused unchanged; what these tests add is the
// `container_observed` rows a real import writes, through the SHIPPED writer.
// Nothing here hand-builds a sync_report row except where the point IS a row no
// shipped writer would produce.
// ─────────────────────────────────────────────────────────────────────────────

// observe writes one container_observed row per container, exactly as the bind
// transaction does — recordContainerObservation, not an INSERT of this file's
// own. A test that hand-wrote the row would still pass if the detail blob's
// field names changed on one side only, which is the drift DEVELOPMENT.md §11
// names.
func observe(t *testing.T, s *Store, instanceID int64, cs ...CatalogueContainer) {
	t.Helper()
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, c := range cs {
			if err := recordContainerObservation(ctx, tx, instanceID, c); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("observe containers on instance %d: %v", instanceID, err)
	}
}

// reclock moves the created_at of every observation of one container.
//
// It is a bare UPDATE because there is no shipped writer for it and there must
// not be: `sync_report.created_at` is SQLite's `datetime('now')` default and the
// import has no business setting it. The row itself is still written by the
// shipped writer above; only its clock is moved, which is the only way to test a
// comparison against a stamp without sleeping through real seconds.
func reclock(t *testing.T, s *Store, instanceID int64, ref, at string) {
	t.Helper()
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE sync_report SET created_at = ?
			 WHERE service_instance_id = ? AND kind = ? AND remote_kind = 'library'
			   AND remote_id = ?`, at, instanceID, SyncReportContainerObserved, ref)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("no observation of %q on instance %d to reclock", ref, instanceID)
		}
		return nil
	}); err != nil {
		t.Fatalf("reclock %q: %v", ref, err)
	}
}

// stampSync stamps last_full_sync_at through the shipped writer.
func stampSync(t *testing.T, s *Store, instanceID int64, at string) {
	t.Helper()
	parsed, err := ParseTime(at)
	if err != nil {
		t.Fatalf("parse %q: %v", at, err)
	}
	if err := s.StampFullSync(t.Context(), instanceID, parsed); err != nil {
		t.Fatalf("stamp instance %d: %v", instanceID, err)
	}
}

func proposalFor(t *testing.T, got []ContainerProposal, instance int64, ref string) ContainerProposal {
	t.Helper()
	for _, p := range got {
		if p.ServiceInstanceID == instance && p.RemoteID == ref {
			return p
		}
	}
	t.Fatalf("no proposal for container %q on instance %d in %+v", ref, instance, got)
	return ContainerProposal{}
}

func hasProposal(got []ContainerProposal, instance int64, ref string) bool {
	for _, p := range got {
		if p.ServiceInstanceID == instance && p.RemoteID == ref {
			return true
		}
	}
	return false
}

// proposalRefs renders the result as "instance/ref" strings, in order, for an
// assertion that wants to say WHICH containers came back rather than how many.
func proposalRefs(got []ContainerProposal) []string {
	out := make([]string, 0, len(got))
	for _, p := range got {
		out = append(out, fmt.Sprintf("%d/%s", p.ServiceInstanceID, p.RemoteID))
	}
	return out
}

// seedObservations is the container list the corpus's two instances reported.
// c-z is deliberately bound already (library 100, kind comic) and c-m is
// deliberately mixed.
func seedObservations(t *testing.T, s *Store) {
	t.Helper()
	observe(t, s, 1,
		CatalogueContainer{RemoteID: "c-a", Name: "Alpha", Kind: "comic"},
		CatalogueContainer{RemoteID: "c-b", Name: "Beta", Kind: "book", KindProvisional: true},
		CatalogueContainer{RemoteID: "c-m", Name: "Mixed", Kind: "book"},
		CatalogueContainer{RemoteID: "c-x", Name: "Podcasts", DeclineReason: "no work.kind for a podcast"},
	)
	observe(t, s, 2, CatalogueContainer{RemoteID: "c-z", Name: "Zed", Kind: "comic"})
}

// THE HEADLINE: the screen's whole row, from the local file, with no adapter
// anywhere near it.
//
// ⚠️ WHAT THIS CATCHES that no other test here does: a field left at its zero
// value. Every field ProposedContainers promises is asserted on ONE proposal, so
// a merge that adds a field to the struct and forgets to fill it in the loop
// goes red here rather than rendering an empty cell on the screen.
func TestProposedContainersRendersTheAcceptScreenFromLocalState(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}

	// c-z is bound to library 100 at kind comic, so it is not on offer. Every
	// other observed container is.
	want := []string{"1/c-a", "1/c-b", "1/c-m", "1/c-x"}
	if !reflect.DeepEqual(proposalRefs(got), want) {
		t.Fatalf("proposed %v, want %v", proposalRefs(got), want)
	}

	// ⚠️ ONE `if` PER FIELD, NEVER A `switch`. A switch stops at the first true
	// case, so a change that blanked THREE fields would report one of them and
	// the next run would report the next — which is a guard that makes a reader
	// fix a defect one field at a time.
	p := proposalFor(t, got, 1, "c-b")
	if p.Name != "Beta" {
		t.Errorf("Name = %q, want the name upstream reported", p.Name)
	}
	if p.Kind != "book" {
		t.Errorf("Kind = %q, want book", p.Kind)
	}
	if !p.KindProvisional {
		t.Errorf("KindProvisional = false; the adapter's guess flag did not survive the " +
			"round trip through the observation row")
	}
	if p.Declined() {
		t.Errorf("c-b reads as declined although it has a kind")
	}
	if p.ServiceInstanceID != 1 {
		t.Errorf("ServiceInstanceID = %d, want 1", p.ServiceInstanceID)
	}
	if p.ServiceName != "Kavita One" {
		t.Errorf("ServiceName = %q, want %q", p.ServiceName, "Kavita One")
	}
	if p.ServiceKind != "kavita" {
		t.Errorf("ServiceKind = %q, want kavita", p.ServiceKind)
	}
	// work 3 is live in c-b; work 4 is tombstoned. A count of 2 means the
	// composition dropped DescribeContainers' tombstone handling.
	if p.ItemCount != 1 {
		t.Errorf("ItemCount = %d, want 1", p.ItemCount)
	}
	if p.SuggestedName != "Beta" {
		t.Errorf("SuggestedName = %q, want Beta", p.SuggestedName)
	}
	if p.ObservedAt == "" {
		t.Errorf("ObservedAt is empty, so the screen cannot date the proposal at all")
	}
	if len(p.BoundTo) != 0 {
		t.Errorf("BoundTo = %+v, want empty", p.BoundTo)
	}
}

// ⚠️ AND IT CALLS NOTHING OUTBOUND, which is the property principle 1 rests on
// and the one a signature cannot state. The check is structural rather than
// behavioural: this file must not reach an adapter or the sync package at all,
// because a proposal read that can block on a Kavita being off is the shape
// ADR-0048's clause 5 excuses ONCE, at connect, and never on a settings screen.
func TestProposalsReachNoAdapterAndNoSyncPackage(t *testing.T) {
	src, err := os.ReadFile("proposals.go")
	if err != nil {
		t.Fatalf("read proposals.go: %v", err)
	}
	for _, forbidden := range []string{"libsync", "adapter", "net/http"} {
		if strings.Contains(string(src), `"`+forbidden) ||
			strings.Contains(string(src), "/"+forbidden+`"`) {
			t.Errorf("proposals.go imports %q. The Accept screen renders from local SQLite; "+
				"a package that can make a request is one refactor away from doing it on a "+
				"render path.", forbidden)
		}
	}
}

// A CONTAINER ALREADY BOUND AT THE OBSERVED KIND IS NOT A PROPOSAL, and one
// bound only at ANOTHER kind still is (ADR-0066 decision 5).
//
// ⚠️ WHAT THIS CATCHES: a ref-keyed exclusion. c-m is a mixed container — prose
// and comics in one BookOrbit library. Accepting its `book` half must leave the
// `comic` half proposable, or the comics in it can never become a library on the
// only screen that creates one. A `NOT IN (SELECT container_ref …)` would pass
// every other test in this file and fail exactly here.
func TestProposedContainersDropsABindingOnlyAtItsOwnKind(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)
	ctx := t.Context()

	// The book half of the mixed container is accepted.
	if _, err := s.AcceptLibraries(ctx, OwnerScope(0), 0, []LibraryAcceptance{
		{Name: "Mixed", Kind: "book", ManagedBy: "auto", Sources: []AcceptedSource{source(1, "c-m")}},
	}); err != nil {
		t.Fatalf("accept the book half: %v", err)
	}

	got, err := s.ProposedContainers(ctx, OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	if hasProposal(got, 1, "c-m") {
		t.Errorf("c-m is still proposed after its `book` library was accepted: %v",
			proposalRefs(got))
	}

	// Now the adapter reports the SAME container at `comic` — the second kind
	// ADR-0066 decision 5 is about. It is a proposal again, and the library it
	// would sit beside travels with it.
	observe(t, s, 1, CatalogueContainer{RemoteID: "c-m", Name: "Mixed", Kind: "comic"})
	got, err = s.ProposedContainers(ctx, OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers after the second kind: %v", err)
	}
	p := proposalFor(t, got, 1, "c-m")
	if p.Kind != "comic" {
		t.Errorf("c-m is proposed at kind %q, want comic", p.Kind)
	}
	if len(p.BoundTo) != 1 || p.BoundTo[0].Kind != "book" {
		t.Errorf("BoundTo = %+v, want the `book` library it would sit beside — the screen "+
			"cannot explain a second library over one container without naming the first",
			p.BoundTo)
	}
}

// A DECLINED CONTAINER TRAVELS WITH ITS REASON, and can never be dropped by the
// already-bound rule.
func TestProposedContainersKeepsADeclinedContainer(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	p := proposalFor(t, got, 1, "c-x")
	if !p.Declined() {
		t.Errorf("c-x does not read as declined: %+v", p)
	}
	if p.DeclineReason != "no work.kind for a podcast" {
		t.Errorf("DeclineReason = %q, want the adapter's own words — §17.8 renders it in "+
			"the Decision column and a decline with no reason is an empty row", p.DeclineReason)
	}
	if p.Kind != "" {
		t.Errorf("Kind = %q on a declined container, want empty", p.Kind)
	}
}

// THE NEWEST OBSERVATION WINS, PER CONTAINER. sync_report is append-only, so
// every import leaves another row; a read that took any row but the last would
// render a name or a kind upstream has since changed.
//
// ⚠️ WHAT THIS CATCHES: dropping the correlated `r.id = (… ORDER BY r2.id DESC
// LIMIT 1)`. Without it every historical row is its own proposal and the screen
// offers the same container once per import it has ever run.
func TestProposedContainersTakesTheNewestObservationPerContainer(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	// A second import: c-a was renamed upstream and retyped.
	observe(t, s, 1, CatalogueContainer{RemoteID: "c-a", Name: "Alpha Prime", Kind: "book"})

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	n := 0
	for _, p := range got {
		if p.RemoteID == "c-a" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("c-a is proposed %d times, want 1: %v", n, proposalRefs(got))
	}
	p := proposalFor(t, got, 1, "c-a")
	if p.Name != "Alpha Prime" || p.Kind != "book" {
		t.Errorf("c-a reads %q/%q, want the newest observation's Alpha Prime/book",
			p.Name, p.Kind)
	}
}

// ObservedAt IS PER CONTAINER AND IS NEVER COLLAPSED TO ONE PER INSTANCE.
//
// ⚠️ WHAT THIS CATCHES: a read that stamped every proposal with the instance's
// last_full_sync_at, or with MAX(created_at) over the instance. Both are one
// statement simpler and both are wrong the moment an import fails partway: the
// containers it never reached keep an older row, and the screen would date them
// by a run that did not see them.
func TestProposedContainersStampsEachContainerSeparately(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	reclock(t, s, 1, "c-a", "2026-08-10 09:00:00")
	reclock(t, s, 1, "c-b", "2026-08-20 09:00:00")

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	a := proposalFor(t, got, 1, "c-a").ObservedAt
	b := proposalFor(t, got, 1, "c-b").ObservedAt
	if a != "2026-08-10 09:00:00" {
		t.Errorf("c-a ObservedAt = %q, want its own row's stamp", a)
	}
	if b != "2026-08-20 09:00:00" {
		t.Errorf("c-b ObservedAt = %q, want its own row's stamp", b)
	}
	if a == b {
		t.Errorf("both containers report the same ObservedAt %q, so the stamp is "+
			"instance-grained and the two containers cannot be told apart", a)
	}
}

// NotSeenByLastSync, INCLUDING THE BOUNDARY THE COMPARISON TURNS ON.
//
// Three arms and the middle one is the whole point: an observation stamped
// EXACTLY at last_full_sync_at counts as SEEN, because last_full_sync_at holds
// the run's START time and both values are second-granular, so same-second is
// indistinguishable from just-after. See notSeenByLastSync.
func TestProposedContainersFlagsWhatTheLastSyncDidNotReport(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	const stamp = "2026-08-20 09:00:00"
	reclock(t, s, 1, "c-a", "2026-08-19 23:59:59") // strictly before  → not seen
	reclock(t, s, 1, "c-b", stamp)                 // exactly equal    → SEEN
	reclock(t, s, 1, "c-m", "2026-08-20 09:00:01") // strictly after   → seen
	stampSync(t, s, 1, stamp)

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	for _, tc := range []struct {
		ref     string
		notSeen bool
		why     string
	}{
		{"c-a", true, "its newest observation predates the last completed run, so that " +
			"run did not report it"},
		{"c-b", false, "an observation stamped EXACTLY at last_full_sync_at is SEEN: the " +
			"stamp is the run's START time and both are second-granular, so a strict " +
			"comparison would flag every container of every fast import as gone"},
		{"c-m", false, "its observation is after the stamp"},
	} {
		p := proposalFor(t, got, 1, tc.ref)
		if p.NotSeenByLastSync != tc.notSeen {
			t.Errorf("%s NotSeenByLastSync = %v, want %v — %s (observed %q, stamp %q)",
				tc.ref, p.NotSeenByLastSync, tc.notSeen, tc.why, p.ObservedAt,
				p.InstanceLastFullSyncAt)
		}
		if p.InstanceLastFullSyncAt != stamp {
			t.Errorf("%s InstanceLastFullSyncAt = %q, want %q", tc.ref,
				p.InstanceLastFullSyncAt, stamp)
		}
	}

	// AND A CONTAINER NEVER DISAPPEARS FROM THE SCREEN because of it. Hiding it
	// would make "vanished upstream" indistinguishable from "never existed", on
	// the only screen that could say which.
	if !hasProposal(got, 1, "c-a") {
		t.Errorf("c-a was hidden rather than flagged: %v", proposalRefs(got))
	}
}

// AN INSTANCE THAT HAS NEVER COMPLETED A FULL SYNC flags nothing, because there
// is no completed run for a container to have been missed by. Its stamp is
// empty, which is a fact the screen can render and not a zero timestamp.
//
// ⚠️ WHAT THIS CATCHES, STATED NARROWLY BECAUSE THE DRILL SAID SO. Deleting
// notSeenByLastSync's `lastFullSyncAt == ""` branch does NOT turn this red —
// measured — because the empty string already sorts before every timestamp, so
// the branch is redundant with the comparison beneath it. What DOES turn it red
// is the branch answering the other way, and so would any rewrite that gave the
// missing stamp a value: an epoch flags nothing forever, a far-future sentinel
// flags everything, and the container that vanished from a never-synced instance
// is indistinguishable either way. This guards the ANSWER at a missing stamp,
// not the presence of a line of code.
func TestProposedContainersFlagsNothingOnANeverSyncedInstance(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)
	reclock(t, s, 1, "c-a", "2020-01-01 00:00:00")

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	p := proposalFor(t, got, 1, "c-a")
	if p.NotSeenByLastSync {
		t.Errorf("c-a is flagged as unseen although instance 1 has never completed a full sync")
	}
	if p.InstanceLastFullSyncAt != "" {
		t.Errorf("InstanceLastFullSyncAt = %q, want empty for never", p.InstanceLastFullSyncAt)
	}
}

// THE JOIN PREDICTION IS THE WRITER'S OWN ANSWER, ASKED EARLY.
//
// ⚠️ WHAT THIS CATCHES: a prediction computed on the raw name instead of §17.8's
// merge key, or one that ignores the kind. Both would put "will join Existing
// Comics" on a row that then creates a second library, or refuse a name that
// would have joined.
func TestProposedContainersPredictsTheJoinTheWriterWouldMake(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	// The name differs from library 100's only by case and whitespace, which is
	// exactly what libraryNameKey normalises away, and the kind matches.
	observe(t, s, 1, CatalogueContainer{RemoteID: "c-a", Name: "  existing COMICS  ", Kind: "comic"})
	// Same name key, WRONG kind. This one must not predict a join: §17.8's merge
	// is name AND kind, and ErrLibraryNameTakenAtOtherKind is what it really gets.
	observe(t, s, 1, CatalogueContainer{RemoteID: "c-b", Name: "Existing Comics", Kind: "book"})

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}

	joining := proposalFor(t, got, 1, "c-a")
	if joining.JoinsLibraryID != 100 {
		t.Errorf("c-a predicts JoinsLibraryID %d, want 100 — §17.8's merge key is "+
			"case-insensitive and whitespace-trimmed, so %q joins %q",
			joining.JoinsLibraryID, joining.SuggestedName, "Existing Comics")
	}
	if joining.JoinsLibraryName != "Existing Comics" {
		t.Errorf("JoinsLibraryName = %q, want the library's STORED name — the screen says "+
			"\"Joining … into Existing Comics\" and a lowered key is not a name",
			joining.JoinsLibraryName)
	}

	if p := proposalFor(t, got, 1, "c-b"); p.JoinsLibraryID != 0 {
		t.Errorf("c-b predicts a join with library %d at kind %q, but library 100 is a "+
			"`comic` library and accepting this would be refused, not joined",
			p.JoinsLibraryID, p.Kind)
	}

	// AND THE PREDICTION IS TRUE. The same acceptance is run for real and must
	// land on the library the read named.
	accepted, err := s.AcceptLibraries(t.Context(), OwnerScope(0), 0, []LibraryAcceptance{
		{
			Name: joining.SuggestedName, Kind: joining.Kind, ManagedBy: "auto",
			Sources: []AcceptedSource{source(1, "c-a")},
		},
	})
	if err != nil {
		t.Fatalf("accept the predicted join: %v", err)
	}
	if !accepted[0].Joined || accepted[0].ID != joining.JoinsLibraryID {
		t.Errorf("accepting the proposal produced %+v, but the screen predicted a join "+
			"with library %d. The prediction and the write are supposed to be ONE "+
			"derivation, not two that agree.", accepted[0], joining.JoinsLibraryID)
	}
}

// THE SUGGESTED NAME, INCLUDING THE FALLBACK THE BIND PATH USED TO OWN.
func TestProposedContainersSuggestsAName(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	observe(t, s, 1,
		CatalogueContainer{RemoteID: "c-a", Name: "  Alpha  ", Kind: "comic"},
		CatalogueContainer{RemoteID: "c-b", Name: "   ", Kind: "book"},
	)

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	if n := proposalFor(t, got, 1, "c-a").SuggestedName; n != "Alpha" {
		t.Errorf("SuggestedName = %q, want the trimmed name", n)
	}
	// ⚠️ THE NAME STILL TRAVELS UNTRIMMED beside it. The suggestion is editable
	// and the observation is evidence; a read that trimmed the evidence too
	// would leave nothing able to say what upstream actually returned.
	if n := proposalFor(t, got, 1, "c-a").Name; n != "  Alpha  " {
		t.Errorf("Name = %q, want upstream's own string verbatim", n)
	}
	if n := proposalFor(t, got, 1, "c-b").SuggestedName; n != "Library c-b" {
		t.Errorf("SuggestedName = %q for a container upstream named nothing, want %q — "+
			"a nameless row on the Accept screen is a row nobody can accept",
			n, "Library c-b")
	}
}

// A SOFT-DELETED INSTANCE OFFERS NOTHING. Its containers are not offers, and its
// rows stay in sync_report because that table is a log.
func TestProposedContainersIgnoresASoftDeletedInstance(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET deleted_at = '2026-08-21 00:00:00' WHERE id = 1`)
		return err
	}); err != nil {
		t.Fatalf("soft-delete instance 1: %v", err)
	}

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a soft-deleted instance still proposes %v — instance 2's only container "+
			"is already bound, so the whole result should be empty", proposalRefs(got))
	}
}

// AN OBSERVATION THIS CODE CANNOT DECODE IS DROPPED, AND ITS NEIGHBOURS SURVIVE.
//
// The row is written by hand because that is the point: recordContainerObservation
// marshals a struct that cannot fail, so a blob that will not decode did not come
// from it.
func TestProposedContainersDropsAnUnreadableObservation(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO sync_report (service_instance_id, kind, remote_kind, remote_id, detail)
			VALUES (1, ?, 'library', 'c-junk', 'this is not json')`, SyncReportContainerObserved)
		return err
	}); err != nil {
		t.Fatalf("write the unreadable row: %v", err)
	}

	got, err := s.ProposedContainers(t.Context(), OwnerScope(0))
	if err != nil {
		t.Fatalf("ProposedContainers: %v", err)
	}
	if hasProposal(got, 1, "c-junk") {
		t.Errorf("an undecodable observation was rendered as a proposal: %+v",
			proposalFor(t, got, 1, "c-junk"))
	}
	want := []string{"1/c-a", "1/c-b", "1/c-m", "1/c-x"}
	if !reflect.DeepEqual(proposalRefs(got), want) {
		t.Errorf("proposed %v, want %v — one unreadable row must cost one proposal, not "+
			"the screen", proposalRefs(got), want)
	}
}

// §14: EXACTLY TWO service_instance COLUMNS BESIDES THE ID AND THE SYNC STAMP.
//
// ⚠️ THIS IS A TEXT ASSERTION AND IT IS THE RIGHT SHAPE FOR THIS RULE. The
// failure it guards is a `SELECT si.*` or an added column, both of which are
// visible in the statement and invisible in the result type until somebody
// copies a field onto a wire struct. internal/httpapi/libraries.go allowlists
// the same two on the other side; this is the store half.
func TestTheProposalStatementReadsNoCredentialColumn(t *testing.T) {
	query, _ := observedContainersSQL(OwnerScope(0))
	for _, col := range []string{"api_key_enc", "base_url", "url_base", "tls_spki_pin", "si.*"} {
		if strings.Contains(query, col) {
			t.Errorf("the proposal statement names %q. service_instance carries a full-admin "+
				"credential and an internal host; §14 lets the NAME and the KIND cross this "+
				"layer and nothing else:\n%s", col, query)
		}
	}
	for _, col := range []string{"si.name", "si.kind", "si.last_full_sync_at"} {
		if !strings.Contains(query, col) {
			t.Errorf("the proposal statement no longer reads %q, so this guard is asserting "+
				"against a statement it does not describe", col)
		}
	}
}

// THE SCOPE FILTERS, on all three of the predicates this read carries.
func TestProposedContainersScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)
	ctx := t.Context()

	// Instance 2 only: instance 1's containers are not offers at all, because a
	// caller who cannot see the instance cannot be told it has containers.
	only2, err := s.ProposedContainers(ctx, Scope{UserID: 0, InstanceIDs: []int64{2}})
	if err != nil {
		t.Fatalf("instance 2's scope: %v", err)
	}
	for _, p := range only2 {
		if p.ServiceInstanceID == 1 {
			t.Errorf("instance 1's container %q is proposed to a caller who cannot see it",
				p.RemoteID)
		}
	}

	// Instance 1 only: c-z's binding hangs off instance 2, so this caller cannot
	// learn library 100 exists — and c-z itself is on instance 2, so it is
	// absent for the same reason.
	only1, err := s.ProposedContainers(ctx, Scope{UserID: 0, InstanceIDs: []int64{1}})
	if err != nil {
		t.Fatalf("instance 1's scope: %v", err)
	}
	if hasProposal(only1, 2, "c-z") {
		t.Errorf("instance 2's container is proposed under instance 1's scope")
	}
	if len(only1) != 4 {
		t.Fatalf("instance 1's scope proposes %v, want its own four", proposalRefs(only1))
	}

	// FAIL CLOSED. No visible instances means nothing, never everything.
	none, err := s.ProposedContainers(ctx, Scope{UserID: 0})
	if err != nil {
		t.Fatalf("empty scope: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("an empty visible set proposed %v", proposalRefs(none))
	}
}

// FIRING THE SCOPE GUARD. The instance predicate is made INEFFECTIVE in the
// SHIPPED statement and the read is re-run against it, exactly as
// TestDescribeContainersScopeGuardFires does for the other two predicates —
// which this read also carries, through DescribeContainers, and which that test
// already fires.
func TestProposedContainersScopeGuardFires(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	scope := Scope{UserID: 0, InstanceIDs: []int64{2}}
	query, args := observedContainersSQL(scope)
	pred, predArgs := scope.instancePredicate("si.id")
	if !strings.Contains(query, pred) {
		t.Fatalf("the shipped statement no longer contains the predicate this arm rewrites, "+
			"so it asserts nothing:\n%s", query)
	}
	broken := strings.Replace(query, pred, "1=1", 1)
	// The scope's arguments sit between the two bound kinds, so they come out of
	// the middle rather than off either end.
	kept := append([]any{}, args[:1]...)
	kept = append(kept, args[1+len(predArgs):]...)

	rows, err := s.DB().Read().QueryContext(t.Context(), broken, kept...)
	if err != nil {
		t.Fatalf("run the neutralised statement: %v", err)
	}
	defer func() { _ = rows.Close() }()
	sawHidden := false
	for rows.Next() {
		var id int64
		var name, kind, ref, createdAt string
		var lastSync, detail sql.NullString
		if err := rows.Scan(&id, &name, &kind, &lastSync, &ref, &detail, &createdAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if id == 1 {
			sawHidden = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate the neutralised statement: %v", err)
	}
	if !sawHidden {
		t.Fatal("neutralising the instance predicate changed nothing, so the predicate was " +
			"never what kept instance 1 out of a scope that hides it — this read has no " +
			"working access scope at all")
	}
}

// ── the query plan ──────────────────────────────────────────────────────────

// proposedContainersPlan renders the SHIPPED statement through
// observedContainersSQL — the same function ProposedContainers calls, per
// docs/DEVELOPMENT.md §11 rule 1.
func proposedContainersPlan(t *testing.T, s *Store, scope Scope) []string {
	t.Helper()
	query, args := observedContainersSQL(scope)
	return planStepsOf(t, s, query, args)
}

// proposedContainersPlanFaults is the assertion the guard and its firing both
// run, so neither can drift from the other. It covers the parts that are the
// same in both scopes; the service_instance leg differs and is asserted by the
// caller.
func proposedContainersPlanFaults(plan []string) []string {
	joined := strings.Join(plan, " | ")
	var faults []string

	const wantOuter = "SEARCH r USING INDEX ix_sync_report_container_latest " +
		"(service_instance_id=? AND kind=? AND remote_kind=?)"
	if !strings.Contains(joined, wantOuter) {
		faults = append(faults, fmt.Sprintf(
			"the sync_report leg is not the three-column seek %q. Without it the outer "+
				"query walks EVERY report row of EVERY kind — items_skipped, "+
				"content_completeness, file_walk_failed — on a screen that renders from "+
				"local SQLite precisely so that it never waits.", wantOuter))
	}
	if strings.Contains(joined, "SCAN r") || strings.Contains(joined, "SCAN sync_report") {
		faults = append(faults,
			"sync_report is SCANNED. It is append-only and every import appends to it, so a "+
				"scan here gets slower every time a sync runs, forever")
	}
	if strings.Contains(joined, "USE TEMP B-TREE") {
		faults = append(faults,
			"the plan sorts. Both orderings this statement needs — the newest-row pick and "+
				"the `ORDER BY si.id, r.remote_id` — come off ix_sync_report_container_latest's "+
				"key order, and a temp b-tree means one of them stopped doing so")
	}

	// The subquery, BY POSITION: its sync_report leg must be the four-equality
	// covering seek migration 0011 exists for, and it must be the subquery's ONLY
	// step and the LAST step of the plan.
	i := indexOfPlanStep(plan, "CORRELATED SCALAR SUBQUERY 1")
	switch {
	case i < 0:
		faults = append(faults,
			"the newest-observation pick is no longer a correlated scalar subquery, so this "+
				"guard is asserting against a statement it does not describe")
	case i+1 >= len(plan):
		faults = append(faults, "the correlated subquery has no step under it at all")
	default:
		const wantSeek = "SEARCH r2 USING COVERING INDEX ix_sync_report_container_latest " +
			"(service_instance_id=? AND kind=? AND remote_kind=? AND remote_id=?)"
		if plan[i+1] != wantSeek {
			faults = append(faults, fmt.Sprintf(
				"the subquery's sync_report leg is %q, not %q — migration 0011 exists to make "+
					"this a four-column covering seek, and without it the newest-observation "+
					"pick walks one instance's whole report history and SORTS it, once per "+
					"observed container", plan[i+1], wantSeek))
		}
		if len(plan) != i+2 {
			faults = append(faults, fmt.Sprintf(
				"the correlated subquery has %d step(s) under it, want exactly 1 (its covering "+
					"seek) and nothing after it: %q",
				len(plan)-i-1, strings.Join(plan[i+1:], " | ")))
		}
	}
	return faults
}

// THE PLAN IS SEEKS AND NO SORT AT ALL, in both scopes.
//
// ⚠️ THE `CROSS JOIN` IS WHAT MAKES IT SO, AND IT IS NOT DECORATION. SQLite
// treats CROSS JOIN as a join-order barrier. Without it, and under owner scope
// where the instance predicate renders as `1=1`, the planner drives from
// sync_report instead and produces `SCAN r USING INDEX
// ix_sync_report_container_latest` plus a temp b-tree for the ORDER BY —
// MEASURED on this engine, this schema, no ANALYZE. That plan reads the whole
// index, every kind included, and sorts. Driving from service_instance instead
// costs a scan of a table with one row per configured service — which is the set
// this read is ABOUT — and turns the sync_report leg into a three-column seek
// with no sort anywhere.
func TestProposedContainersPlanIsSeeksAndNoSort(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedObservations(t, s)

	owner := proposedContainersPlan(t, s, OwnerScope(0))
	if faults := proposedContainersPlanFaults(owner); len(faults) > 0 {
		t.Errorf("the owner plan is wrong:\n  plan: %s\n  %s",
			strings.Join(owner, "\n        "), strings.Join(faults, "\n  "))
	}
	// In owner scope the instance leg is a full scan of service_instance, and
	// that is the INTENDED shape rather than a tolerated one: the read is "every
	// instance in scope", the predicate is `1=1`, and the table holds one row per
	// configured service.
	if owner[0] != "SCAN si" {
		t.Errorf("the owner plan's first step is %q, want %q — anything else means the "+
			"planner reordered the join and the CROSS JOIN barrier stopped working:\n  %s",
			owner[0], "SCAN si", strings.Join(owner, "\n  "))
	}

	scoped := proposedContainersPlan(t, s, Scope{UserID: 0, InstanceIDs: []int64{1, 2}})
	if faults := proposedContainersPlanFaults(scoped); len(faults) > 0 {
		t.Errorf("the scoped plan is wrong:\n  plan: %s\n  %s",
			strings.Join(scoped, "\n        "), strings.Join(faults, "\n  "))
	}
	if scoped[0] != "SEARCH si USING INTEGER PRIMARY KEY (rowid=?)" {
		t.Errorf("the scoped plan's instance leg is %q, want a rowid seek per visible "+
			"instance:\n  %s", scoped[0], strings.Join(scoped, "\n  "))
	}
}

// FIRING THE PLAN GUARD. Two arms, each breaking a different thing it protects,
// each running the SAME proposedContainersPlanFaults the test above runs.
func TestProposedContainersPlanGuardFires(t *testing.T) {
	// Arm 1 — ix_sync_report_container_latest is dropped outright, which is
	// EXACTLY migration 0011's Down block. Measured result: the subquery falls
	// back to ix_sync_report_instance's single-column seek, a temp b-tree appears
	// UNDER the subquery (once per observed container), and the outer ORDER BY
	// grows one of its own.
	//
	// ⚠️ dropIndexesAndConfirm, NEVER A BARE `DROP INDEX`. The plan above is
	// rendered on the READ pool, and a read connection that has already planned
	// this statement goes on planning it against the dropped index — so a bare
	// drop re-prints the HEALTHY plan and the arm passes while measuring nothing.
	// The helper proves on the read pool that the drop landed.
	t.Run("ix_sync_report_container_latest dropped", func(t *testing.T) {
		s := newTestStore(t)
		seedProposalsCorpus(t, s)
		seedObservations(t, s)
		dropIndexesAndConfirm(t, s, "ix_sync_report_container_latest")

		plan := proposedContainersPlan(t, s, OwnerScope(0))
		faults := proposedContainersPlanFaults(plan)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan with ix_sync_report_container_latest dropped:\n  %s",
				strings.Join(plan, "\n  "))
		}
		t.Logf("plan without the index:\n  %s\nfaults:\n  %s",
			strings.Join(plan, "\n  "), strings.Join(faults, "\n  "))
	})

	// Arm 2 — the CROSS JOIN barrier is removed from the SHIPPED statement text.
	// This is the regression a later "tidy up the SQL" commit produces, it
	// compiles clean, it returns the right rows, and it silently turns the read
	// into a full index scan plus a sort under owner scope.
	t.Run("the cross join barrier removed", func(t *testing.T) {
		s := newTestStore(t)
		seedProposalsCorpus(t, s)
		seedObservations(t, s)

		query, args := observedContainersSQL(OwnerScope(0))
		if !strings.Contains(query, "CROSS JOIN sync_report") {
			t.Fatalf("the shipped statement no longer contains the barrier this arm "+
				"removes:\n%s", query)
		}
		plan := planStepsOf(t, s, strings.Replace(query, "CROSS JOIN", "JOIN", 1), args)
		faults := proposedContainersPlanFaults(plan)
		if len(faults) == 0 {
			t.Fatalf("the guard passed a plan with the join-order barrier removed:\n  %s",
				strings.Join(plan, "\n  "))
		}
		t.Logf("plan without the barrier:\n  %s\nfaults:\n  %s",
			strings.Join(plan, "\n  "), strings.Join(faults, "\n  "))
	})
}
