package imagecache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The derivation is asserted against a SECOND, INDEPENDENT COMPUTATION of it
// rather than against a golden string, so the test states the rule
// (`sha256(url)[:16]`) instead of memorising one answer. A golden string would
// pass for a Key that hashed the wrong thing as long as nobody recomputed it.
func TestKeyIsTheDocumentedDerivation(t *testing.T) {
	const src = "https://books.example/api/v1/books/42/cover"

	got, err := Key(src)
	if err != nil {
		t.Fatalf("Key(%q): %v", src, err)
	}

	sum := sha256.Sum256([]byte(src))
	want := hex.EncodeToString(sum[:])[:16]
	if got != want {
		t.Errorf("Key derived %q; migration 00005 spells cache_key as sha256(source_url)[:16], "+
			"which is %q. A key that is not this one is a cache the reader never looks in.", got, want)
	}
	if !ValidKey(got) {
		t.Errorf("Key produced %q, which ValidKey rejects — so the writer and the reader disagree "+
			"about what a cache key is", got)
	}

	again, err := Key(src)
	if err != nil || again != got {
		t.Errorf("Key is not deterministic: %q then %q (%v)", got, again, err)
	}
	if other, _ := Key(src + "x"); other == got {
		t.Error("two different source URLs derived the same key; the derivation is not a hash of the URL")
	}
}

// TestKeyRefusesACredentialledURL is the DRILL for security.md §5's assertion,
// fired with a value that must trigger it.
//
// It fails against a Key that hashes whatever it is given — which is what a
// derivation written without the rule in mind looks like — because such a Key
// returns a valid-looking key and no error for every case below.
func TestKeyRefusesACredentialledURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		// Deliberately NOT a textbook example value: `gitleaks` waives anything
		// matching `.+EXAMPLE$` by rule, so an example key is the one drill
		// value that proves nothing about a secret-scanned tree.
		{"apikey parameter", "https://books.example/cover?apikey=not-a-real-key-just-a-drill"},
		{"camelCase spelling", "https://books.example/cover?apiKey=not-a-real-key-just-a-drill"},
		{"opensubsonic token", "https://books.example/cover?t=abc&s=def"},
		{"tracker passkey", "https://tracker.example/cover?passkey=drill-value-not-a-passkey"},
		{"userinfo", "https://someone:drill-value@books.example/cover"},
		{"unparseable", "://not a url at all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := Key(tc.url)
			if err == nil {
				t.Fatalf("Key accepted a credentialled URL and returned %q; security.md §5 "+
					"requires that no row be written whose source_url carries a credential, and "+
					"cache_key is derived from that same string", key)
			}
			if !errors.Is(err, ErrCredentialInURL) {
				t.Errorf("Key refused with %v, which is not ErrCredentialInURL — a caller cannot "+
					"tell a credential refusal from a transport failure", err)
			}
			if key != "" {
				t.Errorf("Key returned both a refusal and a key %q; a caller that ignores the "+
					"error would write the row anyway", key)
			}
		})
	}
}

func TestKeyAcceptsACredentialFreeURLWithAQuery(t *testing.T) {
	// A non-credential query parameter must survive: the guard is a deny-list,
	// not "no query strings allowed".
	if _, err := Key("https://books.example/cover?size=large&page=2"); err != nil {
		t.Errorf("Key refused a credential-free URL: %v", err)
	}
}

// TestPutRoundTripsThroughOpen is the guard the package doc's central argument
// is about: the writer and the reader must agree on the layout.
//
// It fails against a Put that computes its own path — the file lands somewhere
// Open does not look, Open returns ErrNotCached, and on a real install that
// would be a cache that silently never hits with nothing failing anywhere.
func TestPutRoundTripsThroughOpen(t *testing.T) {
	c := New(t.TempDir())
	// Mostly zeroes, on store_test.go's fixtureAPIKey convention: a fixture that
	// looks like a real high-entropy secret is one `make check`'s gitleaks pass
	// reports as a leak, and waiving it would train the next reader to waive.
	const key = "00000000000000ab"

	for _, w := range Widths() {
		body := []byte("bytes for width " + w)
		if err := c.Put(key, w, body); err != nil {
			t.Fatalf("Put(%q, %q): %v", key, w, err)
		}

		f, info, err := c.Open(key, w)
		if err != nil {
			t.Fatalf("Open(%q, %q) after Put: %v — the writer and the reader disagree about the layout", key, w, err)
		}
		got, rerr := io.ReadAll(f)
		_ = f.Close()
		if rerr != nil {
			t.Fatalf("reading back w=%s: %v", w, rerr)
		}
		if string(got) != string(body) {
			t.Errorf("w=%s round-tripped %q, want %q", w, got, body)
		}
		if info.Size() != int64(len(body)) {
			t.Errorf("w=%s stat says %d bytes, wrote %d", w, info.Size(), len(body))
		}
	}

	// The shard is part of the layout Open depends on; assert it explicitly so a
	// change to path() that happens to keep Put and Open in step still gets seen.
	if _, err := os.Stat(filepath.Join(c.Root(), key[:2], key, WidthOrig)); err != nil {
		t.Errorf("expected the file under <root>/%s/%s/%s: %v", key[:2], key, WidthOrig, err)
	}
}

func TestPutOverwritesAnExistingRendition(t *testing.T) {
	c := New(t.TempDir())
	const key = "00000000000000cd"

	if err := c.Put(key, Width92, []byte("first")); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if err := c.Put(key, Width92, []byte("second-and-longer")); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	f, _, err := c.Open(key, Width92)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(f)
	_ = f.Close()
	if string(got) != "second-and-longer" {
		t.Errorf("re-encode did not replace the rendition: got %q", got)
	}

	// A rename-based Put must leave no temporary files behind — a growing pile
	// of them in a cache directory is how a disk fills with nothing to blame.
	entries, err := os.ReadDir(filepath.Join(c.Root(), key[:2], key))
	if err != nil {
		t.Fatalf("reading the asset directory: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected exactly the one rendition, found %v", names)
	}
}

// TestPutRefusesAKeyThatIsAPath drills the traversal guard. The key becomes a
// directory name, so this is the write side of the safety property ValidKey
// exists for — and the write side is where a traversal creates something.
func TestPutRefusesAKeyThatIsAPath(t *testing.T) {
	root := t.TempDir()
	c := New(root)

	bad := []string{
		"../../../../etc/x",
		"/absolute/path",
		"00000000000000a",   // fifteen — too short
		"00000000000000abc", // seventeen — too long
		"00000000000000AB",  // uppercase is not the alphabet
		"",
	}
	for _, key := range bad {
		if err := c.Put(key, Width92, []byte("x")); err == nil {
			t.Errorf("Put accepted the malformed key %q", key)
		}
	}
	if err := c.Put("00000000000000ab", "1000", []byte("x")); err == nil {
		t.Error("Put accepted a width outside the allowlist, which is how an arbitrary ?w= " +
			"becomes an unbounded number of cache entries")
	}

	// Nothing may have been created by any of the above.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading the cache root: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the refused writes left %d entries in the cache root", len(entries))
	}
}

func TestPutWithNoCacheDirectoryIsItsOwnAnswer(t *testing.T) {
	c := New("")
	err := c.Put("00000000000000ab", Width92, []byte("x"))
	if !errors.Is(err, ErrNoCacheDir) {
		t.Errorf("Put with no root returned %v; \"this process has no image cache\" is a "+
			"configuration state and not a disk failure", err)
	}
}
