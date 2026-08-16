package main

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The credential the fake Prowlarr puts in every indexer's fields[]. It is the
// same string fakeprowlarr_test.go writes, and the whole of this file exists to
// prove it cannot reach a client.
const fixtureIndexerPasskey = "indexer-passkey-should-not-leak"

type indexerBody struct {
	InstanceID   int64  `json:"instance_id"`
	InstanceName string `json:"instance_name"`
	IndexerID    int64  `json:"indexer_id"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Privacy      string `json:"privacy"`
	Enabled      bool   `json:"enabled"`
	Priority     int64  `json:"priority"`

	SupportsSearch     bool `json:"supports_search"`
	SupportsRSS        bool `json:"supports_rss"`
	SupportsPagination bool `json:"supports_pagination"`

	SearchTypes   []string `json:"search_types"`
	LimitsMax     *int64   `json:"limits_max"`
	LimitsDefault *int64   `json:"limits_default"`

	Categories []indexerCategoryBody `json:"categories"`
	FetchedAt  time.Time             `json:"fetched_at"`
}

type indexerCategoryBody struct {
	ID            int32                 `json:"id"`
	Name          string                `json:"name"`
	SubCategories []indexerCategoryBody `json:"sub_categories"`
}

type indexerInstanceBody struct {
	InstanceID   int64      `json:"instance_id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Enabled      bool       `json:"enabled"`
	Status       string     `json:"status"`
	Message      string     `json:"message"`
	Action       string     `json:"action"`
	FetchedAt    *time.Time `json:"fetched_at"`
	IndexerCount int        `json:"indexer_count"`
}

type indexersBody struct {
	Status    string                `json:"status"`
	Message   string                `json:"message"`
	Action    string                `json:"action"`
	Instances []indexerInstanceBody `json:"instances"`
	Indexers  []indexerBody         `json:"indexers"`
}

