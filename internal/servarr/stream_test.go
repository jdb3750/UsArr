package servarr

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// ---- test doubles ---------------------------------------------------------

// doerFunc is an HTTPDoer for tests that need a body no cassette can hold. The
// cassettes stay the source of truth for wire SHAPES; these tests are about
// SIZES, and a 40 MB fixture does not belong in the repository.
type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

// ssrfRedactedMarker is whatever internal/ssrf substitutes for a credential in a
// URL. It is DERIVED rather than hard-coded because that package owns its own
// vocabulary and this one deliberately does not try to unify the two (see
// RedactedPlaceholder's doc comment).
var ssrfRedactedMarker = func() string {
	u, err := url.Parse(RedactURL("http://x/?apikey=secret"))
	if err != nil {
		panic(err)
	}
	return u.Query().Get("apikey")
}()

func newFakeClient(t *testing.T, do doerFunc) *Client {
	t.Helper()
	c, err := NewProwlarr(Options{
		BaseURL:    testBaseURL,
		APIKey:     testAPIKey,
		HTTPClient: do,
		AppVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

// listItem is a stand-in for the *Arr list resources that have not landed yet
// (MovieResource, SeriesResource). Its SIZE is the point: sync.md §2's "10k
// movies is 30-80 MB" works out at 3-8 KB per element, so a 300-byte toy would
// measure the wrong thing in both directions — too many decode calls per
// megabyte, and a decoder window nothing like the real one.
type listItem struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Year     int      `json:"year"`
	Overview string   `json:"overview"`
	Path     string   `json:"path"`
	HasFile  bool     `json:"hasFile"`
	Genres   []string `json:"genres"`
	Ratings  struct {
		Value float64 `json:"value"`
		Votes int     `json:"votes"`
	} `json:"ratings"`
}

// gennedOverview pads one element to the ~3.5 KB a real movie resource occupies.
var gennedOverview = strings.Repeat(
	"A synthetic overview, padded to the length a real one reaches once a *Arr has "+
		"filled in its metadata: overview, alternate titles, images, ratings and a "+
		"few dozen scalars. ", 18)

// gennedTail is everything after the id-dependent prefix, built ONCE. The
// generator writes straight into its buffer rather than formatting a fresh 3.5 KB
// string per element: a fixture that allocates more than the code under test is a
// fixture that measures itself.
var gennedTail = `,"overview":"` + gennedOverview + `","path":"/data/movies/Title (2001)",` +
	`"hasFile":true,"genres":["Drama","Science Fiction"],"ratings":{"value":7.5,"votes":1000}}`

func writeGennedElement(b *bytes.Buffer, i int) {
	b.WriteString(`{"id":`)
	b.WriteString(strconv.Itoa(i))
	b.WriteString(`,"title":"Title `)
	b.WriteString(strconv.Itoa(i))
	b.WriteString(`","year":`)
	b.WriteString(strconv.Itoa(1980 + i%40))
	b.WriteString(gennedTail)
}

// gennedBytes is the exact size of the array arrayGen produces for n elements.
func gennedBytes(n int) int {
	if n == 0 {
		return 2
	}
	var scratch bytes.Buffer
	total := 2 + (n - 1) // brackets plus commas
	for i := range n {
		scratch.Reset()
		writeGennedElement(&scratch, i)
		total += scratch.Len()
	}
	return total
}

// arrayGen is a JSON array of n elements produced ON READ. It never holds the
// whole document, which is the point: a fixture that materialised 40 MB would
// dominate the very measurement it exists to make.
type arrayGen struct {
	total   int
	emitted int
	started bool
	closed  bool
	buf     bytes.Buffer

	// stopAfter, when >= 0, cuts the stream off after that many elements —
	// stopErr non-nil for a connection that died, nil for a clean truncation.
	stopAfter int
	stopErr   error
}

func newArrayGen(n int) *arrayGen { return &arrayGen{total: n, stopAfter: -1} }

func (g *arrayGen) Read(p []byte) (int, error) {
	for g.buf.Len() < len(p) {
		switch {
		case !g.started:
			g.buf.WriteByte('[')
			g.started = true
		case g.stopAfter >= 0 && g.emitted >= g.stopAfter:
			if g.buf.Len() > 0 {
				return g.buf.Read(p)
			}
			if g.stopErr != nil {
				return 0, g.stopErr
			}
			return 0, io.EOF
		case g.emitted < g.total:
			if g.emitted > 0 {
				g.buf.WriteByte(',')
			}
			writeGennedElement(&g.buf, g.emitted)
			g.emitted++
		case !g.closed:
			g.buf.WriteByte(']')
			g.closed = true
		default:
			if g.buf.Len() > 0 {
				return g.buf.Read(p)
			}
			return 0, io.EOF
		}
	}
	return g.buf.Read(p)
}

func (g *arrayGen) Close() error { return nil }

// arrayResponse builds a 200 whose body is a generated array.
func arrayResponse(g *arrayGen) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain"}, "X-Application-Version": []string{"5.28.0.10229"}},
		Body:       g,
	}
}

func stringResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// ---- the read path --------------------------------------------------------

func TestStreamListDeliversEveryElementAndPreservesTheRequestContract(t *testing.T) {
	var gotReq *http.Request
	c := newFakeClient(t, func(r *http.Request) (*http.Response, error) {
		gotReq = r
		return arrayResponse(newArrayGen(2500)), nil
	})

	var seen int
	var lastID int
	n, err := StreamList(ctx(t), c, "/movie", url.Values{"includeImages": {"false"}}, func(it listItem) error {
		if it.ID != seen {
			t.Fatalf("element %d arrived out of order: id=%d", seen, it.ID)
		}
		seen++
		lastID = it.ID
		return nil
	})
	if err != nil {
		t.Fatalf("StreamList: %v", err)
	}
	if n != 2500 || seen != 2500 || lastID != 2499 {
		t.Fatalf("n=%d seen=%d lastID=%d, want 2500/2500/2499", n, seen, lastID)
	}

	// Everything do guarantees about the outbound request, asserted on the new path.
	if got := gotReq.Header.Get("X-Api-Key"); got != testAPIKey {
		t.Errorf("X-Api-Key = %q, want the configured key", got)
	}
	if gotReq.URL.Query().Has("apikey") {
		t.Errorf("the key went into the query string: %s", gotReq.URL.RawQuery)
	}
	if got := gotReq.Header.Get("Accept"); got != "application/json, text/plain;q=0.9, */*;q=0.1" {
		t.Errorf("Accept = %q; ReturnHttpNotAcceptable=true means a bare application/json can 406", got)
	}
	if got := gotReq.Header.Get("User-Agent"); !strings.HasPrefix(got, "UsArr/") {
		t.Errorf("User-Agent = %q", got)
	}
	if gotReq.Header.Get("X-Prowlarr-Client") != "" {
		t.Error("X-Prowlarr-Client must never be sent")
	}
	if got := gotReq.URL.Path; got != "/api/v1/movie" {
		t.Errorf("path = %q; StreamList's path is relative to the client's API path", got)
	}
	// The body was text/plain and was parsed as JSON anyway.
	// X-Application-Version is captured on the streaming path too, or version
	// detection silently stops working for the sync engine.
	if c.AppVersion() != "5.28.0.10229" {
		t.Errorf("AppVersion() = %q, want the streamed response's header", c.AppVersion())
	}
	if c.Breaker().State() != BreakerClosed {
		t.Errorf("breaker = %s after a clean stream", c.Breaker().State())
	}
}

