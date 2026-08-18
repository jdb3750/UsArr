package libsync

import (
	"database/sql"
	"testing"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// TestReleaseYearOfMirrorsKavitasOwnValidityFloor pins the SENTINEL rule, which
// is the only reason this projection is not a cast.
//
// The floor is not UsArr's invention and the table says so member by member:
// Kavita's own `NumberHelper.IsValidYear(int number) => number is >= 1000`
// (Kavita.Common/Helpers/NumberHelper.cs:7) gates BOTH of its writers, and the
// scanner's `MinimumReleaseYear` returns `DefaultIfEmpty()`'s 0 for a series
// whose chapters carry no usable date (ChapterListExtensions.cs:51-54). So 0 is
// "no year", not year zero — and Kavita reads it back that way itself
// (`.Where(sm => sm.ReleaseYear != 0)`, StatisticService.cs:111).
//
// A 0 written through to work.year renders in the Home screen's Year column as
// a year the upstream never claimed, which is strictly worse than the em dash a
// NULL renders as. That is the failure this guards.
func TestReleaseYearOfMirrorsKavitasOwnValidityFloor(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   int32
		want sql.NullInt64
	}{
		{"a real year", 2012, sql.NullInt64{Int64: 2012, Valid: true}},
		{"Kavita's own sentinel for 'no year'", 0, sql.NullInt64{}},
		{"the floor itself is valid", 1000, sql.NullInt64{Int64: 1000, Valid: true}},
		{"one below the floor is not", 999, sql.NullInt64{}},
		{"a negative year", -1, sql.NullInt64{}},
		// DateTime.MinValue.Year. It cannot reach releaseYear through
		// MinimumReleaseYear's filter, but SeriesService's writer is not the
		// only thing that has ever written this column and the projection must
		// not depend on that.
		{"the .NET zero date's year", 1, sql.NullInt64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := releaseYearOf(kavita.SeriesMetadataDto{ReleaseYear: tc.in})
			if got != tc.want {
				t.Errorf("releaseYearOf(%d) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

// TestStreamCreditsCarriesTheYearOnTheSameRoundTrip is the wiring half: the
// metadata read that produces the credits must hand the year over WITH them.
// Without this the projection above is correct and unreachable.
func TestStreamCreditsCarriesTheYearOnTheSameRoundTrip(t *testing.T) {
	c := &fakeCreditClient{md: map[int32]kavita.SeriesMetadataDto{
		1: {ReleaseYear: 2012, Writers: []kavita.PersonDto{person("Brian K. Vaughan")}},
		2: {ReleaseYear: 0, Writers: []kavita.PersonDto{person("Fiona Staples")}},
	}}
	src := NewKavitaSource(c)

	var got []store.CreditSet
	if _, err := src.StreamCredits(t.Context(), []CreditRequest{
		{RemoteKind: "series", RemoteID: "1", Kind: "comic"},
		{RemoteKind: "series", RemoteID: "2", Kind: "comic"},
	}, func(s store.CreditSet) error { got = append(got, s); return nil }); err != nil {
		t.Fatalf("StreamCredits: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("delivered %d sets, want 2", len(got))
	}
	if want := (sql.NullInt64{Int64: 2012, Valid: true}); got[0].Year != want {
		t.Errorf("series 1 delivered Year %+v, want %+v", got[0].Year, want)
	}
	if got[1].Year.Valid {
		t.Errorf("series 2 delivered Year %+v for Kavita's 0, want NULL", got[1].Year)
	}
	// One call per series and no more: the year rides on the credit read rather
	// than paying a second GET for a field already in the response.
	if len(c.calls) != 2 {
		t.Errorf("made %d SeriesMetadata calls for 2 series, want 2", len(c.calls))
	}
}
