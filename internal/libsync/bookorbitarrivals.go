package libsync

import (
	"context"
	"fmt"
	"time"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// arrivalsLookbehind is how far BELOW the stored cursor position request 1 asks,
// once per walk.
//
// # ⚠️ It is applied ONCE, TO THE WATERMARK, TO SEED THE CURSOR — never per page
//
// Request 1 sends `after(W − lookbehind)`. Every subsequent request sends
// `after(<cursor>)` with NO lookbehind, where the cursor comes from the tie
// mitigation.
//
// 🚩 APPLYING IT PER PAGE IS A NON-TERMINATING WALK, and it is a failure mode
// offset paging could not have had: under offset paging the filter is fixed for
// the whole walk and only the offset moves, so there is nothing to get wrong.
// Under a keyset the filter value IS the cursor, so `after(pageMax − 5 min)` is,
// for any page spanning less than five minutes of arrivals, AT OR BELOW where
// that page started — the identical page returns forever, bounded only by the
// client's loop bound, i.e. five thousand pages of pure re-read before the walk
// gives up. Recorded as a hazard rather than a live defect; the test that a
// two-page walk terminates is what keeps it one.
//
// # What it protects against, and it is NOT clock skew
//
// Two properties of BookOrbit's own WRITE PATH, both about VISIBILITY rather
// than about comparison:
//
//  1. COMMIT-VISIBILITY LAG. books.added_at is `DEFAULT now()` and Postgres's
//     now() is transaction_timestamp(), so a row can be stamped at T and become
//     visible later; a poll in between can observe higher stamps and advance
//     past a row that was not yet there. The exposure is a transaction's own
//     duration. Its SIZE is unmeasured against any live BookOrbit.
//  2. `status = 'processing'` → visible. The books query ands
//     `ne(books.status,'processing')` onto its predicate, so a book stamped at T
//     and invisible until T+d is delivered only if a later poll still asks back
//     to T.
//
// # ⚠️ §7.1a's FORMULA IS NOT IMPLEMENTED, AND THE FIVE MINUTES IS A NEW CONSTANT
//
// The specified `max(5 min, 2 × |clock_skew_secs| + poll interval)` is retired
// for this source. `+ poll interval` is undefined: there is no timer. The skew
// term is dead — see clock_skew_secs below. What survives is a FIXED five
// minutes, and it is a NEW constant for a NEWLY NAMED hazard rather than the old
// floor carried forward: its equality with §7.1a's floor is a coincidence, not a
// derivation.
//
// IT IS UNMEASURED, and the direction the error is safe in is what licenses
// shipping it anyway: too large costs bounded, idempotent redelivery; too small
// loses rows. ERRING LARGE IS CORRECT. What would measure it is already
// collected — every delta_walk row carries watermark_before and
// min_delivered_added_at, both upstream's own values with no local clock in
// between, so `min_delivered < watermark_before` with works_created > 0 is an
// empirical lower bound on the window actually required, and never seeing it is
// evidence the window can be cut. That data can only be collected WHILE the
// lookbehind exists.
//
// # ⚠️ clock_skew_secs — THE SEAM, AND WHY 3b DOES NOT NEED IT
//
// `service_instance.clock_skew_secs` (migration 00001, "from the Date response
// header") is the column §7.1a's formula would read. Stated at the seam, on
// internal/store/libraries.go's precedent for `orphaned_at`:
//
//   - ⚠️ NOTHING WRITES IT AND NOTHING READS IT. Measured over the tree:
//     `clock_skew_secs`, and any Go, TypeScript or Svelte spelling of it, occurs
//     in migration 00001, in the generated schema snapshot and in
//     docs/reference/schema.md — and NOWHERE ELSE. There is no probe, no
//     accessor and no reader.
//
//   - IT IS NOT NEEDED HERE, and that is CONDITIONAL ON THE SHAPE OF THIS
//     COMPARISON RATHER THAN A PROPERTY OF THE AXIS. The seed is a maximum
//     `books.addedAt` BookOrbit itself reported, and it is compared against
//     `books.addedAt` inside BookOrbit's own WHERE. Both sides are the same
//     machine's values, so UsArr's clock never enters and there is no skew term
//     for the column to hold. THE COMPARISON HAS EXACTLY ONE BOUND AND THAT
//     BOUND IS AN UPSTREAM-OBSERVED VALUE; ANY TWO-BOUNDED FORM REINTRODUCES
//     THE SKEW. `between` is exactly that form — it needs an upper bound, UsArr
//     has no upstream-clock value for "now", so the bound comes from UsArr's
//     clock and a clock running ahead excludes the newest arrivals PERMANENTLY.
//
//   - IT IS RETAINED BECAUSE THE SEAM IS REAL. A source whose watermark is
//     computed against UsArr's OWN clock — anything where the two sides of the
//     comparison come from two machines — would need it, and channel 3 for the
//     *Arrs is specified in exactly that shape. So the column stays.
//
//   - ⚠️ THIS IS NOT "WIRING PENDING". Nothing is pending. 3b is the channel
//     that would have wired it and has decided, with a reason, that it must not:
//     a correct measurement feeding a term that guards nothing would cost a
//     per-instance probe and a reader, and would produce a number inviting the
//     reading "we have accounted for clock skew" on the one axis where there is
//     no skew to account for.
const arrivalsLookbehind = 5 * time.Minute

// arrivalsFold is the ONE producer of the arrivals facts a cursor position is
// minted from. Both channels fold through it: channel 1 so a full import can
// mint the first watermark, channel 3b so a delta can advance it.
//
// ⚠️ ITS INPUT IS THE WIRE RENDERING AND NOT A time.Time. Two instants that
// differ below a millisecond render identically, and treating them as one value
// can only make a cursor LAG — a re-read, which the idempotent upsert absorbs.
// Folding in time.Time and rendering afterwards would let this reason about a
// distinction the wire cannot express, which is how a cursor ends up above a row
// the server will never return again.
type arrivalsFold struct {
	max string
	min string

	// unreadable counts rows whose arrival stamp did not parse. A non-zero count
	// makes the fold's maximum UNTRUSTWORTHY rather than wrong: the rows were
	// delivered and applied, but a maximum computed over a set with unreadable
	// members is not known to be a maximum, so it is fail-closed.
	unreadable int
}

func (f *arrivalsFold) add(t time.Time) {
	if t.IsZero() {
		f.unreadable++
		return
	}
	at := bookorbit.FormatArrivalsInstant(t)
	if at > f.max {
		f.max = at
	}
	if f.min == "" || at < f.min {
		f.min = at
	}
}

// watermark is the fold's maximum as a durable cursor position, or an invalid
// one — which is what an unreadable stamp, and an empty run, both produce.
func (f *arrivalsFold) watermark() store.ArrivalsWatermark {
	if f.max == "" || f.unreadable > 0 {
		return store.ArrivalsWatermark{}
	}
	return store.ArrivalsWatermark{Value: f.max, Valid: true}
}

func (f *arrivalsFold) minimum() store.ArrivalsWatermark {
	if f.min == "" {
		return store.ArrivalsWatermark{}
	}
	return store.ArrivalsWatermark{Value: f.min, Valid: true}
}

// ArrivalsMax is the maximum arrival this source observed on the walk it just
// ran, full import or delta alike. See DeltaSource.
func (s *BookOrbitSource) ArrivalsMax() store.ArrivalsWatermark {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.arrivals.watermark()
}

// MeasuredEmpty reports whether every container this run graded holds NO books
// by BookOrbit's own UNFILTERED count, and says in UsArr's words why not when it
// cannot state that.
//
// # ⚠️ It is the permission-versus-empty guard, and it asks about the OBSERVATION
//
// A full import that observed zero books mints no cursor position, and "the
// catalogue held no books" is only ONE cause of that observation. This project's
// record already names another AT THE SOURCE: LibraryAccessGuard throws an
// IDENTICAL ForbiddenException for "the library exists and this account has no
// access row" and for "there is no such library", so FORBIDDEN IS
// INDISTINGUISHABLE FROM ABSENT; and a content filter lands in the books
// LEFT JOIN ON rather than in a WHERE, so it SHORTS a library's visible count
// without dropping the library row. See bookorbitcompleteness.go, which exists
// because of the second and states the first as the axis it cannot reach.
//
// 🚩 SO THIS DOES NOT ASK WHETHER A PERMISSION CHANGED. A guard scoped to one
// cause of an observation is blind to every other cause of the same
// observation — a permission test passes a content-filter change, a scanner bug,
// a route that moved, and a cause nobody has thought of yet. It asks instead
// whether an INDEPENDENT, UNFILTERED measurement AGREES that there is nothing
// there: GET /libraries/:id/stats takes neither a user nor a content filter, so
// its total is the second opinion, and `unverified` is refused as loudly as a
// non-zero total because a measurement that did not happen is not agreement.
//
// ⚠️ ITS SCOPE IS STATED RATHER THAN IMPLIED. It covers the containers the
// credential can SEE. Whether whole libraries are hidden from UsArr's account is
// unanswerable read-only, for the ForbiddenException reason above, so this
// closes the shortfall axis and NOT the membership axis. An empty verdict map —
// Containers() never ran, or the credential can see no library at all — is
// therefore refused too, rather than read as "nothing is there".
func (s *BookOrbitSource) MeasuredEmpty() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.completeness) == 0 {
		return false, "UsArr's credential can see no library on this service, which is not the same fact as this service holding no books"
	}
	for _, c := range s.completeness {
		switch {
		case c.State == store.CompletenessUnverified:
			return false, fmt.Sprintf("the library %q could not be measured against BookOrbit's own unfiltered count, so an empty result there is not evidence the library is empty", c.Name)
		case c.Total > 0:
			return false, fmt.Sprintf("the library %q holds %d books by BookOrbit's own unfiltered count, so a run that observed none observed something other than an empty catalogue", c.Name, c.Total)
		case c.Visible > 0:
			return false, fmt.Sprintf("the library %q reports books to UsArr's credential, so a run that observed none observed something other than an empty catalogue", c.Name)
		}
	}
	return true, ""
}

