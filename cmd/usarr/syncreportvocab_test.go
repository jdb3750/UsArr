package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/libsync"
	"github.com/jdb3750/UsArr/internal/repofiles"
	"github.com/jdb3750/UsArr/internal/store"
)

// syncReportKindName matches the declaration form every sync_report.kind in this
// tree is written in: a string constant whose identifier begins `syncReport` or
// `SyncReport`.
//
// That is a NAMING CONVENTION doing load-bearing work, and it is stated as one
// rather than left implicit. A kind that is not declared this way is invisible
// here — which is exactly why the three that used to be inline literals
// (`identity_conflict`, `container_declined`, `container_bind_failed`) became
// constants in the same change that added this test.
var syncReportKindName = regexp.MustCompile(`^[sS]yncReport[A-Z]`)

// TestSyncReportKindsDoNotCollide is the vocabulary check that internal/store's
// SyncReportDeletionSweep and internal/libsync's SyncReportDeltaWalk each used to
// carry as a hand-copied list in a doc comment.
//
// # Why the list moved out of the comments
//
// Both lists were wrong, in the two different ways such a list can be wrong, and
// both were green the whole time:
//
//   - SyncReportDeltaWalk's named `comic_residue`, WHICH IS NOT A MEMBER AND
//     NEVER WAS, and omitted three that are.
//   - SyncReportDeletionSweep's named eight members and omitted four. It named no
//     non-member, so it was incomplete rather than wrong — the milder shape of
//     the same defect.
//
// A vocabulary check exists to prove a new kind collides with nothing. That
// proof is void if the enumeration is not the enumeration, and nothing
// mechanical was checking that it was: the list and the tree are maintained by
// two different acts, so they drift, and the list is the half that looks
// authoritative. Deriving the enumeration from the tree removes the second act.
//
// # What is asserted
//
// That the values are pairwise distinct. A duplicate WITHIN one Go package is a
// compile error only if the identifiers collide too — two differently named
// constants holding the same string compile fine, and `sync_report.kind` carries
// no CHECK, so the collision would surface as one reader's count silently
// absorbing another writer's rows. Across packages there is no build-time
// protection at all.
//
// The count is not asserted and is not stated anywhere in this file, in either
// direction. A number maintained by a different act than the thing it counts is
// the drift being removed here (DEVELOPMENT.md §11).
func TestSyncReportKindsDoNotCollide(t *testing.T) {
	kinds := syncReportVocabulary(t)

	byValue := map[string][]string{}
	for name, value := range kinds {
		byValue[value] = append(byValue[value], name)
	}
	for value, names := range byValue {
		if len(names) > 1 {
			sort.Strings(names)
			t.Errorf("two sync_report kinds share the value %q: %s\n"+
				"sync_report.kind carries no CHECK, so a collision is not a constraint "+
				"violation — it is one kind's reader silently counting another kind's rows.",
				value, strings.Join(names, ", "))
		}
	}

	t.Logf("derived sync_report.kind vocabulary: %s", strings.Join(sortedValues(kinds), " "))
}

// TestTheSyncReportVocabularyScanSeesEveryPackageThatDeclaresOne is the
// VACUITY GUARD, and it is the arm that makes the test above worth believing.
//
// A scanner that matched nothing would report an empty vocabulary and pass the
// distinctness assertion trivially — the same failure the check exists to
// prevent, one level up. So this arm names anchors BY SYMBOL rather than by
// string literal: a rename, a retype or a deletion is a compile error here, and
// a constant the scanner failed to see is a test failure.
//
// The anchors are chosen to span all three packages that declare a kind, because
// a scan that silently stopped at one package's directory would otherwise look
// healthy.
func TestTheSyncReportVocabularyScanSeesEveryPackageThatDeclaresOne(t *testing.T) {
	kinds := syncReportVocabulary(t)
	present := map[string]bool{}
	for _, v := range kinds {
		present[v] = true
	}

	anchors := map[string]string{
		"internal/store (reconcile.go)": store.SyncReportDeletionSweep,
		"internal/store (refusal)":      store.SyncReportDeletionSweepRefused,
		"internal/store (catalogue.go)": store.SyncReportIDReused,
		"internal/libsync":              libsync.SyncReportDeltaWalk,
		"cmd/usarr":                     syncReportComicSeriesSynthesized,
	}
	for where, value := range anchors {
		if !present[value] {
			t.Errorf("the vocabulary scan did not see %s's %q; "+
				"every assertion in TestSyncReportKindsDoNotCollide is vacuous to that extent",
				where, value)
		}
	}
}

// syncReportVocabulary reads every sync_report.kind out of the non-test Go this
// repository owns, keyed by the identifier that declares it.
//
// WHICH FILES THIS REPOSITORY OWNS is internal/repofiles' question, not this
// file's — a walk that answered it here would scan every agent lane's nested
// checkout, which is the defect internal/store/auditlint_test.go records.
func syncReportVocabulary(t *testing.T) map[string]string {
	t.Helper()

	root := vocabRepoRoot(t)
	files, err := repofiles.NonTestGoFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("list non-test go files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no non-test .go files found; every assertion below would be vacuous")
	}

	kinds := map[string]string{}
	fset := token.NewFileSet()
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) || !syncReportKindName.MatchString(name.Name) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("%s: unquote %s: %v", rel, name.Name, err)
					}
					kinds[rel+":"+name.Name] = value
				}
			}
		}
	}
	if len(kinds) == 0 {
		t.Fatal("the scan found no sync_report kinds at all; the naming convention " +
			"syncReportKindName encodes has been abandoned and this guard is disarmed")
	}
	return kinds
}

// vocabRepoRoot walks up from the package directory to the directory holding
// go.mod, on internal/store/auditlint_test.go's repoRoot.
func vocabRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the package directory")
		}
		dir = parent
	}
}

func sortedValues(kinds map[string]string) []string {
	out := make([]string, 0, len(kinds))
	for _, v := range kinds {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
