package imagecache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// The WRITE half of this package, and the reason the package doc says there are
// exactly two users of the layout.
//
// Everything here exists so the fetcher never has to know what the layout IS.
// Put takes a key, a width and bytes; Key takes a source URL and answers the
// only string the reader will ever look under. A fetcher that derived either by
// hand would produce a cache that silently never hits — every write landing
// somewhere Open does not look, every request 404-ing, and nothing failing.

// ErrCredentialInURL is returned by Key for a source URL that still carries a
// credential.
//
// ⚠️ IT IS A REFUSAL AND NOT A STRIP, and that is the whole point of putting the
// check HERE rather than in the caller. security.md §5 requires that no row be
// written whose `source_url` carries a credential parameter; deriving the key
// and enforcing that rule in one function means there is no way to obtain a
// cache key for a credentialed URL at all, so the rule cannot be skipped by a
// caller that simply forgot to ask. Stripping silently would be worse than
// either: it would make a credential in a source URL — which is a BUG in
// whatever constructed it — invisible, and it would mint a key for a URL nobody
// audited.
var ErrCredentialInURL = errors.New("imagecache: source url carries a credential")

// Key derives the `image_asset.cache_key` for one source URL.
//
// THE DERIVATION IS `sha256(credential-stripped source_url)[:16]`, lowercase
// hex, exactly as migration 00005 spells the column and as ARCHITECTURE.md §4.4
// spells the route. Concretely: SHA-256 over the UTF-8 bytes of the URL string
// as given, hex-encoded, truncated to the first 16 characters — the first 8
// bytes of the digest. Nothing is normalised first: two spellings of one URL are
// two rows, which is the honest outcome for a UNIQUE column whose uniqueness is
// over the string.
//
// WHY IT IS DERIVED FROM THE URL AND NOT THE BYTES. security.md §5 names the
// consequence directly — because the key is a function of the URL, rotating a
// provider key would invalidate the entire image cache if the credential were
// part of the input. It is also why the caller must hand over the STRIPPED URL:
// the same URL with and without a rotated key must hash the same.
//
// It returns ErrCredentialInURL rather than a key when the URL still carries
// one, so the refusal reaches the caller instead of a silently different key.
func Key(sourceURL string) (string, error) {
	if err := CheckSourceURL(sourceURL); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(sourceURL))
	// 8 bytes → 16 hex characters, which is exactly what keyPattern admits.
	return hex.EncodeToString(sum[:8]), nil
}

// CheckSourceURL reports whether a URL is fit to be stored in
// `image_asset.source_url`: it must parse, and it must carry no credential.
//
// WHAT COUNTS AS A CREDENTIAL IS `internal/ssrf`'s TO SAY AND NOT THIS
// FUNCTION'S. ssrf.IsCredentialParam consults the ONE deny-list
// (internal/ssrf/redact.go's credentialParams); a shorter list restated here
// would be the second deny-list, and the one that drifts is the one that leaks.
// Userinfo is refused outright — `https://user:pass@host/…` needs no list.
//
// ⚠️ WHAT IT DOES NOT COVER, stated so nobody reads more into it. A credential
// hidden in a PATH SEGMENT is not refused. internal/ssrf redacts those from
// logs, but only through a deliberately lossy heuristic (looksLikeCredential,
// biased towards missing a passkey rather than eating a real path segment), and
// a heuristic that fires wrongly here would refuse a legitimate cover. The
// obligation security.md §5 actually states is over the credential PARAMETER,
// and that is what this enforces.
func CheckSourceURL(sourceURL string) error {
	if sourceURL == "" {
		return fmt.Errorf("%w: empty source url", ErrCredentialInURL)
	}
	u, err := url.Parse(sourceURL)
	if err != nil {
		// "It did not parse" is not evidence that it holds no secret, so an
		// unparseable URL is refused rather than passed through. The URL itself
		// is NOT in the message: it is the thing suspected of carrying a
		// credential.
		return fmt.Errorf("%w: source url does not parse", ErrCredentialInURL)
	}
	if u.User != nil {
		return fmt.Errorf("%w: source url carries userinfo", ErrCredentialInURL)
	}
	for name := range u.Query() {
		if ssrf.IsCredentialParam(name) {
			// The NAME is safe to report — it is a member of a public
			// deny-list. The value never is.
			return fmt.Errorf("%w: query parameter %q", ErrCredentialInURL, name)
		}
	}
	return nil
}

// dirPerm and filePerm are what the cache creates. 0o700 / 0o600: the cache
// holds artwork whose entitlement is computed elsewhere, on a single-user
// self-hosted box where the process owner is the only principal that should read
// it. There is no reason for group or other to see any of it.
const (
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// ErrNoCacheDir is returned by Put when no cache directory is configured. It is
// distinct from a write failure: "this process has no image cache" is a
// configuration state the caller renders, not a broken disk.
var ErrNoCacheDir = errors.New("imagecache: no cache directory is configured")

// Put writes the rendered bytes for one asset at one width.
//
// IT IS ATOMIC PER FILE, via a temporary file in the same directory and a
// rename. The reader is `Open`, which either finds a whole file or finds
// nothing; a torn write would otherwise be served to a browser as a truncated
// image with a 200, and cached for a year (imageCacheControl is
// `immutable, max-age=31536000`). A partial file that is never renamed is a
// miss, which is the recoverable failure.
//
// The key and the width are validated here for Open's reason and not for
// tidiness: the key becomes a DIRECTORY NAME, so `..`, a separator or an
// absolute path in it is a traversal, and the write side is the side where a
// traversal creates something.
func (c *Cache) Put(key, width string, b []byte) error {
	if c.root == "" {
		return ErrNoCacheDir
	}
	if !ValidKey(key) {
		return fmt.Errorf("imagecache: refusing to write under a malformed cache key")
	}
	if !ValidWidth(width) {
		return fmt.Errorf("imagecache: %q is not one of the widths this cache stores", width)
	}

	// path() and nothing hand-written: see the package doc. The directory is
	// derived from the same call the reader uses, so the two cannot drift.
	dst := c.path(key, width)
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("imagecache: create cache directory: %w", errNoPath(err))
	}

	tmp, err := os.CreateTemp(dir, ".tmp-"+width+"-*")
	if err != nil {
		return fmt.Errorf("imagecache: create temporary file: %w", errNoPath(err))
	}
	tmpName := tmp.Name()
	// Best-effort cleanup on every failure path below. A rename makes it a
	// no-op; a failure leaves nothing behind for the next run to trip over.
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(filePerm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("imagecache: set cache file mode: %w", errNoPath(err))
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("imagecache: write cached image: %w", errNoPath(err))
	}
	// Sync before rename. Without it the rename can be durable while the bytes
	// are not, which after a crash is a file the reader finds and serves as a
	// zero-length image — the exact outcome the temp-and-rename exists to avoid.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("imagecache: sync cached image: %w", errNoPath(err))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("imagecache: close cached image: %w", errNoPath(err))
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("imagecache: publish cached image: %w", errNoPath(err))
	}
	return nil
}
