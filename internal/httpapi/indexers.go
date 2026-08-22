package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// GET /api/v1/indexers lists the indexers each configured indexer service has,
// so the REQUESTS screen's indexer and category filters are populated on FIRST
// RENDER.
//
// REQUESTS, NOT SEARCH. This comment used to name Search, and that stopped
// being true when the release-results table, the indexer picker and Grab moved
// onto /requests — the free-text indexer path is §17.5's. /search is §17.4's
// search over the LOCAL REPLICA, which has no indexer to pick, so there is
// nothing here for it to populate. A reader deciding where the picker belongs
// should land on Requests.
//
// ⚠️ THAT SENTENCE READ `/search is now the deliberate not-built gap screen for
// §17.4's…`, WHICH ASSERTED A BUILD STATUS ON THIS COMMENT'S OWN AUTHORITY —
// §17.4 makes no such claim, and a comment is the wrong place for one either
// way. The scope argument above is what the picker's placement actually rests
// on and it does not depend on the answer. For the status itself:
// `web/src/routes` and internal/httpapi/server.go's route table are
// authoritative, and no sentence here is.
//
// ⚠️ AND NO CLIENT CALLS IT YET, as of this commit. The picker composed by
// web/src/routes/requests/+page.svelte builds its catalogue in
// $lib/indexerscope.svelte from the indexers that search SSE frames have
// already named — a union that only grows, and is therefore empty before the
// first search. This endpoint exists to replace that fallback with the
// configured list; until the client is wired to it, the contract below is
// served and unconsumed. It is not a claim that the picker reads it today.
//
// IT MAKES NO UPSTREAM CALL, AND THAT IS THE ONLY REASON IT CAN EXIST. The
// picker renders before the search does, so this is a render path, and a render
// path may not block on an *Arr (ARCHITECTURE.md §2.3 rule 1). Proxying
// Prowlarr's GET /api/v1/indexer here — even to build a small projection —
// would put a remote call under a browser's paint, which is precisely what the
// architecture is built to prevent. So the list is REPLICATED into
// indexer_catalog by the background prober (migration 0004,
// registry.RunProber → replicateIndexers in cmd/usarr/services.go) and this
// handler reads the replica, which is principle 1 applied literally: every
// user-facing read renders from local SQLite.
//
// fetched_at travels with every row and every instance so the UI can show
// staleness rather than pay for freshness in latency.
//
// IT ALWAYS ANSWERS 200 WHEN THE REQUEST ITSELF IS WELL-FORMED. "No indexer
// service configured" and "configured but never successfully read" are not
// errors — they are states of an install, and a picker that 4xx's on them makes
// a screen render an error box while everything else on it works. They are
// reported as a named `status` plus a sentence and the one action that changes
// it, which is principle 3. Only a malformed request fails: an ?instance= that
// is not an id, is not visible, or names a service that is not an indexer.
//
// SECURITY, LOAD-BEARING. Prowlarr's IndexerResource carries `fields[]` —
// passkeys, RSS keys, API keys and cookies for private trackers. Nothing in the
// chain that feeds this handler can carry one: mapping.CatalogIndexer projects
// the upstream resource member by member, store.Indexer has no member for it,
// and indexerResponse below is a third allowlist. There is no `fields`, no raw
// JSON pass-through and no map[string]any anywhere on the path.

// Catalogue statuses. They are a wire contract like ErrorCode, but they travel
// on a 200 and describe an install rather than a failure, so they are
// deliberately not error codes.
const (
	// catalogOK means at least one indexer is known.
	catalogOK = "ok"
	// catalogNotConfigured means no indexer-role service exists at all.
	catalogNotConfigured = "not_configured"
	// catalogNeverFetched means a service is configured and UsArr has never
	// successfully read its indexer list. Distinct from catalogEmpty on
	// purpose: one says "we have not managed to ask yet", the other says "we
	// asked and the answer was none", and they need different actions.
	catalogNeverFetched = "never_fetched"
	// catalogEmpty means the service answered and has no indexers configured.
	catalogEmpty = "empty"
	// catalogServiceDisabled means the service exists but the user turned it
	// off, so its indexers are deliberately not offered.
	catalogServiceDisabled = "service_disabled"
)

