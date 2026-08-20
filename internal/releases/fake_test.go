package releases

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
)

// fakeStore is an in-memory CandidateStore/ProvenanceStore.
//
// internal/store's exact signatures are not settled yet, which is why this
// package depends on the narrow interfaces in store.go rather than on a concrete
// type. This fake is the payoff: the flow is testable with no database and no
// migrations.
type fakeStore struct {
	mu           sync.Mutex
	nextID       int64
	candidates   map[int64]Candidate
	provenance   []Provenance
	insertErr    error
	candidateErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{candidates: map[int64]Candidate{}}
}

func (f *fakeStore) InsertCandidates(_ context.Context, cands []Candidate) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return nil, f.insertErr
	}
	ids := make([]int64, 0, len(cands))
	for _, c := range cands {
		f.nextID++
		c.ID = f.nextID
		f.candidates[c.ID] = c
		ids = append(ids, c.ID)
	}
	return ids, nil
}

func (f *fakeStore) Candidate(_ context.Context, scope Scope, id int64) (Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.candidateErr != nil {
		return Candidate{}, f.candidateErr
	}
	c, ok := f.candidates[id]
	if !ok {
		return Candidate{}, ErrCandidateNotFound
	}
	// The real store enforces the scope in the SQL. Mirroring it here keeps the
	// fake honest about the contract.
	if !scope.Allows(c.ServiceInstanceID) {
		return Candidate{}, ErrCandidateNotFound
	}
	return c, nil
}

func (f *fakeStore) DeleteExpiredCandidates(_ context.Context, t time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for id, c := range f.candidates {
		if c.ExpiresAt.Before(t) {
			delete(f.candidates, id)
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) InsertProvenance(_ context.Context, p Provenance) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	p.ID = f.nextID
	f.provenance = append(f.provenance, p)
	return p.ID, nil
}

func (f *fakeStore) put(c Candidate) int64 {
	ids, _ := f.InsertCandidates(context.Background(), []Candidate{c})
	return ids[0]
}

// searchCall records one per-indexer leg AS ENCODED, so the fan-out is asserted
// on what actually went on the wire rather than on what the SearchRequest struct
// meant to say.
//
// Recording req.IndexerIDs and friends straight off the struct is the exact
// shape of the bug that broke the first real grab: the field can be perfectly
// right while Values() drops it, comma-joins it, or spells the parameter name
// differently from what Prowlarr binds — and a fake that reads the struct would
// pass through every one of those. Everything below is therefore parsed back OUT
// of url.Values, and values is kept raw so a test can assert on the repeated
// form directly.
type searchCall struct {
	values url.Values

	indexerIDs []int32
	categories []int32
	query      string
	limit      *int32
	offset     *int32
}

// newSearchCall decodes an encoded query back into assertable fields.
//
// A value that does not parse as a bare integer — a comma-joined "1,2", say — is
// DROPPED rather than tolerated, so an encoding regression shows up as a failed
// assertion instead of quietly satisfying one.
func newSearchCall(v url.Values) searchCall {
	return searchCall{
		values:     v,
		indexerIDs: parseInt32s(v["indexerIds"]),
		categories: parseInt32s(v["categories"]),
		query:      v.Get("query"),
		limit:      parseInt32Ptr(v, "limit"),
		offset:     parseInt32Ptr(v, "offset"),
	}
}

func parseInt32s(raw []string) []int32 {
	var out []int32
	for _, s := range raw {
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			continue
		}
		out = append(out, int32(n))
	}
	return out
}

func parseInt32Ptr(v url.Values, key string) *int32 {
	if !v.Has(key) {
		return nil
	}
	n, err := strconv.ParseInt(v.Get(key), 10, 32)
	if err != nil {
		return nil
	}
	x := int32(n)
	return &x
}

// fakeClient is a scripted IndexerClient.
type fakeClient struct {
	mu sync.Mutex

	indexers    []servarr.IndexerResource
	indexersErr error
	statuses    []servarr.IndexerStatusResource
	statusErr   error
	clients     []servarr.DownloadClientResource
	clientsErr  error

	// searchByIndexer scripts the per-indexer response. A delay makes a leg
	// exceed its deadline.
	searchByIndexer map[int32][]servarr.ReleaseResource
	searchErrs      map[int32]error
	searchDelays    map[int32]time.Duration

	// searchBudgets records, per indexer, how much context budget the leg was
	// handed when it was called. That is a fact about the deadline the caller
	// installed, not a measurement of how fast this box is, which is what lets
	// the per-indexer-deadline test assert without a stopwatch.
	searchBudgets map[int32]time.Duration

	// grabResponses is consumed in order, so a 404 can be followed by a success.
	grabResponses []error

	searchCalls []searchCall
	grabCalls   int

	// grabbed records every resource handed to Grab, so a test can assert on what
	// actually crossed the client boundary rather than on a call count.
	grabbed []servarr.ReleaseResource
}

func (f *fakeClient) Indexers(context.Context) ([]servarr.IndexerResource, error) {
	return f.indexers, f.indexersErr
}

func (f *fakeClient) IndexerStatus(context.Context) ([]servarr.IndexerStatusResource, error) {
	return f.statuses, f.statusErr
}

