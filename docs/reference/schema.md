# Reference — Data model and DDL

**Status:** partly implemented. This document is authoritative for the *shape* of the schema, not
for how much of that shape has been created — **`internal/db/migrations` answers that**, and
reading the files there is the only form of the answer that does not go stale. A table given in
full below may still be design-only.

⚠️ **Where this file states which migration creates a thing, it has been wrong before.** §13 said
"All four tables are in migration 0001" when none of them existed anywhere. Those claims are
removed rather than refreshed: the per-section **v0.x** markers below say which *milestone* owns a
table, which is a different question, and `internal/db/migrations` is the only answer to the other
one. `internal/db/migrate_test.go`'s `TestDeferredTablesAreAbsent` carries the exhaustive
present/absent lists in executable form.
**Scope:** tables marked **v0.1** ship in the first milestone; everything else is in the "later
tables" appendix at the bottom.
**Parent document:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §6 carries the design and the
reasoning; this file carries the DDL and the invariant behind each index.

SQLite dialect, `STRICT` tables throughout. **Minimum SQLite version: 3.43.0** — `STRICT` needs
3.37, but FTS5 `contentless_delete=1` (§7 below) arrived in 3.43.0 and is mandatory.

⚠️ **2026-08-21 — this number is now a measurement, and it was not one before.** The floor is a claim
about **readers**: UsArr bundles its own engine and never consults it. From migration 0005 until
migration `00013`, `trg_library_unfiled_no_delete` raised a `||`-concatenated message — legal as
`RAISE()`'s second argument only from **3.47.0** — and because SQLite stores schema objects as text,
every reader below 3.47.0 failed at PREPARE on **every** statement with `malformed database schema`.
`00013` rewrote that message as a static literal and a real 3.43.0 build now answers
`PRAGMA integrity_check` with `ok` over the whole schema, contentless FTS5 and all.
**`RAISE()`'s message must be a static literal in anything that reaches the persisted schema** —
[ADR-0075](../DECISIONS.md#adr-0075), enforced by `TestPersistedSchemaRaisesOnlyLiterals`.

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
                      'book','comic','comic_issue','person','game')),
                    -- NOTE: 'audiobook' is deliberately NOT a kind. An audiobook is an
                    -- edition of a 'book' work (edition.format='audiobook'). See §2.
                    -- NOTE: 'comic' is the SERIES; 'comic_issue' is the issue or chapter,
                    -- and it is excluded from the search corpus exactly as episode/track are
                    -- (ADR-0030). There is no 'manga' kind and no Volume level; manga is
                    -- work_comic.reading_direction plus a derived type:manga tag, and Kavita's
                    -- Volume is work_comic_issue.volume_label + volume_sort.
                    -- NOTE: 'person' is a credited human — author, translator, editor,
                    -- illustrator, writer, penciller, inker, colorist, letterer, cover
                    -- artist, narrator (ADR-0033). It is the target of
                    -- work_credit.creator_work_id for books and comics. It is NOT top-level:
                    -- excluded from the media-type navigation enum (ARCHITECTURE §17.2),
                    -- from the Tier 1 client prefix index (§4.5) and from the FTS corpus
                    -- (§8.2), because there is no person screen in any milestone.
                    -- 'artist' still means a MUSIC artist and still maps to the Music type;
                    -- a credit points at 'artist' only when a connected service models it as
                    -- a top-level catalogue entity of its own.
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
  poster_asset_id   INTEGER REFERENCES image_asset(id) ON DELETE SET NULL,
  backdrop_asset_id INTEGER REFERENCES image_asset(id) ON DELETE SET NULL,
  -- Denormalised rollups. Recomputed per dirty-mark flush batch, NOT per child write.
  have_count        INTEGER NOT NULL DEFAULT 0,
  want_count        INTEGER NOT NULL DEFAULT 0,
  size_on_disk      INTEGER NOT NULL DEFAULT 0,
  monitored         INTEGER NOT NULL DEFAULT 0,
  rollup_dirty      INTEGER NOT NULL DEFAULT 0,   -- set by a child write; cleared by the flush
  availability      TEXT,   -- JSON, POLYMORPHIC by medium — see "The availability blob" below
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

**`poster_asset_id` and `backdrop_asset_id` carry `ON DELETE SET NULL`, and the clause is what
gives `ix_img_state(state, expires_at)` a reader at all.** That index has no plausible use but the
image expiry sweep, and under the DEFAULTED `ON DELETE NO ACTION` — which is what this file
specified until 2026-08-17 — the sweep's `DELETE FROM image_asset WHERE state = ? AND expires_at <=
?` fails with `FOREIGN KEY constraint failed` for every asset any `work` row points at, i.e. for
every poster on the Home screen. So the index served a query that could never succeed. Executed
both ways; `TestImageAssetExpirySweepEvicts` pins it, including that the *work* survives the
eviction (SET NULL, never CASCADE — a cover expiring must not delete the movie).

⚠️ **`media_file.provenance_id` (§3) is the third defaulted foreign key and it is deliberately
LEFT that way**, judged separately rather than swept up with these two. Nothing deletes a
`provenance` row — the table is immutable by §6's own rule, has no expiry column, no index that
would serve a sweep, and no parent whose cascade could reach it — so `NO ACTION` blocks no sweep
there. What it does block is an accidental `DELETE FROM provenance` while a file still cites it,
which for an acquisition-history table is the outcome to want.

**Why the partial predicates on `ix_work_added` / `ix_work_pop`:** without `WHERE deleted_at IS
NULL` the "recently added" and "popular" grids read soft-deleted rows from the index and filter them
afterwards, so the 7-day tombstone window degrades exactly those two views. `ix_work_kind_sort`
already had it; the other two were inconsistent.

### The availability blob, per medium

ADR-0031 decision #5 (*"the availability rollup is edition-keyed for music"*) and ARCHITECTURE §6.1
(*"the rollup gains a `total_source` field"*) are both normative and neither had a representation
here, so an implementer coding from this file would have written the tier shape for all six types.
**The blob carries a `"k"` discriminator as its first key and a renderer switches on it.** Without
one, a renderer cannot tell a tier key from an edition key in the same object.

```jsonc
// k=tier — video. total is a property of the parent work. (ARCHITECTURE §6.3)
{"k":"tier","1080p":{"have":250,"total":300},"2160p":{"have":40,"total":300}}

// k=edition — music. total is a property of the EDITION: choosing the 2017 remaster over the
// 2000 original changes the track list, the count and the durations, so a bare fraction is a
// guess. `label` is what the renderer puts beside the fraction. (ADR-0031)
{"k":"edition",
 "edition:mbz_release:def-456":{"have":0,"total":22,"label":"2017 remaster (2CD)"},
 "edition:mbz_release:abc-123":{"have":12,"total":12,"label":"2000 original"}}

// k=count — comics, and anything else with no honest denominator. `total` is present ONLY when
// the series status is ENDED/Completed/Cancelled AND a total is declared; total_source names
// where the declaration came from, because every "total" in the domain is a declaration
// (ComicInfo `Count`, whose own spec concedes it "could be different on each book in a series").
// `missing` is the number that is always honest — contiguity, computed locally from
// work_comic_issue.number_sort with no upstream help. (ARCHITECTURE §6.1)
{"k":"count","have":43,"total":60,"total_source":"komga:totalBookCount",
 "missing":["7","12","30-32"]}
{"k":"count","have":43,"total":null,"total_source":null,"missing":["7","12"]}
```

Three rules the shape imposes. **`k` is required on every non-null blob**, so a v0.1 writer that
only ever emits `k:"tier"` still produces forward-compatible rows. **`total: null` is not
`total: 0`** — the first means "nobody honestly knows", the second means "the series is empty", and
§6.3's render rule (`have == total && total > 0` → ✓) must never fire on the first. And **the
250 ms dirty-flush batch (§6.3) is unchanged by any of this** — the blob is opaque to the flush,
which recomputes and rewrites it whole.

**`ix_work_kind_sort` serves the keyset query; it does not cover it.** `EXPLAIN QUERY PLAN` on the
§13 grid query yields `SEARCH work USING INDEX ix_work_kind_sort (kind=? AND sort_title>?)` — a
search, because the SELECT list includes `title, year, poster_asset_id, have_count, want_count,
availability`. 100 row lookups per page is fine; the query-plan assertion in
`internal/db/queryplan_test.go`, which `make check` runs, must pin `SEARCH … USING INDEX`, not
`COVERING INDEX`.

### 1.1 Subtype tables · **v0.1 for movie/series/episode and for book/comic/comic_issue**

⚠️ **This heading read *"v0.1 for movie/series/episode"* and the comment above `work_album` below
scheduled all six remaining subtype tables as *"later tables"*. Both were falsified by
[ADR-0041](../DECISIONS.md#adr-0041)**, which made **Kavita** v0.1's first catalogue adapter:
`work_book`, `work_comic` and `work_comic_issue` are the tables Kavita writes, so they ship in v0.1
and exist in the tree. [ADR-0040](../DECISIONS.md#adr-0040) is confirmed rather than reopened — each
subtype table still lands with the source that writes it, and only the *date* of three of them moved.

⚠️ **A FOURTH FOLLOWED, on the same rule ([ADR-0044](../DECISIONS.md#adr-0044), owner-approved
2026-08-17): `work_credit` also lands with Kavita**, in `00007_work_credit.sql`. This paragraph read
*"`work_album`, `work_track` and `work_credit` still wait for Navidrome"* and that is now false for
the third of the three. Kavita's `SeriesMetadataDto` reports writers, cover artists, pencillers,
inkers, colorists, letterers, editors and translators — eight roles, every one already a member of
`work_credit.role`'s `CHECK` below — so ADR-0040's rule points at Kavita for this table. The cost is
stated where it belongs, under `creator_work_id`: a credit points at a `work` of kind `person`, so the
Kavita adapter now creates person rows. **`work_album` and `work_track` still wait for Navidrome**,
which has no adapter; `internal/db/migrations` is the answer to what exists.

Rule: **every `kind` has a subtype table or an explicit justification for not having one.**
`season`, `artist` and `game` have none and need none today. **`person` (ADR-0033) has none by
design**: a credited human is a name, an optional set of `external_id` rows (OLIDs, Comic Vine
person ids) and the credits that point at it, all of which `work`, `external_id` and `work_credit`
already carry. A `work_person` table would hold birth year and a biography, which no v0.1 source
reports for authors, and inventing it now is a column nothing writes.

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

-- THE MUSIC THREE, and they are the only subtype tables still waiting. They land
-- with NAVIDROME (and their command sink, Lidarr, is in no milestone before
-- that), first in the post-v0.1 sequence ADR-0036 established and ADR-0041
-- re-numbered. The books-and-comics three further down are NOT here with them
-- any more: ADR-0041 made Kavita v0.1's first adapter, and per ADR-0040 a
-- subtype table lands with the source that writes it.
CREATE TABLE work_album (
  work_id    INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  album_type TEXT, disambiguation TEXT, track_count INTEGER
) STRICT;

-- ADR-0031: position is a property of the (track-work, edition) pair, NOT of the track work.
-- The same recording is track 4 on the original CD and track 6 on the 2017 reissue, with a
-- different track MBID each. edition_id costs 8 bytes a row on the day work_track is created;
-- adding it later is a backfill over the largest table in the schema.
CREATE TABLE work_track (
  work_id        INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  edition_id     INTEGER NOT NULL REFERENCES edition(id) ON DELETE RESTRICT,
                                     -- RESTRICT, not CASCADE: deleting an edition must not
                                     -- silently destroy its track rows. An adapter that
                                     -- re-synthesises the primary edition on every sync would
                                     -- otherwise delete the album's whole track set. See the
                                     -- mandatory-edition rule below.
  disc_number    INTEGER NOT NULL DEFAULT 1,
  disc_title     TEXT,
  track_number   TEXT NOT NULL,      -- TEXT: real values are 'A1', 'B2', '1.01' (Lidarr ships
                                     -- a string plus a separate int; an INTEGER column sorts a
                                     -- double LP randomly)
  track_position INTEGER NOT NULL,   -- derived sort key for track_number
  title          TEXT,               -- edition-local display text; two releases carry one
                                     -- recording as 'Idioteque' and 'Idioteque (Album Version)'
  duration_secs  INTEGER,
  isrc           TEXT,
  PRIMARY KEY (work_id, edition_id)
) STRICT;
-- A track's position must be unique within one edition. That IS the constraint under ADR-0031,
-- and it is declarable:
CREATE UNIQUE INDEX ux_track_pos ON work_track(edition_id, disc_number, track_position);
```

