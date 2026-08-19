package bookorbit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The catalogue read — SLICE 1's half of this package.
//
// Slice 0 deliberately knew nothing about a library (see doc.go). This file is
// what changes that, and it is the whole of the change: two routes, both reads,
// both under the /api/v1 prefix ADR-0052 constrains this adapter to, and both
// declaring no @RequirePermission upstream so a shared account with an EMPTY
// permission set reaches them.
//
// Verified from source at bookorbit/bookorbit@73b7877d2fede2221b0ca360af9bfced7c3797f3
// — library.controller.ts, library.service.ts, library.repository.ts,
// book.controller.ts, book.service.ts, book/pipes/book-query.pipe.ts,
// book-sort-builder.service.ts, book/utils/assemble-book-cards.ts,
// packages/types/src/book.ts, packages/types/src/query.ts,
// common/constants/pagination.constants.ts, db/schema/books.ts.
//
// WHAT IS NOT HERE, so the absence reads as a decision rather than an oversight:
// GET /books/:id (the per-book detail read — description, folderPath, the twelve
// further provider ids and the comic roles, one request per book with no batch
// route anywhere in the controller), the thumbnail route, and GET /series. The
// series endpoints are excluded on a measurement rather than on a budget:
// SeriesRepository.findAll projects id, name, bookCount, readCount and
// lastAddedAt only, so there is no series watermark to walk, and every fact they
// carry rides the book stream already. ⚠️ ADR-0068 DEPENDS ON THAT MEASUREMENT
// RATHER THAN MERELY INHERITING IT: comics bind to a parent series through
// BookCard.seriesId, so if the series facts did NOT ride the book stream this
// package would owe a second endpoint. They do, and it does not.
//
// ⚠️ THAT LIST USED TO READ *"the cover and thumbnail routes"*. The COVER route
// is no longer excluded — Client.Cover is in cover.go, deliberately in its own
// file because it is the one read in this package that returns bytes rather than
// a projected document, and the allowlist-DTO discipline this file is built on
// has nothing to say about an image. The THUMBNAIL route is still excluded, and
// cover.go says why.

// ─── GET /api/v1/libraries ───────────────────────────────────────────────────

