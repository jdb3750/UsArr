package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

// fakeKavita is a Kavita-shaped test double.
//
// It exists for the same reason fakeProwlarr does — no public demo instance, no
// Docker in CI — and it models the three behaviours that decide whether the
// connection test tells a user the truth, none of which a cassette can exercise
// end to end:
//
//  1. GET /api/Health is [AllowAnonymous] and answers the BARE STRING `Ok`, with
//     no key and no JSON. A client that treats a 200 here as proof of anything
//     is wrong, and this double will happily answer it for a bad key.
//  2. Every other route requires the `x-api-key` HEADER, and a bad one is a 401
//     with an EMPTY BODY.
//  3. GET /api/Server/server-info-slim is ADMIN-ONLY. With admin=false it
//     answers 403 while /api/Library/libraries still answers 200 — the case
//     where a perfectly valid key must NOT be reported as a bad credential.
//
// Payload shapes are taken from api/specs/kavita-develop.json.
type fakeKavita struct {
	t       *testing.T
	authKey string

	// admin decides whether server-info-slim answers or 403s.
	admin bool
	// libraries is how many libraries this account can see. Library i+1 gets
	// LibraryType i, so `libraries = 3` yields one Manga, one Comic and one Book
	// — the shape the catalogue import's kind mapping is exercised on.
	libraries int
	// series is what POST /api/Series/all-v2 answers. It is nil by default,
	// which serves the EMPTY ARRAY the connection-test tests expect; a test that
	// wants a catalogue sets it before the instance is added. It is written only
	// during setup, before the server can be reached.
	series []map[string]any
	// metadata is what GET /api/Series/metadata?seriesId=N answers, keyed by
	// series id. A series with no entry gets an EMPTY SeriesMetadataDto rather
	// than a 404, which is what a real Kavita returns for a series nobody has
	// filled in — the ordinary case on a fresh instance. Written during setup,
	// or later through setMetadata — which takes mu, as the handler's read does
	// — so a re-import test can change what the upstream says between two runs.
	metadata map[int]map[string]any

	mu sync.Mutex
	// keySeen records every x-api-key header value received, so a test can
	// prove the credential went in the header and not the query string.
	keySeen []string
	// queriesSeen records every raw query string received, for the same reason
	// from the other side.
	queriesSeen []string
	// paths records every path served, so a test can prove which endpoints the
	// connection test actually called.
	paths []string

	// hold, when non-nil, parks every POST /api/Series/all-v2 until it is
	// closed. It is what makes an import OBSERVABLY IN FLIGHT: without it a
	// fixture import finishes in microseconds and "a second import while the
	// first is running" is a state no test can be standing in.
	hold chan struct{}

	srv *httptest.Server
}

