package libsync

import (
	"testing"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// malBareSource is the source string LS-10 shipped and LS-27 retired. It is
// named here rather than spelled inline at each assertion so that a regression
// to it fails on a symbol a reader can follow back to the reason.
const malBareSource = "mal"

// TestWeblinkParsedIDsAreNeverWorkStrong is the UNIT half of the LS-12 rule:
// nothing the three weblink-parsed fields can carry may leave kavitaExternalIDs
// at confidence 1.0.
//
// It drives kavitaExternalIDs rather than webLinkIdentity on purpose. The bug
// this guards is not "the constant is wrong" — it is "a field got wired to
// `add`, which hard-codes 1.0", which is exactly how these three came to be at
// 1.0 in the first place. A test of webLinkIdentity alone would pass while that
// was true.
func TestWeblinkParsedIDsAreNeverWorkStrong(t *testing.T) {
	// Both library regimes, because for these three fields the inherit flag is
	// NOT a refusal condition the way it is for ComicVine — it is the only
	// scanner path they have — and a future edit that borrows comicVineIdentity's
	// refusal must fail here rather than silently drop the owner's manga ids.
	for _, inherit := range []bool{false, true} {
		d := kindDecision{Kind: "comic", ReadingDirection: "rtl", InheritsWebLinks: inherit}
		ids := kavitaExternalIDs(kavita.SeriesDto{
			AniListID:   30002,
			MalID:       2,
			MangaBakaID: 3391,
		}, d)

		got := map[string]store.ExternalIdentifier{}
		for _, x := range ids {
			got[x.Source] = x
		}
		if len(got) != 3 {
			t.Fatalf("inherit=%v: mapped %d identifiers, want 3: %+v", inherit, len(got), ids)
		}
		for _, want := range []struct{ source, value string }{
			{AniListSource, "30002"},
			{MalMangaSource, "2"},
			{MangaBakaSource, "3391"},
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
				t.Errorf("inherit=%v: %s=%s came out at confidence %v. Kavita parses that field out "+
					"of a free-text <Web> element (ProcessSeries.cs:363-366 at v0.9.0.2), so §6.4 "+
					"amendment 3 forbids 1.0 — at 1.0 it satisfies ux_extid_work_strong and MERGES WORKS, "+
					"and v0.1 has no work_merge table to undo that",
					inherit, x.Source, x.Value, x.Confidence)
			}
			if x.Confidence != WebLinkConfidence {
				t.Errorf("inherit=%v: %s confidence %v, want WebLinkConfidence (%v)",
					inherit, x.Source, x.Confidence, WebLinkConfidence)
			}
		}
		if _, ok := got[malBareSource]; ok {
			t.Errorf("inherit=%v: a MyAnimeList id was written under the BARE source %q",
				inherit, malBareSource)
		}
	}
}

// TestWeblinkParsedIDsAreAbsentRatherThanZero: the ordinary untagged case.
// §6.4's "not identified" state is the ABSENCE of an external_id row, so a zero
// field must produce no row at all rather than a row reading "0" — which would
// then collide with every other unidentified series on (source, value).
func TestWeblinkParsedIDsAreAbsentRatherThanZero(t *testing.T) {
	ids := kavitaExternalIDs(kavita.SeriesDto{}, kindDecision{Kind: "book"})
	if len(ids) != 0 {
		t.Errorf("an untagged series produced %+v; want no identifiers at all", ids)
	}
}

// TestWeblinkIdentityEndToEndAgainstTheDatabase runs the WHOLE channel-1 path —
// cassettes, adapter, batching, real migrated SQLite — and then asks the
// database what it holds, in ux_extid_work_strong's own terms.
//
// It is the only assertion in this file that can catch a store-side write which
// re-strengthens what the mapping weakened, and the only one that can prove the
// consequence rather than the number: series 301 and 302 claim THE SAME AniList
// id, so if the cap ever comes off they do not merely gain a strong row, they
// COLLAPSE INTO ONE WORK.
func TestWeblinkIdentityEndToEndAgainstTheDatabase(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := NewKavitaSource(newCassetteClient(t,
		"kavita_libraries_weblink_ids.yaml", "kavita_series_all_v2_weblink_ids.yaml"))

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
		// 301 — a flag-OFF Manga library, where the plain scanner cannot have
		// written either field. Still 0.90: the DTO carries no provenance and
		// the Edit Series dialog reaches both.
		"301": {
			{AniListSource, "30002", WebLinkConfidence},
			{MalMangaSource, "2", WebLinkConfidence},
		},
		// 302 — the same AniList id as 301, from a flag-ON library.
		"302": {{AniListSource, "30002", WebLinkConfidence}},
		// 303 — MangaBaka, which has no provider writer in Kavita at all.
		"303": {{MangaBakaSource, "3391", WebLinkConfidence}},
		// 304 — MAL id 1 in a Book library, which Kavita+ cannot reach.
		"304": {{MalMangaSource, "1", WebLinkConfidence}},
		// 305 — the untagged control. Nothing, and that is not an error.
		"305": nil,
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
	// terms. A single row here means one of these three can merge two works.
	strong := countRows(t, s, `
		SELECT count(*) FROM external_id
		 WHERE source IN ('anilist', 'mal_manga', 'mangabaka')
		   AND work_id IS NOT NULL AND confidence >= 1.0`)
	if strong != 0 {
		t.Errorf("%d weblink-parsed rows satisfy ux_extid_work_strong's predicate "+
			"(work_id IS NOT NULL AND confidence >= 1.0); want 0", strong)
	}

	// ── THE CONSEQUENCE, not the number. 301 and 302 share aniListId 30002 and
	// are different series; at 1.0 the tier-1 reuse in store.ApplyCatalogueBatch
	// makes them one work, unrecoverably.
	var distinct int
	if err := s.DB().Read().QueryRowContext(t.Context(), `
		SELECT count(DISTINCT work_id) FROM service_item_link
		 WHERE service_instance_id = ? AND remote_id IN ('301', '302')`, inst).Scan(&distinct); err != nil {
		t.Fatalf("count distinct works: %v", err)
	}
	if distinct != 2 {
		t.Errorf("series 301 and 302 resolved to %d distinct works, want 2 — they share "+
			"aniListId 30002, and a strong write on a free-text-derived id MERGED THEM", distinct)
	}

	// ── The bare namespace is gone from the database, not merely from the
	// mapping. LS-10's collision (MAL numbers anime and manga separately) is
	// closed by the source string, so a regression has to fail here.
	if n := countRows(t, s, `SELECT count(*) FROM external_id WHERE source = 'mal'`); n != 0 {
		t.Errorf("%d rows carry the bare source 'mal'; MyAnimeList numbers anime and manga "+
			"separately (myanimelist.net/anime/1 is Cowboy Bebop, /manga/1 is Monster), so a "+
			"bare namespace lets two different works collide on (source, value)", n)
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
