package bookorbit

import (
	"crypto/sha1" //nolint:gosec // reproducing git's object name, which is SHA-1; nothing here authenticates anything
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// THE OFFLINE HALF OF THE BOOKORBIT TYPE-DRIFT GUARD. In `make check`.
//
// WHY THIS EXISTS AT ALL. Every wire shape internal/bookorbit reads is
// HAND-TRANSCRIBED from BookOrbit's TypeScript. ADR-0052 made BookOrbit v0.1's
// SOLE catalogue source, so a rename, a retyped field or a dropped key upstream
// is not the degradation of one source among several — it is a single point of
// failure for the whole library, and until this landed it was invisible until an
// import broke against a real server.
//
// WHY IT IS TYPESCRIPT AND NOT AN OPENAPI DOCUMENT. BookOrbit commits no spec:
// server/src/swagger.ts builds one at RUNTIME and main.ts mounts it only when
// SWAGGER_ENABLED is true, which parseBooleanFlag defaults to false. So there is
// nothing to fetch from a running instance and nothing checked in to vendor.
// ADR-0046's floor/ceiling split and ADR-0047's blob-identity pin both assume a
// committed document; the second shape transfers to source files, the first does
// not. packages/types is the substitute, and it is not a second-best — it is the
// only machine-readable statement of the wire vocabulary upstream publishes.
//
// THE SHAPE IS ADR-0047's, EXTENDED FROM ONE FILE TO A DIRECTORY:
//
//	offline, in `make check`      — this file. Are the vendored bytes still the
//	                                bytes this package was written against?
//	                                Deterministic, no network, so it may sit in
//	                                the gate.
//	network, in `make spec-drift` — specdrift_upstream_test.go. Has UPSTREAM moved
//	                                away from what we vendored? Needs github.com,
//	                                so it must NOT sit in the gate.
//
// ⚠️ WHAT NEITHER HALF COVERS. Read this before quoting a green.
//
//  1. IT PINS THE FILE WE VENDORED, NOT THE SERVER THE OWNER RUNS. packages/types
//     is what BookOrbit's own frontend compiles against on one commit. The
//     owner's instance may be older, newer, or a fork. A real instance remains
//     the only evidence about a real instance.
//  2. IT SEES TYPES, NOT BEHAVIOUR. A handler that starts omitting a field, a
//     route that gains a @RequirePermission, a query that silently narrows — none
//     of those touch packages/types, and every one of them breaks this adapter.
//  3. IT SEES WHAT UPSTREAM DECLARES, NOT WHETHER WE READ IT RIGHT. A green says
//     packages/types has not moved. It says nothing about whether catalogue.go
//     transcribed it correctly in the first place — that is what
//     TestMediaKindVocabularyMatchesTheSource and
//     TestPermissionVocabularyMatchesTheSource do, and they stay necessary.
//  4. THE SERVER'S OWN DTOs ARE NOT IN packages/types. Where a controller returns
//     a NestJS class rather than a shared type, no file here describes it. The
//     citations in catalogue.go, errors.go and doc.go name those separately —
//     controllers, services, repositories — and NONE of them is pinned by
//     anything. That is the largest uncovered surface, and it is uncovered
//     because vendoring a NestJS server is not a proportionate answer to it.
//  5. NOTHING HERE RUNS ON ITS OWN. The network half is opt-in behind a build
//     tag and a make target that a person has to type. There is no CI. A drift
//     nobody runs `make spec-drift` for is a drift nobody hears about.

const (
	// vendoredTypesCommit is the BookOrbit commit every transcription in this
	// package cites. It is the one fact a reader must be able to check.
	vendoredTypesCommit = "73b7877d2fede2221b0ca360af9bfced7c3797f3"

	// vendoredTypesUpstreamPath is where the vendored directory came from.
	vendoredTypesUpstreamPath = "packages/types"

	// vendoredTypesTree is GIT'S OWN NAME for that directory at that commit —
	// `git rev-parse 73b7877d:packages/types` printed exactly this on 2026-08-19.
	//
	// A git tree object name rather than a SHA-256 over some concatenation we
	// invented, and the choice is load-bearing twice over:
	//
	//   - It is COMPARABLE TO UPSTREAM WITH NO DOWNLOAD. A blobless, depth-1
	//     fetch resolves a path to its tree name out of the tree objects alone,
	//     so `make spec-drift` answers "has upstream moved" without pulling
	//     200 KB of TypeScript. Same argument ADR-0047 makes for the Prowlarr
	//     blob, one level up the object graph.
	//   - It is UPSTREAM'S NAME, not ours. Nobody has to trust that we hashed the
	//     right bytes in the right order; anyone with a clone can print it.
	//
	// This is also why api/specs/bookorbit-types.manifest lives OUTSIDE the
	// vendored directory. One file added inside it would change the tree, and the
	// identity with upstream — the whole point — would be gone.
	vendoredTypesTree = "4cb990a36b8325845abb79eb4b7a4445e6df679b"
)

// vendoredTypesRoot is the one place the vendored directory's location is
// written down.
//
// api/specs/ rather than docs/reference/, DECIDED rather than inherited. The
// ROADMAP item that raised this obligation headed itself "UNDER docs/reference/"
// and then labelled that heading an inference it wanted settled. The tree settles
// it: api/specs/SOURCES.md opens "These files are vendored verbatim, never
// fetched at build or test time" and carries a provenance-and-hash table that
// contract tests read, while docs/reference/ holds ten hand-written Markdown
// documents and not one vendored artefact. This is a verbatim upstream artefact
// read by a contract test, so it is the first kind. Filing it under
// docs/reference/ would have made that directory two things at once and left
// SOURCES.md — the register that exists precisely to answer "which bytes belong
// there" — not knowing about the most consequential vendored artefact in the
// tree.
func vendoredTypesRoot() string {
	return filepath.Join("..", "..", "api", "specs", "bookorbit-types")
}

func vendoredTypesManifestPath() string {
	return filepath.Join("..", "..", "api", "specs", "bookorbit-types.manifest")
}

// ─── git object names, reimplemented rather than shelled out ─────────────────

// gitBlobName returns the object name git gives these bytes: SHA-1 over
// "blob <len>\x00" + content.
//
// Reimplemented rather than exec'ing `git hash-object` because the gate must not
// depend on a git checkout being present. internal/servarr's gitBlobSHA1 makes
// the same call for the same reason; it is not shared because the two packages
// pin different upstreams and a shared helper would couple them for no gain.
func gitBlobName(b []byte) []byte {
	h := sha1.New() //nolint:gosec // reproducing git's object name, not authenticating anything
	// sha1's Write never errors. strconv rather than fmt.Fprintf so there is no
	// error return to drop.
	h.Write([]byte("blob " + strconv.Itoa(len(b)) + "\x00"))
	h.Write(b)
	return h.Sum(nil)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// treeEntry is one row of a git tree object, before serialisation.
type treeEntry struct {
	mode string
	name string
	sum  []byte
}

// gitTreeName returns the object name git would give the directory at dir.
//
// A tree object is "tree <len>\x00" followed by, per entry,
// "<mode> <name>\x00" and then that entry's 20 RAW sha bytes. Modes are "100644"
// for a regular file and "40000" for a subtree — no leading zero on the tree
// mode, which is the detail that silently yields a wrong-but-plausible hash if
// you guess it. Entries sort by name, with a subtree sorting as though its name
// ended in "/".
//
// Executable files and symlinks are refused rather than guessed at: packages/types
// contains neither, and a tree hash that quietly disagrees with git is worse than
// no tree hash, because it would fail every run for a reason nobody could find.
func gitTreeName(dir string) ([]byte, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	rows := make([]treeEntry, 0, len(ents))
	for _, e := range ents {
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			sum, err := gitTreeName(full)
			if err != nil {
				return nil, err
			}
			rows = append(rows, treeEntry{mode: "40000", name: e.Name(), sum: sum})
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file (mode %s); gitTreeName names blobs and "+
				"trees only, and refuses to guess a mode", full, info.Mode())
		}
		if info.Mode().Perm()&0o111 != 0 {
			return nil, fmt.Errorf("%s is executable (mode %s); git would name it 100755, and this "+
				"vendored tree is expected to hold 100644 files only", full, info.Mode())
		}
		raw, err := os.ReadFile(full) //nolint:gosec // a vendored path this test computes itself
		if err != nil {
			return nil, err
		}
		rows = append(rows, treeEntry{mode: "100644", name: e.Name(), sum: gitBlobName(raw)})
	}
	slices.SortFunc(rows, func(a, b treeEntry) int {
		return strings.Compare(treeSortKey(a), treeSortKey(b))
	})

	var body strings.Builder
	for _, r := range rows {
		body.WriteString(r.mode + " " + r.name + "\x00")
		body.Write(r.sum)
	}
	h := sha1.New() //nolint:gosec // reproducing git's object name
	h.Write([]byte("tree " + strconv.Itoa(body.Len()) + "\x00"))
	h.Write([]byte(body.String()))
	return h.Sum(nil), nil
}