// StreamItemsSince is channel 3b's walk: every book that arrived strictly after
// the seed, mapped exactly as the full import maps one.
//
// # One walk per library, and why the grain is the instance
//
// The walk is PER LIBRARY, because BookCard carries no libraryId and the
// container an item belongs to is the route parameter and nothing else. The
// cursor position is per INSTANCE, holding the maximum over all libraries — so
// the ALL-OR-NOTHING rule is what makes one column safe: if library 2 of 3
// fails, LibrariesWalked comes back short, the caller declines to advance, and
// the next run re-reads libraries 1 and 3's window as well. That over-delivery
// is absorbed by the idempotent upsert.
//
// ⚠️ AND THE PRICE OF THAT RULE IS NAMED RATHER THAN LEFT TO BE DISCOVERED. One
// library answering 403 forever means the watermark never advances again for ANY
// library on the instance, and the window then grows with wall-clock time until
// the walk fails on its own size. That is survivable ONLY because the trigger is
// a person: a run that declines to advance FAILS TO ITS CALLER, synchronously,
// with the reason. It is not survivable under a timer, which is why a stalled
// cursor being visible without anyone pressing anything is a precondition on any
// timer this channel ever gets — not a nice-to-have afterwards.
//
// The mapping, the kept cards and the comic residue are the full import's, byte
// for byte: an arrival is a `book` row upstream and comes back on the same
// book-grain filter, and mapComic already files a comic issue under its series.
// A delta that mapped differently would be a second write path for the same
// rows.
func (s *BookOrbitSource) StreamItemsSince(
	ctx context.Context, seed store.ArrivalsWatermark, fn func(store.CatalogueItem) error,
) (int, DeltaObservation, error) {
	var obs DeltaObservation
	if err := s.gate(ctx); err != nil {
		return 0, obs, err
	}

	s.mu.Lock()
	libs := append([]bookorbit.Library(nil), s.containers...)
	s.mu.Unlock()

	obs.LibrariesTotal = len(libs)
	if len(libs) == 0 {
		// Containers() was never called, or the credential can see nothing.
		// Neither is an error here, for StreamItems's reason: the importer calls
		// Containers first and would have reported an empty list already.
		return 0, obs, nil
	}

	cursor, err := seedCursor(seed)
	if err != nil {
		return 0, obs, err
	}

	var read int
	var fold arrivalsFold
	for _, l := range libs {
		ref := containerRef(l.ID)
		tally := s.tallyFor(ref)
		residue := s.residueFor(ref)

		walk, err := s.Client.StreamBooksSince(ctx, l.ID, cursor, func(b bookorbit.Book) error {
			// FOLDED BEFORE THE MAPPING DECISION, not after, and that is
			// deliberate: an arrival UsArr declines to map is still an arrival,
			// and a cursor that advanced only past the rows this adapter happened
			// to understand would re-read every unmappable book on every poll,
			// forever.
			fold.add(b.AddedAt)

			switch b.MediaKind() {
			case bookorbit.MediaKindComic:
				return fn(mapComic(b, ref, residue))
			case bookorbit.MediaKindEbook, bookorbit.MediaKindAudiobook:
				s.keepCard(b)
				return fn(mapBook(b, ref))
			default:
				tally.Unknown++
				return nil
			}
		})
		read += walk.Count
		obs.Pages += walk.Pages
		if walk.Wedged {
			obs.Wedged = true
			obs.WedgeAt = walk.WedgeAt
		}
		if err != nil {
			s.foldInto(&obs, &fold)
			return read, obs, fmt.Errorf("bookorbit: arrivals walk of library %q (%s) after %d books: %w",
				l.Name, ref, walk.Count, err)
		}
		obs.LibrariesWalked++
	}

	s.foldInto(&obs, &fold)
	return read, obs, nil
}

