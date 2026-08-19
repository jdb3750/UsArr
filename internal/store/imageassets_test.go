package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// seedImageAssetCorpus builds the smallest corpus that can separate every answer
// LookupImageAsset has: an asset owned as a poster, one owned as a backdrop, one
// on a tombstoned work, one on a work only the SECOND instance replicated, and
// one no work points at at all.
//
// ⚠️ THESE INSERTS ARE IN A TEST FILE AND THAT IS LOAD-BEARING.
// TestImageWritesValidateTheFormatVocabulary exempts `_test.go`, so seeding
// image_asset here does not make that guard non-vacuous. ⚠️ It is no longer
// vacuous ANYWAY — imagewrite.go is the production writer, and it is what
// discharges the format-vocabulary check and the credential-stripped
// `source_url` assertion (security.md §5). Nothing in this file is a model for
// how a real writer should look; PutPosterAsset is, and imagewrite_test.go is
// where both obligations are exercised.
func seedImageAssetCorpus(t *testing.T, s *Store) {
	t.Helper()

	asset := func(id int64, key, format, state string) string {
		f := "NULL"
		if format != "" {
			f = "'" + format + "'"
		}
		return fmt.Sprintf(
			`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format, state)
			   VALUES (%d, 'http://kavita.example/cover/%d', 'configured', 'poster', '%s', %s, '%s')`,
			id, id, key, f, state)
	}
	work := func(id int64, title, deletedAt string, poster, backdrop any) string {
		del := "NULL"
		if deletedAt != "" {
			del = "'" + deletedAt + "'"
		}
		return fmt.Sprintf(
			`INSERT INTO work (id, kind, title, sort_title, normalized_title,
			                   deleted_at, poster_asset_id, backdrop_asset_id)
			   VALUES (%d, 'comic', '%s', '%s', '%s', %s, %v, %v)`,
			id, title, strings.ToLower(title), strings.ToLower(title), del, poster, backdrop)
	}

	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'kavita', 'library', 'Kavita Manga', 'http://kavita.example', X'00')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (2, 'kavita', 'library', 'Kavita Books', 'http://kavita2.example', X'00')`,

		// 1 — a ready poster on a live work replicated by instance 1.
		asset(1, "1111111111111111", ImageFormatJPEG, ImageStateReady),
		// 2 — a ready BACKDROP, so the other arm of the OR has a row.
		asset(2, "2222222222222222", ImageFormatJPEG, ImageStateReady),
		// 3 — pending: a row that exists, is owned, and has no bytes.
		asset(3, "3333333333333333", "", "pending"),
		// 4 — owned only by a TOMBSTONED work.
		asset(4, "4444444444444444", ImageFormatJPEG, ImageStateReady),
		// 5 — owned by a work only instance 2 replicated.
		asset(5, "5555555555555555", ImageFormatJPEG, ImageStateReady),
		// 6 — ORPHANED: no work points at it.
		asset(6, "6666666666666666", ImageFormatJPEG, ImageStateReady),

		work(1, "Berserk", "", 1, "NULL"),
		work(2, "Vinland Saga", "", "NULL", 2),
		work(3, "Vagabond", "", 3, "NULL"),
		work(4, "A Deleted Manga", "2026-08-16 09:00:00", 4, "NULL"),
		work(5, "An Instance Two Manga", "", 5, "NULL"),

		// Replication links. Works 1-4 came from instance 1, work 5 from
		// instance 2 — which is what lets a partial scope separate them.
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 1, 'k1', 'series', '2026-08-16 09:00:00')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 2, 'k2', 'series', '2026-08-16 09:00:00')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 3, 'k3', 'series', '2026-08-16 09:00:00')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (1, 4, 'k4', 'series', '2026-08-16 09:00:00')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (2, 5, 'k5', 'series', '2026-08-16 09:00:00')`,
	}

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("%s: %w", stmt, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

// The happy path, both artwork roles. A poster and a backdrop are two different
// columns and therefore two different halves of the OR, and a test that only
// ever asked for a poster would pass with the backdrop arm deleted.
func TestLookupImageAssetResolvesBothArtworkRoles(t *testing.T) {
	s := newTestStore(t)
	seedImageAssetCorpus(t, s)

	for _, tc := range []struct {
		name string
		key  string
		want int64
	}{
		{"poster", "1111111111111111", 1},
		{"backdrop", "2222222222222222", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := s.LookupImageAsset(t.Context(), OwnerScope(1), tc.key)
			if err != nil {
				t.Fatalf("LookupImageAsset: %v", err)
			}
			if !ok {
				t.Fatalf("asset %s is owned by a live, visible work and was not found", tc.key)
			}
			if got.ID != tc.want {
				t.Errorf("id = %d, want %d", got.ID, tc.want)
			}
			if got.CacheKey != tc.key {
				t.Errorf("cache_key = %q, want %q", got.CacheKey, tc.key)
			}
			if !got.Format.Valid || got.Format.String != ImageFormatJPEG {
				t.Errorf("format = %v, want %q — the Content-Type is derived from this "+
					"column and from nothing else", got.Format, ImageFormatJPEG)
			}
			if got.State != ImageStateReady {
				t.Errorf("state = %q, want %q", got.State, ImageStateReady)
			}
		})
	}
}

