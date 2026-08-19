package libsync

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/jdb3750/UsArr/internal/imagepipeline"
)

// Phase D of a full import: one cover per imported item.
//
// # Where it runs, and why that is the whole design
//
// BETWEEN COMMITTED BATCHES, exactly like the credit pass and the file walk, and
// NEVER inside applyOneItem or any open write transaction. ApplyCatalogueBatch
// wraps a whole batch — up to min(2000 rows, 100 ms) — in one BEGIN IMMEDIATE on
// a ONE-CONNECTION writer pool, so a fetch issued from inside it would hold the
// process's only writer across a network round trip PER BOOK; every interactive
// write in the server would queue behind the slowest cover in the library.
// internal/db/sqlite.go's Write says it in those words: "fn must not call Write,
// and must not hold the transaction across a network call: the whole process
// shares one writer connection."
//
// The transaction-free seam is streamAndApply's post-commit []ImportedItem
// slice, and this pass consumes it on the same terms the other two do.
//
// # It is keyed by REMOTE ID and never by work id
//
// ImportedItem carries no work id by design. store.PosterAsset re-resolves the
// work through service_item_link inside its OWN write transaction, for
// store.CreditSet's reason quoted verbatim in internal/store/credits.go:
// "Carrying a work id across the two passes would mean trusting an id read in a
// different transaction, and an item deleted between the two passes would then
// have its credits written onto whatever now holds that id." The hazard is
// strictly worse here, because the gap between resolving an id and using it is
// not a fast local read — it is a cover fetch against a service that may be
// slow. `work.id` is INTEGER PRIMARY KEY, so SQLite may reuse a deleted row's
// id, which turns a stale id from "no such work" into the wrong book's cover.
//
// # A cover NEVER fails an import
//
// fetchCovers returns nothing. That is the signature saying what a comment would
// otherwise only promise: there is no error path by which artwork can stop a
// catalogue being written or stop last_full_sync_at being stamped. A partial
// catalogue that says so beats no catalogue, and a complete catalogue with
// partial artwork beats both.

// PosterFetcher is the one thing the cover pass needs from internal/imagepipeline.
//
// IT IS DEFINED HERE, WHERE IT IS CONSUMED, on that package's own reasoning for
// CoverSource: the pass is testable against a fake that fetches nothing, and
// adding it costs internal/imagepipeline nothing — no import of this package, no
// interface to satisfy, not a line changed there.
//
// *imagepipeline.Pipeline satisfies it structurally, with no adapter;
// TestTheRealPipelineSatisfiesPosterFetcher is the compile-time proof, so a
// signature that moves fails the build rather than being absorbed by a wrapper.
type PosterFetcher interface {
	// Poster fetches, renders and records the cover for ONE item, keyed by the
	// pair store.PosterAsset takes. See imagepipeline.Pipeline.Poster for the
	// two-store write order and for what each returned sentinel means.
	Poster(ctx context.Context, instanceID int64, remoteKind, remoteID string) (imagepipeline.Result, error)
}

// coverRequest is one item the cover pass will consider. It is ImportedItem's
// projection for this pass, on creditRequest()/fileRequest()'s pattern.
type coverRequest struct {
	RemoteKind string
	RemoteID   string
}

func (i ImportedItem) coverRequest() coverRequest {
	return coverRequest{RemoteKind: i.RemoteKind, RemoteID: i.RemoteID}
}

