package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// §17.2's per-type library grid, and §17.8's library scope chip, as one read:
// GET /api/v1/library. It is Block C's corpus filtered and re-ordered, and it
// returns Block C's row — see RecentWork, which this deliberately does not
// duplicate.
//
// WHICH SIDE OF THE ACCESS-SCOPE RULE THIS IS ON. store.go rule 2: every read
// that aggregates across instances takes a Scope IN THE SIGNATURE, second, and
// filters on it. This one does, through the same Scope.workVisibilityPredicate
// recent.go uses, and TestBrowseWorksScopeActuallyFilters breaks the predicate
// and watches the assertion catch it rather than trusting that the parameter is
// threaded.
//
// IT IS A LOCAL READ AND NOTHING ELSE (principle 1). One statement per page —
// two on the one added_at/undated boundary ListWorks documents — against the
// local file, no upstream call, no metadata provider, no image fetch.
//
// WHY IT IS A NEW FILE AND NOT AN EXTENSION OF recent.go. recent.go is ONE query
// with ONE order and no filters — a fact about the statement that file holds
// today, not a ceiling any document sets on it. ⚠️ This paragraph used to add
// "and §17.2 is emphatic that it stays that way: a sixth media type adds rows to
// it, never a sixth region", which pins the wrong half of §17.2 to the wrong
// claim: §17.2 is emphatic about the SHAPE of Home's Block C — one table rather
// than one strip per type, so "a sixth type adds rows to an existing list rather
// than a sixth region to scan" — and of that table it requires, in the same
// sentence, that "it sorts, it filters, it Ctrl+Fs (§4.5)". ADR-0028 puts Block
// C's scope on the `?lib=` chip. Neither declines a filter.
//
// The reason stands without them, and it is this file's own: this read is the
// opposite shape — three orders, two filters and a cursor codec per order — and
// growing recent.go into it would make the unfiltered single-order statement an
// argument-dependent special case of the filtered one. The two share what they
// genuinely share (the row type, the scope predicate, the kind allowlist, the
// media-type mapping) by calling it, not by merging.

// WorksSort is one of the three orders this read serves.
//
// THE WIRE VOCABULARY IS `library.default_sort`'S OWN, VERBATIM — migration
// 0005 spells it `CHECK (default_sort IN ('sort_title','added_at','year',
// 'popularity'))` — and these constants are those strings. That is the point
// rather than a coincidence: a library carries a default sort in the schema, a
// client that renders that library has to be able to ask for it, and inventing
// a second spelling ("newest", "az") for the same four states is the
// two-vocabularies-one-name shape this repo has already been bitten by.
//
// ⚠️ 'year' IS THE ONE MEMBER OF THAT CHECK THAT IS NOT HERE, and its absence is
// a fact about the schema rather than an oversight. `work.year` has NO INDEX at
// all — internal/db/testdata/schema.sql carries six indexes on `work` and none
// of them leads with `year` — so a year-ordered page is a temp b-tree over the
// whole filtered corpus on every page, which is precisely what ADR-0051 refuses
// to put on a render path. A library whose default_sort is 'year' is therefore
// legal in the schema and unservable by this read; ListWorks returns
// ErrUnservableSort for it, and the caller renders that as a 400 naming the
// missing index. Serving it is a new migration (an `ix_work_kind_year`, or a
// `(year, id)` index if the unfiltered case is ever wanted), not an edit here.
type WorksSort string

const (
	// WorksSortAdded is the default: `added_at DESC, id DESC` off ix_work_added,
	// the same order and the same index Block C walks.
	WorksSortAdded WorksSort = "added_at"

	// WorksSortTitle is `sort_title ASC, id ASC` off ix_work_kind_sort. ⚠️ IT
	// REQUIRES A MEDIA TYPE THAT RESOLVES TO EXACTLY ONE work.kind, because
	// that index is `(kind, sort_title, id)` and SQLite cannot supply ORDER BY
	// from an index whose leading column is constrained by IN — the same
	// property store.go's userPredicate note records for ix_prov_user_grabbed,
	// and it is measured, not assumed. See ListWorks for what that costs
	// `media_type=music`.
	WorksSortTitle WorksSort = "sort_title"

	// WorksSortPopularity is `popularity DESC, id DESC` off ix_work_pop.
	// `work.popularity` is `REAL NOT NULL DEFAULT 0`, so unlike added_at it has
	// no undated tail and unlike sort_title it needs no kind equality.
	WorksSortPopularity WorksSort = "popularity"
)

