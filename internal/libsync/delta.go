package libsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jdb3750/UsArr/internal/store"
)

// DeltaSource is the OPTIONAL delta half of Source.
//
// It is a separate interface rather than three methods on Source for exactly
// CreditSource's reason, and that reason is worth restating rather than citing:
// not every catalogue source can serve a delta, and a Source that had to
// implement a no-op StreamItemsSince would be a Source whose author had to
// decide what a no-op means. The Kavita adapter is the live example — it is
// sunset but green, and its own 3b would be a CLIENT-SIDE STOP over a sorted
// list rather than a server-side filter, because Kavita's SeriesFilterField
// carries no timestamp member at all. Forcing it to implement a filtered
// `Since` method makes it implement a lie.
//
// The importer type-asserts for it and REFUSES the whole channel when a source
// does not implement it. A source that cannot do a delta simply does not
// implement this, and that absence is the whole capability answer — there is no
// Capabilities() returning booleans, because that moves the answer out of the
// type system, where the assertion checks it at compile time, into a runtime
// value that can disagree with the methods that exist.
type DeltaSource interface {
	// StreamItemsSince hands fn every item the source reports as having arrived
	// strictly after the seed, and returns how many it handed over plus what it
	// observed about its own walk.
	//
	// The seed is OPAQUE TO THE IMPORTER: it comes out of the store and goes
	// into the adapter without anything in between reading it. The adapter that
	// minted the format is the only code that parses it, subtracts a lookbehind
	// from it and formats it back, because that adapter is the only code that
	// knows whose clock it is in.
	//
	// An INVALID seed is the bootstrap case and is a legal input, not an error;
	// MeasuredEmpty is what decides whether the importer may pass one.
	StreamItemsSince(ctx context.Context, seed store.ArrivalsWatermark,
		fn func(store.CatalogueItem) error) (int, DeltaObservation, error)

	// ArrivalsMax is the maximum arrivals-axis value this source observed on the
	// walk it just ran — FULL IMPORT OR DELTA ALIKE.
	//
	// It is the accessor a full import reads to mint the FIRST watermark, on the
	// same precedent as BookOrbitSource.Skipped / Completeness / Comics: a fact
	// about the run that has to outlive the run, read off the adapter after the
	// stream closes rather than threaded through store.CatalogueItem.
	//
	// ⚠️ AN INVALID RETURN IS FAIL-CLOSED AND NOT AN ERROR. It means "no maximum
	// was observed, or the maximum is not known to be one", and the mint is
	// DECLINED rather than defaulted.
	ArrivalsMax() store.ArrivalsWatermark

	// MeasuredEmpty answers the one question that separates the two ways a
	// watermark can be absent on an instance that HAS been fully imported: was
	// the catalogue measured empty, or did something else produce a zero?
	//
	// It returns false plus UsArr's own one-clause reason when the source cannot
	// state, on an INDEPENDENT measurement, that every container it can see
	// holds no items. See deltaPreconditions for why the question is put this
	// way round.
	MeasuredEmpty() (bool, string)
}

