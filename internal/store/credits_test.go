package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Every test in this file runs against a REAL MIGRATED DATABASE and POPULATES
// BEFORE IT ASSERTS, on catalogue_test.go's rule: an invariant asserted over an
// empty table is a query with no subject.

// importOne runs one item through the real catalogue path so that the credit
// pass has a service_item_link to resolve — which is the whole point of running
// it rather than inserting a `work` row directly. A credit set whose remote item
// was never imported takes the ItemsUnresolved branch, so a test that skipped
// the import would assert nothing about the credit write at all.
func importOne(t *testing.T, s *Store, inst int64, binds map[string]CatalogueBinding, it CatalogueItem) int64 {
	t.Helper()
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{it}, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	var id int64
	if err := s.db.Read().QueryRowContext(t.Context(), `
		SELECT work_id FROM service_item_link
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?`,
		inst, it.RemoteKind, it.RemoteID).Scan(&id); err != nil {
		t.Fatalf("the item did not import: %v", err)
	}
	return id
}

// creditFixture imports two books into one library and returns the store, the
// instance and both work ids.
func creditFixture(t *testing.T, s *Store) (inst, bookA, bookB int64) {
	t.Helper()
	inst = fixtureInstance(t, s, "kavita-credits")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{{RemoteID: "2", Name: "Ebooks", Kind: "book"}})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	bookA = importOne(t, s, inst, binds, item("10", "2", "book", "Piranesi"))
	bookB = importOne(t, s, inst, binds, item("11", "2", "book", "Jonathan Strange & Mr Norrell"))
	return inst, bookA, bookB
}

func credit(role, name string, pos int) Credit {
	// The normalised form is spelled out rather than computed, because
	// internal/store cannot import the normaliser (that package imports this
	// one) and a test that recomputed it with its own copy would agree with
	// itself no matter what the adapter does.
	return Credit{Role: role, Name: name, NormalizedName: normForTest(name), Position: pos}
}

// normForTest is the v1 normaliser's behaviour for the ASCII names in this file:
// lowercase, punctuation to a separator, whitespace collapsed. It is
// deliberately NOT a reimplementation of NormalizeTitle — it handles exactly the
// inputs here — and TestPersonNormVersionMatchesTheNormaliser is what ties the
// real one to this table.
func normForTest(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = b.Len() > 0
		}
	}
	return b.String()
}

// ─── The dedupe guard ───────────────────────────────────────────────────────

