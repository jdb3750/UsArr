package kavita

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---- a doer that answers from the test, so headers can be asserted ----------

// fakeDoer answers every request from fn and records what it was asked. The
// cassettes prove the shapes; this proves the wire discipline — which header went
// out, which did not, and what the client did with the answer.
type fakeDoer struct {
	fn   func(*http.Request) (*http.Response, error)
	seen []*http.Request
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.seen = append(f.seen, req)
	return f.fn(req)
}

func respond(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newFakeClient(t *testing.T, fn func(*http.Request) (*http.Response, error)) (*Client, *fakeDoer) {
	t.Helper()
	d := &fakeDoer{fn: fn}
	c, err := New(Options{
		BaseURL:    testBaseURL,
		APIKey:     testAuthKey,
		HTTPClient: d,
		AppVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c, d
}

// ---- construction ----------------------------------------------------------

// TestNewRefusesToBuildItsOwnTransport is the SSRF seam, asserted rather than
// commented. A client that falls back to http.DefaultClient bypasses the
// per-instance policy in internal/ssrf entirely, and the failure is invisible:
// everything works, against every host on the network.
func TestNewRefusesToBuildItsOwnTransport(t *testing.T) {
	_, err := New(Options{BaseURL: testBaseURL, APIKey: testAuthKey})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("New with no HTTPClient returned %v; it must refuse", err)
	}
	if !strings.Contains(err.Error(), "SSRF") {
		t.Errorf("the refusal must say why: %q", err)
	}
}

func TestNewStripsCredentialsAndJunkFromTheBaseURL(t *testing.T) {
	d := &fakeDoer{fn: func(*http.Request) (*http.Response, error) { return respond(200, "Ok", nil), nil }}
	c, err := New(Options{
		BaseURL:    "http://user:hunter2@kavita.test:5000/kavita/?x=1#frag",
		APIKey:     testAuthKey,
		HTTPClient: d,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.BaseURL(); got != "http://kavita.test:5000/kavita" {
		t.Errorf("BaseURL() = %q; userinfo, query and fragment must all be gone", got)
	}
	assertNoCredential(t, "BaseURL()", c.BaseURL())
	if strings.Contains(c.BaseURL(), "hunter2") {
		t.Errorf("BaseURL() still carries userinfo: %q", c.BaseURL())
	}
}

func TestNewRejectsANonHTTPScheme(t *testing.T) {
	d := &fakeDoer{fn: func(*http.Request) (*http.Response, error) { return respond(200, "", nil), nil }}
	for _, raw := range []string{"file:///etc/passwd", "gopher://kavita.test:5000", "", "://nope"} {
		if _, err := New(Options{BaseURL: raw, HTTPClient: d}); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("New(%q) returned %v; want ErrInvalidRequest", raw, err)
		}
	}
}

// ---- the auth header -------------------------------------------------------

// TestAuthKeyGoesInTheHeaderAndNeverTheQuery pins doc.go's central claim against
// the actual bytes: one header, on every authenticated call, and no query-string
// credential anywhere except the one endpoint that has no other way.
func TestAuthKeyGoesInTheHeaderAndNeverTheQuery(t *testing.T) {
	bodies := map[string]string{
		"/api/Health":                  "Ok",
		"/api/Server/server-info-slim": `{"installId":"i","kavitaVersion":"0.9.0.20"}`,
		"/api/Library/libraries":       `[]`,
		"/api/Plugin/version":          `"0.9.0.20"`,
	}
	c, d := newFakeClient(t, func(r *http.Request) (*http.Response, error) {
		return respond(200, bodies[r.URL.Path], nil), nil
	})

	ctx := context.Background()
	if err := c.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if _, err := c.ServerInfo(ctx); err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}
	if _, err := c.Libraries(ctx); err != nil {
		t.Fatalf("Libraries: %v", err)
	}
	if _, err := c.PluginVersion(ctx); err != nil {
		t.Fatalf("PluginVersion: %v", err)
	}

	if len(d.seen) != 4 {
		t.Fatalf("expected 4 requests, saw %d", len(d.seen))
	}
	for _, r := range d.seen {
		path := r.URL.Path

		// The header, on everything except the anonymous probe.
		if path == "/api/Health" {
			if got := r.Header.Get(authHeader); got != "" {
				t.Errorf("GET /api/Health sent %s=%q; it is the [AllowAnonymous] probe and must be "+
					"answerable with no key, or it cannot tell 'host down' from 'key wrong'", authHeader, got)
			}
		} else if got := r.Header.Get(authHeader); got != testAuthKey {
			t.Errorf("%s sent %s=%q, want the Auth Key", path, authHeader, got)
		}

		// The query string, on everything except the one endpoint with no header
		// path.
		if path != "/api/Plugin/version" && r.URL.RawQuery != "" && strings.Contains(r.URL.RawQuery, testAuthKey) {
			t.Errorf("%s put the Auth Key in the query string: %q", path, r.URL.RawQuery)
		}

		// UsArr must not be attributed as a Kavita UI client.
		if got := r.Header.Get("X-Kavita-Client"); got != "" {
			t.Errorf("%s sent X-Kavita-Client=%q; that header is Kavita's own client identifier "+
				"and participates in device tracking", path, got)
		}
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "UsArr/") {
			t.Errorf("%s sent User-Agent=%q", path, ua)
		}
	}
}

// TestPluginVersionURLIsRedacted fires the one credential-in-a-URL this client
// can produce, through the redactor that has to catch it.
func TestPluginVersionURLIsRedacted(t *testing.T) {
	var raw string
	c, _ := newFakeClient(t, func(r *http.Request) (*http.Response, error) {
		raw = r.URL.String()
		return respond(200, `"0.9.0.20"`, nil), nil
	})
	if _, err := c.PluginVersion(context.Background()); err != nil {
		t.Fatalf("PluginVersion: %v", err)
	}
	if !strings.Contains(raw, testAuthKey) {
		t.Fatalf("the endpoint takes the key in the query; the request should carry it: %q", raw)
	}
	assertNoCredential(t, "RedactURL of the PluginVersion URL", RedactURL(raw))
}

func TestPluginVersionRefusesWithNoKey(t *testing.T) {
	d := &fakeDoer{fn: func(*http.Request) (*http.Response, error) {
		t.Error("PluginVersion made a request with no Auth Key to put in the query")
		return respond(200, `""`, nil), nil
	}}
	c, err := New(Options{BaseURL: testBaseURL, HTTPClient: d})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.PluginVersion(context.Background()); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("PluginVersion with no key returned %v; want ErrInvalidRequest", err)
	}
}

