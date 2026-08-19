package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/store"
)

// GET /img/{key} is the SERVING half of ARCHITECTURE.md §4.4's image pipeline:
// the route a browser puts in an <img src>, and the only way artwork reaches a
// client.
//
// ⚠️ THE OTHER HALF IS NOT BUILT AND THIS FILE DOES NOT PRETEND OTHERWISE.
// Nothing in the tree fetches an image, encodes one, or writes an `image_asset`
// row — the fetcher needs catalogue rows carrying cover URLs and no adapter
// produces them yet. So on today's tree every request here resolves to a
// not-cached answer, and that is an honest empty cache rather than a bug. What
// this route buys is that the fetch half becomes a small piece the moment an
// adapter produces rows: the key, the authorization, the cache lookup, the
// Content-Type and the wire field are all already here and all already tested.
//
// THREE RULES THAT ARE NOT OBVIOUS, each of them from reference/security.md §4:
//
//  1. IT IS AUTHENTICATED AND AUTHORIZED AGAINST THE OWNING ITEM. Not "behind
//     the session" only — the session says who is asking, and
//     store.LookupImageAsset says whether that person is entitled to the WORK
//     the artwork belongs to. An /img that only checked for a session would
//     serve every cover in the install to every account, which on a multi-user
//     install is the whole restricted-content leak wearing a different URL.
//
//  2. `Cache-Control: private`, ALWAYS, AND IT IS A CONSTANT. A content-derived
//     key justifies IMMUTABILITY, not PUBLICNESS — those are different
//     properties. `public` on a per-user-authorized resource tells a shared
//     cache (a reverse proxy, a corporate middlebox, a service worker shared
//     across profiles) that it may store this response and re-serve it to
//     somebody else, which converts a correct authorization check into no check
//     at all for every request after the first. The directive is therefore not
//     computed from the row, not switchable by a parameter, and not branched on
//     `origin_class`: TestImageCacheControlIsAlwaysPrivate walks every
//     origin_class the schema admits — 'provider' included — and asserts one
//     answer.
//
//  3. GENUINELY PUBLIC ARTWORK GETS A DISTINCT PATH, NOT A FLAG. §4 requires
//     `/img/public/*` "so the distinction is structural". That route is NOT
//     registered here, because nothing produces provider artwork yet and an
//     unauthenticated route with nothing behind it is a hole waiting for
//     content. The structural requirement is still met: publicness on this
//     route is not expressible, so there is nothing for a later change to flip.
//     A future public surface is a new registration and a new handler, and
//     TestImageRouteHasNoPublicBypass pins that this one has no way to become
//     it.
//
// IT IS A LOCAL READ (principle 1). One SQLite statement with a pinned plan and
// one file open. No upstream call, no *Arr, no metadata provider, and — the
// point of the whole pipeline — no image FETCH on the request path.

// imageMediaTypes is `image_asset.format`'s token vocabulary mapped onto the
// media types the Content-Type header carries.
//
// ⚠️ THE CONTENT-TYPE COMES FROM THIS COLUMN AND FROM NOWHERE ELSE. Not from
// sniffing the bytes, and not echoed from whatever the upstream said when the
// image was fetched. Both alternatives are wrong for the same reason and it is
// worth stating: the bytes on disk are produced by UsArr's own encoder in the
// codec the row names (ADR-0050 clause 1 — there is no passthrough width, `orig`
// included), so the row is the only party that KNOWS the encoding; a sniff is a
// guess made from attacker-influenced content, and an echoed upstream header is
// a claim made by a host the user merely configured. `X-Content-Type-Options:
// nosniff` is set globally by securityHeadersMiddleware, so the browser will not
// second-guess this value either — which is only safe because the value is
// derived rather than observed.
//
// A row whose format is not on this map is served as not-cached, never as
// application/octet-stream and never with the header omitted. ADR-0050 named
// this lookup as one line owed by the /img surface; the refusal is the second.
var imageMediaTypes = map[string]string{
	store.ImageFormatJPEG: "image/jpeg",
}

// imageCacheControl is the one caching directive this route emits.
//
// `private` — see rule 2 above. `max-age=31536000, immutable` is a year and a
// promise not to revalidate, which the key earns: `cache_key` is
// sha256(credential-stripped source_url)[:16] over a UNIQUE source_url, so one
// key names one asset for the life of the row, and a re-render under a new codec
// writes the same path for the same key rather than changing what the key means.
//
// ⚠️ THAT LAST SENTENCE IS THE LIMIT OF THE PROMISE, and it is worth being
// precise because ARCHITECTURE §4.1 calls the key "content-derived" and it is
// not: it is derived from the URL. If an upstream replaces the bytes at a URL it
// has already served, the key does not change and a client holding this response
// keeps the old picture until the year is up. That is the trade `immutable`
// makes, it is the same trade the id would have made, and it is acceptable
// because cover art is decoration whose staleness is visible and harmless.
const imageCacheControl = "private, max-age=31536000, immutable"

