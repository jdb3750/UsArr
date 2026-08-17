package libsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// person builds a PersonDto with the roles field DELIBERATELY POPULATED WRONG.
//
// Every fixture person here claims to be a Writer AND a Colorist AND a
// Publisher, because that is what a real instance returns: PersonDto.roles is
// the set of roles that person holds ACROSS THE WHOLE INSTANCE, not in this
// series. If the mapping ever starts reading `roles` instead of the array a
// person arrived in, every test in this file goes red at once.
func person(name string) kavita.PersonDto {
	return kavita.PersonDto{
		Name:  name,
		Roles: []kavita.PersonRole{kavita.PersonRoleWriter, kavita.PersonRoleColorist, kavita.PersonRolePublisher},
	}
}

// TestEveryKavitaPersonRoleIsAccountedFor reads the VENDORED SPEC and requires a
// decision for every PersonRole member it declares — the same guard shape
// TestEveryLibraryTypeMemberIsMapped uses, for the same reason.
//
// "Accounted for" means the member appears in kavitaCreditRoles, either with a
// role (it becomes a credit) or with empty roles (it is dropped, and the table
// says why). A fourteenth member added upstream goes red here rather than being
// quietly ignored by a mapping that never looks at it.
func TestEveryKavitaPersonRoleIsAccountedFor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "specs", "kavita.json"))
	if err != nil {
		t.Fatalf("read the vendored spec: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas struct {
				PersonRole struct {
					Enum         []int64  `json:"enum"`
					EnumVarNames []string `json:"x-enum-varnames"`
				} `json:"PersonRole"`
				SeriesMetadataDto struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"SeriesMetadataDto"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode the vendored spec: %v", err)
	}
	names := doc.Components.Schemas.PersonRole.EnumVarNames
	if len(names) != len(doc.Components.Schemas.PersonRole.Enum) || len(names) == 0 {
		t.Fatalf("the spec's PersonRole has %d members and %d names; the assertion below "+
			"would be vacuous", len(doc.Components.Schemas.PersonRole.Enum), len(names))
	}

	// Kavita's PersonRole member names and SeriesMetadataDto's array names are
	// singular/plural forms of each other (Writer→writers, CoverArtist→
	// coverArtists). The mapping keys on the ARRAY name, so this is the join.
	arrayFor := map[string]string{
		"Writer": "writers", "Penciller": "pencillers", "Inker": "inkers",
		"Colorist": "colorists", "Letterer": "letterers", "CoverArtist": "coverArtists",
		"Editor": "editors", "Publisher": "publishers", "Character": "characters",
		"Translator": "translators", "Imprint": "imprints", "Team": "teams",
		"Location": "locations",
	}

	mapped := map[string]creditRole{}
	for _, cr := range kavitaCreditRoles {
		mapped[cr.name] = cr
	}

	for _, member := range names {
		array, ok := arrayFor[member]
		if !ok {
			t.Errorf("PersonRole.%s is a member of the vendored spec and this test does not "+
				"know which SeriesMetadataDto array carries it. Kavita added a role; decide "+
				"what it is — a credit or a drop — in kavitaCreditRoles, and add the array "+
				"name here.", member)
			continue
		}
		if _, ok := doc.Components.Schemas.SeriesMetadataDto.Properties[array]; !ok {
			t.Errorf("SeriesMetadataDto no longer declares %q, which PersonRole.%s needs",
				array, member)
		}
		if _, ok := mapped[array]; !ok {
			t.Errorf("kavitaCreditRoles has no entry for %q (PersonRole.%s). Every member is "+
				"accounted for either as a credit or as an explicit drop; silently ignoring "+
				"one is how a whole category of credit goes missing.", array, member)
		}
	}

	// And nothing in the table is an array the spec does not have.
	for _, cr := range kavitaCreditRoles {
		if _, ok := doc.Components.Schemas.SeriesMetadataDto.Properties[cr.name]; !ok {
			t.Errorf("kavitaCreditRoles maps %q, which SeriesMetadataDto does not declare", cr.name)
		}
	}
	if len(kavitaCreditRoles) != len(names) {
		t.Errorf("kavitaCreditRoles has %d entries for %d PersonRole members", len(kavitaCreditRoles), len(names))
	}
}

