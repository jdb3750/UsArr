-- Migration 0006 — the three subtype tables Kavita writes.
--
-- SCOPE. Exactly the books-and-comics half of schema.md §1.1's deferred six,
-- and nothing else. It creates:
--
--   work_book · work_comic · work_comic_issue
--
-- plus ix_comic_issue_sort. No other object, no ALTER, no rebuild.
--
-- WHY NOW, AND WHY THIS THREE AND NOT SIX. ADR-0040 decided that each of the
-- six subtype tables lands with the CATALOGUE SOURCE THAT WRITES IT rather than
-- with a milestone — 00001's rule, quoted in 00005's header: "a migration that
-- creates a table nothing queries is a schema claim nobody has tested."
-- ADR-0041 then made KAVITA v0.1's first adapter (amending ADR-0036's "no
-- catalogue source ships in v0.1"), and Kavita is a books-and-comics/manga
-- source (ADR-0035). So the landing point moved and the rule did not:
-- ADR-0041's own words are "the landing point is the source, not the date", and
-- it states in terms that "`work_book`, `work_comic` and `work_comic_issue` are
-- now due with THIS work, not later … They arrive in a NEW migration —
-- `00005_library_sync.sql` is merged and `CLAUDE.md` is unambiguous that a
-- merged migration is never edited." This is that migration.
--
-- DELIBERATELY NOT HERE, and which source owns it:
--
--   * work_album, work_track, work_credit — the music three. They still wait
--     for NAVIDROME, which has no adapter in the tree (internal/ has kavita and
--     servarr, and nothing else that reads a catalogue). ADR-0041 says so
--     explicitly — "The music three are unaffected" — and ADR-0040 stands
--     whole. Nothing here reopens either.
--
--     ⚠️ AND THE CONSEQUENCE OF DEFERRING work_credit IS STATED RATHER THAN
--     DISCOVERED, because it is the one that bites in a comics milestone and
--     not in a music one: work_credit is where an author, writer, penciller,
--     inker, colorist, letterer or cover artist would be stored (its `role`
--     CHECK in schema.md §1.1 names all of them), and it is not here. So any
--     creator a Kavita series reports has NOWHERE TO LAND in v0.1 — not a
--     lossy landing, none at all. That is ADR-0041's decision and not an
--     oversight of this migration: it keeps work_credit with Navidrome, and
--     work_credit.creator_work_id points at a `work` of kind 'person', which
--     nothing in v0.1 creates either. It is free to add later for exactly
--     ADR-0040's reason — a brand-new table with no dependants — and expensive
--     to guess at now.
--
--   * request, request_quota (v0.2), work_relation (v0.3), work_merge,
--     tag_rule, tag_alias, tag_implies, saved_filter, playback_state,
--     play_history, playlist, playlist_item and the RBAC tables (v1.0) —
--     schema.md's "later tables" appendix, unchanged by this migration.
--
-- NOTHING IS IRREVERSIBLE HERE AND NOTHING NEEDED TO BE PULLED FORWARD WITH
-- THESE TABLES, which was checked against 00005's header rather than assumed.
-- The one-way doors it names are `work.kind`'s twelve-member CHECK and the
-- kind_byte codec (ADR-0030, ADR-0033, ARCHITECTURE §5.3); both were paid for
-- in 00005 and both already carry 'book', 'comic' and 'comic_issue'. This
-- migration therefore adds no CHECK member, changes no codec, and touches no
-- existing table.
--
-- NO 12-STEP REBUILD, AND THAT IS A CHECKED CLAIM RATHER THAN AN ASSUMPTION.
-- The procedure exists to change a table that already has rows, indexes and
-- referring foreign keys. All three tables here are brand-new, so:
--
--   * There is no data to copy, hence no copy step and hence no way for this
--     migration to lose a row. TestMigrate0006 says the same thing in the test
--     file, because "1→N against an empty database proves the shape and nothing
--     about data" is the lesson 0005 paid for, and the honest answer here is
--     that there IS no data — not that it was checked and survived.
--   * No table in this schema references work_book, work_comic or
--     work_comic_issue, so the Down block's DROP TABLE fires no cascade into a
--     child. (Each is a CHILD of `work`; none is a parent of anything.)
--   * PRAGMA foreign_keys=OFF is not written, for 00005's two reasons: goose
--     runs each migration inside a transaction and SQLite documents the pragma
--     as a no-op there, so the line would look like protection and provide
--     none — and there is no rebuild for it to protect in the first place.
--
-- NO CHECK CONSTRAINT IS WRITTEN ON ANY COLUMN BELOW, and that is a decision
-- with 00005's own precedent behind it rather than an omission. schema.md §1.1
-- gives these three tables no CHECK either; the vocabularies it lists —
-- reading_direction ltr|rtl|vertical|webtoon, total_issues_source
-- comicinfo|comicvine|kavitaplus|null, special_version
-- tpb|hard-cover|omnibus|one-shot|volume-as-issue|cover — are comments there,
-- and they are the newest and least settled strings in the schema: every one of
-- them is a projection over MULTIPLE upstreams (Kavita, Komga, ComicInfo,
-- Comic Vine) whose own enums do not agree, and Komga alone contributes a
-- reading direction Kavita does not model. SQLite cannot ALTER a CHECK, so a
-- growing vocabulary in one costs a 12-step rebuild per string. That is exactly
-- the argument 00005 made for write_queue.state and sync_report.kind, and
-- 0003's provenance.acquisition_state is the shipped precedent: the vocabulary
-- lives in internal/store and is enforced there. work_series.series_type
-- (standard|daily|anime) and work_alt_title.kind in 00005 are the two closest
-- shapes in the tree and neither carries a CHECK.
--
--   ⚠️ FOR WHOEVER ADDS ONE LATER: every column named above is NULLABLE, so the
--   only correct form is `CHECK (x IS NULL OR x IN (…))`. `CHECK (x IN (NULL,
--   …))` is DB-01 — one NULL inside an IN list makes the comparison NULL for
--   every non-matching value, a CHECK passes on NULL, and the constraint then
--   accepts anything at all. It shipped twice in this project, the second time
--   in 00005's own library_override.field_name, and
--   TestNullableCheckConstraintsActuallyConstrain is the witness. Note also
--   that 'null' is written as a MEMBER of total_issues_source's vocabulary in
--   schema.md, which is precisely the sentence someone transcribes into an IN
--   list by hand.
--
-- NO TIMESTAMP COLUMN EXISTS ON ANY OF THE THREE, which is schema.md's shape
-- and 00005's for work_movie/work_series/work_episode: a subtype row's lifetime
-- is its parent work's, and `work` already carries created_at/updated_at/
-- deleted_at. The project rule still stands for whoever adds one — SQLite
-- datetime() text, 'YYYY-MM-DD HH:MM:SS', UTC, no 'T' and no 'Z', not ISO-8601
-- — because ix_work_added and its siblings order such columns
-- lexicographically and two formats in one column break that silently.
--
-- DDL is reproduced from docs/reference/schema.md §1.1, which is authoritative
-- for the SHAPE, with the conventions 00005 actually shipped taking precedence
-- over the reference file's presentation. §1.1's own heading still reads "v0.1
-- for movie/series/episode" and its comment above work_album still schedules
-- all six as "later tables"; both are now falsified for these three and
-- schema.md is updated in the same change.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up
-- +goose StatementBegin

