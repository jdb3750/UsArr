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
		// DEGRADED IDENTITY IS THE DEFAULT HERE because it is the ordinary case
		// for an UNTAGGED library. ⚠️ Not because "a free Kavita writes none of
		// the Kavita+ identifier fields", which is what this said and is false:
		// LS-30 found that four of these six have plain-scanner writers reading
		// ComicInfo.xml's <Web> element, so a ComicTagger-tagged free instance
		// fills them in.
		"aniListId": 0, "malId": 0, "hardcoverId": 0, "metronId": 0,
		"comicVineId": "", "mangaBakaId": 0, "cbrId": 0,
	}
	for k, v := range extra {
		s[k] = v
	}
	return s
}

// kavitaMetadata builds a SeriesMetadataDto body with the named person arrays
// populated. Every array Kavita declares is emitted, empty where unnamed, which
// is the shape a real response has — an adapter that only works when an array is
// absent would pass a fixture that omitted them.
func kavitaMetadata(seriesID int, people map[string][]string) map[string]any {
	out := map[string]any{"id": seriesID, "seriesId": seriesID, "summary": ""}
	for _, array := range []string{
		"writers", "coverArtists", "publishers", "characters", "pencillers",
		"inkers", "imprints", "colorists", "letterers", "editors", "translators",
		"teams", "locations",
	} {
		dtos := []map[string]any{}
		for i, name := range people[array] {
			dtos = append(dtos, map[string]any{"id": i + 1, "name": name})
		}
		out[array] = dtos
	}
	return out
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

func countIn(t *testing.T, env *testEnv, q string, args ...any) int {
	t.Helper()
	var n int
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), q, args...).Scan(&n); err != nil {
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
		// AniList id. ⚠️ They must NOT resolve onto one work: this said "§6.4
		// tier 1 must resolve them onto one work", on the premise that aniListId
		// is matcher-written. LS-30 measured otherwise, so §6.4 amendment 3 caps
		// it at 0.90 and tier 1 skips it. The pair is kept precisely to prove
		// that — a deluxe re-release must not swallow the original.
		kavitaSeries(44, 1, "Berserk", map[string]any{"aniListId": 30013}),
		kavitaSeries(45, 2, "Berserk (Deluxe)", map[string]any{"aniListId": 30013}),
	}
	// Credits (ADR-0044). Kentaro Miura writes AND draws both Berserk rows, and
	// the two rows resolve onto ONE work — so the same person must not be
	// created twice, and the two remote items' credit sets must not fight. The
	// Hobbit is in a BOOK library, so its `writers` array becomes an 'author'
	// and not a 'writer'.
	kav.metadata = map[int]map[string]any{
		41: kavitaMetadata(41, map[string][]string{
			"writers": {"Kanehito Yamada"}, "coverArtists": {"Tsukasa Abe"},
			// Not a creator: a publisher is an organisation, and UsArr credits
			// none of Kavita's five non-creator arrays.
			"publishers": {"Shogakukan"}, "characters": {"Frieren"},
		}),
		43: kavitaMetadata(43, map[string][]string{"writers": {"J. R. R. Tolkien"}}),
		44: kavitaMetadata(44, map[string][]string{
			"writers": {"Kentaro Miura"}, "pencillers": {"Kentaro Miura"},
		}),
		45: kavitaMetadata(45, map[string][]string{"writers": {"Kentaro Miura"}}),
		// 42 (Saga) deliberately has NO entry: the fake answers an empty
		// metadata object, which is what a real Kavita returns for a series
		// nobody has tagged.
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

	// Five series, FIVE CATALOGUE works.
	//
	// ⚠️ THIS ASSERTED FOUR, AND THE FOURTH WAS A MERGE THAT SHOULD NEVER HAVE
	// HAPPENED. It read: "Five series, FOUR CATALOGUE works: the two Berserk rows
	// share an AniList id and tier 1 resolves them onto one work across two
	// libraries." They do share `aniListId: 30013` — but LS-30 measured
	// Series.AniListId at Kavita v0.9.0.2 and found a plain-scanner writer
	// (ProcessSeries.cs:363) and an Edit-Series-dialog writer
	// (ExternalMetadataIdHelper.cs:13) beside the Kavita+ one, so §6.4 amendment
	// 3 caps it at 0.90, tier 1 skips anything below 1.0, and A DELUXE
	// RE-RELEASE NO LONGER SWALLOWS THE ORIGINAL. v0.1 has no work_merge table
	// and no un-merge, which is why that mattered.
	//
	// ⚠️ THE PREDICATE EXCLUDES 'person' AND THAT IS ADR-0044's COST, STATED.
	// `work` is no longer a table of catalogue items alone: a credited author is
	// a work of kind 'person' (ADR-0033), so `SELECT COUNT(*) FROM work` counts
	// people too. The people are asserted separately below rather than swept
	// into this number.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind <> 'person'`); n != 5 {
		t.Errorf("catalogue work rows = %d, want 5 — the two Berserk series share an AniList id "+
			"parsed out of free text, and merging on one is unrecoverable in v0.1", n)
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
	// ⚠️ THE TWO BERSERK ROWS EACH KEEP THEIR OWN reading_direction, and LS-07's
	// last-writer-wins case IS NO LONGER EXERCISED BY THIS FIXTURE. Said plainly
	// rather than left as a passing number, because losing coverage quietly is
	// worse than losing it.
	//
	// This assertion read `!= 1`, with the note: "THE SHARED WORK'S
	// reading_direction IS LAST-WRITER-WINS … one work now sits in a Manga library
	// (rtl) and a Comic library (ltr), and work_comic holds exactly one value".
	// That was true only while the two rows merged on a 0.90-capped AniList id,
	// which LS-30 stopped. The FINDING stands — two remote items that resolve onto
	// one work still overwrite each other's reading_direction, and it still lands
	// in a column §6.5 rule 4 makes editable — but reaching it now needs a
	// genuinely work-strong shared id (`hardcover_book`, `openlibrary_work`,
	// `goodreads_work`), which this fixture does not carry. A fixture that
	// exercises it deliberately belongs to whoever revisits LS-07 or LS-35.
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM work_comic wc JOIN work w ON w.id = wc.work_id
		 WHERE w.title LIKE 'Berserk%'`); n != 2 {
		t.Errorf("the two Berserk works have %d work_comic rows between them, want 2 — one each, "+
			"because a free-text-derived AniList id no longer merges them", n)
	}
	// And they are genuinely two, with the two DIFFERENT reading directions their
	// two libraries imply — which is the positive form of the same fact.
	if n := countIn(t, env, `
		SELECT COUNT(DISTINCT wc.reading_direction) FROM work_comic wc
		  JOIN work w ON w.id = wc.work_id WHERE w.title LIKE 'Berserk%'`); n != 2 {
		t.Errorf("the two Berserk works share %d distinct reading_directions, want 2 (rtl from the "+
			"Manga library, ltr from the Comic one)", n)
	}

	// DEGRADED IDENTITY IS THE ORDINARY CASE. Three of the five works carry no
	// external id at all, and all five are still filed and still indexed. The
	// other two are the Berserk pair, which now hold ONE 0.90 `anilist` row each
	// instead of one shared 1.0 row — see LS-30.
	//
	// A PERSON CARRIES NO external_id EITHER, by design — see
	// store.personWorkID: Kavita's person id is instance-local, so writing it
	// would make two installs both claim person 5 and ux_extid_work_strong would
	// read that as a merge signal between unrelated humans. So people are
	// excluded here too, rather than inflating the count.
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM work w
		 WHERE w.kind <> 'person'
		   AND NOT EXISTS (SELECT 1 FROM external_id e WHERE e.work_id = w.id)`); n != 3 {
		t.Errorf("unidentified works = %d, want 3 — a work with no resolvable identity is KEPT", n)
	}

	// ── credits, through the real HTTP path (ADR-0044) ──────────────────────
	//
	// FOUR people, and this number is now a STRONGER assertion than it was.
	// Kentaro Miura is credited on BOTH Berserk rows, and since LS-30 those are
	// two separate works — so the person dedupe can no longer be carried by the
	// works having merged. He is one person work because store.personWorkID
	// resolves him to one, which is the thing worth testing.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work WHERE kind = 'person'`); n != 4 {
		t.Errorf("person works = %d, want 4 (Yamada, Abe, Tolkien, Miura). If this is 5 the "+
			"person dedupe did not survive one human being credited on two different works.", n)
	}

	// ⚠️ LS-19's LAST-WRITER-WINS CREDIT CASE IS ALSO NO LONGER EXERCISED HERE,
	// for the same reason as LS-07's above, and is recorded rather than quietly
	// dropped. The note read: "A WORK SHARED BY TWO REMOTE ITEMS GETS
	// LAST-WRITER-WINS CREDITS … remote 44 writes Miura as writer AND penciller,
	// and remote 45 — the same work, a different Kavita row with a thinner
	// metadata entry — then clears both and writes writer alone. FOUR credits,
	// not five: Frieren 2, The Hobbit 1, Berserk 1, Saga 0."
	//
	// Since LS-30 remotes 44 and 45 are two works, so nothing overwrites anything:
	// SIX credits — Frieren 2, The Hobbit 1, Berserk 2 (writer + penciller),
	// Berserk (Deluxe) 1 (writer), Saga 0.
	//
	// LS-19's FINDING STANDS UNCHANGED — ApplyCredits still replaces one WORK's
	// credits wholesale per REMOTE ITEM, and the alternative it rejected
	// (accumulating across remote items, which turns a wholesale replace into a
	// merge v0.1 does not have) is still rejected. What changed is only that this
	// fixture no longer produces a shared work to demonstrate it on. Whoever
	// revisits LS-19 needs a fixture with a genuinely work-strong shared id.
	if n := countIn(t, env, `SELECT COUNT(*) FROM work_credit`); n != 6 {
		t.Errorf("work_credit rows = %d, want 6 — see the note above; the Berserk pair no longer "+
			"share a work, so neither clears the other's credits", n)
	}
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM work_credit c JOIN work w ON w.id = c.work_id
		 WHERE w.title LIKE 'Berserk%'`); n != 3 {
		t.Errorf("the two Berserk works hold %d credits between them, want 3 — Miura as writer "+
			"and penciller on remote 44, and as writer alone on remote 45's thinner entry", n)
	}
	// The kind-dependent role: The Hobbit is in a BOOK library, so Kavita's
	// `writers` array becomes an 'author'. Frieren is in a MANGA library, so the
	// same array becomes a 'writer'.
	var hobbitRole string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT c.role FROM work_credit c
		  JOIN work w ON w.id = c.work_id
		  JOIN work p ON p.id = c.creator_work_id
		 WHERE w.title = 'The Hobbit' AND p.title = 'J. R. R. Tolkien'`).Scan(&hobbitRole); err != nil {
		t.Fatalf("The Hobbit has no Tolkien credit: %v", err)
	}
	if hobbitRole != "author" {
		t.Errorf("Tolkien is credited on The Hobbit as %q, want \"author\" — Kavita has no "+
			"`authors` array, so a book library's writers ARE its authors", hobbitRole)
	}
	var frierenRole string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(), `
		SELECT c.role FROM work_credit c
		  JOIN work w ON w.id = c.work_id
		  JOIN work p ON p.id = c.creator_work_id
		 WHERE w.title = 'Frieren' AND p.title = 'Kanehito Yamada'`).Scan(&frierenRole); err != nil {
		t.Fatalf("Frieren has no Yamada credit: %v", err)
	}
	if frierenRole != "writer" {
		t.Errorf("Yamada is credited on Frieren as %q, want \"writer\"", frierenRole)
	}
	// The five NON-creator arrays produced nothing. Shogakukan is a publisher
	// and Frieren is a character; neither is a credited human.
	for _, name := range []string{"Shogakukan", "Frieren"} {
		if n := countIn(t, env,
			`SELECT COUNT(*) FROM work WHERE kind = 'person' AND title = ?`, name); n != 0 {
			t.Errorf("%q became a person work. Publishers, imprints and teams are "+
				"ORGANISATIONS and characters and locations are not people at all.", name)
		}
	}
	// Saga has no metadata entry at all, so the fake answers an empty
	// SeriesMetadataDto — the ordinary case on a fresh Kavita. It gets no
	// credits and that is not an error.
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM work_credit c JOIN work w ON w.id = c.work_id
		 WHERE w.title = 'Saga'`); n != 0 {
		t.Errorf("Saga has %d credits from an empty metadata response", n)
	}
	// The application-enforced invariant migration 0007 cannot declare.
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM work_credit c JOIN work w ON w.id = c.creator_work_id
		 WHERE w.kind NOT IN ('artist','person')`); n != 0 {
		t.Errorf("%d credit(s) point at a work that is neither an artist nor a person", n)
	}
	// AND NO PERSON REACHED THE SEARCH CORPUS OR A LIBRARY (schema.md §6.1),
	// asserted here as well as in internal/store because this is the path a real
	// import takes.
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM search_doc d JOIN work w ON w.id = d.work_id
		 WHERE w.kind = 'person'`); n != 0 {
		t.Errorf("%d person work(s) reached the search corpus through the real import", n)
	}
	if n := countIn(t, env, `
		SELECT COUNT(*) FROM library_member m JOIN work w ON w.id = m.work_id
		 WHERE w.kind = 'person'`); n != 0 {
		t.Errorf("%d person work(s) were filed into a library through the real import", n)
	}

	// ── §7 invariants 2 and 5, over a populated corpus ──────────────────────
	docs := countIn(t, env, `SELECT COUNT(*) FROM search_doc`)
	fts := countIn(t, env, `SELECT COUNT(*) FROM search_fts`)
	trgm := countIn(t, env, `SELECT COUNT(*) FROM search_trgm`)
	// Five, not four: the Berserk pair are two works since LS-30, and a work that
	// exists is a work that is indexed.
	if docs != 5 {
		t.Errorf("search_doc rows = %d, want 5", docs)
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
