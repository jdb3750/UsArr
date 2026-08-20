package libsync

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// The BookOrbit catalogue adapter — prose end to end with its credits, its files
// and its year, and comics as ISSUES UNDER SERIES.
//
// ADR-0052 makes BookOrbit v0.1's catalogue source. This file is the second half
// of slice 1 (internal/bookorbit/catalogue.go is the first): it turns two
// BookOrbit reads into store.CatalogueContainer and store.CatalogueItem values
// and hands them to the same channel-1 importer the Kavita adapter feeds. Read
// kavita.go first; this then reads as a translation rather than an invention.
//
// # What this adapter does, and the two things it deliberately does not
//
// DOES: list the libraries the credential can see, walk each one's books page by
// page, and write ONE work of kind 'book' per PROSE book — an ebook or an
// audiobook, which are one kind differing only in edition.format (ADR-0031) —
// plus ONE work of kind 'comic_issue' per COMIC book, under a 'comic' series
// work (ADR-0068). A BookOrbit comic is one file, so it is an issue; the series
// is either the one BookCard.seriesId names or one synthesized for a comic that
// has none. See mapComic.
// The three passes are here and the second and third cost NO extra HTTP:
// StreamItems (this file), StreamCredits (bookorbitcredits.go) and StreamFiles
// (bookorbitfiles.go), the latter two served from cards this walk kept.
//
// ⚠️ THIS HEADER LISTED FOUR THINGS THE ADAPTER DID NOT DO AND TWO OF THEM HAVE
// LANDED, which is worth recording rather than deleting because the reasons
// given were the right reasons and they were discharged rather than waived:
//
//   - *"CREDITS AND EDITIONS … it is the slice that decides how much of a page
//     is buffered"*. It decided: see BookOrbitSource.cards, which keeps one
//     small struct per MAPPED book and states what that costs against
//     streamAndApply's "ONE UNBOUNDED ALLOCATION IN THE IMPORT". Nothing buffers
//     a page.
//   - *"work.year … store.CatalogueItem has no Year field … it belongs with the
//     pass that has a reason to touch it"*. That pass is the credit pass, and
//     store.CreditSet has carried Year since Kavita's. No field was added to
//     CatalogueItem and no read was added anywhere.
//
// ⚠️ THIS LIST'S THIRD ENTRY WAS *"COMICS. A comic-format book is SKIPPED AND
// COUNTED, never guessed at. The unit-of-work question for comics is open —
// BookOrbit's series have no library and a book can belong to several of them, so
// the series work a comic would hang under has no container to bind to"*.
// ADR-0068 answers both halves and the entry is discharged rather than waived.
// The container question is answered on UsArr's side (ADR-0066 decision 5: the
// `comic` library is minted over the same `library_source` container ref), and
// the several-series question is answered by binding on BookOrbit's own
// maintained primary and RECORDING the rest. The §6.4 caution that made the
// entry right stands: that is why the parent binding rests on a measurement of
// BookOrbit's source rather than on the shape of its JSON.
//
// STILL DOES NOT:
//
//  1. CREDITS, FILES OR COVERS FOR A COMIC. A comic gets no card kept, so all
//     three later passes miss it and none of them issues a request for it. That
//     is ADR-0068's own budget — "the allowlist widens by four fields, and that is
//     the acquisition cost in full … Zero extra HTTP" — and the cover pass is the
//     one that makes it load-bearing rather than tidy: it issues one HTTP request
//     per item it is handed, and ARCHITECTURE §13 sizes the comic side at ~90,000
//     issues.
//  2. CHANNEL 3b AND CHANNEL 4. No delta walk, no reconciliation sweep, and no
//     per-book detail read. And STILL no migration: every column the three
//     passes write already exists in 00005_library_sync.sql and
//     00007_work_credit.sql.
//
//     ⚠️ THIS CLAUSE ALSO READ *"no cover fetch"* AND THAT IS NOW FALSE OF AN
//     IMPORT, though still true of this file. It was accurate when written:
//     nothing anywhere fetched a cover. covers.go is the pass that changed it —
//     phase D of FullImport calls internal/imagepipeline once per imported book,
//     between committed batches, against the same *bookorbit.Client this adapter
//     holds. THE ROUTE DOES NOT COME THROUGH HERE: the pipeline takes the client
//     directly, so BookOrbitReader gained no method and this adapter's three
//     passes still cost exactly the two HTTP reads they always did. What the
//     sentence was really promising — that mapping a book does not drag an image
//     fetch into the item stream — is still kept, and covers.go's header is where
//     the reason lives.

// BookOrbitReader is the slice of *bookorbit.Client this adapter uses.
//
// It is an interface for KavitaReader's two reasons: the mapping can be tested
// against hand-built books with no HTTP round trip, and nothing here can reach a
// method that bypasses the injected SSRF-policy client.
//
// Authenticate is ON THIS INTERFACE and that is not incidental — it is the §14
// gate. See gate().
type BookOrbitReader interface {
	// Authenticate mints the access token if none is held and returns the §14
	// scope verdict for the account behind it. It costs no request of its own
	// beyond the mint: the login response carries permissions, isSuperuser and
	// provisioningMethod in the same body as the accessToken.
	Authenticate(ctx context.Context) (bookorbit.ScopeVerdict, error)

	// Libraries is GET /api/v1/libraries.
	Libraries(ctx context.Context) ([]bookorbit.Library, error)

	// StreamBooks is the paged walk of POST /api/v1/libraries/{id}/books. It is
	// on this interface rather than a second one for the reason
	// KavitaReader.SeriesMetadata is on that one: a BookOrbit client that could
	// list libraries but not read the books in them does not exist — same
	// controller, same credential.
	StreamBooks(ctx context.Context, libraryID int64, fn func(bookorbit.Book) error) (bookorbit.BookPage, error)

	// LibraryStats is GET /api/v1/libraries/{id}/stats — the unfiltered
	// present-book count this adapter subtracts the credential's own bookCount
	// from. It is on this interface even though it reads NO CATALOGUE DATA,
	// because the completeness pass that calls it must be testable against a
	// hand-built reader like everything else here, and because a client that
	// could not be asked would make the check untestable rather than optional.
	//
	// ⚠️ IT IS THE ONE METHOD HERE WHOSE ROUTE MAY BE GUARDED LATER. See
	// bookorbitcompleteness.go: an error from it is a verdict of `unverified`
	// and never a failed import.
	LibraryStats(ctx context.Context, libraryID int64) (bookorbit.LibraryStats, error)
}

