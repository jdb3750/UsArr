# UsArr — Research Digest

Condensed from five parallel deep-research tracks executed **2026-08-16**. This is the
evidence base for [`ARCHITECTURE.md`](./ARCHITECTURE.md). Where the architecture asserts a
fact, the citation is here.

**Verification legend — preserved exactly as the researchers marked it:**

| Mark | Meaning |
|---|---|
| ✅ | **Verified live** — the researcher called the API, fetched the doc, or read the source during the pass |
| 📄 | Read from vendor documentation or a reputable secondary source, but not independently exercised |
| ⚠️ | **Unverified or volatile** — re-check before code depends on it |
| **[F]** / **[R]** / **[A]** | Track 03's tags: researched **F**act with a source / **R**ecommendation-judgement / **A**ssumption to confirm |

**Rate limits, licensing terms and API shapes change.** Treat every ⚠️ as a live TODO, and
re-verify anything load-bearing before it ships.

**Local artifacts** captured during the research pass (not in this repo; regenerate if
needed): the six shipped OpenAPI specs (`sonarr`, `radarr`, `lidarr`, `readarr`, `prowlarr`,
`whisparr`) pulled raw from each project's `develop` branch, plus a cross-app endpoint-path
matrix.

---

## Track 01 — The *Arr ecosystem: APIs and data models

### 1.1 The seven facts that shape the design

1. **Five of the six apps are literally the same codebase** (NzbDrone fork lineage). Auth,
   paging, tags, quality, commands, SignalR, webhooks, `system/status`, `rootfolder`,
   `qualityprofile` are byte-for-byte identical contracts. **Build one Servarr client plus one
   per-media-type mapping layer, not six clients.** ✅
2. **API version is per app and does not track app version.** Sonarr (4.x) / Radarr (5.x) /
   Whisparr (2.x–3.x) → `/api/v3`. Lidarr / Readarr / Prowlarr → `/api/v1`. **There is no
   "Radarr v5 API."** ✅
3. **Auth is trivially uniform**: `X-Api-Key: <key>` header, or `?apikey=<key>`, or
   `Authorization: Bearer <key>` — all three accepted by the same handler. ✅
4. **CORS is `AllowAnyOrigin` for `/api/vN/*` but NOT for `/signalr/messages`.** A browser can
   call the REST API cross-origin but cannot open the SignalR hub. → **UsArr needs a
   server-side proxy at minimum for real-time, and for everything in practice.** ✅
5. **`protocol: "usenet" | "torrent"`** is a first-class enum on Prowlarr's `IndexerResource`
   and on every Servarr `ReleaseResource`/`QueueResource`. **The source tag comes straight off
   the wire; no inference needed.** ✅
6. **Newznab/Torznab category IDs are the cleanest cross-media type signal**: 2000=Movies,
   3000=Audio, 5000=TV, 6000=XXX, 7000=Books, 1000=Console, 4000=PC, 8000/0000=Other,
   100000+ = site-specific. ✅
7. **Readarr is dead** — the repo was archived by the owner on **2025-06-27**. ✅

### 1.2 Apps, ports, API paths ✅

| App | Current major | API base | Default port |
|---|---|---|---|
| Sonarr (TV) | v4 | `/api/v3` | 8989 |
| Radarr (Movies) | v5 | `/api/v3` | 7878 |
| Lidarr (Music) | v2 | `/api/v1` | 8686 |
| Readarr (Books) **RETIRED** | v0.4 | `/api/v1` | 8787 |
| Prowlarr (Indexers) | v1/v2 | `/api/v1` | 9696 |
| Whisparr (Adult) | v2 "Eros" | `/api/v3` | 6969 |

Ports are the `servers[0].variables.hostpath.default` values baked into each spec. Sonarr's own
spec says *"The v3 API docs apply to both v3 and v4 versions of Sonarr"*, confirmed in
`src/NzbDrone.Host/Startup.cs`. **Whisparr v3's API is a Sonarr clone**
(`series`/`episode`/`episodefile`/`seasonpass`), not a Radarr clone ✅; Whisparr v2 "Eros" is
⚠️ unverified — treat only the `develop` shape as canonical.

> 🚩 **`app.UseSwagger()` is guarded by `if (BuildInfo.IsDebug)` in `Startup.cs`, so a
> production instance does NOT serve `/docs/v3/openapi.json`.** ✅ Ship the specs with UsArr;
> do not try to fetch them from the user's instance.

### 1.3 Auth mechanics ✅

From `src/Sonarr.Http/Authentication/AuthenticationBuilderExtensions.cs`, identical in all apps:

```csharp
.AddApiKey("API",     o => { o.HeaderName = "X-Api-Key"; o.QueryName = "apikey"; })
.AddApiKey("SignalR", o => { o.HeaderName = "X-Api-Key"; o.QueryName = "access_token"; })
```

`ApiKeyAuthenticationHandler.ParseApiKey()` checks, in order: query param → `X-Api-Key` header
→ `Authorization: Bearer` (string-replacing `"Bearer "` → `""`). A wrong key returns
`NoResult()` not `Fail`, so you get a **401 challenge, never a 403**, for a bad key. ✅

`config.xml` element names, from `ConfigFileProvider.cs`, root `<Config>`: `ApiKey`, `Port`,
`SslPort` (9898), `EnableSsl`, `UrlBase`, `BindAddress`, `InstanceName`,
`AuthenticationMethod`, `AuthenticationRequired`, `Branch`, `PostgresHost`/`Port`/`MainDb`. ✅
**The API key is auto-generated if missing and is 32 hex chars.** ⚠️ The env-var prefix for
injecting it (`SONARR__AUTH__APIKEY`-style) was **not confirmed** — the options binding
(`_authOptions.ApiKey`) exists, the key prefix was not verified.

**Every API response carries `X-Application-Version`** (`VersionMiddleware.cs`, const
`VERSIONHEADER`). ✅ Free version detection on every call.

**Unauthenticated probes:** `GET|HEAD /ping` → `{status: "OK"|"Error"}`, marked
`[AllowAnonymous]` in `PingController.cs`, returns **500 with `status:"Error"` if the DB is
unreachable** ✅ — so it distinguishes "not a Servarr app" from "sick Servarr app". `GET /api`
exists in every spec but its exact body is ⚠️ unverified (the controller was not located;
observed convention is `{"current":"v3"}`).

### 1.4 Shared conventions ✅

**Paging.** `page`, `pageSize`, `sortKey`, `sortDirection` → `{page, pageSize, sortKey,
sortDirection, totalRecords, records[]}`. Paged: `/queue`, `/history`, `/blocklist`,
`/wanted/missing`, `/wanted/cutoff`, Radarr `/exclusions/paged`, Sonarr
`/importlistexclusion/paged`.
**Not paged — bare arrays:** `/series`, `/movie`, `/artist`, `/author`, `/album`, `/book`,
`/episode`, `/calendar`, `/tag`, `/qualityprofile`, `/rootfolder`, `/health`, `/command`,
`/indexer`, `/downloadclient`, `/notification`.

**Tags** are universally `TagResource {id: int, label: string}`, and every entity has
`tags: int[]`. `GET /tag/detail` → `TagDetailsResource {id, label, delayProfileIds,
importListIds, notificationIds, restrictionIds, indexerIds, downloadClientIds, autoTagIds,
seriesIds}`. **Tags are integer-keyed, per-instance labels with no namespace — two instances'
tag `3` are unrelated.** ✅ There is also a full `AutoTaggingResource` with `specifications`
(implementation + fields + `negate` + `required`) and `removeTagsAutomatically`. ✅

**Commands.** `POST /api/v3/command` `{name, ...args}` → `CommandResource {id, name,
commandName, message, body, priority, status(queued|started|completed|failed|aborted|
cancelled|orphaned), result, queued, started, ended, duration, exception, trigger, …}`.
⚠️ **The complete list of valid command `name` values is unverified** — the spec models `body`
as an untyped `Command`; the authoritative list is in `NzbDrone.Core/**/*Command.cs`.

**Quality.** `QualityModel {quality: Quality, revision: Revision}`;
`Quality {id, name, source, resolution}`; `Revision {version, real, isRepack}`.
⚠️ **`Quality.source` enum members are per-app** — Sonarr's
(`television|televisionRaw|web|webRip|dvd|bluray|blurayRaw`) differ from Radarr's
(`bluray|webdl|webrip|dvd|tv|cam|telesync|…`) and Lidarr's are audio formats. **Do not model
`source` as a shared enum.** ✅ (Sonarr's members dumped; Radarr's/Lidarr's ⚠️ not
exhaustively enumerated.)

**Images.** `MediaCover {coverType, url, remoteUrl}`. **`url` is instance-relative
(`/MediaCover/123/poster.jpg`), served under `/api/v3/mediacover/{id}/{file}`, and requires the
API key** — so UsArr must proxy images. `remoteUrl` points at TheTVDB/TMDB and is publicly
fetchable. ✅

### 1.5 Per-app external IDs ✅

| App | Primary key | Other external IDs |
|---|---|---|
| Sonarr `SeriesResource` | **`tvdbId` (int)** | `tvRageId`, `tvMazeId`, `tmdbId`, `imdbId` (string `tt…`), `titleSlug` |
| Radarr `MovieResource` | **`tmdbId` (int)** | `imdbId` (string), `titleSlug`. **No tvdbId.** |
| Lidarr `ArtistResource` | **`foreignArtistId`** (MusicBrainz artist MBID) | `mbId`, `tadbId`, `discogsId`, `allMusicId` |
| Lidarr `AlbumResource` | **`foreignAlbumId`** (MB **release-group** MBID) | `AlbumReleaseResource.foreignReleaseId` = MB *release* MBID |
| Lidarr `TrackResource` | `foreignTrackId` (MB track) | `foreignRecordingId` (MB recording) |
| Readarr `AuthorResource` | `foreignAuthorId` = **Goodreads author id** | — |
| Readarr `BookResource` | `foreignBookId` = **Goodreads work id** | `foreignEditionId` |
| Readarr `EditionResource` | `foreignEditionId` = Goodreads edition/GRID | **`isbn13`, `asin`** |

