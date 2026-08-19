// Command usarr is UsArr's single static binary: the API, the SSE stream and
// the embedded SPA, over one SQLite database.
//
// Configuration is level 1 only — flags and the environment, never a file and
// never the database (docs/CONFIGURATION.md §1). Service instances and their
// credentials are level 2 and are written exclusively through the UI, because
// the master key decrypts the database and the trusted-proxy allowlist decides
// whether a forwarded header is believed: if either were editable through the
// surface it protects, a UI authorization bug would be a full compromise.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jdb3750/UsArr/internal/config"
	"github.com/jdb3750/UsArr/internal/httpapi"
)

// Stamped by -ldflags at build time; the Makefile's LDFLAGS names exactly these
// three symbols.
var (
	version   = config.DefaultVersion
	commit    = "unknown"
	buildDate = "unknown"
)

// printVersion writes the build identity that --version reports.
//
// The shape is a contract, not a cosmetic choice: deploy/update.sh compares the
// commit it prints against the checkout's short HEAD to catch a binary that was
// never rebuilt, and deploy/status.sh greps the same line. Each field is on its
// own line with a fixed `key: value` prefix so a one-line `sed`/`awk` can lift
// the raw SHA out without parsing prose. Changing a key here breaks those
// scripts silently — they would read an empty commit and could no longer tell
// stale from current.
func printVersion(w io.Writer) {
	// One call, and the write error is discarded on purpose: the only caller
	// writes to stdout and is about to exit 0. A closed stdout is the caller's
	// problem to notice, and there is nothing useful this could do about it.
	_, _ = fmt.Fprintf(w, "usarr %s\ncommit: %s\nbuilt:  %s\ngo:     %s\n",
		version, commit, buildDate, goVersion())
}

// buildInfo is the stamped build identity, in the shape internal/httpapi wants.
// It is a function so that every entry point — the server and the maintenance
// subcommands — reports the identity the same way.
func buildInfo() httpapi.BuildInfo {
	return httpapi.BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
		GoVersion: goVersion(),
	}
}

// drainDeadline bounds graceful shutdown.
//
// The SSE stream is why this is not longer: an event stream never ends on its
// own, so shutdown has to close the hub and then wait only for ordinary
// requests. Waiting for the stream would mean waiting forever.
const drainDeadline = 15 * time.Second

