package httpapi

import (
	"database/sql"
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
// THIS HANDLER READS `limit` AND `cursor` AND NOTHING ELSE — the parameter
// table is docs/reference/http-api.md §1.1. Unrecognised parameters are ignored
// rather than refused, so `?lib=…` sent here does not 400; it answers 200 over
// the whole catalogue. "This endpoint has no such filter" is therefore a silent
// answer, and only legible to a caller who knows where the filter does live:
//
//   - `?lib=` IS BUILT, on handleBrowseWorks below — GET /api/v1/library,
//     http-api.md §7.3. Slugs are resolved by store.LibraryIDsBySlug and
//     applied as store.WorksFilter.LibraryIDs. ⚠️ This bullet used to read
//     "No `?lib=` library scope", and gave as its reason that the scope would
//     be "a join to library_member, whose key leads with sort_title rather
//     than added_at, so it is a different plan and a different commit". Both
//     halves went stale: the commit landed, and ADR-0051 made the scope a
//     WORK-DRIVEN EXISTS over library_member rather than a join, which is
//     order-independent and so was never blocked by this endpoint's order.
//     Block C carries no chip because §17.2 gives it one table, one order and
//     no filters — not because the scope could not be built here.
//
//     Should Block C ever be given the chip, it MUST reuse that resolver and
//     that EXISTS, and it MUST refuse an unresolvable slug rather than drop it:
//     §7.3's rule is that dropping a slug WIDENS the page, so a filter has no
//     safe direction to fail in the way `limit` does. The seam is unchanged —
//     the scope is a parameter of the store call, not of the SQL string.
//
// WHAT IS GENUINELY NOT BUILT ANYWHERE, checked one item at a time rather than
// asserted as a list:
//
//   - No cover-art BYTES, here or on the grid — but ⚠️ THIS BULLET USED TO READ
//     "No cover art ... server.go routes no image endpoint at all, so shipping
//     poster_asset_id would be an id the client cannot turn into anything", AND
//     BOTH HALVES ARE NOW FALSE. `GET /img/{key}` is routed (images.go), and
//     both this response and the grid's carry `poster_key` —
//     `image_asset.cache_key`, which resolves through that route. What is not
//     built is the WIRING of §4.4's fetch half. ⚠️ "nothing writes
//     `image_asset`" was true when this bullet was written and is not any more:
//     internal/imagepipeline fetches, renders and records a poster, and
//     internal/store's PutPosterAsset writes the row. What is still true of a
//     real install is that no import calls it yet, so the key is absent on
//     every row and every /img request still answers not_cached.
//   - No Block A. It is §17.2's per-type rollup row. ⚠️ THIS BULLET USED TO
//     READ "No Block A and no Block B ... and server.go routes neither", AND
//     BOTH HALVES OF THAT WERE WRONG BY THE TIME IT WAS READ. Block A's
//     per-type COUNT is routed — facets.go, GET /api/v1/library/facets,
//     http-api.md §8 — so what is still unbuilt is Block A itself and the rest
//     of its row: the availability rollup and the last-import time are further
//     aggregates and nothing serves them. AND BLOCK B IS DRAWN. It is the
//     attention block, computed in web/src/lib/home.ts from
//     GET /api/v1/services/health and rendered by web/src/routes/+page.svelte,
//     so it hangs off SERVICE state rather than off a catalogue route — which
//     is why nothing in this file serves it, and why its absence from this
//     file is not evidence of its absence from the screen.

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

	// PosterKey is `image_asset.cache_key` for the work's poster: the key
	// `GET /img/{key}` addresses, which is the ONLY way artwork reaches a
	// client. A client builds `/img/{poster_key}?w=342` from it; see
	// images.go for the route and internal/imagecache for the width allowlist.
	//
	// ABSENT rather than empty when the work has no poster asset. ⚠️ That used
	// to be every work; internal/imagepipeline writes posters now, though no
	// import calls it yet, so on a real install it is still every work. `""`
	// would read as a key whose value is unknown, and a renderer that built a
	// URL from it would request `/img/` on every row.
	//
	// ⚠️ IT IS NOT AN EXISTENCE ORACLE, structurally rather than carefully. It
	// is a column of a row this response already carries, selected under the
	// same access-scope predicate: a work the scope hides has no row here, so it
	// has no key here either, and there is no way to ask for the key without
	// asking for the work. store.PosterKeyExpr carries the argument;
	// internal/store's TestLookupImageAssetScopeActuallyFilters proves it by
	// deleting the predicate rather than by threading a parameter.
	//
	// ⚠️ IT IS THE cache_key AND NOT `work.poster_asset_id`. This response used
	// to ship neither, and http-api.md §7.1 gave as the reason that
	// poster_asset_id "would be an id the client cannot turn into anything".
	// Now that the route exists, the thing a client can turn into something is
	// the key the route is spelled with — ARCHITECTURE.md §4.1 and §13 both say
	// `/img/{cache_key}` — so that is what ships.
	PosterKey string `json:"poster_key,omitempty"`
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
func (s *Server) availabilityFor(workID int64, blob sql.NullString) json.RawMessage {
	if !blob.Valid {
		return nil
	}
	raw := blob.String

	// One pass, not two: unmarshalling into this probe validates the whole
	// document exactly as json.Valid did AND yields the discriminator, so
	// checking the shape costs nothing over checking the syntax.
	var probe struct {
		K string `json:"k"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		s.log.Warn("work.availability will not decode and was dropped from the response",
			"work_id", workID, "err", redactText(err.Error()))
		return nil
	}
	if probe.K == "" {
		s.log.Warn(`work.availability has no "k" discriminator and was dropped from the response`,
			"work_id", workID)
		return nil
	}
	if !availabilityKinds[probe.K] {
		s.log.Warn(`work.availability has an unrecognised "k" discriminator and was dropped from the response`,
			"work_id", workID, "k", redactText(probe.K))
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
	out.Availability = s.availabilityFor(w.ID, w.Availability)
	out.PosterKey = posterKeyFor(s, w.ID, w.PosterKey)
	return out
}

// posterKeyFor is the poster key as it crosses to a browser, or "" when there is
// none to send.
//
// A KEY THAT IS NOT SHAPED LIKE ONE IS DROPPED RATHER THAN SHIPPED. The client
// turns this into a URL path segment, and store.ValidImageCacheKey is the same
// shape the /img route refuses at — so a malformed value would produce a row
// whose cover 404s on every render, with nothing saying why. It is LOGGED rather
// than merely dropped, for the reason availabilityFor logs its four cases: a
// writer bug and an honestly-absent poster would otherwise be byte-identical to
// a consumer.
//
// It is one function for the three responses that carry the field, because the
// three render one row component and a second copy of this rule is a second
// thing to keep in step.
func posterKeyFor(s *Server, workID int64, key sql.NullString) string {
	if !key.Valid {
		return ""
	}
	if !store.ValidImageCacheKey(key.String) {
		// The work id, not the key: the key came out of the database and the
		// row is what makes this findable —
		// `SELECT cache_key FROM image_asset ia JOIN work w ON w.poster_asset_id = ia.id WHERE w.id = ?`.
		s.log.Warn("image_asset.cache_key is not a cache key and was dropped from the response",
			"work_id", workID)
		return ""
	}
	return key.String
}

// GET /api/v1/library is §17.2's per-type library grid and §17.8's library
// scope chip: the same corpus and the same row as Block C above, filtered by
// media type and by library, in one of three orders, keyset-paginated.
//
// IT IS A DIFFERENT ENDPOINT FROM /library/recent AND NOT A SUPERSET OF IT.
// §17.2 is emphatic that Block C is ONE table with ONE order and no filters — a
// sixth media type adds rows to it, never a sixth region — so collapsing the two
// into `/library/recent?media_type=…` would make the Home block a special case
// of the grid and put a filter on the endpoint whose whole design is that it has
// none. They share the row shape (recentWorkResponse) and the allowlist that
// builds it (toRecentWorkResponse), by calling them.
//
// IT IS A LOCAL READ (principle 1). One SQLite statement per page — two on the
// added_at/undated boundary internal/store documents — plus at most one small
// statement to resolve `?lib=` slugs. No upstream call, no *Arr, no metadata
// provider, no image fetch.
//
// THE WIRE CONTRACT IS IN docs/reference/http-api.md §7, where a consumer can
// find it. This comment is not reachable from a browser tab.
//
// ⚠️ A PARAMETER NAME THIS HANDLER DOES NOT READ IS IGNORED, NOT REFUSED, and
// that is an API-WIDE contract — http-api.md's preamble, not a quirk of this
// endpoint. Adding a "reject unknown parameters" loop here would be a wire
// change that breaks forward compatibility in both directions, and it is what
// TestUnrecognisedQueryParametersAreIgnoredNotRefused exists to catch. The typo
// hazard it leaves is answered instead by refusing a bad VALUE on a name this
// handler does read, and by the envelope echoing what was applied.
//
// WHAT IT DOES NOT DO YET: no cover-art BYTES, and no facet counts beside the
// chips.
//
// ⚠️ THIS SENTENCE USED TO READ "no cover art (there is no image endpoint, so
// poster_asset_id would be an id the client cannot turn into anything)", AND
// BOTH HALVES ARE FALSE NOW. `GET /img/{key}` is routed (images.go) and this
// response carries `poster_key`, the `image_asset.cache_key` that route takes.
// What is unbuilt is the WIRING of §4.4's fetch half. ⚠️ "nothing writes
// `image_asset`" is no longer true — internal/imagepipeline renders a poster and
// internal/store's PutPosterAsset records it — but nothing CALLS it during an
// import, so on a real install the key is still absent on every row and every
// /img request still answers not_cached.
//
// ⚠️ THE FACET COUNTS ARE BUILT — they are just not on THIS response. See
// facets.go and http-api.md §8: GET /api/v1/library/facets answers all six in
// one call. It is a separate endpoint rather than a field here for the reason
// browseWorksResponse gives above ("the grid will want facet counts that Home
// must never grow") and one more: the counts carry NO `?lib=` scope, so
// returning them inside a scoped page would put two differently-scoped numbers
// in one envelope.

// browseWorksResponse is the browse envelope.
//
// It is Block C's envelope key for key, deliberately and not accidentally: the
// two endpoints page identically, so a client that has learned one has learned
// the other (http-api.md §1.2's clamp rule is written once and governs both).
// It is a SEPARATE struct rather than a shared one so that the two can diverge
// later — the grid will want facet counts that Home must never grow — without
// that divergence silently reaching the Home block.
type browseWorksResponse struct {
	Items []recentWorkResponse `json:"items"`

	// Limit is the page size the SERVER applied and is AUTHORITATIVE. See
	// recentWorksResponse.Limit: a client pages against this number, never
	// against the one it sent.
	Limit int `json:"limit"`

	// MediaType, Sort and Lib are ECHOED because they are what the cursor was
	// minted under. A cursor is minted under one sort and refused under another
	// (store.ErrCursorSortMismatch), so a client that has stored a cursor has
	// to be able to recover the query it belongs to — and a UI that restores a
	// deep link needs to know which chips to light up. All three are absent
	// when the request did not name them, so absence stays distinguishable
	// from "movies", from "added_at" and from a scope of one library.
	//
	// 🚩 THE CURSOR BINDS TO `sort` ALONE, AND THESE THREE FIELDS ARE HOW A
	// CLIENT SEES THAT. Replaying a cursor under a changed `media_type` or a
	// changed `lib` does not error — store.WorksCursor carries the sort
	// discriminator and the row position and no other part of the query, so the
	// position decodes and the page is served over a DIFFERENT corpus, silently
	// skipping or repeating rows. The
	// echo does not fix that binding. It makes it observable from the side that
	// can act on it: the echo changes while the cursor the client sent did not,
	// which is exactly the mismatch, and a client can drop the cursor rather
	// than page through a corpus its position was never taken in.
	MediaType string `json:"media_type,omitempty"`
	Sort      string `json:"sort,omitempty"`

	// Lib is the `?lib=` slug list, in the order sent and trimmed to the slugs
	// that RESOLVED — which is all of them, because an unresolvable slug is a
	// 400 (resolveBrowseLibraries) rather than a silent drop.
	//
	// ⚠️ THAT REFUSAL IS WHAT MAKES AN OMITTED `lib` MEAN ONE THING. If any way
	// of naming a scope the server will not apply ever became a silent drop,
	// this key would go missing on a page served over the whole catalogue, and
	// "you asked for no scope" and "you asked for a scope and did not get it"
	// would share a spelling. The echo exists to remove that ambiguity and must
	// not become a source of it, so
	// TestBrowseEnvelopeOmitsLibOnlyWhenNoScopeWasApplied pins the property
	// rather than the field: for any request that carried `lib`, the answer is
	// a 400 or a 200 that echoes it, and there is no third outcome.
	//
	// It is a LIST rather than the comma-joined string the request used: the
	// chip is a multi-select, and joining would make the client re-parse what
	// it had just sent.
	Lib []string `json:"lib,omitempty"`

	// NextCursor is absent when this page is the last one, and its absence is
	// the "Load more" button's off switch.
	NextCursor string `json:"next_cursor,omitempty"`
}

// browseMediaTypes is §17.2's navigation enum as an ALLOWLIST for the wire.
//
// ⚠️ THE PARAMETER IS `media_type` AND NOT `kind`, AND THAT IS A CORRECTNESS
// DECISION RATHER THAN A NAMING PREFERENCE. `kind` is a real column with a
// twelve-member vocabulary and it SHIPS ON THIS WIRE under its own name, in
// every row (recentWorkResponse.Kind). A parameter called `kind` that accepted
// `movies` and rejected `movie` — or accepted `tv` and rejected `series` — would
// be two vocabularies wearing one name, on an endpoint that publishes both of
// them. The six values here are the navigation enum, they are the strings
// `internal/store` publishes as MediaTypeMovies … MediaTypeComics, and they are
// what `web/src/lib/library.ts` already holds.
//
// Two of the six are NOT a kind at all: Ebooks and Audiobooks are both
// `kind = 'book'` separated by `edition.format`, which is why the store resolves
// them server-side (§17.2: the Tier 1 client index ships no format).
var browseMediaTypes = map[string]bool{
	store.MediaTypeMovies:     true,
	store.MediaTypeTV:         true,
	store.MediaTypeMusic:      true,
	store.MediaTypeEbooks:     true,
	store.MediaTypeAudiobooks: true,
	store.MediaTypeComics:     true,
}

// browseSorts is the `?sort=` allowlist, and it is `library.default_sort`'s own
// vocabulary verbatim — migration 0005 spells it
// `CHECK (default_sort IN ('sort_title','added_at','year','popularity'))`.
//
// Using the schema's words rather than inventing wire words ("newest", "az") is
// the same rule the paragraph above applies to `media_type`: a library carries a
// default sort in the database, a client rendering that library has to ask for
// it, and a second spelling for the same four states is the drift this endpoint
// exists on the far side of.
//
// ⚠️ 'year' IS DELIBERATELY ABSENT. It is legal in the column and unservable by
// this read — `work.year` has no index, so the order would be a temp b-tree over
// the whole filtered corpus on every page. It is refused HERE, at the `?sort=`
// parse, by the browseUnservableSort arm below — a 400 that names the missing
// index rather than a 500 or a silently slow page. store.ListWorks would also
// return ErrUnservableSort for it, but that is a second line of defence for
// non-HTTP callers which this handler never reaches: `year` is not in this map,
// so it never becomes a filter.Sort. See browseUnservableSortAction, which
// depends on exactly that. See §7.2 of docs/reference/http-api.md.
// browseUnservableSort is the fourth member of `library.default_sort`'s CHECK,
// named so the refusal above can be specific. It is a legal column value and an
// unservable order, and those are different failures.
const browseUnservableSort = "year"

var browseSorts = map[string]store.WorksSort{
	string(store.WorksSortAdded):      store.WorksSortAdded,
	string(store.WorksSortTitle):      store.WorksSortTitle,
	string(store.WorksSortPopularity): store.WorksSortPopularity,
}

// browseMaxLibrarySlugs bounds `?lib=`.
//
// §17.8's reference install has SEVEN libraries and they are a set a person
// created by hand, so 32 is far above any real chip selection. The bound exists
// because the slug list becomes an `IN (…)` with one placeholder per entry: a
// caller sending ten thousand slugs would otherwise render a ten-thousand-term
// statement and a ten-thousand-row resolve on a render path. It is a REFUSAL and
// not a clamp, unlike `limit`: silently dropping slugs would WIDEN the page
// (fewer scope terms means more rows), which is the one direction a filter must
// never fail in.
const browseMaxLibrarySlugs = 32

// handleBrowseWorks serves one keyset page of the library grid.
func (s *Server) handleBrowseWorks(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}
	q := r.URL.Query()

	// The page size clamp is Block C's, shared on purpose: the two endpoints
	// page identically and http-api.md documents the rule once. See
	// recentWorksLimit for the whole table and for why it clamps rather than
	// rejects.
	effective, err := recentWorksLimit(r)
	if err != nil {
		return err
	}

	filter := store.WorksFilter{}

	// ⚠️ AN UNRECOGNISED media_type IS A 400, NEVER A SILENTLY UNFILTERED PAGE.
	// Widening on a typo turns "show me my comics" into "show me everything",
	// which looks like it worked. This follows the cursor-parse precedent below,
	// which has never silently reset either.
	if raw := strings.TrimSpace(q.Get("media_type")); raw != "" {
		if !browseMediaTypes[raw] {
			// The value is NOT echoed back: it is caller-supplied text rendered
			// into a response body. The action carries the whole vocabulary,
			// which is what the caller actually needs.
			return errStatus(http.StatusBadRequest, CodeBadRequest,
				"media_type is not one this library serves").
				withAction("use one of movies, tv, music, ebooks, audiobooks, comics — " +
					"or omit media_type for every type at once")
		}
		filter.MediaType = raw
	}

	if raw := strings.TrimSpace(q.Get("sort")); raw != "" {
		sort, ok := browseSorts[raw]
		switch {
		case ok:
			filter.Sort = sort
		case raw == browseUnservableSort:
			// ⚠️ NAMED SEPARATELY FROM AN UNKNOWN SORT, because it is not a
			// typo: `year` is a legal `library.default_sort` value, so a client
			// reading a library's own default back to this endpoint lands here
			// through no fault of its own. The message says what is actually
			// wrong — there is no index — rather than implying the word is
			// misspelt. See browseSorts.
			return errStatus(http.StatusBadRequest, CodeBadRequest,
				"this library cannot be sorted by year: work.year has no index, so that "+
					"order would sort the whole library on every page").
				withAction("sort by added_at, sort_title or popularity instead")
		default:
			return errStatus(http.StatusBadRequest, CodeBadRequest,
				"sort is not an order this library serves").
				withAction("use one of added_at, sort_title, popularity — " +
					"or omit sort for newest first")
		}
	}

	libs, err := s.resolveBrowseLibraries(r, a, &filter)
	if err != nil {
		return err
	}

	// A cursor that will not parse is a 400, never a silent reset to page one:
	// resetting turns a stale bookmark into a Load-more loop that re-serves the
	// first page for ever and looks like the list is stuck. A cursor minted
	// under a DIFFERENT sort parses and is refused by the store, which is the
	// same 400 for the same reason — see below.
	cur, err := store.DecodeWorksCursor(strings.TrimSpace(q.Get("cursor")))
	if err != nil {
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"the cursor on this request is not one this endpoint issued").
			withAction("reload the page to start from the beginning of this list").
			wrapping(err)
	}

	// SCOPE. storeScope derives it from the session and nothing else — the
	// caller cannot widen it with a query parameter, which is the whole reason
	// it is not one.
	rows, next, err := s.store.ListWorks(r.Context(), storeScope(a), filter, cur, effective)
	switch {
	case errors.Is(err, store.ErrCursorSortMismatch):
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"that cursor belongs to a different sort order").
			withAction("drop the cursor to start this order from the beginning").
			wrapping(err)
	case errors.Is(err, store.ErrUnservableSort):
		// The action is chosen from the request that caused it, never shared
		// across causes — see browseUnservableSortAction for why this is the
		// only place that can tell them apart.
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"this library cannot be sorted that way").
			withAction(browseUnservableSortAction(filter)).
			wrapping(err)
	case err != nil:
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your library could not be read").wrapping(err)
	}

	out := browseWorksResponse{
		Items:     make([]recentWorkResponse, 0, len(rows)),
		Limit:     effective,
		MediaType: filter.MediaType,
		Lib:       libs,
	}
	if filter.Sort != "" {
		out.Sort = string(filter.Sort)
	}
	for _, row := range rows {
		out.Items = append(out.Items, s.toRecentWorkResponse(row))
	}
	if next.NullTail || next.Value.Valid {
		out.NextCursor = store.EncodeWorksCursor(next)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// browseUnservableSortAction picks the action for a store.ErrUnservableSort out
// of the request that raised it, so the caller is told about the refusal they
// actually hit and not about one they did not.
//
// ⚠️ THE STORE RAISES ONE ERROR FOR TWO CAUSES, AND ONLY ONE OF THEM CAN REACH
// THIS HANDLER. internal/store/browse.go returns ErrUnservableSort both for
// `sort=year`, which has no index at all, and for `sort_title` over a media type
// that is not exactly one work.kind. `year` never gets that far from here:
// browseSorts admits only the three servable orders into filter.Sort, and the
// browseUnservableSort arm of the `?sort=` parse answers `year` above with its
// own 400 naming the missing index. So every error arriving here is the
// sort_title one, and an action mentioning `year` would be describing a refusal
// this caller cannot have hit. (The previous single string named both, plus
// `music`, whichever of the two the caller asked for.)
//
// WHY THE SPLIT IS HERE AND NOT IN THE STORE'S ERROR TAXONOMY. ErrUnservableSort
// states one property — this (sort, media type) pair has no index behind it —
// and that property is what the store knows. Which WORDS help depends on what
// the request asked for, and the request is only in scope at this boundary;
// growing a second store error per phrasing would put HTTP copy in the store and
// leave the next phrasing needing a third. One error, discriminated by its
// caller, is the same shape resolveBrowseLibraries uses for slug resolution.
//
// EVERY BRANCH NAMES SOMETHING THE CALLER CAN DO INSTEAD. That is the one thing
// the shared string got right and it is not worth losing to make a point.
func browseUnservableSortAction(f store.WorksFilter) string {
	if f.Sort != store.WorksSortTitle {
		// Defensive, and deliberately not an unreachable-panic: if a fourth
		// order ever joins browseSorts and the store refuses some pairing of
		// it, this still answers with an order that works rather than with
		// sort_title advice that does not apply.
		return "sort by added_at or popularity instead — both work for every media type"
	}
	if f.MediaType == "" {
		// The all-types grid: six kinds, and ix_work_kind_sort is
		// (kind, sort_title, id), so there is no single kind to anchor it on.
		// The caller asked for alphabetical across everything, so the first
		// thing offered is the narrowing that makes alphabetical work.
		return "add a media_type — movies, tv, ebooks, audiobooks or comics — to sort by " +
			"title, or sort by added_at or popularity, which work across every type at once"
	}
	// A media type that covers more than one work.kind, which today is `music`
	// alone: it is artists AND albums. Named by shape rather than by literal so
	// the sentence stays true if a second multi-kind type is ever added, and so
	// it never tells a caller about a media type they did not choose.
	return "this media type covers more than one kind of work, so it has no single " +
		"alphabetical order — sort by added_at or popularity instead, or pick a " +
		"media_type that is one kind (movies, tv, ebooks, audiobooks, comics) to sort by title"
}

// resolveBrowseLibraries turns `?lib=` slugs into the library ids the filter
// takes, and returns the slugs it resolved so the envelope can echo them. It
// returns a nil slice exactly when the request carried no `lib` at all — every
// other outcome is either a resolved non-empty list or an error, which is the
// property browseWorksResponse.Lib's meaning rests on.
//
// ⚠️ AN UNKNOWN SLUG IS A 400, FOR THE SAME REASON AN UNKNOWN media_type IS.
// Dropping it silently widens the page, and dropping every slug removes the
// scope entirely — so a stale bookmark to a deleted library would answer with
// the whole catalogue rather than saying the library is gone. The store's
// resolver reports HOW MANY slugs did not resolve and never WHICH — and that
// count is wrapped into the LOGGED error, never the response body, so the wire
// carries one fixed sentence whichever slug failed. A caller must not be able to
// probe another user's library names by watching the message change, and a slug
// the caller cannot see is deliberately indistinguishable from one that does not
// exist.
func (s *Server) resolveBrowseLibraries(
	r *http.Request, a authSession, filter *store.WorksFilter,
) ([]string, error) {
	// ⚠️ PRESENCE, NOT EMPTINESS, IS WHAT DECIDES "no scope was asked for".
	// Get returns "" both for a parameter that was never sent and for `?lib=`,
	// so testing the VALUE here — above the split — would let `?lib=` and
	// `?lib=%20` fall through as an absent chip and answer 200 with the whole
	// catalogue, while `?lib=,,` was correctly refused a few lines below. Three
	// spellings of "a scope was asked for and nothing was named", two answers.
	// Every empty spelling now reaches the one refusal below.
	query := r.URL.Query()
	if !query.Has("lib") {
		return nil, nil
	}

	slugs := make([]string, 0, 4)
	for _, part := range strings.Split(query.Get("lib"), ",") {
		if part = strings.TrimSpace(part); part != "" {
			slugs = append(slugs, part)
		}
	}
	if len(slugs) == 0 {
		// `?lib=`, `?lib=%20` and `?lib=,,` are not "no scope" — the caller
		// asked for a scope and named nothing. Answering with the whole
		// catalogue would be the silent widening this function exists to
		// prevent.
		return nil, errStatus(http.StatusBadRequest, CodeBadRequest,
			"lib was sent with no library in it").
			withAction("send lib as one or more library slugs separated by commas, " +
				"or omit it for every library")
	}
	if len(slugs) > browseMaxLibrarySlugs {
		return nil, errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("lib names more libraries than this endpoint accepts (%d)",
				browseMaxLibrarySlugs)).
			withAction("select fewer libraries, or omit lib for every library")
	}

	ids, err := s.store.LibraryIDsBySlug(r.Context(), storeScope(a), slugs)
	if err != nil {
		return nil, errStatus(http.StatusBadRequest, CodeBadRequest,
			"lib names a library that does not exist").
			withAction("reload the Libraries screen — a library may have been renamed or removed").
			wrapping(err)
	}
	filter.LibraryIDs = ids
	// The slugs are echoed rather than the ids: a slug is the library's public
	// URL identity (migration 0005 keeps it durable across a rename), and an id
	// is an internal row number the caller never sent and cannot use.
	return slugs, nil
}
