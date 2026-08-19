package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// ─────────────────────────────────────────────────────────────────────────────
// THE SWEEP'S LOOSE END, MECHANISED.
//
// Whole-tokening the plan guards that exist today says nothing about the ones
// that land tomorrow. This repo has concurrent lanes, and a fresh
// `strings.Contains(plan, "SEARCH sdl")` arrived from a lane that had no way to
// know a sweep was under way — which is not that lane's mistake, it is what a
// point-in-time sweep IS. So the sweep ships with a guard that turns "we fixed
// the eighteen we found" into "the nineteenth fails the build".
//
// WHAT IT FLAGS, and it is deliberately narrow so that a false positive is rare
// enough that nobody learns to route around it. A call is a fault when ALL of:
//
//	1. it is `strings.Contains` or `strings.Count`,
//	2. in a _test.go file,
//	3. with a STRING LITERAL needle,
//	4. on a receiver that is a query plan — either the needle itself carries
//	   SQLite plan vocabulary (`SEARCH `, `USING INDEX`, …), or the receiver is a
//	   variable this file traced back to QueryPlan / EXPLAIN QUERY PLAN,
//	5. where the needle ENDS IN AN IDENTIFIER — its last byte is an identifier
//	   byte and its last word carries a lowercase letter. SQLite's plan
//	   vocabulary is uppercase and this schema's identifiers are lowercase
//	   snake_case, so "the last word has a lowercase letter" separates
//	   `SEARCH sdl` from `SEARCH m USING PRIMARY KEY` without an allowlist to
//	   maintain,
//	6. and the call is NEGATED — `if !strings.Contains(...)`, a POSITIVE
//	   assertion that the identifier is present.
//
// Rules 5 and 6 are the classification the sweep used, in executable form.
// A negative assertion (`if strings.Contains(plan, "SCAN sdl")`) is immune
// because a prefix match there fires TOO EAGERLY — a false alarm, never a
// silent green — so flagging it would be noise. A punctuation-terminated needle
// (`(work_id=?)`) and a fixed SQLite phrase (`TEMP B-TREE`, `MULTI-INDEX OR`)
// cannot be satisfied by an `_x` suffix at all.
//
// WHAT WALKS PAST IT, stated rather than hidden:
//   - a NON-LITERAL needle. `strings.Contains(joined, tc.index)` is the exact
//     shape this sweep fixed in TestQueryPlans, and this guard cannot see it:
//     the needle's shape is what rules 5 and 6 judge, and a table-driven needle
//     has no shape until run time. Flagging every raw Contains on a plan-valued
//     receiver instead would flag the immune ones too, which is how a guard
//     gets disabled.
//   - an UPPERCASE identifier. Rule 5 would read it as plan vocabulary.
//   - a plan reached through a helper this file's coarse dataflow does not
//     follow.
//
// It is a floor, not a proof — the same claim internal/store's
// imagelint_test.go makes about its own regex.
// ─────────────────────────────────────────────────────────────────────────────

// planVocabulary is the SQLite EXPLAIN QUERY PLAN phrases that mark a string as
// a plan needle on their own, without needing to know what the receiver holds.
var planVocabulary = []string{
	"SEARCH ", "SCAN ", "USING INDEX", "USING COVERING INDEX",
	"USING PRIMARY KEY", "USING INTEGER PRIMARY KEY", "USING AUTOMATIC",
	"CORRELATED SCALAR SUBQUERY", "TEMP B-TREE", "MULTI-INDEX OR",
	"BLOOM FILTER", "LIST SUBQUERY", "CO-ROUTINE",
}

// needleEndsInAnIdentifier is rule 5: the needle can be satisfied by a LONGER
// identifier, so a rename to `<old>_x` leaves the assertion green and empty.
func needleEndsInAnIdentifier(s string) bool {
	if s == "" || !isPlanIdentByte(s[len(s)-1]) {
		return false
	}
	fields := strings.Fields(s)
	last := fields[len(fields)-1]
	return strings.ContainsFunc(last, func(r rune) bool {
		return unicode.IsLower(r) && r < 128
	})
}

// planLintFault is one flagged call site.
type planLintFault struct {
	Pos    string
	Needle string
}

