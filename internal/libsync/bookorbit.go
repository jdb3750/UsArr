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

// The BookOrbit catalogue adapter — PROSE ONLY, END TO END, with its credits,
// its files and its year.
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
// audiobook, which are one kind differing only in edition.format (ADR-0031).
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
// STILL DOES NOT:
//
//  1. COMICS. A comic-format book is SKIPPED AND COUNTED, never guessed at. The
//     unit-of-work question for comics is open — BookOrbit's series have no
//     library and a book can belong to several of them, so the series work a
//     comic would hang under has no container to bind to — and a wrong
//     work.kind is written once at ingest and can never be merged away (§6.4's
//     cascade). Counting is the whole of the honesty here: a skipped item that
//     vanishes silently looks exactly like one that was never there. See
//     Skipped.
//  2. CHANNEL 3b AND CHANNEL 4. No delta walk, no reconciliation sweep, no
//     cover fetch, and no per-book detail read. And STILL no migration: every
//     column the three passes write already exists in 00005_library_sync.sql and
//     00007_work_credit.sql.

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
	// ⚠️ A COMIC AND AN UNKNOWN GET NO ENTRY. They are skipped by StreamItems,
	// so no work exists for their credits to hang on and the importer never asks
	// for them; keeping their cards would be a buffer that only ever grew.
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
		Client: c,
		skips:  map[string]*SkipTally{},
		cards:  map[string]bookOrbitCard{},
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

// bookKind is the work.kind every container this adapter binds is given.
//
// ⚠️ IT IS A CONSTANT BECAUSE BOOKORBIT SUPPLIES NO CONTAINER-LEVEL KIND AT ALL,
// not because every BookOrbit library holds prose. `libraries`
// (db/schema/libraries.ts) has no type, kind or mediaType column — see
// bookorbit.Library — so §6.4's rule that "work.kind is derived from a rule
// UsArr controls — the library's declared kind" has no INPUT here. What UsArr
// keeps is the OWNERSHIP of the decision; what changes is that the input is now
// the source's own per-BOOK format fact rather than a declared container type.
//
// The consequence is stated rather than hidden: a BookOrbit library holding both
// prose and comics binds to ONE UsArr library of kind 'book', and its comics are
// skipped and counted (see Skipped). §17.8's "one library per upstream
// container" is honoured; what is not honoured is the assumption that a
// container holds one kind. Splitting one container into two libraries is a
// deviation from §17.8 that needs an ADR, and it is comics' slice to ask for.
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

// Containers reads GET /api/v1/libraries and gives each one the 'book' kind.
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

// StreamItems walks every container's books and hands the PROSE ones to fn.
//
// # One walk per container, in the order Containers returned them
//
// BookCard carries no libraryId, so the per-library route is what supplies the
// container id every item needs. The count returned is the number of books the
// WALK DELIVERED to this adapter — including the comics it then skipped —
// because Report.ItemsRead is documented as "how far the read got, never how
// many rows are correct", and a count that silently excluded the skips would
// make a 20,000-comic library look like a 40-book one that read fine.
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

		page, err := s.Client.StreamBooks(ctx, l.ID, func(b bookorbit.Book) error {
			switch b.MediaKind() {
			case bookorbit.MediaKindComic:
				tally.Comics++
				return nil
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
		if tally.Total() > 0 {
			// LOGGED PER CONTAINER, as it finishes, and not only summed at the
			// end: a walk that dies on library three must still have said what
			// libraries one and two skipped.
			s.log().Info("bookorbit: books read but not mapped",
				"library_id", ref, "library", l.Name,
				"read", page.Count, "skipped_comics", tally.Comics, "skipped_unknown", tally.Unknown,
				"reason", "slice 1 maps prose only; a comic-format book has no settled unit of work")
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
		if t.Total() == 0 {
			continue
		}
		out = append(out, ContainerSkips{RemoteID: ref, Name: byRef[ref], SkipTally: *t})
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
