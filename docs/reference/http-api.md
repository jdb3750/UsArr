# UsArr's own HTTP API — the contract a client can hold

**This file exists because the contract had nowhere to live.** The wire shape of
`GET /api/v1/library/recent` was written down in three places — the handler's doc comments, a
review-log entry and a handover message — and none of the three is reachable from a browser tab.
A frontend consequently modelled `limit` as *the value it sent* rather than the value the server
echoed back, which mis-pages at the boundary. A contract a consumer cannot find is not a contract.

**Read the tree for WHICH endpoints exist**, not this file: `internal/httpapi/server.go`'s route
table is authoritative, and `web/src/routes` is authoritative for which screens call them. This file
documents the *semantics* of the endpoints whose contract has been settled, and it is deliberately
short — an endpoint appears here when a client has to be able to rely on something the JSON shape
does not say by itself.

**Every response body in the package shares one error shape**, from `internal/httpapi/json.go`:

```jsonc
{"error": "bad_request", "message": "shown to a user", "action": "the one thing that fixes it"}
```

`action` is omitted when there is nothing actionable. `error` is a code from
`internal/httpapi/errorcodes.go` and is the field to switch on; `message` is prose and may change.

---

## 1 · `GET /api/v1/library/recent` — Home's Block C

One unified recently-added table across **every** media type, newest first, keyset-paginated
(ARCHITECTURE §17.2 as amended by [ADR-0028](../DECISIONS.md#adr-0028)). Not one strip per media
type and not one endpoint per media type: a sixth media type adds **rows**, not a sixth region.

It is a local read (principle 1) — one SQLite statement per page, no \*Arr call, no metadata
provider, no image fetch. Requires an authenticated session; without one it is `401 unauthorized`.

### 1.1 Query parameters

| Parameter | Type | Default | Behaviour |
| --- | --- | --- | --- |
| `limit` | integer | `50` | The page size **requested**. See §1.2 — it is a clamp, not a validated range, and the response says what was actually applied. |
| `cursor` | opaque string | — | A token minted by a previous response's `next_cursor`. Never construct one; never edit one. A token that will not parse is `400 bad_request`, never a silent reset to page one. |

There is **no `?lib=` scope** and no per-type filter. Both are later commits; §17.2's library chip is
a join this read does not carry.

### 1.2 `limit` is a clamp, not a validated range — and the echoed `limit` is authoritative

🚩 **A client pages against the `limit` in the RESPONSE, never against the one it sent.** This is the
single most load-bearing sentence on this page. The server serves at most `200` rows per page and
says so by echoing the page size it applied; a client that sent `10000`, received `200` rows and
believed its own number reads a full page as a short one and stops at the boundary.

| Sent | Result |
| --- | --- |
| absent, empty, or `0` | `200 OK`, page size `50`. `0` is spelled the same as "unspecified" on purpose: a client rendering `?limit=${n}` with `n` unset gets a page, not an empty one. |
| `1` … `200` | `200 OK`, page size as asked |
| anything above `200` — **including values beyond 2³¹ and 2⁶³** | `200 OK`, page size `200`, **silently**, echoed in `limit` |
| negative, or not a whole number (`-1`, `abc`, `1.5`, `0x10`) | `400 bad_request` with an `action` |

**Why clamp rather than reject.** A page size that came out too big is not a request the server
cannot answer — it is one it can answer with fewer rows. Refusing it fails a whole screen over a
number the server was about to ignore anyway, and the honest recovery ("ask for fewer") is exactly
what the clamp already does. A negative or non-numeric value is different in kind: it is not a page
size that came out too big, it is not a page size, and there is no honest clamp target for it.

**The clamp is total over the positive integers.** It previously had an invisible cliff at 2³¹ —
`?limit=2147483647` clamped to 200 while `?limit=2147483648` returned `400 "is not a non-negative
integer"`, of a value that plainly is one. `recentWorksLimit` parses at 64 bits and treats
`strconv`'s range error as the saturation it already is, so the rule holds with no exceptions.
`TestRecentWorksLimitIsAClampNotAValidatedRange` pins the whole table above by execution.

### 1.3 Response

```jsonc
{
  "items": [
    {
      "id": 1,
      "media_type": "comics",
      "kind": "comic",
      "title": "Berserk",
      "year": 2020,
      "added_at": "2026-08-10T10:00:00Z",
      "have_count": 43,
      "want_count": 17,
      "availability": {"k": "count", "have": 43, "total": null,
                       "total_source": null, "missing": ["7", "12"]}
    },
    {
      // The SAME shape with every optional key absent — and this row is not an
      // error. `year`, `added_at` and `availability` are each absent when the
      // catalogue has nothing true to put there.
      "id": 2,
      "media_type": "movies",
      "kind": "movie",
      "title": "Train Dreams",
      "have_count": 0,
      "want_count": 1
    }
  ],
  "limit": 50,
  // This one decodes to the undated tail at work 2 — see §1.5. Its internals are
  // shown here only to make the point that they are the SERVER's business.
  "next_cursor": "MW4y"
}
```

| Field | Always present? | Meaning |
| --- | --- | --- |
| `items[].id` | yes | `work.id`. The catalogue's public name for a row; item routes are `/library/{media_type}/{id}`. |
| `items[].media_type` | yes | §17.2's six-value navigation enum: `movies`, `tv`, `music`, `ebooks`, `audiobooks`, `comics`. **Resolved server-side and it must be** — the Ebooks/Audiobooks split is not derivable from `kind`, so a client that computed this itself would collapse two of the six into one. |
| `items[].kind` | yes | `work.kind` verbatim — the schema's word, where `media_type` is the user's. |
| `items[].title` | yes | |
| `items[].year` | **no** | Absent, not `0`, when the catalogue has no year. A rendered `0` is a claim about a release date. |
| `items[].added_at` | **no** | RFC 3339 UTC. Absent when the upstream reported no creation date — a state Kavita reaches. **An absent value sorts LAST, never first.** |
| `items[].have_count` | yes | Denormalised rollups. The numerator and the gap behind §17.2's `have / total · N missing` grammar. |
| `items[].want_count` | yes | |
| `items[].availability` | **no** | The polymorphic blob — see §1.4. |
| `limit` | yes | **Authoritative** (§1.2). |
| `next_cursor` | **no** | Absent when this page is the last one; its absence is the "Load more" button's off switch. Absent rather than empty, because `""` reads as a cursor whose value is unknown. |

**Nothing service-side is on the wire and nothing can be.** No `remote_path` (a filesystem path on
the upstream's box), no service instance, no credential. Which Kavita a series came from is not
something Home renders, and naming it would publish the install's topology.

### 1.4 `availability` is optional, and absent means absent

The blob is `reference/schema.md` §1's polymorphic rollup, **forwarded verbatim**. It carries a
`"k"` discriminator as its first key and a renderer switches on it; the three shapes are:

| `k` | Medium | Shape |
| --- | --- | --- |
| `tier` | video | per-quality-tier fractions — `{"k":"tier","1080p":{"have":250,"total":300}}` |
| `edition` | music | edition-keyed fractions with a `label`, because a remaster changes the track list |
| `count` | comics, and anything with no honest denominator | `have`, a `total` that may be `null`, a `total_source` naming the declaration, and `missing` |

⚠️ **`total: null` is not `total: 0`.** The first means nobody honestly knows; the second means the
series is empty. §6.3's render rule (`have == total && total > 0` → ✓) must never fire on the first.

🚩 **The key is absent when a work has no blob, and that is a legitimate state rather than a
failure.** A renderer treats absence as absence: it shows what it knows (`have_count`) and **does
not invent a denominator** out of `have_count + want_count`.

**A corrupt blob is also absent from the wire — but it is no longer silent.** Four cases are dropped
rather than forwarded, because this response is marshalled whole and one bad blob would otherwise
fail the entire block for the sake of its decoration:

| Stored value | Wire | Log |
| --- | --- | --- |
| SQL `NULL` | absent | **none** — this is not a fault, and a warning per honestly-empty work would make the log worthless |
| not JSON, or JSON that is not an object | absent | `WARN work.availability will not decode …` with `work_id` |
| an object with no `"k"` | absent | `WARN work.availability has no "k" discriminator …` with `work_id` |
| an object whose `"k"` is none of the three above | absent | `WARN work.availability has an unrecognised "k" discriminator …` with `work_id` and `k` |

Each warning carries the **work id**, which is what makes the row findable —
`SELECT availability FROM work WHERE id = ?`. It does not carry the blob: that text came out of the
database and has no length bound.

⚠️ **A fourth `k` is a writer bug, not a newer server.** In v0.1 the writer and this reader are the
same binary and ship in the same commit, so there is no version skew to be forward-compatible with.
Adding a shape is therefore an edit in **both** `reference/schema.md` §1 and
`httpapi.availabilityKinds`; `TestAvailabilityKindsMatchSchemaMd` reads this repo's own schema
document and fails if the two disagree, so it cannot be done in one place only.

### 1.5 Paging

Keyset, not offset. Walk it by following `next_cursor` until it is absent:

```
GET /api/v1/library/recent?limit=50
GET /api/v1/library/recent?limit=50&cursor=<next_cursor from the previous page>
```

The token is an **encoding, not a secret** — it carries the sort key of the last row on the page,
both halves of which that row already published. It is base64url for **transport**, because the
timestamp inside it contains a space. It is versioned, so an unknown prefix is a parse failure rather
than a best effort. Send `limit` again on every page; the server does not remember it.

⚠️ **Works with no `added_at` form a tail after every dated row**, and reaching that tail is the
cursor's job rather than the client's. Do not attempt to reproduce the ordering by sending your own
`after_added_at` / `after_id`: a plain row-value comparison makes every undated work unreachable on
every page but the first, silently.

### 1.6 Errors

| Status | `error` | When |
| --- | --- | --- |
| `400` | `bad_request` | `limit` is negative or not a whole number (§1.2); or `cursor` is not a token this endpoint issued. Both carry an `action`. |
| `401` | `unauthorized` | no session. |
| `500` | `internal` | the local read failed. |

---

## 2 · `GET /api/v1/services/health` — the two catalogue-freshness fields

**This section documents `last_full_sync_at` and `work_count` and nothing else on the endpoint.**
The rest of the row's contract has not been settled here; read `internal/httpapi/services.go`'s
`serviceHealthResponse` for it. The two fields below are what the Services screen's
`Last successful sync` and `Items` columns render, and both were hardcoded client-side before they
existed on the wire.

Like every read on this screen it is served **entirely from SQLite** (principle 1): no \*Arr call,
no probe issued on the request path. Requires an authenticated session.

### 2.1 The two fields

```jsonc
{
  "services": [
    {
      // …the rest of the health row…
      "last_full_sync_at": "2026-08-16T12:00:00Z",
      "work_count": 3
    }
  ]
}
```

| Field | Always present? | Type | Meaning |
| --- | --- | --- | --- |
| `services[].last_full_sync_at` | **yes**, as a value or as an explicit `null` | RFC 3339 UTC, or `null` | When this instance last **completed** a full catalogue import. `null` means **never**. |
| `services[].work_count` | **yes** | integer | How many **distinct works** this instance contributes to the local catalogue. |

### 2.2 `null` and `0` are different facts, and neither is omitted

🚩 **Read the pair, never either half alone.** A single number cannot carry the distinction the
screen needs, because `0` is the answer for a service that has never run *and* for one that ran and
found an empty library:

| `last_full_sync_at` | `work_count` | What it means |
| --- | --- | --- |
| `null` | `0` | **Never synced.** No full import has completed. |
| a timestamp | `0` | **Synced, and found nothing.** The upstream really is empty. |
| a timestamp | `> 0` | The ordinary case. |
| `null` | `> 0` | **A partial import's rows stand.** Batches committed before the import failed; the set is not known to be complete. |

**Neither field carries `omitempty`, deliberately.** `"last_full_sync_at": null` is a *positive*
statement — "this instance has never completed a full import". An absent key would be
indistinguishable from a server build that does not send the field at all, which is the ambiguity
this endpoint exists to remove.

**`work_count` is never nulled out to match the timestamp.** The fourth row above is why: a partial
import leaves its committed batches behind (see `internal/libsync`'s `FullImport`), and hiding those
rows would replace one wrong answer with another. `work_count` is always a real count.

⚠️ **A service with no catalogue at all reports `null` / `0` too** — a Prowlarr has no library to
sync. The field that separates *that* from an unsynced Kavita is `role`, which is already on the
row: `role: "indexer"` means the two columns are **not applicable**, not empty.

### 2.3 What `work_count` counts, exactly

Distinct `work` rows reachable from this instance's **live** item links. Three narrowings, each
ruling out a number that would be a different claim:

* **Distinct works, not links.** §6.4 tier 1 merges two remote items that share a strong external id
  onto one work, so a link count reports a library as larger than it is.
* **Live links only.** A link inside its 7-day tombstone window is an item the upstream no longer
  reports.
* **Live works only.** Same window, on the work.

It is **not** a media-file count and **not** an edition count — nothing on this screen claims to know
how many files an upstream holds. It is also **not** per-library: a user-defined library binds
containers across instances (§17.8), so a per-library number is a different query.

Credited people are outside it by construction rather than by a filter: the credit pass creates a
`work` of kind `person` with no `service_item_link` at all, so no instance contributes one.
