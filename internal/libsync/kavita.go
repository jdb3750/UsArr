package libsync

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// KavitaReader is the slice of *kavita.Client this adapter uses. It is an
// interface so the mapping can be tested against a hand-built series list
// without an HTTP round trip, and so nothing here can reach a method that
// bypasses the injected SSRF policy client.
type KavitaReader interface {
	Libraries(ctx context.Context) ([]kavita.LibraryDto, error)
	StreamSeries(ctx context.Context, opts kavita.SeriesListOptions, fn func(kavita.SeriesDto) error) (kavita.SeriesPage, error)
}

// KavitaSource adapts one Kavita instance to Source.
type KavitaSource struct {
	Client KavitaReader

	// containerKind remembers each library's declared LibraryType between the
	// Containers call and the item stream, because SeriesDto reports libraryId
	// and NOT the library's type — and work.kind comes from the type.
	containerKind map[string]kindDecision
}

// NewKavitaSource wraps a client.
func NewKavitaSource(c KavitaReader) *KavitaSource {
	return &KavitaSource{Client: c, containerKind: map[string]kindDecision{}}
}

// kindDecision is what one LibraryType resolves to.
type kindDecision struct {
	// Kind is a work.kind, or "" when UsArr has no kind for this container.
	Kind string

	// ReadingDirection is work_comic.reading_direction, or "" for none.
	ReadingDirection string

	// Reason is why a container was declined. Empty when Kind is non-empty.
	Reason string
}

