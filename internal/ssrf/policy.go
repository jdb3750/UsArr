package ssrf

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// Class selects the egress policy. It is chosen by the caller from the database
// row that produced the URL — image_asset.origin_class, service_instance, the
// provider template — and is never inferred from the URL string. Inferring it
// from the string is the bug: a derived URL that happens to look like a provider
// URL is still attacker-influenced. See docs/reference/security.md §2.
type Class int

const (
	// ClassConfigured is a URL an admin typed into the service form. It may reach
	// private address space — that is the entire point of a homelab hub — but only
	// the one validated host:port of that service_instance.
	ClassConfigured Class = iota + 1

	// ClassProvider is a URL UsArr constructed from a known public provider
	// template (TMDB, Fanart.tv, Cover Art Archive). Public internet only.
	ClassProvider

	// ClassDerived is a URL read out of an upstream response body — MediaCover.remoteUrl,
	// a Prowlarr ReleaseResource.posterUrl, a media server's cover field. Nobody
	// configured it, and on the *Arr side it can carry indexer- and NFO-supplied
	// strings. Allowed to the originating instance's own host:port, or else the
	// strict ClassProvider policy. Nothing in between.
	ClassDerived
)

func (c Class) String() string {
	switch c {
	case ClassConfigured:
		return "configured"
	case ClassProvider:
		return "provider"
	case ClassDerived:
		return "derived"
	default:
		return fmt.Sprintf("class(%d)", int(c))
	}
}

func (c Class) valid() bool {
	return c == ClassConfigured || c == ClassProvider || c == ClassDerived
}

func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic("ssrf: bad built-in prefix " + s + ": " + err.Error())
	}
	return p
}

// nonPublicV4 is everything IANA does not consider globally reachable, plus the
// two cloud-metadata addresses called out by name in security.md §2.
//
// 100.64.0.0/10 (CGNAT) is here on purpose and only here: the strict public-only
// policy denies it, because a *TMDB* URL resolving to a tailnet peer is genuinely
// wrong. It is deliberately NOT a blanket denylist entry — on the flagship
// deployment the user's Komga, Jellyfin, Audiobookshelf and Navidrome *are*
// tailnet devices at 100.x.y.z and their artwork must load. Instance-originated
// traffic reaches them through the host:port allowlist instead of through a CIDR
// exception. See security.md §2.1.
var nonPublicV4 = []netip.Prefix{
	mustPrefix("0.0.0.0/8"),       // "this network"
	mustPrefix("10.0.0.0/8"),      // RFC1918
	mustPrefix("100.64.0.0/10"),   // CGNAT / tailnet; includes 100.100.100.100 (Alibaba metadata)
	mustPrefix("127.0.0.0/8"),     // loopback
	mustPrefix("169.254.0.0/16"),  // link-local; includes 169.254.169.254 (EC2/GCP/Azure metadata)
	mustPrefix("172.16.0.0/12"),   // RFC1918
	mustPrefix("192.0.0.0/24"),    // IETF protocol assignments
	mustPrefix("192.0.2.0/24"),    // TEST-NET-1
	mustPrefix("192.88.99.0/24"),  // 6to4 relay anycast
	mustPrefix("192.168.0.0/16"),  // RFC1918
	mustPrefix("198.18.0.0/15"),   // benchmarking
	mustPrefix("198.51.100.0/24"), // TEST-NET-2
	mustPrefix("203.0.113.0/24"),  // TEST-NET-3
	mustPrefix("224.0.0.0/4"),     // multicast
	mustPrefix("240.0.0.0/4"),     // reserved, and 255.255.255.255 broadcast
}

// nonPublicV6 mirrors the above. The transition ranges (NAT64, 6to4, Teredo)
// are denied wholesale rather than unwrapped: they embed an IPv4 address that a
// naive check would never see, and UsArr has no reason to speak to any of them.
var nonPublicV6 = []netip.Prefix{
	mustPrefix("::/128"),        // unspecified
	mustPrefix("::1/128"),       // loopback
	mustPrefix("64:ff9b::/96"),  // NAT64 well-known
	mustPrefix("64:ff9b:1::/48"), // NAT64 local-use
	mustPrefix("100::/64"),      // discard-only
	mustPrefix("2001::/23"),     // IETF protocol assignments, includes Teredo 2001::/32
	mustPrefix("2001:db8::/32"), // documentation
	mustPrefix("2002::/16"),     // 6to4
	mustPrefix("fc00::/7"),      // unique local
	mustPrefix("fe80::/10"),     // link-local
	mustPrefix("ff00::/8"),      // multicast
}