// treeSortKey is git's tree ordering: a subtree sorts as if its name ended in
// "/". Without it, a directory holding both "src" and "src-extra" orders
// differently from git and the hash is wrong.
func treeSortKey(e treeEntry) string {
	if e.mode == "40000" {
		return e.name + "/"
	}
	return e.name
}

// ─── the gate pin ────────────────────────────────────────────────────────────

// TestVendoredBookOrbitTypesAreTheUpstreamTree is the guard the whole vendoring
// exists for, and the one that must never be relaxed.
//
// It cannot detect upstream moving — nothing offline can. What it detects is the
// vendored copy moving under this package: a re-vendor, a hand-edit, a bad merge,
// a stray formatter. Deterministic and offline, so it belongs in `make check`;
// the half that needs github.com is `make spec-drift`.
func TestVendoredBookOrbitTypesAreTheUpstreamTree(t *testing.T) {
	sum, err := gitTreeName(vendoredTypesRoot())
	if err != nil {
		t.Fatalf("naming the vendored tree: %v\n"+
			"api/specs/bookorbit-types/ is vendored, not fetched. If it is missing, restore it from\n"+
			"git rather than downloading one; api/specs/SOURCES.md says which bytes belong there.", err)
	}
	got := hex.EncodeToString(sum)
	if got == vendoredTypesTree {
		return
	}

	// A bare "these two hashes differ" is useless to whoever hits it, so the
	// manifest is read HERE to name the files that moved. The manifest is a
	// diagnosis and never an authority: the constant above is what this test
	// asserts, so editing the manifest to match a tampered file changes nothing
	// about the verdict.
	t.Errorf("api/specs/bookorbit-types/ names git tree %s; this package is pinned to %s\n"+
		"(upstream %s at %s).\n%s\n\n"+
		"WHAT TO DO, in the order worth checking:\n"+
		"  (a) YOU DID NOT TOUCH IT. Then something else did — a bad merge, an editor, a\n"+
		"      formatter that does not know api/specs/ is off limits. Restore it:\n"+
		"          git checkout -- api/specs/bookorbit-types\n"+
		"      A vendored upstream artefact is NEVER hand-edited. If a transcription in this\n"+
		"      package disagrees with it, this package is what is wrong.\n"+
		"  (b) YOU RE-VENDORED DELIBERATELY, at a newer BookOrbit commit. That is the good\n"+
		"      case, and it is a piece of WORK rather than a hash to paste. Re-vendor whole:\n"+
		"          git -C <bookorbit clone> archive <commit> packages/types \\\n"+
		"            | tar -x -C api/specs/bookorbit-types --strip-components=2\n"+
		"      then update, in the same change: vendoredTypesCommit, vendoredTypesTree and the\n"+
		"      dependedOnDeclarations digests here; api/specs/bookorbit-types.manifest; and the\n"+
		"      BookOrbit rows in api/specs/SOURCES.md. Then RE-READ the transcriptions that feed\n"+
		"      off the changed files — catalogue.go, scope.go, stats.go, resources.go — because a\n"+
		"      moved type is exactly when a hand-transcription stops being true.\n"+
		"  (c) A FILE WAS ADDED OR REMOVED under api/specs/bookorbit-types/. Nothing may be\n"+
		"      added there: the pinned hash IS upstream's own tree name, and one extra file\n"+
		"      destroys that identity. The manifest lives OUTSIDE the directory for this reason.",
		got, vendoredTypesTree, vendoredTypesUpstreamPath, vendoredTypesCommit, whichFilesMoved())
}

