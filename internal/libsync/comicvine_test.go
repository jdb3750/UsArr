package libsync

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// TestParseComicVineWebLinks pins the ONE thing SeriesDto.comicVineId cannot
// tell you: which ComicVine number space an id belongs to.
//
// The two canonical URL shapes are quoted from Kavita's own source at v0.9.0.2
// (Kavita.Common/Helpers/WeblinkParser.cs:23-31), and the /4000-159233/ case is
// the exact string Kavita's unit test uses
// (Kavita.Common.Tests/Helpers/WeblinkParserTests.cs:24) — where Kavita returns
// "159233" with no kind attached, and this returns the kind.
func TestParseComicVineWebLinks(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []ComicVineRef
	}{
		{"empty", "", nil},
		{"whitespace only", "   \t ", nil},
		{
			"a 4050 volume url, the shape Kavita documents as Series",
			"https://comicvine.gamespot.com/batman-the-caped-crusader/4050-112794/",
			[]ComicVineRef{{ComicVineVolume, "112794"}},
		},
		{
			"a 4000 issue url, the shape Kavita's own test uses",
			"https://comicvine.gamespot.com/chew-1-taster-s-choice-part-1-of-5/4000-159233/",
			[]ComicVineRef{{ComicVineIssue, "159233"}},
		},
		{
			// ProcessSeries.cs:886-893 joins ComicInfo.xml's <Web> with commas,
			// so this is the shape chapter.WebLinks actually has on disk.
			"the comma-joined multi-link form Kavita stores",
			"https://anilist.co/manga/30013," +
				"https://comicvine.gamespot.com/chew/4050-38442/," +
				"https://myanimelist.net/manga/2",
			[]ComicVineRef{{ComicVineVolume, "38442"}},
		},
		{
			// ProcessSeries.cs:891 splits on ' ' when there is no comma, so a
			// space-separated <Web> is equally real.
			"the space-separated form Kavita also accepts",
			"https://comicvine.gamespot.com/chew/4050-38442/ https://comicvine.gamespot.com/chew-1/4000-159233/",
			[]ComicVineRef{{ComicVineVolume, "38442"}, {ComicVineIssue, "159233"}},
		},
		{
			"an unlinked token, the shape a user copies out of the url bar",
			"4050-112794",
			[]ComicVineRef{{ComicVineVolume, "112794"}},
		},
		{
			"http and a www host, both of which a pasted link may carry",
			"http://www.comicvine.gamespot.com/saga/4050-42563/",
			[]ComicVineRef{{ComicVineVolume, "42563"}},
		},
		{
			// The whole reason the id regex is anchored to a path segment: a
			// slug is free text and may contain the digits.
			"a slug containing 4050 must not be read as the id",
			"https://comicvine.gamespot.com/some-4050-thing/4000-1/",
			[]ComicVineRef{{ComicVineIssue, "1"}},
		},
		{
			"a comicvine link with no id segment at all",
			"https://comicvine.gamespot.com/profile/someone/",
			nil,
		},
		{
			"a look-alike host is not comicvine",
			"https://comicvine.gamespot.com.evil.test/chew/4050-38442/",
			nil,
		},
		{
			"non-comicvine links are skipped, not errors",
			"https://anilist.co/manga/30013,https://mangabaka.org/3391",
			nil,
		},
		{
			"a bare number carries no kind and is not a ref",
			"159233",
			nil,
		},
		{
			"the same volume linked twice is one ref",
			"https://comicvine.gamespot.com/chew/4050-38442/,https://comicvine.gamespot.com/chew/4050-38442/",
			[]ComicVineRef{{ComicVineVolume, "38442"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseComicVineWebLinks(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseComicVineWebLinks(%q) = %v, want %v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("ref %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestVolumeRefRefusesToGuess: two DIFFERENT volume ids in one field describe
// two different ComicVine volumes, and picking one is a guess. It is the same
// conservatism Kavita applies at ProcessSeries.cs:159-163, which sets
// series.ComicVineId only when the distinct count is exactly 1.
func TestVolumeRefRefusesToGuess(t *testing.T) {
	if _, ok := VolumeRef(nil); ok {
		t.Error("VolumeRef(nil) claimed a volume")
	}
	if _, ok := VolumeRef([]ComicVineRef{{ComicVineIssue, "159233"}}); ok {
		t.Error("an issue-only list yielded a volume")
	}
	v, ok := VolumeRef([]ComicVineRef{{ComicVineIssue, "1"}, {ComicVineVolume, "38442"}})
	if !ok || v.ID != "38442" {
		t.Errorf("one volume among issues = %+v/%v, want 38442/true", v, ok)
	}
	if _, ok := VolumeRef([]ComicVineRef{{ComicVineVolume, "1"}, {ComicVineVolume, "2"}}); ok {
		t.Error("two different volume ids yielded a pick; UsArr must refuse to guess")
	}
	if v, ok := VolumeRef([]ComicVineRef{{ComicVineVolume, "7"}, {ComicVineVolume, "7"}}); !ok || v.ID != "7" {
		t.Errorf("the same volume twice = %+v/%v, want 7/true", v, ok)
	}
}

// TestComicVineIdentity is the decision table, stated as behaviour.
//
// The `wantSource` column is empty exactly when nothing may be written, and
// every such row must carry a REASON: principle 3's "says what is missing and
// why" is what StreamItems logs.
func TestComicVineIdentity(t *testing.T) {
	const (
		volumeURL = "https://comicvine.gamespot.com/batman-the-caped-crusader/4050-112794/"
		issueURL  = "https://comicvine.gamespot.com/chew-1-taster-s-choice-part-1-of-5/4000-159233/"
	)
	cases := []struct {
		name       string
		raw        string
		inherit    bool
		wantSource string
		wantValue  string
		wantReason bool
	}{
		{"the ordinary untagged case is silent", "", false, "", "", false},
		{"a zero is the same as absent", "0", false, "", "", false},
		{"a bare id with the flag off is a volume id", "112794", false, ComicVineVolumeSource, "112794", false},
		{"a 4050 url is a volume id whatever the flag says", volumeURL, false, ComicVineVolumeSource, "112794", false},
		{"a 4050 url survives the inherit flag, because its kind is proven", volumeURL, true, ComicVineVolumeSource, "112794", false},
		{"a 4000 url is an issue and is refused", issueURL, false, "", "", true},
		{"an issue url is refused with the flag on too", issueURL, true, "", "", true},
		{"🚩 a bare id with the flag ON is refused", "159233", true, "", "", true},
		{"a shape UsArr does not recognise is refused", "cv:159233", false, "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, reason, ok := comicVineIdentity(tc.raw, tc.inherit)
			if ok != (tc.wantSource != "") {
				t.Fatalf("comicVineIdentity(%q, %v) ok=%v, want %v (reason %q)",
					tc.raw, tc.inherit, ok, tc.wantSource != "", reason)
			}
			if (reason != "") != tc.wantReason {
				t.Errorf("reason = %q, want a reason: %v", reason, tc.wantReason)
			}
			if !ok {
				return
			}
			if id.Source != tc.wantSource || id.Value != tc.wantValue {
				t.Errorf("got %s=%s, want %s=%s", id.Source, id.Value, tc.wantSource, tc.wantValue)
			}
			// THE INVARIANT. ux_extid_work_strong is
			// UNIQUE(source, value) WHERE work_id IS NOT NULL AND confidence >= 1.0,
			// and §6.4 amendment 3 caps anything parsed out of a free-text field
			// at 0.90 precisely because a 1.0 write merges works and v0.1 cannot
			// un-merge.
			if id.Confidence >= 1.0 {
				t.Errorf("comicVineIdentity(%q, %v) returned confidence %v; a ComicVine id is "+
					"ALWAYS parsed out of ComicInfo.xml's free-text <Web> element or typed by a "+
					"user, so §6.4 amendment 3 caps it below 1.0 — at 1.0 it satisfies "+
					"ux_extid_work_strong and MERGES WORKS, which v0.1 cannot undo",
					tc.raw, tc.inherit, id.Confidence)
			}
			if id.Confidence != ComicVineConfidence {
				t.Errorf("confidence %v, want ComicVineConfidence (%v)", id.Confidence, ComicVineConfidence)
			}
		})
	}
}

// TestComicVineIsNeverWorkStrongThroughTheMapping is the same invariant asserted
// one level up, through the real projection rather than through the helper.
//
// It exists because the helper could be correct and the CALLER still wrong:
// kavitaExternalIDs's `add` closure hard-codes Confidence 1.0, and the bug this
// whole change corrects was precisely a ComicVine id going through it.
func TestComicVineIsNeverWorkStrongThroughTheMapping(t *testing.T) {
	comic := kindDecision{Kind: "comic", ReadingDirection: "ltr"}
	inherited := kindDecision{Kind: "comic", ReadingDirection: "ltr", InheritsWebLinks: true}

	for _, tc := range []struct {
		name string
		dto  kavita.SeriesDto
		d    kindDecision
	}{
		{"bare id", kavita.SeriesDto{ID: 1, Name: "A", ComicVineID: "112794"}, comic},
		{"volume url", kavita.SeriesDto{
			ID: 2, Name: "B",
			ComicVineID: "https://comicvine.gamespot.com/chew/4050-38442/",
		}, comic},
		{"issue url", kavita.SeriesDto{
			ID: 3, Name: "C",
			ComicVineID: "https://comicvine.gamespot.com/chew-1/4000-159233/",
		}, comic},
		{"inherited bare id", kavita.SeriesDto{ID: 4, Name: "D", ComicVineID: "159233"}, inherited},
		{"inherited volume url", kavita.SeriesDto{
			ID: 5, Name: "E",
			ComicVineID: "https://comicvine.gamespot.com/chew/4050-38442/",
		}, inherited},
		// The other identifier fields must keep going through at 1.0: this
		// change is about ComicVine, and §6.4 amendment 3 reaching the rest is
		// recorded as an OPEN follow-up (REVIEW-LOG LS-12), not silently done.
		{"a manga series with typed ids", kavita.SeriesDto{
			ID: 6, Name: "F",
			AniListID: 30013, MalID: 2, ComicVineID: "159233",
		}, inherited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, x := range mapSeries(tc.dto, tc.d).ExternalIDs {
				if x.Source != ComicVineVolumeSource && x.Source != "comicvine" {
					continue
				}
				if x.Confidence >= 1.0 {
					t.Errorf("%s=%s came out at confidence %v — it satisfies "+
						"ux_extid_work_strong and would MERGE WORKS", x.Source, x.Value, x.Confidence)
				}
			}
		})
	}
}

// TestInheritedFirstChapterIssueIDIsNeverWritten is the guard the whole
// correction hangs on, stated as narrowly as it can be.
//
// 🚩 Library 11 in the fixtures has inheritWebLinksFromFirstChapter ON, so
// ProcessSeries.cs:365 has already overwritten series.ComicVineId with the FIRST
// CHAPTER's id and thrown away the 4050/4000 discriminator. "159233" is Kavita's
// own test's ISSUE id. It must produce NO external_id row at all — not a weak
// one, not one under a different source: there is no source string under which a
// value of unknown kind is a true statement.
func TestInheritedFirstChapterIssueIDIsNeverWritten(t *testing.T) {
	it := mapSeries(
		kavita.SeriesDto{ID: 204, Name: "Invincible", ComicVineID: "159233"},
		kindDecision{Kind: "comic", ReadingDirection: "ltr", InheritsWebLinks: true},
	)
	for _, x := range it.ExternalIDs {
		t.Errorf("an inherit-flagged library produced %s=%s at confidence %v; it must produce "+
			"NOTHING, because Kavita discarded the only fact that said whether 159233 names a "+
			"volume or an issue", x.Source, x.Value, x.Confidence)
	}

	// And the same value from a library WITHOUT the flag is a volume id, which
	// is what makes the assertion above about the FLAG rather than about the
	// value.
	ok := mapSeries(
		kavita.SeriesDto{ID: 204, Name: "Invincible", ComicVineID: "159233"},
		kindDecision{Kind: "comic", ReadingDirection: "ltr"},
	)
	want := store.ExternalIdentifier{
		Source: ComicVineVolumeSource, Value: "159233", Confidence: ComicVineConfidence,
	}
	if len(ok.ExternalIDs) != 1 || ok.ExternalIDs[0] != want {
		t.Errorf("without the flag the same value gave %v, want exactly [%+v]", ok.ExternalIDs, want)
	}
}

// TestComicVineIdentityEndToEndAgainstTheDatabase runs the WHOLE channel-1 path
// — cassettes, adapter, batching, real migrated SQLite — and then asks the
// database what it actually holds. It is the only assertion here that can catch
// a store-side write that re-strengthens what the mapping weakened.
//
// The fixture library set is deliberately minimal and exists to separate ONE
// variable: libraries 10 and 11 are both LibraryType 1 (Comic) and differ only
// in inheritWebLinksFromFirstChapter.
func TestComicVineIdentityEndToEndAgainstTheDatabase(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)
	src := NewKavitaSource(newCassetteClient(t,
		"kavita_libraries_comicvine.yaml", "kavita_series_all_v2_comicvine.yaml"))

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

	// Every external_id row in the database, keyed by the remote series it came
	// from. Read back through the LINK so the assertion is about "what this
	// series claims", not about a bare row count.
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
		// 201 — a bare id in a library with the flag OFF. The only scanner path
		// that can have written it is the 4050-only one.
		"201": {{ComicVineVolumeSource, "112794", ComicVineConfidence}},
		// 202 — a pasted 4050 URL. Kind proven by the URL itself.
		"202": {{ComicVineVolumeSource, "38442", ComicVineConfidence}},
		// 203 — a pasted 4000 URL. An ISSUE is not the work.
		"203": nil,
		// 204 — 🚩 the inherited first-chapter issue id. Nothing.
		"204": nil,
		// 205 — the untagged book control. Nothing, and that is not an error.
		"205": nil,
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

	// The structural assertion, asked of SQLite in ux_extid_work_strong's own
	// terms. A single row here would mean a ComicVine id can merge two works,
	// and v0.1 has no work_merge table to undo it with.
	strong := countRows(t, s, `
		SELECT count(*) FROM external_id
		 WHERE source LIKE 'comicvine%' AND work_id IS NOT NULL AND confidence >= 1.0`)
	if strong != 0 {
		t.Errorf("%d ComicVine rows satisfy ux_extid_work_strong's predicate "+
			"(work_id IS NOT NULL AND confidence >= 1.0); want 0", strong)
	}

	// All five works exist and are searchable regardless: a refused identity
	// claim degrades identity, it never drops the work.
	if n := countRows(t, s, `SELECT count(*) FROM work`); n != 5 {
		t.Errorf("%d work rows, want 5 — refusing an identity claim must not drop the work", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM search_doc`); n != 5 {
		t.Errorf("%d search_doc rows, want 5", n)
	}
}

// TestKavitaSourceLogsARefusedComicVineClaim: the refusal is not silent.
// Principle 3 — "it says what is missing and why" — and a dropped identity claim
// with no trace is exactly the empty screen that rule exists to prevent.
func TestKavitaSourceLogsARefusedComicVineClaim(t *testing.T) {
	var buf bytes.Buffer
	src := NewKavitaSource(newCassetteClient(t,
		"kavita_libraries_comicvine.yaml", "kavita_series_all_v2_comicvine.yaml"))
	src.Log = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if _, err := src.StreamItems(t.Context(), func(store.CatalogueItem) error { return nil }); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}

	out := buf.String()
	// Exactly the two refusals the fixture contains: the 4000 issue URL (203)
	// and the inherited bare id (204).
	if n := strings.Count(out, "refused a ComicVine identity claim"); n != 2 {
		t.Errorf("logged %d refusals, want 2:\n%s", n, out)
	}
	for _, want := range []string{"series_id=203", "series_id=204", "inheritWebLinksFromFirstChapter"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal log never mentions %q:\n%s", want, out)
		}
	}
}
