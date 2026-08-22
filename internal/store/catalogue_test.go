package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/ssrf"
	"github.com/ncruces/go-sqlite3"
)

// Every test in this file runs against a REAL MIGRATED DATABASE — newTestStore
// opens internal/db and applies every migration — and every one of them
// POPULATES BEFORE IT ASSERTS. A search-invariant test over an empty corpus
// proves the shape of a query and nothing about data: 0 == 0 == 0 satisfies
// invariant 2 forever.

func fixtureInstance(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	id, err := s.CreateServiceInstance(t.Context(), ServiceInstance{
		Kind: "kavita", Role: "library", Name: name,
		BaseURL: "http://kavita.test:5000", APIKeyEnc: []byte{1, 2, 3}, KEKID: 1,
		Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	return id
}

func comicContainer(id, name string) CatalogueContainer {
	return CatalogueContainer{RemoteID: id, Name: name, Kind: "comic"}
}

// acceptContainers is §17.8's Accept step, used as a FIXTURE.
//
// ⚠️ IT EXISTS BECAUSE THE BIND PATH STOPPED CREATING LIBRARIES (ADR-0048).
// Every test whose SUBJECT is something else — membership, the sweep, search
// scope, the item pass — used to get its libraries from BindContainers as a side
// effect, and now has to say who accepted them. This is a user ticking the
// pre-checked proposals with NOTHING EDITED, which is §17.8's default, so the
// state it leaves is the state the product leaves: `managed_by = 'auto'`, one
// library per container, named for the container.
//
// A DECLINED container is skipped rather than refused: there is no proposal to
// tick for a container UsArr has no work.kind for.
//
// The name derivation is bindOneContainer's step 2 join key — trimmed, with the
// same "Library <ref>" fallback for an unnamed container — because the whole
// point of the fixture is that the bind that follows RESOLVES what this
// accepted, and it resolves by that key.
func acceptContainers(t *testing.T, s *Store, instanceID, userID int64, cs ...CatalogueContainer) {
	t.Helper()
	var accepts []LibraryAcceptance
	for _, c := range cs {
		if c.Kind == "" {
			continue
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = "Library " + c.RemoteID
		}
		accepts = append(accepts, LibraryAcceptance{
			Name: name, Kind: c.Kind, ManagedBy: "auto",
			Sources: []AcceptedSource{{
				ServiceInstanceID: instanceID, ContainerKind: "remote_library",
				ContainerRef: c.RemoteID, ContainerIdentity: c.Name,
			}},
		})
	}
	if len(accepts) == 0 {
		return
	}
	if _, err := s.AcceptLibraries(t.Context(), OwnerScope(userID), accepts); err != nil {
		t.Fatalf("accept libraries for the fixture: %v", err)
	}
}

// acceptedBind is the ordinary fixture since ADR-0048: accept a proposal per
// container, then bind. It is BindContainers as every test that needs libraries
// to exist has to spell it now — the bind resolves what Accept created, and
// creates nothing itself.
func acceptedBind(t *testing.T, s *Store, instanceID int64, cs ...CatalogueContainer) map[string]CatalogueBinding {
	t.Helper()
	acceptContainers(t, s, instanceID, SystemUserID, cs...)
	binds, skipped, err := s.BindContainers(t.Context(), instanceID, SystemUserID, cs)
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("BindContainers skipped %+v", skipped)
	}
	for _, c := range cs {
		if c.Kind == "" {
			continue
		}
		if b := binds[c.RemoteID]; b.NoLibrary || b.LibraryID == 0 {
			t.Fatalf("container %q did not resolve to the library the fixture accepted: %+v",
				c.RemoteID, b)
		}
	}
	return binds
}

func libraryNameSlug(t *testing.T, s *Store, id int64) (string, string) {
	t.Helper()
	var name, slug string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT name, slug FROM library WHERE id = ?`, id).Scan(&name, &slug); err != nil {
		t.Fatalf("read library %d: %v", id, err)
	}
	return name, slug
}

func item(remoteID, container, kind, title string, ids ...ExternalIdentifier) CatalogueItem {
	return CatalogueItem{
		RemoteID: remoteID, RemoteKind: "series", ContainerID: container, Kind: kind,
		Title: title, SortTitle: title, NormalizedTitle: strings.ToLower(title), NormVersion: 1,
		AddedAt: testNow, RemoteUpdatedAt: testNow, HasFile: true,
		ExternalIDs: ids,
	}
}

func count(t *testing.T, s *Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.Read().QueryRowContext(t.Context(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// ── the invariant queries, written ONCE and used by every assertion ─────────
//
// Migration 0005 says the search-document builder "must" uphold two invariants
// SQLite cannot declare and that "the same test pins the query". These two
// functions are those queries. They are shared so that a test which breaks the
// invariant deliberately and a test which asserts it after a real import are
// checking the SAME predicate — a copy-pasted variant is how an invariant
// assertion silently stops covering the thing it names.

// docsWithNoScope is invariant 5: every search_doc row has at least one
// search_doc_library row.
func docsWithNoScope(t *testing.T, s *Store) int {
	t.Helper()
	return count(t, s, `
		SELECT COUNT(*) FROM search_doc d
		 WHERE NOT EXISTS (SELECT 1 FROM search_doc_library l WHERE l.doc_rowid = d.rowid)`)
}

// corpusCounts is invariant 2: count(search_fts) == count(search_trgm) ==
// count(search_doc).
func corpusCounts(t *testing.T, s *Store) (docs, fts, trgm int) {
	t.Helper()
	return count(t, s, `SELECT COUNT(*) FROM search_doc`),
		count(t, s, `SELECT COUNT(*) FROM search_fts`),
		count(t, s, `SELECT COUNT(*) FROM search_trgm`)
}

// misalignedRowids is the invariant-2 predicate stated as what it actually
// means rather than as a count.
//
// ⚠️ COUNTING IS A PROXY AND IT WAS MEASURED FAILING. This file used to check
// invariant 2 with three COUNT(*)s, and replacing the explicit
// `INSERT INTO search_fts (rowid, …)` with an implicit one left every one of
// those counts equal — so the guard for the rule migration 0005 calls out by
// name ("a single implicit rowid fuses unrelated documents, because RRF fuses on
// rowid") never fired. The counts agree because symmetric inserts and deletes
// make two independent rowid sequences advance in lockstep; they say nothing
// about the sequences being the SAME. The real condition is that the three
// tables hold the same rowid SET, and this asks it directly.
func misalignedRowids(t *testing.T, s *Store) int {
	t.Helper()
	return count(t, s, `
		SELECT (SELECT COUNT(*) FROM search_doc d
		         WHERE NOT EXISTS (SELECT 1 FROM search_fts f WHERE f.rowid = d.rowid))
		     + (SELECT COUNT(*) FROM search_doc d
		         WHERE NOT EXISTS (SELECT 1 FROM search_trgm g WHERE g.rowid = d.rowid))
		     + (SELECT COUNT(*) FROM search_fts f
		         WHERE NOT EXISTS (SELECT 1 FROM search_doc d WHERE d.rowid = f.rowid))
		     + (SELECT COUNT(*) FROM search_trgm g
		         WHERE NOT EXISTS (SELECT 1 FROM search_doc d WHERE d.rowid = g.rowid))`)
}

func assertCorpusInvariants(t *testing.T, s *Store, wantDocs int) {
	t.Helper()
	docs, fts, trgm := corpusCounts(t, s)
	if docs != wantDocs {
		t.Errorf("search_doc has %d rows, want %d — populate before asserting", docs, wantDocs)
	}
	if docs == 0 {
		t.Fatal("the corpus is empty, so these invariants are vacuous: 0 == 0 == 0")
	}
	if docs != fts || docs != trgm {
		t.Errorf("§7 invariant 2 broken: search_doc=%d search_fts=%d search_trgm=%d "+
			"(they are three tables SQLite cannot hold equal; the builder must)", docs, fts, trgm)
	}
	if n := misalignedRowids(t, s); n != 0 {
		t.Errorf("§7 invariant 2 broken where COUNTING CANNOT SEE IT: %d rowids appear in one "+
			"of the three tables and not the others. search_doc.rowid is THE allocator and both "+
			"FTS inserts must name it explicitly", n)
	}
	if n := docsWithNoScope(t, s); n != 0 {
		t.Errorf("§7 invariant 5 broken: %d search_doc rows have no search_doc_library row. "+
			"Such a doc matches no scope and vanishes from search for every user including its owner", n)
	}
}

// TestBindContainersCreatesNoLibrary is the headline of ADR-0048's removal, and
// it is what TestBindContainersCreatesOneLibraryPerContainer became.
//
// That test asserted the opposite — two containers, two libraries, both
// reporting Created — because until now a first connect created a library per
// container with no screen involved (§17.8 measures that path). ADR-0048 makes a
// `library` row conditional on Accept, so the fact under test inverts: the bind
// path resolves what already exists and creates nothing.
func TestBindContainersCreatesNoLibrary(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita-1")

	binds, skipped, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Manga"),
		{RemoteID: "2", Name: "Ebooks", Kind: "book"},
		{RemoteID: "4", Name: "Wallpapers", DeclineReason: "no UsArr kind"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped %+v; nothing is created, so nothing can violate a constraint", skipped)
	}

	// ⚠️ THE TWO NON-DECLINED CONTAINERS STILL GET A BINDING, and that is not a
	// detail. ApplyCatalogueBatch skips an item whose container has NO ENTRY in
	// this map, so a container that vanished from it would have its items applied
	// nowhere — and ADR-0048's amendment runs the first import BEFORE Accept
	// precisely so the proposals carry real item counts. The binding is what says
	// "apply this, file it nowhere".
	if len(binds) != 2 {
		t.Fatalf("bound %d containers, want 2 (the declined one must get no binding)", len(binds))
	}
	if _, ok := binds["4"]; ok {
		t.Error("a DECLINED container got a binding; its items would then be applied, and " +
			"UsArr has no work.kind to apply them as")
	}
	for _, ref := range []string{"1", "2"} {
		if !binds[ref].NoLibrary {
			t.Errorf("container %s reports a library: %+v", ref, binds[ref])
		}
	}
	if binds["1"].Kind != "comic" || binds["2"].Kind != "book" {
		t.Errorf("kinds came out wrong: %+v", binds)
	}

	// THE EFFECT, READ OFF THE TABLES rather than off the returned struct: no
	// library beyond the reserved row, and no source binding one.
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE id <> ?`, UnfiledLibraryID); n != 0 {
		t.Errorf("%d libraries exist after a bind, want 0 — AcceptLibraries is the only "+
			"writer of a library row", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_source`); n != 0 {
		t.Errorf("%d library_source rows exist, want 0: a container nobody accepted is bound "+
			"to nothing, which is what makes ProposedContainers' exclusion of bound "+
			"containers mean anything", n)
	}
}

func TestBindContainersIsIdempotentAndJoinsBySameNameAndKind(t *testing.T) {
	s := newTestStore(t)
	a := fixtureInstance(t, s, "kavita-a")
	b := fixtureInstance(t, s, "kavita-b")

	// THE LIBRARY EXISTS BECAUSE SOMEBODY ACCEPTED IT. This test used to get it
	// from the bind path itself; since ADR-0048 that path creates nothing, so the
	// Accept step is the fixture. What is under test is unchanged and is step 1
	// and step 2 of bindOneContainer, both of which ADR-0048 leaves alone.
	acceptContainers(t, s, a, SystemUserID, comicContainer("1", "Manga"))

	first, _, err := s.BindContainers(t.Context(), a, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers a: %v", err)
	}
	if first["1"].NoLibrary || first["1"].LibraryID == 0 {
		t.Fatalf("the accepted library was not resolved: %+v", first["1"])
	}
	// Re-running the same instance must not create a second library.
	again, _, err := s.BindContainers(t.Context(), a, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers a again: %v", err)
	}
	if again["1"].LibraryID != first["1"].LibraryID {
		t.Errorf("a re-import bound the container elsewhere: %+v then %+v", first["1"], again["1"])
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE id <> ?`, UnfiledLibraryID); n != 1 {
		t.Errorf("%d libraries exist after a re-import, want the 1 that was accepted", n)
	}

	// §17.8: a SECOND INSTANCE of the same kind JOINS the existing library
	// rather than creating a new one. The merge key is case-insensitive and
	// whitespace-trimmed, so "  manga  " must join "Manga".
	joined, _, err := s.BindContainers(t.Context(), b, SystemUserID, []CatalogueContainer{comicContainer("7", "  manga  ")})
	if err != nil {
		t.Fatalf("BindContainers b: %v", err)
	}
	if joined["7"].LibraryID != first["1"].LibraryID {
		t.Errorf("second instance bound container 7 to library %d instead of joining %d — §17.8's "+
			"default is join, and getting it wrong destroys the two-source badge",
			joined["7"].LibraryID, first["1"].LibraryID)
	}
	if joined["7"].NoLibrary {
		t.Errorf("a join reported %+v", joined["7"])
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_source WHERE library_id = ?`, first["1"].LibraryID); n != 2 {
		t.Errorf("the joined library has %d sources, want 2", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE id <> ?`, UnfiledLibraryID); n != 1 {
		t.Errorf("%d libraries exist, want the 1 that was accepted", n)
	}
}

func TestBindContainersWillNotJoinAcrossKinds(t *testing.T) {
	// library.kind is exactly one value, so a name collision between a comic
	// container and a book container must NOT join: the books would render in a
	// comics library and no format filter could rescue them.
	//
	// ⚠️ WHAT THE BOOK CONTAINER GETS INSTEAD HAS CHANGED, and that is the whole
	// of ADR-0048 at this site. It used to get a library of its own under a
	// disambiguated name; it now gets NO library, and the user is offered a
	// proposal for it. The refusal to join is what is under test and it is
	// untouched.
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	acceptContainers(t, s, inst, SystemUserID, comicContainer("1", "Library"))

	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Library"),
		{RemoteID: "2", Name: "Library", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if binds["1"].NoLibrary {
		t.Fatalf("the accepted comic library was not resolved: %+v", binds["1"])
	}
	if !binds["2"].NoLibrary {
		t.Fatalf("a book container was joined into a comic library: %+v", binds["2"])
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE kind = 'book'`); n != 0 {
		t.Errorf("%d book libraries were created; the create path is gone and the book "+
			"container is a proposal", n)
	}
}

// TestTheBindPathNoLongerDerivesALibraryName is what THREE tests became, and
// they are folded rather than dropped because they asserted three cases of ONE
// derivation that no longer exists:
//
//   - TestBindContainersDisambiguatesASlugCollisionNotJustANameCollision — two
//     names that reduce to one slug, `Sci-Fi` and `Sci Fi`, where the create
//     died on ux_library_slug and the ordinal loop rescued it.
//   - TestBindContainersDoesNotCollideWithTheReservedUnfiledLibrary — a
//     container genuinely named `Unfiled`, which collided with library 0's own
//     row in both unique indexes.
//   - TestBindContainersRebindsToTheSameDisambiguatedLibraryEveryTime — that the
//     ordinal did not walk `(2)`, `(3)`, `(4)` across four imports.
//
// Every one of them was a fact about bindOneContainer's step 3, which created a
// library under a name and slug it derived. ADR-0048 removes the create, so the
// derivation went with it: there is no name to collide, no slug to disambiguate
// and no ordinal to re-run. What replaces the three assertions is one, and it is
// stronger — the collisions they were written against cannot arise, because
// nothing is written.
//
// ⚠️ THE UNIQUENESS RULES THEY DEFENDED ARE NOT UNDEFENDED. They moved with the
// create: AcceptLibraries refuses a name held at another kind, refuses the
// reserved name, and refuses a free name whose SLUG is taken — deliberately,
// because a user typed the name and this path can ask rather than invent.
// proposals_test.go holds those.
func TestTheBindPathNoLongerDerivesALibraryName(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	// Each of these is one of the three collisions, and the reserved row is real
	// rather than seeded here: migration 00005 seeds library 0 as `Unfiled`,
	// kind 'movie', owned by user 0 — which is the acting user of a v0.1 import.
	cs := []CatalogueContainer{
		comicContainer("1", "Sci-Fi"),
		comicContainer("2", "Sci Fi"),
		{RemoteID: "3", Name: "Unfiled", Kind: "movie"},
	}
	for run := 1; run <= 3; run++ {
		binds, skipped, err := s.BindContainers(t.Context(), inst, SystemUserID, cs)
		if err != nil {
			t.Fatalf("run %d: BindContainers: %v", run, err)
		}
		if len(skipped) != 0 {
			t.Fatalf("run %d skipped %+v; nothing is created, so no unique index can be "+
				"violated", run, skipped)
		}
		for _, ref := range []string{"1", "2", "3"} {
			if !binds[ref].NoLibrary {
				t.Errorf("run %d: container %s got library %d", run, ref, binds[ref].LibraryID)
			}
		}
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE id <> ?`, UnfiledLibraryID); n != 0 {
		t.Errorf("%d libraries exist after three imports of three colliding names, want 0", n)
	}
	// The reserved row is untouched, which is the one assertion of the three that
	// is about a row rather than about a derivation.
	rn, rs := libraryNameSlug(t, s, UnfiledLibraryID)
	if rn != "Unfiled" || rs != "unfiled" {
		t.Errorf("library 0 is now (%q, %q); the reserved row must not be renamed or re-slugged", rn, rs)
	}
	if _, err := s.db.Writer().ExecContext(t.Context(), `DELETE FROM library WHERE id = 0`); err == nil {
		t.Error("library 0 became deletable; trg_library_unfiled_no_delete must still fire")
	}
}

// TestAnUnknownKindNoLongerSkipsAContainer is what
// TestBindContainersSkipsAnUnbindableContainerAndRecordsWhy became, and the
// change is a MEASUREMENT rather than a preference.
//
// That test bound a container whose adapter reported kind 'score', which
// library.kind's CHECK does not permit, and asserted the whole degradation
// story: the container is rolled back to its savepoint, recorded as
// `container_bind_failed`, returned in []SkippedContainer, and every other
// container still binds. The mechanism was the CREATE — it was the only
// statement in the bind path that could violate a constraint.
//
// ⚠️ SO `container_bind_failed` IS NOW UNREACHABLE THROUGH THE CONTAINER LIST,
// and that is stated here rather than left for someone to find. With no create,
// an unknown kind is not a constraint violation at all: it is a container nobody
// can accept, which lands in the same place as every other unaccepted container.
// The savepoint, isSkippableBindError and the `container_bind_failed` row are
// KEPT — a constraint the schema owns and this path re-derives is exactly the
// pair that drifts, and the machinery costs nothing standing — but nothing in
// this test can fire them any more.
//
// # WHAT WOULD FALSIFY THAT ABSENCE, named so a later reader can check it
//
// The claim is exactly: no statement reachable from BindContainers' loop can
// violate a constraint. It stops being true the moment one can, and these are
// the ways in:
//
//   - bindOneContainer regains a write to `library`. Any auto-create, rename or
//     re-slug on this path puts ux_library_name, ux_library_slug and
//     library.kind's CHECK back in reach — which is precisely what used to make
//     this test's container skippable.
//   - insertLibrarySource loses its `ON CONFLICT … DO UPDATE`, or
//     library_source gains a CHECK on container_kind. Step 2's join writes that
//     row on every new binding.
//   - bindProvisional's retype becomes reachable from the container list. Its
//     `UPDATE library SET kind = ?` CAN violate library.kind's CHECK today, but
//     only under bindSiblingKind — which comes from resolveBinding on the ITEM
//     pass, never from this loop, whose branch adopts rather than retypes.
//   - recordSyncReport or noteKindChange gains a constraint on sync_report.
//
// If any of those lands, `len(skipped) != 0` below is the assertion that starts
// failing, and the machinery to catch it is already standing.
//
// CLAUDE.md principle 3 is what the old test was really about and it still
// holds: one container UsArr cannot use does not take down the import.
func TestAnUnknownKindNoLongerSkipsAContainer(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	binds, skipped, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Manga"),
		{RemoteID: "2", Name: "Sheet Music", Kind: "score"},
		{RemoteID: "3", Name: "Ebooks", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %+v; with no create there is no constraint left to violate", skipped)
	}
	if len(binds) != 3 {
		t.Fatalf("bound %+v, want a binding for all three: a container UsArr cannot type is "+
			"still a container whose items are read", binds)
	}
	for _, ref := range []string{"1", "2", "3"} {
		if !binds[ref].NoLibrary {
			t.Errorf("container %s got library %d", ref, binds[ref].LibraryID)
		}
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE kind = 'container_bind_failed'`); n != 0 {
		t.Errorf("%d container_bind_failed rows were written; no bind failed", n)
	}
	// ⚠️ AND THE UNKNOWN KIND IS STILL OBSERVED. The proposal it produces is
	// unacceptable — library.kind's CHECK refuses 'score' — but §17.8 renders it
	// with a reason rather than hiding it, and the observation row is where that
	// reason comes from.
	if got := observations(t, s, inst); len(got["2"]) != 1 {
		t.Errorf("the unbindable container has %d observations, want 1", len(got["2"]))
	}
}

func TestApplyCatalogueBatchWritesTheWholeShape(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds := acceptedBind(t, s, inst,
		comicContainer("1", "Manga"),
		CatalogueContainer{RemoteID: "2", Name: "Ebooks", Kind: "book"},
	)

	comic := item("41", "1", "comic", "Frieren")
	comic.ReadingDirection = sql.NullString{String: "rtl", Valid: true}
	comic.RemotePath = "/mnt/user/media/manga/Frieren"
	comic.RemoteSubtype = "1"
	comic.AltTitles = []AltTitle{{Title: "葬送のフリーレン", Normalized: "葬送のフリーレン", Kind: "original"}}
	comic.OriginalTitle = "葬送のフリーレン"

	book := item("77", "2", "book", "The Hobbit", ExternalIdentifier{Source: "hardcover_book", Value: "445", Confidence: 1.0})
	book.PageCount = sql.NullInt64{Int64: 310, Valid: true}

	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{comic, book}, testNow)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	if res.WorksCreated != 2 || res.SearchDocs != 2 || res.Members != 2 {
		t.Fatalf("result = %+v, want 2 created / 2 docs / 2 members", res)
	}
	if res.Unidentified != 1 {
		t.Errorf("Unidentified = %d, want 1 (the comic has no external id and that is the ordinary case)", res.Unidentified)
	}
	if res.ExternalIDsWritten != 1 {
		t.Errorf("ExternalIDsWritten = %d, want 1", res.ExternalIDsWritten)
	}

	// Subtype rows exist for both kinds, and the READING DIRECTION landed.
	var dir sql.NullString
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT wc.reading_direction FROM work_comic wc
		   JOIN work w ON w.id = wc.work_id WHERE w.title = 'Frieren'`).Scan(&dir); err != nil {
		t.Fatalf("read work_comic: %v", err)
	}
	if dir.String != "rtl" {
		t.Errorf("reading_direction = %q, want rtl", dir.String)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_book WHERE page_count = 310`); n != 1 {
		t.Errorf("work_book rows with page_count 310 = %d, want 1", n)
	}
	// No book row for a comic and no comic row for a book.
	if n := count(t, s, `
		SELECT COUNT(*) FROM work w
		  JOIN work_book b ON b.work_id = w.id WHERE w.kind = 'comic'`); n != 0 {
		t.Errorf("%d comic works have a work_book row", n)
	}

	// The link carries the container, the verbatim path and BOTH hashes.
	var libID, path, subtype, rhash, ihash string
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT remote_library_id, remote_path, remote_subtype, remote_hash, remote_identity_hash
		  FROM service_item_link WHERE service_instance_id = ? AND remote_id = '41'`,
		inst).Scan(&libID, &path, &subtype, &rhash, &ihash); err != nil {
		t.Fatalf("read link: %v", err)
	}
	if libID != "1" || path != "/mnt/user/media/manga/Frieren" || subtype != "1" {
		t.Errorf("link = (%q,%q,%q)", libID, path, subtype)
	}
	if len(rhash) != 64 || len(ihash) != 64 {
		t.Errorf("hashes are not sha256 hex: %q / %q", rhash, ihash)
	}

	// Membership landed in the RIGHT library, edition-grained with the whole-work
	// sentinel, and carries the denormalised sort title.
	var gotLib, gotEd int64
	var gotSort string
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT m.library_id, m.edition_id, m.sort_title FROM library_member m
		  JOIN work w ON w.id = m.work_id WHERE w.title = 'The Hobbit'`).
		Scan(&gotLib, &gotEd, &gotSort); err != nil {
		t.Fatalf("read library_member: %v", err)
	}
	if gotLib != binds["2"].LibraryID || gotEd != 0 || gotSort != "The Hobbit" {
		t.Errorf("membership = (%d, %d, %q), want (%d, 0, The Hobbit)", gotLib, gotEd, gotSort, binds["2"].LibraryID)
	}

	assertCorpusInvariants(t, s, 2)

	// THE EXPLICIT-ROWID RULE, PROVED BY A MATCH RATHER THAN BY A SELECT. Both
	// FTS tables are content='' contentless, so their columns read back as NULL
	// and cannot be inspected directly — a MATCH is the only way to see what
	// went in. Searching for a term that occurs in exactly ONE document must
	// return the rowid of THAT document's search_doc row; if the builder had let
	// SQLite allocate the FTS rowid implicitly, this join would land on some
	// other work, which is the "fuses unrelated documents" failure migration
	// 0005 names.
	for _, tc := range []struct {
		table, query, wantTitle string
	}{
		{"search_fts", "Frieren", "Frieren"},
		{"search_fts", "Hobbit", "The Hobbit"},
		// The trigram table indexes title and alt_titles only, and its
		// tokenizer makes any 3-character substring matchable.
		{"search_trgm", "obbi", "The Hobbit"},
	} {
		var gotTitle string
		q := fmt.Sprintf(`
			SELECT w.title FROM %s f
			  JOIN search_doc d ON d.rowid = f.rowid
			  JOIN work w ON w.id = d.work_id
			 WHERE f.%s MATCH ?`, tc.table, tc.table)
		if err := s.db.Read().QueryRowContext(t.Context(), q, tc.query).Scan(&gotTitle); err != nil {
			t.Fatalf("%s MATCH %q: %v", tc.table, tc.query, err)
		}
		if gotTitle != tc.wantTitle {
			t.Errorf("%s MATCH %q resolved through its rowid to work %q, want %q — "+
				"the FTS rowid is not the search_doc rowid",
				tc.table, tc.query, gotTitle, tc.wantTitle)
		}
	}

	// The alternate title reached BOTH documents, under the same rowid.
	//
	// The search is run against search_trgm rather than search_fts, and the
	// reason is a real property of the tokenizers rather than test convenience:
	// unicode61 treats a run of CJK ideographs as ONE token, so
	// "葬送のフリーレン" is a single term and a substring of it matches nothing.
	// That is exactly what the trigram table is for (ARCHITECTURE.md §8.2's two
	// retrievers), and asserting it here records the behaviour instead of
	// leaving the next reader to rediscover it.
	for _, q := range []string{"葬送のフリーレン", "フリーレン"} {
		var trgmHits int
		if err := s.db.Read().QueryRowContext(t.Context(),
			`SELECT COUNT(*) FROM search_trgm WHERE search_trgm MATCH ?`, q).Scan(&trgmHits); err != nil {
			t.Fatalf("search_trgm MATCH %q: %v", q, err)
		}
		if trgmHits != 1 {
			t.Errorf("search_trgm MATCH %q hit %d documents, want 1: the alt title did not reach the doc", q, trgmHits)
		}
	}
	var ftsWholeToken int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM search_fts WHERE search_fts MATCH ?`, `"葬送のフリーレン"`).Scan(&ftsWholeToken); err != nil {
		t.Fatalf("search_fts MATCH on the whole alt title: %v", err)
	}
	if ftsWholeToken != 1 {
		t.Errorf("search_fts hit %d documents on the whole alt title, want 1", ftsWholeToken)
	}
}

func TestApplyCatalogueBatchIsIdempotent(t *testing.T) {
	// The same import run twice must leave exactly one of everything. Without
	// the delete-then-insert in writeSearchDoc this is where invariant 2
	// breaks: the second run adds a doc and leaves the first one's FTS rows.
	//
	// ⚠️ TWO ITEMS, NOT ONE, AND THAT IS THE WHOLE POINT. With a single work this
	// test PASSED with the FTS delete removed: the one search_doc row is always
	// the max rowid, so deleting and reinserting it hands back the same number
	// and the orphaned FTS row is silently overwritten. A second work makes the
	// rebuilt one a NON-max rowid, the reinsert allocates a fresh number, and the
	// orphan survives — which is the real failure. Measured: with the delete
	// removed this now reports search_fts = 3 against search_doc = 2.
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds := acceptedBind(t, s, inst, comicContainer("1", "Manga"))
	it := item("41", "1", "comic", "Frieren")
	other := item("42", "1", "comic", "Vagabond")

	for run := 1; run <= 2; run++ {
		if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
			[]CatalogueItem{it, other}, testNow); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work`); n != 2 {
		t.Errorf("work rows = %d, want 2", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member`); n != 2 {
		t.Errorf("library_member rows = %d, want 2", n)
	}
	assertCorpusInvariants(t, s, 2)

	// A RETITLE must move the member row rather than leave a second one under
	// the old sort key — sort_title leads library_member's primary key.
	it.Title, it.SortTitle = "Frieren: Beyond Journey's End", "Frieren Beyond"
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{it}, testNow); err != nil {
		t.Fatalf("retitle: %v", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member`); n != 2 {
		t.Errorf("after a retitle there are %d member rows, want 2", n)
	}
	var sortTitle string
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT m.sort_title FROM library_member m
		  JOIN work w ON w.id = m.work_id WHERE w.title LIKE 'Frieren%'`).Scan(&sortTitle); err != nil {
		t.Fatalf("read member: %v", err)
	}
	if sortTitle != "Frieren Beyond" {
		t.Errorf("member sort_title = %q, want the new one", sortTitle)
	}
	assertCorpusInvariants(t, s, 2)
}

