package bookorbit

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestEveryBookOrbitPermissionIsClassified is the enum-completeness guard, on
// internal/libsync's TestEveryKavitaPersonRoleIsAccountedFor pattern.
//
// It is the only mechanism IN THE GATE that notices BookOrbit growing a
// permission. TestSpecDriftBookOrbitTypesStillMatchUpstream notices too — it
// compares vendored src/permissions.ts against upstream — but it sits behind
// `//go:build upstream` in `make spec-drift`, so it runs only when a person
// types that target with network access and USARR_SPEC_DRIFT=1 set. A gate arm
// runs on every commit whether or not anyone is thinking about it; an opt-in
// network check runs when somebody remembers.
//
// BookOrbit commits no OpenAPI document — server/src/swagger.ts builds one at
// RUNTIME and main.ts mounts it only when SWAGGER_ENABLED is true, which
// defaults to false. ADR-0046's floor/ceiling split and ADR-0047's
// blob-identity pin both assume a committed document; the second shape
// transfers to source files, the first does not. That is vendoredtypes_test.go's
// wording, and this comment now agrees with it rather than contradicting it.
//
// CORRECTED 2026-08-19, and the correction is recorded rather than swapped in
// silently. This header used to say NEITHER shape transferred and that this
// test was the substitute for both. True when it was written at c324cbf;
// falsified by 72207f8, which vendored packages/types under
// api/specs/bookorbit-types/ pinned by git's own tree name, with a
// comment-blind declaration digest over the five transcribed files, offline
// guards in `make check` and the upstream comparison in `make spec-drift` —
// ADR-0047's shape, one level up the git object graph. src/permissions.ts, the
// file this test's vocabulary is transcribed from, is one of the five. The
// ADR-0046 half of the old sentence was right and stands: two pins are two
// points only where upstream regenerates a committed document per release, and
// BookOrbit has neither a release-tag line nor a checked-in document to split.
// See api/specs/SOURCES.md and REVIEW-LOG's LS-378.
//
// A 24th permission upstream reaches this package as an unrecognised string,
// classifyScope grades it ELEVATED, and this test is where someone finds out.
func TestEveryBookOrbitPermissionIsClassified(t *testing.T) {
	for _, p := range allPermissions {
		_, elevated := elevatedPermissions[p]
		_, unneeded := unneededPermissions[p]
		switch {
		case elevated && unneeded:
			t.Errorf("%s is in BOTH classifications", p)
		case !elevated && !unneeded:
			t.Errorf("%s is in NEITHER classification; every member of the vocabulary must be graded", p)
		}
	}
	if got := len(elevatedPermissions) + len(unneededPermissions); got != len(allPermissions) {
		t.Errorf("%d classified entries for %d permissions; a name in a map that is not in allPermissions "+
			"is a typo that silently classifies nothing", got, len(allPermissions))
	}
}

// TestPermissionVocabularyMatchesTheSource transcribes
// packages/types/src/permissions.ts@73b7877d and asserts this package carries
// exactly it — no more, no fewer, same strings.
//
// The literal list is duplicated here deliberately: a test that iterated
// allPermissions would agree with itself no matter what was dropped from it.
func TestPermissionVocabularyMatchesTheSource(t *testing.T) {
	want := []string{
		"library_download", "library_upload", "library_edit_metadata", "library_delete_books",
		"book_dock_access", "demo_restricted",
		"kobo_sync", "koreader_sync", "hardcover_sync", "readwise_sync", "storygraph_sync", "opds_access",
		"email_send", "manage_email",
		"manage_libraries", "manage_metadata_config", "manage_icons", "manage_app_settings",
		"manage_book_dock", "manage_users", "view_user_activity", "view_audit_log",
		"notification_access",
	}
	if len(want) != 23 {
		t.Fatalf("the transcription itself has %d entries; the enum has 23", len(want))
	}
	got := make([]string, 0, len(allPermissions))
	for _, p := range allPermissions {
		got = append(got, string(p))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("vocabulary drift:\n got %v\nwant %v", got, want)
	}
}

// TestTheCorrectCredentialIsMinimal is ADR-0052's gate answered in the
// affirmative: a shared account with an empty permission set produces NO
// findings, which is what makes a finding mean something.
func TestTheCorrectCredentialIsMinimal(t *testing.T) {
	v := classifyScope(AccountView{
		ID: 7, Username: "usarr", Active: true, ProvisioningMethod: provisioningShared,
	})
	if !v.Minimal() {
		t.Errorf("the correctly scoped account produced findings: %v", v.Findings)
	}
	if v.Elevated() {
		t.Error("Elevated() = true for an account with no permissions at all")
	}
}

