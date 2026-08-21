package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBookOrbit is a BookOrbit-shaped test double for the cmd-level gate tests.
//
// ⚠️ IT IS A FAKE, NOT A BOOKORBIT, and internal/bookorbit's fake_test.go says
// why that distinction has to be stated rather than assumed: no live BookOrbit
// is available to this repository, so a fake proves THIS BUILD'S behaviour and
// never the server's. The routes and shapes are taken from BookOrbit's own
// source at bookorbit/bookorbit@73b7877d — auth.controller.ts,
// health.controller.ts, app-info.controller.ts, AuthService.buildUserResponse,
// @nestjs/terminus's HealthCheckResult and common/filters/http-exception.filter.ts.
//
// It is a SECOND fake rather than internal/bookorbit's, because that one lives
// in package bookorbit and these tests are package main. What it models is the
// three things that decide whether the connection test tells the truth, none of
// which the client's own tests can reach from here:
//
//  1. GET /api/v1/health is @Public(): it answers with NO credential, and it
//     will happily answer for a bad magic-link token. A test that stopped there
//     would store a broken credential and report success.
//  2. POST /api/v1/auth/magic-links/login takes the token in the BODY and 401s
//     an unknown one. It is the only thing that can prove the credential.
//  3. GET /api/v1/app-info needs a bearer and no permission at all, so it is
//     where "the token minted but the account cannot make an ordinary call"
//     becomes visible.
type fakeBookOrbit struct {
	t     *testing.T
	token string

	srv *httptest.Server

	mu     sync.Mutex
	seen   []boRequest
	issued []string

	// user is what the login response reports. The zero-value account built by
	// newFakeBookOrbit is the CORRECTLY SCOPED one: not a superuser, provisioned
	// 'shared', no permissions at all. A test that wants the §14 warning
	// mutates it before the instance is added.
	user map[string]any

	// health, when non-nil, replaces the Terminus report. Set before the
	// instance is added.
	health map[string]any
	// healthStatus is the status code served with it. 503 is what Terminus
	// answers when an indicator is down, and the BODY is the same shape.
	healthStatus int

	// appInfoStatus, when non-zero, makes app-info answer that status instead of
	// the app info — the "valid token, refused bearer" case.
	appInfoStatus int
	// appInfoMessage rides with it.
	appInfoMessage string

	// version is what a successful app-info reports.
	version string

	// ── the catalogue half ──────────────────────────────────────────────────
	//
	// libraries is what GET /api/v1/libraries answers, in order. The shape is
	// LibraryRepository.findAllForUser's — the NON-SUPERUSER projection, which
	// is the one UsArr's shared viewer account actually receives, and it is
	// narrower than the whole row a superuser gets.
	libraries []map[string]any

	// books is the book cards, keyed by library id. Each is one BookCard as
	// book/utils/assemble-book-cards.ts emits it.
	//
	// ⚠️ THERE IS NO "serve a smaller page than was asked for" KNOB, and the
	// omission is deliberate. A real BookOrbit serves exactly the requested size
	// until the last page, and the walk's stop condition is a SHORT page — so a
	// fake that shrank pages on its own would let a broken stop condition pass
	// here and hang in production. The paging loop is crossed against
	// internal/bookorbit's own fake instead, with a generated library larger
	// than one page.
	books map[int][]map[string]any

	// stats is what GET /libraries/{id}/stats answers for `totalBooks`, keyed by
	// library id. A library with no entry answers its OWN declared bookCount,
	// i.e. no content filter — so a test has to opt into a shortfall rather than
	// stumble into one.
	stats map[int]int

	// statsStatus makes the stats route answer an error status instead, which is
	// how the guard-later scenario is modelled: BookOrbit adds a
	// @RequirePermission to a route that carries none today, and every probe
	// starts answering 403.
	statsStatus map[int]int

	// queries is every book listing the fake was asked for, in order, with the
	// filter tree DECODED.
	//
	// 🚩 WITHOUT THIS, AN ARRIVALS WALK AND A FULL PAGE WALK ARE THE SAME
	// REQUEST HERE. The two differ in exactly one place — channel 1 sends no
	// `filter`, channel 3b sends one `after(addedAt, cursor)` rule — and a fake
	// that neither parsed nor applied it made every delta assertion pass for a
	// walk that had sent no filter at all. `record` above captures the raw body,
	// but a raw body is not an assertion anybody writes.
	queries []boBookQuery
}

