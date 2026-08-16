package releases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/servarr/mapping"
)

// GrabResult is what a successful grab produced.
//
// Prowlarr's grab returns no download id and no queue id — the 200 IS the
// confirmation — so there is nothing here to poll. What UsArr keeps is the
// provenance row, so that when a library-bearing service is later added and
// imports the file, the provenance join has something to attach to
// (ARCHITECTURE.md §8.5, §10).
type GrabResult struct {
	CandidateID  int64
	ProvenanceID int64

	ReleaseTitle string // verbatim
	Protocol     string
	IndexerName  string

	// DownloadClientID is the Prowlarr-side client the grab was routed to, when
	// the release named one. Nil means Prowlarr picked by protocol.
	DownloadClientID *int32

	// ReSearched is true when the candidate had fallen out of Prowlarr's grab
	// cache and UsArr transparently re-searched for it before retrying.
	ReSearched bool

	GrabbedAt time.Time
}

// Grab sends a stored release candidate back to Prowlarr.
//
// The flow, and why each step is there:
//
//  1. Load the candidate through the access scope. Out-of-scope is reported as
//     not-found so a caller cannot probe for other users' rows.
//  2. Refuse an expired candidate. Prowlarr's grab cache is a non-rolling 30
//     minutes; past that the POST will 404 and the user gets a confusing error
//     instead of "search again".
//  3. Preflight GET /api/v1/downloadclient, mirroring Prowlarr's own routing:
//     by downloadClientId when the release names one, by protocol otherwise.
//     Either way, with no ENABLED client the grab returns 500 with a full .NET
//     stack trace, which no user can act on. Catching it here turns it into an
//     instruction.
//  4. POST the stored resource. On a 404 cache miss, re-search transparently and
//     retry exactly once.
//  5. Record provenance.
func (s *Service) Grab(ctx context.Context, scope Scope, candidateID int64) (GrabResult, error) {
	var zero GrabResult

	if !scope.Allows(s.cfg.InstanceID) {
		return zero, ErrForbidden
	}
	cand, err := s.cfg.Store.Candidate(ctx, scope, candidateID)
	if err != nil {
		return zero, err
	}
	if cand.ServiceInstanceID != s.cfg.InstanceID {
		// The candidate belongs to a different instance; this Service cannot grab
		// it, and saying "not found" avoids leaking which instance owns it.
		return zero, ErrCandidateNotFound
	}
	now := s.now()
	if !cand.ExpiresAt.IsZero() && !now.Before(cand.ExpiresAt) {
		return zero, fmt.Errorf("%w (expired %s)", ErrCandidateExpired, cand.ExpiresAt.UTC().Format(time.RFC3339))
	}

	// RawReleaseJSON is the only place this credential-bearing blob is read.
	var rel servarr.ReleaseResource
	if err := json.Unmarshal(cand.RawReleaseJSON, &rel); err != nil {
		return zero, fmt.Errorf("decoding stored release for candidate %d: %w", cand.ID, err)
	}

	if err := s.preflightDownloadClient(ctx, rel); err != nil {
		return zero, err
	}

	grabbed, reSearched, err := s.grabWithReSearch(ctx, cand, rel)
	if err != nil {
		return zero, err
	}

	prov, err := s.recordProvenance(ctx, cand, rel, grabbed, s.now())
	if err != nil {
		// The grab HAS happened upstream. Failing the whole call here would tell
		// the user nothing was downloaded, which is false, so report the grab as
		// successful with no provenance id and log loudly.
		s.log.Error("grab succeeded but provenance could not be recorded",
			"instance_id", s.cfg.InstanceID, "candidate_id", cand.ID, "err", err)
	}

	return GrabResult{
		CandidateID:      cand.ID,
		ProvenanceID:     prov,
		ReleaseTitle:     firstNonEmpty(grabbed.Title, rel.Title, cand.Title),
		Protocol:         mapping.SourceValue(rel.Protocol),
		IndexerName:      firstNonEmpty(rel.Indexer, cand.Indexer),
		DownloadClientID: rel.DownloadClientID,
		ReSearched:       reSearched,
		GrabbedAt:        s.now(),
	}, nil
}

