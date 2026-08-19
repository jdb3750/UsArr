package bookorbit

import (
	"context"
	"net/http"
	"strconv"
)

// The cover fetch.
//
// Verified from source at bookorbit/bookorbit@73b7877d2fede2221b0ca360af9bfced7c3797f3
// — book.controller.ts (BookController.getCover, :335-360), book.service.ts
// (BookService.getCoverPath, :1253-1271; verifyBookAccess, :489-498),
// common/book-cover-storage.ts (findPreferredBookCoverFileName, :24-26),
// common/image-content-type.ts.
//
// ONE ROUTE, one read, under the /api/v1 prefix ADR-0052 constrains this adapter
// to. GET /books/:id/cover declares no @RequirePermission — the nearest ones in
// the controller are on routes above and below it — so the shared account with
// an EMPTY permission set that scope.go argues for reaches it, exactly as the
// catalogue routes do.
//
// WHAT IS STILL NOT HERE: GET /books/:id/thumbnail. It is the same shape one
// path segment along and it is left out because nothing asks for it — the writer
// this method exists for re-encodes to its own dimensions from the full cover
// (ADR-0050), so a second fetch of a smaller JPEG buys nothing. catalogue.go's
// exclusion list still names it.

// coverAccept is what this client asks an image route for.
//
// It is `image/*` rather than application/json for the reason request.accept
// exists, and it is a WILDCARD rather than a list because BookOrbit picks the
// type from the stored file's extension — imageContentTypeFromPath maps .jpg,
// .jpeg, .png, .gif, .webp, .bmp and .avif, and falls back to image/jpeg for
// anything else — so a client that enumerated types would be asserting a closed
// set that upstream's own fallback already contradicts.
const coverAccept = "image/*"

// Cover is one book's cover image as BookOrbit answered it: the status, the
// bytes, and the type BookOrbit claimed they are.
//
// 🚩 IT IS RETURNED FOR EVERY ANSWERED STATUS, NOT ONLY FOR 200, and that is the
// whole point of the type. See Client.Cover.
type Cover struct {
	// Status is the HTTP status BookOrbit answered with, VERBATIM and
	// unclassified. It is meaningful whenever Client.Cover returned a nil error,
	// which includes every 4xx and 5xx.
	Status int

	// ContentType is the response's Content-Type header, carried as A HINT AND
	// NOTHING MORE. It is neither parsed, normalised into a known set, nor
	// validated as an image type: the caller decodes the bytes and its decoder
	// decides the real format, which is the only thing that can — BookOrbit
	// derives this header from the stored FILE EXTENSION
	// (imageContentTypeFromPath) and falls back to image/jpeg for an extension it
	// does not know, so it is upstream's guess about its own disk rather than a
	// measurement of the bytes below.
	//
	// It goes through text() because that is this package's boundary rule for
	// every piece of upstream prose (catalogue.go), not because the value is
	// being interpreted. Empty means BookOrbit sent no Content-Type.
	ContentType string

	// Body is the response body, whole and untouched.
	//
	// On a 200 it is the image. On a non-2xx it is usually GlobalExceptionFilter's
	// JSON envelope, and it is handed over rather than discarded so a caller that
	// wants the upstream message for a log line has it without a second request.
	// It is buffered rather than streamed on purpose: a cover is image-sized, the
	// 32 MiB bound in roundTrip covers it with room to spare, and a reader would
	// hand the caller a lifetime to manage for no gain.
	Body []byte
}

