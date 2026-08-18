package libsync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// The Kavita credit adapter (ADR-0044): how a SeriesMetadataDto becomes
// work_credit rows and the 'person' works they point at.
//
// # Why this is a second pass and not a field on CatalogueItem
//
// SeriesDto — what POST /api/Series/all-v2 streams — carries NO creator field.
// The only endpoint that reports one is GET /api/Series/metadata?seriesId=N, one
// series at a time, and the three bulk-shaped alternatives were checked against
// the vendored api/specs/kavita-develop.json and none of them can rebuild the
// series→creator mapping:
//
//   - POST /api/Person/all returns BrowsePersonDto with no series linkage;
//   - GET /api/Person/series-known-for is documented "the top 20 series";
//   - GET /api/Person/chapters-by-role is documented "Limited to 20 results".
//
// So the cost is one GET per series, and reference/sync.md §2 already settled
// that shape for the *Arrs — "the rule is not 'never fetch children
// per-parent' — it is 'fetch them per parent, bounded'". Bounded here means:
// after the item stream has closed (never inside its callback, which would hold
// the streaming connection open across N round trips), one at a time, and with
// a per-series failure dropped rather than fatal.

// CreditRequest names one already-imported item to fetch credits for.
//
// Kind travels with it because ONE KAVITA ARRAY MAPS TO TWO UsArr ROLES: a
// PersonDto in `writers` is a comic's `writer` and a book's `author`. The kind
// decision was made once at ingest from the container (ARCHITECTURE §6.4) and is
// carried here rather than re-derived, so the two passes cannot disagree about
// it.
type CreditRequest struct {
	RemoteKind string
	RemoteID   string
	Kind       string
}

// CreditSource is the OPTIONAL half of Source.
//
// It is a separate interface rather than a method on Source because not every
// catalogue source reports credits, and a Source that had to implement a
// no-op StreamCredits would be a Source whose author had to decide what a no-op
// means. The importer type-asserts for it and skips the whole phase when a
// source does not implement it — which is principle 3's "degrade honestly"
// applied to an adapter rather than to a service.
type CreditSource interface {
	// StreamCredits hands fn one CreditSet per item it could read, and returns
	// HOW MANY IT HANDED OVER. Like StreamItems it reports that count on failure
	// too: the calls to fn happened and their effects stand.
	StreamCredits(ctx context.Context, reqs []CreditRequest, fn func(store.CreditSet) error) (int, error)
}

// creditRole is one of Kavita's person arrays and where it lands.
type creditRole struct {
	// name is the SeriesMetadataDto field, for the mapping table below to be
	// readable as a mapping.
	name string

	// people extracts the array from a metadata DTO.
	people func(kavita.SeriesMetadataDto) []kavita.PersonDto

	// comicRole and bookRole are the work_credit.role each array maps to, per
	// work.kind. An empty string means "not credited for this kind".
	comicRole string
	bookRole  string
}

