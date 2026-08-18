package libsync

import (
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// TestSearchNormaliserAgreesWithTheWriter is the guard on a FORCED duplication.
//
// internal/store's NormalizeForSearch is a second copy of NormalizeTitle. It has
// to be: internal/libsync imports internal/store, so internal/store cannot
// import back (credits.go records the same constraint for the same reason). The
// duplication is invisible from either side — both compile, both look right, and
// they can drift a step at a time.
//
// THE FAILURE IT CATCHES IS SILENT AND TOTAL. `norm_title` is written by
// NormalizeTitle; the library search compares a typed query against that column
// through NormalizeForSearch. If the two stop agreeing, every string-similarity
// score in the re-rank is computed between two differently-normalised strings —
// "Amélie" typed against "amelie" stored — so the ranking degrades everywhere at
// once and no test of either function alone can see it.
//
// THIS TEST IS THE ONLY PLACE THE TWO CAN MEET. Only this package can import
// both, which is why NormalizeForSearch is exported at all, and why the
// assertion lives here rather than beside either implementation.
//
// The corpus below is every case internal/libsync/normalize.go argues about by
// name — casefold vs ToLower, NFKD vs NFD, combining marks, punctuation as a
// separator — plus the titles this project's own adapter actually writes.
func TestSearchNormaliserAgreesWithTheWriter(t *testing.T) {
	for _, in := range []string{
		"",
		"Frieren: Beyond Journey's End",
		"Amélie",
		"Fallout: New Vegas",
		"AC/DC",
		"Re:Zero",
		"Spy x Family",
		"葬送のフリーレン",
		// Casefold vs ToLower: ß and ẞ both fold to "ss" and neither lowercases
		// to it.
		"Straße",
		"STRAẞE",
		// NFKD vs NFD: the ligature, the circled digit and full-width Latin.
		"ﬁnal",
		"① One",
		"ＦＵＬＬＷＩＤＴＨ",
		// Combining marks arriving already decomposed.
		"e\u0301clair",
		// Whitespace runs and leading/trailing space.
		"  The   Hobbit  ",
		"tab\tseparated",
		// Symbols, which normalize.go treats as separators alongside punctuation.
		"50% Off",
		"C++",
		"emoji 🎬 title",
		// Turkish dotless ı — normalize.go names this as its known limit, so the
		// two copies must at least be wrong in the same way.
		"Işık",
	} {
		want := NormalizeTitle(in)
		if got := store.NormalizeForSearch(in); got != want {
			t.Errorf("the two normalisers disagree on %q:\n"+
				"  libsync.NormalizeTitle    = %q\n"+
				"  store.NormalizeForSearch  = %q\n"+
				"norm_title is written by the first and searched by the second, "+
				"so every string-similarity score in the re-rank is now computed "+
				"between two differently-normalised strings.", in, want, got)
		}
	}
}
