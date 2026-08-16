# Reference — Cross-media linking

**Status:** designed, not implemented. **Scope:** **v0.3.**
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §9.

---

## 1. Why Wikidata

Wikidata is the only free, CC0, queryable source carrying **both** the book→film adaptation edge
(`P144 based on`) **and** the external IDs of every downstream provider (`P345` IMDb, `P4947` TMDB
movie, `P648` Open Library, `P434`/`P436` MusicBrainz). Nothing else does both, and nothing else has
genuinely unencumbered terms: **CC0 — no attribution obligation, no commercial restriction, no cache
limit.** Contrast TMDB: mandatory attribution, a **6-month cache limit**, and a
commercial/non-commercial split.

---

## 2. The Train Dreams worked example, with verified IDs

Searching Wikidata for `Train Dreams` returns **five** items — a perfect illustration of the
disambiguation problem:

| QID | Label | Description |
|---|---|---|
| `Q85810391` | Train Dreams | 2011 novella by Denis Johnson |
| `Q126086662` | Train Dreams | 2025 film directed by Clint Bentley |
| `Q137480492` | Train Dreams | 2025 film score by Bryce Dessner |
| `Q137993803` | Train Dreams | Song from the 2025 film |
| `Q131428569` | Train Dreams | **Wikimedia disambiguation page** |

**Disambiguation pages are items too and must be filtered** (`P31 = Q4167410`).

**The novella `Q85810391`:** `P31` = `Q7725634` (literary work), `P50` author = `Q340016` (Denis
Johnson), `P577` = 2011-08-30, `P7937` = `Q149537` (novella), **`P648` Open Library =
`OL15916948W`**, `P8383` Goodreads work = `2046778`, `P1411` nominated for the Pulitzer
(`Q833633`). It carries **no `P4969`**.

**The film `Q126086662`:** `P31` = `Q11424` (film), **`P144` based on = `Q85810391`** ← the
adaptation edge, **`P345` IMDb = `tt29768334`**, **`P4947` TMDB = `1241983`**, `P57` director =
`Q131863949` (Clint Bentley), `P2047` duration = 102 min.

**Open Library work `OL15916948W`** has **9 editions**, including `OL34486604M`
(ISBN-13 `9781847086624`, 2013 Granta), `OL24823347M` (ISBN-13 `9780374281144`, the 2011 Farrar,
Straus and Giroux edition) and — critically — `OL62068093M`, ***Sonhos e Comboios***, ISBN-13
`9789896412913`, the Portuguese translation. **Pure title matching would never find that.** Only the
OL Work→Edition edge does. That is the argument for the `edition` layer in one example.

> **A note on a correction.** An earlier draft said an alternative research track's identifiers
> "are wrong", listing TMDB `1156593`, IMDb `tt13444824`, OL work `OL15170152W` and ISBN
> `9780374281144`. The first three are indeed the wrong work. **ISBN-13 `9780374281144` is not
> wrong** — it is edition `OL24823347M` of `OL15916948W`, verified live against
> `openlibrary.org/works/OL15916948W/editions.json`. It is an **edition-level** identifier for the
> correct work, not an error, and the distinction is exactly what the edition layer exists to make.

### The structural finding that shapes the implementation

The novella carries **no `P4969`** statement pointing at the film. **The adaptation edge exists only
in the film → book direction.** Any implementation that starts from a book and reads its statements
finds nothing. You must query the **inverse**:

```sparql
SELECT ?adaptation ?typeLabel ?imdb ?tmdb ?year WHERE {
  ?adaptation wdt:P144 wd:Q85810391 .
  OPTIONAL { ?adaptation wdt:P31   ?type }
  OPTIONAL { ?adaptation wdt:P345  ?imdb }
  OPTIONAL { ?adaptation wdt:P4947 ?tmdb }
  OPTIONAL { ?adaptation wdt:P577  ?year }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}
-- verified live: → Q126086662, "Train Dreams", film, tt29768334, 1241983, 2025
```

**That inverse query is the core primitive of UsArr's cross-media search.** Everything else is
caching, fallback and UI.

The two directions UsArr executes:

```
A. Have the film (Radarr), want the book:
   tmdbId 1241983 → QID via haswbstatement:P4947=1241983 → Q126086662
   → P144 → Q85810391 → P648 → OL15916948W → editions.json → 9 editions, ISBNs

B. Have the book (an EPUB, ISBN 9781847086624), want the film:
   ISBN → OL edition → OL work OL15916948W → QID via haswbstatement:P648=… → Q85810391
   → INVERSE: ?x wdt:P144 wd:Q85810391 → Q126086662 → P4947 → 1241983 → Radarr
```

---

## 3. Coverage, measured

