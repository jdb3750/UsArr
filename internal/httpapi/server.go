package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/store"
)

// BuildInfo is what GET /api/v1/system/status reports about this binary. cmd
// fills it from the -ldflags-stamped variables.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

// Config assembles the server. Everything is required except the optional
// collaborators, each of which degrades honestly rather than panicking: with no
// ConnectionTester the test endpoint says the binary was built without one,
// which is a truthful thing to say and a debuggable one.
type Config struct {
	Store *store.Store

	// Keyring seals and opens service credentials. Required: without it a
	// service cannot be saved, and pretending otherwise would store a
	// plaintext key.
	Keyring *crypto.Keyring

	// GrabRowIDKey is crypto.DeriveGrabRowIDKey's output: the HMAC key behind
	// the opaque row identity the Recent-grabs read publishes instead of the
	// raw provenance rowid. Required, and required for the same reason the
	// Keyring is — the fallback for a missing key would be shipping the rowid,
	// which is the leak the key exists to close (review finding RG-01.3).
	//
	// It must be the SAME bytes across restarts, which it is because it is
	// derived from the master key and the KEK salt: a random per-process key
	// would give a client a different id for the same row on every poll.
	GrabRowIDKey []byte

	// AuditRowIDKey is crypto.DeriveAuditRowIDKey's output, and it does for the
	// definite-failure arm of Recent grabs what GrabRowIDKey does for the
	// handed-over arm: audit_log.id is an INTEGER PRIMARY KEY and therefore the
	// same cross-user volume oracle.
	//
	// A SEPARATE KEY, not a second use of GrabRowIDKey. The two ids are drawn
	// from two independent sequences, so provenance row 41 and audit row 41 are
	// unrelated rows that would share a token under one key — and one response
	// carries both arms, so the client would key two rows on one id. See
	// crypto's infoAuditRowID.
	AuditRowIDKey []byte

	// SchemaVersion is the migration version applied at startup. Readiness is
	// "migrations applied and the listener accepting", and this is the first
	// half — recorded once, not re-queried, because ready must touch nothing.
	SchemaVersion int64

	// URLBase is USARR_URL_BASE: a leading slash and no trailing slash, or
	// empty. Every route is served under it.
	URLBase string

	// TrustedProxies is USARR_TRUSTED_PROXIES. Empty trusts nothing, which is
	// the default and is correct for a direct-port deployment.
	TrustedProxies []netip.Prefix

	Build BuildInfo

	// SPA is the embedded frontend handler (internal/web.Handler). Nil serves a
	// 404 that says the binary was built without the frontend.
	SPA http.Handler

	Releases  ReleaseServices
	Tester    ConnectionTester
	Probes    HealthProbes
	Logger    *slog.Logger
	Now       func() time.Time
	EventsBuf int
}

// Server is the HTTP surface. One per process.
type Server struct {
	cfg Config
	log *slog.Logger
	now func() time.Time

	store   *store.Store
	keyring *crypto.Keyring
	hub     *Hub

	handler http.Handler

	// listening is the second half of readiness. cmd flips it once Listen has
	// returned a bound socket, and back off when the drain starts, so a
	// draining process reports not-ready and a load balancer stops sending it
	// work before it stops answering.
	listening atomic.Bool

	startedAt time.Time
}

