package bookorbit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/vcrscrub"
)

// ⚠️ THE BOOKORBIT CASSETTES IN testdata/cassettes ARE SYNTHETIC. Every one was
// hand-authored from BookOrbit's own source at
// bookorbit/bookorbit@73b7877d2fede2221b0ca360af9bfced7c3797f3
// ("i18n(client): sync translations from Crowdin (#1036)", 2026-08-18) — the
// controllers, the DTOs, packages/types, @nestjs/terminus's result shape and
// GlobalExceptionFilter's envelope. NONE was captured off a wire, and no live
// BookOrbit was contacted: ADR-0052 records that this project has never probed
// one, and inventing a capture would be worse than saying so.
//
// A SYNTHETIC CASSETTE PROVES THIS CLIENT'S PARSING, NOT THE SERVER'S BEHAVIOUR.
// It cannot discover that a middleware arrives in a different case, that a
// controller enforces something the source does not appear to, or that a field
// is always null in practice. Those are what a live probe is for.
//
// WHAT STANDS BEHIND THEM INSTEAD, and what does not:
//
//   - There is NO vendored OpenAPI document to pin. BookOrbit builds its
//     document at RUNTIME (server/src/swagger.ts) and main.ts mounts it only
//     when SWAGGER_ENABLED is true, which parseBooleanFlag defaults to FALSE.
//     There is no committed spec file in the repository. ADR-0046's
//     floor/ceiling split therefore does not transfer — it needs a committed
//     document regenerated per release, and there is neither. ADR-0047's
//     blob-identity pin DOES transfer, to source files instead of a spec:
//     72207f8 vendored packages/types under api/specs/bookorbit-types/ and
//     pinned it by git's own tree name. (This bullet used to say neither
//     transferred; corrected 2026-08-19, see scope_test.go's header.)
//   - The substitutes are that vendored pin plus the enum-completeness guard in
//     scope_test.go, on the pattern internal/libsync already uses, plus the
//     behavioural tests against the local fake in client_test.go.
//   - Every behavioural fact this client is built on was read from BookOrbit's
//     CONTROLLERS and SERVICES at the pinned commit, per DEVELOPMENT.md §5's rule
//     that the controller wins over a schema.
//
// Re-record against a real BookOrbit with USARR_RECORD=1, which switches
// internal/vcrscrub's recorder to ModeRecordOnce and lets the scrubbing hook do
// its job for real.

func cassettePath(name string) string {
	return filepath.Join("..", "..", "testdata", "cassettes", name)
}

// newCassetteClient builds a Client wired to a cassette.
func newCassetteClient(t *testing.T, cassetteName string) *Client {
	t.Helper()

	rec, err := vcrscrub.New(cassettePath(cassetteName))
	if err != nil {
		t.Fatalf("opening cassette %s: %v", cassetteName, err)
	}
	t.Cleanup(func() {
		if err := rec.Stop(); err != nil {
			t.Errorf("stopping recorder: %v", err)
		}
	})

	c, err := New(Options{
		BaseURL:        testBaseURL,
		MagicLinkToken: testMagicLink,
		HTTPClient:     rec.GetDefaultClient(),
		AppVersion:     "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

func TestHealthFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_health")

	report, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !report.OK() {
		t.Errorf("report = %+v, want ok", report)
	}
	if len(report.Indicators) != 1 || report.Indicators[0].Status != "up" {
		t.Errorf("Indicators = %+v", report.Indicators)
	}
}

func TestUnhealthyFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_health_database_down")

	report, err := c.Health(context.Background())
	if !errors.Is(err, ErrUnhealthy) {
		t.Fatalf("err = %v, want ErrUnhealthy", err)
	}
	if got := report.Failing(); len(got) != 1 || got[0].Name != "database" {
		t.Fatalf("Failing() = %+v", got)
	}
	if !strings.Contains(err.Error(), "ECONNREFUSED") {
		t.Errorf("the indicator's message did not survive into the error: %v", err)
	}
}

// TestMintThenReadFromCassette walks the whole slice-0 path against recorded
// bytes: the mint, the §14 verdict computed from the login body, and the one
// authenticated read.
//
// The cassette's accessToken is `<redacted>` — see the fixture's header — so
// this also pins the client's tolerance of it: mint requires the field to be
// NON-EMPTY, not well-formed, and tokenExpiry degrades to fallbackTokenTTL for
// anything it cannot decode.
func TestMintThenReadFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_app_info")
	ctx := context.Background()

	verdict, err := c.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !verdict.Minimal() {
		t.Errorf("the fixture's correctly scoped shared account produced findings: %v", verdict.Findings)
	}
	acct, ok := c.Account()
	if !ok {
		t.Fatal("no account after a successful mint")
	}
	if acct.Username != "usarr" || acct.ProvisioningMethod != provisioningShared {
		t.Errorf("Account = %+v", acct)
	}

	info, err := c.AppInfo(ctx)
	if err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	if info.Version != "v1.4.2" || !info.VersionKnown() {
		t.Errorf("AppInfo = %+v", info)
	}
}

func TestRejectedMagicLinkFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_magic_link_rejected")

	_, err := c.Authenticate(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	assertNoCredential(t, "the rejected-login error", err.Error())
	if !strings.Contains(err.Error(), "BookOrbit does not say which") {
		t.Errorf("the five-causes-one-message ambiguity was not reported: %v", err)
	}
}

// TestRecordingALoginLeaksNothing is THE DRILL, and it is a real recording
// rather than an assertion about one.
//
// It sets USARR_RECORD=1, drives one live magic-link login against the local
// fake through internal/vcrscrub's recorder, and then reads the bytes that
// reached disk. Both credentials are in play in one exchange and in opposite
// directions — the magic-link token goes out in a REQUEST body, the JWT comes
// back in a RESPONSE body — which is the pairing that made this endpoint worth
// drilling rather than reasoning about.
//
// The cassette is written into t.TempDir() and never committed. That is
// deliberate: this test's subject is the recording path, not a fixture.
func TestRecordingALoginLeaksNothing(t *testing.T) {
	t.Setenv("USARR_RECORD", "1")
	f := newFake(t)
	path := filepath.Join(t.TempDir(), "bookorbit-login-drill")

	rec, err := vcrscrub.New(path)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	c, err := New(Options{
		BaseURL:        f.srv.URL,
		MagicLinkToken: testMagicLink,
		HTTPClient:     rec.GetDefaultClient(),
		AppVersion:     "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	if _, err := c.AppInfo(context.Background()); err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	if err := rec.Stop(); err != nil {
		t.Fatalf("stopping recorder: %v", err)
	}

	raw, err := os.ReadFile(path + ".yaml")
	if err != nil {
		t.Fatalf("reading the recorded cassette: %v", err)
	}
	got := string(raw)

	if strings.Contains(got, testMagicLink) {
		t.Fatalf("SCRUBBER FAILED — the magic-link token reached the cassette:\n%s", got)
	}
	if jwt := f.latestToken(); strings.Contains(got, jwt) {
		t.Fatalf("SCRUBBER FAILED — the accessToken JWT reached the cassette:\n%s", got)
	}
	if want := `"accessToken":"<redacted>"`; !strings.Contains(got, want) {
		t.Errorf("the recorded cassette is missing %q; full text:\n%s", want, got)
	}
	if want := `"token":"<redacted>"`; !strings.Contains(got, want) {
		t.Errorf("the recorded cassette is missing %q; full text:\n%s", want, got)
	}
	// The control. Without a benign sibling surviving, a pass would show only
	// that the secrets are absent — which blanket-wiping the body would also
	// achieve, and a cassette that redacts everything has stopped being a
	// fixture.
	if want := `"username":"usarr"`; !strings.Contains(got, want) {
		t.Errorf("over-redacted — the benign sibling %q did not survive:\n%s", want, got)
	}
	// And the Authorization header on the read that followed.
	if !strings.Contains(got, vcrscrub.Placeholder) {
		t.Errorf("nothing in the cassette is redacted at all:\n%s", got)
	}

	// A cassette written by this drill must also satisfy the on-disk scanner
	// that guards the committed ones. Running it here is what makes the two
	// mechanisms agree about the same bytes.
	if findings := vcrscrub.ScanText(filepath.Base(path)+".yaml", got); len(findings) > 0 {
		for _, fi := range findings {
			t.Errorf("on-disk scan flagged the freshly recorded cassette: %s", fi)
		}
	}
}

// TestCommittedBookOrbitCassettesCarryNoCredential is the on-disk backstop
// applied to this package's own fixtures.
//
// internal/vcrscrub already scans the whole directory, so this is a second
// assertion about the same bytes — and it is here anyway because the failure it
// guards is "someone adds a bookorbit_*.yaml and the shared test is not the one
// they run".
func TestCommittedBookOrbitCassettesCarryNoCredential(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "cassettes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var seen int
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "bookorbit_") {
			continue
		}
		seen++
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // fixture path, test-only
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for _, fi := range vcrscrub.ScanText(e.Name(), string(b)) {
			t.Errorf("%s", fi)
		}
	}
	if seen == 0 {
		t.Fatal("scanned 0 bookorbit cassettes — a green here would mean nothing")
	}
	t.Logf("scanned %d bookorbit cassettes", seen)
}

// TestCassetteModeIsReplayByDefault. A suite that silently switched to recording
// would try to reach a host that does not exist, and the failure would look like
// a parsing bug.
func TestCassetteModeIsReplayByDefault(t *testing.T) {
	t.Setenv("USARR_RECORD", "")
	c := newCassetteClient(t, "bookorbit_health")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Health(ctx); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
}
