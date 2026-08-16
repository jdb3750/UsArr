package releases

import (
	"context"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
)

// Scope is the caller's access scope.
//
// Every read that aggregates across instances takes one, in the signature, from
// the first commit — multi-user is in the schema from migration 0001 and the UI
// merely hides what has not shipped (CLAUDE.md principle 4). In v0.1 there is one
// scope, the owner's, containing every instance; that is the degenerate case of
// the general rule, not the design.
//
// This is a local struct rather than an import so that this package does not block
// on whatever internal/auth eventually names its scope type. If that type lands
// with a different shape, adapt at the call sites — the invariant that matters is
// that the parameter exists.
type Scope struct {
	// UserID is the acting user.
	UserID int64

	// AllInstances grants visibility of every service instance. True for the
	// owner, which in v0.1 is everyone.
	AllInstances bool

	// InstanceIDs is the visible set when AllInstances is false. An empty slice
	// with AllInstances false means the caller can see nothing, and every check
	// below refuses — failing closed is the whole point of carrying the parameter.
	InstanceIDs []int64
}

// Allows reports whether the scope covers a service instance.
func (s Scope) Allows(instanceID int64) bool {
	if s.AllInstances {
		return true
	}
	for _, id := range s.InstanceIDs {
		if id == instanceID {
			return true
		}
	}
	return false
}

// Candidate mirrors the release_candidate row (schema.md §6). It is ephemeral and
// TTL-evicted.
type Candidate struct {
	ID                int64
	WorkID            *int64 // NULL in Search-and-Grab mode: there is no library
	ServiceInstanceID int64

	GUID       string
	Title      string // the raw scene/P2P name, verbatim
	Indexer    string
	IndexerID  int32
	Protocol   string
	Categories []int32 // raw ids; persisted as a JSON array, never collapsed

	SizeBytes int64
	Seeders   *int32
	Leechers  *int32
	AgeDays   float64

	InfoURL          string
	InfoHash         string
	DownloadClientID *int32

	// RawReleaseJSON is the ReleaseResource as returned by Prowlarr, put through
	// servarr.SanitizeRelease first. That drops downloadUrl and magnetUrl, and
	// dropping them is what keeps this column free of a credential.
	//
	// Why it is safe to drop them, and why the grab still works.
	// SearchController.MapReleases rewrites both fields into
	// `{serverUrl}{urlBase}/{indexerId}/download?apikey={ApiKey}&link=…`, so
	// unsanitised they carry Prowlarr's FULL ADMIN API KEY in plaintext on every
	// result — into this DB file, into every VACUUM INTO backup, and into every
	// restored copy of the volume, forever. The grab does not need them: Grab
	// POSTs rel.GrabBody(), which is guid + indexerId + downloadClientId (see
	// ReleaseResource.GrabBody — sending the whole resource "buys nothing and
	// would echo the embedded API key back over the wire"), and Prowlarr resolves
	// the release from its own cache keyed "{indexerId}_{guid}". No production
	// path anywhere reads DownloadURL or MagnetURL off a decoded stored release.
	//
	// This column is still server-side only: it must never be selected into an API
	// response, an SSE frame, a log line, an error message or a support bundle.
	// The only code that reads it is Grab, and Result deliberately has no field it
	// could be assigned to. That is defence in depth now rather than the sole
	// control, which is the point of sanitising on the way in.
	RawReleaseJSON []byte

	Rejected         bool
	RejectionReasons []string

	FetchedAt time.Time
	// ExpiresAt is FetchedAt + 25 minutes for Prowlarr-sourced candidates, against
	// a 30-minute upstream cache. An expired candidate is never rendered grabbable.
	ExpiresAt time.Time
}

// Provenance mirrors the provenance row (schema.md §6). It is immutable, one row
// per acquisition event, and is never overwritten on upgrade — a new row is
// inserted instead, which gives upgrade history for free.
type Provenance struct {
	ID       int64
	Protocol string // usenet|torrent|irc|direct|manual|unknown

	IndexerName       string
	IndexerID         int32
	IndexerPrivacy    string
	IndexerCategories []int32
	IndexerFlags      []string

	// DownloadID is the join key (nzo_id / torrent infohash). Prowlarr's grab
	// returns NO download id and no queue id — the 200 is the whole confirmation —
	// so for a torrent this is the infohash and for usenet it is empty until a
	// library-bearing service later imports the file and supplies one.
	DownloadID      string
	TorrentInfoHash string

	NZBInfoURL  string
	ReleaseGUID string

	// ReleaseTitle is the raw scene/P2P name, stored VERBATIM, FOREVER. Every
	// parsed field is re-derivable from it; it is not re-derivable from them.
	ReleaseTitle string

	PublishedAt *time.Time
	GrabbedAt   time.Time

	SourceSystem   string // "prowlarr"
	SourceRecordID string
	Confidence     float64

	// DownloadURL is deliberately absent from this struct even though the column
	// exists: a Prowlarr download URL is a proxy link carrying the admin API key,
	// and persisting it would put that key in every VACUUM INTO backup.
}

// CandidateStore is the narrow slice of internal/store this package needs.
//
// It is an interface defined here, by the consumer, rather than an import of a
// concrete store type — so the exact signatures on the store side can settle
// independently, and so tests need no database.
type CandidateStore interface {
	// InsertCandidates persists candidates and returns their assigned ids in the
	// same order. Callers persist as results arrive, so batches are small.
	InsertCandidates(ctx context.Context, cands []Candidate) ([]int64, error)

	// Candidate loads one candidate, enforcing the access scope in the query
	// itself. It returns ErrCandidateNotFound both when the row does not exist and
	// when it is out of scope.
	Candidate(ctx context.Context, scope Scope, id int64) (Candidate, error)

	// DeleteExpiredCandidates evicts rows whose expires_at is before t.
	DeleteExpiredCandidates(ctx context.Context, t time.Time) (int64, error)
}

// ProvenanceStore is the narrow slice needed to record a completed grab.
type ProvenanceStore interface {
	InsertProvenance(ctx context.Context, p Provenance) (int64, error)
}

// Store is what Service needs from persistence.
type Store interface {
	CandidateStore
	ProvenanceStore
}

// IndexerClient is the narrow slice of *servarr.Client this package needs.
//
// Depending on the interface rather than the concrete type keeps the fake in the
// tests trivial, and is what lets the fan-out give each indexer its own client
// wrapper later without touching this package.
type IndexerClient interface {
	Indexers(ctx context.Context) ([]servarr.IndexerResource, error)
	IndexerStatus(ctx context.Context) ([]servarr.IndexerStatusResource, error)
	DownloadClients(ctx context.Context) ([]servarr.DownloadClientResource, error)
	Search(ctx context.Context, req servarr.SearchRequest) ([]servarr.ReleaseResource, error)
	Grab(ctx context.Context, rel servarr.ReleaseResource) (servarr.ReleaseResource, error)
}
