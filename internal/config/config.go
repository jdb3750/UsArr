// Package config loads UsArr's level-1 bootstrap configuration.
//
// Level 1 is everything the process needs before it can open its database or
// trust a request: where state lives, what socket to bind, how to log, the
// master key, and which proxies may be believed. It comes from flags and the
// environment only — never from the database and never from the UI, because the
// master key decrypts the database and the trusted-proxy allowlist decides
// whether a forwarded header is believed. If either were editable through the
// surface it protects, a UI authorization bug would be a full compromise.
//
// Level 2 — service instances and their credentials — lives in SQLite and is
// written only by the UI. There is no config file, no services.yaml and no
// per-service environment variable. See docs/CONFIGURATION.md §1.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Default values for every variable that has one. docs/CONFIGURATION.md §2.
const (
	DefaultConfigDir   = "/config"
	DefaultBindAddress = "0.0.0.0"

	// DefaultPort is 8484 because it avoids every default port in the
	// ecosystem UsArr talks to (8989/7878/8686/8787/9696/6969/5299/8096/
	// 32400/8080/6789/9091/8112/6767/5055).
	DefaultPort = 8484

	DefaultLogLevel  = "info"
	DefaultLogFormat = "auto"
	DefaultTimezone  = "Etc/UTC"

	// DefaultVersion is used when main was built without -ldflags stamping.
	DefaultVersion = "0.0.0-dev"
)

// File and directory modes. UsArr sets these itself rather than inheriting a
// process umask, so the on-disk layout in docs/CONFIGURATION.md §5 holds
// regardless of how the process was started.
const (
	DirMode       os.FileMode = 0o700
	FileMode      os.FileMode = 0o600
	SecretDirMode os.FileMode = 0o700
	SecretMode    os.FileMode = 0o600
)

// Config is the resolved level-1 configuration. Every field is final: nothing
// re-reads the environment after Load returns.
type Config struct {
	// Core process.
	ConfigDir   string
	DataDir     string
	BindAddress string
	Port        int
	URLBase     string
	LogLevel    string
	LogFormat   string

	// Security. SecretKey holds the raw value of USARR_SECRET_KEY exactly as
	// supplied — including the empty string, which is a fatal condition rather
	// than "absent" (see ResolveMasterKey). Use ResolveMasterKey; do not read
	// this field directly.
	SecretKey       string
	SecretKeySet    bool
	SecretKeyFile   string
	TrustedProxies  []netip.Prefix
	trustedRawEmpty bool

	// Metadata.
	MetadataUserAgent string

	// Container.
	Timezone string

	// Development only. Neither is read by a production code path.
	Integration bool
	Record      bool

	// Version is the build version, stamped into the default user agent.
	Version string

	// EnvFile is the path passed to --env-file, if any, and EnvFileMissing
	// reports that the path did not exist. A missing file is deliberately not
	// fatal: `make dev` passes --env-file .env unconditionally and prints its
	// own note when the file is absent.
	EnvFile        string
	EnvFileMissing bool
}

// Address is the host:port to listen on.
// redactedMarker is what stands in for the master key in any rendering of
// Config. It is non-empty so "the key was set" stays distinguishable from "the
// key was absent" — which is a real diagnostic distinction, since an empty
// USARR_SECRET_KEY is a fatal condition and an unset one is not.
const redactedMarker = "<redacted>"

