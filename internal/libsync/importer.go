package libsync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// Source is one catalogue adapter. See doc.go for why the boundary is here.
type Source interface {
	// Containers lists the containers the upstream itself named, each with the
	// work.kind UsArr decided for it (or a decline reason).
	Containers(ctx context.Context) ([]store.CatalogueContainer, error)

	// StreamItems hands fn each item as it decodes and returns HOW MANY IT
	// HANDED OVER — on success and on failure alike. A stream that dies
	// mid-array returns a non-zero count alongside its error and those calls to
	// fn happened; their effects stand.
	StreamItems(ctx context.Context, fn func(store.CatalogueItem) error) (int, error)
}

// Defaults for the batch sizing reference/sync.md §6 rule 3 specifies.
const (
	// DefaultBatchRows is the row half of min(2000 rows, 100 ms).
	DefaultBatchRows = 2000

	// DefaultBatchWindow is the wall-clock half, and it is the half that
	// matters: the bulk importer and every interactive write share one writer
	// connection, so a batch that runs long stalls the request path.
	DefaultBatchWindow = 100 * time.Millisecond
)

// ImportedItem is one item that reached a COMMITTED batch, carried forward to
// the two per-item passes that run after the stream closes.
//
// IT IS ONE TYPE FEEDING TWO PASSES rather than two parallel slices, because
// the slice it lives in is the import's one unbounded allocation
// (streamAndApply's comment budgets it) and duplicating it per pass would
// double that for no benefit. The per-pass request types are projections of it.
type ImportedItem struct {
	RemoteKind string
	RemoteID   string
	Kind       string

	// RemoteSubtype is only read by the file pass, which needs it to give the
	// edition a format without a second HTTP read — see files.go's FileRequest.
	RemoteSubtype string
}

func (i ImportedItem) creditRequest() CreditRequest {
	return CreditRequest{RemoteKind: i.RemoteKind, RemoteID: i.RemoteID, Kind: i.Kind}
}

func (i ImportedItem) fileRequest() FileRequest {
	return FileRequest{
		RemoteKind: i.RemoteKind, RemoteID: i.RemoteID, RemoteSubtype: i.RemoteSubtype,
	}
}

// DeclinedContainer is one upstream container UsArr has no kind for.
type DeclinedContainer struct {
	RemoteID string
	Name     string
	Reason   string
}

// Report is what one full import did. It is returned on failure too, describing
// how far the import got.
type Report struct {
	InstanceID int64
	StartedAt  time.Time

	// FinishedAt is stamped on EVERY return path, the failed ones included, so
	// Duration() means something on a partial import too. It is why FullImport's
	// returns are named — see its doc comment — and it is in memory only:
	// nothing persists it and no endpoint carries it
	// (docs/reference/http-api.md §3.5).
	FinishedAt time.Time

	ContainersSeen     int
	LibrariesCreated   int
	LibrariesJoined    int
	DeclinedContainers []DeclinedContainer

	// SkippedContainers is the containers that HAD a kind and still got no
	// library, because the create violated a library uniqueness index. Distinct
	// from DeclinedContainers, which never had a kind — see
	// store.SkippedContainer. Each one is also in sync_report as
	// `container_bind_failed`, written by the bind transaction itself.
	SkippedContainers []store.SkippedContainer

	// ItemsRead is what the adapter handed over. It is "how far the read got",
	// never "how many rows are correct".
	ItemsRead int

	// ItemsApplied is how many reached a committed batch. On a stream that died
	// mid-array it is smaller than ItemsRead by at most one batch.
	ItemsApplied int

	Batches int

	store.BatchResult

	// CreditItemsRead is how many items the credit pass got metadata for. It is
	// 0 when the source implements no CreditSource, which is not a failure —
	// see FullImport's phase 3.
	CreditItemsRead int

	// Credits is what the credit pass wrote. It is a NAMED FIELD rather than a
	// second embedded struct because store.BatchResult and store.CreditResult
	// both carry an Add method, and embedding both would make rep.Add ambiguous
	// at the call site that already exists.
	Credits store.CreditResult

	// FileItemsRead is how many items the file walk got volumes for. It is 0
	// when the source implements no FileSource, which is not a failure — see
	// FullImport's phase 4.
	FileItemsRead int

	// FileReadFailures is how many items the file walk COULD NOT read. It is
	// the other half of FileItemsRead and it is not derivable from it: the walk
	// also skips items whose remote id this adapter never wrote, so
	// `len(items) - FileItemsRead` counts two different things as one.
	//
	// ⚠️ IT IS NOT AN IMPORT FAILURE and does not stop last_full_sync_at being
	// stamped. It is the fact that makes an uncounted work explicable: each one
	// is also a `file_walk_failed` row in sync_report, and the pair is what lets
	// a screen say "3 series could not be read" instead of leaving three works
	// rendering http-api.md §1.4.1's "not counted yet" with no reason on offer.
	FileReadFailures int

	// Files is what the file walk wrote, on Credits's terms.
	Files store.FileResult

	// Covers is what the cover pass did. It is all zeroes when no PosterFetcher
	// is configured, which is not a failure — see FullImport's phase D.
	Covers CoverResult

	// Rollups is what the flush computed. It is the pass that turns the file
	// rows into have_count and the availability blob; without it the walk
	// leaves 151 series with file rows underneath them and 151 crossed circles
	// on top (REVIEW-LOG.md LS-211.1).
	Rollups store.RollupResult

	// Deletions is what channel 4's deletion pass did — the links and works
	// tombstoned because the read did not report them, and the containers and
	// libraries stamped absent with them. It is all zeroes on any import that
	// did not complete, because the sweep does not run on one.
	Deletions store.SweepResult

	// Completed is true only when the whole stream was read AND every batch
	// committed. It is what gates last_full_sync_at.
	Completed bool
}

