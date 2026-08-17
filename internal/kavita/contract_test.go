package kavita

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// Contract tests against the VENDORED OpenAPI document (api/specs/kavita.json,
// provenance in api/specs/SOURCES.md).
//
// Vendored rather than fetched because CI has no network. Kavita differs from the
// *Arrs in one way worth knowing: its spec is a CHECKED-IN file at the repository
// root, not a debug-only endpoint, so the drift job for this one is cheap.
//
// What these assert is narrow and on purpose: that the Go structs cover every
// property the spec declares for the shapes UsArr consumes, that the integer
// enums carry every documented member, that the endpoints UsArr calls exist, and
// that the ONE body UsArr sends can only marshal to something the endpoint binds.
//
// They do NOT assert the spec is correct. It is not, always — and the divergences
// this package is built around were read out of Kavita's controllers, not this
// file: HealthController's [AllowAnonymous], ServerController's class-level admin
// policy, GetAllSeriesV2's unconditional `Statements.Add`, UserParams' int.MaxValue
// PageSize default, and the `Pagination` response header, which is middleware and
// appears nowhere in this document.

type openAPI struct {
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Security   []map[string][]string                  `json:"security"`
	Components struct {
		Schemas         map[string]openAPISchema `json:"schemas"`
		SecuritySchemes map[string]struct {
			Type string `json:"type"`
			Name string `json:"name"`
			In   string `json:"in"`
		} `json:"securitySchemes"`
	} `json:"components"`
}

