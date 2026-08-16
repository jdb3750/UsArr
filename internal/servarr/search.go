package servarr

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// SearchType is the `type` query parameter of GET /api/v1/search.
//
// An unrecognised value does NOT error server-side: Prowlarr silently degrades to
// a basic search, which looks like "the id filter did nothing" and is very hard to
// diagnose from the outside. So this is validated client-side.
type SearchType string

const (
	SearchTypeBasic SearchType = "search"
	SearchTypeTV    SearchType = "tvsearch"
	SearchTypeMovie SearchType = "movie"
	SearchTypeMusic SearchType = "music"
	SearchTypeBook  SearchType = "book"
)

// Valid reports whether t is one of the five values Prowlarr recognises.
func (t SearchType) Valid() bool {
	switch t {
	case SearchTypeBasic, SearchTypeTV, SearchTypeMovie, SearchTypeMusic, SearchTypeBook:
		return true
	}
	return false
}

// Magic indexer ids accepted by the indexerIds parameter.
const (
	// AllUsenetIndexers selects every enabled usenet indexer.
	AllUsenetIndexers int32 = -1
	// AllTorrentIndexers selects every enabled torrent indexer.
	AllTorrentIndexers int32 = -2
)

// Criteria carries the typed search criteria.
//
// Prowlarr does NOT accept these as query parameters. SearchController extracts
// them from the `query` string with a regex over `{Key:Value}` tokens, e.g.
//
//	query=The Simpsons{Season:1}{Episode:1}
//
// Keeping the Go API typed and building the tokens here means callers never hand-
// assemble that string, and an invalid combination is caught before the request.
type Criteria struct {
	// Shared / tv+movie identifiers.
	IMDbID   string // tt-prefixed or bare digits; Prowlarr accepts both
	TMDbID   string
	TraktID  string
	DoubanID string
	Year     string
	Genre    string

	// tvsearch only.
	TVDbID   string
	TVMazeID string
	RID      string // TvRage id
	Season   string
	Episode  string

	// music only.
	Artist string
	Album  string
	Track  string
	Label  string

	// book only.
	Author    string
	Publisher string
	Title     string
}

// criterion is one token: the wire key, the value, and the types that accept it.
type criterion struct {
	key   string
	value string
	types []SearchType
}

func (c Criteria) criteria() []criterion {
	tv := []SearchType{SearchTypeTV}
	movie := []SearchType{SearchTypeMovie}
	music := []SearchType{SearchTypeMusic}
	book := []SearchType{SearchTypeBook}
	tvMovie := []SearchType{SearchTypeTV, SearchTypeMovie}
	all := []SearchType{SearchTypeTV, SearchTypeMovie, SearchTypeMusic, SearchTypeBook}

	// Order is fixed so a request is byte-stable across runs, which is what makes a
	// cassette match on the URL at all.
	return []criterion{
		{"ImdbId", c.IMDbID, tvMovie},
		{"TmdbId", c.TMDbID, tvMovie},
		{"TraktId", c.TraktID, movie},
		{"DoubanId", c.DoubanID, tvMovie},
		{"TvdbId", c.TVDbID, tv},
		{"TvMazeId", c.TVMazeID, tv},
		{"RId", c.RID, tv},
		{"Season", c.Season, tv},
		{"Episode", c.Episode, tv},
		{"Artist", c.Artist, music},
		{"Album", c.Album, music},
		{"Track", c.Track, music},
		{"Label", c.Label, music},
		{"Author", c.Author, book},
		{"Publisher", c.Publisher, book},
		{"Title", c.Title, book},
		{"Year", c.Year, all},
		{"Genre", c.Genre, all},
	}
}

// Empty reports whether no criterion is set.
func (c Criteria) Empty() bool {
	for _, cr := range c.criteria() {
		if cr.value != "" {
			return false
		}
	}
	return true
}

// Tokens renders the criteria as `{Key:Value}` tokens for the given search type,
// in a fixed order.
//
// It fails rather than silently dropping a criterion the type cannot express: a
// caller that sets Artist on a tvsearch has a bug, and returning a query that
// quietly ignores it would surface as "search returns the wrong things".
func (c Criteria) Tokens(t SearchType) (string, error) {
	if !t.Valid() {
		return "", fmt.Errorf("%w: unknown search type %q", ErrInvalidRequest, string(t))
	}
	var b strings.Builder
	for _, cr := range c.criteria() {
		if cr.value == "" {
			continue
		}
		// The server-side regex is delimited by braces, so a brace in a value would
		// terminate the token early and corrupt every token after it.
		if strings.ContainsAny(cr.value, "{}") {
			return "", fmt.Errorf("%w: criterion %s contains a brace", ErrInvalidRequest, cr.key)
		}
		if !acceptsType(cr.types, t) {
			return "", fmt.Errorf("%w: criterion %s is not valid for type %q", ErrInvalidRequest, cr.key, string(t))
		}
		b.WriteByte('{')
		b.WriteString(cr.key)
		b.WriteByte(':')
		b.WriteString(cr.value)
		b.WriteByte('}')
	}
	return b.String(), nil
}