// Duration is how long the import ran.
func (r Report) Duration() time.Duration {
	if r.FinishedAt.IsZero() {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}

// Progress is one progress observation, published as it happens.
type Progress struct {
	InstanceID int64  `json:"instance_id"`
	Phase      string `json:"phase"` // containers | items | credits | files | covers | done

	// ItemsRead is a RUNNING count, incremented per hand-over as the stream is
	// consumed rather than settled when it closes. A mid-run frame is therefore
	// a truthful "this far so far"; it was not always, and the three streaming
	// phases each carry the counter that makes it so.
	ItemsRead int `json:"items_read"`

	Applied int `json:"applied"`

	// Total is UsArr's OWN count of the work a per-item pass was handed: the
	// length of the request list built from the items that reached a committed
	// batch in the `items` phase. It is NOT a figure the upstream reported.
	//
	// ⚠️ This comment used to say "the upstream's own total when it reported
	// one", which was never true of either site that sets it — both
	// streamAndApplyCredits and streamAndApplyFiles set it to len(reqs).
	// http-api.md §5.3 has said the true thing since LS-270 recorded the
	// mismatch; LS-288 fixed the comment.
	//
	// No upstream total is available to put here. Kavita reports its own item
	// total in a `Pagination` header that is middleware and absent from its
	// OpenAPI document, which is why the `containers`, `items` and `done`
	// phases send no total at all.
	//
	// omitempty, so 0 is absent on the wire and absent means UNKNOWN — not
	// zero, and not a denominator to fall back on. A client must render it as
	// unknown.
	Total int `json:"total,omitempty"`
}

// ProgressFunc receives progress. It is a plain callback rather than a
// dependency on internal/httpapi's Hub, for one reason: ARCHITECTURE.md §2.3
// rule 1 keeps browser-facing handlers and outbound clients in separate
// packages, and cmd/usarr is the only place that sees both. cmd/usarr passes
// one line that calls Hub.Publish, which is what makes "report progress over the
// existing SSE channel" cheap rather than a new subsystem.
type ProgressFunc func(Progress)

// Importer runs channel 1 for one instance.
type Importer struct {
	Store  *store.Store
	Source Source
	Log    *slog.Logger

	// UserID owns the libraries this import creates. library is a user-scoped
	// table, so the owner is a caller's fact rather than an assumption made
	// three layers down.
	UserID int64

	// Covers is the cover fetcher for phase D. NIL DISABLES THE PASS, which is
	// the ordinary state for every adapter that has no cover route wired — see
	// fetchCovers, and covers.go for where the pass sits and why.
	Covers PosterFetcher

	// Progress is optional.
	Progress ProgressFunc

	// BatchRows and BatchWindow default to DefaultBatchRows / DefaultBatchWindow.
	BatchRows   int
	BatchWindow time.Duration

	// Now is injectable so tests can pin timestamps.
	Now func() time.Time
}

func (im *Importer) now() time.Time {
	if im.Now != nil {
		return im.Now()
	}
	return time.Now()
}

func (im *Importer) log() *slog.Logger {
	if im.Log != nil {
		return im.Log
	}
	return slog.New(slog.DiscardHandler)
}

func (im *Importer) publish(p Progress) {
	if im.Progress != nil {
		im.Progress(p)
	}
}

// FullImport is channel 1: read one catalogue source end to end and replace what
// the replica knows about it.
//
// # Order, and why each step is where it is
//
//  1. CONTAINERS FIRST. work.kind comes from the container, so nothing can be
//     mapped before the container list is known. It is also where §17.8's
//     library binding happens, which the membership write in every later batch
//     depends on.
//  2. THEN THE STREAM, in batches. Each batch is one BEGIN IMMEDIATE
//     transaction committed at min(BatchRows, BatchWindow).
//  3. THEN CREDITS, if the source reports any — phase B, after the item stream
//     has CLOSED. It is third because a credit is resolved through the
//     service_item_link the item pass wrote, so nothing can be credited before
//     its work exists; and it is after the stream rather than inside it because
//     Kavita reports no creator on the series list, so credits cost one HTTP
//     GET per series and issuing those from inside the stream callback would
//     hold the streaming connection open across all of them
//     (internal/libsync/credits.go carries the endpoint evidence).
//  4. THEN THE FILE WALK, if the source reports one — phase C, after the
//     credits, and third of the three per-item passes for the same reason the
//     credit pass is second: it resolves through the service_item_link the item
//     pass wrote. It runs AFTER the credits rather than before because the
//     rollup's denominator comes off the credit pass's metadata read, so the
//     order that leaves the least time with a numerator and no denominator is
//     this one. It is also the pass that sets work.rollup_dirty, so it must be
//     the last thing that touches a work before the flush.
//  5. THEN THE ROLLUP FLUSH, which is where have_count and the availability
//     blob come from. It is LAST among the writers because it reads what they
//     all left: a child write marks its ancestor dirty in the same transaction
//     and the flush re-aggregates the dirty set once (ARCHITECTURE.md §6.3).
//     Running it before the walk would aggregate over zero file rows — which
//     internal/store/rollup.go refuses to do anyway, in the writer rather than
//     by sequencing, because plans slip.
//  6. THEN THE COVER PASS, if one is configured — phase D, and it is AFTER the
//     rollup flush rather than beside the other two per-item passes. Covers are
//     decoration and the rollup is truth: have_count and the availability blob
//     are what stop the grid rendering 151 crossed circles, and putting a cover
//     fetch per book ahead of them would delay every correct number on the Home
//     screen behind the whole image queue. ARCHITECTURE.md §4.4.1 rule 4 asks
//     for exactly that order — "the grid is never blocked on the image queue".
//     ⚠️ IT CANNOT FAIL AN IMPORT: fetchCovers returns nothing. See covers.go.
//  7. ANALYZE, then last_full_sync_at, ON SUCCESS ONLY. Both are skipped on a
//     failed or partial import: a freshness claim written over half a library is
//     worse than none, because the Services screen renders it as current.
//
// # What a partial import leaves behind
//
// Committed batches STAND. That is not sloppiness, it is the only behaviour
// compatible with streaming: rolling back a 60 MB import because element 41,000
// was malformed would mean nothing is ever imported from a flaky instance. The
// rows that landed are correct rows; last_full_sync_at is what says the set is
// complete, and it is not written. The returned error and the returned Report
// both describe it.
//
// It is a background replication write and takes no access scope; see
// internal/store/catalogue.go's header.
//
// # Why the returns are named
//
// The defer below stamps rep.FinishedAt, and a defer can only reach the value a
// caller receives through a NAMED result. With unnamed returns the `return rep,
// …` copied rep into the result before the defer ran, so every caller got the
// ZERO TIME in FinishedAt and 0 from Duration() — including the success log,
// which printed `duration=0s` for the whole life of this function. Do not
// un-name them.
func (im *Importer) FullImport(ctx context.Context, instanceID int64) (rep Report, err error) {
	rep = Report{InstanceID: instanceID, StartedAt: im.now()}
	// GUARDED, so the success path can stamp the finish before it logs the
	// duration and the logged instant is the one the caller gets, rather than
	// two readings of the clock that differ.
	defer func() {
		if rep.FinishedAt.IsZero() {
			rep.FinishedAt = im.now()
		}
	}()

	bindings, containerRefs, err := im.bindPhase(ctx, instanceID, "full import", &rep)
	if err != nil {
		return rep, err
	}

	var seen []store.LinkRef
	imported, err := im.streamAndApply(ctx, instanceID, bindings, &rep, im.Source.StreamItems, &seen)
	if err != nil {
		return rep, err
	}

	if err := im.streamAndApplyCredits(ctx, instanceID, imported, &rep); err != nil {
		return rep, err
	}

	if err := im.streamAndApplyFiles(ctx, instanceID, imported, &rep); err != nil {
		return rep, err
	}

	// THE SCOPE IS THE OWNER'S, NAMED AT THE CALL SITE RATHER THAN DEFAULTED
	// INSIDE THE FLUSH. work.have_count and work.availability are denormalised
	// columns, so they hold ONE scope's answer; v0.1 has exactly one user and
	// OwnerScope's predicate is `1=1`, so the stored figure and the true figure
	// coincide. Writing it here means the day a second user exists, the
	// question "whose numbers are in that column" has an answer in the code
	// rather than in a reconstruction.
	rollups, err := im.Store.FlushRollups(ctx, store.OwnerScope(im.UserID), 0)
	rep.Rollups = rollups
	if err != nil {
		return rep, fmt.Errorf("full import of service_instance %d: flush rollups: %w", instanceID, err)
	}

	// PHASE D. It takes no error return and therefore has no `if err != nil`
	// arm — the absence is the contract, not an omission. See covers.go.
	im.fetchCovers(ctx, instanceID, imported, &rep)

	// ── CHANNEL 4'S DELETION PASS, and it is here rather than on a timer of its
	// own for one reason: this is the only place in the tree that has just read
	// an upstream's WHOLE list, which is SweepDeletions' uncheckable
	// precondition. §7.4's every-6-h scheduler is not built and this is not it.
	//
	// AFTER the item stream, the two per-item passes and the rollup flush, so
	// nothing downstream of it re-reads a row it tombstoned; and BEFORE
	// rep.Completed, because "read one catalogue source end to end and replace
	// what the replica knows about it" is not done while the replica still shows
	// items the source has stopped reporting.
	//
	// ⚠️ ON A FAILED OR PARTIAL IMPORT IT NEVER RUNS AT ALL. Every earlier
	// return path is an error return, so a stream that died mid-array reaches
	// this line only by not reaching it — which is the whole tombstone-safety
	// argument in §7.4 applied one layer up: half a list read is a list of
	// absences that are not absences.
	deletions, err := im.Store.SweepDeletions(ctx, instanceID, seen, containerRefs, im.now())
	rep.Deletions = deletions
	if err != nil {
		return rep, fmt.Errorf("full import of service_instance %d: %w", instanceID, err)
	}

	rep.Completed = true

	// ANALYZE after a bulk import (reference/sync.md §6 rule 5). It runs on the
	// writer, serialised with the queue, and AFTER the last batch commits so it
	// measures the table the reads will actually see.
	if err := im.Store.Analyze(ctx); err != nil {
		return rep, fmt.Errorf("full import of service_instance %d: %w", instanceID, err)
	}
	if err := im.Store.StampFullSync(ctx, instanceID, rep.StartedAt); err != nil {
		return rep, fmt.Errorf("full import of service_instance %d: %w", instanceID, err)
	}

	// ⚠️ THE ARRIVALS BOOTSTRAP, IMMEDIATELY AFTER THE FRESHNESS STAMP AND
	// DELIBERATELY NOT INSIDE IT. This is what gives channel 3b a cursor position
	// to start from; without it the delta never runs once. It is a SEPARATE write
	// because the two facts are different — see mintArrivalsWatermark, which
	// carries the argument and the reason this landed in the same change as the
	// walk rather than after it.
	if err := im.mintArrivalsWatermark(ctx, instanceID); err != nil {
		return rep, err
	}

	for _, c := range rep.IdentityConflicts {
		// The merge SIGNAL, recorded rather than merged. v0.1 has no work_merge
		// table; see ApplyCatalogueBatch's doc comment for what is done instead.
		detail, err := json.Marshal(map[string]any{
			"source": c.Source, "value": c.Value,
			"existing_work_id": c.ExistingWorkID, "attempted_work_id": c.AttemptedWorkID,
			"resolution": "the existing work keeps the identifier; no merge machinery exists in v0.1",
		})
		if err != nil {
			return rep, fmt.Errorf("full import of service_instance %d: encode conflict: %w", instanceID, err)
		}
		if err := im.Store.RecordSyncReport(ctx, instanceID,
			"identity_conflict", "series", c.RemoteID, string(detail)); err != nil {
			return rep, fmt.Errorf("full import of service_instance %d: %w", instanceID, err)
		}
	}

	im.publish(Progress{
		InstanceID: instanceID, Phase: "done",
		ItemsRead: rep.ItemsRead, Applied: rep.ItemsApplied,
	})
	// STAMPED BEFORE THE LOG, not left to the defer: the defer runs after this
	// statement, so a duration read here off an unstamped field is 0 no matter
	// how the returns are declared.
	rep.FinishedAt = im.now()
	im.log().Info("full import finished",
		"instance_id", instanceID,
		"items", rep.ItemsApplied,
		"works_created", rep.WorksCreated,
		"works_reused", rep.WorksReused,
		"unidentified", rep.Unidentified,
		"people_created", rep.Credits.PeopleCreated,
		"credits_written", rep.Credits.CreditsWritten,
		"years_written", rep.Credits.YearsWritten,
		"files_written", rep.Files.FilesWritten,
		"files_removed", rep.Files.FilesRemoved,
		"editions_created", rep.Files.EditionsCreated,
		"covers_fetched", rep.Covers.Fetched,
		"covers_already_held", rep.Covers.AlreadyHeld,
		"covers_absent", rep.Covers.Absent,
		"covers_forbidden", rep.Covers.Forbidden,
		"covers_failed", rep.Covers.Failed,
		"covers_deferred", rep.Covers.Deferred,
		"cover_bytes", rep.Covers.Bytes,
		"rollups_flushed", rep.Rollups.WorksFlushed,
		"blobs_written", rep.Rollups.BlobsWritten,
		"blobs_withheld", rep.Rollups.BlobsWithheld,
		"totals_offered", rep.Rollups.TotalsOffered,
		"file_read_failures", rep.FileReadFailures,
		"declined_containers", len(rep.DeclinedContainers),
		"skipped_containers", len(rep.SkippedContainers),
		"identity_conflicts", len(rep.IdentityConflicts),
		"ids_reused", rep.IDsReused,
		"links_tombstoned", rep.Deletions.LinksTombstoned,
		"works_tombstoned", rep.Deletions.WorksTombstoned,
		"sources_missing", rep.Deletions.SourcesMissing,
		"libraries_orphaned", rep.Deletions.LibrariesOrphaned,
		"libraries_restored", rep.Deletions.LibrariesRestored,
		"duration", rep.Duration())
	return rep, nil
}

// bindPhase is the container read, the library bind and the declined-container
// record — the head of every catalogue channel rather than of channel 1 alone.
//
// IT IS ONE FUNCTION BECAUSE BOTH CHANNELS MUST BIND IDENTICALLY. A delta that
// bound containers differently from a full import would file the same upstream
// container into a different library depending on which channel reached it
// first, and library membership is the one thing in this pipeline that is not
// idempotent under a different answer.
//
// `what` names the caller in the errors, because "full import of instance 3:
// containers" and "delta sync of instance 3: containers" are the same failure
// reported to two different people pressing two different buttons.
//
// It returns the container REFS the upstream reported alongside the bindings,
// and the two are not the same set: a DECLINED container has no binding and is
// still a container the upstream reported. Channel 4 needs the reported set —
// "UsArr has no kind for this" and "the upstream has stopped reporting this" are
// different facts, and stamping the first as the second would mark a library
// missing because an adapter's mapping changed.
func (im *Importer) bindPhase(
	ctx context.Context, instanceID int64, what string, rep *Report,
) (map[string]store.CatalogueBinding, []string, error) {
	containers, err := im.Source.Containers(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%s of service_instance %d: containers: %w", what, instanceID, err)
	}
	rep.ContainersSeen = len(containers)
	refs := make([]string, 0, len(containers))
	for _, c := range containers {
		refs = append(refs, c.RemoteID)
	}
	for _, c := range containers {
		if c.Kind == "" {
			rep.DeclinedContainers = append(rep.DeclinedContainers, DeclinedContainer{
				RemoteID: c.RemoteID, Name: c.Name, Reason: c.DeclineReason,
			})
		}
	}
	im.publish(Progress{InstanceID: instanceID, Phase: "containers"})

	bindings, skipped, err := im.Store.BindContainers(ctx, instanceID, im.UserID, containers)
	if err != nil {
		return nil, nil, fmt.Errorf("%s of service_instance %d: bind libraries: %w", what, instanceID, err)
	}
	// A container that could not be bound is NOT an import failure. It is one
	// missing library, reported as one missing library — BindContainers has
	// already written the sync_report row inside the bind transaction, so this
	// is the operator-facing half and not the durable record.
	rep.SkippedContainers = skipped
	for _, sk := range skipped {
		im.log().Warn("container skipped: it could not be bound to a library",
			"instance_id", instanceID, "remote_id", sk.RemoteID,
			"name", sk.Name, "reason", sk.Reason)
	}
	for _, b := range bindings {
		if b.Created {
			rep.LibrariesCreated++
		} else {
			rep.LibrariesJoined++
		}
	}

	// A declined container is reported to the operator through sync_report, not
	// only through the return value: an import triggered by a background connect
	// has no caller left to read the Report by the time anyone asks why a Kavita
	// library is missing from the Libraries screen.
	for _, d := range rep.DeclinedContainers {
		detail, err := json.Marshal(map[string]string{"name": d.Name, "reason": d.Reason})
		if err != nil {
			return nil, nil, fmt.Errorf("%s of service_instance %d: encode decline: %w", what, instanceID, err)
		}
		if err := im.Store.RecordSyncReport(ctx, instanceID,
			"container_declined", "library", d.RemoteID, string(detail)); err != nil {
			return nil, nil, fmt.Errorf("%s of service_instance %d: %w", what, instanceID, err)
		}
	}
	return bindings, refs, nil
}

// streamAndApply is the batching loop.
//
// The batch is flushed FROM INSIDE the stream callback. That holds the upstream
// connection open for the duration of one commit, which is the trade streaming
// makes on purpose: the alternative — a channel to a writer goroutine — decouples
// them at the cost of an unbounded queue between a fast reader and a slow disk,
// which is the buffering this whole design exists to avoid. Memory stays bounded
// at BatchRows items.
// It returns the items that reached a COMMITTED batch, as credit requests. Only
// those: a credit is resolved through the service_item_link the item pass wrote,
// so an item from a batch that never committed has nothing to attach to and
// asking Kavita for its metadata would be a round trip that can only be
// discarded.
//
// ⚠️ THIS IS THE ONE UNBOUNDED ALLOCATION IN THE IMPORT AND IT IS BUDGETED
// RATHER THAN HIDDEN. Everything else here holds at most BatchRows items; this
// slice grows with the whole library, because the credit pass CANNOT run inside
// the stream (see FullImport's phase 3) and therefore needs the list to survive
// it. Three short strings per item: the reference library's ~2,000 Kavita series
// is well under a megabyte, and a 100,000-series library is a few megabytes —
// two orders of magnitude below the 200–400 MB peak reference/sync.md §2
// measures for the buffering this design exists to avoid. If that ever stops
// being true the fix is a temp table, not a bigger slice.
// stream is the item-producing half of streamAndApply, taken as a parameter so
// channel 1 and channel 3b share ONE batching loop.
//
// ⚠️ IT IS A PARAMETER RATHER THAN A SECOND COPY OF THE LOOP because the loop is
// where the batch sizing, the partial-delivery contract, the deliberate refusal
// to flush a tail on a failed stream and the child-item exclusion all live. Two
// copies of those four rules is four places for them to drift, silently, and the
// delta and the full import must apply items identically or the delta is a
// second write path for the same rows.
type stream func(ctx context.Context, fn func(store.CatalogueItem) error) (int, error)

// seen, when non-nil, accumulates EVERY link key this loop wrote — children and
// synthesised parents included, which is what separates it from the returned
// []ImportedItem. That slice is the per-item passes' work list and deliberately
// excludes children (see the flush below); a deletion pass fed from it would
// find every `comic_issue` link absent from the read and tombstone the whole
// child side of the catalogue on the first sweep.
//
// ⚠️ IT IS A NILABLE PARAMETER RATHER THAN A FIELD ON Report, AND THAT IS THE
// GUARD. A delta walk returns only what changed, so the set difference against it
// is the entire library; passing nil is how channel 3b says it has no whole-list
// read to reconcile against, and there is no other way for this loop to hand one
// out.
func (im *Importer) streamAndApply(
	ctx context.Context, instanceID int64,
	bindings map[string]store.CatalogueBinding, rep *Report, read stream,
	seen *[]store.LinkRef,
) ([]ImportedItem, error) {
	rows := im.BatchRows
	if rows <= 0 {
		rows = DefaultBatchRows
	}
	window := im.BatchWindow
	if window <= 0 {
		window = DefaultBatchWindow
	}

	batch := make([]store.CatalogueItem, 0, rows)
	batchStarted := im.now()
	var imported []ImportedItem

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := im.Store.ApplyCatalogueBatch(ctx, instanceID, bindings, batch, im.now())
		if err != nil {
			return err
		}
		rep.Add(res)
		rep.ItemsApplied += len(batch)
		rep.Batches++
		// AFTER the commit, never before: these are the items whose links now
		// exist.
		//
		// ⚠️ A CHILD ITEM IS NOT CARRIED FORWARD, and the cover pass is why it is
		// a rule here rather than a filter in one adapter. Everything downstream
		// of this slice is per-item: phase D issues ONE HTTP REQUEST PER ENTRY,
		// and ARCHITECTURE §13 sizes the child side of the catalogue at ~90,000
		// `comic_issue` rows behind ~3,000 series. ADR-0068 states its whole
		// acquisition cost as "Zero extra HTTP", so admitting children here would
		// spend, on the ADR's own numbers, ninety thousand requests it did not
		// authorise. It also keeps this slice — "THE ONE UNBOUNDED ALLOCATION IN
		// THE IMPORT", budgeted above against the number of TOP-LEVEL works — at
		// the size that budget was written for.
		//
		// The three passes lose nothing they could have used: each resolves
		// through an adapter-side map keyed on the upstream id, and no adapter
		// keeps an entry for a child. When one wants to, this is the line that
		// changes, and the budget above is the argument it has to answer.
		for _, it := range batch {
			if seen != nil {
				// BEFORE the child skip, and with the parent's own key beside
				// the item's: applyOneItem writes a link for both, so both are
				// links the read observed.
				*seen = append(*seen, store.LinkRef{
					RemoteKind: it.RemoteKind, RemoteID: it.RemoteID,
				})
				if it.Parent != nil {
					*seen = append(*seen, store.LinkRef{
						RemoteKind: it.Parent.RemoteKind, RemoteID: it.Parent.RemoteID,
					})
				}
			}
			if it.Parent != nil {
				continue
			}
			imported = append(imported, ImportedItem{
				RemoteKind: it.RemoteKind, RemoteID: it.RemoteID, Kind: it.Kind,
				RemoteSubtype: it.RemoteSubtype,
			})
		}
		batch = batch[:0]
		batchStarted = im.now()
		im.publish(Progress{
			InstanceID: instanceID, Phase: "items",
			ItemsRead: rep.ItemsRead, Applied: rep.ItemsApplied,
		})
		return nil
	}

	delivered, streamErr := read(ctx, func(it store.CatalogueItem) error {
		// COUNTED HERE, ONE PER HAND-OVER, and that is the whole point: the
		// flush above publishes rep.ItemsRead, so a counter assigned only after
		// the stream closes makes every frame but the last say "0 items read"
		// while `applied` climbs past it. That frame is not merely imprecise,
		// it is false, and it renders as a stalled import that is running fine.
		rep.ItemsRead++
		batch = append(batch, it)
		if len(batch) >= rows || im.now().Sub(batchStarted) >= window {
			return flush()
		}
		return nil
	})
	// read is reported whether or not the stream failed: it is the adapter's
	// partial-delivery contract, and a report that hid it would say "0 items"
	// about an import that wrote 40,000.
	//
	// It still OVERWRITES the running count above rather than being dropped for
	// it, because the two legitimately differ: KavitaSource.StreamItems returns
	// what StreamSeries handed IT, which counts series in a declined library
	// that never reach fn at all. The adapter's number stays the Report's
	// documented meaning — "how far the READ got" — and the running count exists
	// to make the intermediate frames truthful, not to redefine the field.
	rep.ItemsRead = delivered
	if streamErr != nil {
		// The tail is deliberately NOT flushed on a failed stream. A partial
		// batch from a body that was cut mid-array is not known-good data, and
		// the rows already committed are enough for the sweep to reconcile from.
		return imported, fmt.Errorf("full import of service_instance %d: read items (delivered %d, applied %d): %w",
			instanceID, delivered, rep.ItemsApplied, streamErr)
	}
	if err := flush(); err != nil {
		return imported, fmt.Errorf("full import of service_instance %d: final batch (delivered %d, applied %d): %w",
			instanceID, delivered, rep.ItemsApplied, err)
	}
	return imported, nil
}

