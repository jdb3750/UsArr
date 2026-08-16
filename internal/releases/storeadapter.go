package releases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// StoreAdapter bridges *store.Store to the narrow interfaces this package
// declares.
//
// The two shapes differ on purpose. internal/store speaks SQL — sql.NullInt64,
// JSON-encoded TEXT columns, RFC3339 strings — because that is what the driver
// wants. This package speaks the domain: []int32 of raw categories, *int32 for a
// field that was genuinely absent upstream. Putting the translation in one small
// adapter keeps the SQL vocabulary out of the flow logic and keeps the flow
// testable against a fake, which is the reason the interfaces exist at all.
type StoreAdapter struct {
	store *store.Store
}

// NewStoreAdapter wraps a store.
func NewStoreAdapter(s *store.Store) *StoreAdapter { return &StoreAdapter{store: s} }

var _ Store = (*StoreAdapter)(nil)

func (a *StoreAdapter) storeScope(scope Scope) store.Scope {
	return store.Scope{
		UserID:       scope.UserID,
		AllInstances: scope.AllInstances,
		InstanceIDs:  scope.InstanceIDs,
	}
}

func (a *StoreAdapter) InsertCandidates(ctx context.Context, cands []Candidate) ([]int64, error) {
	rows := make([]store.ReleaseCandidate, 0, len(cands))
	for _, c := range cands {
		cats, err := json.Marshal(nonNilInt32s(c.Categories))
		if err != nil {
			return nil, fmt.Errorf("encoding categories for %q: %w", c.GUID, err)
		}
		reasons := ""
		if len(c.RejectionReasons) > 0 {
			b, err := json.Marshal(c.RejectionReasons)
			if err != nil {
				return nil, fmt.Errorf("encoding rejection reasons for %q: %w", c.GUID, err)
			}
			reasons = string(b)
		}
		rows = append(rows, store.ReleaseCandidate{
			WorkID:            nullInt64Ptr(c.WorkID),
			ServiceInstanceID: c.ServiceInstanceID,
			GUID:              c.GUID,
			Title:             c.Title,
			Indexer:           c.Indexer,
			IndexerID:         nullInt64(int64(c.IndexerID), c.IndexerID != 0),
			Protocol:          c.Protocol,
			Categories:        string(cats),
			SizeBytes:         nullInt64(c.SizeBytes, true),
			Seeders:           nullInt32(c.Seeders),
			Leechers:          nullInt32(c.Leechers),
			AgeDays:           sql.NullFloat64{Float64: c.AgeDays, Valid: true},
			// DownloadURL is left EMPTY on purpose. The column exists, but a
			// Prowlarr download URL is a proxy link carrying that instance's full
			// admin API key; raw_release_json already holds the verbatim resource
			// the grab needs, and one copy of a credential is one too many already.
			DownloadURL:      "",
			InfoURL:          c.InfoURL,
			InfoHash:         c.InfoHash,
			DownloadClientID: nullInt32(c.DownloadClientID),
			RawReleaseJSON:   string(c.RawReleaseJSON),
			Rejected:         c.Rejected,
			RejectionReasons: reasons,
			FetchedAt:        c.FetchedAt,
			ExpiresAt:        c.ExpiresAt,
		})
	}
	return a.store.InsertReleaseCandidates(ctx, rows)
}

