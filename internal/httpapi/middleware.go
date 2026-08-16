package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"
)

// clientIPKey carries the resolved peer address. It is resolved once, by the one
// middleware allowed to look at a forwarded header, so nothing downstream is
// tempted to re-derive it from X-Forwarded-For.
type clientIPKeyType struct{}

var clientIPKey clientIPKeyType

// clientIPMiddleware resolves the client address under USARR_TRUSTED_PROXIES.
//
// Trusted headers are a footgun (reference/security.md §6). If UsArr believed
// X-Forwarded-For unconditionally, anyone reaching the app port directly —
// bypassing the proxy — could claim any address, which silently disables per-IP
// rate limiting and makes audit_log.actor_ip a field of attacker-supplied
// strings. GHSA-qcmf-gmhm-rfv9 is exactly that bug in Jellyfin.
//
// So: the header is read ONLY when the immediate peer is inside a configured
// CIDR, the default configuration is empty, and empty means the peer address is
// used as-is. The forwarded headers are also stripped from the request itself,
// so no later handler can read the raw value by accident.
func (s *Server) clientIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := peerAddr(r.RemoteAddr)
		ip := peer
		proto := schemeOf(r)

		if s.trusts(peer) {
			if fwd := firstForwardedFor(r.Header.Get("X-Forwarded-For")); fwd.IsValid() {
				ip = fwd
			}
			if p := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); p != "" {
				proto = strings.ToLower(p)
			}
		}

		// Strip after reading, always — including when nothing is trusted, which
		// is the case that matters. A handler that reads the raw header cannot
		// then be the one place the rule is forgotten.
		r.Header.Del("X-Forwarded-For")
		r.Header.Del("X-Forwarded-Proto")
		r.Header.Del("X-Real-Ip")
		r.Header.Del("Forwarded")

		ctx := context.WithValue(r.Context(), clientIPKey, clientState{ip: ip, scheme: proto})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type clientState struct {
	ip     netip.Addr
	scheme string
}

func (s *Server) trusts(addr netip.Addr) bool {
	if !addr.IsValid() || len(s.cfg.TrustedProxies) == 0 {
		return false
	}
	for _, p := range s.cfg.TrustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func clientOf(ctx context.Context) clientState {
	cs, _ := ctx.Value(clientIPKey).(clientState)
	return cs
}

// ClientIP is the resolved peer address as a string, for the audit log.
func ClientIP(ctx context.Context) string {
	cs := clientOf(ctx)
	if !cs.ip.IsValid() {
		return ""
	}
	return cs.ip.String()
}

func peerAddr(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// firstForwardedFor takes the LEFTMOST entry, which is the original client. The
// rightmost is the nearest proxy and is the one an attacker cannot forge — but
// it is also the proxy's own address, which is not what actor_ip means.
func firstForwardedFor(header string) netip.Addr {
	if header == "" {
		return netip.Addr{}
	}
	first, _, _ := strings.Cut(header, ",")
	addr, err := netip.ParseAddr(strings.TrimSpace(first))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// securityHeadersMiddleware applies the fixed response headers from
// reference/security.md §7. The CSP is strict — no unsafe-inline and no
// unsafe-eval — because SvelteKit with adapter-static needs neither.
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		// There is no style-src-attr here, so inline style ATTRIBUTES fall back to
		// style-src and are refused. One known violation follows from that:
		// SvelteKit's route announcer, which .svelte-kit/generated/root.svelte
		// hides with a style attribute, so the browser reports a CSP violation for
		// it on every page load.
		//
		// THE REFUSAL IS REPORTED BUT NOT ENFORCED, and the difference matters
		// enough to write down, because this comment used to claim the opposite.
		// Svelte 5 builds the fragment by assigning to a <template>'s innerHTML
		// and cloning the content in, and on that path Chromium fires the
		// violation while the already-parsed declaration survives the clone.
		// Measured in Chromium 141.0.7390.37 against this exact header:
		// #svelte-announcer has style.length 11, computed position `absolute`,
		// clip-path `inset(50%)` and a 1x1 box, and removing the attribute flips
		// all of them back to static/none/auto. The control says the directive
		// itself is not toothless — the same declaration reaching the same page
		// through setAttribute IS blocked, style.length 0 and position `static`.
		// It is this one construction path that gets past it.
		//
		// Tolerated anyway, because what neutralises the announcer is a rule and
		// not the refusal: `#svelte-announcer { display: none }` in
		// web/src/app.css hides it and takes it out of the accessibility tree,
		// and the shell's own polite live region in +layout.svelte is the one
		// that announces navigations. So the applied style positions an element
		// that nothing can see and nothing can reach, and the report is the
		// entire remaining symptom.
		//
		// It is deliberately NOT silenced by adding `style-src-attr
		// 'unsafe-inline'`. That directive is not scoped to the announcer: it
		// would re-permit every inline style attribute on every page, which is
		// the exact injection sink this policy exists to close, in exchange for
		// suppressing one report that is already understood and bounded.
		//
		// 'report-sample' rides on style-src and script-src so that a violation
		// record carries a prefix of the offending declaration. Without it the
		// announcer's record is unidentifiable: empty `sample`, and a column
		// offset into a content-hashed minified chunk that changes every build.
		//
		// ⚠️ SEAM. Reports are CONSOLE-ONLY today — the policy below has no
		// report-uri and no report-to, nothing sets Reporting-Endpoints, and
		// there is no reporting handler in this package — so the sample never
		// leaves the user's own browser and the exposure is nil. A reporting
		// endpoint would be configured HERE, and adding one means re-deciding
		// 'report-sample' at the same time: the sample stops being a console
		// string and becomes page content posted off the user's machine, which
		// §14 of docs/ARCHITECTURE.md judges, not this line.
		//
		// And it is a hint, not an identifier. The sample truncates at 40
		// characters, which for the announcer is
		// "position: absolute; left: 0; top: 0; cli" — a visually-hidden clip
		// pattern, naming no element and no file. It narrows the search; the
		// console line's style hash is still what pins the violation to a
		// specific declaration. Do not read a populated sample as an answer.
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'report-sample'; "+
				"script-src 'self' 'report-sample'; "+
				"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// recoverMiddleware turns a panic into a 500 and a log line.
//
// It is the OUTERMOST middleware, so it also covers redaction itself. The stack
// trace goes to the log and never to the client: a Go stack names internal paths
// and, on a bad day, values.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// http.ErrAbortHandler is the documented way to abandon a response;
			// it is not a bug and must not be logged as one.
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			s.log.Error("panic serving request",
				"request", RequestLine(r.Context()),
				"panic", redactText(toString(rec)),
				"stack", string(debug.Stack()))
			writeJSON(w, http.StatusInternalServerError, errorBody{
				Error:   CodeInternal,
				Message: "the request could not be completed",
			})
		}()
		next.ServeHTTP(w, r)
	})
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case error:
		return t.Error()
	default:
		return "non-string panic value"
	}
}

// statusRecorder captures the status for the access log without buffering the
// body. It forwards Flush so SSE still streams through it.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *statusRecorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// accessLogMiddleware logs one line per request.
//
// It names the request only through RequestLine, which is the redacted form
// produced upstream of it. There is no code path here that can reach r.URL.
func (s *Server) accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		s.log.Debug("request",
			"request", RequestLine(r.Context()),
			"status", rec.status,
			"bytes", rec.bytes,
			"ip", ClientIP(r.Context()),
			"duration", s.now().Sub(start).Round(time.Millisecond).String())
	})
}
