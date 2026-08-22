package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// §17.8's Accept step, store side: what a connect probe DESCRIBES, and what
// accepting one of its proposals WRITES.
//
// ADR-0048 is the decision this file implements and the one that shapes it: *"A
// library proposal lives in the connect probe's response. It is never persisted.
// A `library` row is created only when the user accepts one."* So there is no
// proposal table here and no proposal state on `library`. DescribeContainers
// computes the proposal set from its two stated inputs — the container list, and
// what is already bound in `library_source` for this user — and AcceptLibraries
// is the single writer that turns one of them into a row.
//
// ⚠️ THE CONTAINER LIST IS AN ARGUMENT AND THIS FILE NEVER ASKS WHERE IT CAME
// FROM. ADR-0048 names the connect probe because that is where the FIRST
// proposal set comes from, and clause 5 excuses that probe's upstream call as a
// setup action. It is not a requirement on this layer, and it must not become
// one: the same containers can be reconstructed from local state — `library_source`
// records every container ever bound and `service_item_link.remote_library_id`
// records every container an item came from — and a Libraries screen that
// re-renders its proposals on every visit wants exactly that, on principle 1.
// So DescribeContainers takes the slice, calls nothing outbound, and issues two
// statements against the local file. A parameter that assumed a live probe, or a
// call into an adapter from here, would close that door.
//
// ⚠️ NEITHER FUNCTION REMOVES CREATION FROM THE BOOTSTRAP IMPORT, and that is
// the other half of the Accept step. ADR-0048's Fact 2 and §17.8 both assign
// that removal to this thread; this commit builds the storage the screen needs
// and leaves `bindOneContainer` reachable from `FullImport` exactly as it is.
// Read `cmd/usarr/import.go` for what still triggers an import.
//
// WHICH SIDE OF THE ACCESS-SCOPE RULE THESE ARE ON (store.go rule 2), which is a
// design question here rather than a style one because catalogue.go's whole file
// is on the other side of it:
//
//   - DescribeContainers is a READ that aggregates across instances — the
//     already-bound half joins `library`, whose rows name service instances
//     through their sources — so the Scope is in the signature and in both
//     statements. TestDescribeContainersScopeActuallyFilters breaks each
//     predicate and watches an assertion catch it.
//   - AcceptLibraries is a WRITE, and catalogue.go's replication writes take no
//     Scope because they have no acting user. This one HAS one: it is a user at
//     a settings screen naming service instances by id. The scope is what says
//     the user may name them. Binding a library to an instance the caller cannot
//     see would publish that instance's item count back through
//     DescribeContainers, which is store.go rule 2's existence oracle written
//     the long way round.
//
// ADR-0048 clause 5 covers the probe's own upstream call and is not this file's
// business: everything here is a statement against the local file.

// ErrLibraryNameTakenAtOtherKind is returned when an acceptance names a library
// that already exists for this user AT A DIFFERENT KIND.
//
// §17.8 specifies the merge: *"Renaming a proposal to match an existing name
// joins rather than creates"*, on a case-insensitive whitespace-trimmed per-user
// key. `library.kind` is exactly one value, so a name collision ACROSS kinds
// cannot join — and unlike the import path, this one must not invent a way out.
//
// ⚠️ IT IS DELIBERATELY NOT bindOneContainer'S ANSWER, AND THE DIFFERENCE IS THE
// USER. That path is an unattended bootstrap with nobody to ask, so its step 3
// disambiguates — `Comics (Books)`, then `Comics (2)` — because a container that
// binds to nothing is a library that never appears. Here a person typed the
// name. Silently storing something other than what they typed is the failure
// §17.8 records against the drawn mockup in the neighbouring case, where a
// rename *"silently changed the shape of the data model"*. The wire layer turns
// this into a 409 naming the conflicting library, and the user picks another
// name or accepts the join.
var ErrLibraryNameTakenAtOtherKind = errors.New(
	"store: that library name is already held by a library of a different kind")

// ErrSourceOutsideScope is returned when an acceptance names a service instance
// the caller's access scope does not admit.
//
// It is the reason AcceptLibraries takes a Scope at all. A library_source row is
// how a container's items reach a library, so accepting one against an unseen
// instance both writes rows the caller may not read and makes that instance's
// contents countable through DescribeContainers afterwards. Failing closed here
// is the write-side half of the predicate every read in this file carries.
var ErrSourceOutsideScope = errors.New(
	"store: that source names a service instance outside the caller's access scope")

// BoundLibrary is one library that already stands over a container.
type BoundLibrary struct {
	ID   int64
	Name string
	Kind string
}

