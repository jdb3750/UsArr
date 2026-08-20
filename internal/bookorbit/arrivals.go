package bookorbit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// THE ARRIVALS WALK — channel 3b's wire half: a TIMESTAMP KEYSET over
// books.addedAt, not offset paging.
//
// # The shape, in one place
//
// Every request carries `page: 0` and a filter `after(<cursor>)`, where the
// cursor is the filter value of the NEXT request and advances by the mitigation
// below. There is no offset above zero, ever. The walk stops on a SHORT page.
//
// # Why a keyset rather than the offset paging channel 1 uses
//
// Under offset paging a deletion BELOW the cursor shifts every later row one
// place towards it, so the row now sitting where the cursor already passed is
// never read again. The obvious guard — refuse to advance if upstream's own
// `total` decreased across the walk — does not catch it: one delete plus one
// arrival nets to zero, `total` reads the same before and after, and the sign of
// a NET figure is not evidence about its components.
//
// Under a keyset there is no cursor for a deletion to shift. Membership is
// decided by the filter alone and every request is a fresh top-of-set read of a
// strictly narrower filter. `total` is not read at all.
//
// ⚠️ THE KEYSET IS NOT "STRICTLY BETTER", AND THE PHRASE IS BANNED HERE. The two
// mechanisms fail on DISJOINT inputs: offset paging skips under a concurrent
// deletion and loses nothing to a tie group, while a naive keyset loses nothing
// to deletion and — measured, against a PostgreSQL 16.13 cluster built from
// BookOrbit's own DDL — lost 70 of 330 rows to one tie group, permanently.
// "Strictly better" asserts a containment between two failure sets; check the
// containment before writing it, or write "differently wrong".

// arrivalsFilterOperator is the ONLY operator this channel uses.
//
// 🚩 THE OTHER THREE ARE TRAPS AND ARE NAMED SO NOBODY REACHES FOR ONE.
// books.addedAt supports after / before / between / withinLast server-side, and
// as a capability list that is accurate — as a menu it offers four operators and
// marks none unusable:
//
//   - `after` is a STRICT `>` (BookQueryBuilder emits gt(books.addedAt, …)),
//     which is what makes a re-read of the boundary group possible and a
//     PERMANENT DUPLICATE impossible. It is the one this channel uses.
//   - `between` is INCLUSIVE AT BOTH ENDS (gte AND lte) and needs a second bound
//     UsArr cannot supply in upstream's clock — so an implementer supplies an
//     upper bound from UsArr's own clock, and a local clock running ahead of
//     BookOrbit's then excludes the newest arrivals FOREVER, because the
//     watermark only moves forward. Wrong twice.
//   - `withinLast` is the trap this design's own summary sentence creates. "The
//     walk looks back five minutes" matches an operator in the grammar, and that
//     operator is `books.added_at >= NOW() - (n * INTERVAL '1 day')`: inclusive,
//     evaluated against BOOKORBIT'S NOW(), and it DISCARDS THE CURSOR ENTIRELY.
//   - `before` walks the wrong way.
const arrivalsFilterOperator = "after"

// arrivalsFilterField is books.addedAt's name in FIELD_OPERATORS.
//
// ⚠️ IT IS addedAt AND NOT updatedAt, AND THE TWO ARE NOT THE SAME CHANNEL.
// 3b for this source is ARRIVALS ONLY; edits and deletions are channel 4's and
// nothing here observes them. The updatedAt axis is refused rather than merely
// unused (ADR-0070), so an "improvement" that swaps this constant changes what
// the channel IS.
const arrivalsFilterField = "addedAt"