func (f *fakeClient) DownloadClients(context.Context) ([]servarr.DownloadClientResource, error) {
	return f.clients, f.clientsErr
}

func (f *fakeClient) Search(ctx context.Context, req servarr.SearchRequest) ([]servarr.ReleaseResource, error) {
	v, err := req.Values()
	if err != nil {
		return nil, err
	}
	call := newSearchCall(v)
	f.mu.Lock()
	f.searchCalls = append(f.searchCalls, call)
	f.mu.Unlock()

	// Script off the ENCODED id too: if the fan-out stopped putting the indexer
	// on the URL, the scripted response must go missing rather than arrive
	// anyway on the strength of the struct field.
	var id int32
	if len(call.indexerIDs) == 1 {
		id = call.indexerIDs[0]
	}
	if dl, ok := ctx.Deadline(); ok {
		f.mu.Lock()
		if f.searchBudgets == nil {
			f.searchBudgets = make(map[int32]time.Duration)
		}
		f.searchBudgets[id] = time.Until(dl)
		f.mu.Unlock()
	}

	if d := f.searchDelays[id]; d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return nil, &servarr.APIError{Op: "Search", Err: servarr.ErrTimeout}
		}
	}
	if err, ok := f.searchErrs[id]; ok {
		return nil, err
	}
	return f.searchByIndexer[id], nil
}

// Grab stands in for the client at the Go-interface boundary, so it sees the
// ReleaseResource and NEVER the JSON body — GrabBody and encoding/json both run
// inside the real client, below this seam. It therefore cannot catch a body that
// marshals to something an *Arr rejects, and must not be assumed to: that is
// covered at the boundary where the body is actually built, by
// servarr.TestOutboundBodiesAreSpecLegal and, end to end, by the fake Prowlarr's
// model binding in cmd/usarr.
func (f *fakeClient) Grab(_ context.Context, rel servarr.ReleaseResource) (servarr.ReleaseResource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.grabCalls
	f.grabCalls++
	f.grabbed = append(f.grabbed, rel)
	if i < len(f.grabResponses) && f.grabResponses[i] != nil {
		return servarr.ReleaseResource{}, f.grabResponses[i]
	}
	return rel, nil
}

func (f *fakeClient) calls() []searchCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]searchCall(nil), f.searchCalls...)
}

// searchBudget reports the context budget the leg for id was handed, and whether
// it was handed a deadline at all. A leg with no deadline is a leg nothing
// bounds, so the two cases are distinguished rather than collapsed into zero.
func (f *fakeClient) searchBudget(id int32) (time.Duration, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.searchBudgets[id]
	return d, ok
}

// ---- shared fixtures ------------------------------------------------------

const testInstanceID int64 = 7

var ownerScope = Scope{UserID: 1, InstanceIDs: []int64{testInstanceID}}

func caps(limitsMax int32, cats ...int32) servarr.IndexerCapabilityResource {
	c := servarr.IndexerCapabilityResource{
		LimitsMax:         servarr.Int32(limitsMax),
		SearchParams:      []string{"q"},
		MovieSearchParams: []string{"q", "imdbId"},
		TvSearchParams:    []string{"q", "season", "ep"},
	}
	for _, id := range cats {
		c.Categories = append(c.Categories, servarr.IndexerCategory{ID: id})
	}
	return c
}

func indexer(id int32, name string, priority int32, proto servarr.DownloadProtocol) servarr.IndexerResource {
	return servarr.IndexerResource{
		ID: id, Name: name, Enable: true, SupportsSearch: true, SupportsRSS: true,
		Protocol: proto, Privacy: servarr.PrivacyPrivate, Priority: priority,
		Capabilities: caps(100, 2000, 5000),
	}
}

func release(guid, title string, indexerID int32, indexerName string, proto servarr.DownloadProtocol, cats ...int32) servarr.ReleaseResource {
	dl := "http://prowlarr.test:9696/" + title + "/download?apikey=SECRETKEY&link=x"
	r := servarr.ReleaseResource{
		GUID: guid, Title: title, IndexerID: indexerID, Indexer: indexerName,
		Protocol: proto, Size: 1 << 30, DownloadURL: &dl,
		PublishDate: time.Unix(1_755_000_000, 0).UTC(),
	}
	for _, c := range cats {
		r.Categories = append(r.Categories, servarr.IndexerCategory{ID: c})
	}
	return r
}

// collect drains a search stream.
func collect(ch <-chan Event) (results []Result, outcomes []IndexerOutcome, report *Report) {
	for ev := range ch {
		switch ev.Kind {
		case EventKindResults:
			results = append(results, ev.Results...)
		case EventKindIndexerDone:
			outcomes = append(outcomes, *ev.Indexer)
		case EventKindDone:
			report = ev.Report
		}
	}
	return results, outcomes, report
}

func outcomeFor(outcomes []IndexerOutcome, id int32) (IndexerOutcome, bool) {
	for _, o := range outcomes {
		if o.IndexerID == id {
			return o, true
		}
	}
	return IndexerOutcome{}, false
}

var errBoom = errors.New("boom")
