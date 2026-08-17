# UsArr — Project Instructions (canonical copy)

This file is the canonical, versioned copy of the text applied to the Project's settings — the
instructions every agent session receives before it reads anything in the repo.

Agents cannot edit project settings. The flow is therefore: the text is updated here, the full
text is posted in the gatekeeper thread, and the project coordinator or Joe applies it to the
Project's settings by hand. The settings text has an 8 KiB limit, so length is a hard
constraint, not a preference.

This file records the instruction text only. The design detail lives in `CLAUDE.md` and
`docs/`. Where this file and those disagree, `docs/ARCHITECTURE.md` section 16 wins.

## Status

| Version | Date | State | Size |
| --- | --- | --- | --- |
| v1.5 | 2026-08-17 | **Applied to project settings** — 2026-08-17 05:05 UTC | 8124 bytes |
| v1.4 | 2026-08-17 | Applied 2026-08-17 04:52 UTC, superseded by v1.5 the same day | 8118 bytes |
| v1.3 | 2026-08-16 | Applied 2026-08-16 16:34 UTC, superseded by v1.4 | 7844 bytes |
| v1.2 | 2026-08-16 | Applied 2026-08-16 07:57 UTC, superseded by v1.3 the same day | 7585 bytes |
| v1.1 | 2026-08-16 | Superseded by v1.2, never applied | 7022 bytes |
| v1.0 | 2026-08-16 | Superseded by v1.2 — applied 2026-08-16, replaced the same day | 3849 bytes |

## v1.5 — as applied

The text applied verbatim to the Project's settings at 05:05 UTC on 2026-08-17, replacing v1.4,
and live in the settings now. It was verified byte-identical to this block by checksum on both
sides of the apply: 8124 bytes, md5 `77f2d90247d00395dea77e94076bc84e`, measured on the extract
before applying and on the settings read-back afterwards.

````
You are working on UsArr: a fast, self-hosted, unified hub and gateway over the media-acquisition ecosystem, running on a single self-hoster's own server. It aggregates the *Arrs (Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian) and media backends (Navidrome, Jellyfin, Audiobookshelf, Komga, Kavita) into one local library you can browse, search and request from, and it exposes protocol surfaces (OpenSubsonic, OPDS) so existing client apps connect to UsArr instead of to each backend individually. It is meant to coexist with the rest of the ecosystem, not replace it. The stack is Go compiled to a single static binary with a SvelteKit SPA embedded in it, over SQLite in WAL mode. Do not state a Go minimum from memory: the go directive in go.mod is authoritative, 1.25.13 at the time of writing, and it is a moving floor raised by the gating govulncheck step rather than by the dependency floor beneath it, with the reasoning in docs/DEVELOPMENT.md. Implementation on main moves faster than the prose describing it. Section 16 stays authoritative for scope; status is a separate question, and no document owns it — the tree does. Treat any claim in the docs that something is or is not built as unverified: read the tree — web/src/routes for a screen, internal/ for a backend surface, internal/db/migrations for the schema — and name the commit you read. Do not write a fresher one; write the pointer. A milestone label is scope, not status.

Before you propose or write anything, read CLAUDE.md at the repo root and then docs/ARCHITECTURE.md. Those two files, plus the ADRs in docs/DECISIONS.md, are the source of truth. Section 16 of ARCHITECTURE.md is authoritative for what belongs in which milestone, and it wins over every other document, this one included.

Four principles govern every decision.

First, replica not proxy: every user-facing read renders from local SQLite, and no screen ever blocks on an *Arr or a metadata provider. Perceived speed is the owner's number-one requirement, so anything that puts a synchronous upstream call on a render path is wrong by default and needs an explicit argument to survive. Three narrow exceptions are documented where they occur, and none of them blocks a render: byte streams on UsArr's own protocol surfaces, where audio, ebooks and comics are proxied with a plain io.Copy, video links out, and images are always proxied and cached; search over unowned items, which runs out of band and streams into an already-rendered page over SSE; and release search across indexers, which is remote and sits behind progressive disclosure.

Second, UsArr is not a player: it never transcodes, never depends on FFmpeg, and does not implement video playback. It routes and links out to whichever media server owns the bytes.

Third, pluggable by default: UsArr must work over a full stack, over any single library-bearing service, or over Prowlarr alone in Search-and-Grab mode, and every feature degrades honestly when a service is absent rather than rendering an empty screen. Presenting a library requires at least one library-bearing service; Prowlarr alone has no library. Requests are a pillar rather than a side feature: the Prowlarr free-text path ships in v0.1 and the *Arr-backed flow in v0.2.

Fourth, single-user in v0.1 but multi-user in the schema from migration 0001. Two rules hold from the first migration: every user-scoped row carries a user_id, and every read path that aggregates across instances takes an access-scope parameter in its query signature, covering the grid, search, the client prefix index, the availability rollup and every northbound surface, defaulting in v0.1 to the owner's full scope. A rollup computed across instances a user cannot see is an existence oracle. The UI merely hides what has not shipped; authorization is enforced server-side from the first commit and is never bolted on later.

Adversarial review is mandatory, and the owner asked for it explicitly. Substantive design, research or synthesis gets a reviewer pass that attacks assumptions, hunts for gaps and omissions, and verifies factual claims against primary sources. Every finding is then applied or rebutted in writing in docs/REVIEW-LOG.md. Findings are never quietly dropped. Several threads work this repo at once: section 11 of docs/DEVELOPMENT.md has the merge cadence, the file-ownership map and the guard rules.

Verify, do not assert. Every claim about an external API, rate limit, licensing term, port, endpoint or field name must cite a primary source: official documentation, an OpenAPI spec, or the service's own source code. Training data about this ecosystem is stale and wrong in specific, load-bearing ways, so treat recollection as a hypothesis to check rather than a fact. Where you are reasoning rather than citing, say so and label it as inference. Never document a feature as existing when it does not. The same standard applies to this project's own gates: report what you measured — the binary, its version and the commit — because a green that names neither its tool nor its tree is a rumour, and fire a guard deliberately before trusting it, since one that has never been triggered is indistinguishable from no guard. The "Ecosystem facts that stale training data gets wrong" section of CLAUDE.md is the list rather than a sample; re-verify any entry against a primary source before relying on it.

Security is not negotiable. *Arr API keys are full-admin credentials: encrypted at rest under a versioned, AAD-bound scheme, never logged, never sent to the browser, and never sent to a host the user has just edited without re-entry. SSRF is a first-class risk because users configure arbitrary internal URLs, so resolve then pin. Argon2id is for user passwords only; per-app API keys verify with a fast keyed hash, because running Argon2id on every request is a remote memory-exhaustion vector. Section 14 of ARCHITECTURE.md owns the full threat model.

Cut before you add, but leave the seams open. The project's largest risk is never shipping, so a proposal that adds a subsystem must say what it removes or defer itself to a later milestone. Deferred is not rejected: docs/FUTURE.md holds the features that are wanted later, each with the specific seam in the current design that keeps it cheap to add. Preserve those seams; do not build the future feature early.

