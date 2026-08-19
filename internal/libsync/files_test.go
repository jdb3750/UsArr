package libsync

import (
	"context"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// fakeVolumeClient answers SeriesVolumes from a map and can be told to fail. It
// is fakeCreditClient's twin and is a separate type only because it answers a
// different method.
type fakeVolumeClient struct {
	KavitaReader
	vols  map[int32][]kavita.VolumeDto
	fail  map[int32]error
	calls []int32
}

func (f *fakeVolumeClient) SeriesVolumes(_ context.Context, id int32) ([]kavita.VolumeDto, error) {
	f.calls = append(f.calls, id)
	if err, ok := f.fail[id]; ok {
		return nil, err
	}
	return f.vols[id], nil
}

// oneFileVolume builds the smallest realistic walk: one volume, one chapter,
// one file — with a REAL-LOOKING filePath, because a fixture whose filePath is
// empty cannot see the surrogate rule break.
func oneFileVolume(fileID int32, ext string) []kavita.VolumeDto {
	return []kavita.VolumeDto{{
		ID: 1, SeriesID: 41, Chapters: []kavita.ChapterDto{{
			ID: 10, Files: []kavita.MangaFileDto{{
				ID:        fileID,
				FilePath:  "/mnt/user/media/manga/Frieren/Vol. 1" + ext,
				Bytes:     62914560,
				Extension: ext,
				Format:    kavita.MangaFormatArchive,
			}},
		}},
	}}
}

// TestTheSurrogateNeverCarriesTheFilePath is the adapter half of the owner's
// 2026-08-18 decision. internal/store's validateReplicaPath is the enforcement;
// this is the assertion that the value the adapter MINTS is derived from the id
// and from nothing else.
func TestTheSurrogateNeverCarriesTheFilePath(t *testing.T) {
	set := fileSetFromVolumes(
		FileRequest{RemoteKind: "series", RemoteID: "41", RemoteSubtype: "1"},
		oneFileVolume(1187, ".cbz"))

	if len(set.Files) != 1 {
		t.Fatalf("walked %d files, want 1", len(set.Files))
	}
	f := set.Files[0]
	if f.Path != "kavita:mangafile:1187" {
		t.Errorf("media_file.path = %q, want kavita:mangafile:1187", f.Path)
	}
	if strings.ContainsAny(f.Path, `/\`) {
		t.Errorf("media_file.path = %q carries a path separator", f.Path)
	}
	if strings.Contains(f.Path, "mnt") || strings.Contains(f.Path, "Frieren") {
		t.Errorf("media_file.path = %q carries part of the HOST FILESYSTEM PATH; the column "+
			"stores an opaque surrogate and the real path never enters the replica", f.Path)
	}
	if f.RemoteFileID != "1187" {
		t.Errorf("remote_file_id = %q, want the raw upstream id 1187", f.RemoteFileID)
	}
	if f.SizeBytes != 62914560 {
		t.Errorf("size_bytes = %d, want the file's own bytes", f.SizeBytes)
	}
}

// TestTheArchiveFormatIsDecidedByExtension fires the one mapping decision the
// enum cannot make on its own: MangaFormat.Archive covers BOTH 'cbz' and 'cbr'.
func TestTheArchiveFormatIsDecidedByExtension(t *testing.T) {
	cases := []struct {
		name    string
		subtype string
		exts    []string
		want    string // "" means SQL NULL
	}{
		{"a zip archive is cbz", "1", []string{".cbz", ".cbz"}, "cbz"},
		{"a bare .zip is still cbz", "1", []string{".zip"}, "cbz"},
		{"a rar archive is cbr", "1", []string{".cbr"}, "cbr"},
		{"a bare .rar is still cbr", "1", []string{".rar"}, "cbr"},
		{"case does not matter", "1", []string{".CBZ"}, "cbz"},
		// The honest-unknown cases. A mixed series does NOT have an archive
		// format, and picking the first would store a coin flip as a fact.
		{"a mixed series is undecided", "1", []string{".cbz", ".cbr"}, ""},
		{"an unrecognised extension is undecided", "1", []string{".7z"}, ""},
		{"a missing extension is undecided", "1", []string{""}, ""},
		{"an archive with no files at all is undecided", "1", nil, ""},
		// The unambiguous members, which need no extension.
		{"epub is ebook", "3", []string{".epub"}, "ebook"},
		{"pdf is pdf", "4", []string{".pdf"}, "pdf"},
		// The two with no member in edition.format's vocabulary.
		{"loose images have no format member", "0", []string{".jpg"}, ""},
		{"Unknown has no format member", "2", []string{".cbz"}, ""},
		{"a subtype this adapter did not write is not a format claim", "", []string{".cbz"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := kavitaEditionFormat(tc.subtype, tc.exts)
			if tc.want == "" {
				if got.Valid {
					t.Errorf("edition.format = %q, want NULL", got.String)
				}
				return
			}
			if !got.Valid || got.String != tc.want {
				t.Errorf("edition.format = %v, want %q", got, tc.want)
			}
		})
	}
}

// TestOneFileUnderTwoChaptersIsOneRow pins the deduplication. Kavita can report
// one MangaFile under two chapters of a split volume; two rows keyed on one
// surrogate would make the reconciliation's "seen this pass" set disagree with
// the rows actually written.
func TestOneFileUnderTwoChaptersIsOneRow(t *testing.T) {
	f := kavita.MangaFileDto{ID: 7, Bytes: 10, Extension: ".cbz"}
	vols := []kavita.VolumeDto{{
		ID: 1, Chapters: []kavita.ChapterDto{
			{ID: 10, Files: []kavita.MangaFileDto{f}},
			{ID: 11, Files: []kavita.MangaFileDto{f}},
		},
	}}
	set := fileSetFromVolumes(FileRequest{RemoteID: "41", RemoteSubtype: "1"}, vols)
	if len(set.Files) != 1 {
		t.Errorf("one file under two chapters produced %d rows, want 1", len(set.Files))
	}
}

// TestAFileWithNoIDIsSkipped covers .NET's zero value reaching the id: a row
// keyed on `kavita:mangafile:0` would collide with every other unidentified
// file in the instance.
func TestAFileWithNoIDIsSkipped(t *testing.T) {
	vols := []kavita.VolumeDto{{ID: 1, Chapters: []kavita.ChapterDto{{
		ID: 10, Files: []kavita.MangaFileDto{{ID: 0, Bytes: 10, Extension: ".cbz"}},
	}}}}
	set := fileSetFromVolumes(FileRequest{RemoteID: "41", RemoteSubtype: "1"}, vols)
	if len(set.Files) != 0 {
		t.Errorf("a file with id 0 produced %d rows, want 0", len(set.Files))
	}
}

// TestStreamFilesDropsAPerSeriesFailure is StreamCredits's rule with a sharper
// edge: a failed walk must NOT reach the store as "this series has no files",
// because the store reconciles by deleting what the pass did not report.
func TestStreamFilesDropsAPerSeriesFailure(t *testing.T) {
	c := &fakeVolumeClient{
		vols: map[int32][]kavita.VolumeDto{
			1: oneFileVolume(101, ".cbz"),
			3: oneFileVolume(103, ".cbz"),
		},
		fail: map[int32]error{2: kavita.ErrNotFound},
	}
	src := NewKavitaSource(c)

	var got []store.FileSet
	n, failed, err := src.StreamFiles(t.Context(), []FileRequest{
		{RemoteKind: "series", RemoteID: "1", RemoteSubtype: "1"},
		{RemoteKind: "series", RemoteID: "2", RemoteSubtype: "1"},
		{RemoteKind: "series", RemoteID: "3", RemoteSubtype: "1"},
	}, func(s store.FileSet) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("one 404 failed the whole walk: %v", err)
	}
	if n != 2 || len(got) != 2 {
		t.Fatalf("delivered %d sets (n=%d), want 2", len(got), n)
	}
	if got[0].RemoteID != "1" || got[1].RemoteID != "3" {
		t.Errorf("delivered %s and %s, want 1 and 3", got[0].RemoteID, got[1].RemoteID)
	}
	// The failed series delivered NOTHING — not an empty set, which would
	// delete its rows.
	for _, s := range got {
		if s.RemoteID == "2" {
			t.Error("the failed series was delivered as an empty set, which would delete its files")
		}
	}

	// ⚠️ AND IT IS REPORTED. This test previously stopped at the line above, so
	// it asserted the drop and said nothing about the record — which is how the
	// bare `continue` stayed green while the failure left no counter, no
	// sync_report row and no log line at all.
	if len(failed) != 1 {
		t.Fatalf("the walk reported %d failures, want 1", len(failed))
	}
	if failed[0].RemoteID != "2" || failed[0].RemoteKind != "series" {
		t.Errorf("reported failure %+v, want the series 2 request", failed[0])
	}
	if failed[0].Reason != "not_found" {
		t.Errorf("reported reason %q, want %q", failed[0].Reason, "not_found")
	}
}

// TestStreamFilesStopsOnAnOpenBreaker is the condition under which continuing
// would issue hundreds of calls that cannot succeed.
func TestStreamFilesStopsOnAnOpenBreaker(t *testing.T) {
	c := &fakeVolumeClient{
		vols: map[int32][]kavita.VolumeDto{1: oneFileVolume(101, ".cbz")},
		fail: map[int32]error{2: kavita.ErrBreakerOpen},
	}
	src := NewKavitaSource(c)

	var got int
	n, _, err := src.StreamFiles(t.Context(), []FileRequest{
		{RemoteKind: "series", RemoteID: "1", RemoteSubtype: "1"},
		{RemoteKind: "series", RemoteID: "2", RemoteSubtype: "1"},
		{RemoteKind: "series", RemoteID: "3", RemoteSubtype: "1"},
	}, func(store.FileSet) error { got++; return nil })
	if err == nil {
		t.Fatal("an open circuit breaker did not stop the file walk")
	}
	if n != 1 || got != 1 {
		t.Errorf("delivered %d sets before stopping, want 1", got)
	}
	if len(c.calls) != 2 {
		t.Errorf("made %d calls, want 2 (the walk stops at the first one that reports the "+
			"breaker)", len(c.calls))
	}
}

// TestStreamFilesDeliversAnEmptySet is the mirror of the drop rule: a series
// that really has no files is a STATEMENT, and it is what removes the rows of a
// series whose chapters were deleted upstream.
func TestStreamFilesDeliversAnEmptySet(t *testing.T) {
	c := &fakeVolumeClient{vols: map[int32][]kavita.VolumeDto{1: {}}}
	src := NewKavitaSource(c)

	var got []store.FileSet
	n, _, err := src.StreamFiles(t.Context(), []FileRequest{
		{RemoteKind: "series", RemoteID: "1", RemoteSubtype: "1"},
	}, func(s store.FileSet) error { got = append(got, s); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(got) != 1 {
		t.Fatalf("a series with no files delivered %d sets, want 1", len(got))
	}
	if len(got[0].Files) != 0 {
		t.Errorf("the empty set carries %d files", len(got[0].Files))
	}
}
