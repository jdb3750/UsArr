package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/imagecache"
	"github.com/jdb3750/UsArr/internal/imagepipeline"
	"github.com/jdb3750/UsArr/internal/libsync"
	"github.com/jdb3750/UsArr/internal/store"
)

// The catalogue full import (channel 1), wired.
//
// It lives beside RunProber because it is the same kind of thing: outbound HTTP
// driven by a background worker, in the one package that is allowed to hold a
// client (ARCHITECTURE.md §2.3 rule 1). It is NOT the prober: the prober runs on
// a ticker, and this deliberately does not.
//
// # What triggers it, and what deliberately does not
//
// THREE TRIGGERS, all explicit:
//
//   - ON CONNECT. bootstrapImport runs once when a Kavita client stack is built
//     for an instance that has never completed a full sync. That is the
//     first-run bootstrap: the user adds a Kavita, and a catalogue appears
//     without them having to find a button.
//   - ON DEMAND, from the Services screen's "Run full sync now" action
//     (ARCHITECTURE.md §17.3), which reaches StartImport through
//     POST /api/v1/services/{id}/sync.
//   - IN PROCESS, by calling FullImport directly.
//
// WHY THE THIRD ONE HAD TO EXIST. bootstrapImport is gated on last_full_sync_at
// and that gate is correct for what it guards — a restart must not re-read the
// whole library — but it left the process with NO WAY AT ALL to ask for a second
// import. Every fix that changes what an import WRITES (the credit pass writing
// work.year is the one that forced this) is unreachable for rows already
// imported, for the life of the database, because the only trigger declines to
// run again. A re-import is therefore not a feature of the sync core: it is what
// makes the sync core's own fixes deliverable.
//
// NO TIMER. There is no periodic re-import here and adding one would be the
// wrong shape anyway: a re-read of the whole library every N hours is channel
// 4's job (the reconciliation sweep, which compares hashes and touches <1% of
// rows), not channel 1's. Neither channel 4 nor channel 3b is built here — which
// is also why this is a FULL re-import rather than a delta: internal/libsync has
// exactly one channel, and a delta walk over ADR-0035 §2a's watermark would
// revisit only what changed upstream and so could never repair a row UsArr
// itself wrote wrongly.

// FullImport runs channel 1 against one instance. It is the manual trigger.
//
// It blocks for the length of the import, which for a large library is minutes.
// Every caller in this process therefore runs it on its own goroutine; the
// contract is stated here rather than hidden behind an internal goroutine,
// because a function that returns before its work is done cannot report what the
// work did.
func (g *registry) FullImport(ctx context.Context, instanceID int64) (libsync.Report, error) {
	// THE MUTUAL-EXCLUSION POINT, and it is here rather than in StartImport
	// because there are three callers and only one of them is StartImport. A
	// bootstrap racing a hand-pressed "Run full sync now" is the case a guard
	// inside the handler would miss entirely.
	if !g.beginImport(instanceID) {
		return libsync.Report{}, fmt.Errorf("%w for service instance %d", httpapi.ErrImportInProgress, instanceID)
	}
	defer g.endImport(instanceID)
	return g.fullImportLocked(ctx, instanceID)
}

// fullImportLocked is FullImport's body, with the guard ALREADY HELD by the
// caller. Splitting it is what lets StartImport claim the guard synchronously —
// so its caller gets a truthful "started" or "already running" — and release it
// from the goroutine that actually finishes the work.
func (g *registry) fullImportLocked(ctx context.Context, instanceID int64) (libsync.Report, error) {
	rep, err := g.runImport(ctx, instanceID)
	if err != nil {
		// THE TERMINAL FRAME FOR A RUN THAT STOPPED, and this wrapper is the
		// only site that can publish it. Two of the failures below happen
		// BEFORE an Importer exists — a service that is not a Kavita, and a
		// stored credential that will not open — so libsync.FullImport cannot
		// see them and a publish inside it would leave those two silent, which
		// is exactly half of the defect (REVIEW-LOG LS-152, LS-180).
		//
		// It is deliberately NOT published for the refusals decided before this
		// function is entered — an import already running (FullImport), and
		// StartImport's own kind/armed checks. Those callers get a synchronous
		// non-2xx and http-api.md §4.4 is explicit that it means NO IMPORT
		// STARTED for that press; a terminal frame there would claim a run that
		// never existed.
		g.publishImportStopped(instanceID, rep)
	}
	return rep, err
}