func acceptsType(types []SearchType, t SearchType) bool {
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

// SearchRequest is one GET /api/v1/search.
type SearchRequest struct {
	// Text is the free-text part. Criteria tokens are appended to it.
	Text     string
	Type     SearchType // "" means SearchTypeBasic
	Criteria Criteria

	// IndexerIDs may contain AllUsenetIndexers / AllTorrentIndexers. Empty means
	// "every enabled indexer" — and note that the ONLY case in which Prowlarr
	// returns 400 SearchFailedException instead of an empty 200 is when this was
	// explicitly non-empty and every named indexer was unavailable.
	IndexerIDs []int32

	// Categories are raw Newznab/Torznab ids.
	Categories []int32

	// Limit and Offset are PER INDEXER and are applied before cross-indexer
	// dedupe. There is no total count and no cursor. Respect each indexer's
	// capabilities.limitsMax, and check supportsPagination before sending Offset.
	Limit  *int32
	Offset *int32
}

// Values renders the request as query parameters.
//
// indexerIds and categories are REPEATED parameters, not comma-separated lists.
// Sending "1,2,3" makes Prowlarr's model binder fail or silently bind nothing.
func (r SearchRequest) Values() (url.Values, error) {
	t := r.Type
	if t == "" {
		t = SearchTypeBasic
	}
	if !t.Valid() {
		return nil, fmt.Errorf("%w: unknown search type %q", ErrInvalidRequest, string(r.Type))
	}

	tokens, err := r.Criteria.Tokens(t)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(r.Text) + tokens
	if query == "" {
		return nil, fmt.Errorf("%w: search needs free text or at least one criterion", ErrInvalidRequest)
	}

	v := url.Values{}
	v.Set("query", query)
	v.Set("type", string(t))
	for _, id := range dedupeInt32(r.IndexerIDs) {
		v.Add("indexerIds", strconv.FormatInt(int64(id), 10))
	}
	for _, c := range dedupeInt32(r.Categories) {
		v.Add("categories", strconv.FormatInt(int64(c), 10))
	}
	if r.Limit != nil {
		if *r.Limit <= 0 {
			return nil, fmt.Errorf("%w: limit must be positive", ErrInvalidRequest)
		}
		v.Set("limit", strconv.FormatInt(int64(*r.Limit), 10))
	}
	if r.Offset != nil {
		if *r.Offset < 0 {
			return nil, fmt.Errorf("%w: offset must not be negative", ErrInvalidRequest)
		}
		v.Set("offset", strconv.FormatInt(int64(*r.Offset), 10))
	}
	return v, nil
}

// dedupeInt32 keeps first-seen order stable so the encoded URL is deterministic.
func dedupeInt32(in []int32) []int32 {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[int32]bool, len(in))
	out := make([]int32, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// Int32 is a helper for the pointer-valued optional fields.
func Int32(v int32) *int32 { return &v }

// EffectiveLimit clamps want against an indexer's advertised caps. A limit above
// limitsMax is rejected by some indexers outright and silently clamped by others,
// so UsArr clamps it itself and always knows what it asked for.
func EffectiveLimit(caps IndexerCapabilityResource, want int32) int32 {
	if want <= 0 {
		if caps.LimitsDefault != nil && *caps.LimitsDefault > 0 {
			return *caps.LimitsDefault
		}
		return 100
	}
	if caps.LimitsMax != nil && *caps.LimitsMax > 0 && want > *caps.LimitsMax {
		return *caps.LimitsMax
	}
	return want
}

// DedupeReleases collapses releases that share a guid, keeping the copy from the
// indexer with the LOWEST priority value.
//
// This mirrors Prowlarr's own rule, which matters because UsArr fans out one
// request per indexer to get progressive results and therefore has to redo the
// cross-indexer dedupe that the aggregate endpoint would have done for it. Using a
// different rule would make UsArr's results differ from Prowlarr's UI for the same
// query, which reads as a bug.
//
// priorityOf returns the priority of an indexer id; a missing indexer sorts last.
func DedupeReleases(in []ReleaseResource, priorityOf func(indexerID int32) (int32, bool)) []ReleaseResource {
	type slot struct {
		rel      ReleaseResource
		priority int32
		order    int
	}
	best := make(map[string]slot, len(in))
	for i, r := range in {
		p := int32(1 << 20) // sorts after any real 1-50 priority
		if priorityOf != nil {
			if got, ok := priorityOf(r.IndexerID); ok {
				p = got
			}
		}
		cur, ok := best[r.GUID]
		if !ok || p < cur.priority {
			best[r.GUID] = slot{rel: r, priority: p, order: i}
		}
	}
	out := make([]slot, 0, len(best))
	for _, s := range best {
		out = append(out, s)
	}
	// Stable, input-order output: callers rank afterwards, and a nondeterministic
	// order here would make tests flake on map iteration.
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	res := make([]ReleaseResource, len(out))
	for i, s := range out {
		res[i] = s.rel
	}
	return res
}