// streamAndApplyCredits is phase B: one metadata read per imported item, batched
// into the same writer on the same sizing as the item pass.
//
// IT IS A NO-OP FOR A SOURCE THAT REPORTS NO CREDITS, and that is principle 3 —
// "every feature degrades honestly when a service is absent" — applied to an
// adapter. A Source that does not implement CreditSource is not an error and is
// not logged as one: the Sonarr and Radarr adapters will not implement it, and a
// Kavita whose operator never filled in any ComicInfo returns empty arrays,
// which reaches the store as "this series has no credits" rather than as
// nothing.
func (im *Importer) streamAndApplyCredits(
	ctx context.Context, instanceID int64, items []ImportedItem, rep *Report,
) error {
	src, ok := im.Source.(CreditSource)
	if !ok || len(items) == 0 {
		return nil
	}
	reqs := make([]CreditRequest, 0, len(items))
	for _, it := range items {
		reqs = append(reqs, it.creditRequest())
	}

	rows := im.BatchRows
	if rows <= 0 {
		rows = DefaultBatchRows
	}
	window := im.BatchWindow
	if window <= 0 {
		window = DefaultBatchWindow
	}

	batch := make([]store.CreditSet, 0, rows)
	batchStarted := im.now()

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := im.Store.ApplyCredits(ctx, instanceID, batch, im.now())
		if err != nil {
			return err
		}
		rep.Credits.Add(res)
		batch = batch[:0]
		batchStarted = im.now()
		im.publish(Progress{
			InstanceID: instanceID, Phase: "credits",
			ItemsRead: rep.CreditItemsRead, Applied: rep.Credits.CreditsWritten,
			Total: len(reqs),
		})
		return nil
	}

	read, streamErr := src.StreamCredits(ctx, reqs, func(set store.CreditSet) error {
		// Counted per hand-over so the credits frames climb; see the item pass.
		rep.CreditItemsRead++
		batch = append(batch, set)
		if len(batch) >= rows || im.now().Sub(batchStarted) >= window {
			return flush()
		}
		return nil
	})
	// The adapter's own count wins at the end, on the item pass's terms.
	rep.CreditItemsRead = read
	if streamErr != nil {
		// Same rule as the item pass: the tail of a failed read is not
		// known-good and is not flushed. What differs is what a failure MEANS
		// here — the works are already imported and correct, and only their
		// credits are incomplete. The error still propagates, because
		// last_full_sync_at must not be stamped over a partial import.
		return fmt.Errorf("full import of service_instance %d: read credits (delivered %d): %w",
			instanceID, read, streamErr)
	}
	if err := flush(); err != nil {
		return fmt.Errorf("full import of service_instance %d: final credit batch (delivered %d): %w",
			instanceID, read, err)
	}
	return nil
}