// runImport is fullImportLocked's body: everything that can fail, with nothing
// between it and the publish above.
func (g *registry) runImport(ctx context.Context, instanceID int64) (libsync.Report, error) {
	entry, err := g.entry(ctx, instanceID)
	if err != nil {
		return libsync.Report{}, err
	}

	log := g.log.With("instance_id", instanceID, "instance", entry.instance.Name)
	src, err := catalogueSource(entry, log)
	if err != nil {
		return libsync.Report{}, err
	}

	im := &libsync.Importer{
		Store:  g.st,
		Source: src,
		Log:    log,
		// v0.1's single owner. The parameter exists so multi-user is a
		// behaviour change rather than a redesign (§1.3 rule 1).
		UserID:   store.SystemUserID,
		Progress: g.importProgress(),
		Covers:   g.coverPipeline(entry, log),
	}
	rep, importErr := im.FullImport(ctx, instanceID)

	// RECORDED WHETHER OR NOT THE IMPORT SUCCEEDED, and before the error is
	// returned. Every skip counted below is a book the walk already read and
	// declined to map; an import that died on library three does not make the
	// comics it skipped in libraries one and two less skipped. Same rule
	// importer.go's recordFileWalkFailures states for a dropped file walk.
	if bo, ok := src.(*libsync.BookOrbitSource); ok {
		g.recordSkippedItems(ctx, instanceID, bo.Skipped(), log)
		// Same rule, same reason: the completeness verdict is a fact about the
		// run that has to outlive the run. It is recorded even for a failed
		// import, because a shortfall measured against library one is still true
		// when the walk dies on library three.
		g.recordCompleteness(ctx, instanceID, bo.Completeness(), log)
		// Same rule a third time. A comic that took a residue default in library
		// one took it whether or not library three then failed, and ADR-0068
		// decision 4 wants the FIRST real import to produce these numbers — an
		// import that has to succeed before it measures anything would postpone
		// the measurement past exactly the runs most worth measuring.
		g.recordComicResidue(ctx, instanceID, bo.Comics(), log)
	}
	return rep, importErr
}

// coverPipeline builds the cover fetcher for one instance, or returns nil.
//
// ⚠️ NIL IS A FIRST-CLASS ANSWER AND NOT A FAILURE. libsync.Importer.Covers nil
// disables phase D entirely, which is the correct state for a Kavita — nothing
// in internal/kavita satisfies imagepipeline.CoverSource and ADR-0052 makes
// BookOrbit the source this milestone proves — and for any install with no image
// cache directory. Returning nil is how "the feature degrades honestly when a
// service is absent" is spelled for a pass rather than for a screen.
//
// IT LIVES IN cmd/usarr FOR §2.3 RULE 1'S REASON: this is the one package
// allowed to hold an outbound client, and the pipeline is built AROUND the
// client, taking *bookorbit.Client directly with no adapter. That is why
// libsync.BookOrbitReader gained no method — the catalogue adapter and the cover
// fetch are two independent users of one client, joined only here.
//
// THE CLIENT IS THE ENTRY'S OWN, so the cover fetch goes through the same
// resolve-then-pin transport, the same breaker and the same credential every
// other call to that instance uses. Building a second HTTP path for images would
// be a second SSRF policy to keep in step, and the one that drifts is the one
// that gets used (imagepipeline's CoverSource header says the same).
func (g *registry) coverPipeline(entry *registryEntry, log *slog.Logger) libsync.PosterFetcher {
	if entry.bookorbit == nil || g.imageCacheDir == "" {
		return nil
	}
	pipe, err := imagepipeline.New(entry.bookorbit, imagecache.New(g.imageCacheDir), g.st, nil)
	if err != nil {
		// A construction failure is a programming error here — every argument
		// is non-nil by the guard above — so it is logged and the pass is
		// disabled rather than failing an import that has nothing to do with
		// artwork.
		log.Warn("no cover pass for this import: the image pipeline could not be built", "error", err)
		return nil
	}
	return pipe
}

