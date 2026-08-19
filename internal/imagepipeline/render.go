package imagepipeline

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"strconv"

	// Decoders, registered for their side effect. This list IS the set of
	// encodings UsArr can ingest; anything else is refused by name in decode()
	// rather than skipped, because a cover that silently never appears looks
	// identical to a cover the upstream never had.
	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/webp"

	xdraw "golang.org/x/image/draw"

	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// The pixel half of the pipeline: bytes in, one JPEG per allowlisted width out.
//
// # Why every width is re-encoded, `orig` included
//
// ADR-0050 clause 1 and migration 00008's header are both normative and say the
// same thing: `image_asset.format` is ONE column for a row that has up to seven
// renditions, so it can only describe them all if they share a codec. THERE IS
// NO PASSTHROUGH WIDTH. `orig` here means "the source's own dimensions,
// re-encoded by us", not "the upstream's bytes". Serving the upstream's bytes at
// `orig` would put a second, unrecorded codec behind one row, and `/img` derives
// its Content-Type from that column alone.
//
// # Why the standard library's encoder
//
// ADR-0050 clause 1 names stdlib `image/jpeg` as the base output format, chosen
// because it costs the static binary no dependency. AVIF is DEFERRED with a
// named reopening condition — do not add an encoder for it here.

// jpegQuality is what every rendition is encoded at.
//
// 82 rather than the stdlib default of 75 or a lossless-looking 95. Cover art is
// the one thing on §17.2's grid a user looks AT rather than through, and the
// jump from 75 to 82 is where blocking on flat colour and text stops being
// visible on a poster; the jump from 82 to 95 roughly doubles the file for a
// difference nobody reports. It is a constant and not a setting because a
// per-install quality knob would make two installs' caches incomparable while
// answering a question nobody has asked.
const jpegQuality = 82

// maxSourceBytes and maxSourcePixels bound what decode() will accept.
//
// ⚠️ BOTH ARE NEEDED AND NEITHER IMPLIES THE OTHER. A byte cap alone does not
// bound memory: PNG and WebP are compressed, and a few hundred kilobytes of
// highly-compressible input decodes to an arbitrarily large pixel buffer — the
// decompression-bomb class. A pixel cap alone does not bound the decoder's own
// scratch allocations. maxSourcePixels is checked from the HEADER, before any
// pixel buffer is allocated, which is the only order in which the check does
// anything at all.
//
// 64 megapixels is roughly 8000×8000, about 256 MB as RGBA. Every real cover is
// two orders of magnitude under it; the number is a bomb ceiling, not a quality
// budget.
const (
	maxSourceBytes  = 32 << 20
	maxSourcePixels = 64 << 20
)

// ErrUnsupportedFormat is an HONEST REFUSAL: the bytes decoded to nothing this
// build understands. It is deliberately distinct from a transport failure, so a
// caller can record "we fetched it and cannot use it" as a terminal state rather
// than retrying it forever.
var ErrUnsupportedFormat = errors.New("imagepipeline: unsupported image encoding")

// ErrImageTooLarge is the decompression-bomb refusal. Also terminal: a retry
// fetches the same enormous image.
var ErrImageTooLarge = errors.New("imagepipeline: source image is too large to decode")

// Rendition is one encoded width, ready to hand to imagecache.Put.
type Rendition struct {
	// Width is the allowlist token (imagecache.Width92 … imagecache.WidthOrig),
	// not a pixel count — it is what `?w=` sends and what the path is keyed on.
	Width string

	// Pixels is how wide the encoded image ACTUALLY is, which is not always what
	// Width says: see renderAll's no-upscale rule.
	Pixels int

	// Bytes is the encoded image, in the codec Format names.
	Bytes []byte
}

// rendered is the whole output of a render pass.
type rendered struct {
	// Format is the codec token for `image_asset.format`. It is the same for
	// every rendition by construction — see the file header.
	Format string

	// Width and Height are the SOURCE's dimensions, which is what
	// `image_asset.width`/`height` record.
	Width, Height int

	Renditions []Rendition
}

// decode turns fetched bytes into an image, bounded twice.
//
// The upstream's declared Content-Type is NOT consulted, here or anywhere. It is
// a hint from a server that has no obligation to be right, and migration 00008's
// header is explicit that `image_asset.format` records what UsArr's own encoder
// produced rather than what the origin called it. The decoder registry decides.
func decode(b []byte) (image.Image, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: no bytes", ErrUnsupportedFormat)
	}
	if len(b) > maxSourceBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds the %d-byte ingest limit",
			ErrImageTooLarge, len(b), maxSourceBytes)
	}

	// The header FIRST. DecodeConfig reads dimensions without allocating the
	// pixel buffer, which is the entire defence against a bomb; decoding and
	// then measuring would have already spent the memory.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		// The error is not wrapped verbatim into the caller's message: it can
		// quote arbitrary bytes from a hostile file.
		return nil, fmt.Errorf("%w: the bytes match no registered decoder", ErrUnsupportedFormat)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("%w: %s header declares a %dx%d image",
			ErrUnsupportedFormat, format, cfg.Width, cfg.Height)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxSourcePixels {
		return nil, fmt.Errorf("%w: %dx%d exceeds the %d-pixel decode limit",
			ErrImageTooLarge, cfg.Width, cfg.Height, maxSourcePixels)
	}

	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%w: %s header parsed but the body did not", ErrUnsupportedFormat, format)
	}
	return img, nil
}

