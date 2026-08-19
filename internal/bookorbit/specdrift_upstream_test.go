//go:build upstream

// The NETWORK half of the BookOrbit type-drift guard. NOT in `make check`.
//
// WHY IT IS BEHIND A TAG. `make check` makes exactly TWO network calls, both to
// vulnerability databases, and `make check-offline` is defined as `check` minus
// those two. A third call would break that contract, and it would break it in the
// worst way: an upstream outage, a rate limit or a plane would turn somebody's
// unrelated commit red. BookOrbit moving a type is NEWS — it is not a reason to
// fail a commit that has nothing to do with it.
//
// So this takes the shape internal/servarr's specdrift_upstream_test.go already
// set, which is `integration`'s and `bench`'s shape: a build tag, its own make
// target (`make spec-drift`), and an env-var opt-in. The offline half — "are the
// vendored bytes still the bytes this package was written against" — is
// TestVendoredBookOrbitTypesAreTheUpstreamTree in vendoredtypes_test.go, and that
// one IS in the gate.
//
// ⚠️ AND IT IS THE LOOSEST LINK IN THE CHAIN, stated here rather than left to be
// discovered. There is no CI in this repo, so nothing runs this on a schedule.
// `make spec-drift` is a thing a person types. The vendored copy plus the offline
// pin are worth having on their own — they make the transcription auditable and
// freeze it against silent local edits — but the "we will hear when upstream
// moves" half is only as real as somebody's habit of running this target.

package bookorbit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// upstreamRepo is read over the git protocol rather than the GitHub REST API on
// purpose: `git ls-remote` and a blobless fetch need no token and no API quota,
// and resolving a path to its TREE name needs the tree objects only — none of the
// 200 KB of TypeScript underneath is downloaded unless something actually moved.
const upstreamRepo = "https://github.com/bookorbit/bookorbit"

// upstreamBranch is where BookOrbit develops. There is no release-tag line to pin
// a floor against — measured 2026-08-19, `git ls-remote --tags` on this repo, and
// the reason ADR-0046's floor/ceiling split has nothing to bite on here even
// setting aside that there is no committed spec.
const upstreamBranch = "refs/heads/main"

// The SPEC_DRIFT_VERDICT lines are a CONTRACT WITH THE MAKEFILE, and they exist
// so `make spec-drift` never states a cause nobody established. The strings are
// deliberately identical to internal/servarr's: the Makefile reads one verdict
// per run and must not care which package produced it.
const (
	// verdictDrift — upstream answered and the tree is not the pinned one. The
	// premise of the vendoring is what changed. This is the news.
	verdictDrift = "SPEC_DRIFT_VERDICT: drift"

	// verdictUnreached — upstream's answer was never obtained: DNS, a proxy, an
	// outage, a rate limit, or git failing locally. NOTHING about upstream is
	// established by this, in either direction.
	verdictUnreached = "SPEC_DRIFT_VERDICT: unreached"

	// verdictPathMoved — upstream answered, but packages/types is no longer at
	// that path on that ref, so the comparison never happened. Also news, and
	// DIFFERENT news from a changed tree.
	verdictPathMoved = "SPEC_DRIFT_VERDICT: path-moved"
)

