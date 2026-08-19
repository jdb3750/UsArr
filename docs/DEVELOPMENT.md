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

Installed by `make tools`, all **version-pinned in the Makefile**. This target **works and is
required** — `make check` invokes all five by absolute path and fails closed if one is missing, so
run it before your first `make check` on a new machine:

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

**Measured 2026-08-16**, in this repo's agent container: `make tools` installed all five into
`/root/go/bin` and they answer at the pinned versions — `gofumpt v0.11.0 (go1.25.13)`,
`golangci-lint has version 2.12.2`, `goose version: v3.27.3`, `govulncheck@v1.7.0`, and
`go version -m /root/go/bin/gitleaks` reporting `github.com/zricethezav/gitleaks/v8 v8.30.1`
(gitleaks cannot answer `--version`; §9 explains why its identity comes from the build info
instead). An earlier revision of this line marked the target **(not yet)**, which was stale.

**There is no FFmpeg dependency, and there never will be.** UsArr does not transcode, remux or
otherwise process media — video routes to the backend that owns it, and the audio/ebook bytes UsArr
does carry on its own OpenSubsonic/OPDS surfaces are a plain `io.Copy` with `Range` handling
(`docs/ARCHITECTURE.md` §5.4). Do not add a media-processing
dependency, an `ffmpeg`/`ffprobe` shell-out, or a codec library. FFmpeg is also absent from the agent
container this project builds in (§8), so such a dependency breaks the build immediately — but the
reason it is banned is architectural, not environmental.

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
│   ├── bookorbit/                  # v0.1's catalogue source as of ADR-0052
│   ├── navidrome/ audiobookshelf/ kavita/ komga/  # catalogue adapters. This read "one milestone
│   │                               #   each after v0.1, in that order subject to the §16.1 probe";
│   │                               #   ADR-0041 moved kavita/ INTO v0.1, then ADR-0052 moved v0.1 off
│   │                               #   it onto bookorbit/. kavita/ is SUNSET, NOT DELETED — it stays
│   │                               #   here and stays green — and it is not added back to the
│   │                               #   after-v0.1 sequence, which is navidrome/, audiobookshelf/,
│   │                               #   komga/ in that order. The adapter that drives a client like
│   │                               #   kavita/ lives in libsync/, not beside it.
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
│   ├── update.sh               # ff-only pull -> make build -> install -> restart -> verify (§12.1)
│   ├── status.sh               # read-only "am I current?" (§12.1.1)
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

**In this order, and it is the whole list.** Nothing here is optional except the line that says so,
and nothing else has to be run first — in particular `make check` no longer needs a `make web-deps`
in front of it, which it did until `fmt` and `fmt-check` were given the `web-deps` prerequisite the
other web targets already had (`REVIEW-LOG.md` FI-02, verified from a fresh clone on 2026-08-16).

```bash
git clone <repo> && cd UsArr
cp .env.example .env       # optional — every value has a working default

make tools                 # REQUIRED FIRST. Installs the pinned toolchain into $GOBIN.
make check                 # the gate. Installs web/node_modules itself on the way through.
make dev                   # backend on :8484
```

`make tools` is not a convenience. Every recipe invokes its pinned binary by **absolute path** under
`$GOBIN` and asserts `--version` against the pin before running it, so a copy on `$PATH` does not
satisfy it and cannot shadow it; without this step the first gate target stops with `missing pinned
tool: …` / `run: make tools`. Verified from a fresh clone on 2026-08-16 — it installs all five and
exits 0.

**The first `make check` is the slow one**, and that is expected rather than a symptom: it runs
`pnpm install --frozen-lockfile`, builds the SPA, and makes the two network calls in `vuln`. Later
runs reuse both caches.

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

### `go test ./...` in a fresh clone skips the SPA assertions — it no longer fails

`internal/web/spa` is the `//go:embed` mirror of the built SPA, and `git ls-files internal/web/spa`
tracks exactly one file — `.gitkeep`. It is populated by `web-build` (a prerequisite of both `build`
and `test-go`) and by nothing else, so a clone that has never built the frontend has no document for
a route test to find.

Every assertion that needs one is guarded by `web.Built()` and **skips**, naming the fix:

```bash
make test        # correct: builds the SPA first, then runs the Go and web suites
go test ./...    # green in a fresh clone; the SPA assertions skip and say why
```

```
--- SKIP: TestUnauthenticatedAndURLBase/SPADeepRoute
    internal/web: no frontend build is embedded: internal/web/spa is empty, so no route
    can return the SPA document. Run `make build` (or `make test`, which builds it too)
    and re-run; see docs/DEVELOPMENT.md §3.
```

**The skip is not allowed to become permanent.** `USARR_REQUIRE_WEB_BUILD=1` turns it into a hard
failure, and `make test-go` sets it precisely because it has just run `web-build`. So inside `make
check` these assertions **execute**; only outside it may they skip. Two guards carry this, written
the same way: `requireBuilt` in `internal/web/web_test.go` — which protects
`TestEmbeddedFSCarriesAppDir`, the only thing that catches a lost `//go:embed all:` prefix — and
`requireSPABuilt` in `cmd/usarr/e2e_test.go`.

**Verified by execution on a fresh clone, 2026-08-16.** Before the guard, `CGO_ENABLED=1 go test
-race ./...` failed in exactly one package with exactly one assertion — `cmd/usarr`,
`e2e_test.go:337: /usarr/search = 404, want the SPA document`; every other package, `internal/web`
among them, was `ok`. After it, all packages are `ok` with that one subtest skipped, and the same
tree under `USARR_REQUIRE_WEB_BUILD=1` fails — which is the guard proving it still has teeth. This
closes `FI-12` in `docs/REVIEW-LOG.md` as coded rather than as documented.

---

## 4. Make targets

| Target | Does |
|---|---|
| `make dev` | Run the backend, loading `.env` through the binary's own parser. |
| `make web-dev` | SvelteKit dev server with HMR. |
| `make build` | `web-build`, then a static `CGO_ENABLED=0` binary at `./usarr` with the SPA embedded. |
| `make test` | `go test ./... -race -shuffle=on` plus `pnpm test`. No network, no Docker. |
| `make test-integration` | Behind the `integration` build tag. Needs a live stack; **never in CI**. |
| `make spec-drift` | Behind the `upstream` build tag. Are the vendored OpenAPI specs still what upstream serves? **Needs network; never in `check`.** §7.2. |
| `make bench` | Wall-clock performance harness. A **release** gate on named hardware, never a merge gate. |
| `make bench-rss` | Memory harness: idle and peak process RSS over a 500k-row database, sweeping `cache_size`. §5. |
| `make lint` | `golangci-lint run`, `svelte-check`, `eslint`. |
| `make fmt` / `fmt-check` | Rewrite / verify formatting. |
| `make secrets` | `gitleaks dir .` over the working tree. **Gating.** |
| `make modverify` | `go mod verify` against `go.sum`. |
| `make vuln` | `govulncheck` + `pnpm audit`. **Gating.** The only step that touches the network. |
| `make migrate`, `migrate-new name=…` | Migration authoring against the dev DB. |
| `make deploy` | Wrapper for `deploy/update.sh`: ff-only pull, `make build`, install, restart, then verify the **running** process is that commit. §12.1. |
| `make deploy-status` | Wrapper for `deploy/status.sh`: read-only "am I current?" across checkout, installed binary and running process. §12.1.1. |
| `make docker` | ⚠️ **Cannot succeed today: it builds `-f deploy/Dockerfile`, and that file is not in the tree.** Intended shape: digest-pinned base enforced, `--provenance` + `--sbom`. Also needs a daemon — §8. To deploy, build and install the binary instead: §12. |
| `make design` | `docs/design/check.mjs` — DESIGN-DIRECTION §13 made runnable: bans, token drift, contrast, overflow, row heights, roving tabindex, the webfont. Needs a browser; **not** part of `check`. |
| `make build-tagged` | `go build -tags=bench ./...`. **Gating.** The packages `go list ./...` cannot see — `internal/db/spike` is behind `//go:build bench`, so a type error in it passed the entire gate until this step existed. |
| `make check-offline` | `fmt-check` + `lint` + `build-tagged` + `modverify` + `secrets` + `test`. Fully hermetic. |
| `make check` | **The pre-commit gate**: `check-offline` + `vuln`. |

`check` runs `fmt-check` (verify-only), not `fmt` (rewrite), so it never mutates your tree while
telling you it passed.

`check` makes **exactly two network calls, both to vulnerability databases**: govulncheck's query to
`vuln.go.dev`, and `pnpm audit`'s to the npm registry. Both are in the `vuln` target, and
**`check-offline` drops both** — it is precisely `check` minus that target. They are a deliberate
exception to the otherwise-hermetic rule: a project that stores a dozen full-admin credentials
cannot ship a known-vulnerable crypto, HTTP or frontend dependency because the scan was advisory.
Use `check-offline` when you have no network; run `check` before you push.

This said "exactly one network call" for the life of the project while the `vuln` target ran two
commands, in three places at once — here, `Makefile`'s honesty notice, and `CLAUDE.md`. Nobody
counted; everybody copied. §11 has the general rule.

`make design` is deliberately **outside** the gate, and the reason is not the obvious one. It is
hermetic — the mockups load over `file://` and the IBM Plex subsets are inlined as `data:` URIs, so
it makes no network call at all — and it finishes in about 40 seconds. What keeps it out is that it
needs a Playwright Chromium, a **~150 MB prerequisite the gate does not otherwise carry**, and on
its own that is still enough. It also guards `docs/design/` — prose, tokens and mockups, none of
which is a shipping artifact, and none of which can break the binary. Run it by hand when the design
moves, overriding `PW_BROWSERS_PATH` to point at your own browser cache.

**The "nothing to pin" half of that argument is now partly discharged, and the Makefile is
authoritative for where it stands.** `web/package.json` pins `playwright-core` at `1.56.1` exactly,
no caret; it declares no install script and downloads no browser, so a fresh install pays nothing
for it. **What is not closed:** `check.mjs`'s resolution ladder asks for the specifier `playwright`,
and the pinned package is `playwright-core`, so the pin does not satisfy the ladder and a fresh
machine still falls through to the fallbacks. Finishing that is a change to `docs/design/check.mjs`.
Until then the version is declared but not enforced, and a gate step that accepts whatever is
installed is still not a gate.

`docs/design/check.mjs` is on `main`: introduced in `f015655`, merged by `e0d4b26`. **Observed
present at 2026-08-16 17:34 UTC after a `git fetch`** — a measurement, not a standing guarantee, and
the sort of claim that goes stale silently. If you are on a branch that predates it, `make design`
says so and exits 1; if Playwright itself is missing, the script prints the install command and every
resolution path it tried.

---

## 5. Testing

### Layers

The **In `make test`** column is also the answer to "would a CI run this" — there is no CI today
(§8), so `make test` *is* the merge-gate suite, and a CI added later inherits this table unchanged.

| Layer | Location | Network? | In `make test` |
|---|---|---|---|
| Unit | `*_test.go` beside the code | No | Yes |
| Golden/table (parsers, mappers, tag rules) | `testdata/` | No | Yes |
| Replayed HTTP (go-vcr cassettes) | `testdata/cassettes/` | No | **Yes — the primary path** |
| Contract (vendored OpenAPI specs) | `api/specs/` | No | **Yes — the counterweight to stale cassettes** |
| Query-plan and row-count assertions | `internal/db` | No | Yes |
| Migration round-trip | `internal/db` | No | Yes |
| Frontend unit | `web/src/**/*.test.ts` (Vitest) | No | Yes |
| Integration (live services) | `//go:build integration` | Yes | **No** |
| Upstream spec drift | `//go:build upstream`, `make spec-drift` | Network only | **No — news, not a gate** |
| Wall-clock benchmarks | `//go:build bench`, `make bench` | No | **No — release gate only** |

