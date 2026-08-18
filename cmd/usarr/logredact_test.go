package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

const (
	leakURL = "http://prowlarr:9696/api/v1/indexer?apikey=abc123def456"
	leakKey = "abc123def456"
)

func redactingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(newRedactHandler(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

// The shape every failure in this package actually takes: `"err", err`.
func TestRedactHandlerRedactsErrorAttrs(t *testing.T) {
	var buf bytes.Buffer
	err := errors.New(`Get "` + leakURL + `": connection refused`)
	redactingLogger(&buf).Error("upstream unreachable", "instance_id", 3, "err", err)

	out := buf.String()
	if strings.Contains(out, leakKey) {
		t.Fatalf("the key survived the handler: %s", out)
	}
	for _, want := range []string{"upstream unreachable", "connection refused", "prowlarr:9696"} {
		if !strings.Contains(out, want) {
			t.Errorf("redaction destroyed the diagnostic: %q missing from %s", want, out)
		}
	}

	// The non-string attribute must not be mangled into one.
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("handler produced invalid JSON: %v", err)
	}
	if line["instance_id"] != float64(3) {
		t.Errorf("instance_id was mangled: %#v", line["instance_id"])
	}
}

func TestRedactHandlerRedactsStringAttrsAndTheMessage(t *testing.T) {
	var buf bytes.Buffer
	redactingLogger(&buf).Info("probing "+leakURL, "url", leakURL)

	if out := buf.String(); strings.Contains(out, leakKey) {
		t.Fatalf("the key survived the handler: %s", out)
	}
}

// Attributes bound with With() render on EVERY subsequent line, so an unredacted
// one leaks once per line rather than once.
func TestRedactHandlerRedactsPreBoundAttrs(t *testing.T) {
	var buf bytes.Buffer
	redactingLogger(&buf).With("url", leakURL).Info("hello")

	if out := buf.String(); strings.Contains(out, leakKey) {
		t.Fatalf("the key survived WithAttrs: %s", out)
	}
}

func TestRedactHandlerRecursesIntoGroups(t *testing.T) {
	var buf bytes.Buffer
	redactingLogger(&buf).Info("grouped", slog.Group("upstream", "url", leakURL))

	if out := buf.String(); strings.Contains(out, leakKey) {
		t.Fatalf("the key survived a group: %s", out)
	}
}

// A LogValuer hides its text behind Resolve(); redacting before resolving would
// inspect the wrapper and pass the payload straight through.
type leakyValuer struct{}

func (leakyValuer) LogValue() slog.Value { return slog.StringValue("failed on " + leakURL) }

func TestRedactHandlerResolvesLogValuers(t *testing.T) {
	var buf bytes.Buffer
	redactingLogger(&buf).Info("valuer", "detail", leakyValuer{})

	if out := buf.String(); strings.Contains(out, leakKey) {
		t.Fatalf("the key survived a LogValuer: %s", out)
	}
}

// Redaction must not swallow the line. A handler that drops records is
// indistinguishable from one that redacts everything.
func TestRedactHandlerPassesEnabledThrough(t *testing.T) {
	var buf bytes.Buffer
	h := newRedactHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled ignored the inner handler's level")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled refused a level the inner handler allows")
	}
}
