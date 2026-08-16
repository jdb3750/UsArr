# Reference — Tag taxonomy

**Status:** partly implemented. `source:`, `type:`, `format:` and `indexer:` are derived from
Prowlarr search results today and carried on the API response; `quality:`, persistence into the
`tag` tables and any filtering by tag are designed, not implemented. **Scope:** derived system tags
(`type:`, `format:`, `source:`, `quality:`, `indexer:`) are **v0.1**; aliases, implications, the
rule engine, saved filters and \*Arr tag import are **v1.0**.
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §10. DDL: [`schema.md`](./schema.md) §8.

---

## 1. Three lessons from the reference implementations

- ***Arr tags are join keys for policy, not labels.** `TagDetailsResource` exposes
  `delayProfileIds`, `importListIds`, `notificationIds`, `restrictionIds`, `indexerIds`,
  `downloadClientIds`, `autoTagIds`, `seriesIds`. **Tags must be attachable to config objects, not
  only to media** — hence `tag_assignment.service_instance_id`.
- **Hydrus has the best tag model in the space:** namespaces (`character:`, `series:`, `creator:`),
  **siblings** (A means the same as B; the better one displays), and **virtual parents** (child
  implies parent — *"parents are virtual; the parent is not actually added to every file"*). Steal
  both.
- **Tags, genres and collections are different things.** Komga/Kavita and Jellyfin all keep them
  separate: genres come from providers (controlled vocabulary), tags are yours (arbitrary),
  collections are ordered and user-curated. Conflating them is a common and expensive mistake.

---

## 2. The vocabulary

Namespaced `namespace:value`, stored **decomposed into two indexed columns**, never as one string.
Reserved system namespaces are immutable to users because they are derived.

**System / derived (never user-editable):**

```
type:            movie | tv | season | episode | music | album | track |
                 book | comic | game                  ← provider + Newznab category
format:          print | ebook | audiobook | comic |
                 bluray | web | remux | dvd |
                 vinyl | cd | flac | lossy            ← edition.format
source:          usenet | torrent | irc | direct | manual | unknown   ← DownloadProtocol
indexer:         <slug>                               ← provenance.indexer_name
indexer-privacy: public | semi-private | private      ← Prowlarr IndexerPrivacy
client:          sabnzbd | nzbget | qbittorrent | deluge | transmission | rtorrent
quality:         2160p | 1080p | 720p | 480p | sd | lossless | lossy | mp3-320 | flac
media-source:    bluray | remux | web-dl | webrip | hdtv | dvd | cam
codec:           x265 | x264 | av1 | hevc | flac | aac | opus
hdr:             hdr10 | hdr10plus | dolby-vision | sdr
edition:         directors-cut | extended | theatrical | unrated | remastered | criterion | imax
lang:            en | ja | pt | …    ← ISO 639-1, from AUDIO TRACKS not the filename
status:          monitored | unmonitored | missing | upgradable | orphaned
provider:        tmdb | tvdb | musicbrainz | openlibrary | wikidata | manual
group:           <release group>
flag:            internal | exclusive | freeleech | neutralleech | halfleech | scene
                 | doubleupload                       ← Prowlarr IndexerFlag, TORRENTS ONLY
                 (+ indexer-specific: PassThePopcorn adds golden | approved)
```

> **`flag:` is not ours to invent — it mirrors Prowlarr's `ReleaseResource.indexerFlags` verbatim.**
> An earlier revision of this line listed `proper`, `repack` and `nuked`. **None of the three is a
> Prowlarr indexer flag**, and the invented names reached the UI mockups before anyone checked; see
> `docs/REVIEW-LOG.md`. Render only what the field can contain.
>
> **Where the vocabulary comes from.** The common set is the statics in
> `src/NzbDrone.Core/Indexers/IndexerFlag.cs` — seven of them, listed above. It is **not a closed
> set**: `IndexerFlag` is an ordinary class, and an indexer definition may subclass it to add its
> own. `PassThePopcornFlag` does exactly that, contributing `golden` and `approved` into the same
> array. So treat any value as possible, match the seven you know, and pass an unrecognised flag
> through as an opaque tag rather than dropping it.
>
> **Re-check it in one command**, against `develop`, and update this list with what you get:
>
> ```bash
> grep -rn "static IndexerFlag" src/          # in a Prowlarr checkout — 9 hits as of 2026-08-16
> ```
>
> Grep for `static IndexerFlag`, **not** for `new IndexerFlag(`: the file uses C# target-typed
> `new(...)`, so the latter pattern matches nothing and reads as "the set is empty" — a probe that
> tests the wrong thing and answers confidently. `DEVELOPMENT.md` §11 rule 1.
>
> ⚠️ **`indexerFlags` is populated for torrents only.** `ReleaseResourceMapper.ToResource` does
> `model as TorrentInfo ?? new TorrentInfo()`, so a usenet release takes the fallback and its flag
> array is **always empty**. An empty array on a usenet result therefore means *"this field does not
> apply to this protocol"*, not *"we checked and no flags are set"* — so a usenet release gets no
> `flag:` tags at all, and the absence must never be rendered as "none apply".

