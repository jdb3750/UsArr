package store

import (
	"context"
	"fmt"
)

// WorkServiceLink is ONE live `service_item_link` row, reduced to the three
// columns that identify which upstream item a work came from.
//
// It is deliberately not the whole row. Everything else on that table —
// remote_path, monitored, quality_profile_id, the container columns, has_file —
// is either sync state or a container-membership input, and a caller that wants
// one of those is asking a different question and should get a read that says
// so. This struct is the answer to "which remote items is this work?" and
// nothing more.
type WorkServiceLink struct {
	// ServiceInstanceID names WHICH configured service the remote id belongs
	// to. It is in the result rather than implied because remote ids are only
	// unique per instance — `ux_sil` is (service_instance_id, remote_kind,
	// remote_id), all three columns — so a remote id handed on without its
	// instance is not addressable.
	ServiceInstanceID int64

	// RemoteKind is the upstream's own item kind, verbatim: series, episode,
	// movie, album, track, book, author.
	RemoteKind string

	// RemoteID is the upstream's own id for the item, RETURNED AS THE TEXT IT
	// IS STORED AS AND NEVER PARSED HERE. The column is TEXT because the
	// ecosystem's ids are not one type — a Sonarr series id is an integer, a
	// MusicBrainz id is a UUID, an Audiobookshelf id is an opaque string — and
	// the store has no way to know which shape any given caller expects.
	//
	// ⚠️ THE VALUE THAT WILL NOT PARSE IS INFORMATION, NOT AN ERROR. A caller
	// that wants an integer parses it AT ITS OWN BOUNDARY, where it also knows
	// what a failure means: a corrupted row, or a link from some other service
	// handed to a fetcher that only speaks one. Parsing here would convert both
	// of those into either an error this read has no standing to raise or a
	// zero it has no standing to substitute, and the caller would lose the
	// ability to tell them apart.
	RemoteID string
}

// workServiceLinksSQL renders the ListWorkServiceLinks statement and its
// arguments.
//
// It is a function rather than a literal inlined below so the query-plan test
// can EXPLAIN THE STATEMENT THAT SHIPS (docs/DEVELOPMENT.md §11 rule 1: a guard
// that asserts against a hand-copied lookalike is probing a proxy).
//
// THE SCOPE PREDICATE IS instancePredicate, NOT workVisibilityPredicate, and the
// difference is the whole security argument. workVisibilityPredicate exists
// because `work` carries no instance column, so "can this user see this work?"
// has to reach the link table through an EXISTS (recent.go). This read's rows
// ARE link rows: they carry `service_instance_id` themselves, so the scope is a
// direct comparison on the column being returned — the same shape
// WorkCountsByInstance uses over the same table.
//
// Using the work-level predicate here would be a real leak rather than a style
// difference, and it was MEASURED rather than reasoned about: swapped in behind
// a distinct alias, TestWorkServiceLinksScopeExcludesOtherInstances goes red on
// work 5 with `a link on an instance outside the caller's scope came back:
// {ServiceInstanceID:3 …}`. It asks whether the work is visible on ANY instance
// in scope, and having answered yes it hands back EVERY link on the work,
// including links on instances the caller cannot see. One shared work replicated
// by two instances lets a user entitled to one of them enumerate the other's
// remote ids — an existence oracle over the configured stack of exactly the kind
// store.go's rule 2 and ARCHITECTURE §14 name. The three branches are
// instancePredicate's, unchanged: `1=1` for the owner, `1=0` for an empty
// visible set (fail closed: no visible instances means no rows, never all
// rows), and the IN seek otherwise.
//
// 🔍 A SECOND, DUMBER REASON THE WORK-LEVEL PREDICATE CANNOT BE USED HERE, also
// measured: workVisibilityPredicate hard-codes `sil` as the alias of its inner
// service_item_link. Rendered over a query whose OUTER table is already aliased
// `sil`, the inner scope shadows the outer and the correlation collapses to
// `sil.work_id = sil.work_id` — trivially true, so the EXISTS degenerates into
// "does the caller see any live link anywhere" and every link on every work
// comes back. It fails loudly against the arms in worklinks_test.go, but it
// fails as a silent full leak rather than as a syntax error, which is worth
// knowing before anyone reaches for that helper over this table.
//
// `deleted_at IS NULL` is in the query AND in ix_sil_work's own partial
// predicate, and that is not belt-and-braces — MEASURED, by deleting the WHERE
// clause and re-running the plan guard. It costs BOTH halves at once: the read
// starts returning links inside their 7-day tombstone window (items the upstream
// no longer reports), AND the plan collapses from
// `SEARCH sil USING INDEX ix_sil_work (work_id=?)` to `SCAN sil`, because SQLite
// may only use a PARTIAL index when the statement's WHERE clause implies the
// index's predicate. Dropping the filter does not degrade the read to
// "index-then-filter"; it takes the index away entirely.
//
// THE WORK'S OWN TOMBSTONE IS DELIBERATELY NOT CONSULTED. This read takes a work
// id the caller already holds and answers what links point at it; it does not
// re-adjudicate whether the work is live, which is a different question with a
// different answer shape (`work.deleted_at`) that the caller can ask directly.
// Joining `work` to filter on it would also put a second table in a plan that is
// otherwise one index seek, to enforce a condition nobody asked for.
//
// NO ORDER BY, ON PURPOSE. A work legitimately has more than one live link —
// two instances replicating it, or the case WorkCountsByInstance's doc names,
// two links on ONE instance pointing at one work after a §6.4 tier-1 merge. The
// caller has decided not to hide that behind a singular API, so there is no
// precedence rule to encode and an ORDER BY here could only look like one.
// Callers must treat the result as a SET, and every assertion in
// worklinks_test.go compares sorted keys rather than row order for that reason.
//
// THE MEASURED PLAN, on this engine (github.com/ncruces/go-sqlite3, SQLite
// 3.53.4), no ANALYZE, IDENTICAL on both scope shapes — the owner's `1=1` and a
// narrowed `service_instance_id IN (?,?)`:
//
//	SEARCH sil USING INDEX ix_sil_work (work_id=?)
//
// One seek, no sort, and the instance filter rides it rather than displacing it
// with a seek on ux_sil. TestWorkServiceLinksPlanGuardFires drops ix_sil_work
// through dropIndexesAndConfirm and watches the same judgement catch the
// fallback, which is `SCAN sil` — a walk of every link on every instance.
func workServiceLinksSQL(scope Scope) (string, []any) {
	scopePred, scopeArgs := scope.instancePredicate("sil.service_instance_id")

	args := make([]any, 0, 1+len(scopeArgs))
	args = append(args, int64(0)) // the work id; filled by the caller
	args = append(args, scopeArgs...)

	return `
		SELECT sil.service_instance_id, sil.remote_kind, sil.remote_id
		  FROM service_item_link sil
		 WHERE sil.work_id = ?
		   AND sil.deleted_at IS NULL
		   AND ` + scopePred, args
}

