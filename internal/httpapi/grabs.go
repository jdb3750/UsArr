package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// GET /api/v1/grabs/recent is the local read behind ARCHITECTURE.md §17.5's
// Recent-grabs block: the newest acquisition events, newest first, from SQLite
// and nothing else. No upstream call, no *Arr, no Prowlarr — a grab's record is
// the one thing that must still be there after Prowlarr's 30-minute cache has
// forgotten the release and after UsArr has restarted.
//
// THE READ IS A UNION OF TWO TABLES, and the cut between them is not
// success-versus-failure:
//
//	provenance — every grab that was HANDED OVER, discriminated on the row by
//	             acquisition_state into "sent" and "sent, outcome unknown".
//	audit_log  — every grab that provably NEVER LEFT THIS PROCESS: a refused
//	             SSRF target, an open breaker, a rejected API key, a Prowlarr
//	             400/409, a corrupt stored blob, and the four pre-dispatch
//	             returns auditNotSent covers. These write no provenance row at
//	             all, so this table is the only record they have.
//
// THE ARMS ARE DISJOINT BY CONSTRUCTION, and that is the one invariant this
// handler must not lose. A grab whose outcome is unknown writes BOTH a
// provenance row (acquisition_state='unconfirmed') AND an audit row — with
// result='warn', carrying the provenance id. The audit arm reads result='fail'
// and nothing else (store.NotSentGrabsQuery), so the warn row is never picked
// up and the grab appears exactly once. Widening that filter to "every failed-
// looking row" would render one grab twice, once as sent-unknown and once as a
// failure, which is precisely the double-grab invitation the three-state model
// exists to prevent — printed twice on one screen.
//
// WHAT THIS READ STILL CANNOT SHOW, stated here because a caller will otherwise
// assume the block is complete:
//
//   - A grab with no session behind it. Two of handleGrab's early returns write
//     no audit row because no candidate id exists yet (a malformed path id, an
//     undecodable body), and the no-session return writes none because
//     actor_user_id would be NULL and the scoped read could never return it.
//     Neither describes a grab.
//   - WHICH Prowlarr. provenance has no service_instance_id; source_system is
//     the system name ("prowlarr"), not an instance. With one indexer instance
//     in v0.1 those coincide; with two they do not.
//   - The library or media type a category resolved to. The raw Newznab
//     category ints are here; the resolver lives in internal/servarr/mapping,
//     which this package may not import (doc.go).
//   - Write-queue state. The vocabulary is store.ValidWriteQueueState's and
//     nowhere else's — this list used to restate it as
//     `pending|inflight|verifying|done|failed`, which had been missing
//     'awaiting_choice' since ADR-0039. Nothing in v0.1 writes write_queue; the
//     one writer in the tree is the bench fixture behind `//go:build bench`.