// LogValue implements slog.LogValuer so that `slog.Any("config", cfg)` — at any
// level, including trace — cannot print the master key.
//
// security.md §5 requires redaction to be "a middleware, not a convention", and
// the *Arr keys already satisfy that structurally: they exist only as sealed
// envelopes in store, and the servarr client rebuilds transport errors
// specifically so the key never reaches an error string. The master key was the
// exception — a plain string field on a struct that any one of
// slog.Debug("config", "cfg", cfg), fmt.Sprintf("%+v", cfg) or a support-bundle
// dump would have written out in full. That key opens every stored *Arr
// credential, so it is the worst single value in the process to leak.
//
// The receiver is a VALUE, deliberately: a method on *Config would leave a
// plain `Config` value unprotected, and the leak paths above are exactly the
// ones that pass a value.
//
// This renders an explicit allow-list of fields rather than reflecting over the
// struct. A new field is therefore invisible here until someone adds it, which
// is the right default: forgetting to add a field costs a log line, forgetting
// to exclude one costs a credential.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("config_dir", c.ConfigDir),
		slog.String("data_dir", c.DataDir),
		slog.String("bind_address", c.BindAddress),
		slog.Int("port", c.Port),
		slog.String("url_base", c.URLBase),
		slog.String("log_level", c.LogLevel),
		slog.String("log_format", c.LogFormat),
		slog.String("secret_key", c.redactedSecretKey()),
		slog.Bool("secret_key_set", c.SecretKeySet),
		// The PATH is not a secret and is load-bearing for diagnosing a
		// first-run key problem; the file's contents never come near this type.
		slog.String("secret_key_file", c.SecretKeyFile),
		slog.Int("trusted_proxies", len(c.TrustedProxies)),
		slog.String("metadata_user_agent", c.MetadataUserAgent),
		slog.String("timezone", c.Timezone),
	)
}

// String implements fmt.Stringer so that %v, %s and %+v on a Config are all
// safe. %+v is the one that matters: without a String method it prints every
// field including SecretKey, and it is what someone reaches for when debugging
// startup. Value receiver for the same reason as LogValue.
func (c Config) String() string {
	return fmt.Sprintf(
		"Config{config_dir:%s data_dir:%s addr:%s:%d url_base:%s log:%s/%s secret_key:%s secret_key_set:%t secret_key_file:%s trusted_proxies:%d tz:%s}",
		c.ConfigDir, c.DataDir, c.BindAddress, c.Port, c.URLBase,
		c.LogLevel, c.LogFormat, c.redactedSecretKey(), c.SecretKeySet,
		c.SecretKeyFile, len(c.TrustedProxies), c.Timezone,
	)
}

// redactedSecretKey never returns any part of the key — not a prefix, not a
// length. A length leaks whether the value is a 32-byte base64 key or a
// passphrase, and a prefix is simply a shorter key to brute-force.
func (c Config) redactedSecretKey() string {
	if !c.SecretKeySet {
		return ""
	}
	if c.SecretKey == "" {
		// Set-but-empty is the fatal case CONFIGURATION.md §3.2 describes.
		// Rendering it distinctly is what makes that error diagnosable.
		return "<empty>"
	}
	return redactedMarker
}

func (c *Config) Address() string {
	return c.BindAddress + ":" + strconv.Itoa(c.Port)
}

// On-disk layout. docs/CONFIGURATION.md §5 is authoritative for this tree.

// KeysDir holds the master key, and ONLY actual secrets. It is excluded from
// every backup, so that "back up this volume" and "never store the key with the
// ciphertext" are not the same instruction.
//
// Nothing goes in here that is not secret. A non-secret that the database
// cannot be opened without — the KEK salt used to live here — turns the
// documented `--exclude='keys'` from correct advice into a trap: the operator
// follows it, the archive silently omits an unrecoverable input, and the
// restore destroys every stored credential. See KEKSaltPath.
func (c *Config) KeysDir() string { return filepath.Join(c.ConfigDir, "keys") }

// SecretKeyPath is the master key file written on first run.
func (c *Config) SecretKeyPath() string { return filepath.Join(c.KeysDir(), "secret.key") }

// NewSecretKeyPath exists only mid-rotation. Its presence at startup means a
// rotation was interrupted and must resume. docs/CONFIGURATION.md §3.4.
func (c *Config) NewSecretKeyPath() string { return filepath.Join(c.KeysDir(), "secret.key.new") }

