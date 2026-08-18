package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// seedIndexer puts one indexer-role instance in the store. No credential is
// needed: resolveIndexerInstance runs entirely before anything is opened.
func seedIndexer(t *testing.T, s *Server, name string, enabled bool, priority int64) int64 {
	t.Helper()
	id, err := s.store.CreateServiceInstance(context.Background(), store.ServiceInstance{
		Kind: "prowlarr", Role: "indexer", Name: name,
		BaseURL: "http://prowlarr:9696", APIKeyEnc: []byte{0},
		Enabled: enabled, Priority: priority, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return id
}

func resolveWithInstance(t *testing.T, s *Server, query string) (int64, error) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/releases/search?query=dune"+query, nil)
	return s.resolveIndexerInstance(r, authSession{
		User: store.User{ID: 1, Username: "joe", IsOwner: true},
	})
}

// Naming a disabled instance is a mistake, and the only honest answer is to say
// so. The two wrong answers are worse in different ways: searching it anyway
// ignores the user's own configuration, and falling back to the auto-picked
// instance answers a DIFFERENT question from the one asked while returning
// results that look entirely correct.
func TestSearchRefusesAnExplicitlyNamedDisabledInstance(t *testing.T) {
	s := newTestServer(t, nil)
	off := seedIndexer(t, s, "Retired Prowlarr", false, 10)
	on := seedIndexer(t, s, "Prowlarr", true, 20)

	got, err := resolveWithInstance(t, s, fmt.Sprintf("&instance=%d", off))
	if err == nil {
		t.Fatalf("resolved instance %d, but %d is disabled and must be refused", got, off)
	}
	// Not the enabled one either: a silent fallback is the failure mode this
	// test exists for, and it would otherwise look like a pass.
	if got == on {
		t.Fatal("a disabled instance fell back to the enabled one; the request named an instance")
	}

	var ae *apiError
	if !errors.As(err, &ae) {
		t.Fatalf("error is %T, not the shape writeError renders", err)
	}
	if ae.Status != http.StatusConflict || ae.Code != CodeServiceDisabled {
		t.Fatalf("status=%d code=%q, want 409 %s", ae.Status, ae.Code, CodeServiceDisabled)
	}
	if ae.Action == "" {
		t.Error("a refusal must name the fix; §17.3 has no unactionable errors")
	}
	if !KnownErrorCode(ae.Code) {
		t.Errorf("%q is not in the enumerable set, so no frontend fixture can name it", ae.Code)
	}
}

// The enabled instance still resolves, and the auto-pick still ignores the
// disabled one — the two behaviours the refusal above must not have broken.
func TestSearchResolvesEnabledInstancesNamedAndAutomatically(t *testing.T) {
	s := newTestServer(t, nil)
	_ = seedIndexer(t, s, "Retired Prowlarr", false, 10)
	on := seedIndexer(t, s, "Prowlarr", true, 20)

	got, err := resolveWithInstance(t, s, fmt.Sprintf("&instance=%d", on))
	if err != nil {
		t.Fatalf("naming the enabled instance: %v", err)
	}
	if got != on {
		t.Fatalf("resolved %d, want %d", got, on)
	}

	got, err = resolveWithInstance(t, s, "")
	if err != nil {
		t.Fatalf("auto-pick: %v", err)
	}
	if got != on {
		t.Fatalf("auto-pick resolved %d, want the only enabled indexer %d", got, on)
	}
}