// boBookQuery is one decoded POST /libraries/{id}/books, reduced to the parts a
// test asks about. The zero Filter means NO filter was sent, which is channel
// 1's request and is a different fact from a filter that matched nothing.
type boBookQuery struct {
	LibraryID int
	Page      int
	Size      int
	// SortField and SortDir are the FIRST sort tier, which is the only one
	// either walk sends.
	SortField string
	SortDir   string
	// Filter is nil unless the request carried one.
	Filter *boBookFilter
}

// boBookFilter is the one rule shape either walk sends, flattened out of
// BookQueryPipe's group-of-rules tree. A request whose tree is anything else
// fails the fake rather than being reduced to this.
type boBookFilter struct {
	Field    string
	Operator string
	Value    string
}

// boBook builds one BookCard. Field names and null-ability are
// packages/types/src/book.ts's; the defaults are the ordinary case — one primary
// epub, present, no identifiers matched.
func boBook(id int, title string, extra map[string]any) map[string]any {
	b := map[string]any{
		"id": id, "status": "present", "coverAspectRatio": "book",
		"title": title, "subtitle": nil,
		"authors": []string{}, "narrators": []string{},
		"seriesId": nil, "seriesName": nil, "seriesIndex": nil, "seriesMemberships": []any{},
		"files": []map[string]any{
			{"id": id*10 + 1, "format": "epub", "role": "primary", "sizeBytes": 1234567},
		},
		"publishedDate": nil, "publishedYear": nil, "language": nil,
		"genres": []string{}, "tags": []string{},
		"rating": nil, "readingProgress": nil, "readStatus": nil,
		"addedAt": "2026-08-01T10:00:00.000Z", "updatedAt": "2026-08-17T07:00:30.118Z",
		"metadataScore": nil, "hasCover": true,
		"hasMetadataLocks": false, "lockedFields": []string{},
		"publisher": nil, "pageCount": nil,
		"isbn13": nil, "hardcoverId": nil, "hardcoverEditionId": nil,
		"customMetadata": []any{},
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// boFile is one BookFileRef, for a test that needs a format other than epub.
func boFile(id int, format, role string) map[string]any {
	return map[string]any{"id": id, "format": format, "role": role, "sizeBytes": 4096}
}

type boRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   string
}

func newFakeBookOrbit(t *testing.T, token string) *fakeBookOrbit {
	t.Helper()
	f := &fakeBookOrbit{
		t: t, token: token, healthStatus: http.StatusOK, version: "v1.4.2",
		user: map[string]any{
			"id": 7, "username": "usarr", "name": "UsArr service account",
			"email": "shared@example.invalid", "active": true,
			"isSuperuser": false, "isDefaultPassword": false,
			"settings": map[string]any{"theme": "dark"}, "avatarUrl": nil,
			"provisioningMethod": "shared", "permissions": []string{},
		},
	}

	mux := http.NewServeMux()

	// @Public() at class level: registered deliberately WITHOUT any credential
	// check, because that is the property the reachability probe depends on.
	mux.HandleFunc("GET /api/v1/health", f.record(func(w http.ResponseWriter, r *http.Request) {
		body := f.health
		if body == nil {
			body = map[string]any{
				"status":  "ok",
				"info":    map[string]any{"database": map[string]any{"status": "up"}},
				"error":   map[string]any{},
				"details": map[string]any{"database": map[string]any{"status": "up"}},
			}
		}
		boWriteJSON(w, f.healthStatus, body)
	}))

	// @Public() and @Throttle({limit:10, ttl:60_000}). The token arrives in the
	// BODY and nowhere else.
	mux.HandleFunc("POST /api/v1/auth/magic-links/login", f.record(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		blob, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(blob, &req); err != nil || req.Token == "" {
			boNestError(w, r, http.StatusBadRequest, "token must be a string")
			return
		}
		if req.Token != f.token {
			// ONE status and ONE body is the whole stimulus this branch needs,
			// because that is all UsArr's side can distinguish: parseErrorBody
			// maps every 401 to the single ErrUnauthorized sentinel and branches
			// on nothing, unlike its 403 arm, which splits one status into two
			// sentinels on an exact message match. There is no second 401 shape
			// worth faking because there is no second 401 sentinel to reach.
			//
			// ⚠️ This comment used to assert that loginWithToken collapses five
			// upstream conditions into one message "on purpose". Those were its
			// words; neither the enumeration nor the intent is checkable from
			// this tree, since BookOrbit's server source is not vendored here.
			// It also disagreed with the same claim in internal/bookorbit's
			// errors.go about what the fifth condition was — "orphaned" here,
			// "belongs to an inactive user" there. Upstream INTENT is UNKNOWN;
			// the ordered enumeration belongs to internal/bookorbit/doc.go,
			// which cites bookorbit@73b7877d for it. The status and body below
			// are unchanged.
			boNestError(w, r, http.StatusUnauthorized, "Invalid or expired magic link")
			return
		}
		tok := boFreshJWT(time.Now().Add(15 * time.Minute))
		f.mu.Lock()
		f.issued = append(f.issued, tok)
		user := f.user
		f.mu.Unlock()
		boWriteJSON(w, http.StatusOK, map[string]any{"accessToken": tok, "user": user})
	}))

	// Behind the global JwtAuthGuard, with no @RequirePermission of its own.
	mux.HandleFunc("GET /api/v1/app-info", f.record(func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			boNestError(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		if f.appInfoStatus != 0 {
			boNestError(w, r, f.appInfoStatus, f.appInfoMessage)
			return
		}
		boWriteJSON(w, http.StatusOK, map[string]any{
			"version": f.version, "updateAvailable": false,
			"latestVersion": f.version, "maxUploadSizeMb": 200,
		})
	}))

	// LibraryController's @Get(): no @RequirePermission, scoped INSIDE the
	// service to what the account was granted. The projection here is
	// findAllForUser's, not findAll's.
	mux.HandleFunc("GET /api/v1/libraries", f.record(func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			boNestError(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		out := f.libraries
		if out == nil {
			out = []map[string]any{}
		}
		boWriteJSON(w, http.StatusOK, out)
	}))

	// LibraryController's @Post(':id/books') with @RequireLibraryAccess('viewer').
	// A POST that READS: it takes a body because the filter tree is too large
	// for a query string, not because it mutates anything.
	mux.HandleFunc("POST /api/v1/libraries/{id}/books", f.record(func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			boNestError(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		libID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			// ParseIntPipe's own answer.
			boNestError(w, r, http.StatusBadRequest, "Validation failed (numeric string is expected)")
			return
		}
		var q struct {
			Sort []struct {
				Field string `json:"field"`
				Dir   string `json:"dir"`
			} `json:"sort"`
			Pagination struct {
				Page int `json:"page"`
				Size int `json:"size"`
			} `json:"pagination"`
			CollapseSeries *bool `json:"collapseSeries"`
			// The filter tree, as BookQueryPipe validates it: a boolean group
			// holding rules. Only the shape either walk actually sends is
			// modelled — see boBookFilter.
			Filter *struct {
				Type  string `json:"type"`
				Join  string `json:"join"`
				Rules []struct {
					Type     string `json:"type"`
					Field    string `json:"field"`
					Operator string `json:"operator"`
					Value    string `json:"value"`
				} `json:"rules"`
			} `json:"filter"`
		}
		blob, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(blob, &q); err != nil {
			boNestError(w, r, http.StatusBadRequest, "Unexpected token in JSON")
			return
		}
		rec := boBookQuery{LibraryID: libID, Page: q.Pagination.Page, Size: q.Pagination.Size}
		if len(q.Sort) > 0 {
			rec.SortField, rec.SortDir = q.Sort[0].Field, q.Sort[0].Dir
		}
		if q.Filter != nil {
			// One AND group holding one rule is the whole filter this channel
			// sends. Anything else is a shape the fake cannot honestly serve,
			// and serving the unfiltered list for it would be the silent wrong
			// answer this recording exists to prevent.
			if len(q.Filter.Rules) != 1 {
				boNestError(w, r, http.StatusBadRequest,
					"this fake models exactly one filter rule; a walk that sends a tree must teach it the tree")
				return
			}
			rec.Filter = &boBookFilter{
				Field:    q.Filter.Rules[0].Field,
				Operator: q.Filter.Rules[0].Operator,
				Value:    q.Filter.Rules[0].Value,
			}
		}
		f.mu.Lock()
		f.queries = append(f.queries, rec)
		f.mu.Unlock()
		// BookQueryPipe's zod bounds, enforced rather than assumed: a client
		// that asked for 500 must see the 400 a real BookOrbit answers.
		if q.Pagination.Size < 1 || q.Pagination.Size > 200 {
			boNestError(w, r, http.StatusBadRequest, "pagination.size must be between 1 and 200")
			return
		}
		if q.Pagination.Page < 0 {
			boNestError(w, r, http.StatusBadRequest, "pagination.page must be >= 0")
			return
		}
		// ⚠️ COLLAPSING WOULD RETURN ONE ROW PER SERIES, NOT ONE PER BOOK. The
		// fake refuses rather than silently serving the same list, so an adapter
		// that ever started sending it fails here instead of importing a
		// catalogue with the wrong cardinality and no symptom.
		if q.CollapseSeries != nil && *q.CollapseSeries {
			boNestError(w, r, http.StatusBadRequest,
				"this fake does not model collapseSeries; a catalogue replica must not ask for it")
			return
		}

		all := f.books[libID]
		if rec.Filter != nil {
			// ⚠️ THE FILTER IS APPLIED, NOT JUST RECORDED. A fake that recorded
			// `after(addedAt, X)` and then served rows at or below X would be
			// modelling a server that ignores its own query language, and every
			// walk built on it would look correct here and re-read the whole
			// library in production.
			//
			// `after` is BookQueryBuilder's STRICT gt on books.addedAt, and the
			// comparison is a plain string compare because the stamps are
			// ISO-8601 UTC with fixed-width fields, where lexical order IS
			// chronological order. That equivalence is the whole reason the
			// cursor is stored as text; see internal/store's
			// ArrivalsWatermarkLayout.
			if rec.Filter.Field != "addedAt" || rec.Filter.Operator != "after" {
				boNestError(w, r, http.StatusBadRequest,
					"this fake models only after(addedAt, …); FIELD_OPERATORS has more and none of them is sent")
				return
			}
			kept := make([]map[string]any, 0, len(all))
			for _, b := range all {
				if added, ok := b["addedAt"].(string); ok && added > rec.Filter.Value {
					kept = append(kept, b)
				}
			}
			all = kept
		}
		size := q.Pagination.Size
		start := q.Pagination.Page * size
		if start > len(all) {
			start = len(all)
		}
		end := start + size
		if end > len(all) {
			end = len(all)
		}
		items := all[start:end]
		if items == nil {
			items = []map[string]any{}
		}
		boWriteJSON(w, http.StatusOK, map[string]any{
			"items": items, "total": len(all), "page": q.Pagination.Page, "size": size,
		})
	}))

	// LibraryController's @Get(':id/stats').
	//
	// ⚠️ IT CARRIES @RequireLibraryAccess('viewer') AND NO @RequirePermission,
	// which is the property the whole completeness check rests on — so the fake
	// models exactly that: a bearer is required, a permission is not. `statsStatus`
	// is how a test makes it behave like a route that HAS been guarded since.
	//
	// getStats takes neither a user nor a content filter (library.repository.ts
	// getStats), so `totalBooks` here is the UNFILTERED present-book count and is
	// deliberately not derived from whatever the listing reported.
	mux.HandleFunc("GET /api/v1/libraries/{id}/stats", f.record(func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			boNestError(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		libID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			boNestError(w, r, http.StatusBadRequest, "Validation failed (numeric string is expected)")
			return
		}
		if status, ok := f.statsStatus[libID]; ok {
			// LibraryAccessGuard's own message for a viewer with no access row,
			// and RolesGuard's for a missing permission, are both plain strings.
			boNestError(w, r, status, "Forbidden resource")
			return
		}
		total, ok := f.stats[libID]
		if !ok {
			total = f.declaredBookCount(libID)
		}
		boWriteJSON(w, http.StatusOK, map[string]any{
			"totalBooks": total, "totalSizeBytes": 1234567, "formatCounts": map[string]int{"epub": total},
		})
	}))

	mux.HandleFunc("/", f.record(func(w http.ResponseWriter, r *http.Request) {
		boNestError(w, r, http.StatusNotFound, "Cannot "+r.Method+" "+r.URL.Path)
	}))

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBookOrbit) URL() string { return f.srv.URL }