// New builds the server and its route table.
func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("httpapi: Store is required")
	}
	if cfg.Keyring == nil {
		return nil, fmt.Errorf("httpapi: Keyring is required")
	}
	if len(cfg.GrabRowIDKey) != crypto.DerivedKeyLen {
		return nil, fmt.Errorf("httpapi: GrabRowIDKey is %d bytes, want %d",
			len(cfg.GrabRowIDKey), crypto.DerivedKeyLen)
	}
	if len(cfg.AuditRowIDKey) != crypto.DerivedKeyLen {
		return nil, fmt.Errorf("httpapi: AuditRowIDKey is %d bytes, want %d",
			len(cfg.AuditRowIDKey), crypto.DerivedKeyLen)
	}
	// The two keys must be different bytes, and that is checked rather than
	// assumed: wiring the same derived key into both fields is a one-line
	// mistake at the call site that compiles, runs, and silently collapses the
	// two id domains into one.
	if bytes.Equal(cfg.GrabRowIDKey, cfg.AuditRowIDKey) {
		return nil, fmt.Errorf("httpapi: GrabRowIDKey and AuditRowIDKey are the same key; " +
			"the provenance and audit row ids would share a keyspace")
	}
	if cfg.URLBase != "" {
		if !strings.HasPrefix(cfg.URLBase, "/") || strings.HasSuffix(cfg.URLBase, "/") {
			return nil, fmt.Errorf("httpapi: URLBase %q must have a leading slash and no trailing slash", cfg.URLBase)
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.EventsBuf <= 0 {
		cfg.EventsBuf = defaultEventBuffer
	}

	s := &Server{
		cfg:       cfg,
		log:       cfg.Logger,
		now:       cfg.Now,
		store:     cfg.Store,
		keyring:   cfg.Keyring,
		hub:       NewHub(cfg.EventsBuf, cfg.Now),
		startedAt: cfg.Now(),
	}
	s.handler = s.buildHandler()
	return s, nil
}

// Handler is the root http.Handler, already mounted under URLBase.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// MarkListening records that the listener is accepting. Readiness is false until
// it is called and false again after Draining.
func (s *Server) MarkListening() { s.listening.Store(true) }

// Draining flips readiness off at the start of a graceful shutdown, so in-flight
// requests still complete but a health check stops saying "send me traffic".
func (s *Server) Draining() { s.listening.Store(false) }

// Events is the SSE hub, so background work started elsewhere in the process can
// publish onto the one stream.
func (s *Server) Events() *Hub { return s.hub }

// Close releases the SSE subscribers. Call it after the HTTP server has drained;
// an SSE connection never ends on its own.
func (s *Server) Close() { s.hub.Close() }

// buildHandler assembles the route table and mounts it under URLBase.
//
// USARR_URL_BASE has to work on EVERY route, not just the SPA's asset paths: a
// reverse proxy at /usarr rewrites nothing, so /usarr/api/events must route.
// StripPrefix in front of one mux is the whole mechanism, and it is here rather
// than sprinkled through the patterns so a new route cannot forget it.
func (s *Server) buildHandler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)

	var root http.Handler = mux
	if s.cfg.URLBase != "" {
		outer := http.NewServeMux()
		outer.Handle(s.cfg.URLBase+"/", http.StripPrefix(s.cfg.URLBase, mux))
		// A bare /usarr with no trailing slash is the link people paste.
		outer.Handle(s.cfg.URLBase, http.RedirectHandler(s.cfg.URLBase+"/", http.StatusMovedPermanently))
		root = outer
	}

	// Order matters and is the point of the comments on each line.
	//
	//  1. recover, so a panic below produces a 500 and a log line rather than a
	//     dropped connection.
	//  2. redact, so everything after it — every log line, every error string,
	//     every SSE frame — can only reach the redacted request line.
	//  3. clientIP, which decides whether a forwarded header is believed at all.
	//  4. securityHeaders, applied to every response including errors.
	//  5. accessLog, which must sit below redact and can therefore not leak.
	return s.recoverMiddleware(
		redactMiddleware(
			s.clientIPMiddleware(
				s.securityHeadersMiddleware(
					s.accessLogMiddleware(root)))))
}

