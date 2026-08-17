package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/httpapi"
)

// The catalogue import's WIRING, end to end through the real binary's own
// startup path.
//
// internal/libsync and internal/store test the importer and the write path
// against a real migrated database. NOTHING TESTED THE WIRING: whether adding a
// Kavita through the HTTP API actually produces a catalogue in a running
// process. That is three separate hand-offs — registry.entry's on-connect hook,
// the prober's context, and the SSE hub attached after the server exists — and
// each of them is a place where a correct importer produces nothing at all.
//
// These tests drive the real buildApp, the real prober goroutine, the real
// service-creation endpoint and the real SSE stream. The only double is the
// Kavita itself.

const (
	importAuthKey = "0123456789abcdef0123456789abcdef"
	// Deliberately different from importAuthKey, and deliberately SELF-LABELLING
	// rather than another run of hex. A near-miss like `…abcdee` is not covered
	// by .gitleaks.toml's sequential-hex allowlist and trips `make secrets` — and
	// that file's own instruction is to make a fixture obviously fake rather than
	// to extend the allowlist, which is the cheaper fix and keeps the list short.
	importProwlarrKey = "prowlarrKEY0123456789abcdef"
)

// kavitaSeries builds one POST /api/Series/all-v2 element. Field names are
// SeriesDto's, from api/specs/kavita.json.
func kavitaSeries(id, libraryID int, name string, extra map[string]any) map[string]any {
	s := map[string]any{
		"id": id, "libraryId": libraryID, "name": name, "sortName": name,
		"originalName": "", "localizedName": "",
		"pages": 100, "format": 1,
		"folderPath": "/mnt/user/media/" + name,
		"created":    "2026-08-01T10:00:00Z",
		// The UTC watermark, not the server's local clock.
		"lastChapterAddedUtc": "2026-08-17T07:00:30.118Z",
		"lastChapterAdded":    "2026-08-17T09:00:30.118Z",
		// DEGRADED IDENTITY IS THE DEFAULT HERE because it is the ordinary case:
		// a free Kavita writes none of the Kavita+ identifier fields.
		"aniListId": 0, "malId": 0, "hardcoverId": 0, "metronId": 0,
		"comicVineId": "", "mangaBakaId": 0, "cbrId": 0,
	}
	for k, v := range extra {
		s[k] = v
	}
	return s
}

// armBootstrapImport does what main() does one line before it starts the
// prober: it gives the registry a process-lifetime context, which is the switch
// that turns the on-connect import on. A test that does not call this gets no
// background import, which is why every OTHER test in this package is
// unaffected by it.
func armBootstrapImport(t *testing.T, env *testEnv) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	env.app.registry.enableBootstrapImport(ctx)
}

func countIn(t *testing.T, env *testEnv, q string) int {
	t.Helper()
	var n int
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), q).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// waitForImport polls until the instance reports a COMPLETED full sync.
//
// It polls last_full_sync_at rather than sleeping, and it polls THAT column
// rather than a row count, because the column is written on success only — a
// count reaching its target says a batch committed, not that the import
// finished.
func waitForImport(t *testing.T, env *testEnv, instanceID int64) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		at, err := env.app.store.LastFullSyncAt(t.Context(), instanceID)
		if err == nil && at.Valid {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no full import completed within the deadline. Process log:\n%s", env.logs())
}

