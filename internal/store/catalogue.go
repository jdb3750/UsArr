package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3"
)

// The catalogue write path: what a full import (channel 1) puts into the
// replica.
//
// WHICH SIDE OF THE ACCESS-SCOPE RULE EVERY FUNCTION HERE IS ON, stated once
// because store.go's rule 2 makes it a design question rather than a style one:
// EVERY function in this file is a REPLICATION WRITE driven by a background
// worker with no acting user, so none of them takes a Scope — the shipped
// precedent is ReplaceIndexers, whose doc comment says the same thing in the
// same words ("it is a replication write driven by the background prober, which
// has no acting user … The scope belongs on the READ"). The reads that render
// any of this — the library grid, the item page, search — are the NEXT commit
// and every one of them takes a Scope in its signature.
//
// The user_id these rows are owned by is passed in explicitly rather than
// assumed, because `library` is a user-scoped table (§1.3 rule 1) and v0.1's
// single owner is a caller's fact, not this layer's.

// UnfiledLibraryID is migration 0005's reserved library 0, "Unfiled".
//
// It is where a work that belongs to no other library is filed, so that §7
// invariant 5 — every search_doc has at least one search_doc_library row — can
// be kept. A work visible through no library matches no scope and vanishes from
// search for every user including its owner.
const UnfiledLibraryID int64 = 0

// CatalogueContainer is one container an upstream service has already named —
// for Kavita, one entry of GET /api/Library/libraries.
//
// Kind is UsArr's decision, not the upstream's (ARCHITECTURE.md §6.4: "the kind
// decision is UsArr's, made once at ingest … derived from … the library's
// declared kind, and never inherited from whichever backend answered first").
// An empty Kind means the adapter could not map this container onto a
// work.kind: it is DECLINED WITH A REASON (§17.8) rather than silently dropped,
// and DeclineReason carries the reason.
type CatalogueContainer struct {
	// RemoteID is the upstream's own id for the container, verbatim. It lands
	// in library_source.container_ref and in service_item_link.remote_library_id,
	// which is the pair the membership predicate joins on.
	RemoteID string

	// Name is the upstream's own name. It becomes the UsArr library's name on
	// first bind, and it is also the container_identity recorded so an upstream
	// that reuses ids cannot silently rebind the library to a different container.
	Name string

	// Kind is a work.kind, or "" for a declined container.
	Kind string

	// DeclineReason is set exactly when Kind is "".
	DeclineReason string

	// KindProvisional says Kind is the adapter's FALLBACK rather than the
	// upstream's answer, so the WALK is the authority on what this container
	// holds and the bind pass is only guaranteeing a row.
	//
	// ⚠️ IT EXISTS BECAUSE ONE ADAPTER CANNOT ANSWER THE QUESTION AT ALL.
	// Kavita SAYS what a library holds, so its Kind is evidence and this stays
	// false. BookOrbit's `libraries` table has no type, kind or mediaType column
	// (bookorbit.Library), so `bookKind` is a constant guess made before a single
	// book has been read — and a comics-only BookOrbit container bound eagerly at
	// 'book' is a library that should not exist, with the comic sibling minted
	// beside it carrying a qualifier it never needed.
	//
	// What the flag buys is exactly two behaviours in bindOneContainer, and
	// nothing else changes for an adapter that leaves it false:
	//
	//   - the EAGER bind (bindFromContainerList) ADOPTS whatever library already
	//     stands over this container ref, at whatever kind, rather than minting a
	//     second one at the fallback kind. A guess must never contradict evidence
	//     a previous import already recorded.
	//   - the ITEM path (bindSiblingKind) may RETYPE that library in place when
	//     it is still empty, instead of minting a sibling beside it.
	//
	// ⚠️ IT NEVER DEFERS OR DELETES A LIBRARY, and that is deliberate rather than
	// incidental: ADR-0066 decision 1 requires that a container whose every item
	// is skipped is STILL BOUND — "Containers() keeps reporting it,
	// bindOneContainer keeps binding it, and the library renders on §17.8 with an
	// item count of zero and a sentence". A content-derived scheme that waited for
	// the walk to mint anything would silently delete exactly that row. The row is
	// created eagerly here as it always was; only its `kind` can still move, and
	// only while nothing has been filed into it.
	KindProvisional bool
}

// CatalogueBinding is where one container's items are filed.
type CatalogueBinding struct {
	LibraryID int64
	Kind      string

	// Created reports whether this call created the library rather than joining
	// an existing one. §17.8's second-instance rule makes joining the default,
	// so a caller that wants to report "joined X as a second source" needs to be
	// able to tell them apart.
	Created bool

	// UserID and ContainerName are what a SIBLING library over the same
	// container ref needs, and they are carried on the binding rather than added
	// to ApplyCatalogueBatch's parameter list for one reason: `library` is
	// user-scoped and every library minted from this container must be owned by
	// the same user and named after the same upstream container, so the two
	// facts belong to the binding rather than to a batch.
	//
	// ⚠️ ONLY THE PARENT PATH READS THEM (ADR-0066 decision 5, activated by
	// ADR-0068). A binding built by hand with both zero still files items
	// exactly as it always did; what it cannot do is mint a comic library.
	UserID        int64
	ContainerName string

	// Provisional carries CatalogueContainer.KindProvisional onto the binding,
	// because the item path needs it after BindContainers has returned.
	//
	// It is what makes applyOneItem re-ask which library a TOP-LEVEL item belongs
	// in. For every other adapter the container's kind and the item's kind agree
	// by construction and the binding is read straight through; for a provisional
	// container they can disagree at runtime — a comic reached first retypes the
	// container's library to 'comic', and a prose book arriving after it must get
	// a 'book' library rather than be filed into that one.
	Provisional bool
}

// SkippedContainer is one upstream container that could NOT be bound to a
// library, and the reason.
//
// It is not a DeclinedContainer. A decline is the adapter's decision — UsArr
// has no work.kind for this container — and it is known before any write. A
// skip is a write that failed a uniqueness constraint the replica cannot
// satisfy for this container, discovered inside the bind transaction. Both are
// reported; they are separate fields because "we do not handle Kavita's Image
// libraries" and "two of your libraries want the same slug" need different
// answers from the operator.
type SkippedContainer struct {
	RemoteID string
	Name     string

	// Reason is the underlying error's text. It names a constraint, so it is
	// database detail rather than upstream text — the upstream's own NAME is
	// carried in Name, on the same terms as DeclinedContainer.
	Reason string
}

// ExternalIdentifier is one identity claim about a work.
//
// Confidence >= 1.0 is a STRONG claim and is what ux_extid_work_strong
// (UNIQUE(source, value) WHERE work_id IS NOT NULL AND confidence >= 1.0)
// polices. A violation of that index is the merge SIGNAL, not an error
// (migration 0005). v0.1 has no work_merge table, so ApplyCatalogueBatch
// resolves it by reusing the existing work rather than by merging — see its doc
// comment.
type ExternalIdentifier struct {
	Source     string
	Value      string
	Confidence float64
}

// AltTitle is one row of work_alt_title.
type AltTitle struct {
	Title      string
	Normalized string
	Kind       string // original|translated|alias|acronym|sort
	Language   string
}

// CatalogueParent is the TOP-LEVEL work a child item is minted under.
//
// It is the two-level seam ADR-0068 needed and this type is the whole of it: a
// child kind — `comic_issue` today, `episode` and `track` the day an adapter
// reports them — is never a top-level item, so it cannot be filed, indexed or
// browsed on its own terms and must arrive with the work it hangs under.
//
// ⚠️ THE PARENT IS RESOLVED, WRITTEN AND FILED BEFORE THE CHILD, in the same
// transaction. `parent_work_id` is never left null on a child this writer
// creates, which is ADR-0068 decision 1 and the one part of the shape §6.4's
// cascade makes unfixable later.
type CatalogueParent struct {
	// RemoteKind and RemoteID identify the parent in service_item_link, and they
	// are what makes the parent RESOLVE rather than be re-minted per child.
	// Every child of one series carries the same pair, so the second child of a
	// series finds the work the first one created.
	//
	// ⚠️ RemoteID MAY BE SYNTHESIZED. A one-shot has no upstream series, so the
	// adapter mints a deterministic id for it (ADR-0068 decision 2). Determinism
	// is the requirement: a re-import must resolve the same series work rather
	// than mint a second one.
	RemoteKind string
	RemoteID   string

	// Kind is the parent's work.kind — 'comic' for a comic series. It is
	// DIFFERENT from the binding's kind whenever a mixed upstream container is
	// involved, and that difference is what mints the sibling library (ADR-0066
	// decision 5).
	Kind string

	Title           string
	SortTitle       string
	NormalizedTitle string
	NormVersion     int64

	// Synthesized reports that no upstream parent exists and this one was minted
	// for a single child. It changes NO write here — the row is an ordinary
	// series work, deliberately, so that /library/comics counts one thing — and
	// exists so the adapter can count the residue for ADR-0068 decision 4.
	Synthesized bool
}

// CatalogueItem is one replicated top-level work, as an adapter projected it.
//
// It is DELIBERATELY NOT an upstream DTO. internal/store never sees a
// kavita.SeriesDto: the projection happens in the adapter, which is what keeps
// this table's write path source-agnostic and keeps host filesystem paths and
// upstream blobs out of it except where a column exists to hold them.
type CatalogueItem struct {
	RemoteID   string
	RemoteKind string // the upstream's own noun: 'series' for Kavita

	// ContainerID is the container this item was reported in. It is matched
	// against CatalogueContainer.RemoteID to pick the library, and stored on the
	// link as remote_library_id.
	ContainerID string

	// Kind is the work.kind. It comes from the CONTAINER, never from the item.
	Kind string

	Title           string
	SortTitle       string
	NormalizedTitle string
	NormVersion     int64
	OriginalTitle   string

	AltTitles []AltTitle

	// Overview lands on work.overview AND in search_fts. Kavita's series list
	// carries none, so it is empty today and a phase-B backfill is what fills it.
	//
	// IT IS PERSISTED ON `work` RATHER THAN ONLY INDEXED, and that is load-bearing
	// rather than tidy: search_fts is contentless, so its stored text CANNOT be
	// read back (measured — `SELECT people FROM search_fts` returns NULL), and
	// fts5 refuses to update a subset of a contentless-delete table's columns
	// ("cannot UPDATE a subset of columns on fts5 contentless-delete table",
	// executed against this schema). Every rewrite of a document is therefore a
	// full DELETE + INSERT of all five columns, and the credit pass now performs
	// one — so any column that lives ONLY in the FTS row is a column the credit
	// pass would silently blank. `work.overview` is the column that already
	// exists for this; whoever writes the phase-B backfill writes it there and
	// rebuilds the doc.
	Overview string

	// RemotePath is the path the UPSTREAM reported, verbatim
	// (service_item_link.remote_path). It is a host filesystem path on the
	// upstream's box and must never reach a browser.
	RemotePath string

	// RemoteSubtype is the upstream's own sub-classification, verbatim and
	// unparsed (§6.5 rule 3).
	RemoteSubtype string

	AddedAt         time.Time
	RemoteUpdatedAt time.Time
	HasFile         bool

	// PageCount lands on work_book.page_count or work_comic_issue.page_count.
	// work_comic has no page count — it is the SERIES level.
	PageCount sql.NullInt64

	// ReadingDirection lands on work_comic.reading_direction. NULL is a real
	// answer and the common one.
	ReadingDirection sql.NullString

	// Parent, when non-nil, makes this a CHILD work rather than a top-level one.
	// See CatalogueParent, and applyOneItem step 0 for what changes.
	Parent *CatalogueParent

	// NumberText and NumberSort land on work_comic_issue. ANY INTEGER COLUMN
	// HERE WOULD BE WRONG (ADR-0030, and migration 00006's own header): issue
	// numbers are '1.MU', '-1', '0', 'Annual 1', '1A', so the text is the
	// upstream's own token and the sort key is a REAL.
	NumberText sql.NullString
	NumberSort sql.NullFloat64

	// IsOneshot lands on work_comic_issue.is_oneshot, and it is WRITTEN rather
	// than left to the column's DEFAULT 0 — ADR-0068 decision 2: "a column with
	// a DEFAULT 0 and no writer is a deaf column".
	IsOneshot bool

	ExternalIDs []ExternalIdentifier
}

