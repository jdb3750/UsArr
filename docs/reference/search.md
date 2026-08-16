# Reference — Search

**Status:** designed, not implemented. **Scope:** tiers 1 and 2 are **v0.1**. There is no tier 3.
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §8.

---

## 1. What the two tiers do, and what they do not

| Tier | Where | Latency | Handles |
|---|---|---|---|
| 1 | Client, IndexedDB | 0 ms | Prefix match over top-level titles. ~80% of real queries. |
| 2 | Server, FTS5 | p50 < 15 ms | Prefix + substring match, cross-entity ranking, diversity injection. |

**Neither tier does typo tolerance, and the design must stop claiming it does.** FTS5's `trigram`
tokenizer is a **substring** matcher: `MATCH 'dremas'` finds rows whose text literally contains the
byte sequence `dremas`. A transposition destroys the match. Executed against the schema:

```
search_fts   MATCH 'train dremas'   -> 0 rows
search_trgm  MATCH 'train dremas'   -> 0 rows
search_fts   MATCH 'train* dremas*' -> 0 rows
search_trgm  MATCH 'dremas'         -> 0 rows
search_fts   MATCH 'dreams'         -> 1 row     (control)
```

The Go re-rank in §4 only reorders candidates that were **already retrieved**, so Jaro-Winkler
cannot rescue a candidate set that never contained the item.

**The candidate fix, recorded and not designed:** a fourth retrieval leg over a title vocabulary —
SQLite's `spellfix1` / `editdist3`, or a Go-side symmetric-delete or BK-tree index over
`search_doc.norm_title` — fused into the RRF alongside the other two. Its costs, stated so a later
milestone can decide honestly:

- `spellfix1` is a **loadable extension**. Whether it is available under `ncruces/go-sqlite3`
  (which compiles upstream SQLite and translates it to Go) is ⚠️ unverified and is the first thing
  to check.
- It needs its own vocabulary table, populated from `search_doc.norm_title` and refreshed whenever
  titles change — a third write on the ingest hot path.
- A Go-side BK-tree avoids the extension question but holds the vocabulary in memory, which
  interacts with the RSS budget.
- **Its recall bound must be measured, not assumed.** Edit distance 1 on short titles is cheap and
  useful; distance 2 on long ones expands the candidate set fast.

Until then the UI says what is true: prefix and substring matching, no typo correction, and the
zero-results screen says so rather than leaving the user guessing.

---

## 2. The corpus

`search_doc` indexes **top-level works only**:

```
movie, series, artist, album, book, comic, game
```

`season`, `episode`, `track`, **`comic_issue`** and **`person`** are **excluded**. The reference library is ~400k episode rows
against ~13k top-level works; indexing episode titles means a corpus of "Pilot", "Part One" and
"The Beginning" swamps every query, and the `title_idf` penalty was tuned for short *movie* titles,
not for that. Episodes and tracks are reachable by **scoped search from within a parent's detail
view**, which queries `work` by `parent_work_id` directly and never touches FTS.

A large manga library does to the corpus with chapter titles exactly what episode titles do
(ADR-0030), which is why `comic_issue` is a kind at all.

`person` (ADR-0033) is excluded for a different reason from the other four: it is not a volume
problem, it is a destination problem — there is no person screen in any milestone, so a person hit
would be a result row with nowhere to go. ⚠️ **The consequence is that "find everything by this
author" is unanswered in v0.1.** The cheap candidate is to fold credited names into the `alt_titles`
column of the works they are credited on, so the query returns the books rather than the person, but
that is a decision for whoever writes the document builder and it is not specified here. Adding
`person` to the corpus later is a predicate change plus a re-index — not a migration, which is why
it can wait and the kind itself could not.

CI asserts
`SELECT COUNT(*) FROM search_doc WHERE kind IN ('season','episode','track','comic_issue','person')`
is 0.

**Permission filtering happens in the join, not after it**, and **the mechanism is a junction table,
`search_doc_library(library_id, doc_rowid)` `PRIMARY KEY (library_id, doc_rowid) WITHOUT ROWID`** —
not a `library_scope` column on `search_doc`. It replaces `instance_scope` (ADR-0026: a library can
be a *subset* of an instance, so instance-level scoping is too coarse and leaks existence), and the
junction rather than a column is the load-bearing part: **a JSON array in a `TEXT` column cannot
participate in an index join.** Filtering it needs `json_each()` or a `LIKE`, both of which are
scans, so the column bought a full scan of the fused candidate set in service of the requirement
this paragraph states. The scoped fusion query joins against the caller's access scope
(ARCHITECTURE §1.3 rule 2):

