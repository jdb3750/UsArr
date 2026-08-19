package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The BookOrbit catalogue import, end to end through the real binary's startup
// path — slice 1's "a user can now see their books" assertion.
//
// It is the BookOrbit half of TestAddingAKavitaProducesACatalogue and it is
// written for the same reason: internal/libsync and internal/store test the
// importer and the write path against a real migrated database, and NOTHING
// tests the WIRING — whether adding a BookOrbit through the HTTP API actually
// produces a catalogue in a running process. That is three hand-offs
// (registry.entry's on-connect hook, the prober's context, catalogueSource's
// adapter choice), each of which is a place where a correct adapter produces
// nothing at all.
//
// The only double is the BookOrbit. The database is real.

// boLibrary is one entry of GET /api/v1/libraries in the NON-SUPERUSER
// projection — LibraryRepository.findAllForUser's select list, which is what a
// shared viewer account receives and is narrower than the whole row.
func boLibrary(id int, name string, bookCount int) map[string]any {
	return map[string]any{
		"id": id, "name": name, "icon": "book", "displayOrder": id,
		"coverAspectRatio": "book", "scanMode": "manual", "fileRenameEnabled": false,
		"createdAt": "2026-07-01T00:00:00.000Z", "updatedAt": "2026-08-01T00:00:00.000Z",
		"bookCount": bookCount,
		"folders":   []map[string]any{{"id": id, "path": "/mnt/books", "createdAt": "2026-07-01T00:00:00.000Z"}},
	}
}

