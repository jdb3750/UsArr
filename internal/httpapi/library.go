package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
// THE WIRE CONTRACT IS WRITTEN DOWN WHERE A CONSUMER WILL FIND IT:
// docs/reference/http-api.md §1. It used to live only in these comments, which
// is how a client came to model `limit` as the value it sent rather than the
// value the server echoed — a doc comment is not reachable from a browser tab.
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
	// IT IS LEGITIMATELY OPTIONAL: a work may genuinely have no blob, the column
	// is nullable, and an absent key is that truth. A renderer treats absence as
	// absence and must not invent a denominator from have_count/want_count.
	//
	// It is ALSO omitted when the stored text is corrupt — see
	// availabilityFor for the four cases and for why every one of them is
	// LOGGED rather than merely dropped. Dropping is the right thing to do on
	// the wire (this struct is marshalled whole, so one bad blob would otherwise
	// fail the entire response and trade the block for its decoration); dropping
	// it *silently* is not, because it made a writer bug and an honestly-empty
	// work byte-identical to a consumer.
	Availability json.RawMessage `json:"availability,omitempty"`
}

type recentWorksResponse struct {
	Items []recentWorkResponse `json:"items"`

	// Limit is the page size the SERVER applied, and it is AUTHORITATIVE: a
	// client pages against this number, never against the one it sent. The
	// server clamps rather than rejects (recentWorksLimit says why), so a client
	// that asked for 10000, got 200 rows and trusted its own 10000 would read
	// the short page as "that is all there is" and stop at the boundary — which
	// is exactly how this field came to be documented properly.
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

	effective, err := recentWorksLimit(r)
	if err != nil {
		return err
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
		out.Items = append(out.Items, s.toRecentWorkResponse(row))
	}
	if next.NullTail || next.AddedAt.Valid {
		out.NextCursor = store.EncodeRecentWorksCursor(next)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// recentWorksLimit reads `?limit=` and returns the page size the server will
// actually apply.
//
// ⚠️ `limit` IS A CLAMP, NOT A VALIDATED RANGE, AND THAT IS THE DECISION rather
// than an accident of the old code. An out-of-range page size is not a request
// the server cannot answer — it is a request it can answer with fewer rows, and
// refusing it would fail a whole screen over a number the server was about to
// ignore anyway. So: any page size at all is served, at most RecentWorksMaxLimit
// of it, and the response's `limit` field says which. The full table:
//
//	absent, empty, or 0   →  RecentWorksDefaultLimit (50). "0" is spelled the
//	                         same as "unspecified" on purpose: a client that
//	                         renders `?limit=${n}` with n unset must get a page,
//	                         not an empty one.
//	1 … 200               →  as asked
//	> 200                 →  200, silently, and echoed back in `limit`
//	negative, or not an
//	integer at all        →  400. There is no honest clamp target for these:
//	                         they are not page sizes that came out too big, they
//	                         are not page sizes.
//
// THE CLAMP IS TOTAL OVER THE POSITIVE INTEGERS, which is why this parses at 64
// bits and treats strconv's range error as the saturation it already is. The
// shared queryInt32 parses at 32, so `?limit=2147483648` used to 400 while
// `?limit=2147483647` clamped to 200 — a cliff at 2^31 with a message claiming
// the value "is not a non-negative integer", which it plainly is. A rule with an
// invisible exception is not a rule a client can hold. queryInt32 is left alone:
// it serves the search endpoints, whose parameters are not this parameter.
func recentWorksLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return store.RecentWorksDefaultLimit, nil
	}
	// On ErrRange, ParseInt returns the saturated bound — MaxInt64 for an
	// enormous positive, MinInt64 for an enormous negative — which is exactly
	// the input the clamp below wants. Any other error means the text was never
	// an integer.
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("limit=%q is not a whole number of items", raw)).
			withAction("omit limit for the default page size, or send a whole number from 1 to " +
				strconv.Itoa(store.RecentWorksMaxLimit))
	}
	if n < 0 {
		return 0, errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("limit=%q is negative, which is not a page size", raw)).
			withAction("omit limit for the default page size, or send a whole number from 1 to " +
				strconv.Itoa(store.RecentWorksMaxLimit))
	}
	switch {
	case n == 0:
		return store.RecentWorksDefaultLimit, nil
	case n > store.RecentWorksMaxLimit:
		return store.RecentWorksMaxLimit, nil
	}
	return int(n), nil
}

