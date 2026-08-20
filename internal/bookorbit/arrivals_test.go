package bookorbit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"testing"
	"time"
)

// ─── the arrivals fake ───────────────────────────────────────────────────────

// arrivalsRow is one book in the server-side fixture, in the two fields the
// walk's correctness turns on.
type arrivalsRow struct {
	id      int64
	addedAt string
}

// arrivalsFixture is a fake BookOrbit that serves POST /libraries/{id}/books the
// way BookQueryBuilder does: `after` is a STRICT `>`, the sort is
// (addedAt ASC, id ASC) because the server appends `books.id ASC`
// unconditionally, and a page is EXACTLY the requested size until the last one.
//
// ⚠️ IT SERVES FULL PAGES HONESTLY. A fake that shrank a page of its own accord
// would let a broken stop condition pass here and then spin against a real
// instance — the same rule defaultBooks already states, restated because the
// keyset's stop condition is the short page and nothing else.
type arrivalsFixture struct {
	rows []arrivalsRow

	// bodies is every request body the walk sent, in order. Asserting the BODY
	// is the whole reason these tests build against this seam: the full walk and
	// the delta walk hit the SAME URL, so a recorded-cassette matcher that
	// ignores bodies structurally cannot tell them apart.
	bodies []bookQueryRequest

	// beforePage runs before each page is served, so a test can mutate the
	// fixture MID-WALK the way a live scanner does.
	beforePage func(f *arrivalsFixture, page int)
}

func (a *arrivalsFixture) handler(t *testing.T, f *fake) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if !f.bearerOK(r) {
			nestErrorText(w, r, http.StatusUnauthorized, "Unauthorized")
			return
		}
		blob, _ := io.ReadAll(r.Body)
		var q bookQueryRequest
		if err := json.Unmarshal(blob, &q); err != nil {
			nestErrorText(w, r, http.StatusBadRequest, "Unexpected token in JSON")
			return
		}
		a.bodies = append(a.bodies, q)
		if a.beforePage != nil {
			a.beforePage(a, len(a.bodies)-1)
		}

		// The filter, exactly as BookQueryBuilder applies it: gt(books.addedAt).
		var cut string
		if q.Filter != nil {
			if len(q.Filter.Rules) != 1 {
				nestErrorJSON(w, r, http.StatusBadRequest, []string{"filter.rules: unexpected shape"})
				return
			}
			rule := q.Filter.Rules[0]
			if rule.Field != "addedAt" || rule.Operator != "after" {
				nestErrorJSON(w, r, http.StatusBadRequest,
					[]string{fmt.Sprintf("filter: %s/%s is not what this fixture serves", rule.Field, rule.Operator)})
				return
			}
			cut = rule.Value
		}

		match := make([]arrivalsRow, 0, len(a.rows))
		for _, row := range a.rows {
			if cut == "" || row.addedAt > cut {
				match = append(match, row)
			}
		}
		sort.SliceStable(match, func(i, j int) bool {
			if match[i].addedAt != match[j].addedAt {
				return match[i].addedAt < match[j].addedAt
			}
			return match[i].id < match[j].id
		})

		start := min(q.Pagination.Page*q.Pagination.Size, len(match))
		end := min(start+q.Pagination.Size, len(match))
		items := make([]map[string]any, 0, end-start)
		for _, row := range match[start:end] {
			items = append(items, fakeCard(row.id, "Book "+strconv.FormatInt(row.id, 10),
				map[string]any{"addedAt": row.addedAt}))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": items, "total": len(match), "page": q.Pagination.Page, "size": q.Pagination.Size,
		})
	}
}

// arrivalsAt renders a fixture instant the way upstream's toISOString() does.
func arrivalsAt(sec int, ms int) string {
	return time.Date(2026, 8, 1, 0, 0, sec, ms*int(time.Millisecond), time.UTC).
		Format(ArrivalsInstantLayout)
}

// walkArrivals runs a delta walk against a fixture and collects the ids.
func walkArrivals(t *testing.T, a *arrivalsFixture, cursor ArrivalsCursor) (*fake, ArrivalsWalk, []int64, error) {
	t.Helper()
	f := newFake(t)
	f.booksHandler = a.handler(t, f)
	c := f.client(t)

	var got []int64
	walk, err := c.StreamBooksSince(context.Background(), 1, cursor, func(b Book) error {
		got = append(got, b.ID)
		return nil
	})
	return f, walk, got, err
}

// ─── page 0, always ──────────────────────────────────────────────────────────

