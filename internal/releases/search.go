package releases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/servarr/mapping"
)

// Query is one release search. It carries free text and never requires a WorkRef,
// which is what makes Search-and-Grab mode possible against a service that has no
// library (ARCHITECTURE.md §8.5).
type Query struct {
	Text     string
	Type     servarr.SearchType // "" means a basic search
	Criteria servarr.Criteria

	// Categories are raw Newznab/Torznab ids. When set, an indexer that advertises
	// none of them is skipped with a stated reason rather than searched pointlessly
	// — Prowlarr would return an empty 200 indistinguishable from no-results.
	Categories []int32

	// IndexerIDs restricts the fan-out. servarr.AllUsenetIndexers (-1) and
	// AllTorrentIndexers (-2) are resolved here rather than sent upstream, because
	// the fan-out addresses one indexer per request.
	IndexerIDs []int32

	// Limit is per indexer, clamped to each indexer's capabilities.limitsMax.
	Limit int32
	// Offset is only sent to indexers whose supportsPagination is true.
	Offset int32
}

// OutcomeStatus is what happened to one indexer during a fan-out.
type OutcomeStatus string

const (
	OutcomeOK          OutcomeStatus = "ok"
	OutcomeFailed      OutcomeStatus = "failed"
	OutcomeTimedOut    OutcomeStatus = "timed_out"
	OutcomeBlocked     OutcomeStatus = "blocked"      // Prowlarr's own indexerstatus says so
	OutcomeBreakerOpen OutcomeStatus = "breaker_open" // UsArr's own breaker says so
	OutcomeDisabled    OutcomeStatus = "disabled"
	OutcomeUnsupported OutcomeStatus = "unsupported"
)

// Answered reports whether the indexer actually returned results.
func (s OutcomeStatus) Answered() bool { return s == OutcomeOK }

// IndexerOutcome is what happened to one indexer. Reason is human-readable and
// credential-free; it is written to be shown to a user.
type IndexerOutcome struct {
	IndexerID    int32
	Name         string
	Status       OutcomeStatus
	Count        int
	Reason       string
	BlockedUntil *time.Time
	Duration     time.Duration
}

// Report is the honesty layer over Prowlarr's empty-`[]`-with-200 ambiguity.
//
// Without it a search that failed on every indexer is indistinguishable from a
// search that genuinely matched nothing, and UsArr would render an empty screen
// that looks broken. With it the UI can say "3 of 8 indexers are down" and name
// them, which is principle 3 (degrade honestly).
type Report struct {
	InstanceID int64
	// Query is the query string as sent upstream, including any {Key:Value}
	// tokens. It carries no credential.
	Query string

	TotalIndexers int
	Answered      int
	Failed        int
	Skipped       int
	Results       int

	Indexers []IndexerOutcome

	StartedAt  time.Time
	FinishedAt time.Time
}

// Degraded reports whether any indexer failed to answer. When true the UI must
// say so alongside the results rather than presenting them as complete.
func (r Report) Degraded() bool { return r.Failed > 0 || r.Skipped > 0 }

// Result is one release as it crosses the boundary toward a client.
//
// It has no DownloadURL and no MagnetURL field, deliberately and permanently:
// Prowlarr embeds its full admin API key in both on every search result, and a
// shape with nowhere to put them cannot leak them. The grab works from the stored
// raw resource instead.
type Result struct {
	// CandidateID is the release_candidate row id. Grab takes this.
	CandidateID int64

	mapping.Release

	Tags []Tag

	// ExpiresAt is when this candidate stops being grabbable. Never render an
	// expired candidate with a Grab button.
	ExpiresAt time.Time

	// SupersedesCandidateID is non-zero when this result replaces one already
	// emitted for the same guid, because a lower-priority-value indexer answered
	// later. Results stream as indexers answer, so the cross-indexer dedupe cannot
	// be done up front; the client replaces the earlier row.
	SupersedesCandidateID int64
}

// EventKind discriminates a Search event.
type EventKind string

