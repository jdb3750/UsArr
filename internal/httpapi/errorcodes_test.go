package httpapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The frontend is building a fixture test that asserts every fixture's `error`
// value is a code a handler actually emits. That test is only worth anything if
// the Go side's list is complete and the handlers cannot emit anything outside
// it, which is what the three tests below enforce.
//
// They read this package's own source rather than exercising every endpoint,
// because the failure being guarded is a code that no test happens to reach: a
// rarely-hit branch returning "sudo_requird" is invisible to a behavioural test
// and obvious to the parser.

// TestErrorCodesAreConstants fails if any handler names an error code with a
// string literal instead of one of the exported constants.
//
// The ErrorCode type alone does not catch this: Go converts an untyped string
// constant to a named string type implicitly, so `errStatus(400, "typo", …)`
// compiles cleanly. The parser is the part that does not.
func TestErrorCodesAreConstants(t *testing.T) {
	declared := declaredErrorCodeNames(t)

	// Two expressions are legal without naming a constant, and both are in the
	// single funnel every handler error passes through rather than in a handler:
	//
	//   - errStatus's own `Code: code`, which is the parameter this test is
	//     checking at every CALL of errStatus, and
	//   - writeError's `Error: ae.Code`, which copies an apiError's already
	//     validated code into the wire body.
	//
	// They are allowed by enclosing function name, not by shape, so a handler
	// that grew a local named `code` would still fail.
	exempt := func(fn string, expr ast.Expr) bool {
		switch e := expr.(type) {
		case *ast.Ident:
			return fn == "errStatus" && e.Name == "code"
		case *ast.SelectorExpr:
			return fn == "writeError" && e.Sel.Name == "Code"
		}
		return false
	}

	// check reports a bad code expression: it must name one of the constants.
	check := func(t *testing.T, fset *token.FileSet, fn, where string, expr ast.Expr) {
		t.Helper()
		if exempt(fn, expr) {
			return
		}
		switch e := expr.(type) {
		case *ast.Ident:
			if !declared[e.Name] {
				t.Errorf("%s: %s at %s is not one of the ErrorCode constants in errorcodes.go",
					where, e.Name, fset.Position(expr.Pos()))
			}
		case *ast.CallExpr:
			// An explicit ErrorCode("…") conversion is the other way a literal
			// sneaks past the named type.
			if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "ErrorCode" {
				t.Errorf("%s at %s converts a value to ErrorCode inline. Declare a constant in "+
					"errorcodes.go instead, so ErrorCodes() reports it.",
					where, fset.Position(expr.Pos()))
				return
			}
			t.Errorf("%s at %s computes its error code. Codes must be constants so the set is "+
				"enumerable; see errorcodes.go.", where, fset.Position(expr.Pos()))
		case *ast.BasicLit:
			if e.Kind == token.STRING {
				val, _ := strconv.Unquote(e.Value)
				t.Errorf("%s at %s names the error code with the literal %q.\n"+
					"Every code a handler emits must be one of the exported constants in "+
					"errorcodes.go, because the frontend's fixture test enumerates that set and "+
					"a literal is invisible to it. Add a constant (and its errorCodes entry) or "+
					"use the existing one.", where, fset.Position(expr.Pos()), val)
			}
		default:
			// A variable or a call. Nothing in this package does that today, and
			// allowing it would let a code escape the set at run time.
			t.Errorf("%s at %s computes its error code. Codes must be constants so the set is "+
				"enumerable; see errorcodes.go.", where, fset.Position(expr.Pos()))
		}
	}

	forEachPackageFile(t, func(t *testing.T, fset *token.FileSet, file *ast.File, name string) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					id, ok := node.Fun.(*ast.Ident)
					if !ok || id.Name != "errStatus" || len(node.Args) < 2 {
						return true
					}
					check(t, fset, fn.Name.Name, name+": errStatus", node.Args[1])
				case *ast.CompositeLit:
					lit, ok := node.Type.(*ast.Ident)
					if !ok {
						return true
					}
					// apiError names the field Code; errorBody names it Error,
					// because that is the JSON key.
					field := ""
					switch lit.Name {
					case "apiError":
						field = "Code"
					case "errorBody":
						field = "Error"
					default:
						return true
					}
					for _, el := range node.Elts {
						kv, ok := el.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if key, ok := kv.Key.(*ast.Ident); ok && key.Name == field {
							check(t, fset, fn.Name.Name, name+": "+lit.Name+"."+field, kv.Value)
						}
					}
				}
				return true
			})
		}
	})
}

// TestErrorCodesSetIsComplete fails if a constant was declared but never added
// to errorCodes, which would make it invisible to ErrorCodes() and so to the
// frontend fixture test — the exact drift this whole file exists to stop.
func TestErrorCodesSetIsComplete(t *testing.T) {
	inSet := make(map[string]bool, len(errorCodes))
	for _, c := range ErrorCodes() {
		inSet[string(c)] = true
	}

	for name, value := range declaredErrorCodeValues(t) {
		if !inSet[value] {
			t.Errorf("%s = %q is declared but missing from the errorCodes map, so ErrorCodes() "+
				"does not report it", name, value)
		}
		delete(inSet, value)
	}
	for leftover := range inSet {
		t.Errorf("errorCodes holds %q, which no exported constant declares", leftover)
	}
}

// TestErrorCodesAreDistinct catches a copy-paste that gives two constants the
// same wire value: the set would silently be one code short and one screen
// would never be reachable by name.
func TestErrorCodesAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for name, value := range declaredErrorCodeValues(t) {
		if prev, dup := seen[value]; dup {
			t.Errorf("%s and %s both equal %q", prev, name, value)
			continue
		}
		seen[value] = name
	}
	if len(seen) != len(errorCodes) {
		t.Errorf("%d distinct declared codes but %d entries in errorCodes", len(seen), len(errorCodes))
	}
}

// TestKnownErrorCodeRejectsAStranger is the positive/negative pair for the
// predicate the frontend's fixture check will lean on.
func TestKnownErrorCodeRejectsAStranger(t *testing.T) {
	if !KnownErrorCode(CodeSudoRequired) {
		t.Error("KnownErrorCode rejected a real code")
	}
	// A plausible typo of a real code. The whole point of exporting the set is
	// that this fails somewhere rather than shipping.
	if KnownErrorCode(ErrorCode("sudo_requird")) {
		t.Error("KnownErrorCode accepted a typo")
	}
	if KnownErrorCode("") {
		t.Error("KnownErrorCode accepted the empty code")
	}
}

// declaredErrorCodeValues reads errorcodes.go and returns constant name → wire
// value for every `X ErrorCode = "y"` declaration.
func declaredErrorCodeValues(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "errorcodes.go", nil, 0)
	if err != nil {
		t.Fatalf("parse errorcodes.go: %v", err)
	}

	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typ, ok := vs.Type.(*ast.Ident)
			if !ok || typ.Name != "ErrorCode" {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Errorf("%s is not a string literal", name.Name)
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", name.Name, err)
				}
				out[name.Name] = val
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no ErrorCode constants found in errorcodes.go")
	}
	return out
}

func declaredErrorCodeNames(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for name := range declaredErrorCodeValues(t) {
		out[name] = true
	}
	return out
}

// forEachPackageFile visits every non-test .go file of this package.
func forEachPackageFile(t *testing.T, fn func(*testing.T, *token.FileSet, *ast.File, string)) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	fset := token.NewFileSet()
	visited := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fn(t, fset, file, name)
		visited++
	}
	if visited == 0 {
		t.Fatal("no package files were parsed; this test would pass vacuously")
	}
}