func (s *Server) routes(mux *http.ServeMux) {
	// ── Health ──────────────────────────────────────────────────────────────
	//
	// Unauthenticated by design: these are the container's healthcheck and they
	// have to answer before there is a session, a user or a library. Neither
	// discloses anything — live touches nothing at all and ready reports two
	// booleans this process already knows.
	mux.HandleFunc("GET /api/health/live", s.handleLive)
	mux.HandleFunc("GET /api/health/ready", s.handleReady)

	// ── System ──────────────────────────────────────────────────────────────
	mux.Handle("GET /api/v1/system/status", s.authenticated(s.wrap(s.handleSystemStatus)))

	// ── Auth ────────────────────────────────────────────────────────────────
	//
	// Not in the endpoint list in ARCHITECTURE.md §4.1, and required by it: the
	// session, CSRF and sudo rules in §14/§6 are meaningless without somewhere
	// to establish a session and somewhere to re-authenticate.
	mux.Handle("GET /api/v1/auth/session", s.wrap(s.handleSession))
	mux.Handle("POST /api/v1/auth/setup", s.csrfProtected(s.wrap(s.handleSetup)))
	mux.Handle("POST /api/v1/auth/login", s.csrfProtected(s.wrap(s.handleLogin)))
	mux.Handle("POST /api/v1/auth/logout", s.csrfProtected(s.authenticated(s.wrap(s.handleLogout))))
	mux.Handle("POST /api/v1/auth/sudo", s.csrfProtected(s.authenticated(s.wrap(s.handleSudo))))

	// ── Service instances ───────────────────────────────────────────────────
	//
	// The literal /services/health beats the /services/{id} wildcard in
	// ServeMux's specificity rules, so the order of registration here does not
	// matter — but the two are adjacent so nobody has to remember that.
	mux.Handle("GET /api/v1/services", s.authenticated(s.wrap(s.handleListServices)))
	mux.Handle("POST /api/v1/services", s.csrfProtected(s.authenticated(s.sudo(s.wrap(s.handleCreateService)))))
	mux.Handle("GET /api/v1/services/health", s.authenticated(s.wrap(s.handleServicesHealth)))
	mux.Handle("POST /api/v1/services/test", s.csrfProtected(s.authenticated(s.sudo(s.wrap(s.handleTestUnsaved)))))
	mux.Handle("GET /api/v1/services/{id}", s.authenticated(s.wrap(s.handleGetService)))
	mux.Handle("PATCH /api/v1/services/{id}", s.csrfProtected(s.authenticated(s.sudo(s.wrap(s.handleUpdateService)))))
	mux.Handle("DELETE /api/v1/services/{id}", s.csrfProtected(s.authenticated(s.sudo(s.wrap(s.handleDeleteService)))))
	mux.Handle("POST /api/v1/services/{id}/test", s.csrfProtected(s.authenticated(s.sudo(s.wrap(s.handleTestService)))))

	// ── Library ─────────────────────────────────────────────────────────────
	//
	// Home's Block C (§17.2, ADR-0028): one unified recently-added table across
	// every media type, keyset-paginated. A pure SQLite read, and the first
	// request the Home screen makes, so nothing on this path may block on an
	// upstream. See library.go.
	mux.Handle("GET /api/v1/library/recent", s.authenticated(s.wrap(s.handleRecentWorks)))

	// ── Search and grab ─────────────────────────────────────────────────────
	//
	// /indexers serves the REQUESTS screen's indexer and category picker
	// (§17.5, not Search — /search is the not-built §17.4 gap screen). It
	// reads the local replica written by the background prober and makes NO
	// upstream call, because the picker paints before the search runs. No
	// client fetches it yet; handleListIndexers in indexers.go carries the
	// argument and the wiring status.
	mux.Handle("GET /api/v1/indexers", s.authenticated(s.wrap(s.handleListIndexers)))
	mux.Handle("GET /api/v1/search", s.authenticated(s.wrap(s.handleSearch)))
	mux.Handle("POST /api/v1/releases/{id}/grab", s.csrfProtected(s.authenticated(s.wrap(s.handleGrab))))
	// Reads from SQLite only. It is the Requests screen's memory of the one
	// irreversible action v0.1 takes (§17.5).
	mux.Handle("GET /api/v1/grabs/recent", s.authenticated(s.wrap(s.handleRecentGrabs)))

	// ── The one SSE stream ──────────────────────────────────────────────────
	mux.Handle("GET /api/events", s.authenticated(s.wrap(s.handleEvents)))

	// ── The SPA ─────────────────────────────────────────────────────────────
	//
	// Registered last and at "/" so it is the fallback for everything that is
	// not an API route. Deep routes (/search, /library/movies/12345) resolve to
	// index.html inside internal/web.
	mux.Handle("/", s.spaHandler())

	// An unknown /api/ path must 404 as JSON, not as the SPA document: handing
	// HTML to fetch() produces an unexplainable parse error in the console.
	mux.Handle("/api/", s.wrap(func(w http.ResponseWriter, r *http.Request) error {
		return errStatus(http.StatusNotFound, CodeNotFound, "no such endpoint: "+RequestLine(r.Context()))
	}))
}

func (s *Server) spaHandler() http.Handler {
	if s.cfg.SPA != nil {
		return s.cfg.SPA
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w,
			"the UsArr frontend is not embedded in this binary: build it with `make build`",
			http.StatusNotFound)
	})
}