// preflightDownloadClient checks that Prowlarr has an enabled download client
// that the grab will actually route to.
//
// It must mirror Prowlarr's own routing, which is by ID when the release names
// one and only otherwise by protocol. Verified against upstream `develop`,
// DownloadService.SendReportToClient:
//
//	var downloadClient = downloadClientId.HasValue
//	    ? _downloadClientProvider.Get(downloadClientId.Value)          // by id — protocol never consulted
//	    : _downloadClientProvider.GetDownloadClient(release.DownloadProtocol, release.IndexerId);
//
// and DownloadClientProvider.Get:
//
//	public IDownloadClient Get(int id) =>
//	    _downloadClientFactory.GetAvailableProviders().Single(d => d.Definition.Id == id);
//
// Checking only the protocol got both directions wrong. `.Single()` throws
// InvalidOperationException — NOT one of the three DownloadClientUnavailableException
// messages errors.go matches — so a stale or since-disabled downloadClientId
// sailed past the preflight and produced a bare 500 with a raw .NET message,
// exactly the outcome the preflight exists to prevent. And symmetrically, a
// release naming a working client was REFUSED whenever no client happened to
// match its protocol, because Prowlarr never consults the protocol on that
// branch. release_candidate rows live 25 minutes and a client can be disabled
// inside that window, so both are reachable.
//
// A failure to LIST the clients is not treated as a failure to grab: Prowlarr
// failures are soft, and refusing a grab because a secondary probe was flaky would
// be worse than attempting it and mapping the 500.
func (s *Service) preflightDownloadClient(ctx context.Context, rel servarr.ReleaseResource) error {
	clients, err := s.cfg.Client.DownloadClients(ctx)
	if err != nil {
		s.log.Warn("could not preflight download clients; attempting the grab anyway",
			"instance_id", s.cfg.InstanceID, "err", err)
		return nil
	}

	// Route by id when the release names one, exactly as Prowlarr does.
	if rel.DownloadClientID != nil {
		want := *rel.DownloadClientID
		for _, c := range clients {
			if c.ID == want && c.Enable {
				return nil
			}
		}
		return fmt.Errorf("%w: this release names Prowlarr download client %d, which is no longer "+
			"enabled — search again to pick up the current client, or re-enable it in Prowlarr "+
			"(Settings → Download Clients)", ErrNoDownloadClient, want)
	}

	for _, c := range clients {
		if c.Enable && c.Protocol == rel.Protocol {
			return nil
		}
	}
	return fmt.Errorf("%w: enable a %s download client in Prowlarr (Settings → Download Clients)",
		ErrNoDownloadClient, mapping.SourceValue(rel.Protocol))
}

// grabWithReSearch POSTs the release, and on a cache miss re-searches once.
//
// Prowlarr resolves a grab from an in-process cache keyed "{indexerId}_{guid}"
// that is populated ONLY by a prior GET /api/v1/search and dies with the process.
// A 404 "Couldn't find requested release in cache, try searching again" therefore
// means the cache entry is gone, not that the release is gone — a Prowlarr restart
// is enough. Re-running the search repopulates the cache, and re-grabbing then
// works. Doing it transparently is the difference between "it works" and "click
// search again, then click grab again".
//
// Exactly one retry. If the re-search does not turn the release up, the release
// really is no longer offered and repeating will not change that.
func (s *Service) grabWithReSearch(ctx context.Context, cand Candidate, rel servarr.ReleaseResource) (servarr.ReleaseResource, bool, error) {
	grabbed, err := s.cfg.Client.Grab(ctx, rel)
	if err == nil {
		return grabbed, false, nil
	}
	if !errors.Is(err, servarr.ErrNotFound) {
		return servarr.ReleaseResource{}, false, s.grabError(err)
	}

	fresh, found, err := s.reSearch(ctx, cand)
	if err != nil {
		return servarr.ReleaseResource{}, true, fmt.Errorf("%w: re-search failed: %w", ErrGrabCacheMiss, err)
	}
	if !found {
		return servarr.ReleaseResource{}, true, ErrGrabCacheMiss
	}
	// Prefer the freshly-searched resource: its download URL may have been
	// re-minted. Carry over the stored download client choice, which is the one
	// field of the body Prowlarr actually honours besides indexerId and guid.
	if fresh.DownloadClientID == nil {
		fresh.DownloadClientID = cand.DownloadClientID
	}
	grabbed, err = s.cfg.Client.Grab(ctx, fresh)
	if err != nil {
		if errors.Is(err, servarr.ErrNotFound) {
			return servarr.ReleaseResource{}, true, ErrGrabCacheMiss
		}
		return servarr.ReleaseResource{}, true, s.grabError(err)
	}
	return grabbed, true, nil
}

