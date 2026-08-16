# Reference — \*Arr API facts

**Status:** facts, not design. Every row here was read from a shipped OpenAPI spec or from upstream
source, and says which. ⚠️ marks anything not verified in this pass.
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §6.2, §7.

The specs referenced are the ones shipped with this design (`sonarr.json`, `radarr.json`,
`lidarr.json`, `readarr.json`, `prowlarr.json`, `whisparr.json`).

---

## 1. API versions

| App | Base path |
|---|---|
| Sonarr 4.x, Radarr 5.x, Whisparr 2.x–3.x | `/api/v3` |
| Lidarr, Readarr, Prowlarr | `/api/v1` |

**API version is per app and does not track the app version. There is no "Radarr v5 API."** Never
guess it from a version string. ⚠️ Sonarr `develop` now carries an **API v5** alongside v3;
version-negotiate at connect time rather than hardcoding.

---

## 2. Field normalisation — the axis is `(app, resource)`, not `app`

The earlier design split `imdbId` and `indexerFlags` **by app**, which is wrong and produces a
deserialiser that fails silently (as a zero value) on the release path. Verified from the shipped
specs:

| App | Resource | Field | Type |
|---|---|---|---|
| Sonarr | `SeriesResource` | `imdbId` | `string` (`"tt0117951"`) |
| Radarr | `MovieResource` | `imdbId` | `string` |
| Sonarr | `ReleaseResource` | `imdbId` | **`string`** |
| Radarr | `ReleaseResource` | `imdbId` | **`integer` (int32)** |
| Prowlarr | `ReleaseResource` | `imdbId` | **`integer` (int32)** |
| Sonarr | `ReleaseResource` | `indexerFlags` | `integer` (int32 bitmask) |
| Radarr | `MovieFileResource` | `indexerFlags` | `integer` (int32 bitmask) |
| Radarr | `ReleaseResource` | `indexerFlags` | **untyped / schemaless — parse defensively as either int or `string[]`** |
| Prowlarr | `ReleaseResource` | `indexerFlags` | **`string[]`** |

So **within Radarr alone**, `imdbId` is a string on `MovieResource` and an int on
`ReleaseResource`; and Sonarr and Radarr **disagree with each other** on `ReleaseResource.imdbId`.

Other normalisation rules:

| Concept | REST resources | Webhook payload |
|---|---|---|
| `tags` | `int[]` | **`string[]` (labels!)** |
| `quality` | `QualityModel` object | `string` name + `qualityVersion` int |
| `protocol` in history `data` | **`int`** `0=unknown, 1=usenet, 2=torrent` | — |
| `protocol` on REST resources | `string` (`"usenet"`) | — |

**Store IMDb canonically as `tt0117951`. Store protocol canonically as the string. Do not share a
deserialiser between the REST and webhook shapes.**

---

## 3. `Quality` is not a shared shape

Verified from the shipped specs:

| App | `Quality` fields | `source` enum |
|---|---|---|
| Sonarr | `{id, name, source, resolution}` | `unknown, television, televisionRaw, web, webRip, dvd, bluray, blurayRaw` |
| Radarr | `{id, name, source, resolution, modifier}` | `unknown, cam, telesync, telecine, workprint, dvd, tv, webdl, webrip, bluray` |
| Whisparr | as Radarr, plus `vr` | — |
| **Lidarr** | **`{id, name}` only** | **none** |
| **Readarr** | **`{id, name}` only** | **none** |

Radarr additionally has `Modifier`: `none, regional, screener, rawhd, brdisk, remux`.

> **The earlier claim that "Lidarr's `Quality.source` are audio formats" is false — Lidarr and
> Readarr have no `source` field at all.** The prescription that followed it was right and is kept:
> **store `quality.id` + `quality.name` + the app that emitted it.** Do not model `source` as a
> shared enum.

---

## 4. List endpoints, content types, and required parameters

**Bare-array, unpaged, no sparse fieldsets:** `/api/v3/movie`, `/series`, `/artist`, `/album`,
`/book`, `/calendar`, `/tag`, `/qualityprofile`, `/rootfolder`, `/health`, `/indexer`.

