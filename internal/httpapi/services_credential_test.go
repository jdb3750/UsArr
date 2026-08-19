package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/store"
)

// recordingTester is a ConnectionTester that passes every test and remembers the
// credential it was handed. The credential is the ONLY thing under test here, so
// nothing else about the result matters.
type recordingTester struct {
	mu   sync.Mutex
	keys []string
}

func (rt *recordingTester) Test(_ context.Context, req TestRequest) (TestResult, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.keys = append(rt.keys, req.APIKey)
	return TestResult{OK: true, Reachable: true, KeyProvenValid: true, APIVersion: "v1"}, nil
}

func (rt *recordingTester) seen() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.keys...)
}

func (rt *recordingTester) last(t *testing.T) string {
	t.Helper()
	seen := rt.seen()
	if len(seen) == 0 {
		t.Fatal("the connection tester was never called, so this test proves nothing about the credential")
	}
	return seen[len(seen)-1]
}

func newCredentialServer(t *testing.T) (*Server, *recordingTester) {
	t.Helper()
	rt := &recordingTester{}
	s := newTestServer(t, func(c *Config) { c.Tester = rt })
	return s, rt
}

// openStoredCredential decrypts what was actually sealed for a row. Asserting on
// the tester alone would miss the half that bit: the create path passed a
// trimmed copy to one and the raw field to the other.
func openStoredCredential(t *testing.T, s *Server, id int64) string {
	t.Helper()
	si, err := s.store.GetServiceInstance(context.Background(), store.OwnerScope(1), id)
	if err != nil {
		t.Fatalf("read back service %d: %v", id, err)
	}
	aad, err := crypto.ServiceInstanceAAD(si.ID, si.BaseURL)
	if err != nil {
		t.Fatalf("AAD for service %d: %v", id, err)
	}
	plain, err := s.keyring.Open(si.APIKeyEnc, aad)
	if err != nil {
		t.Fatalf("open the sealed credential for service %d: %v", id, err)
	}
	return string(plain)
}

// A pasted credential arrives with whitespace around it far more often than not
// — a double-click selection, a terminal wrap, a trailing newline from `cat`.
// `name`, `base_url` and `url_base` have always been normalised; the credential
// was the one field that was not, and create and PATCH disagreed about it, so
// the SAME paste was refused on create and silently accepted on edit. That
// asymmetry is the defect: it makes the failure unreproducible from the screen
// the user is looking at.
func TestCredentialIsTrimmedOnEveryPathThatSendsOrSealsIt(t *testing.T) {
	// One fixture, quoted into each body rather than retyped, so the value the
	// request carries and the value the assertion expects cannot drift apart.
	const padded = "  \tabc123\n "
	const want = "abc123"
	quoted := quoteJSON(padded)

	t.Run("POST /api/v1/services", func(t *testing.T) {
		s, rt := newCredentialServer(t)
		code, body := asOwner(t, s, s.handleCreateService, http.MethodPost, "/api/v1/services",
			`{"kind":"prowlarr","name":"Prowlarr","base_url":"http://prowlarr:9696",`+
				`"api_key":`+quoted+`}`, 0)
		if code != http.StatusCreated {
			t.Fatalf("create = %d, want 201: %s", code, body)
		}
		if got := rt.last(t); got != want {
			t.Errorf("the connection test was sent %q, want %q", got, want)
		}
		if got := openStoredCredential(t, s, 1); got != want {
			t.Errorf("the SEALED credential is %q, want %q — a stray newline is unrecoverable "+
				"once it is inside the envelope", got, want)
		}
	})

	t.Run("PATCH /api/v1/services/{id}", func(t *testing.T) {
		s, rt := newCredentialServer(t)
		id := seedInstance(t, s, "")
		code, body := asOwner(t, s, s.handleUpdateService, http.MethodPatch,
			"/api/v1/services/1", `{"api_key":`+quoted+`}`, id)
		if code != http.StatusOK {
			t.Fatalf("patch = %d, want 200: %s", code, body)
		}
		if got := rt.last(t); got != want {
			t.Errorf("the connection test was sent %q, want %q", got, want)
		}
		if got := openStoredCredential(t, s, id); got != want {
			t.Errorf("the SEALED credential is %q, want %q", got, want)
		}
	})

	t.Run("POST /api/v1/services/test", func(t *testing.T) {
		s, rt := newCredentialServer(t)
		code, body := asOwner(t, s, s.handleTestUnsaved, http.MethodPost, "/api/v1/services/test",
			`{"kind":"prowlarr","base_url":"http://prowlarr:9696","api_key":`+quoted+`}`, 0)
		if code != http.StatusOK {
			t.Fatalf("test-unsaved = %d, want 200: %s", code, body)
		}
		if got := rt.last(t); got != want {
			t.Errorf("the connection test was sent %q, want %q", got, want)
		}
	})

	t.Run("POST /api/v1/services/{id}/test", func(t *testing.T) {
		s, rt := newCredentialServer(t)
		id := seedInstance(t, s, "")
		code, body := asOwner(t, s, s.handleTestService, http.MethodPost,
			"/api/v1/services/1/test", `{"api_key":`+quoted+`}`, id)
		if code != http.StatusOK {
			t.Fatalf("test-saved = %d, want 200: %s", code, body)
		}
		if got := rt.last(t); got != want {
			t.Errorf("the connection test was sent %q, want %q", got, want)
		}
	})
}

