package mapping

import (
	"sort"

	"github.com/jdb3750/UsArr/internal/servarr"
)

// CatalogIndexer is the credential-free view of one configured indexer.
//
// IT IS AN ALLOWLIST, AND THAT IS THE WHOLE POINT OF THE TYPE. Prowlarr's
// IndexerResource carries `fields[]`, whose entries have
// privacy ∈ {password, apiKey, userName} — for a private tracker that array
// holds the user's PASSKEY, its RSS key, its API key or its session cookie. A
// leaked passkey is account termination on a private tracker, because it is
// what the tracker attributes traffic by.
//
// So this type does not redact `fields`, does not carry a `map[string]any` and
// does not embed the upstream resource: it names, one member at a time, the
// things a picker actually needs, and a credential has nowhere to land. The
// same reasoning as Release, which has no DownloadURL and no MagnetURL member
// for the same structural reason. Adding a member here is adding it to a
// security boundary — do not add one that can hold a value UsArr did not choose
// field by field.
//
// Presets is likewise absent: preset entries are IndexerResource values and
// carry their own fields[].
type CatalogIndexer struct {
	// ID is the id the SERVICE uses. It is what ?indexer= sends back to
	// GET /api/v1/search and what the search fan-out addresses.
	ID   int32
	Name string

	Protocol string // usenet | torrent | unknown
	Privacy  string // public | semi-private | private, or "" when unstated

	// Enabled mirrors IndexerResource.enable. GET /api/v1/indexer returns
	// enabled AND disabled indexers with no filter parameter, so a picker that
	// does not carry this flag offers indexers that will be skipped.
	Enabled bool

	// Priority is 1-50 and LOWER WINS — the same field the cross-indexer dedupe
	// tiebreaks on.
	Priority int32

	SupportsSearch     bool
	SupportsRSS        bool
	SupportsPagination bool

	// SearchTypes is UsArr's own vocabulary — search, tvsearch, movie, music,
	// book — derived from capabilities.*SearchParams. It is here so a picker can
	// say up front which indexers can serve the selected type, rather than
	// letting the user choose one that the fan-out skips with a reason.
	SearchTypes []string

	// LimitsMax and LimitsDefault are nil when the indexer advertised neither.
	// nil and 0 are different: 0 would read as "this indexer returns nothing".
	LimitsMax     *int32
	LimitsDefault *int32

	// Categories is the RAW Newznab/Torznab tree, never collapsed: 3030 is the
	// only reliable machine signal separating audiobook from music, and 7030
	// likewise for comics.
	Categories []CatalogCategory
}

// CatalogCategory is one Newznab/Torznab category, projected member by member
// for the same reason as CatalogIndexer.
type CatalogCategory struct {
	ID            int32             `json:"id"`
	Name          string            `json:"name,omitempty"`
	SubCategories []CatalogCategory `json:"sub_categories,omitempty"`
}

// FromProwlarrIndexer projects one IndexerResource.
//
// Every assignment below is deliberate. There is no `out := ix` and no struct
// embedding, so a field added upstream cannot arrive here by default — which is
// exactly how `fields[]` would otherwise reappear the day Prowlarr renames it.
func FromProwlarrIndexer(ix servarr.IndexerResource) CatalogIndexer {
	return CatalogIndexer{
		ID:                 ix.ID,
		Name:               ix.Name,
		Protocol:           SourceValue(ix.Protocol),
		Privacy:            PrivacyValue(ix.Privacy),
		Enabled:            ix.Enable,
		Priority:           ix.Priority,
		SupportsSearch:     ix.SupportsSearch,
		SupportsRSS:        ix.SupportsRSS,
		SupportsPagination: ix.SupportsPagination,
		SearchTypes:        SearchTypes(ix),
		LimitsMax:          ix.Capabilities.LimitsMax,
		LimitsDefault:      ix.Capabilities.LimitsDefault,
		Categories:         catalogCategories(ix.Capabilities.Categories),
	}
}

func catalogCategories(cs []servarr.IndexerCategory) []CatalogCategory {
	if len(cs) == 0 {
		return nil
	}
	out := make([]CatalogCategory, 0, len(cs))
	for _, c := range cs {
		out = append(out, CatalogCategory{
			ID:            c.ID,
			Name:          c.Name,
			SubCategories: catalogCategories(c.SubCategories),
		})
	}
	// Newznab ids are the stable identity; sorting by them makes the stored
	// projection byte-stable across refreshes, so an unchanged upstream produces
	// an unchanged row rather than a spurious diff.
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SupportsSearchType reports whether an indexer's capabilities can serve one
// search type.
//
// It lives here rather than in internal/releases because two callers now need
// the same answer — the fan-out planner, which skips an indexer with a stated
// reason, and the catalogue projection, which tells the picker the same thing
// before a search is run. Two copies of this rule would let the picker offer an
// indexer the planner then skips, which reads as a bug in the filter.
func SupportsSearchType(c servarr.IndexerCapabilityResource, t servarr.SearchType) bool {
	switch t {
	case servarr.SearchTypeBasic:
		// Basic search is universal; an empty searchParams array is normal.
		return true
	case servarr.SearchTypeTV:
		return len(c.TvSearchParams) > 0
	case servarr.SearchTypeMovie:
		return len(c.MovieSearchParams) > 0
	case servarr.SearchTypeMusic:
		return len(c.MusicSearchParams) > 0
	case servarr.SearchTypeBook:
		return len(c.BookSearchParams) > 0
	}
	return false
}

// SearchTypes lists the search types one indexer can serve, in a fixed order.
//
// An indexer whose supportsSearch is false serves NONE of them — it is RSS-only
// — and returns an empty list rather than the universal "search", because the
// fan-out skips it outright.
func SearchTypes(ix servarr.IndexerResource) []string {
	if !ix.SupportsSearch {
		return nil
	}
	all := []servarr.SearchType{
		servarr.SearchTypeBasic,
		servarr.SearchTypeTV,
		servarr.SearchTypeMovie,
		servarr.SearchTypeMusic,
		servarr.SearchTypeBook,
	}
	out := make([]string, 0, len(all))
	for _, t := range all {
		if SupportsSearchType(ix.Capabilities, t) {
			out = append(out, string(t))
		}
	}
	return out
}
