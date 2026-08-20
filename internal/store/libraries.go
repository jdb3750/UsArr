package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// The Libraries screen's read (ARCHITECTURE.md §17.8).
//
// §17.8's row view is `name · kind · item count · source chips with per-source
// health · request destination (absent in v0.1) · state · reorder handle`, and
// this is the one read behind it. Before this commit §17.8's own build-status
// block was exact about the hole: *"There is no read path at all. No store
// method lists libraries for display; the only SELECT over the table in non-test
// Go is the binding's own name-collision lookup."* That sentence is what this
// file falsifies.
//
// WHICH SIDE OF THE ACCESS-SCOPE RULE THIS IS ON. store.go rule 2: every read
// that aggregates across instances takes a Scope IN THE SIGNATURE. This one
// aggregates across instances twice over — a library's sources NAME service
// instances, and its item count is a rollup over works those instances
// replicated — so both halves carry the scope, and TestListLibrariesScopeActuallyFilters
// breaks each of them and watches the assertion catch it.
//
// IT IS A LOCAL READ AND NOTHING ELSE (principle 1). Three statements against
// the local file, no upstream call, no capability probe. ⚠️ THE THIRD ONE READS
// A MEASUREMENT THAT WAS MADE UPSTREAM, and that is not a contradiction: the
// completeness verdict is computed once at IMPORT and written to sync_report, so
// what happens here is a SELECT over a row that already exists. Nothing on this
// path may ever ask a service how complete a library is. In particular this is NOT
// the connect probe: ADR-0048 puts the proposal set in the probe's response and
// says a `library` row exists only once the user has accepted one, so everything
// this read returns is, by that ADR's clause 4, an accepted library. A proposal
// has no row and therefore cannot appear here.
//
// WHAT IT DELIBERATELY DOES NOT CARRY, so a caller does not wait for it:
//
//   - The request destination. §17.8: *"The `Request destination` column does not
//     render in v0.1, and it returns with the first service that can be a
//     destination."* The four `sink_*` columns are written by nothing in the
//     tree, so reading them would publish four nulls per row.
//   - `managed_by`. ADR-0048 Fact 1 and §17.8 both measure that `'user'` has
//     never been written by any code path, so the column is `'auto'` on every
//     row by construction. §17.4 rule 5 — a column whose value is identical for
//     every row is not data — applies to a wire field as much as to a table
//     cell, and shipping it would let a client build the one-way-door indicator
//     out of a constant.
//   - `icon` and `default_sort`. §17.8's DETAIL view's Identity and Visibility
//     groups, not its row view. The detail read is its own commit and its own
//     screen. `formats` is the exception among the three and travels — see
//     Library.Formats for the one reason it earns its place here.
//   - WHICH MEDIA TYPES A SOURCE SUPPLIES. Asked for and LEFT OUT, deliberately.
//     The answer is not on `library_source`: it would take an aggregate over
//     `library_member` → `work.kind` → `edition.format` per source, i.e. a join
//     across the catalogue that this read otherwise never makes, to produce a
//     number the library's own (kind, formats) pair already bounds. That is not
//     nearly free, so it is not here. The LIBRARY-level pair is, and it costs
//     nothing because it is on the row already being read.
//   - Anything credential-shaped. See librarySourcesSQL for the allowlist over
//     service_instance, which is the join this read has to make and the one that
//     touches the row holding `api_key_enc`.

// Library is one row of §17.8's Libraries list, as the store reads it.
//
// It is deliberately not the whole `library` row — see the file header for each
// column that is absent and why.
type Library struct {
	ID   int64
	Name string
	Slug string

	// Kind is `library.kind` verbatim, the schema's word. §17.8 requires the
	// LABEL to be the product's vocabulary instead — *"Label it `Movies · TV ·
	// Music · Books · Comics`, let the schema value be the value"* — and that
	// mapping is the renderer's, exactly as §17.2's media-type enum is
	// resolved server-side for the opposite reason. The two are not the same
	// vocabulary and must not be conflated: this one has seven members and
	// answers "what is this library of", the other has six and answers "what
	// is this work".
	Kind string

	// Formats is `library.formats`: a JSON array over `edition.format`, NULL
	// meaning "any". It is forwarded as the raw column and NOT parsed here,
	// exactly as RecentWork forwards `availability`.
	//
	// It travels because (kind, formats) is §17.2's media-type PAIR — the
	// Ebooks/Audiobooks split is `["ebook"]` and `["audiobook"]` over one
	// `book` kind — so without it a renderer cannot name a library's media type
	// at all for two of the six. Reading it costs nothing: it is a column on
	// the row this statement already reads, no join and no second statement.
	//
	// ⚠️ NOTHING WRITES IT. `library.formats` has no writer in non-test Go —
	// catalogue.go's create names user_id, name, slug, kind, managed_by,
	// enabled and include_in_search, and there is no `UPDATE library` anywhere
	// — so it is NULL on every row today and a renderer falls back to kind
	// alone. The column is the seam the Audiobookshelf split lands on, and the
	// split is not v0.1's (§17.8 marks it "from the milestone Audiobookshelf
	// lands in, not v0.1").
	Formats sql.NullString

	SortOrder       int64
	Enabled         bool
	IncludeInSearch bool

	// OrphanedAt is §6.5 rule 5's retained-with-a-reason state: set when the
	// last library_source goes away, the row kept rather than deleted because
	// it carries owned corrections.
	//
	// ⚠️ NOTHING WRITES IT. Measured over the tree: `orphaned_at` has no writer
	// and no reader in non-test Go. It is read here because the state is
	// §17.8's and the read is the place it becomes visible, not because the
	// state is reachable today — a renderer that shows an "orphaned" chip is
	// showing a state nothing can currently produce.
	OrphanedAt sql.NullString

	// ItemCount is the number of library_member rows in this library that the
	// scope admits. See listLibrariesSQL for the grain and for why this is the
	// cheap count rather than a distinct one.
	ItemCount int64

	// Sources is §17.8's source chips, ordered deterministically. It is empty
	// for an orphaned library, and it is also empty when every source of a
	// visible library sits on an instance the scope hides — which is the state
	// the scope exists to produce and is why the library still lists.
	Sources []LibrarySource

	// Completeness is what the last import measured about how much of this
	// library's upstream containers UsArr's credential could actually see.
	//
	// ⚠️ NIL IS "NOTHING WAS MEASURED", NEVER "COMPLETE". It is nil for every
	// library whose sources come from an adapter that runs no completeness check
	// — which today is every Kavita library and every library that has never
	// been imported — and a renderer that painted a positive mark on nil would
	// be reporting the absence of a check as a pass. That is the same defect
	// this field exists to close, one level up. See completeness.go.
	Completeness *LibraryCompleteness

	// Skips is what the last import recorded about items it READ in this
	// library's containers and deliberately did not map. It is the sibling axis
	// to Completeness and is independent of it: a library can be fully visible
	// and still be short.
	//
	// ⚠️ NIL IS "NOTHING OBSERVED THIS LIBRARY", NEVER "NOTHING WAS SKIPPED".
	// The two are different facts and skips.go explains why the table alone
	// cannot separate them. See LibrarySkips.
	Skips *LibrarySkips
}

