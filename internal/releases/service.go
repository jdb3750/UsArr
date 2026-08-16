package releases

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
)

// CandidateTTL is how long a Prowlarr-sourced release_candidate stays grabbable.
//
// Prowlarr's grab cache is a non-rolling 30 minutes that dies with the process.
// 25 minutes leaves a five-minute margin for clock skew between the two hosts and
// for the round-trip of the grab itself. An expired candidate is never rendered
// as grabbable (ARCHITECTURE.md §8.4).
const CandidateTTL = 25 * time.Minute

// Defaults for the fan-out. Per-indexer deadlines are single-digit seconds
// deliberately: Prowlarr's own per-indexer HTTP timeout is 15 s with 2 Polly
// retries, so waiting for it means waiting up to 45 s for one dead indexer.
// UsArr cuts its own leg at 8 s and reports the indexer as timed out, which is
// far more useful than a spinner.
const (
	DefaultPerIndexerTimeout = 8 * time.Second
	DefaultOverallTimeout    = 60 * time.Second
	DefaultConcurrency       = 6
	DefaultPerIndexerLimit   = 100
)

// Config configures a Service. One Service per Prowlarr service_instance.
type Config struct {
	// InstanceID is the service_instance row this service fronts.
	InstanceID int64

	// Client is the Prowlarr client. Required.
	Client IndexerClient

	// Store persists release candidates and provenance. Required.
	Store Store

	PerIndexerTimeout time.Duration
	OverallTimeout    time.Duration
	Concurrency       int
	PerIndexerLimit   int32

	// Breaker configures the PER-INDEXER breakers this service keeps. They are
	// separate from the client's per-instance breaker: one dead indexer must not
	// stop the other seven from being searched, which is the same reasoning as
	// "Radarr being down must not stop Sonarr syncing" one level down.
	Breaker servarr.BreakerConfig

	Now    func() time.Time
	Logger *slog.Logger
}

// Service is the Search-and-Grab flow over one Prowlarr instance.
type Service struct {
	cfg Config
	now func() time.Time
	log *slog.Logger

	mu       sync.Mutex
	breakers map[int32]*servarr.Breaker
}

// NewService builds a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Client == nil {
		return nil, ErrNotConfigured
	}
	if cfg.Store == nil {
		return nil, ErrNotConfigured
	}
	if cfg.PerIndexerTimeout <= 0 {
		cfg.PerIndexerTimeout = DefaultPerIndexerTimeout
	}
	if cfg.OverallTimeout <= 0 {
		cfg.OverallTimeout = DefaultOverallTimeout
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = DefaultConcurrency
	}
	if cfg.PerIndexerLimit <= 0 {
		cfg.PerIndexerLimit = DefaultPerIndexerLimit
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		cfg:      cfg,
		now:      now,
		log:      log,
		breakers: map[int32]*servarr.Breaker{},
	}, nil
}

// InstanceID reports which service_instance this Service fronts.
func (s *Service) InstanceID() int64 { return s.cfg.InstanceID }

// breakerFor returns the per-indexer breaker, creating it on first use.
func (s *Service) breakerFor(indexerID int32) *servarr.Breaker {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.breakers[indexerID]
	if !ok {
		b = servarr.NewBreaker(s.cfg.Breaker, s.now, nil)
		s.breakers[indexerID] = b
	}
	return b
}

// EvictExpired removes release candidates whose grab window has closed. Run it
// from the maintenance worker; nothing on a request path needs it, because Grab
// checks expiry itself.
func (s *Service) EvictExpired(ctx context.Context) (int64, error) {
	return s.cfg.Store.DeleteExpiredCandidates(ctx, s.now())
}