// SkipTally is what one container's walk declined to map, by reason.
//
// IT IS A COUNT AND NOT A LIST on purpose: a list of every skipped book in a
// 20,000-comic library is a second unbounded allocation, and the question a user
// asks is "how many of my books did UsArr not take", which a count answers.
type SkipTally struct {
	// Comics is books whose primary file is cbz, cbr, cb7 or cbx.
	//
	// ⚠️ ITS EXPECTED VALUE IS NOW 0, AND THE FIELD IS KEPT DELIBERATELY.
	// ADR-0068 imports comics, so nothing increments this any more; the ADR's own
	// consequences say the field stays and reading 0 is the fourth of its four
	// done-checks. Removing it would erase the history: rows written by every
	// import before that ADR carry a `skipped_comics` key, and store.SkipNote's
	// JSON contract is explicit that renaming or dropping one "silently orphans
	// that half of the history".
	Comics int

	// Unknown is books BookOrbit itself classifies as media kind "unknown" —
	// getBookMediaKind's answer for an absent or blank format, INCLUDING a book
	// with no files at all. It is counted separately from Comics because the two
	// need different answers from an operator: a comic is UsArr's gap, an
	// unknown is usually a book whose file BookOrbit has not classified yet
	// (status 'processing') or a folder with nothing in it.
	Unknown int
}

// Total is everything this container's walk read and did not map.
func (t SkipTally) Total() int { return t.Comics + t.Unknown }

// ContainerSkips names one container's tally.
type ContainerSkips struct {
	RemoteID string
	Name     string
	SkipTally
}

// ComicResidue is what one container's comics fell back on, by residue case.
//
// ⚠️ IT EXISTS SO THE FIRST REAL IMPORT MEASURES THE TWO DEFAULTS ADR-0068
// DECISION 4 NAMES, rather than so anyone estimates them. Both cases are legal,
// both are silent, and neither is visible on any screen — which is precisely the
// shape a count has to carry, on the argument ADR-0063 already made for the skip
// tally: an unmeasured fallback is indistinguishable from one that never fired.
//
// IT IS COUNTS PLUS A BOUNDED SAMPLE, not a list. A 20,000-comic library whose
// operator never matched a series would put 20,000 entries in one sync_report
// row; the cover pass already refused that shape in terms — "a row each would be
// tens of thousands of rows carrying the same sentence. The counts are what an
// operator needs, and they fit in one row".
type ComicResidue struct {
	// SynthesizedSeries is comics whose card reported NO series at all, each of
	// which was ingested as an issue under a series synthesized for it, with
	// work_comic_issue.is_oneshot written to 1 (ADR-0068 decision 2).
	SynthesizedSeries int

	// MultiSeries is comics that belong to MORE THAN ONE series upstream, and
	// ExtraMemberships is how many memberships beyond the primary those carried
	// in total. The parent binding used the scalar seriesId and nothing else;
	// these are what was DECLINED, not what was resolved (ADR-0068 decision 3).
	MultiSeries      int
	ExtraMemberships int

	// Sample is up to comicResidueSampleCap of the declined memberships, as
	// NUMERIC IDS ONLY.
	//
	// ⚠️ NO UPSTREAM TEXT. reference/security.md §5 keeps upstream response
	// bodies out of sync_report.detail, and store.SkipNote states the rule for
	// this column in terms — "EVERY STRING IN IT IS USARR'S OWN PROSE". A series
	// NAME is upstream text; a series id is a number, so the sample carries ids
	// and the operator joins them upstream if he wants names.
	Sample []DeclinedMembership
}

// DeclinedMembership is one series a comic belongs to that UsArr did not bind
// it to.
type DeclinedMembership struct {
	// BookID is the BookOrbit book id, and SeriesIDs are the series ids beyond
	// the primary. The primary is deliberately absent: it is not declined, it is
	// the parent.
	BookID    int64   `json:"book_id"`
	SeriesIDs []int64 `json:"series_ids"`
}

// comicResidueSampleCap bounds ComicResidue.Sample.
//
// It is small on purpose. The sample answers "what does one of these actually
// look like" for an operator who has only sync_report in front of him; the
// COUNTS answer "how often", which is the question ADR-0068 decision 4 asks. A
// bigger cap would trade an unbounded row for a marginally better anecdote.
const comicResidueSampleCap = 25

// Total is every comic in this container that took a residue default.
func (r ComicResidue) Total() int { return r.SynthesizedSeries + r.MultiSeries }

// ContainerComics names one container's residue.
type ContainerComics struct {
	RemoteID string
	Name     string
	ComicResidue
}

// BookOrbitSource adapts one BookOrbit instance to Source.
type BookOrbitSource struct {
	Client BookOrbitReader

	// Log is optional and defaults to discard. It is where the §14 scope verdict
	// and the per-container skip counts go.
	Log *slog.Logger

	// mu guards containers and skips. StreamItems is called from one goroutine
	// by the importer, but Skipped() is read by the caller afterwards and the
	// scope gate is a sync.Once — a mutex here costs nothing and removes the
	// question.
	mu sync.Mutex

	// containers remembers the library list between Containers() and
	// StreamItems(), because the book walk is PER LIBRARY: BookCard carries no
	// libraryId (BookDetail does; the card does not), so the container an item
	// belongs to is the route parameter and nothing else. That is also why this
	// adapter cannot use the global POST /books/query — a global walk could not
	// file anything, since store.ApplyCatalogueBatch matches
	// CatalogueItem.ContainerID against the binding map.
	containers []bookorbit.Library

	// skips is the per-container tally, keyed by the container's remote id.
	skips map[string]*SkipTally

	// residue is the per-container comic residue, keyed the same way and
	// populated by the same rule: an entry exists exactly for a container the
	// walk REACHED (see Skipped, and ADR-0063 for why absence must mean "not
	// looked" rather than "nothing found").
	residue map[string]*ComicResidue

	// completeness is the per-container content-filter verdict, keyed by the
	// container's remote id and written once by checkCompleteness.
	//
	// ⚠️ NIL AND EMPTY BOTH MEAN "NOT CHECKED", NEVER "ALL COMPLETE". Every
	// container that WAS checked has an entry, including the ones that were fine
	// — see bookorbitcompleteness.go for why the two absences must not collapse.
	completeness map[string]ContainerCompleteness

	// cards is what the item walk KEEPS BACK for the two passes that run after
	// it — the credits, the year and the files, all of which ride the card the
	// walk already decoded. It is keyed by the book's remote id, the same string
	// CreditRequest.RemoteID and FileRequest.RemoteID carry.
	//
	// ⚠️ THIS MAP IS THE DECISION THIS SLICE EXISTS TO MAKE, and the cost is
	// named rather than absorbed. streamAndApply calls its []ImportedItem "THE
	// ONE UNBOUNDED ALLOCATION IN THE IMPORT" and budgets it at three short
	// strings per item; this is a SECOND allocation of the same order, one entry
	// per MAPPED book, holding a handful of names and a file reference each. It
	// is not a new order of growth, and it does not scale with page size — a
	// page is decoded, mapped and released exactly as before.
	//
	// THE ALTERNATIVE WAS TO RE-WALK, AND IT IS WORSE ON EVERY AXIS. BookOrbit
	// has no per-book batch route (bookOrbitExternalIDs's header says so of the
	// detail read), so a credit pass that did not keep the card would either
	// issue one GET per book or replay the whole paged walk — trading a bounded
	// allocation for N round trips against a live upstream, on data UsArr
	// already had in hand and threw away. The seam if a library ever measures
	// too large for this is the map itself: it is written in exactly one place
	// and read in exactly two.
	//
	// ⚠️ A COMIC AND AN UNKNOWN GET NO ENTRY, AND THE TWO NOW HAVE DIFFERENT
	// REASONS. An unknown is skipped by StreamItems, so no work exists for its
	// credits to hang on. A COMIC IS IMPORTED and does have a work — it is left
	// out on ADR-0068's budget instead: the three passes this map feeds are
	// driven by `imported`, the cover pass among them issues one HTTP request per
	// item, and the ADR's acquisition cost is "Zero extra HTTP". The importer
	// drops child items from `imported` to match, so a comic reaches none of the
	// three and this map is never asked for one.
	cards map[string]bookOrbitCard

	// gateOnce makes the §14 consultation happen exactly once per source,
	// whichever catalogue read comes first.
	gateOnce sync.Once
	gateErr  error
}

