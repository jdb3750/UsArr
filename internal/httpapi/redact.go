package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// Redaction is middleware, not a convention (ARCHITECTURE.md §14.5,
// reference/security.md §5, CONFIGURATION.md §2.1).
//
// The northbound OpenSubsonic credential rides in the request line by
// specification, so an unredacted access log IS a credential file. The rule this
// file implements: the redacted form of a request exists BEFORE any log line,
// audit row, error string or SSE payload does, at every level including trace.
//
// Mechanically that means one thing — nothing downstream of redactMiddleware may
// read r.URL for the purpose of rendering it. Use RequestLine(ctx) instead. The
// query-parameter deny-list and the URL rewriting itself are ssrf.RedactURL's;
// there is deliberately no second implementation here, because two deny-lists
// drift and the one that drifts is the one that leaks.

// redactedHeaders are stripped from anything rendered for a human. They are not
// removed from the request — X-Api-Key is how a client authenticates — only from
// every representation of it.
var redactedHeaders = []string{"Authorization", "X-Api-Key", "Cookie", "X-CSRF-Token"}

// RedactedPlaceholder is what a redacted header value renders as.
const RedactedPlaceholder = "<redacted>"

type ctxKey int

const (
	ctxKeyRequestLine ctxKey = iota
	ctxKeySession
	ctxKeyRequestID
)

// redactMiddleware computes the redacted request line once, before any other
// middleware or handler runs, and puts it on the context. It is the FIRST thing
// in the chain for that reason: a panic, a log line or an error produced by
// anything after it can only reach the redacted form.
func redactMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ctxKeyRequestLine, redactRequestLine(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// redactRequestLine renders "METHOD /path?query" with every credential-bearing
// query parameter replaced.
func redactRequestLine(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	// Only the path and query are wanted: the authority is UsArr's own and the
	// scheme is not carried on a server-side URL anyway.
	u := &url.URL{Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	return r.Method + " " + ssrf.RedactURL(u)
}

// RequestLine returns the redacted request line for this request. Every log line
// that names a request uses this and never r.URL.
func RequestLine(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyRequestLine).(string)
	return s
}

// RedactedHeaders renders headers safe to log. It is not used on any hot path —
// it exists so that "dump the request" in a support bundle cannot be written the
// wrong way.
func RedactedHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for name, values := range h {
		if isRedactedHeader(name) {
			out[name] = RedactedPlaceholder
			continue
		}
		out[name] = strings.Join(values, ", ")
	}
	return out
}

func isRedactedHeader(name string) bool {
	for _, h := range redactedHeaders {
		if strings.EqualFold(h, name) {
			return true
		}
	}
	return false
}

// redactText strips credentials out of free-form text that may embed a URL.
//
// It is a one-line shim over ssrf.RedactText, which is where the implementation
// lives: internal/kavita needs the same scanner on its response-body path, and a
// second copy is exactly what the one-implementation rule at the top of this file
// forbids. The shim stays so this package's 38 call sites read unchanged.
func redactText(s string) string { return ssrf.RedactText(s) }

// redactURLField renders a URL harvested from upstream (an indexer's infoUrl,
// commentUrl or posterUrl) for a client.
//
// It applies the SAME fixed deny-list as everything else, which is the point:
// one list, one implementation. This is not the place to widen it — the list is
// owned by internal/ssrf and documented in ARCHITECTURE.md §14.5 item 5 and
// reference/security.md §5, and a second deny-list bolted on here is how a
// parameter ends up redacted on one code path and not the other.
//
// The tracker-specific names — passkey, torrent_pass, torrentpass, rsskey,
// authkey, apipasskey, cookie — ARE covered now. They were not, and that was a
// real leak rather than a documented limitation: infoUrl is indexer-supplied and
// is serialised to the browser as info_url on every search result, and private
// trackers carry the user's personal passkey in exactly that URL. See
// TestPrivateTrackerPasskeysAreRedacted in internal/ssrf.
func redactURLField(raw string) string {
	if raw == "" {
		return ""
	}
	return ssrf.RedactRawURL(raw)
}
