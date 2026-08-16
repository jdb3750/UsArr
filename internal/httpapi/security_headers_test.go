package httpapi

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// The CSP and the headers beside it were each paid for by a specific incident,
// and until this test existed NOTHING anywhere asserted any of them — no Go
// test, no browser test. The whole policy could be weakened or deleted and
// every gate stayed green.
//
// GRANULARITY, and why. This pins DIRECTIVES, not the header string. An
// exact-string comparison fails on reordering and on whitespace, so it fails on
// every legitimate edit, which trains the next person to re-paste the literal
// without reading it — the failure mode where a guard is present and useless.
// A directive-level assertion survives reordering but can miss an addition, so
// it is paired with a blanket rule that no directive anywhere permits the
// additions that matter. Concretely, per policy:
//
//   - every directive the policy is supposed to carry must be PRESENT, so
//     deleting `base-uri 'none'` or `frame-ancestors 'none'` fails;
//   - the source list is EXACT for the directives where every source is
//     load-bearing and any addition is a weakening. That includes `script-src
//     'self' 'report-sample'`: the SPA could not boot under `script-src 'self'`
//     until a specific fix, so re-adding `'unsafe-inline'` to make a future
//     change "just work" is the regression this exists to notice, and dropping
//     `'report-sample'` (added in b3f6aaf) makes the announcer's violation
//     record unidentifiable again;
//   - no directive anywhere carries `unsafe-inline`, `unsafe-eval`,
//     `unsafe-hashes` or a `*` wildcard. This is the blanket rule and it is what
//     catches the additions the exact lists cannot cover — above all
//     `style-src-attr 'unsafe-inline'`, which middleware.go refuses ON PURPOSE
//     (the announcer violation is tolerated instead) and which arrives as a new
//     directive that no exact list would mention.
func TestSecurityHeadersArePinnedOnEveryResponse(t *testing.T) {
	s := newTestServer(t, func(c *Config) {
		// A real document on the SPA route, so the SPA case is a 200 rendering
		// bytes and not the "frontend is not embedded" 404.
		c.SPA = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!doctype html><title>UsArr</title>"))
		})
	})
	s.MarkListening()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	routes := []struct {
		name string
		path string
		want int
	}{
		{"spa document", "/", http.StatusOK},
		{"api json", "/api/health/live", http.StatusOK},
		// buildHandler puts securityHeaders above the router precisely so an
		// error response is not the hole. Assert that, rather than trusting it.
		{"api error", "/api/v1/services", http.StatusUnauthorized},
	}

	policies := make(map[string]string, len(routes))
	for _, rt := range routes {
		t.Run(rt.name, func(t *testing.T) {
			code, hdr := headersOf(t, srv.URL+rt.path)
			if code != rt.want {
				t.Fatalf("GET %s = %d, want %d", rt.path, code, rt.want)
			}

			for header, want := range map[string]string{
				"X-Content-Type-Options":     "nosniff",
				"Referrer-Policy":            "no-referrer",
				"Cross-Origin-Opener-Policy": "same-origin",
			} {
				if got := hdr.Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}

			policy := hdr.Get("Content-Security-Policy")
			if policy == "" {
				t.Fatalf("GET %s carries no Content-Security-Policy at all", rt.path)
			}
			policies[rt.path] = policy
			assertCSP(t, policy)
		})
	}

	// One middleware sets it for everything, so every route must see the same
	// bytes. A route that acquired its own copy would drift away from this test
	// one directive at a time.
	for path, policy := range policies {
		if policy != policies["/"] {
			t.Errorf("the policy on %s differs from the SPA's:\n %s\n %s", path, policy, policies["/"])
		}
	}
}

// assertCSP checks one policy string. See the granularity note on
// TestSecurityHeadersArePinnedOnEveryResponse for what is pinned and what is not.
func assertCSP(t *testing.T, policy string) {
	t.Helper()
	got := parseCSP(policy)

	exact := map[string][]string{
		"default-src":     {"'self'"},
		"script-src":      {"'self'", "'report-sample'"},
		"style-src":       {"'self'", "'report-sample'"},
		"connect-src":     {"'self'"},
		"frame-ancestors": {"'none'"},
		"base-uri":        {"'none'"},
		"form-action":     {"'self'"},
	}
	// Sorted, so a multi-directive failure is reported in a stable order and two
	// runs diff cleanly.
	for _, name := range slices.Sorted(maps.Keys(exact)) {
		want := exact[name]
		sources, ok := got[name]
		if !ok {
			t.Errorf("the policy has no %s directive; policy = %q", name, policy)
			continue
		}
		if !sameSet(sources, want) {
			t.Errorf("%s = %v, want exactly %v", name, sources, want)
		}
	}

	// img-src is deliberately NOT pinned exactly: `data:` is intentional and a
	// further image source is a plausible legitimate edit. It must still exist
	// and still be rooted at the origin, and the blanket rule below still
	// applies to it, so a wildcard here fails like anywhere else.
	if sources, ok := got["img-src"]; !ok {
		t.Errorf("the policy has no img-src directive; policy = %q", policy)
	} else if !slices.Contains(sources, "'self'") {
		t.Errorf("img-src = %v, want it to include 'self'", sources)
	}

	for _, name := range slices.Sorted(maps.Keys(got)) {
		for _, src := range got[name] {
			switch strings.ToLower(src) {
			case "'unsafe-inline'", "'unsafe-eval'", "'unsafe-hashes'", "*":
				t.Errorf("%s carries %s; no directive in this policy may permit it", name, src)
			}
		}
	}
}

// parseCSP splits a policy into directive name -> source list. Names are
// ASCII-case-insensitive (CSP3 §2.2) so they are folded; source expressions are
// compared as written, since a host source's path is case-sensitive.
func parseCSP(policy string) map[string][]string {
	out := make(map[string][]string)
	for _, directive := range strings.Split(policy, ";") {
		fields := strings.Fields(directive)
		if len(fields) == 0 {
			continue
		}
		out[strings.ToLower(fields[0])] = fields[1:]
	}
	return out
}

// sameSet compares source lists ignoring order, which is what makes this
// reordering-tolerant rather than a string comparison in disguise.
func sameSet(got, want []string) bool {
	g, w := slices.Clone(got), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	return slices.Equal(g, w)
}

func headersOf(t *testing.T, url string) (int, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, resp.Header.Clone()
}