Some things are permanently refused rather than deferred. Section 1.4 of ARCHITECTURE.md lists six: a video transcoder, an in-app media player, any FFmpeg dependency, reimplementing the *Arr download and import engines, a required sidecar (optional backends may exist, but Postgres, Redis or a search server may never be required), and being a dashboard. Section 16 adds native TV or mobile apps. Do not propose these and do not reopen them. Section 16 does name two measured conditions that would reopen playback, a hostile or unusable Jellyfin API and at least two engineers who can own an FFmpeg surface indefinitely including security response; neither is met, so treat it as closed. Anything out of scope that is not on those two lists is deferred rather than closed, so check docs/FUTURE.md before assuming either way.

On interface design, read section 17 of ARCHITECTURE.md before touching a screen. It is authoritative over the screens, and docs/design/ specifies the visual system that renders them — DESIGN-DIRECTION.md, tokens.css and the mockups. Read both, and where they disagree, section 17 wins. The constraint is utilitarian over stylish: standard patterns in preference to novel ones, density and speed over animation, and no visual flair that costs render time. Navidrome is the reference point, and "sleek" and "modern" are explicitly not goals. Concretely: no animation on any list, grid or navigation transition; native controls and real middle-clickable links; compact by default; every screen usable in a phone browser; and no skeleton shimmer. Section 17 enumerates the screens and section 16 says which ship in v0.1; read both rather than assuming a count. A degraded backend gets a non-modal banner; the catalogue never greys out.
````

## v1.4 — superseded

The text applied verbatim to the Project's settings at 04:52 UTC on 2026-08-17, replacing v1.3 —
8118 bytes. It was live in the settings until v1.5 above replaced it at 05:05 UTC the same day, and
is no longer live. The applied text was verified byte-identical to this block by checksum on both
sides of the apply: 8118 bytes, md5 `2d52321ed5c04b8481df6eb4c0ccb8e8`, measured on the extract
before applying and on the settings read-back afterwards.

````
You are working on UsArr: a fast, self-hosted, unified hub and gateway over the media-acquisition ecosystem, running on a single self-hoster's own server. It aggregates the *Arrs (Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian) and media backends (Navidrome, Jellyfin, Audiobookshelf, Komga, Kavita) into one local library you can browse, search and request from, and it exposes protocol surfaces (OpenSubsonic, OPDS) so existing client apps connect to UsArr instead of to each backend individually. It is meant to coexist with the rest of the ecosystem, not replace it. The stack is Go compiled to a single static binary with a SvelteKit SPA embedded in it, over SQLite in WAL mode. Do not state a Go minimum from memory: the go directive in go.mod is authoritative, 1.25.13 at the time of writing, and it is a moving floor raised by the gating govulncheck step rather than by the dependency floor beneath it, with the reasoning in docs/DEVELOPMENT.md. Implementation on main moves faster than the prose describing it. Section 16 stays authoritative for scope; status is a separate question, and the landed/not-yet split in section 16, the README's status column and CLAUDE.md's status paragraph are hand-edited after the fact and lag the tree. Treat a "not yet" in any of the three as unverified: read the tree — web/src/routes for a screen, internal/ for a subsystem — and name the commit you read. A milestone label is scope, not status.

Before you propose or write anything, read CLAUDE.md at the repo root and then docs/ARCHITECTURE.md. Those two files, plus the ADRs in docs/DECISIONS.md, are the source of truth. Section 16 of ARCHITECTURE.md is authoritative for what belongs in which milestone, and it wins over every other document, this one included.

Four principles govern every decision.

First, replica not proxy: every user-facing read renders from local SQLite, and no screen ever blocks on an *Arr or a metadata provider. Perceived speed is the owner's number-one requirement, so anything that puts a synchronous upstream call on a render path is wrong by default and needs an explicit argument to survive. Three narrow exceptions are documented where they occur, and none of them blocks a render: byte streams on UsArr's own protocol surfaces, where audio, ebooks and comics are proxied with a plain io.Copy, video links out, and images are always proxied and cached; search over unowned items, which runs out of band and streams into an already-rendered page over SSE; and release search across indexers, which is remote and sits behind progressive disclosure.

Second, UsArr is not a player: it never transcodes, never depends on FFmpeg, and does not implement video playback. It routes and links out to whichever media server owns the bytes.

Third, pluggable by default: UsArr must work over a full stack, over any single library-bearing service, or over Prowlarr alone in Search-and-Grab mode, and every feature degrades honestly when a service is absent rather than rendering an empty screen. Presenting a library requires at least one library-bearing service; Prowlarr alone has no library. Requests are a pillar rather than a side feature: the Prowlarr free-text path ships in v0.1 and the *Arr-backed flow in v0.2.

Fourth, single-user in v0.1 but multi-user in the schema from migration 0001. Two rules hold from the first migration: every user-scoped row carries a user_id, and every read path that aggregates across instances takes an access-scope parameter in its query signature, covering the grid, search, the client prefix index, the availability rollup and every northbound surface, defaulting in v0.1 to the owner's full scope. A rollup computed across instances a user cannot see is an existence oracle. The UI merely hides what has not shipped; authorization is enforced server-side from the first commit and is never bolted on later.

Adversarial review is mandatory, and the owner asked for it explicitly. Substantive design, research or synthesis gets a reviewer pass that attacks assumptions, hunts for gaps and omissions, and verifies factual claims against primary sources. Every finding is then applied or rebutted in writing in docs/REVIEW-LOG.md. Findings are never quietly dropped. Several threads work this repo at once: section 11 of docs/DEVELOPMENT.md has the merge cadence, the file-ownership map and the guard rules.

Verify, do not assert. Every claim about an external API, rate limit, licensing term, port, endpoint or field name must cite a primary source: official documentation, an OpenAPI spec, or the service's own source code. Training data about this ecosystem is stale and wrong in specific, load-bearing ways, so treat recollection as a hypothesis to check rather than a fact. Where you are reasoning rather than citing, say so and label it as inference. Never document a feature as existing when it does not. The same standard applies to this project's own gates: report what you measured — the binary, its version and the commit — because a green that names neither its tool nor its tree is a rumour, and fire a guard deliberately before trusting it, since one that has never been triggered is indistinguishable from no guard. The "Ecosystem facts that stale training data gets wrong" section of CLAUDE.md is the list rather than a sample; re-verify any entry against a primary source before relying on it.

Security is not negotiable. *Arr API keys are full-admin credentials: encrypted at rest under a versioned, AAD-bound scheme, never logged, never sent to the browser, and never sent to a host the user has just edited without re-entry. SSRF is a first-class risk because users configure arbitrary internal URLs, so resolve then pin. Argon2id is for user passwords only; per-app API keys verify with a fast keyed hash, because running Argon2id on every request is a remote memory-exhaustion vector. Section 14 of ARCHITECTURE.md owns the full threat model.

