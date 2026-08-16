// Package ssrf is UsArr's outbound HTTP egress policy. Every outbound request in
// the process goes through it; nothing uses http.DefaultClient, and bypassing it
// is a review-blocking change (docs/DEVELOPMENT.md §10).
//
// Users configure arbitrary internal URLs and UsArr fetches them server-side from
// inside the LAN, which is textbook SSRF-by-design. Jellyfin shipped exactly this
// bug (GHSA-rgjw-4fwc-9v96 / CVE-2021-29490). The defences here are specified in
// docs/reference/security.md §2 and are normative:
//
//   - Three policy classes, chosen by the caller from a database row and never
//     inferred from the URL string. See Class.
//   - Resolve-then-pin: this package resolves the hostname itself, validates every
//     resulting A and AAAA address, and dials a validated IP — while the request
//     keeps its original hostname for Host, SNI and certificate verification.
//     Validation re-runs on every dial, which is what closes the DNS-rebinding
//     window.
//   - At most 3 redirect hops, revalidated at every hop, no scheme change, and no
//     credential — header or query parameter — carried across.
//   - http and https only, plus numeric size caps and layered timeouts.
//
// Usage is one client per service instance (docs/ARCHITECTURE.md §7.5), built
// once when the instance is loaded and reused, so keep-alive works and the
// per-instance circuit breaker has something to count against:
//
//	c, err := ssrf.NewClient(ssrf.Options{
//	        Class:           ssrf.ClassConfigured,
//	        AllowedHostPort: inst.HostPort, // canonical, from ValidateURL at save time
//	        SPKIPin:         inst.TLSSPKIPin,
//	        MaxBytes:        ssrf.MaxListBytes,
//	        TotalTimeout:    ssrf.ListTotalTimeout,
//	})
package ssrf

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

// Layered timeouts, from docs/ARCHITECTURE.md §7.5. Each layer bounds a
// different stall: a total timeout alone lets a slow-loris upstream hold a
// connection for the whole budget while sending one byte a second.
const (
	ConnectTimeout        = 3 * time.Second
	TLSHandshakeTimeout   = 3 * time.Second
	ResponseHeaderTimeout = 10 * time.Second

	// DefaultTotalTimeout applies to everything that is not a list endpoint.
	DefaultTotalTimeout = 10 * time.Second

	// ListTotalTimeout applies to the *Arr list endpoints, which legitimately
	// take tens of seconds on a large library.
	ListTotalTimeout = 60 * time.Second
)

// Options configures one client. Build one per service instance, or one per
// provider, and reuse it.
type Options struct {
	// Class is the egress policy, taken from the row that produced the URL —
	// image_asset.origin_class, the service_instance, the provider template.
	// Required.
	Class Class

	// AllowedHostPort is the canonical host:port this client may reach, as
	// produced by NormalizeHostPort. Required for ClassConfigured. For
	// ClassDerived it names the originating instance, and a request to exactly
	// that host:port is treated as an integration fetch; anything else falls to
	// the strict public-only policy. Ignored for ClassProvider.
	AllowedHostPort string

	// SPKIPin is service_instance.tls_spki_pin: the SHA-256 of the peer leaf
	// certificate's SubjectPublicKeyInfo, recorded on first connect (TOFU).
	//
	// When set, the pin *is* the server authentication for this instance,
	// replacing chain verification — that is the whole point, since the
	// certificate it exists for is a homelab self-signed one that no chain will
	// ever validate. The pin is compared in constant time and the check fails
	// closed. This is the supported alternative to verify_tls=0, which UsArr does
	// not offer at all: an unverified TLS connection would send a full-admin *Arr
	// key to whatever answered the socket.
	//
	// CONSTRAINT, known and currently unfixed: the pin is installed on the whole
	// transport via VerifyPeerCertificate, so a client built WITH a pin can reach
	// exactly one https host — the pinned one. For ClassConfigured that is
	// precisely right, since the dialler already constrains it to AllowedHostPort.
	// For ClassDerived it is not: security.md §2's table says a derived URL that
	// is not on the originating instance should fall through to the strict
	// ClassProvider policy, and with a pin installed those public-CDN fetches are
	// refused instead. It fails CLOSED, which is the right direction, but it
	// silently breaks the documented fallback.
	//
	// Until that is fixed — by selecting the TLS config per connection, or by
	// building ClassDerived clients without the pin and relying on the class
	// policy — do not set SPKIPin on a ClassDerived client that is expected to
	// reach anything but the instance itself.
	SPKIPin []byte

	// MaxBytes caps the response body. Use MaxArtworkBytes, MaxMetadataBytes or
	// MaxListBytes. Zero means MaxMetadataBytes.
	MaxBytes int64

	// TotalTimeout bounds the whole request including body read. Zero means
	// DefaultTotalTimeout; use ListTotalTimeout for *Arr list endpoints.
	TotalTimeout time.Duration

	// Resolver overrides DNS. Zero means net.DefaultResolver.
	Resolver Resolver
}

