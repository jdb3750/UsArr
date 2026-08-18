package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/httpapi"
)

// The TERMINAL FAILURE FRAME, end to end through the real binary.
//
// docs/reference/http-api.md §5.5 is the contract these tests are written to,
// and it was written before the producer existed: a run that stops publishes
// one `import.progress` frame with phase "stopped", on the SAME struct, with NO
// new field and no cause of any kind. Before it, a failed import published
// nothing at all and silence meant three different things — failed, still
// running, dead server (REVIEW-LOG LS-152).
//
// WHAT IS ASSERTED HERE THAT NOTHING ELSE ASSERTS. The healthy-path tests end
// on `done` and cannot see a failure at all; internal/libsync tests the
// importer, which is blind to two of these failures because they happen before
// an Importer exists. Only the wiring in cmd/usarr can be held to §5.5.

// importFaultBody is the upstream 500's body in these tests. It is
// SELF-LABELLING and unmistakable so that assertNoSecret over the stream dump
// is a real check: if any upstream text ever reaches the frame, this string is
// what reaches it.
const importFaultBody = "kavita-said-this-and-it-must-not-reach-the-browser"

// stoppedFrameFields is the frame's WHOLE field set (§5.5.1). `total` is
// omitempty and only the credits publish site sets it, so a stopped frame has
// four keys and no more. The test is on the KEY SET rather than on the values
// of a struct, because a `reason` field added later would decode into no Go
// field and a struct-based assertion would never see it.
var stoppedFrameFields = []string{"applied", "instance_id", "items_read", "phase"}

// awaitImportPhase reads import.progress frames for one instance until the
// named phase arrives, and returns that frame decoded as raw keys plus every
// phase seen on the way, in order.
//
// It returns the raw key map on purpose: see stoppedFrameFields.
func awaitImportPhase(
	t *testing.T, stream *sseStream, instanceID int64, phase string,
) (map[string]json.RawMessage, []string) {
	t.Helper()
	var seen []string
	deadline := time.After(20 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("no %s frame with phase %q for instance %d arrived; phases seen: %v. Stream:\n%s",
				httpapi.EventImportProgress, phase, instanceID, seen, stream.dump())
			return nil, nil
		case ev := <-stream.events:
			if ev.name != httpapi.EventImportProgress {
				continue
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(ev.data), &raw); err != nil {
				t.Fatalf("decode %s frame %q: %v", ev.name, ev.data, err)
			}
			var envelope struct {
				InstanceID int64  `json:"instance_id"`
				Phase      string `json:"phase"`
			}
			mustUnmarshal(t, ev.data, &envelope)
			if envelope.InstanceID != instanceID {
				continue
			}
			seen = append(seen, envelope.Phase)
			if envelope.Phase == phase {
				return raw, seen
			}
		}
	}
}

// assertNoPhase drains whatever is already on the stream and fails if any frame
// for this instance carries the named phase.
//
// ⚠️ IT EXISTS BECAUSE awaitImportPhase CANNOT SEE PAST WHAT IT WAITED FOR. A
// producer that published `stopped` UNCONDITIONALLY — after `done`, on a run
// that succeeded — passed every other assertion in this file, because the wait
// for `done` returned before the bad frame was read. That break is the reason
// for this helper, and the reason the window is drained rather than sampled:
// the publish is synchronous inside FullImport, so by the time the caller has
// returned, an offending frame is already in the channel.
func assertNoPhase(t *testing.T, stream *sseStream, instanceID int64, phase string) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case <-deadline:
			return
		case ev := <-stream.events:
			if ev.name != httpapi.EventImportProgress {
				continue
			}
			var envelope struct {
				InstanceID int64  `json:"instance_id"`
				Phase      string `json:"phase"`
			}
			mustUnmarshal(t, ev.data, &envelope)
			if envelope.InstanceID == instanceID && envelope.Phase == phase {
				t.Errorf("a %q frame arrived for instance %d when none was allowed: %s",
					phase, instanceID, ev.data)
			}
		}
	}
}