// libraryDTO is what LibraryService.findAll returns, NARROWED TO WHAT A
// NON-SUPERUSER ACTUALLY GETS.
//
// 🚩 THE SHAPE DEPENDS ON WHO ASKS, and this client is written for the smaller
// of the two. findAll branches on user.isSuperuser: a superuser gets
// LibraryRepository.findAll, which is `...getTableColumns(libraries)` — the
// whole row, allowedFormats and formatPriority and organizationMode included. A
// non-superuser gets findAllForUser, whose select list is exactly
// {id, name, icon, displayOrder, coverAspectRatio, scanMode, fileRenameEnabled,
// createdAt, updatedAt, bookCount}. UsArr's credential is a shared VIEWER
// account by design (scope.go), so the second projection is the one that
// arrives, and anything this client needed from the first would be silently
// absent in production and present in any fixture written from the superuser
// shape.
//
// The three fields below are in BOTH projections. Nothing else is read.
type libraryDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`

	// BookCount is `count(books.id)` over books whose status is 'present'
	// (LIBRARY_BOOK_STATUS_PRESENT, joined in the ON clause), so it is NOT the
	// number of rows POST /libraries/:id/books will return — that query has no
	// status predicate. It is carried for the log line that says how much of a
	// library the walk found, and it is never used as a total or a denominator.
	BookCount int64 `json:"bookCount"`
}

// Library is the allowlisted projection of one BookOrbit library.
//
// WHAT IS DELIBERATELY NOT HERE: `folders`, which is a list of HOST FILESYSTEM
// PATHS on the BookOrbit box and has no reader in UsArr; `icon`, which is an
// operator-supplied string BookOrbit renders and UsArr does not; and
// coverAspectRatio / scanMode / fileRenameEnabled / displayOrder, which are that
// application's presentation and scanning settings. A denylist would have passed
// every one of them through.
//
// ⚠️ THERE IS NO KIND FIELD, AND THAT IS A FACT ABOUT BOOKORBIT RATHER THAN AN
// OMISSION HERE. `libraries` (db/schema/libraries.ts) has no type, kind or
// mediaType column of any sort — the whole column list is name, icon,
// displayOrder, coverAspectRatio, watch, autoScanCronExpression,
// metadataPrecedence, formatPriority, allowedFormats, organizationMode,
// excludePatterns, readingThreshold, markAsFinishedPercentComplete, the
// fileWrite* group, fileRenameEnabled, fileNamingPattern,
// metadataFetchPreferences, bookMetadataFetchConfig,
// bookMetadataFetchLastRunAt, bookMetadataFetchLastQueuedCount, scanMode,
// pollInterval, createdAt, updatedAt. So a caller cannot derive work.kind from
// the container the way internal/libsync's Kavita adapter derives it from
// LibraryDto.type; BookOrbit's only media-kind signal is per BOOK, computed from
// one file's extension (see MediaKind). internal/libsync/bookorbit.go owns what
// UsArr does about that.
type Library struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int64  `json:"book_count"`
}

func toLibrary(d libraryDTO) Library {
	return Library{ID: d.ID, Name: text(d.Name), BookCount: d.BookCount}
}

// text is what every piece of upstream prose goes through on the way into a
// projection: redacted (clean already calls ssrf.RedactText), bounded, and
// TRIMMED.
//
// The trim is here rather than in the adapter because it is a property of the
// boundary and not of one caller: a title with a trailing space is a different
// string from the same title without one, and it would make work.title,
// work.sort_title and the library merge key disagree with themselves depending
// on which layer happened to remember. store.bindOneContainer trims the
// container name again; that is a belt on a brace, not a duplicate rule.
func text(s string) string { return strings.TrimSpace(clean(s)) }

// Libraries calls GET /api/v1/libraries — the container list.
//
// ONE REQUEST, and the response is already scoped to what the credential can
// see: LibraryService.findAll hands a non-superuser
// findAllForUser(user.id, user.contentFilters), which INNER JOINs
// userLibraryAccess. A library UsArr's shared account was not granted is not in
// this list at all, which is the shape ADR-0052's "explicit libraryIds grant"
// produces.
//
// ⚠️ bookCount IS NOT A TOTAL — but the list IS the granted set. `contentFilters`
// lands in the books LEFT JOIN ON rather than in a WHERE, so it shorts each
// count without dropping a row; membership is userLibraryAccess alone. Treat
// this as "what the credential was granted", never as the whole instance.
//
// The shortfall is measurable rather than guessable: GET /libraries/{id}/stats
// takes neither a user nor filters, so its totalBooks minus a status='present'
// page count is exactly what a filter hid. Recorded so the first caller that
// wants that number subtracts for it instead of inferring it.
func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	var raw []libraryDTO
	if err := c.do(ctx, request{
		op: "Libraries", method: http.MethodGet, path: apiPrefix + "/libraries",
	}, &raw); err != nil {
		return nil, err
	}
	out := make([]Library, 0, len(raw))
	for _, d := range raw {
		out = append(out, toLibrary(d))
	}
	return out, nil
}

// ─── POST /api/v1/libraries/:id/books ────────────────────────────────────────

// BookPageSize is the page size the walk asks for.
//
// It is BookQueryPipe's ceiling, read off the zod schema:
// `size: z.number().int().min(1).max(200).default(50)`. A larger value is a 400,
// not a silently clamped page.
const BookPageSize = 200

// maxBookPages bounds the walk.
//
// It is derived from BookOrbit's own offset ceiling rather than invented:
// BookQueryPipe refuses `page * size > MAX_BOOK_QUERY_OFFSET_ROWS`, which is
// 1,000,000 (common/constants/pagination.constants.ts — note the OTHER constant,
// MAX_OFFSET_ROWS = 50,000, governs the generic endpoints and not this one). At
// BookPageSize that is 5,000 pages. The bound exists so that an upstream which
// answered a full page forever could not spin this loop indefinitely; a real
// library ends long before it.
const maxBookPages = 5000

// bookQueryRequest is the body BookQueryPipe accepts.
//
// It carries EXACTLY the two keys this walk needs. `collapseSeries` is omitted
// rather than sent false, and that omission is load-bearing: collapsing would
// return one representative row per series instead of one row per book, which is
// the opposite of what a catalogue replica reads. `filter` and `q` are omitted
// because filtering the comics out upstream would make the count of what was
// skipped unobservable — see internal/libsync/bookorbit.go, where the skip is
// counted rather than hidden.
type bookQueryRequest struct {
	Sort       []bookSortSpec `json:"sort"`
	Pagination bookPagination `json:"pagination"`
}

type bookSortSpec struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

type bookPagination struct {
	// Page is 0-BASED (`page: z.number().int().min(0).default(0)`).
	Page int `json:"page"`
	Size int `json:"size"`
}

// fullWalkSort is the ordering a channel-1 full import asks for, and it is NOT
// the ordering a delta walk will ask for.
//
// 🚩 THE OBVIOUS CHOICE IS `updatedAt desc` AND IT IS THE WRONG ONE HERE.
// internal/libsync/kavita.go sends its delta ordering on the full import too, so
// that "channel 3b, which resumes from the same ordering, is reading the same
// sequence this wrote" — and that argument holds for Kavita because Kavita's
// full read is ONE response with no offsets. BookOrbit's is OFFSET-PAGED, and
// under offset paging the two orderings behave differently while the walk is
// running:
//
//   - `updatedAt DESC`: a book updated mid-walk moves to index 0 and shifts
//     every later row one place further away, so the next page re-reads a row
//     already seen. Harmless (the write is an upsert), but any DELETION ahead of
//     the cursor skips a row instead.
//   - `addedAt ASC`: a book ADDED mid-walk lands at the END, behind the cursor,
//     and disturbs nothing at all. A scan running on the BookOrbit side while
//     UsArr imports is the ordinary case, and this is the ordering under which
//     it costs nothing.
//
// Both orderings are TOTAL and deterministic, because BookSortBuilder.build
// appends `books.id ASC` as an unconditional last tier no matter what was asked
// for. `addedAt` is a member of SORT_FIELDS and SORT_FIELD_MAP binds it to
// `books.addedAt`.
//
// ⚠️ `addedAt` IS NOT IMMUTABLE. BookRepository.updateAddedAt exists and is
// reachable from PATCH /books/:id/added-at, so a person can move a book's place
// in this ordering. It is a deliberate, rare, one-book edit rather than
// something a background job does, and it also bumps updatedAt — so it is a
// worse hazard for the delta channel than for this one.
var fullWalkSort = []bookSortSpec{{Field: "addedAt", Dir: "asc"}}

// booksPageDTO is BooksPage (packages/types/src/book.ts).
type booksPageDTO struct {
	Items []bookCardDTO `json:"items"`
	Total int64         `json:"total"`
	Page  int           `json:"page"`
	Size  int           `json:"size"`
}

// bookCardDTO is the slice of BookCard this walk decodes.
//
// It is NARROWER THAN THE WIRE SHAPE ON PURPOSE, and that is a second allowlist
// rather than a shortcut: encoding/json drops every key with no field here, so
// readStatus, readingProgress, rating, metadataScore, lockedFields,
// hasMetadataLocks, customMetadata, collapsedSeries, coverAspectRatio, genres
// and tags never enter this process at all. Three of those are PER-USER reading
// state belonging to the BookOrbit account UsArr borrows, which is the last
// thing a replica should be storing.
//
// The pointer types are the wire's nulls: BookCard declares `title: string |
// null`, `updatedAt: string | null` and so on, and a *string keeps "absent"
// distinguishable from "empty" at the decode boundary even where the projection
// then flattens the two.
type bookCardDTO struct {
	ID    int64   `json:"id"`
	Title *string `json:"title"`

	// Status is books.status, whose CHECK constraint is
	// `in ('present','missing','processing')` (db/schema/books.ts,
	// books_status_chk).
	Status string `json:"status"`

	Subtitle *string `json:"subtitle"`

	// Authors and Narrators are BookCard.authors and BookCard.narrators —
	// `string[]`, not objects (packages/types/src/book.ts:129, :154). They are
	// NOT nullable on the wire: assembleBookCards emits
	// `authorsByBook.get(row.id) ?? []` and `narratorsByBook.get(row.id) ?? []`
	// (assemble-book-cards.ts:207, :228), so a book with neither carries two
	// empty arrays rather than two nulls.
	//
	// ⚠️ THAT THEY ARE DECLARED IS NOT THAT THEY ARE POPULATED, and on this one
	// the two really do come apart. assembleBookCards takes narratorRows with a
	// `= []` DEFAULT (assemble-book-cards.ts:96), so a caller that omits the
	// argument gets a card whose `narrators` is empty for every book — and one
	// caller does exactly that (scanner.service.ts:331). The route this walk
	// uses is not that caller: POST /libraries/:id/books → queryBooks →
	// BookService.queryForLibrary → executeBooksQuery passes authorRows,
	// narratorRows and fileRows straight out of BookRepository.findCards
	// (library.controller.ts:42-46, book.service.ts:1033-1046, :1127-1148).
	Authors   []string `json:"authors"`
	Narrators []string `json:"narrators"`

	// The FOUR SERIES FIELDS, and they are the whole acquisition cost of comics
	// (ADR-0068's consequences: "the allowlist widens by four fields … Zero
	// extra HTTP"). They are already on the wire in the walk that ships; this
	// declaration is what stops encoding/json dropping them.
	//
	// ⚠️ SeriesID IS BOOKORBIT'S OWN MAINTAINED PRIMARY, not memberships[0] and
	// not null under multi-membership. It is a real scalar column
	// (bookMetadata.seriesId) that the card reads on a path independent of the
	// membership join, and series-membership.service.ts keeps it equal to the
	// displayOrder = 0 membership on EVERY membership write (replaceForBook →
	// syncPrimaryMetadata). Measured at bookorbit/bookorbit commit
	// 73b7877d2fede2221b0ca360af9bfced7c3797f3; ADR-0068 carries the citations.
	// It is null only when the membership list is EMPTY.
	//
	// `seriesId` and `seriesMemberships` are OPTIONAL on the wire (`?:` in
	// BookCard), so absent and null are both a nil pointer / nil slice here, and
	// the projection treats them the same: no primary series.
	SeriesID          *int64                    `json:"seriesId"`
	SeriesName        *string                   `json:"seriesName"`
	SeriesIndex       *float64                  `json:"seriesIndex"`
	SeriesMemberships []bookSeriesMembershipDTO `json:"seriesMemberships"`

	Files         []bookFileDTO `json:"files"`
	PublishedYear *int64        `json:"publishedYear"`
	Language      *string       `json:"language"`
	PageCount     *int64        `json:"pageCount"`
	AddedAt       *string       `json:"addedAt"`
	UpdatedAt     *string       `json:"updatedAt"`

	// HardcoverID is the ONLY identifier field on the card this adapter writes;
	// isbn13 and hardcoverEditionId are on the card too and are deliberately not
	// decoded. Both are EDITION identifiers and external_id's CHECK requires
	// exactly one of work_id / edition_id, so neither may be written against a
	// work.
	//
	// ⚠️ THIS COMMENT USED TO CLOSE ON *"and slice 1 writes no edition rows"*,
	// AND THAT HALF IS NOW FALSE — internal/libsync/bookorbitfiles.go writes one
	// primary edition per book. The conclusion is unchanged and the surviving
	// reason is a different one: there is still no PATH by which an adapter can
	// write an edition-scoped external_id. The edition is minted inside
	// internal/store's file writer, which returns no edition id, and
	// store.FileSet carries files and a format and no identifier. Adding that
	// path is a change to the shared write path, not a field this DTO is waiting
	// on.
	HardcoverID *string `json:"hardcoverId"`
}

