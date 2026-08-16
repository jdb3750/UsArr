-- Migration 0001 — the tables the shipped code uses.
--
-- SCOPE. This is NOT the whole v0.1 schema. It carries exactly the tables the
-- code in this commit reads or writes: identity (user, session,
-- client_credential, audit_log), services (service_instance), the
-- search-and-grab path (release_candidate, provenance), the command sink
-- (write_queue) and tags (tag, tag_assignment).
--
-- Deliberately absent, landing in the migration that ships library sync:
-- work, work_movie, work_series, work_episode, edition, media_file,
-- external_id, service_item_link, service_item_alias, search_doc, search_fts,
-- image_asset, sync_report. A migration that creates a table nothing queries is
-- a schema claim nobody has tested.
--
-- Columns here that reference one of those tables keep the column and DROP the
-- foreign key, with a "-- why" comment naming the migration that restores it.
-- SQLite resolves foreign keys at DML time, so a dangling REFERENCES clause
-- would not fail until the first insert — which is exactly the wrong place to
-- find out.
--
-- DDL is copied from docs/reference/schema.md. STRICT tables throughout,
-- SQLite >= 3.43.0.
--
-- Timestamps are SQLite datetime() text — 'YYYY-MM-DD HH:MM:SS', UTC, no 'T'
-- and no 'Z'. That is NOT ISO-8601, which this header used to claim while every
-- DEFAULT below reads datetime('now'). The columns are ordered lexicographically
-- by ix_audit_ts / ix_rel_expiry / ix_wq_runnable, and lexicographic ordering
-- breaks if two formats ever share a column, so there is exactly one format.
--
-- A merged migration is NEVER edited. Write a new one.

-- +goose Up
-- +goose StatementBegin

-- ─── Users, sessions, credentials · schema.md §9 ─────────────────────────────

CREATE TABLE user (
  id            INTEGER PRIMARY KEY,   -- id 0 is the reserved shared/system sentinel
  username      TEXT NOT NULL UNIQUE,
  display_name  TEXT, email TEXT,
  auth_source   TEXT NOT NULL CHECK (auth_source IN ('local','jellyfin','plex','tailscale')),
  external_id   TEXT,
  password_hash TEXT,                  -- full PHC string, Argon2id. NULL for external users.
  is_owner      INTEGER NOT NULL DEFAULT 0,
  is_disabled   INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  last_login_at TEXT,
  UNIQUE (auth_source, external_id)
) STRICT;

-- The shared/system sentinel. tag_assignment.user_id is NOT NULL DEFAULT 0
-- REFERENCES user(id), so this row must exist before any shared tag can be
-- written: NULL is not usable as an indexed equality, which is why the sentinel
-- is a real row rather than a NULL. It is disabled and has no password, so it
-- can never be logged into.
INSERT INTO user (id, username, display_name, auth_source, is_owner, is_disabled)
  VALUES (0, '_system', 'System', 'local', 0, 1);

CREATE TABLE session (
  id            TEXT PRIMARY KEY,       -- hash of the cookie value
  user_id       INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL CHECK (kind IN ('web','device')),
  device_label  TEXT, user_agent TEXT, ip TEXT,
  created_at    TEXT NOT NULL, last_seen_at TEXT NOT NULL,
  idle_expires_at TEXT NOT NULL, absolute_expires_at TEXT NOT NULL,
  sudo_until    TEXT,                   -- ARCHITECTURE §12.1 sudo mode
  revoked_at    TEXT
) STRICT;
CREATE INDEX ix_session_user ON session(user_id, revoked_at);

CREATE TABLE client_credential (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
  label        TEXT NOT NULL,
  protocol     TEXT NOT NULL CHECK (protocol IN ('subsonic','opds','native')),
  key_prefix   TEXT NOT NULL,           -- first 8 chars; the lookup key AND what appears in logs
  key_hash     BLOB NOT NULL,           -- HMAC-SHA256(server_key, full_key). NOT Argon2id.
  created_at   TEXT NOT NULL DEFAULT (datetime('now')),
  last_used_at TEXT, revoked_at TEXT
) STRICT;
CREATE UNIQUE INDEX ux_cc_prefix ON client_credential(key_prefix);
CREATE INDEX ix_cc_user ON client_credential(user_id, revoked_at);

