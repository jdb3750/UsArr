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
// WHY BUILDING IT NOW IS LEGITIMATE ANYWAY: the producer is REAL.
// internal/bookorbit's Client.Cover landed in `1eee6fa` and satisfies CoverSource
// below STRUCTURALLY — no adapter, no wrapper, nothing between them, and
// TestTheRealClientSatisfiesCoverSource is a compile-time proof of it.
//
// ⚠️ AND THERE IS NO CALLER. That is measured, not suspected. The work↔book-id
// pairing exists in exactly ONE lexical scope in this tree — applyOneItem
// (internal/store/catalogue.go), which IS the import pass — there is no
// work_id → remote_id read in the other direction, no single-work store read, no
// per-work HTTP route and no work-detail screen. So the honest end state of this
// package is: complete, tested, and waiting on ONE call site that is a shape
// decision about UsArr rather than either lane's to pick. Poster is per-work
// precisely so that decision is not forced to be the import's batch gate.
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
	"strconv"
	"time"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// CoverSource is the two things this package needs from a catalogue client.
//
// IT IS DEFINED HERE, WHERE IT IS CONSUMED, AND NOT WHERE IT IS IMPLEMENTED.
// That is the Go idiom and it has a concrete payoff on this tree: the pipeline
// is testable against a fabricated image with no HTTP round trip, and adding it
// costs the client package nothing — no import of this one, no interface to
// satisfy, not a line changed there.
//
// ⚠️ *bookorbit.Client SATISFIES IT STRUCTURALLY, WITH NO ADAPTER, and that is a
// requirement rather than a happy accident. An earlier draft of this package
// declared its own Cover struct and wrapped the client in a fifteen-line
// translation; the wrapper was the only place a field could be dropped or a
// status re-interpreted on the way past, and it bought nothing that the client's
// own types do not already say. TestTheRealClientSatisfiesCoverSource is the
// compile-time proof, so the day a signature moves the build says so rather than
// the wrapper silently absorbing it.
//
// ⚠️ THE COST OF THAT CHOICE, RECORDED BECAUSE IT IS A REAL ONE: this interface
// now names bookorbit.Cover, so the package is BookOrbit-shaped rather than
// source-agnostic. A second catalogue source does not slot in behind it — it
// needs either its own entry point or this interface generalised. That is a
// deliberate trade of a seam for the absence of a translation layer, on a
// milestone with exactly one catalogue source (ADR-0052), and the generalisation
// is a day's work rather than a redesign because everything below Poster —
// decode, renderAll, the cache, the store write — takes bytes and knows nothing
// about where they came from.
//
// SSRF IS THE IMPLEMENTATION'S, NOT THIS PACKAGE'S, and that is deliberate
// rather than an omission. A BookOrbit cover URL is security.md §2's CLASS 1 —
// an admin-configured service URL, built from a book id UsArr already holds,
// never harvested from an upstream response body — so it is reached through the
// same resolve-then-pin *http.Client every other call to that instance goes
// through. Building a second egress path here would be a second mechanism to
// keep in step with the policy, and the one that drifts is the one that gets used.
type CoverSource interface {
	// Cover fetches one book's cover. Its contract is doRaw's, verbatim: err is
	// non-nil ONLY for a failure that happened before a status existed, so a 404
	// arrives as Cover{Status: 404} with a nil error. classify() is the only
	// place in this tree that turns a status into a decision.
	Cover(ctx context.Context, bookID int64) (bookorbit.Cover, error)

	// BaseURL is the credential-free base URL, which is where
	// `image_asset.source_url` comes from — see Poster.
	BaseURL() string
}

// PosterWriter is the slice of *store.Store this package uses. Same reason as
// CoverFetcher: one method, defined at the point of use, so the orchestration is
// testable without a database and the database write is testable without a
// fetcher.
type PosterWriter interface {
	PutPosterAsset(ctx context.Context, p store.PosterAsset) (int64, error)
}

