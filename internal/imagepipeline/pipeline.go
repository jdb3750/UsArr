// Package imagepipeline is the WRITE half of ARCHITECTURE.md §4.4's image
// pipeline: the missing middle between an upstream cover and a rendered poster.
//
// It fetches one cover, decodes it, renders every width on §4.4's allowlist,
// writes the bytes through internal/imagecache and records the row through
// internal/store — after which `GET /img/{key}` has something to serve and
// PosterKeyExpr has a key to ship.
//
// ⚠️ STATUS, STATED HERE BECAUSE A GREEN TEST SUITE WILL OTHERWISE BE THE ONLY
// THING A READER SEES. This pipeline has been TESTED AGAINST A FAKE FETCHER AND
// NEVER AGAINST A REAL COVER. Every image it has ever processed was fabricated
// by its own tests; no byte from a running BookOrbit has been through it. That
// is the same register as deploy/Dockerfile's written-not-built, and it is not a
// hedge about quality — it is a fact about what has been executed.
//
// WHY BUILDING IT NOW IS LEGITIMATE ANYWAY: the caller is NAMED, not
// hypothetical. internal/bookorbit is adding a Cover method that this package's
// CoverFetcher is shaped to, and the batch cover pass over an import is gated
// behind the owner's first real import rather than absent from anyone's plan.
// What is missing is the wiring, not the consumer.
//
// # What is deliberately NOT here
//
// No batch pass and no import loop. Poster is a PER-WORK operation, and a batch
// is a loop over it that belongs to whoever owns the import. Building the batch
// shape first and extracting the single-item call from it afterwards is how the
// per-work trigger — which needs no gate at all — ends up unreachable.
//
// No eviction and no expiry sweep. `image_asset.expires_at` and ix_img_state
// exist for one, and it is a different job from writing a row.
package imagepipeline

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// Cover is one fetched cover: what the upstream answered, unclassified.
//
// THE STATUS IS SEPARATE FROM THE ERROR ON PURPOSE. A 404 and a 503 are both
// "no bytes", and they mean opposite things about whether to try again — so a
// fetcher that collapsed either into an error would take that decision away from
// the only code that can make it.
type Cover struct {
	// SourceURL is the credential-free URL the bytes were fetched from.
	//
	// IT TRAVELS WITH THE RESPONSE RATHER THAN BEING RECONSTRUCTED BY THIS
	// PACKAGE, and that is the whole reason it is a field. The URL is
	// `image_asset.source_url` and the input to the cache key, so a second
	// construction of it here — base URL plus a path this package guessed —
	// would be free to drift from the one actually fetched, and the symptom
	// would be a cache that never hits rather than an error.
	SourceURL string

	// Status is the HTTP status the upstream answered. 0 means no status
	// existed, which is a transport failure and travels as an error instead.
	Status int

	// ContentType is the upstream's declared media type.
	//
	// ⚠️ IT IS A HINT AND IS NEVER TRUSTED FOR THE STORED FORMAT. Migration
	// 00008's header is explicit that `image_asset.format` records what UsArr's
	// own encoder produced, not what the origin called it. This package carries
	// the value so a diagnostic can report a disagreement; the decoder decides.
	ContentType string

	// Body is the response body.
	Body []byte
}

// CoverFetcher is the one thing this package needs from a catalogue client.
//
// IT IS DEFINED HERE, WHERE IT IS CONSUMED, AND NOT WHERE IT IS IMPLEMENTED.
// That is the Go idiom and it has a concrete payoff on this tree: the pipeline
// is testable against a fabricated image with no HTTP round trip, and adding it
// costs the client package nothing — no import of this one, no interface to
// satisfy, not a line changed.
//
// SSRF IS THE IMPLEMENTATION'S, NOT THIS PACKAGE'S, and that is deliberate
// rather than an omission. A BookOrbit cover URL is security.md §2's CLASS 1 —
// an admin-configured service URL, constructed from a book id UsArr already
// holds, never harvested from an upstream response body — so it is reached
// through the same resolve-then-pin *http.Client every other call to that
// instance goes through. Building a second egress path here would be a second
// mechanism to keep in step with the policy, and the one that drifts is the one
// that gets used.
type CoverFetcher interface {
	// Cover fetches one book's cover. err is non-nil only for a failure that
	// happened before a status existed.
	Cover(ctx context.Context, bookID int64) (Cover, error)
}

// PosterWriter is the slice of *store.Store this package uses. Same reason as
// CoverFetcher: one method, defined at the point of use, so the orchestration is
// testable without a database and the database write is testable without a
// fetcher.
type PosterWriter interface {
	PutPosterAsset(ctx context.Context, p store.PosterAsset) (int64, error)
}

