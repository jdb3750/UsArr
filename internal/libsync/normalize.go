package libsync

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// NormVersion is the algorithm version NormalizeTitle implements, written into
// work.norm_version.
//
// The column exists so that a later, better normaliser can be identified and
// backfilled row by row (ARCHITECTURE.md §6.4: "normalized_title is
// deterministic and versioned, so norm_version tells you which rows are
// stale"). Bumping this constant without writing the migration that rebuilds
// the older rows is the one way to make it lie.
const NormVersion int64 = 1

// NormalizeTitle implements the v1 normaliser ARCHITECTURE.md §6.4 specifies,
// in its stated order: casefold, NFKD, strip combining marks, strip
// punctuation, collapse whitespace.
//
// EVERY STEP IS THE ONE NAMED, not an approximation of it, and that matters
// because the column is called normalized_title and norm_version claims which
// algorithm produced it:
//
//   - CASEFOLD, not strings.ToLower. They differ: Unicode case folding maps
//     "ß" to "ss" and "ẞ" to "ss", while ToLower leaves both as "ß". A German
//     title indexed under one and searched under the other never matches.
//   - NFKD, not NFD. The compatibility decomposition is what folds "ﬁ" to "fi",
//     "①" to "1" and full-width Latin to ASCII — all of which occur in
//     scene-named comic and manga titles.
//   - STRIP COMBINING MARKS after NFKD, which is what turns "é" (now e + U+0301)
//     into "e". Doing it before NFKD would strip nothing from a precomposed
//     string.
//
// ⚠️ KNOWN LIMIT, carried openly because §6.4 names it: this is locale-blind.
// Turkish dotless ı folds to "i" here, which is wrong for Turkish and right for
// everything else. The v2 normaliser in the schema reference is where that is
// addressed; v1 is deliberately simple and deterministic.
//
// The dependency this needs — golang.org/x/text — is BSD-3-Clause, which is
// AGPL-compatible (CLAUDE.md's dependency rule).
func NormalizeTitle(s string) string {
	folded := cases.Fold().String(s)
	decomposed := norm.NFKD.String(folded)

	var b strings.Builder
	b.Grow(len(decomposed))
	pendingSpace := false
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
			// A combining mark. Dropped, not replaced by a space: "e" + U+0301
			// is one letter and must stay one.
		case unicode.IsSpace(r):
			pendingSpace = b.Len() > 0
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			// Punctuation becomes a SEPARATOR rather than nothing. Deleting it
			// outright turns "Spy x Family" and "Spyx Family" into the same
			// string only by accident, and turns "Re:Zero" into "rezero", which
			// no user types. A collapsed space is the honest reading of a
			// boundary character.
			pendingSpace = b.Len() > 0
		default:
			if pendingSpace {
				b.WriteByte(' ')
				pendingSpace = false
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
