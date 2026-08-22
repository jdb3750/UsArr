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

	// ImageCacheDir is Config.ImageCacheDir(): the directory GET /img/{key}
	// serves rendered artwork out of. Empty is honest rather than fatal —
	// principle 3 — and means this process has no image cache, so every /img
	// request answers not_cached. That is the same answer an install with an
	// empty cache directory gives. ⚠️ THIS USED TO ADD "which is every install
	// today: nothing in the tree writes an image yet", FALSIFIED 2026-08-19 BY
	// `c4a3277`: internal/libsync's phase D (covers.go) fetches covers during a
	// BookOrbit import, so a cache directory can now be non-empty. An empty one
	// is still ordinary — see internal/httpapi/images.go's header.
	ImageCacheDir string

	// SPA is the embedded frontend handler (internal/web.Handler). Nil serves a
	// 404 that says the binary was built without the frontend.
	SPA http.Handler

	Releases  ReleaseServices
	Tester    ConnectionTester
	Probes    HealthProbes
	Logger    *slog.Logger
	Now       func() time.Time
	EventsBuf int

	// Imports re-runs a catalogue import on demand. Nil is honest rather than
	// fatal: the sync endpoint answers not_configured, which is a truthful
	// thing to say about a build with no importer wired in.
	Imports CatalogueImports
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
	// "Run full sync now" (§17.3). Gated exactly like every other write on this
	// screen — CSRF, a session, and the sudo window — because a write on this
	// screen that skipped sudo would be the odd one out rather than the
	// reasonable exception. It returns 202 without waiting for the import
	// (principle 1); see imports.go for what a caller may and may not conclude
	// from that.
	//
	// ⚠️ THIS COMMENT COUNTED THE NEIGHBOURS ("its five neighbours … the sixth
	// way this screen changes something") UNTIL 2026-08-21, and the delta route
	// registered below falsified both numbers. The property is that no write
	// here is gated more weakly than the rest; a count is not that property, and
	// DEVELOPMENT.md §11 asks for the phrasing that cannot go stale.
	mux.Handle("POST /api/v1/services/{id}/sync", s.csrfProtected(s.authenticated(s.sudo(s.wrap(s.handleSyncService)))))
	// Channel 3b's arrivals walk, on demand. Gated identically to the sync above
	// it and to every other write on this screen, and the argument transfers
	// unchanged: a delta writes the same catalogue rows through the same pipeline
	// as a full import, so a gate that was right for one cannot be optional for
	// the other.
	// It returns 202 without waiting, for the full sync's reason and one more — a
	// delta can escalate to a full import, so its tail is a full import's tail;
	// see imports.go for what a caller may and may not conclude.
	//
	// The literal /sync beats nothing here and needs to beat nothing: ServeMux
	// matches path segments, so /sync and /sync/delta are separate patterns
	// rather than one shadowing the other.
	mux.Handle("POST /api/v1/services/{id}/sync/delta", s.csrfProtected(s.authenticated(s.sudo(s.wrap(s.handleDeltaSyncService)))))

	// ── Library ─────────────────────────────────────────────────────────────
	//
	// Home's Block C (§17.2, ADR-0028): one unified recently-added table across
	// every media type, keyset-paginated. A pure SQLite read, and the first
	// request the Home screen makes, so nothing on this path may block on an
	// upstream. See library.go.
	mux.Handle("GET /api/v1/library/recent", s.authenticated(s.wrap(s.handleRecentWorks)))

	// §17.2's per-type library grid and §17.8's library scope chip: the same
	// corpus and the same row as Block C, filtered by `?media_type=` and
	// `?lib=`, in one of three orders. A pure SQLite read, and the FIRST request
	// a grid screen makes, so nothing on this path may block on an upstream
	// either. ADR-0051 is why the library scope is a work-driven EXISTS rather
	// than a join. See library.go.
	//
	// ⚠️ IT IS A SEPARATE ROUTE FROM /library/recent RATHER THAN A PARAMETER ON
	// IT, AND §17.2 IS NOT WHY. This comment used to say "§17.2 closes Block C
	// at one table, one order and no filters", which is the inverse of what
	// §17.2 says: of Block C's one table it says "it sorts, it filters, it
	// Ctrl+Fs (§4.5)", and ADR-0028 puts Block C's scope on the `?lib=` chip.
	// The routes are split over the SHAPE OF THE QUERY, and
	// internal/store/browse.go owns that argument: this read is three orders,
	// two filters and a cursor codec per order, where /library/recent is one
	// unfiltered statement in one order, and folding them together would make
	// the simple statement an argument-dependent special case of the filtered
	// one.
	mux.Handle("GET /api/v1/library", s.authenticated(s.wrap(s.handleBrowseWorks)))

	// The per-type facet count: how many works of each of §17.2's six navigation
	// types the caller can see. It unblocks Block A, the media-type summary.
	// Two SQLite reads, both with a pinned plan, and no upstream call. See
	// facets.go.
	//
	// ⚠️ IT IS NOT ADR-0053's REOPENING CONDITION, though it was routed as
	// though it were. That condition needs a predicate saying WHETHER a type
	// has content; this returns per-type COUNTS, which bucket a two-format book
	// once and would hide Audiobooks from someone who has them. The condition
	// was refined rather than discharged on 2026-08-19 (ADR-0059); the sidebar
	// is unchanged either way.
	mux.Handle("GET /api/v1/library/facets", s.authenticated(s.wrap(s.handleLibraryFacets)))

	// §17.8's Libraries screen, row view: the user-defined libraries, each with
	// the containers a service already named. Two SQLite reads and no upstream
	// call — it is not the connect probe, and ADR-0048 puts the proposal set
	// there rather than in the table this serves. See libraries.go.
	mux.Handle("GET /api/v1/libraries", s.authenticated(s.wrap(s.handleListLibraries)))

	// §17.8's Accept step, read and write. The read is the proposal set — the
	// containers UsArr has been told about that are not already a library — and
	// it is served from `container_observed` rows in the local file, NOT from a
	// connect probe: ADR-0048 clause 5 excuses the probe's upstream call as a
	// setup action, and a settings screen the user navigates to is not one. So
	// neither of these blocks on an *Arr or a media server. See proposals.go.
	//
	// ⚠️ ACCEPT IS `/libraries/accept` RATHER THAN `POST /api/v1/libraries`, and
	// the second path is left unclaimed on purpose — it belongs to §17.8's
	// **Add library**, which creates one named library, where this is a batch
	// whose per-item outcome may be a JOIN into a library that already exists.
	// proposals.go's header carries the argument.
	//
	// CSRF plus a session, and NO sudo: sudo gates the writes that touch a
	// stored *Arr credential (§12.1), and this writes libraries and their
	// membership. §17.8 puts no credential on this screen at all.
	mux.Handle("GET /api/v1/libraries/proposals",
		s.authenticated(s.wrap(s.handleListLibraryProposals)))
	mux.Handle("POST /api/v1/libraries/accept",
		s.csrfProtected(s.authenticated(s.wrap(s.handleAcceptLibraries))))

	// ── Search and grab ─────────────────────────────────────────────────────
	//
	// /indexers serves the REQUESTS screen's indexer and category picker
	// (§17.5, not Search — /search is §17.4's search over the local replica and
	// has no indexer to pick). ⚠️ The parenthesis read `/search is the not-built
	// §17.4 gap screen`, asserting a build status on its own authority eleven
	// lines above the route it calls unbuilt; `web/src/routes` and this table are
	// authoritative for that, and the placement argument never needed it. It
	// reads the local replica written by the background prober and makes NO
	// upstream call, because the picker paints before the search runs. No
	// client fetches it yet; handleListIndexers in indexers.go carries the
	// argument and the wiring status.
	mux.Handle("GET /api/v1/indexers", s.authenticated(s.wrap(s.handleListIndexers)))
	mux.Handle("GET /api/v1/releases/search", s.authenticated(s.wrap(s.handleSearch)))

	// ── Search over your own library ────────────────────────────────────────
	//
	// A DIFFERENT QUESTION OVER A DIFFERENT CORPUS from the line above, and the
	// two names say so. This one reads the local FTS corpus and answers in its
	// own body; /releases/search asks Prowlarr and answers 202 with results on
	// the SSE stream. ARCHITECTURE.md §13 budgets THIS path at p50 < 15 ms,
	// which is the measurement that separated them.
	mux.Handle("GET /api/v1/search", s.authenticated(s.wrap(s.handleLibrarySearch)))
	mux.Handle("POST /api/v1/releases/{id}/grab", s.csrfProtected(s.authenticated(s.wrap(s.handleGrab))))
	// Reads from SQLite only. It is the Requests screen's memory of the one
	// irreversible action v0.1 takes (§17.5).
	mux.Handle("GET /api/v1/grabs/recent", s.authenticated(s.wrap(s.handleRecentGrabs)))

	// ── Artwork ─────────────────────────────────────────────────────────────
	//
	// ARCHITECTURE.md §4.1's `/img/{cache_key}?w={allowlisted}`, at the top
	// level rather than under /api/v1 because §4.1 lists it as a peer of the
	// JSON API and not a member of it — a browser puts this in an <img src>,
	// not in a fetch().
	//
	// AUTHENTICATED like the rest of the API, and authorized against the owning
	// item INSIDE the handler (security.md §4). The session gate here says who
	// is asking; internal/store.LookupImageAsset says whether they are entitled
	// to the work the artwork belongs to, and an /img with only the first would
	// serve every cover in the install to every account.
	//
	// ⚠️ THERE IS DELIBERATELY NO /img/public/* REGISTRATION. §4 requires that
	// genuinely public provider artwork live on a distinct path "so the
	// distinction is structural, not conditional" — and the structural half is
	// satisfied by this route having no way to express publicness at all, not by
	// a second route existing. Nothing produces provider artwork yet, and an
	// unauthenticated route with nothing behind it is a hole waiting for
	// content. See images.go.
	mux.Handle("GET /img/{key}", s.authenticated(s.wrap(s.handleImage)))

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
