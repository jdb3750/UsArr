package kavita

import (
	"encoding/json"
	"fmt"
)

// The resource types UsArr reads from Kavita, and the ONE body it sends.
//
// Every enum here is an INTEGER in Kavita, not a string. That inverts the trap
// docs/reference/arr-apis.md §7.1 documents for the *Arrs, and it is worth being
// precise about because the fix is the opposite one:
//
//   - *Arr enums are STRINGS. Go's zero value marshals as "", System.Text.Json
//     rejects "" as a member, and the fix is `omitempty` so the field is absent.
//   - Kavita's enums are INTEGERS. Go's zero value marshals as 0, which for some
//     of these IS a valid member (FilterCombination.Or, FilterEntityType.Series)
//     and for others is NOT (SeriesSortField starts at 1; QueryContext is
//     {1,2,4}). `omitempty` would drop a legitimate 0, so the fix is a
//     constructor that cannot produce a non-member and a test that proves it.
//
// TestEnumFieldsCannotMarshalAsEmpty is that test, ported to this polarity.

// ---- enums ----------------------------------------------------------------

// QueryContext is Kavita's `context` query parameter. Members are 1, 2 and 4 —
// there is NO zero member, so a Go zero value is not a legal value to send.
//
// The binder default is QueryContextNone: GetAllSeriesV2's signature is
// `QueryContext context = QueryContext.None` (SeriesController.cs at the pinned
// commit). Sending it explicitly therefore AGREES with the default rather than
// overriding it — and it means a future change to that default cannot silently
// change what UsArr asks for.
type QueryContext int32

const (
	QueryContextNone      QueryContext = 1
	QueryContextSearch    QueryContext = 2
	QueryContextDashboard QueryContext = 4
)

// FilterCombination is how a filter's statements combine. Or=0, And=1.
type FilterCombination int32

const (
	FilterCombinationOr  FilterCombination = 0
	FilterCombinationAnd FilterCombination = 1
)

// FilterEntityType is what a filter selects. Series=0.
type FilterEntityType int32

const (
	FilterEntityTypeSeries      FilterEntityType = 0
	FilterEntityTypeReadingList FilterEntityType = 1
	FilterEntityTypePerson      FilterEntityType = 2
	FilterEntityTypeAnnotation  FilterEntityType = 3
)

// SeriesSortField is the sort key for POST /api/Series/all-v2. Members start at
// 1; there is no zero member.
//
// SeriesSortFieldLastChapterAdded is the one channel 3b is built on
// (ARCHITECTURE.md §7.1a, verified against a live instance in ADR-0035 §2a).
// SeriesSortFieldLastModifiedDate exists and is USELESS for a delta: SeriesDto
// returns no last-modified property, so a walk sorted by it cannot record a
// watermark it can compare against.
type SeriesSortField int32

const (
	SeriesSortFieldSortName           SeriesSortField = 1
	SeriesSortFieldCreatedDate        SeriesSortField = 2
	SeriesSortFieldLastModifiedDate   SeriesSortField = 3
	SeriesSortFieldLastChapterAdded   SeriesSortField = 4
	SeriesSortFieldTimeToRead         SeriesSortField = 5
	SeriesSortFieldReleaseYear        SeriesSortField = 6
	SeriesSortFieldReadProgress       SeriesSortField = 7
	SeriesSortFieldAverageRating      SeriesSortField = 8
	SeriesSortFieldRandom             SeriesSortField = 9
	SeriesSortFieldUserRating         SeriesSortField = 10
	SeriesSortFieldUnreadChapterCount SeriesSortField = 11
)

// MangaFormat is SeriesDto.format. Image=0, Archive=1, Unknown=2, Epub=3, Pdf=4.
type MangaFormat int32

const (
	MangaFormatImage   MangaFormat = 0
	MangaFormatArchive MangaFormat = 1
	MangaFormatUnknown MangaFormat = 2
	MangaFormatEpub    MangaFormat = 3
	MangaFormatPdf     MangaFormat = 4
)

// LibraryType is LibraryDto.type. It is what §17.8's library binding reads to
// decide whether a Kavita library is books or comics.
type LibraryType int32

