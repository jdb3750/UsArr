package kavita

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// MaxStreamBytes bounds one streamed list body.
//
// It is the same 200 MB as internal/ssrf.MaxListBytes, and deliberately so: that
// transport is the ceiling every request already passes through, and a client
// bound below it would silently truncate a body the layer beneath it permits.
//
// The value is DUPLICATED rather than imported because this package must not
// depend on internal/ssrf (see doc.go: the SSRF policy client is injected, and
// this package must not be able to reach around it).
// TestMaxStreamBytesTracksTheSSRFCeiling pins the two together so the duplication
// cannot drift.
const MaxStreamBytes int64 = 200 << 20

// errBodyTooLarge is limitedBody's internal signal. It never escapes: both read
// paths translate it into an *APIError wrapping ErrResponseTooLarge.
var errBodyTooLarge = errors.New("kavita: response body exceeded the limit")

// callbackError carries an error the CALLER's element function returned, so the
// classifier can tell it apart from a decode failure and hand it back verbatim
// instead of relabelling somebody else's error as bad JSON.
type callbackError struct{ err error }

func (e *callbackError) Error() string { return e.err.Error() }
func (e *callbackError) Unwrap() error { return e.err }

// limitedBody bounds a response body and FAILS at the bound instead of
// truncating. io.LimitReader reports a clean io.EOF at its limit, which is
// indistinguishable from a body that genuinely ended.
//
// It also remembers the underlying reader's last error, which is what lets the
// streaming path tell "the connection died mid-array" (a transport failure, the
// breaker's business) from "the server sent malformed JSON" (a decode failure).
type limitedBody struct {
	r   io.Reader
	n   int64
	max int64

	exceeded bool
	lastErr  error // last non-EOF error from r
}

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.exceeded {
		return 0, errBodyTooLarge
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	if err != nil && !errors.Is(err, io.EOF) {
		l.lastErr = err
	}
	if l.max > 0 && l.n > l.max {
		l.exceeded = true
		// Drop the overshooting bytes: the read is fatal, so nothing above will
		// look at them, and returning them alongside the error invites a caller to
		// parse a body it has already been told is over the limit.
		return 0, errBodyTooLarge
	}
	return n, err
}

// SeriesPage is what a streamed read reports about itself once the body is done.
type SeriesPage struct {
	// Count is how many SeriesDto values were handed to the callback. It is
	// reported on failure too — see the partial-delivery contract on StreamSeries.
	Count int

	// Pagination is Kavita's `Pagination` response header, decoded, or nil when
	// the header was absent or unparseable.
	//
	// It is NOT in the OpenAPI document (it is middleware — see the Pagination
	// type), so it is read opportunistically and never required. A nil here means
	// "the total is unknown", which callers must render as unknown rather than as
	// zero.
	Pagination *Pagination
}

// StreamSeries reads POST /api/Series/all-v2 and hands fn each SeriesDto as it
// decodes, holding at most one element plus the decoder's window in memory.
//
// WHY THIS ENDPOINT IS NEVER BUFFERED: UserParams.PageSize defaults to
// int.MaxValue and coerces an explicit 0 to int.MaxValue as well, so a call with
// no paging returns the WHOLE library in ONE response. ARCHITECTURE.md §7.2 is
// "Stream the JSON; never io.ReadAll" for exactly this shape.
//
// # The partial-delivery contract
//
// StreamSeries returns SeriesPage.Count = the number of elements ALREADY HANDED
// TO fn, on success and on failure alike. A stream that dies mid-array — a
// truncated body, a reset connection, a deadline — returns a non-zero Count
// alongside the error, and those calls to fn happened: their effects stand. A
// caller whose fn writes must make its own writes recoverable and must treat
// Count as "how far the read got", never as "how many rows are correct".
//
// An error fn itself returns aborts the stream and comes back UNWRAPPED, so
// errors.Is against the caller's own sentinels works.
func (c *Client) StreamSeries(ctx context.Context, opts SeriesListOptions, fn func(SeriesDto) error) (SeriesPage, error) {
	const op = "SeriesList"
	path := apiPrefix + "/Series/all-v2"

	var page SeriesPage
	if fn == nil {
		return page, &APIError{Op: op, Method: http.MethodPost, Path: path, Message: "element function is required", Err: ErrInvalidRequest}
	}
	resolved, err := opts.resolve()
	if err != nil {
		return page, &APIError{Op: op, Method: http.MethodPost, Path: path, Message: err.Error(), Err: ErrInvalidRequest}
	}
	filter, err := NewSeriesFilter(resolved.Sort, resolved.Ascending)
	if err != nil {
		return page, &APIError{Op: op, Method: http.MethodPost, Path: path, Message: err.Error(), Err: ErrInvalidRequest}
	}

	err = c.stream(ctx, request{
		op: op, method: http.MethodPost, path: path,
		query: resolved.values(), body: filter, timeout: c.timeouts.List,
	}, func(hdr http.Header, dec *json.Decoder) error {
		page.Pagination = parsePaginationHeader(hdr)

		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return &APIError{Op: op, Method: http.MethodPost, Path: path, Message: "empty body", Err: ErrDecode}
			}
			return err
		}
		if d, ok := tok.(json.Delim); !ok || d != '[' {
			return &APIError{
				Op: op, Method: http.MethodPost, Path: path,
				Message: fmt.Sprintf("expected a JSON array, got %v", tok), Err: ErrDecode,
			}
		}
		truncated := func() error {
			return &APIError{
				Op: op, Method: http.MethodPost, Path: path,
				Message: fmt.Sprintf("stream ended mid-array after %d elements; those %d were delivered", page.Count, page.Count),
				Err:     ErrDecode,
			}
		}
		for dec.More() {
			var v SeriesDto
			if err := dec.Decode(&v); err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					return truncated()
				}
				return err
			}
			if err := fn(v); err != nil {
				return &callbackError{err: err}
			}
			page.Count++
		}
		// The closing bracket is read rather than assumed. dec.More() answers false
		// at a read error just as it does at ']', so WITHOUT this a body cut exactly
		// on an element boundary would return a nil error — a half payload applied
		// silently and reported as a complete list.
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return truncated()
			}
			return err
		}
		return nil
	})
	return page, err
}

