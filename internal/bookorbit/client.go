package bookorbit

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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jdb3750/UsArr/internal/breaker"
)

// DefaultPort is BookOrbit's default HTTP port.
//
// 🔍 INFERENCE, not a measurement: server/src/config/config.ts reads
// `PORT ?? 3000`. Nothing in this client depends on it — a BaseURL without a
// port is a configuration error the caller resolves, not one this package
// guesses at.
const DefaultPort = 3000

// apiPrefix is the whole of BookOrbit's API path shape, and it is a decision
// rather than an observation.
//
// main.ts calls app.setGlobalPrefix('api/v1', {exclude: ['api/kobo/…',
// 'api/v3/…', 'api/UserStorage/…']}). ADR-0052 constrains this adapter to the
// /api/v1 routes ONLY, because those are the ones behind the global JwtAuthGuard
// whose strategy never reads a credential out of a query string. Every path this
// package builds starts here; there is no second prefix and no escape hatch.
const apiPrefix = "/api/v1"

// HTTPDoer is the minimal HTTP surface this package needs.
//
// It is an interface, not an *http.Client, so the caller injects the SSRF-policy
// client bound to this service_instance's validated host:port (internal/ssrf).
// This package must not be able to construct its own transport and bypass
// resolve-then-pin — the user configures an arbitrary internal URL here, which
// is textbook SSRF-by-design.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Timeouts are the per-call deadlines applied when the caller's context has
// none.
type Timeouts struct {
	Default time.Duration

	// List is the budget for a paged list read: THE WHOLE WALK, not one page.
	// StreamBooks takes it once around the entire page loop and gives each
	// individual request Timeouts.Default — catalogue.go:690 carries the
	// argument for that split.
	//
	// ⚠️ THIS COMMENT USED TO READ *"Nothing in slice 0 uses it — this client
	// makes no list call yet — and it is here so that the field's meaning is
	// fixed before the first caller needs it rather than after"*. The caller
	// arrived: Client.Libraries and Client.StreamBooks are both in this package.
	// The field was correctly placed and the sentence simply outlived the state
	// it described.
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
	// https://box:3000 or https://box/bookorbit. The /api/v1 prefix is this
	// package's and must NOT be included.
	BaseURL string

	// MagicLinkToken is the raw magic-link token a superuser minted for a SHARED
	// account. It is a §14 credential: header-and-body only, never a query
	// string, never logged, never returned in an error, and stored only under the
	// versioned AAD-bound envelope.
	//
	// Empty is legal and useful: GET /api/v1/health is @Public(), so a client
	// with no credential can still answer "is the host up".
	MagicLinkToken string

	// HTTPClient is required. There is no fallback to http.DefaultClient: that
	// would silently bypass internal/ssrf's resolve-then-pin, and the whole
	// reason this field is an interface is that this package must not be able to
	// build its own transport.
	HTTPClient HTTPDoer

	// UserAgent overrides the default.
	UserAgent string

	// AppVersion is UsArr's own version, used to build the default User-Agent.
	AppVersion string

	Timeouts Timeouts
	Breaker  BreakerConfig

	// IsPolicyError reports whether an error was UsArr's own egress policy
	// refusing to make the request (internal/ssrf.IsPolicyError). Such a refusal
	// never reached the network, so it is a configuration bug and must NOT trip
	// the circuit breaker. Injected rather than imported so this package does not
	// depend on internal/ssrf's client half.
	IsPolicyError func(error) bool

	Now    func() time.Time
	Rand   func() float64
	Logger *slog.Logger
}

// BreakerConfig, Breaker and the BreakerState constants are internal/breaker's,
// re-exported so a caller wires one client the way it wires the others.
type (
	// BreakerConfig is the tuning for this instance's circuit breaker.
	BreakerConfig = breaker.Config
	// Breaker is the per-instance circuit breaker.
	Breaker = breaker.Breaker
	// BreakerState is its state.
	BreakerState = breaker.State
)

const (
	// BreakerClosed passes every call through.
	BreakerClosed = breaker.StateClosed
	// BreakerOpen refuses every call until the cooldown expires.
	BreakerOpen = breaker.StateOpen
	// BreakerHalfOpen lets exactly one probe through.
	BreakerHalfOpen = breaker.StateHalfOpen
)