Cut before you add, but leave the seams open. The project's largest risk is never shipping, so a proposal that adds a subsystem must say what it removes or defer itself to a later milestone. Deferred is not rejected: docs/FUTURE.md holds the features that are wanted later, each with the specific seam in the current design that keeps it cheap to add. Preserve those seams; do not build the future feature early.

Some things are permanently refused rather than deferred. Section 1.4 of ARCHITECTURE.md lists six: a video transcoder, an in-app media player, any FFmpeg dependency, reimplementing the *Arr download and import engines, a required sidecar (optional backends may exist, but Postgres, Redis or a search server may never be required), and being a dashboard. Section 16 adds native TV or mobile apps. Do not propose these and do not reopen them. Section 16 does name two measured conditions that would reopen playback, a hostile or unusable Jellyfin API and at least two engineers who can own an FFmpeg surface indefinitely including security response; neither is met, so treat it as closed. Anything out of scope that is not on those two lists is deferred rather than closed, so check docs/FUTURE.md before assuming either way.

On interface design, read section 17 of ARCHITECTURE.md before touching a screen. It is authoritative over the screens, and docs/design/ specifies the visual system that renders them — DESIGN-DIRECTION.md, tokens.css and the mockups. Read both, and where they disagree, section 17 wins. The constraint is utilitarian over stylish: standard patterns in preference to novel ones, density and speed over animation, and no visual flair that costs render time. Navidrome is the reference point, and "sleek" and "modern" are explicitly not goals. Concretely: no animation on any list, grid or navigation transition; native controls and real middle-clickable links; compact by default; every screen usable in a phone browser; and no skeleton shimmer. Section 17 enumerates the screens and section 16 says which ship in v0.1; read both rather than assuming a count. A degraded backend gets a non-modal banner; the catalogue never greys out.
````

## v1.3 — superseded

The text applied verbatim to the Project's settings at 16:34 UTC on 2026-08-16, replacing v1.2.
It was live in the settings until v1.4 above replaced it at 04:52 UTC on 2026-08-17, and is no
longer live. Preserved verbatim as the record of what v1.3 said — the settings read-back was
confirmed to match it, 7844 bytes.

````
You are working on UsArr: a fast, self-hosted, unified hub and gateway over the media-acquisition ecosystem, running on a single self-hoster's own server. It aggregates the *Arrs (Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian) and media backends (Navidrome, Jellyfin, Audiobookshelf, Komga, Kavita) into one local library you can browse, search and request from, and it exposes protocol surfaces (OpenSubsonic, OPDS) so existing client apps connect to UsArr instead of to each backend individually. It is meant to coexist with the rest of the ecosystem, not replace it. The stack is Go compiled to a single static binary with a SvelteKit SPA embedded in it, over SQLite in WAL mode. Do not state a Go minimum from memory: the go directive in go.mod is authoritative, 1.25.13 at the time of writing, and it is a moving floor raised by the gating govulncheck step rather than by the dependency floor beneath it, with the reasoning in docs/DEVELOPMENT.md. The first slice of code is on main — the Prowlarr Search-and-Grab path runs end to end — and everything else is still design. CLAUDE.md, the README and section 16 each carry an accurate landed / not-yet split, so read them rather than assuming in either direction: a milestone label is scope, not status.

Before you propose or write anything, read CLAUDE.md at the repo root and then docs/ARCHITECTURE.md. Those two files, plus the ADRs in docs/DECISIONS.md, are the source of truth. Section 16 of ARCHITECTURE.md is authoritative for what belongs in which milestone, and it wins over every other document, this one included.

Four principles govern every decision.

First, replica not proxy: every user-facing read renders from local SQLite, and no screen ever blocks on an *Arr or a metadata provider. Perceived speed is the owner's number-one requirement, so anything that puts a synchronous upstream call on a render path is wrong by default and needs an explicit argument to survive. Three narrow exceptions are documented where they occur, and none of them blocks a render: byte streams on UsArr's own protocol surfaces, where audio, ebooks and comics are proxied with a plain io.Copy, video links out, and images are always proxied and cached; search over unowned items, which runs out of band and streams into an already-rendered page over SSE; and release search across indexers, which is remote and sits behind progressive disclosure.

Second, UsArr is not a player: it never transcodes, never depends on FFmpeg, and does not implement video playback. It routes and links out to whichever media server owns the bytes.

Third, pluggable by default: UsArr must work over a full stack, over any single library-bearing service, or over Prowlarr alone in Search-and-Grab mode, and every feature degrades honestly when a service is absent rather than rendering an empty screen. Presenting a library requires at least one library-bearing service; Prowlarr alone has no library. Requests are a pillar rather than a side feature: the Prowlarr free-text path ships in v0.1 and the *Arr-backed flow in v0.2.

Fourth, single-user in v0.1 but multi-user in the schema from migration 0001. Two rules hold from the first migration: every user-scoped row carries a user_id, and every read path that aggregates across instances takes an access-scope parameter in its query signature, covering the grid, search, the client prefix index, the availability rollup and every northbound surface, defaulting in v0.1 to the owner's full scope. A rollup computed across instances a user cannot see is an existence oracle. The UI merely hides what has not shipped; authorization is enforced server-side from the first commit and is never bolted on later.

Adversarial review is mandatory, and the owner asked for it explicitly. Substantive design, research or synthesis gets a reviewer pass that attacks assumptions, hunts for gaps and omissions, and verifies factual claims against primary sources. Every finding is then applied or rebutted in writing in docs/REVIEW-LOG.md. Findings are never quietly dropped. Several threads work this repo at once: section 11 of docs/DEVELOPMENT.md has the merge cadence and the file-ownership map.

Verify, do not assert. Every claim about an external API, rate limit, licensing term, port, endpoint or field name must cite a primary source: official documentation, an OpenAPI spec, or the service's own source code. Training data about this ecosystem is stale and wrong in specific, load-bearing ways, so treat recollection as a hypothesis to check rather than a fact. Where you are reasoning rather than citing, say so and label it as inference. Never document a feature as existing when it does not. The "Ecosystem facts that stale training data gets wrong" section of CLAUDE.md is the list rather than a sample; re-verify any entry against a primary source before relying on it. The licence is settled: UsArr is licensed AGPL-3.0, confirmed by the owner on 2026-08-16, with the verbatim text in LICENSE and the reasoning in ADR-0024. There are no per-file licence headers, and new dependencies need an AGPL-compatibility check.

Security is not negotiable. *Arr API keys are full-admin credentials: encrypted at rest under a versioned, AAD-bound scheme, never logged, never sent to the browser, and never sent to a host the user has just edited without re-entry. SSRF is a first-class risk because users configure arbitrary internal URLs, so resolve then pin. Argon2id is for user passwords only; per-app API keys verify with a fast keyed hash, because running Argon2id on every request is a remote memory-exhaustion vector. Section 14 of ARCHITECTURE.md owns the full threat model.