// TestTheFTSRowidIsTheSearchDocRowidWhenTheTablesHaveDRIFTED is the guard for
// the rule migration 0005 states and nothing was checking.
//
// ⚠️ THE OBVIOUS TEST DOES NOT WORK, AND THAT IS WHY THIS ONE IS SHAPED LIKE
// THIS. Replacing the explicit `INSERT INTO search_fts (rowid, …)` with an
// implicit one leaves EVERY count equal and EVERY rowid identical, so a
// straightforward import test passes with the rule broken — measured against
// three separate tests before this was written. Two independent rowid sequences
// only diverge once the tables have drifted apart, so this test PLANTS exactly
// the drift migration 0005 says can happen (an FTS row missing under its doc)
// and then requires the next rebuild to land back on the doc's own rowid.
//
// The failure being guarded is not "a count is wrong". It is that
// `search_fts MATCH` resolves through the rowid to a DIFFERENT work than the one
// that contains the term — §8.2's two retrievers fusing on rowid, fused onto the
// wrong row.
func TestTheFTSRowidIsTheSearchDocRowidWhenTheTablesHaveDrifted(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	keep := item("41", "1", "comic", "Vagabond")
	rebuilt := item("42", "1", "comic", "Frieren")
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{keep, rebuilt}, testNow); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// The drift: `keep`'s FTS rows vanish while its search_doc row stays. This is
	// invariant 2 already broken — the state the migration says nothing in SQLite
	// can prevent — and the question is what the BUILDER does next.
	var keepDoc int64
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT d.rowid FROM search_doc d JOIN work w ON w.id = d.work_id
		 WHERE w.title = 'Vagabond'`).Scan(&keepDoc); err != nil {
		t.Fatalf("read the doc rowid: %v", err)
	}
	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM search_fts WHERE rowid = ?`,
			`DELETE FROM search_trgm WHERE rowid = ?`,
		} {
			if _, err := tx.ExecContext(ctx, q, keepDoc); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("planting the drift: %v", err)
	}
	if misalignedRowids(t, s) == 0 {
		t.Fatal("the planted drift was not detected, so this test would prove nothing")
	}

	// Now rebuild the OTHER work. Its doc is deleted and reinserted, and the new
	// doc rowid is one the FTS tables' own sequence would never have chosen.
	rebuilt.Title = "Frieren: Beyond Journey's End"
	rebuilt.NormalizedTitle = "frieren beyond journey s end"
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{rebuilt}, testNow); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// THE ASSERTION: the term resolves through the rowid to ITS OWN work.
	for _, tc := range []struct{ table, query string }{
		{"search_fts", "Beyond"},
		{"search_trgm", "Journey"},
	} {
		var got string
		q := fmt.Sprintf(`
			SELECT w.title FROM %s f
			  JOIN search_doc d ON d.rowid = f.rowid
			  JOIN work w ON w.id = d.work_id
			 WHERE f.%s MATCH ?`, tc.table, tc.table)
		if err := s.db.Read().QueryRowContext(t.Context(), q, tc.query).Scan(&got); err != nil {
			t.Fatalf("%s MATCH %q found no document at all — the FTS row was written under a "+
				"rowid no search_doc row carries: %v", tc.table, tc.query, err)
		}
		if got != rebuilt.Title {
			t.Errorf("%s MATCH %q resolved through its rowid to %q, want %q — the FTS rowid is "+
				"not the search_doc rowid, and RRF fuses on rowid",
				tc.table, tc.query, got, rebuilt.Title)
		}
	}

	// And the rebuilt doc is aligned again: the ONLY misalignment left is the
	// planted one, which nothing in this commit repairs.
	if n := misalignedRowids(t, s); n != 2 {
		t.Errorf("misaligned rowids = %d, want exactly the 2 planted holes (fts and trgm) — "+
			"the rebuild introduced new drift of its own", n)
	}
}