// ErrUnservableSort is returned when the requested (sort, media type) pair has
// no index behind it. It is a distinct error rather than a generic one because
// the caller renders it as a 400 with an action, and a 500 would claim the
// server is broken when what is actually true is that the request asked for an
// order the schema cannot produce cheaply. The two reachable causes are named
// on WorksSortTitle and on the 'year' note above.
var ErrUnservableSort = fmt.Errorf("store: this sort order has no index behind it")

// ErrCursorSortMismatch is returned when a cursor minted under one sort order is
// replayed under another.
//
// IT IS ENFORCED HERE, IN THE STORE, and not only at the HTTP boundary. A
// popularity cursor replayed on the added_at order is not a parse failure — both
// tokens decode — it is a page that silently starts in the wrong place and
// either skips rows or repeats them for ever. The check cannot be forgotten by a
// second caller if it lives beside the statement the cursor is bound into.
var ErrCursorSortMismatch = fmt.Errorf("store: this cursor was minted under a different sort order")

// WorksFilter is what narrows the corpus. The zero value is "every media type
// Block C admits, every library", newest first — i.e. Block C's own corpus, in
// Block C's own order.
type WorksFilter struct {
	// MediaType is one of §17.2's six navigation-enum values (the MediaType*
	// constants in recent.go), or "" for all six. It is NOT work.kind: two of
	// the six are a (kind, formats) pair, which is why this resolves through
	// browseKinds and the audiobook predicate rather than through a column
	// comparison. An unrecognised value is an error, never a silently
	// unfiltered page — see ListWorks.
	MediaType string

	// LibraryIDs scopes the page to §17.8's library chip. Empty is "every
	// library", NOT "no library": the chip's cleared state is the whole
	// catalogue, and a caller that means "nothing" passes a scope that says so.
	// Ids rather than slugs, because a slug is a URL identity that belongs to
	// the HTTP boundary — LibraryIDsBySlug is the resolver, and it is where an
	// unknown slug becomes an error instead of a wider page.
	LibraryIDs []int64

	// Sort is the order. The zero value ("") means WorksSortAdded.
	Sort WorksSort
}

// sort returns the effective order, resolving the zero value.
func (f WorksFilter) sort() WorksSort {
	if f.Sort == "" {
		return WorksSortAdded
	}
	return f.Sort
}

// browseKinds maps §17.2's navigation enum onto the work.kind values a media
// type covers, and it is the INVERSE of recent.go's mediaTypeOf.
//
// The two must agree, and that they do is executed rather than asserted:
// TestBrowseFilterAgreesWithMediaTypeOf runs every media type through this read
// and through ListRecentWorks + mediaTypeOf and requires the same rows. A filter
// that admits a work the Type column then renders as a different media type is
// the exact defect this pairing can produce, and it would look like a rendering
// bug rather than a query bug.
//
// The second return value reports whether the media type is known at all. It is
// a bool rather than an empty slice so that "" (no filter) and "moovies" (a
// typo) cannot be the same answer.
func browseKinds(mediaType string) ([]string, bool) {
	switch mediaType {
	case "":
		return recentWorkKinds, true
	case MediaTypeMovies:
		return []string{"movie"}, true
	case MediaTypeTV:
		return []string{"series"}, true
	case MediaTypeMusic:
		return []string{"artist", "album"}, true
	case MediaTypeComics:
		return []string{"comic"}, true
	case MediaTypeEbooks, MediaTypeAudiobooks:
		return []string{"book"}, true
	}
	return nil, false
}

// Bounds on ListWorks. They are Block C's bounds, on purpose: §17.2's grid and
// §17.2's recently-added table are two windows over one corpus, and a client
// that has learned one endpoint's paging rule should not have to learn a second
// (docs/reference/http-api.md §1.2 documents the rule once). The default is the
// larger-than-§17.5 50 for the same reason recent.go's is — §17.1's floor is 25
// items above the fold — and a grid client that wants §13's 100-item budget page
// asks for 100.
const (
	BrowseWorksDefaultLimit = RecentWorksDefaultLimit
	BrowseWorksMaxLimit     = RecentWorksMaxLimit
)

// browseWorksSQLMaxRows is the clamp browseWorksSQL applies: the page cap PLUS
// ONE, for recentWorksSQLMaxRows's reason — the +1 is ListWorks's has-a-next-
// page probe row, and clamping it away would report "no next page" whenever the
// last page is exactly full.
const browseWorksSQLMaxRows = BrowseWorksMaxLimit + 1

