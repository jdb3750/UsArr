package store

import (
	"database/sql"
	"strings"
	"testing"
)

// The two-level import seam — ADR-0068, and ADR-0066 decision 5 which it
// activates.
//
// ⚠️ THE ONE THING THESE TESTS EXIST TO CATCH is not a wrong number, it is a
// wrong SHAPE: a `comic` work per comic file passes every count assertion an
// import can make about itself and is the alternative ADR-0066 refused
// pre-emptively, because §6.4's cascade makes a wrong work.kind at ingest
// permanently unmergeable. Every parity assertion below is stated as the failure
// condition rather than as a number to look at, for that reason.

// issue builds one comic_issue under a named series.
func issue(remoteID, container, title, seriesRef, seriesTitle string) CatalogueItem {
	it := item(remoteID, container, "comic_issue", title)
	it.RemoteKind = "book"
	it.Parent = &CatalogueParent{
		RemoteKind:      "series",
		RemoteID:        seriesRef,
		Kind:            "comic",
		Title:           seriesTitle,
		SortTitle:       seriesTitle,
		NormalizedTitle: strings.ToLower(seriesTitle),
		NormVersion:     1,
	}
	return it
}

// TestIssuesAreMintedUnderOneSeriesAndTheSeriesIsNotPerRow.
func TestIssuesAreMintedUnderOneSeriesAndTheSeriesIsNotPerRow(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "bookorbit")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		{RemoteID: "1", Name: "Shelf", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

	items := []CatalogueItem{
		issue("101", "1", "Saga #1", "5", "Saga"),
		issue("102", "1", "Saga #2", "5", "Saga"),
		issue("103", "1", "Saga #3", "5", "Saga"),
		item("201", "1", "book", "The Hobbit"),
	}
	items[0].IsOneshot = false
	items[0].NumberSort = sql.NullFloat64{Float64: 1, Valid: true}

	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, items, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	issues := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'comic_issue'`)
	series := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'comic'`)
	if issues != 3 {
		t.Fatalf("comic_issue works = %d, want 3", issues)
	}
	// ⚠️ ADR-0068's SECOND DONE-CHECK, STATED AS A FAILURE CONDITION. "If the
	// series count EQUALS the issue count, the per-row implementation shipped and
	// this check MUST FAIL … it is the only outcome here that is worse than not
	// shipping."
	if series == issues {
		t.Fatalf("series works = issue works = %d — that is one 'comic' work per comic FILE, "+
			"the alternative ADR-0066 pre-emptively refused; §6.4's cascade makes it "+
			"permanently unmergeable", series)
	}
	if series != 1 {
		t.Errorf("comic works = %d, want 1 — three issues of one series", series)
	}
	// Every issue resolves to a parent. Zero rows with parent_work_id IS NULL is
	// the ADR's first done-check.
	if n := count(t, s,
		`SELECT COUNT(*) FROM work WHERE kind = 'comic_issue' AND parent_work_id IS NULL`); n != 0 {
		t.Errorf("parentless issues = %d, want 0", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work c
	                       JOIN work p ON p.id = c.parent_work_id
	                      WHERE c.kind = 'comic_issue' AND p.kind = 'comic'`); n != 3 {
		t.Errorf("issues resolving to a 'comic' parent = %d, want 3", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_comic_issue`); n != 3 {
		t.Errorf("work_comic_issue rows = %d, want 3 — the subtype row is what makes the "+
			"kind concrete", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work_comic`); n != 1 {
		t.Errorf("work_comic rows = %d, want 1", n)
	}

	// ── THE CORPUS AND THE GRID ─────────────────────────────────────────────
	//
	// The issue is in NEITHER. writeSearchDoc returns an ERROR on this kind
	// rather than skipping it, so a build that routed an issue through the
	// top-level path would have failed the call above; this pins the other half —
	// that the series DOES get a document, so the screen is not merely empty in a
	// different way.
	if n := count(t, s, `SELECT COUNT(*) FROM search_doc WHERE kind = 'comic_issue'`); n != 0 {
		t.Errorf("comic_issue search_doc rows = %d, want 0", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM search_doc WHERE kind = 'comic'`); n != 1 {
		t.Errorf("comic search_doc rows = %d, want 1 — the SERIES is what the corpus sees", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member lm JOIN work w ON w.id = lm.work_id
	                      WHERE w.kind = 'comic_issue'`); n != 0 {
		t.Errorf("comic_issue library_member rows = %d, want 0 — §17.8's item count and "+
			"/library/comics must not mean two different things", n)
	}

	// ── ADR-0066 DECISION 5 ─────────────────────────────────────────────────
	//
	// One mixed container, two libraries, ONE container ref.
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE id <> 0`); n != 2 {
		t.Errorf("libraries = %d, want 2 — a 'book' library and a 'comic' library", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_source ls JOIN library l ON l.id = ls.library_id
	                      WHERE ls.container_ref = '1' AND l.id <> 0`); n != 2 {
		t.Errorf("library_source rows over container '1' = %d, want 2 — decision 5's two "+
			"libraries stand over the SAME container ref, which needs no migration because "+
			"library_source's uniqueness carries library_id", n)
	}
	// NO SERIES WORK IS EVER MINTED INTO NO LIBRARY AT ALL.
	if n := count(t, s, `SELECT COUNT(*) FROM work w WHERE w.kind = 'comic'
	                       AND NOT EXISTS (SELECT 1 FROM library_member lm WHERE lm.work_id = w.id)`); n != 0 {
		t.Errorf("unfiled comic series = %d, want 0", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member lm
	                       JOIN work w ON w.id = lm.work_id
	                       JOIN library l ON l.id = lm.library_id
	                      WHERE w.kind = 'comic' AND l.kind = 'comic'`); n != 1 {
		t.Errorf("comic series filed into a 'comic' library = %d, want 1", n)
	}

	// The series is linked under its own remote_kind, so a BookOrbit book id and
	// a BookOrbit series id cannot collide in ux_sil.
	if n := count(t, s,
		`SELECT COUNT(*) FROM service_item_link WHERE remote_kind = 'series' AND remote_id = '5'`); n != 1 {
		t.Errorf("series links = %d, want exactly 1 — the parent is RESOLVED per issue, not "+
			"re-minted", n)
	}
}

// TestReimportingIssuesDoesNotMintASecondSeries is the idempotence half, and it
// is what a synthesized one-shot depends on entirely: a non-deterministic
// synthesized ref would double /library/comics on every import.
func TestReimportingIssuesDoesNotMintASecondSeries(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "bookorbit")
	oneshot := issue("101", "1", "Endless Nights", "oneshot:101", "Endless Nights")
	oneshot.Parent.Synthesized = true
	oneshot.IsOneshot = true

	for range 3 {
		// REBOUND EVERY ROUND, exactly as a second import does. The bind pass is
		// where the kind-aware library_source lookup earns its keep: without it
		// the prose container's second bind can match the COMIC library instead.
		binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
			{RemoteID: "1", Name: "Shelf", Kind: "book"},
		})
		if err != nil {
			t.Fatalf("BindContainers: %v", err)
		}
		if b := binds["1"]; b.Kind != "book" {
			t.Fatalf("the prose container rebound to kind %q; one container ref names two "+
				"libraries now, so the lookup must be kind-aware", b.Kind)
		}
		if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds,
			[]CatalogueItem{oneshot}, testNow); err != nil {
			t.Fatalf("ApplyCatalogueBatch: %v", err)
		}
	}

	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'comic'`); n != 1 {
		t.Errorf("comic works after three imports = %d, want 1", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'comic_issue'`); n != 1 {
		t.Errorf("comic_issue works after three imports = %d, want 1", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE kind = 'comic' AND id <> 0`); n != 1 {
		t.Errorf("comic libraries after three imports = %d, want 1", n)
	}

	// ⚠️ is_oneshot IS READ BACK, because ADR-0068 decision 2's whole point is
	// that the column has a WRITER: "a column with a DEFAULT 0 and no writer is a
	// deaf column". A test that only asserted the row exists would pass against
	// the default.
	var flag int
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT wci.is_oneshot FROM work_comic_issue wci
		   JOIN work w ON w.id = wci.work_id WHERE w.kind = 'comic_issue'`).Scan(&flag); err != nil {
		t.Fatalf("read is_oneshot: %v", err)
	}
	if flag != 1 {
		t.Errorf("is_oneshot = %d, want 1", flag)
	}
}

