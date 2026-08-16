# UsArr Development Guide

> **Status: pre-alpha. The first code has landed** — the Prowlarr Search-and-Grab path, the storage
> and security layers under it, and an embedded SPA shell — so the build, test and check workflows
> below are real and do run. Most of the *layout* is still contract rather than description:
> commands referencing files that do not exist yet are marked **(not yet)**.
>
> Roadmap shorthand: **v0.1** unified library + search · **v0.2** requests · **v0.3** cross-media ·
> **v0.4** gateway · **v1.0** breadth. `docs/ARCHITECTURE.md` §16 is authoritative.

---

## 1. Prerequisites

| Tool | Version | Why this floor | Install |
|---|---|---|---|
| **Go** | **1.25.13+** | Not 1.25.0, and not 1.25.7 either. Two independent floors push this up. (1) Dependencies: the Makefile pins goose `v3.27.3`, whose `go.mod` declares `go 1.25.7` — verified by execution, `go 1.25.0` fails with `github.com/pressly/goose/v3@v3.27.3 requires go >= 1.25.7`. (`ncruces/go-sqlite3` v0.35's `go 1.25.0` is the lower floor, not the binding one.) (2) **`make vuln` is the binding constraint**, and it is the higher of the two: on 1.25.7 `govulncheck` reports 15 *called* standard-library vulnerabilities — `crypto/tls`, `net/url`, `os`, `net/http` — with fixes spread across 1.25.8, 1.25.9 and 1.25.10; 1.25.12 still reports 5. There is no code-level workaround for a stdlib advisory; the only fix is the toolchain. 1.25.13 is the newest 1.25.x and scans clean. This floor is time-dependent — a new stdlib advisory moves it, so re-check with `make vuln` rather than trusting the number here, and treat `go.mod` as authoritative. | <https://go.dev/dl/> |
| **Node.js** | **22+ (LTS)** | SvelteKit 2 + Vite baseline. Verified in-container: `v22.22.2`. | <https://nodejs.org/> or `fnm`/`nvm` |
| **pnpm** | **10+** | Package manager for `web/`. Verified in-container: `10.33.0`. | `corepack enable && corepack prepare pnpm@latest --activate` |
| `git` | any recent | — | — |
| `make` | GNU make | The `Makefile` uses GNU-isms (`.PHONY`, `?=`, `define`). BSD make will not work. | — |

Installed by `make tools` **(not yet)**, all **version-pinned in the Makefile**:

| Tool | Purpose |
|---|---|
| `gofumpt` | Stricter `gofmt`. The formatting authority. |
| `golangci-lint` v2 | Meta-linter. **v2 config format** — `.golangci.yml` must start with `version: "2"`; v2 binaries cannot parse v1 configs. |
| `goose` | SQL migration runner (`github.com/pressly/goose/v3`). |
| `govulncheck` | Vulnerability scan. **Gating, and part of `make check`.** Not advisory. |
| `gitleaks` | Secret scan of the working tree. **Gating, and part of `make check`.** |

`@latest` is forbidden in the `Makefile`. `make tools` runs `go install`, which executes module build
logic on a machine that may hold a master key and *Arr admin credentials in `.env`; a floating
version there is a supply-chain hole, not merely a flaky gate.

**You do not need these on `$PATH`, and putting a different copy there cannot shadow them.** `go
install` writes to `$GOBIN` (or `$(go env GOPATH)/bin`), and every recipe invokes them from there by
absolute path, asserting `--version` against the pin first. See §9.

**There is no FFmpeg dependency, and there never will be.** UsArr does not transcode, remux or
otherwise process media — video routes to the backend that owns it, and the audio/ebook bytes UsArr
does carry on its own OpenSubsonic/OPDS surfaces are a plain `io.Copy` with `Range` handling
(`docs/ARCHITECTURE.md` §5.4). Do not add a media-processing
dependency, an `ffmpeg`/`ffprobe` shell-out, or a codec library. FFmpeg is also absent from the CI
container, so such a dependency breaks the build immediately — but the reason it is banned is
architectural, not environmental.

**No CGO, ever.** UsArr builds with `CGO_ENABLED=0` and produces a static binary.

### The SQLite driver, stated correctly

`ncruces/go-sqlite3` **no longer uses wazero.** It moved to the maintainer's own `wasm2go`
translator: the upstream SQLite C source is compiled to Wasm and then *translated to Go*, so the
shipped artifact is pure Go with no Wasm runtime in the graph. Verified against
`github.com/ncruces/go-sqlite3@v0.35.3`'s `go.mod` (2026-08-16), whose requires are
`github.com/ncruces/go-sqlite3-wasm/v3`, `julianday`, `sort`, `wbt` and `golang.org/x/sys` — **no
`tetratelabs/wazero` at any version**.

Three consequences, because earlier drafts of several documents got this wrong:

* Any claim that UsArr gets wazero "for free, already a dependency" is **false**. Adopting wazero for
  anything would be a new dependency with its own cost. (This is moot for plugins — a WASM tier is
  deferred, `docs/FUTURE.md` §1 — but it also removes the argument, so nobody should re-derive it.)
* "Bit-for-bit upstream SQLite behaviour" needs its own evidence under a compile-then-translate
  pipeline. Do not assert it; assert "the upstream C source, not a Go reimplementation", which is
  what is actually true and is the property that mattered.
* Memory behaviour is a **different** profile from a cgo driver, not necessarily a smaller one.
  Navidrome idling at ~50 MB is evidence about cgo SQLite, and it does not transfer. Measure before
  quoting an idle-RSS number: a one-day spike (500k-row fixture, WAL, the intended pragmas, arm64)
  belongs before the schema work, not after.

---

## 2. Repository layout (target)

```
UsArr/
├── cmd/
│   └── usarr/                  # main package. Flags, wiring, graceful shutdown, --env-file.
├── internal/
│   ├── config/                 # env + flag loading, two-level precedence (CONFIGURATION.md §1)
│   ├── crypto/                 # AES-256-GCM envelope encryption, key wrap/rotate, Argon2id
│   ├── db/
│   │   ├── migrations/         # NNNN_name.sql, goose format, embedded via embed.FS
│   │   └── sqlite.go           # open + PRAGMA journal_mode=WAL, busy_timeout, foreign_keys
│   ├── httpapi/                # REST handlers, middleware (auth, CSRF, rate limit, redaction)
│   ├── servarr/                # ONE client for Sonarr/Radarr/Lidarr/Readarr/Prowlarr/Whisparr
│   │   └── mapping/            # per-resource mapping onto the unified schema
│   ├── search/                 # unified search: SQLite FTS5 + a Go re-rank. No second engine.
│   ├── requests/               # v0.2 — request → route to the right *Arr by media type
│   ├── crossmedia/             # v0.3 — Wikidata edge resolution
│   ├── metadata/               # v0.2+ — tmdb, tvmaze, musicbrainz, openlibrary, wikidata
│   ├── navidrome/ audiobookshelf/ kavita/ komga/  # catalogue adapters, one milestone each after
│   │                                              # v0.1, in that order subject to the §16.1 probe
│   ├── jellyfin/                # v1.0 southbound adapter
│   ├── lazylibrarian/          # v0.3 as a Tier 1 YAML manifest (ARCHITECTURE §16); Go code only
│   │                           #   if the manifest ceiling is hit — cmd= RPC, HTTP 200 + Success:false
│   ├── tagging/                # namespaced derived tags
│   ├── ssrf/                   # outbound HTTP policy: DNS pinning, hop revalidation, caps
│   └── web/                    # embed.FS wrapper over web/build
├── api/
│   └── specs/                  # VENDORED upstream OpenAPI specs — see §7.2
│       ├── sonarr.v3.json  radarr.v3.json  prowlarr.v1.json
│       ├── lidarr.v1.json  readarr.v1.json whisparr.v3.json
│       └── SOURCES.md          # URL + commit/date each spec was pulled from
├── web/                        # SvelteKit, adapter-static, SPA fallback
│   └── build/                  # pnpm output -> embedded by internal/web (gitignored)
├── testdata/
│   ├── cassettes/              # go-vcr recordings, secrets scrubbed (§7.1)
│   └── releases/               # real release-name corpus for the parser tests
├── deploy/
│   ├── Dockerfile              # digest-pinned distroless base, non-root 65532
│   └── compose/
│       ├── usarr.yml           # UsArr alone
│       └── dev-stack.yml       # *Arr stack for RECORDING fixtures (§7.3). Never in CI.
├── docs/
├── Makefile
├── .env.example
└── .gitignore
```

There is **no `internal/plugin/`**. A WASM plugin tier is **deferred, not rejected**
(`docs/FUTURE.md` §1) and is on no milestone — a sandbox executing
third-party code inside the process that holds every one of the user's admin-grade API keys is a
security-critical subsystem this project cannot staff a review capacity for today. Service breadth comes
from compiled-in Go providers plus declarative YAML manifests, and the manifests are **not** "fully
sandboxed": a manifest is a server-side HTTP request generator that runs with the instance's stored
credential. Treat it accordingly.

Two rationales worth stating once, because they are decisions and not conventions.

**UsArr is northbound-thin and southbound-wide.** The media-server packages are read + handoff
adapters: they populate the unified library and produce a deep link. **None of them carry video
bytes, ever** — video links out to the backend's own client. Audio, ebook and comic bytes requested
through UsArr's *own* OpenSubsonic and OPDS surfaces go through the gateway's `io.Copy` proxy
(`docs/ARCHITECTURE.md` §5.4), never through an adapter. `internal/requests` is the write path.

**`internal/servarr` is one client, not six.** Sonarr, Radarr, Lidarr, Readarr, Prowlarr and Whisparr
are forks of one codebase; auth, paging, tags, commands, `/ping` and `system/status` are identical
contracts. What differs is the API version path (`/api/v3` vs `/api/v1`) and the resource shapes,
which belong in `mapping/`. Six clients would be six times the maintenance for one contract.

---

## 3. Getting running

```bash
git clone <repo> && cd UsArr
cp .env.example .env       # optional — every value has a working default

make tools                 # install the pinned toolchain      (not yet)
make dev                   # backend on :8484                  (not yet)
```

**Do not put a master key in `.env`.** On first run UsArr generates one into
`$USARR_CONFIG_DIR/keys/secret.key` at mode `0600` and logs a line saying so. That path has real
entropy and cannot accidentally be a string copied out of a public file. `.env.example` ships **no**
uncommented key for exactly that reason (`docs/CONFIGURATION.md` §3.2).

`.env` is **data, not shell**. `make dev` passes `--env-file .env` to the binary's own parser rather
than `source`-ing it, because bash performs expansion and command substitution while Docker Compose's
`env_file` parser does neither — the same file would otherwise mean two different things, on a file
that can hold the master key.

### Two-process dev loop (the normal one)

```bash
make dev            # Terminal 1 — Go backend, hot reload -> http://localhost:8484
make web-dev        # Terminal 2 — Vite with HMR -> :5173, proxies /api -> :8484
```

In this mode `internal/web` serves nothing. The SPA is baked in only by `make build`, which runs
`web-build` first and embeds `web/build`. **Consequence: `go build ./cmd/usarr` by hand produces a
binary that 404s on `/`.** The Makefile wires the dependency; running `go build` directly bypasses it.

---

## 4. Make targets

| Target | Does |
|---|---|
| `make dev` | Run the backend, loading `.env` through the binary's own parser. |
| `make web-dev` | SvelteKit dev server with HMR. |
| `make build` | `web-build`, then a static `CGO_ENABLED=0` binary at `./usarr` with the SPA embedded. |
| `make test` | `go test ./... -race -shuffle=on` plus `pnpm test`. No network, no Docker. |
| `make test-integration` | Behind the `integration` build tag. Needs a live stack; **never in CI**. |
| `make bench` | Wall-clock performance harness. A **release** gate on named hardware, never a merge gate. |
| `make bench-rss` | Memory harness: idle and peak process RSS over a 500k-row database, sweeping `cache_size`. §5. |
| `make lint` | `golangci-lint run`, `svelte-check`, `eslint`. |
| `make fmt` / `fmt-check` | Rewrite / verify formatting. |
| `make secrets` | `gitleaks dir .` over the working tree. **Gating.** |
| `make modverify` | `go mod verify` against `go.sum`. |
| `make vuln` | `govulncheck` + `pnpm audit`. **Gating.** The only step that touches the network. |
| `make migrate`, `migrate-new name=…` | Migration authoring against the dev DB. |
| `make docker` | Build the image. Digest-pinned base enforced; `--provenance` + `--sbom`. Needs a daemon — §8. |
| `make design` | `docs/design/check.mjs` — DESIGN-DIRECTION §13 made runnable: bans, token drift, contrast, overflow, row heights, roving tabindex, the webfont. Needs a browser; **not** part of `check`. |
| `make check-offline` | `fmt-check` + `lint` + `modverify` + `secrets` + `test`. Fully hermetic. |
| `make check` | **The pre-commit gate**: `check-offline` + `vuln`. |

`check` runs `fmt-check` (verify-only), not `fmt` (rewrite), so it never mutates your tree while
telling you it passed.

`check` makes **exactly one network call**, govulncheck's query to `vuln.go.dev`. That is a
deliberate exception to the otherwise-hermetic rule: a project that stores a dozen full-admin
credentials cannot ship a known-vulnerable crypto or HTTP dependency because the scan was advisory.
Use `check-offline` when you have no network; run `check` before you push.

`make design` is deliberately **outside** the gate, and the reason is not the obvious one. It is
hermetic — the mockups load over `file://` and the IBM Plex subsets are inlined as `data:` URIs, so
it makes no network call at all — and it finishes in about 40 seconds. What keeps it out is that it
needs a Playwright Chromium, a ~150 MB prerequisite the gate does not otherwise carry, and that
there is nothing to pin it to: `check.mjs` resolves Playwright at run time — the bare specifier
first, then `web/node_modules`, then the npm global root — so it runs wherever the module happens
to be installed, but no manifest declares which version. A gate step that accepts whatever is
installed is not a gate. It also guards `docs/design/` — prose, tokens and mockups, none of which
is a shipping artifact, and none of which can break the binary. Run it by hand when the design
moves, overriding `PW_BROWSERS_PATH` to point at your own browser cache. If Playwright ever lands
as a pinned `devDependency`, the browser cost is worth re-arguing on its own merits.

`docs/design/check.mjs` landed on `main` in `56101c1` (observed 2026-08-16). If you are on a branch
that predates it, `make design` says so and exits 1; if Playwright itself is missing, the script
prints the install command and every resolution path it tried.

---

## 5. Testing

### Layers

| Layer | Location | Network? | Runs in CI |
|---|---|---|---|
| Unit | `*_test.go` beside the code | No | Yes |
| Golden/table (parsers, mappers, tag rules) | `testdata/` | No | Yes |
| Replayed HTTP (go-vcr cassettes) | `testdata/cassettes/` | No | **Yes — the primary path** |
| Contract (vendored OpenAPI specs) | `api/specs/` | No | **Yes — the counterweight to stale cassettes** |
| Query-plan and row-count assertions | `internal/db` | No | Yes |
| Migration round-trip | `internal/db` | No | Yes |
| Frontend unit | `web/src/**/*.test.ts` (Vitest) | No | Yes |
| Integration (live services) | `//go:build integration` | Yes | **No** |
| Wall-clock benchmarks | `//go:build bench`, `make bench` | No | **No — release gate only** |

```bash
make test                                  # everything CI runs
go test ./internal/servarr/... -run Sonarr -v
go test ./... -race -coverprofile=cover.out && go tool cover -html=cover.out
USARR_INTEGRATION=1 make test-integration  # only with a live stack
```

Always `-race` for the backend. UsArr holds a webhook receiver and background sync jobs writing the
same SQLite handle; data races are the expected failure mode.

### Performance: what CI enforces and what it does not

**In CI, because they are deterministic and hardware-independent:**

* `EXPLAIN QUERY PLAN` assertions on every hot query. ~30 lines that catch an index regression
  forever. Assert the *plan string*, and fail on `SCAN` where a `SEARCH … USING INDEX` is required.
* **Row-count assertions** — "the library page query reads ≤ 200 rows", "resolving one northbound ID
  reads ≤ 3 rows". A better proxy for the thing being protected than wall-clock time, and it does not
  flake.
* Index-integrity assertions: `count(search_fts) == count(search_doc)`, no `search_doc` row whose
  `kind` is outside the indexed set.

**Not in CI, in `make bench`:** every p50/p99 latency budget. Enforcing millisecond gates needs either
a self-hosted single-point-of-failure runner or emulation whose numbers mean nothing, and latency
gates on shared CI are flake generators — the predictable outcome is that they get disabled in month
two, but only after blocking real work first. Record `make bench` output in `docs/BENCHMARKS.md`
with the hardware and commit named, and treat regressions as a release conversation.

### Memory: `make bench-rss`

```bash
make bench-rss                                       # 500k rows, 3 pragma cells, ~15 s + fixture
make bench-rss SPIKE_FLAGS='-rows=50000'             # fast smoke run
make bench-rss SPIKE_FLAGS='-rebuild'                # rebuild the fixture, remeasure the import peak
make bench-rss SPIKE_FLAGS='-cache=-2000'            # one cell
make bench-rss SPIKE_FLAGS='-mmap=134217728'         # re-check mmap; see the note below
```

**What it does.** Builds a 500k-row fixture through the **real `internal/db` open path** — so the
pragmas under test are the ones the binary actually sets, not a copy — then walks *idle → one pinned
read connection → the whole `NumCPU*2` read pool → a 10,000-row write burst*, sampling process RSS at
each step, once per `cache_size` cell. `mmap_size` is no longer swept by default — it left the pragma
list once this harness proved it inert — but `-mmap=...` still sweeps it on demand.

Three properties worth knowing before you read a number off it:

* **RSS, not `MemStats`.** `VmRSS` and `VmHWM` from `/proc/self/status`. SQLite's page cache is not on
  the Go heap; on the reference run the Go heap read 0.3 MB against 235 MB of RSS. On a platform
  without `/proc` every RSS column reads `n/a` and the output says the run measured nothing —
  deliberately, rather than substituting a heap figure that would be pasted into an ADR as if it were
  RSS.
* **One child process per cell.** `VmHWM` is a per-process high-water mark that never falls, so nine
  cells in one process would produce one peak and eight fictions.
* **Each cell gets its own copy of the fixture**, so a cell's write burst cannot bias the next.

**What to do with the output.** It prints a table and writes the same table to `.dev/rss-spike.md`
(gitignored, removed by `make clean`), formatted to paste into **ADR-0001** — which is where the
measurement lives, next to the budget it justifies. **Name the hardware.** Architecture, core count
and page size all move these figures: the read pool is `NumCPU*2`, and the page cache is
per-connection, so a result from one machine is not a result for another. The harness prints
`GOOS`/`GOARCH`/CPU count/page size/total RAM into its own header for exactly that reason.

**It is not in `make check` and must never be.** It is not in `make bench` either: that target runs Go
benchmarks, and this is a minutes-long measurement tool whose output is prose for an ADR. Both live
behind the `bench` build tag, so neither compiles into a normal build.

The current recorded result — **x86-64 only; arm64 is unmeasured** — is in ADR-0001, correction 3.
Two things it settled: `mmap_size` is a **no-op** under this driver (mmap is compiled out), and
`cache_size` is **per-connection**, so it multiplies by pool size rather than costing what it says.
Both defaults changed as a result (ADR-0001, amendment): `mmap_size` was dropped from the pragma list
and `cache_size` cut to `-8000`. That is a **memory-side** decision — this harness does not measure
query latency.

### Rules

* **A test that needs the network is not a unit test.** If it reaches the internet it is behind the
  `integration` tag or it uses a cassette.
* **Never commit a real API key.** Cassettes are scrubbed by a recorder hook before they touch disk
  (§7.1); `make secrets` is the mechanical backstop.
* **Test the error envelopes, not just the happy path.** This ecosystem's failure modes are unusual
  and are where integrations actually break:
  * *Arr: a bad key returns **401**, never 403 — the handler returns `NoResult()`, not `Fail()`.
  * *Arr content negotiation: `ReturnHttpNotAcceptable = true` is confirmed in the shipped Servarr
    `Startup.cs`, so `Accept: text/html` gets a 406. But the shipped specs declare several of the
    **largest** endpoints as `text/plain` only — `radarr /api/v3/movie`, `lidarr /api/v1/album` and
    `/track`, `readarr /api/v1/author`, `/book`, `/history/since`. Sending a bare
    `Accept: application/json` may therefore 406 the single most important endpoint in the import
    path. **Send `Accept: application/json, text/plain;q=0.9, */*;q=0.1` and parse the body as JSON
    regardless of the returned `Content-Type`**, and have the connection wizard record what each
    instance actually returns. ⚠️ Not yet exercised against a live Radarr.
  * *Arr child endpoints **require a parent id**. `GET /api/v3/episode` with none of
    `seriesId`/`episodeIds`/`episodeFileId` throws
    `BadRequestException("seriesId or episodeIds must be provided")` in `EpisodeController` — the
    OpenAPI spec marks them `required: false`, so the spec is *wrong* here and the controller is
    right. Same for `/api/v3/episodefile` and Radarr's `/api/v3/moviefile`. Any "never fetch children
    per-parent" rule is unimplementable; the achievable rule is one call per parent, 4–8 concurrent,
    jittered, behind a per-instance token bucket. Pin it with a test.
  * Prowlarr: query/grab limit breaches have historically returned **HTTP 200 with an error in the
    body**; upstream indexer 429s surface as generic connection failures. Treat Prowlarr search
    failures as soft and read `indexerstatus.disabledTill`.
  * Prowlarr: grabbable releases are cached server-side for **30 minutes**. A test that persists a
    release and grabs it later must assert the failure, not the success.
  * Prowlarr history is **indexer telemetry**, not entity change: `HistoryEventType` is
    `unknown|releaseGrabbed|indexerQuery|indexerRss|indexerAuth|indexerInfo`, and `HistoryResource`
    carries no `movieId`/`seriesId`. Never treat it as a delta-sync cursor.
  * LazyLibrarian: returns **HTTP 200 with `{"Success": false, "Error": {...}}`**. Inspect the body,
    never the status. Keys must be exactly 32 chars or you get `503 Invalid API key`.
  * Transmission: the first RPC call returns **409 + `X-Transmission-Session-Id`** and must be retried
    with that header. Bake the handshake into the client's test.
  * qBittorrent: cookie session that expires; assert re-login on 403.
  * Sonarr webhooks: `eventType: "Download"` has **two different body shapes** (single import vs
    import-complete). Discriminate on `episodeFile` vs `episodeFiles`. Both shapes need a fixture.
* **Type normalisation deserves its own tests, and the axis is `(app, resource)` — not app.**
  Verified against the vendored specs:

  | Field | Shape |
  |---|---|
  | `Sonarr.SeriesResource.imdbId` | `string` |
  | `Radarr.MovieResource.imdbId` | `string` |
  | `Sonarr.ReleaseResource.imdbId` | `string` |
  | `Radarr.ReleaseResource.imdbId` | **`int32`** |
  | `Prowlarr.ReleaseResource.imdbId` | `int32` |
  | `Radarr.MovieFileResource.indexerFlags` | `int32` bitmask |
  | `Radarr.ReleaseResource.indexerFlags` | **untyped in the spec — parse defensively as int or `string[]`** |
  | `Prowlarr.ReleaseResource.indexerFlags` | `string[]` |
  | `Quality.source` | Exists only in **Sonarr, Radarr and Whisparr**, a different enum in each (all begin with `unknown`; Whisparr adds `vr`). **Lidarr and Readarr's `Quality` carries only `{id, name}`.** Store `quality.id` + `quality.name` + the emitting app. |

  Also: `tags` are `int[]` in REST and **string labels** in webhooks; `Protocol` inside *Arr history
  `data` is the **integer** enum (`0` unknown, `1` usenet, `2` torrent) while every REST resource uses
  the string form. Each is a one-line bug and a one-line test.
* **Assert the SSRF policy, not just the happy fetch.** A handler that 302-redirects to
  `169.254.169.254` must be refused, and the test asserts the request never left the process.

---

## 6. Database and migrations

SQLite via `ncruces/go-sqlite3`, WAL mode, opened with (at minimum):

```
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;
```

`foreign_keys` is off by default in SQLite and is **per-connection**, so it must be set on every
connection in the pool, not once at startup.

⚠️ The SQLite floor is **3.43**, not 3.37. 3.37 is only what `STRICT` requires; `contentless_delete=1`
on an FTS5 table — without which a contentless index can never `DELETE` or `UPDATE`, so deleted works
haunt search results forever — arrived in **3.43.0**.

### Workflow

```bash
make migrate-new name=add_tag_rules
# -> internal/db/migrations/20260816T120000_add_tag_rules.sql
make migrate            # apply to the dev DB
make test               # migration round-trip test runs here
```

Rules:

* Migrations are **embedded** (`//go:embed migrations/*.sql`) and applied automatically at startup, so
  a binary can always bring its own database forward without an external tool.
* Migrations are **forward-only (proposed)**. Write a `-- +goose Down` block anyway — it is the
  cheapest way to test locally — but downgrades are not a supported user path, and
  `docs/CONFIGURATION.md` §6.3 says so.
* **Never edit a migration that has shipped.** Add a new one.
* SQLite's `ALTER TABLE` support is narrow. For anything beyond `ADD COLUMN`/`RENAME`, write the
  12-step table rebuild explicitly (create new → copy → drop old → rename), inside a transaction,
  with `PRAGMA foreign_keys=OFF` around it. Do not hope.
* The round-trip test runs every migration against an empty in-memory DB and asserts the result
  matches a checked-in `schema.sql` snapshot. This catches "works on my dev DB because it was created
  three migrations ago" drift.
* `cache.db` is a second database and is **never `ATTACH`ed inside a `usarr.db` write transaction** —
  cross-database locking reintroduces the `SQLITE_BUSY` class the single-writer design exists to
  avoid.

---

## 7. Developing with no *Arr stack at all

**This is the normal case, not the degraded one.** The project owner runs no Sonarr, Radarr or
Lidarr; CI has no Docker daemon and no FFmpeg. So the two offline mechanisms below are the *primary*
path, and the live stack exists only to record into them.

State it plainly so nobody spends an afternoon looking: **there is no public demo instance of Sonarr,
Radarr, Lidarr, Prowlarr, Readarr or Whisparr with API access.** Searched 2026-08-16; none official,
none community-run. Jellyfin publishes one (`https://demo.jellyfin.org/stable`, user `demo`, empty
password) and Navidrome is 📄 widely referenced at `demo.navidrome.org` (⚠️ unverified in this pass) —
both are for eyeballing response shapes by hand, never for automated tests: read-only, shared,
rate-limited by circumstance, and their uptime is nobody's commitment to you.

### 7.1 Recorded HTTP fixtures (go-vcr) — mechanism one

[`gopkg.in/dnaeon/go-vcr.v4`](https://github.com/dnaeon/go-vcr) records real HTTP interactions to a
YAML "cassette" and replays them offline. Every *Arr, media-server and metadata-provider client is
tested this way, with zero infrastructure.

Loop:

1. Point at a stack — the compose fixture (§7.3), or a contributor's homelab.
2. Run the test with `USARR_RECORD=1`. Real calls go out; a cassette lands in `testdata/cassettes/`.
3. Run it again without the flag. Replay. No network.
4. Commit the cassette. CI and every contributor now run that test with no stack at all.

**Scrubbing is mandatory and must be a recorder hook, not a manual step.** *Arr keys appear in three
places — the `X-Api-Key` header, an `Authorization: Bearer` header, and the `?apikey=` /
`?access_token=` query parameters. SABnzbd's key is *only* ever a query parameter. Jellyfin's token
sits inside a structured `Authorization: MediaBrowser Token="…"` header. Register a `BeforeSave` hook
rewriting all of them to a fixed placeholder, plus any `Set-Cookie` (qBittorrent's `SID`/`QBT_SID_*`)
and the `X-Transmission-Session-Id` value.

Cassette hygiene:

* One cassette per test, named after it. Cassettes are fixtures, not a shared corpus.
* Re-record when the upstream contract changes. A stale cassette passes forever against a contract
  that no longer exists — go-vcr's real failure mode, and §7.2 is the counterweight.
* Keep them small. Never record `GET /series` against a 5 000-item library: these endpoints are **not
  paged** and serialise the whole object graph. Seed a handful of items.
* Metadata providers get cassettes too, and there it is not optional — MusicBrainz allows 1 req/s and
  returns 503 above it, so a suite that hits it live is a suite that gets you blocked.

### 7.2 Contract tests against vendored OpenAPI specs — mechanism two

Every Servarr app ships an OpenAPI document in its repo. All six were retrieved during research
(2026-08-16) and must be vendored verbatim into `api/specs/` in an early commit, with a `SOURCES.md`
recording the URL and the date/commit pulled:

```
https://raw.githubusercontent.com/Sonarr/Sonarr/develop/src/Sonarr.Api.V3/openapi.json
https://raw.githubusercontent.com/Radarr/Radarr/develop/src/Radarr.Api.V3/openapi.json
https://raw.githubusercontent.com/Lidarr/Lidarr/develop/src/Lidarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Readarr/Readarr/develop/src/Readarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Prowlarr/Prowlarr/develop/src/Prowlarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Whisparr/Whisparr/develop/src/Whisparr.Api.V3/openapi.json
```

They must be vendored rather than fetched, because *Arr apps guard `app.UseSwagger()` behind
`if (BuildInfo.IsDebug)` — you **cannot** pull `/docs/v3/openapi.json` from a running production
instance.

Three uses, all offline:

1. **Schema validation of cassettes.** Replay a cassette and assert each recorded response body
   validates against the spec's schema for that path. This is what makes a stale cassette detectable
   without a live server: if the vendored spec moves and the cassette does not, the test fails.
2. **Struct-coverage tests.** Assert UsArr's Go structs cover every field the spec marks required for
   the resources UsArr consumes, and that enum-typed fields accept every documented value.
   `DownloadProtocol` (`unknown|usenet|torrent`) is identical across Sonarr, Radarr and Prowlarr and
   is the backbone of source tagging — pin it.
3. **Drift detection.** A scheduled job — *not* the PR gate, it needs network — re-downloads the
   specs, diffs them against `api/specs/`, and opens an issue on change. Upstream `develop` moves;
   UsArr should hear about a renamed field from a bot, not from a user's bug report.

⚠️ **The spec is not always right** (see `/api/v3/episode` in §5). Where a controller enforces
something the schema does not express, the contract test encodes the controller's behaviour and a
comment says why.

Caveats: Jellyfin publishes an OpenAPI spec, but the 10.11 schema was reported invalid (malformed 503
response headers), which breaks naive codegen — generate *types* if useful, but hand-write the ~25
endpoints UsArr needs. LazyLibrarian has no spec at all: a single CherryPy endpoint with 184 `cmd=`
values and an untyped envelope, so it gets hand-written fixtures and a hand-written contract test.

### 7.3 The compose fixture stack — for *recording only*, never CI

`deploy/compose/dev-stack.yml` **(not yet)** brings the ecosystem up on a developer workstation
(Sonarr 8989, Radarr 7878, Lidarr 8686, Prowlarr 9696, plus the media servers). ⚠️ Image names and
internal port mappings were not verified in this pass — check each project's own compose example
before committing the file.

The awkward part is **seeding**: a fresh *Arr has an empty library and a random API key.

**Seed with a script, never with committed config volumes.** An earlier draft proposed committing
seeded `config/` directories with "intentionally fake" API keys under `deploy/compose/seed/`. That is
now gitignored and the approach is rejected: a generated 32-hex key is indistinguishable from a real
one to a reviewer *and* to a secret scanner, so the first person who regenerates the seed from a
working stack commits a live full-admin credential, and `make secrets` cannot tell the difference.

Instead, `make seed` **(not yet)** drives each app's own API after startup:

* Write a fixed, structurally-obvious fake key into `config.xml` before first start —
  `0000000000000000000000000000dead`. Its element names are stable (`ApiKey`, `Port`, `UrlBase`,
  `AuthenticationMethod`, `AuthenticationRequired`), so this is scriptable, and the constant is
  exact-matchable in a `.gitleaksignore` entry.
* Add a fixed handful of series/movies through the API.
* The result is regenerable rather than a binary blob, and nothing under `deploy/compose/` is ever
  committed.

**This stack never runs in CI.** See §8.

### 7.4 Fake servers, for what a cassette cannot reach

`httptest.Server` implementations of the handful of endpoints UsArr calls. These prove nothing about
the real world, so use them only where reality is hard to provoke: auth failures, 406 on a wrong
`Accept`, timeouts, truncated bodies, connection resets, Transmission's 409 challenge, qBittorrent
session expiry, Prowlarr's HTTP-200-with-an-error body, LazyLibrarian's `Success: false`, and the
SSRF redirect cases.

---

## 8. CI: no Docker daemon, no FFmpeg, one network call

**The Docker daemon is unavailable in the CI/agent container.** Verified: `docker info` fails. FFmpeg
is absent too, and stays absent.

Therefore:

* `make test`, `make lint`, `make check-offline` and `make build` **must not require Docker, a live
  service, or network access.** If any of them ever does, that is a bug in the target, not an
  environment problem to work around.
* `make check` adds exactly one network call — govulncheck against `vuln.go.dev`. Nothing else in the
  gate touches the network, and no target may add a second without a stated reason.
* Testcontainers, dockertest and "spin up a real Postgres/Sonarr in a test" are **out of bounds** for
  the default suite. This is also why UsArr uses SQLite rather than anything needing a server.
* Anything requiring the stack goes behind `//go:build integration`, is gated on
  `USARR_INTEGRATION=1`, and runs only under `make test-integration` on a developer machine.
* `make docker` exists for humans and for release pipelines that do have a daemon. It is never part
  of `check` and CI must not call it.

A `make check-offline` that passes on a machine with no daemon, no *Arr stack and no network is the
bar. If you cannot get a test to that bar, it belongs behind the integration tag.

---

## 9. Lint and format

### Go

```bash
make fmt          # gofumpt -l -w .   — stricter superset of gofmt; the formatting authority
make fmt-check    # gofumpt -l .      — the gating form, used by `make check`
make lint-go      # golangci-lint run — v2; .golangci.yml MUST declare version: "2"
```

**Run these through `make`, not by hand.** `make tools` installs the pinned binaries into `$GOBIN`,
which is usually *not* on `$PATH`, so a bare `golangci-lint run` in your shell runs whichever copy
the machine happens to carry — not the pinned one. The recipes invoke every pinned tool by its
absolute path under `$(GOBIN_DIR)` and assert `--version` against the pin before running it. If you
must invoke one directly, use the full path: `$(go env GOPATH)/bin/golangci-lint run`.

This is not a style preference. The gate spent the project's whole life so far resolving
`golangci-lint` off `$PATH`, where it found a system-wide **v2.5.0** while the `Makefile` pinned
**v2.12.2** — and reported `check: OK` the entire time, because the older gosec had no G123 or G124
to fire. See `docs/reference/security.md` §7 and the block above `require_tool` in the `Makefile`.
`gitleaks` is the one tool with no version assertion: installed by `go install` it reports
"version is set by build process", because upstream stamps the real version with `-ldflags` at
release time. It gets the existence guard alone, which is exactly why the absolute path rather than
the assertion is the primary mechanism.

`.golangci.yml` — v2 format, `linters.default` replaces v1's `enable-all`/`disable-all`. This is a
summary; the file itself is the source of truth:

```yaml
version: "2"
linters:
  default: standard
  enable:
    - errcheck        # unchecked errors are how credential-handling bugs hide
    - govet
    - staticcheck
    - ineffassign
    - unused
    - bodyclose       # UsArr makes a LOT of outbound HTTP; leaked bodies leak sockets
    - noctx           # every outbound request must carry a context with a timeout
    - errorlint
    - gosec
    - sqlclosecheck
    - rowserrcheck
  exclusions:
    rules:
      - path: _test\.go     # fixture credentials and deliberately malformed inputs
        linters: [gosec]
issues:
  max-same-issues: 0        # 0 = unlimited. See below — the defaults hide findings.
  max-issues-per-linter: 0
formatters:
  enable:
    - gofumpt
    - goimports
```

`noctx` and `bodyclose` are not stylistic here. UsArr fans out to a dozen services; a request without
a timeout or a body that is never closed becomes a hung dashboard.

**The `issues:` block is load-bearing.** golangci-lint defaults to `max-same-issues: 3` and
`max-issues-per-linter: 50`, and it drops the remainder *silently* — no "N more" line, nothing in the
summary. This repo carried 7 `noctx` findings that a default run displayed as 3, so clearing the
three on screen would have looked like finishing the job. Do not restore the defaults to shorten the
output.

### Frontend

```bash
pnpm -C web lint          # eslint
pnpm -C web format        # prettier --write
pnpm -C web check         # svelte-check (types + a11y + unused CSS)
```

`svelte-check` is part of `make lint`, not an afterthought — with `adapter-static` a type error in a
route only surfaces at build time otherwise.

### Pre-commit

```bash
make check
```

Wire it as a git hook if you like; do not wire it as a hook that auto-formats, because a hook that
rewrites your files mid-commit produces commits you did not read.

---

## 10. Conventions

* **Errors**: wrap with `%w` and context
  (`fmt.Errorf("sonarr %s: fetch series: %w", inst.Name, err)`). Never log a secret in an error path —
  the *Arr key is often in the URL you are tempted to include.
* **Contexts**: every outbound call takes a `context.Context` with a deadline.
* **Outbound HTTP goes through `internal/ssrf`**, never `http.DefaultClient`. That package resolves
  the hostname, validates the IP against the policy **for that request's class**, dials the pinned IP
  while keeping the original hostname for `Host`/SNI/certificate verification, revalidates on **every**
  hop of a redirect (max 3, no cross-scheme, no downgrade to `http`, never carrying credentials
  across), denies non-HTTP(S) schemes, and caps response size and time. Bypassing it is a
  review-blocking change.
  There are **three** request classes, not two, and the third is the biggest:
  1. **configured** — an admin-entered service URL. May reach private space; that is the whole point
     of a homelab hub.
  2. **provider** — a metadata/artwork URL for a known public provider. Strict public-only deny-list.
  3. **derived** — *a URL read out of an upstream response body*: `MediaCover.remoteUrl`, a media
     server's cover field, a manifest's `posterUrl`. **No admin configured these.** Policy: strict
     public-only deny-list, *unless* the URL is host-and-port-identical to the originating instance's
     `base_url`, in which case it is an integration fetch to that one host. Nothing in between. This
     is the shape of CVE-2021-29490 with the attacker moved one hop upstream, and it is the class
     that carries indexer- and NFO-supplied strings.
* **Credentials never appear in a stored URL.** Strip credential query parameters before writing any
  URL to `image_asset.source_url`, to the HTTP cache, or to a log. Derive cache keys from the
  stripped URL so rotating a provider key does not invalidate the whole image cache. TMDB v3,
  Fanart.tv and Comic Vine all authenticate by query parameter, and those columns are not encrypted.
* **UsArr never invokes a media processing tool.** No `exec.Command("ffmpeg", …)`, ever.
* **UsArr never carries video bytes, and never transcodes anything.** Video is catalogued and
  deep-linked. Audio, ebooks and comics on UsArr's own northbound surfaces are proxied with a plain
  `io.Copy` and correct `Range`/`206` handling, because those protocols have no link-out affordance
  (`ARCHITECTURE.md` §5.4, ADR-0017). There is no redirect mode and no `USARR_STREAM_MODE`.
* **Never join on an *Arr's local `id`.** Those integers are per-instance and unrelated across
  instances — the same is true of *Arr tag IDs, where instance A's tag `3` has nothing to do with
  instance B's tag `3`. Join on external IDs (tvdb/tmdb/imdb/MBID/OLID/ISBN) or on
  `(instance_id, remote_kind, remote_id)`. Note the `remote_kind`: Sonarr series 42 and episode 42
  both exist, so a two-part key is ambiguous.
* **`id` reuse is real.** *Arr primary keys are not `AUTOINCREMENT`, so a deleted movie's id is
  handed to the next one. Before resurrecting a tombstoned link, compare a stable external identity;
  on mismatch, hard-delete the tombstone and create a new link rather than rebinding the old work.
* **Store raw release names verbatim, forever.** Every parsed field is re-derivable if the parser
  improves; the raw name is not recoverable once discarded.
* **Do not write a release-name parser from scratch.** Port or vendor an existing one and use
  Sonarr's regex corpus (158 regexes in `Parser.cs`) and its test fixtures as the oracle.
* **Commits**: imperative subject, ≤72 chars, body explains *why*.

---

## 11. Onboarding an AI agent

An agent working in this repo should assume:

* `make check` is the definition of done for a code change. It must pass with no Docker and no live
  services; `make check-offline` must pass with no network at all.
* No Docker. No stack. If a task seems to need either, the task is misframed — reach for a cassette
  (§7.1) or the vendored spec (§7.2).
* `docs/CONFIGURATION.md` is the contract for anything config-shaped, and §5 of it is authoritative
  for on-disk layout for the whole project. A new setting goes in the §2 table, in `.env.example`, and
  in `internal/config` in the same commit — one that exists in two of the three is a bug.
  **Adding an environment variable is a decision to defend, not a default.** The surface is
  deliberately fourteen variables; services are configured in the UI and stored in the database, and
  there is no env channel, no config file and no provisioning file for them.
* Facts about upstream APIs come from `api/specs/` or the research notes, **not from memory** — and
  where the spec and the controller disagree, from a test that says which won. This ecosystem has
  many near-miss details (`/api/v1` vs `/api/v3`, `imdbId` string vs int *within one app*, 401 vs
  403, PascalCase webhook `eventType` against camelCase REST) that are easy to get plausibly wrong.
* When something is unverified, say so in the comment or the doc, with the date. This repo marks
  uncertainty rather than papering over it, and reviewers rely on that.

### Working alongside other threads

Several threads work this repo in parallel, on branches cut from the same base. The collisions that
result are usually **semantic rather than textual**: a merge can go through cleanly and still ship a
paragraph describing a repo that no longer exists.

* **Merge to `main` early and in small batches.** Direct merges are fine at this stage; no PR is
  required. Batch size is the real control — a branch sitting twenty-odd commits behind is what turns
  a trivial collision into a semantic one.
* **Announce before pushing** an edit to a shared document outside the area you lead, both in your own
  thread and to the project coordinator, so the other side hears about it before building on text you
  are about to change.
* **`ARCHITECTURE.md` §16 carries two different kinds of edit.** *Scope* — which milestone a thing
  lands in — is owned by the ADRs, and §16 is authoritative for it. *Status* — what has actually
  landed — is a separate question. Do not change one while intending the other.

Who leads which area, roughly:

| Area | Led by |
|---|---|
| Go and Svelte source: `internal/`, `cmd/`, `web/` | the implementation work |
| implementation-status wording in `CLAUDE.md`, `README.md` and `ARCHITECTURE.md` §16 | the implementation work |
| `ARCHITECTURE.md` §17 and `docs/design/` | the design work |
| `docs/PROJECT-INSTRUCTIONS.md` | the instructions gatekeeper |

**These are leads, not exclusive ownership**, because in practice edits cross the lines routinely.
`README.md`, `CLAUDE.md` and `ARCHITECTURE.md` are shared documents; a §17 change routinely lands
§8.x amendments alongside it; and `docs/reference/` follows whichever change drove it. The map says
who to talk to, not who is permitted to type.