// bookOrbitCard is the slice of one BookCard that the item pass keeps for the
// credit and file passes.
//
// IT IS A PROJECTION OF A PROJECTION, and holding bookorbit.Book whole would be
// the easier and wrong choice: the card also carries the title, the subtitle,
// the identifiers and two timestamps, every one of which the item pass has
// ALREADY WRITTEN. Keeping them would double the buffer to carry values nobody
// reads again.
type bookOrbitCard struct {
	// Authors and Narrators are the card's arrays, already trimmed and
	// blank-free by internal/bookorbit. They are the credit pass's whole input.
	Authors   []string
	Narrators []string

	// PublishedYear is BookCard.publishedYear, 0 for absent. It rides here
	// rather than on store.CatalogueItem because CatalogueItem has no Year field
	// and store.CreditSet does — see bookorbitcredits.go.
	PublishedYear int64

	// Files is the card's file references. The file pass filters them by role
	// and never re-reads them.
	Files []bookorbit.BookFile
}

// NewBookOrbitSource wraps a client.
func NewBookOrbitSource(c BookOrbitReader) *BookOrbitSource {
	return &BookOrbitSource{
		Client:  c,
		skips:   map[string]*SkipTally{},
		residue: map[string]*ComicResidue{},
		cards:   map[string]bookOrbitCard{},
	}
}

func (s *BookOrbitSource) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.DiscardHandler)
}

// gate is ADR-0052's §14 credential-scope gate, honoured at the FIRST CATALOGUE
// READ.
//
// # What the gate says, and where it therefore belongs
//
// ADR-0052: "the BookOrbit adapter may not read a catalogue under a
// shared-account credential until the scope that account grants has been
// enumerated against §14". ADR-0058 discharges the enumeration in code
// (internal/bookorbit/scope.go) and is explicit that "what ships is the
// mechanism, not the gate's enforcement — ADR-0052's condition is on the
// catalogue read, so the first StreamItems in slice 1 must consult
// ScopeVerdict.Elevated()".
//
// 🚩 IT IS CALLED FROM Containers AS WELL AS FROM StreamItems, AND THAT IS A
// DELIBERATE WIDENING OF THE LETTER OF ADR-0058's SENTENCE. Containers()
// performs GET /api/v1/libraries, which runs FIRST and is itself a catalogue
// read — the container list IS catalogue data, it is what the Libraries screen
// renders, and FullImport calls it before the stream exists. Honouring the gate
// only at StreamItems would mean reading and binding a library list under a
// credential whose scope had never been graded. The ADR's own words are "the
// catalogue read"; StreamItems is named there because it was the read in view,
// not because Containers is exempt. sync.Once means the widening costs nothing:
// one consultation per source either way.
//
// # It reports. It does not refuse.
//
// ADR-0058 turned this down explicitly and gave the reason: refusing "leaves the
// operator with a service that will not talk to them and no visible reason why
// — the exact opposite of principle 3", and it makes the §14 finding LESS
// visible rather than more. So an elevated verdict is a WARN naming every
// finding, and the read proceeds. The only error this returns is a failure to
// authenticate at all, which is not a verdict — it is a credential that cannot
// open, and no read is possible under it anyway.
//
// The verdict is surfaced the same way the connection test surfaces it
// (cmd/usarr's testBookOrbit and bookOrbitScopeNote): the account name and the
// findings, never the token.
func (s *BookOrbitSource) gate(ctx context.Context) error {
	s.gateOnce.Do(func() {
		verdict, err := s.Client.Authenticate(ctx)
		if err != nil {
			s.gateErr = fmt.Errorf("bookorbit: authenticate before the first catalogue read: %w", err)
			return
		}
		switch {
		case verdict.Minimal():
			s.log().Info("bookorbit credential scope graded before the first catalogue read",
				"account", verdict.Account.Username, "verdict", "minimal")
		default:
			details := make([]string, 0, len(verdict.Findings))
			for _, f := range verdict.Findings {
				details = append(details, f.String())
			}
			// WARN for both severities, and the level is not the signal —
			// `elevated` is. A finding that is merely "more reach than UsArr
			// uses" still belongs in front of the operator, because the fix is
			// the same one sentence either way.
			s.log().Warn("reading a BookOrbit catalogue under a credential broader than a replica needs",
				"account", verdict.Account.Username,
				"elevated", verdict.Elevated(),
				"findings", strings.Join(details, "; "),
				"effect", "the read proceeds; ADR-0058 reports rather than refuses")
		}
	})
	return s.gateErr
}

// bookKind is the FALLBACK kind this adapter's containers are bound at, pending
// what the walk finds.
//
// ⚠️ IT IS A CONSTANT BECAUSE BOOKORBIT SUPPLIES NO CONTAINER-LEVEL KIND AT ALL,
// not because every BookOrbit library holds prose. `libraries`
// (db/schema/libraries.ts) has no type, kind or mediaType column — see
// bookorbit.Library — so §6.4's rule that "work.kind is derived from a rule
// UsArr controls — the library's declared kind" has no INPUT here. What UsArr
// keeps is the OWNERSHIP of the decision; what changes is that the input is now
// the source's own per-BOOK format fact rather than a declared container type.
//
// ⚠️ THIS COMMENT'S SECOND HALF READ *"a BookOrbit library holding both prose
// and comics binds to ONE UsArr library of kind 'book', and its comics are
// skipped and counted … Splitting one container into two libraries is a deviation
// from §17.8 that needs an ADR, and it is comics' slice to ask for"*. THE ADR WAS
// ASKED FOR AND GRANTED: ADR-0066 decision 5 rules the split and ADR-0068
// activates it. A mixed container now binds to a 'book' library HERE and a
// 'comic' library minted lazily over the same container ref by
// store.resolveBinding, on the first comic the walk actually reaches — lazily,
// because this constant runs before any book has been read and a comic library
// minted for a prose-only container would be an empty row on the Libraries
// screen for ever.
//
// ⚠️ AND THIS COMMENT SAID *"'book' is the kind every container this adapter
// BINDS is given"*, WHICH IS NO LONGER TRUE OF THE END STATE. It is still the
// kind every container is bound at, because it is still all this constant can
// know; what changed is that the walk may move it. A container that turns out to
// hold ONLY comics ends up as one 'comic' library — the same row, retyped in
// place, keeping the container's name and its slug — rather than an empty 'book'
// library with a 'Comics (Comics)' sibling beside it. That is what
// store.CatalogueContainer.KindProvisional buys, and Containers() sets it.
//
// What is unchanged is what this constant IS: BookOrbit supplies no
// container-level kind, so 'book' is a guess made before the walk, and the comic
// side is derived per book from the source's own format fact.
const bookKind = "book"

