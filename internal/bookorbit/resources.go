package bookorbit

import (
	"sort"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// ─── Wire shapes ─────────────────────────────────────────────────────────────
//
// These are what BookOrbit sends. They are unexported, and every one of them has
// an exported PROJECTION beside it that is a field-by-field ALLOWLIST. Nothing
// outside this package ever sees a wire struct.
//
// The allowlist is a distinct TYPE rather than a copy-with-some-fields-cleared,
// for the reason internal/kavita's SeriesView gives: a denylist silently passes
// through every field upstream adds next, and the next field is not knowable
// from here. BookOrbit's own user object is the case in point — it carries
// `settings` as an open Record<string, unknown>, so a projection that removed
// known-bad fields would ship whatever an operator put in there.

// magicLinkLoginRequest is the body of POST /api/v1/auth/magic-links/login.
//
// It has EXACTLY one field, and that is not tidiness. Nest's global
// ValidationPipe runs with whitelist:true AND forbidNonWhitelisted:true
// (server/src/main.ts), so an extra key is a 400 rather than an ignored field.
// MagicLinkLoginDto declares `token: string` with @MinLength(1) @MaxLength(512).
type magicLinkLoginRequest struct {
	Token string `json:"token"`
}

// magicLinkLoginResponse is what AuthService.issueTokensForUser returns.
//
// 🚩 THE CREDENTIAL COMES BACK IN A RESPONSE BODY, under the key `accessToken`.
// That is the direction internal/ssrf's deny-list was not originally written for
// and the reason `accesstoken` was added to it (see internal/ssrf/redact.go's
// camelCase note, and internal/vcrscrub's TestAccessTokenDrillArmed, which
// recorded a live JWT into a cassette before the entry existed).
//
// The `refresh_token` and `access_token` COOKIES the same response sets are
// deliberately ignored — this client re-mints instead of running a rotating
// cookie jar. See auth.go.
type magicLinkLoginResponse struct {
	AccessToken string  `json:"accessToken"`
	User        userDTO `json:"user"`
}

// userDTO is AuthService.buildUserResponse's shape.
//
// Everything here is decoded and only some of it is projected. The fields that
// exist on the wire and are NOT carried into AccountView are named in that
// type's comment with the reason.
type userDTO struct {
	ID                 int64          `json:"id"`
	Username           string         `json:"username"`
	Name               string         `json:"name"`
	Email              *string        `json:"email"`
	Active             bool           `json:"active"`
	IsSuperuser        bool           `json:"isSuperuser"`
	IsDefaultPassword  bool           `json:"isDefaultPassword"`
	Settings           map[string]any `json:"settings"`
	AvatarURL          *string        `json:"avatarUrl"`
	ProvisioningMethod string         `json:"provisioningMethod"`
	Permissions        []string       `json:"permissions"`
}

// AccountView is the ONLY shape of the BookOrbit account behind UsArr's stored
// credential that may cross this package's boundary — toward a log line, the
// Services screen, or an SSE payload.
//
// What is deliberately NOT here, and why:
//
//   - Email, Name, AvatarURL — personal details of an account that is not
//     UsArr's user. A shared service account may still carry the operator's real
//     name and address, and no UsArr screen needs either.
//   - Settings — an open Record<string, unknown> on BookOrbit's side. It is
//     whatever that install put in it, which is exactly the field an allowlist
//     exists to stop.
//
// What IS here is the §14 enumeration's whole input: the flags and the
// permission array scope.go classifies. IsDefaultPassword is carried because it
// is the difference between "your token is bad" and "the account behind the link
// is in a state that 403s every guarded route" — see ErrPasswordChangeRequired,
// which records why the mint path cannot produce that state.
type AccountView struct {
	ID                 int64        `json:"id"`
	Username           string       `json:"username"`
	Active             bool         `json:"active"`
	IsSuperuser        bool         `json:"is_superuser"`
	IsDefaultPassword  bool         `json:"is_default_password"`
	ProvisioningMethod string       `json:"provisioning_method"`
	Permissions        []Permission `json:"permissions"`
}

// toAccountView projects a userDTO onto the allowlist.
func toAccountView(u userDTO) AccountView {
	v := AccountView{
		ID:                 u.ID,
		Username:           u.Username,
		Active:             u.Active,
		IsSuperuser:        u.IsSuperuser,
		IsDefaultPassword:  u.IsDefaultPassword,
		ProvisioningMethod: u.ProvisioningMethod,
	}
	for _, p := range u.Permissions {
		v.Permissions = append(v.Permissions, Permission(p))
	}
	return v
}

// ─── GET /api/v1/health ──────────────────────────────────────────────────────

// terminusHealth is @nestjs/terminus's HealthCheckResult.
//
// The controller is @Public() at CLASS level (health.controller.ts) and its one
// check is a database ping, so this is the §17.3 reachability probe and it needs
// NO credential — which is strictly better than Kavita's arrangement, where the
// equivalent probe proves nothing about the key either but at least shares its
// transport.
//
// Terminus answers 200 when every indicator is up and 503 when any is down; the
// BODY is the same shape either way. That is why Health parses the body rather
// than reading the status code alone.
type terminusHealth struct {
	Status  string                       `json:"status"`
	Info    map[string]terminusIndicator `json:"info"`
	Error   map[string]terminusIndicator `json:"error"`
	Details map[string]terminusIndicator `json:"details"`
}

type terminusIndicator struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HealthReport is the allowlisted projection of one health check.
//
// Terminus lets an indicator return arbitrary extra keys beside `status`, so
// this carries the three fields UsArr renders and drops the rest. The Message is
// upstream text and goes through clean() on the way in — HealthService
// sanitizeErrorMessage already strips newlines and caps it at 200 characters,
// but "the upstream already bounded it" is not a reason to skip our own bound.
type HealthReport struct {
	Status     string            `json:"status"`
	Indicators []HealthIndicator `json:"indicators"`
}

// HealthIndicator is one named check.
type HealthIndicator struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// OK reports whether every indicator is up.
func (h HealthReport) OK() bool { return h.Status == "ok" }

// Failing lists the indicators that are not up, for the one-line "what is broken
// and how to fix it" the Services screen owes (§17.3).
func (h HealthReport) Failing() []HealthIndicator {
	var out []HealthIndicator
	for _, i := range h.Indicators {
		if i.Status != "up" {
			out = append(out, i)
		}
	}
	return out
}

func toHealthReport(t terminusHealth) HealthReport {
	r := HealthReport{Status: clean(t.Status)}
	// `details` is the union of `info` and `error` in Terminus's own contract, so
	// it is the one map to read; falling back to the other two keeps a partial
	// body legible instead of empty.
	src := t.Details
	if len(src) == 0 {
		src = map[string]terminusIndicator{}
		for k, v := range t.Info {
			src[k] = v
		}
		for k, v := range t.Error {
			src[k] = v
		}
	}
	for name, ind := range src {
		r.Indicators = append(r.Indicators, HealthIndicator{
			Name:    clean(name),
			Status:  clean(ind.Status),
			Message: clean(ind.Message),
		})
	}
	sort.Slice(r.Indicators, func(i, j int) bool { return r.Indicators[i].Name < r.Indicators[j].Name })
	return r
}

// ─── GET /api/v1/app-info ────────────────────────────────────────────────────

// appInfoDTO is packages/types/src/app-info.ts's AppInfoResponse.
type appInfoDTO struct {
	Version         string  `json:"version"`
	UpdateAvailable *bool   `json:"updateAvailable"`
	LatestVersion   *string `json:"latestVersion"`
	MaxUploadSizeMB float64 `json:"maxUploadSizeMb"`
}

// LocalBuildVersion is the literal a self-built or development BookOrbit
// reports.
//
// config/config.ts resolves `app.version` as `process.env.APP_VERSION ?? 'Local
// build'`, and AppInfoService.getAppInfo passes it through untouched. ANY
// VERSION-FLOOR CHECK MUST TREAT THIS AS "UNKNOWN" AND NOT AS "TOO OLD" — a
// developer's own instance is the case UsArr must not lock out. VersionKnown is
// the predicate that says so.
const LocalBuildVersion = "Local build"

// AppInfo is the allowlisted projection of the app-info read.
//
// This is the authenticated call that proves the credential works: the route
// declares no @RequirePermission and no library guard, so it succeeds for any
// valid JWT and fails 401 for none. It is deliberately NOT GET /libraries —
// knowing what a library is belongs to slice 1, and this slice must not learn it.
type AppInfo struct {
	Version         string `json:"version"`
	UpdateAvailable *bool  `json:"update_available,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	MaxUploadSizeMB int64  `json:"max_upload_size_mb"`
}

// VersionKnown reports whether Version names a release. It is false for the
// empty string and for LocalBuildVersion.
func (a AppInfo) VersionKnown() bool {
	return a.Version != "" && a.Version != LocalBuildVersion
}

func toAppInfo(d appInfoDTO) AppInfo {
	a := AppInfo{
		Version:         clean(d.Version),
		UpdateAvailable: d.UpdateAvailable,
		MaxUploadSizeMB: int64(d.MaxUploadSizeMB),
	}
	if d.LatestVersion != nil {
		a.LatestVersion = clean(*d.LatestVersion)
	}
	return a
}

// RedactedPlaceholder is what a stripped credential is replaced with in a HEADER
// or a request body. A visible marker rather than an empty string, so a redacted
// value still reads as "there was a secret here".
const RedactedPlaceholder = "<redacted>"

// RedactURL strips credential-bearing query parameters and any userinfo from a
// URL so it is safe to log, store or return in an error.
//
// For BookOrbit no credential is ever SUPPOSED to be in a URL — that is
// ADR-0052's load-bearing constraint, and jwt.strategy.ts is the primary source:
// its extractors are the Authorization header and the access_token cookie, never
// the query string. This exists anyway for the two cases the constraint does not
// cover: a base URL the operator typed with userinfo in it, and a reverse proxy
// in front that adds its own query parameter. Delegating to internal/ssrf keeps
// exactly one deny-list in the codebase.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}
	return ssrf.RedactRawURL(raw)
}