> **`type:audiobook` does not exist.** An audiobook is an `edition` of a `book` work, so it is
> `type:book` + `format:audiobook` (ARCHITECTURE §6.1). This is the tag-layer half of that decision;
> getting it wrong here reintroduces the contradiction the schema change was made to remove.

**User (free-form; users may mint arbitrary namespaces):**

```
user:        favorites | to-watch | rewatch | lent-out | …
collection:  halloween-2026 | dad-recommendations | …
rating:      1..5
<anything>:  <anything>
```

**Rules:** a bare tag with no colon lands in an implicit `tag:` namespace, so users never hit a
syntax wall; **`type:` and `format:` are single-valued per entity** (enforced by `tag.cardinality`),
most others are multi-valued; `is_system` tags render as chips you can filter by but not delete.

---

## 3. Source tagging, and the join that makes it work

Source tagging is **not inference**. `protocol` is a first-class enum
(`{"enum":["unknown","usenet","torrent"]}`, **byte-identical in the Prowlarr, Sonarr and Radarr
specs**) asserted by the indexer definition and carried through the pipeline. LazyLibrarian adds two
more via `listIRCProviders` / `listDirectProviders`.

The engineering work is the **join**, because the import event drops the provenance:

```
grabbed.data                    → Indexer, Protocol(int!), Guid, TorrentInfoHash,
                                  NzbInfoUrl, ReleaseGroup, DownloadClient, Size,
                                  PublishedDate, IndexerFlags, ReleaseType, …
downloadFolderImported.data     → FileId, DroppedPath, ImportedPath, DownloadClient,
                                  ReleaseGroup, CustomFormatScore, Size, IndexerFlags
                                  ── NO Indexer. NO Protocol. NO Guid. NO InfoHash. ──
```

⇒ **The provenance join key is `downloadId`.** Walk history, pair `grabbed` ↔
`downloadFolderImported` on `downloadId`, stamp the resulting file. **Fallback** when the grab
record is gone: `DownloadClient` in the import event is the client *implementation type*
(`Sabnzbd`, `NzbGet`, `QBittorrent`, `Deluge`, `Transmission`, `RTorrent`), and the implementation
type determines protocol unambiguously — recorded with `provenance.confidence ≈ 0.6` rather than
1.0.

**Media type from categories.** The Newznab/Torznab parent category is an independent,
always-present media-type signal: `floor(cat/1000)*1000` →
`{2000: movie, 5000: tv (5070→anime), 3000: music (3030→audiobook), 7000: book (7020→ebook,
7030→comic, 7010→magazine), 6000: adult, 1000/4000: game/software, 0/8000: other}`. Categories
≥ 100000 are site-specific and need the indexer's `t=caps`; fall back to the parent cat Prowlarr
also emits. **Category `3030` is the only reliable machine signal separating audiobook from music at
acquisition time, and `7030` likewise for comics** — which is exactly the `type:book` +
`format:audiobook` pair. Capture the raw array; never collapse it.

**Three categories in that map have no `type:` value, and the mapper must not invent one.** The
vocabulary in §2 is `movie | tv | season | episode | music | album | track | book | comic | game`.
The table above names media types that are not in it:

| Category | Named above as | Resolution |
|---|---|---|
| `5070` | anime | **Decided: `type:tv` + `genre:anime`.** §7 already states this — anime is a genre, not a media type, and reinterpreting the user's data as a separate type is explicitly rejected there. `5070` therefore sets `type:tv`, and the genre tag comes from the provider, not from the category. |
| `7010` | magazine | **Not decided.** No `type:` and no `format:` value exists. The consistent shape, following the `type:book` + `format:audiobook` precedent, is `type:book` + a new `format:magazine`; that is a recommendation, not a decision. |
| `6000` | adult | **Not decided.** No `type:` value exists, and the choice interacts with whether adult content is filtered or merely typed. |

Until `7010` and `6000` are decided, the mapper assigns **no `type:` tag** for them rather than
guessing. That is the honest degradation: an untyped item is visibly untyped, whereas laundering a
magazine into `type:book` produces a library that is quietly wrong and cannot be corrected later
without knowing which rows were guesses.

---

## 4. Virtual parents, aliases, inheritance

`tag_implies` is resolved **at query time**, never materialised: `quality:flac` implies
`quality:lossless`; `media-source:remux` implies `media-source:bluray`. Materialising these would
double-write every ingest and make changing an implication a full re-tag.

`tag_alias` handles siblings: `type:movie` ← `type:film`, `type:tv` ← `type:series`. The canonical
wins on display; both resolve on query.

