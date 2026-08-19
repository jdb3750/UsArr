package servarr

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Contract tests against the VENDORED OpenAPI document.
//
// They are vendored rather than fetched because *Arr apps guard
// app.UseSwagger() behind `if (BuildInfo.IsDebug)`, so a production instance does
// not serve /docs/v1/openapi.json — and because the gate makes no network call
// for it: `make check`'s only two are the vulnerability scans. Drift detection is
// `make spec-drift`, which is allowed the network and is NOT in the gate; it is
// not this file.
//
// What these assert is narrow and on purpose: that the Go structs cover every
// property the spec declares for the resources UsArr consumes, that the enums
// accept every documented value, and that the endpoints UsArr calls exist with the
// parameters it sends. They do NOT assert the spec is correct — it is not, always.
// IndexerResource.status is declared and is always null in practice, and
// docs/DEVELOPMENT.md §5 lists other places where the controller wins.
//
// ── ONE SPEC, NOT TWO, AND WHY (ADR-0047) ────────────────────────────────────
// ADR-0046 gave Kavita a FLOOR (the release the owner runs) and a CEILING
// (`develop`), because Kavita regenerates its checked-in spec most releases and
// the two files genuinely differ. Prowlarr does not regenerate per release, so
// that pattern does not port: `src/Prowlarr.Api.V1/openapi.json` is the SAME GIT
// BLOB at tag v2.5.2.5491 and at `develop` (measured 2026-08-17), and a
// floor/ceiling split here would vendor 145 KB twice and produce a green that
// proves exactly what one copy proved.
//
// The consequence is worse than "one file instead of two": the one file describes
// NEITHER ref reliably. It was last regenerated upstream on 2025-06-07
// (60740fa25) and 33 release tags have shipped since. It already misdescribes
// both — see knownSpecDivergences. Treat it as evidence about the API's SHAPE,
// and Prowlarr's source at a tag as evidence about a release.

// specSkewAdvice is appended to the failures in this file that a reader has to
// diagnose before fixing.
//
// WHY it is this long: a bare "the vendored spec no longer declares X" has three
// possible causes needing three different actions, and picking the wrong one is
// how a real upstream change gets papered over at 2am. Naming the three is the
// whole point; shortening this to one line would restore the problem.
const specSkewAdvice = "\n\n" +
	"WHICH OF THESE THREE IS IT? They need different fixes — decide before editing anything:\n" +
	"  (1) THEIR PROWLARR IS TOO OLD. The thing exists upstream but not on the release the owner\n" +
	"      runs (v2.5.2.5491 — owner-confirmed 2026-08-17, stated of his actual install).\n" +
	"      FIX: make internal/servarr degrade when it is absent. Do not relax this test.\n" +
	"  (2) OUR EXPECTATION IS TOO NEW. UsArr models a name upstream never shipped, or renamed.\n" +
	"      FIX: correct internal/servarr. The spec is not the thing that is wrong.\n" +
	"  (3) THE VENDORED DOCUMENT IS SIMPLY STALE — the default, and the likeliest.\n" +
	"      api/specs/prowlarr.json was last regenerated upstream on 2025-06-07 (60740fa25),\n" +
	"      33 releases ago, so it describes neither v2.5.2.5491 nor develop.\n" +
	"      FIX: read Prowlarr's SOURCE at the tag (src/Prowlarr.Api.V1/**.cs) — the controller\n" +
	"      wins over the schema — then record the gap in knownSpecDivergences,\n" +
	"      in internal/servarr/contract_test.go, so the next reader inherits it.\n" +
	"NEVER hand-edit api/specs/prowlarr.json to make this pass. Re-vendor it whole, from the URL\n" +
	"in api/specs/SOURCES.md, and update vendoredSpecBlob with it. See ADR-0047."

// vendoredSpecBlob is the git blob SHA-1 of api/specs/prowlarr.json.
//
// The git blob hash rather than a plain SHA-256 so that it can be compared to
// upstream WITHOUT downloading anything:
//
//	git rev-parse v2.5.2.5491:src/Prowlarr.Api.V1/openapi.json
//	git rev-parse develop:src/Prowlarr.Api.V1/openapi.json
//
// Both printed this value on 2026-08-17. That identity is a coincidence of
// upstream neglect, not a property, and `make spec-drift` is the guard that
// notices when it ends. This constant is the offline half: it only says the
// vendored bytes are the bytes this suite was written against.
const vendoredSpecBlob = "134d31d7df5e80714c454a6224e7449df512c55e"

