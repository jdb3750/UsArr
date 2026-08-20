package main

import (
	"net/http"
	"testing"
)

// A BookOrbit container's LIBRARIES FOLLOW WHAT THE WALK ACTUALLY FOUND, and
// the three shapes a container can have are asserted here as three separate
// imports because they are three different answers.
//
// # What this is fixing, measured before it was changed
//
// BookOrbit's `libraries` table has no type, kind or mediaType column
// (bookorbit.Library), so `Containers()` stamps `bookKind` on every container it
// reports and `bindOneContainer` created a `book` library up front. On a
// COMICS-ONLY container that invented a `book` library that should not exist,
// and ADR-0066 decision 5's `comic` sibling was then minted beside it carrying a
// kind qualifier it never needed. Measured on this exact fixture before the
// change, through GET /api/v1/libraries:
//
//	name="Comics"          kind="book"  item_count=0
//	name="Comics (Comics)" kind="comic" item_count=1
//
// Two rows for one upstream library, one of them permanently empty, and the
// qualifier — whose whole job is to say WHICH OF TWO a row is — standing next to
// a row nobody would otherwise have made.
//
// # Why the qualifier is the tell rather than the bug
//
// `Fiction (Comics)` is right when a container really is mixed: it says which of
// two libraries this is, and its PRESENCE is the signal that a container was
// split (store.kindQualifier). A container that was never split has nothing to
// disambiguate, so a qualifier there is a statement about a second library that
// should not exist.
//
// # The rule this rests on
//
// ADR-0066's Consequences hand the next adapter "a container you walked is a
// library you bind, whatever the walk made of its contents", ruled on 2026-08-19
// to mean BIND VERSUS DECLINE and nothing wider — see that ADR's rider. Nothing
// below declines a container. What moves is only the KIND and COUNT of the
// libraries a bound container produces, which ADR-0066's own decision 5 already
// makes conditional on observed contents (store.parentBinding mints the comic
// sibling lazily, on the first comic actually reached).

// TestAComicsOnlyBookOrbitContainerIsOneComicLibrary is the shape that was
// wrong: one container, one library, named for the container, NO qualifier.
func TestAComicsOnlyBookOrbitContainerIsOneComicLibrary(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Comics", 2)}
	bo.books = map[int][]map[string]any{
		1: {
			boBook(105, "Saga, Vol. 1", map[string]any{
				"files":       []map[string]any{boFile(1051, "cbz", "primary")},
				"seriesId":    5,
				"seriesName":  "Saga",
				"seriesIndex": 1,
			}),
			boBook(106, "Saga, Vol. 2", map[string]any{
				"files":       []map[string]any{boFile(1061, "cbz", "primary")},
				"seriesId":    5,
				"seriesName":  "Saga",
				"seriesIndex": 2,
			}),
		},
	}
	env, _ := boImport(t, bo)

	rows := boLibraryRows(t, env)
	if len(rows) != 1 {
		t.Fatalf("GET /api/v1/libraries returned %d rows, want 1 — a comics-only container "+
			"has nothing to disambiguate, so it is one library and not a pair: %+v",
			len(rows), rows)
	}
	if rows[0].Name != "Comics" {
		t.Errorf("the library is named %q, want %q — the container's own name, with NO kind "+
			"qualifier. `Comics (Comics)` is the shape this test exists to refuse: the "+
			"qualifier says WHICH OF TWO a row is, and there is no second row",
			rows[0].Name, "Comics")
	}
	if rows[0].Kind != "comic" {
		t.Errorf("the library's kind = %q, want \"comic\" — every book in this container is a "+
			"comic, and `book` here is the constant `Containers()` stamped before a single "+
			"book had been read", rows[0].Kind)
	}
	// The series is IN it. A retype that left the members behind in a library
	// that no longer exists on the screen would pass every assertion above.
	if rows[0].ItemCount != 1 {
		t.Errorf("item_count = %d, want 1 — the Saga series", rows[0].ItemCount)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE id <> 0`); n != 1 {
		t.Errorf("library rows = %d, want 1 — an empty `book` library that the API no longer "+
			"lists is still a row somebody will find", n)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM library_source WHERE container_kind = 'remote_library'`); n != 1 {
		t.Errorf("library_source rows = %d, want 1", n)
	}
}