// TestTwoWorksByOneAuthorShareOnePersonWork is THE dedupe assertion.
//
// Two books by Susanna Clarke must produce ONE work of kind 'person' and TWO
// credits pointing at it. The failure this guards is not cosmetic: a person work
// per credit turns `work` into a table whose row count grows with credits rather
// than with the catalogue, and every one of those rows carries a kind the
// navigation enum has no home for.
//
// It also covers the two halves separately, because they fail independently: the
// per-transaction cache (both books in ONE ApplyCredits call) and the database
// lookup (the second book in a LATER call, which a cache-only dedupe would
// miss). The second half is the one that matters — a real import batches at
// 2,000 rows and a large library crosses batches.
func TestTwoWorksByOneAuthorShareOnePersonWork(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	// Half 1: two books credited in ONE call.
	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{credit("author", "Susanna Clarke", 0)}},
		{RemoteKind: "series", RemoteID: "11", Credits: []Credit{credit("author", "Susanna Clarke", 0)}},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if res.PeopleCreated != 1 || res.PeopleReused != 1 {
		t.Errorf("one author on two books = %d created / %d reused, want 1/1",
			res.PeopleCreated, res.PeopleReused)
	}
	if res.CreditsWritten != 2 {
		t.Errorf("CreditsWritten = %d, want 2", res.CreditsWritten)
	}

	// Half 2: a SEPARATE call, which is what a second batch or a second import
	// looks like. Nothing may be created.
	res2, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{credit("author", "Susanna Clarke", 0)}},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits again: %v", err)
	}
	if res2.PeopleCreated != 0 {
		t.Errorf("a second ApplyCredits call created %d person work(s); the dedupe is a "+
			"per-transaction cache and not a database lookup", res2.PeopleCreated)
	}

	people := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'person'`)
	if people != 1 {
		t.Errorf("`work` holds %d person rows for one author, want 1.\n"+
			"The dedupe key is work.normalized_title under kind='person'; if this is 2 or 3, "+
			"either the key is not being used or the lookup is not seeing committed rows.", people)
	}
	credits := count(t, s, `SELECT COUNT(*) FROM work_credit`)
	if credits != 2 {
		t.Errorf("work_credit holds %d rows, want 2 (one author, two books)", credits)
	}

	// And the two credits really do point at the SAME row.
	distinct := count(t, s, `SELECT COUNT(DISTINCT creator_work_id) FROM work_credit`)
	if distinct != 1 {
		t.Errorf("the two credits point at %d distinct creators, want 1", distinct)
	}
}

// TestCaseAndPunctuationVariantsAreOnePerson covers the variants the normaliser
// is supposed to absorb, and the one it is NOT.
//
// The split case is asserted as a KNOWN LOSS rather than left undiscovered:
// "A. Moore" and "Alan Moore" become two people, nothing merges them, and
// credits.go's doc comment names the seam (PersonDto.aliases) that would.
func TestCaseAndPunctuationVariantsAreOnePerson(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Alan Moore", 0),
			credit("writer", "alan  moore", 0),
			credit("editor", "ALAN MOORE!", 0),
		}},
		{RemoteKind: "series", RemoteID: "11", Credits: []Credit{
			credit("author", "A. Moore", 0),
		}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}

	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'person'`); n != 2 {
		t.Errorf("%d person rows, want 2 — three spellings of Alan Moore collapse, "+
			"and 'A. Moore' does not", n)
	}
	var title string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT title FROM work WHERE kind = 'person' AND normalized_title = 'alan moore'`).
		Scan(&title); err != nil {
		t.Fatalf("the three spellings did not collapse onto one row: %v", err)
	}
	// FIRST WRITER WINS on the displayed title. It is asserted rather than left
	// implicit: the person row is created once and never re-titled, so the
	// spelling that reaches the importer first is the one a credit renders.
	if title != "Alan Moore" {
		t.Errorf("the person's title is %q, want the first spelling seen, \"Alan Moore\"", title)
	}
}

// ─── The corpus guard ───────────────────────────────────────────────────────

// peopleInCorpus is the invariant query for schema.md §1.1's exclusion rule,
// written once and shared by the assertion and by the test that breaks it —
// catalogue_test.go's reason: a copy-pasted variant is how an invariant
// assertion silently stops covering the thing it names.
//
// It asks the question directly rather than counting: does any search_doc row
// point at a work of kind 'person'? A count-based check ("docs == items") passes
// the moment someone also stops indexing something else.
func peopleInCorpus(t *testing.T, s *Store) int {
	t.Helper()
	return count(t, s, `
		SELECT COUNT(*) FROM search_doc d
		  JOIN work w ON w.id = d.work_id
		 WHERE w.kind = 'person'`)
}

// TestPeopleNeverEnterTheSearchCorpus is the §1.1 exclusion guard.
//
// docs/reference/schema.md §1.1 and migration 0005's own comment on
// search_doc.kind ("'season','episode','track','comic_issue' and 'person' are
// excluded") make this a rule, and until ADR-0044 nothing could break it because
// nothing created a person. Now something does, so the rule needs a witness.
//
// WHY IT MATTERS BEYOND TIDINESS: a person hit is a search result with nowhere
// to land — there is no person screen in any milestone — and every person doc
// also lands in a library through search_doc_library, so an author would appear
// inside the user's Ebooks library grid as an item.
//
// It asserts on THREE tables, not one, because they fail independently:
// search_doc (the doc), search_fts/search_trgm (the index rows, which carry no
// foreign key and no trigger can reach), and search_doc_library (the visibility
// junction).
func TestPeopleNeverEnterTheSearchCorpus(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
			credit("cover_artist", "Vince Haig", 0),
		}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}

	// The fixture is populated: two people exist and two books are indexed.
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'person'`); n != 2 {
		t.Fatalf("fixture: %d person works, want 2 — nothing below would mean anything", n)
	}
	assertCorpusInvariants(t, s, 2)

	if n := peopleInCorpus(t, s); n != 0 {
		t.Errorf("%d person work(s) have a search_doc row. schema.md §1.1 excludes 'person' "+
			"from the FTS corpus because there is no person screen in any milestone, so the "+
			"hit is a result with nowhere to land — and search_doc_library would also put "+
			"the author inside the user's library grid as an item", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM search_doc WHERE kind = 'person'`); n != 0 {
		t.Errorf("%d search_doc rows carry kind='person'", n)
	}
	// The two FTS tables are checked through the corpus-count invariant above,
	// which is only equal because no extra rows were inserted for the people.
	// This asserts the total directly so a person doc cannot hide behind a
	// symmetric insert into all three.
	docs, fts, trgm := corpusCounts(t, s)
	if docs != 2 || fts != 2 || trgm != 2 {
		t.Errorf("corpus = doc %d / fts %d / trgm %d, want 2/2/2 — the two BOOKS and "+
			"neither of their creators", docs, fts, trgm)
	}
	// And no person is visible through any library.
	if n := count(t, s, `
		SELECT COUNT(*) FROM library_member m JOIN work w ON w.id = m.work_id
		 WHERE w.kind = 'person'`); n != 0 {
		t.Errorf("%d person work(s) were filed into a library; a person is not a "+
			"catalogue item and belongs to none", n)
	}
}

// ─── The rest of the write path ─────────────────────────────────────────────

// TestCreditsAreReplacedWholesale pins the rule that a credit REMOVED upstream
// is removed here — the same rule work_alt_title has, for the same reason.
func TestCreditsAreReplacedWholesale(t *testing.T) {
	s := newTestStore(t)
	inst, bookA, bookB := creditFixture(t, s)

	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
			credit("editor", "Someone Removed Later", 0),
		}},
		{RemoteKind: "series", RemoteID: "11", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
		}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_credit WHERE work_id = ?`, bookA); n != 2 {
		t.Fatalf("fixture: book A has %d credits, want 2", n)
	}

	// The editor is gone upstream.
	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
		}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits again: %v", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_credit WHERE work_id = ?`, bookA); n != 1 {
		t.Errorf("book A kept %d credits after the editor was dropped upstream, want 1", n)
	}
	// The OTHER book's credits are untouched — the delete is work-scoped.
	if n := count(t, s, `SELECT COUNT(*) FROM work_credit WHERE work_id = ?`, bookB); n != 1 {
		t.Errorf("book B lost credits it still has upstream: %d rows, want 1", n)
	}
	// And the now-uncredited person row SURVIVES. It is not garbage-collected
	// here: a person credited on nothing is harmless (no doc, no library, no
	// screen) and collecting it inside a per-work replace would delete a person
	// another work still credits whenever the two are in different batches.
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'person'`); n != 2 {
		t.Errorf("%d person works after the drop, want 2 — the uncredited one is left "+
			"standing on purpose", n)
	}
}

// TestCreditsWithNoImportedItemAreCountedNotWritten covers the partial-import
// path: the credit pass ran over an item whose batch never committed.
func TestCreditsWithNoImportedItemAreCountedNotWritten(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "99", Credits: []Credit{credit("author", "Nobody", 0)}},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if res.ItemsUnresolved != 1 || res.CreditsWritten != 0 || res.PeopleCreated != 0 {
		t.Errorf("an unimported item gave %+v, want ItemsUnresolved=1 and nothing written", res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'person'`); n != 0 {
		t.Errorf("a person was created for an item that does not exist (%d rows)", n)
	}
}