// bookSeriesMembershipDTO is BookSeriesMembership
// (packages/types/src/book.ts:115-122).
//
// SeriesIndex is `number | null` and DisplayOrder is not: the primary slot is
// displayOrder 0 and BookOrbit writes the whole list positionally, so a missing
// key here would be a shape this route does not emit rather than a null to
// carry.
type bookSeriesMembershipDTO struct {
	SeriesID     int64    `json:"seriesId"`
	SeriesName   *string  `json:"seriesName"`
	SeriesIndex  *float64 `json:"seriesIndex"`
	DisplayOrder int      `json:"displayOrder"`

	// ExpectedBookCount is series-level and shared by every book in the series.
	// It is decoded because it is the one number that could ever answer "how
	// many issues should this series have"; NOTHING READS IT YET and it is
	// carried no further than BookSeriesMembership.
	ExpectedBookCount *int64 `json:"expectedBookCount"`
}

// bookFileDTO is BookFileRef (packages/types/src/book.ts:75-80).
type bookFileDTO struct {
	ID     int64   `json:"id"`
	Format *string `json:"format"`
	Role   string  `json:"role"`

	// SizeBytes is BookFileRef.sizeBytes, `number | null` on the wire because
	// `bookFiles.sizeBytes` is a nullable bigint (db/schema/books.ts:63).
	// findCards reads it straight onto the card (book.repository.ts:640-644).
	SizeBytes *int64 `json:"sizeBytes"`
}

