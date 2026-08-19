package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	c, err := Load(Options{Version: "1.2.3"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"ConfigDir", c.ConfigDir, DefaultConfigDir},
		{"DataDir", c.DataDir, DefaultConfigDir},
		{"BindAddress", c.BindAddress, DefaultBindAddress},
		{"Port", c.Port, DefaultPort},
		{"URLBase", c.URLBase, ""},
		{"LogLevel", c.LogLevel, DefaultLogLevel},
		{"LogFormat", c.LogFormat, DefaultLogFormat},
		{"Timezone", c.Timezone, DefaultTimezone},
		{"MetadataUserAgent", c.MetadataUserAgent, "UsArr/1.2.3 (+https://github.com/jdb3750/UsArr)"},
		{"Integration", c.Integration, false},
		{"Record", c.Record, false},
	}
	for _, tc := range checks {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	if len(c.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies = %v, want empty (trust nothing)", c.TrustedProxies)
	}
	if !c.TrustsNothing() {
		t.Error("TrustsNothing() = false with no proxies configured; empty must mean trust nothing")
	}
}

// TestPrecedence walks the level-1 chain from docs/CONFIGURATION.md §1:
// flag > environment > default.
func TestPrecedence(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want func(*Config) (string, any, any)
	}{
		{
			name: "default wins when nothing is set",
			want: func(c *Config) (string, any, any) { return "Port", c.Port, 8484 },
		},
		{
			name: "env beats default",
			env:  map[string]string{"USARR_PORT": "9000"},
			want: func(c *Config) (string, any, any) { return "Port", c.Port, 9000 },
		},
		{
			name: "flag beats env",
			args: []string{"--port", "7000"},
			env:  map[string]string{"USARR_PORT": "9000"},
			want: func(c *Config) (string, any, any) { return "Port", c.Port, 7000 },
		},
		{
			name: "flag beats env for strings",
			args: []string{"--log-level", "debug"},
			env:  map[string]string{"USARR_LOG_LEVEL": "error"},
			want: func(c *Config) (string, any, any) { return "LogLevel", c.LogLevel, "debug" },
		},
		{
			name: "data dir defaults to config dir",
			env:  map[string]string{"USARR_CONFIG_DIR": "/srv/usarr"},
			want: func(c *Config) (string, any, any) { return "DataDir", c.DataDir, "/srv/usarr" },
		},
		{
			name: "data dir overrides independently",
			env:  map[string]string{"USARR_CONFIG_DIR": "/srv/usarr", "USARR_DATA_DIR": "/mnt/scratch"},
			want: func(c *Config) (string, any, any) { return "DataDir", c.DataDir, "/mnt/scratch" },
		},
		{
			// An empty environment value is not a choice; it falls through to
			// the default. USARR_SECRET_KEY is the sole exception and is
			// handled by the key ladder, not here.
			name: "empty env value falls through to the default",
			env:  map[string]string{"USARR_LOG_LEVEL": ""},
			want: func(c *Config) (string, any, any) { return "LogLevel", c.LogLevel, "info" },
		},
		{
			name: "flag can set an empty url base explicitly",
			args: []string{"--url-base", ""},
			env:  map[string]string{"USARR_URL_BASE": "/usarr"},
			want: func(c *Config) (string, any, any) { return "URLBase", c.URLBase, "" },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Load(Options{Args: tc.args, Env: tc.env})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			name, got, want := tc.want(c)
			if got != want {
				t.Errorf("%s = %v, want %v", name, got, want)
			}
		})
	}
}

func TestURLBaseNormalisation(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ""},
		{in: "/", want: ""},
		{in: "/usarr", want: "/usarr"},
		{in: "usarr", want: "/usarr"},
		{in: "/usarr/", want: "/usarr"},
		{in: "usarr/", want: "/usarr"},
		{in: "/media/usarr", want: "/media/usarr"},
		{in: "  /usarr  ", want: "/usarr"},
		{in: "https://home.example/usarr", wantErr: true},
		{in: "/usarr/../etc", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			c, err := Load(Options{Env: map[string]string{"USARR_URL_BASE": tc.in}})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Load(%q) = nil error, want error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(%q): %v", tc.in, err)
			}
			if c.URLBase != tc.want {
				t.Errorf("URLBase = %q, want %q", c.URLBase, tc.want)
			}
		})
	}
}

