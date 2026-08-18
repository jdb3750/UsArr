package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/config"
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

// TestNewLoggerWrapsTheHandler is the guard for LS-170 step 3's WIRING.
//
// The tests above prove redactHandler redacts; none of them proves the process
// logger is built with one. Unwrapping either branch of newLogger back to a bare
// slog.NewTextHandler / NewJSONHandler left this package green, which is the
// "indistinguishable from no guard" case CLAUDE.md names. So this drives
// newLogger itself, on a real config, over BOTH format branches — the two are
// separate `return` statements and an unwrapped one is exactly the mistake that
// hides behind the other's coverage.
//
// os.Stdout is swapped for a pipe because newLogger writes there by
// construction: it is the process logger, and taking a writer argument purely to
// make it testable would move the wiring under test out of production code.
func TestNewLoggerWrapsTheHandler(t *testing.T) {
	for _, tc := range []struct {
		format string
		// wantJSON is what the branch is supposed to select, so a test that
		// passes because both branches collapsed onto one handler still fails.
		wantJSON bool
	}{
		{format: "text", wantJSON: false},
		{format: "json", wantJSON: true},
		// auto with a pipe on stdout is not a terminal, so it must resolve to JSON.
		{format: "auto", wantJSON: true},
	} {
		t.Run(tc.format, func(t *testing.T) {
			cfg, err := config.Load(config.Options{
				Args:    []string{"--config-dir", t.TempDir()},
				Env:     map[string]string{"USARR_LOG_FORMAT": tc.format},
				Version: "0.0.0-test",
			})
			if err != nil {
				t.Fatalf("config: %v", err)
			}
			if cfg.LogFormat != tc.format {
				t.Fatalf("config did not take the format: %q", cfg.LogFormat)
			}

			out := captureStdout(t, func() {
				// The shape every upstream failure in this package takes.
				newLogger(cfg).Error("upstream unreachable",
					"instance_id", 3,
					"err", errors.New(`Get "`+leakURL+`": connection refused`))
			})

			if strings.Contains(out, leakKey) {
				t.Fatalf("newLogger built an UNWRAPPED handler: the key reached stdout: %s", out)
			}
			// Not vacuous: the line was written, and it is still diagnostic.
			for _, want := range []string{"upstream unreachable", "connection refused", "prowlarr:9696", "REDACTED"} {
				if !strings.Contains(out, want) {
					t.Errorf("%q missing from the log line: %s", want, out)
				}
			}
			gotJSON := json.Unmarshal([]byte(out), &map[string]any{}) == nil
			if gotJSON != tc.wantJSON {
				t.Errorf("format %q produced json=%v, want json=%v: %s", tc.format, gotJSON, tc.wantJSON, out)
			}
			t.Logf("%s: %s", tc.format, strings.TrimSpace(out))
		})
	}
}

// captureStdout runs fn with os.Stdout replaced by a pipe and returns what was
// written. newLogger captures os.Stdout at call time, so fn must build the
// logger, not just use one.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = saved }()

	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("closing the pipe: %v", err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatalf("closing the pipe: %v", err)
	}
	return out
}
