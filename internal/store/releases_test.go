package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func seedInstance(t *testing.T, s *Store) int64 {
	t.Helper()
	id, err := s.CreateServiceInstance(t.Context(), ServiceInstance{
		Kind: "prowlarr", Role: "indexer", Name: "Prowlarr",
		BaseURL: "http://prowlarr.lan:9696", APIKeyEnc: []byte{1}, KEKID: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("seed service_instance: %v", err)
	}
	return id
}

func TestReleaseCandidateInsertGetExpire(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	instanceID := seedInstance(t, s)
	owner := OwnerScope(1)

	// Search-and-Grab: work_id is NULL because nothing owns the result yet.
	expires := testNow.Add(25 * time.Minute)
	ids, err := s.InsertReleaseCandidates(ctx, []ReleaseCandidate{
		{
			ServiceInstanceID: instanceID,
			GUID:              "guid-1",
			Title:             "Some.Movie.2026.1080p.BluRay.x264-GROUP",
			Indexer:           "an-indexer",
			Protocol:          "torrent",
			Categories:        `[2040,2045]`,
			SizeBytes:         sql.NullInt64{Int64: 8 << 30, Valid: true},
			Seeders:           sql.NullInt64{Int64: 42, Valid: true},
			RawReleaseJSON:    `{"guid":"guid-1"}`,
			FetchedAt:         testNow,
			ExpiresAt:         expires,
		},
		{
			ServiceInstanceID: instanceID,
			GUID:              "guid-2",
			Title:             "Another.Release-GROUP",
			RawReleaseJSON:    `{"guid":"guid-2"}`,
			FetchedAt:         testNow,
			ExpiresAt:         testNow.Add(-time.Minute), // already stale
		},
	})
	if err != nil {
		t.Fatalf("InsertReleaseCandidates: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2", len(ids))
	}

	rc, err := s.GetReleaseCandidate(ctx, owner, ids[0], testNow)
	if err != nil {
		t.Fatalf("GetReleaseCandidate: %v", err)
	}
	if rc.GUID != "guid-1" || rc.Title != "Some.Movie.2026.1080p.BluRay.x264-GROUP" {
		t.Errorf("round trip wrong: %+v", rc)
	}
	if rc.WorkID.Valid {
		t.Error("work_id should be NULL in Search-and-Grab")
	}
	// The raw ReleaseResource is needed verbatim for the grab.
	if rc.RawReleaseJSON != `{"guid":"guid-1"}` {
		t.Errorf("raw_release_json = %q, want it stored verbatim", rc.RawReleaseJSON)
	}
	// Raw Newznab categories are not collapsed.
	if rc.Categories != `[2040,2045]` {
		t.Errorf("categories = %q, want the raw ints preserved", rc.Categories)
	}
	if !rc.ExpiresAt.Equal(expires) {
		t.Errorf("expires_at = %v, want %v", rc.ExpiresAt, expires)
	}

	// An expired candidate reports expiry, not absence: "this listing went
	// stale, search again" is a different message from "no such release".
	_, err = s.GetReleaseCandidate(ctx, owner, ids[1], testNow)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("GetReleaseCandidate on a stale row = %v, want ErrExpired", err)
	}

	// The sweep removes only what has actually expired.
	n, err := s.ExpireReleaseCandidates(ctx, testNow)
	if err != nil {
		t.Fatalf("ExpireReleaseCandidates: %v", err)
	}
	if n != 1 {
		t.Fatalf("expired %d rows, want 1", n)
	}
	if _, err := s.GetReleaseCandidate(ctx, owner, ids[0], testNow); err != nil {
		t.Fatalf("the live candidate was swept: %v", err)
	}
	if _, err := s.GetReleaseCandidate(ctx, owner, ids[1], testNow); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the stale candidate survived the sweep: %v", err)
	}
}

