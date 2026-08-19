package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// parseVersionOutput lifts the `key: value` lines back out of --version's output
// the same way deploy/update.sh and deploy/status.sh do. If this stops working,
// those scripts stop being able to tell a stale binary from a current one.
func parseVersionOutput(t *testing.T, out string) map[string]string {
	t.Helper()
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		got[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return got
}

func TestPrintVersionShape(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("--version printed %d lines, want 4:\n%s", len(lines), buf.String())
	}
	if !strings.HasPrefix(lines[0], "usarr ") {
		t.Errorf("first line = %q, want it to start with %q", lines[0], "usarr ")
	}

	fields := parseVersionOutput(t, buf.String())
	for key, want := range map[string]string{
		"commit": commit,
		"built":  buildDate,
		"go":     goVersion(),
	} {
		if got := fields[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
		if fields[key] == "" {
			t.Errorf("%s is empty; the deploy scripts would read it as unknown", key)
		}
	}
}

// TestRunHandlesVersionFlag exercises the real entry point rather than
// printVersion alone: --version has to answer before run() opens a database or
// reaches for the master key, so an operator can ask an installed binary what it
// is without any of the privileges the service itself needs.
func TestRunHandlesVersionFlag(t *testing.T) {
	// A config dir that does not exist and could not be created. If run() gets
	// as far as resolving the layout, it fails here instead of printing.
	t.Setenv("USARR_CONFIG_DIR", "/proc/self/usarr-must-not-be-created")

	origArgs := os.Args
	origStdout := os.Stdout
	t.Cleanup(func() { os.Args, os.Stdout = origArgs, origStdout })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Args = []string{"usarr", "--version"}
	os.Stdout = w

	runErr := run()
	_ = w.Close()
	os.Stdout = origStdout

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if runErr != nil {
		t.Fatalf("run() with --version = %v, want nil (exit 0)", runErr)
	}
	if got := parseVersionOutput(t, string(out))["commit"]; got != commit {
		t.Errorf("commit on stdout = %q, want %q; output was:\n%s", got, commit, out)
	}
}

// TestRunHandlesHelpFlag pins the whole of what -h owes: exit 0, on stdout, with
// the usage block intact. It goes through run() rather than through
// config.WriteUsage because the three are only true together — WriteUsage alone
// would still pass with -h wired back to the parse-error path, printing the same
// text to stderr and exiting 1, which is exactly what this replaced.
//
// Asserted with a config dir that cannot be created, like --version above: an
// operator must be able to ask an installed binary how to run it without holding
// the privileges running it needs.
func TestRunHandlesHelpFlag(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Setenv("USARR_CONFIG_DIR", "/proc/self/usarr-must-not-be-created")

		origArgs := os.Args
		origStdout := os.Stdout
		t.Cleanup(func() { os.Args, os.Stdout = origArgs, origStdout })

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		os.Args = []string{"usarr", arg}
		os.Stdout = w

		runErr := run()
		_ = w.Close()
		os.Stdout = origStdout

		out, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read captured stdout: %v", err)
		}
		if runErr != nil {
			t.Errorf("run() with %s = %v, want nil (exit 0)", arg, runErr)
		}
		for _, want := range []string{"Usage:", "key rotate", "Flags:", "-secret-key-file"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("`usarr %s` stdout does not contain %q:\n%s", arg, want, out)
			}
		}
		if strings.Contains(string(out), "parse flags") {
			t.Errorf("`usarr %s` stdout carries a parse-error prefix:\n%s", arg, out)
		}
	}
}
