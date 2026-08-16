package mapping

import (
	"testing"

	"github.com/jdb3750/UsArr/internal/servarr"
)

func TestParentCategory(t *testing.T) {
	for _, tc := range []struct{ in, want int32 }{
		{2000, 2000},
		{2045, 2000},
		{5070, 5000},
		{3030, 3000},
		{7020, 7000},
		{100001, 100000},
		{0, 0},
	} {
		if got := ParentCategory(tc.in); got != tc.want {
			t.Errorf("ParentCategory(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestMediaType(t *testing.T) {
	tests := []struct {
		name       string
		cats       []int32
		typ, forma string
		ok         bool
	}{
		{name: "movie", cats: []int32{2000, 2045}, typ: "movie", ok: true},
		{name: "tv", cats: []int32{5000, 5040}, typ: "tv", ok: true},
		{
			// anime is not in the type: vocabulary; the signal survives in the raw
			// category array, which is stored intact.
			name: "anime is tv", cats: []int32{5000, 5070}, typ: "tv", ok: true,
		},
		{name: "music", cats: []int32{3000, 3010}, typ: "music", ok: true},
		{
			// THE case the schema was shaped for. type:audiobook does not exist —
			// an audiobook is an edition of a book work.
			name: "3030 is a book with an audiobook format",
			cats: []int32{3000, 3030}, typ: "book", forma: "audiobook", ok: true,
		},
		{name: "ebook", cats: []int32{7000, 7020}, typ: "book", forma: "ebook", ok: true},
		{name: "comic", cats: []int32{7000, 7030}, typ: "comic", ok: true},
		{name: "magazine", cats: []int32{7010}, typ: "book", ok: true},
		{name: "book fallback", cats: []int32{7000}, typ: "book", ok: true},
		{name: "console game", cats: []int32{1000, 1030}, typ: "game", ok: true},
		{name: "pc software", cats: []int32{4000}, typ: "game", ok: true},
		{
			// Recognised but outside the v0.1 type: vocabulary. Laundering it into
			// type:movie would be worse than emitting nothing.
			name: "adult has no type", cats: []int32{6000, 6010}, ok: false,
		},
		{name: "unknown", cats: []int32{8000}, ok: false},
		{name: "empty", cats: nil, ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typ, forma, ok := MediaType(tc.cats)
			if ok != tc.ok || typ != tc.typ || forma != tc.forma {
				t.Errorf("MediaType(%v) = (%q, %q, %v), want (%q, %q, %v)",
					tc.cats, typ, forma, ok, tc.typ, tc.forma, tc.ok)
			}
		})
	}
}

func TestPrivacyValueUsesTheTagVocabulary(t *testing.T) {
	// The wire form is camelCase; the tag vocabulary is kebab-case.
	if got := PrivacyValue(servarr.PrivacySemiPrivate); got != "semi-private" {
		t.Errorf("PrivacyValue(semiPrivate) = %q, want semi-private", got)
	}
	if got := PrivacyValue(servarr.IndexerPrivacy("nonsense")); got != "" {
		t.Errorf("unknown privacy = %q, want empty", got)
	}
}

func TestSourceValueNeverLaundersUnknown(t *testing.T) {
	// schema.md §6: do not launder `unknown` into `torrent`.
	for _, in := range []servarr.DownloadProtocol{"", "irc", "nonsense"} {
		if got := SourceValue(in); got != "unknown" {
			t.Errorf("SourceValue(%q) = %q, want unknown", in, got)
		}
	}
	if got := SourceValue(servarr.ProtocolTorrent); got != "torrent" {
		t.Errorf("SourceValue(torrent) = %q", got)
	}
}

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Fake Torrents (API)", "fake-torrents-api"},
		{"  AnimeBytes  ", "animebytes"},
		{"freeleech", "freeleech"},
		{"HD-Torrents", "hd-torrents"},
		{"", ""},
		{"!!!", ""},
	} {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFromProwlarrReleaseHasNoCredentialFields(t *testing.T) {
	dl := "http://prowlarr.test:9696/1/download?apikey=SECRET&link=x"
	hash := "abc"
	rel := servarr.ReleaseResource{
		GUID: "g", Title: "T", IndexerID: 1, Indexer: "Fake",
		Protocol:    servarr.ProtocolTorrent,
		Size:        100,
		DownloadURL: &dl,
		MagnetURL:   &dl,
		InfoHash:    &hash,
		PosterURL:   "http://169.254.169.254/latest/meta-data/",
		Categories:  []servarr.IndexerCategory{{ID: 2000, SubCategories: []servarr.IndexerCategory{{ID: 2045}}}},
		AgeHours:    48,
	}
	ix := servarr.IndexerResource{ID: 1, Name: "Fake", Priority: 10, Privacy: servarr.PrivacySemiPrivate}

	got := FromProwlarrRelease(rel, &ix)

	if got.InfoHash != "abc" {
		t.Errorf("infoHash = %q", got.InfoHash)
	}
	if got.IndexerPriority != 10 || got.IndexerPrivacy != "semi-private" {
		t.Errorf("indexer fields = %d/%q", got.IndexerPriority, got.IndexerPrivacy)
	}
	if got.AgeDays != 2 {
		t.Errorf("ageDays = %v, want 2 (derived from the more precise ageHours)", got.AgeDays)
	}
	// Raw categories, flattened but never collapsed.
	if len(got.Categories) != 2 || got.Categories[0] != 2000 || got.Categories[1] != 2045 {
		t.Errorf("categories = %v", got.Categories)
	}
	// posterUrl is preserved, because it is real data — but it is SSRF class
	// `derived` and this fixture is exactly why.
	if got.PosterURL == "" {
		t.Error("posterUrl should be preserved for the fetcher to policy-check")
	}
	// The unified type has nowhere to put a credential-bearing URL. This is a
	// compile-time property; the test documents it so a "helpful" addition of a
	// DownloadURL field has to delete an assertion first.
	if _, hasField := any(got).(interface{ GetDownloadURL() string }); hasField {
		t.Error("Release grew a download URL accessor")
	}
}

func TestBlockedUntilEmptyMeansHealthy(t *testing.T) {
	m := BlockedUntil(nil)
	if len(m) != 0 {
		t.Fatalf("empty indexerstatus should yield an empty map, got %v", m)
	}
	if _, blocked := m[1]; blocked {
		t.Error("an absent key must mean healthy")
	}
}