// TestCreditRolesAreMembersOfTheSchemaVocabulary checks the other end of the
// mapping: every role kavitaCreditRoles produces must be a member of
// work_credit.role's CHECK, read out of the SHIPPED MIGRATION.
//
// Without this the adapter and the database can disagree, and the disagreement
// surfaces as CreditResult.CreditsRejected on a user's real library rather than
// in CI.
func TestCreditRolesAreMembersOfTheSchemaVocabulary(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "db", "migrations", "00007_work_credit.sql"))
	if err != nil {
		t.Fatalf("read migration 0007: %v", err)
	}
	ddl := string(raw)
	i := strings.Index(ddl, "CHECK (role IN (")
	if i < 0 {
		t.Fatal("migration 0007 no longer declares work_credit.role's CHECK")
	}
	list := ddl[i:]
	list = list[:strings.Index(list, "))")]

	for _, cr := range kavitaCreditRoles {
		for _, role := range []string{cr.comicRole, cr.bookRole} {
			if role == "" {
				continue
			}
			if !strings.Contains(list, "'"+role+"'") {
				t.Errorf("%s maps to role %q, which work_credit.role's CHECK does not accept. "+
					"Every such credit is silently rejected at import time.", cr.name, role)
			}
		}
	}
}

// TestKavitaRoleMappingIsExhaustiveAndCorrect is the mapping table itself,
// asserted by executing it against a metadata DTO with EVERY array populated.
//
// It is written as the full expected output rather than as a spot check,
// because the interesting property is "and nothing else": the five dropped
// arrays must produce no credits at all, and a future edit that starts mapping
// characters would otherwise pass every other test here.
func TestKavitaRoleMappingIsExhaustiveAndCorrect(t *testing.T) {
	md := kavita.SeriesMetadataDto{
		Writers:      []kavita.PersonDto{person("Brian K. Vaughan")},
		CoverArtists: []kavita.PersonDto{person("Fiona Staples")},
		Pencillers:   []kavita.PersonDto{person("A Penciller")},
		Inkers:       []kavita.PersonDto{person("An Inker")},
		Colorists:    []kavita.PersonDto{person("A Colorist")},
		Letterers:    []kavita.PersonDto{person("Fonografiks")},
		Editors:      []kavita.PersonDto{person("An Editor")},
		Translators:  []kavita.PersonDto{person("A Translator")},

		// The five that must produce nothing.
		Publishers: []kavita.PersonDto{person("Image Comics")},
		Imprints:   []kavita.PersonDto{person("Vertigo")},
		Teams:      []kavita.PersonDto{person("The Justice League")},
		Characters: []kavita.PersonDto{person("Batman")},
		Locations:  []kavita.PersonDto{person("Gotham City")},
	}

	for _, tc := range []struct {
		kind string
		want []string
	}{
		{
			kind: "comic",
			want: []string{
				"cover_artist:Fiona Staples", "colorist:A Colorist", "editor:An Editor",
				"inker:An Inker", "letterer:Fonografiks", "penciller:A Penciller",
				"translator:A Translator", "writer:Brian K. Vaughan",
			},
		},
		{
			kind: "book",
			// The ONE difference, and the consequential one: writers become
			// AUTHORS on a book. Kavita has no `authors` array — its Writer role
			// is where an EPUB's dc:creator lands — so mapping it to 'writer'
			// would leave work_credit.role's 'author' member with no writer and
			// file every novelist under a comics role.
			want: []string{
				"author:Brian K. Vaughan", "colorist:A Colorist", "cover_artist:Fiona Staples",
				"editor:An Editor", "inker:An Inker", "letterer:Fonografiks",
				"penciller:A Penciller", "translator:A Translator",
			},
		},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			got := make([]string, 0, len(tc.want))
			for _, c := range creditsFromMetadata(md, tc.kind) {
				got = append(got, c.Role+":"+c.Name)
			}
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if strings.Join(got, " | ") != strings.Join(want, " | ") {
				t.Errorf("credits for a %s =\n  %v\nwant\n  %v", tc.kind, got, want)
			}
			// Explicitly: none of the five dropped arrays leaked.
			for _, banned := range []string{"Image Comics", "Vertigo", "The Justice League", "Batman", "Gotham City"} {
				for _, g := range got {
					if strings.HasSuffix(g, ":"+banned) {
						t.Errorf("%q became a credit (%s). Publishers, imprints and teams are "+
							"ORGANISATIONS and characters and locations are not people at all; "+
							"credits.go names the reason for each.", banned, g)
					}
				}
			}
		})
	}
}

