// Package releases implements Search-and-Grab mode (ARCHITECTURE.md §8.5).
//
// It owns the flow, not the HTTP layer. The HTTP layer turns the event channel
// this package returns into SSE frames and knows nothing about Prowlarr; this
// package knows nothing about SSE.
//
// Three facts about Prowlarr shape the whole design, and none of them are
// negotiable:
//
//  1. GET /api/v1/search returns HTTP 200 with `[]` when every indexer failed,
//     when all are rate-limited, and when no enabled indexer supports the
//     requested categories — identically to a genuine no-results. Per-indexer
//     failures are swallowed. So every search is correlated against
//     GET /api/v1/indexerstatus and every result set carries a Report saying which
//     indexers answered, which were skipped and why. "3 of 8 indexers are down" is
//     principle 3 (degrade honestly) and it is not retrofittable onto a bare `[]`.
//
//  2. The aggregate endpoint gives no per-indexer progress and answers only when
//     the slowest indexer has answered or failed — 45-60 s is an ordinary worst
//     case, and Prowlarr has stated an aggregate multi-indexer streaming endpoint
//     will not be added. Progressive results therefore require UsArr to fan out
//     one request per indexer itself and merge, which is what Search does.
//
//  3. A release can only be grabbed if it is still in Prowlarr's in-process cache,
//     keyed "{indexerId}_{guid}", non-rolling 30-minute TTL, lost on process
//     restart. Candidates expire at 25 minutes and an expired candidate is never
//     rendered as grabbable.
//
// Nothing here may be called from a render path. Release search is remote and
// slow by construction; it is a user action behind progressive disclosure, or —
// in Search-and-Grab mode — the primary surface, which is still a user action.
package releases