// scanPlanAssertions is the guard's whole judgement, extracted from the tree
// walk so TestPlanLintGuardFires can run it against source that does NOT exist
// on disk. Without that extraction the failing branch would ship having never
// executed the moment the tree is clean — which, after this sweep, it is.
func scanPlanAssertions(fset *token.FileSet, rel string, file *ast.File) []planLintFault {
	var faults []planLintFault

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		// Coarse, function-scoped dataflow: which locals hold a rendered plan.
		planVars := map[string]bool{}
		explainInFunc := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if strings.Contains(unquote(lit.Value), "EXPLAIN QUERY PLAN") {
					explainInFunc = true
				}
			}
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			if !rhsIsPlanValued(as.Rhs, planVars) {
				return true
			}
			for _, lhs := range as.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" && id.Name != "err" {
					planVars[id.Name] = true
				}
			}
			return true
		})

		// Which calls sit directly under a `!`. That is what separates a
		// positive assertion from a negative one.
		negated := map[*ast.CallExpr]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.NOT {
				if call, ok := u.X.(*ast.CallExpr); ok {
					negated[call] = true
				}
			}
			return true
		})

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "strings" {
				return true
			}
			isCount := sel.Sel.Name == "Count"
			if sel.Sel.Name != "Contains" && !isCount {
				return true
			}
			// Rule 6. strings.Count needles are always positive measurements —
			// there is no `!Count` — so they are judged on shape alone.
			if !isCount && !negated[call] {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true // rule 3: a non-literal needle has no shape to judge
			}
			needle := unquote(lit.Value)
			if !needleEndsInAnIdentifier(needle) {
				return true // rule 5
			}
			// Rule 4.
			onAPlan := explainInFunc
			if id, ok := call.Args[0].(*ast.Ident); ok && planVars[id.Name] {
				onAPlan = true
			}
			if !onAPlan {
				for _, v := range planVocabulary {
					if strings.Contains(needle, v) {
						onAPlan = true
						break
					}
				}
			}
			if !onAPlan {
				return true
			}
			faults = append(faults, planLintFault{
				Pos:    rel + ":" + strconv.Itoa(fset.Position(call.Pos()).Line),
				Needle: needle,
			})
			return true
		})
	}
	return faults
}

// rhsIsPlanValued recognises the three ways a plan reaches a local in this
// repo: QueryPlan itself, a helper whose name ends in `Plan`, and
// strings.Join over something already known to be one.
func rhsIsPlanValued(rhs []ast.Expr, planVars map[string]bool) bool {
	for _, e := range rhs {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			continue
		}
		name := ""
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			name = fn.Sel.Name
			if x, ok := fn.X.(*ast.Ident); ok && x.Name == "strings" && name == "Join" {
				if len(call.Args) > 0 {
					if id, ok := call.Args[0].(*ast.Ident); ok && planVars[id.Name] {
						return true
					}
				}
				continue
			}
		}
		if name == "QueryPlan" || strings.HasSuffix(name, "Plan") {
			return true
		}
	}
	return false
}

func unquote(s string) string {
	if v, err := strconv.Unquote(s); err == nil {
		return v
	}
	return s
}