// recentGrabResponse is one acquisition event as it crosses to a client.
//
// There is NO download_url and NO api key, permanently and by construction:
// provenance.download_url is never written in the first place (it is a Prowlarr
// proxy link carrying the full admin key, and provenance rows are immutable and
// outlive everything), and this shape has nowhere to put one. nzb_info_url is
// also deliberately absent — the block does not render it, and an indexer URL is
// where a private tracker's passkey lives.
type recentGrabResponse struct {
	// ID is an OPAQUE, STABLE row key — never provenance.id, and never
	// audit_log.id.
	//
	// Review finding RG-01.3. provenance.id is INTEGER PRIMARY KEY, so it is a
	// single monotonic sequence shared by every user. A caller who sees 104 and
	// later 341 on two of their OWN grabs learns that 236 provenance rows were
	// written in between, and under multi-user — which the schema is built for
	// from migration 0001 — those rows are other people's. That is a volume and
	// existence oracle, and it SURVIVES THE ACCESS SCOPE FILTER: the filter
	// decides which rows come back, not what their ids say about the ones that
	// did not. Principle 4 names this shape exactly.
	//
	// What replaces it is grabRowID's keyed hash. It is a string on the wire
	// because it is not a number and nothing may treat it as one — the client
	// keys rows with it for focus and hover and does nothing else with it.
	//
	// THE NOT-SENT ARM GETS THE SAME TREATMENT FROM A DIFFERENT KEY.
	// audit_log.id is an INTEGER PRIMARY KEY too, and a globally monotonic one,
	// so shipping it raw would reopen exactly the oracle this closed — on a
	// table that records every user's actions rather than only their grabs.
	// auditRowID derives it identically under a key from a distinct HKDF label,
	// so the two families cannot collide even where the two sequences happen to
	// have handed out the same integer.
	ID string `json:"id"`

	// ReleaseTitle is the raw scene/P2P name, verbatim. It is the identifying
	// string for a grab and it is the reason this read can answer "did that one
	// work?" after a restart — release_candidate, the only other place the name
	// exists, is swept 25 minutes after the search.
	ReleaseTitle string `json:"release_title"`

	Protocol    string  `json:"protocol"`
	IndexerName string  `json:"indexer_name,omitempty"`
	Categories  []int32 `json:"categories,omitempty"`

	// SizeBytes is absent on a not-sent row, and absent rather than zero. Size
	// lives on the provenance row, and a grab that was never handed over has
	// none — 0 would render as "0 B", which is a claim about a file that does
	// not exist.
	SizeBytes *int64 `json:"size_bytes,omitempty"`

	// GrabbedAt is the time §17.5's first column renders, and it is the SORT KEY
	// the union merges on, which is why both arms fill it rather than carrying a
	// second field for the second arm.
	//
	// It is the time of the ATTEMPT, on both arms. On a provenance row that is
	// provenance.grabbed_at, written at dispatch and not at completion — UsArr
	// stops observing after handoff and never learns when a download finished.
	// On a not-sent row it is the audit row's ts, which is when the user pressed
	// Grab. So the column is honestly labelled "Time", never "Downloaded".
	GrabbedAt *time.Time `json:"grabbed_at,omitempty"`

	// DownloadID is the nzo_id or, for a torrent, the infohash. It is here
	// because §17.5's ambiguous row sends the user to their download client —
	// where the truth actually lives — and this is the string that finds the
	// entry there. It is not a secret: an infohash is derivable from the torrent
	// by anyone, and the row is already inside the caller's access scope.
	DownloadID string `json:"download_id,omitempty"`

	// SourceSystem is the SYSTEM that produced the acquisition, not the
	// instance. See the note above the handler.
	SourceSystem string `json:"source_system,omitempty"`

	// AcquisitionState is the column verbatim, so an open vocabulary the schema
	// deliberately does not police with a CHECK cannot be flattened on the way
	// out. Today: "confirmed" or "unconfirmed".
	//
	// ITS ABSENCE IS INFORMATION. A not-sent row has no provenance row and
	// therefore no acquisition state at all, so the key is omitted rather than
	// sent empty: "" would read as a state whose name nobody knows, which is a
	// different and wrong claim. Absent means "there is no provenance row here".
	AcquisitionState string `json:"acquisition_state,omitempty"`

	// ErrorCode is the apiError code the failed grab returned — "expired",
	// "no_longer_offered", "no_download_client", "grab_failed" and the rest of
	// errorcodes.go. Present only on the not-sent arm; a handed-over grab has no
	// error to name.
	//
	// A CLOSED VOCABULARY, AND DELIBERATELY NOT THE ERROR PROSE. §17.5 asks a
	// failed row to carry the upstream error, and the code carries its identity
	// while the message carries its text. The code is what the row actually
	// needs: §17.5's action rule keys on WHICH failure this was — "Search again"
	// on an expired or withdrawn release, nothing offered on a Prowlarr-side
	// rejection the user cannot act on — so the client has to branch on the code
	// whether or not it also has the sentence.
	//
	// The sentence is withheld from THIS surface on purpose. It is assembled
	// from an upstream error string and passed through redactText, which strips
	// credentials out of a URL but leaves scheme://host behind; shipping it here
	// would mean loosening this endpoint's response-bytes guard (no URL, no
	// passkey, no key) to accommodate a field, which trades a guard for a
	// nicety. The user was already shown that sentence, verbatim, on the grab
	// itself — the live error is where prose belongs, and this is the history.
	ErrorCode string `json:"error_code,omitempty"`

	// Outcome is AcquisitionState rendered in the vocabulary §17.5 specifies,
	// and it exists because "unconfirmed" reads as failure and is not one.
	//
	// The boundary that matters is HANDED-OVER versus NOT-HANDED-OVER, not
	// success versus failure. Prowlarr adds the release to the download client
	// BEFORE it configures it and never rolls back, so a 5xx can cover an
	// operation that already partly succeeded — the owner's own grab downloaded
	// end to end in Deluge while UsArr reported "Grab failed — HTTP 502". Both
	// values below therefore begin "sent", and a row carrying either one must
	// never be worded as though the grab did not happen, and must never offer
	// Retry on the unknown one: retrying is exactly what produces two copies of
	// a 68 GB release.
	//
	// The third value, wireOutcomeNotSent, is the other side of that boundary:
	// UsArr knows nothing was handed over, so the row may be worded as a
	// failure, and it is the ONLY one of the three that may offer an action
	// which grabs again.
	Outcome string `json:"outcome"`
}

