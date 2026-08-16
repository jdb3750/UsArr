# UsArr — Adversarial review log

Two rounds so far. **Round 1** reviewed the design documents; **Round 2** reviewed the first code
drop. Round 2 is below; Round 1 begins at [§R1](#round-1--the-design-documents).

---

# Round 2 — the first code drop

**Date:** 2026-08-16. **Target:** `a279517` (`claude/hearth-thread-kirqa7`), *"feat: land the v0.1
core packages and embedded web shell"*.
**Input:** one adversarial review of the shipped Go code, in which every claim was executed,
decompiled to primary source, or reproduced rather than asserted from a comment.
**Scope of this log:** all 19 findings, plus items that came out of the build and wiring work rather
than the review. Nothing is dropped: a finding is either **applied** or **rebutted in writing**.

`internal/httpapi/` and `cmd/usarr/` were out of scope for the review itself (in flight at the time)
but are in scope for the fixes, since they landed before this pass finished.

---

## Counts

| Severity | Count | IDs |
|---|---|---|
| **Critical** | 0 | — |
| **High** | 3 | SEC-01, DB-02, BUILD-01 |
| **Medium** | 6 | DB-01, SSRF-01, CRYPTO-01, GRAB-01, CFG-01, WEB-01 |
| **Low** | 7 | DB-03, SSRF-02, SSRF-03, MOD-01, LINT-01, DOC-01, DOC-02 |
| **Nit** | 3 | CRYPTO-02, TEST-01, GO-01 |
| **Total from the review** | **19** | |
| Raised outside the review (build, wiring, gate) | 9 | W-01…W-03, B-01…B-06 |

| Disposition | Count |
|---|---|
| **Applied in full** | 24 |
| **Applied, with the reviewer's proposed fix rebutted** — see §2 | 2 |
| **Deferred deliberately, with the seam documented in code** — see §2 | 1 |
| **Rebutted** | 0 |
| Recorded as a deliberate decision rather than a defect | 1 |

Plus **2 unproven suspicions** (SUS-01, SUS-02), which the reviewer labelled as such and did not
count. Both are dispositioned in §3.

**Two of the reviewer's proposed fixes were wrong and were verified wrong by execution** (DB-02's
`ON DELETE NO ACTION`, GO-01's `go 1.25.0`). The findings themselves were right in both cases. See
§2.1 and §2.2 — this is the part of the review most worth not repeating.

---

## 1. Disposition of every finding

### 1.1 High

| # | Finding, in one line | Disposition |
|---|---|---|
| **SEC-01** | `persist()` marshals the unsanitised `ReleaseResource`, writing Prowlarr's full-admin API key to `release_candidate.raw_release_json` in plaintext, in every backup, forever — while `servarr.SanitizeRelease` exists with zero production call sites | **Applied.** `search.go` now marshals `servarr.SanitizeRelease(rel)`. **Verified independently before changing anything:** `SanitizeRelease` drops only `DownloadURL` and `MagnetURL`, and a grep of the whole tree confirms no production path reads either off a decoded stored release — `Client.Grab` sends `rel.GrabBody()` (guid + indexerId + downloadClientId only), and `recordProvenance` reads eleven other fields. The two comments claiming the blob is "needed verbatim for the grab" were the inverted ones and are corrected (`releases/store.go`, `releases/search.go`); `servarr/resources.go`'s boundary note now names **persistence** as a boundary, which was the conceptual gap. New `TestPersistedBlobNeverCarriesTheAPIKey` asserts the blob contains no `apikey=`, no key value and neither URL field, **and** grabs the candidate end-to-end from the sanitised blob to prove the fix cannot break the grab. Tripwire validated: reverting the one-line fix fails the test on five assertions |
| **DB-02** | `audit_log.actor_user_id … ON DELETE SET NULL` performs an implicit UPDATE that the append-only trigger aborts, so no user with an audit row can ever be deleted | **Applied**, with the reviewer's first proposed fix rebutted — see §2.1. Reproduced against the real migration first (`REJECTED DELETE user 5 … audit_log is append-only`). The `REFERENCES` clause is dropped entirely; `actor_user_id` is now a documented *historical* id. New `TestUserDeleteSucceedsWhenTheUserHasAuditRows` covers exactly the case the existing `TestSessionCascadesOnUserDelete` misses, and additionally asserts the audit row survives still naming the deleted actor and that the hash chain still verifies. `schema.md` §9 fixed at source with the reasoning |
| **BUILD-01** | `Makefile` exports `CGO_ENABLED := 0` globally while `GOTESTFLAGS` is `-race`, so `make test-go` — and therefore `make test`, `check-offline` and `check` — dies before running a test | **Applied.** Reproduced verbatim. `CGO_ENABLED := 0` stays as the *default* so anything producing a shipping artifact inherits it and a future build target cannot pick up the ambient value; `test-go` and `test-integration` carry a target-scoped `export CGO_ENABLED := 1`. **`make check` now passes for the first time** — see §4 for what else had to be fixed to get there, none of which was in the review |

### 1.2 Medium

| # | Finding | Disposition |
|---|---|---|
| **DB-01** | `write_queue.fail_reason`'s `CHECK (… IN (NULL,'rejected',…))` is a no-op that accepts any value | **Applied.** Reproduced exactly (`ACCEPTED fail_reason='TOTAL-GARBAGE'`, with `state` as the control rejecting correctly). `x IN (NULL,…)` yields `NULL` rather than `FALSE`, and a `CHECK` passes on `NULL`. Rewritten as `CHECK (fail_reason IS NULL OR fail_reason IN (…))` in the migration and `schema.md` §10, both with the reasoning. New `TestNullableCheckConstraintsActuallyConstrain` asserts the legal values still pass, garbage is rejected, and keeps `state` as the control |
| **SSRF-01** | Go sets `Referer` to the previous request's full URL *before* `CheckRedirect` runs, so a redirect leaks credential query parameters even though the target's query is stripped | **Applied.** Confirmed by execution against the real `ssrf.NewClient` before fixing: the assertion added to `TestRedirectStripsCredentials` failed with `Referer … ?api_key=originalsecret`. `"Referer"` added to `credentialHeaders`, with a comment explaining that it is unlike the others — nothing in UsArr sets it, `net/http` synthesises it — and why deleting it outright beats re-deriving it from the stripped URL |
| **CRYPTO-01** | The AAD binds `host:port` only, so an `https→http` downgrade on the same port does not invalidate the stored credential — the one edit §1.6 is normative about | **Applied.** New `crypto.NormalizeOrigin` returns `scheme://host:port` and `ServiceInstanceAAD` uses it; `AAD.HostPort` renamed to `AAD.Origin`. **The coupling matters more than the rename:** `httpapi`'s two re-entry comparisons used `NormalizeHostPort`, and leaving them would have let a scheme-only edit through without re-entry and produced a credential that could never be opened again — both now use `NormalizeOrigin`, with the trap documented at all three sites. `hostOf` (audit metadata) also switched, so a scheme change no longer logs as two identical hosts. `security.md` §1.2's formula updated. The old `TestSchemeIsNotInTheAAD` **asserted the gap** and is inverted into `TestSchemeIsBoundIntoTheAAD`; a "scheme downgraded" case added to the AAD-mutation matrix |
| **GRAB-01** | The preflight checks the protocol, but Prowlarr routes by `downloadClientId` when the release names one, so the preflight both passes grabs that will 500 and refuses grabs that would work | **Applied.** Both primary sources re-fetched from upstream `develop` and confirmed **verbatim** — `DownloadService.SendReportToClient`'s `downloadClientId.HasValue ? Get(id) : GetDownloadClient(protocol, indexerId)` and `DownloadClientProvider.Get(int id) => …Single(d => d.Definition.Id == id)`. `.Single()` throws `InvalidOperationException`, which is none of the three `DownloadClientUnavailableException` markers `errors.go` maps, hence the bare 500. `preflightDownloadClient` now takes the whole release and routes by id first, falling back to protocol. Three new subtests cover the stale id, the disabled named client, and the false-refusal direction; the stale-id message tells the user to search again, which is the actual recovery |
| **CFG-01** | `Config.SecretKey` holds the master key in a plain string with no `LogValuer`/`Stringer` guard | **Applied.** `LogValue()` and `String()` added, both on **value** receivers so a plain `Config` is protected and not just a `*Config` — the leak paths (`slog.Debug`, `fmt.Sprintf("%+v")`) are exactly the ones that pass a value. They render an explicit allow-list of fields rather than reflecting, so a future secret field is invisible until someone adds it. `<empty>` and `<redacted>` are distinguished, because an empty `USARR_SECRET_KEY` is fatal and an unset one is not. New `TestConfigNeverRendersTheMasterKey` drives eight renderings including a real `slog` handler, and asserts no prefix survives either |
| **WEB-01** | The embed regression tests skip silently in the gate, so the `//go:embed all:` trap they guard is unguarded | **Applied.** `make test-go` now depends on `web-build`, and sets `USARR_REQUIRE_WEB_BUILD=1`, which turns `requireBuilt`'s skip into a failure. The skip is right for a bare `go test` in a fresh clone and is kept for that case. Both halves verified by execution: with nothing embedded, a bare `go test` **skips** and the same test under the env var **fails** with an explanatory message |

### 1.3 Low

| # | Finding | Disposition |
|---|---|---|
| **DB-03** | `release_candidate` has no uniqueness on `(service_instance_id, guid)`, so repeated searches accumulate duplicates | **Recorded as a deliberate decision, which is one of the two outcomes the reviewer asked for** ("decide deliberately; right now it is neither"). `schema.md` §6 now states the decision, its bound (the 25-minute TTL, swept by `ix_rel_expiry`, so table size tracks search volume in a 25-minute window rather than uptime), and why the upsert alternative was rejected for v0.1: `ON CONFLICT DO UPDATE SET expires_at=…` silently extends UsArr's grab window while Prowlarr's own cache entry stays pinned to its original non-rolling 30 minutes, converting a visible "search again" into an invisible failure at grab time. Note the at-rest half of this finding is now moot — after SEC-01 the duplicated blob no longer holds a credential |
| **SSRF-02** | A pinned client cannot fetch any `derived` URL that is not on the pinned instance, silently breaking `security.md` §2's documented fallback | **Applied as documentation, which is the reviewer's own stated minimum.** The constraint is now written on `Options.SPKIPin`: it fails closed (the right direction) but does break the fallback, and `ClassDerived` clients expected to reach anything but the instance must not carry a pin. The two real fixes (per-connection TLS config, or two transports) are named. Not implemented now because no v0.1 path builds a pinned `ClassDerived` client — `SPKIPin()` is exported but uncalled, so the enrolment half does not exist yet either |
| **SSRF-03** | Stripping `p`, `t`, `s` from redirect *targets* will break legitimate redirects (cache-busters, sizes, pages) | **Deferred deliberately, with the seam and the argument recorded in code** — see §2.3. Not applied, and the reasoning is written at `stripCredentials` so the next author does not have to rediscover it |
| **MOD-01** | `go.mod`/`go.sum` are not tidy — four directly-imported modules marked `// indirect` — and `make modverify` cannot detect it | **Applied.** `go mod tidy` run; the four modules are now direct and `go.sum` is complete. `modverify` gained `go mod tidy -diff`, which exits non-zero and prints the patch without mutating the tree under `GOFLAGS=-mod=readonly`, and reads the module cache rather than the network so it stays inside the `check-offline` contract |
| **LINT-01** | `security.md` §6 claims an audit-log lint rule that does not exist | **Applied by building the missing control, not by downgrading the claim** — `CLAUDE.md` forbids documenting a control that is not there. New `TestNoCodeMutatesTheAuditLog` walks every non-test `.go` file's string literals for `UPDATE audit_log` / `DELETE FROM audit_log`. It is a test rather than a `.golangci.yml` entry because `forbidigo` matches function-call identifiers, not string-literal contents — so the rule as the document described it **could not have existed**, and §6 now says so. §6 is also corrected on scope: the rule covers **production code**, not "anywhere in the codebase", because `TestAuditChainDetectsTampering` and `TestAuditLogIsAppendOnly` mutate `audit_log` on purpose to prove the triggers reject it. Tripwire validated against a probe file |
| **DOC-01** | `security.md` §1.4 ("treated as absent") contradicts `CONFIGURATION.md` §3.2 ("refuse to start") on an empty `USARR_SECRET_KEY`; the code follows `CONFIGURATION.md` | **Applied.** §1.4 corrected to match the code, with `CONFIGURATION.md` §3.2 named as the owner and the tie-break stated for next time. The *reason* is added rather than just the verdict: treating set-but-empty as absent would silently generate a new master key and orphan every stored credential |
| **DOC-02** | `schema.md` claims ISO-8601 while every DDL default is `datetime('now')` | **Applied.** `schema.md` and the migration header now both say SQLite `datetime()` text, `YYYY-MM-DD HH:MM:SS`, UTC — with the reason (the timestamp columns are ordered lexicographically by three indexes, and that breaks the moment two formats share a column) |

### 1.4 Nits

| # | Finding | Disposition |
|---|---|---|
| **CRYPTO-02** | "No AEAD construction produces 40 bytes for a 32-byte plaintext, therefore the field is AES-KW" is not sound | **Applied.** The reviewer is right: RFC 5649 (AES-KWP) also produces 40 bytes for a 32-byte key, as would GCM with a truncated 64-bit tag. The comment now says the length is *consistent with* RFC 3394 rather than proving it, and argues the choice on its merits — RFC 3394 is the plain fixed-length wrap for input already a multiple of 8 bytes (a 32-byte DEK always is), so KWP's padding buys nothing; plus determinism and a built-in integrity check. The conclusion was right; only the reasoning was doing work it could not support |
| **TEST-01** | `TestServiceInstanceListScanIsIntentional` asserts with `t.Logf`, so it cannot fail and pins nothing | **Applied.** Changed to `t.Errorf` with a message naming `schema.md` §5 and asking for a deliberate edit. The point is precisely that adding an index should force someone to come here |
| **GO-01** | `go.mod` pins `go 1.25.7` while `DEVELOPMENT.md` says 1.25+ and `CLAUDE.md` says 1.24+ | **Applied, with the reviewer's proposed fix rebutted** — see §2.2. The *documents* were wrong, not `go.mod`. The floor has in fact risen further, to **1.25.13**, for a reason the review did not reach — see B-06 |

### 1.5 Raised outside the review

| # | Item | Disposition |
|---|---|---|
| **W-01** | **New credential leak, high.** The redaction deny-list covered provider and OpenSubsonic parameter names but no private-tracker passkey names. `ReleaseResource.infoUrl` is indexer-supplied and is surfaced to the browser as `info_url`, so a private tracker's passkey shipped straight to the client | **Applied.** `passkey`, `torrent_pass`, `torrentpass`, `rsskey`, `authkey`, `apipasskey`, `cookie` added to `ssrf.credentialParams` — the single deny-list, extended rather than duplicated — along with `auth_token`, `secret`, `secret_key`. `ARCHITECTURE.md` §14.5 item 5 and `security.md` §5 updated together with it, and the stale "these are NOT covered" comment at `httpapi.redactURLField` corrected. New `TestPrivateTrackerPasskeysAreRedacted` covers all seven names through **both** entry points (`RedactURL` and `stripCredentials`) plus case-insensitivity. A leaked passkey means account termination on a private tracker, since it is what the tracker attributes traffic by |
| **W-02** | The session cookie sets `Secure` only when the request is HTTPS, while the docs say always | **Applied as the code's behaviour, recorded here.** `CONFIGURATION.md` §0 makes plain HTTP on a LAN a supported v0.1 deployment, and an unconditionally-`Secure` cookie is silently discarded by the browser over plain HTTP — which makes login impossible on exactly that supported deployment. Conditional `Secure` is the correct behaviour; the documents overstated it |
| **W-03** | `/api/v1/services` returns `has_credential: true` rather than a masked key | **Applied as the code's behaviour, recorded here.** Masking would require decrypting a full-admin credential on a render path, which violates principle 1 and puts the plaintext key in process memory to produce four asterisks. A boolean carries the same information to the UI at none of the cost |
| **B-01** | ADR-0024 §6's `paths.relative: false` bullet is wrong | **Correction recorded here rather than applied**, because ADR-0024 lives on branch `claude/hearth-thread-vn9w7u`. SvelteKit skips the relative-path rewrite for the SPA fallback document — `@sveltejs/kit/src/runtime/server/page/render.js:120-122`, `if (paths.relative) { if (!state.prerendering?.fallback) {` — verified by building both ways and diffing the output. The setting is belt-and-braces, not load-bearing, and the ADR should not present it as the thing that makes root-absolute `/_app/…` paths work |
| **B-02** | The `//go:embed all:` trap is real | **Reproduced and guarded.** Dropping the `all:` prefix makes `_app` vanish from the embedded FS with **no error at any stage** — it compiles, it runs, and the page renders blank. Confirmed by execution: `TestEmbeddedFSCarriesAppDir` fails with `_app is missing from the embedded FS`. That test is the only tripwire, which is why WEB-01 mattered enough to fix rather than note |
| **B-03** | `//go:embed` cannot reach outside its own package directory, so `internal/web` embeds a mirror at `internal/web/spa` synced from `web/build` by `web/scripts/sync-embed.mjs`. `make clean` removed `web/build` but not the mirror | **Applied.** A stale mirror meant `make clean && make build` would ship whatever was embedded before — a silent wrong-artifact bug. `clean` now clears the mirror while preserving `.gitkeep`, which is what keeps `//go:embed` compiling in a tree where the frontend has never been built. Verified by execution |
| **B-04** | `pnpm.overrides` pins `cookie ^0.7.2` | **Recorded here, because JSON takes no comments and this is the only place the reason can live.** `@sveltejs/kit@2.70.2` declares `cookie: ^0.6.0`, which carries GHSA-pxg6-pf52-xh8x and made `pnpm audit` — and therefore `make vuln`, and therefore `make check` — exit 1. Remove the override when Kit's own floor moves past 0.7 |
| **B-05** | Doc contradictions surfaced by the build | **Applied.** Empty `USARR_SECRET_KEY` and the timestamp format are DOC-01 and DOC-02 above. Additionally: `docs/reference/tags.md` maps Newznab categories to media types its own `type:` vocabulary does not define — `5070` (anime), `7010` (magazine) and `6000` (adult). §3 now tabulates all three: `5070` resolves to `type:tv` + `genre:anime`, which §7 of the same file had already decided; `7010` and `6000` are marked **not decided**, with a recommended shape but no invented vocabulary, and the mapper assigns no `type:` for them rather than guessing. Laundering a magazine into `type:book` produces a library that is quietly wrong and cannot be corrected later without knowing which rows were guesses |
| **B-06** | `make check`'s `vuln` step failed on 15 *called* standard-library vulnerabilities | **Applied by raising the Go floor to 1.25.13.** Not in the review, and it is the second thing blocking the gate after BUILD-01. On 1.25.7 `govulncheck` reports called vulnerabilities in `crypto/tls`, `net/url`, `os` and `net/http`, with fixes spread across 1.25.8, 1.25.9 and 1.25.10 — there is no code-level workaround for a stdlib advisory. 1.25.13 is the newest 1.25.x and scans clean (0 called vulnerabilities). `go.mod` and `DEVELOPMENT.md` updated, the latter now naming **both** floors: goose `v3.27.3` forces ≥ 1.25.7, and `make vuln` forces ≥ 1.25.13 |

---

## 2. Rebuttals, and where the reviewer's fix was wrong

Recorded because a finding that is quietly ignored comes back — and because a *fix* that is quietly
corrected comes back the same way.

### 2.1 DB-02 — the finding is right; its first proposed fix does not work

The reviewer offered two fixes: *"`actor_user_id INTEGER REFERENCES user(id) ON DELETE NO ACTION`
(or no `REFERENCES` at all, and treat it as a historical id)"*. **The first does not fix the bug.**
Verified by executing all three variants against the real trigger:

```
-- A. ON DELETE SET NULL  (the defect)
   DELETE user  : REJECTED : sqlite3: constraint failed: audit_log is append-only
   users left=1  audit actor_user_id=5 (valid=true)

-- B. ON DELETE NO ACTION (reviewer's first suggestion)
   DELETE user  : REJECTED : sqlite3: constraint failed: FOREIGN KEY constraint failed
   users left=1  audit actor_user_id=5 (valid=true)

-- C. no REFERENCES       (reviewer's second suggestion)
   DELETE user  : OK
   users left=0  audit actor_user_id=5 (valid=true)
```

In SQLite, `ON DELETE NO ACTION` does not fire the trigger — it simply leaves the foreign key
violated, so the `DELETE FROM user` still fails. Only the error message changes. A reader who took
the first option would have believed the bug fixed while user deletion remained impossible, and the
existing test suite would not have told them otherwise.

Option C is applied. It is also the semantically correct one, which is the parenthetical the
reviewer got right: the audit log's stated purpose is "who deleted this", so the actor id must
survive the actor.

**On editing migration 0001.** `CLAUDE.md` says a merged migration is never edited. Migration 0001
is **not merged to `main`** — it is on this branch, in the drop under review — so editing it in
place is the correct move and is enormously cheaper than the alternative. Fixing this after merge
would mean a full table rebuild: SQLite cannot drop a foreign key with `ALTER TABLE`, so it requires
create-new / copy / drop / rename on a table carrying a hash chain, plus a second migration for
DB-01's `CHECK`. The rule exists to stop history being rewritten under deployed databases; there are
no deployed databases. This is called out explicitly so the edit is not mistaken for the rule being
ignored.

### 2.2 GO-01 — the finding is right; its proposed fix does not build

The reviewer proposed *"Set `go 1.25.0` in `go.mod`"*. Verified by execution:

```
$ go build ./...
go: github.com/pressly/goose/v3@v3.27.3 requires go >= 1.25.7 (running go 1.25.0)
```

goose `v3.27.3` is pinned by the Makefile and declares `go 1.25.7` in its own `go.mod`, so the
version in `go.mod` was correct and the **documents** were the thing out of step — the reverse of
the finding's conclusion. `DEVELOPMENT.md` is corrected, and now records both binding floors and the
evidence for each, so the next reader does not "simplify" it back to 1.25.

`CLAUDE.md`'s stack line still reads *Go 1.24+* and needs the same one-line correction to
**1.25.13+**. It is **left for the owner** rather than changed here: `CLAUDE.md` is the project's
instruction file and is not mine to edit on an agent's say-so. Flagging it so it is not lost.

### 2.3 SSRF-03 — deferred, not applied, with the argument written into the code

The finding is correct on the facts: `stripCredentials` shares its deny-list with the redactor, so
the OpenSubsonic short names `p`, `t` and `s` are removed from redirect *targets* too, where they
are far more likely to be a cache-buster, a size or a page number than a credential. An upstream
that redirects to `…?t=1699999999` gets a different resource, or a 400.

Not applied now, for three reasons:

1. **Over-stripping fails closed.** The failure mode is a broken fetch, which is visible. The
   alternative failure mode — under-stripping — is a silently leaked credential.
2. **Nothing in v0.1 follows a redirect where this bites.** The image and artwork pipeline that
   fetches CDN URLs is a later milestone. Splitting the list now creates a second deny-list with no
   consumer to validate it against, which is exactly the drift `redact.go`'s own comment warns about
   and the mechanism behind W-01.
3. It is a judgement call with a real security trade-off on both sides, and the milestone that has
   to live with it should make it.

What *was* done: the constraint, the recommended narrower list, and the reason the two lists would
differ are written at `stripCredentials`, so whoever lands the image pipeline meets the decision
rather than rediscovering the bug. W-01's additions were chosen to be long, specific names precisely
so they do not worsen this.

### 2.4 One place the review was too generous, corrected while fixing something else

The review records `TestRedirectStripsCredentials` as covering the redirect path and notes only that
it "never inspects `Referer`". That undersells it: the test was **asserting the credential-bearing
behaviour was safe** while the credential left in a header the test did not look at. The same shape
appeared twice more and is worth naming as a pattern, because in both cases a test was actively
pinning a defect in place rather than merely failing to catch it:

- `TestSearchPersistsCandidatesWithTheGrabWindow` asserted `raw_release_json` **must** contain
  `SECRETKEY`, on the false premise that the grab echoes the resource back verbatim (SEC-01).
- `TestSchemeIsNotInTheAAD` asserted that two URLs differing only in scheme produce the **same**
  AAD, with a comment explaining why that was acceptable (CRYPTO-01).

Both are inverted, and both carry a comment saying what they used to assert and why it was wrong, so
the inversion is not silently reverted by someone reading the old design docs. Neither was weakened
to make a fix pass — in both cases the new assertion is strictly stronger than the old one.

---

## 3. The two unproven suspicions

| # | Suspicion | Disposition |
|---|---|---|
| **SUS-01** | With `InsecureSkipVerify: true`, Go skips `VerifyPeerCertificate` on a *resumed* handshake, so TLS session resumption might bypass the SPKI pin | **Not actioned, correctly labelled.** The reviewer's own reasoning is the right one: a resumed session is bound to the certificate that was pinned on the original full handshake, so the pin holds transitively. Left open. If someone wants certainty the measurement is cheap — set `SessionTicketsDisabled: true` on pinned configs and measure the handshake cost — but shipping a change on an unverified mechanism is what `CLAUDE.md`'s "verify, don't assert" rule exists to prevent |
| **SUS-02** | `ClassConfigured` permits `169.254.169.254`, so an admin who types the cloud-metadata address into the service form gets a working SSRF primitive with a credential attached | **Per spec, not a defect — the reviewer's own judgement, and it is right.** `security.md` §2's table says `configured` "may reach private space, but only the exact validated host:port", and that is deliberate: users configure arbitrary internal URLs and an allowlist of "private space except the bits we think are dangerous" is unmaintainable. The mitigating structure is real — the credential is bound by AAD to that origin, so it is not *some other service's* key being sent. Recorded so the decision is visible rather than incidental |

---

## 4. What `make check` needed beyond BUILD-01

BUILD-01 was necessary but not sufficient. Recorded because "the gate has never run" means every
*other* gate failure is also undiscovered, and three were:

1. **`-race` needs cgo** — BUILD-01. Fixed by target-scoped `CGO_ENABLED := 1`.
2. **`make secrets` failed on six pre-existing fixture credentials** in `_test.go` files. None were
   introduced by this pass. Waived in a new checked-in `.gitleaks.toml` rather than by dropping the
   step, as the Makefile's own note requires. **The waiver matches the secret VALUE only, with no
   path clause, and that is load-bearing:** with `condition = "AND"` plus `paths = ['_test\.go$']`,
   gitleaks exempts the whole matching *file* rather than requiring both conditions — verified by
   execution, a probe file containing `const apiKey = "<a random 32-hex string>"` scanned
   **clean** under that config and is **caught** under the one that shipped. A path-based waiver
   would have meant a real 32-hex \*Arr admin key pasted into any test file scans clean, and every
   developer has one of those in `.env`.
3. **`make vuln` failed on 15 called stdlib vulnerabilities** — B-06. Fixed by the toolchain floor.

`gofumpt` also flagged one file from the concurrent `cmd/usarr` work; formatted, no logic touched.

**Final state — every gate green:**

```
go build ./...      (no output)
go vet ./...        (no output)
gofumpt -l .        (no output)
golangci-lint run   0 issues.
make test-go        ok × 11 packages, -race -shuffle=on
make check-offline  check-offline: OK
make check          check: OK
```

The race detector has now run over this code for the first time, including `runFanOut`, and is
clean.

---

## 5. What changed, in one list

- **Security fixes:** SEC-01 (credential no longer persisted), W-01 (tracker passkeys redacted),
  SSRF-01 (`Referer` stripped), CRYPTO-01 (scheme bound into the AAD), CFG-01 (master key
  unloggable).
- **Correctness fixes:** DB-02 (users can be deleted), DB-01 (the `CHECK` constrains), GRAB-01
  (preflight mirrors Prowlarr's routing).
- **Gate fixes:** BUILD-01, B-06, `.gitleaks.toml`, `go mod tidy`, `modverify` drift detection,
  WEB-01's tripwire, `make clean`'s stale-embed bug.
- **Controls built rather than claimed:** LINT-01's audit-log check.
- **Tests:** 8 new, 3 inverted (each of which had been asserting a defect), 0 weakened or deleted.
- **Docs corrected at source:** `reference/schema.md` (§9, §10, the timestamp claim, §6's DB-03
  decision), `reference/security.md` (§1.2, §1.4, §5, §6), `reference/tags.md` (§3),
  `ARCHITECTURE.md` (§14.5), `DEVELOPMENT.md` (the Go floor).
- **Left for the owner:** `CLAUDE.md`'s *Go 1.24+* stack line → 1.25.13+ (§2.2). ADR-0024 §6's
  `paths.relative` bullet, which lives on another branch (B-01).

---
---

# Round 1 — the design documents

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
