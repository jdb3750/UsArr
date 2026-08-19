package libsync

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// ⚠️ THE BOOKORBIT CASSETTES THIS FILE REPLAYS ARE SYNTHETIC, and each says so in
// its own header along with the BookOrbit commit it was authored from. None was
// captured off a wire: ADR-0052 records that this project has never probed a
// live BookOrbit. A SYNTHETIC CASSETTE PROVES THIS MAPPING, NOT THE SERVER'S
// BEHAVIOUR — it cannot discover that a field is always null in practice or that
// a controller enforces something its source does not appear to.
//
// THE DATABASE SIDE IS NOT SYNTHETIC. The end-to-end assertions run against a
// real migrated database opened through internal/db.

const bookOrbitBaseURL = "http://bookorbit.test:3000"

// fakeBookOrbitReader is a hand-built BookOrbitReader: no HTTP, no cassette,
// just the values the mapping is asked to project. It is what makes the mapping
// tests readable — a fixture that has to be expressed as a JSON body to change
// one field stops being a test of the mapping.
type fakeBookOrbitReader struct {
	verdict  bookorbit.ScopeVerdict
	authErr  error
	auths    int
	libs     []bookorbit.Library
	libsErr  error
	books    map[int64][]bookorbit.Book
	walkErr  map[int64]error
	walkedAt []int64

	// stats is what GET /libraries/{id}/stats answers per library, and statsErr
	// is the refusal that stands in for the guard-later scenario. A library in
	// NEITHER map answers a total equal to its own bookCount, so the ordinary
	// fixture — which says nothing about stats at all — produces `complete` and
	// a test has to opt into a shortfall.
	stats     map[int64]int64
	statsErr  map[int64]error
	statsSeen []int64
}

func (f *fakeBookOrbitReader) Authenticate(context.Context) (bookorbit.ScopeVerdict, error) {
	f.auths++
	return f.verdict, f.authErr
}

func (f *fakeBookOrbitReader) Libraries(context.Context) ([]bookorbit.Library, error) {
	return f.libs, f.libsErr
}

func (f *fakeBookOrbitReader) StreamBooks(
	_ context.Context, libraryID int64, fn func(bookorbit.Book) error,
) (bookorbit.BookPage, error) {
	f.walkedAt = append(f.walkedAt, libraryID)
	page := bookorbit.BookPage{Total: int64(len(f.books[libraryID]))}
	for _, b := range f.books[libraryID] {
		if err := fn(b); err != nil {
			return page, err
		}
		page.Count++
	}
	return page, f.walkErr[libraryID]
}

func (f *fakeBookOrbitReader) LibraryStats(
	_ context.Context, libraryID int64,
) (bookorbit.LibraryStats, error) {
	f.statsSeen = append(f.statsSeen, libraryID)
	if err, ok := f.statsErr[libraryID]; ok {
		return bookorbit.LibraryStats{}, err
	}
	if total, ok := f.stats[libraryID]; ok {
		return bookorbit.LibraryStats{TotalBooks: total}, nil
	}
	// The unremarkable default: the unfiltered total equals what the credential
	// was shown, i.e. no content filter. It is derived from the fixture's own
	// bookCount rather than fixed, so a test that sets one gets agreement
	// without also having to set a stats entry.
	for _, l := range f.libs {
		if l.ID == libraryID {
			return bookorbit.LibraryStats{TotalBooks: l.BookCount}, nil
		}
	}
	return bookorbit.LibraryStats{}, nil
}

func proseBook(id int64, title string) bookorbit.Book {
	return bookorbit.Book{
		ID: id, Title: title, Status: "present",
		Files:     []bookorbit.BookFile{{ID: id*10 + 1, Format: "epub", Role: "primary"}},
		AddedAt:   time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 17, 7, 0, 30, 0, time.UTC),
	}
}

func withFormat(b bookorbit.Book, format string) bookorbit.Book {
	b.Files = []bookorbit.BookFile{{ID: b.ID*10 + 1, Format: format, Role: "primary"}}
	return b
}