Cut before you add, but leave the seams open. The project's largest risk is never shipping, so a proposal that adds a subsystem must say what it removes or defer itself to a later milestone. Deferred is not rejected: docs/FUTURE.md holds the features that are wanted later, each with the specific seam in the current design that keeps it cheap to add. Preserve those seams; do not build the future feature early.

Some things are permanently refused rather than deferred. Section 1.4 of ARCHITECTURE.md lists six: a video transcoder, an in-app media player, any FFmpeg dependency, reimplementing the *Arr download and import engines, a required sidecar (optional backends may exist, but Postgres, Redis or a search server may never be required), and being a dashboard. Section 16 adds native TV or mobile apps. Do not propose these and do not reopen them. Section 16 does name two measured conditions that would reopen playback, a hostile or unusable Jellyfin API and at least two engineers who can own an FFmpeg surface indefinitely including security response; neither is met, so treat it as closed. Anything out of scope that is not on those two lists is deferred rather than closed, so check docs/FUTURE.md before assuming either way.

On interface design, read section 17 of ARCHITECTURE.md before touching a screen. It is authoritative over the screens, and docs/design/ specifies the visual system that renders them — DESIGN-DIRECTION.md, tokens.css and the mockups. Read both, and where they disagree, section 17 wins. The constraint is utilitarian over stylish: standard patterns in preference to novel ones, density and speed over animation, and no visual flair that costs render time. Navidrome is the reference point, and "sleek" and "modern" are explicitly not goals. Concretely: no animation on any list, grid or navigation transition; native controls and real middle-clickable links; compact by default; every screen usable in a phone browser; and no skeleton shimmer. Section 17 enumerates the screens and section 16 says which ship in v0.1; read both rather than assuming a count. A degraded backend gets a non-modal banner; the catalogue never greys out.
````

## v1.2 — superseded

The text applied verbatim at 07:57 UTC on 2026-08-16, replacing v1.0 and superseding v1.1. It
was live in the Project's settings until v1.3 replaced it at 16:34 UTC the same day. Preserved
verbatim as the record of what was live until then — 7585 bytes.

````
You are working on UsArr: a fast, self-hosted, unified hub and gateway over the media-acquisition ecosystem, running on a single self-hoster's own server. It aggregates the *Arrs (Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian) and media backends (Navidrome, Jellyfin, Audiobookshelf, Komga, Kavita) into one local library you can browse, search and request from, and it exposes protocol surfaces (OpenSubsonic, OPDS) so existing client apps connect to UsArr instead of to each backend individually. It is meant to coexist with the rest of the ecosystem, not replace it. The stack is Go compiled to a single static binary with a SvelteKit SPA embedded in it, over SQLite in WAL mode. Do not state a Go minimum from memory: the go directive in go.mod is authoritative, 1.25.13 at the time of writing, and it is a moving floor raised by the gating govulncheck step rather than by the dependency floor beneath it, with the reasoning in docs/DEVELOPMENT.md. Implementation has begun on feature branches while CLAUDE.md, the README and section 16 of ARCHITECTURE.md all still say nothing is implemented; that wording is stale wherever code exists, so check the working tree before describing project status, and name the branch you checked.

Before you propose or write anything, read CLAUDE.md at the repo root and then docs/ARCHITECTURE.md. Those two files, plus the ADRs in docs/DECISIONS.md, are the source of truth. Section 16 of ARCHITECTURE.md is authoritative for what belongs in which milestone, and it wins over every other document, this one included.

Four principles govern every decision.

First, replica not proxy: every user-facing read renders from local SQLite, and no screen ever blocks on an *Arr or a metadata provider. Perceived speed is the owner's number-one requirement, so anything that puts a synchronous upstream call on a render path is wrong by default and needs an explicit argument to survive. Three narrow exceptions are documented where they occur, and none of them blocks a render: byte streams on UsArr's own protocol surfaces, where audio, ebooks and comics are proxied with a plain io.Copy, video links out, and images are always proxied and cached; search over unowned items, which runs out of band and streams into an already-rendered page over SSE; and release search across indexers, which is remote and sits behind progressive disclosure.

Second, UsArr is not a player: it never transcodes, never depends on FFmpeg, and does not implement video playback. It routes and links out to whichever media server owns the bytes.

Third, pluggable by default: UsArr must work over a full stack, over any single library-bearing service, or over Prowlarr alone in Search-and-Grab mode, and every feature degrades honestly when a service is absent rather than rendering an empty screen. Presenting a library requires at least one library-bearing service; Prowlarr alone has no library. Requests are a pillar rather than a side feature: the Prowlarr free-text path ships in v0.1 and the *Arr-backed flow in v0.2.

Fourth, single-user in v0.1 but multi-user in the schema from migration 0001. Two rules hold from the first migration: every user-scoped row carries a user_id, and every read path that aggregates across instances takes an access-scope parameter in its query signature, covering the grid, search, the client prefix index, the availability rollup and every northbound surface, defaulting in v0.1 to the owner's full scope. A rollup computed across instances a user cannot see is an existence oracle. The UI merely hides what has not shipped; authorization is enforced server-side from the first commit and is never bolted on later.

Adversarial review is mandatory, and the owner asked for it explicitly. Substantive design, research or synthesis gets a reviewer pass that attacks assumptions, hunts for gaps and omissions, and verifies factual claims against primary sources. Every finding is then applied or rebutted in writing in docs/REVIEW-LOG.md. Findings are never quietly dropped.

Verify, do not assert. Every claim about an external API, rate limit, licensing term, port, endpoint or field name must cite a primary source: official documentation, an OpenAPI spec, or the service's own source code. Training data about this ecosystem is stale and wrong in specific, load-bearing ways, so treat recollection as a hypothesis to check rather than a fact. Where you are reasoning rather than citing, say so and label it as inference. Never document a feature as existing when it does not. The "Ecosystem facts that stale training data gets wrong" section of CLAUDE.md is the list rather than a sample; re-verify any entry against a primary source before relying on it. The licence is settled: UsArr is licensed AGPL-3.0, confirmed by the owner on 2026-08-16, with the verbatim text in LICENSE and the reasoning in ADR-0024. There are no per-file licence headers, and new dependencies need an AGPL-compatibility check.

Security is not negotiable. *Arr API keys are full-admin credentials: encrypted at rest under a versioned, AAD-bound scheme, never logged, never sent to the browser, and never sent to a host the user has just edited without re-entry. SSRF is a first-class risk because users configure arbitrary internal URLs, so resolve then pin. Argon2id is for user passwords only; per-app API keys verify with a fast keyed hash, because running Argon2id on every request is a remote memory-exhaustion vector. Section 14 of ARCHITECTURE.md owns the full threat model.

Cut before you add, but leave the seams open. The project's largest risk is never shipping, so a proposal that adds a subsystem must say what it removes or defer itself to a later milestone. Deferred is not rejected: docs/FUTURE.md holds the features that are wanted later, each with the specific seam in the current design that keeps it cheap to add. Preserve those seams; do not build the future feature early.

