package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSudoRegeneratesTheSessionID is the drill for reference/security.md §6's
// "the session id is regenerated on every privilege change".
//
// Login satisfied that by minting a whole new row. Opening the sudo window is
// the OTHER privilege change UsArr has — the moment a session becomes able to
// add a service, edit a base URL and re-enter an \*Arr API key — and it used to
// be an in-place UPDATE that left the id alone, so any pre-sudo cookie value was
// carried straight across the boundary.
//
// It is driven through the real middleware chain with a real browser-shaped
// client, because the half that breaks in practice is the cookie: regenerating
// the id server-side without re-issuing the cookie in the SAME response is not a
// hardening, it is a logout. Both halves are asserted here.
func TestSudoRegeneratesTheSessionID(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s := newTestServer(t, func(c *Config) { c.Now = func() time.Time { return now } })
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	c := &cookieClient{t: t, jar: map[string]string{}}
	c.get(srv.URL + "/api/v1/auth/session")
	if code, body := c.postJSON(srv.URL+"/api/v1/auth/setup",
		`{"username":"joe","password":"correct horse battery"}`); code != http.StatusOK {
		t.Fatalf("setup = %d: %s", code, body)
	}

	before := c.jar[sessionCookie]
	if before == "" {
		t.Fatal("no session cookie after setup")
	}

	// Past the sudo window, so this is the real re-authentication and not a
	// no-op refresh of a window that was still open.
	now = now.Add(6 * time.Minute)

	code, body := c.postJSON(srv.URL+"/api/v1/auth/sudo", `{"password":"correct horse battery"}`)
	if code != http.StatusOK {
		t.Fatalf("sudo = %d: %s", code, body)
	}

	after := c.jar[sessionCookie]
	if after == before {
		t.Fatal("the session cookie is unchanged across the sudo grant. §6 requires the " +
			"session id to be regenerated at every privilege change, and opening the sudo " +
			"window is one: a fixated pre-sudo cookie is otherwise carried into a session " +
			"that can now edit service credentials")
	}
	if after == "" {
		t.Fatal("the sudo response cleared the session cookie instead of re-issuing it")
	}

	// The old value must be DEAD, not merely superseded. If the row were copied
	// rather than renamed, both cookies would work and regeneration would have
	// bought nothing.
	old := &cookieClient{t: t, jar: map[string]string{
		sessionCookie: before,
		csrfCookie:    c.jar[csrfCookie],
	}}
	if _, oldBody := old.get(srv.URL + "/api/v1/auth/session"); strings.Contains(oldBody, `"authenticated":true`) {
		t.Errorf("the PRE-sudo cookie still authenticates: %s", oldBody)
	}

	// And the caller is not logged out — the half a server-side-only fix breaks.
	// The window is open under the NEW id, which is the whole point.
	_, body = c.get(srv.URL + "/api/v1/auth/session")
	var resp struct {
		Authenticated bool       `json:"authenticated"`
		SudoUntil     *time.Time `json:"sudo_until"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("session body: %v: %s", err, body)
	}
	if !resp.Authenticated {
		t.Fatalf("the post-sudo cookie does not authenticate — regeneration logged the user out: %s", body)
	}
	if resp.SudoUntil == nil || !resp.SudoUntil.Equal(now.Add(5*time.Minute)) {
		t.Errorf("sudo_until = %v under the new session id, want the freshly opened window: %s",
			resp.SudoUntil, body)
	}

	// The absolute expiry is NOT extended by re-authenticating: this is the same
	// session under a new name. Renaming the row rather than inserting a fresh
	// one is what keeps that true, so it is asserted rather than assumed.
	sess, err := s.store.GetSession(t.Context(), sessionID(after), now)
	if err != nil {
		t.Fatalf("read the regenerated session: %v", err)
	}
	if !sess.CreatedAt.Equal(now.Add(-6 * time.Minute)) {
		t.Errorf("created_at = %v, want the ORIGINAL login time — sudo must not mint a new "+
			"session lifetime, or re-authenticating becomes a way to live past the absolute "+
			"timeout for ever", sess.CreatedAt)
	}
}
