package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// GET /api/v1/library/recent is Home's Block C: ONE unified recently-added table
// across every media type, newest first, keyset-paginated
// (ARCHITECTURE.md §17.2 as amended by ADR-0028).
//
// Not one strip per media type, and not one endpoint per media type. §17.2's
// arithmetic is explicit about why: six horizontal strips put ~16 items above a
// 900 px fold against the design's own 25-item floor, on the screen whose entire
// job is inventory. A sixth media type must add ROWS to this list, not a sixth
// region to scan — so there is one query, one order and one page.
//
// IT IS A LOCAL READ (principle 1). One SQLite statement per page, no upstream
// call, no *Arr, no metadata provider, no image fetch. This is the endpoint that
// has to be fast, because it is the first thing the browser asks for.
//
// WHAT IT DOES NOT DO YET, stated so a caller does not assume it:
//
//   - No `?lib=` library scope. §17.2's chip is a multi-select over user-defined
//     libraries and it is carried on routes that already exist; adding it here
//     is a join to library_member, whose key leads with sort_title rather than
//     added_at, so it is a different plan and a different commit. The seam is
//     that the scope is a parameter of the store call, not of the SQL string.
//   - No cover art. There is no image endpoint, so shipping poster_asset_id
//     would be an id the client cannot turn into anything.
//   - No Block A and no Block B. Those are a per-type rollup and an attention
//     list; each is its own read.

// recentWorkResponse is one Block C row as it crosses to a browser.
//
// A FIELD-BY-FIELD ALLOWLIST, never a denylist, and the struct IS the allowlist:
// the store's RecentWork is copied into this by hand, so a column added to the
// read cannot reach the wire by default. TestRecentWorksResponseKeysAreTheAllowlist
// pins the key set so growing it is a deliberate edit.
//
// Nothing service-side is here and nothing can be. The `work` row's remote-shaped
// columns — content_hash, size_on_disk — are not read at all, and remote_path
// lives on service_item_link, which this read never touches: it is a filesystem
// path on the upstream's box and §5's own note says it must never reach a
// browser. No service instance is named either; which Kavita a series came from
// is not something Home renders, and naming it would publish the topology of the
// install to every future non-owner user.
type recentWorkResponse struct {
	// ID is work.id, published deliberately. §4.5's Tier 1 client prefix index
	// ships `{id, title, year, kind, availability_state}` for every top-level
	// work by design, and item routes are `/library/{type}/{id}` — so the id is
	// already the catalogue's public name for a row. It is NOT the shape
	// provenance.id has: the catalogue is shared rather than per-user, so a gap
	// between two ids says how many works exist, not what somebody else
	// acquired. That is why grabs.go publishes an HMAC and this does not.
	ID int64 `json:"id"`

	// MediaType is §17.2's six-value navigation enum — movies, tv, music,
	// ebooks, audiobooks, comics — and it is what the Type column renders.
	//
	// RESOLVED SERVER-SIDE, WHICH IS NOT AN IMPLEMENTATION DETAIL. §17.2 states
	// that the Tier 1 client index carries no format, so a client CANNOT
	// separate ebooks from audiobooks: "the Ebooks/Audiobooks split is
	// server-side only in v0.1". If this field were derived in the browser from
	// Kind, two of the six chips would be one chip.
	MediaType string `json:"media_type"`

	// Kind is work.kind verbatim, and it travels beside MediaType rather than
	// instead of it because they answer different questions: kind is the
	// schema's word, media type is the user's. A client that needs to build a
	// link needs the first; a client that needs to render the Type cell needs
	// the second.
	Kind string `json:"kind"`

	Title string `json:"title"`

	// Year is absent rather than 0 when the column is NULL. A rendered "0"
	// is a claim about a release date; an absent key is the truth.
	Year *int64 `json:"year,omitempty"`

	// AddedAt is the sort key, and it is absent when the upstream reported no
	// creation date — which is a state Kavita can reach. An absent value sorts
	// LAST, never first: §17.2's block answers "what did I just get", and a row
	// with no date must not be able to claim the top of it.
	AddedAt *time.Time `json:"added_at,omitempty"`

	// HaveCount and WantCount are work's denormalised rollups, and they are the
	// numerator and the gap behind §17.2's `have / total · N missing` grammar.
	HaveCount int64 `json:"have_count"`
	WantCount int64 `json:"want_count"`

	// Availability is schema.md §1's polymorphic blob, forwarded verbatim as
	// JSON. It is POLYMORPHIC BY MEDIUM and carries a "k" discriminator as its
	// first key precisely so a renderer switches on it — flattening it here
	// would destroy the distinction between a tier fraction, an edition-keyed
	// fraction and a comic's `have` with `total: null`, and §6.3's render rule
	// (`have == total && total > 0` → ✓) must never fire on the last of those.
	//
	// It is omitted when the column is NULL and when the stored text is not
	// valid JSON. The second case is dropped rather than forwarded because this
	// struct is marshalled as a whole: one malformed historical blob would
	// otherwise fail the entire response, which trades the block for its
	// decoration.
	Availability json.RawMessage `json:"availability,omitempty"`
}

