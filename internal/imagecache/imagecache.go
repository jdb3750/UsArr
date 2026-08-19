// Package imagecache is the on-disk half of ARCHITECTURE.md §4.4's image
// pipeline: where a rendered image lives, what widths may be asked for, and how
// a serving path opens one.
//
// WHY IT IS ITS OWN PACKAGE AND NOT A FUNCTION IN internal/httpapi. The layout
// below has exactly two users — the route that READS a rendered image and the
// fetcher that WRITES one — and they are the two halves of the pipeline. Only
// the reader is built. A path derivation copied into the writer later, rather
// than imported, produces a cache that silently never hits: every write lands
// somewhere the reader does not look, every request 404s, and nothing fails.
// One package is the whole cost of making that impossible.
//
// ⚠️ WHAT IS NOT HERE, so nothing reads more into the package name. There is no
// fetch, no HTTP client, no decoder, no encoder, no downscale and no eviction.
// Those live in internal/imagepipeline, which is the WRITER this package's
// argument above is about — it calls Put and Key rather than deriving either.
//
// ⚠️ THIS PARAGRAPH USED TO SAY "the fetch half of the pipeline is not built"
// AND THAT NOTHING PUT FILES HERE. Both are false as of internal/imagepipeline:
// something writes now. What is still true is the shape — this package owns the
// layout and the allowlist and nothing else — and one thing that is NOT about
// the tree at all: internal/imagepipeline has been exercised only against
// fabricated images, never against a cover from a running service, so an empty
// cache remains the state of every real install until an import wires it up.
package imagecache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// The width allowlist from ARCHITECTURE.md §4.4, verbatim and closed.
//
// ⚠️ IT IS AN ALLOWLIST BECAUSE AN ARBITRARY `?w=` IS A LIVE CVE CLASS, not
// because seven sizes happen to be enough. §4.4 names the shape:
// "arbitrary `?w=` is a cache-poisoning DoS", GHSA-rrr6-mvwg-9pg9. An unbounded
// width parameter lets one caller mint an unbounded number of distinct cache
// entries, each of which costs a decode, a downscale, an encode and a file — so
// the resource the cache exists to protect is the resource the parameter
// consumes. The bound has to be a fixed set and not a range: a range still
// admits every integer inside it.
//
// `orig` IS A RE-ENCODE, NOT A PASSTHROUGH. ADR-0050 clause 1 is normative that
// every stored width, `orig` included, is produced by UsArr's own encoder in the
// codec `image_asset.format` names — so there is no width on this list whose
// bytes came from upstream unchanged, and no width whose Content-Type could
// legitimately be derived from anything but that column.
const (
	Width92   = "92"
	Width154  = "154"
	Width200  = "200"
	Width342  = "342"
	Width500  = "500"
	Width780  = "780"
	WidthOrig = "orig"
)

// DefaultWidth is what a request with no `?w=` asks for.
//
// `orig` and not a small one: an omitted parameter means "the image", and
// serving a 92px thumbnail to a caller who did not choose a size would be a
// silent downgrade that looks like a bad encode. A caller that wants a grid
// thumbnail names one — §13's own budget line is `?w=342`.
const DefaultWidth = WidthOrig

// widths is every legal `?w=` value. Adding a member is the entire cost of a new
// size, and removing one orphans the files already written at it.
var widths = map[string]bool{
	Width92: true, Width154: true, Width200: true, Width342: true,
	Width500: true, Width780: true, WidthOrig: true,
}

// ValidWidth reports whether w is one of §4.4's seven allowed widths.
//
// The empty string is NOT a member: a caller holding "" has not chosen a width,
// and the route substitutes DefaultWidth before it gets here. Admitting "" would
// make "no width was sent" and "a width was sent and it was empty" the same
// cache entry.
func ValidWidth(w string) bool { return widths[w] }

// Widths returns the allowlist, for a caller that renders it — an error message
// naming what it would have accepted, or a test that walks every member.
func Widths() []string {
	return []string{Width92, Width154, Width200, Width342, Width500, Width780, WidthOrig}
}

