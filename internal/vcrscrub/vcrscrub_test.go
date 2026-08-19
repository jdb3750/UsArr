package vcrscrub_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/jdb3750/UsArr/internal/vcrscrub"
)

// fixtureAuthKey is the probe credential: a Kavita Auth Key's real SHAPE, a
// canonical GUID, GENERATED FRESH ON EVERY RUN and never written down.
//
// Two reasons it is not a literal, and both were measured rather than assumed:
//
//  1. CHOOSING THE PROBE IS PART OF THE CHECK. A recognisable sample credential
//     can be allowlisted by a scanner upstream, and a guard drilled with one
//     then reports clean because the probe was ignored — "proving" the guard off
//     while it is on. A value that has never existed before this process started
//     cannot be on anyone's allowlist.
//  2. A GUID literal assigned to an identifier containing "Key" TRIPS gitleaks'
//     generic-api-key rule under this repo's own .gitleaks.toml — verified:
//     `const fixtureAuthKey = "<a fresh uuid4>"` in a _test.go scanned exit 1,
//     "leaks found: 1". Generating it keeps `make secrets` honest without
//     extending .gitleaks.toml's waiver list, which that file explicitly asks
//     callers not to do.
var fixtureAuthKey = freshGUID()

func freshGUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("no entropy for the probe credential: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// fakeKavita stands in for the two routes that carry the Auth Key in the URL,
// in the TWO POSITIONS it can occupy. Nothing in this file may point at a real
// instance.
//
//   - /api/Image/series-cover?apiKey= — the QUERY position. The live probe that
//     started this work found the Image routes refuse the `x-api-key` header
//     (400) and accept only the query parameter (200), so the fake refuses a
//     keyless request the same way.
//   - /api/Opds/{apiKey}/series — the PATH-SEGMENT position. Kavita's OPDS
//     routes put the key there, where there is no parameter name for any
//     deny-list to match on.
//
// Both are drilled because they fail differently, and one of the two failure
// modes is the dangerous one: `make secrets` CATCHES the query form and MISSES
// the path form (measured — see the note on ScanDir).
func fakeKavita(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/Image/series-cover", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apiKey") == "" {
			// The measured behaviour: header-only is a 400 here, not a 401.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		// A response body that carries a credential BACK, which is the direction
		// the old per-package regexes were not written for.
		_, _ = fmt.Fprintf(w, `{"refreshToken":%q,"next":"/api/Image/series-cover?seriesId=2&apiKey=%s"}`,
			fixtureAuthKey, fixtureAuthKey)
	})
	mux.HandleFunc("/api/Opds/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = fmt.Fprint(w, `<feed/>`)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// coverURL and opdsURL are the two request shapes under drill.
func coverURL(base string) string {
	return base + "/api/Image/series-cover?seriesId=1&apiKey=" + fixtureAuthKey
}

func opdsURL(base string) string {
	return base + "/api/Opds/" + fixtureAuthKey + "/series"
}

// fetchBoth drives one request per credential POSITION through doer.
func fetchBoth(t *testing.T, doer *http.Client, base string) {
	t.Helper()
	for _, u := range []string{coverURL(base), opdsURL(base)} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		// Sent as well, because a real client sends both and the header must be
		// scrubbed too.
		req.Header.Set("x-api-key", fixtureAuthKey)
		resp, err := doer.Do(req)
		if err != nil {
			t.Fatalf("fetch %s: %v", u, err)
		}
		if _, err := io.ReadAll(resp.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d for %s — the fake refused the credential shape under test", resp.StatusCode, u)
		}
	}
}

func readCassette(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path + ".yaml")
	if err != nil {
		t.Fatalf("reading cassette: %v", err)
	}
	return string(b)
}

// TestScrubDrillArmed records a real interaction against a local fake and proves
// the credential never reaches the file. It is the positive half of the drill
// docs/DEVELOPMENT.md §11 asks for; TestScrubDrillNeutered is the other half and
// the two must be read together.
func TestScrubDrillArmed(t *testing.T) {
	t.Setenv("USARR_RECORD", "1")
	srv := fakeKavita(t)
	path := filepath.Join(t.TempDir(), "armed")

	rec, err := vcrscrub.New(path)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	fetchBoth(t, rec.GetDefaultClient(), srv.URL)
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	got := readCassette(t, path)
	if strings.Contains(got, fixtureAuthKey) {
		t.Fatalf("SCRUBBER FAILED — the Auth Key reached the cassette:\n%s", got)
	}
	for _, want := range []string{
		"apiKey=REDACTED",                          // the request URL, QUERY position
		"/api/Opds/REDACTED/series",                // the request URL, PATH position
		"X-Api-Key:\n                - <redacted>", // the request header
		`"refreshToken":"<redacted>"`,              // the response body, JSON field
		`apiKey=<redacted>`,                        // the response body, URL fragment
	} {
		if !strings.Contains(got, want) {
			t.Errorf("cassette is missing %q; full text:\n%s", want, got)
		}
	}
	t.Logf("ARMED — recorded cassette:\n%s", got)
}

