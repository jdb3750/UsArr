package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
	"github.com/jdb3750/UsArr/internal/store"
)

// sweeperTestApp builds the minimum app the sweeper touches: a real migrated
// database and a real store. Nothing here needs the HTTP server, the registry or
// the keyring, and wiring them in would make the test about startup rather than
// about the sweep.
func sweeperTestApp(t *testing.T) *app {
	t.Helper()
	cfg := testConfig(t)
	database, err := db.Open(t.Context(), cfg.DatabasePath())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return &app{
		cfg:   cfg,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		db:    database,
		store: store.New(database),
	}
}

// seedCandidates inserts one row that has already expired and one that has not,
// and returns their ids in that order.
func seedCandidates(t *testing.T, a *app) (expired, live int64) {
	t.Helper()
	ctx := t.Context()

	instanceID, err := a.store.CreateServiceInstance(ctx, store.ServiceInstance{
		Kind: "prowlarr", Role: "indexer", Name: "Prowlarr",
		BaseURL: "http://prowlarr.lan:9696", APIKeyEnc: []byte{1}, KEKID: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("seed service_instance: %v", err)
	}

	now := time.Now()
	ids, err := a.store.InsertReleaseCandidates(ctx, []store.ReleaseCandidate{
		{
			ServiceInstanceID: instanceID, GUID: "stale", Title: "Stale.Release",
			RawReleaseJSON: `{"guid":"stale"}`,
			FetchedAt:      now.Add(-time.Hour), ExpiresAt: now.Add(-30 * time.Minute),
		},
		{
			ServiceInstanceID: instanceID, GUID: "fresh", Title: "Fresh.Release",
			RawReleaseJSON: `{"guid":"fresh"}`,
			FetchedAt:      now, ExpiresAt: now.Add(25 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("seed release_candidate: %v", err)
	}
	return ids[0], ids[1]
}

func candidateExists(t *testing.T, a *app, id int64) bool {
	t.Helper()
	// A zero `now` means "do not judge expiry", so this asks only whether the
	// ROW is still there — which is the whole question the sweep answers.
	_, err := a.store.GetReleaseCandidate(t.Context(), store.Scope{AllInstances: true}, id, time.Time{})
	return err == nil
}

// TestCandidateSweeperRemovesExpiredRows is the regression test for
// release_candidate growing without bound.
//
// Store.ExpireReleaseCandidates and releases.Service.EvictExpired both existed
// and neither had a production caller, so nothing in the running binary ever
// deleted a candidate. reference/schema.md §6 argues that duplicate rows are
// harmless BECAUSE the TTL is swept; without a sweeper that argument is false
// and the table grows for the life of the process.
func TestCandidateSweeperRemovesExpiredRows(t *testing.T) {
	a := sweeperTestApp(t)
	expired, live := seedCandidates(t, a)

	a.sweepCandidatesOnce(t.Context())

	if candidateExists(t, a, expired) {
		t.Error("an expired release_candidate survived the sweep — the table grows without bound")
	}
	if !candidateExists(t, a, live) {
		t.Error("the sweep deleted a candidate that was still grabbable")
	}
}

// TestCandidateSweeperSweepsOnStartAndStopsOnCancel covers the loop rather than
// the statement: the first pass must not wait a full interval (a process that
// was down comes back to a table full of rows that expired while it was gone),
// and cancelling the context must actually end the goroutine so shutdown does
// not hang and nothing is still writing when the database closes.
func TestCandidateSweeperSweepsOnStartAndStopsOnCancel(t *testing.T) {
	a := sweeperTestApp(t)
	expired, _ := seedCandidates(t, a)

	ctx, cancel := context.WithCancel(context.Background())
	done := a.startCandidateSweeper(ctx)

	// The immediate pass, without waiting out candidateSweepInterval.
	deadline := time.Now().Add(5 * time.Second)
	for candidateExists(t, a, expired) {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("the sweeper did not run a pass at start")
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the sweeper did not stop when its context was cancelled; " +
			"shutdown would hang and the database would close under a live writer")
	}
}