// Outcome values ON THE WIRE. Deliberately not derived by trimming or prefixing
// the column: the store's vocabulary is open by design (migration 0003 ships no
// CHECK, and v0.2's request path may want a "pending"), so an unrecognised state
// must land somewhere honest rather than be renamed into one of these two.
//
// THE `wire` PREFIX IS LORE-BEARING, not decoration. grab.go carries a SECOND
// outcome vocabulary — outcomeNotSent / outcomeSentUnknown / outcomeSentConfirmed
// — which is audit_log.metadata_json's, and the two disagree on spelling:
// "sent_unknown" there is "sent_outcome_unknown" here. They collided on one
// identifier when the two threads merged. Renaming the values to agree would be
// the wrong repair — audit metadata is an internal record with its own history,
// this one is a published shape web/src/lib/requests.ts pins by string — so the
// NAMES are separated and the strings are left exactly as each side ships them.
//
// wireOutcomeNotSent AND outcomeNotSent HAPPEN TO SPELL THE SAME STRING, and
// that agreement is a coincidence rather than a contract. It is not collapsed
// into one constant precisely because the two vocabularies are free to diverge
// — the pair above already has — and a single shared identifier would make a
// change to the internal record silently republish itself on the wire. Anything
// that needs them equal must assert it; nothing does.
const (
	wireOutcomeSent         = "sent"
	wireOutcomeSentUnknown  = "sent_outcome_unknown"
	wireOutcomeStateUnknown = "unknown"

	// wireOutcomeNotSent is §17.5's third state: UsArr knows the release never
	// left this process. It is the only outcome that may be worded as "it did
	// not happen", and the only one whose row may offer an action that grabs
	// again — the other two were handed over, and a retry there is how a user
	// ends up with two copies of a 68 GB release.
	wireOutcomeNotSent = "not_sent"
)

// grabRowIDBytes is how much of the HMAC ships. 128 bits is far past any
// collision or guessing concern for a per-install row key, and truncating a
// SHA-256 HMAC is the standard construction (RFC 2104 §5). The value is 22
// base64url characters, which stays copy-safe and short enough to sit in a DOM
// attribute without comment.
const grabRowIDBytes = 16

// grabRowID renders a provenance row's identity for the wire.
//
// TWO PROPERTIES, and every choice here serves one of them.
//
// STABLE. The key comes from crypto.DeriveGrabRowIDKey over the master secret
// and the KEK salt, so it is the same after a restart, and the input is the
// row's immutable identity. A random per-response token would be simpler and
// would break identity-keyed focus and hover on every poll.
//
// CARRYING NO ORDER OR VOLUME. HMAC-SHA256 output is indistinguishable from
// random without the key, so adjacent rowids give unrelated ids: an attacker
// holding two of their own learns nothing about what was written between them.
// That is also why this is a keyed hash and not a per-user sequence — a
// sequence would need a migration, and the schema is where this project's
// expensive mistakes live.
//
// USER_ID IS IN THE INPUT, deliberately. It costs one field and buys per-user
// domain separation: the same row seen by two users — the shared/system
// sentinel 0 rows migration 0002 backfilled, or any future view that widens the
// scope — hashes to two unrelated ids, so nobody can correlate across users by
// comparing tokens. Nothing joins on this value, so there is no cost to it
// differing per user. provenance.user_id is historical and never rewritten, so
// stability holds.
//
// Both inputs are fixed-width big-endian rather than text, so there is no
// separator to get wrong: (user 1, row 23) and (user 12, row 3) cannot render
// the same 16 bytes.
func grabRowID(key []byte, userID, rowID int64) string {
	mac := hmac.New(sha256.New, key)
	var buf [8]byte
	// #nosec G115 -- int64 -> uint64 here is a bit reinterpretation into an
	// HMAC input, not an arithmetic conversion. Every int64 maps to a distinct
	// uint64, which is the only property this needs: nothing reads the value
	// back, and a negative id (impossible for a rowid) would still hash
	// injectively rather than wrap into another row's identity.
	binary.BigEndian.PutUint64(buf[:], uint64(userID))
	mac.Write(buf[:])
	// #nosec G115 -- see above.
	binary.BigEndian.PutUint64(buf[:], uint64(rowID))
	mac.Write(buf[:])
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:grabRowIDBytes])
}