// ── The bound ───────────────────────────────────────────────────────────────
//
// ⚠️ THIS IS A NEW BOUND AND IT IS NOT INHERITED FROM ANYWHERE. Three
// measurements, each checked in the tree rather than remembered:
//
//   - ARCHITECTURE.md §4.4's min(NumCPU, 4) semaphore is about image
//     TRANSCODING, not fetching, and it is PROSE — nothing implements it.
//   - The only semaphore that exists in this repository is internal/crypto's
//     Argon2id gate, whose own comment says §4.4 "is a design statement for a
//     pipeline that is not built yet".
//   - internal/imagepipeline builds none either, and says so at Poster:
//     "Whoever writes the loop owns the bound, and owes it a test that N+1 is
//     refused."
//
// So this file is that loop and this is that bound. It is a semaphore rather
// than a rate limiter for the reason internal/crypto/password.go gives: a
// limiter needs per-something state, a policy and somewhere for both to live,
// while N permits caps concurrent fetches at N regardless of arrival rate with
// none of that.
//
// WHAT IT ACTUALLY CAPS, stated because a bound whose subject is vague is not
// one — AND IT IS WIDER THAN "FETCHES", DELIBERATELY. One permit covers one
// imagepipeline.Poster call end to end, and that call is a fetch PLUS a decode
// PLUS a seven-width render PLUS seven cache writes. Poster's duration is in
// fact dominated by the CPU half, under internal/imagepipeline's 32 MiB / 64 MP
// ceiling. So this gate bounds concurrent TRANSCODES — which is what §4.4 asks
// for and what jellyfin#9795 is about — and bounds concurrent connections only
// as a consequence of the two being one call.
//
// ⚠️ SO N PERMITS IS NOT N CONCURRENT REQUESTS, and a reader sizing this against
// an upstream must know it. internal/bookorbit's client re-mints its access
// token and RETRIES ONCE on a 401, so a single Poster can be up to three wire
// exchanges — mint, request, re-minted request — none of which this gate can
// see. Bounding the upstream's request rate is a different job with a different
// instrument, and it belongs in the client that issues the requests, not in a
// loop that counts calls.
//
// It is PROCESS-WIDE and not per-import: two instances importing at once share
// these permits rather than getting min(NumCPU, 4) each, which is the only
// version of the number that means anything on a Pi.
//
// SIZING. min(NumCPU, 4), which is §4.4's own figure adopted deliberately rather
// than copied: the citation behind it (jellyfin#9795) is an unbounded-transcode
// OOM, and Poster transcodes. REVIEW-LOG LS-260 measured a cover fetch against
// the owner's instance at a median 5 ms — "~0.8s serially over 151 series, ~0.2s
// at 4 concurrent" — so four is already most of the available speed-up on a real
// library, and the fifth connection buys latency the owner would not notice
// against memory a Pi would.
const maxCoverConcurrency = 4

// coverGate is the semaphore. A buffered channel rather than
// golang.org/x/sync/semaphore, for kdfGate's reason: x/sync is an indirect
// dependency today and every acquisition here is one unit.
var coverGate = make(chan struct{}, coverPermits())

func coverPermits() int {
	if n := runtime.NumCPU(); n < maxCoverConcurrency {
		return n
	}
	return maxCoverConcurrency
}

// CoverPermits reports how many cover fetches may be in flight at once.
//
// Exported so the bound is inspectable rather than folklore — and because a test
// that wants to FILL the gate needs to know how many permits to take. See
// AcquireCover.
func CoverPermits() int { return cap(coverGate) }

// coverMaxWait is how long one item waits for a permit before the pass gives up
// on this run. It is a var for exactly one reason: this package's own drill
// shortens it so the test does not cost seconds per assertion. Nothing in
// production writes it.
//
// FIVE SECONDS, and the number is chosen against what a refusal COSTS rather
// than against the fetch time. A refused cover is one book rendering without
// artwork until the next import, which is decoration; a wait long enough to
// matter is an import that appears to hang. Anything above a few multiples of a
// fetch means the permits are held by another import, and queueing a whole
// library behind that is worse than deferring the whole library to the next run
// — which is what fetchCovers does on the first refusal.
var coverMaxWait = 5 * time.Second

// ErrCoverFetchBusy means no cover-fetch permit came free within coverMaxWait.
//
// It is a distinct sentinel and not folded into the pipeline's transient class,
// because it is not a fact about the upstream at all: BookOrbit is fine, THIS
// PROCESS declined to open another connection. Reporting it as an upstream
// failure would put a red mark on a healthy service.
var ErrCoverFetchBusy = errors.New("libsync: no cover-fetch permit came free")

// AcquireCover reserves one cover-fetch permit. The returned function releases
// it and must be called exactly once.
//
// ⚠️ IT IS EXPORTED FOR THE DRILL, on internal/crypto's AcquireKDF precedent and
// with the same honesty about it: NO PRODUCTION CALLER USES THIS — fetchCovers
// takes its permits through the unexported acquireCover. It exists because a
// limiter nobody has watched refuse is indistinguishable from no limiter, and
// because filling the gate deterministically is the only way to drill one that
// does not rot the moment the machine gets faster. A timing-calibrated guard is
// a green protecting nothing.
func AcquireCover(ctx context.Context) (func(), error) { return acquireCover(ctx) }

// acquireCover takes one permit, waiting at most coverMaxWait.
//
// The uncontended case does not touch a timer: the first select's default arm
// makes a free permit cost one channel send, so on a single-user install with
// one import running the gate is invisible.
func acquireCover(ctx context.Context) (func(), error) {
	release := func() { <-coverGate }
	select {
	case coverGate <- struct{}{}:
		return release, nil
	default:
	}

	timer := time.NewTimer(coverMaxWait)
	defer timer.Stop()
	select {
	case coverGate <- struct{}{}:
		return release, nil
	case <-timer.C:
		return nil, ErrCoverFetchBusy
	case <-ctx.Done():
		// A caller that has gone away is not a saturated server, and filing the
		// two under one error would make a cancelled import look like an
		// overload. crypto/password.go draws the same line.
		return nil, ctx.Err()
	}
}