// LibrarySkips is one library's skip verdict, folded from its containers'.
//
// # The three states, and where they come from (ADR-0063)
//
// Every container an import WALKED gets a skip row, zero or not; a container
// nothing walked gets none. So all three readings come off the skip rows alone:
//
//	a row with a non-zero total → SkipsLeftOut
//	a row with a zero total     → SkipsNone   ("walked, none recorded")
//	no row at all               → nil         ("nothing walked it")
//
// ⚠️ THIS USED TO LEAN ON THE COMPLETENESS ROW AND NO LONGER DOES. Under
// ADR-0061 §5 a skip row existed only where something had been skipped, so the
// table could say *yes* or *silence* and silence covered both remaining states;
// the completeness verdict — a row per container observed — was pressed into
// service as the evidence that separated them. ADR-0063 supersedes that: the
// writer emits the zero row, and this read is evidence for itself. Nothing about
// the neighbouring measurement is load-bearing here any more, and
// attachLibrarySkips no longer has to run after attachLibraryCompleteness.
//
// ⚠️ AND THE IMPRECISION THAT COUPLING CARRIED IS GONE WITH IT. The completeness
// verdict is recorded from Containers(), BEFORE the walk, so an import that dies
// part-way writes one for containers it never reached — and those used to read
// SkipsNone when the truth was "not observed". Skip rows are raised from tallies
// created during the walk, so a container the walk never reached has none and
// reads nil.
//
// ⚠️ WHAT IS STILL OPEN, WRITTEN DOWN RATHER THAN LEFT TO BE INFERRED: the one
// container the walk died INSIDE does get a row, from what it had read so far,
// so a container that had skipped nothing before failing reads SkipsNone for a
// partial read. At most one per import. The compensating control is the one it
// always was and is not this feature's: an instance whose import did not finish
// renders *"An import did not finish · this count may be short"* on every one of
// its libraries (web/src/lib/libraries.ts). SkipsNone also renders NOTHING, so
// the cost is a sentence that was not shown rather than a claim that was made.
//
// # What it does not say
//
// ⚠️ SkipsNone IS NOT A COMPLETENESS CLAIM. It says the adapter recorded no
// unmapped items. It says nothing about whether the credential saw the whole
// container — that is Completeness, and it is a different measurement with a
// different failure mode.
type LibrarySkips struct {
	// State is SkipsLeftOut or SkipsNone. A library with neither has a nil
	// *LibrarySkips rather than a third member.
	State SkipState

	// Items is how many items were read and not mapped, summed over the
	// containers that recorded a skip. 0 under SkipsNone, where it is a fact
	// rather than a sentinel: nothing was recorded, so nothing was left out that
	// anything knows of.
	Items int64

	// Reason is UsArr's own short sentence about why, taken from the first
	// container that recorded one. Empty under SkipsNone, and never upstream
	// text.
	Reason string

	// RecordedAt is when the newest folded row was written, in the store's own
	// timestamp layout. It answers "how old is this", which is the question any
	// stored measurement raises — and under SkipsNone it is the completeness
	// verdict's stamp, because that row is the observation the state rests on.
	RecordedAt string

	// Containers is the per-container breakdown, one entry per container row
	// folded in, in the order the statement returned them.
	//
	// ⚠️ IT IS SERVED RATHER THAN FOLDED AWAY, AND THAT IS THE WHOLE POINT OF
	// THE FIELD. Items above is the library's total and is true of the library;
	// the containers are where the skips actually happened, and a reader holding
	// several libraries needs them to tell one container's skip counted twice
	// from two containers' skips counted once. Two BookOrbit instances with a
	// mixed container each bind three libraries — book over A and B, comic over
	// A, comic over B — whose container SETS are all different while the skips
	// underneath overlap, so nothing derived from the library total separates
	// those two cases. See librarySkipsResponse in internal/httpapi.
	//
	// ⚠️ AND THE TOTAL IS NEVER APPORTIONED ACROSS THEM, in either direction. A
	// skip is a fact about a container, so each entry carries what that container
	// recorded and Items is their sum; dividing a library's total across its
	// containers would invent a precision no import measured.
	Containers []LibrarySkipContainer
}