// TestAWhollySkippedBookOrbitContainerIsSTILLBOUND is ADR-0066 DECISION 1, and
// it is the edge this whole change had to not break.
//
// ⚠️ READ THIS BEFORE TOUCHING THE BIND PATH. Decision 1: "A container whose
// every item is skipped is BOUND, and no adapter declines one for being wholly
// skipped. `Containers()` keeps reporting it, `bindOneContainer` keeps binding
// it, and the library renders on §17.8 with an item count of zero and a
// sentence." The obvious way to make libraries follow observed content — mint
// nothing until the walk proves a kind — creates NO library for a container that
// yields nothing mappable, which deletes exactly the row that decision exists to
// guarantee, and it would look like the change working. The row is not deferred:
// the eager bind creates it exactly as it always did, and only its `kind` can
// still move, and only while it is empty.
//
// # The kind such a row gets, and why
//
// `book` — the adapter's declared fallback. A container that yielded nothing has
// produced NO evidence about its contents, so there is nothing for a kind to
// follow; `book` is what `Containers()` says and `library.kind` is "EDITABLE
// (§6.5 rule 4)" in the schema's own comment, so the user is not stuck with it.
// This is ADR-0066's residual bullet, unchanged, on the only shape that still
// reaches it: comics now import (ADR-0068), so a wholly skipped container is one
// whose files BookOrbit itself cannot classify.
func TestAWhollySkippedBookOrbitContainerIsSTILLBOUND(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Oddities", 2)}
	bo.books = map[int][]map[string]any{
		// Two books whose files carry no format at all. BookOrbit's own
		// getBookMediaKind calls that "unknown", StreamItems counts them and
		// hands NEITHER to fn, so this container walks clean and yields nothing.
		1: {
			boBook(301, "Something Processing", map[string]any{
				"status": "processing",
				"files":  []map[string]any{boFile(3011, "", "primary")},
			}),
			boBook(302, "Something Else", map[string]any{
				"status": "processing",
				"files":  []map[string]any{boFile(3021, "", "primary")},
			}),
		},
	}
	env, _ := boImport(t, bo)

	rows := boLibraryRows(t, env)
	if len(rows) != 1 {
		t.Fatalf("GET /api/v1/libraries returned %d rows, want 1 — ADR-0066 decision 1: a "+
			"container whose every item is skipped is STILL BOUND, and its row is what the "+
			"whole ADR exists to guarantee. Zero rows here is not this change working, it "+
			"is the row silently deleted: %+v", len(rows), rows)
	}
	if rows[0].Name != "Oddities" {
		t.Errorf("the bound library is named %q, want the container's name %q",
			rows[0].Name, "Oddities")
	}
	if rows[0].Kind != "book" {
		t.Errorf("the bound library's kind = %q, want \"book\" — the adapter's declared "+
			"fallback. A container that yielded nothing produced no evidence for a kind to "+
			"follow, and inventing one from an empty walk is the guess ADR-0066 refused",
			rows[0].Kind)
	}
	if rows[0].ItemCount != 0 {
		t.Errorf("item_count = %d, want 0 — decision 1's honest zero", rows[0].ItemCount)
	}
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM library_source WHERE container_kind = 'remote_library'`); n != 1 {
		t.Errorf("library_source rows = %d, want 1 — §17.8 renders `library` JOINED to "+
			"`library_source`, so a library with no source row is invisible, which is the "+
			"same screen as no row at all", n)
	}
}

// TestAProseOnlyBookOrbitContainerIsUnchanged is the case that must not have
// moved. It is here because a change that makes comics-only right by making
// prose-only wrong has traded one invented library for another.
func TestAProseOnlyBookOrbitContainerIsUnchanged(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 2)}
	bo.books = map[int][]map[string]any{
		1: {
			boBook(101, "The Hobbit", map[string]any{"pageCount": 310}),
			boBook(102, "Project Hail Mary", map[string]any{
				"files": []map[string]any{boFile(1021, "m4b", "primary")},
			}),
		},
	}
	env, _ := boImport(t, bo)

	rows := boLibraryRows(t, env)
	if len(rows) != 1 {
		t.Fatalf("GET /api/v1/libraries returned %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].Name != "Fiction" || rows[0].Kind != "book" || rows[0].ItemCount != 2 {
		t.Errorf("prose-only library = %+v, want Fiction/book/2 — unchanged from before "+
			"libraries followed content", rows[0])
	}
}

// TestABookOrbitContainerKeepsItsKindAcrossImports is the half a one-shot import
// cannot see, and it is where the naive version of this change breaks.
//
// The kind a container is BOUND at is still the adapter's fallback `book` on
// every import, because BookOrbit still supplies none. On the SECOND import of a
// comics-only container the library standing over that container ref is `comic`,
// so a bind that insisted on its own guessed kind would find no match, refuse to
// join a library of a different kind, and mint `Comics (Books)` — an empty
// second row, on every import, for ever. store.CatalogueContainer.KindProvisional
// is what makes the eager pass adopt the evidence instead.
func TestABookOrbitContainerKeepsItsKindAcrossImports(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Comics", 1)}
	bo.books = map[int][]map[string]any{
		1: {
			boBook(105, "Saga, Vol. 1", map[string]any{
				"files":       []map[string]any{boFile(1051, "cbz", "primary")},
				"seriesId":    5,
				"seriesName":  "Saga",
				"seriesIndex": 1,
			}),
		},
	}
	env, instanceID := boImport(t, bo)
	if rows := boLibraryRows(t, env); len(rows) != 1 {
		t.Fatalf("after the FIRST import: %d rows, want 1: %+v", len(rows), rows)
	}

	// The same instance, imported again. reimport drives the real import path
	// rather than reconstructing it.
	boReimport(t, env, instanceID)

	rows := boLibraryRows(t, env)
	if len(rows) != 1 {
		t.Fatalf("after the SECOND import: %d rows, want 1 — the container's kind is a "+
			"fallback on every import, and a bind that re-guessed `book` here mints an empty "+
			"`Comics (Books)` beside the real one, on every import: %+v", len(rows), rows)
	}
	if rows[0].Name != "Comics" || rows[0].Kind != "comic" {
		t.Errorf("after the second import the library is %+v, want Comics/comic", rows[0])
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library WHERE id <> 0`); n != 1 {
		t.Errorf("library rows = %d, want 1 — a row the API stopped listing is still a row", n)
	}
}