// IdentityConflict is one ux_extid_work_strong violation, recorded rather than
// raised.
type IdentityConflict struct {
	Source string
	Value  string

	// ExistingWorkID already claims (Source, Value) at confidence >= 1.0.
	ExistingWorkID int64

	// AttemptedWorkID is the work this batch was writing when it collided.
	AttemptedWorkID int64

	RemoteID string
}

func (c IdentityConflict) String() string {
	return fmt.Sprintf("%s=%s is already claimed by work %d; remote item %s resolved to work %d",
		c.Source, c.Value, c.ExistingWorkID, c.RemoteID, c.AttemptedWorkID)
}

// BatchResult is what one ApplyCatalogueBatch call did.
type BatchResult struct {
	WorksCreated int
	WorksReused  int
	WorksUpdated int

	ExternalIDsWritten int
	IdentityConflicts  []IdentityConflict

	// Unidentified counts items that ended the batch with no external_id row at
	// all. On a free Kavita this is EVERY item, which is the ordinary case and
	// not an error (ARCHITECTURE.md §6.4, ADR-0035 §1).
	Unidentified int

	// SearchDocs counts search-document REBUILDS, not distinct documents. Two
	// remote items that tier 1 resolves onto one work rebuild that work's single
	// document twice, so this can exceed the number of rows in search_doc — which
	// is the honest reading of "work done", and the reason the invariant
	// assertions count the TABLE rather than this field.
	SearchDocs int

	// Members counts library_member writes, one per applied item, on the same
	// terms as SearchDocs.
	Members int
}

func (r *BatchResult) add(o BatchResult) {
	r.WorksCreated += o.WorksCreated
	r.WorksReused += o.WorksReused
	r.WorksUpdated += o.WorksUpdated
	r.ExternalIDsWritten += o.ExternalIDsWritten
	r.IdentityConflicts = append(r.IdentityConflicts, o.IdentityConflicts...)
	r.Unidentified += o.Unidentified
	r.SearchDocs += o.SearchDocs
	r.Members += o.Members
}

// Add merges another result into r. Exported so an importer can accumulate
// across batches without reimplementing the sum.
func (r *BatchResult) Add(o BatchResult) { r.add(o) }

// libraryNameKey is §17.8's stated merge key for library names:
// case-insensitive, whitespace-trimmed, per user. It is written down as one
// function so the bind path and any future proposal UI cannot disagree about it.
func libraryNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// slugify allocates a library slug from its name, ONCE. library.slug is durable
// by design — a rename must not change the permalink — so this runs on create
// and never again.
func slugify(name string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "library"
	}
	return s
}

// BindContainers derives one UsArr library per upstream container and returns
// where each container's items are filed.
//
// §17.8's shape, and the two rules it makes decisions rather than defaults:
//
//   - ONE LIBRARY PER UPSTREAM CONTAINER. §17.8: "one per upstream library for
//     Audiobookshelf / Kavita / Navidrome".
//   - A SECOND SOURCE JOINS AN EXISTING LIBRARY RATHER THAN CREATING A NEW ONE,
//     on the stated merge key — case-insensitive, whitespace-trimmed, per user —
//     PROVIDED THE KIND AGREES. library.kind is exactly one value, so a name
//     collision across kinds cannot join; that container gets a disambiguated
//     name instead of silently landing in a library of the wrong kind.
//
// container_kind IS 'remote_library', NOT 'instance', AND THAT IS A DECISION.
// 'instance' means "everything this service reports", and it is expressible only
// where the instance yields exactly one kind. A Kavita with a Manga library and
// an Ebooks library — the shape testdata/cassettes/kavita_libraries.yaml
// actually carries — yields two kinds, and library.kind is exactly one, so an
// 'instance' binding would file books into a comic library. 'remote_library' is
// equality on an id the upstream itself reported (§6.5 rule 3), it is held in
// service_item_link.remote_library_id, and ix_sil_container exists for precisely
// this join. ARCHITECTURE.md §17.8 is the source; the reasoning above is the
// whole of it.
//
// ⚠️ THIS DECISION IS OWED AN ADR AND DOES NOT HAVE ONE. An earlier draft of
// this file cited "ADR-0042" for it; ADR-0042 has since landed as *v0.1's
// minimal write path re-sequences with the \*Arr adapters*, which is a different
// decision entirely, so the citation was removed rather than repointed. ADR
// numbers are a shared counter that cannot be allocated safely by reading
// (DEVELOPMENT.md §11), and writing one here would race the next thread; it is
// recorded as LS-06 instead.
//
// Declined containers (Kind == "") get no library and no source row. They are
// returned to the caller in neither the map nor an error: the caller already
// holds the reason and reports it.
//
// # ONE UNBINDABLE CONTAINER DOES NOT TAKE DOWN THE IMPORT
//
// A container whose create violates ux_library_name or ux_library_slug is
// SKIPPED — rolled back to its own savepoint, recorded in sync_report as
// `container_bind_failed`, and returned in the []SkippedContainer — and every
// other container in the list still binds. Before this it returned an error,
// which aborted BindContainers, which aborted FullImport before a single item
// was read: two Kavita libraries whose names reduce to the same slug meant NO
// library synced at all. CLAUDE.md principle 3 is that a feature degrades
// honestly rather than failing wholesale, and "honestly" is the reason the skip
// is recorded rather than logged: a background import has no caller left to
// read a return value by the time anyone asks why a library is missing.
//
// The disambiguation in step 3 should make a skip unreachable in practice. It
// is kept anyway, because a uniqueness rule the schema owns and this file
// re-derives is exactly the pair that drifts.
//
// It is a replication write and takes no Scope. See the file header.
func (s *Store) BindContainers(
	ctx context.Context, instanceID, userID int64, cs []CatalogueContainer,
) (map[string]CatalogueBinding, []SkippedContainer, error) {
	out := make(map[string]CatalogueBinding, len(cs))
	var skipped []SkippedContainer
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for i, c := range cs {
			if c.Kind == "" {
				continue
			}
			sp := fmt.Sprintf("bind_%d", i)
			if _, err := tx.ExecContext(ctx, "SAVEPOINT "+sp); err != nil {
				return fmt.Errorf("savepoint %s: %w", sp, err)
			}
			b, bindErr := bindOneContainer(ctx, tx, instanceID, userID, c, bindFromContainerList)
			if bindErr == nil {
				if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
					return fmt.Errorf("release savepoint %s: %w", sp, err)
				}
				out[c.RemoteID] = b
				continue
			}
			wrapped := fmt.Errorf("bind container %q (%s) on service_instance %d: %w",
				c.Name, c.RemoteID, instanceID, bindErr)
			// Only a CONSTRAINT violation is "this container cannot be bound".
			// An I/O error, a corrupt page or a locked database is a fact about
			// the DATABASE, not about this container, and skipping every
			// container in turn would report a broken disk as a tidy list of
			// unbindable libraries.
			if !isSkippableBindError(bindErr) {
				return wrapped
			}
			// Undo just this container. Everything already bound in this
			// transaction stands — the savepoint is per container for the same
			// reason ApplyCatalogueBatch's is per external id.
			if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sp); err != nil {
				return fmt.Errorf("rollback to savepoint %s after %w: %w", sp, bindErr, err)
			}
			if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
				return fmt.Errorf("release savepoint %s: %w", sp, err)
			}
			sk := SkippedContainer{RemoteID: c.RemoteID, Name: c.Name, Reason: bindErr.Error()}
			detail, err := json.Marshal(map[string]string{"name": sk.Name, "reason": sk.Reason})
			if err != nil {
				return fmt.Errorf("encode skipped container %q: %w", c.RemoteID, err)
			}
			// Recorded INSIDE the same transaction as the rollback that made it
			// true, and by the store rather than the caller: the caller can
			// ignore a returned slice, and a skip nobody can see is the failure
			// mode this whole change exists to remove.
			if err := recordSyncReport(ctx, tx, instanceID,
				"container_bind_failed", "library", c.RemoteID, string(detail)); err != nil {
				return err
			}
			skipped = append(skipped, sk)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return out, skipped, nil
}

// isSkippableBindError decides whether a failed bind is a fact about THIS
// CONTAINER or a fact about the database.
//
// Only a CONSTRAINT violation is the former: a name or slug that cannot be made
// unique, a kind the schema does not know, a foreign key that no longer
// resolves. An I/O error, a corrupt page, a full disk or a locked database is
// the latter, and skipping container after container over one would report a
// broken database as a tidy list of unbindable libraries — the opposite of
// degrading honestly.
func isSkippableBindError(err error) bool {
	return errors.Is(err, sqlite3.CONSTRAINT)
}

// SyncReportContainerKindChanged is sync_report.kind for a container whose
// ADAPTER-DECIDED kind is not the kind it was bound at last time, so this import
// binds it to a DIFFERENT library than the last one did.
//
// ⚠️ IT EXISTS BECAUSE THE ALTERNATIVE IS ITEMS MOVING BETWEEN LIBRARIES WITH NO
// RECORD. bindOneContainer's step 1 matches on (container, kind); a container
// retyped upstream — a Kavita library switched from Manga to Book — therefore
// misses its old library and gets one of the right kind instead. That is the
// correct outcome (§6.4: the kind decision is UsArr's, made once at ingest, and
// a library holding items of a kind it does not declare is the worse state), but
// from the Libraries screen it looks like a library that emptied itself and a
// second one that appeared. One row per occurrence is what makes it readable
// afterwards.
//
// ⚠️ IT IS NOT WRITTEN FOR THE ADR-0066 DECISION 5 SIBLING MINT, and that
// distinction is the whole reason bindReason exists. A `comic` library minted
// beside a `book` one over the same container is a container bound at a second
// kind, which is indistinguishable in the schema from a container whose kind
// changed — but it is not a change, nothing moved, and a row for it would fire
// on every mixed BookOrbit library on the first comic it reaches.
//
// It needs no migration: sync_report.kind carries no CHECK (migration 00005) and
// `detail` is untyped JSON. Same seam container_declined and
// container_bind_failed use.
const SyncReportContainerKindChanged = "container_kind_changed"

// bindReason is WHO is asking for a binding, and it exists for exactly one
// decision: whether a container already bound at a different kind is a CHANGE
// (worth a sync_report row) or an EXPECTED SECOND KIND inside one container
// (ADR-0066 decision 5, worth nothing). The schema cannot tell those apart —
// both are two library_source rows on one container ref — so the caller says.
type bindReason int

const (
	// bindFromContainerList is BindContainers walking the adapter's container
	// list. Exactly one kind per container per import, so a different existing
	// kind means the adapter's DECISION moved.
	bindFromContainerList bindReason = iota

	// bindSiblingKind is parentBinding minting the second library a mixed
	// container needs. A different existing kind is the precondition, not news.
	bindSiblingKind
)

// kindQualifier is the parenthetical a SIBLING library carries so its name says
// what it holds rather than when it was made.
//
// ⚠️ NEVER AN ORDINAL. `Fiction (2)` encodes creation order, which is an
// implementation fact no reader can interpret: it says a second library exists
// and not one word about why. `Fiction (Comics)` says which of the two this is,
// and the qualifier's PRESENCE is itself the signal that a container was split.
//
// ⚠️ PARENTHESES RATHER THAN A SEPARATOR GLYPH. The name travels through a
// terminal, a CSV and a URL, and `(` `)` survive all three unescaped; a middot
// does not survive the first.
//
// ⚠️ ONLY THE NEW SIBLING IS QUALIFIED. The library that was already there keeps
// its name — renaming it to `Fiction (Books)` for symmetry would rewrite a name
// the user has already seen, and library.slug is durable by design, so the pair
// would agree in the list and disagree in every permalink.
//
// An unknown kind returns "", and the caller then falls through to the ordinal
// loop rather than inventing a word for it. library.kind is CHECK-constrained to
// exactly these seven (migration 00005), so "" is unreachable today and is
// handled anyway: the CHECK is the schema's list and this is a second copy of
// it, which is the pair that drifts.
func kindQualifier(kind string) string {
	switch kind {
	case "movie":
		return "Movies"
	case "series":
		return "Series"
	case "artist":
		return "Artists"
	case "album":
		return "Albums"
	case "book":
		return "Books"
	case "comic":
		return "Comics"
	case "game":
		return "Games"
	default:
		return ""
	}
}

