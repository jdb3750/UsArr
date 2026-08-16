package ssrf

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeResolver stands in for DNS. Hostnames map to whatever addresses the test
// wants, including the addresses a permissive system resolver would invent for
// the legacy numeric forms.
type fakeResolver struct {
	mu    sync.Mutex
	zones map[string][]netip.Addr
	// answers, when set for a name, is consumed one entry per lookup so a test
	// can simulate a rebinding resolver that changes its answer between dials.
	answers map[string][][]netip.Addr
	calls   map[string]int
}

func newFakeResolver(zones map[string][]netip.Addr) *fakeResolver {
	return &fakeResolver{zones: zones, calls: map[string]int{}}
}

func (f *fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.TrimSuffix(strings.ToLower(host), ".")
	f.calls[name]++
	if seq, ok := f.answers[name]; ok && len(seq) > 0 {
		ans := seq[0]
		if len(seq) > 1 {
			f.answers[name] = seq[1:]
		}
		return ans, nil
	}
	addrs, ok := f.zones[name]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return addrs, nil
}

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, 0, len(ss))
	for _, s := range ss {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

// redirectDialTo sends every validated dial at one real listener, so tests can
// use plausible public addresses without touching the network. It runs *after*
// the address policy, so a blocked address never reaches it.
func redirectDialTo(target string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := &net.Dialer{Timeout: ConnectTimeout}
		if network == "tcp4" || network == "tcp6" {
			network = "tcp"
		}
		return d.DialContext(ctx, network, target)
	}
}

func mustClient(t *testing.T, opts Options, dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	t.Helper()
	norm, err := opts.normalize()
	if err != nil {
		t.Fatalf("normalize options: %v", err)
	}
	c, err := newClientWithDial(norm, dial)
	if err != nil {
		t.Fatalf("newClientWithDial: %v", err)
	}
	return c
}

func serverHostPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

// get issues a GET through the client under test. Tests go through it rather
// than client.Get so every request carries a context and no body is leaked.
func get(t *testing.T, c *http.Client, rawURL string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("build request for %s: %v", rawURL, err)
	}
	return c.Do(req)
}

// getOK requires the request to succeed, and returns the body.
func getOK(t *testing.T, c *http.Client, rawURL string) []byte {
	t.Helper()
	resp, err := get(t, c, rawURL)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", rawURL, err)
	}
	return body
}

// getErr requires the request to fail, and returns the error.
func getErr(t *testing.T, c *http.Client, rawURL string) error {
	t.Helper()
	resp, err := get(t, c, rawURL)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("GET %s: want an error, got %s", rawURL, resp.Status)
	}
	return err
}

// ---------------------------------------------------------------------------
// Encoding bypasses
// ---------------------------------------------------------------------------

// TestEncodingBypassesAreBlocked covers the forms named in security.md §2.2
// item 4. The point is not that this package parses them — it must not try. A
// permissive system resolver happily turns "0x7f.1" and "2130706433" into
// 127.0.0.1, so the defence has to be validating the *resolved address*. The
// fake resolver here behaves like getaddrinfo does.
func TestEncodingBypassesAreBlocked(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("request reached the server; the encoding bypass was not blocked")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	loopback := addrs("127.0.0.1")
	res := newFakeResolver(map[string][]netip.Addr{
		"0x7f.1":           loopback, // hex, two-part
		"0x7f000001":       loopback, // hex dword
		"2130706433":       loopback, // decimal dword
		"0177.0.0.1":       loopback, // octal
		"127.1":            loopback, // short form
		"0x7f.0x0.0x0.0x1": loopback,
		"017700000001":     loopback, // octal dword
	})

	hosts := []string{
		"0x7f.1",
		"0x7f000001",
		"2130706433",
		"0177.0.0.1",
		"127.1",
		"0x7f.0x0.0x0.0x1",
		"017700000001",
		// Literals this package parses itself, so the resolver is never consulted.
		"[::ffff:127.0.0.1]",
		"127.0.0.1.",
		"[::1]",
		"[0:0:0:0:0:ffff:7f00:1]",
	}

	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	for _, host := range hosts {
		err := getErr(t, client, "http://"+host+"/latest/meta-data/")
		if !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("http://%s/: got %v, want ErrBlockedAddress", host, err)
		}
		if !IsPolicyError(err) {
			t.Errorf("http://%s/: want a policy error, got %v", host, err)
		}
	}
}

