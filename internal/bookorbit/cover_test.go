package bookorbit

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// pngBytes is a real 74-byte 2×2 PNG, the same bytes the
// bookorbit_book_cover.yaml cassette carries under !!binary.
//
// A REAL image rather than a string of ASCII, because the subject of
// Client.Cover is that bytes survive it intact — 0x89 in the signature and the
// zlib payload in IDAT are both invalid UTF-8, which is exactly what a body path
// that quietly went through a string coercion would mangle.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x02, 0x08, 0x02, 0x00, 0x00, 0x00, 0xfd, 0xd4, 0x9a,
	0x73, 0x00, 0x00, 0x00, 0x11, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xb8, 0xa1, 0x6e, 0x2f,
	0xa8, 0x64, 0xcc, 0x00, 0xa1, 0x00, 0x1a, 0x7c, 0x03, 0x49, 0xbe, 0xf9, 0x30, 0x80, 0x00, 0x00,
	0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// ─── the contract: a status is not an error ──────────────────────────────────

// TestACoverlessBookIsA404AndNotAnError IS THE POINT OF THIS METHOD.
//
// The caller — the cover-image pipeline's writer — has to tell "this book has no
// cover", which it caches as permanently absent, from "the fetch failed", which
// it retries. Those two decisions are opposite and irreversible in different
// directions, and only the status separates them. A method that returned
// ErrNotFound here would look correct, satisfy "it reports 404s", and have
// destroyed the distinction.
//
// So: nil error, Status 404, and the upstream message still in hand.
func TestACoverlessBookIsA404AndNotAnError(t *testing.T) {
	f := newFake(t)
	// No entry for book 7 — which is what a book whose cover directory holds no
	// usable file looks like upstream (BookService.getCoverPath returns null,
	// BookController.getCover :344 throws NotFoundException on it).
	c := f.client(t)

	cov, err := c.Cover(context.Background(), 7)
	if err != nil {
		t.Fatalf("a coverless book came back as an ERROR (%v). It must come back as a STATUS: "+
			"the caller caches absence and retries failure, and collapsing the two loses the "+
			"only signal that tells them apart", err)
	}
	if cov.Status != http.StatusNotFound {
		t.Fatalf("Status = %d, want 404", cov.Status)
	}
	if !strings.Contains(string(cov.Body), "No cover for book 7") {
		t.Errorf("the upstream message did not survive into Body: %q", cov.Body)
	}
	// And the negative control: the error taxonomy was not reached at all.
	if errors.Is(err, ErrNotFound) {
		t.Error("err unwraps to ErrNotFound; the 404 was classified after all")
	}
}

// TestAServerErrorIsAlsoAStatusRatherThanAnError is the same contract on the
// other side of the split. A 500 is retryable and a 404 is not, and this method
// classifies neither — deciding which statuses deserve an error is the caller's
// job, and a method that erred on 5xx but not 4xx would have made half of it.
func TestAServerErrorIsAlsoAStatusRatherThanAnError(t *testing.T) {
	f := newFake(t)
	f.coverHandler = func(w http.ResponseWriter, r *http.Request) {
		nestErrorText(w, r, http.StatusInternalServerError, "boom")
	}
	c := f.client(t)

	cov, err := c.Cover(context.Background(), 7)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if cov.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want 500", cov.Status)
	}
	if !strings.Contains(string(cov.Body), "boom") {
		t.Errorf("the upstream message did not survive into Body: %q", cov.Body)
	}
}

// TestATransportFailureIsAnErrorAndCarriesNoStatus is the other half of the
// same contract: err is for what happened BEFORE a status existed. Without this
// the "404 is not an error" rule could be satisfied by a method that never
// returns an error at all.
func TestATransportFailureIsAnErrorAndCarriesNoStatus(t *testing.T) {
	errBoom := errors.New("dial tcp: connection refused")
	f := newFake(t)
	c := f.client(t, func(o *Options) {
		o.HTTPClient = doerFunc(func(*http.Request) (*http.Response, error) { return nil, errBoom })
	})

	cov, err := c.Cover(context.Background(), 7)
	if err == nil {
		t.Fatal("a dead host came back as a nil error; the caller would cache a failure as absence")
	}
	if cov.Status != 0 {
		t.Errorf("Status = %d on a failure that never got one, want 0", cov.Status)
	}
	if len(cov.Body) != 0 {
		t.Errorf("Body = %q on a transport failure, want empty", cov.Body)
	}
}

