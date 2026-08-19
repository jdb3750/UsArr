package libsync

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jdb3750/UsArr/internal/store"
)

// The BookOrbit credit pass: how a BookCard's `authors` and `narrators` become
// work_credit rows, and how work.year arrives with them.
//
// # It costs no HTTP at all, which is the whole reason it is affordable
//
// credits.go opens by explaining why the Kavita credit pass is a second pass:
// SeriesDto carries no creator field, so the only endpoint that reports one is
// GET /api/Series/metadata, one series at a time. BookOrbit is the opposite
// case. `authors` and `narrators` are ON THE CARD the item walk already decoded
// (packages/types/src/book.ts:129, :154), so this pass reads a map that
// StreamItems filled and issues no request — and it therefore has no per-item
// failure mode, no circuit breaker to check and no drop to record.
//
// What that costs instead is the buffer: see BookOrbitSource.cards, which is
// where this slice's one real trade-off is written down.
//
// # Two arrays, two roles, no kind branch
//
// kavitaCreditRoles has to branch on work.kind because ONE Kavita array maps to
// two UsArr roles — `writers` is a comic's 'writer' and a book's 'author'.
// BookOrbit needs no branch, and not because the branch was skipped: every work
// this adapter writes is kind 'book' (bookKind, and its comment says why the
// kind is a constant), and each of BookOrbit's two arrays names exactly one
// role in work_credit.role's CHECK. `authors` → 'author'; `narrators` →
// 'narrator'.
//
// ⚠️ THIS IS THE FIRST WRITER OF 'narrator' IN THE SCHEMA. Migration 0007's own
// comment on the role CHECK says "the music seven and illustrator/narrator have
// no v0.1 writer and are here because SQLite cannot ALTER a CHECK" — true when
// it was merged, and this pass is what changes it. The migration is NOT edited
// to say so (a merged migration is never edited); this comment is the record.
//
// # What is NOT written, and why each absence is deliberate
//
//   - edition.narrators, the JSON array ADR-0031 put on `edition` beside
//     duration_seconds and abridged. A narrator is BOTH a credit and an edition
//     property, and only one of the two has a writer today: store.Credit exists
//     and store.FileSet has no Narrators field, so writing the edition half
//     means a new field on the shared write path. The credit is the half that
//     makes a person searchable and links two of their books together, so it is
//     the half worth having first. The other is a field addition on FileSet
//     whenever an edition-grained reader wants it.
//   - work_comic.publisher. BookCard carries `publisher` and this pass could
//     read it, and it must not: the column hangs off work_comic and every work
//     here is a 'book'. LS-18 already files it under the phase-B backfill.
//   - work.status and work_comic.total_issues_declared. CreditSet carries both
//     and they are left INVALID, which is the documented "this source makes no
//     statement": BookOrbit models publication status and a declared volume
//     count on a SERIES, and a series is not what this adapter imports.

// bookOrbitYearFloor and bookOrbitYearCeiling are BookOrbit's OWN validity band
// for a published year, copied rather than invented.
//
// It is enforced upstream in three places that agree:
//
//   - the DATABASE, `book_metadata_published_year_range_chk`:
//     `published_year is null or (published_year >= 1000 and published_year <= 2200)`
//     (server/src/db/schema/metadata.ts:114);
//   - UpdateBookMetadataDto, `@IsOptional() @IsInt() @Min(1000) @Max(2200)`
//     (server/src/modules/book/dto/update-book-metadata.dto.ts:55);
//   - the book-dock DTO, the same decorators
//     (server/src/modules/book-dock/dto/index.ts:94).
//
// UsArr adopts the predicate exactly, for the reason credits.go adopts Kavita's
// IsValidYear floor: the two must not be able to disagree about what a year is.
//
// ⚠️ THE CEILING IS REAL HERE AND IT IS NOT IN THE KAVITA PATH, and the
// asymmetry is a fact about the two sources rather than an inconsistency.
// kavitaMinValidYear's comment refuses an upper bound BECAUSE KAVITA'S OWN
// WRITERS HAVE NONE — "an upper bound here would be UsArr inventing a rule its
// only source does not enforce". BookOrbit does enforce one, in a CHECK
// constraint, so honouring it is that same rule applied and not a departure
// from it. A value outside the band cannot have come from a BookOrbit that is
// storing what it says it stores.
const (
	bookOrbitYearFloor   = 1000
	bookOrbitYearCeiling = 2200
)

// bookOrbitYear projects BookCard.publishedYear onto work.year.
//
// A value outside the band is NO YEAR rather than a clamped one. Clamping would
// store 2200 for a book that said 9999 — a year nobody claimed, rendered by the
// Home screen's Year column as though someone had.
//
// 0 is outside the band and that is how absence arrives: `publishedYear` is
// `number | null` on the wire and internal/bookorbit flattens null to 0, which
// the CHECK above says can never be a stored value.
func bookOrbitYear(year int64) sql.NullInt64 {
	if year < bookOrbitYearFloor || year > bookOrbitYearCeiling {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: year, Valid: true}
}