type openAPIOperation struct {
	Parameters []struct {
		Name   string        `json:"name"`
		In     string        `json:"in"`
		Schema openAPISchema `json:"schema"`
	} `json:"parameters"`
	// Security is the per-operation override. Every operation in this spec leaves
	// it nil, which is what TestAuthIsOneGlobalHeaderScheme proves.
	Security    []map[string][]string `json:"security"`
	RequestBody *struct {
		Content map[string]struct {
			Schema openAPISchema `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
}

type openAPISchema struct {
	Type       string                   `json:"type"`
	Enum       []json.RawMessage        `json:"enum"`
	Properties map[string]openAPISchema `json:"properties"`
	Ref        string                   `json:"$ref"`
	Format     string                   `json:"format"`
}

func loadSpec(t *testing.T) openAPI {
	t.Helper()
	path := filepath.Join("..", "..", "api", "specs", "kavita.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading vendored spec (see api/specs/SOURCES.md): %v", err)
	}
	var spec openAPI
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing vendored spec: %v", err)
	}
	return spec
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

// intEnum renders a schema's enum as int64s. Every enum in this spec is an
// integer one — which is the whole reason the outbound-body rule here is the
// mirror image of the *Arr rule.
func intEnum(t *testing.T, name string, sch openAPISchema) []int64 {
	t.Helper()
	out := make([]int64, 0, len(sch.Enum))
	for _, raw := range sch.Enum {
		var v int64
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: enum member %s is not an integer — this spec's enums are integers and "+
				"the outbound-body guards are written on that assumption", name, raw)
		}
		out = append(out, v)
	}
	return out
}

// jsonFieldNames returns the JSON property names a struct type declares.
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
// field on the corresponding Go struct. A field UsArr does not read is still
// worth declaring: its absence is how a renamed-or-added upstream field goes
// unnoticed. SeriesDto gaining mangaBakaId on the develop line is the live
// example.
func TestStructsCoverSpecProperties(t *testing.T) {
	spec := loadSpec(t)

	cases := []struct {
		schema string
		typ    reflect.Type
		skip   map[string]string
	}{
		{schema: "SeriesDto", typ: reflect.TypeOf(SeriesDto{})},
		{schema: "ServerInfoSlimDto", typ: reflect.TypeOf(ServerInfoSlimDto{})},
		{schema: "LibraryDto", typ: reflect.TypeOf(LibraryDto{})},
		{schema: "SeriesFilterV2Dto", typ: reflect.TypeOf(SeriesFilter{})},
		{schema: "SeriesFilterStatementDto", typ: reflect.TypeOf(SeriesFilterStatement{})},
		{schema: "SeriesSortOptionDto", typ: reflect.TypeOf(SeriesSortOption{})},
		// The credit path (ADR-0044). SeriesMetadataDto is the only endpoint
		// that reports a creator, and its THIRTEEN person arrays are all
		// declared even though UsArr maps eight of them: an array that is in the
		// spec and absent from the struct is an upstream change nobody sees.
		{schema: "SeriesMetadataDto", typ: reflect.TypeOf(SeriesMetadataDto{})},
		{schema: "PersonDto", typ: reflect.TypeOf(PersonDto{})},
		{schema: "GenreTagDto", typ: reflect.TypeOf(GenreTagDto{})},
		{schema: "TagDto", typ: reflect.TypeOf(TagDto{})},
	}

	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[tc.schema]
			if !ok {
				t.Fatalf("%s is not in the vendored spec", tc.schema)
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
				t.Errorf("%s: spec declares properties the Go struct does not model: %v", tc.schema, missing)
			}
		})
	}
}

// TestEnumsCoverSpecValues pins every integer enum UsArr branches on.
//
// A silently added member matters more here than for a string enum: a Go constant
// block that stops one short does not fail to compile, it just mislabels whatever
// upstream added.
func TestEnumsCoverSpecValues(t *testing.T) {
	spec := loadSpec(t)

	cases := []struct {
		schema string
		have   []int64
	}{
		{"QueryContext", []int64{
			int64(QueryContextNone), int64(QueryContextSearch), int64(QueryContextDashboard),
		}},
		{"FilterCombination", []int64{
			int64(FilterCombinationOr), int64(FilterCombinationAnd),
		}},
		{"FilterEntityType", []int64{
			int64(FilterEntityTypeSeries), int64(FilterEntityTypeReadingList),
			int64(FilterEntityTypePerson), int64(FilterEntityTypeAnnotation),
		}},
		{"SeriesSortField", []int64{
			int64(SeriesSortFieldSortName), int64(SeriesSortFieldCreatedDate),
			int64(SeriesSortFieldLastModifiedDate), int64(SeriesSortFieldLastChapterAdded),
			int64(SeriesSortFieldTimeToRead), int64(SeriesSortFieldReleaseYear),
			int64(SeriesSortFieldReadProgress), int64(SeriesSortFieldAverageRating),
			int64(SeriesSortFieldRandom), int64(SeriesSortFieldUserRating),
			int64(SeriesSortFieldUnreadChapterCount),
		}},
		{"MangaFormat", []int64{
			int64(MangaFormatImage), int64(MangaFormatArchive), int64(MangaFormatUnknown),
			int64(MangaFormatEpub), int64(MangaFormatPdf),
		}},
		// PersonRole starts at 3 — there is no 0, 1 or 2 — which is exactly the
		// shape this test exists for: a Go constant block written from memory
		// starts at 0 and mislabels every role by three.
		{"PersonRole", []int64{
			int64(PersonRoleWriter), int64(PersonRolePenciller), int64(PersonRoleInker),
			int64(PersonRoleColorist), int64(PersonRoleLetterer), int64(PersonRoleCoverArtist),
			int64(PersonRoleEditor), int64(PersonRolePublisher), int64(PersonRoleCharacter),
			int64(PersonRoleTranslator), int64(PersonRoleImprint), int64(PersonRoleTeam),
			int64(PersonRoleLocation),
		}},
		{"LibraryType", []int64{
			int64(LibraryTypeManga), int64(LibraryTypeComic), int64(LibraryTypeBook),
			int64(LibraryTypeImage), int64(LibraryTypeLightNovel), int64(LibraryTypeComicVine),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[tc.schema]
			if !ok {
				t.Fatalf("%s is not in the vendored spec", tc.schema)
			}
			want := intEnum(t, tc.schema, schema)
			got := slices.Clone(tc.have)
			slices.Sort(want)
			slices.Sort(got)
			if !slices.Equal(want, got) {
				t.Errorf("%s: spec has %v, Go has %v", tc.schema, want, got)
			}
		})
	}
}

// TestAuthIsOneGlobalHeaderScheme pins the fact the whole client is built on:
// ONE apiKey scheme, in a HEADER, applied globally, with no operation ever
// overriding it.
//
// If upstream ever adds a per-operation security requirement — an OAuth scope, a
// second scheme — this client's "same header on every endpoint" assumption stops
// being true and something has to change. This is where that shows up.
func TestAuthIsOneGlobalHeaderScheme(t *testing.T) {
	spec := loadSpec(t)

	if n := len(spec.Components.SecuritySchemes); n != 1 {
		t.Fatalf("expected exactly one securityScheme, got %d: %v", n, spec.Components.SecuritySchemes)
	}
	sch, ok := spec.Components.SecuritySchemes["AuthKey"]
	if !ok {
		t.Fatalf("the AuthKey securityScheme is gone: %v", spec.Components.SecuritySchemes)
	}
	if sch.Type != "apiKey" || sch.In != "header" || sch.Name != authHeader {
		t.Errorf("AuthKey is now {type:%q in:%q name:%q}; this client sends %q as a header and nothing else",
			sch.Type, sch.In, sch.Name, authHeader)
	}
	if len(spec.Security) != 1 {
		t.Errorf("expected one global security requirement, got %v", spec.Security)
	}

	var overrides []string
	for path, ops := range spec.Paths {
		for method, op := range ops {
			if len(op.Security) > 0 {
				overrides = append(overrides, strings.ToUpper(method)+" "+path)
			}
		}
	}
	sort.Strings(overrides)
	if len(overrides) > 0 {
		t.Errorf("operations now override the global security requirement, so one header is no longer "+
			"the whole auth story: %v", overrides)
	}
}

// TestAPIKeyQueryIsNotGeneralAuth pins the OTHER half of the auth fact: `?apiKey=`
// is a handful of special cases, not an alternative to the header, and this
// client must not generalise it.
//
// The count is asserted loosely — that it is a small set, not the whole API —
// because pinning it at exactly 9 would fail on an unrelated upstream addition.
// What is pinned exactly is the client's own behaviour: exactly ONE method sends
// it, and it is the documented non-admin version fallback.
func TestAPIKeyQueryIsNotGeneralAuth(t *testing.T) {
	spec := loadSpec(t)

	var withQueryKey []string
	for path, ops := range spec.Paths {
		for method, op := range ops {
			for _, p := range op.Parameters {
				if strings.EqualFold(p.Name, "apiKey") && p.In == "query" {
					withQueryKey = append(withQueryKey, strings.ToUpper(method)+" "+path)
				}
			}
		}
	}
	total := 0
	for _, ops := range spec.Paths {
		total += len(ops)
	}
	if len(withQueryKey) == 0 {
		t.Fatal("no operation takes ?apiKey= any more; PluginVersion is built on one that does")
	}
	if len(withQueryKey) > total/10 {
		t.Errorf("?apiKey= now appears on %d of %d operations — it has stopped being a special case, "+
			"and the client's header-only rule needs revisiting: %v", len(withQueryKey), total, withQueryKey)
	}
	if !slices.Contains(withQueryKey, "GET /api/Plugin/version") {
		t.Errorf("GET /api/Plugin/version no longer takes ?apiKey=; PluginVersion sends it: %v", withQueryKey)
	}

	src, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatalf("reading client.go: %v", err)
	}
	if n := strings.Count(string(src), `"apiKey"`); n != 1 {
		t.Errorf(`client.go mentions "apiKey" %d times; exactly one method may put the Auth Key in a `+
			`query string, and it is PluginVersion`, n)
	}
}

// TestEndpointsExistInSpec pins the paths and query parameters the client sends.
func TestEndpointsExistInSpec(t *testing.T) {
	spec := loadSpec(t)

	for _, tc := range []struct {
		path, method string
		params       []string
	}{
		{path: "/api/Health", method: "get"},
		{path: "/api/Server/server-info-slim", method: "get"},
		{path: "/api/Plugin/version", method: "get", params: []string{"apiKey"}},
		{path: "/api/Library/libraries", method: "get"},
		{
			path: "/api/Series/all-v2", method: "post",
			// The casing is load-bearing: ASP.NET's [FromQuery] binder on
			// UserParams matches these exactly.
			params: []string{"PageNumber", "PageSize", "context"},
		},
		// The credit path. The parameter name is camelCase here and PascalCase
		// on all-v2 above, which is not an inconsistency this file may tidy:
		// ASP.NET's binder matches what the controller declares, and the two
		// controllers declare different things.
		{path: "/api/Series/metadata", method: "get", params: []string{"seriesId"}},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			ops, ok := spec.Paths[tc.path]
			if !ok {
				t.Fatalf("%s is not in the vendored spec", tc.path)
			}
			op, ok := ops[tc.method]
			if !ok {
				t.Fatalf("%s %s is not in the vendored spec", tc.method, tc.path)
			}
			have := map[string]bool{}
			for _, p := range op.Parameters {
				have[p.Name] = true
			}
			for _, want := range tc.params {
				if !have[want] {
					t.Errorf("%s %s no longer accepts %q", tc.method, tc.path, want)
				}
			}
		})
	}
}

// TestThereIsNoServerInfoEndpoint pins a name that is easy to get wrong from
// memory, and that this project got wrong in an early draft: the endpoint is
// server-info-SLIM. A plain /api/Server/server-info does not exist.
func TestThereIsNoServerInfoEndpoint(t *testing.T) {
	spec := loadSpec(t)
	if _, ok := spec.Paths["/api/Server/server-info"]; ok {
		t.Error("/api/Server/server-info now exists; ServerInfo() deliberately calls server-info-slim, " +
			"and if upstream added a fatter sibling somebody should decide which one UsArr wants")
	}
	if _, ok := spec.Paths["/api/Server/server-info-slim"]; !ok {
		t.Fatal("/api/Server/server-info-slim is gone; ServerInfo() has nothing to call")
	}
}

// TestNoTimestampFilterFieldExists is the guard behind channel 3b's whole shape.
//
// ARCHITECTURE.md §7.1a: the delta is a sorted page walk with a CLIENT-SIDE stop,
// not a re-request at the watermark, because SeriesFilterField carries no
// timestamp member and so no server-side "since" is expressible. If upstream ever
// adds one, that design choice is worth revisiting — and this is the test that
// says so, rather than a comment nobody re-reads.
func TestNoTimestampFilterFieldExists(t *testing.T) {
	spec := loadSpec(t)
	sch, ok := spec.Components.Schemas["SeriesFilterField"]
	if !ok {
		t.Fatal("SeriesFilterField is not in the vendored spec")
	}
	members := intEnum(t, "SeriesFilterField", sch)
	if len(members) != 35 {
		t.Logf("SeriesFilterField now has %d members (was 35 at the pinned commit)", len(members))
	}

	// The member NAMES live in the x-enum-varnames extension, which the narrow
	// struct above does not model, so re-read the raw description — Swashbuckle
	// writes every member name into it.
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "specs", "kavita.json"))
	if err != nil {
		t.Fatalf("reading vendored spec: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				EnumVarNames []string `json:"x-enum-varnames"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing vendored spec: %v", err)
	}
	names := doc.Components.Schemas["SeriesFilterField"].EnumVarNames
	if len(names) == 0 {
		t.Fatal("SeriesFilterField no longer carries x-enum-varnames; the member names cannot be checked")
	}
	// ReadingDate is a READ-PROGRESS date, not a content-change timestamp, and is
	// the one name that would otherwise trip this. It is named rather than
	// pattern-excluded so the exception is a decision on the record.
	for _, n := range names {
		if n == "ReadingDate" {
			continue
		}
		l := strings.ToLower(n)
		if strings.Contains(l, "modified") || strings.Contains(l, "updated") ||
			strings.Contains(l, "changed") || strings.Contains(l, "since") {
			t.Errorf("SeriesFilterField now has %q — a server-side since-filter may be expressible, "+
				"and ARCHITECTURE.md §7.1a's client-side stop was chosen because it was not", n)
		}
	}
}

// TestRecentlyUpdatedSeriesIsNotWired documents a deliberate omission, and it is
// the sharpest trap in this API.
//
// 🚩 POST /api/Series/recently-updated-series reads like a delta feed and is NOT
// one: it is a HARD 12-DAY WINDOW, so a series that changed 13 days ago is absent
// and a sync built on it silently loses everything older than the window. Channel
// 3b uses the ordered walk instead. If someone wires this up, this test is where
// they should have to argue with a comment first.
func TestRecentlyUpdatedSeriesIsNotWired(t *testing.T) {
	spec := loadSpec(t)
	if _, ok := spec.Paths["/api/Series/recently-updated-series"]; !ok {
		t.Skip("upstream removed the endpoint; nothing to guard")
	}
	for _, f := range []string{"client.go", "stream.go", "resources.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		if strings.Contains(string(src), "recently-updated-series") {
			t.Errorf("%s references recently-updated-series; it is a 12-day window, not a delta feed", f)
		}
	}
}

