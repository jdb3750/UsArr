package store

import (
	"context"
	"database/sql"
	"fmt"
)

// The per-media-type facet count: how many works of each of §17.2's six
// navigation types the caller can see, in one call.
//
// It is the read ARCHITECTURE.md §17.2's Block A (the media-type summary) has
// been blocked on: Block A's row renders a NUMBER with a unit, so a boolean
// presence read would have to be replaced rather than extended.
//
// ⚠️ IT IS NOT THE READ ADR-0053 REOPENS ON, AND MISTAKING IT FOR ONE IS THE
// WHOLE HAZARD. A count does NOT answer presence here, however much it looks
// like it does — the Ebooks/Audiobooks block below is why: this read BUCKETS
// every book exactly once, so a library whose only audiobooks are second
// editions of ebooks counts `audiobooks: 0` while §17.2's row-5 EXISTS says the
// type has content. Hiding a nav entry on this number would hide Audiobooks
// from someone who has audiobooks. ADR-0053's reopening condition was REFINED
// rather than discharged on 2026-08-19 for exactly that (ADR-0059); it now
// names the independent EXISTS over edition.format, which is a different query
// from anything in this file. This file does not change the sidebar.
//
// WHICH SIDE OF THE ACCESS-SCOPE RULE THIS IS ON. store.go rule 2 in its most
// literal form: this IS a rollup across instances, and *"a rollup computed
// across instances a user cannot see is an existence oracle"* is a sentence
// about exactly this shape — six numbers whose whole content is how much exists.
// So the Scope is in the signature, second, and in BOTH statements, and
// TestMediaTypeCountsScopeActuallyFilters breaks each predicate in turn and
// watches an assertion catch it rather than trusting that the parameter is
// threaded.
//
// IT IS A LOCAL READ AND NOTHING ELSE (principle 1). Two statements against the
// local file, no upstream call, no metadata provider, no image fetch.
//
// # WHAT THE COUNTS COUNT — the whole contract, stated rather than implied
//
// The wire-facing copy of this is docs/reference/http-api.md §8, which is where
// a consumer will look. It says the same things in the same order; if the two
// ever disagree, the code is right about behaviour and the divergence is a bug
// in whichever was edited alone.
//
// A work is counted when ALL of these hold:
//
//   - `work.deleted_at IS NULL`. A tombstoned work is not in the library, and
//     the 7-day tombstone window would otherwise inflate every number for a week
//     after an upstream deletion.
//   - `work.kind` is one of recentWorkKinds — the SAME six-kind allowlist Block C
//     and the library grid use, by calling it rather than by repeating it. So
//     `person` is not counted (it is a credit, not a thing you have a library
//     of — ADR-0033), and neither are the child kinds `season`, `episode`,
//     `track` and `comic_issue` (ADR-0030: a manga library's chapters would
//     swamp the number and Block A's row would report chapters where the user
//     reads series).
//   - The access scope admits it, through the same Scope.workVisibilityPredicate
//     Block C and the grid use.
//
// And these are the EXCLUSIONS THAT ARE NOT APPLIED, which is the half a facet
// count gets wrong silently:
//
//   - LIBRARY MEMBERSHIP IS NOT CONSULTED AT ALL. A work that is in no
//     user-defined library is counted. See the block below — it is a decision,
//     not an oversight.
//   - THERE IS NO `?lib=` SCOPE ON THIS READ. The counts are of what you OWN,
//     not of what the library chip currently selects. §17.2 settles that
//     directly — *"Ownership decides shape; scope decides numbers"* — for a
//     sidebar whose numbers ADR-0053 then removed, which leaves ownership as the
//     only axis this read has to serve. A caller that wants counts under a chip
//     selection is asking for a different read, and must not read these as
//     though they were it.
//   - `enabled` and `include_in_search` on `library` are not consulted either,
//     for the same reason: they are properties of a library, and this read is
//     not about libraries.
//
// # THE Unfiled DECISION: unfiled works ARE counted, and here is the reasoning
//
// Both answers were defensible and silence was not, so: COUNTED.
//
// Three reasons, strongest first.
//
//  1. THE COUNT MUST EQUAL THE LIST THE USER REACHES BY CLICKING IT. Block A's
//     row and the sidebar entry both lead to `GET /api/v1/library?media_type=X`,
//     and that endpoint applies NO library predicate unless `?lib=` was sent
//     (browseWorksSQL builds `libPred` only when `f.LibraryIDs` is non-empty).
//     So the grid's default corpus is every visible work of the type, unfiled
//     ones included. A facet count that excluded them would print a number the
//     grid contradicts on the same click — and it would be the SMALLER number,
//     so the disagreement would read as "some of my library is missing" rather
//     than as a counting rule. TestMediaTypeCountsAgreeWithTheBrowseRead pins
//     the equality from the CONSUMER's side, by paging the grid and counting
//     what came back, rather than by re-deriving it from this file's query.
//  2. §17.2's OWN "has content" PREDICATES NAME NO LIBRARY. All six are over
//     `work` and `edition` — `EXISTS (SELECT 1 FROM work WHERE kind='movie' AND
//     deleted_at IS NULL)` and so on. The section that specifies the consumer
//     specifies a membership-independent corpus.
//  3. EXCLUDING THEM HIDES OWNED CONTENT ON THE FIRST SCREEN. The type totals
//     would stop summing to the catalogue, and the residue would be invisible:
//     nothing on Home names it, and there is no "unfiled" screen to reach it
//     from. A number that is too small by an amount the user cannot discover is
//     worse than one that is larger than any single library.
//
// ⚠️ WHAT WAS VERIFIED ABOUT THE RESERVED `Unfiled` ROW, because the received
// account of it is half right and the wrong half matters. Measured over the
// tree at the time of writing:
//
//   - TRUE: migration 0005 seeds `library.id = 0`, "Unfiled", `managed_by =
//     'auto'`, `user_id = 0`, and protects it with trg_library_unfiled_no_delete.
//   - TRUE: `managed_by = 'user'` is written by NOTHING in non-test Go. The two
//     writers of the column are the seed above and catalogue.go's create, which
//     hardcodes `'auto'`.
//   - TRUE: the existing reads exclude it in the SQL — listLibrariesSQL and
//     LibraryIDsBySlug both bind `l.id <> ?` from UnfiledLibraryID.
//   - ⚠️ FALSE, AND THIS IS THE HALF THAT CHANGES THE QUESTION: there are no
//     items in it. Nothing anywhere writes a `library_member` row with
//     `library_id = 0`; the one production writer of that table
//     (catalogue.go's item pass) always names a real binding's library. The
//     reserved row is a SEARCH-SCOPE sentinel, not a membership one — its only
//     use is `search_doc_library`, where invariant 5 files a doc that no library
//     scopes so that it is not invisible to its own owner.
//
// So "an unfiled work" means A WORK WITH NO `library_member` ROW AT ALL, not a
// member of library 0, and the decision above is about those. The reserved row
// cannot move these numbers in either direction — this read never touches
// `library_member`, and nothing puts a member row in library 0 to touch.
// TestMediaTypeCountsCountUnfiledWorks pins BOTH readings, including the one
// the tree cannot currently produce, so that a future writer that starts filing
// into library 0 finds an assertion rather than a surprise.
//
// # ZERO AND INVISIBLE ARE THE SAME ANSWER, IN ONE DIRECTION ONLY
//
// A media type the caller cannot see and a media type with genuinely no rows
// are INDISTINGUISHABLE on this read, and the collapse runs one way: restriction
// renders as zero, and zero never widens into "hidden" or "unknown". There is no
// third state — no null, no absent key, no flag.
//
// That is the direction the fourth principle forces. The whole risk this read
// carries is that a caller who cannot see something learns that it is there; a
// caller who CAN see everything learning that a type is empty is not a leak, it
// is the answer. So the two must share a spelling, and it has to be the spelling
// that says less. Publishing "restricted" as its own value would turn every
// hidden type into a positive statement that content exists — which is the
// existence oracle with a label on it.
//
// The cost is real and is paid elsewhere: this read cannot tell an empty screen
// WHY it is empty. It does not have to. ADR-0053's `browseEmptyState`
// (web/src/lib/librarygrid.ts) already words that from the services read,
// separating "no library-bearing service is connected" from "this type has no
// rows yet" from "the library scope excludes everything".