func TestTrailingDotIsNotAnAllowlistBypass(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"sonarr.lan": addrs("10.0.0.5")})
	client := mustClient(t,
		Options{Class: ClassConfigured, AllowedHostPort: "sonarr.lan:8989", Resolver: res},
		redirectDialTo(srv.Listener.Addr().String()))

	// "sonarr.lan." and "sonarr.lan" are the same name and must be treated as such.
	getOK(t, client, "http://sonarr.lan.:8989/api/v3/system/status")
}

// ---------------------------------------------------------------------------
// Class policy
// ---------------------------------------------------------------------------

func TestConfiguredReachesPrivateSpaceButOnlyItsOwnHost(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{
		"sonarr.lan": addrs("192.168.1.20"),
		"radarr.lan": addrs("192.168.1.21"),
	})
	client := mustClient(t,
		Options{Class: ClassConfigured, AllowedHostPort: "sonarr.lan:8989", Resolver: res},
		redirectDialTo(srv.Listener.Addr().String()))

	getOK(t, client, "http://sonarr.lan:8989/api/v3/series")

	// Same private network, different instance: refused. This is what stops a
	// full-admin X-Api-Key being sent somewhere it does not belong.
	if err := getErr(t, client, "http://radarr.lan:8989/api/v3/movie"); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("configured client reached another host: got %v, want ErrHostNotAllowed", err)
	}
	// Same host, different port: also refused.
	if err := getErr(t, client, "http://sonarr.lan:9999/api/v3/series"); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("configured client reached another port: got %v, want ErrHostNotAllowed", err)
	}
}

func TestProviderIsPublicOnly(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{
		"api.themoviedb.org": addrs("93.184.216.34"),
		"rebound.example":    addrs("169.254.169.254"),
		"tailnet.example":    addrs("100.64.1.5"),
	})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	getOK(t, client, "http://api.themoviedb.org/3/movie/550")

	if err := getErr(t, client, "http://rebound.example/latest/meta-data/"); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("cloud metadata reachable from provider class: %v", err)
	}
	// A TMDB URL resolving to a tailnet peer is genuinely wrong; security.md §2.1.
	if err := getErr(t, client, "http://tailnet.example/cover.jpg"); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("tailnet reachable from provider class: %v", err)
	}
}

func TestDerivedAllowsOnlyItsOriginatingInstance(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{
		"komga.tailnet": addrs("100.64.1.5"), // a real tailnet backend, per §2.1
		"prowlarr.lan":  addrs("192.168.1.30"),
		"metadata.aws":  addrs("169.254.169.254"),
		"cdn.example":   addrs("93.184.216.34"),
	})
	opts := Options{Class: ClassDerived, AllowedHostPort: "komga.tailnet:8080", Resolver: res}
	client := mustClient(t, opts, redirectDialTo(srv.Listener.Addr().String()))

	// The originating instance's own host:port, on a tailnet address: allowed.
	getOK(t, client, "http://komga.tailnet:8080/api/v1/books/1/thumbnail")

	// A posterUrl pointing at the cloud metadata service. This is the CVE-2021-29490
	// shape with the attacker one hop upstream.
	if err := getErr(t, client, "http://metadata.aws/latest/meta-data/"); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("derived fetch reached cloud metadata: %v", err)
	}
	// A posterUrl pointing at a *different* private backend: nothing in between.
	if err := getErr(t, client, "http://prowlarr.lan:9696/x.jpg"); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("derived fetch reached another private host: %v", err)
	}
	// A genuinely public posterUrl falls through to the provider policy: allowed.
	getOK(t, client, "http://cdn.example/poster.jpg")
}

// ---------------------------------------------------------------------------
// Resolve-then-pin
// ---------------------------------------------------------------------------

// TestValidationRerunsOnEveryDial is the DNS-rebinding case. The name resolves
// publicly the first time and to loopback the second; validating once at
// configuration time would let the second request through.
func TestValidationRerunsOnEveryDial(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Defeat keep-alive so the second request has to dial again, which is
		// precisely the moment revalidation has to happen.
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := newFakeResolver(nil)
	res.answers = map[string][][]netip.Addr{
		"rebind.example": {addrs("93.184.216.34"), addrs("127.0.0.1")},
	}
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	getOK(t, client, "http://rebind.example/first")

	if err := getErr(t, client, "http://rebind.example/second"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("second request after rebinding: got %v, want ErrBlockedAddress", err)
	}
}