func TestScopeClassification(t *testing.T) {
	tests := []struct {
		name         string
		account      AccountView
		wantElevated bool
		wantSubstr   string
	}{
		{
			name: "a superuser is every permission there is",
			account: AccountView{
				Active: true, ProvisioningMethod: provisioningShared,
				IsSuperuser: true, Permissions: []Permission{SuperuserPermission},
			},
			wantElevated: true, wantSubstr: "SUPERUSER",
		},
		{
			name: "a write permission is elevated",
			account: AccountView{
				Active: true, ProvisioningMethod: provisioningShared,
				Permissions: []Permission{PermLibraryDeleteBooks},
			},
			wantElevated: true, wantSubstr: "can delete books",
		},
		{
			name: "a harmless extra is reported but not elevated",
			account: AccountView{
				Active: true, ProvisioningMethod: provisioningShared,
				Permissions: []Permission{PermKoboSync},
			},
			wantElevated: false, wantSubstr: "Kobo sync is unused",
		},
		{
			name: "an unrecognised permission is ELEVATED, never assumed harmless",
			account: AccountView{
				Active: true, ProvisioningMethod: provisioningShared,
				Permissions: []Permission{"manage_time_machines"},
			},
			wantElevated: true, wantSubstr: "unrecognised permission",
		},
		{
			name: "a non-shared account is flagged; magic links are only mintable for shared ones",
			account: AccountView{
				Active: true, ProvisioningMethod: "local",
			},
			wantElevated: false, wantSubstr: `not "shared"`,
		},
		{
			name: "the wildcard without the superuser flag is still elevated",
			account: AccountView{
				Active: true, ProvisioningMethod: provisioningShared,
				Permissions: []Permission{SuperuserPermission},
			},
			wantElevated: true, wantSubstr: "wildcard permission",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := classifyScope(tc.account)
			if v.Elevated() != tc.wantElevated {
				t.Errorf("Elevated() = %v, want %v (findings: %v)", v.Elevated(), tc.wantElevated, v.Findings)
			}
			var joined []string
			for _, f := range v.Findings {
				joined = append(joined, f.String())
			}
			if !strings.Contains(strings.Join(joined, "\n"), tc.wantSubstr) {
				t.Errorf("findings do not mention %q:\n%s", tc.wantSubstr, strings.Join(joined, "\n"))
			}
		})
	}
}

// TestSuperuserIsReportedOnceNotTwice. buildUserResponse emits isSuperuser AND
// the ["*"] array together; two findings for one fact would read as two problems
// on the Services screen.
func TestSuperuserIsReportedOnceNotTwice(t *testing.T) {
	v := classifyScope(AccountView{
		Active: true, ProvisioningMethod: provisioningShared,
		IsSuperuser: true, Permissions: []Permission{SuperuserPermission},
	})
	if len(v.Findings) != 1 {
		t.Errorf("Findings = %v, want exactly one", v.Findings)
	}
}

// TestScopeIsPopulatedByTheMintAtNoExtraCost. The whole §14 enumeration arrives
// in the login response body, so it costs zero additional requests — which is
// what makes running it on every mint affordable.
func TestScopeIsPopulatedByTheMintAtNoExtraCost(t *testing.T) {
	f := newFake(t)
	f.user.Permissions = []string{string(PermManageUsers)}
	c := f.client(t)

	if _, ok := c.Scope(); ok {
		t.Error("Scope() reported a verdict before anything authenticated")
	}

	v, err := c.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !v.Elevated() {
		t.Errorf("manage_users did not read as elevated: %v", v.Findings)
	}
	if v.Account.Username != "usarr" {
		t.Errorf("Account.Username = %q, want usarr", v.Account.Username)
	}
	if got := len(f.requests()); got != 1 {
		t.Errorf("the scope verdict cost %d requests, want 1 (the mint itself)", got)
	}
}

// TestAccountViewIsAnAllowlist. BookOrbit's user object carries an operator's
// real name, their email address, their avatar URL and an OPEN
// Record<string, unknown> of settings. None of it may cross this package's
// boundary, and the guard is a distinct TYPE rather than a filter — a filter
// would pass through whatever field upstream adds next.
func TestAccountViewIsAnAllowlist(t *testing.T) {
	email := "joe@example.invalid"
	avatar := "https://bookorbit.test/avatars/7.png"
	v := toAccountView(userDTO{
		ID: 7, Username: "usarr", Name: "Joe's real name", Email: &email,
		Active: true, ProvisioningMethod: provisioningShared,
		AvatarURL: &avatar,
		Settings:  map[string]any{"secretNote": "do not ship me"},
		// The two flags that ARE carried, and why: one drives the §14 verdict,
		// the other distinguishes "your token is bad" from an account state on
		// BookOrbit's side — see ErrPasswordChangeRequired for why the mint path
		// cannot produce that state.
		IsSuperuser: false, IsDefaultPassword: true,
		Permissions: []string{"kobo_sync"},
	})

	if !v.IsDefaultPassword {
		t.Error("IsDefaultPassword was dropped; it is the one input that names the account state behind ErrPasswordChangeRequired")
	}
	if len(v.Permissions) != 1 || v.Permissions[0] != PermKoboSync {
		t.Errorf("Permissions = %v, want the one grant", v.Permissions)
	}

	// The negative half, asserted structurally rather than by reading the struct
	// definition: nothing that was on the wire and is not on the allowlist may be
	// reachable from the projection.
	blob := renderAccount(t, v)
	for _, forbidden := range []string{"Joe's real name", email, avatar, "do not ship me", "secretNote"} {
		if strings.Contains(blob, forbidden) {
			t.Errorf("AccountView leaked %q:\n%s", forbidden, blob)
		}
	}
}

func renderAccount(t *testing.T, v AccountView) string {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshalling AccountView: %v", err)
	}
	return string(b)
}
