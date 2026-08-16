# Reference — Data model and DDL

**Status:** designed, not implemented. **Scope:** tables marked **v0.1** ship in the first
milestone; everything else is in the "later tables" appendix at the bottom and is **not** in
migration 0001.
**Parent document:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §6 carries the design and the
reasoning; this file carries the DDL and the invariant behind each index.

SQLite dialect, `STRICT` tables throughout. **Minimum SQLite version: 3.43.0** — `STRICT` needs
3.37, but FTS5 `contentless_delete=1` (§7 below) arrived in 3.43.0 and is mandatory.

**Timestamps are SQLite `datetime()` text — `YYYY-MM-DD HH:MM:SS`, UTC, no `T` and no `Z`.** This is
*not* ISO-8601, which this document previously claimed while every column default in it reads
`DEFAULT (datetime('now'))`. The SQLite format is the correct one and the code implements it
(`internal/store/store.go`, and `internal/db/sqlite.go` sets `_timefmt=sqlite` to match): the
timestamp columns are ordered lexicographically by `ix_audit_ts`, `ix_rel_expiry` and
`ix_wq_runnable`, and lexicographic ordering breaks the moment two formats share a column. One
format, chosen by the column defaults, applied everywhere.

Migrations are plain SQL run by `goose`, embedded via
`//go:embed migrations/*.sql`, and the migration files are ordered dependencies-first even though
SQLite resolves foreign keys at DML time.

Naming: the brief's `file` table is **`media_file`**; the brief's `release` table is split into
**`release_candidate`** (ephemeral, TTL-evicted) and **`provenance`** (immutable, one row per
acquisition event) — `RELEASE` is a SQLite keyword (`RELEASE SAVEPOINT`).

---

## 1. `work` — the abstract creative work · **v0.1**

```sql
CREATE TABLE work (
  id                INTEGER PRIMARY KEY,
  kind              TEXT NOT NULL CHECK (kind IN (
                      'movie','series','season','episode',
                      'artist','album','track',
                      'book','comic','game')),
                    -- NOTE: 'audiobook' is deliberately NOT a kind. An audiobook is an
                    -- edition of a 'book' work (edition.format='audiobook'). See §2.
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
  poster_asset_id   INTEGER REFERENCES image_asset(id),
  backdrop_asset_id INTEGER REFERENCES image_asset(id),
  -- Denormalised rollups. Recomputed per dirty-mark flush batch, NOT per child write.
  have_count        INTEGER NOT NULL DEFAULT 0,
  want_count        INTEGER NOT NULL DEFAULT 0,
  size_on_disk      INTEGER NOT NULL DEFAULT 0,
  monitored         INTEGER NOT NULL DEFAULT 0,
  rollup_dirty      INTEGER NOT NULL DEFAULT 0,   -- set by a child write; cleared by the flush
  availability      TEXT,   -- JSON {"1080p":{"have":250,"total":300}, ...}  see ARCHITECTURE §6.3
  added_at          TEXT,
  created_at        TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at        TEXT NOT NULL DEFAULT (datetime('now')),
  deleted_at        TEXT,               -- soft delete; 7-day tombstone
  content_hash      TEXT
) STRICT;

CREATE INDEX ix_work_kind_sort ON work(kind, sort_title, id) WHERE deleted_at IS NULL;
CREATE INDEX ix_work_parent    ON work(parent_work_id, kind);
CREATE INDEX ix_work_norm      ON work(normalized_title, year, kind);
CREATE INDEX ix_work_added     ON work(added_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_work_pop       ON work(popularity DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_work_dirty     ON work(rollup_dirty) WHERE rollup_dirty = 1;
```

**Why the partial predicates on `ix_work_added` / `ix_work_pop`:** without `WHERE deleted_at IS
NULL` the "recently added" and "popular" grids read soft-deleted rows from the index and filter them
afterwards, so the 7-day tombstone window degrades exactly those two views. `ix_work_kind_sort`
already had it; the other two were inconsistent.

**`ix_work_kind_sort` serves the keyset query; it does not cover it.** `EXPLAIN QUERY PLAN` on the
§13 grid query yields `SEARCH work USING INDEX ix_work_kind_sort (kind=? AND sort_title>?)` — a
search, because the SELECT list includes `title, year, poster_asset_id, have_count, want_count,
availability`. 100 row lookups per page is fine; the CI assertion must assert `SEARCH … USING
INDEX`, not `COVERING INDEX`.