func bindOneContainer(
	ctx context.Context, tx *sql.Tx, instanceID, userID int64, c CatalogueContainer,
	why bindReason,
) (CatalogueBinding, error) {
	// Step 1: is this exact container already bound AT THIS KIND? If so nothing
	// about the library changes — a rename upstream must not re-propose or
	// re-slug it.
	//
	// ⚠️ `AND l.kind = ?` IS LOAD-BEARING AND WAS ADDED FOR ADR-0066 DECISION 5.
	// `library_source`'s uniqueness is (library_id, service_instance_id,
	// container_kind, container_ref), so ONE container ref may legitimately name
	// TWO libraries — a `book` library and a `comic` library over the same
	// BookOrbit library, which is exactly what a mixed container becomes once
	// comics have a unit of work (ADR-0068). Without the kind predicate this
	// QueryRow matches BOTH and takes whichever row SQLite happens to return
	// first, so on the second import the prose container could bind to the comic
	// library and every book in it would be written into a library of the wrong
	// kind. That is not a hypothetical the sibling mint below creates: it is the
	// state the FIRST import leaves behind.
	//
	// ⚠️ AND IT CHANGES ONE PRE-EXISTING BEHAVIOUR, stated rather than hidden. A
	// container whose ADAPTER-DECIDED kind changes between imports — a Kavita
	// library retyped from Manga to Book — used to keep its old library and have
	// its items written into it under the new kind. It now falls through to
	// steps 2 and 3 and gets a library of the right kind instead. §6.4 is that
	// "the kind decision is UsArr's, made once at ingest", and a library holding
	// items of a kind it does not declare was already the worse of the two.
	var libID int64
	var kind string
	err := tx.QueryRowContext(ctx, `
		SELECT l.id, l.kind
		  FROM library_source ls
		  JOIN library l ON l.id = ls.library_id
		 WHERE ls.service_instance_id = ?
		   AND ls.container_kind = 'remote_library'
		   AND ls.container_ref = ?
		   AND l.kind = ?`, instanceID, c.RemoteID, c.Kind).Scan(&libID, &kind)
	switch {
	case err == nil:
		// The container is back: clear any missing_since the sweep set.
		// DELIBERATELY NOT LIBRARY-SCOPED. Where one container ref names two
		// libraries, the container coming back un-misses BOTH of them: it is one
		// upstream container and one fact about it, and `container_identity` is
		// the same upstream name on both rows by construction.
		if _, err := tx.ExecContext(ctx, `
			UPDATE library_source SET missing_since = NULL, container_identity = ?
			 WHERE service_instance_id = ? AND container_kind = 'remote_library' AND container_ref = ?`,
			c.Name, instanceID, c.RemoteID); err != nil {
			return CatalogueBinding{}, fmt.Errorf("refresh source: %w", err)
		}
		return CatalogueBinding{
			LibraryID: libID, Kind: kind, UserID: userID, ContainerName: c.Name,
			Provisional: c.KindProvisional,
		}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return CatalogueBinding{}, fmt.Errorf("look up existing source: %w", err)
	}

	// Step 1b: THIS CONTAINER IS NOT BOUND AT THIS KIND. Is it bound at another
	// one? The answer is needed twice below and read once here, before anything
	// is written, so both readings are of the state the LAST import left.
	prior, err := containerBoundKinds(ctx, tx, instanceID, c.RemoteID)
	if err != nil {
		return CatalogueBinding{}, err
	}

	// Step 1c: A PROVISIONAL KIND IS A GUESS, AND A GUESS LOSES TO EVIDENCE.
	// See CatalogueContainer.KindProvisional. This runs only for an adapter that
	// cannot read a container's kind upstream, and it splits on WHO is asking,
	// which is what bindReason is already for.
	if c.KindProvisional && len(prior) > 0 {
		b, done, err := bindProvisional(ctx, tx, instanceID, userID, c, why, prior)
		if err != nil {
			return CatalogueBinding{}, err
		}
		if done {
			return b, nil
		}
	}

	// Step 2: join a library of the same name AND the same kind, if one exists.
	// The join key is §17.8's: case-insensitive, whitespace-trimmed, per user.
	// It is evaluated in Go over this user's libraries rather than in SQL,
	// because ux_library_name is a plain UNIQUE on the raw name and there is no
	// index over a lowered, trimmed form to seek — a homelab has single-digit
	// libraries, so the scan is cheaper than the index would be.
	existing, err := userLibraries(ctx, tx, userID)
	if err != nil {
		return CatalogueBinding{}, err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = "Library " + c.RemoteID
	}
	if got, ok := existing.joinable[libraryNameKey(name)]; ok && got.kind == c.Kind {
		if err := insertLibrarySource(ctx, tx, got.id, instanceID, c); err != nil {
			return CatalogueBinding{}, err
		}
		if err := noteKindChange(ctx, tx, instanceID, c, why, prior, got.id); err != nil {
			return CatalogueBinding{}, err
		}
		return CatalogueBinding{
			LibraryID: got.id, Kind: got.kind, UserID: userID, ContainerName: c.Name,
			Provisional: c.KindProvisional,
		}, nil
	}

	// Step 3: create, under a name and slug that are BOTH free.
	//
	// TWO UNIQUE INDEXES, NOT ONE, and that is the whole of the bug this loop
	// used to have. Migration 0005 declares ux_library_name(user_id, name) AND
	// ux_library_slug(user_id, slug); the loop tested only the first. slugify
	// is lossy — every run of non-alphanumerics collapses to one dash — so
	// "Sci-Fi" and "Sci Fi" are two DIFFERENT names (the join key never
	// matches, the name index never fires) that reduce to the one slug
	// "sci-fi", and the create aborted on ux_library_slug. Measured, not
	// reasoned: TestBindContainersDisambiguatesASlugCollision.
	//
	// AND THE TAKEN SET INCLUDES THE RESERVED "Unfiled" ROW. It is excluded
	// from `joinable` on purpose — nothing may ever join library 0 — but it is
	// still a row in ux_library_name and ux_library_slug, so an upstream
	// library actually named "Unfiled" collided with it. Reserving a name
	// against joining is not the same as pretending it is free.
	// AND THE FIRST CANDIDATE IS A KIND QUALIFIER, NOT AN ORDINAL, WHENEVER THE
	// NAME IS HELD BY A LIBRARY OF A DIFFERENT KIND. That is precisely the
	// collision step 2 just refused to join across, and it is the shape ADR-0066
	// decision 5 makes ordinary: one BookOrbit container becomes a `book` library
	// and a `comic` library, and the second of them used to be called
	// `Fiction (2)`. `Fiction (Comics)` says which one it is; `Fiction (2)` says
	// only that it was made second. See kindQualifier for the rest of the reasons.
	//
	// ⚠️ THE ORDINAL LOOP BELOW IS NOT REPLACED, AND ITS SURVIVING JOB IS A
	// DIFFERENT COLLISION. `Sci-Fi` and `Sci Fi` are two book libraries that
	// reduce to one slug; a kind qualifier there would read `Sci Fi (Books)`
	// beside a `Sci-Fi` that is also books, which states a difference that does
	// not exist. The qualifier answers a KIND collision; the ordinal answers
	// everything else.
	//
	// ⚠️ AND THE QUALIFIED NAME CAN ITSELF BE TAKEN — library names are UNIQUE
	// per user (ux_library_name, migration 00005), so an upstream container
	// genuinely named `Fiction (Comics)` collides with the derived one. Nothing
	// here invents a rule for that: the candidate is simply not free, and the
	// pre-existing ordinal loop runs from `base` exactly as it did before this
	// change. What that case SHOULD do is an open design question and is recorded
	// as one rather than settled by whichever branch happened to reach it.
	base := name
	slug := slugify(name)
	if held, ok := existing.joinable[libraryNameKey(name)]; ok && held.kind != c.Kind {
		if q := kindQualifier(c.Kind); q != "" {
			cand := fmt.Sprintf("%s (%s)", base, q)
			if candSlug := slugify(cand); !existing.names[libraryNameKey(cand)] && !existing.slugs[candSlug] {
				name, slug = cand, candSlug
			}
		}
	}
	for n := 2; existing.names[libraryNameKey(name)] || existing.slugs[slug]; n++ {
		name = fmt.Sprintf("%s (%d)", base, n)
		slug = slugify(name)
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO library (user_id, name, slug, kind, managed_by, enabled, include_in_search)
		VALUES (?,?,?,?, 'auto', 1, 1)`, userID, name, slug, c.Kind)
	if err != nil {
		return CatalogueBinding{}, fmt.Errorf("create library %q: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CatalogueBinding{}, fmt.Errorf("create library %q: id: %w", name, err)
	}
	if err := insertLibrarySource(ctx, tx, id, instanceID, c); err != nil {
		return CatalogueBinding{}, err
	}
	if err := noteKindChange(ctx, tx, instanceID, c, why, prior, id); err != nil {
		return CatalogueBinding{}, err
	}
	return CatalogueBinding{
		LibraryID: id, Kind: c.Kind, Created: true, UserID: userID, ContainerName: c.Name,
		Provisional: c.KindProvisional,
	}, nil
}

// bindProvisional is the whole of what CatalogueContainer.KindProvisional buys,
// and it runs only when the container is ALREADY bound at some other kind.
//
// `done` false means "this is an ordinary bind after all" and bindOneContainer
// carries on through steps 2 and 3 exactly as it does for every other adapter.
//
// # The eager pass ADOPTS, because its kind is a guess
//
// bindFromContainerList is the adapter's container list, whose kind for a
// provisional container is a constant chosen before any book was read. If a
// previous import already recorded what this container actually holds, minting a
// second library at the fallback kind would contradict the evidence with the
// guess — and would do it on EVERY import, producing a permanently empty
// `Comics (Books)` beside the real `Comics`. Adopting returns before
// noteKindChange on purpose: adopting is not a kind CHANGE, nothing moved, and a
// sync_report row here would fire on every re-import of every comics library.
//
// # The item pass may RETYPE, because its kind is evidence
//
// bindSiblingKind is a walk that has actually seen a comic. If the only library
// over this container is still EMPTY, it is the row the eager pass minted at the
// fallback kind and nothing has been filed into it — so the honest answer is that
// one library, at the kind the contents just proved, keeping its name and its
// slug. Minting a sibling instead is what produced two rows for a comics-only
// container: `Comics` holding nothing and `Comics (Comics)` carrying a qualifier
// that answers a question nobody asked.
//
// ⚠️ THE ROW IS RETYPED, NEVER REMOVED, AND THAT IS ADR-0066 DECISION 1. A
// container whose every item is skipped must still be bound and still render
// "with an item count of zero and a sentence". Nothing here can delete a library
// or decline to create one: the eager pass has already created it, and a walk
// that yields nothing simply never reaches this function.
//
// # Every guard on the retype, and the state each one refuses
//
//   - EXACTLY ONE library over the ref. Two means the container is already split
//     (ADR-0066 decision 5's mixed case) and both rows are somebody's answer.
//   - ZERO library_member rows. One member means real content was filed here and
//     retyping would relabel a library the user is looking at. Note that a comic
//     ISSUE files no membership (applyOneItem step 8 returns early for a child
//     kind), which is exactly why a comics-only container's `book` library reads
//     as empty and is retypable.
//   - EXACTLY ONE library_source row. A library fed by two containers, or by two
//     service instances, is shared, and retyping it on one container's evidence
//     would change the kind out from under the other.
//   - managed_by = 'auto'. A library the user made is theirs; §6.5 rule 4 makes
//     library.kind editable BY THE USER, and this must not overwrite that.
func bindProvisional(
	ctx context.Context, tx *sql.Tx, instanceID, userID int64, c CatalogueContainer,
	why bindReason, prior []libraryRow,
) (CatalogueBinding, bool, error) {
	if why == bindFromContainerList {
		// Step 1 has already established that none of these stands at the
		// fallback kind, so every candidate here is evidence and the first is
		// taken in containerBoundKinds' stable (kind, id) order — stable so a
		// re-import of a split container adopts the same one every time rather
		// than alternating.
		adopt := prior[0]
		if _, err := tx.ExecContext(ctx, `
			UPDATE library_source SET missing_since = NULL, container_identity = ?
			 WHERE service_instance_id = ? AND container_kind = 'remote_library' AND container_ref = ?`,
			c.Name, instanceID, c.RemoteID); err != nil {
			return CatalogueBinding{}, false, fmt.Errorf("refresh source: %w", err)
		}
		return CatalogueBinding{
			LibraryID: adopt.id, Kind: adopt.kind, UserID: userID, ContainerName: c.Name,
			Provisional: true,
		}, true, nil
	}

	if len(prior) != 1 {
		return CatalogueBinding{}, false, nil
	}
	var members, sources int64
	var managedBy string
	if err := tx.QueryRowContext(ctx, `
		SELECT l.managed_by,
		       (SELECT COUNT(*) FROM library_member lm WHERE lm.library_id = l.id),
		       (SELECT COUNT(*) FROM library_source ls WHERE ls.library_id = l.id)
		  FROM library l WHERE l.id = ?`, prior[0].id).Scan(&managedBy, &members, &sources); err != nil {
		return CatalogueBinding{}, false, fmt.Errorf(
			"read library %d before retyping it to %q: %w", prior[0].id, c.Kind, err)
	}
	if managedBy != "auto" || members != 0 || sources != 1 {
		return CatalogueBinding{}, false, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE library SET kind = ? WHERE id = ?`, c.Kind, prior[0].id); err != nil {
		return CatalogueBinding{}, false, fmt.Errorf(
			"retype library %d to %q: %w", prior[0].id, c.Kind, err)
	}
	return CatalogueBinding{
		LibraryID: prior[0].id, Kind: c.Kind, UserID: userID, ContainerName: c.Name,
		Provisional: true,
	}, true, nil
}

