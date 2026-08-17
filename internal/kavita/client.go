package kavita

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultPort is Kavita's default HTTP port.
const DefaultPort = 5000

// apiPrefix is the whole of Kavita's API path shape. There is NO version segment:
// routes are /api/<Controller>/<action> with a PascalCase controller name, and
// they are case-sensitive on the controller.
const apiPrefix = "/api"

// authHeader is the ONE authentication mechanism. See doc.go: the spec declares a
// single securityScheme, applied globally, with no per-operation overrides.
const authHeader = "x-api-key"

// HTTPDoer is the minimal HTTP surface this package needs.
//
// It is an interface, not an *http.Client, so the caller injects the SSRF-policy
// client bound to this service_instance's validated host:port (internal/ssrf).
// This package must not be able to construct its own transport and bypass that.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Timeouts are the per-call deadlines applied when the caller's context has none.
type Timeouts struct {
	Default time.Duration

	// List is the budget for a streamed list read. It covers the WHOLE body, not
	// just the response headers, because the deadline is on the context and the
	// context governs every Read.
	//
	// It is large on purpose: an unpaged POST /api/Series/all-v2 returns the
	// entire library in one response (UserParams.PageSize defaults to
	// int.MaxValue), and the Default would fail that on any ordinary link.
	// 10 minutes is an INFERENCE from the *Arr list sizes in
	// docs/reference/sync.md §2, not a measurement against a large Kavita — it is
	// a ceiling that stops a worker leaking, not a target.
	List time.Duration
}

func (t Timeouts) withDefaults() Timeouts {
	if t.Default <= 0 {
		t.Default = 10 * time.Second
	}
	if t.List <= 0 {
		t.List = 10 * time.Minute
	}
	return t
}

// Options configures a Client.
type Options struct {
	// BaseURL is scheme://host:port plus any reverse-proxy base path, e.g.
	// https://box:5000 or https://box/kavita.
	BaseURL string

	// APIKey is Kavita's Auth Key. For the admin account UsArr is normally given
	// it is a full-admin credential and is treated as one: header only, never a
	// query string (the single exception is PluginVersion, which documents
	// itself), never logged, never returned in an error.
	APIKey string

	// HTTPClient is required. There is no fallback to http.DefaultClient: that
	// would silently bypass the SSRF policy and the per-instance timeouts.
	HTTPClient HTTPDoer

	// UserAgent overrides the default.
	//
	// UsArr does NOT send X-Kavita-Client. That header is Kavita's own client
	// identifier and it participates in device tracking; UsArr is not a Kavita UI
	// and must not be attributed as one.
	UserAgent string

	// AppVersion is UsArr's own version, used to build the default User-Agent.
	AppVersion string

	Timeouts Timeouts
	Breaker  BreakerConfig

	// IsPolicyError reports whether an error was UsArr's own egress policy
	// refusing to make the request (internal/ssrf.IsPolicyError). Such a refusal
	// never reached the network, so it is a configuration bug and must NOT trip
	// the circuit breaker. Injected rather than imported so this package does not
	// depend on internal/ssrf.
	IsPolicyError func(error) bool

	Now    func() time.Time
	Rand   func() float64
	Logger *slog.Logger
}

// Client is one Kavita instance's client. One Client per service_instance: it
// owns that instance's HTTP client and its circuit breaker, and neither is
// shared.
type Client struct {
	base   *url.URL
	apiKey string
	http   HTTPDoer
	ua     string

	timeouts    Timeouts
	breaker     *Breaker
	isPolicyErr func(error) bool
	log         *slog.Logger

	// maxStream bounds one streamed body. It is a field rather than a constant so
	// a test can fire the bound without pushing MaxStreamBytes (200 MB) through a
	// race-instrumented decoder; nothing outside this package can change it.
	maxStream int64

	// appVersion caches the last version seen. Unlike an *Arr's, it is NOT free:
	// Kavita puts no version on an ordinary API response, so this is only
	// populated by a call that asks for it. Empty means "not asked yet", never
	// "not available".
	appVersion atomic.Pointer[string]
}