// TestSpecDriftBookOrbitTypesStillMatchUpstream is the guard that fires the day
// BookOrbit's wire vocabulary moves under this adapter.
//
// THE `TestSpecDrift` PREFIX IS A CONTRACT WITH THE MAKEFILE, not a style choice.
// `make spec-drift` selects on SPEC_DRIFT_PREFIX and then asserts a FLOOR on how
// many tests carrying it actually RAN, so renaming this function away cannot
// leave the target exiting 0 over nothing. Adding this test raised
// SPEC_DRIFT_FLOOR from 1 to 2 in the Makefile; the two must move together.
//
// IT ASKS TWO QUESTIONS, and they are different questions.
//
//	(1) WAS THE VENDORING FAITHFUL? `<pinned commit>:packages/types` must name the
//	    tree the offline pin carries. This is not a tautology: it is the only
//	    thing in the repo that can catch a vendored copy that was subtly wrong
//	    from the day it landed — a partial extraction, a file dropped, a byte
//	    mangled in transit. The offline pin proves the tree has not moved SINCE;
//	    only upstream can prove it was right to begin with.
//	(2) HAS UPSTREAM MOVED SINCE? `main:packages/types` compared to the same
//	    value. This is the drift.
//
// A failure of (1) and a failure of (2) mean opposite things — the first is our
// bug, the second is upstream's news — and they are reported separately for that
// reason.
func TestSpecDriftBookOrbitTypesStillMatchUpstream(t *testing.T) {
	requireOptIn(t)

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	run(ctx, t, verdictUnreached, dir, "git", "init", "--quiet")
	run(ctx, t, verdictUnreached, dir, "git", "remote", "add", "origin", upstreamRepo)

	// --filter=blob:none --depth 1: commits and trees, no file contents.
	// Resolving a path to its tree NAME needs the trees and nothing more.
	run(ctx, t, verdictUnreached, dir, "git", "fetch", "--quiet", "--filter=blob:none", "--depth", "1",
		"origin", vendoredTypesCommit)
	atPin := strings.TrimSpace(run(ctx, t, verdictPathMoved, dir,
		"git", "rev-parse", "FETCH_HEAD:"+vendoredTypesUpstreamPath))
	t.Logf("%s at the pinned commit %s: tree %s", vendoredTypesUpstreamPath, vendoredTypesCommit, atPin)

	if atPin != vendoredTypesTree {
		t.Log(verdictDrift)
		t.Fatalf("THE VENDORED COPY IS NOT WHAT UPSTREAM HAS AT THE COMMIT IT CLAIMS.\n"+
			"  upstream %s at %s is tree %s\n"+
			"  api/specs/bookorbit-types/ is pinned to      %s\n\n"+
			"This is OUR bug, not upstream news, and it is the one failure here that invalidates\n"+
			"everything else this package asserts: every transcription in internal/bookorbit cites\n"+
			"that commit, and the vendored copy is supposed to BE it. Re-vendor from a clean clone:\n"+
			"    git -C <clone> archive %s %s \\\n"+
			"      | tar -x -C api/specs/bookorbit-types --strip-components=2\n"+
			"then regenerate api/specs/bookorbit-types.manifest and update vendoredTypesTree.\n"+
			"Do NOT simply paste the observed tree over the constant — find out which files differ\n"+
			"first, because the answer decides whether any transcription needs re-reading.",
			vendoredTypesUpstreamPath, vendoredTypesCommit, atPin, vendoredTypesTree,
			vendoredTypesCommit, vendoredTypesUpstreamPath)
	}

	run(ctx, t, verdictUnreached, dir, "git", "fetch", "--quiet", "--filter=blob:none", "--depth", "1",
		"origin", upstreamBranch)
	atHead := strings.TrimSpace(run(ctx, t, verdictPathMoved, dir,
		"git", "rev-parse", "FETCH_HEAD:"+vendoredTypesUpstreamPath))
	head := strings.TrimSpace(run(ctx, t, verdictUnreached, dir, "git", "rev-parse", "FETCH_HEAD"))
	t.Logf("%s at %s (%s): tree %s", vendoredTypesUpstreamPath, upstreamBranch, head, atHead)

	if atHead == vendoredTypesTree {
		t.Logf("upstream %s is unchanged since %s — the vendored copy is current",
			vendoredTypesUpstreamPath, vendoredTypesCommit)
		return
	}

	t.Log(verdictDrift)
	t.Errorf("BookOrbit's %s has MOVED.\n"+
		"  vendored (%s): tree %s\n"+
		"  upstream (%s at %s): tree %s\n\n%s\n\n"+
		"THIS IS THE NEWS THIS TEST EXISTS FOR, not a broken build. It is not a gate failure:\n"+
		"this target is not in `make check` and must not be added to it. Handle it as deliberate\n"+
		"work — re-vendor at the new commit, update vendoredTypesCommit, vendoredTypesTree, the\n"+
		"manifest, the dependedOnDeclarations digests and the BookOrbit rows in\n"+
		"api/specs/SOURCES.md, then re-read every transcription that feeds off a file marked\n"+
		"TRANSCRIBED above.",
		vendoredTypesUpstreamPath, vendoredTypesCommit, vendoredTypesTree,
		upstreamBranch, head, atHead, classifyMovedFiles(ctx, dir, head))
}