// gitBlobSHA1 computes the object name git would give these bytes: SHA-1 over
// "blob <len>\x00" + content. Reimplemented rather than shelled out to `git`
// because the gate must not depend on a git checkout being present.
func gitBlobSHA1(b []byte) string {
	h := sha1.New()
	// git names a blob by SHA-1 over "blob <len>\x00" + content. sha1's Write
	// never errors, so the header and the content are both written unchecked —
	// the header is assembled with strconv rather than fmt.Fprintf so there is no
	// error return to drop and no Write([]byte(fmt.Sprintf(...))) for staticcheck
	// to rewrite back into one.
	h.Write([]byte("blob " + strconv.Itoa(len(b)) + "\x00"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// TestVendoredSpecIsThePinnedBlob is the offline half of ADR-0047's guard.
//
// It cannot detect upstream regenerating their spec — nothing offline can. What
// it does detect is the vendored file changing under this suite: a re-vendor, a
// hand-edit, a bad merge. Deterministic, no network, so it belongs in the gate;
// the ref-identity check that DOES need the network is `make spec-drift`.
func TestVendoredSpecIsThePinnedBlob(t *testing.T) {
	raw, err := os.ReadFile(specPath())
	if err != nil {
		t.Fatalf("reading the vendored spec: %v", err)
	}
	got := gitBlobSHA1(raw)
	if got != vendoredSpecBlob {
		t.Errorf("api/specs/prowlarr.json is git blob %s; this suite is pinned to %s.\n"+
			"The file changed. If you re-vendored it deliberately, that is the good case: update\n"+
			"vendoredSpecBlob and the SHA-256 row in api/specs/SOURCES.md, then run `make spec-drift`\n"+
			"and re-read knownSpecDivergences — a regenerated spec is exactly when those entries stop\n"+
			"being true. If you did NOT re-vendor it, something hand-edited a vendored upstream\n"+
			"document, which is never allowed: restore it with\n"+
			"  git checkout -- api/specs/prowlarr.json",
			got, vendoredSpecBlob)
	}
}

type openAPI struct {
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Components struct {
		Schemas map[string]openAPISchema `json:"schemas"`
	} `json:"components"`
}

type openAPIOperation struct {
	Parameters []openAPIParameter `json:"parameters"`
}

type openAPIParameter struct {
	Name   string        `json:"name"`
	In     string        `json:"in"`
	Schema openAPISchema `json:"schema"`
}

type openAPISchema struct {
	Type       string                   `json:"type"`
	Enum       []string                 `json:"enum"`
	Properties map[string]openAPISchema `json:"properties"`
	Ref        string                   `json:"$ref"`
	Format     string                   `json:"format"`
	// Nullable is OpenAPI 3.0's spelling (this document is 3.0.4). Swashbuckle
	// emits it for a C# `int?` and omits it for an `int`, so its ABSENCE on an
	// integer parameter is meaningful — that is what knownSpecDivergences reads.
	Nullable bool `json:"nullable"`
}

// specPath is the one place the vendored file's location is written down.
func specPath() string {
	return filepath.Join("..", "..", "api", "specs", "prowlarr.json")
}

func loadSpec(t *testing.T) openAPI {
	t.Helper()
	raw, err := os.ReadFile(specPath())
	if err != nil {
		t.Fatalf("reading api/specs/prowlarr.json: %v\nIt is vendored, not fetched — if it is missing, "+
			"restore it from git rather than downloading one; api/specs/SOURCES.md says which bytes belong there.", err)
	}
	var spec openAPI
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing api/specs/prowlarr.json: %v\nA vendored upstream document does not become "+
			"invalid JSON on its own. Something edited it: `git checkout -- api/specs/prowlarr.json`.", err)
	}
	return spec
}

// jsonFieldNames returns the JSON property names a struct type declares,
// including those promoted from embedded structs.
func jsonFieldNames(typ reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
	return out
}

// TestStructsCoverSpecProperties asserts every property the spec declares has a
// field on the corresponding Go struct. A field UsArr does not read is still worth
// declaring: its absence is how a renamed-or-added upstream field goes unnoticed.
func TestStructsCoverSpecProperties(t *testing.T) {
	spec := loadSpec(t)

	cases := []struct {
		schema string
		typ    reflect.Type
		// skip lists properties deliberately not modelled, each with a reason.
		skip map[string]string
	}{
		{schema: "ReleaseResource", typ: reflect.TypeOf(ReleaseResource{})},
		{schema: "IndexerStatusResource", typ: reflect.TypeOf(IndexerStatusResource{})},
		{schema: "IndexerCategory", typ: reflect.TypeOf(IndexerCategory{})},
		{schema: "IndexerCapabilityResource", typ: reflect.TypeOf(IndexerCapabilityResource{})},
		{
			schema: "IndexerResource",
			typ:    reflect.TypeOf(IndexerResource{}),
			skip: map[string]string{
				"message":          "ProviderMessage is UI chrome UsArr does not render",
				"legacyUrls":       "historical indexer URLs, unused",
				"downloadClientId": "modelled",
			},
		},
		{
			schema: "DownloadClientResource",
			typ:    reflect.TypeOf(DownloadClientResource{}),
			skip: map[string]string{
				"message":  "ProviderMessage is UI chrome UsArr does not render",
				"infoLink": "documentation link, unused",
			},
		},
		{
			schema: "SystemResource",
			typ:    reflect.TypeOf(SystemResource{}),
			skip: map[string]string{
				"isAdmin": "process privilege, not UsArr's business", "isUserInteractive": "same",
				"startupPath": "host filesystem paths UsArr must not store", "appData": "same",
				"isNetCore": "runtime trivia", "isLinux": "runtime trivia", "isOsx": "runtime trivia",
				"isWindows": "runtime trivia", "mode": "RuntimeMode, unused",
				"packageAuthor": "packaging trivia", "packageUpdateMechanism": "UsArr never triggers an *Arr update",
				"packageUpdateMechanismMessage": "same", "runtimeName": "runtime trivia",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[tc.schema]
			if !ok {
				t.Fatalf("api/specs/prowlarr.json declares no schema %q, and %s is modelled on it%s",
					tc.schema, tc.typ, specSkewAdvice)
			}
			have := jsonFieldNames(tc.typ)
			var missing []string
			for prop := range schema.Properties {
				if have[prop] {
					continue
				}
				if _, skipped := tc.skip[prop]; skipped {
					continue
				}
				missing = append(missing, prop)
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("%s declares %v, which %s does not model. Either add the fields, or add each "+
					"to this case's skip map WITH A REASON — an unmodelled property decodes to nothing "+
					"and is how a renamed upstream field goes unnoticed.%s",
					tc.schema, missing, tc.typ, specSkewAdvice)
			}
		})
	}
}

// TestEnumsCoverSpecValues pins the enums UsArr branches on. DownloadProtocol is
// the backbone of `source:` tagging and is byte-identical across the Prowlarr,
// Sonarr and Radarr specs — a silently added member would be laundered into
// "unknown" and mis-tag every release from a new protocol.
func TestEnumsCoverSpecValues(t *testing.T) {
	spec := loadSpec(t)

	cases := []struct {
		schema string
		have   []string
	}{
		{
			schema: "DownloadProtocol",
			have:   []string{string(ProtocolUnknown), string(ProtocolUsenet), string(ProtocolTorrent)},
		},
		{
			schema: "IndexerPrivacy",
			have:   []string{string(PrivacyPublic), string(PrivacySemiPrivate), string(PrivacyPrivate)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[tc.schema]
			if !ok {
				t.Fatalf("api/specs/prowlarr.json declares no enum %q, which internal/servarr branches on%s",
					tc.schema, specSkewAdvice)
			}
			want := append([]string(nil), schema.Enum...)
			got := append([]string(nil), tc.have...)
			sort.Strings(want)
			sort.Strings(got)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("%s: the spec has %v, Go has %v. A member Go is missing gets laundered into "+
					"\"unknown\" and mis-tags every release carrying it, so this is never just a list to "+
					"sync — find out what the new member MEANS first.%s",
					tc.schema, want, got, specSkewAdvice)
			}
		})
	}
}