// ---- the error taxonomy ----------------------------------------------------

func TestErrorTaxonomy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
		msg    string
	}{
		{
			name:   "401 has an empty body and must not be parsed",
			status: 401, body: "", want: ErrUnauthorized, msg: "bad, missing or expired Auth Key",
		},
		{
			// The failure this package has a sentinel for that the *Arr client does
			// not: a VALID key whose account is not an admin.
			name: "403 is not 401", status: 403, body: "", want: ErrForbidden,
		},
		{
			name: "400 ValidationProblemDetails", status: 400,
			body: `{"type":"https://tools.ietf.org/html/rfc9110#section-15.5.1","title":"One or more validation errors occurred.","status":400,"errors":{"sortOptions.sortField":["The field sortField is invalid."]}}`,
			want: ErrValidation, msg: "One or more validation errors occurred.",
		},
		{
			// Kavita's own handlers return BadRequest(localisedString), which
			// serialises as a bare quoted JSON string rather than an envelope.
			name: "400 bare localised string", status: 400,
			body: `"Series does not exist"`, want: ErrValidation, msg: "Series does not exist",
		},
		{name: "404", status: 404, body: "", want: ErrNotFound},
		{name: "500", status: 500, body: `{"title":"boom"}`, want: ErrServer, msg: "boom"},
		{name: "418", status: 418, body: "", want: ErrUnexpectedStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
				return respond(tc.status, tc.body, nil), nil
			})
			_, err := c.Libraries(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error is not an *APIError: %v", err)
			}
			if apiErr.Status != tc.status {
				t.Errorf("Status = %d, want %d", apiErr.Status, tc.status)
			}
			if tc.msg != "" && !strings.Contains(apiErr.Message, tc.msg) {
				t.Errorf("Message = %q, want it to contain %q", apiErr.Message, tc.msg)
			}
			if strings.Contains(err.Error(), "http://") {
				t.Errorf("the error carries a URL, which for this client can carry a key: %q", err)
			}
		})
	}
}