// KEKSaltPath holds the per-install HKDF salt for key derivation. It sits
// beside usarr.db, NOT in keys/.
//
// docs/reference/security.md §1.3 requires a stored per-install salt but does
// not say where it lives, and the schema has no settings table to hold it.
//
// It used to live in keys/, on the reasoning that it shares the master key's
// fate — lose either and every stored credential is unrecoverable. That much is
// true, and the fail-closed check in ResolveKEKSalt exists because it is true.
// But it is the wrong conclusion about WHERE the file goes, and it produced a
// data-loss bug: keys/ is the one directory the documented backup deliberately
// excludes, so a non-secret that the database cannot be opened without was
// being dropped from every archive an operator was told to take.
//
// The salt is not secret — its value does not depend on confidentiality, only
// on being per-install and stable — so it does not belong with the secrets. It
// belongs with the ciphertext it is an input to. Beside the database, an
// ordinary backup of the config volume captures it and `--exclude='keys'` is
// correct exactly as written, which makes the catastrophic restore unreachable
// rather than merely documented.
//
// It is CONFIG_DIR and not DATA_DIR, and that is load-bearing rather than
// arbitrary. CONFIGURATION.md §5 defines DATA_DIR as regenerable and documents
// it as safe to delete — the cache, the logs, scratch space, all of it rebuilt
// on the next run. An input that cannot be regenerated therefore cannot live
// there: putting the salt under DATA_DIR would make "safe to delete" false for
// exactly one file in it, and the operator who acts on the documented promise
// loses every stored credential. Do not move it to sit next to the caches.
//
// ResolveKEKSalt COPIES an existing keys/kek.salt here on startup and leaves the
// original in place forever; see LegacyKEKSaltPath. It is 0600 like every other
// file UsArr writes, which is caution rather than necessity.
func (c *Config) KEKSaltPath() string { return filepath.Join(c.ConfigDir, "kek.salt") }

// LegacyKEKSaltPath is where the KEK salt lived before KEKSaltPath moved beside
// the database. Installs created before that change still have it here, so
// startup COPIES it forward rather than treating it as missing — treating it as
// missing would destroy exactly the credentials the change exists to protect.
//
// The file at this path is never deleted and never renamed. ResolveKEKSalt says
// why the carry-forward must not be a move.
func (c *Config) LegacyKEKSaltPath() string { return filepath.Join(c.KeysDir(), "kek.salt") }

// DatabasePath is the main database: library, services, users, audit log.
func (c *Config) DatabasePath() string { return filepath.Join(c.ConfigDir, "usarr.db") }

// ProvidersDir holds Tier 1 YAML service manifests. User data, not regenerable.
func (c *Config) ProvidersDir() string { return filepath.Join(c.ConfigDir, "providers") }

// BackupsDir holds `VACUUM INTO` output: ciphertext, never a key.
func (c *Config) BackupsDir() string { return filepath.Join(c.ConfigDir, "backups") }

// CacheDatabasePath is the disposable HTTP/metadata response cache. It is a
// separate database and is never ATTACHed inside a usarr.db write transaction.
func (c *Config) CacheDatabasePath() string { return filepath.Join(c.DataDir, "cache.db") }

// ImageCacheDir holds proxied artwork, content-keyed. Safe to delete.
func (c *Config) ImageCacheDir() string { return filepath.Join(c.DataDir, "cache", "images") }

// LogsDir holds the rotating log files (fixed at 10 MB × 5).
func (c *Config) LogsDir() string { return filepath.Join(c.DataDir, "logs") }

// TrustedProxyWarnings reports configured prefixes that are wider than any
// reverse-proxy network should be. These are warnings, not startup failures:
// docs/CONFIGURATION.md §2.2 states the rule for the operator but does not make
// it fatal, and refusing to start would break an existing deployment on upgrade.
//
// A prefix this wide means every request appears to come from a trusted proxy,
// which silently disables per-IP rate limiting and makes the audit log's
// actor_ip meaningless.
func (c *Config) TrustedProxyWarnings() []string {
	var out []string
	for _, p := range c.TrustedProxies {
		if p.Bits() == 0 {
			out = append(out, fmt.Sprintf(
				"USARR_TRUSTED_PROXIES contains %s, which trusts every host: "+
					"per-IP rate limiting and audit actor_ip stop working", p))
		}
	}
	return out
}