// TestSearchTypeValuesMatchSpecSearchParams cross-checks that every search type
// UsArr sends has a corresponding *SearchParams array in the spec — which is what
// an indexer advertises to say it supports that mode.
func TestSearchTypeValuesMatchSpecSearchParams(t *testing.T) {
	spec := loadSpec(t)
	caps, ok := spec.Components.Schemas["IndexerCapabilityResource"]
	if !ok {
		t.Fatal("api/specs/prowlarr.json declares no IndexerCapabilityResource" + specSkewAdvice)
	}
	for _, prop := range []string{"searchParams", "tvSearchParams", "movieSearchParams", "musicSearchParams", "bookSearchParams"} {
		if _, ok := caps.Properties[prop]; !ok {
			t.Errorf("IndexerCapabilityResource no longer declares %s. The search fan-out reads it to "+
				"decide whether an indexer supports a mode at all; without it UsArr either queries "+
				"indexers that cannot answer or skips ones that can.%s", prop, specSkewAdvice)
		}
	}
}

// TestEndpointsExistInSpec pins the paths and query parameters the client sends.
func TestEndpointsExistInSpec(t *testing.T) {
	spec := loadSpec(t)

	for _, tc := range []struct {
		path, method string
		params       []string
	}{
		{path: "/ping", method: "get"},
		{path: "/api/v1/system/status", method: "get"},
		{path: "/api/v1/indexer", method: "get"},
		{path: "/api/v1/indexerstatus", method: "get"},
		{path: "/api/v1/downloadclient", method: "get"},
		{
			path: "/api/v1/search", method: "get",
			params: []string{"query", "type", "indexerIds", "categories", "limit", "offset"},
		},
		{path: "/api/v1/search", method: "post"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			ops, ok := spec.Paths[tc.path]
			if !ok {
				t.Fatalf("api/specs/prowlarr.json declares no path %s, and the client calls it%s",
					tc.path, specSkewAdvice)
			}
			op, ok := ops[tc.method]
			if !ok {
				t.Fatalf("api/specs/prowlarr.json declares %s but not %s on it, and the client sends that method%s",
					tc.path, strings.ToUpper(tc.method), specSkewAdvice)
			}
			have := map[string]bool{}
			for _, p := range op.Parameters {
				have[p.Name] = true
			}
			for _, want := range tc.params {
				if !have[want] {
					t.Errorf("%s %s no longer declares the %q parameter, and internal/servarr sends it. "+
						"An unbound query parameter is not an error to ASP.NET — it is ignored — so the "+
						"symptom is a search that quietly returns the wrong set, not a 400.%s",
						strings.ToUpper(tc.method), tc.path, want, specSkewAdvice)
				}
			}
		})
	}
}

