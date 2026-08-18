package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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
// It records TWO runs into the one file, because the stream carries two
// unrelated subsystems and a recording of only one of them pins only half the
// contract: a Prowlarr search, and a Kavita catalogue import. The import half is
// the reason the fixture has to drive a second service at all — `import.progress`
// has no search_id, is published by a different package (internal/libsync,
// through cmd/usarr's importProgress), and carries `total`, a field no search
// frame has.
//
// NOT RECORDED, and it is four of the seven names rather than a gap:
// `search.failed` and `stream.missed` do not occur on a healthy run, so there is
// nothing here to record them from — web/src/lib/api.test.ts covers those two
// from synthesised payloads instead.
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
	// age_days is derived from wall-clock now. It keeps its real recorded value
	// in the fixture, because the SPA's own test reads it, but it cannot be
	// compared against a later run.
	"age_days": true,
}

// timestampFields hold a timestamp and NOTHING else, so the whole value is
// replaced by a fixed placeholder when the recording is regenerated and the
// checked-in file does not churn.
var timestampFields = map[string]bool{
	"expires_at":    true,
	"published_at":  true,
	"started_at":    true,
	"finished_at":   true,
	"blocked_until": true,
}

// embeddedTimestampFields hold PROSE with a wall-clock timestamp inside it —
// releases.blockedReason emits "Prowlarr has this indexer blocked until
// 2026-08-17T02:46:36Z". Blanking the whole value the way timestampFields are
// blanked would throw away the sentence, and the sentence is the signal: it is
// what tells "blocked until a deadline" apart from "blocked after repeated
// failures" and from "indexer is disabled in Prowlarr", all three of which the
// SPA renders verbatim into the degraded banner (web/src/lib/api.ts,
// indexerProblems). So only the timestamp inside the string is normalised, both
// when the fixture is written and again on both sides before it is compared —
// the prose keeps being checked, and only the part that cannot be reproduced
// stops being checked.
var embeddedTimestampFields = map[string]bool{
	"reason": true,
}

// rfc3339InText matches an RFC 3339 instant anywhere inside a string.
// time.RFC3339 is what blockedReason formats with; the optional fractional
// seconds and numeric offset are there so a later caller that formats with
// RFC3339Nano or in a non-UTC zone is normalised too rather than silently
// reintroducing the churn.
var rfc3339InText = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)

// normaliseEmbeddedTimestamps rewrites every RFC 3339 instant inside a string
// value to placeholderTime and leaves every other byte alone. Non-strings pass
// through untouched. It is idempotent: placeholderTime is itself RFC 3339, so
// running it over an already-normalised fixture value is a no-op, which is what
// lets covers() run it over the recorded and the live value alike.
func normaliseEmbeddedTimestamps(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	return rfc3339InText.ReplaceAllLiteralString(s, placeholderTime)
}

const (
	placeholderSearchID = "SEARCH_ID"
	placeholderTime     = "2026-08-16T00:00:00Z"
	// placeholderDurationMS must not be 0. duration_ms is `omitempty` on an
	// int64 (search.go), so 0 is the one value the wire can never carry:
	// recording it would pin a value that no later run can reproduce, and
	// covers() — which requires every recorded key to still be PRESENT — would
	// then fail with "field is gone from the wire" on any run whose legs
	// finished inside a millisecond. Any non-zero placeholder is fine, because
	// duration_ms is volatile and only its presence is compared.
	placeholderDurationMS = 1
)