// bookRemoteKind is service_item_link.remote_kind for every row this adapter
// writes.
//
// 'book' is the upstream's OWN noun for the row — the table is `books`, the
// route is /books, the DTO is BookCard. remote_kind takes it verbatim, and it is
// part of ux_sil's key, so it must never be "series" the way Kavita's is.
//
// IT IS A CONSTANT RATHER THAN A LITERAL BECAUSE THREE PLACES NOW SPELL IT:
// mapBook writes it, and the credit and file passes match on it to decide
// whether a request is one of theirs. Two spellings would not fail to compile —
// they would produce an import whose second and third passes silently found
// nothing.
const bookRemoteKind = "book"

// The two work.kind values the comic path writes, and the remote_kind the
// series is linked under.
//
// ⚠️ 'comic' IS THE SERIES AND 'comic_issue' IS THE ISSUE (ADR-0030, and
// migration 00005's own CHECK comment: "'comic' is the SERIES, 'comic_issue' the
// issue or chapter"). A BookOrbit comic is ONE FILE — MediaKindComic is "one of
// cbz, cbr, cb7, cbx" — so it is an ISSUE, and ADR-0068 mints it under a series
// work rather than as one. Getting these two the wrong way round writes a wrong
// work.kind at ingest, which §6.4's cascade makes permanently unmergeable.
const (
	comicSeriesKind = "comic"
	comicIssueKind  = "comic_issue"

	// comicSeriesRemoteKind is service_item_link.remote_kind for the SERIES row.
	//
	// It is 'series' — BookOrbit's own noun for the table (`series`), the route
	// (/series) and the DTO — on bookRemoteKind's rule, and it is a DIFFERENT
	// remote_kind from the issue's so that ux_sil's key cannot collide: a
	// BookOrbit book id and a BookOrbit series id are two independent serials
	// that will overlap constantly.
	comicSeriesRemoteKind = "series"
)

// Containers reads GET /api/v1/libraries and gives each one the 'book' kind, as
// a FALLBACK the walk is allowed to move.
//
// NOTHING IS DECLINED HERE, and that is a difference from the Kavita adapter
// worth naming. mapLibraryType declines Kavita's Image type because Kavita SAYS
// what a library holds and UsArr has no kind for that answer. BookOrbit says
// nothing, so there is no answer to decline on — a container can only be judged
// by what is inside it, and that judgement happens per book in StreamItems.
func (s *BookOrbitSource) Containers(ctx context.Context) ([]store.CatalogueContainer, error) {
	if err := s.gate(ctx); err != nil {
		return nil, err
	}
	libs, err := s.Client.Libraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("bookorbit: read libraries: %w", err)
	}

	s.mu.Lock()
	s.containers = libs
	// ⚠️ THE MAP IS CREATED EMPTY AND IS NEVER SEEDED FROM libs, WHICH IS A
	// DECISION RATHER THAN AN OMISSION (ADR-0063). Skipped() now returns an entry
	// per container so cmd/usarr can write a zero row for a clean one, and those
	// entries come from tallyFor DURING the walk. Seeding them here — from the
	// container list, before a single book has been read — would hand every
	// container the walk never reached a row saying it left nothing out, which is
	// exactly the before-the-walk imprecision the completeness check below still
	// carries and which the skip read was decoupled from.
	if s.skips == nil {
		s.skips = map[string]*SkipTally{}
	}
	s.mu.Unlock()

	// THE CONTENT-FILTER CHECK, HERE AND NOT IN StreamItems, and it cannot fail
	// this call. bookCount — the "what UsArr can see" half of the subtraction —
	// arrives on the listing above and nowhere else, and a container list bound
	// without ever being graded is the state the check exists to prevent. See
	// bookorbitcompleteness.go.
	s.checkCompleteness(ctx, libs)

	out := make([]store.CatalogueContainer, 0, len(libs))
	for _, l := range libs {
		out = append(out, store.CatalogueContainer{
			RemoteID: containerRef(l.ID),
			Name:     l.Name,
			Kind:     bookKind,
			// ⚠️ THE KIND ABOVE IS A FALLBACK AND SAYS SO. BookOrbit supplies no
			// container-level kind at all, so 'book' here is a constant chosen
			// before a single book has been read, and the WALK is the authority.
			// Without this flag a comics-only container binds eagerly at 'book'
			// and then gets a `comic` sibling minted beside it — two rows, one of
			// them permanently empty, and the other carrying a kind qualifier that
			// only earns its place when a container really is mixed. See
			// store.CatalogueContainer.KindProvisional for what it changes, and
			// what it deliberately does NOT: the row is still created here and
			// eagerly, because ADR-0066 decision 1 requires a container whose
			// every item is skipped to be bound anyway.
			KindProvisional: true,
		})
	}
	return out, nil
}

// containerRef is a BookOrbit library id as store.CatalogueContainer.RemoteID,
// store.CatalogueItem.ContainerID and library_source.container_ref all spell it.
//
// It is one function rather than four `strconv.FormatInt(id, 10)` calls because
// the string is a JOIN KEY: the completeness verdict this adapter records is
// filed under it and read back by joining sync_report.remote_id to
// library_source.container_ref, so a second formatting rule anywhere would be a
// silently empty join rather than a compile error.
func containerRef(id int64) string { return strconv.FormatInt(id, 10) }

