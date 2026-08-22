package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// THE CONTENT-FILTER SHORTFALL, END TO END THROUGH THE REAL BINARY.
//
// internal/libsync tests the measurement against a hand-built reader and
// internal/httpapi tests the wire against hand-written rows; neither reaches the
// WIRING — whether an import through the real startup path actually probes the
// stats route and lands a row. That is the same argument
// TestAddingABookOrbitProducesACatalogue makes for the catalogue itself.
//
// The only double is the BookOrbit. The database is real.

// boCompletenessNote reads back the newest completeness verdict for a container.
func boCompletenessNote(t *testing.T, env *testEnv, remoteID string) store.CompletenessNote {
	t.Helper()
	var detail string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT detail FROM sync_report
		 WHERE kind = ? AND remote_kind = 'library' AND remote_id = ?
		 ORDER BY id DESC LIMIT 1`,
		store.SyncReportContentCompleteness, remoteID).Scan(&detail); err != nil {
		t.Fatalf("no completeness row for library %s: %v", remoteID, err)
	}
	var note store.CompletenessNote
	if err := json.Unmarshal([]byte(detail), &note); err != nil {
		t.Fatalf("decode the completeness detail %q: %v", detail, err)
	}
	return note
}

// completenessReasonsOnTheWire is EVERY value CompletenessNote.Reason is allowed
// to hold once it has landed in sync_report.detail. HAND-COPIED from
// internal/libsync/bookorbitcompleteness.go.
//
// ⚠️ HAND-COPIED ON PURPOSE, AND THE DUPLICATION WITH internal/libsync's OWN
// COPY IS ALSO ON PURPOSE. A set derived from the production constants — or
// shared with that test through an exported helper — is satisfied by whatever
// the producer produces, which is precisely the leak this guard is here to
// catch. Two independent hardcoded copies mean a new arm has to be admitted
// twice, by hand, by someone who has read it. Do not tidy either into the other.
//
// ⚠️ IT REPLACED A SUBSTRING BLACKLIST over "Forbidden resource" and "403",
// which ran on the 403 scenario and named only the tokens that scenario
// produces. Appending err.Error() to completenessReason's default arm left it
// green — measured, not supposed.
//
// "" is a member: Reason is unset under `complete` and `shortfall`. Where an
// empty reason would itself be the defect, Reason != "" is asserted separately.
var completenessReasonsOnTheWire = map[string]bool{
	"": true,
	"BookOrbit refused UsArr the statistics this check compares against":     true,
	"BookOrbit has no statistics route for this library":                     true,
	"the statistics this check compares against could not be read":           true,
	"the two counts disagreed in the direction only a moving table explains": true,
}

func assertNoteReasonIsUsArrsOwnWords(t *testing.T, reason string) {
	t.Helper()
	if completenessReasonsOnTheWire[reason] {
		return
	}
	t.Errorf("sync_report.detail carries a completeness reason that is not one of "+
		"UsArr's own sentences.\n"+
		"  got: %q\n"+
		"This value reaches a browser through GET /api/v1/libraries, and "+
		"reference/security.md §5 keeps upstream response bodies out of this column. "+
		"Either an upstream error string leaked in — fix the producer in "+
		"internal/libsync/bookorbitcompleteness.go — or a new arm was added there, in "+
		"which case add its literal to completenessReasonsOnTheWire in this file, by "+
		"hand. Do NOT derive this set from the production constants: a derived set "+
		"passes for every future arm automatically.", reason)
}

// TestABookOrbitContentFilterIsDetectedAndRecorded is the feature: BookOrbit's
// library listing is shorted by a content filter on UsArr's shared account, the
// unfiltered stats route says how many books are really there, and the
// difference lands in the database where the Libraries screen reads it.
//
// The measurement is verified at bookorbit/bookorbit@73b7877d —
// findAllForUser's bookCount and getStats's totalBooks are the same
// `status = 'present'` count over the same table, and only the first carries the
// account's content filter (library.repository.ts:30-51 and :150-178).
func TestABookOrbitContentFilterIsDetectedAndRecorded(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{
		boLibrary(1, "Fiction", 2),
		boLibrary(2, "Audio", 1),
	}
	// Fiction really holds five present books; the filtered listing showed two.
	bo.stats = map[int]int{1: 5}
	bo.books = map[int][]map[string]any{
		1: {boBook(101, "The Hobbit", nil), boBook(102, "Dune", nil)},
		2: {boBook(201, "Project Hail Mary", map[string]any{
			"files": []map[string]any{boFile(2011, "m4b", "primary")},
		})},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)
	waitForImport(t, env, created.ID)

	note := boCompletenessNote(t, env, "1")
	if note.State != string(store.CompletenessShortfall) {
		t.Fatalf("state = %q, want shortfall", note.State)
	}
	if note.Total != 5 || note.Visible != 2 || note.Hidden != 3 {
		t.Errorf("verdict = %+v, want 5 held / 2 visible / 3 hidden", note)
	}
	if note.Container != "Fiction" {
		t.Errorf("the row does not name its container: %+v", note)
	}
	// ⚠️ EVERY ROW STATES THE SCOPE OF ITS OWN CLAIM. A clean book-level check
	// must not be readable as "no library is hidden from UsArr", which is a
	// different question and is unanswerable read-only.
	if note.Covers == "" || note.DoesNotCover == "" {
		t.Errorf("the row does not say what it covers and what it does not: %+v", note)
	}

	// ⚠️ THE UNFILTERED LIBRARY GETS A ROW TOO, saying so positively. If only
	// shortfalls were recorded, "checked and fine" and "never checked" would be
	// the same absence — which is the defect this feature closes, recreated
	// inside its own fix.
	clean := boCompletenessNote(t, env, "2")
	if clean.State != string(store.CompletenessComplete) {
		t.Errorf("library 2's state = %q, want complete", clean.State)
	}
	if clean.Hidden != 0 {
		t.Errorf("a complete library claims %d hidden", clean.Hidden)
	}
	// The `shortfall` and `complete` arms, which leave Reason unset.
	assertNoteReasonIsUsArrsOwnWords(t, note.Reason)
	assertNoteReasonIsUsArrsOwnWords(t, clean.Reason)

	// AND THE IMPORT STILL RAN. A shortfall is a report, never a refusal: the
	// books the credential COULD see are in the catalogue.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work`); n != 3 {
		t.Errorf("works = %d, want 3 — a shortfall must not cost the books that DID import", n)
	}

	// ── and it reaches the screen ───────────────────────────────────────────
	//
	// The verdict is filed against a CONTAINER; the Libraries screen renders a
	// LIBRARY. This is the join between them, and it is the half a store test
	// with hand-written rows cannot prove: container_ref and remote_id have to
	// be the same string, written by two different packages.
	// The library exists because it was accepted (ADR-0048): completeness is a
	// verdict ATTACHED to a library_source row, so there is nothing to attach it
	// to until a proposal is ticked.
	acceptLibraries(t, env, created.ID, acceptSpec{ref: "1", name: "Fiction", kind: "book"})

	var libs struct {
		Items []struct {
			Name         string `json:"name"`
			Completeness *struct {
				State   string `json:"state"`
				Total   int64  `json:"total_items"`
				Visible int64  `json:"visible_items"`
				Hidden  int64  `json:"hidden_items"`
			} `json:"completeness"`
		} `json:"items"`
	}
	env.do(t, "GET", "/api/v1/libraries", nil, &libs)
	var seen int
	for _, l := range libs.Items {
		if l.Name != "Fiction" {
			continue
		}
		seen++
		if l.Completeness == nil {
			t.Fatal("the Libraries wire carries no completeness for a library that was measured")
		}
		if l.Completeness.State != "shortfall" || l.Completeness.Total != 5 ||
			l.Completeness.Visible != 2 || l.Completeness.Hidden != 3 {
			t.Errorf("the wire's verdict = %+v, want shortfall 5/2/3", l.Completeness)
		}
	}
	if seen != 1 {
		t.Fatalf("Fiction is not on the Libraries wire: %+v", libs.Items)
	}
	t.Logf("bookorbit completeness: Fiction holds 5, the service account sees 2")
}