// assertStoppedFrameShape holds one frame to §5.5.1: the four fields, no fifth,
// and no `total`.
func assertStoppedFrameShape(t *testing.T, raw map[string]json.RawMessage, instanceID int64) {
	t.Helper()
	got := make([]string, 0, len(raw))
	for k := range raw {
		got = append(got, k)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(stoppedFrameFields, ",") {
		t.Errorf("the stopped frame carries fields %v, want exactly %v — §5.5.1 adds NO field, "+
			"and §5.5.5 forbids a cause anywhere on this frame", got, stoppedFrameFields)
	}
	if string(raw["phase"]) != `"stopped"` {
		t.Errorf("phase = %s, want \"stopped\" — §5.5.2: `failed` is taken in the SPA with the "+
			"OPPOSITE meaning", raw["phase"])
	}
	var id int64
	mustUnmarshal(t, string(raw["instance_id"]), &id)
	if id != instanceID {
		t.Errorf("the stopped frame names instance %d, want %d", id, instanceID)
	}
}

// newImportedKavita brings up an app with one healthy Kavita registered and a
// stream open on it, WITHOUT arming the bootstrap import: every import in these
// tests is one this test asked for, so a failure cannot be a race with a
// bootstrap goroutine.
func newImportedKavita(t *testing.T, kav *fakeKavita) (*testEnv, *sseStream, serviceBody) {
	t.Helper()
	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	stream := env.openStream(t)
	t.Cleanup(stream.close)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)
	return env, stream, created
}

func TestAFailedImportPublishesATerminalStoppedFrame(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = []map[string]any{kavitaSeries(41, 1, "Frieren", nil)}

	env, stream, created := newImportedKavita(t, kav)

	// The failure is MID-IMPORT: the importer exists, the libraries are already
	// bound and the `containers` frame is already out when the series read
	// dies. That is the case FullImport's own error paths cover and the one
	// that used to return before the terminal publish.
	kav.failSeriesList(importFaultBody)

	if _, err := env.app.registry.FullImport(t.Context(), created.ID); err == nil {
		t.Fatal("the import of a Kavita whose series list 500s was reported as successful")
	}

	raw, seen := awaitImportPhase(t, stream, created.ID, "stopped")
	assertStoppedFrameShape(t, raw, created.ID)

	// It is TERMINAL, and it comes after the frames the run did manage to send:
	// a client that saw `containers` and then silence could not tell a failure
	// from a hang, which is the whole defect.
	if len(seen) == 0 || seen[len(seen)-1] != "stopped" {
		t.Errorf("phases seen = %v, want the run to end on stopped", seen)
	}
	if len(seen) < 2 || seen[0] != "containers" {
		t.Errorf("phases seen = %v, want the frames the run sent before it stopped", seen)
	}

	// last_full_sync_at is NOT written (§5.5.4): the frame is the fast notice,
	// the column is the evidence, and a stopped run must never move it.
	at, err := env.app.store.LastFullSyncAt(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("read last_full_sync_at: %v", err)
	}
	if at.Valid {
		t.Error("last_full_sync_at moved on a failed import; the stopped frame would be " +
			"contradicted by the only authoritative evidence a client has")
	}

	// §5.5.5: no upstream message, body, URL or status line may ride this
	// stream. The upstream said importFaultBody and the operator reads it in
	// the process log — never here.
	assertNoSecret(t, "SSE stream", stream.dump(),
		importFaultBody, importAuthKey, "/mnt/user/media")
}

