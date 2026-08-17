package httpapi

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// serviceKinds (services.go) is authoritative for which kinds the handler
// accepts; SERVICE_KINDS (web/src/lib/api.ts) is the add-form's kind picker.
// Until this test they were held in step by a comment on each side, and a
// comment does not fail a build: a kind offered in the browser but refused by
// the handler is a picker that 400s, and a kind the handler accepts but the
// picker never offers is a service nobody can add.
//
// The guard lives on the GO side on purpose. Go owns the answer, so the
// cross-language read belongs with the truth rather than with the follower —
// a vitest copy would be the picker checking itself against a list it cannot
// see. test-go runs inside `make check` exactly as vitest does.
const serviceKindsTSPath = "../../web/src/lib/api.ts"

// The declaration this test reads, matched whole rather than by scanning for
// quoted words anywhere in the file. Narrow on purpose: if api.ts renames,
// reformats across lines in a way this does not admit, or grows an element
// that is not a plain string literal, the parse FAILS. It must never degrade
// to "found nothing", because an empty list agrees with every possible Go map
// and would turn this guard into a permanent pass.
var serviceKindsDecl = regexp.MustCompile(`export const SERVICE_KINDS = \[([^\]]*)\] as const;`)

// One element of that literal: a single- or double-quoted lowercase kind.
// Both quote styles are admitted so a prettier setting change cannot redden
// this test for a reason that has nothing to do with drift.
var serviceKindsElem = regexp.MustCompile(`^'([a-z0-9_-]+)'$|^"([a-z0-9_-]+)"$`)

// parseServiceKindsTS returns SERVICE_KINDS in DECLARED ORDER — order is
// load-bearing, see TestServiceKindsPickerDefault — or an error naming what it
// could not read. It never returns a short list quietly.
func parseServiceKindsTS(path string) ([]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	m := serviceKindsDecl.FindSubmatch(src)
	if m == nil {
		return nil, fmt.Errorf(
			"%s: no `export const SERVICE_KINDS = [...] as const;` declaration found. "+
				"If it was renamed, moved or reformatted, update serviceKindsDecl in "+
				"internal/httpapi/service_kinds_web_test.go — do not delete this test, it is "+
				"the only thing keeping the browser's kind picker in step with serviceKinds",
			path)
	}
	body := strings.TrimSpace(string(m[1]))
	body = strings.TrimSuffix(body, ",") // a trailing comma is prettier's, not an element
	if body == "" {
		return nil, fmt.Errorf("%s: SERVICE_KINDS is declared empty; the add form would offer no kinds", path)
	}
	var kinds []string
	for _, raw := range strings.Split(body, ",") {
		elem := strings.TrimSpace(raw)
		e := serviceKindsElem.FindStringSubmatch(elem)
		if e == nil {
			return nil, fmt.Errorf(
				"%s: SERVICE_KINDS element %q is not a plain quoted kind. This test can only "+
					"compare literals; a spread, identifier or template string would be read as "+
					"no kind at all", path, elem)
		}
		kinds = append(kinds, e[1]+e[2]) // exactly one submatch is non-empty
	}
	return kinds, nil
}

// TestServiceKindsMatchWebPicker asserts BOTH directions. Either one alone
// finds half the bugs: Go-minus-TS is a kind nobody can add, TS-minus-Go is a
// picker entry that 400s.
func TestServiceKindsMatchWebPicker(t *testing.T) {
	web, err := parseServiceKindsTS(serviceKindsTSPath)
	if err != nil {
		t.Fatalf("cannot read the browser's kind list: %v", err)
	}
	inWeb := make(map[string]bool, len(web))
	for _, k := range web {
		inWeb[k] = true
	}

	for kind := range serviceKinds {
		if !inWeb[kind] {
			t.Errorf(
				"kind %q is in serviceKinds (internal/httpapi/services.go) but MISSING from "+
					"SERVICE_KINDS (%s): the handler accepts it and the add form never offers it. "+
					"Add %q to SERVICE_KINDS.",
				kind, serviceKindsTSPath, kind)
		}
	}
	for _, kind := range web {
		if _, ok := serviceKinds[kind]; !ok {
			t.Errorf(
				"kind %q is in SERVICE_KINDS (%s) but MISSING from serviceKinds "+
					"(internal/httpapi/services.go): the add form offers it and the handler would "+
					"refuse it with 400. Add %q to serviceKinds with its role, or drop it from "+
					"SERVICE_KINDS.",
				kind, serviceKindsTSPath, kind)
		}
	}
}

// TestServiceKindsPickerDefault pins index 0 SEPARATELY, because the test above
// compares membership and a reorder passes it clean. SERVICE_KINDS[0] is the
// add form's default kind in two places — web/src/routes/services/+page.svelte:177
// (`let fKind = $state<string>(SERVICE_KINDS[0]);`) and :405 (`fKind =
// SERVICE_KINDS[0];`, the reset when the form is opened as 'add') — so moving
// another kind to the front silently changes what every install adds first.
// Prowlarr is that default: it is the one service every install needs.
func TestServiceKindsPickerDefault(t *testing.T) {
	web, err := parseServiceKindsTS(serviceKindsTSPath)
	if err != nil {
		t.Fatalf("cannot read the browser's kind list: %v", err)
	}
	if web[0] != "prowlarr" {
		t.Errorf(
			"SERVICE_KINDS[0] is %q, want \"prowlarr\" (%s). Index 0 is the add form's default "+
				"kind at web/src/routes/services/+page.svelte:177 and :405, so this reorder "+
				"changes what the form pre-selects. Full order: %v",
			web[0], serviceKindsTSPath, web)
	}
}
