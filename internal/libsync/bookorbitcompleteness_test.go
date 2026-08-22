package libsync

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// THE CONTENT-FILTER SHORTFALL CHECK.
//
// Every test here is written so that it goes RED if the check stops being made,
// stops being three-valued, or starts refusing an import — which are the three
// ways this could turn back into the defect it closes.

func completenessByRef(t *testing.T, src *BookOrbitSource) map[string]ContainerCompleteness {
	t.Helper()
	out := map[string]ContainerCompleteness{}
	for _, c := range src.Completeness() {
		out[c.RemoteID] = c
	}
	return out
}

// completenessReasonsUsArrCanWrite is EVERY value ContainerCompleteness.Reason
// is allowed to hold, HAND-COPIED from bookorbitcompleteness.go rather than
// imported from it.
//
// ⚠️ THE DUPLICATION IS THE POINT AND IT MUST NOT BE REFACTORED AWAY. A set
// derived from completenessReason's own returns is satisfied by whatever that
// function returns — including a future arm that returns err.Error(), which is
// exactly the leak this guard exists to catch, wearing the guard's clothes.
// Hardcoded, a sixth arm goes RED and a person has to look at it before
// admitting its sentence here by hand. That failure is the feature.
//
// ⚠️ IT REPLACED A SUBSTRING BLACKLIST over "403" and "bookorbit:", which spoke
// only about the tokens it happened to name, only on the one arm it happened to
// run on. Appending err.Error() to completenessReason's default arm left that
// blacklist green — measured, not supposed. Membership speaks about the WHOLE
// value, on every arm the assertion is placed on, and the placement is half the
// guard: see the call sites below.
//
// "" is a member. Reason is unset under `complete` and `shortfall`, and the
// arms where an empty reason would itself be the defect assert Reason != ""
// separately.
var completenessReasonsUsArrCanWrite = map[string]bool{
	"": true,
	"BookOrbit refused UsArr the statistics this check compares against":     true,
	"BookOrbit has no statistics route for this library":                     true,
	"the statistics this check compares against could not be read":           true,
	"the two counts disagreed in the direction only a moving table explains": true,
}

func assertReasonIsUsArrsOwnWords(t *testing.T, reason string) {
	t.Helper()
	if completenessReasonsUsArrCanWrite[reason] {
		return
	}
	t.Errorf("ContainerCompleteness.Reason is not one of UsArr's own sentences.\n"+
		"  got: %q\n"+
		"This value is copied into sync_report.detail and rendered in a browser, and "+
		"reference/security.md §5 keeps upstream response text out of it. Either an "+
		"upstream error leaked in — fix the producer in bookorbitcompleteness.go — or a "+
		"new arm was added there with a new UsArr-authored sentence, in which case add "+
		"that literal to completenessReasonsUsArrCanWrite in this file, by hand. Do NOT "+
		"make the set derive from completenessReason: a derived set passes for every "+
		"future arm automatically.", reason)
}

// TestAContentFilterShortfallIsMeasuredNotGuessed is the headline: BookOrbit's
// library listing says 389 and its own unfiltered stats say 412, and UsArr
// records the 23 rather than never noticing.
func TestAContentFilterShortfallIsMeasuredNotGuessed(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{
			{ID: 1, Name: "Fiction", BookCount: 389},
			{ID: 2, Name: "Audio", BookCount: 12},
		},
		// Library 1 is filtered; library 2 is not.
		stats: map[int64]int64{1: 412, 2: 12},
	}
	var buf bytes.Buffer
	src := NewBookOrbitSource(r)
	src.Log = logTo(&buf)

	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("Containers: %v", err)
	}

	got := completenessByRef(t, src)
	fiction, ok := got["1"]
	if !ok {
		t.Fatalf("no verdict for library 1; got %v", got)
	}
	if fiction.State != store.CompletenessShortfall {
		t.Errorf("state = %q, want shortfall", fiction.State)
	}
	if fiction.Total != 412 || fiction.Visible != 389 || fiction.Hidden() != 23 {
		t.Errorf("verdict = %+v, want 412 held / 389 visible / 23 hidden", fiction)
	}

	// ⚠️ THE CLEAN LIBRARY GETS A VERDICT TOO. If only shortfalls were recorded,
	// an instance whose probes had all started failing would be indistinguishable
	// from an instance with nothing wrong.
	audio, ok := got["2"]
	if !ok {
		t.Fatalf("library 2 was measured and produced no verdict; got %v", got)
	}
	if audio.State != store.CompletenessComplete {
		t.Errorf("library 2's state = %q, want complete", audio.State)
	}
	if audio.Hidden() != 0 {
		t.Errorf("a complete library reports %d hidden", audio.Hidden())
	}
	// The `shortfall` and `complete` arms, which leave Reason unset. They are
	// asserted on because a producer that started explaining a shortfall with the
	// upstream's words would leak through a path no unverified test visits.
	assertReasonIsUsArrsOwnWords(t, fiction.Reason)
	assertReasonIsUsArrsOwnWords(t, audio.Reason)

	// The operator has to be able to find this in the log too, with the fix in
	// it: the filter is on the BookOrbit account, not on anything in UsArr.
	if log := buf.String(); !strings.Contains(log, "hidden_books=23") ||
		!strings.Contains(log, "BookOrbit account") {
		t.Errorf("the shortfall was not reported with its fix:\n%s", log)
	}
}