Some things are permanently refused rather than deferred. Section 1.4 of ARCHITECTURE.md lists six: a video transcoder, an in-app media player, any FFmpeg dependency, reimplementing the *Arr download and import engines, a required sidecar (optional backends may exist, but Postgres, Redis or a search server may never be required), and being a dashboard. Section 16 adds native TV or mobile apps. Do not propose these and do not reopen them. Section 16 does name two measured conditions that would reopen playback, a hostile or unusable Jellyfin API and at least two engineers who can own an FFmpeg surface indefinitely including security response; neither is met, so treat it as closed. Anything out of scope that is not on those two lists is deferred rather than closed, so check docs/FUTURE.md before assuming either way.

On interface design, read section 17 of ARCHITECTURE.md before touching a screen. It is the only place the screens are specified, and neither CLAUDE.md nor the README points at it. The constraint is utilitarian over stylish: standard patterns in preference to novel ones, density and speed over animation, and no visual flair that costs render time. Navidrome is the reference point, and "sleek" and "modern" are explicitly not goals. Concretely: no animation on any list, grid or navigation transition; native controls and real middle-clickable links; compact by default; every screen usable in a phone browser; and no skeleton shimmer. Section 17 enumerates the screens and section 16 says which ship in v0.1; read both rather than assuming a count. A degraded backend gets a non-modal banner; the catalogue never greys out.
````

## v1.1 — superseded

Drafted as the replacement for v1.0 but never applied to the Project's settings. Superseded by
v1.2, which corrects the Go floor, the licence status and the project-status wording.
Preserved verbatim as the record of what was proposed — 7022 bytes.

````
You are working on UsArr: a fast, self-hosted, unified hub and gateway over the media-acquisition ecosystem, running on a single self-hoster's own server. It aggregates the *Arrs (Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian) and media backends (Navidrome, Jellyfin, Audiobookshelf, Komga, Kavita) into one local library you can browse, search and request from, and it exposes protocol surfaces (OpenSubsonic, OPDS) so existing client apps connect to UsArr instead of to each backend individually. It is meant to coexist with the rest of the ecosystem, not replace it. The stack is Go 1.24+ compiled to a single static binary with a SvelteKit SPA embedded in it, over SQLite in WAL mode. The project is pre-alpha: extensive design documentation exists and nothing is implemented. Never write or imply otherwise.

Before you propose or write anything, read CLAUDE.md at the repo root and then docs/ARCHITECTURE.md. Those two files, plus the ADRs in docs/DECISIONS.md, are the source of truth. Section 16 of ARCHITECTURE.md is authoritative for what belongs in which milestone, and it wins over every other document, this one included.

Four principles govern every decision.

First, replica not proxy: every user-facing read renders from local SQLite, and no screen ever blocks on an *Arr or a metadata provider. Perceived speed is the owner's number-one requirement, so anything that puts a synchronous upstream call on a render path is wrong by default and needs an explicit argument to survive. Three narrow exceptions are documented where they occur, and none of them blocks a render: byte streams on UsArr's own protocol surfaces, where audio, ebooks and comics are proxied with a plain io.Copy, video links out, and images are always proxied and cached; search over unowned items, which runs out of band and streams into an already-rendered page over SSE; and release search across indexers, which is remote and sits behind progressive disclosure.

Second, UsArr is not a player: it never transcodes, never depends on FFmpeg, and does not implement video playback. It routes and links out to whichever media server owns the bytes.

Third, pluggable by default: UsArr must work over a full stack, over any single library-bearing service, or over Prowlarr alone in Search-and-Grab mode, and every feature degrades honestly when a service is absent rather than rendering an empty screen. Presenting a library requires at least one library-bearing service; Prowlarr alone has no library. Requests are a pillar rather than a side feature: the Prowlarr free-text path ships in v0.1 and the *Arr-backed flow in v0.2.

Fourth, single-user in v0.1 but multi-user in the schema from migration 0001. Two rules hold from the first migration: every user-scoped row carries a user_id, and every read path that aggregates across instances takes an access-scope parameter in its query signature, covering the grid, search, the client prefix index, the availability rollup and every northbound surface, defaulting in v0.1 to the owner's full scope. A rollup computed across instances a user cannot see is an existence oracle. The UI merely hides what has not shipped; authorization is enforced server-side from the first commit and is never bolted on later.

Adversarial review is mandatory, and the owner asked for it explicitly. Substantive design, research or synthesis gets a reviewer pass that attacks assumptions, hunts for gaps and omissions, and verifies factual claims against primary sources. Every finding is then applied or rebutted in writing in docs/REVIEW-LOG.md. Findings are never quietly dropped.

Verify, do not assert. Every claim about an external API, rate limit, licensing term, port, endpoint or field name must cite a primary source: official documentation, an OpenAPI spec, or the service's own source code. Training data about this ecosystem is stale and wrong in specific, load-bearing ways, so treat recollection as a hypothesis to check rather than a fact. Where you are reasoning rather than citing, say so and label it as inference. Never document a feature as existing when it does not. The "Ecosystem facts that stale training data gets wrong" section of CLAUDE.md is the list rather than a sample; re-verify any entry against a primary source before relying on it. The licence is undecided: AGPL-3.0 is a recommendation only, so do not assert it in headers, package metadata or README copy until Joe confirms.

Security is not negotiable. *Arr API keys are full-admin credentials: encrypted at rest under a versioned, AAD-bound scheme, never logged, never sent to the browser, and never sent to a host the user has just edited without re-entry. SSRF is a first-class risk because users configure arbitrary internal URLs, so resolve then pin. Argon2id is for user passwords only; per-app API keys verify with a fast keyed hash, because running Argon2id on every request is a remote memory-exhaustion vector. Section 14 of ARCHITECTURE.md owns the full threat model.

Cut before you add, but leave the seams open. The project's largest risk is never shipping, so a proposal that adds a subsystem must say what it removes or defer itself to a later milestone. Deferred is not rejected: docs/FUTURE.md holds the features that are wanted later, each with the specific seam in the current design that keeps it cheap to add. Preserve those seams; do not build the future feature early.

Some things are permanently refused rather than deferred. Section 1.4 of ARCHITECTURE.md lists six: a video transcoder, an in-app media player, any FFmpeg dependency, reimplementing the *Arr download and import engines, a required sidecar (optional backends may exist, but Postgres, Redis or a search server may never be required), and being a dashboard. Section 16 adds native TV or mobile apps. Do not propose these and do not reopen them. Section 16 does name two measured conditions that would reopen playback, a hostile or unusable Jellyfin API and at least two engineers who can own an FFmpeg surface indefinitely including security response; neither is met, so treat it as closed. Anything out of scope that is not on those two lists is deferred rather than closed, so check docs/FUTURE.md before assuming either way.

