# CLAUDE.md — UsArr

Instructions for coding agents working in this repo. Read this, then `docs/ARCHITECTURE.md`,
before proposing or writing anything.

## What UsArr is

A fast, self-hosted, unified hub and gateway over the media-acquisition ecosystem, running on
one self-hoster's own server. It aggregates the *Arrs (Sonarr, Radarr, Lidarr, Prowlarr,
LazyLibrarian) and media backends (Navidrome, Jellyfin, Audiobookshelf, Komga, Kavita) into a
single local library you can browse, search and request from, and it exposes protocol surfaces
(OpenSubsonic, OPDS) so existing client apps connect to UsArr instead of to each backend
individually.

**UsArr coexists with the ecosystem; it does not replace it.** The *Arrs keep doing their job,
the media servers keep owning the bytes. UsArr is the hub and gateway in front of them, and a
proposal that starts by reimplementing something an existing service already does well has
misread the project.

Owner: Joe (GitHub `jdb3750`). Repo: `github.com/jdb3750/UsArr`.

**Status: pre-alpha.** The binary builds, serves an embedded SPA, and runs the Prowlarr
Search-and-Grab path end to end on top of the §14 security floor. Whether there is a catalogue
behind any of it, and how much of one, is a tree question rather than a paragraph question:
`internal/libsync`'s package doc names channel by channel what the sync core has and has not got,
and `cmd/usarr/import.go`'s names what triggers an import. **Past that, this file deliberately does
not list what has landed** — such a list is false within days, and this one has been twice, most
recently on a sentence about the sync — so read it off the tree:
`web/src/routes` for screens, `internal/` for backend surfaces, `internal/db/migrations` for the
schema that exists. `docs/ARCHITECTURE.md` §16 is authoritative for which milestone owns a thing,
which is a different question from whether it is built. Read the code before describing behaviour
— "no invented status" cuts both ways.

## The four principles

**1. Replica, not proxy.** Every user-facing read renders from local SQLite. No screen ever
blocks on an *Arr or a metadata provider. The *Arrs are replication sources and command sinks,
not a live backend. Perceived speed is the owner's number-one requirement: a change that puts
a synchronous upstream call on a render path is wrong by default, and needs an explicit
argument to survive review. The single deliberate exception is byte streams on UsArr's own
protocol surfaces, and those are documented as exceptions where they occur. See
`docs/ARCHITECTURE.md` §2 and ADR-0004.

**2. Not a player.** UsArr never transcodes, never depends on FFmpeg, and does not implement
video playback. It routes and links out to the media server that owns the bytes. Do not
propose an in-app player, a transcoding path, or an FFmpeg dependency — the question is
closed. See ADR-0005, ADR-0006, ADR-0018.

**3. Pluggable by default.** UsArr must work over just Prowlarr, over a full stack, or over
anything in between. Every feature degrades honestly when a service is absent — it says what
is missing and why, rather than rendering an empty screen that looks broken.

**4. Single-user in v0.1, multi-user in the schema from migration 0001.** Every user-scoped
row carries `user_id` from the very first migration; the UI simply hides what has not shipped
yet. Authorization is enforced server-side from the first commit and is never bolted on later.
See ADR-0019, ADR-0011.

## UI direction

Utilitarian over stylish. Tried and true beats novel: standard, familiar patterns, easy to use,
and snappy above all. No visual flair that costs render time — if an effect buys nothing but
looks, it does not ship. **Navidrome is the reference point for the bar to hit.**

Five screens are essential, and all five ship in v0.1:
- **Home**, three fixed blocks — a media-type summary, an attention block hidden when empty, and one
  unified recently-added table across all types. Not one strip per media type. See
  `docs/ARCHITECTURE.md` §17.2, as amended by ADR-0028.
- **Service setup and health**, which must show what in the pipeline is broken and how to fix it.
- **Libraries**, where a user-defined library is bound to the upstream containers a service already
  named, and given a request destination. See §17.8.
- **Search** across your media.
- **Requests**, covering both the *Arr-backed path and the Prowlarr free-text indexer
  search-and-grab path. The Prowlarr path is in v0.1 (Search-and-Grab mode); the *Arr-backed
  request flow is v0.2.

## Working practice

**Adversarial review is mandatory.** The owner asked for this explicitly. Substantive design,
research or synthesis gets an adversarial reviewer pass against strict standards before it is
treated as final — a reviewer that attacks assumptions, hunts for gaps and omissions, and
verifies factual claims against primary sources. Every finding is either applied or rebutted
in writing in `docs/REVIEW-LOG.md`. Findings are never silently dropped.

**Verify, don't assert.** Every claim about an external API, rate limit, licensing term, port,
endpoint or field name cites a primary source: official docs, the OpenAPI spec, or the
service's own source code. Training data about this ecosystem is stale and actively wrong in
the ways listed below. Where you are reasoning rather than citing, mark it as inference. The same
holds inside the repo: a docstring, a code comment or a doc sentence describing behaviour is a
claim, not a specification — check it against the code before relying on it, and when one is
wrong, fix or delete it rather than annotating it.

