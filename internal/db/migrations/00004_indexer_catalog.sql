-- Migration 0004 — indexer_catalog: the local replica of each indexer
-- service's own indexer list, so a picker renders WITHOUT calling Prowlarr.
--
-- WHY THIS EXISTS. The Search screen's indexer filter had no source of truth
-- except the `search.indexer` SSE frames a search emits, so it was empty until
-- the user had already run one search — which is exactly the screen where the
-- filter is meant to be used. The obvious fix, proxying GET /api/v1/indexer on
-- request, is the one thing ARCHITECTURE.md §2 forbids: the picker renders
-- before the search, so it is a render path, and a render path may not block on
-- an *Arr. Principle 1 says every user-facing read renders from local SQLite,
-- so the list is REPLICATED here and refreshed out of band by the background
-- prober (cmd/usarr/services.go), on the same schedule and by the same code
-- path that already refreshes health warnings and blocked indexers.
--
-- WHY A TABLE RATHER THAN AN IN-PROCESS CACHE. A process-lifetime map would
-- have moved the caveat rather than removed it: empty until the first search
-- becomes empty until the first probe after every restart, and the screen would
-- still have to apologise for itself. `service_instance`'s own health columns
-- already carry the precedent — they are persisted precisely so "a restart must
-- not blank the Services screen".
--
-- WHAT IS DELIBERATELY NOT HERE, AND IT IS THE POINT OF THE TABLE'S SHAPE.
--
--   * `fields[]`. Prowlarr's IndexerResource carries a fields array whose
--     entries have privacy ∈ {password, apiKey, userName}: for a private
--     tracker that is the user's PASSKEY, its RSS key, its API key or its
--     session cookie. A leaked passkey is account termination on a private
--     tracker, and this project has already written one to disk once — see
--     migration-era note on release_candidate.info_url and
--     internal/releases.persist. There is no fields column, no cookie column
--     and no raw-resource column, so the leak has nowhere to land. THE COLUMN
--     LIST BELOW IS AN ALLOWLIST: a value reaches SQLite only because a named
--     column put it there.
--   * The raw upstream body. `categories` and `search_types` hold JSON, and
--     that JSON is UsArr'S OWN, marshalled from the projected structs in
--     internal/servarr/mapping (CatalogIndexer / CatalogCategory), never
--     re-serialised upstream JSON. Those structs have no member that could
--     hold a credential.
--
-- REPLACE-WHOLE, NOT UPSERT. The prober deletes this instance's rows and
-- reinserts them in one transaction. Upstream is authoritative: an indexer
-- deleted in Prowlarr must disappear from the picker rather than linger as a
-- filter that silently matches nothing.
--
-- WHY service_instance.indexers_fetched_at IS A SEPARATE COLUMN. Three states
-- have to be distinguishable from the data alone, because the screen says a
-- different sentence for each: no indexer service configured at all; one
-- configured that UsArr has never successfully talked to; and one that answered
-- and genuinely has zero indexers. The last two both have zero rows in this
-- table, so the fetch timestamp cannot live on the rows. NULL means "never
-- successfully fetched" — the honest degradation of principle 3, instead of an
-- empty list that reads as "you have no indexers".
--
-- NO TABLE REBUILD IS NEEDED, checked on the same three grounds 0002 and 0003
-- checked by execution and pinned in TestMigration0004NeedsNoRebuild:
--
--   * ADD COLUMN is legal on a STRICT table.
--   * ADD COLUMN of a NULLABLE column with no default rewrites the stored DDL
--     in place and does not rewrite the rows.
--   * Nothing here touches a CHECK constraint, which is the one thing
--     ALTER TABLE cannot express.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up
-- +goose StatementBegin

CREATE TABLE indexer_catalog (
  service_instance_id INTEGER NOT NULL
                        REFERENCES service_instance(id) ON DELETE CASCADE,
  -- indexer_id is the id the SERVICE uses, not a UsArr id: it is what
  -- ?indexer= sends back to GET /api/v1/search, and what the search fan-out
  -- addresses one request at a time.
  indexer_id          INTEGER NOT NULL,
  name                TEXT NOT NULL,
  protocol            TEXT NOT NULL DEFAULT 'unknown',  -- usenet|torrent|unknown
  privacy             TEXT NOT NULL DEFAULT '',         -- public|semi-private|private
  enabled             INTEGER NOT NULL DEFAULT 0,
  -- 1-50 and LOWER WINS. Same field the cross-indexer dedupe tiebreaks on.
  priority            INTEGER NOT NULL DEFAULT 0,
  supports_search     INTEGER NOT NULL DEFAULT 0,
  supports_rss        INTEGER NOT NULL DEFAULT 0,
  supports_pagination INTEGER NOT NULL DEFAULT 0,
  -- JSON array of UsArr's own search-type vocabulary: search|tvsearch|movie|
  -- music|book. Derived from capabilities.*SearchParams, so the picker can grey
  -- out an indexer that cannot serve the selected type instead of letting the
  -- user pick one that will be skipped with a reason after the fact.
  search_types        TEXT NOT NULL DEFAULT '[]',
  limits_max          INTEGER,   -- NULL means the indexer advertised none
  limits_default      INTEGER,
  -- JSON array of {id, name, sub_categories}: the RAW Newznab/Torznab tree,
  -- never collapsed (3030 is the only reliable audiobook-vs-music signal, 7030
  -- likewise for comics). This is what makes filtering by category possible on
  -- a screen that has not run a search yet.
  categories          TEXT NOT NULL DEFAULT '[]',
  fetched_at          TEXT NOT NULL,
  PRIMARY KEY (service_instance_id, indexer_id)
) STRICT;

-- The one read this table has: one instance's indexers, in name order. name is
-- in the index so the ORDER BY comes from it rather than from a temp b-tree —
-- pinned by TestIndexerCatalogReadUsesTheInstanceIndex.
CREATE INDEX ix_indexer_catalog_instance ON indexer_catalog(service_instance_id, name);

-- NULL means UsArr has never successfully read this instance's indexer list.
-- It is set only on a successful fetch, so it never claims a freshness the
-- replica does not have.
ALTER TABLE service_instance ADD COLUMN indexers_fetched_at TEXT;

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
--
-- The index goes FIRST: SQLite refuses to DROP COLUMN on an indexed column, and
-- dropping the table it belongs to while it exists is needless ordering risk.
-- +goose StatementBegin
DROP INDEX IF EXISTS ix_indexer_catalog_instance;
DROP TABLE IF EXISTS indexer_catalog;
ALTER TABLE service_instance DROP COLUMN indexers_fetched_at;
-- +goose StatementEnd