**Declared 200 content types** (verified — note that `application/json` is present everywhere, so
`Accept: application/json` does **not** 406, contrary to a review claim that read the first entry
only):

| Endpoint | Declared |
|---|---|
| `radarr /api/v3/movie` | `text/plain`, `application/json`, `text/json` |
| `lidarr /api/v1/album`, `/api/v1/track` | `text/plain`, `application/json`, `text/json` |
| `readarr /api/v1/author`, `/book`, `/history/since` | `text/plain`, `application/json`, `text/json` |
| `lidarr /api/v1/artist` | `application/json` |
| `sonarr /api/v3/series`, `/api/v3/episode` | `application/json` |
| `radarr /api/v3/history/since` | `application/json` |

`options.ReturnHttpNotAcceptable = true` **is** confirmed in the shipped Servarr `Startup.cs`, so an
`Accept` the server cannot satisfy is a 406 — send
`Accept: application/json, text/plain;q=0.9, */*;q=0.1` and parse as JSON regardless of the returned
`Content-Type`. Response compression is on; enable gzip/br.

**Endpoints that require a parent id — enforced in the controller, not in the schema:**

| Endpoint | Required (one of) | Evidence |
|---|---|---|
| `GET /api/v3/episode` (Sonarr) | `seriesId`, `episodeIds`, `episodeFileId` | `EpisodeController.GetEpisodes` throws `BadRequestException("seriesId or episodeIds must be provided")`. The spec marks all parameters `required: false`. |
| `GET /api/v3/episodefile` (Sonarr) | `seriesId`, `episodeFileIds` | controller |
| `GET /api/v3/moviefile` (Radarr) | `movieId`, `movieFileIds` | controller |

**Consequence:** `/episode` is not a bare-array endpoint, and "never fetch children per-parent" is
unimplementable. The achievable rule is one call per series, 4–8 concurrent, jittered, token-bucketed
— and ~2,000 round trips for a 2k-series library, which must be budgeted (sync.md §2).

---

## 5. `/history/since`

Present in **six** apps, not five: `/api/v3/history/since` in Sonarr, Radarr and Whisparr;
`/api/v1/history/since` in Lidarr, Readarr and Prowlarr. Parameters: `date` (date-time), `eventType`,
plus per-app include flags (`includeSeries`/`includeEpisode` in Sonarr, `includeMovie` in Radarr).
**There is no `limit` parameter** (unlike `/history`), so the response is unbounded.
⚠️ Behavioural parity across apps is **not** verified — probe at connect time.

**Prowlarr's history is telemetry, not entity change:**

```
prowlarr.HistoryEventType        = ["unknown","releaseGrabbed","indexerQuery","indexerRss",
                                    "indexerAuth","indexerInfo"]
sonarr.EpisodeHistoryEventType   = ["unknown","grabbed","seriesFolderImported",
                                    "downloadFolderImported","downloadFailed",…]
```

`prowlarr.HistoryResource` also lacks `movieId`/`seriesId`. `indexerQuery`/`indexerRss` fire on every
RSS poll of every indexer. **Prowlarr is not a delta-sync source**; filter `eventType=releaseGrabbed`
and treat it as provenance input.

---

## 6. Media cover paths differ per app

Verified from the shipped specs:

| App | Path |
|---|---|
| Sonarr | `/api/v3/mediacover/{seriesId}/{filename}` |
| Radarr | `/api/v3/mediacover/{movieId}/{filename}` |
| Lidarr | `/api/v1/mediacover/artist/{artistId}/{filename}` and `/api/v1/mediacover/album/{albumId}/{filename}` |
| Readarr | `/api/v1/mediacover/author/{authorId}/{filename}` and `/api/v1/mediacover/book/{bookId}/{filename}` |
| Prowlarr | none |

The generic `/api/vN/mediacover/{id}/{file}` shape is **wrong for Lidarr and Readarr**, which
interpose a resource segment.

`MediaCover.url` is **instance-relative** and **requires the API key**, which is why images are
proxied (ARCHITECTURE §4.4). It must be resolved **only** by joining to the owning instance's base
URL — never by trusting an absolute URL in that field (security.md §2). `MediaCover.remoteUrl` points
at TMDB/TheTVDB and is publicly fetchable; prefer it at ingest where present, under the provider
fetch policy.

