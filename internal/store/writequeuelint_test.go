package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/repofiles"
)

// writeQueueWriteSQL matches an INSERT, REPLACE or UPDATE aimed at write_queue
// in a SQL string, tolerating the whitespace and newlines a real query is
// written with.
//
// ⚠️ IT IS THE SAME PATTERN AS imageAssetWriteSQL, WITH A DIFFERENT TABLE NAME,
// AND THAT IS DELIBERATE RATHER THAN AN OVERSIGHT. Parameterising one regex over
// both would move the pattern away from imagelint_test.go's comment, which
// documents FOUR MEASURED WIDENINGS of it — a quoted identifier, a backticked
// one, a bracketed one, a schema qualifier, and `UPDATE OR IGNORE` — each of
// which walked straight past the first version. That comment is worth more
// attached to the thing it explains than a shared helper is worth. The
// subtest below re-measures every one of those spellings against THIS name, so
// the two cannot drift silently.
//
// COARSE ON PURPOSE, the same way. The rule is "if anything in this repository
// writes write_queue, the state vocabulary must be validated in Go" — including
// a writer that only touches `attempts`, because the fix is one reference and
// the alternative is parsing SQL column lists to decide. A DELETE writes no
// state and owes nothing.
//
// What still walks past it, stated rather than hidden: a query assembled by
// concatenation or fmt.Sprintf where the verb and the table name never appear in
// one string literal. An AST walk over BasicLits cannot see that, and no regex
// can. The guard is a floor, not a proof.
var writeQueueWriteSQL = regexp.MustCompile(
	`(?is)\b(insert|replace|update)\s+(or\s+\w+\s+)?(into\s+)?` +
		`(["` + "`" + `\[]?\w+["` + "`" + `\]]?\s*\.\s*)?["` + "`" + `\]]?write_queue["` + "`" + `\]]?\b`)

