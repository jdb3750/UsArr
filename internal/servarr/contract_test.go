package servarr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// Contract tests against the VENDORED OpenAPI document.
//
// They are vendored rather than fetched because *Arr apps guard
// app.UseSwagger() behind `if (BuildInfo.IsDebug)`, so a production instance does
// not serve /docs/v1/openapi.json — and because CI has no network. Drift
// detection is a scheduled job that re-downloads and diffs; it is not this test.
//
// What these assert is narrow and on purpose: that the Go structs cover every
// property the spec declares for the resources UsArr consumes, that the enums
// accept every documented value, and that the endpoints UsArr calls exist with the
// parameters it sends. They do NOT assert the spec is correct — it is not, always.
// IndexerResource.status is declared and is always null in practice, and
// docs/DEVELOPMENT.md §5 lists other places where the controller wins.

type openAPI struct {
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Components struct {
		Schemas map[string]openAPISchema `json:"schemas"`
	} `json:"components"`
}

type openAPIOperation struct {
	Parameters []struct {
		Name string `json:"name"`
		In   string `json:"in"`
	} `json:"parameters"`
}

type openAPISchema struct {
	Type       string                   `json:"type"`
	Enum       []string                 `json:"enum"`
	Properties map[string]openAPISchema `json:"properties"`
	Ref        string                   `json:"$ref"`
	Format     string                   `json:"format"`
}

func loadSpec(t *testing.T) openAPI {
	t.Helper()
	path := filepath.Join("..", "..", "api", "specs", "prowlarr.json")
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
				t.Fatalf("%s is not in the vendored spec", tc.schema)
			}
			want := append([]string(nil), schema.Enum...)
			got := append([]string(nil), tc.have...)
			sort.Strings(want)
			sort.Strings(got)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("%s: spec has %v, Go has %v", tc.schema, want, got)
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
		t.Fatal("IndexerCapabilityResource missing from the spec")
	}
	for _, prop := range []string{"searchParams", "tvSearchParams", "movieSearchParams", "musicSearchParams", "bookSearchParams"} {
		if _, ok := caps.Properties[prop]; !ok {
			t.Errorf("spec no longer declares %s; the fan-out's capability check is built on it", prop)
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
