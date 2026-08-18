package store

import (
	"database/sql"
	"testing"
)

// work.year's write path, which lives on the credit pass because the metadata
// read that produces it is the same round trip (see CreditSet.Year).
//
// Every test here runs against a REAL MIGRATED DATABASE and imports through
// ApplyCatalogueBatch first, on credits_test.go's rule: the credit pass resolves
// its work through the service_item_link the item pass wrote, so a test that
// inserted a `work` row directly would assert nothing about the real path.

// yearOf reads work.year back as the column actually holds it, so a NULL and a
// 0 are distinguishable — which is the entire point of the guard under test.
func yearOf(t *testing.T, s *Store, workID int64) sql.NullInt64 {
	t.Helper()
	var y sql.NullInt64
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT year FROM work WHERE id = ?`, workID).Scan(&y); err != nil {
		t.Fatalf("read work %d year: %v", workID, err)
	}
	return y
}

func year(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: true} }

// TestACreditPassWritesTheYearAndRewritesIt covers the whole lifecycle in one
// sequence, because the interesting part is what the SECOND and THIRD calls do:
// an insert-only implementation passes step 1 and fails step 3.
func TestACreditPassWritesTheYearAndRewritesIt(t *testing.T) {
	s := newTestStore(t)
	inst, bookA, bookB := creditFixture(t, s)

	set := func(remoteID string, y sql.NullInt64) CreditSet {
		return CreditSet{
			RemoteKind: "series", RemoteID: remoteID, Year: y,
			Credits: []Credit{credit("author", "Susanna Clarke", 0)},
		}
	}

	// 1. First import. The item pass left year NULL; the credit pass fills it.
	if y := yearOf(t, s, bookA); y.Valid {
		t.Fatalf("fixture: the item pass wrote year %v, want NULL", y)
	}
	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		set("10", year(2004)), set("11", sql.NullInt64{}),
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if got := yearOf(t, s, bookA); got != year(2004) {
		t.Errorf("book A year = %+v after the first import, want 2004", got)
	}
	if got := yearOf(t, s, bookB); got.Valid {
		t.Errorf("book B year = %+v, want NULL — the source reported none", got)
	}
	if res.YearsWritten != 1 {
		t.Errorf("YearsWritten = %d on the first import, want 1 (book B was NULL and stayed NULL)",
			res.YearsWritten)
	}

	// 2. A RE-IMPORT REPORTING THE SAME YEAR TOUCHES NOTHING. This is what the
	// `year IS NOT ?` guard buys, and it is asserted through updated_at rather
	// than only through the counter, so a counter that lied would still fail.
	var beforeUpdated string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT updated_at FROM work WHERE id = ?`, bookA).Scan(&beforeUpdated); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	later := testNow.Add(48 * 60 * 60 * 1e9)
	res, err = s.ApplyCredits(t.Context(), inst, []CreditSet{
		set("10", year(2004)), set("11", sql.NullInt64{}),
	}, later)
	if err != nil {
		t.Fatalf("ApplyCredits again: %v", err)
	}
	if res.YearsWritten != 0 {
		t.Errorf("YearsWritten = %d on an unchanged re-import, want 0", res.YearsWritten)
	}
	var afterUpdated string
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT updated_at FROM work WHERE id = ?`, bookA).Scan(&afterUpdated); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	if afterUpdated != beforeUpdated {
		t.Errorf("an unchanged re-import bumped updated_at from %q to %q", beforeUpdated, afterUpdated)
	}

	// 3. A CHANGED YEAR IS REWRITTEN, and a year the upstream stops reporting is
	// ERASED. The second half is the direction an insert-only or
	// COALESCE-flavoured implementation gets wrong: an operator who deletes a
	// bad year in Kavita must not keep seeing it in UsArr forever.
	res, err = s.ApplyCredits(t.Context(), inst, []CreditSet{
		set("10", sql.NullInt64{}), set("11", year(1999)),
	}, later)
	if err != nil {
		t.Fatalf("ApplyCredits a third time: %v", err)
	}
	if got := yearOf(t, s, bookA); got.Valid {
		t.Errorf("book A year = %+v after the upstream dropped it, want NULL", got)
	}
	if got := yearOf(t, s, bookB); got != year(1999) {
		t.Errorf("book B year = %+v after the upstream gained one, want 1999", got)
	}
	if res.YearsWritten != 2 {
		t.Errorf("YearsWritten = %d, want 2 — one erased and one written", res.YearsWritten)
	}
}

// TestAYearIsNeverWrittenForAnUnimportedItem keeps the year on the same footing
// as the credits: an item whose batch never committed has no link, so there is
// nothing to write a year onto and nothing must be invented.
func TestAYearIsNeverWrittenForAnUnimportedItem(t *testing.T) {
	s := newTestStore(t)
	inst, _, _ := creditFixture(t, s)

	res, err := s.ApplyCredits(t.Context(), inst, []CreditSet{
		{RemoteKind: "series", RemoteID: "99", Year: year(2012)},
	}, testNow)
	if err != nil {
		t.Fatalf("ApplyCredits: %v", err)
	}
	if res.ItemsUnresolved != 1 || res.YearsWritten != 0 {
		t.Errorf("an unimported item gave %+v, want ItemsUnresolved=1 and YearsWritten=0", res)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM work WHERE year IS NOT NULL`); n != 0 {
		t.Errorf("%d works have a year after a set that resolved to nothing", n)
	}
}