### 1.1 Subtype tables · **v0.1 for movie/series/episode**

Rule: **every `kind` has a subtype table or an explicit justification for not having one.**
`season`, `artist` and `game` have none and need none today.

```sql
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

-- later tables (v1.0, with Lidarr / Komga)
CREATE TABLE work_album (
  work_id    INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  album_type TEXT, disambiguation TEXT, track_count INTEGER
) STRICT;

CREATE TABLE work_track (
  work_id       INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  disc_number   INTEGER NOT NULL DEFAULT 1,
  track_number  INTEGER NOT NULL,
  duration_secs INTEGER,
  isrc          TEXT
) STRICT;
-- A track's position must be unique within its parent album. The parent lives on
-- work.parent_work_id, so the constraint is expressed as a unique index over the join key
-- maintained by the importer (SQLite cannot express it declaratively across tables):
CREATE UNIQUE INDEX ux_track_pos ON work_track(work_id);
--   plus an application-enforced invariant, asserted in CI over a fixture:
--   UNIQUE(parent_work_id, disc_number, track_number) via
--   SELECT w.parent_work_id, t.disc_number, t.track_number FROM work_track t
--     JOIN work w ON w.id = t.work_id GROUP BY 1,2,3 HAVING COUNT(*) > 1  →  must be empty.

CREATE TABLE work_book (
  work_id     INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  page_count  INTEGER,
  series_name TEXT, series_position REAL
) STRICT;

CREATE TABLE work_comic (
  work_id      INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  issue_number REAL,
  volume       TEXT
) STRICT;

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
```

Without `work_track`'s disc and track numbers a multi-disc album is unrepresentable, `getAlbum`
cannot return tracks in order, and `work_album.track_count` has nothing to count against — which
blocks the whole OpenSubsonic surface.

---

## 2. `edition` — a specific released form · **v0.1 (table present, lightly used)**

```sql
CREATE TABLE edition (
  id           INTEGER PRIMARY KEY,
  work_id      INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  label        TEXT,      -- "Director's Cut" | "2013 Granta pb" | "Remastered"
  format       TEXT CHECK (format IN (
                 'print','ebook','audiobook','comic',
                 'bluray','web','remux','dvd',
                 'vinyl','cd','flac','lossy')),
  language     TEXT,
  quality_tier TEXT,      -- 2160p|1080p|720p|lossless|lossy
  is_primary   INTEGER NOT NULL DEFAULT 0,
  published_at TEXT,
  publisher    TEXT
) STRICT;
CREATE INDEX ix_edition_work ON edition(work_id, is_primary DESC);
```

**`edition.format` is where `audiobook` lives.** This is the single decision that makes
ebook-vs-audiobook routing a schema property rather than adapter special-casing, and it is why
`work.kind` has no `audiobook` member. Propagation: `request` carries `(kind, edition_format)`; the
tag vocabulary has a `format:` namespace rather than `type:audiobook`; `Caps.MediaKinds` is a list
of `(Kind, Format)` pairs.

---

## 3. `media_file` — concrete bytes · **v0.1**

```sql
CREATE TABLE media_file (
  id                  INTEGER PRIMARY KEY,
  work_id             INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  edition_id          INTEGER REFERENCES edition(id) ON DELETE SET NULL,
  service_instance_id INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE CASCADE,
                      -- instance 0 is a reserved sentinel row meaning
                      -- "discovered on the filesystem, not owned by any service".
  remote_file_id      TEXT,
  provenance_id       INTEGER REFERENCES provenance(id),
  path                TEXT NOT NULL,
  content_key         TEXT,   -- physical identity: hex(size_bytes) || ':' || sha256(first 64 KiB)
                              -- DEFERRED: populated from the first milestone that aggregates a
                              -- media server alongside a *Arr. Costs one 64 KiB read per file,
                              -- which is not free on a NAS, and v0.1 has no cross-instance
                              -- file dedup to perform.
  size_bytes          INTEGER NOT NULL DEFAULT 0,
  quality             TEXT, resolution TEXT,
  video_codec         TEXT, audio_codec TEXT, audio_channels REAL,
  languages           TEXT,   -- JSON array
  release_group       TEXT,
  date_added          TEXT,
  media_info          TEXT    -- JSON blob; NEVER selected in list views
) STRICT;
CREATE INDEX ix_file_work ON media_file(work_id);
CREATE UNIQUE INDEX ux_file_path ON media_file(service_instance_id, path);
CREATE INDEX ix_file_content ON media_file(content_key) WHERE content_key IS NOT NULL;
```