// TestEndToEndIndexerCatalogue drives GET /api/v1/indexers through the whole
// real stack — the background prober fetching from a Prowlarr-shaped double,
// the projection, migration 0004's table, and the HTTP boundary.
//
// It exists for two claims that only hold end to end:
//
//  1. The picker is populated WITHOUT the user having searched, and the handler
//     itself never calls Prowlarr — the prober did, off the render path.
//  2. The tracker credential in Prowlarr's fields[] does not reach the client.
//     A private tracker's passkey is what it attributes traffic by, and a leak
//     is account termination; the assertion searches the WHOLE response body,
//     so it cannot pass because the test forgot to look at a field.
func TestEndToEndIndexerCatalogue(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"

	prowlarr := newFakeProwlarr(t, apiKey)
	env := newTestApp(t)

	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	// Before any service exists the endpoint answers 200 and says what is
	// missing. An error here would put a red box on a screen where nothing is
	// wrong — the install simply has not been set up.
	var empty indexersBody
	env.do(t, "GET", "/api/v1/indexers", nil, &empty)
	if empty.Status != "not_configured" {
		t.Fatalf("with nothing configured, status = %q, want not_configured", empty.Status)
	}
	if empty.Action != "Add Prowlarr" {
		t.Errorf("the empty state must name its one action, got %q", empty.Action)
	}
	t.Logf("empty install: status=%s message=%q action=%q", empty.Status, empty.Message, empty.Action)

	var svc serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prowlarr.URL(), "api_key": apiKey,
	}, &svc)

	// Adding a service asks for an out-of-band probe, and the probe is what
	// replicates the catalogue. NO SEARCH IS RUN ANYWHERE IN THIS TEST — that is
	// the point of it.
	var got indexersBody
	var raw string
	for range 100 {
		got = indexersBody{}
		var code int
		code, raw = env.raw(t, "GET", "/api/v1/indexers", nil)
		if code != http.StatusOK {
			t.Fatalf("GET /api/v1/indexers = %d: %s", code, raw)
		}
		mustUnmarshal(t, raw, &got)
		if len(got.Indexers) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// ── the credential assertion ────────────────────────────────────────────
	//
	// Over the RAW response body, not over a parsed field: fields[] would have
	// to survive as something for a parsed check to see it, and the failure
	// being guarded is precisely one that adds a member nobody thought about.
	if strings.Contains(raw, fixtureIndexerPasskey) {
		t.Fatalf("the indexers response carries a tracker passkey:\n%s", raw)
	}
	for _, banned := range []string{"passkey", "fields", "presets"} {
		if strings.Contains(strings.ToLower(raw), banned) {
			t.Errorf("the response mentions %q, which means the upstream fields array found a "+
				"way through:\n%s", banned, raw)
		}
	}
	// And the log is a boundary too.
	if strings.Contains(env.logs(), fixtureIndexerPasskey) {
		t.Error("a tracker passkey reached the log")
	}

	// ── the picker is actually usable ───────────────────────────────────────
	if got.Status != "ok" {
		t.Fatalf("status = %q, want ok: %s", got.Status, raw)
	}
	if len(got.Indexers) != 3 {
		t.Fatalf("got %d indexers, want the fixture's 3: %s", len(got.Indexers), raw)
	}
	if len(got.Instances) != 1 || got.Instances[0].IndexerCount != 3 {
		t.Fatalf("instance summary = %+v", got.Instances)
	}
	if got.Instances[0].FetchedAt == nil {
		t.Fatal("fetched_at is null after a successful refresh; the UI has no way to show staleness")
	}
	t.Logf("catalogue: status=%s message=%q fetched_at=%s",
		got.Status, got.Message, got.Instances[0].FetchedAt.Format(time.RFC3339))

	byName := map[string]indexerBody{}
	for _, ix := range got.Indexers {
		byName[ix.Name] = ix
		if ix.FetchedAt.IsZero() {
			t.Errorf("%s carries no fetched_at", ix.Name)
		}
		if ix.InstanceID != svc.ID || ix.InstanceName != "Prowlarr" {
			t.Errorf("%s does not name its instance: %+v", ix.Name, ix)
		}
	}

	working, ok := byName["Working Tracker"]
	if !ok {
		t.Fatalf("the fixture's indexers are missing: %v", byName)
	}
	if working.IndexerID != 1 || working.Protocol != "torrent" || working.Privacy != "private" {
		t.Errorf("Working Tracker = %+v", working)
	}
	if working.LimitsMax == nil || *working.LimitsMax != 100 {
		t.Errorf("limits_max = %v, want 100", working.LimitsMax)
	}
	// The fixture sets searchParams, tvSearchParams and movieSearchParams and
	// no music or book ones, so the picker can honestly grey out those two.
	if strings.Join(working.SearchTypes, ",") != "search,tvsearch,movie" {
		t.Errorf("search_types = %v", working.SearchTypes)
	}
	// The raw Newznab tree, sub-categories intact: this is what makes the
	// category filter work on a screen that has not searched yet.
	if len(working.Categories) != 2 || working.Categories[0].ID != 2000 ||
		len(working.Categories[0].SubCategories) != 1 || working.Categories[0].SubCategories[0].ID != 2040 {
		t.Errorf("categories = %+v", working.Categories)
	}

	// A DISABLED indexer is listed and marked, never hidden: hiding it makes
	// "why is my indexer missing?" unanswerable from this screen.
	disabled, ok := byName["Disabled Tracker"]
	if !ok || disabled.Enabled {
		t.Errorf("the disabled indexer is missing or reported as enabled: %+v", disabled)
	}
	if disabled.Protocol != "usenet" {
		t.Errorf("Disabled Tracker protocol = %q, want usenet", disabled.Protocol)
	}

	// ?instance= narrows to one service and validates it like the search path.
	var one indexersBody
	env.do(t, "GET", "/api/v1/indexers?instance="+strconv.FormatInt(svc.ID, 10), nil, &one)
	if len(one.Instances) != 1 || len(one.Indexers) != 3 {
		t.Errorf("filtered read = %d instances / %d indexers", len(one.Instances), len(one.Indexers))
	}
	if code, body := env.raw(t, "GET", "/api/v1/indexers?instance=99999", nil); code != http.StatusNotFound {
		t.Errorf("an unknown instance = %d, want 404: %s", code, body)
	}

	// ── the replica outlives the upstream ───────────────────────────────────
	//
	// The whole reason this is a table and not a process-lifetime map: a picker
	// that empties whenever Prowlarr blinks is a picker the user cannot trust.
	// The double is torn down and nothing re-probes, so what is served here is
	// what SQLite holds and nothing else.
	prowlarr.srv.Close()
	var afterOutage indexersBody
	env.do(t, "GET", "/api/v1/indexers", nil, &afterOutage)
	if len(afterOutage.Indexers) != 3 {
		t.Errorf("with Prowlarr down the catalogue served %d indexers, want the replicated 3",
			len(afterOutage.Indexers))
	}
	if afterOutage.Status != "ok" {
		t.Errorf("status with Prowlarr down = %q; the replica is still what it is", afterOutage.Status)
	}
}