// ─── the bytes ───────────────────────────────────────────────────────────────

func TestCoverReturnsTheBytesIntactAndTheUpstreamContentType(t *testing.T) {
	f := newFake(t)
	// image/webp rather than the image/jpeg fallback, so a method that hard-coded
	// a type from ADR-0050's encoder rather than reading the header would fail.
	f.covers = map[int64]fakeCover{101: {contentType: "image/webp", body: pngBytes}}
	c := f.client(t)

	cov, err := c.Cover(context.Background(), 101)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if cov.Status != http.StatusOK {
		t.Fatalf("Status = %d, want 200", cov.Status)
	}
	if !bytes.Equal(cov.Body, pngBytes) {
		t.Errorf("Body = % x, want % x", cov.Body, pngBytes)
	}
	if cov.ContentType != "image/webp" {
		t.Errorf("ContentType = %q; upstream said image/webp and this is a passthrough hint, "+
			"not a normalised or validated value", cov.ContentType)
	}
}

// TestCoverAsksAnImageRouteForAnImage. Nothing upstream negotiates on Accept
// today — Fastify's reply.send(stream) ignores it — so this is about the claim
// the request makes rather than about a failure it avoids. The day a Nest
// interceptor does start negotiating, `Accept: application/json` on a route that
// answers a PNG is a 406 nobody would look for here.
func TestCoverAsksAnImageRouteForAnImage(t *testing.T) {
	f := newFake(t)
	f.covers = map[int64]fakeCover{101: {contentType: "image/png", body: pngBytes}}
	seen := map[string]string{}
	f.coverHandler = func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("Accept")
		f.defaultCover(w, r)
	}
	f.appInfoHandler = func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("Accept")
		f.defaultAppInfo(w, r)
	}
	c := f.client(t)

	if _, err := c.Cover(context.Background(), 101); err != nil {
		t.Fatalf("Cover: %v", err)
	}
	// The control. Without it a pass would show only that ONE header changed —
	// which setting the package default to image/* would also achieve, and that
	// would break every JSON route silently.
	if _, err := c.AppInfo(context.Background()); err != nil {
		t.Fatalf("AppInfo: %v", err)
	}

	if got := seen[apiPrefix+"/books/101/cover"]; got != coverAccept {
		t.Errorf("the cover route was asked with Accept %q, want %q", got, coverAccept)
	}
	if got := seen[apiPrefix+"/app-info"]; got != "application/json" {
		t.Errorf("the JSON default did not survive: app-info was asked with Accept %q", got)
	}
}

// ─── §14: nothing goes in the URL ────────────────────────────────────────────

// TestCoverPutsNoCredentialAndNoCacheBusterInTheURL.
//
// This is ADR-0052's constraint applied to the one route in this package that
// has a query parameter at all. `?t=` is NOT a token — it selects a
// Cache-Control value (book.controller.ts:348) — and the HMAC-token-in-a-URL
// shape lives only in the OPDS guard, which this adapter does not touch. The
// assertion is therefore blunt on purpose: NO query string, and neither secret
// anywhere in the URL.
func TestCoverPutsNoCredentialAndNoCacheBusterInTheURL(t *testing.T) {
	f := newFake(t)
	f.covers = map[int64]fakeCover{101: {contentType: "image/jpeg", body: pngBytes}}
	c := f.client(t)

	if _, err := c.Cover(context.Background(), 101); err != nil {
		t.Fatalf("Cover: %v", err)
	}

	var sawCover bool
	jwt := f.latestToken()
	for _, r := range f.requests() {
		if !strings.HasPrefix(r.Path, apiPrefix+"/") {
			t.Errorf("%s %s is outside %s, the prefix ADR-0052 constrains this adapter to",
				r.Method, r.Path, apiPrefix)
		}
		if r.Query != "" {
			t.Errorf("%s %s carried a query string (%q). The cover route accepts ?t=, and it is a "+
				"cache-buster selecting a Cache-Control value rather than a token — UsArr has no "+
				"value for it and must not invent one", r.Method, r.Path, r.Query)
		}
		assertNoCredential(t, "the request path "+r.Path, r.Path, jwt)
		assertNoCredential(t, "the query string of "+r.Path, r.Query, jwt)
		if r.Path == apiPrefix+"/books/101/cover" {
			sawCover = true
			if r.Auth != authScheme+jwt {
				t.Errorf("the cover request's Authorization was %q; the bearer is the ONLY place "+
					"the credential may travel", RedactURL(r.Auth))
			}
		}
	}
	if !sawCover {
		t.Fatal("no cover request was recorded — the assertions above scanned nothing")
	}
}