// indexerCategoryResponse is one Newznab/Torznab category. The tree is never
// collapsed: 3030 is the only reliable machine signal separating audiobook from
// music, and 7030 likewise for comics.
type indexerCategoryResponse struct {
	ID            int32                     `json:"id"`
	Name          string                    `json:"name,omitempty"`
	SubCategories []indexerCategoryResponse `json:"sub_categories,omitempty"`
}

// indexerResponse is one indexer as it crosses to a client.
//
// There is NO fields array, permanently, and no member that could hold one.
// See the security note above.
type indexerResponse struct {
	InstanceID   int64  `json:"instance_id"`
	InstanceName string `json:"instance_name,omitempty"`

	// IndexerID is the id to send back as ?indexer= on GET /api/v1/releases/search.
	IndexerID int64  `json:"indexer_id"`
	Name      string `json:"name"`

	Protocol string `json:"protocol"`          // usenet | torrent | unknown
	Privacy  string `json:"privacy,omitempty"` // public | semi-private | private

	// Enabled is routinely false: the upstream endpoint returns enabled AND
	// disabled indexers and has no filter parameter, so a disabled indexer is
	// listed and marked rather than hidden — hiding it makes "why is my indexer
	// missing?" unanswerable from this screen.
	Enabled bool `json:"enabled"`

	// Priority is 1-50 and LOWER WINS.
	Priority int64 `json:"priority"`

	SupportsSearch     bool `json:"supports_search"`
	SupportsRSS        bool `json:"supports_rss"`
	SupportsPagination bool `json:"supports_pagination"`

	// SearchTypes is the subset of search|tvsearch|movie|music|book this
	// indexer can serve. Empty means it is RSS-only and every search skips it.
	SearchTypes []string `json:"search_types"`

	// LimitsMax and LimitsDefault are absent when the indexer advertised
	// neither. Absent and 0 are different things.
	LimitsMax     *int64 `json:"limits_max,omitempty"`
	LimitsDefault *int64 `json:"limits_default,omitempty"`

	Categories []indexerCategoryResponse `json:"categories,omitempty"`

	// FetchedAt is when this row was replicated, so the UI can say how stale it
	// is instead of blocking to find out.
	FetchedAt time.Time `json:"fetched_at"`
}

// indexerInstanceResponse is one indexer service's contribution, WITHOUT its
// indexers: they are in the flat top-level list, each carrying its instance id,
// so the picker has one array to iterate and nothing is serialised twice.
type indexerInstanceResponse struct {
	InstanceID int64  `json:"instance_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Enabled    bool   `json:"enabled"`

	Status  string `json:"status"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`

	// FetchedAt is null when UsArr has never successfully read this instance's
	// indexer list. That is what separates "never asked" from "answered none".
	FetchedAt *time.Time `json:"fetched_at"`

	IndexerCount int `json:"indexer_count"`
}

type indexersResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`

	Instances []indexerInstanceResponse `json:"instances"`
	Indexers  []indexerResponse         `json:"indexers"`
}

