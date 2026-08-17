package httpapi

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// The SPA's unit tests replay web/src/lib/__fixtures__/sse-frames.json through
// normalizeStreamEvent, and cmd/usarr's TestSSEFramesMatchTheClientContract
// proves every field the fixture records is still on the wire. Neither catches
// the opposite drift: a field in the fixture that the API does NOT emit. The
// contract test tolerates it by design — covers() lets the LIVE payload carry
// more than the recording, and a key present only in the recording fails only
// if the wire is missing it, which a hand-edited fixture can hide by being
// checked in without a regeneration.
//
// So the fixture can grow a field nobody serves, the SPA's tests go green
// against it, and the screen renders from a key that does not exist in
// production. This test closes that direction: every recorded frame is decoded
// into the Go type the handler actually marshals, with DisallowUnknownFields,
// so a key the response struct does not declare is a failure here.
//
// NOT COVERED, deliberately:
//   - the SSE envelope. search.go publishes map[string]any literals, so there is
//     no production struct for the outer {search_id, phase, indexer} object.
//     The envelopes below mirror those literals by hand; they pin what the
//     FIXTURE may contain, not what search.go emits. A key added to the map
//     literal and never recorded stays invisible to this test.
//   - search.failed and stream.missed. Neither occurs on a healthy run, so the
//     recording has no frame of either name, and there is nothing here to
//     decode. The table rejects unknown names so adding one to the fixture
//     forces this test to grow.
//   - everything outside the SSE stream. sse-frames.json is the only fixture
//     under web/src; the REST response shapes have no recorded counterpart to
//     check, so this proves nothing about them.
const sseFixturePath = "../../web/src/lib/__fixtures__/sse-frames.json"

type startedFrame struct {
	SearchID   string `json:"search_id"`
	InstanceID int64  `json:"instance_id"`
	Query      string `json:"query"`
	Type       string `json:"type"`
}

type indexerFrame struct {
	SearchID string                 `json:"search_id"`
	Phase    string                 `json:"phase"`
	Indexer  indexerOutcomeResponse `json:"indexer"`
}

type resultsFrame struct {
	SearchID string            `json:"search_id"`
	Results  []releaseResponse `json:"results"`
}

type doneFrame struct {
	SearchID string         `json:"search_id"`
	Report   reportResponse `json:"report"`
}

func TestRecordedFramesCarryNoFieldTheAPINeverEmits(t *testing.T) {
	// One fresh destination per frame, because a decoder cannot report unknown
	// fields into a shared value.
	targets := map[string]func() any{
		EventSearchStarted: func() any { return new(startedFrame) },
		EventSearchIndexer: func() any { return new(indexerFrame) },
		EventSearchResults: func() any { return new(resultsFrame) },
		EventSearchDone:    func() any { return new(doneFrame) },
	}

	blob, err := os.ReadFile(sseFixturePath)
	if err != nil {
		t.Fatalf("read %s: %v", sseFixturePath, err)
	}
	var frames []struct {
		Name string          `json:"name"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(blob, &frames); err != nil {
		t.Fatalf("decode %s: %v", sseFixturePath, err)
	}
	if len(frames) == 0 {
		t.Fatalf("%s records no frames", sseFixturePath)
	}

	seen := map[string]bool{}
	for i, frame := range frames {
		newTarget, ok := targets[frame.Name]
		if !ok {
			t.Errorf("%s frame %d records event %q, which this test has no Go type for; "+
				"add one rather than letting the frame go unchecked", sseFixturePath, i, frame.Name)
			continue
		}
		seen[frame.Name] = true

		dec := json.NewDecoder(bytes.NewReader(frame.Data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(newTarget()); err != nil {
			t.Errorf("%s frame %d (%s) does not fit the shape the API emits: %v\n"+
				"the SPA reads this recording; a field here that no handler marshals renders "+
				"from a key that does not exist in production",
				sseFixturePath, i, frame.Name, err)
		}
	}

	// A recording that has quietly lost a frame kind stops proving anything
	// about that kind, silently.
	for name := range targets {
		if !seen[name] {
			t.Errorf("%s records no %s frame, so its shape is unchecked; "+
				"regenerate with USARR_UPDATE_SSE_FIXTURE=1", sseFixturePath, name)
		}
	}
}
