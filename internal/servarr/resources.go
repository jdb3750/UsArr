package servarr

import "time"

// Resource types for Prowlarr's /api/v1 surface, transcribed from the vendored
// OpenAPI document (api/specs/prowlarr.json).
//
// JSON conventions that shape every type here:
//
//   - Properties are camelCase; enums are camelCase STRINGS on output (integers
//     are accepted on input, which is why the enum types are string-kinded but
//     never compared against integers).
//   - JsonSerializerOptions sets DefaultIgnoreCondition = WhenWritingNull, so a
//     null field is OMITTED ENTIRELY rather than emitted as null. That makes
//     "absent" and "zero" indistinguishable for any value type, so every nullable
//     scalar below is a pointer. `*Seeders == nil` means the indexer is usenet or
//     did not report; `*Seeders == 0` means a torrent with no seeds. Collapsing
//     those two into 0 is a real bug on a results screen.
//   - Dates are yyyy-MM-ddTHH:mm:ssZ, UTC, second precision. RFC3339 parses them.

// DownloadProtocol is byte-identical across the Prowlarr, Sonarr and Radarr specs
// and is the backbone of `source:` tagging (tags.md §3).
type DownloadProtocol string

const (
	ProtocolUnknown DownloadProtocol = "unknown"
	ProtocolUsenet  DownloadProtocol = "usenet"
	ProtocolTorrent DownloadProtocol = "torrent"
)

// IndexerPrivacy feeds the `indexer-privacy:` tag. Note the wire form is
// camelCase "semiPrivate" while the tag vocabulary is "semi-private".
type IndexerPrivacy string

const (
	PrivacyPublic      IndexerPrivacy = "public"
	PrivacySemiPrivate IndexerPrivacy = "semiPrivate"
	PrivacyPrivate     IndexerPrivacy = "private"
)

// PingResource is the body of GET {base}/ping. The endpoint is [AllowAnonymous],
// which makes it the only probe that distinguishes "host down" from "key wrong",
// and it returns 500 with status "Error" when the *Arr's own database is
// unreachable — so it also distinguishes "not a Servarr app" from "sick Servarr".
type PingResource struct {
	Status string `json:"status"` // "OK" | "Error"
}

// APIInfoResource is the body of GET {base}/api (requires the API key).
//
// This is the forward-compatible version check: read `current` and refuse anything
// that is not the version this client speaks. NEVER derive the API path from the
// app version — Prowlarr 2.x still serves /api/v1.
type APIInfoResource struct {
	Current    string   `json:"current"`
	Deprecated []string `json:"deprecated"`
}

// SystemResource is GET /api/v1/system/status.
//
// AppName is the handshake: Sonarr and Radarr answer this identical shape on their
// own paths, so the path alone proves nothing about which app answered.
type SystemResource struct {
	AppName         string    `json:"appName"`
	InstanceName    string    `json:"instanceName"`
	Version         string    `json:"version"`
	BuildTime       time.Time `json:"buildTime"`
	IsDebug         bool      `json:"isDebug"`
	IsProduction    bool      `json:"isProduction"`
	OsName          string    `json:"osName"`
	OsVersion       string    `json:"osVersion"`
	IsDocker        bool      `json:"isDocker"`
	Branch          string    `json:"branch"`
	DatabaseType    string    `json:"databaseType"`
	DatabaseVersion string    `json:"databaseVersion"`
	// Authentication is AuthenticationType:
	// none | basic | forms | external | disabledForLocalAddresses.
	//
	// When it is disabledForLocalAddresses, a request from a local address
	// SUCCEEDS WITH NO KEY. A 200 from system/status therefore does not prove the
	// stored key is valid, and the health UI must say "reachable" rather than
	// "credential verified".
	Authentication   string    `json:"authentication"`
	MigrationVersion int32     `json:"migrationVersion"`
	URLBase          string    `json:"urlBase"`
	RuntimeVersion   string    `json:"runtimeVersion"`
	StartTime        time.Time `json:"startTime"`
	PackageVersion   string    `json:"packageVersion"`
}

