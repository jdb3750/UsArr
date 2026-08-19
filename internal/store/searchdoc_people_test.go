package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// The `search_fts.people` guards.
//
// WHAT THESE TESTS CAN AND CANNOT LOOK AT, stated once because it shapes every
// assertion below: search_fts is CONTENTLESS. `SELECT people FROM search_fts`
// returns NULL for every row (measured), so no test here can read the column it
// is about. The only honest witness is the thing the column exists for — a
// column-scoped MATCH — so these tests assert on RETRIEVAL rather than on
// stored text. That is also the stronger assertion: a column written with the
// right bytes into the wrong fts5 column would pass a read-back check and fail
// these.

// worksMatching runs one fts5 query and returns the TITLES of the works whose
// documents matched, sorted.
//
// It goes through search_doc rather than reading search_fts directly for the
// reason above, and it returns titles rather than a count so a failure message
// can say WHAT matched instead of how many.
func worksMatching(t *testing.T, s *Store, match string) []string {
	t.Helper()
	rows, err := s.db.Read().QueryContext(t.Context(), `
		SELECT w.title FROM work w
		  JOIN search_doc d ON d.work_id = w.id
		 WHERE d.rowid IN (SELECT rowid FROM search_fts WHERE search_fts MATCH ?)
		 ORDER BY w.title`, match)
	if err != nil {
		t.Fatalf("MATCH %q: %v", match, err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("MATCH %q: scan: %v", match, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("MATCH %q: %v", match, err)
	}
	return out
}

// clarkeOnPiranesi credits one author on the first fixture book and returns the
// result, so each test below starts from the same populated state.
func clarkeOnPiranesi(t *testing.T, s *Store, inst int64) CreditResult {
	t.Helper()
	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Susanna Clarke", 0),
		}},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	return res
}

// ─── The column is filled at all ────────────────────────────────────────────

// TestAWorkIsFoundByItsAuthorsName is the whole point of the column.
//
// docs/reference/schema.md §1.1 and docs/reference/search.md §2 both carry the
// same ⚠️ — *"find everything by this author" is unanswered in v0.1* — because
// `person` is excluded from the corpus and the document builder wrote
// `search_fts.people` as the empty string. This is the assertion that it no
// longer is.
//
// THE QUERY TERM APPEARS NOWHERE IN THE WORK'S OWN TEXT, and the test proves
// that rather than assuming it: without that check a match could be coming from
// the title column and the `people` column could still be empty. "Susanna
// Clarke" is not a substring of "Piranesi", so the only column that can produce
// this hit is the one under test.
func TestAWorkIsFoundByItsAuthorsName(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)
	clarkeOnPiranesi(t, s, inst)

	// The fixture's own precondition: the name is not in the title, so a hit
	// cannot be the title column answering.
	for _, part := range []string{"Susanna", "Clarke"} {
		if strings.Contains(strings.ToLower("Piranesi"), strings.ToLower(part)) {
			t.Fatalf("the fixture is broken: %q appears in the title, so a match "+
				"would prove nothing about the people column", part)
		}
	}
	if got := worksMatching(t, s, `title : Clarke`); len(got) != 0 {
		t.Fatalf("the fixture is broken: %q matched the TITLE column, so a people "+
			"match below would prove nothing", got)
	}

	got := worksMatching(t, s, `people : Clarke`)
	if len(got) != 1 || got[0] != "Piranesi" {
		t.Errorf("searching the people column for the author returned %q, want "+
			"[Piranesi]. schema.md §1.1's owed enrichment is the credited names "+
			"reaching search_fts.people; an empty result means the column is still "+
			"being written blank and no user can find a book by its author", got)
	}
	// The name must not have leaked onto the OTHER book, which shares nothing
	// with this one but the library.
	if got := worksMatching(t, s, `people : Clarke`); len(got) > 1 {
		t.Errorf("%d works matched the author, want exactly the one credited: %q", len(got), got)
	}
	assertCorpusInvariants(t, s, 2)
}

// TestBothTheGivenNameAndTheSurnameReachTheColumn pins that the whole name is
// tokenised, not just whatever the join happened to put first.
func TestBothTheGivenNameAndTheSurnameReachTheColumn(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)
	clarkeOnPiranesi(t, s, inst)

	for _, term := range []string{`people : Susanna`, `people : Clarke`, `people : "Susanna Clarke"`} {
		if got := worksMatching(t, s, term); len(got) != 1 {
			t.Errorf("MATCH %q returned %q, want the one credited work", term, got)
		}
	}
}