// logTo builds a logger writing into buf, so a test can assert on what the
// adapter SAID as well as on what it did. The §14 verdict is only observable
// that way: ADR-0058's decision is that the client REPORTS, so "it reported" is
// the behaviour under test.
func logTo(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// ─── the §14 scope gate ──────────────────────────────────────────────────────

// TestTheScopeGateIsConsultedBeforeTheFirstCatalogueRead is ADR-0052's gate and
// ADR-0058's discharge of it, asserted at the place slice 1 owns.
//
// ADR-0058: "what ships is the mechanism, not the gate's enforcement —
// ADR-0052's condition is on the catalogue read, so the first StreamItems in
// slice 1 must consult ScopeVerdict.Elevated(), and slice 0 ships the thing it
// will consult."
//
// 🚩 IT IS ASSERTED ON Containers, NOT ONLY ON StreamItems, and that is the
// widening bookorbit.go's gate() argues for: Containers performs
// GET /api/v1/libraries, it runs FIRST, and the container list is catalogue
// data. A gate that only fired at StreamItems would have already read and bound
// a library list under an ungraded credential.
func TestTheScopeGateIsConsultedBeforeTheFirstCatalogueRead(t *testing.T) {
	r := &fakeBookOrbitReader{libs: []bookorbit.Library{{ID: 1, Name: "Fiction"}}}
	src := NewBookOrbitSource(r)

	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if r.auths != 1 {
		t.Fatalf("the credential was graded %d times before the container read, want 1", r.auths)
	}
	// And exactly once for the whole source: the walk must not re-grade per
	// library, which would cost a mint per container against a 10/minute
	// throttle.
	if _, err := src.StreamItems(context.Background(), func(store.CatalogueItem) error { return nil }); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	if r.auths != 1 {
		t.Errorf("the credential was graded %d times across containers + items, want 1", r.auths)
	}
}

// TestAnElevatedCredentialWarnsAndReadsANYWAY is the half of ADR-0058 that is
// easiest to "fix" into a bug.
//
// The ADR turned refusal down by name: refusing "leaves the operator with a
// service that will not talk to them and no visible reason why — the exact
// opposite of principle 3", and it makes the §14 finding LESS visible rather
// than more. So the assertion is a PAIR — the finding is reported AND the
// catalogue is read — because either half alone passes for the wrong build.
func TestAnElevatedCredentialWarnsAndReadsAnyway(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs:  []bookorbit.Library{{ID: 1, Name: "Fiction"}},
		books: map[int64][]bookorbit.Book{1: {proseBook(101, "The Hobbit")}},
		verdict: bookorbit.ScopeVerdict{
			Account: bookorbit.AccountView{
				Username: "usarr", Active: true, ProvisioningMethod: "shared",
				Permissions: []bookorbit.Permission{bookorbit.PermManageUsers},
			},
			Findings: []bookorbit.ScopeFinding{{
				Severity:   bookorbit.ScopeElevated,
				Permission: bookorbit.PermManageUsers,
				Detail:     "can create and edit users, including granting superuser",
			}},
		},
	}
	var buf bytes.Buffer
	src := NewBookOrbitSource(r)
	src.Log = logTo(&buf)

	containers, err := src.Containers(context.Background())
	if err != nil {
		t.Fatalf("Containers refused an elevated credential; ADR-0058 reports rather than refuses: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(containers))
	}
	var items int
	if _, err := src.StreamItems(context.Background(), func(store.CatalogueItem) error {
		items++
		return nil
	}); err != nil {
		t.Fatalf("StreamItems refused an elevated credential: %v", err)
	}
	if items != 1 {
		t.Errorf("read %d items, want 1 — the read must proceed", items)
	}

	got := buf.String()
	if !strings.Contains(got, "elevated=true") {
		t.Errorf("the log line does not say the verdict is elevated:\n%s", got)
	}
	if !strings.Contains(got, "manage_users") {
		t.Errorf("the log line does not name the finding, so an operator cannot act on it:\n%s", got)
	}
	if !strings.Contains(got, "usarr") {
		t.Errorf("the log line does not name the account:\n%s", got)
	}
}

// TestTheScopeVerdictNeverCarriesTheCredential. The verdict is logged on every
// import; a token in that line would be in every operator's log file.
func TestTheScopeVerdictLogLineCarriesNoSecret(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{{ID: 1, Name: "Fiction"}},
		verdict: bookorbit.ScopeVerdict{
			Account: bookorbit.AccountView{Username: "usarr", IsSuperuser: true, Active: true},
			Findings: []bookorbit.ScopeFinding{{
				Severity: bookorbit.ScopeElevated,
				Detail:   "the account is a SUPERUSER, which is every permission there is",
			}},
		},
	}
	var buf bytes.Buffer
	src := NewBookOrbitSource(r)
	src.Log = logTo(&buf)
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	// AccountView is itself the allowlist — no email, no name, no settings blob —
	// so the strongest thing this can check is that nothing outside it appears.
	for _, forbidden := range []string{"accessToken", "Bearer", "magic"} {
		if strings.Contains(buf.String(), forbidden) {
			t.Errorf("the verdict log line contains %q:\n%s", forbidden, buf.String())
		}
	}
}

// TestACredentialThatWillNotOpenStopsTheReadRatherThanReadingNothing.
//
// A failed Authenticate is NOT a verdict — it is a credential that cannot mint,
// and no read is possible under it. Returning an empty container list would tell
// the operator their BookOrbit has no libraries, which is the wrong sentence
// entirely.
func TestACredentialThatWillNotOpenStopsTheRead(t *testing.T) {
	boom := errors.New("bookorbit rejected the magic-link token")
	src := NewBookOrbitSource(&fakeBookOrbitReader{authErr: boom})

	_, err := src.Containers(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the authenticate failure", err)
	}
	if !strings.Contains(err.Error(), "before the first catalogue read") {
		t.Errorf("the error does not say where it happened: %v", err)
	}
}

// ─── containers ──────────────────────────────────────────────────────────────

func TestEveryBookOrbitLibraryBindsAsABookLibrary(t *testing.T) {
	r := &fakeBookOrbitReader{libs: []bookorbit.Library{
		{ID: 1, Name: "Fiction", BookCount: 12},
		{ID: 2, Name: "Manga", BookCount: 900},
	}}
	src := NewBookOrbitSource(r)

	got, err := src.Containers(context.Background())
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d containers, want 2", len(got))
	}
	for _, c := range got {
		// NOTHING IS DECLINED, and the reason is a fact about BookOrbit: its
		// `libraries` table has no type, kind or mediaType column, so there is no
		// container-level answer to decline on. A library named "Manga" is not
		// evidence — the kind decision happens per book, in the item walk.
		if c.Kind != "book" || c.DeclineReason != "" {
			t.Errorf("container %+v: want kind 'book' and no decline reason", c)
		}
	}
	if got[0].RemoteID != "1" || got[0].Name != "Fiction" {
		t.Errorf("got[0] = %+v", got[0])
	}
}

// ─── the prose/comic split, which is the whole of this slice ─────────────────

