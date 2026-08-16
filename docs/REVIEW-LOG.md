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