func TestTierOneReusesTheWorkThatAlreadyHoldsTheIdentifier(t *testing.T) {
	// Two DIFFERENT remote items on two DIFFERENT instances carrying the same
	// strong id are the same work. That is §6.4 tier 1, and it is also how the
	// overwhelming majority of would-be ux_extid_work_strong collisions are
	// absorbed BEFORE they happen.
	s := newTestStore(t)
	a := fixtureInstance(t, s, "kavita-a")
	b := fixtureInstance(t, s, "kavita-b")

	id := ExternalIdentifier{Source: "anilist", Value: "30013", Confidence: 1.0}
	bindsA := acceptedBind(t, s, a, comicContainer("1", "A"))
	bindsB := acceptedBind(t, s, b, comicContainer("1", "B"))
	if _, err := s.ApplyCatalogueBatch(t.Context(), a, bindsA,
		[]CatalogueItem{item("1", "1", "comic", "Berserk", id)}, testNow); err != nil {
		t.Fatalf("apply a: %v", err)
	}
	res, err := s.ApplyCatalogueBatch(t.Context(), b, bindsB,
		[]CatalogueItem{item("9", "1", "comic", "Berserk", id)}, testNow)
	if err != nil {
		t.Fatalf("apply b: %v", err)
	}
	if res.WorksReused != 1 || res.WorksCreated != 0 {
		t.Fatalf("result = %+v, want the existing work reused", res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work`); n != 1 {
		t.Errorf("work rows = %d, want 1: tier 1 did not join them", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM service_item_link`); n != 2 {
		t.Errorf("link rows = %d, want 2: one work, two services", n)
	}
	// The same work is now in two libraries, so it has two scope rows and still
	// exactly one document.
	assertCorpusInvariants(t, s, 1)
	if n := count(t, s, `SELECT COUNT(*) FROM search_doc_library`); n != 2 {
		t.Errorf("search_doc_library rows = %d, want 2", n)
	}
}

func TestTierOneNeverMergesAcrossKind(t *testing.T) {
	// §6.4 rule 1: "never auto-merge across kind" — the film and the novella are
	// linked, never merged. Same id, different kind, two works.
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Manga"), {RemoteID: "2", Name: "Ebooks", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	id := ExternalIdentifier{Source: "mal", Value: "2", Confidence: 1.0}
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{
		item("1", "1", "comic", "Berserk", id),
		item("2", "2", "book", "Berserk", id),
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	if res.WorksCreated != 2 || res.WorksReused != 0 {
		t.Fatalf("result = %+v, want two separate works", res)
	}
	// The second one's identity write is the one that collides, because
	// ux_extid_work_strong is UNIQUE(source, value) with no kind in it.
	if len(res.IdentityConflicts) != 1 {
		t.Fatalf("IdentityConflicts = %+v, want exactly one", res.IdentityConflicts)
	}
}

func TestTwoItemsClaimingOneIDInOneBatchAreResolvedByReuse(t *testing.T) {
	// The instruction migration 0005 gives is that a ux_extid_work_strong
	// violation is the MERGE SIGNAL. v0.1 has no work_merge table, so the
	// resolution is REUSE, and the ordinary in-batch case never reaches the
	// savepoint at all: tier 1 runs BEFORE the external_id write, so the second
	// item resolves onto the first item's work and both links point at it.
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	id := ExternalIdentifier{Source: "anilist", Value: "30013", Confidence: 1.0}

	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{
		item("101", "1", "comic", "Berserk", id),
		item("102", "1", "comic", "Berserk (Deluxe)", id),
		item("103", "1", "comic", "Vagabond"),
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	if len(res.IdentityConflicts) != 0 {
		t.Errorf("IdentityConflicts = %+v, want none: tier 1 absorbs this before the write",
			res.IdentityConflicts)
	}
	if res.WorksCreated != 2 || res.WorksReused != 1 {
		t.Errorf("result = %+v, want 2 created and 1 reused", res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work`); n != 2 {
		t.Errorf("work rows = %d, want 2", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM service_item_link`); n != 3 {
		t.Errorf("link rows = %d, want 3: three remote items, two works", n)
	}
	assertCorpusInvariants(t, s, 2)
}

func TestExternalIDConflictIsRecordedAndTheBatchSurvives(t *testing.T) {
	// THE SAVEPOINT'S WHOLE JOB, on the case tier 1 CANNOT absorb.
	//
	// An EXISTING LINK pins which work a remote item is, and it outranks
	// identity resolution — the upstream's own id is authoritative for that
	// question. So when Kavita+ later matches an already-imported series to an
	// id another work already holds, the work is already decided and the
	// external_id write is the thing that collides. Without the per-row
	// savepoint this aborts the whole 2,000-row transaction.
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	id := ExternalIdentifier{Source: "anilist", Value: "30013", Confidence: 1.0}

	// Import 1: the two series exist, only one of them identified.
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{
		item("101", "1", "comic", "Berserk", id),
		item("102", "1", "comic", "Berserk (Deluxe)"),
	}, testNow); err != nil {
		t.Fatalf("first import: %v", err)
	}
	var pinned int64
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT work_id FROM service_item_link WHERE remote_id = '102'`).Scan(&pinned); err != nil {
		t.Fatalf("read pinned work: %v", err)
	}

	// Import 2: Kavita+ has now matched 102 to the SAME AniList id.
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{
		item("102", "1", "comic", "Berserk (Deluxe)", id),
		item("103", "1", "comic", "Vagabond"),
	}, testNow)
	if err != nil {
		t.Fatalf("the batch aborted on an identity conflict, which is exactly what the "+
			"per-row SAVEPOINT exists to prevent: %v", err)
	}
	if len(res.IdentityConflicts) != 1 {
		t.Fatalf("IdentityConflicts = %+v, want exactly one", res.IdentityConflicts)
	}
	c := res.IdentityConflicts[0]
	if c.Source != "anilist" || c.Value != "30013" || c.RemoteID != "102" ||
		c.AttemptedWorkID != pinned || c.ExistingWorkID == pinned || c.ExistingWorkID == 0 {
		t.Errorf("conflict = %+v, want anilist/30013 attributed to remote item 102 on work %d",
			c, pinned)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM external_id`); n != 1 {
		t.Errorf("external_id rows = %d, want 1: the conflicting row must NOT have landed", n)
	}
	// The rest of the batch survived: Vagabond is a new work.
	if n := count(t, s, `SELECT COUNT(*) FROM work`); n != 3 {
		t.Errorf("work rows = %d, want 3: the batch after the conflict must still apply", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE title = 'Vagabond'`); n != 1 {
		t.Error("the row AFTER the conflicting one was lost: the savepoint is not per row")
	}
	// The loser keeps its own work and is simply not identified — kept, marked,
	// searchable (§6.4).
	if res.Unidentified != 2 {
		t.Errorf("Unidentified = %d, want 2 (the conflict loser and the id-less item)", res.Unidentified)
	}
	assertCorpusInvariants(t, s, 3)
}

// TestUnfiledIsWhereAWorkBoundToNoOtherLibraryLands covers the case where the
// BINDING itself points at library 0.
//
// ⚠️ IT DOES NOT EXERCISE writeSearchDoc's FALLBACK, and it used to claim it
// did. Measured: deleting that fallback outright left this test green, because
// applyOneItem writes the library_member row BEFORE it builds the document, so
// the `SELECT … FROM library_member` always finds library 0 and the fallback
// branch is never entered. The fallback's own guard is
// TestRebuildSearchDocFilesAStrandedDocAsUnfiled below.
func TestUnfiledIsWhereAWorkBoundToNoOtherLibraryLands(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	it := item("41", "1", "comic", "Frieren")
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{it}, testNow); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Rebind the same container to library 0 so the next apply files the work
	// nowhere else, then re-apply. writeSearchDoc must reach for Unfiled.
	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM library_member`)
		return err
	}); err != nil {
		t.Fatalf("clearing membership: %v", err)
	}
	orphan := map[string]CatalogueBinding{"1": {LibraryID: UnfiledLibraryID, Kind: "comic"}}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, orphan, []CatalogueItem{it}, testNow); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	var libID int64
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT library_id FROM search_doc_library`).Scan(&libID); err != nil {
		t.Fatalf("read scope: %v", err)
	}
	if libID != UnfiledLibraryID {
		t.Errorf("the doc's only scope is library %d, want the reserved Unfiled library 0", libID)
	}
	assertCorpusInvariants(t, s, 1)
}

// TestRebuildSearchDocFilesAStrandedDocAsUnfiled fires invariant 5's ACTUAL
// landing place.
//
// The fallback is unreachable through ApplyCatalogueBatch — membership is always
// written first — so it is reached the only way it ever will be: by calling the
// builder directly for a work that belongs to no library. That is not a
// contrived shape. Migration 0005 names exactly this case as the debt this code
// owes: "any OTHER library's deletion can still strand a doc whose only scope
// was that library … the search-document builder … must re-file a stranded doc
// into library 0 in the same transaction".
//
// Testing it directly rather than through the batch is the honest option. The
// alternative — deleting the branch because no shipped caller reaches it — would
// remove the one piece of code the migration says owes this invariant, and the
// caller that needs it (the library-delete path) is a later commit.
func TestRebuildSearchDocFilesAStrandedDocAsUnfiled(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	it := item("41", "1", "comic", "Frieren")
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{it}, testNow); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var workID int64
	if err := s.db.Read().QueryRowContext(t.Context(), `SELECT id FROM work`).Scan(&workID); err != nil {
		t.Fatalf("read work: %v", err)
	}

	// Strand it: the work is now a member of nothing at all, which is what a
	// library delete leaves behind.
	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM library_member WHERE work_id = ?`, workID)
		return err
	}); err != nil {
		t.Fatalf("stranding the work: %v", err)
	}

	// Re-derived from the replica rather than rebuilt from `it`, because that is
	// the shape the caller this branch exists for has: the library-delete path
	// holds a work id and no CatalogueItem.
	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		d, err := readSearchDocText(ctx, tx, workID)
		if err != nil {
			return err
		}
		return writeSearchDoc(ctx, tx, workID, d)
	}); err != nil {
		t.Fatalf("writeSearchDoc on a stranded work: %v", err)
	}

	if n := docsWithNoScope(t, s); n != 0 {
		t.Fatalf("%d docs came out of the builder with no scope at all. §7 invariant 5 says "+
			"there must be none: such a doc matches no scope and vanishes from search for "+
			"every user including its owner", n)
	}
	var libID int64
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT library_id FROM search_doc_library`).Scan(&libID); err != nil {
		t.Fatalf("read scope: %v", err)
	}
	if libID != UnfiledLibraryID {
		t.Errorf("the stranded doc was filed into library %d, want the reserved Unfiled library 0", libID)
	}
	assertCorpusInvariants(t, s, 1)
}

// TestSearchDocInvariantQueriesCatchABreak fires the two invariant assertions
// DELIBERATELY, by writing exactly the rows the builder is forbidden to write.
//
// Migration 0005 records both invariants as debts owed by "the search-document
// builder … asserted in CI rather than pretended away" — its words, quoted from
// a migration that is merged and therefore never edited. The venue it names has
// never existed; what actually asserts them is this file, run by `make check`.
// An assertion that has never been triggered is indistinguishable from no
// assertion, so this test breaks each one on purpose and requires the shared
// query to notice.
func TestSearchDocInvariantQueriesCatchABreak(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{item("41", "1", "comic", "Frieren")}, testNow); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if n := docsWithNoScope(t, s); n != 0 {
		t.Fatalf("the builder's own output already violates invariant 5 (%d unscoped docs)", n)
	}

	t.Run("invariant 5 — a doc with no library row", func(t *testing.T) {
		var docID int64
		if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx, `
				INSERT INTO search_doc (work_id, kind, norm_title)
				SELECT id, kind, normalized_title FROM work LIMIT 1`)
			if err != nil {
				return err
			}
			docID, err = res.LastInsertId()
			return err
		}); err != nil {
			t.Fatalf("planting an unscoped doc: %v", err)
		}
		if n := docsWithNoScope(t, s); n != 1 {
			t.Fatalf("the invariant-5 query found %d unscoped docs after one was planted; "+
				"it does not detect what it claims to", n)
		}
		if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM search_doc WHERE rowid = ?`, docID)
			return err
		}); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
		if n := docsWithNoScope(t, s); n != 0 {
			t.Fatalf("cleanup left %d unscoped docs", n)
		}
	})

	t.Run("invariant 2 — an FTS row with no doc", func(t *testing.T) {
		docs, fts, trgm := corpusCounts(t, s)
		if docs != fts || docs != trgm {
			t.Fatalf("the corpus was already unequal before the break: %d/%d/%d", docs, fts, trgm)
		}
		if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO search_fts (rowid, title) VALUES (999999, 'an orphan')`)
			return err
		}); err != nil {
			t.Fatalf("planting an orphan FTS row: %v", err)
		}
		docs2, fts2, trgm2 := corpusCounts(t, s)
		if docs2 == fts2 {
			t.Fatalf("the invariant-2 counts did not move when an orphan FTS row was planted: %d/%d/%d",
				docs2, fts2, trgm2)
		}
		if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM search_fts WHERE rowid = 999999`)
			return err
		}); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	})

	assertCorpusInvariants(t, s, 1)
}

