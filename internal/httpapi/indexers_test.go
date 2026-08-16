package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

type indexerFixture struct {
	server *Server
	userID int64
}

func newIndexerFixture(t *testing.T) indexerFixture {
	t.Helper()
	// NOTHING is wired into Releases, Tester or Probes. That is not an
	// oversight, it is the assertion: this endpoint renders a picker, so it may
	// not reach an *Arr, and a build with no upstream collaborator at all must
	// serve it perfectly.
	s := newTestServer(t, nil)
	userID, err := s.store.CreateUser(t.Context(), store.User{
		Username: "joe", AuthSource: "local", IsOwner: true,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return indexerFixture{server: s, userID: userID}
}

func (f indexerFixture) addService(t *testing.T, name string, enabled bool, role string) int64 {
	t.Helper()
	id, err := f.server.store.CreateServiceInstance(t.Context(), store.ServiceInstance{
		Kind: "prowlarr", Role: role, Name: name,
		BaseURL: "http://" + strings.ToLower(strings.ReplaceAll(name, " ", "-")) + ":9696",
		Enabled: enabled, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	return id
}

// list calls the handler directly with a session in the context, the same way
// the grab tests do, and returns the decoded body.
func (f indexerFixture) list(t *testing.T, query string) (int, indexersResponse, string) {
	t.Helper()
	ctx := t.Context()
	user, err := f.server.store.GetUser(ctx, f.userID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	ctx = context.WithValue(ctx, ctxKeySession, authSession{User: user})

	r := httptest.NewRequestWithContext(ctx, "GET", "/api/v1/indexers"+query, nil)
	w := httptest.NewRecorder()
	if err := f.server.handleListIndexers(w, r); err != nil {
		f.server.writeError(w, r, err)
		return w.Code, indexersResponse{}, w.Body.String()
	}
	var out indexersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding response: %v\n%s", err, w.Body.String())
	}
	return w.Code, out, w.Body.String()
}

func (f indexerFixture) replicate(t *testing.T, instanceID int64, ixs []store.Indexer) {
	t.Helper()
	if err := f.server.store.ReplaceIndexers(t.Context(), instanceID, ixs, time.Now()); err != nil {
		t.Fatalf("ReplaceIndexers: %v", err)
	}
}

// The four states an install can be in are four different sentences, and
// collapsing any of them into "no indexers" tells a user something false.
//
// This is principle 3 in the only form that matters: the screen has to be able
// to tell "you have not configured Prowlarr" from "we have not managed to read
// it yet" from "it answered, and it has none".
func TestIndexerCatalogueDegradesHonestly(t *testing.T) {
	t.Run("nothing configured", func(t *testing.T) {
		f := newIndexerFixture(t)
		code, got, body := f.list(t, "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", code, body)
		}
		if got.Status != catalogNotConfigured {
			t.Errorf("status = %q, want %q", got.Status, catalogNotConfigured)
		}
		if got.Action != "Add Prowlarr" {
			t.Errorf("action = %q; the state must name the one thing that changes it", got.Action)
		}
		if len(got.Instances) != 0 || len(got.Indexers) != 0 {
			t.Errorf("unexpected content: %+v", got)
		}
		// An empty ARRAY, never null: a client that has to branch on null before
		// it can iterate is one forgotten check from a crash on an empty install.
		if !strings.Contains(body, `"indexers":[]`) {
			t.Errorf("indexers must serialise as [] and not null: %s", body)
		}
	})

	t.Run("configured but never read", func(t *testing.T) {
		f := newIndexerFixture(t)
		f.addService(t, "Prowlarr", true, "indexer")

		_, got, _ := f.list(t, "")
		if got.Status != catalogNeverFetched {
			t.Fatalf("status = %q, want %q", got.Status, catalogNeverFetched)
		}
		if len(got.Instances) != 1 || got.Instances[0].Status != catalogNeverFetched {
			t.Fatalf("instance row: %+v", got.Instances)
		}
		if got.Instances[0].FetchedAt != nil {
			t.Error("fetched_at must be null when the list has never been read")
		}
		if !strings.Contains(got.Message, "not yet read") {
			t.Errorf("message does not say what is missing: %q", got.Message)
		}
	})

	t.Run("read, and genuinely empty", func(t *testing.T) {
		f := newIndexerFixture(t)
		id := f.addService(t, "Prowlarr", true, "indexer")
		f.replicate(t, id, nil)

		_, got, _ := f.list(t, "")
		if got.Status != catalogEmpty {
			t.Fatalf("status = %q, want %q", got.Status, catalogEmpty)
		}
		if got.Instances[0].FetchedAt == nil {
			t.Error("a successful-but-empty read must still report when it happened")
		}
		if !strings.Contains(got.Message, "no indexers configured") {
			t.Errorf("message = %q; it must say the service answered", got.Message)
		}
	})

	t.Run("disabled service", func(t *testing.T) {
		f := newIndexerFixture(t)
		id := f.addService(t, "Prowlarr", false, "indexer")
		f.replicate(t, id, []store.Indexer{{
			ServiceInstanceID: id, IndexerID: 1, Name: "Stale Tracker",
		}})

		_, got, _ := f.list(t, "")
		if got.Status != catalogServiceDisabled {
			t.Fatalf("status = %q, want %q", got.Status, catalogServiceDisabled)
		}
		// The rows are still in SQLite; offering them would populate a picker
		// with choices the search path refuses to use.
		if len(got.Indexers) != 0 {
			t.Errorf("a disabled service's indexers were offered: %+v", got.Indexers)
		}
		if got.Action != "Enable this service" {
			t.Errorf("action = %q", got.Action)
		}
	})
}

func TestIndexerCatalogueServesTheReplica(t *testing.T) {
	f := newIndexerFixture(t)
	id := f.addService(t, "Prowlarr", true, "indexer")
	f.replicate(t, id, []store.Indexer{
		{
			ServiceInstanceID: id, IndexerID: 7, Name: "Alpha Tracker",
			Protocol: "torrent", Privacy: "private", Enabled: true, Priority: 25,
			SupportsSearch: true, SupportsRSS: true, SupportsPagination: true,
			SearchTypesJSON: `["search","movie"]`,
			CategoriesJSON:  `[{"id":2000,"name":"Movies","sub_categories":[{"id":2040,"name":"Movies/HD"}]}]`,
		},
		{
			ServiceInstanceID: id, IndexerID: 8, Name: "Beta Tracker",
			Protocol: "usenet", Enabled: false,
		},
	})

	code, got, body := f.list(t, "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %s", code, body)
	}
	if got.Status != catalogOK || len(got.Indexers) != 2 {
		t.Fatalf("unexpected summary: %s / %d indexers", got.Status, len(got.Indexers))
	}

	alpha := got.Indexers[0]
	if alpha.IndexerID != 7 || alpha.Name != "Alpha Tracker" {
		t.Fatalf("first indexer = %+v", alpha)
	}
	if alpha.InstanceID != id || alpha.InstanceName != "Prowlarr" {
		t.Errorf("the flat list must name its instance: %+v", alpha)
	}
	if alpha.FetchedAt.IsZero() {
		t.Error("fetched_at must travel with the row so the UI can show staleness")
	}
	if len(alpha.SearchTypes) != 2 || alpha.SearchTypes[0] != "search" {
		t.Errorf("search types lost: %v", alpha.SearchTypes)
	}
	if len(alpha.Categories) != 1 || alpha.Categories[0].ID != 2000 ||
		len(alpha.Categories[0].SubCategories) != 1 {
		t.Errorf("the category tree did not survive the JSON column: %+v", alpha.Categories)
	}

	// A DISABLED indexer is listed and marked, not hidden. The upstream endpoint
	// returns disabled indexers with no filter parameter, and hiding one makes
	// "why is my indexer missing?" unanswerable from this screen.
	if got.Indexers[1].Enabled {
		t.Error("the disabled indexer is reported as enabled")
	}
	if got.Instances[0].IndexerCount != 2 {
		t.Errorf("indexer_count = %d, want 2", got.Instances[0].IndexerCount)
	}
}

// ?instance= narrows to one service and validates it the same way the search
// path does, because the two have to agree about what an indexer service is.
func TestIndexerCatalogueInstanceFilter(t *testing.T) {
	f := newIndexerFixture(t)
	prowlarr := f.addService(t, "Prowlarr", true, "indexer")
	second := f.addService(t, "Prowlarr Two", true, "indexer")
	radarr := f.addService(t, "Radarr", true, "library")

	f.replicate(t, prowlarr, []store.Indexer{{ServiceInstanceID: prowlarr, IndexerID: 1, Name: "One"}})
	f.replicate(t, second, []store.Indexer{{ServiceInstanceID: second, IndexerID: 1, Name: "Two"}})

	// Unfiltered: every indexer service the caller can see, so the picker
	// matches whatever the search is about to run against.
	_, all, _ := f.list(t, "")
	if len(all.Instances) != 2 || len(all.Indexers) != 2 {
		t.Fatalf("unfiltered read = %d instances / %d indexers, want 2/2",
			len(all.Instances), len(all.Indexers))
	}
	// A library-role service is not an indexer service and must not appear.
	for _, in := range all.Instances {
		if in.InstanceID == radarr {
			t.Error("a library-role service was listed as an indexer service")
		}
	}

	_, one, _ := f.list(t, "?instance="+strconv.FormatInt(prowlarr, 10))
	if len(one.Instances) != 1 || one.Instances[0].InstanceID != prowlarr {
		t.Fatalf("filtered read = %+v", one.Instances)
	}
	if len(one.Indexers) != 1 || one.Indexers[0].Name != "One" {
		t.Fatalf("filtered indexers = %+v", one.Indexers)
	}

	// A malformed request IS an error: these are bugs in the caller, not states
	// of an install.
	for _, tc := range []struct {
		name, query string
		want        int
		code        ErrorCode
	}{
		{"not an id", "?instance=nope", http.StatusBadRequest, CodeBadRequest},
		{"no such service", "?instance=99999", http.StatusNotFound, CodeNotFound},
		{"not an indexer", "?instance=" + strconv.FormatInt(radarr, 10), http.StatusBadRequest, CodeBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, body := f.list(t, tc.query)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %s", code, tc.want, body)
			}
			if !strings.Contains(body, string(tc.code)) {
				t.Errorf("body does not name %s: %s", tc.code, body)
			}
		})
	}
}