// Options are the inputs to Load. Both Args and Env are supplied explicitly so
// the whole of level 1 is testable without touching the process environment.
type Options struct {
	// Args is the command line without argv[0].
	Args []string
	// Env is the process environment as a map.
	Env map[string]string
	// Version is stamped into the default metadata user agent.
	Version string
	// ReadFile reads --env-file. Nil means os.ReadFile.
	ReadFile func(string) ([]byte, error)
}

// LoadOS loads configuration from the real command line and environment.
func LoadOS(version string) (*Config, error) {
	return Load(Options{
		Args:    os.Args[1:],
		Env:     Environ(os.Environ()),
		Version: version,
	})
}

// Environ converts os.Environ()'s KEY=VALUE slice to a map.
func Environ(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		env[k] = v
	}
	return env
}

// Load resolves level-1 configuration. Precedence, first match wins:
//
//  1. command-line flag
//  2. environment variable
//  3. USARR_SECRET_KEY_FILE   (the one variable with a file twin)
//  4. built-in default
//
// Values read from --env-file sit below the real process environment: an
// explicitly exported variable always beats a stale line in a checked-out .env.
// This matches Docker Compose, where `environment:` overrides `env_file:`.
func Load(o Options) (*Config, error) {
	f, set, err := parseFlags(o.Args)
	if err != nil {
		return nil, err
	}

	env := o.Env
	if env == nil {
		env = map[string]string{}
	}

	c := &Config{Version: cmp(o.Version, DefaultVersion)}

	if f.envFile != "" {
		c.EnvFile = f.envFile
		readFile := o.ReadFile
		if readFile == nil {
			readFile = os.ReadFile
		}
		raw, err := readFile(f.envFile)
		switch {
		case errors.Is(err, os.ErrNotExist):
			c.EnvFileMissing = true
		case err != nil:
			return nil, fmt.Errorf("read --env-file %s: %w", f.envFile, err)
		default:
			fileEnv, err := ParseEnvFile(string(raw))
			if err != nil {
				return nil, fmt.Errorf("parse --env-file %s: %w", f.envFile, err)
			}
			merged := make(map[string]string, len(env)+len(fileEnv))
			for k, v := range fileEnv {
				merged[k] = v
			}
			for k, v := range env {
				merged[k] = v
			}
			env = merged
		}
	}

	// USARR_CONFIG_DIR — irreplaceable state.
	c.ConfigDir = pick(set["config-dir"], f.configDir, env, "USARR_CONFIG_DIR", DefaultConfigDir)
	if c.ConfigDir == "" {
		return nil, errors.New("USARR_CONFIG_DIR: must not be empty")
	}
	c.ConfigDir = filepath.Clean(c.ConfigDir)

	// USARR_DATA_DIR — regenerable state; defaults to the config dir so a
	// single-volume install needs no second setting.
	c.DataDir = pick(set["data-dir"], f.dataDir, env, "USARR_DATA_DIR", c.ConfigDir)
	c.DataDir = filepath.Clean(c.DataDir)

	c.BindAddress = pick(set["bind-address"], f.bindAddress, env, "USARR_BIND_ADDRESS", DefaultBindAddress)
	if _, err := netip.ParseAddr(c.BindAddress); err != nil {
		return nil, fmt.Errorf("USARR_BIND_ADDRESS: %q is not an IP address", c.BindAddress)
	}

	portRaw := pick(set["port"], strconv.Itoa(f.port), env, "USARR_PORT", strconv.Itoa(DefaultPort))
	port, err := strconv.Atoi(strings.TrimSpace(portRaw))
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("USARR_PORT: %q is not a port number in 1..65535", portRaw)
	}
	c.Port = port

	c.URLBase, err = normalizeURLBase(pick(set["url-base"], f.urlBase, env, "USARR_URL_BASE", ""))
	if err != nil {
		return nil, err
	}

	c.LogLevel = pick(set["log-level"], f.logLevel, env, "USARR_LOG_LEVEL", DefaultLogLevel)
	if !oneOf(c.LogLevel, "trace", "debug", "info", "warn", "error") {
		return nil, fmt.Errorf("USARR_LOG_LEVEL: %q is not one of trace, debug, info, warn, error", c.LogLevel)
	}

	c.LogFormat = pick(set["log-format"], f.logFormat, env, "USARR_LOG_FORMAT", DefaultLogFormat)
	if !oneOf(c.LogFormat, "auto", "text", "json") {
		return nil, fmt.Errorf("USARR_LOG_FORMAT: %q is not one of auto, text, json", c.LogFormat)
	}

	// USARR_SECRET_KEY has no flag twin on purpose: a command line is visible
	// to every process on the host via ps(1) and to the container runtime's
	// inspect output. The path twin below is a path, not a secret, so it does
	// get a flag.
	c.SecretKey, c.SecretKeySet = env["USARR_SECRET_KEY"]
	c.SecretKeyFile = pick(set["secret-key-file"], f.secretKeyFile, env, "USARR_SECRET_KEY_FILE", "")

	trustedRaw := pick(set["trusted-proxies"], f.trustedProxies, env, "USARR_TRUSTED_PROXIES", "")
	c.TrustedProxies, err = parseTrustedProxies(trustedRaw)
	if err != nil {
		return nil, err
	}
	c.trustedRawEmpty = strings.TrimSpace(trustedRaw) == ""

	c.MetadataUserAgent = pick(set["metadata-user-agent"], f.metadataUserAgent, env,
		"USARR_METADATA_USER_AGENT", defaultUserAgent(c.Version))

	c.Timezone = cmp(env["TZ"], DefaultTimezone)

	c.Integration = truthy(env["USARR_INTEGRATION"])
	c.Record = truthy(env["USARR_RECORD"])

	return c, nil
}

