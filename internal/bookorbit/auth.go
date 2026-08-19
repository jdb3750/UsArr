package bookorbit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// ─── The credential, and the two different things called "token" ─────────────
//
// THE STORED SECRET is the raw MAGIC-LINK TOKEN. A superuser mints it with
// POST /api/v1/auth/magic-links for a SHARED account. ⚠️ This used to say only
// sha256(raw) is kept on BookOrbit's side, which ADR-0060 falsified at 73b7877:
// `magic_access_tokens` carries `raw_token` in PLAINTEXT beside `token_hash`,
// and any superuser can read every row's raw value back over
// GET /api/v1/auth/magic-links. Login is hash-based, so the plaintext column is
// not load-bearing — it exists so the link can be shown again. ⚠️ THAT IS A
// MEASUREMENT AT 73b7877d2fede2221b0ca360af9bfced7c3797f3 (re-read 2026-08-19),
// NOT A PROMISE, and what would falsify it is one commit: dropping `rawToken`
// from `MagicLinkRepository.findAll`'s select while keeping it in the create
// response would restore the show-once story this paragraph exists to correct,
// and would take the "compare it, do not rotate it" advice in
// httpapi.credentialAction, likelyCauses and bookOrbitTestAction down with it.
// It is long-lived,
// reusable, optionally non-expiring, capped at 25 per user, and it mints
// unlimited access tokens for that account — a §14 credential in full. It is
// encrypted at rest under the existing versioned, AAD-bound envelope
// (internal/crypto; service_instance.api_key_enc, AAD bound to the row and to
// the normalised scheme://host:port), never logged, never sent to the browser,
// and re-entry is required when the host changes. This package adds NO crypto:
// the envelope, the KEK derivation and the HKDF info labels are untouched, which
// is the point — renaming a label would make every stored credential
// undecryptable and no test would catch it.
//
// THE HELD SECRET is the ACCESS TOKEN: an HS256 JWT that
// AuthService.issueTokensForUser returns in the login response body as
// `accessToken`. Its lifetime is config.auth.jwtExpiresIn, default '15m'
// (config/config.ts). It is held IN MEMORY on the Client and never written
// anywhere.
//
// THE IGNORED SECRET is the refresh cookie. The same response sets
// `refresh_token` and `access_token` cookies, and AuthService.refresh rotates
// the former with a 30 s reuse grace. Replicating a rotating cookie jar in Go is
// more machinery and more failure modes than re-minting from a token we already
// hold, so this client keeps no cookie jar at all. The cost is one extra request
// per ~15 minutes of activity against a 10-per-minute throttle.
//
// ⚠️ A full import that outlives the access token WILL re-mint mid-walk, and the
// request that got the 401 is retried once after it. That is the only place the
// auth model touches a read loop, and do() is where it is handled so that no
// caller has to remember.

// authScheme and authHeader are the ONE way a credential leaves this process.
//
// ADR-0052's `/api/v1`-only constraint is what makes that true and it is
// load-bearing rather than stylistic: those routes sit behind the global
// JwtAuthGuard (app.module.ts), and JwtStrategy's extractors are
// fromAuthHeaderAsBearerToken() and the access_token cookie — NEVER the query
// string (jwt.strategy.ts). The `?t=` the cover routes accept is a cache-buster
// that only selects a Cache-Control value (book.controller.ts). The
// HMAC-token-in-a-URL shape is confined to the OPDS surface this adapter does
// not touch (opds/opds-auth.guard.ts).
//
// That is the escape from the class the Kavita cover probe measured, where the
// header arm returned 400 and only `?apiKey=` in the query returned 200 —
// forcing a full-admin credential into every upstream access log.
const (
	authHeader = "Authorization"
	authScheme = "Bearer "
)

// magicLinkLoginPath is the ONE @Public() route under /api/v1 this client uses
// besides health. auth.controller.ts: @Post('magic-links/login') @Public()
// @Throttle({limit:10, ttl:60_000}) @HttpCode(200).
const magicLinkLoginPath = apiPrefix + "/auth/magic-links/login"

// refreshSkew is how far before expiry the access token is re-minted.
//
// 60 s covers ordinary clock skew between UsArr's host and BookOrbit's without
// costing anything: at a 15-minute lifetime it moves the mint from every 15
// minutes to every 14, against a 10-per-minute throttle.
const refreshSkew = 60 * time.Second

