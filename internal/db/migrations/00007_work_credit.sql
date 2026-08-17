-- Migration 0007 — work_credit, the table an author lands in.
--
-- SCOPE. Exactly one table and one index, and nothing else:
--
--   work_credit · ix_credit_creator
--
-- No ALTER, no rebuild, no new work.kind member, no other object.
--
-- WHY NOW, AND WHY THIS ONE AND NOT THE OTHER TWO OF THE MUSIC THREE.
-- ADR-0040 decided that each of the six subtype tables lands with the
-- CATALOGUE SOURCE THAT WRITES IT rather than with a milestone, on 00001's
-- rule: "a migration that creates a table nothing queries is a schema claim
-- nobody has tested." When ADR-0040 was written the first catalogue source was
-- assumed to be a music server and "a credit" meant a performer, so work_credit
-- was filed with work_album and work_track under NAVIDROME. ADR-0041 then made
-- KAVITA the first adapter, and Kavita writes books and comics — which is a
-- books-and-comics source that reports WRITERS, COVER ARTISTS, PENCILLERS,
-- INKERS, COLORISTS, LETTERERS, EDITORS and TRANSLATORS on every series it has
-- metadata for. work_credit is the table all eight of those land in; its `role`
-- CHECK below names every one of them. So ADR-0040's own rule now points at
-- Kavita for this table, and ADR-0044 applies the rule rather than overriding
-- it. 00006's header stated the consequence of NOT doing this in terms — "any
-- creator a Kavita series reports has NOWHERE TO LAND in v0.1 — not a lossy
-- landing, none at all" — and this migration is the answer to that sentence.
--
-- work_album and work_track DO NOT MOVE. They are the two of the music three
-- whose writer really is Navidrome: an album and a track are music objects, no
-- v0.1 source produces either, and internal/ still has no navidrome package.
-- ADR-0040 stands whole for them and this migration reopens nothing.
--
-- THE HONEST COST, WHICH IS A ROW AND NOT A COLUMN. work_credit.creator_work_id
-- is a foreign key to `work`, and the creator it points at is a work of kind
-- 'person' (ADR-0033). Nothing in the tree created a 'person' work before this
-- change, so landing this table means the Kavita adapter now creates author
-- rows in `work` as well as credit rows here. ADR-0040 rejected the six-table
-- variant partly on "a foreign key whose referents cannot exist"; the referents
-- exist as of the commit this migration ships in, which is the condition that
-- objection set. `work.kind` already carries 'person' — 00005 paid for the full
-- twelve-member CHECK precisely so this would cost no rebuild — so THIS
-- MIGRATION ADDS NO CHECK MEMBER AND CHANGES NO kind_byte.
--
-- 'person' HAS NO SUBTYPE TABLE AND THIS MIGRATION DOES NOT INVENT ONE.
-- schema.md §6.1's rule is "every `kind` has a subtype table or an explicit
-- justification for not having one", and 'person' has the justification: a
-- credited human is a name, an optional set of external_id rows and the credits
-- that point at it, all of which `work`, `external_id` and this table already
-- carry. A work_person table would hold a birth year and a biography, which no
-- v0.1 source reports for an author, so it would be columns nothing writes.
--
-- NO 12-STEP REBUILD, CHECKED RATHER THAN ASSUMED, on 00006's three points:
--
--   * work_credit is brand-new, so there is no data to copy and hence no way
--     for this migration to lose a row. TestMigrate0007 says exactly that
--     rather than claiming data survived a copy that never happened.
--   * No table in this schema references work_credit — it is a CHILD of `work`
--     twice over and a parent of nothing — so the Down block's DROP fires no
--     cascade into anything.
--   * PRAGMA foreign_keys=OFF is not written, for 00005's and 00006's reasons:
--     goose runs each migration inside a transaction, SQLite documents the
--     pragma as a no-op there, and there is no rebuild for it to protect.
--
-- THE ONE CHECK HERE IS ON A NOT NULL COLUMN, AND THAT IS WHY IT IS WRITTEN AS
-- A BARE IN LIST. DB-01 — `CHECK (x IN (NULL, …))`, which enforces nothing
-- because one NULL in an IN list makes the comparison NULL for every
-- non-matching value and a CHECK passes on NULL — shipped twice in this project
-- and TestNullableCheckConstraintsActuallyConstrain is the witness. It applies
-- to NULLABLE columns. `role` is NOT NULL, so `role IN (…)` is the correct and
-- complete form and `role IS NULL OR role IN (…)` would be WRONG here: it would
-- widen a NOT NULL column's constraint to admit a value the column type already
-- forbids, which reads as carelessness rather than as care.
--
--   ⚠️ FOR WHOEVER ADDS A NULLABLE CHECKED COLUMN TO THIS TABLE LATER — the
--   obvious candidate is a vocabulary on `credited_as`, which IS nullable — the
--   only correct form is `CHECK (x IS NULL OR x IN (…))`. `credited_as` carries
--   no CHECK today because it holds a free-text printed name, not a member of
--   any enum.
--
-- WHY `role` GETS A CHECK AT ALL WHEN 00006 GAVE ITS THREE TABLES NONE. 00006's
-- header argued that reading_direction, total_issues_source and special_version
-- are "the newest and least settled strings in the schema … every one of them is
-- a projection over MULTIPLE upstreams whose own enums do not agree", so a
-- growing vocabulary would cost a 12-step rebuild per string. `role` is the
-- opposite case on both halves. It is not a projection over disagreeing
-- upstreams — it is a FIXED vocabulary schema.md §1.1 declares in full and has
-- declared since ADR-0031, and it is the column the PRIMARY KEY is built on, so
-- a typo'd role does not render oddly, it silently creates a SECOND credit row
-- for the same person on the same work. That is the class of defect a CHECK
-- exists for. The vocabulary is reproduced verbatim from schema.md §1.1,
-- eighteen members, and TestWorkCreditRoleVocabularyMatchesTheReference is what
-- says the two have not drifted.
--
-- NO TIMESTAMP COLUMN, which is schema.md's shape and 00005's and 00006's for
-- every subtype table: a credit's lifetime is its work's, and `work` already
-- carries created_at/updated_at/deleted_at.
--
-- DDL is reproduced from docs/reference/schema.md §1.1, which is authoritative
-- for the SHAPE, with the conventions 00005 and 00006 actually shipped taking
-- precedence over the reference file's presentation. §1.1's paragraph
-- "`work_album`, `work_track` and `work_credit` still wait for Navidrome" is
-- falsified for this one table by ADR-0044 and schema.md is updated in the same
-- change.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up
-- +goose StatementBegin