```bash
make test                                  # the whole merge-gate suite
go test ./internal/servarr/... -run Sonarr -v
go test ./... -race -coverprofile=cover.out && go tool cover -html=cover.out
USARR_INTEGRATION=1 make test-integration  # only with a live stack
```

Always `-race` for the backend. UsArr holds a webhook receiver and background sync jobs writing the
same SQLite handle; data races are the expected failure mode.

### Performance: what the gate enforces and what it does not

There is no CI (§8), so "the gate" means `make check` — required by `CLAUDE.md` before any commit,
run by hand. The split below is the one a CI would inherit if one is ever added.

**In the gate, because they are deterministic and hardware-independent:**

* `EXPLAIN QUERY PLAN` assertions on every hot query. ~30 lines that catch an index regression
  forever. Assert the *plan string*, and fail on `SCAN` where a `SEARCH … USING INDEX` is required.
* **Row-count assertions** — "the library page query reads ≤ 200 rows", "resolving one northbound ID
  reads ≤ 3 rows". A better proxy for the thing being protected than wall-clock time, and it does not
  flake.
* Index-integrity assertions: `count(search_fts) == count(search_doc)`, no `search_doc` row whose
  `kind` is outside the indexed set.

**Not in the gate, in `make bench`:** every p50/p99 latency budget. Enforcing millisecond gates needs
either a self-hosted single-point-of-failure runner or emulation whose numbers mean nothing, and
latency gates on shared CI runners are flake generators — the predictable outcome is that they get
disabled in month two, but only after blocking real work first. This is also the standing answer to
"should the CI we add enforce latency": no. Record `make bench` output in `docs/BENCHMARKS.md`
with the hardware and commit named, and treat regressions as a release conversation.

### Browser-driven frontend checks: `pnpm bench:list` and `pnpm test:freeze`

```bash
cd web
PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers pnpm bench:list   # the List primitive (ADR-0029)
PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers pnpm bench:list -- --quick
PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers pnpm test:freeze  # freeze-while-aimed (ADR-0038)
```

`vitest.config.ts` is `environment: 'node'`, so **the unit run cannot import a rune component and
has no DOM at all**. Anything whose behaviour is decided by layout, hit testing, a physical pointer
coordinate or the instant at which the browser dispatches an event lives in one of these two
scripts instead. Each builds its own dev-only Vite root under `web/scripts/` — never part of
`pnpm build`, and never a route, because a route would be a product surface and every row on these
pages is fabricated — serves it, drives it with Playwright, and exits non-zero on a failed
assertion.

* **`pnpm bench:list`** — `web/scripts/list-bench.mjs` over `web/scripts/harness/`. Measurement plus
  the ARIA and roving invariants for `$lib/List.svelte`. Its own header says which parts are
  load-bearing; the containment **control run** and the **frame-per-step** drift loop both look like
  removable overhead and are the reason the numbers mean anything.
* **`pnpm test:freeze`** — `web/scripts/freeze-check.mjs` over `web/scripts/freeze-harness/`. The
  negative control for ADR-0038: real pointer, real focus, real appends inside a real Svelte flush,
  and **every assertion on the order of the rendered rows rather than on an aim flag**. That
  distinction is the whole point — the rule shipped broken past a set of checks that asserted the
  flag. Its header carries the measured browser behaviour the scenarios depend on, and says how to
  fire it deliberately against the pre-fix code.

**Neither is in `make check`**, both need a Chromium, and both are one-command reproducible from a
clone with the browsers path set.

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
  12-step table rebuild explicitly (create new → copy → drop old → rename), inside a transaction.
  ⚠️ **Do not write `PRAGMA foreign_keys=OFF` around it — steps 1 and 12 of the published procedure
  are unavailable here.** SQLite documents that pragma as a **no-op inside a transaction**, and goose
  runs every migration in one (`internal/db/migrate.go`), so the line looks like protection and is
  none. Measured on `49dfa6c`, Go 1.25.13, `ncruces/go-sqlite3 v0.35.3`: inside `BEGIN IMMEDIATE`,
  `PRAGMA foreign_keys = OFF` **raises no error** and `PRAGMA foreign_keys` still reads **`1`**; the
  identical statement on the same connection outside the transaction reads **`0`**; and an insert
  violating a foreign key inside that transaction still fails with `FOREIGN KEY constraint failed`.
  What to do instead: the pragma exists so that the `DROP TABLE` of step 6 does not cascade into
  **child** tables, so **establish that the table you are rebuilding is a parent of nothing** and the
  step is not needed — and assert it rather than assuming it, with `PRAGMA foreign_key_check` in a
  test, where a violation can fail a build instead of being discarded mid-migration. If the table
  *is* a parent, the rebuild needs a different design, not a pragma. `PRAGMA defer_foreign_keys` is
  not the escape either — it relocates the same failure to `COMMIT` with a worse message.
  **Worked example: `internal/db/migrations/00005_library_sync.sql`'s `write_queue` rebuild**, whose
  header carries this argument in full next to the SQL, plus `REVIEW-LOG.md` §M5.7.
* The round-trip test runs every migration against an empty in-memory DB and asserts the result
  matches a checked-in `schema.sql` snapshot. This catches "works on my dev DB because it was created
  three migrations ago" drift.
* ⚠️ **That round-trip test is a check on SHAPE, and it proves nothing whatever about DATA.** Against
  an empty database a table-rebuild's `INSERT INTO new SELECT … FROM old` copies **zero rows**, so
  every way the copy can be wrong — a dropped column, a swapped column, a `CASE` that quietly
  rewrites a value — passes it. This is not hypothetical: migration `00005`'s `write_queue` copy
  NULLed every non-NULL `work_id` and went through the whole suite green
  (`REVIEW-LOG.md` **M5-12**, **M5-13**).
* **So a rebuild needs a populated test of its own, and the shape is fixed:** migrate to **N-1**,
  populate the table being rebuilt with rows that exercise every column — `NULL` and non-`NULL` for
  every nullable one, every value a `CHECK` permits, the boundary integers, non-BMP and control
  characters in the free-text columns — then migrate to **N** and **diff every column of every row**,
  reading column names off the result set so a column a later migration adds is compared too. **The
  pattern to copy is `internal/db/migrate_test.go`'s `TestMigrate0005WriteQueueCopyIsLossless` and
  `TestMigrate0005AbortsRatherThanDroppingWorkID`** — the second because the honest outcome for rows a
  rebuild cannot carry is to **abort with a message naming the count and the remedy**, never to write
  a `NULL` and continue. 🔥 Fire the test against the unfixed migration before trusting it; both of
  those were.
* `cache.db` is a second database and is **never `ATTACH`ed inside a `usarr.db` write transaction** —
  cross-database locking reintroduces the `SQLITE_BUSY` class the single-writer design exists to
  avoid.

---

## 7. Developing with no *Arr stack at all

**This is the normal case, not the degraded one.** The project owner runs no Sonarr, Radarr or
Lidarr; the agent container the build runs in has no Docker daemon and no FFmpeg (§8). So the two
offline mechanisms below are the *primary* path, and the live stack exists only to record into them.

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
4. Commit the cassette. Every contributor — and any CI added later — now runs that test with no
   stack at all.

**Scrubbing is mandatory and must be a recorder hook, not a manual step — and it is already
written.** `internal/vcrscrub` owns it: `vcrscrub.New(path)` is the ONLY supported way to open a
cassette, and it installs the `BeforeSave` hook and the redacting matcher rather than offering them,
so there is no wiring left for a caller to forget. It reads `USARR_RECORD` itself, so step 2 above
works without any per-package mode handling. Do not write a second one.

What it strips, and where the boundary sits:

* Credential **query parameters**, in the request URL, in a form body, inside a URL embedded in a
  response body, and as a JSON object key. The parameter NAMES come from `internal/ssrf`'s
  `credentialParams` — the one deny-list — via `ssrf.IsCredentialParam`. Do not add names anywhere
  else.
* Credential **path segments**. Kavita's OPDS routes carry the Auth Key as a bare path segment,
  where there is no name for a deny-list to match on.