// TestCoverRefusesANonPositiveBookID. Upstream's ParseIntPipe accepts a negative
// id and the row will not exist, so this would come back as a 404 — the one
// status the caller records as PERMANENTLY ABSENT. A caller's own bug would be
// written down as a fact about a book.
func TestCoverRefusesANonPositiveBookID(t *testing.T) {
	f := newFake(t)
	c := f.client(t)

	for _, id := range []int64{0, -1} {
		cov, err := c.Cover(context.Background(), id)
		if !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("Cover(%d) err = %v, want ErrInvalidRequest", id, err)
		}
		if cov.Status != 0 {
			t.Errorf("Cover(%d) Status = %d, want 0 — no request was made", id, cov.Status)
		}
	}
	if n := len(f.requests()); n != 0 {
		t.Errorf("%d request(s) reached the fake; a locally invalid id must not leave the process", n)
	}
}

// ─── the cassettes ───────────────────────────────────────────────────────────

func TestCoverFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_book_cover")

	cov, err := c.Cover(context.Background(), 101)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if cov.Status != http.StatusOK {
		t.Fatalf("Status = %d, want 200", cov.Status)
	}
	if cov.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", cov.ContentType)
	}
	// The recorded body is a real PNG, carried through YAML's !!binary tag. If it
	// arrives equal to the literal above, non-UTF-8 bytes survive the whole
	// cassette round trip AND this method.
	if !bytes.Equal(cov.Body, pngBytes) {
		t.Errorf("Body = % x (%d bytes), want the recorded PNG (% d bytes)",
			cov.Body, len(cov.Body), len(pngBytes))
	}
}

// TestCoverAbsentFromCassette replays the answer to the question
// docs/REVIEW-LOG.md's LS-260 left open, re-asked against BookOrbit: a book with
// no cover file answers 404 and NOT a 200 carrying a placeholder image.
//
// 🔍 The fixture is authored from BookController.getCover :344, whose whole
// no-cover path is `if (!coverPath) throw new NotFoundException(…)`. That is a
// read of the controller, not a probe of a running one.
func TestCoverAbsentFromCassette(t *testing.T) {
	c := newCassetteClient(t, "bookorbit_book_cover_absent")

	cov, err := c.Cover(context.Background(), 404)
	if err != nil {
		t.Fatalf("the absent-cover fixture came back as an error: %v", err)
	}
	if cov.Status != http.StatusNotFound {
		t.Fatalf("Status = %d, want 404", cov.Status)
	}
	// NOT an image. The negative assertion is the one that matters: were BookOrbit
	// to answer 200 with a placeholder, a caller would cache a grey rectangle as a
	// real cover and nothing in the response would say otherwise.
	if strings.HasPrefix(cov.ContentType, "image/") {
		t.Errorf("ContentType = %q on a 404 — that would be a placeholder image, which is the "+
			"one shape this fixture exists to rule out", cov.ContentType)
	}
	if !strings.Contains(string(cov.Body), "No cover for book 404") {
		t.Errorf("the NotFoundException message did not survive into Body: %q", cov.Body)
	}
}