// TestAllResolvedAddressesMustPass: a hostile resolver answering with a public
// address alongside a private one must not get the private one dialled.
func TestAllResolvedAddressesMustPass(t *testing.T) {
	t.Parallel()

	res := newFakeResolver(map[string][]netip.Addr{
		"mixed.example": addrs("93.184.216.34", "127.0.0.1"),
	})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res},
		func(context.Context, string, string) (net.Conn, error) {
			t.Error("dialled despite a poisoned answer set")
			return nil, errors.New("unreachable")
		})

	if err := getErr(t, client, "http://mixed.example/"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want ErrBlockedAddress", err)
	}
}

func TestBothFamiliesAreValidated(t *testing.T) {
	t.Parallel()

	// The AAAA record is the private one. Validating only A would leave it open.
	res := newFakeResolver(map[string][]netip.Addr{
		"dual.example": addrs("93.184.216.34", "fd00::1"),
	})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res},
		func(context.Context, string, string) (net.Conn, error) {
			t.Error("dialled despite a private AAAA record")
			return nil, errors.New("unreachable")
		})

	if err := getErr(t, client, "http://dual.example/"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want ErrBlockedAddress", err)
	}
}

func TestRequestKeepsOriginalHostname(t *testing.T) {
	t.Parallel()

	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"sonarr.lan": addrs("192.168.1.20")})
	client := mustClient(t,
		Options{Class: ClassConfigured, AllowedHostPort: "sonarr.lan:8989", Resolver: res},
		redirectDialTo(srv.Listener.Addr().String()))

	getOK(t, client, "http://sonarr.lan:8989/api/v3/system/status")

	// The dial went to a pinned address; the request must still carry the name.
	// This is what keeps Host, SNI and certificate verification correct, and what
	// removes the temptation to reach for InsecureSkipVerify.
	if gotHost != "sonarr.lan:8989" {
		t.Errorf("Host header = %q, want %q", gotHost, "sonarr.lan:8989")
	}
}

// ---------------------------------------------------------------------------
// Redirects
// ---------------------------------------------------------------------------

func TestRedirectToLoopbackIsBlocked(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "http://evil.example/latest/meta-data/", http.StatusFound)
		default:
			t.Errorf("reached %s; the redirect was followed to a blocked address", r.URL.Path)
		}
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{
		"public.example": addrs("93.184.216.34"),
		"evil.example":   addrs("127.0.0.1"),
	})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	if err := getErr(t, client, "http://public.example/start"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("redirect to loopback: got %v, want ErrBlockedAddress", err)
	}
}

func TestRedirectToLiteralLoopbackIsBlocked(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "http://127.0.0.1:1/", http.StatusFound)
			return
		}
		t.Errorf("reached %s", r.URL.Path)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	if err := getErr(t, client, "http://public.example/start"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want ErrBlockedAddress", err)
	}
}

func TestRedirectOffTheConfiguredInstanceIsBlocked(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/series" {
			http.Redirect(w, r, "http://attacker.example/collect", http.StatusFound)
			return
		}
		t.Errorf("reached %s with the *Arr key attached", r.URL.Path)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{
		"sonarr.lan":       addrs("192.168.1.20"),
		"attacker.example": addrs("93.184.216.34"),
	})
	client := mustClient(t,
		Options{Class: ClassConfigured, AllowedHostPort: "sonarr.lan:8989", Resolver: res},
		redirectDialTo(srv.Listener.Addr().String()))

	if err := getErr(t, client, "http://sonarr.lan:8989/api/v3/series"); !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("got %v, want ErrHostNotAllowed", err)
	}
}

func TestRedirectStripsCredentials(t *testing.T) {
	t.Parallel()

	var second *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			// The upstream nominates a URL that re-attaches the credentials.
			http.Redirect(w, r, "/second?apikey=leaked&token=leaked&sig=leaked&s=leaked&t=leaked&p=leaked&keep=1",
				http.StatusFound)
		case "/second":
			clone := r.Clone(context.Background())
			second = clone
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://public.example/first?api_key=originalsecret", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer originalsecret")
	req.Header.Set("X-Api-Key", "0123456789abcdef")
	req.Header.Set("Cookie", "session=originalsecret")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = resp.Body.Close()

	if second == nil {
		t.Fatal("the redirect was not followed")
	}
	for _, h := range credentialHeaders {
		if v := second.Header.Get(h); v != "" {
			t.Errorf("%s survived the redirect with value %q", h, v)
		}
	}
	q := second.URL.Query()
	for _, name := range []string{"apikey", "token", "sig", "s", "t", "p", "api_key"} {
		if q.Has(name) {
			t.Errorf("credential parameter %q survived the redirect", name)
		}
	}
	if q.Get("keep") != "1" {
		t.Errorf("a non-credential parameter was dropped: %v", second.URL.RawQuery)
	}
}

func TestRedirectHopLimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := 0
		_, _ = fmt.Sscanf(r.URL.Path, "/hop/%d", &n)
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n+1), http.StatusFound)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	if err := getErr(t, client, "http://public.example/hop/0"); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("got %v, want ErrTooManyRedirects", err)
	}
}

func TestRedirectWithinBudgetSucceeds(t *testing.T) {
	t.Parallel()

	// TMDB, Cover Art Archive and Fanart.tv all redirect; the budget has to be
	// usable or someone will turn the control off wholesale.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, "/b", http.StatusFound)
		case "/b":
			http.Redirect(w, r, "/c", http.StatusFound)
		default:
			_, _ = w.Write([]byte("cover bytes"))
		}
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res}, redirectDialTo(srv.Listener.Addr().String()))

	if body := getOK(t, client, "http://public.example/a"); string(body) != "cover bytes" {
		t.Errorf("body = %q", body)
	}
}

func TestRedirectSchemeChangeIsRejected(t *testing.T) {
	t.Parallel()

	pc := &policyClient{class: ClassProvider}
	ctx := context.Background()
	mk := func(from, to string) error {
		prev, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
		if err != nil {
			t.Fatal(err)
		}
		next, err := http.NewRequestWithContext(ctx, http.MethodGet, to, nil)
		if err != nil {
			t.Fatal(err)
		}
		return pc.checkRedirect(next, []*http.Request{prev})
	}

	if err := mk("https://cdn.example/a", "http://cdn.example/b"); !errors.Is(err, ErrRedirectScheme) {
		t.Errorf("https -> http downgrade: got %v, want ErrRedirectScheme", err)
	}
	if err := mk("http://cdn.example/a", "https://cdn.example/b"); !errors.Is(err, ErrRedirectScheme) {
		t.Errorf("http -> https cross-scheme: got %v, want ErrRedirectScheme", err)
	}
	if err := mk("https://cdn.example/a", "ftp://cdn.example/b"); !errors.Is(err, ErrScheme) {
		t.Errorf("https -> ftp: got %v, want ErrScheme", err)
	}
	if err := mk("https://cdn.example/a", "https://cdn.example/b"); err != nil {
		t.Errorf("same-scheme hop was rejected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Schemes, caps, timeouts
// ---------------------------------------------------------------------------

func TestNonHTTPSchemesAreDenied(t *testing.T) {
	t.Parallel()

	client := mustClient(t, Options{Class: ClassProvider, Resolver: newFakeResolver(nil)}, nil)

	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://127.0.0.1:70/_x",
		"dict://127.0.0.1:2628/",
		"ftp://127.0.0.1/",
	} {
		if err := getErr(t, client, raw); !errors.Is(err, ErrScheme) {
			t.Errorf("%s: got %v, want ErrScheme", raw, err)
		}
	}
}

func TestResponseCapDeclaredLength(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("x", 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res, MaxBytes: 1024},
		redirectDialTo(srv.Listener.Addr().String()))

	if err := getErr(t, client, "http://public.example/big.jpg"); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("got %v, want ErrResponseTooLarge", err)
	}
}

func TestResponseCapChunkedFailsRatherThanTruncates(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Length: the cap must hold on the read path too.
		flusher, _ := w.(http.Flusher)
		for range 8 {
			_, _ = io.WriteString(w, strings.Repeat("y", 1024))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res, MaxBytes: 2048},
		redirectDialTo(srv.Listener.Addr().String()))

	resp, err := get(t, client, "http://public.example/stream")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// A silently truncated body is worse than a failure: truncated JSON reads as
	// an upstream bug, and a truncated *Arr list reads as deleted content.
	if _, err := io.ReadAll(resp.Body); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("reading an over-cap body: got %v, want ErrResponseTooLarge", err)
	}
}

func TestResponseUnderCapIsIntact(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("z", 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res, MaxBytes: 2048},
		redirectDialTo(srv.Listener.Addr().String()))

	// A body exactly at the cap must read cleanly.
	if body := getOK(t, client, "http://public.example/ok"); len(body) != len(payload) {
		t.Errorf("read %d bytes, want %d", len(body), len(payload))
	}
}