// TestAMixedBookOrbitContainerNAMESTHESAMEEITHERWAYROUND is half one of the
// naming rule, and the ONLY thing it asserts is that the two walk orders agree.
//
// # What it is refusing
//
// A mixed container's two libraries cannot both be called `Fiction`, so one of
// them carries the kind qualifier. Which one used to depend on WHICH KIND THE
// WALK REACHED FIRST: the eager bind mints `Fiction` at BookOrbit's fallback
// `book`, and the first comic RETYPES that row in place (store.bindProvisional),
// so a comics-first walk left `Fiction` standing over the comic library and the
// prose that arrived after it got `Fiction (Books)` — while the identical
// library walked prose-first came out `Fiction` + `Fiction (Comics)`.
//
// ⚠️ THE SAME UPSTREAM LIBRARY, TWO DIFFERENT NAMES, DECIDED BY TRAVERSAL ORDER.
// That is an implementation fact no reader of the Libraries screen can
// interpret — the exact defect that got the ordinal `Fiction (2)` refused
// (store.kindQualifier): it says a split happened and not one word about which
// half he is looking at.
//
// # The rule, and why it is prose that keeps the plain name
//
// `book` is the kind the CONTAINER declares — BookOrbit's `Containers()` stamps
// it before a single book is read — so it is the one answer available that no
// walk order can move. The comic library is the one that was discovered, and the
// discovery is what the qualifier announces.
//
// ⚠️ IT IS NOT A DEFERRED BIND. Nothing waits for the walk to finish: the row is
// still created up front for every container (see
// TestAWhollySkippedBookOrbitContainerIsSTILLBOUND, which is what a deferred
// bind fails). The name is settled when the SECOND kind arrives, from a fact the
// run already holds — whether this run minted the row (store.runMint).
func TestAMixedBookOrbitContainerNAMESTHESAMEEITHERWAYROUND(t *testing.T) {
	comic := boBook(105, "Saga, Vol. 1", map[string]any{
		"files":       []map[string]any{boFile(1051, "cbz", "primary")},
		"seriesId":    5,
		"seriesName":  "Saga",
		"seriesIndex": 1,
	})
	prose := boBook(101, "The Hobbit", map[string]any{"pageCount": 310})

	for _, tc := range []struct {
		order string
		books []map[string]any
	}{
		{"comics first", []map[string]any{comic, prose}},
		{"prose first", []map[string]any{prose, comic}},
	} {
		t.Run(tc.order, func(t *testing.T) {
			bo := newFakeBookOrbit(t, boMagicLink)
			bo.libraries = []map[string]any{boLibrary(1, "Fiction", 2)}
			bo.books = map[int][]map[string]any{1: tc.books}
			env, instanceID := boImport(t, bo)

			want := map[string]string{"Fiction": "book", "Fiction (Comics)": "comic"}
			assertMixedFiction(t, env, want, tc.order)

			// AND IT STAYS PUT. The naming rule may not turn a re-import into a
			// rename: the second import binds against libraries it did not mint,
			// which is the other half of the rule, and a swap here would move the
			// owner's permalinks on a run that found nothing new.
			boReimport(t, env, instanceID)
			assertMixedFiction(t, env, want, tc.order+", re-imported")
		})
	}
}