// New builds a client for one Kavita instance.
func New(opts Options) (*Client, error) {
	if opts.HTTPClient == nil {
		return nil, fmt.Errorf("%w: HTTPClient is required (inject the SSRF-policy client)", ErrInvalidRequest)
	}
	raw := strings.TrimSpace(opts.BaseURL)
	if raw == "" {
		return nil, fmt.Errorf("%w: BaseURL is required", ErrInvalidRequest)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing BaseURL: %w", ErrInvalidRequest, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: BaseURL scheme must be http or https", ErrInvalidRequest)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: BaseURL has no host", ErrInvalidRequest)
	}
	// A credential in the base URL would end up in every stored/logged path.
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/")

	ua := opts.UserAgent
	if ua == "" {
		v := opts.AppVersion
		if v == "" {
			v = "0.0.0-dev"
		}
		ua = "UsArr/" + v + " (+https://github.com/jdb3750/UsArr)"
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &Client{
		base:        u,
		apiKey:      opts.APIKey,
		http:        opts.HTTPClient,
		ua:          ua,
		timeouts:    opts.Timeouts.withDefaults(),
		breaker:     NewBreaker(opts.Breaker, opts.Now, opts.Rand),
		isPolicyErr: opts.IsPolicyError,
		log:         log,
		maxStream:   MaxStreamBytes,
	}, nil
}

// BaseURL returns the credential-free base URL. Safe to log.
func (c *Client) BaseURL() string { return c.base.String() }

// Breaker exposes the per-instance circuit breaker so health UI can render its
// state and its next retry time.
func (c *Client) Breaker() *Breaker { return c.breaker }

// Version returns the Kavita version last observed, or "" if nothing has asked
// for it yet.
//
// "" is NOT "this Kavita has no version". Kavita sends no version header, so this
// is populated only by ServerInfo or PluginVersion — see doc.go.
func (c *Client) Version() string {
	if p := c.appVersion.Load(); p != nil {
		return *p
	}
	return ""
}

func (c *Client) setVersion(v string) {
	if v == "" {
		return
	}
	c.appVersion.Store(&v)
}

// ---- requests -------------------------------------------------------------

type request struct {
	op      string
	method  string
	path    string // absolute path under the base, e.g. "/api/Series/all-v2"
	query   url.Values
	body    any
	timeout time.Duration

	// anonymous skips the Auth Key header. Only /api/Health is [AllowAnonymous]
	// among the endpoints this client calls with it set.
	anonymous bool
}

// newRequest builds the outbound *http.Request for one request.
//
// BOTH read paths — do and stream — go through here, so the Auth Key header, the
// Accept header and the URL assembly exist exactly once. A second copy is how one
// of them ends up putting the key in a query string.
func (c *Client) newRequest(ctx context.Context, r request) (*http.Request, error) {
	u := *c.base
	u.Path = c.base.Path + r.path
	if len(r.query) > 0 {
		u.RawQuery = r.query.Encode()
	}

	var bodyReader io.Reader
	var hasBody bool
	if r.body != nil {
		b, err := json.Marshal(r.body)
		if err != nil {
			return nil, &APIError{Op: r.op, Method: r.method, Path: r.path, Message: "encoding request body", Err: ErrInvalidRequest}
		}
		hasBody = true
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), bodyReader)
	if err != nil {
		return nil, &APIError{Op: r.op, Method: r.method, Path: r.path, Message: "building request", Err: ErrInvalidRequest}
	}

	// Kavita's controllers declare text/plain, application/json and text/json on
	// most 200s, and GET /api/Health returns the bare string Ok as text/plain.
	// Accept all three and decide how to parse per endpoint rather than trusting
	// the returned Content-Type.
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", c.ua)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if !r.anonymous && c.apiKey != "" {
		req.Header.Set(authHeader, c.apiKey)
	}
	return req, nil
}

