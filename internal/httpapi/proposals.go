package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// §17.8's Accept step on the wire: what UsArr PROPOSES, and what accepting a
// proposal creates.
//
// Two endpoints, and they are the read and the write of one screen:
//
//	GET  /api/v1/libraries/proposals — the proposal set
//	POST /api/v1/libraries/accept    — turn proposals into libraries
//
// The wire contract is written down where a consumer will find it:
// docs/reference/http-api.md §2a and §2b. A doc comment is not reachable from a
// browser tab, which is the lesson /library/recent's entry records.
//
// # BOTH ARE LOCAL, AND THE READ ONE IS THE ONE THAT HAD TO BE ARGUED
//
// [ADR-0048](../../docs/DECISIONS.md#adr-0048) clause 1 computes the proposal set
// from *"what the connected instance reports as its containers"*, and its clause
// 5 excuses the CONNECT PROBE's upstream call as a setup action. That excuse does
// not reach here: /settings/libraries is a screen a user navigates to, and a
// proposal that exists only inside a probe response would be available for a few
// seconds after adding a service and never again. store.ProposedContainers reads
// the replicated half — `container_observed` rows written by the bind
// transaction — so this endpoint renders from the local file on every visit,
// which is what principle 1 requires of a user-facing read. Neither handler in
// this file may grow an upstream call;
// TestLibraryProposalHandlersReachNothingOutbound is the structural guard.
//
// # WHY ACCEPT IS ITS OWN PATH RATHER THAN `POST /api/v1/libraries`
//
// The route table's convention is a resource path plus an action segment for an
// operation that is not a plain create — `POST /api/v1/services/test`,
// `POST /api/v1/services/{id}/sync`, `POST /api/v1/releases/{id}/grab` — and
// Accept is exactly that shape: a BATCH whose per-item outcome may be a JOIN into
// a library that already exists (§17.8's merge rule), not a create of one named
// resource. `POST /api/v1/libraries` is left unclaimed for §17.8's **Add
// library**, which is the one-at-a-time create; folding the two together would
// make the create an argument-dependent special case of the batch, which is the
// same argument server.go's route table makes for keeping /library and
// /library/recent apart.
//
// # NO SUDO, AND THAT IS A DECISION RATHER THAN AN OMISSION
//
// Every write on the SERVICES screen is sudo-gated because it touches a stored
// *Arr credential (§12.1). Accept touches none: it writes `library`,
// `library_source`, `library_member` and `search_doc_library`, and it takes a
// service instance's id only to bind a source to it, once the scope has said the
// caller may name it. §17.8 puts credentials behind Services plus sudo and asks
// for nothing of the sort on this screen — *"No credential field ever appears on
// this screen"* — so the gate is CSRF plus an authenticated session, which is
// what POST /releases/{id}/grab carries.

