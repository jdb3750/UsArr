package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeProwlarr is a Prowlarr-shaped test double.
//
// It exists because there is no public *Arr demo instance and CI has no Docker
// daemon (docs/DEVELOPMENT.md §8), and because the behaviours that matter here
// are the awkward ones a recorded cassette makes hard to exercise: the
// api-key-in-downloadUrl rewrite, the 200-with-[] ambiguity, blocked indexers,
// and the grab cache.
//
// Payload shapes are taken from the shipped OpenAPI spec in api/specs.
type fakeProwlarr struct {
	t      *testing.T
	apiKey string

	// urlBase is the reverse-proxy sub-path this instance is served under, or ""
	// for the root. Every route is registered under it and NOTHING is registered
	// at the root, so a client that ignores url_base gets a 404 from the mux
	// rather than quietly passing against a fixture that answers both ways.
	urlBase string

	mu sync.Mutex
	// grabbed records every POST /api/v1/search body, so a test can assert
	// what UsArr actually sent upstream.
	grabbed []map[string]any
	// searched counts GET /api/v1/search by indexer id, proving the fan-out is
	// one request per indexer rather than one aggregate call.
	searched map[string]int
	// keySeen records every X-Api-Key value received.
	keySeen []string

	// blockedIndexerID, when non-zero, is reported by /api/v1/indexerstatus.
	blockedIndexerID int32

	srv *httptest.Server
}

func newFakeProwlarr(t *testing.T, apiKey string) *fakeProwlarr {
	t.Helper()
	return newFakeProwlarrAt(t, apiKey, "")
}

// newFakeProwlarrAt serves the same fixture behind a reverse-proxy sub-path,
// which is what a `https://host/prowlarr` deployment looks like from UsArr's
// side: the base URL is the origin, and Prowlarr's own `URL Base` is the prefix
// every route hangs off. urlBase must be "" or a leading-slash path with no
// trailing slash — the shape UsArr concatenates onto base_url.
func newFakeProwlarrAt(t *testing.T, apiKey, urlBase string) *fakeProwlarr {
	t.Helper()
	f := &fakeProwlarr{t: t, apiKey: apiKey, urlBase: urlBase, searched: map[string]int{}}

	mux := http.NewServeMux()
	// handle registers "METHOD /path" under the sub-path, so the routing table
	// below reads as Prowlarr's own and the prefix is applied in exactly one
	// place.
	handle := func(pattern string, h http.HandlerFunc) {
		method, path, ok := strings.Cut(pattern, " ")
		if !ok {
			t.Fatalf("fake prowlarr: %q is not a %q pattern", pattern, "METHOD /path")
		}
		mux.HandleFunc(method+" "+urlBase+path, h)
	}

	handle("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		// [AllowAnonymous] upstream: no key required, on purpose.
		writeJSONTest(w, map[string]string{"status": "OK"})
	})
	handle("GET /api", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONTest(w, map[string]any{"current": "v1", "deprecated": []string{}})
	})
	handle("GET /api/v1/system/status", f.authed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONTest(w, map[string]any{
			"appName": "Prowlarr", "instanceName": "Prowlarr", "version": "2.1.3.5150",
			"isProduction": true, "isDocker": true, "branch": "master",
			// Prowlarr reports its OWN url base here, so the fixture does too.
			"authentication": "forms", "urlBase": urlBase, "runtimeVersion": "8.0.10",
			"databaseType": "sqLite", "databaseVersion": "3.46.0", "migrationVersion": 45,
			"startTime": time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339),
		})
	}))
	handle("GET /api/v1/health", f.authed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONTest(w, []map[string]any{{
			"id": 1, "source": "IndexerStatusCheck", "type": "warning",
			"message": "Indexers unavailable due to failures: Broken Tracker",
			"wikiUrl": "https://wiki.servarr.com/prowlarr/system#indexers-are-unavailable-due-to-failures",
		}})
	}))
	handle("GET /api/v1/indexer", f.authed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONTest(w, []map[string]any{
			indexerJSON(1, "Working Tracker", "torrent", "private", 25, true),
			indexerJSON(2, "Broken Tracker", "torrent", "public", 30, true),
			indexerJSON(3, "Disabled Tracker", "usenet", "private", 40, false),
		})
	}))
	handle("GET /api/v1/indexerstatus", f.authed(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		blocked := f.blockedIndexerID
		f.mu.Unlock()
		if blocked == 0 {
			// An empty array means everything is healthy — which is NOT the
			// same as the call having failed.
			writeJSONTest(w, []map[string]any{})
			return
		}
		writeJSONTest(w, []map[string]any{{
			"id": 7, "indexerId": blocked,
			"disabledTill":      time.Now().UTC().Add(20 * time.Minute).Format(time.RFC3339),
			"mostRecentFailure": time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
			"initialFailure":    time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
		}})
	}))
	handle("GET /api/v1/downloadclient", f.authed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONTest(w, []map[string]any{{
			"id": 1, "name": "qBittorrent", "implementation": "QBittorrent",
			"enable": true, "protocol": "torrent", "priority": 1, "supportsCategories": true,
		}})
	}))
	handle("GET /api/v1/search", f.authed(f.handleSearch))
	handle("POST /api/v1/search", f.authed(f.handleGrab))

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// URL is the ORIGIN only, never the sub-path: UsArr stores the two in separate
// columns (base_url and url_base) because the credential's AAD is bound to the
// origin and must survive a move to a different sub-path.
func (f *fakeProwlarr) URL() string { return f.srv.URL }

// URLBase is what belongs in the service row's url_base column.
func (f *fakeProwlarr) URLBase() string { return f.urlBase }

