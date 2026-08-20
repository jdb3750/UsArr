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

// imageAssetWriteSQL matches an INSERT or UPDATE aimed at image_asset in a SQL
// string, tolerating the whitespace and newlines a real query is written with.
//
// Like mutatingAuditSQL, it does not attempt to parse SQL. The rule is coarse on
// purpose: "if anything in this repository writes image_asset, the format
// vocabulary must be validated in Go". A coarse matcher that a strange spelling
// occasionally walks past is worth more than a precise one nobody finishes.
//
// ⚠️ THE FIRST VERSION OF THIS PATTERN WAS TOO COARSE IN FOUR MEASURED WAYS, and
// each one is now a case in the matcher subtest below rather than a comment
// promising to handle it: a quoted identifier (`"image_asset"`, backticked, or
// bracketed), a schema qualifier (`main.image_asset`), and `UPDATE OR IGNORE`
// / `UPDATE OR REPLACE` all walked straight past it. Those are not exotic —
// they are what a query builder emits and what a developer writes when SQLite
// complains about a bare identifier. The alternation below therefore matches an
// OPTIONAL schema qualifier and an OPTIONAL quote character on either side of
// the table name, and gives UPDATE the same `OR <verb>` arm INSERT already had.
//
// What still walks past it, stated rather than hidden: a query assembled by
// concatenation or fmt.Sprintf where the verb and the table name never appear in
// one string literal. An AST walk over BasicLits cannot see that, and no regex
// can. The guard is a floor, not a proof.
var imageAssetWriteSQL = regexp.MustCompile(
	`(?is)\b(insert|replace|update)\s+(or\s+\w+\s+)?(into\s+)?` +
		`(["` + "`" + `\[]?\w+["` + "`" + `\]]?\s*\.\s*)?["` + "`" + `\[]?image_asset["` + "`" + `\]]?\b`)

