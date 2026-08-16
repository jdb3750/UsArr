package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// mutatingAuditSQL matches an UPDATE or DELETE aimed at audit_log in a SQL
// string, tolerating the whitespace and newlines a real query is written with.
//
// It deliberately does not try to parse SQL. The rule being enforced is coarse
// on purpose — "no statement anywhere in this repository mutates audit_log" —
// and a coarse matcher with a documented waiver mechanism is more useful than a
// precise one that a slightly unusual spelling walks past.
var mutatingAuditSQL = regexp.MustCompile(`(?is)\b(update\s+audit_log|delete\s+from\s+audit_log)\b`)

// TestNoCodeMutatesTheAuditLog implements the control that
// docs/reference/security.md §6 claims: "no UPDATE/DELETE statements against
// audit_log anywhere in the codebase, enforced by a lint rule AND BEFORE
// UPDATE/DELETE triggers that raise".
//
// The triggers existed and work. The lint rule did not, so the append-only
// property was enforced only at runtime, on the writer, one row at a time — and
// the document asserted a control that was not there. That is the specific thing
// CLAUDE.md forbids ("no invented status"), so rather than downgrade the claim,
// here is the missing half.
//
// It lives as a test rather than a .golangci.yml entry because golangci-lint's
// forbidigo matches function-call identifiers, not the contents of string
// literals, and every one of these statements would be a string literal. A test
// that walks the AST reads every SQL string in the tree and runs in the same
// `make check` gate, which is what actually matters.
//
// SCOPE: production code only. _test.go files are exempt, and the exemption is
// not a loophole — it is required. TestAuditChainDetectsTampering and
// TestAuditLogIsAppendOnly mutate audit_log on purpose, precisely to prove the
// triggers REJECT the mutation and that the hash chain notices when the file is
// edited underneath it. Forbidding those statements in tests would delete the
// tests that verify the guarantee. security.md §6 has been amended to state this
// scope, because "anywhere in the codebase" was not literally what was meant.
//
// Waiving: there is no waiver for production code. If a legitimate need to
// mutate audit_log ever appears, it is a change to the append-only guarantee
// itself and belongs in an ADR plus a migration that drops the triggers — not in
// an exception list here.
func TestNoCodeMutatesTheAuditLog(t *testing.T) {
	root := repoRoot(t)

	var offences []string
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", ".svelte-kit", ".dev":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			// An unparseable Go file is someone else's failure; do not mask it,
			// but do not fail this test for it either.
			t.Logf("skipping unparseable %s: %v", path, perr)
			return nil
		}

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, uerr := strconv.Unquote(lit.Value)
			if uerr != nil {
				// A raw string with an odd escape; scan the literal as written.
				val = lit.Value
			}
			if mutatingAuditSQL.MatchString(val) {
				rel, _ := filepath.Rel(root, path)
				offences = append(offences,
					rel+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+": "+firstLine(val))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	for _, o := range offences {
		t.Errorf("a SQL statement mutates audit_log:\n  %s\n"+
			"audit_log is append-only (security.md §6). The BEFORE UPDATE/DELETE triggers "+
			"will abort this at runtime; it must not be written in the first place.", o)
	}
}

// repoRoot walks up from the package directory to the directory holding go.mod.
func repoRoot(t *testing.T) string {
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

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}
