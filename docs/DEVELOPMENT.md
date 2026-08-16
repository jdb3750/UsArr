# UsArr Development Guide

> **Status: pre-alpha, zero lines of application code.** This document describes the intended
> toolchain, layout, and workflows. Commands that reference files or packages which do not exist yet
> are marked **(not yet)**. Nothing here is a description of working software; it is the contract
> the first commits are expected to satisfy.

---

## 1. Prerequisites

| Tool | Version | Why this floor | Install |
|---|---|---|---|
| **Go** | **1.24+** | The toolchain in the reference container is `go1.24.7`. Uses `go tool` directives, `embed.FS`, and generics-heavy driver code. | <https://go.dev/dl/> or your package manager |
| **Node.js** | **22+ (LTS)** | SvelteKit 2 + Vite 5/6 baseline. Verified in-container: `v22.22.2`. | <https://nodejs.org/> or `fnm`/`nvm` |
| **pnpm** | **10+** | Package manager for `web/`. Verified in-container: `10.33.0`. | `corepack enable && corepack prepare pnpm@latest --activate` |
| `git` | any recent | — | — |
| `make` | GNU make | The `Makefile` uses GNU-isms (`.PHONY`, `?=`). BSD make will not work. | — |

Optional, installed on demand by `make tools` **(not yet)**:

| Tool | Purpose |
|---|---|
| `gofumpt` | Stricter `gofmt`. The formatting gate. |
| `golangci-lint` v2 | Meta-linter. **v2 config format** — `.golangci.yml` must start with `version: "2"`; v2 binaries cannot parse v1 configs. |
| `goose` | SQL migration runner (`github.com/pressly/goose/v3`). |
| `govulncheck` | Vulnerability scan against the Go vuln DB. Advisory in CI, not a gate. |

**There is no FFmpeg dependency, and there never will be.** UsArr does not stream, transcode,
remux, or serve media bytes — it routes clients to the backend that owns them (Jellyfin, Navidrome,
Audiobookshelf, Komga/Kavita). Do not add a media-processing dependency, a `ffmpeg`/`ffprobe` shell
out, or a codec library. If a feature seems to need one, it belongs in a backend, not here. (FFmpeg
is also absent from the agent container, so such a dependency would break the build immediately —
but the reason it is banned is architectural, not environmental.)

**No CGO, ever.** UsArr builds with `CGO_ENABLED=0` and produces a static binary.
`ncruces/go-sqlite3` is a WebAssembly build of SQLite executed by a pure-Go runtime, which is
precisely why it was chosen over `mattn/go-sqlite3`. If you find yourself needing a C toolchain,
something has gone wrong — stop and raise it.

⚠️ **Watch item:** `ncruces/go-sqlite3` currently runs its SQLite Wasm on **wazero**, but upstream
has signalled a move to `wasm2go` in a future release. That is a transitive-dependency change, not
an API change, but it may affect binary size and startup cost. Track
<https://github.com/ncruces/go-sqlite3/discussions/361>. Note this is *separate* from UsArr's own
use of wazero/Extism to host WASM plugins — that stays.

---

## 2. Repository layout (target)