// DeltaObservation is what a delta walk noticed about its own progress.
//
// ⚠️ WHAT IS DELIBERATELY NOT ON IT: anything about container binding. The
// newly-bound-container escalation is decided BEFORE the walk runs and is
// produced by the bind step, so it rides on store.CatalogueBinding where the
// bind already reports its outcome. Putting it here would make the importer ask
// the walk about something the walk never saw.
type DeltaObservation struct {
	// MaxObserved is the maximum arrivals value actually SEEN, folded across
	// every library. It is fed from the walk's own fold and NEVER from a cursor
	// variable — under the tie mitigation the cursor deliberately lags this by
	// one tie group, so a cursor stored here would be permanently short of the
	// truth in a way nobody could see.
	MaxObserved store.ArrivalsWatermark

	// MinDelivered is the minimum arrivals value over rows delivered this run.
	// It exists to MEASURE THE LOOKBEHIND rather than to drive anything: with
	// the watermark the run started from, it is a skew-free pair of upstream
	// values, so `min_delivered < watermark_before` with works_created > 0 is an
	// empirical lower bound on the window actually required, and never catching
	// anything is evidence the window can be cut. It can only be collected while
	// the lookbehind exists.
	MinDelivered store.ArrivalsWatermark

	// Wedged reports that a single instant filled a whole page, so the cursor
	// could not advance. It is not data loss; it is an outage above that instant
	// until it clears, and the repair is a full import.
	Wedged bool

	// WedgeAt is that instant, in UsArr's own rendering of an upstream value.
	WedgeAt string

	// UnreadableStamp counts rows whose arrival timestamp did not parse. The
	// rows are still delivered and applied; a non-zero count DECLINES THE
	// ADVANCE, because a maximum computed over a set with unreadable members is
	// not known to be a maximum.
	UnreadableStamp int

	// LibrariesWalked and LibrariesTotal are what the all-or-nothing rule reads.
	LibrariesWalked int
	LibrariesTotal  int

	// Pages is how many HTTP requests the walk made across every library.
	Pages int

	// WindowItemsSkipped and WindowComicResidue are WINDOW-SCOPED tallies, and
	// their names say so.
	//
	// 🚩 THEY EXIST HERE PRECISELY SO THEY CANNOT REACH THE LIBRARIES SCREEN.
	// The per-container skip vocabulary is a claim about a CONTAINER — "an
	// import walked this library's containers and recorded nothing left out" —
	// and the screen's read is latest-wins. A delta's five-minute window that
	// skipped nothing would write a zero-count row that overwrites a full
	// import's "300 comics left out", and the screen would then read "nothing
	// left out" while the 300 are still not imported. So the delta path writes
	// no items_skipped and no comic-residue row at all, and these two numbers
	// ride in the delta_walk report where nothing on that screen reads them.
	WindowItemsSkipped int
	WindowComicResidue int
}

// SyncReportDeltaWalk is sync_report.kind for one delta run — ONE ROW PER RUN
// PER INSTANCE, success and failure alike.
//
// IT IS A CONSTANT for SyncReportFileWalkFailed's reason: sync_report.kind
// carries no CHECK, so a typo is not a constraint violation, it is a count that
// silently reads zero.
//
// THE COLLISION CHECK IS DERIVED, NOT COPIED. cmd/usarr's
// TestSyncReportKindsDoNotCollide (syncreportvocab_test.go) reads every
// `syncReport…`/`SyncReport…` string constant in non-test Go out of the tree and
// asserts the members are pairwise
// distinct, so this kind joins the check by existing. No enumeration is stated
// here, in either direction. The WIRE word for the same concept is `page-walk
// delta`; the two are one concept in two vocabularies and nothing translates
// between them by string manipulation.
//
// ⚠️ THIS COMMENT CARRIED A HAND-COPIED ENUMERATION AND IT WAS WRONG TWICE, WHICH
// IS WHY IT IS GONE RATHER THAN EXTENDED. It first read `comic_residue`, WHICH
// IS NOT A MEMBER AND NEVER WAS — nothing in the tree writes that string as a
// sync_report.kind; the two literals the comic path actually writes are
// cmd/usarr/import.go's syncReportComicSeriesSynthesized and
// syncReportComicSeriesDeclined, and the only other occurrence of the substring
// is `window_comic_residue`, a PAYLOAD KEY on the delta row below — a different
// vocabulary at a different grain. Corrected 2026-08-21 with ADR-0074 to a list
// ending *"comic_series_memberships_declined, id_reused and deletion_sweep. No
// collision."* — and the same slice then added
// store.SyncReportDeletionSweepRefused and store.SyncReportRevivedWithoutIdentity
// without returning here, so the corrected list was incomplete before the branch
// it landed on was finished. A VOCABULARY CHECK THAT NAMES A MEMBER THE TREE
// DOES NOT HAVE IS A CHECK THAT DID NOT RUN, and one that omits a member the
// tree does have is the same defect in the other direction: its whole purpose is
// to prove a new kind collides with nothing, and that proof is void if the
// enumeration is not the enumeration. A list maintained by a different act than
// the thing it lists drifts (DEVELOPMENT.md §11); the derived check cannot.
// internal/store/reconcile.go's SyncReportDeletionSweep took this same treatment
// in the same slice.
const SyncReportDeltaWalk = "delta_walk"