// LibrarySkipContainer is one container's contribution to a library's skips.
//
// The identity is the same triple library_source publishes — instance, kind and
// the upstream's own ref — because that is what makes an entry on one library
// recognisable as THE SAME CONTAINER as an entry on another. It is deliberately
// not the library_source id: two libraries over one container have two source
// rows, and keying on those would make one container look like two.
type LibrarySkipContainer struct {
	ServiceInstanceID int64
	ContainerKind     string
	ContainerRef      string

	// Items is what this container's newest skip row recorded. 0 is the measured
	// negative for that container, not an absence.
	Items int64
}

// LibraryCompleteness is one library's verdict, folded from the per-container
// verdicts its sources carry.
//
// # The fold, and why unverified wins
//
// A library can bind several containers. The states are combined WORST-FIRST
// with `unverified` at the top, not `shortfall`:
//
//	any unverified → unverified
//	else any shortfall → shortfall
//	else → complete
//
// `unverified` outranks `shortfall` because Total and Visible are LIBRARY-LEVEL
// numbers once folded, and an unmeasured container makes both of them wrong. A
// library reading *"holds 412; the account can see 389"* while a second source
// was never checked is a precise-looking claim that is not true. The shortfall
// on the other container is not lost — it is in the process log and in its own
// sync_report row, which is where a number that cannot be summed belongs.
//
// In v0.1 this fold is almost always over one container: catalogue.go's bind
// path creates one library per upstream container (§17.8).
type LibraryCompleteness struct {
	// State is one of the three members in completeness.go.
	State CompletenessState

	// Total and Visible are summed over the containers that produced a
	// measurement. Both are 0 when State is unverified, and a caller must read
	// State before either of them: they are not a claim in that state.
	Total   int64
	Visible int64

	// Hidden is Total - Visible, as measured. 0 unless State is a shortfall.
	Hidden int64

	// Reason is UsArr's own sentence about why the check could not be made.
	// Non-empty only when State is unverified, and never upstream text.
	Reason string

	// CheckedAt is when the newest of the folded verdicts was recorded, in the
	// store's own timestamp layout. It answers "how old is this claim", which is
	// the question any stored measurement raises.
	CheckedAt string

	// Containers is how many container verdicts were folded into this one. It is
	// on the struct because the fold above is lossy: a caller that wants to know
	// whether a shortfall figure covers the whole library can compare this
	// against the number of sources.
	Containers int
}

// LibrarySource is one `library_source` row plus the two fields of its service
// instance that §17.8's source chip renders.
//
// THE JOIN TO service_instance IS THE DANGEROUS ONE AND THIS STRUCT IS THE
// ALLOWLIST. `service_instance` carries `api_key_enc` — a full-admin *Arr
// credential (CLAUDE.md, Security rules) — and `base_url`, which is an internal
// host the user typed. Exactly two columns are read: `name`, which is the chip's
// label and the string §17.8's cross-link copy uses (*"Radarr feeds 2
// libraries"*), and `kind`, which is the icon. `base_url` is absent because
// §17.8 states outright that *"No credential field ever appears on this screen"*
// and because §17.3's Services screen is where an instance's address is edited,
// behind sudo. Nothing is copied by reflection and nothing is embedded, so a
// column added to serviceInstanceColumns cannot arrive here by default.
type LibrarySource struct {
	ID int64

	ServiceInstanceID int64
	ServiceName       string
	ServiceKind       string

	// ContainerKind is one of migration 0005's five, and ContainerRef is the
	// container the UPSTREAM ITSELF reported, verbatim. In v0.1 only
	// 'remote_library' is ever written (catalogue.go's insertLibrarySource
	// hardcodes it), so the ref is a Kavita library id.
	ContainerKind string
	ContainerRef  string

	// ContainerIdentity is the container's own name as the upstream reported it
	// at bind time — §17.8's *"upstream's own name beneath it, greyed and
	// non-editable"*. It is nullable in the schema and the bind path always
	// writes it.
	ContainerIdentity sql.NullString

	IsMetadataAuthority bool

	// MissingSince is §17.8's per-source health: the upstream stopped reporting
	// this container, and the row is retained and shown rather than dropped.
	//
	// ⚠️ NOTHING SETS IT. Measured: the only two statements in non-test Go that
	// touch the column both CLEAR it — `insertLibrarySource`'s ON CONFLICT and
	// catalogue.go's rebind both write `missing_since = NULL` — and no code
	// path anywhere sets a non-NULL value. So the "source has gone missing"
	// state is specified, storable, cleared on rebind, and unreachable. It is
	// carried on this read because the sweep that sets it is the next commit
	// and the field is the seam; it is documented as unreachable so nobody
	// reads a screen with no missing sources as evidence that none are missing.
	MissingSince sql.NullString
}

// libraryVisibilityPredicate renders the access scope as a predicate over one
// library-id column, plus its arguments.
//
// WHY A library READ NEEDS ONE. `library` has a `user_id`, and the user half is
// carried separately by userPredicate — that is principle 4, not access scope.
// What makes this an INSTANCE-scoped read is `library_source`: a library row
// names, through its sources, which service instances exist. Listing a library
// whose only sources are on instances the caller cannot see is store.go rule
// 2's existence oracle in its most literal form, because the response quite
// literally carries the instance's name.
//
// Three branches, matching Scope.workVisibilityPredicate:
//
//   - AllInstances — v0.1's owner — is `1=1` and adds nothing to the plan.
//   - An empty visible set FAILS CLOSED at `1=0`. "No visible instances" must
//     mean no rows, never all rows.
//   - Otherwise: a library is visible if it has a source on a visible instance,
//     OR if it has no sources at all. The second arm is not a loophole — a
//     source-less library is §6.5 rule 5's retained orphan, it names no
//     instance, so there is nothing for it to leak, and dropping it would hide
//     a user's own library (and the corrections keyed to it) behind a fact
//     about services.
func (s Scope) libraryVisibilityPredicate(column string) (string, []any) {
	if s.AllInstances {
		return "1=1", nil
	}
	if len(s.InstanceIDs) == 0 {
		return "1=0", nil
	}
	args := make([]any, 0, len(s.InstanceIDs))
	for _, id := range s.InstanceIDs {
		args = append(args, id)
	}
	ls := librarySourceAlias(column)
	return `(NOT EXISTS (SELECT 1 FROM library_source ` + ls + `
	                      WHERE ` + ls + `.library_id = ` + column + `)
	         OR EXISTS (SELECT 1 FROM library_source ` + ls + `
	                     WHERE ` + ls + `.library_id = ` + column + `
	                       AND ` + ls + `.service_instance_id IN (` + placeholders(len(args)) + `)))`, args
}

