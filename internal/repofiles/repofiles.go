// Package repofiles answers one question — which .go files does THIS repository
// own? — for the source-scanning guards that would otherwise each answer it
// their own way.
//
// FOUR GUARDS ASKED IT SEPARATELY AND ALL FOUR GOT IT WRONG THE SAME WAY.
// internal/db/planlint_test.go, and internal/store's imagelint, auditlint and
// writequeuelint guards, each carried an identical filepath.WalkDir from the
// module root with an identical hand-maintained prune list — .git, node_modules,
// dist, build, .svelte-kit, .dev — and that list did not name `.claude`. Agent
// lanes keep their own checkouts of this repo under `.claude/worktrees/`, dozens
// of them, so every one of those walks descended into every one of those
// checkouts and scanned its files as though they were this tree's.
//
// Measured on main at c9006dc, in the primary checkout, for the non-test .go
// enumeration the three internal/store guards use: 5464 files inspected, of
// which 5322 — 97.4% — were under `.claude/`. The 142 that were not are exactly
// the 142 `git ls-files` reports. planlint had already reported the same shape
// on the _test.go side and gone red with it: 875 faults across 5975 files, every
// fault path foreign, in a repo with 158 tracked _test.go files.
//
// The three store guards were green throughout, and that is the point: they
// scan NON-test .go files, and the foreign checkouts happened to hold no
// violation there. A guard that is correct because the files it should not have
// been reading happened to be clean is not a guard, it is a coincidence with a
// good record.
//
// WHY ASK GIT RATHER THAN GROW THE PRUNE LIST. Both turn the tree green today.
// A prune list is an enumeration of hazards, and it goes wrong the way every
// growing enumeration goes wrong: it fixes the directory somebody already
// tripped over and leaves the next one to be found the same way, by a red gate
// nobody can explain. Asking git makes "this repo's files" a question with one
// answer rather than a list to maintain — and it is the answer the Makefile's
// fmt-check already gives, having hit this identical wall and switched
// GO_SRC_LIST to the same command. Every enumeration in the repo that asks git
// agrees with every other by construction rather than by two lists happening to
// match.
//
// The decisions below are c4640c9's, carried forward rather than reinvented;
// that commit's message and the doc comments here are the record of why each
// one is the way it is.
package repofiles

import (
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
)

// TestGoFiles reports the absolute paths of the _test.go files this repository
// owns.
//
// Build-tagged test files are included: the callers parse source without
// resolving build tags, so `upstream`- and `bench`-tagged files are scanned
// exactly as untagged ones are, and a guard's coverage does not depend on which
// tags the run was invoked with.
func TestGoFiles(ctx context.Context, root string) ([]string, error) {
	return goFiles(ctx, root, true)
}

// NonTestGoFiles reports the absolute paths of the .go files this repository
// owns that are NOT _test.go files.
//
// The asymmetry is deliberate and is the callers' own scope rule rather than a
// convenience: internal/store's auditlint guard forbids statements that its own
// tests must be free to write, precisely so those tests can prove the database
// triggers reject them. Exempting _test.go there is required, not a loophole.
func NonTestGoFiles(ctx context.Context, root string) ([]string, error) {
	return goFiles(ctx, root, false)
}

// goFiles is the single enumeration both exported functions partition, so the
// two can never disagree about which tree they are describing.
func goFiles(ctx context.Context, root string, wantTests bool) ([]string, error) {
	all, err := listGoFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(all))
	for _, path := range all {
		if strings.HasSuffix(path, "_test.go") == wantTests {
			files = append(files, path)
		}
	}
	return files, nil
}

// listGoFiles asks git, and falls back to walking the filesystem when git
// cannot answer.
//
// An EMPTY listing from git is still an answer and is returned as one. Deciding
// otherwise would make an empty repository indistinguishable from a broken git,
// and it is not this function's job to catch a vacuous run: the callers assert
// their own floors on the count they receive, which is the only place that can
// know what "too few" means for a given guard.
func listGoFiles(ctx context.Context, root string) ([]string, error) {
	if files, ok := gitListed(ctx, root); ok {
		return files, nil
	}
	return walked(root)
}

// gitListed reports the listing and whether git could answer at all. Every
// failure — git absent, root outside a work tree, a broken index — falls
// through to the walk rather than returning an error, because "not a git
// checkout" is a supported way to hold this source (a release tarball, an
// extracted module zip) and not a fault in it.
//
// The context is threaded from the caller's *testing.T rather than being
// created here: `noctx` fails the lint on an exec.Command that cannot be
// cancelled, and it caught exactly that in c4640c9's first draft.
func gitListed(ctx context.Context, root string) ([]string, bool) {
	// #nosec G204 -- the executable is the literal "git" and no shell is
	// involved; the only variable is `root`, which reaches git as the argument
	// to -C and can therefore name a directory but never a command or a flag.
	// The same two calls live in .golangci.yml's gosec-exempt _test.go scope
	// today (c4640c9); moving them into a package both callers can import is
	// what makes the suppression visible, not what makes it necessary.
	if err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil, false
	}
	// --others --exclude-standard rather than --cached alone is load-bearing:
	// it is tracked files PLUS untracked ones .gitignore does not exclude, so a
	// brand-new .go file is scanned before it is ever staged, while `.claude/`
	// (ignored at .gitignore:117) is not.
	// #nosec G204 -- see above; every argument but `root` is a literal, and
	// `root` is separated from the pathspec by `--`.
	out, err := exec.CommandContext(ctx, "git", "-C", root,
		"ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "*.go").Output()
	if err != nil {
		return nil, false
	}
	// -z, because a path is allowed to contain a newline and a listing split on
	// one would silently lose the file rather than fail.
	files := []string{}
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" {
			continue
		}
		files = append(files, filepath.Join(root, rel))
	}
	return files, true
}

// prunedDirs is the fallback's prune list. It exists only on the path git
// cannot serve, and `.claude` is in it because a source tree extracted without
// its .git can still carry one.
var prunedDirs = map[string]bool{
	".git": true, ".claude": true, "node_modules": true,
	"dist": true, "build": true, ".svelte-kit": true, ".dev": true,
}

// walked is the non-git fallback. It is the fallback and not the primary
// because it cannot tell this repo's files from a checkout that happens to sit
// inside it — the list above is a guess at where those hide, and the whole
// history of this package is that guess being wrong.
func walked(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if prunedDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", root, err)
	}
	return files, nil
}
