package servarr

import (
	"errors"
	"testing"
)

// Prowlarr does not take typed criteria as query parameters. SearchController
// regex-extracts {Key:Value} tokens out of `query`, so the token string IS the
// wire format and getting it wrong silently degrades the search.
func TestCriteriaTokens(t *testing.T) {
	tests := []struct {
		name     string
		criteria Criteria
		typ      SearchType
		want     string
		wantErr  error
	}{
		{
			name:     "the documented tv example",
			criteria: Criteria{Season: "1", Episode: "1"},
			typ:      SearchTypeTV,
			want:     "{Season:1}{Episode:1}",
		},
		{
			name:     "movie ids",
			criteria: Criteria{IMDbID: "tt15239678", Year: "2024"},
			typ:      SearchTypeMovie,
			want:     "{ImdbId:tt15239678}{Year:2024}",
		},
		{
			name:     "music",
			criteria: Criteria{Artist: "Boards of Canada", Album: "Geogaddi"},
			typ:      SearchTypeMusic,
			want:     "{Artist:Boards of Canada}{Album:Geogaddi}",
		},
		{
			name:     "book",
			criteria: Criteria{Author: "Le Guin", Title: "The Dispossessed"},
			typ:      SearchTypeBook,
			want:     "{Author:Le Guin}{Title:The Dispossessed}",
		},
		{
			name:     "empty criteria produce no tokens",
			criteria: Criteria{},
			typ:      SearchTypeBasic,
			want:     "",
		},
		{
			// Silently dropping it would surface as "the season filter did nothing".
			name:     "a criterion the type cannot express is an error, not a no-op",
			criteria: Criteria{Artist: "x"},
			typ:      SearchTypeTV,
			wantErr:  ErrInvalidRequest,
		},
		{
			// A brace in a value terminates the server-side regex early and corrupts
			// every token after it.
			name:     "a brace in a value is rejected",
			criteria: Criteria{Genre: "post{rock"},
			typ:      SearchTypeMovie,
			wantErr:  ErrInvalidRequest,
		},
		{
			name:     "an unknown search type is rejected client-side",
			criteria: Criteria{},
			typ:      SearchType("tv"),
			wantErr:  ErrInvalidRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.criteria.Tokens(tc.typ)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Tokens: %v", err)
			}
			if got != tc.want {
				t.Errorf("Tokens = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSearchRequestValues(t *testing.T) {
	req := SearchRequest{
		Text:       "The Simpsons",
		Type:       SearchTypeTV,
		Criteria:   Criteria{Season: "1", Episode: "1"},
		IndexerIDs: []int32{1, 2, 1}, // duplicate collapsed
		Categories: []int32{5000, 5070},
		Limit:      Int32(50),
	}
	v, err := req.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if got, want := v.Get("query"), "The Simpsons{Season:1}{Episode:1}"; got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
	if got := v.Get("type"); got != "tvsearch" {
		t.Errorf("type = %q", got)
	}

	// indexerIds and categories are REPEATED parameters, never comma-separated.
	if got := v["indexerIds"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("indexerIds = %v, want two repeated values", got)
	}
	if got := v["categories"]; len(got) != 2 {
		t.Errorf("categories = %v, want two repeated values", got)
	}
	if v.Encode() == "" || v.Get("limit") != "50" {
		t.Errorf("limit = %q", v.Get("limit"))
	}
}

func TestSearchRequestRejectsEmptyQuery(t *testing.T) {
	if _, err := (SearchRequest{}).Values(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
	// Criteria alone are enough: an id-only search is legitimate.
	if _, err := (SearchRequest{Type: SearchTypeMovie, Criteria: Criteria{IMDbID: "tt1"}}).Values(); err != nil {
		t.Fatalf("id-only search: %v", err)
	}
}

func TestSearchRequestDefaultsToBasicSearch(t *testing.T) {
	v, err := (SearchRequest{Text: "dune"}).Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if got := v.Get("type"); got != "search" {
		t.Errorf("type = %q, want search", got)
	}
}

// Prowlarr's own dedupe keeps the copy from the lowest-priority-value indexer.
// UsArr fans out per indexer and so has to redo it; using a different rule would
// make UsArr's results disagree with Prowlarr's own UI for the same query.
func TestDedupeReleasesKeepsLowestPriorityIndexer(t *testing.T) {
	priority := map[int32]int32{1: 25, 2: 10, 3: 50}
	priorityOf := func(id int32) (int32, bool) {
		p, ok := priority[id]
		return p, ok
	}

	in := []ReleaseResource{
		{GUID: "a", IndexerID: 1, Title: "from-25"},
		{GUID: "b", IndexerID: 3, Title: "unique"},
		{GUID: "a", IndexerID: 2, Title: "from-10"},
		{GUID: "a", IndexerID: 3, Title: "from-50"},
	}
	got := DedupeReleases(in, priorityOf)
	if len(got) != 2 {
		t.Fatalf("got %d releases, want 2", len(got))
	}
	for _, r := range got {
		if r.GUID == "a" && r.Title != "from-10" {
			t.Errorf("kept %q for guid a, want the priority-10 indexer's copy", r.Title)
		}
	}
}

func TestDedupeReleasesUnknownIndexerSortsLast(t *testing.T) {
	priorityOf := func(id int32) (int32, bool) {
		if id == 1 {
			return 40, true
		}
		return 0, false
	}
	in := []ReleaseResource{
		{GUID: "a", IndexerID: 9, Title: "unknown-indexer"},
		{GUID: "a", IndexerID: 1, Title: "known-indexer"},
	}
	got := DedupeReleases(in, priorityOf)
	if len(got) != 1 || got[0].Title != "known-indexer" {
		t.Errorf("got %+v, want the known indexer's copy", got)
	}
}

func TestEffectiveLimitFallsBackWhenCapsAreUnknown(t *testing.T) {
	if got := EffectiveLimit(IndexerCapabilityResource{}, 0); got != 100 {
		t.Errorf("EffectiveLimit with no caps = %d, want 100", got)
	}
	if got := EffectiveLimit(IndexerCapabilityResource{}, 25); got != 25 {
		t.Errorf("EffectiveLimit(25) = %d", got)
	}
}