// The reasons a completed delta declines to advance the watermark. Each one
// leaves the column BYTE-FOR-BYTE unchanged and each one is recorded.
const (
	// declineNoRows is the MODAL OUTCOME of an arrivals channel and is neither
	// an error nor a failure.
	declineNoRows = "no_rows"

	// declineLibraryFailed is the all-or-nothing rule: one library short means
	// the instance's watermark does not move at all.
	declineLibraryFailed = "library_failed"

	// declineTieWedge is a page of rows sharing one instant.
	declineTieWedge = "tie_wedge"

	// declineUnreadableStamp is a delivered row whose arrival stamp did not
	// parse, so the maximum is not known to be a maximum.
	declineUnreadableStamp = "unreadable_stamp"
)

// ErrNoDeltaChannel is what a delta asked of a source that has none refuses
// with.
//
// It is a REFUSAL WITH A REASON rather than silence, and the difference is the
// trigger: not implementing an optional half is not a fault on an automatic
// path, but a person who presses a button and is told nothing has been lied to.
var ErrNoDeltaChannel = errors.New("libsync: this catalogue source has no delta channel; a full sync is the only read it offers")

// ErrEscalateToFullImport is what a delta returns when the instance is not in a
// state where an arrivals filter is meaningful.
//
// ⚠️ IT IS A SENTINEL SO THE CALLER MUST HANDLE IT, AND THE CALLER MUST
// ESCALATE ON THE PATH THAT ALREADY HOLDS THE MUTUAL-EXCLUSION GUARD. A delta
// that has claimed the guard and then calls the guard-claiming full import gets
// "an import is already in progress", so NEITHER runs and the user is told a
// sync is running that is not. The wrong call compiles and fails only on a
// new-container run.
var ErrEscalateToFullImport = errors.New("libsync: this instance needs a full import, not a delta")

// DeltaReport is a delta run's Report plus the facts only a delta has.
type DeltaReport struct {
	Report

	// WatermarkBefore is the stored cursor position at run start, and
	// WatermarkAfter is what it holds at run end — equal to Before whenever the
	// advance was declined.
	WatermarkBefore store.ArrivalsWatermark
	WatermarkAfter  store.ArrivalsWatermark

	// Advanced reports whether the watermark moved.
	Advanced bool

	// DeclineReason is one of the decline* constants, or empty when the
	// watermark advanced.
	DeclineReason string

	// Escalated reports that no walk ran because the instance needed a full
	// import; EscalationReason is UsArr's own words for why.
	Escalated        bool
	EscalationReason string

	// Observation is the walk's own account of itself.
	Observation DeltaObservation
}