On interface design, read section 17 of ARCHITECTURE.md before touching a screen. It is the only place the screens are specified and nothing else links to it. The constraint is utilitarian over stylish: standard patterns in preference to novel ones, density and speed over animation, and no visual flair that costs render time. Navidrome is the reference point, and "sleek" and "modern" are explicitly not goals. Concretely: no animation on any list, grid or navigation transition; native controls and real middle-clickable links; compact by default; every screen usable in a phone browser; and no skeleton shimmer. Four screens matter, Home, Services health, Search and Requests, and the first three ship in v0.1. A degraded backend gets a non-modal banner; the catalogue never greys out.
````

## v1.0 — superseded

Applied to the Project's settings on 2026-08-16 and replaced by v1.2 the same day. Preserved
verbatim as the record of what was live until then. Verified byte-identical to the
version posted in the research thread at 02:31 UTC on 2026-08-16 — 3849 bytes, md5
`bad4dec2c0766979efcd911a5e5cff75`.

````
You are working on UsArr: a fast, self-hosted, unified hub and gateway over the media-acquisition ecosystem, running on a single self-hoster's own server. It aggregates the *Arrs (Sonarr, Radarr, Lidarr, Prowlarr, LazyLibrarian) and media backends (Navidrome, Jellyfin, Audiobookshelf, Komga, Kavita) into one local library you can browse, search and request from, and it exposes protocol surfaces (OpenSubsonic, OPDS) so existing client apps connect to UsArr instead of to each backend individually. It is meant to coexist with the rest of the ecosystem, not replace it. The stack is Go 1.24+ compiled to a single static binary with a SvelteKit SPA embedded in it, over SQLite in WAL mode. The project is pre-alpha: extensive design documentation exists and essentially no code does.

Before you propose or write anything, read CLAUDE.md at the repo root and then docs/ARCHITECTURE.md. Those two files, plus the ADRs in docs/DECISIONS.md, are the source of truth. The roadmap section of ARCHITECTURE.md is authoritative for what belongs in which milestone.

Four principles govern every decision. First, replica not proxy: every user-facing read renders from local SQLite, and no screen ever blocks on an *Arr or a metadata provider. Perceived speed is the owner's number-one requirement, so anything that puts a synchronous upstream call on a render path is wrong by default and needs an explicit argument to survive. Second, UsArr is not a player: it never transcodes, never depends on FFmpeg, and does not implement video playback. It routes and links out to whichever media server owns the bytes. Third, pluggable by default: UsArr must work over just Prowlarr, over a full stack, or anything in between, and every feature degrades honestly when a service is absent rather than rendering an empty screen. Fourth, single-user in v0.1 but multi-user in the schema from migration 0001: every user-scoped row carries a user_id from the first migration, the UI merely hides what has not shipped, and authorization is enforced server-side from the first commit rather than bolted on later.

Adversarial review is mandatory, and the owner asked for it explicitly. Any substantive design, research or synthesis gets an adversarial reviewer pass against strict standards before it is treated as final — one that attacks assumptions, hunts for gaps and omissions, and verifies factual claims against primary sources. Every finding is then either applied or rebutted in writing in docs/REVIEW-LOG.md. Findings are never quietly dropped.

Verify, do not assert. Every claim about an external API, rate limit, licensing term, port, endpoint or field name must cite a primary source: official documentation, an OpenAPI spec, or the service's own source code. Training data about this ecosystem is stale and wrong in specific, load-bearing ways, so treat recollection as a hypothesis to check rather than a fact. Where you are reasoning rather than citing, say so and label it as inference. Never document a feature as existing when it does not.

Cut before you add, but leave the seams open. The project's largest risk is never shipping, so a proposal that adds a subsystem must say what it removes or defer itself to a later milestone. Deferred is not rejected: docs/FUTURE.md holds the features that are wanted later, each with the specific seam in the current design that keeps it cheap to add. Preserve those seams; do not build the future feature early.

The permanent non-goals are video transcoding, an in-app media player, and any FFmpeg dependency. Everything else that is out of scope is deferred rather than closed.

On interface design: utilitarian over stylish. The bar is tried-and-true, easy to use, and snappy, with no visual flair that costs render time and standard patterns in preference to novel ones. Navidrome is the reference point.
````

## Changelog

### v1.5 — 2026-08-17 (applied 05:05 UTC)

A correctness fix, not a refinement — v1.4's live text pointed agents at a landed/not-yet inventory
in `docs/ARCHITECTURE.md` §16 that commit `0b8637c` had already deleted. An adversarial review
confirmed the dangling referent and recommended the authority framing.

- **v1.4 named three surfaces as lagging:** §16's landed/not-yet split, the README's status column
  and `CLAUDE.md`'s status paragraph. `0b8637c` removed the first outright — §16's v0.1 entry now
  ends "Which of the above is built is not listed here, and the omission is the correction" — and
  narrowed `CLAUDE.md`'s to two coarse facts plus an explicit refusal to enumerate. An agent
  following v1.4 literally goes looking for a section that is not there.
- **Worse than incomplete, the enumeration pointed away from the risk.** The three named surfaces
  are now the three most carefully corrected documents in the repo, while 29 status claims across
  12 files sit in documents v1.4 never mentions — the eight `docs/reference/*` headers,
  `CONFIGURATION.md`, `DEVELOPMENT.md`'s "(not yet)" markers, the mockup banners and
  `ARCHITECTURE.md` §17.5's shipped-vs-target table. `docs/reference/schema.md:3` was already
  contradicting the tree, claiming one migration where four exist.
- **So v1.5 names no surfaces at all.** Enumerating them was the residual state-dependence that made
  v1.4 need this edit; there is nothing left in the sentence for a document to falsify.
- **It adds the author-side rule the reader-side one could not carry:** "no document owns it — the
  tree does" and "Do not write a fresher one; write the pointer." Without it an agent that finds a
  stale count corrects it to a fresher count in a document that still has no business holding one,
  and the cycle repeats. This is `docs/REVIEW-LOG.md` SD-01 compressed to two clauses; the full rule
  belongs in `CLAUDE.md`, not in an 8192-byte settings text.
- **`internal/` is now described as holding "backend surfaces"** rather than "a subsystem", matching
  the vocabulary `CLAUDE.md` and §16 already use, and `internal/db/migrations` is named for the
  schema.
- **No cut was needed:** v1.5 is 8124 bytes against v1.4's 8118. Note for a future editor facing the
  byte ceiling: an earlier review ranked the §17.1 concrete constraints and the ecosystem-facts
  pointer as equally safe cuts. That was wrong. The ecosystem-facts pointer is the worse cut — its
  operative content, that `CLAUDE.md`'s list is the list rather than a sample, exists nowhere else,
  whereas §17.1's constraints arrive one read later via the instruction to read §17 before touching
  a screen.
- **The record lagged the apply by eleven hours.** v1.5 went into the settings at 05:05 UTC, but this
  file kept calling it proposed and kept naming v1.4 as the live text until the 16:02 UTC drift check
  caught it — so for eleven hours the canonical record of which version was live named the wrong one.
  That is the same defect this entry is about, in the file that documents it, and the fix is the
  same: the record has to be updated as part of the apply, not after it.