// THE DEFECT THIS CHANGE EXISTS FOR. The buffered path capped every response at
// 32 MB, under an ssrf transport that permits 200 MB, while sync.md §2 puts a
// 10k-movie GET /api/v3/movie at 30-80 MB.
func TestStreamListReadsBodiesTheBufferedCapWouldHaveRejected(t *testing.T) {
	const elements = 11_000 // ~38 MB at ~3.5 KB per element
	size := gennedBytes(elements)
	if int64(size) <= maxBufferedBody {
		t.Fatalf("fixture is %d bytes, which the old 32 MB cap would have accepted; it proves nothing", size)
	}

	c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return arrayResponse(newArrayGen(elements)), nil
	})
	var sum int64
	n, err := StreamList(ctx(t), c, "/movie", nil, func(it listItem) error {
		sum += int64(it.Year)
		return nil
	})
	if err != nil {
		t.Fatalf("streaming %d bytes: %v", size, err)
	}
	if n != elements {
		t.Fatalf("n = %d, want %d", n, elements)
	}
	t.Logf("streamed %.1f MB / %d elements through the default %d MB bound",
		float64(size)/(1<<20), n, c.maxStream>>20)

	// The same body on the buffered path must fail LOUDLY rather than truncate.
	var out []listItem
	err = c.do(ctx(t), request{op: "Buffered", method: http.MethodGet, path: "/api/v1/movie"}, &out)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("buffered read of %d bytes: err = %v, want ErrResponseTooLarge", size, err)
	}
	if errors.Is(err, ErrDecode) {
		t.Error("an over-cap body must not be reported as malformed JSON; that diagnoses the wrong thing")
	}
	if len(out) != 0 {
		t.Errorf("the buffered path decoded %d elements from a truncated body", len(out))
	}
}

// The bound is lifted, not removed.
func TestStreamListEnforcesItsBound(t *testing.T) {
	c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return arrayResponse(newArrayGen(20_000)), nil
	})
	// Fired against a small bound rather than 200 MB: the arithmetic is the same
	// at both values and a race-instrumented 200 MB decode is a two-minute test.
	c.maxStream = 1 << 20

	n, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error { return nil })
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
	if n == 0 {
		t.Error("elements decoded before the bound was hit should still be reported")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("not an *APIError: %T", err)
	}
	if apiErr.Status != 200 {
		t.Errorf("status = %d, want 200: the response was fine, the size was not", apiErr.Status)
	}
	assertNoCredential(t, "over-limit error", err.Error())
}

// The 200 MB stream bound and internal/ssrf's MaxListBytes are the same number in
// two packages, because servarr must not import ssrf (doc.go). Pin them together
// so the duplication cannot drift: a client bound below the transport's is the
// 32 MB bug all over again.
func TestMaxStreamBytesTracksTheSSRFCeiling(t *testing.T) {
	if MaxStreamBytes != ssrf.MaxListBytes {
		t.Fatalf("MaxStreamBytes = %d, ssrf.MaxListBytes = %d; the client bound must equal the transport ceiling",
			MaxStreamBytes, ssrf.MaxListBytes)
	}
	if maxBufferedBody >= MaxStreamBytes {
		t.Errorf("the buffered cap (%d) is meant to stay well under the stream bound (%d)", maxBufferedBody, MaxStreamBytes)
	}
}

// ---- the partial-delivery contract ----------------------------------------

// A body that stops mid-array must not look like a short list. The caller gets an
// error AND the count of elements already handed to it.
func TestStreamListTruncatedMidArrayReportsHowFarItGot(t *testing.T) {
	g := newArrayGen(500)
	g.stopAfter = 137 // clean EOF partway through: a server that died mid-write
	c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return arrayResponse(g), nil
	})

	var applied []int
	n, err := StreamList(ctx(t), c, "/movie", nil, func(it listItem) error {
		applied = append(applied, it.ID)
		return nil
	})
	if err == nil {
		t.Fatal("a truncated array must not be reported as a complete list")
	}
	if !errors.Is(err, ErrDecode) {
		t.Fatalf("err = %v, want ErrDecode", err)
	}
	// The contract: n is what fn actually saw, and fn saw exactly that many.
	if n != len(applied) {
		t.Fatalf("reported n = %d but fn was called %d times", n, len(applied))
	}
	// The generator emits 137 whole elements and then cuts, so every one of them
	// can legitimately be delivered — what must NOT happen is a silent success.
	// This is the case the closing-bracket read exists for: dec.More() simply
	// returns false at a read error, so without that final Token() a body cut
	// exactly on an element boundary would return (137, nil) — a half payload
	// applied silently, reported as a complete list.
	if n < 100 || n > 137 {
		t.Fatalf("n = %d, want the elements delivered before the cut (0 < n <= 137)", n)
	}
	t.Logf("truncation contract: %d elements delivered, then %v", n, err)
	assertNoCredential(t, "truncation error", err.Error())
}

