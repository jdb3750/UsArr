package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
