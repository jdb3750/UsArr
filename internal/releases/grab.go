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

	// ResponseUnreadable is true when Prowlarr CONFIRMED the grab with a 2xx and
	// UsArr could not parse the body it sent back. The grab happened; only our
	// reading of the receipt failed. servarr.ErrDecode is produced strictly
	// after Client.do's non-2xx branch has returned, so it is not reachable on a
	// failed request.
	ResponseUnreadable bool

	GrabbedAt time.Time
}

// grabAttempt is what one dispatch produced, beyond the resource itself. It
// exists so that "confirmed" and "confirmed but unreadable" do not have to be
// smuggled through a widening tuple of bools.
type grabAttempt struct {
	Resource   servarr.ReleaseResource
	ReSearched bool
	// ResponseUnreadable mirrors GrabResult.ResponseUnreadable.
	ResponseUnreadable bool
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
//
// THE THREE OUTCOMES. Step 4 does not answer yes or no; it answers one of
// three, and the difference is whether the release reached the download client:
//
//	definitely not sent — a rejected or refused request, an open breaker, a
//	                      cache miss, an unconfigured download client. Reported
//	                      as the failure it is.
//	sent, unknown       — ErrGrabOutcomeUnknown. Grab returns a POPULATED
//	                      GrabResult ALONGSIDE the error, because a provenance
//	                      row is still written (marked unconfirmed) to keep the
//	                      infohash join key, and the caller wants its id.
//	sent and confirmed  — including ErrDecode, which can only follow a 2xx.
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

	att, grabErr := s.grabWithReSearch(ctx, cand, rel)
	if grabErr != nil && !errors.Is(grabErr, ErrGrabOutcomeUnknown) {
		return zero, grabErr
	}

	// A row is written for the sent-unknown case too, and this is the only place
	// in the codebase that writes provenance for something UsArr did not
	// confirm. The reason is the join key: for a torrent, download_id IS the
	// infohash, and if the download really did start — which it usually did,
	// because Prowlarr adds before it configures — a missing row means the file
	// can never be attached to the grab that produced it once library sync
	// lands. The row carries acquisition_state='unconfirmed' so that no reader
	// starting from provenance can mistake it for an acquisition.
	state := AcquisitionConfirmed
	if grabErr != nil {
		state = AcquisitionUnconfirmed
	}

	prov, err := s.recordProvenance(ctx, scope, cand, rel, att.Resource, state, s.now())
	if err != nil {
		// The grab HAS happened upstream. Failing the whole call here would tell
		// the user nothing was downloaded, which is false, so report the grab as
		// successful with no provenance id and log loudly.
		s.log.Error("grab could not be recorded in provenance",
			"instance_id", s.cfg.InstanceID, "candidate_id", cand.ID,
			"acquisition_state", state, "err", err)
	}

	// grabErr is returned alongside a populated result on the sent-unknown path;
	// it is nil on both confirmed paths.
	return GrabResult{
		CandidateID:        cand.ID,
		ProvenanceID:       prov,
		ReleaseTitle:       firstNonEmpty(att.Resource.Title, rel.Title, cand.Title),
		Protocol:           mapping.SourceValue(rel.Protocol),
		IndexerName:        firstNonEmpty(rel.Indexer, cand.Indexer),
		DownloadClientID:   rel.DownloadClientID,
		ReSearched:         att.ReSearched,
		ResponseUnreadable: att.ResponseUnreadable,
		GrabbedAt:          s.now(),
	}, grabErr
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
//
// Re-searching is only safe because the 404 provably precedes dispatch: the
// _remoteReleaseCache lookup is the first thing SearchController.GrabRelease
// does, well before SendReportToClient. Nothing was sent, so retrying cannot
// double-send.
func (s *Service) grabWithReSearch(ctx context.Context, cand Candidate, rel servarr.ReleaseResource) (grabAttempt, error) {
	att, err := s.classify(s.cfg.Client.Grab(ctx, rel))
	if err == nil {
		return att, nil
	}
	if !errors.Is(err, servarr.ErrNotFound) {
		return grabAttempt{}, s.grabError(err)
	}

	fresh, found, err := s.reSearch(ctx, cand)
	if err != nil {
		// The re-search did not COMPLETE. That says nothing about whether the
		// indexer still offers the release, so it must not be reported as a
		// withdrawal — ErrGrabCacheMiss's message names the indexer, and using
		// it here blamed the indexer for UsArr's own timeout.
		return grabAttempt{ReSearched: true}, fmt.Errorf("%w: %w", ErrReSearchFailed, err)
	}
	if !found {
		return grabAttempt{ReSearched: true}, ErrGrabCacheMiss
	}
	// Prefer the freshly-searched resource: its download URL may have been
	// re-minted. Carry over the stored download client choice, which is the one
	// field of the body Prowlarr actually honours besides indexerId and guid.
	if fresh.DownloadClientID == nil {
		fresh.DownloadClientID = cand.DownloadClientID
	}
	att, err = s.classify(s.cfg.Client.Grab(ctx, fresh))
	att.ReSearched = true
	if err != nil {
		if errors.Is(err, servarr.ErrNotFound) {
			return grabAttempt{ReSearched: true}, ErrGrabCacheMiss
		}
		return grabAttempt{ReSearched: true}, s.grabError(err)
	}
	return att, nil
}

// classify turns one Client.Grab return into either a confirmed attempt or the
// client's error, unchanged, for grabError to map.
//
// The one thing it decides is servarr.ErrDecode, and it is a SUCCESS.
// Client.do's non-2xx branch returns before any decode of the response body, so
// ErrDecode is produced strictly after a 2xx: Prowlarr confirmed the grab and
// UsArr failed to parse its own receipt. Reporting that as a failed grab was a
// second, independent false failure — the release is in the download client and
// the user is being invited to fetch it again.
func (s *Service) classify(grabbed servarr.ReleaseResource, err error) (grabAttempt, error) {
	switch {
	case err == nil:
		return grabAttempt{Resource: grabbed}, nil
	case errors.Is(err, servarr.ErrDecode):
		s.log.Warn("Prowlarr confirmed the grab but its response body could not be read; "+
			"treating the grab as done, because a 2xx is the whole confirmation",
			"instance_id", s.cfg.InstanceID, "err", err)
		return grabAttempt{Resource: grabbed, ResponseUnreadable: true}, nil
	}
	return grabAttempt{}, err
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
//
// The question every case here answers is DID THE RELEASE REACH THE DOWNLOAD
// CLIENT, and it is answered from the sentinel rather than from the status
// code, because the status code cannot answer it. A 500 is both "no enabled
// download client, nothing was dispatched" and "added to Deluge, then the label
// step threw" — two different outcomes behind one number. What separates them
// is which sentinel parseErrorBody produced, and for the second there is no
// signal at all beyond a frame in a .NET stack trace, which UsArr deliberately
// does not read (docs/reference/search.md).
//
// Anything not named here stays in the definitely-not-sent remainder, which is
// the safe default: an unlisted error keeps asserting failure rather than
// silently claiming a release might be downloading.
func (s *Service) grabError(err error) error {
	switch {
	case errors.Is(err, servarr.ErrNoDownloadClient):
		// DownloadClientUnavailableException is SendReportToClient's FIRST
		// statement, before anything is added, so this provably means nothing
		// was dispatched even though it arrives as a 500.
		return fmt.Errorf("%w: enable a matching download client in Prowlarr", ErrNoDownloadClient)
	case errors.Is(err, servarr.ErrNotFound):
		return ErrGrabCacheMiss
	case errors.Is(err, servarr.ErrServer),
		errors.Is(err, servarr.ErrTimeout),
		errors.Is(err, context.Canceled):
		// Sent, outcome unknown. The POST left the process and the answer says
		// nothing about what the download client did with it — see
		// ErrGrabOutcomeUnknown. context.Canceled is here for the same reason a
		// timeout is: the request was already on the wire when we stopped
		// waiting, and abandoning a reply is not evidence that nothing happened.
		return fmt.Errorf("%w: %w", ErrGrabOutcomeUnknown, err)
	case errors.Is(err, servarr.ErrValidation):
		// A 400 on a grab is UsArr's fault, not Prowlarr's: the body is built
		// entirely server-side from a stored release, so nothing the user typed
		// can reach it. Keep the upstream text — it names the property — but
		// classify it so the caller does not report a healthy Prowlarr as a bad
		// gateway. See ErrRequestRejected.
		return fmt.Errorf("%w: %w", ErrRequestRejected, err)
	}
	return err
}

// recordProvenance writes the immutable acquisition record.
//
// provenance rows are never overwritten on upgrade — a new row is inserted and
// the new media_file linked, which gives upgrade history for free (schema.md §6).
func (s *Service) recordProvenance(
	ctx context.Context,
	scope Scope,
	cand Candidate,
	rel servarr.ReleaseResource,
	grabbed servarr.ReleaseResource,
	state string,
	at time.Time,
) (int64, error) {
	var infoHash string
	if rel.InfoHash != nil {
		infoHash = *rel.InfoHash
	} else if grabbed.InfoHash != nil {
		infoHash = *grabbed.InfoHash
	}

	p := Provenance{
		// Who grabbed it, recorded from the acting scope. The column carries no
		// foreign key on purpose: this row must still name the user after that
		// user is deleted, exactly like audit_log.actor_user_id.
		UserID: scope.UserID,
		// The size the indexer reported. release_candidate has always carried
		// it and provenance did not, so before migration 0002 the size of an
		// acquisition vanished when the candidate expired 25 minutes later.
		SizeBytes:         firstNonZero(rel.Size, grabbed.Size, cand.SizeBytes),
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
		// Whether UsArr ever confirmed this. NOT folded into Confidence, which
		// means match/link confidence and is gated at >= 1.0 by the partial
		// index schema.md §7 defines — an unconfirmed row is perfectly
		// identified, and demoting its confidence would hide it from the reads
		// that most want it. See migration 0003.
		AcquisitionState: state,
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