// containerBoundKinds is every (kind, library) this container is ALREADY bound
// to, read before this bind writes anything.
//
// It is ordered by kind so the sync_report detail below is stable between runs:
// a report whose field reorders is a report a reader cannot diff.
func containerBoundKinds(
	ctx context.Context, tx *sql.Tx, instanceID int64, remoteID string,
) ([]libraryRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT l.kind, l.id
		  FROM library_source ls
		  JOIN library l ON l.id = ls.library_id
		 WHERE ls.service_instance_id = ?
		   AND ls.container_kind = 'remote_library'
		   AND ls.container_ref = ?
		 ORDER BY l.kind, l.id`, instanceID, remoteID)
	if err != nil {
		return nil, fmt.Errorf("look up bound kinds for container %q: %w", remoteID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []libraryRow
	for rows.Next() {
		var r libraryRow
		if err := rows.Scan(&r.kind, &r.id); err != nil {
			return nil, fmt.Errorf("look up bound kinds for container %q: scan: %w", remoteID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("look up bound kinds for container %q: %w", remoteID, err)
	}
	return out, nil
}

// noteKindChange writes the SyncReportContainerKindChanged row, or does nothing.
//
// It does nothing in the two ordinary cases: a container nobody has bound before
// (prior is empty, which is every container of every first import) and the
// ADR-0066 decision 5 sibling mint, which is a second kind by design.
//
// It writes exactly one row when BindContainers — the adapter's own container
// list, one kind per container per import — asks for a kind this container was
// not bound at, because that means the adapter DECIDED differently than it did
// last time and this container's items land in a different library from now on.
// The detail carries both sides: a reader who finds a library that emptied
// itself needs to know where its items went, and a reader who finds a library
// that appeared needs to know it is not new data.
func noteKindChange(
	ctx context.Context, tx *sql.Tx, instanceID int64, c CatalogueContainer,
	why bindReason, prior []libraryRow, boundTo int64,
) error {
	if why != bindFromContainerList || len(prior) == 0 {
		return nil
	}
	was := make([]map[string]any, 0, len(prior))
	for _, r := range prior {
		was = append(was, map[string]any{"kind": r.kind, "library_id": r.id})
	}
	detail, err := json.Marshal(map[string]any{
		"name": c.Name,
		"was":  was,
		"now":  map[string]any{"kind": c.Kind, "library_id": boundTo},
	})
	if err != nil {
		return fmt.Errorf("encode kind change for container %q: %w", c.RemoteID, err)
	}
	return recordSyncReport(ctx, tx, instanceID,
		SyncReportContainerKindChanged, "library", c.RemoteID, string(detail))
}

type libraryRow struct {
	id   int64
	kind string
}

// userLibrarySet is what one user already holds, read ONCE and split into the
// two questions the bind path asks — which are different questions and were
// previously one map, which is how library 0 came to be invisible to a
// uniqueness check it participates in.
type userLibrarySet struct {
	// joinable is "which library may this container JOIN", keyed by
	// libraryNameKey. The reserved Unfiled row is absent: it is where the
	// membership derivation files a work belonging to nowhere else, and a
	// container binding into it would make that meaningless.
	joinable map[string]libraryRow

	// names and slugs are "which values would violate a UNIQUE index", and they
	// include EVERY row this user owns, Unfiled included.
	names map[string]bool
	slugs map[string]bool
}

func userLibraries(ctx context.Context, q querier, userID int64) (userLibrarySet, error) {
	out := userLibrarySet{
		joinable: map[string]libraryRow{},
		names:    map[string]bool{},
		slugs:    map[string]bool{},
	}
	rows, err := q.QueryContext(ctx,
		`SELECT id, name, slug, kind FROM library WHERE user_id = ?`, userID)
	if err != nil {
		return out, fmt.Errorf("list libraries for user %d: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var r libraryRow
		var name, slug string
		if err := rows.Scan(&r.id, &name, &slug, &r.kind); err != nil {
			return out, fmt.Errorf("list libraries for user %d: scan: %w", userID, err)
		}
		out.names[libraryNameKey(name)] = true
		out.slugs[slug] = true
		if r.id != UnfiledLibraryID {
			out.joinable[libraryNameKey(name)] = r
		}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("list libraries for user %d: %w", userID, err)
	}
	return out, nil
}

func insertLibrarySource(ctx context.Context, tx *sql.Tx, libraryID, instanceID int64, c CatalogueContainer) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO library_source
		  (library_id, service_instance_id, container_kind, container_ref, container_identity)
		VALUES (?,?, 'remote_library', ?, ?)
		ON CONFLICT (library_id, service_instance_id, container_kind, container_ref)
		DO UPDATE SET container_identity = excluded.container_identity, missing_since = NULL`,
		libraryID, instanceID, c.RemoteID, c.Name)
	if err != nil {
		return fmt.Errorf("bind source %s to library %d: %w", c.RemoteID, libraryID, err)
	}
	return nil
}

// ApplyCatalogueBatch writes one batch of replicated items in ONE
// BEGIN IMMEDIATE transaction.
//
// ONE TRANSACTION PER BATCH, sized by the caller at min(2000 rows, 100 ms)
// (reference/sync.md §6 rule 3). The wall-clock half is not decoration: every
// interactive write in the process funnels through the same single writer
// connection, so a batch that runs long stalls the request path, which is the
// failure the architecture exists to avoid.
//
// # The external_id write is fenced by a per-row SAVEPOINT, and why
//
// ux_extid_work_strong is UNIQUE(source, value) WHERE work_id IS NOT NULL AND
// confidence >= 1.0. Migration 0005 says a violation of it "is the merge signal,
// not an error". v0.1 HAS NO work_merge TABLE and this commit builds no merge
// machinery, so the signal is resolved the only honest way available:
//
//  1. BEFORE writing anything, an item with no existing link is resolved
//     through tier 1 of the §6.4 cascade — an exact strong external id, same
//     kind — and REUSES the work that already holds that id. That is where the
//     overwhelming majority of would-be conflicts are absorbed, and it is
//     "reuse the existing work id" done as a lookup rather than as error
//     recovery.
//  2. What tier 1 cannot absorb is a collision INSIDE the batch: two remote
//     items that both claim the same strong id but resolved to different works
//     (the first created a work, the second matched an older link). The
//     SAVEPOINT catches that, the offending external_id row is dropped, the item
//     keeps its own work and is reported as unidentified for that source, and
//     the conflict is returned in BatchResult.IdentityConflicts. NOTHING ELSE IN
//     THE BATCH IS LOST — which is the whole reason the savepoint is per row.
//
// A merge would be the right answer and it is not available; recording the
// signal and continuing is strictly better than aborting 2,000 rows over it.
//
// # The search-document invariants this function OWNS
//
// Migration 0005 names "the search-document builder — the code that writes
// search_doc, search_fts and search_trgm" as the owner of two invariants SQLite
// cannot declare. This is that builder:
//
//   - INVARIANT 5 — every search_doc row has at least one search_doc_library
//     row. Membership is written first; a work that ends up in no library is
//     filed into library 0 ("Unfiled") in the SAME transaction.
//   - INVARIANT 2 — count(search_fts) == count(search_trgm) == count(search_doc).
//     search_doc.rowid is THE allocator: the doc is inserted first, and its
//     rowid is then inserted EXPLICITLY into both FTS tables. A single implicit
//     rowid fuses unrelated documents, because RRF fuses on rowid.
//
// Both are asserted by TestSearchDocInvariantsAfterImport and
// TestSearchDocInvariantQueriesCatchABreak, which `make check` runs.
//
// It is a replication write and takes no Scope. See the file header.
func (s *Store) ApplyCatalogueBatch(
	ctx context.Context,
	instanceID int64,
	bindings map[string]CatalogueBinding,
	items []CatalogueItem,
	now time.Time,
) (BatchResult, error) {
	var res BatchResult
	if len(items) == 0 {
		return res, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res = BatchResult{}
		// ONE CACHE PER BATCH, not per process: it is scoped to the transaction
		// whose writes made its entries true, so a rolled-back batch cannot leave
		// a work id behind for the next one to hand a child to.
		parents := parentCache{}
		// SECOND CACHE, SAME LIFETIME AND THE SAME REASON. A provisional
		// container's library is re-derived from each item's own kind (see
		// resolveBinding), which without this would be one indexed query per
		// item — ARCHITECTURE §13 sizes that at ~90,000 comic issues. It is
		// per-batch and not per-process because its entries are only true
		// inside the transaction whose writes made them so.
		binds := bindCache{}
		for _, it := range items {
			b, ok := bindings[it.ContainerID]
			if !ok {
				// A container with no binding was DECLINED (no work.kind) or
				// SKIPPED (its library could not be created). Either way the
				// reason is already recorded upstream of here. Skipping rather
				// than erroring keeps one unusable Kavita library from failing
				// the whole import.
				continue
			}
			_, one, err := applyOneItem(ctx, tx, instanceID, b, it, now, parents, binds)
			if err != nil {
				return fmt.Errorf("apply %s %s from service_instance %d: %w",
					it.RemoteKind, it.RemoteID, instanceID, err)
			}
			res.add(one)
		}
		return nil
	})
	return res, err
}