func (s *Server) handleListIndexers(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}
	scope := storeScope(a)

	// ONE read, under ONE snapshot. The instance's indexers_fetched_at and its
	// indexer_catalog rows are written by a single transaction and have to be
	// read by a single transaction too: asking for them separately renders a
	// state the database never held — an instance reporting "UsArr has not yet
	// read the indexer list" beside the three indexers it just read. See
	// store.ReadIndexerCatalog.
	catalogs, err := s.indexerCatalogs(r, scope)
	if err != nil {
		return err
	}

	resp := indexersResponse{
		Instances: make([]indexerInstanceResponse, 0, len(catalogs)),
		Indexers:  []indexerResponse{},
	}
	for _, cat := range catalogs {
		si := cat.Instance
		row := indexerInstanceResponse{
			InstanceID: si.ID,
			Name:       si.Name,
			Kind:       si.Kind,
			Enabled:    si.Enabled,
		}
		if si.IndexersFetchedAt.Valid {
			if t, err := store.ParseTime(si.IndexersFetchedAt.String); err == nil {
				row.FetchedAt = &t
			}
		}

		// A disabled service's indexers are not offered: the search path refuses
		// to use it, so listing them would populate a picker with choices that
		// cannot be searched. The row still appears, with the reason and the one
		// action that changes it.
		if !si.Enabled {
			row.Status = catalogServiceDisabled
			row.Message = fmt.Sprintf("%q is disabled, so its indexers are not offered", si.Name)
			row.Action = "Enable this service"
			resp.Instances = append(resp.Instances, row)
			continue
		}

		// Already read, locally and in the same snapshot as si. The scope was
		// applied again inside that query even though the instance list was
		// already scoped: the parameter exists to be applied in the SQL, not
		// merely accepted.
		ixs := cat.Indexers
		for _, ix := range ixs {
			resp.Indexers = append(resp.Indexers, s.toIndexerResponse(si, ix))
		}
		row.IndexerCount = len(ixs)
		row.Status, row.Message, row.Action = instanceCatalogState(si, len(ixs))
		resp.Instances = append(resp.Instances, row)
	}

	resp.Status, resp.Message, resp.Action = catalogSummary(resp.Instances, len(resp.Indexers))
	writeJSON(w, http.StatusOK, resp)
	return nil
}

// indexerCatalogs resolves which services this request is about, and reads each
// one's replicated indexers alongside it in the same snapshot.
//
// With ?instance= it is that one, validated the same way the search path
// validates it, because the two have to agree about what "an indexer service"
// is. Without it, every indexer-role service the caller can see — the common
// install has exactly one, and a picker that silently picked one of two would
// show a list that does not match the search it is about to run.
func (s *Server) indexerCatalogs(r *http.Request, scope store.Scope) ([]store.IndexerCatalog, error) {
	if raw := strings.TrimSpace(r.URL.Query().Get("instance")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return nil, errStatus(http.StatusBadRequest, CodeBadRequest, "instance must be a service id")
		}
		cats, err := s.store.ReadIndexerCatalog(r.Context(), scope, id)
		if err != nil {
			return nil, notFoundOr(err, "service")
		}
		// A named instance resolves to exactly one catalogue or to ErrNotFound.
		// Checked rather than assumed: a handler must not index into a slice on
		// the strength of another package's invariant.
		if len(cats) != 1 {
			return nil, notFoundOr(store.ErrNotFound, "service")
		}
		if si := cats[0].Instance; si.Role != "indexer" {
			return nil, errStatus(http.StatusBadRequest, CodeBadRequest,
				fmt.Sprintf("%q is a %s service, not an indexer", si.Name, si.Role))
		}
		// A DISABLED instance is not an error here, unlike on the search path.
		// Search refuses because it was asked to do something it cannot do;
		// this is a list, and "it is off" is the most useful thing it can say.
		return cats, nil
	}

	all, err := s.store.ReadIndexerCatalog(r.Context(), scope, 0)
	if err != nil {
		return nil, errStatus(http.StatusInternalServerError, CodeInternal,
			"the configured services could not be read").wrapping(err)
	}
	out := make([]store.IndexerCatalog, 0, len(all))
	for _, cat := range all {
		if cat.Instance.Role == "indexer" {
			out = append(out, cat)
		}
	}
	return out, nil
}