// Under a keyset there is no offset. `page` is 0 on every request forever, and
// the filter value is what moves — which is mechanically checkable, so it is
// checked rather than asserted in a comment.
func TestArrivalsWalkAsksForPageZeroOnEveryRequest(t *testing.T) {
	var rows []arrivalsRow
	for i := range 450 {
		rows = append(rows, arrivalsRow{id: int64(i + 1), addedAt: arrivalsAt(i, 0)})
	}
	a := &arrivalsFixture{rows: rows}

	_, walk, got, err := walkArrivals(t, a, ArrivalsCursor{})
	if err != nil {
		t.Fatalf("StreamBooksSince: %v", err)
	}
	// 452 HAND-OVERS FOR 450 BOOKS, AND THE TWO EXTRA ARE THE MITIGATION WORKING
	// RATHER THAN A DEFECT. The cursor after a full page is the largest value
	// STRICTLY BELOW the page's last, so the page's final row is re-read on the
	// next request — one row per page boundary here, because these stamps are all
	// distinct. That redelivery is the mitigation's whole price and the idempotent
	// upsert is what pays it.
	if walk.Pages != 3 {
		t.Fatalf("pages = %d, want 3", walk.Pages)
	}
	seen := map[int64]bool{}
	for _, id := range got {
		seen[id] = true
	}
	if len(seen) != 450 {
		t.Fatalf("delivered %d distinct books, want 450", len(seen))
	}
	if len(got) != 452 {
		t.Errorf("hand-overs = %d, want 452 (450 books plus one re-read per page boundary); "+
			"exactly 450 would mean the cursor advanced to the page's last value, which is the "+
			"advance that loses a straddling tie group", len(got))
	}
	for i, b := range a.bodies {
		if b.Pagination.Page != 0 {
			t.Errorf("request %d: page = %d, want 0 — a keyset never asks for an offset",
				i, b.Pagination.Page)
		}
		if b.Pagination.Size != BookPageSize {
			t.Errorf("request %d: size = %d, want %d", i, b.Pagination.Size, BookPageSize)
		}
		if len(b.Sort) != 1 || b.Sort[0].Field != "addedAt" || b.Sort[0].Dir != "asc" {
			t.Errorf("request %d: sort = %+v, want addedAt asc", i, b.Sort)
		}
	}
	// Request 1 carries no filter (no cursor); every later request carries the
	// mitigation's value, which for distinct stamps is the SECOND-LARGEST on the
	// page just read.
	if a.bodies[0].Filter != nil {
		t.Errorf("request 0 sent a filter with no cursor: %+v", a.bodies[0].Filter)
	}
	if got, want := a.bodies[1].Filter.Rules[0].Value, arrivalsAt(198, 0); got != want {
		t.Errorf("request 1 filter = %q, want %q — the cursor is the largest value on the page "+
			"STRICTLY BELOW its last, never the last itself", got, want)
	}
	// Page 2 read everything strictly above second 198, so its last value is
	// second 398 and the cursor takes second 397 — one below, again.
	if got, want := a.bodies[2].Filter.Rules[0].Value, arrivalsAt(397, 0); got != want {
		t.Errorf("request 2 filter = %q, want %q", got, want)
	}
}

// ─── the mitigation ──────────────────────────────────────────────────────────