// The three refusals a caller must be able to tell apart.
var (
	// ErrNoCover is a 404: this book has no cover. TERMINAL FOR THIS RUN — a
	// retry fetches the same 404 and costs a round trip to learn it.
	ErrNoCover = errors.New("imagepipeline: upstream has no cover for this book")

	// ErrUpstreamUnavailable is any other non-2xx. TRANSIENT: a 503 during a
	// restart, a 502 from a proxy, a 429. Worth trying again later.
	ErrUpstreamUnavailable = errors.New("imagepipeline: upstream did not answer with a cover")

	// ErrNoBody is a 2xx that carried nothing. Treated as transient — an empty
	// 200 is a proxy or a truncation far more often than it is a considered
	// answer.
	ErrNoBody = errors.New("imagepipeline: upstream answered with no bytes")
)

// ⚠️ WHICH OF 404 AND 200-WITH-A-PLACEHOLDER BOOKORBIT ACTUALLY SENDS FOR A BOOK
// WITH NO COVER IS UNMEASURED HERE. REVIEW-LOG LS-260 asks exactly this question;
// it was framed against Kavita, which is sunset, and there is no live BookOrbit
// in this environment to ask. NOTHING IN THIS FILE ASSERTS AN ANSWER — both
// paths are handled, and the classification above is the policy for whichever
// one turns out to be true, not a claim about which one it is. If the answer
// arrives, it belongs in a comment WITH ITS CITATION; a comment asserting
// unobserved behaviour is a source the next reader copies.

// IsRetryable reports whether a Poster failure is worth attempting again.
//
// It exists so a caller does not have to know which sentinels are terminal —
// that knowledge belongs with the classification, and a caller that guessed
// would either hammer a 404 forever or give up on a restart.
func IsRetryable(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrUpstreamUnavailable), errors.Is(err, ErrNoBody):
		return true
	// A cover that does not exist, does not decode, or is a decompression bomb
	// answers identically every time. Retrying is a round trip spent to learn
	// the same thing.
	case errors.Is(err, ErrNoCover),
		errors.Is(err, ErrUnsupportedFormat),
		errors.Is(err, ErrImageTooLarge),
		errors.Is(err, imagecache.ErrCredentialInURL),
		errors.Is(err, store.ErrCredentialInSourceURL),
		errors.Is(err, store.ErrInvalidImageAsset),
		errors.Is(err, store.ErrNoSuchWork):
		return false
	default:
		// An unrecognised failure — a disk error, a database error, a context
		// deadline. Retryable is the safe default: the cost of retrying a
		// permanent failure is one round trip, the cost of NOT retrying a
		// transient one is a work with no cover until someone notices.
		return true
	}
}

// Result is what one successful Poster call did.
type Result struct {
	// AssetID is the `image_asset` row.
	AssetID int64

	// CacheKey is the key `GET /img/{key}` now answers for.
	CacheKey string

	// Format is the codec token stored in `image_asset.format`.
	Format string

	// SourceWidth and SourceHeight are the ORIGINAL image's dimensions, which
	// are what the row records.
	SourceWidth, SourceHeight int

	// Renditions is how many widths were written — always the full allowlist,
	// because the no-upscale rule clamps rather than skips (see renderAll).
	Renditions int

	// CachedBytes is the total written to disk across every width. It is the
	// number a caller reports when someone asks what an import cost.
	CachedBytes int
}

// Pipeline turns one book id into a rendered, recorded poster.
type Pipeline struct {
	fetcher CoverFetcher
	cache   *imagecache.Cache
	writer  PosterWriter
	now     func() time.Time
}

// New builds a pipeline. now may be nil, in which case time.Now is used; it is a
// parameter so a test can pin `fetched_at` rather than assert around a clock.
func New(fetcher CoverFetcher, cache *imagecache.Cache, writer PosterWriter, now func() time.Time) (*Pipeline, error) {
	if fetcher == nil {
		return nil, errors.New("imagepipeline: a fetcher is required")
	}
	if cache == nil {
		return nil, errors.New("imagepipeline: a cache is required")
	}
	if writer == nil {
		return nil, errors.New("imagepipeline: a poster writer is required")
	}
	if now == nil {
		now = time.Now
	}
	return &Pipeline{fetcher: fetcher, cache: cache, writer: writer, now: now}, nil
}