// specDivergence is one place where the vendored spec is KNOWN to contradict
// Prowlarr's source, with the citation that settles it.
//
// This is ADR-0047's answer to the same problem ADR-0046 solved with
// `ceilingOnlyProperties`, shaped for a different upstream. Kavita's two specs
// let the delta be RECOMPUTED from files; Prowlarr's one stale spec cannot, so
// the delta is written out by hand against upstream source and machine-checked
// in the only direction a single file can be checked: the divergence must still
// be there. An entry that stops being true means upstream finally regenerated,
// which is news — and news is what a comment cannot deliver, because a comment
// that has gone false looks exactly like one that has not.
type specDivergence struct {
	// name is what a failure prints, in upstream's vocabulary.
	name string
	// param is the query parameter on GET /api/v1/search the entry is about.
	param string
	// goField is the field on SearchRequest that follows the SOURCE, not the spec.
	goField string
	// since cites the upstream commit and the first release carrying it.
	since string
	// consequence is what breaks if someone "corrects" UsArr to match the spec.
	// Written per entry rather than shared, because the two are NOT the same:
	// upstream added a `> 0` guard for limit and none for offset, and a message
	// that claimed otherwise would be teaching the reader something false at the
	// moment they are least able to check it.
	consequence string
}

// knownSpecDivergences: the vendored spec is WRONG here, and UsArr is right.
//
// `SearchResource.Limit` and `.Offset` were `int` and became `int?` in
// c687bdb1f ("Fixed: Don't send limit=0 to Newznab indexers", PR #2654,
// 2026-04-11), first released in v2.3.6.5351 — so they are nullable both on the
// owner's v2.5.2.5491 and on develop. The vendored spec, regenerated ten months
// before that commit, still declares them as plain non-nullable int32.
//
// This is not pedantry about a schema annotation. GET /api/v1/search binds
// [FromQuery] SearchResource, and when Limit was a non-nullable int an omitted
// parameter bound to 0, which the Newznab request generator emitted as
// `&limit=0` — and a spec-conformant indexer returns ZERO RESULTS for it
// (upstream confirmed DrunkenSlug and NZBGeek). Anyone who "fixed" UsArr to
// match the vendored spec by making SearchRequest.Limit a plain int32 would
// resurrect that bug against every Prowlarr released since April.
var knownSpecDivergences = []specDivergence{
	{
		name:    "SearchResource.Limit",
		param:   "limit",
		goField: "Limit",
		since:   "c687bdb1f (PR #2654), first released in v2.3.6.5351",
		consequence: "limit=0 is forwarded to the indexer verbatim, and a spec-conformant Newznab " +
			"indexer answers it with ZERO RESULTS — upstream confirmed DrunkenSlug and NZBGeek, and " +
			"that same commit hardened NewznabRequestGenerator to `searchCriteria.Limit is > 0`",
	},
	{
		name:    "SearchResource.Offset",
		param:   "offset",
		goField: "Offset",
		since:   "c687bdb1f (PR #2654), first released in v2.3.6.5351",
		// Deliberately NOT limit's story. Upstream added no `> 0` guard for
		// offset, so offset=0 is harmless on the wire; the reason the pointer
		// still matters is UsArr's own rule two lines above SearchRequest.Offset.
		consequence: "offset=0 is harmless to the indexer — upstream added no `> 0` guard for it — but " +
			"a non-pointer makes it UNCONDITIONAL, and internal/servarr's own rule is to send offset " +
			"only after checking an indexer's supportsPagination. The pointer is what makes " +
			"\"do not paginate this one\" expressible at all",
	},
}