// do performs one request: breaker gate, deadline, headers, status handling, JSON
// decode. out may be nil to discard the body.
//
// do BUFFERS the whole body, so it is for the small, fixed-shape endpoints only.
// POST /api/Series/all-v2 is unpaged by default and returns an entire library in
// one response — that goes through StreamSeries, which never holds more than one
// element.
func (c *Client) do(ctx context.Context, r request, out any) error {
	if err := c.breaker.Allow(); err != nil {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Err: err}
	}

	timeout := r.timeout
	if timeout <= 0 {
		timeout = c.timeouts.Default
	}
	// Always bound the call. An unbounded request against a dead host is how a
	// worker goroutine leaks for an hour.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := c.newRequest(ctx, r)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return c.transportError(r, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	// Bound the body. It FAILS rather than truncating: a truncated JSON body
	// decodes as a malformed document and gets diagnosed as an upstream bug, and
	// a silently truncated list looks like content deleted upstream — which would
	// drive a destructive sweep.
	lb := &limitedBody{r: resp.Body, max: maxBufferedBody}
	body, err := io.ReadAll(lb)
	if err != nil {
		if lb.exceeded {
			c.recordStatus(resp.StatusCode)
			return &APIError{
				Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode,
				Message: fmt.Sprintf("response exceeded the %d-byte buffered limit; this endpoint needs the streaming path", maxBufferedBody),
				Err:     ErrResponseTooLarge,
			}
		}
		return c.transportError(r, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := parseErrorBody(r.op, r.method, r.path, resp.StatusCode, body)
		c.recordStatus(resp.StatusCode)
		return apiErr
	}

	c.recordStatus(resp.StatusCode)

	if out == nil {
		return nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode, Message: "empty body", Err: ErrDecode}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &APIError{
			Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode,
			Message: "decoding response: " + truncate(err.Error()), Err: ErrDecode,
		}
	}
	return nil
}

// recordStatus feeds one answered request to the breaker.
//
// 4xx is UsArr sending a bad request, or a key whose account lacks a role; it
// says nothing about instance health, so it must not trip the breaker. 5xx does.
func (c *Client) recordStatus(status int) {
	if status >= 500 {
		c.breaker.Failure()
		return
	}
	c.breaker.Success()
}

// maxBufferedBody caps the body do reads into memory. It stays at 32 MB
// deliberately: do's endpoints are small and fixed-shape, and raising it here
// would only make the buffered path a bigger memory hazard.
const maxBufferedBody int64 = 32 << 20

// transportError classifies a failure that happened before a status code existed.
// The error is deliberately RECONSTRUCTED rather than wrapped verbatim:
// *url.Error carries the full request URL, and for this package that URL can
// carry an Auth Key.
func (c *Client) transportError(r request, err error) error {
	e := &APIError{Op: r.op, Method: r.method, Path: r.path}

	switch {
	case c.isPolicyErr != nil && c.isPolicyErr(err):
		// An SSRF-policy refusal never reached the network. It is a configuration
		// or provenance bug, not upstream flakiness, so it must not count toward
		// the breaker and must not be retried.
		e.Message = redactErr(err)
		e.Err = ErrInvalidRequest
		return e
	case errors.Is(err, context.DeadlineExceeded):
		e.Message = "deadline exceeded"
		e.Err = ErrTimeout
	case errors.Is(err, context.Canceled):
		e.Message = "canceled"
		e.Err = context.Canceled
		// A cancelled request is the caller's decision, not an upstream failure.
		return e
	default:
		e.Message = redactErr(err)
		e.Err = ErrServer
	}
	c.breaker.Failure()
	return e
}

// redactErr strips any URL out of a transport error message. net/http wraps
// everything in *url.Error, whose Error() prints the full request URL.
func redactErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + " " + RedactURL(ue.URL) + ": " + redactErr(ue.Err)
	}
	return truncate(err.Error())
}

// ---- endpoints ------------------------------------------------------------