// catalogueSource picks the adapter for one instance's client stack.
//
// TWO CATALOGUE SOURCES NOW, and the switch is on which client the entry holds
// rather than on entry.instance.Kind, because that is the field the compiler
// checks: exactly one of the three client fields is non-nil, chosen by kind
// when the stack was built, so a kind that grows a client without an adapter
// falls into the refusal below rather than into a nil dereference.
func catalogueSource(entry *registryEntry, log *slog.Logger) (libsync.Source, error) {
	switch {
	case entry.kavita != nil:
		// The adapter gets its OWN logger because it is where a refused identity
		// claim is reported, and the importer never sees one — the mapping has
		// already dropped it by the time a CatalogueItem exists.
		src := libsync.NewKavitaSource(entry.kavita)
		src.Log = log
		return src, nil
	case entry.bookorbit != nil:
		// Its logger carries two things the importer cannot see: the §14 scope
		// verdict consulted before the first catalogue read (ADR-0052's gate,
		// honoured in BookOrbitSource.gate), and the per-library count of books
		// read but not mapped.
		src := libsync.NewBookOrbitSource(entry.bookorbit)
		src.Log = log
		return src, nil
	}
	return nil, fmt.Errorf(
		"%q has kind %q; v0.1 imports a catalogue from bookorbit and from kavita, and from nothing else (ADR-0052, ADR-0041)",
		entry.instance.Name, entry.instance.Kind)
}

// skipCovers and skipDoesNotCover are written into EVERY skip row, on
// completenessCovers's reasoning and for a sharper version of the same hazard.
//
// ⚠️ A SKIP COUNT IS NOT A COMPLETENESS VERDICT. It says how many items the walk
// read and did not map, and it says nothing at all about whether the items it
// DID map are all the items there are — that is the other axis, measured by a
// different check with a different failure mode. A row carrying one clean number
// and no scope invites exactly the reading ADR-0061 exists to prevent.
const (
	skipCovers = "how many items this container's walk read and deliberately " +
		"did not map, and why"
	skipDoesNotCover = "whether the items that WERE mapped are all of them — a " +
		"skip count is not a completeness verdict, and the two are measured " +
		"separately (sync_report kind 'content_completeness')"
)

// skipReason is the short clause a §17.8 row renders, and skipEffect is the long
// form that stays in the database.
//
// ⚠️ THE SPLIT IS §9.1's, not tidiness. The reason reaches a browser and lands in
// a table cell whose overflow policy is a wrap, so it is one clause — the same
// constraint completenessReason is written against. Everything the operator
// needs and the cell cannot hold is in the effect, which does not travel.
//
// ⚠️ BOTH USED TO NAME COMICS AND NEITHER MAY ANY MORE. skipReason read *"UsArr
// maps prose books only; a comic or an unclassified file has no row"* and
// skipEffect carried *"A comic-format book has no settled unit of work in
// UsArr"*. ADR-0068 gave comics a unit of work and this binary imports them, so
// the only remaining reason a BookOrbit book is skipped is the one BookOrbit
// itself cannot classify. A row's own prose is what an operator reads out of the
// database months later; leaving the comics clause in would have it explain a
// count that is now structurally zero.
const (
	skipReason = "a file BookOrbit itself cannot classify has no row"
	skipEffect = "those books have no work row, and this library's item count is short by " +
		"that many; every other book in the library was imported. A book whose primary " +
		"file has no format — or that has no files at all — is one BookOrbit itself " +
		"classifies as media kind 'unknown', which is a fact about the upstream scan " +
		"rather than about UsArr's coverage"
)

