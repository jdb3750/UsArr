package libsync

import (
	"context"
	"testing"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// The credit and file passes of the BookOrbit adapter.
//
// THE ONE ASSERTION THIS FILE EXISTS FOR is that an audiobook comes out
// classified as an audiobook. Everything else here guards a way of getting that
// right for the wrong reason — a format table that agrees with BookOrbit by
// accident, a cover counted as content, a year nobody claimed.

// walkedSource runs the item walk over one hand-built library, which is what
// fills the card buffer the two passes read. NOTHING ELSE FILLS IT, so a test
// that skipped this step would be testing an empty map.
func walkedSource(t *testing.T, books ...bookorbit.Book) *BookOrbitSource {
	t.Helper()
	src := NewBookOrbitSource(&fakeBookOrbitReader{
		libs:  []bookorbit.Library{{ID: 1, Name: "Books"}},
		books: map[int64][]bookorbit.Book{1: books},
	})
	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if _, err := src.StreamItems(t.Context(), func(store.CatalogueItem) error { return nil }); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	return src
}

// fileSetsOf runs the file pass over the books the walk mapped, in the shape the
// importer runs it: one FileRequest per imported item, carrying the
// remote_subtype the item pass wrote.
func fileSetsOf(t *testing.T, src *BookOrbitSource, reqs ...FileRequest) []store.FileSet {
	t.Helper()
	var out []store.FileSet
	n, failed, err := src.StreamFiles(t.Context(), reqs, func(s store.FileSet) error {
		out = append(out, s)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamFiles: %v", err)
	}
	if len(failed) != 0 {
		t.Errorf("StreamFiles reported %d failures; this pass issues no request and "+
			"therefore has nothing that can fail to be read: %+v", len(failed), failed)
	}
	if n != len(out) {
		t.Errorf("StreamFiles counted %d and delivered %d; the count is the hand-overs", n, len(out))
	}
	return out
}

func fileReq(id, subtype string) FileRequest {
	return FileRequest{RemoteKind: bookRemoteKind, RemoteID: id, RemoteSubtype: subtype}
}

// ─── the assertion this slice owes ───────────────────────────────────────────

// TestAnAudiobookGetsAnAudiobookEditionAndAnEbookDoesNot is the bug this pass
// fixes, at the grain the adapter owns.
//
// Before FileSource existed, BOTH of these books produced no edition at all, and
// store.mediaTypeOf's documented answer for a work with no editions is Ebooks —
// so an m4b imported from BookOrbit rendered as an Ebook and /library/audiobooks
// was empty. The two arms are asserted together because either alone passes for
// a build that hardcodes one answer.
func TestAnAudiobookGetsAnAudiobookEditionAndAnEbookDoesNot(t *testing.T) {
	src := walkedSource(t,
		proseBook(101, "The Hobbit"), // epub
		withFormat(proseBook(201, "Project Hail Mary"), "m4b"),
	)
	sets := fileSetsOf(t, src, fileReq("101", "epub"), fileReq("201", "m4b"))
	if len(sets) != 2 {
		t.Fatalf("delivered %d sets, want 2", len(sets))
	}

	if got := sets[0].EditionFormat; !got.Valid || got.String != "ebook" {
		t.Errorf("the epub's edition.format = %+v, want 'ebook'", got)
	}
	if got := sets[1].EditionFormat; !got.Valid || got.String != "audiobook" {
		t.Errorf("the m4b's edition.format = %+v, want 'audiobook'. This is THE row "+
			"store.mediaTypeOf's MIN(format = 'audiobook') and browseAudiobookPredicate "+
			"both key on; without it the book renders as an Ebook", got)
	}
}

// TestEveryBookFormatBookOrbitCanReportIsMappedOrRefused walks BookOrbit's whole
// declared format vocabulary rather than the two members the case above needs.
//
// The list is DEFAULT_FORMAT_PRIORITY (packages/types/src/library.ts:8-24),
// which is BOOK_FORMATS — `export const BOOK_FORMATS = DEFAULT_FORMAT_PRIORITY`
// (packages/types/src/book.ts:12). A format BookOrbit adds later arrives here as
// an unrecognised token and lands on the 'ebook' default branch exactly as
// getBookMediaKind puts it, so the test pins the CURRENT vocabulary rather than
// claiming to be future-proof.
func TestEveryBookFormatBookOrbitCanReportIsMappedOrRefused(t *testing.T) {
	for _, tc := range []struct {
		format string
		want   string // "" means NULL — no medium claimed
		why    string
	}{
		{"epub", "ebook", "the ordinary case"},
		{"kepub", "ebook", "Kobo's EPUB; still an ebook"},
		{"mobi", "ebook", ""},
		{"azw3", "ebook", ""},
		{"azw", "ebook", ""},
		{"fb2", "ebook", ""},
		{"pdf", "pdf", "edition.format has a pdf member and BookOrbit's kind vocabulary does not"},
		{"m4b", "audiobook", ""},
		{"mp3", "audiobook", ""},
		{"m4a", "audiobook", ""},
		{"opus", "audiobook", ""},
		{"ogg", "audiobook", ""},
		{"flac", "audiobook", "an audio format on a BOOK is an audiobook, not a music edition"},
		{"cbz", "", "a comic never reaches this pass; claiming 'cbz' for a 'book' work " +
			"would assert a medium the import path refused"},
		{"cbr", "", ""},
		{"cb7", "", ""},
		{"", "", "no format token: BookOrbit's own 'unknown', and unknown claims nothing"},
		{"   ", "", "whitespace normalises to empty"},
		{"M4B", "audiobook", "the source lowercases before it classifies, so this must too"},
		{"wibble", "ebook", "getBookMediaKind's DEFAULT branch is ebook, and UsArr must not " +
			"invent a second opinion about a token its source already classified"},
	} {
		got := bookOrbitEditionFormat(tc.format)
		if tc.want == "" {
			if got.Valid {
				t.Errorf("format %q → %q, want NULL (%s)", tc.format, got.String, tc.why)
			}
			continue
		}
		if !got.Valid || got.String != tc.want {
			t.Errorf("format %q → %+v, want %q (%s)", tc.format, got, tc.want, tc.why)
		}
	}
}

// TestAnEditionFormatIsAlwaysAMemberOfTheColumnsCheck is the guard that stops
// this mapping inventing a value the database will reject.
//
// edition.format's CHECK is in migration 0005 and it is the vocabulary's single
// definition; a Go-side copy would be a second one to drift. The copy here is
// deliberately the WHOLE list, so that a mapping which started returning, say,
// 'audio' fails on the member rather than on a foreign-key error three layers
// down at import time.
func TestAnEditionFormatIsAlwaysAMemberOfTheColumnsCheck(t *testing.T) {
	members := map[string]bool{
		"print": true, "ebook": true, "audiobook": true, "comic": true,
		"cbz": true, "cbr": true, "pdf": true,
		"bluray": true, "web": true, "remux": true, "dvd": true,
		"vinyl": true, "cd": true, "cassette": true, "digital": true,
		"flac": true, "lossy": true,
	}
	for _, f := range []string{
		"epub", "kepub", "pdf", "cbz", "cbr", "cb7", "mobi", "azw3", "azw", "fb2",
		"m4b", "mp3", "m4a", "opus", "ogg", "flac", "", "wibble",
	} {
		got := bookOrbitEditionFormat(f)
		if got.Valid && !members[got.String] {
			t.Errorf("format %q → %q, which edition.format's CHECK does not admit", f, got.String)
		}
	}
}

// ─── the files themselves ────────────────────────────────────────────────────

// TestOnlyContentFilesBecomeMediaFileRows is the have_count guard.
//
// work.have_count is COUNT(media_file) (internal/store/rollup.go), and a
// BookOrbit book's files[] routinely carries a cover and an .opf beside its
// content — `book_files_role_chk` is
// `role in ('content','cover','metadata','supplement')` (db/schema/books.ts:92).
// Counting those would render "4 files held" for a one-file book.
func TestOnlyContentFilesBecomeMediaFileRows(t *testing.T) {
	b := proseBook(101, "The Hobbit")
	b.Files = []bookorbit.BookFile{
		{ID: 1011, Format: "epub", Role: "primary", SizeBytes: 1234567},
		{ID: 1012, Format: "epub", Role: "content", SizeBytes: 42},
		{ID: 1013, Format: "jpg", Role: "cover", SizeBytes: 9999},
		{ID: 1014, Format: "opf", Role: "metadata", SizeBytes: 8888},
		{ID: 1015, Format: "txt", Role: "supplement", SizeBytes: 7777},
	}
	sets := fileSetsOf(t, walkedSource(t, b), fileReq("101", "epub"))
	if len(sets) != 1 {
		t.Fatalf("delivered %d sets, want 1", len(sets))
	}
	got := sets[0].Files
	if len(got) != 2 {
		t.Fatalf("media_file rows = %d, want 2 — the primary and the content file; "+
			"the cover, the .opf and the supplement are not files a reader holds: %+v", len(got), got)
	}
	if got[0].Path != "bookorbit:bookfile:1011" || got[1].Path != "bookorbit:bookfile:1012" {
		t.Errorf("paths = %q, %q; want the opaque surrogates", got[0].Path, got[1].Path)
	}
	if got[0].SizeBytes != 1234567 {
		t.Errorf("size_bytes = %d, want 1234567 — the upstream's own number", got[0].SizeBytes)
	}
	if got[0].RemoteFileID != "1011" {
		t.Errorf("remote_file_id = %q, want \"1011\"", got[0].RemoteFileID)
	}
	// date_added is NOT written: BookFileRef carries no timestamp, and the
	// book's own addedAt is not the file's. A zero time is stored as NULL.
	if !got[0].DateAdded.IsZero() {
		t.Errorf("date_added = %v, want the zero time — the card reports no per-file "+
			"timestamp, and substituting the book's would be a date nobody stated", got[0].DateAdded)
	}
}

// TestAMediaFilePathNeverContainsASeparator fires internal/store's own guard
// against this adapter's surrogate.
//
// validateReplicaPath rejects a path separator, and BookOrbit is the source that
// makes that guard matter: `bookFiles.absolutePath` is a NOT NULL varchar(4096)
// upstream, so a future slice reading BookDetail has a real host path in hand.
func TestAMediaFilePathNeverContainsASeparator(t *testing.T) {
	for _, id := range []int64{1, 1011, 9007199254740991} {
		got := bookOrbitFilePathSurrogate(id)
		for _, sep := range []string{"/", `\`} {
			if len(got) == 0 || contains(got, sep) {
				t.Errorf("surrogate %q contains %q; internal/store's validateReplicaPath "+
					"rejects it and the row would be counted as FilesRejected", got, sep)
			}
		}
	}
	if got := bookOrbitFilePathSurrogate(1011); got != "bookorbit:bookfile:1011" {
		t.Errorf("surrogate = %q, want bookorbit:bookfile:1011", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestABookWithNoContentFilesDeliversAnEmptySetRatherThanBeingSkipped pins the
// rule files.go states with teeth: an empty set is what REMOVES the rows of a
// book whose bytes went away, and skipping it is how a deleted file counts
// forever.
func TestABookWithNoContentFilesDeliversAnEmptySetRatherThanBeingSkipped(t *testing.T) {
	b := proseBook(101, "Cover Only")
	b.Files = []bookorbit.BookFile{{ID: 1013, Format: "jpg", Role: "cover"}}
	sets := fileSetsOf(t, walkedSource(t, b), fileReq("101", "jpg"))
	if len(sets) != 1 {
		t.Fatalf("delivered %d sets, want 1 — an empty set is a statement, not a skip", len(sets))
	}
	if len(sets[0].Files) != 0 {
		t.Errorf("files = %+v, want none", sets[0].Files)
	}
	// And with no files, store gives the item no edition at all, so the book
	// stays Ebooks — mediaTypeOf's documented answer for a work with no
	// editions, and the honest one for a book with no bytes.
	if sets[0].EditionFormat.Valid && sets[0].EditionFormat.String == "audiobook" {
		t.Error("a book with no content files claimed the audiobook medium")
	}
}

// TestOneFileReportedTwiceBecomesOneRow guards the set the store reconciles
// against and the rows actually written from disagreeing.
func TestOneFileReportedTwiceBecomesOneRow(t *testing.T) {
	b := proseBook(101, "Doubled")
	b.Files = []bookorbit.BookFile{
		{ID: 1011, Format: "epub", Role: "primary"},
		{ID: 1011, Format: "epub", Role: "content"},
		{ID: 0, Format: "epub", Role: "content"}, // .NET/serial zero value, not an id
	}
	sets := fileSetsOf(t, walkedSource(t, b), fileReq("101", "epub"))
	if n := len(sets[0].Files); n != 1 {
		t.Errorf("media_file rows = %d, want 1 — a repeated id is one file and 0 is not an id", n)
	}
}

// ─── the credits ─────────────────────────────────────────────────────────────

func creditSetsOf(t *testing.T, src *BookOrbitSource, reqs ...CreditRequest) []store.CreditSet {
	t.Helper()
	var out []store.CreditSet
	n, err := src.StreamCredits(t.Context(), reqs, func(s store.CreditSet) error {
		out = append(out, s)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamCredits: %v", err)
	}
	if n != len(out) {
		t.Errorf("StreamCredits counted %d and delivered %d", n, len(out))
	}
	return out
}

func creditReq(id string) CreditRequest {
	return CreditRequest{RemoteKind: bookRemoteKind, RemoteID: id, Kind: bookKind}
}

// TestAuthorsAndNarratorsBecomeTheirOwnRoles is the mapping, including the one
// that had no writer in the schema until now.
func TestAuthorsAndNarratorsBecomeTheirOwnRoles(t *testing.T) {
	b := withFormat(proseBook(201, "Project Hail Mary"), "m4b")
	b.Authors = []string{"Andy Weir"}
	b.Narrators = []string{"Ray Porter", "Second Voice"}

	sets := creditSetsOf(t, walkedSource(t, b), creditReq("201"))
	if len(sets) != 1 {
		t.Fatalf("delivered %d sets, want 1", len(sets))
	}
	got := sets[0].Credits
	if len(got) != 3 {
		t.Fatalf("credits = %d, want 3: %+v", len(got), got)
	}

	if got[0].Role != "author" || got[0].Name != "Andy Weir" {
		t.Errorf("credit 0 = %+v, want the author. 'author' and not 'writer': the two are "+
			"separate CHECK members split by MEDIUM, and 'writer' is the comics credit", got[0])
	}
	if got[0].NormalizedName != NormalizeTitle("Andy Weir") {
		t.Errorf("normalized name = %q; it is the dedupe key personWorkID uses", got[0].NormalizedName)
	}
	for i, want := range []string{"Ray Porter", "Second Voice"} {
		c := got[i+1]
		if c.Role != "narrator" || c.Name != want {
			t.Errorf("credit %d = %+v, want narrator %q", i+1, c, want)
		}
		// POSITION IS PER ROLE. Both narrators would collide on work_credit's
		// PRIMARY KEY (work_id, role, position, creator_work_id) if the counter
		// were shared with the author's or not incremented.
		if c.Position != i {
			t.Errorf("narrator %q has position %d, want %d — position is per role and "+
				"is part of work_credit's PRIMARY KEY", want, c.Position, i)
		}
	}
	if got[0].Position != 0 {
		t.Errorf("the author's position = %d, want 0 — the counter restarts per role", got[0].Position)
	}
}

// TestEveryBookOrbitPersonArrayIsAccountedFor is the check bookOrbitCreditRoles
// names in its own doc comment: the table is the mapping, and a third array
// appearing on BookCard must not be silently dropped.
//
// It is the BookOrbit analogue of TestEveryKavitaPersonRoleIsAccountedFor, and
// it is deliberately narrow: BookCard has exactly two `string[]` fields naming
// people, and the reason there is no third to DROP is in the table's comment.
func TestEveryBookOrbitPersonArrayIsAccountedFor(t *testing.T) {
	want := map[string]string{"authors": "author", "narrators": "narrator"}
	if len(bookOrbitCreditRoles) != len(want) {
		t.Fatalf("the table has %d entries and BookCard names %d person arrays; "+
			"a new array must be mapped or dropped WITH A REASON, never left out",
			len(bookOrbitCreditRoles), len(want))
	}
	for _, cr := range bookOrbitCreditRoles {
		w, ok := want[cr.name]
		if !ok {
			t.Errorf("the table maps %q, which is not a BookCard person array", cr.name)
			continue
		}
		if cr.role != w {
			t.Errorf("%q → %q, want %q", cr.name, cr.role, w)
		}
	}
}

// TestANameThatSaysNothingIsNotStored keeps this adapter's rule identical to
// Kavita's. A name one source stored and the other dropped would be a 'person'
// work only one of them could ever match.
func TestANameThatSaysNothingIsNotStored(t *testing.T) {
	b := proseBook(101, "Anon")
	b.Authors = []string{"Real Name", "   ", "", "..."}

	got := creditSetsOf(t, walkedSource(t, b), creditReq("101"))[0].Credits
	if len(got) != 1 || got[0].Name != "Real Name" {
		t.Fatalf("credits = %+v, want only Real Name — blank names have nothing to dedupe "+
			"against and punctuation normalises to nothing", got)
	}
	if got[0].Position != 0 {
		t.Errorf("position = %d, want 0 — a dropped name must not leave a gap in the "+
			"billing order it was never part of", got[0].Position)
	}
}

// TestABookWithNoPeopleStillDeliversItsSet is credits.go's rule: an empty set is
// what removes credits deleted upstream.
func TestABookWithNoPeopleStillDeliversItsSet(t *testing.T) {
	sets := creditSetsOf(t, walkedSource(t, proseBook(101, "Anonymous")), creditReq("101"))
	if len(sets) != 1 {
		t.Fatalf("delivered %d sets, want 1", len(sets))
	}
	if len(sets[0].Credits) != 0 {
		t.Errorf("credits = %+v, want none", sets[0].Credits)
	}
}

// ─── the year, which rides the credit set ────────────────────────────────────

// TestTheYearComesFromTheCardAndAnImpossibleOneDoesNot pins UsArr's band to
// BookOrbit's own.
//
// The band is `book_metadata_published_year_range_chk`:
// `published_year is null or (published_year >= 1000 and published_year <= 2200)`
// (server/src/db/schema/metadata.ts:114), with @Min(1000) @Max(2200) on the two
// write DTOs. A value outside it cannot have come from a BookOrbit storing what
// it says it stores, and it is refused rather than clamped: clamping would put
// 2200 in work.year for a book that said 9999, which the Home screen's Year
// column renders as a year somebody claimed.
func TestTheYearComesFromTheCardAndAnImpossibleOneDoesNot(t *testing.T) {
	for _, tc := range []struct {
		year  int64
		valid bool
	}{
		{1937, true},
		{1000, true},  // the floor itself is admitted
		{2200, true},  // and the ceiling
		{999, false},  // one below the floor
		{2201, false}, // one above the ceiling
		{0, false},    // how "publishedYear: null" arrives
		{-5, false},
	} {
		b := proseBook(101, "Dated")
		b.PublishedYear = tc.year
		got := creditSetsOf(t, walkedSource(t, b), creditReq("101"))[0].Year
		if got.Valid != tc.valid {
			t.Errorf("publishedYear %d → %+v, want valid = %v", tc.year, got, tc.valid)
			continue
		}
		if tc.valid && got.Int64 != tc.year {
			t.Errorf("publishedYear %d → %d", tc.year, got.Int64)
		}
	}
}

// TestTheCreditPassMakesNoStatusOrTotalClaim guards two fields that would be
// wrong rather than merely absent.
//
// CreditSet.Status and TotalDeclared exist for Kavita, whose SERIES have a
// publicationStatus and a totalCount. A BookOrbit book is not a series and has
// neither, so "" and INVALID are the documented "this source makes no
// statement" — and store's rollup gate reads Status to decide whether a work
// gets an availability DENOMINATOR, so a guessed value there would put a
// fraction on a screen.
func TestTheCreditPassMakesNoStatusOrTotalClaim(t *testing.T) {
	set := creditSetsOf(t, walkedSource(t, proseBook(101, "A Book")), creditReq("101"))[0]
	if set.Status != "" {
		t.Errorf("status = %q, want \"\" — BookOrbit states no publication status for a book", set.Status)
	}
	if set.TotalDeclared.Valid {
		t.Errorf("total declared = %+v, want invalid", set.TotalDeclared)
	}
}

// ─── what the two passes do with a request they have no card for ─────────────

// TestARequestTheWalkNeverSawIsSkippedByBothPasses.
//
// A miss is NOT a failure: nothing was attempted and there is no upstream to
// blame. Recording it as a FileWalkFailure would put a `file_walk_failed` row in
// sync_report describing a read that never happened, and the Libraries screen
// would offer a reason for a work that has none.
func TestARequestTheWalkNeverSawIsSkippedByBothPasses(t *testing.T) {
	src := walkedSource(t, proseBook(101, "The Hobbit"))

	sets := fileSetsOf(t, src, fileReq("101", "epub"), fileReq("999", "epub"))
	if len(sets) != 1 || sets[0].RemoteID != "101" {
		t.Errorf("file sets = %+v, want only the book the walk actually read", sets)
	}

	credits := creditSetsOf(t, src, creditReq("101"), creditReq("999"))
	if len(credits) != 1 || credits[0].RemoteID != "101" {
		t.Errorf("credit sets = %+v, want only 101", credits)
	}

	// A request for a remote_kind this adapter never writes is a miss for the
	// same reason, and it must not collide with a book of the same id.
	other := []FileRequest{{RemoteKind: "series", RemoteID: "101", RemoteSubtype: "epub"}}
	if got := fileSetsOf(t, src, other...); len(got) != 0 {
		t.Errorf("a remote_kind this adapter never writes delivered %+v", got)
	}
}

// TestASkippedComicIsNeverBuffered guards the buffer from being the one thing
// that grows without bound over a comics library.
func TestASkippedComicIsNeverBuffered(t *testing.T) {
	src := walkedSource(t,
		proseBook(101, "The Hobbit"),
		withFormat(proseBook(103, "Saga, Vol. 1"), "cbz"),
		withFormat(proseBook(105, "Still Scanning"), ""),
	)
	src.mu.Lock()
	n := len(src.cards)
	src.mu.Unlock()
	if n != 1 {
		t.Errorf("buffered %d cards, want 1 — a comic and an unknown are skipped by the "+
			"item pass, so no work exists for their credits to hang on", n)
	}
}

// TestBothOptionalHalvesAreImplemented is the assertion the importer makes at
// run time, made here at build time.
//
// The importer TYPE-ASSERTS for CreditSource and FileSource and SILENTLY SKIPS
// the whole phase when a source implements neither — which is principle 3 for an
// adapter that has no answer, and which is also exactly how this bug hid: the
// adapter was correct, the phases simply never ran, and no test failed.
func TestBothOptionalHalvesAreImplemented(t *testing.T) {
	var src any = NewBookOrbitSource(&fakeBookOrbitReader{})
	if _, ok := src.(CreditSource); !ok {
		t.Error("BookOrbitSource does not implement CreditSource; the importer would " +
			"skip phase B without reporting anything")
	}
	if _, ok := src.(FileSource); !ok {
		t.Error("BookOrbitSource does not implement FileSource; the importer would skip " +
			"phase C, no edition row would be written, and EVERY book would render as an Ebook")
	}
}

// TestAPassStopsOnACancelledContext. Neither pass issues a request, so this is
// the only condition that ends one early — and both loops are O(imported items).
func TestAPassStopsOnACancelledContext(t *testing.T) {
	src := walkedSource(t, proseBook(101, "The Hobbit"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, _, err := src.StreamFiles(ctx, []FileRequest{fileReq("101", "epub")},
		func(store.FileSet) error { return nil }); err == nil {
		t.Error("StreamFiles ran to completion under a cancelled context")
	}
	if _, err := src.StreamCredits(ctx, []CreditRequest{creditReq("101")},
		func(store.CreditSet) error { return nil }); err == nil {
		t.Error("StreamCredits ran to completion under a cancelled context")
	}
}
