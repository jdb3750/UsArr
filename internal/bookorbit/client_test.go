package bookorbit

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- construction ---------------------------------------------------------

// TestNewRequiresAnInjectedHTTPClient fires the rule Options.HTTPClient's
// comment states. There is no fallback to http.DefaultClient, because a fallback
// would silently bypass internal/ssrf's resolve-then-pin — and the user
// configures an arbitrary internal URL here, which is exactly the SSRF-by-design
// shape docs/reference/security.md §2 is written for.
func TestNewRequiresAnInjectedHTTPClient(t *testing.T) {
	_, err := New(Options{BaseURL: testBaseURL, MagicLinkToken: testMagicLink})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v; there must be no fallback to http.DefaultClient", err)
	}
}

func TestNewRejectsBadBaseURLs(t *testing.T) {
	f := newFake(t)
	for _, tc := range []struct{ name, url string }{
		{"empty", ""},
		{"no scheme", "bookorbit.test:3000"},
		{"ftp", "ftp://bookorbit.test"},
		{"no host", "http://"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Options{BaseURL: tc.url, HTTPClient: f.srv.Client()})
			if !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("err = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

// TestBaseURLDropsUserinfo: a base URL with credentials in it would otherwise be
// rendered into every stored path, every log line and every error.
func TestBaseURLDropsUserinfo(t *testing.T) {
	f := newFake(t)
	c, err := New(Options{
		BaseURL:    "http://admin:hunter2@bookorbit.test:3000/proxy/",
		HTTPClient: f.srv.Client(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.BaseURL(); got != "http://bookorbit.test:3000/proxy" {
		t.Errorf("BaseURL() = %q; userinfo and the trailing slash must be gone", got)
	}
	if strings.Contains(c.BaseURL(), "hunter2") {
		t.Fatal("the base URL still carries userinfo")
	}
}

// ---- where the credential travels -----------------------------------------

// TestCredentialTravelsInHeadersAndOneBodyOnly is ADR-0052's load-bearing
// constraint, fired.
//
// The magic-link token may appear in EXACTLY ONE place — the body of
// POST /api/v1/auth/magic-links/login — and the access token in EXACTLY ONE —
// the Authorization header. Neither may reach a query string, because
// jwt.strategy.ts never reads one and because a credential in a URL is a
// credential in every upstream access log.
func TestCredentialTravelsInHeadersAndOneBodyOnly(t *testing.T) {
	f := newFake(t)
	c := f.client(t)

	if _, err := c.AppInfo(context.Background()); err != nil {
		t.Fatalf("AppInfo: %v", err)
	}

	reqs := f.requests()
	if len(reqs) != 2 {
		t.Fatalf("saw %d requests, want 2 (the mint and the read): %+v", len(reqs), reqs)
	}
	login, read := reqs[0], reqs[1]

	if login.Path != magicLinkLoginPath {
		t.Errorf("first request went to %q, want %q", login.Path, magicLinkLoginPath)
	}
	if !strings.Contains(login.Body, testMagicLink) {
		t.Errorf("the magic-link token did not reach the login body: %q", login.Body)
	}
	if login.Auth != "" {
		t.Errorf("the mint carried a bearer (%q); the route is @Public() and a second credential has no business there", login.Auth)
	}

	tok := f.latestToken()
	if read.Auth != authScheme+tok {
		t.Errorf("read carried Authorization %q, want the bearer", read.Auth)
	}
	if !strings.Contains(read.Path, apiPrefix) {
		t.Errorf("read path %q is not under %q; ADR-0052 constrains this adapter to /api/v1", read.Path, apiPrefix)
	}

	for _, r := range reqs {
		assertNoCredential(t, "query string of "+r.Path, r.Query, tok)
		assertNoCredential(t, "path of "+r.Path, r.Path, tok)
	}
	if strings.Contains(read.Body, testMagicLink) {
		t.Error("the magic-link token reached a body other than the login's")
	}
}

// ---- minting, caching and re-minting --------------------------------------

// TestTokenIsCachedAcrossCalls: a client that minted per request would burn its
// 10-per-minute budget on the eleventh read.
func TestTokenIsCachedAcrossCalls(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	ctx := context.Background()

	for i := range 5 {
		if _, err := c.AppInfo(ctx); err != nil {
			t.Fatalf("AppInfo #%d: %v", i, err)
		}
	}
	if got := f.mintCount(); got != 1 {
		t.Errorf("minted %d times for 5 reads, want 1 — the login route is throttled at 10/minute", got)
	}
}

// TestExpiringTokenIsReMintedBeforeUse fires the refreshSkew rule: a token
// inside refreshSkew of expiry is replaced BEFORE it is sent, so an ordinary
// read never pays for a 401 it could have avoided.
func TestExpiringTokenIsReMintedBeforeUse(t *testing.T) {
	f := newFake(t)
	// Shorter than refreshSkew, so the token is stale the instant it arrives.
	f.tokenTTL = 30 * time.Second
	c := f.client(t)
	ctx := context.Background()

	if _, err := c.AppInfo(ctx); err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	if _, err := c.AppInfo(ctx); err != nil {
		t.Fatalf("AppInfo #2: %v", err)
	}
	if got := f.mintCount(); got != 2 {
		t.Errorf("minted %d times, want 2 — a token inside refreshSkew of expiry must be replaced before use", got)
	}
}

// TestUnexpectedUnauthorizedReMintsAndRetriesOnce is the case doc.go warns
// about: a full import that outlives its access token gets a 401 mid-walk. The
// retry is the client's, so no caller has to remember it.
func TestUnexpectedUnauthorizedReMintsAndRetriesOnce(t *testing.T) {
	f := newFake(t)
	// The token does not look stale to the client — it claims 15 minutes — but
	// the server rejects anything but the newest, which is what an expiry
	// disagreement or a tokenVersion bump looks like from here.
	f.acceptOnlyLatest = true
	c := f.client(t)
	ctx := context.Background()

	if _, err := c.AppInfo(ctx); err != nil {
		t.Fatalf("first AppInfo: %v", err)
	}

	// Force the server to invalidate the client's held token by issuing a newer
	// one behind its back.
	f.defaultLogin(discardWriter{}, mustLoginRequest(t))

	info, err := c.AppInfo(ctx)
	if err != nil {
		t.Fatalf("AppInfo after the token went stale: %v", err)
	}
	if info.Version == "" {
		t.Error("the retry returned an empty AppInfo")
	}

	var reads int
	for _, r := range f.requests() {
		if r.Path == apiPrefix+"/app-info" {
			reads++
		}
	}
	if reads != 3 {
		t.Errorf("app-info was requested %d times, want 3 (ok, 401, retried ok)", reads)
	}
}

// TestPersistentUnauthorizedIsNotRetriedForever: exactly one re-mint, then the
// 401 is the answer. A loop here would hammer a throttled route.
func TestPersistentUnauthorizedIsNotRetriedForever(t *testing.T) {
	f := newFake(t)
	f.appInfoHandler = func(w http.ResponseWriter, r *http.Request) {
		nestErrorText(w, r, http.StatusUnauthorized, "Unauthorized")
	}
	c := f.client(t)

	_, err := c.AppInfo(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if got := f.mintCount(); got != 2 {
		t.Errorf("minted %d times, want 2 (the first mint plus exactly one re-mint)", got)
	}
	var reads int
	for _, r := range f.requests() {
		if r.Path == apiPrefix+"/app-info" {
			reads++
		}
	}
	if reads != 2 {
		t.Errorf("app-info was requested %d times, want 2 (the original and one retry)", reads)
	}
}

// TestConcurrentCallersMintOnce fires the single-flight rule. Twenty concurrent
// readers each noticing an absent token and minting would spend the whole
// 10-per-minute budget in one tick and then 429 the import that needed it.
func TestConcurrentCallersMintOnce(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = c.AppInfo(ctx)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}
	if got := f.mintCount(); got != 1 {
		t.Errorf("20 concurrent readers produced %d mints, want 1", got)
	}
}

// TestNoCredentialConfiguredIsItsOwnError. Health still works, because
// HealthController is @Public(); everything else fails with a sentence that
// names the fix.
func TestNoCredentialConfiguredIsItsOwnError(t *testing.T) {
	f := newFake(t)
	c := f.client(t, func(o *Options) { o.MagicLinkToken = "" })
	ctx := context.Background()

	if _, err := c.Health(ctx); err != nil {
		t.Errorf("Health needs no credential and failed: %v", err)
	}
	_, err := c.AppInfo(ctx)
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("err = %v, want ErrNoCredential", err)
	}
}

// TestBadMagicLinkSaysWhatBookOrbitWillNotSay pins auth.go's message
// substitution: a 401 on the login route must reach the caller as a sentence
// that names no single cause. It cannot name one — parseErrorBody maps every
// 401 to the single ErrUnauthorized sentinel and branches on nothing, unlike
// its 403 arm, which splits one status in two on an exact message match.
//
// This comment used to say loginWithToken collapses five distinct causes into
// one message "deliberately". The enumeration is doc.go's, read from
// bookorbit@73b7877d and not re-checkable from this tree; upstream's intent is
// UNKNOWN, so the word is dropped rather than hedged.
func TestBadMagicLinkSaysWhatBookOrbitWillNotSay(t *testing.T) {
	f := newFake(t)
	c := f.client(t, func(o *Options) { o.MagicLinkToken = "9876543210zyxwvutsrq" })

	_, err := c.AppInfo(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if !strings.Contains(err.Error(), "BookOrbit does not say which") {
		t.Errorf("the error hides the ambiguity BookOrbit's own code creates: %v", err)
	}
}

// ---- error classification --------------------------------------------------

func TestNestErrorEnvelopeIsParsed(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		handler http.HandlerFunc
		want    error
		check   func(*testing.T, *APIError)
	}{
		{
			name:   "403 password change required is its own sentinel",
			status: http.StatusForbidden,
			handler: func(w http.ResponseWriter, r *http.Request) {
				nestErrorText(w, r, http.StatusForbidden, passwordChangeRequiredMessage)
			},
			want: ErrPasswordChangeRequired,
		},
		{
			name:   "any other 403 is plain forbidden",
			status: http.StatusForbidden,
			handler: func(w http.ResponseWriter, r *http.Request) {
				nestErrorText(w, r, http.StatusForbidden, "Forbidden resource")
			},
			want: ErrForbidden,
		},
		{
			name:   "429 is throttling, not a server error",
			status: http.StatusTooManyRequests,
			handler: func(w http.ResponseWriter, r *http.Request) {
				nestErrorText(w, r, http.StatusTooManyRequests, "ThrottlerException: Too Many Requests")
			},
			want: ErrThrottled,
		},
		{
			name:   "400 with an ARRAY message keeps every complaint",
			status: http.StatusBadRequest,
			handler: func(w http.ResponseWriter, r *http.Request) {
				nestErrorJSON(w, r, http.StatusBadRequest, []string{"property foo should not exist", "token must be a string"})
			},
			want: ErrValidation,
			check: func(t *testing.T, e *APIError) {
				if len(e.Validation) != 2 {
					t.Errorf("Validation = %#v, want both complaints — Nest's `message` is a string[] here", e.Validation)
				}
			},
		},
		{
			name:   "500 is a server error",
			status: http.StatusInternalServerError,
			handler: func(w http.ResponseWriter, r *http.Request) {
				nestErrorText(w, r, http.StatusInternalServerError, "Internal server error")
			},
			want: ErrServer,
		},
		{
			name:   "an empty body degrades to the status alone",
			status: http.StatusBadGateway,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			},
			want: ErrServer,
		},
		{
			name:   "a reverse proxy's HTML is not JSON and must not be invented into one",
			status: http.StatusBadGateway,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
			},
			want: ErrServer,
			check: func(t *testing.T, e *APIError) {
				if !strings.Contains(e.Message, "502 Bad Gateway") {
					t.Errorf("Message = %q; the only diagnostic in the body was dropped", e.Message)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			f.appInfoHandler = tc.handler
			c := f.client(t)

			_, err := c.AppInfo(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err is %T, want *APIError", err)
			}
			if apiErr.Status != tc.status {
				t.Errorf("Status = %d, want %d", apiErr.Status, tc.status)
			}
			if tc.check != nil {
				tc.check(t, apiErr)
			}
		})
	}
}

// TestErrorBodyIsRedactedAndBounded fires the two jobs clean() does, in one
// test, because skipping either is the same defect: the error string reaches
// slog AND service_instance.last_error.
func TestErrorBodyIsRedactedAndBounded(t *testing.T) {
	f := newFake(t)
	f.appInfoHandler = func(w http.ResponseWriter, r *http.Request) {
		// An upstream that quotes the URL it failed on, with a credential in it.
		// BookOrbit's own envelope echoes request.url back in `path`, so an error
		// message carrying a URL is the DEFAULT shape here, not an edge case.
		long := strings.Repeat("A", 4000)
		nestErrorText(w, r, http.StatusBadGateway,
			"upstream fetch failed for https://provider.example/v1/thing?apiKey="+testMagicLink+" "+long)
	}
	c := f.client(t)

	_, err := c.AppInfo(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	assertNoCredential(t, "APIError.Error()", err.Error())
	if !strings.Contains(err.Error(), "apiKey=REDACTED") {
		t.Errorf("the credential was not redacted in place: %v", err)
	}
	if len(err.Error()) > 1024 {
		t.Errorf("the error is %d bytes; a multi-kilobyte upstream body must be bounded before it reaches a log line", len(err.Error()))
	}
}

// TestNothingLogsTheCredential. The client logs a WARN on an over-scoped
// account, which is the one log line in this package that mentions the
// credential's OWNER; it must never mention the credential.
func TestNothingLogsTheCredential(t *testing.T) {
	f := newFake(t)
	f.user.IsSuperuser = true
	f.user.Permissions = []string{"*"}

	var buf bytes.Buffer
	var mu sync.Mutex
	c := f.client(t, func(o *Options) {
		o.Logger = slog.New(slog.NewTextHandler(&lockedWriter{w: &buf, mu: &mu}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})

	if _, err := c.AppInfo(context.Background()); err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	mu.Lock()
	logged := buf.String()
	mu.Unlock()

	if logged == "" {
		t.Fatal("nothing was logged; the over-scope warning is the thing under test")
	}
	assertNoCredential(t, "the log", logged, f.latestToken())
	if !strings.Contains(logged, "SUPERUSER") {
		t.Errorf("the §14 warning did not name the finding:\n%s", logged)
	}
}

// ---- the breaker -----------------------------------------------------------

// TestBreakerIgnores4xxAndTripsOn5xx is recordStatus's rule, fired.
func TestBreakerIgnores4xxAndTripsOn5xx(t *testing.T) {
	for _, tc := range []struct {
		status    int
		wantState BreakerState
	}{
		{http.StatusBadRequest, BreakerClosed},
		{http.StatusForbidden, BreakerClosed},
		{http.StatusNotFound, BreakerClosed},
		{http.StatusTooManyRequests, BreakerClosed},
		{http.StatusInternalServerError, BreakerOpen},
		{http.StatusServiceUnavailable, BreakerOpen},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			f := newFake(t)
			f.appInfoHandler = func(w http.ResponseWriter, r *http.Request) {
				nestErrorText(w, r, tc.status, http.StatusText(tc.status))
			}
			c := f.client(t)
			for range 6 {
				_, _ = c.AppInfo(context.Background())
			}
			if got := c.Breaker().State(); got != tc.wantState {
				t.Errorf("after 6×%d the breaker is %v, want %v", tc.status, got, tc.wantState)
			}
		})
	}
}

// TestBreakerDefaultsMatchTheArchitectureNumbers pins §7.5 from THIS package, so
// a change in internal/breaker fails here as well as in internal/kavita and
// internal/servarr.
func TestBreakerDefaultsMatchTheArchitectureNumbers(t *testing.T) {
	cfg := BreakerConfig{}.WithDefaults()
	if cfg.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5 (ARCHITECTURE.md §7.5)", cfg.FailureThreshold)
	}
	if cfg.BaseCooldown != 5*time.Second {
		t.Errorf("BaseCooldown = %v, want 5s", cfg.BaseCooldown)
	}
	if cfg.MaxCooldown != 15*time.Minute {
		t.Errorf("MaxCooldown = %v, want 15m", cfg.MaxCooldown)
	}
	if cfg.JitterFraction != 0.2 {
		t.Errorf("JitterFraction = %v, want 0.2", cfg.JitterFraction)
	}
}

// TestBreakerOpenNeverReachesTheNetwork.
func TestBreakerOpenIsNotEvidenceAboutTheUpstream(t *testing.T) {
	f := newFake(t)
	f.appInfoHandler = func(w http.ResponseWriter, r *http.Request) {
		nestErrorText(w, r, http.StatusInternalServerError, "boom")
	}
	c := f.client(t)
	for range 6 {
		_, _ = c.AppInfo(context.Background())
	}
	before := len(f.requests())

	_, err := c.AppInfo(context.Background())
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("err = %v, want ErrBreakerOpen", err)
	}
	if len(f.requests()) != before {
		t.Error("the refused call still went out; an open breaker must not reach the network")
	}
}

// TestPolicyRefusalDoesNotTripTheBreaker. An internal/ssrf refusal never reached
// the network, so it is a configuration bug rather than upstream flakiness.
func TestPolicyRefusalDoesNotTripTheBreaker(t *testing.T) {
	errPolicy := errors.New("ssrf: blocked address")
	f := newFake(t)
	c := f.client(t, func(o *Options) {
		o.HTTPClient = doerFunc(func(*http.Request) (*http.Response, error) { return nil, errPolicy })
		o.IsPolicyError = func(err error) bool { return errors.Is(err, errPolicy) }
	})

	for range 10 {
		_, err := c.AppInfo(context.Background())
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("err = %v, want ErrInvalidRequest", err)
		}
	}
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("breaker is %v after 10 policy refusals; the request never left the process, "+
			"so it is UsArr's bug and says nothing about the instance", got)
	}
}

// TestMintAndReadShareOneBreakerProbe is why the breaker gate lives in
// roundTrip. An authenticated call can be TWO exchanges; gating the call instead
// of the exchange would consume the half-open probe on the mint and then refuse
// the very request it was minted for, leaving a recovered instance stuck open.
func TestMintAndReadShareOneBreakerProbe(t *testing.T) {
	f := newFake(t)
	fail := true
	f.appInfoHandler = func(w http.ResponseWriter, r *http.Request) {
		if fail {
			nestErrorText(w, r, http.StatusInternalServerError, "boom")
			return
		}
		f.defaultAppInfo(w, r)
	}
	now := time.Now()
	c := f.client(t, func(o *Options) {
		o.Now = func() time.Time { return now }
		o.Rand = func() float64 { return 0.5 } // no jitter
	})

	for range 6 {
		_, _ = c.AppInfo(context.Background())
	}
	if got := c.Breaker().State(); got != BreakerOpen {
		t.Fatalf("breaker = %v, want open", got)
	}

	// Walk past the cooldown so the next call gets the half-open probe, and make
	// the instance healthy again. The client must recover in ONE call even
	// though that call needs a fresh mint first.
	fail = false
	c.invalidate()
	now = now.Add(time.Minute)

	if _, err := c.AppInfo(context.Background()); err != nil {
		t.Fatalf("a recovered instance did not recover in one call: %v", err)
	}
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("breaker = %v after a successful call, want closed", got)
	}
}