### v1.4 — 2026-08-17 (applied 04:52 UTC)

Produced by the 04:07 UTC drift check after `main` advanced 264 commits overnight. An adversarial
review of the draft raised 3 majors and 2 minors; all were applied.

- **The project-status sentence is rewritten to be stateless, and this is deliberate.** v1.2 told
  agents to distrust the status prose; v1.3 reversed it because the documents had just been
  corrected; within eleven hours they were wrong again — the Services, Home and Requests screens
  landed 2.5 to 4.5 hours after v1.3 was applied, and `ARCHITECTURE.md:2331` still said the
  Services screen UI did not exist. The new wording never asserts what has landed, only that the
  status prose lags the tree and where to look instead, so it stays true whether the documents are
  currently right or wrong. **Do not flip it back the next time a doc sweep makes them accurate.**
- **The scope/status distinction is now explicit.** The draft applied one undifferentiated
  "unverified" verdict to §16 one paragraph before declaring §16 authoritative over every document
  including the instructions. The text now separates §16's authority over scope from the
  unreliability of status prose, borrowing the formulation `docs/DEVELOPMENT.md` §11 already uses.
- **"Read the tree" now names where** — `web/src/routes` for a screen, `internal/` for a subsystem
  — and restores v1.2's demand to name what you checked, as "name the commit you read". Without it
  the instructions would demand measured evidence for gate results while accepting unmeasured
  status claims, which rot faster.
- **The verify-don't-assert standard is extended to the project's own gates**, from
  `CLAUDE.md:84-88`: report the binary, its version and the commit, and fire a guard before
  trusting it. This is not duplication — v1.3's paragraph governs claims about external sources;
  this governs claims about this repo's own tooling, which it never covered.
  `docs/DEVELOPMENT.md` §11 records four separate incidents where a gate reported green without
  having run what it claimed.
- **The licence sentence is cut** to fit the 8192-byte limit. `CLAUDE.md:186-189` carries all three
  of its operative facts in more detail — AGPL-3.0 confirmed 2026-08-16, no per-file headers, an
  AGPL-compatibility check on new dependencies — and the instructions' first order is to read
  `CLAUDE.md`. The only agent this could harm has already ignored instruction one.
- **Not changed:** the four principles, the permanent refusals, the security paragraph, the Go
  floor and the UI paragraph all re-verified clean against `main`. ADRs 0036, 0037 and 0038 are
  scope-only or component-level and force no edit, since the instructions defer to §16.

### v1.3 — 2026-08-16 (applied 16:34 UTC)

Produced by the 16:02 UTC drift check after `main` advanced 52 commits, merging the
implementation, the design system and ADRs 0025-0034. An adversarial review of the draft raised
1 blocker and 2 majors; all were applied.

- **Project-status sentence rewritten.** v1.2 said implementation was confined to feature
  branches and that `CLAUDE.md`, `README.md` and `ARCHITECTURE.md` §16 all still claimed nothing
  was implemented. All three have since been corrected and the code is on `main`, so v1.2's
  sentence would now make an agent distrust accurate status text. The replacement quotes §16's
  own rule that a milestone label is scope, not status.
- **The §17 sentence corrected twice.** v1.2 said neither `CLAUDE.md` nor the `README` points at
  §17; `README.md:196` and `CLAUDE.md:62`/`:65` now both do. And `docs/design/` landed on `main`,
  so the instructions name it — `CLAUDE.md`'s "Where things live" table still omits it, so an
  agent reading `CLAUDE.md` first would not otherwise find it. The precedence rule is quoted from
  `DESIGN-DIRECTION.md:18`, not invented.
- **`docs/design/` is described as specifying, not implementing.** The review rejected an earlier
  draft that said it implements the visual system: `DESIGN-DIRECTION.md:3` says "None of this
  design is implemented". The wording is "specifies … renders them", because the instructions
  must not break their own no-invented-status rule.
- **The multi-thread convention is referenced, not inlined.** An earlier draft put the whole
  file-ownership map into the settings text; the review blocked it, because none of it was
  written down in the repo, it assigned ownership of a file that was not yet on `main`, and its
  boundaries were already contradicted by commits `c9a60e2`, `9fd4f06`, `26b1cce` and `61e1f95`.
  It is now committed to `docs/DEVELOPMENT.md` §11 where any thread can correct it, and the
  instructions carry one sentence pointing there.
