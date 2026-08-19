package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// posterFixture writes one item, one work and (optionally) a poster on it.
func posterFixture(t *testing.T, s *Store, instanceID int64, remoteID string, withPoster bool) {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

	bindings, _, err := s.BindContainers(ctx, instanceID, SystemUserID,
		[]CatalogueContainer{{RemoteID: "1", Name: "Fiction", Kind: "book"}})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	if _, err := s.ApplyCatalogueBatch(ctx, instanceID, bindings, []CatalogueItem{{
		RemoteID: remoteID, RemoteKind: "book", ContainerID: "1", Kind: "book",
		Title: "T" + remoteID, SortTitle: "T" + remoteID, NormalizedTitle: "t" + remoteID,
		NormVersion: 1, AddedAt: now, RemoteUpdatedAt: now, HasFile: true,
	}}, now); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}
	if !withPoster {
		return
	}
	url := "http://bookorbit.test/api/v1/books/" + remoteID + "/cover"
	if _, err := s.PutPosterAsset(ctx, PosterAsset{
		RemoteKind: "book", RemoteID: remoteID, SourceURL: url,
		CacheKey: fmt.Sprintf("%016x", instanceID*100000+int64(len(remoteID))*1000+int64(remoteID[0])*7+int64(len(url))), Format: ImageFormatJPEG,
		OriginClass: ImageOriginConfigured, ServiceInstanceID: instanceID,
		Width: 500, Height: 750, FetchedAt: now,
	}); err != nil {
		t.Fatalf("PutPosterAsset: %v", err)
	}
}

// TestPosteredItemsReturnsOnlyItemsWhoseWorkHasAPoster is the idempotence read's
// drill.
//
// ⚠️ IT IS TWO-SIDED. A read that returned NOTHING would make internal/libsync's
// cover pass re-fetch every cover on every import — expensive but invisible —
// and a read that returned EVERYTHING would make it fetch none at all, which is
// a library that never gets artwork. Both directions are asserted.
func TestPosteredItemsReturnsOnlyItemsWhoseWorkHasAPoster(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "bookorbit")

	posterFixture(t, s, inst, "1", true)
	posterFixture(t, s, inst, "2", false)

	got, err := s.PosteredItems(t.Context(), inst)
	if err != nil {
		t.Fatalf("PosteredItems: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("PosteredItems returned %d rows, want 1: %+v", len(got), got)
	}
	if got[0].RemoteID != "1" || got[0].RemoteKind != "book" {
		t.Errorf("row = %+v, want the book with a poster, keyed by (kind, remote id)", got[0])
	}
	if got[0].ServiceInstanceID != inst {
		t.Errorf("ServiceInstanceID = %d, want %d — a remote id with no instance is not addressable",
			got[0].ServiceInstanceID, inst)
	}
}

// TestPosteredItemsIsScopedToOneInstance. Remote ids are only unique PER
// INSTANCE (ux_sil), so a read that leaked another instance's rows would make
// the cover pass skip a book because a DIFFERENT service happened to use the
// same id.
func TestPosteredItemsIsScopedToOneInstance(t *testing.T) {
	s := newTestStore(t)
	a := fixtureInstance(t, s, "one")
	b, err := s.CreateServiceInstance(t.Context(), ServiceInstance{
		Kind: "bookorbit", Role: "library", Name: "other",
		BaseURL: "http://other.test", APIKeyEnc: []byte{1}, KEKID: 1,
		Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}

	posterFixture(t, s, a, "1", true)
	posterFixture(t, s, b, "1", false)

	got, err := s.PosteredItems(t.Context(), b)
	if err != nil {
		t.Fatalf("PosteredItems: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("instance %d got %d rows, want 0 — instance %d's poster on the same "+
			"remote id leaked across: %+v", b, len(got), a, got)
	}
	if got, err := s.PosteredItems(t.Context(), a); err != nil || len(got) != 1 {
		t.Fatalf("instance %d: %d rows, err %v, want 1 row", a, len(got), err)
	}
}

// TestPosteredItemsSeeksTheIndexRatherThanScanningEveryWork.
//
// CLAUDE.md puts query-plan assertions in the gate. Migration 00010's
// ix_work_poster is PARTIAL — `WHERE poster_asset_id IS NOT NULL` — and this
// read's predicate is written to match it, so the planner reaches the postered
// set through the index instead of walking `work`. On a 100,000-book library the
// difference is the whole cost of the pass's first step.
func TestPosteredItemsSeeksTheIndexRatherThanScanningEveryWork(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "bookorbit")
	for i := 1; i <= 300; i++ {
		posterFixture(t, s, inst, fmt.Sprint(i), i%2 == 0)
	}
	if err := s.Analyze(t.Context()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	rows, err := s.DB().Read().QueryContext(context.Background(), `
		EXPLAIN QUERY PLAN
		SELECT sil.service_instance_id, sil.remote_kind, sil.remote_id
		  FROM service_item_link sil
		  JOIN work w ON w.id = sil.work_id
		 WHERE sil.service_instance_id = ?
		   AND sil.deleted_at IS NULL
		   AND w.deleted_at IS NULL
		   AND w.poster_asset_id IS NOT NULL`, inst)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var plan string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	// A FLOOR, NOT A PIN. What must never happen is a full table scan of either
	// side; which index SQLite picks between ux_sil and ix_work_poster is its
	// business and depends on ANALYZE's statistics.
	if plan == "" {
		t.Fatal("EXPLAIN QUERY PLAN returned nothing; the assertion below would be vacuous")
	}
	// ⚠️ THE ALIASES, NOT THE TABLE NAMES. SQLite reports "SCAN sil", never
	// "SCAN service_item_link", and a guard written against the table name is
	// one that can never fire — measured: with the index predicates deliberately
	// defeated the plan reads "SCAN sil" and the table-name form stayed green.
	//
	// The measured plan at 300 items (150 postered) is:
	//
	//	SEARCH w USING INDEX ix_work_poster (poster_asset_id>?)
	//	SEARCH sil USING INDEX ix_sil_work (work_id=?)
	//
	// Both seeks. The fixture is 300 rather than 1 for the same reason: at one
	// row SQLite picks "SCAN sil" legitimately, so a one-row fixture would pin
	// an artefact of an empty table rather than the plan a library gets.
	for _, bad := range []string{"SCAN sil", "SCAN w"} {
		if strings.Contains(plan, bad) {
			t.Errorf("the plan contains %q, which is a full table scan:\n%s", bad, plan)
		}
	}
}
