package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ReleaseCandidate is one grabbable release from an indexer search.
//
// These are ephemeral by design. Prowlarr caches grabbable releases server-side
// for 30 minutes, so a candidate persisted and grabbed later fails — ExpiresAt
// is capped below that (<= 25 min) so UsArr notices first and can say why.
type ReleaseCandidate struct {
	ID int64

	// UserID is the user whose search produced this candidate. NOT NULL from
	// migration 0002, defaulting to the shared/system sentinel 0. It carries
	// REFERENCES user(id) ON DELETE CASCADE: these rows are ephemeral
	// operational state, so a deleted user's pending results go with them.
	UserID int64

	WorkID            sql.NullInt64 // NULL in Search-and-Grab: nothing owns the result yet
	ServiceInstanceID int64
	GUID              string
	Title             string
	Indexer           string
	IndexerID         sql.NullInt64
	Protocol          string
	Categories        string // JSON array of raw Newznab category ints
	SizeBytes         sql.NullInt64
	Seeders           sql.NullInt64
	Leechers          sql.NullInt64
	AgeDays           sql.NullFloat64
	Quality           string
	DownloadURL       string
	InfoURL           string
	InfoHash          string
	DownloadClientID  sql.NullInt64

	// RawReleaseJSON is the full upstream ReleaseResource. It is needed
	// verbatim for the grab: the grab POSTs the release back, so any field
	// dropped on the way in cannot be reconstructed on the way out.
	RawReleaseJSON string

	Rejected         bool
	RejectionReasons string
	FetchedAt        time.Time
	ExpiresAt        time.Time
}

