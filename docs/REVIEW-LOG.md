# UsArr — Adversarial review log

**Date:** 2026-08-16. **Inputs:** three independent adversarial reviews — technical correctness
(`C-nn`), security/secrets/operations (`S-nn`), and scope/honesty/buildability (`P-nn`).
**Scope of this log:** every **BLOCKER** and **MAJOR** finding that touches `ARCHITECTURE.md`,
`DECISIONS.md`, `README.md` or the files created in this revision. Findings against
`CONFIGURATION.md`, `DEVELOPMENT.md`, `SETUP-CHECKLIST.md`, `.env.example`, `Makefile` and
`.gitignore` were addressed separately and are marked *delegated* here.

---

## Counts

**83 BLOCKER + MAJOR findings across the three reviews.**

| Disposition | Count |
|---|---|
| **Applied in full** | **73** |
| Applied here, remainder delegated to the configuration/tooling files | 3 |
| Delegated entirely (other files) | 3 |
| **Applied, with part of the finding rebutted** — reasoning in §2 | 3 |
| **Rebutted / overridden** — reasoning in §2 | 1 |

By review: correctness 11 BLOCKER + 27 MAJOR (38); security 6 + 16 (22); scope 6 + 17 (23).

> **A counting note on the correctness review.** Its header states *"11 BLOCKER · 22 MAJOR · 16
> MINOR"*, but the body contains **27** findings between C-12 and C-38 inclusive. The body is
> authoritative; 54 findings total, not 49. All 27 majors are dispositioned below.

MINOR findings (C-39…C-54, S-23…S-28, P-24…P-28) are not individually tabulated. Those touching
these documents were applied — including the broken §6.6 cross-reference (C-39), the per-app
`mediacover` path shapes (C-40), the redundant `ix_extid_lookup` (C-41), the missing
`deleted_at IS NULL` predicates (C-42), the "covered by" misstatement (C-43), the dead leading
column on `ix_relation_review` (C-44, moot once the review inbox was deferred), the §1.3 / ADR-0019
list mismatch (C-45), the §4.8 self-contradiction (C-46), the "four channels" miscount (C-47), the
ISBN "is wrong" correction (C-48 — see §2.5), the `apiKey` wire details (C-50), the AniList rate
limit (C-51), the pragma/RSS marks (C-52), absolute-numbered anime (C-53), the unused
`last_history_id` (C-54), the audit-log mechanism (S-24), the unspecified pepper (S-27), the SSRF
redirect rule and numeric caps (S-28), the Jellyfin-client diagram node (P-24), the flagship framing
(P-25), the offline claim (P-26), the licence (P-27) and Meilisearch (P-28).

---

## 1. Disposition of every BLOCKER and MAJOR

### 1.1 Correctness (C)

