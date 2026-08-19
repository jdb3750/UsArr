package bookorbit

import (
	"fmt"
	"sort"
)

// ─── The §14 scope gate, discharged ──────────────────────────────────────────
//
// ADR-0052 left a NAMED gate rather than an open end: "the BookOrbit adapter may
// not read a catalogue under a shared-account credential until the scope that
// account grants has been enumerated against §14 — specifically whether it
// confers write or admin reach beyond the catalogue read UsArr needs … Closes in
// the adapter thread, before the first credential is stored."
//
// This file is that enumeration, and it is enumerated in CODE rather than in a
// document on purpose: a paragraph saying "an empty permission set is
// sufficient" cannot notice when BookOrbit adds a 24th permission, and
// TestEveryBookOrbitPermissionIsClassified can.
//
// WHAT THE ENUMERATION FOUND, from source at bookorbit/bookorbit@73b7877d:
//
//   - Authorization is three independent APP_GUARDs in order (app.module.ts):
//     JwtAuthGuard, PermissionGuard, LibraryAccessGuard.
//   - Permissions are ADDITIVE AND DEFAULT TO NONE. UserService.createSharedUser
//     calls userRepo.setPermissions only when permissionNames.length > 0, and
//     PermissionGuard returns true for a route that declares no
//     @RequirePermission.
//   - Every route this adapter needs — health, app-info, and (from slice 1)
//     GET /libraries, POST /libraries/:id/books, POST /books/query,
//     GET /books/:id and its cover routes — declares NO @RequirePermission.
//   - Library visibility is a separate, explicit grant: userLibraryAccess rows
//     created from CreateSharedUserDto.libraryIds.
//
// So THE CORRECT CREDENTIAL IS A SHARED ACCOUNT WITH AN EMPTY PERMISSION SET AND
// AN EXPLICIT libraryIds GRANT. Anything more is over-scoped, and this file is
// what says so at runtime instead of hoping the operator read the docs.
//
// WHY THIS CLIENT REPORTS RATHER THAN REFUSES. An over-scoped credential is a
// §14 problem, but refusing to connect would leave the operator staring at a
// service that will not talk to them with no way to see why — the opposite of
// principle 3's "says what is missing and why". The verdict is therefore
// computed on every mint, exposed through Client.Scope, and logged at WARN. The
// gate that ADR-0052 actually names is on READING A CATALOGUE, which is slice 1;
// slice 1 consults Elevated() before its first StreamItems and this slice ships
// the mechanism it will consult.

// Permission is one member of BookOrbit's permission vocabulary.
//
// The values are the enum's STRING values, not its TypeScript member names,
// because the string is what crosses the wire in the login response's
// `user.permissions` array (packages/types/src/permissions.ts@73b7877d).
type Permission string

// The 23 members of BookOrbit's Permission enum, in the source's own order and
// grouping. Transcribed from packages/types/src/permissions.ts@73b7877d.
const (
	// Content
	PermLibraryDownload     Permission = "library_download"
	PermLibraryUpload       Permission = "library_upload"
	PermLibraryEditMetadata Permission = "library_edit_metadata"
	PermLibraryDeleteBooks  Permission = "library_delete_books"
	PermBookDockAccess      Permission = "book_dock_access"
	PermDemoRestricted      Permission = "demo_restricted"

	// Devices & Access
	PermKoboSync       Permission = "kobo_sync"
	PermKoreaderSync   Permission = "koreader_sync"
	PermHardcoverSync  Permission = "hardcover_sync"
	PermReadwiseSync   Permission = "readwise_sync"
	PermStorygraphSync Permission = "storygraph_sync"
	PermOpdsAccess     Permission = "opds_access"

	// Email
	PermEmailSend   Permission = "email_send"
	PermManageEmail Permission = "manage_email"

	// Administration
	PermManageLibraries      Permission = "manage_libraries"
	PermManageMetadataConfig Permission = "manage_metadata_config"
	PermManageIcons          Permission = "manage_icons"
	PermManageAppSettings    Permission = "manage_app_settings"
	PermManageBookDock       Permission = "manage_book_dock"
	PermManageUsers          Permission = "manage_users"
	PermViewUserActivity     Permission = "view_user_activity"
	PermViewAuditLog         Permission = "view_audit_log"

	// Notifications
	PermNotificationAccess Permission = "notification_access"
)

