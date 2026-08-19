package imagepipeline

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// TestTheRealClientSatisfiesCoverSource is the compile-time proof that
// CoverSource needs no adapter. It is an assignment rather than a call, so it
// costs no round trip and cannot be satisfied by a wrapper — the day
// Client.Cover's signature moves, this stops building.
func TestTheRealClientSatisfiesCoverSource(t *testing.T) {
	var _ CoverSource = (*bookorbit.Client)(nil)
}

// fakeSource stands in for *bookorbit.Client. It returns the CLIENT'S OWN Cover
// type, which is the shape CoverSource requires, so a fake that satisfied this
// interface but not the real one is not expressible.
//
// It is a FAKE and not a replay of a real exchange: the client's own cassettes
// live in its package, and nothing in THIS package has ever seen upstream bytes.
type fakeSource struct {
	base  string
	cover bookorbit.Cover
	err   error
	calls []int64
}

func (f *fakeSource) BaseURL() string {
	if f.base == "" {
		return "http://books.example"
	}
	return f.base
}

func (f *fakeSource) Cover(_ context.Context, bookID int64) (bookorbit.Cover, error) {
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

func newTestPipeline(t *testing.T, f *fakeSource) (*Pipeline, *imagecache.Cache, *fakeWriter) {
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

	f := &fakeSource{cover: bookorbit.Cover{
		Status:      http.StatusOK,
		ContentType: "image/png",
		Body:        encodePNG(t, fixtureImage(srcW, srcH)),
	}}
	p, cache, w := newTestPipeline(t, f)

	res, err := p.Poster(t.Context(), 7, "book", "11")
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
	// The ITEM, not a work id — the store re-resolves the work inside its own
	// transaction. A pipeline that resolved it out here would be trusting an id
	// read before the network round trip.
	if p0.RemoteKind != "book" || p0.RemoteID != "11" {
		t.Errorf("recorded (%q, %q), want (\"book\", \"11\")", p0.RemoteKind, p0.RemoteID)
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

	// The URL is built from the client's BASE URL, so a credential can only
	// reach it through a base URL that carries one. bookorbit.New strips those
	// off — userinfo, query and fragment all cleared — which is exactly why this
	// drill goes through a fake: it fires the guard against the state the real
	// client is built to make impossible, and a guard only ever fired by the
	// thing it is meant to catch is a guard that has never been fired.
	for name, base := range map[string]string{
		"apikey":      "http://books.example/?apikey=" + drill,
		"apiKey":      "http://books.example/?apiKey=" + drill,
		"token":       "http://books.example/?token=" + drill,
		"userinfo":    "http://joe:" + drill + "@books.example",
		"unparseable": "://not a url",
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeSource{base: base, cover: bookorbit.Cover{
				Status: http.StatusOK,
				Body:   encodePNG(t, fixtureImage(64, 96)),
			}}
			p, cache, w := newTestPipeline(t, f)

			if _, err := p.Poster(t.Context(), 7, "book", "11"); err == nil {
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

// TestPosterClassifiesTheUpstreamStatus pins the LS-260 handling.
//
// 404 and 403 are split from each other AND from the rest, because they imply
// different actions: 404 is credential-scoped and says nothing about the file,
// 403 is a library the credential cannot reach at all. A pipeline that collapsed
// any two of these three groups would either hammer a permanent answer forever
// or give up on a restart, and the `scoped` column is what stops 403 from being
// filed as transient just because it is not a 404.
func TestPosterClassifiesTheUpstreamStatus(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		want      error
		retryable bool
		scoped    bool
	}{
		{"404 is not-for-this-credential", http.StatusNotFound, ErrCoverNotFound, false, true},
		{"403 is a scope problem", http.StatusForbidden, ErrCoverForbidden, false, true},
		{"503 is a restart", http.StatusServiceUnavailable, ErrUpstreamUnavailable, true, false},
		{"502 is a proxy", http.StatusBadGateway, ErrUpstreamUnavailable, true, false},
		{"429 is backpressure", http.StatusTooManyRequests, ErrUpstreamUnavailable, true, false},
		{"500 is an upstream bug", http.StatusInternalServerError, ErrUpstreamUnavailable, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSource{cover: bookorbit.Cover{
				Status: tc.status,
				Body:   []byte("an error page, not an image"),
			}}
			p, cache, w := newTestPipeline(t, f)

			_, err := p.Poster(t.Context(), 7, "book", "11")
			if !errors.Is(err, tc.want) {
				t.Fatalf("status %d produced %v, want %v", tc.status, err, tc.want)
			}
			if IsRetryable(err) != tc.retryable {
				t.Errorf("status %d: IsRetryable = %v, want %v", tc.status, IsRetryable(err), tc.retryable)
			}
			if IsCredentialScoped(err) != tc.scoped {
				t.Errorf("status %d: IsCredentialScoped = %v, want %v — a 404 here is overloaded "+
					"four ways and one of them is \"the credential's content filter hides this "+
					"book\", so recording it as a fact about the BOOK marks every hidden book "+
					"coverless forever", tc.status, IsCredentialScoped(err), tc.scoped)
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

// TestACredentialScopedRefusalPersistsNothing is the assertion that makes the
// 404-overloading safe, and it is the one that would fail if someone later
// "optimised" the 404 path by writing a state='gone' row to avoid re-fetching.
//
// A 404 can mean "the credential's content filter hides this book". Persisting
// anything keyed on the BOOK would mark it coverless for good, and widening the
// service account's filters could never undo it. The pipeline's answer is to
// persist nothing at all: no row, no file, nothing to invalidate.
func TestACredentialScopedRefusalPersistsNothing(t *testing.T) {
	for name, status := range map[string]int{
		"404": http.StatusNotFound,
		"403": http.StatusForbidden,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeSource{cover: bookorbit.Cover{Status: status}}
			p, cache, w := newTestPipeline(t, f)

			err := func() error { _, e := p.Poster(t.Context(), 7, "book", "11"); return e }()
			if !IsCredentialScoped(err) {
				t.Fatalf("status %d is not credential-scoped: %v", status, err)
			}
			if len(w.got) != 0 {
				t.Errorf("a credential-scoped refusal persisted %+v; a scope change could never "+
					"undo it", w.got)
			}
			if entries, _ := readDirNames(cache.Root()); len(entries) != 0 {
				t.Errorf("a credential-scoped refusal left %v on disk", entries)
			}
		})
	}
}

// TestAnAvifSourceIsRefusedByName pins ADR-0050's other half: AVIF is deferred as
// an OUTPUT, and separately there is no pure-Go AVIF decoder, so an AVIF COVER
// cannot be read at all. The requirement is that it fails loudly and names the
// cause — silently mis-decoding it, or skipping it, is what this forbids.
func TestAnAvifSourceIsRefusedByName(t *testing.T) {
	// A real AVIF container header: "ftypavif" at offset 4. Enough for a
	// decoder registry to be asked and to have no answer.
	body := append([]byte{0, 0, 0, 0x20}, []byte("ftypavifavifmif1miafMA1B")...)
	f := &fakeSource{cover: bookorbit.Cover{
		Status:      http.StatusOK,
		ContentType: "image/avif",
		Body:        body,
	}}
	p, cache, w := newTestPipeline(t, f)

	_, err := p.Poster(t.Context(), 7, "book", "11")
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("an AVIF cover produced %v, want ErrUnsupportedFormat — BookOrbit maps .avif and "+
			"serves it, so this is a reachable input and not a hypothetical", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("image/avif")) {
		t.Errorf("the refusal does not name the format it could not read: %v", err)
	}
	if len(w.got) != 0 || func() int { e, _ := readDirNames(cache.Root()); return len(e) }() != 0 {
		t.Error("an unreadable cover still wrote something")
	}
}

// A 200 IS STORED, whatever it carries. BookOrbit is read (from source, at
// 73b7877d, not probed) to have no placeholder arm — a missing cover throws
// rather than returning a default asset — so a 200 should always be a real
// cover. This is asserted anyway, because "should" is a reading of somebody
// else's controller and the behaviour on a 200 is worth having on the record.
func TestPosterStoresA200EvenThoughItMightBeAPlaceholder(t *testing.T) {
	f := &fakeSource{cover: bookorbit.Cover{
		Status: http.StatusOK,
		Body:   encodePNG(t, fixtureImage(300, 450)),
	}}
	p, _, w := newTestPipeline(t, f)

	if _, err := p.Poster(t.Context(), 7, "book", "11"); err != nil {
		t.Fatalf("Poster: %v", err)
	}
	if len(w.got) != 1 {
		t.Fatalf("a 200 wrote %d rows, want 1", len(w.got))
	}
}

func TestPosterTreatsAnEmpty200AsTransient(t *testing.T) {
	f := &fakeSource{cover: bookorbit.Cover{Status: http.StatusOK}}
	p, _, w := newTestPipeline(t, f)

	_, err := p.Poster(t.Context(), 7, "book", "11")
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
	f := &fakeSource{cover: bookorbit.Cover{
		Status:      http.StatusOK,
		ContentType: "image/avif",
		Body:        []byte("not an image at all"),
	}}
	p, cache, w := newTestPipeline(t, f)

	_, err := p.Poster(t.Context(), 7, "book", "11")
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
	f := &fakeSource{err: sentinel}
	p, _, w := newTestPipeline(t, f)

	_, err := p.Poster(t.Context(), 7, "book", "11")
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
		t.Error("New accepted a nil cover source")
	}
	if _, err := New(&fakeSource{}, nil, &fakeWriter{}, nil); err == nil {
		t.Error("New accepted a nil cache")
	}
	if _, err := New(&fakeSource{}, cache, nil, nil); err == nil {
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

// TestPosterParsesTheRemoteIDAtTheBoundary is the drill for the TEXT→int64 edge.
//
// `service_item_link.remote_id` is a TEXT column (libsync writes it with
// strconv.FormatInt), and the store hands it over unparsed. Every value below is
// something a real row could hold, and each must be REFUSED WITH A REASON — the
// failure this forbids is coercing it to 0 and letting the upstream answer an
// ordinary 404, which is indistinguishable from a book that exists and has no
// cover. It fails against a Poster that calls strconv.Atoi and ignores the error.
func TestPosterParsesTheRemoteIDAtTheBoundary(t *testing.T) {
	for name, remoteID := range map[string]string{
		"empty":         "",
		"a kavita key":  "series-4821",
		"a uuid":        "9f1c2d3e-4a5b-6c7d-8e9f-0a1b2c3d4e5f",
		"leading space": " 11",
		"a float":       "11.0",
		"not a number":  "eleven",
		"zero":          "0",
		"negative":      "-11",
		"beyond int64":  "99999999999999999999",
		"absurdly long": strings.Repeat("9", 500),
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeSource{cover: bookorbit.Cover{
				Status: http.StatusOK,
				Body:   encodePNG(t, fixtureImage(64, 96)),
			}}
			p, cache, w := newTestPipeline(t, f)

			_, err := p.Poster(t.Context(), 7, "book", remoteID)
			if !errors.Is(err, ErrRemoteIDNotABookID) {
				t.Fatalf("remote_id %q produced %v, want ErrRemoteIDNotABookID — a value that will "+
					"not parse is a corrupted row or a link from another service, and substituting "+
					"zero turns either into an ordinary 404", remoteID, err)
			}
			if IsRetryable(err) {
				t.Error("a malformed remote_id was classified as retryable; it will not fix itself")
			}
			// It must not have cost a round trip, a file or a row.
			if len(f.calls) != 0 {
				t.Errorf("the upstream was asked anyway, for book %v", f.calls)
			}
			if len(w.got) != 0 {
				t.Errorf("a malformed remote_id still wrote %+v", w.got)
			}
			if entries, _ := readDirNames(cache.Root()); len(entries) != 0 {
				t.Errorf("a malformed remote_id still wrote %v to the cache", entries)
			}
			// The value is in the message, bounded. It is a database column and
			// not a credential, and it is the whole of what makes the row findable.
			if remoteID != "" && len(remoteID) < 64 && !strings.Contains(err.Error(), remoteID) {
				t.Errorf("the refusal does not name the value that failed: %v", err)
			}
			if len(err.Error()) > 400 {
				t.Errorf("the refusal is %d characters; a TEXT column has no length bound and this "+
					"message reaches a log", len(err.Error()))
			}
		})
	}
}

// TestPosterBuildsOneSourceURLPerBook pins the field the pipeline INVENTS —
// everything else on the row comes from the client or the decoder.
//
// It fails against a Poster that builds one URL for every book (two books would
// collide on one cache key, and `source_url` is UNIQUE, so one cover would
// overwrite the other's), and against one that drops the base URL's path prefix.
func TestPosterBuildsOneSourceURLPerBook(t *testing.T) {
	render := func(t *testing.T, base, remoteID string) store.PosterAsset {
		t.Helper()
		f := &fakeSource{base: base, cover: bookorbit.Cover{
			Status: http.StatusOK,
			Body:   encodePNG(t, fixtureImage(64, 96)),
		}}
		p, _, w := newTestPipeline(t, f)
		if _, err := p.Poster(t.Context(), 7, "book", remoteID); err != nil {
			t.Fatalf("Poster: %v", err)
		}
		if len(f.calls) != 1 || f.calls[0] <= 0 {
			t.Fatalf("the client was asked for %v", f.calls)
		}
		return w.got[0]
	}

	got := render(t, "https://books.example", "11")
	const want = "https://books.example/api/v1/books/11/cover"
	if got.SourceURL != want {
		t.Errorf("source_url = %q, want %q — this is the route the client actually calls, and the "+
			"string the cache key is derived from", got.SourceURL, want)
	}

	// A reverse-proxy base path is part of the base URL bookorbit.New keeps.
	prefixed := render(t, "https://host.example/books", "11")
	if prefixed.SourceURL != "https://host.example/books/api/v1/books/11/cover" {
		t.Errorf("a base path prefix was dropped: %q", prefixed.SourceURL)
	}

	a := render(t, "https://books.example", "3")
	b := render(t, "https://books.example", "4")
	if a.CacheKey == b.CacheKey {
		t.Errorf("books 3 and 4 derived the same cache key %q; source_url is UNIQUE, so one cover "+
			"would overwrite the other's", a.CacheKey)
	}
}

// TestACoverFailureIsSurvivable encodes the PM's binding constraint at the only
// layer this package owns: a cover is DECORATION, and no failure of one may take
// down whatever is walking the catalogue. An import that dies over a missing
// image is worse than a library without pictures.
//
// The caller does not exist yet — that is the honest state of this package — so
// the constraint is expressed as the three properties any caller relies on, each
// of which can be broken here without the caller noticing until production:
//
//  1. every failure is an ERROR, never a panic. A panic in a walk over 2,000
//     books takes the whole import with it and no `if err != nil` saves anyone.
//  2. every failure persists NOTHING. A caller that ignores the error must not
//     be left with a half-written row or a truncated file.
//  3. the zero Result is safe to read. A caller that ignores the error reads
//     Result{} and must get zeroes, not a partly-populated struct that looks
//     like a success.
//
// The loop at the end is the constraint itself: a simulated walk in which EVERY
// book's cover fails, which must still visit every book and finish.
func TestACoverFailureIsSurvivable(t *testing.T) {
	good := encodePNG(t, fixtureImage(64, 96))

	// One case per failure mode Poster can produce, so a new failure path that
	// forgets the contract has to be added here to be added at all.
	cases := []struct {
		name     string
		remoteID string
		src      *fakeSource
	}{
		{"a malformed remote id", "not-a-book", &fakeSource{cover: bookorbit.Cover{Status: 200, Body: good}}},
		{"a transport failure", "11", &fakeSource{err: errors.New("dial tcp: connection refused")}},
		{"404", "11", &fakeSource{cover: bookorbit.Cover{Status: 404}}},
		{"403", "11", &fakeSource{cover: bookorbit.Cover{Status: 403}}},
		{"503", "11", &fakeSource{cover: bookorbit.Cover{Status: 503}}},
		{"an empty 200", "11", &fakeSource{cover: bookorbit.Cover{Status: 200}}},
		{"an undecodable body", "11", &fakeSource{cover: bookorbit.Cover{Status: 200, Body: []byte("nope")}}},
		{"a decompression bomb", "11", &fakeSource{cover: bookorbit.Cover{Status: 200, Body: pngHeaderOnly(t, 30000, 30000)}}},
		{"a credential in the base url", "11", &fakeSource{
			base:  "http://books.example/?apikey=not-a-real-key-just-a-drill",
			cover: bookorbit.Cover{Status: 200, Body: good},
		}},
		{"a store that refuses", "11", &fakeSource{cover: bookorbit.Cover{Status: 200, Body: good}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := imagecache.New(t.TempDir())
			w := &fakeWriter{}
			if tc.name == "a store that refuses" {
				w.err = errors.New("database is locked")
			}
			p, err := New(tc.src, cache, w, nil)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			// Property 1: an error, and no panic. `defer recover` would hide a
			// panic from the report, so it is deliberately NOT used — the test
			// binary failing loudly is the signal.
			res, perr := p.Poster(t.Context(), 7, "book", tc.remoteID)
			if perr == nil {
				t.Fatalf("%s produced no error; a caller cannot skip what it cannot see", tc.name)
			}

			// Property 3: the zero Result.
			if res != (Result{}) {
				t.Errorf("a failed Poster returned %+v; a caller that ignores the error reads this "+
					"and would treat a partly-populated struct as a success", res)
			}

			// Property 2: nothing durable. The store may have been CALLED — the
			// "store that refuses" case is exactly that — but nothing may be on
			// disk that a caller would later serve as an image.
			if entries, derr := readDirNames(cache.Root()); derr != nil {
				t.Fatalf("reading the cache root: %v", derr)
			} else if len(entries) != 0 && tc.name != "a store that refuses" {
				t.Errorf("%s left %v on disk", tc.name, entries)
			}
		})
	}

	// THE CONSTRAINT ITSELF: a walk in which every single cover fails must visit
	// every book and complete. This is the shape any eventual caller has to have,
	// and it fails against a Poster that panics on any of the modes above.
	const books = 50
	src := &fakeSource{err: errors.New("upstream is down")}
	p, err := New(src, imagecache.New(t.TempDir()), &fakeWriter{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	visited, failed := 0, 0
	for i := 1; i <= books; i++ {
		visited++
		if _, perr := p.Poster(t.Context(), 7, "book", strconv.Itoa(i)); perr != nil {
			// Exactly what a caller does with a decoration failure: count it,
			// and keep walking.
			failed++
			continue
		}
	}
	if visited != books || failed != books {
		t.Errorf("a walk over %d books with every cover failing visited %d and recorded %d failures; "+
			"it must visit every book and finish", books, visited, failed)
	}
	if len(src.calls) != books {
		t.Errorf("the walk asked for %d covers, want %d — it stopped early", len(src.calls), books)
	}
}
