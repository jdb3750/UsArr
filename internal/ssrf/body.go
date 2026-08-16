package ssrf

import (
	"fmt"
	"io"
	"net/http"
	"net/netip"
)

// Response size caps. security.md §2.2 item 6 gives these as numbers rather than
// as "cap the response size", because an unnamed cap is a cap nobody sets.
const (
	// MaxArtworkBytes caps cover art and poster fetches.
	MaxArtworkBytes int64 = 20 << 20 // 20 MB

	// MaxMetadataBytes caps metadata JSON from providers.
	MaxMetadataBytes int64 = 32 << 20 // 32 MB

	// MaxListBytes caps the *Arr list endpoints, whose payloads run 30-80 MB on a
	// large library.
	MaxListBytes int64 = 200 << 20 // 200 MB
)

// cappedTransport enforces the byte cap. It fails rather than truncating: a
// truncated JSON body parses as a malformed document and gets diagnosed as an
// upstream bug, while a silently truncated *Arr list looks like content that was
// deleted upstream and would drive a destructive sweep.
type cappedTransport struct {
	base  http.RoundTripper
	class Class
	max   int64
}

func (t *cappedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The scheme check lives here rather than only in ValidateURL so that a
	// file:, gopher:, dict: or ftp: URL is refused by the client itself, with a
	// typed error, however it reached the caller.
	if err := CheckScheme(req.URL.Scheme); err != nil {
		return nil, policyErr("validate", t.class, req.URL.Host, netip.Addr{}, err)
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if t.max <= 0 {
		return resp, nil
	}
	// A declared Content-Length lets us refuse before reading a single byte.
	if resp.ContentLength > t.max {
		_ = resp.Body.Close()
		return nil, policyErr("body", t.class, req.URL.Host, netip.Addr{},
			fmt.Errorf("%w: content-length %d > %d", ErrResponseTooLarge, resp.ContentLength, t.max))
	}
	resp.Body = &cappedBody{rc: resp.Body, max: t.max, class: t.class, host: req.URL.Host}
	return resp, nil
}

type cappedBody struct {
	rc    io.ReadCloser
	n     int64
	max   int64
	class Class
	host  string
}

func (b *cappedBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	b.n += int64(n)
	if b.n > b.max {
		return 0, policyErr("body", b.class, b.host, netip.Addr{},
			fmt.Errorf("%w: read %d > %d bytes", ErrResponseTooLarge, b.n, b.max))
	}
	return n, err
}

func (b *cappedBody) Close() error { return b.rc.Close() }