// readHeaderTimeout is the slow-loris bound. Every other timeout on the server
// is deliberately absent: WriteTimeout would cut the SSE stream off at a fixed
// interval, which is exactly what it must not do.
const readHeaderTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		// Fatal errors name the variable or file the bad value came from; that
		// is a property of the errors themselves, and this only prints them.
		fmt.Fprintln(os.Stderr, "usarr: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadOS(version)
	switch {
	case errors.Is(err, config.ErrHelpRequested):
		// stdout and exit 0, for the same reason --version does it below: -h is
		// a question, and answering it is not a failure. It used to arrive here
		// as an ordinary parse error, so `usarr --help` printed
		// `usarr: parse flags: flag: help requested` to stderr and exited 1 —
		// the first command a new operator runs, reporting that it broke.
		config.WriteUsage(os.Stdout)
		return nil
	case errors.Is(err, config.ErrVersionRequested):
		// stdout, not stderr, and exit 0: this is the answer to a question, so
		// it has to be pipeable into grep by deploy/status.sh.
		printVersion(os.Stdout)
		return nil
	case errors.Is(err, config.ErrKeyRotateRequested):
		// A different program, selected by the same parser that resolved the
		// configuration it runs against. It returns instead of falling through:
		// nothing below this line — the listener, the prober, the sweeper —
		// belongs to a maintenance command.
		//
		// Signals are trapped so a Ctrl-C during a long re-wrap ends between
		// batches rather than mid-transaction. Interrupting is safe at any
		// point: both keys are on disk until the promote step, so an aborted
		// rotation leaves every row readable and resumes on the next run.
		rotateCtx, stopRotate := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stopRotate()
		return runKeyRotate(rotateCtx, cfg, newLogger(cfg), buildInfo(), os.Stdout)
	case err != nil:
		return err
	}
	log := newLogger(cfg)

	if cfg.EnvFile != "" && cfg.EnvFileMissing {
		// Not fatal on purpose: `make dev` passes --env-file .env
		// unconditionally and a fresh checkout has no .env.
		log.Info("--env-file was not found; continuing with the process environment", "path", cfg.EnvFile)
	}
	if err := applyTimezone(cfg, log); err != nil {
		return err
	}

	log.Info("starting UsArr",
		"version", version, "commit", commit, "build_date", buildDate,
		"config_dir", cfg.ConfigDir, "data_dir", cfg.DataDir, "address", cfg.Address(),
		"url_base", orNone(cfg.URLBase))

	// Signals are trapped BEFORE the database is opened, so a Ctrl-C during a
	// long migration is a clean exit rather than a SIGKILL mid-transaction.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := buildApp(ctx, cfg, log, buildInfo())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := a.Close(); cerr != nil {
			log.Error("shutdown: database did not close cleanly", "err", cerr)
		}
	}()

	// The prober is the only thing in the process that talks to an *Arr for
	// health, and it runs on its own goroutine with its own context so no
	// render path can ever wait on it.
	proberCtx, stopProber := context.WithCancel(context.Background())
	defer stopProber()
	// The on-connect full import runs under the PROBER's context, not a
	// request's: it is minutes of work and a request deadline would kill it
	// halfway. Arming it here rather than in buildApp keeps buildApp from
	// returning with a goroutine already writing to a database it might still
	// fail to finish setting up — the same rule the candidate sweeper follows.
	a.registry.enableBootstrapImport(proberCtx)
	go a.registry.RunProber(proberCtx)

	// The release_candidate sweep. It starts here and not inside buildApp
	// because buildApp must not return with a goroutine already writing to the
	// database it might still fail to finish setting up; by this line the
	// migrations have been applied and the app is whole.
	//
	// Unlike the prober, this one is WAITED FOR: it writes, so it has to be
	// finished before a.Close() closes the database under it. This defer is
	// registered after the Close defer above and therefore runs before it.
	sweeperCtx, stopSweeper := context.WithCancel(context.Background())
	sweeperDone := a.startCandidateSweeper(sweeperCtx)
	defer func() {
		stopSweeper()
		<-sweeperDone
	}()

	srv := &http.Server{
		Handler:           a.server.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		BaseContext:       func(net.Listener) context.Context { return context.Background() },
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	// Listen before Serve so a bound-port failure is a startup error naming the
	// variable, not a goroutine that dies silently a moment later.
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", cfg.Address())
	if err != nil {
		return fmt.Errorf("USARR_BIND_ADDRESS/USARR_PORT: cannot listen on %s: %w", cfg.Address(), err)
	}

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	// Readiness is exactly "migrations applied and the listener accepting", and
	// this is the second half of it.
	a.server.MarkListening()
	log.Info("listening", "address", listener.Addr().String(), "url_base", orNone(cfg.URLBase))

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	log.Info("shutting down", "drain_deadline", drainDeadline.String())
	// Stop advertising readiness first, so anything in front stops sending work
	// while in-flight requests still complete.
	a.server.Draining()
	stopProber()
	// Cancel the sweep alongside the prober so it is not still running through
	// the drain; the deferred wait above is what guarantees it has actually
	// stopped before the database closes.
	stopSweeper()
	// An SSE connection never ends by itself; closing the hub ends the streams
	// so Shutdown has only ordinary requests left to wait for.
	a.server.Close()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), drainDeadline)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("drain deadline reached with requests still in flight", "err", err)
		_ = srv.Close()
	}
	<-serveErr
	log.Info("stopped")
	return nil
}

// newLogger builds the process logger.
//
// `auto` is text on a TTY and JSON otherwise, which is what a container wants
// without asking. Note what is NOT here: there is no level at which redaction is
// relaxed. trace is not an exemption (CONFIGURATION.md §2.1), and the redaction
// that makes that true is middleware in internal/httpapi, not a rule this
// function could enforce.
func newLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.LogLevel {
	case "trace", "debug":
		// slog has no trace level; the lowest it offers is Debug, and mapping
		// trace onto it is honest about what the binary can actually do.
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}

	format := cfg.LogFormat
	if format == "auto" {
		format = "json"
		if isTerminal(os.Stdout) {
			format = "text"
		}
	}
	// Both handlers are wrapped: redaction in this binary is middleware, not a
	// convention. See logredact.go.
	if format == "text" {
		return slog.New(newRedactHandler(slog.NewTextHandler(os.Stdout, opts)))
	}
	return slog.New(newRedactHandler(slog.NewJSONHandler(os.Stdout, opts)))
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// applyTimezone resolves TZ once, at startup, so logs and schedules agree.
func applyTimezone(cfg *config.Config, log *slog.Logger) error {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		// Not fatal: a distroless image without tzdata would otherwise refuse
		// to start over a cosmetic setting.
		log.Warn("TZ could not be resolved; using UTC", "tz", cfg.Timezone, "err", err)
		time.Local = time.UTC
		return nil
	}
	time.Local = loc
	return nil
}

func orNone(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(none)"
	}
	return v
}