// Field is the shared *Arr provider-settings field model.
//
// Privacy ∈ {normal, password, apiKey, userName} tells you exactly which fields
// carry a credential. UsArr never sends a Field array to the browser at all —
// see SanitizeIndexer — but the type is here because a POST/PUT of a provider
// resource must round-trip the whole array unmodified.
type Field struct {
	Order    int32  `json:"order"`
	Name     string `json:"name"`
	Label    string `json:"label"`
	HelpText string `json:"helpText,omitempty"`
	Value    any    `json:"value,omitempty"`
	Type     string `json:"type,omitempty"`
	Advanced bool   `json:"advanced,omitempty"`
	Privacy  string `json:"privacy,omitempty"`
}

// IndexerCategory is a Newznab/Torznab category. Capture the raw ids; never
// collapse the array (arr-apis.md §8) — 3030 is the only reliable machine signal
// separating audiobook from music, and 7030 likewise for comics.
type IndexerCategory struct {
	ID            int32             `json:"id"`
	Name          string            `json:"name,omitempty"`
	Description   string            `json:"description,omitempty"`
	SubCategories []IndexerCategory `json:"subCategories,omitempty"`
}

// IndexerCapabilityResource drives how UsArr queries each indexer: LimitsMax caps
// the per-indexer `limit`, and the *SearchParams arrays say whether an id-based
// query (imdbId, tvdbId…) is supported or whether it must fall back to free text.
type IndexerCapabilityResource struct {
	ID                int32             `json:"id"`
	LimitsMax         *int32            `json:"limitsMax,omitempty"`
	LimitsDefault     *int32            `json:"limitsDefault,omitempty"`
	Categories        []IndexerCategory `json:"categories,omitempty"`
	SupportsRawSearch bool              `json:"supportsRawSearch"`
	SearchParams      []string          `json:"searchParams,omitempty"`
	TvSearchParams    []string          `json:"tvSearchParams,omitempty"`
	MovieSearchParams []string          `json:"movieSearchParams,omitempty"`
	MusicSearchParams []string          `json:"musicSearchParams,omitempty"`
	BookSearchParams  []string          `json:"bookSearchParams,omitempty"`
}

// IndexerResource is GET /api/v1/indexer.
//
// The endpoint returns ENABLED AND DISABLED indexers, sorted by name, with no
// filter parameter — filter on Enable client-side.
//
// Status is declared on this resource but is null in practice; read blocked
// indexers from GET /api/v1/indexerstatus instead.
type IndexerResource struct {
	ID                 int32                     `json:"id"`
	Name               string                    `json:"name"`
	Fields             []Field                   `json:"fields,omitempty"` // CREDENTIALS — never serve to a client
	ImplementationName string                    `json:"implementationName,omitempty"`
	Implementation     string                    `json:"implementation,omitempty"`
	ConfigContract     string                    `json:"configContract,omitempty"`
	InfoLink           string                    `json:"infoLink,omitempty"`
	Tags               []int32                   `json:"tags,omitempty"`
	Presets            []IndexerResource         `json:"presets,omitempty"`
	IndexerUrls        []string                  `json:"indexerUrls,omitempty"`
	DefinitionName     string                    `json:"definitionName,omitempty"`
	Description        string                    `json:"description,omitempty"`
	Language           string                    `json:"language,omitempty"`
	Encoding           string                    `json:"encoding,omitempty"`
	Enable             bool                      `json:"enable"`
	Redirect           bool                      `json:"redirect"`
	SupportsRSS        bool                      `json:"supportsRss"`
	SupportsSearch     bool                      `json:"supportsSearch"`
	SupportsRedirect   bool                      `json:"supportsRedirect"`
	SupportsPagination bool                      `json:"supportsPagination"`
	AppProfileID       int32                     `json:"appProfileId"`
	Protocol           DownloadProtocol          `json:"protocol"`
	Privacy            IndexerPrivacy            `json:"privacy"`
	Capabilities       IndexerCapabilityResource `json:"capabilities"`
	// Priority is 1-50 and LOWER WINS. It is the tiebreak Prowlarr itself uses when
	// deduplicating results by guid across indexers, and UsArr matches that rule.
	Priority         int32                  `json:"priority"`
	DownloadClientID int32                  `json:"downloadClientId,omitempty"`
	Added            *time.Time             `json:"added,omitempty"`
	Status           *IndexerStatusResource `json:"status,omitempty"`
	SortName         string                 `json:"sortName,omitempty"`
}

