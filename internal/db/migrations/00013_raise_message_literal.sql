-- Migration 0013 — `trg_library_unfiled_no_delete`'s RAISE message becomes a
-- STATIC LITERAL, so the PERSISTED schema can be read by a SQLite at this
-- project's declared floor.
--
-- The decision, its measurements and its rejected alternative are
-- [ADR-0075](../../../docs/DECISIONS.md#adr-0075). This header states only what
-- the file does; the reasoning has one home and this is not it.
--
-- SCOPE. One trigger, dropped and re-created. No table, no column, no index, no
-- data. The trigger's EVENT (`BEFORE DELETE ON library`), its CONDITION
-- (`WHEN OLD.id = 0`) and its EFFECT (`RAISE(ABORT, …)` with a message that is
-- byte-for-byte the string 00005's four fragments concatenate to) are all
-- carried across unchanged. The only thing that changes is WHERE the
-- concatenation happens: at authoring time here, rather than at PREPARE time in
-- every reader of the stored schema.
--
-- ⚠️ 00005 IS NOT EDITED, and this migration exists because it must not be. A
-- merged migration is never edited; the defect is in an object 00005 created,
-- so the repair is a new migration that replaces the object.
--
-- ⚠️ WHY A LITERAL AND NOT A SHORTER MESSAGE. The operator-facing text is
-- unchanged on purpose: `TestUnfiledLibraryIsProtected` compares the abort
-- message this trigger raises against the one 00005's trigger raised and
-- requires equality, so a "tidier" message here would be a silent change to a
-- diagnostic an operator reads at the moment a delete is refused.
--
-- ⚠️ THE OTHER `RAISE(… || …)` IN 00005 STAYS, AND THIS PARAGRAPH IS WHY — a
-- grep for `RAISE` and `||` finds two hits in this schema's history and only one
-- of them is a defect. `trg_wq_rebuild_guard` (00005, under the `write_queue`
-- rebuild) is created on the SCRATCH table `write_queue_new` and is
-- `DROP TRIGGER`-ed by 00005 itself, before the rename — so it exists only for
-- the span of that one migration and NEVER lands in `sqlite_schema`. Nothing
-- reads it back, so no reader ever prepares it, so it cannot cause the failure
-- this migration fixes. Its expression message is also load-bearing there: it
-- carries a `COUNT` the operator needs. Do not "fix" it. ADR-0075 records this
-- as something it deliberately does not decide.
--
-- PRAGMA foreign_keys=OFF is not written, for the reason 00005 through 00012
-- each give: goose runs each migration inside a transaction, SQLite documents
-- the pragma as a no-op there, and a trigger swap is not a rebuild.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up

DROP TRIGGER trg_library_unfiled_no_delete;

-- +goose StatementBegin

-- The event, the WHEN and the message are 00005's. Only the concatenation moved.
CREATE TRIGGER trg_library_unfiled_no_delete BEFORE DELETE ON library
  WHEN OLD.id = 0
  BEGIN
    SELECT RAISE(ABORT, 'library 0 ("Unfiled") is reserved and cannot be deleted: it is where the membership derivation files a work that belongs to no other library, and without it such a work is invisible in search to every user including its owner. This also blocks DELETE FROM user WHERE id = 0, the shared/system sentinel, deliberately.');
  END;

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
--
-- ⚠️ WHAT ROLLING BACK COSTS, stated because it is not "nothing": it restores
-- 00005's expression-valued message verbatim, and with it the defect — a
-- database rolled back past 0013 is again unreadable by any SQLite below
-- 3.47.0. The Down block is the previous body, not an improved one, because a
-- Down that does not restore what was there is not a rollback.

DROP TRIGGER trg_library_unfiled_no_delete;

-- +goose StatementBegin

CREATE TRIGGER trg_library_unfiled_no_delete BEFORE DELETE ON library
  WHEN OLD.id = 0
  BEGIN
    SELECT RAISE(ABORT, 'library 0 ("Unfiled") is reserved and cannot be deleted: it is where '
      || 'the membership derivation files a work that belongs to no other library, and without '
      || 'it such a work is invisible in search to every user including its owner. This also '
      || 'blocks DELETE FROM user WHERE id = 0, the shared/system sentinel, deliberately.');
  END;

-- +goose StatementEnd
