package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// flags mirrors the level-1 settings that have a command-line twin. Every
// field starts at its zero value: whether a flag was actually supplied is
// tracked separately via FlagSet.Visit, because that is what makes
// "flag > environment > default" work without the flag's default value
// masquerading as a user choice.
type flags struct {
	envFile           string
	configDir         string
	dataDir           string
	bindAddress       string
	port              int
	urlBase           string
	logLevel          string
	logFormat         string
	secretKeyFile     string
	trustedProxies    string
	metadataUserAgent string

	// showVersion is not a configuration value at all: it is a request to print
	// the build identity and exit. It lives here anyway so that ONE parser owns
	// the command line — a second, hand-rolled scan of os.Args would drift from
	// this FlagSet's idea of what a valid argument is.
	showVersion bool

	// keyRotate is the `usarr key rotate` subcommand, and it is here for the
	// same reason showVersion is: the alternative is a second scan of os.Args
	// with its own opinion of what an argument means. It is not a configuration
	// value either — it selects which program runs.
	keyRotate bool

	// backup is the `usarr backup` subcommand: one consistent VACUUM INTO
	// snapshot, taken on demand. Same reasoning as keyRotate — it selects a
	// program, and it goes through this parser so there is still exactly one.
	backup bool
}

// keyRotateArgs is two words rather than one because `key` is a noun with more
// verbs possible under it, and a flat `rotate` would have to be renamed the
// moment a second one landed.
var keyRotateArgs = []string{"key", "rotate"}

// backupArgs is one word because `backup` is the whole verb: there is no second
// thing to back up, so `usarr backup something` would be a distinction without a
// difference. It stays a leaf until there genuinely is one.
var backupArgs = []string{"backup"}

// subcommands is the ONE table of accepted positional forms. Parsing, the
// mutual-exclusion check and the usage block all read it, so adding a verb is
// one entry rather than four edits that can disagree — which is the same reason
// there is one FlagSet rather than a second scan of os.Args.
//
// No entry may be a prefix of another. Nothing enforces that mechanically, and
// the parser below relies on it: it lifts the FIRST matching form and never
// reconsiders, so `foo` and `foo bar` as siblings would make `usarr foo bar`
// mean whichever happens to be listed first.
var subcommands = []struct {
	words []string
	// selected returns the field this form sets, so the table can both set it
	// during parsing and read it back when deciding whether two programs were
	// asked for at once.
	selected func(*flags) *bool
	summary  string
}{
	{
		words:    keyRotateArgs,
		selected: func(f *flags) *bool { return &f.keyRotate },
		summary:  "rotate the master key in place (docs/CONFIGURATION.md §3.4)",
	},
	{
		words:    backupArgs,
		selected: func(f *flags) *bool { return &f.backup },
		summary:  "write a consistent database snapshot, and say what it does not cover (docs/CONFIGURATION.md §6)",
	},
}

// selectedSubcommand reports which form, if any, f names. It returns the index
// so callers can name the form in an error without rebuilding the string.
func selectedSubcommand(f *flags) int {
	for i := range subcommands {
		if *subcommands[i].selected(f) {
			return i
		}
	}
	return -1
}

