package store

// CATALOGUE COMPLETENESS — "did UsArr's credential see everything the upstream
// container holds?"
//
// # Why the vocabulary lives HERE and not in the adapter that computes it
//
// It has three sides, not two. internal/libsync/bookorbitcompleteness.go
// MEASURES it, cmd/usarr WRITES it into sync_report.detail, and libraries.go
// READS it back and hands it to the Libraries screen. sync_report.kind carries
// no CHECK (migration 00005 says why) and `detail` is an untyped JSON column, so
// a typo on any one of those three sides is not a constraint violation: it is a
// completeness verdict that silently disappears. SyncReportFileWalkFailed is
// already a constant for exactly that reason with only two sides; this one has
// three.
//
// # The state that makes the rest of it honest
//
// ⚠️ "NO SHORTFALL" AND "NOT CHECKED" ARE DIFFERENT VALUES AND MUST STAY THAT
// WAY. The check rests on an upstream route being unguarded — BookOrbit's
// `GET /libraries/:id/stats` carries @RequireLibraryAccess('viewer') and no
// @RequirePermission at 73b7877d — which is a property of somebody else's
// service and not a promise to UsArr. If it is guarded later every probe answers
// 403, and a two-state design would report every library as complete on the day
// it stopped being able to tell. So CompletenessUnverified is a first-class
// answer, and no consumer may collapse it into either of the other two.
//
// # What this does NOT measure
//
// ⚠️ IT IS A BOOK-LEVEL CHECK OVER CONTAINERS THE CREDENTIAL CAN ALREADY SEE.
// Whether WHOLE containers are hidden from UsArr's account is a second axis and
// is unanswerable read-only: BookOrbit's LibraryAccessGuard throws an identical
// ForbiddenException for "exists, no access row" and for "no such library". A
// `complete` verdict therefore says every book in THIS container arrived; it
// says nothing whatever about containers that never appeared.

// SyncReportContentCompleteness is sync_report.kind for one container's
// completeness verdict.
//
// ONE ROW PER CONTAINER PER IMPORT, INCLUDING THE CONTAINERS THAT WERE FINE. An
// absent completeness row means nothing was ASKED — not that there was nothing
// to find — and the cost of saying so is one small append-only row per container
// per run.
//
// ⚠️ IT USED TO BE THE DIFFERENCE FROM `items_skipped`, AND IS NOT ANY MORE.
// That kind was written only where something had been skipped, so the two
// absences meant opposite things one column apart on the same screen; ADR-0063
// gave `items_skipped` this rule too. Same convention, both readers, no
// cross-reference needed to tell silence from a measured zero.
const SyncReportContentCompleteness = "content_completeness"

// CompletenessState is the three-valued verdict, as it travels through
// sync_report.detail and onto the wire.
//
// It is a string rather than an integer because it round-trips through JSON in a
// database column and out to a browser, and a named vocabulary survives that in
// a way an ordinal does not.
type CompletenessState string

// The three members. THERE IS NO ZERO-VALUE MEMBER: an empty CompletenessState
// is a broken row, never "complete", and CompletenessStateOf refuses anything it
// does not recognise rather than defaulting it.
const (
	// CompletenessComplete is measured agreement: the container's unfiltered
	// count and what UsArr's credential was shown are the same number.
	CompletenessComplete CompletenessState = "complete"

	// CompletenessShortfall is measured disagreement: the container holds more
	// than this credential is shown, which upstream is a content filter on the
	// account UsArr uses — something the owner can change.
	CompletenessShortfall CompletenessState = "shortfall"

	// CompletenessUnverified is the ABSENCE of a measurement and is a real
	// answer rather than an error. It must never render as complete.
	CompletenessUnverified CompletenessState = "unverified"
)

// CompletenessStateOf reads one stored state string, refusing an unknown one.
//
// AN UNRECOGNISED VALUE IS NOT COERCED. A row written by a future adapter with a
// fourth member, or a corrupted blob, comes back as ("", false) and the caller
// drops the row — which renders as "nothing was measured", the only reading that
// cannot overstate. Mapping it to the nearest member would be a guess published
// as a measurement.
func CompletenessStateOf(v string) (CompletenessState, bool) {
	switch CompletenessState(v) {
	case CompletenessComplete, CompletenessShortfall, CompletenessUnverified:
		return CompletenessState(v), true
	default:
		return "", false
	}
}

// CompletenessNote is the JSON contract of sync_report.detail for a
// SyncReportContentCompleteness row.
//
// It is a struct rather than an ad-hoc map because it is a WIRE between
// packages that never call each other: cmd/usarr marshals it, this package
// unmarshals it, and nothing in the compiler connects the two but this
// declaration.
//
// ⚠️ EVERY STRING IN IT IS USARR'S OWN PROSE. reference/security.md §5 keeps
// upstream response bodies out of this column, and the note reaches a browser
// from here, so `Reason` carries UsArr's sentence about what could not be
// measured and never the upstream's error text — which stays in the process log
// where an operator can read it and a browser cannot.
type CompletenessNote struct {
	// State is one of the three members above.
	State string `json:"state"`

	// Container is the upstream's own name for the container, as it was at
	// import time. It is here so the row is readable in the database by a person
	// who has only sync_report in front of them.
	Container string `json:"container,omitempty"`

	// Total is the container's own unfiltered count. ⚠️ -1, NEVER 0, when State
	// is unverified: 0 is a legal total for an empty container, so a sentinel
	// that collided with it would put "not measured" and "empty" in one value.
	Total int64 `json:"total_items"`

	// Visible is what UsArr's credential was shown, counted under the same
	// upstream predicate as Total. It is known in every state.
	Visible int64 `json:"visible_items"`

	// Hidden is Total - Visible, and is 0 in every state but shortfall. It is
	// stored rather than recomputed on read so the row says what it meant at the
	// time even if the subtraction rule is ever changed.
	Hidden int64 `json:"hidden_items"`

	// Reason is why State is unverified, in UsArr's words. Empty otherwise.
	Reason string `json:"reason,omitempty"`

	// Covers and DoesNotCover are the SCOPE OF THE CLAIM, written into every row
	// rather than left to a doc comment nobody reading a database has. The
	// second one is the load-bearing half: a clean book-level check must not be
	// read as a statement that no whole container is hidden.
	Covers       string `json:"covers,omitempty"`
	DoesNotCover string `json:"does_not_cover,omitempty"`
}