// bookFilterRule and bookFilterGroup are BookQueryPipe's filter tree, carrying
// exactly the one rule this walk sends.
//
// The tree nests — the validator accepts boolean groups to depth 5 — and this
// channel deliberately uses none of that: one AND group holding one rule is the
// whole filter, so there is no shape here for a later reader to mistake for a
// general query builder.
type bookFilterRule struct {
	Type     string `json:"type"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type bookFilterGroup struct {
	Type  string           `json:"type"`
	Join  string           `json:"join"`
	Rules []bookFilterRule `json:"rules"`
}

// arrivalsSort is the ordering the delta walk asks for.
//
// ⚠️ IT IS A SEPARATE VAR FROM fullWalkSort THOUGH THE TWO ARE CURRENTLY EQUAL,
// and that is deliberate rather than duplication. fullWalkSort's own header
// opens by saying it is NOT the ordering a delta walk asks for, and its body
// argues from OFFSET PAGING — what happens when a mid-walk insert shifts a
// cursor — which does not apply to a keyset at all. Sharing the value would make
// a later edit to channel 1's ordering silently change what this channel is,
// and under a keyset the direction is not a preference: the cursor advances
// UPWARD through the axis, so ASC is the walk.
//
// The ordering is TOTAL because BookSortBuilder.build appends `books.id ASC` as
// an unconditional last tier no matter what was asked for.
var arrivalsSort = []bookSortSpec{{Field: arrivalsFilterField, Dir: "asc"}}

// ArrivalsCursor is the FILTER VALUE OF THE NEXT REQUEST — wire vocabulary, and
// deliberately not the storage word.
//
// ⚠️ IT IS NOT A WATERMARK AND THE TWO NEVER SHARE A TERM (docs/DEVELOPMENT.md
// §11). A watermark is durable, is the maximum observed on a run that completed
// every phase for every library, and lives in one column. A cursor lives for one
// walk, moves once per page, is discarded when the walk ends, and under the
// mitigation below deliberately LAGS the maximum the walk has actually seen. An
// implementer who persists a cursor as a watermark stores a value that is one
// tie group short of what was observed, so the same group is re-read forever
// without anyone being able to see that it is wrong.
//
// Set false is the MEASURED-EMPTY bootstrap: request 1 carries no filter key at
// all. It is not "filter on the zero time", which would be a filter nothing
// observed.
type ArrivalsCursor struct {
	Value string
	Set   bool
}

// ArrivalsInstantLayout is how an instant is rendered ONTO THE WIRE for this
// filter.
//
// It matches what upstream itself emits — assemble-book-cards.ts calls
// Date.toISOString(), i.e. RFC 3339 with exactly three fractional digits and a
// Z — so a value UsArr sends and a value UsArr received are the same spelling of
// the same instant.
//
// ⚠️ IT IS SPELLED OUT HERE RATHER THAN IMPORTED FROM internal/store, though the
// two layouts are byte-identical today. They are two vocabularies: this one is
// what BookOrbit's parser reads, that one is what UsArr's column holds, and the
// day either side changes, the other must not follow silently. The equality is a
// coincidence worth keeping and not a dependency worth creating.
const ArrivalsInstantLayout = "2006-01-02T15:04:05.000Z"

// FormatArrivalsInstant renders an instant for the wire.
//
// .UTC() FIRST, ALWAYS. The layout ends in a LITERAL Z — Go's reference layout
// treats a trailing bare "Z" as a literal, not as a zone field — so a value
// formatted without .UTC() would carry a Z and lie about its zone. That is safe
// here and only here BECAUSE of this call; the storage layout, which has no such
// call site to rely on, uses the Z07:00 zone field instead.
func FormatArrivalsInstant(t time.Time) string { return t.UTC().Format(ArrivalsInstantLayout) }

// ArrivalsWalk is what one library's delta walk observed about ITSELF.
//
// It is a superset of BookPage because a caller needs the same partial-delivery
// facts plus the four the mitigation and the fail-closed rules read.
type ArrivalsWalk struct {
	// Count is how many Books were handed to fn, on success AND on failure.
	Count int

	// Pages is how many HTTP requests the walk made. It is asserted in tests:
	// it is what distinguishes a walk that WEDGED (stopped) from one that never
	// terminated (span the loop bound).
	Pages int

	// ⚠️ THE MAXIMUM ARRIVAL THIS WALK SAW IS DELIBERATELY NOT HERE, though this
	// walk computes one per page and could report it for free. It has exactly
	// one producer — the CALLER's fold over the rows it was handed — because the
	// value ends up in a durable cursor position, and a fact with two producers
	// in two packages is a fact with two spellings the day one of them changes.
	// What this type reports is what only the WALK can know: how many requests
	// it made, and whether it could advance.

	// Wedged reports the residual: the cursor could not be advanced.
	Wedged bool

	// WedgeAt is the instant the walk wedged on, in the rendering that went on
	// the wire. It is UsArr's own formatting of an upstream value, not upstream
	// prose.
	WedgeAt string
}

// StreamBooksSince walks POST /api/v1/libraries/{id}/books filtered to arrivals
// strictly after the cursor, and hands fn each book as a page decodes.
//
// # The loop, exactly
//
//  1. Request with `page: 0`, `size: 200`, sort addedAt ASC, and — unless the
//     cursor is unset — one `after(cursor)` rule.
//  2. Hand every row to fn. Fold the maximum readable arrival; count the rest.
//  3. SHORT PAGE (n < size) ⇒ the filtered set above the cursor is exhausted and
//     this library is DONE. This is the only stop that means "finished", and it
//     is why the maximum a short page produces is safe to advance a watermark
//     from: n < size means the server returned everything matching, so no
//     undelivered row can share the last row's value.
//  4. FULL PAGE ⇒ apply the mitigation and go to 1, or WEDGE.
//
// # ⚠️ THE MITIGATION, which is mandatory and not an optimisation
//
// After a full page, let T be the largest readable arrival on it. The cursor
// advances to the LARGEST ARRIVAL ON THE PAGE STRICTLY LESS THAN T — never to T
// itself.
//
// SOUND, in one sentence: the page is a PREFIX of the ordering, so if a value C
// strictly below the page's last value appears on the page, every row carrying C
// is already on the page; advancing to C therefore excludes only rows already
// delivered, and the cursor never sits at the boundary of a group that might
// continue past the page.
//
// TERMINATES: every row on the page satisfies `addedAt > cursor_prev`, and C is
// one of those values, so C > cursor_prev strictly. The cursor is strictly
// monotone over a finite set of distinct values — and the walk ASSERTS that
// strictness below rather than assuming it.
//
// COSTS one tie group re-read per page, at most size−1 rows, absorbed by the
// idempotent upsert the item apply already relies on far more heavily for its
// lookbehind window.
//
// WHY THIS AND NOT A PRECISION ARGUMENT. The mitigation needs NO assumption
// about now() semantics, insert batching, sub-millisecond remainders or a future
// version bump: it is correct whether values tie in the database, only on the
// wire, or not at all. Without it, measured against 330 rows on PostgreSQL
// 16.13 — 150 ordinary arrivals, 120 backdated to one exact instant, 60 later —
// a naive keyset delivered 260 rows, TERMINATED NORMALLY ON A SHORT PAGE,
// REPORTED SUCCESS, and advanced past the tie instant. 70 rows were never
// delivered by any page, and `added_at > W` excludes them on every future poll
// forever.
//
// # The residual: the wedge
//
// More than `size` rows sharing ONE instant fills a page whose first value
// equals its last, so there is no C and the cursor cannot advance. That is NOT
// data loss — nothing is skipped and nothing is advanced — and it IS an outage
// for everything above that instant until it clears. The walk STOPS and says so;
// it does not loop, and it does not advance anyway. The repair is the full
// import, and the caller is what names it.
//
// The live source of exact ties is not a bulk import sharing a transaction —
// measured, upstream's scanner inserts one book per autocommit statement and
// 1000 of them produced 1000 distinct timestamps. It is the shipped
// `PATCH /books/:id/added-at`, which takes a BARE DATE KEY and stores
// toTimeZoneStartOfDay, i.e. EXACT MIDNIGHT with a zero sub-second component —
// so any number of books given the same date key receive a byte-identical value,
// continuously, at user discretion, in arbitrarily large groups.
//
// # The partial-delivery contract
//
// Identical to StreamBooks's. Count is what already reached fn, on success and
// on failure alike; those calls happened and their effects stand. An error fn
// returns aborts the walk and comes back unwrapped.
func (c *Client) StreamBooksSince(
	ctx context.Context, libraryID int64, cursor ArrivalsCursor, fn func(Book) error,
) (ArrivalsWalk, error) {
	const op = "LibraryBooksSince"
	path := apiPrefix + "/libraries/" + strconv.FormatInt(libraryID, 10) + "/books"

	var walk ArrivalsWalk
	if fn == nil {
		return walk, &APIError{
			Op: op, Method: http.MethodPost, Path: path,
			Message: "element function is required", Err: ErrInvalidRequest,
		}
	}
	if libraryID < 0 {
		return walk, &APIError{
			Op: op, Method: http.MethodPost, Path: path,
			Message: "library id must not be negative", Err: ErrInvalidRequest,
		}
	}

	// The same deadline split StreamBooks documents: the WALK gets Timeouts.List
	// and each PAGE gets Timeouts.Default, so a library that keeps answering full
	// pages cannot run unbounded and one wedged page cannot silently consume the
	// whole walk's budget.
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.List)
	defer cancel()

	for n := 0; n < maxBookPages; n++ {
		var body booksPageDTO
		err := c.do(ctx, request{
			op: op, method: http.MethodPost, path: path,
			body: arrivalsRequest(cursor),
		}, &body)
		if err != nil {
			return walk, err
		}
		walk.Pages++

		// The page's readable arrivals, in the rendering that goes on the wire.
		//
		// ⚠️ THE FOLD AND THE MITIGATION BOTH WORK IN THE WIRE RENDERING RATHER
		// THAN IN time.Time, AND THAT IS THE SAFE DIRECTION. Two instants that
		// differ below a millisecond render identically here, so treating them as
		// one value can only make the cursor lag further — a RE-READ, absorbed by
		// the idempotent upsert. Folding in time.Time and rendering afterwards
		// would let the walk reason about a distinction the wire cannot express,
		// which is how a cursor ends up above a row the server will never return
		// again.
		seen := make([]string, 0, len(body.Items))
		for _, card := range body.Items {
			b := toBook(card)
			if err := fn(b); err != nil {
				return walk, err
			}
			walk.Count++
			if b.AddedAt.IsZero() {
				// parseISO returns the ZERO TIME for a value it cannot read, by
				// design, and its own header says the cost falls on "a delta
				// channel it does not have yet". This is that channel, so it
				// pays: the row is still delivered and applied, because refusing
				// a page over one bad stamp would cost the catalogue — but a zero
				// NEVER BECOMES A CURSOR. The caller counts them and fails closed
				// on the watermark; the walk only has to keep them out of the
				// mitigation's candidate set.
				continue
			}
			seen = append(seen, FormatArrivalsInstant(b.AddedAt))
		}

		if len(body.Items) < BookPageSize {
			// EXHAUSTED. The only termination that means "finished".
			return walk, nil
		}

		next, ok := advanceArrivalsCursor(seen)
		// ⚠️ THE SECOND HALF OF THIS CONDITION IS NOT BELT AND BRACES. The
		// mitigation's termination proof says C > cursor_prev strictly, and the
		// proof holds over the VALUES THE SERVER COMPARES. It does not, on its
		// own, hold over the RENDERING: two arrivals inside one millisecond
		// render to one string, so a page could in principle yield a C that
		// renders exactly where the cursor already is. Asserting the advance
		// turns that into the loud, local condition below instead of a walk that
		// re-reads one page until the loop bound stops it.
		if !ok || (cursor.Set && next <= cursor.Value) {
			walk.Wedged = true
			if len(seen) > 0 {
				walk.WedgeAt = seen[len(seen)-1]
			}
			return walk, &APIError{
				Op: op, Method: http.MethodPost, Path: path,
				Message: fmt.Sprintf(
					"library %d answered a full page of %d books that share one arrival instant (%s), "+
						"so the arrivals cursor cannot advance without skipping rows; "+
						"nothing was skipped and nothing was advanced, and the repair is a full import",
					libraryID, BookPageSize, walk.WedgeAt),
				Err: ErrUnexpectedStatus,
			}
		}
		cursor = ArrivalsCursor{Value: next, Set: true}
	}

	return walk, &APIError{
		Op: op, Method: http.MethodPost, Path: path,
		Message: fmt.Sprintf(
			"library %d still had a full page after %d pages of %d arrivals; "+
				"a keyset walk this long is a delta that has stopped being one, and the repair is a full import",
			libraryID, maxBookPages, BookPageSize),
		Err: ErrUnexpectedStatus,
	}
}

// arrivalsRequest builds one page's body.
//
// ⚠️ `page` IS 0 HERE AND ON EVERY REQUEST, FOREVER, and it is a literal rather
// than a loop variable so that no future edit can reintroduce an offset by
// accident. maxBookPages exists to bound the LOOP; it is derived from
// BookOrbit's offset ceiling, which a page-0 keyset never approaches.
func arrivalsRequest(cursor ArrivalsCursor) bookQueryRequest {
	req := bookQueryRequest{
		Sort:       arrivalsSort,
		Pagination: bookPagination{Page: 0, Size: BookPageSize},
	}
	if cursor.Set {
		req.Filter = &bookFilterGroup{
			Type: "group", Join: "AND",
			Rules: []bookFilterRule{{
				Type:     "rule",
				Field:    arrivalsFilterField,
				Operator: arrivalsFilterOperator,
				Value:    cursor.Value,
			}},
		}
	}
	return req
}

// advanceArrivalsCursor is the mitigation: the largest value on the page
// STRICTLY BELOW the page's largest value.
//
// It reads a page's rendered arrivals in delivery order and does NOT assume they
// are sorted — the sort is the server's claim, and a fold costs nothing. ok is
// false when every readable value on the page is the same, or when there is no
// readable value at all: both are "there is no C", which is the wedge.
func advanceArrivalsCursor(seen []string) (string, bool) {
	var top, below string
	for _, v := range seen {
		switch {
		case v > top:
			top, below = v, top
		case v < top && v > below:
			below = v
		}
	}
	if below == "" {
		return "", false
	}
	return below, true
}