// MediaTypeCounts is one count per §17.2 navigation type.
//
// IT IS A SIX-FIELD STRUCT RATHER THAN A MAP, and that is the point rather than
// a style choice. §17.2's axis table closes the enum at *"bounded at 6, by
// construction"*, and a struct makes "all six answers are always present" a
// property of the TYPE instead of a property of a loop that filled a map — a
// map with a missing key and a map with a zero value are different values that
// a consumer has to tell apart, which is exactly the zero-versus-invisible
// distinction the block above exists to remove. A seventh media type is an edit
// here, at recentWorkKinds and at mediaTypeOf, and TestMediaTypeCountsCoverEveryMediaType
// is what makes it a deliberate one.
type MediaTypeCounts struct {
	Movies     int64
	TV         int64
	Music      int64
	Ebooks     int64
	Audiobooks int64
	Comics     int64
}

// add books one count into the bucket a media type names. It exists so the scan
// loop maps through mediaTypeOf — the SAME function that renders the Type cell
// of every Block C and grid row — instead of carrying a second kind switch that
// could drift from it.
func (c *MediaTypeCounts) add(mediaType string, n int64) error {
	switch mediaType {
	case MediaTypeMovies:
		c.Movies += n
	case MediaTypeTV:
		c.TV += n
	case MediaTypeMusic:
		c.Music += n
	case MediaTypeEbooks:
		c.Ebooks += n
	case MediaTypeAudiobooks:
		c.Audiobooks += n
	case MediaTypeComics:
		c.Comics += n
	default:
		return fmt.Errorf("store: %q is not one of the six media types", mediaType)
	}
	return nil
}

