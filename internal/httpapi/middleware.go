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
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; "+
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
				Error:   "internal",
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
