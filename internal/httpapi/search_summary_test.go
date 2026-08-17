package httpapi

import (
	"database/sql"
	"testing"

	"github.com/jdb3750/UsArr/internal/releases"
	"github.com/jdb3750/UsArr/internal/store"
)

// The degraded summary is user-visible copy on the first screen a new install
// reaches, and the one-indexer install is the common case — "1 of 1 indexers
// answered" shipped and survived there. Copy bugs come back, so the singular is
// asserted rather than assumed.
func TestReportSummaryAgreesWithItsCounts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		rep  releases.Report
		want string
	}{
		{
			// The bug this test exists for.
			name: "one indexer, healthy",
			rep:  releases.Report{TotalIndexers: 1, Answered: 1},
			want: "1 of 1 indexer answered",
		},
		{
			// The noun agrees with the denominator, not with the numerator.
			name: "one indexer, none answered",
			rep:  releases.Report{TotalIndexers: 1, Failed: 1},
			want: "0 of 1 indexer answered — 1 failed, 0 skipped; these results are incomplete",
		},
		{
			name: "no indexers at all",
			rep:  releases.Report{},
			want: "0 of 0 indexers answered",
		},
		{
			name: "many indexers, healthy",
			rep:  releases.Report{TotalIndexers: 9, Answered: 9},
			want: "9 of 9 indexers answered",
		},
		{
			name: "many indexers, degraded",
			rep:  releases.Report{TotalIndexers: 8, Answered: 5, Failed: 2, Skipped: 1},
			want: "5 of 8 indexers answered — 2 failed, 1 skipped; these results are incomplete",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := reportSummary(tc.rep); got != tc.want {
				t.Errorf("reportSummary() = %q, want %q", got, tc.want)
			}
		})
	}
}

// countNoun is the package's single pluralisation idiom; the catalogue sentences
// below and the search summary above all route through it.
func TestCountNoun(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		n    int
		noun string
		want string
	}{
		{0, "indexer", "0 indexers"},
		{1, "indexer", "1 indexer"},
		{2, "indexer", "2 indexers"},
		{1, "indexer service", "1 indexer service"},
		{3, "indexer service", "3 indexer services"},
	} {
		if got := countNoun(tc.n, tc.noun); got != tc.want {
			t.Errorf("countNoun(%d, %q) = %q, want %q", tc.n, tc.noun, got, tc.want)
		}
	}
}

// The per-instance catalogue line carried the same bug: an install with one
// indexer configured read "1 indexers from "Prowlarr"".
func TestInstanceCatalogStateAgreesWithItsCount(t *testing.T) {
	t.Parallel()

	fetched := store.ServiceInstance{
		Name:              "Prowlarr",
		IndexersFetchedAt: sql.NullString{String: "2026-08-17T00:00:00Z", Valid: true},
	}

	for _, tc := range []struct {
		name  string
		count int
		want  string
	}{
		{"one", 1, `1 indexer from "Prowlarr"`},
		{"many", 6, `6 indexers from "Prowlarr"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, got, _ := instanceCatalogState(fetched, tc.count)
			if got != tc.want {
				t.Errorf("instanceCatalogState() message = %q, want %q", got, tc.want)
			}
		})
	}

	// Zero is its own sentence, not a count phrase — asserted so a later
	// pluralisation tidy-up does not collapse it into "0 indexers from ...".
	if _, got, _ := instanceCatalogState(fetched, 0); got != `"Prowlarr" answered and has no indexers configured` {
		t.Errorf("instanceCatalogState() at zero = %q", got)
	}
}

// catalogSummary and instanceCatalogState sit beside the search summary on the
// same screen, and carried the same bug.
func TestCatalogSummaryAgreesWithItsCounts(t *testing.T) {
	t.Parallel()

	one := []indexerInstanceResponse{{Name: "Prowlarr"}}
	two := []indexerInstanceResponse{{Name: "Prowlarr Public"}, {Name: "Prowlarr Private"}}

	for _, tc := range []struct {
		name      string
		instances []indexerInstanceResponse
		total     int
		want      string
	}{
		{"one indexer on one service", one, 1, "1 indexer across 1 indexer service"},
		{"many indexers on one service", one, 7, "7 indexers across 1 indexer service"},
		{"one indexer across two services", two, 1, "1 indexer across 2 indexer services"},
		{"many indexers across two services", two, 12, "12 indexers across 2 indexer services"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, got, _ := catalogSummary(tc.instances, tc.total)
			if got != tc.want {
				t.Errorf("catalogSummary() message = %q, want %q", got, tc.want)
			}
		})
	}
}