// A connection that dies mid-array is upstream flakiness, not bad JSON.
func TestStreamListMidStreamTransportErrorIsClassifiedAndRedacted(t *testing.T) {
	g := newArrayGen(500)
	g.stopAfter = 40
	g.stopErr = &url.Error{
		Op:  "Get",
		URL: testBaseURL + "/api/v1/movie?apikey=" + testAPIKey,
		Err: errors.New("read: connection reset by peer"),
	}
	c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return arrayResponse(g), nil
	})
	// One failure is enough to open, so the breaker's verdict is readable from
	// State() without an accessor that exists only for a test.
	c.breaker = NewBreaker(BreakerConfig{FailureThreshold: 1}, nil, nil)

	n, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error { return nil })
	if !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if n == 0 {
		t.Error("the elements delivered before the reset should still be reported")
	}
	// transportError reconstructs rather than wraps precisely because *url.Error
	// prints the full URL, and on this integration that URL can carry the key.
	assertNoCredential(t, "mid-stream transport error", err.Error())
	// A URL may survive in the message, but only after ssrf's deny-list has been
	// through it — that is what redactErr is for.
	if got := err.Error(); strings.Contains(got, "apikey=") && !strings.Contains(got, "apikey="+ssrfRedactedMarker) {
		t.Errorf("a query credential survived into the error: %q", got)
	}
	if got := c.Breaker().State(); got != BreakerOpen {
		t.Errorf("breaker = %s; a connection reset mid-stream is upstream flakiness and must count", got)
	}
}

// An error from the caller's own element function comes back untouched.
func TestStreamListReturnsTheCallersErrorVerbatim(t *testing.T) {
	errStore := errors.New("writing chunk to sqlite")
	c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return arrayResponse(newArrayGen(500)), nil
	})
	c.breaker = NewBreaker(BreakerConfig{FailureThreshold: 1}, nil, nil)

	n, err := StreamList(ctx(t), c, "/movie", nil, func(it listItem) error {
		if it.ID == 12 {
			return errStore
		}
		return nil
	})
	if !errors.Is(err, errStore) {
		t.Fatalf("err = %v, want the caller's own error", err)
	}
	if errors.Is(err, ErrDecode) {
		t.Error("a caller's error must not be relabelled as a decode failure")
	}
	if n != 12 {
		t.Errorf("n = %d, want 12 (elements delivered before the caller stopped)", n)
	}
	// The upstream answered correctly; only the caller failed. Nothing to hold
	// against the instance — and with a threshold of 1, one wrongly-counted
	// failure would open the breaker.
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("breaker = %s; a caller-side failure is not evidence about the instance", got)
	}
}

// ---- the error taxonomy, on the streaming path ----------------------------

