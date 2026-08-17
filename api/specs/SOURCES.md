# Vendored upstream API specifications

These files are **vendored verbatim**, never fetched at build or test time.

Two reasons, both hard constraints:

1. *Arr apps guard `app.UseSwagger()` behind `if (BuildInfo.IsDebug)`, so a production
   instance does **not** serve `/docs/vN/openapi.json`. You cannot pull the spec from a running
   instance, and there is no public demo instance of any Servarr app with API access
   (searched 2026-08-16 — none official, none community-run).
2. CI has no network. The contract tests in `internal/servarr` and `internal/kavita` read these files directly.

Drift detection is **not the PR gate**: it needs network. Upstream `develop` moves; UsArr should
hear about a renamed field from a bot, not from a user's bug report. **`make spec-drift` is the
runnable half of that, and it exists now** — opt-in on `USARR_SPEC_DRIFT=1`, behind the `upstream`
build tag, never part of `make check`. It currently covers the Prowlarr row (see below); the Kavita
rows are still checked by hand with the recipes in this file.

## Files

| File | Upstream URL | Branch | Commit | Retrieved | SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `prowlarr.json` | `https://raw.githubusercontent.com/Prowlarr/Prowlarr/develop/src/Prowlarr.Api.V1/openapi.json` | `develop` | `1f7db1e` | 2026-08-16 | `efe3dfb9a928658d8a1f2f307a965fb1275bad2853012a2f9bdc2404215d0fbb` |
| `kavita-develop.json` | `https://raw.githubusercontent.com/Kareadita/Kavita/develop/openapi.json` | `develop` | `9c3e540` | 2026-08-17 | `3bd8363f0a4e847bf159127dbd9f9972725a5a0efb8d69831d2513ed3bab946d` |
| `kavita-v0.9.0.2.json` | `https://raw.githubusercontent.com/Kareadita/Kavita/v0.9.0.2/openapi.json` | tag `v0.9.0.2` | `6bcd568` | 2026-08-17 | `6d06c0a7081888cab6a1eadc06a5cb8158af9d6a138d6878284fae38c56c2f1a` |

Prowlarr version at that commit: **v2.6.2** — the in-development line, read from `azure-pipelines.yml`
(`majorVersion: '2.6.2'`) at `1f7db1e`, not from the spec, which self-reports a placeholder. The
highest RELEASED tag as of 2026-08-17 is `v2.6.1.5509`. ⚠️ **`prowlarr.json` is byte-identical at
`develop` and at tag `v2.5.2.5491` and is ten months older than either** — see the Prowlarr section
below before drawing any conclusion from this row.