// StreamItems walks every container's books and hands the MAPPABLE ones to fn —
// a prose book as a 'book', a comic as a 'comic_issue' under its series
// (ADR-0068). An unknown media kind is skipped and counted.
//
// # One walk per container, in the order Containers returned them
//
// BookCard carries no libraryId, so the per-library route is what supplies the
// container id every item needs. The count returned is the number of books the
// WALK DELIVERED to this adapter — including the ones it then skipped, which
// since ADR-0068 are the unknown kinds rather than the comics — because
// Report.ItemsRead is documented as "how far the read got, never how many rows
// are correct", and a count that silently excluded the skips would make a
// library that was read in full look like only the part of it that mapped.
//
// # Partial delivery
//
// On failure the count still reflects everything already handed over, on
// StreamBooks's contract and on the Source interface's. A library that fails
// mid-walk aborts the whole stream rather than being skipped: an import that
// silently omitted one library would present a partial catalogue as a complete
// one, and last_full_sync_at must not be stamped over that.
func (s *BookOrbitSource) StreamItems(ctx context.Context, fn func(store.CatalogueItem) error) (int, error) {
	if err := s.gate(ctx); err != nil {
		return 0, err
	}

	s.mu.Lock()
	libs := append([]bookorbit.Library(nil), s.containers...)
	s.mu.Unlock()

	if len(libs) == 0 {
		// Containers() was never called, or the credential can see nothing. The
		// first is a programming error and the second is a real, legal state —
		// a shared account granted no libraryIds. Neither is an error here: the
		// importer calls Containers first and would have reported an empty list
		// already, and inventing a failure would turn "you granted no libraries"
		// into "your BookOrbit is broken".
		return 0, nil
	}

	var read int
	for _, l := range libs {
		ref := containerRef(l.ID)
		tally := s.tallyFor(ref)
		residue := s.residueFor(ref)

		page, err := s.Client.StreamBooks(ctx, l.ID, func(b bookorbit.Book) error {
			switch b.MediaKind() {
			case bookorbit.MediaKindComic:
				// ⚠️ NO keepCard, AND THAT IS ADR-0068'S OWN BUDGET RATHER THAN AN
				// OMISSION. The three passes that read the card map — credits,
				// files and covers — are fed from `imported`, and the cover pass
				// issues ONE HTTP REQUEST PER ITEM it is given. ADR-0068 states
				// the acquisition cost in full — "the allowlist widens by four
				// fields, and that is the acquisition cost in full … Zero extra
				// HTTP" — so a per-issue cover fetch is outside what it
				// authorises. The importer drops child items from `imported` for
				// the same sentence; see streamAndApply.
				return fn(mapComic(b, ref, residue))
			case bookorbit.MediaKindEbook, bookorbit.MediaKindAudiobook:
				// KEPT BEFORE IT IS HANDED OVER, not after. fn may return an
				// error that ends the stream, and a book the importer HAS
				// already applied must not be missing from the buffer the two
				// later passes read — the stream's partial-delivery contract is
				// that "the calls to fn happened and their effects stand".
				s.keepCard(b)
				return fn(mapBook(b, ref))
			default:
				tally.Unknown++
				return nil
			}
		})
		read += page.Count
		if err != nil {
			return read, fmt.Errorf("bookorbit: walk library %q (%s) after %d books: %w",
				l.Name, ref, page.Count, err)
		}
		if residue.Total() > 0 {
			// The residue's operator-facing half, on the skip line's terms: a
			// number nobody can see is the same as no number. The durable half is
			// the sync_report row cmd/usarr writes from Comics().
			s.log().Info("bookorbit: comics ingested on a residue default",
				"library_id", ref, "library", l.Name,
				"synthesized_series", residue.SynthesizedSeries,
				"multi_series_books", residue.MultiSeries,
				"declined_memberships", residue.ExtraMemberships,
				"effect", "a synthesized series is one issue with is_oneshot=1; a declined "+
					"membership is recorded and never resolved (ADR-0068)")
		}
		if tally.Total() > 0 {
			// LOGGED PER CONTAINER, as it finishes, and not only summed at the
			// end: a walk that dies on library three must still have said what
			// libraries one and two skipped.
			//
			// ⚠️ THIS ZERO GATE STAYS AND Skipped()'s DOES NOT, deliberately.
			// ADR-0063 turned the RECORD into one row per walked container
			// because an absent row there is read as a fact. A log line is not
			// read that way — nobody infers "this container was walked clean"
			// from the absence of a line in a process log — so a line per clean
			// library would be noise on every import and would buy the honesty
			// nothing. The record is the row.
			s.log().Info("bookorbit: books read but not mapped",
				"library_id", ref, "library", l.Name,
				"read", page.Count, "skipped_comics", tally.Comics, "skipped_unknown", tally.Unknown,
				"reason", "a book BookOrbit itself classifies as media kind 'unknown' has no "+
					"format to map from")
		}
	}
	return read, nil
}

// keepCard files one mapped book's credits, year and files for the two passes
// that run after the stream closes. See BookOrbitSource.cards.
//
// THE KEY IS THE REMOTE ID ALONE and not (kind, id), because this adapter writes
// exactly one remote_kind — bookRemoteKind — and mapBook is the only place a
// BookOrbit item's identifiers are minted. A second kind arriving here would be
// a bug in mapBook rather than a collision this map could resolve.
func (s *BookOrbitSource) keepCard(b bookorbit.Book) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cards == nil {
		s.cards = map[string]bookOrbitCard{}
	}
	s.cards[strconv.FormatInt(b.ID, 10)] = bookOrbitCard{
		Authors:       b.Authors,
		Narrators:     b.Narrators,
		PublishedYear: b.PublishedYear,
		Files:         b.Files,
	}
}