// mapLibraryType is THE kind decision, and it is UsArr's rather than Kavita's.
//
// ARCHITECTURE.md §6.4: "The kind decision is UsArr's, made once at ingest …
// work.kind is derived from a rule UsArr controls — the library's declared kind
// (§6.5) — and never inherited from whichever backend answered first." The input
// is LibraryDto.type. It is NEVER SeriesDto.format: MangaFormat is how the bytes
// are packaged (Archive, Epub, Pdf), and a PDF in a comic library is a comic.
//
// EVERY MEMBER OF LibraryType IS ACCOUNTED FOR BELOW, including the ones that do
// not map. The enum is read from the vendored api/specs/kavita.json (develop @
// 9c3e540): {0 Manga, 1 Comic (Flexible), 2 Book, 3 Image, 4 Light Novel,
// 5 Comic (ComicVine)}, and TestEveryLibraryTypeMemberIsMapped fails if the
// vendored spec grows a seventh.
//
// ⚠️ ARCHITECTURE.md §17.8 CARRIES A WITHDRAWAL THAT CONTRADICTS THE VENDORED
// SPEC, and this comment says so rather than silently picking a side. §17.8
// reads: "re-checked against Kavita main on 2026-08-16,
// API/Entities/Enums/LibraryType.cs declares exactly Manga = 0, Comic = 1,
// Book = 2 and no Image member at all". The vendored spec, which is what this
// repository actually pins and what internal/kavita's contract test asserts
// against, declares SIX members with x-enum-varnames
// ["Manga","Comic","Book","Image","LightNovel","ComicVine"]. Both cannot be
// true of the same tree. This code maps what the pinned artefact says, because
// that is the artefact CI can check; the §17.8 note is recorded in
// docs/REVIEW-LOG.md as LS-04 and is NOT resolved here — resolving it needs a
// fresh read of Kavita's source, which is a network fact this pass could not
// verify.
//
// The reading direction is 🔍 INFERENCE and is marked as such wherever it lands.
// SeriesDto reports no reading direction at all; ADR-0030 nevertheless makes
// reading_direction the axis that carries manga-ness ("there is no 'manga' kind
// — manga is reading_direction below plus a derived type:manga system tag"), so
// dropping the LibraryType distinction entirely would lose the one fact Kavita
// does report. A Manga library is read right-to-left by convention and a Comic
// library left-to-right; a webtoon filed in a Manga library is the case this
// gets wrong, and it is wrong in a column the user can correct rather than in
// work.kind, which they cannot.
func mapLibraryType(t kavita.LibraryType) kindDecision {
	switch t {
	case kavita.LibraryTypeManga:
		// Manga → 'comic'. There is no 'manga' kind (ADR-0030) and there is no
		// third level for Kavita's Volume.
		return kindDecision{Kind: "comic", ReadingDirection: "rtl"}

	case kavita.LibraryTypeComic:
		// "Comic (Flexible)" — Kavita's own generic comic library.
		return kindDecision{Kind: "comic", ReadingDirection: "ltr"}

	case kavita.LibraryTypeComicVine:
		// "Comic (ComicVine)" is the SAME kind of content as LibraryTypeComic;
		// the difference is which metadata provider Kavita matches it against,
		// which is Kavita's business and not a UsArr kind. Mapping it to
		// anything else would split one user's comics across two work.kinds and
		// the §6.4 cascade forbids those ever merging.
		return kindDecision{Kind: "comic", ReadingDirection: "ltr"}

	case kavita.LibraryTypeBook:
		// Book → 'book'. Note what this does NOT do: an audiobook is not a kind
		// (it is an edition with format='audiobook', ADR-0031), and Kavita
		// serves no audio, so nothing here can produce one.
		return kindDecision{Kind: "book"}

	case kavita.LibraryTypeLightNovel:
		// 🔍 INFERENCE, and the weakest mapping here. A light novel is prose
		// with illustrations — Kavita ships it as a SEPARATE library type from
		// both Manga and Book, and this pass could not read
		// LibraryType.cs to learn what that separation buys it. 'book' is chosen
		// because the content is prose and 'comic' would put a novel in a
		// comics grid with an issue-contiguity report that can never be
		// satisfied. The cost of being wrong is one editable library kind
		// (§6.5 rule 4: "a library's kind is required and editable"), which is
		// the cheapest place in the design to be wrong.
		return kindDecision{Kind: "book"}

	case kavita.LibraryTypeImage:
		// DECLINED WITH A REASON, which is §17.8's rule — "a container UsArr has
		// no work.kind for is declined with a reason, not silently dropped".
		// library.kind's CHECK is {movie,series,artist,album,book,comic,game};
		// there is no image kind in it and inventing one is a migration on the
		// largest one-way door in the schema.
		return kindDecision{Reason: "Kavita's Image library type has no UsArr kind: " +
			"a library of loose images is neither a book nor a comic, and library.kind " +
			"offers no member for it"}

	default:
		// A member Kavita added after the vendored spec was pinned. It is
		// DECLINED rather than defaulted, because a wrong kind is written once
		// at ingest and the §6.4 cascade then forbids the resulting works from
		// ever merging with the right ones.
		return kindDecision{Reason: fmt.Sprintf(
			"Kavita reported LibraryType %d, which is not a member of the vendored "+
				"api/specs/kavita.json enum (0-5). UsArr declines a container whose kind it "+
				"cannot derive rather than guessing one", int32(t))}
	}
}

// Containers reads Kavita's libraries and decides a kind for each.
func (s *KavitaSource) Containers(ctx context.Context) ([]store.CatalogueContainer, error) {
	libs, err := s.Client.Libraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("kavita: read libraries: %w", err)
	}
	if s.containerKind == nil {
		s.containerKind = map[string]kindDecision{}
	}
	out := make([]store.CatalogueContainer, 0, len(libs))
	for _, l := range libs {
		d := mapLibraryType(l.Type)
		ref := strconv.FormatInt(int64(l.ID), 10)
		s.containerKind[ref] = d
		out = append(out, store.CatalogueContainer{
			RemoteID:      ref,
			Name:          l.Name,
			Kind:          d.Kind,
			DeclineReason: d.Reason,
		})
	}
	return out, nil
}

