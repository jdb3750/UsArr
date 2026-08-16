package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// GET /api/v1/grabs/recent is the local read behind ARCHITECTURE.md §17.5's
// Recent-grabs block: the newest acquisition events, newest first, from SQLite
// and nothing else. No upstream call, no *Arr, no Prowlarr — a grab's record is
// the one thing that must still be there after Prowlarr's 30-minute cache has
// forgotten the release and after UsArr has restarted.
//
// WHAT THIS READ CANNOT SHOW, stated here because a caller will otherwise assume
// the block is complete:
//
//   - A grab that NEVER LEFT THIS PROCESS. provenance is written only after the
//     request was dispatched, so a refused SSRF target, an open circuit breaker,
//     a rejected API key, a Prowlarr 400/409 and a corrupt stored blob leave no
//     row here at all. audit_log is not a substitute today: ListAuditLog takes
//     no scope and no filters, seven pre-dispatch paths in grab.go write no
//     audit row, and the metadata that is written carries no release title. So
//     §17.5's third state — "genuinely failed, nothing was sent" — is ABSENT
//     from this surface rather than mis-rendered by it.
//   - WHICH Prowlarr. provenance has no service_instance_id; source_system is
//     the system name ("prowlarr"), not an instance. With one indexer instance
//     in v0.1 those coincide; with two they do not.
//   - The library or media type a category resolved to. The raw Newznab
//     category ints are here; the resolver lives in internal/servarr/mapping,
//     which this package may not import (doc.go).
//   - Write-queue state (pending|inflight|verifying|done|failed). Nothing writes
//     write_queue yet.

// recentGrabResponse is one acquisition event as it crosses to a client.
//
// There is NO download_url and NO api key, permanently and by construction:
// provenance.download_url is never written in the first place (it is a Prowlarr
// proxy link carrying the full admin key, and provenance rows are immutable and
// outlive everything), and this shape has nowhere to put one. nzb_info_url is
// also deliberately absent — the block does not render it, and an indexer URL is
// where a private tracker's passkey lives.
type recentGrabResponse struct {
	ID int64 `json:"id"`

	// ReleaseTitle is the raw scene/P2P name, verbatim. It is the identifying
	// string for a grab and it is the reason this read can answer "did that one
	// work?" after a restart — release_candidate, the only other place the name
	// exists, is swept 25 minutes after the search.
	ReleaseTitle string `json:"release_title"`

	Protocol    string  `json:"protocol"`
	IndexerName string  `json:"indexer_name,omitempty"`
	Categories  []int32 `json:"categories,omitempty"`

	SizeBytes *int64     `json:"size_bytes,omitempty"`
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
	AcquisitionState string `json:"acquisition_state"`

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
	// The third value, "not_sent", is NOT emitted by this endpoint — not because
	// it cannot happen but because those grabs are not readable at all yet. See
	// the handler note.
	Outcome string `json:"outcome"`
}

// Outcome values ON THE WIRE. Deliberately not derived by trimming or prefixing
// the column: the store's vocabulary is open by design (migration 0003 ships no
// CHECK, and v0.2's request path may want a "pending"), so an unrecognised state
// must land somewhere honest rather than be renamed into one of these two.
//
// THE `wire` PREFIX IS LORE-BEARING, not decoration. grab.go carries a SECOND
// outcome vocabulary — outcomeNotSent / outcomeSentUnknown / outcomeSentConfirmed
// — which is audit_log.metadata_json's, and the two disagree on both spelling and
// membership: "sent_unknown" there is "sent_outcome_unknown" here, and this
// surface has no "not_sent" at all because those grabs write no provenance row.
// They collided on one identifier when the two threads merged. Renaming the
// values to agree would be the wrong repair — audit metadata is an internal
// record with its own history, this one is a published shape web/src/lib/
// requests.ts pins by string — so the NAMES are separated and the strings are
// left exactly as each side ships them.
const (
	wireOutcomeSent         = "sent"
	wireOutcomeSentUnknown  = "sent_outcome_unknown"
	wireOutcomeStateUnknown = "unknown"
)

func toRecentGrabResponse(p store.Provenance) recentGrabResponse {
	out := recentGrabResponse{
		ID:               p.ID,
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

	rows, err := s.store.ListRecentProvenance(r.Context(), storeScope(a), effective)
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your recent grabs could not be read").wrapping(err)
	}

	out := make([]recentGrabResponse, 0, len(rows))
	for _, p := range rows {
		out = append(out, toRecentGrabResponse(p))
	}
	writeJSON(w, http.StatusOK, recentGrabsResponse{Grabs: out, Limit: effective})
	return nil
}