const (
	// EventKindIndexerStarted is emitted before an indexer is queried, so the UI
	// can show which indexers are still outstanding.
	EventKindIndexerStarted EventKind = "indexer_started"
	// EventKindResults carries newly-surviving results. The client merges and
	// re-ranks; ranking is progressive by construction.
	EventKindResults EventKind = "results"
	// EventKindIndexerDone carries one indexer's outcome, including failures.
	EventKindIndexerDone EventKind = "indexer_done"
	// EventKindDone is the final event and carries the Report. The channel closes
	// immediately after it.
	EventKindDone EventKind = "done"
)

// Event is one item on the Search stream. The HTTP layer turns these into SSE
// frames; it does not know that Prowlarr exists.
type Event struct {
	Kind    EventKind
	Indexer *IndexerOutcome
	Results []Result
	Report  *Report
}

// Search fans out one request per eligible indexer and streams results as each
// indexer answers.
//
// It MUST NOT be called from a render path. Release search is remote and slow: it
// is a user action behind progressive disclosure, or the primary surface of
// Search-and-Grab mode, which is still a user action.
//
// The fan-out is UsArr's own, not Prowlarr's. GET /api/v1/search dispatches every
// indexer in parallel server-side but answers only when the slowest has finished,
// gives no per-indexer progress, and swallows per-indexer failures — and Prowlarr
// has stated an aggregate streaming endpoint will not be added. So one request per
// indexer, per-indexer deadlines in single-digit seconds, independent breakers,
// and known-blocked indexers skipped entirely rather than re-timed-out on.
//
// The returned error covers synchronous setup failure only. Once the channel is
// returned, every failure arrives on it and the channel always closes.
func (s *Service) Search(ctx context.Context, scope Scope, q Query) (<-chan Event, error) {
	if !scope.Allows(s.cfg.InstanceID) {
		return nil, ErrForbidden
	}
	searchType := q.Type
	if searchType == "" {
		searchType = servarr.SearchTypeBasic
	}
	if !searchType.Valid() {
		return nil, fmt.Errorf("%w: unknown search type %q", ErrInvalidQuery, string(q.Type))
	}
	// Build the query string once, up front, so an invalid criterion fails before
	// any request goes out rather than N times inside the fan-out.
	tokens, err := q.Criteria.Tokens(searchType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidQuery, err)
	}
	queryString := strings.TrimSpace(q.Text) + tokens
	if queryString == "" {
		return nil, fmt.Errorf("%w: search needs free text or at least one criterion", ErrInvalidQuery)
	}

	// The indexer list and the blocked list are both cheap and both required
	// before the first leg can be planned, so they are on the caller's goroutine.
	listCtx, cancel := context.WithTimeout(ctx, s.cfg.PerIndexerTimeout)
	defer cancel()
	indexers, err := s.cfg.Client.Indexers(listCtx)
	if err != nil {
		return nil, fmt.Errorf("listing indexers: %w", err)
	}
	// A failure here must not fail the search: it only costs honesty about which
	// indexers are blocked, and an unusable search is worse than a less precise
	// report. Prowlarr failures are soft.
	blocked := map[int32]*time.Time{}
	if sts, err := s.cfg.Client.IndexerStatus(listCtx); err != nil {
		s.log.Warn("indexerstatus unavailable; blocked indexers cannot be reported",
			"instance_id", s.cfg.InstanceID, "err", err)
	} else {
		blocked = mapping.BlockedUntil(sts)
	}

	plan := planFanOut(indexers, blocked, q, searchType)
	if len(plan.legs) == 0 && len(plan.skipped) == 0 {
		return nil, ErrNoIndexers
	}

	out := make(chan Event, 16)
	go s.runFanOut(ctx, scope, queryString, plan, out)
	return out, nil
}

// leg is one planned per-indexer request.
type leg struct {
	indexer servarr.IndexerResource
	req     servarr.SearchRequest
}

type fanOutPlan struct {
	legs    []leg
	skipped []IndexerOutcome
	byID    map[int32]servarr.IndexerResource
}