// WorksCursor is one keyset position in one of the three orders.
//
// IT CARRIES ITS OWN SORT, which is the whole reason it is not RecentWorksCursor
// with a field added. The three orders walk three different indexes with three
// different key types, and the same `(value, id)` pair means a different row in
// each. A cursor that did not name its order would decode cleanly under the
// wrong one and mis-page silently — see ErrCursorSortMismatch.
//
// THE UNDATED TAIL IS added_at's ALONE. `work.added_at` is nullable and
// ix_work_added is DESC, so SQLite's NULL-sorts-lowest rule puts the undated
// rows in a TAIL after every dated row that a `(added_at, id) < (?, ?)`
// row-value comparison can never reach — RecentWorksCursor's doc comment carries
// the full argument and it applies here unchanged. `sort_title` is NOT NULL and
// `popularity` is NOT NULL DEFAULT 0, so neither of those two orders has a tail
// and NullTail is meaningless under them; ListWorks rejects it rather than
// ignoring it, because a caller that sets it has misunderstood something.
type WorksCursor struct {
	// Sort is the order this position was minted in. The zero value ("") means
	// WorksSortAdded, matching WorksFilter.
	Sort WorksSort

	// Value is the sort key of the last row on the previous page, rendered as
	// text: the added_at string, the sort_title, or the popularity formatted by
	// strconv.FormatFloat(…, 'g', -1, 64) — the shortest form that round-trips
	// a float64 exactly. It is invalid only in the undated tail.
	Value sql.NullString

	// ID is work.id, the tiebreaker. Every one of the three orders needs one:
	// added_at has one-second resolution, two works can share a sort_title, and
	// popularity is 0 for everything nothing has scored.
	ID int64

	// NullTail selects added_at's undated tail. Separate from "Value is not
	// valid" so the zero cursor stays unambiguously "the first page" — a caller
	// that forgets to set it asks for page one, which is wrong-but-visible,
	// rather than for the tail, which is wrong-and-silent.
	NullTail bool
}

func (c WorksCursor) sort() WorksSort {
	if c.Sort == "" {
		return WorksSortAdded
	}
	return c.Sort
}

