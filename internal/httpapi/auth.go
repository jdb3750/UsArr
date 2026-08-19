package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/releases"
	"github.com/jdb3750/UsArr/internal/store"
)

// Cookie names. The session cookie is HttpOnly; the CSRF cookie deliberately is
// not, because the SPA has to read it to echo it back in the header — that is
// what makes the double-submit check work.
const (
	sessionCookie = "usarr_session"
	csrfCookie    = "usarr_csrf"
	csrfHeader    = "X-CSRF-Token"
)

// tokenBytes is 32 bytes of crypto/rand for both the session value and the CSRF
// token: 256 bits, so guessing is not a threat model.
const tokenBytes = 32

// newToken mints an opaque credential.
func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// sessionID is what goes in the database: the hash of the cookie value, never
// the value. A database read — a backup, a support bundle, a SELECT — must not
// yield a usable session token.
func sessionID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// authSession is the authenticated caller.
type authSession struct {
	Session store.Session
	User    store.User
}

// sudoActive reports whether the 5-minute re-authentication window is open.
func (a authSession) sudoActive(now time.Time) bool {
	if !a.Session.SudoUntil.Valid {
		return false
	}
	until, err := store.ParseTime(a.Session.SudoUntil.String)
	if err != nil {
		return false
	}
	return now.Before(until)
}

func sessionFrom(r *http.Request) (authSession, bool) {
	a, ok := r.Context().Value(ctxKeySession).(authSession)
	return a, ok
}

// storeScope is the access scope every aggregate read takes, in its signature,
// from the first commit (ARCHITECTURE.md §1.3 rule 2). In v0.1 there is one
// owner and the scope is total; that is the degenerate case of the rule, not
// the absence of one.
func storeScope(a authSession) store.Scope {
	if a.User.IsOwner {
		return store.OwnerScope(a.User.ID)
	}
	// No non-owner accounts exist in v0.1. Failing closed here — an empty
	// InstanceIDs with AllInstances false renders as `1=0` in the query — means
	// the day one appears, it sees nothing rather than everything.
	return store.Scope{UserID: a.User.ID}
}

// releasesScope adapts the store scope to internal/releases, which declares its
// own Scope type so the two packages could settle independently. The invariant
// that matters is that the parameter exists and is derived from the caller, not
// which package owns the struct.
func (s *Server) releasesScope(r *http.Request, a authSession) (releases.Scope, error) {
	scope := storeScope(a)
	if scope.AllInstances {
		return releases.Scope{UserID: a.User.ID, AllInstances: true}, nil
	}
	// Enumerating is only for a scope that is genuinely partial. An empty list
	// with AllInstances false means "sees nothing", and both packages fail
	// closed on it — which is the whole point of carrying the parameter.
	instances, err := s.store.ListServiceInstances(r.Context(), scope)
	if err != nil {
		return releases.Scope{}, errStatus(http.StatusInternalServerError, CodeInternal,
			"the configured services could not be read").wrapping(err)
	}
	ids := make([]int64, 0, len(instances))
	for _, in := range instances {
		ids = append(ids, in.ID)
	}
	return releases.Scope{UserID: a.User.ID, InstanceIDs: ids}, nil
}

// authenticated resolves the cookie session and rejects everything else.
//
// The cookie path and any future bearer/API-key path stay STRICTLY separate
// (reference/security.md §6): a browser must not be able to authenticate an
// endpoint with an ambient credential it did not intend to send. v0.1 has no
// bearer path at all, so presenting one here is a client bug and is refused
// rather than ignored.
func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
			s.writeError(w, r, errStatus(http.StatusUnauthorized, CodeUnauthorized,
				"this endpoint authenticates with the session cookie only; "+
					"bearer and API-key credentials are not accepted here").
				withAction("Sign in"))
			return
		}

		a, err := s.resolveSession(r)
		if err != nil {
			s.writeError(w, r, err)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeySession, a)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) resolveSession(r *http.Request) (authSession, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return authSession{}, errStatus(http.StatusUnauthorized, CodeUnauthorized,
			"this request has no session").withAction("Sign in")
	}
	now := s.now()
	sess, err := s.store.GetSession(r.Context(), sessionID(c.Value), now)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return authSession{}, errStatus(http.StatusUnauthorized, CodeUnauthorized,
				"this session has expired or been revoked").withAction("Sign in")
		}
		return authSession{}, errStatus(http.StatusInternalServerError, CodeInternal,
			"the session could not be read").wrapping(err)
	}
	user, err := s.store.GetUser(r.Context(), sess.UserID)
	if err != nil {
		return authSession{}, errStatus(http.StatusUnauthorized, CodeUnauthorized,
			"the account for this session no longer exists").withAction("Sign in").wrapping(err)
	}
	if user.IsDisabled {
		return authSession{}, errStatus(http.StatusForbidden, CodeForbidden,
			"this account is disabled")
	}

	// Slide the idle window. The absolute window is never extended: the two
	// timeouts exist because they fail differently, and extending both would
	// leave only one.
	if err := s.store.TouchSession(r.Context(), sess.ID, now); err != nil {
		s.log.Warn("session touch failed", "err", redactText(err.Error()))
	}
	return authSession{Session: sess, User: user}, nil
}