// TestAGuardedStatsRouteReadsAsUnverifiedAndNeverAsComplete is constraint 2, and
// it is the test this whole design exists for.
//
// The check rests on BookOrbit's `GET /libraries/:id/stats` carrying no
// @RequirePermission. That is a property of somebody else's service. On the day
// it changes, every probe answers 403 — and the answer must be "we do not know",
// never "everything is fine".
func TestAGuardedStatsRouteReadsAsUnverifiedAndNeverAsComplete(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs:     []bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 389}},
		statsErr: map[int64]error{1: fmt.Errorf("stats: %w", bookorbit.ErrForbidden)},
	}
	var buf bytes.Buffer
	src := NewBookOrbitSource(r)
	src.Log = logTo(&buf)

	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("a refused stats probe failed the container read: %v", err)
	}

	got := completenessByRef(t, src)["1"]
	if got.State != store.CompletenessUnverified {
		t.Fatalf("state = %q, want unverified — a refused probe must never read as complete", got.State)
	}
	// ⚠️ -1, NOT 0. Zero is a legal total for an empty library, so a sentinel
	// that collided with it would put "not measured" and "empty" in one value.
	if got.Total != -1 {
		t.Errorf("Total = %d, want -1: an unmeasured total must not be a number "+
			"an empty library could also produce", got.Total)
	}
	if got.Hidden() != 0 {
		t.Errorf("an unverified verdict claims %d hidden", got.Hidden())
	}
	if got.Reason == "" {
		t.Error("an unverified verdict carries no reason, so no screen can say why")
	}
	assertReasonIsUsArrsOwnWords(t, got.Reason)
	if !strings.Contains(buf.String(), "unverified, never as complete") {
		t.Errorf("the degradation was not reported:\n%s", buf.String())
	}
}

// TestACompletenessProbeNeverFailsAnImport is constraint 1: a partial replica
// that says so beats no replica.
func TestACompletenessProbeNeverFailsAnImport(t *testing.T) {
	boom := errors.New("the network went away")
	r := &fakeBookOrbitReader{
		libs:     []bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 2}},
		statsErr: map[int64]error{1: boom},
		books:    map[int64][]bookorbit.Book{1: {proseBook(1, "A"), proseBook(2, "B")}},
	}
	src := NewBookOrbitSource(r)

	containers, err := src.Containers(t.Context())
	if err != nil {
		t.Fatalf("Containers refused over a failed completeness probe: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("containers = %v, want the library the listing returned", containers)
	}

	var read int
	n, err := src.StreamItems(t.Context(), func(store.CatalogueItem) error { read++; return nil })
	if err != nil {
		t.Fatalf("StreamItems refused over a failed completeness probe: %v", err)
	}
	if n != 2 || read != 2 {
		t.Errorf("the walk delivered %d of 2 books; a reporting probe must not cost the catalogue", read)
	}
	got := completenessByRef(t, src)["1"]
	if got.State != store.CompletenessUnverified {
		t.Errorf("state = %q, want unverified", got.State)
	}
	// completenessReason's DEFAULT arm, which is what `boom` reaches. This test
	// read State and nothing else, and that is how a blacklist on the 403 test
	// alone came to be the only thing standing between err.Error() and a browser.
	assertReasonIsUsArrsOwnWords(t, got.Reason)
}

