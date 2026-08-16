package servarr

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
	"sync/atomic"
	"time"
)

// DefaultProwlarrPort is Prowlarr's default HTTP port. Its default SSL port is
// 9697 (medium confidence — verify before relying on it).
const DefaultProwlarrPort = 9696

// App identifies which *Arr this client talks to. Only Prowlarr is implemented.
type App string

const AppProwlarr App = "Prowlarr"

// HTTPDoer is the minimal HTTP surface this package needs.
//
// It is an interface, not an *http.Client, so the caller injects the SSRF-policy
// client bound to this service_instance's validated host:port (internal/ssrf).
// This package must not be able to construct its own transport and bypass that.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Timeouts are the per-call deadlines applied when the caller's context has none.
//
// Search gets its own, much longer budget because Prowlarr dispatches every
// indexer in parallel and answers only when the slowest has answered or failed:
// 15 s per-indexer HTTP timeout with 2 Polly retries means 45-60 s is an ordinary
// worst case. There is no async variant and no per-indexer progress.
type Timeouts struct {
	Default time.Duration
	Search  time.Duration
}

func (t Timeouts) withDefaults() Timeouts {
	if t.Default <= 0 {
		t.Default = 10 * time.Second
	}
	if t.Search <= 0 {
		t.Search = 60 * time.Second
	}
	return t
}

// Options configures a Client.
type Options struct {
	// BaseURL is scheme://host:port plus any urlBase, e.g. https://box:9696/prowlarr.
	BaseURL string

	// APIKey is a FULL-ADMIN credential. It is sent only in the X-Api-Key header —
	// never as ?apikey=, which would leak it into access logs, Referer headers and
	// any reverse proxy in front.
	APIKey string

	// HTTPClient is required. There is no fallback to http.DefaultClient: that
	// would silently bypass the SSRF policy and the per-instance timeouts.
	HTTPClient HTTPDoer

	// UserAgent overrides the default. UsArr must NOT send X-Prowlarr-Client — its
	// presence makes Prowlarr attribute the grab to its own UI instead of to UsArr.
	UserAgent string

	// AppVersion is UsArr's own version, used to build the default User-Agent.
	AppVersion string

	Timeouts Timeouts
	Breaker  BreakerConfig

	// IsPolicyError reports whether an error was UsArr's own egress policy
	// refusing to make the request (internal/ssrf.IsPolicyError). Such a refusal
	// never reached the network, so it is a configuration bug and must NOT trip the
	// circuit breaker. Injected rather than imported so this package does not
	// depend on internal/ssrf.
	IsPolicyError func(error) bool

	Now    func() time.Time
	Rand   func() float64
	Logger *slog.Logger
}

// Client is one *Arr instance's client. One Client per service_instance: it owns
// that instance's HTTP client and its circuit breaker, and neither is shared.
type Client struct {
	app     App
	apiPath string // "/api/v1"; "/api/v3" for Sonarr/Radarr when they land
	base    *url.URL
	apiKey  string
	http    HTTPDoer
	ua      string

	timeouts    Timeouts
	breaker     *Breaker
	isPolicyErr func(error) bool
	log         *slog.Logger

	// appVersion is read from the X-Application-Version header, which is present
	// on every /api/* response — so version detection costs nothing extra.
	appVersion atomic.Pointer[string]
}

// NewProwlarr builds a client for one Prowlarr instance.
//
// The API version path is hard-coded to v1 and is NEVER derived from the app
// version: Prowlarr 2.x still serves /api/v1. Use CheckAPIVersion for the
// forward-compatible check.
func NewProwlarr(opts Options) (*Client, error) {
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
		app:         AppProwlarr,
		apiPath:     "/api/v1",
		base:        u,
		apiKey:      opts.APIKey,
		http:        opts.HTTPClient,
		ua:          ua,
		timeouts:    opts.Timeouts.withDefaults(),
		breaker:     NewBreaker(opts.Breaker, opts.Now, opts.Rand),
		isPolicyErr: opts.IsPolicyError,
		log:         log,
	}, nil
}

// App reports which *Arr this client speaks to.
func (c *Client) App() App { return c.app }

// BaseURL returns the credential-free base URL. Safe to log.
func (c *Client) BaseURL() string { return c.base.String() }

// Breaker exposes the per-instance circuit breaker so health UI can render its
// state and its next retry time.
func (c *Client) Breaker() *Breaker { return c.breaker }

// AppVersion returns the upstream version last seen in X-Application-Version, or
// "" if no API call has been made yet.
func (c *Client) AppVersion() string {
	if p := c.appVersion.Load(); p != nil {
		return *p
	}
	return ""
}

// ---- requests -------------------------------------------------------------

type request struct {
	op      string
	method  string
	path    string // absolute path under the base, e.g. "/api/v1/search"
	query   url.Values
	body    any
	timeout time.Duration

	// anonymous skips the API key header. Only /ping is [AllowAnonymous].
	anonymous bool
}

