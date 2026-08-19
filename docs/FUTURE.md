# UsArr — Deferred features and the seams that keep them cheap

**Status:** none of this is in a roadmap milestone. **Nothing here is a rejection.**

This document exists because the alternative — deleting an idea from the design because it is not in
v0.1 — loses the reasoning along with the scope, and because building the base *without* knowing
what might land on top produces a base that fights every addition.

> **Build the base first, but build it with intentional space for what comes later.**

Every entry below has four parts: **what it is**, **why it is deferred**, **what it would cost**,
and — the part that actually matters — **the seam**: the specific, named place in the v0.1 design
that makes it cheap to add. Every seam listed here costs near-nothing to include now and is
materially more expensive to retrofit, which is the whole reason it is being written down before
there is code.

**A framing that governs all of this:** *UsArr is meant to coexist with the rest of the ecosystem,
not replace it.* Sonarr keeps doing acquisition, Jellyfin keeps doing playback, Navidrome keeps
being an excellent music server. UsArr is the layer that makes them one catalogue. Deferred features
that would move UsArr toward *replacing* a neighbour (a transcoder, a player) are permanent
non-goals and are **not** in this document. Everything here is additive to the layer UsArr occupies.

**Revisit discipline:** each entry names a **trigger** — a condition that makes it worth reopening.
Reopen on the trigger, not on enthusiasm. If an entry has no trigger, it is not ready to be
reconsidered.

---

## 1. A WASM plugin tier (Extism / wazero)

**What.** A third provider tier alongside compiled-in Go and declarative YAML manifests: user-dropped
`.wasm` modules implementing the same `Provider` interface, sandboxed, so a service needing real
control flow — a login handshake, a JSON-RPC envelope, XML parsing — can be added without a UsArr
release.

**Why deferred.** Not because the idea is wrong; it is the natural answer to the manifest's stated
ceiling (`reference/providers.md` §3.2). It is deferred on **staffing and blast radius**: a sandbox
that executes third-party code *inside the process that holds every \*Arr admin key* is a
security-critical subsystem, and this project does not yet have the capacity to review third-party
modules or to answer for one that misbehaves. Shipping the design before the base exists also
invites someone to build it first.

**What it would cost.** The runtime is the small part. The specification is the work, and all of it
is currently unwritten: a fuel/interrupt budget and a hard per-call `context.WithTimeout` (say 5 s);
`WithMemoryLimitPages` capped (say 64 MiB) — wazero does not bound CPU, wall-clock or memory growth
by default, so a `memory.grow` loop takes down the DB writer, the sync engine and every northbound
client on a Pi; one invocation at a time per instance behind a semaphore; a host-function API
(`http_request`, `log`, `kv_get`/`kv_put`) with a **decided and stated** answer to the question that
determines whether the sandbox is real at all — *does `http_request` take a path with the host
attaching the credential, or does the plugin receive the decrypted key?* (It must be the former: a
plugin must never receive a decrypted credential.) Plus module provenance: an explicit admin enable
step showing the module hash, the hash recorded, and a refusal to load a module whose hash changed
without re-approval. Plus a decision on where `kv_*` lives (`cache.db`, namespaced, size-capped,
rate-limited — never `usarr.db`, where a plugin writing 4 GB blocks the single writer).

**The seam.** `reference/providers.md` §1: providers are resolved from a **registry of
`ProviderFactory` implementations**, and the sync engine never references a concrete provider type.
A WASM host is one more factory. `RemoteItem` is the neutral wire type every tier produces, so the
canonical mapper does not learn that a third tier exists. **Adding the tier changes zero code in the
sync engine** — that property is the entire reason the registry is written this way in v0.1.

**Trigger.** Tier 1 manifests have demonstrably hit their expressiveness ceiling for a service
people actually run *and* there is someone who can own sandbox security response.

**A correction to carry forward:** the old argument for WASM was "wazero is already a dependency via
`ncruces/go-sqlite3`, so the plugin host is free." That is **no longer true** — the library moved to
the maintainer's `wasm2go` translator and now lists Go and `x/sys` as its only direct dependencies.
A WASM host is a **new** dependency with its own cost, and the case must be argued on wazero's own
merits (pure Go, zero CGO, real sandboxing), not on shared-runtime economy.

**New evidence, in the other direction:** **Navidrome has shipped an Extism + wazero plugin system**
(`plugins/` on `master`). That is a reference implementation in the same language, in the same
domain, on the same hardware class — which does not change the deferral (the reason was staffing and
blast radius, not feasibility) but it does change the *trigger*: it is now possible to read a working
answer to the host-function and sandbox-budget questions above rather than inventing one. Read it
before writing any of this.

---

## 2. An external search engine (Meilisearch, or another)

**What.** A `SearchProvider` implementation backed by an external engine, for users with very large
libraries or a taste for better fuzzy matching than FTS5 can give.

**Why deferred.** The design's own numbers argue against making it a shipped tier today: the library
is ~50k short strings, not 50M documents, and a 200 MB search server for that is disproportionate on
a Pi or a NAS. It also collides with the "no required sidecar" rule if it ever drifts from optional
to expected. And a named tier with config variables and a README row implies someone will build and
support it, which nobody has agreed to.

**What it would cost.** A provider implementation, an index-sync path (every `work` write has to
reach it, which is a second write on the ingest hot path), a reindex/backfill job, failure semantics
when the engine is down (fall back to FTS5 silently? tell the user?), and a config surface. Plus
documentation for a second daemon.

**The seam.** Two, both already in v0.1:

1. **The `SearchProvider` interface boundary is retained** even though there is exactly one
   implementation. It costs one Go interface.
2. **Retrieval is separated from ranking.** The FTS5 hybrid is *two retrievers* fused by Reciprocal
   Rank Fusion, and the Go re-rank operates on a candidate list that carries no engine-specific
   score (`reference/search.md` §4). RRF exists precisely because BM25-over-unicode61 and
   BM25-over-trigram are not on a common scale — which means **an external engine is a third
   retriever, added as another leg of the fusion, with no change to ranking, diversity injection or
   the result shape.** That is the property worth protecting: the ranking layer must never learn
   where a candidate came from.

**Trigger.** A real user with a library where FTS5 latency or recall is measurably inadequate —
measured, on `make bench`, not assumed.

---

## 3. Typo tolerance (`spellfix1` / `editdist3`, or a Go-side index)

**What.** `dremas` → `dreams`. The v0.1 search does prefix and substring matching and **does not do
this**, and the documents now say so plainly rather than claiming a capability FTS5's `trigram`
tokenizer does not have (`reference/search.md` §1).

**Why deferred.** It needs its own retrieval leg and its own data structure, and its recall bound
has to be measured rather than assumed.

**What it would cost.** Either SQLite's `spellfix1`/`editdist3` — whose availability under
`ncruces/go-sqlite3` is ⚠️ unverified and is the first thing to check — with a vocabulary table
populated from `search_doc.norm_title` and refreshed on every title change (a third write on the
ingest hot path); or a Go-side symmetric-delete or BK-tree index, which avoids the extension
question but holds the vocabulary in memory and therefore interacts with the RSS budget.

**The seam.** The same one as §2: retrieval is separated from ranking, and the fusion takes N legs.
A typo-tolerant retriever is a fourth leg. Nothing above it changes.

**Trigger.** Measured user demand, plus a measured recall/latency result from a spike.

---

## 4. External identity: OIDC, passkeys, TOTP, forward-auth

**What.** OIDC with PKCE against Authelia/Authentik/Keycloak; WebAuthn passkeys for the web UI;
TOTP; forward-auth trusted-header integration.