// TestCreditPositionsAreScopedPerRole pins that position is billing order WITHIN
// a role, not a running index across the whole set — which is what the primary
// key (work_id, role, position, creator_work_id) means by it.
func TestCreditPositionsAreScopedPerRole(t *testing.T) {
	md := kavita.SeriesMetadataDto{
		Writers:   []kavita.PersonDto{person("First Writer"), person("Second Writer")},
		Letterers: []kavita.PersonDto{person("First Letterer"), person("Second Letterer")},
	}
	got := map[string]int{}
	for _, c := range creditsFromMetadata(md, "comic") {
		got[c.Role+":"+c.Name] = c.Position
	}
	want := map[string]int{
		"writer:First Writer": 0, "writer:Second Writer": 1,
		"letterer:First Letterer": 0, "letterer:Second Letterer": 1,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("positions = %v, want %v — position restarts at 0 for each role", got, want)
	}
}

// TestUnnamedPeopleAreDropped covers the two names that cannot be stored: empty,
// and punctuation-only (which normalises to nothing and so has no dedupe key).
//
// PersonDto declares `name` as required AND nullable in the same schema, so an
// empty one is reachable from a real instance.
func TestUnnamedPeopleAreDropped(t *testing.T) {
	md := kavita.SeriesMetadataDto{
		Writers: []kavita.PersonDto{
			{Name: ""},
			{Name: "   "},
			{Name: "???"},
			{Name: "Real Person"},
		},
	}
	got := creditsFromMetadata(md, "comic")
	if len(got) != 1 || got[0].Name != "Real Person" {
		t.Fatalf("credits = %+v, want only the one real name", got)
	}
	// And the survivor's position is 0: a dropped name must not leave a gap that
	// makes the first real credit look like the second-billed one.
	if got[0].Position != 0 {
		t.Errorf("the only credit is at position %d, want 0", got[0].Position)
	}
}

// TestNormalizedNameIsTheRealNormaliser ties the dedupe key the adapter computes
// to the shipped normaliser, rather than to a plausible-looking lowercase.
func TestNormalizedNameIsTheRealNormaliser(t *testing.T) {
	md := kavita.SeriesMetadataDto{Writers: []kavita.PersonDto{person("Brian K. Vaughan")}}
	got := creditsFromMetadata(md, "comic")
	if len(got) != 1 {
		t.Fatalf("credits = %+v", got)
	}
	if want := NormalizeTitle("Brian K. Vaughan"); got[0].NormalizedName != want {
		t.Errorf("NormalizedName = %q, want %q", got[0].NormalizedName, want)
	}
	if got[0].NormalizedName != "brian k vaughan" {
		t.Errorf("NormalizedName = %q, want %q — this is the string the dedupe keys on, "+
			"so it is asserted literally as well as against the function",
			got[0].NormalizedName, "brian k vaughan")
	}
}

// ─── StreamCredits ──────────────────────────────────────────────────────────

// fakeCreditClient answers SeriesMetadata from a map and can be told to fail.
type fakeCreditClient struct {
	KavitaReader
	md    map[int32]kavita.SeriesMetadataDto
	fail  map[int32]error
	calls []int32
}

func (f *fakeCreditClient) SeriesMetadata(_ context.Context, id int32) (kavita.SeriesMetadataDto, error) {
	f.calls = append(f.calls, id)
	if err, ok := f.fail[id]; ok {
		return kavita.SeriesMetadataDto{}, err
	}
	return f.md[id], nil
}

// TestStreamCreditsDropsAPerSeriesFailure pins the failure policy: a 404 on one
// series costs that series its credits and NOTHING ELSE.
//
// An import over a library where one series was deleted between the two passes
// must still credit the other 1,999.
func TestStreamCreditsDropsAPerSeriesFailure(t *testing.T) {
	c := &fakeCreditClient{
		md: map[int32]kavita.SeriesMetadataDto{
			1: {Writers: []kavita.PersonDto{person("A")}},
			3: {Writers: []kavita.PersonDto{person("C")}},
		},
		fail: map[int32]error{2: kavita.ErrNotFound},
	}
	src := NewKavitaSource(c)

	var got []store.CreditSet
	n, err := src.StreamCredits(t.Context(), []CreditRequest{
		{RemoteKind: "series", RemoteID: "1", Kind: "comic"},
		{RemoteKind: "series", RemoteID: "2", Kind: "comic"},
		{RemoteKind: "series", RemoteID: "3", Kind: "comic"},
	}, func(s store.CreditSet) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("one 404 failed the whole pass: %v", err)
	}
	if n != 2 || len(got) != 2 {
		t.Fatalf("delivered %d sets (n=%d), want 2 — the 404'd series is skipped and the "+
			"other two are not", len(got), n)
	}
	if got[0].RemoteID != "1" || got[1].RemoteID != "3" {
		t.Errorf("delivered %s and %s, want 1 and 3", got[0].RemoteID, got[1].RemoteID)
	}
}

