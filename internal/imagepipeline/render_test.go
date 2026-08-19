package imagepipeline

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strconv"
	"testing"

	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// fixtureImage builds a cover-shaped test image with STRUCTURE in it — a
// diagonal gradient plus a hard-edged block — rather than a flat colour.
//
// The structure is load-bearing. A flat image survives every resampler, a
// nearest-neighbour one included, so a test built on one would pass against a
// pipeline that did no real work. The gradient means a downscale that dropped
// pixels instead of averaging them produces measurably different output.
func fixtureImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{
				R: uint8((x * 255) / max(w-1, 1)),
				G: uint8((y * 255) / max(h-1, 1)),
				B: uint8((x ^ y) & 0xff),
				A: 255,
			}
			if x > w/3 && x < 2*w/3 && y > h/4 && y < h/2 {
				c = color.RGBA{R: 10, G: 200, B: 40, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEGFixture(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return buf.Bytes()
}

func encodeGIFFixture(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return buf.Bytes()
}

// TestRenderAllProducesEveryAllowlistedWidthAsJPEG is the core assertion of the
// pixel half, and it fails against a stub on four independent counts: the count
// of renditions, the codec each one decodes as, the pixel width of each, and the
// preserved aspect ratio.
//
// The JPEG re-decode is what makes "encoded" mean something. A rendition holding
// the SOURCE's PNG bytes would satisfy a length check and fail here — which is
// precisely the passthrough ADR-0050 clause 1 forbids.
func TestRenderAllProducesEveryAllowlistedWidthAsJPEG(t *testing.T) {
	const srcW, srcH = 800, 1200
	img, err := decode(encodePNG(t, fixtureImage(srcW, srcH)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	out, err := renderAll(img)
	if err != nil {
		t.Fatalf("renderAll: %v", err)
	}

	if out.Format != store.ImageFormatJPEG {
		t.Errorf("rendered format %q; ADR-0050 clause 1 names stdlib JPEG as the base output format", out.Format)
	}
	if !store.ValidImageFormat(out.Format) {
		t.Errorf("rendered format %q is outside image_asset.format's vocabulary", out.Format)
	}
	if out.Width != srcW || out.Height != srcH {
		t.Errorf("rendered reports the source as %dx%d, want %dx%d", out.Width, out.Height, srcW, srcH)
	}

	want := imagecache.Widths()
	if len(out.Renditions) != len(want) {
		t.Fatalf("got %d renditions, want %d — §4.4 stores every allowlisted width",
			len(out.Renditions), len(want))
	}

	seen := map[string]bool{}
	for i, r := range out.Renditions {
		if r.Width != want[i] {
			t.Errorf("rendition %d is width %q, want %q", i, r.Width, want[i])
		}
		seen[r.Width] = true

		if len(r.Bytes) == 0 {
			t.Errorf("w=%s carries no bytes", r.Width)
			continue
		}

		// Decoded, not sniffed. This is what proves the bytes were re-encoded by
		// US and are not the source's PNG passed through.
		cfg, format, derr := image.DecodeConfig(bytes.NewReader(r.Bytes))
		if derr != nil {
			t.Errorf("w=%s did not decode: %v", r.Width, derr)
			continue
		}
		if format != "jpeg" {
			t.Errorf("w=%s decoded as %q, not jpeg — there is NO passthrough width (ADR-0050 clause 1, "+
				"migration 00008), because image_asset.format is one column for all seven", r.Width, format)
		}

		wantW := srcW
		if r.Width != imagecache.WidthOrig {
			n, _ := strconv.Atoi(r.Width)
			wantW = n
		}
		if cfg.Width != wantW {
			t.Errorf("w=%s encoded %d pixels wide, want %d", r.Width, cfg.Width, wantW)
		}
		if cfg.Width != r.Pixels {
			t.Errorf("w=%s reports %d pixels but encoded %d", r.Width, r.Pixels, cfg.Width)
		}

		// Aspect ratio, within one pixel of rounding.
		wantH := (srcH*wantW + srcW/2) / srcW
		if cfg.Height < wantH-1 || cfg.Height > wantH+1 {
			t.Errorf("w=%s is %dx%d, which is not the source's 2:3 — want height ~%d",
				r.Width, cfg.Width, cfg.Height, wantH)
		}
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("no rendition for allowlisted width %q; GET /img?w=%s would answer not_cached forever", w, w)
		}
	}

	// A smaller width must be smaller on disk. A renderer that encoded the same
	// full-size image seven times would pass every check above.
	var w92, worig int
	for _, r := range out.Renditions {
		switch r.Width {
		case imagecache.Width92:
			w92 = len(r.Bytes)
		case imagecache.WidthOrig:
			worig = len(r.Bytes)
		}
	}
	if w92 >= worig {
		t.Errorf("w=92 is %d bytes and w=orig is %d; the downscale did not happen", w92, worig)
	}
}

// TestRenderAllNeverUpscalesAndNeverSkips pins BOTH halves of the no-upscale
// rule. It fails against a renderer that enlarges (the width assertions) and
// against one that skips (the count assertion) — two opposite bugs, one test.
func TestRenderAllNeverUpscalesAndNeverSkips(t *testing.T) {
	const srcW, srcH = 120, 180
	img, err := decode(encodePNG(t, fixtureImage(srcW, srcH)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := renderAll(img)
	if err != nil {
		t.Fatalf("renderAll: %v", err)
	}

	if len(out.Renditions) != len(imagecache.Widths()) {
		t.Fatalf("a narrow source produced %d renditions, want all %d — a skipped width leaves "+
			"GET /img?w=780 answering not_cached, which reads as a cold cache forever",
			len(out.Renditions), len(imagecache.Widths()))
	}

	for _, r := range out.Renditions {
		cfg, _, derr := image.DecodeConfig(bytes.NewReader(r.Bytes))
		if derr != nil {
			t.Fatalf("w=%s did not decode: %v", r.Width, derr)
		}
		if cfg.Width > srcW {
			t.Errorf("w=%s was upscaled to %d pixels from a %d-pixel source", r.Width, cfg.Width, srcW)
		}
		wantW := srcW
		if r.Width == imagecache.Width92 {
			wantW = 92 // the one target genuinely narrower than the source
		}
		if cfg.Width != wantW {
			t.Errorf("w=%s is %d pixels wide, want %d (clamped, not skipped, not enlarged)",
				r.Width, cfg.Width, wantW)
		}
	}

	// The clamped widths must share ONE encode, not repeat it — the sharing is
	// what keeps the clamp from costing six redundant encodes.
	byWidth := map[string]string{}
	for _, r := range out.Renditions {
		byWidth[r.Width] = string(r.Bytes)
	}
	for _, w := range []string{imagecache.Width154, imagecache.Width500, imagecache.Width780} {
		if byWidth[w] != byWidth[imagecache.WidthOrig] {
			t.Errorf("w=%s and w=orig both clamp to %d pixels but hold different bytes; "+
				"they were encoded twice", w, srcW)
		}
	}
}

// TestDecodeAcceptsEveryRegisteredEncoding is what makes the blank imports in
// render.go load-bearing rather than decoration. Removing any one of them turns
// the matching case red.
func TestDecodeAcceptsEveryRegisteredEncoding(t *testing.T) {
	img := fixtureImage(64, 96)
	for name, body := range map[string][]byte{
		"png":  encodePNG(t, img),
		"jpeg": encodeJPEGFixture(t, img),
		"gif":  encodeGIFFixture(t, img),
		"webp": webpFixture(t),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decode(body)
			if err != nil {
				t.Fatalf("decode refused a %s cover: %v — the decoder for it is not registered", name, err)
			}
			if got.Bounds().Dx() <= 0 {
				t.Errorf("decoded a %s cover to %v", name, got.Bounds())
			}
		})
	}
}

// TestDecodeRefusesHonestly: an unusable cover must be a NAMED refusal, not a
// silent skip and not a generic error, because the caller uses the distinction
// to decide whether a retry is worth a round trip.
func TestDecodeRefusesHonestly(t *testing.T) {
	for name, body := range map[string][]byte{
		"empty":                          nil,
		"not an image":                   []byte("<html><body>404 not found</body></html>"),
		"a truncated png":                encodePNG(t, fixtureImage(64, 96))[:30],
		"an svg, which we do not decode": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decode(body)
			if err == nil {
				t.Fatal("decode accepted bytes that are not an image this build understands")
			}
			if !errors.Is(err, ErrUnsupportedFormat) {
				t.Errorf("refused with %v, want ErrUnsupportedFormat — a caller cannot tell "+
					"\"we fetched it and cannot use it\" from a transport failure", err)
			}
			if IsRetryable(err) {
				t.Error("an undecodable cover was classified as retryable; it answers identically forever")
			}
		})
	}
}

// TestDecodeRefusesADecompressionBomb fires the bound that a byte cap alone does
// NOT provide. The fixture is a hand-built PNG whose IHDR declares 30000x30000 —
// under a kilobyte on the wire, 3.6 GB as RGBA.
//
// It fails against a decode() that calls image.Decode first and measures
// afterwards: that one allocates the buffer before it can object.
func TestDecodeRefusesADecompressionBomb(t *testing.T) {
	_, err := decode(pngHeaderOnly(t, 30000, 30000))
	if err == nil {
		t.Fatal("decode accepted a 30000x30000 header; the pixel cap is checked after the " +
			"allocation, or not at all")
	}
	if !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("refused with %v, want ErrImageTooLarge", err)
	}
	if IsRetryable(err) {
		t.Error("a decompression bomb was classified as retryable; a retry fetches the same bomb")
	}

	// And the byte cap, which the pixel cap does not imply.
	if _, err := decode(make([]byte, maxSourceBytes+1)); !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("decode of %d bytes returned %v, want ErrImageTooLarge", maxSourceBytes+1, err)
	}
}

// TestScaleFlattensTransparencyOntoWhite pins the one input class JPEG cannot
// represent. Go's encoder drops alpha, so a fully-transparent pixel would arrive
// as BLACK — a transparent-background cover would render as a black rectangle.
//
// It fails against a scale() that composites with draw.Src onto a zero-valued
// destination, which is the obvious implementation.
func TestScaleFlattensTransparencyOntoWhite(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	// Entirely transparent: every pixel is the zero RGBA.
	out, err := renderAll(src)
	if err != nil {
		t.Fatalf("renderAll: %v", err)
	}
	var rendition []byte
	for _, r := range out.Renditions {
		if r.Width == imagecache.WidthOrig {
			rendition = r.Bytes
		}
	}
	got, err := jpeg.Decode(bytes.NewReader(rendition))
	if err != nil {
		t.Fatalf("decoding the rendition: %v", err)
	}
	r, g, b, _ := got.At(20, 20).RGBA()
	if r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Errorf("a fully-transparent source encoded to rgb(%d,%d,%d); JPEG has no alpha, so "+
			"transparency must be flattened onto white or a transparent cover becomes a black box",
			r>>8, g>>8, b>>8)
	}
}