// ── The pass ────────────────────────────────────────────────────────────────

// CoverResult is what one cover pass did.
//
// ⚠️ ITS COUNTERS SUM TO THE NUMBER OF ITEMS THE PASS WAS HANDED, and
// TestEveryImportedItemLandsInExactlyOneCoverCounter pins that. A pass whose
// counters do not add up is one where an item disappeared silently, which is the
// failure this struct exists to make impossible — "how many of my books got
// artwork" must have an answer checkable against "how many books were there".
type CoverResult struct {
	// Fetched is covers fetched, rendered and recorded.
	Fetched int

	// Bytes is the total written to the image cache across every width.
	Bytes int

	// AlreadyHeld is items whose work already carried a poster. NOT re-fetched;
	// see store.PosteredItems for what "already" spans.
	AlreadyHeld int

	// Absent is a 404.
	//
	// ⚠️ IT IS NOT A VERDICT AND NOTHING PERSISTS IT. BookOrbit throws the SAME
	// NotFoundException for a book with no cover file, a book id that does not
	// exist, and a book the credential's content filters HIDE
	// (imagepipeline.ErrCoverNotFound cites book.service.ts for all three), so a
	// 404 is not evidence about a file. Recording "this book has no artwork"
	// from one would permanently mislabel every book the service account cannot
	// see. The next import asks again.
	//
	// REVIEW-LOG LS-260 is open on precisely this question for the other
	// adapter — "whether a series that exists but has no cover returns a 404 or
	// a 200 placeholder is unmeasured, and therefore whether a failed cover
	// fetch should be retried or cached as permanently absent is still
	// undecided". Retrying is the answer that is safe under BOTH readings.
	Absent int

	// Forbidden is a 403: the credential cannot reach that library at all.
	// Counted apart from Absent because the fix differs — a scope to widen, not
	// a cover to re-fetch (imagepipeline.ErrCoverForbidden).
	Forbidden int

	// Failed is every other refusal: a transient upstream, an undecodable
	// image, a cache write that would not land, a malformed remote id.
	Failed int

	// Deferred is items this run did not attempt at all — the permit was
	// refused, or the pass stopped before reaching them. Distinct from Failed
	// because nothing was asked of the upstream and nothing is known about them.
	Deferred int
}

// Total is every item the pass was handed.
func (r CoverResult) Total() int {
	return r.Fetched + r.AlreadyHeld + r.Absent + r.Forbidden + r.Failed + r.Deferred
}

// syncReportCoverPassIncomplete is sync_report.kind for a cover pass that did
// not settle every item it was asked about.
//
// ⚠️ ONE ROW PER IMPORT AND NOT ONE PER ITEM, which is where this deliberately
// departs from recordFileWalkFailures. That pass writes a row per dropped item
// because the count is bounded by items whose walk failed and the ordinary case
// is zero; here the ordinary case is NOT zero — a library where the service
// account cannot see half the books answers 404 for half the books — and a row
// each would be tens of thousands of rows carrying the same sentence. The counts
// are what an operator needs, and they fit in one row.
//
// It is an unexported literal rather than a store constant, on
// `container_declined`'s pattern: sync_report.kind carries no CHECK (migration
// 00005 says why), and store.SyncReportFileWalkFailed is a constant only because
// it has a READER in another package. Nothing reads this one yet, so a second
// exported spelling would be a promise with no second side.
const syncReportCoverPassIncomplete = "cover_pass_incomplete"

// coverProgressStride is how many settled items pass between progress frames.
//
// The other three passes publish once per COMMITTED BATCH, which is a beat they
// get for free. This pass has no batch — imagepipeline.Poster commits per item —
// so a frame per item would be one SSE frame per book. A stride is the cheapest
// thing that keeps the readout moving without turning the stream into a
// firehose, and the pass publishes a final frame regardless, so the last numbers
// are never a stride out of date.
const coverProgressStride = 50

// coverOutcome is one worker's answer, carried back to the single goroutine that
// counts. It exists so no counter, log line or progress frame is produced from a
// worker — see the accumulation loop in fetchCovers.
type coverOutcome struct {
	req   coverRequest
	bytes int
	err   error
}