CREATE TABLE audit_log (
  id INTEGER PRIMARY KEY,
  ts TEXT NOT NULL DEFAULT (datetime('now')),
  -- actor_user_id carries NO foreign key, deliberately. It is a HISTORICAL id:
  -- the user it names may since have been deleted, and the whole point of this
  -- log is that it still says who did the thing.
  --
  -- It used to be `REFERENCES user(id) ON DELETE SET NULL`, which made deleting
  -- any user who had ever acted IMPOSSIBLE. ON DELETE SET NULL performs an
  -- implicit UPDATE on audit_log, trg_audit_no_update below RAISE(ABORT)s it,
  -- and the whole DELETE FROM user fails. Since a login writes an audit row,
  -- that is every user.
  --
  -- ON DELETE NO ACTION does NOT fix it — verified by execution: it leaves the
  -- constraint violated instead, so the DELETE still fails, just with
  -- "FOREIGN KEY constraint failed" rather than "audit_log is append-only".
  -- Dropping the reference is the only option that both permits the delete and
  -- keeps the actor recorded. SET NULL would have destroyed exactly the record
  -- the log exists for (security.md §6: "who deleted this").
  actor_user_id INTEGER,
  actor_ip TEXT, action TEXT NOT NULL,
  target_type TEXT, target_id TEXT,
  result TEXT NOT NULL, metadata_json TEXT,   -- secret VALUES never appear here; see security.md §5
  prev_hash TEXT                              -- rolling chain: sha256(prev_hash || row) — makes
                                              -- tampering EVIDENT, not impossible
) STRICT;
CREATE INDEX ix_audit_ts ON audit_log(ts DESC);

-- +goose StatementEnd

-- Triggers carry their own BEGIN…END, so each needs its own statement block.
-- +goose StatementBegin
CREATE TRIGGER trg_audit_no_update BEFORE UPDATE ON audit_log
  BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_audit_no_delete BEFORE DELETE ON audit_log
  BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;
-- +goose StatementEnd

-- +goose StatementBegin

-- ─── Services · schema.md §5 ─────────────────────────────────────────────────

CREATE TABLE service_instance (
  id            INTEGER PRIMARY KEY,   -- NEVER reused, even after delete
  kind          TEXT NOT NULL,         -- sonarr|radarr|lidarr|whisparr|prowlarr|lazylibrarian|
                                       -- jellyfin|navidrome|audiobookshelf|komga|kavita|
                                       -- <manifest name>
  role          TEXT NOT NULL DEFAULT 'library' CHECK (role IN (
                  'library','acquisition','indexer','download_client')),
  name          TEXT NOT NULL UNIQUE,  -- "Radarr 4K"
  base_url      TEXT NOT NULL,
  url_base      TEXT NOT NULL DEFAULT '',
  api_key_enc   BLOB NOT NULL,         -- versioned envelope; see reference/security.md §1
  kek_id        INTEGER NOT NULL DEFAULT 1,   -- plain column so rotation can resume
  api_version   TEXT,                  -- v1 | v3 | v5 — per app, NOT the app version
  verify_tls    INTEGER NOT NULL DEFAULT 1,
  tls_spki_pin  BLOB,                  -- TOFU pin; see ARCHITECTURE §11.3
  enabled       INTEGER NOT NULL DEFAULT 1,
  priority      INTEGER NOT NULL DEFAULT 0,
  managed_by    TEXT NOT NULL DEFAULT 'ui' CHECK (managed_by IN ('ui','env','file')),
  -- health / circuit breaker
  health_state  TEXT NOT NULL DEFAULT 'unknown',
  breaker_state TEXT NOT NULL DEFAULT 'closed',
  breaker_until TEXT,
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  last_ok_at    TEXT, last_error TEXT,
  clock_skew_secs INTEGER NOT NULL DEFAULT 0,   -- from the Date response header (sync.md §3)
  -- identity generation guard (sync.md §4)
  identity_fingerprint TEXT,
  identity_epoch       INTEGER NOT NULL DEFAULT 1,
  needs_reidentification INTEGER NOT NULL DEFAULT 0,
  max_remote_id_seen   TEXT,           -- JSON {remote_kind: max_id}
  capabilities  TEXT,                  -- JSON Caps, probed live
  last_full_sync_at  TEXT,
  last_delta_sync_at TEXT,
  config_json        TEXT,
  deleted_at         TEXT              -- tombstone; id stays burned
) STRICT;

