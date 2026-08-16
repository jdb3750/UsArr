// Package mapping turns *Arr resources into UsArr's unified shapes.
//
// It is separate from the transport package for two reasons. First, the mapping
// is where UsArr's own vocabulary lives (docs/reference/tags.md), and that must
// not drift into a client whose job is HTTP. Second, when Sonarr and Radarr land
// they emit differently-shaped resources for the same concepts — Radarr's
// ReleaseResource.imdbId is int32 while Sonarr's is a string, indexerFlags is a
// bitmask in one place and a string array in another — and the place to absorb
// that is one mapper per (app, resource), not one client per app.
package mapping

import (
	"strings"
	"time"
	"unicode"

	"github.com/jdb3750/UsArr/internal/servarr"
)

// Release is the credential-free unified view of one indexer result.
//
// DownloadURL and MagnetURL are DELIBERATELY ABSENT. Prowlarr embeds its full
// admin API key in both on every search result, so the unified shape simply has
// nowhere to put them; the grab path works from the stored raw resource instead.
type Release struct {
	GUID  string
	Title string // the raw scene/P2P name — stored verbatim, forever

	IndexerID       int32
	IndexerName     string
	IndexerPriority int32 // 1-50, lower wins
	IndexerPrivacy  string

	Protocol string // usenet | torrent | unknown

	// Categories is the RAW Newznab/Torznab id array. Never collapse it: 3030 is
	// the only reliable machine signal separating audiobook from music at
	// acquisition time, and 7030 likewise for comics.
	Categories   []int32
	IndexerFlags []string

	SizeBytes   int64
	Files       *int32
	Grabs       *int32
	Seeders     *int32
	Leechers    *int32
	AgeDays     float64
	PublishedAt time.Time

	InfoURL    string
	CommentURL string
	InfoHash   string

	// PosterURL is indexer-supplied, read out of a response body: SSRF class
	// `derived` (security.md §2). Allowlisted to the originating instance's own
	// resolved host:port, or the strict `provider` policy. Never `configured`.
	PosterURL string

	// External ids. Prowlarr emits imdbId as a NUMERIC int32 with 0 meaning
	// absent, not the tt-prefixed string Sonarr/Radarr use on their library
	// resources.
	IMDbID   int32
	TMDbID   int32
	TVDbID   int32
	TVMazeID int32

	SubGroup         string
	ReleaseHash      string
	DownloadClientID *int32
}

// FromProwlarrRelease maps one ReleaseResource. ix may be nil when the owning
// indexer is not known; priority and privacy are then left zero/empty.
func FromProwlarrRelease(r servarr.ReleaseResource, ix *servarr.IndexerResource) Release {
	out := Release{
		GUID:             r.GUID,
		Title:            r.Title,
		IndexerID:        r.IndexerID,
		IndexerName:      r.Indexer,
		Protocol:         string(normalizeProtocol(r.Protocol)),
		Categories:       r.CategoryIDs(),
		IndexerFlags:     r.IndexerFlags,
		SizeBytes:        r.Size,
		Files:            r.Files,
		Grabs:            r.Grabs,
		Seeders:          r.Seeders,
		Leechers:         r.Leechers,
		AgeDays:          float64(r.Age),
		PublishedAt:      r.PublishDate,
		InfoURL:          r.InfoURL,
		CommentURL:       r.CommentURL,
		PosterURL:        r.PosterURL,
		IMDbID:           r.IMDbID,
		TMDbID:           r.TMDbID,
		TVDbID:           r.TVDbID,
		TVMazeID:         r.TVMazeID,
		SubGroup:         r.SubGroup,
		ReleaseHash:      r.ReleaseHash,
		DownloadClientID: r.DownloadClientID,
	}
	// AgeHours is more precise than the whole-day Age when both are present.
	if r.AgeHours > 0 {
		out.AgeDays = r.AgeHours / 24
	}
	if r.InfoHash != nil {
		out.InfoHash = *r.InfoHash
	}
	if ix != nil {
		out.IndexerPriority = ix.Priority
		out.IndexerPrivacy = PrivacyValue(ix.Privacy)
		if out.IndexerName == "" {
			out.IndexerName = ix.Name
		}
	}
	return out
}

// normalizeProtocol maps an unset or unrecognised protocol onto "unknown" rather
// than laundering it into "torrent". schema.md §6 is explicit that manual and
// filesystem imports get protocol='manual' and that unknown is never upgraded.
func normalizeProtocol(p servarr.DownloadProtocol) servarr.DownloadProtocol {
	switch p {
	case servarr.ProtocolUsenet, servarr.ProtocolTorrent:
		return p
	default:
		return servarr.ProtocolUnknown
	}
}

