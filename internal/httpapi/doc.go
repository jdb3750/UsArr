// Package httpapi is UsArr's HTTP surface: the JSON API, the one SSE stream and
// the embedded SPA.
//
// # The boundary that makes replica-not-proxy true
//
// This package MUST NOT import internal/servarr, and no handler here may hold an
// outbound HTTP client (ARCHITECTURE.md §2.3 rule 1, §4.1, ADR-0004). Handlers
// read from internal/store and drive internal/releases; anything that has to
// talk to an *Arr is reached through one of the small interfaces in ports.go,
// implemented by cmd/usarr, which owns the clients.
//
// Two consequences that look like awkwardness and are not:
//
//   - GET /api/v1/search does not search. It validates, allocates a search id,
//     hands the work to a background goroutine and returns. Results arrive on
//     GET /api/events. A render path that waited for Prowlarr would wait 45-60 s
//     on an ordinary bad day.
//   - GET /api/v1/services/health renders entirely from SQLite plus the last
//     probe snapshot taken off the render path. The *Arr's own health warnings
//     and its blocked indexers are in that snapshot; they are never fetched
//     while a browser waits.
//
// The one deliberate exception is POST /api/v1/services/{id}/test — the wizard's
// mandatory connection test (§11.3). It is a user action whose entire purpose is
// to make an upstream call, it is bounded by a deadline, and it is not a render
// path.
//
// # Credential rules
//
// A service API key is accepted on create and update and is never returned by
// any response. release_candidate.raw_release_json holds Prowlarr's full admin
// key inside downloadUrl/magnetUrl and is server-side only, forever; the types
// this package serialises have nowhere to put it, and TestNoDownloadURLEver
// asserts that at the HTTP boundary as well.
package httpapi
