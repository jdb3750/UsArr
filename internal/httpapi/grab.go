package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jdb3750/UsArr/internal/releases"
	"github.com/jdb3750/UsArr/internal/store"
)

// grabTimeout bounds the grab. It is a user action with a button waiting on it,
// so it is short — and internal/releases may spend part of it on a transparent
// re-search when Prowlarr's grab cache has dropped the release.
const grabTimeout = 45 * time.Second

type grabRequest struct {
	// InstanceID is optional: the candidate's own instance is authoritative and
	// this only exists so a client can assert which one it thinks it is talking
	// to. A mismatch is an error rather than a silent redirect.
	InstanceID int64 `json:"instance_id"`
}

type grabResponse struct {
	CandidateID  int64  `json:"candidate_id"`
	ProvenanceID int64  `json:"provenance_id,omitempty"`
	ReleaseTitle string `json:"release_title"`
	Protocol     string `json:"protocol"`
	IndexerName  string `json:"indexer_name,omitempty"`

	// ReSearched is true when the candidate had fallen out of Prowlarr's
	// 30-minute grab cache and UsArr transparently re-searched for it. Saying so
	// is the difference between "it worked" and an unexplained delay.
	ReSearched bool      `json:"re_searched"`
	GrabbedAt  time.Time `json:"grabbed_at"`
	Message    string    `json:"message"`
}

// handleGrab sends a stored release candidate back to Prowlarr.
//
// The request body names a candidate id and NOTHING ELSE that matters. The
// release resource itself is read server-side from
// release_candidate.raw_release_json, which holds Prowlarr's full admin API key
// inside downloadUrl and magnetUrl. That blob is server-side only, forever: a
// client cannot supply it, cannot see it, and cannot influence which one is
// used beyond naming a candidate the access scope already covers.
func (s *Server) handleGrab(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, "unauthorized", "this request has no session")
	}
	candidateID, err := pathInt64(r, "id")
	if err != nil {
		return err
	}
	var req grabRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	scope := storeScope(a)
	// Read the candidate first, within scope, to learn which instance owns it.
	// now is zero so the store does not reject an expired row here: expiry is
	// reported by internal/releases with the message that names the fix, which
	// is a better error than "not found".
	cand, err := s.store.GetReleaseCandidate(r.Context(), scope, candidateID, time.Time{})
	if err != nil && !errors.Is(err, store.ErrExpired) {
		return notFoundOr(err, "release")
	}
	if req.InstanceID != 0 && req.InstanceID != cand.ServiceInstanceID {
		return errStatus(http.StatusConflict, "instance_mismatch",
			"that release belongs to a different service instance")
	}

	searcher, err := s.searcherFor(r.Context(), cand.ServiceInstanceID)
	if err != nil {
		return err
	}
	rscope, err := s.releasesScope(r, a)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(r.Context(), grabTimeout)
	defer cancel()

	result, err := searcher.Grab(ctx, rscope, candidateID)
	if err != nil {
		s.audit(r, "release.grab", "release_candidate", candidateID, "fail",
			fmt.Sprintf(`{"instance_id":%d}`, cand.ServiceInstanceID))
		return grabError(err)
	}

	s.audit(r, "release.grab", "release_candidate", candidateID, "ok",
		fmt.Sprintf(`{"instance_id":%d,"indexer":%q,"protocol":%q}`,
			cand.ServiceInstanceID, result.IndexerName, result.Protocol))

	msg := "sent to Prowlarr's download client"
	if result.ReSearched {
		msg = "the release had aged out of Prowlarr's cache; UsArr re-searched for it and sent it"
	}
	writeJSON(w, http.StatusOK, grabResponse{
		CandidateID:  result.CandidateID,
		ProvenanceID: result.ProvenanceID,
		ReleaseTitle: result.ReleaseTitle,
		Protocol:     result.Protocol,
		IndexerName:  result.IndexerName,
		ReSearched:   result.ReSearched,
		GrabbedAt:    result.GrabbedAt,
		Message:      msg,
	})
	return nil
}

// grabError maps internal/releases' typed errors onto statuses and, more
// importantly, onto the one action that fixes each.
func grabError(err error) error {
	switch {
	case errors.Is(err, releases.ErrCandidateNotFound):
		return errStatus(http.StatusNotFound, "not_found", "no such release")
	case errors.Is(err, releases.ErrForbidden):
		return errStatus(http.StatusNotFound, "not_found", "no such release")
	case errors.Is(err, releases.ErrCandidateExpired):
		return errStatus(http.StatusGone, "expired",
			"this release listing went stale: Prowlarr keeps a grabbable release for 30 minutes").
			withAction("Search again")
	case errors.Is(err, releases.ErrGrabCacheMiss):
		return errStatus(http.StatusGone, "no_longer_offered",
			"that indexer no longer offers this release").
			withAction("Search again")
	case errors.Is(err, releases.ErrNoDownloadClient):
		return errStatus(http.StatusConflict, "no_download_client", redactText(err.Error())).
			withAction("Enable a download client in Prowlarr")
	}
	return errStatus(http.StatusBadGateway, "grab_failed",
		"the grab did not go through: "+redactText(err.Error())).
		withAction("Test connection").wrapping(err)
}
