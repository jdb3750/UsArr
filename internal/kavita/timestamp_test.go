package kavita

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTimestampParsesBothNETForms is the guard behind the bug that this package
// hit for real while it was being written.
//
// The vendored spec declares every one of these `format: date-time`, and Go's
// encoding/json unmarshals a time.Time with time.RFC3339 and NOTHING ELSE. But
// System.Text.Json writes a .NET DateTime whose Kind is Unspecified with no zone
// at all. Before Timestamp existed, decoding the series-list fixture failed with:
//
//	decoding response: parsing time "2026-08-17T03:00:30.118" as
//	"2006-01-02T15:04:05Z07:00": cannot parse "" as "Z07:00"
//
// — and because encoding/json aborts the whole document on one bad field, that
// ONE property made the ENTIRE series list undecodable.
func TestTimestampParsesBothNETForms(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		hasZone bool
		zero    bool
		year    int
	}{
		{raw: "2026-08-17T07:00:30.118Z", hasZone: true, year: 2026},
		{raw: "2026-08-17T09:00:30.118+02:00", hasZone: true, year: 2026},

		// The forms that broke it. Kind=Unspecified, so no offset — and .NET trims
		// trailing zeros from the fraction, so the precision varies.
		{raw: "2026-08-17T03:00:30.118", hasZone: false, year: 2026},
		{raw: "2026-08-17T03:00:30", hasZone: false, year: 2026},
		{raw: "2026-08-17T03:00:30.1189876", hasZone: false, year: 2026},

		// .NET's default DateTime, which is what latestReadDate is on every series
		// nobody has opened. It parses, and it must still read as "no time here".
		{raw: "0001-01-01T00:00:00", hasZone: false, zero: true, year: 1},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var ts Timestamp
			if err := json.Unmarshal([]byte(`"`+tc.raw+`"`), &ts); err != nil {
				t.Fatalf("unmarshal %q: %v", tc.raw, err)
			}
			if ts.Time().Year() != tc.year {
				t.Errorf("year = %d, want %d", ts.Time().Year(), tc.year)
			}
			if ts.HasZone() != tc.hasZone {
				t.Errorf("HasZone() = %v, want %v — a zoneless value is a WALL CLOCK on the Kavita host, "+
					"not an instant, and a caller comparing it across machines has to be able to tell",
					ts.HasZone(), tc.hasZone)
			}
			if ts.IsZero() != tc.zero {
				t.Errorf("IsZero() = %v, want %v", ts.IsZero(), tc.zero)
			}
			if ts.Raw() != tc.raw {
				t.Errorf("Raw() = %q, want %q", ts.Raw(), tc.raw)
			}
		})
	}
}

// TestTimestampDoesNotFailTheWholeDocument is the deliberate asymmetry with every
// other bound in this package. A byte bound FAILS rather than degrades, because a
// truncated list is indistinguishable from a deletion. An unparseable timestamp
// is one unusable field, and failing the decode would throw away the entire
// series list over it.
func TestTimestampDoesNotFailTheWholeDocument(t *testing.T) {
	const body = `[{"id":7,"name":"Kept","lastChapterAddedUtc":"tomorrow-ish"}]`
	c := streamClient(t, 200, body, nil)

	var got []SeriesDto
	page, err := c.StreamSeries(t.Context(), SeriesListOptions{}, func(s SeriesDto) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("one unreadable timestamp must not fail the read: %v", err)
	}
	if page.Count != 1 || got[0].Name != "Kept" {
		t.Fatalf("the element was dropped: %+v", got)
	}
	if !got[0].LastChapterAddedUtc.IsZero() {
		t.Error("an unparseable value must read as no-time-here, not as some invented instant")
	}
	if got[0].LastChapterAddedUtc.Raw() != "tomorrow-ish" {
		t.Errorf("the raw bytes must survive so a human can see what arrived: %q",
			got[0].LastChapterAddedUtc.Raw())
	}
}

func TestTimestampHandlesNullAndAbsent(t *testing.T) {
	var withNull struct {
		A Timestamp `json:"a"`
		B Timestamp `json:"b"`
	}
	if err := json.Unmarshal([]byte(`{"a":null}`), &withNull); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !withNull.A.IsZero() || withNull.A.Raw() != "" {
		t.Errorf("null decoded to %+v", withNull.A)
	}
	if !withNull.B.IsZero() {
		t.Errorf("an absent property decoded to %+v", withNull.B)
	}
	if err := json.Unmarshal([]byte(`{"a":12345}`), &withNull); err == nil {
		t.Error("a non-string date-time property should be an error, not a silent zero")
	}
}