// magicLinkPastes are what BookOrbit's own settings screen hands an operator.
// `getMagicUrl` in client/src/features/settings/MagicLinksSettings.vue builds
// `<origin>/magic?token=<raw>`, and the copy button beside every row is the ONLY
// way that screen surfaces a token at all — the table shows the label, the
// account, the expiry and the use count, never the raw value. So the URL is not
// a mis-paste to be corrected; it is the artefact the product means to give out,
// and BookOrbit's own public `/magic` route (router/index.ts →
// MagicLinkLoginView.vue → useAuth.loginWithMagicLink) does exactly the
// reduction below before POSTing `{"token": raw}`. Measured at
// bookorbit/bookorbit@73b7877d2fede2221b0ca360af9bfced7c3797f3, 2026-08-19.
// ADR-0067 records why this file once asserted the opposite.
var magicLinkPastes = []struct {
	name string
	in   string
	want string
}{
	{
		"the copy button's whole URL",
		"https://books.example.net/magic?token=" + strings.Repeat("a", 64),
		strings.Repeat("a", 64),
	},
	{
		"over plain http",
		"http://10.0.0.9:3000/magic?token=" + strings.Repeat("b", 64),
		strings.Repeat("b", 64),
	},
	{
		"pasted without the scheme",
		"books.example.net/magic?token=" + strings.Repeat("c", 64),
		strings.Repeat("c", 64),
	},
	{
		"with the whitespace a paste brings",
		"  https://books.example.net/magic?token=" + strings.Repeat("d", 64) + "\n",
		strings.Repeat("d", 64),
	},
	{
		// Percent-decoding is why net/url does this rather than strings.Index:
		// a link that has been through something that re-encodes query values
		// still carries the same token, and cutting the string would hand the
		// escapes to the login route verbatim.
		"a token that arrived percent-encoded",
		"https://books.example.net/magic?token=" + strings.Repeat("%61", 64),
		strings.Repeat("a", 64),
	},
	{
		"a fragment after the token",
		"https://books.example.net/magic?token=" + strings.Repeat("f", 64) + "#done",
		strings.Repeat("f", 64),
	},
}

// The bare token still has to get through, or the guard has replaced one
// unusable service with another.
const bareMagicLinkToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// What is sealed and what is sent must both be the BARE TOKEN. Sealing the URL
// would store a credential that 401s for ever, invisibly: the value is never
// returned to the browser, so nothing on any screen could show that the wrong
// half is in the envelope.
func TestBookOrbitAcceptsAPastedMagicLinkAndSealsOnlyTheToken(t *testing.T) {
	for _, tc := range magicLinkPastes {
		t.Run(tc.name, func(t *testing.T) {
			s, rt := newCredentialServer(t)
			code, body := asOwner(t, s, s.handleCreateService, http.MethodPost, "/api/v1/services",
				`{"kind":"bookorbit","name":"BookOrbit","base_url":"http://books:3000",`+
					`"api_key":`+quoteJSON(tc.in)+`}`, 0)
			if code != http.StatusCreated {
				t.Fatalf("create with a pasted magic link = %d, want 201: %s", code, body)
			}
			if got := rt.last(t); got != tc.want {
				t.Errorf("the connection test was sent %q, want the bare token %q", got, tc.want)
			}
			if got := openStoredCredential(t, s, 1); got != tc.want {
				t.Errorf("the SEALED credential is %q, want the bare token %q — a URL in the "+
					"envelope 401s for ever and cannot be read back from any screen", got, tc.want)
			}
		})
	}
}