// sudo gates the operations that a stolen cookie must not be enough for:
// adding or changing a service credential, changing a base_url, and issuing or
// revoking a client credential (reference/security.md §6).
func (s *Server) sudo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := sessionFrom(r)
		if !ok {
			s.writeError(w, r, errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session"))
			return
		}
		if !a.sudoActive(s.now()) {
			s.writeError(w, r, errStatus(http.StatusForbidden, CodeSudoRequired,
				"this operation touches a stored credential and needs a fresh password confirmation").
				withAction("Confirm your password"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfProtected applies the two checks that together stop a cross-origin form
// or fetch from acting as the logged-in user.
//
// SameSite=Lax alone is NOT sufficient (reference/security.md §6): it does not
// cover every browser, it does not cover a same-site subdomain, and top-level
// navigations are exempt from it by design. So:
//
//  1. Content-Type: application/json, which a cross-origin <form> cannot send
//     without a preflight, and which a preflight will refuse.
//  2. A double-submit token: a non-HttpOnly cookie the SPA reads and echoes in
//     X-CSRF-Token, compared in constant time.
func (s *Server) csrfProtected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := checkContentTypeJSON(r); err != nil {
			s.writeError(w, r, err)
			return
		}
		if err := checkCSRFToken(r); err != nil {
			s.writeError(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func checkContentTypeJSON(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	media, _, _ := strings.Cut(ct, ";")
	if !strings.EqualFold(strings.TrimSpace(media), "application/json") {
		return errStatus(http.StatusUnsupportedMediaType, CodeUnsupportedMediaType,
			"state-changing requests must be sent as Content-Type: application/json")
	}
	return nil
}

func checkCSRFToken(r *http.Request) error {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return errStatus(http.StatusForbidden, CodeCSRF,
			"this request carries no CSRF token").withAction("Reload the page")
	}
	sent := r.Header.Get(csrfHeader)
	if sent == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(c.Value)) != 1 {
		return errStatus(http.StatusForbidden, CodeCSRF,
			"the CSRF token does not match this session").withAction("Reload the page")
	}
	return nil
}

// setCSRFCookie issues or refreshes the double-submit token.
func (s *Server) setCSRFCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", errStatus(http.StatusInternalServerError, CodeInternal, "a CSRF token could not be generated").wrapping(err)
	}
	// nolint:gosec // G124 fires here for two reasons, and both are the design.
	// (1) HttpOnly is false ON PURPOSE: double-submit requires the SPA to read
	// this cookie and echo it in the CSRF header, so an HttpOnly CSRF cookie
	// would not be a hardening, it would stop the scheme working. The token is
	// not a credential — it authenticates nothing on its own, and the session
	// cookie beside it IS HttpOnly. (2) Secure is a function call rather than a
	// constant true, so gosec cannot prove it; see secureCookies below for why
	// it is conditional. SameSite=Lax is set. Disagree with (1) only by
	// replacing double-submit with a different CSRF scheme.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: see the note above
		Name:     csrfCookie,
		Value:    token,
		Path:     s.cookiePath(),
		HttpOnly: false, // the SPA must read it; that is the double-submit design
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, value string) {
	// G124 fires only because Secure is computed per request rather than a
	// constant true; gosec cannot see through the call. HttpOnly and
	// SameSite=Lax are both set unconditionally on the line below. The
	// conditional Secure is deliberate and argued in secureCookies.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure is conditional by design, see secureCookies
		Name:     sessionCookie,
		Value:    value,
		Path:     s.cookiePath(),
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
		// No Max-Age. The lifetime that matters is the server-side one in the
		// session row; a browser-side expiry would be advisory and a second,
		// disagreeing source of truth.
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	// Same as setSessionCookie: the attributes must match the cookie being
	// cleared or the browser keeps the original. HttpOnly and SameSite are set;
	// only the conditional Secure trips G124.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure is conditional by design, see secureCookies
		Name:     sessionCookie,
		Value:    "",
		Path:     s.cookiePath(),
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Server) cookiePath() string {
	if s.cfg.URLBase == "" {
		return "/"
	}
	return s.cfg.URLBase + "/"
}

// secureCookies decides the Secure attribute.
//
// reference/security.md §6 asks for `HttpOnly; Secure; SameSite=Lax`, and
// CONFIGURATION.md §0 states that v0.1's simplest supported deployment is
// PLAIN HTTP on a trusted LAN. Those two cannot both be satisfied
// unconditionally: a browser silently discards a Secure cookie sent over http,
// so an always-Secure cookie means nobody can log in on the documented default
// deployment, and the fix a user reaches for is worse than the flag.
//
// So Secure is set whenever the request is HTTPS — directly, or via a trusted
// proxy's X-Forwarded-Proto, which is only believed inside
// USARR_TRUSTED_PROXIES. On plain HTTP it is omitted, because there is no
// downgrade to protect against when there is no TLS in the first place.
func (s *Server) secureCookies(r *http.Request) bool {
	return clientOf(r.Context()).scheme == "https"
}

// ── handlers ────────────────────────────────────────────────────────────────

type sessionResponse struct {
	Authenticated bool       `json:"authenticated"`
	SetupRequired bool       `json:"setup_required"`
	CSRFToken     string     `json:"csrf_token"`
	UserID        int64      `json:"user_id,omitempty"`
	Username      string     `json:"username,omitempty"`
	IsOwner       bool       `json:"is_owner,omitempty"`
	SudoUntil     *time.Time `json:"sudo_until,omitempty"`
}

// handleSession is the SPA's bootstrap call: it hands out the CSRF token and
// says whether anyone is signed in and whether the owner account exists yet.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) error {
	token, err := s.setCSRFCookie(w, r)
	if err != nil {
		return err
	}
	resp := sessionResponse{CSRFToken: token}

	if _, err := s.store.Owner(r.Context()); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return errStatus(http.StatusInternalServerError, CodeInternal, "the owner account could not be read").wrapping(err)
		}
		resp.SetupRequired = true
	}

	if a, err := s.resolveSession(r); err == nil {
		resp.Authenticated = true
		resp.UserID = a.User.ID
		resp.Username = a.User.Username
		resp.IsOwner = a.User.IsOwner
		if a.Session.SudoUntil.Valid {
			if until, perr := store.ParseTime(a.Session.SudoUntil.String); perr == nil && s.now().Before(until) {
				resp.SudoUntil = &until
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSetup creates the single v0.1 owner. It succeeds exactly once: once an
// owner exists this endpoint is closed, so it cannot be used to mint a second
// administrator against a running install.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.store.Owner(r.Context()); err == nil {
		return errStatus(http.StatusConflict, CodeAlreadySetup,
			"an owner account already exists").withAction("Sign in")
	} else if !errors.Is(err, store.ErrNotFound) {
		return errStatus(http.StatusInternalServerError, CodeInternal, "the owner account could not be read").wrapping(err)
	}

	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		return errStatus(http.StatusBadRequest, CodeBadRequest, "username must not be empty")
	}
	if len(req.Password) < 12 {
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"the owner password must be at least 12 characters")
	}

	hash, err := crypto.HashPassword(r.Context(), req.Password)
	if err != nil {
		if kdfBusy(err) {
			return kdfBusyStatus(err)
		}
		return errStatus(http.StatusInternalServerError, CodeInternal, "the password could not be hashed").wrapping(err)
	}
	id, err := s.store.CreateUser(r.Context(), store.User{
		Username:     req.Username,
		DisplayName:  req.Username,
		AuthSource:   "local",
		PasswordHash: hash,
		IsOwner:      true,
	})
	if err != nil {
		return errStatus(http.StatusConflict, CodeConflict,
			"that account could not be created: "+redactText(err.Error())).wrapping(err)
	}
	s.audit(r, "user.create", "user", id, "ok", `{"is_owner":true}`)

	return s.startSession(w, r, id)
}