// TestOutboundBodiesAreSpecLegal validates the ONE JSON body UsArr POSTs to
// Kavita against the schema the spec says that endpoint binds.
//
// THE RULE, from docs/reference/arr-apis.md §7.1: the body is model-bound to the
// fully typed resource BEFORE the handler runs, so every property sent is
// type-checked whether or not the handler reads it. Go marshals every field
// without omitempty, so "present and wrong" is the default unless deliberately
// prevented.
//
// Kavita inverts the *Arr polarity — its enums are integers, so the hazard is a
// zero that is not a member rather than an empty string — and adds one of its
// own: `"statements":null` nil-derefs GetAllSeriesV2 into a 500, and a nil Go
// slice marshals as exactly that. Assert on the MARSHALLED BYTES; a zero-valued
// struct field is invisible until encoding/json has had it.
func TestOutboundBodiesAreSpecLegal(t *testing.T) {
	spec := loadSpec(t)

	built, err := NewSeriesFilter(SeriesSortFieldLastChapterAdded, false)
	if err != nil {
		t.Fatalf("building the filter: %v", err)
	}

	for _, tc := range []struct {
		name, endpoint, schema string
		body                   any
	}{
		{
			name: "series-list", endpoint: "POST /api/Series/all-v2", schema: "SeriesFilterV2Dto",
			body: built,
		},
		{
			// The minimum a caller can express: a SeriesFilter literal with every
			// field at its zero value, statements nil. Nothing in this package
			// builds one, and the encoder still has to make it safe — because the
			// type is exported and the next caller will.
			name: "series-list/zero-valued", endpoint: "POST /api/Series/all-v2", schema: "SeriesFilterV2Dto",
			body: SeriesFilter{SortOptions: SeriesSortOption{SortField: SeriesSortFieldSortName}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[tc.schema]
			if !ok {
				t.Fatalf("%s is not in the vendored spec", tc.schema)
			}
			raw, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshalling the %s body: %v", tc.name, err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("re-decoding the %s body: %v", tc.name, err)
			}

			// (1) statements must be present and must be an ARRAY, never null.
			stmts, present := got["statements"]
			if !present {
				t.Errorf("%s omits `statements` (body: %s)", tc.endpoint, raw)
			} else if _, isArray := stmts.([]any); !isArray {
				t.Errorf("%s sends statements=%#v. GetAllSeriesV2's first act is "+
					"`seriesFilterDto.Statements.Add(stmt)`, so null nil-derefs into a 500. "+
					"It must be a JSON array (body: %s)", tc.endpoint, stmts, raw)
			}

			// (2) every property must be declared, and every enum-valued one must
			// carry a member.
			assertSpecLegal(t, spec, schema, tc.endpoint, tc.schema, got, raw)
		})
	}
}