// The endpoint must be unreachable without a session, like every other read.
func TestIndexerCatalogueNeedsASession(t *testing.T) {
	f := newIndexerFixture(t)
	r := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/indexers", nil)
	w := httptest.NewRecorder()
	err := f.server.handleListIndexers(w, r)
	if err == nil {
		t.Fatal("a request with no session was served")
	}
	if !strings.Contains(err.Error(), string(CodeUnauthorized)) {
		t.Errorf("err = %v, want unauthorized", err)
	}
}

// The scope is in the signature AND applied in the SQL. A caller who can see no
// instance reads no indexers — the same fail-closed shape the audit read was
// fixed to have.
func TestIndexerCatalogueIsScoped(t *testing.T) {
	f := newIndexerFixture(t)
	id := f.addService(t, "Prowlarr", true, "indexer")
	f.replicate(t, id, []store.Indexer{{ServiceInstanceID: id, IndexerID: 1, Name: "One"}})

	// A non-owner in v0.1 gets Scope{UserID} — AllInstances false, no visible
	// instances — which must render as "sees nothing", never "sees everything".
	blind := store.Scope{UserID: f.userID}
	got, err := f.server.store.ListIndexers(t.Context(), blind, id)
	if err != nil {
		t.Fatalf("ListIndexers: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a scope that can see no instance read %d indexers", len(got))
	}
}
