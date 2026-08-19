package bookorbit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The catalogue read's tests. The fake is local (fake_test.go) and it is a FAKE,
// not a BookOrbit — what it proves is this client's behaviour, never the
// server's. The facts the fake itself encodes are pinned against BookOrbit's own
// source by TestMediaKindVocabularyMatchesTheSource and its siblings below,
// which read the vendored transcription rather than trusting it.

func TestLibrariesProjectsOnlyTheThreeAllowlistedFields(t *testing.T) {
	f := newFake(t)
	f.libraries = []map[string]any{
		{
			"id": 3, "name": " Fiction ", "bookCount": 1204,
			// Everything below is on the wire and must NOT cross the boundary.
			"icon": "book", "displayOrder": 1, "coverAspectRatio": "book",
			"scanMode": "manual", "fileRenameEnabled": true,
			"createdAt": "2026-07-01T00:00:00.000Z", "updatedAt": "2026-08-01T00:00:00.000Z",
			"folders": []map[string]any{
				// 🚩 A HOST FILESYSTEM PATH ON THE BOOKORBIT BOX. This is the
				// field the allowlist exists for.
				{"id": 9, "path": "/mnt/user/media/books", "createdAt": "2026-07-01T00:00:00.000Z"},
			},
		},
	}
	c := f.client(t)

	libs, err := c.Libraries(context.Background())
	if err != nil {
		t.Fatalf("Libraries: %v", err)
	}
	if len(libs) != 1 {
		t.Fatalf("got %d libraries, want 1", len(libs))
	}
	got := libs[0]
	if got.ID != 3 || got.Name != "Fiction" || got.BookCount != 1204 {
		t.Errorf("Library = %+v", got)
	}

	// THE ALLOWLIST, ASSERTED RATHER THAN ASSUMED. A projection is only an
	// allowlist if nothing else can ride along, and the way to see that from a
	// test is to serialise it and look at the keys — a struct field added later
	// shows up here whether or not anyone remembered this test exists.
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(blob, &keys); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]bool{"id": true, "name": true, "book_count": true}
	for k := range keys {
		if !want[k] {
			t.Errorf("Library carries %q across the package boundary; the projection is an "+
				"ALLOWLIST and every field in it needs a reader", k)
		}
	}
	if strings.Contains(string(blob), "/mnt/") {
		t.Errorf("a BookOrbit host filesystem path reached the projection: %s", blob)
	}
}

