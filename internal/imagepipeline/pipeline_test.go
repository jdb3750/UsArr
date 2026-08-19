package imagepipeline

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// fakeFetcher stands in for internal/bookorbit's Cover method, which is being
// added in the sync lane. It is a FAKE and not a mock of a real exchange: no
// cassette of a real cover exists, so nothing here has ever seen upstream bytes.
type fakeFetcher struct {
	cover Cover
	err   error
	calls []int64
}

func (f *fakeFetcher) Cover(_ context.Context, bookID int64) (Cover, error) {
	f.calls = append(f.calls, bookID)
	return f.cover, f.err
}

// fakeWriter records what the pipeline asked the store to persist, so the
// orchestration can be asserted without a database. The database half is
// asserted separately, against a real one, in internal/store.
type fakeWriter struct {
	got []store.PosterAsset
	id  int64
	err error
}

func (w *fakeWriter) PutPosterAsset(_ context.Context, p store.PosterAsset) (int64, error) {
	w.got = append(w.got, p)
	if w.err != nil {
		return 0, w.err
	}
	if w.id == 0 {
		w.id = 4242
	}
	return w.id, nil
}

const fixtureURL = "http://books.example/api/v1/books/11/cover"

func newTestPipeline(t *testing.T, f *fakeFetcher) (*Pipeline, *imagecache.Cache, *fakeWriter) {
	t.Helper()
	cache := imagecache.New(t.TempDir())
	w := &fakeWriter{}
	p, err := New(f, cache, w, func() time.Time {
		return time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, cache, w
}

// TestPosterWritesEveryWidthAndRecordsTheRow is the end-to-end assertion for the
// per-work entry point, against a fabricated cover.
//
// It fails against an empty implementation at every stage independently: the
// fetcher must be called with the book id, seven files must be readable back
// THROUGH imagecache.Open (not merely written somewhere), each must decode as a
// JPEG, and the store call must carry the derived key, the source URL, the
// configured origin class and the source's own dimensions.
func TestPosterWritesEveryWidthAndRecordsTheRow(t *testing.T) {
	const srcW, srcH = 600, 900

	f := &fakeFetcher{cover: Cover{
		SourceURL:   fixtureURL,
		Status:      http.StatusOK,
		ContentType: "image/png",
		Body:        encodePNG(t, fixtureImage(srcW, srcH)),
	}}
	p, cache, w := newTestPipeline(t, f)

	res, err := p.Poster(t.Context(), 77, 11, 7)
	if err != nil {
		t.Fatalf("Poster: %v", err)
	}

	if len(f.calls) != 1 || f.calls[0] != 11 {
		t.Errorf("the fetcher was called with %v, want exactly [11]", f.calls)
	}

	wantKey, err := imagecache.Key(fixtureURL)
	if err != nil {
		t.Fatalf("deriving the expected key: %v", err)
	}
	if res.CacheKey != wantKey {
		t.Errorf("Poster used cache key %q, want %q — the key the SERVING path will look under "+
			"is derived from the source URL and nothing else", res.CacheKey, wantKey)
	}
	if res.Renditions != len(imagecache.Widths()) {
		t.Errorf("Poster reports %d renditions, want %d", res.Renditions, len(imagecache.Widths()))
	}
	if res.SourceWidth != srcW || res.SourceHeight != srcH {
		t.Errorf("Poster reports the source as %dx%d, want %dx%d",
			res.SourceWidth, res.SourceHeight, srcW, srcH)
	}
	if res.AssetID != 4242 {
		t.Errorf("Poster returned asset id %d, want the one the writer answered", res.AssetID)
	}

	// THE BYTES ARE READ BACK THROUGH Open, which is the only way to catch a
	// writer and a reader that disagree about the layout. A test that stat-ed
	// the paths it had just computed would agree with itself and with nothing.
	total := 0
	for _, width := range imagecache.Widths() {
		file, info, oerr := cache.Open(res.CacheKey, width)
		if oerr != nil {
			t.Fatalf("Open(%q, %q) after Poster: %v — every /img request for this width would "+
				"answer not_cached", res.CacheKey, width, oerr)
		}
		body, rerr := io.ReadAll(file)
		_ = file.Close()
		if rerr != nil {
			t.Fatalf("reading w=%s: %v", width, rerr)
		}
		if int64(len(body)) != info.Size() {
			t.Errorf("w=%s: read %d bytes, stat says %d", width, len(body), info.Size())
		}
		_, format, derr := image.DecodeConfig(bytes.NewReader(body))
		if derr != nil || format != "jpeg" {
			t.Errorf("w=%s on disk decoded as %q (%v), want jpeg — ADR-0050 clause 1 allows no "+
				"passthrough width", width, format, derr)
		}
		total += len(body)
	}
	if res.CachedBytes != total {
		t.Errorf("Poster reports %d cached bytes, %d are on disk", res.CachedBytes, total)
	}

	if len(w.got) != 1 {
		t.Fatalf("the store was called %d times, want once", len(w.got))
	}
	p0 := w.got[0]
	if p0.WorkID != 77 {
		t.Errorf("recorded work id %d, want 77", p0.WorkID)
	}
	if p0.SourceURL != fixtureURL {
		t.Errorf("recorded source_url %q, want %q", p0.SourceURL, fixtureURL)
	}
	if p0.CacheKey != wantKey {
		t.Errorf("recorded cache_key %q, want %q — the row and the disk must agree or the "+
			"lookup finds a row whose bytes are somewhere else", p0.CacheKey, wantKey)
	}
	if p0.Format != store.ImageFormatJPEG {
		t.Errorf("recorded format %q, want %q", p0.Format, store.ImageFormatJPEG)
	}
	if !store.ValidImageFormat(p0.Format) {
		t.Errorf("recorded format %q is outside image_asset.format's vocabulary", p0.Format)
	}
	// The class comes from WHERE THE URL CAME FROM, never from what it looks
	// like. A cover URL constructed from a book id against an admin-typed base
	// URL is security.md §2 class 1, not the derived class.
	if p0.OriginClass != store.ImageOriginConfigured {
		t.Errorf("recorded origin_class %q, want %q", p0.OriginClass, store.ImageOriginConfigured)
	}
	if p0.ServiceInstanceID != 7 {
		t.Errorf("recorded origin_service_instance_id %d, want 7", p0.ServiceInstanceID)
	}
	if p0.Width != srcW || p0.Height != srcH {
		t.Errorf("recorded %dx%d, want the SOURCE's %dx%d", p0.Width, p0.Height, srcW, srcH)
	}
	if !p0.FetchedAt.Equal(time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("recorded fetched_at %v, want the injected clock's value", p0.FetchedAt)
	}
}

// TestPosterRefusesACredentialledCoverURL is the DRILL for the credential guard
// at the pipeline's own boundary, and it asserts the refusal is TOTAL: nothing
// on disk, nothing in the store.
//
// It fails against a pipeline that derives its key by hashing whatever it is
// given — that one writes seven files and a row for every case below.
func TestPosterRefusesACredentialledCoverURL(t *testing.T) {
	// Not a textbook example value: gitleaks waives `.+EXAMPLE$` by rule.
	const drill = "not-a-real-key-just-a-drill"

	for name, u := range map[string]string{
		"apikey":      "http://books.example/cover?apikey=" + drill,
		"apiKey":      "http://books.example/cover?apiKey=" + drill,
		"token":       "http://books.example/cover?token=" + drill,
		"userinfo":    "http://joe:" + drill + "@books.example/cover",
		"unparseable": "://not a url",
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeFetcher{cover: Cover{
				SourceURL: u,
				Status:    http.StatusOK,
				Body:      encodePNG(t, fixtureImage(64, 96)),
			}}
			p, cache, w := newTestPipeline(t, f)

			if _, err := p.Poster(t.Context(), 77, 11, 7); err == nil {
				t.Fatal("Poster accepted a credentialled cover URL; security.md §5 forbids storing " +
					"one, and cache_key = sha256(source_url)[:16] would bake it into the key")
			} else if !errors.Is(err, imagecache.ErrCredentialInURL) {
				t.Errorf("refused with %v, want ErrCredentialInURL", err)
			}

			if len(w.got) != 0 {
				t.Errorf("the refusal still reached the store with %+v", w.got)
			}
			// Nothing on disk under ANY key. The cache root must be untouched.
			if entries, err := readDirNames(cache.Root()); err != nil {
				t.Fatalf("reading the cache root: %v", err)
			} else if len(entries) != 0 {
				t.Errorf("the refusal left %v in the cache", entries)
			}
		})
	}
}