// searchLegDelay is held by the Prowlarr double on every search leg, so
// duration_ms is deterministically non-zero and therefore deterministically ON
// the wire. Without it the field rounds to 0, omitempty drops it, and the
// recording cannot pin its spelling at all — the SPA reads value.duration_ms
// (web/src/lib/api.ts) against nothing.
const searchLegDelay = 2 * time.Millisecond

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
	prowlarr.slowSearches(searchLegDelay)

	stream := env.openStream(t)
	defer stream.close()

	var accepted searchAcceptedBody
	env.do(t, "GET", "/api/v1/search?query=Test+Release&type=search&category=2000", nil, &accepted)
	if accepted.SearchID == "" {
		t.Fatal("the search must return a search id immediately")
	}

	live := collectSearchFrames(t, stream, accepted.SearchID)

	// ── the second subsystem on the same stream ─────────────────────────────
	//
	// A Kavita is added AFTER the search has finished, so no import frame can be
	// swallowed by collectSearchFrames — that collector drops every frame whose
	// search_id is not the one it wants, and an import.progress frame has no
	// search_id at all.
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = []map[string]any{
		kavitaSeries(41, 1, "Frieren", nil),
		// No metadata entry, so this one contributes a credit REQUEST and no
		// credits — which is what makes the credits phase's `applied` differ
		// from its `total` in the recording rather than coincide with it.
		kavitaSeries(42, 1, "Saga", nil),
	}
	kav.metadata = map[int]map[string]any{
		41: kavitaMetadata(41, map[string][]string{
			"writers": {"Kanehito Yamada"}, "coverArtists": {"Tsukasa Abe"},
			"pencillers": {"Tsukasa Abe"},
		}),
	}
	armBootstrapImport(t, env)

	var kavitaSvc serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &kavitaSvc)
	live = append(live, collectImportFrames(t, stream, kavitaSvc.ID)...)

	// ── the names, exactly ──────────────────────────────────────────────────
	//
	// search.failed and stream.missed are real names (events.go) but neither
	// occurs on a healthy run, so they are covered by the client's unit test
	// rather than recorded here.
	wantNames := []string{
		httpapi.EventImportProgress,
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

	// duration_ms is `omitempty`, so it is on the wire only when a leg took a
	// whole millisecond — which is why slowSearches is set above. Asserting it
	// by name here is what stops a future regeneration from quietly dropping the
	// field back out of the recording and leaving its spelling unobserved: a
	// fixture that does not carry it cannot fail when it is renamed.
	if !anyIndexerOutcomeHasField(t, live, "duration_ms") {
		t.Errorf("no indexer outcome on the stream carries %q even though the Prowlarr double "+
			"was made to take %s per leg; web/src/lib/api.ts reads it", "duration_ms", searchLegDelay)
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

// collectImportFrames reads import.progress frames for one instance until the
// terminal `done` arrives, and returns the LAST frame of each phase in the order
// the phases occur.
//
// One per phase, and the last one, for two reasons. The batcher flushes on
// min(2000 rows, 100 ms) (internal/libsync), so how MANY items/credits frames a
// run produces is wall-clock dependent and nothing stable can be recorded from
// it — but the last frame of a phase always carries that phase's final counts,
// which are a function of the fixture data alone. And all four phases have to be
// in the recording, because they do not carry the same fields: only `credits`
// ever sets `total`, so recording `done` alone would leave that field's spelling
// unpinned on the wire the SPA reads it from.
//
// ⚠️ The loop can only end on `done`, so a run that STOPS parks it until the
// deadline. That is deliberate: this helper records a HEALTHY run's frames, and
// a `stopped` in the recording would be a broken fixture, not a shape to pin.
//
// ⚠️ THIS COMMENT USED TO SAY "a FAILED import publishes nothing at all", and
// the failure message with it. That was true of the whole tree and is now false:
// a failed run publishes one terminal `stopped` frame (http-api.md §5.5,
// REVIEW-LOG LS-180), published by cmd/usarr around fullImportLocked rather than
// by libsync — whose error paths do still all return before its own terminal
// publish, which is why the frame could not live there. The deadline therefore
// no longer means "failure and hang are indistinguishable": the phases it
// reports include `stopped` when one arrived. cmd/usarr/import_stopped_test.go
// is where that frame is asserted; nothing here drives a failure.
func collectImportFrames(t *testing.T, stream *sseStream, instanceID int64) []recordedFrame {
	t.Helper()
	byPhase := map[string]recordedFrame{}
	var order []string
	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("no terminal %s frame for instance %d within the deadline; "+
				"got phases %v. A `stopped` among them is a FAILED import (§5.5) and the "+
				"fixture data is what to look at; no frames at all is a hang or an "+
				"unwired hub. Stream:\n%s",
				httpapi.EventImportProgress, instanceID, order, stream.dump())
		case ev := <-stream.events:
			if ev.name != httpapi.EventImportProgress {
				continue
			}
			var envelope struct {
				InstanceID int64  `json:"instance_id"`
				Phase      string `json:"phase"`
			}
			mustUnmarshal(t, ev.data, &envelope)
			if envelope.InstanceID != instanceID {
				continue
			}
			if _, seen := byPhase[envelope.Phase]; !seen {
				order = append(order, envelope.Phase)
			}
			byPhase[envelope.Phase] = recordedFrame{
				Name: ev.name, Data: json.RawMessage(ev.data),
			}
			if envelope.Phase == "done" {
				out := make([]recordedFrame, 0, len(order))
				for _, phase := range order {
					out = append(out, byPhase[phase])
				}
				return out
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
			if embeddedTimestampFields[key] {
				// Compare the prose, not the deadline buried in it. The
				// recorded side is already normalised by stabilise(), and
				// normalising it again is a no-op.
				wv, gv = normaliseEmbeddedTimestamps(wv), normaliseEmbeddedTimestamps(gv)
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

// anyIndexerOutcomeHasField reports whether any search.indexer frame's outcome
// object carries the named key.
func anyIndexerOutcomeHasField(t *testing.T, frames []recordedFrame, field string) bool {
	t.Helper()
	for _, f := range frames {
		if f.Name != httpapi.EventSearchIndexer {
			continue
		}
		var payload struct {
			Indexer map[string]any `json:"indexer"`
		}
		mustUnmarshal(t, string(f.Data), &payload)
		if _, ok := payload.Indexer[field]; ok {
			return true
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
				out[k] = placeholderDurationMS
			case timestampFields[k]:
				out[k] = placeholderTime
			case embeddedTimestampFields[k]:
				out[k] = normaliseEmbeddedTimestamps(val)
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