// IndexerStatusResource is one entry of GET /api/v1/indexerstatus.
//
// The endpoint returns ONLY BLOCKED indexers. An empty array means every indexer
// is healthy — it does not mean the call failed. Correlating a search against this
// is the only way to distinguish "no results" from "three of your eight indexers
// are down", because search itself swallows per-indexer failures.
type IndexerStatusResource struct {
	ID                int32      `json:"id"`
	IndexerID         int32      `json:"indexerId"`
	DisabledTill      *time.Time `json:"disabledTill,omitempty"`
	MostRecentFailure *time.Time `json:"mostRecentFailure,omitempty"`
	InitialFailure    *time.Time `json:"initialFailure,omitempty"`
}

// DownloadClientCategory maps an indexer category onto a client-side label.
type DownloadClientCategory struct {
	ID             int32   `json:"id"`
	ClientCategory string  `json:"clientCategory,omitempty"`
	Name           string  `json:"name,omitempty"`
	Categories     []int32 `json:"categories,omitempty"`
}

// DownloadClientResource is GET /api/v1/downloadclient.
//
// Preflighting this list is what turns Prowlarr's 500 + .NET stack trace ("Torrent
// Download client isn't configured yet") into an actionable message before the
// grab is even attempted.
type DownloadClientResource struct {
	ID                 int32                    `json:"id"`
	Name               string                   `json:"name"`
	Fields             []Field                  `json:"fields,omitempty"` // CREDENTIALS
	ImplementationName string                   `json:"implementationName,omitempty"`
	Implementation     string                   `json:"implementation,omitempty"`
	ConfigContract     string                   `json:"configContract,omitempty"`
	Tags               []int32                  `json:"tags,omitempty"`
	Presets            []DownloadClientResource `json:"presets,omitempty"`
	Enable             bool                     `json:"enable"`
	Protocol           DownloadProtocol         `json:"protocol"`
	Priority           int32                    `json:"priority"`
	Categories         []DownloadClientCategory `json:"categories,omitempty"`
	SupportsCategories bool                     `json:"supportsCategories"`
}