// kavitaCreditRoles is THE mapping, and EVERY ONE of Kavita's thirteen
// PersonRole members is accounted for — the eight that map and the five that do
// not, with the reason each is dropped written where the drop happens rather
// than in a document.
//
// The mapping runs off the ARRAY NAME, not off PersonDto.roles, and that is a
// correctness point rather than a convenience. A PersonDto's `roles` array
// carries every role that person holds ACROSS THE WHOLE INSTANCE: a person who
// writes one series and colors another comes back with {Writer, Colorist} in
// both series' metadata. Reading `roles` would therefore credit a series'
// writer as its colorist. The array a PersonDto arrives in is the only
// series-scoped statement Kavita makes.
//
// TestEveryKavitaPersonRoleIsAccountedFor fails if the vendored spec grows a
// fourteenth member.
var kavitaCreditRoles = []creditRole{
	// ── The eight that become credits ───────────────────────────────────────
	{
		// PersonRole.Writer (3). THE ONE THAT SPLITS BY KIND, and the single
		// most consequential line in this table: Kavita has NO `authors` array.
		// Its Writer role is where a book's author lands — it is what an EPUB's
		// dc:creator and a ComicInfo `Writer` element both parse into — so a
		// book library's writers ARE its authors and mapping them to 'writer'
		// would leave work_credit.role's 'author' member with no writer at all
		// while filing every novelist under a comics role.
		name:      "writers",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Writers },
		comicRole: "writer",
		bookRole:  "author",
	},
	{
		name:      "coverArtists",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.CoverArtists },
		comicRole: "cover_artist",
		// A book's cover artist is a cover artist, not an 'illustrator':
		// 'illustrator' means the person who drew the interior art, which is a
		// different credit and one Kavita has no array for.
		bookRole: "cover_artist",
	},
	{
		name:      "pencillers",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Pencillers },
		comicRole: "penciller",
		// A penciller on a BOOK library is Kavita letting a comics field
		// through on prose. It is still a penciller — the role is what the
		// upstream said, and inventing 'illustrator' here would be UsArr
		// deciding what the operator meant.
		bookRole: "penciller",
	},
	{
		name:      "inkers",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Inkers },
		comicRole: "inker",
		bookRole:  "inker",
	},
	{
		name:      "colorists",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Colorists },
		comicRole: "colorist",
		bookRole:  "colorist",
	},
	{
		name:      "letterers",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Letterers },
		comicRole: "letterer",
		bookRole:  "letterer",
	},
	{
		name:      "editors",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Editors },
		comicRole: "editor",
		bookRole:  "editor",
	},
	{
		name:      "translators",
		people:    func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Translators },
		comicRole: "translator",
		bookRole:  "translator",
	},

	// ── The five that are DROPPED, each with its reason ──────────────────────
	//
	// All five are declared here with empty roles rather than simply omitted,
	// so that "accounted for" is checkable by a test rather than by reading a
	// commit message. Three are ORGANISATIONS and two are not people at all;
	// Kavita stores all five in the same table as its writers, and UsArr does
	// not inherit that conflation.
	{
		// PersonRole.Publisher (10). An organisation, not a creator, and
		// work_credit.role has no member for one. It DOES have a home in the
		// schema — work_comic.publisher, a TEXT column migration 0006 created
		// and nothing writes — and this pass deliberately does not write it:
		// that column belongs to the phase-B metadata backfill that also owns
		// SeriesMetadataDto's summary, which is a different change. Recorded in
		// docs/REVIEW-LOG.md as LS-18. (`releaseYear` was named here too and is
		// no longer owed: it lands on work.year via releaseYearOf below —
		// REVIEW-LOG.md LS-110.)
		name:   "publishers",
		people: func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Publishers },
	},
	{
		// PersonRole.Imprint (13). A publisher's sub-brand — Vertigo under DC.
		// An organisation, and one with no column anywhere in the schema.
		name:   "imprints",
		people: func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Imprints },
	},
	{
		// PersonRole.Team (14). A creative studio or a fictional super-team,
		// depending on who filled in the ComicInfo. Either way not a credited
		// human, and the ambiguity is itself a reason not to guess.
		name:   "teams",
		people: func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Teams },
	},
	{
		// PersonRole.Character (11). FICTIONAL. A character is a subject of the
		// work, not a creator of it, and crediting Batman as a person who made
		// Batman is the kind of row that is very hard to delete later. Its real
		// home is the tag system (schema.md §8), which is v1.0.
		name:   "characters",
		people: func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Characters },
	},
	{
		// PersonRole.Location (15). A place. Same argument as Character, and
		// the same eventual home.
		name:   "locations",
		people: func(m kavita.SeriesMetadataDto) []kavita.PersonDto { return m.Locations },
	},
}

// creditsFromMetadata projects one SeriesMetadataDto onto work_credit rows.
//
// POSITION IS THE ARRAY INDEX, per role, which is the only billing order Kavita
// states. It is not a quality claim — Kavita returns the array in whatever order
// its own query produced — but it is stable for a given metadata state, so
// re-importing an unchanged series rewrites identical rows.
func creditsFromMetadata(md kavita.SeriesMetadataDto, kind string) []store.Credit {
	var out []store.Credit
	for _, cr := range kavitaCreditRoles {
		role := cr.comicRole
		if kind == "book" {
			role = cr.bookRole
		}
		if role == "" {
			// A dropped array. See the table.
			continue
		}
		pos := 0
		for _, p := range cr.people(md) {
			name := strings.TrimSpace(p.Name)
			if name == "" {
				continue
			}
			norm := NormalizeTitle(name)
			if norm == "" {
				// A name that normalises to nothing — punctuation only. It
				// cannot be deduped against anything, so it is not stored.
				continue
			}
			out = append(out, store.Credit{
				Role:           role,
				Name:           name,
				NormalizedName: norm,
				Position:       pos,
				// CreditedAs is left empty: Kavita reports ONE name per person
				// and no per-series printed variant, so there is nothing that
				// could differ from the person work's title.
			})
			pos++
		}
	}
	return out
}

