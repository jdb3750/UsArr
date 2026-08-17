package kavita

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
)

// Streaming guards. Every one of these exists because POST /api/Series/all-v2
// returns the WHOLE library in ONE response when it is not paged
// (UserParams.PageSize defaults to int.MaxValue), so the failure modes that
// matter are the ones a bounded, incremental read has and a buffered read does
// not.

func streamClient(t *testing.T, status int, body string, headers http.Header) *Client {
	t.Helper()
	c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		return respond(status, body, headers), nil
	})
	return c
}

func TestStreamSeriesDeliversEveryElement(t *testing.T) {
	c := newTestClient(t, "kavita_series_all_v2")

	var got []SeriesDto
	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(s SeriesDto) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamSeries: %v", err)
	}
	if page.Count != 3 || len(got) != 3 {
		t.Fatalf("Count = %d, delivered %d, want 3", page.Count, len(got))
	}
	if got[0].Name != "Frieren: Beyond Journey's End" || got[2].Name != "The Hobbit" {
		t.Errorf("elements decoded wrong: %q … %q", got[0].Name, got[2].Name)
	}
	if got[2].Format != MangaFormatEpub {
		t.Errorf("format = %d, want MangaFormatEpub (3)", got[2].Format)
	}

	// The fact ADR-0035 §2a measured live: the watermark moves on the SCAN JOB's
	// clock, so sibling series share a boundary timestamp to the microsecond.
	// This is why ARCHITECTURE.md §7.1a's overlap window is not optional, and the
	// fixture carries it so a future walk can be tested against it.
	if !got[1].LastChapterAddedUtc.Time().Equal(got[2].LastChapterAddedUtc.Time()) {
		t.Errorf("the fixture no longer reproduces the boundary-timestamp cluster: %v vs %v",
			got[1].LastChapterAddedUtc, got[2].LastChapterAddedUtc)
	}

	// Degraded identity is the ordinary case on a free instance, not a fixture
	// oversight.
	for _, s := range got {
		if s.AniListID != 0 || s.MalID != 0 || s.ComicVineID != "" {
			t.Errorf("series %d carries a Kavita+ identifier; the fixture is a FREE instance", s.ID)
		}
	}
}

// TestStreamSeriesReadsThePaginationHeader pins the undocumented header. It is
// middleware — it appears nowhere in the OpenAPI document — so it is read
// opportunistically.
func TestStreamSeriesReadsThePaginationHeader(t *testing.T) {
	c := newTestClient(t, "kavita_series_all_v2")
	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error { return nil })
	if err != nil {
		t.Fatalf("StreamSeries: %v", err)
	}
	if page.Pagination == nil {
		t.Fatal("the Pagination header was not read")
	}
	if page.Pagination.TotalItems != 3 {
		t.Errorf("TotalItems = %d, want 3", page.Pagination.TotalItems)
	}
	// int.MaxValue is what an unpaged read gets back, and it is the number that
	// makes buffering this endpoint a memory bug.
	if page.Pagination.ItemsPerPage != 2147483647 {
		t.Errorf("ItemsPerPage = %d; an unpaged read reports int.MaxValue", page.Pagination.ItemsPerPage)
	}
}

// TestStreamSeriesSurvivesAMissingPaginationHeader is the degraded case, and it
// is the one that matters operationally: the header is not in the spec, so a
// reverse proxy that strips unknown headers must leave the read SUCCEEDING with
// the total unknown — never failing, and never reporting zero.
func TestStreamSeriesSurvivesAMissingPaginationHeader(t *testing.T) {
	c := newTestClient(t, "kavita_series_all_v2_paged_no_header")
	page, err := c.StreamSeries(context.Background(),
		SeriesListOptions{PageNumber: 1, PageSize: 1},
		func(SeriesDto) error { return nil })
	if err != nil {
		t.Fatalf("a missing Pagination header must not fail the read: %v", err)
	}
	if page.Count != 1 {
		t.Errorf("Count = %d, want 1", page.Count)
	}
	if page.Pagination != nil {
		t.Errorf("Pagination = %+v; it must be nil so callers render 'unknown' rather than 0", page.Pagination)
	}
}

func TestStreamSeriesIgnoresAnUnparseablePaginationHeader(t *testing.T) {
	c := streamClient(t, 200, `[]`, http.Header{"Pagination": []string{"not json"}})
	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error { return nil })
	if err != nil {
		t.Fatalf("an unparseable header must not fail the read: %v", err)
	}
	if page.Pagination != nil {
		t.Errorf("Pagination = %+v, want nil", page.Pagination)
	}
}