// parsePaginationHeader decodes the undocumented `Pagination` response header.
//
// Absent or unparseable returns nil, never an error: the header is middleware, it
// is not in the OpenAPI document, and a reverse proxy that strips unknown headers
// must not be able to fail a read that succeeded.
func parsePaginationHeader(h http.Header) *Pagination {
	raw := h.Get("Pagination")
	if raw == "" {
		return nil
	}
	var p Pagination
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil
	}
	return &p
}

// stream performs one request and hands the response headers plus a
// *json.Decoder over the body to fn. It is do's sibling and does everything do
// does — breaker gate, deadline, Auth Key and Accept headers, the error taxonomy,
// the 4xx/5xx breaker policy, URL-free errors — with one difference: on 2xx the
// body is never buffered.
//
// A NON-2xx RESPONSE IS NEVER STREAMED. It takes the bounded buffered read and
// parseErrorBody, exactly as do does, and fn is not called at all.
func (c *Client) stream(ctx context.Context, r request, fn func(http.Header, *json.Decoder) error) error {
	if err := c.breaker.Allow(); err != nil {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Err: err}
	}

	timeout := r.timeout
	if timeout <= 0 {
		timeout = c.timeouts.Default
	}
	// The deadline covers the whole body, not just the headers: the context
	// governs every Read on resp.Body. That is why streamed reads get their own,
	// much larger budget (Timeouts.List).
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := c.newRequest(ctx, r)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return c.transportError(r, err)
	}
	defer func() {
		// Bounded drain, same as do. On an aborted 200 MB stream this reads 1 MB
		// and gives up rather than paying for the rest to reuse a connection.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The error path stays buffered and bounded. Error envelopes are small,
		// they have to be parsed whole to be classified, and a 401 must not be
		// parsed at all — parseErrorBody owns all three rules.
		body, err := io.ReadAll(&limitedBody{r: resp.Body, max: maxBufferedBody})
		if err != nil {
			body = nil // a body we could not read is a body with nothing to say
		}
		apiErr := parseErrorBody(r.op, r.method, r.path, resp.StatusCode, body)
		c.recordStatus(resp.StatusCode)
		return apiErr
	}

	lb := &limitedBody{r: resp.Body, max: c.maxStream}
	err = fn(resp.Header, json.NewDecoder(lb))

	var ce *callbackError
	switch {
	case err == nil:
		c.recordStatus(resp.StatusCode)
		return nil

	case errors.As(err, &ce):
		// Checked FIRST, and before lb.lastErr: a Read may hand json a final chunk
		// AND an error together, so a caller's error on the last element must not
		// be relabelled as upstream flakiness. The upstream answered; the caller
		// failed.
		c.recordStatus(resp.StatusCode)
		return ce.err

	case lb.exceeded:
		c.recordStatus(resp.StatusCode)
		return &APIError{
			Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode,
			Message: fmt.Sprintf("response exceeded the %d-byte stream limit", c.maxStream),
			Err:     ErrResponseTooLarge,
		}

	case lb.lastErr != nil:
		// The connection died mid-array. That is upstream flakiness and the
		// breaker's business, and transportError is what keeps the *url.Error — and
		// the credential-bearing URL inside it — out of the returned error.
		return c.transportError(r, lb.lastErr)
	}

	// Past here the read succeeded, so nothing that follows is evidence about
	// instance health: the instance answered.
	c.recordStatus(resp.StatusCode)

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == 0 {
			apiErr.Status = resp.StatusCode
		}
		return apiErr
	}
	// Whatever encoding/json said about the bytes. Its message quotes the
	// offending token, never a URL or a header, so it is safe to carry — but it is
	// truncated anyway, because a 2 MB HTML error page from a reverse proxy would
	// otherwise arrive here as one enormous error string.
	return &APIError{
		Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode,
		Message: "decoding response: " + truncate(err.Error()), Err: ErrDecode,
	}
}