// BookFile is the allowlisted projection of one file reference.
//
// ⚠️ THIS COMMENT USED TO READ *"sizeBytes is on the wire and is not carried:
// media_file is slice 2's, and a size with no row to sit on is a field that
// would go stale before it had a reader"*. Slice 2 is here and the row exists,
// so the size is carried.
//
// # Role is a FIVE-value vocabulary on the card, not the four the table stores
//
// `book_files_role_chk` is `role in ('content','cover','metadata','supplement')`
// (db/schema/books.ts:92), and scanner/lib/classify.ts:17 declares those same
// four as FILE_ROLES. THE CARD ADDS A FIFTH. assembleBookCards elects one file
// to speak for the book — books.primaryFileId, else role 'primary', else role
// 'content', else files[0] — and REWRITES that file's role to the literal
// 'primary' on the wire (assemble-book-cards.ts:174-186). So a card carries at
// most one 'primary' entry, and it is a per-response label rather than a stored
// fact. PrimaryFile and RoleCarriesContent both read it.
type BookFile struct {
	ID     int64  `json:"id"`
	Format string `json:"format"`
	Role   string `json:"role"`

	// SizeBytes is 0 when BookOrbit reported none — the column is nullable and
	// stays null for a file the scanner could not stat. 0 is not a size a file
	// can have, so "absent" and "zero" need no distinguishing here; media_file's
	// writer stores the value as it stands.
	SizeBytes int64 `json:"size_bytes,omitempty"`
}

