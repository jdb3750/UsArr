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

There is **no `?lib=` scope here and no per-type filter here** — and both exist, on §7. §17.2 closes
Block C at one table, one order and *no* filters, so this endpoint refuses the chip by design rather
than by backlog; a client that wants the scope calls §7. Unrecognised parameters are **ignored, not
refused**, so `?lib=…` sent here is `200` over the whole catalogue rather than a `400`.

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
| `items[].have_count` | yes | Denormalised rollups. The numerator and the gap behind §17.2's `have / total · N missing` grammar. ⚠️ **`0` is also the column default, so a `0` here is not evidence on its own** — §1.4.1. |
| `items[].want_count` | yes | |
| `items[].availability` | **no** | The polymorphic blob — see §1.4. **Absent means *not counted*, never *none held*** — §1.4.1. |
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

⚠️ **`k:"count"` is the only shape anything in the tree writes today, and it ships WITHOUT `missing`.**
The writer is `internal/store/rollup.go`, fed by the Kavita volume walk, and it emits exactly
`{"k":"count","have":N,"total":…,"total_source":…}` for works of kind `book`, `comic` and
`comic_issue`. `missing` is **contiguity** — *"43 issues · #7, #12 and #30-32 missing"* — computed
locally from `work_comic_issue.number_sort`, and nothing writes `work_comic_issue` yet, so the key is
**absent rather than empty**: an empty `missing` array would claim a complete run, which is a
different statement from "nobody has worked out what is missing". A renderer must treat an absent
`missing` as unknown, never as none.

**`total` is present only when the series has STOPPED and a total was declared** (`ARCHITECTURE.md`
§6.1), so on an ordinary comics library it is `null` on most rows. `total_source` names the
declaration's origin — `comicinfo` for everything Kavita reports, because Kavita's `totalCount`
derives from ComicInfo's `Count`. Both keys are always **present**; it is their VALUE that is `null`.

⚠️ **`total: null` is not `total: 0`.** The first means nobody honestly knows; the second means the
series is empty. §6.3's render rule (`have == total && total > 0` → ✓) must never fire on the first.

🚩 **The key is absent when a work has no blob, and that is a legitimate state rather than a
failure.** A renderer treats absence as absence and **does not invent a denominator** out of
`have_count + want_count`. ⚠️ Nor does it fall back to `have_count` as the thing it "knows" —
§1.4.1 says what absence actually means, and a bare `have_count` is not it.

**A corrupt blob is also absent from the wire — but it is no longer silent.** Four cases are dropped
rather than forwarded, because this response is marshalled whole and one bad blob would otherwise
fail the entire block for the sake of its decoration:

| Stored value | Wire | Log |
| --- | --- | --- |
| SQL `NULL` | absent | **none** — this is not a fault, and a warning per uncounted work would make the log worthless |
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

#### 1.4.1 Absence means NOT COUNTED — it does not mean the user holds nothing

**The absence of `availability` means no count has ever been computed for that work.** It is not a
statement about the user's library, and it is emphatically not a zero.

🚩 **A consumer must not render an absent blob as `0`, as "none", or as any glyph, bar or accessible
name that asserts emptiness.** The honest rendering is **"not counted yet"**, or whatever that
screen's vocabulary calls the same thing. It is honest and it is also mute — it cannot say WHY the
work is uncounted, and §3.4's `file_read_failures` is the per-instance number that can. A crossed circle reading *none held* on a series the user
demonstrably has on disk is the failure this paragraph exists to prevent.

⚠️ **`have_count: 0` is not evidence of anything on its own.** `have_count` is sent
unconditionally and its column is `NOT NULL DEFAULT 0` (`reference/schema.md` §1), so a work nobody
has ever counted and a work genuinely holding nothing carry the same number. It is only meaningful
**alongside a present blob** — which is also where a truthful zero lives: §6.3's render rule
(`have == 0` → ✗) fires on a *present* blob and on nothing else.

