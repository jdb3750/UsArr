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
	env, instanceID := boImport(t, bo)

	// ⚠️ THE FIXTURE NOW HAS TWO IMPORTS AND AN ACCEPT BETWEEN THEM, and the
	// shape is ADR-0048's ordering rather than a workaround. The first import
	// replicates the container's contents and creates nothing. The user is then
	// offered ONE proposal — at BookOrbit's declared fallback `book`, because
	// that is all `Containers()` can say — and accepts it. The second import is
	// where bindProvisional's retype runs: the library is empty (its member
	// filing matched `w.kind = 'book'` and every work here is a comic), it is
	// auto-managed, and exactly one container feeds it, so the evidence retypes
	// it in place. One library, the container's own name, no qualifier.
	acceptLibraries(t, env, instanceID, acceptSpec{ref: "1", name: "Comics", kind: "book"})
	boReimport(t, env, instanceID)

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
	env, instanceID := boImport(t, bo)

	// ⚠️ DECISION 1 SURVIVES ADR-0048 AND ITS SHAPE MOVES ONE STEP. The container
	// is still REPORTED, still observed, and still offered as a proposal at the
	// adapter's fallback kind — a walk that yielded nothing does not make the
	// container disappear from the Accept screen, which is what decision 1 is
	// really protecting. What it no longer does is create the row unasked. Accept
	// it and the row is exactly what decision 1 specifies: bound, at `book`, with
	// an honest zero.
	acceptLibraries(t, env, instanceID, acceptSpec{ref: "1", name: "Oddities", kind: "book"})

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
	env, instanceID := boImport(t, bo)
	acceptLibraries(t, env, instanceID, acceptSpec{ref: "1", name: "Fiction", kind: "book"})

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
	// Accepted at the adapter's fallback, then re-imported so the retype runs —
	// the same two-step this file's first test explains.
	acceptLibraries(t, env, instanceID, acceptSpec{ref: "1", name: "Comics", kind: "book"})
	boReimport(t, env, instanceID)
	if rows := boLibraryRows(t, env); len(rows) != 1 || rows[0].Kind != "comic" {
		t.Fatalf("after the retype: %+v, want one Comics/comic", rows)
	}

	// The same instance, imported AGAIN — the run this test exists for, where the
	// container's kind is the fallback `book` and the library standing over it is
	// `comic`.
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
// # WHAT THIS TEST CATCHES, stated first because the instrument was replaced
//
// The wrong behaviour it goes red on is: THE LIBRARY NAMES ON THE SCREEN ARE A
// FUNCTION OF THE WALK ORDER. Before ADR-0048 the mechanism was concrete — the
// eager bind minted `Fiction` at BookOrbit's fallback `book`, the first comic
// RETYPED that row in place (store.bindProvisional), and so a comics-first walk
// left `Fiction` standing over the COMIC library while the prose arriving after
// it got `Fiction (Books)`, where the identical upstream library walked
// prose-first came out `Fiction` + `Fiction (Comics)`. The same upstream
// library, two different names, decided by traversal order: an implementation
// fact no reader of the Libraries screen can interpret.
//
// # WHY THE INSTRUMENT CHANGED, AND WHAT IT MEASURES NOW
//
// The mechanism is gone: the import derives no name at all (ADR-0048), so the
// old assertion — that the derived pair matches between orders — has nothing to
// read. What has NOT gone is the property. The names come from Accept, so the
// test drives the identical two walk orders, accepts the IDENTICAL two proposals
// in both, and asserts the resulting screen is the same. Any code that re-derived
// or rewrote a name from what a walk found would make the two orders disagree,
// and this goes red on exactly that — the same wrong behaviour, read through a
// different instrument.
//
// ⚠️ IT ALSO ASSERTS THE STRONGER FACT THE REMOVAL BOUGHT: before the Accept, the
// import has produced NO library and therefore no name for an order to decide.
// That arm cannot be expressed against the old code at all.
//
// The re-import arm is unchanged in intent: the naming rule may not turn a
// re-import into a rename.
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

			// THE IMPORT NAMED NOTHING, whichever order it walked.
			if rows := boLibraryRows(t, env); len(rows) != 0 {
				t.Fatalf("%s: the import produced %+v; it creates no library, so there is no "+
					"name for a walk order to decide", tc.order, rows)
			}

			// The user ticks the same two proposals in both orders. These are the
			// names §17.8's screen offers: the container's own for the kind it
			// declared, and a qualified one for the kind the walk discovered.
			acceptLibraries(t, env, instanceID,
				acceptSpec{ref: "1", name: "Fiction", kind: "book"},
				acceptSpec{ref: "1", name: "Fiction (Comics)", kind: "comic"},
			)

			want := map[string]string{"Fiction": "book", "Fiction (Comics)": "comic"}
			assertMixedFiction(t, env, want, tc.order)

			// AND IT STAYS PUT. The second import binds against libraries it did
			// not create, and a swap here would move the owner's permalinks on a
			// run that found nothing new.
			boReimport(t, env, instanceID)
			assertMixedFiction(t, env, want, tc.order+", re-imported")
		})
	}
}

