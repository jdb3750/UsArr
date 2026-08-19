// Package kavita is UsArr's client for one Kavita instance.
//
// # Why this is not internal/servarr
//
// Kavita is not an *Arr. It shares none of the family's quirks: no
// ReturnHttpNotAcceptable trap, no `/ping`, no `X-Application-Version` header, no
// `/api/vN` version segment, and none of the three *Arr error envelopes. Bending
// internal/servarr around it would mean teaching a package whose doc.go says
// "UsArr's single client for the *Arr family" a second dialect for every one of
// those. What IS shared is the shape of the problem, so the proven parts of that
// package are reproduced here on purpose and named the same: the injected
// HTTPDoer, the two read paths, the error taxonomy, the redaction discipline, the
// per-instance breaker, and the 4xx/5xx breaker policy. Read internal/servarr
// first; this package will then read as a translation rather than an invention.
//
// # Authentication: one long-lived key in one header, and no token dance
//
// Kavita's OpenAPI document declares exactly one securityScheme —
// `AuthKey` = {type: apiKey, in: header, name: x-api-key} — applied as a GLOBAL
// security requirement with ZERO per-operation overrides — verified against BOTH
// vendored specs (ADR-0046): api/specs/kavita-v0.9.0.2.json, the release the
// owner runs, 462 paths; and api/specs/kavita-develop.json, 488 paths. Zero
// operation-level `security` keys in either. So:
//
//   - Every authenticated call is the same header on every endpoint.
//
//   - There is NO JWT to exchange and NO token to cache or refresh.
//     POST /api/Plugin/authenticate exists and returns a JWT; it is optional, it
//     is not the documented path, and this package deliberately does not use it.
//
//   - `?apiKey=` is NOT general authentication. It is a small special case, plus
//     the OPDS routes where it is a PATH SEGMENT. It is not generalised into this
//     client, and the one method that sends it (PluginVersion) says why in its
//     own doc comment.
//
//     ⚠️ THE SIZE OF THAT SPECIAL CASE DIFFERS BETWEEN THE TWO LINES, which is
//     why this bullet no longer names one number. On develop it is 9 operations;
//     on stable v0.9.0.2 — the release the owner runs — it is 20, because every
//     `/api/Image/*` cover route still accepts the key in the query string there
//     and develop has since dropped it. An earlier draft of this comment stated
//     develop's 9 as if it were Kavita's, which is precisely the error ADR-0046
//     exists to stop. TestAPIKeyQueryIsNotGeneralAuth asserts the property that
//     is true on both — that it is a small set, not the whole API — rather than
//     either count.
//
// # What the two-spec pin can and cannot attest
//
// Both vendored documents are pinned and drift-checked, and that pin is worth
// what an OpenAPI document is worth, which is a narrower thing than it looks. It
// attests THE SHAPE THE VENDOR PUBLISHED — which routes exist, what names they
// take, what they claim to return. It cannot attest how a controller BINDS those
// parameters at runtime, because the document is generated from attributes and
// the binding is what the method body does with whatever arrived. Two instances,
// both measured rather than reasoned:
//
//   - The `/api/Image/*` cover routes. api/specs/kavita-v0.9.0.2.json — the
//     release the owner runs — declares `apiKey` there as an ordinary OPTIONAL
//     query parameter, under the same global `x-api-key` header requirement every
//     other route carries. A live probe against that instance found the reverse:
//     header-only 400, query-only 200. The optional parameter is the required
//     one, and the required header does nothing. (The develop document does not
//     agree with the stable one about this either — it drops the parameter
//     entirely. Two documents, three stories.)
//   - GET /api/Plugin/version carries the same global header requirement in the
//     document and is [AllowAnonymous] in the source, validating the query key
//     itself.
//
// So a contract test against these specs pins what this client sends and how it
// parses what comes back, and stops there. Where BEHAVIOUR is the question — an
// authentication path, a status code, an ordering guarantee — the controller
// source at the pinned commit is the evidence (DEVELOPMENT.md §5), and a live
// probe beats both.
//
// One consequence of the first instance is not about specs at all. ONCE A
// CREDENTIAL IS REQUIRED IN A URL, THE DISCLOSURE SURFACE IS EVERYTHING THAT
// PRINTS A URL — a debug log line, an error string, a bug report pasted into an
// issue, a screenshot of devtools. The go-vcr cassette internal/vcrscrub scrubs
// is one instance of that shape rather than the class, which is why the fix
// belongs at the point a URL is RECORDED rather than at each place it might
// later be shown. internal/ssrf's redactors carry `apikey` case-insensitively,
// so the log and error paths look covered by construction — that is a claim
// someone OWES a drill against the real cover-fetch error shapes when the fetch
// path is built, not one to bank from reading the deny-list. And the browser
// must never see a keyed URL at all: ARCHITECTURE.md §4.4 serves images from
// UsArr's own cache under UsArr's own path and attaches credentials at request
// time, so a design that proxied or redirected the upstream Kavita URL to the
// client would put a full-admin key in devtools — the one place no redactor can
// reach it.
//
// The Auth Key is a user-scoped credential and, for the admin account UsArr will
// normally be given, a full-admin one. Same rules as an *Arr key: encrypted at
// rest under the versioned AAD-bound envelope, never logged, never sent to the
// browser, re-entry required when the host changes (CLAUDE.md, security.md §5).
//
// # Kavita has no version header, so version costs a call
//
// The *Arrs put X-Application-Version on every /api/* response, which makes
// version detection free. Kavita does not: Kavita.Server/Startup.cs puts exactly
// one custom header on an /api/* response, `Pagination`, and nothing carries the
// build. The version is therefore FETCHED, once, and cached on the Client — see
// ServerInfo and Version. Two sources, and neither is universal:
//
//   - GET /api/Server/server-info-slim is ADMIN-ONLY. ServerController is
//     [Authorize(PolicyGroups.AdminPolicy)] at class level, so a non-admin Auth
//     Key gets 403 here, not 401.
//   - GET /api/Plugin/version?apiKey= is [AllowAnonymous] and validates the key
//     itself, so it works for any account — at the cost of putting the credential
//     in a URL. It is never called automatically.
//
// There is no /api/Server/server-info. The endpoint is server-info-slim.
//
// # Two read paths, and picking the wrong one is a memory bug
//
//   - do buffers the whole body, bounded at 32 MB. It is for the small,
//     fixed-shape endpoints — /api/Health, server-info-slim, Library/libraries.
//   - StreamSeries hands each element to a callback as it decodes, bounded at
//     MaxStreamBytes (200 MB, the same ceiling internal/ssrf's transport
//     enforces). POST /api/Series/all-v2 is the reason it exists:
//     UserParams.PageSize defaults to int.MaxValue (verified in
//     Kavita.Common/Helpers/UserParams.cs — and PageSize=0 is coerced to
//     int.MaxValue too), so a call with no paging returns the ENTIRE series list
//     in ONE response.
//
// Both bounds FAIL rather than truncate. A truncated series list is
// indistinguishable from content deleted upstream and would drive a destructive
// reconciliation sweep (ARCHITECTURE.md §7.4).
//
// # Two rules this package enforces rather than documents
//
//   - The Auth Key never appears in an error string, a log line or a returned
//     URL. transportError RECONSTRUCTS the error rather than wrapping it, because
//     *url.Error prints the full request URL and PluginVersion's URL carries the
//     key.
//   - The POST /api/Series/all-v2 body is built from a type that can only express
//     what the endpoint reads (docs/reference/arr-apis.md §7.1). Kavita binds with
//     System.Text.Json exactly as the *Arrs do, so "omitted" and "sent empty" are
//     different — with the polarity inverted, because Kavita's enums are INTEGERS.
//     See SeriesFilter.
//
// The caller injects the *http.Client (as an HTTPDoer). That is deliberate: the
// SSRF policy lives in internal/ssrf, is bound to one service_instance's validated
// host:port, and this package must not be able to bypass it.
//
// # Scope
//
// This is the CLIENT only. No sync loop, no import, no schema writes — that
// boundary is unchanged and is what this section is for. What HAS changed is the
// sentence that used to follow it, "channels 1, 3b and 4 (ADR-0041) are the
// commits after this one": channel 1 arrived, in internal/libsync, which consumes
// this package. Channels 3b and 4 are still ahead; internal/libsync's doc.go is
// the authority on that split, because it is the package the split is about.
package kavita
