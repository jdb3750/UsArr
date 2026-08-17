package libsync

import (
	"github.com/jdb3750/UsArr/internal/store"
)

// EditableIDConfidence is the confidence every Metron and ComicBookRoundup
// identity claim UsArr derives from Kavita is written at, and like
// ComicVineConfidence and WebLinkConfidence it is deliberately below 1.0.
//
// ARCHITECTURE.md §6.4 amendment 3: "Identity parsed out of a free-text field is
// never strong … those get confidence 0.90, never 1.00, because a
// confidence-1.00 write hits ux_extid_work_strong and MERGES WORKS — a mistyped
// link must not be able to do that."
//
// # Why ONE constant here where ComicVine and the weblink three got two
//
// weblinkid.go says the two existing caps are separate constants "on purpose:
// the two caps rest on different measurements of different Kavita code paths."
// That reasoning is kept, and it lands differently here: metronId and cbrId rest
// on ONE measurement — THE EDIT-SERIES DIALOG REACHES BOTH, AND SeriesDto
// CARRIES NO PROVENANCE FOR EITHER — so the weakest writer governs both for the
// same reason. ⚠️ IF A FUTURE MEASUREMENT SEPARATES THEM, THIS CONSTANT SPLITS
// RATHER THAN MOVES. Their writer sets already differ (Metron has no provider
// writer at any ref; ComicBookRoundup has one on develop), and it is only
// because the free-text writer governs both that one number is honest.
//
// # metronId — every writer, at tag v0.9.0.2 (the owner's version, ADR-0035 §2a)
//
// A whole-tree grep for `MetronId` over the v0.9.0.2 tree, migrations excluded,
// finds exactly TWO assignment sites and they are at DIFFERENT LEVELS:
//
//	Kavita.Services/Scanner/ProcessSeries.cs:749   chapter.MetronId = info.MetronId ?? 0;
//	Kavita.Server/Helpers/ExternalMetadataIdHelper.cs:31-33  entity.MetronId = dto.MetronId.Value;
//
// 🚩 NOTHING ON ANY SCAN PATH WRITES Series.MetronId. The scanner's Metron id is
// parsed out of ComicInfo.xml's <Notes> element (Parser.cs:1350 via
// DefaultParser.cs:178-181) and lands on the CHAPTER. There is no
// ProcessSeries.cs:162 analogue for Metron — no Distinct()-over-parsed-files
// aggregate that promotes a chapter id to the series — so the only writer of the
// field UsArr reads is SetExternalMetadataIds, reached from SeriesController.cs:188
// with the user's own UpdateSeriesDto.
//
// # THE HANDED-OVER PREMISE IS TRUE AND ITS CONSEQUENCE IS NOT (REVIEW-LOG LS-70)
//
// This correction was commissioned on the reading that "Metron's only populated
// level is the chapter — one level below the work", i.e. §6.4 amendment 4's
// shape, i.e. LS-14's outright refusal. THE FIRST HALF IS EXACTLY RIGHT ABOUT
// THE SCANNER AND THE SECOND HALF DOES NOT FOLLOW, because the chapter id and
// the field UsArr reads are written by different code and never meet:
//
//	<Notes> --parse--> ParserInfo.MetronId --> Chapter.MetronId --> ChapterDto.metronId
//	                                                                ^^^^^^^^^^^^^^^^^^
//	                                                                UsArr NEVER READS THIS
//	Edit Series dialog --> UpdateSeriesDto.MetronId --> Series.MetronId --> SeriesDto.metronId
//	                                                                       ^^^^^^^^^^^^^^^^^^
//	                                                                       the only field read
//
// Kavita never promotes the issue id into the series field, so amendment 4 —
// which bars an id from a level BELOW the work — has nothing to bite on. What is
// left is amendment 3's hazard and only that: a hand-typed number, at 0.90.
//
// ⚠️ THE RESIDUAL RISK IS A USER PASTING AN ISSUE ID INTO THE SERIES BOX, and it
// is real, because the form gives them nothing to go on. The Edit modal renders
// one bare numeric input per id from a SHARED component used at series, volume
// and chapter level (edit-external-metadata-form.component.html), and the label
// is the untyped "Metron Id" (UI/Web/src/assets/langs/en.json). Metron's `Issue`
// and `Series` are separate Django models with separate auto primary keys
// (Metron-Project/metron comicsdb/models/{issue,series}.py, checked 2026-08-17),
// so issue 42 and series 42 are different objects — ComicVine's 4050/4000 hazard
// with NO prefix to distinguish them. That is an ACCURACY risk, which 0.90 is
// exactly the defence against, and it is why MetronSeriesSource is namespaced.
//
// # cbrId — what it is, and every writer, at BOTH refs
//
// `cbr` is COMIC BOOK ROUNDUP (comicbookroundup.com), not the CBR archive
// format — see ComicBookRoundupSource for why that mattered enough to rename.
// It is Kavita+'s third metadata provider (MetadataProvider.ComicBookRoundup = 4,
// Kavita.Models/Entities/Enums/MetadataProvider.cs at develop 9c3e540).
//
// It is SERIES-LEVEL, which is the question amendment 4 turns on, and three
// independent readings agree:
//
//   - KavitaPlusConfiguration.MetadataProvidersForLibraryTypes offers it for the
//     Comic, Image and ComicVine LIBRARY types, and matching runs per SERIES —
//     MatchSeriesV3Async returns match.Series.CbrId (ExternalMetadataService.cs:205-223).
//   - HasRequiredId gates the whole provider on `series.CbrId > 0` (:264).
//   - Kavita's own link builder is
//     `https://comicbookroundup.com/comic-books/reviews/${id}`
//     (UI/Web/src/app/shared/utils/provider-url.util.ts:8). On that site
//     /comic-books/reviews/{publisher}/{series} is a SERIES page and the issue
//     page is one segment deeper (fetched 2026-08-17).
//
// So amendment 4 does not reach it either. Its writers at develop 9c3e540:
//
//	ExternalMetadataService.cs:223      series.CbrId = match.Series.CbrId ?? 0;   PROVIDER, licence-gated
//	ExternalMetadataService.cs:1773-75  series.CbrId = externalMetadata.CbrId;    PROVIDER, licence-gated
//	ExternalMetadataIdHelper.cs:15      entity.CbrId = dto.CbrId ?? 0;            free text (Edit dialog)
//
// There is NO scanner writer at either ref: no <Web> prefix, no <Notes> regex,
// nothing in Kavita.Services/Scanner. So unlike aniListId this field is never
// overwritten by a rescan — and it is capped anyway, on weblinkid.go's reason 1
// standing alone: SeriesDto CARRIES NO PROVENANCE, so UsArr cannot tell a
// Kavita+ match from a typed number, and the weakest writer has to govern or the
// amendment is unenforceable by construction.
//
// 🚩 AND THE FREE-TEXT WRITER IS STRICTLY WORSE ON DEVELOP THAN AT THE TAG.
// ExternalMetadataIdHelper at v0.9.0.2 guards every assignment with `is > 0`;
// develop's rewrites all six as `dto.X ?? 0` unconditionally. So on the very
// releases where cbrId is reachable, saving the Edit Series dialog OVERWRITES a
// Kavita+-matched id with whatever is in the form, INCLUDING CLEARING IT TO
// ZERO. A provider-supplied value there is not merely indistinguishable from a
// typed one; it stops being provider-supplied the moment anyone opens the modal.
//
// # Was refusing considered
//
// Yes, for Metron, and it is rejected on measurement rather than on cost.
// LS-11 refused two ComicVine shapes and both refusals rest on something absent
// here: a value whose text PROVES it names an issue (LS-14), or a value Kavita
// ITSELF filled from a chapter after erasing the discriminator (LS-13). Neither
// applies — `SeriesDto.metronId` is series-level by construction and Kavita never
// corrupts it — while refusing would refuse 100% of what the field can produce,
// since it has exactly one writer. comicvine.go's own test for that ("there is
// no source string under which a value of unknown kind is a true statement")
// fails here in the other direction: `metron_series` IS a true statement about
// every value this writer can produce. The cap is the proportionate defence.
const EditableIDConfidence = 0.90