// TestScrubDrillNeutered records the same interaction with the hook removed and
// asserts the credential DOES land in the file.
//
// It is not a curiosity. A guard that has never been fired is indistinguishable
// from no guard, and the only way to know Scrub is doing the work — rather than
// the client happening not to send the key, or go-vcr happening not to persist
// it — is to watch the leak happen with the hook off. Keeping it as a test means
// the demonstration is repeated on every run instead of living in one commit
// message.
//
// The cassette it writes goes to t.TempDir, never to testdata/cassettes.
func TestScrubDrillNeutered(t *testing.T) {
	srv := fakeKavita(t)
	path := filepath.Join(t.TempDir(), "neutered")

	rec, err := recorder.New(path,
		recorder.WithMode(recorder.ModeRecordOnce),
		// Everything vcrscrub.New installs EXCEPT the BeforeSave hook.
		recorder.WithMatcher(vcrscrub.Matcher),
		recorder.WithSkipRequestLatency(true),
	)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	fetchBoth(t, rec.GetDefaultClient(), srv.URL)
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	got := readCassette(t, path)
	for _, leak := range []string{
		"apiKey=" + fixtureAuthKey,          // QUERY position
		"/api/Opds/" + fixtureAuthKey + "/", // PATH position
		`"refreshToken":"` + fixtureAuthKey, // response body
	} {
		if !strings.Contains(got, leak) {
			t.Fatalf("the neutered run did NOT leak %q, so the armed run proves nothing about it:\n%s", leak, got)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, fixtureAuthKey) {
			t.Logf("NEUTERED — leaked line: %s", strings.TrimSpace(line))
		}
	}
}

// TestScrubbedCassetteStillReplays is the design constraint, checked by
// execution rather than by argument: record with the hook armed, then replay the
// scrubbed file with a client that sends the REAL key, and require a match.
//
// It passes because Matcher redacts the incoming URL with the same function
// Scrub used on the stored one. Comparing raw URLs would fail here, and the only
// way to make raw comparison work would be to store the credential.
func TestScrubbedCassetteStillReplays(t *testing.T) {
	srv := fakeKavita(t)
	path := filepath.Join(t.TempDir(), "roundtrip")

	t.Setenv("USARR_RECORD", "1")
	rec, err := vcrscrub.New(path)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	fetchBoth(t, rec.GetDefaultClient(), srv.URL)
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Replay. The fake is still up, so a miss would be invisible without this.
	t.Setenv("USARR_RECORD", "")
	replay, err := vcrscrub.New(path)
	if err != nil {
		t.Fatalf("replay recorder: %v", err)
	}
	if replay.Mode() != recorder.ModeReplayOnly {
		t.Fatalf("expected replay-only, got %v", replay.Mode())
	}
	fetchBoth(t, replay.GetDefaultClient(), srv.URL)
	if err := replay.Stop(); err != nil {
		t.Fatalf("replay stop: %v", err)
	}

	// And prove the raw-URL matcher the rest of the tree used would NOT have
	// matched, so the change of matcher is load-bearing and not cosmetic.
	c, err := cassette.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for n, live := range []string{coverURL(srv.URL), opdsURL(srv.URL)} {
		stored := c.Interactions[n].Request.URL
		if stored == live {
			t.Fatalf("stored URL still equals the live one, so nothing was scrubbed: %q", stored)
		}
		if vcrscrub.RedactURL(stored) != vcrscrub.RedactURL(live) {
			t.Fatalf("redacted URLs disagree:\n stored %q\n live   %q", vcrscrub.RedactURL(stored), vcrscrub.RedactURL(live))
		}
		t.Logf("stored %q vs live %q — unequal raw, equal redacted", stored, live)
	}
}

