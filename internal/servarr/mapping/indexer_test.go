package mapping

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/servarr"
)

// fixturePasskey is the credential this whole file exists to chase. It is
// deliberately word-shaped and unmistakable, so a leak assertion fails loudly
// rather than on a substring that might be a coincidence.
const fixturePasskey = "PASSKEY-cf19d0a7e4b28356-MUST-NOT-LEAK"

// indexerFixture is one GET /api/v1/indexer entry with a POPULATED fields[],
// transcribed from the shape Prowlarr actually returns (api/specs/prowlarr.json
// and testdata/cassettes/prowlarr_indexers.yaml).
//
// Every one of the four field entries is a credential class a private tracker
// really uses: the passkey it attributes traffic by, an API key, an RSS key,
// and a session cookie. Prowlarr marks three of them with
// privacy ∈ {apiKey, password}, and the fourth — the cookie — with nothing at
// all, which is exactly why UsArr does not filter this array by privacy and
// instead never carries it.
const indexerFixture = `{
  "id": 4,
  "name": "Private Tracker",
  "sortName": "private tracker",
  "implementation": "Cardigann",
  "implementationName": "Cardigann",
  "configContract": "CardigannSettings",
  "definitionName": "privatetracker",
  "infoLink": "https://wiki.servarr.com/prowlarr/supported-indexers",
  "indexerUrls": ["https://private.tracker.test/"],
  "fields": [
    {"order": 0, "name": "passkey",  "label": "Passkey",  "value": "` + fixturePasskey + `", "privacy": "apiKey"},
    {"order": 1, "name": "apiKey",   "label": "API Key",  "value": "` + fixturePasskey + `-api", "privacy": "apiKey"},
    {"order": 2, "name": "rssKey",   "label": "RSS Key",  "value": "` + fixturePasskey + `-rss", "privacy": "password"},
    {"order": 3, "name": "cookie",   "label": "Cookie",   "value": "session=` + fixturePasskey + `-cookie"}
  ],
  "presets": [
    {"id": 0, "name": "Private Tracker (preset)",
     "fields": [{"order": 0, "name": "passkey", "label": "Passkey", "value": "` + fixturePasskey + `-preset", "privacy": "apiKey"}]}
  ],
  "enable": true,
  "redirect": false,
  "supportsRss": true,
  "supportsSearch": true,
  "supportsRedirect": false,
  "supportsPagination": true,
  "protocol": "torrent",
  "privacy": "private",
  "priority": 25,
  "appProfileId": 1,
  "capabilities": {
    "id": 4,
    "limitsMax": 100,
    "limitsDefault": 50,
    "supportsRawSearch": false,
    "searchParams": ["q"],
    "tvSearchParams": ["q", "season", "ep"],
    "movieSearchParams": ["q", "imdbid"],
    "categories": [
      {"id": 5000, "name": "TV", "subCategories": [{"id": 5040, "name": "TV/HD"}]},
      {"id": 2000, "name": "Movies", "subCategories": [{"id": 2040, "name": "Movies/HD"}]}
    ]
  }
}`

func decodeIndexerFixture(t *testing.T) servarr.IndexerResource {
	t.Helper()
	var ix servarr.IndexerResource
	if err := json.Unmarshal([]byte(indexerFixture), &ix); err != nil {
		t.Fatalf("decoding the indexer fixture: %v", err)
	}
	// The fixture has to actually carry the credential, or the leak test below
	// passes by testing nothing — which is the failure mode that makes a
	// security test worse than no test.
	if len(ix.Fields) != 4 {
		t.Fatalf("the fixture decoded %d fields, want 4: the leak assertions would prove nothing", len(ix.Fields))
	}
	if len(ix.Presets) != 1 {
		t.Fatalf("the fixture decoded %d presets, want 1", len(ix.Presets))
	}
	found := false
	for _, f := range ix.Fields {
		if s, ok := f.Value.(string); ok && strings.Contains(s, fixturePasskey) {
			found = true
		}
	}
	if !found {
		t.Fatal("no field of the decoded fixture carries the passkey")
	}
	return ix
}

// TestCatalogProjectionCannotCarryAnIndexerCredential is the test the whole
// GET /api/v1/indexers path exists behind.
//
// Prowlarr's indexer list carries the user's tracker credentials in fields[].
// A leaked passkey is account termination on a private tracker, and this
// project has already had one credential reach permanent storage once. So the
// projection is an ALLOWLIST rather than a redaction pass, and the assertion is
// the strongest available: marshal the projection and search the WHOLE BYTE
// STRING for the credential, so it cannot hide in a member this test forgot to
// name.
func TestCatalogProjectionCannotCarryAnIndexerCredential(t *testing.T) {
	ix := decodeIndexerFixture(t)

	got := FromProwlarrIndexer(ix)
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshalling the projection: %v", err)
	}
	if strings.Contains(string(blob), fixturePasskey) {
		t.Fatalf("the projected indexer carries a tracker credential:\n%s", blob)
	}
	// Not just the one string: nothing that even smells of the fields array.
	for _, banned := range []string{"passkey", "rssKey", "apiKey", "cookie", "fields", "presets"} {
		if strings.Contains(strings.ToLower(string(blob)), strings.ToLower(banned)) {
			t.Errorf("the projection mentions %q, which means fields[] found a way through:\n%s", banned, blob)
		}
	}
}