func TestValidationFailuresAreFlattenedIntoTheError(t *testing.T) {
	c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return respond(400, `{"title":"One or more validation errors occurred.","errors":{"sortOptions.sortField":["The field sortField is invalid."]}}`, nil), nil
	})
	_, err := c.Libraries(context.Background())
	if !strings.Contains(err.Error(), "sortOptions.sortField The field sortField is invalid.") {
		t.Errorf("the field-level complaint is not in the message: %q", err)
	}
}

// TestTransportErrorNeverCarriesTheURL is the redaction rule that made
// transportError reconstruct rather than wrap. net/http returns *url.Error, whose
// Error() prints the full request URL — and for this client that URL can be
// PluginVersion's, with the Auth Key in it.
func TestTransportErrorNeverCarriesTheURL(t *testing.T) {
	c, _ := newFakeClient(t, func(r *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: errors.New("dial tcp: connection refused")}
	})
	_, err := c.PluginVersion(context.Background())
	if err == nil {
		t.Fatal("expected a transport error")
	}
	assertNoCredential(t, "transport error", err.Error())
	if !strings.Contains(err.Error(), "apiKey=REDACTED") {
		t.Errorf("the URL should survive redacted rather than being dropped, so the user can see which "+
			"call failed: %q", err)
	}
	if !errors.Is(err, ErrServer) {
		t.Errorf("a refused dial should classify as ErrServer, got %v", err)
	}
}

func TestPolicyRefusalIsNotUpstreamFlakiness(t *testing.T) {
	policyErr := errors.New("ssrf: destination is not the pinned host")
	d := &fakeDoer{fn: func(*http.Request) (*http.Response, error) { return nil, policyErr }}
	c, err := New(Options{
		BaseURL: testBaseURL, APIKey: testAuthKey, HTTPClient: d,
		IsPolicyError: func(e error) bool { return errors.Is(e, policyErr) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for range 10 {
		if _, err := c.Libraries(context.Background()); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("a policy refusal must be ErrInvalidRequest, got %v", err)
		}
	}
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("breaker is %v after 10 policy refusals; the request never left the process, so it is "+
			"a configuration bug and not evidence about the upstream", got)
	}
}

// ---- the breaker policy ----------------------------------------------------

// TestBreakerIgnores4xxAndTripsOn5xx is the rule stated in recordStatus, fired.
// A 403 from a non-admin key must not open the circuit on an instance that is
// perfectly healthy — that would take the whole instance down over a role.
func TestBreakerIgnores4xxAndTripsOn5xx(t *testing.T) {
	for _, tc := range []struct {
		status    int
		wantState BreakerState
	}{
		{403, BreakerClosed},
		{401, BreakerClosed},
		{400, BreakerClosed},
		{404, BreakerClosed},
		{500, BreakerOpen},
		{503, BreakerOpen},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
				return respond(tc.status, "", nil), nil
			})
			for range 5 {
				_, _ = c.Libraries(context.Background())
			}
			if got := c.Breaker().State(); got != tc.wantState {
				t.Errorf("after 5×%d the breaker is %v, want %v", tc.status, got, tc.wantState)
			}
		})
	}
}

func TestBreakerDefaultsMatchTheArchitectureNumbers(t *testing.T) {
	// WithDefaults is exported since the state machine moved to
	// internal/breaker; this test still lives HERE so that a change to the §7.5
	// numbers fails every client's suite rather than one shared one.
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

func TestBreakerOpenIsNotEvidenceAboutTheUpstream(t *testing.T) {
	calls := 0
	c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		return respond(500, "", nil), nil
	})
	for range 5 {
		_, _ = c.Libraries(context.Background())
	}
	before := calls
	_, err := c.Libraries(context.Background())
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("got %v, want ErrBreakerOpen", err)
	}
	if calls != before {
		t.Error("the refused call still went out; an open breaker must not reach the network")
	}
}

// ---- the buffered bound ----------------------------------------------------