// instanceCatalogState names one instance's state, the sentence for it, and the
// one action that changes it.
//
// The three cases are genuinely different and a single "no indexers" would
// collapse them into a message that is wrong for two of them.
func instanceCatalogState(si store.ServiceInstance, count int) (status, message, action string) {
	switch {
	case !si.IndexersFetchedAt.Valid:
		return catalogNeverFetched,
			fmt.Sprintf("UsArr has not yet read the indexer list from %q. It refreshes in the "+
				"background; if this does not clear, the connection is the thing to check", si.Name),
			"Test connection"
	case count == 0:
		return catalogEmpty,
			fmt.Sprintf("%q answered and has no indexers configured", si.Name),
			"Add an indexer in " + si.Name
	default:
		return catalogOK, fmt.Sprintf("%s from %q", countNoun(count, "indexer"), si.Name), ""
	}
}

// catalogSummary rolls the instances up into the one line the screen shows when
// the list is not usable. It never says "no indexers" for an install that has
// simply not been read yet.
func catalogSummary(instances []indexerInstanceResponse, total int) (status, message, action string) {
	if len(instances) == 0 {
		return catalogNotConfigured,
			"no indexer service is configured, so there are no indexers to filter by",
			"Add Prowlarr"
	}
	if total > 0 {
		return catalogOK,
			fmt.Sprintf("%s across %s", countNoun(total, "indexer"), countNoun(len(instances), "indexer service")),
			""
	}
	// Nothing to offer. Report the FIRST instance's own reason rather than a
	// generic one: with a single service — the common install — its sentence is
	// the whole truth, and with several the per-instance rows carry the rest.
	first := instances[0]
	return first.Status, first.Message, first.Action
}

// countNoun renders "1 indexer" / "3 indexers": the one pluralisation idiom in
// this package, so a second one does not grow beside it. Suffix-s nouns only —
// a noun with an irregular plural needs its own call site, not a rule here.
func countNoun(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// toIndexerResponse is the last of the three allowlists on this path. It copies
// named fields and decodes the two JSON columns into declared shapes; there is
// no member here that could carry a credential even if one somehow reached the
// row.
func (s *Server) toIndexerResponse(si store.ServiceInstance, ix store.Indexer) indexerResponse {
	out := indexerResponse{
		InstanceID:         si.ID,
		InstanceName:       si.Name,
		IndexerID:          ix.IndexerID,
		Name:               ix.Name,
		Protocol:           ix.Protocol,
		Privacy:            ix.Privacy,
		Enabled:            ix.Enabled,
		Priority:           ix.Priority,
		SupportsSearch:     ix.SupportsSearch,
		SupportsRSS:        ix.SupportsRSS,
		SupportsPagination: ix.SupportsPagination,
		SearchTypes:        []string{},
		FetchedAt:          ix.FetchedAt,
	}
	if ix.LimitsMax.Valid {
		v := ix.LimitsMax.Int64
		out.LimitsMax = &v
	}
	if ix.LimitsDefault.Valid {
		v := ix.LimitsDefault.Int64
		out.LimitsDefault = &v
	}

	// A column that will not decode is a UsArr bug, not a user-visible failure:
	// degrade to "this indexer advertises nothing" rather than 500 a whole
	// picker over one malformed row, and leave the trail in the log.
	if raw := strings.TrimSpace(ix.SearchTypesJSON); raw != "" && raw != "[]" {
		var types []string
		if err := json.Unmarshal([]byte(raw), &types); err != nil {
			s.log.Warn("indexer_catalog.search_types will not decode",
				"instance_id", si.ID, "indexer_id", ix.IndexerID, "err", redactText(err.Error()))
		} else {
			out.SearchTypes = types
		}
	}
	if raw := strings.TrimSpace(ix.CategoriesJSON); raw != "" && raw != "[]" {
		var cats []indexerCategoryResponse
		if err := json.Unmarshal([]byte(raw), &cats); err != nil {
			s.log.Warn("indexer_catalog.categories will not decode",
				"instance_id", si.ID, "indexer_id", ix.IndexerID, "err", redactText(err.Error()))
		} else {
			out.Categories = cats
		}
	}
	return out
}
