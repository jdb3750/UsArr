package store

import (
	"strings"
	"unicode"
)

// Query construction for the library search — docs/reference/search.md §3, the
// exact transformation, implemented as the document writes it.
//
// It is its own file, and every step of it is a decision the document argues
// for rather than an implementation detail:
//
//	raw query  →  tokens  →  q_prefix (search_fts)  and  q_raw (search_trgm)
//
// ⚠️ THE STAKES ARE A 500, NOT A ZERO-RESULT. FTS5's MATCH argument is a query
// LANGUAGE, not a string. `:`, `*`, `^`, `-`, `"`, `NEAR`, `AND` and `OR` are
// all syntax in it, so an unescaped user query does not "match nothing" — it
// raises "fts5: syntax error near ..." and the endpoint answers 500. The
// flagship case is `Fallout: New Vegas`, where the colon is read as a column
// filter naming a column that does not exist. TestFTSQueriesWorkedExamples runs
// §3's own seven examples, and TestEveryConstructedQueryIsAcceptedByFTS5
// executes each one against a real fts5 table, because a table of expected
// strings cannot tell you the strings parse.

// searchMaxTokens is §3 step 1's cap. A longer query is pathological — a pasted
// paragraph, not a title — and is truncated rather than refused, with the caller
// told so it can say what happened. The cap also bounds the OR-expansion: the
// keyword leg is one disjunct per token, and the candidate set §4 fuses is
// bounded by LIMIT 200 per leg regardless.
const searchMaxTokens = 12

// trigramMinChars is §3 step 5's floor. SQLite's trigram tokenizer indexes
// nothing shorter than three characters, so a query whose LONGEST token is
// under three cannot match anything on that leg — it is skipped entirely and
// the RRF degenerates to the single keyword leg, which is a correct answer
// rather than a degraded one.
const trigramMinChars = 3

// ftsQuote wraps one token as an FTS5 string literal: §3 step 2, verbatim.
//
// Doubling the internal double quote is the whole escape — FTS5 string literals
// have no backslash. `say "hello"` becomes `"say"` and `"""hello"""`, which is
// the document's own last worked example and the one that catches an
// implementation that merely wrapped without doubling.
func ftsQuote(tok string) string {
	return `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
}

// ftsQueries is the pair §3 produces, plus the two facts a caller needs about
// how the raw text was treated.
type ftsQueries struct {
	// prefix is the search_fts MATCH argument. Empty means the keyword leg does
	// not run — reachable only when the whole query was a single one-character
	// token, which step 4 drops.
	prefix string

	// raw is the search_trgm MATCH argument. Empty means the trigram leg does
	// not run, per step 5's floor.
	raw string

	// tokens is what step 1 produced, AFTER the cap. It is carried because the
	// re-rank compares the typed text against norm_title and wants the same
	// tokens the retrieval used, not a second split of the raw string.
	tokens []string

	// Truncated says step 1's cap fired. §3: "a longer query is pathological and
	// is truncated with a UI note" — so the note has to be reachable, which
	// means this travels to the wire.
	Truncated bool
}

// empty reports that neither leg can run, so there is nothing to ask the
// database. It is a legitimate state, not an error: `a` is a one-character
// query, step 4 drops it from the keyword leg and step 5 keeps it off the
// trigram leg, and the honest answer is no results rather than a 400 telling
// the user their letter was invalid.
func (q ftsQueries) empty() bool { return q.prefix == "" && q.raw == "" }

// buildFTSQueries performs §3's five steps on one raw user query.
//
// STEP BY STEP, with the reason each is what it is:
//
//  1. Tokenise on Unicode whitespace, drop empties, cap at searchMaxTokens.
//     unicode.IsSpace rather than a byte comparison: a non-breaking space or an
//     ideographic space pasted from a web page is whitespace to the person who
//     typed it and must be to us.
//
//  2. ftsQuote every token. See the ⚠️ at the top of this file.
//
//  3. The operator is OR, not FTS5's default AND. This decides whether the
//     second tier works at all: under AND, a two-token query with one token
//     the title does not contain returns NOTHING, and a re-rank with an empty
//     candidate set has nothing to reorder. Under OR the legs retrieve
//     anything containing any token and the re-rank does the discrimination.
//     The cost is a larger candidate set, bounded by LIMIT 200 per leg.
//
//  4. Prefix the LAST token only. `train dr` → `"train" OR "dr"*`. Prefixing
//     every token inflates the candidate set for nothing — the user has
//     finished typing the earlier ones. A one-character final token is dropped
//     from this leg: `prefix='2 3 4'` indexes 2-, 3- and 4-character prefixes,
//     so a one-character prefix is the one case the index cannot serve and
//     would be a walk of the whole term list.
//
//  5. The trigram leg is skipped when EVERY token is shorter than three
//     characters.
//
// ⚠️ STEP 5'S RULE IS THE ONE §3 STATES TWICE AND INCONSISTENTLY, and this
// implements the version §3 itself corrects to. The worked-example table's row
// for `train dr` shows q_raw as *(skipped)*, and the Notes cell on that same row
// says why that is wrong: *"Longest token 5 ≥ 3 → trigram runs on `"train dr"`;
// shown skipped only if every token < 3"*. The rule is therefore "skip when the
// LONGEST token is under the floor", not "skip when the LAST token is". The
// table cell is superseded by its own note; REVIEW-LOG LS-190 records it.
func buildFTSQueries(raw string) ftsQueries {
	var q ftsQueries

	// Step 1.
	fields := strings.FieldsFunc(raw, unicode.IsSpace)
	if len(fields) > searchMaxTokens {
		fields = fields[:searchMaxTokens]
		q.Truncated = true
	}
	q.tokens = fields
	if len(fields) == 0 {
		return q
	}

	// Steps 2-4. The last token is handled apart from the rest because it is
	// the only one that carries a `*`, and because it is the only one that can
	// be dropped.
	last := len(fields) - 1
	parts := make([]string, 0, len(fields))
	for i, tok := range fields {
		if i == last {
			// A one-character final token is dropped from this leg entirely
			// rather than merely left unprefixed. Keeping it as an exact term
			// (`"d"`) would retrieve only documents containing the whole
			// one-letter word "d", which is noise dressed as a result; the
			// user is mid-word, and the honest answer to a letter is the
			// earlier tokens' answer.
			if len([]rune(tok)) < 2 {
				break
			}
			parts = append(parts, ftsQuote(tok)+"*")
			break
		}
		parts = append(parts, ftsQuote(tok))
	}
	q.prefix = strings.Join(parts, " OR ")

	// Step 5. The trigram argument is the whole token sequence quoted ONCE, not
	// per token: the trigram index matches substrings, so the phrase is the
	// query, and re-joining on a single space normalises whatever run of
	// whitespace the user typed.
	longest := 0
	for _, tok := range fields {
		if n := len([]rune(tok)); n > longest {
			longest = n
		}
	}
	if longest >= trigramMinChars {
		q.raw = ftsQuote(strings.Join(fields, " "))
	}
	return q
}