**The same standard applies to the repo's own gates.** Report what you measured, not just the
verdict — the binary, its version and the commit — because a green that names neither its tool nor
its tree is a rumour, and fire a guard deliberately before trusting it, since one that probes a
proxy for its condition or has never been triggered is indistinguishable from no guard.
`docs/DEVELOPMENT.md` §11 carries the mechanics. One class of defect no gate catches at all:
**a wire vocabulary and a storage vocabulary never share a term** — §11 carries that rule as
well, and the three instances that earned it, every one of which compiled clean.

**No invented status.** Never document a feature as existing when it does not.
`docs/ARCHITECTURE.md` §16 is authoritative for what lands in which milestone; the README's
status tables are maintained by hand against it — nothing in the repo generates them, so they lag
§16 and have. If §16 does not say a thing ships, it does not ship.

**Status is read off the tree, not off a document.** No document owns what is built —
`web/src/routes`, `internal/` and `internal/db/migrations` do. When you find a status claim that
has gone stale, do not write a fresher one; write the pointer.

**Cut before you add.** This project's biggest risk is never shipping. A proposal that adds a
subsystem must say what it removes, or defer itself to a later milestone. "And also" is not a
plan.

**Build the base with intentional space for what comes later.** Several deferred features have
an obvious seam in the current design: the provider registry is an interface a plugin host
could implement, the search retriever is pluggable behind one boundary, `work_relation` is designed
to carry confidence and evidence columns — its DDL is `docs/reference/schema.md` §11, the table
itself is deferred to v0.3, and `TestDeferredTablesAreAbsent` fails if a migration creates it
early. Keep those seams — they cost almost nothing now and are expensive to retrofit. This is
not a licence to build the deferred feature early: the seam ships, the feature does not.

## Ecosystem facts that stale training data gets wrong

These are correct as of the dates given. Re-verify against a primary source before relying on
any of them — this ecosystem moves.

- **Readarr was archived 2025-06-27.** Books are fragmented across LazyLibrarian and community
  forks. The Goodreads API is dead; use Open Library.
- **Overseerr and Jellyseerr merged into Seerr in Feb 2026**; `sct/overseerr` was archived
  2026-02-15.
- **TheTVDB v4 has no free tier.** Default to TMDB + TVmaze, and AniList for anime.
- **MusicBrainz is 1 request/second** and requires a descriptive User-Agent. **TMDB** requires
  visible attribution and caps cache duration at 6 months.
- **Servarr API versions do not track app versions.** Sonarr 4 / Radarr 5 / Whisparr 2 use
  `/api/v3`; Lidarr / Readarr / Prowlarr use `/api/v1`.
