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
var credentialParams = map[string]struct{}{
	"apikey":       {},
	"api_key":      {},
	"token":        {},
	"access_token": {},
	"sig":          {},
	"signature":    {},
	"p":            {},
	"t":            {},
	"s":            {},
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