// TestASecondKindInALaterImportDoesNotRenameWhatIsAlreadyThere is half two, and
// it is the constraint a refactor loses quietly, because losing it makes the
// screen MORE consistent rather than obviously broken.
//
// A comics-only container is one `comic` library named for the container. When
// prose turns up in a LATER import, the pair that results is `Fiction` (comic) +
// `Fiction (Books)` — the plain name on the comic library and the qualifier on
// the prose, which is the OPPOSITE of what the same two kinds produce inside one
// walk.
//
// ⚠️ THAT INCONSISTENCY IS THE DECISION, NOT A GAP IN IT. Making the two cases
// agree means renaming `Fiction` — a row the owner has had on his Libraries
// screen since the last import, and whose slug is in every permalink to it
// (store.slugify: "durable by design"). A library that renames itself under him
// reads as a bug; two containers that split differently because they were
// discovered differently is merely untidy. Stable beats tidy, so the newcomer
// takes the qualifier EVEN WHEN THE NEWCOMER IS THE PROSE.
func TestASecondKindInALaterImportDoesNotRenameWhatIsAlreadyThere(t *testing.T) {
	bo := newFakeBookOrbit(t, boMagicLink)
	bo.libraries = []map[string]any{boLibrary(1, "Fiction", 1)}
	bo.books = map[int][]map[string]any{
		1: {
			boBook(105, "Saga, Vol. 1", map[string]any{
				"files":       []map[string]any{boFile(1051, "cbz", "primary")},
				"seriesId":    5,
				"seriesName":  "Saga",
				"seriesIndex": 1,
			}),
		},
	}
	env, instanceID := boImport(t, bo)
	rows := boLibraryRows(t, env)
	if len(rows) != 1 || rows[0].Name != "Fiction" || rows[0].Kind != "comic" {
		t.Fatalf("after the comics-only import: %+v, want one Fiction/comic", rows)
	}
	// The row the owner has been looking at, by the two things a rename would
	// move: its id is what every library_member points at, its slug is what
	// every permalink to it contains.
	wasID, wasSlug := boLibraryIdentity(t, env, "Fiction")

	// A prose book appears in the same upstream library and the owner syncs.
	bo.books[1] = append(bo.books[1], boBook(101, "The Hobbit", map[string]any{"pageCount": 310}))
	boReimport(t, env, instanceID)

	assertMixedFiction(t, env, map[string]string{
		"Fiction": "comic", "Fiction (Books)": "book",
	}, "a second kind arriving later")

	nowID, nowSlug := boLibraryIdentity(t, env, "Fiction")
	if nowID != wasID {
		t.Errorf("the library called `Fiction` is now id %d, was %d — the name did not move, "+
			"it was TAKEN, which is a rename wearing a different shape: the row the owner "+
			"knew as Fiction is somewhere else now", nowID, wasID)
	}
	if nowSlug != wasSlug {
		t.Errorf("`Fiction`'s slug = %q, was %q — library.slug is durable by design and every "+
			"permalink the owner holds contains it", nowSlug, wasSlug)
	}
	if nowKind := boLibraryKind(t, env, wasID); nowKind != "comic" {
		t.Errorf("library %d's kind = %q, want \"comic\" — the prose gets its OWN library, it "+
			"does not repurpose the one holding the comics", wasID, nowKind)
	}
}