// Client is one BookOrbit instance's client. One Client per service_instance: it
// owns that instance's HTTP client, its circuit breaker AND its cached access
// token, and none of the three is shared.
type Client struct {
	base      *url.URL
	magicLink string
	http      HTTPDoer
	ua        string

	timeouts    Timeouts
	breaker     *Breaker
	isPolicyErr func(error) bool
	log         *slog.Logger
	now         func() time.Time

	// authMu guards sess AND serialises minting. See accessToken: the login route
	// is throttled at 10/minute, so concurrent mints are a real failure mode
	// rather than a theoretical one.
	authMu sync.Mutex
	sess   session

	// appVersion caches the last version seen. BookOrbit puts no version on an
	// ordinary API response, so this is only populated by AppInfo. Empty means
	// "not asked yet", never "not available".
	appVersion atomic.Pointer[string]
}

// New builds a client for one BookOrbit instance.
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
	// A credential in the base URL would end up in every stored or logged path.
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
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return &Client{
		base:        u,
		magicLink:   opts.MagicLinkToken,
		http:        opts.HTTPClient,
		ua:          ua,
		timeouts:    opts.Timeouts.withDefaults(),
		breaker:     breaker.New(opts.Breaker, ErrBreakerOpen, opts.Now, opts.Rand),
		isPolicyErr: opts.IsPolicyError,
		log:         log,
		now:         now,
	}, nil
}

// BaseURL returns the credential-free base URL. Safe to log.
func (c *Client) BaseURL() string { return c.base.String() }

// Breaker exposes the per-instance circuit breaker so health UI can render its
// state and its next retry time.
func (c *Client) Breaker() *Breaker { return c.breaker }

// Version returns the BookOrbit version last observed, or "" if nothing has
// asked for it yet.
//
// "" is NOT "this BookOrbit has no version", and neither is LocalBuildVersion.
// See AppInfo.
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
	path    string // absolute path under the base, e.g. "/api/v1/app-info"
	query   url.Values
	body    any
	timeout time.Duration

	// anonymous skips the bearer. Exactly two routes set it: GET /api/v1/health,
	// which is @Public() at class level, and POST /api/v1/auth/magic-links/login,
	// which is @Public() on the action and is where the bearer comes FROM.
	anonymous bool

	// accept overrides the Accept header. Empty means application/json, which is
	// every route in this package but one: the cover route answers a
	// createReadStream, and asking an image endpoint for JSON is a claim about
	// this client's expectations that is simply false. Nothing upstream negotiates
	// on it today — Fastify's reply.send(stream) ignores Accept, and BookOrbit has
	// no ReturnHttpNotAcceptable equivalent (doc.go) — so this is honesty rather
	// than a workaround, and it is the field that stops the day a Nest interceptor
	// does start negotiating from being a mystery.
	accept string
}

// newRequest builds the outbound *http.Request.
//
// EVERY read path goes through here, so the Authorization header, the Accept
// header and the URL assembly exist exactly once. A second copy is how one of
// them ends up putting the credential somewhere else.
func (c *Client) newRequest(ctx context.Context, r request, bearer string) (*http.Request, error) {
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

	accept := r.accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.ua)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if !r.anonymous && bearer != "" {
		req.Header.Set(authHeader, authScheme+bearer)
	}
	return req, nil
}