func TestStreamListNeverStreamsANonSuccessResponse(t *testing.T) {
	called := 0
	cases := []struct {
		name    string
		status  int
		body    string
		want    error
		breaker BreakerState
	}{
		{
			// A 401 has an EMPTY BODY and must not be parsed at all.
			name: "401 empty body", status: 401, body: "", want: ErrUnauthorized, breaker: BreakerClosed,
		},
		{
			name: "400 bare validation array", status: 400,
			body: `[{"propertyName":"Page","errorMessage":"must not be empty"}]`,
			want: ErrValidation, breaker: BreakerClosed,
		},
		{
			name: "400 problem details", status: 400,
			body: `{"title":"One or more validation errors occurred.","status":400,"errors":{"$.page":["not a number"]}}`,
			want: ErrValidation, breaker: BreakerClosed,
		},
		{
			name: "500 arr envelope", status: 500,
			body: `{"message":"Internal Server Error","description":"NzbDrone.Core.Exceptions..."}`,
			want: ErrServer, breaker: BreakerClosed, // one failure is not five
		},
		{
			name: "404", status: 404, body: `{"message":"Not Found"}`, want: ErrNotFound, breaker: BreakerClosed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
				return stringResponse(tc.status, tc.body), nil
			})
			n, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error {
				called++
				t.Error("the callback saw a non-2xx body")
				return nil
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if n != 0 {
				t.Errorf("n = %d, want 0", n)
			}
			if errors.Is(err, ErrDecode) {
				t.Errorf("an error envelope was reported as a decode failure: %v", err)
			}
			if got := c.Breaker().State(); got != tc.breaker {
				t.Errorf("breaker = %s, want %s", got, tc.breaker)
			}
			assertNoCredential(t, tc.name, err.Error())
		})
	}
	if called != 0 {
		t.Fatalf("the callback ran %d times on non-2xx responses", called)
	}
}

// 4xx must not trip the breaker; 5xx must.
func TestStreamListBreakerPolicyMatchesTheBufferedPath(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   BreakerState
	}{
		{400, BreakerClosed},
		{401, BreakerClosed},
		{404, BreakerClosed},
		{500, BreakerOpen},
		{503, BreakerOpen},
	} {
		c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
			return stringResponse(tc.status, `{"message":"nope"}`), nil
		})
		c.breaker = NewBreaker(BreakerConfig{FailureThreshold: 1}, nil, nil)
		if _, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error { return nil }); err == nil {
			t.Fatalf("%d: want an error", tc.status)
		}
		if got := c.Breaker().State(); got != tc.want {
			t.Errorf("%d: breaker = %s, want %s", tc.status, got, tc.want)
		}
		// And an open breaker refuses the next call without touching the network.
		if tc.want == BreakerOpen {
			_, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error { return nil })
			if !errors.Is(err, ErrBreakerOpen) {
				t.Errorf("%d: second call err = %v, want ErrBreakerOpen", tc.status, err)
			}
		}
	}
}