// TestBufferedBoundFailsRatherThanTruncating fires the bound do enforces. It must
// produce ErrResponseTooLarge and NOT a decode error: a truncated body decodes as
// malformed JSON and gets diagnosed as an upstream bug.
func TestBufferedBoundFailsRatherThanTruncating(t *testing.T) {
	c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       io.NopCloser(io.LimitReader(neverEndingArray{}, maxBufferedBody+1024)),
		}, nil
	})
	_, err := c.Libraries(context.Background())
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("got %v, want ErrResponseTooLarge", err)
	}
	if errors.Is(err, ErrDecode) {
		t.Error("the bound was reported as a decode failure, which is the misdiagnosis it exists to prevent")
	}
}

// neverEndingArray emits a JSON array that never closes.
type neverEndingArray struct{}

func (neverEndingArray) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// ---- version capture -------------------------------------------------------

// TestVersionIsEmptyUntilSomethingAsks pins the difference from an *Arr. Kavita
// sends no version header, so "" here means "not asked yet" and must never be
// rendered as "this Kavita has no version".
func TestVersionIsEmptyUntilSomethingAsks(t *testing.T) {
	c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		// A response carrying every header an *Arr would have used.
		return respond(200, `[]`, http.Header{"X-Application-Version": []string{"9.9.9"}}), nil
	})
	if got := c.Version(); got != "" {
		t.Fatalf("Version() = %q before any call", got)
	}
	if _, err := c.Libraries(context.Background()); err != nil {
		t.Fatalf("Libraries: %v", err)
	}
	if got := c.Version(); got != "" {
		t.Errorf("Version() = %q after an ordinary call. Kavita sends no version header — a client that "+
			"picked one up here would be reading something else's", got)
	}
}

// ---- cassette-backed endpoint shapes ---------------------------------------

func TestHealthReplaysAnonymously(t *testing.T) {
	c := newTestClient(t, "kavita_health")
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestServerInfoParsesAndCachesTheVersion(t *testing.T) {
	c := newTestClient(t, "kavita_server_info_slim")
	info, err := c.ServerInfo(context.Background())
	if err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}
	if info.KavitaVersion != testKavitaVersion {
		t.Errorf("KavitaVersion = %q, want %q", info.KavitaVersion, testKavitaVersion)
	}
	if !info.IsDocker {
		t.Error("isDocker did not decode")
	}
	if info.FirstInstallDate.IsZero() || info.FirstInstallDate.Time().Year() != 2025 {
		t.Errorf("firstInstallDate did not decode: %v", info.FirstInstallDate)
	}
	if !info.FirstInstallDate.HasZone() {
		t.Error("firstInstallDate arrived with a zone in this fixture; HasZone() must say so")
	}
	if got := c.Version(); got != testKavitaVersion {
		t.Errorf("Version() = %q after ServerInfo; it must cache what it was told", got)
	}
}

// TestServerInfoOnANonAdminKeyIsForbiddenNotUnauthorized is the distinction the
// whole ErrForbidden sentinel exists for.
func TestServerInfoOnANonAdminKeyIsForbiddenNotUnauthorized(t *testing.T) {
	c := newTestClient(t, "kavita_server_info_forbidden")
	_, err := c.ServerInfo(context.Background())
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Error("a non-admin key was reported as a bad credential; the user would be told to re-enter a " +
			"key that is fine")
	}
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("a 403 opened the breaker (%v); the instance is healthy, the account is not an admin", got)
	}
}

func TestLibrariesIsTheCredentialHandshake(t *testing.T) {
	c := newTestClient(t, "kavita_libraries")
	libs, err := c.Libraries(context.Background())
	if err != nil {
		t.Fatalf("Libraries: %v", err)
	}
	if len(libs) != 2 {
		t.Fatalf("got %d libraries, want 2", len(libs))
	}
	if libs[0].Type != LibraryTypeManga || libs[1].Type != LibraryTypeBook {
		t.Errorf("library types did not decode: %v, %v", libs[0].Type, libs[1].Type)
	}
	if len(libs[0].Folders) != 1 {
		t.Errorf("folders did not decode: %v", libs[0].Folders)
	}
}