// TestTheCorpusGuardStillRefusesAComicIssue.
//
// ⚠️ THE GUARD IS NOT RELAXED BY THIS SLICE, and that is a consequence ADR-0068
// states in terms: "writeSearchDoc's guard gets its first real caller and must
// keep refusing. The implementing slice routes comic_issue rows AROUND the
// top-level item path; it does not relax corpusExcludedKinds." An import that
// wrote parentless issues through that path would not quietly render zero — it
// would FAIL, which is why orphan issues were refused.
func TestTheCorpusGuardStillRefusesAComicIssue(t *testing.T) {
	if !corpusExcludedKinds["comic_issue"] {
		t.Fatal("corpusExcludedKinds no longer excludes comic_issue; ADR-0030's corpus " +
			"exclusion is what makes the parent rule necessary, and relaxing it here " +
			"would swamp the corpus with ~90,000 rows (ARCHITECTURE §13)")
	}
	if !childKinds["comic_issue"] {
		t.Fatal("childKinds no longer holds comic_issue; the item pass would then file it " +
			"into a library and hand it to writeSearchDoc, which errors")
	}
	// The two lists are DIFFERENT questions and must not be folded into one.
	if childKinds["person"] {
		t.Error("childKinds holds 'person'; a person is excluded from the corpus for a " +
			"DESTINATION reason and is not a child of anything")
	}
}