// card reads back what keepCard filed.
//
// A MISS IS NOT AN ERROR, and both passes treat it as Kavita treats an
// unparseable remote id: nothing was attempted and there is no upstream to
// blame. It happens for a request whose remote id this adapter never wrote —
// an item left in service_item_link by some other source, or a source reused
// across two imports — and inventing a failure for it would put a
// `file_walk_failed` row in sync_report describing a read that never happened.
func (s *BookOrbitSource) card(remoteKind, remoteID string) (bookOrbitCard, bool) {
	if remoteKind != bookRemoteKind {
		return bookOrbitCard{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cards[remoteID]
	return c, ok
}

func (s *BookOrbitSource) residueFor(ref string) *ComicResidue {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.residue == nil {
		s.residue = map[string]*ComicResidue{}
	}
	r, ok := s.residue[ref]
	if !ok {
		r = &ComicResidue{}
		s.residue[ref] = r
	}
	return r
}

func (s *BookOrbitSource) tallyFor(ref string) *SkipTally {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.skips == nil {
		s.skips = map[string]*SkipTally{}
	}
	t, ok := s.skips[ref]
	if !ok {
		t = &SkipTally{}
		s.skips[ref] = t
	}
	return t
}

// Skipped reports, per container, how many books the walk read and did not map.
//
// ⚠️ IT IS THE ONLY PLACE THE NUMBER EXISTS IN THIS PROCESS, which is why it is
// exported rather than left to the log line above. cmd/usarr writes it into
// sync_report after the import, because a Report the caller forgets to read and
// a log line nobody greps are the same thing as silence — the argument
// importer.go's recordFileWalkFailures already makes for a dropped file walk.
//
// # ONE ENTRY PER CONTAINER THE WALK TOUCHED, ZERO OR NOT (ADR-0063)
//
// ⚠️ A ZERO TALLY IS RETURNED RATHER THAN FILTERED OUT, and that reversal is
// this method's whole contract. It used to drop any container whose total was
// zero, so cmd/usarr wrote a row only where something had been skipped — and the
// read one field over then had to borrow the completeness row as evidence that a
// container had been walked at all. "Nothing was left out" and "nothing looked"
// are different facts, and returning only the non-zero tallies rendered them
// identically. The neighbouring content_completeness rule already goes the other
// way (a row per container observed), and two adjacent readers with opposite
// absence conventions is its own hazard.
//
// # AND THE SET IS THE WALK'S, NOT Containers()'s — WHICH IS THE POINT
//
// ⚠️ NOTHING PRE-POPULATES THIS MAP. Containers() creates it empty and adds no
// entries; tallyFor creates an entry at the top of one container's iteration in
// StreamItems, so an entry exists exactly for a container the walk REACHED. An
// import that dies on library three returns libraries one, two and three and NOT
// four and five — which is what lets the skip read stop inheriting the
// completeness row's before-the-walk imprecision, rather than merely moving it.
// Seeding this map from s.containers would move it here.
// TestSkippedNamesOnlyTheContainersTheWalkReached is what stops that.
//
// ⚠️ WHAT IS NOT CLOSED, stated rather than implied: the one container the walk
// died INSIDE comes back with the tally it had reached, so a container that had
// skipped nothing before failing reads as a clean zero for a partial read. At
// most one per import — StreamItems returns on the first container error.
// ADR-0063 records it as still open.
//
// The slice is sorted by container id so two runs over the same instance produce
// the same order.
func (s *BookOrbitSource) Skipped() []ContainerSkips {
	s.mu.Lock()
	defer s.mu.Unlock()

	byRef := map[string]string{}
	for _, l := range s.containers {
		byRef[containerRef(l.ID)] = l.Name
	}
	out := make([]ContainerSkips, 0, len(s.skips))
	for ref, t := range s.skips {
		out = append(out, ContainerSkips{RemoteID: ref, Name: byRef[ref], SkipTally: *t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RemoteID < out[j].RemoteID })
	return out
}

// Comics reports, per container the walk reached, which residue defaults its
// comics took.
//
// ⚠️ ONE ENTRY PER CONTAINER THE WALK REACHED, ZERO OR NOT, on Skipped()'s rule
// and for its reason (ADR-0063): "nothing was left out" and "nothing looked" are
// different facts and an absent row renders them identically. A container of
// pure prose therefore reports a clean zero, which is what makes a LATER zero
// readable as "no comic took a default" rather than as "the walk never got
// there".
//
// It is the only place these numbers exist in this process; cmd/usarr writes
// them into sync_report, which is ADR-0068 decision 4 — "the first real import
// against the owner's library measures how often each occurs. Sizing comes from
// instrumentation, not from estimates, and not from asking the owner to run
// SQL".
//
// Sorted by container id so two runs over the same instance produce the same
// order.
func (s *BookOrbitSource) Comics() []ContainerComics {
	s.mu.Lock()
	defer s.mu.Unlock()

	byRef := map[string]string{}
	for _, l := range s.containers {
		byRef[containerRef(l.ID)] = l.Name
	}
	out := make([]ContainerComics, 0, len(s.residue))
	for ref, r := range s.residue {
		out = append(out, ContainerComics{RemoteID: ref, Name: byRef[ref], ComicResidue: *r})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RemoteID < out[j].RemoteID })
	return out
}

// mapBook projects one prose BookCard onto the schema.
func mapBook(b bookorbit.Book, containerID string) store.CatalogueItem {
	// TITLE IS NOT DEFENDED AGAINST EMPTY, and that is a measurement rather than
	// an oversight: assembleBookCards emits `row.title ?? basename(row.folderPath)`
	// and books.folder_path is a NOT NULL varchar(4096), so a card's title is
	// non-empty for any book BookOrbit can serve. Inventing a placeholder for a
	// case the source cannot produce would put text in work.title that no
	// upstream ever said.
	title := strings.TrimSpace(b.Title)

	it := store.CatalogueItem{
		RemoteID: strconv.FormatInt(b.ID, 10),

		RemoteKind: bookRemoteKind,

		ContainerID: containerID,
		Kind:        bookKind,

		Title: title,
		// BookOrbit has no sort title on the card. `primaryAuthorSortName` is a
		// column on books and is NOT projected onto BookCard, so there is nothing
		// to read; work.sort_title is NOT NULL and the title is the honest
		// default. Deriving one by stripping a leading article is exactly the
		// parse §6.5 rule 3 forbids.
		SortTitle:       title,
		NormalizedTitle: NormalizeTitle(title),
		NormVersion:     NormVersion,

		// RemotePath IS LEFT EMPTY AND THE REASON IS THE CARD, NOT A CHOICE ABOUT
		// PATHS. books.folderPath exists and BookDetail carries it; BookCard does
		// not, and slice 1 makes no per-book detail read. When one lands it goes
		// on service_item_link.remote_path — the column that holds what the
		// upstream reported verbatim — and nowhere a browser can read it.

		// remote_subtype is "the upstream's own sub-classification, stored
		// verbatim and unparsed" (§6.5 rule 3). For BookOrbit that is the primary
		// file's format token — 'epub', 'm4b', 'pdf' — which is a fact about THIS
		// book rather than about its container, and it is the input the file pass
		// turns into edition.format without a second read — see
		// bookOrbitEditionFormat, which is the reader this column was written
		// for.
		RemoteSubtype: primaryFormat(b),

		AddedAt:         b.AddedAt,
		RemoteUpdatedAt: b.UpdatedAt,

		// has_file is books.status, whose CHECK is ('present','missing','processing').
		// ✅ THIS IS STRICTLY BETTER THAN THE KAVITA ADAPTER'S `Pages > 0` PROXY:
		// BookOrbit distinguishes "the row is here and the bytes are not"
		// ('missing') from "deleted", so the reconciliation sweep will be able to
		// tell a vanished book from a broken one. Only 'present' is a file.
		HasFile: b.Status == bookStatusPresent,
	}

	if b.PageCount > 0 {
		it.PageCount = sql.NullInt64{Int64: b.PageCount, Valid: true}
	}

	// The subtitle, as an ALIAS alt title.
	//
	// work_alt_title.kind's vocabulary is original|translated|alias|acronym|sort
	// and there is no `subtitle` member; `alias` is "another string this work is
	// known by", which a subtitle is. The point is not the label — it is that
	// store's search-document builder folds alt titles into the FTS document, so
	// a user searching for "The Making of the Atomic Bomb" finds a book titled
	// "The Making" with that subtitle. Dropped when it merely repeats the title,
	// for kavita.go's reason: a duplicate inflates every document it appears in.
	if sub := strings.TrimSpace(b.Subtitle); sub != "" && sub != title {
		it.AltTitles = append(it.AltTitles, store.AltTitle{
			Title: sub, Normalized: NormalizeTitle(sub), Kind: "alias",
		})
	}

	it.ExternalIDs = bookOrbitExternalIDs(b)
	return it
}

// mapComic projects one comic-format BookCard onto the schema, as an ISSUE under
// a SERIES.
//
// # The parent, and why it is never null
//
// ADR-0068 decision 1: every imported comic file becomes a `comic_issue` work and
// every one of them is minted under a `comic` parent. A parentless issue is not a
// degraded comic — it is a row NO SHIPPED READ CAN SEE: `recentWorkKinds`,
// `browseKinds` and `corpusExcludedKinds` all exclude the kind, and the third
// does not merely exclude it, `writeSearchDoc` RETURNS AN ERROR on it. That is
// the whole argument against orphan issues, and the store routes children around
// the top-level path rather than relaxing the guard.
//
// # The binding is the scalar seriesId, and that was MEASURED
//
// ADR-0068 decision 3. `BookCard.seriesId` is BookOrbit's own maintained PRIMARY
// — a real scalar column that `series-membership.service.ts` keeps equal to the
// `displayOrder = 0` membership on every membership write — so binding on the
// scalar and binding on `memberships[displayOrder = 0]` are THE SAME BINDING by
// construction. That is why this function never has to look inside
// SeriesMemberships to find a parent, and therefore why recording the remainder
// can never quietly become resolving it.
//
// # Two residue defaults, both counted
//
//  1. NO SERIES → a SYNTHESIZED single-issue series named for the book, with
//     is_oneshot written to 1. The comic is never dropped and is never promoted
//     to a `comic` work in its own right — that second one is the per-row shape
//     ADR-0066 pre-emptively refused, and it would make /library/comics mean two
//     different things depending on upstream metadata quality.
//  2. SEVERAL SERIES → the primary binds and the rest are RECORDED. No second
//     parent, no second membership, no work_relation edge. The tier that would
//     adjudicate them is v0.3 and this must not build toward it.
func mapComic(b bookorbit.Book, containerID string, residue *ComicResidue) store.CatalogueItem {
	title := strings.TrimSpace(b.Title)

	it := store.CatalogueItem{
		RemoteID: strconv.FormatInt(b.ID, 10),

		// STILL bookRemoteKind. remote_kind is "the upstream's OWN noun for the
		// row", and upstream this is a row in `books` served by /books as a
		// BookCard — a .cbz is a book to BookOrbit. It is UsArr that calls it an
		// issue. Spelling it 'comic_issue' here would put UsArr's vocabulary in
		// the column that holds the upstream's, and would split one BookOrbit
		// book across two ux_sil keys the day its format changed.
		RemoteKind: bookRemoteKind,

		ContainerID: containerID,
		Kind:        comicIssueKind,

		Title:           title,
		SortTitle:       title,
		NormalizedTitle: NormalizeTitle(title),
		NormVersion:     NormVersion,

		RemoteSubtype: primaryFormat(b),

		AddedAt:         b.AddedAt,
		RemoteUpdatedAt: b.UpdatedAt,
		HasFile:         b.Status == bookStatusPresent,
	}

	if b.PageCount > 0 {
		it.PageCount = sql.NullInt64{Int64: b.PageCount, Valid: true}
	}

	// number_sort ONLY, and number_text DELIBERATELY LEFT NULL. Migration 00006
	// models the pair as Komga does — "a string plus a float sort key" — because
	// real issue numbers are '1.MU', 'Annual 1', '1A'. BookOrbit has no such
	// token: `BookCard.seriesIndex` is `number | null` and there is no text form
	// anywhere on the card. Rendering the float back into a string would be
	// UsArr inventing an upstream's token, which §6.5 rule 3 forbids, so the
	// column that has no source stays NULL until a source reports one.
	if b.SeriesIndex != nil {
		it.NumberSort = sql.NullFloat64{Float64: *b.SeriesIndex, Valid: true}
	}

	if b.SeriesID > 0 {
		it.Parent = comicSeriesParent(b.SeriesID, b.SeriesName)
	} else {
		it.IsOneshot = true
		it.Parent = synthesizedComicSeriesParent(b.ID, title)
		residue.SynthesizedSeries++
	}

	// THE DECLINED MEMBERSHIPS. They are counted and sampled and then dropped:
	// nothing downstream of here can reach them, which is what makes "recorded,
	// not resolved" structural rather than a promise.
	if extra := declinedSeriesIDs(b); len(extra) > 0 {
		residue.MultiSeries++
		residue.ExtraMemberships += len(extra)
		if len(residue.Sample) < comicResidueSampleCap {
			residue.Sample = append(residue.Sample,
				DeclinedMembership{BookID: b.ID, SeriesIDs: extra})
		}
	}

	if sub := strings.TrimSpace(b.Subtitle); sub != "" && sub != title {
		it.AltTitles = append(it.AltTitles, store.AltTitle{
			Title: sub, Normalized: NormalizeTitle(sub), Kind: "alias",
		})
	}

	// ⚠️ NO EXTERNAL IDS, and the reason is a GRAIN mismatch rather than an
	// oversight. The one identifier bookOrbitExternalIDs writes is
	// `hardcover_book`, and §6.4 amendment 4 elects it work-strong AT THE BOOK
	// grain. Writing it against a comic_issue would let ux_extid_work_strong
	// resolve a prose work and an issue onto each other across kinds — which
	// tier 1's `AND w.kind = ?` exists to prevent — and writing it against the
	// SERIES would claim a whole series is one Hardcover book. comicvineId is on
	// BookOrbit's metadata table and would be the right source at this grain; it
	// is not on BookCard (see bookOrbitExternalIDs), and it is an ISSUE id in
	// BookOrbit's own mapper, so it is not a series id either.

	return it
}

// comicSeriesParent is the parent for a comic that HAS an upstream series.
func comicSeriesParent(seriesID int64, seriesName string) *store.CatalogueParent {
	name := strings.TrimSpace(seriesName)
	if name == "" {
		// A series that exists and has no name. `work.title` is NOT NULL, and
		// this is bindOneContainer's own fallback shape — `"Library " +
		// c.RemoteID` for a container with a blank name — rather than a new
		// convention. The alternative, naming the series after one of its
		// issues, is worse: it would make the series row assert a title no
		// upstream ever gave it.
		name = "Series " + strconv.FormatInt(seriesID, 10)
	}
	return &store.CatalogueParent{
		RemoteKind:      comicSeriesRemoteKind,
		RemoteID:        strconv.FormatInt(seriesID, 10),
		Kind:            comicSeriesKind,
		Title:           name,
		SortTitle:       name,
		NormalizedTitle: NormalizeTitle(name),
		NormVersion:     NormVersion,
	}
}

// synthesizedComicSeriesParent is the parent for a comic that has NO upstream
// series — ADR-0068 decision 2's single-issue series.
//
// ⚠️ THE REMOTE ID MUST BE DETERMINISTIC AND MUST NOT COLLIDE WITH A REAL SERIES
// ID. It is derived from the BOOK id under a prefix no BookOrbit series id can
// take, so a re-import resolves the same series work through the same
// service_item_link rather than minting a second one every time it runs — which
// would double /library/comics on every import.
func synthesizedComicSeriesParent(bookID int64, title string) *store.CatalogueParent {
	return &store.CatalogueParent{
		RemoteKind: comicSeriesRemoteKind,
		RemoteID:   oneshotSeriesRef(bookID),
		Kind:       comicSeriesKind,
		// NAMED FOR THE BOOK, which is the one honest name available: the series
		// has exactly one issue and no upstream identity of its own.
		Title:           title,
		SortTitle:       title,
		NormalizedTitle: NormalizeTitle(title),
		NormVersion:     NormVersion,
		Synthesized:     true,
	}
}

// oneshotSeriesRef is the synthesized series' remote id.
//
// The prefix is what keeps it out of BookOrbit's own id space: `series.id` is a
// positive integer serial, so no upstream series can ever be spelled
// "oneshot:12".
func oneshotSeriesRef(bookID int64) string {
	return "oneshot:" + strconv.FormatInt(bookID, 10)
}

// declinedSeriesIDs is every series this book belongs to EXCEPT the primary.
//
// It is the whole of "record what you declined to act on" (ADR-0063's precedent,
// applied by ADR-0068 decision 3) and it returns ids rather than memberships
// because the record lands in sync_report.detail, where upstream TEXT may not go
// (reference/security.md §5).
//
// ⚠️ IT FILTERS ON THE SERIES ID, NOT ON displayOrder. The two agree by
// construction — BookOrbit keeps the scalar equal to the displayOrder 0 entry —
// and filtering on the id is correct even if a future BookOrbit reorders the
// list, because the id is what the parent was actually bound to. A book with no
// primary at all (the synthesized case) declines EVERY membership it has, which
// is the honest reading: none of them was acted on.
func declinedSeriesIDs(b bookorbit.Book) []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, m := range b.SeriesMemberships {
		if m.SeriesID == b.SeriesID || seen[m.SeriesID] {
			continue
		}
		seen[m.SeriesID] = true
		out = append(out, m.SeriesID)
	}
	return out
}

// bookStatusPresent is the one member of books_status_chk that means the bytes
// are there. The other two are 'missing' and 'processing'.
const bookStatusPresent = "present"

// primaryFormat is the primary file's format token, or "" when there is none.
func primaryFormat(b bookorbit.Book) string {
	f, ok := b.PrimaryFile()
	if !ok {
		return ""
	}
	return f.Format
}

// HardcoverBookSource is external_id.source for a Hardcover BOOK id.
//
// It is the SAME STRING internal/libsync/kavita.go writes, and that is the
// point: §6.4's amendment 4 names "hardcover_book" as one of exactly three
// work-strong book sources, so a book UsArr knows from Kavita and from BookOrbit
// resolves onto ONE work through ux_extid_work_strong rather than two.
const HardcoverBookSource = "hardcover_book"

// bookOrbitExternalIDs projects the ONE identifier BookCard carries that may be
// written against a work.
//
// # Why exactly one, when BookOrbit stores fourteen
//
// bookMetadata (db/schema/metadata.ts) carries fourteen typed provider id
// columns — googleBooksId, goodreadsId, amazonId, hardcoverId,
// hardcoverEditionId, openLibraryId, itunesId, koboId, audibleId, librofmId,
// comicvineId, ranobedbId, lubimyczytacId, aladinId — plus isbn10 and isbn13.
// BookCard exposes THREE of them: isbn13, hardcoverId and hardcoverEditionId.
// The other twelve are on BookDetail, which costs one GET per book with no batch
// route anywhere in book.controller.ts, and that read is not in this slice.
//
// Of the three on the card:
//
//   - hardcoverId → hardcover_book, at 1.0. §6.4 amendment 4 ELECTS it
//     work-strong; that is the architecture naming a source, not this mapping
//     trusting a field.
//   - isbn13 → an EDITION identifier, and amendment 4 is categorical that "an
//     ISBN or an ASIN must never satisfy ux_extid_work_strong". external_id's
//     CHECK requires exactly one of work_id / edition_id, so it may not be
//     written against the work. HELD, NOT WRITTEN. ⚠️ THIS ENTRY USED TO ADD
//     *"slice 1 writes no edition rows"* as its second reason and that reason
//     has expired — bookorbitfiles.go writes one primary edition per book. The
//     answer does not change, because there is still no way for an adapter to
//     write an edition-scoped external_id: store mints the edition inside its
//     own file writer and returns no id, and store.FileSet has no identifier
//     field. That is a change to the shared write path when someone wants it.
//   - hardcoverEditionId → an edition identifier by its own name. Same answer.
//
// ⚠️ openLibraryId IS THE ONE THIS SLICE MOST WANTS AND CANNOT HAVE. It is a
// genuine Open Library WORK id — open-library.mapper.ts strips the `/works/`
// prefix before storing it — and §6.4 amendment 4 names `openlibrary_work`
// work-strong, which would make it the first work-strong identifier any UsArr
// catalogue source has offered. It is NOT ON BookCard. It arrives with the
// per-book detail read, and it is the strongest single argument for scheduling
// that slice.
//
// 🚩 AND comicvineId IS NEVER WRITTEN BY ANY SLICE OF THIS ADAPTER AGAINST A
// WORK. comicvine.mapper.ts's mapIssueToCandidate sets `providerId:
// String(issue.id)` from a ComicVineIssue — it is an ISSUE id, one level BELOW
// the work, the OPPOSITE grain from Kavita's SeriesDto.comicVineId that
// comicvine.go treats as a volume. comicVineIdentity's own refusal text applies
// verbatim: "an issue is one level below the work and is never written as a work
// id".
//
// # Degraded identity is the ordinary case and is not an error
//
// A BookOrbit whose operator has matched nothing returns null for hardcoverId on
// every card, this returns an empty slice, and the work is still written, filed,
// indexed and searchable. §6.4's "not identified" state is the ABSENCE of an
// external_id row.
func bookOrbitExternalIDs(b bookorbit.Book) []store.ExternalIdentifier {
	var out []store.ExternalIdentifier

	// add() hard-codes 1.0, exactly as kavita.go's does, and carries the same
	// obligation: A FIELD WIRED TO add() IS ASSERTING THAT §6.4 NAMES ITS SOURCE
	// WORK-STRONG. If a future field does not have that, route it through
	// webLinkIdentity or editableIdentity instead — both of which CANNOT return
	// 1.0, which is what makes the cap structural rather than a number each call
	// site has to remember.
	add := func(source, value string) {
		v := strings.TrimSpace(value)
		if v == "" || v == "0" {
			return
		}
		out = append(out, store.ExternalIdentifier{Source: source, Value: v, Confidence: 1.0})
	}
	add(HardcoverBookSource, b.HardcoverID)

	return out
}
