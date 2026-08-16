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
//	AAD = table_name || ":" || column_name || ":" || primary_key || ":" || sha256(normalised scheme://host:port)
//
// Without this, AES-256-GCM authenticates the ciphertext but not its location.
// Anyone with database write access — a restored backup, a NAS share, an
// operator, a SQL-injection-equivalent bug — could copy the Radarr row's
// ciphertext into a service_instance row whose base_url they control, and UsArr
// would decrypt it and transmit it to them.
//
// The bound value is the full ORIGIN, scheme included, not just host:port. It
// used to be host:port only, which left one edit uncaught by the cryptographic
// layer: flipping `https://nas:443` to `http://nas:443` changed nothing in the
// AAD, so the envelope still opened and UsArr would send a full-admin X-Api-Key
// over plaintext to a host the attacker can now MITM. security.md §1.6 is
// normative that a SCHEME, host or port change invalidates the credential, and
// §1.2's whole argument is that the cryptographic layer has to hold when the
// application layer is bypassed — so the application-layer re-entry rule could
// not be the only control against the one attack it was written to back up.
type AAD struct {
	// Table is the table name, e.g. "service_instance".
	Table string
	// Column is the column name, e.g. "api_key_enc".
	Column string
	// PrimaryKey is the row's primary key rendered as text.
	PrimaryKey string
	// Origin is the normalised scheme://host:port of the instance the credential
	// is for, as returned by NormalizeOrigin.
	Origin string
}

// Bytes renders the AAD.
//
// The sha256 of the normalised origin is rendered as lowercase hex rather than
// raw bytes. The document says "sha256(...)" without fixing an encoding; hex
// keeps the whole AAD printable, which matters because this value appears in the
// audit trail when a decryption fails.
func (a AAD) Bytes() []byte {
	sum := sha256.Sum256([]byte(a.Origin))
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
	origin, err := NormalizeOrigin(baseURL)
	if err != nil {
		return AAD{}, err
	}
	return AAD{
		Table:      "service_instance",
		Column:     "api_key_enc",
		PrimaryKey: strconv.FormatInt(id, 10),
		Origin:     origin,
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
//   - The scheme is lowercased and used only to supply a default port. It is NOT
//     part of THIS function's output, which is the value internal/ssrf wants for
//     Options.AllowedHostPort — that field is split with net.SplitHostPort and
//     must stay a bare host:port.
//     The AAD does not use this function. It uses NormalizeOrigin, which keeps
//     the scheme, so that http://nas:443 and https://nas:443 produce DIFFERENT
//     AADs and a same-port scheme downgrade is caught cryptographically rather
//     than only by security.md §1.6's re-entry rule. Anything comparing two base
//     URLs to decide whether the stored credential is still valid must use
//     NormalizeOrigin, not this: the two have to agree or a legal edit produces
//     a credential that can never be opened again.
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

// NormalizeOrigin reduces a service base URL to the canonical
// "scheme://host:port" that goes into the AAD.
//
// It is NormalizeHostPort plus the lowercased scheme, and it exists because the
// AAD must bind the scheme: without it, editing https://nas:443 to
// http://nas:443 leaves the AAD unchanged, the envelope opens, and UsArr sends a
// full-admin API key in cleartext to a host that can now be MITM'd. That is the
// exact §1.2 threat ("a restored backup, a NAS share, an operator") which the
// application-layer re-entry rule is only supposed to be redundant with.
//
// Every comparison that decides "is the stored credential still valid for this
// base URL" must use THIS function, not NormalizeHostPort. If a caller decides
// re-entry is unnecessary using host:port while the AAD is built from the
// origin, a scheme-only edit silently produces an unopenable credential.
func NormalizeOrigin(rawURL string) (string, error) {
	hostPort, err := NormalizeHostPort(rawURL)
	if err != nil {
		return "", err
	}
	// NormalizeHostPort has already validated that the scheme is http or https.
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("crypto: normalise origin: parse %q: %w", rawURL, err)
	}
	return strings.ToLower(u.Scheme) + "://" + hostPort, nil
}