// whichFilesMoved turns "the tree hash differs" into "these files differ", which
// is the difference between a guard that reports and a guard that only accuses.
func whichFilesMoved() string {
	want, err := readTypesManifest()
	if err != nil {
		return "  (the manifest could not be read, so there is no per-file diagnosis: " + err.Error() + ")"
	}
	got, err := walkVendoredBlobs()
	if err != nil {
		return "  (the vendored directory could not be walked: " + err.Error() + ")"
	}
	var lines []string
	for _, p := range sortedKeys(want) {
		switch g, ok := got[p]; {
		case !ok:
			lines = append(lines, "  MISSING  "+p)
		case g != want[p]:
			lines = append(lines, "  CHANGED  "+p+"  ("+want[p][:12]+" → "+g[:12]+")"+dependencyNote(p))
		}
	}
	for _, p := range sortedKeys(got) {
		if _, ok := want[p]; !ok {
			lines = append(lines, "  ADDED    "+p)
		}
	}
	if len(lines) == 0 {
		return "  (every file matches the manifest, so the difference is a file MODE or an empty\n" +
			"   directory — something git records that a per-file blob comparison does not.)"
	}
	return strings.Join(lines, "\n")
}

// dependencyNote marks the files internal/bookorbit actually transcribes, so a
// reader can tell a change that matters from one that does not.
//
// It is this guard's answer to the cry-wolf problem, and the answer is
// deliberately at REPORT time rather than at VENDOR time. The whole package is
// vendored, because the next slice of this adapter will read files this one does
// not — GET /books/:id, the series endpoints, custom-metadata — and a subset
// vendored today silently un-covers tomorrow's transcription with no warning that
// it has. Grading at report time costs nothing and can be revised in one line.
func dependencyNote(path string) string {
	if _, ok := dependedOnDeclarations[path]; ok {
		return "  ← internal/bookorbit TRANSCRIBES THIS FILE"
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func readTypesManifest() (map[string]string, error) {
	raw, err := os.ReadFile(vendoredTypesManifestPath())
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		blob, path, ok := strings.Cut(strings.TrimSpace(line), "  ")
		if !ok {
			return nil, fmt.Errorf("manifest line %q is not `<blob>  <path>`", line)
		}
		out[strings.TrimSpace(path)] = blob
	}
	return out, nil
}

func walkVendoredBlobs() (map[string]string, error) {
	root := vendoredTypesRoot()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(p) //nolint:gosec // a vendored path this test computes itself
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = hex.EncodeToString(gitBlobName(raw))
		return nil
	})
	return out, err
}