// A pending row is FOUND, with a NULL format. Not-found and no-bytes-yet are
// different answers: §4.4.1's cold start is an install where most assets are in
// exactly this state, and collapsing it into not-found would make the serving
// path unable to tell "we have not fetched this" from "there is no such image".
func TestLookupImageAssetFindsAPendingRowWithNoFormat(t *testing.T) {
	s := newTestStore(t)
	seedImageAssetCorpus(t, s)

	got, ok, err := s.LookupImageAsset(t.Context(), OwnerScope(1), "3333333333333333")
	if err != nil {
		t.Fatalf("LookupImageAsset: %v", err)
	}
	if !ok {
		t.Fatal("a pending asset on a visible work was reported not-found")
	}
	if got.Format.Valid {
		t.Errorf("format = %q on a pending row; the column's way of saying "+
			"'no encoded bytes yet' is SQL NULL (migration 00008)", got.Format.String)
	}
	if got.State == ImageStateReady {
		t.Error("a pending row reported state ready")
	}
}

// The three ways an asset is invisible, all answering the SAME not-found, so
// nothing on this read distinguishes "no such key" from "not yours".
func TestLookupImageAssetHidesWhatTheCallerMayNotSee(t *testing.T) {
	s := newTestStore(t)
	seedImageAssetCorpus(t, s)

	for _, tc := range []struct {
		name  string
		scope Scope
		key   string
	}{
		// A key no row carries.
		{"no such asset", OwnerScope(1), "0000000000000000"},
		// Owned only by a tombstoned work. A soft-deleted item's artwork is
		// not the item's artwork any more.
		{"owner is tombstoned", OwnerScope(1), "4444444444444444"},
		// Owned, live, and outside the caller's instance set.
		{"owner is out of scope", Scope{UserID: 1, InstanceIDs: []int64{1}}, "5555555555555555"},
		// No work points at it. There is no owning item to authorize against,
		// so there is no way to say yes.
		{"orphaned asset", OwnerScope(1), "6666666666666666"},
		// The empty visible set fails closed: it must mean nothing, never
		// everything.
		{"empty scope sees nothing", Scope{UserID: 2}, "1111111111111111"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok, err := s.LookupImageAsset(t.Context(), tc.scope, tc.key)
			if err != nil {
				t.Fatalf("LookupImageAsset: %v", err)
			}
			if ok {
				t.Errorf("%s resolved; it must be indistinguishable from a key that "+
					"names no row (security.md §4)", tc.name)
			}
		})
	}

	// The positive control for the scope arm: instance 2's scope DOES see 5, so
	// the refusal above is the scope filtering and not the fixture being empty.
	if _, ok, err := s.LookupImageAsset(
		t.Context(), Scope{UserID: 1, InstanceIDs: []int64{2}}, "5555555555555555",
	); err != nil || !ok {
		t.Fatalf("instance 2's own scope cannot see its own asset: ok=%v err=%v; the "+
			"out-of-scope assertion above is then vacuous", ok, err)
	}
}

