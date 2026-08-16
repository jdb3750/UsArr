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

Prowlarr version at that commit: **v2.6.2** (2026-08-12).

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