// The four refusals a caller must be able to tell apart.
var (
	// ErrCoverNotFound is a 404.
	//
	// ⚠️ BOOKORBIT MAKES A CONTENT-FILTERED BOOK DELIBERATELY INDISTINGUISHABLE
	// FROM AN ABSENT ONE. verifyBookAccess throws the SAME NotFoundException for
	// a book id that does not exist and for a book the credential's content
	// filters hide (book.service.ts:489-498) — that is a design decision
	// upstream, not an accident of error handling, and it is there so a caller
	// cannot use 404-vs-403 to enumerate what it may not see.
	//
	// Two more arms answer with the same status: a cover file that is genuinely
	// missing (getCoverPath returns null, the controller throws —
	// book.controller.ts:343-344, book.service.ts:1253-1271), and Nest's
	// catch-all.
	//
	// SO ANYTHING DOWNSTREAM THAT TREATS "NOT FOUND" AS A FACT ABOUT A FILE IS
	// REASONING PAST THAT DECISION. The failure it produces is specific: every
	// book the service account's filter hides gets permanently recorded as
	// having no artwork, when what it is is HIDDEN — wrong, silent, and green in
	// every test that does not know about the overloading. The upstream cannot
	// tell you which arm you hit, so no amount of care downstream recovers it;
	// the only sound move is not to derive a durable fact from this status.
	//
	// ⚠️ SOURCE, WITH ITS LIMIT: read from BookOrbit's controller at
	// bookorbit/bookorbit@73b7877d, NEVER observed against a running server.
	// There is no placeholder arm in that code — nothing sits between the null
	// and the throw — but that is a reading of source, not a measurement.
	ErrCoverNotFound = errors.New("imagepipeline: upstream returned no cover for this credential")

	// ErrCoverForbidden is a 403, and it is SEPARATE FROM THE 404 because it is
	// a different problem with a different fix. BookOrbit's
	// libraryService.verifyUserAccess throws ForbiddenException
	// (library.service.ts:65-69): the credential cannot reach that library at
	// all. That is a scope to widen, not a cover to re-fetch, and folding it in
	// with the transient statuses would retry it forever against a permission
	// that will not change on its own.
	ErrCoverForbidden = errors.New("imagepipeline: the credential may not reach this book")

	// ErrUpstreamUnavailable is any other non-2xx. TRANSIENT: a 503 during a
	// restart, a 502 from a proxy, a 429. Worth trying again later.
	ErrUpstreamUnavailable = errors.New("imagepipeline: upstream did not answer with a cover")

	// ErrNoBody is a 2xx that carried nothing. Treated as transient — an empty
	// 200 is a proxy or a truncation far more often than it is a considered
	// answer.
	ErrNoBody = errors.New("imagepipeline: upstream answered with no bytes")
)

// IsCredentialScoped reports whether a failure is a property of the CREDENTIAL
// rather than of the book, and is therefore invalidated by a scope change.
//
// It exists because REVIEW-LOG LS-260's question — what a coverless book returns
// — turned out to have an answer that does not settle what the caller needs. A
// 404 answers "not for you, now"; widening the service account's content filters
// or its library access can turn the same request into a 200 tomorrow. A caller
// that persists ANY per-book cover verdict must key it on the scope that
// produced it, or re-derive it after a scope change.
//
// ⚠️ THE PIPELINE ITSELF PERSISTS NOTHING ON THESE PATHS, which is the strongest
// form of "invalidatable by a scope change" available: there is no row and no
// file to invalidate. This predicate is for a caller that wants to remember
// something anyway.
func IsCredentialScoped(err error) bool {
	return errors.Is(err, ErrCoverNotFound) || errors.Is(err, ErrCoverForbidden)
}