// TestWriteQueueWritesValidateTheStateVocabulary is the mechanism ADR-0039 owed
// and did not have.
//
// THE FAILURE IT EXISTS TO PREVENT IS THE ONE THIS PROJECT ALREADY HAD, AND HAD
// FOR A YEAR OF COMMITS. ADR-0039 decision 1 dropped write_queue.state's CHECK
// and said the vocabulary "moves to Go and is documented and validated there".
// It never did. The ADR carries a dated correction admitting the sentence was
// false the day it was written, and its consequences bullet has always read
// "Until that code exists, the vocabulary is documented in 00005's header and
// nowhere else, which is a real gap". Nothing noticed for as long as it took
// someone to go and read the ADR, because the promise lived only in prose.
// internal/store/writequeue.go is now that code; this is what stops it becoming
// prose again.
//
// ⚠️ IT DOES NOT START VACUOUS, unlike its sibling
// TestImageWritesValidateTheFormatVocabulary, which logged VACUOUS PASS for
// months because nothing wrote image_asset yet. write_queue HAS a writer:
// internal/db/spike/fixture.go seeds 8,000 rows and picks among four state
// literals. It is behind `//go:build bench` so `go build ./...` never compiles
// it, but this walk parses source rather than building it, which is the right
// choice here — a build-tagged writer writes real rows into a real column, and a
// guard that could not see it would be a guard with a hole exactly where this
// repository's only writer sits. The load-bearing branch therefore runs on every
// `make check` from the day this lands, and it went RED on that writer before
// the writer was routed through ValidWriteQueueState.
//
// WHAT IT DOES NOT CLAIM. It checks that the validator is REFERENCED, not that
// it is called on the right value at the right moment — an AST walk cannot know
// that, and pretending otherwise would be the same kind of over-claim ADR-0039
// made. What it removes is the silent-skip path, which is how the last one was
// lost.
//
// SCOPE: production code only, matching TestNoCodeMutatesTheAuditLog and
// TestImageWritesValidateTheFormatVocabulary. Test files are exempt, and must
// be: this file's own regex-fixture subtest contains the exact strings it
// forbids, and writequeue_test.go INSERTs a deliberately misspelt state to prove
// the column has no CHECK.
//
// WHICH FILES "PRODUCTION CODE" MEANS is internal/repofiles' question, not
// this file's. This guard used to answer it itself, with a filepath.WalkDir
// from the module root and a prune list that did not name `.claude` — so it
// scanned every agent lane's nested checkout of this repo as though those
// files were this tree's. It stayed green anyway, because those checkouts
// happened to hold no violation in a non-test .go file; that is a coincidence,
// not a property, and its sibling internal/db/planlint_test.go — same walk,
// same prune list, _test.go scope — went red with 875 foreign faults.
func TestWriteQueueWritesValidateTheStateVocabulary(t *testing.T) {
	// Fire the matcher before trusting it. A matcher that has never been shown
	// to match is indistinguishable from no matcher at all.
	t.Run("matcher", func(t *testing.T) {
		shouldMatch := []string{
			`INSERT INTO write_queue (idempotency_key, state) VALUES (?, ?)`,
			"insert\n  into write_queue\n  (idempotency_key, state)",
			`INSERT OR IGNORE INTO write_queue (idempotency_key) VALUES (?)`,
			`REPLACE INTO write_queue (idempotency_key) VALUES (?)`,
			`UPDATE write_queue SET state = 'inflight' WHERE id = ?`,
			// The five spellings imagelint_test.go measured walking past the
			// first version of this pattern, re-measured against this name.
			`INSERT INTO "write_queue" (state) VALUES (?)`,
			"INSERT INTO `write_queue` (state) VALUES (?)",
			`INSERT INTO main.write_queue (state) VALUES (?)`,
			`UPDATE OR IGNORE write_queue SET state = ?`,
			`UPDATE main.write_queue SET state = ?`,
			`UPDATE "write_queue" SET state = ?`,
			`INSERT INTO write_queue(state) VALUES (?)`,
		}
		for _, s := range shouldMatch {
			if !writeQueueWriteSQL.MatchString(s) {
				t.Errorf("writeQueueWriteSQL failed to match a write it must catch:\n  %q", s)
			}
		}
		shouldNotMatch := []string{
			// The reconciliation guard's read, and the retry sweep's — the two
			// statements sync.md §4 actually specifies. Neither writes a state.
			`SELECT id FROM write_queue WHERE state IN ('pending','inflight','verifying')`,
			`SELECT id FROM write_queue WHERE user_id = ? AND idempotency_key = ?`,
			// A DELETE writes no state, so it owes no validation.
			`DELETE FROM write_queue WHERE settled_at < ?`,
			// The rebuild's scratch tables are their own objects; 00005 is SQL,
			// not Go, but a Go caller naming them must not be swept up by the
			// `\w+` in the schema-qualifier arm.
			`INSERT INTO write_queue_new (state) VALUES (?)`,
			`UPDATE main.write_queue_old SET state = ?`,
			`UPDATE provenance SET acquisition_state = ? WHERE id = ?`,
		}
		for _, s := range shouldNotMatch {
			if writeQueueWriteSQL.MatchString(s) {
				t.Errorf("writeQueueWriteSQL matched a statement that is not a write_queue write:\n  %q", s)
			}
		}
	})

	root := repoRoot(t)

	paths, err := repofiles.NonTestGoFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("enumerating this repository's non-test .go files: %v", err)
	}

	var writes []string
	validatorReferenced := false
	inspected := 0
	fset := token.NewFileSet()

	for _, path := range paths {
		// writequeue.go is the declaration, not a reference to it.
		if filepath.Base(path) == "writequeue.go" && filepath.Base(filepath.Dir(path)) == "store" {
			continue
		}

		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Logf("skipping unparseable %s: %v", path, perr)
			continue
		}
		inspected++

		rel, _ := filepath.Rel(root, path)
		scanWriteQueueWrites(fset, rel, file, &writes, &validatorReferenced)
	}
	// A floor, per DEVELOPMENT.md §11's rule 4. Both branches below read as a
	// verdict about the tree; over an empty listing they would be a verdict
	// about nothing, and the two would be indistinguishable in the output.
	if inspected == 0 {
		t.Fatal("the enumeration found no non-test .go files at all, so this guard is checking nothing")
	}
	t.Logf("inspected %d non-test .go files this repository owns, %d write_sites", inspected, len(writes))

	if len(writes) == 0 {
		t.Error("no production code writes write_queue at all, which was NOT true when this " +
			"guard landed: internal/db/spike/fixture.go seeded 8,000 rows across four state " +
			"literals. If that writer was deliberately removed, delete this assertion in the " +
			"same commit and say so — but a guard that quietly became vacuous is exactly the " +
			"failure ADR-0039 spent a year in.")
		return
	}
	if !validatorReferenced {
		t.Errorf("production code writes write_queue but nothing references "+
			"store.ValidWriteQueueState:\n  %s\n"+
			"write_queue.state carries NO CHECK constraint (migration 00005, ADR-0039 "+
			"decision 1) because the lifecycle vocabulary is still growing. Go is where "+
			"that vocabulary is enforced, and this is the writer that owes the call. "+
			"ADR-0039 promised exactly this validation and shipped without it; that is "+
			"the outcome this test exists to make impossible.",
			strings.Join(writes, "\n  "))
	}
}

