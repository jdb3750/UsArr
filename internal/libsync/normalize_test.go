package libsync

import "testing"

func TestNormalizeTitle(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"plain", "Frieren", "frieren"},
		{"case and space", "  The   HOBBIT ", "the hobbit"},
		{
			// The step that needs NFKD: "é" arrives precomposed and must come
			// out as a bare "e".
			"precomposed diacritic", "Café Society", "cafe society",
		},
		{
			// The step that needs CASEFOLD rather than ToLower. strings.ToLower
			// leaves "ß" alone; Unicode case folding maps it to "ss", and a
			// German title indexed one way and searched the other never matches.
			"eszett folds to ss", "Straße", "strasse",
		},
		{
			// The step that needs NFKD rather than NFD: the ligature and the
			// circled digit are COMPATIBILITY decompositions, invisible to NFD.
			"compatibility forms", "ﬁrst ① edition", "first 1 edition",
		},
		{
			// Punctuation becomes a separator, not nothing.
			"punctuation separates", "Re:Zero", "re zero",
		},
		{"apostrophe", "Frieren: Beyond Journey's End", "frieren beyond journey s end"},
		{"collapse and trim", "A  --  B", "a b"},
		{"empty", "", ""},
		{"punctuation only", "!!!", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeTitle(tc.in); got != tc.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeTitleIsDeterministic(t *testing.T) {
	// work.norm_version claims which algorithm produced a row. That claim is
	// only worth anything if the algorithm is a function.
	const in = "Frieren: Beyond Journey's End (葬送のフリーレン)"
	first := NormalizeTitle(in)
	for i := 0; i < 100; i++ {
		if got := NormalizeTitle(in); got != first {
			t.Fatalf("run %d produced %q, first run produced %q", i, got, first)
		}
	}
	// Idempotence: normalising an already-normalised title must be a no-op, or
	// a re-import silently rewrites every row.
	if again := NormalizeTitle(first); again != first {
		t.Errorf("NormalizeTitle is not idempotent: %q -> %q", first, again)
	}
}

func TestNormalizeTitleTurkishLimitIsRealAndDocumented(t *testing.T) {
	// ARCHITECTURE.md §6.4 names the Turkish dotless ı as a known source of
	// locale-dependent bugs in this normaliser. This test does not assert the
	// bug away — it PINS the behaviour so that a future v2 normaliser is a
	// deliberate change with a norm_version bump behind it rather than a silent
	// one that leaves every stored row stale and unmarked.
	if got := NormalizeTitle("ırmak"); got != "ırmak" {
		t.Errorf("NormalizeTitle(%q) = %q; the v1 normaliser is locale-blind and leaves "+
			"dotless ı alone. If this changed, bump NormVersion and write the backfill",
			"ırmak", got)
	}
	if NormVersion != 1 {
		t.Errorf("NormVersion = %d; bumping it needs the migration that rebuilds older rows", NormVersion)
	}
}