// keyPattern is the shape of a cache key: `sha256(credential-stripped
// source_url)[:16]` (migration 00005), which is sixteen lowercase hex
// characters.
//
// ⚠️ THIS IS THE FILESYSTEM'S GUARD AND NOT A FORMATTING CHECK. The key arrives
// as a URL path segment and becomes a directory name, so `..`, a separator, a
// NUL or an absolute path in it is a traversal. The pattern admits sixteen
// characters from [0-9a-f] and therefore admits none of them — the safety
// property is that the alphabet excludes every dangerous byte, not that a
// cleaning step removes them afterwards. internal/store.ValidImageCacheKey
// applies the same shape before the key reaches SQL; both exist because a check
// in one place is a check that the next caller skips.
var keyPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ValidKey reports whether s is shaped like an `image_asset.cache_key`.
func ValidKey(s string) bool { return keyPattern.MatchString(s) }

// ErrNotCached is returned when the asset exists but its bytes at that width do
// not. It is DISTINCT from an unreadable cache: "we have not rendered this yet"
// is the ordinary state of §4.4.1's cold start and of every install today, and a
// caller renders it differently from a broken disk.
var ErrNotCached = errors.New("imagecache: no bytes cached at this width")

// Cache is a directory of rendered images. Root is Config.ImageCacheDir().
type Cache struct{ root string }

// New wraps a cache directory. An empty root is legal and means the process has
// no image cache configured; every lookup then misses with ErrNotCached rather
// than reading from the working directory, which is what an unchecked empty root
// joined onto a relative path would do.
func New(root string) *Cache { return &Cache{root: root} }

// Root is the directory this cache reads from, empty when none is configured.
func (c *Cache) Root() string { return c.root }

// Path is where the bytes for one asset at one width live:
//
//	<root>/<key[:2]>/<key>/<width>
//
// THE FILE HAS NO EXTENSION, AND THAT IS DELIBERATE. `image_asset.format` is the
// authority on a cached image's encoding — it is the column the Content-Type is
// derived from — and a `.jpg` on disk would be a second, independent claim about
// the same fact, free to disagree with the first. A filename that cannot state
// an encoding cannot state a wrong one. It also means a re-encode under a new
// codec (ADR-0050 names the condition that reopens AVIF) rewrites the same path
// instead of leaving a stale sibling.
//
// The two-character shard keeps one directory from holding an entry per asset:
// §17.8's flagship topology is tens of thousands, and a flat directory of that
// size is slow to enumerate on every filesystem and pathological on some.
//
// Path assumes a validated key and width. It is unexported precisely so the
// validation cannot be skipped by a caller that reaches for the path first;
// Open is the entry point and it validates.
func (c *Cache) path(key, width string) string {
	return filepath.Join(c.root, key[:2], key, width)
}

// Open returns the cached bytes for one asset at one width, and the file's
// metadata for a conditional-request path.
//
// The caller closes the file. A miss — including "there is no cache directory at
// all" — is ErrNotCached and never an error naming a filesystem path: the path
// embeds the host's directory layout, which has no business in an HTTP response
// and, on this route, would be reflected back at whoever guessed the key.
func (c *Cache) Open(key, width string) (*os.File, fs.FileInfo, error) {
	if c.root == "" || !ValidKey(key) || !ValidWidth(width) {
		return nil, nil, ErrNotCached
	}
	f, err := os.Open(c.path(key, width))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, ErrNotCached
		}
		// A permission problem or a broken disk is NOT a miss: reporting it as
		// one would turn "the cache directory is mode 0000" into an install
		// where every image is quietly absent and nothing says why. The path is
		// deliberately not in the message.
		return nil, nil, fmt.Errorf("imagecache: open cached image: %w", errNoPath(err))
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("imagecache: stat cached image: %w", errNoPath(err))
	}
	// A directory where a file should be is a corrupt cache, not a hit:
	// ServeContent would happily send its (empty) contents as an image.
	if info.IsDir() {
		_ = f.Close()
		return nil, nil, ErrNotCached
	}
	return f, info, nil
}

// errNoPath strips the filename from an *fs.PathError, keeping the underlying
// cause. os errors embed the path they failed on, and this package's errors
// travel into log lines and, wrapped, toward an HTTP handler.
func errNoPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s: %w", pe.Op, pe.Err)
	}
	return err
}