-- ─── Provenance and release candidates · schema.md §6 ────────────────────────

CREATE TABLE provenance (
  id                 INTEGER PRIMARY KEY,
  protocol           TEXT NOT NULL CHECK (protocol IN (
                       'usenet','torrent','irc','direct','manual','unknown')),
  indexer_name       TEXT, indexer_id INTEGER,
  indexer_privacy    TEXT,             -- public|semiPrivate|private
  indexer_categories TEXT,             -- JSON array of raw Newznab cat ints — DO NOT collapse
  indexer_flags      TEXT,             -- JSON array
  download_client_type TEXT,           -- Sabnzbd|NzbGet|QBittorrent|Deluge|Transmission|RTorrent
  download_client_name TEXT,
  download_id        TEXT,             -- THE JOIN KEY: nzo_id / torrent infohash
  torrent_info_hash  TEXT,
  nzb_info_url TEXT, download_url TEXT, release_guid TEXT,
  release_title      TEXT NOT NULL,    -- the raw scene/P2P name, VERBATIM, FOREVER
  release_group      TEXT,
  quality_source     TEXT, quality_resolution TEXT,
  video_codec TEXT, audio_codec TEXT, audio_channels TEXT,
  edition_label      TEXT,
  languages          TEXT,
  proper_repack      INTEGER,
  published_at TEXT, grabbed_at TEXT, imported_at TEXT,
  source_system    TEXT NOT NULL,      -- sonarr|radarr|prowlarr|manual|filesystem
  source_record_id TEXT,
  confidence       REAL NOT NULL DEFAULT 1.0
) STRICT;
CREATE INDEX ix_prov_protocol ON provenance(protocol);
CREATE INDEX ix_prov_indexer  ON provenance(indexer_name);
CREATE INDEX ix_prov_dlid     ON provenance(download_id);

CREATE TABLE release_candidate (
  id                  INTEGER PRIMARY KEY,
  work_id             INTEGER,                                          -- NULL in Search-and-Grab
                      -- FK to work(id) ON DELETE CASCADE dropped: `work` lands with library sync.
  service_instance_id INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE CASCADE,
  guid TEXT NOT NULL, title TEXT NOT NULL,
  indexer TEXT, indexer_id INTEGER, protocol TEXT,
  categories TEXT,
  size_bytes INTEGER, seeders INTEGER, leechers INTEGER, age_days REAL,
  quality TEXT, download_url TEXT, info_url TEXT, info_hash TEXT,
  download_client_id INTEGER,          -- Prowlarr ReleaseResource.downloadClientId
  raw_release_json TEXT NOT NULL,      -- the full ReleaseResource, needed verbatim for the grab
  rejected INTEGER NOT NULL DEFAULT 0, rejection_reasons TEXT,
  fetched_at TEXT NOT NULL,
  expires_at TEXT NOT NULL             -- <= 25 min for Prowlarr; see ARCHITECTURE §8.4
) STRICT;
CREATE INDEX ix_rel_expiry ON release_candidate(expires_at);

-- ─── The write queue · schema.md §10 ─────────────────────────────────────────

CREATE TABLE write_queue (
  id              INTEGER PRIMARY KEY,
  idempotency_key TEXT NOT NULL,          -- client ULID, or server-derived for northbound
  user_id         INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,          -- add|delete|monitor|unmonitor|grab|tag_add|refresh
  work_id             INTEGER,
                      -- FK to work(id) ON DELETE CASCADE dropped: `work` lands with library sync.
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  payload         TEXT NOT NULL,          -- JSON
  state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN (
                    'pending','inflight','verifying','done','failed')),
  -- `x IN (NULL, ...)` yields NULL, not FALSE, when x matches nothing, and a
  -- CHECK passes on NULL. Putting NULL in the list therefore made this
  -- constraint accept EVERY value. NULL has to be tested separately.
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
CREATE UNIQUE INDEX ux_wq_idem ON write_queue(user_id, idempotency_key);
CREATE INDEX ix_wq_runnable ON write_queue(state, next_attempt_at)
  WHERE state IN ('pending','inflight','verifying');
CREATE INDEX ix_wq_work ON write_queue(work_id, state);

