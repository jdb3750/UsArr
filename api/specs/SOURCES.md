# Vendored upstream API specifications

These files are **vendored verbatim**, never fetched at build or test time.

Two reasons, both hard constraints:

1. *Arr apps guard `app.UseSwagger()` behind `if (BuildInfo.IsDebug)`, so a production
   instance does **not** serve `/docs/vN/openapi.json`. You cannot pull the spec from a running
   instance, and there is no public demo instance of any Servarr app with API access
   (searched 2026-08-16 — none official, none community-run).
2. CI has no network. The contract tests in `internal/servarr` read these files directly.

Drift detection is a **scheduled job, not the PR gate**: it needs network, re-downloads each
spec, diffs it against this directory, and opens an issue on change. Upstream `develop` moves;
UsArr should hear about a renamed field from a bot, not from a user's bug report.

## Files

| File | Upstream URL | Branch | Commit | Retrieved | SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `prowlarr.json` | `https://raw.githubusercontent.com/Prowlarr/Prowlarr/develop/src/Prowlarr.Api.V1/openapi.json` | `develop` | `1f7db1e` | 2026-08-16 | `efe3dfb9a928658d8a1f2f307a965fb1275bad2853012a2f9bdc2404215d0fbb` |
| `kavita.json` | `https://raw.githubusercontent.com/Kareadita/Kavita/develop/openapi.json` | `develop` | `9c3e540` | 2026-08-17 | `3bd8363f0a4e847bf159127dbd9f9972725a5a0efb8d69831d2513ed3bab946d` |

Prowlarr version at that commit: **v2.6.2** (2026-08-12).
Kavita version declared inside `kavita.json`: **`info.version` = `0.9.0.20`**, and the same string
appears in `info.description` as *"Built against v0.9.0.20"*. 900763 bytes, `openapi: 3.0.4`,
488 paths.

**Kavita is different from every \*Arr in one way that matters here: the spec is a checked-in file,
not a debug-only endpoint.** `openapi.json` sits at the repository root and is generated from
`Kavita.Server/Startup.cs`'s Swashbuckle configuration, so it is fetchable without a running
instance. That does not change the vendoring rule — CI still has no network and the contract tests
in `internal/kavita` still read the file directly — but it does make the drift job cheaper for this
one spec.

⚠️ **Two documents call this file `kavita-openapi.json`** — `ARCHITECTURE.md` §7.1a and
`RESEARCH.md`, both of which say a claim was *"read from the vendored specs"* while no Kavita spec
was in this directory until 2026-08-17. The file is named `kavita.json` to match this directory's
own convention (`prowlarr.json`), and the pointer is recorded here rather than by editing two
documents this thread does not own. **The §7.1a claim itself re-verified clean against the bytes
vendored here**: exactly two operations across all 488 paths carry a parameter matching
`since|modified|updated|changed`, and they are `GET /api/Collection` and
`POST /api/ReadingList/lists`, both named `sortByLastModified` — neither on Series, Volume or
Chapter, exactly as §7.1a states.

### ⚠️ `kavita.json` is a branch ahead of the deployment, and further ahead than it looks

Same shape as the Prowlarr warning below, one degree worse. The vendored file tracks **`develop`**
at `info.version` **0.9.0.20**. The owner's instance — the only real Kavita this project has ever
talked to — runs **stable v0.9.0.2** (observed 2026-08-17, ADR-0035 §2a).

**The skew is not only the version string.** Measured 2026-08-17 by fetching the same file at tag
`v0.9.0.2`: that copy declares `info.version` **0.9.0.0** and has **462 paths**, against develop's
**488**. So the checked-in spec lags even its own release tag, and the vendored develop copy is 26
paths ahead of the stable line. **A green contract test here is evidence about `develop`, and the
owner's server is two steps away from it.** Every fact `internal/kavita` relies on was therefore
checked against Kavita's *controller source* at the pinned commit as well as against this document —
`Kavita.Server/Controllers/*.cs`, `Kavita.Common/Helpers/UserParams.cs`,
`Kavita.Server/Extensions/HttpExtensions.cs` — because, as `DEVELOPMENT.md` §5 puts it for the
\*Arrs, the controller wins over the schema.

**Record the Kavita version on every cassette — and it is NOT free here.** Kavita sends no
version response header: `Kavita.Common/Constants/Headers.cs` declares `x-kavita-version`, but the
only custom header `Startup.cs` puts on an `/api/*` response is `Pagination` (exposed via CORS at
lines 305 and 314). The version has to be *asked for*, at `GET /api/Server/server-info-slim`
(admin-only) or `GET /api/Plugin/version?apiKey=` (anonymous, and it puts the credential in the
query string). Record it in the cassette file's header comment, by hand.

**Observed 2026-08-17 18:30 UTC:** `refs/heads/develop` resolved to
`9c3e5400007f8a0282f7d883f2ad5e71716e514d`, and the `openapi.json` that URL served hashed to the
SHA-256 in the table — byte-identical to the vendored copy, `diff` clean, 900763 bytes. As with the
Prowlarr row: that is a measurement at a moment, not a standing property.

Re-check it the same way:

```bash
git ls-remote https://github.com/Kareadita/Kavita develop     # -> the commit column
curl -sS https://raw.githubusercontent.com/Kareadita/Kavita/develop/openapi.json \
  | sha256sum                                                 # -> must equal the SHA-256 column
```

## ⚠️ The vendored spec is a branch ahead of the deployment

`prowlarr.json` tracks **`develop`**, because `develop` is where the API is defined and it is the
right thing to pin. The only known real deployment runs **stable 2.5.2.5491** (observed 2026-08-16)
— a minor version behind. Nothing here is wrong, but it bounds what the contract tests prove: **a
green contract test is evidence about `develop`, not evidence about the server the owner is actually
talking to.** A real instance is the only evidence about a real instance. This is not hypothetical —
a grab failure came from exactly this gap in another form, where the test fake was written from the
spec and so inherited the spec's silence, validating nothing.

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