// newFlagSet registers the command line into f. It is separate from parseFlags
// for one reason: -h now answers on stdout and exits 0 (ErrHelpRequested), so
// the caller prints the usage block AFTER Load has returned and the FlagSet
// parseFlags built is long gone. Both paths build the FlagSet here, so there is
// still exactly ONE definition of what a flag is and what it says — a second,
// hand-written usage string would drift from the flags it claims to describe on
// the first flag anyone adds.
func newFlagSet(f *flags) *flag.FlagSet {
	fs := flag.NewFlagSet("usarr", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&f.envFile, "env-file", "",
		"read KEY=VALUE lines from this file as environment defaults")
	fs.StringVar(&f.configDir, "config-dir", DefaultConfigDir,
		"irreplaceable state: database, keys, provider manifests, backups (USARR_CONFIG_DIR)")
	fs.StringVar(&f.dataDir, "data-dir", "",
		"regenerable state: caches, logs, temp; defaults to --config-dir (USARR_DATA_DIR)")
	fs.StringVar(&f.bindAddress, "bind-address", DefaultBindAddress,
		"interface to listen on (USARR_BIND_ADDRESS)")
	fs.IntVar(&f.port, "port", DefaultPort,
		"HTTP listen port (USARR_PORT)")
	fs.StringVar(&f.urlBase, "url-base", "",
		"sub-path when reverse-proxied, e.g. /usarr (USARR_URL_BASE)")
	fs.StringVar(&f.logLevel, "log-level", DefaultLogLevel,
		"trace, debug, info, warn or error (USARR_LOG_LEVEL)")
	fs.StringVar(&f.logFormat, "log-format", DefaultLogFormat,
		"auto, text or json (USARR_LOG_FORMAT)")
	fs.StringVar(&f.secretKeyFile, "secret-key-file", "",
		"path to a file holding the master key (USARR_SECRET_KEY_FILE)")
	fs.StringVar(&f.trustedProxies, "trusted-proxies", "",
		"comma-separated CIDRs whose X-Forwarded-* headers are believed; empty trusts nothing (USARR_TRUSTED_PROXIES)")
	fs.StringVar(&f.metadataUserAgent, "metadata-user-agent", "",
		"contact string sent to MusicBrainz and Open Library (USARR_METADATA_USER_AGENT)")
	fs.BoolVar(&f.showVersion, "version", false,
		"print the build identity (version, commit, build date, Go version) and exit")

	// There is deliberately no --secret-key. A command line is world-readable
	// through ps(1) and appears in container inspect output; the master key
	// belongs in the environment, a Docker secret, or the key file.

	// The FlagSet's own usage prints flags and nothing else, which left `key
	// rotate` undiscoverable from the binary: a subcommand the parser accepts,
	// docs/CONFIGURATION.md §3.4 documents, and `usarr --help` never mentions.
	// This is the one place every usage path already goes through — a parse
	// error and -h alike — so naming the subcommand here reaches both.
	fs.Usage = func() {
		// The write error is discarded for PrintDefaults' own reason: it
		// discards its writes too, so checking here would buy nothing and
		// there is no useful action left when printing usage has failed.
		_, _ = fmt.Fprint(fs.Output(),
			"Usage:\n"+
				"  usarr [flags]\n    \trun the server\n")
		for _, sc := range subcommands {
			_, _ = fmt.Fprintf(fs.Output(), "  usarr %s [flags]\n    \t%s\n",
				strings.Join(sc.words, " "), sc.summary)
		}
		_, _ = fmt.Fprint(fs.Output(), "\nFlags:\n")
		fs.PrintDefaults()
	}

	return fs
}

// WriteUsage prints the usage block -h reaches, to w. Exported because the
// answer now goes to stdout from the caller rather than out of an error, and
// because printing it is the whole of what ErrHelpRequested asks for.
func WriteUsage(w io.Writer) {
	var f flags
	fs := newFlagSet(&f)
	fs.SetOutput(w)
	fs.Usage()
}

func parseFlags(args []string) (flags, map[string]bool, error) {
	var f flags

	// A LEADING subcommand is lifted off before the FlagSet sees it. Go's flag
	// package stops parsing at the first non-flag argument, so without this
	// `usarr key rotate --config-dir /config` would treat --config-dir as a
	// positional and reject it — which is the form every operator will type
	// first. The trailing form is handled after Parse; both reach the same
	// FlagSet, so there is still exactly one parser and one idea of what a
	// valid argument is.
	for _, sc := range subcommands {
		if len(args) >= len(sc.words) && slices.Equal(args[:len(sc.words)], sc.words) {
			*sc.selected(&f) = true
			args = args[len(sc.words):]
			break
		}
	}

	fs := newFlagSet(&f)

	if err := fs.Parse(args); err != nil {
		// -h and --help are the ONE parse outcome that is not a failure: the
		// operator asked a question and the FlagSet answered it. Reporting a
		// non-zero status for that is what --version already declines to do.
		// Every OTHER parse error keeps its exit code and its stderr, prefix
		// and all — widening this would change how the binary reports a
		// genuinely malformed command line.
		if errors.Is(err, flag.ErrHelp) {
			return f, nil, ErrHelpRequested
		}
		var b strings.Builder
		fs.SetOutput(&b)
		fs.Usage()
		return f, nil, fmt.Errorf("parse flags: %w\n\n%s", err, b.String())
	}
	// Positional arguments. Exactly one form is a subcommand; EVERY other
	// positional keeps erroring exactly as it did before subcommands existed,
	// including the near misses — a bare `usarr key`, a `usarr key rotate now`,
	// or a typo'd verb. A parser that quietly accepted a prefix of a subcommand
	// would let `usarr key` mean something, and there is nothing it can mean.
	if fs.NArg() > 0 {
		matched := false
		// A trailing form is accepted only when nothing was lifted off the
		// front. Without that condition `usarr backup key rotate` would select
		// BOTH programs and run whichever Load happens to test for first.
		if selectedSubcommand(&f) < 0 {
			for _, sc := range subcommands {
				if slices.Equal(fs.Args(), sc.words) {
					*sc.selected(&f) = true
					matched = true
					break
				}
			}
		}
		if !matched {
			return f, nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
	}
	if i := selectedSubcommand(&f); f.showVersion && i >= 0 {
		// Two programs asked for at once. Silently letting --version win would
		// exit 0 having done nothing, which is the worst possible answer to
		// "did my key rotate?" or "did my backup run?".
		return f, nil, fmt.Errorf("--version and %q are two different commands; pass one",
			strings.Join(subcommands[i].words, " "))
	}

	set := map[string]bool{}
	fs.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	return f, set, nil
}