// handleLogin authenticates and starts a session.
//
// Unknown-user and bad-password produce IDENTICAL text and, because a dummy
// verification runs on the unknown-user branch, comparable timing
// (reference/security.md §6).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	badCredentials := errStatus(http.StatusUnauthorized, CodeUnauthorized,
		"that username and password do not match").withAction("Try again")

	// THERE IS EXACTLY ONE KDF CALL BELOW, AND EVERY BRANCH GOES THROUGH IT.
	// That is the point of this shape, and it used to be three separate calls
	// with the two unknown-account ones discarding their error.
	//
	// The KDF is bounded by a semaphore now (crypto.maxKDFConcurrency), so a
	// call can fail with ErrKDFBusy — and three call sites, two of them ignoring
	// the result, would have meant a saturated server answering 401 fast on the
	// unknown-account branch and 503 on the known one. That is a
	// user-enumeration oracle, reachable by any attacker who can saturate the
	// gate, and it would have been introduced by the fix for the memory problem.
	// The dummy hash exists precisely to close that oracle; folding the branches
	// into one gated call keeps it closed for the failure case too.
	//
	// It also means the unknown-account path CONSUMES a permit rather than
	// skipping the bound, so the cheapest request an attacker can send — no
	// valid username needed — is not the one that dodges the cap.
	user, lookupErr := s.store.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	phc, reason, targetID := user.PasswordHash, "", user.ID
	switch {
	case lookupErr != nil:
		phc, reason, targetID = dummyPHC, "no such account", 0
	case user.IsDisabled || user.PasswordHash == "":
		phc, reason = dummyPHC, "account cannot sign in"
	}

	// Burn the same work an existing account would have cost, so the response
	// time does not answer "does this account exist".
	verifyErr := crypto.VerifyPassword(r.Context(), phc, req.Password)
	if kdfBusy(verifyErr) {
		// Identical for every branch above, which is what keeps the timing
		// equaliser intact under saturation. No audit row: a shed request
		// carries no information about the account, and an unauthenticated
		// endpoint that appends a row per request is a log-flood vector.
		return kdfBusyStatus(verifyErr)
	}
	if reason != "" {
		s.audit(r, "auth.login", "user", targetID, "fail", `{"reason":"`+reason+`"}`)
		return badCredentials
	}
	if verifyErr != nil {
		s.audit(r, "auth.login", "user", user.ID, "fail", `{"reason":"bad password"}`)
		return badCredentials
	}

	if err := s.store.TouchUserLogin(r.Context(), user.ID, s.now()); err != nil {
		s.log.Warn("last_login_at not recorded", "err", redactText(err.Error()))
	}
	s.audit(r, "auth.login", "user", user.ID, "ok", "")
	return s.startSession(w, r, user.ID)
}