// TestAnImpossibleCountDisagreementIsUnverifiedRatherThanComplete covers the
// direction the subtraction cannot explain: the UNFILTERED total came back lower
// than the FILTERED listing.
//
// A book deleted between the two reads produces this ordinarily and harmlessly,
// and so would a getStats that had quietly stopped meaning what it means. This
// side cannot tell those apart, so it says so.
func TestAnImpossibleCountDisagreementIsUnverifiedRatherThanComplete(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs:  []bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 10}},
		stats: map[int64]int64{1: 9},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	got := completenessByRef(t, src)["1"]
	if got.State != store.CompletenessUnverified {
		t.Errorf("state = %q, want unverified", got.State)
	}
	if got.Total != -1 {
		t.Errorf("Total = %d, want -1", got.Total)
	}
	// The one unverified reason that is NOT completenessReason's: it is assigned
	// at the impossible-disagreement branch instead, so it is a fifth arm of the
	// same field and belongs in the same set.
	assertReasonIsUsArrsOwnWords(t, got.Reason)
}

// TestANotFoundStatsRouteIsDeclinedInUsArrsOwnWords covers completenessReason's
// remaining arm, which no test reached until the membership assertion made the
// gap visible: a 404 says the route has moved, which is a different operator
// action from a 403 and gets a different sentence.
func TestANotFoundStatsRouteIsDeclinedInUsArrsOwnWords(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs:     []bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 7}},
		statsErr: map[int64]error{1: fmt.Errorf("stats: %w", bookorbit.ErrNotFound)},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("a 404 stats probe failed the container read: %v", err)
	}
	got := completenessByRef(t, src)["1"]
	if got.State != store.CompletenessUnverified {
		t.Fatalf("state = %q, want unverified — a missing stats route is not a clean bill", got.State)
	}
	if got.Reason == "" {
		t.Error("an unverified verdict carries no reason, so no screen can say why")
	}
	assertReasonIsUsArrsOwnWords(t, got.Reason)
}

// TestCompletenessIsEmptyBeforeAnythingIsChecked pins the third absence: no
// containers read means no verdicts, and that is NOT a clean bill of health.
func TestCompletenessIsEmptyBeforeAnythingIsChecked(t *testing.T) {
	src := NewBookOrbitSource(&fakeBookOrbitReader{})
	if got := src.Completeness(); len(got) != 0 {
		t.Errorf("a source that has read nothing reports %v", got)
	}
}

// TestTheStatsProbeIsMadeOncePerLibraryAndNotPerBook is the cost guard. One
// extra request per library per import is the budget; one per page or one per
// book is not.
func TestTheStatsProbeIsMadeOncePerLibraryAndNotPerBook(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 3}, {ID: 2, Name: "Audio", BookCount: 0}},
		books: map[int64][]bookorbit.Book{
			1: {proseBook(1, "A"), proseBook(2, "B"), proseBook(3, "C")},
		},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if _, err := src.StreamItems(t.Context(), func(store.CatalogueItem) error { return nil }); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	if len(r.statsSeen) != 2 {
		t.Errorf("the stats route was called %d times for 2 libraries and 3 books: %v",
			len(r.statsSeen), r.statsSeen)
	}
}

// TestTheCompletenessCheckDoesNotClaimAnythingAboutHiddenLibraries is
// constraint 4, asserted rather than left to a comment.
//
// The listing returns ONE library. Whether the account was granted access to
// others is unanswerable read-only — LibraryAccessGuard throws an identical
// ForbiddenException for "exists, no access row" and for "no such library" — so
// nothing here may be phrased as a statement about the instance.
func TestTheCompletenessCheckDoesNotClaimAnythingAboutHiddenLibraries(t *testing.T) {
	r := &fakeBookOrbitReader{libs: []bookorbit.Library{{ID: 1, Name: "Fiction", BookCount: 5}}}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(t.Context()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	got := src.Completeness()
	if len(got) != 1 {
		t.Fatalf("verdicts = %v, want exactly one — one per container the listing returned", got)
	}
	// The verdict is filed under a CONTAINER id. There is no instance-level
	// verdict on this type at all, which is what stops one being rendered.
	if got[0].RemoteID != "1" {
		t.Errorf("verdict is keyed by %q, want the container id", got[0].RemoteID)
	}
	if got[0].Name != "Fiction" {
		t.Errorf("verdict names %q rather than the container", got[0].Name)
	}
}

// The interface really is satisfied by the shipping client, which is the one
// thing a hand-built fake cannot prove.
var _ BookOrbitReader = (*bookorbit.Client)(nil)