// The projection has to be USEFUL as well as safe: a picker needs the id to
// send back, the name to render, and enough capability data to filter by
// category and by search type.
func TestCatalogProjectionKeepsWhatThePickerNeeds(t *testing.T) {
	got := FromProwlarrIndexer(decodeIndexerFixture(t))

	if got.ID != 4 || got.Name != "Private Tracker" {
		t.Errorf("identity lost: %+v", got)
	}
	if got.Protocol != "torrent" {
		t.Errorf("protocol = %q, want torrent", got.Protocol)
	}
	// The tag vocabulary is kebab-case; the wire form is camelCase. This is the
	// value a filter is written against.
	if got.Privacy != "private" {
		t.Errorf("privacy = %q, want private", got.Privacy)
	}
	if !got.Enabled || got.Priority != 25 {
		t.Errorf("enable/priority lost: %+v", got)
	}
	if !got.SupportsSearch || !got.SupportsRSS || !got.SupportsPagination {
		t.Errorf("capability flags lost: %+v", got)
	}
	if got.LimitsMax == nil || *got.LimitsMax != 100 ||
		got.LimitsDefault == nil || *got.LimitsDefault != 50 {
		t.Errorf("limits lost: %+v %+v", got.LimitsMax, got.LimitsDefault)
	}

	// searchParams/tvSearchParams/movieSearchParams are set; music and book are
	// not, and must not be claimed.
	want := []string{"search", "tvsearch", "movie"}
	if len(got.SearchTypes) != len(want) {
		t.Fatalf("search types = %v, want %v", got.SearchTypes, want)
	}
	for i := range want {
		if got.SearchTypes[i] != want[i] {
			t.Fatalf("search types = %v, want %v", got.SearchTypes, want)
		}
	}

	// The raw category tree survives, sub-categories included: 3030 and 7030 are
	// the only reliable machine signals for audiobook and comic, so a collapsed
	// tree is a lost distinction. Sorted by id, so a refresh over unchanged
	// upstream data produces an unchanged row.
	if len(got.Categories) != 2 || got.Categories[0].ID != 2000 || got.Categories[1].ID != 5000 {
		t.Fatalf("categories = %+v, want [2000 5000] in id order", got.Categories)
	}
	if len(got.Categories[0].SubCategories) != 1 || got.Categories[0].SubCategories[0].ID != 2040 {
		t.Errorf("sub-categories collapsed: %+v", got.Categories[0])
	}
}

// An RSS-only indexer serves NO search type, including the otherwise-universal
// basic search: the fan-out skips it outright with "RSS only". A picker that
// offered it would offer a choice that silently returns nothing.
func TestRSSOnlyIndexerAdvertisesNoSearchType(t *testing.T) {
	ix := decodeIndexerFixture(t)
	ix.SupportsSearch = false
	if got := FromProwlarrIndexer(ix).SearchTypes; len(got) != 0 {
		t.Errorf("an RSS-only indexer advertises %v, want none", got)
	}
}

// SupportsSearchType is the single home of a rule internal/releases' fan-out
// planner also acts on. If the two ever disagree the picker offers an indexer
// the planner then skips, which reads as a broken filter rather than as a
// deliberate skip.
func TestSupportsSearchTypeMatchesTheCapabilityArrays(t *testing.T) {
	caps := servarr.IndexerCapabilityResource{
		TvSearchParams: []string{"q"},
	}
	if !SupportsSearchType(caps, servarr.SearchTypeBasic) {
		t.Error("basic search is universal and must be supported with an empty searchParams")
	}
	if !SupportsSearchType(caps, servarr.SearchTypeTV) {
		t.Error("tvSearchParams is set, so tvsearch must be supported")
	}
	for _, t2 := range []servarr.SearchType{servarr.SearchTypeMovie, servarr.SearchTypeMusic, servarr.SearchTypeBook} {
		if SupportsSearchType(caps, t2) {
			t.Errorf("%s claimed with no params array", t2)
		}
	}
	if SupportsSearchType(caps, servarr.SearchType("nonsense")) {
		t.Error("an unknown search type must not be claimed")
	}
}
