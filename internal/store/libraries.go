package store

import (
	"context"
	"database/sql"
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
// IT IS A LOCAL READ AND NOTHING ELSE (principle 1). Two statements against the
// local file, no upstream call, no capability probe. In particular this is NOT
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
	return `(NOT EXISTS (SELECT 1 FROM library_source ls
	                      WHERE ls.library_id = ` + column + `)
	         OR EXISTS (SELECT 1 FROM library_source ls
	                     WHERE ls.library_id = ` + column + `
	                       AND ls.service_instance_id IN (` + placeholders(len(args)) + `)))`, args
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