func TestStreamListRejectsBodiesThatAreNotArrays(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"empty", ""},
		{"object", `{"message":"this endpoint is not a list"}`},
		{"html from a reverse proxy", "<html><body>200 but wrong</body></html>"},
		{"array of the wrong type", `["a","b"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
				return stringResponse(200, tc.body), nil
			})
			n, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error { return nil })
			if !errors.Is(err, ErrDecode) {
				t.Fatalf("err = %v, want ErrDecode", err)
			}
			if n != 0 {
				t.Errorf("n = %d, want 0", n)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != 200 {
				t.Errorf("err = %+v; a 200 that decodes badly is still a 200", apiErr)
			}
			assertNoCredential(t, tc.name, err.Error())
		})
	}
}

func TestStreamListTransportFailureBeforeAStatusCode(t *testing.T) {
	c := newFakeClient(t, func(r *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: r.URL.String() + "?apikey=" + testAPIKey, Err: errors.New("dial tcp: connect: connection refused")}
	})
	n, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error { return nil })
	if !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	assertNoCredential(t, "dial failure", err.Error())
}

func TestStreamListRequiresACallback(t *testing.T) {
	c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		t.Error("no request should have been made")
		return nil, errors.New("unreachable")
	})
	if _, err := StreamList[listItem](ctx(t), c, "/movie", nil, nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestStreamListEmptyArrayIsNotAnError(t *testing.T) {
	c := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return stringResponse(200, "[]"), nil
	})
	n, err := StreamList(ctx(t), c, "/movie", nil, func(listItem) error {
		t.Error("callback ran on an empty array")
		return nil
	})
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v, want 0/nil", n, err)
	}
}

// ---- the memory claim, measured -------------------------------------------

// measure runs f with GC-sampled peak heap. peak is the highest HeapAlloc seen
// above the pre-run baseline; churn is total bytes allocated.
//
// Sampling rather than instrumenting is the honest way to get PEAK: runtime's own
// counters give cumulative allocation, and a 60 MB buffered read that is freed
// before the function returns would otherwise look identical to one that was
// never held.
func measure(f func()) (peak, churn uint64) {
	runtime.GC()
	var start runtime.MemStats
	runtime.ReadMemStats(&start)

	stop := make(chan struct{})
	result := make(chan uint64, 1)
	go func() {
		var m runtime.MemStats
		var hi uint64
		tick := time.NewTicker(200 * time.Microsecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				result <- hi
				return
			case <-tick.C:
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > hi {
					hi = m.HeapAlloc
				}
			}
		}
	}()

	f()

	close(stop)
	hi := <-result
	var end runtime.MemStats
	runtime.ReadMemStats(&end)
	if hi > start.HeapAlloc {
		peak = hi - start.HeapAlloc
	}
	churn = end.TotalAlloc - start.TotalAlloc
	return peak, churn
}

// BenchmarkListRead measures what the change is FOR. Run it with `make bench`, or
// directly:
//
//	go test ./internal/servarr -run '^$' -bench BenchmarkListRead -benchmem
//
// It is a benchmark and not a test on purpose: `go test ./...` compiles it but
// does not run it, so the gate — `make check` — pays nothing, and a memory ratio
// asserted on a shared runner is a flake generator (docs/DEVELOPMENT.md §5).
//
// Both arms are the REAL code paths — c.do and StreamList — over the same
// generated body at 24 MB, which is the largest size the 32 MB buffered cap can
// carry at all.
func BenchmarkListRead(b *testing.B) {
	const elements = 6_800 // ~24 MB, just under the buffered cap
	size := gennedBytes(elements)
	b.Logf("payload %.1f MB / %d elements", float64(size)/(1<<20), elements)

	newClient := func() *Client {
		c, err := NewProwlarr(Options{
			BaseURL: testBaseURL, APIKey: testAPIKey, AppVersion: "0.0.0-test",
			HTTPClient: doerFunc(func(*http.Request) (*http.Response, error) {
				return arrayResponse(newArrayGen(elements)), nil
			}),
		})
		if err != nil {
			b.Fatalf("building client: %v", err)
		}
		return c
	}

	b.Run("buffered", func(b *testing.B) {
		c := newClient()
		var peak, churn uint64
		b.ResetTimer()
		for range b.N {
			p, ch := measure(func() {
				var out []listItem
				if err := c.do(b.Context(), request{op: "Bench", method: http.MethodGet, path: "/api/v1/movie"}, &out); err != nil {
					b.Fatalf("do: %v", err)
				}
				if len(out) != elements {
					b.Fatalf("decoded %d elements", len(out))
				}
			})
			if p > peak {
				peak = p
			}
			churn += ch
		}
		b.ReportMetric(float64(peak)/(1<<20), "peakMB")
		b.ReportMetric(float64(churn)/float64(b.N)/(1<<20), "churnMB/op")
	})

	b.Run("streaming", func(b *testing.B) {
		c := newClient()
		var peak, churn uint64
		b.ResetTimer()
		for range b.N {
			p, ch := measure(func() {
				n, err := StreamList(b.Context(), c, "/movie", nil, func(listItem) error { return nil })
				if err != nil {
					b.Fatalf("StreamList: %v", err)
				}
				if n != elements {
					b.Fatalf("streamed %d elements", n)
				}
			})
			if p > peak {
				peak = p
			}
			churn += ch
		}
		b.ReportMetric(float64(peak)/(1<<20), "peakMB")
		b.ReportMetric(float64(churn)/float64(b.N)/(1<<20), "churnMB/op")
	})
}