// TestVendoredBookOrbitTypesManifestIsCurrent keeps the DIAGNOSIS honest.
//
// The manifest is only ever read on a failure path, so a stale one would sit
// undetected until the day it was needed and then name the wrong files. A guard
// whose error message lies is worse than one that says nothing, and this is the
// only thing that opens the manifest on a green run.
func TestVendoredBookOrbitTypesManifestIsCurrent(t *testing.T) {
	want, err := readTypesManifest()
	if err != nil {
		t.Fatalf("reading api/specs/bookorbit-types.manifest: %v", err)
	}
	got, err := walkVendoredBlobs()
	if err != nil {
		t.Fatalf("walking the vendored tree: %v", err)
	}
	if len(want) == 0 {
		t.Fatal("the manifest lists 0 files — a green here would mean nothing")
	}
	for p, w := range want {
		g, ok := got[p]
		if !ok {
			t.Errorf("the manifest lists %s, which is not in api/specs/bookorbit-types/", p)
			continue
		}
		if g != w {
			t.Errorf("%s is blob %s; the manifest says %s. Regenerate the manifest in the SAME "+
				"change that re-vendors the tree, never separately.", p, g, w)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			t.Errorf("api/specs/bookorbit-types/%s is not in the manifest", p)
		}
	}
	t.Logf("manifest and vendored tree agree on %d files", len(want))
}