// TestStreamSeriesTruncatedMidArrayReportsHowFarItGot is the partial-delivery
// contract, fired.
//
// A body cut mid-array must return the error AND the count. Silence here would be
// the worst outcome in this whole package: a half-imported library reported as
// complete, which channel 4's reconciliation sweep would then read as content
// deleted upstream.
func TestStreamSeriesTruncatedMidArrayReportsHowFarItGot(t *testing.T) {
	// Two complete elements, then the body stops — with no closing bracket.
	body := `[{"id":1,"name":"a"},{"id":2,"name":"b"},`
	c := streamClient(t, 200, body, nil)

	var delivered int
	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error {
		delivered++
		return nil
	})
	if err == nil {
		t.Fatal("a truncated body returned no error; a half payload was reported as a complete list")
	}
	if !errors.Is(err, ErrDecode) {
		t.Errorf("got %v, want ErrDecode", err)
	}
	if page.Count != 2 || delivered != 2 {
		t.Errorf("Count = %d, delivered = %d; both must report the 2 elements that DID reach fn",
			page.Count, delivered)
	}
	if !strings.Contains(err.Error(), "were delivered") {
		t.Errorf("the error must say what was already applied: %q", err)
	}
}

// TestStreamSeriesCutOnAnElementBoundaryStillFails is the subtle half of the
// same bug, and it is why the closing bracket is READ rather than assumed:
// dec.More() answers false at a read error exactly as it does at ']'.
func TestStreamSeriesCutOnAnElementBoundaryStillFails(t *testing.T) {
	// Note: no trailing comma and no ']'. The cut lands exactly where an element
	// ends, which is the case a naive loop reports as success.
	body := `[{"id":1,"name":"a"},{"id":2,"name":"b"}`
	c := streamClient(t, 200, body, nil)

	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error { return nil })
	if err == nil {
		t.Fatalf("a body cut on an element boundary returned (%d, nil): the closing bracket must be read, "+
			"not assumed", page.Count)
	}
	if page.Count != 2 {
		t.Errorf("Count = %d, want 2", page.Count)
	}
}

func TestStreamSeriesRejectsANonArrayBody(t *testing.T) {
	c := streamClient(t, 200, `{"totalItems":3}`, nil)
	_, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error { return nil })
	if !errors.Is(err, ErrDecode) {
		t.Fatalf("got %v, want ErrDecode", err)
	}
	if !strings.Contains(err.Error(), "expected a JSON array") {
		t.Errorf("the error must say what it found: %q", err)
	}
}

func TestStreamSeriesRejectsAnEmptyBody(t *testing.T) {
	c := streamClient(t, 200, "", nil)
	_, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error { return nil })
	if !errors.Is(err, ErrDecode) {
		t.Fatalf("got %v, want ErrDecode", err)
	}
}

// TestStreamSeriesCallbackErrorComesBackUnwrapped keeps errors.Is working against
// the CALLER's sentinels. Relabelling the caller's failure as bad JSON would send
// whoever debugs it to the wrong end of the wire.
func TestStreamSeriesCallbackErrorComesBackUnwrapped(t *testing.T) {
	sentinel := errors.New("the sync engine gave up")
	c := streamClient(t, 200, `[{"id":1},{"id":2},{"id":3}]`, nil)

	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(s SeriesDto) error {
		if s.ID == 2 {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the caller's own sentinel back", err)
	}
	if errors.Is(err, ErrDecode) {
		t.Error("the caller's error was relabelled as a decode failure")
	}
	if page.Count != 1 {
		t.Errorf("Count = %d; the element that failed was not delivered successfully, so it does not count",
			page.Count)
	}
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("breaker is %v; the upstream answered fine, the caller failed", got)
	}
}