// TestRedactBodyAsksTheSharedDenyList covers the names the three deleted
// per-package regexes did NOT have. Each of these was redacted by the logging
// path and not by the recording path before this package existed.
func TestRedactBodyAsksTheSharedDenyList(t *testing.T) {
	for _, name := range []string{"passkey", "torrent_pass", "rsskey", "authkey", "signature", "secret", "auth_token", "refreshToken", "APIKEY"} {
		t.Run(name, func(t *testing.T) {
			q := vcrscrub.RedactBody("https://x/rss?" + name + "=" + fixtureAuthKey)
			if strings.Contains(q, fixtureAuthKey) {
				t.Errorf("query form not redacted: %s", q)
			}
			j := vcrscrub.RedactBody(`{"` + name + `":"` + fixtureAuthKey + `"}`)
			if strings.Contains(j, fixtureAuthKey) {
				t.Errorf("JSON form not redacted: %s", j)
			}
		})
	}
	// A non-credential parameter must survive, or a cassette stops being evidence.
	if got := vcrscrub.RedactBody(`{"query":"dune","seriesId":"12"}`); got != `{"query":"dune","seriesId":"12"}` {
		t.Errorf("over-redacted: %s", got)
	}
}

// TestRedactURLHandlesAPathSegmentGUID covers Kavita's OPDS shape, where the key
// has no parameter name for any deny-list to match on.
func TestRedactURLHandlesAPathSegmentGUID(t *testing.T) {
	got := vcrscrub.RedactURL("http://kavita.test:5000/api/opds/" + fixtureAuthKey + "/series")
	if strings.Contains(got, fixtureAuthKey) {
		t.Fatalf("OPDS path-segment key survived: %s", got)
	}
	if want := "http://kavita.test:5000/api/opds/REDACTED/series"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestCassettesOnDiskCarryNoCredential is the backstop. It scans the committed
// cassettes, so a cassette recorded with the hook bypassed is caught by the
// suite rather than by a person reading a diff.
func TestCassettesOnDiskCarryNoCredential(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "cassettes")
	findings, err := vcrscrub.ScanDir(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
	if len(findings) > 0 {
		t.Fatalf("%d suspected credential(s) in committed cassettes", len(findings))
	}
}

// TestScanFiresOnAPlantedCredential fires the backstop, both tiers, against the
// two shapes that actually threaten this repository.
func TestScanFiresOnAPlantedCredential(t *testing.T) {
	tests := []struct {
		name string
		text string
		tier int
	}{
		{
			// The exact line the Kavita cover fetch produces.
			name: "tier1 query parameter",
			text: "        url: http://kavita.test:5000/api/Image/series-cover?apiKey=" + fixtureAuthKey + "&seriesId=1\n",
			tier: 1,
		},
		{
			name: "tier1 JSON field",
			text: `              "refreshToken": "` + fixtureAuthKey + `",` + "\n",
			tier: 1,
		},
		{
			// No name anywhere: the OPDS path segment.
			name: "tier2 bare path segment",
			text: "        url: http://kavita.test:5000/api/opds/" + fixtureAuthKey + "/series\n",
			tier: 2,
		},
		{
			name: "tier2 32-hex *Arr key in a comment",
			text: "# recorded with key 0123456789abcdef0123456789abcdef\n",
			tier: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := vcrscrub.ScanText("planted.yaml", tc.text)
			if len(got) == 0 {
				t.Fatalf("scan found NOTHING in %q — the backstop is not a backstop", tc.text)
			}
			var tiers []int
			for _, f := range got {
				tiers = append(tiers, f.Tier)
			}
			found := false
			for _, x := range tiers {
				if x == tc.tier {
					found = true
				}
			}
			if !found {
				t.Fatalf("wanted a tier-%d finding, got %v (%v)", tc.tier, tiers, got)
			}
			t.Logf("fired: %v", got)
		})
	}

	// The two legitimate hex blobs already in the committed cassettes must NOT
	// fire, or the backstop is noise and someone will switch it off.
	for _, ok := range []string{
		`              "infoHash": "d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0",`,
		`              "installId": "5a4f2b1c-9e83-4d17-a6b0-2c7d1e4f8a90",`,
	} {
		if got := vcrscrub.ScanText("ok.yaml", ok); len(got) != 0 {
			t.Errorf("false positive on %q: %v", ok, got)
		}
	}
}

