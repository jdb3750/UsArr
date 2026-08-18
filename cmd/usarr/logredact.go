package main

import (
	"context"
	"log/slog"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// A redacting slog.Handler, so redaction in cmd/usarr is middleware rather than a
// convention (reference/security.md §5, ARCHITECTURE.md §14.5).
//
// internal/httpapi meets that requirement by hand, at 38 call sites of its
// redactText shim. cmd/usarr met it at none: every `"err", err` in this package
// put an upstream error string — including a Kavita or Prowlarr error that quotes
// the URL it failed on, ?apikey= and all — straight into a log line. Hand-fixing
// the known sites would have left the next one to be written unguarded, which is
// the failure mode "middleware, not a convention" names. So this wraps the
// handler instead, and no site in this package hand-redacts.
//
// SCOPE, stated so it is not mistaken for a general scrubber. It runs
// ssrf.RedactText over the message and over string- and error-valued attributes,
// recursing into groups and resolving LogValuers first. ssrf.RedactText's own
// limit carries over: a bare secret that is not inside a URL is not caught by
// shape. Numbers, times and structs that are not LogValuers are left alone — they
// do not carry upstream text — and API keys must still never be passed to a
// logger in the first place.
type redactHandler struct{ inner slog.Handler }

// newRedactHandler wraps h. Every handler this binary builds goes through here.
func newRedactHandler(h slog.Handler) slog.Handler { return &redactHandler{inner: h} }

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, ssrf.RedactText(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

// WithAttrs and WithGroup redact the pre-bound attributes too: an attribute
// attached with logger.With() is rendered on every subsequent line, so leaving it
// unredacted would leak once per log line rather than once.
func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return &redactHandler{inner: h.inner.WithAttrs(redacted)}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: h.inner.WithGroup(name)}
}

// redactAttr resolves and rewrites one attribute.
//
// Resolve() first, because a LogValuer's whole point is that the interesting text
// is not in the Value as stored; redacting before resolving would inspect the
// wrapper and pass the payload through.
func redactAttr(a slog.Attr) slog.Attr {
	a.Value = redactValue(a.Value.Resolve())
	return a
}

func redactValue(v slog.Value) slog.Value {
	switch v.Kind() {
	case slog.KindString:
		return slog.StringValue(ssrf.RedactText(v.String()))
	case slog.KindGroup:
		attrs := v.Group()
		redacted := make([]slog.Attr, len(attrs))
		for i, a := range attrs {
			redacted[i] = redactAttr(a)
		}
		return slog.GroupValue(redacted...)
	case slog.KindAny:
		// An error is the case this exists for: `"err", err` is how every upstream
		// failure reaches a log line, and err.Error() is where the URL is. It is
		// flattened to its redacted string, which is what a handler would have
		// rendered anyway.
		if err, ok := v.Any().(error); ok && err != nil {
			return slog.StringValue(ssrf.RedactText(err.Error()))
		}
		if s, ok := v.Any().(string); ok {
			return slog.StringValue(ssrf.RedactText(s))
		}
		return v
	default:
		return v
	}
}
