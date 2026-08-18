package store

import (
	"strings"
	"testing"
)

// TestFTSQueriesWorkedExamples runs docs/reference/search.md §3's OWN worked
// examples, all seven of them, as the document tabulates them.
//
// They are the document's examples rather than examples of the document because
// §3 exists to be implemented literally: every user-visible search behaviour
// depends on the transformation, and the document says in as many words that
// "leaving it to the reader is how a project gets a 500 on every apostrophe".
// A test that invented its own cases would be checking this code against itself.
//
// ⚠️ ONE ROW DEPARTS FROM THE TABLE CELL AND FOLLOWS THE TABLE'S OWN NOTE. §3's
// row for `train dr` prints q_raw as *(skipped)* while the Notes cell on the
// same row says "Longest token 5 ≥ 3 → trigram runs on `"train dr"`; shown
// skipped only if every token < 3". The cell and the note contradict each other
// and the note is the rule — the floor is a property of the LONGEST token,
// because that is what SQLite's trigram tokenizer can index. REVIEW-LOG LS-190
// records the discrepancy so the next reader does not think this is a bug.
func TestFTSQueriesWorkedExamples(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       string
		prefix    string
		trgm      string
		truncated bool
	}{
		{
			name:   "single token, prefixed",
			raw:    "severance",
			prefix: `"severance"*`,
			trgm:   `"severance"`,
		},
		{
			name:   "last token prefixed, earlier ones not",
			raw:    "train dreams",
			prefix: `"train" OR "dreams"*`,
			trgm:   `"train dreams"`,
		},
		{
			// The row where the table cell and its own note disagree; see the
			// ⚠️ above. The longest token is 5, so the trigram leg RUNS.
			name:   "short final token, long earlier token — trigram still runs",
			raw:    "train dr",
			prefix: `"train" OR "dr"*`,
			trgm:   `"train dr"`,
		},
		{
			name:   "two characters — the trigram floor skips the leg",
			raw:    "it",
			prefix: `"it"*`,
			trgm:   "",
		},
		{
			name:   "a slash is inert inside the quotes",
			raw:    "AC/DC",
			prefix: `"AC/DC"*`,
			trgm:   `"AC/DC"`,
		},
		{
			// THE FLAGSHIP CASE. Unescaped, the colon is an FTS5 COLUMN FILTER
			// naming a column that does not exist, so this is a 500 on every
			// request rather than a zero-result — see
			// TestEveryConstructedQueryIsAcceptedByFTS5, which is the half of
			// this pair that can actually tell.
			name:   "a colon is quoted, so it is not a column filter",
			raw:    "Fallout: New Vegas",
			prefix: `"Fallout:" OR "New" OR "Vegas"*`,
			trgm:   `"Fallout: New Vegas"`,
		},
		{
			name:   "internal quotes are doubled",
			raw:    `say "hello"`,
			prefix: `"say" OR """hello"""*`,
			trgm:   `"say ""hello"""`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildFTSQueries(tc.raw)
			if got.prefix != tc.prefix {
				t.Errorf("q_prefix(%q) = %q, want %q", tc.raw, got.prefix, tc.prefix)
			}
			if got.raw != tc.trgm {
				t.Errorf("q_raw(%q) = %q, want %q", tc.raw, got.raw, tc.trgm)
			}
			if got.Truncated != tc.truncated {
				t.Errorf("Truncated(%q) = %v, want %v", tc.raw, got.Truncated, tc.truncated)
			}
		})
	}
}