// ─── the declaration digest: what a change MEANS, not merely that there was one ───

// dependedOnDeclarations pins the DECLARATION SURFACE of the five files
// internal/bookorbit hand-transcribes, separately from their bytes.
//
// WHY A SECOND HASH, when the tree hash already pins every byte. Because the two
// answer different questions, and only one of them is worth waking somebody for:
//
//	the tree hash   — "the vendored bytes changed". Catches everything, including
//	                  a typo fixed in a JSDoc line and a file rewrapped by a newer
//	                  Prettier. OFFLINE that is exactly right: those bytes are
//	                  frozen and must not move for any reason at all.
//	the decl digest — "the DECLARATIONS changed". Comments, blank lines and every
//	                  whitespace run outside a string literal are removed before
//	                  hashing, so upstream rewording a doc comment does not read
//	                  the same as upstream renaming a field.
//
// `make spec-drift` uses both: the tree hash to NOTICE, the digest to tell the
// reader whether what it noticed can affect this adapter. Without the second,
// every upstream comment edit arrives as a full-volume alarm — and an alarm that
// is usually noise is an alarm nobody reads, which is the same failure as no
// alarm reached by a longer road.
//
// WHAT THE DIGEST CATCHES: a renamed type, interface, field or enum member; a
// field added or removed; a field retyped; a changed string-literal union; a
// changed enum VALUE; a declaration added or deleted.
//
// WHAT IT DOES NOT CATCH: anything the type system does not state. A field that
// stays `string` and starts arriving null. A route that stops populating it. A
// guard added to a controller. See the numbered list at the top of this file.
//
// WHAT IT DELIBERATELY IGNORES: line and block comments, blank lines, indentation
// and every other whitespace run OUTSIDE a string literal. Whitespace INSIDE a
// literal is preserved, because there the spacing is data.
//
// ONLY THESE FIVE ARE DIGESTED, and the limit is measured rather than cautious.
// stripTypeScript is a lexer, not a parser, and it cannot tell a regex literal
// from a division: api/specs/bookorbit-types/src/pattern-resolver.ts:71 holds a
// character-class regex containing a double quote, which would send the scanner
// into a string state it never leaves. These five contain no such literal, and
// stripTypeScript ASSERTS it finished in code state — so a file that would
// silently corrupt its own digest fails loudly instead of being pinned to
// nonsense.
var dependedOnDeclarations = map[string]string{
	// catalogue.go — BooksPage, BookCard, BookFileRef, BookMediaKind,
	// AUDIO_FORMATS, COMIC_FORMATS, getBookMediaKind.
	"src/book.ts": "abad42024bee483995fdd6ace31469ae9985784fa0ba6dc87f0a8f320b2698f4",
	// catalogue.go — BookQuery, its sort keys and its paging. ADR-0052's claim
	// that a delta channel is possible at all rests on this file.
	"src/query.ts": "29a20331ff765b715e488221dbc30e79a1cea8507f90e20680f3f3d8852e5fc4",
	// scope.go — the Permission enum, all 23 members.
	"src/permissions.ts": "e29ab52d6504a0538efbf8810942ac560d7e2bb07bcc91727a5ca3f5db33e7b2",
	// stats.go — LibraryStats; internal/libsync — DEFAULT_FORMAT_PRIORITY.
	"src/library.ts": "a077c864ba3ef04bbe36d1825766169b4eeface9ecfa2177d692d4c10b165a4e",
	// resources.go — AppInfoResponse.
	"src/app-info.ts": "efeb5ff227afae9f7963917ec19ddb3379c188b5d02a28b0e1ff9e85062a05a1",
}