// TestKnownSpecDivergencesStillHold asserts, for each entry, all three of:
//
//	(a) the vendored spec STILL declares the parameter non-nullable — i.e. the
//	    entry is still needed. It failing is the good news case: upstream
//	    regenerated, and the whole document deserves re-reading.
//	(b) SearchRequest's field is a POINTER, so "unset" is representable at all.
//	(c) Values() OMITS the parameter when the field is nil — the wire behaviour
//	    (a) and (b) exist to protect. Omitting and sending zero are different
//	    things to a Newznab indexer, and only (c) can tell them apart.
func TestKnownSpecDivergencesStillHold(t *testing.T) {
	spec := loadSpec(t)
	searchGet, ok := spec.Paths["/api/v1/search"]["get"]
	if !ok {
		t.Fatal("GET /api/v1/search is not in the vendored spec; every entry below would pass vacuously" + specSkewAdvice)
	}
	if len(knownSpecDivergences) == 0 {
		t.Fatal("knownSpecDivergences is empty: this test would pass vacuously. If a regenerated " +
			"spec really did resolve every entry, delete the test rather than leaving an empty guard.")
	}

	params := map[string]openAPIParameter{}
	for _, p := range searchGet.Parameters {
		params[p.Name] = p
	}
	reqType := reflect.TypeOf(SearchRequest{})

	for _, d := range knownSpecDivergences {
		t.Run(d.name, func(t *testing.T) {
			// (a) the spec has not caught up.
			p, ok := params[d.param]
			if !ok {
				t.Fatalf("GET /api/v1/search no longer declares %q at all%s", d.param, specSkewAdvice)
			}
			if p.Schema.Nullable {
				t.Errorf("the vendored spec now declares %s (%s) NULLABLE, so it has caught up with "+
					"upstream source and this knownSpecDivergences entry is stale.\n"+
					"This is the GOOD failure: it means api/specs/prowlarr.json was regenerated for the "+
					"first time since 2025-06-07. Delete this entry — and do not stop there: re-read the "+
					"whole document against internal/servarr, because every other assumption in this "+
					"file was made about the OLD one. Then update ADR-0047, whose premise was that "+
					"Prowlarr does not regenerate per release.",
					d.name, d.param)
			}

			// (b) the Go field can represent "unset".
			f, ok := reqType.FieldByName(d.goField)
			if !ok {
				t.Fatalf("SearchRequest has no field %s; knownSpecDivergences is out of date with the code", d.goField)
			}
			if f.Type.Kind() != reflect.Pointer {
				t.Errorf("SearchRequest.%s is %s, not a pointer.\n"+
					"If you changed it to match api/specs/prowlarr.json, THE SPEC IS THE STALE ONE: "+
					"upstream made %s nullable in %s, so it is nullable on the owner's release and on "+
					"develop, and the vendored document predates that by ten months.\n"+
					"A non-pointer sends %s=0 on every search that does not set it. %s.\n"+
					"Restore the pointer.",
					d.goField, f.Type, d.name, d.since, d.param, d.consequence)
			}

			// (c) the wire behaviour, which is the only half a user can feel.
			v, err := SearchRequest{Text: "ubuntu"}.Values()
			if err != nil {
				t.Fatalf("encoding a search with no %s: %v", d.param, err)
			}
			if v.Has(d.param) {
				// The order here is load-bearing. This prints the OBSERVED value,
				// which need not be zero, so the general rule has to come first;
				// leading with the zero-value story explained a case the reader
				// may not be looking at, right under a line showing a different
				// one.
				t.Errorf("SearchRequest.Values() emits %s=%q on a search that never set %s.\n"+
					"Whatever that value is, EMITTING IT AT ALL is the defect: omitting a parameter and "+
					"sending one the caller never chose are DIFFERENT THINGS here, which is the entire "+
					"point of upstream's %s.\n"+
					"And the zero value is the one that bites — %s.\n"+
					"Only send %s when the caller asked for it.",
					d.param, v.Get(d.param), d.goField, d.since, d.consequence, d.param)
			}
		})
	}
}