// TestImageWritesValidateTheFormatVocabulary is the guard that makes migration
// 00008's "no CHECK constraint" decision honest.
//
// THE FAILURE IT EXISTS TO PREVENT IS ONE THIS PROJECT ALREADY HAD.
// ADR-0039 dropped write_queue.state's CHECK and said the vocabulary "moves to
// Go and is validated there". It did not, for a year of commits: the ADR carries
// a dated correction admitting that no Go code declared or validated
// write_queue.state, and the column was enforced nowhere that ran. Nothing
// noticed, because nothing could — the promise lived only in prose. Migration
// 00008 makes the same trade for image_asset.format, so it owes a mechanism
// rather than a promise, and this is it.
//
// ⚠️ write_queue.state HAS ITS VALIDATOR NOW (internal/store/writequeue.go), and
// its own guard in this shape — TestWriteQueueWritesValidateTheStateVocabulary,
// in writequeuelint_test.go. It did NOT start vacuous, because
// internal/db/spike/fixture.go was already writing the column.
//
// HOW IT BEHAVES, WHICH IS THE POINT.
//   - While nothing writes image_asset it passes because there is nothing to
//     check. That is a vacuous pass and it is labelled as one in the log output.
//     ⚠️ That label is weaker than it sounds: `t.Log` prints only under `-v` and
//     `make check` does not pass it, so the label reaches whoever is already
//     reading this file and nobody else. TestImageLintGuardFires is the part
//     that does not rely on being read — it executes the failing branch against
//     a synthetic source, so the branch ships having run.
//     ⚠️ THAT BRANCH IS NO LONGER THE ONE THIS TREE TAKES. internal/store/
//     imagewrite.go writes image_asset and calls ValidImageFormat, so the scan
//     below now runs its LOAD-BEARING branch on every `make check`. Verified by
//     drill: deleting the ValidImageFormat call from PosterAsset.validate turns
//     this test red with "production code writes image_asset but nothing
//     references store.ValidImageFormat".
//   - The moment production code contains an INSERT or UPDATE against
//     image_asset, this test requires that some production code also references
//     ValidImageFormat. A writer that lands without validating the vocabulary
//     fails `make check` rather than shipping.
//
// WHAT IT DOES NOT CLAIM. It checks that the validator is REFERENCED, not that
// it is called on the right value at the right moment — an AST walk cannot know
// that, and pretending otherwise would be the same kind of over-claim ADR-0039
// made. What it removes is the silent-skip path, which is how the last one was
// lost.
//
// SCOPE: production code only, matching TestNoCodeMutatesTheAuditLog. Test files
// are exempt, and must be: this file's own regex-fixture subtest contains the
// exact strings it forbids.
//
// WHICH FILES "PRODUCTION CODE" MEANS is internal/repofiles' question, not
// this file's. This guard used to answer it itself, with a filepath.WalkDir
// from the module root and a prune list that did not name `.claude` — so it
// scanned every agent lane's nested checkout of this repo as though those
// files were this tree's. It stayed green anyway, because those checkouts
// happened to hold no violation in a non-test .go file; that is a coincidence,
// not a property, and its sibling internal/db/planlint_test.go — same walk,
// same prune list, _test.go scope — went red with 875 foreign faults.
func TestImageWritesValidateTheFormatVocabulary(t *testing.T) {
	// Fire the guard before trusting it. A matcher that has never been shown to
	// match is indistinguishable from no matcher at all, which is the whole
	// reason CLAUDE.md asks for this.
	t.Run("matcher", func(t *testing.T) {
		shouldMatch := []string{
			`INSERT INTO image_asset (source_url) VALUES (?)`,
			"insert\n  into image_asset\n  (source_url, format)",
			`INSERT OR IGNORE INTO image_asset (source_url) VALUES (?)`,
			`REPLACE INTO image_asset (source_url) VALUES (?)`,
			`UPDATE image_asset SET state = 'ready' WHERE id = ?`,
			// The four spellings the first version of this pattern missed. Each
			// was confirmed to walk past it before the pattern was widened.
			`INSERT INTO "image_asset" (source_url) VALUES (?)`,
			"INSERT INTO `image_asset` (source_url) VALUES (?)",
			`INSERT INTO main.image_asset (source_url) VALUES (?)`,
			`UPDATE OR IGNORE image_asset SET state = ?`,
			`UPDATE main.image_asset SET state = ?`,
			`UPDATE "image_asset" SET state = ?`,
			`INSERT INTO image_asset(source_url) VALUES (?)`,
		}
		for _, s := range shouldMatch {
			if !imageAssetWriteSQL.MatchString(s) {
				t.Errorf("imageAssetWriteSQL failed to match a write it must catch:\n  %q", s)
			}
		}
		shouldNotMatch := []string{
			`SELECT id FROM image_asset WHERE state = ? AND expires_at <= ?`,
			// A DELETE writes no format, so it owes no validation. The expiry
			// sweep in ARCHITECTURE §4.4 is exactly this statement.
			`DELETE FROM image_asset WHERE state = ? AND expires_at <= ?`,
			`INSERT INTO image_asset_thumbnail (id) VALUES (?)`,
			`UPDATE work SET poster_asset_id = ? WHERE id = ?`,
			// The widened pattern must not start matching the neighbours it
			// gained the ability to reach: a different table behind the same
			// schema qualifier, and a table whose name merely contains this one.
			`INSERT INTO main.work (id) VALUES (?)`,
			`UPDATE main.image_asset_thumbnail SET w = ?`,
		}
		for _, s := range shouldNotMatch {
			if imageAssetWriteSQL.MatchString(s) {
				t.Errorf("imageAssetWriteSQL matched a statement that is not an image_asset write:\n  %q", s)
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
		// images.go is the declaration, not a reference to it.
		if filepath.Base(path) == "images.go" && filepath.Base(filepath.Dir(path)) == "store" {
			continue
		}

		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Logf("skipping unparseable %s: %v", path, perr)
			continue
		}
		inspected++

		rel, _ := filepath.Rel(root, path)
		scanImageAssetWrites(fset, rel, file, &writes, &validatorReferenced)
	}
	// A floor, per DEVELOPMENT.md §11's rule 4. Both branches below read as a
	// verdict about the tree; over an empty listing they would be a verdict
	// about nothing, and the two would be indistinguishable in the output.
	if inspected == 0 {
		t.Fatal("the enumeration found no non-test .go files at all, so this guard is checking nothing")
	}
	t.Logf("inspected %d non-test .go files this repository owns, %d write_sites", inspected, len(writes))

	if len(writes) == 0 {
		t.Log("VACUOUS PASS: no production code writes image_asset, so there is " +
			"nothing for this guard to check yet. It becomes load-bearing with the " +
			"first writer. See migration 00008's header and ADR-0050.")
		return
	}
	if !validatorReferenced {
		t.Errorf("production code writes image_asset but nothing references "+
			"store.ValidImageFormat:\n  %s\n"+
			"image_asset.format carries NO CHECK constraint (migration 00008, ADR-0050) "+
			"because the codec vocabulary is expected to grow. Go is where that "+
			"vocabulary is enforced, and this is the writer that owes the call. "+
			"ADR-0039 made this same trade for write_queue.state and left the validator "+
			"unwritten for a year of commits; that is the outcome this test exists to "+
			"make impossible.",
			strings.Join(writes, "\n  "))
	}
}

// scanImageAssetWrites is the guard's whole judgement, extracted from the tree
// walk so it can be run against a source file that does NOT exist on disk.
//
// That extraction is the point, and the reason it is the point has changed.
// ⚠️ THIS USED TO READ "While nothing writes image_asset the walk above can only
// ever take its vacuous branch" — true when written, falsified 2026-08-19 by
// 7e5934d, which landed internal/store/imagewrite.go. The header above carries
// the same correction and the drill that confirmed it.
//
// The extraction still earns its place, on a reason that does not depend on the
// tree being empty: the branch that MATTERS is "a writer landed and nothing
// validates", and NO state of a green tree produces that — a tree in which it
// fires is a tree where `make check` is already red. So without a synthetic
// source that branch would ship never having executed.
// TestImageLintGuardFires runs it.
func scanImageAssetWrites(
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
			if imageAssetWriteSQL.MatchString(val) {
				*writes = append(*writes,
					rel+":"+strconv.Itoa(fset.Position(node.Pos()).Line)+": "+firstLine(val))
			}
		case *ast.Ident:
			// Catches both the in-package `ValidImageFormat(...)` and the
			// cross-package `store.ValidImageFormat(...)`, since a selector's
			// Sel is an Ident too.
			if node.Name == "ValidImageFormat" {
				*validatorReferenced = true
			}
		}
		return true
	})
}