// mediaTypeCountsSQL renders the per-kind count and its arguments.
//
// It is a function rather than a literal inlined below so the query-plan test
// can EXPLAIN THE STATEMENT THAT SHIPS (docs/DEVELOPMENT.md §11 rule 1: a guard
// that asserts against a hand-copied lookalike is probing a proxy).
//
// ⚠️ THERE IS NO UNARY PLUS ON `w.kind` HERE, AND ITS ABSENCE IS AS DELIBERATE AS
// ITS PRESENCE IS IN recentWorksSQL. Block C writes `+w.kind` to DISQUALIFY
// ix_work_kind_sort, because that read has an `added_at` order it needs a
// different index to supply and the kind index would cost it a temp b-tree. This
// statement has no order to protect and nothing to return but counts, so
// ix_work_kind_sort(kind, sort_title, id) WHERE deleted_at IS NULL is exactly
// what it wants: an equality seek per kind, and — because the IN list is walked
// in sorted order — the GROUP BY supplied by the index rather than by a sort.
// MEASURED on the real schema, this engine (github.com/ncruces/go-sqlite3,
// SQLite 3.53.4), NO ANALYZE, which is the planner the suite sees:
//
//	SEARCH w USING INDEX ix_work_kind_sort (kind=?)
//
// with no `USE TEMP B-TREE FOR GROUP BY` line. So the statement never reads a
// row of `work` and never sorts: six seeks, whatever the catalogue's size.
//
// ⚠️ IT IS *NOT* REPORTED AS `COVERING`, AND THE PREDICTION THAT IT WOULD BE WAS
// WRONG. The SELECT list is `kind, COUNT(*)` and reads no column off the table,
// so "covering" looked structural — and the measurement above says otherwise.
// Recorded rather than quietly corrected because it is the second time this
// index has produced that expectation: reference/schema.md §1 already states
// *"`ix_work_kind_sort` serves the keyset query; it does not cover it"*, and it
// gives the standing rule for every guard over it — *"the assertion must assert
// `SEARCH … USING INDEX`, not `COVERING INDEX`"*, because covering-ness depends
// on a SELECT list and pinning it fails the moment one grows a column. The plan
// guard follows that rule; it does not pin COVERING here.
//
// THE `book` ROW COMES BACK AS ONE NUMBER and is split afterwards. `book` is two
// media types (§17.2 rows 4 and 5) separated by `edition.format`, and putting
// that split in the GROUP BY key would make the group expression a correlated
// subquery over `edition` — evaluated once per work, and grouped through a temp
// b-tree because no index can order a computed column. So this statement counts
// books once, audiobookCountSQL counts the audiobook half, and the ebook half is
// the difference. mediaTypeOf's `book` arm already answers Ebooks for a work
// with no edition rows, so scanning through it puts the whole book count in
// Ebooks and the subtraction moves the audiobooks out; the two never disagree
// about the total because the total is only ever read once.
func mediaTypeCountsSQL(scope Scope) (string, []any) {
	args := make([]any, 0, len(recentWorkKinds)+len(scope.InstanceIDs))
	for _, k := range recentWorkKinds {
		args = append(args, k)
	}
	scopePred, scopeArgs := scope.workVisibilityPredicate("w.id")
	args = append(args, scopeArgs...)

	return `
		SELECT w.kind, COUNT(*)
		  FROM work w
		 WHERE w.deleted_at IS NULL
		   AND w.kind IN (` + placeholders(len(recentWorkKinds)) + `)
		   AND ` + scopePred + `
		 GROUP BY w.kind`, args
}