// handleImage serves one cached image.
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	// The width, before anything touches the database or the disk. An arbitrary
	// `?w=` is a cache-poisoning DoS (§4.4, GHSA-rrr6-mvwg-9pg9), so this is a
	// REFUSAL and not a clamp — unlike `?limit=` on the library reads, where
	// clamping costs nothing. Clamping here would mint a cache entry for every
	// value a caller invented, which is the resource the allowlist protects.
	//
	// ⚠️ THE VALUE IS NOT TRIMMED, NOT LOWERCASED AND NOT NORMALISED IN ANY
	// WAY. The vocabulary is seven literal tokens, so "exactly one of these
	// seven strings" is the strongest and simplest form the rule can take;
	// every normalisation step added here is a second spelling that has to be
	// reasoned about, on a parameter whose whole job is to be bounded.
	width := r.URL.Query().Get("w")
	if width == "" {
		width = imagecache.DefaultWidth
	}
	if !imagecache.ValidWidth(width) {
		// The rejected value is NOT echoed: it is caller-supplied text and this
		// message is rendered. The allowlist is echoed instead, which is the
		// half a caller can act on.
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"that is not an image width this server renders").
			withAction("ask for one of: " + strings.Join(imagecache.Widths(), ", "))
	}

	key := r.PathValue("key")

	// SCOPE. storeScope derives it from the session and nothing else — there is
	// no query parameter that widens it, which is why it is not one. The lookup
	// answers not-found for "no such key" and for "not yours" alike, and this
	// handler keeps them one answer: a distinguishable "exists but not yours"
	// is the existence oracle security.md §4 is written against.
	asset, found, err := s.store.LookupImageAsset(r.Context(), storeScope(a), key)
	if err != nil {
		if errors.Is(err, store.ErrImageAssetAmbiguous) {
			// Two rows share this key, so there is no single owning item to
			// have authorized against. It is logged as the operational event it
			// is and answered as a miss, because the caller did nothing wrong
			// and there is nothing they can do.
			s.log.Error("two image assets share one cache key; neither will be served",
				"err", redactText(err.Error()))
			return errImageNotFound()
		}
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"that image could not be read").wrapping(err)
	}
	if !found {
		return errImageNotFound()
	}

	// The row exists and is the caller's to see. Whether there are BYTES is a
	// separate question with a separate answer, because §4.4.1's cold start is
	// an install where most assets are in exactly this state and a client that
	// cannot tell "not yet" from "no such image" cannot render either honestly.
	mediaType, known := imageMediaTypes[asset.Format.String]
	if !asset.Format.Valid || !known {
		// A NULL format is the ordinary pending row. A format the map does not
		// know is a row written by a newer binary, or by a writer that skipped
		// store.ValidImageFormat — either way this process cannot state the
		// encoding, and it will not guess one.
		if asset.Format.Valid {
			s.log.Warn("image asset names a format this binary cannot serve",
				"asset_id", asset.ID, "format", redactText(asset.Format.String))
		}
		return errImageNotCached()
	}

	f, info, err := s.images().Open(asset.CacheKey, width)
	if err != nil {
		if errors.Is(err, imagecache.ErrNotCached) {
			return errImageNotCached()
		}
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"that image could not be read").wrapping(err)
	}
	defer func() { _ = f.Close() }()

	h := w.Header()
	// Set BEFORE ServeContent, and that ordering is load-bearing:
	// http.ServeContent sniffs the first 512 bytes to pick a Content-Type only
	// when the header is unset, which is exactly the sniff rule 2 above forbids.
	// The empty name passed to ServeContent is the other half — a name would let
	// it choose a type from the file extension, and the cached files
	// deliberately have none.
	h.Set("Content-Type", mediaType)
	h.Set("Cache-Control", imageCacheControl)

	http.ServeContent(w, r, "", modTimeOrZero(info.ModTime()), f)
	return nil
}

// modTimeOrZero passes the file's mtime through, or the zero time when the
// filesystem reports something unusable.
//
// It feeds ServeContent's Last-Modified and its conditional-request handling. A
// zero time makes ServeContent emit neither, which is the honest answer for a
// file whose age is unknown — and costs nothing here, because `immutable` means
// a conforming client does not revalidate within the year anyway.
func modTimeOrZero(t time.Time) time.Time {
	if t.IsZero() || t.Year() < 1970 {
		return time.Time{}
	}
	return t
}

// errImageNotFound is the answer for a key that names no row and for a key that
// names a row the caller may not see. ONE message for both, on purpose.
func errImageNotFound() error {
	return errStatus(http.StatusNotFound, CodeNotFound, "no such image")
}

// errImageNotCached is the answer for an asset the caller may see whose bytes
// this server does not hold at that width.
//
// IT IS A DIFFERENT CODE FROM not_found AND THAT DISCLOSES NOTHING EXTRA: the
// caller already got the asset key from a browse response it was entitled to
// read, so "this asset exists" is not news to it. What the split buys is that a
// client can distinguish a broken link from a cover that has not been rendered
// yet — §4.4.1's whole cold-start problem — and render a placeholder rather than
// a broken image.
func errImageNotCached() error {
	return errStatus(http.StatusNotFound, CodeNotCached,
		"that image has not been rendered yet").
		withAction("it will appear once the image pipeline has fetched it")
}

// images is the cache this process serves from. It is built per request rather
// than held on the Server because it is two words and holding it would make the
// zero Config look configured.
func (s *Server) images() *imagecache.Cache {
	return imagecache.New(s.cfg.ImageCacheDir)
}