// TestPosterClassifiesTheUpstreamStatus pins the LS-260 handling: 404 terminal,
// every other non-2xx transient. It fails against a pipeline that collapses both
// into one error, which is the shape that either hammers a 404 forever or gives
// up on a restart.
func TestPosterClassifiesTheUpstreamStatus(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		want      error
		retryable bool
	}{
		{"404 is a book with no cover", http.StatusNotFound, ErrNoCover, false},
		{"503 is a restart", http.StatusServiceUnavailable, ErrUpstreamUnavailable, true},
		{"502 is a proxy", http.StatusBadGateway, ErrUpstreamUnavailable, true},
		{"429 is backpressure", http.StatusTooManyRequests, ErrUpstreamUnavailable, true},
		{"403 is a scope problem", http.StatusForbidden, ErrUpstreamUnavailable, true},
		{"500 is an upstream bug", http.StatusInternalServerError, ErrUpstreamUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeFetcher{cover: Cover{
				SourceURL: fixtureURL,
				Status:    tc.status,
				Body:      []byte("an error page, not an image"),
			}}
			p, cache, w := newTestPipeline(t, f)

			_, err := p.Poster(t.Context(), 77, 11, 7)
			if !errors.Is(err, tc.want) {
				t.Fatalf("status %d produced %v, want %v", tc.status, err, tc.want)
			}
			if IsRetryable(err) != tc.retryable {
				t.Errorf("status %d: IsRetryable = %v, want %v", tc.status, IsRetryable(err), tc.retryable)
			}
			if len(w.got) != 0 {
				t.Errorf("a non-2xx still wrote a row: %+v", w.got)
			}
			if entries, _ := readDirNames(cache.Root()); len(entries) != 0 {
				t.Errorf("a non-2xx still wrote %v to the cache", entries)
			}
		})
	}
}

