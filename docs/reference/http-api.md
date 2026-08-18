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

## 2 · `GET /api/v1/libraries` — the Libraries screen, row view

The user-defined libraries, each with the upstream containers a service already named
(ARCHITECTURE §17.8). §17.8 splits this screen from Services in one sentence — *"Services answers
'is the pipe up, and how do I fix it?'. Libraries answers 'what is in it, what is it called, and
where do requests go?'"* — and this endpoint answers the first two thirds. The third is §2.4.

It is a local read (principle 1) — two SQLite statements, no \*Arr call, no metadata provider, no
capability probe. **In particular it is not the connect probe.**
[ADR-0048](../DECISIONS.md#adr-0048) puts the *proposal* set in the probe's response and never in a
table, so this endpoint serves accepted libraries only; see §2.5. Requires an authenticated session;
without one it is `401 unauthorized`.

### 2.1 Query parameters

**None, and there is no paging.** A user's libraries are a set they created by hand — §17.8's
reference install has seven — and the row view is a reorderable list, which a keyset window cannot
express. There is nothing to clamp, so unlike §1 there is no `400` path at all.

### 2.2 Response

```jsonc
{
  "items": [
    {
      "id": 2,
      "name": "Ebooks",
      "slug": "ebooks",
      "kind": "book",
      "formats": ["ebook"],
      "sort_order": 5,
      "enabled": true,
      "include_in_search": false,
      "item_count": 3,
      "sources": [
        {
          "id": 2,
          "service_instance_id": 1,
          "service_name": "Kavita Manga",
          "service_kind": "kavita",
          "container_kind": "remote_library",
          "container_ref": "12",
          "container_name": "Books",
          "is_metadata_authority": true,
          "missing_since": "2026-08-17T09:30:00Z"
        },
        {
          // The SAME shape with the optional keys absent. A healthy source has
          // no `missing_since`; this is not an error.
          "id": 3,
          "service_instance_id": 2,
          "service_name": "Kavita Books",
          "service_kind": "kavita",
          "container_kind": "remote_library",
          "container_ref": "21",
          "container_name": "More Books",
          "is_metadata_authority": false
        }
      ]
    },
    {
      // §6.5 rule 5's retained orphan: no sources left, never auto-deleted,
      // shown with its reason. `sources` is `[]` and never absent.
      "id": 3,
      "name": "Loose Ends",
      "slug": "loose-ends",
      "kind": "book",
      "sort_order": 20,
      "enabled": true,
      "include_in_search": true,
      "item_count": 0,
      "orphaned_at": "2026-08-17T08:00:00Z",
      "sources": []
    }
  ]
}
```

`items` is ordered by `sort_order`, ties broken by `name`. `items` is always present and is `[]` on
an empty install — §17.8's screen has a zero state, and an absent key would be indistinguishable
from a failure.

| Field | Always present? | Meaning |
| --- | --- | --- |
| `items[].id` | yes | `library.id`. **Never `0`** — see §2.3. |
| `items[].name` | yes | The user-owned name. §17.8's merge key on it is case-insensitive and whitespace-trimmed, per user. |
| `items[].slug` | yes | The URL identity: the chip is `?lib=ebooks`. ⚠️ **Do not render it as a path.** §17.8: *"The row's identifier is not rendered as a path … Drop the slash and the mono face"* — a self-hoster who reads `/movies` under a library concludes UsArr scans `/movies`, on the screen whose banner says it never reads a filesystem. |
| `items[].kind` | yes | `library.kind` verbatim — the schema's word, one of `movie`, `series`, `artist`, `album`, `book`, `comic`, `game`. ⚠️ **Not §1's `media_type`.** That enum has six members and answers *"what is this work"*; this one has seven and answers *"what is this library of"*. §17.8 requires the **label** to be the product's vocabulary — `Movies · TV · Music · Books · Comics` — so the mapping is the renderer's. |
| `items[].formats` | **no** | A JSON array over `edition.format`, forwarded verbatim. `(kind, formats)` is §17.2's media-type **pair**: the Ebooks/Audiobooks split is `["ebook"]` and `["audiobook"]` over one `book` kind. Absent means **any format**, which is every row today — see §2.4. A stored value that is not a JSON array of strings is dropped and logged with the `library_id`. |
| `items[].sort_order` | yes | The reorder handle's value. Served rather than left implicit in the array order, so a client that re-sorts locally can still send back a correct reorder. |
| `items[].enabled` | yes | |
| `items[].include_in_search` | yes | Independent of `enabled`; both are read from their own column. |
| `items[].item_count` | yes | `library_member` rows in this library **that the caller's access scope admits**. Edition-grained by the table's key; equal to a count of distinct works today, because the only writer files every work under the `edition_id = 0` "whole work" sentinel. |
| `items[].orphaned_at` | **no** | RFC 3339 UTC. §6.5 rule 5's retained-with-a-reason state, set when the last source goes away. ⚠️ **Nothing writes it** — see §2.4. |
| `items[].sources` | yes | Possibly `[]`. Never absent: an absent key reads as *"unknown"*, and *"this library has no sources"* is precisely what §17.8's orphaned state renders. |
| `sources[].id` | yes | `library_source.id`. |
| `sources[].service_instance_id` | yes | What §17.8's cross-link needs: *"a degraded source on a library row links to that instance's Services row"*. |
| `sources[].service_name` | yes | The chip's label, and the string §17.8's warning copy uses (*"Radarr feeds 2 libraries"*). |
| `sources[].service_kind` | yes | The icon. |
| `sources[].container_kind` | yes | One of `instance`, `root_folder`, `remote_library`, `tag`, `series_type`. In v0.1 always `remote_library`. |
| `sources[].container_ref` | yes | The container the **upstream itself** reported, verbatim. A Kavita library id in v0.1. |
| `sources[].container_name` | **no** | The container's own name as the upstream reported it at bind time — §17.8's *"upstream's own name beneath it, greyed and non-editable"*. Absent, not `""`, when unrecorded. |
| `sources[].is_metadata_authority` | yes | §17.8 suppresses the *control* below two sources; the fact still travels. |
| `sources[].missing_since` | **no** | RFC 3339 UTC. §17.8's per-source health: the upstream stopped reporting this container. ⚠️ **Nothing sets it** — see §2.4. |

**Nothing service-side beyond a name and a kind is on the wire, and nothing can be.** This is the
only user-facing read in the product that joins a list to `service_instance`, the row carrying
`api_key_enc` — a full-admin \*Arr credential — and `base_url`, an internal host the user typed.
Exactly two of that table's columns are read and exactly two reach the browser. §17.8 states the rule
for the whole screen: *"No credential field ever appears on this screen"*; §12.1 keeps API keys
behind Services plus sudo, and an instance's address is §17.3's to render.

### 2.3 The reserved `Unfiled` library is never here

Migration `00005_library_sync.sql` seeds `library.id = 0`, `Unfiled`, as the landing place the
membership derivation needs so that a work belonging to no other library still matches a scope. Its
own comment says what it is not: **"Never listed on the Libraries screen, never offered in the scope
chip, never proposed."** This endpoint is the Libraries screen, so the row is excluded in the SQL,
unconditionally, with no parameter that can include it. A client never has to filter for it and must
not treat `id = 0` as a library.

### 2.4 Three fields describe states nothing in the tree can currently reach

Stated here rather than left to be inferred, because a screen that renders none of them is reporting
the **writer's** silence and not the upstream's:

| Field | Status |
| --- | --- |
| `sources[].missing_since` | The two statements in non-test Go that touch the column both **clear** it; no code path sets a non-NULL value. So *"no source is missing"* is not a positive health check. |
| `items[].orphaned_at` | No writer and no reader in non-test Go. |
| `items[].formats` | No writer: `library.formats` is NULL on every row, so the Ebooks/Audiobooks split it exists for is not reachable in v0.1 (§17.8 marks that split *"from the milestone Audiobookshelf lands in, not v0.1"*). |

**And the request destination is absent from this response entirely.** §17.8: *"The `Request
destination` column does not render in v0.1, and it returns with the first service that can be a
destination"*, because no service v0.1 connects can be a library's request sink at all. The four
`sink_*` columns are written by nothing in the tree, so serving four nulls per row would make a
complete deferral look like a half-built one. ⚠️ **This is sequencing, not a cut.**

**What is deliberately not computed: which media types a source supplies.** It is not a property of
`library_source`; answering it means an aggregate over `library_member` → `work.kind` →
`edition.format` per source — a join across the catalogue that this read otherwise never makes, for
a number the library's own `(kind, formats)` pair already bounds. It is left out.

### 2.5 There is no "proposed" state, and there is nothing for one to carry

A client cannot ask this endpoint to distinguish a library the user has never been shown from one
they accepted, and the reason is that [ADR-0048](../DECISIONS.md#adr-0048) removed the state rather
than this endpoint declining to serve it:

> **A library proposal lives in the connect probe's response. It is never persisted. A `library` row
> is created only when the user accepts one.**

— from which *"once no row exists before Accept, every row is an accepted row by construction, and a
column that cannot express 'proposed' is not being asked to."* `library.managed_by` is therefore not
on this wire either: ADR-0048 Fact 1 and §17.8 both measure that its second value `'user'` **has
never been written by any code path**, so the column is `'auto'` on every row and §17.4 rule 5 — *a
column whose value is identical for every row is not data* — applies to a wire field as much as to a
table cell.

⚠️ **ADR-0048 is equally explicit that the removal it implies is not built.** §17.8: *"Libraries
come into existence, on a first successful connect to a Kavita, with no screen involved … The Accept
step below does not gate it, because the Accept step does not exist."* ADR-0048 clause 4 answers
exactly that case — existing `managed_by = 'auto'` rows are **declared** accepted on upgrade, with no
migration and no backfill — so the invariant holds by declaration today and by construction once
Accept lands. Either way the field would be a constant.

### 2.6 Errors

| Status | `error` | When |
| --- | --- | --- |
| `401` | `unauthorized` | no session. |
| `500` | `internal` | the local read failed. |

There is no `400`: the endpoint takes no input. A caller whose access scope admits no service
instances receives `200 OK` with `{"items": []}` — the scope **fails closed**, so an empty visible
set means no libraries rather than every library.

---

## 3 · `GET /api/v1/services/health` — the two catalogue-freshness fields

**This section documents `last_full_sync_at` and `work_count` and nothing else on the endpoint.**
The rest of the row's contract has not been settled here; read `internal/httpapi/services.go`'s
`serviceHealthResponse` for it. The two fields below are what the Services screen's
`Last successful sync` and `Items` columns render, and both were hardcoded client-side before they
existed on the wire.

Like every read on this screen it is served **entirely from SQLite** (principle 1): no \*Arr call,
no probe issued on the request path. Requires an authenticated session.

### 3.1 The two fields

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

### 3.2 `null` and `0` are different facts, and neither is omitted

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

### 3.3 What `work_count` counts, exactly

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