// ⚠️ THE MUTATION, not the parameter. A scope a query accepts and never filters
// on is indistinguishable from no scope at all, so these arms DELETE the
// predicate that does the filtering from the statement that ships and assert
// that the hidden row appears. Threading the argument and checking that the
// argument arrived would pass with the WHERE clause gone.
//
// The asset id on the browse response is a property of a row the caller can
// already see, and this is what makes that true rather than asserted: the same
// predicate hides the row and its asset, and breaking it un-hides both.
func TestLookupImageAssetScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedImageAssetCorpus(t, s)

	// Instance 1's scope must not see instance 2's asset, and no scope at all
	// may see the tombstoned work's.
	scope := Scope{UserID: 1, InstanceIDs: []int64{1}}
	scopePred, _ := scope.workVisibilityPredicate("w.id")

	for _, tc := range []struct {
		name string
		key  string
		// mutate returns the broken statement and the arguments it now takes.
		mutate func(query string, args []any) (string, []any)
	}{
		{
			// The access scope itself: the service_item_link EXISTS goes away
			// and the out-of-scope asset becomes visible.
			name: "the access-scope predicate",
			key:  "5555555555555555",
			mutate: func(query string, args []any) (string, []any) {
				return strings.Replace(query, scopePred, "1=1", 1), args[:1]
			},
		},
		{
			// The tombstone predicate: a soft-deleted work's artwork is not the
			// item's artwork any more, and `deleted_at IS NULL` is the only
			// thing saying so.
			name: "the tombstone predicate",
			key:  "4444444444444444",
			mutate: func(query string, args []any) (string, []any) {
				return strings.Replace(query, "AND w.deleted_at IS NULL", "AND 1=1", 1), args
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query, args := imageAssetSQL(scope)
			args[0] = tc.key

			shipped, err := rowExists(t, s, query, args)
			if err != nil {
				t.Fatalf("shipped statement: %v", err)
			}
			if shipped {
				t.Fatal("the SHIPPED statement already returns this asset, so the " +
					"mutation below has nothing to prove")
			}

			broken, brokenArgs := tc.mutate(query, args)
			if broken == query {
				t.Fatalf("the shipped statement no longer contains what this arm "+
					"mutates, so it asserts against nothing:\n%s", query)
			}
			leaked, err := rowExists(t, s, broken, brokenArgs)
			if err != nil {
				t.Fatalf("mutated statement: %v", err)
			}
			if !leaked {
				t.Errorf("removing %s changed nothing, so it is not what hides this "+
					"asset and the assertions in this file are testing a proxy", tc.name)
			} else {
				t.Logf("MUTATION CONFIRMED: with %s removed the statement returns the "+
					"hidden asset", tc.name)
			}
		})
	}
}

func rowExists(t *testing.T, s *Store, query string, args []any) (bool, error) {
	t.Helper()
	rows, err := s.DB().Read().QueryContext(t.Context(), query, args...)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	got := rows.Next()
	return got, rows.Err()
}

// A malformed key never reaches SQL and never reaches a filesystem path. The
// route's whole input is one path segment, so this is a safety check rather than
// a formatting one.
func TestLookupImageAssetRefusesAKeyThatIsNotOne(t *testing.T) {
	s := newTestStore(t)
	seedImageAssetCorpus(t, s)

	for _, key := range []string{
		"",
		"../../../etc/passwd",
		"1111111111111111/../../secret",
		"1111111111111111 ",
		"1111111111111111%00",
		"111111111111111",   // fifteen
		"11111111111111111", // seventeen
		"1111111111111111'",
		"111111111111111G",
		"AAAAAAAAAAAAAAAA", // uppercase hex is not what sha256 hex renders
		"%2e%2e%2f",
	} {
		if ValidImageCacheKey(key) {
			t.Errorf("ValidImageCacheKey(%q) = true", key)
		}
		if _, ok, err := s.LookupImageAsset(t.Context(), OwnerScope(1), key); ok || err != nil {
			t.Errorf("LookupImageAsset(%q) = ok %v, err %v; want not-found and no error",
				key, ok, err)
		}
	}
	if !ValidImageCacheKey("1111111111111111") {
		t.Error("ValidImageCacheKey rejected a well-formed key, so every rejection " +
			"above is vacuous")
	}
}