// auditRowID renders an audit row's identity for the wire.
//
// It is grabRowID's construction over grabRowID's inputs, under a DIFFERENT KEY
// — crypto.DeriveAuditRowIDKey's, from its own HKDF label. Every property in
// grabRowID's comment carries over unchanged; only the domain differs, and this
// is the whole of why the domains cannot collide:
//
// provenance.id and audit_log.id are two INDEPENDENT monotonic sequences, so
// (user 3, row 41) names a different row in each, and ONE RESPONSE CARRIES BOTH
// ARMS. Under a shared key those two rows would hash to the same token, the
// client would key both on one id, and focus, hover and any future row action
// would land on whichever arrived last. Separating them by key rather than by a
// prefix byte means the collision is not merely improbable but unconstructible
// without the secret, and it costs one HKDF label — the seam the derivation was
// built with.
func auditRowID(key []byte, userID, rowID int64) string {
	return grabRowID(key, userID, rowID)
}

func (s *Server) toRecentGrabResponse(p store.Provenance) recentGrabResponse {
	out := recentGrabResponse{
		ID:               grabRowID(s.cfg.GrabRowIDKey, p.UserID, p.ID),
		ReleaseTitle:     p.ReleaseTitle,
		Protocol:         p.Protocol,
		IndexerName:      p.IndexerName,
		Categories:       decodeCategories(p.IndexerCategories),
		DownloadID:       p.DownloadID,
		SourceSystem:     p.SourceSystem,
		AcquisitionState: p.AcquisitionState,
		Outcome:          outcomeFor(p.AcquisitionState),
	}
	if p.SizeBytes.Valid {
		size := p.SizeBytes.Int64
		out.SizeBytes = &size
	}
	// A grabbed_at that will not parse is dropped rather than guessed at. A
	// wrong timestamp on an irreversible action is worse than a missing one:
	// §17.5's block is how a user answers "did I already grab this an hour ago?".
	if p.GrabbedAt.Valid {
		if at, err := store.ParseTime(p.GrabbedAt.String); err == nil {
			out.GrabbedAt = &at
		}
	}
	return out
}

// toNotSentGrabResponse renders one definite failure — an audit_log row — into
// the same shape as a handed-over grab.
//
// It reports ok=false rather than a zero row, and the one caller drops the row.
// The rejected case is a NULL actor_user_id, and asserting it here is the point
// of the second return value: audit_log.actor_user_id is nullable (unlike
// provenance.user_id), and the scoped predicate `actor_user_id IN (0, :uid)`
// already makes such a row unreadable, because `NULL IN (0, 1)` is unknown
// rather than true. So one cannot arrive — but if the predicate ever changed,
// sql.NullInt64's zero value would silently feed user 0 into the HMAC and mint a
// stable id in the WRONG DOMAIN, colliding with the shared/system sentinel's
// rows. A row that vanishes is a bug someone notices; a row with a plausible id
// belonging to someone else is not.
//
// A row whose metadata will not parse still renders, with whatever the columns
// themselves carry. Same reasoning as decodeCategories: the block exists to
// answer "did that one work?", and losing the whole row because one historical
// blob has an unexpected shape trades the answer for its decoration.
func (s *Server) toNotSentGrabResponse(e store.AuditEntry) (recentGrabResponse, bool) {
	if !e.ActorUserID.Valid {
		return recentGrabResponse{}, false
	}

	var meta grabAuditMeta
	// The error is deliberately not propagated; see above.
	_ = json.Unmarshal([]byte(e.MetadataJSON), &meta)

	at := e.TS
	return recentGrabResponse{
		ID: auditRowID(s.cfg.AuditRowIDKey, e.ActorUserID.Int64, e.ID),
		// Empty on the candidate-not-found path, where the listing had already
		// been swept and the title is genuinely unknown. Omitted rather than
		// guessed at — "the listing had been swept" is the honest answer, and
		// inventing a name for a release nobody can identify is worse than a
		// blank.
		ReleaseTitle: meta.ReleaseTitle,
		Protocol:     meta.Protocol,
		IndexerName:  meta.Indexer,
		GrabbedAt:    &at,
		ErrorCode:    meta.Error,
		Outcome:      wireOutcomeNotSent,
	}, true
}

