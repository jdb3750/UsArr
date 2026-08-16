package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/httpapi"
)

// The SSE stream is the ONLY way a search result reaches the browser, and both
// sides of it are hand-written: internal/httpapi picks the event names and the
// json tags, web/src/lib/api.ts decides which names to listen for and which
// fields to read. Nothing joins them at compile time, so nothing caught the two
// drifting apart — the client listened for `search.result` and `search.status`,
// which the server has never emitted, and the search screen stayed empty with no
// error anywhere.
//
// This test is the join. It drives a real search through the real API against
// the Prowlarr double, then checks the frames that came off the wire against
// web/src/lib/__fixtures__/sse-frames.json — the recording the SPA's own unit
// tests replay through normalizeStreamEvent. A rename or a retagged field on
// this side breaks this test; the same rename on the client side breaks
// api.test.ts. Neither can move alone.
//
// Regenerate the recording with:
//
//	USARR_UPDATE_SSE_FIXTURE=1 go test ./cmd/usarr -run TestSSEFramesMatchTheClientContract
//	make fmt   # the file lives under web/, so prettier owns its formatting

const sseFixturePath = "../../web/src/lib/__fixtures__/sse-frames.json"

// volatileFields carry a value that changes every run, so this test checks only
// that the field is still on the wire, spelled the same — not what is in it.
var volatileFields = map[string]bool{
	"search_id":     true,
	"expires_at":    true,
	"published_at":  true,
	"started_at":    true,
	"finished_at":   true,
	"blocked_until": true,
	"duration_ms":   true,
	// age_days is derived from wall-clock now, and reason embeds a wall-clock
	// deadline ("blocked until 2026-…"). Both keep their real recorded values in
	// the fixture, because the SPA's own test reads them, but neither can be
	// compared against a later run.
	"age_days": true,
	"reason":   true,
}

// timestampFields are rewritten to a fixed value when the recording is
// regenerated, so the checked-in file does not churn.
var timestampFields = map[string]bool{
	"expires_at":    true,
	"published_at":  true,
	"started_at":    true,
	"finished_at":   true,
	"blocked_until": true,
}

const (
	placeholderSearchID = "SEARCH_ID"
	placeholderTime     = "2026-08-16T00:00:00Z"
)

type recordedFrame struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

func TestSSEFramesMatchTheClientContract(t *testing.T) {
	const apiKey = "prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3"

	prowlarr := newFakeProwlarr(t, apiKey)
	env := newTestApp(t)

	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	var svc serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prowlarr.URL(), "api_key": apiKey,
	}, &svc)

	// One blocked indexer, so the run produces the degraded report the client has
	// to be able to render — an all-green run would never exercise it.
	prowlarr.blockIndexer(2)

	stream := env.openStream(t)
	defer stream.close()

	var accepted searchAcceptedBody
	env.do(t, "GET", "/api/v1/search?query=Test+Release&type=search&category=2000", nil, &accepted)
	if accepted.SearchID == "" {
		t.Fatal("the search must return a search id immediately")
	}

	live := collectSearchFrames(t, stream, accepted.SearchID)

	// ── the names, exactly ──────────────────────────────────────────────────
	//
	// search.failed and stream.missed are real names (events.go) but neither
	// occurs on a healthy run, so they are covered by the client's unit test
	// rather than recorded here.
	wantNames := []string{
		httpapi.EventSearchDone,
		httpapi.EventSearchIndexer,
		httpapi.EventSearchResults,
		httpapi.EventSearchStarted,
	}
	if got := distinctNames(live); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("event names on the wire = %v, want %v\n"+
			"web/src/lib/api.ts registers one EventSource listener per name; a name it "+
			"does not know about is dropped in silence", got, wantNames)
	}

	if os.Getenv("USARR_UPDATE_SSE_FIXTURE") == "1" {
		writeSSEFixture(t, live)
		return
	}

	// ── the recording the SPA replays ───────────────────────────────────────
	recorded := readSSEFixture(t)
	if got, want := distinctNames(recorded), distinctNames(live); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s records names %v, but this run emitted %v\n"+
			"regenerate with USARR_UPDATE_SSE_FIXTURE=1", sseFixturePath, got, want)
	}

	for i, frame := range recorded {
		var want any
		mustUnmarshal(t, string(frame.Data), &want)

		matched := false
		var reasons []string
		for _, candidate := range live {
			if candidate.Name != frame.Name {
				continue
			}
			var got any
			mustUnmarshal(t, string(candidate.Data), &got)
			if problems := covers(fmt.Sprintf("%s[%d]", frame.Name, i), want, got); len(problems) == 0 {
				matched = true
				break
			} else {
				reasons = append(reasons, strings.Join(problems, "; "))
			}
		}
		if !matched {
			t.Errorf("no %s frame on the wire matches the recorded one in %s:\n  %s\n"+
				"the SPA reads exactly these fields; regenerate with USARR_UPDATE_SSE_FIXTURE=1 "+
				"and update web/src/lib/api.ts to match",
				frame.Name, sseFixturePath, strings.Join(reasons, "\n  "))
		}
	}

	// The fields the client cannot render a row without, called out by name so a
	// failure says which one went missing rather than "the frame did not match".
	for _, field := range []string{"candidate_id", "title", "protocol", "size_bytes", "indexer_name"} {
		if !anyResultHasField(t, live, field) {
			t.Errorf("no release on the stream carries %q; web/src/lib/api.ts reads it", field)
		}
	}
}

