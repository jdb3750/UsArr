// Package servarr is UsArr's single client for the *Arr family.
//
// Sonarr, Radarr, Lidarr, Prowlarr and Whisparr are forks of one codebase. They
// share the auth header (X-Api-Key), the content-negotiation trap
// (ReturnHttpNotAcceptable = true), the anonymous /ping probe, the
// X-Application-Version response header and — critically — the same three error
// envelope shapes. Writing six clients would mean fixing each of those quirks six
// times, so there is one client here and one set of resource types per service.
// See docs/DEVELOPMENT.md §5.
//
// Only Prowlarr is implemented. The seam for Sonarr/Radarr is the apiPath field
// on Client plus a second resources file; nothing else in this package assumes v1.
// No speculative abstraction has been built for services with no code behind them.
//
// Two rules this package enforces rather than documents:
//
//   - The API key never appears in an error string, a log line or a returned URL.
//     *Arr keys are full-admin credentials (CLAUDE.md, security.md §5).
//   - ReleaseResource.downloadUrl and .magnetUrl carry Prowlarr's own API key in
//     plaintext, because SearchController.MapReleases rewrites them into
//     `{serverUrl}{urlBase}/{indexerId}/download?apikey={ApiKey}&link=…`. Nothing
//     may hand those two fields to a browser or to a log. See Redact and
//     SanitizeRelease.
//
// The caller injects the *http.Client (as an HTTPDoer). That is deliberate: the
// SSRF policy lives in internal/ssrf and is bound to one service_instance's
// validated host:port, and this package must not be able to bypass it.
package servarr