// recordSkippedItems writes the durable half of "UsArr did not take all of your
// books".
//
// ⚠️ A LOG LINE IS NOT A RECORD. The adapter already logged each container's
// tally as it finished, and an import triggered by a background connect has no
// caller left to read the Report by the time anyone opens the Libraries screen
// and asks why a library of 900 shows 40. That is the same argument
// importer.go makes for writing container_declined rows rather than only
// returning them, and it is the whole reason the skip is counted at all: a
// skipped item that vanishes silently is indistinguishable from one that never
// existed.
//
// # ONE ROW PER CONTAINER THE WALK REACHED, ZERO OR NOT (ADR-0063)
//
// ⚠️ THIS USED TO WRITE A ROW ONLY WHERE SOMETHING WAS SKIPPED, and ADR-0063
// supersedes ADR-0061 §5 on exactly that. "None skipped" and "nothing observed
// this container" rendered identically as an absent row, so the read had to
// borrow the completeness row as evidence a container had been walked at all —
// and the reader one field over already uses row-per-walked-container, so the
// two absences meant opposite things one column apart. The cost of the fix is
// one row per container per import.
//
// ⚠️ THE INVARIANT IS NOT ENFORCED HERE AND CANNOT BE. This function writes a
// row for every element of the slice it is handed and knows nothing about which
// containers exist; the set is BookOrbitSource.Skipped's, which is populated
// during the walk. Synthesising the zero rows here would mean taking a container
// list from Containers() — i.e. from before the walk — and handing a row to
// every container an aborted import never reached, which is the imprecision the
// change exists to retire.
//
// A FAILURE TO RECORD DOES NOT FAIL THE IMPORT. The rows are already committed
// and correct; losing the note about what was skipped is worth a warning, not a
// rollback of a catalogue.
func (g *registry) recordSkippedItems(
	ctx context.Context, instanceID int64, skips []libsync.ContainerSkips, log *slog.Logger,
) {
	for _, s := range skips {
		note := store.SkipNote{
			Name:         s.Name,
			Comics:       int64(s.Comics),
			Unknown:      int64(s.Unknown),
			Covers:       skipCovers,
			DoesNotCover: skipDoesNotCover,
		}
		// ⚠️ NO REASON AND NO EFFECT ON A ZERO ROW. Both sentences explain a skip
		// that happened; on a row recording that nothing was skipped they would
		// assert a cause for a non-event, to an operator reading sync_report and
		// to the fold that lifts Reason onto the wire. The scope pair above stays
		// on every row, because "a skip count is not a completeness verdict" is
		// true of a zero exactly as it is true of a thousand.
		if s.Total() > 0 {
			note.Reason, note.Effect = skipReason, skipEffect
		}
		detail, err := json.Marshal(note)
		if err != nil {
			log.Warn("cannot encode the skipped-item note", "library_id", s.RemoteID, "err", err)
			continue
		}
		if err := g.st.RecordSyncReport(ctx, instanceID,
			store.SyncReportItemsSkipped, "library", s.RemoteID, string(detail)); err != nil {
			log.Warn("cannot record how many books were skipped; the import itself stands",
				"library_id", s.RemoteID, "err", err)
		}
	}
}

// completenessCovers and completenessDoesNotCover are written into EVERY
// completeness row, and the second one is the reason both exist.
//
// ⚠️ A CLEAN BOOK-LEVEL CHECK MUST NOT BE READ AS COMPLETENESS. The check
// compares two counts INSIDE one container UsArr's credential can already see.
// Whether whole containers are hidden from the account is a second axis and is
// unanswerable read-only — BookOrbit's LibraryAccessGuard throws an identical
// ForbiddenException for "the library exists and this account has no access row"
// and for "there is no such library" — so `complete` on every library UsArr can
// see is not a statement that UsArr can see every library.
//
// The scope travels IN THE ROW rather than only in a doc comment, because the
// row is what an operator reads out of the database and what a browser renders,
// and neither of those has this file open.
const (
	completenessCovers = "how many of this container's items UsArr's credential " +
		"was shown, against the container's own unfiltered count"
	completenessDoesNotCover = "whether whole containers are hidden from UsArr's " +
		"credential — the upstream returns the same refusal for a container that " +
		"exists and one that does not, so it cannot be measured from here"
)

