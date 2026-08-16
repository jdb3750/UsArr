package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Indexer is one row of indexer_catalog: the local replica of one indexer a
// service has configured.
//
// IT IS AN ALLOWLIST, LIKE THE TABLE. Prowlarr's IndexerResource carries
// `fields[]` — passkeys, RSS keys, API keys, cookies — and there is deliberately
// no member here that could hold it, no raw-resource blob and no
// map[string]any. The projection that fills this struct is
// mapping.FromProwlarrIndexer, which names every value one at a time. Adding a
// member here is adding one to a security boundary; see migration 0004's
// header.
//
// SearchTypesJSON and CategoriesJSON hold UsArr'S OWN JSON, marshalled from
// mapping.CatalogIndexer's projected structs. They are never re-serialised
// upstream JSON, and the difference is the whole reason the leak class is
// closed rather than redacted.
type Indexer struct {
	ServiceInstanceID int64

	// IndexerID is the id the SERVICE uses, not a UsArr id: it is what
	// ?indexer= sends back to GET /api/v1/search. It is int64 rather than the
	// int32 the wire uses so nothing on this path performs a narrowing
	// conversion; SQLite stores an INTEGER either way.
	IndexerID int64

	Name     string
	Protocol string // usenet | torrent | unknown
	Privacy  string // public | semi-private | private, or "" when unstated

	// Enabled mirrors the service's own flag. GET /api/v1/indexer returns
	// enabled AND disabled indexers, so this is routinely false.
	Enabled bool

	// Priority is 1-50 and LOWER WINS.
	Priority int64

	SupportsSearch     bool
	SupportsRSS        bool
	SupportsPagination bool

	// SearchTypesJSON is a JSON array of search|tvsearch|movie|music|book.
	SearchTypesJSON string

	// LimitsMax and LimitsDefault are NULL when the indexer advertised neither.
	// NULL and 0 are different: 0 would read as "returns nothing".
	LimitsMax     sql.NullInt64
	LimitsDefault sql.NullInt64

	// CategoriesJSON is a JSON array of {id, name, sub_categories}: the RAW
	// Newznab/Torznab tree, never collapsed.
	CategoriesJSON string

	FetchedAt time.Time
}

