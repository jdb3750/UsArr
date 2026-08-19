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
	if strings.Contains(got.Reason, "403") || strings.Contains(got.Reason, "bookorbit:") {
		t.Errorf("the upstream's own error text reached a field that renders in a browser: %q", got.Reason)
	}
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
	if got := completenessByRef(t, src)["1"].State; got != store.CompletenessUnverified {
		t.Errorf("state = %q, want unverified", got)
	}
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