From live SPARQL `COUNT(*)` queries against `query.wikidata.org`:

| Query | Count |
|---|---|
| Films (`P31=Q11424`) with a TMDB ID (`P4947`) | 241,618 |
| Films with a `based on` (`P144`) statement | **15,360** |
| Films with **both** TMDB ID and `P144` | 14,443 (**94%** of adaptations are TMDB-resolvable) |
| Films with `P144` whose source work has an Open Library ID | 6,134 |
| TV series (`P31=Q5398426`) with `P144` | 5,314 |
| All `P4969` derivative-work statements, any type | 34,673 |
| Items with an Open Library ID (`P648`) | 508,194 |
| Items with a MusicBrainz release-group ID (`P436`) | 243,634 |
| Items with an IMDb ID (`P345`) | 1,190,531 |

**Interpretation:** the adaptation graph is **small and clean** — ~15k film adaptations, ~5k TV,
~35k derivative-work edges total. 94% of `P144` films carry a TMDB ID, so once you have the edge you
can nearly always land it in Radarr. And coverage is **excellent for famous works, poor for the long
tail** (~6.1k book→film pairs with an OL ID on the book side), which resolves *Dune*, *No Country
for Old Men* and *Train Dreams* and misses most mid-list adaptations.

**That last fact was previously used to argue that a fuzzy tier and a human review UI "are not
optional". The conclusion has been reversed** — see §5. The measurement is unchanged; what changed
is the judgement about what to do with the gap.

---

## 4. The artifact

> **`wikidata-edges.db` contains edges only:** `(from_qid, to_qid, rel_type, evidence)` plus the
> external-id columns needed to resolve each side (`P345`, `P4947`, `P648`, `P436`). No labels, no
> descriptions — labels resolve from the local `work` row. **Expected size: single-digit MB.**

**It is generated by `tools/build-wikidata-edges`, a committed script** running a fixed set of
paginated SPARQL queries against `query.wikidata.org`, and **regenerated per release, not weekly.**
Nothing in UsArr degrades if it is months stale; the adaptation graph changes slowly.

This replaces an earlier design that committed the project to ingesting Wikidata **full dumps**
(tens of GB) weekly, forever, unpaid, as a one-person project — an ongoing operational commitment
costed nowhere, which quietly rots the feature if it lapses. It is also unnecessary: the §3 counts
are a few tens of thousands of rows, retrievable by paginated SPARQL in minutes. The earlier text
also quoted the artifact's size three incompatible ways ("a few MB", "a few hundred MB", "fits in
the release artifact"); a few hundred MB does not fit in a < 40 MB container image and is not a
release asset a self-hoster wants pulled on every upgrade. **One number now: single-digit MB.**

**Redistribution is legal** — all Wikidata structured data is CC0.

**Operational notes, learned the hard way:**

