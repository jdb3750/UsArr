package imagecache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The allowlist is §4.4's seven and nothing else, asserted as an exact set so
// that adding an eighth is a deliberate edit here as well as in the constant.
func TestWidthAllowlistIsExactlyArchitectures(t *testing.T) {
	want := []string{"92", "154", "200", "342", "500", "780", "orig"}
	if got := strings.Join(Widths(), ","); got != strings.Join(want, ",") {
		t.Errorf("Widths() = %v, want %v — ARCHITECTURE.md §4.4 names these seven", Widths(), want)
	}
	for _, w := range want {
		if !ValidWidth(w) {
			t.Errorf("ValidWidth(%q) = false", w)
		}
	}
	// The refusals, including the ones that look like widths. An arbitrary
	// `?w=` is a cache-poisoning DoS (GHSA-rrr6-mvwg-9pg9), so a value one away
	// from a member is refused rather than snapped to it.
	for _, w := range []string{
		"", " ", "91", "93", "341", "343", "0", "-92", "+92", "092", "92.0",
		"1e2", "orig ", " orig", "ORIG", "Orig", "92px", "92;", "*", "../92",
	} {
		if ValidWidth(w) {
			t.Errorf("ValidWidth(%q) = true", w)
		}
	}
	if DefaultWidth != WidthOrig {
		t.Errorf("DefaultWidth = %q, want %q — an omitted parameter means "+
			"'the image', not a silent thumbnail", DefaultWidth, WidthOrig)
	}
}

// The key alphabet is the traversal guard, so this asserts the property that
// makes it one: every dangerous byte is outside it.
func TestValidKeyIsSixteenLowercaseHex(t *testing.T) {
	if !ValidKey("0123456789abcdef") {
		t.Fatal("a well-formed key was rejected, so every rejection below is vacuous")
	}
	for _, k := range []string{
		"", "..", "../..", "0123456789abcde", "0123456789abcdef0",
		"0123456789ABCDEF", "0123456789abcde/", "/0123456789abcde",
		"0123456789abcde.", "0123456789abcd\x00", "0123456789abcd ",
		"0123456789abcdg", "0123456789abcde\n", "%2e%2e%2f", "~/x",
	} {
		if ValidKey(k) {
			t.Errorf("ValidKey(%q) = true", k)
		}
	}
}

// A hit, read back through Open, and the layout asserted as a property rather
// than as a hardcoded path: the width is part of the path (two widths of one
// asset are two files) and the key is part of the path (two assets never share
// one).
func TestOpenReadsWhatThePathWrote(t *testing.T) {
	root := t.TempDir()
	c := New(root)

	write := func(key, width, body string) {
		t.Helper()
		p := c.path(key, width)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("aaaaaaaaaaaaaaaa", Width342, "a-342")
	write("aaaaaaaaaaaaaaaa", WidthOrig, "a-orig")
	write("bbbbbbbbbbbbbbbb", Width342, "b-342")

	for _, tc := range []struct{ key, width, want string }{
		{"aaaaaaaaaaaaaaaa", Width342, "a-342"},
		{"aaaaaaaaaaaaaaaa", WidthOrig, "a-orig"},
		{"bbbbbbbbbbbbbbbb", Width342, "b-342"},
	} {
		f, info, err := c.Open(tc.key, tc.width)
		if err != nil {
			t.Fatalf("Open(%s, %s): %v", tc.key, tc.width, err)
		}
		buf := make([]byte, info.Size())
		if _, err := f.Read(buf); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		if string(buf) != tc.want {
			t.Errorf("Open(%s, %s) read %q, want %q", tc.key, tc.width, buf, tc.want)
		}
	}

	// THE FILE HAS NO EXTENSION. `image_asset.format` is the authority on the
	// encoding, and a `.jpg` on disk would be a second claim free to disagree
	// with it.
	if ext := filepath.Ext(c.path("aaaaaaaaaaaaaaaa", Width342)); ext != "" {
		t.Errorf("the cached file carries the extension %q", ext)
	}
	// The two-character shard keeps one directory from holding an entry per
	// asset.
	rel, err := filepath.Rel(root, c.path("aaaaaaaaaaaaaaaa", Width342))
	if err != nil {
		t.Fatal(err)
	}
	if parts := strings.Split(rel, string(filepath.Separator)); len(parts) != 3 ||
		parts[0] != "aa" || parts[1] != "aaaaaaaaaaaaaaaa" || parts[2] != Width342 {
		t.Errorf("path layout = %q, want <shard>/<key>/<width>", rel)
	}
}

// Every way of missing, and each of them is ErrNotCached rather than an error
// naming a filesystem path.
func TestOpenMissesHonestly(t *testing.T) {
	root := t.TempDir()

	for _, tc := range []struct {
		name  string
		cache *Cache
		key   string
		width string
	}{
		{"no cache directory configured", New(""), "aaaaaaaaaaaaaaaa", WidthOrig},
		{"nothing written", New(root), "aaaaaaaaaaaaaaaa", WidthOrig},
		{"malformed key", New(root), "../../etc/passwd", WidthOrig},
		{"unlisted width", New(root), "aaaaaaaaaaaaaaaa", "343"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, err := tc.cache.Open(tc.key, tc.width)
			if f != nil {
				_ = f.Close()
				t.Fatal("a miss returned an open file")
			}
			if !errors.Is(err, ErrNotCached) {
				t.Fatalf("err = %v, want ErrNotCached", err)
			}
			if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), tc.key) {
				t.Errorf("the error names a path or the caller's key: %v", err)
			}
		})
	}
}