// proposalBoundLibraryResponse is one library a proposed container already feeds.
//
// A FIELD-BY-FIELD ALLOWLIST, like every wire struct in this package's Libraries
// surface: store.BoundLibrary is copied into it by hand.
type proposalBoundLibraryResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// proposalJoinResponse is the library accepting this proposal AS SUGGESTED would
// join rather than create.
//
// ⚠️ IT IS A PREDICTION ABOUT THE SUGGESTED NAME AND NOTHING ELSE. The store
// computes it by calling the same lookup the writer calls, on §17.8's same
// case-insensitive whitespace-trimmed merge key, at the same kind — so the
// screen's *"Joining Kavita Manga into Comics as a second source."* and the
// acceptance's actual join are one derivation. A user who edits the name in the
// text box moves the answer, and recomputing it while they type is the browser's
// job: this object describes `suggested_name`, not whatever is in the field now.
type proposalJoinResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// containerProposalResponse is one row of §17.8's Accept step as it crosses to a
// browser.
//
// A FIELD-BY-FIELD ALLOWLIST, never a denylist, and the struct IS the allowlist:
// store.ContainerProposal is copied into this by hand and
// TestProposalsResponseKeysAreTheAllowlist pins the key set, so a field added to
// the store type cannot reach the wire by default.
//
// THIS IS A §14 STRUCT FOR THE SAME REASON librarySourceResponse IS. A proposal
// names the service instance that reported the container, and `service_instance`
// carries `api_key_enc` — a full-admin *Arr credential — and `base_url`, an
// internal host the user typed. The store read allowlists two columns off that
// table (see store.ContainerProposal.ServiceName) and this allowlists the same
// two onto the wire: the NAME, which is the chip's label, and the KIND, which is
// its icon.
//
// WHAT IS ABSENT AND WHY:
//
//   - `instance_last_full_sync_at`. It is on the store type as the INPUT to
//     `not_seen_by_last_sync`, which this response already carries as a decided
//     answer, and the timestamp itself is GET /api/v1/services/health's field
//     (reference/http-api.md §3). Publishing it here would put the same fact on
//     two endpoints with two derivations behind one screen.
//   - `container_kind`. Every proposal this endpoint can serve is a
//     `remote_library`: the store's statement filters `remote_kind = 'library'`,
//     and §6.5 rule 3's other container kinds have no membership derivation in
//     the tree. §17.4 rule 5 — a column whose value is identical for every row is
//     not data — applies to a wire field as much as to a table cell. It arrives
//     with the second kind that can be filed, on the same commit.
type containerProposalResponse struct {
	// ServiceInstanceID and ContainerRef are the proposal's IDENTITY, and they
	// are the pair an acceptance sends back. There is no proposal id, because
	// ADR-0048 clause 1 keeps a proposal out of storage entirely: it is a value
	// recomputed from local state on every read, and an id would be a handle on
	// a row that does not exist.
	ServiceInstanceID int64  `json:"service_instance_id"`
	ContainerRef      string `json:"container_ref"`

	ServiceName string `json:"service_name"`
	ServiceKind string `json:"service_kind"`

	// ContainerName is what the upstream itself called the container. §17.8
	// renders it beneath the editable name, *"greyed and non-editable"*.
	ContainerName string `json:"container_name"`

	// Kind is the work kind the adapter decided this container holds, absent on
	// a DECLINED container. Declined and KindProvisional are the two facts about
	// that decision, and they are separate keys because they are separate
	// questions: whether UsArr took the container at all, and whether the kind
	// it took is a guess.
	Kind            string `json:"kind,omitempty"`
	KindProvisional bool   `json:"kind_provisional"`

	// Declined is store.ContainerProposal.Declined() — the adapter refused this
	// container — and DeclineReason is the word §17.8's `Decision` column
	// renders. ⚠️ The reason is UsArr's own sentence about what it has no
	// `work.kind` for; it is never upstream text (reference/security.md §5).
	Declined      bool   `json:"declined"`
	DeclineReason string `json:"decline_reason,omitempty"`

	// ItemCount is how many top-level works from this container are ALREADY in
	// the replica. It is the number ADR-0048's 2026-08-21 amendment exists to
	// put on this screen: *"You'd be ticking proposals with real item counts
	// beside them"*, against *"deciding blind"*. Those works are sitting
	// unfiled until an acceptance files them.
	ItemCount int64 `json:"item_count"`

	// SuggestedName is the name the Accept screen pre-fills, editable in place.
	SuggestedName string `json:"suggested_name"`

	// Joins is the library accepting this proposal AS SUGGESTED would join.
	// Absent means it would create one. See proposalJoinResponse.
	Joins *proposalJoinResponse `json:"joins,omitempty"`

	// BoundTo is the libraries this container ALREADY feeds, and it is always
	// present — `[]` rather than absent, on librariesResponse.Items's reasoning:
	// an absent key reads as "unknown", and "this container feeds nothing yet"
	// is precisely the common case this screen renders.
	//
	// ⚠️ A NON-EMPTY `bound_to` DOES NOT MEAN THIS IS NOT A PROPOSAL. The store
	// drops a container already bound AT THE OBSERVED KIND and keeps one bound
	// only at another, because ADR-0066 decision 5 puts two libraries over one
	// mixed container and the second kind is a genuine proposal; what
	// `bound_to` then says is which library it would sit beside.
	BoundTo []proposalBoundLibraryResponse `json:"bound_to"`

	// ObservedAt is when UsArr last recorded this container being reported.
	// Absent when the stored stamp will not parse, on libraryTime's reasoning:
	// a wrong date on a staleness claim is worse than a missing one.
	ObservedAt *time.Time `json:"observed_at,omitempty"`

	// NotSeenByLastSync says the last COMPLETED full sync did not report this
	// container. It is the ONLY thing that can say a proposal went away: an
	// unbound container has no `library_source` row, so nothing else in the
	// schema records that it stopped being reported.
	//
	// ⚠️ FALSE ON AN INSTANCE THAT HAS NEVER COMPLETED A FULL SYNC, because
	// there is no completed run for one to have been missed by — not because
	// the container was seen.
	NotSeenByLastSync bool `json:"not_seen_by_last_sync"`
}