type recentWorksResponse struct {
	Items []recentWorkResponse `json:"items"`

	// Limit is the limit the SERVER applied, echoed because the server clamps.
	// A client that asked for 10000 and got 200 rows would otherwise read the
	// short answer as "that is all there is".
	Limit int `json:"limit"`

	// NextCursor is absent when this page is the last one, and its absence is
	// the "Load more" button's off switch. It is absent rather than empty for
	// the same reason Year is: `""` reads as a cursor whose value is unknown.
	NextCursor string `json:"next_cursor,omitempty"`
}

// handleRecentWorks serves one keyset page of Block C.
func (s *Server) handleRecentWorks(w http.ResponseWriter, r *http.Request) error {
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
		effective = store.RecentWorksDefaultLimit
	case effective > store.RecentWorksMaxLimit:
		effective = store.RecentWorksMaxLimit
	}

	// A cursor that will not parse is a 400, never a silent reset to page one:
	// resetting turns a stale bookmark into a Load-more loop that re-serves the
	// first page for ever and looks like the list is stuck.
	cur, err := store.DecodeRecentWorksCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
	if err != nil {
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"the cursor on this request is not one this endpoint issued").
			withAction("reload the page to start from the newest items").
			wrapping(err)
	}

	// SCOPE. storeScope derives it from the session and nothing else — the
	// caller cannot widen it with a query parameter, which is the whole reason
	// it is not one. v0.1's owner gets the full scope; a non-owner gets an empty
	// visible set, which the store renders as `1=0` and which therefore returns
	// no rows rather than every row.
	rows, next, err := s.store.ListRecentWorks(r.Context(), storeScope(a), cur, effective)
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your library could not be read").wrapping(err)
	}

	out := recentWorksResponse{
		Items: make([]recentWorkResponse, 0, len(rows)),
		Limit: effective,
	}
	for _, row := range rows {
		out.Items = append(out.Items, toRecentWorkResponse(row))
	}
	if next.NullTail || next.AddedAt.Valid {
		out.NextCursor = store.EncodeRecentWorksCursor(next)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// toRecentWorkResponse is the allowlist applied. Every field is named; nothing
// is copied by reflection or embedded.
func toRecentWorkResponse(w store.RecentWork) recentWorkResponse {
	out := recentWorkResponse{
		ID:        w.ID,
		MediaType: w.MediaType,
		Kind:      w.Kind,
		Title:     w.Title,
		HaveCount: w.HaveCount,
		WantCount: w.WantCount,
	}
	if w.Year.Valid {
		year := w.Year.Int64
		out.Year = &year
	}
	// A stored timestamp that will not parse is dropped rather than guessed at,
	// exactly as grabs.go drops an unparseable grabbed_at: a wrong date on the
	// recently-added table is worse than a missing one, because the whole column
	// is an ordering claim.
	if w.AddedAt.Valid {
		if at, err := store.ParseTime(w.AddedAt.String); err == nil {
			out.AddedAt = &at
		}
	}
	if w.Availability.Valid && json.Valid([]byte(w.Availability.String)) {
		out.Availability = json.RawMessage(w.Availability.String)
	}
	return out
}