**Inheritance across Series → Season → Episode, Artist → Album → Track, Book series → Book →
Edition.** Do **not** materialize inherited tags onto children — store at the level applied and walk
*up* `work.parent_work_id` with a recursive CTE. At home-library scale the CTE is fine.

The `tag.inheritable` flag controls direction, and getting it wrong is a real bug:

- `user:favorites`, `collection:*` — **inherit downward.** Favouriting a series should favourite its
  episodes for filter purposes.
- `source:*`, `quality:*`, `codec:*`, `hdr:*`, `group:*` — **per-file, must NOT inherit downward.**
  "This series is `source:usenet`" is meaningless when episode 4 came from a torrent.
- `type:*`, `format:*` — **inherit downward** and are also independently derivable. Keep both and
  assert they agree; a mismatch is a sync bug.

Some tags **roll up** ("this series has any 4K file"). Model those as **computed saved filters, not
stored tags**.

---

## 5. Smart vs static

- **Static tag** — a `tag_assignment` row with `source='user'`. Persists regardless of state.
- **Rule tag** — `source='rule'`, re-evaluated on ingest and on rule change.
  `tag_rule.remove_automatically` mirrors \*Arr's flag, so a rule that stops matching un-tags.
  Auditable via `rule_id`. **This is why the extra unique indexes in schema.md §8 matter:** the
  natural implementation of "re-evaluate on ingest" is an insert, and without uniqueness on
  edition/file/instance targets the same tag lands unboundedly.
- **Smart filter** — **never a tag.** A `saved_filter` row that *looks* like a tag in the sidebar.

> **Rule of thumb: if the answer changes when the underlying data changes, it is a filter, not a
> tag.** This prevents the classic bug where a "smart tag" goes stale and users cannot tell why an
> item is in a collection.

The rule engine copies \*Arr's `AutoTaggingSpecification` shape almost verbatim —
`{implementation, fields, negate, required}` plus `removeTagsAutomatically` — because it already
exists, users already understand it, and `negate` + `required` is exactly the expressiveness needed
without becoming a query language.

---

## 6. Fast filtering

The canonical predicate is **`WHERE tag_id = ? AND user_id IN (0, :uid)`** against
`ix_ta_tag(tag_id, user_id, work_id)`.

The earlier design claimed `(tag_id, work_id)` turns "all items with `source:usenet`" into an
index-only scan. Measured, that is true only for a query that **ignores user scope**:

```
-- user-scoped, as the design rule mandates:
SELECT work_id FROM tag_assignment WHERE tag_id=? AND (user_id IS NULL OR user_id=?)
  -> SEARCH tag_assignment USING INDEX ix_ta_tag (tag_id=?)          [NOT covering]

-- ignoring user scope:
SELECT work_id FROM tag_assignment WHERE tag_id=?
  -> SEARCH tag_assignment USING COVERING INDEX ix_ta_tag (tag_id=?)
```

The covering property is lost the moment `user_id` enters the predicate — one row lookup per
matching assignment, which for a system tag on 400k items is 400k random reads, and the
selectivity-ordering optimisation makes it *worse* for the common tag, which is the one users filter
on. Hence `user_id` inside the index, `NOT NULL DEFAULT 0`, with `0` as the shared sentinel:
`NULL` is not usable as an indexed equality.

Other rules:

- **AND of N tags:** either `INTERSECT` of N index scans, or
  `GROUP BY work_id HAVING COUNT(DISTINCT tag_id) = N`. The former is usually faster with SQLite's
  planner; **benchmark both** rather than assuming.
- **Selectivity ordering is the highest-leverage optimisation.** Start from the rarest tag;
  `tag.item_count` is maintained denormalised so the query builder can order the intersection
  cheapest-first without a count query.
- **`ANALYZE` after bulk import** — the planner is materially better with stats for multi-index
  intersections.
- **Keep `tag_assignment` narrow.** It is the hot path; do not put timestamps in the hot index.
- Add the chosen plan to the CI `EXPLAIN QUERY PLAN` assertions, so a future index change that
  silently drops the intersection is caught.

---

## 7. Importing \*Arr tags (v1.0)

\*Arr tags are **integer-keyed, per-instance labels with no namespace**; two instances' tag `3` are
unrelated. Import them into a dedicated namespace to avoid collisions and keep round-tripping safe:

```
sonarr-tag:<label>   radarr-tag:<label>   lidarr-tag:<label>   prowlarr-tag:<label>
```

Offer a one-click **"promote to UsArr tag"** that creates a `tag_alias` from `sonarr-tag:anime` →
`type:tv` + `genre:anime`, rather than silently reinterpreting the user's data. If UsArr ever writes
tags *back* to an \*Arr, **only write from the `sonarr-tag:` namespace** — never push a namespaced
UsArr tag into a flat-label system.