// ContainerProposal is one row of §17.8's Accept step: a container the connected
// service itself reported, plus what UsArr already knows about it.
//
// ⚠️ IT HAS TWO CONSTRUCTORS AND THEY FILL DIFFERENT FIELDS, which is the one
// thing to know before reading a value of this type:
//
//   - DescribeContainers is handed a container list and ONE instance id. It
//     fills the adapter's own five fields, ItemCount, BoundTo and
//     ServiceInstanceID, and it leaves every field below ServiceInstanceID at
//     its zero value — it has no observation row to read them from, and no
//     second statement is worth issuing to invent them.
//   - ProposedContainers reads the container list off local state, across every
//     instance in scope, and fills ALL of them. It is what §17.8's screen calls,
//     and it composes with DescribeContainers rather than duplicating it.
//
// So a zero ObservedAt means "nobody asked", never "never observed". A caller
// that needs the local-state fields calls the function that fills them; there is
// deliberately no flag saying which constructor ran, because a reader that has
// to branch on that is reading the wrong function's output.
type ContainerProposal struct {
	// RemoteID, Name, Kind, DeclineReason and KindProvisional are the adapter's
	// answer, forwarded unchanged. See CatalogueContainer for what each means;
	// an empty Kind is a DECLINED container and DeclineReason carries the word
	// §17.8's `Decision` column renders.
	RemoteID        string
	Name            string
	Kind            string
	DeclineReason   string
	KindProvisional bool

	// ItemCount is how many top-level works from this container are ALREADY in
	// the replica — §17.8's item count, computed before the library exists.
	//
	// It is the whole reason ADR-0048's 2026-08-21 amendment runs the first
	// import BEFORE Accept: *"You'd be ticking proposals with real item counts
	// beside them"*, against the alternative of *"deciding blind"*. Those works
	// are sitting unfiled — a work with no library_member row at all, which is
	// what facets.go means by unfiled and is NOT membership of library 0 — until
	// an acceptance files them.
	//
	// Child kinds are excluded, because they are excluded from membership: a
	// comic library that counted its issues would report a number
	// /library/comics disagrees with. See childKindsExcluded.
	ItemCount int64

	// BoundTo is the libraries this container is already a source of, empty when
	// it is unbound.
	//
	// ⚠️ IT IS A SLICE AND IT CANNOT BE ONE ID, which reads as over-engineering
	// until ADR-0066 decision 5. `library_source`'s uniqueness is
	// (library_id, service_instance_id, container_kind, container_ref), so one
	// container ref may legitimately name TWO libraries — the BookOrbit mixed
	// container that becomes a `book` library and a `comic` library once comics
	// have a unit of work (ADR-0068). bindOneContainer's step 1 already carries
	// `AND l.kind = ?` for exactly that reason, and a single-valued field here
	// would make the Accept screen show whichever of the two SQLite returned
	// first.
	//
	// ⚠️ A NON-EMPTY BoundTo DOES NOT MEAN "not a proposal". ProposedContainers
	// drops a container that is bound AT THE OBSERVED KIND and keeps one that is
	// bound only at another, for decision 5's reason above: the second kind over
	// a mixed container is a genuine proposal, and the library it would sit
	// beside is exactly what the screen needs to show.
	BoundTo []BoundLibrary

	// ─── Filled by ProposedContainers only. See the type's own comment. ───

	// ServiceInstanceID is the instance that reported this container.
	// DescribeContainers fills it too, from its argument.
	ServiceInstanceID int64

	// ServiceName and ServiceKind are `service_instance.name` and `.kind`.
	//
	// ⚠️ THESE TWO COLUMNS AND NO OTHERS CROSS THIS LAYER off that table, which
	// is a §14 rule rather than a preference: `service_instance` carries
	// `api_key_enc`, a full-admin *Arr credential, and `base_url`, an internal
	// host the user typed. internal/httpapi/libraries.go's librarySourceResponse
	// allowlists the same two onto the wire and states the rule for the whole
	// screen — the name is the chip's label and the kind is its icon. A field
	// added here is a field a wire struct can copy.
	ServiceName string
	ServiceKind string

	// ObservedAt is when the container_observed row this proposal was read from
	// was written, in the store's own timestamp layout.
	//
	// PER PROPOSAL, NEVER COLLAPSED TO ONE PER INSTANCE. Every observation row
	// is written inside the bind transaction, so a run that fails partway leaves
	// some of an instance's containers stamped by this run and the rest by an
	// earlier one — and an instance-level stamp would date all of them by
	// whichever run touched any of them. It is also the input to
	// NotSeenByLastSync, which is a per-container question.
	ObservedAt string

	// InstanceLastFullSyncAt is `service_instance.last_full_sync_at`, empty when
	// the instance has never completed a full sync. Empty is unambiguous — no
	// timestamp renders as the empty string — so this is a plain string rather
	// than a sql.NullString.
	InstanceLastFullSyncAt string

	// NotSeenByLastSync says the last COMPLETED full sync did not report this
	// container. It is SERVER-SET, computed from the two timestamps above; see
	// notSeenByLastSync for the comparison and for which way an equal timestamp
	// falls.
	//
	// ⚠️ IT IS THE ONLY THING THAT CAN SAY A PROPOSAL WENT AWAY. A bound
	// container has `library_source.missing_since`, maintained by the whole-list
	// sweep and cleared on every rebind. An unbound one has no library_source
	// row at all, so nothing else in the schema records that it stopped being
	// reported — its sync_report row simply stops being replaced. See
	// SyncReportContainerObserved.
	NotSeenByLastSync bool

	// SuggestedName is the name the Accept screen offers, pre-filled and
	// editable. It is the derivation the bind path used to own; see
	// suggestedLibraryName.
	SuggestedName string

	// JoinsLibraryID and JoinsLibraryName are the library that accepting this
	// proposal AS SUGGESTED would join rather than create — zero and empty when
	// it would create one.
	//
	// It is a PREDICTION, and it is computed by calling the same userLibraries
	// the writer calls, on §17.8's same merge key, at the same kind — so the
	// screen's "this will join Comics" and AcceptLibraries' actual join are one
	// derivation rather than two that agree today. It predicts for the SUGGESTED
	// name only: a user who edits the name in place moves the answer, and
	// recomputing it while they type is the browser's job, not this read's.
	JoinsLibraryID   int64
	JoinsLibraryName string
}

// Declined reports whether the adapter refused this container.
//
// A METHOD RATHER THAN A FIELD, and that is the whole point: "declined" and "no
// kind" are ONE fact — CatalogueContainer documents DeclineReason as *"set
// exactly when Kind is ”"* — so a bool beside Kind would be a second copy of it
// that compiles clean while disagreeing, which is DEVELOPMENT.md §11's defect
// class exactly. The wire layer wants a `declined` field; it gets it by calling
// this, so there is one derivation and no way to set one without setting the
// other.
func (p ContainerProposal) Declined() bool { return p.Kind == "" }

// LibraryAcceptance is one accepted proposal, as the caller edited it.
type LibraryAcceptance struct {
	// Name and Kind are what the user is accepting, after any inline edit.
	// Name is matched on §17.8's merge key, so the case and the surrounding
	// whitespace of what they typed do not decide whether it joins.
	Name string
	Kind string

	// Formats is `library.formats`: the format filter over `edition.format`,
	// nil meaning "any" and stored as NULL. It is the column §17.8's flagship
	// Ebooks/Audiobooks split lands on, and this is its FIRST writer.
	Formats []string

	// ManagedBy is 'auto' or 'user', and it is the CALLER'S DECISION rather than
	// this layer's. §17.8's one-way door is a fact about the SCREEN — *"Editing
	// any proposal marks that library user-managed"* — and only the screen knows
	// whether a row was edited. The store validates the value and stores it.
	//
	// ⚠️ THIS IS THE FIRST CODE PATH THAT CAN WRITE 'user'. ADR-0048's Fact 1 and
	// §17.8 both measured that no path ever had, which is why the door was
	// specified with no hinge. Nothing READS the column yet: what a later connect
	// may offer against a user-managed library is ADR-0048's open question 2 and
	// is not answered here.
	ManagedBy string

	// Sources are the containers this library is bound to. Two proposals merged
	// by §17.8's rename rule arrive as ONE acceptance carrying both.
	Sources []AcceptedSource
}

// AcceptedSource is one container binding an acceptance asks for.
type AcceptedSource struct {
	ServiceInstanceID int64

	// ContainerKind is `library_source.container_kind`. ⚠️ ONLY
	// 'remote_library' HAS A MEMBERSHIP DERIVATION IN THE TREE. §6.5 rule 3
	// tabulates a different predicate per kind — a path prefix for
	// 'root_folder', `remote_tag_ids` for 'tag', `remote_subtype` for
	// 'series_type' — and this file implements the one v0.1's adapters produce.
	// The others are refused loudly rather than accepted into a library that
	// would then file nothing and read as empty.
	ContainerKind string

	// ContainerRef is the upstream's own id for the container, verbatim: the
	// value that lands in `service_item_link.remote_library_id` and is the other
	// half of ix_sil_container's key.
	ContainerRef string

	// ContainerIdentity is the container's own name as the upstream reported it,
	// recorded so an upstream that reuses ids cannot silently rebind the library
	// to a different container.
	ContainerIdentity string
}

