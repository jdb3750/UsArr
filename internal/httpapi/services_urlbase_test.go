package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// url_base is a per-instance reverse-proxy sub-path, and cmd/usarr/services.go
// builds the upstream address by plain concatenation — BaseURL + URLBase. So a
// value that is not a path is not a cosmetic problem: `prowlarr` where
// `/prowlarr` was meant produced http://host:9696prowlarr, which resolves to
// nothing and fails later, somewhere else, as an instance that is "down".
func TestServiceURLBaseNormalisation(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty means the root", "", ""},
		{"whitespace is not a sub-path", "   ", ""},
		{"a bare slash is the root", "/", ""},
		{"already correct", "/prowlarr", "/prowlarr"},
		// The repair the whole change exists for. A bare segment has exactly one
		// possible meaning here and nothing competes with it, so guessing is not
		// involved — unlike every rejection below.
		{"a missing leading slash is repaired", "prowlarr", "/prowlarr"},
		{"a trailing slash is dropped", "/prowlarr/", "/prowlarr"},
		{"both at once", "prowlarr/", "/prowlarr"},
		{"surrounding whitespace", "  /prowlarr  ", "/prowlarr"},
		{"nested paths survive", "/apps/prowlarr", "/apps/prowlarr"},
		{"percent-encoding is passed through", "/my%20arr", "/my%20arr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeServiceURLBase(tc.in)
			if err != nil {
				t.Fatalf("normalizeServiceURLBase(%q) = error %v, want %q", tc.in, err, tc.want)
			}
			if got != tc.want {
				t.Fatalf("normalizeServiceURLBase(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Everything here is REJECTED rather than repaired, because there is more than
// one thing the user could have meant and inventing configuration on their
// behalf is worse than making them retype nine characters.
func TestServiceURLBaseRejections(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"a full URL belongs in base_url", "http://prowlarr:9696/prowlarr"},
		{"a scheme on its own", "https://prowlarr"},
		{"a protocol-relative host", "//prowlarr:9696/prowlarr"},
		{"a host and port", "prowlarr:9696"},
		{"a query string", "/prowlarr?apikey=abc"},
		{"a fragment", "/prowlarr#top"},
		{"traversal", "/prowlarr/../admin"},
		{"a bare dot segment", "/./prowlarr"},
		{"an empty interior segment", "/prowlarr//api"},
		{"a space", "/pro wlarr"},
		{"a backslash", `\prowlarr`},
		// Surrounding whitespace is trimmed (a paste artefact, unambiguous); one
		// in the middle of a segment is not.
		{"an embedded newline", "/prow\nlarr"},
		{"a control character", "/prowlarr\x00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeServiceURLBase(tc.in)
			if err == nil {
				t.Fatalf("normalizeServiceURLBase(%q) = %q, want a rejection", tc.in, got)
			}
			var ae *apiError
			if !errors.As(err, &ae) {
				t.Fatalf("error is %T, not the shape writeError renders", err)
			}
			// The code, not the prose, is what the screen branches on to put the
			// message next to the field the user got wrong.
			if ae.Status != http.StatusBadRequest || ae.Code != "invalid_url_base" {
				t.Fatalf("status=%d code=%q, want 400 invalid_url_base", ae.Status, ae.Code)
			}
			if ae.Action == "" {
				t.Error("a rejection must name the fix; §17.3 has no unactionable errors")
			}
		})
	}
}

// ── the endpoints ───────────────────────────────────────────────────────────

// asOwner runs one handler with an authenticated owner in the context, skipping
// the router so the test is about validation and not about cookies.
func asOwner(t *testing.T, s *Server, h handler, method, target, body string, id int64) (int, string) {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if id != 0 {
		r.SetPathValue("id", strconv.FormatInt(id, 10))
	}
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: true},
	}))
	w := httptest.NewRecorder()
	s.wrap(h)(w, r)
	return w.Code, w.Body.String()
}