// contentRoles is the set of BookFile.Role values meaning "these are the book's
// own bytes", as against the ones that decorate it.
//
// IT IS TWO MEMBERS OUT OF FIVE, AND THE OTHER THREE ARE NOT FILES A READER
// HOLDS. classifyFile assigns them (scanner/lib/classify.ts:26-34): an .opf or
// .nfo is 'metadata', a cover.jpg is 'cover', any other image or unrecognised
// extension is 'supplement'. Counting those as media_file rows would put a
// cover thumbnail into work.have_count — the number §6.3's availability blob
// renders as how much of the thing the user holds — which is a wrong number on
// a screen rather than a harmless extra row.
//
// 'primary' is in the set because it IS a content role: it is the card's label
// for whichever file BookOrbit elected, and that election prefers role
// 'content' before it falls back at all (see BookFile).
var contentRoles = map[string]bool{"primary": true, "content": true}

// RoleCarriesContent reports whether a file in this role is one of the book's
// own bytes. See contentRoles.
func RoleCarriesContent(role string) bool {
	return contentRoles[strings.ToLower(strings.TrimSpace(role))]
}

// Book is the allowlisted projection of one BookCard.
type Book struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`

	// Subtitle is BookCard.subtitle, trimmed. Empty when BookOrbit reported none.
	Subtitle string `json:"subtitle,omitempty"`

	// Status is verbatim: present | missing | processing.
	Status string `json:"status"`

	Files []BookFile `json:"files,omitempty"`

	// Authors and Narrators are the card's own arrays, trimmed, with blanks
	// dropped, IN THE UPSTREAM'S ORDER. That order is BookOrbit's declared
	// billing order rather than an accident of the query: findCards reads
	// authors `.orderBy(bookAuthors.displayOrder)` and narrators
	// `.orderBy(bookNarrators.displayOrder)` (book.repository.ts:634-639,
	// :653-658), and assembleBookCards appends row by row into a per-book list
	// (:101-106, :138-143), so each book's slice arrives in displayOrder.
	//
	// THEY ARE NAMES AND NOTHING ELSE. BookOrbit has an `authors` table with an
	// id and the card carries none of it — `select({ name: authors.name })` is
	// the whole projection — so there is no upstream person id to key a credit
	// on, and internal/libsync dedupes on the normalized name exactly as it does
	// for Kavita, which reports a PersonDto id and still cannot be trusted to
	// mean the same person as another source's.
	Authors   []string `json:"authors,omitempty"`
	Narrators []string `json:"narrators,omitempty"`

	// SeriesID is BookOrbit's own maintained PRIMARY series id, 0 when the card
	// reported none. See bookCardDTO.SeriesID for the measurement that makes
	// "primary" a fact rather than a reading of the JSON.
	SeriesID int64 `json:"series_id,omitempty"`

	// SeriesName is the primary series' name, trimmed. Empty when there is no
	// primary series, and — separately — when BookOrbit reported a series with
	// no name, which the caller must not treat as "no series": SeriesID is the
	// test for that.
	SeriesName string `json:"series_name,omitempty"`

	// SeriesIndex is the book's place in the primary series. A POINTER because
	// 0 is a legal index — a prequel numbered 0 is an ordinary shape — so
	// "absent" and "zero" cannot be flattened here the way PageCount's can.
	SeriesIndex *float64 `json:"series_index,omitempty"`

	// SeriesMemberships is EVERY series this book belongs to, in the upstream's
	// order, INCLUDING the primary. It is RECORDED AND NOT RESOLVED (ADR-0068
	// decision 3, on ADR-0063's record-what-you-declined precedent): the parent
	// binding uses SeriesID and nothing else, and nothing here mints a second
	// parent, a second membership or a work_relation edge. The tier that would
	// adjudicate multi-series membership is v0.3.
	SeriesMemberships []BookSeriesMembership `json:"series_memberships,omitempty"`

	// PublishedYear is 0 when BookOrbit reported none. Year 0 is not a book.
	PublishedYear int64 `json:"published_year,omitempty"`

	// Language is BCP-47-ish upstream text, unparsed.
	Language string `json:"language,omitempty"`

	// PageCount is 0 when BookOrbit reported none.
	PageCount int64 `json:"page_count,omitempty"`

	// AddedAt and UpdatedAt are the parsed ISO-8601 instants
	// (assemble-book-cards.ts calls .toISOString() on both). A value that will
	// not parse lands as the zero Time rather than failing the walk: a clock
	// UsArr cannot read is not a reason to refuse a catalogue.
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// HardcoverID is a Hardcover BOOK id — the work grain, distinct from
	// hardcoverEditionId, which BookOrbit stores in its own column and this
	// projection does not carry.
	HardcoverID string `json:"hardcover_id,omitempty"`
}

// BookSeriesMembership is the allowlisted projection of one
// BookSeriesMembership.
//
// ⚠️ IT EXISTS TO BE RECORDED, NOT ACTED ON. ADR-0068 binds the parent on the
// scalar seriesId alone, and binding on the scalar and on
// memberships[displayOrder = 0] are the same binding BY CONSTRUCTION — so this
// slice is never needed to FIND a parent, which is precisely what keeps
// recording it from silently becoming a resolution.
type BookSeriesMembership struct {
	SeriesID     int64    `json:"series_id"`
	SeriesName   string   `json:"series_name,omitempty"`
	SeriesIndex  *float64 `json:"series_index,omitempty"`
	DisplayOrder int      `json:"display_order"`

	// ExpectedBookCount is 0 when BookOrbit reported none.
	ExpectedBookCount int64 `json:"expected_book_count,omitempty"`
}

// MediaKind is BookOrbit's own per-book media classification.
//
// It is BookOrbit's, not UsArr's: the vocabulary and the derivation are
// packages/types/src/book.ts's BookMediaKind and getBookMediaKind, transcribed
// rather than reinterpreted. TestMediaKindVocabularyMatchesTheSource pins the
// format sets against that file.
type MediaKind string

// The four members of BookMediaKind, in the source's own order.
const (
	// MediaKindEbook is the DEFAULT branch upstream: any format that is neither
	// audio nor comic.
	MediaKindEbook MediaKind = "ebook"
	// MediaKindAudiobook is one of m4b, mp3, m4a, opus, ogg, flac.
	MediaKindAudiobook MediaKind = "audiobook"
	// MediaKindComic is one of cbz, cbr, cb7, cbx.
	MediaKindComic MediaKind = "comic"
	// MediaKindUnknown is what getBookMediaKind returns for an absent or blank
	// format — INCLUDING a book with no files at all, since getPrimaryBookFile
	// returns null for an empty list. It is a real answer and not an error.
	MediaKindUnknown MediaKind = "unknown"
)

// NormalizeFormat is the one comparison rule for a format token: the
// `format?.trim().toLowerCase()` getBookMediaKind opens with
// (packages/types/src/book.ts:98).
//
// It is exported and shared rather than repeated because a caller that
// classified with MediaKindOf and then compared the raw token against a literal
// would be normalising twice by two rules — and it is the SECOND one that would
// silently be wrong on an upstream that ever emitted "PDF" or " epub".
func NormalizeFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}

// audioFormats and comicFormats are AUDIO_FORMATS and COMIC_FORMATS, transcribed
// verbatim from packages/types/src/book.ts@73b7877d.
var (
	audioFormats = map[string]bool{"m4b": true, "mp3": true, "m4a": true, "opus": true, "ogg": true, "flac": true}
	comicFormats = map[string]bool{"cbz": true, "cbr": true, "cb7": true, "cbx": true}
)

// PrimaryFile is getPrimaryBookFile, transcribed:
//
//	files.find(f => f.role === 'primary')
//	  ?? files.find(f => f.format != null)
//	  ?? files[0]
//	  ?? null
//
// It is COPIED rather than invented for the reason internal/libsync/credits.go
// copies Kavita's IsValidYear floor: UsArr's answer and the source's answer must
// not be able to disagree about which file speaks for the book.
//
// ⚠️ ON CARD DATA THE FIRST BRANCH ALWAYS WINS WHEN THERE IS ANY FILE, and that
// is worth knowing rather than discovering. assembleBookCards picks the primary
// server-side — books.primaryFileId, else role 'primary', else role 'content',
// else files[0] — and then REWRITES that file's role to 'primary' on the wire.
// So the card carries at most one role=='primary' entry and it is the server's
// choice, which means this function agrees with BookOrbit by construction rather
// than by luck. The later branches exist for a body that did not come from
// assembleBookCards.
func (b Book) PrimaryFile() (BookFile, bool) {
	for _, f := range b.Files {
		if f.Role == "primary" {
			return f, true
		}
	}
	for _, f := range b.Files {
		if f.Format != "" {
			return f, true
		}
	}
	if len(b.Files) > 0 {
		return b.Files[0], true
	}
	return BookFile{}, false
}

// MediaKind is getBookMediaKind over PrimaryFile's format.
//
// ⚠️ THE PRIMARY FILE'S FORMAT DECIDES, EVEN WHEN ANOTHER FILE DISAGREES. A book
// holding an .epub and an .m4b is ONE kind here, whichever file BookOrbit
// nominated — its own getBookMediaProfile exposes hasEbook/hasAudio/hasComic
// beside primaryMediaKind for exactly that case, and this deliberately reads
// only the latter. UsArr's ebook-versus-audiobook distinction lives on
// edition.format (ADR-0031).
//
// ⚠️ THIS COMMENT USED TO END *"which is slice 2's to write; carrying a second
// opinion here before there is a row for it would be a field with no reader"*,
// and slice 2 has since landed. What did NOT change is the answer: the file
// pass gives the work ONE primary edition and derives its format from this same
// primary-file token (internal/libsync/bookorbitfiles.go), so UsArr and
// BookOrbit still cannot disagree about which file speaks for the book. What
// the mixed case costs is stated where it is paid — a book holding an .epub
// beside an .m4b gets one edition, not two.
func (b Book) MediaKind() MediaKind {
	f, ok := b.PrimaryFile()
	if !ok {
		return MediaKindUnknown
	}
	return MediaKindOf(f.Format)
}

// MediaKindOf is getBookMediaKind over a bare format token
// (packages/types/src/book.ts:97-103).
//
// IT IS EXPORTED BECAUSE THE FILE PASS HAS THE TOKEN AND NOT THE BOOK. The
// primary file's format is stored verbatim on service_item_link.remote_subtype
// during the item walk and carried back to the file pass on FileRequest, so the
// edition's format is settled without a second read — and it must be settled by
// THIS function rather than by a second format table in internal/libsync, which
// would be free to disagree with the classification that decided whether the
// book was imported at all.
func MediaKindOf(format string) MediaKind {
	n := NormalizeFormat(format)
	switch {
	case n == "":
		return MediaKindUnknown
	case audioFormats[n]:
		return MediaKindAudiobook
	case comicFormats[n]:
		return MediaKindComic
	default:
		return MediaKindEbook
	}
}

// Prose reports whether this book is one UsArr's book cascade can hold — an
// ebook or an audiobook, which are one work.kind ('book') differing only in
// edition.format (ADR-0031, and work.kind's own comment: "'audiobook' is
// deliberately NOT a kind").
//
// A comic is NOT prose and neither is an unknown: both are excluded here rather
// than defaulted into 'book', because work.kind "is written once at ingest" and
// §6.4's cascade then forbids works of different kinds from ever merging.
func (b Book) Prose() bool {
	k := b.MediaKind()
	return k == MediaKindEbook || k == MediaKindAudiobook
}

func toBook(d bookCardDTO) Book {
	b := Book{
		ID:            d.ID,
		Title:         text(deref(d.Title)),
		Subtitle:      text(deref(d.Subtitle)),
		Status:        text(d.Status),
		PublishedYear: derefInt(d.PublishedYear),
		Language:      text(deref(d.Language)),
		PageCount:     derefInt(d.PageCount),
		AddedAt:       parseISO(deref(d.AddedAt)),
		UpdatedAt:     parseISO(deref(d.UpdatedAt)),
		HardcoverID:   text(deref(d.HardcoverID)),
	}
	for _, f := range d.Files {
		b.Files = append(b.Files, BookFile{
			ID:        f.ID,
			Format:    strings.ToLower(text(deref(f.Format))),
			Role:      text(f.Role),
			SizeBytes: derefInt(f.SizeBytes),
		})
	}
	b.Authors = names(d.Authors)
	b.Narrators = names(d.Narrators)

	// The primary series. A seriesId of 0 or below is treated as absent for
	// add()'s reason in bookOrbitExternalIDs: BookOrbit's ids are positive
	// serials, so a 0 is a shape this route does not emit rather than a series.
	if d.SeriesID != nil && *d.SeriesID > 0 {
		b.SeriesID = *d.SeriesID
		b.SeriesName = text(deref(d.SeriesName))
		b.SeriesIndex = d.SeriesIndex
	}
	for _, m := range d.SeriesMemberships {
		if m.SeriesID <= 0 {
			continue
		}
		b.SeriesMemberships = append(b.SeriesMemberships, BookSeriesMembership{
			SeriesID:          m.SeriesID,
			SeriesName:        text(deref(m.SeriesName)),
			SeriesIndex:       m.SeriesIndex,
			DisplayOrder:      m.DisplayOrder,
			ExpectedBookCount: derefInt(m.ExpectedBookCount),
		})
	}
	return b
}

// names trims a card's `string[]` and drops the entries that say nothing.
//
// A BLANK IS DROPPED RATHER THAN CARRIED AS A CREDIT WITH NO NAME. `authors.name`
// is NOT NULL upstream, so a blank should not arrive, but a name that is only
// whitespace is indistinguishable from an absent one by the time it reaches
// work_credit — where it would become a 'person' work titled "" that every
// later nameless credit then deduped onto.
func names(in []string) []string {
	var out []string
	for _, n := range in {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int64) int64 {
	if p == nil || *p < 0 {
		return 0
	}
	return *p
}

// parseISO reads the timestamps assemble-book-cards.ts produces with
// Date.toISOString(), i.e. RFC 3339 with milliseconds and a Z.
//
// AN UNPARSEABLE VALUE IS THE ZERO TIME, NOT AN ERROR. A timestamp UsArr cannot
// read costs a delta channel it does not have yet; refusing the whole page over
// one would cost the catalogue.
func parseISO(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// BookPage is what one completed walk reports about itself.
type BookPage struct {
	// Count is how many Books were handed to fn, on success AND on failure —
	// the partial-delivery contract internal/kavita's StreamSeries states, kept
	// identical here so an adapter written against one reads correctly against
	// the other.
	Count int

	// Pages is how many HTTP requests the walk made.
	Pages int

	// Total is the `total` BookOrbit reported on the LAST page read, or -1 if no
	// page was read. It is the upstream's own count and it is NOT a promise: it
	// is recomputed per page against a live table, so it can move under the walk.
	// Never use it as a denominator for "how far are we".
	Total int64
}

// StreamBooks walks POST /api/v1/libraries/{id}/books and hands fn each book as
// a page decodes.
//
// # Why this is paged rather than streamed, unlike internal/kavita
//
// Kavita's POST /api/Series/all-v2 returns the WHOLE library in one response
// (UserParams.PageSize defaults to int.MaxValue), which is why that client
// streams the array with a json.Decoder and never buffers it. BookOrbit refuses
// to do that: BookQueryPipe caps `size` at 200 and a larger value is a 400. So
// the bounded thing here is a PAGE, memory is bounded at one page rather than at
// one element, and the ceiling is 200 cards — small enough that streaming a page
// would buy nothing and cost the ability to see `total`.
//
// # The partial-delivery contract
//
// Identical to StreamSeries's. BookPage.Count is what already reached fn, on
// success and on failure alike. Those calls happened and their effects stand; a
// caller must treat Count as "how far the read got", never "how many rows are
// correct". An error fn returns aborts the walk and comes back unwrapped, so
// errors.Is against a caller's own sentinels works.
//
// # Where the walk stops
//
// A SHORT PAGE — fewer items than were asked for — is the end, which is the only
// stop condition that is correct while the table is moving underneath. `total`
// is deliberately NOT the stop condition: it is recomputed per page, so a book
// added mid-walk would move it and a walk that trusted it would either stop
// early or ask for a page past the end.
func (c *Client) StreamBooks(ctx context.Context, libraryID int64, fn func(Book) error) (BookPage, error) {
	const op = "LibraryBooks"
	path := apiPrefix + "/libraries/" + strconv.FormatInt(libraryID, 10) + "/books"

	page := BookPage{Total: -1}
	if fn == nil {
		return page, &APIError{
			Op: op, Method: http.MethodPost, Path: path,
			Message: "element function is required", Err: ErrInvalidRequest,
		}
	}
	if libraryID < 0 {
		return page, &APIError{
			Op: op, Method: http.MethodPost, Path: path,
			Message: "library id must not be negative", Err: ErrInvalidRequest,
		}
	}

	// THE WALK gets Timeouts.List; each PAGE gets Timeouts.Default. That split is
	// the shape of this endpoint rather than a copy of Kavita's: there a single
	// request carries the whole library and needs the long budget, here a request
	// carries at most 200 cards and only the SEQUENCE of them is long. Without
	// the outer deadline a library that keeps answering full pages could run
	// unbounded; without the inner one a single wedged page would consume the
	// whole walk's budget silently.
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.List)
	defer cancel()

	for n := 0; n < maxBookPages; n++ {
		var body booksPageDTO
		err := c.do(ctx, request{
			op: op, method: http.MethodPost, path: path,
			body: bookQueryRequest{
				Sort:       fullWalkSort,
				Pagination: bookPagination{Page: n, Size: BookPageSize},
			},
		}, &body)
		if err != nil {
			return page, err
		}
		page.Pages++
		page.Total = body.Total

		for _, card := range body.Items {
			if err := fn(toBook(card)); err != nil {
				return page, err
			}
			page.Count++
		}
		if len(body.Items) < BookPageSize {
			return page, nil
		}
	}
	return page, &APIError{
		Op: op, Method: http.MethodPost, Path: path,
		Message: fmt.Sprintf("library %d still had a full page after %d pages of %d; "+
			"BookOrbit refuses an offset past %d rows, so a walk this long cannot be finished",
			libraryID, maxBookPages, BookPageSize, maxBookPages*BookPageSize),
		Err: ErrUnexpectedStatus,
	}
}