// resolve follows a $ref into components/schemas, once. The spec nests no deeper
// for the properties this file checks.
func (s openAPI) resolve(sch openAPISchema) openAPISchema {
	if sch.Ref == "" {
		return sch
	}
	name := strings.TrimPrefix(sch.Ref, "#/components/schemas/")
	if target, ok := s.Components.Schemas[name]; ok {
		return target
	}
	return sch
}

// TestOutboundBodiesAreSpecLegal validates every JSON body UsArr POSTs to an *Arr
// against the schema the spec says that endpoint binds.
//
// THE RULE THIS ENFORCES: for these APIs, omitting a property and sending it empty
// are different things. Prowlarr binds with System.Text.Json, so an absent property
// leaves the C# member at its default, but a PRESENT property must convert to the
// member's type. Go's zero values make "present and empty" the default for every
// field without `omitempty`, which is how a grab body built by zeroing a
// ReleaseResource came to send `"protocol":""` and take a 400 from every real
// Prowlarr on every grab — while the hand-authored fixtures accepted it.
//
// The check is deliberately generic rather than a pin on `protocol`: the next
// outbound body, and the Sonarr and Radarr write paths, fail the same way with a
// different field name. Assert on the MARSHALLED BYTES; a zeroed struct field is
// invisible until encoding/json has had it.
func TestOutboundBodiesAreSpecLegal(t *testing.T) {
	spec := loadSpec(t)

	dcID := int32(4)
	full := ReleaseResource{
		GUID: "https://tracker.example/details/1234", IndexerID: 1, Title: "A.Release",
		Protocol: ProtocolTorrent, PublishDate: time.Now().UTC(), DownloadClientID: &dcID,
	}

	for _, tc := range []struct {
		name, endpoint, schema string
		body                   any
	}{
		{
			name: "grab", endpoint: "POST /api/v1/search", schema: "ReleaseResource",
			body: full.GrabBody(),
		},
		{
			// The minimum a caller can hand us: every optional field at its zero
			// value. This is the case the live bug was in.
			name: "grab/zero-valued", endpoint: "POST /api/v1/search", schema: "ReleaseResource",
			body: ReleaseResource{GUID: "g", IndexerID: 1}.GrabBody(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[tc.schema]
			if !ok {
				t.Fatalf("api/specs/prowlarr.json declares no schema %q, and %s binds its request body to it%s",
					tc.schema, tc.endpoint, specSkewAdvice)
			}
			raw, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshalling the %s body: %v", tc.name, err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("re-decoding the %s body: %v", tc.name, err)
			}

			for prop, v := range got {
				declared, ok := schema.Properties[prop]
				if !ok {
					t.Errorf("%s sends %q, which %s does not declare: the binder will ignore it at best",
						tc.endpoint, prop, tc.schema)
					continue
				}
				resolved := spec.resolve(declared)

				if len(resolved.Enum) > 0 {
					s, isString := v.(string)
					if !isString || !slices.Contains(resolved.Enum, s) {
						t.Errorf("%s sends %s=%#v, which is not one of %v — omit the field instead of "+
							"sending a zero value; the converter rejects the empty string rather than "+
							"treating it as absent (body: %s)",
							tc.endpoint, prop, v, resolved.Enum, raw)
					}
				}

				if resolved.Format == "date-time" {
					s, isString := v.(string)
					if !isString {
						t.Errorf("%s sends %s=%#v for a date-time property", tc.endpoint, prop, v)
						continue
					}
					ts, err := time.Parse(time.RFC3339, s)
					if err != nil {
						t.Errorf("%s sends %s=%q, which is not a bindable date-time: %v", tc.endpoint, prop, s, err)
						continue
					}
					if ts.IsZero() {
						t.Errorf("%s sends %s=%q — Go's zero time, marshalled because the field lacks "+
							"omitempty. Omit it rather than asserting the release was published in year 1.",
							tc.endpoint, prop, s)
					}
				}
			}
		})
	}
}