// ReplaceIndexers replaces one instance's whole catalogue and records the fetch
// time on the instance row.
//
// REPLACE-WHOLE, NOT UPSERT, and in ONE transaction. Upstream is authoritative:
// an indexer deleted in Prowlarr has to disappear from the picker rather than
// linger as a filter that silently matches nothing. Doing the delete and the
// inserts in separate transactions would leave a window in which a concurrent
// read renders an empty picker on a healthy instance.
//
// indexers_fetched_at is written HERE and only on success, so it never claims a
// freshness the replica does not have. It is set even when ixs is empty,
// because "this service answered and has zero indexers" is a real state the
// screen must be able to distinguish from "never successfully connected" — and
// zero rows alone cannot tell them apart.
//
// It takes NO access scope, deliberately: it is a replication write driven by
// the background prober, which has no acting user, exactly like
// UpdateServiceInstanceHealth beside it. The scope belongs on the READ — see
// ListIndexers.
func (s *Store) ReplaceIndexers(ctx context.Context, instanceID int64, ixs []Indexer, fetchedAt time.Time) error {
	if fetchedAt.IsZero() {
		fetchedAt = time.Now()
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM indexer_catalog WHERE service_instance_id = ?`, instanceID); err != nil {
			return fmt.Errorf("replace indexer_catalog for service_instance %d: clear: %w", instanceID, err)
		}

		if len(ixs) > 0 {
			stmt, err := tx.PrepareContext(ctx, `
				INSERT INTO indexer_catalog (
				  service_instance_id, indexer_id, name, protocol, privacy, enabled, priority,
				  supports_search, supports_rss, supports_pagination, search_types,
				  limits_max, limits_default, categories, fetched_at
				) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
			if err != nil {
				return fmt.Errorf("replace indexer_catalog for service_instance %d: prepare: %w", instanceID, err)
			}
			defer func() { _ = stmt.Close() }()

			for _, ix := range ixs {
				_, err := stmt.ExecContext(ctx,
					instanceID, ix.IndexerID, ix.Name,
					defaultString(ix.Protocol, "unknown"), ix.Privacy, ix.Enabled, ix.Priority,
					ix.SupportsSearch, ix.SupportsRSS, ix.SupportsPagination,
					defaultString(ix.SearchTypesJSON, "[]"),
					ix.LimitsMax, ix.LimitsDefault,
					defaultString(ix.CategoriesJSON, "[]"),
					FormatTime(fetchedAt))
				if err != nil {
					return fmt.Errorf("replace indexer_catalog for service_instance %d: insert indexer %d: %w",
						instanceID, ix.IndexerID, err)
				}
			}
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET indexers_fetched_at = ? WHERE id = ? AND deleted_at IS NULL`,
			FormatTime(fetchedAt), instanceID)
		if err != nil {
			return fmt.Errorf("replace indexer_catalog for service_instance %d: stamp fetch time: %w", instanceID, err)
		}
		return expectOneRow(res, "service_instance", instanceID)
	})
}

// indexerListSQL renders one scoped catalogue read: the statement and its
// arguments.
//
// Split out from ListIndexers so TestIndexerCatalogReadUsesTheInstanceIndex can
// EXPLAIN the query this function ACTUALLY issues, rather than a copy pasted
// into a test — which is how a query-plan assertion silently stops covering the
// query it names. Same arrangement as auditListSQL.
func indexerListSQL(scope Scope, instanceID int64) (string, []any) {
	// The scope is applied HERE, in the SQL, not merely accepted in the
	// signature. That distinction is the whole point of the parameter: a read
	// that returned another user's visible instances' indexers would be an
	// existence oracle for services that user cannot see.
	pred, args := scope.instancePredicate("service_instance_id")
	query := `SELECT service_instance_id, indexer_id, name, protocol, privacy, enabled, priority,
	                 supports_search, supports_rss, supports_pagination, search_types,
	                 limits_max, limits_default, categories, fetched_at
	            FROM indexer_catalog
	           WHERE service_instance_id = ? AND ` + pred + `
	           ORDER BY name ASC`
	return query, append([]any{instanceID}, args...)
}

// ListIndexers reads one instance's replicated indexers, in name order.
//
// IT TAKES THE ACCESS SCOPE IN ITS SIGNATURE AND APPLIES IT IN THE SQL, and
// both halves are load-bearing (ARCHITECTURE.md §1.3 rule 2). In v0.1 the owner
// scope renders `1=1` and nothing is filtered; the parameter exists so
// multi-user is a behaviour change rather than a redesign, and so a scope that
// can see nothing returns nothing rather than everything.
//
// It reads ONE instance per call rather than taking a set, and that is
// deliberate: `service_instance_id = ?` is an equality on the leading column of
// ix_indexer_catalog_instance, so the plan is a SEARCH that also supplies the
// ORDER BY. An `IN (…)` over the same index cannot supply the order and adds a
// temp b-tree (the effect pinned in TestScopedProvenanceOrderNeedsASort). A
// homelab has single-digit instances, so the caller's loop is cheaper than the
// sort it avoids.
//
// It does NOT report whether the replica has ever been populated: zero rows can
// mean "never fetched" or "fetched, and this service genuinely has no
// indexers". That distinction lives on service_instance.indexers_fetched_at,
// because it is a fact about the instance and not about any row here.
func (s *Store) ListIndexers(ctx context.Context, scope Scope, instanceID int64) ([]Indexer, error) {
	return listIndexers(ctx, s.db.Read(), scope, instanceID)
}

// listIndexers is ListIndexers's body, over any querier, so ReadIndexerCatalog
// can run it inside the same snapshot as the instance read.
func listIndexers(ctx context.Context, q querier, scope Scope, instanceID int64) ([]Indexer, error) {
	query, args := indexerListSQL(scope, instanceID)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list indexer_catalog for service_instance %d: %w", instanceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Indexer
	for rows.Next() {
		var ix Indexer
		var fetchedAt string
		if err := rows.Scan(
			&ix.ServiceInstanceID, &ix.IndexerID, &ix.Name, &ix.Protocol, &ix.Privacy,
			&ix.Enabled, &ix.Priority, &ix.SupportsSearch, &ix.SupportsRSS, &ix.SupportsPagination,
			&ix.SearchTypesJSON, &ix.LimitsMax, &ix.LimitsDefault, &ix.CategoriesJSON, &fetchedAt,
		); err != nil {
			return nil, fmt.Errorf("list indexer_catalog for service_instance %d: scan: %w", instanceID, err)
		}
		if ix.FetchedAt, err = ParseTime(fetchedAt); err != nil {
			return nil, err
		}
		out = append(out, ix)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list indexer_catalog for service_instance %d: %w", instanceID, err)
	}
	return out, nil
}

// IndexerCatalog is one service instance together with its replicated indexers,
// read as a pair.
//
// The pairing is the point. Instance.IndexersFetchedAt is the ONLY thing that
// separates "never successfully read" from "read, and this service has zero
// indexers", so the row count alone cannot be interpreted without it — and a
// count read at one instant against a timestamp read at another is not a state
// the database was ever in.
type IndexerCatalog struct {
	Instance ServiceInstance
	Indexers []Indexer
}

// ReadIndexerCatalog reads instances and their replicated indexers under ONE
// snapshot. instanceID selects a single instance; 0 means every instance in
// scope. A named instance that is not visible is ErrNotFound, exactly as
// GetServiceInstance reports it.
//
// THE READ TRANSACTION IS THE WHOLE POINT, and it is the read-side half of
// ReplaceIndexers's write transaction above. That write stamps
// service_instance.indexers_fetched_at and rewrites indexer_catalog atomically
// so no reader can see one without the other — but atomicity on the write side
// only buys that if the read side asks once. Two statements on the read pool are
// two WAL snapshots: issued either side of a replication commit, the first
// returns indexers_fetched_at NULL and the second returns the freshly written
// rows, and the caller renders an instance that says "UsArr has not yet read the
// indexer list" while listing the indexers it just read. That torn read was
// reproducible on GET /api/v1/indexers at roughly 1 request in 100 on a loaded
// four-core box; it is closed here, in the store, because it is a property of
// the pair rather than of any one caller.
//
// It reads the catalogue for every instance it returns, including instances the
// caller will discard as disabled or non-indexer: the role and enabled policy
// belongs to the caller, and each extra read is an indexed equality lookup on a
// table with single-digit instances in it. Keep the transaction short —
// db.ReadTx's note about starving the WAL checkpointer applies.
func (s *Store) ReadIndexerCatalog(ctx context.Context, scope Scope, instanceID int64) ([]IndexerCatalog, error) {
	var out []IndexerCatalog
	err := s.db.ReadTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var instances []ServiceInstance
		if instanceID > 0 {
			si, err := getServiceInstance(ctx, tx, scope, instanceID)
			if err != nil {
				return err
			}
			instances = []ServiceInstance{si}
		} else {
			var err error
			if instances, err = listServiceInstances(ctx, tx, scope); err != nil {
				return err
			}
		}

		out = make([]IndexerCatalog, 0, len(instances))
		for _, si := range instances {
			ixs, err := listIndexers(ctx, tx, scope, si.ID)
			if err != nil {
				return err
			}
			out = append(out, IndexerCatalog{Instance: si, Indexers: ixs})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
