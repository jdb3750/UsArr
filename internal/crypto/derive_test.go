package crypto

import (
	"bytes"
	"strings"
	"testing"
)

// The three derived keys must differ. Sharing one secret between the vault KEK
// and the URL-signing key would mean rotating the vault silently invalidated
// every outstanding stream URL — the exact failure the distinct info labels in
// docs/reference/security.md §1.3 exist to prevent.
func TestDerivedKeysAreDistinct(t *testing.T) {
	secret := bytes.Repeat([]byte{0x01}, 32)
	salt := bytes.Repeat([]byte{0x02}, 32)

	kek, err := DeriveKEK(secret, salt)
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	stream, err := DeriveStreamTokenKey(secret, salt)
	if err != nil {
		t.Fatalf("DeriveStreamTokenKey: %v", err)
	}
	client, err := DeriveClientCredentialKey(secret, salt)
	if err != nil {
		t.Fatalf("DeriveClientCredentialKey: %v", err)
	}

	for _, k := range [][]byte{kek, stream, client} {
		if len(k) != DerivedKeyLen {
			t.Fatalf("derived key length = %d, want %d", len(k), DerivedKeyLen)
		}
	}
	if bytes.Equal(kek, stream) || bytes.Equal(kek, client) || bytes.Equal(stream, client) {
		t.Fatal("derived keys collide; the info labels are not separating them")
	}
}