```
UsArr/
├── cmd/
│   └── usarr/                  # main package. Flag parsing, wiring, graceful shutdown.
├── internal/
│   ├── config/                 # env/file/flag loading + precedence (docs/CONFIGURATION.md §1)
│   ├── crypto/                 # AES-256-GCM envelope encryption, key wrap/rotate, Argon2id
│   ├── db/
│   │   ├── migrations/         # NNNN_name.sql, goose format, embedded via embed.FS
│   │   └── sqlite.go           # open + PRAGMA journal_mode=WAL, busy_timeout, foreign_keys
│   ├── httpapi/                # REST handlers, middleware (auth, CSRF, rate limit, trusted proxy)
│   ├── servarr/                # ONE client for Sonarr/Radarr/Lidarr/Readarr/Prowlarr/Whisparr
│   │   └── mapping/            # per-media-type mapping onto the unified schema
│   ├── lazylibrarian/          # separate: cmd= RPC, HTTP 200 + {"Success":false} errors
│   ├── jellyfin/               # video playback handoff + identity delegation
│   ├── navidrome/              # music, over OpenSubsonic (apiKeyAuthentication ONLY)
│   ├── audiobookshelf/         # audiobooks; ABS owns listening position, UsArr mirrors it
│   ├── komga/                  # comics/manga; X-API-Key, OPDS 1.2 + 2.0
│   ├── kavita/                 # comics/ebooks
│   ├── downloadclient/         # sabnzbd, nzbget, qbittorrent, transmission, deluge
│   ├── requests/               # request → route to the right *Arr by media type
│   ├── tsnet/                  # embedded Tailscale node + WhoIs-based identity
│   ├── metadata/               # tmdb, tvmaze, musicbrainz, openlibrary, wikidata, anilist
│   ├── search/                 # unified search; FTS5 default, meilisearch plugin
│   ├── tagging/                # namespaced tags, aliases, virtual parents, rule engine
│   ├── plugin/                 # wazero/Extism host, capability grants, sandboxing
│   ├── ssrf/                   # outbound HTTP policy: DNS pinning, no redirects, range denial
│   └── web/                    # embed.FS wrapper over web/build
├── api/
│   └── specs/                  # VENDORED upstream OpenAPI specs — see §7.3
│       ├── sonarr.v3.json  radarr.v3.json  lidarr.v1.json
│       ├── readarr.v1.json prowlarr.v1.json whisparr.v3.json
│       └── SOURCES.md          # URL + commit/date each spec was pulled from
├── web/                        # SvelteKit, adapter-static, SPA fallback
│   ├── src/
│   ├── static/
│   └── build/                  # pnpm build output -> embedded by internal/web (gitignored)
├── testdata/
│   ├── cassettes/              # go-vcr recordings, secrets scrubbed (§7.2)
│   └── releases/               # real release-name corpus for the parser tests
├── deploy/
│   ├── Dockerfile
│   └── compose/
│       ├── usarr.yml           # UsArr alone
│       └── dev-stack.yml       # full *Arr stack for RECORDING fixtures (§7.1)
├── docs/
├── Makefile
├── .env.example
└── .gitignore
```

Two rationales worth stating once, because they are design decisions and not conventions.

**UsArr is northbound-thin and southbound-wide.** `internal/{jellyfin,navidrome,audiobookshelf,
komga,kavita}` are read + handoff adapters: they populate the unified library and produce a playback
URL or a deep link. None of them proxy media bytes in v0.1. `internal/requests` is the write path —
it takes "I want this" and routes it to Sonarr/Radarr/Lidarr/LazyLibrarian by media type, which is
why the *Arr clients are the ones with real write coverage.

**`internal/servarr` is one client, not six.** Sonarr, Radarr, Lidarr, Readarr, Prowlarr, and
Whisparr are forks of the same codebase; auth, paging, tags, commands, SignalR, `/ping`, and
`system/status` are byte-for-byte identical contracts. What differs is the API version path
(`/api/v3` vs `/api/v1`) and the resource shapes, which belong in `mapping/`. Writing six clients
would be six times the maintenance for one contract.

---

## 3. Getting running

```bash
git clone <repo> && cd UsArr
cp .env.example .env
# Generate a real master key — .env.example ships a placeholder that will not work:
printf 'USARR_SECRET_KEY=%s\n' "$(openssl rand -base64 32)" >> .env

make tools          # install gofumpt, golangci-lint, goose  (not yet)
make dev            # backend on :8484 with the SPA proxied   (not yet)
```

### Two-process dev loop (the normal one)

The backend and the frontend run separately in development, and are only combined at build time.

```bash
# Terminal 1 — Go backend, hot reload on .go changes
make dev            # -> http://localhost:8484

# Terminal 2 — Vite dev server with HMR
make web-dev        # -> http://localhost:5173, proxies /api -> :8484
```

In this mode `internal/web` serves nothing; you develop against Vite on :5173 and it forwards API
calls to :8484. The SPA is only baked into the binary by `make build`, which runs `make web-build`
first and embeds `web/build` through `embed.FS`. **Consequence: `make build` without a prior
`web-build` produces a binary that serves a 404 for `/`.** The Makefile wires the dependency so this
does not happen by accident, but it will bite you if you run `go build ./cmd/usarr` by hand.

### First run

With no `USARR_SECRET_KEY` and an empty database, UsArr generates a key, writes
`$USARR_CONFIG_DIR/secret.key` at mode 0600, and prints it once. In dev, set the key in `.env`
instead so it is stable across `rm -rf ./config`.

---

## 4. Make targets