// reSearch re-runs the search that could have produced this candidate, scoped to
// the one indexer, and looks for the guid.
//
// It searches by the release title because release_candidate does not store the
// originating query — and does not need to: the title is the most selective free
// text available, and the search is pinned to a single indexer.
func (s *Service) reSearch(ctx context.Context, cand Candidate) (servarr.ReleaseResource, bool, error) {
	if cand.Title == "" || cand.IndexerID <= 0 {
		return servarr.ReleaseResource{}, false, fmt.Errorf("candidate %d has no title or indexer to re-search with", cand.ID)
	}
	req := servarr.SearchRequest{
		Text:       cand.Title,
		Type:       servarr.SearchTypeBasic,
		IndexerIDs: []int32{cand.IndexerID},
		Limit:      servarr.Int32(servarr.EffectiveLimit(servarr.IndexerCapabilityResource{}, s.cfg.PerIndexerLimit)),
	}
	rels, err := s.cfg.Client.Search(ctx, req)
	if err != nil {
		return servarr.ReleaseResource{}, false, err
	}
	for _, r := range rels {
		if r.GUID == cand.GUID {
			return r, true, nil
		}
	}
	return servarr.ReleaseResource{}, false, nil
}

// grabError maps the client's typed errors onto this package's, so callers of
// Grab never have to know which HTTP status produced them.
func (s *Service) grabError(err error) error {
	switch {
	case errors.Is(err, servarr.ErrNoDownloadClient):
		return fmt.Errorf("%w: enable a matching download client in Prowlarr", ErrNoDownloadClient)
	case errors.Is(err, servarr.ErrNotFound):
		return ErrGrabCacheMiss
	}
	return err
}

// recordProvenance writes the immutable acquisition record.
//
// provenance rows are never overwritten on upgrade — a new row is inserted and
// the new media_file linked, which gives upgrade history for free (schema.md §6).
func (s *Service) recordProvenance(
	ctx context.Context,
	cand Candidate,
	rel servarr.ReleaseResource,
	grabbed servarr.ReleaseResource,
	at time.Time,
) (int64, error) {
	var infoHash string
	if rel.InfoHash != nil {
		infoHash = *rel.InfoHash
	} else if grabbed.InfoHash != nil {
		infoHash = *grabbed.InfoHash
	}

	p := Provenance{
		Protocol:          mapping.SourceValue(rel.Protocol),
		IndexerName:       firstNonEmpty(rel.Indexer, cand.Indexer),
		IndexerID:         rel.IndexerID,
		IndexerCategories: rel.CategoryIDs(),
		IndexerFlags:      rel.IndexerFlags,
		TorrentInfoHash:   infoHash,
		// rel comes from the sanitised stored blob, so this is already redacted.
		// Redacting again is not belt-and-braces theatre: provenance rows are
		// IMMUTABLE AND PERMANENT, so this is the one write in the codebase where
		// a credential that slipped through can never be deleted, and the call is
		// idempotent. See servarr.SanitizeRelease on why the passkey matters.
		NZBInfoURL:  servarr.RedactURL(rel.InfoURL),
		ReleaseGUID: rel.GUID,
		// Verbatim, forever. Every parsed field is re-derivable from this; it is
		// not re-derivable from them.
		ReleaseTitle:   firstNonEmpty(grabbed.Title, rel.Title, cand.Title),
		GrabbedAt:      at,
		SourceSystem:   "prowlarr",
		SourceRecordID: fmt.Sprintf("%d_%s", rel.IndexerID, rel.GUID),
		Confidence:     1.0,
	}
	// Prowlarr's grab returns no download id, so for a torrent the infohash IS the
	// join key; for usenet there is nothing until an importer supplies an nzo_id.
	if p.Protocol == string(servarr.ProtocolTorrent) {
		p.DownloadID = infoHash
	}
	if !rel.PublishDate.IsZero() {
		pub := rel.PublishDate
		p.PublishedAt = &pub
	}

	// DownloadURL is deliberately never persisted: it is a Prowlarr proxy link
	// carrying the admin API key, and provenance rows outlive everything.

	return s.cfg.Store.InsertProvenance(ctx, p)
}