// browseWorksSQL renders the ListWorks statement and its arguments.
//
// It is a function rather than a literal inlined below so the query-plan test
// can EXPLAIN THE STATEMENT THAT SHIPS (docs/DEVELOPMENT.md §11 rule 1: a guard
// that asserts against a hand-copied lookalike is probing a proxy). The limit is
// clamped here, beside the statement it bounds, so a second caller cannot render
// this query with an unbounded LIMIT.
//
// ⚠️ THE UNARY PLUS ON `+w.kind` IS LOAD-BEARING, IS NOT A TYPO, AND IS APPLIED
// TO TWO OF THE THREE ORDERS ONLY. recentWorksSQL carries the full measurement
// and the citation; the short version is that a plain `w.kind IN (…)` makes
// SQLite prefer ix_work_kind_sort for the kind filter and then sort the whole
// result, which is the recently-added view degrading exactly as schema.md §1
// warns. For added_at and popularity the ordering index is ix_work_added /
// ix_work_pop and the kind filter must be disqualified from index use, so the
// plus is there. For sort_title the ordering index IS ix_work_kind_sort and the
// kind equality is what opens it, so the plus must NOT be there — writing it
// would disqualify the very index that supplies the order. Both directions are
// pinned by TestBrowseWorksPlanGuardFires.
//
// ⚠️ THE PLANS BELOW, AND EVERY PLAN GUARD IN browse_test.go, ARE THE NO-ANALYZE
// PLANNER'S. A fresh test database has no `sqlite_stat1`, and the unary plus is
// one of the places where that matters: with statistics present SQLite may reach
// the same plan without it. The binary usually runs WITH statistics, because the
// importer runs ANALYZE after a full import. So these guards pin the planner the
// SUITE sees, which is the conservative one — a plan that holds without
// statistics holds with them — and they are NOT a measurement of production.
// `make bench` is where the with-statistics wall clock lives.
func browseWorksSQL(scope Scope, f WorksFilter, cur WorksCursor, limit int) (string, []any, error) {
	switch {
	case limit <= 0:
		limit = BrowseWorksDefaultLimit
	case limit > browseWorksSQLMaxRows:
		limit = browseWorksSQLMaxRows
	}

	kinds, ok := browseKinds(f.MediaType)
	if !ok {
		// Never a silently unfiltered page. An unknown media type is a caller
		// error the caller renders as a 400 — the same rule
		// DecodeRecentWorksCursor follows for a cursor, and for the same reason:
		// a filter that silently widens turns a typo into "your library screen
		// shows everything", which looks like it worked.
		//
		// The value is NOT echoed into the error. It is caller-supplied text and
		// the caller renders the message.
		return "", nil, fmt.Errorf("store: unknown media type")
	}

	sort := f.sort()
	switch sort {
	case WorksSortAdded, WorksSortTitle, WorksSortPopularity:
	case "year":
		// `year` IS a legal `library.default_sort` value — migration 0005's
		// CHECK admits it — and it is the one member of that CHECK this read
		// cannot serve: `work.year` has no index at all, so the order would be a
		// temp b-tree over the whole filtered corpus on every page. Named
		// explicitly rather than falling into the "unknown sort" arm below,
		// because a caller reading a library's own default_sort back to this
		// endpoint deserves the real reason.
		return "", nil, ErrUnservableSort
	default:
		// Never a silent fallback to the default order. A misspelt sort that
		// quietly served added_at would look like the parameter worked.
		return "", nil, fmt.Errorf("store: unknown sort order")
	}
	if sort != cur.sort() && (cur.Value.Valid || cur.NullTail) {
		return "", nil, ErrCursorSortMismatch
	}
	if cur.NullTail && sort != WorksSortAdded {
		return "", nil, fmt.Errorf(
			"store: only the %s order has an undated tail", WorksSortAdded)
	}
	if sort == WorksSortTitle && len(kinds) != 1 {
		// ix_work_kind_sort is (kind, sort_title, id) and needs the kind as an
		// EQUALITY. media_type=music resolves to two kinds and the unfiltered
		// case to six, and for those the ordering would be a temp b-tree over
		// the whole corpus on every page.
		return "", nil, ErrUnservableSort
	}

	args := make([]any, 0, len(kinds)+len(f.LibraryIDs)+len(scope.InstanceIDs)+8)

	// The Ebooks/Audiobooks split for the SELECT list, resolved server-side
	// because §17.2 says the Tier 1 client index ships no format at all. This is
	// recentWorksSQL's expression verbatim and it feeds mediaTypeOf, which is
	// what makes the Type column of a browse row and of a Block C row the same
	// answer for the same work.
	args = append(args, editionFormatAudiobook)

	kindPred := "+w.kind IN (" + placeholders(len(kinds)) + ")"
	if sort == WorksSortTitle {
		// Exactly one kind, checked above, and NO unary plus — see the header.
		kindPred = "w.kind = ?"
	}
	for _, k := range kinds {
		args = append(args, k)
	}

	formatPred := ""
	switch f.MediaType {
	case MediaTypeAudiobooks:
		formatPred = "\n\t\t   AND " + browseAudiobookPredicate
		args = append(args, editionFormatAudiobook, editionFormatAudiobook)
	case MediaTypeEbooks:
		formatPred = "\n\t\t   AND NOT " + browseAudiobookPredicate
		args = append(args, editionFormatAudiobook, editionFormatAudiobook)
	}

	scopePred, scopeArgs := scope.workVisibilityPredicate("w.id")
	args = append(args, scopeArgs...)

	libPred := ""
	if len(f.LibraryIDs) > 0 {
		// ADR-0051: WORK-DRIVEN EXISTS, never a join to library_member. The
		// ordering comes off `work`, the membership table is only ever probed,
		// and a work that is a member of two of the named libraries — which is
		// §17.8's flagship shape, one upstream library offered as Ebooks AND as
		// Audiobooks, so one work carries ONE MEMBERSHIP ROW PER LIBRARY while
		// a browse row is work-keyed — comes back ONCE. A join returns it once
		// per matching membership row. ⚠️ The duplicate is per-LIBRARY, not
		// per-edition: `library_member`'s key carries `edition_id`, but the only
		// production writer hardcodes the `0` "whole work" sentinel
		// (catalogue.go's item pass; REVIEW-LOG.md LS-213), so nothing in the
		// tree files one work twice in ONE library. A join cannot promise
		// either. See the ADR for the topology that would reopen it.
		libPred = "\n\t\t   AND EXISTS (SELECT 1 FROM library_member lm\n" +
			"\t\t                        WHERE lm.work_id = w.id\n" +
			"\t\t                          AND lm.library_id IN (" +
			placeholders(len(f.LibraryIDs)) + "))"
		for _, id := range f.LibraryIDs {
			args = append(args, id)
		}
	}

	// keyExpr is the LAST column of the SELECT list and it is the sort key of
	// the row, whatever the order is. It exists so that minting the next
	// cursor is a read of the page's last row rather than a second opinion
	// about what the ORDER BY did: RecentWork carries added_at and nothing
	// else, and §4.5's "never load full item records" rule is why it does not
	// carry sort_title, popularity and every other column a future order might
	// key on. One column, named by the order, keeps that rule and keeps the
	// cursor honest.
	//
	// It is scanned as TEXT for all three. `popularity` is REAL, and
	// database/sql renders a float64 into a string with
	// strconv.FormatFloat(f, 'g', -1, 64) — the shortest form that parses back
	// to the identical float64, which is exactly what a keyset boundary needs.
	keyExpr, cursorPred, orderBy := "", "", ""
	switch sort {
	case WorksSortTitle:
		keyExpr, orderBy = "w.sort_title", "w.sort_title ASC, w.id ASC"
		if cur.Value.Valid {
			cursorPred = "\n\t\t   AND (w.sort_title, w.id) > (?, ?)"
			args = append(args, cur.Value.String, cur.ID)
		}
	case WorksSortPopularity:
		keyExpr, orderBy = "w.popularity", "w.popularity DESC, w.id DESC"
		if cur.Value.Valid {
			pop, err := strconv.ParseFloat(cur.Value.String, 64)
			if err != nil {
				return "", nil, fmt.Errorf("store: cursor has no usable popularity")
			}
			cursorPred = "\n\t\t   AND (w.popularity, w.id) < (?, ?)"
			args = append(args, pop, cur.ID)
		}
	default:
		keyExpr, orderBy = "w.added_at", "w.added_at DESC, w.id DESC"
		switch {
		case cur.NullTail:
			// `added_at IS NULL` reads as an equality on the index's leading
			// column, so this stays a seek rather than a walk from the top.
			cursorPred = "\n\t\t   AND w.added_at IS NULL AND w.id < ?"
			args = append(args, cur.ID)
		case cur.Value.Valid:
			cursorPred = "\n\t\t   AND (w.added_at, w.id) < (?, ?)"
			args = append(args, cur.Value.String, cur.ID)
		}
	}

	args = append(args, limit)

	return `
		SELECT w.id, w.kind, w.title, w.year, w.added_at,
		       w.have_count, w.want_count, w.availability,
		       (SELECT MIN(e.format = ?) FROM edition e WHERE e.work_id = w.id),
		       ` + PosterKeyExpr + `,
		       ` + keyExpr + `
		  FROM work w
		 WHERE w.deleted_at IS NULL
		   AND ` + kindPred + formatPred + `
		   AND ` + scopePred + libPred + cursorPred + `
		 ORDER BY ` + orderBy + `
		 LIMIT ?`, args, nil
}

