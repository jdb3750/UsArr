package httpapi

import (
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// TestEventNamesAreTheWireContract pins the event names as literal strings.
//
// These names are a contract with the SPA, which registers an EventSource
// listener per name and drops anything it was not told about. The SPA pins the
// same list from its side in web/src/lib/api.test.ts ("the SSE event-name
// contract"). Renaming an event here without renaming it there — or the reverse
// — must fail a test, because the failure mode when it does not is a search
// screen that stays empty forever with nothing logged anywhere.
func TestEventNamesAreTheWireContract(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{EventSearchStarted, "search.started"},
		{EventSearchIndexer, "search.indexer"},
		{EventSearchResults, "search.results"},
		{EventSearchDone, "search.done"},
		{EventSearchFailed, "search.failed"},
		{EventStreamMissedEvents, "stream.missed"},
	} {
		if tc.got != tc.want {
			t.Errorf("event name = %q, want %q — web/src/lib/api.ts must be updated in the same commit", tc.got, tc.want)
		}
	}

	// import.progress is the SEVENTH name. It HAS a producer (cmd/usarr's
	// importProgress, proved end to end over a real SSE connection by
	// TestAddingAKavitaProducesACatalogue) and, since 74ea1e5, a listener:
	// web/src/lib/api.ts's STREAM_EVENT_NAMES carries it and
	// web/src/lib/importstream.svelte.ts folds it into the Services screen. Both
	// halves of the contract are live, so this name is pinned on the same footing
	// as the six above rather than as a known gap. LS-05 is closed.
	if EventImportProgress != "import.progress" {
		t.Errorf("event name = %q, want %q", EventImportProgress, "import.progress")
	}

	// ⚠️ THIS BLOCK IS A REMINDER, NOT A DETECTOR, AND SAYING SO IS THE POINT.
	// The map is a hand-written literal, so len(names) can only ever be the
	// number of keys someone typed — it CANNOT notice a constant added to
	// events.go, and it did not notice `import.progress` for as long as that one
	// went unlistened-for. The real contract is cross-language (this list against
	// web/src/lib/api.ts's STREAM_EVENT_NAMES) and is still pinned by two
	// independent hardcoded lists, which now AGREE but which nothing joins: a
	// name added to events.go and to neither list breaks nothing.
	//
	// What does have a joining test is the frame SHAPE, for the five names a
	// healthy run emits: cmd/usarr's TestSSEFramesMatchTheClientContract records
	// them off a live stream into web/src/lib/__fixtures__/sse-frames.json, and
	// TestRecordedFramesCarryNoFieldTheAPINeverEmits checks the other direction.
	// search.failed and stream.missed occur on no healthy run and so are outside
	// even that.
	names := map[string]bool{
		EventSearchStarted: true, EventSearchIndexer: true, EventSearchResults: true,
		EventSearchDone: true, EventSearchFailed: true,
		EventStreamMissedEvents: true, EventImportProgress: true,
	}
	if len(names) != 7 {
		t.Fatalf("the event-name set has %d distinct names, want 7 — two names collided", len(names))
	}
}

// TestHubDeliversTheSystemSentinelToEveryStream is the property that makes
// §7.2's "progress over SSE" reach anybody at all.
//
// The catalogue import has NO ACTING USER, so it publishes under
// store.SystemUserID — the same id the libraries it writes are owned by, which
// store.Scope resolves as `user_id IN (SystemUserID, UserID)`. Before this, the
// hub matched the owner exactly, so the sentinel meant "everybody" in the
// database and "nobody" on the stream, and every import.progress frame was
// published into a void.
func TestHubDeliversTheSystemSentinelToEveryStream(t *testing.T) {
	h := NewHub(16, time.Now)
	defer h.Close()

	alice, _, _ := h.subscribe(1, 0)
	bob, _, _ := h.subscribe(2, 0)

	// One frame first, so the replay cursor below is non-zero: subscribe treats
	// lastID 0 as "no cursor" and replays nothing at all.
	cursor := h.Publish(1, EventSearchStarted, map[string]string{"search_id": "s1"})
	h.Publish(store.SystemUserID, EventImportProgress, map[string]string{"phase": "done"})
	<-alice.ch // the search frame alice owns, so the assertion below reads the shared one

	for name, sub := range map[string]*subscriber{"alice": alice, "bob": bob} {
		select {
		case ev := <-sub.ch:
			if ev.Name != EventImportProgress {
				t.Errorf("%s got %q, want the shared frame", name, ev.Name)
			}
		case <-time.After(time.Second):
			t.Errorf("%s received no shared frame: a system-owned event reached nobody, "+
				"which is what an import progress bar that never moves looks like", name)
		}
	}

	// A REPLAY MUST USE THE SAME PREDICATE. A reconnect that filtered differently
	// would lose exactly the shared frames the live stream had been delivering.
	_, replay, _ := h.subscribe(3, cursor)
	if len(replay) != 1 || replay[0].Name != EventImportProgress {
		t.Errorf("replay = %+v, want the shared frame", replay)
	}

	// And it stays ONE-WAY: a frame owned by a real user still reaches that user
	// alone. Getting this backwards turns one screen's fix into a scope leak.
	h.Publish(1, EventSearchResults, map[string]string{"for": "alice"})
	select {
	case ev := <-bob.ch:
		t.Fatalf("bob received an event addressed to alice: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

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