// AcceptedLibrary is what one acceptance did.
type AcceptedLibrary struct {
	ID   int64
	Name string
	Slug string
	Kind string

	// Created and Joined are exclusive and exactly one is true. They are two
	// fields rather than one because §17.8 requires the screen to SAY which
	// happened — *"Joining Kavita Manga into Comics as a second source."* — and
	// a caller reading `!Created` as "joined" is the inference CatalogueBinding
	// already documents going wrong.
	Created bool
	Joined  bool

	// MembersFiled is how many library_member rows this acceptance wrote. It is
	// the count of works that MOVED from unfiled into this library, so on a
	// re-accept of the same container it is 0 rather than the library's size.
	MembersFiled int64
}

// ProposedContainers is §17.8's Accept screen: every container UsArr has been
// told about that is not already a library, across every instance in scope.
//
// # IT IS THE READ THAT MAKES /settings/libraries A SCREEN RATHER THAN A MOMENT
//
// ADR-0048 clause 1 computes the proposal set from *"what the connected instance
// reports as its containers"*, and clause 5 excuses the connect probe's upstream
// call as a setup action. Neither gives a screen the user NAVIGATES to anything
// to render: a proposal that exists only inside a probe response is available
// for a few seconds after adding a service and never again. This function reads
// the replicated half — `container_observed` rows, written by the bind
// transaction — so the screen renders from the local file on every visit, which
// is what principle 1 requires of a user-facing read.
//
// ⚠️ NO UPSTREAM CALL, NO ADAPTER, NO PROBE. This file imports nothing from
// internal/libsync and nothing from an adapter package, and it must stay that
// way: the whole argument above collapses the moment a render path can block on
// a Kavita that is off. The container list is reconstructed from
// SyncReportContainerObserved's detail blob, which exists for exactly this.
//
// # STILL COMPOSED WITH DescribeContainers, NOT COPIED FROM IT
//
// The item count and the already-bound set are DescribeContainers' two
// statements, argued there, and they are called here rather than re-rendered.
// That costs one pair of statements per instance instead of one pair overall —
// a homelab has single-digit instances, so it is single-digit statements against
// a local file — and it buys the thing a fused statement would lose: the
// membership derivation, the child-kind exclusion, the tombstone handling and
// both scope predicates stay written once. A second copy of the count is a
// second answer to *"how many items does this container have"*, and §17.8's
// screen is the one place where two such answers are visible side by side.
//
// # WHAT IS DROPPED, AND WHAT IS DELIBERATELY NOT
//
// A container already bound AT THE OBSERVED KIND is not a proposal — accepting
// it would be a no-op — and it is dropped. A container bound only at ANOTHER
// kind is KEPT, because ADR-0066 decision 5 puts two libraries over one mixed
// container and the second one is a real proposal; its BoundTo says what it
// would sit beside. See boundAtKind.
//
// A DECLINED container is kept, with its reason. §17.8 requires it be *"declined
// with a reason"* in the `Decision` column, and DescribeContainers keeps it for
// the same requirement. It can never be dropped by the rule above: its Kind is
// "" and `library.kind` is never "".
//
// A container the last completed sync did not report is ALSO kept, flagged
// rather than hidden — see NotSeenByLastSync. Hiding it would make a container
// that vanished upstream indistinguishable from one that was never there, on the
// only screen that could say so.
func (s *Store) ProposedContainers(ctx context.Context, scope Scope) ([]ContainerProposal, error) {
	observed, err := s.observedContainers(ctx, scope)
	if err != nil {
		return nil, err
	}
	if len(observed) == 0 {
		return nil, nil
	}

	// The user's libraries, read ONCE for the whole batch, by the same function
	// resolveAcceptedLibrary calls. This is a read with no writer racing it, so
	// unlike AcceptLibraries' deliberate re-read per acceptance there is nothing
	// for a second read to see.
	held, err := userLibraries(ctx, s.db.Read(), scope.UserID)
	if err != nil {
		return nil, err
	}

	var out []ContainerProposal
	for _, inst := range observed {
		cs := make([]CatalogueContainer, 0, len(inst.containers))
		at := make(map[string]string, len(inst.containers))
		for _, o := range inst.containers {
			cs = append(cs, o.container)
			at[o.container.RemoteID] = o.observedAt
		}
		described, err := s.DescribeContainers(ctx, scope, inst.id, cs)
		if err != nil {
			return nil, err
		}
		for _, p := range described {
			if boundAtKind(p.BoundTo, p.Kind) {
				continue
			}
			// KEYED BY REF, NOT BY POSITION. DescribeContainers does return one
			// proposal per input in order, but that is its loop's business and
			// not a promise in its signature; the newest-row-per-container pick
			// below already guarantees one observation per ref per instance, so
			// the map is exact and cannot be desynchronised by a reordering.
			p.ObservedAt = at[p.RemoteID]
			p.ServiceName = inst.name
			p.ServiceKind = inst.kind
			p.InstanceLastFullSyncAt = inst.lastFullSyncAt
			p.NotSeenByLastSync = notSeenByLastSync(p.ObservedAt, inst.lastFullSyncAt)
			p.SuggestedName = suggestedLibraryName(p.Name, p.RemoteID)
			if joins, ok := held.joinable[libraryNameKey(p.SuggestedName)]; ok && joins.kind == p.Kind {
				p.JoinsLibraryID = joins.id
				p.JoinsLibraryName = joins.name
			}
			out = append(out, p)
		}
	}
	return out, nil
}

// boundAtKind is the "this is not a proposal any more" test: is this container
// already a source of a library OF THE KIND THE ADAPTER LAST REPORTED IT AT?
//
// ⚠️ THE KIND IS THE WHOLE PREDICATE AND A REF-ONLY TEST IS WRONG. It is the
// same question bindOneContainer's step 1 asks with `AND l.kind = ?`, for
// ADR-0066 decision 5's reason: `library_source`'s uniqueness is
// (library_id, service_instance_id, container_kind, container_ref), so one
// BookOrbit container legitimately feeds a `book` library and a `comic` library.
// Dropping on the ref alone would make the second kind unproposable — the mixed
// container's comics would have no way to become a library, on the one screen
// that exists to create one.
//
// A DECLINED container (Kind "") can never match, and does not need a special
// case to say so: `library.kind` is NOT NULL with a CHECK over real work kinds,
// so no library is ever at kind "".
func boundAtKind(bound []BoundLibrary, kind string) bool {
	for _, b := range bound {
		if b.Kind == kind {
			return true
		}
	}
	return false
}

// suggestedLibraryName is the name §17.8's Accept screen pre-fills, from what
// upstream called the container.
//
// ⚠️ THIS DERIVATION USED TO LIVE IN bindOneContainer AND IT MOVED HERE WITH THE
// DECISION IT SERVES. Before ADR-0048's Accept step the import derived a name
// and created a library with it, unattended; now nothing creates a library
// without a user, so the derivation's whole job is to fill a text box the user
// can overwrite. It is deliberately the SAME derivation — trim, and fall back to
// the ref — so a name accepted unchanged is the name the import would have
// picked, and an install that predates Accept does not see its libraries
// re-proposed under different names.
//
// ⚠️ AND IT DOES NOT DISAMBIGUATE, unlike the path it came from. That path
// minted `Comics (Books)` and `Comics (2)` because a container that binds to
// nothing is a library that never appears and there was nobody to ask. Here
// there is somebody to ask: a suggested name that collides is a JOIN prediction
// (JoinsLibraryID) or a refusal the user resolves by typing, and inventing a
// qualifier they did not choose is the failure §17.8 records against the drawn
// mockup — a rename that *"silently changed the shape of the data model"*.
func suggestedLibraryName(name, ref string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return "Library " + ref
}