// renderAll produces one encoded rendition per allowlisted width.
//
// # The no-upscale rule, and what it does instead of upscaling
//
// A target wider than the source is CLAMPED to the source's width; it is never
// enlarged. Enlarging invents detail, costs bytes and makes `?w=780` on a 400px
// cover look worse than `?w=orig` on the same one.
//
// It is CLAMPED RATHER THAN SKIPPED, and that is the load-bearing half. Skipping
// would leave `?w=780` with no file, and the serving path renders a missing file
// as ErrNotCached — "we have not rendered this yet" — which for a source that
// will never be that wide is a permanent lie that looks like a cold cache. Every
// allowlisted width therefore always has bytes.
//
// Clamped widths are encoded ONCE and shared: on a 400px source, 500, 780 and
// `orig` are the same rendition, so the duplication costs one encode and one
// pixel buffer, not three. It does still cost three FILES — imagecache.Path is
// keyed on the width token, and collapsing them would need a symlink or an
// indirection the reader knows nothing about, which is a worse trade than a few
// duplicated kilobytes on the small minority of covers narrower than 500px.
func renderAll(img image.Image) (rendered, error) {
	src := img.Bounds()
	srcW, srcH := src.Dx(), src.Dy()
	if srcW <= 0 || srcH <= 0 {
		return rendered{}, fmt.Errorf("%w: decoded to a %dx%d image", ErrUnsupportedFormat, srcW, srcH)
	}

	out := rendered{Format: store.ImageFormatJPEG, Width: srcW, Height: srcH}

	// Encoded bytes are cached by TARGET PIXEL WIDTH, so the clamped widths
	// above share one encode.
	byPixels := map[int][]byte{}

	for _, token := range imagecache.Widths() {
		want, err := targetPixels(token, srcW)
		if err != nil {
			return rendered{}, err
		}

		encoded, ok := byPixels[want]
		if !ok {
			scaled := scale(img, want, srcW, srcH)
			encoded, err = encodeJPEG(scaled)
			if err != nil {
				return rendered{}, err
			}
			byPixels[want] = encoded
		}
		out.Renditions = append(out.Renditions, Rendition{Width: token, Pixels: want, Bytes: encoded})
	}
	return out, nil
}

// targetPixels resolves one width token against a source width, applying the
// no-upscale clamp. `orig` is the source's own width — re-encoded, never passed
// through (ADR-0050 clause 1).
func targetPixels(token string, srcW int) (int, error) {
	if token == imagecache.WidthOrig {
		return srcW, nil
	}
	n, err := strconv.Atoi(token)
	if err != nil || n <= 0 {
		// Unreachable while imagecache.Widths() is the source of the tokens,
		// which is exactly why it is an error and not a panic: the day someone
		// adds a non-numeric token to that allowlist, this says so.
		return 0, fmt.Errorf("imagepipeline: width token %q is not a pixel count and is not %q",
			token, imagecache.WidthOrig)
	}
	return min(n, srcW), nil
}

// scale resamples to the target width, preserving aspect ratio.
//
// CatmullRom rather than ApproxBiLinear or NearestNeighbor: this runs ONCE per
// cover at ingest and the result is cached for a year, so the quality/cost trade
// is nothing like the one a per-request resize faces. Nearest-neighbour aliases
// badly on the text that covers are mostly made of.
//
// THE DESTINATION IS PRE-FILLED WITH WHITE AND COMPOSITED WITH Over, WHICH
// MATTERS FOR EXACTLY ONE INPUT CLASS. JPEG has no alpha channel, so a PNG or
// WebP cover with transparency has to be flattened against something. Go's
// encoder resolves an RGBA pixel by dropping alpha, which turns a
// fully-transparent pixel into BLACK — a transparent-background logo would
// arrive as a black rectangle. White is the conventional and less surprising
// backdrop, and for a fully-opaque source (every JPEG, most covers) Over is
// pixel-identical to Src.
func scale(img image.Image, targetW, srcW, srcH int) image.Image {
	targetH := max((srcH*targetW+srcW/2)/srcW, 1)

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

// encodeJPEG is the ONE encoder call in this package, so the quality constant
// cannot be applied to some widths and not others.
func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("imagepipeline: encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
