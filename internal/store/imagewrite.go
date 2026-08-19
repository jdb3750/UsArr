package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// The WRITE half of §4.4's image pipeline, and the discharge of the two
// obligations imageassets.go restates but cannot carry.
//
// imageassets.go's header names them, and this file is the writer they were
// waiting for:
//
//   - every `image_asset` writer must reference ValidImageFormat (images.go,
//     migration 00008, ADR-0050). It is CALLED here, on the value actually
//     stored, not merely mentioned — TestImageWritesValidateTheFormatVocabulary
//     can only see the reference, so the reference alone would satisfy a test
//     and not the rule.
//   - no row may be written whose `source_url` still carries a credential
//     parameter (security.md §5, "URLs stored in the database"). That is
//     ErrCredentialInSourceURL below, and internal/ssrf's credentialParams is
//     the list — consulted through ssrf.IsCredentialParam, never copied.

// ErrCredentialInSourceURL refuses a row whose `source_url` carries a
// credential.
//
// ⚠️ IT REJECTS THE WRITE; IT DOES NOT STRIP AND CONTINUE. security.md §5 asks
// for an assertion and not a sanitiser, and the difference is what happens to
// the bug that produced the URL. Stripping silently would store a correct row
// and leave the caller still constructing credentialed URLs — into log lines,
// into an HTTP cache key, into the next table that has no such check. A refusal
// surfaces it once, at the moment it is introduced.
//
// It also protects the CACHE KEY. `cache_key = sha256(source_url)[:16]`, so a
// credential inside the hashed string means rotating that credential silently
// invalidates the whole image cache (security.md §5 names this consequence
// directly).
var ErrCredentialInSourceURL = errors.New("store: image_asset.source_url carries a credential")

// ErrInvalidImageAsset is a malformed row refused before it reaches SQL —
// an unknown format token, a misshapen cache key, a missing width.
var ErrInvalidImageAsset = errors.New("store: invalid image_asset row")

// ErrNoSuchWork is returned when the work an asset was fetched for is not there
// to point at it — deleted, or never imported.
//
// The asset row is NOT written in that case: the transaction rolls back whole.
// An `image_asset` no work points at is visible to nobody (imageassets.go's
// access scope is an EXISTS over `work`), so committing one would leave bytes on
// disk and a row in the table that no request can ever reach.
var ErrNoSuchWork = errors.New("store: no such work")

// Image roles — `image_asset.role`'s vocabulary, of which the pipeline writes
// one. The column carries no CHECK, for migration 00008's reasons.
const ImageRolePoster = "poster"

// Origin classes — `image_asset.origin_class`, which DOES carry a CHECK in
// migration 00005. It is what lets the fetcher pick an SSRF policy from the ROW
// rather than by pattern-matching the URL string.
const (
	// ImageOriginConfigured is a URL built against a base URL an admin typed
	// into the service form. A cover fetched from BookOrbit is this class: the
	// path is constructed from a book id UsArr already holds, so nothing about
	// the URL came out of an upstream response body.
	ImageOriginConfigured = "configured"

	// ImageOriginDerived is a URL read OUT of an upstream response body.
	ImageOriginDerived = "derived"

	// ImageOriginProvider is a URL built from a public provider template.
	ImageOriginProvider = "provider"
)

var imageOriginClasses = map[string]bool{
	ImageOriginConfigured: true,
	ImageOriginDerived:    true,
	ImageOriginProvider:   true,
}

// PosterAsset is one rendered cover, ready to be recorded.
//
// It carries the SOURCE's dimensions rather than any rendition's: `image_asset`
// has one width/height pair for a row that has up to seven renditions, and the
// only one of them that is not a function of the allowlist is the original.
type PosterAsset struct {
	// WorkID is the work whose poster this is. The write fails with
	// ErrNoSuchWork rather than orphaning the asset.
	WorkID int64

	// SourceURL is the credential-stripped URL the bytes came from. UNIQUE in
	// the schema, and the input the cache key is derived from.
	SourceURL string

	// CacheKey is `sha256(SourceURL)[:16]`. It is passed in rather than derived
	// here because imagecache owns the derivation and the on-disk bytes have
	// already been written under it — recomputing it in this package would be
	// the second copy imagecache's package doc warns about.
	CacheKey string

	// Format is the codec token the renditions were encoded in. Checked against
	// ValidImageFormat; NULL is not expressible here because this constructor is
	// only ever called once bytes exist.
	Format string

	// OriginClass is `image_asset.origin_class` — the SSRF policy input.
	OriginClass string

	// ServiceInstanceID is where the bytes came FROM. It is provenance and an
	// SSRF-policy input, NEVER a permission: imageassets.go's read authorizes
	// against the owning work, not against this column.
	ServiceInstanceID int64

	// Width and Height are the SOURCE image's pixel dimensions.
	Width, Height int

	// FetchedAt is when the bytes were retrieved.
	FetchedAt time.Time
}