| Target | Does |
|---|---|
| `make dev` | Run the backend with live reload, reading `.env`. |
| `make web-dev` | Run the SvelteKit dev server with HMR. |
| `make build` | `web-build`, then a static `CGO_ENABLED=0` binary at `./usarr` with the SPA embedded. |
| `make web-build` | `pnpm build` in `web/` → `web/build`. |
| `make test` | `go test ./... -race` plus `pnpm test` in `web/`. No network, no Docker. |
| `make test-integration` | Tests behind the `integration` build tag. Requires a live stack; **not run in CI**. |
| `make lint` | `golangci-lint run`, `svelte-check`, `eslint`. |
| `make fmt` | `gofumpt -w`, `prettier --write`. Mutates files. |
| `make migrate` | Apply pending migrations to the dev database. |
| `make migrate-new name=add_tag_rules` | Scaffold a timestamped migration pair. |
| `make docker` | Build the container image locally. Needs a Docker daemon — see §8. |
| `make clean` | Remove `./usarr`, `web/build`, `web/.svelte-kit`, coverage output. |
| `make check` | **The pre-commit gate**: `fmt-check` + `lint` + `test`. Must pass before every commit. |

`make check` runs `fmt-check` (verify-only), not `fmt` (rewrite), so it never mutates your tree
while telling you it passed.

---

## 5. Testing

### Layers

| Layer | Location | Network? | Runs in CI |
|---|---|---|---|
| Unit | `*_test.go` beside the code | No | Yes |
| Golden/table (parsers, mappers, tag rules) | `testdata/` | No | Yes |
| Replayed HTTP (go-vcr cassettes) | `testdata/cassettes/` | No | Yes |
| Contract (vendored OpenAPI specs) | `api/specs/` | No | Yes |
| Migration round-trip | `internal/db` | No | Yes |
| Integration (live services) | `//go:build integration` | Yes | **No** |
| Frontend unit | `web/src/**/*.test.ts` (Vitest) | No | Yes |

```bash
make test                                  # everything CI runs
go test ./internal/servarr/... -run Sonarr -v
go test ./... -race -coverprofile=cover.out && go tool cover -html=cover.out
USARR_INTEGRATION=1 make test-integration  # only with a live stack
```

Always `-race` for the backend. UsArr holds long-lived SignalR connections, a webhook receiver, and
background sync jobs writing the same SQLite handle; data races are the expected failure mode.

### Rules

* **A test that needs the network is not a unit test.** If it reaches the internet, it is behind the
  `integration` tag or it uses a cassette.
* **Never commit a real API key.** Cassettes are scrubbed by a recorder hook before they touch disk
  (§7.2). The pre-commit gate should grep cassettes for 32-hex-character strings.
* **Test the error envelopes, not just the happy path.** This ecosystem's failure modes are unusual
  and are where integrations actually break:
  * *Arr: bad key returns **401**, never 403 — the handler returns `NoResult()`, not `Fail()`.
  * *Arr: `Accept: text/html` returns **406** (`ReturnHttpNotAcceptable = true`). Send
    `Accept: application/json` or nothing.
  * Prowlarr: query/grab limit breaches have historically returned **HTTP 200 with an error in the
    body**; upstream indexer 429s surface as generic connection failures. Treat Prowlarr search
    failures as soft and read `indexerstatus.disabledTill`.
  * Prowlarr: grabbable releases are cached server-side for **30 minutes**. A test that persists a
    release and grabs it later must assert the failure, not the success.
  * LazyLibrarian: returns **HTTP 200 with `{"Success": false, "Error": {...}}`**. Inspect the body,
    never the status code. Keys must be exactly 32 chars or you get `503 Invalid API key`.
  * Transmission: first RPC call returns **409 + `X-Transmission-Session-Id`** and must be retried
    with that header. Bake the handshake into the client's test.
  * qBittorrent: cookie session that expires; assert re-login on 403.
  * Sonarr webhooks: `eventType: "Download"` has **two different body shapes** (single import vs
    import-complete). Discriminate on `episodeFile` vs `episodeFiles`. Both shapes need a fixture.
* **Type normalisation deserves its own tests.** `imdbId` is a string `"tt0117951"` in Sonarr/Radarr
  and an int `117951` in Prowlarr. `indexerFlags` is an int bitmask in Sonarr/Radarr and a
  `string[]` in Prowlarr. `tags` are `int[]` in the REST API and **string labels** in webhooks.
  `Protocol` in *Arr history `data` is the **integer** enum value (`0` unknown, `1` usenet,
  `2` torrent) while every REST resource uses the string form. Each of these is a one-line bug and a
  one-line test.

