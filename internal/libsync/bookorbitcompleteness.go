package libsync

import (
	"context"
	"errors"
	"sort"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// CONTENT-FILTER SHORTFALL DETECTION — "how much of this library can UsArr's
// credential actually see?"
//
// # The defect this closes
//
// A BookOrbit content filter lands in the books LEFT JOIN ON of
// findAllForUser, not in a WHERE, so it shorts each library's bookCount without
// dropping a library row (library.repository.ts:30-51). The consequence is the
// nastiest shape a replica can have: UsArr imports a library, the library
// appears, the counts look plausible, and a slice of the books simply is not
// there — with nothing anywhere saying so. See internal/bookorbit/stats.go for
// the two-select asymmetry that makes the shortfall a subtraction rather than a
// guess.
//
// # Three states, and the third is the point
//
// A boolean would have recreated the defect inside its own fix. "No shortfall"
// and "never checked" must not be the same value, because the check rests on an
// upstream property nobody promised us — `GET /libraries/:id/stats` is
// unguarded TODAY (@RequireLibraryAccess('viewer'), no @RequirePermission) and
// BookOrbit may guard it tomorrow. On the day it does, every probe answers 403,
// and a two-state design would quietly report every library as complete. So:
//
//	complete    — measured, and the two counts agree.
//	shortfall   — measured, and the credential sees fewer than the library holds.
//	unverified  — NOT measured. Says so, and never reads as either of the above.
//
// # What it does NOT cover, stated because the clean half must not be read as
// the whole
//
// ⚠️ THIS IS A BOOK-LEVEL CHECK AND THERE IS A SECOND AXIS IT CANNOT REACH.
// Whether WHOLE LIBRARIES are hidden from UsArr's account is unanswerable
// read-only: membership is `libraries INNER JOIN userLibraryAccess`, and
// LibraryAccessGuard throws an identical `ForbiddenException('No library
// access')` for "the library exists and this account has no access row" and for
// "there is no such library" (common/guards/library-access.guard.ts). Probing
// ids to tell them apart would be enumeration against the operator's own
// service and would still not distinguish the two. So a `complete` verdict on
// every library the credential can SEE says nothing about libraries it cannot,
// and every string this file produces is scoped to the container it names.
//
// # It never refuses
//
// Not one path here can fail an import. A partial replica that says it is
// partial beats no replica: refusing would turn a reporting improvement into an
// outage, and the books that DID import are correct either way.

// The three states are store's vocabulary, not this file's, and that is
// deliberate: internal/store/completeness.go owns them because the verdict has
// THREE sides — measured here, written into sync_report.detail by cmd/usarr,
// read back by the Libraries screen's store read — and sync_report.kind carries
// no CHECK. A second copy of the words is how one of the three ends up spelling
// a state the other two never match.

// ContainerCompleteness is one container's answer.
type ContainerCompleteness struct {
	RemoteID string
	Name     string

	State store.CompletenessState

	// Total is the container's own count of present books, computed upstream
	// with no user and no content filter.
	//
	// ⚠️ IT IS -1, NEVER 0, WHEN State IS unverified. Zero is a
	// legal total for an empty library, so a sentinel that collided with it
	// would put "we did not measure" and "the library is empty" in the same
	// value — which is the exact defect this whole file exists to avoid, one
	// level down.
	Total int64

	// Visible is what UsArr's own credential was shown: the library listing's
	// bookCount, which carries the same `status = 'present'` predicate Total
	// does and additionally carries the account's content filter. It is known in
	// every state, including unverified — the listing is what produced the
	// container in the first place.
	Visible int64

	// Reason is why State is unverified, in UsArr's own words. It is
	// empty in the other two states.
	//
	// ⚠️ NEVER UPSTREAM TEXT. It is written into sync_report.detail, which
	// reference/security.md §5 keeps free of upstream response bodies, and it
	// reaches a browser from there. The upstream's own error is in the process
	// log, where the operator can read it and a browser cannot.
	Reason string
}

// Hidden is how many present books the content filter kept from UsArr.
//
// It is 0 in every state but a shortfall, and a caller must therefore
// read State first: 0 here means "none hidden" only once State says the
// question was answered.
func (c ContainerCompleteness) Hidden() int64 {
	if c.State != store.CompletenessShortfall {
		return 0
	}
	return c.Total - c.Visible
}

// checkCompleteness probes every container the listing returned and records one
// verdict each.
//
// ONE EXTRA REQUEST PER LIBRARY PER IMPORT, on the import path and nowhere near
// a render (principle 1). It runs from Containers rather than from StreamItems
// because bookCount — the "visible" half of the subtraction — arrives on the
// library listing and nowhere else, and because a container list that binds
// without ever being graded is the state this check exists to prevent.
//
// EVERY CONTAINER GETS A ROW, INCLUDING THE ONES THAT ARE FINE. That is the
// difference between this and Skipped(), which reports only non-zero tallies:
// a skip that is absent means nothing was skipped, whereas a completeness
// verdict that is absent means nothing was ASKED, and those two absences have to
// look different downstream.
func (s *BookOrbitSource) checkCompleteness(ctx context.Context, libs []bookorbit.Library) {
	if s.Client == nil {
		return
	}
	out := make(map[string]ContainerCompleteness, len(libs))
	for _, l := range libs {
		out[containerRef(l.ID)] = s.completenessOf(ctx, l)
	}
	s.mu.Lock()
	s.completeness = out
	s.mu.Unlock()
}

// completenessOf is one container's probe, and it cannot return an error.
func (s *BookOrbitSource) completenessOf(ctx context.Context, l bookorbit.Library) ContainerCompleteness {
	c := ContainerCompleteness{
		RemoteID: containerRef(l.ID),
		Name:     l.Name,
		Visible:  l.BookCount,
		Total:    -1,
	}

	stats, err := s.Client.LibraryStats(ctx, l.ID)
	if err != nil {
		c.State = store.CompletenessUnverified
		// ⚠️ THE NAMED DEGRADATION CONDITION, AND IT IS A REASON RATHER THAN A
		// FAILURE. The 403 arm is the guard-later scenario ADR-XXXX records:
		// this check rests on GET /libraries/:id/stats carrying no
		// @RequirePermission, which is a property of BookOrbit's source at
		// 73b7877d and not a promise to UsArr. The other arm covers everything
		// else — a timeout, an open breaker, a route that has moved, a body that
		// is not a count.
		c.Reason = completenessReason(err)
		s.log().Warn("bookorbit: could not check whether this credential sees every book in a library",
			"library_id", c.RemoteID, "library", l.Name,
			"visible_books", l.BookCount,
			"effect", "the import proceeds and the library is recorded as unverified, never as complete",
			"err", err)
		return c
	}

	c.Total = stats.TotalBooks
	switch {
	case stats.TotalBooks > l.BookCount:
		c.State = store.CompletenessShortfall
		s.log().Warn("bookorbit: a content filter is hiding books from UsArr's credential",
			"library_id", c.RemoteID, "library", l.Name,
			"total_books", stats.TotalBooks, "visible_books", l.BookCount,
			"hidden_books", stats.TotalBooks-l.BookCount,
			"fix", "the filter is on the BookOrbit account UsArr uses; widen it there and re-import")
	case stats.TotalBooks == l.BookCount:
		c.State = store.CompletenessComplete
	default:
		// ⚠️ FEWER BOOKS IN THE UNFILTERED TOTAL THAN IN THE FILTERED LISTING IS
		// NOT AN ANSWER, AND IT IS DELIBERATELY NOT ROUNDED UP TO `complete`.
		// The two counts are two statements against a live table, so a book
		// deleted between them produces this ordinarily and harmlessly — but so
		// would a getStats that had quietly stopped meaning what it means, and
		// this side cannot tell those apart. `unverified` is the honest answer
		// to a pair of numbers that cannot both be right, and it is the safe one
		// because it never reads as complete.
		c.State = store.CompletenessUnverified
		c.Total = -1
		c.Reason = "the two counts disagreed in the direction only a moving table explains"
	}
	return c
}

// completenessReason turns one probe failure into UsArr's own words.
//
// The 403 arm is separated from the rest because the OPERATOR'S NEXT ACTION
// differs: a permission guard on the stats route is a BookOrbit change nobody
// can configure around, and it is the scenario ADR-XXXX names, whereas a
// timeout or a 500 is a service problem the Services screen already covers.
func completenessReason(err error) string {
	switch {
	case errors.Is(err, bookorbit.ErrForbidden), errors.Is(err, bookorbit.ErrUnauthorized):
		// ⚠️ EVERY ONE OF THESE IS RENDERED IN A TABLE CELL, so they are one
		// short clause rather than a paragraph — §9.1's overflow policy is a
		// wrap, and a three-line cell is a row nobody scans. What is missing
		// from them is deliberate: no status code, no route, no upstream text.
		return "BookOrbit refused UsArr the statistics this check compares against"
	case errors.Is(err, bookorbit.ErrNotFound):
		return "BookOrbit has no statistics route for this library"
	default:
		return "the statistics this check compares against could not be read"
	}
}

// Completeness reports, per container, how much of it UsArr's credential could
// see — INCLUDING the containers that were fine and the ones that could not be
// checked.
//
// ⚠️ IT IS THE ONLY PLACE THE ANSWER EXISTS IN THIS PROCESS, on Skipped()'s
// reasoning: cmd/usarr writes it into sync_report after the import, because a
// Report the caller forgets to read and a log line nobody greps are the same
// thing as silence.
//
// An EMPTY slice means the check never ran — Containers() was never called, or
// the credential can see no library at all. It does not mean everything is
// complete, and nothing downstream may read it that way.
//
// Sorted by container id so two runs over the same instance produce the same
// order.
func (s *BookOrbitSource) Completeness() []ContainerCompleteness {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]ContainerCompleteness, 0, len(s.completeness))
	for _, c := range s.completeness {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RemoteID < out[j].RemoteID })
	return out
}