// Every endpoint that accepts a credential does the same reduction, for the same
// reason TestEveryEndpointTakingURLBaseValidatesIt exists: a rule applied on
// three of four paths is a rule with a hole in it. Here the hole would be worse
// than an inconsistency — PATCH would seal a URL that create refuses to.
func TestEveryEndpointTakingACredentialStripsAPastedMagicLink(t *testing.T) {
	paste := magicLinkPastes[0].in
	want := magicLinkPastes[0].want

	cases := []struct {
		name    string
		handler func(*Server) handler
		method  string
		body    string
		seeded  bool
		code    int
	}{
		{
			name: "POST /api/v1/services", method: http.MethodPost, code: http.StatusCreated,
			handler: func(s *Server) handler { return s.handleCreateService },
			body: `{"kind":"bookorbit","name":"BookOrbit","base_url":"http://books:3000",` +
				`"api_key":` + quoteJSON(paste) + `}`,
		},
		{
			name: "PATCH /api/v1/services/{id}", method: http.MethodPatch, code: http.StatusOK,
			handler: func(s *Server) handler { return s.handleUpdateService },
			body:    `{"api_key":` + quoteJSON(paste) + `}`, seeded: true,
		},
		{
			name: "POST /api/v1/services/test", method: http.MethodPost, code: http.StatusOK,
			handler: func(s *Server) handler { return s.handleTestUnsaved },
			body: `{"kind":"bookorbit","base_url":"http://books:3000",` +
				`"api_key":` + quoteJSON(paste) + `}`,
		},
		{
			name: "POST /api/v1/services/{id}/test", method: http.MethodPost, code: http.StatusOK,
			handler: func(s *Server) handler { return s.handleTestService },
			body:    `{"api_key":` + quoteJSON(paste) + `}`, seeded: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, rt := newCredentialServer(t)
			var id int64
			if tc.seeded {
				id = seedBookOrbitInstance(t, s)
			}
			code, body := asOwner(t, s, tc.handler(s), tc.method, "/api/v1/services", tc.body, id)
			if code != tc.code {
				t.Fatalf("%s = %d, want %d: %s", tc.name, code, tc.code, body)
			}
			if got := rt.last(t); got != want {
				t.Errorf("%s sent %q upstream, want the bare token", tc.name, got)
			}
		})
	}
}

// notAMagicLinkCredential is everything the two accepted shapes do not cover.
// The refusal is the FALLBACK the accept path leans on: without it a mistyped
// value goes upstream and comes back 401, which is byte-identical to a revoked
// token and sends the operator to mint a replacement for a credential that was
// never the problem — the failure that started all of this.
var notAMagicLinkCredential = []struct {
	name string
	in   string
}{
	{"one character short", strings.Repeat("a", 63)},
	{"uppercase, which is not what Node's hex encoder mints", strings.ToUpper(bareMagicLinkToken)},
	{"a link with no token at all", "https://books.example.net/magic"},
	{"a link whose token is not one", "https://books.example.net/magic?token=hunter2"},
	{"a different query parameter", "https://books.example.net/magic?t=" + strings.Repeat("a", 64)},
	{
		// The strongest argument the refuse-everything rule had, kept: where a
		// paste has no single right answer, UsArr does not invent one.
		"two different tokens in one link",
		"https://books.example.net/magic?token=" + strings.Repeat("a", 64) +
			"&token=" + strings.Repeat("b", 64),
	},
}

func TestBookOrbitRefusesACredentialThatIsNeitherShape(t *testing.T) {
	for _, tc := range notAMagicLinkCredential {
		t.Run(tc.name, func(t *testing.T) {
			s, rt := newCredentialServer(t)
			code, body := asOwner(t, s, s.handleCreateService, http.MethodPost, "/api/v1/services",
				`{"kind":"bookorbit","name":"BookOrbit","base_url":"http://books:3000",`+
					`"api_key":`+quoteJSON(tc.in)+`}`, 0)
			if code != http.StatusBadRequest {
				t.Fatalf("create with %q = %d, want 400: %s", tc.name, code, body)
			}
			// §17.3 has no unactionable errors, and "invalid credential" would
			// send the operator back to BookOrbit to mint a replacement — the
			// exact wrong move. Asserted on the decoded MESSAGE rather than on
			// the whole body: `action` names the screen too, so a body-wide
			// substring check passes even when the message says nothing, which
			// is a guard that cannot fail.
			var eb errorBody
			if err := json.Unmarshal([]byte(body), &eb); err != nil {
				t.Fatalf("decode the refusal: %v: %s", err, body)
			}
			msg := strings.ToLower(eb.Message)
			if !strings.Contains(msg, "magic link") || !strings.Contains(msg, "64") {
				t.Errorf("the refusal message must name both shapes that would work: %q", eb.Message)
			}
			if eb.Action == "" {
				t.Errorf("the refusal must carry an action: %s", body)
			}
			// And nothing may have gone upstream. A value that reaches the login
			// route is precisely the 401 this fallback exists to prevent.
			if seen := rt.seen(); len(seen) != 0 {
				t.Errorf("a shape-refused credential was still sent to the connection test: %q", seen)
			}
		})
	}
}

