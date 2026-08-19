package imagepipeline

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io/fs"
	"os"
	"testing"
)

// The fixtures every test in this package runs against.
//
// ⚠️ EVERY IMAGE THIS PIPELINE HAS EVER PROCESSED WAS BUILT HERE. There is no
// live BookOrbit in this environment and no recorded cover, so "the pipeline
// works" means "it works on images this file fabricated" and nothing stronger.
// The package doc says the same thing where a reader will meet it first; it is
// repeated here because this is the file that makes it true.

// tinyWebP is a 1x1 lossless (VP8L) WebP, base64-encoded because it cannot be
// produced from the tree: golang.org/x/image ships a WebP DECODER and no
// encoder, so unlike the PNG/JPEG/GIF fixtures this one cannot be generated.
//
// It is verified as a WebP by the test that uses it — image.DecodeConfig reports
// format "webp" — rather than trusted from its provenance. Its purpose is
// narrow: to prove the `_ "golang.org/x/image/webp"` import in render.go is
// registered and reachable. Delete that import and the webp case goes red.
const tinyWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func webpFixture(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(tinyWebP)
	if err != nil {
		t.Fatalf("decoding the webp fixture: %v", err)
	}
	return b
}

// pngHeaderOnly builds a PNG that is nothing but a signature and an IHDR
// declaring the given dimensions.
//
// It is the decompression-bomb fixture, and it is built by hand rather than
// encoded because THE POINT IS THAT IT IS TINY. A genuinely 30000x30000 image
// would have to be allocated to be encoded, which is the allocation the guard
// exists to prevent — the test would then be a bomb rather than a test of one.
// image.DecodeConfig answers from IHDR alone, so a header is the whole fixture.
func pngHeaderOnly(t *testing.T, w, h uint32) []byte {
	t.Helper()

	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	var ihdr bytes.Buffer
	_ = binary.Write(&ihdr, binary.BigEndian, w)
	_ = binary.Write(&ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{
		8, // bit depth
		6, // colour type: truecolour with alpha
		0, // compression
		0, // filter
		0, // interlace
	})

	chunk := func(typ string, data []byte) {
		_ = binary.Write(&buf, binary.BigEndian, uint32(len(data)))
		buf.WriteString(typ)
		buf.Write(data)
		crc := crc32.NewIEEE()
		_, _ = crc.Write([]byte(typ))
		_, _ = crc.Write(data)
		_ = binary.Write(&buf, binary.BigEndian, crc.Sum32())
	}
	chunk("IHDR", ihdr.Bytes())
	return buf.Bytes()
}

// readDirNames lists a directory, treating "it does not exist" as empty. The
// refusal tests use it to assert that nothing was written, and a cache root that
// was never created is the strongest form of that.
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