// TestPlanGuardsUseTheWholeTokenHelper walks every _test.go file in the
// repository and fails on a raw substring plan assertion that a rename could
// silently satisfy.
//
// IS IT VACUOUS? No, and the distinction matters because internal/store's
// imagelint guard logged VACUOUS PASS for months. This walk classifies real
// call sites on every run — thirty-odd plan assertions across internal/db and
// internal/store go through rules 4, 5 and 6 and come out immune. What it does
// NOT do on a clean tree is take its FAILING branch, so that branch is executed
// against synthetic source by TestPlanLintGuardFires below, and was drilled
// against the real tree before this landed: restoring
// `strings.Contains(joined, "SEARCH sdl")` in TestScopedSearchIsASeekNotAScan
// turns this test red naming that line.
//
// SCOPE is _test.go files, which is the inverse of imagelint_test.go's scope
// and is right for this defect: a query plan is only ever asserted in a test.
// This file exempts itself — its fixtures are the strings it forbids, and they
// live inside synthetic program text where the AST sees a string, not a call.
func TestPlanGuardsUseTheWholeTokenHelper(t *testing.T) {
	root := repoRootDir(t)
	fset := token.NewFileSet()

	var faults []planLintFault
	inspected := 0

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
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(path) == "planlint_test.go" {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Logf("skipping unparseable %s: %v", path, perr)
			return nil
		}
		inspected++
		rel, _ := filepath.Rel(root, path)
		faults = append(faults, scanPlanAssertions(fset, rel, file)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	if inspected == 0 {
		t.Fatal("the walk found no _test.go files at all, so this guard is checking nothing")
	}

	for _, f := range faults {
		t.Errorf("%s: strings.Contains/Count asserts the plan identifier %q, which a rename "+
			"to %q would still satisfy — the guard would stay GREEN while pinning nothing.\n"+
			"    Use db.PlanHas(plan, %q), which requires the match to end at a token boundary.\n"+
			"    If this needle is NOT a plan identifier, say so where it is written; this "+
			"guard is deliberately narrow and the fix is never to widen its exemptions.",
			f.Pos, f.Needle, f.Needle+"_x", f.Needle)
	}
	t.Logf("inspected %d _test.go files, %d faults", inspected, len(faults))
}

// TestPlanLintGuardFires executes the failing branch against source that does
// not exist on disk, and pins the four immunities beside it — because a guard
// that only ever says "clean" is indistinguishable from one that always says
// "clean", and a guard that also flags the immune cases is one that gets
// deleted the first time it cries wolf.
func TestPlanLintGuardFires(t *testing.T) {
	parse := func(t *testing.T, src string) *ast.File {
		t.Helper()
		f, err := parser.ParseFile(token.NewFileSet(), "synthetic_test.go", src, 0)
		if err != nil {
			t.Fatalf("parsing the synthetic source: %v", err)
		}
		return f
	}
	scan := func(t *testing.T, body string) []planLintFault {
		t.Helper()
		src := "package p\n\nimport \"strings\"\n\nfunc TestX(t *testing.T) {\n" + body + "\n}\n"
		return scanPlanAssertions(token.NewFileSet(), "synthetic_test.go", parse(t, src))
	}

	caught := []struct {
		name string
		body string
	}{
		{
			// The exact assertion that arrived after the sweep.
			name: "an alias needle on a plan-vocabulary string",
			body: `if !strings.Contains(joined, "SEARCH sdl") { t.Error("x") }`,
		},
		{
			name: "a bare index name on a QueryPlan-derived variable",
			body: `plan, _ := QueryPlan(ctx, db, "SELECT 1")
	joined := strings.Join(plan, " | ")
	if !strings.Contains(joined, "ix_wq_runnable") { t.Error("x") }`,
		},
		{
			name: "a plan scanned out of a raw EXPLAIN QUERY PLAN",
			body: `var p string
	_ = db.QueryRow("EXPLAIN QUERY PLAN SELECT 1").Scan(&p)
	if !strings.Contains(p, "ix_prov_unconfirmed") { t.Error("x") }`,
		},
		{
			name: "a table name after SCAN",
			body: `if !strings.Contains(joined, "SCAN service_instance") { t.Error("x") }`,
		},
		{
			name: "strings.Count, which has no ! to look for",
			body: `if strings.Count(joined, "SEARCH sdl") != 2 { t.Error("x") }`,
		},
	}
	for _, tc := range caught {
		t.Run("caught/"+tc.name, func(t *testing.T) {
			if got := scan(t, tc.body); len(got) == 0 {
				t.Fatalf("the guard did not flag a rot-exposed plan assertion; its failing "+
					"branch is unreachable and it protects nothing:\n  %s", tc.body)
			}
		})
	}

	immune := []struct {
		name string
		body string
	}{
		{
			// The whole point: this is the fixed form and must not be flagged.
			name: "PlanHas is the fix, not a fault",
			body: `if !PlanHas(joined, "SEARCH sdl") { t.Error("x") }`,
		},
		{
			name: "a negative assertion fires too eagerly, never silently green",
			body: `if strings.Contains(joined, "SCAN sdl") { t.Error("x") }`,
		},
		{
			name: "punctuation-terminated cannot be satisfied by _x",
			body: `if !strings.Contains(joined, "SEARCH work USING INDEX ix_work_kind_sort (kind=? AND sort_title>?)") { t.Error("x") }`,
		},
		{
			name: "keyword-terminated cannot be satisfied by _x",
			body: `if !strings.Contains(joined, "SEARCH m USING PRIMARY KEY") { t.Error("x") }`,
		},
		{
			name: "a fixed SQLite phrase is not an identifier",
			body: `if !strings.Contains(joined, "TEMP B-TREE") { t.Error("x") }`,
		},
		{
			name: "an ordinary error-message assertion is not a plan assertion",
			body: `if !strings.Contains(err.Error(), "awaiting_choice") { t.Error("x") }`,
		},
		{
			name: "a DDL assertion is not a plan assertion",
			body: `if !strings.Contains(ddl, "prefix='2 3 4'") { t.Error("x") }`,
		},
	}
	for _, tc := range immune {
		t.Run("immune/"+tc.name, func(t *testing.T) {
			if got := scan(t, tc.body); len(got) != 0 {
				t.Fatalf("the guard flagged an assertion the sweep classified as immune, "+
					"which is how a guard gets routed around:\n  %s\n  %+v", tc.body, got)
			}
		})
	}
}

// repoRootDir walks up to the directory holding go.mod.
func repoRootDir(t *testing.T) string {
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