// authed records the key and enforces it, exactly as Prowlarr does: the key
// travels in X-Api-Key, never in the query string.
func (f *fakeProwlarr) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		f.mu.Lock()
		f.keySeen = append(f.keySeen, key)
		f.mu.Unlock()
		if key != f.apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSONTest(w, map[string]string{"error": "Unauthorized"})
			return
		}
		next(w, r)
	}
}

func (f *fakeProwlarr) handleSearch(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["indexerIds"]
	f.mu.Lock()
	for _, id := range ids {
		f.searched[id]++
	}
	f.mu.Unlock()

	query := r.URL.Query().Get("query")
	if len(ids) != 1 {
		// The fan-out addresses exactly one indexer per request. Anything else
		// means UsArr regressed to the aggregate endpoint, which gives no
		// per-indexer progress.
		f.t.Errorf("fake prowlarr: expected exactly one indexerIds parameter, got %v", ids)
	}
	if ids[0] != "1" {
		// Only indexer 1 is searchable in this fixture; 2 is blocked when the
		// test says so and 3 is disabled.
		writeJSONTest(w, []map[string]any{})
		return
	}

	writeJSONTest(w, []map[string]any{f.releaseJSON(1, "Working Tracker", query)})
}

// releaseJSON builds a ReleaseResource the way Prowlarr's
// SearchController.MapReleases does — INCLUDING the rewrite that embeds the full
// admin API key in downloadUrl and magnetUrl. That is the whole point of the
// fixture: the test then proves the key never reaches a response body or a log.
func (f *fakeProwlarr) releaseJSON(indexerID int32, indexerName, query string) map[string]any {
	title := "Test.Release.2026.1080p.WEB-DL.x264-USARR"
	if strings.TrimSpace(query) != "" {
		title = strings.ReplaceAll(strings.TrimSpace(query), " ", ".") + ".2026.1080p.WEB-DL.x264-USARR"
	}
	return map[string]any{
		"guid":      "https://tracker.example/details/1234",
		"age":       2,
		"ageHours":  50.5,
		"size":      2147483648,
		"indexerId": indexerID,
		"indexer":   indexerName,
		"title":     title,
		"sortTitle": strings.ToLower(title),
		"categories": []map[string]any{
			{"id": 2000, "name": "Movies", "subCategories": []map[string]any{{"id": 2040, "name": "Movies/HD"}}},
		},
		"indexerFlags": []string{"freeleech", "internal"},
		"publishDate":  time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
		"protocol":     "torrent",
		"seeders":      42,
		"leechers":     3,
		"infoHash":     "0123456789abcdef0123456789abcdef01234567",
		// An indexer-supplied URL carrying a deny-listed credential parameter.
		"infoUrl":    "https://tracker.example/details/1234?apikey=" + f.apiKey,
		"commentUrl": "https://tracker.example/details/1234#comments",
		"posterUrl":  "https://tracker.example/poster/1234.jpg",
		// THE CREDENTIAL. Prowlarr really does this on every search result.
		// {serverUrl}{urlBase}/{indexerId}/download?apikey=… — the sub-path is part
		// of what Prowlarr builds, so the fixture builds it the same way.
		"downloadUrl": fmt.Sprintf("%s%s/1/download?apikey=%s&link=abc123", f.srv.URL, f.urlBase, f.apiKey),
		"magnetUrl": fmt.Sprintf("magnet:?xt=urn:btih:0123456789abcdef&tr=%s%s/1/announce?apikey=%s",
			f.srv.URL, f.urlBase, f.apiKey),
	}
}

func (f *fakeProwlarr) handleGrab(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSONTest(w, map[string]string{"message": "bad body"})
		return
	}
	f.mu.Lock()
	f.grabbed = append(f.grabbed, body)
	f.mu.Unlock()

	if body["guid"] == nil || body["guid"] == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSONTest(w, map[string]any{"message": "guid must not be empty"})
		return
	}
	// A 200 echoing the resource back IS the confirmation: there is no download
	// id and no queue id.
	writeJSONTest(w, body)
}

func (f *fakeProwlarr) grabs() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, len(f.grabbed))
	copy(out, f.grabbed)
	return out
}

func (f *fakeProwlarr) searchCounts() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, v := range f.searched {
		out[k] = v
	}
	return out
}

func (f *fakeProwlarr) blockIndexer(id int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blockedIndexerID = id
}

func indexerJSON(id int32, name, protocol, privacy string, priority int32, enable bool) map[string]any {
	return map[string]any{
		"id": id, "name": name, "protocol": protocol, "privacy": privacy,
		"priority": priority, "enable": enable,
		"supportsRss": true, "supportsSearch": true, "supportsPagination": true,
		"implementation": "Cardigann", "definitionName": strings.ToLower(name),
		// fields[] carries the indexer's OWN credentials upstream. It is here so
		// the fixture is realistic; nothing in UsArr may serve it to a client.
		"fields": []map[string]any{
			{"order": 0, "name": "passkey", "label": "Passkey", "value": "indexer-passkey-should-not-leak", "privacy": "apiKey"},
		},
		"capabilities": map[string]any{
			"id": id, "limitsMax": 100, "limitsDefault": 50,
			"searchParams":      []string{"q"},
			"movieSearchParams": []string{"q", "imdbid"},
			"tvSearchParams":    []string{"q", "season", "ep"},
			"categories": []map[string]any{
				{"id": 2000, "name": "Movies", "subCategories": []map[string]any{{"id": 2040, "name": "Movies/HD"}}},
				{"id": 5000, "name": "TV", "subCategories": []map[string]any{{"id": 5040, "name": "TV/HD"}}},
			},
		},
	}
}

func writeJSONTest(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Application-Version", "2.1.3.5150")
	_ = json.NewEncoder(w).Encode(v)
}