// applyOneItem writes one replicated item.
//
// It is one linear sequence of ten dependent writes, and it is deliberately not
// split: ten helpers that each take the same eight values and the same *sql.Tx
// would move the order into call sites and make the ordering constraints
// (membership BEFORE the search doc, tier 1 BEFORE the external_id write)
// invisible. The order is the algorithm, which is why the complexity linters are
// answered rather than obeyed here.
//
// It returns the work id it wrote as well as the tally, because a CHILD item's
// parent is written through this same function and the child needs the id back.
//
//nolint:gocyclo,maintidx // see above: the order is the algorithm
func applyOneItem(
	ctx context.Context, tx *sql.Tx, instanceID int64,
	b CatalogueBinding, it CatalogueItem, now time.Time,
	parents parentCache, binds bindCache,
) (int64, BatchResult, error) {
	var res BatchResult
	nowStr := FormatTime(now)

	// ── 0a. WHICH LIBRARY THIS ITEM BELONGS IN, when the container's kind was
	// a guess rather than the upstream's answer (CatalogueContainer.KindProvisional).
	//
	// ⚠️ IT IS RE-ASKED RATHER THAN COMPARED, and `it.Kind != b.Kind` is NOT the
	// guard it looks like it should be. On a provisional container the binding's
	// Kind is what the EAGER pass guessed, and a comic reached earlier in this
	// same walk may have retyped that library to 'comic' underneath it — leaving a
	// binding that says 'book', names a library that is now 'comic', and agrees
	// with a prose book's own kind. Comparing the two would file the prose into
	// the comic library. The cache is what makes re-asking cheap.
	//
	// A CHILD KIND IS SKIPPED because it is filed into no library at all
	// (step 8 returns early for one); its parent's library is resolved by
	// parentBinding below, from the PARENT's kind.
	if b.Provisional && !childKinds[it.Kind] {
		rb, err := resolveBinding(ctx, tx, instanceID, b, it.ContainerID, it.Kind, binds)
		if err != nil {
			return 0, res, err
		}
		b = rb
	}

	// ── 0. THE PARENT, when this item is a child. It is written FIRST and in
	// this same transaction, so `parent_work_id` below is never null and never
	// points at a row a later failure could roll back out from under it
	// (ADR-0068 decision 1).
	//
	// The parent is an ordinary top-level item and is written by an ordinary
	// recursive call — one level deep and no deeper, because parentItem() clears
	// Parent. Writing it any other way would mean a second, parallel
	// implementation of the ten steps below, and the two would drift.
	var parentWorkID int64
	if it.Parent != nil {
		pb, err := parentBinding(ctx, tx, instanceID, b, it.ContainerID, *it.Parent, binds)
		if err != nil {
			return 0, res, err
		}
		key := parentKey{
			containerID: it.ContainerID,
			remoteKind:  it.Parent.RemoteKind,
			remoteID:    it.Parent.RemoteID,
		}
		if id, ok := parents[key]; ok {
			// ALREADY WRITTEN IN THIS BATCH. Ninety thousand issues sit under
			// three thousand series (ARCHITECTURE §13), so re-running the parent's
			// ten steps — including a full search-document delete-and-rebuild —
			// once per ISSUE would be thirty times the work for an identical row.
			parentWorkID = id
		} else {
			id, pres, err := applyOneItem(ctx, tx, instanceID, pb, parentItem(it), now, parents, binds)
			if err != nil {
				return 0, res, fmt.Errorf("apply parent %s %s: %w",
					it.Parent.RemoteKind, it.Parent.RemoteID, err)
			}
			parentWorkID = id
			parents[key] = id
			res.add(pres)
		}
	}

	// ── 1. An existing link pins the work. The upstream id is authoritative for
	// which work this is, so identity resolution does not re-run for it.
	var workID int64
	err := tx.QueryRowContext(ctx, `
		SELECT work_id FROM service_item_link
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?`,
		instanceID, it.RemoteKind, it.RemoteID).Scan(&workID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workID, res, fmt.Errorf("look up link: %w", err)
	}

	// ── 2. Tier 1 of the §6.4 cascade: an exact strong external id, SAME KIND.
	// The kind constraint is rule 1 of §6.4 — "never auto-merge across kind" —
	// and without it a graphic novel and its novelisation sharing an id would
	// collapse into one row.
	if workID == 0 {
		for _, x := range it.ExternalIDs {
			if x.Confidence < 1.0 {
				continue
			}
			var candidate int64
			err := tx.QueryRowContext(ctx, `
				SELECT e.work_id FROM external_id e
				  JOIN work w ON w.id = e.work_id
				 WHERE e.source = ? AND e.value = ? AND e.work_id IS NOT NULL
				   AND e.confidence >= 1.0 AND w.kind = ?`,
				x.Source, x.Value, it.Kind).Scan(&candidate)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return workID, res, fmt.Errorf("tier-1 identity lookup on %s=%s: %w", x.Source, x.Value, err)
			}
			workID = candidate
			res.WorksReused++
			break
		}
	}

	// ── 3. Create or update the work.
	switch workID {
	case 0:
		r, err := tx.ExecContext(ctx, `
			INSERT INTO work (kind, parent_work_id, title, sort_title, normalized_title, norm_version,
			                  original_title, overview, added_at, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			it.Kind, nullInt64(parentWorkID), it.Title, it.SortTitle, it.NormalizedTitle, it.NormVersion,
			nullString(it.OriginalTitle), nullString(it.Overview),
			nullTime(it.AddedAt), nowStr, nowStr)
		if err != nil {
			return workID, res, fmt.Errorf("insert work: %w", err)
		}
		if workID, err = r.LastInsertId(); err != nil {
			return workID, res, fmt.Errorf("insert work: id: %w", err)
		}
		res.WorksCreated++
	default:
		// parent_work_id is written on the UPDATE too, and by COALESCE rather
		// than by assignment: a re-import must be able to REPAIR a child whose
		// parent is missing, and must never blank a parent an earlier import set
		// because this particular pass happens to carry no CatalogueParent.
		if _, err := tx.ExecContext(ctx, `
			UPDATE work SET title = ?, sort_title = ?, normalized_title = ?, norm_version = ?,
			                original_title = ?, overview = ?,
			                parent_work_id = COALESCE(?, parent_work_id),
			                updated_at = ?, deleted_at = NULL
			 WHERE id = ?`,
			it.Title, it.SortTitle, it.NormalizedTitle, it.NormVersion,
			nullString(it.OriginalTitle), nullString(it.Overview),
			nullInt64(parentWorkID), nowStr, workID); err != nil {
			return workID, res, fmt.Errorf("update work %d: %w", workID, err)
		}
		res.WorksUpdated++
	}

	// ── 4. The subtype row. Its presence is what makes the kind concrete, so it
	// is written even when every column in it is NULL.
	switch it.Kind {
	case "book":
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_book (work_id, page_count) VALUES (?,?)
			ON CONFLICT (work_id) DO UPDATE SET page_count = excluded.page_count`,
			workID, it.PageCount); err != nil {
			return workID, res, fmt.Errorf("upsert work_book %d: %w", workID, err)
		}
	case "comic":
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_comic (work_id, reading_direction) VALUES (?,?)
			ON CONFLICT (work_id) DO UPDATE SET reading_direction = excluded.reading_direction`,
			workID, it.ReadingDirection); err != nil {
			return workID, res, fmt.Errorf("upsert work_comic %d: %w", workID, err)
		}
	case "comic_issue":
		// is_oneshot IS IN THE COLUMN LIST AND IN THE DO UPDATE, which is the
		// whole of ADR-0068 decision 2's "the flag is WRITTEN, not merely
		// tolerated". Leaving it to `DEFAULT 0` would make a one-shot
		// indistinguishable from an unimported issue, and the column would have
		// no writer at all — "a column with a DEFAULT 0 and no writer is a deaf
		// column, and this project has found several".
		//
		// The four columns this does NOT write — volume_label, volume_sort,
		// is_special, special_version — have no source on a BookOrbit card and
		// are left to their defaults rather than guessed at. `special_version`
		// enumerates 'one-shot' and is deliberately not written from is_oneshot:
		// they are different facts (a flag versus an edition label) and ADR-0030
		// allocated both.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_comic_issue (work_id, number_text, number_sort, is_oneshot, page_count)
			VALUES (?,?,?,?,?)
			ON CONFLICT (work_id) DO UPDATE SET
			  number_text = excluded.number_text,
			  number_sort = excluded.number_sort,
			  is_oneshot  = excluded.is_oneshot,
			  page_count  = excluded.page_count`,
			workID, it.NumberText, it.NumberSort, it.IsOneshot, it.PageCount); err != nil {
			return workID, res, fmt.Errorf("upsert work_comic_issue %d: %w", workID, err)
		}
	default:
		return workID, res, fmt.Errorf("no subtype table for work.kind %q", it.Kind)
	}

	// ── 5. Alternate titles. Replaced wholesale for this work: an upstream that
	// drops a localised name must not leave the old one searchable forever.
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_alt_title WHERE work_id = ?`, workID); err != nil {
		return workID, res, fmt.Errorf("clear alt titles for work %d: %w", workID, err)
	}
	for _, a := range it.AltTitles {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_alt_title (work_id, title, normalized, kind, language)
			VALUES (?,?,?,?,?)`,
			workID, a.Title, a.Normalized, a.Kind, nullString(a.Language)); err != nil {
			return workID, res, fmt.Errorf("insert alt title %q for work %d: %w", a.Title, workID, err)
		}
	}

	// ── 6. External ids, each behind its own SAVEPOINT. See the doc comment.
	for i, x := range it.ExternalIDs {
		sp := fmt.Sprintf("extid_%d", i)
		if _, err := tx.ExecContext(ctx, "SAVEPOINT "+sp); err != nil {
			return workID, res, fmt.Errorf("savepoint %s: %w", sp, err)
		}
		_, insErr := tx.ExecContext(ctx, `
			INSERT INTO external_id (work_id, edition_id, source, value, confidence)
			VALUES (?, NULL, ?, ?, ?)
			ON CONFLICT (source, value, COALESCE(work_id, -1), COALESCE(edition_id, -1))
			DO UPDATE SET confidence = excluded.confidence`,
			workID, x.Source, x.Value, x.Confidence)
		if insErr == nil {
			if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
				return workID, res, fmt.Errorf("release savepoint %s: %w", sp, err)
			}
			res.ExternalIDsWritten++
			continue
		}
		// The conflict is the merge signal. Undo just this row and carry on.
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sp); err != nil {
			return workID, res, fmt.Errorf("rollback to savepoint %s after %w: %w", sp, insErr, err)
		}
		if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
			return workID, res, fmt.Errorf("release savepoint %s: %w", sp, err)
		}
		var owner int64
		if err := tx.QueryRowContext(ctx, `
			SELECT work_id FROM external_id
			 WHERE source = ? AND value = ? AND work_id IS NOT NULL AND confidence >= 1.0`,
			x.Source, x.Value).Scan(&owner); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return workID, res, fmt.Errorf("read conflicting external_id %s=%s: %w", x.Source, x.Value, err)
		}
		res.IdentityConflicts = append(res.IdentityConflicts, IdentityConflict{
			Source: x.Source, Value: x.Value,
			ExistingWorkID: owner, AttemptedWorkID: workID, RemoteID: it.RemoteID,
		})
	}

	// ── 7. The service link.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO service_item_link (
		  service_instance_id, work_id, remote_id, remote_kind, remote_path,
		  remote_library_id, remote_subtype, has_file, remote_updated_at,
		  remote_hash, remote_identity_hash, synced_at, deleted_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,NULL)
		ON CONFLICT (service_instance_id, remote_kind, remote_id) DO UPDATE SET
		  work_id = excluded.work_id,
		  remote_path = excluded.remote_path,
		  remote_library_id = excluded.remote_library_id,
		  remote_subtype = excluded.remote_subtype,
		  has_file = excluded.has_file,
		  remote_updated_at = excluded.remote_updated_at,
		  remote_hash = excluded.remote_hash,
		  synced_at = excluded.synced_at,
		  deleted_at = NULL`,
		instanceID, workID, it.RemoteID, it.RemoteKind, nullString(it.RemotePath),
		nullString(it.ContainerID), nullString(it.RemoteSubtype), it.HasFile,
		nullTime(it.RemoteUpdatedAt), it.remoteHash(), it.identityHash(), nowStr); err != nil {
		return workID, res, fmt.Errorf("upsert link: %w", err)
	}
	// remote_identity_hash is written AT FIRST SIGHT and never overwritten
	// (sync.md §4 guard 1): it is the O(1) comparison that tells an id reused
	// upstream from the same item coming back. The ON CONFLICT list above
	// deliberately omits it.

	// ── 8 AND 9 ARE THE TOP-LEVEL HALF, AND A CHILD KIND SKIPS BOTH.
	//
	// ⚠️ THIS IS NOT AN OPTIMISATION AND IT IS NOT OPTIONAL: writeSearchDoc
	// RETURNS AN ERROR on an excluded kind rather than skipping it, so a
	// `comic_issue` routed through step 9 does not degrade to an empty screen —
	// IT FAILS THE IMPORT. corpusExcludedKinds' own doc names this caller ("the
	// phase-B comic_issue walk is the obvious one"), and ADR-0068 is the ADR that
	// makes it real. The guard stays exactly as it is; the caller routes around
	// it. The membership skip travels with it for one reason rather than for
	// convenience: `library_member` is what §17.8's item count counts and what
	// ADR-0051's library predicate tests, so filing 90,000 issues would make a
	// comic library's item count read issues while /library/comics reads series
	// — one library with two different meanings for "how many".
	//
	// The CHILD IS NOT INVISIBLE for it: it is reachable through
	// `parent_work_id` and ix_work_parent from a series that IS filed and IS
	// indexed, which is the whole shape ADR-0030 chose over parentless issues.
	//
	// Step 10's identity count goes with them, and that is deliberate too:
	// `Unidentified` is a figure about the top-level works this batch wrote, and
	// every issue counting itself into it would make an import of 90,000 issues
	// under 3,000 identified series report 90,000 unidentified works.
	if childKinds[it.Kind] {
		return workID, res, nil
	}

	// ── 8. Membership. Rewritten for this (library, work) pair, because
	// sort_title leads the primary key and a retitle would otherwise leave a
	// second member row under the old key.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM library_member WHERE library_id = ? AND work_id = ?`,
		b.LibraryID, workID); err != nil {
		return workID, res, fmt.Errorf("clear membership of work %d in library %d: %w", workID, b.LibraryID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO library_member (library_id, sort_title, work_id, edition_id, added_at)
		VALUES (?,?,?,0,?)`, b.LibraryID, it.SortTitle, workID, nowStr); err != nil {
		return workID, res, fmt.Errorf("file work %d into library %d: %w", workID, b.LibraryID, err)
	}
	res.Members++

	// ── 9. The search document. THIS IS THE INVARIANT-OWNING BLOCK.
	//
	// The `people` column is read from work_credit rather than taken from `it`,
	// and it is read HERE — on the ITEM pass — even though the credit pass has
	// not run yet in this import. That is not pointless on a re-import: the
	// credits from the PREVIOUS import are still in work_credit when this runs,
	// and writing "" would blank the column for the whole window between the two
	// passes. On a first import it reads back nothing and the credit pass in
	// §credits.go fills it.
	d := it.searchDocText()
	people, err := creditedNames(ctx, tx, workID)
	if err != nil {
		return workID, res, err
	}
	d.people = people
	if err := writeSearchDoc(ctx, tx, workID, d); err != nil {
		return workID, res, err
	}
	res.SearchDocs++

	// ── 10. The "not identified" state is DERIVED, not stored. §6.4 describes it
	// as costing "one nullable column", and migration 0005 shipped no such
	// column — status is read off the tree, so the marker is the absence of an
	// external_id row, which ix_extid_work(work_id, source) serves as a covered
	// seek. Nothing to write; it is counted so the import can report it.
	var identified int
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM external_id WHERE work_id = ?)`, workID).Scan(&identified); err != nil {
		return workID, res, fmt.Errorf("read identity state of work %d: %w", workID, err)
	}
	if identified == 0 {
		res.Unidentified++
	}
	return workID, res, nil
}