func TestStampFullSyncAndAnalyze(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	at, err := s.LastFullSyncAt(t.Context(), inst)
	if err != nil {
		t.Fatalf("LastFullSyncAt: %v", err)
	}
	if at.Valid {
		t.Fatal("a fresh instance claims a completed full sync")
	}
	if err := s.StampFullSync(t.Context(), inst, testNow); err != nil {
		t.Fatalf("StampFullSync: %v", err)
	}
	at, err = s.LastFullSyncAt(t.Context(), inst)
	if err != nil {
		t.Fatalf("LastFullSyncAt: %v", err)
	}
	if at.String != FormatTime(testNow) {
		t.Errorf("last_full_sync_at = %q, want %q", at.String, FormatTime(testNow))
	}
	if err := s.Analyze(t.Context()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sqlite_schema WHERE name = 'sqlite_stat1'`); n != 1 {
		t.Error("ANALYZE produced no sqlite_stat1: the planner got no statistics")
	}

	// A missing instance must be an error, never a silent no-op, or a freshness
	// claim could be written against nothing.
	if err := s.StampFullSync(t.Context(), 987654, testNow); err == nil {
		t.Error("stamping a nonexistent instance succeeded")
	}
}

func TestRemoteHashCoversOnlyTheSyncedSubset(t *testing.T) {
	a := item("1", "1", "comic", "Berserk")
	b := a
	if a.remoteHash() != b.remoteHash() {
		t.Fatal("the same item hashed differently")
	}
	b.Title = "Berserk Deluxe"
	if a.remoteHash() == b.remoteHash() {
		t.Error("a title change did not move remote_hash")
	}
	// Field-boundary safety: the separator must be one no field can contain, or
	// two different splits hash the same.
	x := item("1", "1", "comic", "ab")
	x.SortTitle = "c"
	y := item("1", "1", "comic", "a")
	y.SortTitle = "bc"
	if x.remoteHash() == y.remoteHash() {
		t.Error(`("ab","c") and ("a","bc") hash identically: the field separator is ambiguous`)
	}
}

func TestIdentityHashIgnoresUpstreamFieldOrder(t *testing.T) {
	// sync.md §4 guard 1 compares this hash to tell "the same item came back"
	// from "the upstream reused an id". A hash that moved when the upstream
	// reordered its JSON would report every item as a resurrection.
	a := item("1", "1", "comic", "x",
		ExternalIdentifier{Source: "anilist", Value: "1", Confidence: 1},
		ExternalIdentifier{Source: "mal", Value: "2", Confidence: 1})
	b := item("1", "1", "comic", "x",
		ExternalIdentifier{Source: "mal", Value: "2", Confidence: 1},
		ExternalIdentifier{Source: "anilist", Value: "1", Confidence: 1})
	if a.identityHash() != b.identityHash() {
		t.Error("identity hash depends on field order")
	}
	c := item("1", "1", "comic", "x", ExternalIdentifier{Source: "anilist", Value: "9", Confidence: 1})
	if a.identityHash() == c.identityHash() {
		t.Error("a different identifier did not move the identity hash")
	}
}

func TestSlugifyIsStableAndNeverEmpty(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Manga", "manga"},
		{"Comics (ongoing)", "comics-ongoing"},
		{"  Light  novels ", "light-novels"},
		{"葬送", "library"},
		{"", "library"},
	} {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBatchWithNoItemsDoesNothing(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, nil, nil, time.Time{})
	if err != nil {
		t.Fatalf("empty batch: %v", err)
	}
	if res.WorksCreated != 0 {
		t.Errorf("an empty batch reported %+v", res)
	}
}

func TestIdentityConflictStringNamesBothWorks(t *testing.T) {
	c := IdentityConflict{Source: "anilist", Value: "30013", ExistingWorkID: 4, AttemptedWorkID: 9, RemoteID: "102"}
	got := c.String()
	for _, want := range []string{"anilist=30013", "work 4", "102", "work 9"} {
		if !strings.Contains(got, want) {
			t.Errorf("conflict string %q is missing %q", got, want)
		}
	}
	_ = fmt.Sprint(c)
}

func TestOnlyAConstraintViolationSkipsAContainer(t *testing.T) {
	// The narrowing in BindContainers, asserted directly because no reachable
	// CatalogueContainer can make SQLite return an I/O error on demand. If this
	// widens to "any error", a failing disk stops being an import failure and
	// starts being a quiet list of libraries the operator is told cannot be
	// bound — which is worse than the abort this whole change removed.
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"unique index", sqlite3.CONSTRAINT_UNIQUE, true},
		{"check constraint", sqlite3.CONSTRAINT_CHECK, true},
		{"foreign key", sqlite3.CONSTRAINT_FOREIGNKEY, true},
		{"wrapped", fmt.Errorf("create library: %w", sqlite3.CONSTRAINT_UNIQUE), true},
		{"disk I/O", sqlite3.IOERR, false},
		{"corrupt database", sqlite3.CORRUPT, false},
		{"database is full", sqlite3.FULL, false},
		{"context cancelled", context.Canceled, false},
	} {
		if got := isSkippableBindError(tc.err); got != tc.want {
			t.Errorf("isSkippableBindError(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

/* ── the second kind over one container, and the kind-change report ─────────
 *
 * Two behaviours that meet in bindOneContainer and are asserted together
 * because they are the same shape from opposite sides: a container bound at a
 * kind it was not bound at before. When that is DELIBERATE (ADR-0066 decision
 * 5's mixed container) the second library is now a PROPOSAL; when it is a
 * CHANGE upstream, and both libraries exist, it needs a RECORD.
 */

// TestASecondKindOverOneContainerIsNowAProposal is what THREE naming tests
// became, and they are folded rather than dropped because all three asserted
// cases of one derivation ADR-0048 removed:
//
//   - TestASiblingLibraryIsNamedForItsKindAndNeverForItsOrder — that a mixed
//     container's second library is `Fiction (Comics)` and never `Fiction (2)`.
//   - TestAKindCollisionQualifiesButANameCollisionStillTakesAnOrdinal — that a
//     SAME-kind slug collision still took the ordinal, which was the boundary
//     between the two disambiguations.
//   - TestAQualifiedNameThatIsItselfTakenFallsBackRatherThanInventingARule —
//     the stop condition, where the qualified name was itself taken.
//
// Each of those named a library that bindOneContainer's step 3 CREATED. Step 3
// creates nothing now, so there is no second library to name: a container bound
// at a second kind, whether by an adapter that retyped it or by the mixed-walk
// mint, yields a proposal and its items land unfiled until somebody accepts one.
//
// ⚠️ ADR-0066 DECISION 5 IS NOT REVERSED BY THIS, and the distinction matters
// because the decision is what stops a comic being filed into a book library.
// Two libraries may still stand over one container ref, they are still resolved
// per kind by steps 1 and 2, and this test's second half proves the kind
// separation survives — what changed is only who creates the second row.
func TestASecondKindOverOneContainerIsNowAProposal(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	// The `book` library exists because somebody accepted it.
	first := acceptedBind(t, s, inst, CatalogueContainer{RemoteID: "1", Name: "Fiction", Kind: "book"})

	second, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		{RemoteID: "1", Name: "Fiction", Kind: "comic"},
	})
	if err != nil {
		t.Fatalf("BindContainers (comic): %v", err)
	}
	if !second["1"].NoLibrary {
		t.Fatalf("the second kind got library %d; nothing on this path creates one",
			second["1"].LibraryID)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE kind = 'comic'`); n != 0 {
		t.Errorf("%d comic libraries were created; the qualifier and the ordinal both went "+
			"with the create they named", n)
	}
	// The original is untouched — it was never this path's to rename, and now
	// there is not even a name being derived beside it.
	if n, sl := libraryNameSlug(t, s, first["1"].LibraryID); n != "Fiction" || sl != "fiction" {
		t.Errorf("the accepted library is (%q, %q), want (\"Fiction\", \"fiction\")", n, sl)
	}

	// AND THE KIND SEPARATION STILL WORKS ONCE BOTH ARE ACCEPTED. This is
	// ADR-0066 decision 5's actual content: one container ref, two libraries, and
	// each kind resolving to its own.
	if _, err := s.AcceptLibraries(t.Context(), OwnerScope(SystemUserID),
		[]LibraryAcceptance{{
			Name: "Fiction (Comics)", Kind: "comic", ManagedBy: "user",
			Sources: []AcceptedSource{{
				ServiceInstanceID: inst, ContainerKind: "remote_library",
				ContainerRef: "1", ContainerIdentity: "Fiction",
			}},
		}}); err != nil {
		t.Fatalf("accept the comic sibling: %v", err)
	}
	both, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		{RemoteID: "1", Name: "Fiction", Kind: "comic"},
	})
	if err != nil {
		t.Fatalf("BindContainers (comic, accepted): %v", err)
	}
	if both["1"].NoLibrary || both["1"].LibraryID == first["1"].LibraryID {
		t.Errorf("the comic container resolved to %+v, want the comic library and not the "+
			"book one — `AND l.kind = ?` on step 1 is what keeps the two apart", both["1"])
	}
}