// TestOnlyProseIsMappedAndTheRestIsCounted.
//
// ⚠️ THE NAME IS NOW HALF FALSE AND IS KEPT ANYWAY. Comics ARE mapped (ADR-0068);
// what is still true, and what this test still guards, is that they are mapped
// AS SOMETHING ELSE — kind 'comic_issue' under a 'comic' parent, never into the
// book cascade — and that the remaining unclassifiable books are counted rather
// than guessed at. Renaming it would break the link to the reasoning in
// docs/REVIEW-LOG.md and in the ADRs that cite it.
func TestOnlyProseIsMappedAndTheRestIsCounted(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{{ID: 1, Name: "Fiction"}, {ID: 2, Name: "Audio"}},
		books: map[int64][]bookorbit.Book{
			1: {
				proseBook(101, "The Hobbit"),
				withFormat(proseBook(102, "A PDF"), "pdf"),
				withFormat(proseBook(103, "Saga, Vol. 1"), "cbz"),
				withFormat(proseBook(104, "Saga, Vol. 2"), "cbr"),
				// No format at all — BookOrbit's own 'unknown'.
				withFormat(proseBook(105, "Still Scanning"), ""),
				// No files at all: getPrimaryBookFile returns null, so 'unknown'
				// again, and it must not be counted as a comic.
				{ID: 106, Title: "Empty Folder", Status: "processing"},
			},
			2: {withFormat(proseBook(201, "Project Hail Mary"), "m4b")},
		},
	}
	var buf bytes.Buffer
	src := NewBookOrbitSource(r)
	src.Log = logTo(&buf)

	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	var mapped []store.CatalogueItem
	read, err := src.StreamItems(context.Background(), func(it store.CatalogueItem) error {
		mapped = append(mapped, it)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamItems: %v", err)
	}

	// READ counts what the WALK delivered, skips included. Report.ItemsRead is
	// documented as "how far the read got, never how many rows are correct", and
	// a count that quietly excluded the skips would make a comic library look
	// like a tiny prose one that read fine.
	if read != 7 {
		t.Errorf("read = %d, want 7 — the count is what the walk delivered, not what was mapped", read)
	}
	// FIVE, not three: the two comics are mapped as issues now. The two
	// unknowns are still skipped, and that pair is what this assertion proves —
	// a build that defaulted an unclassifiable file into SOME kind would give 7.
	if len(mapped) != 5 {
		t.Fatalf("mapped %d items, want 5 (epub, pdf, m4b and two comics as issues)", len(mapped))
	}
	byKind := map[string]int{}
	for _, it := range mapped {
		byKind[it.Kind]++
		switch it.Kind {
		case "book":
			if it.Parent != nil {
				t.Errorf("%q is a prose book and carries a parent; only a child kind may", it.Title)
			}
		case "comic_issue":
			// ⚠️ THE PARENT IS THE ONE PART §6.4's CASCADE MAKES UNFIXABLE LATER
			// (ADR-0068 decision 1), so it is asserted per row rather than in
			// aggregate.
			if it.Parent == nil {
				t.Fatalf("%q is a comic issue with no parent; parent_work_id is never null "+
					"on a row this importer writes, and a parentless issue is a row no "+
					"shipped read can see", it.Title)
			}
			if it.Parent.Kind != "comic" {
				t.Errorf("%q hangs under kind %q, want 'comic' — 'comic' is the SERIES and "+
					"'comic_issue' the issue (ADR-0030)", it.Title, it.Parent.Kind)
			}
		default:
			t.Errorf("%q mapped to kind %q; an audiobook is an EDITION of a book work "+
				"(ADR-0031) and work.kind has no 'audiobook' member", it.Title, it.Kind)
		}
	}
	if byKind["book"] != 3 || byKind["comic_issue"] != 2 {
		t.Errorf("kinds = %v, want 3 book and 2 comic_issue", byKind)
	}

	// ⚠️ TWO TALLIES, AND THIS ASSERTION IS INVERTED RATHER THAN DELETED. It read
	// *"want exactly one container's tally — library 2 skipped nothing"*, because
	// Skipped() dropped every zero. ADR-0063 reversed that: library 2 was WALKED,
	// so it gets a tally saying so, and only a container the walk never reached
	// is absent.
	skips := src.Skipped()
	if len(skips) != 2 {
		t.Fatalf("Skipped() = %+v, want a tally for BOTH walked containers — library 2 "+
			"skipped nothing and that is a measured zero, not an absence", skips)
	}
	if skips[0].RemoteID != "1" || skips[0].Name != "Fiction" {
		t.Errorf("the tally names the wrong container: %+v", skips[0])
	}
	if skips[1].RemoteID != "2" || skips[1].Total() != 0 {
		t.Errorf("library 2's tally = %+v, want remote id 2 and a total of 0", skips[1])
	}
	// ⚠️ ZERO, AND IT IS ASSERTED RATHER THAN DROPPED. ADR-0068's fourth
	// done-check is that this field reads 0 once comics import; a test that
	// simply stopped looking at it could not tell "comics are imported" from
	// "the tally stopped being written".
	if skips[0].Comics != 0 {
		t.Errorf("comics skipped = %d, want 0 — comics are imported as issues now "+
			"(ADR-0068), and this field's expected value is 0", skips[0].Comics)
	}
	if skips[0].Unknown != 2 {
		t.Errorf("unknowns skipped = %d, want 2 — a blank format and a book with no files at all; "+
			"neither is a comic and lumping them together hides the difference", skips[0].Unknown)
	}
	if skips[0].Total() != 2 {
		t.Errorf("total skipped = %d, want 2", skips[0].Total())
	}
	if !strings.Contains(buf.String(), "skipped_comics=0") {
		t.Errorf("the per-container log line does not carry the comic count:\n%s", buf.String())
	}
}

// TestAFailedWalkAbortsTheImportRatherThanDroppingOneLibrary.
//
// Skipping the failed library and carrying on would present a PARTIAL catalogue
// as a complete one, and FullImport stamps last_full_sync_at on completion.
func TestAFailedWalkAbortsTheWholeStream(t *testing.T) {
	boom := errors.New("upstream 500")
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{{ID: 1, Name: "Fiction"}, {ID: 2, Name: "Audio"}},
		books: map[int64][]bookorbit.Book{
			1: {proseBook(101, "The Hobbit")},
			2: {proseBook(201, "Never Reached")},
		},
		walkErr: map[int64]error{1: boom},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}

	read, err := src.StreamItems(context.Background(), func(store.CatalogueItem) error { return nil })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the walk failure", err)
	}
	if read != 1 {
		t.Errorf("read = %d, want 1 — the partial-delivery contract reports what already reached fn", read)
	}
	if len(r.walkedAt) != 1 {
		t.Errorf("walked %v; the stream must stop at the failure rather than skipping to the next library", r.walkedAt)
	}
}

