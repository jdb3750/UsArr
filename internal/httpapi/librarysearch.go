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

// GET /api/v1/search is search over YOUR OWN LIBRARY: the local corpus, ranked,
// as one flat list (ARCHITECTURE.md §8.2, §17.4, docs/reference/search.md §3-§4).
//
// IT IS THE ENDPOINT THE PERFORMANCE TABLE WAS WRITTEN FOR. ARCHITECTURE.md
// §13 budgets `GET /api/v1/search?q=…` (FTS hybrid + rerank) at p50 < 15 ms
// and p99 < 50 ms. It can hold that because it is a local read and nothing else
// (principle 1): two SQLite statements, no upstream call, no metadata provider,
// no image fetch. The Prowlarr indexer fan-out used to live on this path and
// cannot meet that budget by construction — §8.4 says so — which is why it now
// answers on /api/v1/releases/search. The two are different questions over
// different corpora and they no longer share a name.
//
// THE WIRE CONTRACT IS WRITTEN DOWN WHERE A CONSUMER WILL FIND IT:
// docs/reference/http-api.md §6. That document, not this comment, is what the
// Search screen is built against — a doc comment is not reachable from a
// browser tab, which is the lesson §1's `limit` field cost.
//
// WHAT IT DOES NOT DO YET, stated so a caller does not assume it:
//
//   - No grouping. §4's grouped card — one row per linked set, with per-medium
//     availability — is derived from work_relation edges at query time, and
//     nothing reads those edges yet. Linked works come back as separate rows.
//   - No "not in your library" section. That is the release search, on its own
//     endpoint, over a different corpus, and it streams over SSE rather than
//     answering in a body.
//   - No `?lib=` scope chip. The scope this applies is the caller's ACCESS
//     scope, which is derived from the session and cannot be widened by a query
//     parameter. A user-chosen narrowing is a different parameter and a
//     different commit; the seam is that the scope is an argument to the store
//     call, not part of the SQL string.
//   - No paging. See searchLimit.
//
// THE SCORE IS ON THE WIRE AS OF THIS COMMIT, AND THE ORDER IS STILL THE
// CONTRACT. This comment used to read "No score on the wire … a published score
// is a number a client would start comparing across queries, and it is
// normalised per query so that comparison would be meaningless" — the second
// half of which is TRUE and survives, as the reason §6.2.1 forbids exactly that
// comparison. What forced the reversal is that ARCHITECTURE §17.4 rule 2 orders
// grouped results "by the group's best-scoring hit", which no client can compute
// from a bare ordering: a row's ordinal position says nothing about how far the
// best row of one group sits from the best row of another. Withholding the
// number did not stop a client comparing scores; it stopped a client building
// the screen. ADR-0054 records the decision and the alternatives it closes.