// streamAndApplyFiles is phase C: one volume walk per imported item, batched
// into the same writer on the same sizing as the two passes before it.
//
// IT IS A NO-OP FOR A SOURCE THAT REPORTS NO FILES, on streamAndApplyCredits's
// reasoning: a Source that does not implement FileSource is not an error and is
// not logged as one.
//
// ⚠️ THE TAIL OF A FAILED WALK IS NOT FLUSHED, and here that is not merely
// consistency with the other two passes — it is what stops a partial read
// deleting rows. The store reconciles by deleting a work's files that the pass
// did not report, so a batch assembled from a walk that died halfway would
// present a truncated file list as the complete one. Dropping it leaves the
// previous rows standing, which is the state a later import corrects.
func (im *Importer) streamAndApplyFiles(
	ctx context.Context, instanceID int64, items []ImportedItem, rep *Report,
) error {
	src, ok := im.Source.(FileSource)
	if !ok || len(items) == 0 {
		return nil
	}
	reqs := make([]FileRequest, 0, len(items))
	for _, it := range items {
		reqs = append(reqs, it.fileRequest())
	}

	rows := im.BatchRows
	if rows <= 0 {
		rows = DefaultBatchRows
	}
	window := im.BatchWindow
	if window <= 0 {
		window = DefaultBatchWindow
	}

	batch := make([]store.FileSet, 0, rows)
	batchStarted := im.now()

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := im.Store.ApplyFiles(ctx, instanceID, batch)
		if err != nil {
			return err
		}
		rep.Files.Add(res)
		batch = batch[:0]
		batchStarted = im.now()
		im.publish(Progress{
			InstanceID: instanceID, Phase: "files",
			ItemsRead: rep.FileItemsRead, Applied: rep.Files.FilesWritten,
			Total: len(reqs),
		})
		return nil
	}

	read, failed, streamErr := src.StreamFiles(ctx, reqs, func(set store.FileSet) error {
		// Counted per hand-over so the files frames climb; see the item pass.
		rep.FileItemsRead++
		batch = append(batch, set)
		if len(batch) >= rows || im.now().Sub(batchStarted) >= window {
			return flush()
		}
		return nil
	})
	// The adapter's own count wins at the end, on the item pass's terms.
	rep.FileItemsRead = read
	rep.FileReadFailures = len(failed)
	// RECORDED BEFORE streamErr IS HANDLED, and before the tail flush, because
	// these are facts about items the walk already gave up on: a breaker that
	// opened at series 90 does not make the six failures before it less true.
	if err := im.recordFileWalkFailures(ctx, instanceID, failed); err != nil {
		return err
	}
	if streamErr != nil {
		return fmt.Errorf("full import of service_instance %d: walk files (delivered %d): %w",
			instanceID, read, streamErr)
	}
	if err := flush(); err != nil {
		return fmt.Errorf("full import of service_instance %d: final file batch (delivered %d): %w",
			instanceID, read, err)
	}
	return nil
}