// recordCompleteness writes the durable half of "UsArr may not be seeing all of
// your books".
//
// ONE ROW PER CONTAINER, INCLUDING THE ONES THAT WERE FINE. An absent
// completeness row means nothing was ASKED — the adapter does not check, or the
// import never reached this container — and if that looked the same as a clean
// verdict, an instance whose probes all started failing would read as an
// instance with nothing wrong. That is the defect this feature exists to close,
// and writing rows only for shortfalls would have recreated it inside the fix.
//
// ⚠️ recordSkippedItems NOW FOLLOWS THE SAME RULE, AND USED NOT TO. This comment
// called that function "the opposite" of this one, and for a while the row
// written here was LOAD-BEARING for it: the skip read borrowed a completeness
// verdict as its evidence that a container had been walked at all, because an
// absent skip row could not tell "nothing left out" from "nobody looked".
// ADR-0063 ended that. Both writers now put a row per container per import, the
// skip read stands on its own rows, and nothing here is evidence for anything
// there — which is the state two adjacent readers on the same screen should have
// been in from the start.
//
// ⚠️ THE TWO ROW SETS ARE STILL NOT THE SAME SET, and that difference is the
// point rather than a bug. These verdicts are measured in Containers(), BEFORE
// the walk, so an import that dies part-way still writes a completeness row for
// every container including the ones it never reached. The skip rows come from
// tallies raised during the walk, so they stop where the walk stopped. Decoupled
// means the skip read no longer inherits the wider set.
//
// A FAILURE TO RECORD DOES NOT FAIL THE IMPORT, on recordSkippedItems's
// reasoning: the catalogue rows are already committed and correct, and losing
// the note about how complete they are is worth a warning, not a rollback.
func (g *registry) recordCompleteness(
	ctx context.Context, instanceID int64, checks []libsync.ContainerCompleteness, log *slog.Logger,
) {
	for _, c := range checks {
		note := store.CompletenessNote{
			State:        string(c.State),
			Container:    c.Name,
			Total:        c.Total,
			Visible:      c.Visible,
			Hidden:       c.Hidden(),
			Reason:       c.Reason,
			Covers:       completenessCovers,
			DoesNotCover: completenessDoesNotCover,
		}
		detail, err := json.Marshal(note)
		if err != nil {
			log.Warn("cannot encode the completeness note", "library_id", c.RemoteID, "err", err)
			continue
		}
		if err := g.st.RecordSyncReport(ctx, instanceID,
			store.SyncReportContentCompleteness, "library", c.RemoteID, string(detail)); err != nil {
			log.Warn("cannot record how much of this library UsArr could see; the import itself stands",
				"library_id", c.RemoteID, "err", err)
		}
	}
}

// The two sync_report kinds ADR-0068 decision 4 asks for, and the scope pair
// every one of their rows carries.
//
// ⚠️ THEY ARE UNEXPORTED LITERALS, on syncReportCoverPassIncomplete's stated
// rule: `sync_report.kind` carries no CHECK, and a store constant exists only
// where the kind has a READER in another package. Nothing reads these yet — they
// are written so the owner's first real import can be measured — so a second
// exported spelling would be "a promise with no second side". The day a screen
// reads one, it moves to internal/store beside SyncReportItemsSkipped.
const (
	syncReportComicSeriesSynthesized = "comic_series_synthesized"
	syncReportComicSeriesDeclined    = "comic_series_memberships_declined"
)

// residueCovers and residueDoesNotCover are the scope of the claim, written into
// EVERY residue row on SkipNote's rule — a number with no scope invites the
// reading it does not support, and the operator reading this out of the database
// does not have this file open.
const (
	residueCovers = "how many of this container's comics were ingested on a " +
		"residue default rather than on a series BookOrbit itself named, and how " +
		"many series memberships were recorded without being acted on"
	residueDoesNotCover = "whether the default was the RIGHT answer for any " +
		"particular comic. It is a count of how often UsArr fell back, not a " +
		"quality verdict on the upstream's metadata, and a high number is a " +
		"sizing input rather than a fault"
)

// comicResidueNote is the JSON contract of sync_report.detail for both residue
// kinds.
//
// ⚠️ EVERY STRING IN IT IS USARR'S OWN PROSE AND EVERY UPSTREAM VALUE IN IT IS A
// NUMBER, which is store.SkipNote's rule made structural rather than remembered:
// reference/security.md §5 keeps upstream response bodies out of this column, and
// a series NAME is an upstream response body. `Name` is the container's name,
// which SkipNote already carries into this column on the same terms — it is what
// makes a row readable by someone who has only sync_report in front of him.
type comicResidueNote struct {
	Name string `json:"name,omitempty"`

	// SynthesizedSeries and MultiSeries/ExtraMemberships are BOTH written into
	// BOTH rows. The row kind says which number the row is ABOUT; carrying the
	// other one costs a field and saves the reader a join against a row he may
	// not have.
	SynthesizedSeries int `json:"synthesized_series"`
	MultiSeries       int `json:"multi_series_books"`
	ExtraMemberships  int `json:"declined_memberships"`

	// Sample is up to libsync's cap of the declined memberships, ids only. It is
	// absent on a synthesized-series row: there is nothing to sample there — the
	// declined set for a comic with no series is empty by definition.
	Sample []libsync.DeclinedMembership `json:"sample,omitempty"`

	// SampleCapped reports that Sample is shorter than ExtraMemberships because
	// the cap bit, so a reader cannot mistake a truncated list for the whole one.
	SampleCapped bool `json:"sample_capped,omitempty"`

	Effect       string `json:"effect,omitempty"`
	Covers       string `json:"covers,omitempty"`
	DoesNotCover string `json:"does_not_cover,omitempty"`
}