// planFanOut decides which indexers to query and with what parameters, and
// records a stated reason for every one it skips. Every skip reason here is
// something the UI can show; none of them are silent.
func planFanOut(
	indexers []servarr.IndexerResource,
	blocked map[int32]*time.Time,
	q Query,
	searchType servarr.SearchType,
) fanOutPlan {
	plan := fanOutPlan{byID: mapping.IndexerByID(indexers)}

	want := indexerFilter(q.IndexerIDs)
	for _, ix := range indexers {
		if !want(ix) {
			continue
		}
		switch {
		case !ix.Enable:
			// GET /api/v1/indexer returns disabled indexers too, with no filter
			// parameter — so this branch is reached routinely, not exceptionally.
			plan.skipped = append(plan.skipped, IndexerOutcome{
				IndexerID: ix.ID, Name: ix.Name, Status: OutcomeDisabled,
				Reason: "indexer is disabled in Prowlarr",
			})
			continue
		case !ix.SupportsSearch:
			plan.skipped = append(plan.skipped, IndexerOutcome{
				IndexerID: ix.ID, Name: ix.Name, Status: OutcomeUnsupported,
				Reason: "indexer does not support search (RSS only)",
			})
			continue
		case !supportsSearchType(ix.Capabilities, searchType):
			plan.skipped = append(plan.skipped, IndexerOutcome{
				IndexerID: ix.ID, Name: ix.Name, Status: OutcomeUnsupported,
				Reason: fmt.Sprintf("indexer does not support %s", searchType),
			})
			continue
		case len(q.Categories) > 0 && !supportsAnyCategory(ix.Capabilities, q.Categories):
			plan.skipped = append(plan.skipped, IndexerOutcome{
				IndexerID: ix.ID, Name: ix.Name, Status: OutcomeUnsupported,
				Reason: "indexer carries none of the requested categories",
			})
			continue
		}
		if till, isBlocked := blocked[ix.ID]; isBlocked {
			// Skip entirely rather than re-timing-out on it every search. This is
			// the single biggest latency win available: a blocked indexer is
			// blocked precisely because it has been timing out.
			plan.skipped = append(plan.skipped, IndexerOutcome{
				IndexerID: ix.ID, Name: ix.Name, Status: OutcomeBlocked,
				Reason: blockedReason(till), BlockedUntil: till,
			})
			continue
		}

		req := servarr.SearchRequest{
			Text:       q.Text,
			Type:       searchType,
			Criteria:   q.Criteria,
			IndexerIDs: []int32{ix.ID},
			Categories: q.Categories,
			Limit:      servarr.Int32(servarr.EffectiveLimit(ix.Capabilities, q.Limit)),
		}
		// Never send an offset to an indexer that does not advertise pagination:
		// some ignore it and return page 1 again, which silently duplicates results.
		if q.Offset > 0 && ix.SupportsPagination {
			req.Offset = servarr.Int32(q.Offset)
		}
		plan.legs = append(plan.legs, leg{indexer: ix, req: req})
	}

	// Query the highest-priority (lowest-numbered) indexers first, so with bounded
	// concurrency the best copy of a duplicated release usually arrives first and
	// fewer supersede events are needed.
	sort.SliceStable(plan.legs, func(i, j int) bool {
		return plan.legs[i].indexer.Priority < plan.legs[j].indexer.Priority
	})
	return plan
}

// indexerFilter turns the requested indexer ids — including the -1/-2 magic
// values — into a predicate. Prowlarr resolves those magic values itself on the
// aggregate endpoint, but the fan-out addresses one indexer per request, so they
// have to be resolved here.
func indexerFilter(ids []int32) func(servarr.IndexerResource) bool {
	if len(ids) == 0 {
		return func(servarr.IndexerResource) bool { return true }
	}
	allUsenet, allTorrent := false, false
	explicit := make(map[int32]bool, len(ids))
	for _, id := range ids {
		switch id {
		case servarr.AllUsenetIndexers:
			allUsenet = true
		case servarr.AllTorrentIndexers:
			allTorrent = true
		default:
			explicit[id] = true
		}
	}
	return func(ix servarr.IndexerResource) bool {
		if explicit[ix.ID] {
			return true
		}
		if allUsenet && ix.Protocol == servarr.ProtocolUsenet {
			return true
		}
		if allTorrent && ix.Protocol == servarr.ProtocolTorrent {
			return true
		}
		return false
	}
}