// ---- health ----------------------------------------------------------------

func TestHealthNeedsNoCredentialAndReportsIndicators(t *testing.T) {
	f := newFake(t)
	c := f.client(t, func(o *Options) { o.MagicLinkToken = "" })

	report, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !report.OK() {
		t.Errorf("report = %+v, want ok", report)
	}
	if len(report.Indicators) != 1 || report.Indicators[0].Name != "database" {
		t.Errorf("Indicators = %+v, want the one database indicator", report.Indicators)
	}
	for _, r := range f.requests() {
		if r.Auth != "" {
			t.Errorf("the health probe carried a credential (%q); the route is @Public()", r.Auth)
		}
	}
}

// TestHealth503IsADiagnosisNotAFailure. Terminus answers 503 with a
// fully-formed body naming the indicator that is down. Collapsing that into
// ErrServer would throw away the one thing §17.3's "what is broken and how to
// fix it" needs — and tripping the breaker on it would silence the probe that
// explains the outage.
func TestHealth503IsADiagnosisNotAFailure(t *testing.T) {
	f := newFake(t)
	f.healthHandler = func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "error",
			"info":   map[string]any{},
			"error": map[string]any{
				"database": map[string]any{"status": "down", "message": "connect ECONNREFUSED 127.0.0.1:5432"},
			},
			"details": map[string]any{
				"database": map[string]any{"status": "down", "message": "connect ECONNREFUSED 127.0.0.1:5432"},
			},
		})
	}
	c := f.client(t)

	report, err := c.Health(context.Background())
	if !errors.Is(err, ErrUnhealthy) {
		t.Fatalf("err = %v, want ErrUnhealthy", err)
	}
	if errors.Is(err, ErrServer) {
		t.Error("a 503 from the health route was classified as an upstream server error; " +
			"the host is up and has told us exactly what is wrong")
	}
	if got := report.Failing(); len(got) != 1 || got[0].Name != "database" {
		t.Fatalf("Failing() = %+v, want the database indicator", got)
	}
	if !strings.Contains(err.Error(), "ECONNREFUSED") {
		t.Errorf("the diagnosis did not survive into the error: %v", err)
	}
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("breaker = %v; a 503 from the health probe must not silence the probe", got)
	}
}

