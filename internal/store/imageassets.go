package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

// The read behind `GET /img/{cache_key}` — ARCHITECTURE.md §4.1 and §4.4, the
// serving half of the image pipeline.
//
// WHAT IS HERE AND WHAT IS NOT. This file resolves an asset by its route key and
// answers whether the caller may see it. It does not fetch, downscale, encode or
// write anything: nothing in this package writes `image_asset` and this file is
// not the exception. The fetch half needs catalogue rows carrying cover URLs,
// which no adapter produces yet; building a fetcher against nothing to fetch
// would produce a pipeline tested only against fixtures.
//
// TWO OBLIGATIONS TRAVEL WITH THAT WRITER RATHER THAN WITH THIS FILE, and are
// restated here so they are found from the same place the reader is:
//
//   - every `image_asset` writer must reference ValidImageFormat (images.go,
//     migration 00008, ADR-0050) or TestImageWritesValidateTheFormatVocabulary
//     fails the build;
//   - no row may be written whose `source_url` still carries a credential
//     parameter (security.md §5, the "URLs stored in the database" bullet), and
//     `internal/ssrf`'s credentialParams is the list, never a second copy.
//
// IT IS A LOCAL READ AND NOTHING ELSE (principle 1). One statement against the
// local file. The bytes it authorizes are read from disk by the caller; nothing
// on this path opens a socket.

// ImageAsset is one `image_asset` row as the serving path reads it, which is
// deliberately four columns and not the row.
//
// §4.5's "never load full item records" applies here more sharply than anywhere
// else: this read runs once per cover, and §17.2's grid paints up to sixty
// covers per screen. `source_url`, `etag`, `last_modified`, `thumbhash`,
// `dominant_color`, `width`, `height` and the two origin columns are all absent
// because serving cached bytes reads none of them — and `source_url` in
// particular is a URL on somebody else's host, which has no business travelling
// further than the fetcher that used it.
type ImageAsset struct {
	// ID is image_asset.id. It never reaches the wire — the route key is
	// CacheKey — and is carried so a log line can name the row.
	ID int64

	// CacheKey is the row's route key, echoed back from the lookup rather than
	// assumed equal to the argument, so a caller that logs it logs what the
	// database matched.
	CacheKey string

	// Format is `image_asset.format`: a CODEC TOKEN from images.go's
	// vocabulary, never a media type and never a file extension. It is what the
	// /img surface derives its Content-Type from, and it is NULLABLE — the
	// column's way of saying "this row has no encoded bytes yet", which is the
	// state every row is created in (migration 00008's header).
	Format sql.NullString

	// State is `image_asset.state`: pending|ready|failed|gone. A row that is
	// not ready has no bytes to serve, and the distinction between "not yet"
	// and "never" is the caller's to render.
	State string
}

// ImageStateReady is the one `image_asset.state` value that means the cached
// bytes exist. The other three — pending, failed, gone — are 00005's remaining
// members and each means there is nothing to serve; only this one is named here
// because it is the only one this read branches on.
const ImageStateReady = "ready"

// ErrImageAssetAmbiguous is returned when two rows share one cache_key.
//
// ⚠️ IT IS A REFUSAL AND NOT A TIE-BREAK, and that is the whole reason this
// error exists. `cache_key` is `sha256(credential-stripped source_url)[:16]`
// (migration 00005, §4.4) over a UNIQUE `source_url`, so two rows sharing a key
// means a 64-bit truncated-SHA-256 collision between two DIFFERENT source URLs.
// Picking either one would serve one item's artwork under the OTHER item's
// authorization decision — the caller asked "may I see the thing this key names"
// and there would be two things. Nothing in the schema forbids the collision
// (migration 00010 says why the index is not UNIQUE), so the read forbids the
// consequence.
var ErrImageAssetAmbiguous = errors.New("store: two image assets share one cache key")

// imageCacheKeyPattern is the shape of a route key: cache_key is documented as
// `sha256(...)[:16]`, which is sixteen lowercase hex characters.
//
// IT IS VALIDATED HERE AS WELL AS AT THE HTTP BOUNDARY, and the duplication is
// deliberate rather than sloppy. The key reaches a FILESYSTEM PATH on the
// serving side, so "is this string a cache key" is a safety question and not a
// formatting one, and a safety question answered in exactly one place is
// answered nowhere the moment a second caller appears. Validating in the store
// also keeps a caller from turning a key into an unbounded LIKE-shaped argument
// against the index this read seeks on.
var imageCacheKeyPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ValidImageCacheKey reports whether s is shaped like an `image_asset.cache_key`.
//
// It is a SHAPE check and says nothing about whether such a row exists; that is
// LookupImageAsset's answer, and it is the answer that carries the access scope.
func ValidImageCacheKey(s string) bool { return imageCacheKeyPattern.MatchString(s) }