---

## 7. Prowlarr search and grab

| Operation | Call | Returns |
|---|---|---|
| Free-text search | `GET /api/v1/search?query=&type=&indexerIds=&categories=&limit=&offset=` | `ReleaseResource[]`, `application/json` |
| Grab | `POST /api/v1/search` with a `ReleaseResource` body | `ReleaseResource` |

`ReleaseResource` fields: `id, guid, age, ageHours, ageMinutes, size, files, grabs, indexerId,
indexer, subGroup, releaseHash, title, sortTitle, imdbId, tmdbId, tvdbId, tvMazeId, publishDate,
commentUrl, downloadUrl, infoUrl, posterUrl, indexerFlags, categories, magnetUrl, infoHash, seeders,
leechers, protocol, fileName, downloadClientId`.

**`downloadClientId` is what makes Search-and-Grab mode affordable:** a grab can name one of
Prowlarr's own configured download clients, so UsArr needs no download-client integration to
complete the flow.

> 🚩 **Prowlarr's grab cache is 30 minutes.** `SearchController.MapReleases()` rewrites download URLs
> to Prowlarr proxy links and caches the original `ReleaseInfo` in memory keyed `"{indexerId}_{guid}"`
> for `TimeSpan.FromMinutes(30)`. **POST the release back within 30 minutes or the grab fails.**

**Prowlarr failures are soft.** It has historically returned **HTTP 200 with an error in the body**
when a query limit was hit, and upstream 429s surface only as generic connection failures. Read
`indexerstatus.disabledTill` and treat search failure as non-fatal.

Also: `posterUrl` on `ReleaseResource` is an **indexer-supplied URL from a response body** — the
derived SSRF class (security.md §2), not a provider URL and not an admin-configured one.

---

## 8. Newznab / Torznab categories

Parent category is an independent, always-present media-type signal:

```
floor(cat/1000)*1000 →
  2000 movie | 5000 tv (5070→anime) | 3000 music (3030→audiobook)
  7000 book (7020→ebook, 7030→comic, 7010→magazine)
  6000 adult | 1000/4000 game/software | 0/8000 other
```

Categories ≥ 100000 are site-specific and need the indexer's `t=caps` to resolve; fall back to the
parent cat Prowlarr also emits. **Category `3030` is the only reliable machine signal separating
audiobook from music at acquisition time.** Capture the raw array; never collapse it.

> 🚩 **Two corrections, both verified against `NewznabStandardCategory.cs` on `develop` and both live
> bugs in the mapping above as it was previously used (2026-08-16).**
>
> 1. **`floor(cat/1000)*1000` is not sufficient, and the parent rule must not run first.**
>    **Audiobooks are `3030`, under `Audio` (3000), not under `Books` (7000)** — so a `type:` tag
>    derived from the *parent* category labels every audiobook release `type:audio`. The special
>    cases are consulted **before** the parent rule, not after it, and **all five now have a stated
>    rule rather than only being named** — `5070` and `7010` were listed as fixed and were not:
>
>    | Category | Verified name | Rule |
>    |---|---|---|
>    | `3030` | `Audio/Audiobook` (line 43) | → `(book, audiobook)`. Parent is `Audio`, so the parent rule tags it `type:audio`. |
>    | `5070` | `TV/Anime` (line 65) | → `type:anime` **in addition to** `type:tv`. The parent `TV` is correct but discards the leaf, and anime routing needs it. |
>    | `7010` | `Books/Mags` (line 82) | → `type:magazine`. The parent `Books` calls a magazine an ebook. |
>    | `7020` | `Books/EBook` | → `(book, ebook)`; `7000`/`7040`/`7060` also accepted, since indexers file ebooks under the parent and under Technical/Foreign. |
>    | `7030` | `Books/Comics` | **ranking signal only**; the comics-and-manga *filter* is the parent `7000` — see (2). |
>
>    `ARCHITECTURE.md` §8.5 carried the parent-first wording, is corrected, and now carries the same
>    five rows.
> 2. **`7030 → comic` returns zero manga, because there is no manga category in the Newznab
>    standard.** `7030 Books/Comics` is the only comics category anywhere in the tree, and
>    **Nyaa — the dominant public manga tracker — maps its `Literature` categories (`3_0`, `3_1`,
>    `3_2`, `3_3`) to `Books` (7000), not to `Books/Comics`**, verified in
>    `Prowlarr/Indexers/definitions/v11/nyaasi.yml`. **A comics-and-manga search therefore filters on
>    the parent `7000`**, accepting that it also returns ebooks, magazines and technical books, and
>    uses `7030` only as a **ranking** signal for western comics.
>
> Scale, for expectations rather than for the mapping: of 543 definitions in `definitions/v11`, **88
> declare `Books/Comics`** and 288 declare some `Books` category, but **only three are comic- or
> manga-specific** (`comicat.yml`, `nyaasi.yml`, `sukebeinyaasi.yml`). And **GetComics — the dominant
> DDL source for western comics, and the only source Kapowarr searches — is not a Prowlarr indexer at
> all**, nor is any other comics DDL site.