func TestLibrariesOnABadKeyIsUnauthorized(t *testing.T) {
	c := newTestClient(t, "kavita_libraries_unauthorized")
	_, err := c.Libraries(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
	if errors.Is(err, ErrDecode) {
		t.Error("the empty 401 body was parsed; parseErrorBody must short-circuit before any decode")
	}
}

func TestPluginVersionReplays(t *testing.T) {
	c := newTestClient(t, "kavita_plugin_version")
	v, err := c.PluginVersion(context.Background())
	if err != nil {
		t.Fatalf("PluginVersion: %v", err)
	}
	if v != testKavitaVersion {
		t.Errorf("PluginVersion() = %q, want %q", v, testKavitaVersion)
	}
	if got := c.Version(); got != testKavitaVersion {
		t.Errorf("Version() = %q; PluginVersion must cache what it was told", got)
	}
}

// ---- the outbound body, on the wire ---------------------------------------

// TestSeriesFilterNeverMarshalsNullStatements is the fatal case from
// docs/reference/arr-apis.md §7.1, in Kavita's dialect. GetAllSeriesV2's FIRST
// act is `seriesFilterDto.Statements.Add(stmt)`, so `"statements":null`
// nil-derefs into a 500 — and a nil Go slice marshals as exactly that.
func TestSeriesFilterNeverMarshalsNullStatements(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    SeriesFilter
	}{
		{"zero value", SeriesFilter{}},
		{"explicit nil", SeriesFilter{Statements: nil}},
		{"from the constructor", mustFilter(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.f)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if strings.Contains(string(raw), `"statements":null`) {
				t.Errorf("body carries a null statements: %s", raw)
			}
			if !strings.Contains(string(raw), `"statements":[]`) {
				t.Errorf("body does not carry an empty statements array: %s", raw)
			}
		})
	}
}

func mustFilter(t *testing.T) SeriesFilter {
	t.Helper()
	f, err := NewSeriesFilter(SeriesSortFieldLastChapterAdded, false)
	if err != nil {
		t.Fatalf("NewSeriesFilter: %v", err)
	}
	return f
}

// TestSeriesListSendsTheVerifiedBody pins the exact bytes against the body that
// was verified working against the owner's live instance, not against a shape
// this repo invented.
func TestSeriesListSendsTheVerifiedBody(t *testing.T) {
	const verified = `{"id":0,"name":null,"statements":[],"combination":1,` +
		`"sortOptions":{"sortField":4,"isAscending":false},"entityType":0,"limitTo":0}`

	var sentBody string
	var sentURL *url.URL
	c, _ := newFakeClient(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		sentBody = string(b)
		sentURL = r.URL
		return respond(200, `[]`, nil), nil
	})
	if _, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error { return nil }); err != nil {
		t.Fatalf("StreamSeries: %v", err)
	}
	if sentBody != verified {
		t.Errorf("body =\n  %s\nwant\n  %s", sentBody, verified)
	}
	if got := sentURL.Query().Get("context"); got != "1" {
		t.Errorf("context = %q, want 1 (QueryContext.None; 0 is not a member)", got)
	}
	if sentURL.Query().Has("PageSize") {
		t.Error("an unpaged read must not send PageSize; UserParams coerces 0 to int.MaxValue anyway")
	}
}

func TestSeriesListPagingCasingIsExact(t *testing.T) {
	var q url.Values
	c, _ := newFakeClient(t, func(r *http.Request) (*http.Response, error) {
		q = r.URL.Query()
		return respond(200, `[]`, nil), nil
	})
	opts := SeriesListOptions{PageNumber: 3, PageSize: 20}
	if _, err := c.StreamSeries(context.Background(), opts, func(SeriesDto) error { return nil }); err != nil {
		t.Fatalf("StreamSeries: %v", err)
	}
	// ASP.NET's [FromQuery] binder on UserParams matches these exactly.
	if q.Get("PageNumber") != "3" || q.Get("PageSize") != "20" {
		t.Errorf("paging params = %v; the casing is what the binder matches", q)
	}
	if q.Has("pageNumber") || q.Has("pagesize") {
		t.Errorf("a lower-cased duplicate went out: %v", q)
	}
}