// TestAContainerWhoseKindChangedIsRecorded is ADR-0066's kind-aware lookup made
// OBSERVABLE.
//
// The lookup itself fixes a defect: the old query matched a container's binding
// at ANY kind and took whichever row SQLite returned first, so a repeated import
// had a nondeterministic bind. Scoping it to the kind makes a retyped container
// land in a library of the right kind — and from the Libraries screen that looks
// like one library emptying and another appearing, with nothing saying why. The
// row is the why.
//
// ⚠️ THE FIXTURE IS NARROWER THAN IT WAS, and the narrowing is a fact about
// ADR-0048 rather than about this row. noteKindChange is reached from step 2 —
// the container JOINS a library of the new kind — and step 2 can only join a
// library that EXISTS. Before ADR-0048 the retype created one, so every retype
// wrote a row; now a retype with no library at the new kind simply leaves the
// items unfiled, and there is no destination to report them moving to. So this
// test builds the state the row still fires on: a library of the new kind that
// somebody accepted, whose name the container matches.
func TestAContainerWhoseKindChangedIsRecorded(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	// The container is bound at `comic`, into a library whose name is NOT the
	// container's — so the join at the new kind is decided by the container name
	// alone, which is what step 2 reads.
	if _, err := s.AcceptLibraries(t.Context(), OwnerScope(SystemUserID),
		[]LibraryAcceptance{
			{
				Name: "Old Comics", Kind: "comic", ManagedBy: "user",
				Sources: []AcceptedSource{{
					ServiceInstanceID: inst, ContainerKind: "remote_library",
					ContainerRef: "1", ContainerIdentity: "Graphic Novels",
				}},
			},
			// The destination: a `book` library the user has, named for the
			// container, with no source of its own yet.
			{Name: "Graphic Novels", Kind: "book", ManagedBy: "user"},
		}); err != nil {
		t.Fatalf("accept the fixture libraries: %v", err)
	}
	first, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Graphic Novels"),
	})
	if err != nil {
		t.Fatalf("BindContainers (comic): %v", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE kind = ?`,
		SyncReportContainerKindChanged); n != 0 {
		t.Fatalf("a bind at the kind it was already bound at wrote %d kind-change rows", n)
	}

	// The retype: same container, the adapter now decides `book`.
	second, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		{RemoteID: "1", Name: "Graphic Novels", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers (book): %v", err)
	}
	if second["1"].LibraryID == first["1"].LibraryID {
		t.Fatalf("the retyped container kept library %d; library.kind is exactly one value, so "+
			"it must not hold items of a kind it does not declare", first["1"].LibraryID)
	}

	var remoteID, detail string
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT remote_id, detail FROM sync_report
		 WHERE kind = ? AND remote_kind = 'library'`,
		SyncReportContainerKindChanged).Scan(&remoteID, &detail); err != nil {
		t.Fatalf("no kind-change row: %v — items appearing to move between libraries with no "+
			"record is the one way this fix looks like a bug", err)
	}
	if remoteID != "1" {
		t.Errorf("the row is filed under container %q, want \"1\"", remoteID)
	}
	var got struct {
		Name string `json:"name"`
		Was  []struct {
			Kind      string `json:"kind"`
			LibraryID int64  `json:"library_id"`
		} `json:"was"`
		Now struct {
			Kind      string `json:"kind"`
			LibraryID int64  `json:"library_id"`
		} `json:"now"`
	}
	if err := json.Unmarshal([]byte(detail), &got); err != nil {
		t.Fatalf("decode %q: %v", detail, err)
	}
	// BOTH SIDES, because the row is read from either end: from the library that
	// emptied ("where did my items go") and from the one that appeared ("is this
	// new data").
	if got.Name != "Graphic Novels" {
		t.Errorf("detail names the container %q, want \"Graphic Novels\"", got.Name)
	}
	if len(got.Was) != 1 || got.Was[0].Kind != "comic" || got.Was[0].LibraryID != first["1"].LibraryID {
		t.Errorf("detail's `was` = %+v, want one entry (comic, library %d)",
			got.Was, first["1"].LibraryID)
	}
	if got.Now.Kind != "book" || got.Now.LibraryID != second["1"].LibraryID {
		t.Errorf("detail's `now` = %+v, want (book, library %d)", got.Now, second["1"].LibraryID)
	}
}