// foldInto copies the run's observations onto the caller's record, including the
// window-scoped tallies.
//
// ⚠️ THE TALLIES ARE COPIED HERE AND NOT WRITTEN AS PER-CONTAINER ROWS, and that
// is the whole of a shipped-screen defect avoided. Skipped() and Comics() are
// tallies of WHAT THIS WALK READ; under a delta that is a five-minute arrivals
// window. The vocabulary they write is a claim about the CONTAINER — "an import
// walked this library's containers and recorded nothing left out" — and the
// Libraries screen's read is latest-wins. So a full import that found 300 comics
// it could not map, followed by one quiet delta poll, would leave that screen
// reading "nothing left out" while the 300 are still not imported: a
// window-scoped verdict overwriting a container-scoped one, on a screen this
// channel ships no UI of its own for.
func (s *BookOrbitSource) foldInto(obs *DeltaObservation, fold *arrivalsFold) {
	obs.MaxObserved = fold.watermark()
	obs.MinDelivered = fold.minimum()
	obs.UnreadableStamp = fold.unreadable

	s.mu.Lock()
	defer s.mu.Unlock()
	// The run's own fold is kept so ArrivalsMax answers for a delta too.
	s.arrivals = *fold
	for _, t := range s.skips {
		obs.WindowItemsSkipped += t.Total()
	}
	for _, r := range s.residue {
		obs.WindowComicResidue += r.Total()
	}
}