// TestAnIllegalRoleIsDroppedNotFatal pins the CreditsRejected path: one bad role
// costs its own credit and nothing else.
func TestAnIllegalRoleIsDroppedNotFatal(t *testing.T) {
	s := newTestStore(t)
	inst, bookA, _ := creditFixture(t, s)

	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
			credit("chief-vibes-officer", "Not A Role", 0),
			credit("editor", "Real Editor", 0),
		}},
	}, testNow)
	if err != nil {
		t.Fatalf("one illegal role aborted the whole batch: %v", err)
	}
	if res.CreditsRejected != 1 {
		t.Errorf("CreditsRejected = %d, want 1", res.CreditsRejected)
	}
	if res.CreditsWritten != 2 {
		t.Errorf("CreditsWritten = %d, want 2 — the two legal credits must survive", res.CreditsWritten)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_credit WHERE work_id = ?`, bookA); n != 2 {
		t.Errorf("book A has %d credits, want 2", n)
	}
}

// TestDuplicateCreditInOneSetIsIdempotent covers the INSERT OR IGNORE: an
// upstream that sends the same person twice in one role array does not fail the
// import.
func TestDuplicateCreditInOneSetIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	inst, bookA, _ := creditFixture(t, s)

	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
			credit("author", "susanna clarke", 0), // same person, same role, same position
		}},
	}, testNow)
	if err != nil {
		t.Fatalf("a duplicated credit failed the batch: %v", err)
	}
	if res.CreditsWritten != 1 {
		t.Errorf("CreditsWritten = %d, want 1 — the second is the same fact", res.CreditsWritten)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_credit WHERE work_id = ?`, bookA); n != 1 {
		t.Errorf("book A has %d credits, want 1", n)
	}
}