// TestTimestampRoundTripsTheRawValue pins that marshalling re-emits the wire
// bytes. Re-rendering would append a `Z` to a zoneless value and manufacture
// exactly the claim about the instant that this type refuses to make.
func TestTimestampRoundTripsTheRawValue(t *testing.T) {
	for _, raw := range []string{"2026-08-17T03:00:30.118", "2026-08-17T07:00:30.118Z"} {
		ts := ParseTimestamp(raw)
		b, err := json.Marshal(ts)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(b) != `"`+raw+`"` {
			t.Errorf("round-trip of %q produced %s", raw, b)
		}
	}
	b, err := json.Marshal(Timestamp{})
	if err != nil {
		t.Fatalf("marshal zero: %v", err)
	}
	if string(b) != "null" {
		t.Errorf("a zero Timestamp marshalled as %s, want null", b)
	}
}

// ---- the browser-facing allowlist ------------------------------------------

// TestSeriesViewIsAnAllowlist fires the disclosure boundary. The projection is a
// distinct TYPE rather than a copy-with-fields-cleared precisely so that a field
// added upstream cannot ride along, and this asserts the marshalled bytes rather
// than the Go fields — which is the only level at which "did it reach the
// browser" is a real question.
func TestSeriesViewIsAnAllowlist(t *testing.T) {
	s := SeriesDto{
		ID: 41, Name: "Frieren", SortName: "Frieren", LibraryID: 1, LibraryName: "Manga",
		Format: MangaFormatArchive, Pages: 3812,
		LastChapterAddedUtc: ParseTimestamp("2026-08-17T07:00:30.118Z"),
		Created:             ParseTimestamp("2026-02-11T18:22:04.771Z"),
		PrimaryColor:        "#3d5a80",

		// Everything below must NOT survive the projection.
		FolderPath:        "/mnt/user/media/manga/Frieren",
		LowestFolderPath:  "/home/joe/media/manga/Frieren",
		LastFolderScanned: ParseTimestamp("2026-08-17T07:00:30.118Z"),
		LatestReadDate:    ParseTimestamp("2026-08-01T10:00:00Z"),
		PagesRead:         512,
		TotalReads:        3,
		UserRating:        4.5,
		HasUserRated:      true,
		IsBlacklisted:     true,
		DontMatch:         true,
		AniListID:         12345,
		MalID:             67890,
		ComicVineID:       "cv-1",
		CoverImage:        "series41",
	}

	raw, err := json.Marshal(ToView(s))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)

	for _, banned := range []struct{ substr, why string }{
		{"/mnt/user", "folderPath is a HOST FILESYSTEM PATH on the Kavita box"},
		{"/home/joe", "lowestFolderPath discloses the operator's home directory"},
		{"pages_read", "per-user read state belongs to the Kavita account, not the UsArr user asking"},
		{"total_reads", "same"},
		{"user_rating", "same"},
		{"latest_read", "same"},
		{"is_blacklisted", "upstream bookkeeping"},
		{"dont_match", "upstream bookkeeping"},
		{"last_folder_scanned", "a scan timestamp about the Kavita host"},
	} {
		if strings.Contains(body, banned.substr) {
			t.Errorf("SeriesView leaked %q — %s. Body: %s", banned.substr, banned.why, body)
		}
	}

	// And the allowlist actually carries what a screen needs.
	for _, want := range []string{`"id":41`, `"name":"Frieren"`, `"library_name":"Manga"`, `"primary_color"`} {
		if !strings.Contains(body, want) {
			t.Errorf("SeriesView dropped %s: %s", want, body)
		}
	}
}

func TestLibraryViewDropsTheHostPaths(t *testing.T) {
	l := LibraryDto{
		ID: 1, Name: "Manga", Type: LibraryTypeManga,
		Folders:         []string{"/mnt/user/media/manga"},
		ExcludePatterns: []string{"**/.@__thumb/**"},
	}
	raw, err := json.Marshal(ToLibraryView(l))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "/mnt/user") {
		t.Errorf("LibraryView leaked the library's host folders: %s", raw)
	}
	if !strings.Contains(string(raw), `"name":"Manga"`) {
		t.Errorf("LibraryView dropped the name §17.8 binds on: %s", raw)
	}
}