// librarySourceAlias names the inner library_source of BOTH arms of the
// predicate above, derived from the outer library-id column they correlate to.
// See scopeLinkAlias for the whole argument and derivedInnerAlias for the
// construction; docs/REVIEW-LOG.md LS-379 is where the class was found.
//
// WHY BOTH ARMS MATTER HERE, and why this predicate is the worse of the two
// cases. An outer aliased `ls` shadowed the inner in each arm, and they
// degenerate in OPPOSITE directions, so neither symptom looks like the other:
//
//   - the EXISTS arm decorrelates to "is ANY library sourced on a visible
//     instance", which is true as soon as one is, so every library leaks;
//   - the NOT EXISTS orphan arm decorrelates to "is library_source EMPTY",
//     which is false as soon as one row exists anywhere, so it stops admitting
//     the source-less libraries it exists to admit — a DISAPPEARANCE rather
//     than a leak, and normally masked by the arm above it.
//
// Measured on seedLibrariesCorpus before the fix: library 3, sourced only on
// instance 2, came back under a scope holding only instance 1; and under a
// scope holding only an instance with no sources bound, source-less library 4
// vanished. TestLibraryVisibilityPredicateSurvivesAnOuterLsAlias holds both.
func librarySourceAlias(column string) string {
	return derivedInnerAlias("ls", column)
}

// listLibrariesSQL renders the library statement and its arguments.
//
// It is a function rather than a literal inlined below so the query-plan test
// can EXPLAIN THE STATEMENT THAT SHIPS (docs/DEVELOPMENT.md §11 rule 1: a guard
// that asserts against a hand-copied lookalike is probing a proxy).
//
// # THE RESERVED ROW IS EXCLUDED, AND IT IS EXCLUDED IN THE SQL
//
// Migration 0005 seeds `library.id = 0`, "Unfiled", and states what it is for
// and what it is not: *"Never listed on the Libraries screen, never offered in
// the scope chip, never proposed."* This read is the Libraries screen. So it is
// excluded, unconditionally, with no option to include it.
//
// It is excluded HERE, in the WHERE clause, rather than by an `if` in the scan
// loop — which is how catalogue.go's `userLibraries` does it. ADR-0048's own
// reading of that site is the argument: an exclusion that is *"an `if` inside a
// scan loop is even less likely to be inherited by the next author's query than
// one written into a `WHERE` clause, because it is not in the query to copy"*.
// The next read of this table should be able to copy this statement and get the
// exclusion for free. The constant is bound as a parameter rather than spelled
// `<> 0` so UnfiledLibraryID stays the single place the number lives.
//
// # THE ITEM COUNT IS A CORRELATED SEEK, AND ITS GRAIN IS STATED
//
// `library_member` leads BOTH of its keys with library_id — the WITHOUT ROWID
// primary key `(library_id, sort_title, work_id, edition_id)` and
// ux_libmem_identity `(library_id, work_id, edition_id)` — so a count
// constrained to one library_id is a range seek either way, with no lookup and
// nothing to add to the schema. MEASURED rather than predicted: SQLite 3.53.4
// picks the narrower one and covers the count with it,
//
//	SEARCH m USING COVERING INDEX ux_libmem_identity (library_id=?)
//
// which is what makes this cheap enough to ship once per row. The guard
// (TestListLibrariesPlanIsSeeks) pins the SEEK and the index name and does NOT
// pin COVERING, per schema.md §1's rule — although here the SELECT list is
// COUNT(*) and selects no column at all, so covering is structural rather than
// lucky.
//
// It counts MEMBER ROWS, which are EDITION-GRAINED (§17.8's flagship
// Ebooks/Audiobooks split is why the key carries edition_id at all). Today that
// equals a count of distinct works, because the only writer of the table —
// catalogue.go's item pass — hardcodes `edition_id = 0`, the "whole work,
// format-independent" sentinel. When an edition-grained writer lands, a work
// with two editions in one library will count twice and this comment is where
// the decision to change it goes. COUNT(DISTINCT work_id) is NOT the safe
// default: the key's second column is sort_title, so a distinct pass over
// work_id cannot be served by the key order and buys a temp b-tree for a
// difference that does not exist yet.
//
// The count carries the SCOPE, through the same workVisibilityPredicate
// Home's Block C uses. Under the owner's full scope that renders as `1=1` and
// costs nothing; under a partial scope it becomes an EXISTS over
// service_item_link per member row, which is slower and correct — a count that
// silently included works from instances the caller cannot see would be the
// rollup store.go rule 2 names as an existence oracle.
//
// # THE ORDERING IS A SORT, DELIBERATELY
//
// §17.8's row view ends in a reorder handle, so `sort_order` is the order and
// `name` breaks its ties. Migration 0005 gives `library` three indexes —
// ux_library_slug, ux_library_name and a partial ix_library_kind — and none of
// them leads with sort_order, which is a schema decision rather than an
// oversight: this is the same posture `service_instance` takes, where
// TestServiceInstanceListScanIsIntentional pins the scan for the same reason.
// A homelab has single-digit libraries and this is a settings screen. The sort
// is therefore EXPECTED here and is asserted as expected in
// TestListLibrariesOrderingSortIsIntentional; what is NOT allowed to degrade is
// the item count, which grows with the catalogue, and that is what the plan
// guard actually protects. If an ordering index is ever wanted it is a NEW
// migration, never an edit to 0005.
func listLibrariesSQL(scope Scope) (string, []any) {
	args := make([]any, 0, len(scope.InstanceIDs)*2+4)

	countPred, countArgs := scope.workVisibilityPredicate("m.work_id")
	args = append(args, countArgs...)

	args = append(args, UnfiledLibraryID)

	userPred, userArgs := scope.userPredicate("l.user_id")
	args = append(args, userArgs...)

	visPred, visArgs := scope.libraryVisibilityPredicate("l.id")
	args = append(args, visArgs...)

	return `
		SELECT l.id, l.name, l.slug, l.kind, l.formats, l.sort_order, l.enabled,
		       l.include_in_search, l.orphaned_at,
		       (SELECT COUNT(*) FROM library_member m
		         WHERE m.library_id = l.id AND ` + countPred + `)
		  FROM library l
		 WHERE l.id <> ?
		   AND ` + userPred + `
		   AND ` + visPred + `
		 ORDER BY l.sort_order, l.name`, args
}