// TestEnumFieldsCannotMarshalAsEmpty is the static half of the rule
// TestOutboundBodiesAreSpecLegal enforces dynamically.
//
// The body is model-bound to a TYPED resource before the handler runs, so a field
// the handler never reads can still fail the whole request — which is precisely
// how a grab that only needed guid, indexerId and downloadClientId died on
// `"protocol":""`. Every enum-kinded field must therefore be unable to marshal as
// the empty string, whether or not anything sends it today: IndexerResource and
// DownloadClientResource are read-only in v0.1, and the first PUT added to either
// would otherwise ship the same 400 again under a different field name.
func TestEnumFieldsCannotMarshalAsEmpty(t *testing.T) {
	enumKinds := map[reflect.Type]bool{
		reflect.TypeOf(DownloadProtocol("")): true,
		reflect.TypeOf(IndexerPrivacy("")):   true,
	}

	for _, typ := range []reflect.Type{
		reflect.TypeOf(ReleaseResource{}),
		reflect.TypeOf(IndexerResource{}),
		reflect.TypeOf(DownloadClientResource{}),
		reflect.TypeOf(GrabRequest{}),
	} {
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				f := typ.Field(i)
				if !enumKinds[f.Type] {
					continue
				}
				tag := f.Tag.Get("json")
				if tag == "-" {
					continue
				}
				if !slices.Contains(strings.Split(tag, ",")[1:], "omitempty") {
					t.Errorf("%s.%s is an enum with `json:%q`: a zero value marshals as \"\", which "+
						"System.Text.Json rejects rather than treating as absent. Add omitempty, or "+
						"drop the field from the type that builds the request body.",
						typ.Name(), f.Name, tag)
				}
			}
		})
	}
}

// TestNoBulkGrabIsWired documents a deliberate omission. POST /api/v1/search/bulk
// exists, but it silently drops partial failures and ignores per-release
// downloadClientId, so UsArr does not use it. If someone wires it up, this test is
// where they should have to argue with a comment first.
func TestNoBulkGrabIsWired(t *testing.T) {
	spec := loadSpec(t)
	if _, ok := spec.Paths["/api/v1/search/bulk"]; !ok {
		t.Skip("upstream removed the bulk endpoint; nothing to guard")
	}
	src, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatalf("reading client.go: %v", err)
	}
	if strings.Contains(string(src), `"/search/bulk"`) {
		t.Error("bulk grab is wired up; it drops partial failures silently")
	}
}