// THE 70-ROW LOSS, REPRODUCED AS A REGRESSION TEST. The shape is the measured
// one: ordinary arrivals, then a tie group large enough to straddle the page
// boundary, then later arrivals. A naive keyset that advanced to the page's last
// value delivered 260 of 330 rows, terminated normally on a short page and
// reported success; the 70 it dropped were excluded by `added_at > W` on every
// future poll forever.
func TestArrivalsWalkRecoversAStraddlingTieGroup(t *testing.T) {
	// ⚠️ THE TIE MUST STRADDLE THE PAGE BOUNDARY OR THIS TEST PROVES NOTHING.
	// The instant sits ABOVE the first 150 arrivals and BELOW the last 60, so the
	// sorted set is 150 distinct rows, then the 120-row group, then 60 more —
	// which puts 50 of the group on page 1 and 70 on page 2. An earlier draft of
	// this fixture used an instant that sorted FIRST; the whole group then fitted
	// inside page 1, nothing straddled, and the test passed against a walk with
	// the mitigation deliberately removed. Fired, and it is the reason this
	// paragraph exists.
	const tie = "2026-08-01T00:02:30.000Z" // == arrivalsAt(150, 0)
	var rows []arrivalsRow
	for i := range 150 { // 1..150, distinct and strictly below the tie
		rows = append(rows, arrivalsRow{id: int64(i + 1), addedAt: arrivalsAt(i, 0)})
	}
	for i := range 120 { // 151..270, ONE byte-identical instant
		rows = append(rows, arrivalsRow{id: int64(151 + i), addedAt: tie})
	}
	for i := range 60 { // 271..330, distinct and above
		rows = append(rows, arrivalsRow{id: int64(271 + i), addedAt: arrivalsAt(3600+i, 0)})
	}
	a := &arrivalsFixture{rows: rows}

	_, walk, got, err := walkArrivals(t, a, ArrivalsCursor{})
	if err != nil {
		t.Fatalf("StreamBooksSince: %v", err)
	}
	if walk.Wedged {
		t.Fatalf("a 120-row tie group is smaller than a page and must not wedge")
	}

	seen := map[int64]bool{}
	for _, id := range got {
		seen[id] = true
	}
	var missing []int64
	for i := int64(1); i <= 330; i++ {
		if !seen[i] {
			missing = append(missing, i)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("%d rows were never delivered (%v…) — this is the 70-row loss the mitigation exists "+
			"to close, and every one of them is excluded from every future poll forever",
			len(missing), missing[:min(5, len(missing))])
	}
	// The tie group is re-read, which is the mitigation's stated price: at most
	// one group per page, absorbed by the idempotent upsert.
	if len(got) <= 330 {
		t.Errorf("delivered %d rows for 330 distinct books; the mitigation re-reads the boundary "+
			"group, so some redelivery is expected and its absence means the cursor advanced past it",
			len(got))
	}
}

// ─── termination ─────────────────────────────────────────────────────────────

// A two-page walk terminates. It is the guard against applying the lookbehind
// PER PAGE: `after(pageMax − 5 min)` is, for any page spanning less than five
// minutes, at or below where that page started, so the identical page returns
// forever. Nothing in the tree does that; this is what keeps it that way.
func TestArrivalsWalkTerminates(t *testing.T) {
	var rows []arrivalsRow
	for i := range 260 {
		// Every row inside one five-minute span, which is exactly the window a
		// per-page lookbehind would subtract.
		rows = append(rows, arrivalsRow{id: int64(i + 1), addedAt: arrivalsAt(i, 0)})
	}
	a := &arrivalsFixture{rows: rows}

	_, walk, got, err := walkArrivals(t, a, ArrivalsCursor{})
	if err != nil {
		t.Fatalf("StreamBooksSince: %v", err)
	}
	if walk.Pages != 2 {
		t.Fatalf("pages = %d, want 2 — more means the cursor did not advance past the page it read",
			walk.Pages)
	}
	if len(got) < 260 {
		t.Errorf("delivered %d of 260", len(got))
	}
}

// A short page is the end, and it is the ONLY stop that means "finished".
func TestArrivalsWalkStopsOnAShortPage(t *testing.T) {
	var rows []arrivalsRow
	for i := range 5 {
		rows = append(rows, arrivalsRow{id: int64(i + 1), addedAt: arrivalsAt(i, 0)})
	}
	a := &arrivalsFixture{rows: rows}

	_, walk, got, err := walkArrivals(t, a, ArrivalsCursor{})
	if err != nil {
		t.Fatalf("StreamBooksSince: %v", err)
	}
	if walk.Pages != 1 || len(got) != 5 {
		t.Errorf("pages = %d, delivered = %d, want 1 and 5", walk.Pages, len(got))
	}
}

// ─── the table moves under the walk ──────────────────────────────────────────

// A mid-walk INSERT above the cursor is delivered; a mid-walk DELETE above the
// cursor is simply not. Neither shifts anything, because a keyset has no offset
// for a deletion to shift — which is the property offset paging does not have
// and the `total`-decrease guard did not restore.
func TestArrivalsWalkSeesAMidWalkInsertAndDelete(t *testing.T) {
	var rows []arrivalsRow
	for i := range 260 {
		rows = append(rows, arrivalsRow{id: int64(i + 1), addedAt: arrivalsAt(i, 0)})
	}
	a := &arrivalsFixture{rows: rows}
	a.beforePage = func(f *arrivalsFixture, page int) {
		if page != 1 {
			return
		}
		// One arrival lands above the cursor, and one row above the cursor is
		// deleted — the compensating pair that nets to zero on `total` and that
		// the sign of a net figure cannot see.
		f.rows = append(f.rows, arrivalsRow{id: 9001, addedAt: arrivalsAt(500, 0)})
		for i, r := range f.rows {
			if r.id == 259 {
				f.rows = append(f.rows[:i], f.rows[i+1:]...)
				break
			}
		}
	}

	_, _, got, err := walkArrivals(t, a, ArrivalsCursor{})
	if err != nil {
		t.Fatalf("StreamBooksSince: %v", err)
	}
	seen := map[int64]bool{}
	for _, id := range got {
		seen[id] = true
	}
	if !seen[9001] {
		t.Errorf("the mid-walk arrival was never delivered")
	}
	if seen[259] {
		t.Errorf("a row deleted mid-walk above the cursor was delivered anyway")
	}
	// The rows BELOW the deleted one must all still be there: under offset paging
	// the delete would have pulled one row across the cursor and lost it.
	for i := int64(1); i <= 258; i++ {
		if !seen[i] {
			t.Fatalf("row %d was skipped — a deletion shifted the walk, which a keyset cannot do", i)
		}
	}
}

// ─── THE WEDGE DRILL ─────────────────────────────────────────────────────────

// FIRING IT, because "loud" is a measurement and not an intention: from the
// outside an alarm never triggered is indistinguishable from no alarm at all.
//
// More than a page of rows sharing ONE instant leaves the page full with its
// first value equal to its last, so there is no value strictly below the last
// for the cursor to take. The walk must STOP — asserted through the REQUEST
// COUNT, which is what distinguishes a wedge from a walk that never terminates —
// must report the condition distinguishably rather than as a generic error, and
// must have delivered exactly the one page it read.
func TestArrivalsWalkWedgesOnAPageThatIsOneInstant(t *testing.T) {
	const midnight = "2026-06-01T00:00:00.000Z"
	var rows []arrivalsRow
	for i := range BookPageSize + 1 { // 201 rows, one instant
		rows = append(rows, arrivalsRow{id: int64(i + 1), addedAt: midnight})
	}
	a := &arrivalsFixture{rows: rows}

	_, walk, got, err := walkArrivals(t, a, ArrivalsCursor{})

	if err == nil {
		t.Fatal("the walk succeeded; a wedge must fail to its caller, not return success with a note")
	}
	if !walk.Wedged {
		t.Errorf("walk.Wedged = false — the condition must be distinguishable from a generic error, "+
			"because the repair named for it (a full import) is not the repair for a timeout: %v", err)
	}
	if walk.WedgeAt != midnight {
		t.Errorf("WedgeAt = %q, want %q — the instant is what a person needs to find the rows",
			walk.WedgeAt, midnight)
	}
	if walk.Pages != 1 {
		t.Fatalf("pages = %d, want 1 — MORE THAN ONE MEANS THE WALK LOOPED rather than wedged, "+
			"and a loop is the failure the wedge exists to convert into a stop", walk.Pages)
	}
	if len(got) != BookPageSize || walk.Count != BookPageSize {
		t.Errorf("delivered %d / Count %d, want %d each: the page it read stands, and nothing beyond it",
			len(got), walk.Count, BookPageSize)
	}
	// Named in UsArr's own words, with the repair, and never upstream prose.
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T, want *APIError", err)
	}
	for _, want := range []string{"share one arrival instant", "nothing was skipped", "full import"} {
		if !contains(apiErr.Message, want) {
			t.Errorf("the wedge message does not say %q: %s", want, apiErr.Message)
		}
	}
}

