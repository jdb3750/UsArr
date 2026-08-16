package crypto

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// HKDF info labels. docs/reference/security.md §1.3.
//
// The labels are distinct so the credential KEK and the URL-signing key can be
// rotated independently. An earlier design said only "derived", which left the
// two purposes sharing one secret — meaning rotating the vault key would
// silently invalidate every outstanding stream URL.
const (
	infoKEK = "usarr/kek/v1"

	// infoStreamToken derives the key that signs UsArr's own stream URLs. It is
	// derived from the first commit even though no surface consumes it yet
	// (v0.4), because deriving it later would mean deciding the label later.
	//
	// #nosec G101 -- an HKDF info label is a domain-separation constant, not a
	// credential. It is public by design and appears in the spec.
	infoStreamToken = "usarr/stream-token/v1"

	// infoClientCredential derives the server key for client_credential HMACs.
	//
	// docs/reference/security.md §1.3 names only the two labels above;
	// docs/reference/schema.md §9 specifies key_hash as
	// HMAC-SHA256(server_key, full_key) without saying how server_key is
	// derived. This label follows the same scheme. INFERENCE, 2026-08-16 — if
	// the docs later name a different label, this is a breaking change that
	// invalidates every issued API key, so it needs a migration that reissues
	// them, not an edit.
	//
	// #nosec G101 -- an HKDF info label, not a credential. See infoStreamToken.
	infoClientCredential = "usarr/client-credential/v1"
)

// DerivedKeyLen is the length of every key derived here.
const DerivedKeyLen = 32

// DeriveKEK derives the key-encryption key that wraps every per-record DEK.
//
// salt is the per-install random value stored beside the master key; it is not
// secret. secret is the master key.
func DeriveKEK(secret, salt []byte) ([]byte, error) {
	return derive(secret, salt, infoKEK)
}

// DeriveStreamTokenKey derives the key that signs stream URLs on UsArr's own
// northbound surfaces.
func DeriveStreamTokenKey(secret, salt []byte) ([]byte, error) {
	return derive(secret, salt, infoStreamToken)
}

// DeriveClientCredentialKey derives the server key used to HMAC per-app API
// keys. See infoClientCredential for the caveat on this label.
func DeriveClientCredentialKey(secret, salt []byte) ([]byte, error) {
	return derive(secret, salt, infoClientCredential)
}

func derive(secret, salt []byte, info string) ([]byte, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("crypto: derive %s: empty secret", info)
	}
	if len(salt) == 0 {
		return nil, fmt.Errorf("crypto: derive %s: empty salt", info)
	}
	key, err := hkdf.Key(sha256.New, secret, salt, info, DerivedKeyLen)
	if err != nil {
		return nil, fmt.Errorf("crypto: derive %s: %w", info, err)
	}
	return key, nil
}

// AAD identifies where a ciphertext sits. It is bound into the AEAD as
// additional authenticated data, so moving a ciphertext to another row, column
// or host makes it fail to open rather than silently succeed.
//
// docs/reference/security.md §1.2:
//
//	AAD = table_name || ":" || column_name || ":" || primary_key || ":" || sha256(normalised host:port)
//
// Without this, AES-256-GCM authenticates the ciphertext but not its location.
// Anyone with database write access — a restored backup, a NAS share, an
// operator, a SQL-injection-equivalent bug — could copy the Radarr row's
// ciphertext into a service_instance row whose base_url they control, and UsArr
// would decrypt it and transmit it to them.
type AAD struct {
	// Table is the table name, e.g. "service_instance".
	Table string
	// Column is the column name, e.g. "api_key_enc".
	Column string
	// PrimaryKey is the row's primary key rendered as text.
	PrimaryKey string
	// HostPort is the normalised host:port of the instance the credential is
	// for, as returned by NormalizeHostPort.
	HostPort string
}

// Bytes renders the AAD.
//
// The sha256 of the normalised host:port is rendered as lowercase hex rather
// than raw bytes. The document says "sha256(...)" without fixing an encoding;
// hex keeps the whole AAD printable, which matters because this value appears
// in the audit trail when a decryption fails.
func (a AAD) Bytes() []byte {
	sum := sha256.Sum256([]byte(a.HostPort))
	var b strings.Builder
	b.Grow(len(a.Table) + len(a.Column) + len(a.PrimaryKey) + 3 + 2*sha256.Size)
	b.WriteString(a.Table)
	b.WriteByte(':')
	b.WriteString(a.Column)
	b.WriteByte(':')
	b.WriteString(a.PrimaryKey)
	b.WriteByte(':')
	const hexDigits = "0123456789abcdef"
	for _, c := range sum {
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0f])
	}
	return []byte(b.String())
}

// ServiceInstanceAAD builds the AAD for service_instance.api_key_enc, the only
// encrypted column today.
func ServiceInstanceAAD(id int64, baseURL string) (AAD, error) {
	hp, err := NormalizeHostPort(baseURL)
	if err != nil {
		return AAD{}, err
	}
	return AAD{
		Table:      "service_instance",
		Column:     "api_key_enc",
		PrimaryKey: strconv.FormatInt(id, 10),
		HostPort:   hp,
	}, nil
}

// defaultPorts fills in the port when a base URL omits it, so that
// "http://nas" and "http://nas:80" produce the same AAD and a credential does
// not become unreadable because someone tidied up the URL.
var defaultPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

// NormalizeHostPort reduces a service base URL to the canonical host:port that
// goes into the AAD.
//
// The normalisation is deliberate, because every rule here decides whether an
// edit to base_url invalidates the stored credential:
//
//   - The scheme is lowercased and used only to supply a default port. It is
//     NOT part of the output, because the documented formula is "host:port".
//     CONSEQUENCE, flagged: http://nas:443 and https://nas:443 produce the same
//     AAD, so a same-port scheme downgrade is not caught cryptographically.
//     docs/reference/security.md §1.6 blocks that edit a second way — changing
//     the scheme, host or port requires re-entering the key, and a connection
//     test against a modified base_url uses only the key typed into the form.
//   - The host is lowercased and a trailing root dot is stripped, because DNS
//     names are case-insensitive and "nas." and "nas" are the same host.
//   - An IP literal is reduced to its canonical form (netip), so ::1, 0:0::1
//     and [::1] agree, and an IPv4-mapped IPv6 address is unmapped.
//   - The port is always present in the output, taken from the URL or from the
//     scheme's default.
//   - Path, query, fragment and userinfo are discarded: url_base is a separate
//     column and moving a service to a different sub-path does not move the
//     credential.
func NormalizeHostPort(rawURL string) (string, error) {
	v := strings.TrimSpace(rawURL)
	if v == "" {
		return "", fmt.Errorf("crypto: normalise host:port: empty URL")
	}

	u, err := url.Parse(v)
	if err != nil {
		return "", fmt.Errorf("crypto: normalise host:port: parse %q: %w", rawURL, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("crypto: normalise host:port: %q has scheme %q; want http or https", rawURL, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("crypto: normalise host:port: %q has no host", rawURL)
	}

	host := u.Hostname()
	port := u.Port()
	if host == "" {
		return "", fmt.Errorf("crypto: normalise host:port: %q has no host", rawURL)
	}
	if port == "" {
		port = defaultPorts[scheme]
	} else {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("crypto: normalise host:port: %q has invalid port %q", rawURL, port)
		}
		port = strconv.Itoa(n)
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.Unmap().String()
	}

	return net.JoinHostPort(host, port), nil
}