// DeltaSync is channel 3b: read one catalogue source's ARRIVALS and apply them
// through the same pipeline a full import uses.
//
// # It is FullImport with five differences and no sixth
//
//	(a) a FILTERED item stream, seeded from the stored cursor position;
//	(b) NO ANALYZE — reference/sync.md §6 rule 5 asks for it after a BULK
//	    import, and this is not one;
//	(c) NO StampFullSync — a delta must never touch the full-sync freshness
//	    claim, because a delta that ran does not mean every row was re-read;
//	(d) Progress nil, so the SSE stream publishes NOTHING (see below);
//	(e) a terminal watermark write that is declined more often than it is taken.
//
// Phases B (credits), C (files), the rollup flush and D (covers) are REUSED
// UNCHANGED. That is not batching: an arrival that went through only the item
// phase would land as a work row with no credits, no media_file, no edition and
// no rollup, and work.have_count would render "none held" when the truth is "not
// counted".
//
// # ⚠️ Progress MUST BE NIL, and it is not a stylistic choice
//
// The importer publishes phase frames and cmd/usarr forwards them as
// `import.progress`; the browser pins CLIENT_PHASES against the phases the
// importer publishes. So a delta reusing this body either publishes the same
// phases — and the Services screen shows an IMPORT RUNNING on every poll, with a
// stopped frame on failure — or publishes a new phase and fails that pin.
// publish() is `if im.Progress != nil`, and cmd/usarr's own progress function
// already returns nil when there is no hub, so nil is a first-class answer the
// pipeline handles rather than a special case.
//
// # What it costs per run, because a delta that hides its cost is not a delta
//
// Two reads are per-INSTANCE rather than per-arrival and neither is on a render
// path: the container bind runs the completeness probe, which is ONE
// GET /libraries/:id/stats PER LIBRARY, and the cover pass reads every postered
// link row for the whole instance to answer a question about a handful of
// arrivals. Both are background writes; principle 1 is about render paths. They
// are named because a delta's cost has to be stated, and narrowing the second is
// what the timer would owe if a timer ever shipped.
func (im *Importer) DeltaSync(ctx context.Context, instanceID int64) (rep DeltaReport, err error) {
	rep.Report = Report{InstanceID: instanceID, StartedAt: im.now()}
	defer func() {
		if rep.FinishedAt.IsZero() {
			rep.FinishedAt = im.now()
		}
	}()

	src, ok := im.Source.(DeltaSource)
	if !ok {
		return rep, fmt.Errorf("delta sync of service_instance %d: %w", instanceID, ErrNoDeltaChannel)
	}

	// PRECONDITION 1, ANSWERED FROM SQLITE ALONE AND BEFORE ANY HTTP. There is
	// nothing to be a delta OF on an instance that has never completed a full
	// import, and finding that out should not cost a request.
	full, err := im.Store.LastFullSyncAt(ctx, instanceID)
	if err != nil {
		return rep, fmt.Errorf("delta sync of service_instance %d: %w", instanceID, err)
	}
	if !full.Valid {
		return im.escalate(ctx, instanceID, rep,
			"this service has never completed a full sync, so there is no catalogue for a delta to add to")
	}

	rep.WatermarkBefore, err = im.Store.ReadArrivalsWatermark(ctx, instanceID)
	if err != nil {
		return rep, fmt.Errorf("delta sync of service_instance %d: %w", instanceID, err)
	}
	rep.WatermarkAfter = rep.WatermarkBefore

	// THE CONTAINER REFS ARE DROPPED HERE ON PURPOSE. They exist for channel 4's
	// deletion pass, and a delta walk reports only what changed — reconciling
	// absences against it would tombstone everything it did not happen to see.
	bindings, _, err := im.bindPhase(ctx, instanceID, "delta sync", &rep.Report)
	if err != nil {
		return rep, err
	}

	if why, escalate := im.deltaPreconditions(src, bindings, rep.WatermarkBefore); escalate {
		return im.escalate(ctx, instanceID, rep, why)
	}

	seed := rep.WatermarkBefore
	var obs DeltaObservation
	// nil: no seen-set, for the reason streamAndApply's own doc gives.
	imported, walkErr := im.streamAndApply(ctx, instanceID, bindings, &rep.Report,
		func(ctx context.Context, fn func(store.CatalogueItem) error) (int, error) {
			var n int
			var err error
			n, obs, err = src.StreamItemsSince(ctx, seed, fn)
			return n, err
		}, nil)
	rep.Observation = obs
	if walkErr != nil {
		// EVERYTHING ALREADY APPLIED STANDS — FullImport's contract, and for its
		// reason: rolling back because element 41,000 was malformed would mean
		// nothing is ever imported from a flaky instance. What does NOT stand is
		// any claim of progress: the watermark is untouched, nothing records a
		// resume point, and the next run re-reads the same window from the same
		// unmoved watermark. Already-applied rows re-apply as no-ops.
		rep.DeclineReason = declineLibraryFailed
		if obs.Wedged {
			rep.DeclineReason = declineTieWedge
		}
		im.recordDeltaWalk(ctx, instanceID, &rep, walkErr)
		return rep, walkErr
	}

	if err := im.streamAndApplyCredits(ctx, instanceID, imported, &rep.Report); err != nil {
		rep.DeclineReason = declineLibraryFailed
		im.recordDeltaWalk(ctx, instanceID, &rep, err)
		return rep, err
	}
	if err := im.streamAndApplyFiles(ctx, instanceID, imported, &rep.Report); err != nil {
		rep.DeclineReason = declineLibraryFailed
		im.recordDeltaWalk(ctx, instanceID, &rep, err)
		return rep, err
	}

	rollups, err := im.Store.FlushRollups(ctx, store.OwnerScope(im.UserID), 0)
	rep.Rollups = rollups
	if err != nil {
		rep.DeclineReason = declineLibraryFailed
		wrapped := fmt.Errorf("delta sync of service_instance %d: flush rollups: %w", instanceID, err)
		im.recordDeltaWalk(ctx, instanceID, &rep, wrapped)
		return rep, wrapped
	}

	im.fetchCovers(ctx, instanceID, imported, &rep.Report)
	rep.Completed = true

	// THE TERMINAL WRITE, AT MOST ONCE, AFTER EVERY PHASE HAS SUCCEEDED FOR
	// EVERY LIBRARY.
	rep.DeclineReason = declineReasonFor(obs)
	if rep.DeclineReason == "" {
		if err := im.Store.StampArrivalsWatermark(ctx, instanceID, obs.MaxObserved); err != nil {
			wrapped := fmt.Errorf("delta sync of service_instance %d: %w", instanceID, err)
			im.recordDeltaWalk(ctx, instanceID, &rep, wrapped)
			return rep, wrapped
		}
		rep.WatermarkAfter = obs.MaxObserved
		rep.Advanced = true
	}

	im.recordDeltaWalk(ctx, instanceID, &rep, nil)
	rep.FinishedAt = im.now()
	im.log().Info("delta sync finished",
		"instance_id", instanceID,
		"items", rep.ItemsApplied,
		"works_created", rep.WorksCreated,
		"pages", obs.Pages,
		"libraries_walked", obs.LibrariesWalked,
		"libraries_total", obs.LibrariesTotal,
		"unreadable_stamps", obs.UnreadableStamp,
		"watermark_advanced", rep.Advanced,
		"advance_declined_reason", rep.DeclineReason,
		"duration", rep.Duration())
	return rep, nil
}