// assertSpecLegal walks a marshalled body against a schema, one level deep into
// object-valued properties (which is as deep as this body goes).
func assertSpecLegal(t *testing.T, spec openAPI, schema openAPISchema, endpoint, schemaName string, got map[string]any, raw []byte) {
	t.Helper()
	for prop, v := range got {
		declared, ok := schema.Properties[prop]
		if !ok {
			t.Errorf("%s sends %q, which %s does not declare: the binder will ignore it at best",
				endpoint, prop, schemaName)
			continue
		}
		resolved := spec.resolve(declared)

		if len(resolved.Enum) > 0 {
			members := intEnum(t, prop, resolved)
			n, isNumber := v.(float64)
			if !isNumber || !slices.Contains(members, int64(n)) {
				t.Errorf("%s sends %s=%#v, which is not one of %v. Kavita's enums are INTEGERS, so a Go "+
					"zero value is a plausible-looking non-member rather than an obvious empty string — "+
					"and [EnumDataType] rejects it (body: %s)", endpoint, prop, v, members, raw)
			}
			continue
		}

		// One level down: sortOptions is an object with its own enum inside.
		if child, isObject := v.(map[string]any); isObject && len(resolved.Properties) > 0 {
			assertSpecLegal(t, spec, resolved, endpoint+" ."+prop, prop, child, raw)
		}
	}
}