// fallbackTokenTTL is assumed when the access token's expiry cannot be read.
//
// It is SHORTER than BookOrbit's default lifetime on purpose. Guessing long
// would mean sitting on a dead token and taking a 401 on a real call; guessing
// short costs one extra mint. The 401 retry is the authoritative mechanism
// either way — this only decides how often it has to fire.
const fallbackTokenTTL = 5 * time.Minute

// session is the in-memory credential state. It is never serialised.
type session struct {
	token   string
	expires time.Time
	account AccountView
	verdict ScopeVerdict
}

func (s session) usable(now time.Time) bool {
	return s.token != "" && now.Add(refreshSkew).Before(s.expires)
}

// accessToken returns a usable bearer token, minting one if needed.
//
// SINGLE-FLIGHT BY MUTEX, and that is the design rather than a shortcut. The
// login route is throttled at 10/minute; a client that let twenty concurrent
// page fetches each notice the expiry and mint would spend its budget in one
// tick and then 429 the whole import. Holding authMu across the HTTP call means
// the second caller waits and then finds a fresh token already in place.
//
// staleToken makes the retry path idempotent: a 401 hands back the token that
// failed, and the mint is skipped if some other goroutine has already replaced
// it. Without that, N concurrent 401s become N mints.
func (c *Client) accessToken(ctx context.Context, staleToken string) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	// A caller that waited on the mutex may have had its deadline pass in the
	// meantime; fail here rather than starting a request that cannot finish.
	if err := ctx.Err(); err != nil {
		return "", err
	}

	switch {
	case staleToken != "" && c.sess.token != staleToken:
		// Someone else already re-minted after the same 401.
		return c.sess.token, nil
	case staleToken == "" && c.sess.usable(c.now()):
		return c.sess.token, nil
	}

	if c.magicLink == "" {
		return "", &APIError{
			Op: "MagicLinkLogin", Method: http.MethodPost, Path: magicLinkLoginPath,
			Message: "no magic-link token is configured; every /api/v1 route except health is behind JwtAuthGuard",
			Err:     ErrNoCredential,
		}
	}

	s, err := c.mint(ctx)
	if err != nil {
		// A failed mint clears the cached session. Keeping a token whose mint
		// just failed would mask a revoked magic link for up to 15 minutes.
		c.sess = session{}
		return "", err
	}
	c.sess = s
	return s.token, nil
}

// mint performs POST /api/v1/auth/magic-links/login. Callers hold authMu.
//
// It goes through roundTrip rather than doRaw — one exchange, no bearer, no
// recursion. anonymous is true because the route is @Public(), and because
// sending a stale bearer alongside the magic-link token would make this the one
// request in the client that carried two credentials.
func (c *Client) mint(ctx context.Context) (session, error) {
	// Authenticate can reach here with an unbounded context. doRaw has already
	// applied the call's budget when the mint is on an authenticated call's
	// critical path, so only fill one in when there is none.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeouts.Default)
		defer cancel()
	}

	var out magicLinkLoginResponse
	status, _, body, err := c.roundTrip(ctx, request{
		op:        "MagicLinkLogin",
		method:    http.MethodPost,
		path:      magicLinkLoginPath,
		body:      magicLinkLoginRequest{Token: c.magicLink},
		anonymous: true,
	}, "")
	if err != nil {
		return session{}, err
	}
	if status < 200 || status >= 300 {
		apiErr := parseErrorBody("MagicLinkLogin", http.MethodPost, magicLinkLoginPath, status, body)
		// This is the ONE place UsArr replaces an upstream message instead of
		// passing it through, and so it is the exception errors.go's
		// ErrUnauthorized comment points at. The reason is that the sentinel
		// cannot carry a cause — parseErrorBody maps every 401 to
		// ErrUnauthorized and branches on nothing — while the operator still
		// needs a sentence naming what to go and check.
		//
		// ⚠️ The condition list below is doc.go's, read from
		// bookorbit@73b7877d. BookOrbit's server source is not vendored here,
		// so it is not re-checkable from this tree. This comment used to add
		// that BookOrbit "deliberately" draws no distinction; that was its
		// word and it is dropped rather than hedged, because upstream's intent
		// is UNKNOWN. The sentence below claims only that UsArr cannot tell
		// the causes apart, which is a fact about this client.
		if status == http.StatusUnauthorized {
			apiErr.Message = "BookOrbit rejected the magic-link token: it is unknown, revoked, deactivated, expired, " +
				"or its account is inactive — BookOrbit does not say which"
		}
		return session{}, apiErr
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return session{}, &APIError{
			Op: "MagicLinkLogin", Method: http.MethodPost, Path: magicLinkLoginPath, Status: status,
			Message: "decoding login response: " + truncate(err.Error()), Err: ErrDecode,
		}
	}
	if out.AccessToken == "" {
		// A 200 with no accessToken is not a BookOrbit answering. Note that a
		// REPLAYED CASSETTE legitimately carries the placeholder here, because the
		// scrubber redacts `accessToken` on the way to disk — so the check is for
		// EMPTY, not for well-formed.
		return session{}, &APIError{
			Op: "MagicLinkLogin", Method: http.MethodPost, Path: magicLinkLoginPath, Status: status,
			Message: "the login succeeded but carried no accessToken", Err: ErrWrongService,
		}
	}

	account := toAccountView(out.User)
	s := session{
		token:   out.AccessToken,
		expires: c.tokenExpiry(out.AccessToken),
		account: account,
		verdict: classifyScope(account),
	}

	// The §14 gate, logged at the point the credential comes into use. The token
	// itself is never in this line — only the account it belongs to.
	if !s.verdict.Minimal() {
		details := make([]string, 0, len(s.verdict.Findings))
		for _, f := range s.verdict.Findings {
			details = append(details, f.String())
		}
		c.log.Warn("bookorbit credential is broader than a catalogue replica needs",
			"account", account.Username,
			"elevated", s.verdict.Elevated(),
			"findings", strings.Join(details, "; "))
	}
	return s, nil
}