// TestTheSiblingMintIsNotReportedAsAKindChange is the other half, and without it
// the row above fires on every mixed BookOrbit library.
//
// ⚠️ THE SCHEMA CANNOT TELL THE TWO APART. "A container bound at a second kind"
// describes both the ADR-0066 decision 5 sibling and a genuine retype; the
// difference is entirely in WHO ASKED, which is why bindReason exists. A row
// here would report a design as an incident, on the first comic of every mixed
// library, forever.
//
// ⚠️ THE SIBLING IS NO LONGER MINTED AT ALL (ADR-0048) — the comic gets a
// proposal instead — so this test now asserts the same silence over a path that
// creates nothing. That is not redundant: bindReason still routes this call, and
// a future step 2 join at the sibling kind would fire the row through exactly
// this path.
func TestTheSiblingMintIsNotReportedAsAKindChange(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	binds := acceptedBind(t, s, inst, CatalogueContainer{RemoteID: "1", Name: "Fiction", Kind: "book"})

	// The lazy resolve, through the path that used to perform the mint: one comic
	// issue whose parent series is a `comic` under a `book` container's binding.
	var sib CatalogueBinding
	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		sib, err = parentBinding(ctx, tx, inst, binds["1"], "1", CatalogueParent{
			RemoteID: "s1", Kind: "comic", Title: "A Series",
		}, bindCache{})
		return err
	}); err != nil {
		t.Fatalf("parentBinding: %v", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM sync_report WHERE kind = ?`,
		SyncReportContainerKindChanged); n != 0 {
		t.Errorf("the sibling resolve wrote %d kind-change rows; a mixed container holding two "+
			"kinds is ADR-0066 decision 5's design, not a change", n)
	}
	if !sib.NoLibrary {
		t.Errorf("the comic series resolved to library %d; the sibling is a proposal now",
			sib.LibraryID)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE kind = 'comic'`); n != 0 {
		t.Errorf("the resolve produced %d comic libraries, want 0", n)
	}
}

