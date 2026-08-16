# Reference — The sync engine

**Status:** designed, not implemented. **Scope:** channels 1, 3 and 4 plus the write queue are
**v0.1**; SignalR (channel 2) and the webhook receiver (4b) are **v1.0**.
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §7.

---

## 1. Channel summary

| # | Channel | Latency | Covers | Fails when | Milestone |
|---|---|---|---|---|---|
| 1 | Full import | minutes | Bootstrap, disaster recovery | Always slow | v0.1 |
| 3 | Delta poll `/history/since` | 30–120 s | Events that generate history | Only covers history-generating events | v0.1 |
| 4 | Reconciliation sweep | 6–24 h | Silent drift, out-of-band edits, deletions | Expensive | v0.1 |
| 2 | SignalR stream | < 1 s | Live changes while connected | Reverse proxy blocks WS; hub not CORS-enabled | v1.0 |
| 4b | Webhook receiver | < 1 s | Push from services without SignalR | Requires writing config into the user's \*Arr | v1.0 |

**Channel 3 is the correctness guarantee; channel 4 is the safety net; channel 2 is an
optimisation.** Correctness must never depend on SignalR — Radarr's own docs note that *"the most
common occurrence [of SignalR failure] is use of a reverse proxy or cloudflare, which needs
websockets enabled."*

> **`/history/since` only reports events that generate history** — grabbed, imported, deleted,
> upgraded, failed. It does **not** report a user toggling `monitored`, editing a quality profile,
> or renaming a root folder, and a movie *removed* from Radarr takes its history rows with it. That
> is precisely why channel 4 is not optional and why it ships in v0.1.

