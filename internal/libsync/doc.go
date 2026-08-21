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
// ALSO HERE, BUT ONLY BEHIND A FULL IMPORT: channel 4's DELETION HALF. FullImport
// collects every link key its walk observed and hands it to store.SweepDeletions
// on the success path, which tombstones the links and works the read did not
// report and stamps the containers and libraries that went absent with them.
// ⚠️ THE PRECONDITION IS THAT THE READ SAW THE WHOLE LIST, AND ONLY THIS PACKAGE
// CAN ENFORCE IT IN GENERAL: a partial import returns before the call, and
// DeltaSync passes a nil seen-set because an arrivals walk's set difference is
// the whole library. ⚠️ THE WORDS "IN GENERAL" ARE LOAD-BEARING and were added
// after the fact — internal/store refuses ONE shape of violated precondition
// itself, a read that reported zero items against an instance with live links
// (store.ErrSweepRefusedEmptyRead), and says in its own voice that it is one
// shape and only one. Every other partial read is still this package's to
// prevent, which is what the sentence was written to say.
// "Observed" is what the UPSTREAM reported and not what this importer applied —
// sweepScope folds in the items the adapter read and could not map, and withholds
// the containers that answered nothing or were measured short, so an absence is
// only ever concluded inside a container the read can be vouched for.
// Guard 1 lives in internal/store, at the upsert where the resurrection it
// answers is detected. What is NOT here is guard 2 and §7.4 step 4's drift
// comparison; see the NOT-HERE list below and ADR-0074.
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
//
//   - Channel 4's GUARD 2 and its DRIFT STEP. ⚠️ THIS BULLET READ "Channel 4,
//     the reconciliation sweep and its two guards. Nothing here tombstones,
//     deletes or detects drift — remote_hash and remote_identity_hash are
//     WRITTEN here so the sweep has something to compare, and read by nothing
//     yet", AND THREE OF ITS FOUR CLAUSES ARE NOW FALSE (ADR-0074, 2026-08-21).
//     The sweep is here — importer.go calls store.SweepDeletions from
//     FullImport's success path — and it tombstones and deletes. Guard 1 is
//     here: applyOneItem compares remote_identity_hash on any upsert that would
//     clear deleted_at.
//
//     ⚠️ THE CLAUSE THAT SURVIVES INTACT IS THE ONE ABOUT remote_hash. It is
//     still WRITTEN here and read by nothing: no SELECT in non-test Go names
//     the column. ADR-0074 permits a store-seam comparison and this slice does
//     not build one, so drift detection is absent for every source, not merely
//     for this one.
//
//     What is also still absent is guard 2 — deferred for BookOrbit on a
//     measured void premise, and simply unbuilt for the *Arrs, who have no
//     adapter here yet. The four service_instance columns it needs are an
//     annotated seam; see internal/store/serviceinstance.go.
//
//   - The write queue. §7.6's verbs have no target in v0.1.
//
//   - Any timer. ⚠️ THIS ENTRY READ *"FullImport is called on connect or on
//     demand, by cmd/usarr"* AND THE LIST OF CALLERS HAS GROWN (ADR-0076):
//     cmd/usarr now also calls it on a SIX-HOURLY CLOCK, which is §7.4's
//     schedule. The bullet itself still holds and is the reason it is worth
//     keeping — the timer is in cmd/usarr, not here, and nothing in this package
//     knows what hour it is.
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