**`ux_file_path` is `(service_instance_id, path)`, never `path` alone.** `media_file` rows are
per-instance *observations*. The moment Radarr and Jellyfin both index
`/media/movies/Train Dreams (2025)/x.mkv` — the normal case, since a media server and a \*Arr point
at the same volume — a unique index on `path` alone rejects the second insert. So does Sonarr plus
Sonarr-Anime over a shared root.

**Two observations of one physical file are reconciled by `content_key`, once it exists.** Until
then, two instances' rows for the same bytes are two rows, and the `availability` rollup counts
per instance, which is the correct v0.1 behaviour anyway (the badge is *about* per-instance
presence). Container mount points differ per service, so `path` equality is not a substitute.

---

## 4. `external_id` — identity · **v0.1**

```sql
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

CREATE UNIQUE INDEX ux_extid ON external_id(
  source, value, COALESCE(work_id, -1), COALESCE(edition_id, -1));

CREATE UNIQUE INDEX ux_extid_work_strong ON external_id(source, value)
  WHERE work_id IS NOT NULL AND confidence >= 1.0;

CREATE INDEX ix_extid_work ON external_id(work_id, source);
-- `ix_extid_lookup(source, value)` is deliberately NOT created: ux_extid already serves
-- (source, value) equality as its leftmost prefix, and a second B-tree write on the hottest
-- insert path in the importer is a real cost. Re-add only with a measurement.
```

**The `ON CONFLICT` clause must repeat the index's expression list literally**, or SQLite raises
*"ON CONFLICT clause does not match any PRIMARY KEY or UNIQUE constraint"*:

```sql
INSERT INTO external_id (work_id, edition_id, source, value, confidence)
VALUES (:work_id, :edition_id, :source, :value, :confidence)
ON CONFLICT (source, value, COALESCE(work_id, -1), COALESCE(edition_id, -1))
DO UPDATE SET confidence = MAX(confidence, excluded.confidence);
```

**A `ux_extid_work_strong` violation is the merge signal, not an error.** Behaviour, normative:

1. The importer wraps each `external_id` insert in a `SAVEPOINT` so a conflict rolls back one row,
   not the whole 2,000-row batch.
2. On violation, look up the incumbent `work_id`, apply the §5 authority rule to decide which work
   survives, write a `work_merge` row, repoint `service_item_link` / `media_file` / `tag_assignment`
   / `request` at the survivor, then retry the insert.
3. Emit a `sync_report` row so the merge is visible.

**Manifest-sourced external ids are capped below 1.0** (`confidence < 1.0`) so a Tier 1 manifest can
never write a strong-identity row and can never trigger a merge.

---

## 5. Services and the M:N link · **v0.1**

```sql
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
CREATE UNIQUE INDEX ux_sil ON service_item_link(service_instance_id, remote_kind, remote_id);
CREATE INDEX ix_sil_work ON service_item_link(work_id) WHERE deleted_at IS NULL;

-- Alias rows keep old northbound IDs resolvable when a pinned instance is deleted.
CREATE TABLE service_item_alias (
  old_instance_id INTEGER NOT NULL,
  old_remote_kind TEXT NOT NULL,
  old_remote_id   TEXT NOT NULL,
  link_id         INTEGER NOT NULL REFERENCES service_item_link(id) ON DELETE CASCADE,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (old_instance_id, old_remote_kind, old_remote_id)
) STRICT, WITHOUT ROWID;
```

**`ux_sil` is only usable in full if `remote_kind` is known at lookup time**, which is why the
northbound ID encodes it (ARCHITECTURE §5.3). Without it, `WHERE service_instance_id=? AND
remote_id=?` yields `SEARCH service_item_link USING INDEX ux_sil (service_instance_id=?)` — a range
scan over every link on that instance, ~400k rows for a 2k-series Sonarr, on every stream resolve
and every `getCoverArt`.

