package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/db"
)

// The remote ids the fixture stores. They are named rather than inlined because
// two assertions below are about their BYTES, not about which row came back.
const (
	// An ordinary *Arr integer id, as TEXT.
	linkIDInteger = "1234"
	// A UUID. Nothing in this ecosystem agrees on an id type — MusicBrainz is
	// UUIDs, Audiobookshelf is an opaque string — and this read must carry
	// either without an opinion.
	linkIDUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	// ⚠️ THE ONE THAT MUST NOT PARSE. A caller that parses at its own boundary
	// needs to SEE this value to classify it — a corrupted row, or a link from
	// the wrong service handed to a fetcher that only speaks one. If the store
	// ever starts parsing, this is the row that turns into an error nobody
	// asked it to raise or a zero nobody asked it to substitute.
	linkIDUnparseable = "not-a-number/0007 "
)

// seedWorkLinkCorpus builds the link topology every assertion in this file
// reads. Direct SQL rather than ApplyCatalogueBatch: the shapes below include a
// tombstoned link and an out-of-scope instance, which are states the write path
// reaches only through a sequence of syncs, and the read under test does not
// care how they got there.
//
// Foreign keys are ON (internal/db/sqlite.go), so the instances and the works
// are real rows.
//
// The topology, and what each work exists to prove:
//
//	work 1 — TWO live links, on instance 1 and instance 2. Multiplicity across
//	         instances. Also the byte-identity fixture: one UUID, one
//	         unparseable id.
//	work 2 — TWO live links on ONE instance, different remote_kind. The case
//	         WorkCountsByInstance's doc names, and the one a "there is only ever
//	         one link" tidy-up would break silently.
//	work 3 — one live link on instance 1, one TOMBSTONED link on instance 2.
//	work 4 — one live link on instance 3 ONLY, which no narrowed scope below can
//	         see. The security arm.
//	work 5 — a live link on instance 1 AND one on instance 3. The leak arm: a
//	         work the narrowed caller CAN see, carrying a link it cannot.
//	work 6 — no links at all, like the 'person' works the credit pass writes.
func seedWorkLinkCorpus(t *testing.T, s *Store) {
	t.Helper()

	work := func(id int64, title string) string {
		return fmt.Sprintf(
			`INSERT INTO work (id, kind, title, sort_title, normalized_title)
			   VALUES (%d, 'series', '%s', '%s', '%s')`,
			id, title, strings.ToLower(title), strings.ToLower(title))
	}
	link := func(id, instance, workID int64, kind, remoteID, deletedAt string) string {
		del := "NULL"
		if deletedAt != "" {
			del = "'" + deletedAt + "'"
		}
		return fmt.Sprintf(
			`INSERT INTO service_item_link
			   (id, service_instance_id, work_id, remote_id, remote_kind, synced_at, deleted_at)
			 VALUES (%d, %d, %d, '%s', '%s', '2026-08-16 09:00:00', %s)`,
			id, instance, workID, remoteID, kind, del)
	}

	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'sonarr', 'library', 'Sonarr', 'http://sonarr.example', X'00')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (2, 'radarr', 'library', 'Radarr', 'http://radarr.example', X'00')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (3, 'kavita', 'library', 'Kavita', 'http://kavita.example', X'00')`,

		work(1, "Berserk"),
		work(2, "Vinland Saga"),
		work(3, "Frieren"),
		work(4, "Pluto"),
		work(5, "Monster"),
		work(6, "Nobody"),

		link(11, 1, 1, "series", linkIDUUID, ""),
		link(12, 2, 1, "movie", linkIDUnparseable, ""),

		link(21, 1, 2, "series", linkIDInteger, ""),
		link(22, 1, 2, "episode", "5678", ""),

		link(31, 1, 3, "series", "live-31", ""),
		link(32, 2, 3, "movie", "dead-32", "2026-08-14 09:00:00"),

		link(41, 3, 4, "book", "hidden-41", ""),

		link(51, 1, 5, "series", "visible-51", ""),
		link(52, 3, 5, "book", "hidden-52", ""),
	}

	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("%s: %w", stmt, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the work-link corpus: %v", err)
	}
}

// key renders one row for order-insensitive comparison. The read ships no
// ORDER BY on purpose, so every assertion here compares SETS — an assertion
// that depended on row order would be pinning a plan accident as a contract.
func (l WorkServiceLink) key() string {
	return fmt.Sprintf("%d/%s/%s", l.ServiceInstanceID, l.RemoteKind, l.RemoteID)
}

func linkKeys(links []WorkServiceLink) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		out = append(out, l.key())
	}
	sort.Strings(out)
	return out
}

func listLinks(t *testing.T, s *Store, scope Scope, workID int64) []WorkServiceLink {
	t.Helper()
	got, err := s.ListWorkServiceLinks(t.Context(), scope, workID)
	if err != nil {
		t.Fatalf("ListWorkServiceLinks(work %d): %v", workID, err)
	}
	return got
}

func assertLinks(t *testing.T, got []WorkServiceLink, want ...string) {
	t.Helper()
	sort.Strings(want)
	gotKeys := linkKeys(got)
	if strings.Join(gotKeys, ",") != strings.Join(want, ",") {
		t.Errorf("links = %v, want %v", gotKeys, want)
	}
}

// ⚠️ THE MULTIPLICITY REQUIREMENT, NAMED SO A LATER TIDY-UP CANNOT QUIETLY TAKE
// IT. A work legitimately has more than one live link, and the caller asked to
// see all of them and decide visibly rather than have the store pick. Both
// shapes are here: two instances on one work, and TWO LINKS ON ONE INSTANCE,
// which is the shape WorkCountsByInstance's doc contemplates and the one a
// `LIMIT 1`, a `DISTINCT`, or a "primary link" concept would erase without
// failing anything else in this file.
func TestWorkServiceLinksReturnsEveryLiveLink(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)
	owner := OwnerScope(1)

	across := listLinks(t, s, owner, 1)
	if len(across) != 2 {
		t.Errorf("work 1 has two live links on two instances; the read returned %d: %v",
			len(across), across)
	}
	assertLinks(t, across, "1/series/"+linkIDUUID, "2/movie/"+linkIDUnparseable)

	within := listLinks(t, s, owner, 2)
	if len(within) != 2 {
		t.Errorf("work 2 has two live links on ONE instance; the read returned %d: %v. "+
			"A read that collapses these has decided which link is 'the' link, which is "+
			"exactly the decision this API refuses to make.", len(within), within)
	}
	assertLinks(t, within, "1/series/"+linkIDInteger, "1/episode/5678")

	// Zero is an ordinary answer, not an error. The credit pass writes 'person'
	// works with no link at all.
	if got := listLinks(t, s, owner, 6); len(got) != 0 {
		t.Errorf("work 6 has no links; got %v", got)
	}
	if got := listLinks(t, s, owner, 999); len(got) != 0 {
		t.Errorf("a work id that does not exist returned %v, want no rows and no error", got)
	}
}

// The tombstone. A link inside its 7-day window is an item the upstream no
// longer reports, and every live read in this package filters on it.
func TestWorkServiceLinksExcludesTombstonedLinks(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)

	got := listLinks(t, s, OwnerScope(1), 3)
	assertLinks(t, got, "1/series/live-31")
	for _, l := range got {
		if l.RemoteID == "dead-32" {
			t.Errorf("a soft-deleted link came back: %+v", l)
		}
	}

	// The exclusion is not vacuous: the row is really there.
	var n int
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT count(*) FROM service_item_link WHERE work_id = 3 AND deleted_at IS NOT NULL`,
	).Scan(&n); err != nil {
		t.Fatalf("counting tombstoned links: %v", err)
	}
	if n != 1 {
		t.Fatalf("the fixture has %d tombstoned links on work 3, want 1: the assertion "+
			"above is testing nothing", n)
	}
}