// childKinds is the set of work.kind values that are NEVER a top-level item.
//
// ⚠️ IT IS THE SAME MEMBERSHIP QUESTION `corpusExcludedKinds` ASKS AND IT IS A
// DIFFERENT LIST ON PURPOSE. That one is "what search.md §2 keeps out of the
// corpus" and it holds 'person' — a kind that is excluded for a DESTINATION
// reason (there is no person screen) and is not a child of anything. This one is
// "what hangs under a parent", which is what decides whether a row is filed into
// a library. Folding the two into one map would make the person kind stop being
// filed, or make a comic issue searchable, depending on which list won.
//
// 'season' and 'track' are here for the day an adapter reports them; nothing
// writes them today, exactly as nothing wrote 'comic_issue' before ADR-0068.
var childKinds = map[string]bool{
	"season": true, "episode": true, "track": true, "comic_issue": true,
}

// parentKey identifies one parent within a batch.
//
// ⚠️ THE CONTAINER IS PART OF THE KEY, and leaving it out is a real defect
// rather than a redundancy. ADR-0068 is explicit that "a BookOrbit series is NOT
// library-scoped upstream", so one series can carry issues from two containers —
// and step 8 files membership per (library, work) pair, ADDING a row rather than
// replacing one. A cache keyed on the upstream id alone would write the parent
// once, for the first container, and the second container's comic library would
// silently never gain the series. Keyed this way the parent is written once per
// (container, series), which is exactly the granularity membership needs.
type parentKey struct{ containerID, remoteKind, remoteID string }

// parentCache is the parents already written in THIS batch, by upstream id.
//
// ARCHITECTURE §13 sizes the shape it exists for: ~3,000 comic series carrying
// ~90,000 issues. Without it every issue would re-run its series' ten steps,
// including a full search-document DELETE-and-INSERT across three tables, for a
// row that has not changed since the previous issue wrote it.
type parentCache map[parentKey]int64

// parentItem projects a child's CatalogueParent onto the CatalogueItem the
// recursive call writes.
//
// ⚠️ Parent IS CLEARED, and that is what bounds the recursion at one level.
// UsArr's deepest cascade is three (series→season→episode), and nothing needs
// the middle level yet; when something does, this is the function that grows a
// second level rather than a new code path.
//
// It carries NO external ids, NO alt titles, NO page count and NO path. A series
// is not the file: it has no bytes, no ISBN and no format, and inventing any of
// them from the child's would put a fact about one issue onto the whole series.
// HasFile is false for the same reason — §6.3's availability for a series comes
// from the rollup over its children, never from a flag set here.
func parentItem(it CatalogueItem) CatalogueItem {
	p := it.Parent
	return CatalogueItem{
		RemoteID:        p.RemoteID,
		RemoteKind:      p.RemoteKind,
		ContainerID:     it.ContainerID,
		Kind:            p.Kind,
		Title:           p.Title,
		SortTitle:       p.SortTitle,
		NormalizedTitle: p.NormalizedTitle,
		NormVersion:     p.NormVersion,
		AddedAt:         it.AddedAt,
	}
}

// parentBinding is the library the PARENT is filed into, which is not always the
// library the child's container bound to. resolveBinding owns the rest of it.
func parentBinding(
	ctx context.Context, tx *sql.Tx, instanceID int64,
	b CatalogueBinding, containerRef string, p CatalogueParent, binds bindCache,
) (CatalogueBinding, error) {
	// ⚠️ `!b.Provisional` IS PART OF THE FAST PATH'S CONDITION, not decoration.
	// On a provisional container the binding's Kind is the eager pass's guess and
	// the library it names may already have been retyped by an earlier item in
	// this same walk, so agreement between the two proves nothing and the
	// question is re-asked. For every other adapter this is exactly the
	// comparison it always was.
	if p.Kind == b.Kind && !b.Provisional {
		return b, nil
	}
	return resolveBinding(ctx, tx, instanceID, b, containerRef, p.Kind, binds)
}

// bindKey is one (container, kind) pair inside a batch. bindCache is what
// resolveBinding remembers, for the lifetime of one ApplyCatalogueBatch
// transaction.
type bindKey struct{ containerRef, kind string }

type bindCache map[bindKey]CatalogueBinding

// resolveBinding is the library one container's items of ONE kind are filed
// into, minted if it is not there yet.
//
// # ADR-0066 decision 5, activated by ADR-0068
//
// One upstream container may hold prose and comics. `library.kind` is exactly
// one value, so such a container "becomes a `book` library and a `comic` library
// over the same `library_source` container ref" — and this is where the second
// one comes from. It needs no migration: 'comic' is already a permitted
// `library.kind`, and `library_source`'s uniqueness is (library_id,
// service_instance_id, container_kind, container_ref), so two libraries may name
// the same container.
//
// # It is LAZY, and that is the difference between decision 5 and an empty screen
//
// The bind pass runs BEFORE the walk, and no upstream tells UsArr what is inside
// a container — BookOrbit's `libraries` table has no kind column at all. Minting
// the comic library at bind time would therefore give EVERY prose-only library a
// permanently empty comic sibling on the Libraries screen, which is the "empty
// screen that looks broken" principle 3 exists to prevent. Minting it on the
// first comic actually seen is what makes decision 5's word MIXED true.
//
// ⚠️ IT CAN ALSO RETYPE RATHER THAN MINT, and that is bindProvisional's job
// rather than this one's. A container that turns out to hold ONLY comics has an
// empty `book` library standing over it from the eager pass, and the answer there
// is that one library at the proven kind — one row, named for the container, with
// no qualifier — not a second row beside a first that will stay empty for ever.
//
// ⚠️ NO SERIES WORK IS EVER MINTED INTO NO LIBRARY AT ALL (ADR-0068 decision 5).
// Every failure below is an error rather than a fallback to library 0.
func resolveBinding(
	ctx context.Context, tx *sql.Tx, instanceID int64,
	b CatalogueBinding, containerRef, kind string, binds bindCache,
) (CatalogueBinding, error) {
	k := bindKey{containerRef: containerRef, kind: kind}
	if got, ok := binds[k]; ok {
		return got, nil
	}
	if b.UserID == 0 && b.ContainerName == "" {
		// A binding built without the two facts a sibling mint needs. It is a
		// programming error at the call site rather than a state the schema can
		// produce, and it is refused rather than defaulted: guessing a user id
		// would put one person's library under another's.
		return CatalogueBinding{}, fmt.Errorf(
			"container %q needs a %q library beside its %q one and the binding carries no owner "+
				"(store.BindContainers stamps CatalogueBinding.UserID; a hand-built binding must too)",
			containerRef, kind, b.Kind)
	}
	rb, err := bindOneContainer(ctx, tx, instanceID, b.UserID, CatalogueContainer{
		// THE CONTAINER REF IS THE ISSUE'S OWN, verbatim — "the `comic` library
		// minted over the `library_source` container ref the issue's book was
		// walked from" (ADR-0068 decision 5). A synthesized ref would be a second
		// container that no upstream ever reported.
		RemoteID: containerRef,
		Name:     b.ContainerName,
		Kind:     kind,
		// CARRIED THROUGH, because it is what licenses the retype below. It is
		// the CONTAINER's property and the binding is the only place the item
		// path can still read it from.
		KindProvisional: b.Provisional,
		// bindSiblingKind, NOT bindFromContainerList. This call is the ONE place
		// a container is deliberately bound at a second kind, so the kind-change
		// report must not fire here — it would fire on every mixed library, on
		// the first comic reached, and say a change happened where the design
		// says two kinds coexist.
	}, bindSiblingKind)
	if err != nil {
		return CatalogueBinding{}, err
	}
	binds[k] = rb
	return rb, nil
}

// searchDocText is the TEXT of one work's search document — the five FTS
// columns plus the two search_doc columns derived from the same facts.
//
// It exists because the document now has TWO writers with two different sources
// for the same fields: the item pass, which already holds every value on the
// CatalogueItem it just wrote, and the credit pass, which holds none of them and
// must re-read them from the replica. One struct and one writer (writeSearchDoc)
// keep the two from drifting into two different documents;
// TestTheCreditPassRebuildKeepsEveryOtherColumn is what proves they have not.
type searchDocText struct {
	kind          string
	normTitle     string
	title         string
	originalTitle string
	altTitles     string
	people        string
	overview      string
}

// searchDocText builds the document from the item the adapter projected. It
// leaves `people` empty; the caller fills it from work_credit, which is the one
// field no CatalogueItem carries.
func (it CatalogueItem) searchDocText() searchDocText {
	return searchDocText{
		kind:          it.Kind,
		normTitle:     it.NormalizedTitle,
		title:         it.Title,
		originalTitle: it.OriginalTitle,
		altTitles:     it.altTitleBlob(),
		overview:      it.Overview,
	}
}