// searchHitResponse is one search result as it crosses to a browser.
//
// A FIELD-BY-FIELD ALLOWLIST, never a denylist, exactly as recentWorkResponse
// is: the store's SearchHit is copied into this by hand, so a column added to
// the read cannot reach the wire by default. Nothing service-side is here —
// no instance is named, no remote_path is read — for the reasons
// recentWorkResponse states at length.
//
// THE RENDERING KEYS ARE recentWorkResponse's KEYS, PLUS `score`. A search
// result row and a recently-added row render identically (§17.2's Type / title /
// year / Have grammar), so a client reuses one row component across Home and
// Search. The two structs are separate because the two reads are separate; the
// rendering key set is the same on purpose, and it is
// TestSearchItemKeysAreRecentWorkKeysPlusScore that pins THAT — the two
// allowlist tests each pin one struct against a hand-copied list of its own and
// neither can see the pair drift apart.
//
// `score` is the one key Home has no analogue for and cannot have one for: Home
// orders by date, so there is no relevance for it to carry. A client reusing the
// row component ignores it, which costs nothing — it is read by the grouping
// layer above the row, not by the row.
type searchHitResponse struct {
	ID int64 `json:"id"`

	// MediaType is §17.2's six-value navigation enum, resolved server-side
	// because §17.2 says the Tier 1 client index carries no format and a client
	// therefore cannot separate ebooks from audiobooks.
	//
	// IT IS OMITTED, NOT EMPTY, for a work whose kind the enum has no row for.
	// Today that is 'game' alone and nothing writes a game work. The store
	// deliberately does not filter such a row out — search.md §2 puts the
	// corpus refusal at the WRITER, and a second allowlist in the read would be
	// a second vocabulary free to drift from it — so the honest wire shape is
	// an absent key rather than an invented type.
	MediaType string `json:"media_type,omitempty"`

	// Kind is work.kind verbatim — the schema's word, which is what an item
	// route needs. MediaType is the user's word. Both travel because they
	// answer different questions.
	Kind string `json:"kind"`

	Title string `json:"title"`

	// Year is absent rather than 0 when the column is NULL: a rendered "0" is a
	// claim about a release date, an absent key is the truth.
	Year *int64 `json:"year,omitempty"`

	// AddedAt is a ranking input (recency) and a rendering field, and it is
	// absent when the upstream reported no creation date — a state Kavita
	// reaches. It is NOT the sort key here: this list is ordered by relevance.
	AddedAt *time.Time `json:"added_at,omitempty"`

	HaveCount int64 `json:"have_count"`
	WantCount int64 `json:"want_count"`

	// Availability is schema.md §1's polymorphic blob, forwarded verbatim, with
	// the same four-case handling recentWorkResponse documents: absent when the
	// column is NULL (silently, it is a legitimate state) and absent when the
	// stored text is unusable (logged, because that is a writer bug).
	Availability json.RawMessage `json:"availability,omitempty"`

	// Score is store.SearchHit.Score, forwarded unrounded, and §6.2.1 is its
	// whole contract. It is UNCONDITIONAL — never omitempty — because every hit
	// has one: rerank writes it for every candidate and the rrf component is
	// strictly positive for any row a leg retrieved, so a `0` here would be a
	// server bug rather than a legitimately absent value, and a key that
	// disappeared on a bug is a key a client learns to treat as optional.
	//
	// IT IS NOT ROUNDED. Rounding would manufacture ties the server did not
	// have, and the one comparison §6.2.1 permits — this hit against that hit,
	// in this response — is the comparison ties break. The digits past the
	// fourth carry no information a client should read anything into; they are
	// not precision worth trusting, and they are not worth destroying either.
	Score float64 `json:"score"`
}

type searchResponse struct {
	// Query is the text the server actually searched, echoed so a client can
	// tell a stale response from a current one when the user is typing fast.
	Query string `json:"query"`

	Items []searchHitResponse `json:"items"`

	// Limit is the row cap the SERVER applied and it is AUTHORITATIVE, on the
	// same reasoning §1.2 gives: the server clamps rather than rejects, so a
	// client that asked for 10000 and trusted its own number would misread the
	// short answer.
	Limit int `json:"limit"`

	// Truncated says search.md §3 step 1's twelve-token cap fired, so the
	// results answer a PREFIX of what was typed. §3 requires a UI note; a note
	// the server never sends cannot be rendered, which is why this is on the
	// wire rather than only in the log.
	Truncated bool `json:"truncated"`
}