// TestAProvisionalRetypeRefusesALibraryTWOCONTAINERSFEED is the guard the
// end-to-end tests cannot reach, because it takes two service instances to
// build the state.
//
// A provisional container's empty library is retyped in place rather than
// getting a sibling (bindProvisional) — that is what makes a comics-only
// BookOrbit container ONE library named for the container. But `library` is
// joined by NAME and KIND (bindOneContainer step 2), so a Kavita container and a
// BookOrbit container that agree on both are ONE library with TWO
// `library_source` rows — and retyping it on one container's evidence would
// change the kind out from under the other, silently, on a library the other
// service is still filling.
//
// The refusal is the ordinary path, not an error. ⚠️ WHAT IT FALLS BACK TO HAS
// CHANGED: it used to mint the sibling, and since ADR-0048 it falls through to a
// proposal. The refusal itself — the shared library keeps its kind — is what
// this test is for, and it is untouched.
func TestAProvisionalRetypeRefusesALibraryTWOCONTAINERSFEED(t *testing.T) {
	s := newTestStore(t)
	kav := fixtureInstance(t, s, "kavita")
	bo := fixtureInstance(t, s, "bookorbit")

	// ONE accepted library fed by BOTH containers — §17.8's merge, which is what
	// the bind path used to reach by joining on the name key.
	if _, err := s.AcceptLibraries(t.Context(), OwnerScope(SystemUserID),
		[]LibraryAcceptance{{
			Name: "Comics", Kind: "book", ManagedBy: "user",
			Sources: []AcceptedSource{
				{
					ServiceInstanceID: kav, ContainerKind: "remote_library",
					ContainerRef: "7", ContainerIdentity: "Comics",
				},
				{
					ServiceInstanceID: bo, ContainerKind: "remote_library",
					ContainerRef: "7", ContainerIdentity: "Comics",
				},
			},
		}}); err != nil {
		t.Fatalf("accept the shared library: %v", err)
	}
	binds, _, err := s.BindContainers(t.Context(), bo, SystemUserID, []CatalogueContainer{
		{RemoteID: "7", Name: "Comics", Kind: "book", KindProvisional: true},
	})
	if err != nil {
		t.Fatalf("BindContainers(bookorbit): %v", err)
	}
	shared := binds["7"].LibraryID
	if binds["7"].NoLibrary || shared == 0 {
		t.Fatalf("the shared library was not resolved: %+v", binds["7"])
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_source WHERE library_id = ?`, shared); n != 2 {
		t.Fatalf("library %d has %d source rows, want the 2 this fixture is about", shared, n)
	}

	// A comic is now reached in the BookOrbit walk. The library is empty and
	// auto-managed — every retype guard but the source count is satisfied.
	var sib CatalogueBinding
	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		sib, err = parentBinding(ctx, tx, bo, binds["7"], "7", CatalogueParent{
			RemoteID: "s1", Kind: "comic", Title: "A Series",
		}, bindCache{})
		return err
	}); err != nil {
		t.Fatalf("parentBinding: %v", err)
	}

	var kind string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT kind FROM library WHERE id = ?`, shared).Scan(&kind); err != nil {
		t.Fatalf("read the shared library's kind: %v", err)
	}
	if kind != "book" {
		t.Errorf("the shared library's kind = %q, want \"book\" — it is fed by a Kavita "+
			"container too, and one container's evidence must not retype a library the "+
			"other is still filling", kind)
	}
	if !sib.NoLibrary {
		t.Errorf("the refusal produced library %d; falling through now yields a proposal",
			sib.LibraryID)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE kind = 'comic'`); n != 0 {
		t.Errorf("the refusal produced %d comic libraries, want 0", n)
	}
}

// ── container_observed: one row per container per import ────────────────────

// observations reads the container_observed rows for one instance, newest first
// per container, decoded through the same struct the writer encodes.
func observations(t *testing.T, s *Store, instanceID int64) map[string][]containerObservation {
	t.Helper()
	rows, err := s.db.Read().QueryContext(t.Context(), `
		SELECT remote_kind, remote_id, detail FROM sync_report
		 WHERE service_instance_id = ? AND kind = ?
		 ORDER BY id DESC`, instanceID, SyncReportContainerObserved)
	if err != nil {
		t.Fatalf("read container_observed rows: %v", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]containerObservation{}
	for rows.Next() {
		var remoteKind, remoteID, detail string
		if err := rows.Scan(&remoteKind, &remoteID, &detail); err != nil {
			t.Fatalf("read container_observed rows: scan: %v", err)
		}
		if remoteKind != "library" {
			t.Errorf("observation of %q has remote_kind %q, want \"library\" — the "+
				"newest-row probe seeks on that column, so a different value is a row "+
				"no reader will ever find", remoteID, remoteKind)
		}
		var o containerObservation
		if err := json.Unmarshal([]byte(detail), &o); err != nil {
			t.Fatalf("decode observation of %q (%q): %v", remoteID, detail, err)
		}
		out[remoteID] = append(out[remoteID], o)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read container_observed rows: %v", err)
	}
	return out
}

// TestBindContainersObservesEveryContainerItSaw is the whole of the row's
// contract: EVERY container in the list, whatever happened to it.
//
// The three fates are all here on purpose, because the row is written at the one
// point in the loop where all three are still ahead of it:
//
//   - BOUND — the ordinary case, and the only one the old container_declined row
//     said nothing about.
//   - DECLINED (Kind == "") — skipped by the bind loop's `continue`, so an
//     observation written after it would be missing exactly the containers
//     §17.8's `Decision` column has to render.
//   - UNTYPABLE — a kind library.kind's CHECK does not permit, so no proposal
//     for it can ever be accepted. It is still a container the upstream reported
//     and §17.8 still renders it, with a reason.
//
// ⚠️ THE THIRD FATE USED TO BE "ROLLED BACK TO ITS SAVEPOINT", and it is not any
// more: with no create in the bind path, an unknown kind violates no constraint
// (see TestAnUnknownKindNoLongerSkipsAContainer). The observation is still
// written BEFORE the savepoint, deliberately, so that a bind which does roll
// back cannot take the record of what upstream said with it — but no reachable
// error produces that rollback today, so this test cannot exercise the position
// and does not claim to.
func TestBindContainersObservesEveryContainerItSaw(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	cs := []CatalogueContainer{
		comicContainer("1", "Manga"),
		{RemoteID: "2", Name: "Podcasts", Kind: "", DeclineReason: "no work.kind for a podcast"},
		{RemoteID: "3", Name: "Sheet Music", Kind: "score"},
		{RemoteID: "4", Name: "Ebooks", Kind: "book", KindProvisional: true},
	}
	if _, _, err := s.BindContainers(t.Context(), inst, SystemUserID, cs); err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

	got := observations(t, s, inst)
	if len(got) != len(cs) {
		t.Fatalf("observed %d containers, want %d (%+v)", len(got), len(cs), got)
	}
	for _, want := range []containerObservation{
		{Name: "Manga", Kind: "comic"},
		{Name: "Podcasts", Kind: "", DeclineReason: "no work.kind for a podcast"},
		{Name: "Sheet Music", Kind: "score"},
		{Name: "Ebooks", Kind: "book", KindProvisional: true},
	} {
		ref := map[string]string{
			"Manga": "1", "Podcasts": "2", "Sheet Music": "3", "Ebooks": "4",
		}[want.Name]
		seen := got[ref]
		if len(seen) != 1 {
			t.Errorf("container %q has %d observations, want 1 per import", ref, len(seen))
			continue
		}
		if seen[0] != want {
			t.Errorf("observation of container %q:\n  got:  %+v\n  want: %+v", ref, seen[0], want)
		}
	}
}

// A CONTAINER THE UPSTREAM STOPS REPORTING STOPS GAINING ROWS, and that is the
// fact ProposedContainers' not-seen-by-last-sync boolean is derived from. The
// row is per import rather than per container, so the newest one is the last
// time this instance's adapter named it.
func TestContainerObservationsAccumulatePerImport(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	first := []CatalogueContainer{comicContainer("1", "Manga"), comicContainer("2", "Webtoons")}
	if _, _, err := s.BindContainers(t.Context(), inst, SystemUserID, first); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	// The second run sees only container 1, and sees it under a NEW NAME.
	if _, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{comicContainer("1", "Manga & Manhwa")}); err != nil {
		t.Fatalf("second bind: %v", err)
	}

	got := observations(t, s, inst)
	if len(got["1"]) != 2 {
		t.Errorf("container 1 has %d observations after two imports, want 2", len(got["1"]))
	} else if got["1"][0].Name != "Manga & Manhwa" {
		t.Errorf("the newest observation of container 1 names it %q, want the name the "+
			"SECOND import reported — the read takes the newest row and a stale name "+
			"there is a stale proposal", got["1"][0].Name)
	}
	if len(got["2"]) != 1 {
		t.Errorf("container 2 has %d observations, want the 1 from the run that saw it: "+
			"a container the upstream stopped reporting must stop gaining rows, which is "+
			"the whole of how its absence is detected", len(got["2"]))
	}
}

// TestContainerObservationsAreRedactedAtRest is the security half of the row's
// contract, and it is asserted at rest rather than on the wire.
//
// `sync_report.detail` promises redaction on the way in (migration 00005), and
// the reason is not only the secret: ProposedContainers surfaces this name as
// §17.8's suggested name, and a value redacted on the READ would differ from the
// value stored — which is what makes an unedited proposal compare as edited. The
// stored string is the only string, and that is what this asserts.
func TestContainerObservationsAreRedactedAtRest(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	const raw = "Manga (from http://kavita.test:5000/api/Library?apiKey=SUPERSECRET)"
	if _, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{{RemoteID: "1", Name: raw, Kind: "comic"}}); err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

	got := observations(t, s, inst)["1"]
	if len(got) != 1 {
		t.Fatalf("observations = %+v, want exactly 1", got)
	}
	if strings.Contains(got[0].Name, "SUPERSECRET") {
		t.Errorf("the stored name carries the api key: %q\n"+
			"It is in the SQLite file, so it is in `usarr backup` output and in "+
			"anything that later logs the row. A wire-side redaction closes none of "+
			"that.", got[0].Name)
	}
	// AND IT IS THE FUNCTION'S OWN ANSWER, not a hand-rolled scrub: one
	// implementation of the deny-list (internal/ssrf's rule), so a container name
	// this store rewrites differently from every other free-text path is a second
	// implementation nobody is maintaining.
	if want := ssrf.RedactText(raw); got[0].Name != want {
		t.Errorf("the stored name is %q, want ssrf.RedactText's own answer %q", got[0].Name, want)
	}
}

// ── the unfiled state: applied, searchable, and a member of nothing ─────────

// TestAnUnacceptedContainersItemsAreAppliedAndFiledNowhere is the heart of
// ADR-0048's ordering, and every assertion in it is about an EFFECT rather than
// a field.
//
// The amendment of 2026-08-21 runs the first import BEFORE Accept so that the
// proposals carry real item counts. That is only true if an item whose container
// has no accepted library is written in full — and only safe if it is written
// into no library at all, including the reserved one.
//
// ⚠️ THE `library_member` ASSERTION NAMES library 0 EXPLICITLY, and that is what
// makes it a test rather than a label. `NoLibrary` spelled as `LibraryID == 0`
// compiles, type-checks and passes any assertion phrased as "the binding says
// unfiled" — and files every work into UnfiledLibraryID's membership, where §7
// invariant 5 has already put the document. The membership row is the one thing
// that tells the two apart.
func TestAnUnacceptedContainersItemsAreAppliedAndFiledNowhere(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if !binds["1"].NoLibrary {
		t.Fatalf("the container was bound to library %d; this test measures nothing without "+
			"the unfiled state", binds["1"].LibraryID)
	}
	res, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{item("41", "1", "comic", "Frieren")}, testNow)
	if err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	// APPLIED. The batch did the work, and said so.
	if res.WorksCreated != 1 || res.SearchDocs != 1 {
		t.Errorf("result = %+v, want one work and one document — an item with no library "+
			"must still be replicated, or the proposal beside it counts nothing", res)
	}
	if res.Members != 0 {
		t.Errorf("result reports %d membership writes, want 0", res.Members)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE deleted_at IS NULL`); n != 1 {
		t.Errorf("work rows = %d, want 1", n)
	}
	// THE LINK, WITH ITS CONTAINER. This is the column §17.8's item count joins
	// on, so an item applied without it is an item no proposal can count.
	var container string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT remote_library_id FROM service_item_link WHERE service_instance_id = ?`,
		inst).Scan(&container); err != nil {
		t.Fatalf("read the link: %v", err)
	}
	if container != "1" {
		t.Errorf("the link names container %q, want \"1\"", container)
	}

	// FILED NOWHERE — including, and especially, not into library 0.
	if n := count(t, s, `SELECT COUNT(*) FROM library_member`); n != 0 {
		t.Errorf("%d library_member rows exist, want 0", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member WHERE library_id = ?`,
		UnfiledLibraryID); n != 0 {
		t.Errorf("%d works were filed into the reserved Unfiled library. `NoLibrary` spelled "+
			"as `LibraryID == 0` produces exactly this row, and nothing else in the schema "+
			"distinguishes it from a library the user accepted", n)
	}

	// AND STILL SEARCHABLE, through §7 invariant 5's EXISTING fallback and no new
	// rule: a document whose work is a member of nothing is scoped to library 0.
	var workID int64
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT id FROM work LIMIT 1`).Scan(&workID); err != nil {
		t.Fatalf("read the work: %v", err)
	}
	if got := docScopes(t, s, workID); len(got) != 1 || got[0] != UnfiledLibraryID {
		t.Errorf("the unfiled work's document is scoped to %v, want exactly [%d] — invariant "+
			"5's fallback is what keeps a pre-Accept catalogue searchable, and a doc with no "+
			"scope row is invisible to every user including its owner", got, UnfiledLibraryID)
	}
	if n := docsWithNoScope(t, s); n != 0 {
		t.Errorf("§7 invariant 5 broken: %d documents have no scope at all", n)
	}
}

// TestADeclinedContainerIsNotAnUnfiledOne is the distinction ADR-0048 leaves
// standing, asserted by what happens to the ITEMS rather than by a flag.
//
// A DECLINE is the adapter saying UsArr has no work.kind for this container, so
// there is nothing to write and its items are never applied. "No accepted
// library" is a user who has not ticked a box yet, and its items are applied in
// full. Collapsing the two in either direction is a data-loss bug in one
// direction and an unwritable-row bug in the other.
func TestADeclinedContainerIsNotAnUnfiledOne(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Manga"),
		{RemoteID: "2", Name: "Wallpapers", DeclineReason: "no work.kind for an image"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{
		item("41", "1", "comic", "Frieren"),
		item("42", "2", "comic", "A Wallpaper"),
	}, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	// The unfiled item was applied; the declined one was not.
	if n := count(t, s, `SELECT COUNT(*) FROM work`); n != 1 {
		t.Fatalf("%d works exist, want 1 — the unfiled container's item and not the "+
			"declined container's", n)
	}
	var container string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT remote_library_id FROM service_item_link`).Scan(&container); err != nil {
		t.Fatalf("read the link: %v", err)
	}
	if container != "1" {
		t.Errorf("the one link came from container %q, want the UNFILED container \"1\"; "+
			"a declined container's items are never fetched, let alone written", container)
	}
	// Both are visible to the Accept screen, and only one of them will ever be
	// acceptable — which is why the observation carries the decline reason.
	got := observations(t, s, inst)
	if len(got["1"]) != 1 || len(got["2"]) != 1 {
		t.Fatalf("observations = %+v, want one per container", got)
	}
	if got["1"][0].DeclineReason != "" || got["2"][0].DeclineReason == "" {
		t.Errorf("the decline reason did not travel with the right container: %+v", got)
	}
}