// TestFTSQueriesEdgesTheWorkedExamplesDoNotReach covers the four states §3
// specifies in prose but does not tabulate.
func TestFTSQueriesEdgesTheWorkedExamplesDoNotReach(t *testing.T) {
	t.Run("the twelve-token cap fires and is reported", func(t *testing.T) {
		got := buildFTSQueries(strings.Repeat("alpha ", 20))
		if !got.Truncated {
			t.Error("a twenty-token query did not report Truncated; §3 requires a UI note " +
				"and a note the server never sends cannot be rendered")
		}
		if n := strings.Count(got.prefix, " OR ") + 1; n != searchMaxTokens {
			t.Errorf("q_prefix has %d disjuncts, want %d", n, searchMaxTokens)
		}
	})

	t.Run("a one-character final token is dropped from the prefix leg", func(t *testing.T) {
		got := buildFTSQueries("train d")
		if got.prefix != `"train"` {
			t.Errorf("q_prefix = %q, want %q — prefix='2 3 4' cannot serve a "+
				"one-character prefix, so the token is dropped rather than starred", got.prefix, `"train"`)
		}
	})

	t.Run("a query that is only a one-character token asks nothing at all", func(t *testing.T) {
		got := buildFTSQueries("d")
		if !got.empty() {
			t.Errorf("q = %+v, want both legs empty: the keyword leg drops a "+
				"one-character final token and the trigram floor is three", got)
		}
	})

	t.Run("unicode whitespace is whitespace", func(t *testing.T) {
		// A non-breaking space and an ideographic space, both of which arrive
		// by paste from a web page.
		got := buildFTSQueries("train dreams　now")
		if len(got.tokens) != 3 {
			t.Errorf("tokens = %q, want three: a non-breaking space is whitespace "+
				"to the person who typed it", got.tokens)
		}
	})

	t.Run("an empty query asks nothing", func(t *testing.T) {
		if got := buildFTSQueries("   \t\n  "); !got.empty() || len(got.tokens) != 0 {
			t.Errorf("q = %+v, want no tokens and no legs", got)
		}
	})
}

// TestEveryConstructedQueryIsAcceptedByFTS5 is the half of the §3 pair a table
// of expected strings CANNOT do: it executes every constructed MATCH argument
// against real fts5 tables.
//
// ⚠️ THE FAILURE THIS CATCHES IS A 500, NOT A ZERO-RESULT. FTS5's MATCH argument
// is a query language. An unescaped `:` is a column filter, an unescaped `*` is
// a prefix operator, `AND`/`OR`/`NOT`/`NEAR` are keywords and a bare `"` is an
// unterminated string — every one of which raises "fts5: syntax error near …"
// and fails the whole request. A string comparison against an expected q_prefix
// proves the builder produced the bytes somebody wrote down; only executing it
// proves those bytes parse.
//
// The inputs are §3's examples plus the operators §3 names by name.
func TestEveryConstructedQueryIsAcceptedByFTS5(t *testing.T) {
	s := newTestStore(t)

	for _, raw := range []string{
		"severance", "train dreams", "train dr", "it", "AC/DC",
		"Fallout: New Vegas", `say "hello"`, "Schindler's List",
		// The operators and metacharacters §3 lists.
		"*", ":", "-", "^", "NEAR", "AND", "OR", "NOT", `"`, `""`,
		"a AND b", "title:dune", "^anchored", "tri-hyphen word",
		"(parens)", "{braces}", "col:umn:s", "emoji 🎬 title",
		"葬送のフリーレン", "Amélie",
	} {
		t.Run(raw, func(t *testing.T) {
			q := buildFTSQueries(raw)
			if q.prefix != "" {
				var n int
				if err := s.db.Read().QueryRowContext(t.Context(),
					`SELECT COUNT(*) FROM search_fts WHERE search_fts MATCH ?`, q.prefix).Scan(&n); err != nil {
					t.Errorf("search_fts rejected q_prefix %q built from %q: %v\n"+
						"That is a 500 on this query, not a zero-result.", q.prefix, raw, err)
				}
			}
			if q.raw != "" {
				var n int
				if err := s.db.Read().QueryRowContext(t.Context(),
					`SELECT COUNT(*) FROM search_trgm WHERE search_trgm MATCH ?`, q.raw).Scan(&n); err != nil {
					t.Errorf("search_trgm rejected q_raw %q built from %q: %v", q.raw, raw, err)
				}
			}
		})
	}
}

// TestFTSQuoteDoublesInternalQuotes pins §3 step 2's escape on its own, because
// it is the one line of the transformation whose absence is invisible in the
// common case: everything without a `"` in it round-trips identically.
func TestFTSQuoteDoublesInternalQuotes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`plain`, `"plain"`},
		{`say"it`, `"say""it"`},
		{`"`, `""""`},
		{`a"b"c`, `"a""b""c"`},
	} {
		if got := ftsQuote(tc.in); got != tc.want {
			t.Errorf("ftsQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