// audiobookCountSQL renders the Audiobooks half of the book split.
//
// THE PREDICATE IS browseAudiobookPredicate, THE ONE THE GRID FILTERS ON, shared
// by reference and not by retyping. That is what makes the count equal the list:
// `GET /api/v1/library?media_type=audiobooks` selects the works this counts, row
// for row, and TestMediaTypeCountsAgreeWithTheBrowseRead checks it from the
// consumer's side. Its own header explains why it is two EXISTS rather than the
// tidier MIN form, and why the second half spells out `format IS NOT NULL AND
// format <> ?`.
//
// THE EBOOK COUNT IS THE COMPLEMENT AND IS NOT ITS OWN STATEMENT, because the
// grid's Ebooks filter is literally `NOT browseAudiobookPredicate` over the same
// `kind = 'book'` corpus. Ebooks + Audiobooks = books, exactly, with no work in
// both and none in neither — so a third statement could only ever confirm
// arithmetic, at the cost of a third plan to keep honest.
//
// ⚠️ THIS IS WHERE THIS READ DIVERGES FROM §17.2's ROWS 4 AND 5, AND THE
// DIVERGENCE IS DELIBERATE. §17.2 gives those rows two INDEPENDENT existence
// queries, under which a book with an EPUB and an M4B makes BOTH media types
// "have content". A COUNT cannot do that and stay a count: Block A's column
// would not sum to the catalogue, and the same work would be reported twice in a
// summary whose whole job is "what do I have?". So the assignment here is
// mediaTypeOf's — every book lands in exactly one of the two, and a mixed book
// lands in Ebooks, for the reason mediaTypeOf gives (its format list is the
// longer one, so the claim stays true of the work). The consequence, stated
// rather than left to be discovered: a library whose ONLY audiobooks are second
// editions of ebooks counts `audiobooks: 0` while §17.2's row-5 predicate would
// say the type has content. That matters the day something hides a type on this
// number, which is ADR-0053's decision to amend and not this read's to take —
// and ADR-0059 records why this count must not be the thing that discharges it.
//
// MEASURED on the real schema, this engine, no ANALYZE — see mediaTypeCountsSQL
// for why that planner is the one pinned. Under the owner's full scope:
//
//	SEARCH w USING INDEX ix_work_kind_sort (kind=?)
//	| CORRELATED SCALAR SUBQUERY 2
//	| SEARCH en USING INDEX ix_edition_work (work_id=?)
//	| SEARCH ea EXISTS USING COVERING INDEX ix_edition_format (format=? AND work_id=?)
//
// and under a partial scope the same, plus
// `SEARCH sil EXISTS USING INDEX ix_sil_work (work_id=?)` — the scope predicate,
// which is `1=1` and absent for the owner.
//
// One seek to reach the books, then a two-column COVERING probe per book on the
// index migration 0009 added for exactly this predicate, then the uncovered
// `work_id` probe for the half no index can seek on (`format IS NOT NULL AND
// format <> ?`). Nothing scans `edition` and nothing sorts. ⚠️ The plan lists
// `en` above `ea`, which is EQP's nesting and not an execution order; the guard
// pins the two probes and their indexes, and deliberately claims nothing about
// which runs first.
func audiobookCountSQL(scope Scope) (string, []any) {
	args := make([]any, 0, len(scope.InstanceIDs)+4)
	args = append(args, bookWorkKind)

	scopePred, scopeArgs := scope.workVisibilityPredicate("w.id")
	args = append(args, scopeArgs...)

	args = append(args, editionFormatAudiobook, editionFormatAudiobook)

	return `
		SELECT COUNT(*)
		  FROM work w
		 WHERE w.deleted_at IS NULL
		   AND w.kind = ?
		   AND ` + scopePred + `
		   AND ` + browseAudiobookPredicate, args
}

