-- index ix_alt_norm
CREATE INDEX ix_alt_norm ON work_alt_title(normalized);

-- index ix_alt_work
CREATE INDEX ix_alt_work ON work_alt_title(work_id);

-- index ix_audit_actor_action
CREATE INDEX ix_audit_actor_action ON audit_log(actor_user_id, action, ts DESC);

-- index ix_audit_ts
CREATE INDEX ix_audit_ts ON audit_log(ts DESC);

-- index ix_cc_user
CREATE INDEX ix_cc_user ON client_credential(user_id, revoked_at);

-- index ix_comic_issue_sort
CREATE INDEX ix_comic_issue_sort ON work_comic_issue(number_sort);

-- index ix_credit_creator
CREATE INDEX ix_credit_creator ON work_credit(creator_work_id, role);

-- index ix_edition_format
CREATE INDEX ix_edition_format ON edition(format, work_id);

-- index ix_edition_work
CREATE INDEX ix_edition_work ON edition(work_id, is_primary DESC);

-- index ix_ep_air
CREATE INDEX ix_ep_air ON work_episode(air_date_utc);

-- index ix_extid_work
CREATE INDEX ix_extid_work ON external_id(work_id, source);

-- index ix_file_content
CREATE INDEX ix_file_content ON media_file(content_key) WHERE content_key IS NOT NULL;

-- index ix_file_work
CREATE INDEX ix_file_work ON media_file(work_id);

-- index ix_img_cache_key
CREATE INDEX ix_img_cache_key ON image_asset(cache_key);

-- index ix_img_state
CREATE INDEX ix_img_state ON image_asset(state, expires_at);

-- index ix_indexer_catalog_instance
CREATE INDEX ix_indexer_catalog_instance ON indexer_catalog(service_instance_id, name);

-- index ix_libmem_work
CREATE INDEX ix_libmem_work ON library_member(work_id);

-- index ix_library_kind
CREATE INDEX ix_library_kind ON library(user_id, kind) WHERE enabled = 1;

-- index ix_libsrc_instance
CREATE INDEX ix_libsrc_instance ON library_source(service_instance_id);

-- index ix_ovr_hash
CREATE INDEX ix_ovr_hash ON library_override(target_identity_hash);

-- index ix_ovr_user
CREATE INDEX ix_ovr_user ON library_override(user_id, created_at DESC);

-- index ix_prov_dlid
CREATE INDEX ix_prov_dlid     ON provenance(download_id);

-- index ix_prov_indexer
CREATE INDEX ix_prov_indexer  ON provenance(indexer_name);

-- index ix_prov_protocol
CREATE INDEX ix_prov_protocol ON provenance(protocol);

-- index ix_prov_unconfirmed
CREATE INDEX ix_prov_unconfirmed ON provenance(user_id, grabbed_at DESC, id DESC)
  WHERE acquisition_state <> 'confirmed';

-- index ix_prov_user_grabbed
CREATE INDEX ix_prov_user_grabbed ON provenance(user_id, grabbed_at DESC, id DESC);

-- index ix_rel_expiry
CREATE INDEX ix_rel_expiry ON release_candidate(expires_at);

-- index ix_sd_work
CREATE INDEX ix_sd_work ON search_doc(work_id);

-- index ix_sdl_doc
CREATE INDEX ix_sdl_doc ON search_doc_library(doc_rowid);

-- index ix_session_user
CREATE INDEX ix_session_user ON session(user_id, revoked_at);

-- index ix_sil_container
CREATE INDEX ix_sil_container ON service_item_link(service_instance_id, remote_library_id)
  WHERE deleted_at IS NULL AND remote_library_id IS NOT NULL;

-- index ix_sil_work
CREATE INDEX ix_sil_work ON service_item_link(work_id) WHERE deleted_at IS NULL;

-- index ix_sync_report_container_latest
CREATE INDEX ix_sync_report_container_latest
  ON sync_report(service_instance_id, kind, remote_kind, remote_id, id);

-- index ix_sync_report_instance
CREATE INDEX ix_sync_report_instance ON sync_report(service_instance_id, created_at DESC);

-- index ix_ta_edition_lookup
CREATE INDEX ix_ta_edition_lookup ON tag_assignment(edition_id, tag_id) WHERE edition_id IS NOT NULL;

-- index ix_ta_file_lookup
CREATE INDEX ix_ta_file_lookup    ON tag_assignment(media_file_id, tag_id) WHERE media_file_id IS NOT NULL;

-- index ix_ta_inst_lookup
CREATE INDEX ix_ta_inst_lookup    ON tag_assignment(service_instance_id, tag_id) WHERE service_instance_id IS NOT NULL;