// StreamItems reads POST /api/Series/all-v2 element by element and hands each
// mapped item to fn.
//
// IT NEVER BUFFERS THE LIST, and that is the reason this method exists rather
// than a ListItems that returns a slice. Kavita's UserParams.PageSize defaults
// to int.MaxValue and coerces an explicit 0 to int.MaxValue as well, so an
// unpaged call returns the WHOLE library in ONE response — reference/sync.md §2
// measures the equivalent *Arr shape at 30-80 MB, peaking at 200-400 MB once
// buffered AND unmarshalled on a 1 GB Pi.
//
// The sort key is LastChapterAdded descending, which is the key ADR-0035 §2a
// verified against a live instance. It buys nothing for a full import, whose
// stop condition is the end of the array — it is sent so that channel 3b, which
// resumes from the same ordering, is reading the same sequence this wrote.
func (s *KavitaSource) StreamItems(ctx context.Context, fn func(store.CatalogueItem) error) (int, error) {
	page, err := s.Client.StreamSeries(ctx, kavita.SeriesListOptions{
		// Ascending is left false — descending, the channel-3b ordering. PageSize
		// is left zero on purpose: Kavita reads that as the whole library in one
		// response, which is precisely channel 1's read and precisely why it is
		// only ever streamed.
		Sort: kavita.SeriesSortFieldLastChapterAdded,
	}, func(dto kavita.SeriesDto) error {
		ref := strconv.FormatInt(int64(dto.LibraryID), 10)
		d, ok := s.containerKind[ref]
		if !ok || d.Kind == "" {
			// A series in a declined or unknown library. Skipped here rather
			// than filtered later so no work row is ever created for it.
			return nil
		}
		return fn(mapSeries(dto, d))
	})
	// page.Count is what StreamSeries handed to the callback, on success and on
	// failure alike — its partial-delivery contract. It is returned either way
	// so the report says how far the read got rather than how many rows are
	// correct.
	return page.Count, err
}

// mapSeries projects one SeriesDto onto the schema.
func mapSeries(dto kavita.SeriesDto, d kindDecision) store.CatalogueItem {
	title := strings.TrimSpace(dto.Name)
	sortTitle := strings.TrimSpace(dto.SortName)
	if sortTitle == "" {
		sortTitle = title
	}

	it := store.CatalogueItem{
		RemoteID:   strconv.FormatInt(int64(dto.ID), 10),
		RemoteKind: "series",

		ContainerID: strconv.FormatInt(int64(dto.LibraryID), 10),
		Kind:        d.Kind,

		Title:           title,
		SortTitle:       sortTitle,
		NormalizedTitle: NormalizeTitle(title),
		NormVersion:     NormVersion,
		OriginalTitle:   strings.TrimSpace(dto.OriginalName),

		// FolderPath is a HOST FILESYSTEM PATH on the Kavita box. It belongs on
		// service_item_link.remote_path — the column that exists to hold what the
		// upstream reported verbatim — and nowhere a browser can read it.
		RemotePath: dto.FolderPath,

		// remote_subtype is "the upstream's own sub-classification", stored
		// verbatim and unparsed (§6.5 rule 3). For Kavita that is MangaFormat,
		// which is a fact about THIS series rather than about its container.
		RemoteSubtype: strconv.FormatInt(int64(dto.Format), 10),

		AddedAt: dto.Created.Time(),

		// LastChapterAddedUtc, never LastChapterAdded. The other is the Kavita
		// server's LOCAL clock, and this is a replica that compares timestamps
		// across machines.
		RemoteUpdatedAt: dto.LastChapterAddedUtc.Time(),

		// Kavita reports no file list at this level, so "has file" is the only
		// file fact available: a series with pages has bytes behind it.
		HasFile: dto.Pages > 0,
	}

	// work.year is LEFT NULL. SeriesDto carries no release year — the year lives
	// on SeriesMetadataDto, which is a per-series call this commit does not make.
	// Deriving one from a title suffix is exactly the parse §6.5 rule 3 forbids.

	switch d.Kind {
	case "book":
		if dto.Pages > 0 {
			it.PageCount = sql.NullInt64{Int64: int64(dto.Pages), Valid: true}
		}
	case "comic":
		// work_comic has NO page_count column — it is the SERIES level, and a
		// page count belongs to an issue. Kavita's Pages is the series total, so
		// it is dropped rather than written somewhere it would be misread.
		if d.ReadingDirection != "" {
			it.ReadingDirection = sql.NullString{String: d.ReadingDirection, Valid: true}
		}
	}

	// Alternate titles. Only the ones Kavita actually reports as DIFFERENT from
	// the title: a localised name equal to the name is not an alias, it is a
	// duplicate that would inflate every FTS document.
	for _, alt := range []struct{ value, kind string }{
		{dto.OriginalName, "original"},
		{dto.LocalizedName, "translated"},
	} {
		v := strings.TrimSpace(alt.value)
		if v == "" || v == title {
			continue
		}
		it.AltTitles = append(it.AltTitles, store.AltTitle{
			Title: v, Normalized: NormalizeTitle(v), Kind: alt.kind,
		})
	}

	it.ExternalIDs = kavitaExternalIDs(dto)
	return it
}