// CheckAddr applies the address policy for a class to one resolved address.
//
// It takes a netip.Addr, never a string. Encoding bypasses (0x7f.1, 2130706433,
// 0177.0.0.1, [::ffff:127.0.0.1], and the mixed forms) defeat string checks and
// parser behaviour differs between libraries; the only safe move is to let the
// resolver produce addresses and test those. security.md §2.2 item 4.
func CheckAddr(addr netip.Addr, class Class) error {
	if !class.valid() {
		return fmt.Errorf("%w: unknown class %d", ErrInvalidOptions, int(class))
	}
	if !addr.IsValid() {
		return policyErr("validate", class, "", addr, ErrBlockedAddress)
	}

	// An IPv4-mapped IPv6 address is an IPv4 address wearing a hat. Unmap before
	// testing so [::ffff:127.0.0.1] is judged as 127.0.0.1.
	addr = addr.Unmap()

	blocked := func() error {
		return policyErr("validate", class, "", addr, ErrBlockedAddress)
	}

	// Never a valid destination for anything, in any class.
	if addr.IsUnspecified() || addr.IsMulticast() || addr.IsInterfaceLocalMulticast() {
		return blocked()
	}

	if class == ClassConfigured {
		// An admin typed this host into the service form and it is pinned to one
		// host:port. Loopback, RFC1918, CGNAT and ULA are all ordinary homelab
		// destinations, so there is nothing further to deny.
		return nil
	}

	// ClassProvider, and ClassDerived that did not match its originating instance.
	prefixes := nonPublicV4
	if addr.Is6() {
		prefixes = nonPublicV6
	}
	for _, p := range prefixes {
		if p.Contains(addr) {
			return blocked()
		}
	}
	// Backstop for anything the table above missed (e.g. future reservations that
	// the stdlib already knows about).
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return blocked()
	}
	return nil
}

// CheckScheme rejects everything that is not http or https: no file:, gopher:,
// dict:, ftp:. security.md §2.2 item 5.
func CheckScheme(scheme string) error {
	switch strings.ToLower(scheme) {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrScheme, scheme)
	}
}

// NormalizeHostPort renders a URL's authority as a canonical "host:port" for
// allowlist comparison: lowercased, default port made explicit, trailing root
// dot removed (example.com. and example.com are the same name), IPv6 literals
// unmapped and re-bracketed. Store the result in service_instance and compare
// against it later; comparing raw url.Host strings is how "example.com." and
// "EXAMPLE.com:443" slip past an allowlist.
func NormalizeHostPort(u *url.URL) (string, error) {
	if u == nil {
		return "", fmt.Errorf("%w: nil url", ErrInvalidOptions)
	}
	if err := CheckScheme(u.Scheme); err != nil {
		return "", err
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("%w: url has no host", ErrInvalidOptions)
	}
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return canonicalHostPort(host, port), nil
}

// canonicalHostPort is the single place that decides what two hosts being "the
// same host" means. Everything that compares hosts goes through it.
func canonicalHostPort(host, port string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	host = strings.Trim(host, "[]")
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.Unmap().String()
	}
	return net.JoinHostPort(host, port)
}

// hostPortFromDialAddr canonicalises the "host:port" the http.Transport hands to
// DialContext. The transport always supplies an explicit port.
func hostPortFromDialAddr(addr string) (host, hostPort string, err error) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("ssrf: parse dial address: %w", err)
	}
	h = strings.TrimSuffix(strings.ToLower(h), ".")
	return h, canonicalHostPort(h, p), nil
}

// effectiveClass resolves ClassDerived to the policy that actually applies to
// this host:port: the originating instance's own host:port is an integration
// fetch, anything else gets the strict public-only policy. Nothing in between.
func effectiveClass(class Class, allowedHostPort, hostPort string) Class {
	if class == ClassDerived && allowedHostPort != "" && hostPort == allowedHostPort {
		return ClassConfigured
	}
	return class
}

// authorizeHostPort decides whether this client may talk to this host:port at all,
// before any DNS lookup happens.
func authorizeHostPort(class Class, allowedHostPort, hostPort string) (Class, error) {
	eff := effectiveClass(class, allowedHostPort, hostPort)
	if class == ClassConfigured && hostPort != allowedHostPort {
		// A configured client is pinned to its one service instance. This is what
		// stops a request parameter, or a redirect, from steering an *Arr client
		// carrying a full-admin X-Api-Key at some other host.
		return eff, policyErr("validate", class, hostPort, netip.Addr{}, ErrHostNotAllowed)
	}
	return eff, nil
}