// declineReasonFor applies the declines-to-advance rules to a COMPLETED walk, in
// the order that reports the most specific cause.
//
// 🚩 THE no_rows ARM IS LOAD-BEARING AND ITS ABSENCE WAS A DATA-LOSS BUG. A
// completed run that observed nothing SUCCEEDS, so a rule that said only "write
// it on success" said WRITE IT — and writing an invalid watermark NULLS THE
// COLUMN. A BookOrbit that has simply been quiet would destroy its own cursor,
// intermittently, on exactly the quiet instances nobody is watching, because a
// run inside the lookbehind window usually does re-observe the window.
func declineReasonFor(obs DeltaObservation) string {
	switch {
	case obs.Wedged:
		return declineTieWedge
	case obs.UnreadableStamp > 0:
		return declineUnreadableStamp
	case obs.LibrariesWalked < obs.LibrariesTotal:
		return declineLibraryFailed
	case !obs.MaxObserved.Valid:
		return declineNoRows
	default:
		return ""
	}
}

// deltaPreconditions decides whether an arrivals filter is meaningful on this
// instance, given that a full import has completed at least once.
//
// # The four states, and v1 of this design saw two
//
//	last_full_sync_at | watermark | meaning                     | delta does
//	------------------|-----------|-----------------------------|------------
//	NULL              | any       | never fully imported        | escalate  (decided by the caller, before any HTTP)
//	set               | valid     | ordinary                    | run, seeded
//	set               | NULL      | zero observed, cause unclear| escalate
//	set               | NULL      | catalogue MEASURED empty    | run UNFILTERED
//
// # ⚠️ THE PERMISSION-VERSUS-EMPTY GUARD, and why it is not scoped to permission
//
// The last two rows are the SAME STORED STATE. A full import mints no watermark
// when it observed no arrivals-axis value at all, and "the catalogue held no
// books" is only ONE cause of that observation. This project's own record
// already names another, at the source: LibraryAccessGuard throws an IDENTICAL
// ForbiddenException for "the library exists and this account has no access row"
// and for "there is no such library" — FORBIDDEN IS INDISTINGUISHABLE FROM
// ABSENT — and separately a content filter lands in the books LEFT JOIN ON
// rather than in a WHERE, so it SHORTS a library's count without dropping the
// library. A credential change can therefore produce a zero-book observation on
// a catalogue that holds thousands.
//
// 🚩 SO THE GUARD DOES NOT ASK WHY THE ZERO HAPPENED. A guard scoped to one
// cause of an observation is blind to every other cause of the same observation:
// a "did permissions change?" test passes a filter change, a scanner bug, a
// route that moved and an upstream that started returning empty pages. It asks
// instead whether an INDEPENDENT, UNFILTERED measurement AGREES that there is
// nothing there — which is a question about the observation, not about its
// cause, and is therefore answered correctly for causes nobody has thought of.
//
// ⚠️ AND ITS SCOPE IS STATED RATHER THAN IMPLIED, because the same record that
// names the property also names its limit: the measurement covers containers the
// credential can SEE, and whether WHOLE LIBRARIES are hidden from UsArr's
// account is unanswerable read-only. So this guard closes the shortfall axis and
// not the membership axis. Escalating is the safe side of both: a full import is
// what re-reads every item in a container, and it is the only repair for a skip,
// a deletion, a wedge or a backdated row.
func (im *Importer) deltaPreconditions(
	src DeltaSource, bindings map[string]store.CatalogueBinding, w store.ArrivalsWatermark,
) (string, bool) {
	// 🚩 THE NEWLY-BOUND-CONTAINER ESCALATION, AND IT DOES NOT KEY ON `Created`.
	// A library newly granted to UsArr's credential is full of books whose
	// arrival stamps are far behind the watermark: an arrivals filter returns
	// NONE of them, the walk succeeds, and a whole library is missing with
	// nothing on screen saying so. `Created` reports whether the call created THE
	// LIBRARY rather than joining an existing one — a different fact, and false
	// in the ordinary homelab case where a BookOrbit library named `Ebooks` joins
	// the `Ebooks` library a Kavita already made. That path writes a brand-new
	// binding row and returns Created false, so a detector keyed on it misses
	// exactly the case this escalation exists to prevent.
	for _, b := range bindings {
		if b.SourceAttached {
			return "a container was bound to this service for the first time, and an arrivals " +
				"filter would return none of the books already in it", true
		}
	}

	if w.Valid {
		return "", false
	}

	if empty, why := src.MeasuredEmpty(); !empty {
		return "no arrivals cursor is stored and " + why, true
	}

	// MEASURED EMPTY. Request 1 carries no filter and the walk keysets normally
	// from there.
	//
	// Its worst case is honest and bounded: if the instance filled up since, the
	// delta walks it all once at full-import-ish cost, and every phase runs
	// correctly because a keyset walk of the whole set IS a correct walk. The
	// alternative — escalating on an empty instance — costs the same read AND
	// re-runs ANALYZE AND re-stamps a freshness claim that did not change.
	return "", false
}