func TestABookWalkPagesUntilAShortPage(t *testing.T) {
	f := newFake(t)
	f.libraries = []map[string]any{{"id": 1, "name": "Fiction", "bookCount": 201}}

	// ONE MORE THAN A PAGE, so the loop has to cross a boundary and then stop on
	// a page of one. 200 exactly would stop on an EMPTY page, which is a
	// different branch and is covered below.
	const total = BookPageSize + 1
	cards := make([]map[string]any, 0, total)
	for i := range total {
		cards = append(cards, fakeCard(int64(i+1), fmt.Sprintf("Book %03d", i+1), nil))
	}
	f.books = map[int64][]map[string]any{1: cards}
	c := f.client(t)

	var seen []int64
	page, err := c.StreamBooks(context.Background(), 1, func(b Book) error {
		seen = append(seen, b.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamBooks: %v", err)
	}
	if page.Count != total || len(seen) != total {
		t.Errorf("Count = %d, delivered %d, want %d", page.Count, len(seen), total)
	}
	if page.Pages != 2 {
		t.Errorf("Pages = %d, want 2", page.Pages)
	}
	if page.Total != total {
		t.Errorf("Total = %d, want %d", page.Total, total)
	}
	// The order the source promised. BookSortBuilder.build appends `books.id ASC`
	// unconditionally, so a walk that lost or repeated a row shows up here.
	for i, id := range seen {
		if id != int64(i+1) {
			t.Fatalf("book %d of the walk is id %d, want %d — the page boundary lost or "+
				"repeated a row", i, id, i+1)
		}
	}
	// And the pages were asked for in order, 0-based, at the pipe's ceiling.
	var pages []int
	for _, r := range f.requests() {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.Path, "/books") {
			continue
		}
		var q bookQueryRequest
		if err := json.Unmarshal([]byte(r.Body), &q); err != nil {
			t.Fatalf("walk body is not JSON: %q", r.Body)
		}
		if q.Pagination.Size != BookPageSize {
			t.Errorf("asked for size %d, want %d", q.Pagination.Size, BookPageSize)
		}
		if len(q.Sort) != 1 || q.Sort[0].Field != "addedAt" || q.Sort[0].Dir != "asc" {
			t.Errorf("sort = %+v; a channel-1 offset walk is ordered by addedAt ASC so that a "+
				"book added mid-walk lands BEHIND the cursor", q.Sort)
		}
		pages = append(pages, q.Pagination.Page)
	}
	if len(pages) != 2 || pages[0] != 0 || pages[1] != 1 {
		t.Errorf("pages requested = %v, want [0 1] (page is 0-BASED upstream)", pages)
	}
}

func TestABookWalkOfAnExactMultipleStopsOnTheEmptyPage(t *testing.T) {
	f := newFake(t)
	cards := make([]map[string]any, 0, BookPageSize)
	for i := range BookPageSize {
		cards = append(cards, fakeCard(int64(i+1), fmt.Sprintf("Book %03d", i+1), nil))
	}
	f.books = map[int64][]map[string]any{1: cards}
	c := f.client(t)

	page, err := c.StreamBooks(context.Background(), 1, func(Book) error { return nil })
	if err != nil {
		t.Fatalf("StreamBooks: %v", err)
	}
	if page.Count != BookPageSize {
		t.Errorf("Count = %d, want %d", page.Count, BookPageSize)
	}
	// TWO requests, not one: a full page is indistinguishable from the last page
	// until the next one comes back empty. Paying one extra request for that is
	// the correct trade — the alternative is trusting `total`, which is
	// recomputed per page against a live table.
	if page.Pages != 2 {
		t.Errorf("Pages = %d, want 2 — a full final page must be followed by an empty one", page.Pages)
	}
}

func TestAnEmptyLibraryIsOneRequestAndNoBooks(t *testing.T) {
	f := newFake(t)
	c := f.client(t)

	page, err := c.StreamBooks(context.Background(), 7, func(Book) error {
		t.Error("a book was delivered from an empty library")
		return nil
	})
	if err != nil {
		t.Fatalf("StreamBooks: %v", err)
	}
	if page.Count != 0 || page.Pages != 1 || page.Total != 0 {
		t.Errorf("page = %+v, want {0 1 0}", page)
	}
}

// TestAWalkErrorCarriesNoCredentialAndReportsHowFarItGot is the partial-delivery
// contract plus the redaction discipline, on one path.
func TestAWalkErrorCarriesNoCredentialAndReportsHowFarItGot(t *testing.T) {
	f := newFake(t)
	cards := make([]map[string]any, 0, BookPageSize)
	for i := range BookPageSize {
		cards = append(cards, fakeCard(int64(i+1), fmt.Sprintf("Book %03d", i+1), nil))
	}
	f.books = map[int64][]map[string]any{1: cards}

	// The SECOND page fails. The first page's books already reached fn and their
	// effects stand; the count must say so.
	var calls int
	f.booksHandler = func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			f.defaultBooks(w, r)
			return
		}
		nestErrorText(w, r, http.StatusInternalServerError, "Internal server error")
	}
	c := f.client(t)

	var delivered int
	page, err := c.StreamBooks(context.Background(), 1, func(Book) error {
		delivered++
		return nil
	})
	if !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if page.Count != BookPageSize || delivered != BookPageSize {
		t.Errorf("Count = %d, delivered = %d, want %d each — a failed walk still reports how far "+
			"the read got", page.Count, delivered, BookPageSize)
	}
	assertNoCredential(t, "the failed walk's error", err.Error(), f.latestToken())
}

// TestAWalkPassesTheCallersErrorBackUnwrapped. A caller's sentinel has to
// survive, or an importer cannot tell its own abort from a decode failure.
func TestAWalkPassesTheCallersErrorBackUnwrapped(t *testing.T) {
	f := newFake(t)
	f.books = map[int64][]map[string]any{1: {fakeCard(1, "A Book", nil)}}
	c := f.client(t)

	sentinel := errors.New("the caller stopped")
	page, err := c.StreamBooks(context.Background(), 1, func(Book) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the caller's own sentinel", err)
	}
	if page.Count != 0 {
		t.Errorf("Count = %d; the book that FAILED was not delivered", page.Count)
	}
}

