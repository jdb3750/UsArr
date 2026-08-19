// Package bookorbit is UsArr's client for one BookOrbit instance.
//
// BookOrbit is v0.1's catalogue source (ADR-0052). This package is SLICE 0 of
// that adapter: the client and the credential path, and nothing else. What that
// excludes is at the bottom and it is the half that gets forgotten.
//
// # Why this is neither internal/servarr nor internal/kavita
//
// BookOrbit is a NestJS/Fastify application. It shares none of the *Arr family's
// quirks and none of Kavita's: no ReturnHttpNotAcceptable trap, no
// X-Application-Version header, no PascalCase controller segments, no
// ASP.NET ValidationProblemDetails, and — the one that decides the shape of this
// package — no long-lived API key in a header. What IS shared is the shape of
// the problem, so the proven parts of internal/kavita are reproduced here on
// purpose and named the same: the injected HTTPDoer, the error taxonomy, the
// redaction discipline, the per-instance breaker and its 4xx/5xx policy. Read
// internal/kavita first; this will then read as a translation rather than an
// invention.
//
// The one part that is NOT reproduced is the breaker itself. internal/kavita's
// copy of it named the condition for undoing the duplication — "the first time a
// THIRD client needs one" — and this is that client, so the state machine now
// lives in internal/breaker with the open sentinel injected, and all three
// clients wrap it.
//
// # Authentication: a magic link is exchanged for a short-lived bearer
//
// Verified from source at bookorbit/bookorbit@73b7877d — auth.controller.ts,
// magic-link.service.ts, auth.service.ts, jwt.strategy.ts,
// common/guards/jwt-auth.guard.ts, config/config.ts, app.module.ts, main.ts.
//
// SETUP, done once by a human against BookOrbit itself:
//
//  1. A superuser creates a SHARED account — POST /api/v1/users/shared reaches
//     UserService.createSharedUser, which sets provisioningMethod:'shared',
//     isDefaultPassword:false, a random password hash nobody holds, and takes
//     permissionNames and libraryIds from the DTO.
//  2. The superuser mints a magic link — POST /api/v1/auth/magic-links.
//     MagicLinkService.createToken refuses a non-superuser actor, refuses a
//     target whose provisioningMethod is not 'shared', caps active tokens at 25
//     per user, and returns the RAW token exactly once; only sha256(raw) is
//     stored.
//  3. The human pastes that raw token into UsArr's BookOrbit service form.
//
// USE, on every UsArr process:
//
//  4. POST /api/v1/auth/magic-links/login with body {"token": "<raw>"}. The
//     route is @Public() — the one /api/v1 route besides health that needs no
//     bearer — and is throttled at 10 requests/minute.
//     MagicLinkService.loginWithToken hashes, looks up, and checks in order: row
//     exists, not revoked, isActive, not expired, user exists and is active. NO
//     PASSWORD IS VALIDATED ANYWHERE IN THAT PATH, which is what ADR-0052 §2
//     falsified about the earlier evaluation.
//  5. The response body is {accessToken, user}. It also sets access_token and
//     refresh_token cookies, which this client ignores.
//  6. Every subsequent /api/v1 call carries Authorization: Bearer <accessToken>.
//
// The access token is an HS256 JWT with a default 15-minute life
// (config.auth.jwtExpiresIn). This client holds it in memory, re-mints ~60 s
// before expiry, and re-mints once and retries once on any unexpected 401. It
// keeps no cookie jar: replicating AuthService.refresh's rotating refresh cookie
// and its 30 s reuse grace is more machinery and more failure modes than
// re-minting from a token already held, at a cost of one request per ~15 minutes
// against a 10/minute budget. auth.go carries the detail.
//
// # The credential is never in a URL, and that is ADR-0052's constraint
//
// JwtStrategy's extractors are fromAuthHeaderAsBearerToken() and the
// access_token cookie — never the query string (jwt.strategy.ts). The `?t=` the
// cover routes accept only selects a Cache-Control value
// (book.controller.ts). The HMAC-token-in-a-URL shape exists in exactly one
// place in the repository and it is the OPDS guard (opds/opds-auth.guard.ts),
// which this adapter does not touch. THAT IS WHY THE ADAPTER IS CONSTRAINED TO
// /api/v1 ROUTES: it is the escape from the class the Kavita cover probe
// measured, where the header arm returned 400 and only ?apiKey= in the query
// returned 200, forcing a full-admin credential into every upstream access log.
//
// Two consequences this package enforces rather than documents:
//
//   - The magic-link token and the access token never appear in an error string,
//     a log line or a returned URL. transportError RECONSTRUCTS the error rather
//     than wrapping it, because *url.Error prints the full request URL.
//   - The credential comes BACK in a response body, under `accessToken`. That is
//     the direction internal/ssrf's deny-list was not originally written for and
//     the reason `accesstoken` is in it — internal/vcrscrub's
//     TestAccessTokenDrillArmed recorded a live JWT into a cassette before the
//     entry existed.
//
// # What is stored, and under which scheme
//
// Exactly one secret is stored: the raw magic-link token, in
// service_instance.api_key_enc, under the EXISTING versioned AAD-bound envelope
// (internal/crypto). This package adds no crypto and touches no HKDF info label
// — renaming one would make every stored credential undecryptable and no test
// would catch it. The access token is never written anywhere.
//
// # The §14 scope gate is discharged in code
//
// ADR-0052 left a named gate: the adapter "may not read a catalogue under a
// shared-account credential until the scope that account grants has been
// enumerated against §14". scope.go is that enumeration, in Go rather than in
// prose, so that a 24th BookOrbit permission turns a test red instead of leaving
// a paragraph quietly wrong. The verdict costs no extra request:
// AuthService.buildUserResponse ships isSuperuser, provisioningMethod and the
// permission array in the same body as the accessToken.
//
// The answer it encodes: the correct credential is a shared account with an
// EMPTY permission set and an explicit libraryIds grant, because every route
// this adapter needs declares no @RequirePermission. Anything more is reported
// through Client.Scope and logged at WARN. This client REPORTS rather than
// refuses — a service that will not talk to you with no visible reason is the
// opposite of principle 3 — and the gate ADR-0052 actually names is on the
// catalogue read, which is slice 1's to consult.
//
// # Scope: this is the client, and slice 0 of it
//
// It does: build against a user-configured base URL behind the injected
// SSRF-policy client, mint and cache the credential, probe reachability with the
// @Public() health route, make ONE authenticated read (GET /api/v1/app-info) to
// prove the bearer is accepted, classify the account's scope, and classify and
// bound errors.
//
// It does NOT: read a catalogue, know what a library is, fetch a cover, write a
// schema row, or import anything. There is no GET /libraries call in this
// package and that absence is deliberate — a slice-0 client that already read
// the container list would have quietly started the catalogue adapter. Slice 1
// is internal/libsync/bookorbit.go and it consumes this package.
package bookorbit