// notSeenByLastSync reports whether the last COMPLETED full sync failed to
// report this container.
//
// # THE COMPARISON, AND WHICH WAY AN EQUAL TIMESTAMP FALLS
//
// SEEN is `observedAt >= lastFullSyncAt`, NON-STRICT, so this function returns
// `observedAt < lastFullSyncAt` and AN EQUAL TIMESTAMP COUNTS AS SEEN.
//
// It is the same window FileWalkFailuresByInstance applies to the same column
// pair — `created_at >= i.last_full_sync_at` — and it is non-strict for the same
// two reasons. First, `last_full_sync_at` holds the run's START time, not its
// finish (StampFullSync), so every observation a completed run wrote is at or
// after the stamp by construction. Second, both values are second-granular text
// in the store's own layout, so an observation written in the same second the
// next run started is INDISTINGUISHABLE from one written just after it.
//
// The equal case therefore has to fall one way or the other on no evidence, and
// SEEN is the direction whose failure is smaller and self-correcting: claiming a
// container is still there when it has just gone shows a proposal that fails on
// accept and disappears at the next import, whereas claiming a container has
// gone when it has not puts a "no longer reported" flag on a live library's
// source on the settings screen, which is a fact the user cannot check.
//
// A LEXICOGRAPHIC COMPARISON IS A CHRONOLOGICAL ONE HERE, and only because both
// operands are written through the same fixed-width zero-padded UTC layout —
// `last_full_sync_at` through FormatTime, `sync_report.created_at` through
// SQLite's `datetime('now')` default, which emits that same layout. store.go's
// timeLayout comment states the property; libraries.go's completeness fold
// already relies on it.
//
// AN INSTANCE THAT HAS NEVER COMPLETED A FULL SYNC returns false for every
// container, because there is no completed run for one to have been missed by.
// The rows it does have came from runs that really happened, which is
// FileWalkFailuresByInstance's `IS NULL` arm reading the same way.
//
// 🔍 THAT BRANCH IS REDUNDANT, EXPLICIT ON PURPOSE, AND MEASURED TO BE BOTH —
// migration 0011's trailing `id` column, the same argument in a different place.
// The empty string sorts before every timestamp this layout produces, so
// `observedAt < ""` is already false for every real observation and DELETING the
// branch changes no answer. It is written out anyway because the property it
// rests on is a fact about a string comparison, not about the domain: a reader
// meeting a bare `observedAt < lastFullSyncAt` has to derive "never synced means
// never flagged" from ASCII ordering, and a later rewrite that coalesced the
// missing stamp to anything else — an epoch, a far-future sentinel, a zero
// time.Time — would silently flag either nothing or everything.
//
// ⚠️ SO THE DRILL ON IT IS PARTIAL AND THE LIMIT IS RECORDED HERE: deleting the
// branch does not turn TestProposedContainersFlagsNothingOnANeverSyncedInstance
// red (measured, 2026-08-22), because there is no behaviour to lose. Making it
// ANSWER `true` does. The test guards the answer; nothing can guard the branch,
// and pretending otherwise would be a guard that has never been triggered.
func notSeenByLastSync(observedAt, lastFullSyncAt string) bool {
	if lastFullSyncAt == "" {
		return false
	}
	return observedAt < lastFullSyncAt
}

// observedInstance is one service instance's container list as the local file
// holds it.
type observedInstance struct {
	id             int64
	name           string
	kind           string
	lastFullSyncAt string
	containers     []observedContainer
}

// observedContainer is one container_observed row, decoded.
type observedContainer struct {
	container  CatalogueContainer
	observedAt string
}

// observedContainersSQL renders the newest-observation-per-container statement.
//
// # THE SAME SHAPE AS containerReportSQL, AND ON PURPOSE
//
// The newest row per (instance, container) is picked by the same correlated
// `r.id = (SELECT r2.id … ORDER BY r2.id DESC LIMIT 1)` the Libraries screen's
// completeness read uses, with the same four equalities in the same order,
// because that is what migration 0011's ix_sync_report_container_latest is
// shaped for: (service_instance_id, kind, remote_kind, remote_id, id). `id`
// rather than `created_at` is the ordering key for that read's reason too —
// created_at is second-granular text and two runs inside one second would tie,
// where `id` is insert order by construction.
//
// ⚠️ IT DRIVES FROM service_instance, WHICH containerReportSQL DOES NOT, and the
// difference is the point. That read starts from a list of library ids the
// caller already has; this one has no list, because a PROPOSAL is precisely a
// container with no `library_source` row to be listed from. So the driving table
// is the instance set the scope admits, and `sync_report` is joined on the
// index's leading three columns.
//
// ⚠️ `CROSS JOIN` IS A JOIN-ORDER BARRIER AND IT IS LOAD-BEARING, NOT STYLE.
// SQLite documents CROSS JOIN as suppressing the join reordering optimisation,
// and that is the only reason it is written here — the semantics are an ordinary
// inner join and the ON clause is doing the joining. MEASURED on this engine
// (github.com/ncruces/go-sqlite3, SQLite 3.53.4), this schema, no ANALYZE, with
// a plain JOIN and OWNER scope — where the instance predicate renders as `1=1`
// and leaves the planner nothing to seek service_instance on:
//
//	SCAN r USING INDEX ix_sync_report_container_latest
//	SEARCH si USING INTEGER PRIMARY KEY (rowid=?)
//	CORRELATED SCALAR SUBQUERY 1
//	  SEARCH r2 USING COVERING INDEX ix_sync_report_container_latest (…)
//	USE TEMP B-TREE FOR ORDER BY
//
// — the whole index, EVERY kind of report in it, plus a sort. With the barrier:
//
//	SCAN si
//	SEARCH r USING INDEX ix_sync_report_container_latest
//	                (service_instance_id=? AND kind=? AND remote_kind=?)
//	CORRELATED SCALAR SUBQUERY 1
//	  SEARCH r2 USING COVERING INDEX ix_sync_report_container_latest
//	                (service_instance_id=? AND kind=? AND remote_kind=? AND remote_id=?)
//
// and NO SORT AT ALL, in either scope. The scan is of `service_instance`, one row
// per configured service, which is the set this read is about; both orderings
// come off the index's own key order. TestProposedContainersPlanIsSeeksAndNoSort
// pins it — EXPLAINing this function rather than a copy of its text — and
// TestProposedContainersPlanGuardFires has an arm that removes the barrier and
// watches the scan and the sort come back.
//
// ⚠️ THE KIND IS BOUND TWICE AND THE ARGUMENT ORDER IS NOT THE LOGICAL ORDER.
// `?` is positional: the join's `r.kind = ?` comes first in the TEXT, the scope
// predicate second, and the subquery's `r2.kind = ?` last. containerReportSQL
// carries the same hazard and states it the same way — getting it wrong here
// produces an empty result rather than an error.
//
// A ROW WITH A NULL remote_id CANNOT APPEAR, and does not need excluding: the
// correlated `r2.remote_id = r.remote_id` is never true for NULL, so such a row
// drops itself. It would be a container with no upstream id, which nothing could
// bind or file anything from anyway.
//
// `si.deleted_at IS NULL` matches librarySourcesSQL and containerReportSQL: a
// soft-deleted instance is not a service any more, and its containers are not
// offers.
func observedContainersSQL(scope Scope) (string, []any) {
	args := make([]any, 0, len(scope.InstanceIDs)+2)
	args = append(args, SyncReportContainerObserved)
	instPred, instArgs := scope.instancePredicate("si.id")
	args = append(args, instArgs...)
	args = append(args, SyncReportContainerObserved)

	// ⚠️ EXACTLY TWO service_instance COLUMNS BESIDES THE ID AND THE SYNC STAMP.
	// That table carries `api_key_enc` and `base_url`; §14 and
	// internal/httpapi/libraries.go's librarySourceResponse allow the NAME and
	// the KIND across this layer and nothing else. `SELECT si.*` here would be a
	// credential one struct-copy away from a browser.
	return `
		SELECT si.id, si.name, si.kind, si.last_full_sync_at,
		       r.remote_id, r.detail, r.created_at
		  FROM service_instance si
		  CROSS JOIN sync_report r ON r.service_instance_id = si.id
		                          AND r.kind = ?
		                          AND r.remote_kind = 'library'
		 WHERE si.deleted_at IS NULL
		   AND ` + instPred + `
		   AND r.id = (
		         SELECT r2.id FROM sync_report r2
		          WHERE r2.service_instance_id = si.id
		            AND r2.kind = ?
		            AND r2.remote_kind = 'library'
		            AND r2.remote_id = r.remote_id
		          ORDER BY r2.id DESC LIMIT 1)
		 ORDER BY si.id, r.remote_id`, args
}