// browseAudiobookPredicate is §17.2 row 5 — "every edition of this work is an
// audiobook" — as a WHERE-clause filter.
//
// ⚠️ IT IS EXACTLY `MIN(e.format = 'audiobook') IS 1`, THE EXPRESSION THE SELECT
// LIST COMPUTES, AND THE EQUIVALENCE IS THE POINT. SQL's MIN ignores NULLs, so
// that expression is 1 when at least one edition compares equal AND no edition
// compares unequal, where a NULL format compares to neither. Written out:
//
//	EXISTS (format = 'audiobook')  AND  NOT EXISTS (format IS NOT NULL AND format <> 'audiobook')
//
// ⚠️ THE SECOND HALF IS `format IS NOT NULL AND format <> ?` AND NOT THE TIDIER
// `format IS NOT ?`. They differ on exactly one row shape and it is a reachable
// one: a book with an M4B edition and a second edition whose format the upstream
// did not report. Under MIN that work is an audiobook (the NULL is ignored);
// under `IS NOT ?` the NULL row satisfies the NOT EXISTS and the work would drop
// out of the Audiobooks grid while still rendering "Audiobooks" in its own Type
// cell. TestBrowseFilterAgreesWithMediaTypeOf has that work in its corpus.
//
// WHY NOT JUST REUSE THE MIN SUBQUERY AS THE PREDICATE. Because the two
// EXISTS halves are what ix_edition_format (migration 0009) can seek on, and the
// selective half runs first: `format = ? AND work_id = ?` is a two-column
// COVERING seek, so most candidate books are rejected without reading an edition
// row at all. The MIN form is a work_id seek plus a row fetch per edition, for
// every candidate, on every page. Measured plans are in browse_test.go.
const browseAudiobookPredicate = `(EXISTS (SELECT 1 FROM edition ea
		                            WHERE ea.format = ? AND ea.work_id = w.id)
		        AND NOT EXISTS (SELECT 1 FROM edition en
		                         WHERE en.work_id = w.id
		                           AND en.format IS NOT NULL AND en.format <> ?))`