// kavitaMinValidYear is Kavita's OWN validity floor for a year, copied rather
// than invented: `NumberHelper.IsValidYear(int number) => number is >= 1000;`
// (Kavita.Common/Helpers/NumberHelper.cs:7, `develop` at 9c3e540).
//
// UsArr adopts that predicate exactly instead of picking its own band, because
// the two must not be able to disagree about what a year is. Kavita applies it
// in both of the places that can write the field:
//
//   - the SCANNER — `series.Metadata.ReleaseYear = chapters.MinimumReleaseYear()`
//     (Kavita.Services/Scanner/ProcessSeries.cs:380-382), and MinimumReleaseYear
//     is `chapters.Select(v => v.ReleaseDate.Year).Where(NumberHelper.IsValidYear)
//     .DefaultIfEmpty().Min()` (Kavita.Services/Extensions/ChapterListExtensions.cs:51-54)
//     — so a series whose chapters carry no usable date yields DefaultIfEmpty()'s **0**;
//   - the EDIT-SERIES dialog — `if (NumberHelper.IsValidYear(...))`
//     (Kavita.Services/SeriesService.cs:102), which refuses a value below the
//     floor and leaves the stored one alone.
//
// So 0 IS KAVITA'S OWN SENTINEL FOR "no year", not a year, and Kavita reads it
// back as one everywhere it renders the field: `.Where(sm => sm.ReleaseYear != 0)`
// (Kavita.Services/StatisticService.cs:111) and
// `item.Series.Metadata?.ReleaseYear is > 0 ? …`
// (Kavita.Services/ReadingLists/CblExportService.cs:166).
//
// THERE IS DELIBERATELY NO UPPER BOUND, and that is a decision rather than an
// omission. Kavita's own writers have none — the field is a bare int32 in the
// vendored spec, with no `maximum` — so an upper bound here would be UsArr
// inventing a rule its only source does not enforce, and it would silently drop
// a year an operator typed on purpose. A wrong-but-plausible year is a DATA
// problem the operator can see and fix upstream; 0 is a SENTINEL problem the
// operator cannot see at all, because it renders as a year that was never
// claimed. Only the second is UsArr's to fix. REVIEW-LOG.md LS-111.
const kavitaMinValidYear = 1000

// releaseYearOf projects SeriesMetadataDto.releaseYear onto work.year.
//
// work.year and Kavita's releaseYear ARE the same notion, which was checked
// before this was written rather than assumed: ARCHITECTURE.md §6.4 amendment 3
// names these two values as the ONE value a cross-source match compares —
// "strip a trailing parenthesised 4-digit year from a Komga series title into
// `year` … or Saga (2012) never matches Kavita's Saga + releaseYear 2012".
// Kavita computes it as the earliest VALID release year across the series'
// chapters (ProcessSeries.cs:382 above), i.e. the year the series started,
// which is what work.year on a 'comic' or 'book' series row holds.
func releaseYearOf(md kavita.SeriesMetadataDto) sql.NullInt64 {
	if md.ReleaseYear < kavitaMinValidYear {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(md.ReleaseYear), Valid: true}
}

// StreamCredits fetches each requested series' metadata and hands its credits to
// fn.
//
// ONE GET PER SERIES, SEQUENTIALLY. sync.md §2 budgets 4–8 concurrent for the
// *Arr episode fetch; this stays at one, deliberately, because the two calls are
// not comparable: that one is the dominant cost of a TV first import and this
// one runs after a Kavita import whose whole item read is a SINGLE response.
// Concurrency here would buy latency on a phase-B pass nobody is waiting on, at
// the cost of a token bucket, a jitter policy and a fan-out this package does
// not otherwise have. If a real library measures slow, the seam is this loop and
// nothing above it changes.
//
// A PER-SERIES FAILURE IS DROPPED, NOT FATAL. A series whose metadata 404s (it
// was deleted between the two passes) or 500s must not cost every later series
// its credits. Two conditions do stop the pass, because continuing past either
// would mean issuing hundreds of calls that cannot succeed: a cancelled context,
// and an open circuit breaker.
func (s *KavitaSource) StreamCredits(
	ctx context.Context, reqs []CreditRequest, fn func(store.CreditSet) error,
) (int, error) {
	var n int
	for _, r := range reqs {
		if err := ctx.Err(); err != nil {
			return n, fmt.Errorf("kavita: read credits: %w", err)
		}
		id, err := strconv.ParseInt(r.RemoteID, 10, 32)
		if err != nil {
			// A remote id this adapter did not write. Skipped rather than
			// errored: it cannot be a Kavita series id.
			continue
		}
		md, err := s.Client.SeriesMetadata(ctx, int32(id))
		if err != nil {
			if errors.Is(err, kavita.ErrBreakerOpen) {
				return n, fmt.Errorf("kavita: read credits for series %s: %w", r.RemoteID, err)
			}
			// Dropped. The series keeps whatever credits it already had, which
			// for a first import is none.
			continue
		}
		credits := creditsFromMetadata(md, r.Kind)
		if len(credits) == 0 {
			// NOT SKIPPED. An empty set is a real statement — "this series has
			// no credits" — and it must reach the store so that credits removed
			// upstream are removed here. Skipping it is how a deleted penciller
			// lives forever.
			credits = nil
		}
		n++
		if err := fn(store.CreditSet{
			RemoteKind: r.RemoteKind, RemoteID: r.RemoteID, Credits: credits,
			// The same response, on the same round trip. See releaseYearOf.
			Year: releaseYearOf(md),
		}); err != nil {
			return n, err
		}
	}
	return n, nil
}
