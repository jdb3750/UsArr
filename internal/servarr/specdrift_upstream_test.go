//go:build upstream

// The network half of ADR-0047's guard. NOT in `make check`.
//
// WHY IT IS BEHIND A TAG. `make check` makes exactly TWO network calls, both to
// vulnerability databases, and `make check-offline` is defined as `check` minus
// those two. A third call would break that contract, and it would break it in the
// worst way: an upstream outage, a rate limit or a plane would turn somebody's
// unrelated commit red. Prowlarr regenerating their OpenAPI document is NEWS —
// it is not a reason to fail a commit that has nothing to do with it.
//
// So this follows the shape `integration` and `bench` already use in this repo: a
// build tag, its own make target (`make spec-drift`), and an env-var opt-in. The
// offline half — "are the vendored bytes still the bytes this suite was written
// against" — is TestVendoredSpecIsThePinnedBlob in contract_test.go, and that one
// IS in the gate.

package servarr

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
// purpose: `git ls-remote` needs no token and no API quota, and a blobless fetch
// resolves a path to its blob name without downloading the 145 KB of JSON.
const upstreamRepo = "https://github.com/Prowlarr/Prowlarr"

// specPathInUpstream is where Prowlarr keeps the document vendored as
// api/specs/prowlarr.json.
const specPathInUpstream = "src/Prowlarr.Api.V1/openapi.json"

// ownerRelease is the Prowlarr release the only known real deployment runs.
//
// OWNER-CONFIRMED 2026-08-17: the owner stated it directly, in his own words, of
// his actual install — checked against a human's box, not inferred from a
// changelog. His Prowlarr auto-updates, so this WILL go stale; keep it a one-line
// pin so re-confirming it later is a one-line change.
const ownerRelease = "v2.5.2.5491"

// TestSpecDriftRefsStillShareThePinnedBlob is the guard that fires the day
// ADR-0047's premise changes.
//
// THE `TestSpecDrift` PREFIX IS A CONTRACT WITH THE MAKEFILE, not a style
// choice. `make spec-drift` selects on it and then asserts a FLOOR on how many
// tests carrying it actually ran, so that renaming this function away cannot
// leave the target exiting 0 over nothing. The prefix used to be `TestUpstream`,
// which is an ordinary English word: `-run Upstream` swept in five unrelated
// tests in internal/kavita, internal/releases and internal/store, so three
// packages printed a bare `ok` and looked drift-checked while holding no
// upstream-tagged test at all. A new drift test takes this prefix and raises
// SPEC_DRIFT_FLOOR in the Makefile; nothing else may take it.
//
// The premise: Prowlarr does NOT regenerate its checked-in OpenAPI document per
// release, so the release the owner runs and `develop` carry the byte-identical
// file, and a floor/ceiling split (ADR-0046's answer for Kavita) would vendor the
// same 145 KB twice. Measured 2026-08-17 — last regenerated 2025-06-07
// (60740fa25), 33 release tags since.
//
// That identity is a coincidence of upstream neglect, not a property. When it
// ends, somebody must revisit the split deliberately instead of rediscovering the
// whole problem from scratch, and this is the thing that tells them.
func TestSpecDriftRefsStillShareThePinnedBlob(t *testing.T) {
	requireOptIn(t)

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// --filter=blob:none --no-checkout: fetch commits and trees, no file
	// contents. Resolving a path to its blob NAME needs the tree and nothing
	// more, so this stays small even against a repo this size.
	run(ctx, t, dir, "git", "init", "--quiet")
	run(ctx, t, dir, "git", "remote", "add", "origin", upstreamRepo)

	// Mismatches are COLLECTED and reported once, after the loop. Reporting
	// inside it printed this whole 14-line explanation per diverging ref — and
	// the ordinary way for the premise to die is upstream regenerating the file,
	// which moves BOTH refs at once, so the common failure was two identical
	// walls of text. Reading (c) below also needs both refs in hand to be
	// answerable at all.
	var diverged []string

	for _, ref := range []struct {
		spec, role string
	}{
		{spec: "refs/tags/" + ownerRelease, role: "the FLOOR — the release the owner confirmed running"},
		{spec: "refs/heads/develop", role: "the CEILING — where the API is defined"},
	} {
		run(ctx, t, dir, "git", "fetch", "--quiet", "--filter=blob:none", "--depth", "1", "origin", ref.spec)
		got := strings.TrimSpace(run(ctx, t, dir, "git", "rev-parse", "FETCH_HEAD:"+specPathInUpstream))

		t.Logf("%s (%s): %s is blob %s", ref.spec, ref.role, specPathInUpstream, got)
		if got != vendoredSpecBlob {
			diverged = append(diverged, fmt.Sprintf("  %s (%s) now carries it as blob %s", ref.spec, ref.role, got))
		}
	}

	if len(diverged) > 0 {
		t.Errorf("%s has moved away from the pinned blob on %d of 2 refs; "+
			"api/specs/prowlarr.json is %s.\n%s\n\n"+
			"THIS IS THE NEWS THIS TEST EXISTS FOR, not a broken test. Three readings, in the\n"+
			"order worth checking them:\n"+
			"  (a) LOCAL MISTAKE, NOT UPSTREAM NEWS — check this first. If the gate's\n"+
			"      TestVendoredSpecIsThePinnedBlob is also failing, then vendoredSpecBlob and\n"+
			"      api/specs/prowlarr.json disagree with EACH OTHER and upstream is innocent.\n"+
			"      Fix that first; this test cannot tell you anything until it is fixed.\n"+
			"  (b) UPSTREAM REGENERATED THE SPEC, for the first time since 2025-06-07. Then\n"+
			"      ADR-0047's premise is dead: the floor and the ceiling may now be DIFFERENT\n"+
			"      documents, and the floor/ceiling split ADR-0046 gave Kavita becomes worth\n"+
			"      doing here. Re-vendor, re-read knownSpecDivergences (a regenerated spec is\n"+
			"      exactly when those stop being true), and supersede ADR-0047.\n"+
			"  (c) ONLY ONE REF MOVED — the list above says which, and both log lines above give\n"+
			"      the observed blob. If the tag and develop now disagree, that alone kills the\n"+
			"      one-file argument.\n"+
			"Nothing here is a gate failure: this target is not in `make check` and must not be\n"+
			"added to it. Fix it as a deliberate piece of work, not as a red build.",
			specPathInUpstream, len(diverged), vendoredSpecBlob, strings.Join(diverged, "\n"))
	}
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

func run(ctx context.Context, t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}