> **`work_track.edition_id NOT NULL` makes an `edition` row mandatory for every album, and that
> requirement is stated here rather than left to be discovered.** Three rules, all normative, all
> consequences of the NOT NULL:
>
> 1. **Every album work has exactly one synthetic primary `edition`**, created
>    in the same transaction as the album, `is_primary = 1`, `label = NULL`. Lidarr and Navidrome
>    report no release concept, so the adapter synthesises it; v0.x models only the active edition,
>    which is what Lidarr does (ADR-0031).
> 2. **Its identity is derived, not allocated per sync.** The adapter resolves the primary edition
>    by `SELECT id FROM edition WHERE work_id = ? AND is_primary = 1` and inserts only when that
>    returns nothing. Allocating a fresh edition per sync would duplicate the whole track set on
>    every re-import, and with `ON DELETE CASCADE` (the earlier shape) it would have destroyed the
>    previous one instead — which is why the cascade is now `RESTRICT`.
> 3. **`work_track.edition_id` must belong to the track's parent album.** SQLite cannot express it
>    (the parent is `work.parent_work_id`, one join away), so it is an application-enforced
>    invariant owed an assertion over a fixture — it is not written yet, because `work_track` does
>    not exist (`internal/db/queryplan_test.go` records why). It lands in `make check` with the
>    migration that creates the table:
>
> ```sql
> -- must return no rows
> SELECT t.work_id, t.edition_id FROM work_track t
>   JOIN work w ON w.id = t.work_id
>   JOIN edition e ON e.id = t.edition_id
>  WHERE e.work_id IS NOT w.parent_work_id;
> ```
>
> The pre-ADR-0031 note here asserted `UNIQUE(parent_work_id, disc_number, track_number)` as a
> second invariant. **That is withdrawn**: under edition scoping it is either redundant with
> `ux_track_pos` or actively wrong, because it forbids the same recording sitting at the same
> position on two editions of one album — the exact case ADR-0031 exists for.

```sql
-- ADR-0031 decision #3: attribution is M:N. There is no artist_id column on an album.
-- The moment attribution is a scalar, Various-Artists compilations, collaborations and
-- classical roles are unrepresentable — which is Lidarr's own limitation
-- (AlbumResource.artistId is singular) and there is no reason to inherit it.
CREATE TABLE work_credit (
  work_id         INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  creator_work_id INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
                                     -- a work of kind 'artist' OR 'person' (ADR-0033).
                                     -- 'artist' when a connected service models the creator as a
                                     -- top-level catalogue entity in its own right (Navidrome and
                                     -- Lidarr artists); 'person' otherwise (authors, translators,
                                     -- editors, illustrators, comic writers and artists,
                                     -- narrators). Renamed from artist_work_id, which asserted
                                     -- the wrong kind for four of the six media types.
  role           TEXT NOT NULL CHECK (role IN (
                   -- music
                   'primary','featured','composer','conductor','performer','remixer','producer',
                   -- books
                   'author','translator','editor','illustrator','narrator',
                   -- comics
                   'writer','penciller','inker','colorist','letterer','cover_artist')),
  position       INTEGER NOT NULL DEFAULT 0,   -- billing order within (work, role)
  credited_as    TEXT,               -- the string this release printed, when it differs from
                                     -- the creator work's title ("Sting" on a Police reissue)
  PRIMARY KEY (work_id, role, position, creator_work_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX ix_credit_creator ON work_credit(creator_work_id, role);
```

**The PK leads with `(work_id, role, position)`** because the only hot read is *"give me this
album's credits in billing order"*, which is then a single covered range scan. `creator_work_id`
trails it to make the row unique when two creators share a role and a position (a co-credit).
`ix_credit_creator` serves the reverse — a creator page listing everything they are credited on.

**`creator_work_id` points at a `work` of kind `artist` or of kind `person`, and `person` is a
member of `work.kind`'s CHECK from the migration that creates `work` (ADR-0033).** The previous shape had no `person` member at all, so a book's author and
a comic's penciller were stored as `artist`-kind works — which put every author into the **Music**
navigation type (ARCHITECTURE §17.2 maps `artist` → Music) and counted every credit against the Tier 1
prefix index's 25,000-item cap (§4.5), which §13 already shows is tripped by the reference library.
Both consequences are gone, at the cost of one CHECK member, one `kind_byte` and this column's name.

**Which kind a given credit points at is a rule, not a choice made per row:** `artist` when a
connected service models the creator as a **top-level catalogue entity in its own right** — a
Navidrome or Lidarr artist, which has albums, a page and a library row — and `person` otherwise. 🔍
That rule is inference from how the sources model their own data, not a citation. **A human who is
both a music artist and a book author is two rows in v0.1**, joined by nothing; that is a stated
loss, and it is smaller than filing every novelist under Music.