// fetchCovers is phase D: one cover per imported item, bounded by coverGate.
//
// IT IS A NO-OP WHEN NO PosterFetcher IS CONFIGURED, and that is principle 3 —
// "every feature degrades honestly when a service is absent" — applied to a pass
// rather than to a service. An importer with no Covers is not an error and is
// not logged as one: the Kavita adapter has no cover fetch wired, and the *Arr
// adapters will not.
//
// ⚠️ IT RETURNS NOTHING, AND THE MISSING error IS THE CONTRACT. Every failure
// below is counted, logged and — when the pass came up short — recorded in
// sync_report; none of them propagates. See this file's header.
func (im *Importer) fetchCovers(ctx context.Context, instanceID int64, items []ImportedItem, rep *Report) {
	if im.Covers == nil || len(items) == 0 {
		return
	}

	reqs := make([]coverRequest, 0, len(items))
	for _, it := range items {
		reqs = append(reqs, it.coverRequest())
	}

	// THE IDEMPOTENCE READ, and a failure here disables the pass rather than
	// widening it. Without the set every item looks un-postered, so carrying on
	// would re-fetch a whole library that already has its artwork — turning a
	// read error into the exact stampede this read exists to prevent. Zero
	// covers is the cheaper wrong answer, and it self-corrects next import.
	held, err := im.Store.PosteredItems(ctx, instanceID)
	if err != nil {
		im.log().Warn("the cover pass was skipped: the set of already-postered items could not be read",
			"instance_id", instanceID, "items", len(reqs), "error", err)
		rep.Covers = CoverResult{Deferred: len(reqs)}
		im.recordCoverShortfall(ctx, instanceID, rep.Covers)
		return
	}
	postered := make(map[coverRequest]bool, len(held))
	for _, l := range held {
		postered[coverRequest{RemoteKind: l.RemoteKind, RemoteID: l.RemoteID}] = true
	}

	res := CoverResult{}
	pending := make([]coverRequest, 0, len(reqs))
	for _, r := range reqs {
		if postered[r] {
			res.AlreadyHeld++
			continue
		}
		pending = append(pending, r)
	}
	if len(pending) == 0 {
		rep.Covers = res
		im.publishCoverProgress(instanceID, res, len(reqs))
		return
	}

	// ONE GOROUTINE PER PERMIT, never more, so an import cannot park a hundred
	// goroutines on a gate of four. The GATE is still the bound and the worker
	// count is not a second one: permits are process-wide, so two imports running
	// at once share four fetches between them rather than taking four each —
	// which a worker pool sized per import could never express.
	workers := CoverPermits()
	if workers > len(pending) {
		workers = len(pending)
	}

	var (
		stopOnce sync.Once
		stop     = make(chan struct{})
		work     = make(chan coverRequest)
		results  = make(chan coverOutcome, workers)
		wg       sync.WaitGroup
	)
	halt := func() { stopOnce.Do(func() { close(stop) }) }

	go func() {
		defer close(work)
		for _, r := range pending {
			select {
			case work <- r:
			case <-stop:
				return
			}
		}
	}()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range work {
				select {
				case <-stop:
					return
				default:
				}
				if ctx.Err() != nil {
					// The rest of the library is Deferred, by subtraction. A
					// cancelled import must not keep opening connections.
					halt()
					return
				}

				release, err := acquireCover(ctx)
				if err != nil {
					// ⚠️ THE FIRST REFUSAL ENDS THE PASS, and it does not merely
					// skip one book. A saturated gate means the permits are held
					// elsewhere for at least coverMaxWait, so the alternative is
					// paying that wait once per remaining item — a five-second
					// stall per book across a whole library. Deferring the lot to
					// the next import costs nothing durable: no verdict is
					// written, and the items come back untouched.
					halt()
					return
				}
				out, err := im.Covers.Poster(ctx, instanceID, r.RemoteKind, r.RemoteID)
				release()

				results <- coverOutcome{req: r, bytes: out.CachedBytes, err: err}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	// ⚠️ EVERY COUNTER, EVERY LOG LINE AND EVERY PROGRESS FRAME IS PRODUCED HERE,
	// on ONE goroutine, and that is not merely tidier than a mutex. ProgressFunc
	// is a caller's callback — cmd/usarr hands it straight to the SSE hub — and
	// the other three passes only ever call it from the import's own goroutine.
	// Publishing from N workers would quietly widen that contract to "must be
	// safe under concurrent calls" for every future implementer, to save a
	// channel.
	attempts := 0
	for o := range results {
		attempts++
		switch {
		case o.err == nil:
			res.Fetched++
			res.Bytes += o.bytes
		case errors.Is(o.err, imagepipeline.ErrCoverNotFound):
			res.Absent++
		case errors.Is(o.err, imagepipeline.ErrCoverForbidden):
			res.Forbidden++
		default:
			res.Failed++
		}

		switch {
		case o.err == nil:
		case errors.Is(o.err, imagepipeline.ErrCoverNotFound),
			errors.Is(o.err, imagepipeline.ErrCoverForbidden):
			// DEBUG, not WARN. On a library the service account only partly
			// sees, these are the ordinary answer, and a warning per book would
			// bury the one line that matters.
			im.log().Debug("no cover for this credential; it will be asked for again next import",
				"instance_id", instanceID, "remote_kind", o.req.RemoteKind,
				"remote_id", o.req.RemoteID, "error", o.err)
		default:
			// WARN, not ERROR: the import continues and completes. Same register
			// as the file walk's dropped series.
			im.log().Warn("a cover could not be fetched and the item was left without one",
				"instance_id", instanceID, "remote_kind", o.req.RemoteKind,
				"remote_id", o.req.RemoteID, "error", o.err)
		}
		if attempts%coverProgressStride == 0 {
			im.publishCoverProgress(instanceID, res, len(reqs))
		}
	}

	// Deferred is DERIVED and not counted, so it cannot disagree with the rest:
	// whatever the workers did not account for is what they never attempted.
	res.Deferred = len(pending) - res.Fetched - res.Absent - res.Forbidden - res.Failed
	rep.Covers = res
	im.publishCoverProgress(instanceID, res, len(reqs))
	im.recordCoverShortfall(ctx, instanceID, res)
}

// publishCoverProgress sends one `covers` frame.
//
// ItemsRead is every item the pass has settled an answer for, skips included,
// and Applied is covers written — which is what http-api.md §5.4's table says
// for this phase. Total is len(items), UsArr's own count of the work the pass
// was handed, never a figure BookOrbit reported.
func (im *Importer) publishCoverProgress(instanceID int64, res CoverResult, total int) {
	im.publish(Progress{
		InstanceID: instanceID, Phase: "covers",
		ItemsRead: res.Fetched + res.AlreadyHeld + res.Absent + res.Forbidden + res.Failed,
		Applied:   res.Fetched,
		Total:     total,
	})
}

// recordCoverShortfall writes the durable half: one sync_report row when the
// pass could not settle every item.
//
// ⚠️ THE Report COUNTER IS NOT THE RECORD, on recordFileWalkFailures's argument
// verbatim — an import triggered by a background connect has no caller left to
// read the Report by the time anyone asks why a shelf is full of blank covers.
//
// ⚠️ AND A 404 ALONE IS NOT A SHORTFALL. A library the credential only partly
// sees answers 404 for the rest, every import, for ever; a row each time would be
// a permanent red mark on a service behaving exactly as configured. Absent is
// REPORTED in the detail when a row is written for another reason, and never
// triggers one on its own.
//
// detail carries NO UPSTREAM TEXT — six integers and two fixed strings — so there
// is nothing here for ssrf.RedactText to find and no path by which a BookOrbit
// response body could reach the column. schema.md asks for redaction; carrying
// nothing to redact is that taken to its limit.
func (im *Importer) recordCoverShortfall(ctx context.Context, instanceID int64, res CoverResult) {
	if res.Failed == 0 && res.Forbidden == 0 && res.Deferred == 0 {
		return
	}
	detail, err := json.Marshal(map[string]any{
		"phase":        "covers",
		"fetched":      res.Fetched,
		"already_held": res.AlreadyHeld,
		"absent":       res.Absent,
		"forbidden":    res.Forbidden,
		"failed":       res.Failed,
		"deferred":     res.Deferred,
		// Named rather than implied, because both halves are the surprising
		// ones: nothing was written to say these items have no artwork, and the
		// catalogue is complete regardless.
		"effect": "the catalogue is complete; these items have no poster yet and no verdict was " +
			"recorded against them, so the next import asks again",
	})
	if err != nil {
		im.log().Warn("the cover shortfall could not be encoded and was not recorded",
			"instance_id", instanceID, "error", err)
		return
	}
	// remote_kind and remote_id are left empty — the row is about the PASS, not
	// about one item, and store.RecordSyncReport writes empty as SQL NULL.
	if err := im.Store.RecordSyncReport(ctx, instanceID,
		syncReportCoverPassIncomplete, "", "", string(detail)); err != nil {
		// Logged and dropped: this is the report of a shortfall in artwork, and
		// failing an otherwise complete import over it would invert the whole
		// point of the pass.
		im.log().Warn("the cover shortfall could not be recorded",
			"instance_id", instanceID, "error", err)
	}
}