**Why deferred.** v1 authentication is a local Argon2id account, an opaque server-side session
cookie, CSRF, and per-app API keys — and that fits the stated deployment (a tailnet, one household).
Each deferred item is a subsystem, not a setting: Authelia's access tokens are opaque by default so
you must call the introspection endpoint rather than validating a JWT locally; WebAuthn needs
resident-credential handling and **only works for the web UI**, so TOTP and device-authorization
flows have to exist anyway; forward-auth needs CIDR allowlists, header-name configuration and
ingress header-stripping to not be a footgun. Three OIDC providers to test against is a test matrix,
not a feature.

**What it would cost.** Per mechanism: a verification path, a session-establishment path, an account
linking/provisioning policy, and a way to turn it off safely. Plus the "which of these can a TV app
use" matrix, which is the question that actually constrains the design.

**The seam.** **Credential verification is isolated behind one interface from v0.1**, and there are
already three distinct verifiers behind it — Argon2id password, HMAC'd API key, and the tailnet
`WhoIs` path — precisely so that a fourth is an implementation rather than a rewrite:

```go
type Authenticator interface {
    // Returns the authenticated user, or ErrUnauthenticated. Never partially authenticates.
    Authenticate(ctx context.Context, r *http.Request) (*User, error)
    // Which surfaces this authenticator may serve. Enforced by the router, not by convention.
    Surfaces() []Surface // web | rest | opds
}
```

The `Surfaces()` half is load-bearing and is why the interface is worth having now: the rule that
**trusted-header and ambient auth must never reach the OpenSubsonic or OPDS surfaces** is enforced
structurally rather than by remembering. `user.auth_source` already carries `local|jellyfin|plex|
tailscale` and takes another value without a migration.

**Trigger.** A user who actually runs an SSO stack asks for it — and, for passkeys specifically, a
decision about what the TV and native clients do instead.

---

## 5. The cross-media Tier-3 fuzzy ladder and the review inbox

**What.** Candidate generation by trigram blocking over cross-kind works, a weighted score (title
similarity, creator overlap, temporal ordering, TMDB "based on novel" keyword, penalties for short
common titles), a calibration step, and a human review inbox for the 0.55–0.85 band with confirm /
reject / not-sure verdicts.

**Why deferred.** v1 stores only edges from authoritative sources at ≥0.85. The measured coverage
gap is real — ~6.1k book→film pairs have an Open Library ID on the book side, so mid-list adaptations
are missed — but **a wrong link is far worse than a missing one**, and the design conceded the point
itself: *"title-similarity-based adaptation guessing is a false-positive machine."* The review inbox
is the mitigation, and it is a second UI surface plus a per-user verdict store — a
false-positive-management product that nobody is staffed to run.

**What it would cost.** The scorer, threshold calibration against a labelled set (which does not
exist and would have to be built), the inbox UI, per-user verdict scoping, and permanent-rejection
semantics — a rejected pair that keeps reappearing is the fastest way to make the feature hated.

**The seam.** **`work_relation` already carries `confidence REAL` and `evidence TEXT NOT NULL`**
(`reference/schema.md` §11) even though v1 writes only 0.90–1.00 values from authoritative sources.
Those two columns are exactly what a fuzzy tier would populate, and `evidence` is what makes a
review UI usable rather than a guessing game. Adding the ladder later is: write lower-confidence
rows, add `status`/`reviewed_by`/`reviewed_at`, build the surface. **No change to how edges are read,
grouped or rendered** — the grouped result card already filters on a confidence threshold.

**Trigger.** Users reporting *specific missing adaptations that Wikidata does not know about*, in
enough volume to justify a review queue — not a general wish for more links.

---

## 6. A Jellyfin-compatible northbound surface

**What.** Emulating enough of Jellyfin's server API for Swiftfin, Findroid and the Android TV client
to browse a *unified* catalogue.

**Why deferred.** The API is large, under active change (the 10.11 auth-header change broke clients;
the 10.11 OpenAPI schema is reported invalid), and its playback-negotiation surface is precisely the
part UsArr does not want to own. It is also the surface most likely to drag UsArr toward transcoding,
which is a permanent non-goal. Prior art exists — DroppedNeedle emulates both OpenSubsonic and
Jellyfin server APIs — so it is possible, just not cheap. It has been removed from the system
diagram, because a diagram is read as a commitment.

**What it would cost.** A large endpoint surface, device-profile handling, `PlaybackInfo` semantics,
and a client test matrix across platforms the developer may not own.

**The seam.** The gateway is already **surface-agnostic below the protocol layer**: the ID codec, the
authorization-on-resolution rule, capability negotiation and the byte proxy are all shared, and
`client_credential.protocol` is a CHECK-constrained enum that takes another value. A new northbound
surface is a new protocol adapter over the same gateway, not a new gateway.

**Trigger.** OpenSubsonic and OPDS both shipped and stable, *and* a concrete user need that a deep
link into Jellyfin's own client does not meet.

---

## 7. Video byte-proxying, and revisiting link-out

**What.** Today UsArr proxies bytes for its own OpenSubsonic and OPDS surfaces (audio, ebooks,
comics) and **links out** for video. The deferred alternative is carrying video bytes too, for
deployments where a client cannot reach the backend directly.

**Why deferred.** On the assumed tailnet deployment every client can reach every backend, so the
link-out costs nothing. Carrying video means a 4K remux at 60 Mb/s through a Pi's single NIC, which
is the whole box. Note carefully what is **not** deferred and never will be: **transcoding**. If
video proxying is ever added it is `io.Copy` and nothing else.

**What it would cost.** The `Range`/`Content-Range`/`If-Range` surface already exists for audio, so
the marginal code is small; the cost is bandwidth, goroutines and the support burden of being on the
byte path for large files.

**The seam.** The stream path is already a proxy, not a redirect, and already handles ranges
(`reference/gateway.md` §4). Extending it to video is a routing decision and a capability flag, not
new machinery. The reverse direction — going back to redirects — is the one that is now closed, and
deliberately: `jellyfin/jellyfin#10808` shows Jellyfin has no per-object scoped token, so a redirect
hands a client a full backend session credential.

**Trigger.** A deployment where clients genuinely cannot reach backends, reported by a real user.

---

## 8. Release calendars aggregated across every subscription

**What.** One calendar across everything you follow — upcoming episodes from every Sonarr, film
release dates from every Radarr, album release dates from Lidarr, book publication dates from the
book services — in one view, with per-media-type filtering, and exportable as an iCal feed a phone
calendar can subscribe to.

⚠️ **Which of those sources can actually supply a date, stated because the sentence above promises
aggregation over services that cannot.** **Only the \*Arrs expose a `/calendar` endpoint.** Lidarr is
v1.0, and the book, audiobook and comic sources — Audiobookshelf and Kavita, which arrive after v0.1,
then Komga — expose **no calendar endpoint at all**. So a catalogue-only source contributes `work.release_date`
where it happens to have one and nothing where it does not, and the calendar must be honest about
which media types it can cover rather than rendering four empty lanes. The **seam**
(`work.release_date`) is unaffected; the **promise** needed narrowing.

**Why deferred.** It is genuinely valuable and genuinely additive, and it is not v0.1 only because
v0.1 has one job: prove the replica thesis on a real library. It is also the kind of feature that is
cheap *if* the data model already holds the dates and expensive if it does not — which is why the
seam below is worth naming now.

**What it would cost.** A calendar query and a view, an iCal serializer, and a decision about how far
forward and backward the window runs. The data is already synced: every \*Arr exposes `/calendar`,
`work.release_date` and `work_episode.air_date_utc` are already columns, and `ix_ep_air` on
`work_episode(air_date_utc)` already exists. Realistically a small feature on top of an existing
replica.

**The seam.** Three things, all already in v0.1:

1. **`work.release_date` and `work_episode.air_date_utc` are stored**, with an index on the latter.
2. **`/calendar` is on the list of endpoints the importer already knows about**
   (`reference/sync.md` §2), so nothing new has to be synced.
3. **Dates are a `work` property, not a provider property** — which means the aggregation across
   instances is a plain query, and cross-media entries (a book publication date next to a film
   release) fall out for free rather than needing a second pipeline.

Keep it that way: resist any temptation to model calendars as a per-provider cache.

**Trigger.** After v0.2. It is a natural companion to requests — "what's coming that I already asked
for" — and it is the cheapest high-visibility feature in this document.

---

## 9. Per-user watch / listen / read statistics

**What.** What you watched, listened to and read; how much; when; trends over time; per-media-type
breakdowns. The unified version of what Jellyfin's playback reporting and Navidrome's play counts do
separately — plus the thing neither can do, which is **one view across video, music, audiobooks and
ebooks**.

**Why deferred.** It depends on northbound write-back and playback state, which land with the
gateway and multi-user milestones. Building the statistics before the events exist would mean
inventing the event stream twice.

**What it would cost.** A query layer and a view. The expensive parts are upstream of it: capturing
scrobbles and position updates reliably, and reconciling backend-owned position state with UsArr's
own. Both are already specified (`reference/gateway.md` §6).

**The seam.** Three decisions in the v0.1/v1.0 design that exist partly for this:

1. **`play_history` is append-only, one row per play event**, with
   `UNIQUE (user_id, work_id, edition_id, started_at)` — **not** a counter. **The `edition_id` is a
   correction ADR-0031 forced and this seam had not absorbed**: position is edition-scoped, and
   ARCHITECTURE §6.1 makes an audiobook an `edition` of a `book` work, so two editions of one work
   consumed on the same day collide under a work-keyed uniqueness constraint. The stated rule is that *play counts
   merge by union of events, never by taking a max*, because taking a max from a backend that reset
   would silently delete history. An append-only event log is exactly what statistics need, and a
   counter is exactly what they cannot be reconstructed from.
2. **Everything user-facing is `user_id`-scoped from migration 0001** (ARCHITECTURE §1.3), so
   per-user statistics do not require a schema change — only a query.
3. **`playback_state.owner`** records whether UsArr or the backend arbitrates a given item's
   position, so a statistics view can tell "we observed this" from "the backend told us this"
   instead of averaging two sources of truth.

**Trigger.** After the gateway milestone, when there are real play events to count. Do not build the
view before the events are trustworthy — statistics computed from a lossy event stream are worse
than no statistics, because people believe them.

---

## 10. Smaller deferred items

| Item | Why deferred | Seam |
|---|---|---|
| **Filtered audit UI** | The audit log ships in v0.1 as a plain paginated list; filters are polish | `audit_log` already carries `action`, `target_type`, `actor_user_id`, `result` — filtering is a query, and half of it is built: `store.ListAuditLog` takes a scope plus an `AuditQuery{Actions, Results}` for Recent grabs' not-sent arm, over `ix_audit_actor_action`. What is deferred is the screen, not the read |
| **Security-posture panel** ("you are exposed; 2FA is off") | Serves internet exposure, which is the secondary deployment | It is a read-only view over settings that already exist |
| **`sqlite-vec` semantic search** ("search by plot description") | Needs an embedding model — a runtime dependency or a remote call, both violating the no-sidecar rule. ⚠️ TMDB's ToU also names ML training/validation as commercial use, so embedding TMDB overviews is legally murky; prefer Wikidata or user-local text | Same retriever seam as §2 |
| **Download-client visibility** (SAB, NZBGet, qBittorrent, Deluge, Transmission) | Needs session-establishment and JSON-RPC transports that manifests cannot express (`reference/providers.md` §3.2) | Tier 0 providers via the same registry; Search-and-Grab already routes grabs through Prowlarr's own clients, so nothing is blocked meanwhile |
| **Recyclarr/Configarr automation absorption** | Proven collapsible by arr-dashboard, but it would dilute the speed story | UsArr already reads custom formats and is required never to clobber them |
| **Optimistic write apply with inverse-patch rollback** | Replaced by a durable command queue; it bought ~200 ms on a rare operation at the cost of the project's hardest correctness problem (ADR-0012a) | The queue's state machine already distinguishes `failed_rejected` from `verifying`, which is the hard half |
| **Cross-media "continue" row** spanning video, audiobook and ebook position | Depends on §9's event stream | `playback_state` is keyed **`(user_id, work_id, edition_id)`** across all kinds. ⚠️ **The work-keyed form previously recorded here was not a seam for this feature — it was its opposite.** The EPUB and the M4B of *Piranesi* are two editions of one `book` work (ARCHITECTURE §6.1), so `(user_id, work_id)` cannot represent "40% through the ebook, 12% through the audiobook", which is the entire content of this row. Corrected in `reference/schema.md`'s appendix |
| **Emby / Plex** | Emby is a feature-flagged Jellyfin variant; Plex paywalled remote playback of personal media on 29 April 2025 | Both are Tier 0 providers behind the registry |
| **Partial-freeleech display** (a 25% or 75% promotion shown as such) | Not visible on the surface UsArr reads. Prowlarr's JSON search derives its flags by **exact equality** on the volume factor — `0.0` freeleech, `0.5` halfleech — so a partial discount arrives with **no flag at all** and is indistinguishable from full price. Nothing to render, and nothing to defer *building*: what is deferred is reading a second surface | **The Torznab passthrough is the seam**, and it is where the number lives: `GET /api/v1/indexer/{id}/newznab` emits `downloadvolumefactor` / `uploadvolumefactor` as numeric torznab attrs, so it carries the 0.25 the JSON surface drops. Per-indexer, XML, output-only, no cross-indexer aggregation. `reference/arr-apis.md` §7.2 owns the citations. ⚠️ **Seam recorded, display not designed** — and the reading ceiling moves, not the conclusion: election is impossible on both surfaces, so no token control becomes possible either way |

---

## 11. Soulseek as a provider — and the several services it can be made of

> **The owner's framing, 2026-08-16, verbatim:** *"soulseek would be a provider (which could be
> materialized in soulseek or soulbeet or lidarr + slskd or just pure slskd)."*

**What.** Soulseek acquisition for music. Not "add slskd" — **one logical provider, `soulseek`, with
several possible concrete materialisations on a real install:**

| Materialisation | What UsArr actually talks to | What it holds |
|---|---|---|
| **pure slskd** | one slskd instance | search and transfers, no catalogue |
| **soulbeet** (slskd + beets post-processing) | one soulbeet instance | the same, plus tagged output |
| **Lidarr + slskd** | **Lidarr** — slskd is *Lidarr's download client*, invisible to UsArr exactly as qBittorrent is | catalogue, monitoring, and an \*Arr-shaped `Add` |
| **the Soulseek client itself** | nothing UsArr can address | out of scope until it grows an API |

**The point that generalises past music, and the reason this entry was rewritten:** *a request
destination must not assume an \*Arr shape.* Three of the four rows above are the same user asking
for the same album from the same network, and UsArr sees three different services with three
different interaction models — one of which is a plain \*Arr `Add` and two of which are not. A
design that reaches for `PUT /api/v3/…` because the destination is "a sink" has already lost.

**Why deferred.** It is still the best-fit non-\*Arr acquisition integration in the whole research
pass — AGPL-3.0 (the licence UsArr itself now carries), a clean REST API, an `X-API-Key` **request
header** which is precisely the credential shape UsArr already handles for the \*Arrs, and **no byte
involvement for UsArr at all**. It is deferred because **no write-capable service ships in v0.1 at
all** (§16), not because Soulseek is exotic. It joins Lidarr, LazyLibrarian and Mylar3 in that
bucket.

