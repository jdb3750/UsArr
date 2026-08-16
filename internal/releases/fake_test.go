package releases

import (
	"context"
	"errors"
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

// searchCall records one per-indexer leg so the fan-out can be asserted on.
type searchCall struct {
	indexerIDs []int32
	query      string
	limit      *int32
	offset     *int32
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

	// grabResponses is consumed in order, so a 404 can be followed by a success.
	grabResponses []error

	searchCalls []searchCall
	grabCalls   int
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
	f.mu.Lock()
	f.searchCalls = append(f.searchCalls, searchCall{
		indexerIDs: req.IndexerIDs, query: v.Get("query"), limit: req.Limit, offset: req.Offset,
	})
	f.mu.Unlock()

	var id int32
	if len(req.IndexerIDs) == 1 {
		id = req.IndexerIDs[0]
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

func (f *fakeClient) Grab(_ context.Context, rel servarr.ReleaseResource) (servarr.ReleaseResource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.grabCalls
	f.grabCalls++
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