// doRaw performs one request and returns the answered status, the response
// HEADERS and the body without classifying any of them. It handles the deadline,
// the bearer and the 401 re-mint retry; it does NOT decide what a status means.
//
// It is separate from do because Health needs a 503 to reach it as a DIAGNOSIS
// rather than as a transport failure: Terminus answers 503 with a fully-formed
// body naming the indicator that is down, and collapsing that into ErrServer
// would throw away the one thing the probe exists to report.
//
// The header is returned rather than dropped because Cover needs Content-Type
// and the alternative was a second buffered-request path beside this one. It is
// the header of the LAST exchange, which is the answering one after a re-mint
// retry. Callers that want only a body ignore it, and every one of them does.
//
// err is non-nil only for a failure that happened before a status existed.
func (c *Client) doRaw(ctx context.Context, r request) (int, http.Header, []byte, error) {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = c.timeouts.Default
	}
	// Always bound the call. An unbounded request against a dead host is how a
	// worker goroutine leaks for an hour. The budget covers the MINT TOO, which
	// is why the deadline is set before the bearer is fetched: an authenticated
	// call that has to log in first must still fit the caller's budget.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var bearer string
	if !r.anonymous {
		var err error
		bearer, err = c.accessToken(ctx, "")
		if err != nil {
			return 0, nil, nil, err
		}
	}

	status, header, body, err := c.roundTrip(ctx, r, bearer)
	if err != nil {
		return 0, nil, nil, err
	}

	// The re-mint retry. A 401 on an authenticated route means the held access
	// token aged out — 15 minutes by default — and NOT that the stored
	// magic-link token is bad. Re-mint once, retry once, and let a second 401
	// through as the real answer.
	if status == http.StatusUnauthorized && !r.anonymous && bearer != "" {
		fresh, mintErr := c.accessToken(ctx, bearer)
		if mintErr != nil {
			return 0, nil, nil, mintErr
		}
		if fresh != bearer {
			status, header, body, err = c.roundTrip(ctx, r, fresh)
			if err != nil {
				return 0, nil, nil, err
			}
		}
	}

	return status, header, body, nil
}

// roundTrip is ONE wire exchange, and it is the only place the circuit breaker
// is consulted or updated.
//
// The gate lives here rather than one level up because a single authenticated
// CALL can be two exchanges — a mint followed by the real request — and gating
// the call would consume the half-open probe slot on the mint and then refuse
// the request it was minted for. One exchange, one Allow, one recordStatus.
func (c *Client) roundTrip(ctx context.Context, r request, bearer string) (int, http.Header, []byte, error) {
	if err := c.breaker.Allow(); err != nil {
		return 0, nil, nil, &APIError{Op: r.op, Method: r.method, Path: r.path, Err: err}
	}

	req, err := c.newRequest(ctx, r, bearer)
	if err != nil {
		return 0, nil, nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, c.transportError(r, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	// Bound the body. It FAILS rather than truncating: a truncated JSON body
	// decodes as a malformed document and gets diagnosed as an upstream bug,
	// while a silently truncated list looks like content deleted upstream — which
	// would drive a destructive reconciliation sweep (ARCHITECTURE.md §7.4).
	lb := &limitedBody{r: resp.Body, max: maxBufferedBody}
	body, err := io.ReadAll(lb)
	if err != nil {
		if lb.exceeded {
			c.recordStatus(resp.StatusCode)
			return 0, nil, nil, &APIError{
				Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode,
				Message: fmt.Sprintf("response exceeded the %d-byte buffered limit", maxBufferedBody),
				Err:     ErrResponseTooLarge,
			}
		}
		return 0, nil, nil, c.transportError(r, err)
	}
	c.recordStatus(resp.StatusCode)
	return resp.StatusCode, resp.Header, body, nil
}

// do is doRaw plus status classification and a JSON decode. out may be nil to
// discard the body.
func (c *Client) do(ctx context.Context, r request, out any) error {
	status, _, body, err := c.doRaw(ctx, r)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return parseErrorBody(r.op, r.method, r.path, status, body)
	}
	if out == nil {
		return nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Status: status, Message: "empty body", Err: ErrDecode}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &APIError{
			Op: r.op, Method: r.method, Path: r.path, Status: status,
			Message: "decoding response: " + truncate(err.Error()), Err: ErrDecode,
		}
	}
	return nil
}

// recordStatus feeds one answered request to the breaker.
//
// 4xx is UsArr sending a bad request, or a credential problem, or the upstream
// throttler; none of it says anything about instance health, so none of it trips
// the breaker. 5xx does.
func (c *Client) recordStatus(status int) {
	if status >= 500 {
		c.breaker.Failure()
		return
	}
	c.breaker.Success()
}

// maxBufferedBody caps the body one call reads into memory.
const maxBufferedBody int64 = 32 << 20