// do performs one request: breaker gate, deadline, headers, status handling,
// JSON decode. out may be nil to discard the body.
func (c *Client) do(ctx context.Context, r request, out any) error {
	if err := c.breaker.Allow(); err != nil {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Err: err}
	}

	timeout := r.timeout
	if timeout <= 0 {
		timeout = c.timeouts.Default
	}
	// Always bound the call. An unbounded request against a dead indexer is how a
	// worker goroutine leaks for an hour.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	u := *c.base
	u.Path = c.base.Path + r.path
	if len(r.query) > 0 {
		u.RawQuery = r.query.Encode()
	}

	var bodyReader io.Reader
	var rawBody []byte
	if r.body != nil {
		b, err := json.Marshal(r.body)
		if err != nil {
			return &APIError{Op: r.op, Method: r.method, Path: r.path, Message: "encoding request body", Err: ErrInvalidRequest}
		}
		rawBody = b
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), bodyReader)
	if err != nil {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Message: "building request", Err: ErrInvalidRequest}
	}

	// Content negotiation: ReturnHttpNotAcceptable = true is confirmed in the
	// shipped Servarr Startup.cs, and several of the largest endpoints are declared
	// text/plain-only in the specs — so a bare `application/json` can 406 the most
	// important endpoint in the import path. Accept both and parse as JSON
	// regardless of the returned Content-Type (docs/DEVELOPMENT.md §5).
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", c.ua)
	if rawBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !r.anonymous && c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return c.transportError(r, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	if v := resp.Header.Get("X-Application-Version"); v != "" {
		c.appVersion.Store(&v)
	}

	// Bound the body. A misconfigured base URL pointing at something that streams
	// forever must not exhaust memory.
	const maxBody = 32 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return c.transportError(r, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := parseErrorBody(r.op, r.method, r.path, resp.StatusCode, body)
		// 4xx is UsArr sending a bad request; it says nothing about instance
		// health, so it must not trip the breaker. 5xx does.
		if resp.StatusCode >= 500 {
			c.breaker.Failure()
		} else {
			c.breaker.Success()
		}
		return apiErr
	}

	c.breaker.Success()

	if out == nil {
		return nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode, Message: "empty body", Err: ErrDecode}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &APIError{
			Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode,
			Message: "decoding response: " + err.Error(), Err: ErrDecode,
		}
	}
	return nil
}

// transportError classifies a failure that happened before a status code existed.
// The error is deliberately reconstructed rather than wrapped verbatim: *url.Error
// carries the full request URL, which for this package can carry a credential.
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

// Ping calls GET {base}/ping. It is [AllowAnonymous], so it succeeds with no key —
// which is exactly what makes it useful: it distinguishes "host down" from
// "host up, key wrong".
func (c *Client) Ping(ctx context.Context) (PingResource, error) {
	var out PingResource
	err := c.do(ctx, request{op: "Ping", method: http.MethodGet, path: "/ping", anonymous: true}, &out)
	return out, err
}

// SystemStatus calls GET /api/v1/system/status and verifies appName.
//
// Sonarr and Radarr answer the identical SystemResource shape on their own paths,
// so appName is the only thing that proves which app is on the other end.
//
// CAVEAT the health UI must repeat honestly: when the instance's
// AuthenticationRequired is DisabledForLocalAddresses, a request from a local
// address succeeds WITH NO KEY. A 200 here therefore does not prove the stored
// API key is valid. Say "reachable", not "credential verified".
func (c *Client) SystemStatus(ctx context.Context) (SystemResource, error) {
	var out SystemResource
	if err := c.do(ctx, request{op: "SystemStatus", method: http.MethodGet, path: c.apiPath + "/system/status"}, &out); err != nil {
		return out, err
	}
	if !strings.EqualFold(out.AppName, string(c.app)) {
		return out, &APIError{
			Op: "SystemStatus", Method: http.MethodGet, Path: c.apiPath + "/system/status",
			Status:  http.StatusOK,
			Message: fmt.Sprintf("expected %s, got %q", c.app, out.AppName),
			Err:     ErrWrongService,
		}
	}
	return out, nil
}

// KeyProvenValid reports whether a successful SystemStatus actually proves the API
// key was accepted. It does not when the instance disables auth for local
// addresses. Surface the difference rather than claiming a verification that did
// not happen.
func KeyProvenValid(s SystemResource) bool {
	return !strings.EqualFold(s.Authentication, "disabledForLocalAddresses") &&
		!strings.EqualFold(s.Authentication, "none")
}