// ReleaseResource is one search result, and the body POSTed back to grab it.
//
// SECURITY, load-bearing: SearchController.MapReleases rewrites DownloadURL and
// MagnetURL into Prowlarr proxy links of the form
// `{serverUrl}{urlBase}/{indexerId}/download?apikey={ApiKey}&link=…&file=…`.
// Both fields therefore contain PROWLARR'S FULL ADMIN API KEY IN PLAINTEXT on
// every single search result. Run SanitizeRelease before anything crosses the
// boundary toward a browser, an SSE frame, a log line — or PERSISTENCE. Writing
// to disk is a boundary too, and the worst of them: the process memory holding
// this resource is gone in minutes, whereas a row in SQLite is in every backup
// and every restored volume forever. internal/releases.persist sanitises before
// it marshals for exactly that reason.
//
// Dropping the two fields costs nothing: the grab sends GrabBody(), which does
// not include them, and no code reads them back off a stored release.
//
// The proxy link's host comes from Request.GetServerUrl(), which honours
// X-Forwarded-Proto and Front-End-Https, so it is header-influenceable and must be
// treated as untrusted input.
//
// MagnetURL is an http(s) proxy link, NOT a magnet: URI. It cannot be handed to a
// torrent client; build one from InfoHash if that is ever needed.
//
// PosterURL is indexer-supplied, read out of a response body: SSRF class
// `derived` (security.md §2), never the `configured` policy.
type ReleaseResource struct {
	// ID is declared in the spec but never emitted by the search controller.
	// Grab validation ignores it. Present only so a round-trip is lossless.
	ID int32 `json:"id,omitempty"`

	GUID        string  `json:"guid,omitempty"`
	Age         int32   `json:"age"`
	AgeHours    float64 `json:"ageHours"`
	AgeMinutes  float64 `json:"ageMinutes"`
	Size        int64   `json:"size"`
	Files       *int32  `json:"files,omitempty"`
	Grabs       *int32  `json:"grabs,omitempty"`
	IndexerID   int32   `json:"indexerId"`
	Indexer     string  `json:"indexer,omitempty"`
	SubGroup    string  `json:"subGroup,omitempty"`
	ReleaseHash string  `json:"releaseHash,omitempty"`
	Title       string  `json:"title,omitempty"`
	SortTitle   string  `json:"sortTitle,omitempty"`

	// IMDbID is NUMERIC here (int32), not the tt-prefixed string Sonarr and Radarr
	// use on SeriesResource/MovieResource. 0 means absent. See
	// docs/DEVELOPMENT.md §5's (app, resource) table.
	IMDbID   int32 `json:"imdbId"`
	TMDbID   int32 `json:"tmdbId"`
	TVDbID   int32 `json:"tvdbId"`
	TVMazeID int32 `json:"tvMazeId"`

	PublishDate time.Time `json:"publishDate"`
	CommentURL  string    `json:"commentUrl,omitempty"`

	// DownloadURL and MagnetURL: see the security note above.
	DownloadURL *string `json:"downloadUrl,omitempty"`
	MagnetURL   *string `json:"magnetUrl,omitempty"`

	InfoURL   string `json:"infoUrl,omitempty"`
	PosterURL string `json:"posterUrl,omitempty"`

	// IndexerFlags is string[] on Prowlarr (an int32 bitmask on Radarr's
	// MovieFileResource, and untyped in Radarr's ReleaseResource).
	IndexerFlags []string          `json:"indexerFlags,omitempty"`
	Categories   []IndexerCategory `json:"categories,omitempty"`

	InfoHash *string `json:"infoHash,omitempty"`
	Seeders  *int32  `json:"seeders,omitempty"`
	Leechers *int32  `json:"leechers,omitempty"`

	Protocol DownloadProtocol `json:"protocol"`

	// FileName is readOnly/computed upstream. Never send it back — hence the
	// omitempty and the fact that GrabBody clears it.
	FileName string `json:"fileName,omitempty"`

	// DownloadClientID names one of PROWLARR'S OWN configured download clients.
	// It is the reason UsArr needs no download-client integration at all in v0.1.
	DownloadClientID *int32 `json:"downloadClientId,omitempty"`
}

// GrabBody returns the minimal, safe body for POST /api/v1/search.
//
// Grab validation requires IndexerID > 0 and a non-empty GUID; everything else in
// the body is ignored EXCEPT DownloadClientID. The handler then looks the release
// up in its own in-process cache keyed "{indexerId}_{guid}" — so sending the
// whole resource back buys nothing and would echo the embedded API key back over
// the wire in DownloadURL/MagnetURL for no reason.
func (r ReleaseResource) GrabBody() ReleaseResource {
	return ReleaseResource{
		GUID:             r.GUID,
		IndexerID:        r.IndexerID,
		DownloadClientID: r.DownloadClientID,
	}
}

// CategoryIDs flattens Categories to raw ids, including subcategories.
func (r ReleaseResource) CategoryIDs() []int32 {
	var out []int32
	var walk func([]IndexerCategory)
	walk = func(cs []IndexerCategory) {
		for _, c := range cs {
			out = append(out, c.ID)
			walk(c.SubCategories)
		}
	}
	walk(r.Categories)
	return out
}