-- index ix_ta_tag
CREATE INDEX ix_ta_tag  ON tag_assignment(tag_id, user_id, work_id);

-- index ix_ta_work
CREATE INDEX ix_ta_work ON tag_assignment(work_id, tag_id);

-- index ix_tag_ns
CREATE INDEX ix_tag_ns ON tag(namespace, value);

-- index ix_work_added
CREATE INDEX ix_work_added     ON work(added_at DESC, id DESC) WHERE deleted_at IS NULL;

-- index ix_work_backdrop
CREATE INDEX ix_work_backdrop ON work(backdrop_asset_id) WHERE backdrop_asset_id IS NOT NULL;

-- index ix_work_dirty
CREATE INDEX ix_work_dirty     ON work(rollup_dirty) WHERE rollup_dirty = 1;

-- index ix_work_kind_sort
CREATE INDEX ix_work_kind_sort ON work(kind, sort_title, id) WHERE deleted_at IS NULL;

-- index ix_work_norm
CREATE INDEX ix_work_norm      ON work(normalized_title, year, kind);

-- index ix_work_parent
CREATE INDEX ix_work_parent    ON work(parent_work_id, kind);

-- index ix_work_pop
CREATE INDEX ix_work_pop       ON work(popularity DESC, id DESC) WHERE deleted_at IS NULL;

-- index ix_work_poster
CREATE INDEX ix_work_poster   ON work(poster_asset_id)   WHERE poster_asset_id   IS NOT NULL;

-- index ix_wq_runnable
CREATE INDEX ix_wq_runnable ON write_queue(state, next_attempt_at)
  WHERE state IN ('pending','inflight','verifying');

-- index ix_wq_work
CREATE INDEX ix_wq_work ON write_queue(work_id, state);

-- index ux_cc_prefix
CREATE UNIQUE INDEX ux_cc_prefix ON client_credential(key_prefix);

-- index ux_extid
CREATE UNIQUE INDEX ux_extid ON external_id(
  source, value, COALESCE(work_id, -1), COALESCE(edition_id, -1));

-- index ux_extid_work_strong
CREATE UNIQUE INDEX ux_extid_work_strong ON external_id(source, value)
  WHERE work_id IS NOT NULL AND confidence >= 1.0;

-- index ux_file_path
CREATE UNIQUE INDEX ux_file_path ON media_file(service_instance_id, path);

-- index ux_libmem_identity
CREATE UNIQUE INDEX ux_libmem_identity ON library_member(library_id, work_id, edition_id);

-- index ux_library_name
CREATE UNIQUE INDEX ux_library_name ON library(user_id, name);

-- index ux_library_slug
CREATE UNIQUE INDEX ux_library_slug ON library(user_id, slug);

-- index ux_libsrc_authority
CREATE UNIQUE INDEX ux_libsrc_authority ON library_source(library_id)
  WHERE is_metadata_authority = 1;

-- index ux_ovr_field
CREATE UNIQUE INDEX ux_ovr_field  ON library_override(work_id, field_name, user_id)
  WHERE verb = 'field';

-- index ux_ovr_member
CREATE UNIQUE INDEX ux_ovr_member ON library_override(library_id, work_id, verb, user_id)
  WHERE verb IN ('exclude','include');

-- index ux_ovr_relink
CREATE UNIQUE INDEX ux_ovr_relink ON library_override(link_id, user_id)
  WHERE verb = 'relink';

-- index ux_sil
CREATE UNIQUE INDEX ux_sil ON service_item_link(service_instance_id, remote_kind, remote_id);

-- index ux_ta_edition
CREATE UNIQUE INDEX ux_ta_edition ON tag_assignment(edition_id, tag_id, user_id)
  WHERE edition_id IS NOT NULL;

-- index ux_ta_file
CREATE UNIQUE INDEX ux_ta_file ON tag_assignment(media_file_id, tag_id, user_id)
  WHERE media_file_id IS NOT NULL;

-- index ux_ta_inst
CREATE UNIQUE INDEX ux_ta_inst ON tag_assignment(service_instance_id, tag_id, user_id)
  WHERE service_instance_id IS NOT NULL;

-- index ux_ta_work
CREATE UNIQUE INDEX ux_ta_work ON tag_assignment(work_id, tag_id, user_id)
  WHERE work_id IS NOT NULL;

-- index ux_wq_idem
CREATE UNIQUE INDEX ux_wq_idem ON write_queue(user_id, idempotency_key);

-- table audit_log
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

-- table client_credential
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

-- table edition
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

-- table external_id
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

-- table image_asset
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
, format TEXT) STRICT;

-- table indexer_catalog
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

-- table library
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

-- table library_member
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

-- table library_override
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

-- table library_source
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

-- table media_file
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