// Derivation is deterministic: the same secret and salt must always give the
// same KEK, or every stored credential becomes unreadable on restart.
func TestDeriveIsDeterministic(t *testing.T) {
	secret := bytes.Repeat([]byte{0x0A}, 32)
	salt := bytes.Repeat([]byte{0x0B}, 32)

	a, err := DeriveKEK(secret, salt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := DeriveKEK(secret, salt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("DeriveKEK is not deterministic")
	}

	// A different salt gives a different KEK.
	c, err := DeriveKEK(secret, bytes.Repeat([]byte{0x0C}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, c) {
		t.Fatal("changing the salt did not change the KEK")
	}
}

func TestDeriveRejectsEmptyInputs(t *testing.T) {
	if _, err := DeriveKEK(nil, bytes.Repeat([]byte{1}, 32)); err == nil {
		t.Error("DeriveKEK accepted an empty secret")
	}
	if _, err := DeriveKEK(bytes.Repeat([]byte{1}, 32), nil); err == nil {
		t.Error("DeriveKEK accepted an empty salt")
	}
}

// The host:port normalisation decides whether an edit to base_url invalidates
// the stored credential, so every rule in it is deliberate and pinned here.
func TestNormalizeHostPort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "explicit port", in: "http://radarr.lan:7878", want: "radarr.lan:7878"},
		{name: "http default port filled in", in: "http://radarr.lan", want: "radarr.lan:80"},
		{name: "https default port filled in", in: "https://radarr.lan", want: "radarr.lan:443"},
		{name: "explicit default port matches the implicit one", in: "http://radarr.lan:80", want: "radarr.lan:80"},
		{name: "scheme case folded", in: "HTTP://radarr.lan:7878", want: "radarr.lan:7878"},
		{name: "host case folded", in: "http://Radarr.LAN:7878", want: "radarr.lan:7878"},
		{name: "root dot stripped", in: "http://radarr.lan.:7878", want: "radarr.lan:7878"},
		{name: "path discarded", in: "http://radarr.lan:7878/radarr", want: "radarr.lan:7878"},
		{name: "query and fragment discarded", in: "http://radarr.lan:7878/x?y=1#z", want: "radarr.lan:7878"},
		{name: "userinfo discarded", in: "http://user:pass@radarr.lan:7878", want: "radarr.lan:7878"},
		{name: "surrounding whitespace", in: "  http://radarr.lan:7878  ", want: "radarr.lan:7878"},
		{name: "ipv4 literal", in: "http://192.168.1.10:7878", want: "192.168.1.10:7878"},
		{name: "ipv6 literal", in: "http://[2001:db8::1]:7878", want: "[2001:db8::1]:7878"},
		{name: "ipv6 canonicalised", in: "http://[2001:0DB8:0000::0001]:7878", want: "[2001:db8::1]:7878"},
		{name: "ipv6 loopback", in: "http://[::1]:8989", want: "[::1]:8989"},
		{name: "ipv4-mapped ipv6 unmapped", in: "http://[::ffff:192.168.1.10]:7878", want: "192.168.1.10:7878"},
		{name: "port leading zeros normalised", in: "http://radarr.lan:07878", want: "radarr.lan:7878"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeHostPort(tc.in)
			if err != nil {
				t.Fatalf("NormalizeHostPort(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeHostPort(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// KNOWN GAP, pinned so it is a decision rather than a surprise.
//
// TestSchemeIsBoundIntoTheAAD is the regression test for CRYPTO-01.
//
// This test used to be TestSchemeIsNotInTheAAD and asserted the opposite: it
// pinned the gap in place. The AAD bound only host:port, so editing
// https://nas.lan:443 to http://nas.lan:443 left the AAD unchanged, the envelope
// opened, and UsArr would send a full-admin X-Api-Key over cleartext to a host
// an attacker can now MITM. security.md §1.6 is normative that a SCHEME change
// invalidates the credential, and §1.2's argument is that the cryptographic
// layer must hold when the application layer is bypassed — so the re-entry rule
// could not be the sole control against the very attack it backs up.
func TestSchemeIsBoundIntoTheAAD(t *testing.T) {
	// The same-port downgrade is the case that used to slip through.
	plain, err := NormalizeOrigin("http://nas.lan:443")
	if err != nil {
		t.Fatal(err)
	}
	secure, err := NormalizeOrigin("https://nas.lan:443")
	if err != nil {
		t.Fatal(err)
	}
	if plain == secure {
		t.Fatalf("http:// and https:// on the same port normalise identically (%q); "+
			"a same-port scheme downgrade is not caught cryptographically", plain)
	}

	// And it must reach the AAD proper, not just the normaliser.
	aPlain, err := ServiceInstanceAAD(7, "http://nas.lan:443")
	if err != nil {
		t.Fatal(err)
	}
	aSecure, err := ServiceInstanceAAD(7, "https://nas.lan:443")
	if err != nil {
		t.Fatal(err)
	}
	if string(aPlain.Bytes()) == string(aSecure.Bytes()) {
		t.Error("ServiceInstanceAAD ignores the scheme; an https->http edit would still decrypt")
	}

	// The usual case still differs too, via the default ports.
	a, err := NormalizeOrigin("http://nas.lan")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NormalizeOrigin("https://nas.lan")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("http:// and https:// with implicit ports must differ")
	}

	// NormalizeHostPort keeps its own contract: a bare host:port, because
	// ssrf.Options.AllowedHostPort splits it with net.SplitHostPort.
	hp, err := NormalizeHostPort("https://nas.lan:8989")
	if err != nil {
		t.Fatal(err)
	}
	if hp != "nas.lan:8989" {
		t.Errorf("NormalizeHostPort = %q, want a bare host:port", hp)
	}
}

// NormalizeOrigin must reject everything NormalizeHostPort rejects: it is the
// AAD's input, so a URL that slips through here is a credential bound to junk.
func TestNormalizeOriginRejectsWhatHostPortRejects(t *testing.T) {
	for _, in := range []string{"", "radarr.lan:7878", "http://", "file:///etc/passwd", "http://radarr.lan:0"} {
		if got, err := NormalizeOrigin(in); err == nil {
			t.Errorf("NormalizeOrigin(%q) = %q, want an error", in, got)
		}
	}
}

func TestNormalizeHostPortRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "no scheme", in: "radarr.lan:7878"},
		{name: "no host", in: "http://"},
		{name: "file scheme", in: "file:///etc/passwd"},
		{name: "gopher scheme", in: "gopher://radarr.lan"},
		{name: "port zero", in: "http://radarr.lan:0"},
		{name: "port too large", in: "http://radarr.lan:70000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := NormalizeHostPort(tc.in); err == nil {
				t.Errorf("NormalizeHostPort(%q) = %q, want an error", tc.in, got)
			}
		})
	}
}

// The AAD renders as table:column:pk:sha256hex, and the digest is a stable
// lowercase hex string.
func TestAADBytes(t *testing.T) {
	aad := AAD{Table: "service_instance", Column: "api_key_enc", PrimaryKey: "42", Origin: "https://radarr.lan:7878"}
	got := string(aad.Bytes())

	parts := strings.Split(got, ":")
	if len(parts) != 4 {
		t.Fatalf("AAD %q has %d colon-separated parts, want 4", got, len(parts))
	}
	if parts[0] != "service_instance" || parts[1] != "api_key_enc" || parts[2] != "42" {
		t.Errorf("AAD prefix = %q", strings.Join(parts[:3], ":"))
	}
	if len(parts[3]) != 64 {
		t.Errorf("AAD digest is %d chars, want 64 hex chars", len(parts[3]))
	}
	if strings.ToLower(parts[3]) != parts[3] {
		t.Errorf("AAD digest %q is not lowercase hex", parts[3])
	}
	// The raw host:port must not appear: only its digest does.
	if strings.Contains(got, "radarr.lan") {
		t.Error("AAD contains the host in the clear; the formula hashes it")
	}
	// Deterministic.
	if string(aad.Bytes()) != got {
		t.Error("AAD.Bytes() is not deterministic")
	}
}