// scanner states for stripTypeScript.
const (
	stCode = iota
	stLine
	stBlock
	stDouble
	stSingle
	stBack
)

// stripTypeScript removes comments and collapses whitespace outside string
// literals, and reports whether it finished somewhere that makes the result
// trustworthy.
//
// It is a lexer over six states — code, line comment, block comment, and the
// three quote flavours — honouring backslash escapes inside quotes. It does NOT
// recognise regex literals; see dependedOnDeclarations for why that is bounded
// rather than ignored, and why the ok return exists.
func stripTypeScript(src string) (code string, ok bool) {
	state := stCode
	var b strings.Builder
	b.Grow(len(src))
	lastWasSpace := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch state {
		case stCode:
			switch {
			case c == '/' && i+1 < len(src) && src[i+1] == '/':
				// NOT resetting lastWasSpace here is load-bearing. Reset it, and
				// `a /*x*/ b` collapses to "a  b" while `a b` collapses to
				// "a b" — two digests for one declaration, which is precisely the
				// cry-wolf this normaliser exists to prevent.
				state, i = stLine, i+1
			case c == '/' && i+1 < len(src) && src[i+1] == '*':
				state, i = stBlock, i+1
			case c == ' ' || c == '\t' || c == '\n' || c == '\r':
				// One space stands in for any whitespace run, so reindentation
				// and rewrapping cannot move the digest. A space rather than
				// nothing, because `type A = B` and `type A=B` are the same
				// declaration while `exportconst` is not the same token stream as
				// `export const`.
				if !lastWasSpace {
					b.WriteByte(' ')
					lastWasSpace = true
				}
			default:
				b.WriteByte(c)
				lastWasSpace = false
				switch c {
				case '"':
					state = stDouble
				case '\'':
					state = stSingle
				case '`':
					state = stBack
				}
			}
		case stLine:
			if c == '\n' {
				state = stCode
				if !lastWasSpace {
					b.WriteByte(' ')
					lastWasSpace = true
				}
			}
		case stBlock:
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				state, i = stCode, i+1
				if !lastWasSpace {
					b.WriteByte(' ')
					lastWasSpace = true
				}
			}
		default: // inside a quote: emit verbatim, honour backslash escapes
			b.WriteByte(c)
			lastWasSpace = false
			switch {
			case c == '\\' && i+1 < len(src):
				i++
				b.WriteByte(src[i])
			case state == stDouble && c == '"',
				state == stSingle && c == '\'',
				state == stBack && c == '`':
				state = stCode
			}
		}
	}
	return strings.TrimSpace(b.String()), state == stCode
}

// declarationDigest is the SHA-256 of a file's stripped declaration text.
func declarationDigest(src string) (string, error) {
	code, ok := stripTypeScript(src)
	if !ok {
		return "", fmt.Errorf("the scanner did not finish in code state — an unterminated string, " +
			"template literal or block comment, or likelier a regex literal containing a quote. " +
			"The digest of this file would be meaningless, so it is refused rather than recorded")
	}
	return sha256Hex(code), nil
}

// TestDependedOnTypeFilesCarryThePinnedDeclarationDigest pins the five files this
// adapter transcribes at the level of what they DECLARE.
//
// On a green run this is also what keeps stripTypeScript exercised: the network
// half calls the same function on bytes it fetches, and a helper that only ever
// runs on the failure path of a target nobody runs is a helper nobody has tested.
func TestDependedOnTypeFilesCarryThePinnedDeclarationDigest(t *testing.T) {
	if len(dependedOnDeclarations) == 0 {
		t.Fatal("no files are pinned — a green here would mean nothing")
	}
	for _, path := range sortedKeys(dependedOnDeclarations) {
		want := dependedOnDeclarations[path]
		raw, err := os.ReadFile(filepath.Join(vendoredTypesRoot(), filepath.FromSlash(path))) //nolint:gosec // a vendored path
		if err != nil {
			t.Errorf("reading the vendored %s: %v", path, err)
			continue
		}
		got, err := declarationDigest(string(raw))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if got != want {
			t.Errorf("%s declares a different surface: digest %s, pinned to %s.\n"+
				"Unlike the tree hash, this one does NOT move for a comment edit or a reformat — so a\n"+
				"failure here means a type, a field, an enum member or a literal union actually changed.\n"+
				"Re-read the transcription that feeds off this file before updating the pin.",
				path, got, want)
		}
	}
}

