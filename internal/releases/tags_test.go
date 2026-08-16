package releases

import (
	"testing"

	"github.com/jdb3750/UsArr/internal/servarr"
)

func tagSet(tags []Tag) map[string]bool {
	m := make(map[string]bool, len(tags))
	for _, t := range tags {
		m[t.String()] = true
	}
	return m
}

func TestDeriveTags(t *testing.T) {
	tests := []struct {
		name string
		rel  servarr.ReleaseResource
		ix   *servarr.IndexerResource
		want []string
		not  []string
	}{
		{
			name: "torrent movie on a semi-private indexer",
			rel: servarr.ReleaseResource{
				Protocol:     servarr.ProtocolTorrent,
				Indexer:      "Fake Torrents (API)",
				IndexerFlags: []string{"freeleech", "Internal"},
				Categories:   []servarr.IndexerCategory{{ID: 2000}, {ID: 2045}},
			},
			ix:   &servarr.IndexerResource{Privacy: servarr.PrivacySemiPrivate},
			want: []string{"source:torrent", "type:movie", "indexer:fake-torrents-api", "indexer-privacy:semi-private", "flag:freeleech", "flag:internal"},
		},
		{
			// The case the schema was shaped for: 3030 is the only reliable machine
			// signal separating an audiobook from music at acquisition time, and an
			// audiobook is an EDITION of a book work, not a work kind.
			name: "audiobook is a book with an audiobook format",
			rel: servarr.ReleaseResource{
				Protocol:   servarr.ProtocolUsenet,
				Indexer:    "Fake Usenet",
				Categories: []servarr.IndexerCategory{{ID: 3000}, {ID: 3030}},
			},
			want: []string{"source:usenet", "type:book", "format:audiobook"},
			not:  []string{"type:audiobook", "type:music"},
		},
		{
			name: "comic",
			rel: servarr.ReleaseResource{
				Protocol:   servarr.ProtocolTorrent,
				Categories: []servarr.IndexerCategory{{ID: 7000}, {ID: 7030}},
			},
			want: []string{"type:comic"},
			not:  []string{"type:book"},
		},
		{
			name: "anime is tv; the raw category keeps the detail",
			rel: servarr.ReleaseResource{
				Protocol:   servarr.ProtocolTorrent,
				Categories: []servarr.IndexerCategory{{ID: 5000}, {ID: 5070}},
			},
			want: []string{"type:tv"},
		},
		{
			// Never launder an unknown protocol into torrent.
			name: "unknown protocol stays unknown",
			rel:  servarr.ReleaseResource{Protocol: "", Categories: []servarr.IndexerCategory{{ID: 2000}}},
			want: []string{"source:unknown"},
			not:  []string{"source:torrent"},
		},
		{
			name: "adult has no type in the v0.1 vocabulary",
			rel: servarr.ReleaseResource{
				Protocol:   servarr.ProtocolTorrent,
				Categories: []servarr.IndexerCategory{{ID: 6000}},
			},
			want: []string{"source:torrent"},
			not:  []string{"type:movie", "type:adult"},
		},
		{
			name: "no indexer means no privacy tag rather than a guessed one",
			rel:  servarr.ReleaseResource{Protocol: servarr.ProtocolTorrent, Indexer: "X"},
			want: []string{"source:torrent", "indexer:x"},
			not:  []string{"indexer-privacy:public"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tagSet(DeriveTags(tc.rel, tc.ix))
			for _, w := range tc.want {
				if !got[w] {
					t.Errorf("missing tag %q; got %v", w, got)
				}
			}
			for _, n := range tc.not {
				if got[n] {
					t.Errorf("unwanted tag %q; got %v", n, got)
				}
			}
		})
	}
}

func TestDeriveTagsFlagOrderIsStable(t *testing.T) {
	rel := servarr.ReleaseResource{
		Protocol:     servarr.ProtocolTorrent,
		IndexerFlags: []string{"Scene", "freeleech", "scene", "FreeLeech"},
	}
	tags := DeriveTags(rel, nil)
	var flags []string
	for _, tag := range tags {
		if tag.Namespace == "flag" {
			flags = append(flags, tag.Value)
		}
	}
	// Deduped and sorted, so the tag set of one release does not churn between
	// searches just because the indexer reordered its array.
	if len(flags) != 2 || flags[0] != "freeleech" || flags[1] != "scene" {
		t.Errorf("flags = %v, want [freeleech scene]", flags)
	}
}

func TestTagStringIsTheDisplayForm(t *testing.T) {
	// Tags are STORED decomposed into two indexed columns, never as one string.
	// String() is presentation only.
	if got := (Tag{Namespace: "source", Value: "torrent"}).String(); got != "source:torrent" {
		t.Errorf("String() = %q", got)
	}
}