**What it would cost.** A provider with `Caps.MediaKinds = [(album, ""), (track, "")]` and **no
catalogue role** — slskd has no library to replicate. A queue verb that can express "search, then
enqueue from a chosen result". Queue polling on the delta channel. Nothing else: no byte proxying,
no import engine, no new protocol surface, no metadata provider.

**What it cannot do, stated so nobody expects it to.** slskd results are **filenames from strangers'
shares** — a path, a size, a bitrate hint and a queue position. There is no release group, no MBID and
no structured metadata, so **UsArr cannot turn a slskd result into a catalogue row**. It can only
record "you asked for X, slskd fetched these files", and any album-completeness claim after a Soulseek
grab is guesswork until Navidrome re-scans.

**The seam, and ⚠️ the half of it that is not there yet.** The provider registry plus the durable
command queue (ADR-0012a). **The property to protect is that the queue can express a two-phase
asynchronous operation** — if every verb is assumed to be a single request with a single outcome,
this becomes a rewrite rather than a provider.

Two of the three parts hold today, and the third does not. `library.sink_service_instance_id`
(`reference/schema.md` §6) takes any capable instance, so the sink binding is fine. The `Grabber`
interface (`reference/providers.md` §2) is already two calls — `Releases` then `Grab(rel)` — so the
verb shape is fine, and `write_queue.kind` is unconstrained free text, so a new verb needs no schema
change. **But `write_queue.state`'s `CHECK` has no legal value meaning "waiting for a human"**, and
that is the state a two-phase sink lives in between the phases. `pending` gets claimed by a worker on
its next pass; `inflight` asserts an outstanding upstream request that does not exist while a person
is deciding; and `verifying` carries a 15-minute `verify_until` TTL that ends in an explicit `failed`
— so a user who wandered off mid-choice would come back to their own request settled as
`fail_reason = 'unknown'`. This document previously listed the queue as the seam without qualifying
it. It was overclaiming: the queue protects the verb, not the wait.