// declaredBookCount is what this fake's own listing reported for one library —
// the default the stats route answers when a test has not asked for a shortfall.
func (f *fakeBookOrbit) declaredBookCount(libID int) int {
	for _, l := range f.libraries {
		id, _ := l["id"].(int)
		if id != libID {
			continue
		}
		n, _ := l["bookCount"].(int)
		return n
	}
	return 0
}

// record captures every request BEFORE handing it on, including the body. The
// body is the point: it is what lets a test prove the magic-link token travelled
// in one place and one place only.
func (f *fakeBookOrbit) record(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		blob, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		f.mu.Lock()
		f.seen = append(f.seen, boRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Auth: r.Header.Get("Authorization"), Body: string(blob),
		})
		f.mu.Unlock()
		r.Body = io.NopCloser(strings.NewReader(string(blob)))
		next(w, r)
	}
}

// bookQueries is every decoded book listing the fake was asked for, in order.
func (f *fakeBookOrbit) bookQueries() []boBookQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]boBookQuery(nil), f.queries...)
}

func (f *fakeBookOrbit) requests() []boRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]boRequest(nil), f.seen...)
}

func (f *fakeBookOrbit) issuedTokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.issued...)
}

func (f *fakeBookOrbit) bearerOK(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || got == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.issued {
		if t == got {
			return true
		}
	}
	return false
}

// boFreshJWT builds a structurally real JWT — three unpadded base64url segments
// — generated fresh on every run, for the reason internal/bookorbit's fake
// gives: a constant would be a credential-shaped literal in the repository, and
// a probe that is identical every run cannot show that a leak is THIS run's
// value.
func boFreshJWT(exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"sub": 7, "ver": 1, "iat": exp.Add(-15 * time.Minute).Unix(), "exp": exp.Unix(),
	})
	if err != nil {
		panic("encoding fixture JWT claims: " + err.Error())
	}
	sig := make([]byte, 32)
	if _, err := rand.Read(sig); err != nil {
		panic("no entropy for the fixture JWT: " + err.Error())
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

func boWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// boNestError is GlobalExceptionFilter's envelope.
func boNestError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	boWriteJSON(w, status, map[string]any{
		"statusCode": status, "message": msg,
		"path": r.URL.RequestURI(), "timestamp": "2026-08-19T00:00:00.000Z",
		"requestId": "req-1",
	})
}
