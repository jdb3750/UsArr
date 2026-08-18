# UsArr — Roadmap working view

> **This file is NOT authoritative for scope.** [`ARCHITECTURE.md`](./ARCHITECTURE.md) §16 is, and
> it wins over every line here. This is a *working view* of what v0.1 still needs, distilled from
> §16 plus the ADRs, kept short enough to re-read on a phone.
>
> **It is not authoritative for status either.** Status is read off the tree —
> `web/src/routes`, `internal/`, `internal/db/migrations`, `git log` — never off this page. Every
> item below carries **a check you can run**, so the list is re-derived rather than trusted. If a
> check disagrees with the box beside it, the check is right and the box is stale.
>
> **No dates and no estimates appear here, ever**, at the owner's standing instruction. Where a line
> is inference rather than something read off §16, an ADR or the tree, it is marked 🔍.

**Last re-derived against:** `origin/main` `222248c` (2026-08-18).

---

## 1. The objective

1. v0.1 proves the **replica thesis on real data**: the owner's own Kavita, imported, delta-synced,
   reconciled, searched and rendered from local SQLite ([ADR-0041](./DECISIONS.md#adr-0041), §16).
2. Alongside it, **Prowlarr Search-and-Grab** is v0.1's one write path — the request surface for all
   six media types (§8.5, §16).
3. Five screens ship: Home, Services, Libraries, Search, Requests (`CLAUDE.md`, §17).

---

## 2. v0.1 remaining work

Ordered roughly by what the rest depends on, not by size.

- [ ] **Channel 3b — the ordered page-walk delta, for Kavita.** The watermark walk with an overlap
      window and a client-side stop, so an import is not the only way the replica moves.
      *Authority:* §7.1a, §16 v0.1 entry, [ADR-0041](./DECISIONS.md#adr-0041).
      *Done when:* `internal/libsync` has a delta path and `internal/libsync/doc.go` stops listing
      channel 3b under "NOT HERE".

- [ ] **Channel 4 — reconciliation, with both sweep guards and 7-day tombstones.** It carries more
      weight for Kavita than for an \*Arr: a page walk cannot observe a deletion, and Kavita's
      watermark moves on a chapter *add* only.
      *Authority:* §7.4, §16 v0.1 entry ([ADR-0035](./DECISIONS.md#adr-0035) §2a).
      *Done when:* `grep -rn 'missing_since\|orphaned_at' --include=*.go internal/` shows a statement
      that **sets** a non-NULL value. Today every one clears it.

- [ ] **Search over your own library — the query, tiers 1 and 2.** The corpus exists and is written;
      the query does not. `GET /api/v1/search` is the Prowlarr indexer fan-out, a different thing
      over a different corpus.
      *Authority:* §8.2, §17.4, §16 v0.1 entry.
      *Done when:* `internal/httpapi/server.go` registers a library-search route, and
      `web/src/routes/search/+page.svelte` stops being the gap notice it is now.

- [ ] **Home Block A — the media-type summary.** Blocks B and C are drawn; A is not.
      *Authority:* §17.2 as amended by [ADR-0028](./DECISIONS.md#adr-0028), §16 v0.1 entry.
      *Done when:* `web/src/routes/+page.svelte`'s block map stops saying `Block A … NOT DRAWN`.

- [ ] **Libraries — the auto-proposal flow and its Accept step.** The row view over
      `GET /api/v1/libraries` is drawn; the proposal step is not, and its storage question is now
      answered: a proposal is **not** a `library` row, and a row is created only on Accept.
      *Authority:* §17.8, [ADR-0026](./DECISIONS.md#adr-0026), [ADR-0048](./DECISIONS.md#adr-0048).
      *Done when:* an Accept path exists that creates a `library` row, and the proposal has a home
      that is not that table.

- [ ] **The per-series volume and chapter walk, and the rows it writes.** Phase A is served; the walk
      that fills `work_comic_issue` and `media_file` is not fetched and is not faked.
      *Authority:* §7.2, `internal/libsync/doc.go`.
      *Done when:* `internal/libsync/kavita.go` performs the walk and `work_comic_issue` has a writer
      in non-test Go.

- [ ] **The "not identified" badge and the column under it.** Free Kavita returns null identifier
      fields, so this is v0.1's ordinary case, not an edge one. The badge is v0.1; **the remedy is
      not** — see §3.
      *Authority:* §6.4, §16 v0.1 entry, [ADR-0035](./DECISIONS.md#adr-0035) §1.
      *Done when:* the state is rendered in `web/src/routes` off a column that exists in
      `internal/db/migrations`.

- [ ] **The image pipeline, including the §4.4.1 cold-start plan.** `image_asset` is in the schema;
      nothing in `internal/` or `cmd/` writes or serves it.
      *Authority:* §4.4, §16 v0.1 entry.
      *Done when:* `grep -rln image_asset --include=*.go internal/ cmd/` returns a non-test writer.

- [ ] **Library grid: "Load more", keyset pagination, `content-visibility` on grid rows with explicit
      ARIA roles.** `GET /api/v1/library/recent` is the only catalogue read on the wire.
      *Authority:* §4.5, §16 v0.1 entry.
      *Done when:* a paged library read exists beyond `/library/recent`.

- [ ] **System tags `type:`, `format:`, `source:`, `quality:`, `indexer:` with the `downloadId`
      provenance join.**
      *Authority:* §10, §16 v0.1 entry.
      *Done when:* a tag vocabulary has a writer and a filter path in `internal/`.

- [ ] **`usarr key rotate`, working, on top of key versioning and AAD.**
      *Authority:* §14, `reference/security.md` §1.5, §16 v0.1 entry.
      *Done when:* `grep -rn 'rotate' cmd/usarr/*.go` finds a subcommand, not nothing.

- [ ] **LS-170 — lift `httpapi.redactText` into `internal/ssrf`, and the three fixes around it.**
      Four steps, in the REVIEW-LOG's order: stop writing `last_error` raw; redact inside
      `parseErrorBody` including the `problemDetails` branch; redact the three `cmd/usarr` log sites
      (better: a redacting `slog.Handler`); add a guard over `parseErrorBody`'s output **and fire it
      deliberately**.
      *Authority:* `REVIEW-LOG.md` LS-170, `reference/security.md` §5. **No ADR** — it applies rules
      those documents already state.
      *Done when:* `redactText` (or its exported successor) is defined in `internal/ssrf` and
      `internal/kavita` calls it; today it is defined in `internal/httpapi/redact.go` and called from
      `internal/httpapi` only.

- [ ] **The Docker image, and `VACUUM INTO` backups as a shipped path.** `cmd/usarr/backup.go` exists;
      there is no `Dockerfile` anywhere in the tree.
      *Authority:* §15, §16 v0.1 entry.
      *Done when:* a `Dockerfile` exists.

- [ ] **The arm64 RSS spike.** §16 calls it a day-one spike. `internal/db/spike/` exists; whether the
      arm64 measurement was taken is not readable from the tree.
      *Authority:* §13, §16 v0.1 entry.
      *Done when:* a recorded measurement exists in `docs/`.

- [ ] **The zero-external-providers evidence clause for Kavita.** §16 says v0.1 needs no TMDB account
      because the source carries its own metadata. That claim was evidenced against Radarr's
      `MovieResource` and Sonarr's `SeriesResource`, and **neither is in v0.1 any more**; the
      equivalent primary-source check against Kavita's payloads is owed and has not been made.
      *Authority:* §16 v0.1 entry, which flags it against itself.
      *Done when:* `docs/RESEARCH.md` carries the citation.

**Already discharged, listed so nobody re-opens them:** the Kavita `LastChapterAdded` watermark probe
(ran 2026-08-17 against the owner's live instance and passed — [ADR-0035](./DECISIONS.md#adr-0035)
§2a); the three Kavita subtype tables (`00006_kavita_subtypes.sql`); `work_credit`
(`00007_work_credit.sql`, [ADR-0044](./DECISIONS.md#adr-0044)).

---

## 3. Blocked and sequenced

| Item | Blocked on / sequenced behind |
|---|---|
| Wiring Kavita's `PluginVersion`, or any second Kavita endpoint taking a credential in a query or path | **All four LS-170 steps must land first.** That ordering is the whole difference between latent and breached (`REVIEW-LOG.md` LS-170, *Ordering constraint*). |
| A second catalogue adapter (Navidrome, then Audiobookshelf, then Komga) | v0.1's Kavita adapter landing and running against a real library. The rule is unchanged across every re-sequencing: **one source, proven on real data, before a second adapter** (§16.0, §16.1, [ADR-0036](./DECISIONS.md#adr-0036)). |
| The minimal write path — `monitor`, `unmonitor`, `delete`, `add`, the queue worker and its settlement loop | **v0.2**, with the first \*Arr adapter ([ADR-0042](./DECISIONS.md#adr-0042), [ADR-0045](./DECISIONS.md#adr-0045)). `write_queue` stays in the schema with **no writer for the whole of v0.1** — that is the seam, and it costs no migration ([ADR-0039](./DECISIONS.md#adr-0039)). |
| [ADR-0039](./DECISIONS.md#adr-0039)'s outstanding Go `state`-vocabulary declaration and validation | The first `write_queue` writer, which is v0.2's. |
| The minimal match-correction UI — the remedy for the badge in §2 | **v0.2** ([ADR-0043](./DECISIONS.md#adr-0043), [ADR-0045](./DECISIONS.md#adr-0045)). v0.1 ships the defect's badge without its remedy for a whole milestone, and §16 states that cost rather than burying it. |
| A request destination on a library binding | A service that advertises `Add` under §8.3's capability filter. **No service v0.1 connects does** — Prowlarr's grab path posts to Prowlarr's own download client — so §17.8 drops the column for v0.1. It returns with Sonarr and Radarr at v0.2. |
| The queue-state column on Requests' `Recent grabs` block | The first `write_queue` writer — a v0.2 addition, not a v0.1 gap. |
| Knowing whether an *already connected* source covers a media type | One capability array on the health row, derived at ingest. **Build neither it nor §8.3's `Caps.MediaKinds` now** — the seam is [`FUTURE.md`](./FUTURE.md) §20. Naming *which source will populate a type* is unblocked and is a constant derived from §16. |

### Open decision — BookOrbit as a books backend

**Tracking, not a decision. No ADR backs it, and §16 assigns BookOrbit nothing.**

Joe is standing up a BookOrbit instance and is leaning toward migrating his books backend off Kavita
(2026-08-18, verbatim: *"in my heart i kind of want to migrate to book orbit… it doesn't have a paid
tier"*). What a 2026-08-17 evaluation at BookOrbit HEAD `4a420a04` found, on both sides:

- **For:** a real versioned `/api/v1`, nothing paywalled, and 14 free per-book external ids.
- **Against:** **no inbound API key** — headless auth needs the account password, which is worse than
  UsArr's Kavita credential model; an `updatedAt` watermark that **misses tag, genre and author
  edits**; and **no manga or comic external ids**.

**The standing recommendation from that evaluation is: do NOT switch UsArr's first adapter off
Kavita.** If Joe confirms after standing his instance up, a BookOrbit adapter becomes a *candidate*
work item — the importer, stream and UI plumbing is source-agnostic and **the Kavita adapter stays
either way** — and §16.1's Navidrome-next sequencing may be revisited. **Nothing is re-planned around
this until he confirms.**

---

## 4. Joe's manual steps

Things no agent in this repo can do. Nothing here is blocked on code.

- **Run `deploy/update.sh`** on the server to pull and restart. `deploy/status.sh` reports what is
  running.
- **Run a full sync on the Kavita instance** so the library the importer reads is current.
- **Confirm or drop the BookOrbit direction** once the instance is up (§3).
- **Verify Symfonium's `apiKeyAuthentication` support against a live client** before any gateway code
  is written. Far out — it gates v0.4, not v0.1 — but it is unverified and the whole v0.4 success
  criterion rests on it (§16 v0.4 entry).

---

## 5. Out of scope

Not restated here, deliberately — one list, one owner:

- **Deferred, wanted later, each with the seam that keeps it cheap:** [`FUTURE.md`](./FUTURE.md).
- **Permanent refusals:** [`ARCHITECTURE.md`](./ARCHITECTURE.md) §1.4 and §16's *Explicitly never*.
  Do not propose them and do not reopen them.
- **Which milestone owns a thing:** §16. Not this file.
