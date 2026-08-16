-- Migration 0003 — provenance.acquisition_state, so an acquisition UsArr could
-- not confirm is distinguishable FROM THE ROW ITSELF.
--
-- WHY THIS EXISTS. Prowlarr's single-release grab adds the release to the
-- download client FIRST and applies its configuration SECOND, with no rollback
-- (docs/reference/search.md, and the Deluge source quoted there). A throw in
-- that tail — Deluge's Label plugin rejecting a label that does not exist —
-- leaves the torrent RUNNING in the client and still returns a bare 500. A 200
-- is the only success signal the API offers, so a 5xx or a timeout on the grab
-- POST means "sent, outcome unknown", not "failed".
--
-- That leaves UsArr with a torrent it may well be downloading and no join key
-- for it. provenance.download_id is THE key that reattaches a grab to the file
-- an importer later produces; writing no row loses the infohash permanently,
-- and by the time library sync lands there is nothing to join back to.
--
-- But writing the row with no discriminator ON THE ROW is worse than writing
-- none. provenance has no back-reference to audit_log — audit_log.target_id
-- holds the RELEASE-CANDIDATE id, and the candidate is swept 25 minutes later —
-- so a reader starting from provenance, which is exactly what the import join
-- and the Home recent-grabs block both do, could not tell an unconfirmed
-- acquisition from a confirmed one. The join would then SUCCEED and attach
-- wrong history, which is a worse failure than an absent row.
--
-- WHY NOT AN EXISTING COLUMN.
--
--   * `confidence` is already spoken for. It means match/link confidence for
--     the v0.3 fuzzy-match tier, and schema.md §7 gates a partial index at
--     `confidence >= 1.0`. An acquisition that is perfectly identified but
--     unconfirmed would be filtered out by the queries that most want it.
--   * `source_system` is a documented enum ('sonarr|radarr|prowlarr|manual|
--     filesystem') that future reads group by; a 'prowlarr-unconfirmed' value
--     breaks every equality filter on 'prowlarr'.
--
-- NO CHECK CONSTRAINT, DELIBERATELY. SQLite cannot ALTER a CHECK, and this
-- project has already paid for that once: 0001's audit_log foreign key made
-- user deletion permanently impossible and had to be dropped outright.
-- audit_log.result sets the precedent this follows — TEXT NOT NULL, no CHECK,
-- vocabulary documented and enforced in Go (internal/store/audit.go for
-- result, internal/store/releases.go for this one). v0.2's request path may
-- want a 'pending' state, and adding it must not cost a table rebuild.
--
-- NO TABLE REBUILD IS NEEDED, on the same three grounds 0002 checked by
-- execution rather than by memory, and TestMigration0003NeedsNoRebuild pins
-- them again for a TEXT column:
--
--   * ADD COLUMN is legal on a STRICT table.
--   * ADD COLUMN with `NOT NULL DEFAULT <non-null>` is legal, and it rewrites
--     the stored table DDL in place without rewriting the rows — so the
--     backfill is the default itself and costs nothing on a provenance table
--     that only ever grows.
--   * Nothing here touches a CHECK constraint, which is the one thing
--     ALTER TABLE cannot express.
--
-- The default 'confirmed' is honest for every pre-existing row: before this
-- migration a provenance row was only ever written after a 2xx.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up
-- +goose StatementBegin

ALTER TABLE provenance
  ADD COLUMN acquisition_state TEXT NOT NULL DEFAULT 'confirmed';

-- "what needs looking at", per user, newest first — the Home attention block.
-- Partial, so it indexes only the rows that are by construction rare: the
-- overwhelming majority of provenance rows are 'confirmed' and are not in it at
-- all. user_id leads because the read predicate always carries it, and id
-- breaks ties because grabbed_at has one-second resolution.
CREATE INDEX ix_prov_unconfirmed ON provenance(user_id, grabbed_at DESC, id DESC)
  WHERE acquisition_state <> 'confirmed';

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
--
-- The index goes FIRST: SQLite refuses to DROP COLUMN on an indexed column.
-- +goose StatementBegin
DROP INDEX IF EXISTS ix_prov_unconfirmed;
ALTER TABLE provenance DROP COLUMN acquisition_state;
-- +goose StatementEnd
