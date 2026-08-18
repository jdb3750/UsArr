package store

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// The string-similarity half of the re-rank: search.md §4's primary text
// signal, and the normalisation that makes the two sides of it comparable.

// NormalizeForSearch applies ARCHITECTURE.md §6.4's v1 normaliser to a QUERY, so
// that it can be compared against the `norm_title` column, which holds the same
// function's output over the work's title.
//
// ⚠️ IT IS A SECOND COPY OF internal/libsync's NormalizeTitle, AND THE
// DUPLICATION IS FORCED. internal/libsync imports internal/store, so this
// package cannot import it back; credits.go records the same constraint for the
// same reason ("internal/store cannot import internal/libsync (that package
// imports this one)"). Comparing a raw query against a normalised column
// silently fails on every accented, cased or punctuated title — "Amélie" is
// stored as "amelie", so an un-normalised "Amélie" scores a Jaro-Winkler
// distance against a string it is supposed to equal.
//
// It is EXPORTED for one reason and it is stated rather than left to look
// arbitrary: only internal/libsync can import both halves, so only a test in
// that package can prove the two implementations still agree.
// TestSearchNormaliserAgreesWithTheWriter is that test, and it is the guard
// against the drift this duplication makes possible.
//
// The algorithm, in §6.4's stated order: casefold, NFKD, strip combining marks,
// turn punctuation and symbols into separators, collapse whitespace. See
// internal/libsync/normalize.go for the argument behind each step — casefold
// rather than ToLower, NFKD rather than NFD, and punctuation as a separator
// rather than as nothing.
func NormalizeForSearch(s string) string {
	folded := cases.Fold().String(s)
	decomposed := norm.NFKD.String(folded)

	var b strings.Builder
	b.Grow(len(decomposed))
	pendingSpace := false
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsSpace(r):
			pendingSpace = b.Len() > 0
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
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

// normalizeForCompare is NormalizeForSearch under the name the re-rank uses.
func normalizeForCompare(s string) string { return NormalizeForSearch(s) }

// jaroWinklerPrefixScale is the standard p = 0.1, and jaroWinklerMaxPrefix the
// standard cap of 4 shared leading characters. Together they bound the boost at
// 0.4, which is the constraint that keeps the result inside [0,1].
const (
	jaroWinklerPrefixScale = 0.1
	jaroWinklerMaxPrefix   = 4
)

// jaroWinklerThreshold is the standard boost floor: the prefix bonus applies
// only to pairs that are already reasonably similar. Without it, "a…" and
// "a…" strings that share nothing but a first letter get lifted toward each
// other, which is the opposite of what a prefix weight is for.
const jaroWinklerThreshold = 0.7

// jaroWinkler returns the Jaro-Winkler similarity of two strings in [0,1], 1
// being identical.
//
// WHY THIS METRIC AND NOT LEVENSHTEIN. §4's reason, which is about people rather
// than about algorithms: it is prefix-weighted, and that matches how people type
// titles — they get the beginning right and trail off. "the lord of the r" and
// "the lord of the rings" are far apart in edit distance relative to their
// length and very close here.
//
// It operates on RUNES, not bytes. A byte-wise implementation gives different
// answers for the same text depending on encoding, and the corpus is full of
// non-ASCII titles even after normalisation (Japanese titles survive NFKD
// intact).
//
// NO DEPENDENCY. It is thirty lines of arithmetic with a published definition
// and no edge cases worth outsourcing, and CLAUDE.md's rule is that a new
// dependency needs an AGPL-compatibility check it does not need to pass here.
func jaroWinkler(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	j := jaro(ra, rb)
	if j < jaroWinklerThreshold {
		return j
	}
	prefix := 0
	for prefix < len(ra) && prefix < len(rb) && prefix < jaroWinklerMaxPrefix {
		if ra[prefix] != rb[prefix] {
			break
		}
		prefix++
	}
	return j + float64(prefix)*jaroWinklerPrefixScale*(1-j)
}

// jaro is the Jaro similarity: the base Jaro-Winkler boosts.
//
// Two characters match when they are equal and no further apart than
// max(len(a), len(b))/2 - 1 positions; transpositions are matched pairs that
// occur in a different order in the two strings.
func jaro(a, b []rune) float64 {
	if len(a) == 0 && len(b) == 0 {
		// Two empty strings are identical. Returning 0 here would score an
		// empty query as maximally unlike an empty title, which is both wrong
		// and unreachable — SearchLibrary refuses an empty query before this.
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	window := max(len(a), len(b))/2 - 1
	if window < 0 {
		window = 0
	}

	matchedA := make([]bool, len(a))
	matchedB := make([]bool, len(b))
	matches := 0
	for i := range a {
		lo := max(0, i-window)
		hi := min(len(b)-1, i+window)
		for k := lo; k <= hi; k++ {
			if matchedB[k] || a[i] != b[k] {
				continue
			}
			matchedA[i], matchedB[k] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}

	// Transpositions: walk the matched characters of both strings in order and
	// count the positions where they disagree. The count is halved because a
	// single swap shows up on both sides.
	transpositions := 0
	k := 0
	for i := range a {
		if !matchedA[i] {
			continue
		}
		for !matchedB[k] {
			k++
		}
		if a[i] != b[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	return (m/float64(len(a)) + m/float64(len(b)) + (m-float64(transpositions)/2)/m) / 3
}