const (
	LibraryTypeManga      LibraryType = 0
	LibraryTypeComic      LibraryType = 1
	LibraryTypeBook       LibraryType = 2
	LibraryTypeImage      LibraryType = 3
	LibraryTypeLightNovel LibraryType = 4
	LibraryTypeComicVine  LibraryType = 5
)

// ---- read shapes -----------------------------------------------------------

// ServerInfoSlimDto is GET /api/Server/server-info-slim.
//
// ADMIN-ONLY: ServerController carries [Authorize(PolicyGroups.AdminPolicy)] at
// class level, so a valid non-admin Auth Key gets 403 here.
type ServerInfoSlimDto struct {
	InstallID           string    `json:"installId"`
	IsDocker            bool      `json:"isDocker"`
	KavitaVersion       string    `json:"kavitaVersion"`
	FirstInstallDate    Timestamp `json:"firstInstallDate"`
	FirstInstallVersion string    `json:"firstInstallVersion"`
}

// LibraryDto is one entry of GET /api/Library/libraries — the libraries the
// authenticated account can see, which is a narrower list than the server's for
// a non-admin key.
//
// This is UsArr's "the key works" handshake AND the input to §17.8's library
// binding: a user-defined UsArr library is bound to upstream containers a service
// has already named, and these are Kavita's names.
type LibraryDto struct {
	ID                              int32       `json:"id"`
	Name                            string      `json:"name"`
	Type                            LibraryType `json:"type"`
	LastScanned                     Timestamp   `json:"lastScanned"`
	CoverImage                      string      `json:"coverImage"`
	FolderWatching                  bool        `json:"folderWatching"`
	IncludeInDashboard              bool        `json:"includeInDashboard"`
	IncludeInRecommended            bool        `json:"includeInRecommended"`
	ManageCollections               bool        `json:"manageCollections"`
	ManageReadingLists              bool        `json:"manageReadingLists"`
	IncludeInSearch                 bool        `json:"includeInSearch"`
	AllowScrobbling                 bool        `json:"allowScrobbling"`
	Folders                         []string    `json:"folders"`
	CollapseSeriesRelationships     bool        `json:"collapseSeriesRelationships"`
	LibraryFileTypes                []int32     `json:"libraryFileTypes"`
	ExcludePatterns                 []string    `json:"excludePatterns"`
	AllowMetadataMatching           bool        `json:"allowMetadataMatching"`
	EnableMetadata                  bool        `json:"enableMetadata"`
	RemovePrefixForSortName         bool        `json:"removePrefixForSortName"`
	InheritWebLinksFromFirstChapter bool        `json:"inheritWebLinksFromFirstChapter"`
	DefaultLanguage                 string      `json:"defaultLanguage"`
	MetadataProvider                int32       `json:"metadataProvider"`
}