func TestReleaseCandidateRespectsScope(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)
	instanceID := seedInstance(t, s)

	ids, err := s.InsertReleaseCandidates(ctx, []ReleaseCandidate{{
		ServiceInstanceID: instanceID, GUID: "g", Title: "t", RawReleaseJSON: "{}",
		FetchedAt: testNow, ExpiresAt: testNow.Add(time.Hour),
	}})
	if err != nil {
		t.Fatal(err)
	}

	// A scope that cannot see the instance cannot see its releases.
	out := Scope{UserID: 2, InstanceIDs: []int64{instanceID + 999}}
	if _, err := s.GetReleaseCandidate(ctx, out, ids[0], testNow); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out-of-scope read = %v, want ErrNotFound", err)
	}
}

func TestInsertReleaseCandidatesEmpty(t *testing.T) {
	ids, err := newTestStore(t).InsertReleaseCandidates(t.Context(), nil)
	if err != nil {
		t.Fatalf("InsertReleaseCandidates(nil): %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("got %d ids, want none", len(ids))
	}
}

// Release candidates cascade when their instance is hard-deleted, and the FK is
// only enforced because foreign_keys=ON reached the writer connection.
func TestReleaseCandidateRequiresARealInstance(t *testing.T) {
	_, err := newTestStore(t).InsertReleaseCandidates(t.Context(), []ReleaseCandidate{{
		ServiceInstanceID: 999999, GUID: "g", Title: "t", RawReleaseJSON: "{}",
		FetchedAt: testNow, ExpiresAt: testNow.Add(time.Hour),
	}})
	if err == nil {
		t.Fatal("inserted a release candidate for a non-existent instance")
	}
}

func TestProvenanceInsertAndJoinByDownloadID(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	const rawName = "Some.Movie.2026.2160p.UHD.BluRay.x265-GROUP"
	id, err := s.InsertProvenance(ctx, Provenance{
		Protocol:          "torrent",
		IndexerName:       "an-indexer",
		IndexerCategories: `[2040]`,
		DownloadID:        "abc123infohash",
		TorrentInfoHash:   "abc123infohash",
		ReleaseTitle:      rawName,
		ReleaseGroup:      "GROUP",
		SourceSystem:      "prowlarr",
	})
	if err != nil {
		t.Fatalf("InsertProvenance: %v", err)
	}
	if id == 0 {
		t.Fatal("InsertProvenance returned id 0")
	}

	got, err := s.GetProvenanceByDownloadID(ctx, "abc123infohash")
	if err != nil {
		t.Fatalf("GetProvenanceByDownloadID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	// The raw scene name is stored verbatim, forever: every parsed field is
	// re-derivable, the raw name is not.
	if got[0].ReleaseTitle != rawName {
		t.Errorf("release_title = %q, want %q verbatim", got[0].ReleaseTitle, rawName)
	}
	if got[0].Confidence != 1.0 {
		t.Errorf("confidence = %v, want the 1.0 default", got[0].Confidence)
	}

	// An upgrade inserts a NEW row rather than overwriting, which gives
	// upgrade history for free.
	if _, err := s.InsertProvenance(ctx, Provenance{
		Protocol: "torrent", DownloadID: "abc123infohash",
		ReleaseTitle: "Some.Movie.2026.2160p.UHD.BluRay.x265-BETTER", SourceSystem: "prowlarr",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetProvenanceByDownloadID(ctx, "abc123infohash")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows after an upgrade, want 2", len(got))
	}
}

// A manual or filesystem import is protocol='manual'. Do not launder 'unknown'
// into 'torrent'.
func TestProvenanceProtocolIsConstrained(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	for _, ok := range []string{"usenet", "torrent", "irc", "direct", "manual", "unknown"} {
		if _, err := s.InsertProvenance(ctx, Provenance{
			Protocol: ok, ReleaseTitle: "x", SourceSystem: "manual",
		}); err != nil {
			t.Errorf("protocol %q rejected: %v", ok, err)
		}
	}
	if _, err := s.InsertProvenance(ctx, Provenance{
		Protocol: "bittorrent", ReleaseTitle: "x", SourceSystem: "manual",
	}); err == nil {
		t.Error("the CHECK constraint accepted an invalid protocol")
	}
}