func (o Options) normalize() (Options, error) {
	if !o.Class.valid() {
		return o, fmt.Errorf("%w: unknown class %d", ErrInvalidOptions, int(o.Class))
	}
	if o.Class == ClassConfigured && o.AllowedHostPort == "" {
		// There is no safe default for "which host may this client reach".
		return o, fmt.Errorf("%w: ClassConfigured requires AllowedHostPort", ErrInvalidOptions)
	}
	if o.AllowedHostPort != "" {
		host, port, err := net.SplitHostPort(o.AllowedHostPort)
		if err != nil {
			return o, fmt.Errorf("%w: AllowedHostPort %q: %w", ErrInvalidOptions, o.AllowedHostPort, err)
		}
		o.AllowedHostPort = canonicalHostPort(host, port)
	}
	if o.MaxBytes == 0 {
		o.MaxBytes = MaxMetadataBytes
	}
	if o.TotalTimeout == 0 {
		o.TotalTimeout = DefaultTotalTimeout
	}
	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}
	return o, nil
}

// policyClient carries the per-client policy into the redirect callback, which
// http.Client hands no other context.
type policyClient struct {
	class           Class
	allowedHostPort string
}

// NewClient builds the one *http.Client for a service instance or provider.
//
// It returns an error rather than a client that fails at request time, so a
// misconfigured instance is caught when it is loaded rather than on the first
// user-visible fetch.
func NewClient(opts Options) (*http.Client, error) {
	opts, err := opts.normalize()
	if err != nil {
		return nil, err
	}
	return newClientWithDial(opts, nil)
}

// newClientWithDial is NewClient with an injectable TCP dial, used by tests to
// point a "public" address at a local listener without weakening the policy.
func newClientWithDial(opts Options, netDial func(context.Context, string, string) (net.Conn, error)) (*http.Client, error) {
	pc := &policyClient{class: opts.Class, allowedHostPort: opts.AllowedHostPort}

	dialer := &pinnedDialer{
		class:           opts.Class,
		allowedHostPort: opts.AllowedHostPort,
		resolver:        opts.Resolver,
		netDial:         netDial,
	}

	tlsCfg, err := tlsConfig(opts.Class, opts.SPKIPin)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		// No proxy, ever. A proxy makes the transport dial the proxy instead of
		// the address this package validated, which discards the pin entirely.
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSClientConfig:       tlsCfg,
		TLSHandshakeTimeout:   TLSHandshakeTimeout,
		ResponseHeaderTimeout: ResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
	}

	return &http.Client{
		Transport:     &cappedTransport{base: transport, class: opts.Class, max: opts.MaxBytes},
		CheckRedirect: pc.checkRedirect,
		Timeout:       opts.TotalTimeout,
	}, nil
}

// SPKIPin computes the value stored in service_instance.tls_spki_pin for a
// certificate: SHA-256 over the DER SubjectPublicKeyInfo. Pinning the key rather
// than the certificate means renewing the certificate with the same key does not
// break the pin.
func SPKIPin(cert *x509.Certificate) []byte {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:]
}