// Health calls GET /api/Health.
//
// HealthController is [AllowAnonymous] at class level and the action is a no-op
// returning the bare string Ok, so this succeeds with NO key — which is exactly
// what makes it useful: it separates "host down" from "host up, key wrong".
//
// It proves NOTHING about the key and NOTHING about the host being a Kavita. The
// body is discarded for that reason: `Ok` is not evidence.
func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, request{
		op: "Health", method: http.MethodGet, path: apiPrefix + "/Health", anonymous: true,
	}, nil)
}

// ServerInfo calls GET /api/Server/server-info-slim.
//
// NOTE THE NAME. There is no /api/Server/server-info; the endpoint is
// server-info-slim, and its shape is ServerInfoSlimDto.
//
// ADMIN-ONLY. ServerController is [Authorize(PolicyGroups.AdminPolicy)] at class
// level, so a valid non-admin Auth Key gets ErrForbidden here while every series
// and library read it needs succeeds. Callers must treat a 403 as "the key is
// fine, this account is not an admin" and NOT as a credential failure.
func (c *Client) ServerInfo(ctx context.Context) (ServerInfoSlimDto, error) {
	var out ServerInfoSlimDto
	if err := c.do(ctx, request{
		op: "ServerInfo", method: http.MethodGet, path: apiPrefix + "/Server/server-info-slim",
	}, &out); err != nil {
		return out, err
	}
	c.setVersion(out.KavitaVersion)
	return out, nil
}

// PluginVersion calls GET /api/Plugin/version?apiKey=… and is the NON-ADMIN
// version fallback.
//
// 🚩 IT PUTS THE AUTH KEY IN THE QUERY STRING, and that is not a choice this
// package makes: PluginController.GetVersion takes `[Required] string apiKey`
// from the query and looks the user up by it. There is no header path. The
// consequences are the ordinary ones — access logs, Referer headers, any reverse
// proxy in front — so:
//
//   - Nothing calls this automatically. It is opt-in, for the case where
//     ServerInfo returned ErrForbidden and the operator still wants a version.
//   - The URL it builds is credential-bearing. Every error path through this
//     client reconstructs rather than wraps for exactly this reason, and
//     RedactURL covers `apiKey` in internal/ssrf's shared deny-list.
//
// The endpoint is [AllowAnonymous] but is NOT unauthenticated: it throws
// KavitaUnauthenticatedUserException when the key resolves to no user, and its
// own remarks say it logs unauthorized requests to the security log. So a 401
// here still means "bad key".
func (c *Client) PluginVersion(ctx context.Context) (string, error) {
	if c.apiKey == "" {
		return "", &APIError{
			Op: "PluginVersion", Method: http.MethodGet, Path: apiPrefix + "/Plugin/version",
			Message: "no Auth Key is configured; this endpoint takes it in the query string and has no header path",
			Err:     ErrInvalidRequest,
		}
	}
	var out string
	if err := c.do(ctx, request{
		op: "PluginVersion", method: http.MethodGet, path: apiPrefix + "/Plugin/version",
		query: url.Values{"apiKey": []string{c.apiKey}},
		// anonymous: false — the header costs nothing and keeps one code path.
	}, &out); err != nil {
		return "", err
	}
	c.setVersion(out)
	return out, nil
}

// Libraries calls GET /api/Library/libraries.
//
// This is the CREDENTIAL HANDSHAKE. LibraryController is [Authorize] at class
// level with no admin policy on this action, so it answers for any valid Auth Key
// and 401s for an invalid one — unlike ServerInfo, which cannot distinguish "bad
// key" from "not an admin" for the caller who only has one of them.
//
// It returns the libraries THIS ACCOUNT can see, which for a non-admin is a
// subset of the server's. That is the right list for §17.8's binding anyway: UsArr
// can only sync what its own credential can read.
func (c *Client) Libraries(ctx context.Context) ([]LibraryDto, error) {
	var out []LibraryDto
	err := c.do(ctx, request{
		op: "Libraries", method: http.MethodGet, path: apiPrefix + "/Library/libraries",
	}, &out)
	return out, err
}

