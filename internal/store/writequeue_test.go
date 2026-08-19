package store

import (
	"context"
	"database/sql"
	"testing"
)

// TestValidWriteQueueStateRejects is the drill, and it leads because the
// canonical-value test below cannot fail for the reason that matters.
//
// A validator that returns true for everything passes every "is 'pending'
// legal?" assertion ever written. What separates a validator from a function
// named like one is the values it REFUSES, so those are asserted first and each
// case names the mistake it stands for.
//
// ⚠️ DRILLED, not assumed: replacing ValidWriteQueueState's body with
// `return true` turns THIS test red on all nine cases, and turns nothing else in
// the package red. That is the check that this file is the guard and not the
// decoration.
func TestValidWriteQueueStateRejects(t *testing.T) {
	for _, tc := range []struct {
		state string
		why   string
	}{
		{"pendign", "ADR-0039's own worked example of what the dropped CHECK now lets reach disk"},
		{"", "the column is NOT NULL with a DEFAULT; there is no empty spelling of 'unset'"},
		{"Pending", "case is not folded anywhere — this inserts cleanly and is then never runnable"},
		{"PENDING", "same, shouted"},
		{"pending ", "a trailing space survives SQLite and defeats ix_wq_runnable's IN list"},
		{" pending", "and a leading one"},
		{"awaiting choice", "the member is awaiting_choice; a space is not an underscore"},
		{"queued", "a plausible synonym that is not in the vocabulary"},
		{"rejected", "a fail_reason value, which is a DIFFERENT closed vocabulary on a " +
			"different column and still carries its own CHECK"},
	} {
		t.Run(tc.state, func(t *testing.T) {
			if ValidWriteQueueState(tc.state) {
				t.Errorf("ValidWriteQueueState(%q) = true, want false — %s.\n"+
					"write_queue.state carries NO CHECK constraint by design (ADR-0039 "+
					"decision 1), so this function is the only thing standing between a "+
					"misspelt state and the column.", tc.state, tc.why)
			}
		})
	}
}

// TestValidWriteQueueStateAcceptsTheVocabulary is the other half, and it is
// written against the LITERALS rather than against writeQueueStates, so that a
// constant whose value drifts from the string the schema and sync.md use is
// caught here rather than discovered as a row that never becomes runnable.
func TestValidWriteQueueStateAcceptsTheVocabulary(t *testing.T) {
	// ADR-0039 decision 1's list, and migration 00005's column comment, spelt
	// out. Not derived from the constants — deriving it would make this test
	// agree with any typo.
	for _, s := range []string{
		"pending", "inflight", "verifying", "awaiting_choice", "done", "failed",
	} {
		if !ValidWriteQueueState(s) {
			t.Errorf("ValidWriteQueueState(%q) = false, but it is one of the six states "+
				"ADR-0039 decision 1 and 00005's column comment both name", s)
		}
	}
	for name, c := range map[string]string{
		"WriteQueueStatePending":        WriteQueueStatePending,
		"WriteQueueStateInflight":       WriteQueueStateInflight,
		"WriteQueueStateVerifying":      WriteQueueStateVerifying,
		"WriteQueueStateAwaitingChoice": WriteQueueStateAwaitingChoice,
		"WriteQueueStateDone":           WriteQueueStateDone,
		"WriteQueueStateFailed":         WriteQueueStateFailed,
	} {
		if !ValidWriteQueueState(c) {
			t.Errorf("the exported constant %s holds %q, which this package's own "+
				"validator rejects", name, c)
		}
	}
	if got := len(writeQueueStates); got != 6 {
		t.Errorf("the vocabulary has %d members, want 6. A seventh is a real decision — "+
			"ADR-0039 decision 2 requires saying whether it belongs in ix_wq_runnable's "+
			"partial predicate, and 'awaiting_choice' is the standing example of one that "+
			"does not.", got)
	}
}

// TestWriteQueueStateHasNoCheckConstraint executes the premise the validator
// exists for, rather than trusting the ADR's word for it.
//
// If the column ever regained a CHECK, Go would no longer be the only place the
// vocabulary lives and this whole file's argument would need rewriting — so this
// asserts BOTH halves: every legal state inserts, AND a state that is obvious
// nonsense ALSO inserts. The second is the uncomfortable one and it is the
// point: SQLite accepts 'pendign' today, which is precisely why
// ValidWriteQueueState has to run before the INSERT rather than after it.
func TestWriteQueueStateHasNoCheckConstraint(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	insert := func(key, state string) error {
		return s.DB().Write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO write_queue (idempotency_key, user_id, kind, payload, state)
				 VALUES (?, 0, 'grab', '{}', ?)`, key, state)
			return err
		})
	}

	for _, state := range []string{
		WriteQueueStatePending, WriteQueueStateInflight, WriteQueueStateVerifying,
		WriteQueueStateAwaitingChoice, WriteQueueStateDone, WriteQueueStateFailed,
	} {
		if err := insert("k-"+state, state); err != nil {
			t.Errorf("write_queue rejected the legal state %q: %v\n"+
				"ADR-0039 decision 1 dropped this column's CHECK; if something put one "+
				"back, the Go vocabulary is no longer the single home it claims to be.",
				state, err)
		}
	}

	// The premise, executed. This INSERT must SUCCEED.
	const misspelt = "pendign"
	if ValidWriteQueueState(misspelt) {
		t.Fatalf("%q is in the Go vocabulary, so this case is no longer testing what it "+
			"says it is", misspelt)
	}
	if err := insert("k-misspelt", misspelt); err != nil {
		t.Errorf("write_queue REJECTED %q at the database: %v\n"+
			"That is good news and it falsifies this file's premise: the column is "+
			"supposed to carry no CHECK (ADR-0039 decision 1), which is the entire reason "+
			"the vocabulary has to be validated in Go. If a CHECK was added, ADR-0039 and "+
			"migration 00005's column comment both need changing in the same commit.",
			misspelt, err)
	}
}
