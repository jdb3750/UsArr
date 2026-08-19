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
			// 🚩 A COMIC WITH NO SERIES — ADR-0068 decision 2's residue. It is
			// read, mapped as a `comic_issue` under a SYNTHESIZED single-issue
			// series named for the book, and flagged is_oneshot. It is never
			// written as a 'book' work, never silently dropped, and never
			// promoted to a 'comic' work in its own right.
			boBook(103, "The Sandman: Endless Nights", map[string]any{
				"files": []map[string]any{boFile(1031, "cbz", "primary")},
			}),
			// 🚩 TWO COMICS OF ONE SERIES, one of which is ALSO in a second
			// series. Two issues under one parent is the smallest shape that can
			// tell ADR-0068's accepted implementation from the per-row one it
			// refused, and the extra membership is decision 3's "recorded, not
			// resolved".
			boBook(105, "Saga, Vol. 1", map[string]any{
				"files":       []map[string]any{boFile(1051, "cbz", "primary")},
				"seriesId":    5,
				"seriesName":  "Saga",
				"seriesIndex": 1,
				"seriesMemberships": []map[string]any{
					{"seriesId": 5, "seriesName": "Saga", "seriesIndex": 1,
						"displayOrder": 0, "expectedBookCount": 11},
				},
			}),
			boBook(106, "Saga, Vol. 2", map[string]any{
				"files":       []map[string]any{boFile(1061, "cbz", "primary")},
				"seriesId":    5,
				"seriesName":  "Saga",
				"seriesIndex": 2,
				"seriesMemberships": []map[string]any{
					{"seriesId": 5, "seriesName": "Saga", "seriesIndex": 2,
						"displayOrder": 0, "expectedBookCount": 11},
					{"seriesId": 9, "seriesName": "Image Firsts", "seriesIndex": nil,
						"displayOrder": 1, "expectedBookCount": nil},
				},
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
	// Two BookOrbit libraries and THREE UsArr libraries. BookOrbit supplies no
	// container-level kind at all — `libraries` has no type, kind or mediaType
	// column — so every container BINDS as 'book', and library 1 turns out to
	// hold comics as well. ADR-0066 decision 5, activated by ADR-0068: a MIXED
	// container "becomes a `book` library and a `comic` library over the same
	// `library_source` container ref".
	//
	// ⚠️ THE COMIC LIBRARY IS MINTED LAZILY, on the first comic the walk actually
	// reaches, and library 2 proves it: it holds only an audiobook and gets NO
	// comic sibling. A comic library minted at bind time for every container
	// would be a permanently empty row on the Libraries screen — principle 3's
	// "empty screen that looks broken" — and decision 5's word is MIXED.
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE id <> 0 AND managed_by = 'auto'`); n != 3 {
		t.Errorf("auto libraries = %d, want 3", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE kind = 'book' AND id <> 0`); n != 2 {
		t.Errorf("book libraries = %d, want 2", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE kind = 'comic' AND id <> 0`); n != 1 {
		t.Errorf("comic libraries = %d, want 1 — library 1 is mixed and library 2 is not", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library_source WHERE container_kind = 'remote_library'`); n != 3 {
		t.Errorf("library_source rows = %d, want 3", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library_source ls JOIN library l ON l.id = ls.library_id
	                          WHERE ls.container_ref = '1'`); n != 2 {
		t.Errorf("library_source rows over container '1' = %d, want 2 — decision 5's two "+
			"libraries stand over the SAME container ref, which needs no migration because "+
			"library_source's uniqueness is (library_id, service_instance_id, "+
			"container_kind, container_ref)", n)
	}

	// ── the items: seven books read, SIX written ────────────────────────────
	//
	// Three prose books, three comics as ISSUES, and the unclassifiable file
	// skipped. The pair is what proves anything: a build that wrote every book
	// would give 7 and would have written a work for a file BookOrbit itself
	// cannot classify; a build that dropped the audiobook would give 5; a build
	// that mapped a comic into the BOOK cascade would give 6 book works and be
	// unrecoverable, because §6.4's cascade makes a wrong work.kind at ingest
	// permanently unmergeable.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind = 'book'`); n != 3 {
		t.Errorf("book works = %d, want 3 — two ebooks and one audiobook, and NOT a comic", n)
	}
	issues := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind = 'comic_issue'`)
	series := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind = 'comic'`)
	if issues != 3 {
		t.Errorf("comic_issue works = %d, want 3 — one one-shot and two Saga volumes", issues)
	}
	// ⚠️ ADR-0068's SECOND DONE-CHECK, STATED AS A FAILURE CONDITION RATHER THAN
	// AS A NUMBER TO LOOK AT. "If the series count EQUALS the issue count, the
	// per-row implementation shipped and this check MUST FAIL … it is the only
	// outcome here that is worse than not shipping."
	if series == issues {
		t.Fatalf("series works = issue works = %d — one 'comic' work per comic FILE is the "+
			"alternative ADR-0066 pre-emptively refused, and it flattens ARCHITECTURE §13's "+
			"thirty-to-one ratio to one-to-one", series)
	}
	if series != 2 {
		t.Errorf("comic works = %d, want 2 — the synthesized one-shot series and Saga", series)
	}
	// ADR-0068's FIRST done-check: zero rows with parent_work_id IS NULL.
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM work WHERE kind = 'comic_issue' AND parent_work_id IS NULL`); n != 0 {
		t.Errorf("parentless comic_issue rows = %d, want 0 — a parentless issue is not a "+
			"degraded comic, it is a row no shipped read can see", n)
	}
	// The one-shot's flag is WRITTEN, not left to the column's DEFAULT 0.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work_comic_issue WHERE is_oneshot = 1`); n != 1 {
		t.Errorf("is_oneshot rows = %d, want 1 — only the comic with no series is a one-shot", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM work_comic_issue WHERE number_sort IS NOT NULL`); n != 2 {
		t.Errorf("issues carrying a number_sort = %d, want 2 — the two Saga volumes report a "+
			"seriesIndex and the one-shot does not", n)
	}
	// The comic side is filed and indexed AT THE SERIES, never at the issue.
	if n := countIn(t, env, `SELECT COUNT(*) FROM search_doc WHERE kind = 'comic_issue'`); n != 0 {
		t.Errorf("comic_issue search_doc rows = %d, want 0 — writeSearchDoc RETURNS AN ERROR "+
			"on this kind, so a build that routed an issue through the top-level path would "+
			"have failed this import outright", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM search_doc WHERE kind = 'comic'`); n != 2 {
		t.Errorf("comic search_doc rows = %d, want 2", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library_member lm JOIN work w ON w.id = lm.work_id
	                          WHERE w.kind = 'comic_issue'`); n != 0 {
		t.Errorf("comic_issue library_member rows = %d, want 0", n)
	}
	// NO SERIES WORK IS EVER MINTED INTO NO LIBRARY AT ALL, and it is the COMIC
	// library it lands in rather than the book one it was walked from.
	if n := countIn(t, env, `SELECT COUNT(*) FROM library_member lm
	                           JOIN work w ON w.id = lm.work_id
	                           JOIN library l ON l.id = lm.library_id
	                          WHERE w.kind = 'comic' AND l.kind = 'comic'`); n != 2 {
		t.Errorf("comic series filed into a comic library = %d, want 2", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM service_item_link WHERE remote_kind = 'book'`); n != 6 {
		t.Errorf("service_item_link rows = %d, want 6 — one per MAPPED book, comics "+
			"included: remote_kind is the UPSTREAM's noun and a .cbz is a `books` row to "+
			"BookOrbit", n)
	}
	// ONE series link per SERIES, not per issue: the parent is resolved through
	// the link the first issue wrote.
	if n := countIn(t, env, `SELECT COUNT(*) FROM service_item_link WHERE remote_kind = 'series'`); n != 2 {
		t.Errorf("series links = %d, want 2", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM service_item_link WHERE remote_id = '104'`); n != 0 {
		t.Errorf("the skipped book left %d links behind; a skip must leave no row at all", n)
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
	//
	// ⚠️ THIS LOOP USED TO REFUSE EVERY `/books/` PATH, and that was right when
	// nothing anywhere fetched a cover. internal/libsync's phase D now issues one
	// GET /api/v1/books/{id}/cover per mapped book, so the assertion is narrowed
	// to the claim it was really making — THE CARD IS NOT READ TWICE — rather than
	// widened into a green by deletion. A cover is not the card: it is a different
	// route, in a different pass, after the item stream has closed, and it can
	// never carry a title, a credit or a year.
	//
	// The cover reads are COUNTED below rather than merely permitted, so a pass
	// that started making two per book, or one per book on a re-import, still
	// fails here.
	covers := 0
	for _, r := range bo.requests() {
		if strings.HasSuffix(r.Path, "/cover") {
			covers++
			continue
		}
		if strings.Contains(r.Path, "/books/") {
			t.Errorf("the import made a per-book read (%s %s); credits, files and the year "+
				"all ride the card the walk already fetched", r.Method, r.Path)
		}
	}
	if covers != 3 {
		t.Errorf("cover reads = %d, want 3 — one per MAPPED book, and never one for a "+
			"book the walk skipped", covers)
	}
	// ⚠️ AND NOTHING WAS RECORDED FROM THEM. This fake answers 404 on the cover
	// route (its catch-all), which BookOrbit returns for a book with no cover
	// file, a book that does not exist AND a book the credential cannot see — so
	// no durable verdict may be derived from one. The import is still green and
	// the catalogue is still complete, which is the whole behaviour.
	if n := countIn(t, env, `SELECT COUNT(*) FROM image_asset`); n != 0 {
		t.Errorf("image_asset rows = %d, want 0: a 404 must not be cached as 'no cover'", n)
	}

	// has_file comes from books.status, which is a first-class three-valued
	// state upstream. Only 'present' is a file.
	//
	// ⚠️ SIX, NOT SEVEN. Every mapped BOOK is 'present', comics included; the two
	// SERIES links carry has_file = 0, because a series has no bytes of its own —
	// §6.3's availability for a series comes from the rollup over its children,
	// never from a flag copied off one issue.
	if n := countIn(t, env, `SELECT COUNT(*) FROM service_item_link WHERE has_file = 1`); n != 6 {
		t.Errorf("links with has_file = %d, want 6 — every mapped book here is 'present'", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM service_item_link WHERE remote_kind = 'series' AND has_file = 1`); n != 0 {
		t.Errorf("series links claiming a file = %d, want 0", n)
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
	// ⚠️ ADR-0068's FOURTH DONE-CHECK: the latest items_skipped row's Comics field
	// reads 0. The field is KEPT rather than removed — every row written before
	// that ADR carries a `skipped_comics` key and store.SkipNote's JSON contract
	// is explicit that dropping one "silently orphans that half of the history" —
	// so the assertion is on the VALUE.
	if note.SkippedComics != 0 {
		t.Errorf("skip tally = %+v, want 0 comics: comics are imported as issues now", note)
	}
	if note.SkippedUnknown != 1 {
		t.Errorf("skip tally = %+v, want 1 unknown — the file with no format", note)
	}
	if note.Name != "Fiction" || note.Reason == "" {
		t.Errorf("the note names no library or gives no reason: %+v", note)
	}
	// ⚠️ THE INVARIANT, AND THIS ASSERTION IS INVERTED RATHER THAN DELETED.
	//
	// It used to read *"library 2 skipped nothing and still got %d notes"*, on
	// ADR-0061 §5: no skip meant no row, because a zero row was thought to put a
	// warning on a fully imported library. It does not — `none` renders nothing
	// on §17.8 — and the absence cost more than it saved, because "walked clean"
	// and "never walked" then looked identical and the read had to borrow the
	// completeness row to tell them apart. ADR-0063 reversed it: EVERY container
	// an import walked gets a row, zero or not.
	var zeroDetail string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT detail FROM sync_report
		 WHERE kind = 'items_skipped' AND remote_kind = 'library' AND remote_id = '2'`,
	).Scan(&zeroDetail); err != nil {
		t.Fatalf("library 2 was walked and skipped nothing, so it must carry a ZERO-COUNT "+
			"skip row; there is none: %v", err)
	}
	var zero struct {
		SkippedComics  int    `json:"skipped_comics"`
		SkippedUnknown int    `json:"skipped_unknown"`
		Reason         string `json:"reason"`
		DoesNotCover   string `json:"does_not_cover"`
	}
	if err := json.Unmarshal([]byte(zeroDetail), &zero); err != nil {
		t.Fatalf("decode library 2's zero skip detail %q: %v", zeroDetail, err)
	}
	if zero.SkippedComics != 0 || zero.SkippedUnknown != 0 {
		t.Errorf("library 2's row = %+v, want both tallies zero", zero)
	}
	// ⚠️ AND NO REASON ON IT. The reason explains a skip; on a row saying nothing
	// was skipped it asserts a cause for a non-event. The SCOPE still travels,
	// because "a skip count is not a completeness verdict" is as true of a zero.
	if zero.Reason != "" {
		t.Errorf("the zero row explains a skip that did not happen: reason = %q", zero.Reason)
	}
	if zero.DoesNotCover == "" {
		t.Error("the zero row carries no scope, so an operator reading it out of the " +
			"database cannot tell it apart from a completeness claim")
	}
	// ⚠️ THE OTHER HALF OF THE INVARIANT: a container NOTHING walked gets no row.
	// This fixture's BookOrbit serves libraries 1 and 2 only, so '3' is a
	// container the walk never reached — and if rows were synthesised from the
	// container list before the walk instead of from tallies raised during it,
	// this is where that would show.
	if n := countIn(t, env, `SELECT COUNT(*) FROM sync_report
		 WHERE kind = 'items_skipped' AND remote_id NOT IN ('1','2')`); n != 0 {
		t.Errorf("%d skip rows exist for containers the walk never reached", n)
	}
	// The row also carries the SCOPE of what it claims, on the completeness
	// row's pattern (ADR-0061 §6): a count with no scope invites being read as
	// "and the rest is complete", which is a claim nothing here measured.
	var scope struct {
		Covers       string `json:"covers"`
		DoesNotCover string `json:"does_not_cover"`
	}
	if err := json.Unmarshal([]byte(detail), &scope); err != nil {
		t.Fatalf("decode the skip scope %q: %v", detail, err)
	}
	if scope.Covers == "" || !strings.Contains(scope.DoesNotCover, "completeness") {
		t.Errorf("the row does not say what it does not cover: %+v — an operator reading "+
			"this out of the database has only the row, not the doc comment", scope)
	}

	// ── AND IT IS VISIBLE WITHOUT DATABASE ACCESS, which is the whole point ──
	//
	// Everything above proves the number was RECORDED. This proves it can be
	// READ: the Libraries screen's own endpoint, over the same rows, with no
	// upstream call on the path (principle 1).
	//
	// ⚠️ THE TWO SILENCES ARE ASSERTED APART HERE TOO. Library 2 was walked,
	// skipped nothing, and gets `none` off its own zero row; a library nothing
	// walked would get no key at all, and if those collapsed, an absent verdict
	// would start reading as an all-clear.
	var libs struct {
		Items []struct {
			Name    string `json:"name"`
			Skipped *struct {
				State  string `json:"state"`
				Items  int64  `json:"items"`
				Reason string `json:"reason"`
			} `json:"skipped"`
		} `json:"items"`
	}
	env.do(t, "GET", "/api/v1/libraries", nil, &libs)
	if len(libs.Items) != 3 {
		t.Fatalf("GET /api/v1/libraries returned %d rows, want the 3 the import created: %+v",
			len(libs.Items), libs.Items)
	}
	// ⚠️ THE COMIC LIBRARY'S NAME IS "Fiction (2)", AND THAT IS THE SHIPPED
	// DISAMBIGUATION RATHER THAN A NAME THIS SLICE CHOSE. bindOneContainer's step
	// 3 walks `name (n)` until BOTH ux_library_name and ux_library_slug are free;
	// the comic sibling cannot JOIN the book library of the same name because
	// step 2 requires the kinds to agree, so it lands on the next free name. It
	// is asserted rather than left implicit BECAUSE IT IS UNSATISFYING: neither
	// ADR-0066 decision 5 nor ADR-0068 rules on what a sibling library is called,
	// so nothing here invents a convention, and this assertion is where a reader
	// who wants one will find the current answer.
	// (byName below is built from these rows; the assertion is with the others.)
	byName := map[string]string{}
	for _, l := range libs.Items {
		if l.Skipped == nil {
			byName[l.Name] = "<absent>"
			continue
		}
		byName[l.Name] = l.Skipped.State
		if l.Skipped.State == "left_out" {
			if l.Skipped.Items != 1 {
				t.Errorf("%s reports %d items left out on the wire, want 1 — the "+
					"unclassifiable file, and NOT the comic, which is imported now",
					l.Name, l.Skipped.Items)
			}
			if l.Skipped.Reason == "" {
				t.Errorf("%s says items were left out and does not say why", l.Name)
			}
		} else if l.Skipped.Items != 0 || l.Skipped.Reason != "" { //nolint:revive // mirrors the state above
			t.Errorf("%s serves a count or a reason under state %q: %+v",
				l.Name, l.Skipped.State, l.Skipped)
		}
	}
	if byName["Fiction"] != "left_out" {
		t.Errorf("Fiction's skip state on the wire = %q, want left_out — the comic it "+
			"skipped is invisible to anyone without database access", byName["Fiction"])
	}
	if _, ok := byName["Fiction (2)"]; !ok {
		t.Errorf("no 'Fiction (2)' library; the comic sibling takes the next free name under "+
			"the existing disambiguation. Rows: %v", byName)
	}
	// ⚠️ THE COMIC LIBRARY REPORTS THE SAME SKIP AS ITS BOOK SIBLING, AND THAT IS
	// RECORDED HERE RATHER THAN FIXED. §17.8's read joins `library` →
	// `library_source` → `sync_report` on the CONTAINER REF, and a skip is a fact
	// about the container: one unclassifiable file was left out of BookOrbit
	// library 1, and both UsArr libraries standing over library 1 say so. It is
	// not wrong, but it is the first place ADR-0066 decision 5's two-libraries-
	// one-container shape shows through to a screen, and NEITHER ADR RULES ON IT
	// — deciding which of two sibling libraries owns a container's skip is a
	// design question, not an implementation detail, so nothing here decides it.
	if byName["Fiction (2)"] != "left_out" {
		t.Errorf("the comic library's skip state = %q; it stands over the same container as "+
			"Fiction and the skip rows are filed under the CONTAINER, so it reads the same "+
			"row", byName["Fiction (2)"])
	}
	if byName["Audio"] != "none" {
		t.Errorf("Audio's skip state on the wire = %q, want `none`: it was observed by "+
			"the same import and left nothing out, which is a MEASURED negative and must "+
			"not look like the absence that means nobody counted. Every row: %v",
			byName["Audio"], byName)
	}
	// ── ADR-0068 DECISION 4: BOTH RESIDUE DEFAULTS EMIT A sync_report ROW ────
	//
	// "The synthesized-series case and the extra-membership case each write one,
	// so the FIRST REAL IMPORT against the owner's library measures how often
	// each occurs. Sizing comes from instrumentation, not from estimates, and
	// NOT FROM ASKING THE OWNER TO RUN SQL." The rows need no migration:
	// sync_report.kind deliberately carries no CHECK.
	assertResidueRow(t, env, "comic_series_synthesized", "1", 1, 1, 1)
	assertResidueRow(t, env, "comic_series_memberships_declined", "1", 1, 1, 1)
	// ⚠️ AND A ZERO ROW FOR THE CONTAINER THAT HAD NO COMICS AT ALL, on
	// ADR-0063's rule: "none" and "nobody looked" must not render identically, or
	// the measurement decision 4 asks for measures nothing.
	assertResidueRow(t, env, "comic_series_synthesized", "2", 0, 0, 0)
	assertResidueRow(t, env, "comic_series_memberships_declined", "2", 0, 0, 0)

	t.Logf("bookorbit import: 7 books read, 6 mapped (3 prose, 3 comic issues under 2 "+
		"series), 1 unknown skipped; note = %s; wire states = %v", detail, byName)
}

// assertResidueRow reads one ADR-0068 decision 4 row and checks its three counts.
func assertResidueRow(
	t *testing.T, env *testEnv, kind, ref string, synthesized, multi, declined int,
) {
	t.Helper()
	var detail string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT detail FROM sync_report
		 WHERE kind = ? AND remote_kind = 'library' AND remote_id = ?`, kind, ref).Scan(&detail); err != nil {
		t.Fatalf("no %s row for library %s: %v", kind, ref, err)
	}
	var note struct {
		Name              string `json:"name"`
		SynthesizedSeries int    `json:"synthesized_series"`
		MultiSeries       int    `json:"multi_series_books"`
		Declined          int    `json:"declined_memberships"`
		Effect            string `json:"effect"`
		DoesNotCover      string `json:"does_not_cover"`
		Sample            []struct {
			BookID    int64   `json:"book_id"`
			SeriesIDs []int64 `json:"series_ids"`
		} `json:"sample"`
	}
	if err := json.Unmarshal([]byte(detail), &note); err != nil {
		t.Fatalf("decode %s detail %q: %v", kind, detail, err)
	}
	if note.SynthesizedSeries != synthesized || note.MultiSeries != multi || note.Declined != declined {
		t.Errorf("%s/%s = %+v, want synthesized=%d multi=%d declined=%d",
			kind, ref, note, synthesized, multi, declined)
	}
	// The scope travels on EVERY row, zero or not, on store.SkipNote's rule: a
	// number with no scope invites a reading it does not support.
	if note.DoesNotCover == "" {
		t.Errorf("%s/%s carries no scope", kind, ref)
	}
	// ⚠️ NO EFFECT SENTENCE ON A ZERO ROW: it would assert a cause for a
	// non-event, which is exactly what recordSkippedItems already refuses.
	if synthesized+multi == 0 && note.Effect != "" {
		t.Errorf("%s/%s explains a residue that did not happen: %q", kind, ref, note.Effect)
	}
	if kind == "comic_series_memberships_declined" && declined > 0 {
		if len(note.Sample) != 1 {
			t.Fatalf("%s/%s sample = %+v, want one entry", kind, ref, note.Sample)
		}
		// ⚠️ IDS ONLY, NEVER NAMES. reference/security.md §5 keeps upstream
		// response bodies out of this column, and a series NAME is one.
		if strings.Contains(detail, "Image Firsts") || strings.Contains(detail, "Saga") {
			t.Errorf("%s/%s carries an upstream series NAME: %s", kind, ref, detail)
		}
		if note.Sample[0].BookID != 106 || len(note.Sample[0].SeriesIDs) != 1 ||
			note.Sample[0].SeriesIDs[0] != 9 {
			t.Errorf("%s/%s sample = %+v, want book 106 declining series 9 — the PRIMARY "+
				"series 5 is not declined, it is the parent", kind, ref, note.Sample[0])
		}
	}
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
