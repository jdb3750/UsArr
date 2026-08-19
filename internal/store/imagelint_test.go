package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
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
// Go and is validated there". It never did: the ADR carries a dated correction
// admitting that no Go code declares or validates write_queue.state, and the
// column is enforced nowhere that runs. Nothing noticed, because nothing could
// — the promise lived only in prose. Migration 00008 makes the same trade for
// image_asset.format, so it owes a mechanism rather than a promise, and this is
// it.
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
//     imagewrite.go writes image_asset and calls ValidImageFormat, so the walk
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

	var writes []string
	validatorReferenced := false
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
		// images.go is the declaration, not a reference to it.
		if filepath.Base(path) == "images.go" && filepath.Base(filepath.Dir(path)) == "store" {
			return nil
		}

		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Logf("skipping unparseable %s: %v", path, perr)
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		scanImageAssetWrites(fset, rel, file, &writes, &validatorReferenced)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

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
			"ADR-0039 made this same trade for write_queue.state and never wrote the "+
			"validator; that is the outcome this test exists to make impossible.",
			strings.Join(writes, "\n  "))
	}
}

// scanImageAssetWrites is the guard's whole judgement, extracted from the tree
// walk so it can be run against a source file that does NOT exist on disk.
//
// That extraction is the point. While nothing writes image_asset the walk above
// can only ever take its vacuous branch, so the branch that MATTERS — "a writer
// landed and nothing validates" — would otherwise ship never having executed.
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