// classifyMovedFiles is what turns "the tree moved" into something worth reading.
//
// It answers three questions the tree hash alone cannot, in increasing order of
// what they cost:
//
//	WHICH FILES  — from `git ls-tree -r` on each ref. Trees only, no blobs.
//	DO WE CARE   — dependedOnDeclarations names the five files internal/bookorbit
//	               transcribes. A change to any other file is real drift and worth
//	               knowing, but it cannot break this adapter today.
//	DOES IT MEAN ANYTHING — for the files we DO care about, and only those, the
//	               blob is fetched and its declaration digest compared. That is
//	               what separates "they rewrote a doc comment in book.ts" from
//	               "they renamed a field in book.ts", and it is the difference
//	               between an alarm worth reading and one that trains people to
//	               ignore it.
//
// ⚠️ NOTHING IN HERE MAY END THE TEST, and that is not a stylistic preference.
// This function is evaluated as an ARGUMENT to the t.Errorf that carries the
// headline, so a t.Fatalf reached from here runs runtime.Goexit before the
// headline is ever formatted: the drift would be detected and then silently
// swallowed, leaving a bare `--- FAIL` and a stack trace. That is why it uses
// softRun rather than the run helper above, and why every failure below returns
// a note instead of raising one. Found by drilling this path on 2026-08-19, in
// company with a panic on the same line — a diagnosis that can destroy its own
// headline is worse than no diagnosis.
func classifyMovedFiles(ctx context.Context, dir, head string) string {
	want, err := readTypesManifest()
	if err != nil {
		return "  (no per-file diagnosis: the manifest could not be read: " + err.Error() + ")"
	}
	lsOut, err := softRun(ctx, dir, "git", "ls-tree", "-r", head, "--", vendoredTypesUpstreamPath)
	if err != nil {
		return "  (no per-file diagnosis: git ls-tree failed: " + err.Error() + ")"
	}
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(lsOut), "\n") {
		// `<mode> <type> <sha>\t<path>`
		meta, path, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			continue
		}
		got[strings.TrimPrefix(path, vendoredTypesUpstreamPath+"/")] = fields[2]
	}

	var matters, other []string
	add := func(path, note string, transcribed bool) {
		line := "  " + path + "  " + note
		if transcribed {
			matters = append(matters, line)
			return
		}
		other = append(other, line)
	}
	for _, p := range sortedKeys(want) {
		pinned, transcribed := dependedOnDeclarations[p]
		g, present := got[p]
		switch {
		case !present:
			add(p, "REMOVED upstream", transcribed)
		case g != want[p]:
			note := "CHANGED (" + want[p][:12] + " → " + g[:12] + ")"
			// The blob is fetched ONLY for a file this adapter transcribes. Any
			// other file has no pinned digest to compare against, so asking the
			// question would cost a download to answer nothing — and reading the
			// absent pin as a hash is what panicked here before.
			if transcribed {
				note += digestVerdict(ctx, dir, pinned, g)
			}
			add(p, note, transcribed)
		}
	}
	for _, p := range sortedKeys(got) {
		if _, ok := want[p]; !ok {
			_, transcribed := dependedOnDeclarations[p]
			add(p, "ADDED upstream", transcribed)
		}
	}

	var b strings.Builder
	b.WriteString("FILES internal/bookorbit TRANSCRIBES — a change here can break this adapter:\n")
	if len(matters) == 0 {
		b.WriteString("  (none — every file this adapter reads is unchanged. The drift is real but it\n" +
			"   cannot affect the current slice. Re-vendor when convenient rather than urgently.)\n")
	} else {
		b.WriteString(strings.Join(matters, "\n") + "\n")
	}
	b.WriteString("\nEVERYTHING ELSE in packages/types — real drift, no current consumer:\n")
	if len(other) == 0 {
		b.WriteString("  (none)")
	} else {
		b.WriteString(strings.Join(other, "\n"))
	}
	return b.String()
}

// digestVerdict fetches ONE changed blob and says whether its declarations moved
// or only its presentation did. Called only for a file this adapter transcribes,
// so the download is bounded by dependedOnDeclarations rather than by how much
// upstream churned — five files at worst, whatever happened upstream.
//
// pinned is that file's recorded declaration digest, passed in rather than looked
// up again: the caller has already established the file HAS one, and re-reading
// the map here is how an absent pin reached a slice expression and panicked.
func digestVerdict(ctx context.Context, dir, pinned, blob string) string {
	// The blobless fetch left this object absent locally; the partial clone's
	// promisor remote fetches exactly it on demand.
	out, err := softRun(ctx, dir, "git", "cat-file", "blob", blob)
	if err != nil {
		return "  [could not fetch the new bytes, so no declaration verdict: " + err.Error() + "]"
	}
	upstreamDigest, err := declarationDigest(out)
	if err != nil {
		return "  [the declaration digest could not be computed: " + err.Error() + "]"
	}
	if upstreamDigest == pinned {
		return "\n      → DECLARATIONS UNCHANGED. Comments, blank lines or formatting only. The\n" +
			"        transcription that reads this file is still correct; re-vendor at leisure."
	}
	return "\n      → DECLARATIONS CHANGED (" + pinned[:12] + " → " + upstreamDigest[:12] + ").\n" +
		"        A type, a field, an enum member or a literal union moved. RE-READ the\n" +
		"        transcription in internal/bookorbit that feeds off this file before doing\n" +
		"        anything else — this is the case the whole guard was built for."
}

// softRun is run's non-fatal twin, for the diagnosis path. See the warning above
// classifyMovedFiles for why the distinction is load-bearing rather than tidy.
func softRun(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

// requireOptIn mirrors `make test-integration`'s refusal. Running `go test
// -tags=upstream ./...` by hand on a machine with no network should say why it
// declined rather than time out against github.com.
func requireOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("USARR_SPEC_DRIFT") != "1" {
		t.Skip("set USARR_SPEC_DRIFT=1 (or run `make spec-drift`); this test needs the network")
	}
}

// run executes one git command, and takes the verdict to print if it fails.
//
// The verdict is a PARAMETER rather than a constant because the failures are not
// alike: a fetch that never completes says nothing about upstream, while a
// rev-parse that cannot resolve the path says upstream moved the directory.
func run(ctx context.Context, t *testing.T, verdict, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Logged BEFORE the Fatalf: t.Fatalf ends the test, and the make target
		// needs this line in the captured output to know what it may say.
		t.Log(verdict)
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}