// imageAssetSQL renders the LookupImageAsset statement and its arguments.
//
// It is a function rather than a literal inlined below so the query-plan test
// can EXPLAIN THE STATEMENT THAT SHIPS (docs/DEVELOPMENT.md §11 rule 1: a guard
// that asserts against a hand-copied lookalike is probing a proxy).
//
// THE ACCESS SCOPE IS THE OWNING ITEM'S, NOT THE ASSET'S. `image_asset` carries
// no user_id and no entitlement-bearing instance column — `origin_service_
// instance_id` names where the bytes came FROM, which is the fetcher's
// SSRF-policy input (security.md §2) and not a permission. security.md §4 says
// an authenticated /img is "authorized against the owning item", and the only
// edges from an asset to an item in this schema are `work.poster_asset_id` and
// `work.backdrop_asset_id`. So the predicate is an EXISTS over `work`, carrying
// the same service_item_link scope every other work read carries — ONE instance
// of it, over `w.id`, covering both artwork roles at once.
//
// AN ORPHANED ASSET IS VISIBLE TO NOBODY, and that is the correct direction. A
// row no work points at — one written ahead of the work that will claim it, or
// one left behind by a work that was hard-deleted — has no owning item, so there
// is no item to authorize against and the answer is not-found. Failing closed
// costs a re-fetch; failing open would serve artwork whose entitlement nothing
// can compute.
//
// THE PLAN, MEASURED on the real schema, this engine (github.com/ncruces/
// go-sqlite3, SQLite 3.53.4), no ANALYZE, with migration 00010's three indexes:
//
//	SEARCH ia USING INDEX ix_img_cache_key (cache_key=?)
//	MULTI-INDEX OR
//	  INDEX 1  SEARCH w USING INDEX ix_work_poster   (poster_asset_id=?)
//	  INDEX 2  SEARCH w USING INDEX ix_work_backdrop (backdrop_asset_id=?)
//
// and, on a partial access scope, one more seek on ix_sil_work. Without those
// indexes the same statement is `SCAN ia` plus a full walk of `work` for the OR
// — sixty of those per screenful of covers (§17.2), against §13's p50 < 3 ms.
//
// ⚠️ THE `OR` DEPENDS ON SQLite's MULTI-INDEX OR OPTIMISATION, WHICH IS WHY THE
// PLAN IS PINNED RATHER THAN ASSUMED. SQLite documents that transformation as
// conditional — it is applied "if it is advantageous", judged from statistics —
// and the fallback is a scan of `work`, which is silent and is exactly the
// degradation the indexes exist to prevent. It was measured to apply here, on
// both scope shapes, and TestLookupImageAssetPlanGuardFires drops each index in
// turn and watches the assertion catch the fallback.
//
// LIMIT 2, NOT LIMIT 1. The second row is the collision probe — see
// ErrImageAssetAmbiguous. Reading one row and trusting it would make a cache_key
// collision serve the wrong item's artwork silently, which is the one outcome
// this read must not have.
func imageAssetSQL(scope Scope) (string, []any) {
	scopePred, scopeArgs := scope.workVisibilityPredicate("w.id")

	args := make([]any, 0, 1+len(scopeArgs))
	args = append(args, "") // the cache key; filled by the caller
	args = append(args, scopeArgs...)

	return `
		SELECT ia.id, ia.cache_key, ia.format, ia.state
		  FROM image_asset ia
		 WHERE ia.cache_key = ?
		   AND EXISTS (SELECT 1 FROM work w
		                WHERE (w.poster_asset_id = ia.id OR w.backdrop_asset_id = ia.id)
		                  AND w.deleted_at IS NULL
		                  AND ` + scopePred + `)
		 LIMIT 2`, args
}

// LookupImageAsset resolves a route key to the asset it names, within the
// caller's access scope.
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL, both halves. A scope a query accepts
// and never filters on is indistinguishable from no scope at all, which is why
// TestLookupImageAssetScopeActuallyFilters breaks the predicate and watches the
// assertion catch it rather than trusting that the parameter is threaded.
//
// THE THREE ANSWERS ARE DELIBERATELY TWO. `ok == false` covers both "no such
// key" and "not yours", and the caller renders one not-found for them — because
// a distinguishable "exists but not yours" is the existence oracle security.md
// §4 is written against, and the ID space is enumerable by design (§4.5 ships
// work ids to every client). An unreadable database is the third answer and is
// an error.
func (s *Store) LookupImageAsset(
	ctx context.Context, scope Scope, cacheKey string,
) (ImageAsset, bool, error) {
	// Refused before it reaches SQL. A malformed key can match no row — the
	// column only ever holds hex — so this is not a correctness gate; it is the
	// gate that keeps a caller-controlled string from reaching a query and,
	// through the caller, a filesystem path, on a route whose whole input is
	// one path segment.
	if !ValidImageCacheKey(cacheKey) {
		return ImageAsset{}, false, nil
	}

	query, args := imageAssetSQL(scope)
	args[0] = cacheKey

	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return ImageAsset{}, false, fmt.Errorf("read image asset: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var found []ImageAsset
	for rows.Next() {
		var a ImageAsset
		if err := rows.Scan(&a.ID, &a.CacheKey, &a.Format, &a.State); err != nil {
			return ImageAsset{}, false, fmt.Errorf("read image asset: scan: %w", err)
		}
		found = append(found, a)
	}
	if err := rows.Err(); err != nil {
		return ImageAsset{}, false, fmt.Errorf("read image asset: %w", err)
	}

	switch len(found) {
	case 0:
		return ImageAsset{}, false, nil
	case 1:
		return found[0], true, nil
	default:
		// The ids and not the key: the key is caller-supplied text and the
		// caller renders the failure, so it is the two ROWS that make this
		// findable — `SELECT source_url FROM image_asset WHERE id IN (…)`.
		return ImageAsset{}, false, fmt.Errorf("%w: image_asset %d and %d",
			ErrImageAssetAmbiguous, found[0].ID, found[1].ID)
	}
}