// The collision refusal. Two rows sharing one cache_key means two DIFFERENT
// source URLs whose truncated SHA-256 collided, and serving either would serve
// one item's artwork under the other item's authorization decision.
//
// The schema permits this — migration 00010's index is deliberately not UNIQUE,
// because the writer that would owe the constraint is not built — so the read
// refuses instead, and this is that refusal executed rather than asserted.
func TestLookupImageAssetRefusesAnAmbiguousCacheKey(t *testing.T) {
	s := newTestStore(t)
	seedImageAssetCorpus(t, s)

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, stmt := range []string{
			// A second row on the SAME key, a different source_url, owned by a
			// live and visible work of its own.
			`INSERT INTO image_asset (id, source_url, origin_class, role, cache_key, format, state)
			   VALUES (7, 'http://elsewhere.example/other', 'provider', 'poster',
			           '1111111111111111', 'jpeg', 'ready')`,
			`INSERT INTO work (id, kind, title, sort_title, normalized_title, poster_asset_id)
			   VALUES (7, 'comic', 'Collided', 'collided', 'collided', 7)`,
			`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			   VALUES (1, 7, 'k7', 'series', '2026-08-16 09:00:00')`,
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("%s: %w", stmt, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the collision: %v", err)
	}

	_, ok, err := s.LookupImageAsset(t.Context(), OwnerScope(1), "1111111111111111")
	if ok {
		t.Error("an ambiguous cache key resolved; one of two items' artwork was about " +
			"to be served under the other's authorization")
	}
	if !errors.Is(err, ErrImageAssetAmbiguous) {
		t.Fatalf("err = %v, want ErrImageAssetAmbiguous", err)
	}
	// The error names the rows, so the collision is findable. It must NOT name
	// the key: that is caller-supplied text.
	if !strings.Contains(err.Error(), "1 and 7") {
		t.Errorf("the error does not name both rows: %v", err)
	}
	if strings.Contains(err.Error(), "1111111111111111") {
		t.Errorf("the error echoes the caller-supplied key: %v", err)
	}
}

// ── the plan ────────────────────────────────────────────────────────────────

func imageAssetPlan(t *testing.T, q *sql.DB, scope Scope) string {
	t.Helper()
	query, args := imageAssetSQL(scope)
	args[0] = "1111111111111111"
	plan, err := db.QueryPlan(t.Context(), q, query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// imageAssetPlanFaults is the judgement both the assertion and the mutation run,
// so the guard and the thing it guards cannot drift.
func imageAssetPlanFaults(plan string) []string {
	var faults []string
	if !strings.Contains(plan, "ix_img_cache_key (cache_key=?)") {
		faults = append(faults, "does not seek ix_img_cache_key on the route key")
	}
	if !strings.Contains(plan, "ix_work_poster (poster_asset_id=?)") {
		faults = append(faults, "does not seek ix_work_poster for the poster arm")
	}
	if !strings.Contains(plan, "ix_work_backdrop (backdrop_asset_id=?)") {
		faults = append(faults, "does not seek ix_work_backdrop for the backdrop arm")
	}
	// The OR is only two seeks because SQLite applied MULTI-INDEX OR. Its
	// absence is the whole fallback, and the fallback is a walk of `work`.
	if !strings.Contains(plan, "MULTI-INDEX OR") {
		faults = append(faults, "SQLite declined MULTI-INDEX OR, so the owner EXISTS is no "+
			"longer two seeks")
	}
	// `SCAN w USING INDEX …` is a walk of every work wearing an index name. It
	// is what the plan degrades to, so it is named rather than left to the
	// absence checks above.
	if strings.Contains(plan, "SCAN w ") || strings.HasSuffix(plan, "SCAN w") {
		faults = append(faults, "degraded to a walk of the work table")
	}
	if strings.Contains(plan, "SCAN ia") {
		faults = append(faults, "degraded to a scan of every image asset")
	}
	return faults
}

// THE PLAN THAT SHIPS, on both scope shapes. The owner's scope is v0.1's, and
// the partial scope is what a multi-user install runs — the second must not
// displace either seek with the service_item_link EXISTS.
func TestLookupImageAssetPlanIsThreeSeeks(t *testing.T) {
	s := newTestStore(t)
	seedImageAssetCorpus(t, s)

	owner := imageAssetPlan(t, s.DB().Read(), OwnerScope(1))
	if faults := imageAssetPlanFaults(owner); len(faults) > 0 {
		t.Errorf("the owner plan is wrong:\n  plan: %s\n  %s", owner, strings.Join(faults, "\n  "))
	}

	scoped := imageAssetPlan(t, s.DB().Read(), Scope{UserID: 1, InstanceIDs: []int64{1, 2}})
	if faults := imageAssetPlanFaults(scoped); len(faults) > 0 {
		t.Errorf("the scoped plan is wrong:\n  plan: %s\n  %s", scoped, strings.Join(faults, "\n  "))
	}
	if !strings.Contains(scoped, "ix_sil_work (work_id=?)") {
		t.Errorf("the scope EXISTS is not a seek on ix_sil_work: %s", scoped)
	}
	if strings.Contains(scoped, "SCAN sil") {
		t.Errorf("the scope EXISTS degraded to a scan of every service link: %s", scoped)
	}
}

// FIRING THE GUARD. Three arms, one per index, each running the SAME
// imageAssetPlanFaults the assertion above runs.
//
// ⚠️ THE DROP GOES THROUGH dropIndexesAndConfirm (store_test.go), NEVER A BARE
// DROP, AND THE PLAN IS MEASURED ON THE READ POOL — the connection the shipped
// read uses, and the one the assertion above measures. Those are one decision,
// not two: `EXPLAIN QUERY PLAN` on the read pool is STALE after a DROP INDEX
// issued on the write connection — three consecutive runs on this tree, all
// re-printing the pre-drop plan — so an arm that dropped and then EXPLAINed on
// a reader without clearing that would report the HEALTHY plan and pass with
// the index gone, which is precisely the vacuous green this arm exists to rule
// out. The helper's sqlite_master read is both the proof the drop landed and
// the unrelated read that clears the staleness; its doc carries the
// measurement and the caveat that the cure holds only for a single-goroutine
// driver, which these arms are.
//
// ⚠️ These arms used to EXPLAIN on the WRITE connection, which sidesteps the
// staleness rather than clearing it: correct, but a second mechanism for a
// hazard the package already had one for, and it left the pin above and the
// arms below measuring two different connections. The `before` plan below is
// taken on the read pool, which PRIMES it — priming is what arms the staleness
// trap — so these arms now exercise the cure rather than avoid the hazard.
func TestLookupImageAssetPlanGuardFires(t *testing.T) {
	for _, index := range []string{"ix_img_cache_key", "ix_work_poster", "ix_work_backdrop"} {
		t.Run(index+" dropped", func(t *testing.T) {
			s := newTestStore(t)
			seedImageAssetCorpus(t, s)

			// The pre-drop measurement, on the same connection the post-drop
			// one uses, so the arm compares like with like.
			before := imageAssetPlan(t, s.DB().Read(), OwnerScope(1))
			if faults := imageAssetPlanFaults(before); len(faults) > 0 {
				t.Fatalf("the plan was already faulty before the drop:\n  %s", before)
			}

			dropIndexesAndConfirm(t, s, index)

			after := imageAssetPlan(t, s.DB().Read(), OwnerScope(1))
			faults := imageAssetPlanFaults(after)
			if len(faults) == 0 {
				t.Fatalf("the guard passed a plan with %s dropped:\n  %s", index, after)
			}
			t.Logf("guard fired with %s dropped:\n  plan:   %s\n  faults: %v",
				index, after, faults)
		})
	}
}