| # | Finding, in one line | Disposition |
|---|---|---|
| **C-01** | Typo tolerance does not exist; the flagship search example returns zero rows | **Applied.** Requirement struck; ARCHITECTURE §8.1 and `reference/search.md` §1 state plainly that FTS5 trigram is a substring matcher; README claim removed. `spellfix1`/BK-tree recorded as a deferred candidate with costs and an ⚠️ availability question (`FUTURE.md` §3) |
| **C-02** | Contentless FTS5 makes DELETE/UPDATE impossible; deleted works stay in search forever | **Applied.** `contentless_delete=1` on both tables; SQLite floor raised 3.37 → **3.43** in ARCHITECTURE §6, `schema.md` §7 and ADR-0002; same-transaction delete rule plus a CI count assertion |
| **C-03** | `UNIQUE(media_file.path)` makes two instances of one file impossible | **Applied.** `UNIQUE(service_instance_id, path)`, `service_instance_id NOT NULL` with a sentinel instance 0. `content_key` (size + first-64-KiB hash) is defined and **explicitly deferred** to the first milestone that aggregates a media server, with the NAS read cost stated |
| **C-04** | The northbound ID cannot be resolved with an index | **Applied.** `kind_byte` encoded into the ID; ARCHITECTURE §5.3, `gateway.md` §3, ADR-0021 amended, with the `EXPLAIN QUERY PLAN` evidence retained |
| **C-05** | `/api/v3/episode` requires a parent id, so "never fetch children per-parent" is unimplementable | **Applied.** Rule rewritten to one bounded call per series; the controller-vs-spec discrepancy documented; **~2,000 round trips added to the §13 cold-start budget** |
| **C-06** | `ncruces/go-sqlite3` moved off wazero to `wasm2go`; the "one runtime, two uses" argument is false | **Applied.** ADR-0001 corrected (including dropping "bit-for-bit upstream behaviour"), ADR-0008 re-argued without it, README bullet 5 rewritten |
| **C-07** | The client prefix index is sized against a library 40× too small | **Applied.** Scoped to top-level works (~13k rows), `sort_title` dropped, ThumbHash as raw bytes, size restated as ~1.5–2.1 MB, **hard cap 25,000 items** with a stated fallback |
| **C-08** | `idempotency_key` is globally UNIQUE, so a collision returns another user's intent | **Applied.** `UNIQUE (user_id, idempotency_key)`, `409` on cross-user collision, and one server-derived key rule for northbound surfaces replacing two incompatible schemes |
| **C-09** | Reconciliation reverts user edits: `applied` has no guard and no timeout | **Applied**, adapted to the command queue: the guard covers `pending`/`inflight`/**`verifying`**, `verifying` has a 15-minute TTL and confirms by targeted refetch |
| **C-10** | "audiobook" is both a `work.kind` and an `edition`; the two models are incompatible | **Applied.** Edition-level chosen and propagated through `work.kind`, `edition.format`, `request`, the `type:`/`format:` tag vocabulary and `Caps.MediaKinds` |
| **C-11** | \*Arr id reuse plus the tombstone resurrects the wrong work | **Applied.** `remote_identity_hash` guard in ARCHITECTURE §7.4 and `sync.md` §4 |
| **C-12** | The normalisation table's axis is wrong; `Radarr.ReleaseResource.imdbId` is an int | **Applied.** Re-axed on `(app, resource)` in `reference/arr-apis.md` §2, verified against the shipped specs |
| **C-13** | "Lidarr's `Quality.source` are audio formats" is false — there is no `source` field | **Applied.** `arr-apis.md` §3 gives the verified per-app shapes and enums |
| **C-14** | Prowlarr's `/history/since` is indexer telemetry, not entity change | **Applied.** Prowlarr removed as a delta source; channel 3 scoped to library-bearing acquisition apps; the "all five apps" undercount corrected to six |
| **C-15** | `Accept: application/json` will 406 because Radarr declares `/movie` as `text/plain` | **Applied in part; premise rebutted** — see §2.2 |
| **C-16** | "`SQLITE_BUSY` structurally impossible" is overstated | **Applied.** Narrowed in ARCHITECTURE §7.7 and ADR-0002, with the residual sources and mitigations listed |
| **C-17** | The single writer is shared, so the write budget cannot hold during an import | **Applied.** Priority scheduler, `min(2000 rows, 100 ms)` batch commits, and §13's write budget restated as measured *during* an import |
| **C-18** | The tag "index-only scan" claim fails once user scoping is applied | **Applied.** `(tag_id, user_id, work_id)` with `user_id NOT NULL DEFAULT 0` and the `IN (0, :uid)` predicate; measurement kept in `tags.md` §6 |
| **C-19** | The ID scheme violates its own length requirement | **Applied.** `enc_byte` hex/UUID compaction, separator byte removed, per-shape lengths tabulated, the bound restated as a target with the misses named |
| **C-20** | "Nothing UsArr computes is in it" is false; priority changes move IDs | **Applied.** `is_northbound_canonical` pin plus `service_item_alias` rows; claim deleted from ARCHITECTURE §5.3 and ADR-0021 |
| **C-21** | The 5-minute overlap is unjustified and `date` semantics are unspecified | **Applied.** Overlap derived from measured `clock_skew_secs`; `date` format/inclusivity probed; unbounded-response risk stated |
| **C-22** | A restored-from-backup \*Arr rewrites the library to match wrong items | **Applied.** `identity_fingerprint` + `max(remote_id)` guard that refuses to sweep |
| **C-23** | Rollback has no compensating action when the write partially applied | **Applied**, adapted: `failed_rejected` vs `verifying` with a targeted verification refetch (ADR-0012a) |
| **C-24** | The preferred credential mitigation contradicts the absolute credential rule | **Applied by supersession** — the redirect is gone (ADR-0017 reversed) |
| **C-25** | Range/seek/resume across a minutes-TTL redirect is asserted and never specified | **Applied by supersession**, and the seek problem is now solved rather than mitigated: bytes come from UsArr, so a short TTL cannot break seek |
| **C-26** | Playlists reference `work_id`, reintroducing ID instability; PK breaks reorder | **Applied.** `schema.md` appendix: `link_id` reference, sparse `REAL` position, `(playlist_id, item_id)` PK, and a stated `home_instance_id` recompute rule |
| **C-27** | `work_relation` holds one global verdict but verdicts are declared per-user | **Applied by removal.** No review inbox, so no verdicts; the multi-user obligation for manual links is recorded explicitly rather than inherited |
| **C-28** | RRF fuses two FTS tables on rowid with nothing enforcing a shared rowid space | **Applied.** Stated as an invariant with an allocator and CI assertions |
| **C-29** | The FTS query string is unspecified: AND/OR, prefixing, escaping | **Applied.** `search.md` §3 gives the transformation, the OR choice with its reason, the escaping function and seven worked examples |
| **C-30** | The OpenSubsonic surface is a milestone described in one paragraph | **Applied.** Narrowed to 13 endpoints (P-13) **and** given a method → tables → degradation map in `gateway.md` §1 |
| **C-31** | No track/disc numbers and no comic subtype; music is unrepresentable | **Applied.** `work_track`, `work_comic`, and the rule that every kind has a subtype or a stated reason |
| **C-32** | The `external_id` uniqueness violation *is* the merge signal and nothing handles it | **Applied.** Merge-on-violation rule, per-row `SAVEPOINT` isolation, and the literal `ON CONFLICT` expression list |
| **C-33** | `tag_assignment` has uniqueness for one of its four targets | **Applied.** Four partial unique indexes plus three lookup indexes |
| **C-34** | Two incompatible directory layouts; the compose file loses user data | **Applied.** ARCHITECTURE §15 now states requirements and points at `CONFIGURATION.md` §5 as authoritative; README compose updated |
| **C-35** | Env precedence specified two contradictory ways; the column cannot express it | **Applied.** `managed_by TEXT CHECK (ui|env|file)`; `CONFIGURATION.md` is authoritative for precedence |
| **C-36** | `USARR_SECRET_KEY` is both mandatory and auto-generated | **Applied.** One behaviour, stated once (`security.md` §1.4): generate if absent, validate if present, empty = absent, placeholder = fail closed, no shipped default |
| **C-37** | TLS verification specified as both a per-row column and a global env list | **Applied.** Per-row is the mechanism; TOFU SPKI pinning replaces `verify_tls=0`; any env list is a bootstrap seed that warns on use |
| **C-38** | The series availability badge has no defined semantics for partial coverage | **Applied.** `{tier: {have, total}}` with rendering thresholds and dirty-mark recomputation on the 250 ms flush |

### 1.2 Security (S)

| # | Finding | Disposition |
|---|---|---|
| **S-01** | `.env.example` ships an active placeholder master key; startup accepts any string | **Applied here + delegated.** The startup ladder, validation, placeholder reject-list and no-shipped-default rule are in `security.md` §1.4 and ARCHITECTURE §14; the `.env.example` and `Makefile` halves belong to the configuration/tooling files |
| **S-02** | YAML manifests labelled "fully sandboxed"; they are a credential-exfiltration primitive | **Applied.** Relabelled as a server-side request generator; `ResolveReference` forbidden with the concrete escape shown; escaping mandatory at load; `externalIds` capped below 1.0; gist distribution removed |
| **S-03** | SSRF: URLs from upstream response bodies are an unclassified third class | **Applied.** Three classes with per-row `origin_class` / `origin_service_instance_id` and policy selected from the row |
| **S-04** | Argon2id on every northbound request is a remote exhaustion primitive | **Applied.** HMAC-SHA256 + constant-time compare, `key_prefix` lookup, `/rest/*` and `/opds/*` in the tighter bucket, **and the reasoning recorded so it is not "fixed" back** |
| **S-05** | The preferred stream mitigation does not exist for Jellyfin | **Applied.** Verified and cited; ADR-0017 reversed. *One sub-point rebutted* — see §2.4 |
| **S-06** | Ciphertext has no key version and no AAD; rotation is non-atomic and unscheduled | **Applied.** Versioned envelope with a plain `kek_id` column, AAD binding row + host:port, two-phase resumable rotation, and `usarr key rotate` on the **v0.1** milestone |
| **S-07** | Editing `base_url` and pressing Test exfiltrates every stored credential | **Applied.** Host change invalidates the credential; a test against a modified URL uses only the typed key; AAD makes it fail closed; sudo mode and an audit event |
| **S-08** | The `100.64.0.0/10` denylist contradicts tailnet deployment | **Applied.** Denylist entry removed for instance-originated artwork; provenance-based allowlist replaces it |
| **S-09** | IDs are enumerable and no authorization is stated at resolution | **Applied.** "Opaque" struck; per-request authorization before any backend call; Subsonic 70 not 403; rate-limited and audit-logged |
| **S-10** | Client index, global FTS and rollups make the authorization rule unenforceable | **Applied.** Access-scope parameter as a §1.3 hard rule; index built per scope; `search_doc.instance_scope` filtered in the join; rollups scoped |
| **S-11** | Northbound keys travel in query strings with no redaction rule | **Applied.** Redaction as middleware with a fixed parameter deny-list at every log level; `key_prefix` only; TLS recommendation; hard rejection of salt/token parameters |
| **S-12** | The signed stream URL has no TTL, nonce, revocation binding or signing key | **Applied.** Numeric TTL (120 s / 600 s max), nonce + replay cache, revocation checked per redemption, distinct HKDF label, skew tolerance, `no-store` + `no-referrer` |
| **S-13** | An empty tailnet allowlist is fail-open and the resulting role is undefined | **Applied here + delegated.** ARCHITECTURE §12.3 states fail-closed, the tagged-device rule, and the v0.1 owner-only mapping; the env var default belongs to the configuration file |
| **S-14** | Backup and migration put the key and the ciphertext in one archive | **Applied.** Key outside the backed-up path, backup jobs exclude it, 0600/0700 modes independent of `UMASK`, backup endpoint gated/audited/session-only, key named as a separate restore step |
| **S-15** | `verify_tls=0` sends a full-admin key to whatever answers; IP pinning worsens it | **Applied.** TOFU SPKI pinning, plaintext warning for non-tailnet hosts, and the SNI/hostname rule that prevents an `InsecureSkipVerify` "fix" |
| **S-16** | Supply chain: non-gating vuln check, `@latest` tools, unsigned images | **Delegated** to the `Makefile`/CI. The requirements it must satisfy (digest-pinned base, provenance, SBOM, signing) are recorded in `security.md` §7 |
| **S-17** | Distroless vs a root PUID entrypoint; tsnet vs the healthcheck; an unauthenticated admin listener | **Applied.** Distroless non-root decided explicitly, `chown` documented instead, health-only loopback listener, `--start-period=60s` |
| **S-18** | The WASM sandbox is asserted, not specified | **Applied** by deferring the tier, with the full specification it *would* need written out in `FUTURE.md` §1 so it is not rediscovered |
| **S-19** | OPDS authentication is never specified | **Applied.** HTTP Basic named, TLS required or warned, `Authorization` stripped before any redirect, client-forwarding behaviour listed as an ⚠️ test case |
| **S-20** | Provider keys persist in `image_asset.source_url` and the HTTP cache | **Applied.** Credential-stripped URLs, `cache_key` derived from the stripped form, an ingest assertion, and stored URLs added to the redaction scope |
| **S-21** | No secret-scanning gate; `services.yaml` / `config.yaml` not gitignored | **Delegated** to `.gitignore` and the `Makefile` |
| **S-22** | `/img/{cache_key}` has no auth and is served `public` | **Applied.** Authenticated and authorized, `Cache-Control: private`, public provider artwork moved to a structurally distinct path |

### 1.3 Scope (P)

| # | Finding | Disposition |
|---|---|---|
| **P-01** | "Runs over just Prowlarr" is asserted, never designed; the product is empty | **Applied** as option (b): **Search-and-Grab mode** (ARCHITECTURE §8.5) with a verified Prowlarr search/grab path, derived tags, and the honest floor stated in §1.1 |
| **P-02** | Owned + unowned search has no design and the replica rule forbids the obvious one | **Applied** as option (a): out-of-band provider search, SSE-streamed, merged client-side, **not persisted until requested**; §2.2 amended so it is a stated exception, not a contradiction |
| **P-03** | README and §16 state different v0.1 scopes | **Applied.** README tables regenerated from §16 with an explicit "§16 wins" rule |
| **P-04** | Cold start is not designed and ThumbHash is unavailable exactly when needed | **Applied.** §4.4.1 (viewport-prioritised queue, 92px-first, `dominant_color` before ThumbHash, progressive render) plus six cold-start budget rows |
| **P-05** | v0.1 has no write path and no reconciliation; it drifts and cannot be corrected | **Applied.** Reconciliation and a minimal write path move into v0.1; SignalR and webhooks move out |
| **P-06** | The roadmap sequences both differentiators last | **Applied.** Reordered to library → requests → cross-media → narrowed gateway → breadth, with the rationale stated in §16 |
| **P-07** | Cut the byte proxy entirely; link out instead | **Rebutted / overridden** — see §2.1 |
| **P-08** | The Wikidata dump pipeline is unstaffed and the size is stated three ways | **Applied.** Edges-only artifact from a committed SPARQL script, per release, one size (single-digit MB) |
| **P-09** | The Tier-3 fuzzy ladder and review inbox are a subsystem nobody will staff | **Applied as a deferral**, per the owner's amendment: cut from v1 scope, kept in `FUTURE.md` §5 with the `confidence`/`evidence` seam preserved |
| **P-10** | The intent log's optimism buys 200 ms at the cost of the hardest correctness problem | **Applied.** Durable command queue (ADR-0012a); optimistic apply deferred with its seam |
| **P-11** | Cut the WASM tier from the documents, not just the roadmap | **Applied as a deferral, not a deletion** — amended by the owner mid-revision. ADR-0008 rewritten as two tiers plus a deferral with a revisit trigger; the registry seam is explicit in `providers.md` §1 and `FUTURE.md` §1 |
| **P-12** | The manifest cannot express several services its own table claims | **Applied.** Stated scope line plus a covers/does-not-cover list; the misleading diversity table removed |
| **P-13** | v0.2 "It is a gateway" is a year of work in six bullets | **Applied.** Narrowed to one Navidrome, ~13 endpoints, read-only, with a one-sentence success criterion |
| **P-14** | The checklist's BLOCKING markers contradict the roadmap | **Applied here + delegated.** ARCHITECTURE and README now state that v0.1 needs **no external metadata provider**; the checklist itself is another agent's file |
| **P-15** | The configuration surface is larger than most shipped applications | **Delegated** to `CONFIGURATION.md` / `.env.example` |
| **P-16** | The search corpus is undefined and would be swamped by episode titles | **Applied.** Corpus defined as top-level kinds, with a CI assertion and a scoped-search path for children |
| **P-17** | The documents never say what the user looks at | **Applied.** ARCHITECTURE §17 "Screens", written to the owner's own UI direction: a stated utilitarian philosophy with Navidrome as the reference, a sectioned home, a first-class **Services health screen**, search, requests covering both paths, and first-run/empty/offline/error states |
| **P-18** | The fuzzy identity tiers and merge machinery are unnecessary and risky for v0.1 | **Applied.** Tier 1 only in v0.1; simple v1 normaliser; `work_merge` out of migration 0001 |
| **P-19** | 3,070 lines cannot function as a working spec | **Applied, with the numeric target missed and the deviation recorded** — see §2.3 |
| **P-20** | CI-enforced millisecond gates will be abandoned or will block every PR | **Applied.** Wall-clock moves to `make bench` as a release gate; `EXPLAIN QUERY PLAN` and row-count assertions stay in CI |
| **P-21** | ADR-0001's memory evidence does not apply to the chosen driver | **Applied, with the finding's own mechanism rebutted** — see §2.2 |
| **P-22** | `/ready` gated on initial sync will restart-loop containers | **Applied.** Readiness redefined; sync state reported separately; `--start-period=60s` documented |
| **P-23** | The internet-exposure apparatus is a second product | **Applied as a deferral.** ADR-0022 states the v1 auth scope; the deferred items keep their seam in `FUTURE.md` §4 |

---

## 2. Rebuttals and partial disagreements

Recorded because a finding that is quietly ignored comes back.

### 2.1 P-07 — "Cut proxy mode entirely; backends that cannot mint a scoped token are catalogued and linked, not streamed." **Overridden.**

**What the finding gets right, and it is decisive:** the redirect design was unsafe. Its evidence
is the same evidence that produced the ADR-0017 reversal, and it is correct that a byte path is a
real cost the owner explicitly declined for video.

**Why the conclusion does not follow for audio and ebooks.** The finding treats "link out" as a
universally available fallback. It is not, for the surfaces UsArr actually ships:

1. **A Subsonic client has exactly one acquisition verb — `stream` — and no affordance for being
   sent somewhere else.** "The catalogue entry links out to the backend's own web UI" is not
   expressible in the OpenSubsonic protocol. Applying it to audio means the OpenSubsonic surface
   does not play music, which is the entire success criterion of the milestone that ships it
   (*"Symfonium connects with one API key, browses, searches and plays"*).
2. The finding's own preferred alternative — advertising a reduced capability — degrades to the same
   thing: a music server that advertises no `stream` is not a music server.
3. OPDS has the same shape: an acquisition link must resolve to bytes.

**What was adopted instead**, which honours the finding's underlying concern (do not become a
byte-delivery product) while keeping the surfaces functional:

- **Video links out** — exactly as the finding proposes — because video *does* have a first-class
  alternative (the backend's own client) and is where the byte cost is ruinous. The northbound
  surfaces advertise no video stream endpoint at all.
- **Audio, ebooks and comics are proxied** with a plain `io.Copy` and correct `Range` handling, and
  **never transcoded**. `USARR_STREAM_MODE` is gone; there is no mode switch.
- **The cost is stated rather than minimised** (ARCHITECTURE §5.4): `Range` handling is bug-prone,
  audio is ~1–5 Mb/s rather than a 60 Mb/s remux, and the failure mode of a bug is a client that
  cannot seek — not a leaked credential.

This is also the rule ADR-0023 generalises: link out where a better neighbour exists, carry bytes
only where the protocol leaves no alternative.

### 2.2 Two findings whose *conclusions* were adopted and whose *premises* were wrong

**C-15 — "Radarr's spec declares `/api/v3/movie` as `text/plain`, so `Accept: application/json`
gets a 406."** The premise is false. Verified against the shipped specs, the declared 200 content
types are:

```
radarr  /api/v3/movie              → text/plain, application/json, text/json
lidarr  /api/v1/album, /track      → text/plain, application/json, text/json
readarr /api/v1/author, /book,
        /history/since             → text/plain, application/json, text/json
sonarr  /api/v3/series, /episode   → application/json
radarr  /api/v3/history/since      → application/json
```

`application/json` is declared on **every** endpoint the finding names, so a plain
`Accept: application/json` does not 406. The review appears to have read the first entry only.
`ReturnHttpNotAcceptable = true` is separately confirmed and does make an unsatisfiable `Accept` a
406, so **the proposed fix was adopted anyway** — the defensive `Accept` header and
"parse as JSON regardless of `Content-Type`" cost nothing and remove the question — but the claim
that "the single most important endpoint in the whole import path 406s" is not supported and is not
repeated in the documents.

**P-21 and C-52 — the idle-RSS findings.** Their *conclusion* is right and is applied: Navidrome
uses a **cgo** SQLite driver, so citing it as the existence proof for UsArr's `< 80 MB` budget does
not transfer, and an arm64 spike must set the number before the schema is written. Their *mechanism*
is now wrong: both describe UsArr's profile as "SQLite compiled to WASM inside wazero, with its own
linear-memory arena and a JIT" (P-21) and quote the driver README's "each database connection
executes within a Wasm sandboxed environment" (C-52). Per C-06 — raised by the same review —
`ncruces/go-sqlite3` **moved off wazero to `wasm2go`** and now lists Go and `x/sys` as its only
direct dependencies. So neither the cgo citation nor the WASM-runtime reasoning describes what UsArr
will actually run, and the documents say exactly that rather than substituting one unverified
mechanism for another. The pragmas are marked ⚠️ pending measurement for the same reason.

### 2.3 P-19 — the restructure was applied; the line target was not met

**Applied:** `ARCHITECTURE.md` went from **3,070 lines to 1,471**, with all reference detail moved
into **nine** new `docs/reference/*.md` files, each carrying a status and milestone header. The main
document is now readable start to finish: principles, components, the gateway, the data model, sync,
search and requests, cross-media, tags, providers, identity, performance, security, deployment, the
roadmap and the screens — with the DDL, endpoint tables, normalisation matrices and provider
inventories linked rather than inlined. The process rule (*no new design document until there is a
running binary*) is adopted in the README.

**Not met:** the finding's numeric target of ~600–900 lines. Stating the reason rather than quietly
missing it: the main document must carry, in the main document, every binding decision a contributor
needs before opening the reference files — and this revision *added* required content (a Screens
section that did not exist, Search-and-Grab mode, the cold-start design, the stream-path reversal
with its evidence, and the extension seams). Cutting a further ~550 lines would mean either deleting
one of those or moving a decision into a reference file where a contributor would not find it, which
defeats the purpose of the restructure. The measure that matters — *is the reference detail out of
the main document* — is met; the line count is 60% above the target and is recorded here rather than
presented as a success.

**Not done:** the finding also proposed splitting `RESEARCH.md` into `docs/research/` by track. That
file was outside this revision's ownership and is untouched; it remains a reasonable follow-up.

### 2.4 S-05, sub-point 3 — a capability bit that is no longer needed

S-05 proposes adding `ScopedStreamToken bool` to `Caps`, probed per instance, "and make the redirect
path structurally unreachable when it is false". With the redirect removed entirely (§2.1), there is
no redirect path to make unreachable and no per-backend token to probe for. The stronger form of the
same protection was adopted instead: **`Caps` has no `Stream` capability at all**, because the
stream path no longer depends on a backend minting anything. Adding a probe for a mechanism that
does not exist in either named backend would be an unverifiable capability check.

### 2.5 Two smaller corrections *to* the reviews, recorded for the record

**C-48 (MINOR) is right and its correction is adopted.** The architecture said an alternative
research track's identifiers "are wrong", listing ISBN `9780374281144` among them. Verified live
against `openlibrary.org/works/OL15916948W/editions.json`, that ISBN **is** edition `OL24823347M` of
the correct work — an *edition-level* identifier, not an error. The text now says "edition-level,
not work-level", which is exactly the distinction the `edition` layer exists to make.

**The "Minimum credible v0.1" proposal to cut the `edition` table was not adopted.** It is not a
numbered finding, but it is a direct recommendation and deserves an answer: books, audiobooks and
ebooks genuinely need the layer, it is what makes ebook-vs-audiobook routing a schema property
rather than adapter special-casing (the exact friction point SeerrNG named), and it is one narrow
table plus a foreign key. Cutting it would also have made C-10 unresolvable, since the chosen
resolution *is* the edition layer.

---

## 3. What changed, in one list

- **New files:** `docs/FUTURE.md`, `docs/REVIEW-LOG.md`, and `docs/reference/{schema, sync, search,
  gateway, crossmedia, tags, providers, arr-apis, security}.md`.
- **Reversed:** ADR-0017 (stream path).
- **Superseded in part:** ADR-0012 (write path) by the new ADR-0012a.
- **Amended:** ADR-0001 (evidence corrected), ADR-0002 (`SQLITE_BUSY` claim narrowed), ADR-0007
  (edges-only artifact; fuzzy tier deferred), ADR-0008 (two tiers; WASM deferred), ADR-0013 (two
  tiers; typo tolerance withdrawn), ADR-0014 (framing demoted), ADR-0019 (scoped list reconciled;
  access-scope rule added), ADR-0020 (audiobook propagation), ADR-0021 (kind byte, pin, opacity
  claim struck).
- **New:** ADR-0022 (v1 authentication scope), ADR-0023 (coexistence framing).
- **Roadmap:** reordered, and every deferred idea moved out of the milestones into `FUTURE.md` with
  a seam and a revisit trigger.

---

## Final consistency pass

A last read of every document against `ARCHITECTURE.md` §16 (roadmap), §1.4 and `FUTURE.md`
(deferred vs permanent), §5.4 (the stream path) and `CONFIGURATION.md` §5 (the on-disk layout).
Findings, all applied:

**Roadmap numbering.** `CLAUDE.md`'s one-line summary put OPDS in v0.4; §16 ships the narrowed
OpenSubsonic subset in v0.4 and the OPDS surface in v1.0. Corrected in `CLAUDE.md`. Its UI section
also listed a Requests screen as v0.1 without qualification; in v0.1 that screen is the Prowlarr
search-and-grab path (§8.5), and the \*Arr-backed request flow is v0.2. `SETUP-CHECKLIST.md` listed
a tailnet as "v0.4 at the earliest" against §16's v1.0 for `tsnet`, and `DEVELOPMENT.md`'s tree
marked LazyLibrarian as a v1.0 Go package against §16's v0.3 Tier 1 manifest. Both corrected to §16.

**Deferred stated as cut.** Five documents described deferred work as cut or rejected, which
`DECISIONS.md`'s own revision-2 preamble forbids: `CONFIGURATION.md` §0 (OIDC/passkeys/TOTP/
forward-auth and multi-user), §2.6 (the same, plus Meilisearch) and §5 (WASM plugins);
`DEVELOPMENT.md` §1 and §2 (WASM plugins); `SETUP-CHECKLIST.md` §5 (Meilisearch, WASM plugins,
external identity); `reference/crossmedia.md` (the Tier 3 ladder and review inbox);
`DECISIONS.md` ADR-0010 ("deferred to v2", a milestone that does not exist). Each now says
deferred, names its `FUTURE.md` section, and says it is on no milestone. The permanent list —
video transcoding, an in-app player, any FFmpeg dependency — is unchanged everywhere.
`CLAUDE.md`'s deferred list was missing video byte-proxying (`FUTURE.md` §7), which the other three
documents carry; added.

**Survivors of the stream-path reversal.** `DEVELOPMENT.md` still asserted in three places that
UsArr never carries media bytes, has no `Range` handling, and catalogues anything whose backend
"cannot issue a scoped credential" — the falsified premise behind the reversed ADR-0017.
`CONFIGURATION.md` §0 and §2.6 and `SETUP-CHECKLIST.md` §5 carried the same claim. All now state
the §5.4 rule: video links out, audio/ebooks/comics are proxied with `io.Copy`, nothing is ever
transcoded. `README.md`'s v0.4 row still called the northbound IDs "opaque", a requirement struck
by S-09; corrected.

**`make check`.** `CLAUDE.md` described the gate as "fmt + lint + vuln + test + secret scan" running
offline. The `Makefile` target is `check-offline` (`fmt-check` `lint` `modverify` `secrets` `test`)
plus `vuln`, and it makes exactly one network call. Prose corrected to the target.

**Stale evidence.** `RESEARCH.md`'s track-03 entry still described `ncruces/go-sqlite3` as running
under wazero. The research record is preserved and a dated ⚠️ supersession note added, rather than
rewriting what the researcher found. `ADR-0001`'s decision line still named a wazero WASM plugin
host; struck, with the pointer to its own correction and ADR-0008.

**One contradiction resolved toward `ARCHITECTURE.md`.** `CONFIGURATION.md` §1 listed
`managed_by` among the things that do not exist, while `ARCHITECTURE.md` §6.3 and §15,
`reference/schema.md` §5 and finding C-35 all require `service_instance.managed_by TEXT CHECK (ui|
env|file)`. Both readings are defensible — there is genuinely no environment channel for service
rows today, which is `CONFIGURATION.md`'s point — so the architecture wins on the narrow question:
the column exists and can express three values; with one writer every row is `ui`.
`CONFIGURATION.md` remains authoritative for precedence.

**Checked and already correct:** every relative link in the 19 markdown files resolves, including
section anchors and all nine `reference/*.md` names; `CONFIGURATION.md` §5 is the only directory
tree in the repository and `ARCHITECTURE.md` §15 and `DEVELOPMENT.md` conform to it; the
fourteen-variable configuration surface agrees across `CONFIGURATION.md` §2, `.env.example` and
`DEVELOPMENT.md` §11; no document claims typo tolerance, Navidrome `apiKeyAuthentication`,
Jellyfin scoped tokens, a 302 default, `services.yaml`/`config.yaml`, `PUID`/`PGID` or the old
~60-variable surface as anything other than a corrected error; and every `README.md` status marker
reads 📋 Planned, with no feature marked as existing.

---

# UsArr — Adversarial review, round 2: the UI design work

**Date:** 2026-08-16. **Input:** one adversarial review (`D-nn`) of `docs/design/DESIGN-DIRECTION.md`,
`docs/design/tokens.css`, ADR-0025, and `docs/design/mockups/` (`usarr.css`, `usarr.js`,
`index.html`, `services.html`, `search.html`, `requests.html`, `prototype.html`, `README.md`).
**Method:** read against `CLAUDE.md`, then `ARCHITECTURE.md` §16/§17, `DECISIONS.md` and
`FUTURE.md`, in that order of authority; every contrast ratio recomputed from the hex values with
the WCAG 2.x relative-luminance formula rather than read from the tables; ten primary sources
re-fetched live; the prototype opened in Chromium at 1440×900 and 390×844 in both themes, with the
rendered result inspected rather than inferred from markup.

## Counts

**48 findings: 4 BLOCKER, 29 MAJOR, 15 MINOR.**

| Disposition | Count |
|---|---|
| **Applied in full** | **30** |
| Applied in part; the remainder is a decision for Joe (D-05, D-15, D-35) | 3 |
| **Recorded for Joe** — a documented decision this review may not reverse (D-17, D-20, D-24, D-25, D-27, D-30) | 6 |
| Recorded; no change a mockup can carry (D-36, D-41 … D-47) | 8 |
| Citation audit (D-48): nine sources verified verbatim, one miscitation found and fixed, one unverifiable | 1 |

Partial rebuttals are argued in §D2. Nothing is dropped.

**The two named fears, answered up front.** *Does it look AI-generated?* Mostly no, and the reasons
are structural rather than stylistic — but three things in the rendered result read as generated,
and none of them is on the grep ban list (D-05, D-19, D-24). *Will it be slow?* The architecture
argument is sound and the mockup is genuinely cheap, but two concrete mechanisms would have made it
slow (D-12, D-21) and one claimed loading tier is not achievable under ADR-0003 as written (D-20).

---

## D1. Disposition of every finding

### D1.1 Accessibility, measured

| # | Sev | Finding, and the evidence | Disposition |
|---|---|---|---|
| **D-01** | **BLOCKER** | **`usarr.css` fails WCAG on the selected-row ground, in both themes, and fails its own stated floors on three foregrounds.** Recomputed from the hex values against all five grounds a foreground can land on (`--bg`, `--surface`, `--hover`, `--selected`, `--inset`): light `--fg-faint #736d63` on `--selected #e0ddd6` = **3.78:1**, below SC 1.4.3's 4.5:1 **for text**; dark = 4.41:1, also below. Light `--border-hi #857f75` on `--selected` = **2.93:1**, below SC 1.4.11's 3:1 — and §11 of the design document quotes the Understanding page's own words, *"2.999:1 would not meet the 3:1 threshold"*. Light `--fg-muted` on `--selected` = 5.12:1 against a stated 5.5:1 target. These are real grounds: `requests.html` puts checkboxes and two buttons inside `tr[aria-selected="true"]`, which is painted `--selected` | **Applied.** The mockup ramp is now the canonical `tokens.css` ramp verbatim. Every floor holds on every one of the five grounds in both themes; worst cases are now 13.58 / 12.47 (primary), 5.93 / 6.25 (muted), 4.64 / 4.78 (placeholder), 3.42 / 3.23 (control border) |
| **D-02** | **BLOCKER** | **The mockup README's contrast table is selectively grounded and its headline claim is false.** It reported "Muted metadata on a hovered row 5.78:1", "Control border on a surface 3.49:1", "Placeholder 4.87:1" — each measured against a ground that passes, never the selected row that fails — under the sentence *"Floors held: … muted metadata ≥ 5.5:1, borders and focus ring ≥ 3.2:1"*. Only the two protocol rows were honestly labelled "worst ground". A table that picks its ground per row is not a measurement | **Applied.** Every row is now the worst of five grounds, the tokens were corrected rather than the wording, and the paragraph says plainly that three foregrounds had been passing on the ground the table happened to quote |
| **D-10** | MAJOR | **The GOV.UK error pattern is specified in §9.3 and was implemented without the part that makes it accessible.** §9.3 requires "a **visually hidden `Error:` prefix**" on the inline message; `services.html` had the icon and the text but no prefix, so a screen reader hears the inline error as ordinary help text | **Applied.** `<span class="sr">Error:</span>` added to both inline messages. Verified in Chromium: on submit the summary unhides, takes focus, both errors appear inline as well as in the summary, and `aria-describedby` resolves to the error then the help text |
| **D-11** | MAJOR | **The Add-service dialog had no accessible name and no heading.** `<dialog id="add-service">` with a `<div class="dialog__head">`: announced as an unnamed "dialog", and the document outline is a run of divs | **Applied.** `aria-labelledby` plus a real `<h2>`. Focus behaviour was then verified rather than assumed: focus enters the dialog, is still inside after 12 Tab presses, Escape closes it, and focus returns to the invoking button on both Escape and Cancel — all of it native `<dialog>` behaviour, which is the right reason it works |
| **D-25** | MAJOR | **The `permission-denied` state §10 requires "from day one" is demonstrated on no screen**, and `filtered-empty` is missing from Requests although that screen has a Filter control. Services' `denied` state is a *sudo-required re-authentication*, which is a different thing from "denied without leaking existence". Home has no `error` state either. Enumerated from the four states switchers: home {live, stale, partial, empty, unconfigured}, services {live, reident, denied, unconfigured}, search {live, filtered, empty, stale, unconfigured}, requests {live, partial, empty, denied, error, unconfigured} | **Recorded for Joe.** Search is the only screen that carries both `empty` and `filtered-empty` with genuinely different copy, and it does that well. The gaps are real but adding a permission-denied rendering to a single-user product is a scope call, not a correction |
| **D-17** | MAJOR | **§5.3's rule "a 28 px compact row therefore carries no inline clickable icons — compact rows are for reading" is violated by every row on Requests and Services, in the default density.** Both ship a checkbox, a grab button and an external-link anchor inside compact rows | **Recorded for Joe, with the measurement that decides it.** Instrumented every visible target at 1440×900 and 390×844: 21 targets are under 24×24 (13×13 checkboxes, 14×14 link icons), and **zero of them violate SC 2.5.8's spacing exception** — the nearest centre-to-centre distance to any other target is ≥ 24 px in every case, at both viewports. So the mockup is *legal* and §5.3's rule is stricter than the criterion. But nothing enforces the clearance: it is a property of the current column widths, and the first added action breaks it silently. **Either relax §5.3 to "inline targets in a compact row must satisfy the SC 2.5.8 spacing exception, checked in CI" — which is what the mockup actually does — or hold the rule and move both screens to relaxed. Do not leave the document saying one thing and the reference implementation doing the other** |

### D1.2 "Does this look AI-generated?"

| # | Sev | Finding, and the evidence | Disposition |
|---|---|---|---|
| **D-05** | MAJOR | **The 2160p column is the single strongest generated-UI tell in the whole prototype, and no grep can catch it.** On `search.html` it is **23 consecutive rows of the identical string "not configured"**, occupying ~190 px of a ~1,230 px content width; on `index.html` it is 15 more. Apply the design document's own §1.6 test — *delete this element; is information lost?* — and the answer is no: the fact is stated once, in a banner directly above the table, and it is the same fact for every row in the library. A column that cannot vary is a column that is not data. It is also §16's own warning made visible: the "1080p ✓ / 4K ✗" badge *"needs two Radarr instances"*, and this install has one | **Rebutted in part, and recorded for Joe.** The reviewer will not delete a column that demonstrates a documented §6.3 rendering rule; that is a design decision. But the current form fails the test the document sets itself. **Recommendation: render the tier column only when at least one instance of that tier is connected, and state the absent tier once — in the section header, next to the count — rather than once per row.** The banner already carries the sentence; the column carries nothing |
| **D-19** | MAJOR | **Design commentary was rendered as product chrome.** Search and Requests each ended with grey paragraphs explaining the design to a reviewer — *"A grab is acknowledged, not applied optimistically. The button writes a queued command…"*, *"Grabs are recorded in `provenance`, so if a Sonarr or Radarr is added later…"* — set in the same muted body style as real metadata, with nothing marking them as annotation. This is §1.6's second catch-all failing: those sentences exist to persuade a reader that the design is right, not to inform a user about system state | **Applied.** All five such paragraphs now carry the same `mockup` tag the states switcher uses, in a `.note` component, so annotation is visibly not product |
| **D-18** | MAJOR | **Product chrome carried architecture claims.** Home's page header read *"Rendered from the local replica. Last delta sync 14:02"*; Search's read *"Results render from the local index as you type. There is no loading state on this screen because nothing here waits on a network call."* Neither tells a user anything about their library. Both are speed claims — in a prototype whose own README says, correctly, *"There are no benchmark numbers anywhere, deliberately: this prototype cannot make a claim about speed"* | **Applied.** Home: "Last delta sync 14:02, 6 minutes ago." Search: "29 results: 18 movies, 11 series." |
| **D-24** | MAJOR | **The blur test fails for two of four screens.** Screens were rendered at 1440×900, downscaled to 180×112 and Gaussian-blurred. Home is instantly identifiable (the dominant-colour poster strip is the only chroma in the system, and it works). Services is identifiable (chunky rows, pale verbatim panels, red-bordered blocks below). **Search and Requests are not distinguishable from each other**: both reduce to "toolbar band, then one wide monochrome dense table". The differentiators that exist — Search's two section headings, Requests' 8 px protocol dots and fan-out bar — all vanish below ~30% scale | **Recorded for Joe.** The honest reading is that this is the *cost* of the achromatic-chrome decision, not a bug in it: with no accent hue, two table screens have nothing left to differ by except structure. The cheapest structural fix inside the existing rules is to give Requests a persistent left rail of protocol colour at the row level (it is a status, so it is within §3) or to keep Search's per-type sections visually heavier. **Not a reason to reopen the achromatic decision** — three of four screens pass, and the alternative reintroduces exactly the tell §3 exists to prevent |
| **D-07** | MAJOR | **Fabricated release data a self-hoster would catch in five places.** (1) `Dune.Part.Two.2024.PROPER.1080p.BluRay.x265-RARBG` — **RARBG shut down in May 2023** and cannot have released a 2024 film. (2) `Dune.Part.Two.2024.720p.WEBRip.x264-GalaxyRG` filed under **Movies/SD**; 720p is HD in every Newznab category map. (3) that same release at **3.4 GB**; GalaxyRG 720p WEBRips run ~1.4 GB. (4) `…Hybrid.2160p.BluRay.REMUX.HDR.DV.HEVC.**DTS-HD.MA.5.1**-playBD` — Dune: Part Two's disc audio is Dolby TrueHD Atmos; that track does not exist. (5) `-PTer` (a private-tracker internal group) served from DrunkenSlug, a usenet indexer | **Applied.** RARBG → `DDP5.1.x265.10bit-QxR`; Movies/SD → Movies/HD; 3.4 GB → 1.4 GB; DTS-HD MA 5.1 → TrueHD.7.1.Atmos; `-PTer` → `-NTb`. Everything else checked and left alone: episode counts are right (ER 331, The Americans 75, The Thick of It 23, Slow Horses 24, The Bear 28, Severance 19), the ages sort ascending as the toolbar claims, sizes are in band, and `Sun, 16 Aug 2026` really is a Sunday |
| **D-45** | MINOR | The Tags column on Search carries **exactly one chip per row in a perfect alternating pattern** — `source:torrent`, `source:usenet`, `source:torrent` — and rows with no file carry `type:movie` instead of, rather than as well as, a source. Real derived tags do not alternate | **Recorded.** Cosmetic in a mockup, but it is the kind of too-tidy data that reads as generated. Worth varying when the screens are rebuilt for real |
| **D-42** | MINOR | Twin Peaks is shown as `18/18`, which counts only *The Return*; a single Sonarr series entry would be 48 | **Recorded** |
| **D-43** | MINOR | *The Wire* S05E10 is given as "Thirty"; the episode is titled `–30–` | **Recorded** |
| **D-44** | MINOR | Search's filter bar uses an `instance:` chip, styled identically to tag chips. §16's v0.1 system tag namespaces are `type:`, `format:`, `source:`, `quality:`, `indexer:` — there is no `instance:` | **Recorded** |
| **D-29** | MAJOR | **The lint checklist forbids the artefact that implements it.** §13 says `[review] No fabricated data anywhere — including screenshots, docs and empty states`. The mockup README's first paragraph says *"Every title, size, seeder count, timestamp and error string on these pages is fabricated sample data"*. Both cannot be right, and a mockup with no data shows nothing | **Applied to the document.** The rule is now scoped to shipped product surfaces, names the mockup as the labelled exception, and gains a second clause that sample data is still checked against reality — which is the rule D-07 was actually testing against |

### D1.3 "Will this be slow?"

| # | Sev | Finding, and the evidence | Disposition |
|---|---|---|---|
| **D-12** | MAJOR | **The roving tabindex forces a synchronous layout once per candidate row, on every arrow key.** `usarr.js` filtered with `el.offsetParent !== null`; reading `offsetParent` flushes layout. At the §13 reference library the design document expects arrow-key navigation over — 10,000 rows — that is **10,000 forced reflows per keypress**, and holding an arrow key repeats it. This is the one thing in the whole prototype that would have been genuinely, measurably slow | **Applied.** Replaced with `el.closest('[hidden]') === null`, which answers the same question from the DOM tree and touches no layout. Verified: the search table is still exactly one tab stop out of 12 rows |
| **D-20** | MAJOR | **Tier 1's "render the shell, the nav, the headers and every field already held" is not achievable on a cold boot under ADR-0003, and nothing says so.** ADR-0003 is `adapter-static` with no server render: the shell *is* the JavaScript. On a first visit, or a hard refresh with an empty cache, or a phone on slow Wi-Fi, the document is blank until the bundle has downloaded, parsed and mounted — there is no shell to render progressively into. §7.2's Tier 1 quietly assumes a warm cache. Tier 0's "show nothing at all" is fine, because it only describes reads *after* boot | **Recorded for Joe, with the fix named.** The gap is cheap to close and expensive to retrofit: **inline the static shell — sidebar, toolbar, page-title slot — as real HTML into the built `index.html` fallback document**, so first paint is the chrome rather than white, and let the SPA hydrate into it. That is a build-config decision against ADR-0025's serving section, not a framework change, and it is the difference between "instant" and "instant once you have been here before" on the one visit that forms the speed opinion |
| **D-21** | MAJOR | **`contain-intrinsic-size` is the load-bearing half of §7.4's recommendation and it is never given a value, anywhere.** The browser uses that number as the placeholder height for each skipped element; when it is wrong the scrollbar jumps as content scrolls in, which reads as *slowness* — the exact failure §7 exists to prevent. It cannot be one constant, because the density control moves row height across 28 / 32 / 36 px plus three more values for two-line and thumbnail rows. The mockup uses no `content-visibility` at all, so the recommendation has never been exercised | **Applied to the document**, as a ⚠️ block in §7.4: derive it from the same custom property the row height reads (`contain-intrinsic-size: auto var(--row-h)`), test it with the density control while scrolling, and treat §7.4 as a direction rather than an implementable rule until that exists. Left unfixed in the mockup, whose tables are 10–15 rows |
| **D-16** | MAJOR | **The motion rule forbids the token two rows above it, and the reference stylesheet breaks it six times.** §6: *"Only `opacity` and `transform` may be transitioned."* The same section's table specifies "Hover / focus colour change 80 ms", `tokens.css` ships `--dur-hover: 80ms`, and `usarr.css` transitions `background-color` on `.nav__link`, `.btn`, `.segment button` and `.subnav a`. A rule the reference implementation cannot obey is a rule nobody will obey, and the greppable lint line in §13 encodes the wrong version | **Applied to the document.** The rule now permits `color` / `background-color` for the 80 ms hover row only, and explicitly names `border-width` and `text-decoration-thickness` among the forbidden geometry properties |
| **D-38** | MINOR | `a:hover { text-decoration-thickness: 2px }` — a geometry change on hover, which §6 forbids in the same breath as "hover changes colour only" | **Applied.** Hover now changes colour only; links are underlined at rest so the affordance never depended on it |
| **D-46** | MINOR | `prototype.html`'s hash router calls `window.scrollTo(0, 0)` on every `hashchange`, including Back — contradicting §7.3 rule 5, which the document argues is *the* difference between feeling fast and feeling slow | **Recorded.** A limitation of the single-file build, not of the design; noted because the prototype is what people will judge |
| **D-40** | MINOR | On a 390 px phone the fan-out bar rendered "8 of 9" as one character per line: `.fanout` was a non-wrapping flex row | **Applied.** `flex-wrap: wrap` plus `white-space: nowrap` on the count |
| **D-23** | MAJOR | **Search shows 20 scannable items above the fold at 1440×900 against the document's own `[review]` rule of ≥ 25.** Measured in Chromium, default compact density. Home passes with 27 (12 poster cards + 15 rows). The README claimed "roughly ten poster cards and fifteen table rows" for home and said nothing about search | **Applied.** README now reports both measured numbers and states plainly that search misses the threshold, rather than rounding up |

### D1.4 Conflicts with `ARCHITECTURE.md`, and roadmap honesty

| # | Sev | Finding | Disposition |
|---|---|---|---|
| **D-22** | MAJOR | **OQ-1 understates how entrenched virtualization is.** The design document cites §4.5 and §16. There is a third site, and it is an **accepted ADR**: ADR-0003 rejects HTMX partly because *"a 10k-item virtualized poster grid with instant client-side filter/sort **is** a rich client-state problem"*. Settling OQ-1 against virtualization means amending three documents, one of them an ADR — not one threshold | **Applied to the document.** The ⚠️ block in §7.4 and OQ-1 now name all three sites and the amendment cost |
| **D-30** | MAJOR | **The mockup README rebuts ADR-0025's own reason for rejecting the system stack.** ADR-0025 and §4.1 reject `system-ui` because cross-platform metric drift is *"not cosmetic"* for a fixed-row-height design. The mockup loads **no webfont at all**, and its README says: *"If Plex is not installed on the viewing machine the system stack renders instead, and that is the correct and expected result: the design does not depend on the specific face, only on the metrics band it sits in."* If that sentence is true, ADR-0025's typographic argument is not; if ADR-0025 is right, the mockup has never demonstrated the typeface decision | **Recorded for Joe.** Both statements are defensible in isolation and they cannot both stand. This is now stated inside OQ-3 rather than left to be discovered |
| **D-31** | MAJOR | **The font budget was the one unmeasured number driving OQ-3, and it is now measured.** By `Content-Length` on the WOFF2 subsets, 2026-08-16: Sans 400 = 44.6 KB, Sans 600 = 44.6 KB, Mono 400 = 14.4 KB → **103.6 KB for `latin`**; with `latin-ext`, 74.8 + 74.8 + 27.4 = **177.2 KB**. The ~120–180 KB estimate was right for `latin` + `latin-ext` and pessimistic by ~40% for `latin` alone | **Applied** to §4.1, ADR-0025's consequences and OQ-3. Neither figure trips the ~200 KB trigger, so **the question is no longer "can we afford it" but "which subset"** — and an accented library needs `latin-ext`, which is where the 73.6 KB actually goes |
| **D-32** | MAJOR | **The claim that decides OQ-3 and ADR-0025 carries no citation and no `INFERENCE` marker**, against the document's own stated convention. "San Francisco, Segoe UI and Roboto have different x-heights and different advance widths" is placed immediately before an MDN citation about something else (`system-ui` and CJK fallback), which reads as though MDN supports it. Verified against MDN: **it does not — the page says nothing about x-heights or advance widths.** The magnitude of the drift has never been measured for this design | **Applied.** Marked `INFERENCE` in both §4.1 and ADR-0025, with the reason it matters stated: it is the sole ground on which the zero-byte option lost |
| **D-15** | MAJOR | **The v0.1 sidebar ships 15 entries against the design document's stated 8.** §8.1 gives the exact v0.1 sidebar and says *"and nothing more"*. The mockups added a Settings sub-tree (General, Tags, UI) and a System sub-tree (Status, Tasks, Backup, Events, Log files). Tasks, Events and Log files are screens `ARCHITECTURE.md` §16 does not ship in v0.1 and no document specifies | **Applied in part.** Tasks, Events and Log files removed. Status and Backup kept — Backup is a v0.1 line item (`VACUUM INTO`) and Status is the system-health list this screen already renders. **The Settings sub-tree is left for Joe:** it is grounded in the settings anatomy on the Services screen, but it is still four nav entries §8.1 does not list |
| **D-14** | MAJOR | **The health count badge is on the wrong nav item.** §8.2 is explicit — *"a severity-coloured count badge on the **Services** sidebar item"* — because Services is UsArr's health screen (§17.3). The mockups put it on System, copying Sonarr's `HealthStatus` placement rather than UsArr's information architecture | **Applied.** Badge moved to Services in all four screens. The Sonarr behaviour it copies was verified live: `PageSidebarStatus.js` does colour by severity (`if (errors) kind = kinds.DANGER; else if (warnings) kind = kinds.WARNING;`) and does `if (!count) { return null; }` |
| **D-06** | MAJOR | **Requests' fan-out counts contradicted the rows on screen in two of six states.** The default state said *"8 of 9 indexers responded … HDBits returned 429 and was skipped"* with an HDBits release sitting in the table below it. The `denied` state said *"PassThePopcorn refused this account, so its releases are missing from the list"* with a PassThePopcorn row visible. A partial-results count that disagrees with the results is worse than no count, and this screen exists to demonstrate exactly that honesty | **Applied.** Every release row now declares the states in which its indexer answered. Complete = 9 of 9, 100%, no failure note, 10 rows. Denied = 8 of 9, PassThePopcorn's row hidden, "9 shown". Partial = 4 of 9 with TorrentDay timed out, 5 rows from the four indexers that answered |
| **D-33** | MAJOR | **The sidebar's Movies and TV entries lead nowhere, and there is no library-grid screen in the mockup at all** — although §16 ships "Library grid, virtualized, keyset pagination" in v0.1. In the four-file build they are anchors into home; in `prototype.html` the link rewriter collapses `index.html#movies` to `#home`, so both land on the home page | **Applied as disclosure.** Recorded in the mockup README as a known limitation, naming the missing screen and pointing at §16. Building it is out of this review's scope |
| **D-08** | MAJOR | **A verbatim error string calls an endpoint shape the repository's own reference says does not exist.** `services.html` showed `Get "http://10.0.0.4:7878/api/v3/movie?page=1&pageSize=250": dial tcp …`. `docs/reference/arr-apis.md` line 83: *"Bare-array, **unpaged**, no sparse fieldsets: `/api/v3/movie`, `/series`, …"*. UsArr would never send that query, and "verbatim" is the one thing this column promises | **Applied.** Query string removed |
| **D-09** | MAJOR | **A quoted upstream health warning that the upstream cannot have emitted.** The Sonarr Anime row surfaced *"System time is off by more than 1 day"* next to a measured clock skew of **+212 s**. Verified against Sonarr's `en.json`: `SystemTimeHealthCheckMessage` = *"System time is off by more than 1 day. Scheduled tasks may not run correctly until the time is corrected"* — it only fires above one day. The screen was quoting a warning that contradicts the number beside it | **Applied.** Replaced with two health-check strings that fit the instance and are verbatim from the same file: *"Indexers unavailable due to failures: Nyaa"* and *"Enable Completed Download Handling"* |
| **D-34** | MINOR | The `[grep] No — (U+2014) in any string under 15 words` lint rule bans a string `ARCHITECTURE.md` §17.7 mandates verbatim: *"Radarr 4K is unreachable — showing cached data from 14:02"*, eight words. §17 wins | **Applied to the document.** The rule now carries the §17.7 exception rather than the banner carrying a rewrite |
| **D-47** | MINOR | ADR-0003's consequences still cite *"the bespoke optimistic-write store (ADR-0012)"*, superseded by ADR-0012a, which removed optimistic apply. The design document gets this right (§7.3 rule 3, "acknowledged, not optimistic") | **Recorded.** Pre-existing, outside this review's files |

### D1.5 Internal consistency, security and the rest

| # | Sev | Finding | Disposition |
|---|---|---|---|
| **D-03** | **BLOCKER** | **`usarr.css` silently diverged from `tokens.css` on 14 of the 15 values both define**, while its header said it "carries its own copy of the token values on purpose" and its protocol-chip comment said "these values match `docs/design/tokens.css`, which owns them" — true of the two protocol hues and of nothing else. Light `--fg` `#1b1917` vs `--n-8` `#1c1a17`; `--fg-muted` `#5e5951` vs `#5a534a`; `--border-hi` `#857f75` vs `#807869`; `--selected` `#e0ddd6` vs `#e7e3dd`; `--focus` `#14120f` vs `#1c1a17`; four different status hues in each theme; a different dark page ground. This is how a "single source of truth" file stops being one, and it is what caused D-01 | **Applied.** Every shared value is now `tokens.css`'s, with the measured ratio in the comment beside it, and the file header says the two cannot drift |
| **D-04** | **BLOCKER** | **On a phone the sidebar took 208 px of a 390 px viewport — the exact bug §8.3 says it is designing against.** `.app[data-sidebar="open"]` re-established the two-column grid inside the ≤ 900 px media query, and the markup ships `data-sidebar="open"`, so every phone load rendered the nav over half the screen. Measured: content column 182 px, home's banner reading one word per line. §8.3 names Sonarr [#7757](https://github.com/Sonarr/Sonarr/issues/7757) ("sidebar hides content") and Prowlarr [#2431](https://github.com/Prowlarr/Prowlarr/issues/2431) as "the specific failure mode to design against, not a checkbox" | **Applied.** Below 900 px the sidebar overlays rather than taking a column, and `usarr.js` starts it collapsed. Re-measured at 390×844: content column 390 px, sidebar hidden. A second defect surfaced during the fix and was also corrected — `[data-sidebar="collapsed"]` kept a two-column `grid-template-columns` under a one-column `grid-template-areas`, which auto-placed `main` at zero width |
| **D-13** | MAJOR | **"Render the upstream error verbatim" reached the DOM through `innerHTML`.** `usarr.js`'s `toast()` concatenated the upstream string into an HTML literal. In the mockup the string is a fixed literal, but this file is the reference implementation of a rule that applies to **text an \*Arr or an indexer controls**, in a project whose `CLAUDE.md` treats SSRF and credential handling as first-class. Teaching "verbatim means innerHTML" is how that becomes real | **Applied.** Title and verbatim body are set with `textContent`; `setChip` builds its element rather than assigning markup |
| **D-26** | MAJOR | **`tokens.css` uses pure black in its only shadow, breaking its own rule, and the lint line cannot catch it.** File header: *"`#fff` and `#000` do not appear in this file and must not appear anywhere in the app."* Line 310: `--shadow-overlay: 0 2px 8px rgb(0 0 0 / 0.18)`. §13's rule greps for `#000` / `#000000` / `black`, none of which matches `rgb(0 0 0 / …)` | **Applied.** Changed to the warm near-black `rgb(28 26 23 / 0.18)`, so the ramp has no exception, with the reason recorded in the file. The mockup's two black-derived literals were warmed to match |
| **D-27** | MAJOR | **`tokens.css` is missing two roles the first real screen needed.** `usarr.css` had to invent `--inset` (the interior of an input, one step *lighter* than the page ground in light and darker in dark) and a `--hover` fill distinct from `--surface`, because `tokens.css` maps `--bg-raised` and `--bg-hover` to the same `--n-1`. With one fill for both, a hovered row is painted the same colour as the sticky header above it | **Recorded for Joe.** The mockup now labels both as mockup-local and measures against them, rather than inventing them silently. **They need a decision against `tokens.css` before the first UI commit**: either add two roles, or accept that a hovered row and the table head are the same colour |
| **D-28** | MAJOR | **The generator for a committed generated file lived outside the repository.** The README said `prototype.html` is generated and *"do not edit it by hand"*, then said the generator *"lives outside the repo, in the scratchpad, and is intentionally not committed"* — which means nobody else can regenerate it, which is how a generated file quietly becomes hand-maintained | **Applied.** `build_prototype.py` is committed beside its output and made path-relative |
| **D-35** | MINOR | Density keys disagreed: the mockup used `default` where `tokens.css` uses `standard`. Row padding also disagrees (mockup compact 4 px, `tokens.css` 6 px), and `--row-2line` / `--row-thumb` are fixed in the mockup rather than tracking density as `tokens.css` specifies | **Applied** for the key names, which are a genuine mismatch. The padding and two-line values are **recorded**: `tokens.css` is the authority and the mockup should adopt its three-per-density set when the screens are built for real |
| **D-36** | MINOR | `tokens.css`'s compact arithmetic does not close: a 28 px `--row-h` with `--row-pad-y: 6px` and an 18 px line box is 30 px. Because these are `min-height`, nothing breaks — but "compact is 28 px" is not what renders (measured: 30 px) | **Recorded** |
| **D-37** | MINOR | `role="listitem"` on `<a class="card">` replaces the link role, so assistive technology stops announcing a focusable navigation element as a link | **Applied** by removing both `role="list"` and `role="listitem"` from all four strips. **Recommendation for the real build: `<ul><li><a>`**, which gives set size and position back |
| **D-39** | MINOR | Base URLs broke mid-token in the Services table: `http://10.0.0.4:898` / `9`. `.tbl td .mono { overflow-wrap: anywhere }` is right for release names and wrong for URLs | **Applied.** The sub-line breaks at word boundaries, the service column has a minimum width, and the verbatim cell is narrowed |
| **D-41** | MINOR | Inline `style="margin-top:8px"`, `style="margin-left:0"`, `style="max-width:120px"` and similar in five places, against the token rule. The `--dc` dominant-colour properties on cards are legitimate — that is data | **Recorded** |
| **D-48** | MINOR | **Citation audit.** Ten primary sources re-fetched live. **Nine verified verbatim:** Sonarr `PageSidebarStatus.js` (severity kinds, `return null` at zero); Sonarr `Styles/Themes/dark.js` (`torrentColor: '#00853d'`, `usenetColor: '#17b1d9'`); Sonarr `PageSidebar.js` (Series/Calendar/Activity/Wanted/Settings/System with the stated children, `statusComponent` on Queue and Status); Prowlarr `PageSidebar.js` (Indexers → Stats, Search, History, Settings, System); MDN `content-visibility` (*"the skipped contents must still be available as normal to user-agent features such as find-in-page, tab order navigation, etc."*, Baseline September 2024); NN/g skeleton screens (all three quotes); WCAG 1.4.11 (*"2.999:1 would not meet the 3:1 threshold"*); Krebs (1,590 sites, 16 detectors, 22 / 32 / 46%, 5–10% false positives, Inter and dark-theme contrast both scored); Viget (39 / 39 / 58, 2.82 / 2.41 / 2.29 s, 59 / 74 / 66%, 10.54 / 9.49 / 9.50 s); NN/g animation duration (both quotes); MDN `font-family` (the `system-ui` warning, quoted correctly). Sonarr's `en.json` strings all check out locally, including `KeyboardShortcutsFocusSearchBox` = "Focus Search Box". **One could not be verified:** `wiki.servarr.com/prowlarr/search` renders client-side and returns no body — the Options/Filter vocabulary claim is independently corroborated by Prowlarr's own `en.json`, which ships both `Options` and `Filter`. **One miscitation found and fixed: D-32.** Everything else in the document says what it claims its source says | **Applied** (D-32); the rest **recorded as verified** |

---

## D2. Rebuttals

### D2.1 D-05, the 2160p column — the *deletion* is rebutted, the *finding* is not.

The reviewer's own test says delete it. The reviewer is not deleting it, because which columns a
screen carries is a design decision and this one demonstrates a documented §6.3 rendering rule
(`have == total && total > 0` → ✓; `have == 0` → ✗; otherwise the fraction; and where no instance
of that tier exists at all, "not configured" — which is *not* the same as a cross, and is exactly
the distinction the column was drawn to make). What was fixed instead is the part that was
unambiguously wrong: the TV tables rendered a **cross** in the 2160p column while the movie tables
rendered **"not configured"**, on an install whose Services screen lists no 2160p-capable Sonarr at
all. A cross claims an instance was asked and had nothing. Twenty-six cells corrected.

The consequence is that the redundancy is now *more* visible, not less, which is the right outcome
for a review: Joe should look at the search screenshot and decide.

### D2.2 The achromatic chrome, the sidebar, the font and Tailwind were not reversed.

All four are documented decisions, three of them with an open question already attached. This
review's job was to attack the reasoning, not to overturn it:

- **Achromatic chrome (§3)** survives the attack. The argument is structurally sound — the tell
  cannot be present if the category is absent — and the poster strip proves the cover-art-supplies-
  the-chroma claim works in practice. Its one real cost is D-24, recorded rather than used as a
  lever.
- **The left sidebar (§8.1, OQ-2)** survives, and is well-evidenced: verified live against both
  Sonarr's and Prowlarr's actual `PageSidebar.js`. The finding against it is D-15 — it shipped
  *more* than §8.1 specified — not the choice itself.
- **IBM Plex (§4.1, OQ-3)** survives on cost, which is now measured (D-31), but its *argument*
  took two hits: the deciding claim is uncited inference (D-32) and the mockup README contradicts
  it outright (D-30). The decision is fine; the reasoning needs one of those two resolved.
- **Tailwind with the theme deleted (ADR-0025, OQ-7)** is untouched by this review. Worth noting
  only that the mockup is hand-written CSS, so ADR-0025's central claim — that `@theme { --*:
  initial; }` makes the generic look *structurally impossible* — has not been exercised by
  anything yet.

### D2.3 What the review looked for and did not find.

Recorded so it is not re-hunted. **The literal ban lists are clean**: no indigo/violet/purple hex
or class, no gradient of any kind, no `backdrop-filter`, no coloured or glowing shadow, no
`rounded-2xl`, no radius above 4 px, no `#fff`/`#000` literal outside the one D-26 caught, no
Inter/Geist/Space Grotesk/Instrument Serif/Poppins, no Google Fonts reference, no emoji anywhere in
markup or CSS, no `outline: none` without a `:focus-visible` replacement, no animation library, no
`startViewTransition`, no banned marketing verb in any string, no exclamation mark, no first-person
plural, no hero, no badge-pill-above-title, no three-card feature grid, no stat banner, no numbered
step row, no testimonial or pricing or FAQ block, no bento grid, no icon in a tinted chip. **Every
icon earns its place** — fifteen glyphs, used only as row/toolbar actions, media-type markers and
status marks, and the sidebar has none, which is the right call. **Padding genuinely differs by
role** (2–4 px intra-row, a 1 px rule between rows, 16–24 px between regions), so the uniform-
rhythm tell does not land. **Density is real**: 30 px measured rows, 27 items above the fold on
home. **The unhappy paths really are the product**: 20 distinct states across four screens, every
error carrying verbatim upstream text in mono, and Search's filtered-empty genuinely differing from
its empty. That is the strongest thing about this work and it should not be lost in the list above.

---

## D3. What Joe has to decide

Everything else in this log is either applied or recorded. These need him.

| # | Decision | Why it cannot be settled below him |
|---|---|---|
| **1** | **OQ-1, virtualization.** Now a three-document amendment including an accepted ADR (D-22), and §7.4's alternative is not implementable until `contain-intrinsic-size` has a value and a test (D-21) | It is a functional trade — Ctrl+F in a library browser against a documented performance rule — with a real cost either way |
| **2** | **OQ-3, and it has changed shape.** Cost is measured and affordable (D-31); what is left is the *subset* (`latin` at 103.6 KB vs `latin-ext` at 177.2 KB), plus the fact that the argument which beat the system stack is uncited inference (D-32) and the mockup README contradicts it (D-30) | Two documents currently say opposite things about whether the specific face matters |
| **3** | **The 2160p column** (D-05). Render a tier column only when an instance of that tier is connected, and state the absent tier once in the section header? | It is the clearest "delete it and lose nothing" failure in the prototype, and fixing it is a screen-design change |
| **4** | **§5.3 versus both busy screens** (D-17). Relax the rule to "inline targets in a compact row must satisfy the SC 2.5.8 spacing exception, checked in CI" — which is what the mockup measurably does — or hold it and move Requests and Services to relaxed | The document and its reference implementation currently disagree, and the mockup is the one that is WCAG-legal |
| **5** | **The two missing token roles** (D-27): add `--bg-inset` and a `--bg-hover` distinct from `--bg-raised` to `tokens.css`, or accept a hovered row painted the same colour as the sticky header above it | `tokens.css` is the single source of truth and the first real screen could not be built from it alone |
| **6** | **The cold-boot shell** (D-20): inline a static app shell into the built fallback document, so a cold first visit paints chrome instead of white | It is the visit that forms the speed opinion, and it is a build-config decision under ADR-0025 |
| **7** | **The Settings sub-tree in the sidebar** (D-15) and, behind it, whether §8.1's "and nothing more" list is the real v0.1 navigation | OQ-2 is still open, and this is the same question in smaller print |

---

# UsArr — Round 3: corrections the six-media-type research forced

**Date:** 2026-08-16. **Input:** five deep-research passes — user-defined libraries
(`R-lib`), multi-type information architecture (`R-ia`), books/ebooks/audiobooks (`R-book`), music
(`R-music`) and comics/manga (`R-comic`) — read against `CLAUDE.md`, then `ARCHITECTURE.md` §16/§17,
`DECISIONS.md` and `FUTURE.md`, in that order of authority.

**This is not an adversarial review.** It is the disposition log for research findings that **overturn
claims already committed to this repository**, which `CLAUDE.md` requires to be applied or rebutted in
writing rather than silently dropped. Every finding below was checked against the citation in the
research file before it was written here; where a research report could not verify something, the
caveat is carried rather than tidied away.

## Counts

| Disposition | Count |
|---|---|
| **Applied in full** | **11** |
| Applied, with the research's own caveat carried forward | 3 |
| **Two research reports contradicted each other; resolved in writing** (§R2.1) | 1 |
| Recorded for Joe — a decision this log may not take | 4 |

---

## R1. Corrections to claims already in the repository

| # | Finding, and the source that overturns it | Disposition |
|---|---|---|
| **R-01** | **MusicBrainz's Live Data Feed is CC BY-NC-SA 3.0, not CC0.** `RESEARCH.md` §5.2 recorded *"CC0 core data… Offers a Live Data Feed + full local replica — the escape hatch from 1 req/s"*. Per the MusicBrainz Data License page, **core data is CC0 but supplementary data and the Live Data Feed replication packets are CC BY-NC-SA 3.0**. So the local-replica escape hatch is not unencumbered: it is non-commercial-licensed. For an AGPL hobby project the practical risk is low; the cell was still wrong | **Applied.** `RESEARCH.md` §5.2 cell rewritten, with the licence split stated per component and the correction dated. Also added from the same page, because both change the design: exceeding 1 req/s declines **100% of requests, not the excess** — a burst does not degrade, it stops — and MusicBrainz asks explicitly that applications **not poll** and that calls be made at **random intervals**, so the provider scheduler needs jitter, not a cron |
| **R-02** | **Last.fm publishes no numeric rate limit, and is non-commercial only.** `RESEARCH.md` §5.2 recorded *"≤5 req/s per IP averaged over 5 min 📄"* — with a **primary-source marker**. That number is not in the Last.fm API Terms of Service, which say limits are set and enforced *"in our sole discretion"* (**§4.4**). §3.1–3.2 restrict use to **non-commercial purposes** | **Applied.** Cell rewritten to ⚠️, the folklore provenance stated, and the non-commercial restriction added — which the earlier cell omitted entirely |
| **R-03** | **`reference/gateway.md` §2 gives the wrong OpenSubsonic error codes.** It specified **error 40** both for "`apiKey` and `u` both present" and for "salt/token without `apiKey`". The apiKeyAuth extension spec: *"passing in `u` must be treated as an **error 43**"*; *"If multiple conflicting authentication parameters are passed in, the server must return an **error 43**"*; *"If a server removes support for token-based authentication, it must return **error 41**… for any other particular authentication mechanism… an **error 42**"* | **Applied.** 43 for `apiKey`+`u`, 42 for an unsupported mechanism, 41 for the removed-token-auth case. The consequence is user-visible and is why it matters: the wrong code makes a client show "wrong password" instead of "this server needs an API key" — the exact confusion the hard-rejection policy exists to prevent |
| **R-04** | **`reference/gateway.md` omits `helpUrl`.** The same spec: *"it is recommended that the server provide a meaningful url… in the `helpUrl`"* — a field introduced precisely for an auth refusal | **Applied.** Populating `helpUrl` with UsArr's own API-key page is now normative on every auth refusal |
| **R-05** | **Two of the three named OpenSubsonic reference clients cannot authenticate to UsArr at all.** `ARCHITECTURE.md` §3's diagram names *"Symfonium · Amperfy · Feishin"*. Read from source: **Amperfy** has **zero** occurrences of `apiKey` in its entire Swift source and builds `u` + `t`/`s`; **Feishin**'s Subsonic controller has no `apiKey` path. Both are salt/token only | **Applied. The policy is right and the matrix was wrong** — which is a `CLAUDE.md` "no invented status" issue, because it applies to capability claims too. §3 now carries the caveat, `gateway.md` §2 repeats it, and §16's v0.4 states the matrix as *Symfonium works; Amperfy and Feishin do not, and that is the price of refusing to store recoverable passwords* |
| **R-06** | **Audiobooks are Newznab category `3030`, under `Audio` (3000), not under `Books` (7000)** — verified in `NewznabStandardCategory.cs`. `ARCHITECTURE.md` §8.5 derives the `type:` tag from the **parent** category, so **every audiobook release would be tagged `type:audio`** | **Applied as a bug fix, not a footnote.** §8.5 now derives `type:` from the category with the special cases (`3030`, `5070`, `7020`, `7030`, `7010`) consulted **before** the parent rule, and `reference/arr-apis.md` §8 carries the same correction. **⚠️ Corrected 2026-08-16 (round-4 C-19): this disposition originally overstated what landed.** §8.5 as written named only `3030`, `7000`, `7020`, `7030`, `7040` and `7060` — **`5070` and `7010` had rules in neither file**, which is the same bug this finding is about, since a parent-first rule mislabels `TV/Anime` and `Books/Mags` exactly as it mislabels an audiobook. All five now carry an actual rule in both files, tabulated. The research called this "a bug waiting to happen and it should be fixed in the tag derivation, not papered over"; it is |
| **R-07** | **`7030 Books/Comics` is the only comics category in the Newznab standard, there is no manga category, and Nyaa maps manga to `7000`** — verified in `NewznabStandardCategory.cs` and in `Prowlarr/Indexers/definitions/v11/nyaasi.yml`, where Nyaa's `Literature` categories (`3_0`…`3_3`) map to `Books`. So `reference/arr-apis.md` §8's `7030→comic` mapping **returns zero manga** | **Applied.** A comics-and-manga search filters on the parent **`7000`** and uses `7030` only as a *ranking* signal for western comics. Scale added for expectations: of 543 definitions in `definitions/v11`, 88 declare `Books/Comics` and only **three** are comic- or manga-specific — and **GetComics, the dominant western-comics DDL source and the only one Kapowarr searches, is not a Prowlarr indexer at all** |
| **R-08** | **`reference/security.md` §5's redaction rule misses two real secret locations.** It is a query-parameter deny-list plus two headers. **Mylar3's `listProviders` returns configured indexer API keys in the *response body*** — and UsArr logs upstream response text **by requirement**, since §17.3's "Problem" column is verbatim. **Kavita carries its API key in a *URL path segment*** (`/api/Opds/{apiKey}/…`), not the query string, so it lands in proxy logs, browser history and `Referer` | **Applied.** Redaction must also run over upstream **response bodies** before they are logged, stored in `sync_report`, shown on Services or put in a support bundle; and it must operate on the **path** as well as the query, driven by each provider's declared credential placement rather than a fixed parameter list. Both were one-line additions now and would have been discovered from a leaked support bundle later. Noted alongside: **Kavita's `GET /api/Image/web-link?url=…`** is a live example of the `derived` SSRF class the earlier threat model omitted — already covered correctly by §2's per-row origin rule |
| **R-09** | **`ARCHITECTURE.md` §11.2 lists Calibre-Web as manifest-covered, and it has no REST API.** It exposes OPDS (Atom) and `/ajax/listbooks`, which is session-cookie authenticated. Neither is a manifest target, and reconstructing a library by parsing Atom on a schedule is slow, fragile and lossy — no identifiers survive the feed | **Applied.** Calibre-Web removed from the manifest list; the right adapter is **Tier 0 Go code opening Calibre's own `metadata.db` read-only**, which is the best ebook data most self-hosters own (`identifiers(book, type, val)` is a native typed external-id table, `books.uuid` is durable, `data(book, format)` is genuinely multi-format, `series_index REAL` sorts correctly, `last_modified` is a real delta key). **That is a filesystem read and is written down as an explicit exception** — a read-only handle on one SQLite file, not a scanner. Calibre-Web stays as the link-out target and byte server. Suwayomi removed from the same list for a different reason: it is GraphQL |
| **R-10** | **`work.kind` has one comic member where it needs two**, and its `work_comic` subtype describes an *issue* while the corpus rule, the prefix index, the `kind_byte` map and the grid treat `comic` as top-level — the same shape of contradiction as the `audiobook` kind-vs-edition one (C-10) | **Applied in migration 0001, which is the only cheap moment.** ADR-0030. Fixing it later means a CHECK-constraint change (a SQLite table rebuild), an FTS re-index, a rebuild of every client prefix index, and a change to the `kind_byte` codec that §5.3 calls **unchangeable once clients cache ids** |
| **R-11** | **`work_track` is work-scoped and track position is edition-scoped.** MusicBrainz: a recording is *"distinct audio"*; a track is *"the way a recording is represented on a particular release (or, more exactly, on a particular medium)"*. The same recording is track 4 on the original CD and track 6 on the 2017 reissue, with a different track MBID each | **Applied.** ADR-0031: `work_track` gains `edition_id`, `track_number` becomes `TEXT` with an integer sort key (Lidarr ships exactly this pair, because real values are `A1`, `B2`, `1.01`), and attribution becomes an M:N `work_credit` rather than a scalar `artist_id`. All are migration-0001 or they are backfills over the largest tables in the schema |
| **R-12** | **`edition` is missing the three columns every audiobook authority puts on it** — narrators, duration and abridgement. Chaptarr's `EditionResource` carries `Narrator`, `NarratorNames[]`, `DurationSeconds`, `ChapterCount`; Audiobookshelf carries `Book.narrators`, `Book.duration`, `Book.abridged` | **Applied.** `edition` gains `narrators`, `duration_seconds`, `abridged`. Also applied from the same source: the `edition.format` CHECK list lacked `cbz`/`cbr`/`pdf` while `ARCHITECTURE.md` §6.1's prose already listed `cbz` — an inconsistency that had to be resolved before migration 0001, not after |
| **R-13** | **ISBN and ASIN must never satisfy `ux_extid_work_strong`**, and book tier-3 matching needs the author. An ISBN is per-edition, per-format and per-territory; the same audiobook usually has an ASIN and no ISBN; publishers reuse ISBNs and put one on an omnibus. Book titles collide far more than film titles | **Applied** to §6.4, with the Open Library redirect rule beside it: a merged work becomes a `/type/redirect` record, so **an OLID stored last month can resolve to a redirect today** — adapters must follow `location` before writing the id. This is why `work_merge` moves forward to the first music milestone rather than waiting for identity tiers 2–5 |
| **R-14** | **A user correction must survive an upstream rescan, and LazyLibrarian proves it does not by default.** Books marked ignored are re-added after an author rescan because the rescan returns the book with a different provider id (LazyLibrarian GitLab issue #2407, ⚠️ no maintainer resolution shown) | **Applied** as the load-bearing rule of ADR-0026: a correction is keyed to **UsArr's** identity (`work_id`/`link_id` + `target_identity_hash`) and is **never cleared** by a sync, a sweep, a tombstone expiry or an id resurrection |

## R1.2 Caveats carried forward rather than tidied away

`CLAUDE.md` requires that where a research report says something could not be verified, the caveat
travels with the claim. Three are load-bearing:

1. ⚠️ **Symfonium's `apiKeyAuthentication` support is unverified**, and **v0.4's entire success
   criterion depends on it** — *"Symfonium connects to UsArr with one API key, browses, searches and
   plays"*. Its documentation does not mention API keys, its changelog is on a forum that could not
   be enumerated, and the app is closed-source. The research called this the highest-risk unverified
   item in its report. **Recorded in §16 and in `gateway.md` §2: verify against a live Symfonium
   before writing a line of gateway code.**
2. ⚠️ **Komga's delta strategy rests on an unverified probe.** Neither Komga nor Kavita has a
   "changed since" endpoint, so delta is sort-by-modified plus paginate — and **whether Komga accepts
   `sort=lastModified,desc` on `POST /series/list` could not be determined from the spec**, because
   Spring `Pageable` sort properties are not enumerated and the DTO field name may not be the entity
   property name. The research named it "the single highest-value thing to probe against a live
   instance". Recorded in ADR-0032.
3. 🔍 **No research exists on carousels in media libraries.** Every carousel finding cited in
   ADR-0028 measures marketing or ecommerce contexts. The transfer argument is that the *interaction*
   is identical and that the content here is weaker than a marketing hero, not stronger. **It is
   reasoning, and ADR-0028 marks it as such rather than quoting it as a finding.** The decisive
   argument there is UsArr's own above-the-fold arithmetic, which needs no external source at all.

---

## R2. Where two research reports disagreed

### R2.1 Home: sectioned by library, or a unified table? — resolved toward the IA report.

`research-libraries.md` §11 proposes that **"Home changes from 'one section per media type present' to
'one section per enabled library with `include_on_home`'"**, arguing it is a replacement rather than
an addition and therefore nearly free. `research-multitype-ia.md` §4 proposes the opposite: **home
becomes three fixed blocks whose height is O(1) in the number of types**, and libraries never appear
as sections at all.

**The IA report wins, and the reason is a cardinality argument rather than a preference.** A media
type is a closed enum of six; a library is user-defined and unbounded. Sectioning Home by library
reproduces, one screen over, exactly the failure the same report documents in Jellyfin's shipping
code — `loadRecentlyAdded` iterating `userViews` and emitting one carousel per library, unbounded,
inside a single home slot. A user with fifteen libraries would get fifteen home sections. The
libraries report's own framing is what makes the resolution clean: it argues Home-by-library is free
*because in the default topology one library per kind makes the two identical* — which is true, and
is precisely why nothing is lost by keeping the axis that stays bounded when the topology is not
default.

**What is kept from the libraries report:** `include_on_home` remains a `library` column (it is also
Kavita's dashboard opt-out and Plex's *Visibility*), and it feeds the **scope**, not a section list.
Both reports agree on the rule that decides most of the rest — a type, section, group or control with
no content is not rendered at all.

**Recorded in ADR-0027 and ADR-0028.**

> ⚠️ **Corrected 2026-08-16 (round-4 C-26). The resolution above stands; the rebuttal in the second
> paragraph does not.** *"A user with fifteen libraries would get fifteen home sections"* is a
> strawman of the proposal it rejects, because that proposal is *"one section per enabled library
> **with `include_on_home`**"* — the flag is an explicit per-library opt-in and is the proposal's own
> bound on section count. **The argument that actually decides it is ADR-0028's above-the-fold
> arithmetic**, which kills strips at *one* section per library just as it does at fifteen, plus the
> observation that an opt-out list is Jellyfin's answer and is seven checkboxes compensating for a
> layout that does not scale. Written out in full in **§4.4.2**. The consequence is also taken:
> **`include_on_home` is cut from the `library` table** rather than kept, because under a
> three-fixed-block Home nothing reads it.

### R2.2 Two smaller disagreements, both resolved without a rebuttal

- **The correction surface's milestone.** `research-libraries.md` §12 recommends schema-and-screen in
  v0.1 and the correction UI in v0.3, and explicitly recommends *against* trading Search-and-Grab out
  of v0.1 to pull corrections forward. ADR-0032 takes that recommendation, and the reason strengthens
  it: under the amended roadmap, Search-and-Grab is the **only** request path for four of the six
  media types, so cutting it would remove the thing that makes deferring the command sinks
  affordable.
- **Whether Navidrome belongs in v0.1 or v0.4.** `research-music.md` §8(d) argues Navidrome should be
  a **read** source in the milestone *preceding* the gateway, marking it 🔍 as a scoping observation
  rather than a verified fact. ADR-0032 adopts it and carries the inference marker.

---

## R3. Where the research contradicts a decision the owner has already made

Two, and both are recorded rather than acted on, because they are his to settle.

1. **Manga titles and the font subset.** The owner confirmed IBM Plex, and OQ-3's remaining half is
   `latin` (103.6 KB) versus `latin`+`latin-ext` (177.2 KB). The six-type expansion **pushes against
   the cheaper answer**: a manga, classical-music or translated-fiction library is full of accented
   and transliterated titles, and romaji/native/English alternate titles are the strongest
   alternate-title matching case in the whole project (*Shingeki no Kyojin* / *Attack on Titan* /
   *進撃の巨人*). `latin-ext` does not cover CJK either way, so the honest statement is that neither
   subset renders native manga titles and `latin-ext` merely covers the transliterations. Recorded in
   the narrowed OQ-3.
2. **The achromatic-chrome cost got slightly larger.** D-24 recorded that with no accent hue, two
   dense table screens do not differ at thumbnail scale. Six media types add more table screens, so
   the cost compounds. **This is not a reason to reopen the decision** — the owner confirmed it, and
   the type chip is deliberately neutral because a type is data, not status. Recorded so it is not
   rediscovered as a new finding.

---

## R4. What still needs Joe

Everything above is applied or recorded. These are not ours to take.

| # | Decision | Why it cannot be settled below him |
|---|---|---|
| **1** | **OQ-3, narrowed: the font subset.** Family settled; `latin` vs `latin`+`latin-ext` is a 73.6 KB first-paint cost against accented and transliterated titles across four of the six types | It is the only remaining decision here that costs bytes on first paint, and two documents still disagree about whether the specific face matters (D-30, D-32) |
| **2** | **Verify Symfonium's API-key support before v0.4 is scoped.** Not a judgement call so much as an errand only he can run — install it, point it at a stub, read the query string | v0.4's success criterion is written in terms of a capability nobody has confirmed exists |
| **3** | **Whether Prowlarr-only remains an honest v0.1 story for *music*.** The best music trackers are private and invite-only, so "runs over just Prowlarr" is materially weaker for music than for film | It is a positioning claim, not an implementation detail |
| **4** | **The five earlier round-2 items in §D3 are unaffected and still open** — the 2160p column (D-05), §5.3 versus the busy screens (D-17), the two missing token roles (D-27), the cold-boot shell (D-20) and the Settings sub-tree (D-15) | Unchanged by this round; listed so they are not lost behind it |

---

# UsArr — Round 4: three adversarial reviews of the six-type revision

**Date:** 2026-08-16. **Branch:** `claude/hearth-thread-vn9w7u` @ `f3153c6`. **Inputs:** three
independent adversarial reviews, run against the whole repository after the six-media-type expansion
landed:

| Review | Dimension | Findings |
|---|---|---|
| **Correctness** (`C-nn`) | citation fidelity · ADR coherence · amendment hygiene · research faithfulness · schema soundness · silently dropped recommendations | 3 BLOCKER · 17 MAJOR · 11 MINOR = **31** |
| **Speed** (`P-nn`) | perceived speed · buildability · accessibility. Every number measured by the reviewer in Chromium via Playwright at 1440×900 and 390×844, both themes; contrast recomputed from the hex literals; spec claims re-fetched and quoted verbatim | 3 BLOCKER · 17 MAJOR · 9 MINOR = **29** |
| **Honesty** (`H-nn`) | "no invented status" · "cut before you add" · principle 3, honest degradation | 4 BLOCKER · 15 MAJOR · 11 MINOR = **30** |
| | | **90 findings** |

> **A note on the prefixes, because they collide with earlier rounds.** Round 1 used `C-`, `S-` and
> `P-`; round 2 used `D-`; round 3 used `R-`. This round reuses `C-` and `P-` for different reviews
> and adds `H-`. **Within this section they always mean round 4.** Where an earlier finding is
> referenced it is written as, for example, *"round-1 C-15"* or *"D-05"*.

**`CLAUDE.md` requires that findings are never silently dropped, so all 90 are dispositioned below —
MINORs individually, not in a summary paragraph.**

## Counts

| Disposition | Count |
|---|---|
| **Applied in full** — the document changed and nothing is outstanding | **55** |
| **Applied to the documents; the artefact half assigned to the concurrent mockup pass** | **19** |
| **Assigned to the concurrent mockup pass** — the finding is wholly a change to `docs/design/mockups/`, which another agent owns and which this pass did not touch | **15** |
| **Recorded, not applied** — outside this pass's remit, escalated with the exact change named (H-29) | **1** |
| | **90** |

**Nothing is rebutted in whole. One finding is rebutted in part** — H-11's first of two options —
argued in §4.4.1; its second option is applied in full, so H-11 is counted above as applied.

By review: **correctness** 29 applied · 1 doc-side with the artefact assigned · 1 to the mockup pass.
**Speed** 13 applied · 12 doc-side with the artefact assigned · 4 to the mockup pass. **Honesty** 13
applied · 6 doc-side with the artefact assigned · 10 to the mockup pass · 1 recorded.
**34 findings touch `docs/design/mockups/` in whole or in part**, which is where the honesty
review's weight lands: 16 of its 30 findings are against artefacts rather than against documents.

**Nothing in `docs/design/mockups/` was edited by this pass.** A concurrent agent owns that
directory; every mockup finding below is recorded with enough detail to be actioned there, and where
the same finding also had a document half, that half is applied here and marked.

---

## 4.1 Correctness — disposition of all 31

| # | Sev | Finding, in one line | Disposition |
|---|---|---|---|
| **C-01** | **BLOCKER** | The four library tables and `work_credit` have no DDL anywhere, but ADR-0026, §6.5 and ADR-0031 all say they do — for the one migration that can never be edited | **Applied.** New [`reference/schema.md`](./reference/schema.md) **§13** carries `library`, `library_source`, `library_member` and `library_override` at the rigour of the existing tables — STRICT, CHECK lists with their real allowed values, keys, `ON DELETE` behaviour, indexes and the `user_id` principle 4 requires — plus `work_credit` in §1.1. C-02, C-03, C-08 and C-16 are resolved **inside** the DDL, as the finding asks, rather than as prose after it |
| **C-02** | **BLOCKER** | `library_member` is work-granular, which makes §17.8's flagship Ebooks/Audiobooks split unimplementable | **Applied**, taking the first of the two offered options because it is the honest one. Membership is keyed `(library_id, sort_title, work_id, edition_id)` with `edition_id NOT NULL DEFAULT 0` as the "whole work" sentinel. The reviewer is right that a work-grained key cannot express it: `library.formats` filters `edition.format` while `library.kind` filters `work.kind`, and a `book` work with an EPUB and an M4B edition is one row, so the Ebooks library would contain audiobook-only works unless the format filter were re-evaluated at query time — which defeats the materialisation and makes every item count wrong. §17.8's *"it costs one `formats` column"* is corrected to name the membership key as well |
| **C-03** | **BLOCKER** | Three of the five `container_kind` values cannot be derived from any column in the schema — including `remote_library`, the only container available for the four new sources | **Applied.** `service_item_link` gains `remote_library_id TEXT`, `remote_tag_ids TEXT` (JSON) and `remote_subtype TEXT`, all stored **verbatim as the upstream reported them**, plus `ix_sil_container`. `schema.md` §5 tabulates the predicate per container kind and the source field per adapter, and §13.2 repeats the predicate table. Two things surfaced while writing it and are recorded rather than buried: ⚠️ **Audiobookshelf's `folderId` is an opaque id and not a path**, so it must never be used with the prefix-matching `root_folder` kind; and ⚠️ **the `tag` predicate is a `json_each` scan, not a seek**, which is acceptable on the background derivation path and must not be copied onto a query path |
| **C-04** | MAJOR | `gateway.md` prescribes error 42 where the spec sentence it quotes requires 41, and never mentions error 44 | **Applied.** The row is split four ways: `u`+`t`+`s` → **41** (UsArr *is* the removed-token-auth case the spec reserves 41 for), `p` → **42**, `apiKey`+`u` → **43**, unknown/revoked `apiKey` → **44**. The error list at the top of §1 now leads with 44 and ends with 40, since *"wrong username or password"* is meaningless on a server with no usernames. Round-3 R-03's disposition text recorded the correct taxonomy and the table then applied it backwards; the taxonomy was right and the application was not |
| **C-05** | MAJOR | The Runyon carousel statistic is misattributed in three places and carries no citation at all | **Applied**, after re-fetching both articles live. **28,928 is ND.edu alone**, mid-October 2012 to 22 January 2013, from the **January 2013** article — confirmed verbatim: *"There was a total of 28,928 clicks on features for this time period"*, with *"84% were on stories in position 1"*. The **July 2013** five-site follow-up reports per-site click-through of **1.07 / 1.266 / 9.409 / 1.51 / 2.985%** and position-1 shares of **89.1 / 71.07 / 54.57 / 62.1 / 84.81%**, and **publishes no aggregate; 28,928 does not appear on that page**. Restated accurately, with both URLs, in all three places — ADR-0028, ARCHITECTURE §17.2 and DESIGN-DIRECTION §8.6. The reviewer is right that the corrected form is the *fairer* one to have in an ADR, because the 9.4% outlier is real |
| **C-06** | MAJOR | `koreader#14681` is **closed**, and it is the stated reason for the OPDS 1.2-before-2.0 ordering | **Applied, and re-verified independently as instructed.** The issue is **Closed, milestone 2026.07** — and the milestone shipped: **KOReader 2026.07 "Sailing Walrus" release notes carry *"there's now OPDS 2.0 basic support (#15696)"***, with 2026.07.1 following up on the OPDS 2.0 HTTP header and the author field. So *"KOReader does not speak 2.0"* is false and is deleted from §16, `FUTURE.md` §13 and README. **The ordering is not reversed; it is re-argued on the ground that still holds** — the long tail (Aldiko, Moon+, MapleRead, FBReader, Marvin) is entirely 1.2, so a 2.0-only surface excludes all of them while a 1.2 surface excludes nobody *including* KOReader, which gained 2.0 **in addition to** 1.x. What changes is that shipping 2.0 second is now a real gain for a real client rather than a hedge, which raises `FUTURE.md` §13's priority. Date-stamped, with a ⚠️ to re-verify before the milestone is scoped |
| **C-07** | MAJOR | §2.2 was never amended, so ADR-0026's narrowing sits beside the unnarrowed original — and ARCHITECTURE outranks DECISIONS, so the unnarrowed rule wins | **Applied.** §2.2 now carries the three-axis table and restates the conflict rule as *"the \*Arr owns the truth about the \*Arr's own **state**"*, with the *Owned outright* row extended to "library organisation and display-identity corrections". §6.5 no longer repeats the table — it points at §2.2, which is the section that owns the rule — so its *"which §2.2 already grants"* is now true rather than aspirational. ADR-0004's header and index row carry the back-reference (C-09) |
| **C-08** | MAJOR | §1.3's `user_id` list was not updated, and `library` has no `user_id` at all despite being called "user-owned" | **Applied, with the ambiguity resolved rather than papered over.** Both readings the reviewer identifies are live in the same document, and they need different schemas. **The resolution is that both objects exist and mean different things:** `library.user_id` records the **owner** — the user whose name, ordering and corrections the row carries — and `user_library_access` (v1.0) records a **grant of read access to a different user**. That is Plex's shape, it satisfies principle 4, and it makes ADR-0028's per-user media-type ordering per-user by construction rather than by a second table. `library` and `library_override` are added to §1.3's enumeration; the reasoning is in `schema.md` §13.1 |
| **C-09** | MAJOR | ADR-0003 is not annotated and its uncorrected sentence still reads as current; ADR-0004, ADR-0009 and ADR-0014 have no back-references either | **Applied.** Index rows and headers for **0003** (*one argument corrected by ADR-0029*), **0004** (*refined by ADR-0026*), **0009** (*refined by ADR-0030, ADR-0031*) and **0014** (*extended by ADR-0026*), following the file's own convention. `DECISIONS.md`'s HTMX sentence has "virtualized" **struck in place** with a note that the rejection stands on its remaining argument, and `RESEARCH.md`'s copy of the same phrasing carries a dated supersession note rather than being rewritten — the research record is preserved, per this log's own §R1 practice |
| **C-10** | MAJOR | Round-3 R-09 was applied to `ARCHITECTURE.md` and not to `reference/providers.md`, which still lists Calibre-Web as manifest-covered; R-01 has the same shape in `CONFIGURATION.md` | **Applied to both, and the procedural point is taken.** `providers.md` drops Calibre-Web with the full reasoning, adds **Suwayomi** (GraphQL) and **Navidrome** (session establishment) to the not-covered list, and states that "covered" describes what a manifest *could* express rather than how anything ships — the tier does not exist until v0.3. `CONFIGURATION.md` gets R-01's verified Live Data Feed terms. **The disposition procedure now includes grepping the whole `docs/reference/` tree**: two of two spot-checks found a second copy, which is a hit rate that justifies the rule |
| **C-11** | MAJOR | The six-item media-type navigation enum has no defined mapping to the schema, so the sidebar cannot be built | **Applied.** §17.2 defines the enum as six explicit `(kind, formats)` tuples with the "has content" query for each. Both consequences the reviewer names are written down: Ebooks and Audiobooks need an existence query over `edition.format`, which `ix_edition_work` does not serve, so **`ix_edition_format ON edition(format, work_id)` joins migration 0001**; and since the Tier 1 prefix-index payload carries no format, **the Ebooks/Audiobooks split is server-side only in v0.1** and the client's type chips resolve to five values, not six. Stated rather than discovered when the sixth chip does nothing |
| **C-12** | MAJOR | ADR-0030 and §6.1 say the manga/comic difference lives in "the Newznab category (7000 vs 7030)"; §8.5 says there is no manga category and that 7030 returns zero manga | **Applied.** Deleted from both lists, with the reason stated in ADR-0030 rather than silently removed: 7000 is 7030's **parent**, §8.5's own rule uses both for both, and the section cited explicitly denies the distinction. The four remaining items carry the argument. The smaller sibling is fixed too — §16.0 said *"books at `7020` and `3030`"*, merging ebooks and audiobooks; it now reads **ebooks at `7020`, audiobooks at `3030`**, matching ADR-0032 and §8.5 |
| **C-13** | MAJOR | "Plex cannot change a library's type" is asserted as fact in five places; the research marked it forum-only | **Applied to the documents; the three mockup copies assigned to the mockup pass.** ARCHITECTURE §6.5 rule 4 and ADR-0026 now read *⚠️ Plex is **reported** not to allow this, on a community feature request rather than any official statement, unverified against a current build*, and both note that the capability stands on its own — it is the *comparison* that is not evidence. Recorded per the reviewer's aside: the `/library/sections` field-shape claim was correctly never carried into the docs and has not been added |
| **C-14** | MAJOR | `library_scope` closing the existence leak is marked INFERENCE in the research and stated as fact in four places, and no rule covers works belonging to no library | **Applied, both halves.** The 🔍 marker is restored in ARCHITECTURE §6.5 and `search.md` with the research's own words — *should be checked against the first real search query written*. And the unchecked half is real, so it gets an explicit rule: **reserved `library.id = 0`, *Unfiled*** — owner-scoped, never listed, never proposed — upholds a new CI-asserted invariant that **every `search_doc` row has at least one `search_doc_library` row**. Both paths the reviewer names reach the empty state by design (a root-folder library covering part of an instance; an `exclude` against the last library), and both now land in Unfiled. The last question is answered explicitly: **`exclude` removes a work from one library's membership and never from search** — they are not the same mechanism |
| **C-15** | MAJOR | The remaster→same-work claim drops the research's caveat and overstates what MusicBrainz says | **Applied, both problems.** §6.1 now attributes the quote to the **Recording** definition, states that **MusicBrainz defines no "remaster"** and that the step to "therefore a new edition" is UsArr's inference, and carries the dropped caveat: **the common real-world reissue with bonus discs and a changed title gets its own release group**, making it a different work joined by a `work_relation` edge — a different code path from the one the section promised. ADR-0031's edition-keyed rollup carries the same ⚠️, since it rests on the un-caveated reading |
| **C-16** | MAJOR | `library_override` houses global identity corrections in a library-scoped table, and the CI assertion does not cover it | **Applied**, taking the first option. `library_id` is `NOT NULL` for `exclude`/`include` and **`NULL` for `relink` and `field`**, enforced by a `CHECK` rather than by prose, which resolves the disagreement toward ADR-0026's own three-axis table — the table puts display identity and "is this link really this work" in a row that mentions no library at all. The CI assertion now names `library_override` and is stated as two assertions, because the reviewer is right that it is the one library-named table that by design *does* feed identity: the **cascade** references none of the three; the **correction applier**, which runs after it, references `library_override` and nothing else |
| **C-17** | MAJOR | ADR-0031's edition-keyed rollup and §6.1's `total_source` never reach the schema | **Applied.** `schema.md` §1 documents the `availability` blob as **polymorphic with a required `"k"` discriminator** and one worked example per medium — `k:"tier"` for video, `k:"edition"` for music (with `label`), `k:"count"` for comics (with `total_source` and the contiguity `missing` list). Without a discriminator a renderer cannot tell a tier key from an edition key in the same object, which is the gap the finding identifies. Two rules come with it: `k` is required on every non-null blob, and **`total: null` is not `total: 0`** — the first means nobody honestly knows, and §6.3's ✓ rule must not fire on it |
| **C-18** | MAJOR | `work_track.edition_id NOT NULL` silently makes an `edition` row mandatory for every album, and that requirement is stated nowhere | **Applied, all four sub-points.** §6.1 and `schema.md` §1.1 now state that **every album carries exactly one synthetic primary edition from migration 0001**, that it is **resolved rather than re-allocated** (`WHERE work_id = ? AND is_primary = 1`, insert on miss), and the cascade is changed to **`ON DELETE RESTRICT`** so an adapter that re-synthesises cannot destroy the track set. The parent-consistency invariant SQLite cannot express is written as a CI query. And the stale comment is **withdrawn** rather than edited (C-31): under edition scoping `UNIQUE(parent_work_id, disc_number, track_number)` is either redundant with `ux_track_pos` or forbids the exact case ADR-0031 exists for |
| **C-19** | MAJOR | Round-3 R-06's disposition claims more was applied to §8.5 than actually was — `5070` and `7010` are recorded as fixed and are not | **Applied**, taking the first option: §8.5 and `arr-apis.md` now carry a five-row table with an actual **rule** for each special case, not just a list of numbers — `5070 TV/Anime` → `type:anime` in addition to `type:tv` (the parent is right but discards the leaf, and anime routing needs it), `7010 Books/Mags` → `type:magazine` (the parent rule calls a magazine an ebook). Verified against `NewznabStandardCategory.cs`. The reviewer's framing is the reason this is a MAJOR and not bookkeeping: R-06's whole point is that a parent-first rule mislabels a media type, so an unapplied special case is the same bug again |
| **C-20** | MAJOR | The ebook↔audiobook link is a v0.3 roadmap item, a deferred FUTURE item, and has a trigger v0.1 already fires | **Applied** — see also H-12, which is the same defect from the honesty review. §16 is authoritative, so it wins and is restated: **the link is the case v0.3 does *not* solve**, and the resolution pass that would compute it stays in `FUTURE.md` §16 with its cost and seam. `FUTURE.md`'s trigger is rewritten to *"after v0.3, once the Wikidata edge pipeline has proved the confidence/evidence path on real data"*, so it no longer fires a milestone before the roadmap line that claimed the feature. The *"not identified"* companion requirement moves **out** of the deferred entry and into ARCHITECTURE §6.4 as a **v0.1 rule**, because v0.1 ships Komga, which supplies no identifiers at all |
| **C-21** | MINOR | The sidebar row budget contradicts itself: §17.2 says "eleven fixed entries" over a list of eight, and ADR-0027's arithmetic is ambiguous | **Applied.** §17.2 reads **"eight fixed entries (six today)"**; ADR-0027 reads **"8 fixed + ≤6 types + the chip = 15 at full expansion, 13 today"**. The reviewer is right that it matters rather than being pedantry: the pin cap is `16 − fixed − types` and moves by two depending on which number is believed |
| **C-22** | MINOR | "Native checkboxes, so … arrows move" is not how native checkboxes behave | **Applied** in both places. Native checkboxes are **Tab**-traversed, not arrow-navigable (only radios rove), and `Esc` is popover behaviour. Restated as *"Space toggles and Tab traverses for free; arrow-key roving, `Esc`-to-close with focus returned, and close-on-`focusout` are the three behaviours the popover must add"*. The reviewer's Navidrome point is taken and recorded: `LibrarySelector` uses MUI `Checkbox` inside a MUI `Popover`, so it was never evidence for the native claim either |
| **C-23** | MINOR | LazyLibrarian's unmatched-file dict is not "used only for a summary log line" | **Applied** in both places: *"a local dict that produces a debug-log line and an 'N unmatched items' banner, and never a database row"*. The load-bearing half — **no row is written** — is verified and unchanged; it was the flourish that was wrong |
| **C-24** | MINOR | GitLab #2407 is stated as established behaviour in two places while this log marks it unresolved | **Applied.** Both ADR-0026 and §6.5 rule 1 now carry ⚠️ with the reason — no maintainer response, and the reporter says they may be reading the wrong code. The rule is right regardless, and both places now say **why** it is right independently: keying a correction to the upstream's id reproduces the failure *by construction*, whether or not that particular report holds |
| **C-25** | MINOR | `CONFIGURATION.md` still hedges the MusicBrainz Live Data Feed terms that round-3 R-01 verified | **Applied**, with the addition the reviewer asks for: the **free access token for non-commercial users** is an operational prerequisite that neither file mentioned. The 100%-decline behaviour above 1 req/s and the no-polling / random-intervals request are carried across too, since both change the provider scheduler |
| **C-26** | MINOR | §R2.1's rebuttal answers a position the libraries report did not hold, and `include_on_home` is left with no defined meaning | **Applied, both halves — see §4.3 for the corrected rebuttal.** The reviewer is right that `include_on_home` is the proposal's own bound on section count, so "fifteen libraries, fifteen sections" is a strawman; the correct argument is ADR-0028's above-the-fold arithmetic plus the observation that an opt-out list is Jellyfin's answer to a layout that does not scale. **And `include_on_home` is cut rather than given a consumer**: under a three-fixed-block Home nothing reads it — Block A is per media *type*, Block C is unified, the scope comes from the `?lib=` chip — and ADR-0028 already pre-wires a per-type `show_on_home`. Two overlapping flags where one has no reader is how a dead column gets into the one migration that can never be edited |
| **C-27** | MINOR | The Komga delta caveat is dropped in the mockups, where it is stated flatly as UI copy | **Assigned to the mockup pass** (`services.html:490`, `prototype.html:1768`) — add "(pending a live probe)" or a `mockups/README.md` limitation. **The document side got stronger rather than staying level:** the caveat now sits in ARCHITECTURE §7.1a's per-source table, ADR-0032's consequences, **and §16 as a named day-one spike before the schema is written** |
| **C-28** | MINOR | `media_file`'s instance-0 sentinel contradicts "UsArr never touches a filesystem" | **Applied.** The comment is re-worded to what it actually means — *"not reported by any networked service instance"*, whose one current use is the v1.0 Tier 0 Calibre adapter opening `metadata.db` read-only, which §11.2 records as an explicit exception — and it now says outright that it does **not** mean "found by scanning a filesystem", since ADR-0026 refuses that and nothing in any milestone scans |
| **C-29** | MINOR | Deleting a library silently destroys its corrections, and the confirmation copy says the opposite | **Applied**, and C-16 makes it a smaller loss than the finding assumed. §17.8's copy now adds *"It **will** discard the N items you excluded from this library"*, omitted when N is zero. Because `relink` and `field` are now global (C-16), only the library-scoped `exclude`/`include` rows cascade, and the copy says that too |
| **C-30** | MINOR | R-02 cites Last.fm §3.1–3.2 precisely for one claim and leaves *"in our sole discretion"* unsectioned | **Applied** — it is **§4.4**, and R-02's disposition above now says so. Trivial, and the reviewer's reason for raising it is the right one: this file's whole value is that its citations are checkable |
| **C-31** | MINOR | The `work_track` invariant comment was not updated with the table | **Applied** — folded into C-18, which withdraws the comment rather than editing it, because under edition scoping the invariant it asserts is either redundant or wrong |

---

## 4.2 Speed, implementability and accessibility — disposition of all 29

| # | Sev | Finding, in one line | Disposition |
|---|---|---|---|
| **P-01** | **BLOCKER** | `content-visibility: auto` has **no effect on a `<tr>`**, so ADR-0029's default list renderer does not exist for any list in this design | **Applied, and it overturns the mechanism rather than the decision.** The spec text is quoted verbatim in ADR-0029, §4.5 and DESIGN-DIRECTION §7.4 — CSS Containment L2 excludes internal table boxes from size, layout and paint containment — alongside the measurement: 5,000 rows, document height **120,000 px with the declaration and 120,000 px without**, against 140,000 px for a working placeholder, while the same test on `<div>` rows gives the expected 185,000 px. **The fix is that a UsArr list row is not a `<tr>`:** rows become `display: grid` with explicit `role="table"` / `role="row"` / `role="columnheader"` / `role="cell"`, which is what the ≤760 px stacking fork already half builds. The CI assertion the reviewer specifies is adopted — **a contained row's container `scrollHeight` must differ from the uncontained case** — and the accessibility obligation is stated as a **required component test**: an ARIA grid must carry by hand the roles, header association and `aria-rowcount`/`aria-colcount` a native table gives for free |
| **P-02** | **BLOCKER** | The roving-tabindex handler hijacks `ArrowUp`/`ArrowDown`/`Home`/`End` inside `<select>` and text inputs; the Kind select is keyboard-inoperable (WCAG 2.1.1, Level A) | **Assigned to the mockup pass** (`usarr.js:277-296`) with the fix quoted: bail out before the key switch when `ev.target.matches('input, select, textarea, [contenteditable]')`, and drop `Home`/`End` unless the target is the row itself. **The document half is applied:** DESIGN-DIRECTION §11 states the rule and §13 adds it as a `[review]` line, because the reviewer is right that this class of bug is invisible to `svelte-check`. It lands on the control §6.5 rule 4 and §17.8 are built around, which is what makes it a blocker rather than a nit |
| **P-03** | **BLOCKER** | `search_doc.library_scope` is an unindexed JSON `TEXT` column that §8.2 requires be filtered in an index join | **Applied**, exactly as specified. The column is replaced by **`search_doc_library(library_id, doc_rowid)`, `PRIMARY KEY (library_id, doc_rowid) WITHOUT ROWID`**, plus `ix_sdl_doc` for the reverse lookup, and the `EXPLAIN QUERY PLAN` assertion joins §13's CI set: `SEARCH sdl USING PRIMARY KEY` must appear and `SCAN search_doc_library` must not. The finding's sharpest point is recorded in the schema comment — the column defeated the requirement it was written for, and §8.2 states its own reason why that matters |
| **P-04** | MAJOR | D-21 is still open and the prescribed fix, `contain-intrinsic-size: auto var(--row-h)`, is measurably wrong twice over — three times, counting the content-box error | **Applied, all three.** `--row-h` is inert on a table row (measured: forcing it to 100 px leaves the row at 28.0 px), which the P-01 grid primitive also fixes as a side effect; **eighteen distinct row heights across three densities**, mean 42.0 px at compact against a declared 28, so a 25,000-row estimate understates by ~33%; and `contain-intrinsic-size` sizes the **content box**, so a 24 px row with `auto 28px` produced a 37 px placeholder. What ships is `auto <measured content-box height>` per row shape, and **the assertion is drift, not frame time** — `|Δ scrollHeight| / scrollHeight < 2%` at 1k / 5k / 25k rows, both themes, all three densities. DESIGN-DIRECTION §5.3 now says its nine density numbers are floors, not rendered heights |
| **P-05** | MAJOR | The density and theme controls are O(all loaded rows) and blow Tier 0's own 100 ms hard fail at 1,000 rows on a desktop; ADR-0029's benchmark measures the cheap operation | **Applied.** ADR-0029's required `make bench` line is rewritten to measure **density toggle, theme toggle, filter/sort and scroll frame time**, plus drift, with the measured table carried into the ADR: 153 ms at 1k, 1,199 ms at 5k, 6,508 ms at 25k for density; 1,356–4,514 ms at 25k for theme; **0.1–0.3 ms for scrolling at any size**. Both controls are top-bar, pure-local and therefore Tier 0 by the design's own definition. **The DOM-row ceiling claim is fixed accordingly** and the Pi-class extrapolation is stated as 🔍 inference with its arithmetic: **100–300 rows as shipped, 300–600 with `table-layout: fixed` and working containment** — hundreds, not tens of thousands. The three cheap mitigations are recorded rather than deferred |
| **P-06** | MAJOR | The Ctrl+F argument that decides ADR-0029 is defeated by the "Load more" pagination shipped in the same sentence, and UsArr already ships a better mechanism | **Applied as the reviewer scopes it — the argument is demoted, the decision is not reversed.** Ctrl+F searches only loaded rows and reports *not found* rather than *partial*, which on a 50,000-track library is roughly 300 of 50,000; and Tier 1, the client prefix index over the whole top-level corpus at < 5 ms p50, beats it on coverage and ranking. ADR-0029 now stands on the grounds that hold — a keyset window keeps the mounted set small either way, and virtualization is a dependency and a scroll-restoration liability — with Ctrl+F kept as a secondary benefit and the other cited losses listed as costs rather than as decisive. The honest threshold from P-05 is recorded in the same place |
| **P-07** | MAJOR | D-12 fixed the forced reflow and left the O(n): the roving tabindex is 55 ms per arrow key at 25,000 rows, and the comment says the problem is solved | **Assigned to the mockup pass** (`usarr.js:265-298`) with the sibling-walk fix quoted and the note that P-02, P-07 and P-20 are the same 34-line block. **The document half is applied:** DESIGN-DIRECTION §11 now carries the measurements (1.18 ms at 1k, 9.69 ms at 5k, 55.12 ms at 25k; 2.25 s of main thread for one second of key repeat) and the rule — an arrow key needs the adjacent row, and the full scan is kept only for `Home`/`End`, which do not repeat |
| **P-08** | MAJOR | The row-selection counter is a full-document `querySelectorAll(':checked')` on every toggle: 32 ms per checkbox at 25,000 rows, O(n²) for a range | **Assigned to the mockup pass** (`usarr.js:361-382`). **Document half applied** in the same DESIGN-DIRECTION §11 paragraph, with the fix named (keep an integer, recount only on rebuild) and the reviewer's point that `requests.html` is the bulk-grab screen, so this is a real interaction rather than a hypothetical |
| **P-09** | MAJOR | §13's reference library is still two media types, so every budget is a two-type budget — and the client prefix index's 25,000 cap becomes the *expected* case | **Applied, all three parts.** §13 now carries a **six-type reference library** in a table: 10k movies, 2k series, 1.5k artists, 5k albums, 6k books, 3k comics = **27,500 top-level works** over ~880,000 `work` rows, with the non-video counts marked 🔍 as chosen rather than measured and floored by the mockups' own 9,411. **The cap is resolved explicitly rather than left implicit:** it is not raised on a guess, and it is not left as "ship no index" — §4.5 specifies a **partial index** (everything in the currently scoped libraries, plus top-N by popularity to fill the budget) with the UI saying so and Tier 2 one keystroke away, and states that **the cap is a byte budget of which 25,000 is the current row figure**, to be re-derived when the six-type payload is measured rather than estimated. New §13 rows cover each source's full import and sweep |
| **P-10** | MAJOR | §7 does not mention Navidrome, Audiobookshelf, Komga or Kavita once; four of six v0.1 media types have no delta channel in the document that defines freshness | **Applied** — see H-02, the same finding from the honesty review. New **§7.1a, channel 3b**, with the watermark (max observed upstream `lastModified`, never `now()`), the ordering guarantee, the derived overlap window, a page-walk stability rule, the structural fact that a page walk **cannot observe a deletion**, and the fallback when the guarantee is absent. Budget rows added to §13. **§17.8 gains a named *no change feed* state** and §17.3 a labelling rule, because the reviewer is right that "no number" and "a number from four hours ago" read identically otherwise. Kavita's total absence of a catalogue sort is recorded, and is one of the three reasons it pays for the amendment (H-01) |
| **P-11** | MAJOR | Five single-character shortcuts with no turn-off, no remap and no focus scoping — WCAG 2.2 SC 2.1.4, Level A — and 2.1.4 is missing from §11's floors table | **Applied.** SC 2.1.4 joins the floors table with the criterion's own three routes quoted, and §11 states the resolution: **a Settings "Keyboard shortcuts" toggle, on by default, satisfies "turn off" for all five at once**; the guard becomes `t.isContentEditable || t.closest('input, select, textarea, [contenteditable]')`; `?` stays unconditionally because the sheet is where the toggle is discovered. §13 gains a `[grep]` line. The reviewer's correction of the code comment's reasoning is adopted verbatim in effect — **a visible mouse equivalent addresses discoverability, and 2.1.4 is not about discoverability**. The `usarr.js` guard change is **assigned to the mockup pass** |
| **P-12** | MAJOR | The scope chip is silent to a screen reader about the thing it exists to be loud about, and tabbing past it leaves the popover open over the nav | **Assigned to the mockup pass** — `aria-live="polite"` on `[data-slot="scope-label"]`, and close on `focusout` when `relatedTarget` is outside `.scope`; both one line. **Document half applied:** §13 gains a `[review]` line requiring a live region on any control that changes a visible summary string, and C-22's correction covers the `Esc`/roving half. Recorded because it is the design's headline claim failing — *"the chip always states the current scope in words, so a control that hides content can never be silent about what it hid"* — and five of seven measured behaviours passed, including the indeterminate announcement and target sizes |
| **P-13** | MAJOR | The poster card's title and year sit on a runtime-computed colour with no contrast rule anywhere, and the mockup's own data already fails SC 1.4.3 | **Applied.** The rule is in DESIGN-DIRECTION §11 and ARCHITECTURE §4.4.1: **pick whichever theme text token scores higher against the computed `dominant_color`; if the winner is still below 4.5:1, move `dominant_color`'s lightness away from the text in 2% OKLCh steps until it clears — the fill is decoration, the title is content.** Two supporting rules make the ratio computable at all: **no `opacity` on either line** (it costs ~0.45 by compositing, through a mechanism no contrast check sees) and **12 px semibold is normal text, not large**. Asserted in CI over any `--dc`/`--dc-fg` pair in a fixture, and in the image pipeline where the colour is produced. The reviewer's framing is why this is MAJOR: one bad swatch would be a nit, **the finding is that there was no rule** and the colour is data. The failing swatch itself is **assigned to the mockup pass** |
| **P-14** | MAJOR | Search shows 12–13 items above the fold against the ≥25 rule, and the cause is fixable: six groups carry six different column sets in one column, so 12 of 13 rows wrap | **Applied to the documents; the redraw assigned to the mockup pass.** §17.4 gains rule 5: **the library column renders only when it varies *within its group*** — the old test was "≥2 libraries", which does not fire where it matters, and four of six groups carry one repeated string costing ~120 px. DESIGN-DIRECTION §9.1 generalises it (a column that cannot vary is not data — D-05 again, in a new place) and adds declared column widths per group. **The reviewer's third sub-point is accepted against the document, not the mockup:** §17.4's *"every result row is one template"* was false beside a README that says the columns differ, so the claim is narrowed to what actually holds — **slot order and slot meaning are identical across groups**, and the column sets are six by design |
| **P-15** | MAJOR | §6.5 points at DDL that does not exist, so the one performance risk the design flags cannot be measured or asserted | **Applied** — the same gap as C-01, and the reviewer's framing is the sharper one: `library_member` had no columns, no primary key and no index anywhere in the repository, so the `EXPLAIN QUERY PLAN` + row-count assertion §6.5 promises was un-writable. It is writable now, and `schema.md` §13.5 lists the six assertions the four tables carry |
| **P-16** | MAJOR | The 8 ms budget for the library-scoped grid rests on a "common case" the design's own worked example already breaks | **Applied, taking the reviewer's recommendation to make the fallback the default.** §6.5's mitigation was *"the default topology is one library per kind"* — and the flagship install has **seven libraries over six kinds**, including the two `book` libraries the feature exists to demonstrate. So `library_member` is keyed `(library_id, sort_title, work_id, edition_id)` `WITHOUT ROWID`, making the scoped keyset a single covered seek at any selectivity, and **CI asserts the plan for both topologies, because only the two-libraries-per-kind case is interesting**. The write cost is stated rather than waved at: one column on an already-materialised table, plus the rule that a title or sort-title change rewrites its member rows |
| **P-17** | MAJOR | D-20 was never applied; Tier 1's "render the shell, the nav, the headers" is unachievable under `adapter-static`, and six types made the shell bigger | **Recorded, escalated a second time, and the caveat is now in the document rather than only in this log.** DESIGN-DIRECTION §7.2 Tier 1 carries a ⚠️ stating plainly that on a cold boot the shell *is* the JavaScript, that six types increased the static chrome the user could have been looking at while the blank window did not change, and that the fix — inlining the static shell into the built fallback `index.html` — is a build-step change before the first UI commit and an expensive retrofit after. **It stays §D3 item 6, open for Joe**, because it is a build-config decision under ADR-0025 that this log may not take; what changes is that a reader of §7.2 now sees the limit instead of an aspiration |
| **P-18** | MAJOR | On a phone, Block A costs ~105 px per media type, and "Needs attention" starts 914 px down an 844 px viewport | **Applied.** §17.2 carries the measurements and the rule: **below 760 px, Block A is a two-line row** (name and count, then availability and sync time) with **no `Type` label**, since the value *is* the row's identity, **and Block B moves above it** — Block B is hidden when empty, so it costs nothing when nothing is wrong, which is exactly why it can go first. The reviewer's framing is the reason this is not cosmetic: §17.1 singles out the phone because *"that is where a request gets made from the sofa"*, and scrolling a screenful of counts before learning Prowlarr rejected the API key is the wrong order |
| **P-19** | MAJOR | Sticky column headers do not stick between 761 px and 1,099 px — on the screen that has six different column sets | **Applied to the documents; the CSS assigned to the mockup pass.** DESIGN-DIRECTION §9.1 now states the mechanism (`overflow-x: auto` with `overflow-y: visible` computes to `auto` on both axes, so the wrapper becomes a scroll container that never scrolls vertically and the sticky header sticks to *it*), the measurement (one header pinned at 1440 px, zero at 1000 px), the exact broken band, and both fixes. Recorded as a rule — **a sticky header is tested at every breakpoint, not just the widest** — because the documents never said headers were lost there |
| **P-20** | MAJOR | The roving tabindex makes the libraries table 8 tab stops, not 1, and a "Load more" append breaks the invariant in the other direction | **Assigned to the mockup pass; the two rules applied to DESIGN-DIRECTION §11.** (a) The row is the *only* stop — everything inside it is `tabindex="-1"`, reached by `Enter`/`Space` or a per-row menu — because adding the row *on top of* the existing links makes seven rows eight stops. (b) **The assignment must be idempotent and run after every append**, since ADR-0029 makes appending the primary interaction: a cloned row inherits `tabindex="0"` and becomes an extra stop, a templated row gets none and `focus()` silently does nothing, which is an arrow key that looks dead |
| **P-21** | MINOR | `.tbl tbody tr { min-height: var(--row-h) }` is inert; `--row-h` is a dead token three documents build on | **Applied to the documents; the CSS assigned to the mockup pass.** DESIGN-DIRECTION §5.3 states that the nine numbers are floors, that the `min-height` must sit on an element it applies to, and that on a `display: table-row` it is inert with the measurement — which is one more reason §7.4's primitive is a grid row, where `min-height` does apply |
| **P-22** | MINOR | `table-layout: fixed` is never set — free performance and a fix for P-14's wrapping | **Applied to the documents; the CSS assigned to the mockup pass.** DESIGN-DIRECTION §9.1 now requires declared column widths per group, with the reviewer's reasoning (auto layout must measure every cell in every row — the one layout mode inherently O(all rows), which no containment helps) and the measurements: 1,199 → 547 ms at 5k, 6,508 → 2,956 ms at 25k. It is also listed among ADR-0029's three cheap mitigations |
| **P-23** | MINOR | The scope chip is unreachable on a phone without opening the drawer, and `l` there focuses a `display: none` element | **Assigned to the mockup pass** (`usarr.js:136`) — guard the handler on the popover being reachable, or open the drawer first. A shortcut that appears broken rather than absent is the worst of the three states |
| **P-24** | MINOR | `paintScope()` derives the URL from label text, so renaming a library changes its permalink | **Applied to the documents; the mockup change assigned.** `library.slug` is in the DDL with the rule written into the column comment: **allocated once from the name at creation and then durable, because renaming must not change the permalink** — §7.3 rule 5 makes the URL the durable state and §17.8 makes the name editable, so deriving one from the other is a contradiction |
| **P-25** | MINOR | The mockup README undercounts posters mode: 20 cards **plus 11 rows** = 31 items | **Assigned to the mockup pass.** Recorded with the reviewer's point that this is the one configuration comfortably clearing the ≥25 rule and the table hides it |
| **P-26** | MINOR | Six sticky `th` all target `top: 40px` at `z-index: 10`; the later group's header paints over the earlier one during the overlap | **Assigned to the mockup pass.** Standard behaviour, but with six *different* column sets the transient shows the wrong column names over rows still being read — give each group's header a stacking context, or pin the group heading with it so the pair moves together |
| **P-27** | MINOR | `.fanout__count` is Tier 3 determinate progress with no live region | **Assigned to the mockup pass** (`aria-live="polite"` on the count, `role="status"` on the bar). **Covered by the document rule added for P-12** |
| **P-28** | MINOR | At ≤760 px `thead { display: none }` removes `columnheader` from the accessibility tree | **Applied**, and P-01 changes its shape. The reviewer verified it is survivable today because Chromium exposes the `::before` generated content, and flags that it is load-bearing and undocumented. Under the ARIA-grid primitive the header row is `role="row"` of `role="columnheader"` cells rather than a `<thead>`, so **"column names must survive the stacked view where the header row is not rendered" is now an explicit clause of the required component test** in §7.4 and §13, rather than an undocumented dependency on generated-content exposure |
| **P-29** | MINOR | The prototype's one new control demonstrates nothing about its own cost; §13 has no budget row for a scope toggle | **Applied.** §13 gains the row: **1 keyset page + 6 sidebar `COUNT(*)` over `library_member ⋈ work` per toggle**, p50 < 15 ms. The reviewer's honest answer is recorded rather than argued with — the scope chip really is O(libraries) and the type summary really is six static rows; what the prototype cannot show is the production interaction, which changes the URL and re-runs both the grid query and the counts |

---

## 4.3 Honesty, scope discipline and degradation — disposition of all 30

| # | Sev | Finding, in one line | Disposition |
|---|---|---|---|
| **H-01** | **BLOCKER** | "Cut before you add" is not satisfied, and ADR-0032 says so in its own words | **Applied. The payment is Kavita, moved to v0.2**, and the reviewer's central point is conceded without qualification: the one "cap" was the correction UI, and §16.0 argues in the next sentence that there was nothing to correct — **a cap on a declared no-op is not a payment**. Kavita earns the cut on evidence already in the research: its identifier fields are behind a paid subscription, so it contributes the least identity value and the most honest-gap UI per adapter; its Series→Volume→Chapter→File hierarchy is the deepest and the one §6.1 already flattens; and it is the only source with **no catalogue delta at all**. **No media type is lost** — Komga covers comics, so all six still have a v0.1 catalogue source. The libraries subsystem and the auto-proposal flow **stay**, because they are what makes the six-type claim usable, and ADR-0032 and §16.0 now **state the remaining cost plainly** — four tables, materialised membership with a denormalised sort key, five container predicates, a proposal engine and a second settings screen — rather than arguing it away. The reviewer's own preferred cut (Navidrome as well) is recorded as a **rejected alternative with its reason**: it costs a media type and Kavita does not |
| **H-02** | **BLOCKER** | "All four are the same shape … no new subsystem" is false, and §7 was never amended | **Applied, all five recommended actions.** (1) **New §7.1a, channel 3b** — an ordered page-walk delta with its watermark, ordering guarantee, overlap window, page-walk stability rule and the reconciliation-only fallback *said out loud on the Services row*. (2) §16 now reads **channels 1, 3, 3b and 4**, with channel 3 scoped to Sonarr and Radarr. (3) **The Komga `sort=lastModified,desc` probe is a day-one spike in §16**, alongside the arm64 RSS spike, with "Komga drops to reconciliation-only" as the stated failure branch. (4) **§11.2 versus §16 is resolved toward Tier 0**: the manifest tier does not exist until v0.3, Navidrome is excluded from it by §11.2's own session-establishment rule, and §11.2 now says outright that these are hand-written Go adapters in the milestone they ship in — which feeds H-01's re-pricing. (5) §16.0's "same shape" sentence is replaced with *three Tier 0 Go adapters, three auth schemes, one token lifecycle, three hierarchies, and one new delta channel they all share*, with the genuinely-shared half named so the honest version is not just the pessimistic one. Kavita's total absence of a catalogue sort — which ADR-0032 did not say — is recorded, and is one of the three reasons it pays for the amendment |
| **H-03** | **BLOCKER** | The "v0.1 screen mockups" are drawn against a stack §16 says v0.1 cannot have | **Assigned to the mockup pass**, with the four recommended actions carried verbatim: remove Lidarr and LazyLibrarian from the service list, library destinations, search panel and home attention rows; redraw Music and Ebooks with `Request destination: none` following the Audiobooks row's existing correct copy; replace the two now-unreachable "honest states" with the two v0.1 actually produces (H-14); and draw the Lidarr `status: deleted` guard and the LazyLibrarian `Success:false` shape in a separately-labelled v1.0 annex, because they *are* worth drawing. **The document side needs no change and is already correct** — §16 excludes both — which is exactly the reviewer's point: the screens contradict the roadmap, not each other. Note for that pass: under H-01, **four of six types have no sink in v0.1** (music, audiobooks, ebooks, comics), which ADR-0032's consequences now state |
| **H-04** | **BLOCKER** | `prototype.html` is built for publication and carries no in-page statement that it is fabricated | **Assigned to the mockup pass** for the artefact (a persistent non-dismissible strip on every page, and the same line emitted by `build_prototype.py` so a regenerate cannot drop it). **The document half is applied**: DESIGN-DIRECTION §9.6's absolute *"Never fabricated data — not in an empty state, not in a screenshot, not in documentation"* now carries the same scoped form §13 already had, **and states why the sibling README is not sufficient** — the single-file build is detached from it by design and is the artefact most likely to reach an outside reader. The two statements of one rule had drifted; they now say the same thing |
| **H-05** | MAJOR | Two first-run empty states tell the user that adding Lidarr will give them a library | **Assigned to the mockup pass.** §8.5's canonical string is *"Add a Sonarr, Radarr or media server to get a library"*; the mockup expanded it and introduced the error. Under §16 Lidarr is a sink whose catalogue is never bound as a source, **in any milestone**, so adding it gives you a library in none of them. Suggested replacement recorded: *"Add a Sonarr, Radarr, Navidrome, Audiobookshelf or Komga to get a library."* — note Kavita drops out under H-01 |
| **H-06** | MAJOR | "Nothing in this ecosystem accepts a comic request over an API" is false by the project's own ADR | **Assigned to the mockup pass**, with the reviewer's rewrite adopted as the target copy. The finding is exactly right about the failure class: **an overclaimed impossibility is the same honesty failure as an overclaimed capability**, because it tells the user to stop looking for something that exists. §8.5 already states the true and narrower sentence — Mylar3 needs the indexer configured inside Mylar3 and operates on issues it knows from Comic Vine; Kapowarr has no indexer concept — so the copy should say *they exist and neither closes the loop from a Prowlarr grab* |
| **H-07** | MAJOR | The Kavita identity numbers are mutually incoherent across three surfaces | **Assigned to the mockup pass**, with the reviewer's suggested arithmetic recorded as the target. The finding's framing is the reason it is MAJOR: this is the one state in the prototype whose entire purpose is counting honestly, and a reviewer who checks the arithmetic — exactly who the state is for — loses confidence in every other figure on the screen. Note for that pass: under H-01 Kavita is a **v0.2** source, so the row may need a milestone marker as well as correct numbers |
| **H-08** | MAJOR | The correction surface is drawn in full with no in-screen deferral marker | **Assigned to the mockup pass** (a `v0.3` chip beside the Corrections heading plus the one-sentence note the reviewer drafts, using the cross-media Dune row as the model). The reviewer's second point is the sharper one and it is recorded in ADR-0032: drawing the capped thing as shipped removed the only visible evidence that anything was capped — which mattered more when the cap was the claimed payment, and matters less now that H-01 supplies a real one |
| **H-09** | MAJOR | §16 is declared authoritative for scope but omits four schema areas v0.1 must ship | **Applied.** §16's v0.1 schema line now enumerates the ADR-0030 and ADR-0031 changes explicitly — `comic_issue` with the `work_comic`/`work_comic_issue` split and its `kind_byte`, `work_track.edition_id`, `track_number TEXT` plus the derived `track_position`, the M:N `work_credit`, and `edition.narrators`/`duration_seconds`/`abridged` — under the heading that they are **migration 0001 or a backfill over the largest tables in the schema**. README gains the corresponding v0.1 row. The reviewer is right about the mechanism: `CLAUDE.md` says if §16 does not say a thing ships, it does not ship, and the README's tables are generated from §16, so the omission propagated |
| **H-10** | MAJOR | `RESEARCH.md` was not amended, so the evidence base for the largest scope change is absent | **Applied.** New **Track 06** carries the six-type findings with every ✅/⚠️/🔍 marker from the five research passes, in four sections: catalogue sources, acquisition and indexers, modelling, and the classical-music impossibility (H-18). Every fact the reviewer lists as load-bearing-but-absent is in it. **Both stale rows are cleared**, with the correction dated rather than the record rewritten: Kavita's API surface *was* retrieved (the full `openapi.json`), so *"Komga's is verified, Kavita's is not"* is superseded and the unverified-list row is struck. The reviewer's framing of why this matters is adopted: until this landed, ADR-0032's consequences were assertions in an ADR rather than citations in the evidence base |
| **H-11** | MAJOR | ADR-0032 is Accepted while its load-bearing premise is an open question escalated to the owner | **Applied as the reviewer's second option, with the first rebutted — see §4.4.** A fifth consequence is added, in substance as drafted: the *"convenience, not capability"* claim is weakest for music and books, 403 of 543 Prowlarr definitions are `type: private`, the dedicated music and book trackers are invite-only, and **for a user without those invites, deferring Lidarr defers capability**. Open: R4 item 3. The same qualification is added to ARCHITECTURE §8.5 and README (H-19) |
| **H-12** | MAJOR | The ebook↔audiobook link is simultaneously in the roadmap and in `FUTURE.md`, and its deferral trigger already fired | **Applied** — the same defect as C-20, from the other side. All three of the reviewer's actions are taken: §16's v0.3 line is replaced with the honest form (*the case v0.3 does **not** solve*), `FUTURE.md` §16's trigger is rewritten to *after v0.3, once the Wikidata edge pipeline has proved the confidence/evidence path*, and the *"not identified"* companion requirement moves **out** of the deferred entry into ARCHITECTURE §6.4 as a v0.1 rule. The reviewer's reason for that last move is decisive and is quoted into §6.4: **v0.1 ships Komga, which supplies no identifiers at all, so the state is reachable on day one** |
| **H-13** | MAJOR | Two `FUTURE.md` seams were invalidated by ADR-0031 and were not updated | **Applied.** `playback_state` becomes `(user_id, work_id, edition_id)` and `play_history` `UNIQUE (user_id, work_id, edition_id, started_at)`, in both `FUTURE.md` §9/§10 and `reference/schema.md`'s appendix. The reviewer's argument is exactly `CLAUDE.md`'s own rule turned on the document: an audiobook is an `edition` of a `book` work, so a work-keyed position **cannot represent "40% through the ebook, 12% through the audiobook"** — which is the entire content of the deferred feature §10 describes. **A seam that cannot express its feature is not a seam**, and it had been recorded as one |
| **H-14** | MAJOR | Four of §17.8's seven named library states are unreachable, including "the replica principle's demo" | **Assigned to the mockup pass**, with *all sources down* and *sources healthy, zero items* named as the minimum, per the reviewer, and the note that redrawing Music and Ebooks under H-03 frees two slots. **One state was added to the document rather than only to the drawing list:** §17.8 gains **no change feed** as an eighth named state (P-10), which did not exist when this finding was written and which is the steady state for the sources on channel 3b's fallback |
| **H-15** | MAJOR | §8.5 requires the UI to say Prowlarr search is free text; the UI never says it | **Assigned to the mockup pass**, with the reviewer's persistent line adopted as the target copy. **The requirement itself is already correct in §8.5** and needs no change — the finding is that the screen does not carry it, and that the only related copy appears in the `empty` state, i.e. **only to users who have already failed**. That framing is the reason it is MAJOR: it is the highest-value honest-impossibility statement on the screen and it is shown last |
| **H-16** | MAJOR | Search shows an item sourced from Kavita inside the library that has no sources | **Assigned to the mockup pass.** The reviewer's second option is recorded as preferred and the reason with it: keeping the row in Ongoing comics with an honest orphan rendering — `Komga "garage" (removed)`, muted, with the tombstone sub-line — is more valuable than moving it, because **no other row in the prototype shows what an orphaned item's provenance looks like**, and this is the state demonstrating the project's most opinionated data decision |
| **H-17** | MAJOR | Home's stale banner reports a freshness time that Services contradicts, overstating freshness by 2h 15m | **The rule is applied to §17.7; the mockup fix is assigned.** §17.7 now states: **the timestamp in a degraded banner is that instance's own last successful sync, never the global delta time** — with the reason, because the number is the banner's whole job and a reassuring wrong one is worse than none. It also covers the case that did not exist when the finding was written: an instance with **no delta channel** quotes its last full compare and says so. The reviewer's diagnosis of the cause is recorded — §17.7's example string was retargeted to a different instance without retargeting its number, which is exactly the bug that gets copied from a mockup into an implementation |
| **H-18** | MAJOR | The classical-music impossibility is verified in research, recommended for the UI, and appears nowhere | **Applied**, and the reviewer is right that it was the only one of the five honest-impossibility items with no treatment anywhere. `RESEARCH.md` **Track 06 §6.4** carries the finding with its ✅ marker and the recommendation. **ARCHITECTURE §6.1 states it as a rule**: UsArr models no composition tier, neither Navidrome nor Lidarr models a composition, a composer therefore renders as an artist, recordings of one work group by release rather than by work, and **where that is visible UsArr says so** — in the same register as the "not identified" badge and the comics gap list. The reviewer's sharpest point is adopted verbatim in effect: **`work_credit(role)` is the seam, not the disclosure, and ADR-0031 mentioning it as making classical "representable" reads as the opposite of the finding.** The mockup row is **assigned to the mockup pass** |
| **H-19** | MAJOR | The invite-only reality of music and book indexers is surfaced nowhere, while the sample data assumes it away | **Applied** in all three places the reviewer names and one more. README's v0.1 Search-and-Grab row, ARCHITECTURE §8.5's per-type caveat paragraph, and ADR-0032's consequences all now carry it: **403 of 543 Prowlarr definitions are `type: private`**, the dedicated music and book trackers are invite-only, only three of 543 are comic-specific, and **GetComics is not a Prowlarr indexer at all**. The reviewer's framing is preserved — this is not a request to make the sample data less realistic; it is that the claim built on top of it was never qualified anywhere the owner or user would read it. R4 item 3 stays open (H-11) |
| **H-20** | MINOR | README says "24 ADRs"; there are 33 | **Applied**, taking the second option — **the count is dropped**, not corrected, so it cannot go stale again. It is the file most likely to be read first |
| **H-21** | MINOR | README's deferred list omits all seven new `FUTURE.md` entries | **Applied.** slskd · Suwayomi · OPDS 2.0 · Chaptarr · MangaBaka · the ebook↔audiobook identity pass · per-library OPDS feeds are appended, since README states that deferred ideas live in `FUTURE.md` and the summary should track it |
| **H-22** | MINOR | The mockups README's own state count contradicts its breakdown (28 vs 6+5+4+7+9 = 31) | **Assigned to the mockup pass.** The reviewer's reason for raising a MINOR arithmetic slip is the right one and is recorded: in a document whose value rests on precise counting, this is the wrong error to have |
| **H-23** | MINOR | README's lead paragraph is present-tense over services in three different milestones | **Applied**, and rewritten rather than split, because H-01 changed the v0.1 set: it now names **Sonarr, Radarr, Prowlarr, Navidrome, Audiobookshelf and Komga for v0.1**, Kavita for v0.2, and Lidarr, LazyLibrarian, Jellyfin and the post-Readarr tools for later milestones, pointing at §16 as authoritative. The pre-alpha banner covers "does not exist yet" and did not cover "these are in the last milestone" |
| **H-24** | MINOR | The mockups README quotes a library slug the mockup does not emit | **Assigned to the mockup pass** (`comics,ongoing-comics` in the README versus `comics,comics-ongoing` in the mockup — make the README match). Related and applied on the document side: P-24 puts the durable-slug rule in the DDL, so neither should be derived from a label |
| **H-25** | MINOR | Home's "Items" column silently mixes units across media types | **Applied to the document; the mockup change assigned.** §17.2's Block A now requires **each count to name its unit in the cell**, with the reviewer's own example: `Music 612` reads as smaller than `Movies 1,204` when it is 4,118 albums and 51,204 tracks. ADR-0031's *"two artist-level numbers must never be rendered bare"* is cited as the principle this generalises — a mixed-unit column labels its unit or it is misinformation. The sidebar counts follow the same rule |
| **H-26** | MINOR | §8.5's required grab-confirmation copy appears in no state | **Assigned to the mockup pass.** §8.5 already specifies the sentence literally — *"Sent to \<download client\>. UsArr does not import downloads."*, naming the watched folder when a library-bearing service is configured — so this is the artefact not carrying a requirement the document already states, and the `nosink` state's ABS line already implies the folder without naming it |
| **H-27** | MINOR | The `permission-denied` state is required "from day one" and is drawn nowhere | **Applied to the document**, taking the reviewer's second option and sharpening it. §10's row now says what "exists from day one" actually means in a single-user product: **the *behaviour* exists — §14 rule 6 plus §1.3's access-scope parameter mean an unauthorized item is *absent* from the response** — so the honest rendering is the ordinary `empty`/`filtered-empty` state and **there is nothing distinct to draw**. A visibly *denied* surface is a v1.0 screen arriving with `user_library_access`. The reviewer's observation that Services' `denied` is a sudo re-auth prompt, i.e. a different thing, is recorded in the same row so the two are not conflated again |
| **H-28** | MINOR | `FUTURE.md` §8's calendar description was not updated for six types | **Applied**, with the reviewer's note adopted in substance: **only the \*Arrs expose `/calendar`**, Lidarr is v1.0, and v0.1's book and comic sources expose no calendar endpoint at all, so a catalogue-only source contributes `work.release_date` where it has one and nothing where it does not — and the calendar must be honest about which types it can cover rather than rendering empty lanes. The **seam** (`work.release_date`) was never in question; the **promise** needed narrowing |
| **H-29** | MINOR | `CLAUDE.md`'s "Where things live" table omits `docs/design/` | **Recorded, not applied — and the reason is a constraint on this pass rather than a disagreement.** The finding is correct: `docs/design/DESIGN-DIRECTION.md`, `tokens.css` and the mockups are normative for UI work and are cited by §17, and an agent following `CLAUDE.md` literally will not find them. **This pass may not edit `CLAUDE.md`**, so the change is escalated to the owner with the exact row to paste: <br>`| `docs/design/` | The visual system: DESIGN-DIRECTION.md, tokens.css (canonical values) and the v0.1 mockups. §17 stays authoritative over all three. |` <br>**Partially mitigated meanwhile:** README's documentation table gains the equivalent row, so the pointer exists somewhere a reader will hit |
| **H-30** | MINOR | The Services mockup renders a delta channel no document specifies | **The document half is applied; the labelling is assigned to the mockup pass.** §7 now specifies channel 3b (H-02), so the mechanism exists — and §17.3 adds the labelling rule the reviewer asks for: **a source on channel 3b is labelled `page-walk delta`, and one with no ordering guarantee reads `no change feed — full compare at 09:12`** rather than printing a bare `delta HH:MM` at the same visual weight as Sonarr's real `/history/since`. §7.1a's per-source table gives each mechanism and watermark, which is what the mockup expanders should quote |

---

## 4.4 Rebuttals and partial disagreements

Recorded because a finding that is quietly ignored comes back. **Exactly one finding is rebutted in
part; nothing is rebutted in whole.**

### 4.4.1 H-11's first option — "mark ADR-0032 **Proposed — pending R4 item 3**" — is rebutted. Its second option is applied in full.

**What the finding gets right, and it is why the second option is adopted without argument:** the
ADR's *"deferring them defers convenience, not capability"* is its load-bearing sentence, this log
had already recorded that the claim is materially weaker for music, and the ADR was marked Accepted
with that gap unlisted among its "four honest gaps". That is the honesty machinery producing a
finding and the decision walking past it, and the fifth consequence now says so in the ADR's own
voice (H-19, §8.5, README).

**Why the status does not change.** Reverting an accepted ADR to *Proposed* would say that the
**decision** is unsettled, and it is not — the decision is *which services land in which milestone*,
and the open question is a **positioning claim about how good the remaining story is for two of six
media types**. Those are different objects, and conflating them has two costs. It would make every
document generated from §16 — the README tables, `FUTURE.md`'s triggers, `DESIGN-DIRECTION.md`
§12 — provisional against a question whose answer changes none of them: **the honest answer to R4
item 3 is either "yes, it is an acceptable v0.1 story" or "no, and therefore Lidarr moves earlier"
— and the second is a *new* decision, not a retraction of this one.** And a `Proposed` status here
would be the only one in the file, against thirty-two Accepted ADRs several of which carry unresolved
⚠️ items in their consequences (ADR-0001's RSS budget, ADR-0016's tsnet specifics, v0.4's entire
success criterion resting on Symfonium's unverified `apiKeyAuthentication`). **This file's
convention is that an accepted decision carries its open questions as marked consequences**, and
applying a different convention to this one ADR would misrepresent it as shakier than its
neighbours rather than as honest about the same thing they are.

So: the consequence is added, the ADR stays Accepted, and R4 item 3 stays open and is now
cross-referenced from the ADR itself rather than only from this log.

### 4.4.2 The corrected §R2.1 rebuttal (C-26), rewritten here rather than edited above

§R2.1's second paragraph argued that *"a user with fifteen libraries would get fifteen home
sections"*. **That is a strawman of the proposal it rejects**, because `research-libraries.md` §11
proposes *"one section per enabled library **with `include_on_home`**"* — the flag is an explicit
per-library opt-in and is the proposal's own bound on section count. The reviewer is right, and the
resolution is unaffected, because the argument that actually decides it was already available and
was not cited:

1. **ADR-0028's above-the-fold arithmetic**, which kills strips regardless of how many render: at
   154 px poster width a ~1,200 px column fits ~8 cards, a card plus its meta line is ~260 px, and a
   section with its header is ~300 px, so a 900 px viewport minus the toolbar shows **2.8 sections
   and ~16 items** against the design's own 25-item floor — on the screen whose entire job is
   inventory. It fails before any citation is consulted, and it fails at *one* section per library
   just as it fails at fifteen.
2. **An opt-out list is Jellyfin's answer, and it is seven checkboxes in user settings compensating
   for a layout that does not scale.** Requiring configuration to make a *default* layout work is
   the defect, not the fix — so `include_on_home` bounding the count is not a defence of the
   proposal, it is a symptom of it.

The cardinality argument (a media type is a closed enum of six; a library is user-defined and
unbounded) still holds and is still worth keeping, but it is the *second* reason, not the first.
**And the consequence C-26 identifies is accepted rather than argued with:** `include_on_home` has
no consumer under a three-fixed-block Home, so it is **cut from the `library` table** rather than
carried into migration 0001 as a column nothing reads.

---

## 4.5 What this round found that the reviewers did not

Two things surfaced while applying the findings and are recorded because they are the same class of
defect the reviews exist to catch.

1. **`work_credit.artist_work_id` points at a `work` of kind `artist`, and `work.kind` has no
   `person`, `author` or `creator` member.** ADR-0031 requires `work_credit` for books and comics
   explicitly — *"it is needed for books too, where role matters: author, translator, editor,
   illustrator"* — so a book's author and a comic's writer are stored as `artist`-kind works. Two
   visible consequences: an author appears under the **Music** navigation type unless filtered out
   (§17.2's enum maps `artist` to Music), and the Tier 1 prefix index counts every author and
   illustrator as a top-level work, which pushes against the 25,000 cap P-09 already shows is
   tripped. **The alternative is a `person` kind, which is a `kind_byte` allocation and a
   CHECK-constraint change — precisely the "migration 0001 or never" class ADR-0030 argues.**
   Recorded in `schema.md` §1.1 with its cost and **left unresolved**, because it is a schema
   decision of the same size as ADR-0030 and it should be made deliberately rather than folded into
   a review pass.
2. **`library_source.container_kind = 'tag'` cannot be a seek.** The predicate is
   `EXISTS (SELECT 1 FROM json_each(remote_tag_ids) WHERE value = ?)`, and a `json_each` over a
   per-row JSON array cannot use an index. It is acceptable where it runs — the background membership
   derivation on the 250 ms flush — but it would be a real problem if copied onto a query path, and
   it is the reason `tag` should be the last container kind offered in the UI. Recorded in
   `schema.md` §13.2 rather than discovered at measurement time.

---

## 4.6 What still needs Joe

R4 items 1–4 above are unchanged. Three additions from this round:

| # | Decision | Why it cannot be settled below him |
|---|---|---|
| **5** | **`CLAUDE.md`'s "Where things live" table is missing `docs/design/`** (H-29). The row is drafted in the disposition above and is a one-line paste | This pass may not edit `CLAUDE.md` |
| **6** | **Whether `work.kind` gains a `person` member in migration 0001** (§4.5 item 1). Authors and illustrators are currently `artist`-kind works, which puts them in the Music navigation type and in the client prefix index | It is a `kind_byte` allocation, which §5.3 states is unchangeable once clients cache ids — the same "now or never" shape as ADR-0030, and the same size of decision |
| **7** | **The cold-boot shell** (P-17) is escalated a second time. It was D-20, was recorded rather than applied, and six media types made the blank first paint measurably worse | Unchanged from §D3 item 6: it is a build-config decision under ADR-0025 |