// THE INVARIANT ADR-0063 TURNS ON, AND THE HALF THAT IS EASY TO GET WRONG:
// every container the walk REACHED reports, zero or not, and a container the
// walk never reached reports NOTHING.
//
// ⚠️ IT IS THE DIFFERENCE BETWEEN RETIRING AN IMPRECISION AND MOVING IT. The
// completeness verdict beside this one is measured in Containers(), before a
// single book is read, so an aborted import leaves a verdict on containers it
// never touched — and while the skip read derived its "walked clean" state from
// that verdict, it inherited the same reach. Now that cmd/usarr writes a row per
// element of Skipped(), the ONLY thing keeping the skip rows honest is that this
// slice is built from tallies raised DURING the walk. Seed the map from
// s.containers instead and library 3 comes back with a clean zero it never
// earned, which is the old defect wearing the new mechanism.
//
// Library 1 walks and skips a comic; library 2 walks clean and fails at the end
// of its walk; library 3 is never reached.
func TestSkippedNamesOnlyTheContainersTheWalkReached(t *testing.T) {
	boom := errors.New("upstream 500")
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{
			{ID: 1, Name: "Fiction"}, {ID: 2, Name: "Audio"}, {ID: 3, Name: "Never Reached"},
		},
		books: map[int64][]bookorbit.Book{
			1: {proseBook(101, "The Hobbit"), withFormat(proseBook(102, "Saga"), "cbz")},
			2: {proseBook(201, "Project Hail Mary")},
			3: {withFormat(proseBook(301, "Unseen"), "cbz")},
		},
		walkErr: map[int64]error{2: boom},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if _, err := src.StreamItems(context.Background(),
		func(store.CatalogueItem) error { return nil }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the walk failure", err)
	}

	skips := src.Skipped()
	if len(skips) != 2 {
		t.Fatalf("Skipped() = %+v, want exactly the two containers the walk reached — "+
			"library 3 was never walked and a row for it would say UsArr left nothing out "+
			"of a container it never opened", skips)
	}
	// ⚠️ THE COMIC IS NO LONGER A SKIP (ADR-0068), so library 1's tally is a
	// MEASURED ZERO. That is exactly the state this test exists to distinguish
	// from an absent row, and library 3's absence below is the other half.
	if skips[0].RemoteID != "1" || skips[0].Total() != 0 {
		t.Errorf("library 1's tally = %+v, want remote id 1 with a total of 0 — its comic "+
			"is imported as an issue now, not skipped", skips[0])
	}
	// ⚠️ THE RESIDUAL, ASSERTED SO IT CANNOT BE MISTAKEN FOR A CLOSED CASE. The
	// container the walk died INSIDE still reports, from what it had read — so a
	// clean partial read is indistinguishable from a clean complete one. That is
	// at most one container per import, it is unchanged by ADR-0063, and the ADR
	// records it as still open rather than implying it went away.
	if skips[1].RemoteID != "2" || skips[1].Total() != 0 {
		t.Errorf("library 2's tally = %+v, want remote id 2 with a total of 0", skips[1])
	}
	for _, s := range skips {
		if s.RemoteID == "3" {
			t.Errorf("library 3 was never walked and still reports %+v", s)
		}
	}
}

func TestStreamItemsWithNoContainersIsNotAnError(t *testing.T) {
	// A shared account granted no libraryIds is a real, legal state. Inventing a
	// failure would turn "you granted no libraries" into "your BookOrbit is
	// broken".
	src := NewBookOrbitSource(&fakeBookOrbitReader{})
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	read, err := src.StreamItems(context.Background(), func(store.CatalogueItem) error {
		t.Error("an item was delivered from an instance with no visible libraries")
		return nil
	})
	if err != nil || read != 0 {
		t.Errorf("read = %d, err = %v; want 0 and nil", read, err)
	}
}

// ─── the mapping, field by field ─────────────────────────────────────────────

func TestMapBookProjectsTheFieldsSliceOneOwns(t *testing.T) {
	b := bookorbit.Book{
		ID:            42,
		Title:         "The Making",
		Subtitle:      "The Making of the Atomic Bomb",
		Status:        "present",
		PublishedYear: 1986,
		Language:      "en",
		PageCount:     890,
		HardcoverID:   "hc-42",
		AddedAt:       time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 8, 17, 7, 0, 30, 0, time.UTC),
		Files: []bookorbit.BookFile{
			{ID: 421, Format: "m4b", Role: "primary"},
			{ID: 422, Format: "epub", Role: "content"},
		},
	}
	it := mapBook(b, "7")

	if it.RemoteID != "42" || it.RemoteKind != "book" {
		t.Errorf("remote = %s/%s; remote_kind must be the upstream's OWN noun and it is part of "+
			"ux_sil's key", it.RemoteKind, it.RemoteID)
	}
	if it.ContainerID != "7" {
		t.Errorf("ContainerID = %q; BookCard carries no libraryId, so the route parameter is the "+
			"only thing that can supply it", it.ContainerID)
	}
	if it.Kind != "book" {
		t.Errorf("Kind = %q, want book", it.Kind)
	}
	if it.Title != "The Making" || it.SortTitle != "The Making" {
		t.Errorf("title/sort = %q/%q; BookCard has no sort title and deriving one by stripping an "+
			"article is the parse §6.5 rule 3 forbids", it.Title, it.SortTitle)
	}
	if it.NormalizedTitle != NormalizeTitle("The Making") || it.NormVersion != NormVersion {
		t.Errorf("normalisation = %q/v%d", it.NormalizedTitle, it.NormVersion)
	}
	if it.RemoteSubtype != "m4b" {
		t.Errorf("remote_subtype = %q, want m4b — the PRIMARY file's format, verbatim", it.RemoteSubtype)
	}
	if !it.HasFile {
		t.Error("has_file is false for a 'present' book")
	}
	if !it.PageCount.Valid || it.PageCount.Int64 != 890 {
		t.Errorf("page_count = %+v", it.PageCount)
	}
	if !it.AddedAt.Equal(b.AddedAt) || !it.RemoteUpdatedAt.Equal(b.UpdatedAt) {
		t.Errorf("timestamps = %v / %v", it.AddedAt, it.RemoteUpdatedAt)
	}
	if it.RemotePath != "" {
		t.Errorf("remote_path = %q; folderPath is on BookDetail and slice 1 makes no detail read, "+
			"so inventing one would put a path in the replica that nothing reported", it.RemotePath)
	}
	if it.Overview != "" {
		t.Errorf("overview = %q; `description` is on BookDetail too", it.Overview)
	}

	if len(it.AltTitles) != 1 || it.AltTitles[0].Kind != "alias" ||
		it.AltTitles[0].Title != "The Making of the Atomic Bomb" {
		t.Fatalf("alt titles = %+v; the subtitle lands as an alias so it reaches the FTS document", it.AltTitles)
	}
	if it.AltTitles[0].Normalized != NormalizeTitle("The Making of the Atomic Bomb") {
		t.Errorf("the alias was not normalised: %+v", it.AltTitles[0])
	}
}

func TestASubtitleThatRepeatsTheTitleIsDropped(t *testing.T) {
	b := proseBook(1, "Dune")
	b.Subtitle = "Dune"
	if got := mapBook(b, "1").AltTitles; len(got) != 0 {
		t.Errorf("alt titles = %+v; a duplicate inflates every document it appears in", got)
	}
}

