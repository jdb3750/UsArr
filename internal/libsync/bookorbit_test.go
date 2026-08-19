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
	if len(mapped) != 3 {
		t.Fatalf("mapped %d items, want 3 (epub, pdf, m4b)", len(mapped))
	}
	for _, it := range mapped {
		if it.Kind != "book" {
			t.Errorf("%q mapped to kind %q; an audiobook is an EDITION of a book work "+
				"(ADR-0031) and work.kind has no 'audiobook' member", it.Title, it.Kind)
		}
	}

	skips := src.Skipped()
	if len(skips) != 1 {
		t.Fatalf("Skipped() = %+v, want exactly one container's tally — library 2 skipped nothing", skips)
	}
	if skips[0].RemoteID != "1" || skips[0].Name != "Fiction" {
		t.Errorf("the tally names the wrong container: %+v", skips[0])
	}
	if skips[0].Comics != 2 {
		t.Errorf("comics skipped = %d, want 2 (cbz and cbr)", skips[0].Comics)
	}
	if skips[0].Unknown != 2 {
		t.Errorf("unknowns skipped = %d, want 2 — a blank format and a book with no files at all; "+
			"neither is a comic and lumping them together hides the difference", skips[0].Unknown)
	}
	if skips[0].Total() != 4 {
		t.Errorf("total skipped = %d, want 4", skips[0].Total())
	}
	if !strings.Contains(buf.String(), "skipped_comics=2") {
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
// first, two of them written, one counted as a comic.
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
	if rep.ItemsApplied != 2 {
		t.Errorf("ItemsApplied = %d, want 2 — the cbz is skipped", rep.ItemsApplied)
	}

	skips := src.Skipped()
	if len(skips) != 1 || skips[0].Comics != 1 || skips[0].Unknown != 0 {
		t.Fatalf("Skipped() = %+v, want one container with exactly one comic", skips)
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
	if works != 2 {
		t.Errorf("book works = %d, want 2", works)
	}
	if comics != 0 {
		t.Errorf("comic works = %d, want 0 — this slice writes no comics at all", comics)
	}
	if links != 2 {
		t.Errorf("service_item_link rows = %d, want 2", links)
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
