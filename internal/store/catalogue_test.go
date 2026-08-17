package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
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

func TestBindContainersCreatesOneLibraryPerContainer(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita-1")

	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Manga"),
		{RemoteID: "2", Name: "Ebooks", Kind: "book"},
		{RemoteID: "4", Name: "Wallpapers", DeclineReason: "no UsArr kind"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if len(binds) != 2 {
		t.Fatalf("bound %d containers, want 2 (the declined one must get no library)", len(binds))
	}
	if _, ok := binds["4"]; ok {
		t.Error("a declined container was bound to a library")
	}
	if binds["1"].Kind != "comic" || binds["2"].Kind != "book" {
		t.Errorf("kinds came out wrong: %+v", binds)
	}
	if !binds["1"].Created || !binds["2"].Created {
		t.Error("both libraries were new, so both should report Created")
	}

	// The source rows are container_kind='remote_library' with the upstream's
	// own id as the ref — the predicate ix_sil_container serves.
	var kind, ref, identity string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT container_kind, container_ref, container_identity FROM library_source
		  WHERE library_id = ?`, binds["1"].LibraryID).Scan(&kind, &ref, &identity); err != nil {
		t.Fatalf("read library_source: %v", err)
	}
	if kind != "remote_library" || ref != "1" || identity != "Manga" {
		t.Errorf("library_source = (%q,%q,%q), want (remote_library, 1, Manga)", kind, ref, identity)
	}
}

func TestBindContainersIsIdempotentAndJoinsBySameNameAndKind(t *testing.T) {
	s := newTestStore(t)
	a := fixtureInstance(t, s, "kavita-a")
	b := fixtureInstance(t, s, "kavita-b")

	first, err := s.BindContainers(t.Context(), a, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers a: %v", err)
	}
	// Re-running the same instance must not create a second library.
	again, err := s.BindContainers(t.Context(), a, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers a again: %v", err)
	}
	if again["1"].LibraryID != first["1"].LibraryID || again["1"].Created {
		t.Errorf("a re-import created a second library: %+v then %+v", first["1"], again["1"])
	}

	// §17.8: a SECOND INSTANCE of the same kind JOINS the existing library
	// rather than creating a new one. The merge key is case-insensitive and
	// whitespace-trimmed, so "  manga  " must join "Manga".
	joined, err := s.BindContainers(t.Context(), b, SystemUserID, []CatalogueContainer{comicContainer("7", "  manga  ")})
	if err != nil {
		t.Fatalf("BindContainers b: %v", err)
	}
	if joined["7"].LibraryID != first["1"].LibraryID {
		t.Errorf("second instance created library %d instead of joining %d — §17.8's "+
			"default is join, and getting it wrong destroys the two-source badge",
			joined["7"].LibraryID, first["1"].LibraryID)
	}
	if joined["7"].Created {
		t.Error("a join reported Created")
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_source WHERE library_id = ?`, first["1"].LibraryID); n != 2 {
		t.Errorf("the joined library has %d sources, want 2", n)
	}
}

func TestBindContainersWillNotJoinAcrossKinds(t *testing.T) {
	// library.kind is exactly one value, so a name collision between a comic
	// container and a book container must NOT join: the books would render in a
	// comics library and no format filter could rescue them.
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")

	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Library"),
		{RemoteID: "2", Name: "Library", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if binds["1"].LibraryID == binds["2"].LibraryID {
		t.Fatal("a comic container and a book container were joined into one library")
	}
	var name, kind string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT name, kind FROM library WHERE id = ?`, binds["2"].LibraryID).Scan(&name, &kind); err != nil {
		t.Fatalf("read library: %v", err)
	}
	if name == "Library" || kind != "book" {
		t.Errorf("disambiguated library = (%q, %q), want a distinct name and kind book", name, kind)
	}
}

func TestApplyCatalogueBatchWritesTheWholeShape(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		comicContainer("1", "Manga"),
		{RemoteID: "2", Name: "Ebooks", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

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
	// the delete-then-insert in rebuildSearchDoc this is where invariant 2
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
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
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
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
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
	bindsA, err := s.BindContainers(t.Context(), a, SystemUserID, []CatalogueContainer{comicContainer("1", "A")})
	if err != nil {
		t.Fatalf("bind a: %v", err)
	}
	bindsB, err := s.BindContainers(t.Context(), b, SystemUserID, []CatalogueContainer{comicContainer("1", "B")})
	if err != nil {
		t.Fatalf("bind b: %v", err)
	}
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
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
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
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
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
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
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
// ⚠️ IT DOES NOT EXERCISE rebuildSearchDoc's FALLBACK, and it used to claim it
// did. Measured: deleting that fallback outright left this test green, because
// applyOneItem writes the library_member row BEFORE it builds the document, so
// the `SELECT … FROM library_member` always finds library 0 and the fallback
// branch is never entered. The fallback's own guard is
// TestRebuildSearchDocFilesAStrandedDocAsUnfiled below.
func TestUnfiledIsWhereAWorkBoundToNoOtherLibraryLands(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	it := item("41", "1", "comic", "Frieren")
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{it}, testNow); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Rebind the same container to library 0 so the next apply files the work
	// nowhere else, then re-apply. rebuildSearchDoc must reach for Unfiled.
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
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
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

	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return rebuildSearchDoc(ctx, tx, workID, it)
	}); err != nil {
		t.Fatalf("rebuildSearchDoc on a stranded work: %v", err)
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
// builder … asserted in CI rather than pretended away". An assertion that has
// never been triggered is indistinguishable from no assertion, so this test
// breaks each one on purpose and requires the shared query to notice.
func TestSearchDocInvariantQueriesCatchABreak(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita")
	binds, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{comicContainer("1", "Manga")})
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
