package store

// ITEMS LEFT OUT — "did UsArr map everything it read?"
//
// # The sibling axis, and why it is not the completeness one
//
// completeness.go answers *did UsArr's credential SEE everything the container
// holds*. This file answers the question one step later: of the items UsArr was
// shown, how many did it READ and deliberately not map. The two are independent
// — a library can be fully visible and still be short, because the adapter has
// no unit of work for part of it — and neither is evidence for the other.
//
// ⚠️ NEITHER IS A COMPLETENESS CLAIM ABOUT THE OTHER, AND THAT IS THE WHOLE
// REASON THIS FILE EXISTS RATHER THAN A FIELD ON THE COMPLETENESS ROW. A skip
// count says how many items were left out and why. It does NOT say the rest
// arrived, it does not say the container was fully visible, and a screen that
// let one clean number read as "this library is complete" would be committing
// the exact error ADR-0061 exists to prevent, on the axis next door.
//
// # Why the vocabulary lives HERE and not in cmd/usarr
//
// Three sides, none of which the compiler connects: internal/libsync COUNTS the
// skips, cmd/usarr WRITES them into sync_report.detail, and libraries.go READS
// them back for §17.8. sync_report.kind carries no CHECK (migration 00005) and
// `detail` is untyped JSON, so a typo on any one side is not a constraint
// violation — it is a skip that silently disappears, which is the defect the
// counting was added to close. completeness.go makes this argument at length for
// the same reason; this const used to be private to cmd/usarr, where the read
// side could not reach it.

// SyncReportItemsSkipped is sync_report.kind for items an adapter read and
// deliberately did not map.
//
// ⚠️ A ROW IS WRITTEN ONLY WHEN SOMETHING WAS SKIPPED — one per container per
// import, and none at all for a container that was walked clean. That is the
// OPPOSITE of SyncReportContentCompleteness's rule, it is deliberate, and
// ADR-0061 §5 records it: an absent skip row means nothing was skipped, an
// absent completeness row means nothing was ASKED.
//
// ⚠️ WHICH IS EXACTLY WHY THE READ CANNOT STOP AT THIS TABLE. "Nothing was
// skipped" and "no import ever counted" are both an absent row, so the read
// pairs the absence with the completeness row to tell them apart. See
// LibrarySkips.
//
// It is distinct from `container_declined`, a whole container UsArr has no kind
// for, and from `container_bind_failed`, a container that had a kind and lost a
// uniqueness race. This one is about ITEMS INSIDE a container that bound fine.
const SyncReportItemsSkipped = "items_skipped"

// SkipState is the verdict as it travels onto the wire.
//
// A string rather than an integer for CompletenessState's reason: it round-trips
// through JSON out to a browser, and a named vocabulary survives that in a way
// an ordinal does not.
type SkipState string

// The two states that are SAID. The third — nothing observed this library at all
// — is the absence of the whole verdict and has no member, exactly as
// completeness's "nothing was measured" is a nil pointer rather than a fourth
// CompletenessState.
const (
	// SkipsLeftOut is a recorded skip: an import read items in this library's
	// containers and mapped none of them.
	SkipsLeftOut SkipState = "left_out"

	// SkipsNone is the measured negative: an import observed this library's
	// containers and recorded nothing left out.
	//
	// ⚠️ IT IS NOT "THIS LIBRARY IS COMPLETE". It says one thing only — that the
	// adapter recorded no unmapped items — and it is silent about whether the
	// credential saw the whole container (completeness.go's axis) and about
	// whether the import that observed it ran to the end.
	SkipsNone SkipState = "none"
)

// SkipStateOf reads one state string, refusing an unknown one.
//
// AN UNRECOGNISED VALUE IS NOT COERCED, on CompletenessStateOf's reasoning: a
// state this build cannot read comes back ("", false) and the caller drops it,
// which renders as "nothing was observed" — the only reading that cannot
// overstate.
func SkipStateOf(v string) (SkipState, bool) {
	switch SkipState(v) {
	case SkipsLeftOut, SkipsNone:
		return SkipState(v), true
	default:
		return "", false
	}
}

// SkipNote is the JSON contract of sync_report.detail for a
// SyncReportItemsSkipped row.
//
// A STRUCT RATHER THAN AN AD-HOC MAP because it is a WIRE between packages that
// never call each other: cmd/usarr marshals it, this package unmarshals it, and
// nothing but this declaration connects the two. ⚠️ EVERY JSON TAG HERE MATCHES
// A KEY ALREADY WRITTEN INTO SHIPPED ROWS — `name`, `skipped_comics`,
// `skipped_unknown`, `reason`, `effect` — so rows written before this struct
// existed decode into it unchanged. Renaming one silently orphans that half of
// the history.
//
// ⚠️ EVERY STRING IN IT IS USARR'S OWN PROSE. reference/security.md §5 keeps
// upstream response bodies out of this column, and Reason reaches a browser from
// here.
type SkipNote struct {
	// Name is the upstream's own name for the container at import time, so the
	// row is readable by someone who has only sync_report in front of them.
	Name string `json:"name,omitempty"`

	// Comics and Unknown are the per-reason tallies.
	//
	// ⚠️ THEY ARE THE BOOKORBIT ADAPTER'S VOCABULARY AND THEY DO NOT TRAVEL TO
	// THE BROWSER. The wire carries the TOTAL plus Reason, which is UsArr's own
	// sentence, because a second adapter will skip for reasons that are not these
	// two and an API field named `comics` would have to be lied to. The split is
	// kept in the row for the operator reading it out of the database.
	Comics  int64 `json:"skipped_comics"`
	Unknown int64 `json:"skipped_unknown"`

	// Reason is why, in UsArr's words, SHORT ENOUGH FOR A TABLE CELL. §9.1's
	// overflow policy is a wrap and a three-line cell is a row nobody scans —
	// the same constraint completenessReason is written against. The long form
	// belongs in Effect and in the process log.
	Reason string `json:"reason,omitempty"`

	// Effect is what the skip did to the catalogue. Operator-facing; it does not
	// travel to the browser.
	Effect string `json:"effect,omitempty"`

	// Covers and DoesNotCover are the SCOPE OF THE CLAIM, written into every row
	// rather than left to a doc comment nobody reading a database has —
	// CompletenessNote carries the same pair for the same reason (ADR-0061 §6).
	// The second one is the load-bearing half here: a skip count is not a
	// completeness verdict, and a row that carried a number and no scope invites
	// exactly that reading.
	Covers       string `json:"covers,omitempty"`
	DoesNotCover string `json:"does_not_cover,omitempty"`
}

// Total is everything this container's walk read and did not map.
func (n SkipNote) Total() int64 { return n.Comics + n.Unknown }