**Prowlarr is not a channel-3 source.** Verified from the shipped spec, `prowlarr.HistoryEventType`
is `["unknown","releaseGrabbed","indexerQuery","indexerRss","indexerAuth","indexerInfo"]` against
`sonarr.EpisodeHistoryEventType`'s `["unknown","grabbed","seriesFolderImported",
"downloadFolderImported","downloadFailed",…]`. Prowlarr has no library entities to refetch;
`indexerQuery`/`indexerRss` fire on every RSS poll of every indexer (thousands of rows per hour on a
20-indexer install); and `prowlarr.HistoryResource` has no `movieId`/`seriesId`. Use
`eventType=releaseGrabbed` only, as provenance input.

Six apps expose `/history/since`, not five: `/api/v3/history/since` in Sonarr, Radarr and Whisparr;
`/api/v1/history/since` in Lidarr, Readarr and Prowlarr (verified from the shipped specs).
⚠️ Behavioural parity across them is **not** verified — probe at connect time.

---

## 2. Channel 1 — full initial import

The \*Arr list endpoints are **unpaged and large** and there is no sparse-fieldset parameter
anywhere in these APIs. A 10k-movie library is ~30–80 MB of JSON in one response.

**Bare-array list endpoints:** `/api/v3/movie`, `/series`, `/artist`, `/album`, `/book`,
`/calendar`, `/tag`, `/qualityprofile`, `/rootfolder`, `/health`, `/indexer`.

**Endpoints that require a parent id and are therefore N calls, not one:**

| Endpoint | Required (one of) | Enforced by |
|---|---|---|
| `GET /api/v3/episode` (Sonarr) | `seriesId`, `episodeIds`, `episodeFileId` | `EpisodeController.GetEpisodes` throws `BadRequestException("seriesId or episodeIds must be provided")` |
| `GET /api/v3/episodefile` (Sonarr) | `seriesId`, `episodeFileIds` | controller |
| `GET /api/v3/moviefile` (Radarr) | `movieId`, `movieFileIds` | controller |

**The OpenAPI specs mark all of these `required: false`**, which is why a spec-derived design gets
it wrong: the constraint lives in the controller, not the schema. Verified against
`specs/sonarr.json` — `/api/v3/episode` lists `seriesId, seasonNumber, episodeIds, episodeFileId,
includeSeries, includeEpisodeFile, includeImages`, none marked required.

**Therefore the rule is not "never fetch children per-parent" — it is "fetch them per parent,
bounded":** one `GET /api/v3/episode?seriesId=N` per series, **4–8 concurrent**, jittered, behind a
per-instance token bucket, never a thundering herd. Pulsarr's staggered polling is the reference.
**Cost, budgeted rather than hidden:** the reference library's 2k series is ~2,000 round trips for
episodes alone, which dominates a TV first import.

**Mandatory implementation rules:**

- **Stream the JSON.** `json.Decoder.Token()` consuming the array element by element. Buffering
  *and* unmarshalling a 60 MB payload peaks at ~200–400 MB on a 1 GB Pi; streaming holds it near
  constant. arr-dashboard retrofitted exactly this after blowing up Node's heap.
- **Chunked transactions**, committing at `min(2000 rows, 100 ms)` — see §6 for why the wall-clock
  half matters.
- **Two-phase.** Phase A: id, title, year, external ids, poster URL. Phase B in the background:
  overview, file details, media info, FTS population. The user sees a populated grid in ~10 s
  instead of ~3 min.
- **Progress over SSE**, with real counts.
- **Accept header:** `Accept: application/json, text/plain;q=0.9, */*;q=0.1`, and parse the body as
  JSON regardless of the returned `Content-Type`. `options.ReturnHttpNotAcceptable = true` is
  confirmed in the shipped Servarr `Startup.cs`, so an unsatisfiable `Accept` is a 406. The large
  list endpoints declare three content types — verified:

  | Endpoint | Declared 200 content types |
  |---|---|
  | `radarr /api/v3/movie` | `text/plain`, `application/json`, `text/json` |
  | `lidarr /api/v1/album`, `/api/v1/track` | `text/plain`, `application/json`, `text/json` |
  | `readarr /api/v1/author`, `/book`, `/history/since` | `text/plain`, `application/json`, `text/json` |
  | `sonarr /api/v3/series`, `/api/v3/episode` | `application/json` only |
  | `radarr /api/v3/history/since` | `application/json` only |

  So `application/json` is declared-supported everywhere it matters and does **not** 406. The
  defensive header costs nothing; the connection wizard records what each instance actually
  returned.
- Response compression is on upstream — enable gzip/br.

---

## 3. Channel 3 — the delta poll

Per instance, every **60 s jittered ±20%**:

1. `GET /history/since?date=<last_delta_sync_at − overlap>`.
2. Extract the distinct affected **entity ids** from the returned records.
3. Refetch **only those** entities canonically (the history DTO and the REST DTO diverge).
4. Advance `last_delta_sync_at` to **the max timestamp actually observed**, never to `now()` —
   `now()` creates a silent gap for records written during the request.
5. Rely on idempotent upserts for the overlap window, subject to the resurrection guard in §4.

**The overlap is derived, not the constant 5 minutes an earlier draft used.** The cursor is a value
read from *the \*Arr's clock*. If that clock runs more than the overlap ahead of UsArr's, the cursor
jumps past events UsArr will never re-query, and they are lost until the sweep — which does not
detect history-only events like a failed grab at all. So:

```
on every health probe:  clock_skew_secs = parse(response.Header["Date"]) - now()
overlap = max(5 min, 2 × |clock_skew_secs| + poll_interval)
warn in the UI when |clock_skew_secs| > 120
```

**Two further unspecified behaviours, both probed at connect time rather than assumed:**

- **`date` format and inclusivity.** Every spec types it `{"type":"string","format":"date-time"}`
  and says nothing about whether the bound is inclusive, or what timezone/offset form the app
  expects. Send RFC-3339 with an explicit `Z`, and probe by issuing a query with a known-recent
  cursor and checking whether the boundary record is returned.
- **Unbounded responses.** There is **no `limit` parameter on `/history/since`** (unlike
  `/history`), so an instance offline for a week returns its entire history in one array. Apply the
  §2 streaming-parse rule.

`service_instance.last_history_id` is **not** used as the cursor and is not declared. `HistoryResource.id`
is monotonic per instance and would sidestep the skew problem entirely — that is a genuine
alternative, and it is recorded here rather than half-implemented: adopt it only after verifying
that `/history/since` accepts no id-based bound, which would mean fetching `/history?page=…&sortKey=id`
instead and paying pagination cost. Not decided; the column stays out of the schema until it is.

**Conditional requests: ⚠️ test per endpoint.** ASP.NET Core does not emit `ETag` on API responses by
default and the \*Arrs largely do not. Where absent, synthesise the benefit: hash the response body,
compare against `http_cache.body_hash`, and skip parsing and diffing on a match. You still pay the
bytes; on a Pi the parse is the expensive half.

---

## 4. Channel 4 — the reconciliation sweep, and its two guards

Every 6 h (configurable) plus on demand:

1. Fetch the full entity list per instance.
2. Left-anti-join against `service_item_link` → **rows deleted upstream**.
3. **Soft-delete with a 7-day tombstone.** An \*Arr that is temporarily empty (misconfigured root
   folder, unmounted NFS share) must not nuke the library.
4. Compare `remote_hash` → drifted rows → refetch. `remote_hash` covers only the **synced subset**;
   `sizeOnDisk` and friends churn constantly and would defeat it. Done right, a sweep touches <1%
   of rows.
5. Emit a `sync_report` row.
6. Run at low priority with a bounded rate.

**Precondition: the write-queue guard.** The sweep may correct an item toward the \*Arr only when
there is **no `write_queue` row for that work in `pending`, `inflight` or `verifying`**. Guarding
only the first two states means any write the \*Arr accepted but that has not been independently
observed is reverted by the next sweep — and with no SignalR in v0.1 there is no independent
observation channel, so that would be *every* write.

### Guard 1 — id resurrection

The \*Arrs allocate `id` from a plain integer primary key with no `AUTOINCREMENT`, so **ids are
reused after deletion**. Delete movie 842 (*Train Dreams*), add a different movie, it becomes 842.
The tombstoned link for `(instance 1, movie, 842)` still matches `ux_sil`, so a naive idempotent
upsert clears `deleted_at` and rebinds the new remote item to the old `work_id` — poster, tags,
requests, provenance and the northbound ID all now pointing at the wrong film. The 7-day tombstone,
introduced as a safety feature, is precisely the window in which this is possible.

```
on an upsert that would clear deleted_at:
    incoming = hash(remote payload's external ids: tmdbId/tvdbId/imdbId/MBID/foreignId)
    if incoming != link.remote_identity_hash:
        hard-delete the tombstoned link
        create a fresh link to the correct work
        emit sync_report{kind: "id_reused", instance, remote_kind, remote_id}
    else:
        clear deleted_at as normal
```

`remote_identity_hash` is recorded at first sight, which makes the comparison O(1) and means the
guard costs one column and one comparison per resurrection.

### Guard 2 — instance identity generation

An \*Arr restored from an older backup moves its id space **backwards**: ids UsArr has mapped now
belong to different content, and ids added after the backup point no longer exist. Under "the \*Arr
owns the truth", the sweep would (a) see drifted `remote_hash`es, refetch, and silently rewrite
titles, paths and provenance on existing `work` rows, and (b) find links with no upstream row,
tombstone them, and after seven days delete the user's tags, requests and playback state.

```
at every connect:
    fingerprint = hash(system/status.instanceName, database GUID or startTime where available)
    if fingerprint != service_instance.identity_fingerprint
       OR max(remote_id) for any kind < service_instance.max_remote_id_seen[kind]:
          set needs_reidentification = 1
          REFUSE to run the sweep for this instance
          surface a blocking banner (ARCHITECTURE §17.6)
          on operator confirmation: re-derive every link from external_id, not from remote_id,
          then bump identity_epoch and clear the flag
```

⚠️ Whether every \*Arr exposes a stable database GUID is unverified; `instanceName` plus a
monotonic-id check is the floor.

---

## 5. The write path — durable command queue

```
Client                     UsArr                        *Arr
  │ POST /api/v1/works/101/monitor                        │
  │  Idempotency-Key: 01J…   │                            │
  ├─────────────────────────►│ BEGIN IMMEDIATE            │
  │                          │  INSERT write_queue(pending)│
  │                          │ COMMIT                     │
  │  202 {command_id}        │                            │
  │◄─────────────────────────┤──── worker picks up ──────►│
  │  (UI shows a pending chip)│      PUT /api/v3/movie     │
  │                          │◄──── 202 / 4xx / timeout ──┤
  │  SSE: command.done|failed │                           │
  │◄─────────────────────────┤                            │
```

**States:** `pending → inflight → done | failed_rejected | verifying → done | failed`.

| Outcome | State | Rule |
|---|---|---|
| 2xx | `verifying` if the kind needs confirmation, else `done` | A 201 from Radarr means *queued*, not *exists consistently* |
| 4xx with a body | `failed`, `fail_reason='rejected'` | It did not land. Show the upstream error verbatim. |
| Timeout / transport error / 5xx | `verifying` | **It might have landed.** Never report failure here. |
| `verifying` exceeded 15 min | one final targeted refetch, then `done` or `failed` (`unknown`) | No state is unbounded |

**`verifying` performs a targeted refetch of the affected entity**, not a full sweep. If the entity
exists in the expected state, `done`. This is what prevents the worst UX in the whole write path:
telling the user their add failed and having the movie appear hours later with no explanation.

**Retry policy:** exponential backoff 2 s → 4 s → 8 s → 30 s → 2 m, max 6 attempts, then `failed`
with `fail_reason='exhausted'`. **Retry only idempotent-safe kinds:** `monitor`, `unmonitor`,
`tag_add`, `delete`, `star`, `rate` are safe; `add` is safe *only* because `service_item_link` is
checked for the remote id first; **`grab` is max 1 attempt** plus a manual retry button, because a
blind retry is a double download.

**Idempotency:** `UNIQUE (user_id, idempotency_key)`. A retried POST (double-click, flaky Wi-Fi,
service-worker replay) returns the existing command. A key that exists under a *different*
`user_id` returns `409` — never the other row, whose `payload` and `work_id` belong to someone
else. Northbound protocols carry no idempotency field, so the key is derived server-side:

```
idempotency_key = ULID_from(hash(user_id, client_credential_id, verb, usarr_id,
                                 floor(unix_time / 60)))
```

One rule for every surface. There is no separate `(user, item, started_at)` scheme for scrobbles.

**No rollback exists, and none is needed**, because nothing is applied locally before the backend
confirms. That is the whole point of replacing the optimistic intent log: `inverse_patch` reverted
only UsArr's local state, which meant a timeout after Radarr had committed produced a local
rollback, a failure toast, and then a reconciliation sweep re-adding the item hours later.

---

## 6. SQLite concurrency discipline

**Pragmas, on every connection:**

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;      -- safe under WAL; ~10× faster than FULL
PRAGMA foreign_keys = ON;
PRAGMA temp_store = MEMORY;
PRAGMA wal_autocheckpoint = 1000;
PRAGMA cache_size = -32000;       -- 32 MB PER CONNECTION — measured; see below
PRAGMA mmap_size = 134217728;     -- a NO-OP under this driver — measured; see below
```

**Both were pending; both are now measured on x86-64** by `make bench-rss`
(`internal/db/spike`, ADR-0001 — a 500k-row fixture through the real `internal/db` open path, process
RSS from `/proc/self/status`, one child process per pragma cell). What the run settled:

- **`mmap_size` does nothing here.** Every requested value reads back as `0`, and `PRAGMA
  compile_options` on the build under test reports `MAX_MMAP_SIZE=0` and `DEFAULT_MMAP_SIZE=0` —
  memory-mapped I/O is compiled out of the driver's SQLite. The line above is inert configuration.
  Do not tune it, and do not cite it as a reason for anything.
- **`cache_size` is per-connection.** Going from one pinned read connection to all eight pool
  readers added +16 MB at `-2000`, +60 MB at `-8000` and +196 MB at `-32000` — tracking the
  per-connection prediction (0.89–1.19× of it) rather than staying flat. **A `NumCPU*2` read pool plus
  the writer therefore pays `cache_size` × (pool + 1)**: on a 4-core box the shipped `-32000` reaches
  **~235 MB peak RSS** under a saturating read workload, against ~35 MB at `-2000`.

The shipped value is unchanged pending an owner decision; the measurement, not the default, is what
this section can now assert. **arm64 is unmeasured** — run `make bench-rss` there and add a row.

**Rule 1 — two pools.** Reads `NumCPU*2`; **writes exactly one connection.** This eliminates
`SQLITE_BUSY` **arising from concurrent writers inside the process**. It does not eliminate
`SQLITE_BUSY`, and claiming otherwise will be believed. Residual sources and their mitigations:

| Source | Mitigation |
|---|---|
| `VACUUM INTO` (nightly backup) and `ANALYZE` take their own locks | Run them from the writer, serialised with the queue |
| WAL checkpoint starvation by a long-lived reader (an SSE handler holding a snapshot) — `wal_autocheckpoint` silently fails, the WAL grows unbounded, an explicit `wal_checkpoint(TRUNCATE)` returns `SQLITE_BUSY` | A checkpoint-starvation metric and a bounded read-transaction lifetime; never hold a read txn across an SSE send |
| A second process: `usarr key rotate`, `usarr keygen`, a user running `sqlite3`, two containers on one volume | A single-instance lock file, checked at startup, naming the holder |
| `cache.db` `ATTACH`ed inside a write transaction | **Never `ATTACH` `cache.db` inside a write transaction.** It is a separate connection. |

**Rule 2 — `BEGIN IMMEDIATE` on every write transaction.** `busy_timeout` does not rescue a
deferred read transaction that upgrades to a write; that path returns `SQLITE_BUSY` immediately.

**Rule 3 — a priority scheduler in front of the writer.** Both the bulk importer and the
interactive write path funnel through the one writer connection. A 5,000-row `BEGIN IMMEDIATE`
batch with FTS inserts, `external_id` upserts and rollup updates is comfortably 200 ms–2 s on a Pi,
and every user write submitted during an import queues behind it — which is exactly the "background
jobs stall the request path" failure the architecture exists to avoid. Therefore:

```
importBatch:  commit at min(2000 rows, 100 ms wall clock)
writer queue: two lanes — interactive (write_queue commands, user edits) and bulk (import,
              sweep, rollup flush). The interactive lane is drained first at every batch
              boundary. Bulk work never holds the writer for more than one batch.
```

The performance budget states the write ack as measured **during a concurrent full import**,
because that is the case that matters and the uncontended number is not interesting.

**Rule 4 — batch sync writes**, never 400k single-row commits.

**Rule 5 — `ANALYZE` after bulk import.** SQLite's planner is materially better with stats for the
multi-index intersections tag filtering depends on.

**Rollup flush.** Child writes set `work.rollup_dirty = 1` on the ancestor in the same transaction;
a flush every 250 ms re-aggregates the dirty set once. Never re-aggregate per child write — a
series has up to ~300 children and an import generates ~400k child events.

⚠️ **The NAS case:** SQLite's many-small-writes pattern causes severe write amplification on ZFS and
other CoW filesystems; Seerr users hit this and the documented workaround is moving the DB to ext4.
WAL plus batching mitigates it; it does not eliminate it. Document it.

---

## 7. Deferred: channel 2 (SignalR) and 4b (webhooks)

Kept here because the research is expensive to re-derive, and because the milestone that adopts
them should not have to.

**SignalR** — verified from Sonarr source; all five \*Arrs share the codebase, so the contract is
identical.

- Endpoint `{urlBase}/signalr/messages`, registered
  `MapHub<MessageHub>("/signalr/messages").RequireAuthorization("SignalR")`.
- **ASP.NET Core SignalR** (not legacy): `POST /signalr/messages/negotiate?negotiateVersion=1`,
  then WebSocket.
- Auth scheme `SignalR` = API key in `X-Api-Key` **or** query `access_token`. The official web UI
  uses the query form. ⚠️ API keys are not validated for URL-safety upstream — a key containing `#`
  breaks the query form. URL-encode it and warn in config.
- **Single client method `receiveMessage`**, payload
  `{name: string, body: {action: 'sync'|'created'|'updated'|'deleted', resource: T}, version?: number}`.
  `SignalRMessage.Action` is `[JsonIgnore]`d at the top level — the action rides *inside* `body`.
- On connect the hub pushes `{name:"version", body:{version:"4.x.y"}}`.
- Sonarr `name` values (read from `SignalRListener.tsx`): `calendar, command, downloadclient,
  episode, episodefile, health, importlist, indexer, metadata, connection, qualitydefinition, queue,
  queue/details, queue/status, rootfolder, series, system/task, tag, version, wanted/cutoff,
  wanted/missing`. Radarr swaps `series`→`movie`, `episode(file)`→`moviefile`, adds `collection`.
  ⚠️ Per-app lists other than Sonarr's are **inferred, not source-read**.

> 🚩 **CORS: the hub is NOT covered by any CORS policy.** `/api/vN/*` is `AllowAnyOrigin` via
> `VersionedApiControllerAttribute`, but `MapHub` has no `.RequireCors(...)`, so the negotiate POST
> is a cross-origin XHR and is **blocked in a browser**. Combined with an \*Arr API key granting
> full admin, this settles it: **UsArr terminates SignalR server-side, and \*Arr API keys never
> reach a browser.**

Implementation rules when it lands: **hand-roll the client** (negotiate → WebSocket → send
`{"protocol":"json","version":1}\x1e` → read `\x1e`-delimited frames → handle `type:1` invocation
and `type:6` ping; ~200 lines — pulling `philippseith/signalr` drags in a server implementation);
**coalesce aggressively** into the same 250 ms flush as the rollup; **treat payloads as invalidation
hints, not data** — refetch canonically on `created`/`updated`, act directly on `deleted`.
⚠️ Sonarr `develop` carries an **API v5** alongside v3 and `RestControllerWithSignalR` has version
filtering that determines whether messages are sent at all — version-negotiate at connect.

**Webhooks** — configure via `POST /api/v3/notification` (schema at `/notification/schema`, verify
with `/notification/test`), which makes setup a one-click wizard step. They give semantic events
SignalR does not: `Grab`, `Download`, `ManualInteractionRequired`, `HealthRestored`. Three gotchas:
**`eventType` is PascalCase**, unlike everything else in these APIs; **`eventType:"Download"` has
two body shapes** (`BuildOnImportCompletePayload()` emits `Download` while building a
`WebhookImportCompletePayload`) — discriminate on `episodeFile` (single) vs `episodeFiles` (import
complete); and webhook payloads are **flattened** relative to REST (`quality` is a plain string,
`tags` are string labels rather than int ids), so **do not share a deserialiser**.