---

## 9. Auth, CORS and other traps

- An \*Arr API key grants **full admin** — delete the library, change root folders, restart the
  process. One opaque 32-hex string, no scoping, accepted three ways: `X-Api-Key` header, `?apikey=`
  query, `Authorization: Bearer`.
- The `/api/vN/*` CORS policy is `AllowAnyOrigin` via `VersionedApiControllerAttribute` — a red
  herring. A browser *can* call an \*Arr directly; doing so ships an admin credential to every user.
- **The SignalR hub is NOT covered by any CORS policy.** `MapHub` has no `.RequireCors(...)`, so the
  negotiate POST is a cross-origin XHR and is blocked in a browser. UsArr terminates SignalR
  server-side.
- ⚠️ API keys are not validated for URL-safety upstream — a key containing `#` breaks the
  `access_token` query form. URL-encode it and warn in config if it contains non-`[A-Za-z0-9]`
  characters.
- ⚠️ **SSL is off by default.** Servarr apps ship with `EnableSsl = false`, so out of the box they
  serve plain HTTP. Where a user enables it, the default SSL port is per-app — Sonarr **9898**
  (confirmed from the shipped `SystemResource`); Radarr 7879, Lidarr 8687, Readarr 8788, Prowlarr
  9697 (⚠️ medium confidence, verify before publishing) — and the certificate is usually
  self-signed.
- Webhook `eventType` is **PascalCase**, unlike everything else in these APIs, and
  `eventType:"Download"` has **two body shapes** — discriminate on `episodeFile` (single) vs
  `episodeFiles` (import complete).

---

## 10. External metadata providers — rate limits

| Provider | Limit | Confidence |
|---|---|---|
| MusicBrainz | **1 req/s per IP**, hard, enforced with 503s; `User-Agent: UsArr/<ver> ( <contact> )` mandatory | verified |
| Open Library | 1 req/s anonymous, 3 req/s with an identifying UA | verified |
| Hardcover | 60 req/min | verified |
| Metron | 20 req/min burst, 5,000/day | verified |
| **TMDB** | **⚠️ unpublished.** The widely-repeated "~40 req/s per IP" is a forum claim, not ToS. | unverified |
| **AniList** | **⚠️ "90 req/min" could not be confirmed** — `docs.anilist.co` returns 403 to automated fetch, and AniList has publicly run degraded at a substantially lower limit for extended periods. | unverified |

**For both unverified entries: read `X-RateLimit-Remaining` where present and back off adaptively on
429 rather than hard-coding a constant.** A client pinned to 90/min against a degraded AniList is
rate-limited by design.

Licensing notes that constrain design: **TMDB requires attribution, imposes a 6-month cache limit,
and its ToU names "training or validating a machine learning or AI system" as commercial use.**
**Wikidata is CC0** — no attribution, no cache limit, redistribution legal. **Goodreads is dead**
(stopped issuing keys 2020-12-08, blanket 403s by late 2025 — this is why Readarr was archived);
useful only as Wikidata property `P8383`. **TheTVDB v4 is not free** — either a negotiated commercial
contract, or "user-supported" access where every end user holds a paid subscription and supplies a
PIN. **`api.lidarr.audio`** is a Servarr-run cache proxy in front of MusicBrainz with documented
multi-hour propagation lag; go to MusicBrainz directly.