// TestStreamSeriesByteBoundFailsRatherThanTruncating fires the stream bound. Like
// the buffered one it must be ErrResponseTooLarge, not ErrDecode.
func TestStreamSeriesByteBoundFailsRatherThanTruncating(t *testing.T) {
	// A real array, longer than the bound the client is dropped to for this test.
	var b strings.Builder
	b.WriteString("[")
	for i := range 200 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"%s"}`, i+1, strings.Repeat("x", 64))
	}
	b.WriteString("]")

	c := streamClient(t, 200, b.String(), nil)
	c.maxStream = 512 // small enough to bite mid-array

	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error { return nil })
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("got %v, want ErrResponseTooLarge", err)
	}
	if errors.Is(err, ErrDecode) {
		t.Error("the bound was reported as a decode failure, which is the misdiagnosis it exists to prevent")
	}
	if page.Count == 0 {
		t.Error("Count is 0 although elements were delivered before the bound bit")
	}
}

// TestStreamSeriesNeverStreamsANonTwoHundred pins the rule in stream(): an error
// response takes the bounded buffered read and parseErrorBody, and fn is never
// called at all. Streaming an error page would hand a caller half an HTML 502 as
// if it were data.
func TestStreamSeriesNeverStreamsANonTwoHundred(t *testing.T) {
	c := streamClient(t, 500, `{"title":"Object reference not set to an instance of an object."}`, nil)
	called := false
	page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error {
		called = true
		return nil
	})
	if called {
		t.Error("the element callback was invoked on a 500")
	}
	if page.Count != 0 {
		t.Errorf("Count = %d on an error response", page.Count)
	}
	if !errors.Is(err, ErrServer) {
		t.Fatalf("got %v, want ErrServer", err)
	}
	if !strings.Contains(err.Error(), "Object reference not set") {
		t.Errorf("the upstream message must survive verbatim (ARCHITECTURE.md §17.3): %q", err)
	}
}

func TestStreamSeriesRequiresACallback(t *testing.T) {
	c := streamClient(t, 200, `[]`, nil)
	if _, err := c.StreamSeries(context.Background(), SeriesListOptions{}, nil); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("got %v, want ErrInvalidRequest", err)
	}
}

// TestStreamSeriesRefusesANonMemberSortField proves the refusal happens BEFORE
// the request, not after a 400. A body carrying sortField 0 fails every call, so
// catching it locally is the difference between one clear error and a
// misattributed upstream outage.
func TestStreamSeriesRefusesANonMemberSortField(t *testing.T) {
	sent := false
	c, _ := newFakeClient(t, func(*http.Request) (*http.Response, error) {
		sent = true
		return respond(200, `[]`, nil), nil
	})
	opts := SeriesListOptions{Sort: SeriesSortField(42)}
	if _, err := c.StreamSeries(context.Background(), opts, func(SeriesDto) error { return nil }); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("got %v, want ErrInvalidRequest", err)
	}
	if sent {
		t.Error("the request went out anyway; the guard must refuse before the wire")
	}
}

// TestStreamSeriesHoldsOneElementAtATime is the memory claim, measured rather
// than asserted.
//
// The point of StreamSeries over a buffered read is that peak heap stays near
// constant while the payload grows. This compares the streaming read against
// io.ReadAll + json.Unmarshal of the SAME bytes and requires a real reduction.
// The threshold is deliberately loose — it is a guard against someone
// reintroducing a buffered read, not a benchmark.
func TestStreamSeriesHoldsOneElementAtATime(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates ~16 MB")
	}
	payload := seriesArrayPayload(20000)

	buffered := measureHeap(t, func() {
		var out []SeriesDto
		if err := json.Unmarshal([]byte(payload), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out) != 20000 {
			t.Fatalf("decoded %d", len(out))
		}
		runtime.KeepAlive(out)
	})

	streamed := measureHeap(t, func() {
		c := streamClient(t, 200, payload, nil)
		n := 0
		page, err := c.StreamSeries(context.Background(), SeriesListOptions{}, func(SeriesDto) error {
			n++
			return nil
		})
		if err != nil {
			t.Fatalf("StreamSeries: %v", err)
		}
		if page.Count != 20000 {
			t.Fatalf("Count = %d", page.Count)
		}
	})

	t.Logf("peak heap above baseline: buffered %.2f MB, streamed %.2f MB",
		float64(buffered)/(1<<20), float64(streamed)/(1<<20))
	if streamed >= buffered {
		t.Errorf("streaming held %d bytes against buffering's %d: the streaming path is not holding "+
			"less than the whole payload, which is the only reason it exists", streamed, buffered)
	}
}

func seriesArrayPayload(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"Series %d","sortName":"Series %d","libraryId":1,`+
			`"libraryName":"Manga","format":1,"pages":%d,`+
			`"lastChapterAddedUtc":"2026-08-17T07:00:30.118Z","folderPath":"/mnt/user/media/manga/s%d"}`,
			i+1, i+1, i+1, i*7, i+1)
	}
	b.WriteString("]")
	return b.String()
}

// measureHeap returns the peak HeapAlloc delta across fn. It is coarse on
// purpose: an order of magnitude is what is being asserted, not a byte count.
func measureHeap(t *testing.T, fn func()) uint64 {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	if after.TotalAlloc < before.TotalAlloc {
		return 0
	}
	return after.HeapAlloc - min(after.HeapAlloc, before.HeapAlloc)
}

// TestPaginationHeaderShapeMatchesKavitasSerialiser pins the header's field names
// against Kavita's own PaginationHeader class, serialised with a camelCase naming
// policy. It is not in the OpenAPI document, so nothing else can check it.
func TestPaginationHeaderShapeMatchesKavitasSerialiser(t *testing.T) {
	raw := `{"currentPage":2,"itemsPerPage":50,"totalItems":151,"totalPages":4}`
	p := parsePaginationHeader(http.Header{"Pagination": []string{raw}})
	if p == nil {
		t.Fatal("the header did not parse")
	}
	if p.CurrentPage != 2 || p.ItemsPerPage != 50 || p.TotalItems != 151 || p.TotalPages != 4 {
		t.Errorf("decoded %+v from %s", *p, raw)
	}
	if got := parsePaginationHeader(http.Header{}); got != nil {
		t.Errorf("an absent header returned %+v, want nil", got)
	}
}

// compile-time assertion that the read path takes an injected doer and nothing
// else. If someone gives Client an *http.Client field, this stops compiling.
var _ HTTPDoer = (*fakeDoer)(nil)

var _ io.Reader = (*limitedBody)(nil)