---

## 6. Database and migrations

SQLite via `ncruces/go-sqlite3`, WAL mode, opened with (at minimum):

```
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;
```

`foreign_keys` is off by default in SQLite and is per-connection, so it must be set on **every**
connection in the pool, not once at startup.

### Workflow

```bash
make migrate-new name=add_tag_rules
# -> internal/db/migrations/20260816T120000_add_tag_rules.sql
make migrate            # apply to the dev DB
make test               # migration round-trip test runs here
```

Rules:

* Migrations are **embedded** (`//go:embed migrations/*.sql`) and applied automatically at startup.
  A binary is always able to bring its own database forward without an external tool.
* Migrations are **forward-only (proposed)**. Write a `-- +goose Down` block anyway — it is the
  cheapest way to test a migration locally — but downgrades are not a supported user path, and the
  restore procedure in `docs/CONFIGURATION.md §7` says so.
* **Never edit a migration that has shipped.** Add a new one.
* SQLite cannot drop or retype a column in older versions and its `ALTER TABLE` support is narrow.
  For anything beyond `ADD COLUMN`/`RENAME`, use the 12-step table rebuild
  (create new → copy → drop old → rename), inside a transaction, with `PRAGMA foreign_keys=OFF`
  around it. Write the rebuild explicitly; do not hope.
* The round-trip test runs every migration against an empty in-memory DB, then asserts the resulting
  schema matches a checked-in `schema.sql` snapshot. This catches "works on my dev DB because it was
  created three migrations ago" drift.

---

## 7. Testing against real services without owning an *Arr stack

This is the hard part of this project, and there is no single answer. Four mechanisms, in the order
you should reach for them.

### 7.1 Docker Compose fixture stack — for *recording*, not for CI

`deploy/compose/dev-stack.yml` **(not yet)** brings up the ecosystem on a developer workstation:

```yaml
# sketch — ports match the defaults UsArr assumes
services:
  sonarr:     { image: lscr.io/linuxserver/sonarr:latest,     ports: ["8989:8989"] }
  radarr:     { image: lscr.io/linuxserver/radarr:latest,     ports: ["7878:7878"] }
  lidarr:     { image: lscr.io/linuxserver/lidarr:latest,     ports: ["8686:8686"] }
  prowlarr:   { image: lscr.io/linuxserver/prowlarr:latest,   ports: ["9696:9696"] }
  jellyfin:   { image: lscr.io/linuxserver/jellyfin:latest,   ports: ["8096:8096"] }
  navidrome:  { image: deluan/navidrome:latest,               ports: ["4533:4533"] }
  audiobookshelf: { image: ghcr.io/advplyr/audiobookshelf:latest, ports: ["13378:80"] }
  komga:      { image: gotson/komga:latest,                   ports: ["25600:25600"] }
  kavita:     { image: jvmilazz0/kavita:latest,               ports: ["5000:5000"] }
  sabnzbd:    { image: lscr.io/linuxserver/sabnzbd:latest,    ports: ["8080:8080"] }
  qbittorrent:{ image: lscr.io/linuxserver/qbittorrent:latest,ports: ["8081:8080"] }
```

⚠️ Image names and internal port mappings for the four southbound backends are from general
knowledge, not verified in this pass — check each project's own compose example before committing
this file.

The awkward part is **seeding**. A fresh *Arr has an empty library and a random API key, so the
stack is not reproducible on its own. Two ways to fix that, both worth having:

1. **Commit seeded config volumes.** Start each app once, disable authentication, set a *known
   fixture* API key in `config.xml`, add a handful of series/movies, then commit the resulting
   `config/` directories under `deploy/compose/seed/`. The key is fake and public **by design**, and
   the seed dirs must never be mounted anywhere real. `config.xml`'s element names are stable
   (`ApiKey`, `Port`, `UrlBase`, `AuthenticationMethod`, `AuthenticationRequired`), so this is
   scriptable.
2. **A `make seed` script** that drives each app's own API to add a fixed set of items after
   startup, so the seed is regenerable rather than a binary blob.