// Cover calls GET /api/v1/books/{id}/cover — the cover-image fetch, returning
// BYTES.
//
// # A STATUS IS NOT AN ERROR HERE, AND THAT IS THE CONTRACT
//
// err is non-nil ONLY for a failure that happened BEFORE a status existed: a
// transport failure, an SSRF-policy refusal, an open breaker, a deadline, or a
// mint that could not produce a bearer. doRaw's own contract is that same
// sentence, and this method deliberately does not add the status classification
// do() applies — it passes doRaw's answer straight out.
//
// SO A 404 COMES BACK AS Cover{Status: 404} WITH A NIL ERROR. That is not
// laxity, it is the distinction the caller is built on: "this book has no cover"
// is a fact to be cached as permanently absent, while "the fetch failed" is a
// thing to retry, and a method that collapsed the first into ErrNotFound would
// have destroyed the difference at the only layer that can still see it. A 500
// comes back the same way, with a nil error and Status 500, because the
// alternative is this method deciding which statuses deserve an error and that
// decision is the caller's.
//
// ⚠️ A 404 IS OVERLOADED UPSTREAM AND MUST NOT BE READ AS "NO COVER FILE" ALONE.
// Four distinct conditions produce one:
//
//   - the book exists and has no cover file — getCoverPath returns null when the
//     cover directory is missing (isMissingFilesystemEntry) or when
//     findPreferredBookCoverFileName finds no custom, extracted or legacy file in
//     it, and the controller throws NotFoundException on that null;
//   - the book id does not exist — verifyBookAccess throws NotFoundException when
//     findLibraryIdByBookId returns null;
//   - the book exists but fails the credential's content filters — the same
//     NotFoundException, deliberately, so a filtered book is indistinguishable
//     from an absent one;
//   - and BookOrbit's catch-all for a path that routes nowhere.
//
// All three of the first are "do not ask again for this book", so caching
// absence on a 404 is sound; treating a 404 as evidence about the FILE is not.
//
// A 403 is a fifth condition and is separate: verifyUserAccess throws
// ForbiddenException when the account has no access to the book's library, which
// is a credential-scope problem rather than a missing image.
//
// # What BookOrbit does NOT do: there is no placeholder
//
// MEASURED FROM SOURCE at 73b7877d, not assumed: getCover's whole no-cover path
// is `if (!coverPath) throw new NotFoundException(...)` — book.controller.ts:344.
// There is no default asset, no generated tint and no fallback image anywhere in
// that handler or in getCoverPath, so a coverless book answers 404 AND NEVER A
// 200 CARRYING A GREY RECTANGLE. That mattered enough to gate the work:
// docs/REVIEW-LOG.md's LS-260 left the question open against Kavita, and a 200
// placeholder would mean a caller caching a placeholder as a real cover with
// nothing in the response to tell it apart.
//
// 🔍 THAT IS A READ OF THE CONTROLLER, NOT A PROBE OF A RUNNING ONE. ADR-0052
// records that this project has never contacted a live BookOrbit, so this
// inherits vcr_test.go's standing caveat: the source says what the handler does,
// and only a live instance can say what a deployment in front of it does. A
// reverse proxy with its own error page is the failure mode this cannot see.
//
// # No credential and no cache-buster reach the URL
//
// The path is built from the book id against the configured base URL and carries
// NO query string at all. The bearer travels in the Authorization header, as it
// does for every route in this package (doc.go).
//
// The `?t=` the route accepts is NOT sent, and it is worth naming because it is
// the thing that LOOKS like a credential in a URL and is not: `const cacheControl
// = t ? 'public, max-age=31536000, immutable' : 'private, max-age=86400'`
// (book.controller.ts:348) — it selects a Cache-Control value and is read by
// nothing else. UsArr has no cache-buster value to supply and fetches once into
// its own store, so sending one would be inventing a parameter to get a header
// it does not read.
//
// The Book projection carries no cover URL or path field — BookCard has none — so
// nothing here is harvested from a response body. This is SSRF class 1,
// admin-configured, and the injected HTTPDoer's resolve-then-pin is the whole of
// the defence, exactly as for every other route here.
func (c *Client) Cover(ctx context.Context, bookID int64) (Cover, error) {
	const op = "BookCover"
	path := apiPrefix + "/books/" + strconv.FormatInt(bookID, 10) + "/cover"

	// Refused here rather than upstream. ParseIntPipe accepts a negative id and
	// the row simply will not exist, so this would come back as a 404 — which is
	// the one status a caller CACHES AS PERMANENTLY ABSENT. A caller's bug would
	// be recorded as a fact about a book.
	if bookID <= 0 {
		return Cover{}, &APIError{
			Op: op, Method: http.MethodGet, Path: path,
			Message: "book id must be positive", Err: ErrInvalidRequest,
		}
	}

	status, header, body, err := c.doRaw(ctx, request{
		op: op, method: http.MethodGet, path: path, accept: coverAccept,
	})
	if err != nil {
		return Cover{}, err
	}
	return Cover{
		Status:      status,
		ContentType: text(header.Get("Content-Type")),
		Body:        body,
	}, nil
}