// ListWorkServiceLinks returns EVERY live service link on one work, within the
// caller's access scope.
//
// It is the reverse of the only direction the store had. Every resolver here
// runs remote_id → work_id — catalogue.go's applyOneItem, credits.go's
// ApplyCredits, files.go's ApplyFiles all open with the same
// `SELECT work_id FROM service_item_link WHERE service_instance_id = ? AND
// remote_kind = ? AND remote_id = ?`, a seek on ux_sil. This is work_id →
// (instance, kind, remote id), a seek on ix_sil_work, and it exists for callers
// holding a local work that need to ask an upstream about it.
//
// ⚠️ IT RETURNS ALL OF THEM, NOT "THE" LINK, AND THAT IS THE CONTRACT. Zero,
// one and many are all ordinary answers. Nothing here picks a primary, filters
// to one remote_kind, or orders the result, because there is no rule in this
// schema that would make such a pick correct — see workServiceLinksSQL. A
// caller that can only act on one link is the thing that must decide which,
// visibly, with both in hand.
//
// AN EMPTY RESULT IS NOT AN ERROR and covers three different facts the store
// cannot separate: no such work, a work with no links at all (the credit pass
// writes 'person' works that never get one), and a work whose every link is on
// an instance outside the caller's scope. The third is merged with the other two
// on purpose — a distinguishable "exists but not yours" is the existence oracle
// ARCHITECTURE §14 is written against, and work ids are enumerable by design
// (§4.5 ships them to every client).
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL, both halves. A scope a query accepts
// and never filters on is indistinguishable from no scope at all, which is why
// TestWorkServiceLinksScopeActuallyFilters breaks the predicate and watches the
// assertion catch it rather than trusting that the parameter is threaded.
func (s *Store) ListWorkServiceLinks(
	ctx context.Context, scope Scope, workID int64,
) ([]WorkServiceLink, error) {
	query, args := workServiceLinksSQL(scope)
	args[0] = workID

	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read service links for work %d: %w", workID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []WorkServiceLink
	for rows.Next() {
		var l WorkServiceLink
		if err := rows.Scan(&l.ServiceInstanceID, &l.RemoteKind, &l.RemoteID); err != nil {
			return nil, fmt.Errorf("read service links for work %d: scan: %w", workID, err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read service links for work %d: %w", workID, err)
	}
	return out, nil
}