```sql
… JOIN search_doc_library sdl ON sdl.doc_rowid = sd.rowid AND sdl.library_id IN (:scope…)
```

a covered index seek per scoped library. **CI asserts `SEARCH sdl USING PRIMARY KEY` and the absence
of `SCAN search_doc_library`**, so it cannot silently regress. Post-filtering FTS hits silently
breaks keyset page sizes and leaks existence through result counts and ranking positions.

⚠️ **A row visible through no library matches no scope and disappears from search for every user
including the owner** — a state the old `instance_scope` could not reach, because every replicated
row came from some instance. Reserved `library.id = 0` (*Unfiled*) upholds the invariant that every
`search_doc` row has at least one `search_doc_library` row, and CI asserts that too. 🔍 That the
library scope can fully replace the instance scope with no second column is **inference**, argued in
ADR-0026 rather than measured, and it should be checked against the first real scoped query written.

---

## 3. Query construction — the exact transformation

Every user-visible search behaviour depends on this, and leaving it to the reader is how a project
gets a 500 on every apostrophe.

```
raw query  →  tokens  →  q_prefix (for search_fts)  and  q_raw (for search_trgm)
```

**Step 1 — tokenise.** Split on Unicode whitespace. Drop empty tokens. Cap at 12 tokens (a longer
query is pathological and is truncated with a UI note).

**Step 2 — escape every token.** Wrap each token in double quotes and double any internal double
quote. This is what makes FTS5 treat it as a literal string rather than as syntax:

```go
func ftsQuote(tok string) string {
    return `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
}
```

Without it, a user typing `"`, `*`, `:`, `-`, `^`, `NEAR`, `AND` or `OR` either gets an FTS5 syntax
error or an unintended operator. `Schindler's List` is safe either way (a single quote is not FTS5
syntax) but `AC/DC` and `Fallout: New Vegas` are not.

**Step 3 — the boolean operator is `OR`, not FTS5's default `AND`.** This choice decides whether
tier 2 works at all. With `AND`, a two-token query where one token is misspelled or is a word the
title does not contain returns nothing, and the re-rank has nothing to reorder. With `OR`, the
query retrieves everything containing *any* token and the re-rank does the discrimination — which
is what the Jaro-Winkler, `title_idf` and `in_library` signals exist for. The cost is a larger
candidate set, which is bounded by the `LIMIT 200` on each leg.

**Step 4 — prefix the last token only.** `train dr` → `"train" OR "dr"*`. Prefixing *every* token
(`"train"* OR "dr"*`) inflates the candidate set for no benefit: the user has finished typing the
earlier tokens. The `prefix='2 3 4'` index makes 2–4 character prefixes free; a 1-character final
token is dropped from the prefix leg.

**Step 5 — the trigram leg has a floor.** SQLite's trigram tokenizer requires **≥3 characters**;
a query whose longest token is shorter than 3 characters skips the `search_trgm` leg entirely and
the RRF degenerates to the single keyword leg. Do not send it a 2-character query and expect
results.

### Worked examples

| Raw input | `q_prefix` (search_fts) | `q_raw` (search_trgm) | Notes |
|---|---|---|---|
| `severance` | `"severance"*` | `"severance"` | Single token, prefixed |
| `train dreams` | `"train" OR "dreams"*` | `"train dreams"` | Last token prefixed |
| `train dr` | `"train" OR "dr"*` | *(skipped)* | Longest token 5 ≥ 3 → trigram runs on `"train dr"`; shown skipped only if every token < 3 |
| `it` | `"it"*` | *(skipped)* | 2 chars: trigram floor. `title_idf` will bury common short titles anyway. |
| `AC/DC` | `"AC/DC"*` | `"AC/DC"` | The slash is inside quotes and is inert |
| `Fallout: New Vegas` | `"Fallout:" OR "New" OR "Vegas"*` | `"Fallout: New Vegas"` | The colon is quoted, so it is not a column filter |
| `say "hello"` | `"say" OR """hello"""*` | `"say ""hello"""` | Internal quotes doubled |

---

## 4. The fusion query and the re-rank

