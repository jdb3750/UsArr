// Package libsync is UsArr's catalogue sync core — channel 1, the full import.
// BookOrbit is v0.1's catalogue source (ADR-0052, which replaced Kavita in that
// slot); kavita.go is sunset, not deleted, and still builds and still passes.
// Which adapters exist is a question for the files in this package, and which
// of them cmd/usarr will actually import from is catalogueSource's answer in
// cmd/usarr/import.go — not a list in this comment.
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
// ALSO HERE, AND REACHABLE: channel 3b for BookOrbit. DeltaSync (delta.go,
// bookorbitarrivals.go) is the arrivals-only walk on books.addedAt that ADR-0070
// decided, asked for with a server-side after-filter rather than by ordering the
// collection and stopping client-side, carrying internal/bookorbit/arrivals.go's
// tie mitigation and its wedge stop.
//
// A USER CAN NOW TRIGGER IT: POST /api/v1/services/{id}/sync/delta, handled by
// internal/httpapi's handleDeltaSyncService and implemented by cmd/usarr's
// registry.StartDeltaSync. The route answers 202 without waiting, on the full
// sync's argument and one more — a delta may escalate to a full import, so its
// tail is a full import's tail. Its refusals are decided SYNCHRONOUSLY inside
// StartDeltaSync, including the one this package owns: a source that does not
// implement DeltaSource is refused before anything is launched, so
// ErrNoDeltaChannel reaches a caller rather than a log line. What a walk actually
// did is the delta_walk sync_report row recordDeltaWalk writes, never the
// response body.
//
// ⚠️ THIS PARAGRAPH READ *"NOTHING USER-FACING CAN TRIGGER IT, BECAUSE THERE
// IS NO HTTP ROUTE … internal/httpapi never names DeltaSync … THE ROUTE IS THE
// NAMED NEXT SLICE"*, AND THE NAMED NEXT SLICE LANDED. Both measurements in it
// are now false and are corrected above rather than deleted, because the reason
// they were written has not expired: the person who would otherwise write "delta
// sync ships" is reading THIS DOC and not the git log. What survives unchanged is
// the other half of that measurement — NO TIMER CALLS IT (see "Any timer" below).
// Channel 3b is on-demand and nothing else; a done-when phrased as "delta sync is
// automatic" is satisfied by nothing in this tree.
//
// NOT HERE, and each one is a named channel with its own milestone rather than
// an omission:
//
//   - Channel 3b for every source that is NOT BookOrbit — the ordered page walk
//     with a CLIENT-SIDE stop (ARCHITECTURE.md §7.1a). BookOrbit's 3b above is
//     not that shape, and the difference is not a detail: §7.1a's client-side
//     stop is the mechanism for a source that CANNOT express a since-filter, and
//     BookOrbit can (ADR-0070). Navidrome, Audiobookshelf, Kavita and Komga each
//     get their own measurement and none of them is walked here.
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
// for it.
//
// ⚠️ THIS PARAGRAPH USED TO END *"What is still NOT fetched for Kavita is the
// per-series volume and chapter walk … and that is where work_comic_issue and
// media_file get their rows. Neither is written here"*. HALF OF THAT IS NOW
// FALSE. The walk is files.go — one GET /api/Series/volumes per series, the
// shape §7.2 budgets for Sonarr's episodes, measured at ~4 ms against the
// owner's instance — and it writes media_file plus one primary `edition` per
// series. work_comic_issue is STILL not written HERE, and the reason is the one
// files.go's header gives: a Kavita chapter would be a work of its own, with its
// own identity resolution, search documents and membership. The table itself is
// no longer unwritten — bookorbit.go maps each BookOrbit comic to a
// 'comic_issue' under a 'comic' series (ADR-0068) and store's `comic_issue`
// branch lands the row — but that is a different adapter over a different unit,
// and nothing computes contiguity off the rows that do exist, so the
// availability blob still carries no `missing` key.
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