// A 200 that carries a PLACEHOLDER image is indistinguishable from a real cover
// and IS stored — that is the honest consequence of not knowing which shape
// BookOrbit uses, and it is asserted so the behaviour is on the record rather
// than discovered later.
func TestPosterStoresA200EvenThoughItMightBeAPlaceholder(t *testing.T) {
	f := &fakeFetcher{cover: Cover{
		SourceURL: fixtureURL,
		Status:    http.StatusOK,
		Body:      encodePNG(t, fixtureImage(300, 450)),
	}}
	p, _, w := newTestPipeline(t, f)

	if _, err := p.Poster(t.Context(), 77, 11, 7); err != nil {
		t.Fatalf("Poster: %v", err)
	}
	if len(w.got) != 1 {
		t.Fatalf("a 200 wrote %d rows, want 1", len(w.got))
	}
}

func TestPosterTreatsAnEmpty200AsTransient(t *testing.T) {
	f := &fakeFetcher{cover: Cover{SourceURL: fixtureURL, Status: http.StatusOK}}
	p, _, w := newTestPipeline(t, f)

	_, err := p.Poster(t.Context(), 77, 11, 7)
	if !errors.Is(err, ErrNoBody) {
		t.Fatalf("an empty 200 produced %v, want ErrNoBody", err)
	}
	if !IsRetryable(err) {
		t.Error("an empty 200 was classified as terminal; it is a proxy or a truncation far more " +
			"often than it is a considered answer")
	}
	if len(w.got) != 0 {
		t.Errorf("an empty 200 still wrote a row: %+v", w.got)
	}
}

func TestPosterDoesNotWriteWhenTheCoverDoesNotDecode(t *testing.T) {
	f := &fakeFetcher{cover: Cover{
		SourceURL:   fixtureURL,
		Status:      http.StatusOK,
		ContentType: "image/avif",
		Body:        []byte("not an image at all"),
	}}
	p, cache, w := newTestPipeline(t, f)

	_, err := p.Poster(t.Context(), 77, 11, 7)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("Poster returned %v, want ErrUnsupportedFormat", err)
	}
	// The upstream's claim is reported as a DIAGNOSTIC — "it said image/avif and
	// we have no decoder" is actionable in a way "undecodable" is not.
	if !bytes.Contains([]byte(err.Error()), []byte("image/avif")) {
		t.Errorf("the failure does not name what the upstream claimed: %v", err)
	}
	if len(w.got) != 0 {
		t.Errorf("an undecodable cover still wrote a row: %+v", w.got)
	}
	if entries, _ := readDirNames(cache.Root()); len(entries) != 0 {
		t.Errorf("an undecodable cover still wrote %v to the cache", entries)
	}
}

func TestPosterSurfacesAFetchFailure(t *testing.T) {
	sentinel := errors.New("circuit breaker is open")
	f := &fakeFetcher{err: sentinel}
	p, _, w := newTestPipeline(t, f)

	_, err := p.Poster(t.Context(), 77, 11, 7)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Poster returned %v; the client's own sentinel must stay reachable through the wrap", err)
	}
	if len(w.got) != 0 {
		t.Errorf("a failed fetch still wrote a row: %+v", w.got)
	}
}

func TestNewRefusesAnIncompletePipeline(t *testing.T) {
	cache := imagecache.New(t.TempDir())
	if _, err := New(nil, cache, &fakeWriter{}, nil); err == nil {
		t.Error("New accepted a nil fetcher")
	}
	if _, err := New(&fakeFetcher{}, nil, &fakeWriter{}, nil); err == nil {
		t.Error("New accepted a nil cache")
	}
	if _, err := New(&fakeFetcher{}, cache, nil, nil); err == nil {
		t.Error("New accepted a nil writer")
	}
}

// TestBoundedContentTypeTruncates pins the bound on a header value that reaches
// a log line and service_instance.last_error.
func TestBoundedContentTypeTruncates(t *testing.T) {
	long := bytes.Repeat([]byte("a"), contentTypeMax*3)
	got := boundedContentType(string(long))
	if len(got) <= contentTypeMax || len(got) > contentTypeMax+16 {
		t.Errorf("boundedContentType returned %d characters for a %d-character header", len(got), len(long))
	}
	if got := boundedContentType("image/jpeg"); got != "image/jpeg" {
		t.Errorf("boundedContentType mangled a normal media type: %q", got)
	}
}