// SourceValue is the `source:` tag value for a download protocol. The enum is
// byte-identical across the Prowlarr, Sonarr and Radarr specs, which is what makes
// source tagging an assertion rather than an inference (tags.md §3).
func SourceValue(p servarr.DownloadProtocol) string {
	return string(normalizeProtocol(p))
}

// PrivacyValue converts Prowlarr's camelCase IndexerPrivacy onto the tag
// vocabulary's kebab-case form: semiPrivate -> semi-private.
func PrivacyValue(p servarr.IndexerPrivacy) string {
	switch p {
	case servarr.PrivacyPublic:
		return "public"
	case servarr.PrivacySemiPrivate:
		return "semi-private"
	case servarr.PrivacyPrivate:
		return "private"
	default:
		return ""
	}
}

// ParentCategory is floor(cat/1000)*1000 (arr-apis.md §8).
//
// Categories >= 100000 are site-specific and need the indexer's t=caps to
// resolve; the parent category Prowlarr also emits is the fallback, which is why
// callers pass the whole raw array and not one id.
func ParentCategory(cat int32) int32 {
	if cat < 0 {
		return 0
	}
	return (cat / 1000) * 1000
}

// MediaType maps a raw category array onto UsArr's (type, format) vocabulary.
//
// It returns the type: value, the format: value (often empty) and whether
// anything was recognised. Note what is deliberately NOT here:
//
//   - There is no type:audiobook. An audiobook is an edition of a book work, so
//     3030 is type:book + format:audiobook (ARCHITECTURE §6.1, tags.md §2).
//     Getting this wrong reintroduces the contradiction the schema change removed.
//   - 5070 (anime) maps to type:tv. "anime" is not in the type: vocabulary and the
//     signal is not lost: the raw category array is stored intact.
//   - 6000 (adult) has no type: value in the v0.1 vocabulary, so it yields ok=false
//     rather than being laundered into type:movie.
//
// Both type: and format: are single-valued per entity, so the first recognised
// category wins and later ones are ignored.
func MediaType(cats []int32) (typeValue, formatValue string, ok bool) {
	// TWO PASSES, and the order is the whole point. Prowlarr emits the parent
	// category alongside the specific one, so [3000, 3030] arrives with the
	// LESS informative id first. A single pass returns type:music for an
	// audiobook — which is precisely the mistake category 3030 exists to prevent.
	for _, c := range cats {
		switch c {
		case 3030: // audiobook: a book work with an audiobook edition, never type:music
			return "book", "audiobook", true
		case 7020: // ebook
			return "book", "ebook", true
		case 7030: // comic
			return "comic", "", true
		case 7010: // magazine — no format: value exists for it in the v0.1 vocabulary
			return "book", "", true
		}
	}
	for _, c := range cats {
		switch ParentCategory(c) {
		case 2000:
			return "movie", "", true
		case 5000: // includes 5070 anime
			return "tv", "", true
		case 3000:
			return "music", "", true
		case 7000:
			return "book", "", true
		case 1000, 4000:
			return "game", "", true
		case 6000:
			// adult: recognised, but outside the v0.1 type: vocabulary.
			continue
		}
	}
	return "", "", false
}

// Slug normalises a free-text name into a tag value: lowercase, non-alphanumeric
// runs collapsed to a single hyphen, trimmed. Indexer names arrive with spaces,
// parentheses and locale suffixes ("AnimeBytes (API)"), and a tag value with a
// space in it is unfilterable.
func Slug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastHyphen := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// IndexerByID indexes a slice of indexers for the lookups the search fan-out and
// the dedupe rule need.
func IndexerByID(ixs []servarr.IndexerResource) map[int32]servarr.IndexerResource {
	m := make(map[int32]servarr.IndexerResource, len(ixs))
	for _, ix := range ixs {
		m[ix.ID] = ix
	}
	return m
}

// BlockedUntil indexes GET /api/v1/indexerstatus by indexer id.
//
// The endpoint returns only blocked indexers, so a missing key means healthy. A
// nil DisabledTill means blocked with no stated end.
func BlockedUntil(sts []servarr.IndexerStatusResource) map[int32]*time.Time {
	m := make(map[int32]*time.Time, len(sts))
	for _, s := range sts {
		m[s.IndexerID] = s.DisabledTill
	}
	return m
}