// IsRetryable reports whether a Poster failure is worth attempting again.
//
// It exists so a caller does not have to know which sentinels are terminal —
// that knowledge belongs with the classification, and a caller that guessed
// would either hammer a 404 forever or give up on a restart.
//
// ⚠️ "NOT RETRYABLE" HERE MEANS "NOT ON THIS RUN", NOT "NEVER". For the two
// credential-scoped answers it is IsCredentialScoped that says what would change
// the outcome; false here only stops the loop that is running now.
func IsRetryable(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrUpstreamUnavailable), errors.Is(err, ErrNoBody):
		return true
	// Asking again inside one run buys nothing: the two credential-scoped
	// answers depend on a scope that will not change mid-run, and an
	// undecodable cover or a decompression bomb answers identically forever.
	case errors.Is(err, ErrCoverNotFound),
		errors.Is(err, ErrCoverForbidden),
		errors.Is(err, ErrRemoteIDNotABookID),
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
	source CoverSource
	cache  *imagecache.Cache
	writer PosterWriter
	now    func() time.Time
}

// New builds a pipeline. now may be nil, in which case time.Now is used; it is a
// parameter so a test can pin `fetched_at` rather than assert around a clock.
func New(source CoverSource, cache *imagecache.Cache, writer PosterWriter, now func() time.Time) (*Pipeline, error) {
	if source == nil {
		return nil, errors.New("imagepipeline: a cover source is required")
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
	return &Pipeline{source: source, cache: cache, writer: writer, now: now}, nil
}

// coverPath is the route CoverSource.Cover calls, spelled here because
// internal/bookorbit builds it from unexported constants and so cannot be asked
// what URL it fetched.
//
// ⚠️ A SECOND COPY OF A PATH, ACKNOWLEDGED RATHER THAN HIDDEN — AND SAFE FOR ONE
// SPECIFIC REASON. This string never fetches anything. The fetch is the client's,
// over the client's own path, through the client's own pinned transport. This
// copy becomes `image_asset.source_url` and, from it, the cache key. So if
// upstream moves the route, the bytes still arrive and what changes is the key
// they are filed under — one cache miss that re-renders, not a broken fetch and
// not a wrong image. That is why it is not worth asking the sync lane to export a
// CoverURL method; if one ever appears, delete this and call it.
const coverPath = "/api/v1/books/"

// ErrRemoteIDNotABookID refuses a `service_item_link.remote_id` that is not a
// BookOrbit book id.
//
// ⚠️ IT IS A REFUSAL WITH A REASON AND NEVER A SUBSTITUTED ZERO. `remote_id` is
// a TEXT column — libsync writes it with strconv.FormatInt — so the store hands
// it over UNPARSED and this is the boundary that parses it. A value that will not
// parse is REAL INFORMATION: a corrupted row, or a link from some other service
// handed to a BookOrbit source. Coercing it to 0 would turn either into a
// perfectly ordinary 404, which the upstream cannot distinguish from a book that
// exists and has no cover — the exact confusion ErrCoverNotFound is written
// against, arriving from our own side this time.
var ErrRemoteIDNotABookID = errors.New("imagepipeline: remote id is not a BookOrbit book id")

// Poster fetches, renders and records the cover for ONE work.
//
// # Concurrency is the CALLER'S to bound, and this package builds no bound
//
// ⚠️ STATED BECAUSE THE OPPOSITE WAS NEARLY WRITTEN HERE. ARCHITECTURE.md §4.4
// describes a min(NumCPU, 4) semaphore, and it is PROSE: it is about image
// TRANSCODING rather than fetching, and no such semaphore is implemented
// anywhere — the tree's only one is the Argon2id gate, whose own comment says
// §4.4 "is a design statement for a pipeline that is not built yet". A comment
// here claiming fetches are bounded by it would be documenting a guard that does
// not exist, which is the exact defect this package was written to stop
// repeating.
//
// So: Poster is ONE fetch, synchronous, and holds no lock and no pool. Calling
// it in a loop is serial by construction; calling it from N goroutines opens N
// connections, and NOTHING HERE STOPS THAT. Whoever writes the loop owns the
// bound, and owes it a test that N+1 is refused.
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
func (p *Pipeline) Poster(ctx context.Context, instanceID int64, remoteKind, remoteID string) (Result, error) {
	bookID, err := parseBookID(remoteID)
	if err != nil {
		return Result{}, err
	}

	// THE SOURCE URL IS BUILT HERE, from the client's own credential-free base
	// URL. bookorbit.New clears userinfo, query and fragment off that base and
	// trims its trailing slash before keeping it, and this route sends no query
	// string of its own — the `?t=` BookOrbit accepts selects a Cache-Control
	// value and is deliberately not sent. So the guards below should NEVER fire
	// on this path, and that is the intended relationship: they exist to catch a
	// bug, and a guard that only ever fires on a bug is not one doing nothing.
	sourceURL := p.source.BaseURL() + coverPath + strconv.FormatInt(bookID, 10) + "/cover"

	cover, err := p.source.Cover(ctx, bookID)
	if err != nil {
		// The client's error is already this project's shape — redacted, and
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
	key, err := imagecache.Key(sourceURL)
	if err != nil {
		// sourceURL is NOT in the message. It is the string suspected of
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
		// THE ITEM, NOT A WORK ID. The store re-resolves the work through
		// service_item_link inside its own transaction — see PosterAsset's
		// header. A work id resolved out here would have been read BEFORE the
		// network round trip above, and an item deleted in between would put
		// this cover on whatever now holds that id.
		RemoteKind: remoteKind,
		RemoteID:   remoteID,
		SourceURL:  sourceURL,
		CacheKey:   key,
		Format:     out.Format,
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

// parseBookID is the TEXT→int64 boundary between `service_item_link.remote_id`
// and CoverSource.Cover.
//
// A non-positive id is refused HERE as well as by the client, and the duplication
// is the point: the client refuses it as a bad argument, while a zero or negative
// remote_id reaching this function is a fact about a ROW, and the two want
// different messages. Refusing it also keeps a caller's bug from arriving
// upstream as an ordinary 404.
func parseBookID(remoteID string) (int64, error) {
	id, err := strconv.ParseInt(remoteID, 10, 64)
	if err != nil {
		// The value IS quoted: it is a database column, not a credential, and it
		// is the whole of what makes a corrupted row findable. It is bounded
		// first, because nothing guarantees a TEXT column is short.
		return 0, fmt.Errorf("%w: %q", ErrRemoteIDNotABookID, boundedRemoteID(remoteID))
	}
	if id <= 0 {
		return 0, fmt.Errorf("%w: %d is not a positive id", ErrRemoteIDNotABookID, id)
	}
	return id, nil
}

// remoteIDMax bounds a remote_id before it is quoted into an error. A real
// BookOrbit id is a handful of digits; anything longer is not one.
const remoteIDMax = 64

func boundedRemoteID(s string) string {
	if len(s) <= remoteIDMax {
		return s
	}
	return s[:remoteIDMax] + "…(truncated)"
}

// classify turns an HTTP status into this package's four answers.
//
// TWO STATUSES ARE SPLIT OUT, and they are split from each other as well as from
// the rest, because they imply different actions. A 404 is credential-scoped and
// says nothing about the file (see ErrCoverNotFound). A 403 is a library the
// credential cannot reach at all (ErrCoverForbidden) — a scope to widen, not a
// cover to re-fetch. Everything else non-2xx is transient by default, which is
// the safe direction: treating a permanent failure as transient costs one round
// trip per attempt, while treating a transient one as permanent leaves a work
// without a cover until a human notices.
func classify(status int, bookID int64) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: book %d", ErrCoverNotFound, bookID)
	case status == http.StatusForbidden:
		return fmt.Errorf("%w: book %d", ErrCoverForbidden, bookID)
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