// InsertReleaseCandidates writes a search's results in one transaction.
//
// One transaction, not one per row: a Prowlarr search returns hundreds of
// releases and the process has a single writer connection, so per-row commits
// would hold it for the whole batch.
func (s *Store) InsertReleaseCandidates(ctx context.Context, rcs []ReleaseCandidate) ([]int64, error) {
	if len(rcs) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(rcs))
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO release_candidate (
			  user_id, work_id, service_instance_id, guid, title, indexer, indexer_id, protocol,
			  categories, size_bytes, seeders, leechers, age_days, quality,
			  download_url, info_url, info_hash, download_client_id,
			  raw_release_json, rejected, rejection_reasons, fetched_at, expires_at
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return fmt.Errorf("insert release_candidate: prepare: %w", err)
		}
		defer func() { _ = stmt.Close() }()

		for i := range rcs {
			rc := &rcs[i]
			res, err := stmt.ExecContext(ctx,
				rc.UserID, rc.WorkID, rc.ServiceInstanceID, rc.GUID, rc.Title, nullString(rc.Indexer),
				rc.IndexerID, nullString(rc.Protocol), nullString(rc.Categories),
				rc.SizeBytes, rc.Seeders, rc.Leechers, rc.AgeDays, nullString(rc.Quality),
				nullString(rc.DownloadURL), nullString(rc.InfoURL), nullString(rc.InfoHash),
				rc.DownloadClientID, rc.RawReleaseJSON, rc.Rejected,
				nullString(rc.RejectionReasons), FormatTime(rc.FetchedAt), FormatTime(rc.ExpiresAt),
			)
			if err != nil {
				return fmt.Errorf("insert release_candidate %q: %w", rc.GUID, err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("insert release_candidate %q: last insert id: %w", rc.GUID, err)
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// GetReleaseCandidate reads one candidate within scope.
//
// BOTH halves of the scope are applied, and both are load-bearing. The instance
// predicate stops a caller reading a candidate from a service it cannot see; the
// user predicate stops it reading another user's search results, which are a
// record of what that user was looking for. Migration 0002 added the user_id
// column that makes the second one possible — before it, this function took a
// scope and could only honour half of it.
//
// now is passed rather than read from the clock so the caller decides what
// "expired" means for this request, and so the expiry branch is testable
// without sleeping. An expired candidate is reported as expired, not as
// missing: "this release listing went stale, search again" is a different
// message from "no such release".
func (s *Store) GetReleaseCandidate(ctx context.Context, scope Scope, id int64, now time.Time) (ReleaseCandidate, error) {
	instPred, instArgs := scope.instancePredicate("service_instance_id")
	userPred, userArgs := scope.userPredicate("user_id")
	query := `
		SELECT id, user_id, work_id, service_instance_id, guid, title, indexer, indexer_id, protocol,
		       categories, size_bytes, seeders, leechers, age_days, quality,
		       download_url, info_url, info_hash, download_client_id,
		       raw_release_json, rejected, rejection_reasons, fetched_at, expires_at
		  FROM release_candidate
		 WHERE id = ? AND ` + instPred + ` AND ` + userPred

	args := make([]any, 0, 1+len(instArgs)+len(userArgs))
	args = append(args, id)
	args = append(args, instArgs...)
	args = append(args, userArgs...)

	var rc ReleaseCandidate
	var indexer, protocol, categories, quality sql.NullString
	var downloadURL, infoURL, infoHash, rejectionReasons sql.NullString
	var fetchedAt, expiresAt string

	err := s.db.Read().QueryRowContext(ctx, query, args...).Scan(
		&rc.ID, &rc.UserID, &rc.WorkID, &rc.ServiceInstanceID, &rc.GUID, &rc.Title, &indexer, &rc.IndexerID,
		&protocol, &categories, &rc.SizeBytes, &rc.Seeders, &rc.Leechers, &rc.AgeDays, &quality,
		&downloadURL, &infoURL, &infoHash, &rc.DownloadClientID,
		&rc.RawReleaseJSON, &rc.Rejected, &rejectionReasons, &fetchedAt, &expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ReleaseCandidate{}, fmt.Errorf("release_candidate %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return ReleaseCandidate{}, fmt.Errorf("read release_candidate %d: %w", id, err)
	}

	rc.Indexer, rc.Protocol, rc.Categories, rc.Quality = indexer.String, protocol.String, categories.String, quality.String
	rc.DownloadURL, rc.InfoURL, rc.InfoHash = downloadURL.String, infoURL.String, infoHash.String
	rc.RejectionReasons = rejectionReasons.String
	if rc.FetchedAt, err = ParseTime(fetchedAt); err != nil {
		return ReleaseCandidate{}, err
	}
	if rc.ExpiresAt, err = ParseTime(expiresAt); err != nil {
		return ReleaseCandidate{}, err
	}
	if !now.IsZero() && !now.Before(rc.ExpiresAt) {
		return rc, fmt.Errorf("release_candidate %d expired at %s: %w", id, expiresAt, ErrExpired)
	}
	return rc, nil
}

// ErrExpired means the release listing went stale before it was grabbed.
// Prowlarr drops grabbable releases from its own cache after 30 minutes, so
// this is a normal outcome, not a fault.
var ErrExpired = errors.New("store: release candidate expired")

// ExpireReleaseCandidates deletes candidates whose expires_at has passed and
// returns how many went. Driven by ix_rel_expiry.
func (s *Store) ExpireReleaseCandidates(ctx context.Context, now time.Time) (int64, error) {
	var n int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM release_candidate WHERE expires_at <= ?`, FormatTime(now))
		if err != nil {
			return fmt.Errorf("expire release_candidate: %w", err)
		}
		n, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("expire release_candidate: rows affected: %w", err)
		}
		return nil
	})
	return n, err
}

// Provenance is one immutable acquisition event.
//
// Never overwrite provenance on upgrade — insert a new row and link the new
// media_file, which gives upgrade history for free. ReleaseTitle is the raw
// scene/P2P name and is stored VERBATIM, FOREVER: every parsed field is
// re-derivable if the parser improves, and the raw name is not recoverable once
// discarded.
type Provenance struct {
	ID int64

	// UserID is who acquired this. NOT NULL from migration 0002, defaulting to
	// the shared/system sentinel 0, and carrying NO foreign key — deliberately,
	// exactly like audit_log.actor_user_id. It is a HISTORICAL id: the user may
	// since have been deleted, and the whole point of the row is that it still
	// records who did it. Migration 0002's header has the full argument.
	UserID int64

	Protocol           string // usenet|torrent|irc|direct|manual|unknown
	IndexerName        string
	IndexerID          sql.NullInt64
	IndexerPrivacy     string
	IndexerCategories  string // JSON array of raw Newznab cat ints — DO NOT collapse
	IndexerFlags       string // JSON array
	DownloadClientType string
	DownloadClientName string

	// DownloadID is THE join key between a grab event and the import event
	// that follows it: nzo_id for usenet, the infohash for torrents. The
	// import event carries no indexer, protocol or guid, so this is the only
	// way to reattach provenance to the imported file.
	DownloadID      string
	TorrentInfoHash string

	NZBInfoURL  string
	DownloadURL string
	ReleaseGUID string

	ReleaseTitle      string
	ReleaseGroup      string
	QualitySource     string
	QualityResolution string
	VideoCodec        string
	AudioCodec        string
	AudioChannels     string
	EditionLabel      string
	Languages         string
	ProperRepack      sql.NullInt64

	// SizeBytes is the release size as the indexer reported it, added by
	// migration 0002. Nullable: not every indexer reports one, and 0 would be a
	// lie rather than an absence. release_candidate has always carried it, so
	// without this column the size of an acquisition was lost the moment its
	// candidate expired 25 minutes later.
	SizeBytes sql.NullInt64

	PublishedAt sql.NullString
	GrabbedAt   sql.NullString
	ImportedAt  sql.NullString

	SourceSystem   string // sonarr|radarr|prowlarr|manual|filesystem
	SourceRecordID string
	Confidence     float64

	// AcquisitionState says whether UsArr ever confirmed this acquisition:
	// AcquisitionConfirmed or AcquisitionUnconfirmed. Empty means confirmed on
	// write, which is what every caller before migration 0003 meant.
	//
	// It is a column and not a lookup through audit_log because provenance has
	// NO back-reference to the audit chain — audit_log.target_id holds the
	// release-candidate id, and that candidate is swept 25 minutes later — so a
	// reader that starts from provenance, which is what the import join and the
	// recent-grabs block both do, has nothing else to ask.
	AcquisitionState string
}

// Acquisition states for Provenance.AcquisitionState.
//
// Deliberately NOT a CHECK constraint: SQLite cannot ALTER one, and migration
// 0001's audit_log foreign key is what that costs when the vocabulary later
// has to grow (v0.2's request path may want a "pending"). This is the same
// arrangement AuditEntry.Result uses — TEXT NOT NULL in the schema, vocabulary
// owned by Go — except that here InsertProvenance also enforces it, because an
// unrecognised state would silently read as "not confirmed" and put a perfectly
// good acquisition in the attention block forever.
const (
	// AcquisitionConfirmed means Prowlarr answered 2xx. That is the only
	// confirmation its grab API offers: there is no download id and no queue id
	// to poll (docs/reference/search.md).
	AcquisitionConfirmed = "confirmed"

	// AcquisitionUnconfirmed means the grab was DISPATCHED and its outcome is
	// unknown — a 5xx or a timeout after the POST left the process. Prowlarr
	// adds the release to the download client before it configures it and never
	// rolls back, so the download may well be running. The row exists only to
	// keep download_id, the join key that reattaches this grab to the file an
	// importer later produces; it must never be counted as an acquisition.
	AcquisitionUnconfirmed = "unconfirmed"
)

// InsertProvenance appends one acquisition event.
func (s *Store) InsertProvenance(ctx context.Context, p Provenance) (int64, error) {
	if p.Confidence == 0 {
		p.Confidence = 1.0
	}
	switch p.AcquisitionState {
	case "":
		p.AcquisitionState = AcquisitionConfirmed
	case AcquisitionConfirmed, AcquisitionUnconfirmed:
	default:
		return 0, fmt.Errorf("insert provenance %q: unknown acquisition_state %q",
			p.ReleaseTitle, p.AcquisitionState)
	}
	var id int64
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO provenance (
			  user_id, size_bytes,
			  protocol, indexer_name, indexer_id, indexer_privacy, indexer_categories,
			  indexer_flags, download_client_type, download_client_name, download_id,
			  torrent_info_hash, nzb_info_url, download_url, release_guid,
			  release_title, release_group, quality_source, quality_resolution,
			  video_codec, audio_codec, audio_channels, edition_label, languages,
			  proper_repack, published_at, grabbed_at, imported_at,
			  source_system, source_record_id, confidence, acquisition_state
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.UserID, p.SizeBytes,
			p.Protocol, nullString(p.IndexerName), p.IndexerID, nullString(p.IndexerPrivacy),
			nullString(p.IndexerCategories), nullString(p.IndexerFlags),
			nullString(p.DownloadClientType), nullString(p.DownloadClientName),
			nullString(p.DownloadID), nullString(p.TorrentInfoHash), nullString(p.NZBInfoURL),
			nullString(p.DownloadURL), nullString(p.ReleaseGUID),
			p.ReleaseTitle, nullString(p.ReleaseGroup), nullString(p.QualitySource),
			nullString(p.QualityResolution), nullString(p.VideoCodec), nullString(p.AudioCodec),
			nullString(p.AudioChannels), nullString(p.EditionLabel), nullString(p.Languages),
			p.ProperRepack, p.PublishedAt, p.GrabbedAt, p.ImportedAt,
			p.SourceSystem, nullString(p.SourceRecordID), p.Confidence, p.AcquisitionState,
		)
		if err != nil {
			return fmt.Errorf("insert provenance %q: %w", p.ReleaseTitle, err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("insert provenance %q: last insert id: %w", p.ReleaseTitle, err)
		}
		return nil
	})
	return id, err
}

// GetProvenanceByDownloadID reads acquisition events by download id, which is
// how an import event is reattached to the grab that produced it.
//
// It returns AcquisitionState and every caller must look at it. This is the one
// read that a row written for an UNCONFIRMED grab exists to serve — the whole
// reason such a row is written is that download_id would otherwise be lost — and
// a caller that joins on the key without checking the state attaches confirmed
// history to something UsArr never confirmed. That is a worse outcome than the
// missing row, so the column travels with the key.
//
// It takes the access scope IN THE SIGNATURE and applies it IN THE SQL. Both
// halves matter: this used to take no scope at all, so any caller holding a
// download id could read any user's acquisition history, and a download id for a
// torrent is the infohash — a value anyone can compute from the torrent. That
// makes an unscoped read here an existence oracle for "did somebody on this box
// grab this release", which is exactly the class ARCHITECTURE.md §1.3 rules out.
func (s *Store) GetProvenanceByDownloadID(ctx context.Context, scope Scope, downloadID string) ([]Provenance, error) {
	userPred, userArgs := scope.userPredicate("user_id")
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT id, user_id, protocol, indexer_name, download_id, release_title,
		       size_bytes, source_system, confidence, acquisition_state
		  FROM provenance
		 WHERE download_id = ? AND `+userPred+`
		 ORDER BY id ASC`, append([]any{downloadID}, userArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("read provenance by download_id %q: %w", downloadID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Provenance
	for rows.Next() {
		var p Provenance
		var indexerName sql.NullString
		var dlID sql.NullString
		if err := rows.Scan(&p.ID, &p.UserID, &p.Protocol, &indexerName, &dlID,
			&p.ReleaseTitle, &p.SizeBytes, &p.SourceSystem, &p.Confidence,
			&p.AcquisitionState); err != nil {
			return nil, fmt.Errorf("read provenance by download_id %q: scan: %w", downloadID, err)
		}
		p.IndexerName, p.DownloadID = indexerName.String, dlID.String
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read provenance by download_id %q: %w", downloadID, err)
	}
	return out, nil
}

// Bounds on ListRecentProvenance.
//
// The default is ten because that is what ARCHITECTURE.md §17.5 specifies the
// Recent-grabs block renders. The cap exists because the scoped predicate is
// `user_id IN (0, :uid)`, and SQLite cannot supply ORDER BY from an index whose
// LEADING column is constrained by IN — internal/db's
// TestScopedProvenanceOrderNeedsASort pins that. The sort is therefore over the
// readable rows with the LIMIT applied, so the LIMIT is the only thing bounding
// the work, and an unbounded one from a query parameter would turn a render read
// into a full sort of a table that grows forever.
const (
	RecentProvenanceDefaultLimit = 10
	RecentProvenanceMaxLimit     = 200
)

// recentProvenanceSQL renders the ListRecentProvenance statement and its
// arguments, scope predicate included.
//
// It is a function rather than a string literal inlined below so the query-plan
// test can EXPLAIN THE STATEMENT THAT SHIPS. docs/DEVELOPMENT.md §11 rule 1: a
// guard that asserts against a hand-copied lookalike is probing a proxy, and a
// proxy agrees with its condition right up until the day it matters.
//
// The limit is clamped HERE rather than in the caller, for the same reason: the
// bound and the statement it bounds must be impossible to separate, so a second
// caller cannot render this query with an unbounded LIMIT.
func recentProvenanceSQL(scope Scope, limit int) (string, []any) {
	switch {
	case limit <= 0:
		limit = RecentProvenanceDefaultLimit
	case limit > RecentProvenanceMaxLimit:
		limit = RecentProvenanceMaxLimit
	}
	userPred, args := scope.userPredicate("user_id")
	return `
		SELECT id, user_id, protocol, indexer_name, indexer_categories, download_id,
		       release_title, size_bytes, grabbed_at, source_system, acquisition_state
		  FROM provenance
		 WHERE ` + userPred + `
		 ORDER BY grabbed_at DESC, id DESC
		 LIMIT ?`, append(args, limit)
}

// ListRecentProvenance reads the newest acquisition events, newest first. It is
// the local read behind ARCHITECTURE.md §17.5's Recent-grabs block.
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL. Both halves, for the same reason
// GetProvenanceByDownloadID carries them: a scope a query accepts and never
// filters on is indistinguishable from no scope at all, and this table is a
// record of what a named person acquired. The predicate also admits the
// shared/system sentinel 0, which is what migration 0002 backfilled every
// pre-attribution row to — reading it as "not mine" would hide the owner's own
// history from them.
//
// AcquisitionState travels on every row and the caller must render it. A row
// here means the grab was DISPATCHED, not that it succeeded: "unconfirmed" is a
// grab Prowlarr never acknowledged, which sits beside "confirmed" (both were
// sent) and NOT beside a failure. Grabs that never left this process write no
// provenance row at all and are not visible from this read.
//
// ORDER BY matches ix_prov_user_grabbed's column order, and id breaks the tie
// because grabbed_at has one-second resolution.
func (s *Store) ListRecentProvenance(ctx context.Context, scope Scope, limit int) ([]Provenance, error) {
	query, args := recentProvenanceSQL(scope, limit)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read recent provenance: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Provenance, 0, RecentProvenanceDefaultLimit)
	for rows.Next() {
		var p Provenance
		var indexerName, categories, dlID sql.NullString
		if err := rows.Scan(&p.ID, &p.UserID, &p.Protocol, &indexerName, &categories, &dlID,
			&p.ReleaseTitle, &p.SizeBytes, &p.GrabbedAt, &p.SourceSystem,
			&p.AcquisitionState); err != nil {
			return nil, fmt.Errorf("read recent provenance: scan: %w", err)
		}
		p.IndexerName, p.IndexerCategories, p.DownloadID = indexerName.String, categories.String, dlID.String
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read recent provenance: %w", err)
	}
	return out, nil
}
