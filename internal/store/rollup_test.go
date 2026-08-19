package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// Every test in this file runs against a REAL MIGRATED DATABASE and POPULATES
// BEFORE IT ASSERTS.

// rollupFixture imports one comic and one book into one instance and returns
// both work ids, so a flush has something with a k="count" shape to aggregate.
func rollupFixture(t *testing.T, s *Store) (inst, comicID, bookID int64) {
	t.Helper()
	inst = fixtureInstance(t, s, "kavita-rollup")
	binds, _, err := s.BindContainers(t.Context(), inst, SystemUserID, []CatalogueContainer{
		{RemoteID: "1", Name: "Manga", Kind: "comic"},
		{RemoteID: "2", Name: "Ebooks", Kind: "book"},
	})
	if err != nil {
		t.Fatalf("BindContainers: %v", err)
	}
	comicID = importOne(t, s, inst, binds, item("41", "1", "comic", "Frieren"))
	bookID = importOne(t, s, inst, binds, item("77", "2", "book", "The Hobbit"))
	return inst, comicID, bookID
}

func readRollup(t *testing.T, s *Store, workID int64) (have int64, blob sql.NullString) {
	t.Helper()
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT have_count, availability FROM work WHERE id = ?`, workID).Scan(&have, &blob); err != nil {
		t.Fatalf("read the rollup of work %d: %v", workID, err)
	}
	return have, blob
}

// applyStatusAndTotal runs the credit path so the denominator gate has its two
// inputs, without hand-writing the rows the pass writes.
func applyStatusAndTotal(t *testing.T, s *Store, inst int64, remoteID, status string, total int64) {
	t.Helper()
	set := CreditSet{RemoteKind: "series", RemoteID: remoteID, Status: status}
	if total > 0 {
		set.TotalDeclared = sql.NullInt64{Int64: total, Valid: true}
	}
	if _, err := s.ApplyCredits(t.Context(), inst, []CreditSet{set}, testNow); err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
}

func walkFiles(t *testing.T, s *Store, inst int64, remoteID string, n int) {
	t.Helper()
	set := FileSet{RemoteKind: "series", RemoteID: remoteID}
	for i := range n {
		set.Files = append(set.Files, mediaFile(
			"kavita:mangafile:"+remoteID+"-"+string(rune('a'+i)), int64(1000+i)))
	}
	if _, err := s.ApplyFiles(t.Context(), inst, []FileSet{set}); err != nil {
		t.Fatalf("ApplyFiles: %v", err)
	}
}

// TestTheFlushCountsFilesAndClearsTheFlag is the happy path end to end: file
// rows in, have_count and a blob out, dirty flag cleared.
func TestTheFlushCountsFilesAndClearsTheFlag(t *testing.T) {
	s := newTestStore(t)
	inst, comicID, _ := rollupFixture(t, s)
	walkFiles(t, s, inst, "41", 3)

	res, err := s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0)
	if err != nil {
		t.Fatalf("FlushRollups: %v", err)
	}
	if res.WorksFlushed != 1 || res.BlobsWritten != 1 || res.BlobsWithheld != 0 {
		t.Errorf("result = %+v; want 1 flushed, 1 blob written, 0 withheld", res)
	}
	have, blob := readRollup(t, s, comicID)
	if have != 3 {
		t.Errorf("have_count = %d, want 3", have)
	}
	if !blob.Valid || blob.String != `{"k":"count","have":3,"total":null,"total_source":null}` {
		t.Errorf("availability = %v", blob)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE rollup_dirty = 1`); n != 0 {
		t.Errorf("%d works are still dirty after the flush", n)
	}

	// A second flush has nothing to do — the flag is what schedules the work,
	// so an idle flush must not rewrite anything.
	res, err = s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0)
	if err != nil {
		t.Fatalf("second FlushRollups: %v", err)
	}
	if res.WorksFlushed != 0 {
		t.Errorf("an idle flush touched %d works", res.WorksFlushed)
	}
}