// TestEveryRoleReachesTheColumnNotJustAuthorship is the "all eighteen roles"
// decision, asserted rather than asserted-in-a-comment.
//
// The alternative that was refused — indexing authorship-ish roles only — would
// leave a comics library's pencillers, letterers and cover artists unsearchable
// while the column looked populated. The failure mode is silent, so it needs a
// witness: a cover artist and a letterer, neither of whom wrote anything.
func TestEveryRoleReachesTheColumnNotJustAuthorship(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("cover_artist", "Vince Haig", 0),
			credit("letterer", "Todd Klein", 1),
		}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}

	for _, term := range []string{`people : Haig`, `people : Klein`} {
		if got := worksMatching(t, s, term); len(got) != 1 || got[0] != "Piranesi" {
			t.Errorf("MATCH %q returned %q, want [Piranesi]. A user who types a cover "+
				"artist's or a letterer's name wants the book they worked on, and no "+
				"other screen answers them", term, got)
		}
	}
}

// TestThePrintedVariantIsIndexedBesideTheCanonicalName covers credited_as.
//
// Both spellings are emitted when they differ: the person work's title is the
// first spelling any source used, credited_as is what THIS release printed. A
// user searching either is asking the same question.
func TestThePrintedVariantIsIndexedBesideTheCanonicalName(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	c := credit("author", "Gordon Sumner", 0)
	c.CreditedAs = "Sting"
	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{c}},
	}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}

	for _, term := range []string{`people : Sumner`, `people : Sting`} {
		if got := worksMatching(t, s, term); len(got) != 1 || got[0] != "Piranesi" {
			t.Errorf("MATCH %q returned %q, want [Piranesi] — both the canonical name "+
				"and the printed variant must be searchable", term, got)
		}
	}
}

// ─── The column stays current ───────────────────────────────────────────────

// TestAChangedCreditUpdatesTheDocument is the step-4 guard.
//
// It is the assertion that separates "the column is filled once at import" from
// "the column is kept current". Credits are written in a phase-B pass AFTER the
// item pass, so on a first import the item pass sees no credit at all and
// everything this column is worth depends on the credit pass going back and
// rebuilding the document in the same transaction.
//
// It asserts BOTH directions — the new name arrives AND the old one leaves —
// because an append-only rebuild would pass a one-directional check while
// leaving an author the operator deleted in Kavita searchable forever, which is
// the same failure the wholesale credit replace exists to prevent.
func TestAChangedCreditUpdatesTheDocument(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)
	clarkeOnPiranesi(t, s, inst)

	if got := worksMatching(t, s, `people : Clarke`); len(got) != 1 {
		t.Fatalf("precondition: the first credit did not reach the document (%q)", got)
	}

	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "10", Credits: []Credit{
			credit("author", "Ursula Le Guin", 0),
		}},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if res.DocsRebuilt != 1 {
		t.Errorf("DocsRebuilt = %d, want 1 — the credited name changed, so the search "+
			"document owed a rebuild in the same transaction", res.DocsRebuilt)
	}
	if got := worksMatching(t, s, `people : "Le Guin"`); len(got) != 1 || got[0] != "Piranesi" {
		t.Errorf("the NEW author matched %q, want [Piranesi]: a credit change that does "+
			"not reach the document leaves the column stale until the next full import", got)
	}
	if got := worksMatching(t, s, `people : Clarke`); len(got) != 0 {
		t.Errorf("the REMOVED author still matches %q. The credit rows were replaced "+
			"wholesale, so a document that still carries the old name is a rebuild that "+
			"appended instead of replacing", got)
	}
	assertCorpusInvariants(t, s, 2)
}

