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

The `raw.githubusercontent.com/.../develop/...` URL serves the branch tip, so the file retrieved
on 2026-08-16 is the branch tip as of that date. The commit column records the commit the facts in
`docs/reference/arr-apis.md` §7 and `internal/servarr` were verified against; ⚠️ it was supplied
by the verification pass rather than read back from the GitHub API (the API was unreachable from
the build sandbox), so re-confirm it before citing it externally. The SHA-256 is the mechanical
identity of the vendored bytes and is what the drift job should compare.

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
