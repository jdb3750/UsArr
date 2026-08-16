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
