package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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
	// Read the raw bytes first: the model binder rejects a body by BYTE POSITION,
	// and the position is the most useful half of the message it returns.
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSONTest(w, map[string]string{"message": "bad body"})
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSONTest(w, map[string]string{"message": "bad body"})
		return
	}
	f.mu.Lock()
	f.grabbed = append(f.grabbed, body)
	f.mu.Unlock()

	// MODEL BINDING RUNS BEFORE THE CONTROLLER. A fixture that accepts whatever it
	// is sent validates nothing, and that is exactly how UsArr shipped a grab body
	// carrying `"protocol":""` — rejected by every real Prowlarr, accepted here.
	if pd := bindReleaseResource(raw, body); pd != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSONTest(w, pd)
		return
	}

	if body["guid"] == nil || body["guid"] == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSONTest(w, map[string]any{"message": "guid must not be empty"})
		return
	}
	// A 200 echoing the resource back IS the confirmation: there is no download
	// id and no queue id.
	writeJSONTest(w, body)
}

// releaseResourceEnums are the ReleaseResource properties whose C# member is an
// enum, with the values System.Text.Json's JsonStringEnumConverter accepts.
//
// An ABSENT property is fine — the member keeps its default. A PRESENT property
// must convert. "" is not a member name, so it is a hard failure rather than a
// synonym for absent, which is the distinction Go's zero values erase.
var releaseResourceEnums = map[string]struct {
	csharpType string
	values     []string
}{
	"protocol": {"NzbDrone.Core.Indexers.DownloadProtocol", []string{"unknown", "usenet", "torrent"}},
}

// releaseResourceDates are the properties bound to a C# DateTime. System.Text.Json
// requires ISO 8601-1 extended format; "" and a bare "0" do not parse.
var releaseResourceDates = []string{"publishDate"}

// bindReleaseResource is the fixture's stand-in for ASP.NET model binding, and
// returns the ValidationProblemDetails a real instance returns on failure — down
// to the byte position, because that is what makes the message diagnosable.
func bindReleaseResource(raw []byte, body map[string]any) map[string]any {
	fail := func(prop, msg string) map[string]any {
		return map[string]any{
			"type":   "https://tools.ietf.org/html/rfc9110#section-15.5.1",
			"title":  "One or more validation errors occurred.",
			"status": http.StatusBadRequest,
			"errors": map[string]any{"$." + prop: []string{msg}},
		}
	}

	for prop, spec := range releaseResourceEnums {
		v, present := body[prop]
		if !present {
			continue
		}
		if n, ok := v.(float64); ok && n == float64(int(n)) && int(n) >= 0 && int(n) < len(spec.values) {
			continue // integers are accepted on input
		}
		// JsonStringEnumConverter matches member names case-insensitively.
		s, ok := v.(string)
		if ok && slices.ContainsFunc(spec.values, func(m string) bool { return strings.EqualFold(m, s) }) {
			continue
		}
		return fail(prop, fmt.Sprintf(
			"The JSON value could not be converted to %s. Path: $.%s | LineNumber: 0 | BytePositionInLine: %d.",
			spec.csharpType, prop, valueBytePosition(raw, prop)))
	}

	for _, prop := range releaseResourceDates {
		v, present := body[prop]
		if !present {
			continue
		}
		s, ok := v.(string)
		if ok {
			if _, err := time.Parse(time.RFC3339, s); err == nil {
				continue
			}
		}
		return fail(prop, fmt.Sprintf(
			"The JSON value could not be converted to System.DateTime. Path: $.%s | LineNumber: 0 | BytePositionInLine: %d.",
			prop, valueBytePosition(raw, prop)))
	}

	return nil
}

// valueBytePosition reports where prop's value ENDS in the raw body.
//
// Utf8JsonReader has already consumed the value token when the converter throws,
// so BytePositionInLine points past it, not at it. The distinction is two bytes on
// an empty string and it is the difference between a fixture that reproduces a
// reported failure and one that merely resembles it: the live grab reported
// BytePositionInLine 202 for a 224-byte body whose `"protocol":""` value starts at
// 200.
func valueBytePosition(raw []byte, prop string) int {
	i := bytes.Index(raw, []byte(`"`+prop+`":`))
	if i < 0 {
		return 0
	}
	pos := i + len(prop) + 3
	for pos < len(raw) && (raw[pos] == ' ' || raw[pos] == '\t') {
		pos++
	}
	if pos < len(raw) && raw[pos] == '"' {
		for end := pos + 1; end < len(raw); end++ {
			if raw[end] == '\\' {
				end++
				continue
			}
			if raw[end] == '"' {
				return end + 1
			}
		}
		return len(raw)
	}
	for end := pos; end < len(raw); end++ {
		if raw[end] == ',' || raw[end] == '}' {
			return end
		}
	}
	return len(raw)
}

