package repofiles

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixture builds a tree with one tracked-shaped .go file, one _test.go file, an
// ignored directory, and a nested checkout under `.claude/worktrees/` carrying
// files with the same names. The nested files are what every caller of this
// package must never see.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("go.mod", "module example.test\n\ngo 1.25\n")
	write(".gitignore", ".claude/\nvendorish/\n")
	write("internal/a/a.go", "package a\n")
	write("internal/a/a_test.go", "package a\n")
	write("vendorish/ignored.go", "package ignored\n")
	// The foreign checkout. Same file names, different tree.
	write(".claude/worktrees/other/internal/a/a.go", "package a\n")
	write(".claude/worktrees/other/internal/a/a_test.go", "package a\n")
	return root
}

func rels(t *testing.T, root string, paths []string) []string {
	t.Helper()
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatalf("rel %s: %v", p, err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "internal/a/a.go", "internal/a/a_test.go"},
	} {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v unavailable in this environment: %v: %s", args, err, out)
		}
	}
}

// TestGitListingExcludesANestedCheckout is the whole reason this package exists.
// It is the drill in miniature: a foreign checkout of the same shape sits inside
// the tree, and neither enumeration may return a single file from it.
func TestGitListingExcludesANestedCheckout(t *testing.T) {
	root := fixture(t)
	gitInit(t, root)

	src, err := NonTestGoFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("NonTestGoFiles: %v", err)
	}
	tests, err := TestGoFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("TestGoFiles: %v", err)
	}

	if got, want := rels(t, root, src), []string{"internal/a/a.go"}; !equal(got, want) {
		t.Errorf("NonTestGoFiles = %v, want %v", got, want)
	}
	if got, want := rels(t, root, tests), []string{"internal/a/a_test.go"}; !equal(got, want) {
		t.Errorf("TestGoFiles = %v, want %v", got, want)
	}
	for _, p := range append(append([]string{}, src...), tests...) {
		if strings.Contains(filepath.ToSlash(p), "/.claude/") {
			t.Errorf("a foreign checkout's file reached a caller: %s", p)
		}
	}
}

// TestGitListingIncludesUntrackedButNotIgnored pins the half of the pathspec
// that is easy to drop: --others --exclude-standard, without which a brand-new
// file is unscanned until somebody stages it.
func TestGitListingIncludesUntrackedButNotIgnored(t *testing.T) {
	root := fixture(t)
	gitInit(t, root)

	if err := os.WriteFile(filepath.Join(root, "internal/a/fresh.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := NonTestGoFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("NonTestGoFiles: %v", err)
	}
	got := rels(t, root, src)
	if !contains(got, "internal/a/fresh.go") {
		t.Errorf("an untracked, unignored .go file was not listed: %v", got)
	}
	if contains(got, "vendorish/ignored.go") {
		t.Errorf("a .gitignore'd .go file was listed: %v", got)
	}
}

// TestWalkFallbackOutsideAWorkTree exercises the branch that by definition only
// runs when git cannot answer, which is why nobody ever runs it. Without this
// the fallback ships untested — and a fallback that has never executed is worth
// what any untested code is worth.
func TestWalkFallbackOutsideAWorkTree(t *testing.T) {
	root := fixture(t) // deliberately NOT a git work tree

	if _, ok := gitListed(t.Context(), root); ok {
		t.Fatal("this fixture must not be inside a git work tree, or the test proves nothing")
	}

	src, err := NonTestGoFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("NonTestGoFiles: %v", err)
	}
	tests, err := TestGoFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("TestGoFiles: %v", err)
	}
	got := rels(t, root, src)
	// The walk has no index to consult, so it cannot honour .gitignore and does
	// return vendorish/ignored.go. That is the documented cost of the fallback;
	// what it must still do is prune `.claude`.
	if !contains(got, "internal/a/a.go") {
		t.Errorf("the fallback missed this tree's own source: %v", got)
	}
	for _, p := range append(append([]string{}, src...), tests...) {
		if strings.Contains(filepath.ToSlash(p), "/.claude/") {
			t.Errorf("the fallback descended into a foreign checkout: %s", p)
		}
	}
	if got, want := rels(t, root, tests), []string{"internal/a/a_test.go"}; !equal(got, want) {
		t.Errorf("fallback TestGoFiles = %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