Because *Arr apps disable Swagger outside debug builds (`app.UseSwagger()` is guarded by
`if (BuildInfo.IsDebug)`), you **cannot** fetch `/docs/v3/openapi.json` from a running production
instance. The specs must be vendored (§7.3).

**This stack never runs in CI.** See §8.

### 7.2 Recorded HTTP fixtures (go-vcr) — the default for everyday work

[`dnaeon/go-vcr`](https://github.com/dnaeon/go-vcr) v4 (`gopkg.in/dnaeon/go-vcr.v4`) records real
HTTP interactions to a YAML "cassette" and replays them offline. This is how the *Arr, Jellyfin, and
metadata-provider clients get tested with zero infrastructure.

Loop:

1. Bring up the compose stack (§7.1), or point at your own homelab.
2. Run the test with `USARR_RECORD=1`. The recorder is in record mode; real calls go out; a cassette
   lands in `testdata/cassettes/`.
3. Run it again without the flag. Replay mode. No network.
4. Commit the cassette. CI and every other contributor now run that test with no stack at all.

**Scrubbing is mandatory and must be a recorder hook, not a manual step.** *Arr keys appear in three
places — the `X-Api-Key` header, an `Authorization: Bearer` header, and the `?apikey=` /
`?access_token=` query parameters. SABnzbd's key is *only* ever a query parameter. Jellyfin's token
sits inside a structured `Authorization: MediaBrowser Token="…"` header. Register a `BeforeSave`
hook that rewrites all of them to a fixed placeholder, plus any `Set-Cookie` (qBittorrent's
`SID`/`QBT_SID_*`) and the `X-Transmission-Session-Id` value.

Cassette hygiene:

* One cassette per test, named after it. Cassettes are test fixtures, not a shared corpus.
* Re-record when the upstream contract changes; a stale cassette will happily pass forever against a
  contract that no longer exists. This is go-vcr's real failure mode, and §7.3 is the counterweight.
* Keep them small. Do not record `GET /series` against a 5 000-item library — these endpoints are
  **not paged** and serialise the whole object graph. Seed a handful of items.
* Metadata providers get cassettes too, and there it is not optional: MusicBrainz allows **1 req/s**
  and returns 503 above it, so a test suite that hits it live is a test suite that gets you blocked.

### 7.3 Vendored OpenAPI specs as contract-test sources

Every Servarr app ships an OpenAPI document in its repo. Vendor them into `api/specs/` with a
`SOURCES.md` recording the URL and the date/commit pulled:

```
https://raw.githubusercontent.com/Sonarr/Sonarr/develop/src/Sonarr.Api.V3/openapi.json
https://raw.githubusercontent.com/Radarr/Radarr/develop/src/Radarr.Api.V3/openapi.json
https://raw.githubusercontent.com/Lidarr/Lidarr/develop/src/Lidarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Readarr/Readarr/develop/src/Readarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Prowlarr/Prowlarr/develop/src/Prowlarr.Api.V1/openapi.json
https://raw.githubusercontent.com/Whisparr/Whisparr/develop/src/Whisparr.Api.V3/openapi.json
```

Three distinct uses, all offline:

1. **Schema validation of cassettes.** Replay a cassette and assert each recorded response body
   validates against the spec's schema for that path. This is what makes a stale cassette detectable
   without a live server: if the vendored spec moves and the cassette does not, the test fails.
2. **Struct-coverage tests.** Assert that UsArr's Go structs cover every field the spec marks
   required for the resources UsArr consumes, and that enum-typed fields accept every documented
   value. `DownloadProtocol` (`unknown|usenet|torrent`) is identical across Sonarr, Radarr, and
   Prowlarr and is the backbone of source tagging — pin it with a test.
3. **Drift detection.** A scheduled job (not the PR gate — it needs network) re-downloads the specs,
   diffs them against `api/specs/`, and opens an issue on change. Upstream `develop` moves; UsArr
   should learn about a renamed field from a bot, not from a user's bug report.

Caveats worth knowing before relying on this: Jellyfin also publishes an OpenAPI spec, but the 10.11
schema was reported invalid (malformed 503 response headers), which breaks naive codegen — generate
*types* if useful, but hand-write the ~25 Jellyfin endpoints UsArr actually needs rather than
depending on a generated client. LazyLibrarian has no spec at all: it is a single CherryPy endpoint
with 184 `cmd=` values and an untyped response envelope, so it gets hand-written fixtures and a
hand-written contract test.

### 7.4 Fake servers, for the paths a cassette cannot reach

`httptest.Server` implementations of the handful of endpoints UsArr calls. These prove nothing about
the real world, so use them only where reality is hard to provoke:

* Auth failures (401 with a bad key, 403), 406 on a wrong `Accept`.
* Timeouts, truncated bodies, connection resets, slow-loris responses.
* Transmission's 409 challenge and qBittorrent's session expiry.
* Prowlarr's HTTP-200-with-an-error body; LazyLibrarian's `Success: false`.
* SSRF policy tests: a handler that 302-redirects to `169.254.169.254` must be refused by the
  outbound client, and the test asserts the request never left.

### 7.5 Public demo instances

| Service | Public demo | Usable for tests? |
|---|---|---|
| **Jellyfin** | `https://demo.jellyfin.org/stable` — username `demo`, **empty password**. An `unstable` demo also exists. Status is tracked at <https://status.jellyfin.org/service/stable-demo>. | **Manual exploration only.** Good for eyeballing response shapes and sanity-checking the `Authorization: MediaBrowser` header. Never in automated tests: it is read-only, shared, rate-limited by circumstance, and its uptime is nobody's commitment to you. Do a manual pass, then record a cassette against your own Jellyfin. |
| **Sonarr / Radarr / Lidarr / Prowlarr / Whisparr** | **None found.** Searched 2026-08-16; no official or community public demo with API access exists. | n/a — compose fixtures + cassettes are the only path. |
| **LazyLibrarian** | None found. | n/a |
| **Navidrome** | `https://demo.navidrome.org` — 📄 widely referenced with credentials `demo` / `demo`; ⚠️ **not verified in this pass**. | Manual exploration only, same caveats as Jellyfin. Useful for confirming whether an OpenSubsonic server advertises `apiKeyAuthentication` via `getOpenSubsonicExtensions`. |
| **Audiobookshelf / Komga / Kavita** | ⚠️ Not checked in this pass. Komga and Kavita both ship Swagger UI on a local instance (`/swagger-ui.html` for Komga), which is a better reference than a demo anyway. | Compose fixtures + cassettes. |
| Metadata providers (TMDB, Open Library, MusicBrainz, Wikidata, TVmaze) | Their production APIs are the "demo" — public and reachable. | Use them **once**, to record cassettes, with a compliant `User-Agent`. Never in the normal test loop; MusicBrainz's 1 req/s and Wikidata's `api.php` throttle make live testing actively hostile. |

**Recommended path if you own no *Arr stack at all:** work entirely from committed cassettes and the
vendored specs (§7.2, §7.3), and ask a contributor with a real stack to re-record when you touch a
client. That is a supported workflow, not a degraded one — it is why the cassettes are committed.

### 7.6 Testing the Tailscale path

`tsnet` joins a real tailnet, so it cannot be exercised in a hermetic test. Split it:

* **Unit-testable, and where the bugs will be:** the identity mapping. Given a `WhoIs` result (or its
  absence, for a tagged device), does UsArr resolve the right user, and does it correctly refuse when
  `USARR_TSNET_AUTH_ALLOWED_LOGINS` does not contain the login? Put `WhoIs` behind a small interface
  and test the mapping against fixtures. Same for the `Tailscale-User-Login` header path — including
  the case that matters most: **a request carrying `Tailscale-User-Login` that did not arrive through
  the trusted path must be rejected**, not honoured.
* **Not unit-testable:** actually joining a tailnet. That lives behind the `integration` tag and runs
  on a developer machine with a real (ideally throwaway) tailnet and a non-reusable auth key.
* Default `USARR_TSNET_ENABLED=false` in dev so the normal loop binds an ordinary TCP port.

---

## 8. CI and the agent container: no Docker daemon

**The Docker daemon is unavailable in the current CI/agent container.** Verified: `docker info`
fails in this environment.

Therefore:

* `make test`, `make lint`, `make check`, and `make build` **must not require Docker**, a live
  service, or network access. If any of them ever does, that is a bug in the target, not an
  environment problem to work around.
* Testcontainers, dockertest, and "spin up a real Postgres/Sonarr in a test" patterns are **out of
  bounds** for the default test suite. This is also why UsArr uses SQLite rather than anything
  needing a server.
* Anything requiring the stack goes behind `//go:build integration`, is gated on
  `USARR_INTEGRATION=1`, and runs only under `make test-integration` on a developer machine.
* `make docker` exists for humans and for release pipelines that do have a daemon. It is not part of
  `make check` and CI must not call it.

A `make check` that passes on a machine with no daemon, no *Arr stack, and no network is the bar. If
you cannot get a test to that bar, it belongs in the integration tag.

---

## 9. Lint and format

### Go

```bash
gofumpt -l -w .                # stricter superset of gofmt; the formatting authority
golangci-lint run              # v2 — .golangci.yml MUST declare version: "2"
golangci-lint fmt              # v2 folds goimports/gofumpt under `formatters`
```

`.golangci.yml` starting point **(not yet)** — v2 format, `linters.default` replaces v1's
`enable-all`/`disable-all`:

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
formatters:
  enable:
    - gofumpt
    - goimports
```

`noctx` and `bodyclose` are not stylistic here. UsArr fans out to a dozen services on every page
load; a request without a timeout or a body that is never closed becomes a hung dashboard.

### Frontend

```bash
pnpm -C web lint          # eslint
pnpm -C web format        # prettier --write
pnpm -C web check         # svelte-check --tsconfig ./tsconfig.json  (types + a11y + unused CSS)
```

`svelte-check` is part of `make lint`, not an afterthought — with `adapter-static` a type error in a
route only shows up at build time otherwise.

### Pre-commit

```bash
make check    # fmt-check + lint + test
```

Run it before every commit. Wire it as a git hook if you like; do not wire it as a hook that
auto-formats, because a hook that rewrites your files mid-commit produces commits you did not read.

---

## 10. Conventions

* **Errors**: wrap with `%w` and context (`fmt.Errorf("sonarr %s: fetch series: %w", inst.Name, err)`).
  Never log a secret in an error path — the *Arr key is often in the URL you are tempted to include.
* **Contexts**: every outbound call takes a `context.Context` with a deadline. A slow Sonarr must not
  hang a page.
* **Outbound HTTP goes through `internal/ssrf`**, never `http.DefaultClient`. That package resolves
  the hostname, validates the resulting IP against the policy for that request class, connects to
  the pinned IP, refuses redirects, denies non-HTTP(S) schemes, and caps response size. Integration
  fetches may reach RFC1918 (that is the whole point of a homelab hub); metadata/image fetches may
  not, including `169.254.169.254` **and the Tailscale CGNAT range `100.64.0.0/10`** — UsArr's
  default deployment is a tailnet, so "private space" is larger than RFC1918. Bypassing this package
  is a review-blocking change.
* **UsArr never invokes a media processing tool.** No `exec.Command("ffmpeg", …)`, ever. Playback is
  delegated; see §1.
* **Never join on an *Arr's local `id`.** Those integers are per-instance and unrelated across
  instances — the same is true of *Arr tag IDs, where instance A's tag `3` has nothing to do with
  instance B's tag `3`. Join on external IDs (tvdb/tmdb/imdb/MBID/OLID/ISBN) or on
  `(instance_id, remote_id)`.
* **Store raw release names verbatim, forever.** Every parsed field is re-derivable if the parser
  improves; the raw name is not recoverable once discarded.
* **Do not write a release-name parser from scratch.** Port or vendor an existing one and use
  Sonarr's regex corpus (158 regexes in `Parser.cs`) and its test fixtures as the oracle.
* **Commits**: imperative subject, ≤72 chars, body explains *why*. Conventional Commits prefixes are
  fine but not enforced.

---

## 11. Onboarding an AI agent

An agent working in this repo should assume:

* `make check` is the definition of done for a code change, and it must pass **offline**.
* No Docker. No network in tests. If a task seems to need either, the task is misframed — reach for
  a cassette or the vendored spec instead.
* `docs/CONFIGURATION.md` is the contract for anything config-shaped. New settings go in the table
  there, in `.env.example`, and in `internal/config` in the same commit — a setting that exists in
  only two of the three is a bug.
* Facts about upstream APIs come from `api/specs/` or from the research notes, not from memory. This
  ecosystem has many near-miss details (`/api/v1` vs `/api/v3`, `imdbId` as string vs int, 401 vs
  403, PascalCase webhook `eventType` against camelCase REST) that are easy to get plausibly wrong.
* When something is unverified, say so in the code comment or the doc. This repo marks uncertainty
  rather than papering over it, and reviewers rely on that.