-- ─── Attribution · schema.md §1.1, ADR-0031, ADR-0033, ADR-0044 ──────────────
--
-- ADR-0031 decision #3: attribution is M:N. There is NO author_id column on a
-- book and no artist_id column on an album. The moment attribution is a scalar,
-- a comic with a writer AND a penciller AND a colorist is unrepresentable — and
-- that is the ordinary shape of every row Kavita reports, not an exotic case.

CREATE TABLE work_credit (
  work_id         INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  -- A work of kind 'artist' OR of kind 'person' (ADR-0033), and WHICH ONE IS A
  -- RULE RATHER THAN A PER-ROW CHOICE: 'artist' when a connected service models
  -- the creator as a top-level catalogue entity in its own right — a Navidrome
  -- or Lidarr artist, which has albums, a page and a library row — and 'person'
  -- otherwise. Kavita models a creator as a PersonDto hanging off series
  -- metadata and not as a catalogue entity, so every credit this migration's
  -- first writer produces points at a 'person'.
  --
  -- THE KIND IS NOT CONSTRAINED HERE AND CANNOT BE. SQLite has no way to say
  -- "this foreign key's target row must have kind IN ('artist','person')" — a
  -- CHECK cannot subquery, and a trigger on work_credit would fire on the
  -- credit rather than on the work it points at. It is an application-enforced
  -- invariant with a CI assertion over a POPULATED fixture, in exactly the form
  -- schema.md §1.1 already uses for work_track.edition_id — must return no rows:
  --
  --   SELECT c.work_id, c.creator_work_id FROM work_credit c
  --     JOIN work w ON w.id = c.creator_work_id
  --    WHERE w.kind NOT IN ('artist','person');
  --
  -- TestCreditedWorksAreArtistsOrPeople is that assertion.
  creator_work_id INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  -- Eighteen members, verbatim from schema.md §1.1. NINE of them are the ones
  -- Kavita writes today (author, translator, editor, writer, penciller, inker,
  -- colorist, letterer, cover_artist); the music seven and illustrator/narrator
  -- have no v0.1 writer and are here because SQLite cannot ALTER a CHECK.
  --
  -- NOT NULL, so the bare IN list is the correct and complete form. See the
  -- header on DB-01 and on why `IS NULL OR` would be wrong here rather than
  -- merely redundant.
  role            TEXT NOT NULL CHECK (role IN (
                    -- music
                    'primary','featured','composer','conductor','performer','remixer','producer',
                    -- books
                    'author','translator','editor','illustrator','narrator',
                    -- comics
                    'writer','penciller','inker','colorist','letterer','cover_artist')),
  -- Billing order WITHIN (work, role). It is part of the primary key, which is
  -- what lets one work carry two writers without them colliding.
  position        INTEGER NOT NULL DEFAULT 0,
  -- The string this release printed, when it differs from the creator work's
  -- title ("Sting" on a Police reissue). Free text; no CHECK, see the header.
  credited_as     TEXT,
  -- The PK LEADS WITH (work_id, role, position) because the only hot read is
  -- "give me this work's credits in billing order", which is then a single
  -- covered range scan over a WITHOUT ROWID table. creator_work_id TRAILS it to
  -- make the row unique when two creators share a role and a position — a
  -- co-credit — which is a real shape and not a defensive tiebreaker.
  PRIMARY KEY (work_id, role, position, creator_work_id)
) STRICT, WITHOUT ROWID;

