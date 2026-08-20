package bookorbit

import (
	"context"
	"net/http"
	"strconv"
)

// The completeness probe — GET /api/v1/libraries/{id}/stats.
//
// # What it is for, and why it is a file of its own
//
// It is not a catalogue read. Nothing it returns is written to a work, an
// edition or a library row: its single purpose is to answer "is the book list
// this credential just walked the WHOLE library, or did a content filter short
// it?" — the question catalogue.go's Libraries comment calls measurable rather
// than guessable, now measured.
//
// # The asymmetry the measurement rests on, verified at 73b7877d
//
// Two counts over the same table with the same status predicate, differing in
// exactly one term:
//
//	LibraryRepository.findAllForUser  (library.repository.ts:30-51)
//	  bookCount = count(books.id)
//	  … leftJoin(books, and(books.libraryId = libraries.id,
//	                        books.status = 'present',
//	                        ...buildContentFilterClauses(user.contentFilters)))
//
//	LibraryRepository.getStats        (library.repository.ts:150-178)
//	  totalBooks = count(*)
//	  … from(books).where(and(books.libraryId = :id, books.status = 'present'))
//
// getStats takes NEITHER a user NOR a filter set — it is a bare `(libraryId)`
// method, and LibraryService.getStats (library.service.ts:298-309) passes it
// nothing else. So `totalBooks - bookCount` is exactly the number of present
// books the credential's content filter hid: a subtraction rather than an
// inference.
//
// ⚠️ NO STATUS RULE IS SENT AND NONE IS NEEDED, AND THE PAGED WALK CARRIES A
// DIFFERENT ONE. `status = 'present'` is applied SERVER-SIDE on both of the
// selects above and cannot be turned off from a client. The paged read's
// predicate is not absent, it is weaker — a distinction that is easy to lose
// because it is TRUE OF THE QUERY BUILDER AND FALSE OF THE READ. The builder
// adds no status term (book-query-builder.service.ts:55-90) unless the caller
// supplies an `isPresent` status rule (statusRuleToSql:789), but the repository
// wraps whatever it built: BookRepository.visibleWhere
// (book.repository.ts:530-532) ands `books.status <> 'processing'` onto
// findCards's `.where` (:574) AND onto its `total` (:579, via countWhere).
// `books_status_chk` admits exactly ('present','missing','processing'), so the
// walk returns PRESENT AND MISSING rows and excludes processing. A shortfall
// computed against StreamBooks's `total` therefore still needs the `isPresent`
// rule supplied — without it, it subtracts a present-only count from a
// present-plus-missing one. That is exactly why this pairs stats against the
// LIBRARY LISTING's bookCount rather than against the walk, and the correction
// above does not touch that: both sides of THIS pair carry `status = 'present'`
// by construction, so the subtraction this file exists for stands and there is
// no predicate for a caller to forget.
//
// # The upstream dependency, named rather than assumed
//
// This measurement rests on a property of somebody else's API that nobody
// promised us: `@Get(':id/stats')` carries `@RequireLibraryAccess('viewer')` and
// NO `@RequirePermission` (library.controller.ts:108-112), so a shared viewer
// account with an empty permission set reaches it today. ⚠️ IF BOOKORBIT ADDS A
// PERMISSION GUARD, this call starts answering 403 and the check must degrade to
// "completeness unverified" — never to "complete". internal/libsync's
// completeness pass is built around that degradation, and ADR-0061 records the
// dependency with the guard-later scenario as its named degradation condition.

// LibraryStats is the allowlisted projection of BookOrbit's LibraryStats
// (packages/types/src/library.ts:90-94).
//
// ONE FIELD OF THREE. `totalSizeBytes` and `formatCounts` are on the wire and
// are deliberately not carried: media_file is a later slice's, so a byte total
// would go stale before it had a reader, and a per-format histogram is a second
// opinion about edition.format that nothing consumes. Adding either is a
// deliberate edit rather than a default, because encoding/json drops every key
// with no field here.
type LibraryStats struct {
	// TotalBooks is `count(*) WHERE library_id = ? AND status = 'present'`,
	// computed with no user and no content filter. It is the DENOMINATOR that
	// the library listing's bookCount is not.
	TotalBooks int64 `json:"total_books"`
}

// libraryStatsDTO is the wire shape. The pointer keeps "absent" distinguishable
// from "zero" at the decode boundary — see LibraryStats below for why that
// distinction is load-bearing here and nowhere else in this package.
type libraryStatsDTO struct {
	TotalBooks *int64 `json:"totalBooks"`
}

// LibraryStats calls GET /api/v1/libraries/{id}/stats.
//
// ONE REQUEST, no paging, called once per library per import and never on a
// render path (principle 1). Its caller is internal/libsync's completeness pass,
// which reads every error here as "unverified" and never as a reason to fail an
// import.
//
// A BODY WITH NO `totalBooks` KEY IS AN ERROR RATHER THAN A ZERO, and this is
// the one place in this package where an absent number must not flatten to the
// zero value. `totalBooks: countRow?.count ?? 0` always emits the key on a
// successful getStats, so an absent one means the route answered something that
// is not this route's shape — and decoding that as 0 would report every book in
// the library as hidden by a filter.
func (c *Client) LibraryStats(ctx context.Context, libraryID int64) (LibraryStats, error) {
	const op = "LibraryStats"
	path := apiPrefix + "/libraries/" + strconv.FormatInt(libraryID, 10) + "/stats"

	if libraryID < 0 {
		return LibraryStats{}, &APIError{
			Op: op, Method: http.MethodGet, Path: path,
			Message: "library id must not be negative", Err: ErrInvalidRequest,
		}
	}

	var raw libraryStatsDTO
	if err := c.do(ctx, request{op: op, method: http.MethodGet, path: path}, &raw); err != nil {
		return LibraryStats{}, err
	}
	if raw.TotalBooks == nil {
		return LibraryStats{}, &APIError{
			Op: op, Method: http.MethodGet, Path: path,
			Message: "response carries no totalBooks", Err: ErrDecode,
		}
	}
	// A negative count is not a count. It cannot come out of `count(*)::int`, so
	// a body carrying one did not come from getStats, and the subtraction it
	// would feed would OVERSTATE the shortfall.
	if *raw.TotalBooks < 0 {
		return LibraryStats{}, &APIError{
			Op: op, Method: http.MethodGet, Path: path,
			Message: "totalBooks is negative", Err: ErrDecode,
		}
	}
	return LibraryStats{TotalBooks: *raw.TotalBooks}, nil
}