func TestOnlyPresentBooksHaveFiles(t *testing.T) {
	for status, want := range map[string]bool{"present": true, "missing": false, "processing": false} {
		b := proseBook(1, "A Book")
		b.Status = status
		if got := mapBook(b, "1").HasFile; got != want {
			t.Errorf("status %q -> has_file %v, want %v — books.status is a three-valued state "+
				"upstream and only 'present' means the bytes are there", status, got, want)
		}
	}
}

// TestOnlyHardcoverIsWrittenAsAWorkIdentifier is §6.4 amendment 4 applied to the
// three identifier fields BookCard actually carries.
func TestOnlyHardcoverIsWrittenAsAWorkIdentifier(t *testing.T) {
	b := proseBook(1, "A Book")
	b.HardcoverID = "hc-42"
	got := bookOrbitExternalIDs(b)
	if len(got) != 1 {
		t.Fatalf("external ids = %+v, want exactly one", got)
	}
	if got[0].Source != HardcoverBookSource || got[0].Value != "hc-42" || got[0].Confidence < 1.0 {
		t.Errorf("external id = %+v; §6.4 amendment 4 elects hardcover_book work-strong", got[0])
	}
	// The SAME source string the Kavita adapter writes, so a book UsArr knows
	// from both resolves onto ONE work through ux_extid_work_strong.
	if HardcoverBookSource != "hardcover_book" {
		t.Errorf("HardcoverBookSource = %q; it must match kavita.go's literal", HardcoverBookSource)
	}
}

func TestAnUnidentifiedBookIsTheOrdinaryCaseAndNotAnError(t *testing.T) {
	if got := bookOrbitExternalIDs(proseBook(1, "A Book")); len(got) != 0 {
		t.Errorf("external ids = %+v, want none — §6.4's 'not identified' state is the ABSENCE "+
			"of an external_id row", got)
	}
	// And a blank or "0" value is absence rather than an identifier.
	for _, v := range []string{"", "   ", "0"} {
		b := proseBook(1, "A Book")
		b.HardcoverID = v
		if got := bookOrbitExternalIDs(b); len(got) != 0 {
			t.Errorf("hardcoverId %q produced %+v", v, got)
		}
	}
}

// ─── end to end, over cassettes, into a real database ────────────────────────

