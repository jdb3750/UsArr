package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/releases"
	"github.com/jdb3750/UsArr/internal/store"
)

// GET /api/v1/search does not search. It validates the query, picks the
// instance, allocates a search id and returns — then a background goroutine runs
// the fan-out and publishes onto the one SSE stream.
//
// That is not an optimisation. Prowlarr's aggregate endpoint answers only when
// the SLOWEST indexer has answered or failed, and 45-60 s is an ordinary worst
// case; internal/releases fans out one request per indexer precisely so results
// can arrive progressively. A handler that waited for that would be a render
// path blocking on an upstream call, which is the one thing the architecture is
// built to prevent (ARCHITECTURE.md §2.3 rule 1).
//
// The "these indexers are blocked or failed" signal rides the same stream, so
// the UI can say what is missing rather than rendering an empty screen that
// looks broken (principle 3).

// searchBudget bounds a background search. It is longer than any HTTP handler's
// budget on purpose: nothing is waiting on it.
const searchBudget = 90 * time.Second

type searchAcceptedResponse struct {
	SearchID   string `json:"search_id"`
	InstanceID int64  `json:"instance_id"`
	Query      string `json:"query"`
	Type       string `json:"type"`
	EventsURL  string `json:"events_url"`
	Message    string `json:"message"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	q := r.URL.Query()
	text := strings.TrimSpace(q.Get("query"))
	if text == "" {
		text = strings.TrimSpace(q.Get("q"))
	}

	searchType := strings.TrimSpace(q.Get("type"))
	categories, err := queryInt32s(r, "category")
	if err != nil {
		return err
	}
	indexerIDs, err := queryInt32s(r, "indexer")
	if err != nil {
		return err
	}
	limit, err := queryInt32(r, "limit")
	if err != nil {
		return err
	}
	offset, err := queryInt32(r, "offset")
	if err != nil {
		return err
	}

	query := releases.Query{
		Text:       text,
		Categories: categories,
		IndexerIDs: indexerIDs,
		Limit:      limit,
		Offset:     offset,
	}
	if err := setSearchType(&query, searchType); err != nil {
		return err
	}
	setCriteria(&query, r)

	instanceID, err := s.resolveIndexerInstance(r, a)
	if err != nil {
		return err
	}
	searcher, err := s.searcherFor(r.Context(), instanceID)
	if err != nil {
		return err
	}
	scope, err := s.releasesScope(r, a)
	if err != nil {
		return err
	}

	// Start the fan-out on a context that is NOT the request's: the request is
	// about to return, and cancelling the search when it does would make the
	// whole design pointless. The budget is explicit, so nothing leaks.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), searchBudget)

	events, err := searcher.Search(ctx, scope, query)
	if err != nil {
		cancel()
		return searchStartError(err)
	}

	searchID, err := newSearchID()
	if err != nil {
		cancel()
		return errStatus(http.StatusInternalServerError, CodeInternal, "a search id could not be generated").wrapping(err)
	}

	s.hub.Publish(a.User.ID, EventSearchStarted, map[string]any{
		"search_id":   searchID,
		"instance_id": instanceID,
		"query":       text,
		"type":        string(query.Type),
	})
	go s.pumpSearch(ctx, cancel, a.User.ID, searchID, events)

	writeJSON(w, http.StatusAccepted, searchAcceptedResponse{
		SearchID:   searchID,
		InstanceID: instanceID,
		Query:      text,
		Type:       string(query.Type),
		EventsURL:  s.cfg.URLBase + "/api/events",
		Message:    "results stream on /api/events as each indexer answers",
	})
	return nil
}

// pumpSearch turns internal/releases' events into SSE frames.
//
// This is the whole of the coupling between the two: internal/releases knows
// nothing about SSE, and this package knows nothing about Prowlarr.
func (s *Server) pumpSearch(ctx context.Context, cancel context.CancelFunc, userID int64, searchID string, events <-chan releases.Event) {
	defer cancel()
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("panic in search pump", "search_id", searchID, "panic", redactText(toString(rec)))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			s.hub.Publish(userID, EventSearchFailed, map[string]any{
				"search_id": searchID,
				"message":   "the search ran out of time before every indexer answered",
				"action":    "Search again",
			})
			return
		case ev, open := <-events:
			if !open {
				return
			}
			s.publishSearchEvent(userID, searchID, ev)
			if ev.Kind == releases.EventKindDone {
				return
			}
		}
	}
}

func (s *Server) publishSearchEvent(userID int64, searchID string, ev releases.Event) {
	switch ev.Kind {
	case releases.EventKindIndexerStarted, releases.EventKindIndexerDone:
		if ev.Indexer == nil {
			return
		}
		s.hub.Publish(userID, EventSearchIndexer, map[string]any{
			"search_id": searchID,
			"phase":     string(ev.Kind),
			"indexer":   toIndexerOutcome(*ev.Indexer),
		})
	case releases.EventKindResults:
		if len(ev.Results) == 0 {
			return
		}
		out := make([]releaseResponse, 0, len(ev.Results))
		for _, res := range ev.Results {
			out = append(out, toReleaseResponse(res))
		}
		s.hub.Publish(userID, EventSearchResults, map[string]any{
			"search_id": searchID,
			"results":   out,
		})
	case releases.EventKindDone:
		if ev.Report == nil {
			return
		}
		s.hub.Publish(userID, EventSearchDone, map[string]any{
			"search_id": searchID,
			"report":    toReportResponse(*ev.Report),
		})
	}
}

// ── response shapes ─────────────────────────────────────────────────────────

// releaseResponse is one grabbable release as it crosses to a client.
//
// There is NO download_url and NO magnet_url field, permanently. Prowlarr
// rewrites both into a proxy link carrying its FULL ADMIN API KEY on every
// search result, so a shape with nowhere to put them cannot leak them. The grab
// works from the server-side stored resource instead — see handleGrab.
//
// InfoURL, CommentURL and PosterURL are indexer-supplied and go through the
// query-parameter deny-list, because a private tracker puts a passkey in them.
type releaseResponse struct {
	CandidateID int64  `json:"candidate_id"`
	GUID        string `json:"guid"`
	Title       string `json:"title"`

	IndexerID       int32  `json:"indexer_id"`
	IndexerName     string `json:"indexer_name,omitempty"`
	IndexerPriority int32  `json:"indexer_priority"`
	IndexerPrivacy  string `json:"indexer_privacy,omitempty"`

	Protocol   string   `json:"protocol"`
	Categories []int32  `json:"categories,omitempty"`
	Flags      []string `json:"indexer_flags,omitempty"`

	SizeBytes int64     `json:"size_bytes"`
	Seeders   *int32    `json:"seeders,omitempty"`
	Leechers  *int32    `json:"leechers,omitempty"`
	Grabs     *int32    `json:"grabs,omitempty"`
	Files     *int32    `json:"files,omitempty"`
	AgeDays   float64   `json:"age_days"`
	Published time.Time `json:"published_at"`

	InfoURL    string `json:"info_url,omitempty"`
	CommentURL string `json:"comment_url,omitempty"`
	PosterURL  string `json:"poster_url,omitempty"`
	InfoHash   string `json:"info_hash,omitempty"`

	Tags []string `json:"tags,omitempty"`

	// ExpiresAt is when this candidate stops being grabbable. Prowlarr's grab
	// cache is a non-rolling 30 minutes; never render an expired candidate with
	// a Grab button.
	ExpiresAt time.Time `json:"expires_at"`

	// Supersedes names a candidate this result replaces, because a
	// higher-priority indexer answered later. Results stream as indexers
	// answer, so the cross-indexer dedupe cannot be done up front.
	Supersedes int64 `json:"supersedes_candidate_id,omitempty"`
}

func toReleaseResponse(res releases.Result) releaseResponse {
	tags := make([]string, 0, len(res.Tags))
	for _, t := range res.Tags {
		tags = append(tags, t.String())
	}
	return releaseResponse{
		CandidateID:     res.CandidateID,
		GUID:            res.GUID,
		Title:           res.Title,
		IndexerID:       res.IndexerID,
		IndexerName:     res.IndexerName,
		IndexerPriority: res.IndexerPriority,
		IndexerPrivacy:  res.IndexerPrivacy,
		Protocol:        res.Protocol,
		Categories:      res.Categories,
		Flags:           res.IndexerFlags,
		SizeBytes:       res.SizeBytes,
		Seeders:         res.Seeders,
		Leechers:        res.Leechers,
		Grabs:           res.Grabs,
		Files:           res.Files,
		AgeDays:         res.AgeDays,
		Published:       res.PublishedAt,
		InfoURL:         redactURLField(res.InfoURL),
		CommentURL:      redactURLField(res.CommentURL),
		PosterURL:       redactURLField(res.PosterURL),
		InfoHash:        res.InfoHash,
		Tags:            tags,
		ExpiresAt:       res.ExpiresAt,
		Supersedes:      res.SupersedesCandidateID,
	}
}

type indexerOutcomeResponse struct {
	IndexerID    int32      `json:"indexer_id"`
	Name         string     `json:"name,omitempty"`
	Status       string     `json:"status"`
	Answered     bool       `json:"answered"`
	Count        int        `json:"count"`
	Reason       string     `json:"reason,omitempty"`
	BlockedUntil *time.Time `json:"blocked_until,omitempty"`
	DurationMS   int64      `json:"duration_ms,omitempty"`
}

func toIndexerOutcome(o releases.IndexerOutcome) indexerOutcomeResponse {
	return indexerOutcomeResponse{
		IndexerID:    o.IndexerID,
		Name:         o.Name,
		Status:       string(o.Status),
		Answered:     o.Status.Answered(),
		Count:        o.Count,
		Reason:       redactText(o.Reason),
		BlockedUntil: o.BlockedUntil,
		DurationMS:   o.Duration.Milliseconds(),
	}
}

type reportResponse struct {
	InstanceID    int64                    `json:"instance_id"`
	Query         string                   `json:"query"`
	TotalIndexers int                      `json:"total_indexers"`
	Answered      int                      `json:"answered"`
	Failed        int                      `json:"failed"`
	Skipped       int                      `json:"skipped"`
	Results       int                      `json:"results"`
	Degraded      bool                     `json:"degraded"`
	Summary       string                   `json:"summary"`
	Indexers      []indexerOutcomeResponse `json:"indexers"`
	StartedAt     time.Time                `json:"started_at"`
	FinishedAt    time.Time                `json:"finished_at"`
}

func toReportResponse(rep releases.Report) reportResponse {
	ixs := make([]indexerOutcomeResponse, 0, len(rep.Indexers))
	for _, o := range rep.Indexers {
		ixs = append(ixs, toIndexerOutcome(o))
	}
	return reportResponse{
		InstanceID:    rep.InstanceID,
		Query:         rep.Query,
		TotalIndexers: rep.TotalIndexers,
		Answered:      rep.Answered,
		Failed:        rep.Failed,
		Skipped:       rep.Skipped,
		Results:       rep.Results,
		Degraded:      rep.Degraded(),
		Summary:       reportSummary(rep),
		Indexers:      ixs,
		StartedAt:     rep.StartedAt,
		FinishedAt:    rep.FinishedAt,
	}
}

// reportSummary is the sentence the UI shows beside the results.
//
// Prowlarr returns HTTP 200 with `[]` when every indexer failed, when all are
// rate-limited, and when nothing matched — identically. Without this sentence a
// total failure is indistinguishable from a genuine no-results, which is exactly
// the empty screen that looks broken.
//
// The noun agrees with the denominator, not the numerator: a one-indexer install
// is the common case, and "1 of 1 indexers answered" on the first screen a new
// user reaches reads as a bug in everything behind it. "%d failed, %d skipped"
// elides the noun on purpose — it stays correct at every count.
func reportSummary(rep releases.Report) string {
	if !rep.Degraded() {
		return fmt.Sprintf("%d of %s answered", rep.Answered, countNoun(rep.TotalIndexers, "indexer"))
	}
	return fmt.Sprintf("%d of %s answered — %d failed, %d skipped; these results are incomplete",
		rep.Answered, countNoun(rep.TotalIndexers, "indexer"), rep.Failed, rep.Skipped)
}

// ── plumbing ────────────────────────────────────────────────────────────────

// resolveIndexerInstance picks the instance to search: the one named by
// ?instance=, or the single enabled indexer-role instance if there is one.
func (s *Server) resolveIndexerInstance(r *http.Request, a authSession) (int64, error) {
	scope := storeScope(a)

	if raw := strings.TrimSpace(r.URL.Query().Get("instance")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, errStatus(http.StatusBadRequest, CodeBadRequest, "instance must be a service id")
		}
		si, err := s.store.GetServiceInstance(r.Context(), scope, id)
		if err != nil {
			return 0, notFoundOr(err, "service")
		}
		if si.Role != "indexer" {
			return 0, errStatus(http.StatusBadRequest, CodeBadRequest,
				fmt.Sprintf("%q is a %s service, not an indexer", si.Name, si.Role))
		}
		// A named-but-disabled instance is a mistake, and the honest answer is to
		// say so. Searching it anyway would ignore the user's own configuration;
		// falling back to the auto-picked instance would silently answer a
		// different question from the one asked, which is worse than an error
		// because the results look right. Distinct code so the client can offer
		// "enable it" rather than the generic "add Prowlarr".
		if !si.Enabled {
			return 0, errStatus(http.StatusConflict, CodeServiceDisabled,
				fmt.Sprintf("%q is disabled, so it cannot be searched", si.Name)).
				withAction("Enable this service")
		}
		return si.ID, nil
	}

	instances, err := s.store.ListServiceInstances(r.Context(), scope)
	if err != nil {
		return 0, errStatus(http.StatusInternalServerError, CodeInternal,
			"the configured services could not be read").wrapping(err)
	}
	var candidates []store.ServiceInstance
	for _, si := range instances {
		if si.Role == "indexer" && si.Enabled {
			candidates = append(candidates, si)
		}
	}
	switch len(candidates) {
	case 0:
		return 0, errStatus(http.StatusConflict, CodeNoIndexerService,
			"no enabled indexer service is configured, so there is nothing to search").
			withAction("Add Prowlarr")
	default:
		// ListServiceInstances orders by priority then name, so this is the
		// user's own stated preference rather than an arbitrary pick.
		return candidates[0].ID, nil
	}
}

func (s *Server) searcherFor(ctx context.Context, instanceID int64) (ReleaseSearcher, error) {
	if s.cfg.Releases == nil {
		return nil, errStatus(http.StatusNotImplemented, CodeNotConfigured,
			"this build has no release-search service wired in")
	}
	searcher, err := s.cfg.Releases.For(ctx, instanceID)
	if err != nil {
		if errors.Is(err, ErrNoIndexerService) {
			return nil, errStatus(http.StatusConflict, CodeNoIndexerService,
				"no enabled indexer service is configured, so there is nothing to search").
				withAction("Add Prowlarr")
		}
		return nil, errStatus(http.StatusBadGateway, CodeServiceUnavailable,
			"that indexer service could not be opened: "+redactText(err.Error())).
			withAction("Test connection").wrapping(err)
	}
	return searcher, nil
}

func searchStartError(err error) error {
	switch {
	case errors.Is(err, releases.ErrInvalidQuery):
		return errStatus(http.StatusBadRequest, CodeBadRequest, redactText(err.Error()))
	case errors.Is(err, releases.ErrNoIndexers):
		return errStatus(http.StatusConflict, CodeNoIndexers,
			"this Prowlarr has no enabled indexer that could serve that query").
			withAction("Enable an indexer in Prowlarr")
	case errors.Is(err, releases.ErrForbidden):
		return errStatus(http.StatusNotFound, CodeNotFound, "no such service")
	}
	return errStatus(http.StatusBadGateway, CodeSearchFailed,
		"the search could not be started: "+redactText(err.Error())).
		withAction("Test connection").wrapping(err)
}

func newSearchID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// setSearchType assigns releases.Query.Type without naming its type.
//
// The field is a servarr.SearchType, and this package may not import
// internal/servarr (doc.go). Untyped string constants convert implicitly on
// assignment, so an explicit switch over the five values Prowlarr recognises
// does the job — and it has to be an explicit switch anyway: an unrecognised
// `type` does NOT error server-side, Prowlarr silently degrades to a basic
// search, which looks like "the id filter did nothing" and is very hard to
// diagnose from the outside.
func setSearchType(q *releases.Query, v string) error {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "search", "basic":
		q.Type = "search"
	case "tvsearch", "tv":
		q.Type = "tvsearch"
	case "movie":
		q.Type = "movie"
	case "music":
		q.Type = "music"
	case "book":
		q.Type = "book"
	default:
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("type=%q is not one of search, tvsearch, movie, music, book", v))
	}
	return nil
}

// setCriteria fills the typed search criteria from query parameters.
//
// Prowlarr does not accept these as query parameters at all — SearchController
// extracts them from the `query` string with a regex over {Key:Value} tokens.
// internal/servarr builds those tokens; this only has to name the fields, which
// it can do without naming the struct's type.
func setCriteria(q *releases.Query, r *http.Request) {
	v := r.URL.Query()
	get := func(name string) string { return strings.TrimSpace(v.Get(name)) }

	q.Criteria.IMDbID = get("imdb_id")
	q.Criteria.TMDbID = get("tmdb_id")
	q.Criteria.TraktID = get("trakt_id")
	q.Criteria.DoubanID = get("douban_id")
	q.Criteria.TVDbID = get("tvdb_id")
	q.Criteria.TVMazeID = get("tvmaze_id")
	q.Criteria.RID = get("rid")
	q.Criteria.Season = get("season")
	q.Criteria.Episode = get("episode")
	q.Criteria.Artist = get("artist")
	q.Criteria.Album = get("album")
	q.Criteria.Track = get("track")
	q.Criteria.Label = get("label")
	q.Criteria.Author = get("author")
	q.Criteria.Publisher = get("publisher")
	q.Criteria.Title = get("book_title")
	q.Criteria.Year = get("year")
	q.Criteria.Genre = get("genre")
}

func queryInt32(r *http.Request, name string) (int32, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 {
		return 0, errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("%s=%q is not a non-negative integer", name, raw))
	}
	return int32(n), nil
}