func TestAddingABookOrbitProducesACatalogue(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{
		boLibrary(1, "Fiction", 4),
		boLibrary(2, "Audio", 1),
	}
	bo.books = map[int][]map[string]any{
		1: {
			// Plain prose, no identifiers matched — the ordinary case.
			boBook(101, "The Hobbit", map[string]any{
				"pageCount": 310, "publishedYear": 1937, "language": "en",
			}),
			// A subtitle, which must reach the FTS document as an alt title.
			boBook(102, "The Making", map[string]any{
				"subtitle": "The Making of the Atomic Bomb",
			}),
			// 🚩 A COMIC. It must be READ, SKIPPED and COUNTED — never written
			// as a 'book' work, and never silently dropped.
			boBook(103, "Saga, Vol. 1", map[string]any{
				"files": []map[string]any{boFile(1031, "cbz", "primary")},
			}),
			// A book whose file has no format at all. BookOrbit's own
			// getBookMediaKind calls that "unknown"; it is counted separately
			// from the comic because the operator's fix is different.
			boBook(104, "Something Processing", map[string]any{
				"status": "processing",
				"files":  []map[string]any{boFile(1041, "", "primary")},
			}),
		},
		2: {
			// An AUDIOBOOK. It is the SAME work.kind as the ebook — 'book' —
			// because ADR-0031 puts the ebook/audiobook distinction on
			// edition.format and work.kind has no 'audiobook' member. A build
			// that gave it its own kind would split one user's library in two
			// and §6.4's cascade forbids those ever merging.
			boBook(201, "Project Hail Mary", map[string]any{
				"files": []map[string]any{
					boFile(2011, "m4b", "primary"),
					// A cover rides the same card. It must NOT become a
					// media_file row: have_count is COUNT(media_file) and a
					// thumbnail counted as a held file is a wrong number on a
					// screen.
					boFile(2012, "jpg", "cover"),
				},
				"hardcoverId": "hc-42",
				// authors and narrators ride the card the walk already fetched,
				// which is what makes the credit pass cost no HTTP at all.
				"authors":       []string{"Andy Weir"},
				"narrators":     []string{"Ray Porter"},
				"publishedYear": 2021,
			}),
		},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)
	waitForImport(t, env, created.ID)

	// ── the containers ──────────────────────────────────────────────────────
	//
	// Two BookOrbit libraries, two UsArr libraries, both of kind 'book'. The
	// kind is a constant here and that is a fact about BookOrbit rather than a
	// shortcut: `libraries` has no type, kind or mediaType column of any sort,
	// so there is nothing to derive one from at the container grain.
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE id <> 0 AND managed_by = 'auto'`); n != 2 {
		t.Errorf("auto libraries = %d, want 2", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE kind = 'book' AND id <> 0`); n != 2 {
		t.Errorf("book libraries = %d, want 2", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library_source WHERE container_kind = 'remote_library'`); n != 2 {
		t.Errorf("library_source rows = %d, want 2", n)
	}

	// ── the items: five books read, THREE written ───────────────────────────
	//
	// The pair is what proves anything. A build that wrote every book would give
	// 5; a build that dropped the audiobook would give 2; a build that mapped
	// the comic into the book cascade would give 4 and be unrecoverable.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind <> 'person'`); n != 3 {
		t.Errorf("catalogue work rows = %d, want 3 — two ebooks and one audiobook; "+
			"the comic and the unclassified file are read and skipped", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind = 'book'`); n != 3 {
		t.Errorf("book works = %d, want 3", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind = 'comic'`); n != 0 {
		t.Errorf("comic works = %d, want 0 — this slice writes no comics at all", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM service_item_link WHERE remote_kind = 'book'`); n != 3 {
		t.Errorf("service_item_link rows = %d, want 3 — one per MAPPED book", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM service_item_link WHERE remote_id = '103' OR remote_id = '104'`); n != 0 {
		t.Errorf("the skipped books left %d links behind; a skip must leave no row at all", n)
	}

	// The audiobook is a 'book' work whose remote_subtype carries the upstream's
	// own format token, verbatim and unparsed (§6.5 rule 3). That token is what
	// the file pass will turn into edition.format without a second read.
	var subtype string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT sil.remote_subtype FROM service_item_link sil
		  JOIN work w ON w.id = sil.work_id WHERE w.title = 'Project Hail Mary'`).Scan(&subtype); err != nil {
		t.Fatalf("read the audiobook's remote_subtype: %v", err)
	}
	if subtype != "m4b" {
		t.Errorf("remote_subtype = %q, want m4b", subtype)
	}

	// ── THE AUDIOBOOK IS AN AUDIOBOOK, and a user can find it ───────────────
	//
	// 🚩 THIS IS THE ASSERTION THE FILE AND CREDIT PASSES EXIST FOR, and until
	// they landed it failed at the last step while every check above it passed.
	// The adapter always read the m4b correctly — MediaKind classified it,
	// remote_subtype stored the token, and the block above proves both. What was
	// missing was the ROW: store.mediaTypeOf splits Ebooks from Audiobooks on
	// `MIN(edition.format = 'audiobook')` and browseAudiobookPredicate seeks
	// `edition.format = 'audiobook'`, BookOrbitSource implemented no FileSource,
	// so no edition existed — and mediaTypeOf's documented answer for a work
	// with no editions is Ebooks. Every BookOrbit book rendered as an Ebook and
	// /library/audiobooks returned none of them.
	//
	// It is asserted through the REAL HTTP SURFACE and not against the edition
	// table, because a row that no query reaches is the same defect one layer
	// down. /library/audiobooks is the SPA route; `?media_type=audiobooks` is
	// the request it makes.
	var audiobooks struct {
		Items []struct {
			Title     string `json:"title"`
			MediaType string `json:"media_type"`
			Kind      string `json:"kind"`
			Year      *int64 `json:"year"`
		} `json:"items"`
	}
	env.do(t, "GET", "/api/v1/library?media_type=audiobooks", nil, &audiobooks)
	if len(audiobooks.Items) != 1 {
		t.Fatalf("/library/audiobooks returned %d works, want 1 — Project Hail Mary: %+v",
			len(audiobooks.Items), audiobooks.Items)
	}
	got := audiobooks.Items[0]
	if got.Title != "Project Hail Mary" || got.MediaType != "audiobooks" {
		t.Errorf("the audiobooks grid holds %+v, want Project Hail Mary as audiobooks", got)
	}
	if got.Kind != "book" {
		t.Errorf("kind = %q, want book — an audiobook is an EDITION of a book work "+
			"(ADR-0031); work.kind has no 'audiobook' member and a build that gave it "+
			"one would split the user's library in two", got.Kind)
	}
	// The year rides the credit set, which is the same card the walk read. A
	// build that dropped it renders the Year column empty for every book.
	if got.Year == nil || *got.Year != 2021 {
		t.Errorf("year = %v, want 2021 — publishedYear is on the card", got.Year)
	}

	// And the OTHER side of the split, which is what makes the first half mean
	// something: the two ebooks are on the Ebooks grid and the audiobook is not.
	var ebooks struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	env.do(t, "GET", "/api/v1/library?media_type=ebooks", nil, &ebooks)
	if len(ebooks.Items) != 2 {
		t.Fatalf("/library/ebooks returned %d works, want 2: %+v", len(ebooks.Items), ebooks.Items)
	}
	for _, it := range ebooks.Items {
		if it.Title == "Project Hail Mary" {
			t.Error("the audiobook is on the Ebooks grid as well; a build that wrote no " +
				"edition at all passes the audiobooks check only by returning nothing, " +
				"so this is the half that catches the reverse mistake")
		}
	}

	// ── the rows underneath, so a failure above is diagnosable ──────────────
	if n := countIn(t, env, `SELECT COUNT(*) FROM edition WHERE format = 'audiobook'`); n != 1 {
		t.Errorf("audiobook editions = %d, want exactly 1", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM edition WHERE format = 'ebook'`); n != 2 {
		t.Errorf("ebook editions = %d, want 2", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM edition`); n != 3 {
		t.Errorf("edition rows = %d, want 3 — ONE PRIMARY EDITION PER BOOK. `edition` has "+
			"no unique index, so a writer that inserted unconditionally would add a row "+
			"per import forever", n)
	}

	// The cover on the audiobook's card is not a file the reader holds.
	if n := countIn(t, env, `SELECT COUNT(*) FROM media_file`); n != 3 {
		t.Errorf("media_file rows = %d, want 3 — one content file per book. The audiobook's "+
			"card also carries a cover, and work.have_count is COUNT(media_file)", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM media_file WHERE path = 'bookorbit:bookfile:2012'`); n != 0 {
		t.Errorf("the cover produced %d media_file rows; a cover is not content", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM media_file WHERE path LIKE '%/%' OR path LIKE '%\%'`); n != 0 {
		t.Errorf("%d media_file rows carry a path separator; v0.1 stores an OPAQUE "+
			"SURROGATE and never a host filesystem path", n)
	}

	// ── the credits, from the same card and with no second request ──────────
	if n := countIn(t, env, `SELECT COUNT(*) FROM work_credit WHERE role = 'author'`); n != 1 {
		t.Errorf("author credits = %d, want 1 — Andy Weir", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM work_credit WHERE role = 'narrator'`); n != 1 {
		t.Errorf("narrator credits = %d, want 1 — Ray Porter. This is the FIRST writer of "+
			"work_credit.role's 'narrator' member in the schema", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM work WHERE kind = 'person' AND title = 'Ray Porter'`); n != 1 {
		t.Errorf("'person' works named Ray Porter = %d, want 1 — a credit points at a work", n)
	}
	// The credit and file passes issue NO requests of their own. Anything else
	// would mean the card was read twice, which is the premise this whole slice
	// rests on.
	for _, r := range bo.requests() {
		if strings.Contains(r.Path, "/books/") {
			t.Errorf("the import made a per-book read (%s %s); credits, files and the year "+
				"all ride the card the walk already fetched", r.Method, r.Path)
		}
	}

	// has_file comes from books.status, which is a first-class three-valued
	// state upstream. Only 'present' is a file.
	if n := countIn(t, env, `SELECT COUNT(*) FROM service_item_link WHERE has_file = 1`); n != 3 {
		t.Errorf("links with has_file = %d, want 3 — every mapped book here is 'present'", n)
	}

	// ── identity ────────────────────────────────────────────────────────────
	//
	// One external_id, and exactly one. hardcoverId is the ONLY identifier on
	// BookCard that may be written against a work: §6.4's amendment 4 elects
	// `hardcover_book` work-strong. isbn13 and hardcoverEditionId are on the
	// same card and are EDITION identifiers, which amendment 4 forbids
	// satisfying ux_extid_work_strong and which have no row to sit on until the
	// edition pass ships.
	var source, value string
	var confidence float64
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(),
		`SELECT source, value, confidence FROM external_id`).Scan(&source, &value, &confidence); err != nil {
		t.Fatalf("read the one external_id: %v", err)
	}
	if source != "hardcover_book" || value != "hc-42" || confidence < 1.0 {
		t.Errorf("external_id = (%q, %q, %v), want (hardcover_book, hc-42, 1.0)", source, value, confidence)
	}

	// ── the subtitle is searchable ──────────────────────────────────────────
	//
	// This is the user-visible reason the subtitle is carried at all: it lands
	// as an `alias` alt title, and the search-document builder folds alt titles
	// into the FTS row, so a search for the subtitle finds the book.
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM work_alt_title WHERE kind = 'alias' AND title = 'The Making of the Atomic Bomb'`); n != 1 {
		t.Errorf("the subtitle produced %d alias rows, want 1", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM search_fts WHERE search_fts MATCH 'atomic'`); n != 1 {
		t.Errorf("searching the subtitle matched %d documents, want 1 — an alt title that never "+
			"reaches the FTS document is a field with no reader", n)
	}

	// ── the skip is VISIBLE, which is the whole point of counting it ────────
	//
	// A log line is not a record: an import triggered by a background connect
	// has no caller left to read the Report by the time anyone opens the
	// Libraries screen and asks why a library of four shows two.
	var detail string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT detail FROM sync_report
		 WHERE kind = 'items_skipped' AND remote_kind = 'library' AND remote_id = '1'`).Scan(&detail); err != nil {
		t.Fatalf("no items_skipped row for library 1: %v", err)
	}
	var note struct {
		Name           string `json:"name"`
		SkippedComics  int    `json:"skipped_comics"`
		SkippedUnknown int    `json:"skipped_unknown"`
		Reason         string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(detail), &note); err != nil {
		t.Fatalf("decode the items_skipped detail %q: %v", detail, err)
	}
	if note.SkippedComics != 1 || note.SkippedUnknown != 1 {
		t.Errorf("skip tally = %+v, want 1 comic and 1 unknown", note)
	}
	if note.Name != "Fiction" || note.Reason == "" {
		t.Errorf("the note names no library or gives no reason: %+v", note)
	}
	// Library 2 skipped nothing, so it must have NO row: a zero written as a row
	// would make the Libraries screen show a warning about a library that is
	// fully imported.
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM sync_report WHERE kind = 'items_skipped' AND remote_id = '2'`); n != 0 {
		t.Errorf("library 2 skipped nothing and still got %d notes", n)
	}
	t.Logf("bookorbit import: 5 books read, 3 mapped, 1 comic and 1 unknown skipped; note = %s", detail)
}

// TestABookOrbitWalkAsksForBooksPerLibraryAndNeverCollapsesSeries pins the two
// properties of the request body that decide whether the walk can file anything
// at all.
//
// Both are invisible in a row count and both are silent when wrong:
//
//  1. THE PER-LIBRARY ROUTE, not the global POST /books/query. BookCard carries
//     no libraryId (BookDetail does; the card does not), so the container an
//     item belongs to is the route parameter and nothing else. A global walk
//     would decode every book and file none of them, because
//     store.ApplyCatalogueBatch matches ContainerID against the binding map.
//  2. NO collapseSeries. Collapsing returns one representative row per series
//     instead of one per book, which would import a fraction of the library
//     with no error anywhere.
func TestABookOrbitWalkAsksForBooksPerLibraryAndNeverCollapsesSeries(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(7, "Fiction", 1)}
	bo.books = map[int][]map[string]any{7: {boBook(1, "A Book", nil)}}

	env := boSetUp(t)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)
	waitForImport(t, env, created.ID)

	var walks int
	for _, r := range bo.requests() {
		switch {
		case r.Method == "POST" && r.Path == "/api/v1/libraries/7/books":
			walks++
			var body map[string]any
			if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
				t.Fatalf("the walk body is not JSON: %q", r.Body)
			}
			if _, ok := body["collapseSeries"]; ok {
				t.Errorf("the walk sent collapseSeries; a catalogue replica reads one row per BOOK: %q", r.Body)
			}
			pag, _ := body["pagination"].(map[string]any)
			if pag == nil || pag["size"] != float64(200) {
				t.Errorf("pagination = %v, want size 200 — BookQueryPipe's own ceiling: %q", pag, r.Body)
			}
		case r.Method == "POST" && r.Path == "/api/v1/books/query":
			t.Errorf("the walk used the GLOBAL book query; BookCard carries no libraryId, so nothing "+
				"it returned could be filed into a library: %q", r.Body)
		}
	}
	if walks == 0 {
		t.Fatal("no per-library book walk happened at all, so this test asserted nothing")
	}
}