**Authority rule when instances disagree on shared metadata:** highest `priority` among
`is_authoritative` links wins; otherwise most-recently-synced. Log divergences.

---

## 6. Provenance and release candidates · **v0.1**

```sql
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
  work_id             INTEGER REFERENCES work(id) ON DELETE CASCADE,   -- NULL in Search-and-Grab
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
  expires_at TEXT NOT NULL             -- ≤ 25 min for Prowlarr; see ARCHITECTURE §8.4
) STRICT;
CREATE INDEX ix_rel_expiry ON release_candidate(expires_at);
```

Three principles: **store `release_title` verbatim, forever** (every parsed field is re-derivable;
the raw name is not); **never overwrite provenance on upgrade** — insert a new row and link the new
`media_file`, which gives upgrade history for free; and **manual/filesystem imports get
`protocol='manual'`** — do not launder `unknown` into `torrent`.

---

## 7. Search · **v0.1**

```sql
CREATE VIRTUAL TABLE search_fts USING fts5(
  title, original_title, alt_titles, people, overview,
  content='', contentless_delete=1,               -- REQUIRES SQLite >= 3.43.0
  tokenize='unicode61 remove_diacritics 2',
  prefix='2 3 4'
);

CREATE VIRTUAL TABLE search_trgm USING fts5(
  title, alt_titles,
  content='', contentless_delete=1,
  tokenize='trigram'
);

CREATE TABLE search_doc (
  rowid        INTEGER PRIMARY KEY,     -- THE allocator for all three tables
  work_id      INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  kind         TEXT NOT NULL,           -- top-level kinds only; see the CI assertion below
  instance_scope TEXT NOT NULL DEFAULT '', -- JSON array of service_instance_ids that can see it;
                                        -- filtered IN THE JOIN, never post-filtered
  popularity   REAL NOT NULL DEFAULT 0,
  in_library   INTEGER NOT NULL DEFAULT 0,
  title_idf    REAL NOT NULL DEFAULT 0,
  norm_title   TEXT NOT NULL
) STRICT;
CREATE INDEX ix_sd_work ON search_doc(work_id);
```

**Three invariants, all CI-asserted, all silent-corruption sources if broken:**

1. `search_fts.rowid == search_trgm.rowid == search_doc.rowid`. The id is allocated by inserting
   into `search_doc` first, then inserted **explicitly** into both FTS tables in the same
   transaction. RRF fuses on `rowid`; a single implicit rowid fuses unrelated documents.
2. `count(search_fts) == count(search_trgm) == count(search_doc)`. Every `work` delete and every
   title change issues the matching FTS `DELETE` in the same transaction. Without
   `contentless_delete=1` this is impossible — a plain contentless table answers
   `cannot DELETE from contentless fts5 table`.
3. `SELECT COUNT(*) FROM search_doc WHERE kind IN ('season','episode','track')` is **0**.

---

## 8. Tags · **v0.1 (subset)**

```sql
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
  work_id             INTEGER REFERENCES work(id)             ON DELETE CASCADE,
  edition_id          INTEGER REFERENCES edition(id)          ON DELETE CASCADE,
  media_file_id       INTEGER REFERENCES media_file(id)       ON DELETE CASCADE,
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  user_id             INTEGER NOT NULL DEFAULT 0
                        REFERENCES user(id) ON DELETE CASCADE,
                        -- 0 = the shared/system sentinel row. NOT NULL because NULL is not
                        -- usable as an indexed equality (see the predicate below).
  source   TEXT NOT NULL CHECK (source IN ('system','rule','user','imported')),
  rule_id  INTEGER REFERENCES tag_rule(id) ON DELETE SET NULL,
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
```

The canonical filter predicate is `WHERE tag_id = ? AND user_id IN (0, :uid)`. Rule tags are
re-evaluated on ingest and on rule change, and the natural implementation is an insert — without
the three extra unique indexes, the same tag lands on the same edition/file/instance unboundedly.