func (a *StoreAdapter) Candidate(ctx context.Context, scope Scope, id int64) (Candidate, error) {
	// The zero time asks the store not to apply its own expiry check: expiry is a
	// domain decision this package makes with its own clock, and it needs the row
	// in order to report WHEN it expired.
	rc, err := a.store.GetReleaseCandidate(ctx, a.storeScope(scope), id, time.Time{})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Candidate{}, ErrCandidateNotFound
		}
		return Candidate{}, err
	}

	var cats []int32
	if rc.Categories != "" {
		if err := json.Unmarshal([]byte(rc.Categories), &cats); err != nil {
			return Candidate{}, fmt.Errorf("decoding categories for candidate %d: %w", id, err)
		}
	}
	var reasons []string
	if rc.RejectionReasons != "" {
		if err := json.Unmarshal([]byte(rc.RejectionReasons), &reasons); err != nil {
			return Candidate{}, fmt.Errorf("decoding rejection reasons for candidate %d: %w", id, err)
		}
	}

	return Candidate{
		ID:                rc.ID,
		WorkID:            int64Ptr(rc.WorkID),
		ServiceInstanceID: rc.ServiceInstanceID,
		GUID:              rc.GUID,
		Title:             rc.Title,
		Indexer:           rc.Indexer,
		IndexerID:         int32Of(rc.IndexerID),
		Protocol:          rc.Protocol,
		Categories:        cats,
		SizeBytes:         rc.SizeBytes.Int64,
		Seeders:           int32PtrOf(rc.Seeders),
		Leechers:          int32PtrOf(rc.Leechers),
		AgeDays:           rc.AgeDays.Float64,
		InfoURL:           rc.InfoURL,
		InfoHash:          rc.InfoHash,
		DownloadClientID:  int32PtrOf(rc.DownloadClientID),
		RawReleaseJSON:    []byte(rc.RawReleaseJSON),
		Rejected:          rc.Rejected,
		RejectionReasons:  reasons,
		FetchedAt:         rc.FetchedAt,
		ExpiresAt:         rc.ExpiresAt,
	}, nil
}

func (a *StoreAdapter) DeleteExpiredCandidates(ctx context.Context, t time.Time) (int64, error) {
	return a.store.ExpireReleaseCandidates(ctx, t)
}

func (a *StoreAdapter) InsertProvenance(ctx context.Context, p Provenance) (int64, error) {
	cats, err := json.Marshal(nonNilInt32s(p.IndexerCategories))
	if err != nil {
		return 0, fmt.Errorf("encoding provenance categories: %w", err)
	}
	flags, err := json.Marshal(nonNilStrings(p.IndexerFlags))
	if err != nil {
		return 0, fmt.Errorf("encoding provenance flags: %w", err)
	}

	row := store.Provenance{
		Protocol:          p.Protocol,
		IndexerName:       p.IndexerName,
		IndexerID:         nullInt64(int64(p.IndexerID), p.IndexerID != 0),
		IndexerPrivacy:    p.IndexerPrivacy,
		IndexerCategories: string(cats),
		IndexerFlags:      string(flags),
		DownloadID:        p.DownloadID,
		TorrentInfoHash:   p.TorrentInfoHash,
		NZBInfoURL:        p.NZBInfoURL,
		// DownloadURL stays empty for the same reason as above, and it matters more
		// here: provenance rows are immutable and outlive everything, so a
		// credential written here is in every backup forever.
		DownloadURL:    "",
		ReleaseGUID:    p.ReleaseGUID,
		ReleaseTitle:   p.ReleaseTitle,
		SourceSystem:   p.SourceSystem,
		SourceRecordID: p.SourceRecordID,
		Confidence:     p.Confidence,
	}
	if p.PublishedAt != nil {
		row.PublishedAt = sql.NullString{String: store.FormatTime(*p.PublishedAt), Valid: true}
	}
	if !p.GrabbedAt.IsZero() {
		row.GrabbedAt = sql.NullString{String: store.FormatTime(p.GrabbedAt), Valid: true}
	}
	return a.store.InsertProvenance(ctx, row)
}

// ---- null helpers ---------------------------------------------------------

func nullInt64(v int64, valid bool) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: valid}
}

func nullInt64Ptr(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

// nullInt32 preserves the absent/zero distinction that WhenWritingNull creates
// upstream: a nil *int32 is "the indexer did not report this", not zero.
func nullInt32(v *int32) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func int64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}

func int32PtrOf(v sql.NullInt64) *int32 {
	if !v.Valid {
		return nil
	}
	out := narrowInt32(v.Int64)
	return &out
}

func int32Of(v sql.NullInt64) int32 {
	if !v.Valid {
		return 0
	}
	return narrowInt32(v.Int64)
}

// narrowInt32 clamps rather than wrapping. These columns hold values that were
// int32 upstream, so an out-of-range value means the row was written by something
// other than this code path; saturating is visibly wrong, whereas a silent wrap
// turns 2^31 into a negative indexer id that matches nothing.
func narrowInt32(v int64) int32 {
	switch {
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	}
	return int32(v)
}

// nonNilInt32s and nonNilStrings make json.Marshal emit `[]` rather than `null`
// for an empty slice, so the stored column always parses back as an array.
func nonNilInt32s(v []int32) []int32 {
	if v == nil {
		return []int32{}
	}
	return v
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