// TestCreditedWorksAreArtistsOrPeople is the application-enforced invariant
// migration 0007's DDL comment names, run over a POPULATED table.
//
// SQLite cannot express "this foreign key's target must have kind IN
// ('artist','person')" — a CHECK cannot subquery — so it is asserted here, in
// the exact form the migration writes down.
func TestCreditedWorksAreArtistsOrPeople(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
			credit("cover_artist", "Vince Haig", 0),
		}},
		{RemoteKind: "series", RemoteID: "11", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
		}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_credit`); n != 3 {
		t.Fatalf("fixture: %d credits, want 3 — the assertion below would be vacuous", n)
	}
	if n := count(t, s, `
		SELECT COUNT(*) FROM work_credit c
		  JOIN work w ON w.id = c.creator_work_id
		 WHERE w.kind NOT IN ('artist','person')`); n != 0 {
		t.Errorf("%d credit(s) point at a work that is neither an artist nor a person. "+
			"schema.md §1.1 makes that a rule, not a convention: a credit pointing at a "+
			"'book' means a book is credited as its own author", n)
	}
}

// TestPersonNormVersionMatchesTheNormaliser ties the constant this package
// stamps on a person work to the one internal/libsync actually implements.
//
// The duplication is deliberate — internal/store cannot import internal/libsync,
// which imports it — and this is the check that keeps the two equal. It reads
// the constant out of the source file rather than importing it, which is the
// only way to compare them from this side of the dependency edge.
func TestPersonNormVersionMatchesTheNormaliser(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "libsync", "normalize.go"))
	if err != nil {
		t.Fatalf("read the normaliser: %v", err)
	}
	want := fmt.Sprintf("const NormVersion int64 = %d", NormVersionForPeople)
	if !strings.Contains(string(raw), want) {
		t.Errorf("internal/libsync/normalize.go does not declare %q.\n"+
			"store.NormVersionForPeople is stamped on every 'person' work and must equal "+
			"libsync.NormVersion; if the normaliser was versioned up, the person rows "+
			"written before it are now mislabelled and need the same backfill every other "+
			"work does.", want)
	}
}

// TestWorkCreditRoleVocabularyMatchesTheReference reads the vocabulary out of
// docs/reference/schema.md and requires the shipped migration to match it.
//
// §1.1 is authoritative for this table's shape and the CHECK is the vocabulary's
// only enforcement, so a role added to the document and not to the database is a
// role an adapter can be written against and cannot store.
func TestWorkCreditRoleVocabularyMatchesTheReference(t *testing.T) {
	s := newTestStore(t)

	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "schema.md"))
	if err != nil {
		t.Fatalf("read schema.md: %v", err)
	}
	// The reference DDL block for work_credit, from its CREATE to its closing
	// paren. Bounded rather than searched globally, so a role word appearing in
	// prose elsewhere cannot satisfy this.
	doc := string(raw)
	start := strings.Index(doc, "CREATE TABLE work_credit (")
	if start < 0 {
		t.Fatal("schema.md §1.1 no longer declares work_credit")
	}
	end := strings.Index(doc[start:], "WITHOUT ROWID;")
	if end < 0 {
		t.Fatal("schema.md's work_credit block is not terminated")
	}
	block := doc[start : start+end]
	// Narrow to the CHECK's own parentheses. The surrounding DDL comments name
	// 'artist' and 'person' — the two KINDS creator_work_id may point at — and a
	// vocabulary check that swept those up would be comparing the wrong list.
	block = checkList(t, block)

	var ddl string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_schema WHERE name = 'work_credit'`).Scan(&ddl); err != nil {
		t.Fatalf("read the shipped DDL: %v", err)
	}

	want := []string{
		"primary", "featured", "composer", "conductor", "performer", "remixer", "producer",
		"author", "translator", "editor", "illustrator", "narrator",
		"writer", "penciller", "inker", "colorist", "letterer", "cover_artist",
	}
	sort.Strings(want)

	if got := quotedRoles(t, block); !slices.Equal(got, want) {
		t.Errorf("schema.md §1.1's work_credit vocabulary = %v\nwant %v", got, want)
	}
	// BOTH DIRECTIONS, over the shipped CHECK: a role the database accepts and
	// the reference does not list is an undocumented vocabulary, and a role the
	// reference lists and the database rejects is a role an adapter can be
	// written against and cannot store.
	shipped := checkList(t, ddl)
	// DB-01, from the vocabulary side. A bare NULL inside the IN list is not a
	// quoted literal, so the comparison below cannot see it — and it is the one
	// edit that makes this CHECK accept everything while still listing all
	// eighteen roles. Measured: writing `IN (NULL, 'primary', …)` leaves the
	// list comparison green.
	if strings.Contains(strings.ToUpper(shipped), "NULL") {
		t.Error("work_credit.role's CHECK contains NULL. `x IN (NULL, …)` evaluates to NULL " +
			"for every non-matching value and a CHECK passes on NULL, so the constraint " +
			"accepts anything at all. This is DB-01, which shipped twice already. `role` is " +
			"NOT NULL, so the bare IN list is correct and complete.")
	}
	if got := quotedRoles(t, shipped); !slices.Equal(got, want) {
		t.Errorf("the shipped work_credit.role CHECK = %v\nwant %v\n"+
			"docs/reference/schema.md §1.1 is authoritative. 0007 is merged, so a genuinely "+
			"wanted role is a new migration and a 12-step rebuild — SQLite cannot ALTER a CHECK.",
			got, want)
	}
}