-- The reverse read: everything one creator is credited on. It is normative in
-- schema.md §1.1 and pinned by a query-plan assertion in
-- internal/db/queryplan_test.go.
--
-- ⚠️ IT HAS NO SCREEN IN ANY MILESTONE AND IT IS STILL THE RIGHT INDEX, which
-- is worth stating so the next reader does not delete it as speculative.
-- 'person' is excluded from the media-type navigation enum, the Tier 1 client
-- prefix index and the FTS corpus (schema.md §6.1) precisely because there is
-- no person screen, so "find everything by this author" is UNANSWERED in v0.1 —
-- carried as a known loss rather than tidied away. What this index actually
-- serves in v0.1 is the WRITE path: the credit applier replaces one work's
-- credits wholesale and must be able to find a person's remaining credits
-- cheaply, and a person work that ends up credited on nothing is the row a
-- later sweep would collect. Without it both are a full scan of every credit in
-- the library.
CREATE INDEX ix_credit_creator ON work_credit(creator_work_id, role);

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
--
-- One DROP and no rebuild. The index goes with the table, so DROP INDEX would
-- be redundant. Nothing else in the schema refers to work_credit, so no cascade
-- reaches a child and no other object needs recreating.
--
-- ⚠️ WHAT THIS DOWN DOES NOT UNDO, stated because it is invisible: the 'person'
-- rows in `work` that the credit path created. They are ordinary work rows and
-- this migration neither created nor owns them; dropping work_credit leaves
-- them standing, credited on nothing. That is the correct behaviour for a
-- schema rollback — a Down block must not delete catalogue data — and a
-- re-Up followed by a re-import rebuilds the credits against the same rows,
-- because the person dedupe key is the normalized name and not a rowid.
-- +goose StatementBegin

DROP TABLE IF EXISTS work_credit;

-- +goose StatementEnd