> **Readarr has no `olid` field** — the Open Library migration never landed
> ([Readarr#3486](https://github.com/Readarr/Readarr/issues/3486)). `isbn13` + `asin` are the
> only portable book identifiers there. ✅

**Structural difference to normalise:** Sonarr's `ratings` is `{votes, value}`; Radarr's is
`{imdb, tmdb, metacritic, rottenTomatoes, trakt}` — each a `RatingChild`. ✅
Radarr's `MovieFileResource` has an `edition` field and **no `releaseType`**; Sonarr's
`EpisodeFileResource` has `releaseType (unknown|singleEpisode|multiEpisode|seasonPack)` and no
`edition`. ✅

**`MediaInfoResource`** (identical in Sonarr/Radarr, and what UsArr needs for any playback
decision): `audioBitrate, audioChannels, audioCodec, audioLanguages, audioStreamCount,
videoBitDepth, videoBitrate, videoCodec, videoFps, videoDynamicRange, videoDynamicRangeType,
resolution ("1920x1080"), runTime, scanType, subtitles`. ✅

### 1.6 Queue, history, releases ✅

**`QueueResource`**: `id, seriesId/movieId/artistId…, languages[], quality, customFormats[],
customFormatScore, title, size, sizeleft, timeleft, estimatedCompletionTime, added, status
(unknown|queued|paused|downloading|completed|failed|warning|delay|downloadClientUnavailable|
fallback), trackedDownloadStatus (ok|warning|error), trackedDownloadState (downloading|
importBlocked|importPending|importing|imported|failedPending|failed|ignored), statusMessages[],
errorMessage, **downloadId**, **protocol**, downloadClient, indexer, outputPath`.
`GET /queue/status` → `{totalCount, count, unknownCount, errors, warnings, …}` — a **cheap
polling target for a badge**. ✅

**`HistoryResource`**: `id, episodeId/movieId, seriesId, sourceTitle, languages[], quality,
customFormats[], customFormatScore, qualityCutoffNotMet, date, **downloadId**, eventType,
data (free-form dict)`. Sonarr `eventType`: `unknown|grabbed|seriesFolderImported|
downloadFolderImported|downloadFailed|episodeFileDeleted|episodeFileRenamed|downloadIgnored`.
Radarr/Lidarr/Readarr have analogous but **differently named** enums — normalise. ✅

**`GET /history/since?date=&eventType=` exists in ALL FIVE apps** — `/api/v3/history/since` in
Sonarr and Radarr, `/api/v1/history/since` in Lidarr, Readarr and Prowlarr, confirmed from the
cross-app path matrix built from the shipped specs. ✅ Sonarr takes
`date, eventType, includeSeries, includeEpisode`; Radarr takes `date, eventType, includeMovie`.
⚠️ **Behavioural parity across all five is not verified — only existence is.**

**`ReleaseResource`** (`GET /api/v3/release?seriesId=&episodeId=` / Radarr `?movieId=`):
`guid, title, indexerId, indexer, size, age, ageHours, publishDate, quality, qualityWeight,
languages[], customFormats[], customFormatScore, releaseGroup, subGroup, releaseHash,
**protocol**, downloadUrl, magnetUrl, infoUrl, commentUrl, infoHash, seeders, leechers,
indexerFlags(int), approved, rejected, temporarilyRejected, rejections[], downloadAllowed,
releaseWeight, sceneSource, fullSeason, seasonNumber, episodeNumbers[], tvdbId, imdbId, …`. ✅

To grab: **POST back the exact `ReleaseResource` you received** — the server keeps a
server-side cache keyed by guid. `POST /api/v3/release/push` accepts an externally-discovered
release. ✅

**`GET /api/v3/parse?title=&path=`** → `ParseResource {title, parsedEpisodeInfo, series,
episodes[], languages[], customFormats[], customFormatScore}` — **very useful for normalising
arbitrary release names without reimplementing the parser.** ✅

### 1.7 SignalR ✅

- Endpoint **`{urlBase}/signalr/messages`**, registered
  `x.MapHub<MessageHub>("/signalr/messages").RequireAuthorization("SignalR")` in
  `src/NzbDrone.Host/Startup.cs`.
- **ASP.NET Core SignalR** (not legacy). `POST {url}/negotiate?negotiateVersion=1` then
  WebSocket/SSE/LongPolling. Server registers `AddSignalR().AddJsonProtocol(...)` and
  `app.UseWebSockets()`.
- **Single client method: `receiveMessage`**, broadcast via
  `Clients.All.SendAsync("receiveMessage", message)` in `MessageHub.cs`.
- DTO: `SignalRMessage { object Body; string Name; [JsonIgnore] ModelAction Action; }` — so
  **`Action` is not serialised at the top level**; it rides inside `Body` as
  `ResourceChangeMessage<T> {resource, action}` (`RestControllerWithSignalR.cs`).
- `ModelAction` values: **`Created`, `Updated`, `Deleted`, `Sync`**. `Sync` means "your copy of
  this whole resource type is stale — refetch" and is used by `TagController`,
  `HealthController`, `QueueController`, `QualityDefinitionController`.
- On connect the hub pushes `{name:"version", body:{version:"4.x.y"}}` (`OnConnectedAsync`).
- `name` values Sonarr's own UI handles (from `frontend/src/Components/SignalRListener.tsx`):
  `calendar, command, downloadclient, episode, episodefile, health, importlist, indexer,
  metadata, connection, qualitydefinition, queue, queue/details, queue/status, rootfolder,
  series, system/task, tag, version, wanted/cutoff, wanted/missing`.
  ⚠️ **Per-app lists other than Sonarr's are inferred, not source-read.**
- **Perf detail:** `SignalRMessageBroadcaster.IsConnected` short-circuits — the server does no
  broadcast work when zero clients are connected. So UsArr holding a persistent hub connection
  imposes a small cost on the *Arr instance.
- ⚠️ **Sonarr `develop` now carries an API v5** alongside v3 (`src/Sonarr.Api.V5/…`), and
  `RestControllerWithSignalR` contains **V3 API version filtering that determines whether
  messages are sent to SignalR clients at all.** Version-negotiate; do not hardcode v3.
- ⚠️ **API keys are not validated for URL-safety** — a key containing `#` "will ruin HTTP
  requests to SignalR" ([Radarr#8441](https://github.com/Radarr/Radarr/issues/8441)).
- Radarr's own docs: SignalR failure's *"most common occurrence is use of a reverse proxy or
  cloudflare, which needs websockets enabled"* — [Servarr wiki](https://wiki.servarr.com/radarr/system). 📄

### 1.8 Webhooks ✅

Configured via `/api/v3/notification` (+ `/schema`, `/test`). Base payload
`{eventType, instanceName, applicationUrl}`.

> 🚩 **`eventType` is PascalCase**, not camelCase — there is a
> `// TODO: In v4 this will likely be changed to the default camel case` comment upstream that
> has still not been actioned. ✅

| App | `WebhookEventType` members |
|---|---|
| **Sonarr** | `Test, Grab, Download, Rename, SeriesAdd, SeriesDelete, EpisodeFileDelete, Health, ApplicationUpdate, HealthRestored, ManualInteractionRequired` |
| **Radarr** | `Test, Grab, Download, Rename, MovieDelete, MovieFileDelete, Health, ApplicationUpdate, MovieAdded, HealthRestored, ManualInteractionRequired` |
| **Lidarr** | `Test, Grab, Download, DownloadFailure, ImportFailure, Rename, ArtistAdd, ArtistDelete, AlbumDelete, Health, Retag, ApplicationUpdate, HealthRestored` |
| **Prowlarr** | `Test, Grab, Download, Rename, Health, ApplicationUpdate, HealthRestored` |

> 🚩 **Gotcha found in source** (`WebhookBase.cs` lines 52–95):
> `BuildOnImportCompletePayload()` emits `EventType = WebhookEventType.Download` — **not** a
> distinct `ImportComplete` — even though it builds a `WebhookImportCompletePayload` with a
> different body (`episodeFiles[]`, `fileCount`, `sourcePath`, `destinationPath`). So
> **`eventType:"Download"` has two possible body shapes.** Discriminate on the presence of
> `episodeFile` (single) vs `episodeFiles` (complete). ✅

**Webhook payloads are flattened relative to REST:** `quality` is a plain string name plus
`qualityVersion` int, and **`tags` are string labels**, not int ids. **Do not share a
deserialiser.** ✅

### 1.9 Prowlarr ✅

`GET /api/v1/search?query=&type=&indexerIds=&categories=&limit=&offset=` → `ReleaseResource[]`.
`type` maps directly to the Newznab `t=` function (`search|tvsearch|movie|music|book`) — the
controller sets `NewznabRequest.t = payload.Type` verbatim. `indexerIds` omitted or `[-1]` ⇒
all enabled indexers.

**Prowlarr's `ReleaseResource` is deliberately different** from the Servarr apps':
`guid, title, sortTitle, indexerId, indexer, size, files, grabs, age, publishDate, downloadUrl,
magnetUrl, infoUrl, commentUrl, posterUrl, fileName, infoHash, seeders, leechers, subGroup,
releaseHash, **protocol**, **categories: IndexerCategory[]**, **indexerFlags: string[]**,
**imdbId (int!)**, tmdbId, tvdbId, tvMazeId, downloadClientId`.

> 🚩 **`imdbId` is an `int` (7157) in Prowlarr and a `string` (`"tt0007157"`) in
> Sonarr/Radarr. `indexerFlags` is `string[]` in Prowlarr and an `int` bitmask in
> Sonarr/Radarr. Normalise at ingest.** ✅

> 🚩 **Prowlarr's grab cache is 30 minutes.** `SearchController.MapReleases()` calls
> `_downloadMappingService.ConvertToProxyLink(...)`, producing
> `{prowlarrUrl}/{indexerId}/download?link=<encoded>&file=<title>`, and caches the original
> `ReleaseInfo` in a memory cache keyed `"{indexerId}_{guid}"` for
> `TimeSpan.FromMinutes(30)`. **You must POST the release back within 30 minutes or the grab
> fails. Do not persist Prowlarr releases as long-lived grabbable objects.** ✅

**`IndexerResource`**: `id, name, definitionName, implementation (Newznab|Torznab|Cardigann|
<site>), configContract, language, enable, redirect, priority, appProfileId, downloadClientId,
tags[], indexerUrls[], fields[Field], **protocol**, **privacy: public|semiPrivate|private**,
supportsRss, supportsSearch, supportsRedirect, **supportsPagination**, capabilities, status`. ✅

**`IndexerCapabilityResource`**: `limitsMax, limitsDefault, categories[], supportsRawSearch,
searchParams[], tvSearchParams[], movieSearchParams[], musicSearchParams[], bookSearchParams[]`
with these enum members — **exactly what smart search should key off**: ✅

```
SearchParam      : q
TvSearchParam    : q, season, ep, imdbId, tvdbId, rId, tvMazeId, traktId, tmdbId, doubanId, genre, year
MovieSearchParam : q, imdbId, tmdbId, imdbTitle, imdbYear, traktId, genre, doubanId, year
MusicSearchParam : q, album, artist, label, year, genre, track
BookSearchParam  : q, title, author, publisher, genre, year
```

**`Field`** — the generic provider-settings model shared by indexer/downloadclient/
notification/importlist across all apps: `{order, name, label, unit, helpText,
helpTextWarning, helpLink, value, type, advanced, selectOptions[],
selectOptionsProviderAction, section, hidden, **privacy: normal|password|apiKey|userName**,
placeholder, isFloat}`. ✅ **`privacy` tells you exactly which fields to redact**, and the model
is the key to a *generic* settings editor rendered from `/schema` with nothing hardcoded.

**How Prowlarr proxies to apps** (`src/NzbDrone.Core/Applications/Sonarr/Sonarr.cs`,
`BuildSonarrIndexer`): it creates in Sonarr an indexer with
`Implementation = protocol == Usenet ? "Newznab" : "Torznab"`,
`baseUrl = "{ProwlarrUrl}/{indexer.Id}/"`, `apiPath = "/api"`,
`apiKey = <Prowlarr's own key>`, `Name = "{indexer.Name} (Prowlarr)"`. ✅
→ **Every Sonarr/Radarr indexer whose `baseUrl` matches `{prowlarr}/{n}/` is a Prowlarr-proxied
indexer, and `Newznab` vs `Torznab` is a second, redundant usenet/torrent signal.** Use it to
de-duplicate "the same indexer seen through four apps."

**`GET /api/v1/indexerstatus`** → `{indexerId, disabledTill, mostRecentFailure,
initialFailure}` ✅ — read `disabledTill` and skip known-down indexers.

### 1.10 Newznab/Torznab substrate

Specs: [Torznab 1.3-draft](https://torznab.github.io/spec-1.3-draft/torznab/Specification-v1.3.html),
[embedded Newznab reference](https://torznab.github.io/spec-1.3-draft/external/newznab/api.html),
[Sonarr's implementer guide](https://github.com/Sonarr/Sonarr/wiki/Implementing-a-Torznab-indexer).

Functions (`t=`): `caps` (mandatory, no apikey), `search`, `tvsearch`, `movie`, `music`,
`book`, `details`, `getnfo`, `get`, `cartadd`, `cartdel`, `comments`, `commentadd`, `register`,
`user`.

**Canonical category tree**, verbatim from
`Prowlarr/src/NzbDrone.Core/Indexers/NewznabStandardCategory.cs` ✅:

```
0000 Other       1000 Console    2000 Movies     3000 Audio     4000 PC
5000 TV          6000 XXX        7000 Books      8000 Other     100000+ site-specific
2030 SD  2040 HD  2045 UHD  2050 BluRay  2060 3D  2070 DVD  2080 WEB-DL  2090 x265
3010 MP3 3020 Video 3030 AUDIOBOOK 3040 Lossless 3050 Other 3060 Foreign
5010 WEB-DL 5030 SD 5040 HD 5045 UHD 5060 Sport 5070 ANIME 5080 Documentary 5090 x265
7010 Mags 7020 EBOOK 7030 COMICS 7040 Technical 7050 Other 7060 Foreign
```

**Media-type mapping is a one-liner:** `floor(catId/1000)*1000`. **Category `3030` is the only
reliable machine signal separating audiobook from music at acquisition time; `7030` likewise
for comics.** Cats ≥ 100000 need the indexer's caps to resolve; fall back to the parent cat
Prowlarr also emits. ✅

**Torznab item attributes add `seeders`, `leechers`, `peers`, `infohash`, `magneturl`,
`downloadvolumefactor`, `uploadvolumefactor`, `minimumratio`, `minimumseedtime`.** The presence
of `<torznab:attr name="seeders">` vs `<newznab:attr name="usenetdate">` is **itself a reliable
usenet/torrent discriminator** if you ever hit a raw indexer without Prowlarr's `protocol`
field. ✅

### 1.11 Books after Readarr

- **github.com/Readarr/Readarr was archived by the owner on 2025-06-27** ✅. Reason per the
  README: *"the project's metadata has become unusable, we no longer have the time to remake or
  repair it, and the community effort to transition to using Open Library as the source has
  stalled."* The metadata backend was `api.bookinfo.club` (Goodreads-derived), now offline.
  **The API still works** — the binary runs and `/api/v1/*` responds — it just cannot fetch
  metadata for new items. So UsArr can still support Readarr read-only for existing libraries.
- Servarr wiki has retired pages: [readarr/faq](https://wiki.servarr.com/readarr/faq),
  [readarr/metadata-issues](https://wiki.servarr.com/readarr/metadata-issues). 📄

| Successor | What it is | Status |
|---|---|---|
| **rreading-glasses** | Drop-in replacement for `api.bookinfo.club`; keeps unmodified Readarr working | Most widely cited fix; even the Readarr README names it as "the most popular mirror" with a no-support disclaimer. **Lowest-effort path: Readarr + rreading-glasses keeps the Readarr v1 contract valid.** 📄 |
| **Bookshelf** ([pennydreadful/bookshelf](https://github.com/pennydreadful/bookshelf)) | Community revival/fork of Readarr, .NET, GPLv3, ~753★ | Active fork 📄 ⚠️ |
| **Shelfmark** | Another successor | ⚠️ Not verified |
| **Bindery** ([vavallee/bindery](https://github.com/vavallee/bindery)) | Ground-up replacement; usenet + SABnzbd; OpenLibrary/Google Books/Hardcover/DNB/Audnex/Audible | ⚠️ Not verified |
| **LazyLibrarian** | Long-lived independent book/mag/comic/audiobook manager | Actively maintained, different API paradigm entirely ✅ |
| Calibre-Web / CWA, BookLore | Library/reader frontends, **not acquisition** | Complementary ⚠️ |

⚠️ **Two research tracks named different successor sets** (Track 01: Bookshelf, Bindery,
LazyLibrarian; Track 02: Bookshelf, Shelfmark, rreading-glasses, LazyLibrarian). The union is
above. **The maintenance status, API shape and version of Bookshelf/Bindery/Shelfmark/BookLore
were not verified from primary sources by either track.** This is precisely why UsArr must not
hard-code a book backend.

### 1.12 LazyLibrarian API ✅ (verified from source)

Source: `https://gitlab.com/LazyLibrarian/LazyLibrarian/-/raw/master/lazylibrarian/api.py`
(GitLab is canonical; the GitHub repo mirrors it). Default port **5299**.
Docs: <https://lazylibrarian.gitlab.io/api/>

- **Style:** `GET http://host:5299/api?apikey=<32-char>&cmd=<command>&<params>` — a single
  CherryPy endpoint. **Not REST.** No versioned path, **no `X-Api-Key` header** — the key is a
  query/form param only.
- **Auth:** `hmac.compare_digest` against `API_KEY` or **`API_RO_KEY` (a read-only second
  key)**. The key must be exactly 32 chars or the API returns `503 Invalid API key`.
- 🚩 **Error envelope: `{"Success": false, "Data": "", "Error": {"Code": …, "Message": …}}`
  returned with HTTP 200.** You must inspect `Success`, not the status code.
- Success for simple commands is the literal string `OK`; long-running commands return `OK`
  immediately and run in a background thread (optional `&wait` to block).
- Params must be `urllib.quote_plus`-encoded.
- **184 commands** in `cmd_dict`; `cmd=help` returns the full list as HTML.
- **Relevant to source tagging:** `listProviders, listNabProviders, listRSSProviders,
  listTorrentProviders, **listIRCProviders**, **listDirectProviders**, addProvider, showCaps`
  — **so LazyLibrarian recognises IRC and direct as first-class source categories**, which is
  why UsArr's `protocol` vocabulary is `usenet | torrent | irc | direct | manual | unknown`.
- **Maturity:** actively maintained (`api.py` is 3,241 lines, modern f-strings,
  `hmac.compare_digest`). But it is untyped, undocumented per-command, and returns HTTP 200
  for errors. **Budget significantly more integration effort per feature than for a Servarr
  app.**

### 1.13 Adjacent services — what a plugin contract must accommodate

| Service | Port | Auth | Style | Notes |
|---|---|---|---|---|
| **SABnzbd** | 8080 | `apikey` **query only** | `GET /api?mode=…&output=json` | Modes: `queue, history, addurl, addfile, pause, resume, config, version, fullstatus, get_cats`. No header auth → the key ends up in URLs and logs. 📄 |
| **NZBGet** | 6789 | HTTP Basic | **JSON-RPC** `POST /{user}:{pass}/jsonrpc` | `listgroups, history, append, status, config, version, editqueue`. ⚠️ **Not verified from primary source.** |
| **qBittorrent** | 8080 | `POST /api/v2/auth/login` → **`SID` cookie**; newer builds also Basic/Bearer | REST-ish, form-encoded | **Stateful session — needs a cookie jar and re-login on 403.** 📄 |
| **Transmission** | 9091 | Basic optional | JSON-RPC `POST /transmission/rpc` | 🚩 **Must handle `409 Conflict` + `X-Transmission-Session-Id` retry.** The handshake is mandatory on first call and after session rotation. 📄 |
| **Deluge** | 8112 / 58846 | `POST /json {"method":"auth.login"}` → cookie | JSON-RPC `/json` | Requires a `daemon.login`/`web.connected` dance. ⚠️ **Not verified.** |
| **Bazarr** | 6767 | **`X-API-KEY`** header, or `?apikey=` / form | Flask-RESTX under `/api`, Swagger shipped | ✅ verified in `bazarr/api/utils.py::authenticate`. Namespaces: `badges, episodes, files, history, movies, providers, series, subtitles, system, webhooks, plex, jellyfin`. 🚩 **Keys off *Arr local ids (`sonarrSeriesId`, `radarrId`) — so Bazarr rows join via the *Arr instance's local id, not an external id.** |
| **Jellyfin** | 8096 | `Authorization: MediaBrowser Token=…, Client=…, Device=…, DeviceId=…, Version=…` | REST + OpenAPI | API keys = admin, no user context; user tokens = scoped. |
| **Emby** | 8096 | `X-Emby-Token` / `api_key` | Jellyfin lineage, diverged | Separate plugin; auth naming and some routes differ. |
| **Plex** | 32400 | `X-Plex-Token` + `X-Plex-Client-Identifier` | XML by default; `Accept: application/json` for JSON | Discovery via plex.tv. |
| **Recyclarr** / **Configarr** | — | uses each *Arr's key | CLI/YAML | Writes TRaSH custom formats + quality profiles. **UsArr must treat custom formats as externally managed and never clobber them.** |
| **Huntarr / Fetcharr / Cleanuparr / Decluttarr** | various | each *Arr's key | web apps | Loop `MissingEpisodeSearch`/`MoviesSearch` or drive `/queue` DELETE. Absorbable, but phase 2. |

**The plugin contract must therefore span:** auth ∈ {header-key, query-key, basic,
cookie-session, bearer, token-with-client-identity}; transport ∈ {REST/JSON, JSON-RPC,
query-string RPC, XML}; error signalling ∈ {HTTP status, **HTTP 200 + `Success:false`**, HTTP
200 + error text}; session lifecycle ∈ {stateless, login→cookie, **409-challenge-retry**};
realtime ∈ {SignalR, webhook, poll-only}; ID space ∈ {global external id, **instance-local int
id only**}.

### 1.14 Gotchas table — Track 01

| # | Gotcha | Impact | Verified |
|---|---|---|---|
| 1 | SignalR hub is **not** CORS-enabled; `/api/vN/*` is `AllowAnyOrigin` | Must terminate SignalR server-side | ✅ |
| 2 | *Arr API key grants **full admin**, unscoped | Never ship it to a browser | ✅ |
| 3 | `imdbId` int in Prowlarr, string elsewhere | Normalise at ingest | ✅ |
| 4 | `indexerFlags` `string[]` in Prowlarr, `int` bitmask elsewhere | Normalise | ✅ |
| 5 | Webhook `tags` are **labels**, REST `tags` are **ints** | Separate deserialisers | ✅ |
| 6 | Webhook `eventType` is **PascalCase** | Parser must not camelCase | ✅ |
| 7 | `eventType:"Download"` has **two body shapes** | Discriminate on `episodeFile` vs `episodeFiles` | ✅ |
| 8 | History `data.Protocol` is an **int**; REST `protocol` is a **string** | Two code paths | ✅ |
| 9 | `downloadFolderImported` **drops** Indexer/Protocol/Guid/InfoHash | Join to `grabbed` on `downloadId` | ✅ |
| 10 | Prowlarr grab cache = **30 min** | Releases expire; don't persist as grabbable | ✅ |
| 11 | Prowlarr returns **HTTP 200 + error body** on query-limit hits | Treat search failure as soft; read `disabledTill` | 📄 ([#1416](https://github.com/Prowlarr/Prowlarr/issues/1416), [#1022](https://github.com/Prowlarr/Prowlarr/issues/1022), [#2635](https://github.com/Prowlarr/Prowlarr/issues/2635)) |
| 12 | List endpoints are **unpaged**; no sparse fieldsets anywhere | Stream the JSON; 10k movies ≈ 30–80 MB | ✅ + [Radarr#5246](https://github.com/Radarr/Radarr/issues/5246) |
| 13 | Production instances **don't serve the OpenAPI spec** (`IsDebug` guard) | Ship the specs | ✅ |
| 14 | `UrlBase` rewrites **every** path incl. `/signalr` and `/ping` | Probe with and without a path segment | ✅ |
| 15 | `ReturnHttpNotAcceptable = true` | Must send `Accept: application/json` or get 406 | ✅ |
| 16 | Provider POST/PUT must round-trip the whole `fields[]` | Partial updates rejected | ✅ |
| 17 | Path names differ subtly: Sonarr `/importlistexclusion` vs Radarr `/exclusions`; Sonarr-only `/languageprofile`, `/wanted/*/{id}`; `/localization/{id}` vs `/localization/language` | Per-app path maps | ✅ |
| 18 | Prowlarr has **no** `/queue`, `/wanted`, `/calendar`, `/rootfolder`, `/qualityprofile`, `/customformat` | Don't assume a universal surface | ✅ |
| 19 | Apps serve HTTPS on `SslPort` with usually **self-signed** certs | Per-connection TLS toggle, not a global flag | ✅ |
| 20 | API keys aren't URL-safety validated (`#` breaks SignalR) | URL-encode; warn on non-alphanumerics | 📄 |
| 21 | **No rate limit on the *Arr's own API**; the bottleneck is CPU/SQLite serialising the object graph | Cache aggressively; invalidate via SignalR, don't poll | ✅ |

### 1.15 Track 01 — explicitly unverified

1. `GET /api` exact response body — endpoint exists in every spec; implementation not located.
2. Complete list of valid `/command` `name` values per app.
3. `HistoryResource.data` key sets — typed `object` in the spec; the listing is conventional.
   *(Partly closed by Track 05, which read `HistoryService.cs` directly — see §5.2 there.)*
4. SignalR message-name lists for Radarr/Lidarr/Readarr/Prowlarr.
5. Whisparr v2 "Eros" vs v3 API differences and its 2026 maintenance status.
6. NZBGet, Deluge, Tdarr APIs — general knowledge, not primary docs.
7. Bookshelf / Bindery / BookLore — maintenance status, API shape, Readarr-contract fidelity.
8. Env-var names for injecting API keys.
9. Exact Lidarr/Radarr `Quality.source` enum members.
10. Current Radarr/Lidarr/Prowlarr version numbers as of Aug 2026 (the GitHub releases API was
    blocked in the research environment; only Sonarr 4.0.19.2997, 2026-08-04, came back, and
    from a **secondary** source).

---

## Track 02 — Prior art

### 2.1 Headline findings

1. **The Overseerr/Jellyseerr era ended in February 2026.** They merged into **Seerr**
   (`seerr-team/seerr`), announced 2026-02-10. `sct/overseerr` was **archived 2026-02-15**.
   LinuxServer.io deprecated its Overseerr image **2026-02-16**. ElfHosted sunset both by end
   of May 2026. **Any competitive analysis written before Feb 2026 is stale.**
   [Seerr release blog](https://docs.seerr.dev/blog/seerr-release/) ·
   [LinuxServer notice](https://info.linuxserver.io/issues/2026-02-16-overseerr/) ·
   [ElfHosted sunset](https://store.elfhosted.com/blog/2026/04/28/overseerr-jellyseerr-sunset-may-2026/)
2. **Readarr is dead**, and the books leg has fragmented into competing successors. A genuine,
   currently-unfilled gap.
3. **Nobody has built UsArr.** The closest attempts each cover ~60% and stop.
4. **The slowness is not a database problem.** Seerr users report **82k-item libraries with
   ~1.5 s initial load and zero SQLite tuning**
   ([overseerr#502](https://github.com/sct/overseerr/discussions/502)). The observed slowness
   comes from (a) live third-party API calls, (b) Node single-thread/event-loop contention
   during background jobs, (c) synchronous disk-write amplification on ZFS/NAS storage.
   **Optimise for those three, not for query planning.**

### 2.2 The comparison table

| Project | Stack | License | ★ | Status (Aug 2026) | Media types | Data model | Key strength | Key weakness |
|---|---|---|---|---|---|---|---|---|
| **Seerr** | Next.js/React 19 + Express + TypeORM + SQLite/PG, SWR | MIT | 12.3k | Active (merged Feb 2026) | Movies, TV | Local availability index; live TMDB w/ image-proxy cache | Best-in-class discovery + request UX; media-server auth | No music/books; jobs block UI; brittle watchlist sync |
| **SeerrNG** | Seerr fork | ⚠️ license unconfirmed | 19 | Active, hobby-scale | Movies, TV, **music, ebooks, audiobooks** | Seerr + Lidarr/Bookshelf adapters | **The only cross-media request app that exists** | Tiny; self-described "evolving"; provider-matching pain |
| **Ombi** | .NET Core + Angular | GPL-2.0 | 4.1k | Active — v4.55.8, 2026-03-02; UI rewrite in progress | Movies, TV, **music** | Syncs with media server | Broad early; mature permissions | Lost the UX race; **no books** ⚠️; dated feel |
| **arr-dashboard** | Next.js 16 + Fastify 4 + Prisma 7 + SQLite/PG, TanStack Query | MIT | 222 | Active — v2.23.0 | Movies, TV, music, books | **Polls + caches locally; does NOT proxy live** | Widest integration set; absorbs 4 automation tools; webhook-driven tagging | Admin-facing; thin discovery; no streaming; heap-pressure history |
| **MediaManager** | Python + SQLAlchemy/Alembic | AGPL-3.0 | 3.3k | Active — 1,196 commits, 69 issues | Movies, TV | Owns the whole pipeline | "One app not four" resonates hard | Reimplements the *Arrs; no music/books |
| **Mydia** | **Elixir/Phoenix LiveView** + Rust NIF | AGPL-3.0 | 778 | Active, 0.x early | Movies, TV | Local library + TMDB/TVDB; pings Plex | **Genuinely fast on weak hardware** (independently reviewed) | Early; no music/books; explicitly not a media server |
| **Reiverr** | Svelte + NestJS | AGPL-3.0 | 2.3k | **Stalled in a 2.0 rewrite** | Movies, TV | Live Jellyfin/TMDB/*Arr | Unified discovery + library + **playback**; plugin playback | Rewrite limbo; **Chromium 69** target |
| **Streamarr** | Next.js + Node + SQLite | MIT | 65 | Active | Movies, TV, music | Dashboard over services | **Authenticated internal proxies**; invite/trial system | Small; Plex-centric |
| **Homarr** | Next.js + tRPC + Drizzle + SQLite/MySQL/PG + **Redis** + WS | Apache-2.0 | 4.5k | Very active — v1.66.1, 2026-06-18 ⚠️ | N/A (dashboard) | Live integrations + Redis cache + WS push | 40+ integrations; widget registry; encrypted server-side creds; **mandatory integration test** | **All-or-nothing page load**; pings scale with app count; key loss = credential loss |
| **Petio** | React/Redux + Express + **MongoDB** | MIT | 288 | Active-ish, 101 open issues | Movies, TV | TMDB + Plex | Simple Plex companion | Mongo dependency; thin maintenance |
| **Requestrr** | .NET Discord bot | — | — | **Original archived 2024-01-06**; fork `thomst08` live | Movies, TV, music | Bot | Zero-friction requests | Chat-only |
| **Doplarr** | Clojure Discord bot | — | — | **Archived 2026-06-15** | Movies, TV | Bot | — | Dead |
| **DroppedNeedle** | **SvelteKit**, single container | AGPL-3.0 | — | Active | **Music only** | Local library + **live MusicBrainz @ 1 rps** | Own download engine (no Lidarr); **Subsonic + Jellyfin API emulation**; deep music features | **1 rps MusicBrainz ceiling → ~50 min for 10k albums** |
| **Bookshelf** | Readarr derivative (.NET) | GPLv3 | 753 | Active fork | Books (ebook + audiobook) | *Arr-style | Keeps books alive post-Readarr | Fragmented (Bookshelf vs BookshelfNG vs Shelfmark) |
| **Pulsarr** | Node | — | — | Active | Movies, TV | Real-time (Plex Pass) or **5-min staggered polling** | Sophisticated routing + quotas | Watchlist-scoped |

### 2.3 The single most important prior-art datapoint

> **arr-dashboard's architecture, in its own words:** *"The dashboard **caches and syncs
> locally — it does not proxy live API calls.** Instead: periodic polling of *arr instances
> stores queue, library, history in local SQLite/PostgreSQL."*

**This is exactly the architecture UsArr wants, and it already exists at 222 stars and works.**
That is simultaneously validation and warning: **the replica architecture is correct and it is
not a moat.** UsArr differentiates by pairing it with discovery-grade UX, cross-media search,
and the gateway role — none of which arr-dashboard has.

### 2.4 Documented failure modes, with sources

| Failure | Project | Evidence |
|---|---|---|
| **Background jobs block the UI** — site slows during scheduled jobs *despite low CPU*, pointing at event-loop blocking or lock contention. Open 2021-08-27, **unresolved at archive** | Overseerr | [#2030](https://github.com/sct/overseerr/issues/2030) |
| "Overseerr loading times are excruciatingly slow" — **4+ minutes** for the requests page, minutes for text-only Settings, on a Synology DS918+. Filed at v1.33.0, **never answered by a maintainer** | Overseerr | [#3510](https://github.com/sct/overseerr/issues/3510) |
| "Running at an absolute crawl" — users restarting Docker every few days | Overseerr | [#3665](https://github.com/sct/overseerr/discussions/3665) |
| **All-or-nothing page load**: *"All the data has to load before anything shows — which means all the pings, icon fetching, and integration data fetch have to run fully before displaying anything."* Dashboards taking 20 s | Homarr | [answeroverflow 1372692732832645191](https://www.answeroverflow.com/m/1372692732832645191) · [1281017679507095647](https://www.answeroverflow.com/m/1281017679507095647) · [1232403284162248725](https://www.answeroverflow.com/m/1232403284162248725) · [ajnart/homarr#1778](https://github.com/ajnart/homarr/discussions/1778) · [#2235](https://github.com/ajnart/homarr/issues/2235) |
| **Health-check pings scale linearly with app count**; Docker socket mounting degrades perf measurably | Homarr | same |
| **Heap pressure** ingesting large *Arr payloads; mitigation "continues" via streaming JSON parsing and cursor pagination | arr-dashboard | v2.18+ release notes |
| **Sync-write amplification on CoW filesystems** — SQLite's many-small-writes pattern on ZFS; documented workaround is moving the DB to ext4 | Seerr | [#931](https://github.com/seerr-team/seerr/discussions/931) |
| **Fan-out search is as slow as the deadest indexer** — users *"must wait for disabled or down indexers to return an invalid response or timeout, with requests taking over 30 seconds"* | Prowlarr | [#712](https://github.com/Prowlarr/Prowlarr/issues/712) |
| **Brittle watchlist sync** — requires per-user Plex tokens; bulk TMDB failures with empty error messages; `Cannot read properties of undefined (reading 'find')` crashes | Seerr/Overseerr | [seerr#1912](https://github.com/seerr-team/seerr/issues/1912) · [#1378](https://github.com/seerr-team/seerr/issues/1378) · [overseerr#4282](https://github.com/sct/overseerr/issues/4282) |
| **Music support in PR limbo for years** — PR #1238 (Lidarr) closed 2025-11-19 in favour of #2132, still not merged to mainline | Seerr | [#1238](https://github.com/seerr-team/seerr/pull/1238) |

### 2.5 DroppedNeedle — the app Joe cited

**Found and verified.** <https://droppedneedle.com> · <https://musicseerr.com> (redirects —
"MusicSeerr is now DroppedNeedle") · `ghcr.io/habirabbu/droppedneedle:latest` ·
[repo org](https://github.com/DroppedNeedle/DroppedNeedle) · AGPL-3.0 · **SvelteKit**, single
Docker container.

**Self-hosted music request/discovery with a built-in native library and download engine, so it
does not require Lidarr.** Notably it **emulates OpenSubsonic and Jellyfin server APIs** so
Symfonium/Feishin/Amperfy/Finamp stream from it directly — the clearest existence proof for
UsArr's northbound-surface strategy. Also: four-tier file identification (MBID tags → fuzzy
text → AcoustID fingerprint → manual review), slskd/SABnzbd downloads, per-app revocable
passwords, OIDC, HIBP breach checking, encrypted API keys.

**What is actually slow about it — first-party, from <https://droppedneedle.com/docs/faq>:**

> *"Expected behaviour on a first scan. **The MusicBrainz API client is rate-limited to 1
> request per second.**"*
> *"**A 10k-album library takes roughly 50 minutes** when around 30% of files need a
> MusicBrainz lookup."*
> *"Subsequent scans are incremental and far faster."*

Their shipped mitigations are **scheduling** fixes:

> *"Searches stay responsive while a library scan is running, and the scan's MusicBrainz
> lookups now **queue behind whatever you're waiting on** instead of competing with it."*
> *"Fingerprinting scales with the host: fpcalc ran two at a time, and now runs one per core,
> up to four."*

**Diagnosis:** a modern SvelteKit UI sitting directly on a 1 req/s external API. MusicBrainz's
limit is a hard, non-negotiable upstream constraint on a donation-funded service. **The fix is
not a faster frontend; it is refusing to put an external API on the interactive path.**

> ⚠️ **Important caveat, and it should be stated to Joe:** **all** DroppedNeedle performance
> evidence is **first-party documentation.** No third-party complaint threads were found. It is
> credible — they are candid about it — but **the specific slowness experienced may differ from
> the documented MusicBrainz scan bottleneck.** Worth asking directly what felt slow.

### 2.6 What to copy, what to avoid

**Copy:** Seerr's discovery UX, request granularity, media-server OAuth + user import, local
availability index, image proxy with disk cache · arr-dashboard's poll-and-cache architecture,
webhook-driven real-time tagging, streaming JSON + cursor pagination, multi-instance model ·
Homarr's widget/integration **registry** (this is the shape of "pluggable services"),
encrypted-at-rest credentials never sent to the client, **mandatory integration test before
save**, versioned migrations from commit one · **Streamarr's authenticated internal proxy** to
embedded *Arr UIs, which dissolves the feature-parity trap, and its invite/trial/usage-limit
model · **DroppedNeedle's protocol emulation** (OpenSubsonic/Jellyfin) and per-app revocable
passwords · **Pulsarr's staggered polling** and rule-based routing with quotas · Mydia's
runtime posture: real concurrency isolation, low overhead on weak hardware.

**Avoid:** running scheduled jobs in the request-serving path (Overseerr's four-year unfixed
sin) · blocking the whole page on the slowest integration (Homarr's dominant complaint) · any
external API on the render path (DroppedNeedle's 1 rps ceiling) · waiting for a full indexer
fan-out (Prowlarr#712) · reimplementing the *Arr download/import engines (MediaManager's risk)
· starting a ground-up rewrite before v1 has won (Reiverr's stall) · chasing TV/console
platforms early (Reiverr's Chromium-69 ceiling) · one-media-server-at-a-time (Seerr's limit) ·
MongoDB (Petio's ops tax) · **an encryption key whose loss silently destroys all stored
credentials with no escrow or rotation path** (Homarr, arr-dashboard).

### 2.7 The gaps, ordered by defensibility

1. **Cross-media catalogue search — wide open.** Searching "Train Dreams" and getting the film
   *and* the novella exists in **exactly one place**: SeerrNG, at 19 stars, self-described as
   evolving. MediaTracker does cross-media *tracking* but no acquisition. **Every 1,000+ star
   project in this space is movies-and-TV-only.**
2. **Speed as the product** — the market has left it on the table. Every project surveyed has
   documented latency complaints with known fixes.
3. **Unified library as source of truth** — partially taken by arr-dashboard.
4. **Streaming/gateway in the same pane — wide open.** Reiverr tried and stalled; Mydia
   explicitly declines; Seerr never attempted it. **DroppedNeedle's protocol emulation is the
   clever trick worth copying: rather than build clients, speak the protocols existing clients
   already know.**
5. **Source tagging (usenet vs torrent) — nobody surfaces this to users.** Prowlarr knows it
   per indexer, the *Arrs know it per release, and **no frontend surveyed exposes source
   provenance as a first-class filterable attribute.** Small, distinctive, cheap.
6. **Absorbing the automation layer** — proven by arr-dashboard, but phase 2.
7. **Books, post-Readarr — a timing opportunity.** Users are re-tooling *right now*, and a hub
   that abstracts over whichever book backend they pick is more valuable in 2026 than in 2024.
   **Do not hard-code Bookshelf.**

### 2.8 Track 02 — explicitly unverified

- SeerrNG's exact license, last-commit date, CI health (19★ — verify it is alive before
  treating it as a reference implementation).
- **Ombi's "slow/clunky" reputation** — widely repeated in comparison articles; **no primary
  complaint thread found. Do not cite performance claims for Ombi.**
- **Whether Ombi supports books** — only movies/TV/music found. **Assume no books until proven.**
- Ombi's in-repo `STANDALONE_MIGRATION_PROGRESS.md` — what migration it refers to.
- Exact last-commit/release dates for Organizr, Dashy, Flame, Heimdall.
- Reiverr's precise last-commit date (status inferred from issue activity into Jan 2026).
- Homarr's current latest release (v1.66.1 of 2026-06-18 observed; may be superseded).
- **All DroppedNeedle performance evidence is first-party** (see §2.5).
- Seerr's current TMDB **response** caching strategy (image caching is confirmed).
- Whether the merged Seerr fixed the job-blocking issue inherited from Overseerr — the archived
  issues were never resolved.
- **Not researched:** Emby specifics; Jellyfin's book/comic depth; LazyLibrarian's current
  state; Shelfmark; Kapowarr/Mylar3 (comics); SoulSync; Managarr (TUI/CLI — possibly
  interesting as an API-surface reference).

---

## Track 03 — Architecture, database, sync, performance, plugins

Track 03's own thesis, stated up front: *"The user's complaint — everything is a live API
call, so the UI feels slow — is not a UI problem. It is an architecture problem, and the fix is
a single structural decision: **UsArr is not a proxy. UsArr is a replica.**"*

### 3.1 Workload sizing **[A]**

- Typical deployment: 1–5 users, **~50k–500k rows** across all media types. A 10k-movie +
  2k-series library is ~400k episode rows — **episodes dominate**.
- Hardware floor: Raspberry Pi 4/5, an N100 mini-PC, or a Synology NAS. **1–2 GB RAM**
  available. ARM64 and AMD64 both mandatory.
- Concurrency shape is **fan-out I/O**: 20–200 concurrent outbound HTTP calls at peak during
  initial import, near-zero at steady state.
- CPU work is bounded but real: **a 10k-movie `/api/v3/movie` response is ~30–80 MB of JSON.**
- **This is not a high-QPS service.** What matters is startup time, idle memory, tail latency
  under a big import, and distribution simplicity.

### 3.2 Backend language comparison

| Criterion | **Go** | Rust (Axum) | Node/TS | Python | .NET 9 | Elixir |
|---|---|---|---|---|---|---|
| Cold start | ~10–50 ms | ~5–20 ms | ~300 ms–1.5 s | ~1–3 s | ~200–600 ms | ~1–2 s |
| Idle RSS | ~30–60 MB | ~10–30 MB | ~90–180 MB | ~80–150 MB | ~120–250 MB | ~90–150 MB |
| RSS under 10k import | GC-spiky ~150–300 MB | ~60–120 MB flat | **~400 MB–1 GB** | ~300–600 MB | ~300–500 MB | ~200–350 MB |
| Fan-out concurrency | Excellent | Excellent | Good (parse blocks) | Poor–fair (GIL) | Excellent | **Best in class** |
| True single binary | **Yes, trivially** | Yes | No | No | Yes (AOT, large) | No |
| Cross-compile ARM64 | **`GOOS/GOARCH`, zero toolchain** | Needs `cross`/zig | n/a | n/a | Ok | Painful |
| **Pure-Go/no-C SQLite** | **Yes** | n/a | native module | stdlib | — | weakest (NIF) |
| **Pure-language WASM host** | **wazero — Go-exclusive property** | wasmtime (native lib) | yes | native lib | native lib | NIF |
| Contributor accessibility | **High** | Low | Highest | Highest | Medium | **Lowest** |

**[F]** Navidrome — Go, single binary, embedded SQLite — idles at **~50 MB RAM**, versus
Jellyfin's 250+ MB; roughly *"5× lighter on RAM and 10× smaller on disk."*
([selfhosting.sh](https://selfhosting.sh/compare/navidrome-vs-jellyfin/),
[SelfHostPicks](https://selfhostpicks.com/navidrome-vs-jellyfin-music/))
**[R]** This is the single most persuasive datapoint because Navidrome is *the same shape of
application* as UsArr: Go + embedded SQLite + a compiled SPA + a background scanner + a
Pi-targeting audience.

**[F]** Three cgo-free SQLite drivers exist: `modernc.org/sqlite` (C transpiled to Go),
**`ncruces/go-sqlite3`** (real SQLite C compiled to WASM, run under wazero — *"runs the exact,
original C SQLite source, guaranteeing 100% bug-for-bug compatibility with upstream"*,
supports FTS5 and extensions), and `glebarez/go-sqlite`.
([ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3))
⚠️ **Superseded since this pass:** `ncruces/go-sqlite3` moved off wazero to the maintainer's own
`wasm2go` translator (discussion #361, 2026-03-05), and its README now lists Go and `x/sys` as its
only direct dependencies — so neither "run under wazero" nor the bit-for-bit compatibility claim
describes what UsArr will run. See ADR-0001 revision 2 and `DEVELOPMENT.md` §1.
**[F]** The cgo-free penalty is real but modest — one benchmark set puts modernc at ~75% of
`mattn/go-sqlite3`, with a 1.17×–5.84× CGO advantage depending on operation.
([sqlite-cgo-no-cgo](https://github.com/multiprocessio/sqlite-cgo-no-cgo))

**[F]** wazero is *"a zero dependency WebAssembly runtime for Go developers"* with a pure-Go
interpreter **and** a pure-Go JIT for arm64/amd64, and does not rely on CGO.
([wazero](https://github.com/wazero/wazero)) **This is a Go-exclusive property — every other
language's WASM runtime is a native library.**

**[F]** SignalR clients confirmed available: `philippseith/signalr` (Go),
`mandrewcito/signalrcore` (Python). **[R]** But the protocol subset the *Arrs use is small
enough to hand-roll in any language; this should not be a deciding factor.

**Rejections, with reasons:** **Node** — decoding a 60 MB payload on the single thread stalls
the event loop for hundreds of ms, and the fix (worker threads) reintroduces the complexity you
chose Node to avoid. **Python** — GIL serialises the parse-heavy import path; worst packaging.
**.NET** — genuinely tempting (reuse the *Arrs' DTOs and SignalR client) but inherits the exact
runtime weight self-hosters complain about, and the homelab C# contributor pool is thin.
**Elixir** — best concurrency model on the list and a beautiful fit for supervising N flaky
upstream connections (circuit breakers are nearly free with OTP). **[R]** Killed by contributor
pool size and deployment weight; *"a real loss — note it as the 'if we had 10 Elixir devs'
answer."*

> ⚠️ **A genuine conflict between Track 02 and Track 03, and it is worth recording.** Track 02
> identified **Mydia (Elixir/Phoenix LiveView)** as the empirical *speed exemplar*, with an
> independent XDA review reporting *"Performance is equally excellent, with Mydia running
> alongside minimal overhead on my fairly underpowered NAS"*, and argued that LiveView's
> preemptive per-process scheduling structurally prevents the Overseerr#2030 failure. Track 03
> acknowledged Elixir as *"the best concurrency model on the list"* and rejected it on
> **contributor pool and deployment weight**. **Resolution: Go.** Go's preemptive scheduler
> solves the same job-blocking problem, and the properties Track 03 weights — static binary,
> trivial ARM cross-compile, pure-Go SQLite, pure-Go WASM host, drive-by contributors — are
> decisive for an OSS project that must survive 24 months. The Elixir case is honest and
> should be recorded as the road not taken, not papered over.

### 3.3 Frontend

**[F]** *"Svelte compiles to vanilla JavaScript with no runtime, producing smaller bundles and
updating the DOM more efficiently"*; *"SvelteKit consistently delivers significantly smaller
bundles compared to equivalent Next.js setups"*; *"React consistently ranks in the middle tier
for raw rendering performance, while Svelte ranks among the fastest."*
([Strapi](https://strapi.io/blog/svelte-vs-react-comparison),
[devMorph](https://www.devmorph.dev/blogs/sveltekit-vs-nextjs-16-performance-benchmarks-2026))

**[F]** *"SvelteKit's 34× download lead over SolidStart (2.02M vs 59k weekly) means
battle-tested patterns for auth, forms, and deployment adapters already exist… most React
component libraries are not compatible [with Solid]."*
([SoloDevStack](https://solodevstack.com/blog/solidjs-vs-svelte-solo-developers))

**[R] SSR is close to worthless for UsArr.** The app is private (no SEO), the server is 1 ms
away on the LAN, and sessions are long-lived so the SPA shell amortises instantly. What matters
is that navigation between library → detail → search is **zero-network** after first load.

**[R] The honest caveat:** *"React is the defensible choice and I would not argue with a team
that picks it."* The strongest reason is **TanStack Query**, which solves — with years of
edge-case hardening — exactly UsArr's problems: stale-while-revalidate, optimistic mutations
with automatic rollback, request deduplication, background refetch. If the team judges
client-side cache coherency to be the risky part, buying that maturity is worth ~80 KB.
**Rejected: SolidStart** (34× smaller ecosystem is disqualifying for a project needing drive-by
contributors); **HTMX + Go templates** (a 10k-item ~~virtualized~~ **large** grid with instant
client-side filter/sort *is* a rich client-state problem, and every HTMX interaction is a network
round trip — **rejected on requirements, not taste**).
⚠️ **Supersession note, 2026-08-16:** UsArr no longer virtualizes by default (ADR-0029), so the word
"virtualized" here is struck. The research record is preserved rather than rewritten; the rejection
stands on the remaining argument, which the client-side prefix index independently proves.

### 3.4 SQLite

**[F]** *"Running SQLite directly within the application process on the same server eliminates
network overhead entirely"*, and WAL *"reduces p99 latency by 30–60% for more than 2 concurrent
writers."*
([Micrologics](https://micrologics.org/blog/sqlite-in-production-optimizing-wal-mode-concurrency-and-vfs-layers-for-low-latency-app-servers))

**[F]** WAL lets readers and writers proceed independently, but *"SQLite still enforces a
single-writer model… if a second connection attempts to write while a write transaction is
active, SQLite immediately returns SQLITE_BUSY."* **[F]** Critically: *"busy_timeout allows
retrying on SQLITE_BUSY, but this doesn't work if a read-only transaction is upgraded to a
write transaction using BEGIN DEFERRED."*
([ADHDecode](https://adhdecode.com/articles/sqlite/sqlite-concurrent-writes-locking/),
[tenthousandmeters](https://tenthousandmeters.com/blog/sqlite-concurrent-writes-and-database-is-locked-errors/))
→ **Hence the two-pool + `BEGIN IMMEDIATE` discipline.**

**[R] Requiring Postgres would be a product failure.** Self-hosters want one container, one
volume, no sidecar; every additional required service costs a meaningful fraction of the user
base. SQLite is also *faster* here — zero network hops. **[F]** Turso's concurrent-writes work
targets the single-writer limitation
([Turso](https://turso.tech/blog/beyond-the-single-writer-limitation-with-tursos-concurrent-writes)),
but **[R]** UsArr does not need it: writes are batched through one writer goroutine by design,
so contention never arises. **DuckDB rejected** — an OLAP engine for an OLTP problem.

### 3.5 Search

**[F]** FTS5's default tokenizers do prefix matching, **not** fuzzy — *"there is no fuzzy
search built-in; word matches only."* **[F]** FTS5 ships a `trigram` tokenizer that is *"good
at generalized substring matching… makes substring and LIKE-style matching fast, at the cost of
a larger index."* ([SQLite FTS5 docs](https://sqlite.org/fts5.html),
[davidmuraya](https://davidmuraya.com/blog/sqlite-fts5-trigram-name-matching/))

**[F]** In head-to-head testing of a misspelled query (`suprman`), *"only Typesense and
Meilisearch properly handled misspellings"* — **but** *"in-memory SQLite is by far the fastest"*
on raw FTS latency. ([fts-benchmark](https://github.com/VADOSWARE/fts-benchmark),
[Supabase](https://supabase.com/blog/postgres-full-text-search-vs-the-rest))

**[F]** RRF over FTS5 plus a second ranker is an established, production-shaped SQLite pattern:
*"hybrid retrieval systems combine BM25 keyword search via FTS5, dense vector search via
sqlite-vec, and Reciprocal Rank Fusion in pure SQL"*, and **RRF *"doesn't attempt to compare
search method scores directly — it uses them purely for `row_number()` ranking within each
set."***
([Simon Willison](https://simonwillison.net/2024/Oct/4/hybrid-full-text-search-and-vector-search-with-sqlite/),
[PatentLLM](https://media.patentllm.org/blog/database/hybrid-rag-200-lines))

**[F]** Bleve has a long-open issue to *"improve fuzzy search to use levenshtein automaton"* —
its fuzzy path is not automaton-backed the way Tantivy's is
([blevesearch/bleve#112](https://github.com/blevesearch/bleve/issues/112)). **[F]** Tantivy's
`FuzzyTermQuery` supports edit distance ≤2 and delivers *"up to 10× faster indexing than Lucene
with sub-millisecond query latencies"*
([tantivy docs](https://docs.rs/tantivy/latest/tantivy/query/struct.FuzzyTermQuery.html)) —
**Rust only**, and a real reason to prefer Rust if the team were already fluent.

**[F]** Meilisearch *"uses LMDB, a memory-mapped database on disk… dataset size is not limited
by available RAM"* and shows *"~30% lower RAM footprint for identical datasets compared to
Typesense"*; Typesense *"stores indexes entirely in memory"* and *"requires at least 2 vCPUs."*
([Meilisearch comparison](https://www.meilisearch.com/docs/resources/comparisons/typesense),
[Typesense requirements](https://typesense.org/docs/guide/system-requirements.html))
→ **Meili over Typesense for a Pi/NAS, and optional either way.**

**[F]** `sqlite-vec`'s `vec0` is *"a flat in-memory index that performs an exact nearest-neighbor
scan; for corpora up to a few hundred thousand vectors this is plenty fast (single-digit
milliseconds)"* ([ceaksan](https://ceaksan.com/en/hybrid-search-fts5-vector-rrf)) — so it would
work at this scale. **[R]** Deferred because it needs an embedding model, which is a runtime
dependency or a remote call, both violating zero-dependency.

### 3.6 The work/edition modelling evidence

**[F]** FRBR defines four levels — Work → Expression → Manifestation → Item (WEMI).
([Code4Lib](https://journal.code4lib.org/articles/16491),
[IFLA](https://www.ifla.org/files/assets/cataloguing/isbd/OtherDocumentation/resource-wemi.pdf))
**[F]** **BIBFRAME collapses Work + Expression into one class.** **[F]** **Wikidata went the
other way** — it *"didn't use the FRBR terms 'expression' or 'manifestation' due to boundary
definition difficulties, instead collapsing those two layers into one 'edition' concept… a
two-layer framework consisting of work and edition."* **[F]** **Open Library likewise uses a
two-layer Work/Edition model**, and critically: ***"if a work has been adapted or retold, it is
considered a unique work, different from the original."***
([Code4Lib OpenWEMI](https://journal.code4lib.org/articles/18412),
[OpenLibrary FAQ](https://openlibrary.org/help/faq/editing))

**[F]** For cross-media linkage Wikidata uses **`P144` "based on"** and its inverse **`P4969`
"derivative work"**; `P144` *"is especially used for adaptations — works a certain work is based
on"*, and community consensus models adaptation via `P144` rather than as a genre.
([P144](https://www.wikidata.org/wiki/Property:P144),
[P4969](https://www.wikidata.org/wiki/Property:P4969))

**[F]** Wikidata maintains Open Library ID as **`P648`** and is pursuing Open Library
integration, so the bridge exists.
([P648](https://www.wikidata.org/wiki/Property:P648),
[OpenLibrary Wikidata integration](https://docs.openlibrary.org/projects/wikidata-integration.html))

> **Two independent, mature bibliographic systems converged on "an adaptation is a distinct work
> with a link."** That is strong evidence, and it is the basis of the `work_relation` design.

### 3.7 Images — the real bottleneck

**[R]** Quantified: a 60-item viewport at 500×750 posters is ~60 requests × 80–150 KB =
**5–9 MB per screenful**, dwarfing the ~30 KB of JSON. *"If you optimise anything else first,
you optimised the wrong thing."*

**[F]** Overseerr built exactly this solution: *"a new image caching system that directly caches
images from TMDB instead of letting NextJS re-optimize them. Cached images are stored in your
mounted config folder, which means the cache will persist between updates"*, and *"they proxy
images through the server, which lets them limit the rate at which they pull images from TMDB to
avoid hitting their new CDN/API rate limits."*
([Overseerr v1.31.0](https://newreleases.io/project/github/sct/overseerr/release/v1.31.0),
[jellyseerr#1160](https://github.com/Fallenbagel/jellyseerr/issues/1160))

**[F]** Jellyfin's cautionary tale: thumbnail generation *"lacks limitations and attempts to
generate thumbnails for thousands of images concurrently… CPU and memory usage spike to extreme
amounts, potentially triggering out-of-memory errors"*
([jellyfin#9795](https://github.com/jellyfin/jellyfin/issues/9795)), and it caches full-size
source images without rescaling, *"resulting in hundreds of MB of unnecessary storage"*
([jellyfin#9069](https://github.com/jellyfin/jellyfin/issues/9069)).

**[F] ThumbHash vs BlurHash:** ThumbHash *"encodes more detail in the same space, also encodes
the aspect ratio, gives more accurate colors, and supports images with alpha"*; benchmarks show
ThumbHash **encode ~864 µs / 1,015 B allocated vs BlurHash ~446 ms / 33 MB**, and **decode
503 µs vs 6.5 ms**. **[F]** *"Decode runs on every page load for every user, while encode runs
once at upload, so it's important to optimize for decode"* — ThumbHash wins on the axis that
matters. ([ThumbHash](https://evanw.github.io/thumbhash/),
[DatoCMS](https://www.datocms.com/blog/introducing-thumbhash-the-compact-placeholder-with-alpha-support))

### 3.8 Deployment conventions

**[F]** LinuxServer.io images *"use s6-overlay to manage processes, and expose the same
PUID/PGID system with a /config volume for persistence."* The internal user `abc` is created
with UID/GID 911, and the init *"chowns exactly three paths on startup: /app, /config and
/defaults."*
([LinuxServer docs](https://docs.linuxserver.io/general/understanding-puid-and-pgid/),
[Korben](https://korben.info/en/linuxserver-docker-images-homelab.html))
**[R]** Match these even though UsArr is not an LSIO image — violating them generates
permission-hell support tickets.

**[R]** Port **8383** is unclaimed in this ecosystem (Sonarr 8989, Radarr 7878, Lidarr 8686,
Readarr 8787, Prowlarr 9696).

**[R] Migrations: `goose`** with embedded SQL. Plain SQL a contributor can read; embedded in the
binary so there is no version skew; runs at startup inside a transaction **after an automatic
pre-migration backup**. Alternatives considered: `atlas` (excellent diffing, heavier concept),
`golang-migrate` (fine, clunkier embedding).

**[R] Backup: nightly `VACUUM INTO`** — the correct way to snapshot a live SQLite DB;
transactionally consistent, no file locking, compacted output.

### 3.9 Plugins

**[F]** HashiCorp go-plugin *"is the plugin system used by HashiCorp tooling for over 4 years…
in use by Terraform, Nomad, Vault"*, where *"plugin authors implement interfaces as if running
in the same process"* over local gRPC — but it *"is designed to work over a local reliable
network."* ([go-plugin](https://github.com/hashicorp/go-plugin))

**[F]** By contrast, *"Wasm plugins are very easy to install by placing the Wasm binary in a
location where the host will find it, and can be secure as the plugin is executed in a sandbox
without access to the local filesystem and network by default."*
([knqyf263/go-plugin](https://github.com/knqyf263/go-plugin))

**[F] Helm 4 explicitly ruled out HashiCorp's go-plugin "as it wasn't as secure as the new Wasm
plugin model"** — a significant, recent datapoint from a major Go project making exactly this
choice. ([Helm HIP-0026](https://helm.sh/community/hips/hip-0026/))

**[F]** Prowlarr's search maps cleanly onto a `Searcher` interface, and is *"compatible with
standard Newznab/Torznab compatible endpoint/parameters"* — but **[F]** the Prowlarr team has
stated *"an aggregate multi-indexer endpoint will not be added"*, so **per-indexer fan-out with
client-side merging is UsArr's job.**
([Servarr wiki: Prowlarr Search](https://wiki.servarr.com/prowlarr/search))

### 3.10 Track 03 — open questions

1. **[A→verify]** Does Lidarr/Readarr's `/history/since` behave identically? *(Existence now
   confirmed for all five from the shipped specs — see Track 01 §1.6. **Behaviour parity
   remains unverified.**)*
2. **[R]** Sonarr `develop` has an **API v5** alongside v3. Version negotiation must be
   first-class from day one, not retrofitted.
3. **[R]** The Wikidata enrichment pass needs a rate limit and an offline strategy. *(Track 05
   answered this: ship a precomputed subset — §5.3 there.)*
4. **[R]** `STRICT` tables require SQLite ≥ 3.37. `ncruces/go-sqlite3` bundles a recent build,
   so this is safe — but note it if the driver changes.
5. **[R] Biggest schedule risk is the write path.** Everything else is mechanical; the intent
   log's failure and reconciliation semantics are where subtle bugs will live. **Budget real
   test time, including a fault-injection harness that randomly 500s and times out a fake *Arr.**

---

## Track 04 — Streaming, playback, identity, security

**Verdict as delivered:** *"Delegate playback. Own identity, the unified hub, and the
request/discovery experience. UsArr should be a playback broker and identity plane, not a
transcoder."*

> **Note on scope:** Joe subsequently narrowed this further — UsArr serves no media bytes as a
> core capability and has **no in-app player at all**. Track 04's analysis is unchanged and
> still decisive; the conclusion simply became stronger. The northbound protocol surfaces
> (§4.3 below) went **up** in priority as a result.
>
> ⚠️ **Note added 2026-08-21 — the narrowing is correct, and this is the citation it was written
> without.** *"Joe subsequently narrowed this further"* named no statement, and this digest's own opening
> promises the opposite: *"Where the architecture asserts a fact, the citation is here."* **It is now
> cited, at both ends, and *"subsequently"* turns out to be literally accurate rather than a hedge — the
> two endpoints are twenty-six minutes apart.**
>
> **Before**, in the project's opening message — `cmsg_01S5UQT5yPAMR4PFkxyLGSj9RXhqeDFjfm9b6JCFa9jyab`,
> **2026-08-16T00:09:09Z** — he wrote: *"we need to be able to stream content from it."* **After** —
> `cmsg_01S5UQT5yPAMR4PFkxyLGSj9V6REyb72SabqnJajSc3NUB`, **2026-08-16T00:35:12Z**, a reply in that same
> thread — he wrote: *"we don't need the capability to stream or do anything else. just have a unified
> source that plugs into your favorite players."* **That second sentence is the one
> [`DECISIONS.md`](./DECISIONS.md) ADR-0018 quotes**, whose `Status:` reads *"Added on the project
> owner's input"* — and the ADR transcribed him **byte-exactly**; what it never wrote down was where the
> words came from. **Both addresses are recorded here so neither has to be rediscovered.**
>
> 📌 **The narrowing was prompted, and he exceeded the prompt. Both halves are recorded, because omitting
> either would be the same defect this note exists to correct.** At **00:26:22Z** an agent in that thread
> recommended delegating video playback to Jellyfin, citing its FFmpeg CVE history; a coordinator post at
> **00:27:24Z** explicitly invited him to object *"before it writes the architecture docs"*. **He did not
> object — and he went further than the recommendation**, because *"or do anything else"* is broader than
> video playback and is what closes off the music and book player too. **It is still his narrowing**, and
> its scope is his rather than the recommendation's.
>
> ⚠️ **One consequence stated so it is not later mistaken for a discrepancy — and it is decided elsewhere,
> in ADR-0018 Revision 2, which is not in question here and is not reopened by this note.** Read strictly,
> *"or do anything else"* is broader than what UsArr does: **rev 2 carves back byte-carrying for audio and
> ebooks**, because OpenSubsonic and OPDS have one acquisition verb and no safe hand-off, and **that
> carve-back is reasoned at its own site, by an ADR, on protocol grounds.** This note neither ratifies nor
> questions it. ✅ **And this page needs no correction on that axis**, because its own wording — *"serves no
> media bytes **as a core capability**"* — already tracks rev 2's phrasing rather than his stricter
> sentence. **The reason for recording it at all is that a reader who spots the gap and finds no
> acknowledgement will reasonably wonder whether anyone noticed; someone did, and this is where it
> points.** ✅ **Nothing here is weakened.** The narrowing stands on §4.1 below — the costing to reach
> *"worse than Jellyfin"*, the transcode decision matrix — and on [`ARCHITECTURE.md`](./ARCHITECTURE.md)
> §1.4, where an in-app player, a video transcoder and any FFmpeg dependency are **permanent refusals**.
> **None of that is reopened, and no argument for reopening it appears here.**

### 4.1 The cost of building a transcoder

| Component | Effort to do *adequately* | To do *well* |
|---|---|---|
| HLS/CMAF packaging + progressive MP4 | 2–4 weeks | 6 weeks |
| Device profile / capability negotiation engine | 6–10 weeks | ongoing forever |
| FFmpeg pipeline builder + safe arg construction | 4–8 weeks | + permanent security burden |
| Hardware accel across 6 backends | 8–16 weeks | requires a patched ffmpeg fork |
| Subtitle handling (extract/convert/burn-in) | 4–6 weeks | 10+ weeks |
| Session mgmt, throttling, seeking, GC | 4–6 weeks | 8 weeks |
| **Total** | **~7–12 engineer-months to reach "worse than Jellyfin"** | multi-year |

**The protocol layer is the easy part.** fMP4/CMAF has collapsed the HLS-vs-DASH choice — one
CMAF encode feeds both — though **Safari/iOS has no MSE for DASH**, so Apple devices require
HLS. *"That decision takes an afternoon. This is not where the cost is."*

**The cost is the transcode decision matrix.** From
[Jellyfin's codec-support matrix](https://jellyfin.org/docs/general/clients/codec-support/):
H.264 8-bit is the only universally safe video codec; **H.264 10-bit has no hardware decoder**
except Apple Silicon and Rockchip; **DTS has no browser support at all**; AC3 and EAC3 fail in
Firefox; **MKV is unsupported in Firefox** and must be remuxed to TS; **PGS bitmap subtitles
only work in MKV, otherwise burn-in** — and the docs are explicit that **"burning in subtitles
is the most intensive method of transcoding,"** forcing a full re-encode even when the video
would otherwise direct-stream.

**Hardware acceleration is a support nightmare, per Jellyfin's own docs**
([hardware-acceleration](https://jellyfin.org/docs/general/post-install/transcoding/hardware-acceleration/)):
six backends (VAAPI, Intel QSV, NVIDIA NVDEC/NVENC, AMD AMF, Apple VideoToolbox, Rockchip
RKMPP); Jellyfin ships **its own patched FFmpeg build** and states *"Using FFmpeg binaries
downloaded from somewhere else will result in partial acceleration"* — **you would have to
maintain a patched FFmpeg fork**; the pipeline is multi-stage and *"some of these stages cannot
be GPU accelerated due to software, hardware or driver limitations"*; Raspberry Pi HWA was
**deprecated** for insufficient maturity.

⚠️ Concurrent-transcode throughput figures (UHD 630 ≈ 4–6 × 1080p; N100 ≈ 3–4 × 4K-HDR→1080p)
are **enthusiast-blog aggregates, not vendor-certified. Order of magnitude only.** Note also
that many popular NAS units have **no usable media engine at all** — e.g. the Synology DS923+
(AMD Ryzen R1600, no iGPU).

### 4.2 The decisive argument: the CVE class ✅

Building a user-facing HTTP surface that assembles FFmpeg command lines is a **high-severity
vulnerability class**, and Jellyfin has been cut by it repeatedly:

| Advisory | Detail |
|---|---|
| [GHSA-2c3c-r7gp-q32m](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-2c3c-r7gp-q32m) (**CVE-2025-31499**, High) | FFmpeg **argument injection** via `/Videos/<itemId>/stream[.container]` and AudioController equivalents → **arbitrary file write → RCE via the plugin system.** Fixed 10.10.7. **This was a bypass of an earlier fix shipped in 10.8.13 — they got it wrong twice.** |
| [GHSA-866x-wj5j-2vf4](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-866x-wj5j-2vf4) | Argument injection in FFmpeg **codec parameters** — same class, different parameter |
| [GHSA-wg4c-c9g9-rxhx](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-wg4c-c9g9-rxhx) (CVE-2021-21402) | Unauthenticated **arbitrary file read** via `ImageByNameController`, `SubtitleController`, **`HlsSegmentController`** — path traversal in the segment server |
| [GHSA-rgjw-4fwc-9v96](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-rgjw-4fwc-9v96) (CVE-2021-29490) | **SSRF** via remote-image endpoints — unauthenticated enumeration of the private network |
| [GHSA-qcmf-gmhm-rfv9](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-qcmf-gmhm-rfv9) (Apr 2025) | **IP spoofing** → an unauthenticated attacker appears to be on LAN and can **restart the server repeatedly** on a default config |
| [GHSA-rrr6-mvwg-9pg9](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-rrr6-mvwg-9pg9) (Mar 2026) | Unauthenticated **resource-exhaustion DoS** via the Branding endpoint generating arbitrary-size images |

> **Read that table as a cost estimate.** A mature, well-staffed project with years of focus on
> exactly this problem is *still* shipping High-severity argument-injection and path-traversal
> bugs in its transcoding endpoints. **A greenfield hub with a small team will do worse. This
> alone ends the debate.**

**The line UsArr draws: UsArr may move bytes; UsArr may not build FFmpeg command lines from
user input.** (And per the final scope: UsArr does not move media bytes either, by default.)

### 4.3 OpenSubsonic — the highest-leverage integration in the report

[OpenSubsonic](https://opensubsonic.netlify.app/docs/) is a community-maintained,
backward-compatible extension of the Subsonic REST API: 100+ endpoints across ~15 categories,
plus ~28 documented extensions negotiated at runtime via `getOpenSubsonicExtensions`.

**Speaking it as a server means ~40 existing clients work on day one** — Symfonium, Amperfy,
Supersonic, play:Sub, Nautiline, NaviBeat, DSub, substreamer, Feishin and more. Servers already
speaking it: Navidrome, gonic, Ampache, LMS, Nextcloud Music, ownCloud Music.

> 🚩 **The sharpest single finding in the whole track.** Classic Subsonic auth is `u` + `t`
> (token) + `s` (salt), where `t = md5(password + salt)`. **This mathematically requires the
> server to hold the password in recoverable form.** Navidrome's own docs admit it: *"Due to
> limitations with the Subsonic API, Navidrome is unable to properly hash passwords and thus
> encrypts them instead"* — with a key that **by default ships in the source code**,
> overridable once via `ND_PASSWORDENCRYPTIONKEY` and then permanently frozen.
> ([Navidrome security](https://www.navidrome.org/docs/usage/admin/security/),
> [OpenSubsonic discussion #25](https://github.com/opensubsonic/open-subsonic-api/discussions/25))
>
> **The fix exists:** the
> [`apiKeyAuthentication` extension](https://opensubsonic.netlify.app/docs/extensions/apikeyauth/)
> lets clients pass `apiKey` instead of `u`/`p`/`t`/`s`, and **the spec states that servers
> providing API-key auth should no longer support salt/token auth.**
>
> **→ UsArr's rule: implement `apiKeyAuthentication` only. Never salt/token. Never store a
> recoverable password.** A minority of ancient clients will not work; that is strictly the
> right trade, and it lets Argon2id remain the only password storage in the system.

**Gapless and ReplayGain are client-side concerns.** The server's job is to expose `replayGain`
metadata and stream bytes cleanly. ⚠️ Client spec coverage is uneven — one source indicates
**Feishin** still targets Navidrome's internal API and the Jellyfin API rather than full
OpenSubsonic. **Use Symfonium as the reference client.**

### 4.4 OPDS and the other non-video surfaces

- **[OPDS 1.2](https://specs.opds.io/)** is Atom + Dublin Core; **[OPDS
  2.0](https://specs.opds.io/opds-2.0.html)** is JSON-LD built on the
  [Readium Web Publication Manifest](https://readium.org/webpub-manifest/) with schema.org
  vocabulary, URI-templated links, multiple collections per feed, and auth-gated acquisition
  links. **Cheap to generate; instantly unlocks KOReader, Panels, Librera, Moon+ Reader.**
- **Audiobookshelf** ([api.audiobookshelf.org](https://api.audiobookshelf.org/)): Bearer token
  from `/login`, plus **OAuth2 + PKCE SSO** via `/auth/openid` — so it can sit behind the same
  OIDC provider. Position sync via `/api/me/progress` (individual **and batch**), storing
  `currentTime`, `duration`, `progress`, `isFinished`; `/api/sessions` for cross-device.
  **Delegate audiobooks; mirror progress; do not become the source of truth for position.**
  ⚠️ Exact play-session request schema (`supportedMimeTypes`, deviceInfo) **not verified**.
- **Komga**: HTTP Basic **or `X-API-Key` header**; `KOMGA-SESSION` cookie or `X-Auth-Token`;
  OpenAPI 3.1 at `/api/v1/`; **supports both OPDS 1.2 and OPDS 2.0.** ✅
- **Kavita**: CBZ/CBR/EPUB/PDF, cross-device progress, OPDS feeds. Scales further than
  Calibre-Web (tested 50,000+ files vs ~20,000). ~~⚠️ REST API surface and auth scheme not
  retrieved~~ → **✅ superseded 2026-08-16 by Track 06: Kavita's full `openapi.json` was retrieved
  and read. Auth is an `x-api-key` header (and an API key in a *path segment* on the OPDS routes).
  Both Komga's and Kavita's API surfaces are now verified.**
- **In-browser reading** is the one place native delivery makes sense, since no backend provides
  an embeddable reader. [epub.js](https://github.com/futurepress/epub.js) is the conservative
  choice; [foliate-js](https://github.com/johnfactotum/foliate-js) covers EPUB/MOBI/KF8/FB2/CBZ/
  PDF with zero dependencies but its author warns it is *"far from complete or stable yet."*

### 4.5 Identity

**Argon2id per [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html):**

| m | t | p |
|---|---|---|
| 47104 KiB (46 MiB) | 1 | 1 |
| **19456 KiB (19 MiB)** | **2** | **1** | ← recommended default for a NAS |
| 12288 KiB (12 MiB) | 3 | 1 |
| 9216 KiB (9 MiB) | 4 | 1 |

⚠️ For a NAS-hosted app, m = 47 MiB × concurrent logins is a real memory spike. Store the full
PHC string so parameters are self-describing and upgradeable by re-hashing on login. Pepper
stored separately; rotating it forces resets. bcrypt only for legacy, cost ≥10, and **enforce
the 72-byte input limit** — if pre-hashing, base64-encode the intermediate to dodge null-byte
truncation.

**Sessions per [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html):**
opaque server-side session ID in a `HttpOnly; Secure; SameSite=Lax` cookie for web — explicitly
*do not store authentication tokens, session IDs, JWTs, or refresh tokens in `localStorage`/
`sessionStorage`.* Both idle **and** absolute timeouts. Rotating refresh tokens for devices.
Server-side state is needed anyway for logout, active-session listing, and admin revocation —
**stateless JWT actively fights that requirement.**

**Jellyfin auth** ([nielsvanvelzen's gist](https://gist.github.com/nielsvanvelzen/ea047d9028f676185832e51ffaf12a6f),
the de-facto spec since [jellyfin.org#499](https://github.com/jellyfin/jellyfin.org/issues/499)
notes the header was never properly documented):
`Authorization: MediaBrowser Token="…", Client="…", Device="…", DeviceId="…", Version="…"`.
**Deprecated and scheduled for removal in 12.0:** `X-Emby-Token`, `X-MediaBrowser-Token`,
`X-Emby-Authorization`, and the `api_key` query param.
⚠️ **Migration hazard:** 10.11.x changed which header the server accepts and broke clients —
Seerr hit this ([seerr#2361](https://github.com/seerr-team/seerr/issues/2361)).
🚩 **DeviceId gotcha: only one access token per `DeviceId` is permitted.** An app authenticating
many users against one server **must** generate a per-user DeviceId; getting it wrong silently
logs users out of each other's sessions.
⚠️ [jellyfin#14839](https://github.com/jellyfin/jellyfin/issues/14839) reports the **10.11
OpenAPI schema is invalid** (malformed 503 response headers), breaking naive codegen. →
**Generate types from the spec but hand-write the ~25 endpoints actually needed.**

**Jellyfin `UserPolicy`** is genuinely rich (`EnableAllFolders`/`EnabledFolders`,
`MaxParentalRating`, `BlockedTags`/`AllowedTags`, `AccessSchedules`, `MaxActiveSessions`,
`RemoteClientBitrateLimit`, …) — ⚠️ **but not airtight**:
[jellyfin#17014](https://github.com/jellyfin/jellyfin/issues/17014) documents that special
features do **not** respect parental controls. **→ Never treat Jellyfin policy as UsArr's only
authorization boundary.**

**The Jellyseerr model, worth copying wholesale:** proxy credentials to `AuthenticateByName`;
mint UsArr's own service key via `/Auth/Keys` rather than storing an admin password; import
users as records whose **authorization source is Jellyfin**, deliberately avoiding storing
password material; refresh external policy on every login.
([DeepWiki](https://deepwiki.com/fallenbagel/jellyseerr/5.1-jellyfin-integration))

🚩 **Plex, as of 29 April 2025, paywalled *remote* playback of personal media** — the server
owner needs a Plex Pass (annual $39.99 → $69.99; lifetime $119.99 → $249.99) or each viewer
needs a $1.99/mo Remote Watch Pass. Local streaming stays free; TV-app enforcement rolled out
through late 2025.
([9to5Mac](https://9to5mac.com/2025/03/19/plex-price-increase-remote-streaming-changes/),
[Engadget](https://www.engadget.com/entertainment/streaming/plex-starts-paywalling-remote-streaming-to-tvs-beginning-with-its-roku-app-160535590.html),
[9to5Mac, Nov 2025](https://9to5mac.com/2025/11/27/plex-paywall-for-remote-streaming-now-being-enforced/))
→ **Plex is a migration source and secondary integration, not the primary target.**

**Permissions:** Overseerr/Seerr use a **bitfield** with `hasPermission()`. **UsArr should not
copy it** — more media types × more verbs than Overseerr means a 64-bit field runs out and
becomes unreadable. Named permission strings, roles as bundles, per-media-type grants, quotas
per `(user, media_type, window)`, and library visibility in UsArr's **own** table.

**OIDC landscape:** **Authelia** is now an **OpenID Certified™ OP** in **open beta**, with PKCE
S256 mandatory — ⚠️ **and its access tokens are opaque by default, so you must use the
introspection endpoint, not local JWT validation.**
([Authelia](https://www.authelia.com/integration/openid-connect/introduction/))
⚠️ One retrieved article claiming *"Authelia is a forward-auth gate, not an OAuth2/OIDC issuer"*
is **out of date and contradicted by Authelia's own docs.** Authentik is "increasingly the
default recommendation"; Keycloak is heaviest; **Pocket ID is ⚠️ unverified — test
compatibility, do not design around it.**

🚩 **Trusted-header auth is a footgun.** Navidrome learned this: its reverse-proxy auth requires
`ND_REVERSEPROXYWHITELIST`, a CIDR allowlist, and it explicitly did **not** support proxy auth
on the Subsonic endpoint for a long time
([navidrome#2557](https://github.com/navidrome/navidrome/issues/2557)).

⚠️ **Passkeys only work for the web UI.** TV/Roku/native clients cannot do WebAuthn. **Never
design a passkey-only system.**

### 4.6 SSRF — per [OWASP SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)

UsArr is genuinely **Case 1** (a fixed set of known integration types) even though URLs are
user-supplied, which permits allowlisting. Key controls, verbatim from the guidance: resolve the
hostname yourself and **use the output value of the resolution as the IP to compare against the
allowlist** (closing the DNS-rebinding window); retrieve both A and AAAA records; **disable
redirect following** (redirects are the standard bypass); use a battle-tested IP parser, since
encoding bypasses (`0x7f.1`, `2130706433`, `[::ffff:127.0.0.1]`) defeat naive string checks and
**libraries genuinely differ** — Apache Commons Validator resists all of them, .NET's
`IPAddress.TryParse` does not.

### 4.7 Client reach — the decisive practical argument

Where people actually watch: **TVs and TV boxes** first, then phones, then desktop browsers.
A web-only frontend does not serve the primary surface, and **Roku has no usable browser at
all.** Building that matrix natively means Kotlin/Leanback, Swift, **BrightScript/SceneGraph**
(Roku's own language and toolchain), Tizen `.wgt`, webOS `ares-*` — a multi-year, multi-team
commitment *on top of* the transcoding work.

**Jellyfin already has it**: Web, Android, Android TV, [Swiftfin](https://github.com/jellyfin/Swiftfin)
(iOS/tvOS), Kodi, Roku, Tizen, webOS, plus community clients like [Moonfin](https://moonfin.io/).
⚠️ Caveat: the official Tizen client generally requires sideloading. **~40+ Subsonic clients and
the OPDS reader ecosystem come free the moment UsArr speaks those protocols.**

### 4.8 Track 04 — unverified

| Claim | Status |
|---|---|
| Concurrent-transcode counts per GPU | Enthusiast blog aggregates. Order of magnitude only. |
| Pocket ID's OIDC conformance and homelab share | **Did not verify.** Test empirically. |
| Exact Jellyfin 10.11 header-behaviour change semantics | Inferred from the gist + seerr#2361. **Verify against a live 10.11 server.** |
| Audiobookshelf's exact play-session request schema | HLS + direct-play modes confirmed; **field names not verified.** |
| ~~Kavita's REST API surface and auth scheme~~ | **Cleared 2026-08-16 (Track 06).** The full `openapi.json` was retrieved; auth is `x-api-key`, with the key in a path segment on OPDS routes. |
| Whether Jellyfin permits CORS for direct browser access to stream URLs | **Unverified** — determines proxy-vs-direct for any web playback. |
| Feishin's current OpenSubsonic coverage | One source says Navidrome-internal + Jellyfin API only; may have changed. |
| Overseerr bitfield permission flag names | Confirmed as a bitfield; individual flags not enumerated. UsArr is not copying it anyway. |

---

## Track 05 — Metadata providers, ID cross-referencing, entity resolution, tagging

### 5.1 The Train Dreams chain, executed live ✅

Every lookup below was run against production endpoints on 2026-08-16.

**Five distinct Wikidata items share the label "Train Dreams"** — a perfect illustration of the
disambiguation problem: ✅

| QID | Label | Description |
|---|---|---|
| `Q85810391` | Train Dreams | 2011 novella by Denis Johnson |
| `Q126086662` | Train Dreams | 2025 film directed by Clint Bentley |
| `Q137480492` | Train Dreams | 2025 film score by Bryce Dessner |
| `Q137993803` | Train Dreams | Song from the 2025 film |
| `Q131428569` | Train Dreams | **Wikimedia disambiguation page** — filter via `P31 = Q4167410` |

**The novella `Q85810391`** ✅ (via `Special:EntityData/Q85810391.json`): `P31` = `Q7725634`
(literary work) · `P50` author = `Q340016` (Denis Johnson) · `P577` = 2011-08-30 · `P7937`
form = `Q149537` (novella) · **`P648` Open Library = `OL15916948W`** · `P8383` Goodreads work =
`2046778` · `P1085` LibraryThing = `1690724` · `P2671` Google KG = `/g/11bykbrm_8` · `P8632`
Book Marks = `train-dreams` · `P1411` nominated for Pulitzer Prize for Fiction.

> **Critical structural finding:** the novella carries **no `P4969` (derivative work)** pointing
> at the film. **The adaptation edge exists only in the film → book direction.** Any
> implementation that starts from a book and naïvely reads its statements finds nothing. **You
> must query the inverse.** ✅

**The film `Q126086662`** ✅: `P31` = `Q11424` (film) · **`P144` based on = `Q85810391`** ←
the edge · **`P345` IMDb = `tt29768334`** · **`P4947` TMDB = `1241983`** · `P57` director =
`Q131863949` (Clint Bentley) · `P58` screenwriter = `Q340016`, `Q113443047` · `P86` composer =
`Q4980674` (Bryce Dessner) · `P577` 2025-01-26 (Sundance), 2025-11-07 (Netflix US) · `P2047`
duration 102 min · `P1258` RT = `m/train_dreams` · `P1712` Metacritic = `movie/train-dreams` ·
`P1874` Netflix = `82020378`.
**TMDB `1241983` verified live** — `themoviedb.org/movie/1241983` resolves to *Train Dreams
(2025)*, dir. Clint Bentley. ✅

⚠️ **Data-quality note:** `P58 screenwriter` includes `Q340016` (Denis Johnson), which looks
like a Wikidata artifact — Bentley and Greg Kwedar wrote the screenplay. **Lesson:
creator-overlap scoring must tolerate noisy roles.** Do not treat "author == screenwriter" as
impossible, but do not weight it as strongly as an explicit `P144`.

**Open Library work `OL15916948W`** ✅: title *"Train dreams"* (**lowercase 'd' —
normalization matters**), author `/authors/OL38799A`, subjects incl. Railroad construction
workers. **No link back to Wikidata in the OL record** — the mapping is one-directional in the
data; reverse it via Wikidata's own index.
`/works/OL15916948W/editions.json` → **9 editions** ✅, including:
- `/books/OL34486604M` — *Train Dreams*, ISBN-13 **`9781847086624`**, 2013 (Granta UK)
- `/books/OL45133917M` — *Train Dreams*, ISBN-13 `9783499237706`, 2006 (German)
- `/books/OL62068093M` — ***Sonhos e Comboios***, ISBN-13 `9789896412913`, 2012 (Portuguese)

> **The Portuguese translation is the argument for the `edition` layer in one example. Pure
> title matching would never find *Sonhos e Comboios*.** Only the OL Work→Edition edge or the
> Wikidata item does.

**The soundtrack `Q137480492`** ✅ is `P31 = Q482994` (album) but at research time carried
**only `P31`** — no `P406` (soundtrack release) edge from the film, no MusicBrainz
release-group ID. **So the film↔soundtrack link for this title cannot be made through Wikidata
today**; fall back to a MusicBrainz release-group search on `"Train Dreams" + Bryce Dessner`.

**The two lookup directions, both verified:** ✅

```
A. Film → book:  tmdbId 1241983
     → QID via CirrusSearch: /w/index.php?search=haswbstatement:P4947=1241983&ns0=1
       → Q126086662                                                          ✅ verified
     → P144 → Q85810391 → P648 → OL15916948W → editions.json → 9 editions

B. Book → film:  ISBN 9781847086624 → OL edition → OL work OL15916948W
     → QID via haswbstatement:P648=OL15916948W → Q85810391                   📄
     → INVERSE SPARQL:  SELECT ?a WHERE { ?a wdt:P144 wd:Q85810391 }
       ✅ VERIFIED — returns exactly {Q126086662, "Train Dreams", film,
                                      tt29768334, 1241983, 2025}
     → TMDB 1241983 → Radarr add/lookup
```

> **That inverse query — `?x wdt:P144 wd:<book>` — is the core primitive of UsArr's cross-media
> search. Everything else is caching, fallback, and UI.**

> ⚠️ **CONFLICT BETWEEN TRACKS, RESOLVED.** Track 03's schema sketch illustrated this same
> example with TMDB `1156593`, IMDb `tt13444824`, OL work `OL15170152W`, ISBN `9780374281144`.
> **Those values are wrong.** Track 05 executed the lookups live and independently confirmed
> the TMDB page resolves. **Track 05's IDs are authoritative.** Track 03's were illustrative
> placeholders and must not be copied into code or fixtures.

### 5.2 Provider comparison table

| Provider | Domain | Auth | Rate limit | License / ToS for self-hosted OSS | ID scheme |
|---|---|---|---|---|---|
| **TMDB** | film/TV | API key (v3) or Bearer (v4) | ⚠️ **~40 req/s per IP is a forum claim, not in the ToS** — the ToS only says "no excessive bandwidth" | ✅ Free for **non-commercial**; commercial needs a signed agreement. **Mandatory TMDB logo + the exact sentence "This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB."** **Cannot cache >6 months.** Cannot be used as an image host. Cannot "cloak or conceal your identity". **Names ML training/validation as commercial use.** | `tmdbId` int; `/find` by external source; `/movie/{id}/external_ids` returns `imdb_id` **and `wikidata_id`** 📄 |
| **TheTVDB v4** | TV/film | Bearer JWT (+ per-user PIN) | ⚠️ unpublished | ✅ **Not free.** Either a negotiated commercial contract, **or** "user-supported" — every one of *your* end users must hold a paid TheTVDB subscription and supply a PIN (⚠️ ~$12/yr cited secondhand) | `tvdbId` int |
| **IMDb** | film/TV | — | — | **No free public API.** IDs are a lingua franca only; map via TMDB `external_ids`, Wikidata `P345`, or OMDb. Bulk `datasets.imdbws.com` files are ⚠️ non-commercial-only | `tt0000000` |
| **Trakt** | film/TV | OAuth2 + client id | ~1 write/s; ~1000 GET / 5 min 📄 ⚠️ | Free, per-user OAuth. Good for watch state; **weak as a primary source — it mirrors TMDB** | slug/id + tmdb/imdb/tvdb on every object |
| **OMDb** | film/TV | API key | Free tier **1,000 req/day** 📄 | Third-party scrape of IMDb. Ratings only; not a base layer | imdbID |
| **TVmaze** | TV | none | **≥20 calls / 10 s per IP**; edge-cached hits don't count 📄 | Free; ⚠️ CC BY-SA for user-contributed data. **Good free fallback when TVDB is off the table** | `tvmazeId`; carries `thetvdb` + `imdb` in `externals` |
| **AniList** | anime/manga | none (public GraphQL) | **90 req/min**, `X-RateLimit-*` headers, 429 + 1-min timeout; not granting increases 📄 | Free. **Best-in-class anime source** | AniList id; `idMal` |
| **AniDB** | anime | client registration | **~1 req / 2 s short-term, 1 / 4 s sustained; aggressive caching mandatory, bans are common** 📄 | Very restrictive. Only worth it for episode-level anime mapping | aid |
| **MusicBrainz** | music | none (**UA required**) | ✅ **1 req/s per IP**; over → HTTP 503 **on 100% of requests, not just the excess** | **CC0 core data — but supplementary data AND the Live Data Feed replication packets are CC BY-NC-SA 3.0** ⚠️ (corrected 2026-08-16; the earlier cell called the Feed the unencumbered escape hatch from 1 req/s, which it is not — it is non-commercial-licensed). **`User-Agent: App/version ( contact )` is MANDATORY.** MusicBrainz also asks explicitly that applications **not poll** for changes and that calls be made at **random intervals**, not on a cron | MBID (UUID) for artist/release-group/release/recording/work |
| **Cover Art Archive** | album art | none | ✅📄 **no rate limiting currently in place** | Internet Archive backed; keyed by MBID | MBID |
| **Discogs** | music releases | token or OAuth | **60 req/min authed, 25 unauthed** 📄 | Free, UA required. Strong for physical release/label data, weak for canonical identity | release/master id |
| **Last.fm** | music | API key | ⚠️ **no numeric limit is published** — the ToS says limits are set and enforced *"in our sole discretion"* (corrected 2026-08-16; the earlier "≤5 req/s per IP averaged over 5 min 📄" carried a primary-source marker and is **not** in the ToS — it is forum folklore) | ⚠️ **Non-commercial use only** (ToS §3.1–3.2). Good for play counts/similarity; **name-keyed not ID-keyed — a real weakness** | artist/album names |
| **ListenBrainz** | listens | optional token | header-driven | CC0, MBID-native. The MetaBrainz answer to Last.fm | MBID |
| **api.lidarr.audio** | music | none | n/a | 📄 **A cache proxy in front of MusicBrainz run by Servarr**, with documented multi-hour propagation lag and repeated real-world 503s / schema-change breakage. **Do not depend on it.** | MBID |
| **Open Library** | books | none (UA advised) | ✅ **1 req/s anonymous; 3 req/s with an identifying UA** | Free. ✅ **"Do not use our APIs for bulk download"** — use the monthly dumps. ⚠️ exact license not fully read | OLID: `OL…W` work, `OL…M` edition, `OL…A` author; ISBN lookup |
| **Google Books** | books | API key | ⚠️ "fair use" quota | Good ISBN coverage; **mediocre identity model — volume IDs are not works** | volumeId |
| **Goodreads** | books | — | — | 📄 **DEAD.** Stopped issuing keys 2020-12-08; blanket 403s by late 2025. **This is why Readarr was archived.** Still useful as Wikidata property `P8383` | goodreads id |
| **Hardcover** | books | Bearer, per-user | 📄 **60 req/min, 30 s timeout, max query depth 3** ⚠️ | GraphQL; currently the most viable Goodreads successor. **Per-user token** ⚠️ commercial terms unverified (docs returned 403 to automated fetch) | book/edition id |
| **ISBNdb** | books | API key | per-minute + daily | 📄 **Paid, $10–$300/mo.** Only if ISBN coverage blocks | ISBN |
| **BookBrainz** | books | none | ⚠️ | MetaBrainz, CC0, 📄 **still alpha** with small coverage. Watch, don't depend | BBID |
| **Audnexus** | audiobooks | none | ⚠️ | Aggregates Audible; what Audiobookshelf uses. 📄 Known reliability complaints spawned **AudiMeta**; **AudiobookDB** positioned as successor | Audible ASIN |
| **Comic Vine** | comics | API key | 📄 **200 req/resource/hour** + velocity detection | GameSpot/Fandom-owned; ⚠️ restrictive ToU, not read | CV id |
| **Metron** | comics | HTTP Basic | ✅📄 **20 req/min burst, 5,000/day**; supports `If-Modified-Since` → 304 and `modified_gt` incremental sync | Open, community-run, far friendlier than Comic Vine. ⚠️ **Limits were reduced in early 2025 — re-read before launch** | Metron id |
| **Wikidata** | universal | none (UA advised) | ✅ `api.php` **rate-limited me within a handful of sequential requests**; `Special:EntityData/{QID}.json` and `query.wikidata.org/sparql` both reliable; SPARQL timeout 60 s 📄 | ✅ **CC0 — no attribution obligation, no commercial restriction, no cache limit. The only provider with genuinely unencumbered terms.** | QID |

Sources: [TMDB ToU](https://www.themoviedb.org/api-terms-of-use) ·
[TMDB external_ids](https://developer.themoviedb.org/reference/movie-external-ids) ·
[TVDB v4 key FAQ](https://support.thetvdb.com/kb/faq.php?id=81) ·
[MusicBrainz rate limiting](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting) ·
[MusicBrainz Live Data Feed](https://musicbrainz.org/doc/Live_Data_Feed) ·
[Cover Art Archive API](https://musicbrainz.org/doc/Cover_Art_Archive/API) ·
[Open Library APIs](https://openlibrary.org/developers/api) ·
[Discogs API](https://www.discogs.com/developers) ·
[AniList rate limiting](https://docs.anilist.co/guide/rate-limiting) ·
[AniDB HTTP API](https://wiki.anidb.net/HTTP_API_Definition) ·
[TVmaze API](https://www.tvmaze.com/api) ·
[Hardcover API](https://docs.hardcover.app/api/getting-started/) ·
[Metron best practices](https://metron-project.github.io/blog/api-best-practices) ·
[Comic Vine API](https://comicvine.gamespot.com/api/) ·
[Audnexus](https://github.com/laxamentumtech/audnexus) ·
[Servarr Lidarr metadata troubleshooting](https://wiki.servarr.com/lidarr/metadata-troubleshooting) ·
[Goodreads API retirement](https://developers.slashdot.org/story/20/12/17/1522242/goodreads-is-retiring-its-current-api-and-book-loving-developers-arent-happy) ·
[Wikidata database download](https://www.wikidata.org/wiki/Wikidata:Database_download)

### 5.3 Wikidata adaptation coverage — measured, not guessed ✅

All from live SPARQL `COUNT(*)` queries, 2026-08-16:

| Query | Count |
|---|---|
| Films (`P31=Q11424`) with a TMDB ID (`P4947`) | 241,618 |
| Films with a `based on` (`P144`) statement | **15,360** |
| Films with **both** TMDB ID and `P144` | **14,443** (94% of adaptations are TMDB-resolvable) |
| Films with `P144` whose source work has an Open Library ID | 6,134 |
| TV series (`P31=Q5398426`) with `P144` | 5,314 |
| All `P4969` derivative-work statements, any type | 34,673 |
| Items with an Open Library ID (`P648`) | 508,194 |
| Items with a MusicBrainz release-group ID (`P436`) | 243,634 |
| Items with an IMDb ID (`P345`) | 1,190,531 |

> **This is the single most decision-relevant table in the whole research corpus.**
>
> - **The adaptation graph is small and clean.** ~15k film adaptations, ~5k TV, ~35k
>   derivative-work edges. **The entire cross-media edge set fits in a SQLite table well under
>   a million rows. Materialize it at build time and ship it. No live SPARQL on the hot path.**
> - **Coverage is excellent for famous works, poor for the long tail.** ~6.1k book→film pairs
>   with an OL ID means UsArr resolves *Dune*, *No Country for Old Men*, *Train Dreams* — and
>   misses most mid-list adaptations. **The fuzzy tier and human-confirm UI are not optional.**
> - **94% of `P144` films carry a TMDB ID**, so once you have the edge you can land it in Radarr.

**Operational notes** ✅: use `Special:EntityData/{QID}.json` (CDN-cached, reliable) for
single-entity fetches, **not `api.php`/`wbgetentities`** (rate-limited within a handful of
requests). `query.wikidata.org/sparql` answered every query including large aggregates.
⚠️ The **May 2025 WDQS graph split** moved scholarly articles to a separate endpoint and the
transitional full endpoint was slated to end December 2025 — irrelevant for media, relevant if
UsArr ever federates.

**Subset filter for a shipped artifact:** items with `P31` in {film, TV series, literary work,
book, album, video game, comic, manga, anime} **OR** carrying any of {`P345`, `P4947`, `P4983`,
`P648`, `P434`, `P436`, `P8383`}, plus their `P144`/`P4969` neighbours. **All Wikidata
structured data is CC0, so redistributing the subset is legal.** ✅ Tools that cut topical
subsets: [WDumper](https://ceur-ws.org/Vol-2873/paper13.pdf),
[wikibase-dump-filter](https://github.com/maxlath/wikibase-dump-filter), WDSub, KGTK.
📄 Full JSON dump was ~130 GiB compressed as of early 2024; the **truthy RDF** dump omits
qualifiers/references and is far smaller.

**Fallbacks below Wikidata:** Wikipedia infoboxes (`Based on: <novel> by <author>`) have higher
recall and much messier data — an *enrichment* job, not runtime. DBpedia's `dbo:basedOn` mirrors
the same extraction but ⚠️ its endpoint is less reliable and the data lags. **TMDB `keywords`**
tags adaptations with e.g. `based on novel or book` (⚠️ stable keyword IDs not verified) — a
cheap boolean for *deciding whether to spend* a Wikidata lookup.

### 5.4 Relationship types to model

| Edge | Wikidata property | Example | Note |
|---|---|---|---|
| adaptation (book→film/TV) | `P144` on the adaptation | Train Dreams novella → 2025 film | **Stored only on the derived item — query the inverse** |
| derivative work | `P4969` on the source | 34,673 statements ✅ | Sparse but explicit; union both directions |
| franchise / series | `P179`, `P8345` | MCU, Discworld | Groups many works of many types |
| film collection | TMDB `belongs_to_collection`; `P179` | LOTR trilogy | TMDB's is high quality for films |
| soundtrack | `P406` ✅ | film → album | Sparse; MB release-group search is the fallback |
| translations/editions | Open Library Work→Editions ✅ | *Sonhos e Comboios* | **Never inferrable from title** |
| same character/universe | `P1080`, `P674` | Sherlock Holmes | Good for "more like this", too loose for identity |
| game→TV / comic→film | `P144` again | The Last of Us, Watchmen | Same mechanism; source `P31` differs |

**Disambiguation rules that actually work:**
1. **Never group on title alone.** Title equality is a candidate *generator*, never evidence.
   *The Wire* (TV) vs *The Wire* (album) vs *Wire* (band). *It* (1986 novel) → *It* (1990
   miniseries) → *It* (2017 film) is a real chain; *It* (1927 Clara Bow film) is **not** related.
2. Require at least one of: shared external ID, a `P144`/`P4969` edge, or (creator overlap
   **AND** temporal plausibility).
3. **Temporal plausibility:** an adaptation is published after its source, typically 0–60 years.
   `film.year < book.year` is near-fatal to the hypothesis.
4. **Novelizations invert the arrow** (Star Wars, Alien). Detect via year ordering and each
   side's `P31`.
5. Filter disambiguation pages (`P31 = Q4167410`) and list articles.

### 5.5 Entity resolution technique

**Normalized key**, deterministic: NFKD + strip combining marks → transliterate non-Latin
(**keep the original as an alias — never discard it**) → locale-invariant lowercase (⚠️ beware
Turkish dotless ı) → expand numerals both ways (`2` ↔ `II` ↔ `two`) → `&`→`and`, strip
punctuation, collapse whitespace → strip leading articles **as a second key kept alongside the
unstripped one** (*The Thing* vs *Thing* is a real distinction) → strip trailing disambiguators
(`(film)`, `(novel)`, `(2025)`) → split a `": "` subtitle into its own field rather than
deleting it.

**Blocking — never compare all pairs:**

| Blocking key | Recall | Cost |
|---|---|---|
| exact `norm_key` | low, free | index lookup |
| `norm_key_nostop` | +articles | index lookup |
| first 4 chars + year bucket | good | index range scan |
| **trigram set overlap (FTS5 `tokenize='trigram'`)** | **best general-purpose** | inverted index |
| metaphone / double-metaphone | catches misspellings + transliteration variants | index lookup |
| sorted-neighbourhood, window 20 | good for near-dupes | one sort |

Union 2–3 blocking keys, **cap at ~200 candidates**, then apply expensive scorers. This is the
standard two-stage pattern: [Splink](https://www.robinlinacre.com/fast_deduplication/)
demonstrates 7M records deduped in minutes and states plainly that **blocking rules are the
primary determinant of runtime**; the
[trigrams-then-Jaro-Winkler rescoring pattern](https://concepttocloud.com/news/entity-resolution-in-postgres-trigrams-vs-embeddings)
is exactly right for UsArr's scale. Reference implementations worth reading rather than
reinventing: [dedupe](https://github.com/dedupeio/dedupe),
[Splink](https://github.com/moj-analytical-services/splink),
[Zingg](https://github.com/zinggAI/zingg),
[recordlinkage](https://recordlinkage.readthedocs.io/).

**Scorers, and when each earns its place:** trigram similarity → candidate generation;
Levenshtein/`editdist3` (SQLite `spellfix1`, which supports per-character cost tables so you can
make `ph`↔`f` cheap) → typo tolerance on short titles; **Jaro-Winkler → final scoring, because
it is prefix-weighted and people get the beginning of a title right**; double metaphone →
transliteration variants; token-set/token-sort ratio → word-order and subtitle noise;
**length/frequency priors → suppressing "It", "Her", "Us", computed as IDF over your own title
corpus** — *"short high-frequency titles must be penalized or the review queue floods."*
Embeddings are real value at real cost — **optional**, local, Tier-3 only, and ⚠️ prefer
embedding Wikidata/local text over TMDB overviews given TMDB's ML clause.

### 5.6 Source provenance ✅

**`DownloadProtocol` enum, byte-identical in the Prowlarr, Sonarr and Radarr shipped specs:** ✅

```json
{"enum": ["unknown", "usenet", "torrent"], "type": "string"}
```

> **Protocol is never inferred — it is asserted by the indexer definition and carried through
> the whole pipeline.** Filename/magnet/NZB heuristics are only needed for manual imports.

**The gotcha: history loses provenance at import.** Read from
`src/NzbDrone.Core/History/HistoryService.cs` on `develop` ✅:

```
grabbed.data:                Indexer, NzbInfoUrl, ReleaseGroup, Age, AgeHours, AgeMinutes,
                             PublishedDate, DownloadClient, DownloadClientName, Size,
                             DownloadUrl, Guid, TvdbId, TvRageId, ImdbId,
                             Protocol (as the INT value of the enum), CustomFormatScore,
                             SeriesMatchType, ReleaseSource, IndexerFlags, ReleaseType,
                             [ReleaseHash], [TorrentInfoHash]

downloadFolderImported.data: FileId, DroppedPath, ImportedPath, DownloadClient,
                             DownloadClientName, ReleaseGroup, CustomFormatScore,
                             Size, IndexerFlags, ReleaseType
```

> 🚩 **Missing from the import event: `Indexer`, `Protocol`, `Guid`, `TorrentInfoHash`.**
> ⇒ **The provenance join is `grabbed.downloadId == downloadFolderImported.downloadId`.**
> 🚩 **And `Protocol` in history `data` is the INTEGER form of the enum
> (`0=unknown, 1=usenet, 2=torrent`), not the string the REST resources use.** A real footgun. ✅

**Always-available fallback:** `DownloadClient` in the import event is the client
*implementation type* (`Sabnzbd`, `NzbGet`, `QBittorrent`, `Deluge`, `Transmission`,
`RTorrent`), and **implementation type alone determines protocol unambiguously**. So even
without the grab record you can recover `source:usenet|torrent`. ✅

**Store the raw `release_title` verbatim, forever** — every parsed field is re-derivable if the
parser improves; the raw name is not recoverable once discarded. **Never overwrite provenance on
upgrade** — insert a new row, giving free upgrade history. **Manual imports get
`protocol='manual'`; do not launder `unknown` into `torrent`.**

### 5.7 Release-name parsing

| Parser | Lang | Scope | Notes |
|---|---|---|---|
| [**GuessIt**](https://github.com/guessit-io/guessit) | Python | movies + TV | Rule-based (rebulk), ~40 property types. Most feature-complete general parser |
| [**parse-torrent-title**](https://github.com/clement-escolano/parse-torrent-title) | JS/TS | torrent titles | Small, fast, extensible handler model |
| [**PTT / parsett**](https://github.com/dreulavelle/PTT) | Python | torrent titles | Heavily extended port; used by Riven/Comet-class stacks |
| [**anitomy**](https://github.com/erengy/anitomy) | C++ | anime filenames | The reference for `[Group] Title - 01 [1080p][HEVC].mkv` fansub conventions |
| **Sonarr `Parser.cs`** | C# | TV | ✅ **158 `new Regex(` declarations** in a 78 KB file, plus `QualityParser.cs` (~28 KB). **A decade of hardening against real indexer output** |
| **Radarr `Parser.cs`** | C# | movies | ✅ **43 `new Regex(`** (28 KB) — movies are simpler, no season/episode combinatorics |

> **Do not write a parser from scratch.** Port or vendor a general parser and **keep Sonarr's
> regex corpus as a test oracle** — its 158 TV regexes encode daily shows, multi-episode ranges,
> absolute numbering, anime specials, `S01E01E02`, `1x01`, `2024.01.15`, part-numbering,
> mini-series. **Even if you don't use their code, mine their test fixtures** — Sonarr and
> Radarr have extensive parser test suites with thousands of real release names.

**Reliably extractable (>95% on scene/P2P):** resolution, source, video/audio codec, channels,
release group, season/episode in standard forms, year, `PROPER`/`REPACK`, HDR flags.
**Unreliable, needs external metadata:** the **title itself** when it contains a year
(*2012*, *1917*, *Blade Runner 2049*) or an episode-looking number (*Se7en*); year-vs-season-pack
ambiguity (`Show.2024.1080p`); edition/cut (~30 spellings, often absent when it should be
present); language/dub-vs-sub (`MULTi`, `VOSTFR` are inconsistently applied — **the file's audio
tracks are ground truth, not the name**); anime episode numbering; whether a "Complete Series"
pack is actually complete; scene-vs-P2P origin (Sonarr's `sceneSource` ✅ is a heuristic).

> **Design consequence: parse for tagging and quality decisions, and treat parsed title/year as
> a query into the metadata provider, never as identity.** Store `parsed` fields alongside
> `provider_confirmed` fields and **never let the parser overwrite a confirmed ID.**

### 5.8 Tag-system prior art

| System | Model | Lesson |
|---|---|---|
| **Sonarr/Radarr** ✅ | `TagResource {id, label}` — flat. But `TagDetailsResource` reveals what tags *do*: `delayProfileIds, importListIds, notificationIds, restrictionIds, indexerIds, downloadClientIds, autoTagIds, seriesIds`. Plus a full **`AutoTaggingResource`** with `specifications` (implementation + fields + `negate` + `required`) and `removeTagsAutomatically` | **(1) Tags are join keys for policy, and must attach to config objects, not just media. (2) Rule-driven auto-tagging with negation already exists — copy `AutoTaggingSpecification` almost verbatim** |
| **Hydrus** 📄 | Namespaced tags, **tag siblings** (A means the same as B), **tag parents** (*"parents are virtual; the parent is not actually added to every file"*), multiple tag services with precedence | **The best tag model in the space. Steal siblings and virtual parents.** Virtual parents are crucial: `type:audiobook` should imply `type:book` at query time without duplicating rows |
| **Calibre** 📄 | Fixed columns + arbitrary typed **custom columns** (`#mycol`) with a template language | Users *will* want typed user fields. Namespaced tags with an optional type give most of this for a fraction of the complexity |
| **Komga / Kavita** 📄 | Read embedded metadata first (`ComicInfo.xml` in CBZ, OPF in EPUB) — it **overrides** filename parsing. Separate concepts for tags, genres, collections, read lists | **Keep tags (arbitrary), genres (provider vocabulary), and collections (ordered, curated) as different things.** Conflating them is a common mistake |
| **Jellyfin** 📄 | Distinct `Genres[]` and `Tags[]` arrays | Same lesson |

**Fast filtering in SQLite:** the `(tag_id, entity)` covering index makes "all items with
`source:usenet`" an index-only scan. For AND-of-N, `INTERSECT` of N index scans is usually
faster than `GROUP BY … HAVING COUNT(DISTINCT …) = N` — **benchmark both. Selectivity ordering
(start from the rarest tag, using a denormalised `tag_count`) is the single highest-leverage
optimization.** `ANALYZE` after bulk import — the planner is materially better with stats for
multi-index intersections.

***Arr tag import:** into `sonarr-tag:<label>` / `radarr-tag:<label>` namespaces, with a
one-click "promote to UsArr tag" creating an alias, **rather than silently reinterpreting user
data**. If ever writing back, **only write from the `*-tag:` namespace** — never push a
namespaced UsArr tag into a flat-label system.

### 5.9 Track 05 — must re-verify before shipping ⚠️

- **TMDB's actual rate limit.** The ToU is silent; "~40 req/s" is a forum post. **Implement
  adaptive backoff on 429 rather than a hard-coded number.**
- **Whether TMDB will grant a bundled key** for a self-hosted OSS app — requires emailing
  `tmdb@themoviedb.org`. **Until then, user-supplied keys.**
- **Whether embedding TMDB overviews counts as "training/validating an ML system"** under their
  commercial clause. Prefer Wikidata/local text.
- **TheTVDB's current subscription price** (~$12/yr cited secondhand) and whether any OSS
  exemption exists.
- **Open Library's exact data license** — `/developers/licensing` not read in full.
- **Hardcover's terms for third-party self-hosted apps** — docs returned 403 to automated
  fetch; the 60 rpm / depth-3 figures are secondhand.
- **MusicBrainz Live Data Feed access terms** for a self-hosted OSS project (free MetaBrainz
  account vs supporter tier).
- **Audnexus vs AudiMeta vs AudiobookDB** — the audiobook provider landscape is actively
  churning. **Pick late.**
- **Metron's rate limits changed in early 2025** (reduced due to load) — re-read before launch.
- **`haswbstatement` CirrusSearch** worked via the web search UI ✅ but the JSON `api.php` form
  was rate-limited during testing. Verify under normal traffic, or just use SPARQL.
- **Comic Vine ToU** for redistribution/caching in a self-hosted app — not read.

---

## Track 06 — The six media types: books, music, comics, libraries, multi-type IA

**Added 2026-08-16.** The six-type expansion produced five deep research passes — user-defined
libraries, multi-type information architecture, books/ebooks/audiobooks, music, comics/manga — and
the facts below are now load-bearing for `ARCHITECTURE.md` §6.1, §6.5, §7.1a, §8.5, §16, §17 and for
ADR-0026 through ADR-0032. **Until this track existed, those were assertions in an ADR rather than
citations in the evidence base**, which is the inversion *"verify, don't assert"* exists to prevent.
Every ✅ / ⚠️ / 🔍 marker below is carried from the research pass that produced it, not re-graded.

### 6.1 Catalogue sources — what they can and cannot supply

| Finding | Marker |
|---|---|
| **None of Navidrome, Audiobookshelf, Komga or Kavita has a "changed since" endpoint.** `komga-openapi.json` has **zero** parameters matching `since\|modified\|updated\|changed` on any path — 20 `lastModified` *fields* and a generic Spring `sort`. `kavita-openapi.json` exposes `sortByLastModified` on exactly two endpoints, `GET /api/Collection` and `POST /api/ReadingList/lists`, and on **none** of the Series, Volume or Chapter endpoints | ✅ read from the vendored specs |
| **Whether Komga accepts `sort=lastModified,desc` on its series list.** Spring `Pageable` sort properties are not enumerated in the spec and the DTO field name may not be the entity property name | ⚠️ **could not be verified; the highest-value thing to probe against a live instance.** ARCHITECTURE §16 makes it a day-one spike |
| **Kavita's identifier matching is a paid subscription feature**, so on a free instance every external id is null | ✅ |
| **Komga exposes no external identifiers at all** — `SeriesMetadataDto` carries only `links`, i.e. whatever a user typed | ✅ read from the spec |
| **Kavita's `LibraryType` members `Manga` / `Comic (Flexible)` / `Comic (ComicVine)` are filename-parsing modes over one identical entity tree** | ✅ |
| **Audiobookshelf: "Most actions in the server apply to the currently selected library, including browsing and searching"**, and an author with series in two libraries shows as **two separate author entries** | ✅ upstream documentation |
| **Navidrome's `LibrarySelector` returns `null` at `!length \|\| length === 1`** | ✅ read from source |
| **Navidrome does not support OpenSubsonic `apiKeyAuthentication`** — not in any release, not in `master`; PRs #4022 and #5731 open | ✅ |
| Auth: Navidrome `POST /auth/login` → JWT **plus** `(subsonicSalt, subsonicToken)`; Audiobookshelf Bearer JWT **or a scoped API key with an expiry**; Komga `X-API-Key` (Basic on OPDS); Kavita `x-api-key`, **with the key in a URL path segment on OPDS routes** | ✅ |

### 6.2 Acquisition and indexers

| Finding | Marker |
|---|---|
| **Newznab: `3030` is `Audio/Audiobook` under `Audio` (3000), not under `Books`**; `7020` `Books/EBook`; `7030` `Books/Comics`; `5070` `TV/Anime`; `7010` `Books/Mags` | ✅ `NewznabStandardCategory.cs` |
| **There is no manga category in the Newznab standard at all.** `7030` is the only comics category, and Nyaa maps its `Literature` categories (`3_0`–`3_3`) to `Books` (7000), so a search filtered on `7030` returns **zero manga** | ✅ `nyaasi.yml` |
| Of 543 definitions in `definitions/v11`, **88 declare `Books/Comics`**, 288 declare some `Books` category, and **only three are comic- or manga-specific**. **GetComics — the dominant western-comics DDL source, and the only source Kapowarr searches — is not a Prowlarr indexer at all** | ✅ |
| **403 of 543 Prowlarr indexer definitions are `type: private`**, and the dedicated music trackers (Redacted, Orpheus, DICMusic, Libble) are **invite-only** — a real limitation on "runs over just Prowlarr" for music and books | ✅ |
| **Prowlarr's `SearchResource` carries only `query`, `type`, `indexerIds`, `categories`, `limit`, `offset`** — no `author`, no `title` — and `SearchController` populates only `q`/`t`/`cat`/`limit`/`offset`. Free text is the whole of what Prowlarr offers, even against an indexer advertising `book-search: [q, title, author]` | ✅ read from source |
| **Mylar3** has shipped no release since 2025-08-17; **Kapowarr's** API documentation reads *"Coming Soon"* | ✅ |
| **Lidarr writes `artist.status = 'deleted'` into its own database when its metadata server 404s** (`ArtistStatusType.Deleted` on `ArtistNotFoundException`), and exposes no health signal for the subsystem that causes it | ✅ read from source |

### 6.3 Modelling

| Finding | Marker |
|---|---|
| **MusicBrainz: a recording is *"distinct audio"*; a track is *"the way a recording is represented on a particular release"***, so the same recording is track 4 on the CD and track 6 on the reissue with a different MBID each | ✅ |
| **MusicBrainz defines no "remaster."** The *"never produced solely through copying or mastering"* phrase is from the **Recording** page; the Release Group page contains no such definition. The step to "therefore a new edition of the same work" is **UsArr's inference** | 🔍 **inference, and it is cited as a definition in ARCHITECTURE §6.1 with the marker now attached** |
| **In practice a reissue with bonus material and a changed title gets its own release group** (*"OK Computer OKNOTOK 1997 2017"*) — a different work joined by a `work_relation` edge, not a new edition. **Both paths must exist** | ⚠️ carried caveat |
| **Lidarr's `AlbumResource.artistId` is singular**, so a VA compilation lands under a synthetic "Various Artists"; **Navidrome superseded its single `ArtistID`/`AlbumArtistID` with a `Participants` model** | ✅ |
| **Lidarr's `albumCount` counts albums that passed a *metadata profile*, a user setting.** Radiohead has 386 primary-type albums on MusicBrainz, so "12 of 579" is true and useless | ✅ |
| **ComicInfo `Count` — the source of every "total issues" in the domain — is conceded by the Anansi specification itself to be unreliable:** *"The `Count` could be different on each book in a series"*; Mylar3's total comes from Comic Vine, whose own code comments that *"comicvine isn't as up-to-date with issue counts"* | ✅ |
| **Komga models no manga/comic distinction** — only `ReadingDirection` — and ComicInfo carries `Manga` as a field on a book otherwise structurally identical to a western comic | ✅ |
| **LazyLibrarian writes no row at all for a file its matcher cannot bind**; the failure lands in a local dict that produces a debug-log line and an *"N unmatched items"* banner. Match ratios documented at *"somewhere around 80% to 90%"*, with looser matching warned to *"get matches against the wrong books"* | ✅ read from `librarysync.py` |
| **LazyLibrarian GitLab #2407 — books marked ignored come back after an author rescan** | ⚠️ **no maintainer resolution; the reporter says they may be reading the wrong code** |
| **Plex cannot change a library's type after creation** | ⚠️ **a community feature request, not an official Plex statement.** Not verified against a current build, and it was carried into five documents as fact |
| **Whether `library_scope` can fully replace `instance_scope` in `search_doc` without a second column** | 🔍 **inference. Argued rather than measured; check against the first real scoped search query written** |

### 6.4 Classical music cannot be modelled, and UsArr must say so rather than fail quietly

This is the one honest-impossibility finding from the six-type research that had **no treatment
anywhere in the repository** — not in this file, not in the architecture, not in a mockup.

> ✅ *"UsArr cannot fix classical, because neither of its southbound sources models compositions."*
> — and the research's own recommendation: *"On the artist page for a composer, say what is
> missing."*

**Neither Navidrome nor Lidarr models a composition/work tier for music.** Both model
artist → album → track, where a "track" is a *recording on a release*. A classical work — Beethoven's
Op. 131, say — exists as dozens of recordings by different quartets across dozens of releases, and
nothing in either backend groups them. The composer is not the "artist" of the recording; the
performer is, or the ensemble is, or a synthetic "Various Artists" is.

**The consequence, stated plainly:** a unified catalogue over these sources renders a classical
library the way its sources do — grouped by release, with the composer appearing as one artist among
performers — and **that is not something UsArr can fix by aggregating harder.** It is the case where
a hub looks most obviously wrong to the user who owns it, and classical is not a niche.

**What follows for the design.** `work_credit(role)` (ADR-0031) makes a composer *representable* as
a credit — that is the **seam**, and ADR-0031 mentions it in exactly those terms. It is **not the
disclosure**, and reading it as one inverts the finding. UsArr models **no composition tier**, a
composer therefore renders as an artist, and recordings of one work are grouped by release rather
than by work. **Where that is visible, UsArr says so** rather than presenting the grouping as
correct — the same rule as the *"not identified"* badge (§6.4 of the architecture) and the comics
gap list. Recorded in `ARCHITECTURE.md` §6.1.

---

## Cross-track conflicts and how they were resolved

| # | Conflict | Resolution |
|---|---|---|
| 1 | **Train Dreams IDs.** Track 03: TMDB `1156593`, IMDb `tt13444824`, OL `OL15170152W`, ISBN `9780374281144`. Track 05: TMDB `1241983`, IMDb `tt29768334`, OL `OL15916948W`, ISBN `9781847086624` | **Track 05 wins.** It executed the lookups live and independently confirmed the TMDB page resolves to the correct film. Track 03's were illustrative placeholders. **Never copy Track 03's values into code or fixtures.** |
| 2 | **Backend runtime.** Track 02 makes an empirical case for Elixir/Phoenix LiveView (Mydia, independently reviewed as fast on weak hardware, with preemptive per-process scheduling that structurally prevents Overseerr#2030). Track 03 rejects Elixir on contributor pool and deployment weight | **Go.** Its preemptive scheduler solves the same problem; static binary + trivial ARM cross-compile + pure-Go SQLite + pure-Go WASM host + contributor accessibility are decisive for OSS survival. **Recorded as the road not taken, not papered over.** ADR-0001. |
| 3 | **Relation-table naming and shape.** Track 03: `work_relation(from,to,rel_type,source,confidence)`. Track 05: `work_link(src,dst,kind,confidence,evidence,status)` | **Track 03's name, Track 05's columns.** `work_relation` with `evidence` and `status` — the review inbox is unusable without being able to explain *why* two things were linked. |
| 4 | **Tag assignment shape.** Track 03 argues **against** a polymorphic `(entity_type, entity_id)` pair (forfeits FK integrity, makes the planner guess) and proposes one join table per taggable entity. Track 05 proposes a single polymorphic `entity_tag` | **Synthesis:** one `tag_assignment` table with **real nullable FK columns** per target and a `CHECK` that exactly one is set. Keeps FK integrity and planner reasoning; avoids four near-identical tables. |
| 5 | **Tag vocabulary.** Track 03: `tag(name, kind)`. Track 05: `tag(namespace, value, is_system)` with aliases and virtual parents | **Track 05.** The namespaced model is required by the brief and is strictly more expressive; Hydrus-style virtual parents solve `type:audiobook` → `type:book` for free. |
| 6 | **External IDs.** Track 01's sketch stores them as a JSON blob on the item. Track 03 and 05 both use a real table | **A real table.** A JSON blob cannot be indexed for `ix_extid_lookup(source, value)`, which is *the* hot lookup during sync. |
| 7 | **External-ID uniqueness.** Track 05: `PRIMARY KEY (scheme, value)` — "an external ID identifies exactly one work". Track 03: unique on `(source, value, work_id, edition_id)` because the same IMDb id can appear on a work *and* an edition | **Both, layered.** The full unique index permits the work/edition case; a **partial** unique index on `(source, value) WHERE work_id IS NOT NULL AND confidence >= 1.0` enforces Track 05's stronger invariant where it actually holds. |
| 8 | **Search result presentation.** Track 03 wants media-type diversity injection so film and novella appear as separate ranked rows. Track 05 wants `work_group` connected components so they collapse into one card with tabs | **Both, at different layers.** Diversity injection operates on **ranking**; grouping operates on **presentation** and is derived at query time from `work_relation` rather than stored — a stored group must be invalidated on every edge change, and the components are tiny. |
| 9 | **Where the speed budget goes.** Track 02: *"SQLite itself is not the bottleneck — don't spend the speed budget on the DB layer"* (82k items, 1.5 s, zero tuning). Track 03 specifies extensive SQLite tuning | **Not a real conflict; both agree on priority.** Track 03's tuning is cheap insurance against the *write* path (bulk import, `SQLITE_BUSY`), not an attempt to fix reads. **The budget goes to scheduling, I/O discipline, and images** — where both tracks point. |
| 10 | **Post-Readarr successors.** Track 01 names Bookshelf, Bindery, LazyLibrarian. Track 02 names Bookshelf, Shelfmark, rreading-glasses, LazyLibrarian, and supplies the better citation for Bookshelf (`pennydreadful/bookshelf`, 753★) | **Union of both, all flagged ⚠️ unverified.** Neither track verified maintenance status or API shape. **This is exactly why UsArr must not hard-code a book backend.** |
| 11 | **`work.kind` vocabulary.** Track 03: `movie, series, season, episode, artist, album, track, book, audiobook, comic`. Track 05: `movie, tv, music, book, audiobook, comic, game` | **Track 03's finer granularity, plus `game`** from Track 05 as a reserved value (Newznab category 1000/4000 emits it even if UsArr never manages it). |
| 12 | **`/history/since` availability.** Track 03 confirmed it for Sonarr and Radarr from source and marked the other three as an assumption to verify | **Closed by the cross-app path matrix built from the shipped specs: it exists in all five.** ✅ **Behaviour parity remains unverified** — probe at connect time. |
