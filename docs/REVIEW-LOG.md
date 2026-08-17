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

## 6. A documented prerequisite was re-scoped, not dropped

**Date:** 2026-08-16. Logged here because ARCHITECTURE §13 and ADR-0001 both carried a hard
prerequisite, and changing one quietly is exactly the drift this log exists to prevent.

**What the documents said.** "**Required before schema work starts:** a one-day spike — this driver, a
500k-row fixture, WAL, the §7.7 pragmas, idle and peak RSS measured **on arm64**." The reference
hardware behind it is a Raspberry Pi 5.

**What was actually true.** The prerequisite was never met: the first code drop shipped with the
provisional pragma values and `reference/sync.md` §6 still read "pending measurement". Meanwhile the
schema landed anyway (migration 0001), so the gate had already been passed without being satisfied —
and the owner's deployment target is a ThinkCentre running Proxmox, i.e. **x86-64**. The only arm64
hardware in reach never runs UsArr. An arm64 measurement would have been a number about a machine
nobody deploys on, blocking work on the machine everybody deploys on.

**Disposition — applied, with the requirement re-scoped rather than deleted.**

1. **The measurement was built and run**, on x86-64: `make bench-rss` (`internal/db/spike`, behind
   the `bench` tag, never in `check`). Full result in **ADR-0001, correction 3**. It settles both of
   `reference/sync.md` §6's pending values — `mmap_size` is a **no-op** under this driver
   (`MAX_MMAP_SIZE=0`), `cache_size` is **per-connection** and therefore multiplies by the read pool
   — and it puts a measured 10 MB idle / 50 MB import-peak under §13's previously unmeasured budget.
2. **arm64 is recorded as explicitly unmeasured**, and its requirement is now a prerequisite to
   **claiming arm64 support**, not to v0.1. The harness is architecture-neutral; the command for that
   day is `make bench-rss` on the arm64 box, and its output is a second row in ADR-0001, not a
   replacement for the first. Until then, the Pi 5 reference hardware in §13 is design intent, not a
   validated target — said in §13 in those words.
3. **Nothing was tuned in the same change that measured.** `cache_size = -32000` costs 235 MB peak
   against 35 MB at `-2000` on 4 cores, which is a real finding and an owner decision; the finding
   was recorded first and the documents were made to assert the measurement rather than the default.
   **Both defaults were then changed as a separate, recorded decision** (ADR-0001, amendment,
   2026-08-16): `mmap_size` dropped from the pragma list as inert, `cache_size` cut to `-8000`
   (~85 MB peak on 4 cores). Both underlying claims were re-confirmed by direct execution before the
   change, and the amendment states its own limit — it is a memory-side decision, since the harness
   does not measure query latency.

**What would make this wrong.** If UsArr is ever run on arm64 — a Pi, an Ampere VPS, an Apple-silicon
container — none of the above transfers, and §13's budget is again unmeasured for that machine. The
re-scoping is a statement about the current deployment target, not a claim that arm64 is fine.

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
| **Recorded for Joe** — a documented decision this review may not reverse (D-17, D-20, D-24, D-25, D-27, D-30 — **D-30 has since been resolved**, 2026-08-16, by shipping the webfont; the count is as of the round) | 6 |
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
| **D-21** | MAJOR | **`contain-intrinsic-size` is the load-bearing half of §7.4's recommendation and it is never given a value, anywhere.** The browser uses that number as the placeholder height for each skipped element; when it is wrong the scrollbar jumps as content scrolls in, which reads as *slowness* — the exact failure §7 exists to prevent. It cannot be one constant, because the density control moves row height across 28 / 32 / 36 px plus three more values for two-line and thumbnail rows. The mockup uses no `content-visibility` at all, so the recommendation has never been exercised | **Applied to the document**, as a ⚠️ block in §7.4: derive it from the same custom property the row height reads (`contain-intrinsic-size: auto var(--row-h)`), test it with the density control while scrolling, and treat §7.4 as a direction rather than an implementable rule until that exists. Left unfixed in the mockup, whose tables are 10–15 rows. **Since fixed there too, and the finding's own objection to a constant is what shaped it:** rows are `display: grid` (P-01), so `content-visibility` is no longer inert, and `usarr.css` carries `contain-intrinsic-size: auto calc(2 * var(--row-py) + var(--row-lines, 1.1) * var(--lh-base))` — derived rather than constant, tracking the density variable across all three settings, sizing the *content* box as the property requires, with `--row-lines` declared per list and measured from that list's own rendered rows (1.11 on Home's recently-added, 2.61 on Requests' indexer failures). `auto` then replaces the estimate with the remembered real size. The premise *"the mockup uses no `content-visibility` at all"* no longer holds. **Fully closed by SW-23**, which supplies the measured values this finding asked for (28 / 32 / 36 px one-line, 45 / 49 / 53 px rich) and confirms the derived expression already lands on them — **and by SW-22**, which found that *"`auto` replaces the estimate with the remembered real size"* is the sentence with the bug in it: the remembered size can be one density stale |
| **D-16** | MAJOR | **The motion rule forbids the token two rows above it, and the reference stylesheet breaks it six times.** §6: *"Only `opacity` and `transform` may be transitioned."* The same section's table specifies "Hover / focus colour change 80 ms", `tokens.css` ships `--dur-hover: 80ms`, and `usarr.css` transitions `background-color` on `.nav__link`, `.btn`, `.segment button` and `.subnav a`. A rule the reference implementation cannot obey is a rule nobody will obey, and the greppable lint line in §13 encodes the wrong version | **Applied to the document.** The rule now permits `color` / `background-color` for the 80 ms hover row only, and explicitly names `border-width` and `text-decoration-thickness` among the forbidden geometry properties |
| **D-38** | MINOR | `a:hover { text-decoration-thickness: 2px }` — a geometry change on hover, which §6 forbids in the same breath as "hover changes colour only" | **Applied.** Hover now changes colour only; links are underlined at rest so the affordance never depended on it |
| **D-46** | MINOR | `prototype.html`'s hash router calls `window.scrollTo(0, 0)` on every `hashchange`, including Back — contradicting §7.3 rule 5, which the document argues is *the* difference between feeling fast and feeling slow | **Recorded.** A limitation of the single-file build, not of the design; noted because the prototype is what people will judge |
| **D-40** | MINOR | On a 390 px phone the fan-out bar rendered "8 of 9" as one character per line: `.fanout` was a non-wrapping flex row | **Applied.** `flex-wrap: wrap` plus `white-space: nowrap` on the count |
| **D-23** | MAJOR | **Search shows 20 scannable items above the fold at 1440×900 against the document's own `[review]` rule of ≥ 25.** Measured in Chromium, default compact density. Home passes with 27 (12 poster cards + 15 rows). The README claimed "roughly ten poster cards and fifteen table rows" for home and said nothing about search | **Applied.** README now reports both measured numbers and states plainly that search misses the threshold, rather than rounding up |

### D1.4 Conflicts with `ARCHITECTURE.md`, and roadmap honesty

| # | Sev | Finding | Disposition |
|---|---|---|---|
| **D-22** | MAJOR | **OQ-1 understates how entrenched virtualization is.** The design document cites §4.5 and §16. There is a third site, and it is an **accepted ADR**: ADR-0003 rejects HTMX partly because *"a 10k-item virtualized poster grid with instant client-side filter/sort **is** a rich client-state problem"*. Settling OQ-1 against virtualization means amending three documents, one of them an ADR — not one threshold | **Applied to the document.** The ⚠️ block in §7.4 and OQ-1 now name all three sites and the amendment cost |
| **D-30** | MAJOR | **The mockup README rebuts ADR-0025's own reason for rejecting the system stack.** ADR-0025 and §4.1 reject `system-ui` because cross-platform metric drift is *"not cosmetic"* for a fixed-row-height design. The mockup loads **no webfont at all**, and its README says: *"If Plex is not installed on the viewing machine the system stack renders instead, and that is the correct and expected result: the design does not depend on the specific face, only on the metrics band it sits in."* If that sentence is true, ADR-0025's typographic argument is not; if ADR-0025 is right, the mockup has never demonstrated the typeface decision | **Recorded for Joe — and since RESOLVED, 2026-08-16.** As recorded at the time: both statements are defensible in isolation and they cannot both stand, and that was stated inside OQ-3 rather than left to be discovered. **It was then resolved by shipping the font rather than by choosing between them.** `mockups/fonts.css` now self-hosts IBM Plex Sans v23 (variable, 100–700) and IBM Plex Mono v20 at 400/600 as base64 `latin`-subset woff2, 76 KB on disk, OFL alongside in `mockups/fonts/`; and `mockups/selftest.mjs` carries a standing canvas advance-width probe — the exact validation §4.1's ⚠️ block demanded, and explicitly *not* `document.fonts.check()`, which had already returned a false positive here. Measured: `"IBM Plex Sans"` 459.000 px vs a bogus family's 401.074 px, `"IBM Plex Mono"` 504.000 px, and **the body's own computed stack 459.000 px — identical to Plex Sans, so the body genuinely renders in Plex and not in a fallback**. The finding's premise (*"the mockup loads no webfont at all"*) is therefore no longer true, and with it the contradiction: ADR-0025's typographic argument is now **demonstrated by** the artefact instead of rebutted by it. The README sentence survives, narrowed to what it can honestly claim — a statement about graceful degradation *if the load is blocked*, not a claim that the specific face never mattered. §4.1's ⚠️ block and OQ-3 are both updated; **what remains open in OQ-3 is the subset alone**. D-32 is untouched by this — the metric-drift claim is still uncited inference |
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
| **D-41** | MINOR | Inline `style="margin-top:8px"`, `style="margin-left:0"`, `style="max-width:120px"` and similar in five places, against the token rule. The `--dc` dominant-colour properties on cards are legitimate — that is data | **Applied, and the severity was understated.** It was 119 attributes across the five screen files, not five places, and the real defect is not the token rule: §14's production CSP drops inline styles, so every one of them demonstrated styling that **cannot ship** — rendering correctly here and differently in the real app, which is worse than rendering wrong because nobody re-checks a screen that looks right. The 80 genuine nudges became the `.u-*` classes in `usarr.css` §2.13; the 38 `--cols` / `--row-lines` declarations became one `.cols-*` class per list in §2.12, **not** collapsed onto shared values, because §7.4 requires `--row-lines` be measured per list and merging equal-today values would let a later remeasure of one silently move another. Verified by rendering rather than by diffing: 734,496 element rects across 592 screen×state×install×theme×viewport combinations, before and after, **zero differences** — every row height, resolved `grid-template-columns`, header width, `contain-intrinsic-size` and the overflow verdict identical. `check.mjs` §1d now bans the attribute with floors on both the file count and the corpus size. The `--dc` exemption stands and is stated in the rule: CSP governs the style attribute in markup, not `el.style.setProperty` from script |
| **D-48** | MINOR | **Citation audit.** Ten primary sources re-fetched live. **Nine verified verbatim:** Sonarr `PageSidebarStatus.js` (severity kinds, `return null` at zero); Sonarr `Styles/Themes/dark.js` (`torrentColor: '#00853d'`, `usenetColor: '#17b1d9'`); Sonarr `PageSidebar.js` (Series/Calendar/Activity/Wanted/Settings/System with the stated children, `statusComponent` on Queue and Status); Prowlarr `PageSidebar.js` (Indexers → Stats, Search, History, Settings, System); MDN `content-visibility` (*"the skipped contents must still be available as normal to user-agent features such as find-in-page, tab order navigation, etc."*, Baseline September 2024); NN/g skeleton screens (all three quotes); WCAG 1.4.11 (*"2.999:1 would not meet the 3:1 threshold"*); Krebs (1,590 sites, 16 detectors, 22 / 32 / 46%, 5–10% false positives, Inter and dark-theme contrast both scored); Viget (39 / 39 / 58, 2.82 / 2.41 / 2.29 s, 59 / 74 / 66%, 10.54 / 9.49 / 9.50 s); NN/g animation duration (both quotes); MDN `font-family` (the `system-ui` warning, quoted correctly). Sonarr's `en.json` strings all check out locally, including `KeyboardShortcutsFocusSearchBox` = "Focus Search Box". **One could not be verified:** `wiki.servarr.com/prowlarr/search` renders client-side and returns no body — the Options/Filter vocabulary claim is independently corroborated by Prowlarr's own `en.json`, which ships both `Options` and `Filter`. **One miscitation found and fixed: D-32.** Everything else in the document says what it claims its source says | **Applied** (D-32); the rest **recorded as verified** |
| **D-49** | MINOR | **The list primitive's row expander had no written contract**, so the three things a hand-written implementation gets wrong were only discoverable by reading `List.svelte`: an open expander is a real row, so `aria-rowindex` has to be a running total rather than `offset + i + 2`; `aria-rowcount` has to include the open expanders too, or index and count disagree and the user is told "row 9 of 7"; and the expander must carry no row identity or the roving model walks rows and expanders alternately and the list stops being one tab stop | **Applied.** §11 now states the contract beside the `aria-rowcount` / `aria-rowindex` rule it extends, including why the expander cannot be a `region` (a `rowgroup` owns rows and nothing else). Written against the shipped `web/src/lib/List.svelte`, which already implements all of it. **Two divergences between mockup and implementation are recorded rather than reconciled:** `--row-lines` (mockup, a unitless multiplier) and `--row-ci` (component, a measured content-box height in px) are the same §7.4 seam in different units, so a number must never be copied between them; and the component writes `--cols` / `--row-ci` through `element.style.setProperty()` against `style-src 'self'`, which independently corroborates D-41 and the CSSOM carve-out in `check.mjs` §1d |
| **D-50** | **The mockup was the last thing still setting the poster title over the art**, which §9.7 had already ruled against — *"the title and year sit BELOW the tile, on the chrome's own ground, never over the art"*. Because it did, `usarr.js` carried `constrainDominant()`: a runtime WCAG solver that picked a text colour against the computed fill and then moved the **fill** until the pair cleared 4.5:1 | **Applied, and the subsystem is deleted rather than improved.** It could not have been made safe: it constrained against a **single averaged colour**, and real cover art is not one colour — a white title over the light half of a Blue Note sleeve fails whatever the average says. With the title on a known ground the problem disappears: it is ordinary `--fg` / `--fg-muted`, which check.mjs §3 already asserts at 12 and 5.5 against all five grounds in both themes. **The `--dc` fill stays** — it is still the image placeholder — and so does the CSSOM carve-out in §1d; `--dc-fg` is gone. **§11's rule is retained with no call site, and that is stated rather than dressed up:** after this change nothing in the mockups or in `web/src/app.css` sets text on a computed fill, so the CI assertion has nothing to run over and §11 and the §13 checklist both say so instead of reading as passing. 🚩 **The render comparison caught a real bug this change introduced**, which is the argument for measuring rather than eyeballing: `.card__art` is a `<span>`, and removing the old `display: flex` left `aspect-ratio` inapplicable to an inline box — the tile collapsed to **2×19 px**. Fixed with `display: block`. Verified over 1,290,784 element rects across 1,040 combinations: **96 combos differ and every one is a poster panel** (64 on Home, 32 on Search, which has its own poster view), all 944 others identical. Within them the change is exactly title + year moving below the tile — every card +38 px, `.card__art` unchanged in size at 112.8×169.19 and moved only where a row above it grew, `.card__meta` unchanged and merely lower. Two side effects, both checked and both wanted: a title now has the full card width (112.8 px) instead of the art's padded box (94.8 px), so two 2-line titles became 1-line; and title line count now affects card height where the fixed-aspect box used to hide it, so one row is 245 px against 229 px — ragged rows hanging from the row top, which is what `align-items: start` already specified and what Jellyfin and Navidrome both do |

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
  **Since resolved in part, 2026-08-16:** D-30 is closed — the font ships, self-hosted, and the
  self-test proves the body renders in it — which is the "one of those two". D-32 stands.
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
| **2** | **OQ-3, and it has changed shape twice.** Cost is measured and affordable (D-31); the font now ships and is proven to render (D-30, resolved 2026-08-16); what is left is the *subset* (`latin` at 103.6 KB vs `latin-ext` at 177.2 KB, and **neither one renders native CJK**), plus the fact that the argument which beat the system stack is uncited inference (D-32) | It costs bytes on first paint, and the subset is the one part a self-hoster cannot change without a rebuild |
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
| **1** | **OQ-3, narrowed: the font subset.** Family settled and now actually shipping (D-30, resolved 2026-08-16); `latin` vs `latin`+`latin-ext` is a 73.6 KB first-paint cost against accented and transliterated titles across four of the six types, and **neither subset covers CJK**, so untransliterated manga titles fall back either way | It is the only remaining decision here that costs bytes on first paint; the deciding argument against the system stack is still uncited inference (D-32) |
| **2** | **Verify Symfonium's API-key support before v0.4 is scoped.** Not a judgement call so much as an errand only he can run — install it, point it at a stub, read the query string | v0.4's success criterion is written in terms of a capability nobody has confirmed exists |
| ~~**3**~~ | ~~**Whether Prowlarr-only remains an honest v0.1 story for *music*.**~~ **CLOSED 2026-08-16 by the owner — SW-08.** The answer is narrower than the question: music is not second-class (a library's catalogue source and request destination are separate bindings, Navidrome catalogues music in v0.1, and Lidarr defers because **no** write-capable service ships in v0.1), and what is genuinely true is that the indexers carrying music are largely private, so a stock Prowlarr returns thin music results. That is now stated on the Requests screen where a user meets it (§17.6, and the mockup's `thinmusic` state) rather than only in an ADR | — |
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
| **P-04** | MAJOR | D-21 is still open and the prescribed fix, `contain-intrinsic-size: auto var(--row-h)`, is measurably wrong twice over — three times, counting the content-box error | **Applied, all three.** `--row-h` is inert on a table row (measured: forcing it to 100 px leaves the row at 28.0 px), which the P-01 grid primitive also fixes as a side effect; **eighteen distinct row heights across three densities**, mean 42.0 px at compact against a declared 28, so a 25,000-row estimate understates by ~33%; and `contain-intrinsic-size` sizes the **content box**, so a 24 px row with `auto 28px` produced a 37 px placeholder. What ships is `auto <measured content-box height>` per row shape, and **the assertion is drift, not frame time** — `|Δ scrollHeight| / scrollHeight < 2%` at 1k / 5k / 25k rows, both themes, all three densities. DESIGN-DIRECTION §5.3 now says its nine density numbers are floors, not rendered heights. **Closed by SW-23**: the measurement exists, the "until it does, this is a direction not a rule" caveat is gone from every file this thread owns, and the mockups were checked against the measured values rather than assumed to match |
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
| ~~**6**~~ | **CLOSED 2026-08-16 — the owner decided it; see §5.5 and [ADR-0033](./DECISIONS.md#adr-0033).** ~~Whether `work.kind` gains a `person` member in migration 0001~~ (§4.5 item 1). Authors and illustrators are currently `artist`-kind works, which puts them in the Music navigation type and in the client prefix index | It is a `kind_byte` allocation, which §5.3 states is unchangeable once clients cache ids — the same "now or never" shape as ADR-0030, and the same size of decision |
| **7** | **The cold-boot shell** (P-17) is escalated a second time. It was D-20, was recorded rather than applied, and six media types made the blank first paint measurably worse | Unchanged from §D3 item 6: it is a build-config decision under ADR-0025 |

---

# UsArr — Round 5: three adversarial UX/UI reviews of the prototype

**Date:** 2026-08-16. **Branch:** `claude/hearth-thread-vn9w7u` @ `271ba14`. **Inputs:** three
independent adversarial reviews, run against `docs/design/mockups/prototype.html` read alongside
`ARCHITECTURE.md` §17 and `design/DESIGN-DIRECTION.md`. Every one of the three drove the artefact in
Chromium rather than reading its source: 32 states × 5 screens, at 1440×900, 1920×900, 1280×900 and
390×844, both themes, all three densities, mouse and keyboard-only, with the accessibility tree read
over CDP.

| Review | Lens | Findings |
|---|---|---|
| **Visual craft** (`V-nn`) | "does this look designed by a person" · typography · density · colour · the blur test | 3 BLOCKER · 8 MAJOR · 5 MINOR = **16** |
| **Usability** (`U-nn`) | task flow · learnability · error recovery. All 32 states walked; every task attempted keyboard-only | 5 BLOCKER · 19 MAJOR · 14 MINOR = **38** |
| **Information architecture** (`I-nn`) | language · IA · and whether this is usable by people who are not looking at it | 4 BLOCKER · 28 MAJOR · 29 MINOR = **61** |
| | | **115 findings** |

> **A note on the prefixes.** Round 1 used `C-`, `S-`, `P-`; round 2 `D-`; round 3 `R-`; round 4
> reused `C-` and `P-` and added `H-`. This round uses `V-`, `U-` and `I-`, none of which has been
> used before, so nothing here collides. Where an earlier finding is referenced it is written as,
> for example, *"round-4 P-01"* or *"D-05"*. The three reviews call themselves "review 3" because
> they are the third pass over the *design* artefacts; this log numbers rounds across the whole
> repository, so they are round 5 here.

**`CLAUDE.md` requires that findings are never silently dropped, so all 115 are dispositioned below —
MINORs individually, not in a summary paragraph.**

## Counts

| Disposition | Count |
|---|---|
| **Applied in full** — the document changed and nothing is outstanding | **65** |
| **Applied to the documents; the artefact half assigned to the concurrent mockup pass** | **34** |
| **Assigned to the concurrent mockup pass** — wholly a change to `docs/design/mockups/`, which another agent owns and which this pass did not touch | **9** |
| **Rebutted in part** — the finding's direction applied, a specific claim or remedy refused in writing: **V-06**, **U-32**, **I-08** | **3** |
| **Applied in part, with the residue left open in the document** — **U-35** | **1** |
| **Recorded, no change required** — a verified *positive* finding, or one wholly resolved by another: **I-57**, **I-58**, **I-61** | **3** |
| | **115** |

**Nothing is rebutted in whole.** By review: **craft** 6 applied · 9 doc-side with the artefact
assigned · 1 rebutted in part. **Usability** 27 applied · 7 doc-side with the artefact assigned ·
2 to the mockup pass · 1 rebutted in part · 1 applied-in-part-and-open. **IA** 32 applied · 18
doc-side with the artefact assigned · 7 to the mockup pass · 1 rebutted in part · 3 recorded.

**43 findings touch `docs/design/mockups/` in whole or in part** — 34 with a document half applied
here and 9 wholly in that directory. **Nothing in that directory was
edited by this pass** — a concurrent agent owns it; every mockup finding below is recorded with
enough detail to be actioned there, and where the same finding also had a document half, that half is
applied here and marked.

**Ten changes in this round were decided by the owner rather than derived from a finding** and are
applied as decisions: the Search→Requests route (U-01), the recent-grabs surface (U-02), the
type-aware post-grab copy (U-05), `scope-empty` and the always-reachable scope control (U-03), the
withdrawal of the empty-state type exemption (V-03), the `aria-rowcount`/`aria-rowindex` requirement
and the availability-glyph rule (I-42, I-01), the library-noun copy rule (I-05), the service-name
field and the connection-test failure surface (U-16, U-04), and **the `person` kind** — which closes
round-4 §4.6 item 6 and lands as **[ADR-0033](./DECISIONS.md#adr-0033)**.

---

## 5.1 Visual craft — disposition of all 16

| # | Sev | Finding, in one line | Disposition |
|---|---|---|---|
| **V-01** | **BLOCKER** | The two fix-it buttons on Services are clipped at **every** desktop width — a fixed 132 px action track inside a wrapper carrying `overflow-x: clip`, so there is no scrollbar to reach them either | **Applied to the documents; the CSS assigned to the mockup pass.** DESIGN-DIRECTION §9.1 gains the rule the declared-column model never had: **a declared-column list needs an overflow policy, and "clip" is not one** — the action track is `minmax(max-content, auto)`, the explanation column absorbs the remainder, and a genuine overflow degrades to a scroll. The interaction with the sticky-header rule is stated (the scroller goes *inside* the row, which satisfies both), and the CI assertion is adopted verbatim: no element's `getBoundingClientRect().right` may exceed `innerWidth` on any screen × state × width triple. The reviewer's framing is why this is a blocker and not a nit: `CLAUDE.md` says this screen *"must show what in the pipeline is broken and how to fix it"*, and the two labels that get sheared — *"Run full sync now"* and *"Update API key"* — are precisely the ones attached to the degraded and the down rows |
| **V-02** | **BLOCKER** | Services and Libraries are a different product from Home, Search and Requests: measured `tr` heights of 103–119 px and **80–160 px** against a stated 28 px compact row, because prose has been put in table cells | **Applied to the documents; the redraw assigned.** Two rules, both in §9.1. **(a) The truncation policy** (V-16), which is the actual cause. **(b) No table cell contains prose, and none contains design rationale** — with the measured instances named: a `Problem` cell holding a six-line essay about a rendering decision on a *different screen*, a `State` cell holding a four-line paragraph with a scare-quoted rebuttal of a reviewer, a `Request destination` cell holding sixty-two words of competitive analysis in a column whose other rows hold the word `None`. The reviewer's diagnosis is adopted as the rule: three kinds of object in one column means the column has no defined role, and the tallest sets the row. §1.6's test is tightened to per-clause. The specific instances are also fixed at their source in ARCHITECTURE §17.3 and §17.8 rather than only as a design rule |
| **V-03** | **BLOCKER** | The empty states are the centred hero the design document explicitly forbids, seven times over — a 24 px heading **larger than the page's own H1**, centred, in a dashed box, byte-identical across five screens | **Applied**, as the owner directed and further than the finding asked. The §1.2 exemption (*"24px exists only in empty states"*) is **withdrawn**; `--text-empty` / `--leading-empty` are **deleted from `tokens.css`** rather than capped, and the §4.3 scale row with them; §9.6 restates the rule as **four constraints a linter can check** — `--text-lg` heading as an `<h2>`, left-aligned at the region's content edge, no container of any kind, one sentence — and §13 gains three `[grep]` lines including the removal of the empty-state exemption from the `text-align: center` ban. The reviewer's own reasoning is why this got the fullest treatment: it is the **one region in the artefact built by applying a template**, in a file where everything else was tuned by hand, so it is where a reader looking for the generated-UI tell will find it. The redraw is **assigned to the mockup pass**. U-35's residue — that the four-sentence first-run copy is good and the rule may be wrong *for first run* — is carried in §9.6 as a ⚠️ rather than resolved by fiat |
| **V-04** | MAJOR | `overflow-wrap: anywhere` on mono cells breaks release names and JSON mid-token — `x264` renders as `x26` / `4` | **Applied to the documents; the CSS assigned.** §9.1's truncation policy tier 2: **machine strings do not wrap**; they scroll in-cell or truncate with the full value behind the expander, and `overflow-wrap: anywhere` is **banned on mono content**, with `break-word` + `word-break: normal` as the fallback if wrapping must stay. The reviewer's sharpest observation is recorded because it generalises: this is the worst possible policy for exactly the content §4.2 identifies as machine data, since release names carry their meaning in their tokens — and it makes the one whole-file craft signature of the design look broken. Noted that the author already knew: a narrower rule two lines below sets `break-word` + `word-break: normal` |
| **V-05** | MAJOR | On Libraries a `✓ healthy` badge lands in four different places in four consecutive rows, because its x-position is a function of where the sentence happened to wrap | **Applied to the documents; the redraw assigned.** §9.5: **a status indicator never sits inline in a run of prose** — fixed slot or nothing — with the measurement carried (four positions across four rows, while the *same* badge two columns right is perfectly aligned on all four). And the reviewer's better half is taken as the actual remedy: the aligned `State` column already carries it, so **the inline copy is redundancy and is deleted**, which fixes the alignment and takes a line off four rows, serving V-02 as well |
| **V-06** | MAJOR | The light warning `#8a5300` is a brown that sits inside the warm neutral ramp, so the achromatic argument's payoff only lands in dark | **Applied in part; the magnitude claim rebutted on measurement — see §5.4.2.** Applied: §3.2 now separates the two success criteria the word and the glyph are bound by — **the word is text at 4.5:1, the glyph is a non-text graphic at 3:1, so the glyph may carry a separate, more saturated token** — which is the finding's real insight and the reason `#8a5300` lost its amber (it was forced dark to be readable *as text*). The candidate is computed and recorded with its arithmetic and **not landed**: `#a9700a` = **3.98:1** on the ground and **3.68:1** on the surface, clearing this document's own 3.2 target, while the finding's own two suggestions fail it (`#c98a00` = 2.80, `#b8860b` = 3.09). Rebutted: *"sits inside the warm neutral ramp"* is not supported — computed ΔE76 from `--fg-muted` is **46.3** light against **54.2** dark, a ~15% narrower gap, not a collapse |
| **V-07** | MAJOR | Every anchor styled as a button renders underlined, so the primary CTA is carried by the link affordance | **Applied to the documents; the one CSS line assigned.** `.btn { text-decoration: none }` goes to the mockup pass. **The document half is the larger half and is new §3.3a**: the reviewer's second point — that `--selected` fill plus 600 weight is a six-unit difference and not enough for the one button on the screen — is taken, together with the craft review's own §4 conclusion that **the one accent worth adding is a weight, not a hue**. `--selected` and `.btn--primary` take a 1 px `--border-strong` border. That fixes selection and the primary action together, adds no chroma, and reopens nothing in §3 |
| **V-08** | MAJOR | The Requests form collapses into a scatter at 390 px — labels to the right of their boxes at unrelated offsets, the submit button mid-column | **Applied to the documents; the redraw assigned.** §9.3: **below ~700 px a form is a stacked column**, label above control, every control full width. The reviewer's contrast is recorded because it is the useful part: the responsive work was done *properly* for the hard case — the stacked tables read better on mobile than their own desktop versions — and skipped for the form |
| **V-09** | MAJOR | Mono is decorative in the `Type` and `Instance` columns, and a single title cell carries three faces | **Applied to the documents; the redraw assigned.** §4.2 gains the boundary: a **taxonomy word** (`movie`, `album`) is an enum rendered as English, and the `Library` column one cell away holds the same kind of value in sans; a **user-chosen label** (`Radarr`, `Sonarr Anime`) is a string the user typed into a settings form and is exactly as human as `Movies`. Both go to sans. And **one value never carries two faces** — the `ER S12E14 …` case, which reads as a rendering fault. This matters more than a consistency nit because §1.2 names mono-as-decoration as a tell and then claims the design *converts* it; an incomplete conversion is the tell, visible |
| **V-10** | MAJOR | `--st-ok` and `--proto-torrent` are the same green, so protocol reads as status | **Applied, taking the reviewer's second option, and flagged to Joe as a change of visible character.** Verified independently: computed ΔE76 is **4.59** in light and **3.09** in dark — i.e. worse in the theme that otherwise works better. §1.1 bans indigo/violet/purple, which forecloses the obvious replacement hue, and every other direction lands on the usenet teal or the error red — so **the protocol chip loses its colour and the words `torrent` / `usenet` carry it**, which they already did in the same cell. That is §1.4's delete-the-decoration rule applied to a fill, it removes two tokens rather than adding a fourth hue, and it restores §3's own argument by leaving chroma to status alone. `--protocol-torrent` / `--protocol-usenet` are deleted from `tokens.css` in both themes and from the `@theme` block; the withdrawal note in the file states how to reverse it |
| **V-11** | MAJOR | On Home's "Your library", chroma marks the three rows that are fine and greys the three with 17 missing films, 857 missing episodes and 34 comics with gaps | **Applied.** §9.5 and ARCHITECTURE §17.2: **a complete row is a muted `✓` in neutral text; an incomplete row carries its gap figure in the warn role.** The finding is §1.1's own quoted rule inverted — *"Grey is a status. A healthy row is neutral"* — on the screen whose job is "what needs my attention". The secondary defect is applied with it and is arguably the bigger one: six rows carried **six grammars for one fact**, so no two rows can be compared; the grammar is normalised to `have / total · N missing`. The redraw is **assigned** |
| **V-12** | MINOR | Tabular figures are defeated by right-aligning the unit, and are missing on the eight composite cells that need them most | **Applied to the documents; the redraw assigned.** §9.1: **a figure and its unit are two slots, not one string** — right-aligned numeric span, fixed-width left-aligned unit span — with `tabular-nums` applied at the **cell** so composites inherit it. The reviewer's credit is recorded too, because it calibrates the rest: 222 cells carry `tabular-nums` and generated UI essentially never does |
| **V-13** | MINOR | Mobile Search turns a scanned results list into a ~70-line stack | **Applied to the documents; the redraw assigned.** §9.1: **a results list and a detail table get different rules below 760 px.** The reviewer's distinction is the whole content of the rule and is right — Services and Libraries rows *are* records you read one at a time, and stacking is correct there; a search result list is scanned, and stacking destroys the scan. Two lines per result |
| **V-14** | MINOR | The section heading is a 3 px step from a bold row title, at the same weight and the same colour | **Applied**, taking the reviewer's constraint that the fix must not be a size step. §4.3 gains a third signal at `lg`/`xl` only — a 1 px rule under the `h2` (the pattern already exists on Search) and/or `letter-spacing: -0.006em`, both zero-pixel. The honest correction to §1.2's own rebuttal is recorded with it: **three of the six steps are within 18% of each other**, so "six sizes, hard stop" is a thinner answer to the "no real hierarchy" tell than it reads. The six steps are kept; the claim is no longer oversold |
| **V-15** | MINOR | Poster titles are set over the cover art with no scrim, constrained against a single averaged colour | **Applied**, and it narrows a rule from the previous round rather than contradicting it. §9.2: **title and year sit below the tile, on the chrome's own ground.** The reviewer's argument is decisive and is quoted in effect — the `constrainDominant()` machinery does real work and *still* cannot be safe, because it averages and real cover art is not one colour, so a white title over the light half of a sleeve fails whatever the average says. Navidrome and every \*Arr poster view solve it by not attempting it. **Round-4 P-13's `dominant_color` contrast rule is narrowed, not withdrawn** — it still governs any text on a computed fill, which is the row-level tint, where the ground is known. This deletes a subsystem from a surface rather than adding a scrim to it |
| **V-16** | MINOR *(as a defect)* / structural *(as a cause)* | No truncation strategy exists anywhere: **one** `text-overflow: ellipsis` in 323 KB, no line clamp, two `overflow-wrap: anywhere` | **Applied, and it is the largest single document change of this round.** The reviewer's framing is exactly right and is adopted as written: *"It is not eight separate bugs; it is one missing decision."* §9.1 now carries the three-tier policy verbatim in substance — **identity fields** truncate with the full string in `title`; **machine strings** do not wrap; **explanations** never appear in a cell at all — plus the statement that **a row has a maximum height and the design says what it is**. It is the root cause of V-02 and V-04 and it is filed as a MINOR by severity and a structural finding by consequence, which is the correct reading |

---

## 5.2 Usability — disposition of all 38

| # | Sev | Finding, in one line | Disposition |
|---|---|---|---|
| **U-01** | **BLOCKER** | Search offers no route to Requests: 31 result rows, **zero** interactive descendants, and the one row saying "you do not have this film" has no exit | **Applied, as an owner decision.** ARCHITECTURE §17.4 gains **rule 6**: every result row whose availability is incomplete — a cross, a partial fraction, or a tier the user does not hold — carries a secondary `Search indexers →` linking to §17.5 with the query pre-filled and the category preselected from the row's media type. The reviewer's diagnosis of *why* it was missing is carried into the rule, because it is a document defect rather than an oversight: §17.5 specified Add as primary and indexer search as **its accompaniment**, and Add ships in v0.2 — so in the exact configuration v0.1 draws, the primary action does not exist and the secondary was never specified standing alone. §17.4's zero-results exit is now explicitly *not* the only route. This is also the reviewer's own "if I could make only one change", and it is one link |
| **U-02** | **BLOCKER** | A grab leaves no record anywhere: the confirmation lives in a transient search-result row and does not survive one navigation | **Applied, as an owner decision, in both documents.** §17.5 gains a **`Recent grabs`** block — ten rows, newest first, with time, release name, indexer, protocol, size, resolved type and last known state — and **§16's v0.1 funds it, with its cost stated plainly**: no new table, no new state machine, no background work, because the states are the durable command queue's own (`pending|inflight|verifying|done|failed`, §7.6) joined to the `provenance` rows §8.5 already writes. One read, one index, one endpoint, one block. The reviewer is right that this had fallen through a gap between three documents — §17.5 required a request list, §16 did not fund one, §16 wins by its own rule, so the shipping answer was "no grab history", stated nowhere. It is explicitly **not** the request model, which stays v0.2. README's v0.1 table gains the row, since it is generated from §16 |
| **U-03** | **BLOCKER** | Unticking every library is reachable, empties the entire application, and is explained only by a 13 px grey string in a sidebar that is `display: none` below 900 px | **Applied, as an owner decision, in three places.** `scope-empty` joins **DESIGN-DIRECTION §10's required state set** and **ARCHITECTURE §17.7's named states**, with the sentence and the one control that undoes it, rendered on Home, on Search and on every type grid. The reviewer's observation that §17.4 rule 1 makes it *worse* on Search — a zero-hit group does not render at all, so the screen draws literally nothing — is carried as part of the reason. And **the scope control is reachable at every viewport whenever the scope is not "all libraries"** (§8.1 point 5, §17.2, §17.7): a control whose entire job is to state what it hid may not itself be hidden, least of all on the device §17.1 singles out |
| **U-04** | **BLOCKER** | The mandatory connection test has no specified result — not pending, not success, and critically **not failure**, on the path a reverse-proxied or Tailscale install will hit | **Applied, as an owner decision.** §17.3 specifies all four states as a table, with the failure state carrying **the verbatim upstream or transport error in mono, exactly as the `Problem` column renders it, plus the two or three most likely causes for that error class**. The reviewer's structural point is why this was a hole rather than an omission: §17.3's verbatim-error contract covers *configured* services, and a service that fails its test is never saved, so it has no row and the error had nowhere to go. The success state names **the probed application and API version**, which is the only thing that catches "the Kind select still said `sonarr` and I pasted a Radarr URL", since both serve `/api/v3/system/status`. §17.7's "three fields" becomes four. The drawing is **assigned to the mockup pass** as a seventh Services state |
| **U-05** | **BLOCKER** | The grab confirmation stops at *"UsArr does not import downloads"* for movies and TV — the two types where the user's prior knowledge produces the **wrong** belief | **Applied, as an owner decision, in both §8.5 and §17.5.** §8.5 now carries three complete sentence shapes chosen from the library's request destination and the type, with the \*Arr case spelled out: *"Nothing will import this. Radarr did not request this release, so it will not pick it up."* The reviewer's reading is exactly right and is quoted in effect — to a Prowlarr → Radarr owner, *"UsArr doesn't"* reads as *"but obviously Radarr does"*, and the failure is silent and cumulative. Every input is already computed. §17.5 requires the complete sentence at the moment of the grab, and names why the four sink-less types were already correct while the two with an \*Arr were not |
| **U-06** | MAJOR | The Action column §17.3 calls *"the point"* is clipped at 1280, 1440, 1680 and 1920, with `overflow-x: clip` so nothing scrolls | **Applied to the documents; the CSS assigned.** Same finding as V-01 from the other review, and the two sets of measurements agree to the pixel. Disposition and the CI assertion are recorded at V-01 |
| **U-07** | MAJOR | The `needs re-identification` banner offers one button, and its own copy tells you not to press it | **Applied.** §17.3: **two buttons** — `Re-link — this is the same Sonarr` and `Not the same instance — remove and re-add` — the second stating in one line what happens to the works and to the libraries that instance feeds. The reviewer's framing is the reason this is MAJOR rather than a copy nit: this is the most destructive decision in the product, the copy says *"Re-link only if you know this is the same library"*, and the correct action when you know it is **not** was not offered, not named and not reachable |
| **U-08** | MAJOR | Renaming a proposal to match an existing one merges libraries silently — two rows both named `Comics`, a banner still saying "4 libraries", a button still saying "Accept 4 proposals" | **Applied.** §17.8: **on collision the rows merge visually into one with two `From` entries**, the button count updates, and the merged row carries the inline note. **The merge key is stated rather than left to be discovered** — case-insensitive, whitespace-trimmed, per user — which the reviewer specifically asked for and which was genuinely unspecified. §17.8's own words are why it is MAJOR: getting this wrong *"quietly destroys the project's most visible power-user feature"*. The one-way `user-managed` door gets its per-row marker in the same edit |
| **U-09** | MAJOR | The Libraries detail view has no Save, no Cancel, no dirty indicator — while a bare `<select>` in it re-derives membership for 1,842 items | **Applied.** §17.8 declares the model: **explicit save, with a sticky `3 unsaved changes · Save · Discard` footer**, plus a confirmation on `Kind` naming the item count and stating that nothing changes upstream. The reviewer's sharpest point is recorded: the Add-service form 400 px away has an explicit `Test` and a `Save` disabled and labelled `No changes`, so **the two most similar forms in the product had opposite and undeclared save models** — and a native `<select>` changes value on one arrow keypress with no menu open |
| **U-10** | MAJOR | The only sentence that says what a library *is* carries the `mockup` badge, i.e. is marked as not shipping | **Applied.** §17.8 promotes it to shipping copy under the page title, un-badged. The reviewer's test is decisive and is the argument in the document: strip the mockup-tagged prose and the Libraries screen contains **no definition of a library anywhere** — for the newest concept in the product, on a screen whose opening claim is that the Services/Libraries split *"is meaningful"*. Cost: one line. The un-badging is **assigned to the mockup pass** |
| **U-11** | MAJOR | Library slugs render as `/movies`, `/tv`, in mono, on the screen whose banner says UsArr never types a path — beside a cell holding a real root folder in the same face | **Applied to the documents; the redraw assigned.** Same finding as I-04. §17.8: **drop the leading slash and the mono face**; the Diagnostics panel already renders it correctly as *"In the URL as `?lib=ebooks`"* and is the only place it earns a row. The reviewer's audience argument is carried into the document, because it is the reason this is MAJOR: a self-hoster burned by a scanner reading the wrong folder reads `/movies` and concludes UsArr scans `/movies` |
| **U-12** | MAJOR | Requests is the only list with no roving tabindex — 30 tab stops inside the table — and its Grab control is an unlabelled icon with no `title`, 8 px from an external-link anchor | **Applied to the documents; the JS assigned.** Same defect as I-02, measured twice. §13 gains the CI assertion (*every list with `role="table"` and focusable descendants has exactly one row at `tabindex="0"`*) and the review line that **an irreversible action is never an icon with no visible label, and never adjacent to a visually similar benign one**; §17.5 requires the visible `Grab` label. The reviewer's detail is kept because it is the argument: the glyph is a download arrow, which in every other application means "download this file to my computer", and here means something else — and `Tab Tab Enter` fires a 68 GB grab with no confirmation |
| **U-13** | MAJOR | No skip link, 21 tab stops before content, and focus stranded on the nav link after a route change | **Applied.** §11's floors table gains **SC 2.4.1 Bypass Blocks (Level A)**, which was missing while 2.1.1, 2.1.2 and 2.1.4 were named, with the honest note that landmarks are an accepted technique so this is not a strict failure — and that a keyboard-only sighted user has no landmarks. The keyboard model gains **focus follows navigation**: on route change, focus moves to the new `<main>`'s `<h1>` and the page name is announced politely. §13 carries both as lint lines. The reviewer is right that §11 was detailed about *lists* and silent about *route changes* |
| **U-14** | MAJOR | The scope chip is pixel-identical to the density `<select>` and nothing changes when a scope is active; only Search says what was hidden | **Applied.** §8.1 gains point 6: **a non-default scope looks different from the default one**, using the status system's existing neutral "not default" step, **and the Search screen's inline line is the pattern for every scoped surface, not just Search**. The reviewer's citation is adopted as the reason: Sonarr's `AllSeriesAreHiddenByTheAppliedFilter` — which §9.6 already quotes approvingly — puts the message *where the content is not*, which is exactly what Home and the type grids lacked |
| **U-15** | MAJOR | Two rules collide and the resolution is unwritten: §17.2 removes a type the user does not have; §8.1 says counts respect the scope. Does scoping to Comics remove the `Movies` sidebar row? | **Applied — a decision, written down.** §17.2: **the sidebar reflects what you *own* and never changes shape with the scope; only the counts narrow, and a narrowed count renders `0 of 1,204` rather than `0`.** Clicking it lands on `scope-empty`. The reviewer's argument for that side is taken verbatim in substance: nav entries appearing and vanishing as a side effect of ticking checkboxes *inside a popover that overlays the nav* is the most disorienting thing a sidebar can do, and it makes the sidebar's height jump under the cursor. This is the two-axis model's hardest case and it was the one case not written down anywhere |
| **U-16** | MAJOR | Two service forms with different fields — the add flow never asks for a name, while the 1080p ✓ / 4K ✗ feature depends on telling two Radarrs apart — and the Services screen contains a Settings nav whose first tab is Services | **Applied, as an owner decision, in both halves.** §17.3 adds **`name` to the add flow**, defaulted from the probe and editable, so the common case still types three things, and §17.7's "three fields" becomes four. The IA half is applied with I-09: **the in-page nav is labelled `Settings` and lists only `General · Tags · UI`**, with one line saying Services and Libraries are top-level, which is what ADR-0027 and §17.8 already decided before a second nav re-introduced the thing they decided against |
| **U-17** | MAJOR | `Test Radarr` and `Run full sync` on Libraries are `<a href="#services">` — imperative verbs that navigate, dumping the user at the top of an eight-row screen | **Applied.** §17.8 and §13: **a control labelled as an operation performs it**, or it is labelled as navigation (`Radarr health →`) and deep-links with the row anchored and highlighted. The reviewer's point that the handler already exists on the Services rows is what makes the first option cheap, and it is recorded |
| **U-18** | MAJOR | Implementation vocabulary in user-facing status: `degraded / breaker open`, `needs re-identification`, `managed_by user`, `no work identity`, and a raw diff as the *body* of a banner | **Applied.** §17.3: **the `State` column and every banner title are UsArr's own words, in plain language**, with the mechanism on the second line and behind the expander — which already carries it precisely. The three replacements the reviewer drafts are adopted in substance: `paused — 7 failed attempts, retrying 14:19`, `this may be a different Sonarr`, `matched by title`. The distinction that makes this a rule rather than a preference is stated: the verbatim-upstream principle is right and it stops at the `Problem` column. §13 gains the grep line; §6.4 carries the `matched by title` half (I-13) |
| **U-19** | MAJOR | `Retry` is offered on an expired-cache grab, where retrying the same opaque release id returns the same 400 for ever — and the same screen already models this condition correctly elsewhere | **Applied.** §17.5: **the failure path branches on the upstream code** — for an expired-cache 400 the button is `Search again` and the chip reads `expired — search again`, not `failed: rejected`, which reads as "the tracker refused you". The reviewer's observation that the `expired` state on the *same screen* already replaces every grab control with `Search again` is the argument, and it is recorded: the failure path contradicted a state modelling the identical condition. The "every toast names its release" half is applied in DESIGN-DIRECTION §9.4 |
| **U-20** | MAJOR | The `allfail` state reasons its way to a correct DNS diagnosis and then offers two buttons that cannot address it | **Applied.** §17.5: **where the correct action is not something UsArr can offer, naming the non-action beats offering a fake one.** The reviewer's reasoning is why: Services will correctly show Prowlarr as reachable, because it *is* — the resolver failure is inside Prowlarr's container, not between UsArr and Prowlarr — and retrying fails identically. The honest surface is the sentence plus `Retry the search`. Recorded with the credit the finding gives it: this is the best-reasoned state in the set, and the gap is only the exit |
| **U-21** | MAJOR | Success is never announced; only failure is. The live region is wired to the failure path alone | **Applied.** §9.4: **success is routed through the same polite region and the row chip stays.** The reviewer's framing is preserved because it is the sting: the one confirmation sentence §8.5 fixes *literally*, quoted verbatim in three documents, was being delivered by the least visible mechanism on the page — to a screen-reader user, by nothing at all |
| **U-22** | MAJOR | `Add library` is a live-looking primary button with no specification anywhere — and it is the recovery path when the auto-proposal got it wrong | **Applied.** §17.8 specifies it: **the proposal row's own field set — name, kind, format filter — plus one `Add source` picker**, four fields reusing two existing components, with the free-text-path ban intact. The reviewer's argument for why it cannot be deferred is adopted: without it the auto-proposal is the only way a library comes into existence and it is a **one-shot**, and "one Radarr, split by tag" is the single most likely reason a user opens this screen |
| **U-23** | MAJOR | The stored API key is silently sent to a host the user has just edited, against a `CLAUDE.md` rule stated as non-negotiable | **Applied.** §17.3 and §9.3: **when the host of `Base URL` changes, the key field clears, Save disables, and the form says why.** The reviewer is right that the rule existed and the UI that would enforce it did not, and right about which edit triggers it — fixing a typo in a hostname is the most common edit on that screen. Noted that the `denied` sudo state gates *changing* a credential, not *repointing* one, which is why it did not cover this |
| **U-24** | MAJOR | `Import in progress` contradicts itself: a banner says 68% done while Block A shows completed figures including a `have/wanted` split that cannot be known | **Applied.** §17.2 states what those columns mean during an import: **`Items` is the source's declared total from first contact** (said once, above the block), **`Have` is null with a progress fraction until phase A commits**, and `Synced` reads `importing`. The reviewer correctly separates the mockup limitation from the design gap, and it is the design gap that is fixed: §17.2 and §17.7 never said, and this is the state a first-run user spends their first several minutes in. I-39's noun consistency is folded in |
| **U-25** | MINOR | Two proposal-table headers are truncated by their declared widths — `Accept` → `Acc…`, on the column that decides what gets created | **Assigned to the mockup pass.** Same root cause as U-06/V-01, whose document half (a declared-column list needs an overflow policy) is applied in §9.1 and covers it |
| **U-26** | MINOR | The `declined` chip overflows its 60 px cell and overlaps the adjacent word, rendering as `declineⓅodcasts` | **Assigned to the mockup pass.** Same root cause; §9.1's overflow rule covers the class |
| **U-27** | MINOR | The disabled `Save` is skipped by Tab, so a keyboard user gets no signal that a blocked commit exists | **Applied to the documents; the markup assigned.** §9.3: **a blocked commit must be reachable by keyboard** — `aria-disabled="true"` on a focusable control, activation swallowed |
| **U-28** | MINOR | `Escape` closes the dialog and discards a pasted 32-character API key with no confirmation | **Applied.** §9.4: a dialog holding unsaved credential input confirms before discarding. The reviewer's scenario is the one that makes it real and is recorded — reflexive `Esc` to dismiss a password manager's popup |
| **U-29** | MINOR | A `readonly` field is styled identically to the editable one two rows above | **Applied to the documents; the CSS assigned.** §9.3: a read-only field must not look editable — *"a lie told in CSS"* |
| **U-30** | MINOR | A routine sudo re-prompt leads with `PUT … 403: {"error":"sudo_required"}`, rendering a normal security step as a fault | **Applied.** §17.3: **a sudo re-prompt is not an error state.** §10 requires verbatim upstream text for *errors*, and the reviewer is right that letting the principle leak into a security prompt makes the product look broken when it is working. DESIGN-DIRECTION §10's `permission-denied` row already warned the two should not be conflated; now §17.3 says so where the prompt is specified |
| **U-31** | MINOR | The destructive `remove` confirmation's only buttons are `Open Libraries` and `Remove anyway` — no `Cancel` | **Applied.** §9.4: **a destructive confirmation always offers the safe option as a control, not as an escape.** The reviewer concedes it is a non-modal banner so navigating away works, and files it anyway as a known pattern failure, which is the right call |
| **U-32** | MINOR | Every problem is stated twice on Services and three times on a stale Home | **Applied in part; the third-appearance half rebutted — see §5.4.3.** Applied: §17.3 requires **one canonical statement per problem**, so the `System status` roll-up **links to the row rather than duplicating its Action button**. Rebutted: the sidebar severity badge is a count with no action and is §8.2's design, not a duplicate, so "three times" overstates it by one |
| **U-33** | MINOR | Block C's unified posters grid mixes 2:3 and 1:1 across six types, so per-card meta lines sit at four different heights on one visual row | **Applied.** §9.7 gains the amendment before the argument: **"shape from the image, not the type" holds inside a single-type grid and needs the case where its premise is false.** In the unified grid the card *box* is one shape and the image is fitted inside it; per-type grids are unchanged. The reviewer's own framing — the rule is right and costs legibility in the one container where all six types meet — is the amendment |
| **U-34** | MINOR | `Grabs 214` and `Peers 41 / 9` are unlabelled composites | **Applied to the documents; the markup assigned.** §9.1: **a composite numeric cell says what its parts are**, with the expansion in a visually-hidden span and in `title`. Same finding as I-14, which carries the ecosystem-verbatim constraint (keep the header `Peers`, fix the cell) |
| **U-35** | MINOR | The first-run empty state is a centred block with four sentences — the prose is good, so §9.6's own rule may be wrong for first run, and one of the two should change | **Applied in part, with the residue left open in the document.** The composition half is fixed by V-03. The **copy** half is genuinely unresolved and is carried in §9.6 as a ⚠️ naming both possibilities rather than settled by fiat, because the reviewer is right on both counts: the rule says one sentence, the first-run copy is four, and the four are good — it is *teaching the product* rather than reporting a state. §9.6 states the escape (extra sentences go below the buttons at `--text-base`, not as a wider centred measure above them) and flags that this belongs to the pass that rewrites the state. **Open — see §5.6** |
| **U-36** | MINOR | The same operation is `Test connection` in the modal and `Test` in the inline form | **Applied to the documents; the mockup assigned.** §13's copy lint: **one label per action across the whole product.** Folded with I-17, which supplies the full inventory |
| **U-37** | MINOR | `Metadata authority` is a radio group with exactly one option, already selected and impossible to change | **Applied.** §17.8: **suppressed entirely below two sources.** The reviewer's own cross-reference is the argument and is used as it: this is §17.4 rule 5's principle — a control that cannot vary is not data — applied to a control instead of a column |
| **U-38** | MINOR | Nothing on the Add-service form mentions a URL base or subpath, which is how a large share of this audience reaches its \*Arrs | **Applied, as an owner decision, with U-04.** §17.3 requires the base-URL help text to name it, and the reviewer's failure mode is what makes it more than a nit: getting it wrong produces a connection that **resolves and then 404s**, which is an error the design had no drawn home for. Recorded as one of the two most likely reasons a first connection fails on a typical install |

---

## 5.3 Information architecture, language and non-visual usability — disposition of all 61

| # | Sev | Finding, in one line | Disposition |
|---|---|---|---|
| **I-01** | **BLOCKER** | Availability is conveyed by an icon with **no accessible text**, on 62 elements: a screen reader hears identical silence on the film you have and the film you do not | **Applied, as an owner decision; the markup assigned.** DESIGN-DIRECTION §11 gains the rule — **no status glyph may have an empty accessible name** — with the word from §6.3's rollup shipping as text or a visually-hidden span and the glyph `aria-hidden`; §13 carries it as a grep line, and `tokens.css`'s status comment names it as the half that was missing. The reviewer's two extra points are both kept: it defeats **the product's central question** for a whole class of user, not merely SC 1.1.1; and it **falsifies a claim the mockup prints on screen** — *"There is no colour-impaired mode setting, because every status here is an icon and a word as well as a colour"* — which is untrue of the most-repeated status in the product. Filed as a lint line rather than a note because it is the second time an icon-only status has slipped through |
| **I-02** | **BLOCKER** | The three Requests tables carry `role="row"` and `tabindex="-1"` and **no row is ever `tabindex="0"`** — the roving handler binds on `[data-roving]` and they do not have it | **Applied to the documents; the JS assigned.** §13 gains the reviewer's own assertion, unchanged: *every list with `role="table"` and focusable descendants has exactly one row at `tabindex="0"`* — one `querySelectorAll` over the rendered DOM, which would have caught it. The measurements are recorded because they are the argument: seventeen lists carry the attribute and the three that do not are on **the one screen in v0.1 with a stateful outbound action**; rows were removed from the tab order without the replacement route being wired, so `ArrowDown`/`j`/`k` are unreachable from a cold start; and the tab rhythm is irregular because disabled Grab buttons make some rows two stops and others three |
| **I-03** | **BLOCKER** | The page sub-caption is not state-aware and contradicts the body of the screen — *"Last delta sync 14:02"* above *"No services configured"*, on the first screen a new user sees | **Applied.** §13: **a derived summary line renders from the same source as the body it sits above, or it is not rendered.** The reviewer's argument against waving it through as mockup wiring is adopted and is the reason it is a blocker: the sub-caption **ships**, it sits above the state block in both the reading order and the accessibility tree, so it is the first thing every user takes in — and `CLAUDE.md`'s "no invented status" exists for exactly this. Recorded that *"8 rows"* over an empty table and *"31 results"* over zero are the same defect. The six replacement strings are recorded for the mockup pass |
| **I-04** | **BLOCKER** | Every library row renders what reads as a filesystem path, in the face reserved for machine data, on the screen whose thesis is that UsArr never touches a path | **Applied to the documents; the redraw assigned.** Same finding as U-11, disposition there. The IA review's extra evidence is what makes it a blocker rather than a MAJOR and is carried into §17.8: **the value is not even a path** — the Diagnostics panel renders the real form as `?lib=ebooks`, with no leading slash — so the `/` is invented decoration that reads as the single thing the screen exists to deny |
| **I-05** | MAJOR | "Library" names two different things in one sentence, on four of six search groups: *"Ebooks: all in Ebooks"* | **Applied, as an owner decision.** DESIGN-DIRECTION §8.1 carries the rule in full — **the noun `library` is mandatory wherever a library name is rendered beside a media-type name** — with the reason: the `<h2>` is a media type (closed enum of six) and the value after "all in" is a library (unbounded, user-named), they collide as strings on the common install, and without the noun **the line teaches that the two axes ADR-0027 exists to separate are one axis**. §17.4 rule 5 carries it at the site, §13 as a copy lint. The reviewer's worst case is recorded because it is real: Audiobookshelf's container is *"Audiobooks & Ebooks"*, UsArr splits it into libraries named **Audiobooks** and **Ebooks**, and those are also two media-type names — three levels of the same word. One word, everywhere it occurs |
| **I-06** | MAJOR | Design-rationale prose ships inside product table cells, arguing with the reader rather than informing them | **Applied in three places.** DESIGN-DIRECTION §9.1 carries the general rule (with V-02); ARCHITECTURE §17.3 narrows the `Problem` column to *the upstream error and nothing else*; §17.8 fixes the `State` and `Request destination` cells at source. The reviewer's test is adopted as written: §1.6's *delete this element* extended to *delete this clause — does the user lose a fact they can act on?* — which every quoted clause fails. The four field-help examples are recorded for the mockup pass with their replacements |
| **I-07** | MAJOR | The "Composers, and what UsArr cannot do with them" section is an essay rendered as a `role="table"`, three levels deep in an album drill-down | **Applied to the documents; the cut assigned.** §6.4's classical-limitation rule gains the form it must take: **one inline note on the artist or album it concerns, never a table, a heading or a section of its own.** All three of the reviewer's sub-failures are named in the document — wayfinding (nothing in any hierarchy puts a composers section below one album), the heading entering a screen reader's heading list as *the title of an argument*, and *"What is actually there"* being a footnote that has been given a column. The limitation itself is genuine and worth surfacing, which the reviewer says and which §6.4 already held; it is the rendering that was wrong |
| **I-08** | MAJOR | The screen named "Requests" contains no requests in v0.1 and is the only route to indexer search — and two other screens have to signpost it | **Rebutted in part; the sub-caption applied — see §5.4.1.** The rename to "Indexer search" is **refused**: it renames the one structure §12 commits to keeping stable, ADR-0020 makes requests a pillar, and — decisively — **U-01's fix pays off most of the discovery cost the rename was buying**, because §17.4 rule 6 now routes the user here from the row that made them want it rather than from a guessed sidebar label. Applied: §17.5 requires the sub-caption to state what the screen is *in this milestone*, including the recent grabs |
| **I-09** | MAJOR | Services and Libraries appear both as top-level sidebar entries and as items inside a "Settings sections" nav, on the same screen, with two elements carrying `aria-current` at once | **Applied**, taking the reviewer's own stronger option, which is the one the documents already argued for. §17.3: **the in-page nav is labelled `Settings` and lists only `General · Tags · UI`**, with a line above it saying Services and Libraries are top-level; two elements never carry `aria-current` simultaneously. The sidebar half is applied with it — `Status` and `Backup` are System sub-items indented under an expanded System, which §8.1's row-budget arithmetic already assumed, and drawing them as top-level rows is what took the sidebar to fifteen against a budget of eight fixed entries |
| **I-10** | MAJOR | Seven renderings of "there is nothing here", four on one screen, with Search rendering *the same fact* as `no file` in one column and `n/a` in the next | **Applied.** §9.1: **three words, used consistently, and nothing else** — `—`, `Not configured`, `Not applicable` — with `Never` kept as a real answer. The reviewer's diagnosis is the rule's reasoning: three distinct concepts are in play (*nothing is wrong* / *not configured* / *not applicable to this row*) and the vocabulary did not separate them, while `not configured` was **already a first-class status with its own token** and was then not used for it |
| **I-11** | MAJOR | The Corrections table's `Verb` column is named after the schema's discriminator, and `field` is not a verb | **Applied.** §17.8: the column becomes **`Correction`** and the values go to the past tense — `Excluded`, `Included`, `Re-linked`, `Field overridden` — because every row is a thing that already happened. The reviewer's two observations both land: nobody has thought *"I performed a verb"*, and one of four values not being a verb is what happens when a column is generated from an enum's column name. The neighbouring `What it does` column is kept and credited; the ⚠️ that in v0.1 the four verbs are defined nowhere (that panel is v0.3) is recorded |
| **I-12** | MAJOR | A library's `Kind` enum offers five schema values that appear nowhere else in the interface, and Music's answer is `artist` | **Applied.** §17.8: **label the control in the product's vocabulary** — `Movies · TV · Music · Books · Comics` — let the schema value be the value, and add one help line under Books about the format filter. The reviewer's point that §17.2's `(kind, formats)` mapping *"has to be written down or the sidebar cannot be built"* was written down **for the implementer and never surfaced to the user** is the argument, and it is the reason this is MAJOR: the user is asked to answer in a vocabulary they have never seen. ADR-0033 interacts and is consistent: `person` is not offered, because a library of authors is not a thing |
| **I-13** | MAJOR | `no work identity` is schema jargon on a search result row, and the useful sentence exists in one state and not in the one where the chip first appears | **Applied.** §6.4: the chip reads **`matched by title`** — a phrase every \*Arr's manual-import screen has already taught this audience — and **the sub-line travels with it everywhere the chip appears**, not only where it was remembered. §17.3 carries the same replacement in its State-column list. The reviewer's framing is exact: `work` is a table in §6.1, and *"no work identity"* is a phrase that exists only inside UsArr |
| **I-14** | MAJOR | The `Peers` column renders `41 / 9` with no legend, no unit and no accessible expansion — and prose two paragraphs below calls the same data "seeders" | **Applied to the documents; the markup assigned.** §9.1: **keep the ecosystem-verbatim header and make the cell self-describing.** The reviewer is right on both halves — `Peers` is Prowlarr's word and must stay, and Prowlarr itself renders the cell with a title attribute expanding it while UsArr rendered bare text. The prose/header divergence four hundred pixels apart is recorded for the mockup pass |
| **I-15** | MINOR | `Category` is singular on a multi-select whose sibling `Indexers` is plural, and Prowlarr's own label is `Categories` | **Assigned to the mockup pass.** Both an ecosystem mismatch and an internal inconsistency inside one form; §13's ecosystem-verbatim rule already covers the class |
| **I-16** | MINOR | `Grab release(s)` ships a parenthesised plural in a UI that knows the count | **Applied to the documents; the mockup assigned.** §13: **no parenthesised plural where the count is known** — and the button is disabled at zero selections, so it always knows. The reviewer's own defence of the string is answered in the rule: keeping Prowlarr's verb `Grab` is what buys the familiarity; the `(s)` buys nothing and does not translate |
| **I-17** | MINOR | Four labels for "test", four for "delete", three for "retry", two for "add a service" | **Applied to the documents; the mockup assigned.** §13: **one label per action across the whole product**, with the reviewer's inventory as the evidence and their one concession kept (`Add Prowlarr` is defensible, because it names the specific thing needed). `Confirm your password` as a *button* label — a task description where a verb belongs — is called out separately |
| **I-18** | MINOR | The copy is consistently en-GB against an ecosystem that is consistently en-US | **Applied as a recorded decision, which is what the finding asks for.** §13 states the locale — **en-GB** — with the exception the reviewer specifies: **a string quoted verbatim from an \*Arr keeps the \*Arr's spelling**. The reviewer is right that it is the owner's call and that en-GB is a perfectly good answer; what was wrong was that it was drift rather than a decision, and that `catalogue source` — one of the product's two coined terms — is the visible edge of it. 📌 Flagged for Joe to confirm; §5.6 item 3 |
| **I-19** | MINOR | `catalogue source` is abbreviated inconsistently, and the Services `Libraries` column drops both nouns on the screen where a user first meets the concept | **Applied.** §17.3: the column reads *"TV — catalogue source, request destination"* / *"Music — catalogue source; no request destination"*. Recorded with the reviewer's verdict on the term itself, which is worth defending: `catalogue source` is *"the strongest naming decision in the design"* and better than anything the reviewer could propose |
| **I-20** | MINOR | `Request destination` is 4/6 identical in value and 4/6 divergent in explanation — §9.1's own "a column whose value is identical is not data" rule firing on a column that survived it | **Applied.** §17.8: **the shared fact is stated once above the table, the cells read `None`, and only the Ebooks row's Readarr note survives as a per-row footnote** — which is exactly the split the reviewer proposes, and for their reason: the Readarr note is a real, dated, specific fact a user cannot infer, and the Comics cell was sixty-two words of competitive analysis (I-06) |
| **I-21** | MAJOR | At 390 px the whole sidebar is `display: none`, so the active library scope is unstated everywhere except Search — there is no navigation landmark in the tree at all until the drawer is opened | **Applied.** §8.1 point 5, §17.2 and §17.7: **the chip hoists into the top bar whenever the scope is not "all libraries"**. The reviewer's scenario is the one in the document, because it is the design's own stated case: a scope set on a desktop travels to the phone in the same `?lib=` URL, and §17.1 singles out that device precisely because *"that is where a request gets made from the sofa"*. Search's inline restatement is credited as exactly right and is generalised to every scoped surface (U-14) |
| **I-22** | MAJOR | The scope chip has three grammars, and the zero case is reachable with no state drawn for it | **Applied**, both halves. §8.1 point 2: **one grammar** — `All libraries (8)` / `2 of 8 libraries` / `No libraries selected` — because the parenthesis convention flipped meaning between the first form and the old `None (0 of 8)`. The zero case becomes the named `scope-empty` state (§10, §17.7, U-03). The reviewer's observation that the machinery is already there — the `Everything` checkbox goes indeterminate correctly — and only the terminal state is missing, is what makes this cheap |
| **I-23** | MINOR | The `Show all N` links are the best copy in the product and should be the template; the one gap is `Show all 23 tracks`, which omits the album | **Applied.** §17.4 rule 3 now states the generalisation: **a "show all" link names its count, its type and its parent scope**, so it survives being read out of context in a links list, which a bare `Show all →` does not. Recorded as written, because a review that only attacks is not calibrated and this one said so itself |
| **I-24** | MINOR | No skip link, and 21 tab stops before content on every screen | **Applied**, with U-13. §11's floors table gains SC 2.4.1 and §13 the grep line, carrying the reviewer's own honest qualification: landmarks are an accepted technique so this is not a strict failure, and keyboard-only sighted users have nothing |
| **I-25** | MINOR | `<nav aria-label="Level">` names the internal component, so landmark lists show "Level, navigation" | **Applied to the documents; the rename assigned.** §13: **an accessible name describes the pattern, not the component's internal name.** It is a breadcrumb |
| **I-26** | MINOR | `aria-current="page"` is used on search filter chips, so two elements claim "current page" at once and all seven chips point at one `href` | **Applied to the documents; the markup assigned.** §13: **`aria-current="page"` marks a destination, never a filter, and never two elements at once** |
| **I-27** | MAJOR | The search type-chip row has no accessible name and no role, and duplicates the six group headings verbatim | **Applied to the documents; the markup assigned.** §13: **a group of related controls carries a role and an accessible name.** The reviewer's description of the experience is the reason it is MAJOR rather than MINOR: seven bare links with no statement of what they are or what activating one does, immediately before six headings carrying **the same seven strings in the same order** |
| **I-28** | MAJOR | Every empty state's title is bold text, not a heading — so on the first-run screen a heading-list navigator finds one heading, "Home" | **Applied.** §9.6 constraint 1 now requires the empty-state heading to be an **`<h2>`** as well as `--text-lg`. Recorded with the reviewer's calibration: the heading hierarchy is otherwise clean and this was the one gap in it |
| **I-29** | MAJOR | The `Radarr 4K` row's `Problem` cell begins *"Nothing is wrong."* — inverting the meaning of the one column a user scans for what is broken | **Applied.** §17.3's `Problem` column is narrowed to *the upstream error and nothing else*, and the document says outright that a cell denying there is a problem inverts the column, and that `not configured` already exists as a state with its own token to say this properly. Filed separately from I-06 by the reviewer because of *where* it is, which is the right call and is why it earns its own sentence in §17.3 |
| **I-30** | MINOR | *"Deleting it is one click and it is yours to make."* — the first sentence informs, the second persuades | **Assigned to the mockup pass** (end at *"…the tags on its 41 works."*; the `Delete library` button says the rest). §1.4's copy rules already forbid the class |
| **I-31** | MINOR | *"One source, so nothing to disagree"* is ungrammatical | **Assigned to the mockup pass** |
| **I-32** | MINOR | *"Where Audiobookshelf will find it"* is a heading that reads as a question with no question mark | **Assigned to the mockup pass** (`Watched folder`, help line unchanged) |
| **I-33** | MINOR | An `Accept` column header sits over a cell reading `declined` | **Applied.** §17.8: the column is headed **`Decision`** — accepted rows keep their checkbox, the declined row keeps its word, and a header no longer contradicts its own cell |
| **I-34** | MINOR | Raw schema identifiers surface in running copy — `managed_by user`, `sort_title` — set in mono, so they look deliberate | **Applied to the documents; the mockup assigned.** §13: **no raw schema identifier in running copy** outside an explicitly-labelled Diagnostics panel, where identifiers are the content. The reviewer's own exemption is honoured: `?lib=ebooks` and `lib_9f3c1a7e` in Diagnostics are fine |
| **I-35** | MAJOR | Four duration formats ship, two of them in adjacent blocks on Home | **Applied.** §9.1: **two formats, chosen by magnitude** — `M:SS` for a single playable item under an hour, `H h MM m` for everything else — with the reviewer's refinement that a library-level total is prose rather than a column, so it reads `5,912 hours` |
| **I-36** | MAJOR | 87 bare `HH:MM` timestamps with no date, on data that can be a day old — including in a degraded banner whose number is its whole job | **Applied.** §9.1: **every user-facing timestamp carries the relative form, and past 24 hours it carries a date.** The reviewer's specific failure is the argument and it is quoted in effect: *"showing cached data from 11:47"* is identical whether the instance has been down for six minutes or twenty-two hours, and §17.7 itself says that number is the banner's whole job. Where the column is too narrow the relative form wins and the absolute goes in `title`, which is the reviewer's own resolution |
| **I-37** | MINOR | Three date formats ship — `08 Aug`, `12 August`, `27 June 2025` | **Applied.** §9.1: **one date format, `8 Aug 2026`**, no leading zero, always with the year |
| **I-38** | MINOR | The `Issues` column mixes a count, a gap list and a unit of a different kind — `11 · #7 missing` beside `1 volume` | **Applied** via §9.1's unit rule, with the reviewer's target recorded as the intended rendering: `11 issues · #7 missing` and `1 volume`, so the two are comparable as labelled quantities rather than as bare numbers. Their concession is kept: §6.1 is right that comics get a gap list rather than a fraction; the gap list and the volume count are simply not the same measurement |
| **I-39** | MINOR | The first-import progress claims final totals it cannot know, and calls comics by a third noun | **Applied**, with U-24. §17.2 states the source of the denominator once — *"Totals reported by each service."* — and requires **one noun per type across the banner, the block and the sidebar** |
| **I-40** | MINOR | `FLAC 44.1/16` carries no units | **Applied to the documents; the mockup assigned.** Covered by §9.1's unit rule. The reviewer's own calibration is kept: self-hosters will read it correctly, and it is filed because nothing else in the product uses an unlabelled compound number |
| **I-41** | MINOR | *"Sorted by Age ascending"* is ambiguous about which end is which, and Prowlarr does not say it in words at all | **Assigned to the mockup pass**, with the target copy recorded: *"Sorted by age, newest first."* The document half is adjacent and applied — §17.4 rule 2 now requires the toolbar to state the group order and to label the Sort control's scope (I-56) |
| **I-42** | MAJOR | `aria-rowcount` and `aria-rowindex` are entirely absent, on lists that are explicitly containment-skipped and paginated, against 643 `aria-colindex` declarations | **Applied, as an owner decision.** §11 requires both **wherever the rendered rows are a window onto a larger set**, which under ADR-0029 is every list — with the two reasons the design has already committed to (Load more holds a prefix by construction; `content-visibility: auto` skips off-screen row contents), 1-based indices over the **full** set, and `aria-rowcount="-1"` for a genuinely unknown total. §13 carries the grep line. The reviewer's sentence is the one in the document: **a user is otherwise told "row 3 of 26" when the truth is "row 3 of 1,204"** — a confidently wrong number, arriving through the accessibility tree. Their point about `aria-colindex` being the attribute this markup does **not** need is recorded as an asymmetry, and it is kept rather than stripped, since it is harmless |
| **I-43** | MAJOR | At 390 px every cell announces its column name twice, because the generated stacked label lands inside the cell's accessible name | **Applied to the documents; the markup assigned.** §9.1: **the stacked label is a real element marked `aria-hidden`, never generated content.** The reviewer's second reason is carried too and is the one nobody would have found later: `data-label` duplicates the header string into an attribute **most translation pipelines will not pick up**, so the two drift on the first translation. Their credit for the visually-hidden header row is recorded — it is the right call, and it is why the `columnheader` nodes survive at all |
| **I-44** | MAJOR | Grabbing a release destroys focus — `document.activeElement` becomes `<body>` on both the success and the failure path, because the handler disables the button synchronously | **Applied to the documents; the JS assigned.** §11: **never disable the control that was just activated** — `aria-disabled`, changed label, still focusable, activations swallowed; and if it truly must be disabled, move focus deliberately first. The compounding failure the reviewer names is recorded: on the failure path this sends the user away from the very toast carrying the recovery action |
| **I-45** | MAJOR | The error toast is a `role="alert"` nested inside an `aria-live="polite"` container, contains interactive buttons, gets no focus, and sits at the end of the document | **Applied to the documents; the markup assigned.** §9.4 gains four structural rules covering all four sub-points: no nested live regions, no interactive control inside the alert (per the ARIA APG), focus moved to the recovery action when the error is the direct result of a user action, and every toast naming the object it is about. The reviewer's own credit is kept — **it does not auto-dismiss, which is correct** — and so is their conclusion that the information is not lost because the row chip holds it; it is the *recovery action* that was unreachable |
| **I-46** | MAJOR | `Escape` does not return focus to the row when the row's action is a form control — which on Requests is every row's first action | **Applied to the documents; the JS assigned.** §11: **`Escape` is handled before the form-control bail-out, not after it.** The reviewer verified that the bail-out itself is correct and is the fix §11 demanded in the previous round, which is exactly why this is worth stating: the correct fix acquired a second, silent effect, and on Requests it leaves a checkbox with no arrow key and no `Escape` that gets back out — only Tab |
| **I-47** | MAJOR | The `?` shortcut sheet is not implemented, which removes the documented discovery route for the SC 2.1.4 off switch | **Assigned to the mockup pass; the document requirement is already correct and needs no change.** §11 requirement 3 already says `?` stays unconditionally *because the sheet it opens is where the toggle is discovered*. The reviewer's verification that the off switch itself **exists, persists and is honoured** — so SC 2.1.4 is technically satisfied — is recorded, along with their sharper point: the toggle currently lives five clicks deep inside the nav that I-09 shows is itself ambiguous, and **a compliance mechanism nobody can find is a compliance mechanism on paper** |
| **I-48** | MAJOR | The `9 of 9 indexers responded` live region announces a bare number, because the inner span is the region and is not atomic — and it is nested inside a `role="status"` | **Applied to the documents; the markup assigned.** §13: **a live region is atomic and carries the whole sentence**, is never nested inside another live region, and is never inside the control whose accessible name it is (which folds in I-52). The reviewer's note that this is the one genuinely dynamic element in the product, carrying two overlapping regions, is why it is MAJOR |
| **I-49** | MAJOR | The grab window is a countdown that never announces and never ticks, so the promise *"an expired release is never offered as grabbable"* is aspirational | **Applied.** §17.5: **the countdown lives in a `role="status"` that updates at 5, 2 and 1 minutes remaining** — not every minute, which is the reviewer's own restraint and is right — **and at zero the grab controls go `aria-disabled` with a row-level note and the screen offers `Search again`.** The document now makes the promise true rather than stated. The `expired` state is credited as well written; nothing transitioned the user into it |
| **I-50** | MINOR | Heading levels skip H2 → H4 on Libraries → edit, and "Identity" appears twice with no way to tell them apart | **Applied to the documents; the markup assigned.** §13: **no skipped heading levels and no two headings with the same text in one document.** The reviewer's rename is recorded as the target — the H4 becomes *"Identity matching"*, which is what it actually reports |
| **I-51** | MINOR | The dialog does not return focus to its invoker on `Escape` | **Assigned to the mockup pass.** §9.4 already requires it explicitly (*"focus returned to the invoking element on close"*), so this is the artefact failing a rule the document holds. The reviewer's list of everything the dialog gets right — native `<dialog>`, `modal=true` in the tree, `aria-labelledby`, Save disabled until the test passes, a GOV.UK-shaped error summary with a visually-hidden `Error:` prefix — is recorded, because it is the pattern the rest of the product should copy |
| **I-52** | MINOR | The scope button has `aria-expanded` and `aria-controls` but no `aria-haspopup`, and its accessible name is itself a live region, so most AT will say it twice | **Applied to the documents; the markup assigned.** Covered by §13's live-region rule (with I-48), plus `aria-haspopup="dialog"`. The reviewer's verification that the disclosure pattern is otherwise correct — `Escape` closes and returns focus, `Everything` goes indeterminate at 7 of 8 — is recorded |
| **I-53** | MINOR | The single-key guard is simultaneously too broad and too narrow — and the version that is too narrow is **the exact guard this design document prescribes** | **Applied, and it corrects this repository's own prescription.** §11 now carries the corrected guard with both measured failures named: focus on a row-select **checkbox** suppresses `/`, so pressing `/` to reach search silently does nothing; and focus on a `<button>` does **not** suppress `l`, which is **the precise bug §11 names as its own motivating example** (*"so `l` fires with focus on 'Add library'"*). Round-4 P-11 prescribed that guard and it did not fix the case it was written for. Measured, not reasoned — which is why it stands |
| **I-54** | MINOR | The overlay sidebar below 900 px does not trap focus and is not marked modal, so tabbing past the last nav item lands on content the user cannot see | **Applied.** §9.4: **an overlay that covers the content is modal, or it is not an overlay** — trap focus and mark it `role="dialog" aria-modal="true"`, or apply `inert` to `main`. Stated as a rule for any future drawer rather than as a fix to one |
| **I-55** | MAJOR | A linked work is counted in exactly one group, so clicking **Movies 3** excludes *Dune* (2021) and nothing on screen says why | **Applied.** §17.4 rule 4 gains the footer line: *"1 more film is on a linked row in the **Ebooks** group: Dune (2021). — [Show it]"*. The reviewer verified the arithmetic first (`14+9+3+2+2+1 = 31`, matching the `All 31` chip, no double counting) and credits the rule as correctly implemented and a real achievement — the finding is purely its unaddressed IA consequence, and the data is already there, since it is what renders the `also film, 2021` chip |
| **I-56** | MAJOR | Group order is unexplained and reads as descending-by-count, which is not the rule; and the `Sort` control's scope is unstated, with three available readings | **Applied.** §17.4 rule 2: **the toolbar states the group order in four words** (*"Groups ordered by best match."*) **and the sort control is labelled with its scope** (`Sort rows within each group:`). Both fit the toolbar and neither costs a row, which is the reviewer's own constraint. Recorded that the only explanation of either shipped inside a `mockup`-tagged note, which by definition does not ship |
| **I-57** | MINOR | The `library` collapse rule fires correctly, and the collapsed line is where I-05 bites | **Recorded, no separate change required** — verified positive, and the string is fixed by I-05. Worth keeping in the record: four of six groups correctly drop the `Library` column while Comics keeps both `Library` and `Instance` because it genuinely varies, which is round-4 P-14's rule working exactly as specified |
| **I-58** | MINOR | Screen-reader group counts are correct and well-formed | **Recorded, no change required** — a verified positive. Each group is an `<h2>` plus a count, each list a named `role="table"`, each truncated group ending in a `Show all N …` link, so a heading-list navigator gets the six groups and their sizes in one pass. The reviewer's assessment is worth preserving: *"this is the part of the six-type search that most needed to work and it does"* |
| **I-59** | MAJOR | The scope popover has no maximum height, no internal scroll and no filter, and breaks at roughly twenty libraries | **Applied.** §8.1 point 4: **`max-height: min(60vh, 420px)` with `overflow-y: auto`, a filter input above ~12 entries, and `Select all` / `Select none`** — all three of which Navidrome's `LibrarySelector` and Jellyfin's user-view picker already do. The measurements are carried (~310 px at nine, **551 px at sixteen**, bottom of a 900 px viewport at ~24 and of a 768 px laptop at ~19), and so is the reason it is MAJOR: **this is the one control §17.2 designates as *the* answer to unbounded cardinality**, so a control that does not survive them defeats its own justification |
| **I-60** | MINOR | Long titles, CJK titles and self-chosen service names all survive — verified; the residual risks are the popover and the Services `Libraries` column | **Applied**, both halves. The positive is recorded because it is load-bearing evidence that `min-height` rather than `height`, `white-space: normal` and declared column widths are doing their jobs — an 85-character Latin title and a CJK title both wrap cleanly at 1440 and 390 with no document scroll. The residual: §9.1 gains **a cell that renders one chip per related object caps at three plus `+N more`**, for the case of one Audiobookshelf feeding fifteen libraries. The popover half is I-59 |
| **I-61** | MINOR | The `2-up` phone layout suppresses the `Have` column label, and Music's value then has no unit anchor | **Recorded, resolved by I-01** — as the reviewer says themselves. Once the tick carries the word `Have`, the phone row reads `Have · 4,118 albums · 51,204 tracks`. The suppression is otherwise the right call, because the other rows' values are self-describing |

---

## 5.4 Rebuttals and partial disagreements

Recorded because a finding that is quietly ignored comes back. **Nothing is rebutted in whole; three
findings are rebutted in part.**

### 5.4.1 I-08 — renaming the `Requests` sidebar entry to `Indexer search` for v0.1 is rebutted. Its diagnosis and its sub-caption fix are applied.

**What the finding gets right, and it is not small.** In v0.1 the screen has no request list, no
approval queue and no `pending → approved → routed → available`; the sidebar label promises a thing
the screen does not have and hides the thing it does. The reviewer's supporting evidence is real and
checkable: two *other* screens have to spell out *"Search indexers instead"* as a call to action,
**because "Requests" does not tell anyone that indexer search lives there** — and a label that needs
a signpost pointing at it from two other screens is doing badly.

**Why the rename does not happen.** Three reasons, in order of weight.

1. **U-01 pays off most of the cost the rename was buying.** §17.4 rule 6 now puts a
   `Search indexers →` action on **every result row whose availability is incomplete**, so the user
   reaches this screen *from the row that made them want it*, with the query pre-filled — not by
   scanning a sidebar and inferring which label hides indexer search. The finding was written against
   a product where the only route was the sidebar. That product no longer exists, and the two
   remaining signposts are then reinforcement rather than compensation.
2. **The sidebar is the one structure this design commits to keeping stable.** DESIGN-DIRECTION §12
   fixes the eventual set — Home · types · Search · Requests · Calendar · Stats · Services ·
   Settings · System — precisely so that arriving features are data changes rather than layout
   changes, and §8.1's row budget is computed against it. A top-level entry that renames itself
   between v0.1 and v0.2 is the one kind of churn that structure exists to prevent, and it lands on
   the users most likely to have muscle memory: the ones who used v0.1.
3. **ADR-0020 makes requests a pillar, not a v0.2 feature that happens to arrive later.** Naming the
   screen after the mechanism it currently uses (indexer search) rather than after the intent it
   serves (getting something you do not have) inverts that, and it would have to be inverted back.

**What is applied instead, and it is the honest half of the finding:** §17.5 requires the screen's
sub-caption to state what the screen is **in this milestone** — free-text indexer search through
Prowlarr, a grab sent to Prowlarr's own download client, and your recent grabs. The label is ahead of
its content by one milestone and the screen says so in its own first line, which is the same
treatment §16 gives every other capability that has not arrived.

### 5.4.2 V-06 — the rule is applied; the claim that `#8a5300` "sits inside the warm neutral ramp" is rebutted on measurement.

**What is applied, without argument, because it is the finding's real insight.** `--status-warn`
light was chosen to clear 4.5:1 as **text**, and forcing an amber dark enough to be readable body
text is what took the amber out of it. But the ⚠ glyph is a **non-text graphic**, bound by SC 1.4.11
at 3:1 rather than by 1.4.3 at 4.5:1 — so **the word and the glyph are bound by different criteria
and may carry different values**, which §3.2 now says. That is a rule the document did not have.

**What is rebutted.** The finding describes `#8a5300` as sitting *inside* the warm neutral ramp,
reading as *"slightly redder body text"*, with *"the colour doing none"* of the work and the glyph
doing all of it. Computed from the shipped hex literals: ΔE76 from `--fg-muted #5a534a` is **46.3**
in light, against **54.2** for the equivalent dark pair (`#e0a33a` from `#b0a89b`). That is a gap
about **15% narrower**, not a collapse into the ramp. The *direction* is right — light warn is
genuinely the weaker of the two, for exactly the reason the finding identifies — and the *magnitude*
is overstated in a way that would justify a larger change than the evidence supports.

**And the finding's own two candidate values fail this document's floor**, which is why the value is
escalated rather than swapped: `#c98a00` measures **2.80:1** against the light ground and `#b8860b`
**3.09:1**, so the first misses even the WCAG 3:1 minimum and the second misses §11's stated 3.2:1
target. `#a9700a` (**3.98** / **3.68**) clears both and is recorded in `tokens.css` as the candidate,
**not landed**.

🔍 **Both instruments are crude, in opposite directions, and this is not settled by arithmetic.**
ΔE76 is a poor model of perceived difference at small sizes and low chroma; the reviewer's instrument
was a judgement on rendered pixels at thumbnail scale, which is a legitimate test that ΔE does not
replace — and the reviewer was looking at DejaVu Sans rather than Plex, which changes the amount of
ink a glyph puts down. Neither settles it alone. **Joe looks at it, on both themes, at thumbnail
scale, after the font loads.** §5.6 item 2.

### 5.4.3 U-32 — "every problem is stated twice, and three times on a stale Home" is applied at two and rebutted at three.

**Applied:** stating a failure in the row's `Problem` cell **with its Action button** and again 900 px
below in the `System status` list **with the same title and the same Action button** gives one fix
two places to be pressed, which is a genuine defect. §17.3 now requires the roll-up to **link to the
row** rather than duplicate its action. The roll-up itself stays: it is Sonarr's shape, DESIGN-
DIRECTION §10 cites that health panel as the model, and a summary of what is wrong is the reason the
screen exists.

**Rebutted:** the third appearance — the sidebar severity badge — is not a duplicate. It is a
**count with no action and no message**, it is §8.2's explicit design, and its whole job is to be
visible from a screen the user is not on. Counting it as a third statement of the problem would make
the finding's remedy delete the one affordance that gets the user to the screen in the first place.
The correct count is two, and two is now one.

---

## 5.5 What this round found that the reviewers did not

Three things surfaced while applying the findings, recorded because they are the same class of defect
the reviews exist to catch.

1. **Round-4 §4.6 item 6 is closed, and closing it turned up a column name that asserted the wrong
   kind for four of six media types.** Joe decided the `person` kind, so **[ADR-0033](./DECISIONS.md#adr-0033)**
   lands: `work.kind` gains `person` with `kind_byte` 13, excluded from the media-type navigation
   enum, from the Tier 1 prefix index and from the FTS corpus. Applying it exposed that
   `work_credit.artist_work_id` — a column name, in the one migration that can never be edited —
   **asserts that every credit points at a music artist**, which is false for books and comics by
   ADR-0031's own role list. It is renamed **`creator_work_id`**, with the rule for which kind it
   points at written down (`artist` when a connected service models the creator as a top-level
   catalogue entity in its own right; `person` otherwise) and marked 🔍, since that rule is inference
   from how the sources model their data rather than a citation. Two consequences are carried rather
   than tidied: **"find everything by this author" is unanswered in v0.1**, and **a human who is both
   a musician and an author is two rows**.
2. **`kind_byte` encodes the *remote* kind, not `work.kind`, and nothing said so.** The map in
   `reference/gateway.md` §3 already carried `author` (10) and `file` (11), neither of which is a
   `work.kind` — so an implementer reading §5.3 would have assumed a one-to-one mapping that has not
   held since the map was written. It is stated now, in both places, as part of allocating byte 13:
   remote `author` and remote `person` both resolve to `work.kind = 'person'`.
3. **`--space-9` lost its only documented consumer when the empty state lost its block.** The token's
   comment read *"empty-state block spacing — the only place this is used"*, and §9.6 now puts the
   empty state in the content flow with no block of its own. The token is kept and its comment
   corrected to say it has **no current consumer and must not acquire one without a reason** —
   recorded rather than silently repurposed, because an unowned spacing step is how uniform padding
   (§1.3's own named tell) gets back in.

---

## 5.6 What still needs Joe

R4 items 1–5 and 7 are unchanged and still open. **R4 item 6 — whether `work.kind` gains a `person`
member — is CLOSED**, decided by the owner on 2026-08-16 and applied as ADR-0033. Four additions from
this round:

| # | Decision | Why it cannot be settled below him |
|---|---|---|
| ~~**8**~~ | ~~**The light-theme warning glyph's value**~~ (V-06, §5.4.2). **CLOSED 2026-08-16 by the owner — SW-05.** He looked at it and said the light value was *"kind of eh… going more orange would be better for warnings"*, which is exactly the judgement this row said only he could make. The escalated candidate `#a9700a` is **rejected on measurement** (3.28:1 on `--selected`, so never a text value); `#a44c00` / `#fb9349` ship, both at Lab hue ≈58° against CSS `orange`'s 59.6°, and **the glyph split is not exercised** because the word clears 4.5:1 on all five grounds | — |
| **9** | **Protocol chips are now colourless** (V-10). `--protocol-torrent` / `--protocol-usenet` are deleted, because the torrent value was the same green as `--status-ok` (ΔE76 4.59 light, 3.09 dark) and §1.1 forecloses the replacement hue | It is a change of *visible character*, not a defect fix — the ecosystem's green/cyan protocol cue is something a self-hoster reads without a legend, and giving it up is a taste call. Reversible in one line; the withdrawal note in `tokens.css` says how |
| **10** | **The first-run empty state's copy versus §9.6's one-sentence rule** (U-35). The composition is fixed; the *length* is not. The four-sentence first-run copy is genuinely good and the rule says one sentence, and one of the two is wrong | §9.6 states both possibilities and refuses to settle it by fiat. It is a question about whether teaching the product is a different job from reporting a state, which is the owner's call about his own product |
| **11** | **Confirm en-GB as the recorded UI locale** (I-18), with the stated exception that a string quoted verbatim from an \*Arr keeps the \*Arr's spelling | The whole corpus already follows it and the reviewer is right that it should be a decision rather than drift. It is a one-word confirmation, and it interacts with every borrowed string |

**And one thing that is not a decision but is a prerequisite for two of them:** ⚠️ **the IBM Plex
choice has never been seen.** The prototype ships no `@font-face` and resolves to DejaVu Sans and
Liberation Mono — measured by a canvas advance-width probe, since `document.fonts` reports Plex
"available" as a false positive. Every density judgement in this round is therefore *conservative*
(DejaVu is ~24% wider, so real Plex would reduce the wrapping, not increase it), every hierarchy
judgement is unaffected (size ratio, weight and colour are face-independent), and **the anti-Inter
argument's payload has never arrived**. §4.1 now records what would validate it: the subsetted WOFF2
faces actually loaded, a probe confirming the *rendered* family, and the density and hierarchy
screens re-shot side by side against the system-stack capture. A concurrent pass is attempting the
wiring; whatever its outcome, the claim stays marked unvalidated until someone has looked at it.

**Two limits on this round's evidence, carried rather than tidied away.** The IA reviewer **could not
verify `aria-colindex` / `aria-colcount` from the CDP accessibility tree**, because Chrome exposes
neither column nor row indices as node properties — established against a purpose-built control page
holding a native `<table>` beside an identical ARIA grid. **That is a tooling limitation and not a
defect**, absence from that tree is evidence of nothing, and the reviewer correctly declined to
report a finding; verifying the attribute family needs a real screen reader. And the craft reviewer's
reference screenshots were fetched from the Navidrome repository rather than from its live demo,
which was unreachable through the agent proxy — so the comparison is against a canonical published
screenshot rather than against a running instance.

---

# Final consistency sweep — findings with no reviewer behind them

**Date:** 2026-08-16. **Branch:** `claude/hearth-thread-vn9w7u`. Not a review round. These were
turned up by a last read of the design documents against the mockup that implements them, looking
for places where two files state opposite things. **No adversarial reviewer raised them** — they are
recorded here under their own heading rather than folded into a round, so the log does not attribute
to a review something a review did not find. Prefix `SW-` has not been used before, so nothing
collides.

**SW-02 to SW-04 came from a different place again, and it is worth naming: the implementation
thread on `claude/hearth-thread-kirqa7`**, which built the first working server and found three
things the Services specification did not cover. They are not reviewer findings and they are not
consistency findings; they are the design catching up with code that already exists. Every claim
below was **re-verified against that branch's source rather than taken from the relay**, and the
`Disposition` column records where the relayed version was wrong.

| # | Finding | Disposition |
|---|---|---|
| **SW-01** | **`font-display` is specified two ways.** `DESIGN-DIRECTION.md` §4.1 says to set `font-display: block` with a short block period, or `optional`, on the reasoning that over a LAN the font always wins the race so `swap` risks a visible reflow for no benefit. `mockups/fonts.css` set `swap` on all three `@font-face` rules **and argued for it in its own header** — *"text paints immediately in that fallback and reflows once, which is the right trade for an application whose first principle is perceived speed"* — which is not a difference of emphasis but the same trade decided the opposite way, in the reference implementation of the document that decides it | **Resolved by the owner, 2026-08-16, toward §4.1, which stands.** The `swap` argument assumes a fetch slow enough for the fallback paint to be worth something; on a LAN or tailnet, with the faces embedded in the binary, the font is effectively available immediately, so that benefit never arrives while `swap`'s cost — a visible reflow — still does. All three declarations in `fonts.css` are now `block`, and the counter-argument in its header is removed rather than softened. Replacing it: a note that the prototype **inlines the faces as base64 data URIs, so there is no fetch, no race and no block period to observe** — the property is unobservable in the mockup and is set only to match what the shipped product should do. The two prose descriptions that restated the old value as fact are corrected with it (`mockups/usarr.css`'s type comment, `mockups/README.md`'s font paragraph). Nothing about the degradation story changes: the system stack stays in the same `font-family` declaration, so a blocked load still falls back to the stack the design was reviewed against |
| **SW-02** | **`url_base` is a field the Services screen never had.** `service_instance.url_base` has been a column distinct from `base_url` since migration `00001_initial.sql`; `internal/httpapi/services.go` accepts `url_base` on `POST /api/v1/services` (`createServiceRequest.URLBase`, line 110), on `PATCH /api/v1/services/{id}` (`updateServiceRequest.URLBase`, line 203) and on both connection-test bodies (`testRequestBody.URLBase`, line 349); and `cmd/usarr/services.go:180` builds the upstream client over `si.BaseURL + si.URLBase`. The screen exposed it in **one** place only, the inline settings form, **behind `Show advanced`**, and not in the add wizard at all — while the wizard's Base URL help text told the user to put the sub-path in the *other* field. A Prowlarr behind a reverse proxy at `https://host/prowlarr` is an ordinary self-hoster setup and could not be added | **Applied, and the relayed claim was right.** `ARCHITECTURE.md` §17.3.1 is new and specifies the field: label, placeholder, optionality, both forms, help text, and validation. Two things the relay did not mention and the code decides: the join is **plain string concatenation with no normalising**, so a missing leading slash produces an unparseable URL and a trailing slash a doubled separator — the form therefore **normalises on blur** rather than only complaining; and `crypto.NormalizeHostPort` **discards path, query, fragment and userinfo**, so `url_base` is provably outside what the credential is bound to. The mockup gains `d-urlbase` in the wizard, promotes `f-urlbase` out of `advanced`, and both help texts now say that changing it clears nothing. `DESIGN-DIRECTION.md` §9.3's "four fields" is corrected to four required plus one optional rather than to five, because the count is of answers the wizard demands |
| **SW-03** | **Changing a service's origin invalidates the stored credential, and the screen had no state for it.** The AAD binds the ciphertext to the normalised origin (`crypto.ServiceInstanceAAD` → `NormalizeOrigin`), so the envelope cannot be opened for another one. The API answers **`400`** with **`error: credential_reentry_required`** and `action: "Re-enter the API key"` | **Applied, and the relay was right on the code and wrong on the scope.** Status and code confirmed verbatim at `internal/httpapi/services.go:266` and asserted in `cmd/usarr/browserflow_test.go:527` and `cmd/usarr/e2e_test.go:102`. Three corrections. **(a)** The relay named only `PATCH`; `POST /api/v1/services/{id}/test` returns the same 400 (line 430), so the screen must handle it on Test as well as on Save. **(b)** `url_base` alone does **not** trigger it — the comparison is `NormalizeOrigin`, which discards the path — and that mattered enough to spec explicitly, because it is what makes SW-02's field a safe edit. **(c)** `ARCHITECTURE.md` §17.3 said the opposite of the code on one point: *"a path or scheme change alone does not count"*. **A scheme change does count**, deliberately — `derive.go` records that binding the scheme is what stops an `https`→`http` same-port downgrade from silently keeping a working credential and putting a full-admin key on the wire. That sentence is now corrected in place and the reason is written down. New §17.3.2 names the state, its copy, its field behaviour and its two exits; `DESIGN-DIRECTION.md` §10 gains a `credential-re-entry` row; the mockup gains a reachable `reentry` state |
| **SW-04** | **Every service write sits behind sudo mode, and a `403` there means "re-authenticate", not "you may not".** The window opens at sign-in and on each confirmation and closes after **5 minutes** (`store.SudoWindow`, `internal/store/user.go:20`). The screen needs a prompt that retries the pending action, not a bare error | **Applied; the timeout and status are as relayed, the count is not.** `403` with `error: sudo_required` and `action: "Confirm your password"` confirmed at `internal/httpapi/auth.go:185`; the 5-minute window at `internal/store/user.go:20`, exercised by `TestSessionSudoWindow`. The relay said **four** writes; the route table at `internal/httpapi/server.go:208-214` gates **five** — create, update, delete, `POST /api/v1/services/test` and `POST /api/v1/services/{id}/test`. Both test endpoints are gated because a test is what sends a credential somewhere, and a screen that expected four would meet an unhandled 403 on its Test button. The distinguishing detail the screen actually needs, which the relay did not carry: the error body is `{"error", "message", "action"}` — the code is on **`error`**, not on a field called `code` — and **two other 403s exist**, `forbidden` (a disabled account, no retry possible) and `csrf` (reload). New §17.3.3 tabulates all three and requires the screen to branch on `error` and never on the status alone. The mockup's existing `denied` state is kept and corrected: it cited `PUT /api/v1/services/3`, a method this API does not have |

**SW-05 to SW-09 came from a third place, and it is the most direct one there is: the owner, in his
own words, on 2026-08-16.** They are not reviewer findings, they are not consistency findings, and
they are not the design catching up with code. They are the owner looking at what exists and saying
what he wants — which is why each row quotes him rather than paraphrasing. Where a finding needed
*work* rather than *prose*, the work is what the disposition records.

| # | Finding | Disposition |
|---|---|---|
| **SW-05** | **The light-theme warning colour is a muddy brown and should be orange.** The owner: *"kind of eh… going more orange would be better for warnings."* `--status-warn` was `#8a5300` light / `#e0a33a` dark. This closes **round-5 item 8**, which escalated the same value to him and recorded `#a9700a` in `tokens.css` as a computed candidate | **Applied, and the escalated candidate is rejected on measurement rather than adopted.** `#a9700a` was never a *text* value: **3.98:1** on the page ground but **3.28:1** on `--selected #e7e3dd`, so it fails 4.5:1 on the ground that binds and could only ever have been a glyph token. What ships is **`#a44c00` light / `#fb9349` dark**, and both halves of the brief are met by one value rather than by a split. **Genuinely orange, measured:** `#a44c00` is CIE Lab hue **58.0°** at chroma **62.1** and `#fb9349` is **58.6°** at chroma **64.0**, against CSS `orange` `#ff7f00`'s **59.6°** — so the two themes are now the same hue at two lightnesses, where before they were a brown (70.4°) beside a gold (77.8°). **Readable as text on the worst of all five grounds:** 5.53 page · 5.11 surface · 4.84 hover · **4.55 selected** · 5.67 inset in light; 8.07 · 7.52 · 6.92 · **6.57** · 8.55 in dark. Dark holds chroma (64.0 vs 61.9) and worst-ground contrast (6.57 vs 6.63) where they were and moves only the hue. **The word/glyph split §3.2 grants is therefore NOT taken** — it stays documented and unexercised, because a second token buys chroma the word already has. Rejected en route and recorded so they are not retried: `#c98a00` (2.80), `#b8860b` (3.09), `#a9700a` (3.28), and `#b04300` — more chromatic and still clearing 4.50, but ΔE76 **18.5** from `--status-error` where `#a44c00` sits at **25.2**. `tokens.css`, `mockups/usarr.css` and DESIGN-DIRECTION §3.2 changed in one commit so they cannot drift, and `docs/design/check.mjs` now asserts the drift and the ratios on every run |
| **SW-06** | **The design rules should be checked by something runnable.** The owner: he is *"open to some kind of adversarial design guideline checking."* The machinery existed and was scattered — `mockups/selftest.mjs` held five rendered-DOM assertions, and the §13 ban sweep was a pipeline of greps **retyped by hand every review round**, rediscovering the same four false positives each time | **Applied as one entry point, `docs/design/check.mjs`; `selftest.mjs` is deleted and folded into it whole.** It covers the §13 ban list, token drift between `tokens.css` and the mockup's copy, contrast worst-of-five-grounds in both themes, overflow at 390/1280/1440/1680/1920 across every screen in every state at two scopes, row heights against **all three** density bands, availability accessible names, one tab stop per list, containment, the webfont, and the copy bans. **Two properties are the point of it.** *It prints what it checked, not only what failed* — 14 font-family declarations, 36 screen×state combinations per width, 1,214 rendered chrome strings, 49 list renderings — because a silent pass is indistinguishable from a glob that matched nothing. *And false positives are excluded **structurally**, never by name:* the icon bans are evaluated only over import specifiers and `<symbol id="i-…">`, so `ev.key === 'ArrowRight'` is out of scope rather than excused; the font bans only over whole family names inside `font-family` declarations, so "Internally" and "The Zone of Interest" never enter; the copy bans only over rendered text **outside any `<td>`**, because a cell is data and not the product's voice — which is safe only because the row-height check keeps prose out of cells, so the two rules hold each other up; and every source scan runs with comments stripped, so a rule cannot fire on its own documentation. **A fifth exclusion is derived from a document rather than declared**: §13's em-dash exception for wording ARCHITECTURE §17 fixes is read out of §17 at run time, which makes it a **copy-drift check** — and it caught one on its first run, the re-identification banner's `Not the same — remove and re-add` against §17.3's `Not the same instance — remove and re-add`. Documented as DESIGN-DIRECTION **§13.0**. ⚠️ It runs against the mockup because there is no `web/` implementation to run against; every DOM assertion is written against the rendered document rather than the mockup's markup idiom, so the port costs a widened file list and nothing else. **The `Makefile` target and the `DEVELOPMENT.md` entry are routed to the implementation thread, not written here** |
| **SW-07** | **Soulseek is a provider with several materialisations, and a request destination must not assume an \*Arr shape.** The owner: *"soulseek would be a provider (which could be materialized in soulseek or soulbeet or lidarr + slskd or just pure slskd)."* `FUTURE.md` §11 recorded slskd as *"a music request sink"* and asserted a seam — *"the queue's verb model can express a two-phase asynchronous operation"* — **without ever checking whether the v0.1 schema provides it** | **Rewritten in the owner's terms, and the seam was tested rather than re-asserted. The answer is split, and one third of it is a real finding.** ✅ **`library.sink_service_instance_id` holds**, for the right reason: it is a pointer to a `service_instance` and encodes *which instance*, never *what shape of conversation*. All four materialisations resolve to exactly one instance row — including *Lidarr + slskd*, where the destination is **Lidarr** and slskd is Lidarr's download client, invisible to UsArr as qBittorrent is. What decides a sink's abilities is the live capability probe, which `providers.md` §2 already forbids inferring from the service kind. **No widening needed.** ✅ **The provider interface holds, and the two-phase interface already ships**: `Requester.Add` returns `(remoteID, error)` and cannot express this, but **`Grabber` can and does** — `Releases(q) []Release` then `Grab(rel)`, in v0.1 for Prowlarr. Soulseek is a `Grabber`, not a `Requester`. ❌ **`write_queue` does not hold.** The verb vocabulary is a *comment*, not a `CHECK`, so a new verb is free; the candidate set needs a table later and no seam now. But **`state`'s `CHECK` has no value for "waiting for a human"** — `pending` would be picked up by a worker, and `verifying` means *"it might have landed"* and carries a **15-minute TTL** that would resolve a sleeping user's request to `unknown`. There is no legal state to park in, so the two-phase verb **cannot be expressed at all**, which is precisely what the old seam paragraph said must not happen. **It is in migration 0001**, and SQLite cannot `ALTER` a `CHECK`, so retrofitting it costs a migration, a table rebuild and a rewrite of `ix_wq_runnable`'s partial-index predicate — against **one string literal** if it is there from the start. **The seam to add: `awaiting_choice` in `write_queue.state`'s `CHECK`, deliberately excluded from `ix_wq_runnable`** so a row waiting on a human is never swept and never TTL'd. Nothing implements it, no verb produces it, no screen renders it — the seam ships, the feature does not. Written up in `FUTURE.md` §11.1; **the `reference/schema.md` §10 change is routed rather than made here**. ⚠️ **The finding stands; this row's remedy is SUPERSEDED by **WQ-03** and **WQ-05** in the seam audit below, which were dispositioned on `main` and win.** Two corrections. **(a)** *"`awaiting_choice` in migration 0001"* is withdrawn — `00001_initial.sql` is merged and `CLAUDE.md` forbids editing it. The premise offered here for overriding that rule, that acting now costs **one string literal** where acting later costs a full table rebuild, was **false**: the rebuild is **already mandatory**, because `write_queue.work_id`'s `work_id → work(id)` foreign key is dropped in 0001 and must be restored when `work` lands with library sync, and SQLite can add neither a foreign key nor a `CHECK` in place. Both changes ride the same 12-step rebuild and the second is free, so no window was closing and there was nothing to weigh against the rule. The `CHECK` value lands in the **library-sync migration**. **(b)** *"deliberately excluded from `ix_wq_runnable`"* is downgraded from a decision to an **open question with a lean** — the lean is still to exclude, but the predicate also serves the reconciliation guard and the call is being made at rebuild time with that code in view. `FUTURE.md` §11.1 is corrected on both points |
| **SW-08** | **The docs frame music as second-class, and the narrower thing that is actually true is not said where a user would meet it.** The owner's framing: the design is already modular — a library's catalogue source and its request destination are separate bindings — so music is Navidrome cataloguing plus Prowlarr free text now, with Lidarr as an optional destination once destinations ship. **Lidarr is deferred because no write-capable service ships in v0.1 at all**, not because music is second-class; LazyLibrarian and Mylar3 are in the same bucket. This closes **R4 item 3**, which asked whether Prowlarr-only remains an honest v0.1 story for music | **Applied, and R4 item 3 is CLOSED — the honest answer turned out to be narrower than the question.** ✅ **What is true and now stated plainly:** the indexers that actually carry music are largely **private and invite-only** — 403 of Prowlarr's 543 definitions are `type: private`, and Redacted, Orpheus and MyAnonaMouse are invite-only — so a stock Prowlarr with public indexers returns a **thin** list for music where the same search returns a full page for a film. A correct, working, healthy search returning one row is otherwise indistinguishable on screen from a broken one. **So it is said on the Requests screen, at the point of use**, not only in an ADR's consequences: `ARCHITECTURE.md` §17.6 now requires the empty and near-empty result states to be **scoped by media type**, with the music one naming the reason and offering the one action that changes it — adding a private indexer the user already has an account on. The mockup gains a reachable **`thinmusic`** state that says exactly that. ❌ **What was implied and is corrected:** `mockups/libraries.html`'s shared note read *"v0.1 connects no request destination for music, audiobooks, ebooks or comics"* and then, in the same paragraph, *"Movies and TV have a destination"* — which both singled out four types and contradicted its own next clause, since the \*Arr-backed request path is v0.2 and **no** type has a destination in v0.1. It now says that, and says that the deferral is of the **write half** rather than of the media type. §17.5 carries the same qualification as a bounded ⚠️ note: the free-text path requests music in v0.1 exactly as it requests films, and Radarr and Sonarr are destinations in v0.2 only because they are already sources. ⚠️ **One clause of this row was superseded by SW-10 and is corrected here rather than left standing:** it originally read *"Navidrome catalogues music in v0.1 exactly as Radarr catalogues films"*, which asserts the very thing the catalogue-source re-sequencing reverses. **No catalogue source ships in v0.1**, so music has no catalogue half to compare — and neither do audiobooks, ebooks or comics. The finding itself stands: music is not second-class, it is in the same position as the other three. **README's equivalent caveat is the implementation thread's file and is routed** |
| **SW-09** | **ADR-0032 picked the wrong one of Komga and Kavita.** The owner: *"actually atm my books and comics/manga are managed by kavita."* ADR-0032 cut Kavita to v0.2 and kept Komga in v0.1, on the reasoning that Kavita contributes the least identity value (paid-tier identifier fields), has the deepest hierarchy, and has no Series-level delta | **Applied as [ADR-0035](./DECISIONS.md#adr-0035), which reverses one member of ADR-0032 and leaves its shape intact.** The original reasoning is **not** retracted — every one of its three observations is still true — but it optimised for the cleaner API rather than for the install that exists, and building an adapter first for a service nobody can point at a real library is the wrong order. **The payment is preserved exactly**: one of the two still comes out, v0.1 still carries three non-\*Arr catalogue sources, and no media type loses its v0.1 source. Three consequences are handled rather than smoothed over. **(1) Identity gets weaker, and it is now the default path** — but the honest comparison is that ADR-0032's own consequence (3) records **Komga supplying no external identifiers at all**, so *comics has no strong-identity path in v0.1 under either choice*; the loss against the plan is near zero and the "not identified" state is now what the screen ordinarily looks like rather than an edge case. **(2) The day-one spike retargets to Kavita and its criterion is written down before it runs.** ✅ Re-verified from Kavita `main` on 2026-08-16 rather than taken from the relay: `SortField.LastModifiedDate = 3` exists (`API/DTOs/Filtering/SortField.cs`) and **`SeriesDto` carries no last-modified property at all** — its complete date set is `LatestReadDate`, `LastChapterAdded`, `Created`, `LastFolderScanned` (`API/DTOs/SeriesDto.cs`) — so Kavita can sort `POST /api/Series/all-v2` by a field it does not return, and a watermark that is absent from every row cannot be carried. **Two candidates are both sortable and present**, `LastChapterAdded` (misses metadata-only edits) and `CreatedDate` (new series only), and the spike's pass condition is stated as three falsifiable clauses. **It decides build *order*, not membership**: Kavita first if the watermark works, **Navidrome first if it does not** — which also de-risks v0.4, since that is the same service. The ADR deliberately does not pre-judge it, because guessing here is the exact mistake ADR-0032 correctly refused to make about Komga's `sort=lastModified,desc`. **(3) ADR-0030's flattening of Kavita's Volume is re-examined and confirmed**: it was never an argument from Kavita's scheduling, and a `volume` `work.kind` in migration 0001 for a single adapter is *more* expensive now that the adapter is v0.1, not less. **Two facts recorded where they were assumed:** the owner's stack is Navidrome + Audiobookshelf + Kavita on **x86-64 under Proxmox**, so v0.4's single-Navidrome assumption is now checked against a real install, and **§13's Raspberry Pi 5 is a conservative floor rather than the owner's machine** — noted in §13 so a Pi figure is never quoted as what he will see. **§16's roadmap rows are the implementation thread's and are routed** |
| **SW-10** | **The catalogue sources are re-sequenced out of v0.1 entirely, which changes what SW-09 was propagating.** The owner delegated the provider count — *"I'm fine with starting small… we can start with 2 services or 5"* — and the call taken is that **v0.1 is Sonarr, Radarr and Prowlarr, with no catalogue source at all**: the \*Arr library sync proves the replica thesis on real data first, and **Navidrome, Audiobookshelf and Kavita then arrive one at a time, after v0.1**. This arrived mid-pass, after a gatekeeper drift check had asked only for the Komga→Kavita swap (SW-09) to be propagated. ⚠️ **It was verified against `origin/main` before anything moved, and the verification matters:** at `8c4a33c`, §16 still reads *"Navidrome, Audiobookshelf and Komga move from v1.0 into the earliest milestone that can carry them"*, ADR-0032 is **Accepted** with no status note, and **ADR-0035 is not on main at all**. So this branch is ahead of the authoritative document on its own subject, and every claim written here is written as *what §16 will say*, never as what it says today | **Applied in this thread's files only, and deliberately not completed.** ✅ **Done.** **(1) [ADR-0035](./DECISIONS.md#adr-0035) is restated rather than retracted** — its choice of Kavita over Komga is a choice about *which* comics-and-books source is built and is untouched by *when*; a dated amendment at its head says the milestone moved under it, re-marks every "v0.1" in its body as "the milestone Kavita lands in", and re-points its watermark spike: the spike is **not wasted work**, it now orders the **post-v0.1 catalogue sequence** (Kavita first if `LastChapterAdded` carries a usable watermark, **Navidrome first if it does not**, which de-risks v0.4's OpenSubsonic target and matters *more* when the sequence is serial), and it is no longer a *day-one* item. **(2) ADR-0035 §1's rendering requirement is kept and re-milestoned**: §17 and the mockups must draw free-Kavita's null identifiers as the **ordinary** case **from the milestone Kavita lands in**, not in v0.1, which draws no comics library at all. **(3) ARCHITECTURE §17 is re-scoped** — §17.2 gains the rule that Home Block A renders **all six types in v0.1 with four in the per-type `unconfigured` state**, naming the service that will populate each; §17.8 gains a scope note that a v0.1 library binds to a Radarr or Sonarr container and that its media-server examples specify the mechanism for a later milestone; §17.3, §17.4 and §17.7 have their catalogue-source examples marked as not-reachable-in-v0.1 rather than silently left reading as v0.1 screens. **(4) The `matched by title` state is marked unreachable in v0.1** — Radarr and Sonarr carry TMDB and TVDB ids, so every v0.1 work resolves at the identifier tier — with ADR-0035 §1's honest comparison carried across intact: **Komga supplies no external identifiers at all**, so comics has no strong-identity path under either choice and only paid Kavita beats both. ❌ **A forbidden claim was found already in the tree and removed from two places**, neither written in this pass: ARCHITECTURE §17.5 and `mockups/requests.html` both asserted that **Navidrome catalogues music in v0.1 exactly as Radarr catalogues films**, which asserts precisely the thing being reversed. SW-08's own row carried it too and is corrected in place rather than left standing. The finding SW-08 recorded still holds — music is not second-class — but the true form is that **music, audiobooks, ebooks and comics are all in the same position**, and the deferral is of the catalogue half for all four alike. ⚠️ **Deliberately NOT done, and this is the honest part.** The owner then asked that *"mockups should probably draw up the fuller stacks"*, so the mockups must draw **both** installs behind a milestone-labelled install switcher — full stack (six populated types) as the default, because that is what the design is judged on, and the v0.1 install complete and reachable beside it. That is a second orthogonal axis across ~4,200 lines of markup and the state machine, two reconciling arithmetic universes, and roughly double the design check's state sweep. **It is not started**, and the partial one-install conversion begun before the change arrived was reverted rather than left half-applied. `DESIGN-DIRECTION.md` §8.4's wireframe is the one design-doc surface updated: it keeps the six populated rows, is **labelled as the full stack and explicitly not v0.1**, and its two wrong instance labels are fixed (Komga→Kavita per ADR-0035; Lidarr→Navidrome, since Lidarr is a v1.0 sink and never a music catalogue source). **The mockups themselves still draw a single unlabelled full stack with Komga in it** — which is the pre-existing state and matches ADR-0032 as merged, but is exactly where `CLAUDE.md`'s "no invented status" rule gets broken once the switcher lands without labels. **§16's roadmap rows, README and DEVELOPMENT are the implementation thread's and are routed, not touched.** ⚠️ **The deferred half of this row is CLOSED by SW-14 below**, which builds the switcher, draws both installs and reconciles their arithmetic; the *"still draws a single unlabelled full stack with Komga in it"* sentence above describes the state this row left behind, and is no longer true |
| **SW-11** | **Three checks in `docs/design/check.mjs` were passing without testing anything, and one could not have fired under any input.** Raised by the frontend thread and confirmed by direct execution here. Same family as the `content-visibility`-on-`<tr>` finding: **assert the effect, not the declaration** | **Applied, with the mechanism changed rather than the call sites patched.** ✅ **(1) The dead predicate.** The `text-align: center` rule exempted dialogs with `allow = (h) => /dialog|modal|toast/i.test(h.text)`, but `h.text` was `m[0]` — the matched declaration, literally `"text-align: center"`. The selector never reached the predicate, so **the exemption could not fire under any input**. It was latent only because nothing centred exists to exempt; the first legitimate centred dialog would have failed the rule, and the likely repair is deleting the rule rather than fixing the harness. **The fix is structural, not local:** an exemption is now **data matched against a named capture group** — `{group, match, why}` — and `rule()` **throws at startup** if the pattern declares no `(?<group>…)` of that name. A predicate that cannot see what it claims to test is no longer expressible, so the audit is a property of the design rather than a sweep somebody must remember to repeat. Each rule also now reports how many hits its exemption absorbed, so an exemption that stops firing is visible. ✅ **(2) Two checks were matching zero things and printing `ok`.** `border-radius` scanned only `border-radius: Npx` and the mockups write `border-radius: var(--radius-0)` everywhere — **0 declarations matched**. The transition-duration ceiling had the identical defect against `var(--dur-…)` — **0 matched**. Both ceilings actually live in the `--radius-*` and `--dur-*` token definitions, which neither pattern read. Widened to cover both positions: radius now finds **3** values, duration **15**. ✅ **(3) Every static check declares a floor and fails below it**, because a glob that matches nothing printed the same `ok` as a genuine pass — the check's most reassuring output was its least trustworthy, and a refactor that moves a file silently disarmed a rule. Floors are on the **corpus**, never on the violations. The count prints against the floor so the number is visible, not just the verdict. `skipLink()` was the sharpest shape — *"every page has a skip link"* is trivially true of zero pages — and the font-stack parse the next. ✅ **(4) `shift = rowH - 28` silently hardcoded a token.** Compact's `--row-h` is now read from the token and **asserted equal to the baseline `CEILING_COMPACT` is written against**, failing loudly on drift, because a miscomputed shift produces a number that looks like a measurement. **The `allow` audit, in full:** the file had exactly **one** `rule()` exemption and it was the dead one. The remaining predicates were each proved live by construction — the emoji rule's `filter` (excludes the 5 html files, 4 of 9 sources scanned), `banIcons()`'s `ALLOWED_IDS` (13 ids matched), and `banCopy()`'s §17 fixed-wording exemption (4 strings exempted on every run, visible in its own output). 🔍 **Recorded but not built, so nothing above fights it:** the check will grow a per-target descriptor (`sources`, a `render` generator, a `vocabulary` carrying icon identity and the list-primitive selector) so it can run against `web/src/**`, and the row-height ceilings will move from screen-named constants to semantic classes. Against the mockups `banIcons()`'s teeth are the `<symbol id="i-…">` allowlist, because the mockups have no icon imports at all; pointed at Svelte components the import arm carries the rule and `ALLOWED_IDS` will not match component names, so icon identity has to become part of that per-target vocabulary rather than a constant |
| **SW-12** | **`check.mjs` imported Playwright from an absolute path outside the repo, so `make design` was unrunnable anywhere but this container.** `import { chromium } from '/opt/node22/lib/node_modules/playwright/index.mjs'` resolves nowhere else, so the target died with a raw `ERR_MODULE_NOT_FOUND` past the Makefile guard whose whole job is to say something friendlier — and an out-of-tree import cannot be pinned, which the Makefile's own *"@latest is FORBIDDEN, pin everything"* rule forbids independently. The Makefile already documents this as reason (2) for keeping `make design` out of the gate | **Applied.** The import is a **bare specifier** now, resolved through a ladder that degrades honestly: `$PLAYWRIGHT_MODULE` override, then bare `playwright`, then `web/node_modules` via `createRequire` — needed because this file lives in `docs/design/` and ESM resolves a bare specifier by walking up from the *importing* file, which never reaches `web/node_modules` — then the npm global root, which is what this container has. If every candidate fails it prints one sentence naming the problem, the exact pinned install command, and the resolution attempts in order, then **exits 1** rather than throwing. Verified both ways: it passes here through the global-root candidate, and a copy with that candidate removed exits 1 with the friendly message. ⚠️ **The matching pinned devDependency is `web/package.json`, which is the frontend thread's file and is NOT touched** — the exact diff is routed instead: `"playwright": "1.56.1"` in `devDependencies`, exact, no caret. Until it lands the bare specifier resolves only via the fallbacks. The Makefile's reason (2) becomes stale once it does, and is routed with it |
| **SW-13** | **§13's hardware note was ~70% restatement of the section it sits in, and it preceded the paragraph it qualifies.** Raised by the code thread | **Applied, all three changes.** The restatement of *"the target is x86-64, the Pi 5 is design intent"* is **cut**, since §13 already says it. What is kept is the part that adds something: the enumeration of the three arguments that actually extrapolate to Pi-class hardware — DESIGN-DIRECTION §7.4's density-toggle cost, ADR-0022's Argon2id parameters and §7.2's streaming-import peak — plus the rule that **a Pi-derived figure must never be quoted as a measured number**. It **moves below** the *"Reference hardware: a Raspberry Pi 5"* paragraph, which is the thing it qualifies. And it now cites **`REVIEW-LOG.md` §287** for the hardware fact rather than ADR-0035, which is about Kavita versus Komga and carries the hardware only as a consequence bullet |
| **SW-14** | **The mockups drew one unlabelled install, and it was the wrong one twice over.** The owner, asked whether the mockups should show more than v0.1's three services: *"yeah, mockups should probably draw up the fuller stacks."* This is the half SW-10 deliberately left undone. Two separate defects sat underneath it: the single drawn stack contained **Komga**, which [ADR-0035](./DECISIONS.md#adr-0035) replaced with Kavita, and it carried **no label at all**, so six populated media types read as v0.1 to anyone who did not already know the roadmap — precisely the `CLAUDE.md` "no invented status" failure, arrived at by omission rather than by assertion | **Applied in full.** ✅ **Two installs behind one switcher, `full` by default.** `full` is Sonarr, Radarr, Prowlarr, Navidrome, Audiobookshelf and **Kavita**, all six types populated — the default, because six populated types is what the layout has to survive. `v01` is **Sonarr, Radarr and Prowlarr**: Movies and TV catalogued, and music, audiobooks, ebooks and comics present in **Home Block A** with a state, a cause, the service that will populate them and an action — not in the sidebar, because a nav entry cannot carry a cause (§17.2, DESIGN-DIRECTION rule 13, unchanged from SW-10 and preserved). ✅ **The label is by milestone, not by name.** The options read **`Full stack — a later milestone`** and **`v0.1 — Sonarr, Radarr, Prowlarr`**. "A later milestone" is the strongest true statement available: §16 sequences the catalogue sources one at a time after v0.1 and has **not** fixed which lands in which release, so naming v0.2 would invent the status this row exists to prevent. The switcher lives **inside the permanent mockup notice** rather than beside it in the product chrome — it is not a UsArr control and drawing it as one would fabricate a setting — and the notice's own sentence changes with the selection, so the labelled-mockup exception rule 13 grants stays true of whichever install is shown. ✅ **The v0.1 numbers are DERIVED, not invented.** Every figure is the full stack's own with the absent services' contribution removed, so the two reconcile row by row: services 8→5 rows, libraries 8→2, scope chip `All libraries (8)`→`(2)`, sidebar types 6→2, Home Block A 6 rows either way (2 counted + 4 sourceless), Block B 4→3 attention items — which then equals Services' own System-status count of 3 — and Block C 26→8 rows across 6 types→2. The sums that already held still hold: comics 553 = Kavita 512 + Manga 0 + orphan 41; Audiobookshelf 2,260 = 418 + 1,842; **TV 275 = Sonarr 214 + Sonarr Anime 61**, which is also what gives the v0.1 proposal flow its rename-to-merge demonstration. ⚠️ **One figure does not survive naive subtraction, and it is the finding worth keeping.** `Dune` is one linked work across the novel, the M4B and the 2021 film, rendered once in its best-scoring medium — Ebooks on the full stack, with the Movies group carrying a one-line pointer at it. Delete the Ebooks group and the row does **not** disappear: the same work's best available medium becomes the film, so it moves into Movies. Subtraction gives 4 results; the correct answer is **5** (Movies 4, TV 1), and the chips, the group count, the scoped state's excluded total and the posters grid all follow it. It was caught on a screenshot rather than by arithmetic, which is exactly why the numbers were re-derived from the rule rather than from the previous total. ✅ **`check.mjs` sweeps both installs**, and its floors are restated against the doubled corpus rather than left where the new reality satisfies them without trying — a floor that cannot fail is the silent pass the floors exist to convert into a failure. `CORPUS_FLOOR` 200k→480k; new floors on screen×state×install combinations (70), rendered rows per screen, `.avail` elements (165), list renderings (78) and chrome strings (2000). Two new checks: **1c** asserts the mechanism's invariant — `data-when` and `data-inst` never write `hidden` on the same element, because two writers and one attribute is a block that quietly reappears in a state it does not belong to — and **8b** asserts the switcher itself: it offers exactly the two installs the sweeps run, **both option labels name a milestone**, the page **loads in `full`**, all five screens actually render differently, and the notice matches the selection. All checks pass; verified in Chromium at 1440×900 and 390×844, both themes, both installs, five screens |
| **SW-15** | **Four defects found while reconciling the arithmetic, none of them introduced by this pass.** Recorded separately because attributing them to the switcher would hide that they were already shipping | **All four applied.** ✅ **(1) The Libraries table claimed a request destination that does not exist.** Movies read `Radarr · requests are queued, not lost` and TV read `Sonarr · HD-1080p`, directly under a paragraph on the same screen stating that no media type has a request destination and that the \*Arr-backed path is v0.2. The rows now read `None · Radarr, from v0.2` and `None · Sonarr, from v0.2`, and Services' `Libraries` column says `catalogue source; destination from v0.2` rather than asserting both halves today. ✅ **(2) `services.html` still carried the withdrawn Kavita `LibraryType 3 (Image)` claim.** It was removed from `ARCHITECTURE.md` and `DESIGN-DIRECTION.md` in the previous commit — re-checked against Kavita `main`, `LibraryType.cs` declares exactly `Manga=0`, `Comic=1`, `Book=2` and no `Image` member — but the mockup's copy of it was missed. The annex row is now **Komga** rather than Kavita and rests on Komga's own sourced behaviour: no library type, no comic/manga distinction beyond `ReadingDirection`, and no external identifier of any kind. ✅ **(3) `grid-template-rows: var(--toolbar-h)` was a fixed track over a top bar that wraps.** Below 560px the bar's second and third lines rendered **under** the page instead of pushing it down, so the brand and the search box overlapped the mockup notice. `minmax(var(--toolbar-h), auto)` — the token stays the minimum and the sticky offset, and the row grows with what is in the bar. ✅ **(4) Every screen marked `aria-current="page"` on both Home and its own nav link.** Two elements claiming to be the current page is a contradiction the same file's own comment names. The shared chrome is now generated from `index.html` into the other four with the marker re-applied from each file's route, so it cannot drift again |

**SW-16 to SW-20 came from a fourth place: a verification pass over the dual-install switcher after
the thread that built it stopped mid-edit.** The instruction was to assume nothing about what was
finished and to check the sample data **row by row rather than trust it**, which is what turned up
SW-16 and SW-17 — two places where the second install was drawn but its data was not derived with
it. SW-18 to SW-20 were queued work, and each one grew a finding while being done. No adversarial
reviewer raised any of these either.

| # | Finding | Disposition |
|---|---|---|
| **SW-16** | **The post-grab sentence in `usarr.js` named Komga, and was blind to the install switcher.** `SINK_NOTE` is the sentence a row gets after a grab, one per media type. Its `comic` entry read *"Komga shows what is already inside the folder it scans"* — [ADR-0035](DECISIONS.md#adr-0035) replaced Komga with Kavita, the `Post-grab behaviour by media type` table on the same screen was updated, and this map was not, so **one screen named two different comics servers** depending on whether you read a row or grabbed one. Worse, the map is a single flat object with no install dimension at all, while every sentence in it names a service: on the v0.1 install, grabbing an audiobook produced *"Audiobookshelf watches `/media/books` and will show it once the file is there"* — the mockup asserting a service the selected install does not have, which is the exact failure the switcher exists to expose | **Applied.** `SINK_NOTE` is now keyed `{both}` or `{full, v01}` per type, and `grabbedNote` resolves against the live `install`. The strings are **the same strings the post-grab table renders, verbatim**, lifted from the markup rather than retyped, so the row a reader compares against and the toast they get cannot drift. `movie` and `series` stay single (`both`) on purpose and not out of laziness: Radarr and Sonarr are connected on either install, so those two sentences genuinely do not move, and writing them twice would only invite them to. Verified by driving both installs in Chromium and grabbing: `full` still produces the Audiobookshelf sentence, `v01` now produces *"and no connected service catalogues audiobooks. Audiobookshelf **would** show the file once it were inside the folder it watches; connecting it is what makes that true"* — which names the service as the thing that **would** fix it rather than as something already running. The `comic` entry names Kavita in both |
| **SW-17** | **The v0.1 install's Libraries edit view gave Sonarr Anime a host belonging to a service that install does not have.** `libraries.html`'s `Catalogue sources` table, in the derived v0.1 copy, listed Sonarr Anime at `http://10.0.0.5:8989`. Services states `http://10.0.0.9:8989` on both installs, and `10.0.0.5` is **Navidrome's** host (`10.0.0.5:4533`). So the one screen where a user inspects a library's binding contradicted the one screen where they configure the service, and did it by borrowing the address of a media server the v0.1 install is defined by not having. Newly introduced with the derived copy — the full-stack table has always been correct | **Applied**, both occurrences (the visible text and the `title=` that mirrors it) corrected to `http://10.0.0.9:8989`. Recorded rather than fixed silently because of what the class of error is: derived content is written by copying a block and editing what changed, and a value that *looks* plausible in the new context is the one that survives the edit unexamined. The reconciliation that catches it is not "is this a valid host" but "does every screen agree", which is why the row-by-row pass was worth running |
| **SW-18** | **`repack` and `proper` were drawn as indexer flags, and the vocabulary was believed to be closed.** Neither is ever emitted in `indexerFlags` — both are release-title qualifiers the \*Arrs parse out of the *name* — so drawing them in that column invented a status the artefact then taught to a reader. `docs/reference/tags.md`'s `flag:` line is where they came from. A first attempt to fix this by matching against a **closed set of seven** would have been a second bug: the "seven" came from a probe that grepped `new IndexerFlag(` and missed C#'s target-typed `new(...)` | **Applied, and the closed-set premise is rejected on primary source.** `IndexerFlag` is a **class, not an enum** (`src/NzbDrone.Core/Indexers/IndexerFlag.cs`, Prowlarr `develop`): seven statics, every one written `new(...)`. `PassThePopcornFlag : IndexerFlag` subclasses it and adds `golden` and `approved` (`Definitions/PassThePopcorn/PassThePopcorn.cs:85-88`). **Nine today and open forever**, so a chip renders whatever arrives, as an opaque tag — an allowlist would have dropped `golden` today and every future indexer's flags after it, and dropped them *invisibly*, the row simply showing fewer chips than the indexer sent. `golden` is now **drawn** on the PassThePopcorn row, because an unexercised path is not a designed one. `freeleech` and `halfleech` keep one step of emphasis, by weight and fill rather than hue, because they alone are **derived** rather than sent — `TorznabRssParser.GetFlags` sets them from `downloadFactor == 0.5` and `== 0.0` — and they alone change what a download costs a ratio. **Absence is unknown, never "not freeleech":** those are exact-equality tests on a double defaulting to `1`, so a 25% or 75% promo yields nothing, and `GetFlags` runs only inside `if (torrentInfo != null)` while `NewznabRssParser` never touches a flag, so usenet always yields an empty array. The column reads `not reported` on usenet, `none reported` on a torrent with an empty list, and **`None` on neither** |
| **SW-19** | **The `playwright-core@1.56.1` pin in `web/package.json` was decorative, and the one failure message covered two unrelated failures.** `check.mjs`'s resolution ladder asked for the bare name `playwright` at every rung. `web/package.json` pins `playwright-core`, so the ladder walked past the pin at every location and the check ran on whatever the machine happened to have globally — the opposite of what pinning is for. Separately, "playwright is not installed" was printed for both a missing module and a missing browser, sending people to `npx playwright install` for a problem `pnpm install` fixes | **Applied.** Every rung now probes **both names**, so the pin resolves. The failure path is two branches with nothing shared but the word *playwright*. **Module missing** is catchable only with a dynamic `await import()` inside try/catch — a static `import` is hoisted and throws `ERR_MODULE_NOT_FOUND` before any handler in the file exists — and advises `pnpm -C web install`. **Browser absent or mismatched** is thrown from `chromium.launch()`, matched on `/Executable doesn't exist at/`, and names a **version mismatch against the cache** as the likely cause *before* mentioning any download: each release pins one browser revision, so a cache filled by a different release satisfies none of it, and `npx playwright install` fixes the symptom expensively by fetching a *second* build while burying the skew that caused it. **Both branches were exercised deliberately rather than reasoned about** — the module moved aside for the first, `PLAYWRIGHT_BROWSERS_PATH` pointed at a cache holding revision 1000 against a module wanting 1194 for the second — and each printed its own message and exited 1. A new **check 0** asserts `chromium.launch()` resolves `chromium_headless_shell` and not the full browser; `executablePath()` cannot answer that, since it reports the full build regardless while the process that starts is the shell, so the check watches the spawn instead. **`web/package.json` itself is untouched** — see the routing note |
| **SW-20** | **§13's copy rules had never once been enforced on any page title, and the hole was larger than that.** `check.mjs` §1b reads **rendered chrome text**, so its corpus was strings that lay out as a block. Everything user-visible that is not laid out at all was outside it: `document.title` (not in the DOM at all), `aria-label` (for an icon-only control, the *entire* user-visible string), `title`, `placeholder`, and `<option>` text — an option has no layout box until the menu opens, so `display` is never blockish and the walk skipped all 1,298 of them | **Applied, with a floor per source rather than one combined floor** — a single floor would be met by `aria-label` alone while `document.title` silently contributed nothing, which is exactly how the title was free to drift. Structural exclusions are the **same** ones the rendered walk uses (`<td>` is data, `.statebar` is scaffolding), so the two halves of one rule cannot disagree about what the product's voice is. **It caught three violations on its first run, all in text nothing could previously lint.** ⚠️ The worst is `prototype.html`'s `<title>`: *"UsArr v0.1 screen mockup — static, invented data, nothing implemented"* — an em dash in a nine-word string, **and a v0.1 claim over a page whose default view is the full stack**, which §16 sequences *after* v0.1. That string had been the browser tab, the bookmark and the history entry for every reader of the published file, and it is the one surface a reader meets before any in-page notice. It now reads *"UsArr screen mockups: static, invented data, nothing implemented"*; the milestone belongs to the install switcher, which can state it truthfully per install, where a fixed title cannot. The other two are both install-switcher `<option>` labels carrying an em dash in a short string, now colon-separated — **rewritten rather than exempted**, since nothing about milestone labelling needs an em dash |

**SW-21 came from a fifth place, and it is the only one of these that is a *settled argument* rather
than a defect: four rounds between three threads — design, frontend and code — over how the release
result list may reorder itself.** It is recorded here because a ruling that expensive is worth
exactly nothing if the next reader re-derives it from the shipped list, and because the *reason*
behind it is the part most likely to be lost in a refactor. It is not a reviewer finding, not a
consistency finding and not a defect; the disposition below is where it was written down, not what
was fixed.

| # | Finding | Disposition |
|---|---|---|
| **SW-21** | **A release result list that reorders itself while a user is reaching for a row moves the target after they aimed — and the affordance it moves is `Grab`, which has no undo.** A grab is irreversible from UsArr's side: it is handed to a download client that UsArr **deliberately stops observing after handoff**, so a mis-click cannot be detected, cannot be reported and cannot be reversed. **Where there is no undo, prevention is the only lever**, which is what makes ordering stability a correctness question on this screen rather than a polish one. The question took **four rounds between three threads** and is **CLOSED**; it is not reopened by anyone re-reasoning about it from the shipped SPA | **Written down, in two places, as specification.** `ARCHITECTURE.md` **§17.5** carries it as the Requests-screen spec and `design/DESIGN-DIRECTION.md` **§9.1a** as the general component rule every mutable list in UsArr inherits. Six clauses. **(1) The governing sentence, which carries the whole rule: _instability is acceptable only while nobody is aiming at anything._** It keys on whether a **person is committed to a target**, never on whether the **app considers itself settled** — a re-derivation that keys on fan-out completion alone has already lost the point. **(2)** Re-sort **live** while the fan-out runs; **freeze on completion** until the user explicitly re-sorts. **(3)** Freeze while the **pointer is inside the results region OR focus is within it**, whatever the fan-out is doing; while frozen, anything that would have reordered surfaces as **one explicit control**, `3 new results · re-sort`, so ordering changes under an engaged user happen **only because the user asked**. **(4)** **Identity, not position**, for focus, hover, selection and pending row state — keyed to the row's stable id, never its index. **(5)** **0 ms, and never animate a re-sort**, because an animation widens the window in which the row under the pointer is neither where it was nor where it is going. **(6)** **Sort keys in the URL**, so a sorted view is linkable and survives reload. ✅ **The frontend thread's contribution is named rather than absorbed, because two of the six clauses are its improvements and one is independent corroboration.** It **converged on the suspend ruling independently** — arriving at "stop reordering while the user is engaged" from the implementation side without having the design thread's argument in front of it, which is why the ruling was treated as settled rather than as one thread's preference — and then **improved it twice**: first by adding **focus-within** beside pointer-within, which is what extends the guarantee to the keyboard user (identity-keyed focus travels with its row; the **physical pointer travels with nothing**, since it sits at a screen coordinate no amount of identity keying can move, so the two input paths fail differently and neither half of the condition is redundant); then by **collapsing the straggler case into the same one control**, replacing a proposed append-below-marked-late mechanism — one condition with two surfaces is two rendering rules to maintain, the *late* marker is meaningless the instant the list is re-sorted, and appending still moves the list under an engaged user. ⚠️ **Deliberately not demonstrated in the mockups, and said so in §17.5**: they are static documents with invented data, so there is no fan-out to run, nothing to arrive late and no honest count for the control to carry. Drawing a frozen screenshot of it would assert a behaviour the artefact cannot exercise — `CLAUDE.md`'s "no invented status" failure by illustration. **No drawn mockup changed** — not a screen, not the CSS, not the JS — so `prototype.html` is unregenerated. 🔍 **Opened as [ADR-0038](DECISIONS.md#adr-0038), once the merge was in.** The rule closes off alternatives (the append-below-marked-late mechanism, an animated reorder, index-keyed row state), so by `CLAUDE.md`'s convention it is ADR-shaped. It was deliberately **not** taken in the pass that wrote this row, because a fresh ADR number is the one edit likeliest to collide with a merge in flight; the number was claimed immediately after that merge landed, against the merged tree rather than against the branch. The ADR is a pointer to §17.5 and §9.1a plus **the rejected alternatives and why**, which is the part neither of those documents records well. ⚠️ **One correction made in the same pass, recorded here rather than given a number of its own, because it is a defect in this log's own routing rather than a new finding.** `design/mockups/README.md`'s routing section carried two items that had **already landed on `main`** — it was written against a pre-merge branch, so it sent readers to fix `docs/reference/tags.md`'s `flag:` line (corrected on `main` before the report was written) and to add the open-vocabulary clause to `ARCHITECTURE.md` §8.5 (landed as **`4171b35`**). Both are now marked **CLOSED** with a pointer to the landed wording. **And the routed replacement text was wrong in its own right, which is the part worth keeping:** it rendered the flag vocabulary as **one flat list of nine**, with PassThePopcorn's `golden` and `approved` sitting as peers of the seven common statics. **The code thread was right to decline it.** The seven are the common set any indexer can emit; the other two are **one indexer's private additions** and are examples of an *unbounded* category rather than members of a fixed one — so a flat nine reproduces the original error one layer along, with a re-checker counting nine and treating *that* as the closed set. Our copy now matches the landed `tags.md` shape, which keeps the groups apart, and carries its re-check command and its trap: `grep -rn "static IndexerFlag" src/`, **never** `grep "new IndexerFlag("`, which returns **zero matches in a file containing seven** because the file constructs with C# target-typed `new(...)`. Only `mockups/README.md` changed; `build_prototype.py` reads the five screen files plus the shared CSS and JS and never the README, so `prototype.html` is correctly unregenerated |

---

# Seam audit — the two-phase request destination

**Date:** 2026-08-16. **Branch:** `main`. Not a review round, and no adversarial reviewer behind it.
A design thread tested one deferred feature's seam by walking the schema as if building it —
`FUTURE.md` §11, slskd as a music request sink — and found the seam half missing. Prefix `WQ-` has
not been used before, so nothing collides. **Nothing slskd-related was built**; the finding is about
the schema the deferred feature would land on.

Every part of the relayed claim was re-verified against `internal/db/migrations/00001_initial.sql`,
`docs/reference/schema.md` §10, `internal/db/testdata/schema.sql`, `internal/store/` and
`internal/db/queryplan_test.go` rather than taken from the relay — the two previous relays in this
log (SW-02…SW-04) were each wrong on some detail. This one was right on every load-bearing part.

| # | Finding | Disposition |
|---|---|---|
| **WQ-01** | **`write_queue.state` has no value meaning "waiting for a human", so a two-phase asynchronous request destination has nowhere to park.** The `CHECK` is `state IN ('pending','inflight','verifying','done','failed')` — verified verbatim in the migration and in the schema snapshot. All three non-terminal values assume a *machine* owes the row an answer: `pending` is claimed by a worker, `inflight` asserts an outstanding upstream request, and `verifying` carries `verify_until`, documented in the DDL as a *15-minute TTL* and specified by ADR-0012a and ARCHITECTURE §7.6 as ending in one final verification and an explicit `failed` — which for an unanswered row is `fail_reason = 'unknown'`. A sink whose flow is search → present → **a person chooses** → enqueue must hold a row for human latency, and a sleeping user is not a failed request | **Confirmed as a real gap; the schema fix is deferred to the library-sync migration, and the documentation is fixed today.** See the three corrections below. Two parts of the relayed claim were *also* checked and are right: `library.sink_service_instance_id` needs no change, and the `Grabber` interface (`reference/providers.md` §2) already expresses two phases as `Releases` then `Grab(rel)`. A third thing the relay did not say and which matters: **`write_queue.kind` carries no `CHECK` at all**, so the *verb* half of a two-phase operation is already expressible with no schema change. The gap is exactly and only `state` |
| **WQ-02** | **`FUTURE.md` §11 claimed this seam was protected.** It named "the provider registry plus the durable command queue" as the seam and said the one property to protect is that "the queue's verb model can express a two-phase asynchronous operation". The verb model is indeed unconstrained — and it is not the binding constraint. The constrained column is `state`, which the entry never mentioned | **Applied.** The relayed overclaim is real. §11's seam paragraph now states what the schema can and cannot express, names `state` as the missing half, records that the verb half genuinely holds, and ends "**identified and scheduled, not protected**" so nothing there reads as shipped |
| **WQ-03** | **The first proposed fix — amend `00001_initial.sql` in place, on a one-time override of the never-edit-a-merged-migration rule — is rejected.** Its premise was that SQLite cannot `ALTER` a `CHECK`, so a later fix costs a full 12-step table rebuild while today it costs one line, and pre-release with no installs that trade favours acting now | **Rebutted, and the rule stands unamended. The premise is false: the rebuild is already mandatory.** Verified — `write_queue.work_id` in `00001_initial.sql` is bare `INTEGER` with the comment *"FK to `work(id)` ON DELETE CASCADE dropped: `work` lands with library sync"*, and `grep` confirms **no** `REFERENCES work` anywhere in 0001, matching the file header's stated pattern of keeping the column and dropping the foreign key. SQLite cannot add a foreign key to an existing column either, so the migration that ships library sync must do the 12-step rebuild of this table regardless. Adding a `CHECK` value during a rebuild that is happening anyway costs nothing, there is no window closing, and so there is no benefit to weigh against overriding the rule. The migration file is unchanged and byte-identical to its committed state |
| **WQ-04** | **A note only in `FUTURE.md` would not be read at the moment it has to be acted on.** The change is free exactly once — while the library-sync migration is being written — and whoever writes it will be reading the DDL, not the deferred-features list | **Applied.** `reference/schema.md` §10 gains a flagged block directly under the `write_queue` DDL and its index prose, listing all four steps: add `'awaiting_choice'` to the `state` `CHECK`; recreate **all three** indexes on the rebuilt table — `ux_wq_idem`, `ix_wq_work`, and `ix_wq_runnable` *with its partial `WHERE` predicate*, since a rebuilt table inherits no indexes and a partial index silently rebuilt as a full one changes which rows the sweep sees; settle the index question in WQ-05; and regenerate the snapshot with `go test ./internal/db -run TestMigrationRoundTrip -update-schema`. It also states plainly that 0001 is not to be edited for this. `FUTURE.md` §11 points at it rather than repeating it |
| **WQ-05** | **Whether `'awaiting_choice'` should sit inside `ix_wq_runnable`'s partial predicate.** The predicate is `WHERE state IN ('pending','inflight','verifying')`, verified in the migration, in the snapshot and in `queryplan_test.go`'s `ix_wq_runnable` case; `schema.md` §10 also makes it the reconciliation guard's index | **Recorded as undecided — the owner's call, deliberately made at rebuild time and not now.** ⚠️ **This is a lean, not a decision.** The lean is to **exclude** it: a row waiting on a person is not runnable, and leaving it in the predicate re-exposes it to the retry sweep and the `verify_until` TTL, which is WQ-01 reintroduced through the index. It is not settled here because the same predicate serves the reconciliation guard, and that decision wants the reconciliation code in view rather than an argument from the schema alone — the guard is unimplemented today; nothing in `internal/` reads `write_queue` yet outside tests. Both `schema.md` §10 and `FUTURE.md` §11 carry it as open, with the instruction that whichever way it goes, the reason is written next to the predicate, because an exclusion reads as an oversight otherwise |

**No code changed and no schema changed.** The whole disposition is three documentation edits —
`reference/schema.md` §10, `FUTURE.md` §11, and this entry — plus the deliberate decision not to
touch a merged migration.

---

# Round 6 — five prongs over `main`, the shipped code and a fresh install

**Date:** 2026-08-16. **Target:** `origin/main` at `d38bc8e`, *"docs: name the write-queue seam a
two-phase sink actually needs"* — the tree as it stands after Round 2's fixes landed. **This entry is
written onto a branch cut from a later `main` (`cb57e43`), which carries documentation commits plus
one `Makefile` change (the new `make design` target, ~50 added lines).** No `.go` file moved between
the two, so every Go citation below is exact on both; the five `Makefile` citations have shifted and
are mapped at the end of FI-03.

**Inputs:** five independent adversarial passes, run in parallel and without sight of each other:

| Prefix | Pass |
|---|---|
| `DS-nn` | Doc/spec consistency — documents against shipped code |
| `DL-nn` | The data layer — migration 0001, the store package, the pools |
| `SR-nn` | Security — crypto, SSRF, CSRF/sudo |
| `FI-nn` | A fresh install — clone, `make tools`, `make check`, first run, backup and restore |
| — | An independent verification of the one Critical finding, recorded at §5 |

Every claim below was executed, reproduced or read out of primary source by the pass that raised it;
the proving command and its verbatim output are carried into each entry rather than summarised away.

**Scope of this log:** all 48 findings. Nothing is dropped and nothing is merged silently — where two
or three passes found the same thing it is recorded **once**, in one entry, saying which passes found
it.

**Disposition of this round differs from Round 2's, and deliberately so.** This thread reviews and
records; it does not fix other threads' code or docs. Every entry is therefore **Open — recorded here
rather than applied**, which is the log's existing vocabulary for a finding that is real but belongs
to another owner (Round 2, B-01: *"Correction recorded here rather than applied, because ADR-0024
lives on branch `claude/hearth-thread-vn9w7u`"*). Each such entry states the **fix shape**, so routing
is a hand-off and not a re-investigation. Findings a reviewer explicitly rebutted, or explicitly
concluded were **not** defects, are recorded as rebutted in §2 with the reasoning, so they are not
rediscovered and re-raised next round.

**Working trees were left clean.** The doc prong made no edits. The data prong deleted its
`zz*_scratch_test.go` harnesses and confirmed `git status` clean with the existing tests green. The
security prong, the verification pass and the fresh-install pass each worked in a separate clone and
left `/home/user/UsArr` at `d38bc8e` with an empty `git status --porcelain`. The fresh-install pass
removed the stray `/config` and `/data` directories it created. The data prong's harness files were
briefly visible to the doc prong mid-run and were correctly left alone.

---

## Counts

| Severity | Count | IDs |
|---|---|---|
| **Critical** | 1 | DS-01 — **confirmed by three independent passes**, see §5 |
| **High** | 12 | SR-01, DL-01, DL-02, DL-03, DS-02, DS-03, DS-04, FI-02, FI-03, FI-04, FI-11, FI-05 |
| **Medium** | 13 | SR-02, SR-03, DL-04, DL-05, DL-06, DL-07, DL-08, DS-05, DS-06, DS-07, DS-08, DS-09, FI-06 |
| **Low** | 17 | SR-04, SR-05, SR-06, DL-09, DL-10, DL-11, DL-12, DL-13, DS-10, DS-11, DS-12, DS-13, DS-14, FI-07, FI-08, FI-09, FI-10 |
| **Informational / inference** | 6 | SR-07…SR-12 |
| **Total** | **49** | |

| Disposition | Count |
|---|---|
| **Open — recorded here rather than applied** (routed; fix shape stated) | 42 |
| **Open — fix in flight in the code thread** | 1 (DS-01) |
| **Recorded as a deliberate decision rather than a defect** | 3 (DL-09, SR-09, DS-08's code half) |
| **Rebutted — checked and found not to be a defect** — see §2 | 3 |
| **Already dispositioned in an earlier round; cross-referenced only** | 2 (DS-08 ↔ W-02, SR-09 ↔ SUS-02) |
| **Environmental, not a repo defect** | 1 (FI-10) |
| **Applied by this thread** | **0 — by design** |

**Found independently by more than one pass** — recorded once each, and named here so the
corroboration is visible:

| Finding | Passes that found it |
|---|---|
| **DS-01**, the `keys/kek.salt` backup defect | Doc/spec, the **fresh-install** pass, and a dedicated **verification** pass — three, each with its own clone, its own binary and its own end-to-end reproduction |
| **DS-02**, `DEVELOPMENT.md` marking working `make` targets *(not yet)* | Doc/spec and fresh-install (its item 7c) |
| **DS-03**, `CONFIGURATION.md` §5 naming files that are never created | Doc/spec and fresh-install (its item 7d) |
| **DS-08**, conditional `Secure` on the session cookie | Doc/spec (against the documents) and security (by execution) — and it is Round 2's W-02 |
| **SR-09**, `ClassConfigured` has no address denylist | Security (executed) and fresh-install (read from `policy.go`, confirmed live) — and it is Round 2's SUS-02 |

**Zero critical findings in the data and security prongs.** The data prong says it in as many words:
*"None. Nothing in the shipped path corrupts data or leaks a credential."* The single Critical is a
**documentation** defect with a data-loss outcome.

**The cryptography itself came out clean under two independent methods.** The doc prong verified the
AAD formula, the envelope layout and the SSRF resolve-then-pin discipline **against the documents**;
the security prong verified the same three **by execution**; the fresh-install pass verified the
whole master-key validation ladder **by running it**. They agree. The disagreements in this round are
almost entirely about what is *written down*, what is *swept*, what is *scoped* and what a fresh
clone can actually run — not about the primitives.

---

## 1. Disposition of every finding

### 1.1 Critical

| # | Finding | Disposition |
|---|---|---|
| **DS-01** | **The documented backup/restore procedure destroys every stored credential.** `keys/kek.salt` is per-install key material, is mandatory to derive the KEK, is regenerated silently when absent, and is excluded by the documented backup. Docs: `docs/CONFIGURATION.md:389-437` (§5, declared *"authoritative for the whole project"* at `:11`; its `keys/` tree lists exactly two files, `secret.key` and `secret.key.new`, at `:398-399`), `:465` (§6.1's recoverability table), `:479-480` (§6.1's `tar --exclude='keys'`, annotated *non-negotiable*), `:512-513` (§6.3 step 3), `:516` (§6.3 step 5), `:525-526` (§6.4 step 2). Code: `internal/config/config.go:187` / `:198` (`KEKSaltPath` → `keys/kek.salt`), `:180-198` (*"losing either makes every stored credential unrecoverable"* — the code comment is correct and the user-facing docs contradict it), `internal/crypto/derive.go:52-80` (HKDF takes the salt directly and `derive` hard-errors on an empty one; a **different** salt silently yields a different KEK, with no error), `internal/config/secretkey.go:221-254` (on `os.ErrNotExist`, falls through to `rand.Read(salt)` + `writeSecretFile`), `cmd/usarr/app.go:117` (the sole caller, which logs nothing) | **CONFIRMED, and Open — fix in flight in the code thread** (shape below the table). **Reproduced end to end by three independent passes, each in its own clone with its own binary, and isolated to `kek.salt` by a control experiment** — see §5 for both full reproductions, the byte-identical master-key hashes and the control. **Fix shape** is in three parts, listed in the order they matter: **(1)** the recoverable unit is `keys/` **in its entirety** — `secret.key` **and** `kek.salt` — so §6.1's `--exclude='keys'`, §6.3 step 3 and §6.4 step 2 are rewritten together, §5's `keys/` tree gains the third file, and §6.1's table (`:465`) stops claiming the master key is the only loss path. **(2)** §6.3 step 5's diagnostic (*"A red test with a decryption error means the key does not match the backup"*) is replaced — it misdiagnoses this exact failure, as does the runtime error string, which tells the operator to *"restore the master key that sealed it"* when they already have. **(3)** the code half: **there is no fail-closed guard for a missing salt**, and the asymmetry is the finding — the master key on the same startup path gets **both** a loud `log.Warn(masterKey.BackupNotice())` (`cmd/usarr/app.go:107`) **and** a hard `ErrMissingKeyForExistingData` refusal when the database holds encrypted rows (`app.go:97-100`, verified working by the fresh-install pass). The salt has **neither**, and `ResolveKEKSalt` returns a bare `([]byte, error)` with **no source indicator**, unlike `MasterKey{Source, Path}`, so the caller *cannot* distinguish "read" from "generated" even if it wanted to. It should fail closed identically |

**The fix in flight, recorded so the routing is visible and the finding is not read as dropped.** The
code thread is **moving `kek.salt` out of `keys/` to sit beside `usarr.db`**, on the reasoning that
**the salt is not secret**, and that `keys/` being excluded from backups is exactly what turned
otherwise-correct backup advice into a trap. Four parts:

1. **Relocation**, with a **startup migration of the existing file** so that current installs are not
   broken by the fix itself — the one thing a fix for this defect must not do is reproduce it.
2. **A fail-closed guard mirroring the master-key path** — the asymmetry named above, closed.
3. **Correcting the misdiagnosing error text** at `cmd/usarr/services.go:151-155`, which today tells
   the operator to restore a master key they already have.
4. **Both doc sites**, together: `CONFIGURATION.md` §5's self-declared-exhaustive `keys/` listing, and
   `security.md`'s unlocated salt at `:75`.

**Fallback if the relocation proves risky: guard-plus-docs only** — items 2, 3 and 4, leaving the file
where it is. That is a strictly smaller change and it still closes the data-loss path, because a guard
that refuses to start beats a silent regeneration whatever the file's address.

**The one precision qualifier, and it is recorded because the original claim overstated it.**
"Documented nowhere" is very slightly too strong. `docs/reference/security.md:75` **does** document
that a stored per-install salt exists, inside the derivation formula:

```
KEK = HKDF-SHA256(USARR_SECRET_KEY, salt=<per-install random, stored>, info="usarr/kek/v1")
```

What is documented nowhere is **the file's name, where it lives, that it is inside `keys/`, that
`--exclude='keys'` discards it, and that it must be restored.** The exact greps, run by the
verification pass:

```
$ grep -rn "kek\.salt" docs/ README.md CLAUDE.md
exit=1   (no match)

$ grep -rniw "kek" docs/ README.md CLAUDE.md --exclude=prototype.html
docs/DECISIONS.md:1199, 2828        (HKDF info labels only)
docs/reference/security.md:22,35,36,69,72,75,79
docs/reference/gateway.md:269
docs/ARCHITECTURE.md:640
```

**This makes it worse in one specific respect rather than better.** `docs/CONFIGURATION.md` §5 calls
itself the single source of truth for on-disk layout and claims property 3, *"Every file both this
document and `ARCHITECTURE.md` name is here"* — so the omission is **from an explicitly exhaustive
list**. The code already knows: `internal/config/config.go:191-197` and `secretkey.go:221-225` both
carry the comment *"security.md §1.3 requires a stored per-install salt but does not say where it
lives"*. Two further doc-side aggravators, both from the verification pass: §6.1's 🚩 (*"`tar -czf
backup.tgz /config` is not a backup, it is a compromise"*) actively steers operators away from the
one archive that **would** have preserved the salt; and §3.5, "If you lose the key", lists what
survives and never mentions that keeping the key is insufficient.

**Recovery paths — none exist, checked exhaustively.** `internal/db/migrations/00001_initial.sql` has
no settings table and no salt column (`grep -rniE "salt|setting|kek"` returns a single hit,
`kek_id INTEGER NOT NULL DEFAULT 1`, a rotation counter). `internal/crypto/envelope.go:27-29` — the
envelope is `kek_id(4) || nonce(12) || wrapped_dek(40) || ciphertext || tag(16)`, **no salt**. No
fallback, no legacy path, no second derivation attempt anywhere in `internal/crypto` or `cmd/usarr`.
32 bytes from `crypto/rand`, not brute-forceable. **And key rotation is not a workaround:**
`usarr key rotate` is marked *(proposed CLI)* and does not exist, and rotation rewraps DEKs under the
same salt regardless. The only repair is §3.5's manual re-entry of every credential — which is
exactly what §6 promises you avoid by keeping the master key.

### 1.2 High

| # | Finding | Disposition |
|---|---|---|
| **SR-01** | **Indexer-supplied credentials are written to SQLite verbatim** — `release_candidate.info_url` and inside `raw_release_json`. `servarr.SanitizeRelease` (`internal/servarr/redact.go:48-53`) drops only `DownloadURL` and `MagnetURL`; `InfoURL` survives into the persisted row (`internal/releases/search.go:638`, `internal/releases/storeadapter.go:74`) and into the marshalled blob. Redaction happens **only** at the HTTP boundary (`internal/httpapi/search.go:278-280`, `redactURLField`), so the browser sees a clean URL while the database keeps the credential. This contradicts the codebase's own rule, stated twice — `internal/releases/storeadapter.go:70-73` (*"one copy of a credential is one too many already"*) and `internal/releases/search.go:589-606` (*"this blob lands in the same file, the same VACUUM INTO backup and the same support bundle, so the same rule has to apply to it"*) — and `internal/ssrf/redact.go:24-32` argues at length that `infoUrl` is exactly the field that carries private-tracker passkeys, *"and a leaked passkey on a private tracker means account termination"* | **Open — recorded here rather than applied.** This is the residue of Round 2's SEC-01 and W-01: SEC-01 sanitised the blob of `downloadUrl`/`magnetUrl`, W-01 added the passkey names to the deny-list **at the HTTP boundary**, and the at-rest half of `infoUrl` was never closed. **Fix shape:** redact in `SanitizeRelease` / at the `Candidate` boundary, not at the HTTP boundary — run `ssrf.RedactRawURL` on `InfoURL`/`CommentURL`/`PosterURL` before `persist()` marshals. The `provenance` table has the same shape (`storeadapter.go:160,164`) and must move with it. A tripwire in the shape of Round 2's `TestPersistedBlobNeverCarriesTheAPIKey` belongs on this path too |
| **DL-01** | **The release-candidate TTL sweep has no production caller — the only shipped write path grows the database without bound.** `internal/releases/service.go:123` defines `EvictExpired`, commented *"Run it from the maintenance worker"*. **There is no maintenance worker.** Load-bearing rather than cosmetic: `docs/reference/schema.md:677-686` argues the missing `UNIQUE (service_instance_id, guid)` is safe precisely because *"the bound on that duplication is the TTL — 25 minutes for Prowlarr-sourced rows, swept via `ix_rel_expiry` — so the table's size is governed by search volume within a 25-minute window, not by uptime."* With no sweeper the table's size **is** governed by uptime, and each row carries `raw_release_json` (`internal/db/migrations/00001_initial.sql:209`) into the file and into every `VACUUM INTO` backup | **Open — recorded here rather than applied.** Directly invalidates the stated bound of Round 2's DB-03 decision, which is why it outranks its own blast radius. **Fix shape:** wire `EvictExpired` to a ticker in `cmd/usarr/main.go` next to `RunProber`. **No schema change needed** — the index is already correct and the sweep is already efficient: `ExpireReleaseCandidates releases.go:158 → SEARCH release_candidate USING COVERING INDEX ix_rel_expiry (expires_at<?)` |
| **DL-02** | **`release_candidate` and `provenance` carry no `user_id`** — the one principle-4 retrofit migration 0001 exists to prevent. `ARCHITECTURE.md:87-91` enumerates the user-scoped tables and does **not** include `release_candidate`. A `release_candidate` row **is** user-generated content: it is the materialised result of one user's free-text query, and `title` is the answer to "what did that person search for". The only scope available is instance-based (`internal/store/store.go:96`, `instancePredicate("service_instance_id")`), so on the expected homelab topology — two users, one shared Prowlarr — user B enumerating candidate ids reads user A's search results and can grab them. `internal/releases/grab.go:46` claims *"Out-of-scope is reported as not-found so a caller cannot probe for other users' rows"*; with no `user_id` column the scope predicate cannot express "other users' rows" at all, so the comment overstates what the check buys. `provenance` is worse in one respect — immutable, permanent, no instance column, and its only reader takes no scope at all: `GetProvenanceByDownloadID` (`internal/store/releases.go:261`), which has no non-test caller today but is the seam, and already lacks the parameter `internal/store/store.go:15-19` says every such read must carry | **Open — recorded here rather than applied.** The reviewer asks for one of two outcomes and is explicit that *"right now it is neither"*: **(a)** a genuine omission, fixed by a table rebuild in migration 0002 while the tables are still empty in the wild, or **(b)** a decision **written down** in `schema.md` §6 the way the `(service_instance_id, guid)` uniqueness decision was in Round 2. **Fix shape:** pick one — and if (a), note that migration 0001 is frozen, so this is now a rebuild, which is exactly the cost principle 4 exists to avoid paying |
| **DL-03** | **The read pool is not read-only — the single-writer discipline is convention, and bypassing it produces an unrescuable `database is locked`.** `internal/db/sqlite.go:127` builds the reader DSN with no `mode=ro`; `internal/db/sqlite.go:147` only *says* so in a comment (`// Read returns the read pool. Never start a write transaction on it.`). `ReadTx` is safe (the driver honours `TxOptions.ReadOnly`); bare `Read().ExecContext` and `Read().BeginTx(ctx, nil)` are not. The harm is the exact failure mode `ARCHITECTURE.md:1321` names — a deferred transaction upgrading to a write, which `busy_timeout` does not cover | **Open — recorded here rather than applied.** **Fix shape:** add `mode=ro` (or `_txlock=deferred` + `mode=ro`) to the reader DSN so the invariant is enforced by SQLite rather than by a comment. **Nothing in the current code writes through the read pool, so this is free today and expensive after the first accidental caller** |
| **DS-02** | **`docs/DEVELOPMENT.md` §3 "Getting running" marks two working `make` targets as `(not yet)`** — `docs/DEVELOPMENT.md:146-147` (and `:23`). `Makefile` `.PHONY: tools` (five pinned `go install` lines) and `.PHONY: dev` (`$(GO) run $(MAIN_PKG) --env-file .env`) both exist and both work — the fresh-install pass confirmed by running them. The doc's own §1 preamble (`:6`) defines `(not yet)` as *"commands referencing files that do not exist yet"*, so this is the one marker a new contributor is told to trust, on the two commands in the quickstart, and it sends them to install the toolchain and start the backend by hand. Same class, same doc: `:557` marks `.golangci.yml` as *"starting point **(not yet)**"* — the file exists at the repo root and its linter list matches the doc's YAML block byte for byte (`errcheck govet staticcheck ineffassign unused bodyclose noctx errorlint gosec sqlclosecheck rowserrcheck`, `formatters: gofumpt goimports`), plus a `_test.go`/gosec exclusion block the doc does not show | **Open — recorded here rather than applied. Found independently by the doc/spec pass and the fresh-install pass (item 7c).** **Fix shape:** drop the three `(not yet)` markers at `:23`, `:146-147` and `:557`. Minor rider in the same edit: `:167` reads `make dev # Terminal 1 — Go backend, hot reload`; the recipe is a plain `go run` and there is **no hot reload** |
| **DS-03** | **`docs/CONFIGURATION.md` §2.1 documents log rotation that does not exist, and §5 names three more artefacts that are never created.** `:126` — *"Log rotation is fixed at 10 MB × 5 files in `$USARR_DATA_DIR/logs/`"* — and `:423`, inside the §5 tree the header at `:11` declares authoritative: `usarr.log # 0600, + rotated usarr.log.1 … (10 MB × 5)`. `cmd/usarr/main.go:165-198` (`newLogger`) writes to `os.Stdout` only, via `slog.NewTextHandler`/`NewJSONHandler`. **No rotation library is in `go.mod`** — no lumberjack, nothing. `cmd/usarr/app.go:192` creates `logs/` and `config.go:217` exposes `LogsDir()`, but `grep -rn "LogsDir()"` shows `ensureDirs` is its only non-test caller. The fresh-install pass confirmed on a real install, after a full first run **and a service add**: no `cache.db`, no `cache/images/`, no `tmp/`; `logs/` created 0700 and empty; all output on stdout | **Open — recorded here rather than applied. Found independently by the doc/spec pass and the fresh-install pass (item 7d).** Stated as present-tense fact, not marked `(proposed)`, in the section declared authoritative — a user configuring log shipping or disk quotas follows it and gets nothing. **Fix shape:** either mark the four claims `(not yet)` or build them; `CLAUDE.md`'s "no invented status" rule points at the marker, not the feature. Note `cache.db` and `cache/images/` belong to the not-yet image pipeline, and `:424`'s `tmp/ # in-progress work; cleared at startup` describes a directory that is never created and nothing clears |
| **DS-04** | **ADR-0025 is Accepted, the frontend now exists, and it implements none of it.** `docs/DECISIONS.md:1593-1600` — ADR-0025, **Status: Accepted** — decides Tailwind v4 via `@tailwindcss/vite` with `@theme { --*: initial; }`, Bits UI, Tabler icons, self-hosted IBM Plex, and its Context states *"as of this ADR there is no styling decision anywhere in the repository and **no frontend code to constrain one**."* There is now 1,755 lines of frontend. `web/package.json` contains **no** `tailwindcss`, `@tailwindcss/vite`, `bits-ui`, `@tabler/icons-svelte` or any font package; `web/src/app.css` is 426 lines of hand-rolled CSS; `web/static/` holds one file, `favicon.svg` — no IBM Plex is self-hosted, and `app.css:115` falls back through `'IBM Plex Sans', system-ui, …` to whatever the OS has | **Open — recorded here rather than applied.** The reviewer is explicit that `app.css:1-17` is **candid** about being scaffolding that copies values out of `docs/design/tokens.css`, and that this is a reasonable engineering call — the finding is that ADR-0025's Context is now factually false and nothing in `DECISIONS.md` records that the accepted stack is unimplemented, so the ADR reads as describing the repo. **Fix shape:** an amendment block on ADR-0025 in the house style, saying the stack is accepted and not yet implemented. **Bonus, exact:** `web/src/app.css:4` cites the wrong ADR — *"ADR-0024 chooses Tailwind v4…"*; ADR-0024 (`DECISIONS.md:1540`) is the AGPL-3.0 licence decision, the styling ADR is **0025** |
| **FI-02** | **`make check` / `make check-offline` fails on a fresh clone: `fmt-check` has no `web-deps` prerequisite.** It is documented as *the* pre-commit gate, and it is the first thing a fresh clone hits — half a second in, before any Go code is examined. `lint-web` and `test-web` both declare `web-deps`; `fmt` and `fmt-check` do not, and `fmt-check` runs **first** in `check-offline`. The undocumented recovery is `make web-deps`, which is mentioned in neither `DEVELOPMENT.md` §3 nor §4 | **Open — recorded here rather than applied.** **Still true on this branch**, checked: `fmt-check` (`Makefile:264-269` here, `:259-263` at `d38bc8e`) has no prerequisite line. **Fix shape:** add `web-deps` as a prerequisite of `fmt` and `fmt-check`, exactly as `lint-web` and `test-web` already declare it — a one-word change on two lines, and the gate's own honesty notice is what makes it worth doing rather than documenting |
| **FI-03** | **The Makefile resolves every pinned tool from bare `PATH`, while `make tools` installs into `$GOBIN` — which is not on `PATH`. Two failure modes, and the second is worse than the first.** All five tools are invoked as bare names, and `make tools` `go install`s into `$(go env GOPATH)/bin` = `/root/go/bin`, which is absent from the container's `PATH`. **(1) Hard 127s:** immediately after a *successful* `make tools`, `make fmt-check` dies with `gofumpt: command not found`, exit 127. Only `secrets` carries a `command -v … \|\| echo "run: make tools"` guard; `fmt-check`, `lint-go` and `vuln` die with no hint. **(2) A silently wrong linter version, which is the real defect:** `golangci-lint` is the one tool that *is* on the default `PATH` — at `/usr/local/bin/golangci-lint`, **v2.5.0** — while the Makefile pins `GOLANGCI_VERSION ?= v2.12.2` and invokes it bare. So `make lint-go` runs the unpinned v2.5.0 and reports **`0 issues. EXIT=0`**, while the pinned v2.12.2 on the same tree reports 7 (FI-04) | **Open — recorded here rather than applied.** **Any green lint claimed from a container in this shape is not evidence**, which is the sentence worth carrying out of this finding. **Fix shape:** resolve the five tools through an explicit `$(GOBIN)`/`$(shell go env GOPATH)/bin` prefix in the Makefile, or export it onto `PATH` at the top of the file, and extend the `command -v` guard to all of them so a missing tool says `run: make tools` instead of 127. **`Makefile` line mapping**, because this file moved between the review target and this branch: bare invocations at `d38bc8e` `:247, :256, :261, :271, :294, :312, :316, :320, :326` are `:252, :261, :267, :321, :344, :368, :372, :376, :382` here; `make tools` at `d38bc8e` `:359-365` is `:409-415` here. `DEVELOPMENT.md` §1 and §3 never tell the user to add `$GOBIN` to `PATH` |
| **FI-04** | **`make check` does not pass under the pinned linter — and the true count is 11, not the 7 the gate prints.** The capped, as-is run with `golangci-lint` **v2.12.2** reports `7 issues: * gosec: 4 * noctx: 3`, `EXIT=1`, and `make check`'s own exit status on this tree is **2**, failing at the `lint-go` recipe. **The uncapped run** (`--max-issues-per-linter=0 --max-same-issues=0`) reports **11** — `gosec: 4`, **`noctx: 7`** — see FI-11 for why four were hidden. **All 7 `noctx` hits are `httptest.NewRequest` call sites:** the three the capped run shows, `internal/httpapi/redact_test.go:19`, `:37` and `internal/httpapi/server_test.go:153`, plus the four it hid, `internal/httpapi/services_urlbase_test.go:105`, `internal/web/web_test.go:213`, `:229` and `:317`. Message on all seven: *"net/http/httptest.NewRequest must not be called. use net/http/httptest.NewRequestWithContext (noctx)"* | **Open — recorded here rather than applied. The 4 gosec hits are excluded as owned elsewhere (§3); the 7 `noctx` hits are the in-scope remainder and nothing else appears at any cap setting.** The **11** is **verified by execution against `origin/main` (`cb57e43`) with the pinned v2.12.2 and the cache cleaned** — established fact, not a report. **Fix shape:** seven call sites move to `httptest.NewRequestWithContext(t.Context(), …)`. Two interactions worth carrying: FI-03 means this is only visible when the pinned binary is used, which is why it survived to now; FI-11 means it was **under**-visible even then, and any earlier "3 noctx" figure — including the one this review first recorded — was a floor |
| **FI-11** | **The pre-commit gate silently drops duplicate-text findings, so every issue count it prints is a floor and not a count.** On `origin/main` at pinned v2.12.2 the gate prints `7 issues`; the true number is **11**. The sole suppressor is golangci-lint's **stock `max-same-issues: 3`**, which caps issues sharing **identical message text** — not issues per linter. All 7 `noctx` findings share one message string, so 4 were dropped with no notice of any kind. **Isolated by execution:** `--max-same-issues=0` alone yields all 11; `--max-issues-per-linter=0` alone still yields 7. **This is an unnoticed upstream default, not a repo misconfiguration** — `.golangci.yml` has no `issues:` section and sets neither cap, and `max-issues-per-linter` defaults to 50, not 3, and is irrelevant here | **Open — fix in flight on another branch, and NOT on `main` as of this entry. Verified rather than taken on report:** `.golangci.yml` at `cb57e43`, the tree this entry is written against, is **33 lines long and has no `issues:` section at all** — it carries `version: "2"`, a `linters:` block (`default: standard` plus the eleven enabled), a single `exclusions.rules` entry excluding `gosec` from `_test\.go`, and a `formatters:` block. `grep -n "issues\|max-same\|max-issues" .golangci.yml` returns **no match**. So neither cap is set here and the gate on `main` still truncates. The code thread reports having set `issues.max-same-issues: 0` (and `max-issues-per-linter: 0`, harmless) on its own branch; that is recorded as **in flight**, not as applied, because it is not in the tree this log lives in. **Fix shape, for whoever merges:** `max-same-issues: 0` is the load-bearing one; confirm it survives the merge, because it is the setting that makes every other number in this section trustworthy. **Why it earns High:** a gate whose output is silently truncated cannot be used as evidence, and this round now has two demonstrations of that in one file — *"0 issues"* from the wrong linter version (FI-03) and *"7 issues"* from the right one were **both** misleading, by different mechanisms. **One sentence worth keeping, because it is a live tripwire:** gosec's 4 were never truncated only by coincidence — its three G124 hits share text and land at **exactly** the limit of 3, and G123's text is distinct. gosec is sitting on the boundary, so it starts silently truncating on the very next duplicate-text finding anyone adds |
| **FI-05** | **The documented setup path creates `/config` and `/data` at the filesystem root.** `docs/DEVELOPMENT.md:144` says `cp .env.example .env # optional — every value has a working default`, and `.env.example:24` and `:28` ship **uncommented** `USARR_CONFIG_DIR=/config` and `USARR_DATA_DIR=/data`; `internal/config/config.go:29` also hardcodes `DefaultConfigDir = "/config"`. **There is no published image — the README says so — so bare metal is the only install path that exists today, and `/config` is not a working default there.** Running as root silently creates both at `/`; `make dev` with no `.env` at all still creates `/config`; as a non-root user it hard-fails instead (`usarr: create …: mkdir …: permission denied`, exit 1) | **Open — recorded here rather than applied.** **Fix shape:** comment out the two lines in `.env.example` (they are the container values, and the container does not exist yet), and give `DEVELOPMENT.md` §3 a local value — the Makefile **already defines `DEV_CONFIG_DIR ?= ./.dev/config`** for `make migrate`, so the right answer is already in the tree and is simply not offered to the reader. The stray directories were removed by the reviewer |

**DL-01, DL-02, DL-03 and SR-01 — the proving commands, carried verbatim:**

```
$ grep -rn "EvictExpired" --include=*.go .
./internal/releases/service.go:123:func (s *Service) EvictExpired(ctx context.Context) (int64, error)
./internal/releases/search_test.go:691:func TestEvictExpired(t *testing.T) {
./internal/releases/search_test.go:704:	n, err := svc.EvictExpired(context.Background())

$ grep -n "go func\|Worker\|maintenance" cmd/usarr/app.go cmd/usarr/main.go cmd/usarr/services.go
cmd/usarr/main.go:123:	go func() {          # http.Serve
```
The only background goroutines in the process are `registry.RunProber` (`cmd/usarr/main.go:105`) and
the HTTP server. Nothing sweeps `release_candidate`. Proven duplication:
```
$ go test ./internal/store -run TestZZDuplicateCandidates -v
    first insert ids=[1] err=<nil>
    second insert of the IDENTICAL (service_instance_id, guid) ids=[2] err=<nil>
    rows with guid='same-guid': 2
```

DL-02, against the live schema:
```
$ PRAGMA table_info(release_candidate)   # via harness
id, work_id, service_instance_id, guid, title, indexer, indexer_id, protocol,
categories, size_bytes, seeders, leechers, age_days, quality, download_url,
info_url, info_hash, download_client_id, raw_release_json, rejected,
rejection_reasons, fetched_at, expires_at        <- no user_id
```
Same for `provenance` (`PRAGMA table_info(provenance)`: no `user_id`, and no `service_instance_id`).

DL-03, both halves:
```
READ  dsn: file:/var/lib/usarr/usarr.db?_pragma=busy_timeout%285000%29&_pragma=journal_mode%28WAL%29&_pragma=synchronous%28NORMAL%29&_pragma=foreign_keys%28ON%29&_pragma=temp_store%28MEMORY%29&_pragma=wal_autocheckpoint%281000%29&_pragma=cache_size%28-8000%29&_timefmt=sqlite
WRITE dsn: ...&_txlock=immediate

$ go test ./internal/db -run TestZZReadPoolCanWrite -v
    INSERT via d.Read(): err=<nil>
    FINDING: the read pool is NOT read-only; it accepted a write
    rows written via read pool: 1
    INSERT inside ReadTx (sql.TxOptions{ReadOnly:true}): err=sqlite3: attempt to write a readonly database

$ go test ./internal/store -run TestZZDeferredUpgradeOnReadPool -v
    deferred read tx upgrading to a write while the writer holds the lock:
      err=sqlite3: database is locked after 26.655µs
    FINDING CONFIRMED: busy_timeout=5000 did NOT rescue it (returned in 89.697µs, not 5s)
```

SR-01:
```
$ go test ./cmd/usarr/ -run TestAudit_CredentialsPersistedInReleaseCandidate -v
info_url        = "https://tracker.example/details/1234?apikey=SECRETPROWLARRADMINKEY0123456789abcdef"
raw_release_json= {...,"infoUrl":"https://tracker.example/details/1234?apikey=SECRETPROWLARRADMINKEY0123456789abcdef",...}
FINDING: release_candidate.info_url stores the credential VERBATIM in SQLite (redacted only on the way out)
FINDING: raw_release_json still carries the credential (inside infoUrl)
the raw SQLite file (4096 bytes) contains the plaintext admin key 0 time(s)
  -wal: 263712 bytes, key appears 2 time(s)
```

**FI-02, FI-03, FI-04 and FI-05 — the proving commands, carried verbatim:**

```
$ git clone …; cp .env.example .env; make tools; make check-offline
> usarr-web@0.0.0 format:check …/web
> prettier --check .
[error] Cannot find package 'prettier-plugin-svelte' imported from …/web/noop.js
 ELIFECYCLE  Command failed with exit code 1.
 WARN   Local package.json exists, but node_modules missing, did you mean to install?
make: *** [Makefile:262: fmt-check] Error 1
EXIT=2      (0.5 s in)

$ go env GOBIN GOPATH
              (empty)
/root/go
$ ls /root/go/bin
gitleaks  gofumpt  golangci-lint  goose  govulncheck
$ echo $PATH
/root/.local/bin:/root/.cargo/bin:/usr/local/go/bin:/opt/node22/bin:/opt/maven/bin:/opt/gradle/bin:/opt/rbenv/bin:/root/.bun/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

$ make fmt-check          # default PATH, no override
/bin/bash: line 1: gofumpt: command not found
make: *** [Makefile:261: fmt-check] Error 127

$ which -a golangci-lint
/usr/local/bin/golangci-lint
$ golangci-lint --version
golangci-lint has version 2.5.0 built with go1.25.1 from ff63786c on 2025-09-21T19:04:05Z
$ /root/go/bin/golangci-lint --version
golangci-lint has version 2.12.2 built with go1.25.13 …
$ /usr/local/bin/golangci-lint run
0 issues.
EXIT=0

$ export PATH="/root/go/bin:$PATH"; golangci-lint run
… (4 gosec hits omitted — owned elsewhere) …
internal/httpapi/redact_test.go:19:28: net/http/httptest.NewRequest must not be called. use net/http/httptest.NewRequestWithContext (noctx)
internal/httpapi/redact_test.go:37:22: net/http/httptest.NewRequest must not be called. use net/http/httptest.NewRequestWithContext (noctx)
internal/httpapi/server_test.go:153:27: net/http/httptest.NewRequest must not be called. use net/http/httptest.NewRequestWithContext (noctx)
7 issues:
* gosec: 4
* noctx: 3
EXIT=1

# FI-11 — the same tree, the same pinned binary, the caps removed.
# Re-run from scratch while writing this entry, on cb57e43, after `golangci-lint cache clean`,
# and it reproduced both numbers and all four hidden paths exactly:
$ golangci-lint run --max-issues-per-linter=0 --max-same-issues=0
11 issues:
* gosec: 4
* noctx: 7
# the four the capped run hid, all httptest.NewRequest:
#   internal/httpapi/services_urlbase_test.go:105
#   internal/web/web_test.go:213
#   internal/web/web_test.go:229
#   internal/web/web_test.go:317

# which cap is doing it, isolated:
$ golangci-lint run --max-issues-per-linter=0      →  7 issues   (not this one; default is 50)
$ golangci-lint run --max-same-issues=0            → 11 issues   (this one; stock default is 3)

# and the config on the tree this entry is written against sets neither:
$ grep -n "issues\|max-same\|max-issues" .golangci.yml
(no match — the file is 33 lines: version, linters, one gosec/_test.go exclusion, formatters)

$ ls -d /config /data
ls: cannot access '/config': No such file or directory
ls: cannot access '/data': No such file or directory
$ ./usarr --env-file .env          # .env is a verbatim copy of .env.example
$ ls -d /config /data
/config
/data
$ ls -la /data
drwx------ 3 root root 4096 … logs
```

### 1.3 Medium

| # | Finding | Disposition |
|---|---|---|
| **SR-02** | **The TOFU SPKI pin is never recorded; the pinned-TLS path is dead in production.** `internal/httpapi/services.go:172` stores `TLSSPKIPin: result.TLSSPKIPin`, but the only `ConnectionTester` implementation — `registry.Test`, `cmd/usarr/services.go:211-299` — never sets `TestResult.TLSSPKIPin` (declared at `internal/httpapi/ports.go:81-83`). So `ssrf.Options.SPKIPin` at `cmd/usarr/services.go:172` is always nil and `internal/ssrf/ssrf.go:213-249`'s pinned branch never executes. Direction is fail-closed (full chain verification instead), but a documented control (`ssrf.go:79-104`, *"the supported alternative to `verify_tls=0`"*) does not exist — **a self-signed homelab HTTPS instance simply cannot be added** | **Open — recorded here rather than applied.** **Fix shape:** populate `TestResult.TLSSPKIPin` in `registry.Test`, then fix the second bug in the same area or the pin still will not apply — `cmd/usarr/services.go:245` assigns `si.TLSSPKIPin` **only** in the stored-key branch, so a test with a typed key would not apply the pin even once the column is populated. Distinct from the G123 session-resumption item excluded in §3, and distinct from Round 2's SSRF-02, which documented a `ClassDerived` constraint on a pin that — as this finding shows — is never set in the first place |
| **SR-03** | **CSRF is a naive double-submit: the token is not bound to the session.** `internal/httpapi/auth.go:229-241` compares the `usarr_csrf` cookie against the `X-CSRF-Token` header and nothing else. Any self-consistent pair passes. Precondition for exploitation is cookie-write on the UsArr host — a sibling subdomain, or a network position on the **documented default deployment** (plain HTTP on a LAN, per the reasoning quoted at `auth.go:293-305`); the cookie has `Path=/` and no `__Host-` prefix | **Open — recorded here rather than applied.** The reviewer is explicit that this is **defence-in-depth, not a working cross-site attack today**: `Content-Type: application/json` is required (`auth.go:219-227`) and there is no CORS handler anywhere — `grep -rn "Access-Control" --include=*.go internal/ cmd/` returns nothing — so a cross-origin `fetch` fails preflight. **Fix shape:** HMAC the token over the session id, or store the expected token in the session row. Not the cookie-flags item excluded in §3 |
| **DL-04** | **`service_instance.name UNIQUE` is not partial — a soft-deleted instance permanently burns its name.** `internal/db/migrations/00001_initial.sql:136` — `name TEXT NOT NULL UNIQUE`, a table-level constraint (`sqlite_autoindex_service_instance_1`, `PRAGMA index_list(service_instance)`), not `CREATE UNIQUE INDEX … WHERE deleted_at IS NULL`. `SoftDeleteServiceInstance` (`internal/store/serviceinstance.go:240`) only sets `deleted_at`, so the row and its name survive: delete "Prowlarr", try to re-add "Prowlarr", get a constraint error with no way to clear it from the UI. **The tombstone design intends the `id` to be burned, not the human-facing label** | **Open — recorded here rather than applied.** **Fix shape:** replace the table-level constraint with a partial unique index in a later migration — which means a table rebuild, since 0001 is frozen. Reviewer's completeness note, not itself a finding: `base_url` has **no** uniqueness at all (`duplicate base_url, different name: err=<nil>`), which is probably right for `url_base`-differentiated deployments |
| **DL-05** | **Three FK child columns have no index — a cascading delete full-scans the largest table.** `internal/db/migrations/00001_initial.sql:201` and `:224` declare `ON DELETE CASCADE` from `service_instance`; `:271` declares `ON DELETE CASCADE` from `user` on `tag_assignment`. `ix_ta_tag` is `(tag_id, user_id, work_id)` — `user_id` is not leading, so it cannot serve the cascade | **Open — recorded here rather than applied.** Latent today: `service_instance` is only ever soft-deleted, so the cascade never fires. It becomes real the moment a hard purge, a rotation cleanup or a user delete ships against a `release_candidate` table that DL-01 has let grow — which is the pairing worth routing together. **Fix shape:** three `CREATE INDEX` statements in a later migration, purely additive, no rebuild |
| **DL-06** | **Every mutating store method takes no scope parameter — authorization is a separate preceding read.** Reads carry scope in the signature exactly as `internal/store/store.go:15-19` demands; the four mutations do not — `UpdateServiceInstance` (`internal/store/serviceinstance_update.go:35`), `UpdateServiceInstanceCredential` (`serviceinstance.go:202`), `UpdateServiceInstanceHealth` (`:215`), `SoftDeleteServiceInstance` (`:240`), each issuing only `WHERE id = ? AND deleted_at IS NULL`. The handlers do gate them — `internal/httpapi/services.go:225` reads with `storeScope(a)` before the update at `:312`, and `:344` before the delete at `:351` — but that is check-then-act across two pools and two transactions, and the safety lives at the call site rather than in the query | **Open — recorded here rather than applied.** **Not exploitable in v0.1** (`OwnerScope` matches everything) — this is a seam finding, and the store package's own stated rule is "in the query signature, not bolted on later", so these four are the bolted-on case. **Fix shape:** thread `Scope` into the four signatures and into the `WHERE`, before multi-user makes it a migration-shaped problem |
| **DL-07** | **`docs/reference/schema.md` §13 and `ARCHITECTURE.md` §6.5 both assert the four library tables are "all in migration 0001". They are not** — `schema.md:1071`, `:1074` (*"**All four tables are in migration 0001**, which `CLAUDE.md` says can never be edited"*), `:1148` (*"Reserved row: `library.id = 0`, Unfiled. Inserted by migration 0001"*), `ARCHITECTURE.md:1007` (*"**Four tables, all in migration 0001.**"*). The live schema after `d.Migrate(ctx)` has no `library`, `library_source`, `library_member` or `library_override`. `schema.md:3-8`'s own status header lists the correct ten tables, **so the document contradicts itself.** Worse: `internal/db/migrations/00001_initial.sql:11-14` enumerates what is "deliberately absent" and **omits all four**, so a reader working from the migration header would conclude they exist | **Open — recorded here rather than applied.** **Fix shape:** correct §13 and §6.5, and add the four to the migration header's deferred list — the header is the artefact an implementer actually reads. Two riders in the same edit: **(a)** `ARCHITECTURE.md:798`, `:903`, `:2349` and `schema.md:191,222,284,305` all argue "do X in migration 0001 because doing it later is a backfill" — 0001 shipped without any of them, so those cost arguments are now moot and everything lands in 0002+ regardless; the docs should say so rather than keep arguing about a moment that has passed. **(b)** `ARCHITECTURE.md:1310` §7.7 items **3** ("a priority scheduler in front of the single writer" — `db.Write`, `internal/db/sqlite.go:171`, is a plain `BeginTx` on a 1-connection pool, FIFO by `database/sql`'s waiter queue) and **5** (`ANALYZE` after bulk import — appears only in `internal/db/spike/fixture.go:267`, never on a production path) are unimplemented; both belong to sync, which has not shipped, so this is a "§16 wins" note rather than a defect |
| **DL-08** | **`VerifyAuditChain` is an unbounded full-table read, and nothing prunes `audit_log`.** `internal/store/audit.go:156` issues `SELECT … FROM audit_log ORDER BY id ASC` with no `LIMIT`, no cursor, no checkpointing — `VerifyAuditChain audit.go:159 → SCAN audit_log`, O(n) forever, on a table that is append-only by trigger (`trg_audit_no_delete`) and has no retention job. Every login writes a row | **Open — recorded here rather than applied.** **Fix shape, and the sequencing is the point:** the index a retention sweep needs already exists (`audit retention sweep → SEARCH audit_log USING COVERING INDEX ix_audit_ts (ts<?)`), but the `BEFORE DELETE` trigger at `internal/db/migrations/00001_initial.sql:119` will `RAISE(ABORT)` on it — so any pruning needs a **new migration** replacing the trigger with one that permits a bounded, audited prune. Worth deciding now, while `audit_log` is empty everywhere. Verification itself wants a cursor or a checkpoint independently |
| **DS-05** | **§3.5's key-loss recovery procedure describes a state the schema and code do not have.** `docs/CONFIGURATION.md:307-308` — *"UsArr marks every instance whose credential fails to open as `needs_credential`. Open each in the UI and paste the key again."* Repeated at `:615` for Navidrome. `grep -rn "needs_credential" --include=*.go --include=*.svelte --include=*.ts .` → **no matches anywhere**. `internal/httpapi/services.go:540-546` enumerates the five states the API can return: `healthy`, `degraded`, `down`, `needs re-identification`, `unknown`; the real behaviour, reproduced by two passes, is `state: "down"` with `action: "Update API key"` | **Open — recorded here rather than applied.** The behaviour is close enough to be usable — the doc names a **token that does not exist**. **Fix shape:** replace the token with what the API returns, or mark §3.5 the way §3.4 (`:274`) already marks the surrounding recovery flow, *"(proposed CLI, v0.1)"*. §3.5 carries no marker at all. Routes with DS-01, whose reproduction runs through this same code path |
| **DS-06** | **§2.1's redaction deny-list is a stale subset of the one deny-list.** `docs/CONFIGURATION.md:128-131` lists nine names (`apiKey`, `api_key`, `apikey`, `token`, `access_token`, `sig`, `p`, `t`, `s`) plus `Authorization` and `X-Api-Key`. `internal/ssrf/redact.go:39-64` has **nineteen** — adding `auth_token`, `signature`, `secret`, `secret_key` and the seven private-tracker names `passkey`, `torrent_pass`, `torrentpass`, `rsskey`, `authkey`, `apipasskey`, `cookie`. `internal/httpapi/redact.go:29` redacts **four** headers, not two (adds `Cookie`, `X-CSRF-Token`) | **Open — recorded here rather than applied.** This is the drift `internal/ssrf/redact.go:21-22` explicitly warns about — *"ARCHITECTURE.md §14.5 item 5 and security.md §5 document it and must be updated together with it"* — and it is Round 2's W-01 leaving one copy behind: `docs/ARCHITECTURE.md:1984-1993` and `docs/reference/security.md:314-317` both carry the **complete** list, and CONFIGURATION.md §2.1 is a **fourth copy nobody registered as one**. **Fix shape:** update §2.1, and add it by name to `redact.go:21-22`'s maintenance contract so the next W-01 updates three files and not two. Also `:131` — the doc says values are *"replaced with `<redacted>`"*; `<redacted>` is the **header** placeholder (`internal/httpapi/redact.go:32`), query-parameter values become `REDACTED` (`internal/ssrf/redact.go:70`) |
| **DS-07** | **§16 says the Services UI does not exist; a 677-line Services screen ships.** `docs/ARCHITECTURE.md:2203` — *"the Services health **screen** (its endpoint exists, the UI does not)"*; `README.md:70` — `\| **Services health screen** … \| 📋 Planned — v0.1 \|` with no partial marker, though the README uses `🚧 Partial` elsewhere (`:66`). `web/src/routes/services/+page.svelte` is 677 lines: it lists instances, renders `state` + verbatim `problem` from `GET /api/v1/services/health`, adds, edits, re-tests, removes, and handles the sudo re-prompt. `web/src/routes/+page.svelte:39` tells a fresh user *"Start on Services"* — because without it there is no way to add a Prowlarr instance at all | **Open — recorded here rather than applied.** The file's own header (`:3-15`) is precise and honest about what it is — *"the SCAFFOLDING version… deliberately NOT the screen §17.3 specifies… Delete this file wholesale when §17.3 lands"* — §16 and the README simply have not absorbed it. **Fix shape:** §16 gains a partial/scaffolding row and the README's row moves to `🚧 Partial`. This is the rare direction where §16 **understates**, and it understates by exactly the amount that matters to a first-time user: whether the install is usable without `curl`. Corroborated by the fresh-install pass, which drove the whole add-a-service flow through the shipped screen |
| **DS-08** | **`HttpOnly; Secure; SameSite=Lax` is stated unconditionally in two docs; `Secure` is conditional in code.** `docs/ARCHITECTURE.md:1786` and `docs/reference/security.md:378`, identical wording. `internal/httpapi/auth.go:306-308` — `secureCookies` returns `clientOf(r.Context()).scheme == "https"`, so `Secure` is omitted on plain HTTP | **Split. The code half is already dispositioned; the doc half is Open.** The reviewer states plainly that **the code is right and the docs are wrong**, and the argument at `auth.go:293-305` is the same one Round 2 accepted at **W-02**: `CONFIGURATION.md:630` (§8.1) makes plain HTTP on a trusted LAN the documented v0.1 default, browsers discard `Secure` cookies over http, so an unconditional `Secure` means nobody can log in on the default deployment. **Recorded as a deliberate decision rather than a defect**, cross-referenced to W-02. What is **still open** is exactly what W-02 did not do: the carve-out exists only as a Go comment, and neither of the two documents a reviewer actually reads records it. **Fix shape:** one sentence in each of `ARCHITECTURE.md:1786` and `security.md:378`. Independently corroborated by the security pass, which verified by execution that both cookies carry `Secure=true` over HTTPS |
| **DS-09** | **§17.3's state vocabulary and the code's disagree in both directions.** `docs/ARCHITECTURE.md:2489` names four states (`healthy` / `degraded` / `down` / `needs re-identification`); `:2493` asserts *"`not configured` is already a first-class state with its own token"*. `internal/httpapi/services.go:540-546` defines **five** — the four plus `unknown`, returned for a disabled instance (`:679`) and as the fallback (`:694`) — and `not configured` **exists nowhere in the codebase**. The function's own doc comment at `:668` says *"derives the four states"* while its body returns five | **Open — recorded here rather than applied.** Both directions are wrong, which is why it is one entry and not two. **Fix shape:** §17.3 gains `unknown` and drops the `not configured` claim; `services.go:668`'s doc comment says five. **Low-severity rider, flagged as INFERENCE by the reviewer:** §17.3's plain-language amendment (`:2503-2509`) requires `paused — 7 failed attempts, retrying 14:19` rather than `degraded / breaker open`, and `this may be a different Sonarr` rather than `needs re-identification`, while the API returns the raw tokens. That is *plausibly* intended as a UI-layer rendering concern and the §17.3 screen is not built — but **nothing in the code or docs says which layer owns it**, and that is the part worth settling |
| **FI-06** | **No single-instance lock: two processes will share one config directory and one database.** Both start, both serve, both write. The port collision **is** caught (`usarr: USARR_BIND_ADDRESS/USARR_PORT: cannot listen on 0.0.0.0:18484: bind: address already in use`, exit 1) — a *different* port on the same volume is not | **Open — recorded here rather than applied.** This defeats the documented single-writer discipline from outside the process, where DL-03 defeats it from inside, and it would break key rotation mid-flight. The realistic first-run mistake is a systemd unit plus a manual `./usarr` to "just check something". **Fix shape:** a `flock` on the config directory, failing closed with a message naming the other PID |

**SR-02, SR-03, DL-04…DL-08 and FI-06 — the proving commands, carried verbatim:**

```
$ go test ./cmd/usarr/ -run TestAudit_SPKIPinIsNeverRecorded -v
service_instance.tls_spki_pin = 0 bytes (), verify_tls = true
FINDING: the column is empty after a successful connection test, so ssrf.Options.SPKIPin
is always nil and internal/ssrf's pinned-TLS path never runs in production.

$ go test ./cmd/usarr/ -run TestAudit_CSRFTokenSessionBinding -v
POST /auth/sudo with attacker-planted cookie+header pair -> 200 {"authenticated":true,...,"sudo_until":"2026-08-16T16:29:37.73155949Z"}
FINDING: the CSRF token is a NAIVE double-submit — any self-consistent cookie/header pair passes;
it is not bound to the session id.

$ go test ./internal/store -run TestZZDuplicateCandidates -v
    soft delete: err=<nil>
    re-create with the SAME name after soft delete:
      err=sqlite3: constraint failed: UNIQUE constraint failed: service_instance.name

$ go test ./internal/store -run TestZZFKChildIndexes -v
release_candidate.service_instance_id (FK child)   SCAN release_candidate   <<< SCAN
write_queue.service_instance_id (FK child)         SCAN write_queue   <<< SCAN
tag_assignment.user_id (FK child)                  SCAN tag_assignment USING COVERING INDEX ix_ta_tag   <<< SCAN
write_queue.user_id (FK child)                     SEARCH write_queue USING COVERING INDEX ux_wq_idem (user_id=?)
tag_assignment.service_instance_id (FK child)      SEARCH tag_assignment USING COVERING INDEX ix_ta_inst_lookup (service_instance_id=?)
tag_assignment.tag_id (FK child)                   SEARCH tag_assignment USING COVERING INDEX ix_ta_tag (tag_id=?)
session.user_id (FK child)                         SEARCH session USING INDEX ix_session_user (user_id=?)
client_credential.user_id (FK child)               SEARCH client_credential USING COVERING INDEX ix_cc_user (user_id=?)

$ go test ./internal/db -run TestZZUpDownUp -v
after UP objects (40): [table audit_log, table client_credential, ..., table user, table write_queue]
# no library, library_source, library_member, library_override

VerifyAuditChain audit.go:159
    SCAN audit_log   <<< SCAN ON A GROWING TABLE
audit retention sweep    SEARCH audit_log USING COVERING INDEX ix_audit_ts (ts<?)

$ ./usarr --config-dir $SP/run1 --port 18484 &     # instance A
$ ./usarr --config-dir $SP/run1 --port 18686 &     # instance B, same config dir
{"level":"INFO","msg":"master key loaded","source":"key file",…}
{"level":"INFO","msg":"listening","address":"0.0.0.0:18686",…}
$ curl :18484/api/health/ready → {"status":"ready",…}
$ curl :18686/api/health/ready → {"status":"ready",…}
port 18484 login HTTP=200
port 18686 login HTTP=200
$ pgrep -a usarr
26912 ./usarr --config-dir …/run1 --port 18484
29216 ./usarr --config-dir …/run1 --port 18686
```

### 1.4 Low

| # | Finding | Disposition |
|---|---|---|
| **SR-04** | **`crypto.NormalizeHostPort` and `ssrf.canonicalHostPort` disagree on zero-padded ports.** `internal/crypto/derive.go:216-221` round-trips the port through `strconv.Atoi`, stripping leading zeros; `internal/ssrf/policy.go:199-206` does not touch the port. `derive.go:170-181` and `:241-245` both state the two must agree | **Open — recorded here rather than applied.** **Fails closed** (an unusable instance and a misleading error), never open. **Fix shape:** canonicalise the port on one side — most cheaply by having `ssrf` parse it the same way — or reject zero-padded ports at `normalizeBaseURL` with SR-05. Sits one line away from DS-12, which is about the same function's godoc; route them together |
| **SR-05** | **Userinfo in `base_url` is stored in cleartext and hidden by redaction.** `normalizeBaseURL` (`internal/httpapi/services.go:744-755`) returns the raw string; `crypto.NormalizeOrigin` discards userinfo (`derive.go:208`, `u.Hostname()`), so the §1.6 re-entry check sees no change; `toServiceResponse` (`services.go:64`) redacts it back out | **Open — recorded here rather than applied.** **No outbound `Authorization: Basic` is produced**, so this is a cleartext secret at rest plus a DB/UI disagreement, not a credential channel. **Fix shape:** reject userinfo in `normalizeBaseURL` |
| **SR-06** | **Secret-bearing structs lack the log guard `config.Config` has.** `config.Config` has both `LogValue` (`internal/config/config.go:125`) and `String` (`:148`) — Round 2's CFG-01. `httpapi.TestRequest.APIKey` (`internal/httpapi/ports.go:55`) and `servarr.Options.APIKey` (`internal/servarr/client.go:65`) have neither | **Open — recorded here rather than applied.** **Latent only:** `grep -rn '%+v\|%#v' --include=*.go internal/ cmd/ \| grep -v _test.go` finds **no production call site**. **Fix shape:** the CFG-01 treatment on both structs — value receivers, explicit allow-list of fields, so a future secret field is invisible until someone adds it |
| **DL-09** | **`Owner()` (`internal/store/user.go:121`) plans `SCAN user`** and sits on the SPA bootstrap path (`internal/httpapi/auth.go:331`, called on every page load) | **Recorded as a deliberate decision rather than a defect — the reviewer's own framing.** Two rows today, <50 at v1.0; genuinely fine. Noted only so it becomes a *recorded* decision the way `TestServiceInstanceListScanIsIntentional` made the `service_instance` scan one |
| **DL-10** | **No session pruning and no index that would serve one** — `session expiry sweep → SCAN session`. `GetSession` filters expired rows out but nothing deletes them, so `session` grows one row per login forever | **Open — recorded here rather than applied.** **Fix shape:** routes with DL-08 — same decision (retention), same migration window, and unlike `audit_log` there is no append-only trigger in the way, only a missing index |
| **DL-11** | **`write_queue` settled-row sweep would scan** — `SELECT id FROM write_queue WHERE state='done' AND settled_at < ?` → `SCAN write_queue`. `ix_wq_runnable` is partial on `('pending','inflight','verifying')` and deliberately excludes `done`/`failed`, so terminal rows are unreachable by index | **Open — recorded here rather than applied.** Latent until the queue ships. **Fix shape:** decide it at the same rebuild WQ-04 already schedules — that entry already requires all three `write_queue` indexes to be recreated by hand, so a fourth index is the cheapest it will ever be. **Distinct from the excluded `awaiting_choice` item** (§3): this is about terminal rows, that one is about non-terminal ones |
| **DL-12** | **`schema.md` §6/§8/§10 print the DDL with `REFERENCES work(id) ON DELETE CASCADE`, `REFERENCES edition(id)`, `REFERENCES media_file(id)` and `REFERENCES tag_rule(id)` still attached.** Migration 0001 drops all six, documented in the migration and not in `schema.md` | **Open — recorded here rather than applied.** Strengthened by a mechanical diff the reviewer ran: across `schema.md`'s ten shipped `CREATE TABLE` blocks against the live `sqlite_schema`, **those six clauses are the only differences** — every other column, type, `NOT NULL`, `DEFAULT` and `CHECK` matches byte-for-byte after whitespace/comment normalisation. **Fix shape:** one line in `schema.md`, so a reader does not copy the reference DDL into migration 0002 |
| **DL-13** | **`sqlite_sequence` survives `Down`** — goose's `goose_db_version.id INTEGER PRIMARY KEY AUTOINCREMENT` creates it | **Open — recorded here rather than applied, and cosmetic.** The repo's own `userObjects` filters `sqlite_%`, so `TestMigrationDownLeavesNothingBehind` is unaffected. `goose_db_version` is also the only non-`STRICT` table in the file, and it isn't ours |
| **DS-10** | **`CLAUDE.md:175`** — *"`docs/reference/` \| Vendored upstream specs and captured API reference material."* `docs/reference/` holds nine `.md` files and **zero specs**; the single vendored spec is `api/specs/prowlarr.json` (with `api/specs/SOURCES.md`) | **Open — recorded here rather than applied.** An agent following `CLAUDE.md`'s table looks in the wrong directory. **Fix shape:** one-line correction — but note `CLAUDE.md` is the project's instruction file, and Round 2 (§2.2) already established it is **left for the owner** rather than edited on an agent's say-so |
| **DS-11** | **`docs/DEVELOPMENT.md:98`** — the layout block names `api/specs/prowlarr.v1.json`; the actual file is `api/specs/prowlarr.json`, and `internal/servarr/contract_test.go:50` reads that exact path | **Open — recorded here rather than applied.** §2 is labelled "(target)", so this is naming drift rather than a false claim — but the one spec that *does* exist is named differently from the target it is supposed to be an instance of. **Fix shape:** align the target on the shipped name |
| **DS-12** | **`internal/crypto/derive.go:164-165`** — the godoc first line reads *"NormalizeHostPort reduces a service base URL to the canonical host:port that goes into the AAD."* Its own body ten lines later (`:174`) says *"The AAD does not use this function. It uses NormalizeOrigin."* | **Open — recorded here rather than applied.** The first line is what godoc shows, and it contradicts `docs/reference/security.md:57-61`, which warns implementers **by name** that using `NormalizeHostPort` for the AAD comparison produces credentials that can never be opened again — i.e. it points a reader at exactly the trap Round 2's CRYPTO-01 closed. **Fix shape:** rewrite the first godoc line. Route with SR-04, same function |
| **DS-13** | **`docs/CONFIGURATION.md:412`** — §5's example backup filename is `usarr-2026-08-16T03-00-00Z.db`; `cmd/usarr/backup.go:73-74` produces `usarr-pre-migration-<ts>-v<N>.db` | **Open — recorded here rather than applied.** The §5 name presumably describes the not-yet nightly job; the only backups a user will actually see today are the pre-migration ones. **Fix shape:** show the real name and mark the nightly one as not-yet. Routes with DS-01 — same section, same edit |
| **DS-14** | **`docs/ARCHITECTURE.md:2193-2199`'s "Landed so far" list omits four shipped endpoints** — `GET /api/v1/system/status`, `GET /api/v1/auth/session`, `POST /api/v1/auth/sudo` and the whole sudo re-authentication window (`internal/httpapi/server.go:189,196,200`; `internal/httpapi/auth.go:56-62`) | **Open — recorded here rather than applied.** **Sudo mode is a substantive shipped security control that §16 does not record as landed** — the same understatement direction as DS-07, and §16 is the authoritative roadmap. **Fix shape:** four rows added to the landed list |
| **FI-07** | **`--help` exits 1 and prints an error line first** — `usarr: parse flags: flag: help requested`, then the usage block; `./usarr --help >/dev/null; echo $?` → `1` | **Open — recorded here rather than applied.** Small but it is the very first command a new user runs, and a non-zero `--help` breaks scripts and packaging checks. **Fix shape:** catch `flag.ErrHelp` in `cmd/usarr`'s flag parsing, print usage to stdout and exit 0 |
| **FI-08** | **No `healthcheck` subcommand, but the README's compose block uses one** — `test: ["CMD","/usarr","healthcheck"]`; `./usarr healthcheck` → `usarr: unexpected argument "healthcheck"`, exit 1 | **Open — recorded here rather than applied, and low precisely because the block is marked a placeholder** and there is no published image for it to run in. **Fix shape:** when the image lands, either the subcommand ships or the compose block uses `wget`/`curl` against `/api/health/ready`, which already exists and answers correctly |
| **FI-09** | **A failed key validation still creates `backups/ keys/ logs/ providers/` before exiting** — `ls -la` after a placeholder-key refusal shows all four directories. `cmd/usarr/app.go:44` runs `ensureDirs` before the master-key ladder | **Open — recorded here rather than applied.** Harmless in itself, and mildly confusing: a refused start leaves a populated-looking config directory behind. **Fix shape:** run the ladder before `ensureDirs`, or clean up on the refusal path. Weigh against FI-05, where the same eagerness is what creates `/config` at the root |
| **FI-10** | **The container ships Go 1.24.7; `go.mod` requires 1.25.13, satisfied only by `GOTOOLCHAIN=auto` downloading a toolchain** — `GOTOOLCHAIN=local go build ./cmd/usarr` → `go: go.mod requires go >= 1.25.13 (running go 1.24.7; GOTOOLCHAIN=local)` | **Recorded as environmental, not a repo defect.** The floor is correct and Round 2's B-06 established why (15 called stdlib vulnerabilities below 1.25.13). Recorded because it has one real consequence worth knowing: **an air-gapped machine, or any builder with `GOTOOLCHAIN=local`, cannot build UsArr without first installing 1.25.13 by hand**, and nothing in `DEVELOPMENT.md` says so |

**SR-04, SR-05 and SR-06 — the proving commands, carried verbatim:**

```
$ go test ./cmd/usarr/ -run TestAudit_ZeroPaddedPortDivergence -v
crypto.NormalizeHostPort("http://127.0.0.1:041883") = "127.0.0.1:41883"
ssrf.NormalizeHostPort("http://127.0.0.1:041883")   = "127.0.0.1:041883"
FINDING: the two normalisers disagree — the AAD/allowlist pair the code says must agree, does not
POST /services with a zero-padded port -> 502 {"error":"connection_test_failed","message":"... ssrf validate [configured]: ssrf: host not allowed for this request class (127.0.0.1:041883)"}

$ go test ./cmd/usarr/ -run 'TestAudit_UserinfoInBaseURL|TestAudit_UserinfoBecomesOutboundBasicAuth' -v
PATCH base_url="http://someuser:somepass@127.0.0.1:44077" (same origin, no api_key) -> 200
service_instance.base_url in SQLite = "http://someuser:somepass@127.0.0.1:44077"
what the API shows                  = {..."base_url":"http://127.0.0.1:44077"...}
  RECEIVED: GET /api/v1/system/status X-Api-Key="TYPED" Authorization="" query="" body=""

$ go test ./internal/httpapi/ -run TestAudit_SecretBearingStructsHaveNoLogGuard -v
fmt.Sprintf("%+v", httpapi.TestRequest{...}) = {InstanceID:0 Kind:prowlarr BaseURL:http://nas:9696 URLBase: APIKey:PLAINTEXT-ADMIN-KEY}
slog.Any("req", TestRequest{...})            = ... req="{... APIKey:PLAINTEXT-ADMIN-KEY}"
```

### 1.5 Informational and inference

The security pass labelled these itself, and they are **not counted as defects**. Recorded so the
trade-off in each is explicit rather than assumed, and so the two inferences are not mistaken for
executed results.

| # | Item | Disposition |
|---|---|---|
| **SR-07** | **Sudo is granted automatically at login** (`internal/httpapi/auth.go:477-480`), so the gate only bites more than 5 minutes after sign-in. Documented intent, but a cookie stolen inside the window is enough for credential operations | **Open — recorded here rather than applied, as an owner decision.** Not a defect against the spec; the question is whether the spec is what the owner wants |
| **SR-08** | **`handleLogout` clears `usarr_session` but not `usarr_csrf`** (`auth.go:499-508`) | **Open — recorded here rather than applied.** Small, and it pairs naturally with SR-03: a CSRF token bound to the session must be cleared with it |
| **SR-09** | **`ClassConfigured` has no address denylist at all** — `169.254.169.254`, `::1`, `fd00::1`, `100.100.100.100` all pass, executed. Deliberate per `internal/ssrf/policy.go:133-138` | **Recorded as a deliberate decision rather than a defect, and already dispositioned — this is Round 2's SUS-02**, resolved there as per-spec: `security.md` §2's table says `configured` "may reach private space, but only the exact validated host:port". Cross-referenced, not re-opened. **Two passes reached the same conclusion independently this round** — the security pass by execution, the fresh-install pass from `policy.go:132-137` and then live, by adding `http://127.0.0.1:19696` as a real instance |
| **SR-10** | **`crypto.AAD.Bytes()` has no length prefixes** (`derive.go:123-139`), so `{Table:"a", Column:"b:c"}` and `{Table:"a:b", Column:"c"}` render identical bytes | **Open — recorded here rather than applied.** Not exploitable today: `Table`/`Column` are hardcoded constants and `PrimaryKey` is numeric. It is a **latent collision** if a future column name ever contains a colon. **Fix shape:** length-prefix the fields, or a comment at the struct forbidding a colon in either constant — the cheap half is worth doing while the constants are few |
| **SR-11** | **INFERENCE, not executed — HTTP/2 connection coalescing.** `ForceAttemptHTTP2: true` (`ssrf.go:191`) means Go's h2 transport may reuse a connection for a different authority whose certificate covers it | **Recorded as an inference the reviewer could not test.** Their reasoning: for `ClassConfigured` the dialler already pins one host:port, and for `ClassProvider` both endpoints are public, so it should not cross a policy boundary — *"but I could not construct a test for it."* Left open, in the shape Round 2 used for SUS-01: an unverified mechanism is not a basis for shipping a change, and not a basis for closing the question either |
| **SR-12** | **INFERENCE, not executed — `VerifyPassword` takes Argon2 cost parameters from the stored PHC string** (`password.go:64`), so a hostile `user.password_hash` row could force a large `m=` on every login | **Recorded and effectively self-rebutted by the reviewer:** it requires DB write access, *"which is already game over."* Noted only so a future reader does not re-raise it as a remote memory-exhaustion vector — the one `CLAUDE.md` names is per-request Argon2id on API keys, and §2.4 records that as verified absent |

---

## 2. Rebuttals — checked, and found not to be defects

Recorded because a finding that is quietly ignored comes back, and a *non-finding* that is never
written down gets re-raised every round.

### 2.1 The missing `UNIQUE (service_instance_id, guid)` on `release_candidate` — rebutted, and its *premise* is now DL-01

The data prong looked at this directly and did **not** report it: *"`schema.md:677-686` argues it
explicitly and rejects the upsert alternative on grab-window grounds. That argument is sound."* That
is Round 2's **DB-03** decision holding up under a second, independent look, which is the outcome
DB-03's disposition was hoping for.

**What is attacked is not the decision but its stated bound.** DB-03's safety argument is *"the bound
on that duplication is the TTL"*, and DL-01 shows the TTL sweep has no production caller. The
decision is right; the sentence supporting it is currently false. Fixing DL-01 makes it true again —
which is the cheapest possible resolution, and the reason DL-01 is ranked High rather than Medium.

### 2.2 `AppendAudit`'s chain-head read is **not** a scan — a false alarm pre-empted

`EXPLAIN QUERY PLAN` reports `SCAN audit_log` for the chain-head read, which reads alarming. The
reviewer measured rather than assumed:

```
$ go test ./internal/store -run TestZZAuditChainHeadScaling -v
chain-head read @  10k rows: 12.246µs
chain-head read @ 100k rows: 11.849µs
chain-head read @ 300k rows: 13.065µs
```

`ORDER BY id DESC LIMIT 1` on the rowid takes the last b-tree entry and stops. **Not a finding** —
recorded here explicitly because the next reader running EQP will hit the same false alarm, and
because it is the counterpart to DL-08, where the `SCAN audit_log` **is** real (`ORDER BY id ASC`,
no `LIMIT`). The two look identical in the plan output and are not the same thing.

### 2.3 Conditional `Secure` on the session cookie is correct — the code is rebutted as a defect, the documents are not

Recorded at **DS-08** and repeated here because it is the third time it has come up (Round 2's W-02,
then both the doc pass and the security pass this round). The verdict is *"I think the code is right
and the docs are wrong"*, and the security pass's execution agrees the shipped behaviour is sound:
`usarr_session` `HttpOnly=true`, `usarr_csrf` `HttpOnly=false` as the double-submit design requires,
both `SameSite=Lax`, both `Secure=true` over HTTPS. **What survives as open is only the
documentation** — the carve-out lives in a Go comment and in this log, and in neither of the two
documents that state the rule.

### 2.4 What the five passes checked and cleared

Not rebuttals — **verified-correct properties**, recorded so the next round does not re-spend its
budget here and so nothing below is rediscovered as a suspicion. Each was executed or read against
primary source by the pass named.

**Crypto, executed** (`go test ./internal/crypto/ -run TestAudit_ -v`):

- **The AAD blocks row swapping, origin swapping and scheme downgrade** — all fail closed, e.g.
  `crypto: authenticated decryption failed: kek_id=1: service_instance.api_key_enc id=2 origin=http://radarr.lan:7878`.
  Round 2's CRYPTO-01 holding.
- **Key versioning is handled correctly:** `kek_id` 1→2 (held key) fails on the RFC 3394 integrity
  check; 1→99 gives `ErrUnknownKEK`; `Rewrap` round-trips cleanly and `KEKID` reports the new id.
- **No nonce or DEK reuse:** 20 000 seals → 20 000 distinct nonces, 20 000 distinct wrapped DEKs.
- **Tamper detection at every offset** (nonce, wrapped DEK, ciphertext, tag) — all `ErrDecrypt`, and
  decrypt errors leak no plaintext.
- **`NormalizeOrigin`'s equivalence classes are sane** — case, trailing dot, default ports,
  IPv4-mapped, path/query/userinfo all collapse correctly; `file:`, `gopher:`, no-host and
  port > 65535 rejected.
- **The envelope layout matches `security.md` exactly** —
  `kek_id(4) || nonce(12) || wrapped_dek(40) || ct || tag(16)` (doc pass, against `envelope.go:33-40`),
  and `kek_id` is a plain column as §1.1 requires.

**The master-key validation ladder, executed twice** — by the security pass
(`go test ./internal/config/ -run TestAudit_ -v`) and end to end by the fresh-install pass, which
drove all six refusal rows against the real binary and got the verbatim messages:

- placeholder → *"USARR_SECRET_KEY is the placeholder value shipped in .env.example. A key published
  in a git repository is not a key."*; empty → *"Empty is not the same as unset…"*; all-zero →
  *"decodes to 32 zero bytes, which is not a key"*; 31 bytes → *"decodes to 31 bytes, expected
  exactly 32. UsArr does not pad or truncate a key."*; not base64 → *"not valid base64 (standard
  alphabet, padding optional)"*; both `USARR_SECRET_KEY` and `_FILE` → *"they are mutually exclusive,
  and UsArr does not guess which you meant."* **All exit 1.** Placeholder detection survives
  surrounding whitespace, and no error text echoes key material.
- **The encrypted-rows-but-no-key guard fails closed** — key file removed with one credential stored
  → *"the database holds 1 encrypted credential(s) but no master key was supplied … Refusing to start
  half-decrypted."*, exit 1. **This is the guard DS-01 shows the salt does not have.**
- **A wrong key degrades honestly rather than silently** — starts, then `state:"down"` with the
  re-enter message, and search returns 502 `service_unavailable`.
- **`keys/` 0700, `secret.key` 0600, salt 32 B and stable, `O_EXCL` refuses to clobber an existing
  key**, and first run emits the loud back-it-up warning.

**SSRF, executed** (`go test ./internal/ssrf/ -run TestAudit_ -v`, plus the 37 pre-existing tests):

- **Resolve-then-pin has no re-resolution window** — with a resolver returning a different address per
  call, dials 1/2/3 went to `93.184.216.34/.35/.36`, each exactly the address validated in that same
  call, one resolver call per dial. **DNS rebinding across dials is caught.**
- **All 15 encoding/metadata/transition forms are blocked under `ClassProvider`**, including
  `169.254.169.254`, `[::ffff:127.0.0.1]`, `100.100.100.100`, NAT64 `[64:ff9b::7f00:1]`, 6to4
  `[2002:7f00:1::]`, and the legacy numerics (`2130706433`, `0177.0.0.1`, `0x7f000001`, `127.1`).
- **Redirects are re-validated at every hop** over the real `http.Client` — 302s to loopback, `[::1]`,
  `169.254.169.254` and `2130706433` all refused — **and `Referer` is never synthesised**, which is
  Round 2's SSRF-01 holding.
- **Allowlist canonicalisation** collapses IPv4-mapped, bracketed, case and trailing-dot spellings to
  one value. **No `100.64.0.0/10` denylist entry**, as `security.md` §2.1 requires (doc pass).

**HTTP surface and auth, executed** (`go test ./cmd/usarr/ -run TestAudit_ -v`):

- **CSRF gating covers every state-changing route** — all 10 (`auth/setup|login|logout|sudo`,
  `services` POST, `services/test`, `services/{id}` PATCH/DELETE, `services/{id}/test`,
  `releases/{id}/grab`): `no-token=403 wrong-token=403 form-ct=415 text-plain=415`, comparison by
  `subtle.ConstantTimeCompare` (`auth.go:236`).
- **The sudo gate is not bypassable** on all 5 credential-touching routes: 403 with the window
  closed, a failed `POST /auth/sudo` does not open it, a successful one does.
- **Re-entry on host change holds — the evil listener received 0 requests.** PATCH to a new host
  without a key → 400 `credential_reentry_required`; test against a new host → 400; blank key → 400;
  `https`↔`http` at the same port → 400; `url_base` traversal → 400. With a key typed in, the evil
  host saw only `X-Api-Key="TYPED-INTO-THE-FORM"`, never the stored key.
- **The API key never reaches the wire or the log** — 3 855 bytes of response transcript over 16
  responses and 3 603 bytes of *debug-level* log: 0 occurrences. Prowlarr's key-in-`downloadUrl`
  never reaches the SSE stream (5 421 bytes of frames). At rest: `api_key_enc` is a 110-byte envelope
  beginning `00000001`, `download_url` empty, `session.id` a 64-char sha256 hex, `audit_log.metadata`
  carries no key.
- **Argon2id is used for user passwords only** — `auth.go:382`, `:420/:425`, `:429`, `:525`, nothing
  else; only `internal/crypto/password.go:12` imports it. The keyed-hash path
  (`crypto.HashClientKey`, HMAC-SHA256 + `ConstantTimeCompare`) has no production caller yet,
  consistent with there being no northbound surface in v0.1. **Note for a future round:**
  `crypto.NeedsRehash` also has no caller, so the "transparent upgrade on login" its doc describes is
  not wired.
- **SSE delivery is per-user scoped**, including replay (`internal/httpapi/events.go:117,163`).

**The data layer, executed:**

- **Pragmas are genuinely per-connection across both pools** — all 8 read connections held
  simultaneously plus the writer, every one reporting `journal_mode=wal foreign_keys=1
  busy_timeout=5000 synchronous=1 temp_store=2 wal_autocheckpoint=1000 cache_size=-8000
  page_size=4096`. **This is the classic bug the task asked about and it is not present.**
- **FK enforcement is behavioural on both pools**, not just a pragma readback.
- **`audit_log.actor_user_id` has no `REFERENCES` and the consequence Round 2's DB-02 claimed is
  real** — the actor id survives the user's deletion, and both append-only triggers fire.
- **`BEGIN IMMEDIATE` is taken at BEGIN, before any statement** — a second handle's write while the
  writer tx is open, with no statement issued yet, gets `database is locked`.
- **Single-writer discipline holds under load, with zero `SQLITE_BUSY`:** `writers=16 readers=16
  iters=150 elapsed=329.934611ms ok=2400 busy=0 otherErr=0`; a phase holding a `ReadTx` snapshot open
  while 8 writers hammered and a goroutine ran `PRAGMA wal_checkpoint(TRUNCATE)` in a loop → 0
  errors; two independent `db.Open` handles on one file → `ok=3200 busy=0 other=0`.
- **Down/up round trip is clean, twice through** — 40 objects → 2 → 40, identical list, sentinel row
  intact.
- **The scope predicate fails closed, including for a zero-value `Scope{}` → `"1=0"`.**
- **Timestamps are one format everywhere** — no unix-int / ISO-text mixing, so the lexicographic
  ordering `ix_audit_ts` / `ix_rel_expiry` / `ix_wq_runnable` depend on is sound. Round 2's DOC-02
  holding.
- **Every index in `schema.md` for a shipped table exists live with identical DDL** — 21/21 `SAME`,
  zero missing, zero extra — and **no claimed index goes unused by the planner**, over seeded volume
  (20 k `release_candidate`/`provenance`/`audit_log`/`write_queue`, 5 k `session`, 2 k
  `client_credential`, with `ANALYZE` applied).
- **Toolchain floor holds:** `ncruces/go-sqlite3` ships **SQLite 3.53.4**, above
  `docs/reference/schema.md:12`'s `>= 3.43.0`.

**Documents against code, doc pass:**

- **The whole Prowlarr `/api/v1` surface matches the vendored spec.** `internal/servarr/client.go:157`
  pins `apiPath = "/api/v1"`, and all seven called paths are present in `api/specs/prowlarr.json` with
  the parameters the code sends: `GET /search` (`client.go:462`, params `query, type, indexerIds,
  categories, limit, offset` — **exact match** to `search.go:200-227`, including that `indexerIds`
  and `categories` are repeated, not comma-joined), `POST /search` (grab, `ReleaseResource` body),
  `GET /system/status`, `GET /indexerstatus`, `GET /indexer`, `GET /downloadclient`, `GET /health`.
  `api/specs/SOURCES.md` records SHA-256 `efe3dfb9…5d0fbb` and `sha256sum` matches byte for byte. The
  key travels in `X-Api-Key`, never `?apikey=` (`client.go:251`, `:62-63`).
- **All 14 env keys** documented in `CONFIGURATION.md` §2 exist in `internal/config/config.go:326-384`
  and in `.env.example`, with matching defaults and no undocumented key read; **all 11 flags** in
  `internal/config/flags.go` match their documented env twins, and there is deliberately no
  `--secret-key`.
- **The §3.2 startup ladder matches `secretkey.go:72-133`** in all seven branches.
- **Migration 0001's scope matches ADR-0019 and its own header** — the ten tables, `user_id` on every
  user-scoped row, dangling FKs dropped with a `-- why` comment, append-only audit enforced by
  `trg_audit_no_update`/`trg_audit_no_delete`. (DL-07 is the exception, and it is about four tables
  the *documents* add, not about the ten that shipped.)
- **README status tables: no row overstates.** ADRs 0026/0027/0028/0029 are unimplemented and
  correctly listed as not-yet; `POST /api/v1/system/backup` is absent from the router and §16 says so;
  every make target named in any doc exists except `make seed`, correctly marked **(not yet)**.

**A fresh install, executed end to end:**

- `go build ./...` succeeds in a fresh clone **before any `pnpm` step** (exit 0, 32 s) —
  `internal/web/spa/.gitkeep` keeps `//go:embed` compiling, which is Round 2's B-03 holding.
- **A hand-built binary 404s on `/` with the actionable message** `DEVELOPMENT.md:169` claims.
- `make tools` installs all five pinned tools (`gofumpt v0.11.0`, `golangci-lint 2.12.2`,
  `goose v3.27.3`, `govulncheck v1.7.0`, `gitleaks`); `make build` produces a **statically linked,
  stripped 14.9 MB ELF** with the SPA embedded; `make test` passes (11 Go packages `ok`, vitest 3
  files / 65 tests); `make modverify`, `make secrets`, `make vuln` (*"No vulnerabilities found"*) and
  `make lint-web` (*"211 FILES 0 ERRORS 0 WARNINGS"*) all exit 0.
- **First run:** key auto-generated with the loud warning and correct modes, migration 0001 applies
  (`schema_version:1`), `usarr.db`/`-wal`/`-shm` all 0600, restart is idempotent and the session
  survives; modes are tightened even on a pre-existing 0755 config dir.
- **Health, SPA and auth:** `/api/health/live` → `{"status":"ok"}`; `/api/health/ready` →
  `{"status":"ready","migrations_applied":true,"schema_version":1,"listener_accepting":true,…}`;
  index, hashed assets, favicon and deep links all serve, unknown API paths 404; the first-run setup
  wizard, login and CSRF token issuance all work.
- **End-to-end Prowlarr add against a stub** → 201 `has_credential:true`, then `state:"healthy"`.
- **Honest degradation with nothing configured** — no service → `409 {"error":"no_indexer_service",
  "action":"Add Prowlarr"}`; a service with no indexers → `409 {"error":"no_indexers","action":"Enable
  an indexer in Prowlarr"}`. Principle 3, working.
- **Port and config-dir validation both fail closed** with exact messages.

**One method caveat, and it is worth a permanent line because it silently poisons evidence.**
`golangci-lint` **replayed cached diagnostics across two different clones with identical content,
printing file paths belonging to the other clone.** `golangci-lint cache clean` fixed it and then
reproduced identical counts. Several passes in this round worked in parallel clones of the same
commit, which is exactly the shape that triggers it. **Any lint run used as evidence must clean the
cache first before its paths are trusted** — the counts survive, the paths do not. The 11/7 figures in
FI-04 and FI-11 were taken after a cache clean.

**Not tested by any pass, and recorded so nobody reads the above as broader than it is:** `make
docker` (no Dockerfile in tree), `make bench` / `make bench-rss`, real Prowlarr/Sonarr/Radarr (an
HTTP stub was used, so live indexer search, grab and SSE streaming against real software were never
exercised), key rotation (no such subcommand), a non-root container run (no image), and
`--url-base` / reverse-proxy paths.

---

## 3. Seen and deliberately excluded

These were surfaced by one pass or another and are **owned or already known elsewhere**. They are
named here so the exclusion is visible and nobody concludes the review missed them:

- **gosec G123 at `internal/ssrf/ssrf.go:234`** — the TLS pin skipped on resumed sessions. Already
  Round 2's **SUS-01**. (Note the interaction with SR-02: the pinned path never runs in production
  today anyway.)
- **gosec G124 at `internal/httpapi/auth.go:249,261,275`** — cookie flags. Already Round 2's **W-02**;
  the surviving doc half is recorded as **DS-08**, which is a *documentation* finding, not the flags.
- **`write_queue`'s missing `awaiting_choice` state** — already the **Seam audit**, WQ-01…WQ-05.
  (DL-11 is a different `write_queue` gap: terminal rows, not non-terminal ones.)
- **The Komga → Kavita documentation sweep.**
- **The stale v1.0 rows at `docs/DEVELOPMENT.md:90`, `docs/SETUP-CHECKLIST.md:142` and
  `docs/CONFIGURATION.md:748`.**
- **The Kavita delta-sync probe.**
- **The Chromium-dependent design check.**

The four gosec hits in FI-04's linter output are the first two bullets above, which is why FI-04
counts only the three `noctx` hits as its in-scope remainder.

---

## 4. What routes together

Not a disposition — a routing hint, because several entries are one edit each if they travel in the
right group and three edits each if they do not.

| Group | Entries | Why |
|---|---|---|
| `CONFIGURATION.md` §5/§6 | DS-01, DS-05, DS-13 | Same two sections; DS-01's reproduction is DS-05's evidence |
| Fail-closed on missing key material | DS-01 (code half), FI-09 | Both are about what `cmd/usarr/app.go` does before and during the ladder |
| At-rest credential redaction | SR-01, DS-06 | One boundary move in `SanitizeRelease`, and the fourth deny-list copy |
| The unbounded-growth trio | DL-01, DL-08, DL-10 | One "who sweeps what, and when" decision — a maintenance worker with three tickers |
| The migration-0002 rebuild set | DL-02, DL-04, DL-05, DL-11 | All need a table rebuild or an additive index; deciding them apart means rebuilding twice |
| The gate | FI-02, FI-03, FI-11, FI-04 | In that order: a Makefile pass fixes FI-02 and FI-03, uncapping (FI-11) makes the real number visible, and only then is FI-04 a fixed-size job rather than a moving one |
| First-run ergonomics | FI-05, FI-07, FI-09, DS-02 | Everything a person meets in the first five minutes |
| `NormalizeHostPort` | SR-04, DS-12 | Same function, ten lines apart |
| Roadmap understatement | DS-07, DS-14 | Both are §16 not recording something that shipped |
| CSRF | SR-03, SR-08 | A token bound to the session must be cleared with it |

---

## 5. DS-01, verified independently — the two reproductions and the control

The Critical was raised by the doc/spec pass, then **reproduced separately and without collaboration
by two further passes**, each with its own clone, its own binary and its own ports. Recorded in full
because a data-loss claim carries the round, and because the control experiment is what turns three
matching reproductions into a proof of cause.

**Reproduction A — the fresh-install pass.** One real Prowlarr credential stored, then §6.1's backup
and §6.4's host move, verbatim:

```
$ tar -czf $SP/usarr-db.tgz --exclude='keys' -C $SP run1 && tar -tzf $SP/usarr-db.tgz
run1/ run1/backups/ run1/providers/ run1/usarr.db run1/logs/     # keys/ excluded, as documented
$ cat $SP/run1/keys/secret.key           # "key → a password manager"
qngOUxL2FN8j2yoDVHcNJtUxn3KBuMcDT70+a0axRZk=
$ mkdir -p $SP/newhost/keys; cp $SP/run1/usarr.db $SP/newhost/
$ cp $SP/run1/keys/secret.key $SP/newhost/keys/secret.key; chmod 600 ...
$ ./usarr --config-dir $SP/newhost --port 18484
{"level":"INFO","msg":"master key loaded","source":"key file",...}   # starts happily
$ curl -b cookies http://127.0.0.1:18484/api/v1/services/health
{"any_unhealthy":true,"services":[{"id":1,"name":"prowlarr",...,"state":"down",
 "problem":"the stored API key for \"prowlarr\" cannot be opened for http://127.0.0.1:19696:
  re-enter it, or restore the master key that sealed it","action":"Update API key",...}]}
```

**Its control — `kek.salt` isolated as the sole cause:**

```
$ cat run1/keys/kek.salt   -> Iw54hP9obfmE6KoP3CSRGe76DQlFsytf2CMkbMJ1h1o=
$ cat newhost/keys/kek.salt-> pawNPqgr1pQwlZX1pOAcI/Autz0HLCBA8/nU2gKq4Gs=   # regenerated
$ cp run1/keys/kek.salt newhost/keys/ && restart
{"any_unhealthy":false,"services":[{"id":1,...,"state":"healthy","app_version":"1.30.2.4939"}]}
```

**Reproduction B — the dedicated verification pass**, on its own ports (19811/18811-18813) to avoid
contaminating another reviewer's stub. Install A stored a credential through the full HTTP path
(`GET /auth/session` → `POST /auth/setup` → `POST /api/v1/services`) and proved the *stored* ciphertext
was live by decrypting it on the wire:

```
POST /api/v1/services/1/test
{"ok":true,"reachable":true,"key_proven_valid":true,"app_name":"Prowlarr",
 "message":"connected to Prowlarr 1.37.0.5076 (api v1)"}
# and the stub logged: GET /api/v1/system/status OK, key accepted
```

Then §6.2's `VACUUM INTO`, §6.1's `tar --exclude='keys'` (`tar -tzf | grep keys` → **nothing**), and a
§6.4 restore into a clean directory. **Startup was completely clean — no error, no warning, not one
line about a salt:**

```
INFO "master key loaded" source="key file" path=".../installB/keys/secret.key"
INFO "listening" address="127.0.0.1:18812"
```

Login with the restored owner account **worked** — Argon2id hashes carry their own per-hash salt, so
authentication is unaffected, **which reinforces the false impression that the restore succeeded** —
and `GET /api/v1/services` showed the row intact with `"has_credential":true`. Only §6.3 step 5,
"confirm each instance tests green", reveals it:

```
{"ok":false,"reachable":false,"key_proven_valid":false,
 "message":"the stored API key for \"prowlarr-main\" cannot be opened for http://127.0.0.1:19811:
  re-enter it, or restore the master key that sealed it",
 "action":"Re-enter the API key"}
```

**The master key was byte-identical in both directions — without which the reproduction proves
nothing:**

```
6c8ff7b96bfd0f0cb73183f14a7f92b0b86051f4106e7f990fa118d0b892d194  installA/keys/secret.key
6c8ff7b96bfd0f0cb73183f14a7f92b0b86051f4106e7f990fa118d0b892d194  installB/keys/secret.key
cmp: IDENTICAL (byte for byte)

157b1180...684b  installA/keys/kek.salt     # freshly generated salts differ, as expected
7d43da30...d77c  installB/keys/kek.salt
```

**Its control, run independently of A's:** stopped install B, copied A's `kek.salt` over B's, changed
nothing else, restarted → `{"ok":true,...,"key_proven_valid":true,...}`. Same database, same master
key, same URL. **`kek.salt` alone is the difference between green and permanently unopenable.**

**Verdict: VERIFIED.** Three passes, two full reproductions, two independent control experiments, one
byte-identical master-key hash pair. The single overstatement in the original claim is *"documented
nowhere"*, corrected in §1.1: `docs/reference/security.md:75` does say a per-install salt exists — it
never says what it is called, where it lives, that `--exclude='keys'` discards it, or that it must be
restored.

---

## 6. Verification addendum — what has since been fixed on `main`, re-checked by execution

**Date:** 2026-08-16. **Target:** `origin/main` at `15a7211` — the commit this round merged at, plus
the fixes the code thread has landed since. **Written by the thread that raised Round 6**, for one
reason: several entries above still read **Open — recorded here rather than applied**, the code
thread has since closed them, and a log that says *open* about something that is fixed misleads a
reader of `main` exactly as badly as one that invents status.

**Nothing above is renumbered, reworded or deleted.** Every row below is an **amendment**; the
original entry stands as written, and the amendment says what changed and how it was checked.

**Every disposition here was verified by running something, not by reading a commit message.** The
command or the file:line that settles it is named in each row. Four findings are new and take the
next free id in their own prefix — **SR-13**, **DS-15**, **FI-12**, **FI-13** — so nothing existing
is renumbered and nothing collides.

**`main` moved underneath this section while it was being written, and the section says so rather
than pretending otherwise.** Everything below was first verified on `15a7211`; the merge that carries
it also brings `1285d88` and `087fbdb`, which between them **fix SR-13 and FI-13** — the two
findings this section itself raised. Both are re-verified against the merged tree and
re-dispositioned in place, raised and closed the same day, and every other row was re-run after each
merge. A round that reviews a moving `main` has to record which commit each claim was measured on;
that is why every measurement below names one.

### 6.1 Amended dispositions

| # | Original disposition | Amended disposition, and what was run |
|---|---|---|
| **DL-01** | Open — recorded here rather than applied | **Closed — fixed on `main` in `ee962a8`, *"fix: sweep expired release_candidate rows periodically"*, verified by execution.** The fix took the shape the entry asked for, one step further: not a bare ticker but a named worker. `cmd/usarr/maintenance.go` adds `startCandidateSweeper` on a 5-minute `candidateSweepInterval` (a constant, with the reasoning that nobody has to *decide* it: `releases.CandidateTTL` is 25 minutes, so the interval only bounds how long a dead row lingers past its TTL), sweeping through `Store.ExpireReleaseCandidates` rather than per-instance so one indexed `DELETE` serves N registered instances. The first sweep runs immediately, so a process that was down for an hour does not wait a full interval. **Verified:** `grep -rn "startCandidateSweeper" --include=*.go .` → the production caller is `cmd/usarr/main.go:116`, which is what the original entry said did not exist; `go test ./cmd/usarr/ -run CandidateSweeper` is green (`cmd/usarr/maintenance_test.go`). The stated bound in `reference/schema.md:677-686` — *"the table's size is governed by search volume within a 25-minute window, not by uptime"* — is true of the shipped binary for the first time |
| **DL-03** | Open — recorded here rather than applied | **Closed — fixed on `main` in `e7afbd2`, *"fix: open the read pool mode=ro"*, verified by reading the built DSN.** `internal/db/sqlite.go` now sets `q.Set("mode", "ro")` on the reader branch (`:157`), with the invariant argued in the comment at `:142-158` and restated on the accessor at `:162` — *"It is opened mode=ro, so a write attempted on it fails with `attempt to write a readonly database` rather than silently succeeding and racing the writer."* The single-writer discipline is now enforced by SQLite instead of by a comment, which is precisely the fix shape the entry named. **Verified:** the DSN branch read at `internal/db/sqlite.go:142-159`, and `go test ./internal/db/` green |
| **DS-01** | **CONFIRMED, and Open — fix in flight in the code thread** | **Closed — fixed on `main` in `9eec372`, *"fix: refuse to start when the KEK salt is missing, and move it out of keys/"*, and independently re-verified here.** The full disposition of the fix is the **Data-loss audit** section below (`SALT-01`…`SALT-07`); what this row adds is the *verification* pass over it, run against `15a7211`. Four things were checked and all four hold — see §6.2 for the detail. **One small item is left open and is recorded as DS-15.** |
| **SR-02** | Open — recorded here rather than applied | **Closed — RESOLVED BY REMOVAL on `main` in `864cefc`, *"refactor: mark TOFU SPKI pinning unimplemented, not half-built"*, verified by execution.** Not the fix shape this entry proposed, and the different shape is the point: rather than populate `TestResult.TLSSPKIPin` and make the dead path live, the code thread **deleted the half that pretended to work**. `internal/httpapi/ports.go:81` now reads *"There is deliberately NO `TLSSPKIPin` field"*, and `internal/httpapi/services.go:164-190` carries the reasoning in the code — auto-capture would **downgrade** an instance behind a publicly-trusted certificate (a pin *replaces* chain verification, so hostname, expiry and revocation checking are dropped in exchange for nothing), and there is **no way back**, because `store.ServiceInstanceUpdate` carries no pin field, so an ACME renewal or a container regenerating its self-signed certificate would lock the instance out permanently. **Verified:** `grep -rn "InsecureSkipVerify\|SPKIPin\|TLSSPKIPin" --include=*.go internal/ cmd/` over all three request classes — the only remaining `InsecureSkipVerify` is `internal/ssrf/ssrf.go:234`, reachable **only** through `tlsConfig(class, pin)` with a non-nil pin (`:179`, `:224`), and nothing in the tree writes the column. A **hand-seeded** pin is still honoured and a mismatched certificate still refused (`:278`, `subtle.ConstantTimeCompare`), including on a resumed session — `TestSPKIPinIsEnforcedOnAResumedSession`, the Gate audit's GATE-03. **Recorded so the removal is not read as a silent retreat:** the code thread is raising an ADR in `DECISIONS.md` for the decision — `grep -n "SPKI\|TOFU" docs/DECISIONS.md` returns **no match** on `15a7211`, so the ADR is not in the tree yet and that is the outstanding half. The two **reopening prerequisites** belong in it: **(1)** a pin field on the service *update* path, so a recorded pin can be cleared and re-accepted, and **(2)** the change-acceptance UI described in `providers.md` §4 / `CONFIGURATION.md` §7.1 — show the fingerprint, have the user accept it. Enrolment is the easy half; neither prerequisite is optional |
| **SR-01** | Open — recorded here rather than applied | **Closed in two commits, both verified by execution. `fdd40fb`, *"fix: stop storing private-tracker passkeys in SQLite"*, closed the QUERY-PARAMETER case; the PATH-SEGMENT case it did not reach is raised here as SR-13 and closed by `bb74081` (§6.3).** The fix landed at the boundary the entry named — `internal/servarr/redact.go:32,57`, in `SanitizeRelease`, not at the HTTP boundary — and it **went further than what this review reported**, which is worth recording plainly: this round found only `info_url` and `raw_release_json`; the code thread found `commentUrl` and `posterUrl` inside the blob as well, and `provenance.nzb_info_url` on the grab path (`internal/releases/grab.go`). **Verified on `15a7211`:** the passkey is gone from `release_candidate.info_url`, from all of `infoUrl`/`commentUrl`/`posterUrl` inside `raw_release_json`, and from `provenance.nzb_info_url` — confirmed down to a byte-grep of the **closed** `.db` and `-wal`, which is the check that matters, since the original reproduction found the plaintext key 0 times in the `.db` and **2 times in the `-wal`**. Tripwires re-run here and green: `go test ./internal/releases/ -run TestPersistedCandidateNeverCarriesATrackerPasskey` and `go test ./internal/servarr/ -run TestSanitizeReleaseRedactsTheIndexerSuppliedURLs` |
| **FI-02** | Open — recorded here rather than applied | **Still open — and now diagnosed precisely rather than described.** The cause is one missing prerequisite: `fmt-check` (`Makefile:362`) invokes `prettier` through the `web` workspace but declares **no `web-deps` prerequisite**, while `lint-web` (`:351`) and `test-web` (`:274`) both declare it. `check-offline` (`:567`) runs `fmt-check` **first**, so a fresh clone dies at the very first target — and **any run after a single `make lint` hides it**, because `lint` → `lint-web` → `web-deps` has already populated `node_modules`. That is why the failure survives: it is invisible to anyone who has ever linted. **Verified by reproducing the identical failure on a fresh clone of `origin/main` at `15a7211`** (the original observation was on `56101c1`; the target has moved, the failure has not) — see the transcript in §6.4. **Fix shape unchanged and now exact:** add `web-deps` as a prerequisite of `fmt` and `fmt-check`. One line. Owned by the code thread |
| **FI-03**, **FI-04** | Open — recorded here rather than applied | **Closed — GREEN on `main`, and this is the first `make check` result in the project that is trustworthy.** `make check` exits **0** on `15a7211` with the **pinned** `golangci-lint v2.12.2`, reporting **0 issues both capped and uncapped**, and `govulncheck` clean. FI-03's two failure modes are both gone: `lint-go` (`Makefile:341-348`) resolves the binary through `$(GOLANGCI_LINT)` and asserts it against the pin via `require_tool`, so a bare-`PATH` v2.5.0 can no longer answer for it. FI-04's 11 issues (4 gosec + 7 `noctx`) are fixed — GATE-03/04/05 in the Gate audit below. **Why this row is worth its space:** every earlier green in this project was measured with the wrong linter version, so "the gate is green" has never been evidence until now. Full transcript with the version banner in §6.4. **The gate did not stay green through the merge, and for a reason that has nothing to do with FI-03 or FI-04 — see FI-13.** |
| **FI-11** | Open — fix in flight on another branch, and NOT on `main` as of this entry | **Closed on `main` — cross-referenced, not re-dispositioned.** The **Gate audit** section below already closes it as **GATE-02** (*"Applied, and the hole is closed"*), and the fix is in the tree: `.golangci.yml:37-39` now carries `issues: max-same-issues: 0 / max-issues-per-linter: 0`. The `grep -n "issues\|max-same\|max-issues" .golangci.yml` that returned **no match** when FI-11 was written now returns those three lines. FI-11 and GATE-02 are the same defect found twice, from two directions, and GATE-02 owns the disposition |

**Counts, restated for this round only.** Of the 49, **seven** are now closed on `main` — DL-01,
DL-03, DS-01, SR-01, SR-02, FI-03, FI-04 — and **one more**, FI-11, is closed under another section's
id. **FI-02 is the one High from this round still open**, and it is a one-line `Makefile` change.
**Four new findings** are added here: SR-13 (High, raised and closed the same day), DS-15 (Low),
FI-12 (Low) and FI-13 (Low, likewise raised and closed the same day). **DS-15 and FI-12 are the two
new items still open**, and both are one-line changes. No existing entry's id, text or severity
changed.

### 6.2 DS-01 — the four things checked on the shipped fix

Recorded in full because DS-01 was the round's Critical, because the fix moves key-derivation
material on an upgrade path, and because "it is fixed" is the one claim a data-loss finding must not
be taken on report.

1. **The relocation uses `link(2)`, not `rename(2)`.** `internal/config/secretkey.go:375`,
   `os.Link(tmpName, path)`, with the reasoning at `:364-368`: *"rename(2) replaces the destination
   unconditionally"* — so a racing process that had already written a good salt would have it
   silently replaced, and any credential sealed under the replaced value becomes unopenable. `link`
   is equally atomic and **fails** rather than clobbering. This is the difference between a fix and a
   second data-loss bug wearing the first one's clothes.
2. **It converges under a real race.** Run with **64 concurrent in-process callers** and **24
   separate processes**, under `-race -count=8`: all runs converged on a **single** salt, and no
   temp files were left behind. Re-run for this addendum on `15a7211`:
   `CGO_ENABLED=1 go test -race -count=8 ./internal/config/ -run 'KEKSalt|Salt'` → `ok`, and
   `CGO_ENABLED=1 go test -race ./cmd/usarr/ -run 'KEKSalt|Salt|KeyLadder'` → `ok`.
3. **The salt lands in `CONFIG_DIR`, beside `usarr.db` — not `DATA_DIR`, and deliberately.**
   `internal/config/config.go:221`, `filepath.Join(c.ConfigDir, "kek.salt")`. §5 documents `DATA_DIR`
   as *"REGENERABLE. Safe to delete"*, so putting an unrecoverable input there would have rebuilt the
   same trap under a different label, and it would fall outside the `--exclude='keys' /config`
   archive. SALT-03 below records the same reasoning from the fixing side; it is repeated here
   because it is the part of the fix most likely to be "tidied" by someone reading only the
   instruction that said *the data directory*.
4. **The legacy `keys/kek.salt` is COPIED, not moved — deliberately**
   (`internal/config/config.go:227`, `LegacyKEKSaltPath`; the copy-forward inside `ResolveKEKSalt`, `secretkey.go:280`).
   A move has an instant in which **neither** path holds the salt, and nothing stops two UsArr
   processes starting against one config directory. A crash in that window loses key-derivation
   material permanently, which is strictly worse than the bug being fixed.
5. **A fail-closed guard now refuses to start** when encrypted credentials exist and the salt is
   missing (`secretkey.go:242` `ErrKEKSaltAbsent` → `:446` `ErrMissingSaltForExistingData`, resolved at `cmd/usarr/app.go:131-135`), matching what the master key on
   the same startup path already did. That asymmetry *was* DS-01's code half.

**Left open, and it is a documentation item, not a defect — DS-15.**

| # | Finding | Disposition |
|---|---|---|
| **DS-15** | **Two code comments in `internal/config/secretkey.go` still describe the mechanism the fix moved away from.** `:274` says the relocation lands *"via `writeSaltFile`'s temp-plus-rename"* and `:316`/`:321` describe `writeSaltFile` as *"temp file … then `rename(2)`"*. The relocation path uses `os.Link` (`:375`), and `:364-368` explains at length **why** it must not be a rename | **Open — recorded here rather than applied. Owned by the code thread.** Cosmetic in effect and **not** cosmetic in kind: these comments describe the exact mechanism the race-safety argument rests on, so the next person to read them is told the code does the one thing `:364-368` says it must never do. **Fix shape:** two comment edits, no code change |

### 6.3 SR-13 — a credential in a URL PATH SEGMENT was redacted nowhere

The half of SR-01 that `fdd40fb` did not reach, raised as its own finding because it has its own
severity and its own reason for having stayed open. **It was raised open and closed inside the same
day** — the entry keeps both states, because the reasoning for *not* fixing it is what the fix had to
answer, and deleting the open state would hide the argument the heuristic is calibrated against.

| # | Finding | Disposition |
|---|---|---|
| **SR-13** (High) | **`ssrf.RedactRawURL` matched a deny-list of query-parameter NAMES only, so a passkey carried in a URL *path segment* — `https://tracker.example/rss/<passkey>/torrents`, a completely ordinary private-tracker RSS shape — was redacted nowhere in UsArr.** As raised, on `15a7211`: `internal/ssrf/redact.go`'s `credentialParams` was a map of parameter names, `isCredentialParam` looked names up in it, and `RedactURL` rewrote `clone.RawQuery` and dropped `clone.User` — **the path was never touched**. `RedactRawURL` is `RedactURL` over a string, and because `SanitizeRelease` is built on it the gap propagated to every sink SR-01 named: `release_candidate.info_url`, all three URL fields inside `raw_release_json`, `provenance.nzb_info_url`, and the raw database bytes. **Severity was not uniform across those sinks, and the distinction is the finding:** a `release_candidate` row **expires**, and since DL-01's sweeper landed it is genuinely swept, so that exposure is bounded by the TTL. `provenance` rows are **immutable** and have **no delete path anywhere in the tree** — `grep -rn "DELETE FROM provenance\|DeleteProvenance" --include=*.go .` returns nothing — so a path-segment passkey written there is **UNRECOVERABLE, not merely stored**, and it travels into every `VACUUM INTO` backup and every support bundle | **Closed — fixed on `main` in `bb74081`, *"fix: redact private-tracker passkeys held in URL path segments"*, verified by execution.** **The open disposition this entry was first written with is kept above the line, because the fix had to answer it.** That argument was: there is no trustworthy threshold for telling a secret path segment from a legitimate one — `/rss/a1b2c3…/torrents` and `/details/tt0111161/cast` are the same shape to a matcher — and a false positive **silently corrupts `provenance`**, the one table with no way to undo it, so guessing is worse than not guessing. **What the fix does instead of guessing: it calibrates to MISS.** `redactPathSegments` (`internal/ssrf/redact.go:151`) splits each path segment on `.` and redacts a part only when it is ≥20 characters, an unbroken `[A-Za-z0-9]` run with no separator, holds **both** a letter and a digit, and does not read like words (`looksLikeCredential:185`, `looksLikeWords:219`). **The numbers are sourced, not invented** — Prowlarr's own log scrubber, `CleanseLogMessage.cs` on `develop`, whose unanchored path rules use `[a-z0-9]{16,}`, raised to 20 here because upstream's rules are anchored to a host or path prefix and this one runs on every segment of every URL; the `.` split is what catches UNIT3D's `/torrent/download/<id>.<rsskey>`. It is deliberately **not** applied to `stripCredentials`, because removing a path segment from a redirect target changes which resource is requested. **Verified on the merged tree**, by throwaway test (removed; `git status --porcelain` clean): `/rss/<22-char alnum>/torrents` → `/rss/REDACTED/torrents`; `/torrent/download/12345.<22-char alnum>` → `…/12345.REDACTED`; and the control `/details/tt0111161/cast` → **unchanged**. `go test ./internal/ssrf/` green including the new `redactpath_test.go`, and SR-01's tripwires still green. **Residual, recorded rather than claimed closed:** the calibration means a passkey that is shorter than 20 characters, all-digits, all-letters, or hyphenated is still carried verbatim — all four confirmed by execution in §6.3's second transcript. That is the deliberate direction of the miss, not an oversight, and the durable close is still the one the open disposition named: an explicit per-indexer declaration of which path positions are secret, which Prowlarr's indexer definitions already know. `FUTURE.md`, against the `SanitizeRelease` seam |

**Reproduced on `15a7211`**, before the fix, in a throwaway test in `internal/ssrf` (removed
afterwards; `git status --porcelain` clean):

```
$ go test ./internal/ssrf -run TestZZPathSegmentCredentialSurvives -v
in : https://tracker.example/rss/NOT-A-REAL-PASSKEY/torrents
out: https://tracker.example/rss/NOT-A-REAL-PASSKEY/torrents      <- unchanged
in : https://tracker.example/details/1?passkey=NOT-A-REAL-PASSKEY
out: https://tracker.example/details/1?passkey=REDACTED            <- fixed by fdd40fb
PASS
```

The two lines together were the whole finding: the same secret, the same function, redacted in one
position and verbatim in the other.

**Re-run on the merged tree, after `bb74081`** — same harness, real-shaped fixtures, and a control:

```
in : https://tracker.example/rss/a1b2c3d4e5f6g7h8i9j0k1/torrents
out: https://tracker.example/rss/REDACTED/torrents                       <- now redacted
in : https://tracker.example/torrent/download/12345.a1b2c3d4e5f6g7h8i9j0k1
out: https://tracker.example/torrent/download/12345.REDACTED             <- UNIT3D shape, split on '.'
in : https://tracker.example/details/tt0111161/cast
out: https://tracker.example/details/tt0111161/cast                      <- control, untouched
```

**And the residual, measured rather than assumed** — four shapes the calibration deliberately lets
through, every one returned verbatim:

```
/rss/a1b2c3d4e5f6g7h8/torrents          -> unchanged   (16 chars, under the 20 floor)
/rss/1234567890123456789012/torrents    -> unchanged   (22, digits only — no letter)
/rss/abcdefghijklmnopqrstuv/torrents    -> unchanged   (22, letters only — no digit)
/rss/a1b2-c3d4-e5f6-g7h8-i9j0/torrents  -> unchanged   (separators break the run)
```

A heuristic that misses is the correct trade here — a false positive corrupts `provenance`
irreversibly and a false negative leaves it exactly where it already was — but *which* shapes it
misses has to be written down, or the next reader takes the fix for complete coverage.

### 6.4 The gate, and the fresh clone — transcripts

**FI-03/FI-04: `make check` on `15a7211`, cache cleaned, pinned linter. The first trustworthy green.**

```
$ export PATH="/root/go/bin:$PATH"
$ golangci-lint cache clean
$ golangci-lint --version
golangci-lint has version 2.12.2 built with go1.25.13 …
$ golangci-lint run                                              →  0 issues.  EXIT=0
$ golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 →  0 issues.  EXIT=0
$ make check
…
check-offline: OK
vuln: scanning 11 Go packages against vuln.go.dev
No vulnerabilities found.
check: OK
EXIT=0
```

Capped and uncapped agree at **0**, which is what makes the number a count rather than a floor —
FI-11/GATE-02's whole point.

**FI-02: a fresh clone of `15a7211`, nothing else run first.**

```
$ git clone … && cd freshclone && make check-offline
tool: /root/go/bin/gofumpt — version v0.11.0, asserted against the pin
fmt-check: checking 110 .go files with gofumpt
> prettier --check .
[error] Cannot find package 'prettier-plugin-svelte' imported from …/web/noop.js
 WARN   Local package.json exists, but node_modules missing, did you mean to install?
make: *** [Makefile:365: fmt-check] Error 1
EXIT=2
```

**FI-12, new, and the reason it is worth a line: the obvious way to run the tests gives a misleading
red.**

| # | Finding | Disposition |
|---|---|---|
| **FI-12** (Low) | **A bare `go test -race ./...` in a fresh clone FAILS** — not because anything is broken, but because `internal/web/spa` is populated by `make test-go`'s `web-build` prerequisite (`Makefile:258`) and by nothing else. `git ls-files internal/web/spa` tracks exactly one file, `.gitkeep`, so a clone has no SPA embedded and `cmd/usarr`'s end-to-end route test has no document to find | **Open — recorded here rather than applied. Not a regression, and not a defect in the gate**, which is green: `make test-go` builds the SPA first and `internal/web`'s own tests skip honestly with *"no frontend build embedded — run `make web-build`"*, escalating to a hard failure under `USARR_REQUIRE_WEB_BUILD=1` so the `//go:embed all:` regression test can never silently skip inside `make check`. The gap is that `cmd/usarr`'s e2e test has **no such skip**, so the most obvious command a newcomer types produces a red that means nothing. **Fix shape:** give the `cmd/usarr` e2e route assertions the same `Built()`-guarded skip `internal/web` already has, or say so in `DEVELOPMENT.md` §3 next to `make test`. Reproduced on a fresh clone of `15a7211`: |

```
$ CGO_ENABLED=1 go test -race ./...
--- FAIL: TestUnauthenticatedAndURLBase (0.06s)
    e2e_test.go:319: /usarr/search = 404, want the SPA document
    e2e_test.go:321: every route honours USARR_URL_BASE=/usarr
FAIL	github.com/jdb3750/UsArr/cmd/usarr	2.609s
EXIT=1
# every other package: ok
```

**FI-13, found while re-running the gate over the merge that carries this section, and closed on
`main` before that merge landed — recorded in full anyway, because it is the third demonstration in
one round that a gate result is only as good as the commit it was measured on.**

| # | Finding | Disposition |
|---|---|---|
| **FI-13** (Low) | **`make check` went RED on `main` again, on a formatting nit in a file this thread must not touch.** `internal/ssrf/redactpath_test.go:190` is not gofumpt-formatted — a comment-alignment difference of two spaces on the `// ULID` trailing comment, introduced by `c2e2c57` *"fix: make the path-passkey fixtures structurally fake"*, which shortened the literal above it and left the aligned comment behind. `fmt-check` fails first (`Makefile:364`), and `lint-go` fails again on the same line through the `gofumpt` formatter (`1 issues: * gofumpt: 1`). **Confirmed pre-existing and not merge-induced:** `gofumpt -l internal/ssrf/` run in a detached worktree at `origin/main` itself lists the file | **Closed — fixed on `main` in `087fbdb`, *"style: restore gofumpt comment alignment in redactpath_test.go"*, while this section was being committed.** Recorded rather than deleted, because the entry's whole value is the sequence. It was **deliberately NOT fixed by this thread**, which is scoped to `docs/REVIEW-LOG.md` alone — a review log that quietly reformats another thread's source is exactly the behaviour the round-6 preamble rules out — and while it was open, the gate was measured target by target so the scope of the red was exact: `modverify` OK, `secrets` OK (*no leaks found*), `test` OK (all Go packages, 65 web tests), `vuln` OK (*No known vulnerabilities found*), with only `fmt-check` and `lint-go` red on that one line. **Re-verified after `087fbdb`: `make check` exits 0**, cache cleaned, banner `golangci-lint has version 2.12.2 built with go1.25.13`, `check: OK`. **The point worth carrying, and it survives the fix:** §6.4's green was real on `15a7211`, was stale by `1285d88`, and is real again at `087fbdb`. FI-03 proved a green can be measured with the wrong *tool*; FI-11 proved it can be measured with the wrong *caps*; FI-13 is the plainest version of the same lesson — it can simply be measured on the wrong *commit*, and on a repository where several threads push to `main` in the same hour that is the common case, not the exotic one. **A gate result without a commit sha attached is not a result** |

### 6.5 A miss by this review, recorded as a method lesson rather than a code finding

Not a defect in the code, and not a finding anyone raised. It is recorded because the round's own
process produced it and the next round will produce it again unless it is written down.

**Two passes each held half of SR-13's severity, and neither of them held the finding.** The data
prong flagged `provenance` as **immutable, permanent, and with no delete path** (DL-02's second
half). The security prong flagged `info_url` as **carrying tracker credentials** (SR-01). Both are
correct, both are in this log, and they are in **different sections written by different passes**.
The real severity is neither one: it is the **composition** — a credential that cannot be removed
once written, in the one table with no removal path. SR-01 was filed as a High about a table that
*expires*; nobody asked what happens when the same value reaches the table that does not.

**Why it happened, stated so it is actionable and not just regretted.** The five prongs were run
**in parallel and without sight of each other**, which is exactly what makes their agreements
worth something — three independent reproductions of DS-01 are strong evidence *because* the passes
could not have coordinated. The cost of that same independence is that no pass could see a
composition, and the synthesis step that merged their outputs deduplicated *identical* findings
(recorded above, at "Found independently by more than one pass") without ever asking whether two
**different** findings intersected.

**The lesson, in one rule:** after deduplication, run a deliberate **composition pass** — for every
finding about a value that must not leak, check it against every finding about a store that cannot
forget. Deduplication asks *"did two passes find the same thing?"*; it never asks *"do two different
things multiply?"*, and the multiplication is where SR-13 was hiding.

**That SR-13 was fixed within hours of being named does not soften the lesson, it sharpens it.** The
fix was cheap once the finding existed — one function, sourced numbers, a day's work. The expensive
part was the four weeks it could have sat unnamed while `provenance` accumulated rows nothing can
delete. What the review nearly missed was not a hard problem; it was an easy problem that no single
pass was standing in the right place to see.

---

# Gate audit — the pre-commit gate was running an unpinned linter

**Date:** 2026-08-16. **Branch:** `main`. **Baseline:** `44ad58f`. Not a review round, and no
adversarial reviewer behind it. Two threads disagreed about whether `make check` passed on `main` —
one reported `check: OK`, the other reported `lint-go` red — and this entry settles which was right
and disposes of the findings that disagreement exposed. Prefix `GATE-` has not been used before, so
nothing collides.

Both runs were honest and both were reproducible. They disagreed because they were not running the
same program.

| # | Finding | Disposition |
|---|---|---|
| **GATE-01** | **`make check` reported `OK` while running a linter three minor versions behind the pin.** `Makefile` pins `GOLANGCI_VERSION ?= v2.12.2` and `make tools` installs it into `$GOBIN` (`/root/go/bin`), but `lint-go` invokes the bare name `golangci-lint`, and `$GOBIN` **is not on `PATH`** — so the gate ran `/usr/local/bin/golangci-lint`, **v2.5.0**, on every invocation. Measured on `44ad58f` with a cleared cache: v2.5.0 reports **0 issues**; the pinned v2.12.2 reports **11**. `gosec`'s G123 and G124 do not exist in the older build, so the green result was not a disagreement about severity — the checks were never executed | **The red run was right; the green run was measured with the wrong tool, and the record is corrected rather than quietly fixed.** Pinning a version is not the same as running it, and a security gate that silently degrades to a weaker gate is worse than no gate, because it produces a green nobody re-examines. `reference/security.md` §7 now records the hole under Supply chain, with the 0-vs-11 numbers and the two acceptable fixes: invoke the linter by its pinned absolute path, or assert `--version` against the pin before running. **The `Makefile` fix is NOT in this commit** — another thread held that file at the time — so the hole is documented and still open. It is the one item here that is not closed |
| **GATE-02** | **The red run's "7 issues" was itself an undercount: the real number is 11.** golangci-lint defaults to `max-same-issues: 3`, and it truncates **silently** — no "N more occurrences" line, and the summary count reflects what survived truncation, not what was found. `noctx` had **7** findings and displayed **3**. Fixing the three on screen would have looked like finishing the job while four remained in `internal/web/web_test.go` and `internal/httpapi/services_urlbase_test.go` | **Applied, and the hole is closed.** `.golangci.yml` gains an `issues:` block setting `max-same-issues: 0` and `max-issues-per-linter: 0` (`0` = unlimited, verified against the baseline: the same tree reports 11 uncapped and 7 capped), with a comment recording why and instructing that the defaults not be restored to shorten the output. Also worth naming for whoever audits this next: **a stale golangci-lint cache produced a different count for the same tree** during this investigation, so every number quoted in this entry was taken after `golangci-lint cache clean` |
| **GATE-03** | **G123 — the SPKI pin was genuinely bypassable on a resumed TLS session. Real, and the only security defect of the four.** `internal/ssrf/ssrf.go` installed the pin via `VerifyPeerCertificate` alongside `InsecureSkipVerify`, the latter deliberately, since the pin replaces chain verification for a TOFU self-signed \*Arr certificate. But `crypto/tls` does not re-verify certificates on resumption: it skips `verifyServerCertificate`, and with it `VerifyPeerCertificate`, calling **only** `VerifyConnection`. Verified against Go 1.25.13 source, not from memory — `handshake_client_tls13.go:619-630` (`if hs.usingPSK`) and `handshake_client.go:616-624`, both citing golang/go#31641; the full-handshake path calls both hooks at `handshake_client.go:1217-1229`. With the pin as the *only* server authentication on a pinned instance, a resumed handshake was accepted with **no check at all** | **Applied.** `tlsConfig` now sets `VerifyConnection` as well, and both hooks share one `checkPin` helper so the two paths cannot drift. Resumption is **enforced rather than disabled**, keeping the pinned path as fast as the unpinned one; `c.peerCertificates` is restored from the session on resumption (`handshake_client_tls13.go:468`, `handshake_client.go:985`) so the check has a certificate to compare. `TestSPKIPinIsEnforcedOnAResumedSession` guards it and was confirmed to **fail without the fix** (`a resumed session bypassed the SPKI pin: got <nil>`). The test asserts the rejection came from the *resumed* path — it wraps `VerifyPeerCertificate` and requires it was never called — so it cannot pass by quietly falling back to a full handshake. `reference/security.md` §2.2 gains item 8. **Exposure was latent, not live:** the transport sets no `ClientSessionCache`, and `loadSession` returns early without one (`handshake_client.go:390`), so the shipped client never offered a ticket to resume from. Anything that later enables resumption — including a future shared `tls.Config` — would have silently switched the pin off |
| **GATE-04** | **G124 on the three `http.Cookie` literals in `internal/httpapi/auth.go` — not real, suppressed with the reasoning inline.** Checked individually rather than as a group | **Suppressed, per-site, with `//nolint:gosec` and a stated reason a reviewer can argue with.** All three set `SameSite=Lax`; both session cookies set `HttpOnly: true`. The CSRF cookie sets `HttpOnly: false` **deliberately** — double-submit requires the SPA to read it and echo it in the header, so an HttpOnly CSRF cookie would stop the scheme working rather than harden it, and the token authenticates nothing on its own. The remaining trigger on all three is that `Secure` is a call to `secureCookies(r)` rather than a constant, which gosec cannot see through; that conditionality is the existing decision recorded above `secureCookies` (plain HTTP on a trusted LAN is v0.1's documented simplest deployment, and a browser discards a `Secure` cookie sent over http, so an unconditional flag makes login impossible there). **No blanket exclusion was added** — a future cookie that genuinely lacks `HttpOnly` or `SameSite` must still fail the gate |
| **GATE-05** | **`noctx` — 7 findings, all in test files, all mechanical.** `httptest.NewRequest` in `redact_test.go` (×2), `server_test.go`, `services_urlbase_test.go` and `web_test.go` (×3) | **Applied.** All 7 now use `httptest.NewRequestWithContext(t.Context(), …)`. The four beyond the originally-reported three are the ones GATE-02 was hiding. The new test added for GATE-03 was held to the same rule: it uses `(*net.Dialer).DialContext` and `(*tls.Conn).HandshakeContext` rather than the timeout-only forms |

**Result:** `make check` passes with the **pinned** v2.12.2 and an uncapped, cache-cleaned run —
`0 issues`. One item, GATE-01's `Makefile` change, is documented and deliberately left open.

---

# Data-loss audit — a restore without `kek.salt` silently destroyed every credential

**Date:** 2026-08-16. **Branch:** `main`. **Baseline:** `8c4a33c`. Raised by the adversarial-review
thread and confirmed by two further independent reproductions. Reproduced again here, end to end,
through the real startup path and the real HTTP API before anything was changed. Prefix `SALT-` has
not been used before, so nothing collides.

**The reproduction, verbatim, on `8c4a33c`.** A fresh install, one Prowlarr credential sealed through
`POST /api/v1/services` (mandatory connection test and all), then `keys/kek.salt` deleted with
`keys/secret.key` left in place — the exact state §6.1's backup procedure produced on restore:

```
run 1: keys/kek.salt = Iy5Pwto7zupA5xbWiPRuRCWSkOWM6pNUDSXMzF06mos=
restore simulated: keys/secret.key present, keys/kek.salt MISSING
REPRO: run 2 STARTED SUCCESSFULLY — no refusal
run 2: keys/kek.salt = qHGds0xBy+ulFNMkv+x0gLEQwtOfV8AjMCAuEATZypY=
REPRO: a DIFFERENT salt was silently generated
POST /api/v1/services/1/test -> 200
  {"ok":false,...,"message":"the stored API key for \"Prowlarr\" cannot be opened for
   http://127.0.0.1:36545: re-enter it, or restore the master key that sealed it",
   "action":"Re-enter the API key"}
```

**Scope, stated precisely.** Not total data loss, and not fully silent. The database, library, users,
password hashes, sessions and the audit log all survive; only `service_instance.api_key_enc` becomes
unopenable. The Services screen does show the service down. That is exactly the severity §3.5 already
documents as "re-enter your API keys" — the defect is that it was **triggered by following the
documented backup procedure**, reached through an error message that named the wrong artifact, over a
file §3.5 never mentioned. Severity comes from being unrecoverable and self-inflicted, not from
breadth.

| # | Finding | Disposition |
|---|---|---|
| **SALT-01** | **Startup silently regenerated a missing KEK salt over existing ciphertext.** `KEK = HKDF-SHA256(secret.key, salt=kek.salt, info="usarr/kek/v1")`, so a fresh salt is a fresh KEK and every sealed envelope becomes noise. `config.ResolveKEKSalt` created the file on `os.ErrNotExist` with no check for sealed data, and `buildApp` called it unconditionally | **Applied — fail closed, following the rule the master key already follows.** `internal/config`'s ladder refuses to start when no key is supplied and the database holds encrypted rows, naming the expected path (`ErrKeyAbsent` → `ErrMissingKeyForExistingData`). The salt is the same class of input with the same consequence, so it now behaves the same way: `ResolveKEKSalt` **never writes** and returns the new `ErrKEKSaltAbsent`; creation moves to an explicit `GenerateKEKSalt`; `buildApp` resolves the sentinel exactly as it resolves `ErrKeyAbsent` — count sealed rows first, generate only on a genuine first run, otherwise fail with `ErrMissingSaltForExistingData`. Both branches share one lazy row count. **Only one site in the tree creates or loads a salt**, so there is no second entry point to bypass |
| **SALT-02** | **The error the operator actually saw named the wrong artifact.** `openCredential` said *"re-enter it, or restore the master key that sealed it"* — surfaced on the connection test and on search. For the overwhelmingly likely cause it was actively misleading: it sent someone who had just restored a backup, and who **had** the master key, off to look for the one file they were already holding. Nothing anywhere named `kek.salt` | **Applied.** The message now names both real causes and the real recoverable unit: *"…Either the base URL was edited (the credential is bound to its scheme, host and port), or the keys/ directory does not match the one that sealed it — restore ALL of keys/ (secret.key AND kek.salt), not just the master key. Otherwise re-enter the API key."* In `cmd/usarr/services.go`; `internal/httpapi/auth.go` was held by another thread and is untouched. The startup refusal separately names `kek.salt` by full path, names the legacy path too, and states that the library, users and audit log are unaffected |
| **SALT-03** | **`kek.salt` was in the one directory the documented backup deliberately excludes.** §6.1's `tar --exclude='keys' /config` plus `cat /config/keys/secret.key` archived the ciphertext, excluded the key directory, and preserved exactly one of the two inputs needed to open it. §5's authoritative tree omitted `kek.salt` entirely while declaring itself exhaustive, so nothing on the page told a careful reader the second file existed | **Applied — the file moved, on the owner's call, rather than the advice being patched.** The salt is **not secret**: its value does not depend on confidentiality, only on being per-install and stable. `keys/` should mean "actual secrets, store them elsewhere", and a non-secret in there is what turned correct advice into a trap. `KEKSaltPath()` is now `$USARR_CONFIG_DIR/kek.salt`, beside `usarr.db`, so an ordinary backup captures it and `--exclude='keys'` is correct exactly as written — the catastrophic restore becomes **unreachable** rather than merely documented. The fail-closed guard from SALT-01 stays: belt and braces for anyone who loses it another way. **Placement note:** the instruction said "the data directory", but `usarr.db` lives in `CONFIG_DIR` and §5 documents `DATA_DIR` as "REGENERABLE. Safe to delete" — putting an unrecoverable input there would have recreated the same bug with a different label, and it would fall outside the `--exclude='keys' /config` archive. "Beside `usarr.db`" is the requirement that carries the reasoning, so `CONFIG_DIR` it is |
| **SALT-04** | **The upgrade path is where a fix like this kills data.** Installs created before the move — the owner's, created the same day — keep the salt at `keys/kek.salt`. Reading only the new path would report it missing on every one of them | **Applied as a COPY that never deletes, not a move.** Nothing stops two UsArr processes starting against one config directory: there is no lock, both bind, both write. A move has an instant in which neither path holds the salt, and a crash there loses key-derivation material **permanently** — strictly worse than the bug being fixed, which leaves the data unopenable-until-restored. So: prefer the new path; fall back to the legacy one; if only the legacy exists, copy it forward and **leave the original in place forever**. The duplicate is one 45-byte non-secret, and it means a backup that captured `keys/` still holds a working salt — the old advice degrades to redundant rather than wrong. The "why it is a copy" argument is in the code comment so nobody tidies it into a rename |
| **SALT-05** | **The guard must not false-positive on a partial write, and `writeSecretFile` writes in place.** A reader catching a half-written salt sees a short value, validation rejects it, and startup refuses — telling an operator their credentials are at risk when they are not | **Applied.** Every salt write now goes through `writeSaltFile`: temp file in the **destination** directory, `chmod 0600`, `fsync`, `rename(2)`, then a best-effort directory `fsync`. `rename` is atomic, so a concurrent reader sees either the old file or the complete new one, never a partial. The temp file must share the destination's directory because `CONFIG_DIR` and `DATA_DIR` may be different filesystems and a cross-device rename fails with `EXDEV`. `GenerateKEKSalt` additionally re-resolves before generating and **re-reads after writing**, so two genuine first runs racing converge on one salt instead of diverging — a credential sealed under a salt another process then replaced would be unopenable forever. `TestResolveKEKSaltIsConcurrencySafe` (16 goroutines) and `TestGenerateKEKSaltConvergesUnderRace` (8) cover both, under `-race`. **Left open:** `writeSecretFile` still writes `secret.key` in place under `O_EXCL`. Pre-existing, and a torn read there fails validation loudly rather than silently, but it deserves the same treatment |
| **SALT-06** | **A doc fix is not the protection.** The archived procedure was wrong for as long as it existed; the next wrong procedure will be someone's blog post | **Applied.** `TestRestoreWithoutKEKSaltRefusesToStart` (`cmd/usarr/keyladder_test.go`) seals a credential through the real API, deletes the salt, restarts through `buildApp`, and fails if startup succeeds. Confirmed to **fail without the fix**: *"startup SUCCEEDED with sealed credentials present and keys/kek.salt missing."* It also asserts the refusal names the salt path, that **no** salt was written despite the refusal, and that restoring it afterwards opens the original credential byte-for-byte — proving the refusal preserved recoverable data rather than merely failing. `TestLegacyKEKSaltIsRelocatedNotLost` drives the full upgrade path and asserts the legacy file survives; `TestFirstRunGeneratesTheKEKSalt`, `TestRestartKeepsTheSameKEKSalt`, `TestBothSaltCopiesPresentPrefersTheNewPath` and `TestKEKSaltLivesOutsideKeysDir` pin the rest, the last of which fails if anyone ever moves the salt back under `keys/` |
| **SALT-07** | **Is any other file under `keys/` exposed to the same hazard?** Audited the directory rather than fixing the reported file alone | **Two others, both dispositioned. `secret.key` — correctly placed and already guarded:** it IS a secret, so `keys/` is right for it, and `GenerateMasterKey` is reached only after the row count says nothing is sealed. **`secret.key.new` — not reachable, left open deliberately:** §3.4 specifies that its presence at startup means an interrupted rotation that must resume, but `Config.NewSecretKeyPath` has **no caller** — rotation is unimplemented, so there is no code to be wrong yet. Flagged for whoever implements §3.4: a rotation that half-completes has this bug's exact shape, and its resume path must fail closed the same way |

**Docs changed together:** `CONFIGURATION.md` §3.2 (three ladder rows, incl. the copy-forward),
§3.5 (retitled "If you lose the key — or the salt"), §5 (the tree, and the "keys/ holds secrets
only" corollary), §6.1 (table, command, upgrade warning), §6.3, §6.4; `SETUP-CHECKLIST.md` §2.5;
`.env.example`. `internal/config/config.go`'s comment claimed the salt belonged beside the key
because they share a fate — true about the fate, wrong about the location — and now explains both.

**Left to another thread:** `docs/reference/security.md` was held throughout and is **not** touched.
Nothing in it is false — §1.3's `salt=<per-install random, stored>` is correct and it defers on-disk
layout to `CONFIGURATION.md` §5 — but it never says where the salt lives or that it is required for
recovery, and §1.3 should now name `$USARR_CONFIG_DIR/kek.salt` and state that it is a non-secret
which the database backup must contain. That is the only item outstanding.

---

# Scope audit — v0.1's catalogue sources were re-sequenced, and the docs disagreed with themselves

**Date:** 2026-08-16. **Branch:** `main`. Not a review round, and no adversarial reviewer behind it.
The trigger was the owner, handed the question of whether v0.1 should grow from two providers to
five, handing it back: *"I'm fine with starting small, it's honestly sort of whatever to me."*
Recorded here because the outcome is a scope change that two Accepted ADRs argued against, and
because the second half of the finding is drift — the repository had begun contradicting itself
about which sources v0.1 ships, which is exactly the failure mode `CLAUDE.md`'s "no invented status"
rule exists to catch.

| # | Finding | Disposition |
|---|---|---|
| **SCOPE-01** | **ADR-0032 and ADR-0035 each answered "which catalogue sources", and neither answered "can v0.1 carry three of them".** ADR-0032 moved four out of the v1.0 bucket and put three into v0.1; ADR-0035 swapped which three. §16.0 priced them honestly — four hand-written Tier 0 adapters, four auth schemes, one with a lifecycle, four hierarchies, and one entirely new sync channel — and then did not treat that price as a constraint on the milestone paying it. Meanwhile what is actually **built** is the Prowlarr path; the \*Arr *library* sync, the tables it writes, the grid, the image pipeline and local search are all still design | **Applied as [ADR-0036](./DECISIONS.md#adr-0036). v0.1 ships Sonarr, Radarr and Prowlarr; no catalogue source ships in v0.1.** The \*Arr library sync lands first, because it is the thing that proves the replica thesis on real data. **This is re-sequencing, not rejection** — Navidrome, Audiobookshelf, Kavita and Komga all still arrive, one at a time after v0.1, each behind a milestone with its own success criterion (*this source appears in the grid, is searchable, delta-syncs, and its Services row is honest about what it cannot do*). ADR-0032's roadmap conclusion is preserved intact; only its v0.1 membership moves. Neither prior ADR is reversed, and both carry a 🚩 note on their Status line pointing here — the same convention ADR-0035 used on ADR-0032 |
| **SCOPE-02** | **The order within the sequence was the temptation, because fixing it would let `SETUP-CHECKLIST.md` name one next service** | **Left to the probe, deliberately.** ADR-0035 §2's `LastChapterAdded` watermark spike still decides it — **Kavita first if it passes, Navidrome first if it fails** — with its three falsifiable clauses unchanged. What moved is only *when it runs*: it was day-one because it decided build order *inside* v0.1, and with no catalogue source there it gates nothing on day one. It now runs immediately before the first catalogue adapter is written. **Deferring the run must not turn it back into a guess**, which is why the pass condition stays written down in advance in ADR-0035 §2, §7.1a and §16 alike. **Komga is last regardless** — nobody on this project can point it at a real library. **Navidrome must precede v0.4**, whose success criterion needs a populated music replica |
| **SCOPE-03** | **v0.1's "unified library across six media types" became false and had to be replaced with something narrower rather than quietly softened** | **Applied, and split into the three claims it was conflating.** The **schema** is six-type — it must be, migration 0001 can never be edited, and the enumeration in §16's v0.1 entry is unchanged. **Requesting** is six-type — Prowlarr Search-and-Grab covers all six categories, and that part is already shipped. **The catalogue is film and TV.** §16's v0.1 entry now carries that as a blockquote, the README's first v0.1 row says it, and `CLAUDE.md`'s one-line v0.1 summary says it. A media type with no configured source degrades honestly (principle 3) rather than rendering an empty grid — which makes the honest-degradation path the *common* path in v0.1 and gets it exercised from the first release |
| **SCOPE-04** | **§6.4 justified a v0.1 rule with a source v0.1 no longer shipped.** *"It is v0.1 because v0.1 ships Komga, which supplies no external identifiers at all"* — written under ADR-0032, left standing after ADR-0035 replaced Komga with Kavita, and false twice over after ADR-0036 | **Applied, and the rule survives on better ground.** The nullable column and the `matched by title` badge are v0.1's **because they cannot be retrofitted**, not because v0.1 renders the state often — v0.1's Sonarr and Radarr carry TVDB and TMDB ids, so the state first *renders* with the first catalogue source, where per ADR-0035 §1 it is the **ordinary** case rather than an edge case. The honest comparison is kept: Komga supplies no external identifiers at all, so comics has no strong-identity path under either choice. §16's v0.3 entry, which repeated *"because v0.1 ships Komga"*, is corrected the same way |
| **SCOPE-05** | **§7.1a's Kavita row still said the thing ADR-0035 had already disproved.** *"⚠️ None. `sortByLastModified` does not exist on the Series, Volume or Chapter endpoints"* — true about query parameters, wrong as a conclusion, and directly contradicted by ADR-0035's verification from Kavita `main` | **Applied.** Kavita's row is now the probed one, stating the three-clause pass condition, recording that `SortField.LastModifiedDate` exists but `SeriesDto` returns no last-modified property (so that key yields no carryable watermark), and noting that its result orders the post-v0.1 sequence. Komga's row becomes *"(later)"* with its own probe at the milestone it lands in. The section intro's *"no sortable delta at all"* is corrected to *"no query-parameter delta; the sort lives in `FilterV2Dto.SortOptions` on `POST /api/Series/all-v2`"*, which is what ADR-0035 §2 actually probes. The table is reordered into expected landing order and the trailing paragraph — which claimed channel 3b for v0.1 — now says it is **specified now, built with the first catalogue adapter**. §7.1's channel table row 3b is corrected to match |
| **SCOPE-06** | **Three rows were stale in the *opposite* direction, filing catalogue sources under v1.0 — the pre-ADR-0032 world.** A sweep that only looked for "v0.1 + Komga" would miss them entirely | **Applied, with a different rewrite each.** `DEVELOPMENT.md`'s tree lumped `jellyfin/ navidrome/ audiobookshelf/ komga/ kavita/` under one *"v1.0 southbound adapters"* comment; the four catalogue adapters are split out as one milestone each after v0.1 and Jellyfin stays v1.0, which it correctly is. `SETUP-CHECKLIST.md`'s prerequisite table filed Audiobookshelf and Komga/Kavita under v1.0 on one shared row; they are now three rows in sequence order, Kavita's *"⚠️ auth scheme unconfirmed"* note **cleared** against `RESEARCH.md` Track 06 (`x-api-key`, with the key in a URL path segment on OPDS routes) and replaced with the Kavita+ identifier caveat that actually matters, and Komga marked *"only if you adopt it"*. Its §3 also said Navidrome-as-a-library-source was **v1.0**, which contradicted everything else on the page; that is now "one of the first two catalogue milestones, and before v0.4 either way". ⚠️ **The relayed line numbers were off** — the rows are `DEVELOPMENT.md:94`, `SETUP-CHECKLIST.md:143-144` and, for the third, **not `CONFIGURATION.md:748` at all**: that file's only Komga mention is line 798, an SSRF example about tailnet-resolved cover art, which is a technical fact and correct as written |
| **SCOPE-07** | **`reference/gateway.md` contradicted §16 on the method count and mis-pointed two cross-references.** Line 12 said the gateway implements *"thirteen methods"* while §16 v0.4 — authoritative — says ~20; line 51 cited *"multi-instance aggregation (§2)"* where §2 is Authentication, and *"OPDS (§5)"* where §5 is capability negotiation | **Applied, without pretending the table is something it is not.** §1's lead now states the ~20-method subset and carries a ⚠️ saying the table below lists the thirteen it was first scoped around, naming the six §16 added plus the error responder, and stating plainly that **§16 wins** on any disagreement — the rows remain correct about tables and degradation for the methods they cover. The two cross-references are re-pointed at what actually discusses them: OPDS's auth scheme is §2's own `### OPDS` subsection, and multi-instance aggregation is §3, whose ID format ships from day one because it cannot change later |
| **SCOPE-08** | **A claim introduced during the fix was wrong and was caught before it landed.** A first draft of §16.0's auth bullet asserted Kavita authenticates with *"a JWT from `/api/Account/login` with a refresh"* — invented, not verified, and contradicted by §11.2 four hundred lines away | **Rewritten against primary-source-backed repository evidence rather than left to stand.** `RESEARCH.md` line 1187 records the Kavita auth question **cleared 2026-08-16 (Track 06)** from the full `openapi.json`: it is `x-api-key`, with the key in a URL path segment on OPDS routes. Both §16.0 and §11.2 now say that, and they now agree. Logged rather than silently fixed because it is the exact failure the "verify, don't assert" rule names, committed while fixing an instance of the same rule being broken |

**Also corrected while in the same paragraphs, because leaving them would have been fresh
self-contradiction:** §16.0's payment argument (the old *"what has to move out is Kavita, to v0.2"*
is now the honest full payment, with **both** earlier undersized payments named); §16.0's shared-read-
machinery bullet, which is now the argument *for* the sequence — shared machinery has to exist and
have run before it can be shared; §16.0's libraries paragraph, which justified the subsystem with
the Ebooks/Audiobooks split over one Audiobookshelf library, a demonstration that **moves with
Audiobookshelf** — the subsystem stays in v0.1 on its real grounds (four tables that belong in
migration 0001, one of the five essential screens, and the request destination the write path routes
on) and is given demonstrations v0.1 can actually perform; §11.2's note, which priced *"three
hand-written Go adapters in v0.1"*; and §16's v0.2 entry, which had pinned Kavita to it as *"the
fourth catalogue source"*.

**A new §16.1 holds the sequence** as a four-row table with the gate on each, so there is one place
that answers "what is next" rather than the answer being spread across an ADR, a channel table and a
setup checklist. ADR-0035's Decision block also pointed at *"the day-one watermark spike in §3
below"* when the spike is its §2 (§3 is the ADR-0030 re-examination); corrected in passing, with the
old pointer noted so the fix is not mistaken for a renumbering.

**Done in parallel by the design thread, and merged rather than duplicated.** The same call was taken
independently in the same hours, which is why it lands in two places: **SW-10** above records it from
the design side, **ADR-0035 carries an amendment** restating its own four findings against the moved
milestone, and **§17 was re-scoped** for a v0.1 with no catalogue sources (`67b0868`) — including
Home Block A's four sourceless rows, `matched by title` marked unreachable in v0.1, and the removal
of *"Navidrome catalogues music in v0.1 exactly as Radarr catalogues films"*. The two accounts agree
and neither restates the other: **ADR-0036 carries the decision, its alternatives and the §16
rewrite; ADR-0035's amendment carries what the move does to ADR-0035 clause by clause.** That split
is what the design thread's own commit asked for — *"ADR-0032's status note and the §16 rows are the
implementation thread's"* — and ADR-0032's status note now points at ADR-0036, because a note has to
point at something and burying a scope decision inside a source decision is what caused this drift
in the first place.

**Left to other threads, not touched:** everything under `docs/design/` beyond what the design thread
has already landed, and `docs/reference/schema.md` (held by the migration-0002 thread) — whose
line 183 *"later tables (v1.0, with Lidarr / Komga)"* and lines 559 and 1188, both scoping
`service_item_link`'s container to *"Navidrome, Audiobookshelf and Komga — three of the six media
types"*, all still read from the pre-ADR-0035 world and need the same re-sequencing plus the
Komga→Kavita swap. **Everything else the `Komga` sweep found is correct as written**: ecosystem and
API facts in `RESEARCH.md`, `reference/tags.md`, `reference/security.md` and `CONFIGURATION.md` §9;
SSRF test fixtures and the `service_kind` comment in migration 0001; and §17's remaining mentions,
which are UI copy and degradation examples stating no milestone.

---

# Round 6 — second addendum: the documentation-fidelity batch

**Written by the documentation thread**, which was handed a batch of "the docs claim something that
isn't true" items and required to verify each against the code before writing the correction. Scope
was docs, the `Makefile`, `.env.example`, `api/specs/` and comments in `internal/config`. Observed on
`main` at `ac1ab29`, 2026-08-16 17:30–18:10 UTC, after a `git fetch`.

**Nothing above is renumbered, reworded or deleted.** The rows here are amendments and new findings;
new ids take the next free number in their own prefix — **FI-14**, **FI-15**.

## 7.1 Amended dispositions

| # | Was | Now |
|---|---|---|
| **DS-15** | Open — recorded rather than applied. Owned by the code thread | **Closed — applied.** Both comments in `internal/config/secretkey.go` now say **temp-plus-`link(2)`** and state *why* the distinction is load-bearing: `rename(2)` replaces the destination unconditionally, `link(2)` refuses when it exists, and "never clobber" is the property that makes two racing first runs converge on one salt instead of each sealing credentials under a KEK the file on disk can no longer derive. The trailing *"so the rename itself survives a power cut"* on the directory `fsync` was stale in the same way and is corrected with them. Comment-only; no behaviour changed |
| **FI-12** | Open — recorded rather than applied. Owned by the code thread | **Closed as documented, not as coded** — which is one of the two fix shapes the original row offered. `DEVELOPMENT.md` §3 now carries *"Run `make test`, not a bare `go test ./...`"*, naming the cause (`internal/web/spa` tracks only `.gitkeep`; `make test-go`'s `web-build` prerequisite is the only thing that populates it), the two commands side by side, and the `Built()`-guarded skip `internal/web` already has. **The other fix shape stays open and is the better one:** give `cmd/usarr`'s e2e route assertions that same skip, and the misleading red stops existing rather than being explained |
| **FI-02** | Open — one-line `Makefile` change | **STILL OPEN, and explicitly NOT verified fixed.** Re-checked directly: `fmt-check` (`Makefile:362-363`) still declares **no `web-deps` prerequisite**, while `lint-web` and `test-web` both do, and `check-offline` still runs `fmt-check` first. **The reason this row exists is the verification, not the status:** the green that has been cited for it was produced on a tree where `make web-deps` had already run, so it never entered the failing path at all — `lint` → `lint-web` → `web-deps` populates `node_modules`, and from that moment the bug is invisible to everyone who has ever linted. **It gets marked applied only after someone runs `make check` on a genuinely fresh clone with no prior `make` invocation of any kind**, and quotes the transcript. Any other evidence is evidence about a different tree |

## 7.2 New findings

| # | Finding | Disposition |
|---|---|---|
| **FI-14** (Low) | **`make check` makes TWO network calls and was documented as making one, in three places simultaneously** — `Makefile`'s honesty notice, `DEVELOPMENT.md` §4 and §8, and `CLAUDE.md`. The `vuln` target runs `govulncheck` against `vuln.go.dev` **and** `pnpm audit` against the npm registry; it has done both since `pnpm audit` was added, and the target's own help text read *"THE ONE NETWORK STEP"* while listing two commands one line below | **Applied in the Makefile and `DEVELOPMENT.md`**, corrected to *two network calls, both to vulnerability databases*, with `check-offline` documented as dropping both. **`CLAUDE.md` is NOT amended** — the documentation thread does not edit `CLAUDE.md` on another agent's instruction; the one-line correction is handed to Joe for his own sign-off. **Why it survived:** nobody counted, everybody copied. The sentence was true when written and became false one commit later, and three copies of a claim are three chances to notice and, in practice, none |
| **FI-15** (Medium) | **An invented API vocabulary in a reference doc reached the UI. One error, two files.** `docs/reference/tags.md:54` listed the `flag:` namespace as `freeleech \| internal \| scene \| proper \| repack \| nuked`. **`proper`, `repack` and `nuked` are not Prowlarr indexer flags and never have been** — grepping a `develop` checkout at `1f7db1e` for `IndexerFlag` alongside those three returns nothing. Four real values were missing: `exclusive`, `neutralleech`, `halfleech`, `doubleupload`. **The downstream consequence is the finding, not a second one:** the same three invented names had already propagated into the design mockups, which rendered a `repack` chip in the indexer-flags position — a value that field cannot produce. The design thread is removing it and rendering only the real set | **Applied in `tags.md`**; the mockup fix is the design thread's and is recorded here as the *consequence* rather than a separate row, because splitting them loses the point. **The cost of an invented value in a reference doc is never the wrong line — it is the UI built on it, found later and further away, by someone who reasonably treated the doc as authoritative.** `tags.md` now states the source (`src/NzbDrone.Core/Indexers/IndexerFlag.cs`), the re-check command, and the torrents-only caveat below |

**A correction to the report that raised FI-15, and it is the same lesson one level up.** The finding
arrived with the claim that the vocabulary is **closed** to the seven statics in `IndexerFlag.cs`,
evidenced by `new IndexerFlag(` appearing nowhere in `src/`. **The set is not closed, and that grep
cannot show it is.** `IndexerFlag.cs` constructs with C# target-typed `new(...)`, so the pattern
`new IndexerFlag(` matches **zero lines in a file containing seven of them** — the probe returns
empty for a repository where the thing is everywhere, and empty reads as confirmation. Grepping
`static IndexerFlag` instead returns **nine** hits: the seven common statics, plus
`PassThePopcornFlag : IndexerFlag` contributing `golden` and `approved` into the same array. So UsArr
must match the seven it knows and **pass an unrecognised flag through as an opaque tag**, which is
the opposite of what a closed set would license. Verified on a `--filter=blob:none --depth 1` clone
of `Prowlarr/Prowlarr` at `1f7db1e`, 2026-08-16.

**Second half applied 2026-08-16:** `ARCHITECTURE.md` §8.5 derived `flag:` from `indexerFlags` with
no statement either way, which left the closed-set reading standing wherever a reader met §8 before
`tags.md`. It now carries the open-vocabulary clause, the pass-through rule, and the same
`static IndexerFlag` / **not** `new IndexerFlag(` re-check — naming the failed probe in the doc,
because a probe that returns empty for a repository where the thing is everywhere will be re-run by
the next person otherwise. `tags.md` needed nothing further and was left untouched.

**`indexerFlags` is torrents-only**, checked in the same clone:
`ReleaseResourceMapper.ToResource` does `model as TorrentInfo ?? new TorrentInfo()`
(`src/Prowlarr.Api.V1/Search/ReleaseResource.cs:68-70`), so a usenet release takes the fallback and
its flag array is **always empty**. Empty on usenet means *the field does not apply*, never *we
checked and none are set*.

## 7.3 The pattern these four share

`FI-15` is the **fourth** documented property today that nobody had measured, and the set is worth
reading together because each was green, plausible and wrong in a different layer:

| | What was claimed | What was true |
|---|---|---|
| **FI-03** | the gate lints with the pinned tool | it resolved `golangci-lint` from `PATH` and ran an old unpinned version, for the life of the project |
| **FI-11** | the gate reports its findings | stock `max-same-issues: 3` truncated 11 to 7 silently; `gosec`'s four sat exactly at the boundary |
| **FI-14** | `make check` makes one network call | it makes two |
| **FI-15** | `tags.md` lists Prowlarr's indexer flags | three were invented, four were missing — and the invented ones were already rendering in the mockups |

**FI-15 is the one that shows the failure crossing a file boundary**, and that is why it is the
expensive one. The first three are wrong statements about the repo that a reader can check against
the repo. The fourth became a chip in a design mockup, where nothing connects it back to the API
field it claims to mirror, and where the next person to touch it would reasonably assume the value
was verified upstream. **A reference doc is an input to other work, so an unverified fact in one does
not stay a documentation defect — it becomes someone else's correct implementation of a wrong
premise.**

The rules these produced are in `docs/DEVELOPMENT.md` §11 — probe the condition rather than a proxy
for it, report what you measured rather than the verdict, exercise the failure path, and declare what
a check should *find* rather than only what it forbids. **Two of the three incidents behind those
rules were introduced by the fixes for the other two**, which is the part worth remembering: a fix is
written under the assumption that the failure mode is now understood, and that is exactly when people
stop checking for it.

---

# Round 6 — third addendum: FI-02 closed by reproduction, not by reading

**FI-02 is the finding that had been asserted fixed and un-asserted more than once**, always for the
same reason: every green cited for it came from a tree where `make web-deps` had already run, so the
failing path was never entered. §7.1 set the bar for closing it — *"run `make check` on a genuinely
fresh clone with no prior `make` invocation of any kind, and quote the transcript"*. That is what
this section is. Observed on `main` at `c628bd1`, 2026-08-16.

## 8.1 The reproduction

`git clone` of the repo into a scratch directory outside the working tree, `web/node_modules` absent,
no `make` target run first. The pinned tools were already installed under `$(go env GOPATH)/bin` —
`make tools` is a prerequisite a first-time clone does need, and it is a separate matter from this
finding.

```console
$ git clone . /tmp/…/fresh-clone && cd /tmp/…/fresh-clone
$ make check
tool: /root/go/bin/gofumpt — version v0.11.0, asserted against the pin
fmt-check: checking 116 .go files with gofumpt

> usarr-web@0.0.0 format:check /tmp/…/fresh-clone/web
> prettier --check .

Checking formatting...
[error] Cannot find package 'prettier-plugin-svelte' imported from /tmp/…/fresh-clone/web/noop.js
 ELIFECYCLE  Command failed with exit code 1.
 WARN   Local package.json exists, but node_modules missing, did you mean to install?
make: *** [Makefile:397: fmt-check] Error 1
```

**The finding was right, including the diagnosis.** `fmt-check` is the first target `check-offline`
runs and the only one that reached `pnpm` before anything had installed. The failure is not in
gofumpt — the Go half passes and prints its count — it is prettier's plugin resolver, one line into
the web half.

## 8.2 The fix, and the second clone that proves it

`web-deps` added as a prerequisite of `fmt` and `fmt-check`, exactly the fix shape this row has
carried since it was raised, and exactly what `lint-web` and `test-web` already declare. `fmt` is
included because it fails identically and is the first thing a contributor runs *after* a red
`fmt-check`; fixing only the gate would leave the recovery broken.

Proved from a **second** fresh clone — the first is dirty now, having installed `node_modules` on its
way to failing:

```console
$ git clone /home/user/UsArr /tmp/…/fresh-clone-2   # + the Makefile fix, uncommitted at the time
$ make check
…
check-offline: OK
tool: /root/go/bin/govulncheck — version v1.7.0, asserted against the pin
vuln: scanning 11 Go packages against vuln.go.dev
No vulnerabilities found.
vuln: auditing the pnpm dependency tree against the npm registry
No known vulnerabilities found
check: OK
```

| # | Was | Now |
|---|---|---|
| **FI-02** | STILL OPEN, and explicitly NOT verified fixed | **Closed — applied and verified by reproduction.** Reproduced by `make check` on a fresh `git clone` with no prior `make` invocation (transcript in §8.1), fixed by adding `web-deps` to `fmt` and `fmt-check` in the `Makefile`, and re-proved green from a second fresh clone (§8.2). Commands and date are both above. `DEVELOPMENT.md` §3 now leads with the ordered first-clone sequence, because the other half of this finding was that the recovery appeared in neither §3 nor §4 |

## 8.3 What else a first clone hits, checked in the same tree

Two things, and neither is a defect — but both are steps, and steps that are not written down are
indistinguishable from a broken repository:

- **`make tools` is genuinely required**, not optional. Every pinned binary is invoked by absolute
  path under `$GOBIN`, and `require_tool` fails closed with `run: make tools`. That guard is good;
  what was missing is that `DEVELOPMENT.md` §3's quickstart annotated the line `(not yet)`, which
  reads as *this target does not work yet* rather than *run this first*.
- **`FI-12`'s claim is still accurate**, re-checked rather than assumed. On the never-built clone,
  `CGO_ENABLED=1 go test -race ./...` fails in exactly one package with exactly one test:
  `cmd/usarr` — `e2e_test.go:337: /usarr/search = 404, want the SPA document`. Every other package,
  `internal/web` included, is `ok`. So the wording in §3 is right and so is the disposition: the
  misleading red is a missing build step, and `make test` does not have it because `test-go` depends
  on `web-build`.

**The order matters more than the list.** `make tools`, then `make check` — and `make check` needs no
`make web-deps` in front of it any more, which was the whole point of the finding.

---

# Round 6 — fourth addendum: FI-12 closed as coded, and the skip given teeth

**FI-12 was closed once as documented, and §7.1 said in writing that the other fix shape was the
better one** — *"give `cmd/usarr`'s e2e route assertions that same skip, and the misleading red stops
existing rather than being explained"*. That is what this section is. Observed on `main` at `e5f07fa`,
2026-08-16.

## 9.1 The reproduction, and the exact scope of the red

`git clone` of the repo into a scratch directory outside the working tree, **no `make` target run
first**, so `internal/web/spa` holds nothing but `.gitkeep`:

```
$ CGO_ENABLED=1 go test -race ./...
--- FAIL: TestUnauthenticatedAndURLBase (0.11s)
    e2e_test.go:337: /usarr/search = 404, want the SPA document
    e2e_test.go:339: every route honours USARR_URL_BASE=/usarr
FAIL	github.com/jdb3750/UsArr/cmd/usarr	3.453s
… every other package ok, internal/web included
```

**Exactly one test, exactly one assertion** — which settles a discrepancy in how the finding had been
reported. The two names in circulation, *"`TestUnauthenticatedAndURLBase` in `cmd/usarr`"* and
*"`e2e_test.go`'s SPA assertion"*, are the **same** failure, not two: `TestUnauthenticatedAndURLBase`
lives in `cmd/usarr/e2e_test.go` and its last assertion is the SPA one. `grep -rn 'raw(t, "GET",
"/[^a]' cmd/usarr/*_test.go | grep -v /api` returns that one line and nothing else, so no second
assertion was hiding.

## 9.2 The fix, and why it is a subtest rather than a whole-test skip

`requireSPABuilt` in `cmd/usarr/e2e_test.go` is `internal/web`'s `requireBuilt` transcribed:
`web.AssertBuilt()` → skip with the fix command, escalating to `t.Fatalf` under
`USARR_REQUIRE_WEB_BUILD=1`. The SPA assertion moved into a `t.Run("SPADeepRoute", …)` subtest so the
skip is **narrow**: `TestUnauthenticatedAndURLBase` proves two properties, authentication on every
`/api/v1` route and `USARR_URL_BASE` applying to all of them, and **neither needs an SPA**. Skipping
the whole test to dodge one assertion would have traded a misleading red for a real coverage hole in
every fresh clone — which is the same failure shape the finding was raised about, pointed the other
way.

## 9.3 Proof the skip cannot become permanent

The risk this fix introduces is the one that matters: a skip nobody notices has disarmed. Three runs,
same never-built clone:

| Command | Result |
|---|---|
| `CGO_ENABLED=1 go test -race ./...` | all packages `ok`; `--- SKIP: TestUnauthenticatedAndURLBase/SPADeepRoute` |
| `USARR_REQUIRE_WEB_BUILD=1 CGO_ENABLED=1 go test -race -run TestUnauthenticatedAndURLBase ./cmd/usarr/` | `--- FAIL: TestUnauthenticatedAndURLBase/SPADeepRoute` |
| `make check` (SPA present via `test-go`'s `web-build`) | the subtest **runs and passes**; it does not skip |

The second row is the assertion asked for, and it is deliberately **not** a separate meta-test: the
guard already fails closed under the one environment variable `make test-go` sets, so a test whose
only job is to assert that another test ran would restate the guard's own condition. `make check`'s
`web-build` prerequisite plus `USARR_REQUIRE_WEB_BUILD=1` is the mechanism; a third copy of it would
be one more thing to keep in step, not one more check.

**FI-12: closed as coded.** `DEVELOPMENT.md` §3 is rewritten from *"this fails and here is why"* to
*"this skips and here is the command"*.

---

# The frontend bench, and a grab that lied — SW-22 to SW-24

**Date:** 2026-08-16. **Branch:** `claude/hearth-thread-vn9w7u`. Continuing the `SW-` sequence, which
now runs past the consistency sweep it started in. None of these three was raised by an adversarial
reviewer, and none was measured or observed by the thread that wrote them down; each is attributed to
the thread that produced it.

**SW-22 and SW-23 came from a sixth place: the frontend thread's measurement run — `pnpm bench:list`
— reported to this thread rather than measured here.** That distinction is the reason they are
recorded under their own heading. **This thread did not run the bench and does not claim the
numbers**; what it did was decide where each one lands in the design documents, check the two that
could be checked against the mockups, and carry the caveats the frontend thread attached. No
adversarial reviewer raised either. SW-22 is the interesting one: it is a *new* finding rather than a
consistency defect, and the frontend thread reports it as **undocumented anywhere they could find,
upstream included** — worth knowing, because a later reader who goes looking for corroboration will
find none, and absence of corroboration is not evidence against it here.

| # | Finding | Disposition |
|---|---|---|
| **SW-22** | **`contain-intrinsic-size: auto` remembers a size measured at the *previous* density, and a keyed `{#each}` reuses row nodes — so after a compact→relaxed switch the remembered size is one density stale and the scrollbar is 14.57% wrong, against 0.65% when the same rows are rebuilt rather than reused.** Measured by the frontend thread's bench at 5,000 rows. This is a live correctness bug rather than a curiosity, because both preconditions are ordinary in this design: density is a first-class control on every screen (`design/DESIGN-DIRECTION.md` §5.3) and the lists are keyed by row id (§9.1a clause 4 requires the key). **Reported as new — they could not find it documented anywhere, including upstream** | **Written into `design/DESIGN-DIRECTION.md` §7.4 as a required rule, not a caution:** when row height changes the remembered intrinsic size **must** be invalidated, by rebuilding the affected nodes (`id + density` as the key, or `{#key density}` around the list body) or by forcing re-measurement of every mounted row before the next paint. Same rule into [ADR-0029](./DECISIONS.md#adr-0029), whose containment prescription it modifies. ⚠️ **Recorded as not enforced, with the reason, rather than as covered:** the condition needs node *reuse* plus thousands of rows, and `docs/design/check.mjs`'s target is `prototype.html` — static HTML with neither — so an assertion written there could not reproduce the condition and **would pass for ever**, which is the failure shape this branch caught three times in a day. Enforcement is queued in the frontend thread's `pnpm bench:list`, **fail above 2% drift** (the drift budget `contain-intrinsic-size` already carries, not a second number), and the sequencing is written down as **fix the 25,000-row Chromium OOM that makes a full `bench:list` run exit non-zero → add the assertion → only then call it enforced**. Also recorded: if the app target later mounts large lists, moving the assertion into `check.mjs` is small and the frontend thread would not object, so the split reads as *where the condition can exist* rather than as a territorial line |
| **SW-23** | **D-21 and P-04 have been open since the first review round on the same point — `contain-intrinsic-size` had no measured value, which left `ARCHITECTURE.md` §4.5 and §7.4 a direction rather than an implementable rule. It now has one.** The frontend thread's bench measured **28 / 32 / 36 px** for a one-line row at compact / standard / relaxed (drift **0.76 / 0.70 / 0.65%** against the 2% budget) and **45 / 49 / 53 px** for rich rows. The same run confirmed the two things the policy rests on: containment is live on the grid-row primitive — 5,000 rows gave a **761,316 px** difference in scroll height with a deliberately wrong placeholder, against **exactly 0** when the same rows were forced to `display: table-row`, the known `<tr>` limitation reproduced as a control — and the density toggle's cost is mostly buyable, **~88% from containment and ~25% from scoping the density attribute, 911 ms down to 80 ms at 5,000 rows** | **Applied. The "no value yet" caveat is removed from every file this thread owns and replaced by the measurement**: `design/DESIGN-DIRECTION.md` §7.4 (both the caveat and the "until that measurement exists" sentence), [ADR-0029](./DECISIONS.md#adr-0029), and a pointer from §5.3, whose nine numbers are floors rather than the placeholder values. ✅ **The mockups already agreed, and that was checked rather than assumed** — re-measured at 1440×900, `prototype.html` renders exactly 28 / 32 / 36 and 45 / 49 / 53, with `contain-intrinsic-size` computing to `auto 27.98 / 31.98 / 35.98px` and `auto 45.08 / 49.08 / 53.08px`, within 0.3% of the border-box height each stands in for (the property sizes the *content* box, so the 1 px row border is the difference). **No mockup change and no `build_prototype.py` regeneration were needed.** ⚠️ **The frontend thread's caveat is carried verbatim in substance: the 25,000-row point is superlinear, so the linear per-row fit is only good to a few thousand rows** (0.15 ms/row at 1,000, 0.24 at 5,000, 0.26 at 25,000). Both places where this thread's text extrapolates — §7.4's and ADR-0029's Pi 5 DOM-row ceiling — now say so, and both extrapolate *downward* into the range the fit covers; nothing extrapolated past a few thousand rows is quoted as measured. Note the caveat's limit in the other direction too: the one-line values are **not** the whole placeholder story, because §7.4's `--row-lines` expression is per row *shape*, and the two measured shapes are two of several |

**SW-24 came from a seventh place, and it is the most valuable kind there is: production.** Not a
review finding, not a consistency finding, not a bench result — the owner ran a real grab against his
real stack and the screen told him something false. It is recorded as a **production observation**
rather than as a design inference because that is what it is, and because an inference would not have
found it: the failure only exists when a real download client is on the far side of a real Prowlarr.
Reported to this thread by the deployment thread.

| # | Finding | Disposition |
|---|---|---|
| **SW-24** | **A grab that succeeded end to end was reported as a failure.** The owner's book downloaded in Deluge while UsArr showed **"Grab failed — HTTP 502"**: Prowlarr added the torrent to the download client, then failed on a *post-add labelling step*, and returned 500 for the whole operation. **An upstream failure response can therefore cover an operation that has already partly succeeded**, and the harm is specific — a false *"failed"* invites the user to grab the same release again, and a grab is irreversible from UsArr's side | **Applied to `ARCHITECTURE.md` §17.5.** The grab-result surface has **three** states, not two — succeeded · failed · **the upstream reported failure and the download may already be running** — and **the third state's copy may never assert that a failed grab did not happen**. Which state a result lands in is decided **by how far the request got rather than by whether an error came back**, and that principle held while the first mapping written against it did not. 🚩 **Verified by the code thread: right in principle, wrong in the specific mapping** — which is the useful form of this record, rather than "we were right" or "we were wrong". The claim was that the existing error codes already coincide with the boundary, with `grab_failed` as the sent-unknown bucket. They do not: **`grab_failed` is the unclassified remainder and carries all three outcomes at once** — six definitely-not-sent cases (bad API key, open circuit breaker, SSRF refusal, Prowlarr 400, Prowlarr 409, corrupt blob); one **definitely sent *and confirmed*** (`ErrDecode`, reachable only after a 2xx, where Prowlarr confirmed the grab and UsArr then failed to parse its own response — **a second, independent false-failure bug**, recorded as its own thing rather than folded into Joe's); and only `ErrServer`, `ErrTimeout` and `Canceled` genuinely ambiguous. So the assignment is **backend classification off the error sentinel, not a relabel of one existing code**, implemented additively by the code thread: `grab_failed` narrows and **a new code carries sent-unknown**. ⚠️ **The near miss is recorded as a caution about scope, not about this mapping:** had the wholesale relabel shipped it would have been **the same double-grab invitation pointed the other way** — sending a user to check their download client for a request that never left UsArr's process. The rule was right; applying it to the wrong set would have produced a fresh instance of the harm the rule exists to prevent. **Unaffected by the correction, and stated as such: the no-Retry-under-ambiguity rule and the honest wording stand exactly as written** — they now attach to the new sent-unknown code rather than to `grab_failed`. Retry means *do it again*, and doing it again is what produces two copies of a 68 GB release, so the honest action points the user at their download client, where the truth actually lives, and the row says plainly that UsArr cannot tell. **Treatment, from the frontend thread and adopted:** the boundary that matters is **handed-over vs not-handed-over**, so **sent-unknown is placed beside *sent*, not beside *failed*** — a 200 means Prowlarr accepted the release, not that a download is running, and UsArr stops observing at handoff either way, so those two are the same epistemic state and only the genuinely-failed one is categorically different. A row that treats them as opposites **lies in both directions**. The sharper consequence is written in too: since detection is deliberately not built, **"sent" is the strongest true word for every handed-over state including the successful one** — never *succeeded*, never *downloading*. ✅ Convergence rather than coincidence, and worth noting as evidence: the provenance design had already chosen *sent* over *succeeded*, and Recent grabs' `done` state was already worded *"the client accepted it"*. The mockups were checked against the rule rather than assumed to pass it — the post-grab chip reads *"grabbed · sent to qBittorrent"*, which names the user's action and then makes the one true claim, so **no mockup copy changed**. The reason is written in as structural: **UsArr deliberately stops observing after handoff**, so it *cannot* resolve the ambiguity by looking, and this is the **second** failure mode in §17.5 where the absence of an undo makes wording load-bearing rather than cosmetic — cross-referenced to §9.1a's freeze-while-aimed rule, which rests on the same irreversibility. ✅ **The one question this raised is closed rather than left open, and it closed the same day**: the deployment thread checked Prowlarr's and Deluge's source and the response is **not distinguishable** — Prowlarr returns **no structured partial-success signal at all**, the 200 *is* the confirmation, and the only discriminator inside a failure is locale-dependent string and stack-frame archaeology specific to Deluge that does not generalise across download clients. Their recommendation not to build detection is written into §17.5 explicitly, so that the ambiguity is not later mistaken for an unclosed gap and answered with a parser. **It strengthens the framing rather than confirming it:** the ambiguity is permanent by the upstream API's nature *and* by the stop-observing decision — two independent reasons, either sufficient. **Two copy corrections follow and are specified**: the current message asserts *"the grab did not go through"*, the one claim that cannot be known, and is replaced by wording that says the download client reported an error and the release **may or may not have been added**, with the upstream message verbatim; and `Test connection` is removed as the offered action, because this is not a connectivity failure and the test will pass. **The state has somewhere to live, decided by the code thread:** `provenance` gains **`acquisition_state`** in **migration 0003** — `TEXT NOT NULL DEFAULT 'confirmed'`, partial index on the not-confirmed case, **no `CHECK` constraint on purpose**, since SQLite cannot `ALTER` one later and v0.2's requests may want `pending` (migration 0001's `audit_log` foreign key is the precedent that paid for that caution). So Recent grabs reads sent-unknown **from provenance**, not from an `audit_log` enrichment, and §17.5's union says so. The reasoning generalises past this table and is worth keeping in their words — **an absent row fails visibly; a phantom row lies quietly**: `provenance` has no back-reference to `audit_log`, so an ambiguous row without a discriminator *on the row* would let library sync's join and Recent grabs attach the wrong history silently. Recorded too: **no existing column could carry it** — `confidence` belongs to the v0.3 fuzzy-match tier and its `>= 1.0` partial index would filter out precisely the rows that matter, and `source_system` is a grouped-on enum. 🔍 **One mechanical fact, because it makes this common rather than exotic: the owner never chose a label.** Prowlarr's Add-client form pre-fills a Default Category of `prowlarr`, so saving the form opts you into the labelling step that failed, and the add happens *before* it with no rollback — a configuration trap reachable by accepting defaults, not a user error |
| **SW-25** | **A rule being applied rather than a defect being found, which is why it is logged.** The Requests thread stopped rendering the outcome detail clause on confirmed rows: it was **constant per row**, and the fact it carried is already stated once in the block header. They cited §9.1's *"a column that cannot vary is not data"* and merged it at `85cae80` | **Noted, and the distinction is the point.** §9.1's rule was written *because* our own review rounds kept finding repeated columns in the mockups — the 2160p column, the four `none` request destinations, the constant Library column in the search groups. This is the **first time another thread has applied it to their own work without being asked**, which is the only real evidence that a written rule is doing anything: a rule nobody reaches for is indistinguishable from a rule nobody wrote. This log is full of findings and thin on rules being used, so the case is recorded as an outcome rather than an incident. Attributed to the Requests thread |
| **SW-25a** | 🚩 **The second time a thread that did not write §9.1 picked it up and applied it without being asked — and the first time one caught itself with it mid-draft.** The Requests thread's category-scoped indexer picker, merged at `08483ee`, first drafted the full indexer-priority clause (*"lower wins"*) onto every row. They measured it — **three wrapped lines under every name** — recognised it as §9.1's own rule about a value identical across every row of a group, and moved it to the panel once. Nobody asked them to, and nobody reviewed the first draft | **Noted, and the increment over SW-25 is the whole reason it is a separate row.** SW-25 showed the rule being *applied*; this shows it being **reached for as a self-check while the wrong version was still being written**, which is a stronger signal and a cheaper one — the cost was a draft rather than a review round. 🚩 **That is evidence the rule is LEGIBLE, not merely correct, and legibility is the property worth tracking**: a rule nobody applies unprompted is a rule that will be relitigated, and this log stays full of findings and thin on rules being used. ℹ️ Their other choice needs no action and is recorded for the same reason: **categories render at the tree's own two levels scoped to the ticked indexers**, so a plain install shows three to eight rows rather than two hundred ids, and **nothing is folded into a bucket UsArr invented** — the same instinct as this project's rules about not inventing structure the upstream does not have. Attributed to the Requests thread; `08483ee` verified on `main` before citing |
| **SW-26** | **The frontend thread's list bench never loaded IBM Plex.** Its Vite root declared no `publicDir`, so `app.css`'s `@font-face` URLs 404'd — reproducibly, for the harness's whole life — and every row-height number it ever printed was measured on the fallback face. Six figures this project cites came from it: one-line **28 / 32 / 36** and rich-row **45 / 49 / 53** | **Re-measured, and all six confirmed** — at 2,000 rows against the real `List.svelte` and `app.css`, face verified by canvas advance-width probe rather than by `document.fonts.check` alone, **byte-identical with the face served and with it blocked**. `body { line-height: var(--leading-base) }` is a fixed 18 px *length*, not a unitless multiplier, so glyph metrics cannot move the line box; the missing font could not have moved these numbers even in principle. The null result is trustworthy because the guard was fired deliberately: `line-height: normal` *does* split the conditions (rich 43 / 47 / 51 served against 39 / 43 / 47 blocked), so the probe can distinguish what it is testing. 🚩 **The durable half is the lesson, not the bug, and it is sharper than "a harness measured the wrong font": the harness measured on the wrong face for its whole life and it did not matter — and nobody could have known that without measuring.** Two inferences ran ahead of the evidence in opposite directions and both were wrong: the original claim that the `min-height: var(--row-h)` floor explains the one-line values, and the correction that three of the six were therefore provisional. **A plausible mechanism is not a measurement.** The fix for a suspect number is to measure the number, not to reason about what could have moved it. Applied to `DESIGN-DIRECTION.md` §7.4 and ADR-0029, which also gain the corrected cause: the floor is **live but slack** — forcing `--row-h: 100px` moves every row, yet `min-height: 0` leaves a one-line row unchanged, because the natural height sits 1 px over it. The coincidence with `--row-h` at all three densities is arithmetic accident, and both documents now say to keep the value and drop the derivation. ⚠️ Scope recorded rather than generalised: **one list configuration was measured** (`stack: 'two-line'`); a `stack: 'labels'` list has one-line rows at 26 / 30 / 34 px, below the floor, where `min-height` *would* bind |
| **SW-27** | **`.tbl td > * + * { margin-top: var(--space-1) }` fires on the span following a `display: none` `.stacksep`, adding 2 px to every cell in a two-line stack.** The neighbouring exemption `.tbl td > .stacklabel + *` exists for exactly this reason and omits `.stacksep`, so the second-line separator re-creates the defect its sibling was written to prevent | **Routed to the frontend thread, not fixed here** — `web/src/app.css` is their file. Recorded because it is the same adjacent-sibling trap this thread hit from the other side while removing the mockups' inline styles (D-41): `.tbl td > * + *` at (0,1,1) and `.tbl td > .stacklabel + *` at (0,2,0) are the two rules that make a naive class replacement change spacing, and an element that is `display: none` is still an element sibling. Worth knowing in both stylesheets |
| **SW-28** | **A consumer hazard created by SW-27's own fix, flagged rather than fixed.** `--row-ci` feeds `contain-intrinsic-size`, which takes a **content-box** height. The `.stacksep` margin fix moves the `two-line` fork's one-line row from content box 28 / 32 / 36 to **27 / 31 / 35** (border box 29 / 33 / 37 → 28 / 32 / 36), so anything that keeps setting `--row-ci` from the published **28 / 32 / 36** is **1 px over per row** from the moment that fix lands | **Routed to the frontend thread, not fixed here** — `web/src/lib/list.ts` is their file and this thread does not touch `web/`. 🚩 **The reason it is worth a log entry rather than a passing remark is that nothing will tell them.** `contain-intrinsic-size: auto` self-corrects once a row has been laid out and remembered, so the estimate is only wrong for rows that have never been on screen; no gate fires, no assertion breaks, nothing visibly moves, and a 1 px-per-row scroll-height error on a 25,000-row list is 25,000 px of drift that resolves as the user scrolls — the exact symptom §7.4 exists to prevent, arriving with no signal. ⚠️ **And the digits give no warning either: "28 / 32 / 36" stays true across the fix**, as the content box before and the border box after, so a reader checking the constant against a fresh measurement gets a match and moves on. `DESIGN-DIRECTION.md` §7.4 and [ADR-0029](./DECISIONS.md#adr-0029) now name the box on every row-height figure for this reason, and §7.4 carries the standing rule that follows: **both forks, all three densities, every number with its box** |

---

# GR-01 — the code side of SW-24, as implemented

**This is the implementation record for SW-24 above**, written by the thread that wrote the code
rather than the one that decided the design. SW-24 owns the design position and the §17.5 copy rules;
this owns what shipped, the upstream source each claim rests on, and what the tests assert. Where the
two touch, SW-24 wins on design and this wins on mechanism.

**Reported from a real grab on the owner's machine.** Prowlarr added the torrent to Deluge, threw on
a later labelling step, and returned a bare 500. UsArr told him it had failed. On a private tracker a
false failure invites grabbing the same release again, which costs a duplicate download and a ratio
hit — so the harm is not cosmetic, and it is paid in the one currency the user cannot get back.

## GR-01.1 The mechanism, verified against upstream source

| Claim | Source, read directly |
|---|---|
| Prowlarr adds first and configures second, with no rollback | `Deluge.AddFromMagnetLink`: `_proxy.AddTorrentFromMagnet` → `SetTorrentSeedingConfiguration` → `SetTorrentLabel`. A throw in the tail leaves the torrent **running** |
| That throw is not caught | `DownloadService.SendReportToClient` catches `ReleaseUnavailableException`, `DownloadClientRejectedReleaseException`, `ReleaseDownloadException` — not `DownloadClientException`. `SearchController.GrabRelease` returns a bare **500** |
| The bulk endpoint is the outlier's opposite | `GrabReleases` wraps each release in its own `try`/`catch`, catches `DownloadClientException`, logs and `continue`s. Prowlarr's own bulk path treats this as non-fatal |
| The state is the DEFAULT, not a corner | `DelugeSettings`' constructor sets `Category = "prowlarr"`, so the Add-client form arrives pre-filled and saving it opts you in. Deluge's Label plugin rejects a label nobody created |
| ⚠️ Prowlarr's own Test button goes green in exactly that state | `Deluge.TestCategory()` returns `null` when `Categories.Count == 0` — the *mapped-categories* list, not `Settings.Category`. With no mappings it never checks the Label plugin and never checks the field the add path uses (`GetCategoryForRelease(release) ?? Settings.Category`) |

**A 200 is the only confirmation the API offers.** The one discriminator is a frame in the .NET stack
trace: Deluge-specific, partly gettext-translated, and matching another project's private internals.
**Not built, deliberately** — recorded in `reference/arr-apis.md` §7 and `reference/search.md`.

⚠️ **The consequence for the Services screen, recorded so nobody assumes it away later:** a passing
connection test is not evidence of a working grab, and a green Services row does not rule this out.
No health-screen logic was built for it here; the fact is recorded so it is not silently assumed.

## GR-01.2 Disposition — a three-way classification, from the sentinel and not the status

**Applied**, additively, exactly as SW-24 specifies. `grab_failed` was the unclassified remainder and carried all three outcomes. It now
carries one, and the split is derived from the `servarr` sentinel because the status code cannot
carry it — a 500 is both *"no enabled download client, nothing dispatched"* and *"added to Deluge,
then the label step threw"*.

| Outcome | Errors | Reported as |
|---|---|---|
| **Definitely not sent** | `ErrValidation`→`ErrRequestRejected`, `ErrBreakerOpen`, `ErrInvalidRequest`, `ErrUnauthorized`, `ErrConflict`, `ErrUnexpectedStatus`, a corrupt stored blob, plus the pre-dispatch codes (`instance_mismatch`, `service_unavailable`, `expired`, `no_longer_offered`, `no_download_client`) | Failure, as now. For these UsArr genuinely knows |
| **Sent, result unknown** | `ErrServer`, `ErrTimeout`, `context.Canceled` | **New code `grab_outcome_unknown`** — additive, not a rename, so nothing branching on `grab_failed` breaks. `audit_log.result = "warn"` |
| **Sent and confirmed** | `ErrDecode` | **Success.** See GR-01.3 |

**The remainder stays in "not sent" on purpose.** Moving an error into sent-unknown has to be a
positive decision backed by evidence the POST left the process; defaulting there would make every
ordinary failure read as ambiguous, and a message that appears on failures which did not dispatch is
a message people learn to ignore — including on the one occasion it is true.

## GR-01.3 `ErrDecode` — a second, independent false failure

**The reachability claim was verified before turning a failure into a success**, because getting it
backwards would be the worst available outcome. In `servarr.Client.do` the non-2xx branch
(`resp.StatusCode < 200 || resp.StatusCode >= 300`) **returns `parseErrorBody`'s `*APIError` before
any decode of the body is attempted**; `ErrDecode` is produced only by the two statements after it,
on an empty body and on a failed `json.Unmarshal`. `Client.Grab` passes a non-nil `out`, so it
reaches them. **`ErrDecode` is therefore reachable only after a 2xx**: Prowlarr confirmed the grab
and UsArr failed to parse its own receipt. Reported as a failure until now.

## GR-01.4 Storage — the join key, and why it needed migration 0003

Failure paths wrote nothing to `provenance` and an `audit_log` row carrying only
`{"instance_id":N}` with `result="fail"` — no error code, so the read side could not tell the three
outcomes apart. Both halves are fixed.

**The audit row** now carries `outcome` (`not_sent` | `sent_unknown` | `sent_confirmed`), the error
`code`, and `provenance_id`. `"warn"` is used for sent-unknown; `internal/store/audit.go` had always
documented the value and nothing used it. It reads naturally: the action's outcome is genuinely not
known, which is what a warning is.

**The provenance row** is the harder half. A torrent on disk with no `provenance` row loses its
infohash — `download_id` is the only key an importer supplies — so when library sync lands there is
nothing to join back to. But writing the row **without a discriminator on the row itself** is worse
than writing none: `provenance` has **no back-reference to `audit_log`** (verified — `audit_log`'s
`target_id` holds the *release-candidate* id, and that candidate is swept 25 minutes later), so a
reader starting from `provenance` — which is what the import join and Recent grabs both do — could
not tell an unconfirmed acquisition from a confirmed one. The join would then *succeed* and attach
wrong history.

No existing column could carry it. `confidence` means match confidence and is gated by a partial
index at `>= 1.0`, so an unconfirmed-but-perfectly-identified row would be filtered out by the
queries that most want it; `source_system` is a documented enum future reads group by. So
**migration 0003** adds `provenance.acquisition_state TEXT NOT NULL DEFAULT 'confirmed'`, plus
`ix_prov_unconfirmed` partial on `acquisition_state <> 'confirmed'`.

**Deliberately no `CHECK` constraint**, following `audit_log.result` exactly: SQLite cannot `ALTER`
one, and 0001's `audit_log` foreign key is what that costs when the vocabulary later has to grow
(v0.2's requests may want `pending`). The vocabulary is documented and enforced in Go, in
`internal/store/releases.go`. `ADD COLUMN … NOT NULL DEFAULT` on a `STRICT` table was **verified by
execution**, as 0002 was, not by memory — `TestMigration0003NeedsNoRebuild` pins the column shape,
that `provenance` is still `STRICT`, that the backfill is the default, that the index is partial, and
that the planner reaches for it.

`store.GetProvenanceByDownloadID` now returns `acquisition_state` alongside the key, so the one read
that an unconfirmed row exists to serve cannot use the key without seeing the state.

**The wording is built to `ARCHITECTURE.md` §17.5's spec rather than invented here**, since the
frontend renders against that section: the message leads with §17.5's own two clauses — the download
client reported an error, and the release *may or may not have been added* — quotes Prowlarr
verbatim, and offers no misleading action. What it adds is mechanism, not a guess: Prowlarr hands the
release over *before* it applies settings, which is why the instruction is to check the download
client before grabbing again rather than merely to be aware. **`Test connection` is gone**, and the
tests fail if it comes back.

## GR-01.5 Also fixed — `no_longer_offered` was blaming the indexer for UsArr's own timeout

One of that code's three paths fired when the transparent **re-search** failed (timeout, open
breaker, 401) and told the user *"that indexer no longer offers this release"*. Nothing was sent, so
this is not the same bug — the verdict was right and the **cause was invented**. It now has its own
sentinel (`ErrReSearchFailed`) and its own message, which says the cache had dropped the release,
that the recovery search did not complete, and that nothing was sent to the download client.

## GR-01.6 What the tests assert

`internal/releases/grab_outcome_test.go` and `internal/httpapi/grab_error_test.go`, one per bucket,
each failing if an error moves:

- **sent-unknown** — a 500 with a message, a bare 500, a timeout and a cancellation each classify as
  `ErrGrabOutcomeUnknown`, write exactly one provenance row, carry the infohash in `download_id`, and
  carry `acquisition_state = 'unconfirmed'` with `confidence` still 1.0.
- **definitely not sent** — seven errors, none of which may classify as sent-unknown or carry its
  wording, and none of which may write a provenance row. This is the guard against reintroducing the
  harm in the other direction.
- **sent and confirmed** — `ErrDecode` returns no error, sets `ResponseUnreadable`, and writes a
  `confirmed` row.
- **wording** — the sent-unknown message says the release was sent, says the outcome is unknown,
  asserts neither, quotes Prowlarr, names the remedy and the screen it lives on, warns about the
  duplicate grab, and offers no `Test connection` action. A separate case covers the no-answer
  variant, which must not dangle a reference to a message that does not exist.

## GR-01.7 Not done, and why

- **No UI change.** Design owns the position that ambiguous rows get no Retry affordance.
- **No stack-trace parsing.** See GR-01.1.
- **No health-screen logic** for the green-test trap; recorded only.

---

# IX-01 — `GET /api/v1/indexers`, and the two ways it could have shipped wrong

**Origin: real use, then a design correction mid-implementation.** Joe asked to filter release search
by category or by a specific indexer. The frontend thread found there was no endpoint listing a
service's indexers at all — their picker was populated from `search.indexer` SSE frames, so it was
empty until a search had already run, and the screen said so. They judged the endpoint small. It is
small; the two ways it could have shipped wrong are not.

## IX-01.1 The render-path trap, and why the obvious shape is the forbidden one

`GET /api/v1/indexer` on Prowlarr is one cheap call, so proxying it looks harmless. It is not: **the
picker paints before the search runs**, which makes this a render path, and no render path may block
on an *Arr (ARCHITECTURE §2.3 rule 1). The trap is that the violation hides behind a projection —
"the handler only builds a small struct" is still a remote call under a browser's paint, and it is
the shape a reviewer waves through.

**An in-process cache was considered and rejected, and the rejection is the useful record.** It was
offered as an acceptable cheaper option and turned down by the coordinating thread on one argument
that settles it: a process-lifetime map does not remove the caveat, it **relocates** it — *empty
until you have searched once* becomes *empty until the first probe after every restart*, and the
screen still has to apologise for itself. The endpoint exists to delete that sentence.

So: **migration 0004, `indexer_catalog`**, written by the background prober — the same loop, the same
schedule and the same `ProbeNow` hook that already refresh health warnings and blocked indexers, so
service-add and a successful connection test both refresh it for free and no new timer exists. The
handler reads the replica and returns. Precedent, not novelty: `service_instance`'s health columns
are persisted for the stated reason that *"a restart must not blank the Services screen"*.

Three consequences that only a replica gets, and each is tested:

- **A failed refresh writes nothing.** The last good copy stands, and `fetched_at` says how old it
  is. An empty picker is worse than a stale one.
- **The replica outlives the upstream.** `TestEndToEndIndexerCatalogue` tears down the Prowlarr
  double and asserts the endpoint still serves all three indexers.
- **Three empty-ish states stay distinguishable**, because they are three different sentences: no
  indexer service configured · configured but never successfully read · read, and genuinely zero
  indexers. Zero rows cannot separate the last two, so the fetch time is stamped on the *instance*
  (`service_instance.indexers_fetched_at`, nullable with no default — NULL means never). Returning
  an empty list for all three would be the empty-screen-that-looks-broken failure wearing a 200.

⚠️ **The endpoint answers 200 for all four states** where the search path answers 409. That is not an
inconsistency: search was *asked to do something it cannot do*, while this is a list, and a picker
that 4xx's on a fresh install paints an error box on a screen where nothing is wrong. Only a
malformed request fails — `?instance=` that is not an id, is not visible, or names a non-indexer.

## IX-01.2 The credential, and why redaction was not on the table

`IndexerResource.fields[]` carries the indexer's own credentials under
`privacy ∈ {password, apiKey, userName}`: on a private tracker that is the user's **passkey**, its
RSS key, its API key and its session cookie. A leaked passkey is account termination, because it is
what the tracker attributes traffic by. **This project has already paid for this leak class once** —
`release_candidate.info_url` held tracker passkeys verbatim in SQLite, and therefore in every backup,
while the HTTP responses looked clean.

The frontend thread endorsed the warning and asked for it stronger; the coordinating thread agreed
and specified the mechanism: **a field-by-field allowlist, not a denylist and not a redaction pass
over a pass-through.** Shipped as three successive allowlists, so the value has nowhere to land at
any layer:

| Layer | Type | What makes it an allowlist |
|---|---|---|
| Projection | `mapping.CatalogIndexer` | Every member assigned by name. No `out := ix`, no embedding, no `map[string]any`. `Presets` dropped too — preset entries are `IndexerResource` values and carry their own `fields[]` |
| Storage | `indexer_catalog` | No `fields` column, no cookie column, no raw-resource column. `search_types` and `categories` hold **UsArr's own JSON**, marshalled from the projection, never re-serialised upstream JSON |
| Wire | `httpapi.indexerResponse` | Declared members only; the two JSON columns decode into declared shapes |

**Note what is *not* done: nothing filters `fields[]` by `privacy`.** The fixture's cookie entry
carries no `privacy` marker at all, which is exactly why filtering would have leaked it. The array is
never carried, so the marker never has to be trusted.

**The guards were fired deliberately rather than trusted.** `FromProwlarrIndexer` was temporarily
edited to append every field value to the indexer's name; both
`TestCatalogProjectionCannotCarryAnIndexerCredential` (unit) and `TestEndToEndIndexerCatalogue`
(whole stack, through the real HTTP boundary) failed, and the edit was reverted. Both assertions run
over the **marshalled byte string**, not over parsed members, so they cannot pass because the test
forgot to look at a field — which is precisely the failure being guarded. `TestMigration0004NeedsNoRebuild`
pins the exact column set, so adding a "let's keep the raw resource so we needn't re-fetch it" column
fails a test rather than passing a review.

## IX-01.3 Also fixed, in passing

`supportsSearchType` had one home in `internal/releases`' fan-out planner. The picker needs the same
answer — which indexers can serve the selected search type — and a second copy would have let the
picker offer an indexer the planner then skips, which reads as a broken filter rather than as a
deliberate skip. It now lives once, in `internal/servarr/mapping`, and the planner delegates to it.

## IX-01.4 Not done, and why

- **No new configuration key** for the refresh interval. It is the prober's existing 60 s
  (`probeInterval`, `cmd/usarr/services.go`), and a second knob for the same loop is surface without
  a question behind it.
- **No `IN (…)` read across instances.** One instance per call, so `service_instance_id = ?` is an
  equality on the index's leading column and `name` supplies the `ORDER BY`; a set read cannot, and a
  homelab has single-digit instances.
- **No filtering of disabled indexers.** They are listed and marked. Hiding one makes *"why is my
  indexer missing?"* unanswerable from the screen that is supposed to answer it.

---

# RG-01 — the Recent-grabs batch, reviewed adversarially and re-checked against a moved `main`

**Origin: an adversarial pass over the Requests thread's Recent-grabs work at `85cae80`.** `main`
had moved by the time the findings were recorded — `dd15d95` landed `GET /api/v1/indexers` and
`ec2a21d` merged it — so **every finding below was re-verified by reading the current tree at
`ec2a21d` rather than the review's diff.** None was overtaken; the closed item at the end was
already closed at `85cae80` and is confirmed closed here for the second time, by a different method.

Seven findings, one follow-up and one closure. **Two are applied (RG-01.2, and RG-01.3 once the
client went opaque at `23cac0f`), one routes to another thread (RG-01.1), and four are rebutted
(RG-01.4 to RG-01.7)** — written down rather than dropped, because a rebuttal that is not on paper
gets re-litigated by the next reviewer to notice the same shape.

## RG-01.1 — §17.5 still specifies a Recent-grabs block that did not ship. **Open; routes to the design thread.**

`docs/ARCHITECTURE.md` §17.5 (the *"A grab leaves a record"* paragraph, line 3019, and §16's cost
note at line 2286) specifies the block as **one keyset-paginated read joining `write_queue` to
`provenance`**, rendering *"the library or media type the category resolved to"* and **last known
state from the write queue's own vocabulary — `pending | inflight | verifying | done | failed`**.

**What shipped is none of those.** Six columns (When · Release · Indexer · Protocol · Size ·
Outcome); `provenance.acquisition_state` rendered through a two-value wire vocabulary; **no join**;
**no keyset pagination** — a `LIMIT` with a server-clamped ceiling instead.

**Every deviation is deliberate and each has a stated reason** — nothing writes `write_queue` yet,
and the category resolver lives in `internal/servarr/mapping`, which `internal/httpapi` may not
import (`doc.go`). **The defect is not the deviation, it is where the reason lives:** a 19-line
comment at `internal/httpapi/grabs.go:17-35`, which is an honest record in the one place a reader
consulting the design will never look. §16 is authoritative for *what ships*; §17.5 is still the
only prose description of *this block*, and it describes something else.

⚠️ **Not fixed here, deliberately. §17 belongs to the design thread** under this repo's
file-ownership convention, and a code thread editing another thread's section is how two threads
produce one conflict. **Routed there** with the three specifics: the state vocabulary, the resolved
media-type column, and the pagination shape.

> ✅ **Applied by the design thread, 2026-08-16, and the sweep found the SAME defect pointing the
> other way in the same section.** §17.5 now carries a shipped-against-target table covering all
> four deviations — the six columns, the `provenance.acquisition_state` outcome vocabulary in place
> of the write queue's five states, the clamped `LIMIT` in place of the keyset join, and the absent
> *nothing-was-sent* state — with the reason for each moved out of the comment above the handler in
> `internal/httpapi/grabs.go` and into the document a reader consulting the design will actually
> open. §16's cost note is marked in place rather than rewritten, because **a cost estimate edited
> after the fact stops being evidence about estimating**. 📌 **The specification is not narrowed to
> what shipped**, which was the routing's one real risk: all four target properties are still
> wanted, and the table doubles as the checklist for closing the distance, ordered by dependency —
> `write_queue` needs a writer before anything else on it can move.
>
> 🚩 **The observation worth more than the fix: §17.5 was wrong in BOTH DIRECTIONS AT ONCE.** The
> block above reads as a description of the present while specifying a future shape; four hundred
> words later the same section said *"the code thread **is adding** `acquisition_state` to
> `provenance` in migration 0003"* — a future tense over something that had already landed at
> `f895ddc`, column, partial index `ix_prov_unconfirmed` and four readers included. Verified against
> the tree before either edit was written, because a relayed "it shipped" is a hypothesis: `f895ddc`
> is an ancestor of `main`, `internal/db/migrations/00003_provenance_acquisition_state.sql` is
> merged, and `internal/store/releases.go`, `internal/releases/grab.go`, `internal/httpapi/grabs.go`
> and `web/src/lib/api.ts` all read the column. Had it been in flight, the correct edit would have
> been the opposite one and a sweep looking for one direction only would have written the drift
> back in. **This is why the failure is not "we forget to update docs when we ship."** If that were
> the cause, every drift would point the same way — doc behind tree. Drift in both directions inside
> one section means **nothing checks tense against the tree at all**, and the two errors are one
> error with two signs. Recorded here rather than fixed with a checker, because the checker is a
> real proposal and not this pass's work: `docs/DEVELOPMENT.md` §11's "fire a guard deliberately"
> rule is the shape it would take.

## RG-01.2 — the secret-leak guard was a denylist. **Applied.**

`internal/httpapi/grabs_test.go` asserted the response carries none of `download_url · downloadUrl ·
magnet · apikey · api_key · apiKey · passkey · nzb_info_url · info_url · http:// · https://`.

**A denylist passes everything it does not enumerate, and this one did not enumerate enough.**
`provenance` carries four columns the list had no entry for — **`guid` / `release_guid` /
`torrent_info_hash` / `download_url`** (`internal/store/releases.go:310`) — and **a Newznab `guid` is
frequently the download URL itself**, therefore passkey-bearing on a private tracker. This is the
same credential class that reached permanent storage once already (IX-01.2, `release_candidate.info_url`).

**Not exploitable as it stood, and that is not a defence.** The SELECT list at
`internal/store/releases.go:422-423` is explicit and omits all four, so nothing leaks today. The
guard's job is to catch the *next* field, and a name-based ban list fails at exactly that: the field
nobody thought to ban is the field that ships.

**Converted to an allowlist over the marshalled JSON keys** (`internal/httpapi/grabs_test.go:145-234`):
the response is unmarshalled, **every object key at every depth** is walked — nesting matters,
because a leak inside `grabs[]` is still a leak — and any key not in `recentGrabsWireKeys` fails.
Adding a member to `recentGrabResponse` now fails a test until its name is added to the constant
deliberately, which is the review step expressed as code.

**This is IX-01.2's idiom, reused rather than reinvented.** `mapping.TestCatalogProjectionCannotCarryAnIndexerCredential`
settled on an allowlist over the marshalled bytes for the same reason; two shapes of the same guard
is how one of them rots.

**The value half was kept, not dropped.** The allowlist governs *keys*; a URL smuggled into an
allowed key's *value* would pass it. So `passkey · magnet: · http:// · https://` still run as a
substring assertion over the body, and the seeded row's `nzb_info_url` is a passkey-bearing tracker
URL precisely so that half has something real to catch. The floor assertion is unchanged: the
response must contain the seeded release title first, so *"found nothing"* and *"looked at nothing"*
cannot produce the same green (DEVELOPMENT §11 rule 4).

**The guard was fired deliberately before it was trusted**, in two probes, both reverted:

- **A key probe** — `ReleaseGUID string \`json:"release_guid,omitempty"\`` added to
  `recentGrabResponse` with the entirely **benign** value `"a-benign-looking-guid"`. The test failed
  on the *key*, nested inside `grabs[]`. **The old denylist would have passed this response
  untouched** — which is the finding, reproduced.
- **A value probe** — `SourceSystem` (an allowed key) set to
  `http://tracker.example/rss?passkey=deadbeef`. The test failed twice, on `passkey` and on `http://`.

Both reverted; the suite is green.

## RG-01.3 — `provenance.id` on the wire is a cross-user volume oracle. **Applied.**

`internal/httpapi/grabs.go:46` ships `ID int64 \`json:"id"\``, which is `provenance.id` — `INTEGER
PRIMARY KEY`, therefore **a globally monotonic rowid shared across every user**.

Under multi-user — **which principle 4 says the schema is already built for**, and migration 0002
gave `provenance` its `user_id` — a caller who sees `id:104` on one of their own grabs and `id:341`
on the next **learns that 236 rows were written by other users in between**. The scope filter is
correct and the rows themselves never cross; **the count leaks through the identifier**, which is
why the filter does not stop it.

**This is not an authorization break** and is not being reported as one. Today there is exactly one
user, so the oracle has nobody to inform.

**Deferred rather than applied, because the field is load-bearing on a screen another thread just
shipped:** `web/src/routes/requests/+page.svelte:535` keys list rows on it
(`key={(g) => String(g.id)}`) and `web/src/lib/api.ts`'s `toRecentGrab` **rejects any row without it** (line 1329 as of `1d7fa01`; that file is churning, so the symbol is the citation) — a change
here blanks the block rather than degrading it.

**Recommended fix, for whoever coordinates it:** an opaque per-row identifier, or a per-user
sequence, exposed instead of the rowid. ⚠️ **It is far cheaper now than later.** One published shape
and one consumer pin it today; every additional client pins it harder, and this is the last cheap
moment.

### Applied — the coordination completed, and the deferral's own condition was met first

**The client went opaque before the server did**, which is what closed the ordering window the
deferral named. `23cac0f` retyped `RecentGrab.id` as a `string` and gave `toRecentGrab` a `rowKey`
helper that accepts a non-empty string **or** a finite number it stringifies, so both wire forms
render and the two changes could land in either order without a blank block in between. The
deferral asked for coordination, not for the finding to lapse; this is the coordination.

**What ships:** `recentGrabResponse.ID` is now a `string` carrying `grabRowID`'s keyed hash
(`internal/httpapi/grabs.go`). **The wire key is unchanged — still `id`** — because the client's own
note said the replacement would arrive under the same name, and renaming it would have reopened the
ordering window the retype closed. An example value: `se_wB7GtQhzj_fdHcvNG8A`.

**HMAC-SHA256, truncated to 128 bits, base64url without padding — 22 characters.** The key comes
from a **new** `crypto.DeriveGrabRowIDKey`, under a **new** HKDF info label `usarr/grab-row-id/v1`.
⚠️ **No existing label was touched, and that restraint is the point.** `usarr/kek/v1`,
`usarr/stream-token/v1` and `usarr/client-credential/v1` are domain-separation inputs bound into
stored ciphertext and issued API keys; editing one silently makes every stored credential
undecryptable, and `derive_test.go` carries no golden vectors, so nothing would catch it. A new
purpose gets a new label. `TestDerivedKeysAreDistinct` now covers all four pairwise.

**A keyed hash rather than a per-user sequence, deliberately.** A sequence needs a column, therefore
a migration, and *"a merged migration is never edited"* — the schema is where this project's
expensive mistakes live. The hash needs nothing stored at all.

**`user_id` is in the HMAC input alongside the rowid.** It costs one field and buys per-user domain
separation: the shared/system sentinel-`0` rows migration 0002 backfilled, and any future view that
widens the scope, would otherwise hand two users the same token for one row and let them correlate
by comparing. Nothing joins on this value, so there is no cost to it differing per user, and
`provenance.user_id` is historical and never rewritten, so stability holds. Both inputs are
fixed-width big-endian, so `(user 1, row 23)` and `(user 12, row 3)` cannot render the same bytes.

**Three tests, pinning the properties rather than the construction** — a truncated hash, a different
hash, or a per-user sequence with a random base would all pass them; a rowid dressed in hex or
base64, which is the tempting cheap fix, fails every one:

- **`TestGrabRowIDIsStableAcrossServerInstances`** — the same row gives the same id across two calls,
  and across a **second server built on the same derived key**, which stands in for a restart. This
  is the arm that rejects a random per-response token; the client keys rows by identity for focus
  and hover, so an id that moves rebuilds the block under the cursor.
- **`TestGrabRowIDCarriesNoOrderOrVolume`** — sorting 64 ids must not reproduce rowid order;
  adjacent rowids must differ in 32–96 of 128 bits and share no leading prefix; the distance to the
  **neighbour** and to the **63rd row** must both sit near half, so the *gap* is not recoverable —
  which is the oracle itself. Plus: two users' ids for one row differ, the fixed-width collision
  case differs, and a different install key gives different ids.
- **`TestRecentGrabsNeverShipsAURLOrACredential`, extended rather than duplicated.** The raw rowid
  would **not** have tripped RG-01.2's allowlist: it leaks through `id`, a key that is *on* the list
  and must stay there. Only a value-level assertion can catch it, so it sits beside the existing
  value check — two shapes of the same guard is how one of them rots (RG-01.2's own argument).

**Both guards were fired deliberately before being trusted** (DEVELOPMENT §11), both reverted:

- **The leak assertion** — `ID` kept as a `string` but filled with `strconv.FormatInt(p.ID, 10)`,
  i.e. the exact cheap "just make it a string" fix. It failed three ways:
  `the response ships the raw provenance rowid 1 as a string; stringifying it changes the type and
  keeps the leak` on the body `{"grabs":[{"id":"1",…}]}`, then `the wire id "1" parses as an
  integer`, then `the wire id is not the keyed hash of (user_id, rowid)`.
- **The order/volume assertion** — `grabRowID` replaced with big-endian rowid in 16 bytes. It failed
  on `sorting the ids reproduces rowid order`, on `rowids 1 and 2 differ in 2 of 128 bits`, and on
  the shared prefix `"AAAAAAAAAAAAAAAAAAAAAQ"` / `"AAAAAAAAAAAAAAAAAAAAAg"`.

**`Config.GrabRowIDKey` is required and `httpapi.New` refuses a wrong length.** The fallback for a
missing key would be shipping the rowid, which is the leak the key exists to close; it fails closed
instead. `cmd/usarr` derives it beside the KEK, from the same master key and salt, which is what
makes the id survive a restart.

**Still on the wire elsewhere, and out of this change's scope:** `grabResponse.ProvenanceID` in
`internal/httpapi/grab.go` returns the raw rowid to the caller who *just made* that grab. That is a
much weaker oracle — one id, self-inflicted, no second sample to difference against — but it is the
same column, and it is a follow-up rather than a thing this finding closed.

## RG-01.4 — the `ORDER BY` is not a DoS. **Rebutted on measurement.**

The suspicion: `ORDER BY grabbed_at DESC, id DESC` over a user-scoped `IN (…)` predicate forces a
full sort of the user's history on every call, so a large `provenance` makes a cheap endpoint
expensive.

**Measured, it does not.** Each `IN` branch arrives **pre-ordered from `ix_prov_user_grabbed`**
(`user_id, grabbed_at DESC, id DESC`, migration 0002 line 94), so SQLite merges two ordered streams
into a bounded sorter and **early-terminates at the limit**. The scoped read is **flat from 1k to
100k rows — 238µs → 120µs** (the fall is noise, not a speed-up). The control, a genuinely unindexed
sort over the same tables, **grows 90× across the same range: 587µs → 53.7ms**. A control that grows
is what makes the flat line evidence rather than an absent measurement.

**And the ceiling is enforced twice, independently:** `handleRecentGrabs`
(`internal/httpapi/grabs.go`) clamps to `store.RecentProvenanceMaxLimit`, and
`recentProvenanceSQL` (`internal/store/releases.go:415-419`) clamps again inside the store — so a
future caller that reaches the store without the handler cannot lift the cap.
`RecentProvenanceMaxLimit = 200`, `RecentProvenanceDefaultLimit = 10`. The query plan is already
pinned by `internal/store/provenance_recent_test.go:217`, which asserts
`SEARCH … USING INDEX ix_prov_user_grabbed`.

## RG-01.5 — no upstream error string reaches the client. **Rebutted.**

No path on this endpoint returns an upstream body, header or error text. Every failure goes through
`(*Server).writeError` (`internal/httpapi/json.go:130-158`), which emits **only** `errorBody{Error,
Message, Action}` — `Message` and `Action` each passed through `redactText`, and the underlying cause
sent to the log (also redacted) rather than to the body. §17.5's *"a `failed` row carries the
verbatim upstream error"* is a **write-queue** row, which this endpoint does not render at all
(see RG-01.1), so the verbatim-error path is not merely redacted here — it is absent.

## RG-01.6 — no rollup existence oracle. **Rebutted.**

The concern: a total row count would tell a caller how much exists beyond what they may read.
**The response carries no server-side total.** `recentGrabsResponse` is `{grabs, limit}`, and `limit`
is the *applied* limit, echoed because the server clamps — not a count of anything.

The header count on the screen is **derived client-side and only when it is unambiguous**:
`web/src/routes/requests/+page.svelte:160` reads
`grabs.length < grabsLimit ? grabs.length : undefined`, so a short page shows an exact number it
already holds and **a full page shows none** — the block degrades to *"the ten most recent"*. Nothing
is inferred about rows the caller cannot see, because nothing counts them.

## RG-01.7 — the route is authenticated and its scope is in the SQL. **Rebutted.**

`handleRecentGrabs` takes its session from `sessionFrom(r)` and returns **401 with no body content**
if there is none; `TestRecentGrabsRequiresASessionCookie`
(`internal/httpapi/grabs_test.go:270-282`) drives it through the **real handler stack** and asserts
both the 401 **and that the body contains no `release_title`** — refusing is not enough if the
refusal leaks the thing it refused. `TestRecentGrabsShowsOnlyTheCallersGrabs` covers the positive
case on the bytes.

📝 **One correction to the finding as originally worded**, recorded because precision here is the
whole point. The rebuttal cited `storeScope` failing closed as `1=0`. **That clause is the
*instance* predicate, and it is not what filters this read.** `storeScope`
(`internal/httpapi/auth.go:77-85`) returns `store.Scope{UserID: …}` with `AllInstances` false for a
non-owner, and the recent-provenance query filters through `Scope.userPredicate`
(`internal/store/store.go:116-118`) → **`user_id IN (?, ?)`** — the system sentinel `0` plus the
caller. The sentinel is deliberate: migration 0002 backfilled every pre-attribution row to it, and
reading it as *"not mine"* would hide the owner's own history from them. **The rebuttal stands** —
authenticated route, user-scoped SQL, asserted on the bytes — **on the correct mechanism.**

## RG-01.8 — JSON responses set `nosniff` but not `Cache-Control: no-store`. **Applied by `2a2d9b1`.**

`writeJSON` (`internal/httpapi/json.go:81-82`) sets `Content-Type` and `X-Content-Type-Options:
nosniff`, and **no `Cache-Control`**. A body of release titles is exactly the thing that should not
sit in a shared or intermediary cache, and Recent grabs is not special here — **this is every JSON
endpoint in `internal/httpapi`**, which is why it is recorded as its own item rather than folded
into RG-01.

**Deliberately not fixed in this commit.** A one-line change in `writeJSON` alters the headers of
every API response at once; that is a change with its own blast radius and its own review, not a
rider on a test conversion.

✅ **Applied by `2a2d9b1`**, as its own change — which is what the deferral asked for, and the
deferral was right: the review the change deserved is the three questions below, none of which a
rider on a test conversion would have asked.

**Scope: every `writeJSON` response, not an allowlist.** `writeJSON` is the single choke point every
JSON body in the package crosses — every handler success, `writeError`'s error shape, and
`recoverMiddleware`'s panic body — so one line there is what covers the endpoint someone adds next.
A per-endpoint allowlist has the opposite failure mode: it keeps looking correct while the list falls
behind the router. **Nothing in the tree depends on caching an API response**, and this was checked
rather than assumed — `internal/httpapi` emits no `ETag` and no `Last-Modified` on any route, and
`requestJson` (`web/src/lib/api.ts:511-520`) sets no `cache` option and sends no conditional request.
The only `Cache-Control` the product wants is `internal/web`'s, on a different handler.

**`no-store`, not `no-cache`.** `no-cache` permits a cache to **store** the response and merely
requires revalidation before reuse (RFC 9111 §5.2.2.4); `no-store` forbids storing any part of it
(§5.2.2.5). Storage is the thing objected to, so `no-cache` would buy the risk and none of the
benefit — with no validator anywhere in the package, a cache holding a stored copy has nothing to
revalidate against and goes to origin regardless. `private` is not added beside it: `no-store`
already binds every cache, private ones included, and the pair only invites "which one wins".

**The health endpoints are NOT exempt**, and the argument for them is operational rather than about
privacy. They carry no user data, but a stale `ready` served out of an intermediary is precisely the
failure that hides a process which is no longer ready, and `/api/v1/system/status` carries build and
schema versions that `health.go:80-81` already names fingerprinting material. Exempting them would
also be an allowlist wearing the other hat.

**SSE does not collide.** `handleEvents` sets `Cache-Control: no-cache, no-transform`
(`internal/httpapi/events.go:206-213`) and then calls `WriteHeader`; both of its error returns are
**above** that block and every return below it is `nil`, so no `writeJSON` ever runs behind those
headers. The stream keeps `no-cache, no-transform` unchanged — `no-transform` is the load-bearing
half there, against a compressing proxy that would buffer the stream into uselessness, and a
never-terminating `text/event-stream` is not a body a cache stores and replays.

**Asserted, and the guard was watched failing.** `TestSecurityHeadersArePinnedOnEveryResponse`
(`internal/httpapi/security_headers_test.go`) now compares `Cache-Control` **exactly** on both JSON
rows, so a weakening to `no-cache` or `private` fails there. Deleting the `h.Set` fails it twice —
`Cache-Control = "", want "no-store"` on `api_json` and on `api_error` — and restoring it passes all
three subtests. The SPA row is deliberately blank: `internal/web` owns that policy (`immutable` for
hashed assets, `no-cache` for the document) and `internal/web/web_test.go:111,179` pins it, so an
assertion here would only pin the stub SPA the test installs.

📝 **One thing rode along, and it is named rather than buried.** Folding `writeJSON`'s
marshal-failure branch into the shared header block — instead of duplicating three `Set` calls —
gives the encode-failure response the `X-Content-Type-Options: nosniff` it had been silently missing.
The middleware set it anyway, so nothing was exposed; the divergence was the defect.

## RG-01.9 — the `outcomeSentUnknown` constant collision. **Closed by `0cb1a18`, re-verified here.**

Verified **against the current tree at `ec2a21d`, not against the fixing diff** — a diff shows what
one commit did, not what the tree now holds after other merges landed beside it:

- **Six distinct declarations, no duplicate identifier.** `internal/httpapi/grabs.go:112-114`
  (`wireOutcomeSent` · `wireOutcomeSentUnknown` · `wireOutcomeStateUnknown`) and
  `internal/httpapi/grab.go:169-171` (`outcomeNotSent` · `outcomeSentUnknown` · `outcomeSentConfirmed`).
- **All seven non-test call sites resolve to the intended vocabulary.** `grabs.go:147,149,151` take
  the `wire` set; `grab.go:107,109,128,276` take the `audit_log.metadata_json` set. No site reaches
  across.
- **The wire values match what the client pins by string.** `sent` · `sent_outcome_unknown` ·
  `unknown` against `web/src/lib/requests.ts:205-207`.

**Nothing left to rename**, and the `wire` prefix is documented in place as lore-bearing rather than
decorative — the two vocabularies genuinely disagree (`sent_unknown` there, `sent_outcome_unknown`
here) and **must** stay apart: one is an internal record with its own history, the other is a
published shape a client pins.

## RH-01 — §7.4 and ADR-0029 carried pre-fix row heights against a post-fix tree. **Applied by `08599d8`, merged as `eb78308`.**

**The finding is not "the numbers were wrong".** Both documents *labelled* their figures as pre-fix,
so nothing in them was lying. The finding is that **`28 / 32 / 36` is true on both sides of
`440e92d`** — the shipped one-line row's **content** box before the `.stacksep` margin fix and its
**border** box after — so the staleness was undetectable by the one check a reader would run. A digit
that keeps its value while its meaning moves underneath does not look stale, and a fresh measurement
confirms it either way.

**Measured on both forks at all three densities, which §7.4's own standing rule for this row
requires**, on `origin/main` at `70a61c9` (with `440e92d` confirmed an ancestor) against the pre-fix
control `3ae0d44^`. Same script both trees; one-line rows at 1440×900; the three traps
`docs/design/check.mjs`'s header records all handled — `await document.fonts.ready` with the face
served out of `web/static` and confirmed drawing by canvas advance probe (IBM Plex 218 px against the
fallback's 221.806 px), `content-visibility: visible` forced so no off-screen row reports its
`contain-intrinsic-size` placeholder, and heights only.

| Fork, tree | border box, as rendered | content box, as rendered | border box, natural | content box, natural | floor |
|---|---|---|---|---|---|
| `two-line`, post-fix | 28 / 32 / 36 | 27 / 31 / 35 | 27 / 31 / 35 | 26 / 30 / 34 | binds |
| `labels`, post-fix | 28 / 32 / 36 | 27 / 31 / 35 | 27 / 31 / 35 | 26 / 30 / 34 | binds |
| `two-line`, pre-fix | 29 / 33 / 37 | 28 / 32 / 36 | 29 / 33 / 37 | 28 / 32 / 36 | **inert** |
| `labels`, pre-fix | 28 / 32 / 36 | 27 / 31 / 35 | 27 / 31 / 35 | 26 / 30 / 34 | binds |

Rich rows are unmoved by the fix and measured rather than assumed to be: **border box 45 / 49 / 53,
content box 44 / 48 / 52**, floor slack, byte-identical on both trees.

⚠️ **AMENDED: those rich-row figures are the MODE, and this entry originally gave them as though the
rich row had one height.** It does not — it is **bimodal**, because rows carrying more chips wrap.
Measured by the frontend thread at compact over 2,000 rows, the content boxes split **44 px × 1,308
and 48 px × 692**, so the mode is content box 44 / 48 / 52 and the **mean** is 45.4 / 49.4 / 53.4.
🚩 **`45 / 49 / 53` is therefore the mean content box AND the modal border box at the same time**,
which is precisely why it has never looked ambiguous — it is the same failure mode this entry is
about, one level up: a figure that keeps its digits while the quantity underneath it changes.

✅ **THE `labels`-FORK DISAGREEMENT IS RESOLVED, AND NO PARTY TO IT WAS WRONG.** Three figures were
standing — §7.4's **26 / 30 / 34**, the frontend thread's **28 / 32 / 36**, and a separate
measurement's **27 / 31 / 35** — and they were not competing readings of one quantity. They are three
different quantities, all three correct and simultaneous:

- **26 / 30 / 34** is the **natural content box**, `min-height` forced to `0`. That is what §7.4
  carried with no box named, and it is why the clause attached to it — *"below the floor, where
  `min-height` would bind"* — was right. It is `2 × --row-pad-y + --leading-base` exactly: 4/6/8 px
  doubled plus a fixed 18 px leading.
- **27 / 31 / 35** is the **content box as rendered**, floor live.
- **28 / 32 / 36** is the **border box as rendered** — the same row as the line above, plus its 1 px
  bottom border.

The reason they read as a 2 px contradiction is that 26 / 30 / 34 sits **two boxes** below
28 / 32 / 36, one `min-height` and one border, and only one of the three ever said which box it was.
🚩 **The residual trap is the middle figure: `27 / 31 / 35` is the rendered content box AND the
natural border box.** Naming the box does not disambiguate that one; whether the floor is live has to
be named too, which both documents now do.

🚩 **The guard was fired, and its first firing was itself a false null — which is the finding this
round nearly missed.** Forcing `--row-h: 100px` **on the list** moves every one-line row to border
box 100 px / content box 99 px, both forks, all three densities. Forcing it on `<html>` moves
**nothing** and the table still computes `--row-h: 28px`. That reads as a clean null and proves
nothing: `List.svelte` stamps `data-density` on the list container and `app.css`'s density blocks
match a bare `[data-density]` as well as `:root`, so the container **re-declares the token one level
below the override**. 🔗 That shadowing is ADR-0029's mitigation 2 working as designed — list-scoping
the density attribute is what takes a 5,000-row density toggle from 107 ms to 80 ms. A probe that has
not noticed it is overriding a token the list re-declares is measuring the wrong cascade.

⚠️ **Two claims in this change's own first draft were measured and found false**, recorded because
both were plausible and neither would have fired a gate:

1. *"The rich row's tallest cell is the actions cell's 32 px `<select>`, which is why the fix passes
   it by."* **False.** At compact the rich row's tallest cells are the numeric second-line cells at
   `<td>` border box **44 px** — the very cells the stray margin was landing on — and the actions cell
   is 32 px. The real mechanism: in a rich row the `.stacksep` is followed by a bare **text node**, so
   `.tbl td > * + *` never matched there on either tree. In the one-line row the same cell renders
   `span.stacksep` + `span.trunc`, and that `.trunc` measures `margin-top: 2px` pre-fix and `0px`
   post-fix — the only cell of seven that differed, taking its `<td>` to 28 px against 26 px for the
   rest. **The margin never reached the rich row; the rich row did not absorb it.**
2. *"The same three digits, 28 / 32 / 36, **are** true of the shipped component's one-line row as its
   content box."* True when written, false after `440e92d`, and left standing in the draft's own
   present tense — the exact defect the entry is about, surviving one revision of the paragraph that
   diagnoses it.

**Applied**: `docs/design/DESIGN-DIRECTION.md` §7.4 and `docs/DECISIONS.md` ADR-0029 now carry the
post-fix measurement with the pre-fix figures kept beside it as a labelled control rather than as
history, since the fix is only legible against them. Every figure in both names its box; the ones a
box alone cannot disambiguate name the floor condition too. ADR-0029's later bullet — *"A
`stack: 'labels'` list has one-line rows at 26 / 30 / 34 px"* — keeps its digits and gains its box.

🚩 **THE LESSON HAS A THIRD CLAUSE, AND THE RICH ROW IS WHERE IT WAS EARNED.** Name the box; name the
floor condition where a box alone cannot disambiguate; **and say whether the figure is a MODE or a
MEAN, whenever the rows are not uniform.** A single number describing a bimodal population is the
same defect as an unlabelled box, and it is worse in one way: an unlabelled box reads as a fact about
every row and is a fact about one of two boxes, while an unlabelled mean reads as a fact about every
row and is a fact about **none of them** — no rich row is 45.4 px tall. The rich row's
`45 / 49 / 53` slipped past every review this project has run because the two statistics collide on
one set of digits, and the failure it sets up is a *correction*: apply the `ROW_INTRINSIC` pattern —
border box, therefore subtract one, therefore 44 / 48 / 52 — to `RELEASE_ROW_INTRINSIC` and you have
replaced a correct mean with a mode, reintroducing this entry's own bug in the opposite direction.
Applied to `docs/design/DESIGN-DIRECTION.md` §7.4 (both citation sites), `docs/DECISIONS.md`
ADR-0029 and `docs/ARCHITECTURE.md` §4.5; `web/src/lib/list.ts` already carries it at the call site.

**Not fixed, reported, because `web/` belongs to another thread.** `ROW_INTRINSIC` in
`web/src/lib/list.ts` holds `28 / 32 / 36` and `List.svelte` writes it to `--row-ci`, which is
`contain-intrinsic-size` and takes a **content-box** height. The measured post-fix content box is
`27 / 31 / 35`, so the placeholder is **1 px over per row** at every density. Its own comment states
the equality it was derived from — *"a one-line row's content box comes out at EXACTLY `--row-h` —
28 / 32 / 36"* — which was true of the `two-line` fork before the fix and is true of neither fork now.
`auto` replaces the estimate with the row's real size after first paint, so nothing visibly breaks and
no gate fires, **which is exactly why it would sit there**. `list.test.ts` pins the constant with
`expect(ROW_INTRINSIC).toEqual({ compact: 28, standard: 32, relaxed: 36 })`, so a correction moves the
test with it. `RECENT_GRAB_ROW_INTRINSIC` in `web/src/lib/requests.ts` is `44 / 48 / 52` and needs no
change — that matches the measured rich-row **modal** content box, and a recent-grab row is the
two-line shape rather than the release row's chip-carrying one. ⚠️ **Do not read that sentence as a
licence to move `RELEASE_ROW_INTRINSIC` to the same digits.** It holds `45 / 49 / 53`, which is the
release row's **mean** content box over a bimodal population, and the mean is the right statistic for
a whole-list placeholder. See the third clause of the lesson above.

**Gate**: `node docs/design/check.mjs` passes on the merged tree at `eb78308` (all checks, both
installs). Measurements were taken in throwaway worktrees off `origin/main` and `3ae0d44^`, not in
the shared checkout.

---

# The poster grids — a migration that claimed completeness, and the gate that could not have known

**Date:** 2026-08-16. **Branch:** `claude/hearth-thread-vn9w7u`, merged to `main` as `cfc45c3`.
**Prefix `PG-` has not been used before.** Three findings and one operational note; `PG-02` is the
one that explains the other two.

| # | Finding | Resolution |
|---|---|---|
| **PG-01** | **`b417a53` said it had migrated the last poster title out of the art box, and it had not.** Its message claimed §9.7's *"the title and year sit BELOW the tile"* was satisfied and that the mockup *"was the last thing still doing it the old way"*. `docs/design/mockups/search.html` still nested both spans **inside** `.card__art` on all ten of its poster cards — `<span class="card__art" title="Dune"><span class="card__t" …>Dune</span><span class="card__y">1965</span></span>` — which is text over the `dominant_color` fill, the exact construction §9.2 and §9.7 ban and the exact one the deleted runtime contrast solver existed to survive. `b417a53` touched `index.html` and never opened `search.html` | **Fixed in `c469bca`.** Both spans now sit after the art, byte-for-byte Home's shape. ✅ **Measured over both Search poster panels rather than asserted**, at 1440×900 with `document.fonts.ready` awaited and scroll normalised, in both installs — 6 ebook cards on the full stack, 4 movie cards on v0.1, **all ten moved**: title top against the art's bottom edge **−168.19px → 0.00px**, title inside `.card__art` **true → false**, title width **110.80 → 112.80px** (the art's padded box → the full card), card height **191.19 → 229.19px**, panel height **286.22 → 324.22** and **270.19 → 308.19px**. ⚠️ **The height is the figure that needed care, and it is a trap this screen sets.** Search's cards read 191.19px uniformly both before and after `a5c9399`'s clamp because `aspect-ratio` pins them — so on this screen *an unchanged height is what a clamp looks like and also what doing nothing looks like*. The load-bearing measurement is the title's position against the art box, not its height. The +38px the cards gain is title 22 + year 16 leaving the art box, the same +38 `b417a53` measured on Home. 🚩 **The lesson is one this repo has now paid for twice: a rename or a migration that compiles, renders and passes its gate is not evidence that it was complete. Count the sites before and after.** A `<span class="card__art">` with any child now returns **0 across 42 cards**, where it returned **10** |
| **PG-02** | 🚩 **THE REAL FINDING. `docs/design/check.mjs` had never clicked a panel switcher, so neither poster grid had ever been rendered while the gate ran.** Three screens hide content behind a `[data-group][data-panel]` control — `homeview` = table\|**posters**, `viewmode` = table\|**posters**, `settings` = services\|**general**\|**tags**\|**ui** — and whatever loaded hidden stayed hidden for the whole run. **Five blocks, outside every rendered sweep for this file's entire existence**: the overflow sweep, the row-height sweep, the availability sweep, the roving-tabindex sweep and the §13 copy-and-attribute corpus. The poster grids are the only card surface in the product and the only surface that ever set text on a computed fill; Search's are `data-inst`-scoped on top of being panel-scoped. This is why PG-01 passed the gate | **Fixed in `1b6f598`, and the evidence was re-run rather than taken on report.** `a5c9399` added **84** `title=` attributes to poster cards (123 → 207 across the five screen files, counted directly). The §13 corpus counts `title` strings. Running **each tree's own unmodified `check.mjs` against its own mockups** gives **74 on `9281a1a` (`a5c9399^`) and 74 on `a5c9399`** — a corpus that cannot see 84 new instances of exactly the attribute it counts is not measuring the page. ⚠️ **And the old comment on that floor explained the small number away as the `<td>` data exclusion working as designed.** It was not. It was two whole panels nothing had ever opened, and a plausible explanation attached to a wrong number is how it survived. `panelsOf` / `setPanels` / `resetPanels` land, and **all five rendered sweeps go through them** — a gate that covered the panels for some sweeps and not others would be worse than the honest gap, because nobody afterwards could tell which was which. New **check 8c** asserts the traversal's one assumption instead of commenting it (on a freshly loaded page the first control in DOM order must be the one marked current), and prints the inventory. **Check 7 also stopped being tables-only**: its corpus was `table[role="table"]`, so the four `.grid[data-roving=".card"]` poster grids were outside the one-tab-stop check twice over — not a table, and inside a panel nothing opened |
| **PG-02a** | **The floors were all set against a page with two panels missing, so re-deriving them is part of the fix rather than a consequence of it** | **Every floor restated from what the traversal finds.** overflow combinations/width **74 → 110** (floor 70 → 104) · rows home **205 → 274** (180 → 240) · rows services **65 → 221** (55 → 190) · libraries, search, requests unmoved · `.avail` elements **195 → 375** (165 → 320) · roving list renderings **90 → 140** (78 → 120), of which **non-table 0 → 12** on a new separate floor of 10 · §13 copy strings **4203 → 6685** (2000 → 5800) · corpus `aria-label` **286 → 468**, **`title` 74 → 406**, `placeholder` **156 → 288**, `option` **1298 → 1602**. ⚠️ **Two of those are qualified rather than quoted.** The row counts are *row renderings*, and most of Home's and Services' rise is the same table measured once per panel pass rather than new markup — the poster panels hold no `<tr>` at all. That misreports nothing, since the check asserts a min, a median and a max against a band and a duplicate moves none of them; the floors move anyway, because a floor of 180 over a population of 274 is a floor that cannot fail. And **8c's three floors are set AT today's figures rather than below them**, which is the one place in this file where that is right: they are an inventory, not a population, and every sweep is scoped by what the inventory finds |
| **PG-02b** | 🚩 **The guard was fired deliberately, four times, one per newly-covered sweep — and the first firing found a real gap, which is the entire reason for firing it** | **Overflow**: a 4000px span inside Search's poster grid → exit 1, *"390px full/search/live viewmode=posters: document scrolls sideways (4016)"*, at all five widths. **Copy**: a banned word in a poster card's `title=` and in the panel's note → exit 1, *'full/search/live viewmode=posters [title]: banned word "seamlessly" in "Dune, seamlessly!"'*. **Availability**: the `.sr` word emptied on one poster card → exit 1, *"2 of 375 .avail elements have an empty accessible name"*. **Roving**: `refreshRoving()` removed from `usarr.js`'s panel-switch handler — which is precisely the bug panel traversal exists to catch, a grid whose cards were assigned tabindex while the panel was hidden having no visible item to give the tab stop to → exit 1, *'roving: full/home/live homeview=posters "Recently added across all media types" (div.grid, 24 items) has 0 items at tabindex 0'*. **Every message names the panel**, which is what proves the traversal found them and not something else. All four reverted; tree green at exit 0. ℹ️ The contrast sweep is deliberately absent from this list: check 3 is token arithmetic over `tokens.css`, not a rendered walk, so panels never affected it and adding traversal there would be theatre |
| **PG-03** | ⚠️ **Found by the copy guard failing to fire, and reported rather than quietly retried.** The first attempt put a banned word in the poster panel's `<h2>Ebooks …</h2>` and `check.mjs` stayed green. `.section__head` is `display: flex`, so its `.section__count` child blockifies to `block`, and check 1b's rule — *the innermost element that lays out as a block with no blockish children* — therefore **skips every group heading in the mockups** and reads only the count span. **Every `.section__head h2` on every screen is outside the §13 copy sweep**, and has been since 1b was written | **NOT FIXED — reported for a follow-up pass.** It has nothing to do with panels: the same heading in the *table* panel is equally invisible, so this is a second, independent hole in the same check that the panel work merely walked into. It is left alone deliberately — `1b6f598` is already large, and folding an unrelated corpus fix into it would make both harder to review and would hide the fact that the guard's first firing was a null. 🚩 **The durable half is that a guard which fails to fire is data.** The obvious move was to pick a different string and move on; the finding was in the string that did not work |
| **PG-04** | ⚠️ **`golangci-lint cache clean` can silently clean the wrong cache.** Two binaries are on this box: `/usr/local/bin/golangci-lint` at **2.5.0** (dated 2025-09-21) and the gate's own `/root/go/bin/golangci-lint` at **2.12.2**, which is what `Makefile`'s `GOLANGCI_LINT := $(GOBIN_DIR)/golangci-lint` resolves to. A bare `golangci-lint` goes through `$PATH` to the **2.5.0** one and does nothing to the cache the gate reads — the frontend thread's first clean was a no-op, and only an absolute-path clean with the gate's binary produced a trustworthy run | **Confirmed here rather than relayed**: `which golangci-lint` → `/usr/local/bin/golangci-lint`, `--version` → 2.5.0; `/root/go/bin/golangci-lint --version` → 2.12.2. Cleaned and run by absolute path with the gate's binary; `make check: OK`. 🚩 **Recorded alongside PG-02 because it is the same defect one layer down, and today produced three of them: a mitigation that reports success while doing nothing.** A sweep that never opened a panel, a corpus that could not see 84 new attributes, and a cache clean that cleaned a cache nobody reads — each returned a green that named neither its tool nor its tree. `CLAUDE.md`'s *"report what you measured, not just the verdict — the binary, its version and the commit"* is the rule all three violate, and **naming the binary is not pedantry when two are installed** |

**Gate**: `node docs/design/check.mjs` passes on the merged tree at `cfc45c3` (exit 0, all checks,
both installs, every panel), and `make check: OK` — Go linting via `/root/go/bin/golangci-lint`
**2.12.2** after an absolute-path `cache clean`, per PG-04. The design check was also run on
`9281a1a` and `a5c9399` in throwaway trees extracted with `git archive`, not in the shared checkout,
to produce PG-02's 74/74.

---

# Consistency audit — the writer's transaction bought nothing, because the reader asked twice

**Date:** 2026-08-16. **Branch:** `main`. **Baseline:** `70a61c9` (the fix), audited forward from
`a3c79b2` (the tree before it). Not a review round, and no adversarial reviewer behind it: this
started as a **flake report** against `TestEndToEndIndexerCatalogue`, and the entry exists because
the flake was not one. Prefix `SNAP-` has not been used before, so nothing collides.

**The mechanism, in one paragraph.** `GET /api/v1/indexers` issued two statements off the **read
pool**: one for `service_instance.indexers_fetched_at`, then one for the `indexer_catalog` rows.
Two statements on the pool are **two WAL snapshots**. `store.ReplaceIndexers` writes the stamp and
the rows inside a single transaction precisely so that no reader can observe one without the other
— its own comment says so, and the comment is correct — but write-side atomicity buys the reader
nothing when the reader asks twice. A replication commit landing between the two reads gave the
first a NULL `indexers_fetched_at` and the second the freshly written rows:

```
{"status":"never_fetched","fetched_at":null,"indexer_count":3}
```

followed by the three indexers, each stamped with the fetch time the instance claims never
happened. The picker renders *"UsArr has not yet read the indexer list from Prowlarr"* beside a
**Test connection** button, on an install where nothing is wrong — arriving on the first paint
after a service is added, which is the one moment the replication write is guaranteed to be in
flight. It is exactly the failure §17 exists to prevent.

**This was a production bug, not a flaky test.** Worth stating plainly, because the report arrived
as a flake and the cheap disposition was `-count 1` and a shrug. The test was reading the API's real
response through the real handler; it failed because the response was genuinely wrong, roughly one
request in a hundred, for every user of that endpoint. The test's only unusual property was running
the write and the read concurrently on purpose — which is what a background prober and a browser do
by default. A test that fails intermittently on a race the product also has is not flaky; it is the
only thing in the repo telling the truth.

**Reproduction.** 48 failures in 4000 runs (1.2%) — eight parallel workers of `-test.count 500` on a
loaded four-core box — always as `TestEndToEndIndexerCatalogue`'s `fetched_at` assertion, never
anything else. After the fix the same loop is **4000/4000**, and 240 runs under `-race` are clean.

| # | Finding | Disposition |
|---|---|---|
| **SNAP-01** | **The catalogue read spanned two WAL snapshots, and rendered a state the database never held.** The invariant `rows present ⇒ the stamp is valid` is one-directional and always true of the *database*; it was not true of the *response* | **Applied at `70a61c9`.** `store.ReadIndexerCatalog` now pairs the instance read and the catalogue read inside `db.ReadTx`. The fix is in the **store, not the handler**, because the invariant belongs to the pair rather than to any one caller: whatever reads a replication stamp beside the rows it stamps has to read them together, and a second caller written next year would otherwise re-open the hole. The three read bodies (`getServiceInstance`, `listServiceInstances`, `listIndexers`) were reshaped to take a `querier`, so one body serves both the pool and a snapshot |
| **SNAP-02** | **`db.ReadTx` existed for exactly this and had no production caller at all.** Written with a correct doc comment — *"so a multi-statement read sees one consistent snapshot"* — and reached only from `internal/db`'s own test | **Closed by SNAP-01's fix, and named here because the shape generalises.** An unused helper is not evidence that nothing needed it; it is evidence that nobody looked. The lesson is recorded in `DEVELOPMENT.md` §11 so the next multi-statement read starts from the question rather than from the pool |
| **SNAP-03** | **The guard has to be watched failing, or it is decoration.** `TestReadIndexerCatalogIsOneSnapshot` asserts the one-directional invariant — indexers present implies `indexers_fetched_at` valid — while a writer replicates against four spinning readers. The converse is a legitimate state and is deliberately **not** asserted | **Applied, and fired deliberately twice** — once when written, once when this entry was compiled, against a `ReadIndexerCatalog` whose `db.ReadTx` was swapped back for `s.db.Read()` and nothing else changed. It fails in **round 0, in 0.03s**, on three of the four readers: *"the catalogue carries 2 indexers while the instance reports `indexers_fetched_at` NULL. ReplaceIndexers writes both in one transaction, so this state never existed in the database — the read spanned two snapshots."* Restored, it is green at `-count 5` |
| **SNAP-04** | **Is this shape anywhere else?** Audited every read path in the tree for it: two or more statements off the read pool whose results a caller or a user expects to agree | **Swept; nothing else is broken.** The sweep is below. `indexers_fetched_at` is the **only** cross-table stamp in the schema — the other two `fetched_at` columns (`release_candidate`, `indexer_catalog`) are per-row and are read in the same statement as their row — so the exposure was one endpoint wide, and it is closed. No second fix was made, and none was invented: an unnecessary read transaction is noise that hides the necessary ones |

## The sweep

Every read path in the tree, with the reason it is safe. Verdicts are **safe** (one statement, or
already in a transaction), **benign** (can disagree; nothing observable depends on it), or **broken**.

| Path | Verdict | Why |
|---|---|---|
| `GET /api/v1/indexers` → `store.ReadIndexerCatalog` | **fixed** | SNAP-01 |
| `GET /api/health/live`, `/ready`, `/api/v1/system/status` | safe | Touch no database at all. Readiness is migrations-applied plus listener-accepting, both recorded at construction, and `handleReady`'s comment already says "nothing more" means it does not touch SQLite |
| `GET /api/v1/services`, `/services/{id}` | safe | One statement each |
| `GET /api/v1/grabs/recent` → `store.ListRecentProvenance` | safe | One statement. `limit` is echoed from the value the server clamped to, not re-derived, so there is no count-versus-rows pair to tear |
| `store.ListAuditLog`, `store.VerifyAuditChain`, `store.GetProvenanceByDownloadID` | safe | One statement each. A single `QueryContext` holds one read transaction for the life of its cursor, so a streamed chain walk is one snapshot even at table scale |
| `GET /api/v1/services/health` | safe on the SQLite side | One statement (`ListServiceInstances`); everything else on the row comes from the prober's in-memory snapshot. Noted rather than fixed: `recordProbe` publishes to memory **before** it writes the health columns, so a row can carry a fresh `observed_at` beside the previous probe's `last_error` for the microseconds between. That is not a WAL-snapshot tear and `db.ReadTx` cannot address it — one side is a map — and it is the state the process genuinely held a moment earlier, self-correcting on the next tick. Left alone; if it ever needs closing, the repair is ordering, not a transaction |
| `resolveSession` → `GetSession` + `GetUser`, on every authenticated route | benign | Two statements, but the pair cannot disagree observably: `user` rows are never deleted and nothing updates `is_disabled` in v0.1, so the second read cannot fail or change. The ordering is also the fail-closed one — the session is read first and the user second, so a future disable is *more* likely to be caught, not less. A revocation landing between the two is a check-then-act latency every design has, not a torn read: one snapshot would not shrink the window, it would only move it |
| `GET /api/v1/auth/session` → `Owner()` + `resolveSession` | benign | The closest thing in the tree to the original shape — a status field (`setup_required`) beside the data it describes (`authenticated`) — and the invariant `authenticated ⇒ an owner exists` is real and one-directional. It cannot be violated, **by causality rather than by luck**: the session cookie is only ever issued by `startSession`, which runs after `CreateUser` has committed, so any request carrying a valid cookie is causally later than the owner row, and a fresh statement on the read pool takes the latest committed snapshot. Recorded because the argument, not the code, is what makes it safe — a future path that mints a session by any other route re-opens it |
| `GET /api/v1/search` → `resolveIndexerInstance` + `registry.entry` | benign | Two reads of the same `service_instance` row, one snapshot apart. `entry` re-checks `Enabled` and `Role` itself and is authoritative and fail-closed; the handler's earlier read exists to *pick* the instance and to word the error. Worst case is a slightly different error message, and the network call that follows is outside any snapshot regardless — `db.ReadTx`'s own note forbids holding one across it |
| `POST /api/v1/releases/{id}/grab` → `GetReleaseCandidate` + `releases.Grab`'s `Candidate` | benign | Two reads of the same row, and the row is immutable: `release_candidate` has `INSERT` and a TTL `DELETE` and **no `UPDATE` anywhere in the tree** (verified against every mutating statement in `internal/store` and against the migrations, which add no triggers beyond `audit_log`'s two). No field can change between the reads; the only divergence is presence, reported as an honest expiry error that names the fix |
| `PATCH`/`DELETE /api/v1/services/{id}`, `POST /services/{id}/test` | benign, and out of shape | Read-then-write, not read-then-read. The window spans a network call, so no read transaction can cover it; the store's `UPDATE … WHERE id = ? AND deleted_at IS NULL` plus `expectOneRow` is what makes the write itself safe. The re-read at the end of `handleUpdateService` deliberately wants the *newest* snapshot, which is the opposite of pairing |
| `store.RedactStoredProvenanceURLs` | benign, by design | A keyset-paged scan-and-rewrite over many snapshots on purpose. It is idempotent and converges, and holding one snapshot across the whole pass is precisely the long-lived read transaction that starves the WAL checkpointer |
| `registry.probeAll` → `probe` → `entry` | benign | Background loop with no user-facing pair; a row deleted mid-sweep fails the per-instance read and is skipped, and the next tick re-reads everything |
| Startup `CountEncryptedCredentials` | safe | One statement, and the only reader of it fails closed on the answer |

**Result:** one genuine instance of the shape, found from a flake report, fixed at `70a61c9` in the
store and guarded by a test watched failing. **Nothing else in the tree has it.** The negative result
is the deliverable for the other thirteen paths, and each one's reason is recorded above so the next
sweep argues with the reasoning rather than repeating the search.

---

# INST-01 — a second indexer service is configurable, is never searched, and nothing says so

**Origin: the Requests thread, building the indexer picker.** It reported a narrow, correct thing —
*the indexer id in the catalogue is the indexer service's own id, so two configured indexer services
can each carry an id 3, and a picker keyed on that id alone reconciles two different rows onto one.*
It fixed the render key (`` `${indexer.instanceId}:${indexer.indexerId}` `` at
`web/src/routes/requests/+page.svelte:968`) and flagged the ambiguity upward.

**The investigation found something wider, and the wider thing is the finding.** Chasing the
collision into the search path turned up a configuration that UsArr *accepts today* and then
*silently under-serves*. **Open. Not applied, not deferred — owners are named at the bottom.**

## INST-01.1 — the framing, because it is the part that matters

🚩 **The id collision is a consequence, not the defect.** It is one symptom of the real shape:

> **Instance selection is implicit on a screen that presents a multi-instance catalogue, and it
> degrades silently.**

The picker enumerates every indexer of every configured indexer service and says so out loud —
`"%d indexers across %s"` at `internal/httpapi/indexers.go:310` renders *"6 indexers across 2 indexer
services"*. The search that the same screen launches asks **one** of those two services. Nothing in
the request, the response, or the screen names which. **Principle 3 forbids exactly this**: *"every
feature degrades honestly when a service is absent — it says what is missing and why, rather than
rendering an empty screen that looks broken."* Here the degradation is worse than an empty screen,
because a screen that renders half an answer under a heading that promises the whole one does not
look broken at all.

📌 **This is a live gap in a supported configuration, not future work.** Nothing in the product
forbids the second service, nothing warns about it, and nothing in `docs/ARCHITECTURE.md` §16 makes
one indexer service a documented constraint. It is reachable by a user following the Services screen
as designed. Filing it as "multi-Prowlarr, later" would be reclassifying a defect as a feature.

## INST-01.2 — what was measured

Executed, not inferred. Two fake Prowlarrs, both registered through the real API, one search run
through the real handler.

| Claim | How it was established | Result |
|---|---|---|
| Two indexer services can be configured | Two `POST /api/v1/services` calls | **Both `201`** |
| Nothing but the name stands in the way | `service_instance` DDL, `internal/db/migrations/00001_initial.sql:136` | **`name TEXT NOT NULL UNIQUE` is the table's ONLY uniqueness.** No constraint on `kind`, none on `role`, none on `base_url` |
| Only one service is searched | Per-instance request counters on the two fakes, one UsArr search | **`P1 searchCounts=map[1:1 2:1]`, `P2 searchCounts=map[]`** — every indexer of the first service was asked once, the second service received **zero requests** |
| The choice is `candidates[0]` | `resolveIndexerInstance`, `internal/httpapi/search.go:364-416` | Filters `role == "indexer" && Enabled`, then returns `candidates[0]` from a list `ListServiceInstances` orders **by priority then name**. The comment calls it *"the user's own stated preference rather than an arbitrary pick"* — which is true of the **ordering** and says nothing about the **silence** |
| Grab is unaffected | `handleGrab` | Resolves the searcher from `cand.ServiceInstanceID`, so a grab always goes back to the service that produced the candidate. **No cross-instance mis-grab exists.** This is the one part of the path that carries the instance end to end |

## INST-01.3 — the exact user-visible failure

In order, on a two-Prowlarr install:

1. The picker's summary reads **"6 indexers across 2 indexer services"** — the user is told, by
   UsArr, that both services are in play.
2. The grid lists all six in **one flat list with no instance label**. Nothing on a row says which
   service it came from.
3. Selection is keyed on the **indexer id alone** — `scope.isSelected(indexer.indexerId)` and
   `toggleIndexer(indexer.indexerId)` at `+page.svelte:983-985`. Where two services share an indexer
   id, **ticking one row ticks its twin.** (The *render* key is already instance-scoped; the
   *selection* key is not, and they are different keys.)
4. Directly under that grid the copy reads **"Selecting none searches all of them."** It does not.
   It searches all of *one service's*.
5. The search returns the first service's results under the degraded report — `internal/httpapi/
   search.go:358`, *"%d of %d indexers answered — %d failed, %d skipped; these results are
   incomplete"*, e.g. **"2 of 3 indexers answered … these results are incomplete."** Every number in
   that sentence is counted **within the one service that was searched**. It therefore reads as a
   **partly-degraded search**, when what actually happened is that **an entire service was never
   asked**. The user is shown the wrong problem, with a count that invites them to go fix one
   indexer.
6. Where the ids **differ** between instances, the planner is honest — the un-matched selection
   surfaces as `OutcomeNotFound`. Where they **collide**, the user ticks one tracker and gets the
   other instance's.

🚩 **Step 5 is the worst step, and it is worse than an error would be.** An empty screen or a "which
service?" prompt is a question the user can answer. A confident, incomplete result under a report
that mis-describes its own incompleteness is a wrong answer that looks like a right one.

## INST-01.4 — the seam is already built, and nothing uses it

⚠️ **`?instance=` already exists, already works, and has no production caller.**
`resolveIndexerInstance` handles it fully: it parses the id, reads the instance in the caller's
scope, **rejects a non-indexer role** with a message naming the actual role, and **rejects a
disabled instance** with `409 CodeServiceDisabled` and the action *"Enable this service"* — with a
comment explaining that falling back to the auto-picked instance would *"silently answer a different
question from the one asked, which is worse than an error because the results look right."* That
comment is a precise description of INST-01, written by the author of the very branch that avoids
it. It is tested in `internal/httpapi/search_resolve_test.go`.

`SearchScope.instanceId` exists on the client too, at `web/src/lib/api.ts:1258`, and is serialised
into `params.set('instance', …)` at line 1275-1276. **No production caller sets it.** The label is
already there as well: the catalogue row carries `instance_name`
(`internal/httpapi/indexers.go:97`) and the client already parses it into `instanceName`
(`web/src/lib/api.ts:1551`) — it is simply never rendered.

So this is not a missing capability. It is a wired socket with nothing plugged into it — the same
shape as **SNAP-02** (`db.ReadTx` written for exactly its purpose and reached only from a test), and
the same lesson applies: *an unused helper is not evidence that nothing needed it; it is evidence
that nobody looked.*

## INST-01.5 — the three candidate fixes, recorded as options rather than chosen

**Deliberately not decided here.** Two threads own the two halves and picking one for them is how a
fix lands twice or not at all.

| Option | What it does | Owner |
|---|---|---|
| **(a)** | The frontend labels each picker row with `instanceName`, keys **selection** on `(instanceId, indexerId)` as the render key already is, and passes `instanceId` on the search. Uses the seam in §INST-01.4 that already exists — `instance_name` is already on the wire and already parsed. **Needs no Go, and no new API field.** | **The Requests thread** |
| **(b)** | The handler states in the search report when it **auto-picked from more than one candidate**, and names the instance it chose and the ones it did not — so any UI, present or future, can say which service was searched and which was not. Closes the honesty half at the source, for every client including ones that never send `?instance=`. | **This thread** |
| **(c)** | Both. (a) and (b) are not alternatives — (a) makes the choice explicit, (b) makes an *implicit* choice audible. A client that never adopts (a) still needs (b); a report that says "auto-picked" still leaves the user no way to pick. | Both threads |

📌 **(a) alone closes the user-visible half without a line of Go**, which is the fact worth carrying
out of this entry: the expensive-looking finding has a cheap first move, and taking it does not
foreclose (b).

**Not proposed, and named so it is not re-proposed:** fanning one search out across every configured
indexer service. That is a real feature with a real cost — merged reports, per-instance partial
failure, duplicate releases across two Prowlarrs pointed at the same tracker — and §16 funds none of
it. **This finding does not ask for multi-instance search. It asks for the single-instance search
UsArr already does to stop presenting itself as something else.**

## INST-01.6 — the two orderings were relayed as disagreeing; one was read backwards, and the real divergence is a different one

**Relayed from the Requests thread as a measured observation:** that
`store.ReadIndexerCatalog` orders instances **name ASC** while `resolveIndexerInstance` picks
`candidates[0]` from **priority, then name**, and that on a live two-service install the catalogue
listed *"Prowlarr Private"* (priority 20) first while an unqualified search would have gone to
*"Prowlarr Public"* (priority 10). The claim is checked in, as the warning block above
`indexerServices` at `web/src/lib/indexercatalog.ts:144-149`.

⚠️ **Re-read against the source before recording, because an entry whose whole point is that two
orders disagree is worth nothing if either is backwards. One is.** Both, in the form the code
actually has:

| Path | Where the **instance** order is actually decided | The order |
|---|---|---|
| Catalogue | `ReadIndexerCatalog` (`internal/store/indexers.go:245`) does not order instances at all — it delegates to `listServiceInstances` (`internal/store/serviceinstance.go:190`) | `ORDER BY priority DESC, name ASC` |
| Search | `resolveIndexerInstance` (`internal/httpapi/search.go:371`) filters `ListServiceInstances` — the exported wrapper over that same `listServiceInstances` — to `role == "indexer" && Enabled`, then takes `candidates[0]` | `ORDER BY priority DESC, name ASC` |

**They are the same statement.** There is exactly one `ORDER BY` over `service_instance` in the tree
(`serviceinstance.go:195`), both paths read through it, and `internal/db/queryplan_test.go:268` pins
that plan. The relayed claim's own conclusion — *"the first row a user sees is not necessarily the
service their unqualified search hits"* — is nonetheless correct; see the ✅ below.

🚩 **The `name ASC` in the claim is a different query.** `store/indexers.go` does carry
`ORDER BY name ASC`, in `indexerListSQL` (`internal/store/indexers.go:138`) — which orders **the
indexers inside one instance**, not the instances. `ReadIndexerCatalog` calls it once per instance,
inside its loop over the already-ordered instance list. A file-level grep for the ORDER BY finds it;
following the call does not.

🚩 **The live example is backwards too, and for a reason worth naming.** `service_instance.priority`
is **highest-wins** (`docs/DECISIONS.md:1340`, *"highest `priority` among healthy instances"*) and
the sort is `DESC`, so priority 20 sorts ahead of priority 10 on **both** paths: the search would
have hit *"Prowlarr Private"* — the service the catalogue listed first — not *"Prowlarr Public"*.
The mis-read is an easy one to make on precisely this screen, because Prowlarr's **own** indexer
priority is smaller-is-better, which `indexercatalog.ts:310-318` documents in full a hundred and
sixty lines below the block that got it wrong. **Two priority fields, opposite directions, one
screen** — that is the hazard here, not the sort key.

✅ **A real divergence does survive, and it is the enabled filter rather than the ordering.** The
catalogue handler keeps a **disabled** indexer service as a row (`internal/httpapi/indexers.go:204`
— *"the row still appears, with the reason and the one action that changes it"*), while
`resolveIndexerInstance` drops it from `candidates` entirely. On an install whose highest-priority
indexer service is switched off, **the first row of the catalogue is still not the service an
unqualified search hits.** The Requests thread's conclusion holds, by a mechanism it did not
observe.

📌 **This lands on option (b), which is why it is recorded here rather than as its own finding.**
(b) has the handler state which instance it auto-picked, and that statement is read against a list:

- **The sort key needs no reconciling.** It is one key, already shared by both paths.
- **The membership does.** A report that names a **position** — *"the first indexer service"* — is
  wrong the moment a disabled service occupies row 1 of the list the user is looking at.
- ⚠️ **So (b) must name the service, never its position:** its `instance_name`, the string already
  on the wire and already parsed per §INST-01.4. Whoever implements (b), that is the constraint this
  sub-section exists to carry.

**Neither ordering is changed here.** Whether the catalogue *should* list a disabled service above
enabled ones is a design question, and this entry does not take it. The stale warning block at
`indexercatalog.ts:144-149` is left alone for the same reason it is cited: correcting another
thread's freshly landed comment mid-flight is how two threads write the same line twice. **Follow-up
for the Requests thread: that comment asserts a `name ASC` catalogue order that the store does not
have, and should be restated as the enabled-filter divergence above.**

---

# The second gate hole, and the unit that had never actually been split

**Date:** 2026-08-17. **Branch:** `claude/hearth-thread-vn9w7u`, merged to `main` as `d89680c`.
**Prefix `SU-` has not been used before.** `SU-01` closes `PG-03`, which the poster-grid pass logged
and deliberately left unfixed; `SU-02` is the answer to the question that finding raised and is the
one worth reading.

| # | Finding | Resolution |
|---|---|---|
| **SU-01** | **`PG-03` closed.** Check 1b's unit was *"the innermost element that lays out as a block with no blockish children"*, and its second test — `if ([...el.children].some((c) => BLOCKISH.test(getComputedStyle(c).display))) return;` — **threw the whole element away rather than the child**, taking the element's own direct text with it. CSS hands out blockish children without being asked: every child of a flex or grid container is blockified whatever its own `display` says, so `<h2>Ebooks <span class="section__count num">14</span></h2>` under `display: flex` failed the test and contributed nothing, while the count span passed it alone and contributed `"14"` | **Fixed in `1fcd188`. THE UNIT IS NOW A RUN OF INLINE CONTENT INSIDE ONE BLOCK BOX**, which is what the browser itself lays out: walk the text nodes in document order, attribute each to its nearest blockish ancestor, cut the string where that ancestor changes. ✅ The old comment's reason for the unit's shape survives intact — an inline `<a>` or `<code>` is not blockish, so it still does not cut a sentence into fragments — and **no element's own text can be lost any more, because every text node is attributed to something**. A blockified child *does* cut the run, and should: `.detail li` is `display: flex` with a `gap`, so `Recomputed <span>13:40</span>, 28 minutes ago` is three gapped boxes on the screen rather than one sentence. That makes the em-dash-under-15-words rule **stricter, not laxer** — a five-word box carrying an em dash is exactly what §13 bans — which is the safe direction for a heuristic to move in |
| **SU-01a** | **The guard was fired deliberately, in both panels, because a sweep that now reaches the headings but would not fail on them is the same defect one layer along** | **Two firings, both exit 1.** Table panel: *`full/search/live viewmode=table: banned word "seamlessly" in "Ebooks seamlessly"`*. Poster panel: *`full/search/live viewmode=posters: banned word "seamlessly" in "Ebooks seamlessly"`*. 🚩 **The poster one is PG-03's original null** — the string that failed to fire is what found the bug, and firing the same string again is what proves it fixed. **The table firing is the load-bearing one**, because it is what shows this was never a panel problem: the identical heading in the default, always-visible panel was equally invisible, which is what PG-03 claimed and had not demonstrated. Both reverted; tree green at exit 0 |
| **SU-01b** | **One floor moves, and it is re-derived from what the sweep finds rather than rounded down from it** | **`STRING_FLOOR` 5800 → 6750**, against **6685 → 6978** strings scanned. **The margin is arithmetic, not taste:** restoring the old element unit costs **293** strings, so a floor with more than 293 of slack sits green through exactly the regression it was moved for. 6978 − 6750 = **228**, which is under that and still leaves room for a dozen paragraphs of ordinary editing, since one authored string is counted once per combo it renders in. The old 5800 had **885** of slack — enough to lose every group heading twice over — which is PG-02a's *"floor that exists only to be quoted"* in its own file. ✅ **Fired**: with the walk reverted and the floor left at 6750, *"§13 copy — scanned only 6685 user-visible string(s), below the floor of 6750."* **No corpus floor moves**; the attribute sweep is untouched (`aria-label` 468, `title` 406, `placeholder` 288, `option` 1602, `document.title` 1) |
| **SU-02** | 🚩 **THE QUESTION PG-03 RAISED: if `display: flex` on one container was enough to hide a heading, what else was hidden? Answer: fifty-eight distinct strings, and the group headings were the smallest part of it.** The hole was never specific to `.section__head` — it fires on **any** blockish element that acquires **any** blockish child, and flex and grid containers create those by default | **Surveyed rather than guessed, by running the old rule and the new one side by side over every screen × state × panel × install of `prototype.html` and diffing the string sets.** **58 distinct strings enter §13 for the first time**, by owning element: `.pagehead__meta` **18** (every screen's one-line summary — *"31 results in 6 of 6 media types."*, *"Last delta sync 14:02, 6 minutes ago. 4 items need attention."*), `<li>` **8**, `.card__meta` **6** (the media-type word on every poster card), `<a>` **5** (the group-jump links), `.avail` **4** (*"10/10"*, *"23 of 23 tracks"*), `.banner__text` **4**, `.check` labels **3**, `<h2>` **3**, `.field__err` **2**, `.testresult__title` **2**, `.fanout` **1**, `.note` **1**. ⚠️ **The group headings were 3 of 58.** PG-03 named the symptom it happened to trip over; the class of defect was an order of magnitude larger, and **it is one fix, not twelve** — which is why it was worth finding the general rule instead of special-casing `h2` |
| **SU-02a** | **A correctness win that fell out rather than being aimed at, and it is a deletion** | `textContent` **reads through `[hidden]` descendants**, so the old element walk welded both `[data-inst]` variants of a summary into strings no user has ever seen. *"No results for duen in 82 libraries."* was the **8** of the full stack and the **2** of v0.1 run together; *"31 results, all hidden by the filter.5 results, all hidden by the filter."* was one span read twice. **12 such fabrications are gone**, because a text walk tests each node's own ancestry instead of its ancestor's. 🚩 They were worse than noise: a banned word in a *hidden* variant would have been reported against a string that does not exist, and the em-dash word count was being run on nonsense |
| **SU-03** | **§9.1 has said since it was written that *"the figure is a right-aligned numeric span, the unit is a fixed-width left-aligned span beside it"*. Nothing rendered it that way.** The owner asked for the split in principle and was offered two options; the default was taken and made cheap to reverse | **Shipped in `d9f1b39` for the size columns — 98 cells — and recorded in §9.1 as the rule.** Figure at full contrast, unit `--fg-muted`, `tabular-nums` left **on the cell** so composite values still inherit it, and the unit box given a **fixed width**. ✅ **The fixed width is the half that does the work, and it was measured rather than asserted, because no column in the sample data mixes units — so nothing on the screen demonstrates the rule and *"it looks fine"* proves nothing.** Rows of the Requests release table moved onto the widest unit in the family, one narrower, and the narrowest: **with the fixed unit box the last digit of `68.4 GiB`, of `820 MiB` and of `4 B` all sits at x = 924.06px, spread 0.00px; with the box switched off, 928.06, 926.06 and 940.06 — spread 14.00px.** Widths belong to the unit **family**, not the value present, or the digits jump the first time a row crosses GiB to MiB; measured in the cell font (13px IBM Plex Sans, `1ch` = 8px) and rounded up to the next half-ch, `MiB` at 22px is the widest size unit, so `.unit--size` is `3ch`. Reversal is deleting one CSS block: `.unit` is a bare span, so the markup falls back to the unit inline at full contrast. ⚠️ **CORRECTED — this row first shipped `2.5ch` and a `9.00px` spread, and both were measured against the wrong unit family.** The reserve was derived from the DECIMAL family `B · KB · MB · GB · TB` that the mockups draw, and the spread from a two-row `68.4 GB` / `4 B` sample that did not contain the widest unit. See **SU-05** for the correction and for the defect underneath it |
| **SU-03a** | **The alternative is rejected in writing, in both §9.1 and the stylesheet, so it is not re-proposed** | **Units in the column header, bare numbers in the cells.** It only works where every row of a column shares one unit, **which rules out size** — a `Size` column holding `68.4 GB` beside `820 MB` beside `4.2 KB` has no unit to put in its header, and the moment one exists the header is lying about some rows. The same disqualifies it for `Age` (`3 years` beside `11 months`) and for Home's `Items`, whose six rows count films, series, artists and books. **A rule that fails on the three columns it was proposed for is not a rule.** ℹ️ The header form stays right where a column's unit is genuinely constant — and that column is already covered, by §9.1's own *"a value identical for every row is not data and is not rendered"* |
| **SU-03b** | ⚠️ **The scope is smaller than intended, and the reason is a measured cost rather than an unfinished opinion. Named here and in §9.1 so a half-applied convention cannot read as a decision** | **A reserved unit box costs column width, and two declared tracks cannot pay it.** `Age` in the release tables is a **68px** track: 24px of padding leaves 44px and `months` alone is **43px**, so the figure has nothing left. Home's `Items` is **107.375px**: 52px of reserve for `episodes` plus 24px of padding leaves 31px against a **36px** `1,842`. Both were **measured wrapping at +18px per cell over 224 cells** across the render sweep, and both are reverted. **Still carrying the old one-string treatment:** `Age` on *Release results* and *Audiobook release results* (**36 cells**), `Items` on Home's *Library by media type* (**16 cells**). Widening a declared track is its own decision on the two densest tables in the product, and §9.1's own overflow bullet is why it is not folded in casually. ℹ️ The 12 `Aired` cells (`17 Nov`, `1 Dec`) are out of scope by rule, not by omission: a month name is a **date** under §9.1's timestamp rule, not a unit |
| **SU-04** | **The adjacent-sibling trap was checked rather than reasoned about, and the reasoning that would have skipped the check was wrong** | A new `<span class="unit">` is a **second element child** of the `<td>`, so it meets `.tbl td > * + *` at **(0,1,1)** with `margin-top: var(--sp-1)` and `.tbl td > .stacklabel + *` at **(0,2,0)** with `margin-top: 0` — the same precedence pair that produced the `.stacksep` bug in SW-27. ⚠️ **The tempting dismissal was *"vertical margins do not apply to inline boxes, so it is inert"*, and it is wrong here: `.unit` is `inline-block`, which DOES take a vertical margin.** What makes it inert is the (0,2,0) rule winning, and that only holds while every such cell is exactly `(stacklabel, unit)` — **confirmed across all 150 candidates**, not assumed. ✅ **Verified by a full before/after render capture** — 5 screens × 2 viewports × 2 themes × 2 installs × every state × every panel, **1040 combos**: **element rects compared 1,287,904, differing 0**; list geometries 1,392, differing 0; overflow verdicts, `document.scrollWidth` and z-index chains all differing 0; **8,832 `.unit` spans rendered across the sweep**, so the zero is a zero over a population rather than over an empty set. 🚩 **The first capture was NOT zero** — it caught SU-03b's wrapping at +18px per cell, which is the entire reason the cycle was run and the reason the scope is 98 cells and not 150 |

**Gate**: `node docs/design/check.mjs` — **exit 0, all checks, both installs, every panel** — on the
merged tree at `d89680c`; §13 copy corpus **6978** strings (unchanged by the unit split, because the
new spans sit inside a `<td>` and a `<td>` is data). `make check: OK`, with Go linting via
**`/root/go/bin/golangci-lint` 2.12.2** after an absolute-path `cache clean` — `PATH` resolves
`golangci-lint` to `/usr/local/bin/golangci-lint` at **2.5.0**, which is not the gate's, per PG-04.
Rendered measurements used Playwright/Chromium with `PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers`,
`document.fonts.ready` awaited, scroll normalised and `content-visibility` forced visible, per the
three traps in `check.mjs`'s own header — the alignment probe reported two rows at zero on its first
run and needed the second and third of them before the numbers meant anything.

**Needs mirroring into `web/`, routed rather than done here** (`web/src/lib/format.ts` is the
frontend thread's file and was not touched): the formatter currently returns a figure and its unit as
one string, and §9.1 now requires two slots. It needs to return them separately — or return a
`{ value, unit }` pair — so the Svelte table can render `<span class="unit unit--size">`. The CSS
class, the `3ch` reserve and the `--fg-muted` treatment are the contract; the column scope is size
only, per SU-03b. ⚠️ **This paragraph said `2.5ch` when it was written; the reserve is `3ch`, per
SU-05.**


---

# A reserve measured against the wrong unit family, and the mockup that produced the wrong number

**Date:** 2026-08-17. **Branch:** `claude/hearth-thread-vn9w7u`. **Follows `SU-03`/`SU-04` above**,
which shipped the figure/unit split and are corrected in place by this round. **`SU-05` is the one
worth reading** — `SU-06` is the arithmetic, `SU-05` is why the arithmetic was wrong.

| # | Finding | Resolution |
|---|---|---|
| **SU-05** | 🚩 **THE ROOT CAUSE, AND IT IS NOT ABOUT UNITS. The design mockups print DECIMAL size units — `KB · MB · GB · TB` — and the application prints BINARY ones — `KiB · MiB · GiB · TiB`.** Every geometry measurement taken over these pages is therefore a measurement of a string the product cannot emit. `SU-03` reserved `2.5ch` for the unit box; it was measured correctly, in the right font, with the right method, against `MB` at 19px — and it was wrong, because `MB` is not a unit UsArr prints. **The lesson: a mockup is a measuring instrument, so sample data that does not match what the product emits is a CALIBRATION error, not a cosmetic one.** A wrong colour in a mockup misleads a reader once; wrong sample data keeps generating numbers that are correct about the mockup and wrong about the product, indefinitely, and each one looks rigorous | **Surveyed rather than spot-fixed, and REPORTED rather than changed, because switching the sample data moves rendered widths in three declared tracks and needs its own capture cycle.** Scope, counted over all six mockup files: **154 rendered decimal size values** — the **98** `.unit--size` split-slot cells of `SU-03` (Search 31 + Requests 18, mirrored into `prototype.html`) plus **56** one-string composites in Home's `Detail` column (`Bluray-1080p · 14.2 GB`, `M4B · 6 h 58 m · 402 MB`) — from **77 authored** occurrences, plus **9** more in HTML comments. ⚠️ **And it is not only size. Two more columns print a shape the API does not produce:** `Age` — **36 cells** rendering `2 days`, `3 years`, `11 months`, where `formatAge` in the shipping app emits only `d` and `h` from `age_days`; and `Category` — **18 cells** rendering Newznab paths (`Movies/UHD`, `Audio/Audiobook`), where `categoryLabel` in the shipping app emits the derived type/format tags (`movie`, `book · audiobook`) or a comma-joined list of raw numeric ids. ℹ️ The 8 `n/a` peers cells are the same class of defect but are already logged under §9.1's *"three words for there is nothing here"*; the app renders `—`. **Not changed in this pass, on the owner's instruction to report scope first**: the size change alone rewrites 77 authored values inside an 88px declared track, the `Age` change lands in a 68px declared track that §9.1 already names as too narrow to widen casually, and the `Category` change lands in a `minmax(0, 0.9fr)` track. Each is a before/after capture of its own. ✅ **SIZE: DONE**, in `SU-07` below — all 77 authored values converted, counts confirmed against this survey, and the *"88px declared track"* above **corrected by measurement to 80px and 84px** (88px is the `Protocol` track, not `Size`). ✅ **`Age` and `Category`: ALSO DONE, in `SU-08` and `SU-09` below, after the owner answered on both later the same day.** ⏳ This clause read *"STILL OPEN, awaiting an owner decision, and deliberately untouched rather than dropped"* when `SU-07` shipped, with `Age`'s 36 cells and `Category`'s 18 cells still printing what the survey found. **It is amended rather than rewritten, because the sequence is the record**: neither was a defect a round declined to fix, each was a decision a round had no answer for, and a deferral that is later honoured is worth more visible than a deferral quietly deleted. 🚩 **All three columns resolved by moving the MOCKUP to the product, and only one — `Age` — was even considered the other way.** That is the pattern `SU-05` was trying to name: the product's actual output is the default truth, and moving it is the exception that needs an argument |
| **SU-05a** | ⚠️ **`SU-03b`'s deferral of `Age` and `Items` survives this, but its stated premise does not, and saying so is the point of `SU-05`** | The deferral's **verdict is untouched**: the move to `3ch` changes the size reserve only, and both columns are excluded from the reserve entirely. Its **premise** is a mockup measurement — *"`months` alone is 43px"* — and `months` is a word the product never prints in that column. **Left standing, annotated in §9.1 and here rather than silently re-derived**, because re-measuring `Age` against `d`/`h` would make the reserve roughly `1ch` and reopen a decision about a declared track on the two densest tables in the product. That is a change, not a correction, and it is not smuggled in behind one |
| **SU-06** | **The `2.5ch` reserve is corrected to `3ch`. §9.1's rule — *"reserve the widest unit the column can ever print, measured in the cell font and rounded up to the next half-ch"* — is UNCHANGED; only the family it was applied to was wrong** | **Re-measured independently rather than taking the frontend thread's numbers on trust, twice over and by two methods** — canvas `measureText` in the cell's resolved font shorthand, and a real inline span appended into a rendered `.is-num` `<td>` and read with `getBoundingClientRect()`. The two agree to under 0.02px on every member. At 13px IBM Plex Sans, `1ch` = 8px: **`B` 8px, `KiB` 19px, `MiB` 22px, `GiB` 20px, `TiB` 18px** — widest `MiB` at **2.75ch**, rounded up to **`3ch`**. (Decimal, for the record: `KB` 16, `MB` 19, `GB` 17, `TB` 15 — widest `MB` at 2.375ch → `2.5ch`, which is exactly what `SU-03` derived and exactly why it was wrong.) ✅ **`2.5ch` was run as the control rather than assumed to fail, and the control is the interesting half:** the figures still align at 0.00px spread, so nothing on screen looks broken — but `MiB`'s ink is **22px inside a 20px box and overhangs it by 2px, into the cell's right padding, at all three densities**. `scrollWidth` reports this as a fit, because it is an integer and clamps to `clientWidth` on a non-scrolling box; the overflow is only visible against a `Range` over the span's own text node. **A reserve that does not hold the widest unit in its family is not a reserve** |
| **SU-06c** | ✅ **Independently corroborated from a different tree, a different measurement and — the part that matters — a different SOURCE OF DATA** | The Requests thread shipped the Size columns onto the split in **`106cb89`** and **`690ee81`** (merged to `main` as **`5f0bc5e`** and **`6a1ef81`**; `6a1ef81` confirmed an ancestor of `origin/main` before citing it). **Their shipped unit boxes measure 24px** — which is `3ch`, not the `2.5ch` that had been documented. That number was reached **against the application**, whose Size cells render real binary units, while the measurements above were taken **against the mockups**, which draw decimal ones; the two agree because the type stack is identical — `--text-base: 13px` / `--leading-base: 18px` in `web/src/app.css` and `--fs-base: 13px` / `--lh-base: 18px` in `usarr.css`, so `1ch` = 8px and `3ch` = 24px in both. 🚩 **The direction of the agreement is the point**: `SU-05` says to trust the tree that renders what the product emits, and the app-side number is the one that was right first |
| **SU-06d** | **Two clauses added to §9.1, both settled by the owner, and both narrowing rather than extending the rule** | **(1) The absent-value premise is upgraded from data-dependent to STRUCTURAL, and verified here rather than relayed:** `toNotSentGrabResponse` in `internal/httpapi/grabs.go:331` assigns `ID`, `ReleaseTitle`, `Protocol`, `IndexerName`, `GrabbedAt`, `ErrorCode` and `Outcome` — and **never `SizeBytes`**, which is `*int64` with `omitempty`. A not-sent row therefore **cannot** carry a size. The release tables' field is `releaseResponse.SizeBytes`, a **plain `int64` with no `omitempty`** (`internal/httpapi/search.go:229`), always on the wire, so **that** em-dash arm is defensive and unreachable. **The distinction is kept because the two are indistinguishable in the markup** — three identical lines, one guaranteed reachable and one that nothing can reach — and it is the honest answer to why the Recent-grabs Size column mattered and the release table's did not. Both `+page.svelte` comments now say which of the two they are. **(2) A scope clause**: the rule constrains the **unit box**; the surrounding table's own conventions govern everything else about an absent value. Recent grabs keeps its `<span class="muted">` wrapper because every other absent value in that table mutes — `when`, `indexer`, `protocol` — and §9.1's requirement, *no `.unit` box on an absent value*, is met. ℹ️ **The clause exists to stop the question recurring**: without it every new table that meets this rule re-litigates its own conventions against it, and a design rule that reaches past what it is about turns every local convention into a conflict |
| **SU-06a** | **The alignment figure was understated, and the reason is that the sample did not contain the widest unit** | `SU-03` measured **two** rows, `68.4 GB` and `4 B`, and reported a spread of **9.00px** with the box off. Over the real family and a three-row sample — **`68.4 GiB` / `820 MiB` / `4 B`** — the figures' right edges sit at **924.06px, 924.06px, 924.06px with the reserved box (spread 0.00px)** and at **928.06, 926.06, 940.06 without it (spread 14.00px)**. `MiB` is what makes it 14. **Corrected in §9.1, in `SU-03` above, in `usarr.css` and `prototype.html`, and in `web/src/app.css`**; `web/src/lib/format.ts` already carried 14.00px from `57f46be`. ℹ️ 9.00px is kept in §9.1 as the explicitly-labelled understatement, because a sample that omits the widest member is the mistake worth naming, not the number |
| **SU-06b** | **Verified by a full before/after render capture, and the no-wrap and row-height claims were measured rather than eyeballed** | **5 screens × 2 viewports × 2 themes × 2 installs × every state × every panel — 1040 combos**, `document.fonts.ready` awaited, scroll normalised, `content-visibility` forced visible, per the three traps in `check.mjs`'s own header. Result below. **No wrapping at any density**: every `.unit--size` span is one line box (18px) for all five binary units at compact, standard and relaxed. **No height change at any density**: the size cell measures **26 / 30 / 34px** and its row **29 / 33 / 37px** at compact / standard / relaxed, byte-identical between `2.5ch` and `3ch` — the reserve is 4px of width and buys no height. ⚠️ **Those are the mockup's numbers and they are not 28px flat**; `--row-h` is a `min-height` of **28 / 32 / 36px** per density and the rendered row sits 1px above it. The claim to carry forward is *"unchanged at every density"*, which is what was measured |

**Gate**: `node docs/design/check.mjs` — **exit 0, all checks, both installs, every panel** — on the
merged tree; §13 copy corpus **6978** strings, unchanged, because the reserve is a width and the
mockups' sample data was deliberately not touched. `make check: OK`, with Go linting via
**`/root/go/bin/golangci-lint` 2.12.2** (built with go1.25.13) after an absolute-path `cache clean` —
`PATH` resolves `golangci-lint` to `/usr/local/bin/golangci-lint` at **2.5.0**, which is not the
gate's, per PG-04. Go tests pass; `web` 333 tests over 9 files pass; `govulncheck` v1.7.0 reports 0
called vulnerabilities.

**The before/after capture, in full.** 1040 combos, **1,296,736 element rects compared, 1080
differing** — and **every one of the 1080 is a `SPAN.unit unit--size`**, with a delta of exactly
`dw = +4.00, dh = 0.00, dy = 0.00`: 540 with `dx = −4.00` (the right-aligned cells, whose box grows
leftward from a pinned right edge) and 540 with `dx = 0.00` (the mobile stacked cells, pinned left).
**List geometries 1,392, differing 0** — no declared column track moved. Overflow verdicts,
`document.scrollWidth` and z-index chains all differing 0; elements past the viewport 0 before and 0
after. 🚩 **The zero-elsewhere is a zero over a population**: of **8,832** `.unit--size` spans the
sweep walks, 7,752 sit in a hidden state, panel or install and report a zero rect, and **all 1,080
that are actually laid out changed 20px → 24px — every single one, with no exceptions and nothing
else on any page moving.**


---

# The size half of SU-05: the mockups now draw the units the product emits

**Date:** 2026-08-17. **Branch:** `claude/hearth-thread-vn9w7u`. **Closes the SIZE half of `SU-05`
above.** `Age` and `Category` are the other two thirds of that finding and are **explicitly still
open** — see `SU-07d`. `SU-06` corrected the *reserve* that the decimal family produced; this round
corrects the *sample data* that produced it.

| # | Finding | Resolution |
|---|---|---|
| **SU-07** | **The mockups printed a size the product cannot emit, in every size cell on every screen. Converted rather than relabelled, because relabelling would leave the defect while looking fixed** | **All 77 authored values converted, and the count reconciles exactly with `SU-05`'s survey rather than being re-asserted:** **49** split-slot occurrences (`search.html` **31**, `requests.html` **18**) and **28** one-string composites in `index.html`'s Home `Detail` column — **77**, mirrored by `build_prototype.py` into `prototype.html` for **154 rendered** values. ✅ **The figures were converted, not the labels**: each decimal value was read as a byte count (`GB` = 10⁹) and re-emitted through the shipping app's own algorithm — `sizeParts` in `web/src/lib/format.ts`, which divides by 1024 and prints one decimal below 100 and none at or above it. So `14.2 GB` → **`13.2 GiB`**, `68.4 GB` → **`63.7 GiB`**, `402 MB` → **`383 MiB`**, `118.7 MB` → **`113 MiB`** (the decimal is dropped because 113 ≥ 100, which is the app's rule, not a rounding choice). 🚩 **Two values change unit member, and that is the useful side effect**: `0.8 MB` → **`781 KiB`** and `1.0 MB` → **`977 KiB`**, so Search's ebook Size column now **mixes `KiB` and `MiB` down one column**. `SU-03` had to justify the fixed unit box against sample data where *"no column mixes units — so nothing on the screen demonstrates the rule and 'it looks fine' proves nothing"*. Two columns now demonstrate it |
| **SU-07a** | ⚠️ **`SU-05`'s *"88px declared track"* is wrong, and it was checked rather than inherited** | **Measured, not read off the stylesheet by counting commas.** Every rendered `Size` cell was walked and matched to its resolved grid track: `Size` sits in an **80px** track on `.cols-requests-releases`, `.cols-requests-releases-audiobook`, `.cols-search-ebooks`, `.cols-search-movies-full`, `.cols-search-movies-v01`, `.cols-search-series` and `.cols-search-series-scoped`, and in an **84px** track on `.cols-search-episodes` and `.cols-search-tracks`. **88px is the `Protocol` track** in `.cols-requests-releases` — column 2, not column 6. ℹ️ Home's `Detail`, which carries the 56 composites, is not a fixed track at all: it is `minmax(0, 2fr)`, resolving to **276.11px** at 1440px |
| **SU-07b** | **Did a declared track come under pressure? NO — and the first answer measured said yes, which is why the metric matters more than the verdict** | **Ink measured with a `Range`, never with `scrollWidth`** — `scrollWidth` is an integer that clamps to `clientWidth` on a non-scrolling box, which is exactly how `MiB`'s 2px overhang hid from `SU-03`. **2,212 measurements**: every rendered `.unit--size` span, every size `<td>`, every Home `Detail` `<td>`, over 2 viewports × 3 densities × 2 installs × 5 screens × every state × every panel. **0 cells on more than one line, 0 ink past any content box — before and after.** ⚠️ **The obvious metric is the wrong one, and it read as a 3px cost.** Cell INK went from **48px** (`14.2 GB`) to **51px** (`13.2 GiB`) in the 80px track's **56px** content box, which looks like slack falling 8px → 5px. **It is not what the track sees.** The unit is a fixed **24px** box, so the width the layout resolves is *figure + gap + 24px* and **the unit string inside it is irrelevant to it**: the widest binary unit grew 19px → 22px of ink and 22 ≤ 24, so it grew entirely into space already reserved. **Measured laid-out width, figure's left edge to the reserved box's right edge: `14.2 GB` 55px, `13.2 GiB` 55px — identical, at compact, standard and relaxed. Slack 1px, before and after.** ✅ **That is the answer the rest of the evidence agrees with**: list geometries differing 0, and the only 32 rects that moved are mobile ones, where the cell is not right-aligned. **The `3ch` reserve is what absorbed the change** — this is the reserve earning its width rather than the track paying for it. **Home's `Detail` track is nowhere near pressure**: its tightest cell has **93.11px** of slack, unchanged, and is not even a size-bearing one. **No declared track was widened, and none needed to be** |
| **SU-07b-i** | 🚩 **A four-digit figure does NOT fit the 80px `Size` track — and it did not fit before this change either, so it is a bound to record, not a regression to fix here** | Found by asking what the widest string `sizeParts` can emit is, rather than what the sample happens to contain. The figure is `< 1024` except in the top unit and prints no decimal at or above 100, so the candidates are **`99.9`** (four glyphs, 28px under `tabular-nums`: three digits at 8px and a point at 4px) and **`1023`** (four digits, 32px). **Every `99.9 <unit>` lays out at 55px in the 56px content box — 1px of slack. Every `1023 <unit>` wraps to two lines, at every density.** ✅ **The control is the half that settles whose defect it is: `1023 MB` and `1023 GB` — the DECIMAL family the mockups drew until this commit — wrap identically.** The overflow is a property of the **4px between a digit and a decimal point** meeting a 56px content box, and is **unchanged by the unit family**. **Not fixed here**, because widening `Size` from 80px is a declared-track decision on the two densest tables in the product and this pass changed sample data, not layout. ℹ️ Realistic exposure is low but not zero: `1023 MiB` and `1023 GiB` are both reachable, a 1,023 MiB release being the likelier of the two |
| **SU-07c** | ✅ **The `3ch` reserve still holds every unit, re-confirmed against the converted data rather than carried over from `SU-06`** | `SU-06` measured the reserve against units injected into a probe. This measures it against **the sample data as authored**: over the whole sweep the widest rendered unit ink is **`MiB` at 22px inside the 24px box — 2px of right headroom, at every density, in both installs, at both viewports, with 0 overhang**. The before tree's widest was `MB` at 19px with 5px of headroom, so the conversion consumed exactly the 3px the family is wider — which is what `3ch` was chosen to absorb. **The reserve is doing the job it was re-derived for, on data that now exercises it** |
| **SU-07d** | ⏳ **`Age` and `Category` are NOT done, and this row exists so that cannot be read as an oversight.** ✅ **SUPERSEDED the same day**, by `SU-08` and `SU-09` | `SU-05` found three columns printing what the product cannot produce. **This round took size only.** `Age` — **36 cells** printing `2 days`, `3 years`, `11 months` — and `Category` — **18 cells** printing Newznab paths such as `Movies/UHD` — are **untouched and awaiting an owner decision**, exactly as `SU-05` left them. ⚠️ **`SU-05`'s premise for `Age` also needs re-measuring when it is taken up**, and not for the reason `SU-05` gives: `SU-03b` and `usarr.css` both state that *"`Age` in the release tables is a 68px track"*, and the track walk in `SU-07a` says **`Age` is 80px** on `.cols-requests-releases` — **68px is the `Grabs` track**. The `Age` deferral therefore rests on a second mis-identified track on top of the mis-measured word. **Reported, not acted on**, because correcting it changes an argument about a declared track and that is the `Age` decision, not this one |
| **SU-07e** | **The prose and the comments were converted with the markup, because a comment that says `GB` next to markup that says `GiB` is the next reader's confusion** | Four references to the same release outside the size column, all in `requests.html`, all reading **`68 GB`**: three in HTML comments (the roving-tabindex note, the Recent-grabs rationale, the *"two copies of a … release"* line) and **one in rendered prose** — the `<p class="note">` under Recent grabs. ℹ️ **The rendered one was outside `SU-05`'s survey**, which counted table cells; found by scanning for the string rather than for the column, and reported rather than folded in silently. All four now read **`64 GiB`** — the prose rounding of the table's `63.7 GiB`, matching the original's rounding of `68.4 GB`. The two verbatim copies of that sentence in `docs/design/mockups/README.md` were converted with them, so the two files cannot drift. **Deliberately NOT converted**: `76 KB` in `README.md` and `101 KB` in `fonts.css` — those are real measurements of real woff2 files, not sample data the product renders, and re-expressing a genuine decimal measurement in binary would be a new error rather than a fix |

**Gate**: `node docs/design/check.mjs` — **exit 0, all checks, both installs, every panel** — on the
merged tree; §13 copy corpus **6978** strings, **unchanged**, which is the expected result: the
converted cells are `<td>` data and a `<td>` is not copy, and the one rendered prose string in
`SU-07e` was edited, not added or removed. Rendered measurements used Playwright/Chromium with
`PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers`, `document.fonts.ready` awaited, scroll normalised and
`content-visibility` forced visible, per the three traps in `check.mjs`'s own header.

**The before/after capture, in full.** 5 screens × 2 viewports × 2 themes × 2 installs × every state
× every panel — **1040 combos, 1,296,736 element rects compared, 32 differing**. **Every one of the
32 is a `SPAN.unit unit--size`**, with a delta of exactly `dx = +4.00, dw = 0.00, dh = 0.00,
dy = 0.00`, and every one is at **390×844** — the mobile stacked cells, which are pinned LEFT, so a
figure that grows pushes the unit box right. They are the two cells whose figure changed shape:
`0.8 MB` → `781 KiB` and `1.0 MB` → `977 KiB`. With `tabular-nums` a digit is 8px and a decimal
point is 4px, so `0.8` at 20px becomes `781` at 24px — **+4.00px, arithmetic, not a reflow**. 🚩 **At
1440px the same cells differ by 0**, because there the column is right-aligned and the unit box is
pinned to the cell's right edge: the figure grows leftward into slack that `SU-07b` measured. **List
geometries 1,392, differing 0** — no declared column track moved, no header cell changed width, no
row changed height. **Overflow verdicts, `document.scrollWidth` and z-index chains all differing 0**;
elements past the viewport **0 before and 0 after**. ℹ️ The mirror-image of `SU-06b`, which is the
check that this capture is measuring what it claims: there the unit BOX grew 20px → 24px and moved
540 desktop cells by −4 while leaving mobile at 0; here the FIGURE grew and moved 32 mobile cells by
+4 while leaving desktop at 0. Same 4px, opposite viewport, for the opposite reason.


---

# SU-05's `Age` column: the mockups move to the product, and the move deletes a wrap

**Date:** 2026-08-17. **Branch:** `claude/hearth-thread-vn9w7u`. **Follows `SU-07`**, which took the
size column. **Owner decision on the shape**, taken after seeing the alternative: `Age` stays at
**hours and days** — *"I just think its better for understanding what you're looking at."* A
coarsening scheme (`5 months`, `2 years`, never compound) was drafted and **reversed before any of it
was written**, so there is no revert to read; it is recorded here because a decision that was
considered and rejected is worth more than one that appears from nowhere.

| # | Finding | Resolution |
|---|---|---|
| **SU-08** | **`Age` printed `2 days`, `11 months`, `3 years`; `formatAge` prints `d` and `h`. The mockups moved to the product, and this is the case where that direction needed no argument at all** | **18 authored cells** in `requests.html` — 36 rendered with the `prototype.html` mirror, reconciling with `SU-05`'s survey — converted to `formatAge`'s output (`web/src/lib/format.ts`: `h` below a day, rounded `d` above it). The word forms become day counts: `2 days` → **`2 d`**, `11 months` → **`330 d`**, `2 years` → **`730 d`**, `3 years` → **`1095 d`**. ✅ **No backend change, and that is the whole point**: the product already emits this and the mockup was the thing out of date. 🚩 **Two of `SU-05`'s three columns resolved by moving the mockup to the product, and only one — `Age` — was even considered the other way.** That is the pattern to keep: **the product's actual output is the default truth, and moving it is the exception that needs an argument.** ⚠️ **What the sample no longer demonstrates**: every rendered `Age` value is now `d`, because the release sample has nothing under a day old. The family still has two members, so `SU-03a`'s verdict — no unit in the header — still stands **on the family**, but the screen no longer shows why. That is the shape of trap `SU-03` fell into, named here rather than fixed, because adding an hours-old release is inventing sample data rather than converting it |
| **SU-08a** | 🚩 **The old sample data was not merely wrong, it was VISIBLY BROKEN, and nobody had measured it. `11 months` overflowed its cell and wrapped to two lines** | Found by measuring the column before changing it rather than after. `Age` is an **80px** track, **56px** of content after padding. Ink, by a `Range`, at 1440px: `2 days` **38px**, `11 days` **46px**, `3 years` **42px**, and **`11 months` 43px on each of two lines** — it wraps, at **compact, standard and relaxed alike**, taking its cell from **26px to 44px** and its row from **33px to 45px**. ✅ **After the conversion: 0 cells on more than one line, at any density**, and the widest value is `1095 d` at **43px in the 56px box — 13px of slack**. **The before/after capture caught exactly that and nothing else** — see below. ℹ️ This is the sharpest argument in the whole `SU-05` sequence for treating sample data as an instrument: the wrong data was not just generating wrong numbers, it was rendering a broken row in the file every design measurement is taken against |
| **SU-08b** | ⚠️ **The `Age` deferral from the unit split rests on TWO wrong numbers, both now corrected in place — and the deferral still stands, for a different reason** | **(1) The track.** `SU-03b`, §9.1, `usarr.css` and `web/src/app.css` all say *"`Age` in the release tables is a 68px track"*. **Walked rather than quoted: it is 80px.** 68px is the **`Grabs`** track, two columns along, and `git log -S` over `.cols-requests-releases` shows the row has read `60px 88px 80px … 80px 68px 84px …` since the class was created in `250f7be` — **the miscount is as old as the class**. **(2) The word.** `months` at 43px is measured correctly and is a word the product never prints. ✅ **Re-measured on the real family, two ways as `SU-06` did**: a span appended into a rendered `Age` cell gives **`h` 7px, `d` 8px** — widest `d` at exactly **1ch**, against 5.5ch for `months`. Widest figure the column can print is four digits at 32px under `tabular-nums`, so figure + gap + a 1ch box is **43px in a 56px content box, 13px of slack, one line**. **The cost argument for the deferral is void.** ✅ **`Age` is deferred anyway, and the reason is now honest**: §9.1's scope is *"size columns only"*, and extending a scope is a design decision rather than a correction. **Recorded in §9.1 and in `usarr.css`, not acted on.** ⏭️ **Needs routing, not done here**: `web/src/app.css` and `web/src/lib/format.ts` carry the same 68px sentence and belong to the frontend thread |

**Gate**: `node docs/design/check.mjs` — **exit 0, all checks, both installs, every panel** — on the
merged tree. `make check: OK`, Go linting via **`/root/go/bin/golangci-lint` 2.12.2** (built with
go1.25.13) after an absolute-path `cache clean`, per PG-04.

**The before/after capture.** Baseline is the tree `SU-07` landed, so this isolates `Age`. **1040
combos, 1,296,736 element rects compared, 644 differing — in 8 combos, all of them
`requests | nosink` at 1440×900**, which is the only state that renders the *Audiobook release
results* table where `11 months` sits. **Three delta shapes and nothing else**: **8** rects at
`dh = −18.00` — the `Age` `<td>`s that stopped wrapping, 44px → 26px; **40** at `dh = −12.00` — the
row and the blocks that contain it, 45px → 33px; **596** at `dy = −12.00` — everything below,
shifted up by the height the wrap was costing. **No `dx`, no `dw`, anywhere.** **List geometries
1,392, differing 8 — and in all 8 the change is the row-height array alone**
(`[79,33,33,33,33,45]` → `[79,33,33,33,33,33]`): **`grid-template-columns` changed in 0, header cell
widths changed in 0, table width 1232px → 1232px. No declared track moved, and none was widened.**
Overflow verdicts, `document.scrollWidth` and z-index chains all differing 0; elements past the
viewport 0 before and 0 after. ℹ️ `SU-03b` costed the reserve at *"+18px per cell"*; the wrap this
deletes cost **exactly −18px per cell**, from the other direction, which is the two measurements
agreeing.


---

# SU-05's `Category` column: our tags in the cell, the indexer's own path in the tooltip

**Date:** 2026-08-17. **Branch:** `claude/hearth-thread-vn9w7u`. **Closes `SU-05`.** With `SU-07`
(size) and `SU-08` (`Age`), all three columns `SU-05` found are now drawing what the product can
produce.

**Owner decision, and the reason belongs here rather than in a commit message.** The same release
found on two indexers comes back with **two different Newznab paths**, so the raw value renders one
fact two ways depending on who answered — the same defect the Requests thread removed from grab
outcomes. UsArr's derived tags are stable across indexers, so **they go in the column**. The raw path
carries detail ours drops — `Movies/UHD` against `movie` — so it goes in **`title`** rather than in
the bin, the same pattern as the poster titles and consistent with §9.1's tier 1.

| # | Finding | Resolution |
|---|---|---|
| **SU-09** | **18 cells printed Newznab paths the API does not put on a release row. They now print `categoryLabel`'s output, with the raw path in `title`** | **18 authored cells** in `requests.html`, 36 rendered with the mirror, reconciling with `SU-05`'s survey. `Movies/UHD` and `Movies/HD` → **`movie`**; `Audio/Audiobook` → **`book · audiobook`**; `Books/EBook` → **`book · ebook`** — verified against `mapping.MediaType` in `internal/servarr/mapping/mapping.go` rather than guessed, including the two-pass rule that makes `[3000, 3030]` an audiobook instead of music. 🚩 **The collapse is the point and it is now visible on screen**: six rows that said `Movies/UHD` and six that said `Movies/HD` all say `movie`, and the only place the difference survives is the tooltip — which is exactly the owner's argument rendered rather than asserted |
| **SU-09a** | ✅ **VERIFIED IN THE TREE BEFORE ANYTHING WAS BUILT ON IT, because a tooltip fed by mockup-only data would be a fresh instance of the bug this whole round is about. The raw path never reaches the browser** | `ReleaseResource.CategoryIDs()` (`internal/servarr/resources.go`) walks `Categories` and appends `c.ID` only — **`Name` is dropped at the first hop** — and `internal/releases/search.go` puts that `[]int32` on the wire. So there is no `raw_category` field and there never was one. **The tooltip is RECONSTRUCTED, not transmitted**, and that sentence is in §9.1 now because the next person will look for the field, not find it, and conclude the tooltip is impossible. The join is the row's `indexerId` and `categories[]` against the **indexer catalogue**, whose `CatalogIndexer.categories` is a per-indexer tree carrying upstream names; `categoryLabelFor` (`web/src/lib/indexercatalog.ts`) returns the upstream name verbatim. **Requests already loads the catalogue** (`catalog`, `catalogIndexers` in `web/src/routes/requests/+page.svelte`), so this needs **no backend work and no new fetch** |
| **SU-09b** | 🚩 **The obvious helper is the wrong one, and using it would silently defeat the entire purpose of the tooltip** | `categoryNames(categoryTree(indexers))` looks like the lookup and is not. **`categoryTree` builds a UNION across indexers**, and its own comment says so: *"First non-empty name wins. Two indexers naming 2000 differently is ordinary."* That is right for the picker, whose list must not reshuffle — and it is **exactly wrong here**, because the one thing this tooltip exists to preserve is the per-indexer divergence that the union discards. **The lookup must resolve the row's `indexerId` to its own `CatalogIndexer` and read that indexer's tree.** Recorded because the union helper is the one already imported on the page |
| **SU-09c** | **Two caveats carried into the design rather than discovered later, both about degrading honestly** | **(1) The catalogue is a probed replica**, so an id newer than the last probe has no name. The existing fallback is already honest and is the one to use: `categoryLabelFor` returns **`Category 2045`** — the number — never a guessed name. **(2) It is dead weight on a surface that does not already load the catalogue.** Requests does. **No other screen fetches the catalogue to populate a tooltip**: there the column shows the derived tag and carries no tooltip, which degrades honestly rather than blocking a render on an upstream fetch — principle 1, and §9.1's own rule that an absent value is not decorated |
| **SU-09d** | ✅ **The zero-delta is a zero over a population, and the old data was 0.41px from wrapping** | **384 rendered `Category` cells walked** — 2 viewports × 3 densities × 2 installs × every state — **384 carry a `title`, 0 wrap, 0 ink past the content box.** The declared track is `minmax(0, 0.9fr)`, resolving to **108.453px**, and it is content-independent, which the capture confirms rather than assumes: **1,296,736 element rects compared, 0 differing; list geometries 1,392, 0 differing.** ⚠️ **The one number that moved is slack, and it moved the right way for a reason worth recording: the tightest cell had 0.41px of it.** `Audio/Audiobook` sat under half a pixel from wrapping its 84.45px content box at 1440px; `book · audiobook` leaves **4.44px**. The old sample data was not only unproducible, it was **one glyph from a second broken column** — the same latent defect `SU-08a` found in `Age`, caught only because the before side was measured |

**Gate**: `node docs/design/check.mjs` — **exit 0, all checks, both installs, every panel** — on the
merged tree; §13 copy corpus **6978** strings and the `title` corpus **406**, both **unchanged** —
and that was checked rather than predicted, because the first draft of this line claimed the `title`
corpus would grow by 18 and it did not. The attribute sweep carries `if (el.closest('td')) return;`,
the same *"a `<td>` is data, not copy"* invariant that exempts the cells themselves, so **a tooltip
on a `<td>` is outside the copy corpus by construction** — as the release titles' own `title`
attributes already were. `make check: OK`,
Go linting via **`/root/go/bin/golangci-lint` 2.12.2** (built with go1.25.13) after an absolute-path
`cache clean`, per PG-04.

⏭️ **Needs routing, not done here.** `web/` is the frontend thread's tree and was not touched: the
Requests table needs the per-indexer `title` built as `SU-09a`/`SU-09b` describe, and
`web/src/app.css` and `web/src/lib/format.ts` still carry the *"`Age` … 68px track"* sentence that
`SU-08b` corrected in the design tree. ⚠️ **`SU-10c` below withdraws half of that last clause**:
68px is the app's own `Age` track and is right where it stands. What is wrong in `format.ts` is the
`43 px` beside it.


---

# The two deferred columns, decided: `Age` ships, `Items` does not, and the mockups and the app disagree about the track under both

**Date:** 2026-08-17. **Branch:** `claude/hearth-thread-vn9w7u`. **Takes up the two columns
`SU-03b` deferred from the figure/unit split** and closes both — one by applying the treatment, one
by re-deriving the cost and keeping the deferral on it. Neither outcome was assumed: a deferral
resting on a void argument reads as a considered decision, which is worse than either answer.

**What the round found, in one line.** Both deferrals were argued from a family measured off the
wrong column and a track quoted rather than walked; corrected, `Age`'s cost is zero and `Items`'
cost is real — and the reason the track numbers kept moving is that **the mockups and the
application declare different widths for the same table**, so a width without a filename attached
means nothing.

**Method, so the numbers can be re-run.** Unit and figure ink by a probe span appended into a
**rendered** cell and read with a `Range` — sub-pixel, and not clamped by any box — over 5 widths ×
3 densities × 2 installs × every state; fit and wrap answered by rewriting the cells **in the
rendered page** before any file was edited, so the decision rests on a measurement rather than on
arithmetic over a track width. `check.mjs`'s three geometry traps honoured throughout:
`document.fonts.ready` before measuring, `content-visibility: visible` forced for the duration, and
`scrollTo(0, 0)` first.

| # | Finding | Resolution |
|---|---|---|
| **SU-10** | 🚩 **THE ROOT CAUSE, AGAIN, AND AGAIN IT IS NOT ABOUT UNITS. The mockups and the application declare different column widths for the same table, so `Age` is an 80px track in one tree and a 68px track in the other — and both readings are correct.** Every argument in the `SU-03`…`SU-09` sequence quoted a width without naming the file it came from, and the two trees hold **both numbers on different columns**, which is precisely how one came to be quoted for the other | **Walked in both trees rather than relayed from either.** `.cols-requests-releases` in `docs/design/mockups/usarr.css` reads `60px 88px 80px minmax(0,3fr) minmax(0,1fr) 80px 68px 84px minmax(0,0.9fr) minmax(0,1fr) minmax(max-content,auto)` over the header row `Select · Protocol · Age · Title · Indexer · Size · Grabs · Peers · Category · Indexer flags · Actions` — so **`Age` 80px, `Grabs` 68px**. `COLUMNS` in `web/src/routes/requests/+page.svelte`, which `gridTemplate()` joins verbatim into `grid-template-columns`, reads `Protocol 80px · Age 68px · Title 2.4fr · Indexer 1.5fr · Category 0.9fr · Size 88px · Grabs 72px · Peers 112px · Flags 1.2fr · Actions minmax(max-content,auto)` — so **`Age` 68px, `Protocol` 80px**. 🚩 **Of the ten columns the two trees share, nine carry different widths — only `Category` (`0.9fr`) still agrees — `Category` is column 9 in the mockups and column 5 in the app, and the app has no `Select` column at all.** *(Eight when first walked; see `SU-10d`, which is why this row says "still".)* ⚠️ **Neither tree is stale in the ordinary sense.** The mockup class was created by **`250f7be`** and has never been edited; the app's widths were rewritten by **`cd94779`** ("widen two columns", 2026-08-16 21:05Z), which moved `Age` 76→68, `Protocol` 88→80 and `Grabs` 80→72. **So `80px is Grabs` was true of the app until `cd94779` and has never been true of the mockups, where 80px is `Age`.** ✅ **The rule this leaves behind: every width cites its file.** §9.1 now does, and so does `usarr.css`. **This is `SU-05`'s defect one level up** — there, a mockup drew data the product cannot emit; here, a mockup draws a layout the product does not have, and both keep generating measurements that are right about one tree and wrong about the other |
| **SU-10a** | ⚠️ **`SU-08b`'s *"a miscount as old as the class"* is withdrawn. The class was never miscounted; the number was imported from the other tree** | `SU-08b` reads: *"`git log -S` over `.cols-requests-releases` shows the row has read `60px 88px 80px … 80px 68px 84px` since the class was created in `250f7be` — **the miscount is as old as the class**."* **The first half is confirmed and the conclusion does not follow.** The 68px sentence entered the design tree at **`d9f1b39`** (2026-08-17 00:28Z) — **3 h 23 min after `cd94779` made the app's `Age` track 68px** — and `250f7be` touches no file under `web/`, so it cannot have introduced an app number. The likeliest provenance is the one the timestamps allow and the mockup's own history forbids: **a correct reading of the app, attributed to the mockup.** ℹ️ **The verdict `SU-08b` reached is untouched** — within the mockups `Age` is 80px and `Grabs` is 68px — and only the account of how the wrong number got there is corrected. 🚩 **Recorded because a freshly-corrected number is the last thing anyone re-checks**, which makes a correction the most dangerous place in the document to be wrong |
| **SU-10b** | 🚩 **The item-noun family is the SAME calibration error a third time, and this one was never a mockup-versus-product mismatch at all — it was a family assembled from the wrong COLUMN of the same table** | `SU-03b` costs Home's `Items` reserve at *"52px of reserve for `episodes`"* and §9.1's family bullet read *"item nouns `films · series · books · artists · issues · episodes`, widest `episodes` at 52 px → `6.5ch`"*. **`Items` cannot print `episodes` or `issues`.** It holds one noun per media type over six types — `films`, `series`, `artists`, `books`, `books`, `series` — while `episodes` and `issues` are words the **`Have`** column prints one column along (`13,204 / 14,061 episodes`, `7,891 issues · 34 with gaps`), and Items counts a comic library in **series**, not issues. ✅ **Re-measured on the four nouns the column can print**, by a probe span in a rendered `Items` cell: `films` **28px**, `series` **34px**, `books` **35px**, `artists` **37px** — widest `artists` at 4.625ch, so the reserve is **`5ch` (40px), not `6.5ch` (52px)**. ℹ️ **This family is the DESIGN's, not the product's, and the difference is stated rather than glossed**: Home's Block A is `NOT DRAWN` in the app — `web/src/routes/+page.svelte` says so in as many words and gives the reason — so unlike `formatAge` there is no shipping function to read the nouns off. §17.2's six rows are the source, and the family is re-derived the day Block A is built |
| **SU-10c** | ⚠️ **`web/src/lib/format.ts` carries the `months` measurement, and `web/src/app.css` carries a mockup measurement of a table the app does not draw. Both are the cross-tree leak in the other direction** | `format.ts` reasons that *"`Age` in the release tables is a declared 68 px track — 24 px of padding leaves 44 px, and the widest unit alone is 43 px"*. **The 68px is right** — it is the app's own track, `--row-pad-x` is 12px at all three densities, so 44px of content is right too — and **the 43px is the mockups' word `months`**, measured at exactly 43px here and unprintable by the `formatAge` three lines below it, which emits `h` (7px) and `d` (8px). `app.css` states *"Home's `Items` is 107.375px"*, which is the **mockups'** `Items` track at 1440px for a block `web/src/routes/+page.svelte` declares `NOT DRAWN`. ⏭️ **Routed, not fixed**: `web/` is the frontend thread's tree. What that thread needs to know is that on the app's own **44px** content box the `Age` reserve leaves **1px of slack**, not the mockups' 13px — figure 32px + gap 3px + box 8px = 43px — which is a materially tighter call than the design tree's, and is the reason the two trees' numbers must not be swapped again. *(Inference, marked as such: computed from the app's declared 68px track and declared 12px padding plus glyph advances measured in the identical 13px/18px type stack, not from a rendered app page.)* |
| **SU-10d** | 🚩 **THE DIVERGENCE WIDENED WHILE THIS SECTION WAS BEING WRITTEN. `SU-10`'s count went from eight of ten to NINE of ten between the walk and the push, and the change was already authored — on a branch neither this thread nor `SU-10`'s walk could see** | `SU-10` was written in `a844e8a` at **03:21:35Z** and read *"only `Category` (`0.9fr`) and `Actions` (`minmax(max-content, auto)`) agree"*. **That was true of `main` at that moment and is false now.** **`7fc932e`** ("the Requests actions column stops sizing to its own row's content") replaced the app's `Actions` track with a fixed **`198px`**; the mockups still read `minmax(max-content, auto)`, so **the only surviving agreement between the two trees is `Category`**. ⚠️ **The timestamps are the finding, and they are not the obvious ones.** `7fc932e` was authored at **02:24:15Z — 57 minutes BEFORE `SU-10` was written** — but it sat on `claude/hearth-thread-d247f2-gate` and reached `main` only as **`9bcb547` at 03:27:11Z**, which is **after** `SU-10` was written, after this section's merge, and while its gates were running. **So the walk was correct against the tree it walked, and was already stale against a tree that existed.** ℹ️ **The other thread's reasoning is sound and is not in dispute**: **ADR-0029** makes every row its own grid, so a content-sized track resolves against one row's content with no cross-row agreement to appeal to — it measured the header's actions track at 61px against a body row's 155.02px, and a body row with no `info_url` link at 63px against its siblings' 155.02px. **The `198px` is a measured reserve, not a guess.** 🚩 **The point is not that the change is wrong; it is that nothing propagated it to the mockups, and nothing would have.** `SU-10` argued the two trees drift because no mechanism holds them together — **this is that absence caught in the act, on one of the two columns the finding had just cited as an agreement.** ⚠️ **A count of divergent columns is therefore a PERISHABLE measurement**, and must name the tree and the moment it was taken, exactly as `SU-10` requires of a width. A width cites its file; a count must cite its commit **and** its clock, because a sibling branch can falsify it without touching either tree you looked at. ⏭️ **Not fixed here**: bringing the mockups' `Actions` track into step is a design decision on `usarr.css` that has to follow ADR-0029's per-row-grid reasoning, and `web/` belongs to the thread that made the change |
| **SU-11** | ✅ **`Age` takes the split. The deferral's cost was re-measured at exactly zero, over 960 rendered cells** | **18 authored cells** in `requests.html`, 36 with the `prototype.html` mirror — reconciling with `SU-05`'s survey — moved to the two-slot form: figure, then `<span class="unit unit--age">`, with `.unit--age { --unit-w: 1ch; }`. **The family is `formatAge`'s** (`web/src/lib/format.ts`: `h` below a day, rounded `d` above it), measured by a probe span in a rendered `Age` cell at **`h` 7px, `d` 8px** — widest `d` at exactly 1ch, so `d` fills the box and does not spill, which is the test `2.5ch` failed for `MiB` in `SU-06`. **The cost, measured rather than argued, and measured twice:** 960 cells per arm by rewriting the cells in the rendered page *before* anything was edited, then **1,920 cells and 1,920 `.unit--age` spans against the shipped class**. The widest value the column can print, `1095 d`, is **43px in the same 56px content box with the box and without it — 13px of slack — and 0 cells sit on more than one line in either arm**, at every width, density, install and state. **`d`'s ink is 8px in an 8px box, so 0 of 1,920 spans spill** — the test `2.5ch` failed for `MiB` in `SU-06`. ✅ **And the muting was confirmed rendered rather than assumed**: every `.unit--age` computes to `#5a534a` in light and `#b0a89b` in dark, which is `--fg-muted` in both, at 7.20:1 and 7.68:1 on the page ground. 🚩 **`--unit-w: 3ch` was NOT reused**, and that is the whole lesson of `SU-06`: the size reserve is a measurement of a size family, and pasting it onto a duration column would be the same error one column along |
| **SU-11a** | ⚠️ **THE CONTROL IS THE HALF WORTH READING, AND IT SAYS THE BOX BUYS 1.00px — NOT 14.00px. A reserve nobody measured the payoff of is a reserve nobody can defend** | Run exactly as `SU-03` ran it on the size column, and run against the **shipped `.unit--age` class** rather than an injected style, so the arm being defended is the one that ships: two rows forced onto the widest and the narrowest member of the family, and the **figures'** right edges read off, because the figure is the thing being compared down the column. **With the box, `2 d` and `3 h` put their last digit at 416.00px and 416.00px — spread 0.00px. Switch the box off and they sit at 416.00px and 417.00px — spread 1.00px.** Size's own control, for comparison, is **14.00px → 0.00px**. ✅ **The box is still right and the reason is stated rather than assumed**: `h` and `d` are two members with two different widths, so the misalignment the rule exists to remove is real and does occur the first time a row crosses from days to hours. 🚩 **But on this column the treatment's larger half is the MUTING, not the box** — the figure is the value and `d` is only its scale — and anyone who arrives expecting size's payoff should be told the number before they look for it. ℹ️ **The one-member test was run rather than skipped.** Had the family had a single width the box would reserve against a variation that cannot occur, and the finding would have been to apply the muting and skip the reserve; `h` ≠ `d` is what makes that branch not apply here |
| **SU-12** | ✅ **Home's `Items` stays deferred, and for the first time on a reason that is true today.** The reserve doubles the wrapping | Measured by rewriting the cells in the rendered page with a **correctly-sized 5ch** box (`SU-10b`) before any file was touched: over the sweep the column goes from **48 wrapped cells to 96**. ⚠️ **The failure is at 1280px and no previous statement of this deferral could have found it, because `Items` is a FRACTIONAL track and every statement quoted a resolution as though it were a width.** `minmax(0, 1.15fr)` is the declared track; it resolves to **107.375px at 1440px** — the number `SU-03b`, §9.1 and `app.css` all quote — and to **87.80px at 1280px**, the narrowest desktop width `check.mjs` sweeps, leaving **63.80px** of content. There the reserve takes **three of the full install's six rows to six of six, and one of the v0.1 install's two to two of two** — which is exactly the 48 → 96 above, over 3 densities and 4 states. At 1440px and above it fits, tightest slack **9.38px** on `1,842 books`. ✅ **So the verdict survives both of its corrections — the wrong family and the wrong track — and that is not the same as being vindicated**: the old argument was *"52px of reserve leaves 31px against a 36px `1,842`"*, and every quantity in it was wrong. **Corrected, re-measured, still deferred.** ⏭️ Widening a fractional track on Home is its own decision and belongs to whoever takes Block A |
| **SU-12a** | 🚩 **AND THE BEFORE SIDE WAS ALREADY BROKEN, which only the before measurement could show — the third time in this sequence that measuring the OLD state found a defect nobody had reported** | At **1280px**, in a **63.80px** content box, **three of Home's six `Items` rows wrap onto two lines today, with no reserve applied**: `1,204 films` **67px**, `612 artists` **64px** and `1,842 books` **74px**. `275 series`, `418 books` and `553 series` fit at 61–62px — `418 books` by **1.80px**. **48 wrapped cells across the sweep**, at compact, standard and relaxed alike, taking the row from 44px to 48px to 52px. ⚠️ **`check.mjs` does not catch this and is not wrong not to**: the row-height band at 1280px is 28–60px compact and 36–68px relaxed, and a wrapped `Items` row lands inside it. The guard for this is the ink-versus-content-box measurement, not the band. ℹ️ Same shape as `SU-08a` (`11 months` wrapping unnoticed) and `SU-09d` (`Audio/Audiobook` 0.41px from wrapping). **Reported, not fixed** — it is a track-width decision on Home, which is the same decision `SU-12` defers |
| **SU-09e** | ⚠️ **`SU-09b`'s conclusion is upgraded from *the tidier option* to *the only one that can work*, and the reason is that the loss is undetectable** | The Requests thread's guard comment on `categoryTree()` (`7ab87c6`, comment-only, confirmed the tip of `origin/main` before being cited) sharpens two things `SU-09b` stated too mildly. **(1) The winner is arbitrary from the user's point of view.** *"First non-empty name wins"* means first in the order the `indexers` array happens to arrive in: `categoryTree` iterates it as given, filtering on `isSearchable` and **never sorting it**, so the survivor is decided by the catalogue's delivery order — a display ordering, not an authority ranking. Whichever indexer comes first supplies the wording every row is labelled with, including rows from other indexers. *(Read in `web/src/lib/indexercatalog.ts` at `7ab87c6`. What that ordering IS upstream was not traced here, and the finding does not need it: an ordering the labelling rule was not designed around is arbitrary whatever it turns out to be.)* **(2) The loss keeps no record of itself.** `CatalogCategory` does not store which indexer supplied the surviving name, and the losing names are discarded rather than kept, so **nothing downstream can tell that a label came from a different indexer than the row it is labelling**. No amount of care at the call site recovers the divergence, because the information is gone before the call site exists. ✅ **So joining against the row's own indexer's subtree is not the better of two workable options — it is the only one that can work**, and a tooltip built on the union would be wrong in a way that is **invisible by construction**. 🚩 **The general shape, which is this whole round's shape:** a lossy transform that keeps no record of what it dropped cannot be checked downstream. The union looks like it succeeded on every row |

**Amended in place, above, rather than rewritten** — the sequence is the record, and a deferral
that is later honoured is worth more visible than one quietly deleted:

- **`SU-03b`** said *"`Age` in the release tables is a **68px** track: 24px of padding leaves 44px
  and `months` alone is **43px** … Home's `Items` is **107.375px**: 52px of reserve for `episodes`
  plus 24px of padding leaves 31px against a **36px** `1,842`"*. Of those six quantities, **one
  survives** — `1,842` is 36px. See `SU-10`, `SU-10b`, `SU-11`, `SU-12`.
- **`SU-05a`** said re-measuring `Age` against `d`/`h` *"would make the reserve roughly `1ch` and
  reopen a decision about a declared track on the two densest tables in the product. That is a
  change, not a correction, and it is not smuggled in behind one."* ✅ **The decision was reopened
  deliberately and answered: `SU-11`.** The reserve is `1ch`, and nothing was smuggled.
- **`SU-08b`** said *"the miscount is as old as the class"*. **Withdrawn — `SU-10a`.**
- **`SU-07d`** said *"`SU-03b` and `usarr.css` both state that '`Age` in the release tables is a
  68px track', and the track walk in `SU-07a` says **`Age` is 80px** … **68px is the `Grabs`
  track**"*. **True of the mockups and false of the application, where 68px is `Age` and 80px is
  `Protocol` — `SU-10`.**
- **`SU-09b`** said the union helper is *"exactly wrong here"*. **Stronger than that: `SU-09e`.**

**Gate**: `node docs/design/check.mjs` — **exit 0, 78 checks, both installs, every panel** — and
`make check: OK`, both run to completion **twice**: once on the merged tree at **`baff65e`**, and
again, **after** `origin/main` moved to **`9bcb547`** and `SU-10d`'s correction was written, on the
final tree of this branch — the one carrying `SU-10d` — immediately before the push, in that order.
*(That tree is named by its content rather than by a SHA because its SHA does not exist until the
commit containing this sentence does. `SU-13` is what makes the ordering worth stating at all: the
sentence is written first and is only made true by running the gate before the push, never after.)* §13 copy corpus
**6978** strings, `title` **406**, `aria-label` **468**, all
**unchanged**, which is what a change confined to `<td>` content should do and was checked rather
than predicted. `make check: OK` — `check-offline: OK`, then `govulncheck` v1.7.0 asserted against
its pin, no vulnerabilities found. Go linting via **`/root/go/bin/golangci-lint` 2.12.2** (built
with go1.25.13), which is the Makefile's own `GOBIN_DIR` pin and **not** the `$PATH` binary at
2.5.0, per PG-04.

❌ **`SU-13` IS WITHDRAWN. It accused a previous agent of fabricating a gate result, and the
artifacts refute it — artifacts that were sitting in the accusing agent's own scratchpad, with
mtimes, while the accusation was being written.** `SU-13` read: *"the paragraph above originally
reported a run that had not happened … That sentence was committed in **`a844e8a` (03:21:35Z)** —
**2 min 37 s before `9fab501` (03:24:12Z) created the final tree it claims to have been run
against.** The merge `baff65e` followed 9 s later and was on `origin/main` within the next 16 s,
against a check that takes roughly four minutes. 🚩 **So the second run was not merely unrecorded,
it was impossible — and the branch reached `origin/main` before any gate had passed on it**."*
**Both runs happened, the second finished before the push, and the tree it ran on is byte-identical
to the pushed tree in every file the gate opens.** The correction is recorded in place, rather than
`SU-13` being deleted, for the same reason `SU-13` itself gave: the sequence is the record.

✅ **Both runs survive as files, and each one fingerprints the tree it ran on.** `check.mjs`'s
`strip()` blanks a comment by replacing every non-newline character with a space, so the *"N chars
scanned"* it prints beside each check is the exact character count of the nine `SOURCES` files —
an unforgeable stamp of the tree under test. The two outputs are identical but for two numbers:
**run 1**, written **03:19:20Z**, 78 `ok` lines, `all design checks pass`, `exit=0`, **839471**
chars over nine sources and **731771** over `prototype.html`; **run 2**, written **03:23:51Z**,
same 78 checks and same verdict, **839498** and **731954**. **Re-derived from the trees rather than
read off the outputs**: `915a328` (*"Merge origin/main into claude/hearth-thread-vn9w7u before the
gate"*, 03:16:41Z) totals **839471 / 731771**; `a844e8a` totals **839498 / 731954**. 🚩 **The
deltas are +27 and +183 and they are `a844e8a`'s own edits.** The **+27** is the single comment line
it rewrites in `usarr.css` (90919 → 90946 chars); the **+183** is that line *plus two earlier
stylesheet-comment edits `prototype.html` had not yet mirrored* — which is precisely what *"brought
back into step"* names. **So run 1 was the merge tree and run 2 was `a844e8a`'s tree, and
`a844e8a`'s tree is the tree the sentence describes.** `make check: OK, exit=0` is logged at
**03:21:00Z**, between the two.

🚩 **`SU-13` attached its 157-second gap to the wrong commit.** `9fab501` touches only
`REVIEW-LOG.md` and `DESIGN-DIRECTION.md` and changes no file `check.mjs` opens; it cannot be *"the
final tree after the stylesheet comments and `prototype.html` were brought back into step"*, because
it brings nothing into step. **`a844e8a` is that commit** — it is the only one in the window that
edits `usarr.css` and regenerates `prototype.html` — **so the sentence describes its own tree, and
the run at 03:23:51Z is on exactly that tree.**

✅ **Run 2 finished 37 s BEFORE the push, not after it.** `origin/main` moved `7ab87c6 → baff65e` at
**03:24:28Z**, from this clone's `.git/logs/refs/remotes/origin/main`. ✅ **And the pushed tree is
the tested tree in every gate-visible file**: `9fab501` and `baff65e` share tree **`4ef3a84`**, and
`git diff --stat a844e8a baff65e` is three lines of prose in `REVIEW-LOG.md` and
`DESIGN-DIRECTION.md` — neither file is in `SOURCES`, neither is `prototype.html`. Recomputed at
`baff65e`: **839498 / 731954**, identical to `a844e8a`.

🚩 **The duration figure did the work, and nobody had measured it.** *"Roughly four minutes"* is
what turned a 37-second margin into *"impossible"*. Timed three times on this box at `4fb96e0`:
**127.6 / 128.5 / 128.5 s**. Relayed downstream the same figure grew to *"about fourteen
minutes"* — 6.5× high. **Both errors ran in the direction that made the accusation stronger**, which
is the signature of a number chosen rather than read.

ℹ️ **What honestly survives is two things, and neither is a fabricated gate result.** **(1)** The
gate sentence was committed at **03:21:35Z**, before the run it describes completed at **03:23:51Z**
— written ahead of its own evidence. **That is the practice the corrected paragraph above defends in
as many words**, and it is unavoidable: a commit cannot name its own SHA, so the line is written
first and made true by running the gate before the push, never after. **(2)** No gate ran on
`baff65e`'s exact SHA before the push — only on a tree identical to it in every file the gate opens.
**That gap was closed 4 min 52 s later**, at **03:29:20Z**, by a run against `origin/main` at
`baff65e` itself: **839498 / 731954**, `all design checks pass`, `EXIT=0`.

🚩 **The lesson is about `SU-13`, and it is worth more than the incident it got wrong.** `SU-13` is a
conclusion asserted rather than walked — **the exact defect it accused someone else of, one level
up** — and it rested on a duration nobody had timed. **The disconfirming evidence was three files in
the accusing agent's own scratchpad and it did not look.** An accusation is a measurement: it names
its artifacts and its clock, or it is a rumour with a flag in front of it. *(The recovery pass that
produced `SU-13` is not in dispute and is not disparaged here. It independently re-ran every
load-bearing number in this section and reproduced each exactly — `months` **43px**, `h` **7px**,
`d` **8px**, `1095 d` **43px** in the mockups' **56px** content box, the `Age` control
**1.00px → 0.00px** against size's **14.00px → 0.00px**, the `Items` nouns **28/34/35/37px**, the
**87.80px** 1280px resolution in which three of six rows already wrap — and it caught the real
staleness recorded as `SU-10d`. One finding in that pass is wrong; the pass around it is sound.)*

❌ **The rule `SU-13` derived is CONSIDERED AND REJECTED, recorded so nobody proposes it again.**
The candidate was *"a gate result whose write time precedes the tree it names is a fabrication."*
**It is false on this very incident** — run 2's output post-dates `a844e8a` by 2 min 16 s and
pre-dates the push by 37 s — **and it would fire on sanctioned practice**, because the gate paragraph
must be written before the commit that carries it and before the merge the gate then runs on. **A
rule that convicts the correct procedure is not a stricter rule, it is a broken one.** ✅ **What does
survive is the standing rule already in `CLAUDE.md`**: a green names its binary, its version and its
tree, and a guard is fired deliberately before it is trusted. **`SU-13` is the case FOR that rule,
not against the agent it accused** — the artifacts here were conclusive only because `check.mjs`
prints a per-tree character count beside every check, so an output cannot be attributed to a tree it
did not run on. A gate that printed only `PASS` would have left this unresolvable in either
direction.

**The before/after capture.** Baseline is the tree `SU-09` landed, so this isolates the `Age` split.
**1040 combos**, **1,296,736 element rects before and 1,299,616 after**. The split **adds**
elements, which the index-aligned rect dump cannot express — the existing `DROP` filter handles a
deletion only — so the compare grew an `ADD` filter that removes the added signature from the
**after** side and realigns the remainder. **`SPAN.unit unit--age` dropped from the after side:
2,880 across the sweep**, which is the whole 2,880-rect difference and a population rather than an
empty set, so the zero below is a zero over something. ✅ **The new filter was fired deliberately
before being trusted**, because a filter that has never been triggered is indistinguishable from no
filter: run with `ADD` off, the compare reports `RECT COUNT … 1252 -> 1270` on every `requests`
combo — **+18 rects per render, which is exactly the added spans** — and skips those combos whole.
And it is **not over-broad**: the sweep's **8,832 `.unit--size` spans stay in the comparison and
differ in 0**, so the same run proves the size column did not move either. **Element rects compared
1,296,736,
differing 0. List geometries 1,392, differing 0 —
`grid-template-columns` changed in 0, header cell widths changed in 0, table widths unchanged. No
declared track moved, and none came under pressure: the `Age` content box is 56px before and after,
holding a 43px `1095 d` both times.** Overflow verdicts, `document.scrollWidth` and z-index chains
all differing 0; elements past the viewport 0 before and 0 after. ℹ️ **A zero-delta capture is the
expected result here and is still worth running**: `d` is 8px and the `1ch` box is 8px, so the ink
is unchanged by construction — what the capture rules out is `SU-04`'s adjacent-sibling trap, which
is inert only while every such cell is exactly `(stacklabel, unit)`, confirmed across all 18.

⏭️ **Needs routing, not done here.** `web/` is the frontend thread's tree and was not touched.
Three items: the `43 px` in `web/src/lib/format.ts` is the mockups' `months` (`SU-10c`); the
*"Home's `Items` is 107.375px"* in `web/src/app.css` is a mockup measurement of a block that file's
own `+page.svelte` declares `NOT DRAWN`; and if the app takes the `Age` split, its reserve sits in a
**44px** content box with **1px** of slack, not the design tree's 13px.

---

## SD-01 — `DESIGN-DIRECTION.md` restated a fact it is not authoritative for. **Applied.**

**Found.** The header of `docs/design/DESIGN-DIRECTION.md` read *"**Status:** design document,
pre-alpha. **None of this design is implemented.** A `web/` directory now exists and carries a
SvelteKit shell — sign-in, a search page and a scaffold `/services` route whose own header says to
delete it when §17.3 lands — but it implements none of the system below: not the tokens, not the
density model, not the component set, not the state sets. Treat every value here as still ahead of
the code."* Every clause of that was false on the tree it sat on: `web/src/app.css` is ~2,556 lines
of ported tokens and components, `web/src/routes/services/+page.svelte`'s header is a §17.3
implementation rather than a delete-me note, and `web/src/routes/` carries `libraries`, `requests`
and `settings` besides. **This is the second correction of this line's class**, which is the finding
— the first correction replaced a wrong status with a right one and bought roughly a day.

**The rule, and it is the point rather than the edit.** *A document that is not authoritative for a
fact should not restate it — it should name the document that is.* A restated fact has no mechanism
holding it in step with the original; it is correct only until the original moves, and nothing
signals when that happens. `ARCHITECTURE.md` §16 owns milestone status and the tree owns what exists
today, so the design document's job is to point at them, not to mirror them. **Applied as wording
that asserts no status at all** rather than as a hedged or dated one, because a carefully hedged
status claim decays on the same schedule as a careless one. The header now opens *"Where
implementation status lives — not here, deliberately"* and routes the reader to §16 and to the code;
the "upstream of the UI" framing and the §17-wins rule below it are unchanged.

**Swept the rest of the file for the same shape, two more applied.** Both in §7.4, both about the
list bench: *"it is not on `main` and does not yet complete a full run (a 25,000-row Chromium
out-of-memory)"* and *"`bench:list` currently exits non-zero on a full run because of a 25,000-row
Chromium out-of-memory, so the OOM is fixed first"*. `pnpm bench:list` is on `main`
(`web/scripts/list-bench.mjs`, `web/package.json`), the OOM is handled as a named ceiling with
`recyclePage`, and the 2% drift gate the passage called *"once it lands"* is section **2b** of that
script. The design content — why `check.mjs` cannot host the assertion, the 2% budget, and the
fix→assert→enforce sequencing — is kept verbatim in substance; only the dated status is gone, with
`bench:list` and ADR-0029 named as the authorities for their own state. Two headings lost a status
clause with them: *"the fact that it is not enforced today"* → *"the one place it cannot be"*, and
*"Threshold, once it lands"* → *"Threshold"*.

⏭️ **Reported, not applied — and now settled; the deferral is kept above so the sequence is
readable.** §11's contrast rule said *"as of §9.7 **no such pair ships**, so the assertion currently
has nothing to run over"*. Same shape, but the surrounding prescription (a runtime WCAG solver over
`dominant_color`) was itself superseded by the poster title moving off the fill, so rewriting the
status sentence alone would have tidied a passage whose design content needed a decision first.
That decision is below.

**§11's computed-fill rule is restated as conditional rather than as current. Applied.** Verified
against the tree before writing: `constrainDominant` survives in `docs/design/mockups/usarr.js`
**only as the comment recording its deletion** — *"What used to live here was constrainDominant():
a WCAG ratio solver … It is gone, and it was not a bug fix — it was doing its job correctly"* — and
nothing in `docs/design/` sets text on a computed fill, so there was no live call site to change
the answer. **The rule survives, because it is right and cheap to keep; only its tense changes.**
The blockquote now opens *"**Where a surface sets text on a computed fill**, pick whichever of the
two theme text tokens scores higher…"*, so it binds a future call site without asserting anything
about the present. The CI sentence loses *"as of §9.7 no such pair ships, so the assertion currently
has nothing to run over and must not be reported as passing"* and gains the reason the assertion is
kept: *"a conditional rule needs a **standing** guard, because a guard added by whoever writes the
first call site is a guard that call site had to know about first — which is the same as having no
rule."* The retention is now deliberate on the page rather than inferable from a status note that
would go stale the moment a call site appeared. Which surfaces set text on a computed fill is named
as §9.2's question and the tree's answer, not §11's — SD-01's own rule, applied to the section that
prompted it. **One rider, and three more left as a follow-up:** four sites attributed the
poster-title move to **§9.7**, which is *"The minimum component set, and where per-type divergence
is allowed"*; the rule they mean is **§9.2**'s poster-grid entry, which is where *"the title and
year sit BELOW the tile"* is actually written. The one **inside** this passage is corrected, because
leaving it would have made the paragraph cite two different sections for one rule. The other three —
§2's summary table (*"The title sits below it, not in it (§9.7)"*), §7.1, and §13's checklist entry —
are **not** touched: that is a citation sweep, not a status one, and it is recorded here rather than
folded in.

ℹ️ **The follow-up has since run, and "three" was an undercount — the sweep found six live sites,
not four.** The three named above are corrected. Three more carried the same misattribution in code
comments the grep for markdown never reached: `docs/design/check.mjs`'s §1d CSSOM carve-out
(*"`--dc-fg` went with §9.7's move of the poster title off the fill"*),
`docs/design/mockups/usarr.css`'s `.card__t` comment (*"this is §9.7's rule rather than a
preference"*) and `docs/design/mockups/usarr.js`'s `constrainDominant` deletion note (*"DESIGN-DIRECTION
9.7 had already ruled that the title and year sit BELOW the tile"*). The last two are copied into
`prototype.html` by `build_prototype.py`, so it was **regenerated rather than hand-edited**, and its
two copies moved with them. **The undercount is the reportable part**, and its cause is that the
citation was counted over `*.md` while three of the six sites are `.mjs`, `.css` and `.js` — a
corpus chosen by file type rather than by where the rule is actually cited. 🚩 **Two further
mentions in this log are deliberately left standing**: `D-50` (*"which §9.7 had already ruled
against"*) and `PG-01` (*"the exact construction §9.2 and §9.7 ban"*). Both are dated records of
what a commit claimed at its own time, and §6.1's convention is that *"nothing above is renumbered,
reworded or deleted"*; correcting them would rewrite the evidence rather than the rule. §11's
superseded *"as of §9.7 no such pair ships"*
is quoted twice above for the same reason and is not a live citation — that sentence no longer
exists in the tree.

---

## SD-01a — the notice was unguarded, not pinned, and the guard gains the properties it was missing. **Applied.**

**Found.** The mockups' permanent notice — the label `DESIGN-DIRECTION` §13's fabricated-data ban
grants as its one exception — read *"Static design mockup of UsArr, which is pre-alpha software:
none of these screens is implemented and every value on them is invented"*, on all five source
pages and in the published `prototype.html`. **The first half went false when the screens shipped**
(`web/src/routes`; `ARCHITECTURE.md` §16's own inventory of the same claim was removed in `0b8637c`
for the same reason). The second half is true, and it is the entire reason the exception exists:
the data really is invented.

**The finding is that nothing asserted this sentence at all, and that is the whole of it.**
`grep -rn "none of these screens"` over the tree returns the five source pages, the generated
`prototype.html` and `mockups/README.md` — **no check at all**. `check.mjs`'s existing notice sweep
tested one property already (`/every catalogue source/`, that the notice describes the selected
install) and simply had no opinion about the rest of the sentence, and `build_prototype.py` asserts
only that `class="mocknote"` is present. So the string was not pinned; it was **unguarded**, which
is how it went stale in the first place. 🚩 **A sentence with no assertion over it has nothing
keeping it true, and will drift silently.** The guard here was **property-based from the start** —
it asserted one property where three were needed — so the fix below is *the missing properties*,
not the conversion of a literal into one.

**The general rule survives; it is the diagnosis of this incident that it is not, and an entry that
confused the two would misteach the next reader.** The rule: a guard that asserts a string
**verbatim** pins whatever that string says. Had such a guard existed here it would have been green
while the claim was wrong, and it would have **failed the first person to correct it** — which
inverts what a guard is for: it stops being a check on drift and becomes the drift's enforcement
mechanism. *A guard should assert a **property**, not a **fact**.* "The footer names its data as
invented" is a property and survives any honest rewording; the sentence itself is a fact, and
pinning a fact makes the guard an obstacle to fixing it. **Worth stating in that direction, because
it is the direction the fix took: a literal assertion over this notice would have been the defect,
not the safeguard.** The rule is why the replacement is three properties rather than one longer
string — it is not what went wrong here, because here there was nothing at all.

**Applied, three properties instead of one.** `check.mjs` now asserts, for **each** install: the
notice describes the selected install (unchanged); **it names its data as invented**, the half rule
13's exception is granted for; and **it makes no implementation-status claim at all** — `/pre-alpha|
unimplemented|implemented|shipped|ships/`. The third is the one that would have caught this, and it
is SD-01's rule expressed as a check rather than as prose: §16 and the tree own that fact, so a
mockup restating it owns a copy nothing keeps in step. The notice itself now reads *"Static design
mockup of UsArr. Every value on these screens is invented."* in both installs, with the compact
form losing its `Pre-alpha ` prefix for the same reason, edited in the five source pages and
regenerated into `prototype.html` through `build_prototype.py` rather than by hand.

**Fired deliberately before being trusted**, per `CLAUDE.md`'s *"fire a guard deliberately … one
that has never been triggered is indistinguishable from no guard"*. With `invented` removed from
the notice and the old *"which is pre-alpha software"* put back, `node docs/design/check.mjs` exits
**1** with exactly **4 FAILURES**, all of them the new checks and two per install:

> `FAIL  install: the full mockup notice does not name its data as invented, which is the one thing
> rule 13's exception is granted for — "Static design mockup of UsArr, which is pre-alpha software.
> Drawn over an install with every catalogue source connected,"`
> `FAIL  install: the full mockup notice makes an implementation-status claim ("pre-alpha") — §16
> and the tree own that fact, a mockup restating it owns a copy that goes stale (SD-01)`

Both repeat for `v01`, and the pre-existing install check stayed green throughout — which is the
other half of the evidence: the old guard could not see either defect.

**Counted rather than assumed, because a green over one fixed instance and eleven unfixed ones is
the failure this whole entry is about.** Measured against `30cd8db`, the tree before the sweep, and
against the tree after it:

| String | Before | After |
|---|---|---|
| the notice, long form — *"…none of these screens is implemented and every value on them is invented"* | **12**, in **6 files** (5 source pages × 2 installs, plus `prototype.html` × 2) | **0** |
| the notice, compact form — *"Pre-alpha mockup, invented data, …"* | **12**, same 6 files | **0** carrying a status word |
| `prototype.html`'s `<title>` — *"…static, invented data, **nothing implemented**"* | **1** generated, from **1** f-string in `build_prototype.py` | **0** |
| `tokens.css`'s header — *"Status: design document, pre-alpha. **None of these tokens is implemented.**"* | **1** | **0** |

**The last two are the point of counting.** The footer was the instance that was reported; the
`<title>` and the token header carried the same claim, in the same defect class, and would have
survived a pass that fixed only what it was pointed at — and the check would have gone green over
them. The `<title>` is the worse of the two, because it is the one user-visible string a rendered
DOM walk **cannot see**: §13's corpus counts `document.title` but does not read it. So the guard is
extended to it — *"document.title makes an implementation-status claim"* — and **fired deliberately
too**, by restoring the old title alone: `node docs/design/check.mjs` exits **1** with **1 FAILURE**,

> `FAIL  document.title makes an implementation-status claim ("implemented") — "UsArr screen mockups:
> static, invented data, nothing implemented". §16 and the tree own that fact (SD-01); the tab is not
> the place to restate it`

while all six notice assertions stayed green — the two guards are independent, and neither covers
the other. `tokens.css`'s header is rewritten to the SD-01 shape and keeps the claim that does **not**
go stale: nothing *imports* that file, `web/src/app.css` and the mockups' `usarr.css` both *port* it,
so its canonicity is a review rule rather than a build dependency.

**Four occurrences of the old wording are retained deliberately**, each framed as history rather
than as a claim: two superseded titles quoted in `build_prototype.py`'s comment, one in `check.mjs`'s
new comment, one in `tokens.css`'s new header. A record of what a string used to say is not a
restatement of it. **One is left for another pass and named here rather than stretched into this
one:** `docs/design/mockups/README.md:269` presents *"UsArr screen mockups: static, invented data,
nothing implemented"* as the **current** title in its changelog of the previous title fix, so it goes
stale with this change. That file is being swept separately. It quotes the same two strings this
entry corrects, in the same wording — **one sentence, not two variants**. ⚠️ **`SD-02r` records that
line and verdicts it *"True — `docs/design/mockups/prototype.html:6` matches byte for byte"*; that
was true when it was measured and is not true after this entry's `<title>` change, so `SD-02r` needs
re-verdicting by the pass that owns it.** `SD-02` is otherwise disjoint from this sweep — no row of
it names `tokens.css`, which is the one site here that was found by counting rather than by being
reported.

⚠️ **This entry's own headline and opening argument were corrected after it landed, and the change
is recorded rather than made silently, because §6.1's convention is that nothing above is
reworded.** It was first written as *"a guard that asserts a fact pins the fact"*, with the
verbatim-pinning argument leading and the *"nothing actually asserted this sentence"* finding
arriving a paragraph later — so the headline asserted a diagnosis the entry's own investigation had
already ruled out. The two are now in evidence order: the notice was **unguarded**, the guard was
property-based from the start and gained the properties it lacked, and the pinning rule is kept as
the counterfactual it always was. **Nothing measured changed** — not a count, not a quoted `FAIL`
line, not the `check.mjs` behaviour; only the framing over them. Logged here because an entry whose
headline contradicts its evidence teaches the wrong rule to whoever reads it next, which is a
sharper failure than a stale sentence.

---

## DS-07 and DS-14 — closed by `0b8637c`. **Amended dispositions.**

Both were **Open — recorded here rather than applied**, both against `ARCHITECTURE.md` §16's
landed/not-yet inventory, and both are closed by the code thread's `0b8637c`, *"docs: replace
per-feature shipped/not-shipped claims with pointers to the tree"* (on `main`; verified with
`git merge-base --is-ancestor 0b8637c origin/main` and by reading the diff). That commit **removed
the inventory** — from §16, and the equivalent claims from `CLAUDE.md` and `README.md` — leaving §16
authoritative for *which milestone owns a thing* and sending the reader to `web/src/routes`,
`internal/` and `internal/db/migrations` for whether it is built.

| # | Original disposition | Amended disposition |
|---|---|---|
| **DS-07** | Open — recorded here rather than applied | **Closed — and closed by a fix it did not propose, which is the clause worth reading.** DS-07 asked for the inventory to be **completed**: *"§16 gains a partial/scaffolding row and the README's row moves to `🚧 Partial`."* `0b8637c` did the opposite and **deleted the inventory**, on the reasoning that a fresher inventory is how this one went wrong. The sentence DS-07 quoted — *"the Services health **screen** (its endpoint exists, the UI does not)"* — is gone from `docs/ARCHITECTURE.md` rather than corrected, so a reader tracing DS-07's fix will not find the row it asked for and should not go looking. The defect is genuinely gone: nothing in §16 now claims the Services UI does not exist, so §17.3's account of that screen's rendered bugs no longer contradicts the section above it |
| **DS-14** | Open — recorded here rather than applied | **Moot by removal, not fixed** — the distinction matters because **nothing was added**. DS-14 named four shipped endpoints missing from §16's *"Landed so far"* list (`GET /api/v1/system/status`, `GET /api/v1/auth/session`, `POST /api/v1/auth/sudo`, and the sudo re-authentication window). `0b8637c` deleted the list. The four are still undocumented as landed *in §16*, and now correctly so: §16 no longer undertakes to say. **Sudo mode remains a substantive shipped security control**, so the underlying want — that a reader can discover it — is now served by `internal/httpapi/` and by `git log`, and by nothing in the docs. If that turns out to be too thin, it is a new finding against a document that chooses to carry an endpoint list, not a reopening of this one |

**The part that is not bookkeeping.** DS-07 was a **correct, written-down, still-open finding while
the documentation it described stayed wrong**. The log recorded the defect and did not prevent it;
what fixed it was a different thread hitting the same wrongness from the other side and arriving at
a better fix than the one recorded here. **A finding recorded is
not a finding fixed, and an open finding with no owner is a record that we knew better.** This log's
promise is that findings are never silently dropped — that promise is about the log, not about the
tree, and DS-07 is what the gap between the two looks like from the inside. Stated here rather than
turned into a process: a rule nobody applies at four in the morning would be a third record of
knowing better.

---

# The status-claim inventory — one rule, twenty-one remaining sites, and three that closed while it was being written

**Date:** 2026-08-17. **Compiled by the instructions gatekeeper from a drift check, not by a
reviewer, and the gatekeeper is applying none of it.** Every site was re-read against
**`0656bd9`**, the tip of `origin/main` when the sites were read, because the candidate list came
out of an earlier check and this repo moves fast enough that part of such a list is routinely
already fixed. **Three sites were**, between the first read at `0e7839e` and this one, and they are
recorded as fixed rather than carried. ℹ️ **`main` then moved to `b62eeb3` before this was
pushed**, and `git diff --stat 0656bd9..b62eeb3` touches `docs/PROJECT-INSTRUCTIONS.md` alone —
**every site cited below is byte-identical at both**, which is why the reads were not repeated and
why the pair is named rather than the later SHA alone. 🚩 **`main` moved again while this was being
corrected, to `a29a07f`, and that one is not inert: it flipped `SD-02r` from *true* to *false*.**
Amended in place with both SHAs on the row, because a verdict that decayed within a working day is
the strongest evidence this entry has and hiding the flip would waste it. The entry exists so the
inventory has a durable home and a named owner instead of living in cross-session messages, which is
the one place a finding reliably dies.

🚩 **SCOPE, AND IT IS THE FIRST THING TO READ: every verdict in this entry was measured against
`0656bd9` and means nothing about any later tree. Re-verify your own row against the current tree
before you act on it.** An inventory published without the SHA it was taken at silently reads as
*now* to every reader, forever — which is `SD-01`'s own defect committed by the document that logs
it, and the reason each verdict below carries a commit rather than a date alone. ⚠️ **This is not a
hypothetical hedge. Two rows needed correcting within about an hour of the entry being written**:
`SD-02r` went from **true** to **false** under `c2cefa3`, and `docs/design/tokens.css` turned out to
be **missing from the inventory entirely** — one decay and one gap, found by re-walking the tree and
recorded below as amendments. ✅ **That is the argument for the caveat, not against the inventory.**
Both defects were caught by exactly the re-verification this line asks of every reader; an inventory
that says which commit it saw is falsifiable, and one that does not is merely old.

## SD-02 — twenty-one sites restate an implementation status they do not own. **Instances of `SD-01`. Queued, not applied.**

**`SD-01` is the governing rule and this is not a second one.** It reads: *A document that is not
authoritative for a fact should not restate it — it should name the document that is. A restated
fact has no mechanism holding it in step with the original; it is correct only until the original
moves, and nothing signals when that happens.* `SD-01` applied it to `DESIGN-DIRECTION.md`'s header
and recorded that this was already **the second correction of that line's class**; `SD-01a` then
turned it into a `check.mjs` assertion for the mockup notice. 🚩 **What follows is one missing rule
applied twenty-one times, not twenty-one findings** — and that distinction is the whole value of
writing it down. A list of stale lines invites a round of one-off corrections and teaches nothing;
the rule is what prevents the twenty-second.

ℹ️ **`SD-01`, not `SD-01a`, is the parent, and the choice was made rather than defaulted to.**
`SD-01a` is the sharper rule — *a guard should assert a **property**, not a **fact*** — but it
governs sites that have a guard, and its own finding records that the notice it fixed *"was not
pinned; it was **unguarded**."* **Every site below is unguarded prose.** `SD-01a` is therefore the
*better fix* wherever a site has somewhere to put an assertion — `check.mjs` for the design tree, a
test for `Makefile` targets — and it is named as such in the rows where that applies; it is not the
rule the sites violate. **Both are quoted from the current text at `0656bd9`, not from an earlier
read**, because `SD-01`'s trailing paragraph was rewritten and `SD-01a` added while this was being
compiled.

**Placement follows §6.1 rather than inventing one.** That section's convention is *"Nothing above
is renumbered, reworded or deleted"*, new findings *"take the next free id in their own prefix … so
nothing existing is renumbered and nothing collides"*, and every claim names the commit it was
measured on. This entry is a pure append; `SD-02` is the next free id after `SD-01` and `SD-01a`; no
earlier round's table row is amended, because none of these sites is an earlier finding — the three
that closed are recorded as closed below rather than as amendments to rows that never existed.

**The fix at every site is one move, and `d11a1ca` is the worked example to copy.** So no row below
proposes corrected wording — each names **what is actually authoritative for the fact**, and the
edit is to route the reader there and assert no status locally. `SD-01`'s reason applies unchanged:
it was *"applied as wording that asserts no status at all rather than as a hedged or dated one,
because a carefully hedged status claim decays on the same schedule as a careless one."* A refreshed
count buys only the interval until the next migration lands.

🚩 **THE SCOPE LINE, AND AN OWNER WHO MISREADS IT WILL DESTROY CORRECT HISTORY. Only claims about
what CURRENTLY EXISTS are in scope. A claim about which migration created a thing is PROVENANCE, it
does not decay, and it must not be swept.** `docs/reference/schema.md` carries **15** `migration
000N` mentions and **13 of them are provenance** — *"`user_id` on these two tables arrived in
migration 0002, not 0001"* (`:768`), *"`acquisition_state` … arrived in migration 0003"* (`:750`),
*"`ix_audit_actor_action` (migration 0002) serves …"* (`:1047`), *"Inserted by migration 0001"*
(`:1297`), the `### 5.1 … · **v0.1, migration 0004**` heading, and the rest. **Every one of those is
a historical fact that is true forever, and deleting them would be a worse outcome than the drift
this entry logs.** Exactly two of the fifteen — `SD-02e` and `SD-02f` below — say something about
*how much exists*, and only those two are in scope. **This entry is not "remove migration references
from `schema.md`."**

### Three states, and two of them are not "open"

⚠️ **A queued work list that marks a site open while somebody is mid-fix invites duplicate work or a
collision on the same line — the same class of problem the entry exists to prevent.** So every site
carries one of:

- ✅ **Fixed** — verified gone at `0656bd9`. Recorded below with its commit and left out of the
  table.
- 🔶 **Claimed** — another area has said it is running this file as its own pass. Not open; talk to
  that area before touching it.
- ⏭️ **Open** — nobody has it.

✅ **Fixed while this was being compiled, all three verified gone at `0656bd9`:**

1. **`docs/reference/schema.md:3-6` — fixed by `d11a1ca`, the derived way.** It had read *"Migration
   0001 exists and creates `user`, `session`, … `tag_assignment`"* against **four** migrations on
   disk. 🚩 **It was worse than stale, and this is the part worth carrying: the roster read as
   COMPLETE while omitting `indexer_catalog`, which `00004_indexer_catalog.sql` creates and which
   the same file documents at §5.1 — so line 3 contradicted its own document.** A reader had no
   signal, because a complete-looking list does not advertise its gap. ✅ **The replacement divides
   authority rather than refreshing the roster**: the file owns the *shape*, `internal/db/migrations`
   owns *what exists*, and — the clause that makes it hold — *"A table given in full below may still
   be design-only."* **That is the template for every remaining row.**
2. and 3. **`docs/design/mockups/README.md:6-8` and `:15-16` — fixed by `2e357a5`, merged as
   `a19df1a`, and logged as `SD-01a`.** The permanent mockup notice and the README's *"none of this
   design has been implemented"* both went. ℹ️ **Reported to this compilation as "6 files, 12+
   occurrences" still outstanding; re-verified, that is no longer the tree.** `grep -rn "none of
   these screens is implemented" docs/` at `0656bd9` returns **one hit, and it is this log quoting
   the old string at `:4546`** — the five source pages and the generated `prototype.html` were all
   regenerated through `build_prototype.py`. The count was true before `a19df1a` and is 0 now, and
   it is recorded that way because a stale count inside an entry about stale claims would be the
   joke telling itself.

⚠️ **"Currently true" is not a defence, and 19 of the 21 remaining rows are — 14 true outright and 5 coarse-but-true, against two that are false.** ⚠️ **One of those two, `SD-02r`, was verdicted *true* here and went false about six hours later; it is amended in place below rather than rewritten, so the flip stays legible.** That is
the finding rather than a mitigation of it: a true restatement is indistinguishable, at the moment
you read it, from one that went false last night. The verdicts mean only *what the tree said at
`0656bd9`* — **true**, **false**, or **coarse-but-true** (defensible as written, but reading as a
narrower or wider claim than the tree supports).

| # | Site | The claim, quoted | At `0656bd9` | Authoritative for that fact | State |
|---|---|---|---|---|---|
| **SD-02a** | `docs/reference/crossmedia.md:3` | *"**Status:** designed, not implemented. **Scope:** **v0.3.**"* | **True** — no Wikidata or link-resolution code under `internal/` or `cmd/` | `ARCHITECTURE.md` §16 for scope; the tree for existence | ⏭️ Open |
| **SD-02b** | `docs/reference/gateway.md:3-4` | *"designed, not implemented … the OpenSubsonic read-only subset is **v0.4**"* | **True** — no OpenSubsonic or OPDS route in `internal/httpapi/server.go`'s mux | `ARCHITECTURE.md` §16; the mux in `internal/httpapi/server.go` | ⏭️ Open |
| **SD-02c** | `docs/reference/search.md:3` | *"designed, not implemented. **Scope:** tiers 1 and 2 are **v0.1**."* | **Coarse-but-true** — no `search_doc` / `search_fts` / `search_trgm` in any migration, so both tiers are genuinely absent; but `internal/httpapi/search.go` **does** ship a Prowlarr indexer search over SSE, and a reader who knows that reads this header as false | `ARCHITECTURE.md` §16; `internal/db/migrations/` for whether the FTS tables exist | ⏭️ Open |
| **SD-02d** | `docs/reference/sync.md:3-4` | *"designed, not implemented. **Scope:** channels 1, 3 and 4 plus the write queue are **v0.1**"* | **True** — no sync package; `write_queue` has a table and no writer (`grep -rl write_queue internal/ --include=*.go` returns the migrations, tests, the spike, and one comment in `internal/httpapi/grabs.go`) | `ARCHITECTURE.md` §16; the tree | ⏭️ Open |
| **SD-02e** | `docs/reference/schema.md:1125` | *"the `REFERENCES work(id) ON DELETE CASCADE` shown above is dropped there … **because `work` does not exist yet**"* | **True**, and 🚩 **it flips the day library sync lands** — the same pin `d11a1ca` just removed from line 3, surviving in the body of the file it was removed from. *(The surrounding sentences about which migration drops the FK are provenance and stay.)* | **`internal/db/migrations/`** | ⏭️ Open |
| **SD-02f** | `docs/reference/schema.md:1517` | *"Present in the design, **not in migration 0001**, added with the milestone named:"* — the *later tables* appendix header | **True**, and 🚩 **the same pin again**: absence is asserted against migration **0001** specifically, so the header goes wrong the moment a later migration creates any row of its own table. ℹ️ It is the second half of the sentence `d11a1ca` corrected — line 7's *"everything else is in the 'later tables' appendix"* now points here, and here still pins | **`internal/db/migrations/`** for existence; this appendix owns the *milestone* column | ⏭️ Open |
| **SD-02g** | `docs/reference/providers.md:3-5` | *"There is a Prowlarr client and a working connection test (§4); the registry seam (§1) and the Go provider interface (§2) are designed, not implemented, and **no code has been written for Sonarr or Radarr**"* | **True** — the only Sonarr/Radarr occurrences under `internal/` are comments asserting field-shape equivalence (`internal/servarr/resources.go:21,61,267,285`; `internal/releases/tags.go:38-39,60`) | `ARCHITECTURE.md` §16; `internal/servarr/` and `api/specs/` | ⏭️ Open |
| **SD-02h** | `docs/reference/security.md:3-4` | *"§1's envelope, AAD binding and `kek_id` column are built (the `usarr key rotate` command is not), and §2 and §5 are built."* | **True** — and it is the most granular status claim in the set, which is exactly what makes it the most expensive to keep true | `internal/crypto/`, `internal/ssrf/`, `internal/db/migrations/`; `ARCHITECTURE.md` §16 for scope | ⏭️ Open |
| **SD-02i** | `docs/reference/tags.md:3-5` | *"`source:`, `type:`, `format:` and `indexer:` are derived from Prowlarr search results today … `quality:`, persistence into the `tag` tables and any filtering by tag are designed, not implemented."* | **True** — `internal/releases/tags.go` derives them onto the response; no `INSERT` into `tag` or `tag_assignment` exists anywhere in `internal/store/` | `internal/releases/tags.go` and `internal/store/`; `ARCHITECTURE.md` §16 | ⏭️ Open |
| **SD-02j** | `docs/CONFIGURATION.md:3-8` | *"A first group of keys is now read by shipped code — the listener, the URL base, the data and config directories, logging, trusted proxies, the metadata User-Agent and the secret key (including `_FILE`)."* | **Coarse-but-true** — that enumerates 11 of the 13 `USARR_*` names `internal/config` reads; the two omitted, `USARR_INTEGRATION` and `USARR_RECORD`, are test-harness switches, so the omission is defensible and undeclared. ⚠️ **Same failure mode as `SD-02e`'s predecessor: a roster that reads complete** | **`internal/config`** — `DEVELOPMENT.md` §11 already binds the §2 table, `.env.example` and `internal/config` to land in one commit, so this header restates a fact that trio owns | ⏭️ Open |
| **SD-02k** | `docs/CONFIGURATION.md:126-132` | *"`USARR_LOG_LEVEL` and `USARR_LOG_FORMAT` are both live … **file logging is not implemented.** `$USARR_DATA_DIR/logs/` is created at startup and stays empty"* | **True** — nothing under `internal/` writes into `logs/` | `internal/config` and the logging setup in `cmd/usarr`; `ARCHITECTURE.md` §16 for the milestone | ⏭️ Open |
| **SD-02l** | `docs/CONFIGURATION.md:296-299`, restated at `:444` | *"There is no `usarr key rotate`, no `usarr keygen`, and no CLI subcommand of any kind — the binary takes flags only and **exits 1 on any positional argument**. `keys/secret.key.new` is never written: the path is defined in `internal/config` and has no caller anywhere in the tree."* | **True** — `internal/config/flags.go:67` rejects `fs.NArg() > 0`, and `Config.NewSecretKeyPath` (`internal/config/config.go:195`) is referenced only from `config_test.go`. ⚠️ **Stated twice in one file**, which is the restatement defect nested inside itself: two copies now decay independently | `internal/config/flags.go` and `cmd/usarr/` | ⏭️ Open |
| **SD-02m** | `docs/CONFIGURATION.md:432-434` | *"**It is the target layout, not an inventory of a running install.** Entries marked `[planned]` belong to subsystems that have not shipped … Everything unmarked is created or written by the code on `main`."* | **True**, and ℹ️ **the best-shaped claim in the set** — it declares its own genre and confines the perishable half to a marker instead of to prose. It is still a hand-maintained per-entry status inventory | `internal/config` for what the binary creates. **§5 of this same file is authoritative for the *layout*** — which is the half it does own, and which the rewrite must keep | ⏭️ Open |
| **SD-02n** | `docs/CONFIGURATION.md:220-224`, restated at `:788` | *"**`USARR_TSNET_*`** — later milestone (§9); documented, not shipped, not in `.env.example`."* / *"**Not in v0.1. Not implemented. Not in `.env.example`.**"* | **True** — no `tsnet` reference in `internal/`, `cmd/` or `go.mod`. **Stated twice** | `go.mod` and `ARCHITECTURE.md` §16 | ⏭️ Open |
| **SD-02o** | `docs/DEVELOPMENT.md:3-6` | *"**Status: pre-alpha. The first code has landed** … Most of the *layout* is still contract rather than description: commands referencing files that do not exist yet are marked **(not yet)**."* | **Coarse-but-true** as a header; ⚠️ it is also the sentence that **licenses** the markers in `SD-02p` and `SD-02q`, so it is the root of that half of the inventory rather than one more instance of it. Fixing the markers without fixing this leaves the mechanism in place | `Makefile` for what a target does; `ARCHITECTURE.md` §16 for the milestone | ⏭️ Open |
| **SD-02p** | `docs/DEVELOPMENT.md:168` | 🚩 *"`make dev                   # backend on :8484                  **(not yet)**`"* | 🚩 **FALSE.** `Makefile:284` declares `.PHONY: dev` and `dev:` runs `$(GO) run $(MAIN_PKG) --env-file .env`. ⚠️ **And this file already knows.** `:48` records *"An earlier revision of this line marked the target **(not yet)**, which was stale"* — about a different target, in the same document, **120 lines above a marker carrying the identical defect**. A correction that does not sweep its own file is `SD-01`'s *"second correction of this line's class"* arriving early | **`Makefile`** — the target list is the only thing that can answer whether a target exists | ⏭️ Open |
| **SD-02q** | `docs/DEVELOPMENT.md:615`, `:628` | *"`deploy/compose/dev-stack.yml` **(not yet)**"* / *"Instead, `make seed` **(not yet)** drives each app's own API after startup"* | **True** — `deploy/compose/` does not exist and the `Makefile` has no `seed` target. ℹ️ **Both are correct today by the same mechanism that made `SD-02p` wrong**: nobody re-walks them, and two of the three happen still to hold | **`Makefile`** and the tree | ⏭️ Open |
| **SD-02r** | `docs/design/mockups/README.md:269` | *"`prototype.html`'s `<title>` read … **It is now** *"UsArr screen mockups: static, invented data, nothing implemented"*"* | ⚠️ **CORRECTED — verdicted True at `0656bd9`, and FALSE at `a29a07f`, which is roughly six hours later.** The original reading stands as taken: `prototype.html:6` did match byte for byte. It no longer does — the design area's own sweep (logged above `DS-07 and DS-14`, and it flagged this row itself) extended `check.mjs` to `document.title` and the tag is now `<title>UsArr screen mockups: static, invented data</title>`, **without** *"nothing implemented"*. So the README's *"It is now"* quotes a title that no longer exists. 🚩 **This was the row called "the weakest in the table" and "NOT the same defect", and it is the only row that has since gone false — which makes it the entry's own thesis demonstrated on the entry, inside one working day.** ℹ️ **The reasoning that filed it as an exception was not wrong**: a dated record of an edit *is* a genre allowed to restate. What the reasoning missed is that this one restates in the **present tense** — *"It is now"* — and a present-tense quotation of a live string is a status claim wearing a historian's coat. **The fix is one word: past-tense it**, so the changelog records what the title was changed *to* at the time without asserting what it is *now* | **`prototype.html`** itself | 🔶 **Claimed by the design area**, which is running this README as its own pass — `SD-01a` and the `document.title` guard already landed here |
| **SD-02s** | `docs/ARCHITECTURE.md` §17.5, the *"Specified above \| Shipped in v0.1 \| Why they differ"* table (~`:3049-3063`) | *"**Six columns** — When · Release · Indexer · Protocol · Size · Outcome"*; *"on the wire as `sent \| sent_outcome_unknown \| unknown`"* | **Coarse-but-true.** Six columns confirmed in `grabColumns` (`web/src/routes/requests/+page.svelte`), though the first header renders `Time`, not `When` — the same file's comment explains why. The wire vocabulary in `internal/httpapi/grabs.go` is **four** values, not three: `wireOutcomeNotSent = "not_sent"` sits beside the three quoted, and the section's own prose does describe it. 📌 **NOT a request to delete the table**, which exists *because* `RG-01.1` found §17.5 describing something that never shipped; the rule asks only that the shipped column cite its two sources rather than assert independently | `internal/httpapi/grabs.go` and `web/src/routes/requests/+page.svelte` for what shipped; `ARCHITECTURE.md` §16 for what was funded | ⏭️ Open |
| **SD-02t** | `docs/ARCHITECTURE.md:2296-2301` | *"⚠️ **The cost estimate held; the shape it costed did not ship whole.** What landed is `GET /api/v1/grabs/recent` — **six columns, no join, no keyset cursor, a server-clamped `LIMIT`**"* | **True**, and ✅ **deliberately left standing** — `RG-01.1` records the reason: *a cost estimate edited after the fact stops being evidence about estimating*. ℹ️ **Listed as a legitimate exception rather than as an instance**: it is a dated estimate plus an amendment, and it already names §17.5 as authoritative for the difference, which is the move this entry asks for everywhere else | `ARCHITECTURE.md` §17.5, which this passage already names | ⏭️ Open, **as an exception to leave alone** |
| **SD-02u** | `api/specs/SOURCES.md:69` | *"They are absent because **no code consumes them yet**, and a vendored spec with no contract test behind it is a file that silently goes stale."* | **True** — `api/specs/` holds `prowlarr.json` and this file; `internal/servarr/contract_test.go` is the only consumer. ℹ️ **The claim is also its own rule** (*"Add each one with the client that reads it"*), which is the healthiest form on the list: it is falsified by the same commit that fixes it | **`api/specs/`** — a directory listing beside `internal/servarr/` | ⏭️ Open |

## Owners, taken from `docs/DEVELOPMENT.md` §11 rather than from the routing message

⚠️ **The owner split was re-derived from §11's own map, because assigning it from a recollection
would be this entry's own defect committed inside the entry that logs it.** §11's map is *"keyed by
area of the repo, not by thread name"*, and its rows are leads rather than exclusive ownership —
*"The map says who to talk to, not who is permitted to type."*

| Sites | Owner, in §11's own words | Which part of §11 |
|---|---|---|
| `SD-02r` — `docs/design/mockups/README.md` | the work that **"owns the screens and the visual system"** — and it is **already there**, so this row is a hand-off note, not an assignment | `ARCHITECTURE.md` §17 and `docs/design/` |
| `SD-02s` — `ARCHITECTURE.md` §17.5 | the same row: §17 is design-owned. ✅ **Precedent inside this file**: `RG-01.1` routed §17.5 to the design area on this convention, gave the reason — *"a code thread editing another thread's section is how two threads produce one conflict"* — and the design area applied it | `ARCHITECTURE.md` §17 and `docs/design/` |
| `SD-02t` — `ARCHITECTURE.md` §16's cost note | the work that **"landed the code being described"** | implementation-status wording in `CLAUDE.md`, `README.md` and `ARCHITECTURE.md` §16 |
| `SD-02a`–`SD-02i` — `docs/reference/*` | §11 gives no table row; its closing paragraph gives the rule instead: **"`docs/reference/` follows whichever change drove it."** So each file goes to the work that lands the code it describes — the backend work for all nine, with `SD-02e` and `SD-02f` specifically to whoever lands the next migration, which is the commit that falsifies them | closing paragraph, not a table row |
| `SD-02j`–`SD-02n` — `docs/CONFIGURATION.md` | §11 gives no ownership row; its onboarding bullet binds the file to the backend instead — **"A new setting goes in the §2 table, in `.env.example`, and in `internal/config` in the same commit — one that exists in two of the three is a bug."** So: the work that lands **`internal/` and `cmd/`** | onboarding bullet, plus the `internal/`/`cmd/` row |
| `SD-02o`–`SD-02q` — `docs/DEVELOPMENT.md` | §11 gives no row for its own file. The facts at issue are `Makefile` targets and `deploy/` paths, so: the work that lands **`internal/` and `cmd/`** and the build | `internal/` and `cmd/` row, reached by the fact rather than by the file |
| `SD-02u` — `api/specs/SOURCES.md` | §11 gives no row; the fact is which spec `internal/servarr` consumes, so: the work that lands **`internal/` and `cmd/`** | `internal/` and `cmd/` row, reached by the fact |

🚩 **The gap is itself worth reporting. §11's map has rows for `internal/`+`cmd/`, `web/`,
implementation-status wording in `CLAUDE.md`/`README.md`/§16, §17 + `docs/design/`,
`PROJECT-INSTRUCTIONS.md` and this file — and none for `docs/reference/`, `docs/CONFIGURATION.md`,
`docs/DEVELOPMENT.md` or `api/specs/`.** Four of the seven assignments above are therefore derived
from prose or from the fact at issue rather than read off the table. That is a defensible derivation
and it is not the same thing as a stated owner. **Offered as the follow-up: either §11 grows four
rows, or its closing "follows whichever change drove it" rule is restated as covering them.**
Deciding which is not this entry's to make.

⏭️ **Queued, not applied. No file in the repository was edited for this entry except this one.** The
gatekeeper compiled and verified the inventory; it owns none of the twenty-one sites, and applying
another area's wording is precisely what §11's *"announce before pushing an edit to a shared
document outside the area you lead"* exists to prevent.

**Two things the owners should not have to rediscover.** ℹ️ **`SD-01`'s reason for asserting no
status at all** — the nineteen non-false rows above are its evidence: they are correct, and they are not
safe, and `SD-02r` is what that sentence looks like when it comes due. 🚩 **And `RG-01.1`'s observation, which is why a one-directional sweep is not enough** — §17.5
was *"wrong in BOTH DIRECTIONS AT ONCE"*, doc-behind-tree in one paragraph and tree-behind-doc four
hundred words later, and the conclusion drawn there holds for every row here: *"nothing checks tense
against the tree at all."* A sweep that only hunts for *doc is stale* will write the other direction
back in. `SD-01a` is the one shape that escapes both — a guard asserting a **property** — and where
a site has a guard that can carry it, that beats prose.

**What was actually done, stated as the method rather than as a verdict.** Each site was read in the
file at **`0656bd9`**, reached by `git fetch origin && git checkout -B main origin/main` immediately
before the read; each existence claim was answered by `ls`, `grep -rl` or `git log` over
`internal/`, `cmd/`, `web/src/routes/`, `api/specs/`, `internal/db/migrations/`, `Makefile` and
`go.mod` — **not from any other document**. ⚠️ **Line numbers are a claim with a shelf life** — §11 says so in as many words — so each is
dated to `0656bd9` and the quoted string is the durable half of every citation; `docs/reference/schema.md`
and `docs/design/mockups/README.md` both moved during compilation and their rows were
re-read after the move. ✅ **Nothing was carried over unverified from the routing messages that
prompted this**, which is the reason `SD-02p` is in the table (reported as one of ~5 markers, and
the only false one), the reason `schema.md:3` is recorded as **fixed** rather than open, and the
reason the mockup banner count reads 0 rather than the 12+ it was handed.

**What a `make check` green on a docs-only commit does and does not attest — measured, because the
absolute version of this claim is wrong.** The tempting sentence is *"`make check` does not read
`docs/` at all"*, and it is **false**: `secrets` (`Makefile:562`) runs `$(GITLEAKS) dir . --redact=100
--no-banner --exit-code 1` over the **whole working tree**, `docs/` included. Everything else does
ignore it — `fmt-check` globs `*.go` on the Go side and runs prettier with its cwd in `web/`;
`modverify`, `test` and `vuln` never leave the module. ✅ **Fired in both directions rather than read
off the `Makefile`**, on `919623a`: a planted `ghp_`-shaped token in `docs/ZZ-probe.md` gives
*"leaks found: 1"* and `make secrets` **exits 1**; a malformed Markdown file at the same path passes
`make fmt-check` at **exit 0** (*"All matched files use Prettier code style!"*); the clean tree
scans **~8,308,092 bytes** and reports no leaks. **So the accurate statement is narrow and
defensible: a green on a docs-only commit attests exactly "no credential-shaped string in it", and
nothing whatever about whether the prose is true.** That is still the reason no green is quoted as
evidence for a single row above — quoting one would be `DEVELOPMENT.md` §11's *"probe the condition,
not a proxy for it"* violated inside an entry about documents claiming things they cannot back — but
it is the reason at its real size.

🚩 **And the first probe of that guard was a false negative, which is the part worth carrying.** The
planted string was the canonical `AKIAIOSFODNN7EXAMPLE` / `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`
pair from AWS's own documentation — **gitleaks allowlists it upstream**, so `make secrets` reported
*"no leaks found"* and exited **0** on a tree with a textbook AWS key pair sitting in `docs/`.
⚠️ **Had the probe stopped there it would have "proven" the secrets step ignores `docs/` — the exact
claim this paragraph was written to correct, arrived at through a working guard.** The `ghp_` re-run
is what separated *the guard did not look* from *the guard looked and correctly declined*. ℹ️ **This
is `DEVELOPMENT.md` §11's rule 1 — *"probe the condition, not a proxy for it"* — landing on the
probe rather than on the guard**, and it generalises: a **negative** result from a guard is only
evidence when the probe is known to be something that guard would catch. §11's existing instances
are all guards asserting the wrong thing; this one is a correct guard asserted with the wrong
stimulus, and it fails just as silently. **Recorded rather than fixed**: nothing in the repo needs
changing — gitleaks is right to allowlist a documentation example, and `.gitleaks.toml` is
untouched — what needed writing down is that the next person verifying this step must not reach for
the most famous fake key on the internet.

### SD-02 — amended dispositions, same day

**Three rows moved within hours of the entry landing, and the mechanism is `§6.1`'s: nothing above
is renumbered, reworded or deleted, and each amendment names what changed and how it was checked.**
Verified at **`98916fe`**.

| # | Original | Amended, and what settles it |
|---|---|---|
| **SD-02p** | ⏭️ Open — the one **false** row: `make dev` marked **(not yet)** | ✅ **Closed by `8756d02`.** `docs/DEVELOPMENT.md:168` now reads `make dev                   # backend on :8484` with the marker gone. Confirmed on disk at `98916fe` |
| **SD-02q** | ⏭️ Open — `deploy/compose/dev-stack.yml` and `make seed`, both **true** | ⏭️ **Still open, and now dated in the file itself by `98916fe`**, which is the better outcome: rather than delete two correct markers, it re-derived them (`deploy/` is absent from disk **and** from `git ls-files`; `seed` does not occur in the `Makefile` at all) and wrote the result down with its SHA. 🚩 **That is the shape this entry was asking for and did not think to ask for — a true claim made *checkable* rather than removed.** The same note records `SD-02p`'s marker as false and removed, so the file now carries the distinction between its three markers instead of one blanket licence |
| **SD-02r** | ⏭️ Open, verdicted **true** | ⚠️ **Amended in place in the table above: FALSE at `a29a07f`.** Cross-referenced here so the two amendments are found together |

✅ **The `§11` ownership gap is closed, by `8756d02`, and it is closed in the direction this entry
could not choose.** The map gained four rows — `api/specs/`, `docs/CONFIGURATION.md`,
`docs/DEVELOPMENT.md` and `docs/reference/` — and the last of them answers the open question rather
than dodging it: *"**has no fixed lead, and that is the answer rather than a gap in this table** — it
follows whichever change drove it … where a note pins itself to the migration state, it goes to
whoever lands the next migration, because that is the commit that falsifies it."* ℹ️ **So the
follow-up's two options were not chosen between; the table grew rows AND one of them states the
rule.** Every owner assignment in the section above is now readable off `§11` directly instead of
derived from prose — which retires the 🚩 that section carries, and the 🚩 is left standing as the
record of why the rows exist.

⏭️ **Still open after all of it: SD-02a–SD-02o, SD-02q, SD-02s–SD-02u — eighteen of the
twenty-one.** The three that moved are the two the routing message happened to name and the one that
decayed on its own. **That distribution is worth noticing**: attention went where a message pointed,
which is exactly the dynamic that put twenty-one sites in one table.
