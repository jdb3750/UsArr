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
//
// Referer is in this list and it is not like the others: nothing in UsArr sets
// it. Go's http.Client synthesises it from the PREVIOUS request's full URL,
// query string and all, and it does so BEFORE calling CheckRedirect
// (net/http/client.go, refererForURL — which suppresses it only on an
// https->http downgrade, a hop this client already refuses for other reasons).
// So stripping the query off the redirect target is not sufficient on its own:
// without this entry the credential simply rides to the next host in a header
// instead of in the URL. It matters most for ClassProvider, since TMDB v3,
// Fanart.tv and Comic Vine all authenticate by query parameter and all of them
// redirect to CDNs. security.md §5 names Referer explicitly as a leak surface.
//
// Deleting it outright rather than rewriting it from the stripped previous URL:
// no upstream UsArr talks to requires a Referer, and "there is no Referer" is a
// far easier invariant to keep true than "the Referer is the redacted one".
var credentialHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Cookie2",
	"Referer",
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