-- ─── Books · schema.md §1.1 ──────────────────────────────────────────────────
--
-- The subtype of work.kind='book'. An AUDIOBOOK is not a kind and has no table:
-- it is an `edition` of a book work with format='audiobook', which is what
-- 00005's edition.format CHECK and its narrators/duration_seconds/abridged
-- columns are for (ADR-0031). Nothing about audiobooks belongs here.

CREATE TABLE work_book (
  work_id     INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  page_count  INTEGER,
  -- 🔍 INFERENCE, marked because schema.md gives these two no prose: this is
  -- the series a book DECLARES it belongs to, as a string and a position, which
  -- is a different fact from work.parent_work_id. The parent is a resolved
  -- link to a `work` that must already exist; these two are what the upstream
  -- said before anything was resolved, and they survive a series work that
  -- UsArr never manages to create. Keeping both is schema.md's shape and is not
  -- reopened here. series_position is REAL for the same reason
  -- work_comic_issue.number_sort is: 1.5 is an ordinary book-series position.
  series_name TEXT, series_position REAL
) STRICT;

-- ─── Comics, SERIES level · schema.md §1.1, ADR-0030 ─────────────────────────
--
-- The subtype splits with the kind: 'comic' is the series and 'comic_issue' is
-- the issue or chapter (ADR-0030). There is no third level for Kavita's Volume
-- and there is no 'manga' kind — manga is reading_direction below plus a
-- derived type:manga system tag, and Kavita's Volume is carried on the issue
-- row as volume_label + volume_sort. ADR-0030 records that flattening as a
-- deliberate loss of fidelity against Kavita specifically, which is worth
-- knowing in the migration that lands with Kavita's adapter.