// TestEnumFieldsCannotMarshalAsEmpty is the static half of the rule
// TestOutboundBodiesAreSpecLegal enforces dynamically, ported to Kavita's
// polarity — and the port is the point, so read the difference rather than the
// name.
//
// internal/servarr's version asserts every enum-kinded field carries `omitempty`,
// because *Arr enums are STRINGS and Go's zero marshals as "" which
// System.Text.Json rejects. Kavita's enums are INTEGERS, and Go's zero marshals
// as 0 — which for FilterCombination and FilterEntityType IS the member UsArr
// wants. So `omitempty` here would be the BUG: it would drop a meaningful zero.
//
// The invariant that survives the port is the one that mattered all along: NO
// FIELD OF AN OUTBOUND BODY MAY MARSHAL TO A NON-MEMBER. This test enumerates
// every enum-kinded field of every type that can reach the wire and checks its
// zero value against the spec, statically, whether or not anything sends it
// today.
func TestEnumFieldsCannotMarshalAsEmpty(t *testing.T) {
	spec := loadSpec(t)

	// The Go type of each enum, and the spec schema that defines its members.
	enumSchemas := map[reflect.Type]string{
		reflect.TypeOf(QueryContext(0)):      "QueryContext",
		reflect.TypeOf(FilterCombination(0)): "FilterCombination",
		reflect.TypeOf(FilterEntityType(0)):  "FilterEntityType",
		reflect.TypeOf(SeriesSortField(0)):   "SeriesSortField",
		reflect.TypeOf(MangaFormat(0)):       "MangaFormat",
		reflect.TypeOf(LibraryType(0)):       "LibraryType",
	}

	// Every type that can be marshalled into a request. SeriesFilter is the only
	// one sent today; the others are here because the next write path added is
	// where the *Arr version of this bug actually shipped.
	outbound := []reflect.Type{
		reflect.TypeOf(SeriesFilter{}),
		reflect.TypeOf(SeriesSortOption{}),
		reflect.TypeOf(SeriesFilterStatement{}),
	}

	// zeroIsMember records, per enum, whether 0 is a legal value to send. A field
	// whose enum has no zero member must be impossible to leave at zero — which
	// for this package means it can only be built through a constructor that
	// refuses one.
	for _, typ := range outbound {
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				f := typ.Field(i)
				schemaName, isEnum := enumSchemas[f.Type]
				if !isEnum {
					continue
				}
				tag := f.Tag.Get("json")
				if tag == "-" {
					continue
				}
				if slices.Contains(strings.Split(tag, ",")[1:], "omitempty") {
					t.Errorf("%s.%s is an integer enum with `json:%q`. omitempty is the *Arr fix and it is "+
						"WRONG here: 0 is a real member of some of these enums, so omitempty silently "+
						"drops a value the caller chose.", typ.Name(), f.Name, tag)
				}

				sch, ok := spec.Components.Schemas[schemaName]
				if !ok {
					t.Fatalf("%s is not in the vendored spec", schemaName)
				}
				members := intEnum(t, schemaName, sch)
				if slices.Contains(members, 0) {
					continue // a zero value is legal; nothing to guard
				}
				// 0 is NOT a member. The zero value of this field is unsendable, so
				// there must be a constructor that cannot produce one.
				if err := zeroValueIsRefused(typ, f.Name); err != nil {
					t.Errorf("%s.%s is a %s, whose members are %v — 0 is not one, so a zero value is a 400 "+
						"on every call. %v", typ.Name(), f.Name, schemaName, members, err)
				}
			}
		})
	}
}