func TestASecondRunSupersedesAStoppedFrameAndASuccessPublishesNone(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = []map[string]any{kavitaSeries(41, 1, "Frieren", nil)}

	env, stream, created := newImportedKavita(t, kav)

	kav.failSeriesList(importFaultBody)
	if _, err := env.app.registry.FullImport(t.Context(), created.ID); err == nil {
		t.Fatal("the import of a Kavita whose series list 500s was reported as successful")
	}
	raw, _ := awaitImportPhase(t, stream, created.ID, "stopped")
	assertStoppedFrameShape(t, raw, created.ID)

	// §5.5.3: the frame carries NO run id, so a later non-terminal frame for the
	// same instance is not a retraction — it is a SECOND RUN. The producer's
	// half of that rule is what is testable here: a re-import after a stop
	// publishes the ordinary phases again, with counters starting over.
	kav.failSeriesList("")
	if _, err := env.app.registry.FullImport(t.Context(), created.ID); err != nil {
		t.Fatalf("the second import failed: %v. Process log:\n%s", err, env.logs())
	}

	_, seen := awaitImportPhase(t, stream, created.ID, "done")
	if len(seen) == 0 || seen[0] != "containers" {
		t.Errorf("phases after the stop = %v, want a fresh run beginning at containers", seen)
	}
	// A SUCCESSFUL RUN PUBLISHES NO STOPPED FRAME. Without this the producer
	// could satisfy every other assertion here by publishing the frame
	// unconditionally, which would make it worthless.
	for _, phase := range seen {
		if phase == "stopped" {
			t.Errorf("a successful import published a stopped frame; phases = %v", seen)
		}
	}
	// And none arrives AFTER `done` either. The loop above alone is not the
	// guard it looks like — see assertNoPhase.
	assertNoPhase(t, stream, created.ID, "stopped")
}

func TestANonCatalogueServicePublishesAStoppedFrame(t *testing.T) {
	// The FIRST of the two failures that happen before an Importer exists
	// (§5.1). libsync cannot publish this one — it is refused in cmd/usarr, one
	// frame above libsync.FullImport — which is why the producer sits where it
	// does.
	prow := newFakeProwlarr(t, importProwlarrKey)
	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	stream := env.openStream(t)
	defer stream.close()

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prow.URL(),
		"api_key": importProwlarrKey,
	}, &created)

	if _, err := env.app.registry.FullImport(t.Context(), created.ID); err == nil {
		t.Fatal("a full import of a Prowlarr was accepted")
	}

	raw, _ := awaitImportPhase(t, stream, created.ID, "stopped")
	assertStoppedFrameShape(t, raw, created.ID)
	// Nothing ran, so nothing stands. Zero here is a real count and not a
	// placeholder: §5.5.4's "say how much stands" answers "none".
	for _, field := range []string{"items_read", "applied"} {
		if string(raw[field]) != "0" {
			t.Errorf("%s = %s on a run that never started, want 0", field, raw[field])
		}
	}
	assertNoSecret(t, "SSE stream", stream.dump(), importProwlarrKey)
}

func TestACredentialThatWillNotOpenPublishesAStoppedFrame(t *testing.T) {
	// The SECOND pre-importer failure (§5.1), and the one §4.4 maps to a `500
	// internal`: the stored credential does not open, so nothing is sent
	// upstream at all. It is reproduced the way it actually happens — the
	// sealed bytes no longer match what the keyring can open — rather than by
	// faking the error.
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	env, stream, created := newImportedKavita(t, kav)

	if err := env.app.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET api_key_enc = ? WHERE id = ?`,
			[]byte("this envelope will not open"), created.ID)
		return err
	}); err != nil {
		t.Fatalf("corrupt the sealed credential: %v", err)
	}

	if _, err := env.app.registry.FullImport(t.Context(), created.ID); err == nil {
		t.Fatal("an import ran with a credential that cannot be opened")
	}

	raw, seen := awaitImportPhase(t, stream, created.ID, "stopped")
	assertStoppedFrameShape(t, raw, created.ID)
	// No `containers` frame precedes it: this failure is decided before an
	// Importer exists, so `stopped` is the ONLY frame the run produces.
	if len(seen) != 1 {
		t.Errorf("phases seen = %v, want stopped alone — nothing upstream was reached", seen)
	}
	assertNoSecret(t, "SSE stream", stream.dump(), importAuthKey)
}