// escalate records the run that declined and hands the caller the sentinel.
func (im *Importer) escalate(
	ctx context.Context, instanceID int64, rep DeltaReport, why string,
) (DeltaReport, error) {
	rep.Escalated = true
	rep.EscalationReason = why
	im.recordDeltaWalk(ctx, instanceID, &rep, nil)
	im.log().Info("delta sync escalated to a full import",
		"instance_id", instanceID, "reason", why)
	return rep, fmt.Errorf("delta sync of service_instance %d: %s: %w", instanceID, why, ErrEscalateToFullImport)
}

// recordDeltaWalk writes the one durable record of a delta run.
//
// WRITTEN WHETHER THE RUN SUCCEEDED OR FAILED, on the precedent that already
// governs the skip, completeness and residue records: a Report the caller forgets
// to read and a log line nobody greps are the same thing as silence.
//
// A FAILURE TO RECORD DOES NOT FAIL THE RUN. It is logged and dropped, because
// the rows the run applied are correct either way and turning a reporting write
// into an outage is the trade this codebase refuses everywhere else.
//
// 🚩 error_class IS UsArr's OWN WORD AND NEVER err.Error(). A walk error is a
// *bookorbit.APIError, whose Error() renders the upstream's own Message —
// filled from the response body, which for a rejected key is one complaint per
// key. Storing it would put BookOrbit's prose into UsArr's record, into a column
// reference/security.md keeps free of upstream response bodies and which reaches
// a browser.
func (im *Importer) recordDeltaWalk(ctx context.Context, instanceID int64, rep *DeltaReport, runErr error) {
	obs := rep.Observation
	detail := map[string]any{
		"window_from":              seedDescription(rep.WatermarkBefore),
		"watermark_before":         watermarkValue(rep.WatermarkBefore),
		"watermark_after":          watermarkValue(rep.WatermarkAfter),
		"advanced":                 rep.Advanced,
		"advance_declined_reason":  rep.DeclineReason,
		"min_delivered_added_at":   watermarkValue(obs.MinDelivered),
		"works_created":            rep.WorksCreated,
		"pages_read":               obs.Pages,
		"rows_delivered":           rep.ItemsRead,
		"rows_applied":             rep.ItemsApplied,
		"libraries_walked":         obs.LibrariesWalked,
		"libraries_total":          obs.LibrariesTotal,
		"window_items_skipped":     obs.WindowItemsSkipped,
		"window_comic_residue":     obs.WindowComicResidue,
		"wedge_at":                 obs.WedgeAt,
		"escalated_to_full_import": rep.Escalated,
		"escalation_reason":        rep.EscalationReason,
		"error_class":              errorClass(runErr),
	}
	blob, err := json.Marshal(detail)
	if err != nil {
		im.log().Warn("delta sync: could not encode its own report",
			"instance_id", instanceID, "err", err)
		return
	}
	// remote_kind and remote_id are empty: this row is about the RUN, not about
	// one container. The Libraries screen's per-container reads all carry a
	// remote_kind predicate, so a row with none is invisible to them by
	// construction — which is the second half of why a delta cannot overwrite a
	// per-container verdict.
	if err := im.Store.RecordSyncReport(ctx, instanceID, SyncReportDeltaWalk, "", "", string(blob)); err != nil {
		im.log().Warn("delta sync: could not record its own report",
			"instance_id", instanceID, "err", err)
	}
}