func outcomeFor(state string) string {
	switch state {
	case store.AcquisitionConfirmed:
		return wireOutcomeSent
	case store.AcquisitionUnconfirmed:
		return wireOutcomeSentUnknown
	default:
		return wireOutcomeStateUnknown
	}
}

// decodeCategories reads provenance.indexer_categories, which is stored as a
// JSON array of RAW Newznab ints and must not be collapsed.
//
// A blob that will not parse yields no categories rather than failing the whole
// response: this is a decorative column on the row, and losing the entire
// Recent-grabs block because one historical row holds an unexpected shape would
// trade the read for its least important field.
func decodeCategories(raw string) []int32 {
	if raw == "" {
		return nil
	}
	var cats []int32
	if err := json.Unmarshal([]byte(raw), &cats); err != nil {
		return nil
	}
	return cats
}

type recentGrabsResponse struct {
	Grabs []recentGrabResponse `json:"grabs"`

	// Limit is the limit the server actually applied. It is echoed because the
	// server clamps: a client that asked for 10000 and got 200 rows would
	// otherwise read the short answer as "that is all there is".
	Limit int `json:"limit"`
}

func (s *Server) handleRecentGrabs(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	limit, err := queryInt32(r, "limit")
	if err != nil {
		return err
	}
	effective := int(limit)
	switch {
	case effective <= 0:
		effective = store.RecentProvenanceDefaultLimit
	case effective > store.RecentProvenanceMaxLimit:
		effective = store.RecentProvenanceMaxLimit
	}

	scope := storeScope(a)

	// Both arms are read at the FULL limit, then merged and truncated. That is
	// not waste: the newest N of a union is not the newest N of either side, so
	// asking each for fewer would drop rows that belong in the answer whenever
	// one arm holds most of the recent activity — which is exactly what a broken
	// stack looks like, and exactly when this block matters.
	sent, err := s.store.ListRecentProvenance(r.Context(), scope, effective)
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your recent grabs could not be read").wrapping(err)
	}
	// The failures are read as a separate query rather than a SQL UNION: the two
	// tables share no column names, no key and no shape, and the audit side
	// needs its metadata JSON decoded in Go regardless. A UNION would buy one
	// round trip against the local file and cost the query-plan guarantee on
	// each arm, which is the thing actually worth keeping.
	notSent, err := s.store.ListNotSentGrabs(r.Context(), scope, effective)
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your recent grabs could not be read").wrapping(err)
	}

	out := make([]recentGrabResponse, 0, len(sent)+len(notSent))
	for _, p := range sent {
		out = append(out, s.toRecentGrabResponse(p))
	}
	for _, e := range notSent {
		row, ok := s.toNotSentGrabResponse(e)
		if !ok {
			// Unreachable through the scoped read, which cannot return an
			// actorless row. Logged rather than dropped silently, because if it
			// ever happens the scope predicate has changed underneath this.
			s.log.Error("recent grabs: audit row has no actor and was dropped", "audit_id", e.ID)
			continue
		}
		out = append(out, row)
	}

	// One order across both arms, newest first, on the time column §17.5
	// renders. SliceStable rather than Slice so rows sharing a timestamp keep a
	// deterministic order between polls: grabbed_at has one-second resolution and
	// audit ts has the same, so ties are ordinary rather than exotic, and a list
	// that reshuffles under the cursor between two polls breaks identity-keyed
	// focus just as thoroughly as an unstable id would.
	sort.SliceStable(out, func(i, j int) bool {
		return sortTimeOf(out[i]).After(sortTimeOf(out[j]))
	})
	if len(out) > effective {
		out = out[:effective]
	}
	writeJSON(w, http.StatusOK, recentGrabsResponse{Grabs: out, Limit: effective})
	return nil
}

// sortTimeOf is the union's sort key. A row with no usable timestamp — a
// provenance row whose grabbed_at would not parse — sorts last rather than
// first: the zero time is older than everything, so an unreadable date cannot
// promote a row to the top of a list whose whole purpose is recency.
func sortTimeOf(g recentGrabResponse) time.Time {
	if g.GrabbedAt == nil {
		return time.Time{}
	}
	return *g.GrabbedAt
}