// ListWorks reads one keyset page of §17.2's library grid.
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL, both halves, and it is the SECOND
// parameter per store.go rule 2. A scope a query accepts and never filters on is
// indistinguishable from no scope at all.
//
// `deleted_at IS NULL` is in the query AND in the partial predicate of all three
// ordering indexes. That is not belt-and-braces: without the WHERE clause the
// read returns tombstoned rows, and without the partial index it reads them from
// the index and filters afterwards, so the 7-day tombstone window degrades
// exactly this view (schema.md §1).
//
// THE TWO-STATEMENT BOUNDARY, WHICH EXISTS FOR THE added_at ORDER ONLY. added_at
// is nullable, so the undated rows form a tail the dated cursor predicate cannot
// reach — see WorksCursor. When a dated page comes back short, the rest of the
// page is in that tail, so this issues the tail query for the remainder. It
// happens at most once per page, only at that one boundary, and only under
// WorksSortAdded; a caller never sees it, and both statements have a pinned
// plan. sort_title and popularity are NOT NULL columns and take one statement.
//
// ⚠️ WHAT `media_type=music` CANNOT DO, stated here because it is the one hole a
// caller will meet. §17.2's Music row is `kind IN ('artist','album')` — two
// kinds — and ix_work_kind_sort is `(kind, sort_title, id)`, so an alphabetical
// music page cannot come off an index: SQLite cannot supply ORDER BY from an
// index whose leading column is constrained by IN. ListWorks returns
// ErrUnservableSort rather than serving a temp b-tree over every artist and
// album in the library, which the caller renders as a 400. The two honest fixes
// are a migration (an index that does not lead with `kind`) or splitting the
// Music grid into Artists and Albums, and both are decisions §17.2 owns rather
// than this function. added_at and popularity are unaffected and are what the
// Music grid uses today.
//
// The second return value is the cursor for the NEXT page, and it is invalid
// (ok=false, i.e. no Value and no NullTail) when this page is the last one.
func (s *Store) ListWorks(
	ctx context.Context, scope Scope, f WorksFilter, cur WorksCursor, limit int,
) ([]RecentWork, WorksCursor, error) {
	switch {
	case limit <= 0:
		limit = BrowseWorksDefaultLimit
	case limit > BrowseWorksMaxLimit:
		limit = BrowseWorksMaxLimit
	}

	// One extra row is read so "is there a next page" is an observation rather
	// than a guess. A cursor minted from a full page that happened to be the
	// last one sends the client back for an empty response, and an empty
	// response after a "Load more" is indistinguishable from a broken one.
	out, err := s.browseWorksPage(ctx, scope, f, cur, limit+1)
	if err != nil {
		return nil, WorksCursor{}, err
	}

	sort := f.sort()
	if sort == WorksSortAdded && len(out) <= limit && cur.Value.Valid && !cur.NullTail {
		// The dated range is exhausted; the rest of the page is in the undated
		// tail. The first page (a zero cursor) needs no handoff — with no cursor
		// predicate the ordered index walk crosses into the tail on its own —
		// and a tail cursor needs none either, being already there.
		tail, err := s.browseWorksPage(ctx, scope, f,
			WorksCursor{Sort: sort, NullTail: true, ID: recentWorksTailStart}, limit+1-len(out))
		if err != nil {
			return nil, WorksCursor{}, err
		}
		out = append(out, tail...)
	}

	if len(out) <= limit {
		return works(out), WorksCursor{}, nil
	}
	out = out[:limit]
	return works(out), browseNextCursor(sort, out[len(out)-1]), nil
}

// browseRow is one page row plus its sort key. The key does NOT escape this
// package: it is the cursor's raw material and nothing renders it, so putting
// it on RecentWork would add a column to a wire-adjacent struct for the benefit
// of one caller.
type browseRow struct {
	work RecentWork
	key  sql.NullString
}

func works(rows []browseRow) []RecentWork {
	out := make([]RecentWork, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.work)
	}
	return out
}

// browseNextCursor mints the position after the last row of a page.
//
// An invalid key means the row has no sort key, which only added_at can produce
// — its column is nullable and its NULLs form the tail. sort_title and
// popularity are NOT NULL, so an invalid key under those orders is impossible;
// it would encode as a tail token, which DecodeWorksCursor then refuses, so the
// failure would be loud rather than a wrong page.
func browseNextCursor(sort WorksSort, last browseRow) WorksCursor {
	return WorksCursor{
		Sort:     sort,
		Value:    last.key,
		ID:       last.work.ID,
		NullTail: !last.key.Valid,
	}
}