`tag_alias`, `tag_implies`, `tag_rule` and `saved_filter` are **later tables** (v1.0); see the
appendix.

---

## 9. Users, sessions, credentials · **v0.1**

```sql
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
  actor_user_id INTEGER,                       -- HISTORICAL id, deliberately NO foreign key. See below.
  actor_ip TEXT, action TEXT NOT NULL,
  target_type TEXT, target_id TEXT,
  result TEXT NOT NULL, metadata_json TEXT,   -- secret VALUES never appear here; see security.md §5
  prev_hash TEXT                              -- rolling chain: sha256(prev_hash || row) — makes
                                              -- tampering EVIDENT, not impossible
) STRICT;
CREATE INDEX ix_audit_ts ON audit_log(ts DESC);

CREATE TRIGGER trg_audit_no_update BEFORE UPDATE ON audit_log
  BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;
CREATE TRIGGER trg_audit_no_delete BEFORE DELETE ON audit_log
  BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;
```

`ux_cc_prefix` makes the northbound auth path a single indexed lookup followed by one HMAC and one
constant-time compare — the cheap pre-check that keeps `GET /rest/ping?apiKey=garbage` from costing
anything.

**`audit_log.actor_user_id` carries no foreign key, and that is deliberate.** It is a *historical*
id: the user it names may since have been deleted, and the point of the log is that it still records
who acted. This column previously read `REFERENCES user(id) ON DELETE SET NULL`, which made deleting
any user who had ever acted **impossible** — `ON DELETE SET NULL` performs an implicit `UPDATE` on
`audit_log`, `trg_audit_no_update` aborts it, and the entire `DELETE FROM user` fails. Since logging
in writes an audit row, that is every user.

`ON DELETE NO ACTION` does **not** fix it. Verified by execution: it leaves the constraint violated
rather than firing the trigger, so the delete still fails — only the error text changes, from
`audit_log is append-only` to `FOREIGN KEY constraint failed`. Dropping the reference is the only
option that both permits the delete and preserves the actor. `SET NULL` would in any case have
destroyed exactly the record the log exists for (security.md §6, "who deleted this").

---

## 10. The write queue · **v0.1**

```sql
CREATE TABLE write_queue (
  id              INTEGER PRIMARY KEY,
  idempotency_key TEXT NOT NULL,          -- client ULID, or server-derived for northbound
  user_id         INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,          -- add|delete|monitor|unmonitor|grab|tag_add|refresh
  work_id             INTEGER REFERENCES work(id) ON DELETE CASCADE,
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  payload         TEXT NOT NULL,          -- JSON
  state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN (
                    'pending','inflight','verifying','done','failed')),
  fail_reason     TEXT CHECK (fail_reason IS NULL OR fail_reason IN (
                    'rejected','unknown','exhausted')),   -- NOT `IN (NULL,...)`: see below
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
```

`ux_wq_idem` is `(user_id, idempotency_key)`, **not** a bare `UNIQUE` on the key. A globally unique
client-supplied key means a weak ULID source, a key reused across accounts, or a replay returns
another user's `payload` and state. A key that exists under a different `user_id` gets `409`.

`ix_wq_runnable` is also the reconciliation guard's index: the sweep skips any `work_id` with a row
in `pending`, `inflight` **or** `verifying`.

**`fail_reason`'s `CHECK` must test `NULL` separately.** It previously read
`CHECK (fail_reason IN (NULL,'rejected','unknown','exhausted'))`, which enforced **nothing at all**:
in SQL, `x IN (NULL, 'a')` evaluates to `NULL` — not `FALSE` — when `x` matches no list entry, and a
`CHECK` constraint *passes* when its expression is `NULL`. One `NULL` in the list poisons the whole
comparison, so every value was accepted, including `'TOTAL-GARBAGE'`. The adjacent `state` column is
the control: the identical pattern without `NULL` in the list rejects correctly. Any nullable column
constrained this way needs the `IS NULL OR …` form.

---

## 11. Cross-media edges · **v0.3**