// The two effect sentences. They are operator-facing and do not travel to a
// browser.
const (
	synthesizedEffect = "each of these comics reported no series, so it was ingested as " +
		"an issue under a single-issue series named for the book, with is_oneshot set. It " +
		"is visible on /library/comics as one series with one issue; it was neither " +
		"dropped nor promoted to a series work in its own right"
	declinedEffect = "each of these comics belongs to more than one series upstream. It " +
		"was bound to BookOrbit's own primary series and the remaining memberships were " +
		"recorded here and not acted on: no second parent, no second membership and no " +
		"work_relation edge was written. The tier that would adjudicate them is v0.3"
)

// recordComicResidue writes the durable half of "these comics did not arrive the
// straightforward way".
//
// # TWO ROWS PER CONTAINER PER IMPORT, ZERO OR NOT
//
// ADR-0063's rule, and recordSkippedItems and recordCompleteness both already
// follow it: "none" and "nobody looked" must not render identically. A zero row
// here is what makes a later zero readable — ADR-0068 decision 4 exists to
// MEASURE how often each default fires, and a measurement whose zero is an
// absence measures nothing.
//
// ⚠️ THE ROWS CARRY NO REASON SENTENCE WHEN THE COUNT IS ZERO, on
// recordSkippedItems's rule: an effect sentence on a row recording that nothing
// happened asserts a cause for a non-event.
//
// A FAILURE TO RECORD DOES NOT FAIL THE IMPORT, on the same reasoning as its two
// neighbours: the catalogue rows are committed and correct, and losing the note
// about how they got there is worth a warning, not a rollback.
func (g *registry) recordComicResidue(
	ctx context.Context, instanceID int64, comics []libsync.ContainerComics, log *slog.Logger,
) {
	for _, c := range comics {
		synth := comicResidueNote{
			Name:              c.Name,
			SynthesizedSeries: c.SynthesizedSeries,
			MultiSeries:       c.MultiSeries,
			ExtraMemberships:  c.ExtraMemberships,
			Covers:            residueCovers,
			DoesNotCover:      residueDoesNotCover,
		}
		if c.SynthesizedSeries > 0 {
			synth.Effect = synthesizedEffect
		}
		g.writeResidueRow(ctx, instanceID, syncReportComicSeriesSynthesized, c.RemoteID, synth, log)

		declined := synth
		declined.Effect = ""
		declined.Sample = c.Sample
		declined.SampleCapped = len(c.Sample) < c.ExtraMemberships
		if c.MultiSeries > 0 {
			declined.Effect = declinedEffect
		}
		g.writeResidueRow(ctx, instanceID, syncReportComicSeriesDeclined, c.RemoteID, declined, log)
	}
}

func (g *registry) writeResidueRow(
	ctx context.Context, instanceID int64, kind, ref string,
	note comicResidueNote, log *slog.Logger,
) {
	detail, err := json.Marshal(note)
	if err != nil {
		log.Warn("cannot encode the comic residue note", "kind", kind, "library_id", ref, "err", err)
		return
	}
	if err := g.st.RecordSyncReport(ctx, instanceID, kind, "library", ref, string(detail)); err != nil {
		log.Warn("cannot record how this library's comics were bound; the import itself stands",
			"kind", kind, "library_id", ref, "err", err)
	}
}

// importPhaseStopped is the terminal failure phase, specified by
// docs/reference/http-api.md §5.5 and consumed by web/src/lib/services.ts.
//
// IT IS `stopped`, NEVER `failed`. `failed` is already taken in the SPA
// (SyncPhase, services.ts) for the refusal that means NO IMPORT STARTED, so the
// catalogue is untouched — the exact negation of this frame, which says a run
// did start, did commit batches, and those rows STAND. §5.5.2 owns that
// reasoning; do not rename it.
const importPhaseStopped = "stopped"