// ---- app-info --------------------------------------------------------------

func TestAppInfoIsTheAuthenticatedHandshake(t *testing.T) {
	f := newFake(t)
	c := f.client(t)

	info, err := c.AppInfo(context.Background())
	if err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	if info.Version != "v1.4.2" {
		t.Errorf("Version = %q, want v1.4.2", info.Version)
	}
	if !info.VersionKnown() {
		t.Error("VersionKnown() = false for a real version")
	}
	if info.MaxUploadSizeMB != 200 {
		t.Errorf("MaxUploadSizeMB = %d, want 200", info.MaxUploadSizeMB)
	}
	if c.Version() != "v1.4.2" {
		t.Errorf("Version() = %q; AppInfo is the only thing that populates it", c.Version())
	}
}

// TestLocalBuildIsUnknownNotTooOld. config/config.ts resolves app.version as
// `process.env.APP_VERSION ?? 'Local build'`, so a developer's own instance
// reports that literal — and a version-floor check that read it as a version
// would lock out exactly the person most likely to be running one.
func TestLocalBuildIsUnknownNotTooOld(t *testing.T) {
	f := newFake(t)
	f.appInfoHandler = func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			nestErrorText(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"version": LocalBuildVersion, "updateAvailable": nil, "latestVersion": nil, "maxUploadSizeMb": 50,
		})
	}
	c := f.client(t)

	info, err := c.AppInfo(context.Background())
	if err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	if info.VersionKnown() {
		t.Errorf("VersionKnown() = true for %q; it must read as UNKNOWN, never as too old", info.Version)
	}
	if c.Version() != "" {
		t.Errorf("Version() = %q; a Local build must not be cached as a version", c.Version())
	}
}

