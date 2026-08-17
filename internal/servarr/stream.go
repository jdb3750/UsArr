package servarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// MaxStreamBytes bounds one streamed list body.
//
// It is the same 200 MB as internal/ssrf.MaxListBytes, and deliberately so: that
// transport is the ceiling every request already passes through, and a client
// bound below it would reimpose exactly the failure this replaced — a 32 MB cap
// under a layer that permits 200 MB, silently truncating a 10k-movie
// GET /api/v3/movie (30-80 MB per docs/reference/sync.md §2).
//
// The value is DUPLICATED rather than imported because this package must not
// depend on internal/ssrf (see doc.go: the SSRF policy client is injected, and
// this package must not be able to reach around it). TestMaxStreamBytesTracksThe
// SSRFCeiling pins the two together so the duplication cannot drift.
const MaxStreamBytes int64 = 200 << 20

// errBodyTooLarge is limitedBody's internal signal. It never escapes: both read
// paths translate it into an *APIError wrapping ErrResponseTooLarge.
var errBodyTooLarge = errors.New("servarr: response body exceeded the limit")

// callbackError carries an error the CALLER's element function returned, so the
// classifier can tell it apart from a decode failure and hand it back verbatim
// instead of relabelling somebody else's error as bad JSON.
type callbackError struct{ err error }

func (e *callbackError) Error() string { return e.err.Error() }
func (e *callbackError) Unwrap() error { return e.err }

// StreamList reads one bare-array *Arr list endpoint and hands fn each element as
// it decodes, holding at most one element plus the decoder's window in memory.
//
// path is relative to the client's API path, so "/movie" is
// GET {base}/api/v3/movie. Only the list endpoints belong here; the small
// fixed-shape ones stay on the buffered path.
//
// docs/reference/sync.md §2, verbatim: "Stream the JSON. json.Decoder.Token()
// consuming the array element by element. Buffering *and* unmarshalling a 60 MB
// payload peaks at ~200-400 MB on a 1 GB Pi; streaming holds it near constant."
//
// # The partial-delivery contract
//
// StreamList returns the number of elements ALREADY HANDED TO fn, on success and
// on failure alike. A stream that dies mid-array — a truncated body, a reset
// connection, a deadline — returns (n, err) with n > 0, and those n calls to fn
// happened: their effects stand. StreamList does not and cannot undo them, so a
// caller whose fn writes must make its own writes recoverable (the sync engine's
// chunked transactions, sync.md §2) and must treat n as "how far the import got",
// never as "how many rows are correct". Nothing is applied silently: the error is
// always returned, never swallowed, and n is always reported next to it.
//
// An error fn itself returns aborts the stream and comes back UNWRAPPED, so
// errors.Is against the caller's own sentinels works.
func StreamList[T any](ctx context.Context, c *Client, path string, query url.Values, fn func(T) error) (int, error) {
	const op = "StreamList"
	full := c.apiPath + path

	if fn == nil {
		return 0, &APIError{Op: op, Method: http.MethodGet, Path: full, Message: "element function is required", Err: ErrInvalidRequest}
	}

	var n int
	err := c.stream(ctx, request{
		op: op, method: http.MethodGet, path: full,
		query: query, timeout: c.timeouts.List,
	}, func(dec *json.Decoder) error {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return &APIError{Op: op, Method: http.MethodGet, Path: full, Message: "empty body", Err: ErrDecode}
			}
			return err
		}
		if d, ok := tok.(json.Delim); !ok || d != '[' {
			return &APIError{
				Op: op, Method: http.MethodGet, Path: full,
				Message: fmt.Sprintf("expected a JSON array, got %v", tok), Err: ErrDecode,
			}
		}
		truncated := func() error {
			return &APIError{
				Op: op, Method: http.MethodGet, Path: full,
				Message: fmt.Sprintf("stream ended mid-array after %d elements; those %d were delivered", n, n),
				Err:     ErrDecode,
			}
		}
		for dec.More() {
			var v T
			if err := dec.Decode(&v); err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					return truncated()
				}
				return err
			}
			if err := fn(v); err != nil {
				return &callbackError{err: err}
			}
			n++
		}
		// The closing bracket is read rather than assumed. dec.More() answers false
		// at a read error just as it does at ']', so WITHOUT this a body cut exactly
		// on an element boundary would return (n, nil) — a half payload applied
		// silently and reported as a complete list. TestStreamListTruncatedMidArray
		// ReportsHowFarItGot is that case.
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return truncated()
			}
			return err
		}
		return nil
	})
	return n, err
}

// stream performs one request and hands a *json.Decoder over the response body to
// fn. It is do's sibling and does everything do does — breaker gate, deadline,
// X-Api-Key and Accept headers, X-Application-Version capture, the error taxonomy,
// the 4xx/5xx breaker policy, URL-free errors — with one difference: on 2xx the
// body is never buffered.
//
// A NON-2xx RESPONSE IS NEVER STREAMED. It takes the bounded buffered read and
// parseErrorBody, exactly as do does, and fn is not called at all.
func (c *Client) stream(ctx context.Context, r request, fn func(*json.Decoder) error) error {
	if err := c.breaker.Allow(); err != nil {
		return &APIError{Op: r.op, Method: r.method, Path: r.path, Err: err}
	}

	timeout := r.timeout
	if timeout <= 0 {
		timeout = c.timeouts.Default
	}
	// The deadline covers the whole body, not just the headers: the context governs
	// every Read on resp.Body. That is why streamed reads get their own, much
	// larger budget (Timeouts.List).
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
		// Bounded drain, same as do. On an aborted 200 MB stream this reads 1 MB and
		// gives up rather than paying for the rest to reuse a connection.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	if v := resp.Header.Get("X-Application-Version"); v != "" {
		c.appVersion.Store(&v)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The error path stays buffered and bounded. Error envelopes are small, they
		// have to be parsed whole to be classified, and a 401 must not be parsed at
		// all — parseErrorBody owns all three rules.
		body, err := io.ReadAll(&limitedBody{r: resp.Body, max: maxBufferedBody})
		if err != nil {
			body = nil // a body we could not read is a body with nothing to say
		}
		apiErr := parseErrorBody(r.op, r.method, r.path, resp.StatusCode, body)
		c.recordStatus(resp.StatusCode)
		return apiErr
	}

	lb := &limitedBody{r: resp.Body, max: c.maxStream}
	err = fn(json.NewDecoder(lb))

	var ce *callbackError
	switch {
	case err == nil:
		c.recordStatus(resp.StatusCode)
		return nil

	case errors.As(err, &ce):
		// Checked FIRST, and before lb.lastErr: a Read may hand json a final chunk
		// AND an error together, so a caller's error on the last element must not be
		// relabelled as upstream flakiness. The upstream answered; the caller failed.
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
	// Whatever encoding/json said about the bytes. Its message quotes the offending
	// token, never a URL or a header, so it is safe to carry — but it is truncated
	// anyway, because a 2 MB HTML error page from a reverse proxy would otherwise
	// arrive here as one enormous error string.
	return &APIError{
		Op: r.op, Method: r.method, Path: r.path, Status: resp.StatusCode,
		Message: "decoding response: " + truncate(err.Error()), Err: ErrDecode,
	}
}