// TestTheRollupRefusesToFabricateAZero is REVIEW-LOG.md LS-211.2 rule 2, as
// code.
//
// A work with no file rows gets NO BLOB — not `have: 0`. The failure it prevents
// is the front end taking the availability.k === 'count' arm and rendering
// `0 / N · N missing` on a healthy library: a denominator with no numerator,
// arriving through a column that now looks computed, which is strictly worse
// than the blank column it replaced.
//
// ⚠️ IT IS ENFORCED IN THE WRITER, NOT BY SEQUENCING. This test marks a work
// dirty with no files at all — the state a walk that has not run yet leaves, and
// the steady state during every subsequent sync — and requires the flush to
// leave the column NULL.
func TestTheRollupRefusesToFabricateAZero(t *testing.T) {
	s := newTestStore(t)
	_, comicID, _ := rollupFixture(t, s)

	// Dirty, with no media_file rows anywhere. This is the sequencing accident
	// rule 2 exists to survive: the rollup landing before the walk.
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work SET rollup_dirty = 1 WHERE id = ?`, comicID)
		return err
	}); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}

	res, err := s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0)
	if err != nil {
		t.Fatalf("FlushRollups: %v", err)
	}
	if res.BlobsWithheld != 1 || res.BlobsWritten != 0 {
		t.Errorf("result = %+v; want the blob WITHHELD, not written", res)
	}
	have, blob := readRollup(t, s, comicID)
	if blob.Valid {
		t.Errorf("availability = %q for a work with no file rows; the rollup fabricated a "+
			"denominator's numerator and the screen will render `0 / N · N missing` on a "+
			"healthy library (LS-211.2 rule 2)", blob.String)
	}
	if have != 0 {
		t.Errorf("have_count = %d with no file rows, want 0", have)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE rollup_dirty = 1`); n != 0 {
		t.Error("a work the flush could not count is still dirty, so every later flush picks it up")
	}
}

// TestABlobIsWithdrawnWhenTheBasisGoesAway is the other direction of the same
// rule, and the one that only exists because the walk reconciles.
//
// A series whose chapters were all deleted upstream ends with zero file rows.
// Leaving yesterday's `have: 43` standing would be a lie; rewriting it as
// `have: 0` would assert emptiness through a computed-looking column. The blob
// is withdrawn, which returns the row to "not counted" — the honest state when
// "never walked" and "walked, and empty" are indistinguishable from here.
func TestABlobIsWithdrawnWhenTheBasisGoesAway(t *testing.T) {
	s := newTestStore(t)
	inst, comicID, _ := rollupFixture(t, s)

	walkFiles(t, s, inst, "41", 2)
	if _, err := s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0); err != nil {
		t.Fatalf("first flush: %v", err)
	}
	if _, blob := readRollup(t, s, comicID); !blob.Valid {
		t.Fatal("the first flush wrote no blob, so this test asserts nothing")
	}

	// Everything deleted upstream: the next walk reports an empty set and the
	// reconciliation removes the rows.
	if _, err := s.ApplyFiles(t.Context(), inst, []FileSet{{RemoteKind: "series", RemoteID: "41"}}); err != nil {
		t.Fatalf("empty walk: %v", err)
	}
	if _, err := s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	have, blob := readRollup(t, s, comicID)
	if blob.Valid {
		t.Errorf("availability = %q after every file was deleted upstream; a stale blob is a "+
			"claim about a library the user no longer has", blob.String)
	}
	if have != 0 {
		t.Errorf("have_count = %d after every file was deleted, want 0", have)
	}
}

