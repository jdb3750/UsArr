package httpapi

import (
	"context"
	"encoding/json"
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

// magicLinkPastes are the shapes BookOrbit's own settings screen hands the user.
// `getMagicUrl` in client/src/features/settings/MagicLinksSettings.vue (measured
// at bookorbit/bookorbit@73b7877) builds `<origin>/magic?token=<raw>`, so the
// COPY BUTTON yields a URL while POST /api/v1/auth/magic-links/login wants the
// bare token. The URL is a valid string, passes BookOrbit's DTO validation,
// misses the hash lookup, and comes back 401 — identical on the wire to a
// revoked token, which is why the owner's first BookOrbit spent a connection
// test being told to mint a replacement for a token that was fine.
var magicLinkPastes = []struct {
	name string
	in   string
}{
	{"the copy button's whole URL", "https://books.example.net/magic?token=" + strings.Repeat("a", 64)},
	{"over plain http", "http://10.0.0.9:3000/magic?token=" + strings.Repeat("b", 64)},
	{"pasted without the scheme", "books.example.net/magic?token=" + strings.Repeat("c", 64)},
	{"with the whitespace a paste brings", "  https://books.example.net/magic?token=" + strings.Repeat("d", 64) + "\n"},
}

// The bare token still has to get through, or the guard has replaced one
// unusable service with another.
const bareMagicLinkToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestBookOrbitRefusesAMagicLinkURLBySHAPEAndSaysWhichHalfToPaste(t *testing.T) {
	for _, tc := range magicLinkPastes {
		t.Run(tc.name, func(t *testing.T) {
			s, rt := newCredentialServer(t)
			code, body := asOwner(t, s, s.handleCreateService, http.MethodPost, "/api/v1/services",
				`{"kind":"bookorbit","name":"BookOrbit","base_url":"http://books:3000",`+
					`"api_key":`+quoteJSON(tc.in)+`}`, 0)
			if code != http.StatusBadRequest {
				t.Fatalf("create with %q = %d, want 400: %s", tc.in, code, body)
			}
			// The message has to name the operation, not merely refuse. §17.3
			// has no unactionable errors, and "invalid credential" would send
			// the user back to BookOrbit to mint a replacement — the exact wrong
			// move, and the one the old wording recommended.
			if !strings.Contains(body, "token=") {
				t.Errorf("the refusal must name the query parameter to cut after: %s", body)
			}
			if !strings.Contains(strings.ToLower(body), "magic-link url") {
				t.Errorf("the refusal must say what was pasted: %s", body)
			}
			// And nothing may have gone upstream. A URL that reaches the login
			// route is precisely the 401 this guard exists to prevent.
			if seen := rt.seen(); len(seen) != 0 {
				t.Errorf("a shape-refused credential was still sent to the connection test: %q", seen)
			}
		})
	}
}

// Every endpoint that accepts a credential enforces the shape, for the same
// reason TestEveryEndpointTakingURLBaseValidatesIt exists: a rule enforced on
// three of four paths is a rule with a hole in it.
func TestEveryEndpointTakingACredentialRefusesAMagicLinkURL(t *testing.T) {
	paste := magicLinkPastes[0].in

	cases := []struct {
		name    string
		handler func(*Server) handler
		method  string
		body    string
		seeded  bool
	}{
		{
			name: "POST /api/v1/services", method: http.MethodPost,
			handler: func(s *Server) handler { return s.handleCreateService },
			body: `{"kind":"bookorbit","name":"BookOrbit","base_url":"http://books:3000",` +
				`"api_key":` + quoteJSON(paste) + `}`,
		},
		{
			name: "PATCH /api/v1/services/{id}", method: http.MethodPatch,
			handler: func(s *Server) handler { return s.handleUpdateService },
			body:    `{"api_key":` + quoteJSON(paste) + `}`, seeded: true,
		},
		{
			name: "POST /api/v1/services/test", method: http.MethodPost,
			handler: func(s *Server) handler { return s.handleTestUnsaved },
			body: `{"kind":"bookorbit","base_url":"http://books:3000",` +
				`"api_key":` + quoteJSON(paste) + `}`,
		},
		{
			name: "POST /api/v1/services/{id}/test", method: http.MethodPost,
			handler: func(s *Server) handler { return s.handleTestService },
			body:    `{"api_key":` + quoteJSON(paste) + `}`, seeded: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newCredentialServer(t)
			var id int64
			if tc.seeded {
				id = seedBookOrbitInstance(t, s)
			}
			code, body := asOwner(t, s, tc.handler(s), tc.method, "/api/v1/services", tc.body, id)
			if code != http.StatusBadRequest || !strings.Contains(body, "token=") {
				t.Fatalf("%s = %d, want 400 naming token=: %s", tc.name, code, body)
			}
		})
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

// And the shape rule is BookOrbit's alone. A Prowlarr key is opaque to UsArr and
// nothing here knows enough about one to refuse it.
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