func newFakeKavita(t *testing.T, authKey string) *fakeKavita {
	t.Helper()
	k := &fakeKavita{t: t, authKey: authKey, admin: true, libraries: 2}

	mux := http.NewServeMux()

	// [AllowAnonymous], bare string, no key required. Deliberately registered
	// WITHOUT the auth wrapper.
	mux.HandleFunc("GET /api/Health", func(w http.ResponseWriter, r *http.Request) {
		k.record(r)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Ok"))
	})

	mux.HandleFunc("GET /api/Library/libraries", k.authed(func(w http.ResponseWriter, r *http.Request) {
		out := make([]map[string]any, 0, k.libraries)
		for i := range k.libraries {
			out = append(out, map[string]any{
				"id": i + 1, "name": fmt.Sprintf("Library %d", i+1), "type": i,
				"lastScanned": "2026-08-17T07:00:30.118Z",
				// A HOST FILESYSTEM PATH. It must never reach the browser.
				"folders": []string{fmt.Sprintf("/mnt/user/media/lib%d", i+1)},
			})
		}
		writeJSONBody(w, out)
	}))

	mux.HandleFunc("GET /api/Server/server-info-slim", k.authed(func(w http.ResponseWriter, r *http.Request) {
		if !k.admin {
			// ServerController is [Authorize(PolicyGroups.AdminPolicy)] at class
			// level: a VALID non-admin key gets 403, not 401.
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeJSONBody(w, map[string]any{
			"installId": "fixture", "isDocker": true, "kavitaVersion": "0.9.0.20",
			"firstInstallDate": "2025-11-02T19:41:07.512Z", "firstInstallVersion": "0.8.6.2",
		})
	}))

	mux.HandleFunc("POST /api/Series/all-v2", k.authed(func(w http.ResponseWriter, r *http.Request) {
		// Parked before a byte is written, and AFTER k.record has run, so a
		// test can see that the request arrived while it is still stuck here.
		if h := k.currentHold(); h != nil {
			<-h
		}
		// A JSON ARRAY, never `null`. An empty `series` slice would encode as
		// `null` and the streaming decoder would reject the body as "expected a
		// JSON array" — a fixture failure that reads exactly like a client bug.
		out := k.series
		if out == nil {
			out = []map[string]any{}
		}
		w.Header().Set("Pagination", fmt.Sprintf(
			`{"currentPage":1,"itemsPerPage":2147483647,"totalItems":%d,"totalPages":1}`, len(out)))
		writeJSONBody(w, out)
	}))

	// The credit path (ADR-0044). One GET per series, which is the only shape
	// Kavita offers — SeriesDto carries no creator field at all.
	mux.HandleFunc("GET /api/Series/metadata", k.authed(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.URL.Query().Get("seriesId"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		md := k.seriesMetadata(id)
		if md == nil {
			// An empty object, NOT a 404 and NOT `null`: a real Kavita returns a
			// SeriesMetadataDto with every array empty for a series nobody has
			// tagged, and the adapter must read that as "no credits" rather than
			// as an error.
			md = map[string]any{"id": 0, "seriesId": id}
		}
		writeJSONBody(w, md)
	}))

	k.srv = httptest.NewServer(mux)
	t.Cleanup(k.srv.Close)
	return k
}

func (k *fakeKavita) URL() string { return k.srv.URL }

// holdSeriesList parks the next and every subsequent series-list read.
//
// The release is registered as cleanup as well, and that is not belt-and-braces:
// httptest.Server.Close blocks on outstanding handlers, so a test that fails
// while a reader is parked would hang at teardown instead of reporting its
// failure. A firing test that cannot report is not a test.
func (k *fakeKavita) holdSeriesList() {
	k.t.Cleanup(k.releaseSeriesList)
	k.mu.Lock()
	defer k.mu.Unlock()
	k.hold = make(chan struct{})
}

// releaseSeriesList lets every parked reader through and stops parking new ones.
func (k *fakeKavita) releaseSeriesList() {
	k.mu.Lock()
	h := k.hold
	k.hold = nil
	k.mu.Unlock()
	if h != nil {
		close(h)
	}
}

func (k *fakeKavita) currentHold() chan struct{} {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.hold
}

// setMetadata replaces the metadata table WHILE THE SERVER IS RUNNING, under
// the same mutex the handler reads it through. The field's own comment says it
// is written only during setup; a test that re-imports has to change what the
// upstream says between two imports, which is the whole point of re-importing.
func (k *fakeKavita) setMetadata(md map[int]map[string]any) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.metadata = md
}

func (k *fakeKavita) seriesMetadata(id int) map[string]any {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.metadata[id]
}

// countPath is how many times one upstream path has been served.
func (k *fakeKavita) countPath(path string) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	n := 0
	for _, p := range k.paths {
		if p == path {
			n++
		}
	}
	return n
}

func (k *fakeKavita) record(r *http.Request) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keySeen = append(k.keySeen, r.Header.Get("x-api-key"))
	k.queriesSeen = append(k.queriesSeen, r.URL.RawQuery)
	k.paths = append(k.paths, r.URL.Path)
}

func (k *fakeKavita) seen() (keys, queries, paths []string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.keySeen...),
		append([]string(nil), k.queriesSeen...),
		append([]string(nil), k.paths...)
}

// authed wraps a handler with Kavita's global AuthKey requirement: the header,
// and a 401 with an EMPTY body when it is wrong.
func (k *fakeKavita) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k.record(r)
		if r.Header.Get("x-api-key") != k.authKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func writeJSONBody(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