func TestAWalkOfAnUnreadableLibraryIsUnauthorized(t *testing.T) {
	f := newFake(t)
	f.booksHandler = func(w http.ResponseWriter, r *http.Request) {
		// LibraryAccessGuard's answer for a library the account was not granted.
		nestErrorText(w, r, http.StatusForbidden, "No access to this library")
	}
	c := f.client(t)

	_, err := c.StreamBooks(context.Background(), 99, func(Book) error { return nil })
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	assertNoCredential(t, "the forbidden-walk error", err.Error(), f.latestToken())
}

// ─── the mapping, against BookOrbit's own transcribed rules ──────────────────

func TestPrimaryFileFollowsTheSourcePredicate(t *testing.T) {
	cases := []struct {
		name  string
		files []BookFile
		want  int64
		ok    bool
	}{
		{"no files at all", nil, 0, false},
		{"role primary wins", []BookFile{
			{ID: 1, Format: "epub", Role: "content"},
			{ID: 2, Format: "m4b", Role: "primary"},
		}, 2, true},
		{"first with a format when no role is primary", []BookFile{
			{ID: 1, Format: "", Role: "supplement"},
			{ID: 2, Format: "pdf", Role: "content"},
		}, 2, true},
		{"first file when nothing has a format", []BookFile{
			{ID: 5, Format: "", Role: "supplement"},
			{ID: 6, Format: "", Role: "content"},
		}, 5, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Book{Files: tc.files}.PrimaryFile()
			if ok != tc.ok || (ok && got.ID != tc.want) {
				t.Errorf("PrimaryFile() = %+v, %v; want id %d, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestMediaKindVocabularyMatchesTheSource is the enum-completeness guard, on the
// pattern internal/libsync's TestEveryLibraryTypeMemberIsMapped sets.
//
// ⚠️ IT PINS A TRANSCRIPTION, NOT A VENDORED SPEC, AND THAT IS THE HONEST LIMIT
// OF IT. BookOrbit commits no OpenAPI document — server/src/swagger.ts builds
// one at RUNTIME and main.ts mounts it only when SWAGGER_ENABLED is true, which
// parseBooleanFlag defaults to false — so neither ADR-0046's two-spec shape nor
// ADR-0047's blob-identity shape transfers. What this test can do is prove the
// derivation is internally consistent and that every format in each set lands
// where the source's own getBookMediaKind puts it. What it CANNOT do is notice
// BookOrbit adding a seventh audio format; only a person re-reading
// packages/types/src/book.ts can, and vendoring that file is the substitute
// worth proposing.
func TestMediaKindVocabularyMatchesTheSource(t *testing.T) {
	// AUDIO_FORMATS and COMIC_FORMATS at bookorbit/bookorbit@73b7877d, written
	// out again rather than derived from the maps under test: a table built from
	// the thing it tests asserts nothing.
	wantAudio := []string{"m4b", "mp3", "m4a", "opus", "ogg", "flac"}
	wantComic := []string{"cbz", "cbr", "cb7", "cbx"}

	if len(audioFormats) != len(wantAudio) {
		t.Errorf("audioFormats has %d members, transcribed from a source with %d", len(audioFormats), len(wantAudio))
	}
	if len(comicFormats) != len(wantComic) {
		t.Errorf("comicFormats has %d members, transcribed from a source with %d", len(comicFormats), len(wantComic))
	}
	for _, f := range wantAudio {
		if got := MediaKindOf(f); got != MediaKindAudiobook {
			t.Errorf("MediaKindOf(%q) = %q, want audiobook", f, got)
		}
		// isAudioFormat lowercases before it looks up, so this must too.
		if got := MediaKindOf(strings.ToUpper(f)); got != MediaKindAudiobook {
			t.Errorf("MediaKindOf(%q) = %q, want audiobook — the source lowercases first", strings.ToUpper(f), got)
		}
	}
	for _, f := range wantComic {
		if got := MediaKindOf(f); got != MediaKindComic {
			t.Errorf("MediaKindOf(%q) = %q, want comic", f, got)
		}
	}
	// The DEFAULT branch: anything not audio and not comic is an ebook, even a
	// format nobody listed. That is getBookMediaKind's own last line and it is
	// why 'pdf' needs no entry anywhere.
	for _, f := range []string{"epub", "pdf", "mobi", "azw3", "fb2", "djvu", "wat"} {
		if got := MediaKindOf(f); got != MediaKindEbook {
			t.Errorf("MediaKindOf(%q) = %q, want ebook (the default branch)", f, got)
		}
	}
	// And the only two ways to reach 'unknown'.
	for _, f := range []string{"", "   "} {
		if got := MediaKindOf(f); got != MediaKindUnknown {
			t.Errorf("MediaKindOf(%q) = %q, want unknown", f, got)
		}
	}
	if got := (Book{}).MediaKind(); got != MediaKindUnknown {
		t.Errorf("a book with NO FILES has kind %q, want unknown — getPrimaryBookFile returns null "+
			"for an empty list and getBookMediaKind(undefined) is 'unknown'", got)
	}
}

func TestProseIsEbookAndAudiobookAndNothingElse(t *testing.T) {
	prose := map[MediaKind]bool{MediaKindEbook: true, MediaKindAudiobook: true}
	for _, k := range []MediaKind{MediaKindEbook, MediaKindAudiobook, MediaKindComic, MediaKindUnknown} {
		var format string
		switch k {
		case MediaKindEbook:
			format = "epub"
		case MediaKindAudiobook:
			format = "m4b"
		case MediaKindComic:
			format = "cbz"
		case MediaKindUnknown:
			format = ""
		}
		b := Book{Files: []BookFile{{ID: 1, Format: format, Role: "primary"}}}
		if got := b.Prose(); got != prose[k] {
			t.Errorf("Prose() for %q = %v, want %v", k, got, prose[k])
		}
	}
}

// TestTheBookProjectionCarriesNoPerUserReadingState is the second half of the
// allowlist argument, and it is the half that matters most: three of BookCard's
// fields are the reading state of the BookOrbit ACCOUNT UsArr borrows, and a
// replica has no business storing them.
func TestTheBookProjectionCarriesNoPerUserReadingState(t *testing.T) {
	f := newFake(t)
	f.books = map[int64][]map[string]any{1: {
		fakeCard(1, "A Book", map[string]any{
			"readStatus": map[string]any{
				"status": "reading", "source": "manual",
				"startedAt": "2026-08-01T00:00:00.000Z", "finishedAt": nil,
				"updatedAt": "2026-08-02T00:00:00.000Z",
			},
			"readingProgress": 0.42,
			"rating":          4.5,
			"metadataScore":   88,
			"customMetadata": []map[string]any{
				{
					"fieldId": 1, "key": "mangabaka_id", "label": "MangaBaka", "type": "text",
					"displayOrder": 0, "value": "12345",
				},
			},
			"isbn13":             "9780000000001",
			"hardcoverEditionId": "hce-9",
		}),
	}}
	c := f.client(t)

	var got Book
	if _, err := c.StreamBooks(context.Background(), 1, func(b Book) error { got = b; return nil }); err != nil {
		t.Fatalf("StreamBooks: %v", err)
	}
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{
		"reading", "0.42", "4.5", "88", "mangabaka", "12345",
		"9780000000001", "hce-9",
	} {
		if strings.Contains(string(blob), forbidden) {
			t.Errorf("the Book projection carries %q: %s", forbidden, blob)
		}
	}
	if got.Title != "A Book" {
		t.Errorf("over-narrow: the title did not survive either: %+v", got)
	}
}

func TestBookFieldsThatDoCross(t *testing.T) {
	f := newFake(t)
	f.books = map[int64][]map[string]any{1: {
		fakeCard(42, "  The Making  ", map[string]any{
			"subtitle": "The Making of the Atomic Bomb",
			"status":   "missing",
			"files": []map[string]any{
				{"id": 421, "format": "M4B", "role": "primary", "sizeBytes": 99},
				{"id": 422, "format": "epub", "role": "content", "sizeBytes": 88},
			},
			"publishedYear": 1986, "language": "en", "pageCount": 890,
			"hardcoverId": "hc-42",
			"addedAt":     "2026-08-01T10:00:00.000Z",
			"updatedAt":   "2026-08-17T07:00:30.118Z",
		}),
	}}
	c := f.client(t)

	var got Book
	if _, err := c.StreamBooks(context.Background(), 1, func(b Book) error { got = b; return nil }); err != nil {
		t.Fatalf("StreamBooks: %v", err)
	}
	if got.ID != 42 || got.Title != "The Making" {
		t.Errorf("id/title = %d/%q", got.ID, got.Title)
	}
	if got.Subtitle != "The Making of the Atomic Bomb" {
		t.Errorf("subtitle = %q", got.Subtitle)
	}
	if got.Status != "missing" {
		t.Errorf("status = %q, want missing — books.status is a three-valued state and only "+
			"'present' means the bytes are there", got.Status)
	}
	if got.PublishedYear != 1986 || got.Language != "en" || got.PageCount != 890 {
		t.Errorf("year/language/pages = %d/%q/%d", got.PublishedYear, got.Language, got.PageCount)
	}
	if got.HardcoverID != "hc-42" {
		t.Errorf("hardcoverId = %q", got.HardcoverID)
	}
	// The format is lowercased on the way in, because getBookMediaKind lowercases
	// before it looks up and UsArr must not disagree with it.
	if k := got.MediaKind(); k != MediaKindAudiobook {
		t.Errorf("MediaKind = %q, want audiobook — an uppercase M4B is still audio upstream", k)
	}
	if got.AddedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps did not parse: added=%v updated=%v", got.AddedAt, got.UpdatedAt)
	}
	if y := got.UpdatedAt.Year(); y != 2026 {
		t.Errorf("updatedAt parsed to year %d", y)
	}
}

func TestAnUnparseableTimestampIsZeroRatherThanAFailedWalk(t *testing.T) {
	f := newFake(t)
	f.books = map[int64][]map[string]any{1: {
		fakeCard(1, "A Book", map[string]any{"updatedAt": "not a date", "addedAt": nil}),
	}}
	c := f.client(t)

	var got Book
	page, err := c.StreamBooks(context.Background(), 1, func(b Book) error { got = b; return nil })
	if err != nil {
		t.Fatalf("StreamBooks: %v — a clock UsArr cannot read is not a reason to refuse a catalogue", err)
	}
	if page.Count != 1 || got.Title != "A Book" {
		t.Errorf("the book did not survive: %+v", got)
	}
	if !got.UpdatedAt.IsZero() || !got.AddedAt.IsZero() {
		t.Errorf("timestamps = %v / %v, want zero", got.AddedAt, got.UpdatedAt)
	}
}

// ─── the constants, pinned against their upstream sources ────────────────────

// TestPageSizeAndOffsetCeilingMatchTheSource keeps the two numbers this walk is
// built on from drifting away from the reasons they were chosen. Both are
// transcriptions; the comment on each is where the citation lives.
func TestPageSizeAndOffsetCeilingMatchTheSource(t *testing.T) {
	if BookPageSize != 200 {
		t.Errorf("BookPageSize = %d; BookQueryPipe's zod schema is .max(200) and a larger value "+
			"is a 400, not a clamp", BookPageSize)
	}
	// MAX_BOOK_QUERY_OFFSET_ROWS is 1_000_000, and the walk must not be able to
	// ask for an offset past it — BookQueryPipe refuses `page * size` beyond it.
	if maxBookPages*BookPageSize > 1_000_000 {
		t.Errorf("the walk can reach offset %d, past BookOrbit's own %d ceiling",
			maxBookPages*BookPageSize, 1_000_000)
	}
}

// TestNoCatalogueRouteEscapesTheApiV1Prefix is ADR-0052's load-bearing
// constraint, asserted on the routes this file added.
//
// The /api/v1 routes are the ones behind the global JwtAuthGuard whose strategy
// reads the Authorization header and the access_token cookie and NEVER the query
// string. Everything outside that prefix — /api/kobo, /api/v3, /api/opds — is
// where the HMAC-token-in-a-URL shape lives, and a catalogue read that wandered
// there would put a credential into every upstream access log.
func TestNoCatalogueRouteEscapesTheApiV1Prefix(t *testing.T) {
	f := newFake(t)
	f.libraries = []map[string]any{{"id": 1, "name": "Fiction", "bookCount": 1}}
	f.books = map[int64][]map[string]any{1: {fakeCard(1, "A Book", nil)}}
	c := f.client(t)

	ctx := context.Background()
	if _, err := c.Libraries(ctx); err != nil {
		t.Fatalf("Libraries: %v", err)
	}
	if _, err := c.StreamBooks(ctx, 1, func(Book) error { return nil }); err != nil {
		t.Fatalf("StreamBooks: %v", err)
	}
	for _, r := range f.requests() {
		if !strings.HasPrefix(r.Path, apiPrefix+"/") {
			t.Errorf("%s %s is outside %s, which is the prefix ADR-0052 constrains this adapter to",
				r.Method, r.Path, apiPrefix)
		}
		if r.Query != "" {
			t.Errorf("%s %s carried a query string (%q); no catalogue route needs one and a "+
				"credential must never reach a URL", r.Method, r.Path, r.Query)
		}
	}
}

// ─── the cassettes ───────────────────────────────────────────────────────────

func TestLibrariesFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_libraries")

	libs, err := c.Libraries(context.Background())
	if err != nil {
		t.Fatalf("Libraries: %v", err)
	}
	if len(libs) != 2 {
		t.Fatalf("got %d libraries, want 2", len(libs))
	}
	if libs[0].ID != 1 || libs[0].Name != "Fiction" || libs[0].BookCount != 3 {
		t.Errorf("libs[0] = %+v", libs[0])
	}
	if libs[1].ID != 2 || libs[1].Name != "Audio" {
		t.Errorf("libs[1] = %+v", libs[1])
	}
}

func TestBookWalkFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_library_books")

	var got []Book
	page, err := c.StreamBooks(context.Background(), 1, func(b Book) error {
		got = append(got, b)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamBooks: %v", err)
	}
	if page.Count != 3 || page.Pages != 1 || page.Total != 3 {
		t.Errorf("page = %+v, want {3 1 3}", page)
	}
	kinds := map[int64]MediaKind{}
	for _, b := range got {
		kinds[b.ID] = b.MediaKind()
	}
	want := map[int64]MediaKind{101: MediaKindEbook, 102: MediaKindAudiobook, 103: MediaKindComic}
	for id, k := range want {
		if kinds[id] != k {
			t.Errorf("book %d has kind %q, want %q", id, kinds[id], k)
		}
	}
	if got[0].Title != "The Hobbit" || got[0].PageCount != 310 {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].HardcoverID != "hc-42" {
		t.Errorf("the audiobook's hardcoverId did not survive: %+v", got[1])
	}
}

// ─── the fixtures' own honesty label ─────────────────────────────────────────

// TestEveryBookOrbitCassetteSaysItIsSynthetic is a guard on the CONVENTION
// rather than on the code, and it is here because the convention is the only
// thing standing between a hand-authored fixture and a reader who believes it
// came off a wire.
//
// internal/bookorbit's vcr_test.go states the rule in prose — "⚠️ THE BOOKORBIT
// CASSETTES IN testdata/cassettes ARE SYNTHETIC … NONE was captured off a wire".
// A prose rule is not self-enforcing and the next cassette is the one that
// forgets it, so every bookorbit_*.yaml must carry the word in its own header.
func TestEveryBookOrbitCassetteSaysItIsSynthetic(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "cassettes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	// The pinned commit every BookOrbit fixture in this tree is authored from.
	pinned := regexp.MustCompile(`bookorbit/bookorbit@73b7877d`)
	var seen int
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "bookorbit_") {
			continue
		}
		seen++
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // fixture path, test-only
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		head := string(raw)
		if !strings.Contains(head, "SYNTHETIC") {
			t.Errorf("%s does not say it is SYNTHETIC. A hand-authored fixture that reads like a "+
				"capture is how a mapping gets mistaken for evidence about a server", e.Name())
		}
		if !pinned.MatchString(head) {
			t.Errorf("%s names no BookOrbit commit it was authored from", e.Name())
		}
	}
	if seen == 0 {
		t.Fatal("scanned 0 bookorbit cassettes — a green here would mean nothing")
	}
	t.Logf("checked the honesty label on %d bookorbit cassettes", seen)
}
