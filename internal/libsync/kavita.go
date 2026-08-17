package libsync

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
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

	// SeriesMetadata is the credit path (ADR-0044). It is on this interface and
	// not on a second one because a Kavita client that could stream series but
	// not read one series' metadata does not exist — both are the same
	// controller and the same credential.
	SeriesMetadata(ctx context.Context, seriesID int32) (kavita.SeriesMetadataDto, error)
}

// KavitaSource adapts one Kavita instance to Source.
type KavitaSource struct {
	Client KavitaReader

	// Log is optional and defaults to discard. It is where a REFUSED identity
	// claim goes — see StreamItems.
	Log *slog.Logger

	// containerKind remembers each library's declared LibraryType and its
	// inheritWebLinksFromFirstChapter setting between the Containers call and
	// the item stream, because SeriesDto reports libraryId and NOT the library's
	// type — and work.kind comes from the type.
	containerKind map[string]kindDecision
}

// NewKavitaSource wraps a client.
func NewKavitaSource(c KavitaReader) *KavitaSource {
	return &KavitaSource{Client: c, containerKind: map[string]kindDecision{}}
}

func (s *KavitaSource) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.DiscardHandler)
}

// kindDecision is what one LibraryType resolves to.
type kindDecision struct {
	// Kind is a work.kind, or "" when UsArr has no kind for this container.
	Kind string

	// ReadingDirection is work_comic.reading_direction, or "" for none.
	ReadingDirection string

	// Reason is why a container was declined. Empty when Kind is non-empty.
	Reason string

	// InheritsWebLinks is LibraryDto.inheritWebLinksFromFirstChapter, carried
	// from the container to every series in it because it POISONS
	// SeriesDto.comicVineId for the whole library — see comicVineIdentity.
	//
	// It is NOT part of mapLibraryType's output: it is a per-library setting,
	// not a property of the LibraryType, and Containers() writes it separately.
	InheritsWebLinks bool
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
// ✅ LS-04 IS SETTLED, AND NOTHING DISAGREES ANY MORE. THE ENUM HAS SIX MEMBERS
// AND Image = 3 IS ONE OF THEM. The vendored spec and ARCHITECTURE.md §17.8 now
// say the same thing: §17.8 carried a withdrawal of the Image example, and its
// 2026-08-17 amendment withdraws the withdrawal on the same measurement this
// mapping was built on — at tag v0.9.0.2,
// Kavita.Models/Entities/Enums/LibraryType.cs declares Manga = 0, Comic = 1,
// Book = 2, Image = 3, LightNovel = 4, ComicVine = 5, byte-identical at develop
// (9c3e5400), and Image has shipped since tag v0.7.11 (2023-12-03).
//
// THE GENERAL LESSON IS THE PART WORTH KEEPING, because it will bite again: A
// KAVITA CITATION WITH AN `API/Entities/…` PATH IS READING THE FROZEN main, NOT
// THE SHIPPED CODE. Kavita's main is pinned at the v0.7.8 release commit
// (97950804, 2023-09-03) while its release line is develop plus tags, and at
// main that path really does declare only three members — which is why the
// withdrawal was reproducible rather than careless. Read a TAG or develop.
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
		// 'book', and no longer a blind inference: LibraryType.cs at v0.9.0.2
		// documents the member as "Allows Books to Scrobble with AniList for
		// Kavita+". The separation from Book buys Kavita a SCROBBLING
		// capability, not a different content model — the content is prose
		// either way — so 'book' is what it maps to, and 'comic' would put a
		// novel in a comics grid with an issue-contiguity report that can never
		// be satisfied. Should this ever be wrong it is wrong in one editable
		// library kind (§6.5 rule 4: "a library's kind is required and
		// editable"), the cheapest place in the design to be wrong.
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
		// The flag is read HERE, from the library list this call already makes,
		// so knowing it costs no extra request. It is exposed by the vendored
		// spec (LibraryDto.inheritWebLinksFromFirstChapter, api/specs/kavita.json)
		// and modelled in internal/kavita/resources.go.
		d.InheritsWebLinks = l.InheritWebLinksFromFirstChapter
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
		// A refused ComicVine id is LOGGED, not swallowed. Principle 3 —
		// "every feature degrades honestly … it says what is missing and why" —
		// and this is the one place in the mapping that silently drops an
		// identity claim on purpose.
		if _, reason, _ := comicVineIdentity(dto.ComicVineID, d.InheritsWebLinks); reason != "" {
			s.log().Info("kavita: refused a ComicVine identity claim",
				"series_id", dto.ID, "series", dto.Name, "library_id", dto.LibraryID,
				"comic_vine_id", dto.ComicVineID,
				"inherit_web_links_from_first_chapter", d.InheritsWebLinks,
				"reason", reason)
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

	it.ExternalIDs = kavitaExternalIDs(dto, d)
	return it
}

// kavitaExternalIDs projects SeriesDto's identifier fields.
//
// ⚠️ THE PREMISE THIS FUNCTION WAS ORIGINALLY WRITTEN ON WAS WRONG, AND THE
// CORRECTION IS THE REASON comicVineIdentity EXISTS. It read: "every one of
// these fields is written only by the Kavita+ match path, so a free instance
// returns 0, null or ” for all of them." That is FALSE FOR TAGGED COMICS.
// Re-verified 2026-08-17 by reading Kavita's source at tag v0.9.0.2 — the
// owner's version, ADR-0035 §2a — where a whole-tree grep for `ComicVineId`
// finds three writers of `Series.ComicVineId` and NOT ONE is behind a licence
// check:
//
//	ProcessSeries.cs:162  series.ComicVineId = comicVineSeriesIds[0];
//	ProcessSeries.cs:365  series.ComicVineId = WeblinkParser.GetComicVineId(...).Item1;
//	ExternalMetadataIdHelper.cs:38  entity.ComicVineId = dto.ComicVineId;
//
// The first two are the PLAIN SCANNER reading ComicInfo.xml's <Web> element,
// which ComicTagger and Mylar both populate; the third is the Edit Series
// dialog. At v0.9.0.2 KAVITA+ DOES NOT WRITE Series.ComicVineId AT ALL.
//
// DEGRADED IDENTITY IS STILL THE ORDINARY CASE for books and manga, and still
// not an error. A free instance with untagged files returns 0, null or "" for
// every field here, this function returns an empty slice, and the work is still
// written, filed, indexed and searchable; §6.4's "not identified" state is then
// simply the absence of an external_id row.
//
// FIVE DECISIONS ABOUT WHICH IDS ARE WRITTEN AND AT WHAT CONFIDENCE:
//
//  1. ✅ LS-12 IS CLOSED, AND THE ANSWER DIFFERED PER FIELD. It read: "the same
//     re-verification found that `Series.AniListId`, `MalId` and `MangaBakaId`
//     are ALSO weblink-parsed at v0.9.0.2 (ProcessSeries.cs:363-366) rather than
//     matcher-written, which means §6.4 amendment 3 arguably reaches them too."
//     Re-measured writer by writer (LS-27): it does reach all three, they now go
//     through webLinkIdentity at WebLinkConfidence (0.90), and weblinkid.go's
//     comment owns the measurement — including the two places the answer is NOT
//     ComicVine's. MangaBaka has no provider writer at all; AniList and MAL each
//     have a real, licence-gated one and still do not earn 1.0.
//  2. ✅ LS-10 IS CLOSED TOO, because it is the same line of code. 'mal' is no
//     longer bare: MyAnimeList really does number anime and manga separately
//     (verified first-party — myanimelist.net/anime/1 is "Cowboy Bebop" and
//     /manga/1 is "Monster"), so it lands under MalMangaSource. AniList's id
//     space really IS global, so AniListSource stays bare. LS-10 could only
//     guess at both; both are now measured, and weblinkid.go cites each.
//  3. THE ComicVine ID IS NEVER WRITTEN AT 1.0, and is written only when its
//     KIND IS PROVEN. comicVineIdentity owns that; its comment owns the why.
//  4. mangaBakaEditionId IS DELIBERATELY NOT WRITTEN. It is an EDITION
//     identifier, and §6.4's amendment 4 is categorical that writing an edition
//     id as a work id "silently claims a paperback and an audiobook are the same
//     edition". external_id's CHECK requires exactly one of work_id/edition_id,
//     this commit writes no edition rows, and the honest answer to "there is
//     nowhere correct to put it" is to not put it anywhere. It is also
//     develop-only: Kavita.Models/Entities/Series.cs:120-125 at v0.9.0.2
//     declares AniListId, MalId, HardcoverId, MetronId, ComicVineId and
//     MangaBakaId — and NO MangaBakaEditionId and no CbrId. Both are modelled on
//     the vendored develop spec and simply absent on the owner's instance.
//  5. ⚠️ 'hardcover_book' AND 'metron' STILL GO OUT AT 1.0, AND PROBABLY SHOULD
//     NOT. The same whole-tree grep found Series.HardcoverId written by
//     ExternalMetadataIdHelper.cs:28 ALONE and Series.MetronId by
//     ExternalMetadataIdHelper.cs:33 alone — the Edit Series dialog, i.e. free
//     text and nothing else, with neither a scanner nor a provider writer. That
//     is a WEAKER provenance than the three capped above, and the last thing
//     going out at 1.0 from here on the strength of an unexamined premise. It is
//     not changed in this commit for exactly the reason LS-12 was not changed
//     inside LS-11 — different fields, different fixtures, and a third behaviour
//     bundled in would make none of them reviewable. docs/REVIEW-LOG.md LS-32
//     carries it with the measurement.
func kavitaExternalIDs(dto kavita.SeriesDto, d kindDecision) []store.ExternalIdentifier {
	var out []store.ExternalIdentifier
	add := func(source, value string) {
		if value == "" || value == "0" {
			return
		}
		out = append(out, store.ExternalIdentifier{Source: source, Value: value, Confidence: 1.0})
	}
	// 'hardcover_book' rather than 'hardcover': §6.4's amendment 4 names
	// "hardcover_book" as one of exactly three work-strong book sources.
	add("hardcover_book", intID(int64(dto.HardcoverID)))
	add("metron", intID(dto.MetronID))
	add("cbr", intID(int64(dto.CbrID)))

	// The weblink-parsed three do NOT go through add(): add() hard-codes
	// confidence 1.0 and §6.4 amendment 3 forbids any of these reaching it.
	// Routing them through a call that CANNOT return 1.0 is what makes the cap
	// structural rather than three call sites remembering a number.
	for _, w := range []struct{ source, value string }{
		{AniListSource, intID(int64(dto.AniListID))},
		{MalMangaSource, intID(dto.MalID)},
		{MangaBakaSource, intID(dto.MangaBakaID)},
	} {
		if id, ok := webLinkIdentity(w.source, w.value); ok {
			out = append(out, id)
		}
	}

	// ComicVine does not go through add() either — and unlike the three above it
	// can be REFUSED outright rather than merely weakened, because its KIND can
	// be unknowable. weblinkid.go's comment records why that refusal does not
	// transfer to AniList, MAL or MangaBaka.
	if id, _, ok := comicVineIdentity(dto.ComicVineID, d.InheritsWebLinks); ok {
		out = append(out, id)
	}
	return out
}

// comicVineIdentity decides what, if anything, one SeriesDto.comicVineId value
// may claim about the work. It returns the identifier, or the reason it was
// refused.
//
// # The trap this function exists to close
//
// An external_id row at confidence 1.0 satisfies ux_extid_work_strong, and in
// UsArr that index IS THE MERGE SIGNAL — store.ApplyCatalogueBatch reuses the
// work that already holds the id, which makes two works one. v0.1 has no
// work_merge table and no un-merge. So a ComicVine id of the WRONG KIND written
// strongly is an unrecoverable, silent merge of unrelated works.
//
// `comicVineId` is exactly that hazard, for three separate reasons, all read off
// Kavita v0.9.0.2's source:
//
//  1. THE KIND IS ERASED. WeblinkParser.cs:55-61 returns
//     `extractedId.Split('-')[1]` — the id with its 4050 (volume) / 4000 (issue)
//     prefix REMOVED — and the surviving `Item2` bool is the only discriminator.
//     4050 and 4000 ids come from DIFFERENT number spaces, so "159233" alone
//     does not say which object it names. Kavita's own test pins the erasure:
//     WeblinkParserTests.cs:24 expects ".../4000-159233/" -> "159233".
//  2. THE FIELD IS NULL ON A ComicTagger-ONLY LIBRARY. ProcessSeries.cs:159-163
//     fills it from ParserInfo.ComicVineSeriesId, and DefaultParser.cs:169-172
//     sets THAT only when the parsed link was a 4050 one. A library tagged with
//     issue links only leaves series.ComicVineId empty.
//  3. 🚩 WITH inheritWebLinksFromFirstChapter ON, IT IS SILENTLY FILLED WITH THE
//     FIRST CHAPTER'S ISSUE ID. ProcessSeries.cs:359-366 runs inside
//     UpdateSeriesMetadata, which ProcessSeries.cs:165 calls AFTER the
//     line-162 assignment, and it writes `GetComicVineId(...).Item1`
//     unconditionally — discarding Item2. So the flag does not merely add a
//     value, it OVERWRITES a correct volume id with a first chapter's issue id,
//     and the DTO carries no discriminator to tell them apart.
//
// # What this function does
//
// The order is: prove the kind, then decide.
//
//   - A value that still carries the discriminator — a full ComicVine URL, or a
//     bare "4050-112794" token, both of which reach the field through the Edit
//     Series dialog, which validates nothing (ExternalMetadataIdHelper.cs:36-38)
//     — is CLASSIFIED. A volume id is written; an issue id is REFUSED, because
//     an issue is not the work.
//   - A bare number with the library's inherit flag OFF is a VOLUME id: the only
//     scanner path that can have written it is ProcessSeries.cs:162, fed by
//     DefaultParser.cs:169-172's 4050-only branch.
//   - A bare number with the library's inherit flag ON is REFUSED. Its kind is
//     unknowable and it may be an issue id.
//
// Everything written lands at ComicVineConfidence (0.90), never 1.0, per §6.4
// amendment 3 — Kavita derived every one of these from a free-text field, so
// none of them may merge works. That cap is what makes the inherit case a
// belt-and-braces refusal rather than the only defence.
func comicVineIdentity(raw string, inheritsWebLinks bool) (store.ExternalIdentifier, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		// The ordinary untagged case. Not a refusal and not worth a reason.
		return store.ExternalIdentifier{}, "", false
	}

	volume := func(id string) (store.ExternalIdentifier, string, bool) {
		return store.ExternalIdentifier{
			Source: ComicVineVolumeSource, Value: id, Confidence: ComicVineConfidence,
		}, "", true
	}

	if refs := ParseComicVineWebLinks(raw); len(refs) > 0 {
		if v, ok := VolumeRef(refs); ok {
			return volume(v.ID)
		}
		return store.ExternalIdentifier{}, "comicVineId carries a ComicVine ISSUE (4000) link and no " +
			"volume (4050) one; an issue is one level below the work and is never written as a work id", false
	}

	if !comicVineBareNumber.MatchString(raw) {
		return store.ExternalIdentifier{}, "comicVineId is neither a ComicVine link nor a bare id " +
			"(" + raw + "); Kavita stores this field verbatim from the Edit Series dialog and " +
			"UsArr will not guess at a shape it does not recognise", false
	}

	if inheritsWebLinks {
		return store.ExternalIdentifier{}, "the library has inheritWebLinksFromFirstChapter enabled, so " +
			"Kavita overwrites series.ComicVineId with the FIRST CHAPTER's id and discards the " +
			"4050/4000 discriminator (ProcessSeries.cs:365); a bare id from such a library may be " +
			"an issue id and its kind cannot be recovered from the DTO", false
	}

	return volume(raw)
}

// comicVineBareNumber matches the shape Kavita's scanner leaves behind once
// WeblinkParser has stripped the 4050/4000 prefix.
var comicVineBareNumber = regexp.MustCompile(`^\d+$`)

func intID(v int64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}