// transportError classifies a failure that happened before a status code
// existed. The error is deliberately RECONSTRUCTED rather than wrapped verbatim:
// *url.Error carries the full request URL, and reconstructing is the discipline
// that survives a future route that puts something in one.
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

// limitedBody fails at max rather than truncating.
type limitedBody struct {
	r        io.Reader
	max      int64
	n        int64
	exceeded bool
}

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.exceeded {
		return 0, ErrResponseTooLarge
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.max {
		l.exceeded = true
		return n, ErrResponseTooLarge
	}
	return n, err
}

// ---- endpoints ------------------------------------------------------------

// Health calls GET /api/v1/health — the §17.3 reachability probe.
//
// HealthController is @Public() at CLASS level, so this succeeds with NO
// credential, which is what makes it useful: it separates "host down" from "host
// up, credential wrong". Better than Kavita's arrangement in one respect worth
// naming — Kavita's /api/Health returns the bare string `Ok`, which is not
// evidence about anything, while this returns a Terminus report naming each
// indicator.
//
// IT PROVES NOTHING ABOUT THE CREDENTIAL and nothing about the host being a
// BookOrbit: any Terminus health controller answers this shape. The handshake
// that proves the rest is Authenticate followed by AppInfo.
//
// A 503 is ANSWERED, not failed. Terminus returns 503 with a fully-formed body
// when an indicator is down, and that body is the diagnosis — so the report is
// returned alongside ErrUnhealthy rather than instead of it, and the breaker is
// told the host answered. Tripping the breaker on the probe that explains what
// is wrong would silence the explanation.
func (c *Client) Health(ctx context.Context) (HealthReport, error) {
	r := request{op: "Health", method: http.MethodGet, path: apiPrefix + "/health", anonymous: true}

	status, _, body, err := c.doRaw(ctx, r)
	if err != nil {
		return HealthReport{}, err
	}
	if status != http.StatusOK && status != http.StatusServiceUnavailable {
		return HealthReport{}, parseErrorBody(r.op, r.method, r.path, status, body)
	}

	var th terminusHealth
	if err := json.Unmarshal(body, &th); err != nil {
		return HealthReport{}, &APIError{
			Op: r.op, Method: r.method, Path: r.path, Status: status,
			Message: "decoding health report: " + truncate(err.Error()), Err: ErrDecode,
		}
	}
	report := toHealthReport(th)
	if status == http.StatusServiceUnavailable || !report.OK() {
		e := &APIError{Op: r.op, Method: r.method, Path: r.path, Status: status, Err: ErrUnhealthy}
		names := make([]string, 0, 2)
		for _, i := range report.Failing() {
			if i.Message != "" {
				names = append(names, i.Name+": "+i.Message)
				continue
			}
			names = append(names, i.Name+" is "+i.Status)
		}
		e.Message = strings.Join(names, "; ")
		return report, e
	}
	return report, nil
}

// AppInfo calls GET /api/v1/app-info — THE AUTHENTICATED READ THAT PROVES THE
// CLIENT WORKS.
//
// AppInfoController declares no @RequirePermission and no @RequireLibraryAccess,
// so it answers for ANY valid JWT and 401s for none. That makes it the right
// handshake for a credential whose whole design goal is an account with an EMPTY
// permission set: a route that needed a permission would prove the wrong thing.
//
// It is deliberately NOT GET /api/v1/libraries. Knowing what a library is
// belongs to slice 1, and a slice-0 client that already read the container list
// would have quietly started the catalogue adapter.
//
// ⚠️ `version` is `process.env.APP_VERSION ?? 'Local build'` (config/config.ts),
// so a self-built or development instance reports the literal string
// LocalBuildVersion. Any version-floor check must treat that as UNKNOWN and not
// as TOO OLD; AppInfo.VersionKnown is the predicate.
func (c *Client) AppInfo(ctx context.Context) (AppInfo, error) {
	var d appInfoDTO
	if err := c.do(ctx, request{
		op: "AppInfo", method: http.MethodGet, path: apiPrefix + "/app-info",
	}, &d); err != nil {
		return AppInfo{}, err
	}
	out := toAppInfo(d)
	if out.VersionKnown() {
		c.setVersion(out.Version)
	}
	return out, nil
}