// readSearchDocText rebuilds the document from the replica, for the writer that
// does not hold a CatalogueItem — the credit pass.
//
// Every field comes from a row the item pass wrote in an earlier transaction, so
// this is a re-derivation and not a second source of truth. It is three reads
// rather than one because alt titles and credits are both 1:N.
//
// ⚠️ ONE FIELD CAN DIVERGE FROM WHAT THE ITEM PASS WROTE, and it is pre-existing
// rather than introduced here: `kind`. The item pass takes it from the container
// binding and writes it into search_doc, but its UPDATE of an existing work does
// NOT set work.kind, so a container an operator retyped in Kavita leaves
// work.kind at the old value and search_doc.kind at the new one. This function
// reads work.kind, i.e. what the rest of the replica — the subtype table,
// recent.go's queries — already believes. Reconciling the two is a separate
// question (changing a work's kind means moving its subtype row) and is not
// this function's to answer.
func readSearchDocText(ctx context.Context, tx *sql.Tx, workID int64) (searchDocText, error) {
	var d searchDocText
	var orig, overview sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT kind, normalized_title, title, original_title, overview
		  FROM work WHERE id = ?`, workID).
		Scan(&d.kind, &d.normTitle, &d.title, &orig, &overview); err != nil {
		return d, fmt.Errorf("read work %d for its search document: %w", workID, err)
	}
	d.originalTitle, d.overview = orig.String, overview.String

	// ORDER BY id is insertion order, which is the order the adapter reported —
	// work_alt_title has an INTEGER PRIMARY KEY and the item pass inserts the
	// alt titles in CatalogueItem.AltTitles order. Nothing about FTS matching
	// depends on it; determinism does, so a test can compare two builds.
	alt, err := scanStrings(ctx, tx,
		`SELECT title FROM work_alt_title WHERE work_id = ? ORDER BY id`, workID)
	if err != nil {
		return d, fmt.Errorf("read alt titles of work %d: %w", workID, err)
	}
	d.altTitles = strings.Join(alt, "\n")

	if d.people, err = creditedNames(ctx, tx, workID); err != nil {
		return d, err
	}
	return d, nil
}

// creditedNames is the `search_fts.people` column: every name credited on this
// work, newline-joined, so that a search for a creator returns the WORK.
//
// # Why the column is filled at all, when 'person' is excluded from the corpus
//
// The two rules point in opposite directions and both hold. schema.md §1.1 and
// search.md §2 keep 'person' WORKS out of search_doc because a person hit is a
// result row with nowhere to land — there is no person screen in any milestone —
// and TestPeopleNeverEnterTheSearchCorpus enforces it. This column is the
// opposite construction: it puts a creator's NAME on the book's document, so
// "Susanna Clarke" returns Piranesi, which is a destination that exists. Filling
// it adds no row to the corpus at all, so it settles the ⚠️ both documents carry
// — "find everything by this author is unanswered in v0.1" — without touching
// the exclusion those documents state.
//
// IT IS NOT THE CANDIDATE EITHER DOCUMENT NAMED. Both proposed folding the names
// into `alt_titles`; this uses `people`, which already exists in the FTS table
// and was already reserved for exactly this. The difference is not cosmetic: a
// name in `alt_titles` is indistinguishable from a title at query time, so it
// can never be weighted, filtered or explained separately, whereas `people` is
// its own fts5 column. REVIEW-LOG LS-100 carries the argument.
//
// # Every role, not a chosen subset
//
// All eighteen members of work_credit.role qualify, and the filter that was
// considered — authorship-ish roles only — is refused on two grounds. It would
// be a SECOND vocabulary in Go beside the CHECK that defines the first, free to
// drift from it (credits.go's Role comment refuses exactly that duplication for
// the same reason). And in a comics-first v0.1 it would empty the column: the
// nine roles Kavita actually reports are mostly the comics ones, so "authors
// only" means a library of manga whose pencillers, letterers and cover artists
// are unsearchable while the column looks populated. A user who types "Vince
// Haig" wants the book he drew the cover for; there is no other screen that
// answers them.
//
// The ranking objection — a long people list diluting a title match — is real
// and is not this function's to answer. `people` is its own FTS column, and
// fts5's bm25() takes one weight per column, so search.md §4's fusion query can
// down-weight it when retrieval lands; today that query passes no weights at all
// because nothing queries either FTS table yet. Choosing `alt_titles` instead
// would have closed that door, which is the concrete reason this column was
// taken over the candidate the documents named.
//
// # credited_as AND the person work's title, not one or the other
//
// Both strings are emitted when they differ. The person work's title is the
// first spelling any source used for that human; credited_as is what THIS
// release printed ("Sting" on a Police reissue). A user searching either one is
// asking the same question, so dropping either loses a real query. The UNION
// dedupes exactly — the same person credited in three roles on one work
// contributes one name, not three, which keeps the term frequency honest.
//
// Kavita reports no printed variant, so credited_as is NULL on every row this
// project writes today; the branch is here for the sources that do.
func creditedNames(ctx context.Context, tx *sql.Tx, workID int64) (string, error) {
	names, err := scanStrings(ctx, tx, `
		SELECT w.title FROM work_credit c JOIN work w ON w.id = c.creator_work_id
		 WHERE c.work_id = ?
		UNION
		SELECT c.credited_as FROM work_credit c
		 WHERE c.work_id = ? AND c.credited_as IS NOT NULL AND trim(c.credited_as) <> ''
		ORDER BY 1`, workID, workID)
	if err != nil {
		return "", fmt.Errorf("read credited names of work %d: %w", workID, err)
	}
	// Newline-joined for altTitleBlob's reason: both tokenizers treat it as a
	// separator, so the join cannot fuse one name's end onto the next name's
	// start into a token neither contains.
	return strings.Join(names, "\n"), nil
}

// corpusExcludedKinds is search.md §2's exclusion list, in the one place the
// document writer can enforce it.
//
// It is a WRITER-SIDE refusal rather than only an after-the-fact assertion,
// because that assertion (TestPeopleNeverEnterTheSearchCorpus) can only report a
// corpus that has already been corrupted, and the FTS tables carry no foreign key, so a bad doc
// cannot be cleaned up by a cascade. Failing the import is the correct blast
// radius: an excluded doc is also a search_doc_library row, so it appears inside
// the user's library grid as an item.
//
// THE FIVE KINDS ARE EXCLUDED FOR TWO DIFFERENT REASONS and search.md §2 keeps
// them apart: 'season', 'episode', 'track' and 'comic_issue' are a VOLUME
// problem (~400k episode rows against ~13k top-level works, swamping every
// query), 'person' is a DESTINATION problem (no person screen in any milestone).
// The refusal is the same either way, which is why one list serves both.
//
// ⚠️ NOTHING SHIPPED REACHES IT. Kavita's adapter maps containers to 'comic' and
// 'book' only (internal/libsync/kavita.go), so today this is a guard against a
// future caller — the phase-B comic_issue walk is the obvious one — routing a
// child kind through the top-level item path. Its witness has to construct the
// call directly; see TestTheDocumentWriterRefusesAnExcludedKind.
var corpusExcludedKinds = map[string]bool{
	"season": true, "episode": true, "track": true, "comic_issue": true, "person": true,
}

// writeSearchDoc replaces one work's search document across all three tables.
//
// The order is forced and every step of it is load-bearing:
//
//  1. read the work's existing doc rowids;
//  2. delete them from search_fts and search_trgm BY ROWID, then from
//     search_doc — the FTS tables carry no foreign key and no trigger can reach
//     them, so a doc deleted without this leaves two orphan FTS rows and breaks
//     invariant 2 permanently;
//  3. insert search_doc FIRST, because its rowid is THE allocator;
//  4. insert that rowid EXPLICITLY into both FTS tables;
//  5. file the doc under every library the work is a member of, and under
//     library 0 if that set is empty — invariant 5.
//
// EVERY WRITER SUPPLIES ALL FIVE FTS COLUMNS, and that is forced rather than
// tidy. Measured against this schema (a throwaway probe, run and then deleted):
// fts5 answers a partial update with "cannot UPDATE a subset of columns on fts5
// contentless-delete table: search_fts", and `SELECT people FROM search_fts`
// returns NULL, so the columns an update would leave alone cannot be read back
// and reconstructed either. Hence searchDocText: whoever rebuilds the document
// must hold all five values, and readSearchDocText exists for the writer that
// holds none of them.
//
// ⚠️ The all-column form DOES work — the same probe ran `UPDATE search_fts SET
// title=…, original_title=…, alt_titles=…, people=…, overview=…` with no error —
// so delete-then-insert is not the only mechanism for search_fts alone. It is
// still the mechanism here, because search_doc.rowid is the allocator for all
// three tables and this function rewrites search_doc and search_doc_library
// beside the two index rows; an UPDATE path would have to keep three tables and
// a junction in step by hand where the delete keeps them in step by construction.
func writeSearchDoc(ctx context.Context, tx *sql.Tx, workID int64, d searchDocText) error {
	if corpusExcludedKinds[d.kind] {
		return fmt.Errorf("work %d is of kind %q, which search.md §2 excludes from the "+
			"search corpus — a document for it would swamp the corpus or be a result row "+
			"with nowhere to land, and its search_doc_library row would put it in the "+
			"user's library grid as an item", workID, d.kind)
	}
	old, err := existingDocRowIDs(ctx, tx, workID)
	if err != nil {
		return err
	}
	for _, id := range old {
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_fts WHERE rowid = ?`, id); err != nil {
			return fmt.Errorf("delete search_fts %d: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_trgm WHERE rowid = ?`, id); err != nil {
			return fmt.Errorf("delete search_trgm %d: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_doc WHERE rowid = ?`, id); err != nil {
			return fmt.Errorf("delete search_doc %d: %w", id, err)
		}
	}

	// popularity and title_idf stay at their defaults. Kavita reports neither,
	// and a fabricated ranking signal is worse than an absent one: title_idf is
	// a corpus statistic that only a pass over the whole corpus can compute.
	res, err := tx.ExecContext(ctx, `
		INSERT INTO search_doc (work_id, kind, popularity, in_library, title_idf, norm_title)
		VALUES (?,?,0,1,0,?)`, workID, d.kind, d.normTitle)
	if err != nil {
		return fmt.Errorf("insert search_doc for work %d: %w", workID, err)
	}
	docID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("insert search_doc for work %d: rowid: %w", workID, err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO search_fts (rowid, title, original_title, alt_titles, people, overview)
		VALUES (?,?,?,?,?,?)`,
		docID, d.title, d.originalTitle, d.altTitles, d.people, d.overview); err != nil {
		return fmt.Errorf("insert search_fts %d: %w", docID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO search_trgm (rowid, title, alt_titles) VALUES (?,?,?)`,
		docID, d.title, d.altTitles); err != nil {
		return fmt.Errorf("insert search_trgm %d: %w", docID, err)
	}

	r, err := tx.ExecContext(ctx, `
		INSERT INTO search_doc_library (library_id, doc_rowid)
		SELECT DISTINCT library_id, ? FROM library_member WHERE work_id = ?`, docID, workID)
	if err != nil {
		return fmt.Errorf("scope search_doc %d: %w", docID, err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return fmt.Errorf("scope search_doc %d: count: %w", docID, err)
	}
	if n == 0 {
		// Invariant 5. A doc with no scope is invisible to every user including
		// its owner, so it is filed into the reserved Unfiled library instead.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO search_doc_library (library_id, doc_rowid) VALUES (?,?)`,
			UnfiledLibraryID, docID); err != nil {
			return fmt.Errorf("file search_doc %d as unfiled: %w", docID, err)
		}
	}
	return nil
}

// existingDocRowIDs reads the search_doc rowids a work already has.
//
// It is its own function so the *sql.Rows is CLOSED BEFORE the caller issues its
// next statement on the same transaction. A cursor left open across the deletes
// below is a read and a write interleaved on one connection, which is the shape
// that makes SQLite return SQLITE_BUSY on a statement that should never have
// contended with anything.
func existingDocRowIDs(ctx context.Context, tx *sql.Tx, workID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT rowid FROM search_doc WHERE work_id = ?`, workID)
	if err != nil {
		return nil, fmt.Errorf("read search docs for work %d: %w", workID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read search docs for work %d: scan: %w", workID, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read search docs for work %d: %w", workID, err)
	}
	return out, nil
}

// scanStrings runs a one-column query and drains it into a slice.
//
// It exists for existingDocRowIDs' reason and not for brevity: the *sql.Rows is
// CLOSED before the caller issues its next statement, and both callers here are
// mid-transaction on the single writer connection, where a cursor held open
// across a write is the shape that produces a SQLITE_BUSY on a statement that
// contended with nothing.
func scanStrings(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// altTitleBlob is what goes into the FTS tables' alt_titles column: this work's
// alternate titles, newline-joined. Both tokenizers treat the newline as a
// separator, so the join cannot fuse the end of one title with the start of the
// next into a token neither contains.
func (it CatalogueItem) altTitleBlob() string {
	if len(it.AltTitles) == 0 {
		return ""
	}
	parts := make([]string, 0, len(it.AltTitles))
	for _, a := range it.AltTitles {
		parts = append(parts, a.Title)
	}
	return strings.Join(parts, "\n")
}

// remoteHash covers the SYNCED SUBSET only (schema.md §5), which is what makes
// the channel-4 drift comparison worth anything: a hash over the whole upstream
// payload churns on read counts and cover colours and would report every row as
// drifted on every sweep.
func (it CatalogueItem) remoteHash() string {
	return hashFields(
		it.Title, it.SortTitle, it.OriginalTitle, it.Kind,
		it.ContainerID, it.RemotePath, it.RemoteSubtype,
		formatOrEmpty(it.RemoteUpdatedAt), boolString(it.HasFile),
	)
}

// identityHash covers the remote's external ids and nothing else. sync.md §4
// guard 1 compares it to decide whether a remote id that came back is the same
// content or a REUSED id pointing at something else.
func (it CatalogueItem) identityHash() string {
	parts := make([]string, 0, len(it.ExternalIDs))
	for _, x := range it.ExternalIDs {
		parts = append(parts, x.Source+"="+x.Value)
	}
	sort.Strings(parts) // upstream field order must not change the hash
	return hashFields(parts...)
}

// hashFields is the one hash both remote_hash and remote_identity_hash use.
//
// The separator is 0x1f (unit separator) rather than a comma or a colon,
// because it cannot occur in any of the fields being joined: with a printable
// separator, ("a|b", "c") and ("a", "b|c") hash identically, and a drift check
// that cannot tell those apart is a drift check with a blind spot.
func hashFields(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0x1f})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func boolString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func formatOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return FormatTime(t)
}

// nullInt64 renders a work id as NULL when it is 0, so `parent_work_id` is a
// real SQL NULL for a top-level work rather than a foreign key pointing at a row
// id that cannot exist.
func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return FormatTime(t)
}

// StampFullSync records that a full import COMPLETED, using the instant that
// import STARTED.
//
// ⚠️ THE TWO HALVES OF THAT SENTENCE ARE DIFFERENT INSTANTS AND BOTH ARE
// DELIBERATE. `at` is the run's START (libsync's FullImport passes
// rep.StartedAt), and the call happens only after the run finished. So the
// column means "the replica is no staler than `at`", which is the only reading
// a freshness banner can safely render: a full import re-reads every item
// between its start and its finish, so the START is the lower bound on every
// row it wrote and the finish is not a bound at all. Passing the finish here
// would overstate freshness by the run's duration, on the one screen —
// ARCHITECTURE.md §17.7's degraded banner — whose whole job is that number.
// docs/reference/http-api.md §3.5 is the wire contract, and it names the three
// instants this is NOT.
//
// ON SUCCESS ONLY, and the caller is what enforces that: this function is not
// called on a failed or partial import. The column is a freshness claim the
// Services screen renders, and a claim written after a stream died mid-array
// would say the replica is current when it holds half a library.
//
// It is a replication write and takes no Scope.
func (s *Store) StampFullSync(ctx context.Context, instanceID int64, at time.Time) error {
	if at.IsZero() {
		at = time.Now()
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET last_full_sync_at = ? WHERE id = ? AND deleted_at IS NULL`,
			FormatTime(at), instanceID)
		if err != nil {
			return fmt.Errorf("stamp last_full_sync_at on service_instance %d: %w", instanceID, err)
		}
		return expectOneRow(res, "service_instance", instanceID)
	})
}

