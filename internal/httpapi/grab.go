package httpapi

import (
	"context"
	"encoding/json"
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
	CandidateID int64 `json:"candidate_id"`

	// ProvenanceID is the OPAQUE row id, the same token GET /api/v1/grabs/recent
	// publishes for this acquisition — grabRowID over (user_id, rowid), not the
	// rowid.
	//
	// REVIEW FINDING RG-01.3, THE SECOND HALF. This field shipped the raw
	// provenance rowid, which is the same cross-user volume oracle the
	// Recent-grabs read was fixed for: provenance.id is an INTEGER PRIMARY KEY
	// and therefore one monotonic sequence shared by every user, so two of a
	// caller's own ids bracket a count of everybody else's rows, and no access
	// scope can close that — the scope decides which rows come back, not what
	// their ids say about the ones that did not.
	//
	// AND IT IS THE EASIER HALF TO HARVEST, not the lesser one. The list read
	// hands over ten ids at a time; this hands one over per grab, on demand,
	// with the caller choosing when — which is a cleaner sampling instrument
	// than the list ever was.
	//
	// Deriving it with the SAME function and the SAME inputs is what keeps the
	// feature: the caller can match the grab they just made to its row in Recent
	// grabs by string equality, and learns nothing else from it.
	ProvenanceID string `json:"provenance_id,omitempty"`

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
		// cand is the zero value here — there is genuinely no release to name,
		// and the row says so rather than inventing a title.
		return s.auditNotSent(r, candidateID, cand, notFoundOr(err, "release"))
	}
	if req.InstanceID != 0 && req.InstanceID != cand.ServiceInstanceID {
		return s.auditNotSent(r, candidateID, cand, errStatus(http.StatusConflict, CodeInstanceMismatch,
			"that release belongs to a different service instance"))
	}

	searcher, err := s.searcherFor(r.Context(), cand.ServiceInstanceID)
	if err != nil {
		return s.auditNotSent(r, candidateID, cand, err)
	}
	rscope, err := s.releasesScope(r, a)
	if err != nil {
		return s.auditNotSent(r, candidateID, cand, err)
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
		auditResult, outcome := store.AuditResultFail, outcomeNotSent
		if apiErr.Code == CodeGrabOutcomeUnknown {
			auditResult, outcome = store.AuditResultWarn, outcomeSentUnknown
		}
		s.audit(r, auditGrabAction, "release_candidate", candidateID, auditResult,
			grabAuditJSON(grabAuditMeta{
				InstanceID:   cand.ServiceInstanceID,
				Outcome:      outcome,
				Error:        string(apiErr.Code),
				Message:      apiErr.Message,
				ReleaseTitle: cand.Title,
				Indexer:      cand.Indexer,
				Protocol:     cand.Protocol,
				ProvenanceID: result.ProvenanceID,
			}))
		return apiErr
	}

	s.audit(r, auditGrabAction, "release_candidate", candidateID, store.AuditResultOK,
		grabAuditJSON(grabAuditMeta{
			InstanceID:   cand.ServiceInstanceID,
			Outcome:      outcomeSentConfirmed,
			ReleaseTitle: result.ReleaseTitle,
			Indexer:      result.IndexerName,
			Protocol:     result.Protocol,
			ProvenanceID: result.ProvenanceID,
		}))

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
		CandidateID: result.CandidateID,
		// a.User.ID is the user_id internal/releases wrote onto the provenance
		// row (releasesScope carries it through), so this token is byte-identical
		// to the one Recent grabs publishes for the same row. If those two ever
		// disagree the correlation silently breaks, which is why the id is built
		// from the session rather than re-read from anywhere.
		ProvenanceID: s.provenanceRowID(a, result.ProvenanceID),
		ReleaseTitle: result.ReleaseTitle,
		Protocol:     result.Protocol,
		IndexerName:  result.IndexerName,
		ReSearched:   result.ReSearched,
		GrabbedAt:    result.GrabbedAt,
		Message:      msg,
	})
	return nil
}

// provenanceRowID renders the opaque id for a provenance row this caller just
// wrote, or "" when there is no row.
//
// The empty case is not an error: rowid 0 means internal/releases wrote no
// provenance row, which is every not-sent failure. Hashing 0 would mint a
// perfectly stable token for a row that does not exist, and `omitempty` then
// could not drop it — so the absence has to be preserved here rather than
// recovered downstream.
func (s *Server) provenanceRowID(a authSession, rowID int64) string {
	if rowID == 0 {
		return ""
	}
	return grabRowID(s.cfg.GrabRowIDKey, a.User.ID, rowID)
}

// The grab audit vocabulary, spelled once. `action` is what the Recent-grabs
// read filters on, and a typo in a string literal here is a row that silently
// never appears in the list it was written for — which is why the action and
// the three results are now taken from internal/store, the side that does the
// reading, instead of being spelled again here.
const (
	auditGrabAction = store.AuditActionGrab

	// outcome names WHERE the release got to, which is the axis Recent grabs
	// sorts on. It is not a synonym for audit_log.result: sent-unknown is
	// result="warn", and both sent states are recorded even though only one of
	// them is a 200.
	outcomeNotSent       = "not_sent"
	outcomeSentUnknown   = "sent_unknown"
	outcomeSentConfirmed = "sent_confirmed"
)

