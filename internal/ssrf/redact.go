package ssrf

import (
	"net/url"
	"strings"
)

// credentialParams are query parameters that carry a secret. TMDB v3, Fanart.tv
// and Comic Vine all authenticate by query parameter, and OpenSubsonic puts the
// salt/token/password triple in "s"/"t"/"p" — so the short, generic-looking names
// are the dangerous ones, not an over-reach.
//
// These are stripped in two places, and both matter: from any URL that is logged
// or stored (image_asset.source_url, the HTTP cache key), and from any redirect
// hop, because a redirect hands the whole query string to whatever the upstream
// nominated. Deriving cache keys from the stripped URL also means rotating a
// provider key does not invalidate the entire image cache.
//
// This is the ONE deny-list. There is deliberately no second copy anywhere: a
// drifting duplicate is how a parameter ends up redacted on one code path and
// not the other. ARCHITECTURE.md §14.5 item 5 and security.md §5 document it and
// must be updated together with it.
//
// PRIVATE-TRACKER PASSKEYS are in this list, and they are not hypothetical.
// Prowlarr's ReleaseResource.infoUrl and .commentUrl are indexer-supplied and
// are surfaced to the browser as info_url; a private tracker's announce and
// details URLs routinely carry the user's personal passkey as a query parameter
// under one of these names. Without them, UsArr would ship a tracker credential
// straight to the client — and a leaked passkey on a private tracker means
// account termination, because it is what the tracker uses to attribute traffic.
// Prowlarr's own indexer definitions name these fields, which is where the list
// comes from.
//
// Adding a name here also removes it from redirect targets (see
// stripCredentials). That is the reason to prefer long, specific names: a short
// generic one like "t" or "s" is a legitimate cache-buster or size parameter on
// plenty of CDNs, so widening this list with short names has a functional cost
// that the tracker-specific names do not.
var credentialParams = map[string]struct{}{
	// Provider / generic.
	"apikey":       {},
	"api_key":      {},
	"token":        {},
	"access_token": {},
	"auth_token":   {},
	"sig":          {},
	"signature":    {},
	"secret":       {},
	"secret_key":   {},

	// OpenSubsonic salt/token/password.
	"p": {},
	"t": {},
	"s": {},

	// Private-tracker passkeys, as named by Prowlarr's indexer definitions.
	"passkey":      {},
	"torrent_pass": {},
	"torrentpass":  {},
	"rsskey":       {},
	"authkey":      {},
	"apipasskey":   {},
	"cookie":       {},
}

// redactedValue is what replaces a credential in a logged URL. It is not empty,
// so a reader can tell "the key was present" from "there was no key".
const redactedValue = "REDACTED"

func isCredentialParam(name string) bool {
	// Case-insensitive: apiKey, ApiKey and APIKEY are all the same parameter to
	// the services that accept them.
	_, ok := credentialParams[strings.ToLower(name)]
	return ok
}

// RedactURL renders a URL safe to log: userinfo removed, credential query
// parameter values replaced. Every log line and every error message that names a
// URL goes through this — the logging middleware included. An *Arr API key is a
// full-admin credential and it is routinely in the URL you are tempted to print.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	clone.User = nil

	if clone.RawQuery != "" {
		q := clone.Query()
		for name, values := range q {
			if !isCredentialParam(name) {
				continue
			}
			for i := range values {
				values[i] = redactedValue
			}
		}
		clone.RawQuery = q.Encode()
	}
	return clone.String()
}

// RedactRawURL is RedactURL for a string that may not parse. An unparseable URL
// is returned as a fixed placeholder rather than verbatim, because "it did not
// parse" is not evidence that it holds no secret.
func RedactRawURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}
	return RedactURL(u)
}

// stripCredentials removes credential query parameters outright (not redacted —
// removed) and drops userinfo. Used on redirect hops, where the goal is that the
// next host never sees the secret at all.
//
// KNOWN OVER-BREADTH, deliberately not fixed yet. This shares credentialParams
// with the redactor, so the OpenSubsonic short names `p`, `t` and `s` are
// stripped from redirect targets too. Those are correct for redaction and
// over-broad here: an upstream that redirects to `…?t=1699999999` (cache-buster),
// `?s=small` (size) or `?p=2` (page) has that parameter silently removed and
// serves a different resource, or a 400. Internet Archive and CDN redirects
// commonly use short names.
//
// It is left as-is for now because over-stripping fails closed and nothing in
// v0.1 follows redirects on a path where this bites — the image/artwork pipeline
// that fetches CDN URLs is a later milestone. Whoever lands that pipeline should
// split the lists: keep the full list for redaction, and use a narrower one here
// (apikey, api_key, access_token, auth_token, token, sig, signature, secret,
// secret_key, and the tracker passkey names — all long enough to be unambiguous),
// with a comment saying why the two differ. Splitting them now would create a
// second deny-list with no consumer to validate it against.
func stripCredentials(u *url.URL) {
	u.User = nil
	if u.RawQuery == "" {
		return
	}
	q := u.Query()
	var dropped bool
	for name := range q {
		if isCredentialParam(name) {
			q.Del(name)
			dropped = true
		}
	}
	if dropped {
		u.RawQuery = q.Encode()
	}
}