// assertMixedFiction asserts the whole Libraries screen at once — name to kind,
// nothing more and nothing less — because both halves of the naming rule are
// claims about the SET of rows, and a per-row assertion passes happily beside a
// third library nobody expected.
func assertMixedFiction(t *testing.T, env *testEnv, want map[string]string, when string) {
	t.Helper()
	got := map[string]string{}
	for _, r := range boLibraryRows(t, env) {
		got[r.Name] = r.Kind
	}
	if len(got) != len(want) {
		t.Fatalf("%s: the Libraries screen shows %v, want %v", when, got, want)
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("%s: the Libraries screen shows %v, want %v — a mixed container's two "+
				"libraries are named by KIND, never by the order the walk found them in",
				when, got, want)
		}
	}
}

func boLibraryIdentity(t *testing.T, env *testEnv, name string) (int64, string) {
	t.Helper()
	var id int64
	var slug string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(),
		`SELECT id, slug FROM library WHERE name = ?`, name).Scan(&id, &slug); err != nil {
		t.Fatalf("read the library named %q: %v", name, err)
	}
	return id, slug
}

func boLibraryKind(t *testing.T, env *testEnv, id int64) string {
	t.Helper()
	var kind string
	if err := env.app.store.DB().Read().QueryRowContext(t.Context(),
		`SELECT kind FROM library WHERE id = ?`, id).Scan(&kind); err != nil {
		t.Fatalf("read library %d's kind: %v", id, err)
	}
	return kind
}

// ── the helpers these tests share ───────────────────────────────────────────

// boLibraryRow is the slice of GET /api/v1/libraries these tests read. The kind
// is on the wire because it is half of what this change moves; asserting it out
// of the database instead would prove the row exists and not that the screen
// says the right thing about it.
type boLibraryRow struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ItemCount int64  `json:"item_count"`
}

func boLibraryRows(t *testing.T, env *testEnv) []boLibraryRow {
	t.Helper()
	var got struct {
		Items []boLibraryRow `json:"items"`
	}
	env.do(t, "GET", "/api/v1/libraries", nil, &got)
	return got.Items
}

// boImport adds one BookOrbit through the real HTTP API and waits for its
// bootstrap import, which is the only path that produces a catalogue.
func boImport(t *testing.T, bo *fakeBookOrbit) (*testEnv, int64) {
	t.Helper()
	env := boSetUp(t)
	armBootstrapImport(t, env)
	var created serviceBody
	env.do(t, "POST", "/api/v1/services", map[string]any{
		"kind": "bookorbit", "name": "BookOrbit", "base_url": bo.URL(), "api_key": boMagicLink,
	}, &created)
	waitForImport(t, env, created.ID)
	return env, created.ID
}

// boReimport runs the SAME instance's import a second time, through the same
// endpoint the owner's "sync now" uses.
//
// It waits on the container-list read rather than on a timestamp:
// last_full_sync_at is second-resolution, so two fixture imports inside one
// second write the identical string and a wait on it returns before the second
// import has done anything.
func boReimport(t *testing.T, env *testEnv, instanceID int64) {
	t.Helper()
	before := countIn(t, env, `SELECT COUNT(*) FROM sync_report`)
	if code, body := syncNow(t, env, instanceID); code != http.StatusAccepted {
		t.Fatalf("POST sync = %d, want 202: %s", code, body)
	}
	waitFor(t, "the second import to write its own sync_report rows", func() bool {
		return countIn(t, env, `SELECT COUNT(*) FROM sync_report`) > before
	})
}