// TestAGuardedStatsRouteRecordsUnverifiedRatherThanComplete is the named
// degradation condition, driven through the binary.
//
// The check depends on `GET /libraries/:id/stats` carrying no
// @RequirePermission. If BookOrbit ever guards it, every probe answers 403 — and
// the recorded answer must be "we could not check", never a silent "complete".
func TestAGuardedStatsRouteRecordsUnverifiedRatherThanComplete(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 2)}
	bo.statsStatus = map[int]int{1: http.StatusForbidden}
	bo.books = map[int][]map[string]any{
		1: {boBook(101, "The Hobbit", nil), boBook(102, "Dune", nil)},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)
	waitForImport(t, env, created.ID)

	note := boCompletenessNote(t, env, "1")
	if note.State != string(store.CompletenessUnverified) {
		t.Fatalf("state = %q, want unverified — a refused probe must never read as complete", note.State)
	}
	if note.Total != -1 {
		t.Errorf("Total = %d, want -1: an unmeasured total must not be a number an "+
			"empty library could also produce", note.Total)
	}
	if note.Hidden != 0 {
		t.Errorf("an unverified verdict claims %d hidden", note.Hidden)
	}
	if note.Reason == "" {
		t.Error("an unverified verdict carries no reason, so no screen can say why")
	}
	// The refusal is the upstream's; its words are not. reference/security.md §5
	// keeps upstream response text out of this column, and it reaches a browser.
	assertNoteReasonIsUsArrsOwnWords(t, note.Reason)

	// And the catalogue is untouched by the refusal.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work`); n != 2 {
		t.Errorf("works = %d, want 2 — a failed completeness probe must not cost an import", n)
	}
}

