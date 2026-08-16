package ssrf

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

// MaxRedirects is the hop budget. security.md §2.2 item 3.
//
// Following redirects at all is deliberate: TMDB, Cover Art Archive (Internet
// Archive backed) and Fanart.tv all redirect, so "just disable redirects" ships a
// broken image pipeline that someone later "fixes" by turning redirects back on
// globally — losing the control entirely. Follow them, but revalidate every hop.
const MaxRedirects = 3

// credentialHeaders never survive a redirect. Go's own client only strips a
// subset, and only when the hop crosses domains; that is not enough here, where
// X-Api-Key is a full-admin *Arr credential and the next host may be attacker-
// nominated even within the same domain.
var credentialHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Cookie2",
	"X-Api-Key",
	"X-Emby-Token",
	"X-MediaBrowser-Token",
}

// checkRedirect is the http.Client's CheckRedirect. The address policy is not
// re-run here — it re-runs in the dialer, which is the only place that sees the
// connection actually being made. This function owns the things the dialer
// cannot see: the hop count, the scheme change, and the credentials riding along.
func (p *policyClient) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > MaxRedirects {
		return policyErr("redirect", p.class, req.URL.Host, netip.Addr{},
			fmt.Errorf("%w: %d hops", ErrTooManyRedirects, len(via)))
	}

	if err := CheckScheme(req.URL.Scheme); err != nil {
		return policyErr("redirect", p.class, req.URL.Host, netip.Addr{}, err)
	}

	// Any scheme change is refused, not just the https -> http downgrade. An
	// upgrade is harmless in itself, but "the scheme the caller chose is the
	// scheme used" is a rule with no exceptions to remember.
	prev := via[len(via)-1].URL.Scheme
	if !strings.EqualFold(prev, req.URL.Scheme) {
		return policyErr("redirect", p.class, req.URL.Host, netip.Addr{},
			fmt.Errorf("%w: %s -> %s", ErrRedirectScheme, prev, req.URL.Scheme))
	}

	hostPort, err := NormalizeHostPort(req.URL)
	if err != nil {
		return policyErr("redirect", p.class, req.URL.Host, netip.Addr{}, err)
	}
	if _, err := authorizeHostPort(p.class, p.allowedHostPort, hostPort); err != nil {
		return err
	}

	for _, h := range credentialHeaders {
		req.Header.Del(h)
	}
	stripCredentials(req.URL)
	return nil
}