// dummyPHC is a real Argon2id hash of a value nobody knows, used only to make
// the unknown-account branch cost what the known-account branch costs.
var dummyPHC = func() string {
	// context.Background(): this runs once at package initialisation, before
	// any request exists, and the gate is empty then by construction.
	h, err := crypto.HashPassword(context.Background(), "usarr-timing-equalizer-not-a-credential")
	if err != nil {
		// Only reachable if Argon2id itself is unusable, in which case no login
		// can succeed anyway; an empty string makes VerifyPassword fail fast.
		return ""
	}
	return h
}()

// kdfBusy reports whether an error is the KDF concurrency bound shedding load.
func kdfBusy(err error) bool { return errors.Is(err, crypto.ErrKDFBusy) }

// kdfBusyStatus is the one answer every KDF-bounded endpoint gives when no
// Argon2id permit came free in time.
//
// 503 rather than 429: 429 says "you have sent too many requests", which is a
// per-caller statement UsArr cannot make — there is no rate limiter and no
// per-caller state (FUTURE.md carries that half). 503 says "this server is not
// doing that right now", which is exactly true and is true of every caller
// equally. Spelled once so the login and sudo paths cannot drift apart, which
// would itself be an enumeration signal.
func kdfBusyStatus(err error) *apiError {
	return errStatus(http.StatusServiceUnavailable, CodeBusy,
		"the server is handling as many sign-in attempts as it can at once").
		withAction("Try again in a moment").wrapping(err)
}