// grabAuditMeta is audit_log.metadata_json for a grab, as a struct rather than
// a hand-built string.
//
// WHY A STRUCT. This was fmt.Sprintf with %q, which is Go quoting and not JSON
// quoting — they agree on ASCII and diverge on the rest, and a release title is
// exactly the field that carries an em dash, a CJK character or a stray control
// byte straight from an indexer. A row whose metadata will not parse is a row
// the reader must skip, which is the failure this change exists to remove.
//
// WHY THESE FIELDS. A Recent-grabs row renders time, release name, indexer,
// protocol and state. The failure arm reads from audit_log, so every one of
// those has to be ON the audit row: before this it carried instance_id, outcome
// and an error CODE, so even a row that existed had nothing to display but an
// enum. Message is the user-facing error text — the same sentence the user was
// shown — so the list can say why without re-deriving it from a code.
//
// NO SECRET VALUES. Everything here is a name, a host-free id or a message that
// has already been through redactText, and Server.audit redacts the rendered
// JSON again on the way in.
//
// InstanceID IS THE ONLY PLACE A GRAB'S SERVICE IS RECORDED, and that asymmetry
// is deliberate rather than pending. `provenance` has no service_instance_id, so
// the audit arm of Recent grabs can name which Prowlarr a grab went to and the
// provenance arm cannot. That is a display gap on one column, not a correctness
// gap — the union works either way — and docs/FUTURE.md §18 carries the argument
// for recording it instead of spending migration 0004 on it, along with the two
// conditions that reopen it.
type grabAuditMeta struct {
	InstanceID   int64  `json:"instance_id"`
	Outcome      string `json:"outcome"`
	Error        string `json:"error,omitempty"`
	Message      string `json:"message,omitempty"`
	ReleaseTitle string `json:"release_title,omitempty"`
	Indexer      string `json:"indexer,omitempty"`
	Protocol     string `json:"protocol,omitempty"`
	ProvenanceID int64  `json:"provenance_id,omitempty"`
}

// grabAuditJSON renders the metadata. A marshal failure yields the minimum
// honest object rather than an empty string, so the row still records the
// outcome even if a field would not encode.
func grabAuditJSON(m grabAuditMeta) string {
	buf, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf(`{"instance_id":%d,"outcome":%q}`, m.InstanceID, m.Outcome)
	}
	return string(buf)
}

// auditNotSent records a grab that failed BEFORE dispatch, and returns the
// error unchanged so a call site stays one line.
//
// WHICH FAILURES REACH HERE, AND WHICH DELIBERATELY DO NOT. handleGrab has
// seven early returns ahead of the dispatch, and only four of them write a row.
// The line is whether the request named a release this user could have grabbed:
//
//	no session          — NO ROW. There is no actor, so actor_user_id is NULL and
//	                      the scoped read (`actor_user_id IN (0, :uid)`) can never
//	                      return it — a row nobody can read is cost without
//	                      signal. Worse, this is the one branch reachable without
//	                      credentials, so appending here lets anyone who can
//	                      reach the port grow an append-only table by request.
//	                      Scanner traffic, not grab history.
//	malformed path id   — NO ROW. `/releases/wat/grab` never named a release;
//	                      target_id would be empty and the row would render as a
//	                      failed grab of nothing. This is a routing rejection
//	                      wearing a handler's clothes.
//	undecodable body    — NO ROW. Same reason one step later: the body is
//	                      rejected before the candidate is read, so there is no
//	                      title, no indexer and no instance — none of the three
//	                      fields the row exists to carry. A client bug belongs in
//	                      the request log, which already has it.
//	candidate not found — ROW. A real attempt: the user clicked Grab on something
//	                      and nothing was sent. The candidate is gone (swept
//	                      after its TTL) or outside their scope, so the title is
//	                      genuinely unknown and the row says so by omitting it
//	                      rather than by guessing. "The listing had been swept" is
//	                      the answer to "why did that do nothing", and it is the
//	                      answer only this row can give.
//	instance mismatch   — ROW, and the best-furnished of them: the candidate
//	                      resolved, so title, indexer and protocol are all known.
//	searcherFor failed  — ROW. The release is known and the Prowlarr that owns it
//	                      could not be opened. Nothing was sent, and this is
//	                      precisely the "your stack is broken" row the Recent-grabs
//	                      failure arm is for.
//	releasesScope failed— ROW. Same shape: the release is known, the scope read
//	                      failed, nothing dispatched.
//
// Everything here is outcome=not_sent and result="fail", never "warn": these
// all precede the POST, so UsArr genuinely knows the release never left the
// process. Asserting failure is honest for exactly this set and for no other.
func (s *Server) auditNotSent(r *http.Request, candidateID int64, cand store.ReleaseCandidate, err error) error {
	code, message := CodeInternal, ""
	var ae *apiError
	if errors.As(err, &ae) {
		code, message = ae.Code, ae.Message
	} else if err != nil {
		message = redactText(err.Error())
	}
	s.audit(r, auditGrabAction, "release_candidate", candidateID, store.AuditResultFail,
		grabAuditJSON(grabAuditMeta{
			InstanceID:   cand.ServiceInstanceID,
			Outcome:      outcomeNotSent,
			Error:        string(code),
			Message:      message,
			ReleaseTitle: cand.Title,
			Indexer:      cand.Indexer,
			Protocol:     cand.Protocol,
		}))
	return err
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
