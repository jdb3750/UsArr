package ssrf

import (
	"errors"
	"net/netip"
	"net/url"
	"testing"
)

func TestCheckAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		addr            string
		allowProvider   bool
		allowConfigured bool
		why             string
	}{
		// Public internet.
		{"8.8.8.8", true, true, "ordinary public v4"},
		{"93.184.216.34", true, true, "ordinary public v4"},
		{"2606:4700:4700::1111", true, true, "ordinary public v6"},

		// Loopback in every disguise.
		{"127.0.0.1", false, true, "loopback"},
		{"127.1.2.3", false, true, "loopback is a /8, not one address"},
		{"::1", false, true, "v6 loopback"},
		{"::ffff:127.0.0.1", false, true, "v4-mapped loopback must be unmapped before testing"},

		// Unspecified and multicast are never a destination, in any class.
		{"0.0.0.0", false, false, "unspecified"},
		{"::", false, false, "unspecified v6"},
		{"224.0.0.1", false, false, "multicast"},
		{"ff02::1", false, false, "v6 multicast"},

		// Cloud metadata, named explicitly in security.md §2.
		{"169.254.169.254", false, true, "EC2/GCP/Azure metadata"},
		{"100.100.100.100", false, true, "Alibaba metadata"},

		// Link-local.
		{"169.254.1.1", false, true, "v4 link-local"},
		{"fe80::1", false, true, "v6 link-local"},

		// RFC1918 and ULA.
		{"10.1.2.3", false, true, "rfc1918"},
		{"172.16.0.1", false, true, "rfc1918"},
		{"172.31.255.254", false, true, "rfc1918"},
		{"192.168.1.10", false, true, "rfc1918"},
		{"fd12:3456::1", false, true, "unique local"},

		// CGNAT: denied for provider, allowed for an instance-originated fetch.
		// This is security.md §2.1 — the tailnet is reached via the host:port
		// allowlist, never via a CIDR hole in the provider policy.
		{"100.64.0.1", false, true, "tailnet"},
		{"100.127.255.255", false, true, "tailnet"},

		// Reserved / transition ranges.
		{"192.0.2.1", false, true, "TEST-NET-1"},
		{"198.51.100.1", false, true, "TEST-NET-2"},
		{"203.0.113.1", false, true, "TEST-NET-3"},
		{"198.18.0.1", false, true, "benchmarking"},
		{"192.0.0.1", false, true, "IETF protocol assignments"},
		{"192.88.99.1", false, true, "6to4 relay anycast"},
		{"240.0.0.1", false, true, "reserved"},
		{"255.255.255.255", false, true, "broadcast"},
		{"2001:db8::1", false, true, "v6 documentation"},
		{"2002:7f00:1::", false, true, "6to4 embeds a v4 address"},
		{"64:ff9b::7f00:1", false, true, "NAT64 embeds a v4 address"},
		{"2001::1", false, true, "Teredo"},
	}

	for _, tc := range cases {
		addr := netip.MustParseAddr(tc.addr)

		got := CheckAddr(addr, ClassProvider) == nil
		if got != tc.allowProvider {
			t.Errorf("CheckAddr(%s, provider) allowed=%v, want %v (%s)", tc.addr, got, tc.allowProvider, tc.why)
		}

		got = CheckAddr(addr, ClassConfigured) == nil
		if got != tc.allowConfigured {
			t.Errorf("CheckAddr(%s, configured) allowed=%v, want %v (%s)", tc.addr, got, tc.allowConfigured, tc.why)
		}

		// ClassDerived that did not match its instance is exactly ClassProvider.
		got = CheckAddr(addr, ClassDerived) == nil
		if got != tc.allowProvider {
			t.Errorf("CheckAddr(%s, derived) allowed=%v, want %v (%s)", tc.addr, got, tc.allowProvider, tc.why)
		}
	}
}

func TestCheckAddrErrorIsTyped(t *testing.T) {
	t.Parallel()

	err := CheckAddr(netip.MustParseAddr("127.0.0.1"), ClassProvider)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("want ErrBlockedAddress, got %v", err)
	}
	if !IsPolicyError(err) {
		t.Fatal("blocked address must be reported as a policy error, not a network failure")
	}
}

func TestCheckAddrRejectsUnknownClass(t *testing.T) {
	t.Parallel()

	if err := CheckAddr(netip.MustParseAddr("8.8.8.8"), Class(0)); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("want ErrInvalidOptions for the zero class, got %v", err)
	}
}

func TestCheckScheme(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"http", "https", "HTTP", "HTTPS"} {
		if err := CheckScheme(s); err != nil {
			t.Errorf("CheckScheme(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range []string{"file", "gopher", "dict", "ftp", "ldap", "jar", ""} {
		if err := CheckScheme(s); !errors.Is(err, ErrScheme) {
			t.Errorf("CheckScheme(%q) = %v, want ErrScheme", s, err)
		}
	}
}

func TestNormalizeHostPort(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"http://sonarr.lan:8989/api/v3", "sonarr.lan:8989"},
		{"http://SONARR.LAN:8989/", "sonarr.lan:8989"},
		{"http://sonarr.lan./", "sonarr.lan:80"},
		{"https://sonarr.lan/", "sonarr.lan:443"},
		{"http://127.0.0.1/", "127.0.0.1:80"},
		{"http://[::ffff:127.0.0.1]:8080/", "127.0.0.1:8080"},
		{"http://[2606:4700::1111]/", "[2606:4700::1111]:80"},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.in)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.in, err)
		}
		got, err := NormalizeHostPort(u)
		if err != nil {
			t.Fatalf("NormalizeHostPort(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("NormalizeHostPort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	u, _ := url.Parse("file:///etc/passwd")
	if _, err := NormalizeHostPort(u); !errors.Is(err, ErrScheme) {
		t.Errorf("want ErrScheme for file:, got %v", err)
	}
}

func TestEffectiveClassForDerived(t *testing.T) {
	t.Parallel()

	// A derived URL that is host-and-port-identical to its originating instance
	// is an integration fetch to that one host.
	if got := effectiveClass(ClassDerived, "komga.lan:8080", "komga.lan:8080"); got != ClassConfigured {
		t.Errorf("derived matching its instance = %v, want configured", got)
	}
	// Anything else gets the strict public-only policy. Nothing in between.
	if got := effectiveClass(ClassDerived, "komga.lan:8080", "komga.lan:9090"); got != ClassDerived {
		t.Errorf("derived on a different port = %v, want derived (strict)", got)
	}
	if got := effectiveClass(ClassDerived, "komga.lan:8080", "evil.example:8080"); got != ClassDerived {
		t.Errorf("derived on a different host = %v, want derived (strict)", got)
	}
	// A provider client never adopts an instance's private reach.
	if got := effectiveClass(ClassProvider, "komga.lan:8080", "komga.lan:8080"); got != ClassProvider {
		t.Errorf("provider = %v, want provider", got)
	}
}

func TestNewClientRejectsBadOptions(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(Options{Class: ClassConfigured}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("configured without AllowedHostPort: want ErrInvalidOptions, got %v", err)
	}
	if _, err := NewClient(Options{}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("zero class: want ErrInvalidOptions, got %v", err)
	}
	if _, err := NewClient(Options{Class: ClassProvider, SPKIPin: []byte("short")}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("short pin: want ErrInvalidOptions, got %v", err)
	}
}
