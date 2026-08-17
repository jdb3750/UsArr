package libsync

import (
	"github.com/jdb3750/UsArr/internal/store"
)

// WebLinkConfidence is the confidence every AniList, MyAnimeList and MangaBaka
// identity claim UsArr derives from Kavita is written at, and like
// ComicVineConfidence it is deliberately below 1.0.
//
// It is the SAME NUMBER as ComicVineConfidence and it is a SEPARATE CONSTANT on
// purpose: the two caps rest on different measurements of different Kavita code
// paths, and collapsing them would let a future correction to one silently move
// the other.
//
// ARCHITECTURE.md §6.4 amendment 3: "Identity parsed out of a free-text field is
// never strong … those get confidence 0.90, never 1.00, because a
// confidence-1.00 write hits ux_extid_work_strong and MERGES WORKS — a mistyped
// link must not be able to do that."
//
// # Why the amendment reaches these three (REVIEW-LOG.md LS-12, closed by LS-27)
//
// Measured against Kavita's source at tag v0.9.0.2 — the owner's version,
// ADR-0035 §2a — by grepping the whole tree for assignments to each field. The
// answer DIFFERS PER FIELD, and it is written out here rather than summarised
// because the differences are the argument:
//
//	Series.AniListId   ProcessSeries.cs:363              free text, unlicensed
//	                   ExternalMetadataService.cs:1106   PROVIDER, licence-gated
//	                   ExternalMetadataIdHelper.cs:13    free text (Edit dialog)
//
//	Series.MalId       ProcessSeries.cs:364              free text, unlicensed
//	                   ExternalMetadataService.cs:1112   PROVIDER, licence-gated
//	                   ExternalMetadataIdHelper.cs:18    free text (Edit dialog)
//
//	Series.MangaBakaId ProcessSeries.cs:366              free text, unlicensed
//	                   ExternalMetadataIdHelper.cs:23    free text (Edit dialog)
//
// So MangaBaka has NO provider writer at all — UpdateExternalIds
// (ExternalMetadataService.cs:1101-1118) writes AniListId and MalId and nothing
// else — while AniList and MAL do have one. That provider path is real: it is
// reached from WriteExternalMetadataToSeries (:532), called at :465 inside a
// flow gated at :249 on `IsPlusEligible(libraryType) && HasActiveLicense()`.
//
// # Why a real provider path still does not buy 1.0
//
// Three reasons, in increasing order of how badly they close the argument:
//
//  1. SeriesDto CARRIES NO PROVENANCE. `aniListId: 30002` on the wire is the
//     same nine bytes whether Kavita+ matched it or the scanner read it out of a
//     ComicInfo.xml <Web> element. UsArr cannot tell, so the WEAKEST writer
//     governs — otherwise the amendment is unenforceable by construction.
//  2. THE FREE-TEXT PATH DOES NOT MERELY COEXIST WITH THE PROVIDER PATH, IT
//     OVERWRITES IT. ProcessSeries.cs:358-367 lives inside UpdateSeriesMetadata,
//     which ProcessSeries.cs:165 calls on EVERY scan of the series, and the
//     assignments are unconditional. A scan after a Kavita+ match replaces the
//     matched id with whatever the first chapter's <Web> holds. So even a value
//     that WAS provider-supplied stops being one, with nothing on the DTO
//     changing to say so.
//  3. THE SAME BLOCK ERASES. `?? 0` on lines 363-364, and GetMangaBakaId's own
//     `?? 0` on line 366, mean a first chapter whose <Web> lacks that site's
//     link sets the field to ZERO. Identity flaps across scans. UsArr's
//     external_id write is an upsert and never a delete, so a wrong id survives
//     the erasure — which is precisely why the cap, not the erasure, has to be
//     the defence.
//
// Point 2 is the one that would survive any future Kavita release note claiming
// these are "Kavita+ fields": the overwrite is in the plain scanner.
//
// # What is NOT done here, and why that is a measurement rather than an omission
//
// comicVineIdentity REFUSES values outright. Nothing here does, and the reason
// is that LS-11's refusal was about a KIND that could not be recovered, not
// about free text as such:
//
//   - ComicVine 4050 (volume) and 4000 (issue) ids come from DIFFERENT number
//     spaces, Kavita strips the prefix that distinguishes them, and with
//     inheritWebLinksFromFirstChapter on the field is filled from a CHAPTER,
//     which is an issue. A bare number was genuinely unknowable.
//   - AniList, MyAnimeList and MangaBaka have NO chapter-level URL space for
//     WeblinkParser to read: its only Series-bound prefixes are
//     "https://anilist.co/manga/", "https://myanimelist.net/manga/" and
//     "https://mangabaka.org/" (WeblinkParser.cs:11-20). A link in a chapter's
//     <Web> element names the SERIES the chapter belongs to. Inheriting it is an
//     accuracy risk when a file is misfiled, not a kind confusion, and 0.90
//     already covers accuracy risk.
//
// There is a sharper consequence of that measurement, and it is the reason
// refusing on the inherit flag would be actively wrong here: at v0.9.0.2 these
// three fields have NO ProcessSeries.cs:162 analogue — no aggregate-over-parsed-
// files writer. THE INHERIT BLOCK IS THE ONLY SCANNER PATH THEY HAVE. Refusing
// on the flag, as ComicVine does, would refuse every AniList, MAL and MangaBaka
// id a free Kavita instance is capable of producing, in a library that is books
// and manga. The cap is the proportionate defence; a refusal is not.
const WebLinkConfidence = 0.90