// publishImportStopped sends the one terminal frame a stopped run gets.
//
// IT IS THE SAME Progress STRUCT AND IT GAINS NO FIELD (§5.5.1). There is no
// cause, no reason, no status code and no upstream text — deliberately, and
// §5.5.5 owns that: reference/security.md §5 forbids upstream strings in an SSE
// payload, and the absence of a field is a stronger guarantee than a rule about
// what may be put in one. The operator reads the real error in the process log,
// in sync_report, and on the Services health row.
//
// The counters are how far the run got, not a result: Applied is the rows that
// reached a COMMITTED batch and therefore stand, because a stopped import is
// never rolled back (§5.5.4). Both are 0 for the two failures that happen
// before an Importer exists, which is honest — nothing was written.
//
// If the hub is not wired it publishes nothing, exactly as a healthy run's
// frames are not published: importProgress returns nil there, so silence on
// that build is not a claim about the import either way.
func (g *registry) publishImportStopped(instanceID int64, rep libsync.Report) {
	publish := g.importProgress()
	if publish == nil {
		return
	}
	publish(libsync.Progress{
		InstanceID: instanceID,
		Phase:      importPhaseStopped,
		ItemsRead:  rep.ItemsRead,
		Applied:    rep.ItemsApplied,
	})
}

// importProgress is the one line that puts §7.2's "progress over SSE with real
// counts" on the existing stream.
//
// It is cheap — a callback, one Publish per committed batch — which is the
// condition under which this was worth doing at all. If the hub is not wired
// (the registry is built before the server in buildApp) it returns nil and the
// importer publishes nothing, rather than the import failing over telemetry.
func (g *registry) importProgress() libsync.ProgressFunc {
	hub := g.hub
	if hub == nil {
		return nil
	}
	return func(p libsync.Progress) {
		// UserID is the owner: an import is not a user's action, and 0 on this
		// hub means the system sentinel, not "everyone".
		hub.Publish(store.SystemUserID, httpapi.EventImportProgress, p)
	}
}

// bootstrapImport is the on-connect trigger.
//
// IT RUNS AT MOST ONCE PER INSTANCE PER DATABASE, gated on last_full_sync_at
// being unset — which is a column written on SUCCESS ONLY, so a failed or
// partial import leaves the gate open and the next connect retries. A restart
// after a successful import does not re-run it.
//
// The goroutine is deliberate and so is its context: it must not block the
// caller (entry() is on the probe path and, through it, on a request path), and
// it must not inherit a request's or a probe's deadline — a 20-second probe
// timeout would kill a five-minute import halfway through. It takes the
// process-lifetime context the prober was started with.
func (g *registry) bootstrapImport(ctx context.Context, instanceID int64) {
	at, err := g.st.LastFullSyncAt(ctx, instanceID)
	if err != nil {
		g.log.Warn("cannot tell whether this service has ever been imported; skipping the bootstrap import",
			"instance_id", instanceID, "err", err)
		return
	}
	if at.Valid {
		return
	}
	g.log.Info("first connect to this catalogue source; starting a full import",
		"instance_id", instanceID)
	rep, err := g.FullImport(ctx, instanceID)
	switch {
	case errors.Is(err, httpapi.ErrImportInProgress):
		// Not a failure and not worth a warning: something else — a hand-pressed
		// "Run full sync now" that arrived first — is already doing exactly this
		// work. It can happen on a never-imported instance, because StartImport
		// builds the client stack and building it is what schedules this.
		g.log.Info("an import for this service is already running; the bootstrap stands down",
			"instance_id", instanceID)
		return
	case err != nil:
		// SOFT. A failed bootstrap must not take the service down: the rows that
		// committed stand, last_full_sync_at is unwritten, and the next connect
		// tries again. The Services screen's own health is the prober's answer,
		// not this one's.
		g.log.Warn("the first full import did not finish; the rows it wrote stand and it will be retried on the next connect",
			"instance_id", instanceID,
			"items_read", rep.ItemsRead, "items_applied", rep.ItemsApplied, "err", err)
		return
	}
	g.log.Info("first full import complete",
		"instance_id", instanceID,
		"libraries_created", rep.LibrariesCreated,
		"libraries_joined", rep.LibrariesJoined,
		"declined_containers", len(rep.DeclinedContainers),
		"items", rep.ItemsApplied,
		"unidentified", rep.Unidentified)
}

