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
	FinishedAt time.Time

	ContainersSeen     int
	LibrariesCreated   int
	LibrariesJoined    int
	DeclinedContainers []DeclinedContainer

	// ItemsRead is what the adapter handed over. It is "how far the read got",
	// never "how many rows are correct".
	ItemsRead int

	// ItemsApplied is how many reached a committed batch. On a stream that died
	// mid-array it is smaller than ItemsRead by at most one batch.
	ItemsApplied int

	Batches int

	store.BatchResult

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
	Phase      string `json:"phase"` // containers | items | done
	ItemsRead  int    `json:"items_read"`
	Applied    int    `json:"applied"`

	// Total is the upstream's own total when it reported one, and 0 when it did
	// not. Kavita's `Pagination` header is middleware and is not in the OpenAPI
	// document, so "unknown" is a state a client must render as unknown rather
	// than as zero.
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
//  3. ANALYZE, then last_full_sync_at, ON SUCCESS ONLY. Both are skipped on a
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
func (im *Importer) FullImport(ctx context.Context, instanceID int64) (Report, error) {
	rep := Report{InstanceID: instanceID, StartedAt: im.now()}
	defer func() { rep.FinishedAt = im.now() }()

	containers, err := im.Source.Containers(ctx)
	if err != nil {
		return rep, fmt.Errorf("full import of service_instance %d: containers: %w", instanceID, err)
	}
	rep.ContainersSeen = len(containers)
	for _, c := range containers {
		if c.Kind == "" {
			rep.DeclinedContainers = append(rep.DeclinedContainers, DeclinedContainer{
				RemoteID: c.RemoteID, Name: c.Name, Reason: c.DeclineReason,
			})
		}
	}
	im.publish(Progress{InstanceID: instanceID, Phase: "containers"})

	bindings, err := im.Store.BindContainers(ctx, instanceID, im.UserID, containers)
	if err != nil {
		return rep, fmt.Errorf("full import of service_instance %d: bind libraries: %w", instanceID, err)
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
			return rep, fmt.Errorf("full import of service_instance %d: encode decline: %w", instanceID, err)
		}
		if err := im.Store.RecordSyncReport(ctx, instanceID,
			"container_declined", "library", d.RemoteID, string(detail)); err != nil {
			return rep, fmt.Errorf("full import of service_instance %d: %w", instanceID, err)
		}
	}

	if err := im.streamAndApply(ctx, instanceID, bindings, &rep); err != nil {
		return rep, err
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
	im.log().Info("full import finished",
		"instance_id", instanceID,
		"items", rep.ItemsApplied,
		"works_created", rep.WorksCreated,
		"works_reused", rep.WorksReused,
		"unidentified", rep.Unidentified,
		"declined_containers", len(rep.DeclinedContainers),
		"identity_conflicts", len(rep.IdentityConflicts),
		"duration", rep.Duration())
	return rep, nil
}

// streamAndApply is the batching loop.
//
// The batch is flushed FROM INSIDE the stream callback. That holds the upstream
// connection open for the duration of one commit, which is the trade streaming
// makes on purpose: the alternative — a channel to a writer goroutine — decouples
// them at the cost of an unbounded queue between a fast reader and a slow disk,
// which is the buffering this whole design exists to avoid. Memory stays bounded
// at BatchRows items.
func (im *Importer) streamAndApply(
	ctx context.Context, instanceID int64,
	bindings map[string]store.CatalogueBinding, rep *Report,
) error {
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

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := im.Store.ApplyCatalogueBatch(ctx, instanceID, bindings, batch, im.now())
		if err != nil {
			return err
		}
		rep.BatchResult.Add(res)
		rep.ItemsApplied += len(batch)
		rep.Batches++
		batch = batch[:0]
		batchStarted = im.now()
		im.publish(Progress{
			InstanceID: instanceID, Phase: "items",
			ItemsRead: rep.ItemsRead, Applied: rep.ItemsApplied,
		})
		return nil
	}

	read, streamErr := im.Source.StreamItems(ctx, func(it store.CatalogueItem) error {
		batch = append(batch, it)
		if len(batch) >= rows || im.now().Sub(batchStarted) >= window {
			return flush()
		}
		return nil
	})
	// read is reported whether or not the stream failed: it is the adapter's
	// partial-delivery contract, and a report that hid it would say "0 items"
	// about an import that wrote 40,000.
	rep.ItemsRead = read
	if streamErr != nil {
		// The tail is deliberately NOT flushed on a failed stream. A partial
		// batch from a body that was cut mid-array is not known-good data, and
		// the rows already committed are enough for the sweep to reconcile from.
		return fmt.Errorf("full import of service_instance %d: read items (delivered %d, applied %d): %w",
			instanceID, read, rep.ItemsApplied, streamErr)
	}
	if err := flush(); err != nil {
		return fmt.Errorf("full import of service_instance %d: final batch (delivered %d, applied %d): %w",
			instanceID, read, rep.ItemsApplied, err)
	}
	return nil
}
