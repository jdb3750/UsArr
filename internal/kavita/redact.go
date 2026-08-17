package kavita

import (
	"github.com/jdb3750/UsArr/internal/ssrf"
)

// RedactedPlaceholder is what a stripped credential is replaced with in a HEADER
// or a request body. It is a visible marker rather than an empty string so a
// redacted value still reads as "there was a secret here".
const RedactedPlaceholder = "<redacted>"

// RedactURL strips credential-bearing query parameters and any userinfo from a
// URL so it is safe to log, store or return in an error.
//
// It matters here for one specific URL: GET /api/Plugin/version takes the Auth
// Key as `?apiKey=`, because the controller reads it from the query and offers no
// header path. `apikey` is in internal/ssrf's shared deny-list and the match is
// case-insensitive, so that URL redacts. TestPluginVersionURLIsRedacted fires it.
//
// It delegates to internal/ssrf so there is exactly one deny-list of credential
// parameter names in the codebase. A second, drifting copy is how a secret ends
// up redacted in one code path and not the other.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}
	return ssrf.RedactRawURL(raw)
}

// SeriesView is the ONLY shape of a Kavita series that may cross the boundary
// toward a browser, a log line, an SSE payload or a support bundle.
//
// IT IS AN ALLOWLIST, FIELD BY FIELD, and it is a distinct TYPE rather than a
// copy-with-some-fields-cleared for one reason: a denylist silently passes
// through every field upstream adds next. SeriesDto gained `mangaBakaId` and
// `mangaBakaEditionId` on the develop line this spec is pinned to; a sanitiser
// that removed known-bad fields would have shipped both without anyone deciding
// to.
//
// What is deliberately NOT here, and why:
//
//   - FolderPath, LowestFolderPath — HOST FILESYSTEM PATHS on the Kavita box.
//     They disclose the operator's directory layout, mount points and often their
//     username, and no screen needs them. The same class of value that
//     internal/servarr's SystemResource skip-list refuses for the *Arrs
//     ("host filesystem paths UsArr must not store").
//   - DontMatch, IsBlacklisted, CoverImageLocked, the three *Locked flags —
//     upstream bookkeeping, not UsArr's to render.
//   - LatestReadDate, PagesRead, TotalReads, UserRating, HasUserRated — per-user
//     read state. It is the Kavita ACCOUNT UsArr's credential belongs to, not the
//     UsArr user asking, so rendering it would attribute one person's progress to
//     another. Per-user statistics are deferred (CLAUDE.md non-goals).
//
// Nothing in SeriesDto is credential-bearing in the way a Prowlarr release is —
// there is no field into which Kavita splices its own key — so this is a
// disclosure boundary rather than a secret boundary. It is written as an
// allowlist anyway, because that is the rule and because the next field upstream
// adds is not knowable from here.
type SeriesView struct {
	ID            int32  `json:"id"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name,omitempty"`
	LocalizedName string `json:"localized_name,omitempty"`
	SortName      string `json:"sort_name,omitempty"`

	LibraryID   int32  `json:"library_id"`
	LibraryName string `json:"library_name,omitempty"`

	Format MangaFormat `json:"format"`
	Pages  int32       `json:"pages"`

	// LastChapterAddedUtc only. The non-Utc twin is the Kavita host's local clock
	// and would render as a different instant to anyone in another zone.
	LastChapterAddedUtc string `json:"last_chapter_added_utc,omitempty"`
	Created             string `json:"created,omitempty"`

	// PrimaryColor and SecondaryColor are hex strings Kavita derives from the
	// cover; they are safe and they are what a grid tile is tinted with.
	PrimaryColor   string `json:"primary_color,omitempty"`
	SecondaryColor string `json:"secondary_color,omitempty"`
}

// ToView projects a SeriesDto onto the allowlist. It is the only supported way to
// get Kavita series data out of this package toward a browser.
func ToView(s SeriesDto) SeriesView {
	v := SeriesView{
		ID:             s.ID,
		Name:           s.Name,
		OriginalName:   s.OriginalName,
		LocalizedName:  s.LocalizedName,
		SortName:       s.SortName,
		LibraryID:      s.LibraryID,
		LibraryName:    s.LibraryName,
		Format:         s.Format,
		Pages:          s.Pages,
		PrimaryColor:   s.PrimaryColor,
		SecondaryColor: s.SecondaryColor,
	}
	// The RAW wire value is carried across, not a re-rendered one. See Timestamp:
	// re-rendering a zoneless .NET DateTime would append a `Z` and manufacture a
	// claim about the instant that the value does not make.
	if !s.LastChapterAddedUtc.IsZero() {
		v.LastChapterAddedUtc = s.LastChapterAddedUtc.Raw()
	}
	if !s.Created.IsZero() {
		v.Created = s.Created.Raw()
	}
	return v
}

// LibraryView is the allowlisted projection of a LibraryDto, for §17.8's binding
// picker.
//
// LibraryDto.Folders is HOST FILESYSTEM PATHS on the Kavita box and is the reason
// this type exists. `/mnt/user/media/manga` tells a browser the operator's mount
// layout for no benefit — the picker binds on id and name.
type LibraryView struct {
	ID   int32       `json:"id"`
	Name string      `json:"name"`
	Type LibraryType `json:"type"`
}

// ToLibraryView projects a LibraryDto onto the allowlist.
func ToLibraryView(l LibraryDto) LibraryView {
	return LibraryView{ID: l.ID, Name: l.Name, Type: l.Type}
}