**`person` is excluded from the media-type navigation enum, the Tier 1 prefix index and the FTS
corpus**, because there is no person screen in any milestone and a person hit would be a search result
with nowhere to land. It is reachable as a credit link on an item (ARCHITECTURE §17.6). **The
exclusion still holds and is still enforced** — `TestPeopleNeverEnterTheSearchCorpus` asserts it over
`search_doc`, `search_fts`/`search_trgm` and `search_doc_library`, and
`internal/store/catalogue.go`'s `writeSearchDoc` now also **refuses** any of search.md §2's five
excluded kinds at the writer rather than only reporting them afterwards (nothing shipped reaches that
branch: Kavita's adapter maps containers to `comic` and `book` only).

**What no longer holds is the consequence.** This paragraph used to carry ⚠️ *"find everything by this
author" is unanswered in v0.1*, and the paragraph after it recorded that the document builder wrote
`search_fts.people` as the empty string. **Both are false as of `internal/store`'s `creditedNames`.**
The column is filled from `work_credit` — the person work's title UNIONed with `credited_as` where
they differ, newline-joined, **every role in the vocabulary and not an authorship-ish subset** — so a
search for a creator returns the *works* they are credited on. A person hit is still impossible; a
book found by its author is not.

**It fills `people`, not the `alt_titles` candidate this section used to name**, and the difference is
not cosmetic: a name in `alt_titles` is indistinguishable from a title at query time and could never
be weighted, filtered or explained apart from one, whereas `people` is its own fts5 column and
search.md §4's fusion query can take a per-column bm25 weight for it when retrieval lands.

**Two writers keep it current, and the second one is conditional.** The item pass reads `work_credit`
rather than writing `""`, so a re-import does not blank the column in the window before phase B runs.
The credit pass then compares the *rendered name list* before and after its wholesale credit replace
and rebuilds the document only when a name actually entered or left — which is the answer to the cost
this section weighed (*"a second FTS write per item, in the transaction the 100 ms batch window exists
to keep short"*): it is a second write per item **whose credits changed**, and a steady-state
re-import rebuilds nothing. `CreditResult.DocsRebuilt` reports the count, and it has to, because
`search_fts` is contentless — its rows are replaced in place and no query outside the store can tell a
rebuild from no rebuild.

⚠️ **Nothing queries either FTS table yet**, so this is an indexing change with no reader. The
retrieval path is a later commit, and it is what will decide the per-column weights. The guards are
`internal/store/searchdoc_people_test.go`; REVIEW-LOG `LS-100` carries the argument and the
alternatives refused.

**Doing this in the migration that creates `work` costs one CHECK member and one byte allocation. Doing it later costs a
CHECK-constraint change (a SQLite table rebuild), an FTS re-index, a rebuild of every client prefix
index, and a change to the `kind_byte` codec that ARCHITECTURE §5.3 states is unchangeable once
clients cache ids** — which is ADR-0030's argument, in a second place, for the same reason.

**`work_credit` above and the three below all ship in v0.1** — Kavita writes all four
([ADR-0041](../DECISIONS.md#adr-0041) for the three, [ADR-0044](../DECISIONS.md#adr-0044) for
`work_credit`), and `internal/db/migrations` is the answer to whether they exist yet.

```sql

CREATE TABLE work_book (
  work_id     INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  page_count  INTEGER,
  series_name TEXT, series_position REAL
) STRICT;

-- ADR-0030: the subtype splits with the kind. work_comic is SERIES level.
CREATE TABLE work_comic (
  work_id                INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  volume_label           TEXT,      -- 'Vol. 3' / '(2012)' — a label, never a node
  volume_year            INTEGER,
  reading_direction      TEXT,      -- ltr|rtl|vertical|webtoon; the manga axis, not a kind
  publisher              TEXT,
  total_issues_declared  INTEGER,   -- a DECLARATION, not a fact — see total_issues_source
  total_issues_source    TEXT       -- comicinfo|comicvine|kavitaplus|null
) STRICT;

-- ISSUE level. number_text is TEXT and number_sort is REAL because real issue numbers are
-- '1.MU', '-1', '0', 'Annual 1', '1A'. Komga models a string plus a float sort key; Kavita
-- models min/max floats plus a string plus a range. Any INTEGER column here is wrong.
CREATE TABLE work_comic_issue (
  work_id         INTEGER PRIMARY KEY REFERENCES work(id) ON DELETE CASCADE,
  number_text     TEXT,
  number_sort     REAL,
  volume_label    TEXT,     -- Kavita's Volume, carried as an attribute (ADR-0030)
  volume_sort     REAL,
  is_special      INTEGER NOT NULL DEFAULT 0,
  is_oneshot      INTEGER NOT NULL DEFAULT 0,
  special_version TEXT,     -- tpb|hard-cover|omnibus|one-shot|volume-as-issue|cover
  page_count      INTEGER
) STRICT;
CREATE INDEX ix_comic_issue_sort ON work_comic_issue(number_sort);
--   ix_comic_issue_sort is what makes the contiguity report cheap: '43 issues · #7, #12 and
--   #30-32 missing' is computed locally with no upstream help, and is the only always-honest
--   completeness number in the domain (ARCHITECTURE §6.1).

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
                 'cbz','cbr','pdf',
                 'bluray','web','remux','dvd',
                 'vinyl','cd','cassette','digital','flac','lossy')),
                 -- format carries the MEDIUM, never the codec: a 2000 UK CD release can be on
                 -- disk as FLAC, so the codec lives on media_file (ADR-0031). The comic file
                 -- shapes are added because ARCHITECTURE §6.1's prose already listed cbz.
  language     TEXT,
  quality_tier TEXT,      -- 2160p|1080p|720p|lossless|lossy
  is_primary   INTEGER NOT NULL DEFAULT 0,
  published_at TEXT,
  publisher    TEXT,
  -- ADR-0031: audiobook production properties. Not work properties (different productions have
  -- different narrators) and not media_file properties (a 30-file audiobook has one runtime).
  narrators    TEXT,       -- JSON array
  duration_seconds INTEGER,
  abridged     INTEGER     -- nullable: unknown is distinct from 'unabridged'
) STRICT;
CREATE INDEX ix_edition_work ON edition(work_id, is_primary DESC);
-- Migration 0009. The reverse direction: "is this work an audiobook", asked as a FILTER once per
-- candidate row of an ordered walk (§17.2 rows 4 and 5, GET /api/v1/library). ix_edition_work seeks
-- on work_id and then fetches `format` from the table; this makes `format = ? AND work_id = ?` a
-- two-column COVERING seek. NOT partial: `edition` has no soft-delete column, and the NULL formats
-- are rows the Ebooks side of the split has to see. ADR-0051.
CREATE INDEX ix_edition_format ON edition(format, work_id);
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
                      -- instance 0 is a reserved sentinel row meaning "not reported by any
                      -- networked service instance". Its ONE current use is the Tier 0 Calibre
                      -- adapter (ARCHITECTURE §11.2, v1.0), which opens Calibre's own
                      -- metadata.db read-only — a single-file read that §11.2 records as an
                      -- explicit exception. It does NOT mean "found by scanning a filesystem":
                      -- ADR-0026 decides that UsArr never touches a filesystem and refuses
                      -- alternative (C) on those grounds, and nothing in any milestone scans.
                      -- (The earlier comment said "discovered on the filesystem", which
                      -- contradicted ADR-0026 outright.)
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
  indexers_fetched_at TEXT,           -- last successful indexer-list replication. Migration 0004.
                                      -- NULL means NEVER; see §5.1.
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
  -- The three container-predicate inputs that library_source.container_kind needs and that this
  -- table previously could not hold. Without them, `remote_library`, `tag` and `series_type` are
  -- CHECK values with no storage behind them — and `remote_library` is the ONLY container kind
  -- available for the catalogue sources (Navidrome, Audiobookshelf, Kavita, then Komga), i.e. for
  -- four of the six media types. Those arrive one at a time AFTER v0.1 (ADR-0036), so these
  -- columns are written by the *Arr sync first and carry no catalogue reader until then.
  remote_library_id   TEXT,          -- the upstream's own library/collection id, verbatim
  remote_tag_ids      TEXT,          -- JSON array of *Arr tag ids, verbatim
  remote_subtype      TEXT,          -- the upstream's own sub-classification, verbatim
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
-- The membership derivation (§13) walks links per instance per container. Without this it is a
-- full scan of every link on the instance for every library the instance feeds.
CREATE INDEX ix_sil_container ON service_item_link(service_instance_id, remote_library_id)
  WHERE deleted_at IS NULL AND remote_library_id IS NOT NULL;

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

⚠️ **The container-column comment above is superseded and is left standing deliberately.** It says
those columns *"are written by the \*Arr sync first and carry no catalogue reader until then"*,
which was true of the build order ADR-0036 set and is not true of the one that shipped:
[ADR-0041](../DECISIONS.md#adr-0041) put Kavita in v0.1, so `remote_library_id` and `remote_subtype`
got their first writer from a **catalogue** source and the \*Arr sync has still written neither. The
comment is not edited here because it transcribes migration 0005's own SQL, and **a merged migration
is never edited** — so the same superseded sentence stands in
`internal/db/migrations/00005_library_sync.sql` and is history there. `internal/libsync` is what
answers who writes these columns today.

⚠️ **`remote_identity_hash`'s *"at first sight"* is superseded in the same way, and is likewise left
standing** (2026-08-21, [ADR-0074](../DECISIONS.md#adr-0074)). The column comment above reads *"hash
of the remote's external ids, at first sight; the O(1) guard against id resurrection (sync.md §4)"*,
and it transcribes migration 0005, so it is not edited here for the reason the paragraph above gives.
**What the tree does is one transition and no other: `empty → present`.** The value is written at
first sight; the ONLY later write permitted is an arriving identity replacing the empty-list hash of
an item nobody had matched yet. An **established** identity never moves again, against the upstream
or against anyone who has got at the upstream, and NULL is not empty and is left alone. The
unnarrowed rule — first sight and never overwritten — read *"the operator matched this book"* as
*"the upstream repointed this id"* and hard-deleted a live link for it, which is the ordinary
BookOrbit lifecycle rather than an attack. `internal/store/catalogue.go`'s `applyOneItem` step 7 is
what implements it, and ARCHITECTURE §7.4 carries the rule.

**`ux_sil` is only usable in full if `remote_kind` is known at lookup time**, which is why the
northbound ID encodes it (ARCHITECTURE §5.3). Without it, `WHERE service_instance_id=? AND
remote_id=?` yields `SEARCH service_item_link USING INDEX ux_sil (service_instance_id=?)` — a range
scan over every link on that instance, ~400k rows for a 2k-series Sonarr, on every stream resolve
and every `getCoverArt`.

**Authority rule when instances disagree on shared metadata:** highest `priority` among
`is_authoritative` links wins; otherwise most-recently-synced. Log divergences.

**Which upstream field populates each container column, per adapter.** All three are stored
**verbatim, as the upstream reported them** — this table never parses, derives or normalises a
container value, which is what keeps membership a deterministic predicate rather than a guess
(ARCHITECTURE §6.5 rule 3). A blank cell means the service has no such concept and the
corresponding `container_kind` is not offered for it.

| Instance kind | `remote_library_id` | `remote_tag_ids` | `remote_subtype` | `root_folder_path` |
|---|---|---|---|---|
| Sonarr | — | `SeriesResource.tags[]` | `SeriesResource.seriesType` (`standard\|daily\|anime`) | `SeriesResource.rootFolderPath` |
| Radarr | — | `MovieResource.tags[]` | — | `MovieResource.rootFolderPath` |
| Navidrome | `libraryId` on the album/artist row (ND's native API) | — | — | — |
| Audiobookshelf | `LibraryItem.libraryId` | — | `Library.mediaType` (`book\|podcast`) | `LibraryItem.folderId` ⚠️ a folder **id**, not a path — never prefix-compared |
| Komga | `SeriesDto.libraryId` | — | — | — |
| Kavita | `Series.libraryId` | — | `Library.type` (`LibraryType` enum) | — |

⚠️ **Audiobookshelf's `folderId` is an opaque id, not a filesystem path.** It is stored in
`remote_library_id`-adjacent form only as an id and is **never** used with the `root_folder`
container kind, whose predicate is a string prefix on a path. Getting that wrong would be UsArr
comparing paths it did not receive — the one thing §6.5 rule 3 forbids.

### 5.1 `indexer_catalog` — the indexer-list replica · **v0.1, migration 0004**

The local replica of each indexer service's own indexer list, so the Search screen's indexer and
category filters render from SQLite instead of from Prowlarr.

```sql
CREATE TABLE indexer_catalog (
  service_instance_id INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE CASCADE,
  indexer_id          INTEGER NOT NULL,   -- the SERVICE's id; what ?indexer= sends back
  name                TEXT NOT NULL,
  protocol            TEXT NOT NULL DEFAULT 'unknown',  -- usenet|torrent|unknown
  privacy             TEXT NOT NULL DEFAULT '',         -- public|semi-private|private
  enabled             INTEGER NOT NULL DEFAULT 0,
  priority            INTEGER NOT NULL DEFAULT 0,       -- 1-50, LOWER WINS
  supports_search     INTEGER NOT NULL DEFAULT 0,
  supports_rss        INTEGER NOT NULL DEFAULT 0,
  supports_pagination INTEGER NOT NULL DEFAULT 0,
  search_types        TEXT NOT NULL DEFAULT '[]',       -- JSON: search|tvsearch|movie|music|book
  limits_max          INTEGER,                          -- NULL = the indexer advertised none
  limits_default      INTEGER,
  categories          TEXT NOT NULL DEFAULT '[]',       -- JSON {id,name,sub_categories}, RAW ids
  fetched_at          TEXT NOT NULL,
  PRIMARY KEY (service_instance_id, indexer_id)
) STRICT;

CREATE INDEX ix_indexer_catalog_instance ON indexer_catalog(service_instance_id, name);
```

🚩 **The column list is an ALLOWLIST, and that is a security property rather than a style
preference.** Prowlarr's `IndexerResource` carries a `fields[]` array whose entries have
`privacy ∈ {password, apiKey, userName}` — for a private tracker that is the user's **passkey**, its
RSS key, its API key or its session cookie, and a leaked passkey is account termination because it
is what the tracker attributes traffic by. There is deliberately **no `fields` column, no cookie
column and no raw-resource column**, so the credential has nowhere to land; the projection that
fills these rows (`internal/servarr/mapping.FromProwlarrIndexer`) names every value one at a time,
and `search_types` / `categories` hold **UsArr's own JSON**, marshalled from that projection, never
re-serialised upstream JSON. `TestMigration0004NeedsNoRebuild` pins the exact column set so adding
one is a deliberate act. Same rule, same reason as `release_candidate.info_url` in §6.

**Written by the background prober only, replace-whole, in one transaction.** Upstream is
authoritative: an indexer deleted in Prowlarr disappears here rather than lingering as a filter that
silently matches nothing. A failed refresh writes nothing and leaves the last good copy standing —
an empty picker is worse than a stale one, and `indexers_fetched_at` is what says how stale.

**`service_instance.indexers_fetched_at` is nullable with no default, and that is load-bearing.**
Three states have to be distinguishable from the data alone, because `GET /api/v1/indexers` says a
different sentence for each: no indexer service configured; one configured that UsArr has never
successfully read; and one that answered and genuinely has zero indexers. The last two both have
zero rows here, so the fetch timestamp cannot live on the rows. NULL means never.

**The one read is `WHERE service_instance_id = ? ORDER BY name ASC`**, one instance per call. `name`
is the index's second column so the order comes from the index rather than a temp b-tree; an
`IN (…)` over a set cannot supply the order (the effect pinned in
`TestScopedProvenanceOrderNeedsASort`), and a homelab has single-digit instances, so the caller's
loop is cheaper than the sort it avoids. Pinned by
`TestIndexerCatalogReadUsesTheInstanceIndex`.

---

---

## 6. Provenance and release candidates · **v0.1**

```sql
CREATE TABLE provenance (
  id                 INTEGER PRIMARY KEY,
  user_id            INTEGER NOT NULL DEFAULT 0,   -- who acquired it. NO foreign key: see below
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
  size_bytes       INTEGER,            -- as the indexer reported it; NULL = not reported
  published_at TEXT, grabbed_at TEXT, imported_at TEXT,
  source_system    TEXT NOT NULL,      -- sonarr|radarr|prowlarr|manual|filesystem
  source_record_id TEXT,
  confidence       REAL NOT NULL DEFAULT 1.0,
  acquisition_state TEXT NOT NULL DEFAULT 'confirmed'  -- confirmed|unconfirmed. Migration 0003
) STRICT;
CREATE INDEX ix_prov_protocol ON provenance(protocol);
CREATE INDEX ix_prov_indexer  ON provenance(indexer_name);
CREATE INDEX ix_prov_dlid     ON provenance(download_id);
CREATE INDEX ix_prov_user_grabbed ON provenance(user_id, grabbed_at DESC, id DESC);
CREATE INDEX ix_prov_unconfirmed  ON provenance(user_id, grabbed_at DESC, id DESC)
  WHERE acquisition_state <> 'confirmed';

CREATE TABLE release_candidate (
  id                  INTEGER PRIMARY KEY,
  user_id             INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
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

> 🚩 **`acquisition_state` is not optional to read, and it arrived in migration 0003.** A Prowlarr
> grab that returns 500 may have succeeded — Prowlarr adds the release to the download client before
> it configures it and never rolls back, so a 200 is the only confirmation the API offers
> (`reference/arr-apis.md` §7). UsArr writes a provenance row for that ambiguous outcome **solely to
> keep `download_id`**, the infohash that is the only join key an importer later supplies; without
> the row a torrent on disk can never be attached to the grab that produced it. **So every read that
> joins on `download_id` must carry `acquisition_state`**: an `'unconfirmed'` row is a *reservation
> of the key*, never an acquisition, and a join that treats it as one attaches history UsArr never
> verified — which is worse than the missing row it replaced. `store.GetProvenanceByDownloadID`
> returns the column for that reason.
>
> It is a separate column rather than a demoted `confidence` because **`confidence` means match
> confidence** and §7's `ux_extid_work_strong` gates at `>= 1.0`: an unconfirmed acquisition is
> perfectly *identified*, and demoting it would hide it from the reads that most want it. It carries
> **no `CHECK` constraint**, exactly as `audit_log.result` carries none — SQLite cannot `ALTER` one,
> 0001's `audit_log` foreign key is what that costs, and v0.2's request path may want a `'pending'`.
> The vocabulary lives in `internal/store/releases.go` and is enforced there.

> ⚠️ **`user_id` on these two tables arrived in migration 0002, not 0001, and the two are treated
> differently on purpose.** Both are `NOT NULL DEFAULT 0` — 0 being the shared/system sentinel — and
> both were backfilled to it, which is honest because v0.1 is single-user and every existing row was
> the owner's. The canonical read predicate is `user_id IN (0, :uid)`, the same one `tag_assignment`
> uses, so those backfilled rows stay visible to the owner who made them.
>
> **`release_candidate.user_id` carries `REFERENCES user(id) ON DELETE CASCADE`**, matching
> `write_queue` and `tag_assignment`. It is ephemeral operational state with a 25-minute TTL, and a
> deleted user's pending search results should go with them.
>
> **`provenance.user_id` carries NO foreign key**, for exactly the reason `audit_log.actor_user_id`
> carries none (§9). It is a *historical* id: the user may since have been deleted and the row must
> still say who acquired the file. `CASCADE` would destroy acquisition history — the one thing this
> table exists to keep; `SET NULL` cannot be spelled on a `NOT NULL` column and would erase the same
> thing; `NO ACTION`/`RESTRICT` makes `DELETE FROM user` fail for anyone who has ever grabbed
> anything. Dropping the reference is the only option that permits the delete and keeps the actor.
>
> **No table rebuild was needed** and that was checked, not assumed: nothing here alters a `CHECK`,
> `ADD COLUMN` with a non-NULL default rewrites the stored DDL without touching the rows, and
> `ADD COLUMN … NOT NULL DEFAULT 0 REFERENCES user(id)` was executed against this project's driver
> under `foreign_keys=ON` and both accepted and enforced. `TestMigration0002NeedsNoRebuild` pins it.

**`ix_prov_user_grabbed` serves the Recent-grabs read, and only for a single-user predicate.**
`EXPLAIN QUERY PLAN` on `WHERE user_id = ? ORDER BY grabbed_at DESC, id DESC LIMIT 50` yields
`SEARCH provenance USING INDEX ix_prov_user_grabbed (user_id=?)` with no temp b-tree — a search, not
a covering index, because the SELECT list carries `release_title`, `indexer_name` and `size_bytes`;
per §1 the query-plan assertion must pin `SEARCH … USING INDEX`, never `COVERING INDEX`. `id DESC` trails
`grabbed_at` because `grabbed_at` has one-second resolution and keyset paging would otherwise repeat
a row. **With the canonical `user_id IN (0, :uid)` predicate SQLite adds `USE TEMP B-TREE FOR ORDER
BY`**: it cannot supply order from an index whose *leading* column is constrained by `IN`. The index
still restricts the scan to the readable rows and the sort is bounded by the `LIMIT`;
`TestScopedProvenanceOrderNeedsASort` pins that so the gap is recorded rather than rediscovered.

**`release_candidate` has no uniqueness on `(service_instance_id, guid)`, and that is a decision, not
an omission.** Two searches for the same term inside the TTL window insert two full copies of every
release; the in-search `emitted` dedupe map is per-search only. The bound on that duplication is the
TTL — 25 minutes for Prowlarr-sourced rows, swept via `ix_rel_expiry` — so the table's size is
governed by search volume within a 25-minute window, not by uptime. For a single-user homelab that
is a handful of rows, and each duplicate is independently grabbable, so nothing is incorrect.

The alternative — a unique index plus `INSERT … ON CONFLICT DO UPDATE SET fetched_at=…,
expires_at=…` — was considered and rejected for v0.1 because it makes every re-search *extend* the
grab window of a candidate whose upstream cache entry is still pinned to the original 30-minute
Prowlarr TTL. That converts a visible "search again" into an invisible failure at grab time, which is
a worse outcome than duplicate rows. Revisit if a multi-user deployment makes write amplification on
the single writer connection measurable; the fix is additive (a unique index plus an upsert) and
needs no data migration beyond de-duplicating existing rows.

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
  kind         TEXT NOT NULL,           -- top-level kinds only; see invariant 3 below
  popularity   REAL NOT NULL DEFAULT 0,
  in_library   INTEGER NOT NULL DEFAULT 0,
  title_idf    REAL NOT NULL DEFAULT 0,
  norm_title   TEXT NOT NULL
) STRICT;
CREATE INDEX ix_sd_work ON search_doc(work_id);

-- Library visibility is a JUNCTION TABLE, not a column on search_doc.
CREATE TABLE search_doc_library (
  library_id INTEGER NOT NULL REFERENCES library(id)      ON DELETE CASCADE,
  doc_rowid  INTEGER NOT NULL REFERENCES search_doc(rowid) ON DELETE CASCADE,
  PRIMARY KEY (library_id, doc_rowid)
) STRICT, WITHOUT ROWID;
CREATE INDEX ix_sdl_doc ON search_doc_library(doc_rowid);
```

> 🚩 **This replaces `search_doc.library_scope TEXT`, a JSON array, and the replacement is
> load-bearing rather than tidy-minded.** ARCHITECTURE §8.2 and §1 of this file both require that
> permission filtering happens **in the index join, never after it**, and both state the reason: a
> post-filter silently breaks keyset page sizes and leaks existence through result counts and
> ranking positions. **A JSON array in a `TEXT` column cannot participate in an index join.**
> Filtering it needs `json_each(library_scope)` or `LIKE '%"7"%'`; both are scans, neither is
> seekable, and no index on the column would help. So the column bought a full scan of the fused
> candidate set on the hot search path for all six media types — to satisfy a requirement it could
> not satisfy.
>
> With the junction table the scoped query is
> `… JOIN search_doc_library sdl ON sdl.doc_rowid = sd.rowid AND sdl.library_id IN (…)`, which is a
> covered index seek per scoped library. **`make check` asserts the plan** (§7 invariant 6) so it
> can never silently regress to a scan.
>
> The `WITHOUT ROWID` primary key is `(library_id, doc_rowid)` in that order because the scope is
> the outer filter and the doc set is the inner range. `ix_sdl_doc` serves the reverse — deleting a
> `search_doc` row, and answering "which libraries can see this" for the item detail page.

**Six invariants, all silent-corruption sources if broken. Five are asserted by `make check`;
invariant 4 is owed, and waits for the code it constrains.**

⚠️ **Asserted by the gate is not schema-enforced, and two of the six are neither declarable nor
enforced — which this list previously left a reader to assume the other way.** Both are stated plainly below,
each with the reason SQLite cannot hold it and the code that owes it. Neither is a defect to be
fixed by a migration; both are debts to be paid by the document builder, and they are written down
here so that whoever writes it inherits them rather than discovers them.

1. `search_fts.rowid == search_trgm.rowid == search_doc.rowid`. The id is allocated by inserting
   into `search_doc` first, then inserted **explicitly** into both FTS tables in the same
   transaction. RRF fuses on `rowid`; a single implicit rowid fuses unrelated documents.
2. `count(search_fts) == count(search_trgm) == count(search_doc)`. Every `work` delete and every
   title change issues the matching FTS `DELETE` in the same transaction. Without
   `contentless_delete=1` this is impossible — a plain contentless table answers
   `cannot DELETE from contentless fts5 table`.

   🚩 **A CODE INVARIANT, and undeclarable in principle.** The two FTS5 tables are *virtual*: they
   can carry no foreign key and no `CHECK`, and no trigger can be created on them. Nothing in
   SQLite can hold three row counts equal. Measured: `DELETE FROM work` cascades `search_doc` away
   and leaves **both** FTS tables holding a document for a work that no longer exists.
   **OWED BY:** the search-document builder — the code that writes `search_doc`, `search_fts` and
   `search_trgm` — which must issue all three deletes in one transaction. Nothing writes those
   three tables yet. `TestSearchDocVisibilityIsACodeInvariant` executes the divergence and pins the
   count query.
3. `SELECT COUNT(*) FROM search_doc WHERE kind IN ('season','episode','track','comic_issue',
   'person')` is **0**. `comic_issue` is in the list for the same reason as the first three: a large
   manga library's chapter titles would swamp every query (ADR-0030). `person` is there for a
   different reason — not corpus volume but the absence of a destination: there is no person screen
   in any milestone, so a person hit would be a result row with nowhere to go (ADR-0033).
4. **No query in the identity path references `library_member`, `library_source` **or
   `library_override`** (ADR-0026). Library membership is never an input to identity —
   jellyfin#10985 is what happens when it is. `library_override` is named explicitly because it is
   the one library-named table that, by design, *does* feed identity: its `relink` verb repoints a
   `service_item_link`. The assertion therefore reads: the identity **cascade** (§6.4 tiers 1–5)
   references none of the three; the correction **applier**, which runs after the cascade and
   overrides its output, references `library_override` and nothing else. Two code paths, one
   assertion each.
5. **Every `search_doc` row has at least one `search_doc_library` row.** A row visible through no
   library matches no scope and is invisible in search to every user *including the owner* — a
   disappearance the old `instance_scope` could not produce, because every replicated row came from
   some instance.
   `SELECT COUNT(*) FROM search_doc sd WHERE NOT EXISTS (SELECT 1 FROM search_doc_library sdl
   WHERE sdl.doc_rowid = sd.rowid)` must be **0**.

   🚩 **ALSO A CODE INVARIANT. Reserved `library.id = 0` (*Unfiled*, §13) is the PLACE the builder
   files an otherwise-unfiled work; it is not a mechanism that upholds anything on its own,** and
   the sentence that used to read *"the invariant is upheld by reserved `library.id = 0`"* said
   otherwise. SQLite has no "at least one child row" constraint, and no trigger position expresses
   it: at insert time the doc necessarily exists *before* its junction row, and the two ways the
   invariant breaks are both **cascades** — `DELETE FROM library`, and `DELETE FROM user`, which
   reaches `library` through `library.user_id` — where an `AFTER DELETE` trigger on the junction
   cannot tell *"this doc lost its last scope"* from *"this doc is being deleted too"*. Both paths
   are executed in `TestSearchDocVisibilityIsACodeInvariant` and both leave an orphan.
   **What the schema does now do** is refuse to delete library 0 itself (`BEFORE DELETE` trigger
   `trg_library_unfiled_no_delete`), which closes the worst single path and, deliberately, also
   blocks `DELETE FROM user WHERE id = 0`.
   **OWED BY:** the same document builder, which must re-file a stranded doc into library 0 in the
   same transaction as any library or user delete.
6. **The scoped search plan is a seek, not a scan.** `EXPLAIN QUERY PLAN` on the scoped query must
   contain `SEARCH sdl …` and must not contain `SCAN sdl` / `SCAN search_doc_library`.

   ⚠️ **This invariant used to name one exact plan —
   `SEARCH sdl USING PRIMARY KEY (library_id=? AND doc_rowid=?)` — and no query produces that
   string.** Measured against the real schema (SQLite 3.53.4): which of the junction's two indexes
   the planner reaches for depends on which side drives the join, and there are two real shapes.

   | Query shape | Measured plan |
   |---|---|
   | doc set known — the RRF fusion, candidates from `search_fts`, scope applied per candidate | `SEARCH sdl USING COVERING INDEX ix_sdl_doc (doc_rowid=? AND library_id=?)` |
   | scope leading — browse a scope with no query text | `SEARCH sdl USING PRIMARY KEY (library_id=?)` |

   ⚠️ **The `sdl` in those two plan lines is the alias, not the table**, and in the statement
   `internal/store` ships it is no longer that constant: the junction is aliased per leg —
   `sdl_f`, `sdl_t` — because a hard-coded inner alias could be shadowed by the caller's own
   (`searchDocLibraryAlias`, REVIEW-LOG LS-379). The plan assertions derive the name; an assertion
   spelling `SEARCH sdl` would go on matching `SEARCH sdl_f` by prefix while covering nothing.

   Both are seeks and neither is a scan, which is the invariant's substance: permission filtering
   happens **in** the index join. `TestScopedSearchIsASeekNotAScan` asserts it in both directions,
   plus the full FTS arm.

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
CREATE INDEX ix_audit_actor_action ON audit_log(actor_user_id, action, ts DESC);

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

**`ix_audit_actor_action` (migration 0002) serves "this user's grab failures"**, the Home screen's
attention block. `ix_audit_ts` orders the whole log and cannot filter it, so that read scanned a
table that grows forever by design. The two equality columns lead and `ts DESC` trails them, so
newest-first comes out of the index rather than a temp b-tree. `EXPLAIN QUERY PLAN` reports it as a
*covering* index for a narrow SELECT list, and per §1 the query-plan assertion still pins only
`SEARCH … USING INDEX`: covering-ness depends on the SELECT list, so pinning it would fail the moment
a caller selects one more column, which is a change to the query and not an index regression.

**The read that uses it is `store.ListAuditLog(ctx, scope, AuditQuery{Actions, Results})`**, and
until it grew a scope and those filters the index was used by **no query in the codebase at all** —
an index that had never been exercised, which is indistinguishable from no index. It is now pinned by
`TestAuditReadUsesTheActorActionIndex`, which `EXPLAIN`s the statement the function itself builds
rather than a copy pasted into a test.

**With the canonical `actor_user_id IN (0, :uid)` predicate SQLite adds `USE TEMP B-TREE FOR ORDER
BY`**, exactly as it does for `ix_prov_user_grabbed` on the other arm of the same union and for the
same reason: it cannot supply order from an index whose *leading* column is constrained by `IN`. The
sentence above about newest-first coming out of the index holds for the single-user equality form
only. The index still cuts the walk from every row ever appended to one actor's rows for one action,
and the sort is over that bounded set under a `LIMIT`; `TestScopedAuditOrderNeedsASort` pins it so
the gap is recorded rather than rediscovered. A row with `actor_user_id IS NULL` matches no scope —
`NULL IN (0, 1)` is unknown — so an unauthenticated action is readable by nobody, which is why the
grab path deliberately writes no audit row for a request that arrived without a session.

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

🚩 **`ix_wq_runnable` is PARTIAL, and a query only reaches it by spelling the predicate
verbatim.** SQLite matches a partial index's `WHERE` clause syntactically and does not derive
implication, so the obvious way to write *"claim the next runnable row"* —
`WHERE state = 'pending' AND next_attempt_at <= ?` — is a logically strict **subset** of the
index's predicate and plans as `SCAN write_queue`, a full scan of a table that grows with every
command UsArr ever issues. The three-state form
`WHERE state IN ('pending','inflight','verifying') AND next_attempt_at <= ?` plans as
`SEARCH write_queue USING COVERING INDEX ix_wq_runnable (state=? AND next_attempt_at<?)`. Both
plans are measured and both are pinned by `TestWriteQueueRunnableNeedsTheVerbatimINList`, so the
constraint is met in `make check` rather than in production. **Any narrower sweep must filter the extra
states in the SELECT list, not in the `WHERE` clause.**

`ix_wq_runnable` is also the reconciliation guard's index. [`sync.md`](./sync.md) §4 states the guard,
and the scope words are load-bearing: *"The sweep may correct an item **toward the \*Arr** only when
there is **no `write_queue` row for that work in `pending`, `inflight` or `verifying`**."* It bounds
**outbound** corrections — writes UsArr would push at the \*Arr — and says nothing about the sweep's
local soft-delete and tombstone-expiry path. An earlier revision of this line dropped *toward the
\*Arr*, and [ADR-0039](../DECISIONS.md#adr-0039) records what that omission cost when the shortened
form was quoted back as an argument.

### ⚠️ Do this during the library-sync migration's `write_queue` rebuild

**There is no `state` value meaning "waiting for a human", and a two-phase asynchronous request
destination needs one.** Every non-terminal state above assumes a *machine* owes the row an answer:
`pending` is claimed by a worker on its next pass, `inflight` means an upstream request is
outstanding, and `verifying` carries `verify_until` — a 15-minute TTL that ends in one final
verification and an explicit `failed` (ADR-0012a, ARCHITECTURE §7.6), which for a row nobody has
answered resolves to `fail_reason = 'unknown'`. A sink whose flow is *search → present results →
**a person chooses** → enqueue* (`FUTURE.md` §11) has to park a row between phase one and phase two
for however long its owner takes to come back, and none of those four states can hold it: a user
asleep is not a failed request. `kind` is unconstrained free text, so the *verb* half of that flow is
already expressible; the `state` half is not.

**The fix is deliberately deferred to the migration that ships library sync, because that migration
must rebuild this table anyway.** `write_queue.work_id` is `INTEGER` with **no** foreign key in
`00001_initial.sql` — the `REFERENCES work(id) ON DELETE CASCADE` shown above is dropped there, with
a comment naming library sync as the migration that restores it, because at the time `00001` was
written there was no `work` table to reference. (**Whether there is one now, and which migration
restored the key, is answered by `internal/db/migrations` and by nothing in this file.**)
SQLite cannot add a foreign key to an existing column, so restoring it costs a full 12-step table
rebuild that is **already mandatory**. Adding a `CHECK` value during a rebuild that is happening
regardless costs nothing, and SQLite equally cannot `ALTER` a `CHECK` constraint, so doing it any
earlier would mean either editing a merged migration or paying for a second rebuild. Neither is
worth it while nothing is released. **`00001_initial.sql` is not to be edited for this.**

⚠️ **The rebuild that instruction was written for has since been decided, and not as step 1
specified.** [ADR-0039](../DECISIONS.md#adr-0039) supersedes step 1 and carries the argument, the
rejected alternatives and the two things the list did not name;
`internal/db/migrations/00005_library_sync.sql`'s header carries that reasoning **as it stood when
0005 landed** — provenance rather than a current summary, since a merged migration is never edited and
the ADR has gained riders since — and `internal/db/testdata/schema.sql` is the current shape. **ADR-0039
is authoritative for where the reasoning stands now, and its Status line is where the divergence is
explained**; this file does not keep a second account of it. Read those, not a summary here — this
file does not own what a migration did.

⚠️ **Still owed, and not by 0005.** 0001 also drops `tag_assignment.work_id` / `.edition_id` /
`.media_file_id` and `release_candidate.work_id`, with comments naming "the migration that ships
library sync". Each is a further 12-step rebuild, neither table has a blocked reader, and nothing is
closing — so the comments in 0001 are pointers to a rebuild that is still owed. Whether it has been
written is `internal/db/migrations`' answer.

The DDL block above shows the shape as `00001_initial.sql` created it, because the reasoning under it
(`ux_wq_idem`, `fail_reason`'s `NULL` trap) is still the reasoning.

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
  source_url TEXT NOT NULL UNIQUE,      -- CREDENTIAL-STRIPPED. See security.md §5.
  origin_class TEXT NOT NULL CHECK (origin_class IN ('configured','derived','provider')),
  origin_service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE SET NULL,
  role       TEXT NOT NULL,             -- poster|backdrop|banner|cover|logo|headshot
  width INTEGER, height INTEGER,
  thumbhash  BLOB,
  dominant_color TEXT,                  -- available BEFORE thumbhash; see ARCHITECTURE §4.4.1
  cache_key  TEXT NOT NULL,             -- sha256(credential-stripped source_url)[:16]
  etag TEXT, last_modified TEXT,
  fetched_at TEXT, expires_at TEXT,
  state      TEXT NOT NULL DEFAULT 'pending', -- pending|ready|failed|gone
  format     TEXT                           -- NULL until encoded. jpeg. Added by 00008; ADR-0050
) STRICT;
CREATE INDEX ix_img_state ON image_asset(state, expires_at);
```

`origin_class` and `origin_service_instance_id` are what let the fetcher select an SSRF policy from
the **row** rather than from the URL string, which is the fix for the derived-URL class
(security.md §2). The ingest path that writes these rows must reject a `source_url` that still
carries a credential parameter. Which parameter names count is `internal/ssrf/redact.go`'s
`credentialParams` to say, and this file deliberately keeps no second copy of that list: a restated
subset is correct only until the code moves, and nothing signals when it does.

**`format` records what UsArr's own encoder produced, not what the origin server served.**
[ARCHITECTURE.md](../ARCHITECTURE.md) §4.4's ingest-time downscale to a seven-width allowlist puts an
encoder on six of the seven widths by construction, and [ADR-0050](../DECISIONS.md#adr-0050) clause 1
puts one on the seventh, so the upstream's declared media type is history. Four properties, each
decided in ADR-0050 and argued at length in `00008_image_asset_format.sql`'s header:

- **It is ONE codec for the WHOLE row, `orig` included.** §4.4 stores **up to seven widths per
  asset** and this column is **one per row**, so it is correct only if every rendition of an asset
  shares a codec — which ADR-0050 clause 1 makes a rule: *there is no passthrough width*. Per-`role`
  variation (PNG for logos) stays expressible, because `role` is a column on the same row;
  per-**width** variation is foreclosed, and reopening it costs an ADR amendment plus a second
  column. ⚠️ Nothing enforces this yet — there is no pipeline — so it is discharged by definition,
  and the first writer is the code that owes it. **[FUTURE.md](../FUTURE.md) §21 states that debt as
  a debt**: what breaks if it is violated, and the two options open to the code that first stores a
  rendition. Read it before writing the fetch path.

- **It is a lowercase codec token, not a media type** — `jpeg`, not `image/jpeg`, and not the `jpg`
  file suffix. `Content-Type` is derived from it by the `/img` surface; the reverse is parsing, and
  a stored media type would admit both `image/jpeg` and the wrong-but-common `image/jpg` for one
  encoding.
- **NULL means "no encoded bytes exist for this row yet"**, and it is the value a row carries at
  INSERT time, when `state` is `pending` and no encoder has run. There is no default. A
  `NOT NULL DEFAULT 'jpeg'` would have made "encoded as JPEG" and "never encoded" the same value.
- **There is no `CHECK`**, on ADR-0039's reasoning and the same reasoning `role` and `state` above
  already follow: the vocabulary is expected to grow (`jpeg` today, `avif` if ADR-0050's deferral is
  reopened, possibly `webp`) and SQLite cannot `ALTER` a `CHECK`. ⚠️ Unlike ADR-0039's promise for
  `write_queue.state`, **the Go-side enforcement shipped with the column**:
  `internal/store/images.go` declares the vocabulary and `ValidImageFormat` checks it, and
  `TestImageWritesValidateTheFormatVocabulary` fails the build if a writer against `image_asset`
  lands without referencing the validator.

**`cache.db`** is a second, disposable database holding `http_cache(url_hash, etag, last_modified,
body_hash, fetched_at, expires_at)`, the job queue, unowned-search result caching, and image-cache
metadata. Deleting it costs a re-sync, not data.

### 12.1 `sync_report` — what the sweep found · **v0.1**

🔍 **Inference, and marked as such.** This table was named in the appendix as v0.1 with *no DDL
anywhere in this document*, so the shape below was derived when it was created, from its only two
specified call sites: [`sync.md`](./sync.md) §4 step 5 (*"emit a `sync_report` row"* per sweep) and
§4 guard 1's `sync_report{kind: "id_reused", instance, remote_kind, remote_id}`. It is not prior
art. ⚠️ **It has since been exercised, and by neither of those two call sites**: this sentence read
*"nothing has exercised it — nothing writes this table yet"*, and the shape is now round-tripped by
`internal/store`'s `RecordSyncReport`, called from the full import (`internal/libsync`) under two
`kind` values of its own — `container_declined` and `identity_conflict`. §4's sweep, the caller the
shape was actually derived from, still does not exist, so the inference above is only half
discharged: the columns hold a row, and neither `kind` the vocabulary was invented for has been
written by anything.

```sql
CREATE TABLE sync_report (
  id                  INTEGER PRIMARY KEY,
  service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE CASCADE,
  kind                TEXT NOT NULL,   -- no CHECK; see below
  remote_kind         TEXT,
  remote_id           TEXT,
  work_id             INTEGER,         -- no foreign key; see below
  detail              TEXT,            -- JSON, REDACTED on the way in
  created_at          TEXT NOT NULL DEFAULT (datetime('now'))
) STRICT;
CREATE INDEX ix_sync_report_instance ON sync_report(service_instance_id, created_at DESC);
```

**No `CHECK` on `kind`**, for ADR-0039's reason: this is the newest and least settled vocabulary in
the schema — `sync.md` names one value and the sweep will add its own as channels 3b and 4 are
built — and SQLite cannot `ALTER` a `CHECK`.

**No foreign key on `work_id`**, for `provenance.user_id`'s reason (§6): it is a *historical* id.
*"Work 4412's link was rebound because Sonarr reused id 842"* must survive work 4412 being deleted,
and `CASCADE` would erase the report of exactly the event most worth reporting.

**Append-only by convention, not by trigger.** `audit_log`'s triggers exist because it is a
tamper-evidence chain; this is an operational log, and a trigger here would recreate `audit_log`'s
foreign-key problem (§9) for no security benefit.

⚠️ **`detail` holds upstream response text, so it is redacted on the way in** — `security.md` §5 and
REVIEW-LOG R-08: Mylar3 returns configured indexer API keys in a response *body*, and Kavita carries
its key in a URL *path* segment, so a query-parameter deny-list is not sufficient.

---

## 13. Libraries — the user's organisation · **v0.1**

Design and reasoning: [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §6.5 and ADR-0026. Everything an
implementer needs is here rather than in prose: types, CHECK lists with their real allowed values,
keys, `ON DELETE` behaviour, indexes, and the `user_id` principle 4 requires.

⚠️ **This section used to open "All four tables are in migration 0001".** That was false — none of
them existed in any migration when it was written. Read `internal/db/migrations` for what exists.

### 13.1 `library`

```sql
CREATE TABLE library (
  id            INTEGER PRIMARY KEY,
  -- Principle 4 / ADR-0019 / ARCHITECTURE §1.3 rule 1: a user-scoped row carries user_id from
  -- migration 0001. A library IS user-scoped — ADR-0026 calls it "user-owned" and §6.5 rule 5
  -- says it "carries a user's name, corrections and access grants". Sentinel 0 is the owner,
  -- exactly as tag_assignment and library_override use it. See "Who owns a library" below.
  user_id       INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  slug          TEXT NOT NULL,        -- URL identity for ?lib= (ARCHITECTURE §17.2). Allocated
                                      -- once from the name at creation and then DURABLE:
                                      -- renaming a library must not change its permalink.
  kind          TEXT NOT NULL CHECK (kind IN (
                  'movie','series','artist','album','book','comic','game')),
                                      -- Exactly one, required, and EDITABLE (§6.5 rule 4).
                                      -- Only top-level kinds: a library of episodes or tracks is
                                      -- not a thing, and the search corpus rule (§7) filters the
                                      -- same set. 'season','episode','track','comic_issue' and
                                      -- 'person' are deliberately absent — a library of authors is
                                      -- not a thing either (ADR-0033).
  formats       TEXT,                 -- JSON array over edition.format, or NULL for "any".
                                      -- ["ebook"] and ["audiobook"] over one Audiobookshelf
                                      -- library are the flagship case (§17.8).
  icon          TEXT,
  sort_order    INTEGER NOT NULL DEFAULT 0,   -- 'order' is a SQLite keyword
  enabled       INTEGER NOT NULL DEFAULT 1,
  include_in_search INTEGER NOT NULL DEFAULT 1,
  default_sort  TEXT NOT NULL DEFAULT 'sort_title'
                  CHECK (default_sort IN ('sort_title','added_at','year','popularity')),
  -- The declared request sink (ARCHITECTURE §8.3). A PIN INSIDE the capability filter, never a
  -- bypass: an instance that does not probe Caps.MediaKinds ∋ (kind, format) and advertise Add
  -- cannot be chosen. NULL sink is a first-class state, not an error.
  sink_service_instance_id INTEGER REFERENCES service_instance(id) ON DELETE SET NULL,
  sink_quality_profile_id  INTEGER,   -- upstream's own id, verbatim; fetched live when the
                                      -- settings panel opens, never on a render path
  sink_root_folder_path    TEXT,
  sink_tag_ids             TEXT,      -- JSON array of upstream tag ids
  managed_by    TEXT NOT NULL DEFAULT 'auto' CHECK (managed_by IN ('auto','user')),
                                      -- 'auto' = created by the proposal flow and still tracking
                                      -- its source; 'user' = the user edited it, so the proposal
                                      -- flow never rewrites it again.
  orphaned_at   TEXT,                 -- set when the last library_source goes away; the row is
                                      -- RETAINED and shown with its reason (§6.5 rule 5)
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
) STRICT;

CREATE UNIQUE INDEX ux_library_slug ON library(user_id, slug);
CREATE UNIQUE INDEX ux_library_name ON library(user_id, name);
CREATE INDEX ix_library_kind ON library(user_id, kind) WHERE enabled = 1;
```

⚠️ **`orphaned_at`'s *"when the last library_source goes away"* is not what the writer does, and the
column comment is left standing** (2026-08-21, [ADR-0074](../DECISIONS.md#adr-0074)) — it transcribes
migration 0005, and a merged migration is never edited. **No `library_source` row ever goes away.**
`internal/store/reconcile.go`'s `sweepOrphans` stamps the column when no source of the library is
still being **reported**; the rows are all retained, and the ones that stopped being reported carry
`missing_since` instead. It is also the only one of the sweep's four stamps with a clearing arm,
because nothing else in the tree clears it and a flag that can only be set marks a library
permanently orphaned the moment its upstream blinks. ⚠️ **AND A SOURCE ON A SOFT-DELETED
`service_instance` IS NOT A SOURCE.** The FK cascade fires on a HARD delete only, so soft-deleting an
instance leaves its `library_source` rows in place with `missing_since` NULL — invisible to
`librarySourcesSQL`, which filters `service_instance.deleted_at IS NULL`, and, before this was fixed,
enough to satisfy the orphan rule's `NOT EXISTS` forever. Both arms of the rule now require the
source's instance to be live, so the read and the sweep use one definition; and because no sweep ever
runs for a deleted instance, `SoftDeleteServiceInstance` performs the orphan pass itself, inside its
own transaction. **Deleting an instance is an orphaning event, and it is the one no sweep can reach.**

**Who owns a library — the question §6.5 left with two live readings, settled here.** ADR-0026 and
§6.5 call a library *user-owned*; §12.2 names `user_library_access` (v1.0) as the mechanism for
library visibility. Read literally those are two different schemas. **The resolution is that both
exist and they mean different things:** `library.user_id` records the **owner**, the user who
created it and whose name, ordering and corrections it carries; `user_library_access` (v1.0)
records a **grant** of read access to a *different* user. That is Plex's shape, it satisfies
principle 4, and it makes `sort_order` and ADR-0028's per-user media-type ordering per-user by
construction rather than by a second table. In v0.1 there is one user and every row is `user_id 0`.

**`include_on_home` is not a column here, and that is a deletion rather than an omission.** §R2.1
of the review log kept it from the libraries research, and under ADR-0028's three-fixed-block Home
nothing consumes it: Block A is per media *type*, Block C is unified across all types, and the
scope is driven by the `?lib=` chip. ADR-0028 separately pre-wires a per-type `show_on_home`
boolean, which is the flag that has a consumer. Two overlapping flags where one has no reader is
how a schema accumulates dead columns in the one migration that can never be edited.

**Reserved row: `library.id = 0`, *Unfiled*.** Inserted alongside the table, `user_id 0`,
`managed_by 'auto'`, `kind` irrelevant (the CHECK is satisfied with `'movie'` and the row is never
rendered). It is the PLACE §7 invariant 5 is upheld *in*: a work that the derivation would otherwise
place in **no** library is filed here instead, which keeps it visible in search to its owner and
nobody else. The invariant itself is upheld by the code that does the filing — see §7, where both
undeclarable invariants are stated with the code that owes them — not by the existence of this row.
It is never listed on the Libraries screen, never offered in the scope chip, and never
proposed. Reachable states that need it: a `root_folder`-scoped library covering only part of an
instance, and an `exclude` correction against a work's last remaining library.

**The row is protected by a `BEFORE DELETE` trigger,** because "reserved" was previously a comment
and nothing else — `DELETE FROM library WHERE id = 0` simply succeeded, nothing recreated the row,
and the derivation had nowhere to file an unfiled work for the life of that database. Two
consequences, both measured in `TestUnfiledLibraryIsProtected`: an **ordinary** user with rows in
every table that references `user(id)` is still deletable (the `audit_log` failure mode, re-run —
see §9), and `DELETE FROM user WHERE id = 0` now **fails**, deliberately, because the sentinel
user's cascade reaches this row.

⚠️ **Unfiled is `user_id = 0`, and `ux_library_slug` / `ux_library_name` are `(user_id, …)`, so no
other user has one.** Invariant 5's safety net does not exist for a second user. v0.1 is
single-user, so this is not a bug today; it is a seam, and [`../FUTURE.md`](../FUTURE.md) §19 says
what the multi-user migration must do about it.

### 13.2 `library_source`

```sql
CREATE TABLE library_source (
  id                  INTEGER PRIMARY KEY,
  library_id          INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
  service_instance_id INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE CASCADE,
  container_kind      TEXT NOT NULL CHECK (container_kind IN (
                        'instance','root_folder','remote_library','tag','series_type')),
  container_ref       TEXT NOT NULL DEFAULT '',
                      -- The container the UPSTREAM ITSELF reported, verbatim. Matched against
                      -- service_item_link per container_kind — see the predicate table below.
                      -- '' for container_kind='instance', which has no ref.
  container_identity  TEXT,          -- a stable property of the container (its name, or its own
                                     -- reported uuid) recorded at bind time, so an upstream that
                                     -- reuses ids does not silently rebind the library to a
                                     -- different folder. Same idea as remote_identity_hash.
  is_metadata_authority INTEGER NOT NULL DEFAULT 0,
  missing_since       TEXT,          -- the upstream stopped reporting this container; the source
                                     -- row is retained and shown, never silently dropped
  created_at          TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (library_id, service_instance_id, container_kind, container_ref)
) STRICT;

CREATE INDEX ix_libsrc_instance ON library_source(service_instance_id);
-- At most one metadata authority per library, enforced declaratively rather than in code:
CREATE UNIQUE INDEX ux_libsrc_authority ON library_source(library_id)
  WHERE is_metadata_authority = 1;
```

**Every `container_kind` now has storage on both sides of the predicate**, which was the defect:
three of the five values could not be derived from any column in the schema, and `remote_library`
is the *only* container available for the catalogue sources — Navidrome, Audiobookshelf and Kavita,
then Komga — which between them cover four of the six media types. Per ADR-0036 none of those ships
in v0.1; they arrive one at a time afterwards, in an order the ADR-0035 §2 watermark probe decides,
with Komga last. So this storage is exercised by the \*Arr sync first and by each catalogue source
as it lands.

| `container_kind` | `container_ref` holds | Membership predicate against `service_item_link` |
|---|---|---|
| `instance` | `''` | `service_instance_id = ?` |
| `root_folder` | the path the upstream reported | `root_folder_path` **prefix** match, on a value the upstream itself reported — the only path comparison UsArr ever performs (§6.5 rule 3) |
| `remote_library` | the upstream library id | `remote_library_id = ?` |
| `tag` | one \*Arr tag id | `EXISTS (SELECT 1 FROM json_each(remote_tag_ids) WHERE value = ?)` |
| `series_type` | e.g. `anime` | `remote_subtype = ?` |

⚠️ **The `tag` predicate is the one that is not a seek.** `json_each` over a per-row JSON array
cannot use an index, so a `tag`-scoped library re-derives by scanning the instance's links. That is
acceptable because it runs on the membership *derivation* path (a background 250 ms flush batch),
never on a render path — but it is the reason `tag` is the last container kind to be offered in the
UI, and it must not be copied onto a query path. Recorded rather than discovered.

### 13.3 `library_member`

```sql
CREATE TABLE library_member (
  library_id  INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
  sort_title  TEXT    NOT NULL,      -- DENORMALISED from work.sort_title. See below.
  work_id     INTEGER NOT NULL REFERENCES work(id) ON DELETE CASCADE,
  edition_id  INTEGER NOT NULL DEFAULT 0,
                                     -- 0 = "the whole work, format-independent". See below.
  added_at    TEXT    NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (library_id, sort_title, work_id, edition_id)
) STRICT, WITHOUT ROWID;

-- Identity, for upsert and for "is this work in this library":
CREATE UNIQUE INDEX ux_libmem_identity ON library_member(library_id, work_id, edition_id);
-- Reverse: "which libraries is this work in", for the item detail page and for the
-- search_doc_library rebuild.
CREATE INDEX ix_libmem_work ON library_member(work_id);
```

**Membership is edition-grained, and it has to be.** §17.8's flagship demonstration is
Audiobookshelf offered as **two** libraries — Ebooks and Audiobooks — over its one `mediaType=book`
library, which ABS itself cannot do because it distinguishes the two only at item level. But
`library.formats` filters `edition.format` while `library.kind` filters `work.kind`, and a `book`
work with an EPUB edition *and* an M4B edition is **one `work` row**. A `(library_id, work_id)` key
therefore cannot express the case at all: a work with only an audiobook edition is still a
legitimate `(Ebooks, work_id)` row unless the `formats` filter is re-evaluated against `edition` at
query time — which defeats the materialisation, and makes the row count on the Libraries screen and
the sidebar either wrong or dependent on the un-materialised join anyway. Keying
`(library_id, work_id, edition_id)` is what makes *"it costs one `formats` column"* true.

**`edition_id = 0` is a sentinel, not a foreign key, and here is why there is no FK.** A
`WITHOUT ROWID` table's primary-key columns are implicitly `NOT NULL`, so "no edition, the whole
work" cannot be `NULL`. `0` carries it, matching the sentinel pattern already used for `user_id` and
`service_instance_id`. The cost is that referential integrity for the non-zero case is maintained by
the derivation rather than by SQLite, so it is an assertion in `make check` instead
(`TestLibraryMemberEditionsBelongToTheirWork`):

```sql
-- must return no rows
SELECT m.library_id, m.work_id, m.edition_id FROM library_member m
 WHERE m.edition_id <> 0
   AND NOT EXISTS (SELECT 1 FROM edition e WHERE e.id = m.edition_id AND e.work_id = m.work_id);
```

Libraries whose `kind` has no format axis (`movie`, `series`) write `edition_id = 0` for every row.
A library with a non-null `formats` writes one row per matching edition.

**`sort_title` is denormalised into the key, and that is the fix for the one performance risk §6.5
flags.** The available index is `ix_work_kind_sort ON work(kind, sort_title, id)`, which §13 of
ARCHITECTURE correctly says *serves* rather than covers. §6.5's stated mitigation — *"a library is
single-kind and the default topology is one library per kind, so the common case is
`work.kind = ?` with membership as a one-row lookup"* — **is false in the design's own flagship
example**: the reference install in the mockups has seven libraries over six kinds, including two
`book` libraries (the ABS split this feature exists to demonstrate) and two `comic` libraries. For a
1%-selective library the keyset page must either seek `ix_work_kind_sort` and probe membership per
candidate (≈18 index rows scanned per row returned), or seek `library_member`, which without a sort
key must fetch and sort every member before the window can be cut. 🔍 *That is inference from the
standard shape rather than a measurement, which is exactly why §6.5 flagged it.*

With `sort_title` leading the key after `library_id`, the scoped keyset query is a single covered
seek regardless of selectivity:

```sql
EXPLAIN QUERY PLAN
SELECT m.work_id FROM library_member m
 WHERE m.library_id = ? AND (m.sort_title, m.work_id) > (?, ?)
 ORDER BY m.sort_title, m.work_id LIMIT 100;
--  MEASURED, SQLite 3.53.4:
--  → SEARCH m USING PRIMARY KEY (library_id=? AND (sort_title,work_id)>(?,?))
--  (this section previously wrote it as `(library_id=? AND sort_title>?)`; the row-value
--   comparison is reported in full. Same seek. TestLibraryScopedKeysetIsASeek asserts the
--   two halves separately so a leading-column-only scan still fails.)
```

**`make check` asserts that plan for both topologies — one library per kind *and* two libraries
over one kind — because only the second is the interesting one.** ⚠️ Both are built with real rows in
`TestLibraryScopedKeysetIsASeek` and **both produce the same plan**, which is recorded rather than
dressed up: `EXPLAIN QUERY PLAN` chooses from the schema, not the data, so with no `ANALYZE`
statistics the topology changes selectivity and not the plan. What the gate pins is that the seek is on
the primary key in both shapes; the row-count difference this paragraph is really worried about
belongs to `make bench`, which is never a merge gate. The write cost is one extra column on a
table that is already materialised and already flushed on the 250 ms batch, plus one rule: **a title
or sort-title change rewrites the member rows for that work**, since the sort key is now duplicated.
A `field` override on `sort_title` dirties them the same way. That is the price of the seek and it
is stated rather than assumed.

### 13.4 `library_override`

```sql
CREATE TABLE library_override (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL DEFAULT 0 REFERENCES user(id) ON DELETE CASCADE,
  verb        TEXT NOT NULL CHECK (verb IN ('exclude','include','relink','field')),
  -- SCOPE. NOT NULL for the two library-scoped verbs, NULL for the two global ones. See below.
  library_id  INTEGER REFERENCES library(id) ON DELETE CASCADE,
  -- TARGETS. Deliberately NOT foreign keys — see "why no FK" below.
  work_id     INTEGER,
  link_id     INTEGER,
  target_identity_hash TEXT NOT NULL,
  -- field verb only
  field_name  TEXT CHECK (field_name IS NULL
                          OR field_name IN ('title','sort_title','year','cover')),
              -- `IS NULL OR … IN (…)`, NOT `IN (NULL, …)`. This column carried the second
              -- instance of DB-01 until migration 0005: one NULL inside an IN list makes the
              -- comparison NULL for every non-matching value, and a CHECK passes on NULL, so
              -- the constraint accepted 'TOTAL-GARBAGE'. Pinned by
              -- TestNullableCheckConstraintsActuallyConstrain.
  field_value TEXT,
  -- relink verb only
  relink_to_work_id INTEGER,
  reason      TEXT,
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),

  -- exclude/include are library-scoped; relink/field are global. Enforced, not documented.
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
```

**Two of the four verbs are not library-scoped concepts, and the schema now says so rather than
implying the opposite.** `relink` changes which `work` a `service_item_link` points at — that is
UsArr's identity graph, global by construction. `field` overrides title, sort title, year or cover
on a `work`; if work W is in libraries A and B and the user renames it in A, either the override is
global or the same work renders under two names in two scopes, and nothing said which. ADR-0026's
own three-axis table puts *display identity* and *"is this link really this work"* in a row whose
owner is *"upstream by default, an explicit user correction"* — **with no mention of a library at
all**. So the axis table and the storage disagreed, and the `CHECK` above resolves it toward the
axis table: `library_id IS NULL` for both, and the effect is global.

The consequence is the one that matters for the delete-confirmation copy (§17.8): **deleting a
library cascades away its `exclude`/`include` rows and nothing else.** Corrections to identity and
display survive it.

**Why no foreign key on `work_id` or `link_id`.** §6.5 rule 1 is that *a correction is never
cleared by a sync, a reconciliation sweep, a tombstone expiry or an id resurrection* — the rule that
exists because LazyLibrarian's ignored books come back after an author rescan (⚠️ GitLab issue
#2407; see the caveat in §6.5). An `ON DELETE CASCADE` would let a tombstone expiry destroy the
correction, which is that failure reproduced; an `ON DELETE SET NULL` would orphan it silently.
`target_identity_hash` — the hash of the target's external identity at the moment the correction was
made — is the durable key, and `ix_ovr_hash` is how a resurrected or re-imported row re-attaches to
it. The columns are integers with no FK on purpose, and a nightly job that reports overrides whose
target has been absent for >90 days is the honest garbage-collection story, not a cascade.

**`field_name` cannot be `kind`.** Changing a work's kind is not a display correction — §6.4 states
that `work.kind` is derived from the library's declared kind and never inherited from a backend, so
the way to change it is to change the library's `kind` (§6.5 rule 4), which re-derives membership.
Allowing both would give two mechanisms for one fact.

### 13.5 The four tables' one assertion set

```sql
-- 1. Identity never reads library state (ADR-0026; see §7 invariant 4 for the two-path form).
-- 2. Every search_doc row is visible through ≥1 library (§7 invariant 5).
-- 3. Scoped keyset is a seek in both topologies (§13.3).
-- 4. edition_id ≠ 0 always resolves to an edition of the same work (§13.3).
-- 5. At most one metadata authority per library (declarative: ux_libsrc_authority).
-- 6. A library_override row with library_id IS NOT NULL has verb IN ('exclude','include')
--    (declarative: the CHECK) — asserted anyway, because it is the C-16 regression guard.
```

**Which of the six are live** is answered by `internal/db/queryplan_test.go` and
`internal/db/migrate_test.go`, not here. Assertions 2, 3, 4 and 5 have tests as of migration 0005
(`TestUnfiledLibraryExists`, `TestLibraryScopedKeysetIsASeek`,
`TestLibraryMemberEditionsBelongToTheirWork`, and `ux_libsrc_authority` declaratively). 1 and 6 wait
for the code they constrain — there is no identity cascade and no correction applier yet, and an
assertion over code that does not exist is a green nobody has exercised.

---

## Appendix — later tables

Present in the design, and **owned by the milestone named** — which is a claim about scope, not about
what has been created. ⚠️ This header used to pin the whole appendix to the absence of a single
migration (*"not in migration 0001"*), which goes wrong the moment any later migration creates one of
these tables; `sync_report` is the row that made it go wrong. **`internal/db/migrations` is the only
answer to whether a table exists**, and `internal/db/migrate_test.go`'s `TestDeferredTablesAreAbsent`
carries the present/absent lists in executable form.

| Table | Milestone | Note |
|---|---|---|
| `request`, `request_quota` | v0.2 | `request` carries `(kind, edition_format)`, not a flat kind |
| `work_merge` | v1.0 | Lands with identity cascade tiers 2–5; nothing in v0.1 merges |
| `tag_alias`, `tag_implies`, `tag_rule`, `saved_filter` | v1.0 | The full tag system |
| `role`, `role_permission`, `user_role`, `user_permission`, `user_library_access` | v1.0 | RBAC tables. The `user_id` columns and the access-scope parameter land in 0001; these do not |
| `playback_state`, `play_history` | v1.0 | Northbound write-back. **Both keys are edition-scoped — see below** |
| `playlist`, `playlist_item` | v1.0 | See the note below |
| `sync_report` | v0.1 | Emitted by the sweep; a plain append-only log. **DDL is now in §12.1** |

**`playback_state` is keyed `(user_id, work_id, edition_id)` and `play_history` is
`UNIQUE (user_id, work_id, edition_id, started_at)` — a correction ADR-0031 forced and nobody
propagated.** `FUTURE.md` §9 and §10 name the *work*-keyed forms as the seams that keep per-user
statistics and the cross-media "continue" row cheap. ADR-0031 made position edition-scoped, and
§6.1 makes an audiobook an `edition` of a `book` work. Combining the two: the EPUB and the M4B of
*Piranesi* are two editions of **one** work, so a `(user_id, work_id)` key cannot represent "40%
through the ebook, 12% through the audiobook" — **which is the entire content of the deferred
feature §10 describes**. The same collapse breaks §9's play-event uniqueness for two editions
consumed on the same day. `CLAUDE.md`'s rule is *the seam ships, the feature does not*; a seam that
cannot express the feature is not a seam. Eight bytes a row when the tables land, against a backfill
over the largest tables in the schema afterwards — which is ADR-0031's own argument applied
consistently. `edition_id` uses the same `0` sentinel as `library_member` for kinds with no edition
axis.

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