// CheckAPIVersion calls GET {base}/api and refuses anything that is not the
// version this client speaks. This is the forward-compatible check; the app
// version is not, because Prowlarr 2.x still serves /api/v1.
func (c *Client) CheckAPIVersion(ctx context.Context) (APIInfoResource, error) {
	var out APIInfoResource
	if err := c.do(ctx, request{op: "APIVersion", method: http.MethodGet, path: "/api"}, &out); err != nil {
		return out, err
	}
	want := strings.TrimPrefix(c.apiPath, "/api/")
	if !strings.EqualFold(out.Current, want) {
		return out, &APIError{
			Op: "APIVersion", Method: http.MethodGet, Path: "/api", Status: http.StatusOK,
			Message: fmt.Sprintf("instance serves API %q, this client speaks %q", out.Current, want),
			Err:     ErrUnsupportedAPIVersion,
		}
	}
	return out, nil
}

// Indexers calls GET /api/v1/indexer.
//
// It returns ENABLED AND DISABLED indexers sorted by name; there is no filter
// parameter, so filter on Enable yourself. The returned resources carry
// fields[] with the indexer's credentials — run SanitizeIndexer before anything
// crosses the boundary toward a browser.
func (c *Client) Indexers(ctx context.Context) ([]IndexerResource, error) {
	var out []IndexerResource
	err := c.do(ctx, request{op: "Indexers", method: http.MethodGet, path: c.apiPath + "/indexer"}, &out)
	return out, err
}

// IndexerStatus calls GET /api/v1/indexerstatus.
//
// It returns ONLY BLOCKED indexers; an empty array means everything is healthy.
// Correlating a search against this is not optional — see Search.
func (c *Client) IndexerStatus(ctx context.Context) ([]IndexerStatusResource, error) {
	var out []IndexerStatusResource
	err := c.do(ctx, request{op: "IndexerStatus", method: http.MethodGet, path: c.apiPath + "/indexerstatus"}, &out)
	return out, err
}

// DownloadClients calls GET /api/v1/downloadclient.
//
// Preflight this before a grab: with no enabled client matching the release
// protocol, the grab fails with a 500 carrying a full .NET stack trace, which is
// not something a user can act on.
func (c *Client) DownloadClients(ctx context.Context) ([]DownloadClientResource, error) {
	var out []DownloadClientResource
	err := c.do(ctx, request{op: "DownloadClients", method: http.MethodGet, path: c.apiPath + "/downloadclient"}, &out)
	return out, err
}

// Search calls GET /api/v1/search.
//
// THE AMBIGUITY THAT SHAPES EVERYTHING ABOVE THIS CALL: it returns HTTP 200 with
// `[]` when every indexer failed, when all are rate-limited, and when no enabled
// indexer supports the requested categories — identically to a genuine no-results.
// Per-indexer failures are swallowed entirely. The only 400 is
// SearchFailedException, and only when IndexerIDs was explicitly non-empty and
// every named indexer was unavailable.
//
// So a bare `[]` from here means NOTHING on its own. Correlate it with
// IndexerStatus before telling a user there are no results; internal/releases
// does that, and is where callers should be.
//
// Every result carries Prowlarr's admin API key inside DownloadURL and MagnetURL.
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]ReleaseResource, error) {
	q, err := req.Values()
	if err != nil {
		return nil, err
	}
	var out []ReleaseResource
	err = c.do(ctx, request{
		op: "Search", method: http.MethodGet, path: c.apiPath + "/search",
		query: q, timeout: c.timeouts.Search,
	}, &out)
	return out, err
}

// Grab calls POST /api/v1/search with a ReleaseResource body.
//
// It returns 200 and echoes the resource back. There is no download id and no
// queue id: the 200 IS the confirmation.
//
// The handler resolves the release from an in-process cache keyed
// "{indexerId}_{guid}", populated only by a prior GET /api/v1/search, with a
// NON-ROLLING 30-minute TTL that dies with the Prowlarr process. You cannot grab
// a release you did not search for, and you cannot grab more than 30 minutes
// after the search. On ErrNotFound ("Couldn't find requested release in cache, try
// searching again") the correct recovery is a transparent re-search then re-grab —
// internal/releases implements that.
//
// Bulk grab (POST /api/v1/search/bulk) exists but silently drops partial failures
// and ignores per-release downloadClientId, so it is deliberately not wrapped.
func (c *Client) Grab(ctx context.Context, rel ReleaseResource) (ReleaseResource, error) {
	var out ReleaseResource
	if rel.IndexerID <= 0 {
		return out, &APIError{Op: "Grab", Method: http.MethodPost, Path: c.apiPath + "/search", Message: "indexerId must be > 0", Err: ErrInvalidRequest}
	}
	if rel.GUID == "" {
		return out, &APIError{Op: "Grab", Method: http.MethodPost, Path: c.apiPath + "/search", Message: "guid must not be empty", Err: ErrInvalidRequest}
	}
	err := c.do(ctx, request{
		op: "Grab", method: http.MethodPost, path: c.apiPath + "/search",
		body: rel.GrabBody(),
	}, &out)
	return out, err
}