- `www.wikidata.org/w/api.php` **rate-limits within a handful of sequential requests** (*"You are
  making too many requests to the API"*). **Do not build anything on `wbgetentities`.**
- `https://www.wikidata.org/wiki/Special:EntityData/{QID}.json` served every request reliably and is
  CDN-cached. **Use it for single-entity fetches.**
- `https://query.wikidata.org/sparql` (GET, `Accept: application/sparql-results+json`, descriptive
  UA) answered every query including aggregate counts over hundreds of thousands of triples. 60 s
  timeout.
- ⚠️ The **May 2025 WDQS graph split** moved scholarly articles to a separate endpoint. Irrelevant
  for media, relevant if UsArr ever federates.

**Subset filter for the build script:** items with `P31` in {film, TV series, literary work, book,
album, video game, comic, manga, anime} **or** carrying any of {`P345`, `P4947`, `P4983`, `P648`,
`P434`, `P436`, `P8383`} — restricted to those that participate in a `P144`/`P4969` edge. Live
SPARQL remains a **cache-miss path, not a dependency.**

---

## 5. The ladder — three tiers, and nothing below 0.85

```
LINK(work W):

── Tier 0: exact identity (1.00, auto-confirm) ────────────────────────────
   For each external id on W, look for another work with the same (source, value).
   Same work → MERGE (schema.md §4), not link.

── Tier 1: Wikidata edges (0.95–0.99, auto-confirm) ───────────────────────
   1a. Resolve W → QID:
         if W has tmdb/imdb/olid/mbid → haswbstatement:P<prop>=<value>  (one HTTP call)
         else                          → SPARQL label match + P31 filter + year ±1
   1b. Collect edges in BOTH directions:
         SELECT ?o WHERE { wd:QID wdt:P144|wdt:P4969|wdt:P179 ?o }
         SELECT ?s WHERE { ?s wdt:P144|wdt:P4969 wd:QID }     ← ESSENTIAL (§2)
   1c. For each neighbour pull P31 (→ kind), P345/P4947/P648/P436 (→ external ids), P577.
   1d. Emit work_relation(rel_type='based_on', confidence=0.98,
         evidence=[{"source":"wikidata","prop":"P144","qid":"Q126086662"}])
   → served from the shipped artifact; live SPARQL only on cache miss.

── Tier 2: provider-native structure (0.90, auto-confirm) ─────────────────
   TMDB belongs_to_collection                     → same_franchise
   Open Library work→editions                     → edition_of / translation_of
   MusicBrainz release-group via P406             → soundtrack_of
   ComicInfo.xml <Series>                         → same_franchise

── Below 0.85: DISCARD. Store nothing. ────────────────────────────────────
```

**Tier 3 fuzzy inference and the review inbox are cut**, and this is a deliberate scope decision
rather than an omission. What was cut: seven weighted signals over trigram-blocked cross-kind
candidates, a calibration step, and a review inbox with a sorted queue, side-by-side cards,
plain-English evidence rendering, three verdict actions, permanent-rejection semantics, per-user
verdict scoping, an inline "review in context" affordance *and* a dedicated surface. That is a small
ML-shaped subsystem plus a second UI plus a per-user verdict store, built to recover the long tail.

The reasons, in order:

1. **A wrong link is far worse than a missing one.** The design conceded the point itself:
   *"title-similarity-based adaptation guessing is a false-positive machine."*
2. **The owner asked for one thing:** *"if I want to look up Train Dreams, I should see results for
   the movie and the book."* Train Dreams is in Wikidata; §2 verifies the exact edge.
3. **This project will not staff a false-positive-management UI.**

> **There is no review inbox.** Links come only from authoritative sources. A user can add a link
> manually (`source='manual'`, confidence 1.0) and delete any link from the item detail page. **If
> Wikidata does not know about an adaptation, UsArr does not claim one.**

`work_relation.evidence` stays `NOT NULL` — it is cheap and it powers the *"Based on the novella by
Denis Johnson"* string. `status`, `reviewed_by` and `reviewed_at` are dropped from migration 0001.
That also resolves a schema contradiction in which verdicts were declared per-user in prose and
stored globally in the DDL with no `user_id` in the primary key, so one user's rejection would have
suppressed a link for everyone. With no verdicts there is nothing to scope — but **manual links will
need `user_id` when multi-user lands**, and that is recorded as a v1.0 obligation in schema.md §11.

---

## 6. Disambiguation rules that actually work

1. **Never group on title alone.** Title equality is a candidate *generator*, never evidence.
   *The Wire* (TV) vs *The Wire* (album) vs *Wire* (band). *It* (1986 novel) → *It* (1990
   miniseries) → *It* (2017 film) is a real chain; *It* (1927 Clara Bow film) is **not** related.
2. **Require at least one of:** a shared external id, a Wikidata `P144`/`P4969` edge, or (creator
   overlap **and** temporal plausibility).
3. **Temporal plausibility:** an adaptation is published *after* its source, typically within 0–60
   years. `film.year < book.year` is near-fatal to an adaptation hypothesis.
4. **Novelizations invert the arrow** (Star Wars, Alien). Detect via year ordering and each side's
   `P31`; store the edge source→derived consistently with `rel_type='novelization_of'`.
5. **Filter disambiguation pages** (`P31 = Q4167410`) and list articles.

⚠️ **Tolerate noisy roles.** Wikidata lists Denis Johnson under `P58 screenwriter` on the film, which
appears to be a data-quality artifact (Bentley and Kwedar wrote the screenplay). Do not treat
"author == screenwriter" as impossible; do not weight it as strongly as an explicit `P144`.

---

## 7. Anime is a separate ID universe

AniDB, AniList, MAL, TVDB and TMDB all number anime differently (season vs absolute numbering, cours
vs series). **Do not solve this.** Vendor the community mapping files and refresh them periodically —
they are static files with no rate limits and no ban risk:
[Fribb/anime-lists](https://github.com/Fribb/anime-lists) (merged JSON keyed by AniDB id, mapping
anidb ↔ anilist ↔ mal ↔ kitsu ↔ tvdb ↔ tmdb ↔ imdb),
[ScudLee/anime-lists](https://github.com/ScudLee/anime-lists) (the classic AniDB↔TVDB XML),
[Goldenfreddy0703/Otaku-Mappings](https://github.com/Goldenfreddy0703/Otaku-Mappings).

This is also why `work_episode` allows `season_number` to be NULL when `absolute_number` is present
(schema.md §1.1) — an AniDB/AniList-sourced episode has only an absolute number.
