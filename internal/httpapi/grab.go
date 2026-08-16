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
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
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
		return errStatus(http.StatusConflict, CodeInstanceMismatch,
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
		apiErr := grabError(err)
		// The audit row records WHICH of the three outcomes this was, because
		// the read side cannot recover it otherwise: the row used to carry only
		// `{"instance_id":N}` and result="fail", so a grab that provably never
		// left the process and a grab that is very likely downloading right now
		// were byte-identical. "warn" rather than "fail" for the ambiguous one —
		// audit_log.result has no CHECK constraint and internal/store/audit.go
		// has always documented the value.
		//
		// result.ProvenanceID is read even though err is non-nil: on the
		// sent-unknown path internal/releases writes an unconfirmed provenance
		// row to keep the infohash join key, and returns its id alongside the
		// error. It is 0 on every other failure.
		auditResult, outcome := "fail", "not_sent"
		if apiErr.Code == CodeGrabOutcomeUnknown {
			auditResult, outcome = "warn", "sent_unknown"
		}
		s.audit(r, "release.grab", "release_candidate", candidateID, auditResult,
			fmt.Sprintf(`{"instance_id":%d,"outcome":%q,"error":%q,"provenance_id":%d}`,
				cand.ServiceInstanceID, outcome, apiErr.Code, result.ProvenanceID))
		return apiErr
	}

	s.audit(r, "release.grab", "release_candidate", candidateID, "ok",
		fmt.Sprintf(`{"instance_id":%d,"indexer":%q,"protocol":%q,"outcome":%q}`,
			cand.ServiceInstanceID, result.IndexerName, result.Protocol, "sent_confirmed"))

	msg := "sent to Prowlarr's download client"
	switch {
	case result.ReSearched:
		msg = "the release had aged out of Prowlarr's cache; UsArr re-searched for it and sent it"
	case result.ResponseUnreadable:
		// Prowlarr answered 2xx and UsArr could not parse the body. The grab is
		// done — the 2xx IS the confirmation, and there is no download id to
		// poll for anyway — so this is a success with a footnote, not a failure.
		msg = "sent to Prowlarr's download client; Prowlarr accepted it but UsArr could not read " +
			"the details it sent back"
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

// grabUnknownMessage is the sent-unknown wording, built once so the two halves
// stay together.
//
// It is written for a condition users meet ROUTINELY, not an exotic one.
// Prowlarr's Deluge settings pre-fill Default Category with "prowlarr"
// (DelugeSettings' constructor), Deluge's Label plugin rejects a label nobody
// created, and Prowlarr's own Test button skips the label check entirely when no
// category mappings are configured (Deluge.TestCategory returns null on
// Categories.Count == 0). Accept the defaults and your first grab lands here
// with a green connection test behind it. So it reads as a normal thing with a
// normal remedy, and it names the remedy without requiring the reader to know
// what a label plugin is.
//
// What it must never do is assert an outcome. It says what UsArr knows — the
// release was sent — says plainly that the rest is unknown, and points at the
// download client, which is the only place the truth exists. There is
// deliberately no "Test connection" action: the connection is fine, and sending
// someone to test it sends them to the one component that is not the problem.
func grabUnknownMessage(detail string) string {
	// §17.5's two required clauses come first and in its words: the download
	// client reported an error, and the release MAY OR MAY NOT have been added.
	// What follows is mechanism rather than a guess — Prowlarr's own ordering,
	// which is why "check before you grab again" is the instruction and not a
	// suggestion. "Sent" is the strongest true word here, per §17.5; never
	// "downloading", never "succeeded".
	const core = "UsArr sent this release to Prowlarr, and the download client reported an error. " +
		"The release may or may not have been added — UsArr cannot tell from here, and Prowlarr " +
		"does not say. Prowlarr hands a release to the download client before it applies its " +
		"settings, so an error at this point often means the download is already running. Check " +
		"your download client before you grab this release again, or you may end up downloading " +
		"it twice."
	if detail == "" {
		return core + " Prowlarr never sent an answer back, so there is nothing further UsArr can tell you."
	}
	return core + " If the message below mentions a label or a category, the fix is to create that " +
		"label in your download client, or to clear the Default Category for that client in " +
		"Prowlarr under Settings → Download Clients. Prowlarr said: " + detail
}

// grabError maps internal/releases' typed errors onto statuses and, more
// importantly, onto the one action that fixes each.
//
// The axis it sorts on is whether the release reached the download client, and
// there are THREE answers, not two:
//
//	not sent          — everything that keeps CodeGrabFailed, plus the codes
//	                    above it. UsArr genuinely knows these never dispatched,
//	                    so asserting failure is honest.
//	sent, unknown     — CodeGrabOutcomeUnknown, below.
//	sent and confirmed — never reaches here; it is a 200.
//
// The remainder stays "not sent" on purpose. Moving an error into sent-unknown
// has to be a positive decision backed by evidence that the POST actually left
// the process; defaulting there instead would make every ordinary failure read
// as ambiguous, and a message that appears on failures which did not dispatch is
// a message people learn to ignore — including on the one occasion it is true.
func grabError(err error) *apiError {
	switch {
	case errors.Is(err, releases.ErrCandidateNotFound):
		return errStatus(http.StatusNotFound, CodeNotFound, "no such release")
	case errors.Is(err, releases.ErrForbidden):
		return errStatus(http.StatusNotFound, CodeNotFound, "no such release")
	case errors.Is(err, releases.ErrCandidateExpired):
		return errStatus(http.StatusGone, CodeExpired,
			"this release listing went stale: Prowlarr keeps a grabbable release for 30 minutes").
			withAction("Search again")
	case errors.Is(err, releases.ErrGrabCacheMiss):
		// Only the path where the re-search RAN and came back without the
		// release. That is the one case where "no longer offered" is a claim the
		// evidence supports.
		return errStatus(http.StatusGone, CodeNoLongerOffered,
			"that indexer no longer offers this release").
			withAction("Search again")
	case errors.Is(err, releases.ErrReSearchFailed):
		// This used to be reported as no_longer_offered too, which told the user
		// the indexer had withdrawn the release when what actually happened was
		// a timeout, an open breaker or a rejected credential on UsArr's own
		// re-search. Nothing was sent either way — the grab-cache 404 precedes
		// dispatch — so failure is the honest verdict; the cause was not.
		return errStatus(http.StatusBadGateway, CodeSearchFailed,
			"Prowlarr had already dropped this release from its 30-minute grab cache, and the "+
				"search UsArr ran to recover it did not complete. Nothing was sent to your "+
				"download client, and the release may well still be there: "+
				redactText(err.Error())).
			withAction("Search again").wrapping(err)
	case errors.Is(err, releases.ErrGrabOutcomeUnknown):
		// 502 because Prowlarr is genuinely the far side of a gateway here and
		// answered badly — but the CODE, not the status, is what carries the
		// meaning, and the code is additive so nothing branching on grab_failed
		// changes behaviour.
		return errStatus(http.StatusBadGateway, CodeGrabOutcomeUnknown,
			grabUnknownMessage(redactText(releases.UpstreamMessage(err)))).
			withAction("Check your download client").wrapping(err)
	case errors.Is(err, releases.ErrNoDownloadClient):
		return errStatus(http.StatusConflict, CodeNoDownloadClient, redactText(err.Error())).
			withAction("Enable a download client in Prowlarr")
	case errors.Is(err, releases.ErrRequestRejected):
		// 500, not 502. Prowlarr answered, promptly and correctly — it refused a
		// body UsArr built. The status code is what tells the user whose problem
		// this is, and a "Test connection" action on a working connection sends
		// them to debug the one component that is fine. No action is offered
		// because there is nothing the owner of the install can do about it.
		return errStatus(http.StatusInternalServerError, CodeGrabFailed,
			"UsArr sent Prowlarr a grab it would not accept — this is a bug in UsArr, not a "+
				"problem with your Prowlarr or with this release: "+redactText(err.Error())).
			wrapping(err)
	}
	return errStatus(http.StatusBadGateway, CodeGrabFailed,
		"the grab did not go through: "+redactText(err.Error())).
		withAction("Test connection").wrapping(err)
}
