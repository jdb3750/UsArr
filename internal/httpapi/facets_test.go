package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// callLibraryFacets runs the handler with a session already resolved, which is
// what the authenticated middleware does before it. `owner` is the half that
// decides the access scope: storeScope gives an owner everything and a
// non-owner an empty visible set.
func callLibraryFacets(t *testing.T, s *Server, owner bool) (int, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/library/facets", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: owner},
	}))
	w := httptest.NewRecorder()
	if err := s.handleLibraryFacets(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

// The counts over seedLibraryCorpus, which is the corpus every other endpoint
// test in this package runs against. Named row by row so a fixture change shows
// up as a named difference rather than as an integer that moved:
//
//	movies      1  — Train Dreams
//	tv          0  — the corpus has no series
//	music       0  — and no artist or album
//	ebooks      1  — Piranesi
//	audiobooks  1  — Project Hail Mary
//	comics      2  — Berserk, Vinland Saga (An Undated Manga is the third)
//
// The exclusions the corpus carries: a soft-deleted comic, a `person`, and a
// `comic_issue`. All three would land in a bucket if they were counted, so
// `comics: 3` rather than 5 is an assertion about two of them.
const wantSeededFacets = `{"counts":{"movies":1,"tv":0,"music":0,"ebooks":1,"audiobooks":1,"comics":3}}`

func TestLibraryFacetsCountsTheSeededCorpus(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	code, body := callLibraryFacets(t, s, true)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if strings.TrimSpace(body) != wantSeededFacets {
		t.Errorf("body:\n got:  %s\n want: %s", strings.TrimSpace(body), wantSeededFacets)
	}
}

// ⚠️ THE FACET NUMBER AND THE GRID PAGE MUST BE THE SAME NUMBER, checked ACROSS
// THE WIRE and from the consumer's side: both endpoints are called, and the rows
// one returns are counted rather than re-derived from the query the other ran.
//
// This is the assertion that makes the count honest. A facet count that silently
// excluded something would agree with every test its own author wrote and
// disagree with the only screen a user reaches by clicking it.
func TestLibraryFacetsAgreeWithTheGridEndpoint(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	_, body := callLibraryFacets(t, s, true)
	var got struct {
		Counts map[string]int64 `json:"counts"`
	}
	mustJSON(t, body, &got)
	if len(got.Counts) != 6 {
		t.Fatalf("expected six counts, got %d: %s", len(got.Counts), body)
	}

	for mediaType, facet := range got.Counts {
		t.Run(mediaType, func(t *testing.T) {
			// limit=2 so the grid really pages: a facet that matched only the
			// first page would be a count of the page size.
			var titles []string
			query := "?media_type=" + mediaType + "&limit=2"
			for {
				code, page := callBrowseWorks(t, s, query)
				if code != http.StatusOK {
					t.Fatalf("grid status %d: %s", code, page)
				}
				titles = append(titles, browseTitles(t, page)...)
				var envelope struct {
					NextCursor string `json:"next_cursor"`
				}
				mustJSON(t, page, &envelope)
				if envelope.NextCursor == "" {
					break
				}
				query = "?media_type=" + mediaType + "&limit=2&cursor=" + envelope.NextCursor
			}
			if int64(len(titles)) != facet {
				t.Errorf("the facet says %d and the grid returns %d: %v", facet, len(titles), titles)
			}
		})
	}
}

// ⚠️ ALL SIX KEYS, ALWAYS, AND NOT ONE MORE. The struct IS the allowlist and
// nothing carries `omitempty`, so a type with no rows ships `0` rather than
// going missing — which is what makes an absent key impossible and therefore
// meaningless. Pinned against the RESPONSE BYTES so a field added by any route,
// including one added by embedding, is caught.
func TestLibraryFacetsResponseKeysAreTheAllowlist(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)

	_, body := callLibraryFacets(t, s, true)

	var envelope map[string]json.RawMessage
	mustJSON(t, body, &envelope)
	if got := keysOf(envelope); strings.Join(got, ",") != "counts" {
		t.Errorf("envelope keys = %v, want [counts]", got)
	}

	var counts map[string]json.RawMessage
	mustJSON(t, string(envelope["counts"]), &counts)
	want := []string{"audiobooks", "comics", "ebooks", "movies", "music", "tv"}
	if got := keysOf(counts); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("count keys = %v\n          want %v", got, want)
	}

	// The vocabulary is §17.2's navigation enum, published by internal/store, and
	// NOT `work.kind`. A key spelled `movie` or `series` here would be the
	// schema's word on a wire that carries the user's.
	for _, mediaType := range []string{
		store.MediaTypeMovies, store.MediaTypeTV, store.MediaTypeMusic,
		store.MediaTypeEbooks, store.MediaTypeAudiobooks, store.MediaTypeComics,
	} {
		if _, ok := counts[mediaType]; !ok {
			t.Errorf("media type %q has no key in the response: %s", mediaType, body)
		}
	}
}

