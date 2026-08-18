// Package libsync is UsArr's catalogue sync core — channel 1, the full import —
// with Kavita as its first adapter (ADR-0041).
//
// # Why "libsync" and not "sync"
//
// `internal/sync` would shadow the standard library's `sync` in every file that
// imports both, and the importer's own future — a bulk lane and an interactive
// lane in front of one writer connection (reference/sync.md §6 rule 3) — is
// exactly the code that needs stdlib sync. An import alias in every such file is
// a permanent tax paid for a name that saves three characters. "libsync" is
// "library sync", which is what docs/reference/sync.md is about and what
// docs/REVIEW-LOG.md's LS- entries are filed under.
//
// # What is here, and what is deliberately not
//
// HERE: channel 1. FullImport reads a catalogue source end to end, maps it onto
// the schema, and writes it in batched transactions through internal/store.
//
// NOT HERE, and each one is a named channel with its own milestone rather than
// an omission:
//
//   - Channel 3b, the ordered page walk with a client-side stop
//     (ARCHITECTURE.md §7.1a). It is v0.1 work per ADR-0041 and it is a separate
//     commit.
//   - Channel 4, the reconciliation sweep and its two guards. Nothing here
//     tombstones, deletes or detects drift — remote_hash and
//     remote_identity_hash are WRITTEN here so the sweep has something to
//     compare, and read by nothing yet.
//   - The write queue. §7.6's verbs have no target in v0.1.
//   - Any timer. FullImport is called on connect or on demand, by cmd/usarr.
//
// # Two-phase, and which phase this is
//
// §7.2 splits a full import into phase A (id, title, year, external ids, poster
// URL — enough to render a grid) and phase B (overview, file details, media
// info). THE ITEM STREAM IS PHASE A, and for Kavita it CANNOT complete phase A
// alone: POST /api/Series/all-v2 returns no overview and no release year, so
// `year` — a phase-A field — has to come from the per-series metadata read that
// runs after the stream closes. That read is what credits.go performs, and it
// carries `releaseYear` back with the credits rather than paying a second GET
// for it. What is still NOT fetched for Kavita is the per-series volume and
// chapter walk — one call per series, the shape §7.2 budgets for Sonarr's
// episodes — and that is where work_comic_issue and media_file get their rows.
// Neither is written here, and neither is faked: see kavita.go.
//
// # Before you trust a field on an upstream DTO
//
// A SPEC TELLS YOU A FIELD EXISTS. IT DOES NOT TELL YOU WHICH CODE PATH
// POPULATES IT, OR WHETHER ANY DOES. That gap is exactly the error this
// package's ComicVine path was first built on: `SeriesDto.comicVineId` was read
// as a matcher-written identifier because the schema declares it as a typed
// field, when Kavita's PLAIN scanner in fact fills it out of ComicInfo.xml's
// free-text <Web> element — and a per-library flag can silently replace it with
// a value of the wrong kind. Read the upstream's WRITERS, not only its schema:
// grep its source for assignments to the field, and read the version the OWNER
// runs rather than the branch the spec was vendored from. comicvine.go and
// kavita.go's kavitaExternalIDs carry the worked example.
//
// # The seam this keeps
//
// Source is the adapter boundary. It is one interface with two methods and it
// deals in internal/store's neutral types, never in a source's DTOs, so a second
// adapter is a file next to kavita.go rather than a change to importer.go. That
// is the seam CLAUDE.md asks to be kept; the second adapter is the feature that
// does not ship.
package libsync