CREATE TABLE work_comic (
  work_id                INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  volume_label           TEXT,      -- 'Vol. 3' / '(2012)' — a label, never a node
  volume_year            INTEGER,
  reading_direction      TEXT,      -- ltr|rtl|vertical|webtoon; the manga axis, not a kind.
                                    -- Komga models vertical/webtoon and Kavita does not, which
                                    -- is one reason this is not a CHECK — see the header.
  publisher              TEXT,
  -- total_issues_declared IS A DECLARATION AND NOT A FACT, which is why it is
  -- stored beside its source rather than alone. ComicInfo's own `Count` spec
  -- concedes it "could be different on each book in a series", so a bare total
  -- renders a completeness claim nobody can stand behind. schema.md §1's
  -- availability blob (k="count") carries total_source for the same reason, and
  -- the number that is always honest is contiguity — computed locally from
  -- work_comic_issue.number_sort, which is what the index below is for.
  total_issues_declared  INTEGER,
  total_issues_source    TEXT       -- comicinfo|comicvine|kavitaplus|null
) STRICT;

-- ─── Comics, ISSUE level · schema.md §1.1, ADR-0030 ──────────────────────────
--
-- number_text is TEXT and number_sort is REAL because real issue numbers are
-- '1.MU', '-1', '0', 'Annual 1', '1A'. Komga models a string plus a float sort
-- key; Kavita models min/max floats plus a string plus a range. ANY INTEGER
-- COLUMN HERE IS WRONG (ADR-0030), and STRICT is what stops '1.MU' being
-- accepted into one by affinity if someone tries.

CREATE TABLE work_comic_issue (
  work_id         INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  number_text     TEXT,
  number_sort     REAL,
  volume_label    TEXT,     -- Kavita's Volume, carried as an attribute (ADR-0030)
  volume_sort     REAL,
  is_special      INTEGER NOT NULL DEFAULT 0,
  is_oneshot      INTEGER NOT NULL DEFAULT 0,
  special_version TEXT,     -- tpb|hard-cover|omnibus|one-shot|volume-as-issue|cover.
                            -- A TPB is its own issue row; ADR-0030 decides UsArr does
                            -- NOT model which issues it collects, because no backend
                            -- reports it and inferring it from title ranges is the
                            -- false-positive machine ADR-0007 refuses.
  page_count      INTEGER
) STRICT;

-- The contiguity report — "43 issues · #7, #12 and #30-32 missing" — is the only
-- always-honest completeness number in the domain (ARCHITECTURE §6.1), and this
-- index is what makes it cheap: an ordered walk of one series' issues rather
-- than a scan of every issue in the library. It is normative in schema.md §1.1
-- and pinned by a query-plan assertion in internal/db/queryplan_test.go.
--
-- ⚠️ ON (number_sort) ALONE, exactly as schema.md declares it, and the reader
-- should know what that does and does not buy: the series-scoped form of the
-- report — "this work's parent's issues, in order" — reaches `work` through
-- ix_work_parent and this index does not participate in it. Widening it to
-- (work_id, number_sort) is not available, because work_id is the PRIMARY KEY
-- and the series is one join away on work.parent_work_id; a genuinely
-- series-scoped index would need parent_work_id denormalised onto this table,
-- which is a column nothing writes yet and a change to schema.md's shape. Left
-- as declared, with a measurement owed by whoever writes the report.
CREATE INDEX ix_comic_issue_sort ON work_comic_issue(number_sort);

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
--
-- Three DROPs and no rebuild, which is the whole of it — see the header for why
-- that is checked rather than assumed. Reverse creation order, matching 00005's
-- Down block; the indexes go with their tables, so DROP INDEX would be
-- redundant. Nothing else in the schema refers to these three, so no cascade
-- reaches a child and no other object needs recreating.
-- +goose StatementBegin

DROP TABLE IF EXISTS work_comic_issue;
DROP TABLE IF EXISTS work_comic;
DROP TABLE IF EXISTS work_book;

-- +goose StatementEnd