// SuperuserPermission is not a member of the enum. AuthService.buildUserResponse
// substitutes the literal ["*"] for a superuser's permission array rather than
// listing them, so it arrives on the wire where a Permission would be and has to
// be recognised.
const SuperuserPermission Permission = "*"

// allPermissions is the whole vocabulary. It exists so
// TestEveryBookOrbitPermissionIsClassified can iterate it: a 24th permission
// upstream makes that test RED, which is the only mechanism in this package that
// notices BookOrbit growing one.
var allPermissions = []Permission{
	PermLibraryDownload, PermLibraryUpload, PermLibraryEditMetadata, PermLibraryDeleteBooks,
	PermBookDockAccess, PermDemoRestricted,
	PermKoboSync, PermKoreaderSync, PermHardcoverSync, PermReadwiseSync, PermStorygraphSync, PermOpdsAccess,
	PermEmailSend, PermManageEmail,
	PermManageLibraries, PermManageMetadataConfig, PermManageIcons, PermManageAppSettings,
	PermManageBookDock, PermManageUsers, PermViewUserActivity, PermViewAuditLog,
	PermNotificationAccess,
}

// elevatedPermissions confer WRITE OR ADMIN REACH beyond a catalogue read, which
// is the exact test §14 applies to a stored credential and the exact words
// ADR-0052's gate uses.
//
// Two entries are broader than the scoping note that preceded this file, and
// both widenings are deliberate:
//
//   - library_download is here even though downloading is a read. A stored
//     credential that can pull every byte of the library is exfiltration reach,
//     and UsArr does not need it: covers come from /books/:id/cover and
//     /books/:id/thumbnail, neither of which declares a permission.
//   - manage_icons is here. The note listed twelve Manage*/View* names and
//     omitted this one; it is an app-settings write like its siblings and there
//     is no argument for treating it differently.
//
// The bias is towards flagging. A false flag costs the operator one sentence on
// the Services screen; a missed one is a full-admin-equivalent credential stored
// under a scheme that says it is not.
var elevatedPermissions = map[Permission]string{
	PermLibraryDownload:      "can download every byte of every library it can see",
	PermLibraryUpload:        "can write files into the library",
	PermLibraryEditMetadata:  "can rewrite the metadata UsArr replicates",
	PermLibraryDeleteBooks:   "can delete books",
	PermEmailSend:            "can send books out by email",
	PermManageEmail:          "can reconfigure BookOrbit's outbound email",
	PermManageLibraries:      "can create, edit and delete libraries",
	PermManageMetadataConfig: "can reconfigure the metadata providers",
	PermManageIcons:          "can write application icon settings",
	PermManageAppSettings:    "can change application settings",
	PermManageBookDock:       "can administer the Book Dock",
	PermManageUsers:          "can create and edit users, including granting superuser",
	PermViewUserActivity:     "can read other people's reading activity",
	PermViewAuditLog:         "can read the audit log",
}

// unneededPermissions grant no write or admin reach but are still more than a
// catalogue replica needs. They are reported at a lower severity rather than
// dropped, because "this credential does more than it has to" is worth one line
// on a setup screen even when nothing is at risk.
var unneededPermissions = map[Permission]string{
	PermBookDockAccess:     "Book Dock access is unused by UsArr",
	PermDemoRestricted:     "demo_restricted RESTRICTS rather than grants, and will make some routes refuse",
	PermKoboSync:           "Kobo sync is unused by UsArr",
	PermKoreaderSync:       "KOReader sync is unused by UsArr",
	PermHardcoverSync:      "Hardcover sync is unused by UsArr",
	PermReadwiseSync:       "Readwise sync is unused by UsArr",
	PermStorygraphSync:     "StoryGraph sync is unused by UsArr",
	PermOpdsAccess:         "OPDS access is unused by UsArr; the adapter reads /api/v1 only (ADR-0052)",
	PermNotificationAccess: "notification access is unused by UsArr",
}

// ScopeSeverity grades one finding.
type ScopeSeverity int

const (
	// ScopeUnneeded is more reach than UsArr uses, with no write or admin power.
	ScopeUnneeded ScopeSeverity = iota + 1
	// ScopeElevated is write or admin reach beyond the catalogue read — the
	// condition ADR-0052's gate names.
	ScopeElevated
)

func (s ScopeSeverity) String() string {
	switch s {
	case ScopeUnneeded:
		return "unneeded"
	case ScopeElevated:
		return "elevated"
	default:
		return "unknown"
	}
}