// ⚠️ THE SECURITY ASSERTION. remote_id comes back verbatim, so a link on an
// instance the caller cannot see would hand that caller a directly usable
// address on a service they are not entitled to — and, by its mere presence,
// the fact that the instance holds this work at all. That is the existence
// oracle store.go's rule 2 and ARCHITECTURE §14 name.
//
// The work-5 arm is the one that matters most: the caller CAN see work 5, on
// instance 1. A scope predicate written at the WORK level — the EXISTS shape
// recent.go uses, which is the wrong one for this table — would answer "yes,
// visible" and then return instance 3's link along with instance 1's. This arm
// is what separates the two predicates.
func TestWorkServiceLinksScopeExcludesOtherInstances(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)
	narrow := Scope{UserID: 2, InstanceIDs: []int64{1, 2}}

	// The owner sees instance 3; the narrowed caller must not. Establishing
	// both halves is what makes the absence below a filter rather than an
	// empty fixture.
	assertLinks(t, listLinks(t, s, OwnerScope(1), 4), "3/book/hidden-41")
	if got := listLinks(t, s, narrow, 4); len(got) != 0 {
		t.Errorf("a work reachable only from an out-of-scope instance returned %v", got)
	}

	assertLinks(t, listLinks(t, s, OwnerScope(1), 5),
		"1/series/visible-51", "3/book/hidden-52")
	got := listLinks(t, s, narrow, 5)
	assertLinks(t, got, "1/series/visible-51")
	for _, l := range got {
		if l.ServiceInstanceID == 3 {
			t.Errorf("a link on an instance outside the caller's scope came back: %+v. "+
				"The caller can see work 5 through instance 1; that must not make "+
				"instance 3's remote id readable.", l)
		}
	}

	// Fail closed. No visible instances means no rows, never all rows.
	if got := listLinks(t, s, Scope{UserID: 3}, 1); len(got) != 0 {
		t.Errorf("an empty scope saw %v, want no rows (fail closed)", got)
	}
}