```sql
-- Reciprocal Rank Fusion, k = 60 (conventional).
-- RRF deliberately does NOT compare the two engines' scores: BM25 over a unicode61 index and
-- BM25 over a trigram index are not on a common scale. It uses them only for row_number()
-- ranking within each set.
WITH kw AS (
  SELECT rowid, ROW_NUMBER() OVER (ORDER BY bm25(search_fts)) AS rnk
  FROM search_fts WHERE search_fts MATCH :q_prefix LIMIT 200
),
tg AS (
  SELECT rowid, ROW_NUMBER() OVER (ORDER BY bm25(search_trgm)) AS rnk
  FROM search_trgm WHERE search_trgm MATCH :q_raw LIMIT 200
)
SELECT COALESCE(kw.rowid, tg.rowid) AS rowid,
       COALESCE(1.0/(60 + kw.rnk), 0) + COALESCE(1.0/(60 + tg.rnk), 0) AS rrf
FROM kw FULL OUTER JOIN tg USING (rowid)
ORDER BY rrf DESC LIMIT 200;
```

> **The fusion is only correct because all three tables share one rowid space**, and that is an
> invariant the schema cannot enforce: `search_fts.rowid == search_trgm.rowid ==
> search_doc.rowid`, allocated by inserting into `search_doc` (an `INTEGER PRIMARY KEY` table)
> first and then inserting **explicitly** into both FTS tables in the same transaction. One missed
> explicit rowid silently fuses unrelated documents and produces plausible-looking wrong results —
> the hardest class of bug to diagnose. CI asserts the row counts match.

Then re-rank the ≤200 candidates in Go — sub-millisecond for 200 short strings:

| Signal | Weight | Why |
|---|---|---|
| **Jaro-Winkler** on `norm_title` | primary | Prefix-weighted, which matches how people type titles: they get the beginning right. |
| **`popularity` prior** | high for short queries | For a 3-character query, popularity beats text score. |
| **`in_library` boost** | high | Items you own outrank items you don't. The single most user-satisfying signal, and everyone forgets it. |
| **`title_idf` penalty** | negative | Penalise short high-frequency titles ("It", "Her", "Us") hard, or they swamp everything. |
| **Recency of `added_at`** | small | Mild tiebreak. |

**Then: media-type diversity injection.** After ranking, guarantee the top 10 contains at least one
item per media type that scored above a floor. **This is what makes the Train Dreams case work** —
without it, whichever medium has better text statistics sweeps the list and the novella never
appears, which is precisely the failure the feature exists to prevent.

**Ranking and grouping are different layers, and both are needed.** Diversity injection operates on
*ranking* (both the film and the novella get retrieved and scored); **grouping** operates on
*presentation* (linked works render as one card with per-medium availability). The grouped card is
derived at query time from `work_relation` edges above the confidence threshold — **not** a stored
group table, because a stored group must be invalidated on every edge change and the connected
components at this scale are tiny.

---

## 5. Tier 1 — the client-side index