```sql
CREATE TABLE work_relation (
  from_work_id INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  to_work_id   INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  rel_type     TEXT NOT NULL CHECK (rel_type IN (
                 'based_on','derivative_of','adaptation_of','remake_of',
                 'sequel_to','prequel_to','soundtrack_of','novelization_of',
                 'same_franchise','translation_of','edition_of','same_universe','spinoff_of')),
  source       TEXT NOT NULL,        -- wikidata|tmdb|openlibrary|musicbrainz|comicinfo|manual
  confidence   REAL NOT NULL DEFAULT 1.0,
  evidence     TEXT NOT NULL,        -- JSON [{"source":"wikidata","prop":"P144","qid":"Q126086662"}]
  created_at   TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (from_work_id, to_work_id, rel_type)
) STRICT, WITHOUT ROWID;
CREATE INDEX ix_relation_to ON work_relation(to_work_id, rel_type);
```

`status`, `reviewed_by` and `reviewed_at` are **not** in this table: with the review inbox cut,
nothing produces a verdict, and only edges at ≥0.85 from authoritative sources are stored. A user
deleting a link deletes the row.

> **v1.0 obligation.** Manually-created links (`source='manual'`) are instance-global today because
> v0.1 is single-user. When multi-user lands they must become user-scoped, or one user's manual link
> is visible to everyone. Either add `user_id` to the primary key for `source='manual'` rows or move
> manual links to their own table. Decide deliberately; do not inherit this by omission.

---

## 12. Images · **v0.1**

```sql
CREATE TABLE image_asset (
  id         INTEGER PRIMARY KEY,
  source_url TEXT NOT NULL UNIQUE,      -- CREDENTIAL-STRIPPED. See security.md §6.
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
```

`origin_class` and `origin_service_instance_id` are what let the fetcher select an SSRF policy from
the **row** rather than from the URL string, which is the fix for the derived-URL class
(security.md §2). An ingest assertion rejects writing a `source_url` containing `api_key`, `apikey`,
`token` or `key=`.

**`cache.db`** is a second, disposable database holding `http_cache(url_hash, etag, last_modified,
body_hash, fetched_at, expires_at)`, the job queue, unowned-search result caching, and image-cache
metadata. Deleting it costs a re-sync, not data.

---

## Appendix — later tables

Present in the design, **not in migration 0001**, added with the milestone named:

| Table | Milestone | Note |
|---|---|---|
| `request`, `request_quota` | v0.2 | `request` carries `(kind, edition_format)`, not a flat kind |
| `work_merge` | v1.0 | Lands with identity cascade tiers 2–5; nothing in v0.1 merges |
| `tag_alias`, `tag_implies`, `tag_rule`, `saved_filter` | v1.0 | The full tag system |
| `role`, `role_permission`, `user_role`, `user_permission`, `user_library_access` | v1.0 | RBAC tables. The `user_id` columns and the access-scope parameter land in 0001; these do not |
| `playback_state`, `play_history` | v1.0 | Northbound write-back |
| `playlist`, `playlist_item` | v1.0 | See the note below |
| `sync_report` | v0.1 | Emitted by the sweep; a plain append-only log |

**`playlist_item`, when it lands, references a link and not a work**, and does not use a dense
integer position:

```sql
CREATE TABLE playlist_item (
  id          INTEGER PRIMARY KEY,
  playlist_id INTEGER NOT NULL REFERENCES playlist(id) ON DELETE CASCADE,
  link_id     INTEGER NOT NULL REFERENCES service_item_link(id) ON DELETE CASCADE,
  work_id     INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,  -- denormalised convenience
  position    REAL NOT NULL          -- sparse; a move is a single-row update
) STRICT;
CREATE INDEX ix_pi_playlist ON playlist_item(playlist_id, position);
```

Three reasons, each a bug in the earlier `(playlist_id, position, work_id)` shape: northbound
identity is a `service_item_link` (ARCHITECTURE §5.3), so storing `work_id` forces a re-derivation
per item and reintroduces exactly the ID instability the pin exists to prevent; a dense integer
`position` in the primary key means any insert or move rewrites every subsequent row, and a naive
two-row swap violates the PK mid-statement because SQLite checks uniqueness per row, not deferred;
and `playlist.home_instance_id` must be **recomputed on every `playlist_item` write** as *the single
distinct instance among members, else NULL*, with the UI told when it flips to NULL — a cross-backend
playlist cannot be written back to any backend, and that limitation must be visible rather than
silent.