// seedDescription says what request 1 asked for, in UsArr's own words.
func seedDescription(w store.ArrivalsWatermark) string {
	if !w.Valid {
		return "no filter: the catalogue was measured empty at the last full import"
	}
	return w.Value
}

func watermarkValue(w store.ArrivalsWatermark) string {
	if !w.Valid {
		return ""
	}
	return w.Value
}

// errorClass names a failure in UsArr's vocabulary. See recordDeltaWalk.
func errorClass(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrEscalateToFullImport):
		return "escalated_to_full_import"
	case errors.Is(err, ErrNoDeltaChannel):
		return "no_delta_channel"
	case errors.Is(err, context.DeadlineExceeded):
		return "walk_deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, store.ErrNoWatermarkToStamp):
		return "refused_to_null_the_cursor"
	default:
		return "walk_failed"
	}
}

// mintArrivalsWatermark is THE BOOTSTRAP: a successful full import stamps the
// first cursor position from its own observed maximum.
//
// ⚠️ WITHOUT THIS THE DELTA NEVER RUNS ONCE, AND IT LANDS IN THE SAME CHANGE AS
// THE WALK RATHER THAN AFTER IT. The rule, in the form worth remembering: A
// CHANGE THAT CREATES THE CONDITION ANOTHER FIX ADDRESSES MUST LAND WITH IT.
// A keyset makes the unbootstrapped state WORSE than offset paging did — under
// offset paging an implementer facing an absent cursor had a bad-but-total
// fallback, walk from offset 0 with no filter, whereas under a keyset REQUEST 1
// REQUIRES A FILTER VALUE. So the implementer is under pressure to supply one,
// and the two values nearest to hand are time.Now() and the run's own StartedAt
// — the latter being exactly what StampFullSync uses two lines away. That is the
// ONE place in this design where UsArr's clock could still move a row across the
// boundary permanently, and it lives in the branch a design that stops at the
// walk leaves unspecified, beside the code that models the wrong answer.
//
// ON SUCCESS ONLY, and it is a SEPARATE WRITE from StampFullSync rather than a
// second parameter to it. StampFullSync's contract is a FRESHNESS CLAIM and
// spends fourteen lines on why the stored instant is the run's START; widening
// it to also carry a CURSOR would put the two facts back inside one function.
// The non-atomicity is bounded and self-correcting: if this write fails the
// column stays as it was, which is a state the preconditions already handle, and
// the next full import mints it.
//
// ⚠️ A DECLINED MINT IS NOT AN ERROR. An invalid maximum means the source
// observed none, or observed one it cannot vouch for — an unreadable stamp is
// the live case — and defaulting it to anything would be a cursor position
// nothing observed.
func (im *Importer) mintArrivalsWatermark(ctx context.Context, instanceID int64) error {
	src, ok := im.Source.(DeltaSource)
	if !ok {
		return nil
	}
	max := src.ArrivalsMax()
	if !max.Valid {
		im.log().Info("full import minted no arrivals cursor",
			"instance_id", instanceID,
			"effect", "the next delta sync escalates to a full import unless the catalogue was measured empty")
		return nil
	}
	if err := im.Store.StampArrivalsWatermark(ctx, instanceID, max); err != nil {
		return fmt.Errorf("full import of service_instance %d: %w", instanceID, err)
	}
	return nil
}