func supportsSearchType(c servarr.IndexerCapabilityResource, t servarr.SearchType) bool {
	switch t {
	case servarr.SearchTypeBasic:
		// Basic search is universal; an empty searchParams array is normal.
		return true
	case servarr.SearchTypeTV:
		return len(c.TvSearchParams) > 0
	case servarr.SearchTypeMovie:
		return len(c.MovieSearchParams) > 0
	case servarr.SearchTypeMusic:
		return len(c.MusicSearchParams) > 0
	case servarr.SearchTypeBook:
		return len(c.BookSearchParams) > 0
	}
	return false
}

func supportsAnyCategory(c servarr.IndexerCapabilityResource, want []int32) bool {
	if len(c.Categories) == 0 {
		// No advertised categories means "unknown", not "none". Query it and let
		// the results speak; refusing would hide working indexers.
		return true
	}
	have := map[int32]bool{}
	var walk func([]servarr.IndexerCategory)
	walk = func(cs []servarr.IndexerCategory) {
		for _, cat := range cs {
			have[cat.ID] = true
			have[mapping.ParentCategory(cat.ID)] = true
			walk(cat.SubCategories)
		}
	}
	walk(c.Categories)
	for _, w := range want {
		if have[w] || have[mapping.ParentCategory(w)] {
			return true
		}
	}
	return false
}

func blockedReason(till *time.Time) string {
	if till == nil {
		return "Prowlarr has this indexer blocked after repeated failures"
	}
	return "Prowlarr has this indexer blocked until " + till.UTC().Format(time.RFC3339)
}

// legResult is one finished fan-out leg, handed to the single collector goroutine.
type legResult struct {
	indexer  servarr.IndexerResource
	releases []servarr.ReleaseResource
	outcome  IndexerOutcome
}