// TestModeReadsUSARRRecord pins the switch docs/DEVELOPMENT.md §7.2 documents.
// Before this package it was read by internal/config and by no recorder at all.
func TestModeReadsUSARRRecord(t *testing.T) {
	for v, want := range map[string]recorder.Mode{
		"":      recorder.ModeReplayOnly,
		"0":     recorder.ModeReplayOnly,
		"no":    recorder.ModeReplayOnly,
		"1":     recorder.ModeRecordOnce,
		" TRUE": recorder.ModeRecordOnce,
		"on":    recorder.ModeRecordOnce,
	} {
		t.Setenv("USARR_RECORD", v)
		if got := vcrscrub.Mode(); got != want {
			t.Errorf("USARR_RECORD=%q: mode %v, want %v", v, got, want)
		}
	}
}

// TestBinaryDoesNotLinkTheRecorder answers the objection that used to justify
// copying the scrubber into each package rather than sharing it — that a shared
// scrubber "would put a go-vcr dependency in the shipped binary".
//
// It asks the go tool what ./cmd/usarr actually links, rather than reasoning
// about it. `go list -deps` is hermetic given a populated module cache.
func TestBinaryDoesNotLinkTheRecorder(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go tool on PATH: %v", err)
	}
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "../../cmd/usarr").CombinedOutput() //nolint:gosec // fixed args
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, forbidden := range []string{"go-vcr", "internal/vcrscrub"} {
		if strings.Contains(string(out), forbidden) {
			t.Errorf("./cmd/usarr transitively links %q", forbidden)
		}
	}
}

// A compile-time reminder that Scrub satisfies go-vcr's hook signature, so a
// go-vcr upgrade that changes it fails here rather than at the next recording.
var _ recorder.HookFunc = vcrscrub.Scrub

var _ = cassette.Interaction{}

// ─── The camelCase gap ───────────────────────────────────────────────────────
//
// Everything above drills the QUERY and PATH positions with Kavita's shapes. The
// tests below drill a third shape that neither covered: a credential returned as
// a camelCase JSON FIELD whose lowercased form was not on the deny-list.
//
// ssrf.isCredentialParam lowercases the incoming name and looks it up, so an
// underscored entry only covers its camelCase spelling if the UNDERSCORE-FREE
// form is in the map too. `refresh_token` had `refreshtoken` beside it and
// `access_token` did NOT have `accesstoken` — so `refreshToken` was redacted and
// `accessToken` was not. A one-entry difference between two adjacent lines,
// invisible unless you spell each name out and do the lookup by hand.
//
// The gap is real in both directions, and neither is hypothetical:
//
//   - BookOrbit — v0.1's catalogue source under ADR-0052 — returns its JWT as
//     `{"accessToken": …}`. Verified from source: `login()` and
//     `issueTokensForUser()` in `server/src/modules/auth/auth.service.ts` on
//     `bookorbit/bookorbit@main` both return `{ accessToken, user }`, and
//     `POST auth/login`, `POST auth/refresh` and `POST auth/magic-links/login`
//     all land on one of them.
//   - Kavita's own VENDORED SPEC already carries the name: `MalUserInfoDto`
//     lists `accessToken` as required (`api/specs/kavita-v0.9.0.2.json`), and
//     that is a MyAnimeList OAuth token. So a cassette in this tree could have
//     captured the shape before BookOrbit was chosen, not only after.

// fixtureJWT is the probe: a JWT's real SHAPE — three unpadded base64url
// segments — GENERATED FRESH ON EVERY RUN, for both of the reasons
// fixtureAuthKey is.
//
// Unpadded is load-bearing. A `=` in the value would let queryPairRe see a
// `name=value` pair INSIDE the token and rewrite it for the wrong reason, and
// the armed half would then pass without the deny-list entry doing any work.
var fixtureJWT = freshJWT()

func freshJWT() string {
	seg := func(n int) string {
		b := make([]byte, n)
		if _, err := rand.Read(b); err != nil {
			panic("no entropy for the probe credential: " + err.Error())
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return seg(24) + "." + seg(48) + "." + seg(32)
}

// fakeBookOrbit serves the one response shape under drill. Nothing in this file
// may point at a real instance.
//
// The sibling `username` is the CONTROL. Without it a pass would show only that
// the JWT is absent, which blanket-redacting the whole body would also achieve —
// and a cassette that redacts everything has stopped being a fixture.
func fakeBookOrbit(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"accessToken":%q,"user":{"id":1,"username":"joe"}}`, fixtureJWT)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// login drives one POST through doer, the way a headless client would.
func login(t *testing.T, doer *http.Client, base string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		base+"/api/auth/login", strings.NewReader(`{"username":"joe","password":"pw"}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d — the fake refused the shape under test", resp.StatusCode)
	}
}