func TestInvalidValuesAreFatal(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{name: "port not a number", env: map[string]string{"USARR_PORT": "http"}, want: "USARR_PORT"},
		{name: "port zero", env: map[string]string{"USARR_PORT": "0"}, want: "USARR_PORT"},
		{name: "port too large", env: map[string]string{"USARR_PORT": "70000"}, want: "USARR_PORT"},
		{name: "bind address not an IP", env: map[string]string{"USARR_BIND_ADDRESS": "localhost"}, want: "USARR_BIND_ADDRESS"},
		{name: "log level unknown", env: map[string]string{"USARR_LOG_LEVEL": "verbose"}, want: "USARR_LOG_LEVEL"},
		{name: "log format unknown", env: map[string]string{"USARR_LOG_FORMAT": "xml"}, want: "USARR_LOG_FORMAT"},
		{name: "trusted proxies garbage", env: map[string]string{"USARR_TRUSTED_PROXIES": "not-a-cidr"}, want: "USARR_TRUSTED_PROXIES"},
		{name: "unknown flag", args: []string{"--nope"}, want: "parse flags"},
		{name: "stray positional argument", args: []string{"serve"}, want: "unexpected argument"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(Options{Args: tc.args, Env: tc.env})
			if err == nil {
				t.Fatalf("Load = nil error, want an error naming %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %q", err, tc.want)
			}
		})
	}
}