// recordFileWalkFailures writes the durable half of a dropped file walk: one
// sync_report row per item the walk could not read.
//
// ⚠️ THE Report COUNTER IS NOT THE RECORD. An import triggered by a background
// connect has no caller left to read the Report by the time anyone asks why a
// series shows no count — the same argument FullImport makes for writing
// `container_declined` rows rather than only returning DeclinedContainers.
//
// THE KIND IS `file_walk_failed`, on `container_bind_failed`'s pattern:
// <the pass that failed>_failed, with remote_kind/remote_id naming the subject.
// sync_report.kind has no CHECK on purpose (migration 00005), so the vocabulary
// is held by convention and by the readers — store.SyncReportFileWalkFailed is
// the one spelling, shared with the read.
//
// ⚠️ detail CARRIES NO UPSTREAM TEXT. schema.md's column comment says the column
// is redacted on the way in; walkFailureReason makes that stronger by never
// producing text to redact. `reason` is its closed vocabulary and `status` is an
// integer, so there is nothing here for ssrf.RedactText to find and no path by
// which a Kavita response body could reach the row.
//
// ONE WRITE TRANSACTION PER ROW, which is what RecordSyncReport offers and what
// the `container_declined` loop above already does. It is bounded by the number
// of items the walk failed on, and the ordinary case is zero. The seam if a
// whole large library ever fails at once is a batched writer in internal/store,
// not a cap here: a cap would make the count on the Services screen wrong, which
// is the exact failure this commit exists to remove.
func (im *Importer) recordFileWalkFailures(
	ctx context.Context, instanceID int64, failed []FileWalkFailure,
) error {
	for _, f := range failed {
		detail, err := json.Marshal(map[string]any{
			"phase":  "files",
			"reason": f.Reason,
			"status": f.Status,
			// Named rather than implied, because it is the surprising half: a
			// failed read is not evidence of an empty library and must not be
			// reconciled as one. See StreamFiles.
			"effect": "the item keeps the file rows it already had; none were removed",
		})
		if err != nil {
			return fmt.Errorf("full import of service_instance %d: encode file walk failure: %w",
				instanceID, err)
		}
		if err := im.Store.RecordSyncReport(ctx, instanceID,
			store.SyncReportFileWalkFailed, f.RemoteKind, f.RemoteID, string(detail)); err != nil {
			return fmt.Errorf("full import of service_instance %d: %w", instanceID, err)
		}
	}
	return nil
}
