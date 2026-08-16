package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

func newIndexerFixture(t *testing.T) (*Store, int64) {
	t.Helper()
	s := newTestStore(t)
	id, err := s.CreateServiceInstance(t.Context(), ServiceInstance{
		Kind: "prowlarr", Role: "indexer", Name: "Prowlarr",
		BaseURL: "http://prowlarr.internal:9696", APIKeyEnc: []byte{0}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	return s, id
}

func sampleIndexers(instanceID int64) []Indexer {
	return []Indexer{
		{
			ServiceInstanceID: instanceID, IndexerID: 2, Name: "Beta Tracker",
			Protocol: "torrent", Privacy: "public", Enabled: true, Priority: 30,
			SupportsSearch: true, SupportsRSS: true,
			SearchTypesJSON: `["search"]`,
			CategoriesJSON:  `[{"id":5000,"name":"TV"}]`,
		},
		{
			ServiceInstanceID: instanceID, IndexerID: 1, Name: "Alpha Tracker",
			Protocol: "torrent", Privacy: "private", Enabled: true, Priority: 25,
			SupportsSearch: true, SupportsRSS: true, SupportsPagination: true,
			SearchTypesJSON: `["search","movie"]`,
			CategoriesJSON:  `[{"id":2000,"name":"Movies"}]`,
			LimitsMax:       sql.NullInt64{Int64: 100, Valid: true},
			LimitsDefault:   sql.NullInt64{Int64: 50, Valid: true},
		},
	}
}

func TestReplaceIndexersRoundTrips(t *testing.T) {
	s, instanceID := newIndexerFixture(t)
	ctx := t.Context()

	if err := s.ReplaceIndexers(ctx, instanceID, sampleIndexers(instanceID), testNow); err != nil {
		t.Fatalf("ReplaceIndexers: %v", err)
	}

	got, err := s.ListIndexers(ctx, OwnerScope(1), instanceID)
	if err != nil {
		t.Fatalf("ListIndexers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d indexers, want 2", len(got))
	}
	// Name order, not insertion order: the picker renders alphabetically and
	// ix_indexer_catalog_instance is what supplies it.
	if got[0].Name != "Alpha Tracker" || got[1].Name != "Beta Tracker" {
		t.Fatalf("not in name order: %q, %q", got[0].Name, got[1].Name)
	}

	alpha := got[0]
	if alpha.IndexerID != 1 || alpha.Priority != 25 || !alpha.SupportsPagination {
		t.Errorf("alpha round-tripped wrong: %+v", alpha)
	}
	if !alpha.LimitsMax.Valid || alpha.LimitsMax.Int64 != 100 {
		t.Errorf("limits_max lost: %+v", alpha.LimitsMax)
	}
	if alpha.SearchTypesJSON != `["search","movie"]` {
		t.Errorf("search_types lost: %q", alpha.SearchTypesJSON)
	}
	if !alpha.FetchedAt.Equal(testNow) {
		t.Errorf("fetched_at = %v, want %v", alpha.FetchedAt, testNow)
	}

	// NULL and 0 are different: Beta advertised no limits, and recording 0
	// would read as "this indexer returns nothing".
	if got[1].LimitsMax.Valid || got[1].LimitsDefault.Valid {
		t.Errorf("beta invented limits it never advertised: %+v", got[1])
	}
}

// Upstream is authoritative. An indexer deleted in Prowlarr must disappear from
// the picker rather than linger as a filter that silently matches nothing.
func TestReplaceIndexersIsWholesale(t *testing.T) {
	s, instanceID := newIndexerFixture(t)
	ctx := t.Context()

	if err := s.ReplaceIndexers(ctx, instanceID, sampleIndexers(instanceID), testNow); err != nil {
		t.Fatalf("first ReplaceIndexers: %v", err)
	}
	later := testNow.Add(time.Hour)
	if err := s.ReplaceIndexers(ctx, instanceID, []Indexer{{
		ServiceInstanceID: instanceID, IndexerID: 1, Name: "Alpha Tracker", Enabled: false,
	}}, later); err != nil {
		t.Fatalf("second ReplaceIndexers: %v", err)
	}

	got, err := s.ListIndexers(ctx, OwnerScope(1), instanceID)
	if err != nil {
		t.Fatalf("ListIndexers: %v", err)
	}
	if len(got) != 1 || got[0].IndexerID != 1 {
		t.Fatalf("the removed indexer survived: %+v", got)
	}
	if got[0].Enabled {
		t.Error("the refreshed row kept the old enabled flag")
	}
	// The defaults fill in rather than the columns going empty: a reader must
	// never see a NULL protocol or a null JSON array.
	if got[0].Protocol != "unknown" || got[0].SearchTypesJSON != "[]" || got[0].CategoriesJSON != "[]" {
		t.Errorf("defaults not applied: %+v", got[0])
	}
	if !got[0].FetchedAt.Equal(later) {
		t.Errorf("fetched_at = %v, want the refresh time %v", got[0].FetchedAt, later)
	}
}

// "Answered and has zero indexers" is a real state, and it must be
// distinguishable from "never successfully read". Zero rows cannot tell them
// apart, so the timestamp is stamped on the INSTANCE and is stamped even when
// the list came back empty.
func TestReplaceIndexersStampsTheFetchTimeEvenWhenEmpty(t *testing.T) {
	s, instanceID := newIndexerFixture(t)
	ctx := t.Context()

	before, err := s.GetServiceInstance(ctx, OwnerScope(1), instanceID)
	if err != nil {
		t.Fatalf("GetServiceInstance: %v", err)
	}
	if before.IndexersFetchedAt.Valid {
		t.Fatal("a fresh instance must report that its indexer list has never been read")
	}

	if err := s.ReplaceIndexers(ctx, instanceID, nil, testNow); err != nil {
		t.Fatalf("ReplaceIndexers with an empty list: %v", err)
	}

	after, err := s.GetServiceInstance(ctx, OwnerScope(1), instanceID)
	if err != nil {
		t.Fatalf("GetServiceInstance: %v", err)
	}
	if !after.IndexersFetchedAt.Valid {
		t.Fatal("an empty-but-successful fetch must still stamp indexers_fetched_at, " +
			"or 'this service has no indexers' is indistinguishable from 'we never asked'")
	}
	if after.IndexersFetchedAt.String != FormatTime(testNow) {
		t.Errorf("indexers_fetched_at = %q, want %q", after.IndexersFetchedAt.String, FormatTime(testNow))
	}
}

// The scope is not decoration. A caller that can see nothing reads nothing —
// failing CLOSED — rather than reading every instance's indexers, which would
// be an existence oracle for services the caller cannot see.
func TestIndexerReadFailsClosedOutsideScope(t *testing.T) {
	s, instanceID := newIndexerFixture(t)
	ctx := t.Context()
	if err := s.ReplaceIndexers(ctx, instanceID, sampleIndexers(instanceID), testNow); err != nil {
		t.Fatalf("ReplaceIndexers: %v", err)
	}

	// AllInstances false with an empty visible set: sees nothing.
	blind, err := s.ListIndexers(ctx, Scope{UserID: 2}, instanceID)
	if err != nil {
		t.Fatalf("ListIndexers: %v", err)
	}
	if len(blind) != 0 {
		t.Fatalf("a scope that can see no instance read %d indexers", len(blind))
	}

	// A scope naming a DIFFERENT instance likewise reads nothing.
	other, err := s.ListIndexers(ctx, Scope{UserID: 2, InstanceIDs: []int64{instanceID + 99}}, instanceID)
	if err != nil {
		t.Fatalf("ListIndexers: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("a scope naming another instance read %d indexers", len(other))
	}

	// And the scope that DOES name it reads them, so the test above is not
	// passing because the write failed.
	visible, err := s.ListIndexers(ctx, Scope{UserID: 2, InstanceIDs: []int64{instanceID}}, instanceID)
	if err != nil {
		t.Fatalf("ListIndexers: %v", err)
	}
	if len(visible) != 2 {
		t.Fatalf("the in-scope read returned %d indexers, want 2", len(visible))
	}
}

// The catalogue read is on a render path — it is what the Requests screen's
// indexer and category picker will read (§17.5, not Search), and that picker
// paints before anything is searched — so it must not scan and must not sort.
//
// It EXPLAINs the statement indexerListSQL actually issues rather than a copy
// pasted into the test, which is how a query-plan assertion silently stops
// covering the query it names.
func TestIndexerCatalogReadUsesTheInstanceIndex(t *testing.T) {
	s, instanceID := newIndexerFixture(t)
	query, args := indexerListSQL(OwnerScope(1), instanceID)

	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")

	if !strings.Contains(joined, "ix_indexer_catalog_instance") {
		t.Fatalf("the catalogue read does not use ix_indexer_catalog_instance:\n  %s\n"+
			"Migration 0004 added that index for this read and nothing else uses it.", joined)
	}
	if !strings.Contains(joined, "SEARCH") {
		t.Errorf("the plan is not a SEARCH:\n  %s", joined)
	}
	if strings.Contains(joined, "SCAN indexer_catalog") {
		t.Errorf("the plan scans the catalogue:\n  %s", joined)
	}
	// name is the index's second column precisely so the ORDER BY comes from
	// it. A temp b-tree here means the index lost half its job.
	if strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("the plan needs a temp b-tree for ORDER BY name:\n  %s\n"+
			"ix_indexer_catalog_instance carries name for exactly this reason.", joined)
	}
}

// TestReadIndexerCatalogIsOneSnapshot is the regression guard for a torn read
// that shipped on GET /api/v1/indexers.
//
// ReplaceIndexers stamps service_instance.indexers_fetched_at and rewrites
// indexer_catalog in ONE transaction, so no reader can see one without the
// other. The endpoint then read them with TWO statements off the read pool —
// two WAL snapshots — and a replication commit landing between them produced a
// response that said "UsArr has not yet read the indexer list from Prowlarr"
// while listing the three indexers it had just read. It reproduced at roughly
// 1 request in 100 on a loaded four-core box, which is exactly the frequency
// that gets a test re-run instead of read.
//
// THE INVARIANT IS ONE-DIRECTIONAL, and that is what makes it assertable:
// indexers_fetched_at is written in the same transaction as the rows and is
// never cleared, so ROWS PRESENT ⟹ FETCHED_AT VALID, always. The reverse is a
// legitimate state (a service that answered and has no indexers), so it is not
// checked.
//
// It hammers rather than orchestrates because the window is a commit landing
// between two statements and there is no hook to stop the reader in it. A fresh
// instance per round is what makes each round a real NULL → stamped transition:
// nothing clears indexers_fetched_at once set, so a reused instance would only
// test the first round. Run against the two-statement version this fails within
// a few hundred rounds; the point of the loop is that it cannot pass vacuously.
func TestReadIndexerCatalogIsOneSnapshot(t *testing.T) {
	const rounds = 400

	s := newTestStore(t)
	ctx := t.Context()
	scope := OwnerScope(1)

	for round := range rounds {
		instanceID, err := s.CreateServiceInstance(ctx, ServiceInstance{
			Kind: "prowlarr", Role: "indexer", Name: fmt.Sprintf("Prowlarr %d", round),
			BaseURL: "http://prowlarr.internal:9696", APIKeyEnc: []byte{0}, Enabled: true,
		})
		if err != nil {
			t.Fatalf("CreateServiceInstance: %v", err)
		}

		// The writer replicates while readers are already spinning, so the
		// commit lands mid-read rather than before the first one.
		written := make(chan error, 1)
		go func() {
			written <- s.ReplaceIndexers(ctx, instanceID, sampleIndexers(instanceID), testNow)
		}()

		var readers sync.WaitGroup
		for range 4 {
			readers.Add(1)
			go func() {
				defer readers.Done()
				// Bounded: a reader that never sees the rows must not hang the
				// suite, and the writer's own error is reported below anyway.
				for range 20000 {
					cats, err := s.ReadIndexerCatalog(ctx, scope, instanceID)
					if err != nil {
						t.Errorf("round %d: ReadIndexerCatalog: %v", round, err)
						return
					}
					if len(cats) != 1 {
						t.Errorf("round %d: read %d catalogues for one instance", round, len(cats))
						return
					}
					if len(cats[0].Indexers) == 0 {
						continue
					}
					if !cats[0].Instance.IndexersFetchedAt.Valid {
						t.Errorf("round %d: the catalogue carries %d indexers while the instance "+
							"reports indexers_fetched_at NULL. ReplaceIndexers writes both in one "+
							"transaction, so this state never existed in the database — the read "+
							"spanned two snapshots.", round, len(cats[0].Indexers))
					}
					return
				}
			}()
		}
		readers.Wait()
		if err := <-written; err != nil {
			t.Fatalf("round %d: ReplaceIndexers: %v", round, err)
		}
		if t.Failed() {
			return
		}
	}
}

// A read for an instance that has never been replicated is empty and is not an
// error: a picker asking about a service UsArr has not managed to talk to yet
// must get an answer it can render, not a failure.
func TestListIndexersOnAnUnreplicatedInstanceIsEmpty(t *testing.T) {
	s, instanceID := newIndexerFixture(t)
	got, err := s.ListIndexers(t.Context(), OwnerScope(1), instanceID)
	if err != nil {
		t.Fatalf("ListIndexers: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("read %d indexers from an unreplicated instance", len(got))
	}
}

// Deleting a service takes its catalogue with it. The rows are a replica of
// that service's state and mean nothing without it, and the FK is what makes
// that automatic rather than a cleanup step someone has to remember.
func TestDeletingAServiceCascadesToItsCatalogue(t *testing.T) {
	s, instanceID := newIndexerFixture(t)
	ctx := t.Context()
	if err := s.ReplaceIndexers(ctx, instanceID, sampleIndexers(instanceID), testNow); err != nil {
		t.Fatalf("ReplaceIndexers: %v", err)
	}

	// SoftDeleteServiceInstance is a tombstone, not a DELETE, so the rows
	// survive it — the hard delete below is what the FK is for.
	if err := s.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM service_instance WHERE id = ?`, instanceID)
		return err
	}); err != nil {
		t.Fatalf("hard delete: %v", err)
	}

	var n int
	if err := s.DB().Read().QueryRowContext(ctx,
		`SELECT count(*) FROM indexer_catalog WHERE service_instance_id = ?`, instanceID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("%d catalogue rows outlived their service", n)
	}
}
