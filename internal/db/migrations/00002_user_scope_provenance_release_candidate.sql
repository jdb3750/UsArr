-- Migration 0002 — the authorization invariant, applied to the two tables that
-- shipped without it, plus the two indexes the reads they serve actually need.
--
-- WHY THIS EXISTS. ARCHITECTURE.md §1.3 and CLAUDE.md principle 4 say every
-- user-scoped row carries `user_id` from the very first migration, and every
-- aggregating read takes an access-scope parameter in its signature. Migration
-- 0001 honoured that for tag_assignment and write_queue and missed it for
-- provenance and release_candidate. A merged migration is never edited, so the
-- fix is here.
--
-- Backfill value is 0, the reserved shared/system sentinel created by 0001.
-- v0.1 is single-user, so every existing row genuinely was the owner's, and the
-- canonical read predicate `user_id IN (0, :uid)` — the same one tag_assignment
-- uses — keeps those rows visible to the owner after multi-user lands. 0 is a
-- real row rather than NULL because NULL is not usable as an indexed equality.
--
-- NO TABLE REBUILD IS NEEDED, and that was checked rather than assumed:
--
--   * SQLite cannot ALTER a CHECK constraint, but nothing here touches one.
--   * ADD COLUMN with `NOT NULL DEFAULT <non-null>` is legal, and it rewrites
--     the stored table DDL in place; it does not rewrite the rows, so the
--     backfill is the default itself and costs nothing on a large provenance
--     table.
--   * ADD COLUMN is legal on a STRICT table.
--   * ADD COLUMN with a REFERENCES clause AND a non-NULL default was executed
--     against this project's own driver (ncruces/go-sqlite3, SQLite 3.53.4,
--     `foreign_keys=ON`) before this migration was written: it is accepted, and
--     the constraint is enforced on the next insert. The restriction that used
--     to forbid it does not apply here. TestMigration0002NeedsNoRebuild pins
--     both halves of that observation so it is a checked claim, not a memory.
--
-- FOREIGN KEYS: THE TWO TABLES ARE TREATED DIFFERENTLY, ON PURPOSE.
--
-- `release_candidate.user_id` carries `REFERENCES user(id) ON DELETE CASCADE`,
-- matching `write_queue.user_id` and `tag_assignment.user_id` in 0001. It is
-- ephemeral operational state — a 25-minute TTL, swept by ix_rel_expiry — and a
-- deleted user's pending search results should go with them.
--
-- `provenance.user_id` carries NO foreign key, for exactly the reason
-- `audit_log.actor_user_id` carries none (see 0001's comment on it, which was
-- read before this was decided). It is a HISTORICAL id: the user it names may
-- since have been deleted, and the whole point of the table is that it still
-- records who acquired the file. All three available behaviours are wrong here:
--
--   * ON DELETE CASCADE destroys acquisition history when a user is removed,
--     which is the one record provenance exists to keep — provenance rows are
--     immutable and are never overwritten on upgrade precisely so that history
--     survives.
--   * ON DELETE SET NULL cannot be spelled at all on a NOT NULL column, and
--     even if it could it would erase the actor, which is the same loss.
--   * ON DELETE NO ACTION / RESTRICT leaves the constraint violated and makes
--     `DELETE FROM user` fail for any user who has ever grabbed anything.
--
-- Dropping the reference is the only option that permits the delete and keeps
-- the actor recorded. That is audit_log's argument, and provenance is the same
-- class of row.
--
-- Timestamps stay SQLite datetime() text — 'YYYY-MM-DD HH:MM:SS', UTC — because
-- ix_prov_user_grabbed orders on grabbed_at lexicographically and two formats in
-- one column silently breaks the ordering.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up
-- +goose StatementBegin

-- ─── The authorization invariant ─────────────────────────────────────────────

ALTER TABLE provenance
  ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;   -- no REFERENCES: historical id, see above

ALTER TABLE release_candidate
  ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0
    REFERENCES user(id) ON DELETE CASCADE;

-- ─── The size a grab actually produced ───────────────────────────────────────
--
-- release_candidate already carries size_bytes; provenance did not, so the size
-- of an acquisition was lost the moment its candidate expired. Nullable, because
-- not every indexer reports a size and 0 would be a lie rather than an absence.

ALTER TABLE provenance
  ADD COLUMN size_bytes INTEGER;

-- ─── Indexes for reads that currently scan ───────────────────────────────────

-- Recent grabs, per user, newest first. The three indexes 0001 shipped
-- (protocol, indexer_name, download_id) serve equality lookups and none of them
-- supplies grab-time order, so the Home screen's recently-added block sorted the
-- whole table. user_id leads because the predicate always carries it; id breaks
-- ties within a second, because grabbed_at has one-second resolution and two
-- grabs in the same second must still have a total order or keyset paging
-- repeats a row.
CREATE INDEX ix_prov_user_grabbed ON provenance(user_id, grabbed_at DESC, id DESC);

-- "this user's failed grabs", the Home screen's attention block. Filtering one
-- user's rows by action scanned the whole log, which grows forever by design.
-- ts DESC trails the two equality columns so the newest-first order comes out of
-- the index rather than a temp b-tree.
CREATE INDEX ix_audit_actor_action ON audit_log(actor_user_id, action, ts DESC);

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
--
-- The indexes go FIRST: SQLite refuses to DROP COLUMN on an indexed column, so
-- dropping provenance.user_id before ix_prov_user_grabbed fails.
-- +goose StatementBegin
DROP INDEX IF EXISTS ix_audit_actor_action;
DROP INDEX IF EXISTS ix_prov_user_grabbed;
ALTER TABLE provenance DROP COLUMN size_bytes;
ALTER TABLE release_candidate DROP COLUMN user_id;
ALTER TABLE provenance DROP COLUMN user_id;
-- +goose StatementEnd