// TestARepeatCreditImportRebuildsNothing is the cost half of the same decision.
//
// schema.md §1.1 left this enrichment owed because folding names in means "a
// second FTS write per item, in the transaction the 100 ms batch window exists
// to keep short". The answer is that it is a second write only when a name
// actually changed: step 3 deletes and re-inserts every credit row
// unconditionally, so a steady-state re-import writes identical rows, and
// comparing the RENDERED NAME LIST rather than the rows is what makes the
// difference visible.
//
// ⚠️ DocsRebuilt IS THE ONLY WITNESS AVAILABLE, and that is not a shortcut. The
// FTS rows are replaced in place, so no query outside the store can tell a
// rebuild that produced identical text from no rebuild at all. The counter is
// therefore load-bearing rather than decorative, which is why it is a field on
// CreditResult. This test pairs it with a retrieval check so that a step 4 which
// never fires at all cannot pass here by doing nothing.
func TestARepeatCreditImportRebuildsNothing(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	first := clarkeOnPiranesi(t, s, inst)
	if first.DocsRebuilt != 1 {
		t.Fatalf("the FIRST credit import rebuilt %d docs, want 1 — nothing below "+
			"means anything if the document was never built from credits at all",
			first.DocsRebuilt)
	}

	second := clarkeOnPiranesi(t, s, inst)
	if second.CreditsWritten != 1 {
		t.Fatalf("the second import wrote %d credits, want 1 — this test is about a "+
			"re-import that DID rewrite the credit rows", second.CreditsWritten)
	}
	if second.DocsRebuilt != 0 {
		t.Errorf("a re-import of identical credits rebuilt %d search documents, want 0. "+
			"Step 3 replaces the credit rows unconditionally, so a rebuild keyed off the "+
			"rows rather than off the rendered names costs every credited item a second "+
			"FTS write on every import", second.DocsRebuilt)
	}
	// …and the document is still correct, so "rebuilt nothing" cannot be
	// satisfied by a step 4 that does nothing ever.
	if got := worksMatching(t, s, `people : Clarke`); len(got) != 1 {
		t.Errorf("after the re-import the author matched %q, want [Piranesi]", got)
	}
}

// TestAnUncreditedItemNeverTouchesTheFTSTables pins the "" == "" case.
func TestAnUncreditedItemNeverTouchesTheFTSTables(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	// A credit set with nothing creditable in it: an empty name is dropped
	// before it becomes a credit, so the rendered list stays "".
	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "11", Credits: []Credit{{Role: "author"}}},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if res.DocsRebuilt != 0 {
		t.Errorf("an item with no credited name rebuilt %d documents, want 0", res.DocsRebuilt)
	}
}

// TestAReimportKeepsTheCreditedNamesInTheDocument is the ITEM pass's half.
//
// The item pass runs BEFORE the credit pass and reads `people` out of
// work_credit rather than writing "". That looks pointless — on a first import
// it reads back nothing — and it is not: on every import after the first, the
// PREVIOUS import's credits are still in work_credit when the item pass rebuilds
// the document. Writing "" there would blank the column for the whole window
// between the two passes, so an author search run mid-import would return
// nothing. Since the credit pass then compares equal and skips its own rebuild,
// the blanking would in fact SURVIVE THE IMPORT and last until a credit changed.
func TestAReimportKeepsTheCreditedNamesInTheDocument(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)
	clarkeOnPiranesi(t, s, inst)

	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{{RemoteID: "2", Name: "Ebooks", Kind: "book"}})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{item("10", "2", "book", "Piranesi")}, testNow); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	if got := worksMatching(t, s, `people : Clarke`); len(got) != 1 || got[0] != "Piranesi" {
		t.Errorf("after re-importing the ITEM, the author matched %q, want [Piranesi]. "+
			"The item pass rebuilds the whole document, so a `people` column it writes "+
			"as \"\" is a column blanked until some credit changes — the credit pass that "+
			"follows compares the unchanged names equal and rebuilds nothing", got)
	}
	assertCorpusInvariants(t, s, 2)
}

// ─── Nothing else in the document is collateral damage ──────────────────────