// librarySourcesSQL renders the source statement for a set of library ids.
//
// THE SELECT LIST IS THE ALLOWLIST OVER service_instance — see LibrarySource.
// Two columns, `si.name` and `si.kind`, chosen one at a time.
//
// `si.deleted_at IS NULL` matches listServiceInstances, which is the read the
// Services screen renders from: an instance the owner deleted is not a service
// any more, and naming it under a library here while §17.3 says it is gone would
// be two screens disagreeing. ⚠️ THE CONSEQUENCE IS STATED RATHER THAN HIDDEN:
// soft-deleting an instance makes its library_source rows vanish from this
// response while the rows themselves remain (the FK cascade fires on a HARD
// delete only), so such a library renders with fewer sources — or with none,
// looking orphaned — and `orphaned_at`, which is the column that should record
// it, is written by nothing. That gap belongs to the sweep, not to this read.
//
// The scope is applied to the SOURCE as well as to the library. A library can be
// visible because one of its sources is on a visible instance while another sits
// on an instance the caller cannot see, and the second must not be listed —
// otherwise the scope decides which libraries appear and then publishes the
// hidden topology inside them anyway.
func librarySourcesSQL(scope Scope, libraryIDs []int64) (string, []any) {
	args := make([]any, 0, len(libraryIDs)+len(scope.InstanceIDs))
	for _, id := range libraryIDs {
		args = append(args, id)
	}
	instPred, instArgs := scope.instancePredicate("ls.service_instance_id")
	args = append(args, instArgs...)

	return `
		SELECT ls.library_id, ls.id, ls.service_instance_id, si.name, si.kind,
		       ls.container_kind, ls.container_ref, ls.container_identity,
		       ls.is_metadata_authority, ls.missing_since
		  FROM library_source ls
		  JOIN service_instance si ON si.id = ls.service_instance_id
		 WHERE ls.library_id IN (` + placeholders(len(libraryIDs)) + `)
		   AND si.deleted_at IS NULL
		   AND ` + instPred + `
		 ORDER BY ls.library_id, ls.id`, args
}

// ListLibraries reads §17.8's Libraries list for one access scope.
//
// SCOPE IS IN THE SIGNATURE AND IN BOTH STATEMENTS. A scope a query accepts and
// never filters on is indistinguishable from no scope at all, which is why
// TestListLibrariesScopeActuallyFilters breaks each predicate in turn and
// watches an assertion catch it rather than trusting that the parameter is
// threaded.
//
// TWO STATEMENTS, NOT ONE JOIN, and the reason is the item count. A single
// library-by-source join fans the library row out once per source, so the
// correlated COUNT over library_member would be evaluated once per source row
// instead of once per library, and the caller would have to de-duplicate a
// number it did not compute. Two statements keep each plan flat and separately
// assertable. The second is skipped entirely when the first returns nothing,
// so a caller with no visible libraries issues one query.
//
// THERE IS NO PAGING, deliberately, and it matches listServiceInstances. A
// user's libraries are a set they created by hand — §17.8's reference install
// has seven — and the row view is a reorderable list, which a keyset window
// cannot express. Growth here is bounded by a person, not by an upstream.
func (s *Store) ListLibraries(ctx context.Context, scope Scope) ([]Library, error) {
	query, args := listLibrariesSQL(scope)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}
	out, err := scanLibraries(rows)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	ids := make([]int64, 0, len(out))
	index := make(map[int64]int, len(out))
	for i, l := range out {
		ids = append(ids, l.ID)
		index[l.ID] = i
	}
	if err := s.attachLibrarySources(ctx, scope, ids, index, out); err != nil {
		return nil, err
	}
	// THE THIRD STATEMENT, and it is a third rather than a join onto the second
	// for the reason the second is not a join onto the first: it fans out per
	// SOURCE and has to be folded back to one verdict per library, which a
	// caller cannot do to a number it did not compute. It reads sync_report,
	// which is an operational log rather than catalogue state, so a library with
	// no verdict is the ordinary case for every source that does not measure
	// completeness at all.
	if err := s.attachLibraryCompleteness(ctx, scope, ids, index, out); err != nil {
		return nil, err
	}
	// THE FOURTH STATEMENT. It reads the sibling axis — what an import read and
	// did not map — and it is INDEPENDENT of the third.
	//
	// ⚠️ IT USED TO REQUIRE THE THIRD TO HAVE RUN FIRST, and that is worth a
	// sentence rather than a silent deletion: its "walked, nothing recorded"
	// state was derived from the completeness verdict, so reordering these two
	// turned every SkipsNone into a nil. ADR-0063 gave the skip read its own zero
	// rows and removed the derivation, so the ordering constraint is gone with
	// it. See LibrarySkips.
	if err := s.attachLibrarySkips(ctx, scope, ids, index, out); err != nil {
		return nil, err
	}
	return out, nil
}