// The scope parameter is threaded AND filtering. A scope a query accepts and
// never applies is indistinguishable from no scope at all, so this breaks the
// predicate the shipped statement renders and watches the assertion above catch
// it — rather than trusting that the parameter reaches the SQL.
func TestWorkServiceLinksScopeActuallyFilters(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)
	narrow := Scope{UserID: 2, InstanceIDs: []int64{1, 2}}

	query, args := workServiceLinksSQL(narrow)
	args[0] = int64(5)

	// The statement as it ships hides instance 3's link.
	if n := countRows(t, s, query, args); n != 1 {
		t.Fatalf("the shipped statement returned %d rows for work 5 under a narrowed "+
			"scope, want 1", n)
	}

	// With the scope clause removed — and NOTHING else changed — it does not.
	// If this still returns 1, the predicate is decoration.
	pred, _ := narrow.instancePredicate("sil.service_instance_id")
	broken := strings.Replace(query, "AND "+pred, "AND 1=1", 1)
	if broken == query {
		t.Fatalf("the scope predicate %q is not in the shipped statement, so this "+
			"mutation tests nothing:\n%s", pred, query)
	}
	if n := countRows(t, s, broken, args[:1]); n != 2 {
		t.Fatalf("with the scope predicate replaced by 1=1 the statement returned %d "+
			"rows, want 2. The predicate is not what hides instance 3's link, so the "+
			"scope assertions in this file are testing a proxy.", n)
	}
	t.Log("MUTATION CONFIRMED: removing the scope predicate leaks instance 3's link")
}

func countRows(t *testing.T, s *Store, query string, args []any) int {
	t.Helper()
	rows, err := s.DB().Read().QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return n
}

// remote_id is TEXT and comes back as the TEXT it was stored as. The caller
// parses at its own boundary, where a value that will not parse is information
// it can classify rather than an error this read raised on its behalf.
func TestWorkServiceLinksRemoteIDIsByteIdentical(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)

	byInstance := map[int64]WorkServiceLink{}
	for _, l := range listLinks(t, s, OwnerScope(1), 1) {
		byInstance[l.ServiceInstanceID] = l
	}

	for _, tc := range []struct {
		instance int64
		want     string
	}{
		{1, linkIDUUID},
		{2, linkIDUnparseable}, // trailing space and a slash, both load-bearing
	} {
		got, ok := byInstance[tc.instance]
		if !ok {
			t.Fatalf("no link came back for instance %d", tc.instance)
		}
		if got.RemoteID != tc.want {
			t.Errorf("remote_id from instance %d = %q, want %q byte for byte",
				tc.instance, got.RemoteID, tc.want)
		}
		if len(got.RemoteID) != len(tc.want) {
			t.Errorf("remote_id from instance %d is %d bytes, want %d: something "+
				"trimmed or normalised it", tc.instance, len(got.RemoteID), len(tc.want))
		}
	}

	// And an integer-shaped id is still TEXT, not a number that lost its
	// leading zeroes or its exact spelling on the way through.
	for _, l := range listLinks(t, s, OwnerScope(1), 2) {
		if l.RemoteKind == "series" && l.RemoteID != linkIDInteger {
			t.Errorf("remote_id = %q, want %q", l.RemoteID, linkIDInteger)
		}
	}
}