// collectSearchFrames reads the stream until search.done, ignoring frames from
// any other search.
func collectSearchFrames(t *testing.T, stream *sseStream, searchID string) []recordedFrame {
	t.Helper()
	var frames []recordedFrame
	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s; got %d frames", httpapi.EventSearchDone, len(frames))
		case ev := <-stream.events:
			var envelope struct {
				SearchID string `json:"search_id"`
			}
			mustUnmarshal(t, ev.data, &envelope)
			if envelope.SearchID != searchID {
				continue
			}
			frames = append(frames, recordedFrame{Name: ev.name, Data: json.RawMessage(ev.data)})
			if ev.name == httpapi.EventSearchDone {
				return frames
			}
		}
	}
}

func distinctNames(frames []recordedFrame) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range frames {
		if !seen[f.Name] {
			seen[f.Name] = true
			out = append(out, f.Name)
		}
	}
	sort.Strings(out)
	return out
}

// covers reports every way in which the recorded value `want` is NOT present in
// the live value `got`. Live payloads may carry MORE than the recording does —
// adding a field is not a breaking change — but every recorded field must still
// be there, spelled the same, with the same value unless it is volatile.
func covers(path string, want, got any) []string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: recorded an object, wire has %T", path, got)}
		}
		var problems []string
		for key, wv := range w {
			gv, present := g[key]
			if !present {
				problems = append(problems, fmt.Sprintf("%s.%s: field is gone from the wire", path, key))
				continue
			}
			if volatileFields[key] {
				continue // presence only: the value changes every run
			}
			problems = append(problems, covers(path+"."+key, wv, gv)...)
		}
		return problems
	case []any:
		g, ok := got.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: recorded an array, wire has %T", path, got)}
		}
		if len(g) < len(w) {
			return []string{fmt.Sprintf("%s: recorded %d entries, wire has %d", path, len(w), len(g))}
		}
		var problems []string
		for i := range w {
			problems = append(problems, covers(fmt.Sprintf("%s[%d]", path, i), w[i], g[i])...)
		}
		return problems
	default:
		if !reflect.DeepEqual(want, got) {
			return []string{fmt.Sprintf("%s: recorded %v, wire has %v", path, want, got)}
		}
		return nil
	}
}

func anyResultHasField(t *testing.T, frames []recordedFrame, field string) bool {
	t.Helper()
	for _, f := range frames {
		if f.Name != httpapi.EventSearchResults {
			continue
		}
		var payload struct {
			Results []map[string]any `json:"results"`
		}
		mustUnmarshal(t, string(f.Data), &payload)
		for _, res := range payload.Results {
			if _, ok := res[field]; ok {
				return true
			}
		}
	}
	return false
}

func readSSEFixture(t *testing.T) []recordedFrame {
	t.Helper()
	blob, err := os.ReadFile(sseFixturePath)
	if err != nil {
		t.Fatalf("read %s: %v\ngenerate it with USARR_UPDATE_SSE_FIXTURE=1", sseFixturePath, err)
	}
	var frames []recordedFrame
	if err := json.Unmarshal(blob, &frames); err != nil {
		t.Fatalf("decode %s: %v", sseFixturePath, err)
	}
	if len(frames) == 0 {
		t.Fatalf("%s records no frames", sseFixturePath)
	}
	return frames
}

// writeSSEFixture records the run, with the values that change every run
// replaced by fixed placeholders so the file is stable and readable.
func writeSSEFixture(t *testing.T, frames []recordedFrame) {
	t.Helper()
	out := make([]recordedFrame, 0, len(frames))
	for _, f := range frames {
		var data any
		mustUnmarshal(t, string(f.Data), &data)
		blob, err := json.Marshal(stabilise(data))
		if err != nil {
			t.Fatalf("marshal %s: %v", f.Name, err)
		}
		out = append(out, recordedFrame{Name: f.Name, Data: blob})
	}

	blob, err := json.MarshalIndent(out, "", "\t")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(sseFixturePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sseFixturePath, append(blob, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", sseFixturePath, err)
	}
	t.Logf("wrote %s (%d frames)", sseFixturePath, len(out))
}

func stabilise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			switch {
			case k == "search_id":
				out[k] = placeholderSearchID
			case k == "duration_ms":
				out[k] = 0
			case timestampFields[k]:
				out[k] = placeholderTime
			default:
				out[k] = stabilise(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = stabilise(t[i])
		}
		return out
	default:
		return v
	}
}