// seedInstance puts one row in the store with no working credential. Nothing
// here needs one: the whole point is the path that runs NO connection test.
func seedInstance(t *testing.T, s *Server, urlBase string) int64 {
	t.Helper()
	id, err := s.store.CreateServiceInstance(context.Background(), store.ServiceInstance{
		Kind: "prowlarr", Role: "indexer", Name: "Prowlarr",
		BaseURL: "http://prowlarr:9696", URLBase: urlBase,
		APIKeyEnc: []byte{0}, Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("seed service instance: %v", err)
	}
	return id
}

// The gap this change closes. PATCH with no api_key runs no connection test, so
// before validation landed a sub-path that could never resolve was accepted with
// a 200 and reported only later, by the background prober, as an instance that
// is down for no visible reason.
func TestPatchWithNoAPIKeyRejectsASubPathThatIsNotAPath(t *testing.T) {
	s := newTestServer(t, nil)
	id := seedInstance(t, s, "/prowlarr")

	code, body := asOwner(t, s, s.handleUpdateService, http.MethodPatch,
		"/api/v1/services/1", `{"url_base":"http://elsewhere.invalid/prowlarr"}`, id)
	if code != http.StatusBadRequest {
		t.Fatalf("PATCH with a URL in url_base = %d, want 400: %s", code, body)
	}
	if !strings.Contains(body, `"error":"invalid_url_base"`) {
		t.Fatalf("the rejection must carry a code the UI can branch on: %s", body)
	}

	// And it must not have been written. A 400 that saved anyway would be the
	// same silent breakage wearing a different status code.
	after, err := s.store.GetServiceInstance(context.Background(), store.OwnerScope(1), id)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after.URLBase != "/prowlarr" {
		t.Fatalf("url_base = %q after a rejected PATCH, want the stored %q", after.URLBase, "/prowlarr")
	}
}

// The other half: the repair is applied on the same untested path, so the value
// that used to concatenate into http://host:9696prowlarr is stored usable.
func TestPatchWithNoAPIKeyRepairsAMissingLeadingSlash(t *testing.T) {
	s := newTestServer(t, nil)
	id := seedInstance(t, s, "")

	code, body := asOwner(t, s, s.handleUpdateService, http.MethodPatch,
		"/api/v1/services/1", `{"url_base":"prowlarr"}`, id)
	if code != http.StatusOK {
		t.Fatalf("PATCH url_base=prowlarr = %d, want 200: %s", code, body)
	}
	if !strings.Contains(body, `"url_base":"/prowlarr"`) {
		t.Fatalf("the response must echo the normalised sub-path: %s", body)
	}

	after, err := s.store.GetServiceInstance(context.Background(), store.OwnerScope(1), id)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after.URLBase != "/prowlarr" {
		t.Fatalf("stored url_base = %q, want %q — %q+%q is not a URL",
			after.URLBase, "/prowlarr", "http://prowlarr:9696", "prowlarr")
	}
}

// Every endpoint that ACCEPTS url_base validates it. Missing one is how a bad
// value gets in: the create path already had the connection test as a backstop,
// and it was the three without one that mattered.
func TestEveryEndpointTakingURLBaseValidatesIt(t *testing.T) {
	s := newTestServer(t, nil)
	id := seedInstance(t, s, "/prowlarr")
	const bad = `"url_base":"http://elsewhere.invalid/prowlarr"`

	cases := []struct {
		name   string
		h      handler
		method string
		body   string
		id     int64
	}{
		{
			name: "POST /api/v1/services", h: s.handleCreateService, method: http.MethodPost,
			body: `{"kind":"prowlarr","name":"Prowlarr","base_url":"http://prowlarr:9696",` +
				bad + `,"api_key":"k"}`,
		},
		{
			name: "PATCH /api/v1/services/{id}", h: s.handleUpdateService, method: http.MethodPatch,
			body: "{" + bad + "}", id: id,
		},
		{
			name: "POST /api/v1/services/test", h: s.handleTestUnsaved, method: http.MethodPost,
			body: `{"kind":"prowlarr","base_url":"http://prowlarr:9696",` + bad + `,"api_key":"k"}`,
		},
		{
			name: "POST /api/v1/services/{id}/test", h: s.handleTestService, method: http.MethodPost,
			body: "{" + bad + "}", id: id,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := asOwner(t, s, tc.h, tc.method, "/api/v1/services", tc.body, tc.id)
			if code != http.StatusBadRequest || !strings.Contains(body, `"error":"invalid_url_base"`) {
				t.Fatalf("%s = %d, want 400 invalid_url_base: %s", tc.name, code, body)
			}
		})
	}
}