// TestBookOrbitFullImportFromCassettes is the whole slice against recorded bytes
// and a real migrated SQLite: two containers bound, three books read from the
// first, and all three written — two prose books, and the cbz as a comic ISSUE
// under a SERIES minted into a comic library over the same container ref
// (ADR-0068, ADR-0066 decision 5).
func TestBookOrbitFullImportFromCassettes(t *testing.T) {
	client, err := bookorbit.New(bookorbit.Options{
		BaseURL:        bookOrbitBaseURL,
		MagicLinkToken: testAuthKey,
		HTTPClient:     chainedDoer(t, []string{"bookorbit_libraries", "bookorbit_library_books"}),
		AppVersion:     "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}

	var buf bytes.Buffer
	src := NewBookOrbitSource(client)
	src.Log = logTo(&buf)

	st := newTestStore(t)
	instanceID := fixtureBookOrbitInstance(t, st)
	im := &Importer{Store: st, Source: src, UserID: store.SystemUserID, Now: func() time.Time { return testNow }}

	rep, err := im.FullImport(t.Context(), instanceID)
	if err != nil {
		t.Fatalf("FullImport: %v\nlog:\n%s", err, buf.String())
	}
	if !rep.Completed {
		t.Fatal("the import did not complete")
	}
	if rep.ContainersSeen != 2 || rep.LibrariesCreated != 2 {
		t.Errorf("containers = %d, libraries created = %d, want 2 and 2",
			rep.ContainersSeen, rep.LibrariesCreated)
	}
	// THREE READ, TWO APPLIED. The pair is the assertion: either number alone
	// passes for a build that dropped the comic silently or for one that
	// imported it as a book.
	if rep.ItemsRead != 3 {
		t.Errorf("ItemsRead = %d, want 3 — the walk delivered three cards", rep.ItemsRead)
	}
	if rep.ItemsApplied != 3 {
		t.Errorf("ItemsApplied = %d, want 3 — the cbz is an issue now, not a skip", rep.ItemsApplied)
	}

	// ⚠️ INVERTED BY ADR-0063, NOT DELETED: this wanted *one* container back,
	// because the other was walked clean and clean containers were dropped. Both
	// walked containers now report, and the second one's zero is the point.
	skips := src.Skipped()
	if len(skips) != 2 || skips[0].Comics != 0 || skips[0].Unknown != 0 {
		t.Fatalf("Skipped() = %+v, want both walked containers with a measured zero: "+
			"ADR-0068's fourth done-check is that Comics reads 0 once comics import", skips)
	}
	if skips[1].Total() != 0 {
		t.Errorf("the second container's tally = %+v, want a measured zero", skips[1])
	}

	var works, comics, links, extIDs, aliases int
	readOne := func(q string) int {
		t.Helper()
		var n int
		if err := st.DB().Read().QueryRowContext(t.Context(), q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	works = readOne(`SELECT COUNT(*) FROM work WHERE kind = 'book'`)
	comics = readOne(`SELECT COUNT(*) FROM work WHERE kind = 'comic'`)
	links = readOne(`SELECT COUNT(*) FROM service_item_link WHERE remote_kind = 'book'`)
	extIDs = readOne(`SELECT COUNT(*) FROM external_id WHERE source = 'hardcover_book'`)
	aliases = readOne(`SELECT COUNT(*) FROM work_alt_title WHERE kind = 'alias'`)
	issues := readOne(`SELECT COUNT(*) FROM work WHERE kind = 'comic_issue'`)
	orphans := readOne(
		`SELECT COUNT(*) FROM work WHERE kind = 'comic_issue' AND parent_work_id IS NULL`)
	if works != 2 {
		t.Errorf("book works = %d, want 2", works)
	}
	if comics != 1 || issues != 1 {
		t.Errorf("comic works = %d, comic_issue works = %d, want 1 and 1 — the cbz is one "+
			"issue under one series", comics, issues)
	}
	// ⚠️ ADR-0068's FIRST DONE-CHECK, executed. A parentless issue is not a
	// degraded comic; it is a row no shipped read can see.
	if orphans != 0 {
		t.Errorf("comic_issue rows with no parent = %d, want 0", orphans)
	}
	// The issue is filed into NO library and has NO search document, and both are
	// asserted rather than assumed: writeSearchDoc RETURNS AN ERROR on this kind,
	// so a build that routed the issue through the top-level path would have
	// failed the import above — but a build that dropped the guard would pass
	// that and fail here.
	if n := readOne(`SELECT COUNT(*) FROM library_member lm JOIN work w ON w.id = lm.work_id
	                  WHERE w.kind = 'comic_issue'`); n != 0 {
		t.Errorf("comic_issue library_member rows = %d, want 0", n)
	}
	if n := readOne(`SELECT COUNT(*) FROM search_doc sd JOIN work w ON w.id = sd.work_id
	                  WHERE w.kind = 'comic_issue'`); n != 0 {
		t.Errorf("comic_issue search_doc rows = %d, want 0 — the corpus refuses the kind "+
			"outright, and this importer routes around the guard rather than relaxing it", n)
	}
	// ADR-0066 decision 5, activated: the comic series is filed into a 'comic'
	// library minted over the SAME container ref the book was walked from, and
	// no series work is ever minted into no library at all.
	if n := readOne(`SELECT COUNT(*) FROM library WHERE kind = 'comic' AND id <> 0`); n != 1 {
		t.Errorf("comic libraries = %d, want 1", n)
	}
	if n := readOne(`SELECT COUNT(*) FROM library_source ls
	                   JOIN library l ON l.id = ls.library_id
	                  WHERE l.kind = 'comic' AND ls.container_ref = '1'`); n != 1 {
		t.Errorf("the comic library names container ref '1' %d times, want 1 — decision 5's "+
			"two libraries stand over the SAME library_source container ref", n)
	}
	if n := readOne(`SELECT COUNT(*) FROM library_member lm
	                   JOIN work w ON w.id = lm.work_id
	                   JOIN library l ON l.id = lm.library_id
	                  WHERE w.kind = 'comic' AND l.kind = 'comic'`); n != 1 {
		t.Errorf("comic series filed into a comic library = %d, want 1", n)
	}
	// One link per MAPPED book plus one for the series, under a DIFFERENT
	// remote_kind so a book id and a series id cannot collide in ux_sil.
	if links != 3 {
		t.Errorf("service_item_link rows with remote_kind 'book' = %d, want 3", links)
	}
	if n := readOne(`SELECT COUNT(*) FROM service_item_link WHERE remote_kind = 'series'`); n != 1 {
		t.Errorf("series links = %d, want 1", n)
	}
	if extIDs != 1 {
		t.Errorf("hardcover_book ids = %d, want 1 — only the audiobook card carries one", extIDs)
	}
	if aliases != 1 {
		t.Errorf("alias alt titles = %d, want 1 — The Hobbit's subtitle", aliases)
	}

	// The isbn13 on the cassette's first card is an EDITION identifier and
	// amendment 4 is categorical that it must never satisfy ux_extid_work_strong.
	//
	// ⚠️ THIS ASSERTION'S REASON CHANGED AND ITS EXPECTED VALUE DID NOT. It read
	// "slice 1 writes no edition rows, so the honest answer is no row anywhere",
	// and edition rows now exist — see the two below. What has NOT arrived is a
	// path to write an EDITION-SCOPED external_id: store.CatalogueItem.ExternalIDs
	// are work-scoped, the edition is minted inside store's file writer, and
	// store.FileSet has no field for an identifier. So the row would still have
	// nowhere correct to go, for a new reason.
	if n := readOne(`SELECT COUNT(*) FROM external_id WHERE source LIKE 'isbn%'`); n != 0 {
		t.Errorf("isbn rows = %d, want 0 — an edition-scoped external_id has no writer", n)
	}

	// ── the editions, the files, the credits and the year ───────────────────
	//
	// ⚠️ THESE FOUR BLOCKS REPLACE `edition rows = 0` AND `media_file rows = 0`.
	// The cassette is unchanged: the same three recorded cards that produced no
	// editions now produce two, because the adapter implements FileSource and
	// CreditSource. That is the whole of this change measured against a real
	// recorded response rather than a hand-built struct.
	if n := readOne(`SELECT COUNT(*) FROM edition WHERE is_primary = 1`); n != 2 {
		t.Errorf("primary editions = %d, want 2 — one per mapped book, never one per import", n)
	}
	if n := readOne(`SELECT COUNT(*) FROM edition WHERE format = 'audiobook'`); n != 1 {
		t.Errorf("audiobook editions = %d, want 1 — the m4b card. This is the row the "+
			"Audiobooks screen seeks on; without it every BookOrbit book renders as an Ebook", n)
	}
	if n := readOne(`SELECT COUNT(*) FROM edition WHERE format = 'ebook'`); n != 1 {
		t.Errorf("ebook editions = %d, want 1 — the epub card", n)
	}

	// The file rows carry the OPAQUE SURROGATE and the upstream's size. The
	// cassette's epub says 1234567 bytes; a build that dropped sizeBytes at the
	// decode boundary gives 0 here and nothing else notices.
	var path string
	var size int64
	if err := st.DB().Read().QueryRowContext(t.Context(), `
		SELECT f.path, f.size_bytes FROM media_file f
		  JOIN work w ON w.id = f.work_id WHERE w.title = 'The Hobbit'`).Scan(&path, &size); err != nil {
		t.Fatalf("read The Hobbit's file row: %v", err)
	}
	if path != "bookorbit:bookfile:1011" || size != 1234567 {
		t.Errorf("media_file = (%q, %d), want (bookorbit:bookfile:1011, 1234567)", path, size)
	}

	// The credits, from the SAME cards — no second request exists in either
	// cassette, which is what makes this pass free.
	if n := readOne(`SELECT COUNT(*) FROM work_credit WHERE role = 'author'`); n != 2 {
		t.Errorf("author credits = %d, want 2 — Tolkien and Weir", n)
	}
	if n := readOne(`SELECT COUNT(*) FROM work_credit WHERE role = 'narrator'`); n != 1 {
		t.Errorf("narrator credits = %d, want 1 — Ray Porter, on the audiobook only", n)
	}
	if n := readOne(
		`SELECT COUNT(*) FROM work WHERE kind = 'person' AND title = 'Ray Porter'`); n != 1 {
		t.Errorf("person works named Ray Porter = %d, want 1", n)
	}

	// And the year, which arrives on the credit set rather than on the item.
	if n := readOne(`SELECT COUNT(*) FROM work WHERE year IS NOT NULL`); n != 2 {
		t.Errorf("works with a year = %d, want 2 — publishedYear rides the card", n)
	}
	if n := readOne(`SELECT year FROM work WHERE title = 'The Hobbit'`); n != 1937 {
		t.Errorf("The Hobbit's year = %d, want 1937", n)
	}

	// And the audiobook is a BOOK work whose subtype token says it is audio.
	var subtype string
	if err := st.DB().Read().QueryRowContext(t.Context(), `
		SELECT sil.remote_subtype FROM service_item_link sil
		  JOIN work w ON w.id = sil.work_id WHERE w.title = 'Project Hail Mary'`).Scan(&subtype); err != nil {
		t.Fatalf("read the audiobook's link: %v", err)
	}
	if subtype != "m4b" {
		t.Errorf("remote_subtype = %q, want m4b", subtype)
	}
}

func fixtureBookOrbitInstance(t *testing.T, s *store.Store) int64 {
	t.Helper()
	id, err := s.CreateServiceInstance(t.Context(), store.ServiceInstance{
		Kind: "bookorbit", Role: "library", Name: "bookorbit",
		BaseURL: bookOrbitBaseURL, APIKeyEnc: []byte{1, 2, 3}, KEKID: 1,
		Enabled: true, ManagedBy: "ui",
	})
	if err != nil {
		t.Fatalf("CreateServiceInstance: %v", err)
	}
	return id
}

// ─── ADR-0068's two residue defaults ─────────────────────────────────────────

// withSeries puts a book in one series, as BookOrbit's primary scalar plus the
// displayOrder 0 membership that the upstream keeps equal to it.
func withSeries(b bookorbit.Book, seriesID int64, name string, index float64) bookorbit.Book {
	b.SeriesID = seriesID
	b.SeriesName = name
	b.SeriesIndex = &index
	b.SeriesMemberships = append(b.SeriesMemberships, bookorbit.BookSeriesMembership{
		SeriesID: seriesID, SeriesName: name, SeriesIndex: &index, DisplayOrder: 0,
	})
	return b
}

// alsoInSeries adds a NON-primary membership.
func alsoInSeries(b bookorbit.Book, seriesID int64, name string) bookorbit.Book {
	b.SeriesMemberships = append(b.SeriesMemberships, bookorbit.BookSeriesMembership{
		SeriesID: seriesID, SeriesName: name, DisplayOrder: len(b.SeriesMemberships),
	})
	return b
}

// TestComicsBindToTheSeriesTheCardNames is ADR-0068 decision 3's happy path, and
// it is the one that pins the two issues of ONE series onto ONE parent.
//
// ⚠️ THAT IS THE ADR'S SECOND DONE-CHECK IN MINIATURE: "if the series count
// EQUALS the issue count, the per-row implementation shipped and this check MUST
// FAIL". Two issues under one parent is the smallest shape that can tell the
// accepted implementation from the rejected one, and it is asserted here rather
// than left to a live import.
func TestComicsBindToTheSeriesTheCardNames(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{{ID: 1, Name: "Comics"}},
		books: map[int64][]bookorbit.Book{
			1: {
				withSeries(withFormat(proseBook(101, "Saga #1"), "cbz"), 5, "Saga", 1),
				withSeries(withFormat(proseBook(102, "Saga #2"), "cbz"), 5, "Saga", 2),
			},
		},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	var mapped []store.CatalogueItem
	if _, err := src.StreamItems(context.Background(), func(it store.CatalogueItem) error {
		mapped = append(mapped, it)
		return nil
	}); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	if len(mapped) != 2 {
		t.Fatalf("mapped %d, want 2", len(mapped))
	}
	parents := map[string]bool{}
	for _, it := range mapped {
		if it.Kind != "comic_issue" {
			t.Fatalf("%q mapped to kind %q, want comic_issue", it.Title, it.Kind)
		}
		if it.IsOneshot {
			t.Errorf("%q is in a named series and is flagged as a one-shot", it.Title)
		}
		if it.Parent == nil || it.Parent.Synthesized {
			t.Fatalf("%q: parent = %+v, want the upstream series rather than a synthesized one",
				it.Title, it.Parent)
		}
		if it.Parent.Title != "Saga" || it.Parent.RemoteID != "5" {
			t.Errorf("%q hangs under %+v, want the series the card names", it.Title, it.Parent)
		}
		if !it.NumberSort.Valid {
			t.Errorf("%q carries no number_sort; seriesIndex is on the card", it.Title)
		}
		if it.NumberText.Valid {
			t.Errorf("%q carries number_text %q; BookOrbit has no issue-number TOKEN and "+
				"rendering the float back into one would be UsArr inventing an upstream's "+
				"text (§6.5 rule 3)", it.Title, it.NumberText.String)
		}
		parents[it.Parent.RemoteID] = true
	}
	if len(parents) != 1 {
		t.Errorf("two issues of one series produced %d parents, want 1 — a parent per ROW is "+
			"the shape ADR-0066 pre-emptively refused and ADR-0068's done-check fails on", len(parents))
	}
	// No residue: this is the ordinary case, and its ZERO has to be measured so
	// a later zero cannot be mistaken for "nobody looked" (ADR-0063).
	res := src.Comics()
	if len(res) != 1 || res[0].Total() != 0 || res[0].ExtraMemberships != 0 {
		t.Errorf("Comics() = %+v, want one container reporting a measured zero", res)
	}
}

// TestAComicWithNoSeriesGetsASynthesizedOneShotSeries is ADR-0068 decision 2.
//
// ⚠️ is_oneshot IS ASSERTED, not merely allowed. The ADR's words are that "a
// column with a DEFAULT 0 and no writer is a deaf column, and this project has
// found several", so the test that matters is the one that fails when the writer
// disappears.
func TestAComicWithNoSeriesGetsASynthesizedOneShotSeries(t *testing.T) {
	r := &fakeBookOrbitReader{
		libs: []bookorbit.Library{{ID: 1, Name: "Comics"}},
		books: map[int64][]bookorbit.Book{
			1: {withFormat(proseBook(101, "The Sandman: Endless Nights"), "cbz")},
		},
	}
	var buf bytes.Buffer
	src := NewBookOrbitSource(r)
	src.Log = logTo(&buf)
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	var mapped []store.CatalogueItem
	if _, err := src.StreamItems(context.Background(), func(it store.CatalogueItem) error {
		mapped = append(mapped, it)
		return nil
	}); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	if len(mapped) != 1 {
		t.Fatalf("mapped %d, want 1 — a comic with no series is NEVER silently dropped", len(mapped))
	}
	it := mapped[0]
	if it.Kind != "comic_issue" {
		t.Errorf("kind = %q, want comic_issue — it is never promoted to a 'comic' work in "+
			"its own right, which would make /library/comics mean two different things "+
			"depending on upstream metadata quality", it.Kind)
	}
	if !it.IsOneshot {
		t.Error("is_oneshot is not set; ADR-0068 decision 2 WRITES the flag")
	}
	if it.Parent == nil || !it.Parent.Synthesized {
		t.Fatalf("parent = %+v, want a synthesized single-issue series", it.Parent)
	}
	if it.Parent.Kind != "comic" || it.Parent.Title != "The Sandman: Endless Nights" {
		t.Errorf("parent = %+v, want a 'comic' series named for the book", it.Parent)
	}
	// DETERMINISM IS THE REQUIREMENT. A ref derived from anything but the book id
	// would mint a second series on every import and double /library/comics.
	if it.Parent.RemoteID != "oneshot:101" {
		t.Errorf("synthesized ref = %q, want a deterministic id derived from the book",
			it.Parent.RemoteID)
	}

	res := src.Comics()
	if len(res) != 1 || res[0].SynthesizedSeries != 1 {
		t.Fatalf("Comics() = %+v, want one synthesized series counted", res)
	}
	if res[0].MultiSeries != 0 || len(res[0].Sample) != 0 {
		t.Errorf("Comics() = %+v: a comic with no series declines nothing — the declined set "+
			"is empty by definition", res[0])
	}
	if !strings.Contains(buf.String(), "synthesized_series=1") {
		t.Errorf("the residue is not in the log:\n%s", buf.String())
	}
}

// TestExtraSeriesMembershipsAreRecordedAndNeverResolved is ADR-0068 decision 3's
// refusal half.
//
// It asserts BOTH directions, which is the only way to state "recorded, not
// resolved": the extra membership appears in the residue, and it appears NOWHERE
// in what reaches the store. The seam it must not build toward — work_relation
// and the v0.3 fuzzy tier — is not reachable from a CatalogueItem at all, which
// is what makes the second half structural; the assertion pins the first.
func TestExtraSeriesMembershipsAreRecordedAndNeverResolved(t *testing.T) {
	b := withSeries(withFormat(proseBook(101, "Crossover #1"), "cbz"), 5, "Saga", 1)
	b = alsoInSeries(b, 9, "Universe Event")
	b = alsoInSeries(b, 12, "Artist Collection")

	r := &fakeBookOrbitReader{
		libs:  []bookorbit.Library{{ID: 1, Name: "Comics"}},
		books: map[int64][]bookorbit.Book{1: {b}},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	var mapped []store.CatalogueItem
	if _, err := src.StreamItems(context.Background(), func(it store.CatalogueItem) error {
		mapped = append(mapped, it)
		return nil
	}); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	if len(mapped) != 1 {
		t.Fatalf("mapped %d, want 1 — three memberships are still ONE issue", len(mapped))
	}
	// ⚠️ THE SCALAR WINS, AND IT IS NOT memberships[0] BY ACCIDENT. BookOrbit
	// maintains seriesId equal to the displayOrder 0 membership on every write
	// (measured at commit 73b7877d), so binding on the scalar and binding on
	// memberships[displayOrder = 0] are the same binding by construction.
	if p := mapped[0].Parent; p == nil || p.RemoteID != "5" {
		t.Fatalf("parent = %+v, want the primary series 5 and nothing else", p)
	}
	if mapped[0].IsOneshot {
		t.Error("a book in three series is not a one-shot")
	}

	res := src.Comics()
	if len(res) != 1 {
		t.Fatalf("Comics() = %+v, want one container", res)
	}
	if res[0].MultiSeries != 1 || res[0].ExtraMemberships != 2 {
		t.Errorf("residue = %+v, want one multi-series book and TWO declined memberships — "+
			"the primary is not declined, it is the parent", res[0])
	}
	if len(res[0].Sample) != 1 {
		t.Fatalf("sample = %+v, want one entry", res[0].Sample)
	}
	got := res[0].Sample[0]
	if got.BookID != 101 || len(got.SeriesIDs) != 2 {
		t.Errorf("sample = %+v, want book 101 and series 9 and 12", got)
	}
	for _, id := range got.SeriesIDs {
		if id == 5 {
			t.Error("the PRIMARY series is in the declined sample; it was acted on, not declined")
		}
	}
}

// TestTheResidueSampleIsBounded is the row-size guard.
//
// A 20,000-comic library whose operator never matched a series would otherwise
// put 20,000 entries in one sync_report row. The cover pass refused that shape in
// terms, and the counts — not the sample — are what ADR-0068 decision 4 asks for.
func TestTheResidueSampleIsBounded(t *testing.T) {
	books := make([]bookorbit.Book, 0, comicResidueSampleCap*2)
	for i := range comicResidueSampleCap * 2 {
		id := int64(1000 + i)
		b := withSeries(withFormat(proseBook(id, "Issue"), "cbz"), 5, "Saga", float64(i))
		books = append(books, alsoInSeries(b, 9, "Event"))
	}
	r := &fakeBookOrbitReader{
		libs:  []bookorbit.Library{{ID: 1, Name: "Comics"}},
		books: map[int64][]bookorbit.Book{1: books},
	}
	src := NewBookOrbitSource(r)
	if _, err := src.Containers(context.Background()); err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if _, err := src.StreamItems(context.Background(),
		func(store.CatalogueItem) error { return nil }); err != nil {
		t.Fatalf("StreamItems: %v", err)
	}
	res := src.Comics()
	if len(res) != 1 {
		t.Fatalf("Comics() = %+v", res)
	}
	if res[0].ExtraMemberships != comicResidueSampleCap*2 {
		t.Errorf("declined memberships = %d, want every one counted — the CAP is on the "+
			"sample, never on the count", res[0].ExtraMemberships)
	}
	if len(res[0].Sample) != comicResidueSampleCap {
		t.Errorf("sample = %d entries, want the cap of %d", len(res[0].Sample), comicResidueSampleCap)
	}
}
