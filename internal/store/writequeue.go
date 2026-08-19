package store

// write_queue.state's vocabulary lives here rather than in a CHECK constraint,
// and this file is the whole of it.
//
// WHY NOT A CHECK. ADR-0039 decision 1 dropped `write_queue.state`'s CHECK
// entirely rather than widening it, and migration 00005's column comment carries
// the same reasoning next to the SQL: the lifecycle vocabulary is still growing
// — it moved once before a single row shipped, when WQ-01 established that no
// existing state means "waiting for a human" — SQLite cannot ALTER a CHECK, and
// audit_log.result and 0003's provenance.acquisition_state are the shipped
// precedent for moving exactly this kind of vocabulary into Go. The cost is
// stated in the ADR and is real: SQLite will accept a misspelt state, and a bug
// that writes 'pendign' reaches disk.
//
// ⚠️ THIS FILE IS THE THING ADR-0039 HAS BEEN OWED SINCE 2026-08-17. That ADR's
// decision 1 first said the vocabulary "moves to Go and is documented and
// validated there", in the present tense, and it was false when written; the ADR
// carries a dated correction saying so, and its consequences bullet has always
// read "Until that code exists, the vocabulary is documented in 00005's header
// and nowhere else, which is a real gap and is why this bullet exists." This is
// that code. internal/store/images.go was written in the meantime and names this
// gap as the failure it refuses to repeat — ValidImageFormat shipped WITH its
// column for that reason. write_queue.state got the validator second.
//
// WHAT IT IS NOT. It does not validate `kind` (never had a CHECK, and the verb
// half is what a new sink extends first) or `fail_reason` (whose CHECK is KEPT,
// because that vocabulary is CLOSED — the terminal taxonomy rejected · unknown ·
// exhausted — and the column is DB-01's regression witness). Only `state` lost
// its constraint, so only `state` is owed a Go replacement.

// write_queue lifecycle states — the values write_queue.state may hold.
//
// The six are ADR-0039 decision 1's list and migration 00005's column comment,
// which are the same list. Adding a member here is the ENTIRE schema-side cost
// of a new state, which is the property the absent CHECK buys.
const (
	// WriteQueueStatePending is the column's DEFAULT and the state a command
	// is queued in. A worker claims it.
	WriteQueueStatePending = "pending"

	// WriteQueueStateInflight asserts an outstanding upstream request.
	WriteQueueStateInflight = "inflight"

	// WriteQueueStateVerifying carries the 15-minute verify_until TTL.
	WriteQueueStateVerifying = "verifying"

	// WriteQueueStateAwaitingChoice is the state a two-phase sink parks in
	// while a PERSON decides (FUTURE.md §11). It is the member the CHECK was
	// going to be widened for, and the reason it was dropped instead.
	//
	// ⚠️ IT IS DELIBERATELY EXCLUDED FROM ix_wq_runnable's PARTIAL PREDICATE
	// (ADR-0039 decision 2), which stays byte-identical to 00001's
	// `WHERE state IN ('pending','inflight','verifying')`. A row waiting on a
	// person is not runnable: listing it would let a worker claim it and expose
	// it to the verify_until TTL, which is WQ-01's defect reintroduced through
	// the index. Being a LEGAL value and being a RUNNABLE one are different
	// questions, and this function answers only the first.
	//
	// Nothing produces it in v0.1 — the seam ships, the feature does not.
	WriteQueueStateAwaitingChoice = "awaiting_choice"

	// WriteQueueStateDone is terminal and successful.
	WriteQueueStateDone = "done"

	// WriteQueueStateFailed is terminal and unsuccessful; fail_reason then
	// carries the closed terminal taxonomy, enforced by its own CHECK.
	WriteQueueStateFailed = "failed"
)

// writeQueueStates is every legal value of write_queue.state.
var writeQueueStates = map[string]bool{
	WriteQueueStatePending:        true,
	WriteQueueStateInflight:       true,
	WriteQueueStateVerifying:      true,
	WriteQueueStateAwaitingChoice: true,
	WriteQueueStateDone:           true,
	WriteQueueStateFailed:         true,
}

// ValidWriteQueueState reports whether s is a legal write_queue.state value.
//
// It is deliberately strict about the empty string and about case. The column is
// NOT NULL with a DEFAULT of 'pending', so there is no "unset" spelling: a
// caller holding "" has a bug rather than a pending row, and must either omit
// the column and take the default or write WriteQueueStatePending. Nothing in
// the schema or the reconciliation guard folds case — sync.md §4 names the three
// runnable states as lowercase literals and ix_wq_runnable's predicate matches
// them byte-for-byte — so "Pending" is a value that would insert cleanly and
// then never be runnable, which is the silent stall this exists to prevent.
func ValidWriteQueueState(s string) bool {
	return writeQueueStates[s]
}