// validate is every refusal this write makes, gathered so the SQL below can
// assume a well-formed row.
func (p PosterAsset) validate() error {
	if p.WorkID <= 0 {
		return fmt.Errorf("%w: work id %d", ErrInvalidImageAsset, p.WorkID)
	}
	// THE CREDENTIAL ASSERTION. It runs before the format check and before
	// anything touches the writer, so a credentialed URL never reaches a
	// prepared statement's arguments — where it would be one slog line away
	// from the log.
	if err := checkImageSourceURL(p.SourceURL); err != nil {
		return err
	}
	if !ValidImageCacheKey(p.CacheKey) {
		// The key is NOT echoed: on the serving side it is caller-supplied text,
		// and an error message is the wrong place to learn that habit.
		return fmt.Errorf("%w: cache key is not sixteen lowercase hex characters", ErrInvalidImageAsset)
	}
	// ⚠️ THE CALL migration 00008 AND ADR-0050 ARE OWED. image_asset.format
	// carries no CHECK constraint because the codec vocabulary is expected to
	// grow (AVIF is deferred, not rejected), so Go is the only place it is
	// enforced. ADR-0039 made this exact trade for write_queue.state, promised
	// the Go validator and never wrote it; this is the line that keeps 00008
	// from repeating it.
	if !ValidImageFormat(p.Format) {
		return fmt.Errorf("%w: %q is not an image_asset.format this build writes", ErrInvalidImageAsset, p.Format)
	}
	if !imageOriginClasses[p.OriginClass] {
		// origin_class DOES carry a CHECK (migration 00005), so a bad value
		// would be caught by SQLite too — but as a constraint violation naming
		// a table, not as a sentence naming the policy input that was wrong.
		return fmt.Errorf("%w: %q is not an origin_class", ErrInvalidImageAsset, p.OriginClass)
	}
	if p.Width <= 0 || p.Height <= 0 {
		return fmt.Errorf("%w: source dimensions %dx%d", ErrInvalidImageAsset, p.Width, p.Height)
	}
	return nil
}

// checkImageSourceURL is the credential assertion security.md §5 requires of the
// ingest path.
//
// It consults ssrf.IsCredentialParam — the ONE deny-list — rather than naming
// parameters here. A shorter list restated in this package would be the second
// deny-list, and the one that drifts is the one that leaks.
//
// ⚠️ ITS LIMIT, so nobody reads it as more than it is: it refuses a credential
// in a QUERY PARAMETER or in userinfo, which is what §5's bullet states. A
// credential smuggled into a PATH SEGMENT is not refused — internal/ssrf's
// path-segment rule is an explicitly lossy heuristic, biased towards missing a
// passkey rather than eating a real path, and a heuristic that fires wrongly
// here would permanently refuse a legitimate cover rather than mangle a log line.
func checkImageSourceURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("%w: source_url is empty", ErrInvalidImageAsset)
	}
	u, err := url.Parse(raw)
	if err != nil {
		// "It did not parse" is not evidence that it holds no secret. The URL
		// is not quoted back for the same reason.
		return fmt.Errorf("%w: source_url does not parse", ErrCredentialInSourceURL)
	}
	if u.User != nil {
		return fmt.Errorf("%w: source_url carries userinfo", ErrCredentialInSourceURL)
	}
	for name := range u.Query() {
		if ssrf.IsCredentialParam(name) {
			// The NAME is a member of a public deny-list and is safe to report;
			// it is the half that makes the bug findable. The VALUE never is.
			return fmt.Errorf("%w: query parameter %q", ErrCredentialInSourceURL, name)
		}
	}
	return nil
}