func TestTotalTimeoutIsEnforced(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer func() { close(release); srv.Close() }()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res, TotalTimeout: 150 * time.Millisecond},
		redirectDialTo(srv.Listener.Addr().String()))

	start := time.Now()
	if err := getErr(t, client, "http://public.example/slow"); IsPolicyError(err) {
		t.Fatalf("a timeout must not be reported as a policy refusal: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("timeout took %v", elapsed)
	}
}

func TestDialFailureIsNotAPolicyError(t *testing.T) {
	t.Parallel()

	res := newFakeResolver(map[string][]netip.Addr{"public.example": addrs("93.184.216.34")})
	client := mustClient(t, Options{Class: ClassProvider, Resolver: res},
		func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		})

	// The circuit breaker counts network failures; it must not count refusals
	// that came from our own policy, and vice versa.
	if err := getErr(t, client, "http://public.example/"); IsPolicyError(err) {
		t.Errorf("a dial failure was classified as a policy error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TLS: SPKI pinning
// ---------------------------------------------------------------------------

func TestSPKIPinAcceptsTheRecordedKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	leaf := srv.Certificate()
	hostPort := serverHostPort(t, srv)

	client := mustClient(t, Options{
		Class:           ClassConfigured,
		AllowedHostPort: hostPort,
		SPKIPin:         SPKIPin(leaf),
	}, nil)

	getOK(t, client, srv.URL+"/api/v3/system/status")
}

func TestSPKIPinRejectsAMismatchedKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer srv.Close()

	// A pin recorded from some other instance's key: one bit different is enough.
	wrong := SPKIPin(srv.Certificate())
	wrong[0] ^= 0xff

	client := mustClient(t, Options{
		Class:           ClassConfigured,
		AllowedHostPort: serverHostPort(t, srv),
		SPKIPin:         wrong,
	}, nil)

	if err := getErr(t, client, srv.URL+"/"); !errors.Is(err, ErrPinMismatch) {
		t.Fatalf("got %v, want ErrPinMismatch", err)
	}
}

func TestWithoutAPinVerificationStaysOn(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer srv.Close()

	client := mustClient(t, Options{
		Class:           ClassConfigured,
		AllowedHostPort: serverHostPort(t, srv),
	}, nil)

	// No pin means ordinary chain verification, and this certificate chains to
	// nothing. If this test ever stops failing, someone has set InsecureSkipVerify.
	err := getErr(t, client, srv.URL+"/")
	var unknown x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if !errors.As(err, &unknown) && !errors.As(err, &hostErr) {
		t.Fatalf("want a certificate verification failure, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ValidateURL
// ---------------------------------------------------------------------------

func TestValidateURL(t *testing.T) {
	t.Parallel()

	res := newFakeResolver(map[string][]netip.Addr{
		"sonarr.lan":         addrs("192.168.1.20"),
		"api.themoviedb.org": addrs("93.184.216.34"),
		"rebound.example":    addrs("127.0.0.1"),
	})
	ctx := context.Background()

	// An admin-typed URL defines its own allowlist; the canonical host:port comes
	// back for storage in service_instance.
	u, hostPort, err := ValidateURL(ctx, "http://SONARR.LAN:8989/api/v3/", Options{Class: ClassConfigured, Resolver: res})
	if err != nil {
		t.Fatalf("configured url: %v", err)
	}
	if hostPort != "sonarr.lan:8989" {
		t.Errorf("hostPort = %q, want sonarr.lan:8989", hostPort)
	}
	if u.Path != "/api/v3/" {
		t.Errorf("path = %q", u.Path)
	}

	if _, _, err := ValidateURL(ctx, "https://api.themoviedb.org/3/movie/550", Options{Class: ClassProvider, Resolver: res}); err != nil {
		t.Errorf("public provider url: %v", err)
	}
	if _, _, err := ValidateURL(ctx, "http://rebound.example/", Options{Class: ClassProvider, Resolver: res}); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("provider url resolving to loopback: got %v, want ErrBlockedAddress", err)
	}
	if _, _, err := ValidateURL(ctx, "file:///etc/passwd", Options{Class: ClassProvider, Resolver: res}); !errors.Is(err, ErrScheme) {
		t.Errorf("file url: got %v, want ErrScheme", err)
	}
	if _, _, err := ValidateURL(ctx, "http://nx.example/", Options{Class: ClassProvider, Resolver: res}); err == nil {
		t.Error("want a resolution failure for an unknown host")
	} else if IsPolicyError(err) {
		t.Errorf("NXDOMAIN must not read as a policy refusal: %v", err)
	}
}
