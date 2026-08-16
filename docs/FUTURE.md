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
   `UNIQUE (user_id, work_id, started_at)` — **not** a counter. The stated rule is that *play counts
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
| **Filtered audit UI** | The audit log ships in v0.1 as a plain paginated list; filters are polish | `audit_log` already carries `action`, `target_type`, `actor_user_id`, `result` — filtering is a query |
| **Security-posture panel** ("you are exposed; 2FA is off") | Serves internet exposure, which is the secondary deployment | It is a read-only view over settings that already exist |
| **`sqlite-vec` semantic search** ("search by plot description") | Needs an embedding model — a runtime dependency or a remote call, both violating the no-sidecar rule. ⚠️ TMDB's ToU also names ML training/validation as commercial use, so embedding TMDB overviews is legally murky; prefer Wikidata or user-local text | Same retriever seam as §2 |
| **Download-client visibility** (SAB, NZBGet, qBittorrent, Deluge, Transmission) | Needs session-establishment and JSON-RPC transports that manifests cannot express (`reference/providers.md` §3.2) | Tier 0 providers via the same registry; Search-and-Grab already routes grabs through Prowlarr's own clients, so nothing is blocked meanwhile |
| **Recyclarr/Configarr automation absorption** | Proven collapsible by arr-dashboard, but it would dilute the speed story | UsArr already reads custom formats and is required never to clobber them |
| **Optimistic write apply with inverse-patch rollback** | Replaced by a durable command queue; it bought ~200 ms on a rare operation at the cost of the project's hardest correctness problem (ADR-0012a) | The queue's state machine already distinguishes `failed_rejected` from `verifying`, which is the hard half |
| **Cross-media "continue" row** spanning video, audiobook and ebook position | Depends on §9's event stream | `playback_state` is keyed `(user_id, work_id)` across all kinds already |
| **Emby / Plex** | Emby is a feature-flagged Jellyfin variant; Plex paywalled remote playback of personal media on 29 April 2025 | Both are Tier 0 providers behind the registry |