// TestTheDenominatorIsGatedOnStatusAndDeclaration fires ARCHITECTURE.md §6.1's
// rule: `43 / 60` only when the series has ENDED and a total is declared.
func TestTheDenominatorIsGatedOnStatusAndDeclaration(t *testing.T) {
	cases := []struct {
		name   string
		status string
		total  int64
		want   string
	}{
		{
			"completed with a declared total", WorkStatusCompleted, 60,
			`{"k":"count","have":2,"total":60,"total_source":"comicinfo"}`,
		},
		{
			"ended with a declared total", WorkStatusEnded, 12,
			`{"k":"count","have":2,"total":12,"total_source":"comicinfo"}`,
		},
		{
			"cancelled with a declared total", WorkStatusCancelled, 5,
			`{"k":"count","have":2,"total":5,"total_source":"comicinfo"}`,
		},
		// The two halves of the gate, each failing on its own.
		{
			"ongoing with a declared total", WorkStatusOngoing, 60,
			`{"k":"count","have":2,"total":null,"total_source":null}`,
		},
		{
			"on hiatus with a declared total", WorkStatusHiatus, 60,
			`{"k":"count","have":2,"total":null,"total_source":null}`,
		},
		{
			"completed with nothing declared", WorkStatusCompleted, 0,
			`{"k":"count","have":2,"total":null,"total_source":null}`,
		},
		// The unmeasured case: an instance that populates neither field sends
		// publicationStatus 0 and totalCount 0, which the adapter maps to
		// ongoing plus no total. It takes the safe branch.
		{
			"an instance that populates neither field", WorkStatusOngoing, 0,
			`{"k":"count","have":2,"total":null,"total_source":null}`,
		},
		{
			"no statement at all", "", 0,
			`{"k":"count","have":2,"total":null,"total_source":null}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			inst, comicID, _ := rollupFixture(t, s)
			applyStatusAndTotal(t, s, inst, "41", tc.status, tc.total)
			walkFiles(t, s, inst, "41", 2)
			if _, err := s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0); err != nil {
				t.Fatalf("FlushRollups: %v", err)
			}
			_, blob := readRollup(t, s, comicID)
			if !blob.Valid || blob.String != tc.want {
				t.Errorf("availability = %v,\n           want %s", blob, tc.want)
			}
		})
	}
}

// TestABookNeverGetsADeclaredTotal pins the kind-scoping. work_book has no
// total column — a book is not a series of issues — so the denominator query
// finds no row and the blob says `total: null` whatever the status claims.
func TestABookNeverGetsADeclaredTotal(t *testing.T) {
	s := newTestStore(t)
	inst, _, bookID := rollupFixture(t, s)
	applyStatusAndTotal(t, s, inst, "77", WorkStatusEnded, 99)
	walkFiles(t, s, inst, "77", 1)
	if _, err := s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0); err != nil {
		t.Fatalf("FlushRollups: %v", err)
	}
	_, blob := readRollup(t, s, bookID)
	const want = `{"k":"count","have":1,"total":null,"total_source":null}`
	if !blob.Valid || blob.String != want {
		t.Errorf("availability = %v, want %s", blob, want)
	}
}

// TestTheRollupScopeActuallyFilters fires the scope predicate, on
// TestRecentWorksScopeActuallyFilters's rule: a scope a query accepts and never
// filters on is indistinguishable from no scope at all.
//
// A rollup computed across instances a user cannot see is an EXISTENCE ORACLE
// (reference/security.md), which is why the parameter is in the signature from
// day one rather than added when multi-user lands.
func TestTheRollupScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	inst, comicID, _ := rollupFixture(t, s)
	walkFiles(t, s, inst, "41", 3)

	markDirty := func() {
		t.Helper()
		if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE work SET rollup_dirty = 1 WHERE id = ?`, comicID)
			return err
		}); err != nil {
			t.Fatalf("mark dirty: %v", err)
		}
	}

	// Arm 1 — the visible set names this instance. The files count.
	markDirty()
	if _, err := s.FlushRollups(t.Context(), Scope{UserID: SystemUserID, InstanceIDs: []int64{inst}}, 0); err != nil {
		t.Fatalf("FlushRollups: %v", err)
	}
	if have, blob := readRollup(t, s, comicID); have != 3 || !blob.Valid {
		t.Errorf("a scope that CAN see the instance produced have_count %d and blob %v", have, blob)
	}

	// Arm 2 — the visible set names a DIFFERENT instance. The same files must
	// not be counted, and the blob must not be written from them.
	other := fixtureInstance(t, s, "kavita-other")
	markDirty()
	res, err := s.FlushRollups(t.Context(), Scope{UserID: SystemUserID, InstanceIDs: []int64{other}}, 0)
	if err != nil {
		t.Fatalf("FlushRollups: %v", err)
	}
	if res.BlobsWithheld != 1 {
		t.Errorf("result = %+v; a scope that cannot see the instance still counted its files", res)
	}
	if have, blob := readRollup(t, s, comicID); have != 0 || blob.Valid {
		t.Errorf("a scope that cannot see the instance produced have_count %d and blob %v; that "+
			"figure is an existence oracle", have, blob)
	}

	// Arm 3 — an EMPTY visible set FAILS CLOSED. "No visible instances" must
	// mean no rows, never every row.
	markDirty()
	res, err = s.FlushRollups(t.Context(), Scope{UserID: SystemUserID}, 0)
	if err != nil {
		t.Fatalf("FlushRollups: %v", err)
	}
	if res.BlobsWithheld != 1 {
		t.Errorf("result = %+v; an empty visible set did not fail closed", res)
	}
	if have, _ := readRollup(t, s, comicID); have != 0 {
		t.Errorf("an empty visible set counted %d files", have)
	}
}