// TestStreamCreditsStopsOnAnOpenBreaker is the other half: a condition under
// which continuing would issue hundreds of calls that cannot succeed.
func TestStreamCreditsStopsOnAnOpenBreaker(t *testing.T) {
	c := &fakeCreditClient{
		md:   map[int32]kavita.SeriesMetadataDto{1: {}},
		fail: map[int32]error{2: kavita.ErrBreakerOpen},
	}
	src := NewKavitaSource(c)

	var got int
	n, err := src.StreamCredits(t.Context(), []CreditRequest{
		{RemoteKind: "series", RemoteID: "1", Kind: "comic"},
		{RemoteKind: "series", RemoteID: "2", Kind: "comic"},
		{RemoteKind: "series", RemoteID: "3", Kind: "comic"},
	}, func(store.CreditSet) error { got++; return nil })
	if err == nil {
		t.Fatal("an open circuit breaker did not stop the credit pass")
	}
	if n != 1 || got != 1 {
		t.Errorf("delivered %d sets before stopping, want 1", got)
	}
	if len(c.calls) != 2 {
		t.Errorf("made %d calls after the breaker opened, want 2 (the pass stops at the "+
			"first one that reports it)", len(c.calls))
	}
}

// TestStreamCreditsDeliversAnEmptySet pins the rule that "no credits" is a
// STATEMENT and not a skip. Skipping it is how a penciller the operator deleted
// in Kavita lives forever in the replica.
func TestStreamCreditsDeliversAnEmptySet(t *testing.T) {
	c := &fakeCreditClient{md: map[int32]kavita.SeriesMetadataDto{1: {}}}
	src := NewKavitaSource(c)

	var got []store.CreditSet
	n, err := src.StreamCredits(t.Context(), []CreditRequest{
		{RemoteKind: "series", RemoteID: "1", Kind: "comic"},
	}, func(s store.CreditSet) error { got = append(got, s); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(got) != 1 {
		t.Fatalf("a series with no credits delivered %d sets, want 1", len(got))
	}
	if len(got[0].Credits) != 0 {
		t.Errorf("the empty set carries %d credits", len(got[0].Credits))
	}
}

// ─── The importer's credit phase ────────────────────────────────────────────

// creditingSource is a fakeSource that also reports credits, so the importer's
// phase-B wiring is exercised without an HTTP round trip.
type creditingSource struct {
	fakeSource

	// byRemoteID is what StreamCredits answers with, per item.
	byRemoteID map[string][]store.Credit

	// sawKinds records the work.kind the importer passed for each request. It is
	// the property that would silently break if CreditRequest stopped carrying
	// the kind: 'writers' maps to two different roles by kind.
	sawKinds map[string]string
}

func (c *creditingSource) StreamCredits(
	_ context.Context, reqs []CreditRequest, fn func(store.CreditSet) error,
) (int, error) {
	if c.sawKinds == nil {
		c.sawKinds = map[string]string{}
	}
	n := 0
	for _, r := range reqs {
		c.sawKinds[r.RemoteID] = r.Kind
		if err := fn(store.CreditSet{
			RemoteKind: r.RemoteKind, RemoteID: r.RemoteID, Credits: c.byRemoteID[r.RemoteID],
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// TestFullImportWritesCredits is the end-to-end assertion, against a REAL
// MIGRATED DATABASE populated by the real importer.
//
// It covers the three things the wiring can get wrong independently: that the
// credit pass runs at all, that it runs over the items that COMMITTED (and so
// have a service_item_link to resolve), and that the kind travels with the
// request.
func TestFullImportWritesCredits(t *testing.T) {
	s := newTestStore(t)
	src := &creditingSource{
		fakeSource: fakeSource{
			containers: []store.CatalogueContainer{{RemoteID: "2", Name: "Ebooks", Kind: "book"}},
			items:      genItems(3, "2", "book"),
		},
		byRemoteID: map[string][]store.Credit{
			"0": {{Role: "author", Name: "Susanna Clarke", NormalizedName: "susanna clarke"}},
			"1": {{Role: "author", Name: "Susanna Clarke", NormalizedName: "susanna clarke"}},
			"2": {{Role: "author", Name: "Alan Moore", NormalizedName: "alan moore"}},
		},
	}
	im := &Importer{Store: s, Source: src, UserID: store.SystemUserID, Now: func() time.Time { return testNow }}

	rep, err := im.FullImport(t.Context(), fixtureInstance(t, s))
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if !rep.Completed {
		t.Fatal("the import did not complete")
	}
	if rep.CreditItemsRead != 3 {
		t.Errorf("CreditItemsRead = %d, want 3", rep.CreditItemsRead)
	}
	if rep.Credits.CreditsWritten != 3 {
		t.Errorf("CreditsWritten = %d, want 3", rep.Credits.CreditsWritten)
	}
	// TWO people for THREE credits: Susanna Clarke is deduped across two books.
	if rep.Credits.PeopleCreated != 2 || rep.Credits.PeopleReused != 1 {
		t.Errorf("people = %d created / %d reused, want 2/1",
			rep.Credits.PeopleCreated, rep.Credits.PeopleReused)
	}
	for id, kind := range src.sawKinds {
		if kind != "book" {
			t.Errorf("the credit request for item %s carried kind %q, want \"book\" — "+
				"without it a book's writers are credited as comic writers", id, kind)
		}
	}

	// And the corpus still holds the three BOOKS and NEITHER AUTHOR — the §6.1
	// exclusion, asserted at the importer level as well as at the store level,
	// because this is the path a real import takes.
	assertCorpusIsBooksOnly(t, s, 3)
}

// TestFullImportSkipsCreditsForASourceThatReportsNone is principle 3 applied to
// an adapter: a Source that does not implement CreditSource is not an error.
func TestFullImportSkipsCreditsForASourceThatReportsNone(t *testing.T) {
	s := newTestStore(t)
	src := &fakeSource{
		containers: []store.CatalogueContainer{{RemoteID: "2", Name: "Ebooks", Kind: "book"}},
		items:      genItems(2, "2", "book"),
	}
	im := &Importer{Store: s, Source: src, UserID: store.SystemUserID, Now: func() time.Time { return testNow }}

	rep, err := im.FullImport(t.Context(), fixtureInstance(t, s))
	if err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if !rep.Completed {
		t.Fatal("the import did not complete")
	}
	if rep.CreditItemsRead != 0 || rep.Credits.CreditsWritten != 0 {
		t.Errorf("a source with no credits reported %d read / %d written",
			rep.CreditItemsRead, rep.Credits.CreditsWritten)
	}
	assertCorpusIsBooksOnly(t, s, 2)
}

// assertCorpusIsBooksOnly checks the corpus holds exactly want documents and no
// person among them.
func assertCorpusIsBooksOnly(t *testing.T, s *store.Store, want int) {
	t.Helper()
	if n := countRows(t, s, `SELECT COUNT(*) FROM search_doc`); n != want {
		t.Errorf("search_doc = %d rows, want %d", n, want)
	}
	if n := countRows(t, s, `
		SELECT COUNT(*) FROM search_doc d JOIN work w ON w.id = d.work_id
		 WHERE w.kind = 'person'`); n != 0 {
		t.Errorf("%d person work(s) reached the search corpus through the importer. "+
			"docs/reference/schema.md §6.1 excludes 'person' from the FTS corpus, the "+
			"navigation enum and the Tier 1 prefix index, because there is no person "+
			"screen in any milestone", n)
	}
}

// TestCreditsAreNotFetchedForItemsThatNeverCommitted pins the ordering rule: a
// credit is resolved through the service_item_link the ITEM pass wrote, so an
// item from a batch that never committed must not even be asked about.
func TestCreditsAreNotFetchedForItemsThatNeverCommitted(t *testing.T) {
	s := newTestStore(t)
	src := &creditingSource{
		fakeSource: fakeSource{
			containers: []store.CatalogueContainer{{RemoteID: "2", Name: "Ebooks", Kind: "book"}},
			items:      genItems(10, "2", "book"),
			failAfter:  4,
			streamErr:  errors.New("connection reset mid-array"),
		},
	}
	im := &Importer{
		Store: s, Source: src, UserID: store.SystemUserID,
		BatchRows: 2, Now: func() time.Time { return testNow },
	}

	rep, err := im.FullImport(t.Context(), fixtureInstance(t, s))
	if err == nil {
		t.Fatal("a stream that died mid-array reported success")
	}
	if rep.Completed {
		t.Error("a partial import reported Completed")
	}
	// The credit pass never runs on a failed item stream — FullImport returns
	// before it — so nothing was asked and nothing was written.
	if len(src.sawKinds) != 0 {
		t.Errorf("the credit pass asked about %d items after the item stream failed; "+
			"those items' links may not exist", len(src.sawKinds))
	}
	if rep.Credits.CreditsWritten != 0 {
		t.Errorf("%d credits were written over a partial import", rep.Credits.CreditsWritten)
	}
}