// observedContainers reads the newest observation of every container on every
// instance in scope, grouped by instance in id order.
//
// A DETAIL BLOB THAT WILL NOT DECODE IS DROPPED, NOT DEFAULTED, on
// attachLibraryCompleteness's terms. The alternative is worse than it looks: the
// ref is on the row, so a defaulted proposal WOULD render — with no name and no
// kind, which is exactly how this screen renders a DECLINED container. A row
// UsArr cannot read would appear as a container upstream refused, with no reason
// beside it, and that is a sentence about the upstream that nothing observed.
// recordContainerObservation is the only writer of this kind and marshals a
// struct that cannot fail, so a blob that will not decode came from somewhere
// else.
func (s *Store) observedContainers(ctx context.Context, scope Scope) ([]observedInstance, error) {
	query, args := observedContainersSQL(scope)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read observed containers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []observedInstance
	for rows.Next() {
		var (
			id             int64
			name, kind     string
			lastFullSync   sql.NullString
			ref, createdAt string
			detail         sql.NullString
		)
		if err := rows.Scan(&id, &name, &kind, &lastFullSync, &ref, &detail, &createdAt); err != nil {
			return nil, fmt.Errorf("read observed containers: scan: %w", err)
		}
		var o containerObservation
		if err := json.Unmarshal([]byte(detail.String), &o); err != nil {
			continue
		}
		// The statement is ordered by si.id, so a new instance is always a new
		// group and the grouping needs no map.
		if len(out) == 0 || out[len(out)-1].id != id {
			out = append(out, observedInstance{
				id: id, name: name, kind: kind, lastFullSyncAt: lastFullSync.String,
			})
		}
		g := &out[len(out)-1]
		g.containers = append(g.containers, observedContainer{
			container: CatalogueContainer{
				RemoteID:        ref,
				Name:            o.Name,
				Kind:            o.Kind,
				DeclineReason:   o.DeclineReason,
				KindProvisional: o.KindProvisional,
			},
			observedAt: createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read observed containers: %w", err)
	}
	return out, nil
}

// DescribeContainers turns the containers an adapter reported into §17.8's
// proposal rows, for one service instance.
//
// It is ADR-0048 clause 1's *"value computed by the probe"*, and it is
// recomputed on every call rather than stored: *"a proposal that is a function
// of its inputs is current by construction"*. Nothing here writes, and nothing
// here calls upstream — the containers arrive as an argument, from a live probe
// or from local state, and this function cannot tell the difference. See the
// file header for why that has to stay true.
//
// TWO STATEMENTS, NOT ONE JOIN, for listLibrariesSQL's reason. The item count
// and the already-bound set are different grains — one work row per item, one
// library row per binding — and a single join would fan the count out per bound
// library and hand the caller a number to de-duplicate that it did not compute.
// Both are skipped entirely when the container list holds nothing to ask about.
//
// A DECLINED CONTAINER IS DESCRIBED, NOT DROPPED. §17.8 requires it be *"declined
// with a reason"* in the `Decision` column, so it travels with its reason and
// with whatever counts it has; whether a decline should be REMEMBERED across
// probes is ADR-0048's open question 3 and is not answered here.
func (s *Store) DescribeContainers(
	ctx context.Context, scope Scope, instanceID int64, cs []CatalogueContainer,
) ([]ContainerProposal, error) {
	out := make([]ContainerProposal, 0, len(cs))
	refs := make([]string, 0, len(cs))
	seen := make(map[string]bool, len(cs))
	for _, c := range cs {
		out = append(out, ContainerProposal{
			RemoteID:          c.RemoteID,
			Name:              c.Name,
			Kind:              c.Kind,
			DeclineReason:     c.DeclineReason,
			KindProvisional:   c.KindProvisional,
			ServiceInstanceID: instanceID,
		})
		// One ref may be reported twice by a caller that is describing the same
		// container under two proposals. The IN list is de-duplicated so the
		// statement does not carry the repeat; the proposals are not.
		if !seen[c.RemoteID] {
			seen[c.RemoteID] = true
			refs = append(refs, c.RemoteID)
		}
	}
	if len(refs) == 0 {
		return out, nil
	}

	counts, err := s.containerItemCounts(ctx, scope, instanceID, refs)
	if err != nil {
		return nil, err
	}
	bound, err := s.containerBindings(ctx, scope, instanceID, refs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].ItemCount = counts[out[i].RemoteID]
		out[i].BoundTo = bound[out[i].RemoteID]
	}
	return out, nil
}

// childKindsExcluded renders `childKinds` as a NOT IN predicate plus its
// arguments, over one work-kind column.
//
// ⚠️ IT IS GENERATED FROM THE MAP AND MUST NEVER BECOME A SQL LITERAL. The map
// is what applyOneItem step 8 tests to decide whether a row is filed into a
// library at all, so a second copy of the list in a string is two membership
// derivations that agree until somebody adds a kind to one of them. That class
// of defect compiles clean and no gate catches it (DEVELOPMENT.md §11).
//
// The order is sorted rather than the map's, because a predicate whose argument
// order changes per process makes a query-plan guard and a golden statement
// unreproducible for no gain.
func childKindsExcluded(column string) (string, []any) {
	kinds := make([]string, 0, len(childKinds))
	for k := range childKinds {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	args := make([]any, 0, len(kinds))
	for _, k := range kinds {
		args = append(args, k)
	}
	return column + " NOT IN (" + placeholders(len(args)) + ")", args
}

// containerItemCountsSQL renders the item-count statement and its arguments.
//
// THE JOIN ix_sil_container EXISTS FOR. Migration 0005 declares it
// `(service_instance_id, remote_library_id) WHERE deleted_at IS NULL AND
// remote_library_id IS NOT NULL` with the comment *"The membership derivation
// walks links per instance per container. Without this it is a full scan of
// every link on the instance for every library the instance feeds."* This is the
// first read to use it, and TestAcceptFilingPlanIsASeek pins it.
//
// COUNT(DISTINCT w.id) RATHER THAN COUNT(*), because ux_sil is
// (service_instance_id, remote_kind, remote_id): one work can hold two links in
// one container under two remote kinds, and the proposal counts WORKS.
//
// The scope is on the count for listLibrariesSQL's reason — a count that
// silently included works from instances the caller cannot see is the rollup
// store.go rule 2 names as an existence oracle. Under the owner's full scope it
// renders as `1=1` and costs nothing.
func containerItemCountsSQL(scope Scope, instanceID int64, refs []string) (string, []any) {
	args := make([]any, 0, len(refs)+len(childKinds)+len(scope.InstanceIDs)+1)
	args = append(args, instanceID)
	for _, r := range refs {
		args = append(args, r)
	}
	kindPred, kindArgs := childKindsExcluded("w.kind")
	args = append(args, kindArgs...)
	visPred, visArgs := scope.workVisibilityPredicate("w.id")
	args = append(args, visArgs...)

	return `
		SELECT sil.remote_library_id, COUNT(DISTINCT w.id)
		  FROM service_item_link sil
		  JOIN work w ON w.id = sil.work_id
		 WHERE sil.service_instance_id = ?
		   AND sil.remote_library_id IN (` + placeholders(len(refs)) + `)
		   AND sil.deleted_at IS NULL
		   AND w.deleted_at IS NULL
		   AND ` + kindPred + `
		   AND ` + visPred + `
		 GROUP BY sil.remote_library_id`, args
}

func (s *Store) containerItemCounts(
	ctx context.Context, scope Scope, instanceID int64, refs []string,
) (map[string]int64, error) {
	query, args := containerItemCountsSQL(scope, instanceID, refs)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count items per container on service instance %d: %w", instanceID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]int64, len(refs))
	for rows.Next() {
		var ref string
		var n int64
		if err := rows.Scan(&ref, &n); err != nil {
			return nil, fmt.Errorf("count items per container on service instance %d: scan: %w",
				instanceID, err)
		}
		out[ref] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count items per container on service instance %d: %w", instanceID, err)
	}
	return out, nil
}

// containerBindingsSQL renders the already-bound statement and its arguments.
//
// THE RESERVED ROW IS EXCLUDED IN THE SQL, matching listLibrariesSQL and
// LibraryIDsBySlug rather than catalogue.go's identity comparison in Go. Nothing
// can bind a source to library 0 today — userLibraries drops it from `joinable`
// so no container may join it — so this is a second lock on a door that is
// already shut, and it is here because the failure it prevents is silent:
// "Unfiled" appearing on the Accept screen as a library a container is already
// part of is exactly ADR-0048's *"a number that is wrong"*.
//
// BOTH SCOPE PREDICATES, because a library row is user-scoped property (principle
// 4, carried by userPredicate) AND names service instances through its sources
// (access scope, carried by libraryVisibilityPredicate). They are different
// questions and libraries.go keeps them apart for the same reason.
func containerBindingsSQL(scope Scope, instanceID int64, refs []string) (string, []any) {
	args := make([]any, 0, len(refs)+len(scope.InstanceIDs)+4)
	args = append(args, instanceID)
	for _, r := range refs {
		args = append(args, r)
	}
	args = append(args, UnfiledLibraryID)

	userPred, userArgs := scope.userPredicate("l.user_id")
	args = append(args, userArgs...)
	visPred, visArgs := scope.libraryVisibilityPredicate("l.id")
	args = append(args, visArgs...)

	return `
		SELECT ls.container_ref, l.id, l.name, l.kind
		  FROM library_source ls
		  JOIN library l ON l.id = ls.library_id
		 WHERE ls.service_instance_id = ?
		   AND ls.container_kind = 'remote_library'
		   AND ls.container_ref IN (` + placeholders(len(refs)) + `)
		   AND l.id <> ?
		   AND ` + userPred + `
		   AND ` + visPred + `
		 ORDER BY ls.container_ref, l.id`, args
}

func (s *Store) containerBindings(
	ctx context.Context, scope Scope, instanceID int64, refs []string,
) (map[string][]BoundLibrary, error) {
	query, args := containerBindingsSQL(scope, instanceID, refs)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read bindings per container on service instance %d: %w", instanceID, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]BoundLibrary{}
	for rows.Next() {
		var ref string
		var b BoundLibrary
		if err := rows.Scan(&ref, &b.ID, &b.Name, &b.Kind); err != nil {
			return nil, fmt.Errorf("read bindings per container on service instance %d: scan: %w",
				instanceID, err)
		}
		out[ref] = append(out[ref], b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read bindings per container on service instance %d: %w", instanceID, err)
	}
	return out, nil
}

// AcceptLibraries creates or joins one library per acceptance, binds its
// sources, and files the works those sources already replicated.
//
// It is the only writer of a `library` row that answers to a user, and ADR-0048
// clause 2 is what makes it worth having: with no row until Accept, *"declining
// a proposal writes nothing and accepting it writes once"*.
//
// # THE WHOLE BATCH IS ONE TRANSACTION AND A CALLER ERROR ABORTS ALL OF IT
//
// A name that collides at another kind, a source outside the scope, a container
// kind with no membership derivation: each returns its error and NOTHING is
// written, not even the acceptances that had already succeeded in the loop.
//
// ⚠️ THIS IS A DECISION AND THE ALTERNATIVE WAS REAL. Accepting the good rows and
// reporting the bad one keeps a user's work, and it is what BindContainers does
// with its per-container savepoint — but that path is an unattended bootstrap
// whose caller is a log line, and this one is a screen. ADR-0048 clause 1 is what
// makes all-or-nothing nearly free HERE and not there: a proposal is a value
// recomputed from the probe, so the rows that did not get created are still
// proposals, the screen recomputes the same set, and the cost of a rejected
// submission is the user fixing one name and clicking again. A partial accept
// costs the opposite — half the ticked rows are now libraries, the rest are
// still proposals, and §17.8's own worked failure is a screen whose count no
// longer describes its data.
//
// ⚠️ AND THERE IS NO PER-ACCEPTANCE SAVEPOINT, deliberately, because with the
// rule above there is nothing for one to save: every failure returns, the outer
// transaction rolls back, and a savepoint that can only ever be rolled back
// alongside its own transaction is ceremony. If partial acceptance is ever
// wanted, the savepoint arrives WITH it, in the same commit as the result type
// that reports which acceptance failed.
//
// THE USER'S LIBRARY SET IS RE-READ PER ACCEPTANCE, and that is not a missed
// optimisation. Two acceptances in one batch can name the same library — two
// proposals both renamed to `Comics` — and the second must JOIN what the first
// created rather than collide with it on ux_library_name. Re-reading inside the
// transaction is what makes that true by construction; a set read once and
// patched in Go is the same fact maintained twice.
func (s *Store) AcceptLibraries(
	ctx context.Context, scope Scope, userID int64, accepts []LibraryAcceptance,
) ([]AcceptedLibrary, error) {
	var out []AcceptedLibrary
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		out = make([]AcceptedLibrary, 0, len(accepts))
		now := FormatTime(time.Now())
		for _, a := range accepts {
			got, err := acceptOne(ctx, tx, scope, userID, a, now)
			if err != nil {
				return err
			}
			out = append(out, got)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func acceptOne(
	ctx context.Context, tx *sql.Tx, scope Scope, userID int64, a LibraryAcceptance, now string,
) (AcceptedLibrary, error) {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return AcceptedLibrary{}, errors.New("accept a library with no name: the name is the " +
			"user's own and this layer has nothing to derive one from")
	}
	if a.Kind == "" {
		return AcceptedLibrary{}, fmt.Errorf("accept library %q with no kind: library.kind is "+
			"exactly one value and is required (§6.5 rule 4)", name)
	}
	switch a.ManagedBy {
	case "auto", "user":
	default:
		return AcceptedLibrary{}, fmt.Errorf("accept library %q: managed_by %q is neither "+
			"'auto' nor 'user'; the screen decides which and the CHECK permits no third",
			name, a.ManagedBy)
	}
	for _, src := range a.Sources {
		if src.ContainerKind != "remote_library" {
			return AcceptedLibrary{}, fmt.Errorf("accept library %q: container_kind %q has no "+
				"membership derivation in the tree — §6.5 rule 3 gives each kind its own "+
				"predicate and only 'remote_library' is built, so binding this source would "+
				"create a library that files nothing and reads as empty",
				name, src.ContainerKind)
		}
		if !scope.admitsInstance(src.ServiceInstanceID) {
			return AcceptedLibrary{}, fmt.Errorf("accept library %q: source on service instance %d: %w",
				name, src.ServiceInstanceID, ErrSourceOutsideScope)
		}
	}

	lib, err := resolveAcceptedLibrary(ctx, tx, userID, name, a, now)
	if err != nil {
		return AcceptedLibrary{}, err
	}

	for _, src := range a.Sources {
		if _, err := upsertLibrarySource(ctx, tx, lib.ID, src.ServiceInstanceID,
			src.ContainerKind, src.ContainerRef, src.ContainerIdentity); err != nil {
			return AcceptedLibrary{}, err
		}
		filed, err := fileContainerMembers(ctx, tx, lib.ID, lib.Kind, src, now)
		if err != nil {
			return AcceptedLibrary{}, err
		}
		lib.MembersFiled += filed
	}
	if err := rescopeSearchDocs(ctx, tx, lib.ID); err != nil {
		return AcceptedLibrary{}, err
	}
	return lib, nil
}

// resolveAcceptedLibrary is §17.8's merge rule: join a library of the same name
// key AND the same kind, otherwise create one, otherwise refuse.
//
// The three exits are the whole of the rule and each one is a decision:
//
//   - SAME NAME KEY, SAME KIND → JOIN, and NOTHING about the existing row is
//     rewritten. Not its name, not its slug, not its formats, and not its
//     managed_by. §17.8's one-way door runs in this direction — *"a later
//     connect can only offer to add sources. It can never reshape the library."*
//     — and an acceptance carrying an edited name is exactly such a later
//     connect from the row's point of view. What the acceptance still does is
//     add its sources and file their members.
//   - SAME NAME KEY, DIFFERENT KIND → ErrLibraryNameTakenAtOtherKind. See that
//     error for why this is not disambiguated the way the import path
//     disambiguates.
//   - NAME FREE → create. The slug is allocated ONCE from the name (slugify),
//     and a slug that is taken while the name was not is the SAME refusal rather
//     than an ordinal: `Sci-Fi` and `Sci Fi` are two names that reduce to one
//     slug, and a user who typed the second and got a library at `sci-fi-2`
//     would have a permalink nobody chose. The import path mints an ordinal
//     there because it has nobody to ask; this one asks.
func resolveAcceptedLibrary(
	ctx context.Context, tx *sql.Tx, userID int64, name string, a LibraryAcceptance, now string,
) (AcceptedLibrary, error) {
	existing, err := userLibraries(ctx, tx, userID)
	if err != nil {
		return AcceptedLibrary{}, err
	}
	key := libraryNameKey(name)

	if held, ok := existing.joinable[key]; ok {
		if held.kind != a.Kind {
			return AcceptedLibrary{}, fmt.Errorf(
				"accept library %q at kind %q: library %d already holds that name at kind %q: %w",
				name, a.Kind, held.id, held.kind, ErrLibraryNameTakenAtOtherKind)
		}
		var joinedName, joinedSlug string
		if err := tx.QueryRowContext(ctx,
			`SELECT name, slug FROM library WHERE id = ?`, held.id).
			Scan(&joinedName, &joinedSlug); err != nil {
			return AcceptedLibrary{}, fmt.Errorf("read library %d to join it: %w", held.id, err)
		}
		return AcceptedLibrary{
			ID: held.id, Name: joinedName, Slug: joinedSlug, Kind: held.kind, Joined: true,
		}, nil
	}

	// The name key is held by a row that is NOT joinable, which is the reserved
	// "Unfiled" library and nothing else — userLibraries keeps it in `names` and
	// out of `joinable` precisely so a uniqueness check can see what a join must
	// not. It is the same refusal: the name is taken, and this one cannot even be
	// joined.
	if existing.names[key] {
		return AcceptedLibrary{}, fmt.Errorf(
			"accept library %q: that name is held by a reserved library that nothing may "+
				"join (library %d): %w", name, UnfiledLibraryID, ErrLibraryNameTakenAtOtherKind)
	}

	slug := slugify(name)
	if existing.slugs[slug] {
		return AcceptedLibrary{}, fmt.Errorf(
			"accept library %q: its slug %q is already taken by another library of this "+
				"user, so the permalink is not free even though the name is: %w",
			name, slug, ErrLibraryNameTakenAtOtherKind)
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO library
		  (user_id, name, slug, kind, formats, managed_by, enabled, include_in_search,
		   created_at, updated_at)
		VALUES (?,?,?,?,?,?,1,1,?,?)`,
		userID, name, slug, a.Kind, formatsJSON(a.Formats), a.ManagedBy, now, now)
	if err != nil {
		return AcceptedLibrary{}, fmt.Errorf("create library %q: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AcceptedLibrary{}, fmt.Errorf("create library %q: id: %w", name, err)
	}
	return AcceptedLibrary{ID: id, Name: name, Slug: slug, Kind: a.Kind, Created: true}, nil
}

// formatsJSON renders LibraryAcceptance.Formats for `library.formats`.
//
// nil is NULL, which the schema defines as "any format", and an EMPTY slice is
// NULL too rather than `[]`: a filter that admits nothing is a library that can
// never hold an item, and no caller means that by sending an empty list.
func formatsJSON(formats []string) any {
	if len(formats) == 0 {
		return nil
	}
	// A []string of format names cannot fail to marshal, so the error is
	// discarded at the one place it can be proven impossible rather than
	// propagated through three signatures.
	b, err := json.Marshal(formats)
	if err != nil {
		return nil
	}
	return string(b)
}

// fileContainerMembers files every top-level work this container replicated into
// the library, and reports how many rows it wrote.
//
// # THE `w.kind = ?` PREDICATE IS LOAD-BEARING, NOT BELT-AND-BRACES
//
// ADR-0066 decision 5 puts TWO libraries over ONE container whenever a BookOrbit
// library holds prose and comics: a `book` library and a `comic` library, both
// sourced on the same container_ref. Without the library's own kind on this
// statement, accepting either of them files EVERY work in the container into it,
// so the comic library holds the novels and the book library holds the series.
// `library.kind` is exactly one value (§6.5), and this is the statement that
// makes membership agree with it.
//
// # AND THE CHILD-KIND EXCLUSION STILL EARNS ITS PLACE
//
// It is redundant TODAY — `library.kind`'s CHECK lists top-level kinds only, so
// `w.kind = ?` cannot match a child kind. It is here because the redundancy is
// held by a CHECK in another file: this is the same membership question
// applyOneItem step 8 answers with the same map, and naming `childKinds` in both
// is what stops a kind added to the map (or admitted by a later migration's
// widened CHECK) from being filed by one derivation and not the other.
//
// # INSERT OR IGNORE, AND WHAT THE COUNT THEN MEANS
//
// ux_libmem_identity is UNIQUE(library_id, work_id, edition_id), so a work
// already filed here is ignored rather than duplicated — which is what makes
// accepting the same container twice idempotent. RowsAffected is therefore the
// number of works that MOVED into the library, not the library's size.
//
// edition_id is 0, the "whole work, format-independent" sentinel, exactly as
// applyOneItem step 8 writes it. Edition-grained membership is what
// `library.formats` will need when the Audiobookshelf split lands, and neither
// half of that is v0.1's: the column has no writer before this one and the
// derivation that would fill edition_id does not exist.
// It is rendered by a function rather than written inline so the plan guard can
// assert against the SHIPPED statement (DEVELOPMENT.md §11 rule 1): a guard that
// probes a hand-copied lookalike is measuring a proxy.
func fileContainerMembersSQL(
	libraryID int64, kind string, src AcceptedSource, now string,
) (string, []any) {
	kindPred, kindArgs := childKindsExcluded("w.kind")
	args := make([]any, 0, len(kindArgs)+5)
	args = append(args, libraryID, now, src.ServiceInstanceID, src.ContainerRef)
	args = append(args, kindArgs...)
	args = append(args, kind)

	return `
		INSERT OR IGNORE INTO library_member (library_id, sort_title, work_id, edition_id, added_at)
		SELECT DISTINCT ?, w.sort_title, w.id, 0, ?
		  FROM service_item_link sil
		  JOIN work w ON w.id = sil.work_id
		 WHERE sil.service_instance_id = ?
		   AND sil.remote_library_id = ?
		   AND sil.deleted_at IS NULL
		   AND w.deleted_at IS NULL
		   AND ` + kindPred + `
		   AND w.kind = ?`, args
}

func fileContainerMembers(
	ctx context.Context, tx *sql.Tx, libraryID int64, kind string, src AcceptedSource, now string,
) (int64, error) {
	query, args := fileContainerMembersSQL(libraryID, kind, src, now)
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("file container %q on service instance %d into library %d: %w",
			src.ContainerRef, src.ServiceInstanceID, libraryID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("file container %q on service instance %d into library %d: count: %w",
			src.ContainerRef, src.ServiceInstanceID, libraryID, err)
	}
	return n, nil
}

// rescopeSearchDocs re-derives `search_doc_library` for the works this library
// now holds.
//
// ⚠️ NOTHING ELSE RE-DERIVES IT, WHICH IS WHY THIS EXISTS. writeSearchDoc builds
// the junction FROM `library_member` AT WRITE TIME, and its invariant-5 fallback
// files a doc with no membership into library 0. Under ADR-0048's ordering the
// import runs BEFORE Accept, so every work a proposal is counting has a doc
// already scoped to library 0 — and filing it into a real library later moves
// nothing on its own. The work would be a member of the library the user
// accepted and searchable only under Unfiled, which is a scope no user's chip
// offers.
//
// TWO STATEMENTS, IN THIS ORDER, AND THE ORDER IS THE INVARIANT. The real scope
// rows go in first; only then are the library-0 rows removed, and only for docs
// that the first statement has just given a real scope. Reversing them opens a
// window inside the transaction where a doc has no scope row at all, and — more
// to the point — writing the DELETE without the subquery would strand every
// still-unfiled doc, which is the exact failure §7 invariant 5 exists to
// prevent. A work that stays unfiled keeps its library-0 row.
//
// SET-BASED, over the library rather than over a list of work ids the caller
// collected: `library_member` is where the answer already is, ix_libmem_work is
// the reverse index migration 0005 declares *"for the item detail page and for
// the search_doc_library rebuild"*, and a rebuild driven by a Go-side id list
// would be the same set assembled twice.
func rescopeSearchDocs(ctx context.Context, tx *sql.Tx, libraryID int64) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO search_doc_library (library_id, doc_rowid)
		SELECT DISTINCT m.library_id, d.rowid
		  FROM library_member m
		  JOIN search_doc d ON d.work_id = m.work_id
		 WHERE m.library_id = ?`, libraryID); err != nil {
		return fmt.Errorf("scope search documents to library %d: %w", libraryID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM search_doc_library
		 WHERE library_id = ?
		   AND doc_rowid IN (
		       SELECT d.rowid
		         FROM library_member m
		         JOIN search_doc d ON d.work_id = m.work_id
		        WHERE m.library_id = ?)`, UnfiledLibraryID, libraryID); err != nil {
		return fmt.Errorf("drop the unfiled fallback scope for library %d: %w", libraryID, err)
	}
	return nil
}

// admitsInstance is Scope.instancePredicate's answer for one id, in Go.
//
// It exists because AcceptLibraries is a WRITE and there is no statement to hang
// a predicate on: the caller hands over a service_instance_id and the question
// is whether it may. The three branches are instancePredicate's own, including
// the one that matters — an empty visible set with AllInstances false admits
// NOTHING, because "no visible instances" must never mean "all of them".
func (s Scope) admitsInstance(id int64) bool {
	if s.AllInstances {
		return true
	}
	for _, v := range s.InstanceIDs {
		if v == id {
			return true
		}
	}
	return false
}