func (s *Store) browseWorksPage(
	ctx context.Context, scope Scope, f WorksFilter, cur WorksCursor, limit int,
) ([]browseRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	query, args, err := browseWorksSQL(scope, f, cur, limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("browse library: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]browseRow, 0, limit)
	for rows.Next() {
		var r browseRow
		w := &r.work
		// allAudiobook is the MIN() in the SELECT list: NULL for a work with no
		// editions.
		var allAudiobook sql.NullInt64
		if err := rows.Scan(&w.ID, &w.Kind, &w.Title, &w.Year, &w.AddedAt,
			&w.HaveCount, &w.WantCount, &w.Availability, &allAudiobook,
			&w.PosterKey, &r.key); err != nil {
			return nil, fmt.Errorf("browse library: scan: %w", err)
		}
		w.MediaType = mediaTypeOf(w.Kind, allAudiobook)
		if w.MediaType == "" {
			// Unreachable: browseKinds and mediaTypeOf are inverses over the
			// same six kinds. It is an error rather than a dropped row because
			// the two drifting apart is precisely the bug that would otherwise
			// ship a Type column with a blank cell.
			return nil, fmt.Errorf(
				"browse library: work %d has kind %q, which browseKinds admits and "+
					"mediaTypeOf does not map; the two have drifted apart", w.ID, w.Kind)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("browse library: %w", err)
	}
	return out, nil
}

// EncodeWorksCursor renders a keyset position as one opaque-looking token.
//
// IT IS AN ENCODING, NOT A SECRET, for EncodeRecentWorksCursor's reasons, which
// are written out in full there and are not repeated: the payload carries a sort
// key and a work.id, both of which the row it came from already published, and
// base64url is for TRANSPORT because an added_at contains a space.
//
// THE SHAPE EXTENDS BLOCK C's BY ONE CHARACTER — the sort discriminator — and
// keeps '1' as the version prefix:
//
//	1 a v <id> . <added_at>    added_at, inside the dated range
//	1 a n <id>                 added_at, inside the undated tail
//	1 t v <id> . <sort_title>  sort_title
//	1 p v <id> . <popularity>  popularity
//	│ │ │
//	│ │ └── state: v = the value is present, n = the undated tail
//	│ └──── sort:  a = added_at, t = sort_title, p = popularity
//	└────── version
//
// THE ID COMES FIRST AND THE VALUE LAST because the value is the only field that
// can contain a '.', and a sort_title certainly can. The decoder splits on the
// FIRST dot, so everything after it is the value verbatim, however many dots it
// holds.
//
// ⚠️ THESE TOKENS ARE DELIBERATELY NOT INTERCHANGEABLE WITH BLOCK C's. Block C
// mints `1d…` / `1n…` (EncodeRecentWorksCursor) and this mints `1a…` / `1t…` /
// `1p…`, so neither decoder accepts the other's token — which is the property
// item 2 of this read's brief asks for, applied to the endpoint boundary as well
// as to the sort boundary. Two endpoints with two orders and one token format is
// how a cursor gets replayed across a corpus it was not minted for, and the
// failure is a silently wrong page rather than an error.
func EncodeWorksCursor(c WorksCursor) string {
	var sortChar byte
	switch c.sort() {
	case WorksSortTitle:
		sortChar = 't'
	case WorksSortPopularity:
		sortChar = 'p'
	default:
		sortChar = 'a'
	}
	payload := "1" + string(sortChar) + "n" + strconv.FormatInt(c.ID, 10)
	if !c.NullTail {
		payload = "1" + string(sortChar) + "v" + strconv.FormatInt(c.ID, 10) + "." + c.Value.String
	}
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// DecodeWorksCursor parses a token minted by EncodeWorksCursor.
//
// STRICT, AND ON PURPOSE, exactly as DecodeRecentWorksCursor is: a cursor that
// fails to parse is a client error the caller renders as a 400, and it is never
// silently treated as "start from the beginning", because that turns a stale
// bookmark into an infinite Load-more loop over page one.
//
// The value half is NOT re-validated against its column type here — it is
// compared as text or parsed as a float where it is BOUND, in browseWorksSQL,
// and an unparseable popularity fails there rather than selecting an arbitrary
// page. The token is never echoed into an error: it is caller-supplied text and
// the caller renders the message.
func DecodeWorksCursor(token string) (WorksCursor, error) {
	if token == "" {
		return WorksCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return WorksCursor{}, fmt.Errorf("store: cursor is not base64url: %w", err)
	}
	payload := string(raw)
	if len(payload) < 4 || payload[0] != '1' {
		return WorksCursor{}, fmt.Errorf("store: cursor is not a library-browse cursor")
	}

	var sort WorksSort
	switch payload[1] {
	case 'a':
		sort = WorksSortAdded
	case 't':
		sort = WorksSortTitle
	case 'p':
		sort = WorksSortPopularity
	default:
		return WorksCursor{}, fmt.Errorf("store: cursor names no sort order this endpoint serves")
	}

	rest := payload[3:]
	switch payload[2] {
	case 'n':
		if sort != WorksSortAdded {
			// Only added_at has a tail. A tail token under another order was
			// either hand-made or minted by a version that ordered differently,
			// and both must fail loudly.
			return WorksCursor{}, fmt.Errorf("store: only the %s order has an undated tail", WorksSortAdded)
		}
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil || id <= 0 {
			return WorksCursor{}, fmt.Errorf("store: cursor has no usable id")
		}
		return WorksCursor{Sort: sort, NullTail: true, ID: id}, nil
	case 'v':
		dot := strings.IndexByte(rest, '.')
		if dot <= 0 || dot == len(rest)-1 {
			return WorksCursor{}, fmt.Errorf("store: cursor is malformed")
		}
		id, err := strconv.ParseInt(rest[:dot], 10, 64)
		if err != nil || id <= 0 {
			return WorksCursor{}, fmt.Errorf("store: cursor has no usable id")
		}
		return WorksCursor{
			Sort:  sort,
			Value: sql.NullString{String: rest[dot+1:], Valid: true},
			ID:    id,
		}, nil
	}
	return WorksCursor{}, fmt.Errorf("store: cursor is not a library-browse cursor")
}

// LibraryIDsBySlug resolves §17.8's `?lib=` slugs to the ids WorksFilter takes.
//
// WHY IT IS A SEPARATE CALL AND NOT A COLUMN IN THE BROWSE STATEMENT. The slug
// is a URL identity — migration 0005 calls it exactly that, "URL identity for
// ?lib=", allocated once and durable across a rename — so joining `library` into
// the grid query per page would make every page pay for a lookup whose answer
// cannot change between pages of one walk. Resolving once and paging on ids is
// also what lets an unknown slug be an error before any rows are read.
//
// AN UNKNOWN SLUG IS AN ERROR, NEVER A DROPPED FILTER. Dropping it silently
// widens the page — and if every slug is unknown the filter disappears entirely
// and the user gets the whole catalogue on a screen they scoped. That is the
// same failure mode browseWorksSQL refuses for an unknown media type. The error
// names how many slugs did not resolve and NOT which ones: a caller cannot
// probe another user's library names by watching the message change.
//
// THE RESERVED Unfiled LIBRARY IS EXCLUDED IN THE SQL, matching listLibrariesSQL
// and migration 0005's own statement that it is "never offered in the scope
// chip". `?lib=unfiled` therefore resolves to nothing and is refused as unknown,
// which is the correct answer rather than an accident.
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL: the user half through the canonical
// `user_id IN (0, :uid)` predicate, and the instance half through
// libraryVisibilityPredicate — the same one ListLibraries uses, for the same
// existence-oracle reason. A slug the caller cannot see is indistinguishable
// from a slug that does not exist, which is the property that makes the count in
// the error message safe to publish.
func (s *Store) LibraryIDsBySlug(
	ctx context.Context, scope Scope, slugs []string,
) ([]int64, error) {
	if len(slugs) == 0 {
		return nil, nil
	}

	args := make([]any, 0, len(slugs)+len(scope.InstanceIDs)+4)
	args = append(args, UnfiledLibraryID)

	userPred, userArgs := scope.userPredicate("l.user_id")
	args = append(args, userArgs...)

	for _, sl := range slugs {
		args = append(args, sl)
	}

	visPred, visArgs := scope.libraryVisibilityPredicate("l.id")
	args = append(args, visArgs...)

	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT l.id, l.slug FROM library l
		 WHERE l.id <> ?
		   AND `+userPred+`
		   AND l.slug IN (`+placeholders(len(slugs))+`)
		   AND `+visPred+`
		 ORDER BY l.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("resolve library slugs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make(map[string]bool, len(slugs))
	var out []int64
	for rows.Next() {
		var id int64
		var slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return nil, fmt.Errorf("resolve library slugs: scan: %w", err)
		}
		found[slug] = true
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("resolve library slugs: %w", err)
	}

	missing := 0
	for _, sl := range slugs {
		if !found[sl] {
			missing++
		}
	}
	if missing > 0 {
		return nil, fmt.Errorf("resolve library slugs: %d of %d name no library you can see",
			missing, len(slugs))
	}
	return out, nil
}