// ⚠️ ZERO AND INVISIBLE ARE THE SAME ANSWER, AND THE BYTES SAY SO. A caller who
// can see nothing gets a response BYTE-IDENTICAL to an owner's over an empty
// database — not merely equivalent, identical — so nothing on this endpoint can
// be used to learn that content exists behind a scope.
//
// The direction is the point and only one of the two would be a leak. A caller
// who can see everything learning that a type is empty is the answer, not a
// disclosure; a caller who cannot see a type learning that it is non-empty is
// the existence oracle principle 4 exists to prevent. So the collapse runs
// restriction → zero, and zero never widens into "hidden" or "unknown". There is
// no third state on this wire, deliberately, and the cost — this endpoint cannot
// tell an empty screen WHY it is empty — is paid by `browseEmptyState`
// (web/src/lib/librarygrid.ts), which words it from the services read.
func TestLibraryFacetsZeroAndRestrictedAreByteIdentical(t *testing.T) {
	seeded := newTestServer(t, nil)
	seedLibraryCorpus(t, seeded)
	code, restricted := callLibraryFacets(t, seeded, false)
	if code != http.StatusOK {
		t.Fatalf("restricted status %d: %s", code, restricted)
	}

	// The premise: the owner over the SAME database sees plenty. Without this
	// the test would pass just as well against a handler that returned zeros to
	// everybody.
	if _, owner := callLibraryFacets(t, seeded, true); strings.TrimSpace(owner) == strings.TrimSpace(restricted) {
		t.Fatalf("the owner and a restricted caller got the same answer over a seeded "+
			"corpus, so the scope changed nothing: %s", owner)
	}

	empty := newTestServer(t, nil)
	_, genuine := callLibraryFacets(t, empty, true)

	if strings.TrimSpace(restricted) != strings.TrimSpace(genuine) {
		t.Errorf("a restricted caller can tell itself apart from an empty library:"+
			"\n restricted: %s\n empty:      %s",
			strings.TrimSpace(restricted), strings.TrimSpace(genuine))
	}
	const wantZeroes = `{"counts":{"movies":0,"tv":0,"music":0,"ebooks":0,"audiobooks":0,"comics":0}}`
	if strings.TrimSpace(genuine) != wantZeroes {
		t.Errorf("an empty library is not six explicit zeroes:\n got:  %s\n want: %s",
			strings.TrimSpace(genuine), wantZeroes)
	}
}

// ⚠️ THE COUNTS ARE OF WHAT YOU OWN AND CARRY NO `?lib=` SCOPE. §17.2:
// "Ownership decides shape; scope decides numbers" — and ADR-0053 removed the
// numbers from the sidebar, leaving ownership as the only axis this read serves.
//
// The boundary is asserted from the CONSUMER's side: a work in no library at all
// is in the facet count and is not on a library-scoped grid page. A client that
// paints a scoped grid beside an unscoped facet is showing two true numbers that
// disagree, and http-api.md §8 has to be the place it learns that before it
// ships.
func TestLibraryFacetsAreNotLibraryScoped(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibraryCorpus(t, s)
	seedUnfiledFilm(t, s)

	_, body := callLibraryFacets(t, s, true)
	var got struct {
		Counts struct {
			Movies int64 `json:"movies"`
		} `json:"counts"`
	}
	mustJSON(t, body, &got)
	if got.Counts.Movies != 2 {
		t.Fatalf("movies = %d, want 2 — the unfiled film must be counted "+
			"(internal/store/facets.go, the Unfiled block): %s", got.Counts.Movies, body)
	}

	// Every library there is. Their union still does not contain the unfiled
	// film, which is what makes the facet number the larger of the two.
	code, page := callBrowseWorks(t, s, "?media_type=movies&lib=manga,books")
	if code != http.StatusOK {
		t.Fatalf("grid status %d: %s", code, page)
	}
	titles := browseTitles(t, page)
	if len(titles) != 1 || titles[0] != "Train Dreams" {
		t.Errorf("a grid page scoped to every library returned %v, want [Train Dreams] — "+
			"either the fixture lost its unfiled work, or `?lib=` stopped scoping", titles)
	}
}

// seedUnfiledFilm adds one movie with NO `library_member` row. That is what
// "unfiled" means in this tree: nothing writes a member row into the reserved
// `Unfiled` library 0, so an unfiled work is one with no membership at all.
// internal/store/facets.go's Unfiled block records the whole verification.
func seedUnfiledFilm(t *testing.T, s *Server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO work (id, kind, title, sort_title, normalized_title, year, added_at,
		                   have_count, want_count)
		   VALUES (30, 'movie', 'An Unfiled Film', 'an unfiled film', 'an unfiled film',
		           2021, '2026-07-01 10:00:00', 1, 0)`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 30, 'r30', 'movie', '2026-08-16 12:00:00')`,
	}
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}