// ── the concurrency guard, and why it is this one ───────────────────────────
//
// beginImport claims the right to import one instance and reports whether it
// got it. endImport releases it, and every acquisition is paired with a defer.
//
// AN IN-PROCESS MAP, chosen over the two alternatives on the table:
//
//   - A `state` column on service_instance would survive the process, which
//     sounds like the stronger guarantee and is the weaker one: a crash or a
//     SIGKILL mid-import leaves it reading "running" with nothing running, and
//     the instance can never be imported again without a hand-edited database.
//     A lock whose holder cannot die is a lock that needs a lease, a clock and
//     a reaper, which is three subsystems for a single-binary self-hosted app.
//   - sync_report is an append-only journal (00005_library_sync.sql), not a
//     mutual-exclusion primitive. Reading "did the last row say started" is a
//     TOCTOU race by construction.
//
// The map is exactly as durable as the thing it guards: an import only exists
// inside this process, so a process that is gone is holding no import. UsArr is
// one binary against one SQLite file (ARCHITECTURE.md §2), so there is no second
// process to coordinate with — and if there ever were, the single-writer
// connection is the boundary that would need the lease, not this map.
//
// It is a SEPARATE mutex from g.mu on purpose. g.mu guards the client registry
// and is taken on the probe and request paths; holding it for the minutes an
// import runs would stall both.
func (g *registry) beginImport(instanceID int64) bool {
	g.importMu.Lock()
	defer g.importMu.Unlock()
	if g.importing[instanceID] {
		return false
	}
	g.importing[instanceID] = true
	return true
}

func (g *registry) endImport(instanceID int64) {
	g.importMu.Lock()
	defer g.importMu.Unlock()
	delete(g.importing, instanceID)
}

// ── httpapi.CatalogueImports ────────────────────────────────────────────────

// StartImport is the Services screen's "Run full sync now" (ARCHITECTURE.md
// §17.3), and it is the on-demand trigger named in this file's header.
//
// IT DOES NOT BLOCK, and that is principle 1 rather than a convenience: the
// handler behind it is a user-facing request and an import is minutes. It
// claims the guard SYNCHRONOUSLY — so the answer "already running" is decided
// before this function returns, not by a goroutine that has not been scheduled
// yet — and then hands the claim to the goroutine that does the work. Returning
// nil means an import is now running; a caller learns it finished from the
// terminal import.progress frame on the SSE stream — phase "done" for a run
// that completed, phase "stopped" for one that did not (http-api.md §5.5) — or
// by asking again and being told it may start. Neither frame is authoritative:
// it can be dropped or missed, so last_full_sync_at stays the evidence (§4.3).
//
// The context is deliberately NOT the caller's. Same reason bootstrapImport's
// is not: a request deadline measured in seconds would kill a five-minute
// import halfway through.
func (g *registry) StartImport(instanceID int64) error {
	entry, err := g.entry(context.Background(), instanceID)
	if err != nil {
		return err
	}
	// The kind check is repeated from FullImport rather than delegated to it,
	// because delegating means the caller only learns "this is a Prowlarr"
	// after a goroutine has already been launched and the HTTP response has
	// already said "started". It asks catalogueSource the same question
	// runImport will, so the two cannot answer differently.
	if _, err := catalogueSource(entry, g.log); err != nil {
		return fmt.Errorf("%w: %q has kind %q (ADR-0052, ADR-0041)",
			httpapi.ErrNotCatalogueSource, entry.instance.Name, entry.instance.Kind)
	}
	if g.importCtx == nil {
		return errImportsNotArmed
	}
	if !g.beginImport(instanceID) {
		return fmt.Errorf("%w for service instance %d", httpapi.ErrImportInProgress, instanceID)
	}

	ctx := g.importCtx
	go func() {
		// The claim is already held; releasing it is this goroutine's job, and
		// FullImport must not try to take it a second time.
		defer g.endImport(instanceID)
		rep, err := g.fullImportLocked(ctx, instanceID)
		if err != nil {
			g.log.Warn("the requested full import did not finish; the rows it wrote stand",
				"instance_id", instanceID,
				"items_read", rep.ItemsRead, "items_applied", rep.ItemsApplied, "err", err)
			return
		}
		g.log.Info("the requested full import is complete",
			"instance_id", instanceID,
			"items", rep.ItemsApplied, "unidentified", rep.Unidentified)
	}()
	return nil
}

// errImportsNotArmed is the honest answer from a build or a test that never
// called enableBootstrapImport: there is no process-lifetime context to run an
// import under, so saying "started" would be a lie.
var errImportsNotArmed = errors.New("this process has no import context; catalogue imports are not armed")