func TestAddingAKavitaProducesACatalogue(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	// Library 1 → LibraryType 0 (Manga) → comic; library 2 → 1 (Comic) → comic;
	// library 3 → 2 (Book) → book. The kind comes from the LIBRARY in all three
	// cases and from no field on any series.
	kav.libraries = 3
	kav.series = []map[string]any{
		kavitaSeries(41, 1, "Frieren", map[string]any{
			"originalName": "葬送のフリーレン", "sortName": "Frieren",
		}),
		kavitaSeries(42, 2, "Saga", nil),
		kavitaSeries(43, 3, "The Hobbit", map[string]any{"pages": 310}),
		// Two series in DIFFERENT libraries of the SAME kind carrying the SAME
		// strong id: §6.4 tier 1 must resolve them onto one work.
		kavitaSeries(44, 1, "Berserk", map[string]any{"aniListId": 30013}),
		kavitaSeries(45, 2, "Berserk (Deluxe)", map[string]any{"aniListId": 30013}),
	}

	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	// The stream is opened BEFORE the instance exists, the way a browser sitting
	// on the Services screen would have it open.
	stream := env.openStream(t)
	defer stream.close()

	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)
	waitForImport(t, env, created.ID)

	// ── the catalogue ───────────────────────────────────────────────────────
	//
	// Three containers, three libraries, and the kind of each derived from the
	// LibraryType rather than from anything in a series payload.
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE id <> 0 AND managed_by = 'auto'`); n != 3 {
		t.Errorf("auto libraries = %d, want 3", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE kind = 'comic' AND id <> 0`); n != 2 {
		t.Errorf("comic libraries = %d, want 2 (LibraryType 0 and 1)", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE kind = 'book'`); n != 1 {
		t.Errorf("book libraries = %d, want 1 (LibraryType 2)", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library_source WHERE container_kind = 'remote_library'`); n != 3 {
		t.Errorf("library_source rows = %d, want 3", n)
	}

	// Five series, FOUR works: the two Berserk rows share an AniList id and tier
	// 1 resolves them onto one work across two libraries.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work`); n != 4 {
		t.Errorf("work rows = %d, want 4 — the two Berserk series are one work", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM service_item_link`); n != 5 {
		t.Errorf("service_item_link rows = %d, want 5 — one per remote series", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind = 'book'`); n != 1 {
		t.Errorf("book works = %d, want 1", n)
	}
	// Frieren's library is LibraryType 0 (Manga), so its work reads right-to-left.
	var frierenDir string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT wc.reading_direction FROM work_comic wc
		  JOIN work w ON w.id = wc.work_id WHERE w.title = 'Frieren'`).Scan(&frierenDir); err != nil {
		t.Fatalf("read Frieren's reading direction: %v", err)
	}
	if frierenDir != "rtl" {
		t.Errorf("Frieren reading_direction = %q, want rtl — LibraryType 0 is the only input there is", frierenDir)
	}
	// ⚠️ THE SHARED WORK'S reading_direction IS LAST-WRITER-WINS, and this
	// asserts it rather than pretending otherwise. One work now sits in a Manga
	// library (rtl) and a Comic library (ltr), and work_comic holds exactly one
	// value; the second container to claim the work overwrites the first. That is
	// inherent to tier-1 reuse across containers of different reading conventions
	// — the alternative would be to refuse the merge, which §6.4 rules out for
	// works of the same kind. It lands in a column §6.5 rule 4 makes editable,
	// which is the cheapest place in the design to be wrong; see LS-07.
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM work_comic wc JOIN work w ON w.id = wc.work_id
		 WHERE w.title LIKE 'Berserk%'`); n != 1 {
		t.Errorf("the shared Berserk work has %d work_comic rows, want exactly 1", n)
	}

	// DEGRADED IDENTITY IS THE ORDINARY CASE. Three of the four works carry no
	// external id at all, and all four are still filed and still indexed.
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM work w
		 WHERE NOT EXISTS (SELECT 1 FROM external_id e WHERE e.work_id = w.id)`); n != 3 {
		t.Errorf("unidentified works = %d, want 3 — a work with no resolvable identity is KEPT", n)
	}

	// ── §7 invariants 2 and 5, over a populated corpus ──────────────────────
	docs := countIn(t, env, `SELECT COUNT(*) FROM search_doc`)
	fts := countIn(t, env, `SELECT COUNT(*) FROM search_fts`)
	trgm := countIn(t, env, `SELECT COUNT(*) FROM search_trgm`)
	if docs != 4 {
		t.Errorf("search_doc rows = %d, want 4", docs)
	}
	if docs != fts || docs != trgm {
		t.Errorf("§7 invariant 2 broken through the wiring: doc %d / fts %d / trgm %d", docs, fts, trgm)
	}
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM search_doc d
		 WHERE NOT EXISTS (SELECT 1 FROM search_doc_library l WHERE l.doc_rowid = d.rowid)`); n != 0 {
		t.Errorf("§7 invariant 5 broken through the wiring: %d docs have no library scope", n)
	}
	// The shared work is scoped to BOTH of its libraries, which is what makes it
	// findable from either — five memberships over four docs.
	if n := countIn(t, env, `SELECT COUNT(*) FROM search_doc_library`); n != 5 {
		t.Errorf("search_doc_library rows = %d, want 5", n)
	}

	// The document is searchable through search_doc's rowid, which is the only
	// thing that proves the FTS rowid was written explicitly.
	var title string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT w.title FROM search_fts f
		  JOIN search_doc d ON d.rowid = f.rowid
		  JOIN work w ON w.id = d.work_id
		 WHERE search_fts MATCH 'Hobbit'`).Scan(&title); err != nil {
		t.Fatalf("search the imported corpus: %v", err)
	}
	if title != "The Hobbit" {
		t.Errorf("search_fts MATCH 'Hobbit' resolved to %q", title)
	}

	// ANALYZE ran after the bulk import.
	if n := countIn(t, env, `SELECT COUNT(*) FROM sqlite_schema WHERE name = 'sqlite_stat1'`); n != 1 {
		t.Error("no sqlite_stat1: ANALYZE did not run after the import")
	}

	// ── progress reached the browser ────────────────────────────────────────
	//
	// §7.2 asks for progress over SSE with REAL COUNTS. The terminal frame is
	// what a client renders as "done", so it is the one asserted.
	deadline := time.After(10 * time.Second)
	var done httpapi.Event
	var lastApplied int
	for done.Name == "" {
		select {
		case ev := <-stream.events:
			if ev.name != httpapi.EventImportProgress {
				continue
			}
			var p struct {
				InstanceID int64  `json:"instance_id"`
				Phase      string `json:"phase"`
				ItemsRead  int    `json:"items_read"`
				Applied    int    `json:"applied"`
			}
			if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
				t.Fatalf("decode %s frame %q: %v", ev.name, ev.data, err)
			}
			if p.InstanceID != created.ID {
				t.Errorf("progress frame names instance %d, want %d", p.InstanceID, created.ID)
			}
			lastApplied = p.Applied
			if p.Phase == "done" {
				done.Name = ev.name
				if p.ItemsRead != 5 || p.Applied != 5 {
					t.Errorf("terminal frame = read %d applied %d, want 5 and 5 — §7.2 asks for "+
						"REAL counts, and a zero here is a progress bar that lies", p.ItemsRead, p.Applied)
				}
			}
		case <-deadline:
			t.Fatalf("no terminal import.progress frame arrived (last applied = %d). Stream:\n%s",
				lastApplied, stream.dump())
		}
	}

	// Nothing on the stream may carry the credential or a host filesystem path.
	assertNoSecret(t, "SSE stream", stream.dump(), importAuthKey, "/mnt/user/media")
	assertNoSecret(t, "process log", env.logs(), importAuthKey)
}

func TestTheBootstrapImportRunsOnceAndThenStopsAskingAgain(t *testing.T) {
	// last_full_sync_at is the DURABLE gate, and it is the one that matters: the
	// in-process map is lost on restart, so if this gate did not hold, every
	// restart would re-import the whole library.
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = []map[string]any{kavitaSeries(41, 1, "Frieren", nil)}

	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)
	armBootstrapImport(t, env)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "kavita", "name": "Kavita", "base_url": kav.URL(), "api_key": importAuthKey,
	}, &created)
	waitForImport(t, env, created.ID)

	countCalls := func() int {
		_, _, paths := kav.seen()
		n := 0
		for _, p := range paths {
			if p == "/api/Series/all-v2" {
				n++
			}
		}
		return n
	}
	before := countCalls()
	if before != 1 {
		t.Fatalf("the series list was read %d times during one import, want 1", before)
	}

	// A fresh process would come up, connect, and reach exactly this call. It
	// must decline, because the instance has a completed full sync.
	env.app.registry.bootstrapImport(t.Context(), created.ID)
	if after := countCalls(); after != before {
		t.Errorf("the series list was read %d times after a second connect, want %d — "+
			"last_full_sync_at did not gate the bootstrap, so every restart re-imports "+
			"the whole library", after, before)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM work`); n != 1 {
		t.Errorf("work rows = %d, want 1", n)
	}
}

func TestAFullImportOfANonCatalogueServiceIsRefusedByName(t *testing.T) {
	// v0.1 imports a catalogue from Kavita and from nothing else (ADR-0041). A
	// Prowlarr must be refused with a message that names the kind, not with a
	// nil-client panic three frames down.
	prow := newFakeProwlarr(t, importProwlarrKey)
	env := newTestApp(t)
	var sess sessionBody
	env.do(t, "GET", "/api/v1/auth/session", nil, &sess)
	env.do(t, "POST", "/api/v1/auth/setup",
		map[string]any{"username": "joe", "password": "correct horse battery"}, &sess)

	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "prowlarr", "name": "Prowlarr", "base_url": prow.URL(),
		"api_key": importProwlarrKey,
	}, &created)

	_, err := env.app.registry.FullImport(t.Context(), created.ID)
	if err == nil {
		t.Fatal("a full import of a Prowlarr was accepted")
	}
	if !strings.Contains(err.Error(), "prowlarr") || !strings.Contains(err.Error(), "kavita") {
		t.Errorf("the refusal must name both the kind it got and the kind it wants: %v", err)
	}
}