// TestOneSeriesSpanningTwoContainersIsFiledInBoth.
//
// ⚠️ THE PREMISE IS ADR-0068'S OWN, NOT A HYPOTHETICAL: "A BookOrbit series is
// NOT library-scoped upstream, so the container question is answered on UsArr's
// side". One series can therefore carry issues walked from two containers, and
// the parent must reach BOTH of the comic libraries those containers mint.
//
// It is the assertion that keeps applyOneItem's per-batch parent cache honest: a
// cache keyed on the upstream series id alone would write the parent once — for
// whichever container came first — and the second container's comic library
// would silently never gain the series, with no error anywhere.
func TestOneSeriesSpanningTwoContainersIsFiledInBoth(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s, "bookorbit")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		{RemoteID: "1", Name: "Shelf One", Kind: "book"},
		{RemoteID: "2", Name: "Shelf Two", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}

	// ONE BATCH, on purpose: across two batches the cache is empty the second
	// time and the defect cannot appear. The batch is where it lives.
	if _, err := s.ApplyCatalogueBatch(t.Context(), inst, binds, []CatalogueItem{
		issue("101", "1", "Saga #1", "5", "Saga"),
		issue("102", "2", "Saga #2", "5", "Saga"),
	}, testNow); err != nil {
		t.Fatalf("ApplyCatalogueBatch: %v", err)
	}

	// STILL ONE series work: two containers do not make two series.
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE kind = 'comic'`); n != 1 {
		t.Errorf("comic works = %d, want 1 — the same series id is the same series", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE kind = 'comic' AND id <> 0`); n != 2 {
		t.Errorf("comic libraries = %d, want 2 — one per mixed container", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library_member lm
	                       JOIN work w ON w.id = lm.work_id
	                       JOIN library l ON l.id = lm.library_id
	                      WHERE w.kind = 'comic' AND l.kind = 'comic'`); n != 2 {
		t.Errorf("the series is filed into %d comic libraries, want 2 — a parent cache keyed "+
			"on the series id alone writes it once and leaves the second library empty", n)
	}
}