func tlsConfig(class Class, pin []byte) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(pin) == 0 {
		// The normal path: ordinary chain and hostname verification against the
		// system roots, using the original hostname (the transport does the
		// handshake itself, so dialling a pinned IP does not change SNI).
		return cfg, nil
	}
	if len(pin) != sha256.Size {
		return nil, fmt.Errorf("%w: SPKIPin must be %d bytes, got %d", ErrInvalidOptions, sha256.Size, len(pin))
	}

	// Pinned path. InsecureSkipVerify disables the *chain* check only; the
	// VerifyPeerCertificate below is mandatory, fails closed, and authenticates
	// the peer by its public key, which is strictly stronger than a chain for a
	// self-signed homelab certificate. It is never a way to skip verification —
	// if you find yourself setting InsecureSkipVerify anywhere else in UsArr,
	// especially to make a pinned-IP dial verify, that is the bug security.md §2.2
	// item 2 and CONFIGURATION.md §7.1 are both warning about.
	cfg.InsecureSkipVerify = true
	want := append([]byte(nil), pin...)
	cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return policyErr("tls", class, "", netip.Addr{}, fmt.Errorf("%w: peer sent no certificate", ErrPinMismatch))
		}
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return policyErr("tls", class, "", netip.Addr{}, fmt.Errorf("%w: parse leaf: %w", ErrPinMismatch, err))
		}
		got := SPKIPin(leaf)
		if subtle.ConstantTimeCompare(got, want) != 1 {
			return policyErr("tls", class, "", netip.Addr{}, ErrPinMismatch)
		}
		return nil
	}
	return cfg, nil
}

// ValidateURL checks a URL against a class before it is used or stored, and
// returns the parsed URL alongside the canonical host:port to persist in
// service_instance / to pass back as Options.AllowedHostPort.
//
// It performs DNS, so it takes a context. It is a pre-flight check, not a
// substitute for the per-dial validation: between this call and the request the
// answer can change, which is why the dialer revalidates.
//
// For ClassConfigured with an empty Options.AllowedHostPort the URL defines its
// own allowlist — that is the admin-just-typed-it case. Everywhere else the
// allowlist must already exist.
func ValidateURL(ctx context.Context, rawURL string, opts Options) (*url.URL, string, error) {
	if !opts.Class.valid() {
		return nil, "", fmt.Errorf("%w: unknown class %d", ErrInvalidOptions, int(opts.Class))
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("ssrf: parse url: %w", err)
	}
	if err := CheckScheme(u.Scheme); err != nil {
		return nil, "", policyErr("validate", opts.Class, u.Host, netip.Addr{}, err)
	}
	hostPort, err := NormalizeHostPort(u)
	if err != nil {
		return nil, "", policyErr("validate", opts.Class, u.Host, netip.Addr{}, err)
	}

	allowed := opts.AllowedHostPort
	if allowed != "" {
		if host, port, splitErr := net.SplitHostPort(allowed); splitErr == nil {
			allowed = canonicalHostPort(host, port)
		}
	} else if opts.Class == ClassConfigured {
		allowed = hostPort
	}

	eff, err := authorizeHostPort(opts.Class, allowed, hostPort)
	if err != nil {
		return nil, "", err
	}

	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	d := &pinnedDialer{class: opts.Class, allowedHostPort: allowed, resolver: resolver}
	addrs, err := d.resolve(ctx, "tcp", u.Hostname())
	if err != nil {
		return nil, "", err
	}
	if len(addrs) == 0 {
		return nil, "", policyErr("validate", eff, u.Hostname(), netip.Addr{}, ErrNoAddress)
	}
	for _, a := range addrs {
		if err := CheckAddr(a, eff); err != nil {
			return nil, "", policyErr("validate", eff, u.Hostname(), a, ErrBlockedAddress)
		}
	}
	return u, hostPort, nil
}