- **Not changed, and deliberately: the v0.1 provider set.** v1.2 defers to §16, which is
  authoritative for scope, so an ADR that moves a provider needs no instructions edit. That still
  holds, and it is why the correction below needs no new version: the instructions text itself is
  unaffected.

  **Correction — the note this bullet originally ended with was already stale on arrival.** As
  first written it said ADR-0035 (Kavita replaces Komga in v0.1) *"is on an unmerged branch and
  §16 has not been updated even there."* The first half was true when the check ran at 16:05 UTC
  and stopped being true minutes later, when the design batch merged ADR-0035 to `main`
  (`ab1a941`, reaching `main` via `e0d4b26`) — so it was already false by the time this entry was
  drafted at ~16:20. The second half is still true. What holds now: **ADR-0035 is Accepted and on
  `main`** (`DECISIONS.md:2852`; `:2881` — *"Komga moves to v0.2, taking Kavita's former
  place"*), while `ARCHITECTURE.md` §16 still pairs Komga with v0.1 and Kavita with v0.2
  (`:2068`, `:2217`), `README.md` still says the same (`:19`, `:61`, `:81`), and
  `SETUP-CHECKLIST.md:142` still files both under v1.0. **§16 is authoritative for milestones and
  currently contradicts the ADR that amends it.** ADR-0035 routed that rewrite to the
  implementation thread (`REVIEW-LOG.md:1634`), so it is queued rather than lost. **Nothing
  anywhere removes Komga from the roadmap — it is re-sequenced to v0.2.**

  **Resolved, and superseded the same day.** The queued §16 rewrite has landed, and it went further
  than the drift fix: [ADR-0036](./DECISIONS.md#adr-0036) — owner-decided, *"I'm fine with starting
  small"* — takes **all four** catalogue sources out of v0.1. §16, §16.0, the new §16.1, §6.4,
  §7.1a, §11.2, the README rows, `FUTURE.md`, `DEVELOPMENT.md`, `SETUP-CHECKLIST.md`,
  `reference/providers.md` and `reference/gateway.md` now agree, and the audit is
  `REVIEW-LOG.md`'s *"Scope audit"* section. **The bullet's conclusion still holds and is what
  matters: nothing removes any source from the roadmap.** Navidrome, Audiobookshelf, Kavita and
  Komga all arrive, one at a time after v0.1, in §16.1's sequence. The instructions text remains
  unaffected — v1.2 defers to §16, which is the whole point of that deferral.

### v1.2 — 2026-08-16 (applied 07:57 UTC)

Produced by a drift check triggered by the scaffold session's report that `CLAUDE.md`'s Go
version was wrong. An adversarial review of the draft raised 1 blocker and 4 majors; all were
applied. Each change below cites what forced it.

- **Go version corrected.** v1.0 and v1.1 both said "Go 1.24+". The binding constraint is
  govulncheck, not any dependency: run against the code on `claude/hearth-thread-kirqa7`, it
  reports 15 called stdlib vulnerabilities at go1.25.7, 5 at go1.25.12, and is clean at
  go1.25.13. goose v3.27.3's own `go` directive is only 1.25.7, so the goose argument for
  1.25.13 overstates it by six patch releases. `go.mod` on that branch declares `go 1.25.13`.
- **The number is stated as a dated instance rather than a fact about the stack**, because the
  floor is time-dependent — it is the newest 1.25.x that scans clean, and when Go 1.27.0 ships,
  1.25 goes EOL and the floor jumps. The text now points at `go.mod` as authoritative and
  `docs/DEVELOPMENT.md` for the reasoning.
- **The reasoning pointer is `docs/DEVELOPMENT.md`, not `CLAUDE.md`.** The draft said
  `CLAUDE.md` carried it; at review time `CLAUDE.md:131` still said "Go 1.24+" on both
  `origin/main` and `kirqa7`, so that pointer would have sent agents to the most wrong of the
  three numbers then in the repo.
- **Licence corrected from undecided to settled.** `LICENSE` (34,523 bytes, AGPL-3.0) landed on
  `main` in commit `719c602`, `CLAUDE.md:168-171` and `README.md:250-253` record owner
  confirmation on 2026-08-16, and ADR-0024 carries the reasoning. The two operational
  consequences are included: no per-file licence headers, and new dependencies need an
  AGPL-compatibility check.
- **Project status reworded.** Code now exists on `claude/hearth-thread-kirqa7` (98 Go files)
  while `CLAUDE.md:22`, `README.md:3` and `ARCHITECTURE.md:1264` all still assert nothing is
  implemented — including on that branch. The draft's "check the branch you are on" gave no
  tiebreak and, because the instructions declare section 16 authoritative over themselves, an
  agent would have resolved the conflict against the instructions. The text now names the stale
  documents explicitly and says to check the working tree.
- **The screens sentence is now a pointer rather than a count.** v1.1 said four screens with the
  first three in v0.1; the unmerged design branch `claude/hearth-thread-vn9w7u` already changes
  that to five screens with the first four, adds section 17.8 (Libraries), and carries ADR-0028
  amending section 17.2 and ADR-0032 amending section 16. A count would need re-applying by hand
  as soon as that merges.
- **"Nothing else links to section 17" narrowed** to "neither `CLAUDE.md` nor the README points
  at it" — seven in-repo references to section 17 do exist, all inside `ARCHITECTURE.md` plus
  `docs/reference/sync.md:204`.

### v1.1 — 2026-08-16 (proposed)

Produced by a drift check against the repo docs at `3b1e6ec`. An adversarial review of the
draft raised 2 blockers, 4 majors and 5 minors; all were applied. Each change below cites the
document that forced it.

- **Permanent refusals corrected.** v1.0 named three (transcoding, an in-app player, FFmpeg)
  and claimed everything else was "deferred rather than closed". `docs/ARCHITECTURE.md`
  section 1.4 lists six permanent refusals, and section 16 adds a seventh (native TV or mobile
  apps). An agent reading v1.0 would treat "add Redis", "ship a mobile app", "be a dashboard"
  or "reimplement the *Arr import engine" as fair game to propose.
- **The refusal list is attributed to both section 1.4 and section 16**, because neither alone
  is complete: section 16 omits "being a dashboard" and section 1.4 omits native apps.
- **The "required sidecar" refusal keeps its qualifier** — optional backends may exist, none
  may ever be required. Without it the text would close the door on the external search engine
  that `docs/FUTURE.md` deliberately defers.
- **Section 16's two measured conditions for reopening playback are named and closed**, so they
  cannot be read as an opening. v1.0 said "do not reopen" absolutely while also declaring that
  section 16 wins over the instructions.
- **Replica-not-proxy now names all three documented exceptions** — byte streams, unowned
  search over SSE, and release search — instead of implying only one. v1.0 would have had an
  agent reject the section 8.6 unowned-search design as a principle violation.
- **The byte carve-out is corrected to audio, ebooks and comics** per ADR-0017, with images as
  the counter-exception: always proxied and cached, never redirected.
- **The multi-user rule now carries both halves of section 1.3** — `user_id` on every
  user-scoped row, and the access-scope parameter in every cross-instance read path's query
  signature. The second half constrains function signatures written in v0.1 and is the
  expensive half to retrofit.
- **Added a pointer to `docs/ARCHITECTURE.md` section 17**, the only screen specification in
  the repo. Nothing else in the repo links to it — neither `CLAUDE.md` nor `README.md` mentions
  it — so a UI agent could miss it entirely. Its operative constraints are summarised.
- **Added the security non-negotiables that v1.0 omitted**, in particular that Argon2id is for
  user passwords only and per-app API keys verify with a fast keyed hash, since Argon2id per
  request is a remote memory-exhaustion vector.
- **Added that the licence is undecided**, so AGPL-3.0 is not asserted in headers, package
  metadata or README copy.
- **Added that requests are a pillar**, with the Prowlarr path in v0.1 and the *Arr-backed flow
  in v0.2.
- **"Works over just Prowlarr" narrowed** to "any single library-bearing service, or Prowlarr
  alone in Search-and-Grab mode". Prowlarr is an *Arr and has no library, which is the exact
  confusion the honest floor exists to prevent.
- **The partial copy of `CLAUDE.md`'s stale-facts list replaced with a pointer to the section**,
  so a partial list is not read as exhaustive and the list has one home.
- **"essentially no code" tightened to "nothing is implemented"**, matching `CLAUDE.md`, which
  forbids softening it.

### v1.0 — 2026-08-16 (applied)

Drafted by the research thread and applied to the Project's settings the same night.
Establishes the four principles, mandatory adversarial review, verify-don't-assert,
cut-before-you-add with seams preserved, and the utilitarian UI bar.

## How this file is maintained

A gatekeeper session reviews project activity — new commits, ADR changes and scope decisions —
on a 12-hour cadence. When something warrants a change it drafts a new version in this file,
adds a changelog entry, and posts the full text in the gatekeeper thread flagged ready to
apply.

What warrants an update:

- scope changes Joe makes;
- reversed or new ADRs;
- new permanent non-goals;
- new services or integrations;
- changed principles;
- corrections of factual errors baked into the instructions;
- drift between `CLAUDE.md`, this file, and the applied settings text.

What does not: cosmetic documentation edits.

The invariant: `CLAUDE.md`, this file, and the applied settings text must never contradict each
other.
