package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jdb3750/UsArr/internal/releases"
	"github.com/jdb3750/UsArr/internal/store"
)

// releaseStore adapts *store.Store to the releases.Store interface.
//
// internal/releases declares the interface it needs by itself, at the consumer,
// rather than importing a concrete store type — so the two packages could settle
// their signatures independently. That is the right call and this file is its
// cost: one small, boring translation, in the wiring layer where a translation
// belongs. Both sides stay free of the other's types.
type releaseStore struct {
	st *store.Store
}

var _ releases.Store = (*releaseStore)(nil)

func newReleaseStore(st *store.Store) *releaseStore { return &releaseStore{st: st} }

// InsertCandidates persists a batch of search results.
func (r *releaseStore) InsertCandidates(ctx context.Context, cands []releases.Candidate) ([]int64, error) {
	rows := make([]store.ReleaseCandidate, 0, len(cands))
	for i := range cands {
		row, err := toStoreCandidate(&cands[i])
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return r.st.InsertReleaseCandidates(ctx, rows)
}

// Candidate loads one candidate, with the access scope enforced in the query.
//
// The zero time is passed for `now` because internal/releases decides what
// expired means for a grab, and reports it with a message that names the fix.
// Letting the store decide here would turn "this listing went stale, search
// again" into a bare not-found.
func (r *releaseStore) Candidate(ctx context.Context, scope releases.Scope, id int64) (releases.Candidate, error) {
	row, err := r.st.GetReleaseCandidate(ctx, toStoreScope(scope), id, time.Time{})
	if err != nil {
		return releases.Candidate{}, fmt.Errorf("%w: %v", releases.ErrCandidateNotFound, err)
	}
	if !scope.Allows(row.ServiceInstanceID) {
		// Indistinguishable from "no such row", on purpose: telling a caller
		// that a row they cannot see exists is an existence oracle.
		return releases.Candidate{}, releases.ErrCandidateNotFound
	}
	return fromStoreCandidate(row)
}

func (r *releaseStore) DeleteExpiredCandidates(ctx context.Context, t time.Time) (int64, error) {
	return r.st.ExpireReleaseCandidates(ctx, t)
}

// InsertProvenance appends one immutable acquisition record.
func (r *releaseStore) InsertProvenance(ctx context.Context, p releases.Provenance) (int64, error) {
	cats, err := json.Marshal(p.IndexerCategories)
	if err != nil {
		return 0, fmt.Errorf("encoding indexer categories: %w", err)
	}
	flags, err := json.Marshal(p.IndexerFlags)
	if err != nil {
		return 0, fmt.Errorf("encoding indexer flags: %w", err)
	}

	row := store.Provenance{
		Protocol:          p.Protocol,
		IndexerName:       p.IndexerName,
		IndexerPrivacy:    p.IndexerPrivacy,
		IndexerCategories: string(cats),
		IndexerFlags:      string(flags),
		DownloadID:        p.DownloadID,
		TorrentInfoHash:   p.TorrentInfoHash,
		NZBInfoURL:        p.NZBInfoURL,
		ReleaseGUID:       p.ReleaseGUID,
		ReleaseTitle:      p.ReleaseTitle,
		SourceSystem:      p.SourceSystem,
		SourceRecordID:    p.SourceRecordID,
		Confidence:        p.Confidence,
	}
	if p.IndexerID > 0 {
		row.IndexerID = sql.NullInt64{Int64: int64(p.IndexerID), Valid: true}
	}
	if p.PublishedAt != nil {
		row.PublishedAt = sql.NullString{String: store.FormatTime(*p.PublishedAt), Valid: true}
	}
	if !p.GrabbedAt.IsZero() {
		row.GrabbedAt = sql.NullString{String: store.FormatTime(p.GrabbedAt), Valid: true}
	}
	// DownloadURL is deliberately never carried across: a Prowlarr download URL
	// is a proxy link holding the admin API key, and provenance rows outlive
	// everything. releases.Provenance has no field for it either.
	return r.st.InsertProvenance(ctx, row)
}

// toStoreScope translates the releases scope into the store's.
//
// AllInstances is deliberately NOT set from an empty list: in store.Scope an
// empty InstanceIDs with AllInstances false renders as `1=0` and returns no
// rows, which is the fail-closed behaviour both packages want.
func toStoreScope(s releases.Scope) store.Scope {
	return store.Scope{UserID: s.UserID, InstanceIDs: s.InstanceIDs}
}

func toStoreCandidate(c *releases.Candidate) (store.ReleaseCandidate, error) {
	cats, err := json.Marshal(c.Categories)
	if err != nil {
		return store.ReleaseCandidate{}, fmt.Errorf("encoding categories for %q: %w", c.GUID, err)
	}
	rejections, err := json.Marshal(c.RejectionReasons)
	if err != nil {
		return store.ReleaseCandidate{}, fmt.Errorf("encoding rejections for %q: %w", c.GUID, err)
	}

	row := store.ReleaseCandidate{
		ServiceInstanceID: c.ServiceInstanceID,
		GUID:              c.GUID,
		Title:             c.Title,
		Indexer:           c.Indexer,
		Protocol:          c.Protocol,
		Categories:        string(cats),
		InfoURL:           c.InfoURL,
		InfoHash:          c.InfoHash,
		// Verbatim, because the grab has to echo it back — and therefore it
		// holds Prowlarr's admin key. Server-side only, forever.
		RawReleaseJSON:   string(c.RawReleaseJSON),
		Rejected:         c.Rejected,
		RejectionReasons: string(rejections),
		FetchedAt:        c.FetchedAt,
		ExpiresAt:        c.ExpiresAt,
	}
	if c.WorkID != nil {
		row.WorkID = sql.NullInt64{Int64: *c.WorkID, Valid: true}
	}
	if c.IndexerID > 0 {
		row.IndexerID = sql.NullInt64{Int64: int64(c.IndexerID), Valid: true}
	}
	if c.SizeBytes > 0 {
		row.SizeBytes = sql.NullInt64{Int64: c.SizeBytes, Valid: true}
	}
	if c.Seeders != nil {
		row.Seeders = sql.NullInt64{Int64: int64(*c.Seeders), Valid: true}
	}
	if c.Leechers != nil {
		row.Leechers = sql.NullInt64{Int64: int64(*c.Leechers), Valid: true}
	}
	if c.AgeDays != 0 {
		row.AgeDays = sql.NullFloat64{Float64: c.AgeDays, Valid: true}
	}
	if c.DownloadClientID != nil {
		row.DownloadClientID = sql.NullInt64{Int64: int64(*c.DownloadClientID), Valid: true}
	}
	return row, nil
}

func fromStoreCandidate(row store.ReleaseCandidate) (releases.Candidate, error) {
	c := releases.Candidate{
		ID:                row.ID,
		ServiceInstanceID: row.ServiceInstanceID,
		GUID:              row.GUID,
		Title:             row.Title,
		Indexer:           row.Indexer,
		Protocol:          row.Protocol,
		SizeBytes:         row.SizeBytes.Int64,
		AgeDays:           row.AgeDays.Float64,
		InfoURL:           row.InfoURL,
		InfoHash:          row.InfoHash,
		RawReleaseJSON:    []byte(row.RawReleaseJSON),
		Rejected:          row.Rejected,
		FetchedAt:         row.FetchedAt,
		ExpiresAt:         row.ExpiresAt,
	}
	if row.WorkID.Valid {
		v := row.WorkID.Int64
		c.WorkID = &v
	}
	if row.IndexerID.Valid {
		c.IndexerID = int32(row.IndexerID.Int64)
	}
	if row.Seeders.Valid {
		v := int32(row.Seeders.Int64)
		c.Seeders = &v
	}
	if row.Leechers.Valid {
		v := int32(row.Leechers.Int64)
		c.Leechers = &v
	}
	if row.DownloadClientID.Valid {
		v := int32(row.DownloadClientID.Int64)
		c.DownloadClientID = &v
	}
	if row.Categories != "" {
		if err := json.Unmarshal([]byte(row.Categories), &c.Categories); err != nil {
			return releases.Candidate{}, fmt.Errorf("decoding categories for candidate %d: %w", row.ID, err)
		}
	}
	if row.RejectionReasons != "" {
		if err := json.Unmarshal([]byte(row.RejectionReasons), &c.RejectionReasons); err != nil {
			return releases.Candidate{}, fmt.Errorf("decoding rejections for candidate %d: %w", row.ID, err)
		}
	}
	return c, nil
}