// startSession mints the session and regenerates the CSRF token.
//
// The session id is new on every privilege change (login is one), so a token
// fixed before authentication cannot survive it.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	value, err := newToken()
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal, "a session could not be created").wrapping(err)
	}
	now := s.now()
	sess := store.Session{
		ID:                sessionID(value),
		UserID:            userID,
		Kind:              "web",
		UserAgent:         truncateUA(r.UserAgent()),
		IP:                ClientIP(r.Context()),
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     now.Add(store.SessionIdleTimeout),
		AbsoluteExpiresAt: now.Add(store.SessionAbsoluteTimeout),
	}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal, "a session could not be created").wrapping(err)
	}
	// Signing in is itself a fresh authentication, so it opens the sudo window.
	if err := s.store.GrantSudo(r.Context(), sess.ID, now); err != nil {
		s.log.Warn("sudo window not opened at login", "err", redactText(err.Error()))
	}

	s.setSessionCookie(w, r, value)
	token, err := s.setCSRFCookie(w, r)
	if err != nil {
		return err
	}
	s.audit(r, "session.create", "session", userID, "ok", "")

	sudoUntil := now.Add(store.SudoWindow)
	writeJSON(w, http.StatusOK, sessionResponse{
		Authenticated: true,
		CSRFToken:     token,
		UserID:        userID,
		SudoUntil:     &sudoUntil,
	})
	return nil
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	if err := s.store.RevokeSession(r.Context(), a.Session.ID, s.now()); err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal, "the session could not be revoked").wrapping(err)
	}
	s.audit(r, "session.revoke", "session", a.User.ID, "ok", "")
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, sessionResponse{})
	return nil
}

type sudoRequest struct {
	Password string `json:"password"`
}

// handleSudo re-authenticates and opens the 5-minute window.
func (s *Server) handleSudo(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	var req sudoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if a.User.PasswordHash == "" {
		return errStatus(http.StatusForbidden, CodeForbidden,
			"this account has no local password to confirm with")
	}
	if err := crypto.VerifyPassword(r.Context(), a.User.PasswordHash, req.Password); err != nil {
		if kdfBusy(err) {
			// Not a failed sudo: the password was never checked. Auditing it as
			// one would put "auth.sudo fail" rows in front of an operator for a
			// load problem.
			return kdfBusyStatus(err)
		}
		s.audit(r, "auth.sudo", "user", a.User.ID, "fail", "")
		return errStatus(http.StatusUnauthorized, CodeUnauthorized,
			"that password does not match").withAction("Try again")
	}
	now := s.now()

	// THE SESSION ID IS REGENERATED HERE, because opening the sudo window is a
	// privilege change and reference/security.md §6 requires regeneration at
	// every one of them. Login satisfied that by minting a new row; this
	// transition used to update the existing row in place, which carried any
	// pre-sudo cookie value across the boundary into a session that can now edit
	// services and re-enter \*Arr API keys.
	//
	// The new cookie MUST go out in this same response. Rewriting the row's id
	// without re-issuing the cookie would leave the browser holding an id no row
	// has any more — a logout, not a privilege grant.
	value, err := newToken()
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"the sudo window could not be opened").wrapping(err)
	}
	if err := s.store.RegenerateSessionForSudo(r.Context(), a.Session.ID, sessionID(value), now); err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal, "the sudo window could not be opened").wrapping(err)
	}
	s.setSessionCookie(w, r, value)

	// The CSRF token is rotated alongside it, exactly as startSession does at
	// the other privilege change. The SPA reads the non-HttpOnly cookie before
	// its cached copy (web/src/lib/api.ts csrfToken), so the cookie alone would
	// do — but the token is returned in the body as well, because a client that
	// caches the bootstrap value and never re-reads the cookie would otherwise
	// start 403-ing the moment sudo succeeded.
	token, err := s.setCSRFCookie(w, r)
	if err != nil {
		return err
	}
	s.audit(r, "auth.sudo", "user", a.User.ID, "ok", "")

	until := now.Add(store.SudoWindow)
	writeJSON(w, http.StatusOK, sessionResponse{
		Authenticated: true,
		CSRFToken:     token,
		UserID:        a.User.ID,
		Username:      a.User.Username,
		IsOwner:       a.User.IsOwner,
		SudoUntil:     &until,
	})
	return nil
}

func truncateUA(ua string) string {
	const max = 256
	if len(ua) > max {
		return ua[:max]
	}
	return ua
}