// kavitaExternalIDs projects the Kavita+ identifier fields.
//
// DEGRADED IDENTITY IS THE ORDINARY CASE HERE, NOT AN ERROR. Every one of these
// fields is written only by the Kavita+ match path, so a free instance returns
// 0, null or "" for all of them (ARCHITECTURE.md §6.4, ADR-0035 §1) and this
// function returns an empty slice. The work is still written, still filed into a
// library, still indexed and still searchable; §6.4's "not identified" state is
// then simply the absence of an external_id row.
//
// THREE DECISIONS ABOUT WHICH IDS ARE WRITTEN AND AT WHAT CONFIDENCE:
//
//  1. Confidence 1.0 — a strong, work-level claim that participates in
//     ux_extid_work_strong — for every id below. §6.4's amendment 3 bars a
//     STRONG claim from an identifier "parsed out of a free-text field", and
//     none of these is: they are typed fields Kavita's own matcher wrote, which
//     is the opposite of Komga's user-typed metadata.links[].
//  2. mangaBakaEditionId IS DELIBERATELY NOT WRITTEN. It is an EDITION
//     identifier, and §6.4's amendment 4 is categorical that writing an edition
//     id as a work id "silently claims a paperback and an audiobook are the same
//     edition". external_id's CHECK requires exactly one of work_id/edition_id,
//     this commit writes no edition rows, and the honest answer to "there is
//     nowhere correct to put it" is to not put it anywhere.
//  3. ⚠️ THE ID NAMESPACES ARE NOT VERIFIED TO BE GLOBAL. MyAnimeList numbers
//     anime and manga separately — myanimelist.net/anime/1 and /manga/1 are
//     different works — so 'mal' as a bare source is only unambiguous while
//     every row carrying it comes from a manga/book source, which is true of
//     Kavita and will stop being true the day an anime source lands. It is
//     recorded here rather than pre-solved with a namespaced source string the
//     schema does not list; docs/REVIEW-LOG.md LS-10 carries it.
func kavitaExternalIDs(dto kavita.SeriesDto) []store.ExternalIdentifier {
	var out []store.ExternalIdentifier
	add := func(source, value string) {
		if value == "" || value == "0" {
			return
		}
		out = append(out, store.ExternalIdentifier{Source: source, Value: value, Confidence: 1.0})
	}
	add("anilist", intID(int64(dto.AniListID)))
	add("mal", intID(dto.MalID))
	// 'hardcover_book' rather than 'hardcover': §6.4's amendment 4 names
	// "hardcover_book" as one of exactly three work-strong book sources.
	add("hardcover_book", intID(int64(dto.HardcoverID)))
	add("metron", intID(dto.MetronID))
	add("mangabaka", intID(dto.MangaBakaID))
	add("cbr", intID(int64(dto.CbrID)))
	add("comicvine", strings.TrimSpace(dto.ComicVineID))
	return out
}

func intID(v int64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}