**Why this is durable rather than a description of today's tree.** The counts and the blob are
written by **one** recompute, not two: a child write sets `work.rollup_dirty` on the ancestor in the
same transaction and the 250 ms flush re-aggregates that dirty set once, rewriting the blob whole
(ARCHITECTURE §6.3, [`sync.md`](./sync.md) *"Rollup flush"*, and `reference/schema.md` §1 — *"the
blob is opaque to the flush, which recomputes and rewrites it whole"*). There is one dirty bit and
there is no second one, so **there is no specified path that moves `have_count` without also writing
the blob**; a non-zero count under an absent key is not a state the mechanism can reach. Where the
rollup has nothing to aggregate it withholds the blob deliberately instead of publishing a
manufactured zero, which arrives at the same signal from the other direction.

**So absence self-corrects, and a client written to this rule needs no second edit.** The first flush
that has anything to count publishes the key — including when the honest answer is `have: 0` — and
the row moves from *not counted* to counted with no wire change behind it.

✅ **That flush now exists** (`internal/store/rollup.go`), and it holds the rule from the writer's
side rather than by sequencing: a work with **no file rows in the viewer's scope** gets **no blob**,
and the flush refuses to publish `have: 0` over an empty basis. `REVIEW-LOG.md` LS-211.2 rule 2 is
the reasoning; `TestTheRollupRefusesToFabricateAZero` is the guard, fired.

⚠️ **ABSENCE HAS A SECOND CAUSE NOW, AND IT CHANGES NOTHING FOR A CONSUMER.** A work that HAD a blob
and whose basis then went away — every file deleted upstream, so the walk reconciled its rows to
nothing — has its blob **withdrawn**, returning the row to *not counted*. So the precise reading of
an absent key is **"no count is currently computable for this work"**, of which "never counted" is
the common case. The consumer rule is unchanged and unchanged deliberately: with zero file rows,
*never walked* and *walked, and empty* are indistinguishable from the server's side, and only one of
them could honestly be rendered as emptiness — so neither is. The alternatives were leaving
yesterday's `have: 43` standing, which is a claim about a library the user no longer has, and writing
`have: 0`, which is the fabrication this whole section exists to prevent.

**One invariant this writer preserves on purpose:** it never moves `have_count` without also writing
the blob. A work whose `kind` has a shape this writer cannot produce — video is `k:"tier"`, music is
`k:"edition"`, and neither has a source that writes files yet — is skipped with **both** columns left
alone rather than counted into a bare number. `TestAKindWithNoBlobShapeLeavesBothColumnsAlone` is the
guard.

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

## 3 · `GET /api/v1/services/health` — the catalogue-freshness fields

**This section documents `last_full_sync_at`, `work_count` and `file_read_failures`, and nothing
else on the endpoint.**
The rest of the row's contract has not been settled here; read `internal/httpapi/services.go`'s
`serviceHealthResponse` for it. The first two are what the Services screen's
`Last successful sync` and `Items` columns render, and both were hardcoded client-side before they
existed on the wire. The third (§3.4) rides on the same `Items` cell as a muted second line, shown
only when it is non-zero; §3.4 says what it means and what a consumer must not do with it.

Like every read on this screen it is served **entirely from SQLite** (principle 1): no \*Arr call,
no probe issued on the request path. Requires an authenticated session.

### 3.1 The three fields

```jsonc
{
  "services": [
    {
      // …the rest of the health row…
      "last_full_sync_at": "2026-08-16T12:00:00Z",
      "work_count": 3,
      "file_read_failures": 0
    }
  ]
}
```

| Field | Always present? | Type | Meaning |
| --- | --- | --- | --- |
| `services[].last_full_sync_at` | **yes**, as a value or as an explicit `null` | RFC 3339 UTC, or `null` | When this instance last **completed** a full catalogue import. `null` means **never**. |
| `services[].work_count` | **yes** | integer | How many **distinct works** this instance contributes to the local catalogue. |
| `services[].file_read_failures` | **yes** | integer | How many of this instance's items the **file walk could not read** — see §3.4. `0` is a positive statement, not an absence. |

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

**No field in this section carries `omitempty`, deliberately** — including `file_read_failures`. `"last_full_sync_at": null` is a *positive*
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

### 3.4 `file_read_failures` — why an item is uncounted, rather than that it is empty

**This is §1.4.1's missing half.** A work whose file walk failed gets no rollup, so
`GET /api/v1/library/recent` omits its `availability` and §1.4.1 requires the honest rendering
*"not counted yet"*. That rendering is truthful and it is also mute: it cannot tell the user
whether nobody has counted yet or whether **the upstream refused to answer**. This field is the
number that lets a screen say *"3 series could not be read"* instead.

The producer is the Kavita file walk (`internal/libsync`'s `StreamFiles`). A per-item failure there
is **dropped, not fatal**: the item keeps every file row it already had, because a failed read is
not evidence of an empty library, and the walk carries on to the next item. Each drop writes a
`sync_report` row of kind `file_walk_failed`, and **this field is that table's only reader**.

| Value | What it means |
| --- | --- |
| `0` | Every item the last walk asked for answered. **Not "no data"** — the key is never omitted. |
| `> 0` | That many **distinct items** could not be read. Their file rows and counts are STALE, not zero. |

**The window is "since the last completed full sync started".** `sync_report` is append-only, so a
bare count would include failures a later successful import already fixed. The read is scoped by
`created_at >= last_full_sync_at`, which is exactly the last completed run plus anything a partial
run has added since — `last_full_sync_at` is stamped with the run's *start* time. An instance whose
`last_full_sync_at` is `null` counts **everything it has**: there is no earlier completed run to
exclude, and the rows it holds came from runs that really happened.

⚠️ **It is distinct items, not failures.** Three partial runs that each failed on the same series
report `1`. A count of rows would report `3` and describe a library that does not exist.

⚠️ **It is not `libsync.Report.FileReadFailures`, despite the matching name.** The Go field is
**one run's** count and is returned to that run's caller; this field is the same fact over the window
above, which can span a completed run and every partial run after it. Same word, two windows — which
is `DEVELOPMENT.md` §11's collision class, kept here because the alternative is a second name for
one concept. A consumer reads the window, not the name.

⚠️ **It carries no reason and no upstream text**, on §5.5.5's rule for the same reason. Each
failure's classified reason (`not_found`, `unauthorized`, `server_error`, …) and the upstream HTTP
status are in `sync_report.detail`, which a browser never sees. The classification is a **closed
vocabulary** rather than a redacted message: `ssrf.RedactText` finds credentials inside URLs and
says in its own doc that "a bare secret that is not inside a URL passes through untouched", and an
upstream body echoing a bare key is exactly the shape `RESEARCH.md` R-08 measured on Mylar3.

🚩 **A non-zero value does not make the instance unhealthy, and must not be rendered as an error.**
The import completed; `last_full_sync_at` is stamped; the works imported. What is incomplete is
those items' **file** facts. The honest sentence is about the items, not about the service.

**What a consumer has to do.** As of 2026-08-19 the Services screen is the one that does (885dac0):
`web/src/lib/api.ts`'s `ServiceHealth` mapper carries the field as `fileReadFailures`, and
`web/src/lib/services.ts`'s `fileReadNote` renders it as *"File list not read for N items"* on the
`Items` cell's muted second line, **only when it is non-zero** and never as a fault state. Any
further consumer owes the same three things: map the field rather than defaulting it; render the
non-zero case only, as a note; and pin both halves the way `services.test.ts` and
`services-screen.test.ts` pin them — a test that passes on the constant `0` passes on the bug.

---

## 4 · `POST /api/v1/services/{id}/sync` — re-run the catalogue import

The Services screen's **Run full sync now** (ARCHITECTURE §17.3, the action named for a
*degraded, partial data* row). It is **per instance**: there is no "re-import everything" route,
because the control lives on a service's own row.

**Why it exists.** The catalogue import had exactly one trigger — a bootstrap that runs on first
connect and is gated on `last_full_sync_at` forever after — so an instance that had imported once
could never import again. That makes every later fix to what an import *writes* undeliverable to
rows already imported. This is the re-run, not a backfill: a backfill repairs one field once.

**It is a full re-import, not a delta.** `internal/libsync` implements channel 1 and nothing else
(read its package doc), and a delta walk would be the wrong tool anyway: ADR-0035 §2a's watermark
moves on an upstream *change*, so a delta could never revisit a row that UsArr itself wrote wrongly.

Gated exactly like the five other writes on this screen: `Content-Type: application/json`, the
double-submit CSRF token, an authenticated session, and the **five-minute sudo window** (§17.3.3).

### 4.1 Request

No body fields. Send `{}` — `Content-Type: application/json` with a JSON body is what
`csrfProtected` requires.

### 4.2 Response — `202 Accepted`

```jsonc
{
  "status": "started",
  "instance_id": 3,
  "kind": "kavita",
  "name": "Kavita"
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `status` | string | `"started"`. The only value a 202 carries. |
| `instance_id` | integer | The instance the import was started for. |
| `kind` | string | Its service kind, e.g. `kavita`. |
| `name` | string | Its configured name — what the row is called on screen. |

**Four fields, allowlisted one at a time.** A `service_instance` row also holds an encrypted
full-admin credential, a KEK id, a TLS pin and a base URL. None of those is here, and the response is
not built by spreading the row.

🚩 **There is no progress field, and there will not be one on this response.** Nothing has been read
when the 202 is written, so any count, percentage or estimate would be invented. Progress, where a
client wants it, is the `import.progress` frame on `GET /api/events`, which carries real counts
because the importer publishes it as batches commit.

### 4.3 `202` does not mean "imported"

**It means accepted and started, and nothing more.** The import outlives the response by minutes and
this endpoint never learns how it ended. A client that renders 202 as "done" is wrong on every
failure.

Two observations are available, and between them they make **"started, then failed"** representable
rather than collapsing it into a binary:

| Question | How a caller answers it |
| --- | --- |
| Is it still running? | `POST` again. `409 import_in_progress` means yes; a `202` means no. |
| Did it succeed? | `last_full_sync_at` on `GET /api/v1/services/health` (§3). It is written **on success only**, so it advancing is the only positive evidence. |
| Did it start and then fail? | It is no longer running (a `202` on a repeat press) **and** `last_full_sync_at` has not moved. |

⚠️ **A failed import leaves the batches it already committed in place.** "Started, then failed"
therefore means a *partially* updated catalogue, never a rolled-back one — which is exactly the
`last_full_sync_at: null` / `work_count > 0` row of §3.2's table. Say so; do not report it as though
nothing happened.

### 4.4 A non-2xx never means "an import you asked for is running"

**Every refusal below is decided before anything upstream is touched.** The kind check, the enabled
check and the in-progress claim all complete synchronously, so a non-2xx means **no import started
for this call**. It does not always mean nothing is running: `import_in_progress` means another one
is, and says so in its own code.

| Status | `error` | Meaning | `action` |
| --- | --- | --- | --- |
| `202` | — | Started. | — |
| `409` | `import_in_progress` | An import for this instance is already running; a second was **not** started. Safe to press twice — the second press changes nothing. | `Wait for the running import to finish` |
| `409` | `not_a_catalogue_source` | The instance has no library to import — an indexer, not a catalogue source ([ADR-0041](../DECISIONS.md#adr-0041)). | `Run a sync on a catalogue service` |
| `409` | `service_disabled` | The service exists and is switched off. | `Enable the service` |
| `404` | `not_found` | No such instance **for this user** — the read is access-scoped. | — |
| `403` | `sudo_required` | The sudo window closed. Prompt, then retry the pending press (§17.3.3). | `Confirm your password` |
| `403` | `csrf` | Stale page token. | (its own) |
| `501` | `not_configured` | This build has no catalogue importer wired in. | — |
| `500` | `internal` | The import could not be started — most often a stored credential that will not open. The upstream reason is in `message`, redacted. | `Test connection` |

**The two 409s are separate codes on purpose.** Their fixes are opposite — wait, versus press this
somewhere else — and a client that switched on the status alone would have to guess which sentence
to show.

---

## 5 · `GET /api/events` — the `import.progress` frame

§4.2 sends every client that wants progress here, so what the frame does and does not promise
belongs here rather than in one screen's design section. Four facts, each measured at the publish
sites, and each one a thing the JSON shape does not say by itself — then **§5.5, which is not one of
them**: it is the contract for the terminal **`stopped`** frame, written ahead of its producer so the
consumer could be built against authoritative names — and **the producer has since landed**
(`cmd/usarr/import.go`'s `publishImportStopped`, LS-180). §5.1 to §5.4 are measurements; §5.5 is a
contract, and §5.1 below is the measurement it falsified.

```jsonc
{"instance_id": 2, "phase": "credits", "items_read": 2, "applied": 3, "total": 2}
```

The recorded frames a client can hold this against are in
`web/src/lib/__fixtures__/sse-frames.json`, regenerated off a live stream by `cmd/usarr`'s
`TestSSEFramesMatchTheClientContract`; the type is `internal/libsync`'s `Progress`.

⚠️ **No citation to `importer.go` in §5 is a line number any more, and that is a repair rather than
a style.** Every one of them used to be `importer.go:<n>`, and every one was wrong by the time anyone
read it: the `files` phase moved all five publish sites at once, and §5.2 and §5.3 went stale in the
same commit that made their line numbers wrong (`REVIEW-LOG.md` LS-250, LS-270). A function name
survives an edit above it; a line number is falsified silently by an insertion anywhere earlier in
the file, and a citation nobody can trust is worse than none because it reads as though it were
checked. Cite the symbol and let `grep -n` find it. ⚠️ **The `:<n>` citations still on this page for
OTHER files are not endorsed by that** — they are simply out of the change that fixed these, and the
same rot applies to them.

### 5.1 Silence means UNKNOWN — never finished, and never failed

⚠️ **This paragraph used to read "a failed import publishes nothing", and the `stopped` producer
falsified it.** A failed run now publishes exactly one terminal frame (§5.5). It is published by a
wrapper around `cmd/usarr`'s `fullImportLocked` rather than from inside `libsync.FullImport`, and
that seam is forced: two of the failures happen **before an `Importer` exists** — a service that is
not a Kavita, and a stored credential that will not open, both refused in `cmd/usarr/import.go`'s
pre-importer checks — so a publish inside `libsync` could not see either, and those two used to send
not even a `containers` frame.

🚩 **Silence still means UNKNOWN, and for three reasons the producer does not touch.** It is now
*usually* possible to tell a stopped run from a running one; it is not guaranteed.

1. **The frame is best-effort.** `Hub.Publish` never blocks and drops a subscriber whose queue is
   full, so a client can miss it entirely (§5.5.3).
2. **A build with no hub publishes nothing at all.** `cmd/usarr/import.go`'s `importProgress`
   returns a nil callback when the hub is not wired (the registry is built before the server), and
   on that build a **successful** run is equally silent — so silence there is not a claim about the
   import in either direction.
3. **A dead server sends nothing**, by construction.

So **`phase: "done"` is still the only positive evidence of completion**, `stopped` is a fast notice
rather than a verdict of record, and §4.3's table remains the only way to answer "did it succeed" —
poll `last_full_sync_at`, which is written on success alone. The defect this closes is
[`REVIEW-LOG.md` LS-152](../REVIEW-LOG.md); the producer is LS-180.

### 5.2 There are five phases on a healthy run, and they are not a progress scale

⚠️ **This subsection used to say "exactly four phases" and list `containers` · `items` · `credits` ·
`done`.** The `files` phase shipped with `streamAndApplyFiles` and this page was not updated with it;
the SPA has had a `files` arm in `progressCounts` for longer than the paragraph above it was wrong.
The correction is `REVIEW-LOG.md` LS-270; the four line numbers this paragraph used to carry were
already stale before that, which is why none are cited now.

`containers` · `items` · `credits` · `files` · `done`, published from four functions in
`internal/libsync/importer.go`:

| `phase` | Published by | How often |
| --- | --- | --- |
| `containers` | `FullImport` | Exactly once, after the container list is read and before anything is bound. |
| `items` | `streamAndApply`'s `flush` | Once per **committed** batch — and **none at all** when nothing was read, because `flush` returns early on an empty batch. |
| `credits` | `streamAndApplyCredits`'s `flush` | Once per committed credit batch. **Skipped entirely** when the Source is not a `CreditSource`, or when no item committed. |
| `files` | `streamAndApplyFiles`'s `flush` | Once per committed file batch. **Skipped entirely** when the Source is not a `FileSource`, or when no item committed. |
| `done` | `FullImport` | Exactly once, and on success only — after `StampFullSync`. |

🚩 **Five is a ceiling, not a shape.** Three of the five are conditional, so a wholly healthy run over
a source that implements neither optional interface publishes `containers` and `done` and nothing
between them. A client that blocks waiting for a phase it was never going to be sent waits for ever;
the phases are ordered, and that is the only sequencing promise on offer.

A phase is carried as the string the server sent. A client that has not heard of a phase should say
less about the frame, not drop it — which `progressCounts`
(`web/src/lib/services.ts`) already does, rendering an unrecognised phase's counts under a `default`
arm that declines to name it. **§5.5's `stopped` is a sixth phase value and is emitted today**, but
from `cmd/usarr` rather than from any of the sites above — which is why a healthy run still shows
only these five, and why a client must handle both a `stopped` that arrives and one that never does
(§5.1's three silences).

### 5.3 Only the two per-item passes send a `total` — `credits` and `files`

⚠️ **This subsection used to read "Only `credits` ever sends a `total`" and cite one line number for
it.** The `files` publish sets one too, on the same terms (LS-270).

`total` is `omitempty`, and the only two sites that set it are the `credits` and `files` publishes
inside `streamAndApplyCredits` and `streamAndApplyFiles`. Both set it to the length of the request
list the pass was handed — **the items that reached a committed batch**, which is UsArr's own count
of the work that pass has to do, not a figure the upstream reported. `containers`, `items` and `done`
send no total at all, because Kavita reports its own item total in a `Pagination` header that is
middleware and is absent from its OpenAPI document.

🚩 **Absent means unknown. It does not mean zero, and it is not a denominator to fall back on.**
Three of the five phases can never fill a progress bar, which is why the Services screen renders a
sentence with real counts rather than a bar.

### 5.4 `applied` counts a different thing in each phase — and is not `total`'s numerator

| `phase` | `items_read` counts | `applied` counts | `total` |
| --- | --- | --- | --- |
| `containers` | nothing yet — always `0` | nothing yet — always `0` | absent |
| `items` | catalogue items read from the source | catalogue items in a **committed** batch | absent |
| `credits` | credit sets read | **credit rows written** | credit *requests*, i.e. **items** |
| `files` | file sets read | **file rows written** | file *requests*, i.e. **items** |
| `done` | catalogue items read, final | catalogue items applied, final | absent |

**`items_read` is a RUNNING count in every streaming phase, not a figure settled when the phase
ends.** It is incremented per item handed over, so a frame published part-way through a phase
reports what has been read *at that moment* and the value climbs across a phase's frames. ⚠️ It did
not always: the counters were assigned from each stream's return value alone, which made every frame
but a phase's last one say `items_read: 0` while `applied` climbed past it — a running import
rendered as a stalled one (`REVIEW-LOG.md` LS-250). A client may rely on the value being
non-decreasing within a run; it may **not** rely on how many frames a phase sends, which is
`min(BatchRows, BatchWindow)` and therefore wall-clock dependent.

⚠️ **A phase's LAST frame is the adapter's own count, and it can exceed the running one.** Each
streaming pass overwrites its counter with the value the adapter returned before it flushes the
tail, because the two legitimately differ: `KavitaSource.StreamItems` returns what `StreamSeries`
handed *it*, which includes series in a **declined** library that are dropped before they ever reach
the importer's callback. So a final `items` frame reading `items_read: 6, applied: 5` over a
five-series import is not an off-by-one — it is one series in a library §17.8 declined, and the
Report says the same thing. `items_read` therefore means *how far the READ got*, which is **not** an
upper bound on what was applied for any reason other than batching.
`TestTheAdapterCountOfAReadWinsOverTheHandOverCount` pins it.

⚠️ **In the `credits` and `files` phases, `applied` and `total` are in different units**, so
`applied / total` is not a fraction of anything. One item can carry several credits, and several
files: the recorded frame above is a real run of two series where one had three credited people and
the other had none, giving `applied: 3` against `total: 2`. The ratio that is meaningful there is
`items_read / total`.

### 5.5 `stopped` — the terminal failure frame

✅ **Built.** This subsection was written as a contract *ahead* of its producer, so the consuming arm
had authoritative names to compile against rather than invented ones; the producer has since landed
and is held to every clause below by `cmd/usarr/import_stopped_test.go`. `grep -rn '"stopped"'
cmd/ internal/` reaches it at `cmd/usarr/import.go`'s `importPhaseStopped`. §5.1's paragraph and
§5.2's phase count are what it falsified, and both say so in place. §5.2 has since been falsified
again, by the `files` phase rather than by this frame: a healthy run publishes **five**, and
`stopped` is a sixth phase *value* that no healthy run ever carries.

#### 5.5.1 The frame — one new `phase` value and NO new field

It is the **same struct**, `internal/libsync`'s `Progress`: consumers parse one shape per event name,
and a second shape under one name is how a client ends up with two decoders and one of them wrong.
The frame gains **no field at all**. The only change is a fifth legal value of `phase`.

```jsonc
{"instance_id": 2, "phase": "stopped", "items_read": 41, "applied": 40}
```

| Field | On a `stopped` frame | Type | Notes |
| --- | --- | --- | --- |
| `instance_id` | **present** | integer | The instance whose import stopped. Same field, same meaning as every other phase. |
| `phase` | **present**, always the literal `"stopped"` | string | The discriminator, and the whole of the new information. |
| `items_read` | **present** | integer | Catalogue items read before the stop. Not a final count of anything — it is how far it got. |
| `applied` | **present** | integer | Catalogue items in **committed** batches, i.e. rows that **stand**. §5.5.4 is why this is the load-bearing field. |
| `total` | **absent** | — | `omitempty`, and only the `credits` and `files` publish sites ever set it (§5.3). A `stopped` frame is a different site, so absent — which means unknown, as it always does. |

**There is no `reason` field, deliberately, and §5.5.5 owns that decision.** No upstream message, no
status code, no error object, and no field for one to arrive in later without this section changing
first.

#### 5.5.2 The name is `stopped` because `failed` WAS TAKEN, in this product, with the OPPOSITE meaning

⚠️ **The collision below is HISTORY, and the argument still binds.** The SPA has since taken this
subsection's own recommendation: `SyncPhase` in `web/src/lib/services.ts` now spells the refusal
`not_started`, and no member named `failed` remains. The reasoning is kept as written because the
conclusion it reaches — **do not rename this frame to `failed`, now or later** — is what stops the
collision being recreated, and because `not_started`'s own doc comment cites this section by number.
Read the two paragraphs below in the past tense.

**As it stood when the name was chosen:** `failed` was a member of `SyncPhase` in
`web/src/lib/services.ts`, and `syncRefusal`'s `default:` arm returned it for the `500`
"could not be started" fault with the consequence copy `NO_IMPORT_STARTED` — verbatim:

> **`'No import started for this press, so the catalogue is untouched.'`**

**That is the exact negation of what this frame means.** A terminal stream frame says the import
*did* start, *did* run, and stopped partway — and per §5.5.4 the batches it committed **stand**, so
the catalogue is emphatically *touched*. One word, one product surface, two opposite claims about the
single fact a user most needs.

🚩 **And the two vocabularies genuinely meet in one field, so this is a structural collision rather
than a naming preference.** `syncNoteWithProgress` (`web/src/lib/services.ts`) folds a
stream frame into `SyncNote.phase` — it assigns `phase: 'finished'` on seeing `phase: "done"` — and
`syncButtonLabel` and `syncButtonBlocked` then switch on that same field. A wire `failed` folded into
that field would sit beside a refusal `failed` meaning the opposite, and every switch over it would
be correct for one and wrong for the other. This project has already paid for that shape once: two
constants named `outcomeSentUnknown` with different values, where a rename compiled clean because a
call site silently rebound to its twin.

**Why `stopped` and not the alternatives**, on the two tests that matter — does it collide, and does
it claim more than the frame knows?

| Candidate | Verdict |
| --- | --- |
| **`stopped`** | **Chosen.** No collision, and — decisively — it is the only candidate that does not assert *fault*. With no `reason` field (§5.5.5) the frame genuinely cannot tell a broken upstream from a clean shutdown, and a name that said "failed" would over-claim on every cancellation. |
| `aborted` | Rejected, and not merely on taste: "abort" is the database word for **rollback**, and §5.5.4's whole point is that committed batches are *not* rolled back. It contradicts the frame's most important fact. |
| `halted` | Rejected. Reads as stopped *by* something external, which is one cause of several. |
| `interrupted` | Rejected. Implies an external cause and a resumable operation; a re-run is a **full** re-import, never a resume. |
| `error` | Rejected. Asserts fault, like `failed`, and collides conceptually with the `ApiError`/`error`-code vocabulary §4.4 already owns. |
| `import_failed` | Rejected. Relocates the collision rather than avoiding it — a reader still has to hold two senses of "failed" apart. |

**One argument for `stopped` that does NOT hold, recorded so it is not repeated as support.** It has
been said that shipped copy already reads *"the import stopped"*, so wire and prose would agree. They
did not: at the time, `grep -n "stopped" web/src/lib/services.ts` found the word only in doc
comments, about a *stalled* readout. No user-facing string in `services.ts` said it. The name is chosen on
the two tests above, which stand on their own; the copy is the frontend's to write.

**Recommendation for the SPA's own `failed`, which is the frontend thread's call and not this
document's.** ✅ **Taken** — `SyncPhase.not_started` shipped, so everything from here to the end of
this subsection is the reasoning that produced it rather than a thing still owed. Renaming
`SyncPhase.failed` was **no longer urgent** — this frame avoids it, so nothing
is ambiguous today. It becomes worth doing at one specific moment: when the SPA folds `stopped` into
`SyncNote.phase`, that single union will hold both vocabularies, and a reader will have `failed`
(nothing started) beside `stopped` (started, then stopped) with no cue that the first is the stronger
claim about the catalogue. The accurate replacement is **`not_started`**, because that is literally
what its own `consequence` string says. Recorded as a recommendation; `web/` is untouched by this
change.

#### 5.5.3 `stopped` is terminal exactly as `done` is — for **one run**, and the stream has no run id

**Confirmed, with one correction a consumer must build for.** `done` publishes once, at the end of
`internal/libsync`'s `FullImport`, and nothing follows it for that run; `stopped`
gets the same rule. A later **silence**, a socket drop, a reconnect, or a `stream.missed` frame
therefore **cannot** downgrade a `stopped` a client has already seen — silence is `UNKNOWN` (§5.1),
and unknown never overwrites a fact.

⚠️ **A later non-terminal frame can, and legitimately does.** The frame carries `instance_id` and
**no run identifier** — unlike `search.*`, which carries `search_id`. So a fresh `containers` frame
for the same `instance_id` is not a retraction; it is a **second run starting** (§4's `POST`, or the
bootstrap). The rule a client implements is therefore: *terminal until a non-terminal phase arrives
for that `instance_id`, and that arrival begins a new run whose counters start over.* Two imports for
one instance cannot overlap — `cmd/usarr/import.go`'s `beginImport` guard makes them mutually
exclusive — so a client never has to hold two live runs for one id.

⚠️ **The frame is deliverable at most once and may be lost.** `Hub.Publish` never blocks and drops a
subscriber whose queue is full (`internal/httpapi/events.go:129`, `:166`); a client that reconnects
past the buffer gets `stream.missed` (`:263-268`) and the `stopped` frame with it. **So this frame is
better evidence, not authoritative evidence.** §4.3's table stays the authority on "did it succeed",
and a client that has seen `stream.missed` must fall back to it rather than to the last frame it
holds.

#### 5.5.4 It must not read as success, and the batches that committed still stand

**Confirmed against the tree.** `FullImport` reaches `StampFullSync` only after it has set
`rep.Completed = true`, so **`last_full_sync_at` does not move on any failure** —
it is written on success alone, which is what makes §3's timestamp the positive evidence and this
frame merely the fast notice.

⚠️ **Committed batches are not rolled back.** `streamAndApply` commits per batch and every error path
returns after the commits that already happened, so a stopped import leaves a **partially updated**
catalogue. On §3 that is the `last_full_sync_at: null` **with** `work_count > 0` row of §3.2's
table — a real state with a real meaning, not a contradiction to smooth over. The `applied` count on
this frame is that same fact arriving early.

So the copy has three jobs, and a client that drops any one of them is misreporting: **say it
stopped**, **say how much stands** (`applied`), and **say the catalogue is not known to be
complete**. Never "imported", never a tick, and never a count presented as a result. In particular it
must not reuse the `failed` refusal's sentence: *"the catalogue is untouched"* is false here, which
is §5.5.2 restated as copy.

#### 5.5.5 There is NO reason field, and there is no upstream text anywhere on this frame

**The frame carries no cause. The first version ships one phase value and zero new fields.**

**The rule, first, because it outlives this version: no `import.progress` frame may ever carry an
upstream message, response body, URL, status line, or any string derived from one.** That holds
whether or not a cause field is ever added.

**Why no field at all, rather than a field with a rule attached.** A field that no producer writes and
no consumer reads is not a seam, it is unbuilt feature surface — and the *absence* of a field is a
stronger guarantee than a rule about its contents, because there is nowhere for upstream text to land
in the first place. `CLAUDE.md`'s "cut before you add" and "the seam ships, the feature does not" both
point the same way. The frame's actual job is to make failure **distinguishable from silence**, which
is LS-152's defect; one phase value does that completely.

**This does not contradict ARCHITECTURE §17.3, because §17.3 already bounds itself.** Its verbatim
requirement is scoped to one column — *"| Problem | **The actual error text**, verbatim, not "an
error occurred" — **and nothing else.** The column holds one object: what the upstream said, or
`—` |"* — and the paragraph immediately below draws the boundary: *"**The `State` column is UsArr's
own words, in plain language.** The verbatim-upstream rule is right and it stops at the `Problem`
column"*. **A progress phase is a State, not a Problem** — a machine value a client switches on,
exactly the class §17.3 assigns to UsArr's own vocabulary. The frame was never covered by the
verbatim rule.

**And free text here would additionally break rules that are not §17.3's.**

1. **`reference/security.md` §5 forbids it.** Its deny-list redacts *"BEFORE any log line, audit row,
   error message, **SSE payload** or support bundle is produced"*. This frame is an SSE payload by
   construction.
2. **The redactor half of this argument has since closed; the other half has not, and it is
   sufficient on its own.** ⚠️ **This item used to read that `internal/kavita`'s `parseErrorBody`
   assigned the upstream body to `APIError.Message` through `truncate` alone, with no redactor, and
   that security.md §5's gap 1 was open. That is stale.** It closed in **LS-170** (`cdeb2f2`, on
   `main`): `redactText` was lifted to `ssrf.RedactText` with a one-line shim keeping the existing
   call sites byte-identical (`internal/httpapi/redact.go:102`); **every** branch of `parseErrorBody`
   now redacts and then bounds, through `clean` = `truncate(ssrf.RedactText(…))`
   (`internal/kavita/errors.go:218`, whose header states there is deliberately no branch that skips
   it); `service_instance.last_error` is redacted **before** the row is written rather than on
   read-out (`cmd/usarr/services.go:653-656`, `:670`, `:756`); and **both** of `cmd/usarr`'s slog
   handlers, text and JSON, are wrapped in a redacting `slog.Handler` (`cmd/usarr/main.go:247`,
   `:249`, `cmd/usarr/logredact.go`). security.md §5 marks gap 1 **met**.

   **The conclusion is unchanged, and the reason it is unchanged is worth stating.** Gap 2 is
   untouched: Kavita carries its key in a **path segment** (`/api/Opds/{apiKey}/…`), `internal/ssrf`'s
   path-segment redaction is a *shape* heuristic that §5 says matches that key only by luck and
   *"must not be relied on"*, so a cause fed from `err.Error()` still inherits it. And the
   load-bearing argument was never the redactor's coverage: **the absence of a field is a stronger
   guarantee than a rule about its contents.** A redactor that covers every path today is a property
   of today's code; no field is a property of the wire.

⚠️ **And the guard that looks like a bound is not one.** `cmd/usarr/import_e2e_test.go:519` sweeps the
whole stream dump, but `assertNoSecret` (`cmd/usarr/e2e_test.go:581-593`) is `strings.Contains`
against **secrets the test was told about**, on the error paths that suite happens to drive. It
catches a known literal on a covered path — not an unknown secret, not an uncovered path, not the
first unusual upstream error. "The handler only puts safe things in it" is a property of today's
handler, not a bound on the field.

**The precedent on this stream agrees.** `search.failed`, the only failure frame that exists, carries
a fixed UsArr-authored sentence and an action (`internal/httpapi/search.go:152-156`), never upstream
text.

⚠️ **Where the operator sees the real error instead**, since the frame will not carry it: the
**process log** (`FullImport` logs every failure with the instance id); **`sync_report`** rows for the
per-container detail recorded before the stop; and the **Services health row** — its `Problem` column
and §4.4's `500 internal` `message`, which is where §17.3's verbatim contract actually lives. Those
surfaces are governed by security.md §5's redaction requirement, and gap 1 above is now **closed** on
them (LS-170); keeping it closed is that section's work, and this frame is deliberately not a fourth
place that would have to be kept closed.

##### 5.5.5.1 If a cause is ever added, it is THIS shape and no other

Normative on *how*, deferred on *whether*. Adding it is a wire change and is written here first.

The field would be **`reason`** — `string`, `omitempty`, present only on `phase: "stopped"` — and its
value would be one member of the closed set below and **nothing else**. The user-facing sentence
stays the client's, taken from this table, which is the shape §4.4 already uses for its REST `error`
codes so a client has one vocabulary pattern rather than two. The test each member had to pass is
**does it name a different fix**.

| `reason` | What failed | Which paths map to it | What a client would say, and offer |
| --- | --- | --- | --- |
| `upstream_unreachable` | The source could not be reached, or did not answer in time. | Transport failures out of `Containers`/`StreamItems`, plus `internal/kavita`'s `ErrTimeout`, `ErrBreakerOpen`, `ErrWrongService`. | "UsArr could not reach this service." → `Test connection` |
| `upstream_rejected` | It was reached and it refused. | `ErrUnauthorized` (401), `ErrForbidden` (403 — a valid key whose account lacks the role), `ErrValidation`, `ErrNotFound`, `ErrUnexpectedStatus`. | "This service refused the request." → `Update API key` |
| `upstream_error` | It answered, but not with a usable catalogue. | `ErrServer` (5xx), `ErrDecode`, `ErrResponseTooLarge`. | "This service returned an error." → `Test connection` |
| `credential_unavailable` | The **stored** credential would not open — nothing was sent upstream. | `g.entry` at `cmd/usarr/import.go:98-101`; one of §5.1's two former pre-importer silences, and the cause §4.4 maps to `500 internal`. | "UsArr could not open the saved credential for this service." → `Update API key` |
| `not_a_catalogue_source` | The instance has no catalogue to import. | `cmd/usarr/import.go:102-107`, the other former pre-importer silence. **Same spelling as §4.4's `409`**, so one cause has one name across two surfaces. | "This service has no library to import." → `—` |
| `local_store_error` | The **local replica** could not be written. Not the upstream's fault. | Every `im.Store.*` return in `FullImport`: `BindContainers`, `RecordSyncReport`, `ApplyCatalogueBatch`, `ApplyCredits`, `Analyze`, `StampFullSync`. | "UsArr could not write to its own database." → `—`; it is a disk or database problem and the log has it |
| `cancelled` | Stopped deliberately — shutdown, or a cancelled context. | `ctx.Err()` on any path. Not a fault of either side, and not a reason to tell anyone to fix anything. | "The import was stopped before it finished." → `Run full sync now` |
| `unknown` | It did not classify. | The floor, and **mandatory**: without it a handler meeting an unclassifiable error has two exits — invent a member, or reach for free text — and the second is what this whole rule exists to prevent. | "The import stopped before it finished." → `—` |

**A value not in the table must be read as `unknown`**, never dropped and never rendered raw — §5.2's
discipline for an unrecognised phase, applied to an unrecognised reason.

**Until such a field exists, a client says the import stopped and says nothing about why.** Inventing
a cause from the last phase seen, or from the counters, is how a disk error gets reported to a user as
a broken API key.

#### 5.5.6 What the producer owed, and what it did

The two guards that would not have noticed this frame on their own, and how the producer that landed
answered each:

* `internal/httpapi/fixture_shape_test.go`'s `importProgressFrame` is a **hand mirror** of `Progress`
  and its own comment puts drift detection at *"the next regeneration"* of
  `web/src/lib/__fixtures__/sse-frames.json`. Since §5.5.1 adds **no field**, that mirror needed no
  change and got none — which is one more reason the fieldless version was the cheap one.
* **A healthy run produces no `stopped` frame**, so the recording cannot contain one — the same
  position that test already records for `search.failed` and `stream.missed`. Its shape is therefore
  unchecked by the fixture guard, and `assertNoSecret` over the stream dump proves nothing on a path
  the suite never fails. **`cmd/usarr/import_stopped_test.go` is the answer to both**: it drives four
  real failures — an upstream 500 mid-import, a non-catalogue service, and a sealed credential that
  will not open — asserts the frame's **whole key set** off the raw JSON (so a field added later is
  caught even though no Go struct would decode it), and runs `assertNoSecret` over the stream with
  the upstream 500's own body as the needle. It also pins the two negatives: a **successful** run
  publishes no `stopped` frame, before or after its `done`, and a second run supersedes an earlier
  `stopped` with ordinary phases (§5.5.3).

---

## 6 · `GET /api/v1/search` — search over your own library

Ranked full-text search across the works UsArr has replicated, as **one flat list** (ARCHITECTURE
§8.2, §17.4, and [`search.md`](./search.md) §3–§4 for the algorithm).

It is a local read (principle 1) — two SQLite statements, no \*Arr call, no metadata provider, no
image fetch. ARCHITECTURE §2073 budgets this path at **p50 < 15 ms, p99 < 50 ms**, which is why the
Prowlarr indexer fan-out no longer answers here: that one cannot meet the budget by construction
(§8.4) and now lives at `GET /api/v1/releases/search`. **The two are different questions over
different corpora.** This one searches what you *have* and answers in its body; that one asks
indexers what *exists* and answers `202` with results on the SSE stream.

Requires an authenticated session; without one it is `401 unauthorized`.

### 6.1 Query parameters

| Parameter | Type | Default | Behaviour |
| --- | --- | --- | --- |
| `q` | string | *required* | The search text. Empty or whitespace-only is `400 bad_request`. |
| `limit` | integer | `25` | The number of results **requested**. A clamp, not a validated range — see §6.3. Maximum `100`. |

**`q` is the only spelling.** `query=` is *not* accepted here, even though `GET
/api/v1/releases/search` accepts both; two spellings of one parameter is a contract a client has to
guess at.

**There is no `?lib=` scope chip and no per-type filter.** The scope this applies is the caller's
*access* scope, derived from the session, and a query parameter cannot widen it. A user-chosen
*narrowing* is a later commit.

**There is no cursor and no second page** — see §6.5.

### 6.2 Response

```jsonc
{
  "query": "frieren",          // the text the server actually searched, echoed back
  "limit": 25,                 // the cap the SERVER applied — authoritative, see §6.3
  "truncated": false,          // the 12-token cap fired; see §6.4
  "items": [
    {
      "id": 41,                // work.id — the id an item route takes
      "kind": "comic",         // work.kind verbatim: the schema's word
      "media_type": "comics",  // §17.2's six-value navigation enum: the user's word
      "title": "Frieren: Beyond Journey's End",
      "year": 2021,            // omitted when the column is NULL
      "added_at": "2026-08-16T12:00:00Z",  // omitted when the upstream reported no date
      "have_count": 43,
      "want_count": 17,
      "availability": {"k": "count", "have": 43, "total": null}
    }
  ]
}
```

**`items` is ordered by relevance and the ORDER IS THE CONTRACT.** No score is published. The score
is normalised per query, so a client comparing one across two queries would be comparing nothing;
publishing it would also freeze §6.6's ranking, which is expected to change.

**The item keys are §1.3's item keys**, deliberately, so one row component renders both Home's
recently-added table and a search result. Same omission rules: `year` and `added_at` are **absent**
rather than `0`/`null` when unknown, and `availability` follows §1.4 exactly — absent when there is
no blob, absent *and logged* when the stored blob is unusable.

🚩 **§1.4.1 applies here in full, and it is the rule most easily lost in a results table.** An absent
`availability` means **no count has been computed**, never that the user holds nothing, so a result
row must not carry an emptiness glyph or an accessible name like *none held* on the strength of a
missing key; *"not counted yet"* is the honest cell. `have_count` is sent unconditionally and
defaults to `0` in the schema, so **`have_count: 0` alone is not evidence** — it means something only
beside a present blob. This is not a property of the current tree: the counts and the blob are one
recompute over one dirty bit, so no specified writer can move the count while leaving the key
absent.

`media_type` is **omitted** for a work whose `kind` §17.2's enum has no row for. Today that is
`game` alone, and nothing writes a game work. It is omitted rather than invented because
[`search.md`](./search.md) §2 puts the corpus refusal at the *writer*, and a second allowlist in
the read would be a second vocabulary free to drift from the first.

**Nothing service-side is on this response.** No instance is named, no `remote_path` is read. Which
Kavita a series came from is not something a search result publishes.

### 6.3 `limit` is a clamp, and the echoed `limit` is authoritative

Identical in rule to §1.2, with this endpoint's bounds:

| Sent | Served |
| --- | --- |
| absent, empty, or `0` | `25` — "`0`" is spelled the same as "unspecified" on purpose |
| `1` … `100` | as asked |
| `> 100` (including `2147483648` and larger) | `100`, silently, and echoed back in `limit` |
| negative, or not an integer | `400 bad_request`, with an `action` |

A client reads the page size off the response, never off what it sent.

### 6.4 `truncated` means the query was cut, not the results

`true` says [`search.md`](./search.md) §3's **twelve-token cap** fired: the query had more than
twelve whitespace-separated tokens and the search answered a *prefix* of what was typed. §3 requires
a UI note for it, which is why the flag is on the wire rather than only in the log. It says nothing
about how many results came back.

### 6.5 There is no second page, and that is structural

The store fuses at most **200 candidates** and re-ranks *the whole of that set* in Go, so the
ordering is a property of the set rather than of any column. There is no keyset position a cursor
could name, and an offset would re-run the entire retrieval to throw the head away. **A caller asks
for up to 100 results and that is every result this endpoint has.** Deeper results are a narrower
query, or the external search engine [`FUTURE.md`](../FUTURE.md) defers.

### 6.6 What the ranking does — and, explicitly, what it does not do today

Stated in full because a consumer cannot see the code, and because inferring capability from silence
is how a screen ends up promising a feature the server does not have.

**It does do:**

* **Two retrieval engines, fused.** A `unicode61` full-text leg that folds diacritics (so `amelie`
  finds `Amélie`) and a `trigram` substring leg that does not. Their disagreement is deliberate: the
  fused answer is the union, which is better than either alone.
* **Titles, alternate titles, original titles, credited people and the overview** are all searched.
  A search for a creator's **name** returns the **work** — "Susanna Clarke" finds *Piranesi*. Titles
  are weighted far above people, and people far above the overview.
* **Reciprocal Rank Fusion** at k = 60, then a Go re-rank on Jaro-Winkler similarity against the
  normalised title (prefix-weighted, because people get the beginning of a title right) with a mild
  recency tiebreak.
* **Media-type diversity injection.** If the top 10 would otherwise be swept by one medium, the
  best-scoring result of each absent media type is promoted into it — `search.md` §4's answer to the
  case where a film's richer text buries the novella of the same name.
* **Scope, enforced server-side, in the index join.** Results are limited to libraries the caller
  can see *and* to instances the caller can see. A caller who can see nothing gets nothing, never
  everything.
* **Honour `library.enabled` and `library.include_in_search`.** A library with either flag off is
  not searched. Works filed under the reserved *Unfiled* library **are** searched — they are not a
  chip, but they are not invisible either.
* **Exclude soft-deleted works.** A work inside the tombstone window is not a result.

**It does NOT do:**

* **No grouping.** A film and its novelisation come back as two rows. `search.md` §4's grouped card
  — one card, per-medium availability — is derived from `work_relation` edges and nothing reads
  those edges yet. A client must not assume one row per title.
* **No "not in your library" section.** This endpoint only ever returns things UsArr has. Finding
  things you do *not* have is `GET /api/v1/releases/search`, over a different corpus, on the SSE
  stream.
* **No popularity signal.** `search.md` §4 lists one; the column is hardcoded to `0` for every row
  by the catalogue writer, because no source this project reads reports popularity. A fabricated
  ranking signal is worse than an absent one.
* **No `title_idf` penalty.** §4 lists one — the thing that would stop short common titles ("It",
  "Her", "Us") swamping a query. The column is hardcoded to `0`: it is a *corpus statistic*, and
  nothing computes one. **Short generic titles will over-rank, and there is nothing on the wire to
  compensate with.**
* **No `in_library` boost.** §4 calls it the single most user-satisfying signal. It is `1` for every
  row and cannot be otherwise, because the corpus holds only what the replica has. It becomes real
  the day the corpus also holds things you do not own.
* **No typo tolerance beyond the trigram leg.** `spellfix1` is deferred ([`FUTURE.md`](../FUTURE.md)).
  A single-character transposition often survives; a misspelled first syllable often does not.
* **No `overview` tuning.** The column is weighted lowest, and the weight is *reasoned rather than
  measured*: `work.overview` is empty for everything the one shipped catalogue adapter writes, so
  there is no real data to tune it against.
* **No search below the top level.** Seasons, episodes, tracks, comic issues and people are not in
  the corpus at all (`search.md` §2). A query for an episode title returns nothing, not a bad match.

**Queries shorter than three characters use only the full-text leg**, because SQLite's trigram
tokenizer indexes nothing shorter. A query that is a single one-character token matches nothing at
all — both legs decline it — and answers `200` with an empty `items`.

**Every metacharacter is safe.** `Fallout: New Vegas`, `AC/DC`, `Schindler's List`, `say "hello"`,
`*`, `AND`, `NEAR` and a bare `"` are all searched literally. They are not operators and they are
not errors.

### 6.7 Errors

| Status | `error` | When |
| --- | --- | --- |
| `400` | `bad_request` | `q` is absent, empty or whitespace-only; or `limit` is negative or not a whole number (§6.3). Both carry an `action`. |
| `401` | `unauthorized` | no session. |
| `500` | `internal` | the local read failed. **No user input can cause this** — every token is escaped before it reaches FTS5 — so a `500` here is a server bug, never something the caller should retry with different text. |

There is **no `404`** and no empty-result error. A query that matches nothing is `200` with
`"items": []`, which is a fact about the library and not a failure.

---

## 7 · `GET /api/v1/library` — the library grid

ARCHITECTURE §17.2's per-type grid and §17.8's library scope chip, as one endpoint. **The same
corpus and the same row as §1**, filtered by media type and by library, in one of three orders,
keyset-paginated.

It is a local read (principle 1) — one SQLite statement per page (two on the `added_at`/undated
boundary §7.5 describes), plus at most one small statement to resolve `?lib=` slugs. No \*Arr call,
no metadata provider, no image fetch. Requires an authenticated session; without one it is
`401 unauthorized`.

**It is a different endpoint from §1, not a superset of it.** §17.2 closes Block C at *one* table,
*one* order and *no* filters — a sixth media type adds rows to it, never a sixth region — so
`/library/recent?media_type=…` would put a filter on the endpoint whose whole design is that it has
none. The two share the row shape and the allowlist that builds it, and they page identically
(§7.4). They do **not** share cursors (§7.5).

### 7.1 Query parameters

| Parameter | Type | Default | Behaviour |
| --- | --- | --- | --- |
| `media_type` | enum | all six | One of `movies` · `tv` · `music` · `ebooks` · `audiobooks` · `comics`. Anything else is `400 bad_request` — **never a silently unfiltered page**. See §7.2. |
| `lib` | comma-separated slugs | every library | Library **slugs**, as `GET /api/v1/libraries` publishes them. An unknown slug is `400`; so is `?lib=` with nothing in it. At most 32. See §7.3. |
| `sort` | enum | `added_at` | One of `added_at` · `sort_title` · `popularity`. `year` is a legal `library.default_sort` value and is refused here, with the reason. See §7.6. |
| `limit` | integer | `50` | The page size **requested**. Identical to §1.2 — a clamp, not a validated range, and the response says what was actually applied. |
| `cursor` | opaque string | — | A token minted by a previous response's `next_cursor`. Never construct one; never edit one. See §7.5. |

There is **no cover art**: there is no image endpoint, so shipping `poster_asset_id` would be an id
the client cannot turn into anything. There are **no facet counts** beside the chips; each is its
own aggregate and its own read.

### 7.2 The parameter is `media_type`, and its vocabulary is **not** `work.kind`

🚩 **`kind` is a real column with twelve members and it ships on this wire, in every row, under its
own name.** `media_type` is §17.2's six-value navigation enum. They are different sets, not two
spellings of one:

| `media_type` | is | `work.kind` |
| --- | --- | --- |
| `movies` | | `movie` |
| `tv` | | `series` |
| `music` | | `artist` **and** `album` — two kinds |
| `ebooks` | | `book` **with at least one non-audiobook edition, or none at all** |
| `audiobooks` | | `book` **whose every edition is an audiobook** |
| `comics` | | `comic` |

Two of the six are the *same* kind separated by `edition.format`, which is why the split is resolved
server-side: §17.2 states the Tier 1 client index carries no format, so **a browser holding `kind`
cannot tell an ebook from an audiobook**. A parameter named `kind` that accepted `movies` and
rejected `movie` — or accepted `tv` and rejected `series` — would be two vocabularies wearing one
name on a response that publishes both.

**An unrecognised value is `400 bad_request`, never an unfiltered page.** Widening on a typo turns
"show me my comics" into "show me everything", which looks like it worked. The refusal carries the
whole vocabulary in its `action` and never echoes the value back.

⚠️ **ARCHITECTURE §13's budget table said `?kind=movie` until 2026-08-19** and has been corrected;
the budget itself did not change.

### 7.3 `?lib=` takes slugs, and an unknown one is a refusal

A slug is a library's **URL identity** — migration 0005 allocates it once from the name and keeps it
durable across a rename, so a permalink survives. `GET /api/v1/libraries` publishes it as `slug`.

```
GET /api/v1/library?lib=ebooks
GET /api/v1/library?lib=ebooks,audiobooks
```

| Sent | Result |
| --- | --- |
| absent | every library — the chip's cleared state is the whole catalogue |
| one or more known slugs | scoped to those libraries |
| any slug that names no library you can see | `400 bad_request` |
| `?lib=` empty, or only commas | `400 bad_request` — a scope was asked for and nothing was named |
| more than 32 slugs | `400 bad_request` |

**Every failure is a refusal and none is a clamp**, which is the opposite of `limit`'s rule and
deliberately so: dropping a slug **widens** the page, and dropping every slug removes the scope
entirely — so a bookmark to a deleted library would answer with the whole catalogue instead of
saying the library is gone. `limit` fails in the safe direction; a filter has no safe direction to
fail in.

**The error names no slug at all, and does not say how many failed either.** The wire body is the
same sentence whichever slug failed and however many did — the store's resolver counts the
unresolved slugs, but that count is wrapped into the logged error and never reaches the response. A
slug you cannot see is deliberately indistinguishable from one that does not exist, so the message
cannot be used to probe another user's library names. The reserved `Unfiled` library is never offered in the scope chip
(migration 0005), so `?lib=unfiled` is refused like any other unknown slug.

**A work in two of the named libraries appears once.** A work carries **one membership row per
library** it is filed in — §17.8's Audiobookshelf split, one upstream library offered as Ebooks
*and* as Audiobooks, is what puts one work in two of them — while a row here is work-keyed, so the
scope is an existence test rather than a join ([ADR-0051](../DECISIONS.md#adr-0051)). The duplicate
a join would return is per-**library**: `library_member`'s key carries `edition_id`, but the only
production writer hardcodes the `0` "whole work" sentinel, so membership is not edition-grained in
the tree today (`REVIEW-LOG.md` LS-213).

### 7.4 Response

```jsonc
{
  "items": [ /* exactly §1.3's row shape, field for field */ ],
  "limit": 50,
  "media_type": "comics",
  "sort": "sort_title",
  "next_cursor": "MXR2My5iZXJzZXJr"
}
```

`items` is **§1.3's row**, key for key — `id`, `media_type`, `kind`, `title`, `year`, `added_at`,
`have_count`, `want_count`, `availability` — with §1.4's rules for `availability` unchanged. Nothing
service-side appears here either.

`limit` is **authoritative** and works exactly as §1.2 says: a client pages against this number,
never against the one it sent. The clamp table in §1.2 governs both endpoints; it is written once.

`media_type` and `sort` are **echoed, and omitted when the request did not name them**, so absence
stays distinguishable from `"movies"` and from `"added_at"`. They are echoed because they are part
of what the cursor *means* (§7.5): a client restoring a deep link has to know which query its stored
cursor belongs to.

### 7.5 Paging, and why a cursor is not portable

Keyset, not offset. Walk it by following `next_cursor` until it is absent, exactly as §1.5
describes — **and send `media_type`, `lib` and `sort` again on every page**, unchanged. The server
does not remember them.

🚩 **A cursor is minted under one sort order and is refused under another.** All three orders encode
to the same-looking token and all three decode, so nothing but the discriminator inside stops a
popularity position being replayed as an `added_at` position — which does not error, it silently
starts the page in the wrong place and skips or repeats rows for ever. Changing the sort chip means
**dropping the cursor**; replaying it is `400 bad_request`.

🚩 **§1's cursors and §7's cursors are not interchangeable either**, in either direction. Both are
versioned and each decoder refuses the other's tokens.

⚠️ **Works with no `added_at` form a tail after every dated row** under the `added_at` order, and
reaching that tail is the cursor's job rather than the client's — §1.5's warning, unchanged. The
other two orders sort NOT NULL columns and have no tail.

### 7.6 The three orders, and the fourth that is refused

The vocabulary is `library.default_sort`'s own, verbatim, so a client that reads a library's default
sort out of `GET /api/v1/libraries` can send it straight back here.

| `sort` | Order | Available |
| --- | --- | --- |
| `added_at` *(default)* | newest first, then id | always |
| `popularity` | highest first, then id | always |
| `sort_title` | A→Z, then id | **only with a `media_type` of exactly one kind** |
| `year` | — | **never** |

**`sort_title` needs a single-kind `media_type`.** Its index is `(kind, sort_title, id)`, and SQLite
cannot supply `ORDER BY` from an index whose leading column is constrained by `IN`. `music` is two
kinds (`artist` **and** `album`) and no `media_type` at all is six, so those two combinations are
`400 bad_request` rather than a sort of the whole library on every page. The honest fixes are a new
index or splitting the Music grid into Artists and Albums; both are §17.2's decisions and neither is
built.

**`year` is refused outright.** `work.year` has no index of any kind, so the order would be a temp
b-tree over the whole filtered corpus on every page. It is named in the refusal rather than lumped in
with a typo, because a client sending it is most likely reading a library's own `default_sort` back.

### 7.7 Errors

| Status | `error` | When |
| --- | --- | --- |
| `400` | `bad_request` | `media_type` is not one of the six (§7.2); `lib` names a library that does not exist, is empty, or names more than 32 (§7.3); `sort` is not one of the three, or is `year`, or is `sort_title` without a single-kind `media_type` (§7.6); `limit` is negative or not a whole number (§1.2); `cursor` is not a token this endpoint issued, or belongs to another sort order (§7.5). Every one carries an `action`. |
| `401` | `unauthorized` | no session. |
| `500` | `internal` | the local read failed. |

There is **no `404`**. A filter that matches nothing is `200` with `"items": []`, which is a fact
about the library and not a failure.