// scanWriteQueueWrites is the guard's whole judgement, extracted from the tree
// walk so it can be run against a source file that does NOT exist on disk.
//
// The extraction matters for the same reason it does in imagelint_test.go: the
// tree has exactly one write_queue writer today, and if it were ever removed the
// walk could only take a branch that says nothing. TestWriteQueueLintGuardFires
// runs the judgement directly.
func scanWriteQueueWrites(
	fset *token.FileSet, rel string, file *ast.File, writes *[]string, validatorReferenced *bool,
) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BasicLit:
			if node.Kind != token.STRING {
				return true
			}
			val, uerr := strconv.Unquote(node.Value)
			if uerr != nil {
				val = node.Value
			}
			if writeQueueWriteSQL.MatchString(val) {
				*writes = append(*writes,
					rel+":"+strconv.Itoa(fset.Position(node.Pos()).Line)+": "+firstLine(val))
			}
		case *ast.Ident:
			// Catches both the in-package `ValidWriteQueueState(...)` and the
			// cross-package `store.ValidWriteQueueState(...)`, since a
			// selector's Sel is an Ident too.
			if node.Name == "ValidWriteQueueState" {
				*validatorReferenced = true
			}
		}
		return true
	})
}

// TestWriteQueueLintGuardFires triggers the guard on purpose against source that
// does not exist on disk, because a guard whose failing branch has never
// executed is indistinguishable from no guard, and this repository has a
// recorded history (SW-11, and ADR-0039 itself) of checks that were green on
// nothing.
func TestWriteQueueLintGuardFires(t *testing.T) {
	parse := func(t *testing.T, src string) *ast.File {
		t.Helper()
		f, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", src, 0)
		if err != nil {
			t.Fatalf("parsing the synthetic source: %v", err)
		}
		return f
	}
	scan := func(t *testing.T, src string) ([]string, bool) {
		t.Helper()
		var writes []string
		validator := false
		scanWriteQueueWrites(token.NewFileSet(), "synthetic.go", parse(t, src), &writes, &validator)
		return writes, validator
	}

	t.Run("a writer with no validator is caught", func(t *testing.T) {
		const src = `package p

func enqueue(db any) {
	q := "INSERT INTO write_queue (idempotency_key, kind, payload, state) VALUES (?, ?, ?, ?)"
	_ = q
}
`
		writes, validator := scan(t, src)
		if len(writes) == 0 {
			t.Fatal("the guard did not see an INSERT INTO write_queue; its failing branch " +
				"is unreachable and it protects nothing")
		}
		if validator {
			t.Error("the guard reported the validator referenced by a file that never names it")
		}
	})

	t.Run("a state transition with no validator is caught", func(t *testing.T) {
		const src = `package p

func claim(db any) {
	q := "UPDATE write_queue SET state = 'inflight', attempts = attempts + 1 WHERE id = ?"
	_ = q
}
`
		writes, _ := scan(t, src)
		if len(writes) == 0 {
			t.Fatal("the guard did not see an UPDATE … SET state; a worker claiming a row is " +
				"the single most likely future writer and it must not walk past")
		}
	})

	t.Run("a writer that references the validator is accepted", func(t *testing.T) {
		const src = `package p

func enqueue(db any, state string) {
	if !store.ValidWriteQueueState(state) {
		return
	}
	q := "INSERT INTO write_queue (idempotency_key, kind, payload, state) VALUES (?, ?, ?, ?)"
	_ = q
}
`
		writes, validator := scan(t, src)
		if len(writes) == 0 {
			t.Fatal("the guard did not see the write")
		}
		if !validator {
			t.Error("the guard did not see store.ValidWriteQueueState through the selector, so " +
				"a cross-package caller would fail the build for doing the right thing")
		}
	})

	t.Run("a reader owes nothing", func(t *testing.T) {
		const src = `package p

func runnable(db any) {
	q := "SELECT id FROM write_queue WHERE state IN ('pending','inflight','verifying')"
	_ = q
}
`
		writes, _ := scan(t, src)
		if len(writes) != 0 {
			t.Errorf("the guard flagged the reconciliation guard's own read as a write: %v\n"+
				"Flagging readers is how a guard gets routed around.", writes)
		}
	})
}