// The external_id.source strings for the two. `source` is plain TEXT with no
// CHECK (migration 0005:448-454), so a namespace is a naming decision rather
// than a migration.
const (
	// MetronSeriesSource is NAMESPACED, for ComicVineVolumeSource's reason
	// measured on Metron rather than assumed by symmetry — LS-39 kept `anilist`
	// and `mangabaka` bare and the difference is the number spaces.
	//
	// Metron has TWO numeric spaces and UsArr is one phase from holding both.
	// `Chapter.MetronId` is an ISSUE id — metron-tagger writes it as the literal
	// token `[issue_id:{issue_id}]` (metrontagger/talker.py create_notes(),
	// fetched 2026-08-17) and Kavita's regex names it the same way
	// (Parser.cs:725, `MetronTagger-.*\[issue_id:(?<Id>\d+)\]`) — and it reaches
	// ChapterDto.metronId, which doc.go's phase-B chapter walk will decode. Under
	// a bare `metron`, Metron series 42 and Metron issue 42 would collide on
	// (source, value), which is precisely what ux_extid and ux_extid_work_strong
	// are keyed on.
	//
	// ⚠️ A COMMIT THAT WRITES THE CHAPTER ID MUST NOT REUSE THIS STRING. It is
	// an issue id, one level below the work, and §6.4 amendment 4 bars it from a
	// work row at ANY confidence — the rule LS-14 applied to ComicVine 4000.
	MetronSeriesSource = "metron_series"

	// ComicBookRoundupSource is RENAMED from the bare `cbr` this line shipped
	// with, and the rename settles ADR-0046's second open question: "external_id
	// .source = 'cbr' is not enumerated anywhere as a legal source … the `cbr`
	// that does appear in 00005_library_sync.sql is edition.format's CHECK, a
	// different thing entirely."
	//
	// 🚩 THAT IS A COLLISION INSIDE UsArr'S OWN VOCABULARY, not a gap in a
	// comment. In this schema `cbr` ALREADY MEANS the Comic Book RAR archive
	// format — it is one of edition.format's CHECK values and
	// internal/store/recent.go:390 lists it beside 'cbz' and 'pdf'. One token,
	// two unrelated meanings, one database. `comicbookroundup` is the site's own
	// name and cannot be read as a file extension.
	//
	// Renaming is free EXACTLY NOW and never again: cbrId is absent from
	// SeriesDto at v0.9.0.2 (ADR-0046's floor; internal/kavita/contract_test.go's
	// ceilingOnlyProperties machine-checks it), so on the owner's install the
	// field decodes to zero and no `cbr` row has ever been written. There is
	// nothing to migrate, and a rename after the first develop user syncs would
	// need one.
	//
	// The space is Kavita+'s own surrogate key rather than a comicbookroundup.com
	// path: the site is slug-addressed and Kavita's search path carries CBR
	// identity as a slug (ExternalMetadataService.cs:415, `potentialCbrSlug`),
	// while cbrId is an int minted by the Kavita+ backend. One space, so no
	// second namespace is needed — the rename is about UsArr's collision only.
	ComicBookRoundupSource = "comicbookroundup"
)

// editableIdentity turns one hand-editable SeriesDto id field into an
// external_id row, or into nothing.
//
// It exists as a named function rather than inline in kavitaExternalIDs for
// webLinkIdentity's reason, which is the whole mechanism of this fix:
// kavitaExternalIDs's `add` closure hard-codes confidence 1.0, and BOTH THESE
// FIELDS WERE WIRED TO IT. Routing them through a call that CANNOT return 1.0
// is what makes the cap structural instead of a number two call sites have to
// remember.
//
// value is the already-formatted decimal id; "" and "0" are the ordinary
// unidentified case (ARCHITECTURE.md §6.4), not an error and not worth a reason.
func editableIdentity(source, value string) (store.ExternalIdentifier, bool) {
	if value == "" || value == "0" {
		return store.ExternalIdentifier{}, false
	}
	return store.ExternalIdentifier{
		Source: source, Value: value, Confidence: EditableIDConfidence,
	}, true
}