func TestTrustedProxies(t *testing.T) {
	c, err := Load(Options{Env: map[string]string{
		"USARR_TRUSTED_PROXIES": "10.0.0.0/8, 192.168.1.5 ,fd00::/8",
	}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := make([]string, 0, len(c.TrustedProxies))
	for _, p := range c.TrustedProxies {
		got = append(got, p.String())
	}
	want := []string{"10.0.0.0/8", "192.168.1.5/32", "fd00::/8"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("TrustedProxies = %v, want %v", got, want)
	}
	if c.TrustsNothing() {
		t.Error("TrustsNothing() = true with proxies configured")
	}
	if len(c.TrustedProxyWarnings()) != 0 {
		t.Errorf("unexpected warnings: %v", c.TrustedProxyWarnings())
	}
}

func TestTrustedProxiesWarnsOnCatchAll(t *testing.T) {
	for _, cidr := range []string{"0.0.0.0/0", "::/0"} {
		c, err := Load(Options{Env: map[string]string{"USARR_TRUSTED_PROXIES": cidr}})
		if err != nil {
			t.Fatalf("Load(%q): %v", cidr, err)
		}
		if len(c.TrustedProxyWarnings()) == 0 {
			t.Errorf("%s produced no warning; it disables per-IP rate limiting", cidr)
		}
	}
}

func TestLayoutPaths(t *testing.T) {
	c, err := Load(Options{Env: map[string]string{
		"USARR_CONFIG_DIR": "/config",
		"USARR_DATA_DIR":   "/data",
	}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// docs/CONFIGURATION.md §5 is authoritative for this tree.
	checks := map[string]string{
		c.KeysDir():           "/config/keys",
		c.SecretKeyPath():     "/config/keys/secret.key",
		c.NewSecretKeyPath():  "/config/keys/secret.key.new",
		c.DatabasePath():      "/config/usarr.db",
		c.ProvidersDir():      "/config/providers",
		c.BackupsDir():        "/config/backups",
		c.CacheDatabasePath(): "/data/cache.db",
		c.ImageCacheDir():     "/data/cache/images",
		c.LogsDir():           "/data/logs",
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	}
}

func TestEnvFileIsReadBelowTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "USARR_PORT=9999\nUSARR_LOG_LEVEL=debug\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// The env file supplies what the environment does not.
	c, err := Load(Options{Args: []string{"--env-file", path}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 9999 || c.LogLevel != "debug" {
		t.Errorf("env file not applied: Port=%d LogLevel=%q", c.Port, c.LogLevel)
	}

	// A real environment variable beats the file, matching Docker Compose's
	// `environment:` over `env_file:`.
	c, err = Load(Options{
		Args: []string{"--env-file", path},
		Env:  map[string]string{"USARR_PORT": "1234"},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 1234 {
		t.Errorf("Port = %d, want 1234: the process environment must beat --env-file", c.Port)
	}
	if c.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug from the file", c.LogLevel)
	}

	// And a flag beats both.
	c, err = Load(Options{
		Args: []string{"--env-file", path, "--port", "4321"},
		Env:  map[string]string{"USARR_PORT": "1234"},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 4321 {
		t.Errorf("Port = %d, want 4321: a flag must beat both", c.Port)
	}
}

// A missing --env-file must not be fatal: `make dev` passes --env-file .env
// unconditionally and prints its own note when the file is absent.
func TestMissingEnvFileIsNotFatal(t *testing.T) {
	c, err := Load(Options{Args: []string{"--env-file", filepath.Join(t.TempDir(), "absent")}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.EnvFileMissing {
		t.Error("EnvFileMissing = false; the caller cannot tell the file was absent")
	}
	if c.Port != DefaultPort {
		t.Errorf("Port = %d, want the default %d", c.Port, DefaultPort)
	}
}

// TestConfigNeverRendersTheMasterKey is the regression test for CFG-01.
//
// Config.SecretKey held the raw USARR_SECRET_KEY in a plain string field with no
// Stringer and no slog.LogValuer, so a single slog.Debug("config", "cfg", cfg),
// fmt.Sprintf("%+v", cfg) or support-bundle dump would have written the master
// key to the log — the key that opens every stored *Arr credential. security.md
// §5 requires redaction to be structural ("a middleware, not a convention"), and
// this is the guarantee being structural rather than remembered.
//
// Both a value and a pointer are exercised: the methods are on a value receiver
// precisely so that neither form leaks, and a future move to a pointer receiver
// would silently reopen the value path. That is why %+v on the plain value is
// asserted here.
// verbS keeps the %s verb out of staticcheck's S1025 pattern match.
var verbS = "%s"

func TestConfigNeverRendersTheMasterKey(t *testing.T) {
	const key = "SUPERSECRETMASTERKEYVALUE0123456789"

	c := Config{
		ConfigDir:    "/config",
		DataDir:      "/data",
		BindAddress:  "0.0.0.0",
		Port:         6969,
		SecretKey:    key,
		SecretKeySet: true,
		// A path is not a secret; the file's contents never reach this struct.
		SecretKeyFile: "/run/secrets/usarr_key",
	}

	renderings := map[string]string{
		"String()": c.String(),
		"%v value": fmt.Sprintf("%v", c),
		// The format string is a variable so staticcheck's S1025 ("use String()")
		// does not rewrite this away. Exercising %s explicitly is the point: it is
		// a distinct fmt verb path from %v, and both must route through Stringer.
		"%s value":     fmt.Sprintf(verbS, c),
		"%+v value":    fmt.Sprintf("%+v", c),
		"%v pointer":   fmt.Sprintf("%v", &c),
		"%+v pointer":  fmt.Sprintf("%+v", &c),
		"LogValue()":   c.LogValue().String(),
		"slog value":   slogLine(t, c),
		"slog pointer": slogLine(t, &c),
	}

	for name, got := range renderings {
		if strings.Contains(got, key) {
			t.Errorf("%s leaked the master key:\n%s", name, got)
		}
		// A prefix is just a shorter key to brute-force, so no part may survive.
		if strings.Contains(got, key[:8]) {
			t.Errorf("%s leaked a prefix of the master key:\n%s", name, got)
		}
		if !strings.Contains(got, "<redacted>") {
			t.Errorf("%s does not mark the key as redacted, so a reader cannot tell "+
				"a set key from an absent one:\n%s", name, got)
		}
		// The non-secret fields must still be there, or this is just an opaque
		// blob and nobody will use it.
		if !strings.Contains(got, "/config") {
			t.Errorf("%s dropped config_dir; redaction must not cost all diagnostics:\n%s", name, got)
		}
	}

	// An unset key and a set-but-empty key must stay distinguishable: an empty
	// USARR_SECRET_KEY is fatal (CONFIGURATION.md §3.2) and an unset one is not.
	unset := Config{ConfigDir: "/config"}
	empty := Config{ConfigDir: "/config", SecretKeySet: true}
	if unset.String() == empty.String() {
		t.Error("an unset key and a set-but-empty key render identically; " +
			"that distinction is what makes the empty-key failure diagnosable")
	}
}

// slogLine renders v through a real slog handler, which is the path that
// actually matters — LogValuer is only consulted by the logging machinery.
func slogLine(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.New(h).Debug("startup", "config", v)
	return buf.String()
}

// TestVersionFlagShortCircuitsLoad pins the two properties --version needs to be
// usable as "what is installed on this host?": it answers before Load can touch
// anything privileged, and it does not weaken argument validation.
func TestVersionFlagShortCircuitsLoad(t *testing.T) {
	// Go's flag package accepts one and two dashes for the same flag; both are
	// documented to operators, so both are pinned.
	for _, arg := range []string{"--version", "-version"} {
		t.Run(arg, func(t *testing.T) {
			c, err := Load(Options{
				Args: []string{arg, "--env-file", "/nonexistent/usarr.env"},
				Env:  map[string]string{"USARR_PORT": "not-a-number"},
				ReadFile: func(string) ([]byte, error) {
					t.Fatal("--version must return before --env-file is read")
					return nil, nil
				},
			})
			if !errors.Is(err, ErrVersionRequested) {
				t.Fatalf("Load = %v, want ErrVersionRequested", err)
			}
			// A nil Config is the point: there is nothing resolved to use, and
			// an invalid USARR_PORT alongside it never had to be validated.
			if c != nil {
				t.Errorf("Load returned a Config = %+v, want nil", c)
			}
		})
	}

	// The short-circuit must not have turned the FlagSet into a permissive one.
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"--nope"}, want: "parse flags"},
		{args: []string{"serve"}, want: "unexpected argument"},
		{args: []string{"--version", "extra"}, want: "unexpected argument"},
	} {
		_, err := Load(Options{Args: tc.args})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Load(%v) = %v, want an error naming %q", tc.args, err, tc.want)
		}
		if errors.Is(err, ErrVersionRequested) {
			t.Errorf("Load(%v) short-circuited to --version", tc.args)
		}
	}
}

// TestUsageNamesTheSubcommand: `usarr --help` must mention `key rotate`.
//
// The FlagSet's default usage prints flags and nothing else, so for as long as
// that was what -h reached, the subcommand was documented everywhere except in
// the binary that implements it — an operator could read the whole of --help and
// conclude UsArr has no way to rotate a key.
//
// It is asserted through Load rather than by calling the usage function, because
// the usage function is unexported and, more to the point, what matters is that
// the text reaches the path -h actually takes.
func TestUsageNamesTheSubcommand(t *testing.T) {
	_, err := Load(Options{Args: []string{"-h"}})
	if err == nil {
		t.Fatal("Load(-h) returned no error; -h has to surface the usage text somewhere")
	}
	for _, want := range []string{"key rotate", "Flags:", "-secret-key-file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("`usarr -h` output does not contain %q:\n%s", want, err)
		}
	}
}

// TestSecretKeyFileRecordsItsSource pins the provenance the refusal in
// cmd/usarr/keyrotate.go reads. The flag and the variable resolve into ONE
// field, so without this bit the only honest thing a diagnostic could say is
// "one of these two", and the message said the wrong one of them outright.
func TestSecretKeyFileRecordsItsSource(t *testing.T) {
	dir := t.TempDir()
	const path = "/keys/elsewhere.key"

	c, err := Load(Options{Args: []string{"--config-dir", dir, "--secret-key-file", path}})
	if err != nil {
		t.Fatal(err)
	}
	if c.SecretKeyFile != path || !c.SecretKeyFileFromFlag {
		t.Errorf("--secret-key-file %q resolved to %q, from flag = %v; want the path and true",
			path, c.SecretKeyFile, c.SecretKeyFileFromFlag)
	}

	c, err = Load(Options{
		Args: []string{"--config-dir", dir},
		Env:  map[string]string{"USARR_SECRET_KEY_FILE": path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.SecretKeyFile != path || c.SecretKeyFileFromFlag {
		t.Errorf("USARR_SECRET_KEY_FILE=%q resolved to %q, from flag = %v; want the path and false",
			path, c.SecretKeyFile, c.SecretKeyFileFromFlag)
	}

	// The flag wins over the variable, and the provenance follows the winner
	// rather than the loser — which is the case that made the old message wrong.
	c, err = Load(Options{
		Args: []string{"--config-dir", dir, "--secret-key-file", path},
		Env:  map[string]string{"USARR_SECRET_KEY_FILE": "/keys/from-the-environment.key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.SecretKeyFile != path || !c.SecretKeyFileFromFlag {
		t.Errorf("flag and variable both set: resolved to %q, from flag = %v; want %q and true",
			c.SecretKeyFile, c.SecretKeyFileFromFlag, path)
	}
}

// TestKeyRotateSubcommand pins the parsing rule for the ONE positional form the
// binary accepts, and — more importantly — that every other positional still
// errors exactly as it did before subcommands existed. A parser that quietly
// accepted a prefix or a near miss would let `usarr key` mean something.
func TestKeyRotateSubcommand(t *testing.T) {
	dir := t.TempDir()

	// Both orderings resolve, because Go's flag package stops at the first
	// non-flag argument and the leading form is the one an operator types.
	for _, args := range [][]string{
		{"key", "rotate", "--config-dir", dir},
		{"--config-dir", dir, "key", "rotate"},
	} {
		c, err := Load(Options{Args: args})
		if !errors.Is(err, ErrKeyRotateRequested) {
			t.Fatalf("Load(%v) = %v, want ErrKeyRotateRequested", args, err)
		}
		// Unlike --version, a resolved Config comes back with it: a rotation
		// needs the config directory, and re-parsing to find it is the drift
		// that one parser exists to prevent.
		if c == nil {
			t.Fatalf("Load(%v) returned a nil Config; the rotation needs the resolved settings", args)
		}
		if c.ConfigDir != dir {
			t.Errorf("ConfigDir = %q, want %q", c.ConfigDir, dir)
		}
	}

	// The near misses. Every one of these is the pre-subcommand error.
	for _, args := range [][]string{
		{"key"},
		{"rotate"},
		{"key", "rotate", "now"},
		{"key", "rotat"},
		{"keys", "rotate"},
		{"serve"},
	} {
		_, err := Load(Options{Args: args})
		if err == nil || !strings.Contains(err.Error(), "unexpected argument") {
			t.Errorf("Load(%v) = %v, want the unchanged \"unexpected argument\" error", args, err)
		}
		if errors.Is(err, ErrKeyRotateRequested) {
			t.Errorf("Load(%v) resolved to a rotation", args)
		}
	}

	// Two programs at once is a refusal, not a silent win for one of them.
	if _, err := Load(Options{Args: []string{"key", "rotate", "--version"}}); err == nil ||
		errors.Is(err, ErrVersionRequested) || errors.Is(err, ErrKeyRotateRequested) {
		t.Errorf("`key rotate --version` = %v, want a refusal naming both commands", err)
	}

	// A rotation still runs against validated settings: the subcommand selects
	// a program, it does not excuse a bad value.
	if _, err := Load(Options{Args: []string{"key", "rotate"}, Env: map[string]string{"USARR_PORT": "0"}}); err == nil ||
		!strings.Contains(err.Error(), "USARR_PORT") {
		t.Errorf("`key rotate` with a bad USARR_PORT = %v, want the port error", err)
	}
}