// THE NEGATIVE CONTROL, and without it the drill cannot distinguish "wedges
// correctly" from "wedges always". A full page of one instant PLUS one row at a
// strictly later value: the mitigation has a value to take, so there is no
// wedge, the walk completes, and every row is delivered.
func TestArrivalsWalkDoesNotWedgeWhenThePageHasASecondValue(t *testing.T) {
	const midnight = "2026-06-01T00:00:00.000Z"
	var rows []arrivalsRow
	for i := range BookPageSize - 1 { // 199 at the tie
		rows = append(rows, arrivalsRow{id: int64(i + 1), addedAt: midnight})
	}
	// The 200th row completes the page at a strictly later instant, and a 201st
	// sits above it so the walk must page at least twice.
	rows = append(rows,
		arrivalsRow{id: 200, addedAt: arrivalsAt(7200, 0)},
		arrivalsRow{id: 201, addedAt: arrivalsAt(7201, 0)})
	a := &arrivalsFixture{rows: rows}

	_, walk, got, err := walkArrivals(t, a, ArrivalsCursor{})
	if err != nil {
		t.Fatalf("StreamBooksSince: %v — 200 tied rows plus a later one is NOT a wedge", err)
	}
	if walk.Wedged {
		t.Fatal("Wedged = true on a page carrying two distinct values; the drill would then be " +
			"proving that the walk wedges always rather than that it wedges correctly")
	}
	seen := map[int64]bool{}
	for _, id := range got {
		seen[id] = true
	}
	for i := int64(1); i <= 201; i++ {
		if !seen[i] {
			t.Fatalf("row %d was never delivered", i)
		}
	}
}