// TrustsNothing reports that no proxy is trusted, so X-Forwarded-* headers are
// ignored and the peer IP is used as-is. This is the default and it is correct
// for a direct-port deployment.
func (c *Config) TrustsNothing() bool { return c.trustedRawEmpty || len(c.TrustedProxies) == 0 }

// defaultUserAgent is transmitted to MusicBrainz (mandatory) and Open Library
// (1 → 3 req/s), and to nobody else. docs/CONFIGURATION.md §2.3.
func defaultUserAgent(version string) string {
	return "UsArr/" + version + " (+https://github.com/jdb3750/UsArr)"
}

// pick applies the level-1 precedence chain for one setting.
func pick(flagSet bool, flagVal string, env map[string]string, envKey, def string) string {
	if flagSet {
		return flagVal
	}
	if v, ok := env[envKey]; ok && v != "" {
		return v
	}
	return def
}

func cmp(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// normalizeURLBase enforces "leading slash, no trailing slash". A trailing
// slash is corrected rather than rejected because the correction is
// unambiguous; anything that looks like a full URL is rejected, because that
// is a misunderstanding of the setting rather than a typo.
func normalizeURLBase(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	if strings.Contains(v, "://") {
		return "", fmt.Errorf("USARR_URL_BASE: %q looks like a URL; it must be a path such as /usarr", raw)
	}
	if strings.Contains(v, "..") {
		return "", fmt.Errorf("USARR_URL_BASE: %q must not contain %q", raw, "..")
	}
	v = "/" + strings.Trim(v, "/")
	if v == "/" {
		return "", nil
	}
	return v, nil
}

// parseTrustedProxies accepts comma-separated CIDRs. A bare IP address is
// accepted and treated as a host route (/32 or /128) — writing a single proxy
// address without a mask is the obvious mistake and there is only one thing it
// can mean.
func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("USARR_TRUSTED_PROXIES: %q is not a CIDR or IP address", part)
		}
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}