-- table provenance
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
, user_id INTEGER NOT NULL DEFAULT 0, size_bytes INTEGER, acquisition_state TEXT NOT NULL DEFAULT 'confirmed') STRICT;

-- table release_candidate
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
, user_id INTEGER NOT NULL DEFAULT 0
    REFERENCES user(id) ON DELETE CASCADE) STRICT;

-- table search_doc
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

-- table search_doc_library
CREATE TABLE search_doc_library (
  library_id INTEGER NOT NULL REFERENCES library(id)       ON DELETE CASCADE,
  doc_rowid  INTEGER NOT NULL REFERENCES search_doc(rowid) ON DELETE CASCADE,
  PRIMARY KEY (library_id, doc_rowid)
) STRICT, WITHOUT ROWID;

-- table search_fts
CREATE VIRTUAL TABLE search_fts USING fts5(
  title, original_title, alt_titles, people, overview,
  content='', contentless_delete=1,
  tokenize='unicode61 remove_diacritics 2',
  prefix='2 3 4'
);

-- table search_fts_config
CREATE TABLE 'search_fts_config'(k PRIMARY KEY, v) WITHOUT ROWID;

-- table search_fts_data
CREATE TABLE 'search_fts_data'(id INTEGER PRIMARY KEY, block BLOB);

-- table search_fts_docsize
CREATE TABLE 'search_fts_docsize'(id INTEGER PRIMARY KEY, sz BLOB, origin INTEGER);

-- table search_fts_idx
CREATE TABLE 'search_fts_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID;

-- table search_trgm
CREATE VIRTUAL TABLE search_trgm USING fts5(
  title, alt_titles,
  content='', contentless_delete=1,
  tokenize='trigram'
);

-- table search_trgm_config
CREATE TABLE 'search_trgm_config'(k PRIMARY KEY, v) WITHOUT ROWID;

-- table search_trgm_data
CREATE TABLE 'search_trgm_data'(id INTEGER PRIMARY KEY, block BLOB);

-- table search_trgm_docsize
CREATE TABLE 'search_trgm_docsize'(id INTEGER PRIMARY KEY, sz BLOB, origin INTEGER);

-- table search_trgm_idx
CREATE TABLE 'search_trgm_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID;

-- table service_instance
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
, indexers_fetched_at TEXT, arrivals_watermark TEXT) STRICT;

-- table service_item_alias
CREATE TABLE service_item_alias (
  old_instance_id INTEGER NOT NULL,
  old_remote_kind TEXT NOT NULL,
  old_remote_id   TEXT NOT NULL,
  link_id         INTEGER NOT NULL REFERENCES service_item_link(id) ON DELETE CASCADE,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (old_instance_id, old_remote_kind, old_remote_id)
) STRICT, WITHOUT ROWID;

-- table service_item_link
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

-- table session
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

-- table sync_report
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

-- table tag
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

-- table tag_assignment
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

-- table user
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

-- table work
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

-- table work_alt_title
CREATE TABLE work_alt_title (
  id         INTEGER PRIMARY KEY,
  work_id    INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  title      TEXT NOT NULL,
  normalized TEXT NOT NULL,
  kind       TEXT NOT NULL,   -- original|translated|alias|acronym|sort
  language   TEXT
) STRICT;

-- table work_book
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

-- table work_comic
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

-- table work_comic_issue
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

-- table work_credit
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

-- table work_episode
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

-- table work_movie
CREATE TABLE work_movie (
  work_id     INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  collection  TEXT, studio TEXT, certification TEXT,
  in_cinemas  TEXT, physical_release TEXT, digital_release TEXT
) STRICT;

-- table work_series
CREATE TABLE work_series (
  work_id      INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  series_type  TEXT,                    -- standard|daily|anime
  network      TEXT, air_time TEXT,
  season_count INTEGER,
  ended        INTEGER NOT NULL DEFAULT 0
) STRICT;

-- table write_queue
CREATE TABLE "write_queue" (
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

-- trigger trg_audit_no_delete
CREATE TRIGGER trg_audit_no_delete BEFORE DELETE ON audit_log
  BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;

-- trigger trg_audit_no_update
CREATE TRIGGER trg_audit_no_update BEFORE UPDATE ON audit_log
  BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;

-- trigger trg_library_unfiled_no_delete
CREATE TRIGGER trg_library_unfiled_no_delete BEFORE DELETE ON library
  WHEN OLD.id = 0
  BEGIN
    SELECT RAISE(ABORT, 'library 0 ("Unfiled") is reserved and cannot be deleted: it is where the membership derivation files a work that belongs to no other library, and without it such a work is invisible in search to every user including its owner. This also blocks DELETE FROM user WHERE id = 0, the shared/system sentinel, deliberately.');
  END;