// runFanOut executes the plan. Workers run concurrently; a single collector
// goroutine owns the dedupe map, the persistence calls and the output channel, so
// none of them need locking and the SQLite writes stay serialised.
func (s *Service) runFanOut(ctx context.Context, scope Scope, queryString string, plan fanOutPlan, out chan<- Event) {
	defer close(out)

	started := s.now()
	ctx, cancel := context.WithTimeout(ctx, s.cfg.OverallTimeout)
	defer cancel()

	report := Report{
		InstanceID:    s.cfg.InstanceID,
		Query:         queryString,
		TotalIndexers: len(plan.legs) + len(plan.skipped),
		StartedAt:     started,
	}
	emit := func(e Event) bool {
		select {
		case out <- e:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Skips are known up front, so report them immediately: the user sees "these
	// three are blocked" before the first indexer has even answered.
	for _, sk := range plan.skipped {
		report.Skipped++
		report.Indexers = append(report.Indexers, sk)
		if !emit(Event{Kind: EventKindIndexerDone, Indexer: &sk}) {
			return
		}
	}

	results := make(chan legResult, len(plan.legs))
	sem := make(chan struct{}, s.cfg.Concurrency)
	var pending int

	for _, l := range plan.legs {
		pending++
		go func(l leg) {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- legResult{indexer: l.indexer, outcome: IndexerOutcome{
					IndexerID: l.indexer.ID, Name: l.indexer.Name,
					Status: OutcomeTimedOut, Reason: "search deadline reached before this indexer was queried",
				}}
				return
			}
			defer func() { <-sem }()
			results <- s.runLeg(ctx, l)
		}(l)
	}

	for _, l := range plan.legs {
		begun := IndexerOutcome{IndexerID: l.indexer.ID, Name: l.indexer.Name}
		if !emit(Event{Kind: EventKindIndexerStarted, Indexer: &begun}) {
			return
		}
	}

	// emitted tracks the best copy of each guid seen so far, so a later, better
	// (lower-priority-value) copy can supersede an already-streamed one.
	type emittedSlot struct {
		candidateID int64
		priority    int32
	}
	emitted := map[string]emittedSlot{}

	for ; pending > 0; pending-- {
		var lr legResult
		select {
		case lr = <-results:
		case <-ctx.Done():
			return
		}

		outcome := lr.outcome
		if outcome.Status == OutcomeOK {
			kept := make([]servarr.ReleaseResource, 0, len(lr.releases))
			var supersedes []int64
			for _, rel := range lr.releases {
				if rel.GUID == "" {
					// Without a guid the release cannot be deduped and cannot be
					// grabbed (grab validation requires it), so it is not a result.
					continue
				}
				prev, seen := emitted[rel.GUID]
				if seen && prev.priority <= lr.indexer.Priority {
					continue
				}
				kept = append(kept, rel)
				if seen {
					supersedes = append(supersedes, prev.candidateID)
				} else {
					supersedes = append(supersedes, 0)
				}
			}

			res, err := s.persist(ctx, scope, lr.indexer, kept)
			if err != nil {
				// Persistence failure means the release cannot be grabbed later,
				// so it is not a result — say so rather than showing a Grab button
				// that will 404.
				outcome.Status = OutcomeFailed
				outcome.Reason = "could not store results: " + err.Error()
				outcome.Count = 0
			} else {
				for i := range res {
					res[i].SupersedesCandidateID = supersedes[i]
					emitted[res[i].GUID] = emittedSlot{candidateID: res[i].CandidateID, priority: lr.indexer.Priority}
				}
				outcome.Count = len(res)
				report.Results += len(res)
				if len(res) > 0 && !emit(Event{Kind: EventKindResults, Results: res}) {
					return
				}
			}
		}

		if outcome.Status == OutcomeOK {
			report.Answered++
		} else {
			report.Failed++
		}
		report.Indexers = append(report.Indexers, outcome)
		if !emit(Event{Kind: EventKindIndexerDone, Indexer: &outcome}) {
			return
		}
	}

	report.FinishedAt = s.now()
	emit(Event{Kind: EventKindDone, Report: &report})
}

// runLeg queries one indexer under its own deadline and its own breaker.
func (s *Service) runLeg(ctx context.Context, l leg) legResult {
	out := legResult{indexer: l.indexer, outcome: IndexerOutcome{IndexerID: l.indexer.ID, Name: l.indexer.Name}}

	br := s.breakerFor(l.indexer.ID)
	if err := br.Allow(); err != nil {
		out.outcome.Status = OutcomeBreakerOpen
		out.outcome.Reason = "skipped: this indexer has been failing; next retry " +
			br.RetryAt().UTC().Format(time.RFC3339)
		return out
	}

	start := s.now()
	legCtx, cancel := context.WithTimeout(ctx, s.cfg.PerIndexerTimeout)
	defer cancel()

	rels, err := s.cfg.Client.Search(legCtx, l.req)
	out.outcome.Duration = s.now().Sub(start)

	switch {
	case err == nil:
		br.Success()
		out.releases = rels
		out.outcome.Status = OutcomeOK
		// A bare empty slice is NOT evidence of "no results" — see the package
		// doc. The Report is what carries the distinction, and this leg's honest
		// contribution to it is "this indexer answered and had nothing".
		if len(rels) == 0 {
			out.outcome.Reason = "indexer answered with no matches"
		}
		return out
	case errors.Is(err, servarr.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		br.Failure()
		out.outcome.Status = OutcomeTimedOut
		out.outcome.Reason = fmt.Sprintf("indexer did not answer within %s", s.cfg.PerIndexerTimeout)
	case errors.Is(err, context.Canceled):
		out.outcome.Status = OutcomeTimedOut
		out.outcome.Reason = "search was cancelled"
	case errors.Is(err, servarr.ErrUnauthorized):
		// Not the indexer's fault and not retryable: do not trip its breaker.
		out.outcome.Status = OutcomeFailed
		out.outcome.Reason = "Prowlarr rejected UsArr's API key"
	case errors.Is(err, servarr.ErrValidation):
		// The only 400 from search is SearchFailedException, raised only when
		// indexerIds was explicitly non-empty and every named indexer was
		// unavailable — which, in a fan-out of one, means exactly this indexer.
		br.Failure()
		out.outcome.Status = OutcomeFailed
		out.outcome.Reason = "Prowlarr reports this indexer is unavailable"
	default:
		br.Failure()
		out.outcome.Status = OutcomeFailed
		out.outcome.Reason = "indexer query failed: " + err.Error()
	}
	return out
}

// persist writes release candidates and returns the client-facing results.
//
// raw_release_json is stored SANITISED — servarr.SanitizeRelease drops
// downloadUrl and magnetUrl, the two fields into which Prowlarr's
// SearchController.MapReleases splices its own FULL ADMIN API KEY on every
// result. It is stored for the provenance fields (protocol, indexer, categories,
// indexerFlags, infoHash, infoUrl, guid, title, publishDate, downloadClientId),
// NOT because the grab needs the resource echoed back verbatim: Client.Grab
// sends rel.GrabBody(), which is guid + indexerId + downloadClientId and nothing
// else, and Prowlarr resolves the release from its own cache keyed
// "{indexerId}_{guid}". Neither dropped field is read by any production path —
// see the note on Candidate.RawReleaseJSON.
//
// Sanitising here rather than at the read boundary is deliberate. provenance
// rows and release_candidate.download_url are already left empty for the stated
// reason that "a credential written here is in every backup forever"; this blob
// lands in the same file, the same VACUUM INTO backup and the same support
// bundle, so the same rule has to apply to it.
//
// The SAME rule covers release_candidate.info_url, which is written from the
// same resource. infoUrl is INDEXER-supplied, and a private tracker puts the
// user's personal passkey in it as a query parameter; redaction used to happen
// only at the HTTP boundary (httpapi.redactURLField), so the API responses were
// clean while SQLite held the passkey verbatim — in every backup, permanently,
// and a leaked passkey is account termination on a private tracker. It is
// redacted rather than dropped: the host and the release path are the provenance
// this column exists for, and nothing on the grab path reads it at all
// (GrabBody is guid + indexerId + downloadClientId).
func (s *Service) persist(ctx context.Context, scope Scope, ix servarr.IndexerResource, rels []servarr.ReleaseResource) ([]Result, error) {
	if len(rels) == 0 {
		return nil, nil
	}
	now := s.now()
	expires := now.Add(CandidateTTL)

	cands := make([]Candidate, 0, len(rels))
	for _, rel := range rels {
		// SanitizeRelease before marshalling, never after: the credential must not
		// reach the byte slice that gets handed to the store at all.
		raw, err := json.Marshal(servarr.SanitizeRelease(rel))
		if err != nil {
			return nil, fmt.Errorf("encoding release %q: %w", rel.GUID, err)
		}
		var infoHash string
		if rel.InfoHash != nil {
			infoHash = *rel.InfoHash
		}
		cands = append(cands, Candidate{
			// The searcher owns the candidate. Written from the scope, not
			// defaulted to the sentinel: a row that cannot say whose search
			// produced it cannot be read back under an access scope either.
			UserID:            scope.UserID,
			ServiceInstanceID: s.cfg.InstanceID,
			GUID:              rel.GUID,
			Title:             rel.Title,
			Indexer:           firstNonEmpty(rel.Indexer, ix.Name),
			IndexerID:         rel.IndexerID,
			Protocol:          mapping.SourceValue(rel.Protocol),
			Categories:        rel.CategoryIDs(),
			SizeBytes:         rel.Size,
			Seeders:           rel.Seeders,
			Leechers:          rel.Leechers,
			AgeDays:           float64(rel.Age),
			// Redacted, not verbatim: see the note on persist. The same
			// deny-list the HTTP boundary uses, so the stored value and the
			// served value cannot disagree.
			InfoURL:          servarr.RedactURL(rel.InfoURL),
			InfoHash:         infoHash,
			DownloadClientID: rel.DownloadClientID,
			RawReleaseJSON:   raw,
			FetchedAt:        now,
			ExpiresAt:        expires,
		})
	}

	ids, err := s.cfg.Store.InsertCandidates(ctx, cands)
	if err != nil {
		return nil, fmt.Errorf("storing release candidates: %w", err)
	}
	if len(ids) != len(cands) {
		return nil, fmt.Errorf("store returned %d ids for %d candidates", len(ids), len(cands))
	}

	out := make([]Result, len(rels))
	for i, rel := range rels {
		out[i] = Result{
			CandidateID: ids[i],
			Release:     mapping.FromProwlarrRelease(rel, &ix),
			Tags:        DeriveTags(rel, &ix),
			ExpiresAt:   expires,
		}
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// firstNonZero picks the first reported value. Zero means "not reported" for
// every field it is used on — a release with a genuine size of zero bytes is not
// a thing an indexer offers — so the fallthrough is to the next source rather
// than to a claim that the release is empty.
func firstNonZero(vals ...int64) int64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}