// TestASecondKindInALaterImportDoesNotRenameWhatIsAlreadyThere is half two, and
// it is the constraint a refactor loses quietly, because losing it makes the
// screen MORE consistent rather than obviously broken.
//
// # WHAT THIS TEST CATCHES, unchanged by the instrument swap
//
// The wrong behaviour is: A LIBRARY THE OWNER ALREADY HAS IS RENAMED OR
// RE-SLUGGED TO MAKE ROOM FOR A KIND THAT ARRIVED LATER. It is detected by the
// two things a rename moves and an id alone does not — `library.id`, which every
// library_member points at, and `library.slug`, which is in every permalink the
// owner holds (store.slugify: "durable by design").
//
// Before ADR-0048 the mechanism was the create path: a comics-only container was
// one `comic` library named `Fiction`, prose turned up in a LATER import, and
// the tempting fix was to rename `Fiction` to `Fiction (Comics)` so the prose
// could take the plain name. The decision was that the NEWCOMER takes the
// qualifier even when the newcomer is the prose: stable beats tidy.
//
// # The instrument now
//
// The import creates nothing, so the newcomer is a PROPOSAL rather than a
// library — which removes the motive for the rename but not the possibility of
// one. The assertion is therefore the same comparison, taken across a later
// import that brings a second kind: id, name and slug of the row the owner has
// been looking at, before and after. Any path that reshaped it to make room goes
// red here exactly as it did before.
//
// ⚠️ AND THE NEWCOMER IS ASSERTED TO HAVE LANDED, not merely to have been
// refused a rename: its works are in the replica, unfiled, which is what makes
// the proposal beside it carry a real count.
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
	// Accepted at the fallback kind and retyped by the evidence on the next run,
	// which is how a comics-only container becomes one `comic` library called
	// `Fiction` (see TestAComicsOnlyBookOrbitContainerIsOneComicLibrary).
	acceptLibraries(t, env, instanceID, acceptSpec{ref: "1", name: "Fiction", kind: "book"})
	boReimport(t, env, instanceID)

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

	// THE SCREEN IS UNCHANGED. The prose did not displace anything, did not take
	// the name, and did not appear as a library of its own — it is a proposal.
	assertMixedFiction(t, env, map[string]string{"Fiction": "comic"},
		"a second kind arriving later")

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
		t.Errorf("library %d's kind = %q, want \"comic\" — the prose gets its OWN library when "+
			"it is accepted, it does not repurpose the one holding the comics", wasID, nowKind)
	}

	// AND THE PROSE IS IN THE REPLICA, unfiled. Without this the test would pass
	// on a build that simply dropped the second kind on the floor, which refuses
	// the rename by refusing the data.
	if n := countIn(t, env,
		`SELECT COUNT(*) FROM work WHERE kind = 'book' AND deleted_at IS NULL`); n != 1 {
		t.Errorf("book works = %d, want 1 — the newcomer must be REPLICATED and unfiled, or "+
			"the proposal offered for it counts nothing", n)
	}
	if n := countIn(t, env, `SELECT COUNT(*) FROM library_member lm JOIN work w ON w.id = lm.work_id
	                          WHERE w.kind = 'book'`); n != 0 {
		t.Errorf("%d prose works were filed into a library; nobody has accepted one for them", n)
	}
}

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
