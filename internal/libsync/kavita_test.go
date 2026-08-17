package libsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// kavitaSpecFiles is BOTH vendored Kavita specs (ADR-0046): the FLOOR
// (`kavita-v0.9.0.2.json`, the stable release the owner runs) and the CEILING
// (`kavita-develop.json`, where the API is defined). Every spec-reading test in
// this package runs against both, in a subtest named for the file, because a
// green against develop alone is evidence about a branch nobody deploys.
// api/specs/SOURCES.md owns the provenance of each.
var kavitaSpecFiles = []string{"kavita-v0.9.0.2.json", "kavita-develop.json"}

func readKavitaSpec(t *testing.T, file string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "specs", file))
	if err != nil {
		t.Fatalf("read the vendored spec %s (see api/specs/SOURCES.md): %v", file, err)
	}
	return raw
}

// TestEveryLibraryTypeMemberIsMapped reads BOTH VENDORED SPECS and requires a
// decision for every member either one declares.
//
// This is the guard that makes the mapping's "every member is accounted for"
// claim mechanical rather than a comment. Kavita adds enum members; when either
// spec is refreshed and LibraryType grows a seventh, this test goes red and
// someone has to decide what it is, rather than the default branch quietly
// declining a library type the user can see in Kavita.
func TestEveryLibraryTypeMemberIsMapped(t *testing.T) {
	for _, file := range kavitaSpecFiles {
		t.Run(file, func(t *testing.T) { everyLibraryTypeMemberIsMapped(t, file) })
	}
}