// An empty root must not resolve against the WORKING DIRECTORY, which is what
// filepath.Join("", "aa", …) produces. The bait is written where a relative join
// would land and the lookup is asserted not to find it.
func TestOpenWithAnEmptyRootDoesNotReadTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	c := New("")

	if p := c.path("aaaaaaaaaaaaaaaa", WidthOrig); filepath.IsAbs(p) {
		t.Fatalf("path with an empty root = %q; this test's premise has changed", p)
	}
	if err := os.MkdirAll(filepath.Join("aa", "aaaaaaaaaaaaaaaa"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("aa", "aaaaaaaaaaaaaaaa", WidthOrig),
		[]byte("leaked"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, _, err := c.Open("aaaaaaaaaaaaaaaa", WidthOrig)
	if f != nil {
		_ = f.Close()
		t.Fatal("an unconfigured cache read a file out of the working directory")
	}
	if !errors.Is(err, ErrNotCached) {
		t.Errorf("err = %v, want ErrNotCached", err)
	}
}

// A directory where a file should be is a corrupt cache, not a hit: without the
// check, http.ServeContent would send its (empty) contents as an image.
func TestOpenRefusesADirectory(t *testing.T) {
	root := t.TempDir()
	c := New(root)
	if err := os.MkdirAll(c.path("aaaaaaaaaaaaaaaa", WidthOrig), 0o700); err != nil {
		t.Fatal(err)
	}
	f, _, err := c.Open("aaaaaaaaaaaaaaaa", WidthOrig)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, ErrNotCached) {
		t.Errorf("err = %v, want ErrNotCached", err)
	}
}

// An unreadable cache is NOT a miss. Reporting it as one would turn "the cache
// directory is mode 0000" into an install where every image is quietly absent
// and nothing says why.
//
// ⚠️ THE FIRST ARM IS THE ONE THAT ALWAYS RUNS, and it is a path too long for
// the kernel rather than a permission, because this suite runs as root in the
// build container and mode 0000 denies root nothing. ENAMETOOLONG is a plain
// non-ENOENT open failure, which is the whole property under test. The mode-0000
// arm is the realistic failure and runs wherever the tests are not root.
func TestOpenReportsAnUnreadableCache(t *testing.T) {
	t.Run("a path the kernel refuses", func(t *testing.T) {
		// Comfortably past Linux's 4096-byte PATH_MAX, in 200-byte components
		// so no single one exceeds NAME_MAX either — the failure must be the
		// total length and not a component.
		root := filepath.Join(t.TempDir(), strings.Repeat(strings.Repeat("x", 200)+"/", 30))
		c := New(root)

		f, _, err := c.Open("aaaaaaaaaaaaaaaa", WidthOrig)
		if f != nil {
			_ = f.Close()
		}
		if err == nil {
			t.Fatal("an impossible path opened")
		}
		if errors.Is(err, ErrNotCached) {
			t.Error("a kernel-level open failure was reported as a cache miss, which " +
				"hides a broken install behind an empty one")
		}
		if strings.Contains(err.Error(), root) {
			t.Errorf("the error names the cache path: %v", err)
		}
	})

	t.Run("mode 0000", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: mode 0000 denies nothing")
		}
		root := t.TempDir()
		c := New(root)
		p := c.path("aaaaaaaaaaaaaaaa", WidthOrig)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("bytes"), 0o000); err != nil {
			t.Fatal(err)
		}

		f, _, err := c.Open("aaaaaaaaaaaaaaaa", WidthOrig)
		if f != nil {
			_ = f.Close()
		}
		if err == nil {
			t.Fatal("an unreadable file opened")
		}
		if errors.Is(err, ErrNotCached) {
			t.Error("a permission failure was reported as a cache miss")
		}
		if strings.Contains(err.Error(), root) {
			t.Errorf("the error names the cache path: %v", err)
		}
	})
}