// LastFullSyncAt reports when one instance's last SUCCESSFULLY COMPLETED full
// import BEGAN, or an invalid NullString for never. See StampFullSync for why
// the stored instant is the run's start rather than its finish.
//
// It takes no Scope, and that is a deliberate placement rather than an
// oversight: this is not a user-facing read. It reads ONE named instance's
// control-plane column so the replication worker can decide whether to bootstrap
// it, exactly as CountEncryptedCredentials reads a control-plane count for the
// key-rotation path. The rule store.go states is about reads that AGGREGATE
// ACROSS INSTANCES, which is where an existence oracle lives; the Services
// screen's read of this same column is a different function and takes a Scope.
func (s *Store) LastFullSyncAt(ctx context.Context, instanceID int64) (sql.NullString, error) {
	var at sql.NullString
	err := s.db.Read().QueryRowContext(ctx,
		`SELECT last_full_sync_at FROM service_instance WHERE id = ? AND deleted_at IS NULL`,
		instanceID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return at, fmt.Errorf("service_instance %d not found", instanceID)
	}
	if err != nil {
		return at, fmt.Errorf("read last_full_sync_at for service_instance %d: %w", instanceID, err)
	}
	return at, nil
}

// WorkCountsByInstance reports, per service instance, HOW MANY DISTINCT WORKS
// THAT INSTANCE CONTRIBUTES to the local catalogue.
//
// # What it counts, precisely, because the name has to be defensible
//
// It counts `work` rows, DISTINCT, reachable from a live `service_item_link` on
// the instance. Three narrowings are load-bearing and each rules out a number
// that would be a different claim:
//
//   - DISTINCT work_id, not a link count. §6.4 tier 1 merges two remote items
//     that share a strong external id onto ONE work, so counting links would
//     report a library as larger than it is — and the same instance can hold
//     two `service_item_link` rows pointing at one work.
//   - `l.deleted_at IS NULL`. A link inside its 7-day tombstone window is an
//     item the upstream no longer reports.
//   - `w.deleted_at IS NULL`. Same window, on the work.
//
// It is NOT a media-file count and NOT an edition count: nothing on the
// Services screen claims to know how many files an upstream holds, and this
// number must not be read as one. It is also not a per-LIBRARY number — a
// user-defined library binds containers across instances (§17.8), so a count
// per library is a different query with a different reader.
//
// Person works are outside it by construction rather than by a filter: the
// credit pass creates a 'person' work with no `service_item_link` at all, so no
// instance contributes one.
//
// # Zero versus absent
//
// AN INSTANCE WITH NO ROWS IS ABSENT FROM THE MAP, not present with 0, and the
// caller decides what that means for it. `0` from this function would be the
// answer for an instance that has never synced, for one that synced an empty
// library, and for a Prowlarr that has no catalogue at all — three different
// facts. The one that separates the first two is `last_full_sync_at`, which is
// on the instance row and not here.
//
// # Scope
//
// It AGGREGATES ACROSS INSTANCES, which is exactly the read store.go's rule
// says must carry one: without the predicate, a caller who can see one instance
// would learn how many works every other instance holds, which is an existence
// oracle over the whole configured stack.
func (s *Store) WorkCountsByInstance(ctx context.Context, scope Scope) (map[int64]int64, error) {
	pred, args := scope.instancePredicate("l.service_instance_id")
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT l.service_instance_id, COUNT(DISTINCT l.work_id)
		  FROM service_item_link l
		  JOIN work w ON w.id = l.work_id
		 WHERE l.deleted_at IS NULL AND w.deleted_at IS NULL AND `+pred+`
		 GROUP BY l.service_instance_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("count works per service_instance: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]int64)
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("count works per service_instance: scan: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count works per service_instance: %w", err)
	}
	return out, nil
}

// FileWalkFailuresByInstance counts the items each instance's file walk could
// not read, per instance.
//
// ⚠️ IT IS sync_report's FIRST READER, and that is the point of it existing.
// Before this, `file_walk_failed` rows were written and nothing anywhere read
// them, which surfaces exactly as much as the bare `continue` it replaced. The
// Services screen is where §17.3 says a broken pipeline has to be visible.
//
// # The window: since the last COMPLETED full sync
//
// sync_report is append-only, so a bare count is every failure the instance has
// ever had — including ones a later successful import already fixed. The window
// is `created_at >= service_instance.last_full_sync_at`, which is exactly the
// last completed run plus anything a partial run has added since:
// last_full_sync_at is stamped with the run's START time (libsync's FullImport),
// and both columns are stored through FormatTime, whose layout is ordered
// lexicographically on purpose (store.go's timeLayout comment).
//
// AN INSTANCE THAT HAS NEVER COMPLETED A RUN counts everything it has, because
// there is no earlier completed run to exclude and the rows it does have came
// from runs that really happened. COUNT(DISTINCT remote_id) is what keeps three
// failed partial runs over one series reporting 1 rather than 3.
//
// # Zero versus absent
//
// An instance with no rows is ABSENT from the map, on WorkCountsByInstance's
// terms — but unlike that function, absent and 0 mean the same thing here and
// the caller may collapse them: "no item failed" is the only reading of no rows,
// and there is no second fact that could distinguish it.
//
// It AGGREGATES ACROSS INSTANCES, so it carries a Scope for the reason
// WorkCountsByInstance states: without the predicate it is an existence oracle
// over the whole configured stack.
func (s *Store) FileWalkFailuresByInstance(ctx context.Context, scope Scope) (map[int64]int64, error) {
	pred, args := scope.instancePredicate("r.service_instance_id")
	args = append([]any{SyncReportFileWalkFailed}, args...)
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT r.service_instance_id, COUNT(DISTINCT r.remote_id)
		  FROM sync_report r
		  JOIN service_instance i ON i.id = r.service_instance_id
		 WHERE r.kind = ?
		   AND (i.last_full_sync_at IS NULL OR r.created_at >= i.last_full_sync_at)
		   AND `+pred+`
		 GROUP BY r.service_instance_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("count file walk failures per service_instance: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]int64)
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("count file walk failures per service_instance: scan: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count file walk failures per service_instance: %w", err)
	}
	return out, nil
}

// Analyze refreshes SQLite's planner statistics after a bulk import
// (reference/sync.md §6 rule 5: "SQLite's planner is materially better with
// stats for the multi-index intersections tag filtering depends on").
//
// It runs on the WRITER connection, not off the read pool, because ANALYZE
// takes its own lock and rule 1's table says such statements must be serialised
// with the write queue rather than raced against it.
func (s *Store) Analyze(ctx context.Context) error {
	if _, err := s.db.Writer().ExecContext(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	return nil
}

// SyncReportFileWalkFailed is sync_report.kind for one item whose file walk
// failed and was dropped.
//
// IT IS A CONSTANT BECAUSE IT HAS TWO SIDES. internal/libsync writes it and
// FileWalkFailuresByInstance reads it back, in different packages; sync_report.kind
// carries no CHECK (migration 00005 says why), so a typo on either side is not a
// constraint violation, it is a count that silently reads zero.
const SyncReportFileWalkFailed = "file_walk_failed"

// RecordSyncReport appends one operational note about a sync.
//
// detail is JSON and REACHES THIS COLUMN FROM UPSTREAM TEXT in some call paths,
// so callers redact before calling; nothing here can un-see a credential.
//
// It is a replication write and takes no Scope.
func (s *Store) RecordSyncReport(
	ctx context.Context, instanceID int64, kind, remoteKind, remoteID, detail string,
) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return recordSyncReport(ctx, tx, instanceID, kind, remoteKind, remoteID, detail)
	})
}

// recordSyncReport is the same append, inside a transaction the caller already
// holds. It exists so a fact discovered mid-transaction — a container skipped
// after a ROLLBACK TO SAVEPOINT — is committed atomically with the state that
// made it true, rather than in a second write that a crash can lose.
func recordSyncReport(
	ctx context.Context, tx *sql.Tx, instanceID int64, kind, remoteKind, remoteID, detail string,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sync_report (service_instance_id, kind, remote_kind, remote_id, detail)
		VALUES (?,?,?,?,?)`,
		instanceID, kind, nullString(remoteKind), nullString(remoteID), nullString(detail))
	if err != nil {
		return fmt.Errorf("record sync_report %q for service_instance %d: %w", kind, instanceID, err)
	}
	return nil
}