// TestAccessTokenDrillArmed is the positive half: record a real login against a
// local fake and require the JWT never to reach the file.
//
// It was FIRED AGAINST THE UNFIXED DENY-LIST before `accesstoken` was added, and
// reported the JWT verbatim in the recorded cassette. That failure is what makes
// the pass afterwards evidence rather than an assumption — the gap was found by
// reading redact.go, but it was confirmed by watching it leak.
func TestAccessTokenDrillArmed(t *testing.T) {
	t.Setenv("USARR_RECORD", "1")
	srv := fakeBookOrbit(t)
	path := filepath.Join(t.TempDir(), "armed-accesstoken")

	rec, err := vcrscrub.New(path)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	login(t, rec.GetDefaultClient(), srv.URL)
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	got := readCassette(t, path)
	if strings.Contains(got, fixtureJWT) {
		t.Fatalf("SCRUBBER FAILED — the accessToken JWT reached the cassette:\n%s", got)
	}
	if want := `"accessToken":"<redacted>"`; !strings.Contains(got, want) {
		t.Errorf("cassette is missing %q; full text:\n%s", want, got)
	}
	if want := `"username":"joe"`; !strings.Contains(got, want) {
		t.Errorf("over-redacted — the benign sibling %q did not survive:\n%s", want, got)
	}
	t.Logf("ARMED — recorded cassette:\n%s", got)
}

// TestAccessTokenDrillNeutered is the other half, in the shape
// TestScrubDrillNeutered established: record the same interaction with the
// BeforeSave hook removed, and require the JWT to land in the file.
//
// It rules out the boring explanation for the armed half's silence — that go-vcr
// declined to persist this body at all, or that the fake never sent it — and so
// keeps the demonstration running on every `go test` rather than living once in
// a commit message.
func TestAccessTokenDrillNeutered(t *testing.T) {
	srv := fakeBookOrbit(t)
	path := filepath.Join(t.TempDir(), "neutered-accesstoken")

	rec, err := recorder.New(path,
		recorder.WithMode(recorder.ModeRecordOnce),
		// Everything vcrscrub.New installs EXCEPT the BeforeSave hook.
		recorder.WithMatcher(vcrscrub.Matcher),
		recorder.WithSkipRequestLatency(true),
	)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	login(t, rec.GetDefaultClient(), srv.URL)
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	got := readCassette(t, path)
	if leak := `"accessToken":"` + fixtureJWT; !strings.Contains(got, leak) {
		t.Fatalf("the neutered run did NOT leak %q, so the armed run proves nothing about it:\n%s", leak, got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, fixtureJWT) {
			t.Logf("NEUTERED — leaked line: %s", strings.TrimSpace(line))
		}
	}
}

// TestRedactBodyCoversCamelCaseSpellings sweeps the whole CLASS rather than the
// one name that was reported, which is the difference between fixing a bug and
// closing a gap.
//
// Every deny-list entry carrying an underscore has a camelCase spelling a JSON
// API would plausibly emit, and the lowercased lookup covers it only if the
// underscore-free form is in the map too. Three of the six underscored entries
// were missing theirs — `access_token`, `auth_token` and `secret_key`. Pinning
// the class here means the next underscored name added without its sibling fails
// a test, instead of waiting for someone to spell the names out by hand again.
func TestRedactBodyCoversCamelCaseSpellings(t *testing.T) {
	for _, name := range []string{
		"accessToken",  // access_token  — BookOrbit's login JWT; Kavita's MalUserInfoDto
		"authToken",    // auth_token
		"secretKey",    // secret_key
		"apiKey",       // api_key       — already covered; pinned so it stays covered
		"refreshToken", // refresh_token — already covered; pinned so it stays covered
		"torrentPass",  // torrent_pass  — already covered; pinned so it stays covered
	} {
		t.Run(name, func(t *testing.T) {
			j := vcrscrub.RedactBody(`{"` + name + `":"` + fixtureJWT + `"}`)
			if strings.Contains(j, fixtureJWT) {
				t.Errorf("JSON form not redacted: %s", j)
			}
			q := vcrscrub.RedactBody("https://x/cb?" + name + "=" + fixtureJWT)
			if strings.Contains(q, fixtureJWT) {
				t.Errorf("query form not redacted: %s", q)
			}
		})
	}
}