// TestAFailedStatsProbeRecordsUsArrsOwnWordsAndNotTheUpstreamsBody drives
// completenessReason's DEFAULT arm through the binary.
//
// ⚠️ IT EXISTS BECAUSE THAT ARM IS THE ONE THAT LEAKS THE MOST AND WAS THE ONE
// NOTHING WATCHED. The 403 arm is reached from a sentinel comparison; the
// default arm is reached from an *bookorbit.APIError whose Error() concatenates
// the upstream's Message and its flattened Validation array (truncated at 512
// bytes), so a producer that appended err.Error() there would put up to that
// much of somebody else's response body into a column a browser renders. The
// internal/libsync sibling of this test drives the same arm from a bare
// errors.New, which cannot show that; only the real client can.
func TestAFailedStatsProbeRecordsUsArrsOwnWordsAndNotTheUpstreamsBody(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 2)}
	bo.statsStatus = map[int]int{1: http.StatusInternalServerError}
	bo.books = map[int][]map[string]any{
		1: {boBook(101, "The Hobbit", nil), boBook(102, "Dune", nil)},
	}

	env := boSetUp(t)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)
	waitForImport(t, env, created.ID)

	note := boCompletenessNote(t, env, "1")
	if note.State != string(store.CompletenessUnverified) {
		t.Fatalf("state = %q, want unverified — an unreadable probe must never read as complete", note.State)
	}
	if note.Reason == "" {
		t.Error("an unverified verdict carries no reason, so no screen can say why")
	}
	assertNoteReasonIsUsArrsOwnWords(t, note.Reason)

	// And the import still ran: a probe that could not be read is a report, not
	// a refusal.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work`); n != 2 {
		t.Errorf("works = %d, want 2 — a failed completeness probe must not cost an import", n)
	}
}