// ScopeFinding is one thing wrong with the account behind the stored credential.
type ScopeFinding struct {
	Severity ScopeSeverity
	// Permission is empty for a finding that is not about a permission — the
	// superuser flag and the provisioning method.
	Permission Permission
	Detail     string
}

func (f ScopeFinding) String() string {
	if f.Permission == "" {
		return fmt.Sprintf("[%s] %s", f.Severity, f.Detail)
	}
	return fmt.Sprintf("[%s] %s: %s", f.Severity, f.Permission, f.Detail)
}

// ScopeVerdict is the whole §14 answer for one credential.
type ScopeVerdict struct {
	Account  AccountView
	Findings []ScopeFinding
}

// Elevated reports whether the account carries write or admin reach beyond the
// catalogue read. This is the predicate ADR-0052's gate turns on, and slice 1
// must consult it before its first catalogue read.
func (v ScopeVerdict) Elevated() bool {
	for _, f := range v.Findings {
		if f.Severity == ScopeElevated {
			return true
		}
	}
	return false
}

// Minimal reports the happy case: a non-superuser, shared-provisioned account
// with no permissions at all.
func (v ScopeVerdict) Minimal() bool { return len(v.Findings) == 0 }

// classifyScope computes the verdict for one account.
//
// It never mutates and never calls out. Everything it needs arrives in the
// magic-link login response — AuthService.buildUserResponse returns isSuperuser,
// provisioningMethod and permissions in the same body as the accessToken — so
// the gate costs ZERO extra requests, which is why it can run on every mint.
func classifyScope(a AccountView) ScopeVerdict {
	v := ScopeVerdict{Account: a}

	if a.IsSuperuser {
		v.Findings = append(v.Findings, ScopeFinding{
			Severity: ScopeElevated,
			Detail: "the account is a SUPERUSER, which is every permission there is; " +
				"MagicLinkService.createToken only mints links for shared accounts, so this account was " +
				"promoted after its link was created",
		})
	}
	// provisioningMethod 'shared' is what MagicLinkService.createToken requires
	// of its target, so anything else here means the account changed under the
	// link. Reported rather than refused: the link still works.
	if a.ProvisioningMethod != "" && a.ProvisioningMethod != provisioningShared {
		v.Findings = append(v.Findings, ScopeFinding{
			Severity: ScopeUnneeded,
			Detail: fmt.Sprintf("provisioningMethod is %q, not %q; magic links are only mintable for shared accounts",
				a.ProvisioningMethod, provisioningShared),
		})
	}
	if !a.Active {
		v.Findings = append(v.Findings, ScopeFinding{
			Severity: ScopeUnneeded,
			Detail:   "the account is marked inactive; BookOrbit will refuse the next mint",
		})
	}

	for _, p := range a.Permissions {
		switch {
		case p == SuperuserPermission:
			// Already covered by the IsSuperuser finding; buildUserResponse emits
			// both together and reporting it twice would read as two problems.
			if !a.IsSuperuser {
				v.Findings = append(v.Findings, ScopeFinding{
					Severity: ScopeElevated, Permission: p,
					Detail: "the wildcard permission is present without the superuser flag",
				})
			}
		case elevatedPermissions[p] != "":
			v.Findings = append(v.Findings, ScopeFinding{
				Severity: ScopeElevated, Permission: p, Detail: elevatedPermissions[p],
			})
		case unneededPermissions[p] != "":
			v.Findings = append(v.Findings, ScopeFinding{
				Severity: ScopeUnneeded, Permission: p, Detail: unneededPermissions[p],
			})
		default:
			// A permission this build has never heard of. UNKNOWN IS ELEVATED, not
			// ignored: the whole value of the classification is that it fails
			// loudly when BookOrbit grows a permission, and defaulting an
			// unrecognised grant to "harmless" would make it fail silently instead.
			v.Findings = append(v.Findings, ScopeFinding{
				Severity: ScopeElevated, Permission: p,
				Detail: "unrecognised permission; this build cannot judge what it grants, so it is treated as elevated",
			})
		}
	}

	sort.SliceStable(v.Findings, func(i, j int) bool {
		return v.Findings[i].Severity > v.Findings[j].Severity
	})
	return v
}

// provisioningShared is the value UserService.createSharedUser writes and
// MagicLinkService.createToken requires.
const provisioningShared = "shared"