**Kavita is vendored TWICE, on purpose ([ADR-0046](../../docs/DECISIONS.md#adr-0046)).** Neither file
alone answers both of the questions a contract test is asked, so the suite runs against both and the
subtest name says which:

| File | Role | `info.version` | Bytes | Paths | What a green against it means |
| --- | --- | --- | --- | --- | --- |
| `kavita-v0.9.0.2.json` | **FLOOR** — the release the owner runs | `0.9.0.0` | 818559 | 462 | the thing UsArr depends on exists on a server somebody actually runs |
| `kavita-develop.json` | **CEILING** — where the API is defined | `0.9.0.20` | 900763 | 488 | nothing upstream added or renamed has gone unmodelled |

Both are `openapi: 3.0.4`. The floor's `info.description` reads *"Built against v0.9.0.0"*, the
ceiling's *"Built against v0.9.0.20"*.

⚠️ **THE FLOOR IS NOT THE OWNER'S SERVER EITHER, and the file says so itself.** `openapi.json` at tag
`v0.9.0.2` declares `info.version` **0.9.0.0** — upstream did not regenerate it for the last two
patch releases on that line. It is the nearest checked-in artefact to the deployment, not the
deployment. **A real instance remains the only evidence about a real instance**, which is why
[ADR-0035](../../docs/DECISIONS.md#adr-0035) §2a's live observations are not superseded by anything
in this directory. Where a claim matters, check the tag's *source*: for the four develop-only
`SeriesDto` properties the floor spec was confirmed against `Kavita.Models/DTOs/SeriesDto.cs` and
`Kavita.Models/Entities/Series.cs` at `6bcd568`, and it agreed.

**Kavita is different from every \*Arr in one way that makes this affordable: the spec is a
checked-in file, not a debug-only endpoint.** `openapi.json` sits at the repository root and is
generated from `Kavita.Server/Startup.cs`'s Swashbuckle configuration, so it is fetchable at any ref
without a running instance — which is exactly why a second copy at a tag costs one `curl`. That does
not change the vendoring rule — CI still has no network and the contract tests in `internal/kavita`
still read the files directly.

**The drift job now has two rows to check, and they are checked differently.** `kavita-develop.json`
tracks a moving branch tip and drifts by default; `kavita-v0.9.0.2.json` is pinned to an immutable
tag and can only change if the tag is re-pointed, so a diff there is a supply-chain signal rather
than routine upstream movement. **When the owner upgrades, the floor row is re-vendored at the new
tag** — new URL, new commit, new date, new hash — and `pinnedSpecs` in
`internal/kavita/contract_test.go` plus `kavitaSpecFiles` in `internal/libsync/kavita_test.go` are
updated with it. `TestBothSpecsAreTheDocumentsSOURCESSays` fails if the file and that table disagree.

⚠️ **Older documents call these files by earlier names.** `ARCHITECTURE.md` §7.1a and `RESEARCH.md`
say `kavita-openapi.json`; `ARCHITECTURE.md` §6.4, `DECISIONS.md` (ADR-0035, ADR-0044),
`SETUP-CHECKLIST.md` and every `REVIEW-LOG.md` entry before this one say `kavita.json`, which was the
develop copy and is now `kavita-develop.json`. The pointer is recorded here rather than by rewriting
documents this thread does not own — and `REVIEW-LOG.md` in particular is a historical record that
**must not** be back-edited. **The §7.1a claim itself re-verified clean against the ceiling's bytes**:
exactly two operations across all 488 paths carry a parameter matching
`since|modified|updated|changed`, and they are `GET /api/Collection` and
`POST /api/ReadingList/lists`, both named `sortByLastModified` — neither on Series, Volume or
Chapter, exactly as §7.1a states.

### ⚠️ What the two files do NOT make safe

Vendoring the floor closes the *"is this endpoint even on the owner's server"* hole. It closes
nothing else, and three gaps survive it:

1. **Other users run other versions.** Two pins are two points on a line, not the line. A user on
   v0.8 gets no coverage from either file, and the honest answer is that UsArr degrades rather than
   that it is verified there.
2. **The controller still wins over the schema**, on both lines. Every fact `internal/kavita` relies
   on was checked against Kavita's *controller source* as well as against these documents —
   `Kavita.Server/Controllers/*.cs`, `Kavita.Common/Helpers/UserParams.cs`,
   `Kavita.Server/Extensions/HttpExtensions.cs` — because, as `DEVELOPMENT.md` §5 puts it for the
   \*Arrs, the controller wins over the schema. Two schemas do not change that.
3. **A field on the ceiling and not the floor decodes to a Go zero, silently.** There is no error
   and no warning; the property is simply absent from the response body.
   `ceilingOnlyProperties` in `internal/kavita/contract_test.go` is the machine-checked list of
   those, and `TestCeilingOnlyPropertiesAreDeclared` recomputes it from these two files on every
   run. As of this vendoring it is `SeriesDto.{cbrId, mangaBakaEditionId, isStandAlone, nameLocked}`
   and `LibraryDto.metadataProvider`. The live consequence: `libsync.kavitaExternalIDs` writes a
   `cbr` external_id from `cbrId`, and **that row is unreachable on the owner's install.**

**Record the Kavita version on every cassette — and it is NOT free here.** Kavita sends no
version response header: `Kavita.Common/Constants/Headers.cs` declares `x-kavita-version`, but the
only custom header `Startup.cs` puts on an `/api/*` response is `Pagination` (exposed via CORS at
lines 305 and 314). The version has to be *asked for*, at `GET /api/Server/server-info-slim`
(admin-only) or `GET /api/Plugin/version?apiKey=` (anonymous, and it puts the credential in the
query string). Record it in the cassette file's header comment, by hand.

**Observed 2026-08-17 22:20 UTC** (re-measured when the floor was vendored; the earlier 18:30 reading
of the same two values agreed): `refs/heads/develop` resolved to
`9c3e5400007f8a0282f7d883f2ad5e71716e514d` and `refs/tags/v0.9.0.2` to
`6bcd5689385d0e96824982d843c54f15ce784ddc`, and the `openapi.json` each URL served hashed to its
SHA-256 in the table — 900763 and 818559 bytes. As with the Prowlarr row: that is a measurement at a
moment, not a standing property. For the tag it is closer to standing, since a tag has to be moved
deliberately.

Re-check both the same way:

```bash
git ls-remote https://github.com/Kareadita/Kavita develop refs/tags/v0.9.0.2   # -> the commit column
curl -sS https://raw.githubusercontent.com/Kareadita/Kavita/develop/openapi.json \
  | sha256sum                                                 # -> kavita-develop.json's row
curl -sS https://raw.githubusercontent.com/Kareadita/Kavita/v0.9.0.2/openapi.json \
  | sha256sum                                                 # -> kavita-v0.9.0.2.json's row
```

## ⚠️ The Prowlarr spec describes NEITHER the deployment nor `develop`

**Prowlarr is vendored ONCE, and that is a decision rather than an omission
([ADR-0047](../../docs/DECISIONS.md#adr-0047)).** [ADR-0046](../../docs/DECISIONS.md#adr-0046) gave
Kavita a floor and a ceiling; the pattern does not port here, and the reason is measured:

| Question | Answer, measured 2026-08-17 |
| --- | --- |
| Blob of `src/Prowlarr.Api.V1/openapi.json` at tag `v2.5.2.5491`? | `134d31d7df5e80714c454a6224e7449df512c55e` |
| …at `develop` (`1f7db1e`)? | `134d31d7df5e80714c454a6224e7449df512c55e` — **the same git blob** |
| …and `api/specs/prowlarr.json`? | the same blob again (`git hash-object api/specs/prowlarr.json`) |
| When was it last regenerated upstream? | **2025-06-07**, commit `60740fa25` ("Automated API Docs update") |
| Releases tagged since? | **33** (`git tag --contains 60740fa25 \| wc -l`) |
| What does the file say its version is? | `info.version` **`1.0.0`** — Swashbuckle's placeholder, **not a Prowlarr version**. Never pin to it |

**Floor and ceiling are one file, so splitting it would vendor 145 KB twice and produce a green that
proves exactly what one copy proved.** The honest consequence is worse than "one file instead of
two": **Prowlarr does not regenerate this document per release, so the one file describes neither ref
reliably.** It is evidence about the API's *shape*, and **Prowlarr's source at a tag is the evidence
about a release** — `git show v2.5.2.5491:src/Prowlarr.Api.V1/…`, no running instance required.

**It already misdescribes both refs, and the divergence is machine-checked rather than described
here.** `SearchResource.Limit` and `.Offset` became `int?` in `c687bdb1f` (PR #2654, first released
in **v2.3.6.5351**, so present on the owner's release *and* on `develop`); the spec, regenerated ten
months earlier, still declares them non-nullable `int32`. `knownSpecDivergences` in
`internal/servarr/contract_test.go` pins that, and asserts the divergence is **still there** — an
entry that stops being true means upstream finally regenerated, which is news.

**The only known real deployment runs stable `v2.5.2.5491`** — **owner-confirmed 2026-08-17**, stated
by the owner of his actual install (checked against a human's box, not inferred from a changelog). It
still bounds what a contract test proves: **a green here is evidence about a document, not about the
server the owner is actually talking to.** A real instance is the only evidence about a real instance. This is not
hypothetical — a grab failure came from exactly this gap in another form, where the test fake was
written from the spec and so inherited the spec's silence, validating nothing.

### Two guards, split across the network line

`make check` makes exactly two network calls, both to vulnerability databases, and `make
check-offline` is defined as `check` minus those two. Adding a third would make a GitHub outage
redden somebody's unrelated commit, so the guard is split:

| Guard | Where | Network | Catches |
| --- | --- | --- | --- |
| `TestVendoredSpecIsThePinnedBlob` | `internal/servarr`, **in `make check`** | no | the vendored file changing under the suite — a re-vendor, a hand-edit, a bad merge |
| `TestUpstreamRefsStillShareThePinnedBlob` | `//go:build upstream`, **`make spec-drift`** | **yes** | the day upstream regenerates and the one-file premise dies |

`make spec-drift` refuses without `USARR_SPEC_DRIFT=1`, exactly as `make test-integration` refuses
without `USARR_INTEGRATION=1`. **A failure there is news, not a broken build.** It resolves both refs
with a blobless fetch, so it costs trees and no file contents:

```bash
git ls-remote https://github.com/Prowlarr/Prowlarr develop refs/tags/v2.5.2.5491
USARR_SPEC_DRIFT=1 make spec-drift
```

**Record the Prowlarr version on every cassette.** A wire capture from 2.5.2 and one from 2.6.2 are
different evidence and must not be indistinguishable in the fixtures directory. There is no excuse
for omitting it: **`X-Application-Version` is on every `/api/*` response**, so the version is free to
capture at record time.

The `raw.githubusercontent.com/.../develop/...` URL serves the branch tip, so the file retrieved
on 2026-08-16 is the branch tip as of that date. The commit column records the commit the facts in
`docs/reference/arr-apis.md` §7 and `internal/servarr` were verified against. The SHA-256 is the
mechanical identity of the vendored bytes and is what the drift job should compare.

**Observed 2026-08-16 17:39 UTC:** `refs/heads/develop` resolved to
`1f7db1e651249f1a3da0d8b55fbc0b2dd980b37a`, and the `openapi.json` that URL served hashed to the
SHA-256 in the table — byte-identical to the vendored copy, `diff` clean, 145360 bytes. That is a
measurement at a moment, not a standing property: `develop` moves, and the next commit to it makes
this line history rather than fact. It replaces an earlier ⚠️ noting the commit had been supplied
rather than read back from a live ref; it has now been read back.

Re-check it in two commands, from the repo root, and update the row plus the observation above with
what you get:

```bash
git ls-remote https://github.com/Prowlarr/Prowlarr develop   # -> the commit column
curl -sS https://raw.githubusercontent.com/Prowlarr/Prowlarr/develop/src/Prowlarr.Api.V1/openapi.json \
  | sha256sum                                                # -> must equal the SHA-256 column
```

Equal means the vendored bytes are still the branch tip. Unequal means upstream moved and the
contract tests in `internal/servarr` are asserting against a stale spec — re-vendor, re-run
`make test`, and record the new commit, date and hash rather than editing the old row in place.
Neither command needs a GitHub token; the REST API does, and is not required for this.

## Not yet vendored

Sonarr, Radarr, Lidarr, Readarr and Whisparr are listed in `docs/DEVELOPMENT.md` §7.2 and belong
here too. They are absent because no code consumes them yet, and a vendored spec with no contract
test behind it is a file that silently goes stale. Add each one with the client that reads it.

```
https://raw.githubusercontent.com/Sonarr/Sonarr/develop/src/Sonarr.Api.V3/openapi.json
https://raw.githubusercontent.com/Radarr/Radarr/develop/src/Radarr.Api.V3/openapi.json
https://raw.githubusercontent.com/Lidarr/Lidarr/develop/src/Lidarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Readarr/Readarr/develop/src/Readarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Whisparr/Whisparr/develop/src/Whisparr.Api.V3/openapi.json
```

⚠️ **The spec is not always right.** Where a controller enforces something the spec does not
declare, the controller wins — see `docs/DEVELOPMENT.md` §5 on `/api/v3/episode`, and note that
Prowlarr's `IndexerResource.status` is declared but is always null in practice.