// TestImageLintGuardFires triggers the guard on purpose, because
// TestImageWritesValidateTheFormatVocabulary passed VACUOUSLY when it landed —
// nothing wrote image_asset then — and a guard whose failing branch has never
// executed is indistinguishable from no guard. CLAUDE.md asks for exactly this
// and this repo has a recorded history (SW-11) of checks that were green on
// nothing.
//
// ⚠️ It is kept now that a real writer exists, and is not redundant: it pins the
// judgement against a source file that does NOT exist on disk, so the failing
// branch stays exercised even if the tree's only writer is ever removed.
//
// ⚠️ Note also what the vacuous pass does NOT do: `t.Log` is invisible unless
// the suite runs with `-v`, and `make check` does not. So the "VACUOUS PASS"
// line labels the state for someone already reading the test, not for someone
// reading a green gate. This test is the part that does not depend on anyone
// reading anything.
func TestImageLintGuardFires(t *testing.T) {
	parse := func(t *testing.T, src string) *ast.File {
		t.Helper()
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "synthetic.go", src, 0)
		if err != nil {
			t.Fatalf("parsing the synthetic source: %v", err)
		}
		return f
	}

	t.Run("a writer with no validator is caught", func(t *testing.T) {
		const src = `package p

func save(db any) {
	q := "INSERT INTO image_asset (source_url, format) VALUES (?, ?)"
	_ = q
}
`
		var writes []string
		validator := false
		scanImageAssetWrites(token.NewFileSet(), "synthetic.go", parse(t, src), &writes, &validator)
		if len(writes) == 0 {
			t.Fatal("the guard did not see an INSERT INTO image_asset; its failing branch " +
				"is unreachable and it protects nothing")
		}
		if validator {
			t.Error("the guard reported the validator referenced by a file that never names it")
		}
	})

	t.Run("a writer that references the validator is accepted", func(t *testing.T) {
		const src = `package p

func save(db any, format string) {
	if !store.ValidImageFormat(format) {
		return
	}
	q := "INSERT INTO image_asset (source_url, format) VALUES (?, ?)"
	_ = q
}
`
		var writes []string
		validator := false
		scanImageAssetWrites(token.NewFileSet(), "synthetic.go", parse(t, src), &writes, &validator)
		if len(writes) == 0 {
			t.Fatal("the guard did not see the write")
		}
		if !validator {
			t.Error("the guard did not see store.ValidImageFormat through the selector, so a " +
				"cross-package caller would fail the build for doing the right thing")
		}
	})
}