// tokenExpiry reads the `exp` claim out of the access token.
//
// 🚩 THE SIGNATURE IS NOT VERIFIED AND CANNOT BE: the HS256 secret is
// BookOrbit's (config.auth.jwtSecret) and UsArr does not have it. So this is
// NOT a security decision and nothing is authorised on the strength of it — it
// only decides WHEN to pre-emptively re-mint. The authoritative signal that a
// token is dead is a 401 from BookOrbit, which do() handles by re-minting and
// retrying once; a forged or nonsense `exp` can make this client mint too often
// or too rarely and can do nothing else.
//
// An unreadable token — including the `<redacted>` placeholder a scrubbed
// cassette legitimately carries — falls back to fallbackTokenTTL.
func (c *Client) tokenExpiry(tok string) time.Time {
	fallback := c.now().Add(fallbackTokenTTL)

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return fallback
	}
	// JWT payloads are base64url WITHOUT padding (RFC 7515 §2).
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fallback
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return fallback
	}
	exp := time.Unix(claims.Exp, 0).UTC()
	if !exp.After(c.now()) {
		// Already expired on arrival — a clock disagreement, most likely. Do not
		// return a past instant: usable() would then be false forever and every
		// single call would mint, which is how a 10/min throttle is exhausted.
		return fallback
	}
	return exp
}

// invalidate drops the cached session. Used when the caller knows the credential
// changed underneath — the host was edited, the token was re-entered.
func (c *Client) invalidate() {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	c.sess = session{}
}

// Scope returns the §14 verdict for the account behind the stored credential,
// and whether one has been computed yet.
//
// It is populated by a successful mint and costs no request of its own —
// AuthService.buildUserResponse ships isSuperuser, provisioningMethod and
// permissions in the same body as the accessToken. ADR-0052's gate ("may not
// read a catalogue under a shared-account credential until the scope … has been
// enumerated") turns on Elevated(); the catalogue read that consults it is
// slice 1's, and this slice ships the mechanism.
func (c *Client) Scope() (ScopeVerdict, bool) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.sess.token == "" {
		return ScopeVerdict{}, false
	}
	return c.sess.verdict, true
}

// Account returns the allowlisted view of the BookOrbit account behind the
// stored credential, and whether one has been observed.
func (c *Client) Account() (AccountView, bool) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.sess.token == "" {
		return AccountView{}, false
	}
	return c.sess.account, true
}

// Authenticate mints an access token if none is held, and reports the §14
// verdict for the account it belongs to.
//
// This is the credential handshake: it is the call the Services screen makes to
// answer "is this token good, and what does it let UsArr do". It proves the
// token, not the catalogue — AppInfo is the authenticated READ that proves the
// bearer is actually accepted on an ordinary route.
func (c *Client) Authenticate(ctx context.Context) (ScopeVerdict, error) {
	if _, err := c.accessToken(ctx, ""); err != nil {
		return ScopeVerdict{}, err
	}
	v, _ := c.Scope()
	return v, nil
}
