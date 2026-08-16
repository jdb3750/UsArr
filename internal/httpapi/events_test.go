package httpapi

import (
	"testing"
	"time"
)

func TestHubDeliversOnlyToTheOwningUser(t *testing.T) {
	h := NewHub(16, time.Now)
	defer h.Close()

	alice, _, _ := h.subscribe(1, 0)
	bob, _, _ := h.subscribe(2, 0)

	h.Publish(1, EventSearchResults, map[string]string{"for": "alice"})

	select {
	case ev := <-alice.ch:
		if ev.Name != EventSearchResults {
			t.Fatalf("alice got %q", ev.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("alice received nothing")
	}

	// An event scoped to one user must not reach another. v0.1 has one user;
	// this is the property that makes multi-user a behaviour change rather than
	// a redesign — and an SSE stream is exactly the place a scope leak would be
	// invisible until it mattered.
	select {
	case ev := <-bob.ch:
		t.Fatalf("bob received an event addressed to alice: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHubReplaysAfterLastEventID(t *testing.T) {
	h := NewHub(16, time.Now)
	defer h.Close()

	first := h.Publish(1, EventSearchStarted, map[string]int{"n": 1})
	h.Publish(1, EventSearchResults, map[string]int{"n": 2})
	h.Publish(1, EventSearchDone, map[string]int{"n": 3})

	_, replay, missed := h.subscribe(1, first)
	if missed {
		t.Error("nothing was dropped; missed must be false")
	}
	if len(replay) != 2 {
		t.Fatalf("replay = %d events, want the 2 after id %d", len(replay), first)
	}
	if replay[0].Name != EventSearchResults || replay[1].Name != EventSearchDone {
		t.Fatalf("replay out of order: %q, %q", replay[0].Name, replay[1].Name)
	}
}

// A reconnect whose cursor predates the buffer has a HOLE. Saying so beats
// silently handing over a partial view the client believes is complete.
func TestHubReportsAGapItCannotReplay(t *testing.T) {
	h := NewHub(2, time.Now)
	defer h.Close()

	for range 5 {
		h.Publish(1, EventSearchResults, map[string]int{})
	}
	_, _, missed := h.subscribe(1, 1)
	if !missed {
		t.Fatal("a cursor older than the replay buffer must report missed events")
	}
}

// A stalled browser must not stall a search. The bounded outcome is to drop the
// subscriber; its own reconnect brings it back with Last-Event-ID.
func TestHubDropsASlowConsumerRatherThanBlocking(t *testing.T) {
	h := NewHub(1024, time.Now)
	defer h.Close()

	sub, _, _ := h.subscribe(1, 0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range subscriberQueue + 10 {
			h.Publish(1, EventSearchResults, map[string]int{})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a subscriber that never read")
	}

	// Drain; the channel must be closed rather than left half-full forever.
	for range sub.ch { //nolint:revive // draining to observe the close
	}
}

func TestHubCloseEndsEveryStream(t *testing.T) {
	h := NewHub(8, time.Now)
	sub, _, _ := h.subscribe(1, 0)
	h.Close()

	select {
	case _, open := <-sub.ch:
		if open {
			t.Fatal("Close must close the subscriber channel")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not end the stream; shutdown would hang on it")
	}

	// Publishing after Close is a no-op, not a panic on a closed channel.
	if id := h.Publish(1, EventSearchResults, nil); id != 0 {
		t.Errorf("Publish after Close returned id %d", id)
	}
}