* The bearer headers: `X-Api-Key`, `Authorization`, `Proxy-Authorization`, `Cookie`/`Set-Cookie`
  (qBittorrent's `SID`/`QBT_SID_*`), `X-Transmission-Session-Id`, and Kavita's `X-License-Key` /
  `X-Anilist-Token`. `Location`, `Content-Location` and `Referer` are redacted rather than replaced,
  because a redirect hop is what the cassette is recording.

**`make secrets` is only half a backstop and it is worth knowing which half.** Drilled against this
repo's `.gitleaks.toml` with a freshly generated uuid4: `?apiKey=<guid>` in a cassette scans **exit
1**, the same guid as a bare path segment scans **exit 0**. gitleaks fires on the adjacent keyword,
not on the credential, so it closes the labelled half of the class and misses the unlabelled half.
`TestCassettesOnDiskCarryNoCredential` is the guard that covers the other half; it scans every
committed cassette on every `make check`.

**Scrubbing has to be fired before it is trusted, and the secrets gate is not a substitute for it.**
A cassette records the request URL VERBATIM, so any credential an API accepts as a query parameter
or a path segment lands in the YAML and, once committed, in git history permanently. `make secrets`
closes only part of that class, and which part depends on the wording of the URL rather than on the
credential. Measured on 2026-08-19 against gitleaks `v8.30.1` — build-info module
`github.com/zricethezav/gitleaks/v8@v8.30.1`, the identity `secrets` asserts — under this repo's
`.gitleaks.toml`, on an otherwise-clean tree plus one cassette carrying a freshly generated GUID:

| Recorded URL | `make secrets` |
| --- | --- |
| `…/api/image/series-cover?seriesId=1&apiKey=<guid>` | **caught** — `generic-api-key`, exit 1 |
| `…/api/Opds/<guid>/series` | **not caught** — "no leaks found", exit 0 |

The rule fires on the adjacent `apiKey=` keyword, not on the credential, so the same key carried as
a bare path segment scans clean. Generate a fresh random GUID if you re-run this: a recognisable
sample value may be allowlisted upstream, and would report a false negative.

So: **recording against a live instance was frozen until a `BeforeSave` scrubber stripped
credentials from the URL — path as well as query — and had been fired against a known-bad recording.
Both conditions are discharged and the freeze is LIFTED**, by the scrubber that landed at `36d7f71`.
Why it existed has not stopped being true, which is why the rule it left behind is that cassettes are
opened through `vcrscrub.New` and nowhere else: a cassette records the request URL VERBATIM, so once
committed the credential is in git history permanently, and an unfired scrubber is indistinguishable
from no scrubber (§11) — a silent, permanent failure rather than a red gate.

What discharged each half:

* **Path as well as query.** `vcrscrub.RedactURL` runs `redactGUIDSegments` *before*
  `ssrf.RedactRawURL`: it splits the raw string on `/` and replaces any whole segment that is a
  canonical GUID. That is the `…/api/Opds/<guid>/series` row in the table above — the one `make
  secrets` scans clean. `internal/ssrf`'s own path heuristic does not catch it and is right not to,
  because ssrf writes immutable provenance rows and is biased towards missing a key rather than
  eating a real path segment; the opposite bias belongs here, where an over-redaction shows up in a
  diff before the commit.
* **Fired.** `TestScrubDrillArmed` records against a local fake and asserts the cassette holds
  `/api/Opds/REDACTED/series`; `TestScrubDrillNeutered` records the SAME interaction with the hook
  removed and asserts the key DOES land, so the drill shows the hook is what did the work rather than
  the client happening not to send the key. `TestRedactURLHandlesAPathSegmentGUID` pins the unit, and
  `TestCassettesOnDiskCarryNoCredential` re-scans every committed cassette on every `make check`.

`vcrscrub.New` redacts the URL on **both** sides of the matcher, because a cassette that could only
match by having stored the credential is precisely what the hook exists to prevent.

Kavita's image endpoints produced this rule; it generalises to any API that takes a key in a URL. On
`api/specs/kavita-v0.9.0.2.json` — the release the owner runs — `GET /api/Image/series-cover`
declares both `seriesId` and `apiKey` `in: query`, and all twelve `/api/Image/*` cover routes accept
the key that way; on `develop` they do not (`internal/kavita/doc.go:31`). The spec's one `AuthKey`
scheme is `x-api-key`, `in: header`, declared globally — but a global declaration is not evidence
about a particular controller, and Kavita has two that ignore it, so whether the header alone
suffices on a cover fetch is a fact about the running controller rather than one the spec settles.

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
3. **Drift detection.** *Not* the PR gate — it needs network. Upstream `develop` moves; UsArr should
   hear about a renamed field from a bot, not from a user's bug report. **`make spec-drift` is the
   runnable form of this and exists now** (ADR-0047): behind the `upstream` build tag, refusing
   without `USARR_SPEC_DRIFT=1`, resolving upstream refs to the blob name of their checked-in spec
   with a blobless `git fetch` so it downloads trees and no file contents. It is **never** part of
   `make check` — the gate makes exactly two network calls, both to vulnerability databases, and a
   GitHub outage must not redden a commit that touched no spec. A failure there is **news**.

   The offline half lives in the gate and answers a different question: `TestVendoredSpecIsThePinnedBlob`
   hashes `api/specs/prowlarr.json` to the git blob name it is pinned to, so a re-vendor, a hand-edit
   or a bad merge fails deterministically with no network at all.

⚠️ **The spec is not always right** (see `/api/v3/episode` in §5). Where a controller enforces
something the schema does not express, the contract test encodes the controller's behaviour and a
comment says why.

⚠️ **And it is not always CURRENT, which is a different failure.** Prowlarr's `openapi.json` was last
regenerated on 2025-06-07 and 33 releases have shipped since, so it describes neither the release the
owner runs nor `develop` (ADR-0047). Where the vendored document is *known* to contradict upstream
source, the gap goes in `knownSpecDivergences` in `internal/servarr/contract_test.go`, which asserts
the divergence is **still there** — a comment cannot do that, because a comment that has gone false
looks exactly like one that has not. `SearchResource.{Limit,Offset}` are the worked example: `int?`
upstream since `c687bdb1f`, still plain `int32` in the vendored spec, and "correcting" UsArr to match
the spec would resurrect the `&limit=0` bug upstream fixed.

Caveats: Jellyfin publishes an OpenAPI spec, but the 10.11 schema was reported invalid (malformed 503
response headers), which breaks naive codegen — generate *types* if useful, but hand-write the ~25
endpoints UsArr needs. LazyLibrarian has no spec at all: a single CherryPy endpoint with 184 `cmd=`
values and an untyped envelope, so it gets hand-written fixtures and a hand-written contract test.

### 7.3 The compose fixture stack — for *recording only*, never CI

`deploy/compose/dev-stack.yml` **(not yet)** brings the ecosystem up on a developer workstation
(Sonarr 8989, Radarr 7878, Lidarr 8686, Prowlarr 9696, plus the media servers). ⚠️ Image names and
internal port mappings were not verified in this pass — check each project's own compose example
before committing the file.

✅ **Both `(not yet)` markers in this section still hold**, but the reason recorded for them on
2026-08-17 at `8756d02` — *"`deploy/` does not exist on disk or in `git ls-files`"* — has since gone
stale: `deploy/` does exist, holding `update.sh` and `status.sh` (§12.1). Nothing under
`deploy/compose/` does, and the `Makefile` still has no `seed` target. Do not trust that sentence or
this one for what is on disk; `git ls-files deploy/` and `grep -c '^seed:' Makefile` answer it in the
form that cannot go stale. The third marker this file carried, on `make dev`, was **false** —
`Makefile` defines the target — and was removed in the same pass.

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

## 8. The unattended environment: no Docker daemon, no FFmpeg, two network calls

**There is no CI, and this section is the constraint a future one inherits.** Checked 2026-08-17 on
`d64b8fc`: no `.github/`, and no `gitlab-ci`/`circleci`/`drone`/`jenkins`/`woodpecker`/`azure`/
`buildkite`/`forgejo`/`gitea` config anywhere in the tree; `core.hooksPath` is unset and `.git/hooks/`
is all `.sample`; the GitHub Actions API reports `total_count: 0` workflows for `jdb3750/UsArr`. So
**every rule below is enforced today only by a person or an agent typing the target.** `make check`
at least carries an obligation — `CLAUDE.md` requires it before any commit — but nothing mechanises
even that, and `make design` is required by no document at all (`docs/REVIEW-LOG.md`, `OPTIN-01`).
Read the rules as binding on whoever runs the build, and as the specification any CI added later must
satisfy; do not read them as a description of something already watching the tree.

**The Docker daemon is unavailable in the agent container.** Verified: `docker info` fails. FFmpeg
is absent too, and stays absent. That container is real and is where this project's builds happen;
it is the environment the rules below were written against, whether or not a CI ever joins it.

Therefore:

* `make test`, `make lint`, `make check-offline` and `make build` **must not require Docker, a live
  service, or network access.** If any of them ever does, that is a bug in the target, not an
  environment problem to work around.
* `make check` adds exactly two network calls, both to vulnerability databases — govulncheck against
  `vuln.go.dev`, and `pnpm audit` against the npm registry. Both live in the `vuln` target. Nothing
  else in the gate touches the network, and no target may add a third without a stated reason.
* Testcontainers, dockertest and "spin up a real Postgres/Sonarr in a test" are **out of bounds** for
  the default suite. This is also why UsArr uses SQLite rather than anything needing a server.
* Anything requiring the stack goes behind `//go:build integration`, is gated on
  `USARR_INTEGRATION=1`, and runs only under `make test-integration` on a developer machine.
* `make docker` exists for humans and for release pipelines that do have a daemon. It is never part
  of `check`, and a CI, if one is ever added, must not call it.

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
`gitleaks` is the one tool asserted a different way: installed by `go install` it reports
"version is set by build process", because upstream stamps the real version with `-ldflags` at
release time. Its identity comes from the Go build info the toolchain records in the binary
(`go version -m`) instead — see §11, "Ask the binary the question it can actually answer".

`.golangci.yml` — v2 format, `linters.default` replaces v1's `enable-all`/`disable-all`. This is a
summary; the file itself is the source of truth:

```yaml
version: "2"
run:
  build-tags: [upstream, bench]  # a file behind a build tag is NEVER OPENED without this
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

**The `run.build-tags` list is load-bearing too, and for a reason that is easy to miss.**
golangci-lint analyses one build context, and without tags that context excludes every file behind a
`//go:build` line — not linting them weakly, never opening them. The silence is total: the run prints
`0 issues` and exits 0 exactly as it would over a clean file. Measured 2026-08-18 with a matched
pair, same package and same statement (`os.Setenv(…)` with the error dropped): planted in
`internal/servarr/specdrift_upstream_test.go` (`//go:build upstream`) golangci-lint reported
`0 issues`, exit 0; planted in an untagged `_test.go` beside it, exit 1 with the errcheck finding.
One defect, two verdicts, decided by a build tag. **`make build-tagged` is not this** — its
`go build -tags=bench` and `go vet -tags=upstream` type-check the hidden files, which closes the
*compile* hole and nothing else. **Keep the list in step with the tree:**
`grep -rn "go:build" --include="*.go" .` names every tag in the repo, and each one absent from
`run.build-tags` is a file the gate does not lint. `bench` was added the same day on the same kind
of pair — a dropped `os.Setenv` in `internal/db/pragma_spike.go` against an untagged twin in
`internal/db/sqlite.go` gave 1 errcheck finding before and 2 after, control still firing — plus a
third probe in `internal/db/spike/workload.go`, because that package is not in `go list ./...` and
the two-file pair alone would not have proved it was opened. The tagged code linted clean, 0 issues,
with nothing suppressed. **One residual gap remains and is not closable from this list:**
`internal/db/spike/rss_other.go` is `bench && !linux`, so GOOS keeps it out on a linux runner —
neither linted nor compiled by `make build-tagged` here.

**The `issues:` block is load-bearing.** golangci-lint defaults to `max-same-issues: 3` and
`max-issues-per-linter: 50`, and it drops the remainder *silently* — no "N more" line, nothing in the
summary. This repo carried 7 `noctx` findings that a default run displayed as 3, so clearing the
three on screen would have looked like finishing the job. Do not restore the defaults to shorten the
output.

### Frontend

```bash
cd web
pnpm lint            # eslint
pnpm format          # prettier --write
pnpm check           # svelte-check (types + a11y + unused CSS)
```

**`cd web` first; do not run `pnpm -C web …` from the repo root.** The repo root has no
`package.json` — this is a Go repository with a `web/` subdirectory, not a JS workspace — so a
corepack-provided pnpm finds no `packageManager` pin there, falls back to its own default version,
and then collides with `web/package.json`'s `pnpm@10.33.0`. Running from inside `web/` puts the pin
where every resolver looks for it. The `Makefile` does the same thing for the same reason, and sets
`COREPACK_ENABLE_DOWNLOAD_PROMPT=0` so an unattended build cannot stall on corepack's interactive
download confirmation.

`svelte-check` is part of `make lint`, not an afterthought — with `adapter-static` a type error in a
route only surfaces at build time otherwise.

#### The `cookie` override in `web/package.json`

```json
"pnpm": { "overrides": { "cookie": "^0.7.2" } }
```

**JSON takes no comments, so the reason lives here.** `@sveltejs/kit` 2.70.2 declares
`"cookie": "^0.6.0"`, and a caret range on a `0.x` version cannot cross the minor — so left alone it
resolves inside `0.6.x`, entirely within **GHSA-pxg6-pf52-xh8x** (out-of-bounds characters accepted
in a cookie name, path or domain; fixed in `cookie@0.7.0`). The override lifts the whole tree to
`^0.7.2`, and `pnpm-lock.yaml` resolves it to exactly one copy — `cookie@0.7.2`, no duplicates,
confirmed with `pnpm why cookie`. Without it `pnpm audit` fails, and `pnpm audit` is a gating step of
`make check` (§4), so this is load-bearing and not a tidiness preference.

**Remove it when, and only when, SvelteKit's own dependency range no longer admits a vulnerable
version.** Check with `cd web && pnpm why cookie`; if the resolved version is ≥ 0.7.2 with the
override deleted, the override has become a no-op and should go. Deleting it while it is still doing
work turns the gate red, which is the intended behaviour.

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

### Writing a guard that can be trusted

Every rule below is here because this repo shipped its opposite and stayed green. They are the
mechanics behind `CLAUDE.md`'s "verify, don't assert" — that rule says what to do, these say how a
check earns the right to be believed.

**1. Probe the condition, not a proxy for it.** `make design` once carried a fourth guard meant to
confirm Playwright would resolve. It did not test resolution; it **grepped the import specifier out
of `check.mjs`** — asserting the shape of the script's source instead of the behaviour it cared
about. When the static import became a dynamic resolution ladder, the grep matched nothing, and an
unrelated refactor had silently disarmed a guard nobody was watching. A proxy and its condition
agree right up until the day they matter. Ask what the guard is really for, then test *that*.

**2. Report what you measured, not just the verdict.** A green with no evidence attached is a
rumour. Two of them here:

* `make check` resolved `golangci-lint` from `PATH` and, for the life of the project, ran an
  **unpinned and much older version** while reporting green. Nothing was wrong with the output — it
  was a true statement made by the wrong tool. This is why `require_tool` now prints the absolute
  binary path and its version before the linter runs, and why the version is asserted against the
  pin rather than merely displayed.
* golangci-lint's stock `max-same-issues: 3` **truncated 11 findings to 7 and said nothing** — no
  "N more" line, no difference in the summary. `gosec`'s four sat exactly at that boundary: one more
  finding with the same message text and the security results would have been the ones hidden. The
  config now sets `max-same-issues: 0` and `max-issues-per-linter: 0`, and a capped run that agrees
  with an uncapped one is what makes a number a count instead of a floor.

So: name the binary, the version, the commit, the flags and the count. **A gate result without a
commit sha attached is not a result** — several threads push to `main` within the same hour here, so
a green measured five minutes ago may already describe a tree that no longer exists.

**Ask the binary the question it can actually answer.** `require_tool` asserts three of the four
pinned tools on their `--version` output. gitleaks cannot answer that way — a `go install`ed gitleaks
prints, verbatim:

```
$ gitleaks --version
gitleaks version version is set by build process
```

That is gitleaks' own default (`version/version.go` sets `var Version = "version is set by build
process"`); upstream overwrites it with `-ldflags -X` when it cuts a release, and `go install` passes
no ldflags. So for most of this project's life the *secrets scanner* — the gate binary you would
least want to be an unknown build — was the only one running with its identity unasserted, on a gate
whose stated standard is that a green naming neither its tool nor its tree is a rumour.

It is asserted now, on the Go build info the **toolchain** stamps into every `go install`ed binary:
`go version -m <bin>` reports the module path, the module version and the go.sum hash of what was
actually fetched and built. `make secrets` now prints

```
tool: /root/go/bin/gitleaks — build-info module github.com/zricethezav/gitleaks/v8@v8.30.1, asserted against the pin (--version is unstamped)
```

and fails when the module path or version does not match `GITLEAKS_MODULE@GITLEAKS_VERSION`. Two
alternatives were tried and rejected, both recorded above the macro in the `Makefile`: re-stamping
the version ourselves at install time reduces the assertion to the Makefile asking the binary to echo
a string the Makefile just handed it, *and* rejects a correct gitleaks installed by any other route;
and a content-hash pin is not reproducible across toolchain patch releases or platforms, so it would
be re-recorded reflexively and mean nothing. Build info is the one claim about the binary that the
binary's author did not get to write.

`require_tool` now takes an identity pin in one form or the other and **fails when given neither** —
existence alone is no longer an accepted answer for a pinned tool. It also takes, and likewise
requires, the names of the variables that pin was computed from, so the banner can say whether the
pin held or was moved; a tool added without them fails the gate rather than printing a banner that
cannot tell the two apart. `go version -m` reads the file on disk and makes no network call, so
`check-offline` keeps its contract.

**And the number in your report is quoted from that banner — never from a second question asked
afterwards.** An agent piped `make check` through `tail -45`, which cut the `tool:` lines out of
view, then filled the version in afterwards by running a bare `golangci-lint --version`. On `$PATH`
that is this box's stray `/usr/local/bin/golangci-lint` 2.5.0, not the pinned
`/root/go/bin/golangci-lint` 2.12.2 the gate had actually run, so the report named a version nothing
in the gate had used. **Nothing was wrong with the gate** — settled twice over: re-running
`CI=true make lint-go` printed the 2.12.2 banner, and pointing the assertion at the stray binary made
it exit 1 with want/got. The defect was entirely in the report, and it is this rule's own first
bullet — *a true statement made by the wrong tool* — **reproduced one level up, by the reporter,
against the very trap the absolute-path pin exists to close.** The pin stops the *recipe* from asking
`$PATH` which binary to run. Nothing stops a *reporter* from asking `$PATH` which binary ran.

* **A version number in a report is quoted from the gate's own `tool:` banner, path included.** That
  line is the only one naming the binary the assertion actually ran; a version obtained any other way
  describes a different binary that happens to share a name.
* **Never truncate the gate's output past a banner you will need to cite.** `tail -45` is the cheap
  reflex and the banner is the *first* line of each step — so the part that says what ran is the
  first part a tail discards. Keep the whole run in a file and read both ends.

This is rule 7 (*quote the tool*) one step earlier: rule 7 governs how faithfully the tool's output is
transcribed, this governs whose output is being transcribed. And the banner is worth quoting because
it carries what a reporter cannot reconstruct from outside — the absolute path, the asserted version,
and, since every pin here is `?=` and therefore overridable, whether that version is the one this repo
ships or one moved on the command line:

```
tool: /usr/local/bin/golangci-lint — version 2.5.0, asserted against an OVERRIDDEN pin (GOLANGCI_VERSION=2.5.0 from the command line) — NOT the version this Makefile ships
```

**3. Exercise the failure path.** A guard's failure branch is code, and untested code does not work.
It is also the branch that by definition only runs when something is already wrong, which is exactly
why nobody ever runs it. Fire the guard deliberately — break the thing on purpose — and confirm it
says what it was written to say. The same `make design` guard from rule 1 makes the point twice
over: under `.SHELLFLAGS := -eu -o pipefail` it **died at its own variable assignment, before
reaching the error message it existed to print**. Even a correct probe would have been mute. A guard
that cannot speak when it fires is indistinguishable from no guard at all.

**4. Adding a check means declaring what it should find, not only what it forbids.** A check with a
knowable floor asserts that floor and fails beneath it. `fmt-check` refuses to pass when it finds
zero `.go` files; `vuln` refuses to pass when `go list` yields zero packages. Both would otherwise
have exited 0 while scanning nothing, which is the most convincing green there is. If you can state
roughly how much a check ought to see, encode that — "found nothing" and "looked at nothing" must
never produce the same exit code.

**`make spec-drift` is the third, and it is here because it shipped without the floor and the rule
caught it.** `go test -run <re>` that matches nothing exits 0, so the target's original
`-run Upstream ./internal/...` would have reported success — with a reassuring *"a failure here is
NEWS"* epilogue — over zero drift checks, and renaming the one drift test was enough to produce
that. Worse, `Upstream` is an ordinary English word and `-run` is an unanchored regex: it also
matched five unrelated tests in `internal/kavita`, `internal/releases` and `internal/store`, so
those three packages printed a bare `ok` and read as drift-checked while holding no upstream-tagged
test at all. **The output was misleading about its own coverage, which is the failure this rule
names, dressed as extra reassurance rather than as silence.** Both halves are closed together: the
selector is the reserved `^TestSpecDrift` prefix, and the target counts the top-level
`--- PASS:`/`--- FAIL:` lines that prefix actually produced and fails below `SPEC_DRIFT_FLOOR`.
A `--- SKIP:` deliberately does not count — a skipped check looked at nothing too.

⚠️ **The epilogue quoted above is historical — do not expect to grep it out of the `Makefile`.**
`d10ca98` replaced that one blanket line with four verdict readings, so an unreachable github.com now
prints *"THIS IS NOT NEWS ABOUT UPSTREAM"* rather than being reported as drift. The old wording is
kept here because it is what the guard actually printed while the floor was missing, and the point of
the example is that the reassurance was the misleading part.

**5. Name the surface, not just the value.** Rule 2 names the *instrument*; this names what the
instrument was pointed at, and it is the half that has been broken three times here — each time by
someone who had written or read the rule the same night. **A measurement is meaningless without the
artifact it was taken on, and two correct numbers taken on different surfaces cannot be compared,
subtracted, or checked against one another's budget.**

* **Two trees declare the same column.** *"The `Age` track is 68 px"* is not a complete claim in a
  repo holding two declarations of it: `.cols-requests-releases` in `docs/design/mockups/usarr.css`
  reads 80 px, `COLUMNS` in `web/src/routes/requests/+page.svelte` reads 68 px, and **both are
  correct**. Nine of the ten columns the two trees share carry different widths.
* **A row-height result measured on the mockups' 80 px `Age` track was applied to the product's
  68 px one.** Right about the tree it came from, wrong about the tree it was quoted for.
* **The sharpest of the three, because nothing about either number was wrong.** A shipped-path
  density toggle — **~75.7 ms** at 200 rows, `prefs.setDensity` → Svelte flush → forced layout — was
  compared against a budget being applied at the bench's own measurement site, **~18 ms**, which in
  `web/scripts/list-bench.mjs` is a bare `setAttribute` plus a forced layout and skips the
  invalidation the shipped path is required to perform. Both figures were individually correct and
  independently verified. Nothing was wrong except the assumption that they were about the same
  thing, and the conclusion drawn from subtracting them was.

So: **every figure carries the artifact it was measured on** — which file, which tree, which code
path, which instrument — and **a comparison between two figures is valid only once you have checked
that they describe the same surface.** Rule 2's *"a gate result without a commit sha attached is not
a result"* is this rule applied to gate output; the same standard governs every other number.

**The same question, asked of two claims that do not look like measurements.** Rule 2 names the
instrument and rule 5 names what it was pointed at; both of these are the second half, in prose
rather than in a number.

* **An absence claim carries the roots it searched.** *"The four figures exist nowhere in the tree"*
  came from a search that never entered `web/scripts/` — one of three scope-wrong searches in a
  single night. The surface of a search is its roots, so report an absence as *"not under `docs/`,
  `web/src/` or `web/scripts/`"* and never as *"nowhere"*: the second is a claim about the whole
  tree that the search did not make. Rule 4 says a check that found nothing and a check that looked
  at nothing must not produce the same exit code. This is that failure with the sign flipped.
* **A probe carries the gate that has to reject it.** Firing a guard on purpose (rule 3) is evidence
  only when the stimulus is one that guard would catch, and *looks like a credential* is not that
  test. Measured 2026-08-17 on `/root/go/bin/gitleaks`, build-info
  `github.com/zricethezav/gitleaks/v8 v8.30.1`, one planted line per file under `gitleaks dir <dir>
  --redact=100 --no-banner --exit-code 1`: AWS's own documentation pair `AKIAIOSFODNN7EXAMPLE` /
  `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` scans **`no leaks found`, exit 0**, because gitleaks
  allowlists it upstream — **and so does `ghp_` followed by 36 sequential characters**, which clears
  the length the rule wants but not its entropy floor. Only a `ghp_` token with a random
  36-character body reports **`leaks found: 1`, exit 1**. Two of those three probes prove nothing
  about the scanner while looking exactly like proof. The full case — including the false negative
  that nearly "proved" `make secrets` ignores `docs/` — is in `docs/REVIEW-LOG.md` under *"What a
  `make check` green on a docs-only commit does and does not attest"*.

**6. Corroboration is coverage, not repetition.** Rule 4 asks what one check should find; this asks
how many *different* things a set of checks could see between them. Agreement is worth what the
distinct stimuli behind it are worth, and nothing more.

* **Checks that share a stimulus agree for free.** Five independent-looking checks all missed the
  same transient, because every one of them could read only files that were present and committed —
  a file written unstaged and deleted before it was ever staged was invisible to all five. Five
  agreeing answers, one stimulus. Count the stimuli, not the checks, and when they are all the same
  stimulus say so instead of reporting five confirmations.
* **A negative claim about other workers has to have visited each one.** *"None of my workers wrote
  that file"* is not establishable from the tree, because the tree is precisely where the evidence
  is absent. The unexplained `ZZ-probe.md` came from a worker whose own transcript had to be read
  before its four unstaged writes were found. Vouch for a set of agents only by naming which ones
  you read and what you read of them; otherwise say you do not know, which is cheaper and true.

**7. Quote the tool; do not paraphrase it.** This is the rest of rule 2's sentence: name the
instrument, then report its output in the instrument's own characters. Paraphrase is where
mis-transcription hypotheses breed — the `ZZ-probe.md` investigation burned its first pass on a
theory about a filename gitleaks had never printed, because the actual output, `leaks found: 1`
with no path in it at all, had been summarised rather than quoted. A summary silently adds detail
the tool did not give you and drops the detail it did. Paste the line.

**8. A count is a measurement — compute it from the artefact, never from recollection.** Three of
these landed in one day, across three threads and three files: a total stated over a list that
contradicted it, twice low and once high, one of them contradicted by its own commit message as
well. Nobody was careless — each figure was true when it was formed, and then the list moved under
it and the number did not. The list is the instrument, so **`wc`/`grep` the landed diff or the list
itself at the moment you write the figure**, not when you first arrived at it. This has ridden along
as per-brief boilerplate for a while; it is written down here so it stops depending on whoever wrote
the brief remembering to include it.

**The pattern worth carrying: two of the first three were introduced by the fixes for the other
two.** That is not bad luck. A fix is written under the assumption that the failure mode is now
understood, and that is precisely the moment people stop checking for it. Treat a guard you just
repaired as the least trustworthy thing in the file, not the most.

**Two rules when gathering lint evidence, and one hazard behind both: there are two `golangci-lint`
binaries on this box.** `/usr/local/bin/golangci-lint` is 2.5.0 and is what a bare name on `PATH`
resolves to; the gate's own `$(GOBIN_DIR)/golangci-lint` is the pinned 2.12.2 — `/root/go/bin/` here
— and is what the `Makefile` invokes, by absolute path, in every recipe. The wrong binary bites
twice: once reporting findings, once clearing a cache.

* **Never invoke `golangci-lint` by bare name — on `PATH` it is the stale 2.5.0.** Only `make check`
  (or `make lint`) is authoritative, because only those go through `require_tool` and assert the
  pin. Quote the tool line it prints alongside any claim of green.
* **Run `cache clean` with the gate's binary by absolute path — `/root/go/bin/golangci-lint cache
  clean` — before trusting any gate result from a tree where worktrees have come and gone, and name
  that path in the report.** Cached diagnostics replay against paths from other trees: a different
  clone of this repo, or a worktree that no longer exists. Two threads hit that on 2026-08-16 with
  two different linters — four gosec findings against the deleted sibling worktree `../wt-flake/`,
  and errcheck at `../merge-wt/internal/db/sqlite.go` where the real tree's copy already carries
  `//nolint:errcheck`. Both cleared on `cache clean`, and both real runs then reported 0 issues — two
  linters, so the cache keys on paths rather than on anything linter-specific. A third thread hit the
  *absolute path* half the same day: a bare `cache clean` emptied 2.5.0's cache, left the gate's
  untouched, and looked performed — only re-running it as `/root/go/bin/golangci-lint cache clean`
  gave a trustworthy run. So **"I ran `cache clean`" is evidence of nothing unless it names the
  binary.** All of that was observed; the two stale-cache cases were false *reds*. **A stale red is
  annoying, but a stale green is invisible** — nothing in the output distinguishes a replayed result
  from a fresh one, so the failure you would never notice is the one that matters. That last point
  alone is inference from the same mechanism, not a case anyone has caught here, and it is why this
  is a rule rather than a note.

**And a green `make check` attests almost nothing about a documentation commit.** Exactly one step
of the gate reads `docs/`: `secrets` runs `$(GITLEAKS) dir .` from the repo root, over the whole
working tree. Everything else stops short of it — `fmt-check` counts `*.go` for gofumpt and runs
prettier through `pnpm_if_web`, which `cd`s into `web/` first, so `prettier --check .` never sees a
file outside that directory and **no Markdown in `docs/` is formatter-gated at all**; `lint`,
`test`, `modverify` and `vuln` read Go packages and `web/`. So green on a docs-only diff attests one
thing — no credential-shaped string anywhere in the tree — and nothing whatever about whether the
prose is true, current, or even well formed. Quote it at that size. The measured version of this,
fired in both directions, is in `docs/REVIEW-LOG.md` under *"What a `make check` green on a
docs-only commit does and does not attest"*.

**Two counts that read as facts about the repository and are facts about the observer.** Each
under-reported without saying so, and neither could tell you whether what it did not count was
absent or merely invisible from where it was standing. The first has since been fixed; the lesson
underneath it has not been retired, which is why it is still written down.

* **✅ Resolved — `lint-go`'s banner counted different packages than the linter opened.** Fixed in
  `eb92062`, *"chore: lint-go's banner counted 13 packages while the linter opened 14"*, which reached
  `main` as merge `7bd45e9`. **Recorded for the lesson, not as an open defect — do not re-open it.**
  The recipe used to derive its count from an untagged `go list ./...` and then run `golangci-lint
  run` **with no path arguments**, so golangci-lint resolved its own package set from `.golangci.yml`,
  whose `run.build-tags` is `[upstream, bench]`. The count was echoed and never passed to the linter,
  so the banner under-reported. **Both figures move as packages land, so read them as dated
  observations and not as constants** — at `a2cbee3`, where the defect was measured, `go list ./...`
  gave **13** and `go list -tags upstream,bench ./...` gave **14**; at `36d7f71` the same two commands
  give **14** and **15**. What does not move is the delta: it is one package, `internal/db/spike`, in
  both readings. **The gate did lint the tagged package throughout — it was a mislabel, not a coverage
  hole.** The banner now prints both figures and names the gap. At `36d7f71` it says:

  ```
  lint-go: linting 15 Go packages — 14 untagged, plus 1 behind .golangci.yml's build-tags (upstream,bench)
  ```

  The tag list in that line is **read out of `.golangci.yml` by `awk`**, not copied into the Makefile —
  the `lint-go` recipe's leading `tags=$(awk …)` scans for the `build-tags:` key and `paste`s the
  entries it finds — so adding a tag to the config widens the count with no Makefile edit, and the two
  cannot silently drift apart again. An empty extraction is not an error: it means the config sets no
  tags, and the two figures then coincide and the banner prints the short form.
* **A compile count is never evidence of lint coverage, and that half stands whatever either banner
  prints.** Reproducing it means writing defects into a source file, so it was measured once and is
  reported rather than re-measured here: two real defects planted in `internal/db/spike/workload.go` —
  a bare `defer stmt.Close()` in place of the checked form, and an unused package-level var — were
  both reported **by name**, `errcheck` and `unused`, and `lint-go` exited **2**. That file is plain
  `//go:build bench`, so it is reachable under the configured tags. With both defects still planted,
  `make build-tagged` was **green, exit 0**: `go build -tags=bench ./...` and `go vet -tags=upstream
  ./...` pass over an unchecked error return and an unused variable alike, while that step prints
  its own `build-tagged: compiling N Go packages with -tags=bench` banner. **Compilation is not a
  fallback for linting.** Two steps in `check` print package counts, only one of them lints, and a
  tree where `lint-go` had genuinely skipped a package would still print the full count under
  `build-tagged` and look covered.
* **The lesson the fix does not retire: a package count cannot distinguish a package that was opened
  from one that was skipped.** Only a planted defect can. A count is the same shape as any check whose
  success condition is an absence — it reports the same number whether the work happened or not — and
  a corrected banner is a better *label*, not a coverage *proof*. Quote either count as a floor guard,
  never as the scope of a lint run.
* **`git rev-list --count` measures the clone's visible depth, not the commit.** One tip, `2ce8ed9`,
  counted in three containers on one night: **811** in a clone that is not shallow, **366** in one
  reported shallow with two graft points, and **146** in one reported shallow with four. Only the
  first is measured here — `git rev-parse --is-shallow-repository` is `false`, there is no `shallow`
  and no `info/grafts` under `git rev-parse --git-common-dir`, and `git replace -l` is empty; the
  other two figures and their graft counts are **reported, not measured here**. Every reading was
  correct about its own tree and none was about the repository. Three things follow, and the last
  two cost more than the count did:

  * **Quote a commit count across containers with the clone's shallow status beside it, or not at
    all.** An unqualified count was broadcast as a general warning that other threads' gate greens
    were weaker than they thought, and it took a read-only agent in another container to disprove
    the general form of it. The local number was real; the generalisation was not.
  * **An unresolvable SHA in an agent container is evidence of a shallow cut, not evidence the SHA
    is wrong.** `git cat-file -t 9eec372` is reported to return *"fatal: Not a valid object name"*
    in the four-graft clone; here the same command returns `commit`, and
    `git merge-base --is-ancestor` puts that commit on `origin/main`. SHAs near the tip resolve
    and anything past the frontier is absent, so absence there is a fact about the clone rather
    than about the SHA. Before disbelieving a SHA an agent reports it cannot confirm, run
    `git rev-parse --is-shallow-repository`; drawing the other conclusion has already cost hours.
  * **The count is not stable even within one container.** That clone is reported to have read 103
    earlier the same night and 146 later, because a shallow fetch appends grafts and the frontier
    moves. A count therefore carries an argument neither between containers nor across time inside
    one.

  What does travel, stated positively: **ahead/behind against your own `origin/…`**, and
  **file-level diffs between two commits both ends can resolve**. Each is measured against something
  the other side also holds, which is precisely why it survives the trip.

### Consistency is a property of the read, not only of the write

**A writer's transaction guarantees only what a reader takes in one statement, or in one
`db.ReadTx`.** `store.ReplaceIndexers` writes `indexers_fetched_at` and the `indexer_catalog` rows
in a single transaction so that no reader can see one without the other, and it is right to. But
`GET /api/v1/indexers` then asked for them with two statements off the read pool — and two
statements on the pool are **two WAL snapshots**. A replication commit landing between them
returned a NULL stamp to the first and the fresh rows to the second, so the endpoint rendered
"UsArr has not yet read the indexer list from Prowlarr" while listing three indexers: a state the
database never held, at 1.2% of requests. `db.ReadTx` existed for exactly this, with a correct doc
comment, and had **no production caller at all**.

This is easy to miss because the write side looks careful and *is* careful — so the pair reads as
solved, and attention stops at the transaction that is already there. Ask the question at the read
instead: **do these two results have to agree?** A parent row and its children, a count and the rows
it counts, a status field and the data it describes, a stamp and the thing it stamps. If they do,
pair them under `db.ReadTx`, and pair them **in the store**, because the invariant belongs to the
pair rather than to whichever handler noticed first. If they do not, leave them alone and say why —
an unnecessary read transaction is noise that hides the necessary ones, and every one of them is a
lock the WAL checkpointer has to wait behind. The full sweep of this codebase's read paths, with the
reason each one is safe, is the consistency audit in `docs/REVIEW-LOG.md`.

### A wire vocabulary and a storage vocabulary never share a term

Three instances of this, and **every one of them compiled clean** — which is the whole reason it is
a convention. A collision the build catches needs no rule. This class is invisible to `go build`, to
`go vet` and to a reading eye, because both sides are spelled correctly and only the meaning differs.

* **Two constants, one name, two values.** `internal/httpapi` carried `outcomeSentUnknown` twice
  after a merge: `grab.go:214`'s is `audit_log.metadata_json`'s vocabulary (`"sent_unknown"`),
  `grabs.go`'s was the published shape of `GET /api/v1/grabs/recent` (`"sent_outcome_unknown"`).
  `go build` named two of the three uses; the third — `outcomeFor`'s unconfirmed arm — rebound to
  the audit spelling and compiled green, which would have shipped an unrecognised string to
  `web/src/lib/requests.ts`. Repaired in `0cb1a18` by prefixing the wire set
  (`wireOutcomeSentUnknown`, `internal/httpapi/grabs.go:206`), leaving both strings as each side
  ships them.
* **A parameter named for the wrong vocabulary — and this one was prose, not code.** §13's
  perf-budget table budgeted `GET /api/v1/library?kind=movie`. `kind` is a real column that ships on
  that endpoint's wire under its own name in every row, while the parameter's vocabulary is
  §17.2's six-value media-type enum — so the row named one vocabulary and spent the other. Corrected
  to `?media_type=movies` in `f80097f` on the browse branch (`claude/hearth-thread-rd9ukh`), which
  reached `main` in `1c13afd`; §13 now carries a note recording the correction, and the budget
  itself never changed. Prose is compiled by nothing at all, so nothing was ever going to catch it —
  a defect in a budget row could not have been a build error even in principle.
* **Two spellings of one concept, each legal in its own set.** `series` is a `work.kind`, `tv` is a
  media type, and a filter that accepted either would silently mean the other.
  `TestBrowseWorksUnknownMediaTypeIsAnError` (`internal/store/browse_test.go`, same branch) asserts
  that `MediaType: "series"` is refused, because nothing in the type system does.

So, two rules:

* **Before naming a new enum member, check it against every existing enum**, not only the one you
  are extending. The collision that matters is between vocabularies, not within one: a duplicate
  inside one set is a build error, and a duplicate across two sets is a green build.
* **When two vocabularies must coexist, prefix them distinctly** — `wireOutcome…` against
  `outcome…` — so a mismatch is a build error rather than a silent rebind. Do not repair a collision
  by making the values agree instead: two vocabularies that match today are free to diverge
  tomorrow, and one shared identifier turns a change to an internal record into a change on the wire.

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
* **Branch late, or rebase early.** `main` moving mid-task cost work three times in one day. The
  clearest case: a thread cut from `85cae80` built its own module equivalents of an ADR's rules, then
  threw them away when `7aa4f26` landed the real `frozenorder.svelte.ts`, `sortspec.ts` and
  `indexerscope.svelte.ts` **eighteen minutes later**. Discarding was the right call and nothing
  shipped wrong, but the work was wasted before it began, because the branch point was already stale
  when it was cut. Cut the branch when you start typing, not when you were assigned.
* **Cite symbols, not line numbers, while `main` is moving.** A `docs/REVIEW-LOG.md` citation to
  `web/src/lib/api.ts:1237` drifted **ninety-odd lines** — the symbol was at 1329 by the time the
  finding was pushed — and survived only because the author re-verified every citation after
  rebasing instead of assuming they still held (`6afe583`). A line number is a claim with a shelf
  life; a symbol name is one that does not decay. Cite the symbol; if you give a line too, date it to
  the commit you read it at.
* **A citation inside a dated record is history, not staleness.** The bullet above governs citations
  you are making now. It does not license a sweep through `docs/REVIEW-LOG.md` "fixing" the line
  numbers and SHAs inside an entry that already carries a date and a tree — those describe the tree
  they were taken on and are supposed to keep describing it, and editing them destroys the evidence
  the entry rests on. Correct such a record the way that file already does it: amend underneath,
  with the new date and the new tree, leaving the original standing (`docs/REVIEW-LOG.md` §6.1
  *Amended dispositions*, whose closing line is *"No existing entry's id, text or severity
  changed"*).
* **Cross-reference another repo document by heading or anchor, never by line number.** The two
  bullets above are the code half and the dated-record carve-out; this is the prose half, and it
  decays faster, because several lanes push `docs/` concurrently and a rotted line number **does not
  announce itself** — it keeps resolving, just to the wrong place. Two `docs/REVIEW-LOG.md` entries
  from 2026-08-19 are the evidence, and both are worth reading over this summary of them. `LS-320`
  corrected two `docs/reference/http-api.md` claims and deliberately wrote the replacements as
  *"Named by anchor, not by line … because this document has just spent a pass moving off citations
  that rot, and a line number would be the same defect in a new place"*, pointing at the `fileReadNote`
  helper and the `Items` cell's muted second line rather than at offsets. `LS-321`, one commit later,
  had its own citations rot **inside the pass that wrote them**: *"This entry's `security.md`
  citations were section names and line numbers when written, and the line numbers were stale within
  the hour"*, because `docs/reference/security.md` was rewritten wholesale while that change sat in a
  rebase. It re-verified against the pushed tree and re-cited by heading and bullet instead — §5,
  *"Redaction is middleware, not a convention"*, under *"URLs stored in the database are in scope
  too"*.
  * ⚠️ **A line number is correct, and preferred, where the file is never edited.** `LS-321` kept
    `internal/db/migrations/00005_library_sync.sql:221` and `internal/db/testdata/schema.sql:292` as
    line cites for exactly that reason — *"because neither file is edited"* — a merged migration, and
    the checked-in schema snapshot `TestMigrationRoundTrip` compares against byte-for-byte. Where
    nothing can move the line, the line is the most precise anchor there is; everywhere else it is a
    claim with a shelf life.
  * 🔍 **`LS-321` names the wrong commit for the rewrite that rotted it**, which is the rule failing
    on its own evidence. Its ⚠️ note attributes the `security.md` rewrite to `bf66828`; at `36d7f71`
    that commit touches `docs/DEVELOPMENT.md` and nothing else. The rewrite is `0ca1be6`, *"docs:
    security.md's present tense claimed guards nothing reaches"*, 193 insertions and 89 deletions in
    `docs/reference/security.md`. Cite `0ca1be6` when repeating this, and leave the entry's own text
    standing — it is a dated record, and the bullet above governs how it gets corrected.
* **ADR and migration numbers are allocated by the coordinator at dispatch, never discovered by
  reading the highest number in a merged file.** Write a placeholder — `ADR-XXXX`, `000NN_` — and ask
  for the id. **The reason is not bookkeeping, and without it this reads as bureaucracy:** an id read
  from a merged file is not free, because the number that invalidates it may be sitting on a branch
  the reader cannot see, and no amount of care with `git log` on your own checkout will surface it.
  The highest merged number is evidence about what has landed, never about what has been *claimed*.
  Measured at `36d7f71`, where the highest ADR in `docs/DECISIONS.md` on `main` was **0051**: **two
  unmerged branches had each independently allocated `ADR-0052`** off that reading —
  `docs/dd-8.1-sidebar-facets` as *"All six media types are always in the sidebar; per-type hiding is
  closed until a facet read exists"*, and `docs/adr0052-bookorbit-20260819` as *"v0.1's catalogue
  source is **BookOrbit**; Kavita is sunset"*. Both threads read `main` correctly and neither could
  see the other. **The race resolved while this bullet was being written**: `840233d` merged the
  BookOrbit ADR as 0052, so at that tip the sidebar branch still carries a second, conflicting
  `ADR-0052` and now has to renumber a merged-and-cited id out of its own history. The cost lands on
  whoever merges second, and it is not paid by the thread that caused it.
  * ⚠️ **A gap in either sequence is correct, and nobody renumbers to close one.** Closing a gap
    rewrites ids that other documents, commit messages and code comments already cite, and it is
    exactly the shared-counter operation this rule exists to avoid — the cure is the disease.
    `docs/REVIEW-LOG.md` already says so of its own ids: *"a gap in either is fine and nobody closes
    one"*.
  * 🔍 **The migrations half now has its instance, and it is recorded in two halves on purpose,
    because neither account alone was sufficient and collapsing them would misrepresent both.**
    **Stated by the participating lane — the only source for the trigger and the timing, and not
    independently checkable from here:** an agent was briefed to create migration `00008`; a collision
    warning reached it mid-run, because another decision had taken that slot; it verified against a
    fresh `origin/main`, wrote `00009_edition_format_index.sql` instead, and regenerated the golden
    dump after rebasing. The correction happened **inside a working tree, before anything landed** —
    and that lane explicitly **cannot** attest whether a `00008`-named file ever existed in a commit.
    **Measured — the only source for what pushed history contains, and it settles a different
    question:** `git log --all --diff-filter=R --summary -- internal/db/migrations` is **empty**, and
    `00008_image_asset_format.sql` and `00009_edition_format_index.sql` are two separate files, each
    added exactly once. **Pushed history carries no rename, which is consistent with the correction
    landing before the first commit** — and is why reading that same absence as *"the reported
    renumber does not reproduce"* proved too strong: an in-tree correction leaves `--diff-filter=R`
    nothing to find, so the empty result never was evidence against it. ⚠️ **The extension is the part
    worth carrying, and it makes the rule stronger than *"your read may be stale"*.** That agent
    **had** checked the number against `main`, and **its read was correct at the moment it was made**.
    The slot was taken underneath it *afterwards*, by a lane it could not see. So re-reading the
    highest number — more carefully, later, or against a fresher fetch — **is not a fix**: there is no
    moment at which such a read is safe, because it is only ever true of the tree as it was, and any
    concurrent lane can invalidate it before you land. **Only allocation fixes it**, because only the
    dispatcher sees every lane at once.
* **Key the worktree decision to the operation, not to the size of your change.** Any *whole-tree*
  git operation — `git add -A`, a `git commit` of an index somebody else may have added to,
  `git checkout <branch>` — belongs in a detached worktree of your own. Targeted single-path
  operations can stay in a shared checkout. The hazard is what the **operation** sweeps, not how
  much you personally edited: `ec4298d` is a one-file edit whose diff carries a second hunk — a
  scope caveat on a different entry — that its own commit message never mentions, because the
  sweep took whatever the tree was holding.
* **Never run two committing agents in one checkout — give each a detached worktree on a branch
  suffixed with its own id, or serialise them.** The bullet above governs how *one* agent should
  operate; nothing governed whether *two* should be launched at once, and that is a decision the
  launching session makes before either agent can protect itself. On 2026-08-17 one session ran two of
  its own committing subagents concurrently in the same working tree on the same branch, and the
  measured consequence is still in `main`: **`d64b8fc` and `547a604` are the same commit twice** —
  identical subject, identical author timestamp (`18:23:39`), the same parent `afe17fb`, a
  **byte-identical `docs/DECISIONS.md` diff**, and a `docs/REVIEW-LOG.md` diff differing in **nothing
  but four heading numbers** (`M5.19`–`M5.22` against `M5.23`–`M5.26`). The second is the amend; the
  first had already been pushed, so both are ancestors of `main` and the ADR-0041 commit appears twice
  in the history. Two further failures from the same launch are **relayed by that session and not
  measured here**: one worker's `git add` swept the other's files into its commit, and both appended
  to `docs/REVIEW-LOG.md` at EOF and collided on merge. **Read-only agents are safe alongside
  anything** — the hazard is writers, and specifically writers sharing an index and a branch ref.
* **A worktree you lint and then delete leaves its findings in the cache your next run reads.**
  `GOLANGCI_LINT_CACHE` — `/root/.cache/golangci-lint` by default — is shared by every worktree in
  one session's container, and the key is the package's import path plus its files' *content*, with
  the module directory stripped out of each filename, while the cached diagnostics themselves still
  carry the absolute paths they were recorded at (read in golangci-lint v2.12.2,
  `internal/cache/cache.go`, `computePkgHash`; that is the pinned gate binary at
  `/root/go/bin/golangci-lint`). So a dead tree's findings are a cache *hit* for the identical files
  in the live one: on 2026-08-19 two of one session's own deleted scratch worktrees replayed into
  **11 phantom issues and a red `lint-go`** on an otherwise unmodified tree, cleared only by removing
  23 cache entries — and sibling agents linting at the same time write that one cache too. This is a
  hazard inside your own session, not one another thread can inflict on you; threads share `origin`,
  not a filesystem. So: if you run a lint gate inside a throwaway worktree, point
  `GOLANGCI_LINT_CACHE` at a directory inside that worktree or do not run the gate there at all, and
  end the worktree with `git worktree remove` rather than deleting the directory, so nothing is left
  registered or half-referenced. **The symptom is `lint-go` reporting issues at paths that do not
  exist**, on a tree where those same files lint clean.
* **A sequential id read out of a file is a race, not a lookup.** `M5-NN` entry ids, `M5.N` subsection
  numbers, ADR numbers and migration numbers are all allocated by reading the highest one already
  present, and **two agents that read at the same moment both get the right answer and both are wrong
  by the time they write.** One day produced three collisions in `docs/REVIEW-LOG.md` alone:
  `952a472` restarted at `M5.14`/`M5.15` after `be95357` had already used both; `d64b8fc` wrote
  `M5.19`–`M5.22` into a tree (`afe17fb`) that **already contained** `M5.19`–`M5.22` from `6f33464`,
  because it had read the number before its own branch point moved under it; and `e7c3b0a` restarted
  at `M5.29`–`M5.31` over `b2dc092`'s. **Neither party was wrong when it looked.** So: **re-read the
  highest id immediately before you commit rather than when you start** — after your last fetch or
  merge, against the tree you are actually about to push — and for ids that are also filenames or
  cross-file references (`docs/DECISIONS.md` ADR numbers, `internal/db/migrations`) check **every
  remote head**, not just your own, the way the `0040` check in `docs/REVIEW-LOG.md` did with
  `git ls-remote` and a `grep` per head. Renumbering your own section before you push is cheap;
  renumbering it after it has landed is a cascade through every cross-reference in the file, and
  §6.1's invariant means the entry's id, text and severity may not move with it.
* **A `docs/REVIEW-LOG.md` entry id carries the prefix of the thread that wrote it, and the generic
  `M5-` prefix is retired for new entries.** This is the amendment to the bullet above, and it is
  narrow: `M5.N` subsection numbers, `docs/DECISIONS.md` ADR numbers and migration numbers are still
  shared counters and still want the re-read-before-you-push discipline. Entry ids no longer do.
  **The reason is the measured one**: `M5-` is a shared counter with no lock — three threads collided
  on it in one evening, one pass colliding twice (`M5-33`→`M5-35`→`M5-36` in a single pass) — while
  **no per-thread prefix has ever collided**. **An id should be a fact about its author, not a claim
  on a global sequence.**

  🚩 **The rule caught a live collision while it was being written, and that case is sharper than the
  historical ones.** A second thread had drafted `M5-36` for its own entry at the same moment this
  thread's `M5-36` landed as `10e7b00`. **Both had read `M5-35` and both were right when they looked**
  — the collision happened entirely inside the uncommitted window, so no re-read by either party could
  have found it. That is the whole argument in one line: **reading the file is not enough, because a
  shared counter's true value includes what nobody has pushed yet.** A monotonic id cannot be
  allocated safely by reading; it can only be won by racing. A per-thread prefix needs no global read
  to be *correct* — only a cheap one to be *unused* — which is why the prefixes have never collided
  where the counter has three times. The second thread renumbered to `ADRC-`. Three rules make it
  hold:
  * **The prefix is multi-letter.** A one-letter namespace is too small to be a fact about anybody,
    and it has already failed: **`C-01` names two different findings in this file** — round 1's
    *"Typo tolerance does not exist"* and round 4's *"The four library tables and `work_credit` have
    no DDL anywhere"* — because the single-letter prefixes are round-scoped and got reused across
    rounds.
  * **`SYNC-` is reserved and may not be claimed.** `SYNC` is already a Mermaid node id in
    `ARCHITECTURE.md` (*"Sync engine<br/>import · delta · reconcile · write queue"*), and the S
    neighbourhood already carries ten prefixes — `S-`, `SD-`, `SR-`, `SU-`, `SW-`, `SALT-`, `SNAP-`,
    `SSRF-`, `SCOPE-`, `SEC-`. `SW-` is a 28-entry batch a reader would reasonably read as "sync" and
    it is the **frontend bench**, so `SYNC-01` beside `SW-01` is a scanning hazard rather than a
    collision. Pick elsewhere in the alphabet.
  * **Check `main`, then land your first entry promptly.** Measured 2026-08-17 on `10e7b00`:
    **52 live entry-id prefixes on `main`** (51 under `grep -oE '\b[A-Z][A-Z0-9]*-[0-9]{2}\b'`, plus
    `OQ-`, whose ids are single-digit), and **the union across the seventeen non-`main` remote heads
    equals `main`'s set exactly** — no head carries a prefix `main` does not (`git ls-remote --heads
    origin` returned eighteen refs, `main` among them). So checking `main` alone is
    sufficient **today**, and the condition is worth more than the conclusion: it holds *because
    prefixes are allocated by landing*. A thread that claims a prefix on a branch and sits on it has
    made the check unsound for everyone else with nothing anyone can observe. Landing promptly is the
    half of the rule that keeps the cheap check honest.
    ⚠️ **Do not answer this with a declared registry of who holds which prefix.** A roster is a shared
    mutable list — the same defect one level up, needing an edit from every thread that claims a
    prefix and certain to drift from the log it describes. **The log is the registry, derived rather
    than declared**: a grep over the entries themselves cannot go stale against the entries, and a
    hand-maintained list of claimants can and eventually will.

  ⚠️ **Existing `M5-` ids stand as dated records, and history is not renumbered.** This retires the
  prefix for **new** entries only; every `M5-NN` already in the file keeps its id, its text and its
  severity, which is `docs/REVIEW-LOG.md` §6.1's own invariant (*"No existing entry's id, text or
  severity changed"*) and the same rule as *a citation inside a dated record is history, not
  staleness* two bullets above. A sweep renumbering them into per-thread prefixes would destroy the
  cross-references and the evidence at once.
* **Give a throwaway branch a name nobody else will pick.** Reusing something generic like
  `main-merge` can move a ref another worktree is standing on, and git's protection against that is
  uneven — reproduced 2026-08-17 on `git version 2.43.0` in a scratch repo: with `shared` checked
  out in a second worktree, `git branch -f shared main` **refuses** (*"fatal: cannot force update
  the branch 'shared' used by worktree at …"*) and `git worktree add -b shared` refuses on the name
  alone, but **`git checkout -B shared` from the primary checkout succeeds** — *"Switched to and
  reset branch 'shared'"*, exit 0 — moving the other worktree's `HEAD` under it and leaving a
  staged modification there that nobody in that worktree made. The one unprotected verb is the one
  everybody types. Suffix the branch with your agent id or a timestamp.
* **Cite the author commit for when a thing was done and the merge commit for when it reached
  `main`.** They are different commits and swapping them mis-dates history: `c9610e2` *("SD-01a said
  'pinned' where its own evidence said 'unguarded'")* is the work, `cda979c` is the merge that put
  it on `main`. Branches here are also pushed straight to `main` as often as they are merged, so
  plenty of commits are both — which is exactly why the one you cite has to be the one you looked
  at (`git log --first-parent` and `git log -1 --format=%p` settle it in two commands).
* **A SHA read off a log is positional, and position is not authorship.** The bullet above says
  which commit to cite; this says why the log will not hand it to you, in two shapes. **A merge
  wears the change it landed**: `git show --stat` renders a merge against its *first* parent, so the
  merge `7bd45e9` prints `Makefile | 26 ++…`, *1 file changed, 24 insertions(+), 2 deletions(-)* —
  byte for byte the stat of `eb92062`, the commit that did the work. The `Merge:` header line is the
  only tell and `git log --oneline` does not carry it. **A neighbour wears it too, with no merge in
  the citation at all**: `0ca1be6` (05:03:47) reached `main` under merge `b2221df` (05:04:05), and
  `bf66828` landed 31 seconds after that at 05:04:36, so a `git log --oneline` taken in that minute
  showed `bf66828` on top with the rewrite two rows down behind a merge — and the SHA that arrived
  *after* the change was read as the SHA that *was* it. The 🔍 note above carries that case and its
  correction; do not re-derive it. Either citation is true in the reachability sense and false in
  the this-commit-contains-it sense. **The discriminator is the path, and it is the actionable
  half.** `git log --no-merges -- <path>` names the commit that contains the change and nothing
  else; plain `git log -- <path>` is usually enough, because history simplification drops a merge
  that is TREESAME to a parent for that path — it does prune `7bd45e9` from `git log -- Makefile` —
  but it is **not** airtight: `cb0e37f` differs from *both* its parents in
  `docs/reference/security.md` (`+10` against one, `+2/-1` against the other), so simplification
  keeps it and it appears in that log looking exactly like an author. For one hunk rather than one
  file, `git log -L 806,812:Makefile` answers `eb92062` on its own.

Who leads which area, roughly. **The map is keyed by area of the repo, not by thread name**, and
deliberately so: thread names churn — the three this table first named are now five, spread across
the sixteen branches now on the remote — while the areas outlive all of them. Find your row by the
paths you are about to touch, not by what your thread is called.

| Area | Led by the work that |
|---|---|
| `internal/` and `cmd/` — the Go backend | lands backend features and their tests |
| `web/` — the SvelteKit SPA and its browser checks | lands screens and frontend behaviour |
| `api/specs/` — the vendored upstream specs and `SOURCES.md` | lands the client that consumes the spec, today `internal/servarr`. A spec arrives with the code that reads it and the contract test behind it, never ahead of either, so what `SOURCES.md` says about which specs are present is a fact about the backend |
| implementation-status wording in `CLAUDE.md`, `README.md` and `ARCHITECTURE.md` §16 | landed the code being described |
| `ARCHITECTURE.md` §17 and `docs/design/` | owns the screens and the visual system |
| `docs/CONFIGURATION.md` | lands `internal/` and `cmd/`. The onboarding bullet above already binds the §2 table, `.env.example` and `internal/config` into one commit, and that binding decides this row: the file moves with the code that reads the keys |
| `docs/DEVELOPMENT.md` — this file | lands `internal/`, `cmd/` and the build. Almost every fact here is a `Makefile` target or a path the build produces, so the change that makes a target true is the change that corrects the sentence describing it |
| `docs/PROJECT-INSTRUCTIONS.md` | maintains the project-settings text |
| `docs/REVIEW-LOG.md` | ran the adversarial review that produced the finding |
| `docs/reference/` — the per-subsystem design notes | **has no fixed lead, and that is the answer rather than a gap in this table** — it follows whichever change drove it. Each file goes to the work that lands the code it describes; where a note pins itself to the migration state, it goes to whoever lands the next migration, because that is the commit that falsifies it |

**These are leads, not exclusive ownership**, because in practice edits cross the lines routinely.
`README.md`, `CLAUDE.md` and `ARCHITECTURE.md` are shared documents; a §17 change routinely lands
§8.x amendments alongside it; and `docs/reference/` follows whichever change drove it. The map says
who to talk to, not who is permitted to type.

---

## 12. Deploying to a server — build, install, restart

**This is the only install path that works today.** No image is published; no `deploy/Dockerfile` is
in the tree; no systemd unit file ships in this repo. What UsArr produces is a single static binary,
so deploying it is exactly three moves: build it, copy it onto `PATH`, restart the service that runs
it. `deploy/update.sh` does all three and then proves they took effect; `deploy/status.sh` answers
"am I running the latest `main`?" without changing anything.

The README's Compose block is flagged **illustrative only** for that reason (`README.md`,
*Quickstart*). Do not read it as a working Docker path.

**Known-good target**, from the owner's own deployment rather than from a template: **Debian 13 LXC
on Proxmox**, checkout at `/opt/UsArr`, binary at `/usr/local/bin/usarr`, systemd unit named
`usarr`. Nothing below is specific to Proxmox — any Linux host with the §1 toolchain and a service
manager works — but that is the shape it has actually been run on, and it is what the two scripts
default to.

### 12.1 The update path, from an existing checkout

```bash
/opt/UsArr/deploy/update.sh          # or: make -C /opt/UsArr deploy
```

That is the whole procedure. The script fetches, fast-forwards, builds, installs and restarts, and
**refuses to half-update**: every step is checked, and each failure names what state the host was
left in — including the one failure that genuinely does leave a half-updated host, an `install` that
succeeded followed by a `systemctl restart` that did not. Pass `--check` to find out whether an
update is available without applying one; it still runs `git fetch`, because the question cannot be
answered without one, but it touches nothing in the working tree and builds nothing.

**Untracked files do not stop it, modified tracked files do**, and the split is deliberate rather
than lenient. A server accumulates scratch files — the owner's checkout carries a
`kavita-watermark-probe.sh` in its root — and an untracked file cannot affect a fast-forward or be
lost by one, so aborting on it would only teach the operator to pass `USARR_ALLOW_DIRTY=1` on every
run, disabling the half of the check that matters. A modified **tracked** file can make the merge
fail or lose work, so that is the case that stops the script and the case `USARR_ALLOW_DIRTY=1`
overrides. The build output is gitignored (`.gitignore`: `/usarr`), so it is not untracked cruft at
all; if that entry ever goes missing, `update.sh` says so by name rather than letting every run
report a tree that looks dirty.

**Why a script rather than three commands.** All three commands below can succeed while the live
process keeps running the old build, and none of them says so. The checkout can merge instead of
fast-forwarding and end up on a commit that is on no branch anywhere; `make build` can be skipped in
favour of `go build` and produce a binary with no SPA in it; `install` replaces the destination inode
while the running process keeps the one it already has open, so a forgotten `systemctl restart` looks
exactly like a successful deploy. That last one is the failure that prompted this: the owner's host
repeatedly ended up not running `main`, with nothing on the machine willing to say so. The script's
final act is therefore not "restart succeeded" but **"the running process reports commit `X`, `X` is
what the checkout is on, and it is still that process three seconds later"** — read out of the
process's own startup log line, not off the disk.

That last clause is not padding. `cmd/usarr/main.go` logs `starting UsArr` **before** `buildApp`
opens the database and runs migrations, and before the listener binds — so a port clash, a failed
migration or an unreadable master key emits a perfectly good startup line carrying the new commit and
*then* exits. Checking only the log line reports `update: OK` over a service that is already dead or
crash-looping, which was measured, not theorised. The script therefore re-reads `is-active` **and**
`MainPID` after a short settle: a state that is no longer `active` means it started and died, and a
`MainPID` that has moved means systemd is restarting it as fast as it dies.

**The underlying three steps**, still true and still worth knowing, because that is what the script
runs and what you fall back to when something in it breaks:

```bash
cd /opt/UsArr && git pull && make build
install -m755 usarr /usr/local/bin/usarr
systemctl restart usarr
```

**No `sudo`, deliberately** — in the script and here alike. The documented target is an LXC
administered as root directly, where `sudo` is not installed at all; prefixing these commands with it
fails outright, which is a real failure that has happened rather than a hypothetical. Because the
binary is absent rather than the rights, the shell answers `sudo: command not found` and not a
permission refusal, and it answers it on the **second and third lines**, after the `make build` above
them has already succeeded — so the transcript reads like a build failure and is not one. **On a host
where you are not root**, add `sudo` to the **last two lines only** (or run the script itself under
`sudo`, accepting the same caveat): `git pull` and `make build` should run as the user that owns the
checkout, because building as root leaves root-owned objects in `/opt/UsArr` and `web/node_modules`
that the next unprivileged build cannot overwrite.

Two details worth knowing rather than rediscovering:

* `make build` writes the binary to **`./usarr` inside the checkout** (`Makefile`'s `BINARY`), and
  stops there. Nothing in the Makefile installs, and no target knows about `/usr/local/bin` — the
  `install -m755` line is the step that puts the new build on `PATH`, and it is not optional.
* `install` replaces the destination by creating and renaming, so a **running** process keeps the
  old inode it already has open. The new binary is not live until `systemctl restart usarr`. **Do not
  infer from `usarr --version` that the service is running that version**: the flag reports the
  identity of the binary on disk that you just invoked, which after an `install` is precisely the one
  the running process is *not* executing. `deploy/status.sh` exists because that distinction cannot
  be collapsed — it reads the disk and the process separately and compares them.

There is no first-install variant of this to document, because there is no first-install artifact to
document it against: the very first deployment was this same sequence preceded by a `git clone` into
`/opt/UsArr` and a unit file written by hand on the host (see §12.4).

### 12.1.1 "Am I current?", in one command

```bash
/opt/UsArr/deploy/status.sh          # or: make -C /opt/UsArr deploy-status
```

Read-only. It writes nothing and restarts nothing; its one side effect is the `git fetch` that makes
"how far behind?" answerable at all, and `USARR_NO_FETCH=1` drops even that.

It prints the checkout's branch, `HEAD`, `origin/main` and commit distance, then **two** file counts —
modified tracked files and untracked files, never one combined "dirty" number — then the installed
binary's path, mtime and `--version` output, and then the service's active state, `MainPID`,
`/proc/<pid>/exe` and the commit from its startup journal line. It ends on one verdict line. Only the
tracked count reaches that verdict: a scratch file sitting in the checkout says nothing about whether
the install is current, and letting it colour the verdict would make a healthy host read as broken.

**Three links, reported separately, because they go stale independently**: the checkout can be behind
`origin`, the installed binary can be older than the checkout, and the running process can be older
than the installed binary. A `(deleted)` suffix on `/proc/<pid>/exe` is the signature of the third —
the binary was replaced under a process that was never restarted — and the script calls it out by
name. Exit status is `0` when all three agree, `1` when something is stale, and **`2` when a reading
could not be taken**: an absent `systemctl`, an unreadable journal or a binary too old to answer
`--version` cannot confirm anything, and reporting "up to date" off a partial reading is the false
green the script exists to remove. A unit that is not `active` is `1`, not `2` — a service that is
down is an answer, not a missing reading, and letting it fall through to "unverified" would bury it.

The journal line is selected by **`_PID` of the running process, read forwards**, rather than out of
the last *N* lines of the unit. The distinction is load-bearing: `starting UsArr` is logged exactly
once, at start, so any fixed window drops it as soon as the host logs past that window, and a fully
current deployment then reports `UNVERIFIED` forever after. That was measured against an `-n 2000`
window and is the reason the query is shaped the way it is; raising the number only moves the
threshold. There is a windowed `-u` fallback for a unit whose logging PID is not `MainPID`.

### 12.1.2 `usarr --version`, and what the scripts do with it

```
$ usarr --version
usarr e77c4ad
commit: e77c4ad
built:  2026-08-17T20:05:43Z
go:     go1.25.13
```

That is a real capture, and the first line looks the way it does for a reason worth knowing: `VERSION`
is `git describe --tags --always --dirty`, **this repo has no tags at all** (`git tag` is empty), so
`describe` falls through to `--always` and yields the bare short SHA — the same value as `commit:`.
The day someone cuts the first tag the first line becomes `usarr v0.1.0-12-gabc1234` and `commit:`
stays a bare short SHA; the scripts read only `commit:`, so that change does not affect them. Do not
copy a `v…` string out of this block today expecting to see one — no release has been tagged.

The values are the ones `-ldflags` stamps in (`Makefile`'s `LDFLAGS` → `main.version`, `main.commit`,
`main.buildDate`), so `commit:` is the short SHA the checkout was on **at build time** — which is
exactly what makes it useful as evidence. The flag short-circuits inside `config.Load` before any
environment is read, any directory is resolved or created, or the master key is touched, so it
answers for an unprivileged caller with no config directory and no environment at all; asking an
installed binary what it is must never depend on being able to start the service.

The one-per-line `key: value` shape is a contract, not decoration: `update.sh` compares `commit:`
against the checkout's short `HEAD` to catch a build that silently produced a stale artifact, and
`status.sh` greps the same line. `TestPrintVersionShape` in `cmd/usarr/version_test.go` pins it.

### 12.1.3 Environment the scripts read

Both scripts are configured entirely through the environment, and both default to the known-good
target above, so on the owner's host neither needs an argument.

| Variable | Default | Meaning |
|---|---|---|
| `USARR_CHECKOUT` | `/opt/UsArr` | The git checkout to update or inspect. `make deploy` / `make deploy-status` pass `$(CURDIR)` instead, because through `make` the checkout in question is by definition the one the Makefile is in. |
| `USARR_INSTALL_PATH` | `/usr/local/bin/usarr` | Where the built binary is installed to. |
| `USARR_SERVICE` | `usarr` | The systemd unit name. |
| `USARR_BRANCH` | `main` | The branch `HEAD` must be on; also what `origin` is compared against. |
| `USARR_ALLOW_DIRTY` | unset | `update.sh` only: `1` builds despite locally modified **tracked** files instead of aborting. The abort lists them. **Untracked files never block an update** and this variable has nothing to say about them. |
| `USARR_NO_FETCH` | unset | `status.sh` only: `1` skips `git fetch` and compares against whatever `origin/$USARR_BRANCH` is already on disk. |

### 12.2 What the server needs before its first `make build`

`make build` is a real build, not a download. The server needs the **Go, Node and pnpm floors from
§1** — the target runs `web-build` → `web-deps` → `pnpm install --frozen-lockfile`, builds the SPA,
then compiles the Go binary with the SPA embedded.

It does **not** need `make tools`. The five pinned binaries (`gofumpt`, `golangci-lint`, `goose`,
`govulncheck`, `gitleaks`) are prerequisites of `make check`, not of `build`; a deployment host that
only ever builds does not need them. It needs no Docker daemon either (§8), and no network beyond
what `git pull` and `pnpm install` fetch.

**Three build traps a fresh operator will hit.** Each is documented in exactly one place already;
read it there rather than trusting a paraphrase, because a second copy is a copy that drifts:

| Trap | Where it is documented |
|---|---|
| **`go build ./cmd/usarr` by hand produces a binary that 404s on `/`.** The SPA is embedded only by `make build`, which runs `web-build` first; a hand-rolled `go build` bypasses that dependency and silently ships an empty frontend. The underlying `//go:embed all:` rule — plain `//go:embed dist` drops the whole application and embeds the favicon — is the same trap one layer down | §3, *Two-process dev loop*, and §3's `USARR_REQUIRE_WEB_BUILD` guard. Mechanism: `docs/DECISIONS.md` ADR-0025 point 6 |
| **corepack's interactive download prompt**, which stalls an unattended build until someone answers it | §9, *Frontend*. The `Makefile` exports `COREPACK_ENABLE_DOWNLOAD_PROMPT := 0` so its own builds cannot hang; a build you drive by hand from `web/` will not have that unless you export it too |
| **The pnpm pin.** `web/package.json` pins `packageManager: pnpm@10.33.0`; the repo root has no `package.json`, so `pnpm -C web …` resolves the launcher against a directory with no pin, falls back to a default version, and then hard-fails on the mismatch. Run from **inside `web/`** | §9, *Frontend*, and the comment block above `web-deps` in the `Makefile` |

### 12.3 Back up before you upgrade — both artifacts, or neither is any use

`docs/CONFIGURATION.md` §6.1 is authoritative and states the split; the one thing to carry into a
deploy checklist is that **there are two artifacts, not one**. As of `9eec372`, `kek.salt` sits
**beside `usarr.db`** in the config directory (it used to live under `keys/`), and `keys/secret.key`
is the master key. `KEK = HKDF-SHA256(secret.key, salt=kek.salt, …)`, so an archive that misses
**either** one leaves every stored service credential permanently unopenable — the database still
opens, and every *Arr API key in it is noise.

They belong in **different** places: the database archive includes `kek.salt` and excludes `keys/`;
the master key goes to a password manager, once. If you are upgrading across `9eec372`, take a fresh
database archive after the first restart — older archives taken with `--exclude='keys'` do not
contain the salt, because it was inside the excluded directory. §6.1 has the exact commands.

### 12.4 What this repo does not ship, stated plainly

Nothing in this section is a gap someone forgot to fill in; each is a real absence you will notice
when you look for it.

* **No systemd unit file.** The unit named `usarr` on the owner's host was written by hand there and
  exists nowhere in this tree. `systemctl restart usarr` above assumes you have written one, and
  `deploy/update.sh` fails loudly rather than skipping the restart when no such unit exists.
* **No *first*-install script.** `deploy/` now exists and holds `update.sh` and `status.sh`, but both
  operate on a checkout that is already there, next to a service that is already defined. Neither
  clones the repo, writes a unit file, creates a user or provisions a config directory; there is
  still no path from a bare host to a running UsArr that is not done by hand.
* **`make docker` cannot succeed on any checkout, because the file it builds from is absent.** The
  recipe passes `-f deploy/Dockerfile`, and that path does not exist in the repo. The `Makefile`'s
  own header says so; §4's target table now says so too. The daemon requirement (§8) is the second
  obstacle, not the first.

**A recommendation, deliberately left unwritten.** A checked-in `deploy/usarr.service` — with the
`User=`, `WorkingDirectory=`, `Environment=USARR_CONFIG_DIR=…` and `Restart=on-failure` settings the
owner's host already encodes somewhere — would be genuinely useful, and would turn §12.1 into a
sequence a second operator could follow end to end. It would also be a new shipping artifact, which
is a code change and not a documentation one. It is noted here rather than invented.
