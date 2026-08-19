package httpapi

import (
	"net/http"

	"github.com/jdb3750/UsArr/internal/store"
)

// GET /api/v1/library/facets is the per-media-type facet count: how many works
// of each of ARCHITECTURE §17.2's six navigation types the caller can see.
//
// It is what §17.2's Block A (the media-type summary) has been blocked on.
//
// ⚠️ IT IS NOT THE READ ADR-0053 REOPENS ON, AND IT LOOKS LIKE IT. That ADR wants
// a predicate answering WHETHER each type has content; this answers HOW MANY
// works are BUCKETED to each type, and the two differ on exactly one case — see
// internal/store/facets.go's audiobookCountSQL. ADR-0053's condition was REFINED
// rather than discharged on 2026-08-19 for that reason (ADR-0059); it now names
// §17.2 rows 4-5's independent EXISTS over edition.format. `TYPE_NAV` in
// `web/src/routes/+layout.svelte` still maps all six with no predicate,
// deliberately, and nothing here changes the sidebar.
//
// IT IS A LOCAL READ (principle 1). Two SQLite statements, no \*Arr call, no
// metadata provider, no image fetch. It is on the render path of the first
// screen the browser asks for, so both statements have a pinned plan in
// internal/store/facets_test.go: six equality seeks on ix_work_kind_sort, no
// sort, and a covering probe on ix_edition_format for the book split.
//
// THE WIRE CONTRACT IS IN docs/reference/http-api.md §8, where a consumer can
// find it — including, in full, what the counts count and what they do not. This
// comment is not reachable from a browser tab. The two say the same things in
// the same order; internal/store/facets.go owns the reasoning behind each.
//
// THIS HANDLER READS NO QUERY PARAMETER AT ALL, and that is a decision rather
// than a gap:
//
//   - NO `?lib=`. §17.2 settles it — *"Ownership decides shape; scope decides
//     numbers"* — and ADR-0053 then removed the numbers from the sidebar, which
//     leaves ownership as the only axis this read has to serve. Counts under a
//     chip selection are a different read; a caller must not read these as
//     though they were it.
//   - NO `?media_type=`. Six numbers is the whole response and it is bounded at
//     six by construction; a filter would let a client ask six times for what
//     one call answers.
//   - NO `limit` and NO `cursor`. There is nothing to page.
//
// An unrecognised parameter is therefore IGNORED here as everywhere, per
// http-api.md's preamble — this endpoint inherits that rule and, like every
// endpoint but the library grid, does not itself pin it.

// mediaTypeCountsResponse is the six counts as they cross to a browser.
//
// A FIELD-BY-FIELD ALLOWLIST, never a denylist, and the struct IS the allowlist:
// store.MediaTypeCounts is copied into this by hand, so a seventh field added to
// the read cannot reach the wire by default.
//
// ⚠️ ALL SIX KEYS ARE ALWAYS PRESENT AND NONE CARRIES `omitempty`. That is the
// contract, not a formatting accident: an absent key and a zero are different
// values a consumer would have to tell apart, and the whole point of §17.2's
// closed enum is that there is nothing to tell apart. A type with no rows is
// `0`; a type the caller cannot see is `0`; there is no third spelling.
// TestLibraryFacetsResponseKeysAreTheAllowlist pins the key set, and
// TestLibraryFacetsZeroAndRestrictedAreByteIdentical pins the collapse.
//
// Nothing else is here and nothing can be. No service instance is named — which
// Kavita a count came from is not something Home renders, and naming it would
// publish the topology of the install to every future non-owner user. No library
// is named either: this read never touches `library_member` (see
// internal/store/facets.go's Unfiled block for why).
type mediaTypeCountsResponse struct {
	Movies     int64 `json:"movies"`
	TV         int64 `json:"tv"`
	Music      int64 `json:"music"`
	Ebooks     int64 `json:"ebooks"`
	Audiobooks int64 `json:"audiobooks"`
	Comics     int64 `json:"comics"`
}

// libraryFacetsResponse is the envelope.
//
// The counts are NESTED under one key rather than being the whole body, because
// §17.2's Block A row is `name · count · availability rollup · last import ·
// see all` and only the first two of those are this read. The rollup and the
// import time are their own aggregates and their own commit; nesting leaves them
// somewhere to land that does not collide with a media type's name.
type libraryFacetsResponse struct {
	Counts mediaTypeCountsResponse `json:"counts"`
}

// handleLibraryFacets serves the six counts.
func (s *Server) handleLibraryFacets(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	// SCOPE. storeScope derives it from the session and nothing else — the
	// caller cannot widen it with a query parameter, which is the whole reason
	// it is not one. v0.1's owner gets the full scope; a non-owner gets an empty
	// visible set, which the store renders as `1=0` and which therefore counts
	// nothing rather than everything. A rollup across instances a caller cannot
	// see is the existence oracle store.go rule 2 names, and six numbers whose
	// entire content is how much exists is that rollup in its most literal form.
	counts, err := s.store.CountWorksByMediaType(r.Context(), storeScope(a))
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your library could not be read").wrapping(err)
	}

	writeJSON(w, http.StatusOK, libraryFacetsResponse{Counts: toMediaTypeCounts(counts)})
	return nil
}

// toMediaTypeCounts is the allowlist applied. Every field is named; nothing is
// copied by reflection or embedded.
func toMediaTypeCounts(c store.MediaTypeCounts) mediaTypeCountsResponse {
	return mediaTypeCountsResponse{
		Movies:     c.Movies,
		TV:         c.TV,
		Music:      c.Music,
		Ebooks:     c.Ebooks,
		Audiobooks: c.Audiobooks,
		Comics:     c.Comics,
	}
}