**✅ The seam exists, and it is not the `CHECK` value this section used to promise.** The
library-sync migration **dropped `write_queue.state`'s `CHECK` entirely** rather than widening it to
add `'awaiting_choice'` ([ADR-0039](./DECISIONS.md#adr-0039)): the rebuild happened either way so both
cost the same, the vocabulary is demonstrably still growing, SQLite cannot `ALTER` a `CHECK`, and
`audit_log.result` and `provenance.acquisition_state` are the shipped precedent for enforcing that
class of vocabulary in Go instead. So the state is expressible today with no further migration, and
the seam is wider than a single literal would have made it. ✅ `'awaiting_choice'` is **excluded from
`ix_wq_runnable`'s partial predicate**, with the reason written beside it in the SQL — a row waiting
on a person is not runnable, and listing it would re-expose it to the retry sweep and the
`verify_until` TTL. That was an open question when this paragraph was written; it is decided.

Nothing implements any of it. No verb produces the state and no screen renders it: searched
2026-08-17 on `b8bb500` with `grep -rn "write_queue" --include=*.go internal/ cmd/ | grep -v
_test.go`, whose only hits under those two roots are the standalone RSS-spike binary in
`internal/db/spike/` and one comment at `internal/httpapi/grabs.go:58`. Roots searched are
`internal/` and `cmd/`; nothing is claimed about any other. The vocabulary now lives in Go, so it is
enforced wherever `internal/store` grows a write-queue writer — which it has not. **The seam ships,
the feature does not.**

---

### 11.1 Does the `sink_service_instance_id` seam actually hold? — tested, and the answer is split

This entry used to assert a seam (*"the queue's verb model can express a two-phase asynchronous
operation"*) without checking whether the v0.1 schema provides it. It was checked against
[`reference/schema.md`](./reference/schema.md) §10 and §13.1 and
[`reference/sync.md`](./reference/sync.md) §5. **Two of the three seams hold and one does not**, and
the one that does not is a `CHECK` constraint in migration 0001 — a merged migration, so the fix is
scheduled into the library-sync rebuild rather than edited in place.

**✅ 1. `library.sink_service_instance_id` HOLDS, and it holds for the right reason.** It is a
pointer to a `service_instance` row and nothing more. It encodes *which instance*, never *what shape
of conversation*. Every one of the owner's four materialisations resolves to **exactly one instance
row** — pure slskd is one, soulbeet is one, and *Lidarr + slskd* is one (Lidarr; slskd is Lidarr's
download client and UsArr never sees it). The thing that decides what a sink can do is the **live
capability probe**, which `reference/providers.md` §2 already forbids inferring from the service
kind: *"Never inferred from `kind` — a Sonarr fork, an old version, or a manifest-described service
may differ."* ARCHITECTURE §8.3's rule that the sink is *"a pin inside the capability filter, not a
bypass"* does the rest. **No widening needed. The column is materialisation-agnostic by
construction.**

**✅ 2. The provider interface HOLDS, and the two-phase interface already ships.** `Requester.Add`
returns `(remoteID string, err error)` — a single-shot signature that a two-phase destination cannot
implement honestly. But it does not have to: **`Grabber` is the two-phase interface**, and it is
already in v0.1 for Prowlarr Search-and-Grab —

```go
type Grabber interface {
    Releases(ctx, inst, q SearchQuery) ([]Release, error)   // phase 1: candidates
    Grab(ctx, inst, rel Release) error                      // phase 2: the chosen one
}
```

Soulseek is a `Grabber`, not a `Requester`. `Releases` maps to `POST /api/v0/searches` plus its poll;
`Grab` maps to `POST /api/v0/transfers/downloads/{username}`. The optional-interface set is doing
exactly the job it was designed for.

**❌ 3. `write_queue` DOES NOT HOLD.** Three specific things, in ascending order of cost:

1. **The verb vocabulary is free — that half is fine.** `write_queue.kind` is
   `TEXT NOT NULL` with `add|delete|monitor|unmonitor|grab|tag_add|refresh` in a **comment**, not a
   `CHECK`. A new verb costs nothing. Good.
2. **There is nowhere to put the candidate set, and `payload` is the wrong place.** A slskd search
   returns per-user file lists of arbitrary size that must survive between phase 1 and phase 2, be
   rendered on Requests, and expire. `write_queue.payload` is JSON and could physically hold it, but
   it is the hot queue table that `ix_wq_runnable` scans. This costs a table later and **no seam
   now** — a queue row can carry a reference as easily as a blob, so nothing needs deciding today.
3. **🚩 The `state` CHECK has no state for "waiting for a human", and it is in migration 0001.**
   This is the finding. ⚠️ **Read the correction under *"The seam that was added"* below before this
   item's present tense.** The `CHECK` quoted here is `00001_initial.sql`'s, and it is **no longer
   the shape on disk** — [ADR-0039](./DECISIONS.md#adr-0039) dropped it entirely instead of widening
   it. Everything below is preserved as the finding was written, because the finding's *reasoning*
   about the five states is what the fix acted on; only its assumption about the fix's shape is
   stale.

   ```sql
   state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN (
             'pending','inflight','verifying','done','failed')),
   ```

   A two-phase request whose search has completed and which is waiting for the user to choose a
   candidate is **none of those five**. It is not `pending` (no worker should pick it up), not
   `inflight` (no request is outstanding), and emphatically not `verifying` — `verifying` means *"it
   might have landed"* and carries a **15-minute TTL** (`verify_until`), after which
   `reference/sync.md` §5 forces one final refetch and then `done` or `failed`. A user who is asleep
   would have their request resolved to `unknown` by a timer. There is no legal state to park in, so
   the two-phase verb cannot be expressed at all — which is the precise failure this entry's old
   "seam" paragraph said must not happen, written down as a protected property but never actually
   protected.

   **Why this one costs a rebuild, and why that rebuild is free anyway.** `CLAUDE.md`: *a merged
   migration is never edited*, and `00001_initial.sql` is merged. Widening a `CHECK` on a shipped
   table therefore means a new migration and a table rebuild, because SQLite cannot `ALTER` a
   `CHECK`. **This entry first argued that made acting inside 0001 worth a one-time override of the
   rule — that argument was wrong, and it is retracted.** Its premise was that a later fix costs a
   full rebuild where today it costs one string literal; in fact the rebuild is **already
   mandatory**. `write_queue.work_id` in `00001_initial.sql` is a bare `INTEGER` whose
   `work_id → work(id)` foreign key is deliberately dropped until `work` lands with library sync, and
   SQLite cannot add a foreign key to an existing column either. The migration that ships library
   sync must rebuild this table regardless, so a `CHECK` value added during that rebuild costs
   nothing and no window is closing. See `REVIEW-LOG.md` WQ-03 — ⚠️ **whose premise survives and
   whose conclusion does not.** The premise (*the rebuild is already mandatory, so nothing is bought
   by editing 0001*) is what ADR-0039 was decided on and still stands. The conclusion it was used for
   — *add the `CHECK` value during that rebuild* — is superseded: the rebuild dropped the `CHECK`
   rather than widening it.

**The seam that was added, and how it differs from the one specified here.** Item 3 above — and the
paragraph that stood in this slot before this one, which said the same thing more directly —
specified that `write_queue.state`'s `CHECK` would *gain* `awaiting_choice` in the library-sync
migration. That migration instead **dropped the `CHECK`**, and both open questions above are now
closed —
[ADR-0039](./DECISIONS.md#adr-0039) carries the argument and the alternatives, `reference/schema.md`
§10 carries the corrected four-step list, and `internal/db/migrations/00005_library_sync.sql`'s
header carries it next to the SQL. `'awaiting_choice'` is excluded from `ix_wq_runnable`'s partial
predicate, as the lean here said it should be. The consequence for this section: the state is
expressible with no further migration at all, but it is enforced nowhere but Go — so the seam is
**wider and less self-policing** than the `CHECK` would have been, which is the trade ADR-0039 makes
explicitly. Nothing in v0.1 implements it, no verb produces the state, and the Requests screen has
no rendering for it. See `REVIEW-LOG.md` WQ-04 and WQ-05, both now dispositioned.

**The seam.** The provider registry, `Grabber`'s two-phase pair, and
`library.sink_service_instance_id`'s indifference to service shape — three parts that genuinely hold
today. The fourth, `write_queue.state`, does not, and is scheduled rather than done.

**Trigger.** A user who actually runs one of the four materialisations asks, *and* the request path
(v0.2) has shipped and settled.

---

## 12. Suwayomi as a comics catalogue source

**What.** `Suwayomi-Server` as a southbound catalogue source and byte owner for manga. It runs Mihon
extensions and pulls chapters from aggregator sites, so it is a library and a byte owner — and
**never a request sink**, which is a genuine structural difference from Mylar3 and Kapowarr.

**Why deferred.** Its API is **GraphQL** (`POST /api/graphql`), which ARCHITECTURE §11.2 puts firmly
outside the manifest tier: a manifest describes a read-mostly JSON-over-HTTP service with stateless
auth, and GraphQL is neither shape. So this is a **Tier 0 Go adapter**, which is the expensive kind,
for a service that overlaps heavily with Komga and Kavita for users who already run one.

**What it would cost.** A Tier 0 provider with a GraphQL client, a query set, and its own pagination
model. ⚠️ Its endpoint path, its three auth modes and whether the legacy REST API at `/api/v1` still
exists were **all unverified** — the project wiki failed to render and the facts came from secondary
sources. Verify before scoping.

**The seam.** The provider registry (§1). A Tier 0 provider is a factory; the sync engine never names
a concrete type. `Caps` already carries the ability to advertise `LibrarySync` without `Add`, which is
exactly Suwayomi's shape.

**Trigger.** A user running Suwayomi *without* Komga or Kavita, so it is the only comics catalogue
they have.

---

## 13. OPDS 2.0, after 1.2 ships

**What.** `application/opds+json` feeds alongside the Atom ones.

**Why deferred behind 1.2 rather than instead of it — re-argued 2026-08-16, because the fact this
rested on stopped being true.** The previous reason was that **KOReader, the single most important
client for a self-hoster with a Kobo or Kindle, does not speak 2.0**, citing `koreader#14681` as an
*open* feature request. ✅ **Verified against the issue and the release notes: #14681 is Closed,
milestone 2026.07, and KOReader 2026.07 "Sailing Walrus" shipped `OPDS 2.0 basic support`
(PR #15696), with 2026.07.1 following up on the OPDS 2.0 HTTP header and the author field.** So
"KOReader does not speak 2.0" must not be repeated anywhere.

**The ordering survives on the half that was always the stronger ground, and only that half: the
long tail is entirely 1.2.** Aldiko, Moon+, MapleRead, FBReader and Marvin all speak 1.x and nothing
else, and Komga is the only server in the whole survey that serves both. A 2.0-only surface excludes
every one of them; a 1.2 surface excludes nobody, KOReader included, since it gained 2.0 in addition
to 1.x rather than instead of it. **What has changed is that shipping 2.0 second is now a real gain
for a real client rather than a hedge** — which raises this entry's priority without reversing its
order. ⚠️ Re-verify before the milestone is scoped; this claim has already moved once.

**What it would cost.** One route returning `application/opds+json`: a `title` in `metadata`, a
`self` link, and at least one collection with role `navigation`, `publications` or `groups`. Search
and pagination are optional in 2.0. Genuinely small — which is why it is a follow-on rather than a
milestone.

**The seam.** The gateway is surface-agnostic below the protocol layer: the ID codec, the
authorization-on-resolution rule and the byte proxy are shared, and `client_credential.protocol` is a
CHECK-constrained enum that takes another value. **1.2's requirements are the demanding ones and they
are what the byte proxy must satisfy anyway** — `Range` support on acquisition URLs is *not* optional,
because KOReader and several iOS readers resume partial downloads, and paging via `rel="next"` is
mandatory in practice because KOReader chokes on 5,000-entry feeds.

**Trigger.** OPDS 1.2 shipped and stable, plus a client that actually wants 2.0.

---

## 14. Chaptarr, if it stabilises

**What.** Chaptarr as a book request sink and a secondary catalogue source.

**Why deferred.** Its **data model is the closest thing in the ecosystem to what UsArr wants** — a
real `Book` → `Edition[]` split with `HardcoverBookId` / `OpenLibraryWorkId` / `GoodreadsWorkId` on
the work and `Isbn13` / `Isbn10` / `Asins[]` / `Narrator` / `DurationSeconds` / `ChapterCount` on the
edition — and it is Servarr-shaped (`/api/v1`, `[V1ApiController]`), so the adapter would be nearly
free. It is deferred on three specific things, not on quality: it is **self-declared beta**; its
metadata resolution runs through **a centralised `api2.chaptarr.com`**, which is architecturally the
same single-point dependency that killed Readarr and is a privacy consideration the Services screen
would have to surface; and it is an *acquisition manager*, so its "library" is what it acquired
rather than what is on your shelf.

**What it would cost.** A Tier 0 or manifest adapter over a familiar shape. ⚠️ Unverified: its exact
auth header (`X-Api-Key` is inference from Servarr convention plus its README's "constant-time API-key
comparison") and whether it exposes an `updatedAt` or delta filter at all.

**The seam.** The provider registry, plus one property worth protecting deliberately: **the ingest
normaliser must be able to accept a work id and an edition id from the same response**. Chaptarr is
the only service in the survey that hands both over at once, so where a user has it, its identifiers
should be *preferred* in the §6.4 cascade. Designing the normaliser so it can do that costs nothing
now.

**Trigger.** Chaptarr leaves beta *and* either self-hostable metadata arrives or the central
dependency is judged acceptable and documented.

---

## 15. MangaBaka as a manga metadata provider

**What.** A metadata provider for manga identity, filling the gap Comic Vine and Metron leave.

**Why deferred.** v0.1 ships **zero external metadata providers** by design, and manga identity is
mostly obtainable from the backends themselves where Kavita+ is present. This is a provider, and
providers are the thing the roadmap deliberately postpones.

**What it makes attractive when the time comes.** No auth at all; 180 req/min (30/min for search);
cached GETs free — and, the part that actually matters, **nightly JSON / JSONL / SQLite dumps**. A
SQLite dump is the same shape as the Wikidata edge artifact (ADR-0007): a file UsArr can ship or fetch
per release, query locally, and never put on a render path. Licence is **CC BY-NC-SA 4.0**, which is
usable for a non-commercial self-hosted tool and must be attributed. Contrast the alternatives:
**Comic Vine's terms are incompatible with a first-party UsArr provider** (non-commercial only, no
reproduction in other formats, 200 req/resource/hour plus velocity detection) — UsArr may *store*
Comic Vine ids that other tools produced and should not fetch from it; **GCD has no API at all**, only
a registration-gated bulk SQLite download with no cover URLs, and ⚠️ its licence could not be
determined because `comics.org` is behind a Cloudflare challenge; **AniList** is the best free manga
identity source with adaptation edges, but ⚠️ its documented rate limit could not be read (403 to
automated fetches) and it has run degraded well below its nominal figure for long periods.

**The seam.** The provider registry, and `external_id.source` — `mangabaka` is a value, not a schema
change. The dump-shipped-per-release pattern is already established by ADR-0007.

**Trigger.** UsArr ships any external metadata provider at all, at which point this is the one to
start manga with.

---

## 16. The cross-media ebook ↔ audiobook link

**What.** Knowing that the EPUB and the M4B on your shelf are the same book — one card, two formats,
one "you own this" answer.

**Why it is here rather than in the roadmap as a solved thing.** The *schema* already handles it:
ARCHITECTURE §6.1 makes an audiobook an `edition` of a `book` work, which is exactly the shape. What
is deferred is the **work-level identity pass that populates it**, because **no backend supplies a
work identifier at all.** Audiobookshelf gives `isbn` and `asin` and nothing else — and those are an
*edition* key each, one for print/ebook and one for audio, sitting on the same row meaning different
things. Komga and Kavita give ISBN. Calibre gives whatever the user's plugins wrote. Chaptarr does it
properly and only inside its own library. **ABS pairs an ebook with its audiobook only when the files
happen to be in the same folder**; everything else treats them as unrelated.

**So the honest framing is that UsArr computes this — no backend hands it over — and it is the single
most valuable book feature UsArr can offer.**

**What it would cost.** A resolution pass, off any render path, on a worker: ISBN →
`openlibrary.org/isbn/{isbn}.json` → follow `works[0].key` → `OL…W`; ASIN → Audnexus `/books/{asin}`
→ title + author → as above; no identifier → title + author, thresholded, written at tier-3 confidence
with `evidence`. Cached in `cache.db`, obeying Open Library's 1 req/s (3 with an identifying
`User-Agent`) and Audnexus's 100 req/min. **And it must follow `/type/redirect`** — a merged Open
Library work becomes a redirect record, so an OLID stored last month can resolve to a redirect today.

**The seam.** Three things, all already present: `edition.format` distinguishes `ebook` from
`audiobook`; `external_id` is `(source, value, work_id, edition_id)`, so an ISBN can be written to the
edition side and an OLID to the work side without a schema change; and `work_relation` carries
`confidence` and `evidence`. **The rule that keeps the seam usable is in ADR-0031 and §6.4: never let
an ISBN or an ASIN satisfy `ux_extid_work_strong`.** Break that and the pass has nothing to attach to.

**A companion requirement, and it is *not* deferred with the rest of this entry — it moved out.** A
visible **"not identified"** state: whatever the backend reports, UsArr keeps the row — a book with
a title and a file and a quiet marker, still searchable. That single behaviour is what
LazyLibrarian's absence of disqualifies it as a catalogue, and it costs one nullable column and one
badge. **It is a v0.1 requirement**, not a v0.3-or-later one, **because it cannot be retrofitted** —
the nullable column belongs in migration 0001 and the badge in the first grid. ⚠️ **The frequency
claim that stood here is inverted, and the inversion is the point.** It read *"it is reached rarely
in v0.1, whose Sonarr and Radarr carry TVDB and TMDB ids, and becomes the ordinary case with the
first catalogue source"* — but [ADR-0041](./DECISIONS.md#adr-0041) made Kavita the first catalogue
source **and** put it in v0.1, and [ADR-0045](./DECISIONS.md#adr-0045) moved Sonarr and Radarr to
v0.2, so the two halves of that sentence swapped milestones: free Kavita's identifier fields are
null and Komga supplies **no external identifiers at all**, which makes "not identified" the
**ordinary** case from v0.1 rather than the rare one. What that changes is not the requirement — it
was always v0.1's, on the same not-retrofittable grounds — but how hard v0.1 exercises it.
`internal/libsync` is where the null-identifier path is actually taken. It now lives in
ARCHITECTURE §6.4 as a rule; it is recorded here only so the connection is not lost.

**Trigger.** ⚠️ **Rewritten, because the previous one — *"the milestone that ships any book
catalogue source with real user data behind it"* — was satisfied by v0.1** under ADR-0032, which
would have made this a deferred entry whose reopening condition fires one milestone *before* the
roadmap line that used to claim it. (⚠️ The parenthetical here read *"under
[ADR-0036](./DECISIONS.md#adr-0036) no book catalogue source ships in v0.1 at all, so the old trigger
would now fire later"*. [ADR-0041](./DECISIONS.md#adr-0041) amended ADR-0036 and put **Kavita** — a
book, comic and manga source — back into v0.1, so the old trigger is back to firing at v0.1 and the
round trip changed nothing: it was the wrong *kind* of condition each time round, being about data
rather than about machinery, and the rewrite stands on that. A trigger that moves whenever the
roadmap does is the defect the rewrite removed.) **The trigger is now: after v0.3, once the Wikidata edge pipeline has
proved the confidence/evidence path on real data.** That is the machinery this pass writes into, and
until it exists there is nowhere to put a computed link.

**ARCHITECTURE §16's v0.3 line is corrected to match**, and now says that the ebook↔audiobook link is
the case v0.3 does **not** solve rather than describing it as v0.3's flagship. One feature had three
homes — a roadmap line, this entry, and a trigger that fired before both — and §16 is authoritative,
so this entry says plainly that it is not on a milestone.

---

## 17. Per-library OPDS feeds

**What.** One OPDS feed per UsArr library, rather than one feed over everything.

**Why deferred.** The first OPDS surface should prove the *unified* catalogue — one feed spanning
Audiobookshelf's audiobooks, Calibre's ebooks and Komga's graphic novels under one credential, which
is the thing no single backend can offer and which, for an ABS user, is the **only** first-party-ish
OPDS path that exists at all (ABS has no OPDS router; the feature requests are closed as not planned).
Splitting it per library is a refinement of a surface that does not exist yet.

**What it would cost.** Routing and one extra navigation level. Kavita's surface is already
library-shaped (`/api/Opds/{apiKey}/libraries`, `…/libraries/{libraryId}`), so the shape is known.
⚠️ Note what **not** to copy from it: Kavita carries the API key **in the URL path segment**, which
leaks into proxy logs, browser history and referrers.

**The seam.** `library` is owed by v0.1 (ADR-0026) with `include_in_search` and `display_order`
already on it — read `internal/db/migrations` for whether the table exists yet — and
`user_library_access` is the multi-user half. An OPDS root of "one
entry per library" is then a query, not a redesign — which is the same seam that makes v0.4's
`getMusicFolders` return the user's `artist`-kind libraries.

**Trigger.** The OPDS surface has shipped and a user with several libraries asks for the split.

---

## 18. `provenance.service_instance_id` — naming which Prowlarr a grab came from

**What.** A column on `provenance` recording the `service_instance` a grab was dispatched to, so a
Recent-grabs row can say *which* Prowlarr produced it.

**Why deferred — and this is the judgement, not an oversight.** Recent grabs (§17.5) is a union:
successes and sent-unknown come from `provenance`, definite failures from `audit_log`. **The failure
arm can already name its instance** — `audit_log.metadata_json` carries `instance_id` on every grab
row, written by `internal/httpapi/grab.go`. Only the `provenance` arm cannot, because migration 0001
gave the table `indexer_name` and `indexer_id` but no service reference.

Four things follow, and together they say *record it*, not *migrate now*:

1. **The union works without it.** This is a display gap on one column of one arm, not a correctness
   gap. Nothing joins wrongly, nothing is attributed to the wrong user, no read fails. That is the
   bar the task set — *"if the column is genuinely needed for the union to work"* — and it is not
   met.
2. **A migration would not close the gap for existing rows.** `provenance.user_id` could be
   backfilled to `0` because `0` is a *real* user row that migration 0001 creates and the canonical
   read predicate already understands. There is no equivalent sentinel in `service_instance`, and
   inventing one would be a row that names no service. So the column would be `NULL` for every
   pre-existing acquisition and the UI would still have to render "unknown" — the migration starts
   closing the gap going forward and does nothing for the history already on disk.
3. **It would carry no referential integrity anyway.** `provenance` is immutable history, so the
   column would take **no** foreign key — exactly the argument migration 0002 wrote out for
   `provenance.user_id`, and 0001 before it for `audit_log.actor_user_id`: `CASCADE` destroys the
   history the table exists to keep, `SET NULL` erases the actor and cannot be spelled on a
   `NOT NULL` column, and `RESTRICT` makes deleting a service impossible. A bare `INTEGER` with no
   constraint is precisely as much integrity as the `audit_log` metadata already gives, at the cost
   of a fourth migration.
4. **Cut before you add.** v0.1 has one indexer service in practice, and §16 funds no multi-Prowlarr
   feature. A migration whose entire benefit is one column of one row in a configuration nobody has
   yet is the "and also" this project is told to refuse.

**What it would cost.** `ALTER TABLE provenance ADD COLUMN service_instance_id INTEGER` — nullable,
no default, **no `CHECK`**, no `REFERENCES`, following 0003's pattern and its no-`CHECK` reasoning.
Plus the write site in `internal/releases`, which already holds the instance id at the moment it
inserts the row.

**The seam, and it is real rather than aspirational.** The instance id is *in hand* at every point
that would need it: `internal/releases` receives it before it builds the `Provenance` value, and
`internal/httpapi/grab.go` already writes it into the audit row on the same request. Nothing has to
be reconstructed later, which is the property that makes this a one-migration change whenever it is
wanted rather than an archaeology problem.

**Trigger — either of these, whichever comes first.**

* **A second indexer service becomes a configuration UsArr expects rather than tolerates.** Two
  Prowlarrs are *creatable* today — `service_instance`'s only uniqueness is `name UNIQUE`
  (migration 0001, `internal/db/migrations/00001_initial.sql:136`), nothing constrains `kind`, and
  both `POST /api/v1/services` calls return 201 — so a different name is the only thing in the way.
  ⚠️ **This entry used to say that each is then "searched separately". That is false, and the
  correction makes the gap larger rather than smaller.** `resolveIndexerInstance`
  (`internal/httpapi/search.go:364`) picks **exactly one** instance — `candidates[0]`, ordered by
  priority then name — and the second service is **never contacted**. Measured against two fake
  Prowlarrs: `P1 searchCounts=map[1:1 2:1]`, `P2 searchCounts=map[]`. So the live problem is not that
  a grab cannot be attributed to one of two *searched* services; it is that one whole service is
  **silently skipped** and no report names it. **That is a live gap in a supported configuration,
  and it is not this entry's to fix** — a `provenance` column records which instance a grab *went
  to*, never which instance the search *declined to ask*. It is tracked as **INST-01** in
  `docs/REVIEW-LOG.md`, and it is open with owners named. This trigger still fires on its own terms:
  when the Libraries screen (§17.8) binds more than one indexer, or when a user reports a
  Recent-grabs row they cannot attribute.
* **Library sync lands and the import join needs the source service.** Reattaching an imported file
  to the grab that produced it goes through `provenance.download_id`; if that join ever has to
  disambiguate by *service* rather than by download id, the column stops being cosmetic and this
  entry closes.

---

## 19. A per-user *Unfiled* library — the multi-user half of §7 invariant 5

**What.** Today there is exactly one *Unfiled* library, `library.id = 0`, `user_id = 0`. It is the
place the membership derivation files a work that belongs to no other library, which is what keeps
`search_doc`'s §7 invariant 5 satisfiable at all — a work visible through no library matches no
search scope and disappears for **every** user including its owner.

**The seam, stated as the defect it will become.** `ux_library_slug` and `ux_library_name` are both
`(user_id, …)`, so *Unfiled* is a per-user name that only user 0 has. A second user with a work the
derivation cannot place has **no** row to file it into: the derivation would have to file it into
library 0, which belongs to user 0, and library scoping is `user_id`-based — so the work is either
invisible to its own owner or visible to somebody else. Both outcomes are exactly the failure
invariant 5 exists to prevent, and neither is reachable in v0.1 because there is one user. **This is
not a bug today. It is a bug on the day the second user is created**, which makes it the multi-user
migration's business rather than a v0.1 fix hunting for a caller.

**What the later migration must do, specifically.** Not "consider per-user Unfiled" — the shape is
already determined by three things the current schema fixes:

1. **Create one `Unfiled` row per user, at user-creation time, in the same transaction as the
   `user` row** — and backfill one for every user that already exists. `id = 0` cannot be the
   mechanism twice, so the reserved *id* stops being the identity and `(user_id, slug='unfiled')`
   becomes it. `ux_library_slug` already makes that pair unique, so no new index is needed.
2. **Keep library 0 exactly as it is, and keep its `BEFORE DELETE` trigger.** It is user 0's own
   Unfiled — user 0 being the shared/system sentinel — and rows filed into it before the migration
   must not move. The trigger (`trg_library_unfiled_no_delete`) must GROW to cover the new rows:
   `WHEN OLD.slug = 'unfiled'` rather than `WHEN OLD.id = 0`. ⚠️ That widening re-opens the
   `audit_log` question the id-0 form was checked against — a trigger that aborts an implicit
   `ON DELETE CASCADE` write makes the parent row undeletable — and under `WHEN OLD.slug =
   'unfiled'` the parent is an **ordinary user**, so `DELETE FROM user` would fail for everyone.
   The migration must therefore either delete the Unfiled row explicitly ahead of the user, or drop
   the trigger in favour of a `library.is_reserved` column the delete path checks. **Decide it
   deliberately; the current trigger is safe only because library 0's owner is the one user nobody
   deletes.**
3. **Whoever writes the document builder owes the re-filing either way** (`reference/schema.md` §7,
   invariants 2 and 5). Per-user Unfiled changes *which* row it files into; it does not change that
   the schema cannot do the filing itself.

**Cost of doing it now: real, and the reason it is here instead.** A per-user Unfiled needs a
user-creation hook that does not exist (v0.1 creates one user, in a migration), and a second
reserved row nothing reads. Cost of doing it later: one `INSERT … SELECT` over the `user` table, one
trigger rewrite, and the decision in (2). Nothing about it is a one-way door.

---

## 20. Per-type catalogue coverage on the wire — the array §16's empty-state rule turns on

**What.** One capability array carried with each connected service on `GET /api/v1/services/health`:
the media types that instance *actually* supplies, derived at ingest from the containers the
instance itself reported. With it a screen can say *"Ebooks — your Kavita has no book library"*.
Without it, the most it can say is *"Ebooks — empty"*.

**Why deferred — and it is a wire gap, not a UI failure.** `ARCHITECTURE.md` §16.1's v0.1 entry
keeps the rule *every screen that would render a library says which source is missing rather than
drawing an empty one* as a standing requirement, and splits it into the half that is answerable and
the half that is not. **Answerable:** which source *will* populate a type — §16 is that mapping, it
covers all six types, and a renderer needs a constant derived from it rather than anything from a
server. **Not answerable:** whether an already-connected source covers a type. §16 says Kavita
covers books, so an install whose Kavita holds only comic libraries has an `ebooks` type whose named
source is connected and healthy with nothing behind it, and **nothing UsArr serves separates that
from an import that has not run**. `GET /api/v1/library/recent` returns `{items, limit,
next_cursor}` with no per-type facet (`recentWorksResponse`, `internal/httpapi/library.go`), and
`GET /api/v1/services/health` carries `kind` plus a `role` whose CHECK admits four values —
`library|acquisition|indexer|download_client` (migration 0001) — and no media-kind array
(`serviceHealthResponse`, `internal/httpapi/services.go`).

**Why it is not built now, which is a scope judgement rather than an oversight.** The gap only
became reachable at all when v0.1 gained a catalogue source: `mapLibraryType`
(`internal/libsync/kavita.go`) emits exactly two `work.kind` values, `comic` and `book`, so two of
the six media types have a source and four do not, and the rule has to hold on a mixed screen for
the first time. But v0.1 connects **one** catalogue source, and with one source the static half
carries the screen — Ebooks names Kavita, and the residual confusion is one milestone of polish
rather than a missing capability. The array earns its cost when several sources are connected and
the answer stops being derivable by hand. *Cut before you add.*

**⚠️ §8.3's `Caps.MediaKinds` is not this, and the resemblance is the trap.** Two independent
grounds, and the second survives even if the first is fixed. **It is unbuilt:** it occurs zero times
in Go, and its only non-prose occurrences are two SQL *comments* on
`library.sink_service_instance_id` (`internal/db/migrations/00005_library_sync.sql` and the schema
dump in `internal/db/testdata/`). **And it answers a different question:** §8.3 defines it as the
filter for instances that advertise `Add` for a `(kind, format)` — a **request sink**. §8.3's own
worked example is why the two must not be folded together: *"Navidrome is an excellent music
catalogue and cannot accept a request."* A design that reused `Caps.MediaKinds` for catalogue
coverage would make every read-only source look like a sink, which is the exact error §8.3 was
written to prevent. (`internal/servarr`'s `Capabilities` field is a third thing again —
`IndexerCapabilityResource`, Prowlarr's per-*indexer* newznab categories and search-param arrays,
`internal/servarr/resources.go` — and belongs to §8.5's search filter.)

**What it would cost.** The distinct `work.kind` values each instance's containers map to, retained
in a queryable shape on `service_instance` or a small join beside it; one field on
`serviceHealthResponse` plus its key allowlist; and the ingest-side write. No change to `work`, no
new sync channel, and **no upstream call on a render path** — the value is computed where the
containers are already being read, and served from SQLite like the rest of that row (principle 1).

**The seam, and it is real rather than aspirational.** The fact this array needs is **already
produced, once, at ingest**: `mapLibraryType` is documented in its own header as *"THE kind
decision, and it is UsArr's rather than Kavita's"*, and `library_source` already records the
container each binding came from as `container_kind` / `container_ref` (§6.5). So the per-container
kind is decided and the container is retained; what is missing is only the rollup and one wire
field. Nothing would have to be reconstructed from upstream later, which is the property that keeps
this a small change whenever it is wanted rather than an archaeology problem.

**Trigger — either of these, whichever comes first.**

* **A second catalogue source lands** (§16.1 #1, Navidrome). With two sources the static §16 mapping
  stops being a sufficient answer on its own: two instances of the same `kind` can cover different
  types, and the screen has to say which one is thin rather than which service the roadmap names.
* **A user reports a type that reads as empty while its source is connected.** That is precisely the
  confusion this removes, and one report is evidence the static half has stopped carrying the
  screen.

---

## 21. One codec per `image_asset` row — the debt the `format` seam is carrying

**What.** [ADR-0050](./DECISIONS.md#adr-0050) clause 1, verbatim:

> **The image pipeline's base output format is JPEG, encoded with the standard library's
> `image/jpeg`, and EVERY rendition UsArr stores for an asset — all seven widths, `orig`
> included — is produced by UsArr's own encoder in the codec `image_asset.format` names. There is
> no passthrough width.**

Per-`role` codec variation stays open (PNG for logos), because `role` is a column on the same row.
Per-**width** variation is foreclosed.

**Why it is in this file at all — it is not a feature, it is an unpaid obligation on a seam.**
`image_asset.format` (migration `00008_image_asset_format.sql`) is one column per row, while
`ARCHITECTURE.md` §4.4 stores up to **seven** renditions per asset. The column can only ever be
truthful *because* of the clause above, and **nothing enforces the clause today**: there is no
fetcher, no downscaler and no encoder, so ADR-0050 discharges it **by definition rather than by
code**. The Go guard that did ship — `ValidImageFormat` and
`TestImageWritesValidateTheFormatVocabulary` in `internal/store/images.go` — checks that a writer
uses a *token from the vocabulary*. It cannot check that seven renditions of one asset agree, because
there is nothing in the schema that even names a rendition.

**What breaks if it is violated**, and both failures are silent:

- **`format` describes six of seven renditions.** Store `orig` as the upstream's untouched bytes and
  the row still says `jpeg`; the column is now a claim about a subset, with nothing recording which
  subset.
- **`/img?w=orig` serves a `Content-Type` that lies about the bytes.** ADR-0050's consequence list
  makes the `/img` surface a one-line token→media-type lookup that reads the row for **every**
  allowlisted width. That is sound only under this clause. Break it and the header is wrong for one
  width out of seven — the case least likely to be in anyone's test fixture.

**What the first writer owes.** The first code that stores a rendition is what makes this real or
breaks it, and it has exactly two honest options:

1. **Enforce one codec per asset in the encoder** — every allowlisted width, `orig` included, goes
   through UsArr's own encode in the codec written to `format`; or
2. **Come back to the schema first.** Storing a rendition in a different codec from its siblings is
   an amendment to ADR-0050 plus a **second** column for the passthrough codec — measured in
   `00008`'s header at one more one-line `ALTER TABLE ADD COLUMN`, not a rebuild.

What is not available is writing the mixed-codec row and leaving `format` as it stands.

**The seam.** The column itself. It is nullable, unconstrained `TEXT` holding a lowercase codec
token, so a second codec costs one map entry in Go and no migration; and because it is a *record*
rather than a promise, the day the promise is broken deliberately is the day it can be made to say
so. `reference/schema.md` §12 carries the column's four properties and points here.

**Trigger.** The first commit that writes bytes for an `image_asset` row. Not the pipeline's design,
not the fetcher — the write.