Contents, per ARCHITECTURE §4.5: **top-level works only**, fields `{id, title, year, kind,
availability_state}` in a columnar payload, with ThumbHashes as raw bytes in a side `ArrayBuffer`
(~25 B each, versus ~34 characters if base64'd into JSON). `sort_title` is **not** shipped — it is a
near-duplicate of `title` and the client can sort on a locale-aware key derived at load.

Sizing, honestly: at 110–160 B/item a 13k-item top-level library is **~1.5–2.1 MB**. The earlier
"~80–120 bytes/item → ~1.2 MB for 10k items" figure was both optimistic per item and computed
against a library 40× smaller than the reference one, because it counted only top-level works while
describing the index as covering "the whole library" — which for the reference library is ~412k
`work` rows and 33–50 MB.

**Hard cap: 25,000 items.** Above it the server does not build the index, the client does not
download it, tier 1 falls back to a debounced server search, and the UI states this in the search
affordance rather than silently changing behaviour.

The index is built **per access scope**, ETagged by `(user_id, access_scope_version)`, and served
from a single endpoint. In v0.1 there is one scope (the owner's) and the payload is the whole
top-level library — that is the degenerate case of the general rule, not the design.

---

## 6. Release search — a different thing with different rules

Catalogue search is local. **Release search** ("which indexer actually has this") is remote, and the
numbers are the reason it is never on a page-load path: Prowlarr users report waits **over 30
seconds** for disabled or down indexers to time out, and FlareSolverr's default timeout is **60 s**
(range 1–180 s), so one Cloudflare-protected indexer can hold a naive implementation for a minute.
Prowlarr has stated that **an aggregate multi-indexer endpoint will not be added** — per-indexer
fan-out with client-side merging is UsArr's job.

- **Behind progressive disclosure** — a user action, never a page load.
- **Stream partial results over SSE** as each indexer answers; never wait for the full fan-out.
- **Per-indexer deadlines** in single-digit seconds, with independent circuit breakers.
- **Skip known-down indexers entirely** rather than re-timing-out on them every search.
- **Rank progressively** as results arrive.
- Respect `capabilities.limitsMax` / `limitsDefault` from the indexer's `t=caps`, and check
  `supportsPagination` before sending an `offset` at all.
- Use `IndexerCapabilityResource.{tv,movie,music,book}SearchParams` to choose *how* to query: if an
  indexer advertises `imdbId` support, query by id rather than by title.

> 🚩 **Prowlarr's grab cache is 30 minutes.** `SearchController.MapReleases()` rewrites download
> URLs to Prowlarr proxy links and caches the original `ReleaseInfo` in memory keyed
> `"{indexerId}_{guid}"` for `TimeSpan.FromMinutes(30)`. **POST the release back within 30 minutes
> of the search or the grab fails.** `release_candidate.expires_at` is 25 minutes for
> Prowlarr-sourced candidates, `raw_release_json` stores the `ReleaseResource` verbatim so the grab
> can echo it back, and an expired candidate is never rendered as grabbable.

**Search-and-Grab mode** (ARCHITECTURE §8.5) is this machinery with no library behind it. Verified
endpoints:

| Operation | Call | Returns |
|---|---|---|
| Free-text search | `GET /api/v1/search?query=&type=&indexerIds=&categories=&limit=&offset=` | `ReleaseResource[]`, `application/json` |
| Grab | `POST /api/v1/search` with a `ReleaseResource` body | `ReleaseResource` |

`ReleaseResource` carries `protocol` (`DownloadProtocol`), `categories` (`IndexerCategory[]`),
`indexerFlags` (`string[]` on Prowlarr), `indexer`, `indexerId`, `seeders`, `leechers`, `size`,
`age`, `downloadUrl`, `infoUrl`, `infoHash`, `magnetUrl` and **`downloadClientId`** — which is what
lets a grab name one of Prowlarr's own configured download clients, so UsArr needs no
download-client integration to complete the flow.

> 🚩 **A grab can partially succeed, and the single-release endpoint cannot tell you so.** Prowlarr
> adds the release to the download client *first* and applies configuration *second*, with no
> rollback: `Deluge.AddFromMagnetLink` calls `_proxy.AddTorrentFromMagnet`, then
> `SetTorrentSeedingConfiguration`, then `SetTorrentLabel`. A throw in that tail — Deluge's Label
> plugin rejects a label that does not exist — leaves the torrent **running in the client** and
> raises `DownloadClientException`. `DownloadService.SendReportToClient` catches only
> `ReleaseUnavailableException`, `DownloadClientRejectedReleaseException` and
> `ReleaseDownloadException`, so that one propagates uncaught and `SearchController.GrabRelease`
> returns a bare **500**. **A 200 is the only success signal Prowlarr offers; a 500 does not mean
> nothing was added.**
>
> **The same failure behaves differently on the bulk endpoint.** `GrabReleases` wraps each release
> in its own `try`/`catch`, catches `DownloadClientException` as well, logs it and `continue`s —
> Prowlarr's own bulk path treats "added, then configuration failed" as non-fatal. The single-release
> endpoint UsArr uses turns the identical condition into a 500.
>
> **Do not try to discriminate by parsing the error body.** The only signal is a frame in the .NET
> stack trace in `description`: that is another project's private internals, it is client-specific
> (qBittorrent, Transmission and rTorrent each fail differently), and it is partly gettext-translated
> so it breaks against a non-English daemon. The API does not carry the information.
>
> **Ordering that UsArr does rely on, verified in `SearchController.GrabRelease`:** the
> `_remoteReleaseCache.Find` lookup and its 404 come *before* `SendReportToClient`, and the
> `DownloadClientUnavailableException` for an unconfigured client is that method's *first* statement.
> So a 404 and a "download client isn't configured yet" 500 both provably mean **nothing was
> dispatched** — which is what makes the transparent re-search on a cache miss safe from
> double-sending (`internal/releases/grab.go`, `grabWithReSearch`). 409 is
> `ReleaseDownloadException`, raised fetching the release from the *indexer*, so it too means nothing
> reached the client.
>
> **How common is it, and can a health check see it coming?** Neither answer is comfortable:
> Prowlarr's Deluge form ships with Default Category pre-filled, and its own Test button goes green
> in the state that breaks the grab. `arr-apis.md` §7 has both, with the upstream source for each.
