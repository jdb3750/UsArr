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
var imageAssetWriteSQL = regexp.MustCompile(
	`(?is)\b(insert\s+(or\s+\w+\s+)?into\s+image_asset|replace\s+into\s+image_asset|update\s+image_asset)\b`)

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
//   - While nothing writes image_asset — the state of the tree as 00008 lands —
//     it passes because there is nothing to check. That is a vacuous pass and it
//     is labelled as one in the log output, so a green here is never mistaken
//     for evidence that a writer was reviewed.
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
		}
		for _, s := range shouldMatch {
			if !imageAssetWriteSQL.MatchString(s) {
				t.Errorf("imageAssetWriteSQL failed to match a write it must catch:\n  %q", s)
			}
		}
		shouldNotMatch := []string{
			`SELECT id FROM image_asset WHERE state = ? AND expires_at <= ?`,
			`DELETE FROM image_asset WHERE state = ? AND expires_at <= ?`,
			`INSERT INTO image_asset_thumbnail (id) VALUES (?)`,
			`UPDATE work SET poster_asset_id = ? WHERE id = ?`,
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
					rel, _ := filepath.Rel(root, path)
					writes = append(writes,
						rel+":"+strconv.Itoa(fset.Position(node.Pos()).Line)+": "+firstLine(val))
				}
			case *ast.Ident:
				// Catches both the in-package `ValidImageFormat(...)` and the
				// cross-package `store.ValidImageFormat(...)`, since a selector's
				// Sel is an Ident too.
				if node.Name == "ValidImageFormat" {
					validatorReferenced = true
				}
			}
			return true
		})
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