// ---- the JWT expiry read ---------------------------------------------------

// TestTokenExpiryIsAScheduleNotAnAuthorisation. The signature is never checked
// and cannot be — the HS256 secret is BookOrbit's. So every malformed, forged or
// missing `exp` must degrade to a conservative schedule rather than to an error
// or to a token held forever.
func TestTokenExpiryIsAScheduleNotAnAuthorisation(t *testing.T) {
	f := newFake(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	c := f.client(t, func(o *Options) { o.Now = func() time.Time { return now } })

	valid := freshJWT(now.Add(15 * time.Minute))
	for _, tc := range []struct {
		name string
		tok  string
		want time.Time
	}{
		{"a real exp is honoured", valid, now.Add(15 * time.Minute)},
		{"not a JWT at all", "nonsense", now.Add(fallbackTokenTTL)},
		{"the scrubbed placeholder a cassette carries", RedactedPlaceholder, now.Add(fallbackTokenTTL)},
		{"three segments, undecodable payload", "a.!!!.c", now.Add(fallbackTokenTTL)},
		{"no exp claim", threeSegment(`{"sub":1}`), now.Add(fallbackTokenTTL)},
		{"an exp already in the past", threeSegment(`{"exp":1}`), now.Add(fallbackTokenTTL)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := c.tokenExpiry(tc.tok)
			if !got.Equal(tc.want.UTC()) {
				t.Errorf("tokenExpiry = %v, want %v", got, tc.want.UTC())
			}
		})
	}
}

// ---- helpers ---------------------------------------------------------------

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

type discardWriter struct{}

func (discardWriter) Header() http.Header         { return http.Header{} }
func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriter) WriteHeader(int)             {}

// threeSegment builds a token with a real JWT SHAPE and the given claims, so a
// malformed-claims case is distinguishable from a malformed-token one.
func threeSegment(claims string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".sig"
}

func mustLoginRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, testBaseURL+magicLinkLoginPath,
		strings.NewReader(`{"token":"`+testMagicLink+`"}`))
	if err != nil {
		t.Fatalf("building the out-of-band login: %v", err)
	}
	return req
}