// bookWorkKind is the one `work.kind` that two media types share. It is a
// constant here because two statements in this file bind it and mediaTypeOf
// switches on it; spelling it three times is how the split comes apart.
const bookWorkKind = "book"

// CountWorksByMediaType answers §17.2's six-value navigation enum with a count
// each, under one access scope.
//
// SCOPE IS IN THE SIGNATURE AND IN BOTH STATEMENTS, per store.go rule 2. See the
// file header for why this read is the literal case that rule was written for.
//
// TWO STATEMENTS, NOT ONE. They are issued separately on the read pool and
// therefore on two WAL snapshots, so an import committing between them could in
// principle make the audiobook count reflect a database the kind counts did not
// see. That is accepted rather than overlooked, and it is bounded: the only
// visible artefact is that Ebooks — the difference of the two — could be off by
// the number of book works that changed in the window, and the guard below
// refuses the one shape that is not merely stale but incoherent. Taking a shared
// snapshot would mean a transaction on the render path for a summary that is a
// snapshot of a moving import either way, and §17.2 already tells Block A to say
// so ("Totals reported by each service").
func (s *Store) CountWorksByMediaType(ctx context.Context, scope Scope) (MediaTypeCounts, error) {
	var out MediaTypeCounts

	query, args := mediaTypeCountsSQL(scope)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return MediaTypeCounts{}, fmt.Errorf("count works by media type: %w", err)
	}
	if err := scanMediaTypeCounts(rows, &out); err != nil {
		return MediaTypeCounts{}, err
	}

	// Every book is in Ebooks at this point, which is mediaTypeOf's answer for a
	// work whose editions say nothing. The audiobooks are moved out below.
	query, args = audiobookCountSQL(scope)
	var audiobooks int64
	if err := s.db.Read().QueryRowContext(ctx, query, args...).Scan(&audiobooks); err != nil {
		return MediaTypeCounts{}, fmt.Errorf("count audiobooks: %w", err)
	}
	if audiobooks > out.Ebooks {
		// Not reachable from a consistent database: the audiobook statement's
		// corpus is a subset of the book statement's, filtered further. It is an
		// error rather than a clamp because the only things that produce it are
		// the two statements disagreeing about the corpus — a scope predicate
		// dropped from one of them, or the kind allowlist and `bookWorkKind`
		// drifting apart — and both of those are bugs that a silent `max(0, …)`
		// would render as a plausible-looking screen.
		return MediaTypeCounts{}, fmt.Errorf(
			"count works by media type: %d audiobooks among %d books; the two statements "+
				"disagree about the corpus", audiobooks, out.Ebooks)
	}
	out.Ebooks -= audiobooks
	out.Audiobooks = audiobooks
	return out, nil
}

func scanMediaTypeCounts(rows *sql.Rows, out *MediaTypeCounts) error {
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var kind string
		var n int64
		if err := rows.Scan(&kind, &n); err != nil {
			return fmt.Errorf("count works by media type: scan: %w", err)
		}
		// The invalid NullInt64 is "this work has no edition rows", which is
		// mediaTypeOf's Ebooks answer for a book. Deliberate: this statement
		// does not read editions, and the audiobook half is subtracted after.
		mediaType := mediaTypeOf(kind, sql.NullInt64{})
		if mediaType == "" {
			// Unreachable: the kind allowlist in the query and the switch in
			// mediaTypeOf are the same six kinds. It is an error rather than a
			// dropped row because the two lists drifting apart is precisely the
			// bug that would otherwise ship a summary that silently omits a
			// type — which reads as "you have none of these".
			return fmt.Errorf(
				"count works by media type: kind %q, which recentWorkKinds admits and "+
					"mediaTypeOf does not map; the two lists have drifted apart", kind)
		}
		if err := out.add(mediaType, n); err != nil {
			return fmt.Errorf("count works by media type: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("count works by media type: %w", err)
	}
	return nil
}