// ── the plan ────────────────────────────────────────────────────────────────

func workLinksPlan(t *testing.T, q *sql.DB, scope Scope) string {
	t.Helper()
	query, args := workServiceLinksSQL(scope)
	args[0] = int64(1)
	plan, err := db.QueryPlan(t.Context(), q, query, args...)
	if err != nil {
		t.Fatalf("QueryPlan: %v", err)
	}
	return strings.Join(plan, " | ")
}

// workLinksPlanFaults is the judgement both the assertion and the mutation run,
// so the guard and the thing it guards cannot drift.
func workLinksPlanFaults(plan string) []string {
	var faults []string
	if !strings.Contains(plan, "ix_sil_work (work_id=?)") {
		faults = append(faults, "does not seek ix_sil_work on the work id")
	}
	// The fallback. `SCAN sil` is a walk of every link on every instance, and
	// it is what the seek degrades to — named rather than left to the absence
	// check above, because a plan can wear a different index name and still be
	// a walk.
	if strings.Contains(plan, "SCAN sil") {
		faults = append(faults, "degraded to a scan of every service link")
	}
	// A read whose whole contract is "return the set" must not be paying for a
	// sort. Its presence would mean an ORDER BY crept in — see
	// workServiceLinksSQL on why there is none.
	if strings.Contains(plan, "TEMP B-TREE") {
		faults = append(faults, "acquired a sort; this read ships no ORDER BY")
	}
	return faults
}

// THE PLAN THAT SHIPS, on both scope shapes. The owner's scope is v0.1's; the
// narrowed scope is what a multi-user install runs, and its `IN (…)` on
// service_instance_id must ride the SAME ix_sil_work seek rather than displace
// it with a scan.
func TestWorkServiceLinksPlanSeeksIxSilWork(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)

	owner := workLinksPlan(t, s.DB().Read(), OwnerScope(1))
	if faults := workLinksPlanFaults(owner); len(faults) > 0 {
		t.Errorf("the owner plan is wrong:\n  plan: %s\n  %s", owner, strings.Join(faults, "\n  "))
	}
	t.Logf("owner plan:  %s", owner)

	scoped := workLinksPlan(t, s.DB().Read(), Scope{UserID: 1, InstanceIDs: []int64{1, 2}})
	if faults := workLinksPlanFaults(scoped); len(faults) > 0 {
		t.Errorf("the scoped plan is wrong:\n  plan: %s\n  %s", scoped, strings.Join(faults, "\n  "))
	}
	t.Logf("scoped plan: %s", scoped)
}

// FIRING THE GUARD.
//
// ⚠️ THE DROP GOES THROUGH dropIndexesAndConfirm (store_test.go), NEVER A BARE
// DROP, AND THE PLAN IS MEASURED ON THE READ POOL — the connection the shipped
// read uses, and the one the assertion above measures. `EXPLAIN QUERY PLAN` on
// the read pool is STALE after a DROP INDEX issued on the write connection, so
// an arm that dropped and then EXPLAINed on a reader without clearing that
// would re-print the HEALTHY plan and pass with the index gone. The `before`
// measurement below is taken on the read pool, which PRIMES it — priming is
// what arms the staleness trap — so this arm exercises the cure rather than
// avoiding the hazard.
func TestWorkServiceLinksPlanGuardFires(t *testing.T) {
	s := newTestStore(t)
	seedWorkLinkCorpus(t, s)

	before := workLinksPlan(t, s.DB().Read(), OwnerScope(1))
	if faults := workLinksPlanFaults(before); len(faults) > 0 {
		t.Fatalf("the plan was already faulty before the drop:\n  %s", before)
	}

	dropIndexesAndConfirm(t, s, "ix_sil_work")

	after := workLinksPlan(t, s.DB().Read(), OwnerScope(1))
	faults := workLinksPlanFaults(after)
	if len(faults) == 0 {
		t.Fatalf("the guard passed a plan with ix_sil_work dropped:\n  %s", after)
	}
	t.Logf("guard fired with ix_sil_work dropped:\n  plan:   %s\n  faults: %v", after, faults)
}
