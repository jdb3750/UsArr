-- Migration 0005 — the library-sync schema, and the write_queue rebuild that
-- migration 0001 scheduled for it.
--
-- SCOPE. This is the schema v0.1's Sonarr/Radarr library sync needs
-- (ARCHITECTURE.md §16's v0.1 entry enumerates it), plus the four library
-- tables and the four search objects that entry names explicitly. It creates:
--
--   image_asset · work · work_movie · work_series · work_episode ·
--   work_alt_title · edition · media_file · external_id ·
--   service_item_link · service_item_alias ·
--   library · library_source · library_member · library_override ·
--   search_doc · search_fts · search_trgm · search_doc_library ·
--   sync_report
--
-- and rebuilds `write_queue`.
--
-- DELIBERATELY NOT HERE, and which milestone owns it:
--
--   * work_album, work_track, work_credit — the music subtype tables. They
--     land with the NAVIDROME catalogue source, §16.1 sequence position 1 or 2
--     (ADR-0036 keeps every catalogue source out of v0.1 and sequences them one
--     at a time afterwards), and their command sink (Lidarr) is not in any
--     milestone before that.
--   * work_book, work_comic, work_comic_issue — the books/comics subtype
--     tables. They land with KAVITA, §16.1 sequence position 1 or 2
--     (ADR-0035 makes Kavita the comics-and-books source, Komga last).
--
--   The rule is 00001's own, quoted verbatim: "a migration that creates a table
--   nothing queries is a schema claim nobody has tested." Nothing in v0.1
--   writes any of those six.
--
--   THE IRREVERSIBILITY ARGUMENT WAS WEIGHED, EXPLICITLY, BECAUSE IT IS REAL
--   FOR THE ROW ABOVE THEM AND NOT FOR THEM. `work.kind` carries its FULL
--   twelve-member CHECK here — every kind, including the six that only those
--   tables serve — precisely because SQLite cannot ALTER a CHECK and adding a
--   member later is a 12-step rebuild of the LARGEST table in the schema, and
--   because ARCHITECTURE §5.3 makes the kind_byte codec unchangeable once
--   clients cache northbound ids (ADR-0030, ADR-0033). That argument does not
--   transfer to the six tables themselves: each is a brand-new table with no
--   dependants, created later by a plain CREATE TABLE, with no rebuild, no
--   backfill and no codec change. `work_track.edition_id` — ADR-0031's "adding
--   it later is a backfill over the largest table in the schema" — is likewise
--   free here, because the table it is a column of does not exist yet and will
--   be created with the column already on it. Nothing about deferring these
--   six is a one-way door.
--
--   * request, request_quota (v0.2), work_merge, work_relation, tag_rule,
--     tag_alias, tag_implies, saved_filter, playback_state, play_history,
--     playlist, playlist_item, and the RBAC tables (v1.0) — schema.md's "later
--     tables" appendix, unchanged.
--
-- TAG_ASSIGNMENT AND RELEASE_CANDIDATE ARE NOT REBUILT HERE, AND THAT IS A
-- DEFERRAL RATHER THAN AN OVERSIGHT. 00001 drops four foreign keys naming "the
-- migration that ships library sync": write_queue.work_id (restored below),
-- tag_assignment.work_id / .edition_id / .media_file_id, and
-- release_candidate.work_id. Only write_queue's is restored here, because only
-- write_queue's rebuild is what schema.md §10 scheduled and priced. Restoring
-- the other four means two more 12-step rebuilds in the same migration, and
-- neither table has a reader that is blocked on it: release_candidate.work_id
-- is NULL for the whole Search-and-Grab path (00001 says so on the column) and
-- nothing writes tag_assignment yet. Nothing is closing — both are ordinary
-- rebuilds, no cheaper today than in the migration that first writes those
-- columns, which is where the argument for doing them belongs. The comments in
-- 00001 naming this migration are, for those two tables, now stale; they are
-- pointers to a rebuild that is still owed, not to one that happened.
--
-- NO SENTINEL service_instance ROW IS CREATED, and that was checked rather than
-- assumed. schema.md §3 describes `media_file.service_instance_id = 0` as "a
-- reserved sentinel row meaning not reported by any networked service
-- instance", and 00001 creates NO such row — it creates only the id-0 sentinel
-- USER. It is deliberately still not created: the sentinel's one stated use is
-- the Tier 0 Calibre adapter (ARCHITECTURE §11.2), which is v1.0, and ADR-0026
-- forbids UsArr ever scanning a filesystem, so nothing in v0.1 or in §16.1's
-- catalogue sequence writes a media_file with instance 0. Creating it now would
-- put a row on the Services screen with a lying `api_key_enc` and a burned
-- `name`, for a reader that does not exist. It is a plain INSERT when the
-- Calibre adapter lands — no rebuild, no backfill.
--
-- FTS5 IS AN EXTENSION IN THIS BUILD, NOT A BUILT-IN. Measured before this file
-- was written: on ncruces/go-sqlite3 v0.35.3 a connection opened without
-- ext/fts5.Register answers `CREATE VIRTUAL TABLE … USING fts5(…)` with "no
-- such module: fts5". internal/db/sqlite.go now registers it on every
-- connection in both pools, and TestFTS5IsAvailableOnEveryConnection proves it
-- across the read pool. The two virtual tables below are also the two objects
-- in this schema that CANNOT be STRICT — SQLite has no such thing for a virtual
-- table — which is why TestAllTablesAreStrict now asks pragma_table_list for
-- each object's type instead of assuming every row of sqlite_schema is an
-- ordinary table. contentless_delete=1 needs SQLite >= 3.43.0; the driver
-- reports 3.53.4 and a DELETE against a contentless table was executed and
-- accepted before this was committed.
--
-- DDL is copied from docs/reference/schema.md, with four corrections that file
-- now carries:
--
--   1. search_doc_library was declared `WITHOUT ROWID` with no STRICT. Every
--      table in this project is STRICT; it is `STRICT, WITHOUT ROWID` here.
--   2. library_override.field_name read
--      `CHECK (field_name IN (NULL,'title',…))`. That is DB-01 verbatim — the
--      defect TestNullableCheckConstraintsActuallyConstrain exists for: one
--      NULL inside an IN list makes the whole comparison NULL for every
--      non-matching value, and a CHECK passes on NULL, so the constraint
--      accepted 'TOTAL-GARBAGE'. It reads `IS NULL OR … IN (…)` here, and the
--      test gained a case for it.
--   3. sync_report had no DDL anywhere. schema.md's appendix names it v0.1 and
--      "a plain append-only log"; the shape below is derived from its only two
--      specified call sites, sync.md §4 step 5 and §4 guard 1's
--      sync_report{kind,instance,remote_kind,remote_id}. Marked as inference in
--      schema.md rather than presented as prior art.
--   4. §13's "All four tables are in migration 0001" was false. They are here.
--
-- Timestamps are SQLite datetime() text — 'YYYY-MM-DD HH:MM:SS', UTC, no 'T'
-- and no 'Z'. Not ISO-8601. ix_work_added, ix_img_state and ix_wq_runnable all
-- order on such a column lexicographically, and two formats in one column
-- silently break that ordering.
--
-- ──────────────────────────────────────────────────────────────────────────
-- THE WRITE_QUEUE REBUILD — three decisions, and the reasoning for each.
-- ──────────────────────────────────────────────────────────────────────────
--
-- (a) THE `state` CHECK IS DROPPED ENTIRELY, not widened to add
--     'awaiting_choice'. schema.md §10 step 1, FUTURE.md §11 and §11.1 all say
--     "gains awaiting_choice"; this supersedes them and they are updated. The
--     rebuild happens either way, so both options cost exactly the same, and
--     that is the whole reason a choice is available at all. Three arguments
--     decide it:
--
--       * The vocabulary is demonstrably still growing. It has been widened
--         once already in the design (WQ-01 → 'awaiting_choice') and v0.2's
--         request path has a live candidate ('pending' at the request layer is
--         not 'pending' here). A CHECK that has to be renegotiated every time a
--         verb is added is a 12-step rebuild per string literal.
--       * There is shipped precedent for exactly this, twice, for exactly this
--         reason: `audit_log.result` carries no CHECK (00001), and 0003's
--         `provenance.acquisition_state` is `TEXT NOT NULL` with no CHECK and
--         says so in its own header — "SQLite cannot ALTER one and 0001's
--         audit_log FK is what that costs — so the vocabulary lives in
--         internal/store and is enforced there". write_queue.state is the same
--         class of column.
--       * `write_queue.kind`, the verb half of the same row, has never had a
--         CHECK. A constrained state beside an unconstrained kind was already
--         inconsistent.
--
--     What is given up is real and is stated: SQLite will now accept a
--     misspelt state. The enforcement moves to Go, where the vocabulary —
--     pending · inflight · verifying · awaiting_choice · done · failed — is
--     documented and validated on the way in. `fail_reason`'s CHECK is KEPT:
--     that vocabulary is closed (it is the terminal taxonomy, not the lifecycle
--     one) and it is the DB-01 regression witness.
--
-- (b) 'awaiting_choice' IS EXCLUDED from ix_wq_runnable's partial predicate.
--     The reason is written next to the predicate below, because an exclusion
--     reads as an oversight otherwise. This settles the open question schema.md
--     §10 and FUTURE.md §11.1 both carry as "a lean, not a decision".
--
-- (c) ALL THREE indexes are recreated — ux_wq_idem, ix_wq_work, and
--     ix_wq_runnable WITH its partial WHERE predicate. A rebuilt table inherits
--     no indexes, and a partial index silently rebuilt as a full one changes
--     which rows the sweep sees. TestMigrate0005WriteQueueRebuild reads them
--     back out of sqlite_schema and asserts the predicate survived, rather than
--     assuming the CREATE ran.
--
-- (d) `work_id` GETS ITS FOREIGN KEY BACK: REFERENCES work(id) ON DELETE
--     CASCADE, as schema.md §10's canonical DDL always had it and as 00001's
--     dropped-FK comment promised. The audit_log precedent was considered and
--     does NOT transfer. That precedent is not "foreign keys are risky"; it is
--     "an ON DELETE action is an implicit write, so a table that FORBIDS
--     implicit writes cannot carry one" — audit_log has trg_audit_no_update and
--     trg_audit_no_delete, which is what turned its FK into an undeletable
--     user. write_queue has no triggers, is not append-only, and is not
--     history; it already carries two ON DELETE CASCADEs of its own, to user
--     and to service_instance. The precedent says CHECK, and the check passes.
--
--     The genuine objection is the other one: a 7-day tombstone expiry that
--     hard-deletes a `work` would silently take a queued command with it, where
--     no FK would leave the command to fail loudly at the *Arr. Answered on
--     three grounds. First, sync.md §4's write-queue guard is normative and
--     says the sweep may not act on a work that has a row in pending, inflight
--     or verifying — so the collision is one the sweep is already forbidden to
--     cause, and ix_wq_runnable (recreated below) is the index that guard uses.
--     Second, a work reaches hard delete only because the *Arr itself no longer
--     has it, so a surviving "monitor this" command can only ever fail; keeping
--     it produces an alarm in the attention block that the user cannot act on
--     and cannot tell from a real fault. Third, "fails loudly" is aspirational
--     either way — nothing in internal/ reads write_queue yet, so without the
--     FK the dangling row simply sits there, and ix_wq_work returns rows for a
--     work_id with no referent. CASCADE is the same answer the table already
--     gives for its other two parents, and it is the one SQLite can enforce.
--
--     Proven by execution against this migration, not reasoned about:
--     TestMigrate0005WorkIDForeignKey inserts a dangling work_id and asserts it
--     is rejected, enqueues against a real work and deletes the work and
--     asserts the row cascaded, and re-runs TestDeletingAUserStillWorks's shape
--     to confirm nothing here made a user undeletable.
--
-- PRAGMA foreign_keys=OFF IS DELIBERATELY NOT WRITTEN, AND THAT IS NOT HOPE.
-- Two independent reasons, both checked. It would be inert: SQLite documents
-- the pragma as a no-op inside a transaction, and goose runs every migration in
-- one (internal/db/migrate.go says so), so the line would look like protection
-- and provide none — the failure mode sqlite.go's own removed-mmap comment
-- warns about. And it is not needed: the 12-step procedure requires it because
-- DROP TABLE fires an implicit DELETE FROM that cascades into CHILD tables, and
-- `write_queue` is a child of three tables and a parent of NONE — no table in
-- this schema references it. TestMigrate0005WriteQueueRebuild runs
-- PRAGMA foreign_key_check over the whole database afterwards and asserts it
-- returns nothing, which is step 10 of the procedure done where its result can
-- actually fail a build.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up
-- +goose StatementBegin

-- ─── Images · schema.md §12 ──────────────────────────────────────────────────
-- First, because work.poster_asset_id and work.backdrop_asset_id reference it.
-- SQLite resolves foreign keys at DML time, so the order is a convention rather
-- than a requirement — but it is the convention 00001 set and a dependency
-- order is what makes a dangling REFERENCES obvious while reading.

CREATE TABLE image_asset (
  id         INTEGER PRIMARY KEY,
  source_url TEXT NOT NULL UNIQUE,      -- CREDENTIAL-STRIPPED. See security.md §6.
  -- origin_class and origin_service_instance_id are what let the fetcher pick
  -- an SSRF policy from the ROW rather than by pattern-matching the URL string,
  -- which is the fix for the derived-URL class (security.md §2).
  origin_class TEXT NOT NULL CHECK (origin_class IN ('configured','derived','provider')),
  origin_service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE SET NULL,
  role       TEXT NOT NULL,             -- poster|backdrop|banner|cover|logo|headshot
  width INTEGER, height INTEGER,
  thumbhash  BLOB,
  dominant_color TEXT,                  -- available BEFORE thumbhash; see ARCHITECTURE §4.4.1
  cache_key  TEXT NOT NULL,             -- sha256(credential-stripped source_url)[:16]
  etag TEXT, last_modified TEXT,
  fetched_at TEXT, expires_at TEXT,
  state      TEXT NOT NULL DEFAULT 'pending'  -- pending|ready|failed|gone
) STRICT;
CREATE INDEX ix_img_state ON image_asset(state, expires_at);

-- ─── The abstract creative work · schema.md §1 ───────────────────────────────

CREATE TABLE work (
  id                INTEGER PRIMARY KEY,
  -- ALL TWELVE kinds are here from the start, including the six no v0.1 source
  -- writes. This is the one thing in this migration that genuinely cannot wait:
  -- SQLite cannot ALTER a CHECK, so adding a member later rebuilds the largest
  -- table in the schema, re-indexes FTS, rebuilds every client prefix index and
  -- changes the kind_byte codec that ARCHITECTURE §5.3 states is unchangeable
  -- once clients cache northbound ids (ADR-0030, ADR-0033). The subtype tables
  -- for six of these kinds are deliberately NOT here — see the header; a new
  -- table is free later, a new CHECK member is not.
  kind              TEXT NOT NULL CHECK (kind IN (
                      'movie','series','season','episode',
                      'artist','album','track',
                      'book','comic','comic_issue','person','game')),
                    -- 'audiobook' is deliberately NOT a kind: an audiobook is an
                    -- edition of a 'book' work (edition.format='audiobook').
                    -- 'comic' is the SERIES, 'comic_issue' the issue or chapter,
                    -- excluded from the search corpus exactly as episode/track
                    -- are (ADR-0030). There is no 'manga' kind and no Volume
                    -- level. 'person' is a credited human (ADR-0033) and is NOT
                    -- top-level: excluded from the media-type navigation enum,
                    -- from the Tier 1 client prefix index and from the FTS
                    -- corpus, because there is no person screen in any
                    -- milestone. 'artist' still means a MUSIC artist.
  parent_work_id    INTEGER REFERENCES work(id) ON DELETE CASCADE,
                    -- series→season→episode; artist→album→track; book series→book.
  title             TEXT NOT NULL,
  sort_title        TEXT NOT NULL,
  normalized_title  TEXT NOT NULL,      -- v1 algorithm: casefold + NFKD + strip combining
                                        -- marks + strip punctuation + collapse whitespace
  norm_version      INTEGER NOT NULL DEFAULT 1,
  original_title    TEXT,
  year              INTEGER,
  release_date      TEXT,
  overview          TEXT,
  runtime_secs      INTEGER,
  language          TEXT,               -- BCP-47
  popularity        REAL NOT NULL DEFAULT 0,
  rating            REAL,
  status            TEXT,
  -- ON DELETE SET NULL, NOT the default NO ACTION, and the difference is what
  -- decides whether ix_img_state(state, expires_at) has a reader at all. That
  -- index has no plausible use but an expiry sweep, and under NO ACTION the
  -- sweep's `DELETE FROM image_asset WHERE …` fails with "FOREIGN KEY
  -- constraint failed" for every asset any work points at — i.e. for every
  -- poster on the Home screen, which is exactly the set the sweep is for.
  -- Executed before this was changed: the delete really did fail. SET NULL is
  -- also what origin_service_instance_id above and library.sink_service_instance_id
  -- below already use, for the same reason — an evicted parent leaves a row
  -- that renders without it, not a row that cannot be evicted.
  poster_asset_id   INTEGER REFERENCES image_asset(id) ON DELETE SET NULL,
  backdrop_asset_id INTEGER REFERENCES image_asset(id) ON DELETE SET NULL,
  -- Denormalised rollups. Recomputed per dirty-mark flush batch, NOT per child write.
  have_count        INTEGER NOT NULL DEFAULT 0,
  want_count        INTEGER NOT NULL DEFAULT 0,
  size_on_disk      INTEGER NOT NULL DEFAULT 0,
  monitored         INTEGER NOT NULL DEFAULT 0,
  rollup_dirty      INTEGER NOT NULL DEFAULT 0,   -- set by a child write; cleared by the flush
  availability      TEXT,   -- JSON, POLYMORPHIC by medium; carries a "k" discriminator
                            -- as its first key (tier|edition|count). schema.md §1.
  added_at          TEXT,
  created_at        TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at        TEXT NOT NULL DEFAULT (datetime('now')),
  deleted_at        TEXT,               -- soft delete; 7-day tombstone
  content_hash      TEXT
) STRICT;

-- The partial predicates are not tidiness. Without WHERE deleted_at IS NULL the
-- "recently added" and "popular" grids read soft-deleted rows out of the index
-- and filter them afterwards, so the 7-day tombstone window degrades exactly
-- those two views.
CREATE INDEX ix_work_kind_sort ON work(kind, sort_title, id) WHERE deleted_at IS NULL;
CREATE INDEX ix_work_parent    ON work(parent_work_id, kind);
CREATE INDEX ix_work_norm      ON work(normalized_title, year, kind);
CREATE INDEX ix_work_added     ON work(added_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_work_pop       ON work(popularity DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_work_dirty     ON work(rollup_dirty) WHERE rollup_dirty = 1;

-- ─── Subtype tables · schema.md §1.1 — the three v0.1 sources write ──────────

CREATE TABLE work_movie (
  work_id     INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  collection  TEXT, studio TEXT, certification TEXT,
  in_cinemas  TEXT, physical_release TEXT, digital_release TEXT
) STRICT;

CREATE TABLE work_series (
  work_id      INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  series_type  TEXT,                    -- standard|daily|anime
  network      TEXT, air_time TEXT,
  season_count INTEGER,
  ended        INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE work_episode (
  work_id         INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  season_number   INTEGER,              -- NULL allowed: absolute-only anime has no season
  episode_number  INTEGER,              -- NULL allowed: see the CHECK below
  absolute_number INTEGER,
  air_date_utc    TEXT,
  has_file        INTEGER NOT NULL DEFAULT 0,
  CHECK ( (season_number IS NOT NULL AND episode_number IS NOT NULL)
       OR  absolute_number IS NOT NULL )
) STRICT;
CREATE INDEX ix_ep_air ON work_episode(air_date_utc);

CREATE TABLE work_alt_title (
  id         INTEGER PRIMARY KEY,
  work_id    INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  title      TEXT NOT NULL,
  normalized TEXT NOT NULL,
  kind       TEXT NOT NULL,   -- original|translated|alias|acronym|sort
  language   TEXT
) STRICT;
CREATE INDEX ix_alt_work ON work_alt_title(work_id);
CREATE INDEX ix_alt_norm ON work_alt_title(normalized);

-- ─── A specific released form · schema.md §2 ─────────────────────────────────

CREATE TABLE edition (
  id           INTEGER PRIMARY KEY,
  work_id      INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  label        TEXT,      -- "Director's Cut" | "2013 Granta pb" | "Remastered"
  -- format carries the MEDIUM, never the codec: a 2000 UK CD release can sit on
  -- disk as FLAC, so the codec lives on media_file (ADR-0031). This column is
  -- where 'audiobook' lives, which is what makes ebook-vs-audiobook routing a
  -- schema property rather than adapter special-casing, and it is why work.kind
  -- has no 'audiobook' member.
  --
  -- NULL is legal and the CHECK still bites: `NULL IN (…)` is NULL and a CHECK
  -- passes on NULL, so an unset format is accepted while a misspelt one is not.
  -- That is the correct idiom for a nullable column — the DB-01 mistake is
  -- putting NULL INSIDE the list, which poisons every comparison.
  format       TEXT CHECK (format IN (
                 'print','ebook','audiobook','comic',
                 'cbz','cbr','pdf',
                 'bluray','web','remux','dvd',
                 'vinyl','cd','cassette','digital','flac','lossy')),
  language     TEXT,
  quality_tier TEXT,      -- 2160p|1080p|720p|lossless|lossy
  is_primary   INTEGER NOT NULL DEFAULT 0,
  published_at TEXT,
  publisher    TEXT,
  -- ADR-0031: audiobook production properties. Not work properties (different
  -- productions have different narrators) and not media_file properties (a
  -- 30-file audiobook has one runtime).
  narrators    TEXT,       -- JSON array
  duration_seconds INTEGER,
  abridged     INTEGER     -- nullable: unknown is distinct from 'unabridged'
) STRICT;
CREATE INDEX ix_edition_work ON edition(work_id, is_primary DESC);

-- ─── Concrete bytes · schema.md §3 ───────────────────────────────────────────

CREATE TABLE media_file (
  id                  INTEGER PRIMARY KEY,
  work_id             INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  edition_id          INTEGER REFERENCES edition(id) ON DELETE SET NULL,
  service_instance_id INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE CASCADE,
                      -- instance 0 would be the reserved sentinel meaning "not
                      -- reported by any networked service instance". NO SUCH
                      -- ROW EXISTS — not in 00001 and not here; see the header
                      -- for why it is still not created. Its only stated use is
                      -- the v1.0 Tier 0 Calibre adapter. It does NOT mean
                      -- "found by scanning a filesystem": ADR-0026 decides that
                      -- UsArr never touches a filesystem, and nothing in any
                      -- milestone scans.
  remote_file_id      TEXT,
  -- THE THIRD DEFAULTED FOREIGN KEY, JUDGED SEPARATELY FROM THE TWO ABOVE AND
  -- DELIBERATELY LEFT AS NO ACTION. The poster/backdrop pair was changed
  -- because a sweep exists to delete their parent and the default blocked it.
  -- Nothing deletes a `provenance` row: schema.md §6 makes the table immutable
  -- ("never overwrite provenance on upgrade — insert a new row"), it is the
  -- acquisition history the *Arr command path exists to keep, it carries no
  -- expiry column and no index that would serve a sweep, and it is the child
  -- of nothing, so no cascade reaches it either (provenance.user_id carries no
  -- foreign key, for that exact reason). So NO ACTION blocks no sweep here —
  -- what it does is make an accidental `DELETE FROM provenance` fail while a
  -- file still cites it, which for an immutable history table is the outcome
  -- to want. SET NULL would quietly break the file→grab join that
  -- acquisition_state exists to keep honest.
  provenance_id       INTEGER REFERENCES provenance(id),
  path                TEXT NOT NULL,
  content_key         TEXT,   -- physical identity: hex(size_bytes) || ':' || sha256(first 64 KiB)
                              -- DEFERRED: populated from the first milestone that aggregates a
                              -- media server alongside a *Arr. One 64 KiB read per file is not
                              -- free on a NAS, and v0.1 has no cross-instance dedup to perform.
  size_bytes          INTEGER NOT NULL DEFAULT 0,
  quality             TEXT, resolution TEXT,
  video_codec         TEXT, audio_codec TEXT, audio_channels REAL,
  languages           TEXT,   -- JSON array
  release_group       TEXT,
  date_added          TEXT,
  media_info          TEXT    -- JSON blob; NEVER selected in list views
) STRICT;
CREATE INDEX ix_file_work ON media_file(work_id);
-- (service_instance_id, path), never path alone: media_file rows are
-- PER-INSTANCE OBSERVATIONS. Radarr and Jellyfin both indexing one volume is
-- the normal case, and a unique index on path alone rejects the second insert.
CREATE UNIQUE INDEX ux_file_path ON media_file(service_instance_id, path);
CREATE INDEX ix_file_content ON media_file(content_key) WHERE content_key IS NOT NULL;

-- ─── Identity · schema.md §4 ─────────────────────────────────────────────────

CREATE TABLE external_id (
  id         INTEGER PRIMARY KEY,
  work_id    INTEGER REFERENCES work(id)    ON DELETE CASCADE,
  edition_id INTEGER REFERENCES edition(id) ON DELETE CASCADE,
  source     TEXT NOT NULL,   -- tmdb_movie|tmdb_tv|tvdb|imdb|tvmaze|musicbrainz_*|
                              -- openlibrary_work|openlibrary_edition|isbn13|asin|
                              -- goodreads_*|anilist|mal|anidb|wikidata|discogs
  value      TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 1.0,   -- <1.0 = heuristic OR manifest-sourced
  CHECK ((work_id IS NULL) != (edition_id IS NULL))
) STRICT;

-- An ON CONFLICT clause must repeat this expression list LITERALLY, or SQLite
-- raises "ON CONFLICT clause does not match any PRIMARY KEY or UNIQUE
-- constraint". See schema.md §4 for the upsert.
CREATE UNIQUE INDEX ux_extid ON external_id(
  source, value, COALESCE(work_id, -1), COALESCE(edition_id, -1));

-- A violation of THIS index is the merge signal, not an error. Manifest-sourced
-- ids are capped below 1.0 so a Tier 1 manifest can never write a strong-identity
-- row and can never trigger a merge.
CREATE UNIQUE INDEX ux_extid_work_strong ON external_id(source, value)
  WHERE work_id IS NOT NULL AND confidence >= 1.0;

CREATE INDEX ix_extid_work ON external_id(work_id, source);
-- ix_extid_lookup(source, value) is deliberately NOT created: ux_extid already
-- serves (source, value) equality as its leftmost prefix, and a second B-tree
-- write on the importer's hottest insert path is a real cost. Re-add only with
-- a measurement.

-- ─── The M:N service link · schema.md §5 ─────────────────────────────────────

CREATE TABLE service_item_link (
  id                  INTEGER PRIMARY KEY,
  service_instance_id INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE CASCADE,
  work_id             INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  edition_id          INTEGER REFERENCES edition(id) ON DELETE SET NULL,
  remote_id           TEXT NOT NULL,
  remote_kind         TEXT NOT NULL,   -- series|episode|movie|album|track|book|author
  remote_path         TEXT,
  monitored           INTEGER NOT NULL DEFAULT 0,
  quality_profile_id  INTEGER,
  root_folder_path    TEXT,
  -- The three container-predicate inputs library_source.container_kind needs.
  -- All three are stored VERBATIM, as the upstream reported them: this table
  -- never parses, derives or normalises a container value, which is what keeps
  -- membership a deterministic predicate rather than a guess (§6.5 rule 3).
  -- Written by the *Arr sync first; `remote_library_id` is the only container
  -- kind available to the catalogue sources, and per ADR-0036 none of those
  -- ships in v0.1, so it carries no catalogue reader until then.
  remote_library_id   TEXT,          -- the upstream's own library/collection id
  remote_tag_ids      TEXT,          -- JSON array of *Arr tag ids
  remote_subtype      TEXT,          -- the upstream's own sub-classification
  is_authoritative    INTEGER NOT NULL DEFAULT 0,
  is_northbound_canonical INTEGER NOT NULL DEFAULT 0,   -- the pin; see ARCHITECTURE §5.3
  has_file            INTEGER NOT NULL DEFAULT 0,
  remote_updated_at   TEXT,
  remote_hash         TEXT,            -- hash of the SYNCED SUBSET only
  remote_identity_hash TEXT,           -- hash of the remote's external ids, at first sight;
                                       -- the O(1) guard against id resurrection (sync.md §4)
  synced_at           TEXT NOT NULL,
  deleted_at          TEXT             -- 7-day tombstone
) STRICT;
-- ux_sil is only usable IN FULL when remote_kind is known at lookup time, which
-- is why the northbound id encodes it (ARCHITECTURE §5.3). Without it,
-- (service_instance_id, remote_id) degrades to a range scan over every link on
-- the instance — ~400k rows for a 2k-series Sonarr, on every stream resolve.
CREATE UNIQUE INDEX ux_sil ON service_item_link(service_instance_id, remote_kind, remote_id);
CREATE INDEX ix_sil_work ON service_item_link(work_id) WHERE deleted_at IS NULL;
-- The membership derivation walks links per instance per container. Without
-- this it is a full scan of every link on the instance for every library the
-- instance feeds.
CREATE INDEX ix_sil_container ON service_item_link(service_instance_id, remote_library_id)
  WHERE deleted_at IS NULL AND remote_library_id IS NOT NULL;

-- Alias rows keep old northbound ids resolvable when a pinned instance is deleted.
CREATE TABLE service_item_alias (
  old_instance_id INTEGER NOT NULL,
  old_remote_kind TEXT NOT NULL,
  old_remote_id   TEXT NOT NULL,
  link_id         INTEGER NOT NULL REFERENCES service_item_link(id) ON DELETE CASCADE,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (old_instance_id, old_remote_kind, old_remote_id)
) STRICT, WITHOUT ROWID;

-- ─── Libraries · schema.md §13, ARCHITECTURE §6.5, ADR-0026 ──────────────────

CREATE TABLE library (
  id            INTEGER PRIMARY KEY,
  -- Principle 4 / ADR-0019 / ARCHITECTURE §1.3 rule 1. library.user_id records
  -- the OWNER; user_library_access (v1.0) records a GRANT to a different user.
  -- In v0.1 there is one user and every row is user_id 0.
  user_id       INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  slug          TEXT NOT NULL,        -- URL identity for ?lib=. Allocated once from the
                                      -- name and then DURABLE: a rename must not change
                                      -- the permalink.
  kind          TEXT NOT NULL CHECK (kind IN (
                  'movie','series','artist','album','book','comic','game')),
                                      -- Exactly one, required, EDITABLE (§6.5 rule 4).
                                      -- Top-level kinds only: 'season','episode','track',
                                      -- 'comic_issue' and 'person' are deliberately absent —
                                      -- a library of episodes, or of authors, is not a thing.
  formats       TEXT,                 -- JSON array over edition.format, NULL for "any".
                                      -- ["ebook"] and ["audiobook"] over one Audiobookshelf
                                      -- library is the flagship case (§17.8).
  icon          TEXT,
  sort_order    INTEGER NOT NULL DEFAULT 0,   -- 'order' is a SQLite keyword
  enabled       INTEGER NOT NULL DEFAULT 1,
  include_in_search INTEGER NOT NULL DEFAULT 1,
  default_sort  TEXT NOT NULL DEFAULT 'sort_title'
                  CHECK (default_sort IN ('sort_title','added_at','year','popularity')),
  -- The declared request sink (ARCHITECTURE §8.3). A PIN INSIDE the capability
  -- filter, never a bypass: an instance that does not probe
  -- Caps.MediaKinds ∋ (kind, format) and advertise Add cannot be chosen. A NULL
  -- sink is a first-class state, not an error.
  sink_service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE SET NULL,
  sink_quality_profile_id  INTEGER,   -- upstream's own id, verbatim; fetched live when the
                                      -- settings panel opens, never on a render path
  sink_root_folder_path    TEXT,
  sink_tag_ids             TEXT,      -- JSON array of upstream tag ids
  managed_by    TEXT NOT NULL DEFAULT 'auto' CHECK (managed_by IN ('auto','user')),
                                      -- 'auto' = created by the proposal flow and still
                                      -- tracking its source; 'user' = the user edited it, so
                                      -- the proposal flow never rewrites it again.
  orphaned_at   TEXT,                 -- set when the last library_source goes away; the row is
                                      -- RETAINED and shown with its reason (§6.5 rule 5)
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
) STRICT;

CREATE UNIQUE INDEX ux_library_slug ON library(user_id, slug);
CREATE UNIQUE INDEX ux_library_name ON library(user_id, name);
CREATE INDEX ix_library_kind ON library(user_id, kind) WHERE enabled = 1;

-- Reserved row: library.id = 0, "Unfiled".
--
-- It is the MECHANISM §7 invariant 5 is upheld BY, and the invariant itself is
-- upheld by code, not by this schema — see the note above search_doc_library
-- below, which says exactly what the schema does and does not guarantee. What
-- this row buys is a place for the membership derivation to put a work it would
-- otherwise file nowhere, so that the derivation CAN keep the invariant. A work
-- visible through no library matches no scope and disappears from search for
-- everyone including its owner — a failure the old instance-scope column could
-- not produce.
--
-- Never listed on the Libraries screen, never offered in the scope chip, never
-- proposed. `kind` is irrelevant to it; 'movie' satisfies the CHECK and the row
-- is never rendered. Reachable states that need it: a root_folder-scoped
-- library covering only part of an instance, and an `exclude` correction
-- against a work's last remaining library.
--
-- Explicit id 0: an omitted INTEGER PRIMARY KEY would be allocated as 1.
INSERT INTO library (id, user_id, name, slug, kind, managed_by, enabled, include_in_search)
  VALUES (0, 0, 'Unfiled', 'unfiled', 'movie', 'auto', 1, 1);

-- …and the row is PROTECTED, because "reserved" was previously a comment and
-- nothing else: `DELETE FROM library WHERE id = 0` succeeded, nothing recreated
-- the row, and the derivation's only landing place for an unfiled work was gone
-- for the life of the database.
--
-- BEFORE DELETE rather than an AFTER DELETE that re-inserts: re-inserting during
-- user 0's cascade would violate library.user_id's own foreign key at statement
-- end anyway, so it would fail with a worse message and a partly-applied delete.
--
-- THE audit_log FAILURE MODE WAS CHECKED, NOT ASSUMED, because it is the one
-- this project has already been bitten by: a trigger that aborts an IMPLICIT
-- write performed by an ON DELETE action makes some row permanently
-- undeletable (audit_log.actor_user_id, schema.md §9). Two consequences,
-- measured in TestUnfiledLibraryIsProtected:
--
--   * An ORDINARY user stays deletable, with rows in every table that
--     references user(id) — library included. The trigger tests `OLD.id = 0`,
--     and library 0 belongs to user 0, so no other user's cascade touches it.
--   * `DELETE FROM user WHERE id = 0` now FAILS where it used to succeed. That
--     is deliberate and it is the second half of the same defect: user 0 is the
--     shared/system sentinel, and deleting it cascaded away library 0, every
--     shared tag_assignment, every shared write_queue row and every shared
--     release_candidate in one statement. A sentinel that can be deleted is not
--     a sentinel. It is recorded here rather than left to be discovered because
--     it is a behaviour change, not a side effect.
CREATE TRIGGER trg_library_unfiled_no_delete BEFORE DELETE ON library
  WHEN OLD.id = 0
  BEGIN
    SELECT RAISE(ABORT, 'library 0 ("Unfiled") is reserved and cannot be deleted: it is where '
      || 'the membership derivation files a work that belongs to no other library, and without '
      || 'it such a work is invisible in search to every user including its owner. This also '
      || 'blocks DELETE FROM user WHERE id = 0, the shared/system sentinel, deliberately.');
  END;

CREATE TABLE library_source (
  id                  INTEGER PRIMARY KEY,
  library_id          INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
  service_instance_id INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE CASCADE,
  container_kind      TEXT NOT NULL CHECK (container_kind IN (
                        'instance','root_folder','remote_library','tag','series_type')),
  container_ref       TEXT NOT NULL DEFAULT '',
                      -- The container the UPSTREAM ITSELF reported, verbatim.
                      -- '' for container_kind='instance', which has no ref.
  container_identity  TEXT,          -- a stable property of the container (its name, or its
                                     -- own reported uuid) recorded at bind time, so an
                                     -- upstream that reuses ids does not silently rebind the
                                     -- library to a different folder.
  is_metadata_authority INTEGER NOT NULL DEFAULT 0,
  missing_since       TEXT,          -- the upstream stopped reporting this container; the row
                                     -- is retained and shown, never silently dropped
  created_at          TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (library_id, service_instance_id, container_kind, container_ref)
) STRICT;

CREATE INDEX ix_libsrc_instance ON library_source(service_instance_id);
-- At most one metadata authority per library, enforced declaratively rather
-- than in code.
CREATE UNIQUE INDEX ux_libsrc_authority ON library_source(library_id)
  WHERE is_metadata_authority = 1;

CREATE TABLE library_member (
  library_id  INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
  -- sort_title is DENORMALISED from work.sort_title and leads the key after
  -- library_id, which is what makes the scoped keyset page a single covered
  -- seek regardless of selectivity. The price, stated: a title or sort-title
  -- change — including a `field` override on sort_title — rewrites this work's
  -- member rows.
  sort_title  TEXT    NOT NULL,
  work_id     INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  -- Membership is EDITION-GRAINED and has to be: §17.8's flagship case is
  -- Audiobookshelf offered as two libraries, Ebooks and Audiobooks, over its
  -- one mediaType=book library — and a book work with an EPUB edition and an
  -- M4B edition is ONE work row, so a (library_id, work_id) key cannot express
  -- it at all.
  --
  -- 0 = "the whole work, format-independent", a sentinel and NOT a foreign key.
  -- A WITHOUT ROWID table's primary-key columns are implicitly NOT NULL, so
  -- "no edition" cannot be NULL; 0 carries it, matching user_id and
  -- service_instance_id. The cost is that integrity for the non-zero case is
  -- maintained by the derivation rather than by SQLite, so it is a CI assertion
  -- instead (schema.md §13.3).
  edition_id  INTEGER NOT NULL DEFAULT 0,
  added_at    TEXT    NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (library_id, sort_title, work_id, edition_id)
) STRICT, WITHOUT ROWID;

-- Identity, for upsert and for "is this work in this library":
CREATE UNIQUE INDEX ux_libmem_identity ON library_member(library_id, work_id, edition_id);
-- Reverse: "which libraries is this work in", for the item detail page and for
-- the search_doc_library rebuild.
CREATE INDEX ix_libmem_work ON library_member(work_id);

CREATE TABLE library_override (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  verb        TEXT NOT NULL CHECK (verb IN ('exclude','include','relink','field')),
  -- SCOPE. NOT NULL for the two library-scoped verbs, NULL for the two global
  -- ones. relink changes which work a service_item_link points at — UsArr's
  -- identity graph, global by construction. field overrides title, sort title,
  -- year or cover on a work, and a work in two libraries renamed in one of them
  -- either renames globally or renders under two names in two scopes.
  library_id  INTEGER REFERENCES library(id) ON DELETE CASCADE,
  -- TARGETS, deliberately NOT foreign keys. §6.5 rule 1: a correction is never
  -- cleared by a sync, a reconciliation sweep, a tombstone expiry or an id
  -- resurrection — the rule that exists because LazyLibrarian's ignored books
  -- come back after an author rescan. ON DELETE CASCADE would let a tombstone
  -- expiry destroy the correction, which is that failure reproduced; SET NULL
  -- would orphan it silently. target_identity_hash is the durable key and
  -- ix_ovr_hash is how a resurrected row re-attaches to it.
  work_id     INTEGER,
  link_id     INTEGER,
  target_identity_hash TEXT NOT NULL,
  -- field verb only. `field_name` cannot be 'kind': §6.4 derives work.kind from
  -- the library's declared kind, so the way to change it is to change the
  -- library's kind (§6.5 rule 4), which re-derives membership.
  --
  -- The nullable form is `IS NULL OR … IN (…)`, NOT `IN (NULL, …)`. schema.md
  -- §13.4 had the latter, which is DB-01 exactly: one NULL inside an IN list
  -- makes the comparison NULL for every non-matching value and a CHECK passes
  -- on NULL, so the constraint accepted anything at all.
  field_name  TEXT CHECK (field_name IS NULL
                          OR field_name IN ('title','sort_title','year','cover')),
  field_value TEXT,
  -- relink verb only
  relink_to_work_id INTEGER,
  reason      TEXT,
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),

  -- exclude/include are library-scoped; relink/field are global. Enforced, not
  -- documented. Deleting a library therefore cascades away its exclude/include
  -- rows and nothing else — corrections to identity and display survive it,
  -- which is what the delete-confirmation copy (§17.8) has to say.
  CHECK ( (verb IN ('exclude','include')) = (library_id IS NOT NULL) ),
  CHECK ( (verb = 'field')  = (field_name IS NOT NULL) ),
  CHECK ( (verb = 'relink') = (link_id IS NOT NULL AND relink_to_work_id IS NOT NULL) ),
  CHECK ( verb = 'relink' OR work_id IS NOT NULL )
) STRICT;

CREATE UNIQUE INDEX ux_ovr_member ON library_override(library_id, work_id, verb, user_id)
  WHERE verb IN ('exclude','include');
CREATE UNIQUE INDEX ux_ovr_field  ON library_override(work_id, field_name, user_id)
  WHERE verb = 'field';
CREATE UNIQUE INDEX ux_ovr_relink ON library_override(link_id, user_id)
  WHERE verb = 'relink';
CREATE INDEX ix_ovr_hash ON library_override(target_identity_hash);
CREATE INDEX ix_ovr_user ON library_override(user_id, created_at DESC);

-- ─── Search · schema.md §7 ───────────────────────────────────────────────────

-- search_doc.rowid is THE allocator for all three tables. The id is allocated
-- by inserting here first, then inserted EXPLICITLY into both FTS tables in the
-- same transaction: RRF fuses on rowid, and a single implicit rowid fuses
-- unrelated documents.
CREATE TABLE search_doc (
  rowid        INTEGER PRIMARY KEY,
  work_id      INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  kind         TEXT NOT NULL,           -- top-level kinds only; 'season','episode','track',
                                        -- 'comic_issue' and 'person' are excluded (§7 inv. 3)
  popularity   REAL NOT NULL DEFAULT 0,
  in_library   INTEGER NOT NULL DEFAULT 0,
  title_idf    REAL NOT NULL DEFAULT 0,
  norm_title   TEXT NOT NULL
) STRICT;
CREATE INDEX ix_sd_work ON search_doc(work_id);

-- Library visibility is a JUNCTION TABLE, not a column on search_doc, and that
-- is load-bearing rather than tidy-minded. ARCHITECTURE §8.2 requires that
-- permission filtering happen IN THE INDEX JOIN, never after it, because a
-- post-filter silently breaks keyset page sizes and leaks existence through
-- result counts and ranking positions. A JSON array in a TEXT column cannot
-- participate in an index join: filtering it needs json_each() or LIKE, both
-- scans, and no index on the column would help.
--
-- The WITHOUT ROWID key is (library_id, doc_rowid) in that order because the
-- scope is the outer filter and the doc set is the inner range; ix_sdl_doc
-- serves the reverse. STRICT is added here — schema.md declared this one table
-- WITHOUT ROWID and forgot it.
--
-- ⚠️ TWO OF §7'S SIX INVARIANTS ARE CODE INVARIANTS, NOT SCHEMA GUARANTEES, AND
-- THIS COMMENT SAYS SO RATHER THAN LETTING THE Unfiled ROW ABOVE IMPLY
-- OTHERWISE. Stated plainly, with the reason each is undeclarable and the code
-- that will owe it:
--
--   * INVARIANT 5 — every search_doc row has at least one search_doc_library
--     row. SQLite has no "at least one child" constraint: at INSERT time the
--     doc necessarily exists before its junction row (that ordering is forced
--     by search_doc_library's own foreign key), so a BEFORE/AFTER INSERT
--     trigger would either fire too early to be true or reject the only legal
--     insert order. And the two ways it BREAKS are both cascades — DELETE FROM
--     library, and DELETE FROM user, which reaches library through
--     library.user_id — where an AFTER DELETE trigger on the junction cannot
--     tell "the doc lost its last scope" from "the doc is being deleted too".
--     Deleting library 0 is now blocked (see the trigger above), which closes
--     the single worst path, but any OTHER library's deletion can still strand
--     a doc whose only scope was that library.
--     OWED BY: the search-document builder — the code that writes search_doc,
--     search_fts and search_trgm — which must re-file a stranded doc into
--     library 0 in the same transaction as any library delete. Nothing writes
--     these three tables yet, so the debt is recorded here and asserted in CI
--     rather than pretended away: TestSearchDocVisibilityIsACodeInvariant
--     executes both breaking paths and pins the invariant's own query.
--
--   * INVARIANT 2 — count(search_fts) == count(search_trgm) == count(search_doc).
--     Same class, worse: the FTS5 tables are virtual, cannot carry a foreign
--     key or a CHECK, and cannot be the target of a trigger. Nothing in SQLite
--     can hold three row counts equal. It is upheld by the same builder, in the
--     same transaction, and the same test pins the query.
CREATE TABLE search_doc_library (
  library_id INTEGER NOT NULL REFERENCES library(id)       ON DELETE CASCADE,
  doc_rowid  INTEGER NOT NULL REFERENCES search_doc(rowid) ON DELETE CASCADE,
  PRIMARY KEY (library_id, doc_rowid)
) STRICT, WITHOUT ROWID;
CREATE INDEX ix_sdl_doc ON search_doc_library(doc_rowid);

-- +goose StatementEnd

-- The two FTS5 virtual tables get their own statement blocks so the reason each
-- one is shaped as it is sits next to it rather than in a wall of DDL.
--
-- contentless_delete=1 REQUIRES SQLite >= 3.43.0. Without it, every work delete
-- and every title change is impossible to mirror — a plain contentless table
-- answers "cannot DELETE from contentless fts5 table" — and §7 invariant 2
-- (count(search_fts) == count(search_trgm) == count(search_doc)) cannot hold.
-- The driver reports 3.53.4 and the DELETE was executed before this was
-- committed. FTS5 itself is a registered extension in this build, not a
-- built-in; see internal/db/sqlite.go.
--
-- These two are the ONLY objects in this schema that are not STRICT. SQLite has
-- no STRICT for a virtual table.
-- +goose StatementBegin
CREATE VIRTUAL TABLE search_fts USING fts5(
  title, original_title, alt_titles, people, overview,
  content='', contentless_delete=1,
  tokenize='unicode61 remove_diacritics 2',
  prefix='2 3 4'
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE VIRTUAL TABLE search_trgm USING fts5(
  title, alt_titles,
  content='', contentless_delete=1,
  tokenize='trigram'
);
-- +goose StatementEnd

-- +goose StatementBegin

-- ─── The sync report · schema.md appendix ("v0.1, emitted by the sweep") ─────
--
-- 🔍 INFERENCE, and marked as such: schema.md gives this table no DDL anywhere.
-- The shape below is derived from its only two specified call sites —
-- sync.md §4 step 5 ("emit a sync_report row" per sweep) and §4 guard 1's
-- `sync_report{kind: "id_reused", instance, remote_kind, remote_id}` — plus
-- §4's tombstone/drift counts. schema.md §12.1 now carries it, flagged the same
-- way, so the two do not drift.
--
-- Append-only by convention, NOT by trigger. audit_log's triggers exist because
-- it is a tamper-evidence chain; this is an operational log the sweep writes
-- and the Services screen reads, and a trigger here would only recreate
-- audit_log's foreign-key problem for no security benefit.
CREATE TABLE sync_report (
  id                  INTEGER PRIMARY KEY,
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  -- No CHECK, for decision (a)'s reason: this vocabulary is the newest and
  -- least settled in the schema. sync.md names 'id_reused'; the sweep will add
  -- its own as channels 3b and 4 are built.
  kind                TEXT NOT NULL,
  remote_kind         TEXT,
  remote_id           TEXT,
  -- No foreign key, for provenance.user_id's reason: this is a HISTORICAL id.
  -- "Work 4412's link was rebound because Sonarr reused id 842" must survive
  -- work 4412 being deleted, and CASCADE would erase the report of exactly the
  -- event that most needs reporting.
  work_id             INTEGER,
  detail              TEXT,   -- JSON. Upstream response text reaches this column, so it is
                              -- REDACTED on the way in — security.md §5 and R-08: Mylar3
                              -- returns indexer API keys in a response BODY and Kavita
                              -- carries its key in a URL PATH segment.
  created_at          TEXT NOT NULL DEFAULT (datetime('now'))
) STRICT;
-- The one read is "this instance's recent reports, newest first" on the
-- Services screen, so the equality column leads and created_at DESC trails it.
CREATE INDEX ix_sync_report_instance ON sync_report(service_instance_id, created_at DESC);

-- ─── The write_queue rebuild · schema.md §10 ─────────────────────────────────
--
-- The 12-step procedure. Steps 1 and 12 (PRAGMA foreign_keys OFF/ON) are
-- deliberately absent and the header says why in full: the pragma is a
-- documented no-op inside a transaction and goose runs each migration in one,
-- and write_queue is a child of three tables and a parent of none, so the DROP
-- fires no cascades. Step 10 (PRAGMA foreign_key_check) is asserted in
-- TestMigrate0005WriteQueueRebuild, where a violation can actually fail a
-- build, rather than written here where its result would be discarded.

CREATE TABLE write_queue_new (
  id              INTEGER PRIMARY KEY,
  idempotency_key TEXT NOT NULL,          -- client ULID, or server-derived for northbound
  user_id         INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,          -- add|delete|monitor|unmonitor|grab|tag_add|refresh
  -- RESTORED. 00001 dropped this foreign key with a comment naming this
  -- migration; the header argues the case, including why audit_log's precedent
  -- does not transfer and what CASCADE gives up.
  work_id             INTEGER REFERENCES work(id) ON DELETE CASCADE,
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  payload         TEXT NOT NULL,          -- JSON
  -- NO CHECK. Decision (a) in the header: the lifecycle vocabulary — pending ·
  -- inflight · verifying · awaiting_choice · done · failed — is still growing,
  -- SQLite cannot ALTER a CHECK, and audit_log.result and 0003's
  -- provenance.acquisition_state are the shipped precedent for moving exactly
  -- this kind of vocabulary into Go. 'awaiting_choice' is the state a two-phase
  -- sink parks in while a PERSON decides (FUTURE.md §11); nothing produces it
  -- in v0.1 — the seam ships, the feature does not.
  state           TEXT NOT NULL DEFAULT 'pending',
  -- KEPT, and note the shape. `x IN (NULL, …)` yields NULL rather than FALSE
  -- when x matches nothing, and a CHECK passes on NULL, so putting NULL in the
  -- list made this constraint accept every value (DB-01). NULL is tested
  -- separately. This vocabulary is closed — it is the terminal taxonomy, not
  -- the lifecycle one — and this column is the regression witness
  -- TestNullableCheckConstraintsActuallyConstrain reads.
  fail_reason     TEXT CHECK (fail_reason IS NULL OR fail_reason IN (
                    'rejected','unknown','exhausted')),
  attempts        INTEGER NOT NULL DEFAULT 0,
  max_attempts    INTEGER NOT NULL DEFAULT 6,
  next_attempt_at TEXT,
  verify_until    TEXT,                   -- 15-minute TTL on `verifying`
  last_error      TEXT,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  settled_at      TEXT
) STRICT;

-- The copy, and the guard in front of it.
--
-- `work` is created EMPTY by this very migration, so EVERY pre-existing
-- non-NULL work_id is dangling by construction and the restored foreign key
-- cannot accept one. That leaves exactly two honest outcomes — abort, or keep
-- the value — and keeping it is not available at any price: `PRAGMA
-- foreign_keys=OFF` is a documented no-op inside goose's transaction, and
-- `PRAGMA defer_foreign_keys` only moves the same failure to COMMIT with a
-- worse message.
--
-- THIS COLUMN USED TO BE COPIED THROUGH
-- `CASE WHEN EXISTS (SELECT 1 FROM work …) THEN work_id END`, WHICH IS THE
-- THIRD OUTCOME AND THE WRONG ONE. It converts every such value to NULL with
-- no error, no log and no sync_report row — reproduced at 2 of 2 rows, and it
-- recurs on every down/up cycle. work_id is the only record of which work a
-- queued command was for; losing it silently is unrecoverable AND invisible,
-- which is strictly worse than a migration that stops and says so.
--
-- The guard is a BEFORE INSERT trigger because SQLite answers a bare
-- `SELECT RAISE(ABORT, …)` with "RAISE() may only be used within a
-- trigger-program" (executed on this driver, 2026-08-17). RAISE's message is an
-- expression rather than a bare literal here, which is what lets it carry the
-- COUNT — also executed, not assumed. The trigger fires ahead of the foreign
-- key, so the operator gets this message and not "FOREIGN KEY constraint
-- failed".
--
-- Expected count today is zero: nothing in internal/ writes write_queue
-- (internal/httpapi/grabs.go:58 says so; the standalone bench binary in
-- internal/db/spike/ reads it). The guard is what makes that an observation
-- rather than a bet.
CREATE TRIGGER trg_wq_rebuild_guard BEFORE INSERT ON write_queue_new
  WHEN NEW.work_id IS NOT NULL
  BEGIN
    SELECT RAISE(ABORT, 'migration 0005 aborted rather than silently discard data: ' ||
      (SELECT count(*) FROM write_queue WHERE work_id IS NOT NULL) ||
      ' write_queue row(s) carry a non-NULL work_id, and this migration creates ' ||
      'the `work` table empty, so every one of them references a work that does ' ||
      'not exist. Nothing in UsArr writes write_queue yet, so these rows cannot ' ||
      'run: list them with "SELECT id, kind, state, work_id FROM write_queue ' ||
      'WHERE work_id IS NOT NULL", then either DELETE them or set their work_id ' ||
      'to NULL, and re-run the migration.');
  END;

INSERT INTO write_queue_new (
  id, idempotency_key, user_id, kind, work_id, service_instance_id, payload,
  state, fail_reason, attempts, max_attempts, next_attempt_at, verify_until,
  last_error, created_at, settled_at)
SELECT
  id, idempotency_key, user_id, kind, work_id, service_instance_id, payload,
  state, fail_reason, attempts, max_attempts, next_attempt_at, verify_until,
  last_error, created_at, settled_at
FROM write_queue;

DROP TRIGGER trg_wq_rebuild_guard;
DROP TABLE write_queue;
ALTER TABLE write_queue_new RENAME TO write_queue;

-- All three indexes, recreated by hand. A rebuilt table inherits none of them.
CREATE UNIQUE INDEX ux_wq_idem ON write_queue(user_id, idempotency_key);
CREATE INDEX ix_wq_work ON write_queue(work_id, state);

-- 'awaiting_choice' IS DELIBERATELY NOT IN THIS PREDICATE, and this comment is
-- the point: an exclusion reads as an oversight otherwise.
--
-- A row waiting on a person is not runnable. This partial index is what the
-- retry sweep walks AND what sync.md §4's reconciliation guard tests, so a
-- state listed here is a state that gets claimed by a worker and exposed to the
-- verify_until TTL — which would resolve a user who wandered off mid-choice to
-- fail_reason='unknown'. That is precisely the defect the state exists to fix,
-- reintroduced through the index. The predicate therefore stays byte-identical
-- to 00001's.
--
-- ⚠️ AND THE CONSTRAINT THE PREDICATE PUTS ON ITS CALLERS, which the reasoning
-- above re-affirmed across fifteen lines without ever stating: a partial index
-- is only usable by a query whose WHERE clause IMPLIES the index's predicate,
-- and SQLite does not derive that implication — it matches the predicate
-- syntactically. So the sweep must spell the IN list verbatim.
--
--   WHERE state IN ('pending','inflight','verifying') AND next_attempt_at <= ?
--       → SEARCH write_queue USING INDEX ix_wq_runnable (state=?)
--   WHERE state = 'pending' AND next_attempt_at <= ?
--       → SCAN write_queue                                  ← a full scan
--
-- The second is the obvious way to write "claim the next pending row", it is
-- logically a strict subset of the first, and it reaches this index not at all.
-- Both plans are executed and pinned by TestWriteQueueRunnableNeedsTheVerbatimINList
-- so the constraint is discovered in CI rather than in production.
--
-- Still open, and not decided here: terminal rows (`done`/`failed`) are
-- unreachable by any index, so the settled-row sweep scans (REVIEW-LOG DL-11).
-- That entry says the rebuild is the cheapest moment to add a fourth index; it
-- is not — a CREATE INDEX needs no rebuild — so it is left to whoever writes
-- the sweep, with a measurement.
CREATE INDEX ix_wq_runnable ON write_queue(state, next_attempt_at)
  WHERE state IN ('pending','inflight','verifying');

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
--
-- The write_queue rebuild is reversed FIRST and in full. Leaving the restored
-- foreign key in place while dropping `work` would leave a dangling REFERENCES
-- clause that SQLite does not complain about until the next insert — exactly
-- the failure 00001's header dropped those foreign keys to avoid.
-- +goose StatementBegin

CREATE TABLE write_queue_old (
  id              INTEGER PRIMARY KEY,
  idempotency_key TEXT NOT NULL,
  user_id         INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,
  work_id             INTEGER,
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  payload         TEXT NOT NULL,
  state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN (
                    'pending','inflight','verifying','done','failed')),
  fail_reason     TEXT CHECK (fail_reason IS NULL OR fail_reason IN (
                    'rejected','unknown','exhausted')),
  attempts        INTEGER NOT NULL DEFAULT 0,
  max_attempts    INTEGER NOT NULL DEFAULT 6,
  next_attempt_at TEXT,
  verify_until    TEXT,
  last_error      TEXT,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  settled_at      TEXT
) STRICT;

-- 0005 may have accepted states 0001's CHECK forbids — 'awaiting_choice' above
-- all, which is the state 0005 exists to permit. They cannot come back, so
-- something has to give.
--
-- THIS FILTERED THEM OUT WITH `WHERE state IN (…)` AND THAT WAS THE WRONG
-- CHOICE, for the same reason as the work_id guard in the Up block above: it
-- deleted rows with no error and no count (3 in, 1 out). A row that vanishes is
-- unrecoverable; a row whose state was remapped is still there to look at.
--
-- So every row is copied, and a state 0001 cannot hold is remapped to 'failed'
-- — the terminal state, so nothing re-runs it — with the original recorded in
-- last_error, which is free text and is where a human looks. A Down followed by
-- an Up therefore preserves row count, ids and every other column, which is
-- what makes the round trip testable at all.
--
-- The alternative considered and rejected: abort the Down, as the Up aborts.
-- The Up aborts because it would otherwise destroy the ONLY copy of a value.
-- Here the value is preserved, in a column built for exactly this, and a Down
-- that cannot run is a local testing tool that does not work — which is the one
-- thing this block is for (docs/CONFIGURATION.md §6.3: downgrades are not a
-- supported user path).
INSERT INTO write_queue_old (
  id, idempotency_key, user_id, kind, work_id, service_instance_id, payload,
  state, fail_reason, attempts, max_attempts, next_attempt_at, verify_until,
  last_error, created_at, settled_at)
SELECT
  id, idempotency_key, user_id, kind, work_id, service_instance_id, payload,
  CASE WHEN state IN ('pending','inflight','verifying','done','failed')
       THEN state ELSE 'failed' END,
  fail_reason, attempts, max_attempts, next_attempt_at, verify_until,
  CASE WHEN state IN ('pending','inflight','verifying','done','failed')
       THEN last_error
       ELSE COALESCE(last_error || ' | ', '')
            || 'migration 0005 Down remapped state=''' || state || ''' to ''failed'': '
            || 'migration 0001''s CHECK cannot hold it' END,
  created_at, settled_at
FROM write_queue;

DROP TABLE write_queue;
ALTER TABLE write_queue_old RENAME TO write_queue;

CREATE UNIQUE INDEX ux_wq_idem ON write_queue(user_id, idempotency_key);
CREATE INDEX ix_wq_runnable ON write_queue(state, next_attempt_at)
  WHERE state IN ('pending','inflight','verifying');
CREATE INDEX ix_wq_work ON write_queue(work_id, state);

DROP TABLE IF EXISTS sync_report;
DROP TABLE IF EXISTS search_doc_library;
DROP TABLE IF EXISTS search_trgm;
DROP TABLE IF EXISTS search_fts;
DROP TABLE IF EXISTS search_doc;
DROP TABLE IF EXISTS library_override;
DROP TABLE IF EXISTS library_member;
DROP TABLE IF EXISTS library_source;
DROP TABLE IF EXISTS library;
DROP TABLE IF EXISTS service_item_alias;
DROP TABLE IF EXISTS service_item_link;
DROP TABLE IF EXISTS external_id;
DROP TABLE IF EXISTS media_file;
DROP TABLE IF EXISTS edition;
DROP TABLE IF EXISTS work_alt_title;
DROP TABLE IF EXISTS work_episode;
DROP TABLE IF EXISTS work_series;
DROP TABLE IF EXISTS work_movie;
DROP TABLE IF EXISTS work;
DROP TABLE IF EXISTS image_asset;

-- +goose StatementEnd