// TestFakeProwlarrBindsTheGrabBodyStrictly proves the fixture's model binding is
// live, not decorative.
//
// It exists because the previous fixture accepted whatever it was sent, so the
// end-to-end test passed green while every grab against the owner's real Prowlarr
// failed with a 400. A test double that validates nothing is worse than no double:
// it converts an unshippable bug into a passing suite.
func TestFakeProwlarrBindsTheGrabBodyStrictly(t *testing.T) {
	f := newFakeProwlarr(t, "test-api-key")

	for _, tc := range []struct {
		name, body string
		wantStatus int
		wantErrKey string
		wantDetail string
	}{
		{
			// THE REGRESSION. A ReleaseResource zeroed down to three fields still
			// marshals `"protocol":""`, and an empty string is not a member name.
			name:       "empty protocol enum",
			body:       `{"guid":"https://tracker.example/details/1234","indexerId":1,"protocol":""}`,
			wantStatus: http.StatusBadRequest,
			wantErrKey: "$.protocol",
		},
		{
			name:       "unrecognised protocol enum",
			body:       `{"guid":"g","indexerId":1,"protocol":"bittorrent"}`,
			wantStatus: http.StatusBadRequest,
			wantErrKey: "$.protocol",
		},
		{
			name:       "unparseable date",
			body:       `{"guid":"g","indexerId":1,"publishDate":""}`,
			wantStatus: http.StatusBadRequest,
			wantErrKey: "$.publishDate",
		},
		{
			// THE CAPTURED BODY, verbatim from the failing install: 224 bytes, and
			// the rejection must land on byte 202 exactly as the live Prowlarr
			// reported. A fixture that gets the position wrong is reproducing a
			// lookalike, not the failure.
			name: "the body that failed on the live install",
			body: `{"guid":"https://tracker.example/details/1234","age":0,"ageHours":0,"ageMinutes":0,` +
				`"size":0,"indexerId":1,"imdbId":0,"tmdbId":0,"tvdbId":0,"tvMazeId":0,` +
				`"publishDate":"0001-01-01T00:00:00Z","protocol":"","downloadClientId":3}`,
			wantStatus: http.StatusBadRequest,
			wantErrKey: "$.protocol",
			wantDetail: "BytePositionInLine: 202.",
		},
		{
			// An ABSENT enum is fine: the C# member keeps its default. This is the
			// asymmetry the bug turned on, so it is pinned in both directions.
			name:       "protocol omitted entirely",
			body:       `{"guid":"g","indexerId":1,"downloadClientId":1}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "protocol as its integer ordinal",
			body:       `{"guid":"g","indexerId":1,"protocol":2}`,
			wantStatus: http.StatusOK,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
				f.URL()+f.URLBase()+"/api/v1/search", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Api-Key", "test-api-key")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.wantStatus, raw)
			}
			if tc.wantErrKey == "" {
				return
			}
			var pd struct {
				Title  string              `json:"title"`
				Errors map[string][]string `json:"errors"`
			}
			if err := json.Unmarshal(raw, &pd); err != nil {
				t.Fatalf("the rejection must be a ValidationProblemDetails, as ASP.NET returns: %v (%s)", err, raw)
			}
			if pd.Title != "One or more validation errors occurred." {
				t.Errorf("title = %q, want ASP.NET's own wording", pd.Title)
			}
			msgs, ok := pd.Errors[tc.wantErrKey]
			if !ok {
				t.Errorf("errors has no %q entry: %s", tc.wantErrKey, raw)
			}
			if tc.wantDetail != "" && (len(msgs) == 0 || !strings.Contains(msgs[0], tc.wantDetail)) {
				t.Errorf("message = %v, want it to contain %q", msgs, tc.wantDetail)
			}
			t.Logf("%s -> %d %s", tc.body, resp.StatusCode, raw)
		})
	}
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