// TestTheSweepIsBlindToTheUnfiledState is the cheap assertion that ADR-0048 did
// not reach channel 4.
//
// Both halves are readable from the code and are pinned here because "it does
// not touch that" is exactly the claim that rots: `absentLinks` keys on
// `service_item_link` and never reads `library_member` or `library`, so an
// unfiled item is swept precisely as a filed one is; and `sweepOrphans` draws
// its candidates from `fedLibraries`, which reads `library_source`, so a
// container with no accepted library has nothing that could be stamped orphaned.
func TestTheSweepIsBlindToTheUnfiledState(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{
		item("41", "1", "comic", "Frieren"),
		item("42", "1", "comic", "Berserk"),
	}, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	// The upstream stops reporting item 42. The container is still reported.
	res, err := s.SweepDeletions(t.Context(), inst,
		[]LinkRef{{RemoteKind: "series", RemoteID: "41"}},
		SweepScope{Reported: []string{"1"}, Observed: []string{"1"}},
		testNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("SweepDeletions: %v", err)
	}
	if res.LinksTombstoned != 1 {
		t.Errorf("result = %+v, want the one absent link tombstoned — the sweep keys on "+
			"service_item_link and an unfiled item is as sweepable as a filed one", res)
	}
	if res.LibrariesOrphaned != 0 || res.SourcesMissing != 0 {
		t.Errorf("result = %+v, want nothing stamped: with no library_source row there is "+
			"no candidate for fedLibraries to hand sweepOrphans", res)
	}
}