// TestDeclarationDigestIgnoresPresentationButNotNames is the drill on the digest
// itself, and it exists because a normaliser that quietly normalised away the
// thing it was meant to catch would look exactly like a working one.
//
// Both halves are asserted: it must be BLIND to comments and layout, and it must
// NOT be blind to a rename, a retype, a dropped field or a changed enum value.
func TestDeclarationDigestIgnoresPresentationButNotNames(t *testing.T) {
	const base = `// a leading line comment
/**
 * A doc block.
 */
export interface Library {
  id: number;
  name: string;      // trailing comment
  formatPriority: string[];
}

export enum Permission {
  KoboSync = "kobo_sync",
}
`
	baseDigest, err := declarationDigest(base)
	if err != nil {
		t.Fatalf("digesting the base: %v", err)
	}

	same := []struct{ name, src string }{
		{"comments reworded", `// an ENTIRELY different leading comment
/**
 * Some other prose, at considerably greater length than before.
 */
export interface Library {
  id: number;
  name: string;      // a different trailing comment
  formatPriority: string[];
}

export enum Permission {
  KoboSync = "kobo_sync",
}
`},
		{"every comment deleted", `export interface Library {
  id: number;
  name: string;
  formatPriority: string[];
}
export enum Permission {
  KoboSync = "kobo_sync",
}
`},
		{"reindented and rewrapped", "export interface Library { id: number; name: string;\n\tformatPriority:\n\t\tstring[]; }\nexport enum Permission { KoboSync = \"kobo_sync\", }\n"},
	}
	for _, tc := range same {
		got, err := declarationDigest(tc.src)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != baseDigest {
			t.Errorf("%s moved the digest (%s → %s). The digest must be blind to presentation, or "+
				"every upstream reformat reads as a wire change and the alarm becomes noise",
				tc.name, baseDigest, got)
		}
	}

	differ := []struct{ name, src string }{
		{"a field renamed", strings.Replace(base, "formatPriority", "formatPriorities", 1)},
		{"a field retyped", strings.Replace(base, "id: number;", "id: string;", 1)},
		{"a field dropped", strings.Replace(base, "  name: string;      // trailing comment\n", "", 1)},
		{"a field added", strings.Replace(base, "  id: number;", "  id: number;\n  slug: string;", 1)},
		{"an enum VALUE changed", strings.Replace(base, `"kobo_sync"`, `"kobo_sync_v2"`, 1)},
		{"an enum member renamed", strings.Replace(base, "KoboSync =", "KoboSyncing =", 1)},
	}
	for _, tc := range differ {
		if tc.src == base {
			t.Errorf("%s: the substitution did not apply, so this case tests nothing", tc.name)
			continue
		}
		got, err := declarationDigest(tc.src)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got == baseDigest {
			t.Errorf("%s did NOT move the digest. That is the class of change this whole guard "+
				"exists to catch, so a digest blind to it is worse than no digest", tc.name)
		}
	}

	// The refusal path. A regex literal carrying a double quote is a real shape
	// in this vendored tree (src/pattern-resolver.ts:71), not a hypothetical.
	if _, err := declarationDigest("const R = /[\"a-z]/g;\nexport type A = 1;\n"); err == nil {
		t.Error("a file that left the scanner mid-string produced a digest instead of an error; " +
			"a meaningless digest recorded as a pin is exactly how a guard goes quietly wrong")
	}
}