func everyLibraryTypeMemberIsMapped(t *testing.T, file string) {
	t.Helper()
	raw := readKavitaSpec(t, file)
	var doc struct {
		Components struct {
			Schemas struct {
				LibraryType struct {
					Enum         []int64  `json:"enum"`
					EnumVarNames []string `json:"x-enum-varnames"`
				} `json:"LibraryType"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	members := doc.Components.Schemas.LibraryType.Enum
	if len(members) == 0 {
		t.Fatalf("%s declares no LibraryType members; this test would pass vacuously", file)
	}
	if len(members) != 6 {
		t.Errorf("%s declares %d LibraryType members (%v), and mapLibraryType was "+
			"written against 6. Decide what the new member is — do NOT let the default branch "+
			"decline a library type the user can see in Kavita",
			file, len(members), doc.Components.Schemas.LibraryType.EnumVarNames)
	}

	// Kind for each member, and the DECLINE for the one that has none. Written
	// out rather than derived, because a table derived from the same function it
	// tests asserts nothing.
	want := map[int64]string{
		0: "comic", // Manga
		1: "comic", // Comic (Flexible)
		2: "book",  // Book
		3: "",      // Image — declined
		4: "book",  // Light Novel
		5: "comic", // Comic (ComicVine)
	}
	for _, m := range members {
		got := mapLibraryType(kavita.LibraryType(m))
		w, known := want[m]
		if !known {
			t.Errorf("LibraryType %d is in %s and this test has no expectation for it", m, file)
			continue
		}
		if got.Kind != w {
			t.Errorf("mapLibraryType(%d) kind = %q, want %q", m, got.Kind, w)
		}
		if got.Kind == "" && got.Reason == "" {
			t.Errorf("LibraryType %d is declined with no reason; §17.8 requires the reason", m)
		}
		if got.Kind != "" && got.Reason != "" {
			t.Errorf("LibraryType %d maps to %q AND carries a decline reason", m, got.Kind)
		}
	}
}

func TestUnknownLibraryTypeIsDeclinedNotDefaulted(t *testing.T) {
	// A member added upstream after the spec was pinned. Guessing a kind here is
	// the one mistake that cannot be undone cheaply: work.kind is written once
	// at ingest and §6.4's cascade then forbids the wrongly-kinded works from
	// ever merging with the right ones.
	got := mapLibraryType(kavita.LibraryType(99))
	if got.Kind != "" {
		t.Errorf("an unknown LibraryType mapped to kind %q instead of being declined", got.Kind)
	}
	if got.Reason == "" {
		t.Error("an unknown LibraryType was declined with no reason")
	}
}

func TestReadingDirectionComesFromTheLibraryType(t *testing.T) {
	// ADR-0030 makes reading_direction the axis that carries manga-ness, and
	// SeriesDto reports none — so the library type is the only input there is.
	for _, tc := range []struct{ lt, want string }{
		{"0", "rtl"}, // Manga
		{"1", "ltr"}, // Comic
		{"5", "ltr"}, // ComicVine
		{"2", ""},    // Book: no reading direction at all
		{"4", ""},    // Light Novel
	} {
		var m kavita.LibraryType
		switch tc.lt {
		case "0":
			m = kavita.LibraryTypeManga
		case "1":
			m = kavita.LibraryTypeComic
		case "2":
			m = kavita.LibraryTypeBook
		case "4":
			m = kavita.LibraryTypeLightNovel
		case "5":
			m = kavita.LibraryTypeComicVine
		}
		if got := mapLibraryType(m).ReadingDirection; got != tc.want {
			t.Errorf("LibraryType %s reading direction = %q, want %q", tc.lt, got, tc.want)
		}
	}
}

func TestContainersAndItemsFromTheOrdinaryCassettes(t *testing.T) {
	src := NewKavitaSource(newCassetteClient(t, "kavita_libraries.yaml", "kavita_series_all_v2.yaml"))

	cs, err := src.Containers(t.Context())
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("read %d containers, want 2", len(cs))
	}
	if cs[0].RemoteID != "1" || cs[0].Name != "Manga" || cs[0].Kind != "comic" {
		t.Errorf("container 0 = %+v", cs[0])
	}
	if cs[1].RemoteID != "2" || cs[1].Name != "Ebooks" || cs[1].Kind != "book" {
		t.Errorf("container 1 = %+v", cs[1])
	}

	var items []store.CatalogueItem
	n, err := src.StreamItems(t.Context(), func(it store.CatalogueItem) error {
		items = append(items, it)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	if n != 3 || len(items) != 3 {
		t.Fatalf("streamed %d elements into %d items, want 3 and 3", n, len(items))
	}

	byID := map[string]store.CatalogueItem{}
	for _, it := range items {
		byID[it.RemoteID] = it
	}

	f := byID["41"]
	if f.Kind != "comic" {
		t.Errorf("Frieren kind = %q, want comic — the kind comes from the LIBRARY, and library 1 is Manga", f.Kind)
	}
	if f.Title != "Frieren: Beyond Journey's End" || f.SortTitle != "Frieren Beyond Journey's End" {
		t.Errorf("Frieren titles = %q / %q", f.Title, f.SortTitle)
	}
	if f.NormalizedTitle != "frieren beyond journey s end" {
		t.Errorf("Frieren normalized_title = %q", f.NormalizedTitle)
	}
	if !f.ReadingDirection.Valid || f.ReadingDirection.String != "rtl" {
		t.Errorf("Frieren reading direction = %+v, want rtl", f.ReadingDirection)
	}
	if f.PageCount.Valid {
		t.Error("a comic got a page count; work_comic is the SERIES level and has no such column")
	}
	if f.RemotePath != "/mnt/user/media/manga/Frieren" {
		t.Errorf("remote_path = %q, want the upstream's own folderPath verbatim", f.RemotePath)
	}
	if f.RemoteSubtype != "1" {
		t.Errorf("remote_subtype = %q, want MangaFormat 1 (Archive) verbatim", f.RemoteSubtype)
	}
	if !f.HasFile {
		t.Error("a series with 3812 pages reported no file")
	}
	// LastChapterAddedUtc, not LastChapterAdded: the other is the server's local
	// clock and this is a replica comparing across machines.
	if f.RemoteUpdatedAt.UTC().Format("15:04:05") != "07:00:30" {
		t.Errorf("remote_updated_at = %v, want the UTC watermark 07:00:30", f.RemoteUpdatedAt)
	}
	// The alt titles: originalName and a DIFFERENT localizedName.
	var kinds []string
	for _, a := range f.AltTitles {
		kinds = append(kinds, a.Kind+"="+a.Title)
	}
	sort.Strings(kinds)
	if len(kinds) != 2 || kinds[0] != "original=葬送のフリーレン" || kinds[1] != "translated=Frieren" {
		t.Errorf("alt titles = %v", kinds)
	}

	// DEGRADED IDENTITY IS THE ORDINARY CASE. Every id in this fixture is 0,
	// null or "", which is what a free Kavita returns.
	for _, it := range items {
		if len(it.ExternalIDs) != 0 {
			t.Errorf("%s reported external ids %+v from a free-instance fixture", it.RemoteID, it.ExternalIDs)
		}
	}

	h := byID["77"]
	if h.Kind != "book" {
		t.Errorf("The Hobbit kind = %q, want book (library 2 is LibraryType Book)", h.Kind)
	}
	if !h.PageCount.Valid || h.PageCount.Int64 != 310 {
		t.Errorf("The Hobbit page count = %+v, want 310", h.PageCount)
	}
	if h.ReadingDirection.Valid {
		t.Error("a book got a reading direction")
	}
	if len(h.AltTitles) != 0 {
		t.Errorf("The Hobbit has alt titles %+v; both of its alt fields are empty", h.AltTitles)
	}
}

func TestIdentifiedCassetteMapsEveryIDSourceAndDropsTheEditionID(t *testing.T) {
	src := NewKavitaSource(newCassetteClient(t,
		"kavita_libraries_all_types.yaml", "kavita_series_all_v2_identified.yaml"))

	cs, err := src.Containers(t.Context())
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(cs) != 6 {
		t.Fatalf("read %d containers, want 6 (one per LibraryType member)", len(cs))
	}
	declined := 0
	for _, c := range cs {
		if c.Kind == "" {
			declined++
			if c.RemoteID != "4" {
				t.Errorf("container %s was declined; only the Image library (4) should be", c.RemoteID)
			}
			if c.DeclineReason == "" {
				t.Error("a declined container carries no reason")
			}
		}
	}
	if declined != 1 {
		t.Errorf("%d containers were declined, want exactly 1", declined)
	}

	byID := map[string]store.CatalogueItem{}
	if _, err := src.StreamItems(t.Context(), func(it store.CatalogueItem) error {
		byID[it.RemoteID] = it
		return nil
	}); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}

	// A series in the DECLINED library must never become an item.
	if _, ok := byID["105"]; ok {
		t.Error("a series from the declined Image library was mapped into an item")
	}
	if len(byID) != 5 {
		t.Fatalf("mapped %d items, want 5 (six series, one in a declined library)", len(byID))
	}

	// ⚠️ THE CONFIDENCE RULE IS NOT UNIFORM, and it used to be asserted here as
	// though it were — on the premise that every one of these is "a typed
	// Kavita+ field, not free text". That premise was false for ComicVine and,
	// LS-38 measured, false for AniList, MyAnimeList and MangaBaka too: all four
	// have plain-scanner writers at v0.9.0.2 reading ComicInfo.xml's <Web>
	// element, and every one of the six has an Edit-Series-dialog writer. §6.4
	// amendment 3 caps all four below 1.0.
	//
	// THE SPLIT IS ASSERTED HERE BY TABLE rather than by prefix, so that a field
	// silently moving from one side to the other fails rather than being
	// absorbed by a startsWith. comicvine_test.go and weblinkid_test.go own the
	// two rules in full; this asserts only that the mapping applies the right one
	// to the right source.
	weakSources := map[string]bool{
		ComicVineVolumeSource: true,
		AniListSource:         true,
		MalMangaSource:        true,
		MangaBakaSource:       true,
	}
	ids := func(remoteID string) map[string]string {
		out := map[string]string{}
		for _, x := range byID[remoteID].ExternalIDs {
			switch {
			case weakSources[x.Source] && x.Confidence >= 1.0:
				t.Errorf("%s: %s=%s came out at confidence %v; Kavita parses that field out of a "+
					"free-text <Web> element, and at 1.0 it satisfies ux_extid_work_strong and MERGES WORKS",
					remoteID, x.Source, x.Value, x.Confidence)
			case !weakSources[x.Source] && x.Confidence < 1.0:
				t.Errorf("%s: %s=%s came out at confidence %v; want 1.0", remoteID, x.Source, x.Value, x.Confidence)
			}
			out[x.Source] = x.Value
		}
		return out
	}

	if got := ids("101"); got[AniListSource] != "30013" || got[MalMangaSource] != "2" || len(got) != 2 {
		t.Errorf("Berserk external ids = %v", got)
	}
	// Library 6 does not set inheritWebLinksFromFirstChapter, so the bare
	// "42563" the scanner wrote can only have come from a 4050 volume link.
	if got := ids("104"); got[ComicVineVolumeSource] != "42563" || got["cbr"] != "7" || len(got) != 2 {
		t.Errorf("Saga external ids = %v", got)
	}
	if got := ids("106"); got[MalMangaSource] != "9115" || len(got) != 1 {
		t.Errorf("Spice and Wolf external ids = %v", got)
	}
	// The bare 'mal' namespace is GONE, not merely unused. LS-10 is closed by
	// the source string, so a regression to it has to fail somewhere.
	for _, remote := range []string{"101", "106"} {
		if v, ok := ids(remote)[malBareSource]; ok {
			t.Errorf("series %s carries the BARE source %q=%s; MyAnimeList numbers anime and "+
				"manga separately (myanimelist.net/anime/1 is Cowboy Bebop, /manga/1 is Monster), "+
				"so a bare namespace lets two different works collide on (source, value)",
				remote, malBareSource, v)
		}
	}

	// mangaBakaEditionId is set on 103 and MUST NOT appear: it is an EDITION
	// identifier, and §6.4 amendment 4 is categorical that writing one as a work
	// id "silently claims a paperback and an audiobook are the same edition".
	p := ids("103")
	if p["hardcover_book"] != "445" {
		t.Errorf("Piranesi external ids = %v, want hardcover_book=445", p)
	}
	for src, v := range p {
		if src != "hardcover_book" {
			t.Errorf("Piranesi carries %s=%s; the only work-strong id in that fixture is hardcover_book", src, v)
		}
	}

	// Light Novel (LibraryType 4) lands as 'book'; ComicVine (5) as 'comic'.
	if byID["106"].Kind != "book" {
		t.Errorf("a Light Novel library series came out as %q, want book", byID["106"].Kind)
	}
	if byID["104"].Kind != "comic" {
		t.Errorf("a ComicVine library series came out as %q, want comic", byID["104"].Kind)
	}
}

func TestStreamItemsSkipsSeriesFromUnknownContainers(t *testing.T) {
	// Containers() has not been called, so no libraryId is known. Every series
	// must be skipped rather than defaulted into some kind — a work created with
	// a guessed kind is unrecoverable.
	src := NewKavitaSource(newCassetteClient(t, "kavita_series_all_v2.yaml"))
	n := 0
	read, err := src.StreamItems(t.Context(), func(store.CatalogueItem) error { n++; return nil })
	if err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	if n != 0 {
		t.Errorf("%d items were mapped with no container list; want 0", n)
	}
	if read != 3 {
		t.Errorf("the stream reported %d elements read, want 3 — the count is what the "+
			"decoder handed over, not what the callback kept", read)
	}
}