-- ─── Tags · schema.md §8 ─────────────────────────────────────────────────────

CREATE TABLE tag (
  id          INTEGER PRIMARY KEY,
  namespace   TEXT NOT NULL DEFAULT 'tag',
  value       TEXT NOT NULL,
  is_system   INTEGER NOT NULL DEFAULT 0,
  cardinality TEXT NOT NULL DEFAULT 'multi' CHECK (cardinality IN ('single','multi')),
  inheritable INTEGER NOT NULL DEFAULT 0,
  color       TEXT,
  item_count  INTEGER NOT NULL DEFAULT 0,
  UNIQUE (namespace, value)
) STRICT;
CREATE INDEX ix_tag_ns ON tag(namespace, value);

CREATE TABLE tag_assignment (
  id                  INTEGER PRIMARY KEY,
  tag_id              INTEGER NOT NULL REFERENCES tag(id) ON DELETE CASCADE,
  work_id             INTEGER,
                      -- FK to work(id) ON DELETE CASCADE dropped: `work` lands with library sync.
  edition_id          INTEGER,
                      -- FK to edition(id) ON DELETE CASCADE dropped: same migration.
  media_file_id       INTEGER,
                      -- FK to media_file(id) ON DELETE CASCADE dropped: same migration.
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  user_id             INTEGER NOT NULL DEFAULT 0
                        REFERENCES user(id) ON DELETE CASCADE,
                        -- 0 = the shared/system sentinel row. NOT NULL because NULL is not
                        -- usable as an indexed equality (see the predicate below).
  source   TEXT NOT NULL CHECK (source IN ('system','rule','user','imported')),
  rule_id  INTEGER,
           -- FK to tag_rule(id) ON DELETE SET NULL dropped: tag_rule is a v1.0 table.
  added_at TEXT NOT NULL DEFAULT (datetime('now')),
  CHECK ((work_id IS NOT NULL) + (edition_id IS NOT NULL)
       + (media_file_id IS NOT NULL) + (service_instance_id IS NOT NULL) = 1)
) STRICT;

-- Uniqueness for ALL FOUR targets, not just work_id.
CREATE UNIQUE INDEX ux_ta_work ON tag_assignment(work_id, tag_id, user_id)
  WHERE work_id IS NOT NULL;
CREATE UNIQUE INDEX ux_ta_edition ON tag_assignment(edition_id, tag_id, user_id)
  WHERE edition_id IS NOT NULL;
CREATE UNIQUE INDEX ux_ta_file ON tag_assignment(media_file_id, tag_id, user_id)
  WHERE media_file_id IS NOT NULL;
CREATE UNIQUE INDEX ux_ta_inst ON tag_assignment(service_instance_id, tag_id, user_id)
  WHERE service_instance_id IS NOT NULL;

-- "all items with tag X", user-scoped. The user_id column is IN the index because the
-- predicate always carries it (ARCHITECTURE §1.3), and without it the covering property is lost.
CREATE INDEX ix_ta_tag  ON tag_assignment(tag_id, user_id, work_id);
CREATE INDEX ix_ta_work ON tag_assignment(work_id, tag_id);
CREATE INDEX ix_ta_edition_lookup ON tag_assignment(edition_id, tag_id) WHERE edition_id IS NOT NULL;
CREATE INDEX ix_ta_file_lookup    ON tag_assignment(media_file_id, tag_id) WHERE media_file_id IS NOT NULL;
CREATE INDEX ix_ta_inst_lookup    ON tag_assignment(service_instance_id, tag_id) WHERE service_instance_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- Downgrades are not a supported user path (docs/CONFIGURATION.md §6.3). This
-- block exists so a migration can be tested locally, and for nothing else.
-- +goose StatementBegin
DROP TABLE IF EXISTS tag_assignment;
DROP TABLE IF EXISTS tag;
DROP TABLE IF EXISTS write_queue;
DROP TABLE IF EXISTS release_candidate;
DROP TABLE IF EXISTS provenance;
DROP TABLE IF EXISTS service_instance;
DROP TRIGGER IF EXISTS trg_audit_no_delete;
DROP TRIGGER IF EXISTS trg_audit_no_update;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS client_credential;
DROP TABLE IF EXISTS session;
DROP TABLE IF EXISTS user;
-- +goose StatementEnd