// zeroValueIsRefused checks that the constructor guarding a zero-less enum field
// actually refuses a zero. It is a real call, not a source grep: a comment
// promising a guard and a guard are different things.
func zeroValueIsRefused(typ reflect.Type, field string) error {
	switch {
	case typ == reflect.TypeOf(SeriesSortOption{}) && field == "SortField":
		if _, err := NewSeriesFilter(0, false); err == nil {
			return fmt.Errorf("NewSeriesFilter(0, …) returned no error; it must refuse a non-member")
		}
		if _, err := (SeriesListOptions{Sort: SeriesSortField(99)}).resolve(); err == nil {
			return fmt.Errorf("SeriesListOptions.resolve() accepted Sort=99; it must refuse a non-member")
		}
		return nil
	}
	return fmt.Errorf("no constructor guard is wired for %s.%s — add one, or this field can reach the wire as 0",
		typ.Name(), field)
}

// TestQueryContextZeroIsNotSendable is the query-string half of the same rule.
// `context` is a QueryContext and QueryContext has no zero member, so a zeroed
// SeriesListOptions must resolve to a real one rather than sending 0.
func TestQueryContextZeroIsNotSendable(t *testing.T) {
	spec := loadSpec(t)
	sch := spec.Components.Schemas["QueryContext"]
	members := intEnum(t, "QueryContext", sch)
	if slices.Contains(members, 0) {
		t.Skip("QueryContext gained a zero member; this guard is moot")
	}

	resolved, err := SeriesListOptions{}.resolve()
	if err != nil {
		t.Fatalf("a zeroed SeriesListOptions must resolve: %v", err)
	}
	if got := resolved.values().Get("context"); got != "1" {
		t.Errorf("context=%q; a zeroed SeriesListOptions must send QueryContext.None (1), never 0", got)
	}
	if _, err := (SeriesListOptions{Context: QueryContext(3)}).resolve(); err == nil {
		t.Error("resolve() accepted Context=3, which is not a member of {1,2,4}")
	}
}

// TestMaxStreamBytesTracksTheSSRFCeiling pins the duplicated constant to the
// transport ceiling it is copied from. This package may not import internal/ssrf
// for its policy (doc.go), so the value is duplicated — and this is what stops
// the duplicate drifting into a client cap below the transport's, which would
// truncate a body the layer beneath it permits.
func TestMaxStreamBytesTracksTheSSRFCeiling(t *testing.T) {
	if MaxStreamBytes != ssrf.MaxListBytes {
		t.Errorf("MaxStreamBytes = %d, ssrf.MaxListBytes = %d: the client bound and the transport bound "+
			"must be the same number", MaxStreamBytes, ssrf.MaxListBytes)
	}
}