// handleLibrarySearch serves one library search.
func (s *Server) handleLibrarySearch(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	// ONE PARAMETER NAME, `q`, and it is the one ARCHITECTURE.md §13's budget
	// row names. The release endpoint accepts `query` as well as `q` for
	// historical reasons; this one does not, because two spellings of the same
	// parameter is a contract a client has to guess at.
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"this search has no query text").
			withAction("send ?q= with something to search for")
	}

	limit, err := searchLimit(r)
	if err != nil {
		return err
	}

	// SCOPE. storeScope derives it from the session and nothing else — the
	// caller cannot widen it with a query parameter, which is the whole reason
	// it is not one. v0.1's owner gets the full scope; a non-owner gets an empty
	// visible set, which the store renders as `1=0` and which therefore returns
	// no rows rather than every row.
	res, err := s.store.SearchLibrary(r.Context(), storeScope(a), query, limit)
	if err != nil {
		// Every token in the FTS argument is quoted by store.ftsQuote, so an
		// fts5 syntax error is a bug in the query builder rather than in the
		// user's typing. There is no input a caller can send that this endpoint
		// should answer 400 to except an absent one, which is handled above.
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your library could not be searched").wrapping(err)
	}

	out := searchResponse{
		Query:     query,
		Items:     make([]searchHitResponse, 0, len(res.Hits)),
		Limit:     limit,
		Truncated: res.Truncated,
	}
	// STORE ORDER, PRESERVED EXACTLY. The store sorts by score and then
	// diversify promotes rows past better-scoring ones, so this loop must not
	// re-sort — least of all by the `score` it is publishing, which would undo
	// the media-type diversity guarantee row by row. §6.2.1 tells a client the
	// same thing; TestSearchOrderIsTheServersAndIsNotScoreOrder holds it here.
	for _, hit := range res.Hits {
		out.Items = append(out.Items, s.toSearchHitResponse(hit))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// searchLimit reads `?limit=` and returns the row cap the server will apply.
//
// IT IS A CLAMP, NOT A VALIDATED RANGE, and the table is §1.2's table with this
// endpoint's bounds — absent/empty/0 gives the default, 1…SearchMaxLimit is
// served as asked, more is silently clamped and echoed back, and a negative or
// non-integer value is a 400 because those are not page sizes at all. The rule
// is copied rather than shared because the two endpoints' constants differ and
// a shared helper parameterised on two constants is harder to read than the
// twenty lines it saves.
//
// ⚠️ THERE IS NO SECOND PAGE, AND THAT IS STRUCTURAL RATHER THAN UNFINISHED.
// The store fuses at most store-side retrievalLimit candidates and re-ranks the
// whole of that set in Go, so the ordering is a property of the SET and not of
// any column — there is no keyset position a cursor could name, and an offset
// would re-run the whole retrieval to throw the head away. A caller asks for up
// to SearchMaxLimit rows and that is every row this endpoint has. If deeper
// results are ever wanted the answer is a narrower query, or the external search
// engine docs/FUTURE.md defers.
func searchLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return store.SearchDefaultLimit, nil
	}
	// On ErrRange, ParseInt returns the saturated bound, which is exactly the
	// input the clamp below wants; any other error means the text was never an
	// integer. Parsing at 64 bits rather than through queryInt32 is what keeps
	// the clamp total over the positive integers — see recentWorksLimit for the
	// cliff at 2^31 that taught it.
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("limit=%q is not a whole number of results", raw)).
			withAction("omit limit for the default, or send a whole number from 1 to " +
				strconv.Itoa(store.SearchMaxLimit))
	}
	if n < 0 {
		return 0, errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("limit=%q is negative, which is not a number of results", raw)).
			withAction("omit limit for the default, or send a whole number from 1 to " +
				strconv.Itoa(store.SearchMaxLimit))
	}
	switch {
	case n == 0:
		return store.SearchDefaultLimit, nil
	case n > store.SearchMaxLimit:
		return store.SearchMaxLimit, nil
	}
	return int(n), nil
}

// toSearchHitResponse is the allowlist applied. Every field is named; nothing is
// copied by reflection or embedded.
func (s *Server) toSearchHitResponse(h store.SearchHit) searchHitResponse {
	out := searchHitResponse{
		ID:        h.ID,
		MediaType: h.MediaType,
		Kind:      h.Kind,
		Title:     h.Title,
		HaveCount: h.HaveCount,
		WantCount: h.WantCount,
		Score:     h.Score,
	}
	if h.Year.Valid {
		year := h.Year.Int64
		out.Year = &year
	}
	// A stored timestamp that will not parse is dropped rather than guessed at,
	// exactly as toRecentWorkResponse drops one.
	if h.AddedAt.Valid {
		if at, err := store.ParseTime(h.AddedAt.String); err == nil {
			out.AddedAt = &at
		}
	}
	out.Availability = s.availabilityFor(h.ID, h.Availability)
	return out
}
