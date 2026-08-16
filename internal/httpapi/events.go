package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// There is ONE SSE stream (ARCHITECTURE.md §4.1): plain HTTP, so it survives
// every reverse proxy, auto-reconnects in the browser with no library, and
// carries an id per frame so Last-Event-ID resumes it. WebSockets buy nothing
// here — UsArr's push traffic is one-directional.

const (
	// defaultEventBuffer is how many recent events the hub keeps for replay.
	// A reconnect gap is measured in seconds and a search emits a handful of
	// events per indexer, so this covers the reconnect case comfortably while
	// staying a fixed, small amount of memory.
	defaultEventBuffer = 512

	// subscriberQueue is per-connection. A client that cannot keep up with this
	// much backlog is not going to catch up; it is dropped and reconnects with
	// Last-Event-ID, which is a bounded, self-healing outcome. Blocking the
	// publisher instead would let one stalled browser stall a search.
	subscriberQueue = 64

	// heartbeat keeps intermediaries from closing an idle stream. It is a
	// comment frame, so it costs a client nothing to ignore.
	heartbeat = 25 * time.Second
)

// Event names. They are namespaced so a client can subscribe by prefix and so a
// new producer cannot collide with search by accident.
//
// EVERY NAME HERE HAS A PRODUCER. `service.health` used to sit in this block
// with no Publish call anywhere: a name the SPA would have had to register a
// listener for, that could never fire. CLAUDE.md's rule is that the seam ships
// and the feature does not — the seam for pushed health is Hub.Publish itself,
// which needs no reserved name. When the Services screen learns to push, the
// constant comes back in the same commit as the call that publishes it.
const (
	EventSearchStarted      = "search.started"
	EventSearchIndexer      = "search.indexer"
	EventSearchResults      = "search.results"
	EventSearchDone         = "search.done"
	EventSearchFailed       = "search.failed"
	EventStreamMissedEvents = "stream.missed"
)

// Event is one frame on the stream.
type Event struct {
	ID   uint64
	Name string
	Data any

	// UserID scopes delivery. An event is delivered only to subscribers
	// belonging to that user; 0 is unused, not "everyone", because "everyone"
	// on a stream that will be multi-user is exactly the leak the access-scope
	// rule exists to prevent.
	UserID int64
}

type subscriber struct {
	userID int64
	ch     chan Event
	closed bool
}

// Hub fans events out to the connected SSE streams and keeps a small replay
// buffer so a reconnect does not lose the frames sent while the socket was down.
type Hub struct {
	now func() time.Time

	mu     sync.Mutex
	next   uint64
	subs   map[*subscriber]struct{}
	ring   []Event
	ringN  int
	closed bool
}

// NewHub builds the hub. size is the replay buffer length.
func NewHub(size int, now func() time.Time) *Hub {
	if size <= 0 {
		size = defaultEventBuffer
	}
	if now == nil {
		now = time.Now
	}
	return &Hub{now: now, subs: map[*subscriber]struct{}{}, ring: make([]Event, 0, size), ringN: size}
}

// Publish delivers an event to userID's subscribers and records it for replay.
// It never blocks: a subscriber whose queue is full is dropped.
func (h *Hub) Publish(userID int64, name string, data any) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return 0
	}

	h.next++
	ev := Event{ID: h.next, Name: name, Data: data, UserID: userID}

	if len(h.ring) == h.ringN {
		copy(h.ring, h.ring[1:])
		h.ring[len(h.ring)-1] = ev
	} else {
		h.ring = append(h.ring, ev)
	}

	for sub := range h.subs {
		if sub.userID != userID || sub.closed {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			// Slow consumer. Dropping is the bounded outcome; the browser's
			// own reconnect brings it back with Last-Event-ID and the replay
			// buffer fills the gap.
			h.dropLocked(sub)
		}
	}
	return ev.ID
}

func (h *Hub) dropLocked(sub *subscriber) {
	if sub.closed {
		return
	}
	sub.closed = true
	close(sub.ch)
	delete(h.subs, sub)
}

// subscribe registers a stream and returns everything after lastID that is still
// in the replay buffer.
//
// missed reports that lastID was older than the buffer, so the client knows its
// view has a hole rather than silently believing it is complete.
func (h *Hub) subscribe(userID int64, lastID uint64) (sub *subscriber, replay []Event, missed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sub = &subscriber{userID: userID, ch: make(chan Event, subscriberQueue)}
	if h.closed {
		sub.closed = true
		close(sub.ch)
		return sub, nil, false
	}
	h.subs[sub] = struct{}{}

	if lastID > 0 {
		if len(h.ring) > 0 && h.ring[0].ID > lastID+1 {
			missed = true
		}
		for _, ev := range h.ring {
			if ev.ID > lastID && ev.UserID == userID {
				replay = append(replay, ev)
			}
		}
	}
	return sub, replay, missed
}

func (h *Hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[sub]; ok {
		h.dropLocked(sub)
	}
}

// Close ends every stream. An SSE connection never ends by itself, so shutdown
// has to reach in and end them or the drain deadline is what closes them.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for sub := range h.subs {
		h.dropLocked(sub)
	}
}

// handleEvents serves the one stream.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"this server cannot stream: the response writer does not flush")
	}

	lastID := lastEventID(r)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	// no-transform matters as much as no-cache: a compressing proxy will buffer
	// an event stream into uselessness otherwise.
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default, which turns SSE into a
	// request that appears to hang. This is the documented opt-out.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sub, replay, missed := s.hub.subscribe(a.User.ID, lastID)
	defer s.hub.unsubscribe(sub)

	// A retry hint plus an immediate comment, so a proxy that waits for bytes
	// before forwarding headers releases them now rather than in 25 seconds.
	if _, err := fmt.Fprintf(w, "retry: 3000\n: connected\n\n"); err != nil {
		return nil
	}
	flusher.Flush()

	if missed {
		s.writeEvent(w, flusher, Event{
			ID:   lastID,
			Name: EventStreamMissedEvents,
			Data: map[string]any{
				"message": "some events were dropped while this stream was disconnected",
				"action":  "Reload to resynchronise",
			},
		})
	}
	for _, ev := range replay {
		if !s.writeEvent(w, flusher, ev) {
			return nil
		}
	}

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, open := <-sub.ch:
			if !open {
				return nil
			}
			if !s.writeEvent(w, flusher, ev) {
				return nil
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}

// writeEvent renders one frame and reports whether the connection survived.
func (s *Server) writeEvent(w http.ResponseWriter, flusher http.Flusher, ev Event) bool {
	payload, err := json.Marshal(ev.Data)
	if err != nil {
		s.log.Error("SSE payload not encodable", "event", ev.Name, "err", redactText(err.Error()))
		return true
	}

	var b strings.Builder
	if ev.ID > 0 {
		b.WriteString("id: ")
		b.WriteString(strconv.FormatUint(ev.ID, 10))
		b.WriteByte('\n')
	}
	b.WriteString("event: ")
	b.WriteString(ev.Name)
	b.WriteByte('\n')
	// One data: line per line of payload. json.Marshal never emits a newline,
	// but the framing is written correctly rather than relying on that.
	for _, line := range strings.Split(string(payload), "\n") {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	if _, err := w.Write([]byte(b.String())); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// lastEventID reads the reconnect cursor. The header is what the browser's own
// EventSource sends; the query parameter exists for clients that cannot set a
// header, which includes anything driving the stream from a plain <img>-style
// context or a constrained test harness.
func lastEventID(r *http.Request) uint64 {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("last_event_id")
	}
	id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