type proposalsResponse struct {
	// Items is always present, `[]` when nothing is proposed. §17.8's screen has
	// a zero state and an absent key would make it indistinguishable from a
	// failure.
	Items []containerProposalResponse `json:"items"`
}

// handleListLibraryProposals serves §17.8's Accept step.
//
// NO PAGING AND NO QUERY PARAMETERS, matching handleListLibraries and matching
// the store read: the proposals are one screen's worth of containers the
// configured services reported, and there is nothing here to clamp, so there is
// no 400 path.
func (s *Server) handleListLibraryProposals(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	// SCOPE. storeScope derives it from the session and nothing else — the
	// caller cannot widen it with a query parameter, which is the whole reason
	// it is not one. A caller who cannot see an instance is not told that
	// instance has containers; the store fails closed on an empty visible set,
	// so "no scope" returns nothing rather than everything.
	rows, err := s.store.ProposedContainers(r.Context(), storeScope(a))
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"the library proposals could not be read").wrapping(err)
	}

	out := proposalsResponse{Items: make([]containerProposalResponse, 0, len(rows))}
	for _, row := range rows {
		out.Items = append(out.Items, toContainerProposalResponse(row))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// toContainerProposalResponse is the allowlist applied. Every field is named;
// nothing is copied by reflection or embedded.
func toContainerProposalResponse(p store.ContainerProposal) containerProposalResponse {
	out := containerProposalResponse{
		ServiceInstanceID: p.ServiceInstanceID,
		ContainerRef:      p.RemoteID,
		ServiceName:       p.ServiceName,
		ServiceKind:       p.ServiceKind,
		ContainerName:     p.Name,
		Kind:              p.Kind,
		KindProvisional:   p.KindProvisional,
		Declined:          p.Declined(),
		DeclineReason:     p.DeclineReason,
		ItemCount:         p.ItemCount,
		SuggestedName:     p.SuggestedName,
		NotSeenByLastSync: p.NotSeenByLastSync,
		BoundTo:           make([]proposalBoundLibraryResponse, 0, len(p.BoundTo)),
	}
	for _, b := range p.BoundTo {
		out.BoundTo = append(out.BoundTo, proposalBoundLibraryResponse{
			ID: b.ID, Name: b.Name, Kind: b.Kind,
		})
	}
	if p.JoinsLibraryID != 0 {
		out.Joins = &proposalJoinResponse{ID: p.JoinsLibraryID, Name: p.JoinsLibraryName}
	}
	if at, err := store.ParseTime(p.ObservedAt); err == nil {
		out.ObservedAt = &at
	}
	return out
}

// ─── POST /api/v1/libraries/accept ──────────────────────────────────────────

// maxAcceptedNameLen bounds a library name on the wire.
//
// The column has no length limit and does not need one; this is a WIRE bound, so
// that a name the caller invented cannot travel back out in an error body at the
// size of the 1 MB request cap. §17.8's names are what fits in a table cell.
const maxAcceptedNameLen = 200

// acceptedSourceRequest is one container an acceptance binds.
//
// ⚠️ THERE IS NO `container_kind` FIELD, on containerProposalResponse's
// reasoning: `remote_library` is the only kind this endpoint's own proposals can
// offer and the only one with a membership derivation in the tree, so a field
// carrying it would have exactly one legal value. The handler sets it; the
// store refuses anything else, which keeps the refusal where the derivation is.
type acceptedSourceRequest struct {
	ServiceInstanceID int64  `json:"service_instance_id"`
	ContainerRef      string `json:"container_ref"`

	// ContainerName is the container's own name as the proposal reported it,
	// stored so an upstream that reuses ids cannot silently rebind the library
	// to a different container. It is `library_source.container_identity`; the
	// wire spells it as GET /api/v1/libraries already spells it.
	ContainerName string `json:"container_name"`
}

// libraryAcceptanceRequest is one accepted proposal, as the user edited it.
type libraryAcceptanceRequest struct {
	// Name is what the user is accepting, after any inline edit. It is matched
	// on §17.8's merge key — case-insensitive, whitespace-trimmed, per user — so
	// a name that differs only in case or padding JOINS rather than collides.
	Name string `json:"name"`

	// Kind is `library.kind`: exactly one value, required (§6.5 rule 4). The
	// handler checks it is present and the schema's CHECK decides whether it is
	// real — a second copy of that list in Go would be two derivations of one
	// membership rule, which is the defect class DEVELOPMENT.md §11 names.
	Kind string `json:"kind"`

	// Formats is the format filter over `edition.format`; absent or empty means
	// any. It is the column §17.8's flagship Ebooks/Audiobooks split lands on.
	Formats []string `json:"formats,omitempty"`

	// Edited is §17.8's one-way door: *"Editing any proposal marks that library
	// user-managed."*
	//
	// ⚠️ IT IS NOT `managed_by`, AND THE DIFFERENCE IS DELIBERATE. The stored
	// column's vocabulary is `auto`/`user`, and DEVELOPMENT.md §11 is explicit
	// that a wire vocabulary and a storage vocabulary never share a term — two
	// vocabularies that match today are free to diverge tomorrow. What the
	// SCREEN knows is whether the row was edited; what the store records is a
	// managed-by value. The translation is one line in this file, and it is the
	// only place the two vocabularies meet.
	Edited bool `json:"edited"`

	// Sources are the containers this library binds. Two proposals merged by
	// §17.8's rename rule arrive as ONE acceptance carrying both.
	Sources []acceptedSourceRequest `json:"sources"`
}

type acceptLibrariesRequest struct {
	Accept []libraryAcceptanceRequest `json:"accept"`
}

// The two outcomes of an acceptance, as the WIRE spells them.
//
// Distinct spellings from the store's own `Created`/`Joined` booleans, on
// DEVELOPMENT.md §11's rule, and ONE field rather than two: the store documents
// the pair as exclusive, and a wire that published both would let a client meet
// a body claiming neither or both. Collapsing them here means the impossible
// state cannot be marshalled.
const (
	wireAcceptCreated = "created"
	wireAcceptJoined  = "joined"
)

// acceptedLibraryResponse is what one acceptance did.
//
// A FIELD-BY-FIELD ALLOWLIST. store.AcceptedLibrary is copied into it by hand.
type acceptedLibraryResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`

	// Slug is the library's URL identity, and §17.8 is emphatic that it is NOT
	// rendered as a path: it is the value behind `?lib=ebooks`, not a folder.
	Slug string `json:"slug"`
	Kind string `json:"kind"`

	// Outcome is `created` or `joined`, and §17.8 requires the screen to SAY
	// which happened — *"Joining Kavita Manga into Comics as a second source."*
	Outcome string `json:"outcome"`

	// MembersFiled is how many works MOVED from unfiled into this library. On a
	// re-accept of the same container it is 0 rather than the library's size,
	// which is what makes accepting twice readable rather than alarming.
	MembersFiled int64 `json:"members_filed"`
}

type acceptLibrariesResponse struct {
	// Items is one entry per acceptance, in the order they were sent. It is
	// always present, and the endpoint refuses an empty batch, so `[]` never
	// reaches a client.
	Items []acceptedLibraryResponse `json:"items"`
}

// handleAcceptLibraries turns §17.8's ticked proposals into libraries.
//
// # ALL OR NOTHING, AND THE STORE IS WHERE THAT IS DECIDED
//
// store.AcceptLibraries runs the whole batch in one transaction: a name that
// collides, a source outside the scope, and NOTHING is written — not even the
// acceptances that had already succeeded. So a non-2xx from this endpoint means
// no library was created or joined by this call. That is affordable precisely
// because ADR-0048 clause 1 keeps a proposal out of storage: the rows that did
// not get created are still proposals, GET /libraries/proposals recomputes the
// same set, and the cost of a refusal is the user fixing one name and pressing
// again.
//
// # WHAT IS VALIDATED HERE AND WHAT IS LEFT TO THE STORE
//
// This handler refuses what is a fact about the REQUEST — an empty batch, a
// nameless or kindless acceptance, an acceptance with no source, a source with
// no instance — so that a client error is a 400 naming the field rather than a
// 500 from a layer that had no reason to expect it. Everything that is a fact
// about the DATA — whether the name is free, whether the caller may name that
// instance — belongs to the store, which decides it inside the transaction where
// it cannot be raced, and returns a sentinel this file maps.
func (s *Server) handleAcceptLibraries(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}
	var req acceptLibrariesRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	accepts, err := storeAcceptances(req)
	if err != nil {
		return err
	}

	// SCOPE, and it is the WRITE side of the same predicate the read carries:
	// binding a library to an instance the caller cannot see would publish that
	// instance's item count back through the proposals read.
	got, err := s.store.AcceptLibraries(r.Context(), storeScope(a), a.User.ID, accepts)
	if err != nil {
		return acceptLibrariesError(err)
	}

	// Rendered in full BEFORE anything is audited, so a batch this layer cannot
	// describe does not leave half a journal behind it. The write itself already
	// committed — the store's transaction is closed by here — so the audit rows
	// record what happened either way; what is avoided is a row for the first
	// two acceptances and none for the third.
	out := acceptLibrariesResponse{Items: make([]acceptedLibraryResponse, 0, len(got))}
	for _, lib := range got {
		outcome, err := acceptOutcome(lib)
		if err != nil {
			return err
		}
		out.Items = append(out.Items, acceptedLibraryResponse{
			ID:           lib.ID,
			Name:         lib.Name,
			Slug:         lib.Slug,
			Kind:         lib.Kind,
			Outcome:      outcome,
			MembersFiled: lib.MembersFiled,
		})
	}
	for _, lib := range out.Items {
		s.audit(r, "library.accept", "library", lib.ID, store.AuditResultOK, "")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// acceptOutcome collapses the store's exclusive pair into the one wire value.
//
// A RESULT THAT IS NEITHER IS AN ERROR RATHER THAN A DEFAULT. The store
// documents Created and Joined as exclusive with exactly one true; if that ever
// stops holding, the honest answer is a 500 the log names, not a body that says
// `created` because it was the first branch.
func acceptOutcome(lib store.AcceptedLibrary) (string, error) {
	switch {
	case lib.Created && !lib.Joined:
		return wireAcceptCreated, nil
	case lib.Joined && !lib.Created:
		return wireAcceptJoined, nil
	}
	return "", errStatus(http.StatusInternalServerError, CodeInternal,
		"a library was accepted and UsArr cannot say whether it was created or joined")
}

// storeAcceptances validates the request and renders it as the store's type.
//
// The 400s are all facts about the request document. Each names the index of the
// acceptance it refused, because a batch whose error says only "a name is
// missing" is one the user cannot act on.
func storeAcceptances(req acceptLibrariesRequest) ([]store.LibraryAcceptance, error) {
	if len(req.Accept) == 0 {
		return nil, errStatus(http.StatusBadRequest, CodeBadRequest,
			"this request accepts no proposals").
			withAction("Tick at least one proposal")
	}
	out := make([]store.LibraryAcceptance, 0, len(req.Accept))
	for i, in := range req.Accept {
		name := strings.TrimSpace(in.Name)
		switch {
		case name == "":
			return nil, badAcceptance(i, "has no name")
		case len(name) > maxAcceptedNameLen:
			return nil, badAcceptance(i, "has a name longer than this endpoint accepts")
		case in.Kind == "":
			return nil, badAcceptance(i, "has no kind")
		case len(in.Sources) == 0:
			return nil, badAcceptance(i,
				"binds no container, so it would create a library that can never hold anything")
		}
		sources := make([]store.AcceptedSource, 0, len(in.Sources))
		for _, src := range in.Sources {
			if src.ServiceInstanceID <= 0 {
				return nil, badAcceptance(i, "names a source with no service")
			}
			if src.ContainerRef == "" {
				return nil, badAcceptance(i, "names a source with no container")
			}
			sources = append(sources, store.AcceptedSource{
				ServiceInstanceID: src.ServiceInstanceID,
				// The one kind with a membership derivation, and the only one
				// this endpoint's proposals can name. See acceptedSourceRequest.
				ContainerKind:     "remote_library",
				ContainerRef:      src.ContainerRef,
				ContainerIdentity: src.ContainerName,
			})
		}
		out = append(out, store.LibraryAcceptance{
			Name:      name,
			Kind:      in.Kind,
			Formats:   in.Formats,
			ManagedBy: managedBy(in.Edited),
			Sources:   sources,
		})
	}
	return out, nil
}

// managedBy is the ONE place §17.8's screen fact — this row was edited — becomes
// the stored column's value. See libraryAcceptanceRequest.Edited for why the two
// vocabularies are kept apart.
func managedBy(edited bool) string {
	if edited {
		return "user"
	}
	return "auto"
}

func badAcceptance(i int, what string) error {
	return errStatus(http.StatusBadRequest, CodeBadRequest,
		"proposal "+strconv.Itoa(i)+" "+what)
}

// acceptLibrariesError maps the store's sentinels onto the wire.
//
// # THE TWO REFUSALS ARE DIFFERENT STATUSES BECAUSE THEY ARE DIFFERENT FIXES
//
// A taken name is the user's to fix, in the text box in front of them, so it is
// a 409 with its own code and an action that says so. A source the scope does
// not admit is not: it is a 404 on notFoundOr's exact reasoning — *"does not
// exist" and "exists but is outside your scope" are deliberately
// indistinguishable: the difference is an existence oracle* — and this endpoint
// is one where that matters, since it would otherwise confirm the id of an
// instance the caller cannot see.
//
// # NEITHER BODY CARRIES THE STORE'S OWN SENTENCE, AND THAT IS DELIBERATE
//
// store.ErrLibraryNameTakenAtOtherKind is wrapped with a message naming a
// library row id and a store-vocabulary prefix. The fact the user needs is the
// name they typed, which this layer already has; the row id is an internal
// handle on a library they may not otherwise know exists. So the message is
// built here from the request, and the cause goes to the log through wrapping().
//
// ⚠️ THE 409 COVERS THREE CONDITIONS, ONE CODE, and the message says the true
// thing about all three. That sentinel is returned when the name is held at a
// DIFFERENT kind, when it is held by the reserved `Unfiled` library that nothing
// may join, and when the name is free but its SLUG is not (`Sci-Fi` and `Sci Fi`
// reduce to one permalink). All three are fixed by typing a different name,
// which is why one code is honest here; a message naming only the first would be
// wrong on the other two.
func acceptLibrariesError(err error) error {
	switch {
	case errors.Is(err, store.ErrLibraryNameTakenAtOtherKind):
		return errStatus(http.StatusConflict, CodeLibraryNameTaken,
			"another of your libraries already holds that name, or the web address it would use").
			withAction("Choose a different name").wrapping(err)
	case errors.Is(err, store.ErrSourceOutsideScope):
		return errStatus(http.StatusNotFound, CodeNotFound, "no such service").wrapping(err)
	}
	// Everything else is UsArr's own failure. The store's text is NOT forwarded:
	// it is a database-layer sentence, and the caller can act on none of it.
	return errStatus(http.StatusInternalServerError, CodeInternal,
		"the libraries could not be accepted").wrapping(err)
}