// The external_id.source strings for the three. `source` is plain TEXT with no
// CHECK (migration 0005), so a namespace is a naming decision rather than a
// migration.
const (
	// AniListSource is BARE, and that is a positive finding rather than a
	// default. LS-10 worried that these namespaces might not be global; for
	// AniList it is. AniList's `Media` id space is SHARED across ANIME and
	// MANGA — one number names one work regardless of type — verified against
	// the live public GraphQL API at https://graphql.anilist.co on 2026-08-17:
	//
	//	Media(id: 1)                 -> {id 1,     type ANIME, "Cowboy Bebop"}
	//	Media(id: 1, type: MANGA)    -> 404 Not Found
	//	Media(id: 30002)             -> {id 30002, type MANGA, "Berserk"}
	//
	// So `anilist=1` cannot mean two things, and namespacing it would invent a
	// distinction the source does not have.
	AniListSource = "anilist"

	// MalMangaSource is NAMESPACED, and this is where LS-10's worry was
	// justified. MyAnimeList numbers anime and manga SEPARATELY, verified
	// first-party on 2026-08-17 by fetching both pages:
	//
	//	https://myanimelist.net/anime/1 -> og:title "Cowboy Bebop"
	//	https://myanimelist.net/manga/1 -> og:title "Monster"
	//
	// One number, two works, two spaces — exactly ComicVine's shape, and a bare
	// 'mal' would let them collide on (source, value).
	//
	// The value is MAL's MANGA space because every writer of Series.MalId puts
	// one there. Kavita says so itself: IHasMetadataIds.cs documents the
	// property as "https://myanimelist.net/store/manga/{MalId}/Blue_Lock", the
	// weblink parser is hard-restricted to the "https://myanimelist.net/manga/"
	// prefix (WeblinkParser.cs:12, :45-48), and Kavita has no anime entity for
	// the Kavita+ path to match against. The one writer that could violate it is
	// the Edit Series dialog, which validates `is > 0` and nothing else
	// (ExternalMetadataIdHelper.cs:16-19) — and at 0.90 a user who pastes an
	// anime id gets a wrong row that cannot merge anything.
	MalMangaSource = "mal_manga"

	// MangaBakaSource is BARE. MangaBaka's series URL is the numeric site root —
	// https://mangabaka.org/3391 resolves to a series page directly, checked
	// 2026-08-17 — which is exactly the shape WeblinkParser.cs:20,:88-91 reads,
	// and Kavita pins it in WeblinkParserTests.cs:32.
	//
	// The adjacent space is `mangaBakaEditionId`, and the DTO keeps it in a
	// SEPARATE FIELD, so nothing erases the distinction the way comicVineId
	// erases 4050/4000. UsArr never writes that field (kavitaExternalIDs,
	// decision 3), so nothing can collide with this source today. ⚠️ A future
	// commit that does write it must NOT reuse this string.
	MangaBakaSource = "mangabaka"
)

// webLinkIdentity turns one weblink-parseable SeriesDto id field into an
// external_id row, or into nothing.
//
// It exists as a named function rather than inline in kavitaExternalIDs for one
// reason: kavitaExternalIDs's `add` closure hard-codes confidence 1.0, and
// routing these three through a call that CANNOT produce 1.0 is what makes the
// cap structural instead of a number three call sites have to remember.
//
// value is the already-formatted decimal id; "" and "0" are the ordinary
// unidentified case (ARCHITECTURE.md §6.4), not an error and not worth a reason.
func webLinkIdentity(source, value string) (store.ExternalIdentifier, bool) {
	if value == "" || value == "0" {
		return store.ExternalIdentifier{}, false
	}
	return store.ExternalIdentifier{
		Source: source, Value: value, Confidence: WebLinkConfidence,
	}, true
}
