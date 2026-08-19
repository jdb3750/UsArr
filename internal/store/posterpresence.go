package store

import (
	"context"
	"fmt"
)

// The one read the cover pass needs: WHICH ITEMS ALREADY HAVE ARTWORK.
//
// It is a separate file from imagewrite.go (the poster write) and from
// imageassets.go (the serving read) because it answers neither of their
// questions. imageassets.go asks "may this caller have these bytes"; this asks
// "is there any point fetching them again", which is a replication question and
// takes no Scope for catalogue.go's reason — see PosteredItems.

// PosteredItems returns the upstream items on ONE instance whose work already
// carries a poster.
//
// # Why the whole set in one query, rather than a probe per item
//
// Its only caller is internal/libsync's cover pass, which runs once per import
// over every item that reached a committed batch. On a re-import of a library
// UsArr has already covered, EVERY item is in this set, so the per-item shape
// would be one round trip per book to learn that no book needs one. One query
// answers it for the whole instance.
//
// The allocation is bounded by the number of works on this instance that have a
// poster, which is at most the number of items the import is already holding in
// streamAndApply's budgeted slice — two short strings per row against that
// slice's three. If that ever stops being affordable the fix is the same one
// that slice's comment names: a temp table, not a smaller answer.
//
// # It is keyed by the ITEM and not by the work, deliberately
//
// The caller cannot use a work id — PutPosterAsset re-resolves the work inside
// its own write transaction precisely because a work id read in one transaction
// and used in another may name a different row by the time it is used
// (imagewrite.go's PosterAsset header). A read that handed back work ids would
// invite exactly that, so it hands back the same (kind, id) pair the write takes.
//
// # What "already has a poster" means, stated because it is broader than it looks
//
// The predicate is `work.poster_asset_id IS NOT NULL`, so a work whose poster
// was recorded from a DIFFERENT service instance counts as covered and its item
// here is not re-fetched. That is intended: the column holds one poster per
// work, and a second source re-fetching over the top would be a per-import
// coin-toss about whose artwork wins rather than an improvement.
//
// It is a replication read and takes no Scope, on the terms catalogue.go's
// header sets for the whole import path.
func (s *Store) PosteredItems(ctx context.Context, instanceID int64) ([]WorkServiceLink, error) {
	// The join to `work` carries two predicates that look redundant and are not:
	// `sil.deleted_at IS NULL` says the LINK is live, and `w.deleted_at IS NULL`
	// says the WORK is — a link can outlive a tombstoned work, and a tombstoned
	// work's stale poster must not suppress a fetch for a live one.
	//
	// `poster_asset_id IS NOT NULL` is also ix_work_poster's partial predicate
	// (migration 00010), which is what lets the planner reach this set through
	// the index rather than by scanning every work.
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT sil.service_instance_id, sil.remote_kind, sil.remote_id
		  FROM service_item_link sil
		  JOIN work w ON w.id = sil.work_id
		 WHERE sil.service_instance_id = ?
		   AND sil.deleted_at IS NULL
		   AND w.deleted_at IS NULL
		   AND w.poster_asset_id IS NOT NULL`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("read postered items for service_instance %d: %w", instanceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []WorkServiceLink
	for rows.Next() {
		var l WorkServiceLink
		if err := rows.Scan(&l.ServiceInstanceID, &l.RemoteKind, &l.RemoteID); err != nil {
			return nil, fmt.Errorf("read postered items for service_instance %d: scan: %w", instanceID, err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read postered items for service_instance %d: %w", instanceID, err)
	}
	return out, nil
}
