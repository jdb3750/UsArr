package libsync

import (
	"testing"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// The two source strings this pass retired. They are named rather than spelled
// inline at each assertion so that a regression to either fails on a symbol a
// reader can follow back to the reason.
const (
	// metronBareSource is what kavitaExternalIDs shipped with. Retired because
	// Metron has TWO numeric spaces — Issue and Series are separate Django
	// models with separate auto primary keys — and UsArr is one phase-B chapter
	// walk away from holding both.
	metronBareSource = "metron"

	// cbrBareSource is what kavitaExternalIDs shipped with. Retired because in
	// UsArr's OWN schema `cbr` already names the Comic Book RAR archive format
	// (an edition.format CHECK value in 00005_library_sync.sql), so the token
	// meant two unrelated things in one database. ADR-0046 open question 2.
	cbrBareSource = "cbr"
)

// TestEditableIDsAreNeverWorkStrong is the UNIT half of the LS-70/LS-71 rule:
// nothing metronId or cbrId can carry may leave kavitaExternalIDs at 1.0.
//
// It drives kavitaExternalIDs rather than editableIdentity on purpose, for
// TestWeblinkParsedIDsAreNeverWorkStrong's reason: the bug is not "the constant
// is wrong", it is "a field is wired to `add`, which hard-codes 1.0", which is
// exactly how both of these came to be at 1.0. A test of editableIdentity alone
// would pass while that was true.
func TestEditableIDsAreNeverWorkStrong(t *testing.T) {
	// Both library regimes, even though the inherit flag cannot reach either
	// field: it drives ProcessSeries.cs:358-367, and neither metronId nor cbrId
	// is in that block at either ref. Asserting the flag is IRRELEVANT here is
	// the point — a future edit that borrows comicVineIdentity's refusal must
	// fail rather than silently drop these.
	for _, inherit := range []bool{false, true} {
		d := kindDecision{Kind: "comic", ReadingDirection: "ltr", InheritsWebLinks: inherit}
		ids := kavitaExternalIDs(kavita.SeriesDto{MetronID: 156409, CbrID: 7}, d)

		got := map[string]store.ExternalIdentifier{}
		for _, x := range ids {
			got[x.Source] = x
		}
		if len(got) != 2 {
			t.Fatalf("inherit=%v: mapped %d identifiers, want 2: %+v", inherit, len(got), ids)
		}
		for _, want := range []struct{ source, value string }{
			{MetronSeriesSource, "156409"},
			{ComicBookRoundupSource, "7"},
		} {
			x, ok := got[want.source]
			if !ok {
				t.Errorf("inherit=%v: no %s identifier at all; %+v", inherit, want.source, ids)
				continue
			}
			if x.Value != want.value {
				t.Errorf("inherit=%v: %s = %q, want %q", inherit, want.source, x.Value, want.value)
			}
			if x.Confidence >= 1.0 {
				t.Errorf("inherit=%v: %s=%s came out at confidence %v. Kavita's Edit Series dialog "+
					"writes that field and SeriesDto carries no provenance to say so, so §6.4 "+
					"amendment 3 forbids 1.0 — at 1.0 it satisfies ux_extid_work_strong and MERGES "+
					"WORKS, and v0.1 has no work_merge table to undo that",
					inherit, x.Source, x.Value, x.Confidence)
			}
			if x.Confidence != EditableIDConfidence {
				t.Errorf("inherit=%v: %s confidence %v, want EditableIDConfidence (%v)",
					inherit, x.Source, x.Confidence, EditableIDConfidence)
			}
		}
		for _, bare := range []string{metronBareSource, cbrBareSource} {
			if _, ok := got[bare]; ok {
				t.Errorf("inherit=%v: an id was written under the BARE source %q", inherit, bare)
			}
		}
	}
}

// TestEditableIDsAreAbsentRatherThanZero: the ordinary untagged case. §6.4's
// "not identified" state is the ABSENCE of an external_id row, so a zero field
// must produce no row at all rather than a row reading "0" — which would then
// collide with every other unidentified series on (source, value).
//
// ⚠️ FOR cbrId THIS IS ALSO THE OWNER'S EVERYDAY CASE, not an edge one. The
// property is absent from SeriesDto at v0.9.0.2 (ADR-0046's floor), so the JSON
// carries no `cbrId` key at all and Go's zero value is what the decoder leaves
// behind. The line is reachable only on develop and later.
func TestEditableIDsAreAbsentRatherThanZero(t *testing.T) {
	ids := kavitaExternalIDs(kavita.SeriesDto{}, kindDecision{Kind: "comic"})
	if len(ids) != 0 {
		t.Errorf("an untagged series produced %+v; want no identifiers at all", ids)
	}
}

// TestHardcoverIsTheONLYRemainingStrongKavitaSource pins the decision that
// survived this pass, so that it stays a decision.
//
// After LS-70/LS-71 exactly one Kavita field still reaches kavitaExternalIDs's
// `add` closure and its hard-coded 1.0, and it is there because §6.4 amendment 4
// NAMES it — "work-strong book sources are openlibrary_work, hardcover_book and
// goodreads_work, and nothing else" — not because its provenance is better. On
// provenance alone it is the WEAKEST of the six: ExternalMetadataIdHelper.cs:28
// at v0.9.0.2 is its only writer, with neither a scanner nor a provider one.
//
// So this test is deliberately two-sided. A new field wired to `add` fails it,
// and so does hardcover_book quietly losing its 1.0 — because that would mean
// the adapter had overruled §6.4 rather than §6.4 having been amended.
func TestHardcoverIsTheONLYRemainingStrongKavitaSource(t *testing.T) {
	ids := kavitaExternalIDs(kavita.SeriesDto{
		AniListID: 30002, MalID: 2, MangaBakaID: 3391,
		HardcoverID: 445, MetronID: 156409, CbrID: 7,
		ComicVineID: "42563",
	}, kindDecision{Kind: "comic", ReadingDirection: "ltr"})

	strong := map[string]string{}
	for _, x := range ids {
		if x.Confidence >= 1.0 {
			strong[x.Source] = x.Value
		}
	}
	if len(strong) != 1 || strong["hardcover_book"] != "445" {
		t.Errorf("the sources written at confidence >= 1.0 are %v; want exactly "+
			"{hardcover_book:445}. A source at 1.0 satisfies ux_extid_work_strong and can MERGE "+
			"TWO WORKS, and §6.4 amendment 4 names only openlibrary_work, hardcover_book and "+
			"goodreads_work as work-strong — so a new name here is an adapter overruling §6.4, "+
			"and a missing hardcover_book is the same thing in the other direction", strong)
	}
}

// TestEditableIDEndToEndAgainstTheDatabase runs the WHOLE channel-1 path —
// cassettes, adapter, batching, real migrated SQLite — and then asks the
// database what it holds, in ux_extid_work_strong's own terms.
//
// It is the only assertion in this file that can catch a store-side write which
// re-strengthens what the mapping weakened, and the only one that can prove the
// CONSEQUENCE rather than the number: series 401/402 share a metronId and
// 403/404 share a cbrId, so if either cap comes off those pairs do not merely
// gain a strong row, they COLLAPSE INTO ONE WORK. Row counts are not asked for
// anywhere below that matters — a row count still adds up after a merge, which
// is why the pair assertions count DISTINCT work_id.
func TestEditableIDEndToEndAgainstTheDatabase(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := NewKavitaSource(newCassetteClient(t,
		"kavita_libraries_editable_ids.yaml", "kavita_series_all_v2_editable_ids.yaml",
		"kavita_series_metadata.yaml", "kavita_series_volumes.yaml"))

	rep, err := newImporter(t, s, src).FullImport(t.Context(), inst)
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if !rep.Completed {
		t.Fatalf("import did not complete: %+v", rep)
	}
	if rep.ItemsApplied != 5 {
		t.Fatalf("applied %d items, want 5", rep.ItemsApplied)
	}

	// Read back THROUGH THE LINK, so the assertion is about "what this series
	// claims" rather than about a bare row count — and so a merge shows up as
	// two remote ids sharing a work rather than as a count that still adds up.
	rows, err := s.DB().Read().QueryContext(t.Context(), `
		SELECT l.remote_id, e.source, e.value, e.confidence
		  FROM external_id e
		  JOIN service_item_link l ON l.work_id = e.work_id
		 WHERE l.service_instance_id = ?
		 ORDER BY l.remote_id, e.source`, inst)
	if err != nil {
		t.Fatalf("query external ids: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("closing rows: %v", err)
		}
	}()

	type claim struct {
		source     string
		value      string
		confidence float64
	}
	got := map[string][]claim{}
	for rows.Next() {
		var remote string
		var c claim
		if err := rows.Scan(&remote, &c.source, &c.value, &c.confidence); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[remote] = append(got[remote], c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	want := map[string][]claim{
		// 401/402 — Kavita's own doc-comment issue id, on two series. It is
		// series-level by construction (nothing on any scan path writes
		// Series.MetronId) and hand-typed, which is amendment 3, not 4.
		"401": {{MetronSeriesSource, "156409", EditableIDConfidence}},
		"402": {{MetronSeriesSource, "156409", EditableIDConfidence}},
		// 403/404 — ComicBookRoundup, which DOES have a licence-gated provider
		// writer on develop and is capped anyway: SeriesDto carries no
		// provenance, and develop's ungated Edit-dialog helper overwrites the
		// match on save.
		"403": {{ComicBookRoundupSource, "7", EditableIDConfidence}},
		"404": {{ComicBookRoundupSource, "7", EditableIDConfidence}},
		// 405 — the untagged control. Nothing, and that is not an error.
		"405": nil,
	}
	for remote, w := range want {
		g := got[remote]
		if len(g) != len(w) {
			t.Errorf("series %s holds %v, want %v", remote, g, w)
			continue
		}
		for i := range g {
			if g[i] != w[i] {
				t.Errorf("series %s claim %d = %+v, want %+v", remote, i, g[i], w[i])
			}
		}
	}

	// ── The structural assertion, asked of SQLite in ux_extid_work_strong's own
	// terms rather than in the mapping's. A single row here means one of these
	// two can merge two works.
	strong := countRows(t, s, `
		SELECT count(*) FROM external_id
		 WHERE source IN ('metron_series', 'comicbookroundup')
		   AND work_id IS NOT NULL AND confidence >= 1.0`)
	if strong != 0 {
		t.Errorf("%d hand-editable rows satisfy ux_extid_work_strong's predicate "+
			"(work_id IS NOT NULL AND confidence >= 1.0); want 0", strong)
	}

	// ── THE CONSEQUENCE, not the number, once per capped field. DISTINCT
	// work_id and not count(*): after a merge the link rows still number two.
	for _, pair := range []struct {
		a, b, field, why string
	}{
		{"401", "402", "metronId 156409", "a Metron id is hand-typed into an untyped " +
			"\"Metron Id\" box, and Metron numbers issues and series separately"},
		{"403", "404", "cbrId 7", "a ComicBookRoundup id is indistinguishable on the DTO " +
			"from one the Edit Series dialog typed over the Kavita+ match"},
	} {
		var distinct int
		if err := s.DB().Read().QueryRowContext(t.Context(), `
			SELECT count(DISTINCT work_id) FROM service_item_link
			 WHERE service_instance_id = ? AND remote_id IN (?, ?)`,
			inst, pair.a, pair.b).Scan(&distinct); err != nil {
			t.Fatalf("count distinct works for %s/%s: %v", pair.a, pair.b, err)
		}
		if distinct != 2 {
			t.Errorf("series %s and %s resolved to %d distinct works, want 2 — they share %s, "+
				"and a strong write MERGED THEM: %s", pair.a, pair.b, distinct, pair.field, pair.why)
		}
	}

	// ── The bare namespaces are gone from the database, not merely from the
	// mapping, so a regression to either has to fail here.
	for _, bare := range []string{metronBareSource, cbrBareSource} {
		if n := countRows(t, s,
			`SELECT count(*) FROM external_id WHERE source = '`+bare+`'`); n != 0 {
			t.Errorf("%d rows carry the bare source %q; %q would collide with Metron's separate "+
				"ISSUE number space, and %q already names the Comic Book RAR archive format in "+
				"edition.format's CHECK", n, bare, metronBareSource, cbrBareSource)
		}
	}

	// All five works exist and are searchable regardless: weakening an identity
	// claim degrades identity, it never drops the work.
	if n := countRows(t, s, `SELECT count(*) FROM work`); n != 5 {
		t.Errorf("%d work rows, want 5 — weakening an identity claim must not drop the work", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM search_doc`); n != 5 {
		t.Errorf("%d search_doc rows, want 5", n)
	}
}
