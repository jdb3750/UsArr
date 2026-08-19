package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jdb3750/UsArr/internal/crypto"
)

// TestASaturatedKDFShedsUnknownAndKnownUsersIdentically is the drill for the
// requirement that makes or breaks the concurrency bound.
//
// THE FAILURE IT EXISTS TO CATCH. Put the semaphore only around the real
// verify and the unknown-account branch skips it — and that branch is the
// CHEAPEST request an attacker can send, needing no valid username, which makes
// the one path that dodges the cap the one an attacker will use. Worse, it
// reintroduces a timing and status asymmetry between the branches under
// saturation: 401 fast for an account that does not exist, 503 for one that
// does. That is a user-enumeration oracle, and the dummy Argon2id hash on the
// unknown branch exists precisely to close it. A memory bound bought with an
// enumeration oracle is a bad trade.
//
// SO THE TEST DRIVES UNKNOWN USERNAMES SPECIFICALLY, and asserts the known one
// answers identically. It fills the gate through crypto.AcquireKDF rather than
// racing a stopwatch against real hashes, so there is nothing machine-dependent
// in it beyond the bounded wait itself.
func TestASaturatedKDFShedsUnknownAndKnownUsersIdentically(t *testing.T) {
	s := newTestServer(t, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	c := &cookieClient{t: t, jar: map[string]string{}}
	c.get(srv.URL + "/api/v1/auth/session")
	if code, body := c.postJSON(srv.URL+"/api/v1/auth/setup",
		`{"username":"joe","password":"correct horse battery"}`); code != http.StatusOK {
		t.Fatalf("setup = %d: %s", code, body)
	}

	// Fill the gate. Every permit is held for the duration of the two requests
	// below, so both must wait and both must be shed.
	releases := make([]func(), 0, crypto.KDFPermits())
	for i := 0; i < crypto.KDFPermits(); i++ {
		release, err := crypto.AcquireKDF(t.Context())
		if err != nil {
			t.Fatalf("hold permit %d: %v", i, err)
		}
		releases = append(releases, release)
	}

	// Concurrently, so the whole test costs one bounded wait rather than two.
	type result struct {
		code int
		body string
	}
	var wg sync.WaitGroup
	got := make([]result, 2)
	for i, creds := range []string{
		`{"username":"nobody-has-this-name","password":"correct horse battery"}`,
		`{"username":"joe","password":"the wrong password entirely"}`,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &cookieClient{t: t, jar: map[string]string{csrfCookie: c.jar[csrfCookie]}}
			code, body := client.postJSON(srv.URL+"/api/v1/auth/login", creds)
			got[i] = result{code, body}
		}()
	}
	wg.Wait()

	unknown, known := got[0], got[1]
	if unknown.code != http.StatusServiceUnavailable || !strings.Contains(unknown.body, `"error":"busy"`) {
		t.Fatalf("an UNKNOWN username with the KDF saturated = %d %s, want 503 busy. "+
			"The unknown-account branch must take a permit like every other: it is the "+
			"cheapest request an attacker can send, and a branch that skips the semaphore "+
			"is the branch the flood will use",
			unknown.code, unknown.body)
	}
	if known.code != unknown.code {
		t.Fatalf("a KNOWN username answered %d and an unknown one %d. Under saturation the two "+
			"branches must be indistinguishable, or the memory bound has been bought with a "+
			"user-enumeration oracle", known.code, unknown.code)
	}
	if known.body != unknown.body {
		t.Errorf("the shed bodies differ:\n known:   %s\n unknown: %s", known.body, unknown.body)
	}

	// Release, and the same two requests go back to being ordinary failed
	// logins — identical to each other, as they were before any of this.
	for _, release := range releases {
		release()
	}
	unknownCode, unknownBody := c.postJSON(srv.URL+"/api/v1/auth/login",
		`{"username":"nobody-has-this-name","password":"correct horse battery"}`)
	knownCode, knownBody := c.postJSON(srv.URL+"/api/v1/auth/login",
		`{"username":"joe","password":"the wrong password entirely"}`)
	if unknownCode != http.StatusUnauthorized || knownCode != http.StatusUnauthorized {
		t.Fatalf("after the permits came back: unknown = %d, known = %d, want 401 for both. "+
			"A gate that never reopens would pass every assertion above",
			unknownCode, knownCode)
	}
	if unknownBody != knownBody {
		t.Errorf("unknown-account and bad-password bodies differ:\n unknown: %s\n known:   %s",
			unknownBody, knownBody)
	}

	// And a correct password still works, so nothing above broke login itself.
	if code, body := c.postJSON(srv.URL+"/api/v1/auth/login",
		`{"username":"joe","password":"correct horse battery"}`); code != http.StatusOK {
		t.Fatalf("a correct login after the gate reopened = %d: %s", code, body)
	}
}