// TestAKindWithNoBlobShapeLeavesBothColumnsAlone pins the invariant
// http-api.md §1.4.1 rests on: there is no path that moves have_count without
// also writing the blob, so a work this writer has no shape for must not have
// its count written either.
//
// Video is k="tier" and music is k="edition". Neither has a source that writes
// files yet, so this state is unreachable through the importer — which is
// exactly why the test constructs it directly.
func TestAKindWithNoBlobShapeLeavesBothColumnsAlone(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := rollupFixture(t, s)

	var movieID int64
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO work (kind, title, sort_title, normalized_title, norm_version,
			                  have_count, rollup_dirty, created_at, updated_at)
			VALUES ('movie','Train Dreams','Train Dreams','train dreams',1,7,1,?,?)`,
			FormatTime(testNow), FormatTime(testNow))
		if err != nil {
			return err
		}
		movieID, err = r.LastInsertId()
		if err != nil {
			return err
		}
		// Files for it, so the flush would have something to count if it tried.
		_, err = tx.ExecContext(ctx, `
			INSERT INTO media_file (work_id, service_instance_id, path, size_bytes)
			VALUES (?,?,'kavita:mangafile:999',10)`, movieID, inst)
		return err
	}); err != nil {
		t.Fatalf("seed the movie: %v", err)
	}

	res, err := s.FlushRollups(t.Context(), OwnerScope(SystemUserID), 0)
	if err != nil {
		t.Fatalf("FlushRollups: %v", err)
	}
	if res.KindsUnsupported != 1 {
		t.Errorf("result = %+v; want the movie counted as an unsupported kind", res)
	}
	have, blob := readRollup(t, s, movieID)
	if have != 7 {
		t.Errorf("have_count = %d; the flush rewrote a count for a kind whose blob shape it "+
			"cannot produce, which is the one state http-api.md §1.4.1 says cannot happen", have)
	}
	if blob.Valid {
		t.Errorf("availability = %q; a movie's blob is k=\"tier\", not k=\"count\"", blob.String)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE rollup_dirty = 1`); n != 0 {
		t.Error("the unsupported work is still dirty, so every later flush picks it up forever")
	}
}

// TestTheDirtyBatchPlanIsASeek EXPLAINs THE STATEMENT THAT SHIPS.
//
// internal/db's plan test already asserts ix_work_dirty for a hand-copied
// `SELECT id FROM work WHERE rollup_dirty = 1`, and until this commit that guard
// had no writer to be green about: the partial index was permanently empty
// (REVIEW-LOG.md LS-211.1). This one runs the real statement, with its extra
// column, its ORDER BY and its LIMIT — the three things a hand-copied lookalike
// would not have.
func TestTheDirtyBatchPlanIsASeek(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := rollupFixture(t, s)
	walkFiles(t, s, inst, "41", 2)

	plan, err := db.QueryPlan(t.Context(), s.DB().Read(), dirtyWorksSQL(), DefaultRollupBatch)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	joined := strings.Join(plan, " | ")
	if !planHas(joined, "ix_work_dirty") {
		t.Errorf("the flush does not pick up its batch through ix_work_dirty, so it scans every "+
			"work row for a column that is 0 almost always:\n  %s", joined)
	}
	if strings.Contains(joined, "SCAN work") {
		t.Errorf("the flush SCANS work:\n  %s", joined)
	}
}