// PutPosterAsset records one rendered cover and points its work at it, in ONE
// BEGIN IMMEDIATE transaction.
//
// # Why both halves are one transaction
//
// The bytes are already on disk when this runs, and disk and database are not
// transactional together — but the two ROWS are, and they are the pair that
// matters. An `image_asset` with no `work` pointing at it is visible to nobody
// (imageassets.go authorizes through an EXISTS over `work`), so a commit that
// wrote the asset and failed the link would leave an unreachable row that the
// UNIQUE on `source_url` then blocks the retry from replacing. Committing them
// together makes a failed run leave the database exactly as it found it, and
// leaves only orphaned bytes on disk — which the next successful run overwrites
// at the same path, because the path is a function of the same URL.
//
// # Why it is an upsert
//
// `source_url` is UNIQUE, and a re-import re-fetches the same cover. ON CONFLICT
// updates in place so the row keeps its id — which `work.poster_asset_id`
// already references from any other work sharing the artwork — instead of
// deleting and reinserting, which would cascade SET NULL through those
// references and blank the posters of works this call never touched.
func (s *Store) PutPosterAsset(ctx context.Context, p PosterAsset) (int64, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}

	fetched := FormatTime(p.FetchedAt)

	var assetID int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// The work first, and NOT as an optimisation. It decides whether this
		// transaction should exist at all, and doing it first means the failure
		// costs no INSERT and no rollback of one.
		var exists int
		err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM work WHERE id = ? AND deleted_at IS NULL`, p.WorkID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: work %d", ErrNoSuchWork, p.WorkID)
		}
		if err != nil {
			return fmt.Errorf("read work %d: %w", p.WorkID, err)
		}

		// state is 'ready' and format is non-NULL together, and that pairing is
		// migration 00008's stated invariant: NULL format is co-extensive with
		// state <> 'ready'. This writer only runs once bytes exist, so it is the
		// one place both are set at once.
		err = tx.QueryRowContext(ctx, `
			INSERT INTO image_asset
			       (source_url, origin_class, origin_service_instance_id, role,
			        width, height, cache_key, format, fetched_at, state)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'ready')
			ON CONFLICT(source_url) DO UPDATE SET
			       origin_class = excluded.origin_class,
			       origin_service_instance_id = excluded.origin_service_instance_id,
			       role       = excluded.role,
			       width      = excluded.width,
			       height     = excluded.height,
			       cache_key  = excluded.cache_key,
			       format     = excluded.format,
			       fetched_at = excluded.fetched_at,
			       state      = 'ready'
			RETURNING id`,
			p.SourceURL, p.OriginClass, nullableInstanceID(p.ServiceInstanceID), ImageRolePoster,
			p.Width, p.Height, p.CacheKey, p.Format, fetched,
		).Scan(&assetID)
		if err != nil {
			// The source URL is deliberately absent from this message. It has
			// been asserted credential-free above, but an error string travels
			// further than a row does and the id is what makes the row findable.
			return fmt.Errorf("write image_asset for work %d: %w", p.WorkID, err)
		}

		// `poster_asset_id IS NOT ?` keeps a re-import that renders an identical
		// cover from reporting a change it did not make — the same guard
		// credits.go's `year IS NOT ?` carries, and for the same reason: a write
		// that touches nothing must look like one.
		if _, err := tx.ExecContext(ctx,
			`UPDATE work SET poster_asset_id = ?
			  WHERE id = ? AND (poster_asset_id IS NOT ?)`,
			assetID, p.WorkID, assetID,
		); err != nil {
			return fmt.Errorf("point work %d at image_asset %d: %w", p.WorkID, assetID, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return assetID, nil
}

// nullableInstanceID renders a zero instance id as SQL NULL.
//
// The column is `REFERENCES service_instance(id) ON DELETE SET NULL`, so 0 is
// not a legal value — it would be a foreign key to a row that does not exist,
// and with foreign keys enforced the insert would fail with a constraint error
// naming neither the column nor why. NULL is the column's own way of saying
// "this asset has no surviving origin instance", which is the state an evicted
// instance leaves behind anyway.
func nullableInstanceID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}