// TestTheCreditPassRebuildKeepsEveryOtherColumn is the drift guard for the
// document having TWO writers.
//
// The item pass builds the document from the CatalogueItem it just wrote; the
// credit pass holds no CatalogueItem and re-derives every field from the
// replica. fts5 refuses to update a subset of a contentless-delete table's
// columns, so the credit pass must supply ALL FIVE — which means every field it
// re-derives wrongly is a field silently blanked the moment a credit changes.
//
// It asserts through retrieval on each column in turn, because that is the only
// thing a contentless table exposes, and it uses terms that appear in ONE column
// each so a hit cannot come from a neighbour.
func TestTheCreditPassRebuildKeepsEveryOtherColumn(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "kavita-drift")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID,
		[]CatalogueContainer{{RemoteID: "2", Name: "Ebooks", Kind: "book"}})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

	it := item("10", "2", "book", "Piranesi")
	it.OriginalTitle = "Piranesium"
	it.Overview = "A man lives alone in a labyrinth of statues and tides."
	it.AltTitles = []AltTitle{
		{Title: "Piranesi Deluxe", Normalized: "piranesi deluxe", Kind: "alias"},
		{Title: "ピラネージ", Normalized: "ピラネージ", Kind: "translated"},
	}
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
		[]CatalogueItem{it}, testNow); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Every column is retrievable BEFORE the credit pass touches anything, so a
	// failure below is the rebuild losing it rather than the import never
	// writing it.
	cols := map[string]string{
		"title":          `title : Piranesi`,
		"original_title": `original_title : Piranesium`,
		"alt_titles":     `alt_titles : Deluxe`,
		"overview":       `overview : labyrinth`,
	}
	for col, q := range cols {
		if got := worksMatching(t, s, q); len(got) != 1 {
			t.Fatalf("precondition: the %s column did not match %q before the credit "+
				"pass ran (%q) — the drift assertion below would be vacuous", col, q, got)
		}
	}

	clarkeOnPiranesi(t, s, inst)

	if got := worksMatching(t, s, `people : Clarke`); len(got) != 1 {
		t.Fatalf("precondition: the credit pass did not rebuild the document (%q)", got)
	}
	for col, q := range cols {
		if got := worksMatching(t, s, q); len(got) != 1 {
			t.Errorf("after the credit pass rebuilt the document, the %s column no longer "+
				"matches %q (%q). The rebuild re-derives every column from the replica and "+
				"must supply all five; a field it derives wrongly is a field silently lost "+
				"the first time a credit changes", col, q, got)
		}
	}
	assertCorpusInvariants(t, s, 1)
}

// ─── The writer-side exclusion ──────────────────────────────────────────────

// TestTheDocumentWriterRefusesAnExcludedKind fires the corpusExcludedKinds
// refusal.
//
// search.md §2 excludes five kinds from the corpus, and until now the rule lived
// only in a CI query (TestPeopleNeverEnterTheSearchCorpus). That query can only
// report a corpus that has ALREADY been corrupted, and the FTS tables carry no
// foreign key, so nothing cascades the bad rows away afterwards. The refusal
// moves the rule to the writer.
//
// It calls the writer DIRECTLY, and that is the honest option rather than a
// contrived one: today's only adapter maps containers to 'comic' and 'book', so
// no shipped caller can reach the branch, and the caller that will — the phase-B
// comic_issue walk — is a later commit. Deleting the branch because nothing
// reaches it yet would delete the rule's only enforcement at the moment it stops
// being unreachable.
func TestTheDocumentWriterRefusesAnExcludedKind(t *testing.T) {
	s := newTestStore(t)
	inst, bookA, _ := creditFixture(t, s)
	_ = inst

	before, _, _ := corpusCounts(t, s)

	for _, kind := range []string{"season", "episode", "track", "comic_issue", "person"} {
		err := s.db.Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			return writeSearchDoc(ctx, tx, bookA, searchDocText{
				kind: kind, normTitle: "piranesi", title: "Piranesi",
			})
		})
		if err == nil {
			t.Errorf("the document writer accepted kind %q. search.md §2 excludes it: "+
				"the doc would either swamp the corpus with child rows or be a result "+
				"row with nowhere to land, and its search_doc_library row would put it "+
				"in the user's library grid as an item", kind)
			continue
		}
		if !strings.Contains(err.Error(), kind) {
			t.Errorf("the refusal of kind %q does not name the kind: %v", kind, err)
		}
	}

	// The refusals left nothing behind — including no half-written document for
	// the work whose id was reused above.
	if docs, fts, trgm := corpusCounts(t, s); docs != before || fts != before || trgm != before {
		t.Errorf("the corpus is now doc %d / fts %d / trgm %d, want %d of each: a refused "+
			"kind must leave no rows at all", docs, fts, trgm, before)
	}
	assertCorpusInvariants(t, s, before)
}