// SeriesDto is one element of POST /api/Series/all-v2's array.
//
// Two things to know before writing anything that reads it:
//
//  1. THERE IS NO lastModified. The spec declares LastChapterAdded and
//     LastChapterAddedUtc and nothing else time-of-change shaped. Use the Utc
//     one — the other is the server's local clock and this is a replica that has
//     to compare across machines.
//  2. THE EXTERNAL IDS ARE USUALLY EMPTY. AniListID, MalID, HardcoverID,
//     MetronID, ComicVineID, MangaBakaID, MangaBakaEditionID and CbrID are
//     written only by the Kavita+ match path, so on a free instance they are 0,
//     null or "". Degraded identity is the ORDINARY case here, not an error
//     (ARCHITECTURE.md §6.4, ADR-0035 §1).
type SeriesDto struct {
	ID            int32  `json:"id"`
	Name          string `json:"name"`
	OriginalName  string `json:"originalName"`
	LocalizedName string `json:"localizedName"`
	SortName      string `json:"sortName"`
	Pages         int32  `json:"pages"`

	CoverImageLocked bool `json:"coverImageLocked"`

	// LastChapterAdded is the server's LOCAL clock. LastChapterAddedUtc is the
	// one channel 3b's watermark is built on.
	LastChapterAdded    Timestamp `json:"lastChapterAdded"`
	LastChapterAddedUtc Timestamp `json:"lastChapterAddedUtc"`

	UserRating     float32   `json:"userRating"`
	HasUserRated   bool      `json:"hasUserRated"`
	TotalReads     int32     `json:"totalReads"`
	PagesRead      int32     `json:"pagesRead"`
	LatestReadDate Timestamp `json:"latestReadDate"`

	Format  MangaFormat `json:"format"`
	Created Timestamp   `json:"created"`

	SortNameLocked      bool `json:"sortNameLocked"`
	LocalizedNameLocked bool `json:"localizedNameLocked"`
	NameLocked          bool `json:"nameLocked"`

	WordCount   int64  `json:"wordCount"`
	LibraryID   int32  `json:"libraryId"`
	LibraryName string `json:"libraryName"`

	MinHoursToRead int32   `json:"minHoursToRead"`
	MaxHoursToRead int32   `json:"maxHoursToRead"`
	AvgHoursToRead float32 `json:"avgHoursToRead"`

	// FolderPath and LowestFolderPath are HOST FILESYSTEM PATHS on the Kavita
	// box. They are modelled so a renamed upstream field is noticed, and they
	// must not be handed to a browser — SanitizeSeries drops them.
	FolderPath        string    `json:"folderPath"`
	LowestFolderPath  string    `json:"lowestFolderPath"`
	LastFolderScanned Timestamp `json:"lastFolderScanned"`

	DontMatch     bool `json:"dontMatch"`
	IsBlacklisted bool `json:"isBlacklisted"`
	IsStandAlone  bool `json:"isStandAlone"`

	CoverImage     string `json:"coverImage"`
	PrimaryColor   string `json:"primaryColor"`
	SecondaryColor string `json:"secondaryColor"`

	// Kavita+ identifiers. Zero / "" on a free instance — see the type comment.
	AniListID          int32  `json:"aniListId"`
	MalID              int64  `json:"malId"`
	HardcoverID        int32  `json:"hardcoverId"`
	MetronID           int64  `json:"metronId"`
	ComicVineID        string `json:"comicVineId"`
	MangaBakaID        int64  `json:"mangaBakaId"`
	MangaBakaEditionID string `json:"mangaBakaEditionId"`
	CbrID              int32  `json:"cbrId"`
}

// Pagination is Kavita's `Pagination` RESPONSE HEADER, which is middleware and is
// NOT in the OpenAPI document.
//
// Kavita.Server/Extensions/HttpExtensions.cs serialises
// Kavita.Common/Helpers/PaginationHeader with a camelCase naming policy and
// appends it. Because it is undocumented, this client READS it when present and
// never requires it: a reverse proxy that strips unknown headers, or an upstream
// that stops sending it, must degrade to "total unknown" rather than fail the
// call.
type Pagination struct {
	CurrentPage  int `json:"currentPage"`
	ItemsPerPage int `json:"itemsPerPage"`
	TotalItems   int `json:"totalItems"`
	TotalPages   int `json:"totalPages"`
}

// ---- the one outbound body -------------------------------------------------

// SeriesSortOption is SeriesFilterV2Dto.sortOptions.
type SeriesSortOption struct {
	SortField   SeriesSortField `json:"sortField"`
	IsAscending bool            `json:"isAscending"`
}