// bookOrbitCreditRoles is THE mapping, in the order the credits are emitted.
//
// It is a table for kavitaCreditRoles's reason — so that "every array BookCard
// carries is accounted for" is checkable by a test rather than by reading a
// commit message — even though BookOrbit has two arrays where Kavita has
// thirteen. TestEveryBookOrbitPersonArrayIsAccountedFor is that check.
//
// THERE IS NO THIRD ARRAY TO DROP. Kavita's table has five dropped entries
// because Kavita files publishers, imprints, teams, characters and locations in
// the same table as its writers. BookOrbit does not: `publisher` is a plain
// TEXT column on bookMetadata, and genres and tags are their own arrays with
// their own (v1.0) home in the tag system. Neither is a person and neither
// arrives shaped like one.
var bookOrbitCreditRoles = []struct {
	// name is the BookCard field, so the table reads as a mapping.
	name string
	// people extracts the array from a kept card.
	people func(bookOrbitCard) []string
	// role is the work_credit.role member it lands on. There is no per-kind
	// split because there is no per-kind question — see the file header.
	role string
}{
	{
		name:   "authors",
		people: func(c bookOrbitCard) []string { return c.Authors },
		// 'author' and not 'writer'. The two are separate members of the CHECK
		// and the split is by MEDIUM, not by synonym: 'writer' is the comics
		// credit (credits.go's Writer entry carries the argument), and every
		// work this adapter writes is prose.
		role: "author",
	},
	{
		name:   "narrators",
		people: func(c bookOrbitCard) []string { return c.Narrators },
		// See the file header: the first writer of this member.
		role: "narrator",
	},
}

// creditsFromCard projects one kept card onto work_credit rows.
//
// POSITION IS THE ARRAY INDEX, PER ROLE, and unlike Kavita's it is a real
// billing order rather than the order a query happened to return: findCards
// reads both arrays `.orderBy(…displayOrder)` (book.repository.ts:634-639,
// :653-658), which is the column BookOrbit's own UI reorders. Position is part
// of work_credit's PRIMARY KEY, so it is also what lets a book carry two
// authors without them colliding.
func creditsFromCard(c bookOrbitCard) []store.Credit {
	var out []store.Credit
	for _, cr := range bookOrbitCreditRoles {
		pos := 0
		for _, raw := range cr.people(c) {
			name := strings.TrimSpace(raw)
			if name == "" {
				// internal/bookorbit already dropped these; the guard is here
				// because this function must be correct against a card built
				// by hand in a test as well as one off the wire.
				continue
			}
			norm := NormalizeTitle(name)
			if norm == "" {
				// A name that normalises to nothing — punctuation only. It
				// cannot be deduped against anything, so it is not stored.
				// credits.go's rule, and it must stay the same rule: a name
				// this adapter stored and Kavita's dropped would be a person
				// work only one source could ever match.
				continue
			}
			out = append(out, store.Credit{
				Role:           cr.role,
				Name:           name,
				NormalizedName: norm,
				Position:       pos,
				// CreditedAs is left empty. BookOrbit reports one name per
				// author — `select({ name: authors.name })` is the whole
				// projection — and no per-book printed variant, so there is
				// nothing that could differ from the person work's title.
			})
			pos++
		}
	}
	return out
}

// StreamCredits hands fn one CreditSet per requested item whose card the walk
// kept.
//
// NO REQUEST IS ISSUED, so the two conditions that stop the Kavita pass — a
// cancelled context and an open circuit breaker — reduce to one. The context
// check stays because this loop is O(imported items) and an import cancelled
// during it should stop, not finish.
//
// AN EMPTY SET IS DELIVERED, NOT SKIPPED. credits.go's rule and it matters more
// here than there: a book whose last author was removed upstream reaches this
// pass with an empty `authors` array, and skipping it is how a deleted author
// lives forever.
func (s *BookOrbitSource) StreamCredits(
	ctx context.Context, reqs []CreditRequest, fn func(store.CreditSet) error,
) (int, error) {
	var n int
	for _, r := range reqs {
		if err := ctx.Err(); err != nil {
			return n, fmt.Errorf("bookorbit: read credits: %w", err)
		}
		c, ok := s.card(r.RemoteKind, r.RemoteID)
		if !ok {
			// See BookOrbitSource.card: nothing was attempted.
			continue
		}
		n++
		if err := fn(store.CreditSet{
			RemoteKind: r.RemoteKind, RemoteID: r.RemoteID,
			Credits: creditsFromCard(c),

			// THE YEAR, AND IT IS FREE. store.CatalogueItem has no Year field,
			// so bookorbit.go's slice-1 header recorded work.year as deferred
			// "until the pass that has a reason to touch it" — this is that
			// pass. CreditSet already carries Year for Kavita, whose series
			// list reports none and whose per-series metadata read does, and
			// the field's own doc says why it rides a type called CreditSet:
			// phase B is "ONE METADATA READ PER IMPORTED ITEM whose first
			// payload happened to be credits". BookOrbit's read is the item
			// walk itself, and the year came back on it. So no new field, no
			// new pass and no second read — the deferral's condition is met
			// rather than waived.
			Year: bookOrbitYear(c.PublishedYear),

			// Status and TotalDeclared are deliberately absent. See the header.
		}); err != nil {
			return n, err
		}
	}
	return n, nil
}
