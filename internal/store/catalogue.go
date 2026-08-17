package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
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
// It is a replication write and takes no Scope. See the file header.
func (s *Store) BindContainers(
	ctx context.Context, instanceID, userID int64, cs []CatalogueContainer,
) (map[string]CatalogueBinding, error) {
	out := make(map[string]CatalogueBinding, len(cs))
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, c := range cs {
			if c.Kind == "" {
				continue
			}
			b, err := bindOneContainer(ctx, tx, instanceID, userID, c)
			if err != nil {
				return fmt.Errorf("bind container %q (%s) on service_instance %d: %w",
					c.Name, c.RemoteID, instanceID, err)
			}
			out[c.RemoteID] = b
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func bindOneContainer(
	ctx context.Context, tx *sql.Tx, instanceID, userID int64, c CatalogueContainer,
) (CatalogueBinding, error) {
	// Step 1: is this exact container already bound? If so nothing about the
	// library changes — a rename upstream must not re-propose or re-slug it.
	var libID int64
	var kind string
	err := tx.QueryRowContext(ctx, `
		SELECT l.id, l.kind
		  FROM library_source ls
		  JOIN library l ON l.id = ls.library_id
		 WHERE ls.service_instance_id = ?
		   AND ls.container_kind = 'remote_library'
		   AND ls.container_ref = ?`, instanceID, c.RemoteID).Scan(&libID, &kind)
	switch {
	case err == nil:
		// The container is back: clear any missing_since the sweep set.
		if _, err := tx.ExecContext(ctx, `
			UPDATE library_source SET missing_since = NULL, container_identity = ?
			 WHERE service_instance_id = ? AND container_kind = 'remote_library' AND container_ref = ?`,
			c.Name, instanceID, c.RemoteID); err != nil {
			return CatalogueBinding{}, fmt.Errorf("refresh source: %w", err)
		}
		return CatalogueBinding{LibraryID: libID, Kind: kind}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return CatalogueBinding{}, fmt.Errorf("look up existing source: %w", err)
	}

	// Step 2: join a library of the same name AND the same kind, if one exists.
	// The join key is §17.8's: case-insensitive, whitespace-trimmed, per user.
	// It is evaluated in Go over this user's libraries rather than in SQL,
	// because ux_library_name is a plain UNIQUE on the raw name and there is no
	// index over a lowered, trimmed form to seek — a homelab has single-digit
	// libraries, so the scan is cheaper than the index would be.
	existing, err := userLibrariesByNameKey(ctx, tx, userID)
	if err != nil {
		return CatalogueBinding{}, err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = "Library " + c.RemoteID
	}
	if got, ok := existing[libraryNameKey(name)]; ok && got.kind == c.Kind {
		if err := insertLibrarySource(ctx, tx, got.id, instanceID, c); err != nil {
			return CatalogueBinding{}, err
		}
		return CatalogueBinding{LibraryID: got.id, Kind: got.kind}, nil
	}

	// Step 3: create. A name taken by a library of a DIFFERENT kind cannot be
	// joined and must not collide with ux_library_name, so it is disambiguated
	// deterministically rather than by a random suffix.
	base := name
	for n := 2; ; n++ {
		if _, taken := existing[libraryNameKey(name)]; !taken {
			break
		}
		name = fmt.Sprintf("%s (%d)", base, n)
	}

	slug := slugify(name)
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
	return CatalogueBinding{LibraryID: id, Kind: c.Kind, Created: true}, nil
}

type libraryRow struct {
	id   int64
	kind string
}

func userLibrariesByNameKey(ctx context.Context, q querier, userID int64) (map[string]libraryRow, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, name, kind FROM library WHERE user_id = ? AND id <> ?`, userID, UnfiledLibraryID)
	if err != nil {
		return nil, fmt.Errorf("list libraries for user %d: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]libraryRow{}
	for rows.Next() {
		var r libraryRow
		var name string
		if err := rows.Scan(&r.id, &name, &r.kind); err != nil {
			return nil, fmt.Errorf("list libraries for user %d: scan: %w", userID, err)
		}
		out[libraryNameKey(name)] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list libraries for user %d: %w", userID, err)
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
// Both are asserted in CI by TestSearchDocInvariantsAfterImport and
// TestSearchDocInvariantQueriesCatchABreak.
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
		for _, it := range items {
			b, ok := bindings[it.ContainerID]
			if !ok {
				// A container with no binding is a declined one. Skipping here
				// rather than erroring keeps a declined Kavita library from
				// failing the whole import.
				continue
			}
			one, err := applyOneItem(ctx, tx, instanceID, b, it, now)
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
//nolint:gocyclo,maintidx // see above: the order is the algorithm
func applyOneItem(
	ctx context.Context, tx *sql.Tx, instanceID int64,
	b CatalogueBinding, it CatalogueItem, now time.Time,
) (BatchResult, error) {
	var res BatchResult
	nowStr := FormatTime(now)

	// ── 1. An existing link pins the work. The upstream id is authoritative for
	// which work this is, so identity resolution does not re-run for it.
	var workID int64
	err := tx.QueryRowContext(ctx, `
		SELECT work_id FROM service_item_link
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?`,
		instanceID, it.RemoteKind, it.RemoteID).Scan(&workID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return res, fmt.Errorf("look up link: %w", err)
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
				return res, fmt.Errorf("tier-1 identity lookup on %s=%s: %w", x.Source, x.Value, err)
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
			INSERT INTO work (kind, title, sort_title, normalized_title, norm_version,
			                  original_title, overview, added_at, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			it.Kind, it.Title, it.SortTitle, it.NormalizedTitle, it.NormVersion,
			nullString(it.OriginalTitle), nullString(it.Overview),
			nullTime(it.AddedAt), nowStr, nowStr)
		if err != nil {
			return res, fmt.Errorf("insert work: %w", err)
		}
		if workID, err = r.LastInsertId(); err != nil {
			return res, fmt.Errorf("insert work: id: %w", err)
		}
		res.WorksCreated++
	default:
		if _, err := tx.ExecContext(ctx, `
			UPDATE work SET title = ?, sort_title = ?, normalized_title = ?, norm_version = ?,
			                original_title = ?, overview = ?, updated_at = ?, deleted_at = NULL
			 WHERE id = ?`,
			it.Title, it.SortTitle, it.NormalizedTitle, it.NormVersion,
			nullString(it.OriginalTitle), nullString(it.Overview), nowStr, workID); err != nil {
			return res, fmt.Errorf("update work %d: %w", workID, err)
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
			return res, fmt.Errorf("upsert work_book %d: %w", workID, err)
		}
	case "comic":
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_comic (work_id, reading_direction) VALUES (?,?)
			ON CONFLICT (work_id) DO UPDATE SET reading_direction = excluded.reading_direction`,
			workID, it.ReadingDirection); err != nil {
			return res, fmt.Errorf("upsert work_comic %d: %w", workID, err)
		}
	default:
		return res, fmt.Errorf("no subtype table for work.kind %q", it.Kind)
	}

	// ── 5. Alternate titles. Replaced wholesale for this work: an upstream that
	// drops a localised name must not leave the old one searchable forever.
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_alt_title WHERE work_id = ?`, workID); err != nil {
		return res, fmt.Errorf("clear alt titles for work %d: %w", workID, err)
	}
	for _, a := range it.AltTitles {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_alt_title (work_id, title, normalized, kind, language)
			VALUES (?,?,?,?,?)`,
			workID, a.Title, a.Normalized, a.Kind, nullString(a.Language)); err != nil {
			return res, fmt.Errorf("insert alt title %q for work %d: %w", a.Title, workID, err)
		}
	}

	// ── 6. External ids, each behind its own SAVEPOINT. See the doc comment.
	for i, x := range it.ExternalIDs {
		sp := fmt.Sprintf("extid_%d", i)
		if _, err := tx.ExecContext(ctx, "SAVEPOINT "+sp); err != nil {
			return res, fmt.Errorf("savepoint %s: %w", sp, err)
		}
		_, insErr := tx.ExecContext(ctx, `
			INSERT INTO external_id (work_id, edition_id, source, value, confidence)
			VALUES (?, NULL, ?, ?, ?)
			ON CONFLICT (source, value, COALESCE(work_id, -1), COALESCE(edition_id, -1))
			DO UPDATE SET confidence = excluded.confidence`,
			workID, x.Source, x.Value, x.Confidence)
		if insErr == nil {
			if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
				return res, fmt.Errorf("release savepoint %s: %w", sp, err)
			}
			res.ExternalIDsWritten++
			continue
		}
		// The conflict is the merge signal. Undo just this row and carry on.
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sp); err != nil {
			return res, fmt.Errorf("rollback to savepoint %s after %w: %w", sp, insErr, err)
		}
		if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
			return res, fmt.Errorf("release savepoint %s: %w", sp, err)
		}
		var owner int64
		if err := tx.QueryRowContext(ctx, `
			SELECT work_id FROM external_id
			 WHERE source = ? AND value = ? AND work_id IS NOT NULL AND confidence >= 1.0`,
			x.Source, x.Value).Scan(&owner); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return res, fmt.Errorf("read conflicting external_id %s=%s: %w", x.Source, x.Value, err)
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
		return res, fmt.Errorf("upsert link: %w", err)
	}
	// remote_identity_hash is written AT FIRST SIGHT and never overwritten
	// (sync.md §4 guard 1): it is the O(1) comparison that tells an id reused
	// upstream from the same item coming back. The ON CONFLICT list above
	// deliberately omits it.

	// ── 8. Membership. Rewritten for this (library, work) pair, because
	// sort_title leads the primary key and a retitle would otherwise leave a
	// second member row under the old key.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM library_member WHERE library_id = ? AND work_id = ?`,
		b.LibraryID, workID); err != nil {
		return res, fmt.Errorf("clear membership of work %d in library %d: %w", workID, b.LibraryID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO library_member (library_id, sort_title, work_id, edition_id, added_at)
		VALUES (?,?,?,0,?)`, b.LibraryID, it.SortTitle, workID, nowStr); err != nil {
		return res, fmt.Errorf("file work %d into library %d: %w", workID, b.LibraryID, err)
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
		return res, err
	}
	d.people = people
	if err := writeSearchDoc(ctx, tx, workID, d); err != nil {
		return res, err
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
		return res, fmt.Errorf("read identity state of work %d: %w", workID, err)
	}
	if identified == 0 {
		res.Unidentified++
	}
	return res, nil
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
// The two rules point in opposite directions and both hold. schema.md §6.1 and
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
// It is a WRITER-SIDE refusal rather than only a CI query, because the CI query
// (TestPeopleNeverEnterTheSearchCorpus) can only report a corpus that has
// already been corrupted, and the FTS tables carry no foreign key, so a bad doc
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

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return FormatTime(t)
}

// StampFullSync records that a full import COMPLETED.
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

// LastFullSyncAt reports when one instance last COMPLETED a full import, or an
// invalid NullString for never.
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
		_, err := tx.ExecContext(ctx, `
			INSERT INTO sync_report (service_instance_id, kind, remote_kind, remote_id, detail)
			VALUES (?,?,?,?,?)`,
			instanceID, kind, nullString(remoteKind), nullString(remoteID), nullString(detail))
		if err != nil {
			return fmt.Errorf("record sync_report %q for service_instance %d: %w", kind, instanceID, err)
		}
		return nil
	})
}