// SeriesFilter is the POST /api/Series/all-v2 body, and it is deliberately a type
// that CANNOT express anything the endpoint does not read.
//
// docs/reference/arr-apis.md §7.1, applied to Kavita: the body is model-bound to
// the fully typed SeriesFilterV2Dto BEFORE the handler runs, so every property
// sent is type-checked whether or not the handler reads it. Three specific
// hazards, each verified rather than assumed:
//
//  1. `statements` MUST NOT be null. GetAllSeriesV2's first act is
//     `seriesFilterDto.Statements.Add(stmt)` in a loop over the profile-privacy
//     statements (SeriesController.cs at the pinned commit). A JSON null
//     overwrites the C# initializer with null and that call nil-derefs into a
//  500. A Go nil slice marshals as `null`, which is exactly the value that
//     breaks it — so MarshalJSON below substitutes `[]`, and the field has no
//     `omitempty`. This is the one field where Go's default behaviour is not
//     merely noisy but fatal.
//  2. `sortOptions.sortField` MUST be a member. SeriesSortField starts at 1, so
//     the Go zero value 0 is not one, and [EnumDataType] validation is what
//     rejects it. NewSeriesFilter cannot produce a zero.
//  3. Everything else is an int32 whose zero IS the value UsArr wants —
//     `id` 0, `limitTo` 0 (no limit), `combination` Or, `entityType` Series — so
//     nothing here carries `omitempty`. Adding it would silently drop a
//     meaningful zero, which is the mirror-image bug of the *Arr one.
type SeriesFilter struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`

	// Statements is the filter's predicates. UsArr sends none: there is no
	// timestamp member in SeriesFilterField (35 members, checked against the
	// vendored spec by TestNoTimestampFilterFieldExists), so no server-side
	// "since" is expressible and the delta is a client-side stop instead
	// (ARCHITECTURE.md §7.1a).
	Statements []SeriesFilterStatement `json:"statements"`

	Combination FilterCombination `json:"combination"`
	SortOptions SeriesSortOption  `json:"sortOptions"`
	EntityType  FilterEntityType  `json:"entityType"`

	// LimitTo is a plain int32, not an enum. 0 means no limit.
	LimitTo int32 `json:"limitTo"`
}

// SeriesFilterStatement is one predicate. UsArr does not build these yet; the
// type exists so the body shape is complete and so the contract test can prove
// the field/comparison enums are integers rather than strings.
type SeriesFilterStatement struct {
	Comparison int32  `json:"comparison"`
	Field      int32  `json:"field"`
	Value      string `json:"value"`
}

// seriesFilterWire is SeriesFilter's marshalling twin. It exists so MarshalJSON
// can normalise `statements` without recursing.
type seriesFilterWire struct {
	ID          int32                   `json:"id"`
	Name        *string                 `json:"name"`
	Statements  []SeriesFilterStatement `json:"statements"`
	Combination FilterCombination       `json:"combination"`
	SortOptions SeriesSortOption        `json:"sortOptions"`
	EntityType  FilterEntityType        `json:"entityType"`
	LimitTo     int32                   `json:"limitTo"`
}

// MarshalJSON emits `"statements":[]` for a nil slice.
//
// This is not tidiness. See hazard (1) on SeriesFilter: `"statements":null` is a
// 500 on the other end, and a nil slice is what Go produces by default. Making it
// unrepresentable in the encoder is the only place the guarantee holds for every
// caller, including one that builds a SeriesFilter literal by hand.
//
// `name` is emitted as JSON null when empty, which the schema declares nullable
// and the handler does not read.
func (f SeriesFilter) MarshalJSON() ([]byte, error) {
	w := seriesFilterWire{
		ID:          f.ID,
		Statements:  f.Statements,
		Combination: f.Combination,
		SortOptions: f.SortOptions,
		EntityType:  f.EntityType,
		LimitTo:     f.LimitTo,
	}
	if f.Name != "" {
		n := f.Name
		w.Name = &n
	}
	if w.Statements == nil {
		w.Statements = []SeriesFilterStatement{}
	}
	return json.Marshal(w)
}

// NewSeriesFilter builds the body UsArr actually sends: every series, no
// predicates, ordered by the sort key the caller names.
//
// It REFUSES a sort field that is not a member rather than letting a zero value
// reach the wire, because the failure it prevents is total — the endpoint 400s
// and every series read fails, not just this one.
func NewSeriesFilter(sortField SeriesSortField, ascending bool) (SeriesFilter, error) {
	if !sortField.valid() {
		return SeriesFilter{}, fmt.Errorf(
			"%w: sortField %d is not a member of SeriesSortField (members are 1..11); "+
				"a zero value here is a 400 on every call, not a default",
			ErrInvalidRequest, int32(sortField))
	}
	return SeriesFilter{
		Statements: []SeriesFilterStatement{},
		// And, not Or, because that is the combination the body verified against
		// the owner's live instance carried (`"combination":1`). With zero
		// statements the two are indistinguishable in result, and "indistinguishable
		// in result" is not a reason to send something other than what was measured.
		Combination: FilterCombinationAnd,
		SortOptions: SeriesSortOption{SortField: sortField, IsAscending: ascending},
		EntityType:  FilterEntityTypeSeries,
		LimitTo:     0,
	}, nil
}

func (f SeriesSortField) valid() bool {
	return f >= SeriesSortFieldSortName && f <= SeriesSortFieldUnreadChapterCount
}

func (c QueryContext) valid() bool {
	switch c {
	case QueryContextNone, QueryContextSearch, QueryContextDashboard:
		return true
	}
	return false
}