// Poster fetches, renders and records the cover for ONE work.
//
// # The order of the two writes, which is not arbitrary
//
// Bytes to DISK first, row to the DATABASE second. `image_asset.state = 'ready'`
// is a claim that the bytes exist, and the two stores are not transactional
// together, so one of the two orders has to be wrong on a crash:
//
//   - disk then database — a failure leaves bytes nothing references. The next
//     run overwrites them at the same path, because the path is a function of
//     the same URL. Harmless.
//   - database then disk — a failure leaves a row saying `ready` with no bytes
//     behind it, and `/img` answers not_cached for a year while the row insists
//     otherwise. Silent, and only fixed by noticing.
//
// So: disk first, and the orphan is the failure mode we accept.
func (p *Pipeline) Poster(ctx context.Context, workID, bookID, instanceID int64) (Result, error) {
	cover, err := p.fetcher.Cover(ctx, bookID)
	if err != nil {
		// The fetcher's error is already this project's shape — redacted, and
		// naming no credential. It is wrapped rather than replaced so a caller
		// can still reach the client's own sentinels.
		return Result{}, fmt.Errorf("imagepipeline: fetch cover for book %d: %w", bookID, err)
	}

	if err := classify(cover.Status, bookID); err != nil {
		return Result{}, err
	}
	if len(cover.Body) == 0 {
		return Result{}, fmt.Errorf("%w: book %d answered %d with an empty body",
			ErrNoBody, bookID, cover.Status)
	}

	// THE CREDENTIAL GATE, and it is the first thing done with the URL. Key
	// refuses rather than strips, so there is no way past this line with a
	// credentialed URL: no key means no path, and no path means nothing is
	// written to disk or to the database. security.md §5.
	key, err := imagecache.Key(cover.SourceURL)
	if err != nil {
		// cover.SourceURL is NOT in the message. It is the string suspected of
		// carrying a secret, and this error reaches a log.
		return Result{}, fmt.Errorf("imagepipeline: cover url for book %d is not storable: %w", bookID, err)
	}

	img, err := decode(cover.Body)
	if err != nil {
		// The upstream's declared type is reported because the DISAGREEMENT is
		// the diagnostic: "it said image/avif and we have no decoder for it" is
		// actionable in a way that "undecodable" is not. It is a media type from
		// a response header, so it is bounded before it is quoted.
		return Result{}, fmt.Errorf("imagepipeline: decode cover for book %d (upstream said %q): %w",
			bookID, boundedContentType(cover.ContentType), err)
	}

	out, err := renderAll(img)
	if err != nil {
		return Result{}, fmt.Errorf("imagepipeline: render cover for book %d: %w", bookID, err)
	}

	written := 0
	for _, r := range out.Renditions {
		if err := p.cache.Put(key, r.Width, r.Bytes); err != nil {
			return Result{}, fmt.Errorf("imagepipeline: cache cover for book %d at w=%s: %w",
				bookID, r.Width, err)
		}
		written += len(r.Bytes)
	}

	assetID, err := p.writer.PutPosterAsset(ctx, store.PosterAsset{
		WorkID:    workID,
		SourceURL: cover.SourceURL,
		CacheKey:  key,
		Format:    out.Format,
		// ClassConfigured: the URL was built against a base URL an admin typed
		// into the service form, from a book id UsArr already held. Nothing
		// about it was read out of an upstream response body, so it is NOT the
		// derived class — security.md §2's classes are picked from where the URL
		// CAME FROM, never from what it looks like.
		OriginClass:       store.ImageOriginConfigured,
		ServiceInstanceID: instanceID,
		Width:             out.Width,
		Height:            out.Height,
		FetchedAt:         p.now(),
	})
	if err != nil {
		return Result{}, fmt.Errorf("imagepipeline: record cover for book %d: %w", bookID, err)
	}

	return Result{
		AssetID:      assetID,
		CacheKey:     key,
		Format:       out.Format,
		SourceWidth:  out.Width,
		SourceHeight: out.Height,
		Renditions:   len(out.Renditions),
		CachedBytes:  written,
	}, nil
}

// classify turns an HTTP status into this package's three answers.
//
// 404 IS THE ONE STATUS SPLIT OUT, because it is the one that changes what a
// caller should DO. Everything else non-2xx is transient by default, which is
// the safe direction: treating a permanent failure as transient costs one round
// trip per attempt, while treating a transient one as permanent leaves a work
// without a cover until a human notices.
func classify(status int, bookID int64) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: book %d", ErrNoCover, bookID)
	default:
		return fmt.Errorf("%w: book %d answered %d", ErrUpstreamUnavailable, bookID, status)
	}
}

// contentTypeMax bounds a header value before it is quoted into an error.
//
// An upstream header is attacker-influenced text on a path that reaches slog and
// service_instance.last_error. A media type is at most a few dozen characters;
// anything longer is not one, and the truncation says so rather than hiding it.
const contentTypeMax = 96

func boundedContentType(s string) string {
	if len(s) <= contentTypeMax {
		return s
	}
	return s[:contentTypeMax] + "…(truncated)"
}