- **Navidrome does not support OpenSubsonic `apiKeyAuthentication`** — not in any release, not
  in `master` (PRs #4022 and #5731 are open). Southbound, UsArr stores the `subsonicSalt` +
  `subsonicToken` pair returned by `POST /auth/login` and never stores a recoverable password.
  Northbound, UsArr's own surface offers api-key auth only and refuses to serve salt/token.
- **`ncruces/go-sqlite3` moved off wazero to `wasm2go` (2026-03-05).** Do not repeat the
  "shared runtime with the plugin host" argument; it no longer holds.

## Security rules — not negotiable

- ***Arr API keys are full-admin credentials.** Encrypt at rest with a versioned, AAD-bound
  scheme. Never log them, never send them to the browser, and never send one to a host the
  user has just edited without re-entry.
- **SSRF is a first-class risk**, because users configure arbitrary internal URLs. Three
  distinct classes: admin-configured service URLs; metadata-provider URLs; and URLs harvested
  from upstream response bodies (cover art, manifest outputs). Resolve-then-pin against DNS
  rebinding — validating a hostname and then letting the HTTP client re-resolve it is a bug.
- **Argon2id is for user passwords only.** Per-app API keys verify with a fast keyed hash.
  Running Argon2id on every request is a remote memory-exhaustion vector.
- **The master key is auto-generated on first run**, and the process fails closed on a
  placeholder value. Never ship a default key.

See `docs/ARCHITECTURE.md` §14 for the full threat model.

## Conventions

**Stack**
- Go 1.25.13+, `CGO_ENABLED=0`, single static binary. The floor is set by govulncheck, not by a
  dependency — goose v3.27.3 only needs 1.25.7, but 1.25.7 has 15 called stdlib vulnerabilities and
  1.25.12 still has 5. Re-check rather than assuming; a new advisory moves it. `go.mod` is
  authoritative.
- SvelteKit with `adapter-static` (SPA), embedded via `embed.FS`.
- SQLite + WAL: a read pool plus a single-writer connection; `BEGIN IMMEDIATE` for writes.
- goose migrations. **A merged migration is never edited** — write a new one.

**Quality gate**
- `gofumpt`, `golangci-lint`, `eslint`, `prettier`, `svelte-check`.
- **`make check` (`provenance` + `fmt-check` + `lint` + `build-tagged` + `modverify` + `secrets` +
  `test`, then `vuln`) is the pre-commit gate and must pass before any commit.** It needs no Docker
  daemon and makes exactly two network calls, both to vulnerability databases — govulncheck's to
  `vuln.go.dev` and `pnpm audit`'s to the npm registry. `make check-offline` drops both.

**Tests**
- Recorded HTTP fixtures (`go-vcr`) plus contract tests against the shipped Servarr OpenAPI
  specs. The agent container the build runs in has no Docker daemon and no ffmpeg, and there is no
  public *Arr demo instance — tests that need a live service do not belong in the default suite.
- **There is no CI**, so `make check` is the whole gate, and nothing mechanises it either: `CLAUDE.md`
  requires it before a commit, a person or an agent has to type it, and `make design` is required by
  no document at all. `docs/DEVELOPMENT.md` §8 states the constraint a CI would inherit.
- Query-plan assertions (`EXPLAIN QUERY PLAN`) belong in the gate, and belong in a CI if one is ever
  added. Wall-clock benchmarks belong in `make bench`, never in a merge gate.

**Style**
- Conventional commit prefixes (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`).
- Comments explain *why*, not *what*.

## Where things live

| Document | What it holds |
| --- | --- |
| `docs/ARCHITECTURE.md` | The design. §16 is the authoritative roadmap. |
| `docs/ROADMAP.md` | A working checklist of what v0.1 still needs, each item with a check you can run. Authoritative for neither scope (§16 is) nor status (the tree is). |
| `docs/DECISIONS.md` | ADRs. **Add one for any decision that closes off an alternative.** |
| `docs/FUTURE.md` | Mostly deferred features, and the seam each one is designed against — unless the heading says *Declined*. |
| `docs/RESEARCH.md` | Ecosystem findings with primary-source citations. |
| `docs/CONFIGURATION.md` | Every configuration key and its semantics. |
| `docs/DEVELOPMENT.md` | Local setup, workflow, the make targets. |
| `docs/REVIEW-LOG.md` | Adversarial-review findings, each applied or rebutted. |
| `docs/design/` | The visual system: DESIGN-DIRECTION.md, tokens.css (canonical values) and the v0.1 mockups. §17 stays authoritative over all three. |
| `docs/reference/` | The long-form companions to `ARCHITECTURE.md`, one per area: the DDL of record, UsArr's own HTTP contract, the security model, captured *Arr API facts. Prose only — no specs live here. |
| `api/specs/` | The vendored upstream specs the contract tests read, and `SOURCES.md`, which records each file's origin, commit and hash. |

Do not duplicate design detail into this file. Link to the document that owns it.

**Licence: AGPL-3.0, confirmed by the owner on 2026-08-16.** The verbatim FSF text is in `LICENSE`
at the repo root; the reasoning is in ADR-0024. **Do not add per-file licence headers** — the root
`LICENSE` and the README's `## License` section are the whole declaration. New dependencies need an
AGPL-compatibility check: MIT, BSD and Apache-2.0 are fine, GPL-2.0-only is not.

## Roadmap in one line each

Detail lives in `docs/ARCHITECTURE.md` §16, which wins over this summary.

- **v0.1** — the sync core behind **one** catalogue adapter + search, on a six-type schema, plus the
  Prowlarr search-and-grab request path. Prove the replica thesis on real data. **Which adapter, and
  what else v0.1 owes, is §16's answer and it has moved twice — read §16.1, not this line.**
  The rule that has held through every move: **one source, proven on real data, before a second
  adapter**, and everything displaced is **re-sequenced, not cut**. ⚠️ This bullet used to read
  *"the \*Arr library sync (Sonarr, Radarr)"* and **"No catalogue source ships in v0.1"**; ADR-0041
  falsified both, and ADR-0042 re-sequenced the minimal \*Arr write path out with them.
- **v0.2** — requests.
- **v0.3** — cross-media linking.
- **v0.4** — the gateway, narrowed: an OpenSubsonic read-only subset over one Navidrome.
- **v1.0** — breadth: more providers, multi-user, the OPDS surface, the full tag system.

## Non-goals, and deferred work

**Permanent non-goals.** Decided against for good. Do not propose them, do not reopen them:
video transcoding · an in-app media player · any FFmpeg dependency.

**Deferred, not rejected.** Out of scope for the current milestone, wanted later, and the
design should leave room for them. Do not build them now; do not treat them as refused either.
`docs/FUTURE.md` owns each one, with the seam in the current design that keeps it cheap to add
(a *Declined* heading there means closed, not deferred):

- WASM / Extism plugins
- an external search engine such as Meilisearch
- OIDC, passkeys, TOTP
- the cross-media fuzzy-match tier and its review inbox
- spellfix1 typo tolerance
- a Jellyfin-compatible northbound surface
- video byte-proxying (audio and ebooks are proxied today; video links out)
- aggregated release calendars across subscriptions
- per-user watch / listen / read statistics
