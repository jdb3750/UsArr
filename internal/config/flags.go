package config

import (
	"flag"
	"fmt"
	"io"
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
}

func parseFlags(args []string) (flags, map[string]bool, error) {
	var f flags
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

	if err := fs.Parse(args); err != nil {
		var b strings.Builder
		fs.SetOutput(&b)
		fs.Usage()
		return f, nil, fmt.Errorf("parse flags: %w\n\n%s", err, b.String())
	}
	if fs.NArg() > 0 {
		return f, nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	set := map[string]bool{}
	fs.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	return f, set, nil
}