func scanLibraries(rows *sql.Rows) ([]Library, error) {
	defer func() { _ = rows.Close() }()

	var out []Library
	for rows.Next() {
		var l Library
		if err := rows.Scan(&l.ID, &l.Name, &l.Slug, &l.Kind, &l.Formats, &l.SortOrder,
			&l.Enabled, &l.IncludeInSearch, &l.OrphanedAt, &l.ItemCount); err != nil {
			return nil, fmt.Errorf("list libraries: scan: %w", err)
		}
		if l.ID == UnfiledLibraryID {
			// Unreachable: the statement excludes it. It is an error rather
			// than a dropped row because the reserved row reaching a caller is
			// exactly the failure migration 0005 added a trigger for after a
			// comment turned out not to be enough.
			return nil, fmt.Errorf(
				"list libraries: the reserved Unfiled row (library %d) came back from a "+
					"statement that excludes it", UnfiledLibraryID)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}
	return out, nil
}

// libraryCompletenessSQL renders the completeness statement.
//
// # The newest verdict per SOURCE, and how it is picked
//
// sync_report is append-only, so a container that has been imported five times
// has five rows. The correlated `r.id = (SELECT … ORDER BY r2.id DESC LIMIT 1)`
// takes the last one written for that (instance, container_ref) pair. The key is
// `id` rather than `created_at` because created_at is second-granular text and
// two runs inside one second would tie; `id` is the insert order by construction.
//
// ⚠️ IT IS NOT WINDOWED ON last_full_sync_at, AND THAT IS THE OPPOSITE CHOICE
// FROM FileWalkFailuresByInstance ON PURPOSE. That read COUNTS accumulating
// failures, so it has to exclude ones a later run already fixed. This one reads
// the LATEST OBSERVATION, and every import writes a fresh row for every
// container it saw — so the newest row is always the newest run's verdict, and
// windowing it would delete the answer for an instance whose import has not
// completed since. A stale-but-labelled measurement (CheckedAt travels) beats no
// measurement.
//
// The join is `sync_report.remote_id = library_source.container_ref`, which is
// the same string on both sides: internal/libsync's containerRef writes the
// container id into CatalogueContainer.RemoteID, catalogue.go's bind path writes
// that into container_ref, and cmd/usarr writes the same value into remote_id.
// `remote_kind = 'library'` narrows it to container-grained rows, so an
// item-grained kind that reused the column could not collide.
//
// THE SCOPE IS THE SOURCE'S, not the library's, and it is the same
// instancePredicate librarySourcesSQL applies for the same reason: a verdict
// names an instance through the source it hangs off, so publishing one for an
// instance the caller cannot see would leak the topology the scope hides.
// `si.deleted_at IS NULL` matches that read too — a soft-deleted instance is not
// a service any more, and its last verdict is not news about a live library.
//
// ⚠️ THE PLAN IS MEASURED, AND IT ONCE CARRIED A SORT INSIDE THE CORRELATED
// SUBQUERY. ix_sync_report_instance is (service_instance_id, created_at DESC),
// so the `ORDER BY r2.id DESC LIMIT 1` above could not come off it: per
// library_source row, SQLite seeked the instance, filtered
// kind/remote_kind/remote_id unindexed, and sorted the survivors — a
// walked-and-sorted set that grew with IMPORT COUNT, on a render path.
//
// ✅ RESOLVED BY MIGRATION 0011 (2026-08-19), which is why the paragraph above
// is in the past tense and why it is still here. ix_sync_report_container_latest
// is (service_instance_id, kind, remote_kind, remote_id, id) — the subquery's
// four equalities and then its ordering column — so the pick is now a single
// COVERING seek with no sort and no row fetch:
//
//	CORRELATED SCALAR SUBQUERY 1
//	  SEARCH r2 USING COVERING INDEX ix_sync_report_container_latest
//	          (service_instance_id=? AND kind=? AND remote_kind=? AND remote_id=?)
//
// The one temp b-tree left in this statement is the outer
// `ORDER BY ls.library_id, ls.id`, which is bounded by what is on the screen.
//
// ⚠️ THE ORDER BY MUST STAY `r2.id`, AND THE FOUR PREDICATES MUST STAY
// EQUALITIES. Both are what the index is shaped for — turning any of the four
// into a range, or ordering by created_at instead, drops this back to the sorted
// plan above. The measured plan, both scopes, the pre-0011 plan it replaced, and
// the reasoning about what each costs are in
// TestLibraryCompletenessPlanIsSeeksAndOneSort — which EXPLAINs THIS function
// rather than a copy of its text, and is fired by
// TestLibraryCompletenessPlanGuardFires, whose arm 1 drops 0011's index and
// watches the sort come back.
func libraryCompletenessSQL(scope Scope, libraryIDs []int64) (string, []any) {
	return containerReportSQL(SyncReportContentCompleteness, scope, libraryIDs)
}

// librarySkipsSQL renders the newest-skip-row-per-container statement.
//
// ⚠️ IT IS THE SAME STATEMENT TEXT AS libraryCompletenessSQL, BY CONSTRUCTION
// AND NOT BY COINCIDENCE — both delegate to containerReportSQL and differ only
// in the kind they bind. That is what makes the plan guard above cover this read
// too without a second copy of it, and TestTheSkipStatementIsTheCompletenessStatement
// asserts the two texts are byte-identical so the coverage cannot quietly lapse.
func librarySkipsSQL(scope Scope, libraryIDs []int64) (string, []any) {
	return containerReportSQL(SyncReportItemsSkipped, scope, libraryIDs)
}

// containerReportSQL is the shared body: the newest sync_report row of one kind
// per (instance, container_ref) pair, joined onto the sources of the given
// libraries and scoped exactly as librarySourcesSQL scopes them.
//
// TWO KINDS, ONE STATEMENT, and the reason is the paragraph above about
// migration 0011's index: the index is shaped for the four equalities and the
// ordering column this text uses, so a second hand-written copy would be a
// second thing that can drift off it. The kind is a bound parameter rather than
// interpolated text, so the two callers produce the same prepared statement.
func containerReportSQL(kind string, scope Scope, libraryIDs []int64) (string, []any) {
	// ⚠️ THE KIND IS BOUND FIRST, BECAUSE `?` IS POSITIONAL AND THE CORRELATED
	// SUBQUERY IS EARLIER IN THE TEXT THAN THE `IN` LIST. The other statements in
	// this file happen to build their arguments in the same order their
	// predicates read, so the hazard does not arise there; here the parameter
	// that leads the SQL is the one that trails the logic, and getting it wrong
	// produces an empty join rather than an error.
	args := make([]any, 0, len(libraryIDs)+len(scope.InstanceIDs)+1)
	args = append(args, kind)
	for _, id := range libraryIDs {
		args = append(args, id)
	}
	instPred, instArgs := scope.instancePredicate("ls.service_instance_id")
	args = append(args, instArgs...)

	// ⚠️ THE THREE ls COLUMNS AFTER library_id ARE THE CONTAINER'S IDENTITY, AND
	// THE COMPLETENESS CALLER SCANS AND DISCARDS THEM. They are here rather than
	// in a second statement because the two reads share this text on purpose —
	// see librarySkipsSQL — and they cost nothing: library_source is the driving
	// table and its row is already fetched. attachLibrarySkips needs them to say
	// WHICH container each folded row came from, which is the one thing a client
	// cannot reconstruct from a library-level total.
	return `
		SELECT ls.library_id, ls.service_instance_id, ls.container_kind,
		       ls.container_ref, r.detail, r.created_at
		  FROM library_source ls
		  JOIN service_instance si ON si.id = ls.service_instance_id
		  JOIN sync_report r ON r.id = (
		        SELECT r2.id FROM sync_report r2
		         WHERE r2.service_instance_id = ls.service_instance_id
		           AND r2.kind = ?
		           AND r2.remote_kind = 'library'
		           AND r2.remote_id = ls.container_ref
		         ORDER BY r2.id DESC LIMIT 1)
		 WHERE ls.library_id IN (` + placeholders(len(libraryIDs)) + `)
		   AND si.deleted_at IS NULL
		   AND ` + instPred + `
		 ORDER BY ls.library_id, ls.id`, args
}

// attachLibraryCompleteness folds the per-source verdicts onto the rows.
//
// A ROW THIS CANNOT READ IS DROPPED, NOT DEFAULTED. A detail blob that will not
// decode, or one carrying a state string no member of the vocabulary matches, is
// skipped: the library then renders as unmeasured, which is the only reading of
// an unreadable measurement that cannot overstate. Dropping it silently is
// deliberate here, unlike libraryFormatsFor's logged drop, because this read has
// no logger and the row is operational rather than user data — and because the
// state it produces (nothing measured) is itself visible on the screen.
func (s *Store) attachLibraryCompleteness(
	ctx context.Context, scope Scope, ids []int64, index map[int64]int, out []Library,
) error {
	query, args := libraryCompletenessSQL(scope, ids)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("list library completeness: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var libraryID int64
		var container LibrarySkipContainer
		var detail sql.NullString
		var createdAt string
		// The container identity is scanned and DROPPED here: this read folds to
		// a library-level verdict and no caller asks which container produced
		// which part of it. The columns are in the text for the skip read, which
		// does. See containerReportSQL.
		if err := rows.Scan(&libraryID, &container.ServiceInstanceID,
			&container.ContainerKind, &container.ContainerRef, &detail, &createdAt); err != nil {
			return fmt.Errorf("list library completeness: scan: %w", err)
		}
		i, ok := index[libraryID]
		if !ok {
			return fmt.Errorf(
				"list library completeness: a verdict belongs to library %d, which the "+
					"library statement did not return", libraryID)
		}
		if !detail.Valid {
			continue
		}
		var note CompletenessNote
		if err := json.Unmarshal([]byte(detail.String), &note); err != nil {
			continue
		}
		state, ok := CompletenessStateOf(note.State)
		if !ok {
			continue
		}
		out[i].Completeness = foldCompleteness(out[i].Completeness, state, note, createdAt)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list library completeness: %w", err)
	}
	return nil
}

// foldCompleteness merges one container verdict into a library's running one.
//
// See LibraryCompleteness for why `unverified` outranks `shortfall` rather than
// the other way round. The sums accumulate ONLY from measured verdicts, so an
// unverified container contributes a state and no numbers — and because it also
// wins the state, the numbers it did not contribute are never published.
func foldCompleteness(
	into *LibraryCompleteness, state CompletenessState, note CompletenessNote, createdAt string,
) *LibraryCompleteness {
	if into == nil {
		into = &LibraryCompleteness{State: state}
	}
	into.Containers++
	// Lexicographic max is chronological max: store.go's timeLayout is ordered
	// that way on purpose, and the same property is what FileWalkFailuresByInstance's
	// window relies on.
	if createdAt > into.CheckedAt {
		into.CheckedAt = createdAt
	}

	switch state {
	case CompletenessUnverified:
		into.State = CompletenessUnverified
		if into.Reason == "" {
			into.Reason = note.Reason
		}
		// The numbers are dropped rather than kept, because the state they would
		// qualify is now one that makes no numeric claim at all.
		into.Total, into.Visible, into.Hidden = 0, 0, 0
		return into
	case CompletenessShortfall, CompletenessComplete:
		if into.State == CompletenessUnverified {
			// An unverified container already won. Count this one and add
			// nothing: a partial sum published under a total-shaped label is
			// exactly the precision this fold refuses.
			return into
		}
		if state == CompletenessShortfall {
			into.State = CompletenessShortfall
		}
		into.Total += note.Total
		into.Visible += note.Visible
		into.Hidden += note.Hidden
	}
	return into
}

// attachLibrarySkips folds the per-container skip rows onto the rows.
//
// # ONE STATEMENT, NO SECOND PASS, NO NEIGHBOUR (ADR-0063)
//
// Every container an import walked has a skip row, zero or not, so the three
// readings come off these rows alone: a row with a non-zero total is
// SkipsLeftOut, a row with a zero total is SkipsNone, and no row at all is nil.
// This used to end with a second pass that turned "no skip row" into SkipsNone
// for any library that had a COMPLETENESS verdict — the only per-container
// record in the schema that an import had gone near a container. That pass is
// gone, and with it the coupling: this read is now evidence for itself.
//
// A ROW THIS CANNOT READ IS DROPPED, NOT DEFAULTED, on
// attachLibraryCompleteness's reasoning: an undecodable detail blob renders as
// "nothing was recorded", the only reading of an unreadable record that cannot
// overstate. ⚠️ WITH THE SECOND PASS GONE, A LIBRARY WHOSE ONLY ROW IS
// UNREADABLE NOW READS nil RATHER THAN SkipsNone. That is a step further into
// the same safe direction — nil understates what is known and never overstates
// it — and it is the honest one, because what the row recorded is precisely what
// could not be read.
//
// ⚠️ IT NO LONGER DEPENDS ON attachLibraryCompleteness HAVING RUN. See
// ListLibraries.
func (s *Store) attachLibrarySkips(
	ctx context.Context, scope Scope, ids []int64, index map[int64]int, out []Library,
) error {
	query, args := librarySkipsSQL(scope, ids)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("list library skips: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var libraryID int64
		var container LibrarySkipContainer
		var detail sql.NullString
		var createdAt string
		if err := rows.Scan(&libraryID, &container.ServiceInstanceID,
			&container.ContainerKind, &container.ContainerRef, &detail, &createdAt); err != nil {
			return fmt.Errorf("list library skips: scan: %w", err)
		}
		i, ok := index[libraryID]
		if !ok {
			return fmt.Errorf(
				"list library skips: a skip row belongs to library %d, which the library "+
					"statement did not return", libraryID)
		}
		if !detail.Valid {
			continue
		}
		var note SkipNote
		if err := json.Unmarshal([]byte(detail.String), &note); err != nil {
			continue
		}
		out[i].Skips = foldSkips(out[i].Skips, container, note, createdAt)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list library skips: %w", err)
	}
	return nil
}

// foldSkips merges one container's skip row into a library's running verdict.
//
// ⚠️ A ROW WITH A ZERO TOTAL IS THE MEASURED NEGATIVE, NOT A SKIP, and telling
// the two apart is this function's job under ADR-0063. A library that binds two
// containers, one of which left items out, is SkipsLeftOut for the whole
// library: the state starts at SkipsNone and only a non-zero total moves it, so
// the order the rows arrive in cannot change the answer.
//
// ⚠️ THE REASON IS TAKEN ONLY FROM A ROW THAT ACTUALLY LEFT SOMETHING OUT. A
// zero row carries none — cmd/usarr does not write one — and lifting a reason
// off it would explain a skip that did not happen. Belt and braces against a
// future adapter that writes one anyway.
func foldSkips(
	into *LibrarySkips, container LibrarySkipContainer, note SkipNote, createdAt string,
) *LibrarySkips {
	if into == nil {
		into = &LibrarySkips{State: SkipsNone}
	}
	// ⚠️ THE ENTRY IS KEPT WHOLE ALONGSIDE THE SUM RATHER THAN INSTEAD OF IT.
	// Items stays the library's total because that is what §17.8's row renders;
	// the entry says where that total came from. Both are true, and only the
	// second one lets a caller holding two libraries tell a shared container
	// from two distinct ones. See LibrarySkips.Containers.
	container.Items = note.Total()
	into.Containers = append(into.Containers, container)
	into.Items += note.Total()
	if note.Total() > 0 {
		into.State = SkipsLeftOut
		if into.Reason == "" {
			into.Reason = note.Reason
		}
	}
	// Lexicographic max is chronological max: store.go's timeLayout is ordered
	// that way on purpose, the same property foldCompleteness relies on.
	if createdAt > into.RecordedAt {
		into.RecordedAt = createdAt
	}
	return into
}

func (s *Store) attachLibrarySources(
	ctx context.Context, scope Scope, ids []int64, index map[int64]int, out []Library,
) error {
	query, args := librarySourcesSQL(scope, ids)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("list library sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var libraryID int64
		var src LibrarySource
		if err := rows.Scan(&libraryID, &src.ID, &src.ServiceInstanceID,
			&src.ServiceName, &src.ServiceKind, &src.ContainerKind, &src.ContainerRef,
			&src.ContainerIdentity, &src.IsMetadataAuthority, &src.MissingSince); err != nil {
			return fmt.Errorf("list library sources: scan: %w", err)
		}
		i, ok := index[libraryID]
		if !ok {
			// The IN list is built from the ids the first statement returned,
			// so this cannot happen without the two statements disagreeing
			// about which libraries exist.
			return fmt.Errorf(
				"list library sources: source %d belongs to library %d, which the library "+
					"statement did not return", src.ID, libraryID)
		}
		out[i].Sources = append(out[i].Sources, src)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list library sources: %w", err)
	}
	return nil
}