// checkList narrows a DDL fragment to the parentheses of role's CHECK.
func checkList(t *testing.T, fragment string) string {
	t.Helper()
	const marker = "CHECK (role IN ("
	i := strings.Index(fragment, marker)
	if i < 0 {
		t.Fatal("no `CHECK (role IN (` in the fragment — work_credit.role lost its CHECK, " +
			"which is the only enforcement this vocabulary has")
	}
	rest := fragment[i+len(marker):]
	j := strings.Index(rest, "))")
	if j < 0 {
		t.Fatal("work_credit.role's CHECK is not terminated")
	}
	return rest[:j]
}

// quotedRoles pulls the single-quoted literals out of a DDL fragment, sorted.
func quotedRoles(t *testing.T, fragment string) []string {
	t.Helper()
	var out []string
	for i := 0; i < len(fragment); {
		start := strings.IndexByte(fragment[i:], '\'')
		if start < 0 {
			break
		}
		start += i
		end := strings.IndexByte(fragment[start+1:], '\'')
		if end < 0 {
			break
		}
		end += start + 1
		out = append(out, fragment[start+1:end])
		i = end + 1
	}
	sort.Strings(out)
	return out
}

// TestApplyCreditsIsOneTransaction pins the batching contract: a failure inside
// the batch leaves NOTHING, so a retry cannot double-write.
func TestApplyCreditsIsOneTransaction(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	// A cancelled context fails the write mid-batch.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.ApplyCredits(ctx, inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{credit("author", "Susanna Clarke", 0)}},
	}, testNow); err == nil {
		t.Fatal("a cancelled context did not fail the write")
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'person'`); n != 0 {
		t.Errorf("%d person work(s) survived a failed batch", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_credit`); n != 0 {
		t.Errorf("%d credit(s) survived a failed batch", n)
	}
}

// TestSoftDeletedPeopleAreNotResurrected pins the tombstone call the lookup
// makes, which is the same one the work path makes.
func TestSoftDeletedPeopleAreNotResurrected(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{credit("author", "Susanna Clarke", 0)}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE work SET deleted_at = ? WHERE kind = 'person'`, FormatTime(testNow))
		return err
	}); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "11", Credits: []Credit{credit("author", "Susanna Clarke", 0)}},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if res.PeopleCreated != 1 {
		t.Errorf("a soft-deleted person was reused (%d created); the lookup excludes "+
			"deleted_at IS NOT NULL on purpose, so a tombstoned row is not resurrected "+
			"by a credit", res.PeopleCreated)
	}
}