// SeriesMetadata calls GET /api/Series/metadata?seriesId=N.
//
// It is the ONLY endpoint that reports a series' creators — see
// SeriesMetadataDto for the three alternatives that were checked and cannot
// rebuild the mapping.
//
// IT GOES THROUGH `do`, NOT THROUGH `stream`, AND THAT IS THE RIGHT SIDE OF THE
// LINE. `do` buffers the whole body and is documented as being for "the small,
// fixed-shape endpoints only". This response is one series' metadata: a summary,
// a handful of tag arrays and thirteen person arrays, all of them bounded by how
// many people a single comic credits. POST /api/Series/all-v2 is the opposite —
// the whole library in one response — which is why that one streams.
//
// THE COST IS N ROUND TRIPS FOR N SERIES AND IT IS BUDGETED RATHER THAN HIDDEN.
// docs/reference/sync.md §2 already settled this shape for the *Arrs: "the rule
// is not 'never fetch children per-parent' — it is 'fetch them per parent,
// bounded'". The caller in internal/libsync runs it as a phase-B pass AFTER the
// item stream has closed, so it never holds the streaming connection open, and
// the breaker and the per-call timeout in `do` bound each one.
func (c *Client) SeriesMetadata(ctx context.Context, seriesID int32) (SeriesMetadataDto, error) {
	var out SeriesMetadataDto
	err := c.do(ctx, request{
		op: "SeriesMetadata", method: http.MethodGet, path: apiPrefix + "/Series/metadata",
		query: url.Values{"seriesId": []string{strconv.FormatInt(int64(seriesID), 10)}},
	}, &out)
	return out, err
}

// SeriesListOptions is one POST /api/Series/all-v2 read.
type SeriesListOptions struct {
	// Sort names the ordering key. Zero means SeriesSortFieldLastChapterAdded,
	// descending — channel 3b's key (ARCHITECTURE.md §7.1a).
	Sort      SeriesSortField
	Ascending bool

	// PageNumber is 1-based (UserParams.PageNumber defaults to 1). Zero means
	// "do not send it".
	PageNumber int

	// PageSize is the page length.
	//
	// ⚠️ ZERO DOES NOT MEAN "a sensible default". UserParams coerces PageSize 0 to
	// int.MaxValue, and an absent PageSize defaults to int.MaxValue too — so zero
	// here means THE WHOLE LIBRARY IN ONE RESPONSE. That is a legitimate request
	// (it is channel 1's read) and it is why this endpoint is only ever streamed.
	PageSize int

	// Context is the `context` query parameter. Zero means QueryContextNone,
	// which is also the binder's own default.
	Context QueryContext
}

func (o SeriesListOptions) resolve() (SeriesListOptions, error) {
	if o.Sort == 0 {
		o.Sort = SeriesSortFieldLastChapterAdded
	}
	if o.Context == 0 {
		o.Context = QueryContextNone
	}
	if !o.Sort.valid() {
		return o, fmt.Errorf("%w: Sort %d is not a member of SeriesSortField", ErrInvalidRequest, int32(o.Sort))
	}
	if !o.Context.valid() {
		return o, fmt.Errorf("%w: Context %d is not a member of QueryContext (members are 1, 2, 4)", ErrInvalidRequest, int32(o.Context))
	}
	if o.PageNumber < 0 || o.PageSize < 0 {
		return o, fmt.Errorf("%w: PageNumber and PageSize must not be negative", ErrInvalidRequest)
	}
	return o, nil
}

// values renders the query string. The casing is EXACT and is not a style
// choice: ASP.NET's [FromQuery] binder on UserParams matches PageNumber and
// PageSize, and Kavita's own UI sends them in that casing.
func (o SeriesListOptions) values() url.Values {
	q := url.Values{}
	if o.PageNumber > 0 {
		q.Set("PageNumber", strconv.Itoa(o.PageNumber))
	}
	if o.PageSize > 0 {
		q.Set("PageSize", strconv.Itoa(o.PageSize))
	}
	// Sent explicitly rather than left to the binder default, so a change to that
	// default cannot silently change what UsArr asks for.
	q.Set("context", strconv.Itoa(int(o.Context)))
	return q
}