// ─── the bootstrap and the full walk's body ──────────────────────────────────

// An unset cursor sends NO filter key at all — not a filter on the zero time,
// which would be a bound nothing observed.
func TestArrivalsWalkOmitsTheFilterWhenTheCursorIsUnset(t *testing.T) {
	a := &arrivalsFixture{rows: []arrivalsRow{{id: 1, addedAt: arrivalsAt(0, 0)}}}
	_, _, got, err := walkArrivals(t, a, ArrivalsCursor{})
	if err != nil {
		t.Fatalf("StreamBooksSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("delivered %d, want 1", len(got))
	}
	if a.bodies[0].Filter != nil {
		t.Errorf("filter = %+v, want absent", a.bodies[0].Filter)
	}
}

// ⚠️ THE FULL WALK'S BODY IS BYTE-IDENTICAL TO WHAT IT WAS BEFORE THE FILTER
// FIELD EXISTED, and this is what makes `omitempty` a FACT rather than an
// intention. BookOrbit's ValidationPipe runs forbidNonWhitelisted, so an unknown
// key is REJECTED rather than ignored; a Filter field that serialised as
// `"filter": null` on every channel-1 request would put the WORKING IMPORT at
// risk, not the new feature.
func TestTheFullWalkRequestBodyCarriesNoFilterKey(t *testing.T) {
	got, err := json.Marshal(bookQueryRequest{
		Sort:       fullWalkSort,
		Pagination: bookPagination{Page: 0, Size: BookPageSize},
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"sort":[{"field":"addedAt","dir":"asc"}],"pagination":{"page":0,"size":200}}`
	if string(got) != want {
		t.Errorf("full walk body =\n  %s\nwant\n  %s", got, want)
	}
}

// The delta's body, pinned in full, so the shape a real BookOrbit's validator
// sees is a fact in this repo rather than a reading of a design document.
func TestTheArrivalsRequestBodyIsExactlyOneAfterRule(t *testing.T) {
	got, err := json.Marshal(arrivalsRequest(ArrivalsCursor{Value: "2026-08-20T09:07:33.987Z", Set: true}))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"sort":[{"field":"addedAt","dir":"asc"}],"pagination":{"page":0,"size":200},` +
		`"filter":{"type":"group","join":"AND","rules":[{"type":"rule","field":"addedAt",` +
		`"operator":"after","value":"2026-08-20T09:07:33.987Z"}]}}`
	if string(got) != want {
		t.Errorf("arrivals body =\n  %s\nwant\n  %s", got, want)
	}
}

// The wire layout: fixed width, three fractional digits kept even when zero, and
// .UTC() applied so the zone is Z and the values are lexicographically ordered.
func TestArrivalsInstantRenderingIsFixedWidthAndOrdered(t *testing.T) {
	zone := time.FixedZone("CEST", 2*60*60)
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 8, 1, 9, 12, 33, 0, time.UTC), "2026-08-01T09:12:33.000Z"},
		{time.Date(2026, 8, 1, 9, 12, 33, 980*int(time.Millisecond), time.UTC), "2026-08-01T09:12:33.980Z"},
		{time.Date(2026, 8, 1, 9, 12, 33, 999999000, time.UTC), "2026-08-01T09:12:33.999Z"},
		{time.Date(2026, 8, 1, 11, 12, 33, 0, zone), "2026-08-01T09:12:33.000Z"},
	}
	for _, c := range cases {
		if got := FormatArrivalsInstant(c.in); got != c.want {
			t.Errorf("FormatArrivalsInstant(%s) = %q, want %q", c.in, got, c.want)
		}
	}
	// Fixed width is what makes string comparison a valid ordering, which both
	// the mitigation and the fold rely on.
	a := FormatArrivalsInstant(time.Date(2026, 8, 1, 9, 12, 33, 0, time.UTC))
	b := FormatArrivalsInstant(time.Date(2026, 8, 1, 9, 12, 33, 980*int(time.Millisecond), time.UTC))
	if len(a) != len(b) || a >= b {
		t.Errorf("%q and %q are not a fixed-width ordered pair", a, b)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
