package ssrf

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// Resolver is the DNS surface this package needs. *net.Resolver satisfies it, so
// callers pass net.DefaultResolver or nothing at all; tests pass a fake.
type Resolver interface {
	// LookupNetIP must return both A and AAAA records for network "ip".
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// pinnedDialer implements resolve-then-pin. security.md §2.2 item 2.
//
// The critical property is that validation re-runs inside DialContext, so it
// happens on every new connection rather than once when the URL was saved.
// Reusing an already-validated keep-alive connection is fine; a fresh dial is
// not, and a fresh dial is exactly what a DNS-rebinding attacker needs.
type pinnedDialer struct {
	class           Class
	allowedHostPort string
	resolver        Resolver

	// netDial is the actual TCP dial, injected in tests so a fake "public"
	// address can be routed at an httptest server. Nil means net.Dialer.
	netDial func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (d *pinnedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, hostPort, err := hostPortFromDialAddr(addr)
	if err != nil {
		return nil, err
	}

	eff, err := authorizeHostPort(d.class, d.allowedHostPort, hostPort)
	if err != nil {
		return nil, err
	}

	addrs, err := d.resolve(ctx, network, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, policyErr("dial", eff, host, netip.Addr{}, ErrNoAddress)
	}

	// Every resolved address must pass, not merely one of them. A rebinding
	// resolver that answers with a public address alongside 127.0.0.1 is already
	// hostile; dialling the "good" half of that answer would just be a slower way
	// of losing.
	for _, a := range addrs {
		if err := CheckAddr(a, eff); err != nil {
			return nil, policyErr("dial", eff, host, a, ErrBlockedAddress)
		}
	}

	_, port, _ := net.SplitHostPort(hostPort)
	var lastErr error
	for _, a := range addrs {
		// Dial the address, never the name. The http.Request keeps the original
		// host, so Host, SNI and certificate verification are unaffected — which
		// is what stops the "certificate is for a name, not an IP" failure whose
		// usual fix is InsecureSkipVerify.
		target := net.JoinHostPort(a.String(), port)
		conn, err := d.dial(ctx, network, target)
		if err != nil {
			lastErr = err
			continue
		}
		return conn, nil
	}
	return nil, fmt.Errorf("ssrf: dial %s: %w", hostPort, lastErr)
}

func (d *pinnedDialer) resolve(ctx context.Context, network, host string) ([]netip.Addr, error) {
	// An IP literal is already an address; sending it to the resolver would only
	// give a permissive system resolver a chance to reinterpret it.
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return filterFamily([]netip.Addr{addr}, network), nil
	}

	// "ip" asks for A *and* AAAA. Validating only one family leaves the other as
	// an unguarded path to the same host.
	lookup := "ip"
	switch network {
	case "tcp4":
		lookup = "ip4"
	case "tcp6":
		lookup = "ip6"
	}
	addrs, err := d.resolver.LookupNetIP(ctx, lookup, host)
	if err != nil {
		return nil, fmt.Errorf("ssrf: resolve %s: %w", host, err)
	}
	return filterFamily(addrs, network), nil
}

func filterFamily(addrs []netip.Addr, network string) []netip.Addr {
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		a = a.Unmap()
		switch network {
		case "tcp4":
			if !a.Is4() {
				continue
			}
		case "tcp6":
			if a.Is4() {
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

func (d *pinnedDialer) dial(ctx context.Context, network, target string) (net.Conn, error) {
	if d.netDial != nil {
		return d.netDial(ctx, network, target)
	}
	dialer := &net.Dialer{Timeout: ConnectTimeout}
	return dialer.DialContext(ctx, network, target)
}