// safeBuf is a log sink a test can read while the server writes to it.
type safeBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A REFUSED CREDENTIAL IS NEVER QUOTED BACK, anywhere. A pasted magic link
// carries two secrets at once — the token, and the operator's own BookOrbit
// hostname — and the refusal is the one place a credential could travel back to
// a browser that is otherwise never sent one (§14; the api_key is write-only on
// every shape in this file). Three sinks are checked because a message that is
// clean in the response and dirty in the audit trail has leaked just the same,
// and the audit row is the one that persists.
func TestARefusedCredentialIsNeverEchoedBack(t *testing.T) {
	const host = "books.private.example"
	paste := "https://" + host + "/magic?token=hunter2"

	logs := &safeBuf{}
	rt := &recordingTester{}
	s := newTestServer(t, func(c *Config) {
		c.Tester = rt
		c.Logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})

	code, body := asOwner(t, s, s.handleCreateService, http.MethodPost, "/api/v1/services",
		`{"kind":"bookorbit","name":"BookOrbit","base_url":"http://books:3000",`+
			`"api_key":`+quoteJSON(paste)+`}`, 0)
	if code != http.StatusBadRequest {
		t.Fatalf("create = %d, want 400: %s", code, body)
	}

	for _, sink := range []struct{ what, text string }{
		{"the response body", body},
		{"the log", logs.String()},
	} {
		for _, secret := range []string{paste, host, "hunter2"} {
			if strings.Contains(sink.text, secret) {
				t.Errorf("%s quotes %q back: %s", sink.what, secret, sink.text)
			}
		}
	}

	rows, err := s.store.ListAuditLog(t.Context(), store.OwnerScope(1), store.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	for _, row := range rows {
		for _, secret := range []string{paste, host, "hunter2"} {
			if strings.Contains(row.MetadataJSON, secret) {
				t.Errorf("audit row %q quotes %q back: %s", row.Action, secret, row.MetadataJSON)
			}
		}
	}
}

// The other direction, and it is not optional: a guard that also rejects the
// correct value has broken the feature rather than fixed it.
func TestBookOrbitAcceptsTheBareToken(t *testing.T) {
	s, rt := newCredentialServer(t)
	code, body := asOwner(t, s, s.handleCreateService, http.MethodPost, "/api/v1/services",
		`{"kind":"bookorbit","name":"BookOrbit","base_url":"http://books:3000",`+
			`"api_key":"  `+bareMagicLinkToken+`  "}`, 0)
	if code != http.StatusCreated {
		t.Fatalf("create with a bare token = %d, want 201: %s", code, body)
	}
	if got := rt.last(t); got != bareMagicLinkToken {
		t.Errorf("the connection test was sent %q, want the trimmed bare token", got)
	}
	if got := openStoredCredential(t, s, 1); got != bareMagicLinkToken {
		t.Errorf("the sealed credential is %q, want the trimmed bare token", got)
	}
}

// And the shape rule is BookOrbit's alone — both halves of it. A Prowlarr key is
// opaque to UsArr: nothing here knows enough about one to refuse it, and nothing
// here may rewrite one either. A key that happens to contain `token=` must reach
// the wire byte for byte.
func TestTheMagicLinkShapeRuleIsBookOrbitOnly(t *testing.T) {
	s, rt := newCredentialServer(t)
	weird := magicLinkPastes[0].in
	code, body := asOwner(t, s, s.handleCreateService, http.MethodPost, "/api/v1/services",
		`{"kind":"prowlarr","name":"Prowlarr","base_url":"http://prowlarr:9696",`+
			`"api_key":`+quoteJSON(weird)+`}`, 0)
	if code != http.StatusCreated {
		t.Fatalf("create = %d, want 201 — the rule must not leak onto other kinds: %s", code, body)
	}
	if got := rt.last(t); got != weird {
		t.Errorf("the connection test was sent %q, want %q untouched", got, weird)
	}
}

func seedBookOrbitInstance(t *testing.T, s *Server) int64 {
	t.Helper()
	id, err := s.store.CreateServiceInstance(context.Background(), store.ServiceInstance{
		Kind: "bookorbit", Role: "library", Name: "BookOrbit",
		BaseURL: "http://books:3000", APIKeyEnc: []byte{0}, Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("seed bookorbit instance: %v", err)
	}
	return id
}

// quoteJSON keeps the fixtures above readable: they are URLs, and hand-escaping
// each one into a JSON literal is where a test starts asserting about its own
// typing rather than about the handler.
func quoteJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