// availabilityKinds is schema.md §1's closed set of `"k"` discriminators, and it
// is the FIRST Go-side statement of that vocabulary: nothing in internal/ writes
// an availability blob yet, so until a writer exists this is the only place the
// three shapes are named in code. §1's own rule is that `k` is required on every
// non-null blob and that a renderer switches on it — a fourth value is therefore
// a writer bug, not a newer server, because in v0.1 the writer and this reader
// are the same binary and ship in the same commit. Adding a shape is an edit
// HERE as well as at the writer, and TestAvailabilityKindsMatchSchemaMd is what
// makes that edit deliberate.
var availabilityKinds = map[string]bool{
	"tier":    true, // video: per-quality-tier fractions (ARCHITECTURE §6.3)
	"edition": true, // music: edition-keyed fractions (ADR-0031)
	"count":   true, // comics and anything with no honest denominator (§6.1)
}

// availabilityFor decides what the `availability` key carries for one row, and
// it is where "absent because there is nothing" is separated from "absent
// because what is stored is garbage".
//
// FOUR CASES, ONE WIRE BEHAVIOUR, TWO DIFFERENT SIGNALS:
//
//	NULL column              → omitted, SILENTLY. A work may honestly have no
//	                           blob. This is not a fault and must not be logged,
//	                           or the log stops being worth reading.
//	not JSON, or not an
//	object                   → omitted, LOGGED.
//	an object with no "k"    → omitted, LOGGED. §1: "k is required on every
//	                           non-null blob".
//	an object with a "k"
//	nobody defined           → omitted, LOGGED. Forwarding it would hand a
//	                           renderer a discriminator its switch has no arm
//	                           for, and §6.3's render rule
//	                           (`have == total && total > 0` → ✓) is exactly the
//	                           kind of arm that must not fire by accident on a
//	                           shape nobody specified.
//
// WHY A LOG AND NOT A sync_report ROW. sync_report IS reachable from here —
// s.store.RecordSyncReport exists and takes no Scope — and it was rejected on
// three counts rather than on ignorance. (1) It requires a
// service_instance_id, foreign_keys is ON (internal/db/sqlite.go), and this read
// deliberately never joins service_item_link: naming the instance is the one
// thing this response refuses to publish, so the row cannot be written without
// adding the join the endpoint exists without. (2) It is a WRITE, and this is a
// render path — it would serialise on the single writer connection on the first
// screen the browser asks for, against principle 1. (3) It would append one row
// per corrupt work PER PAGE VIEW, so a refresh loop grows the table without
// bound; migration 0005 calls sync_report "an operational log the sweep writes
// and the Services screen reads", and a browser refresh is not a sweep.
//
// The log line carries the WORK ID, which is what makes the row findable:
// `SELECT availability FROM work WHERE id = ?`. It does not carry the blob —
// that text came out of the database and may be arbitrarily long.
func (s *Server) availabilityFor(w store.RecentWork) json.RawMessage {
	if !w.Availability.Valid {
		return nil
	}
	raw := w.Availability.String

	// One pass, not two: unmarshalling into this probe validates the whole
	// document exactly as json.Valid did AND yields the discriminator, so
	// checking the shape costs nothing over checking the syntax.
	var probe struct {
		K string `json:"k"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		s.log.Warn("work.availability will not decode and was dropped from the response",
			"work_id", w.ID, "err", redactText(err.Error()))
		return nil
	}
	if probe.K == "" {
		s.log.Warn(`work.availability has no "k" discriminator and was dropped from the response`,
			"work_id", w.ID)
		return nil
	}
	if !availabilityKinds[probe.K] {
		s.log.Warn(`work.availability has an unrecognised "k" discriminator and was dropped from the response`,
			"work_id", w.ID, "k", redactText(probe.K))
		return nil
	}
	return json.RawMessage(raw)
}

// toRecentWorkResponse is the allowlist applied. Every field is named; nothing
// is copied by reflection or embedded.
func (s *Server) toRecentWorkResponse(w store.RecentWork) recentWorkResponse {
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
	out.Availability = s.availabilityFor(w)
	return out
}