// seedCursor turns a stored cursor position into request 1's filter value, ONCE.
//
// The subtraction happens HERE, in the adapter that minted the format, because
// the adapter is the only code that knows whose clock the value is in. The
// importer reads it from the store, hands it over, and never parses it.
//
// ⚠️ AN UNPARSEABLE STORED VALUE FAILS THE RUN RATHER THAN FALLING BACK TO AN
// UNFILTERED WALK. A cursor position UsArr wrote and cannot read back is a bug
// in UsArr, and the two fallbacks that suggest themselves are both worse than
// stopping: walking unfiltered turns a delta into a silent full read, and
// seeding from UsArr's own clock is the one move that can push a row across the
// boundary permanently.
func seedCursor(seed store.ArrivalsWatermark) (bookorbit.ArrivalsCursor, error) {
	if !seed.Valid {
		// The measured-empty bootstrap. Request 1 carries no filter key at all —
		// not a filter on the zero time, which would be a bound nothing observed.
		return bookorbit.ArrivalsCursor{}, nil
	}
	at, err := seed.Time()
	if err != nil {
		return bookorbit.ArrivalsCursor{}, fmt.Errorf("bookorbit: seed the arrivals walk: %w", err)
	}
	return bookorbit.ArrivalsCursor{
		Value: bookorbit.FormatArrivalsInstant(at.Add(-arrivalsLookbehind)),
		Set:   true,
	}, nil
}
