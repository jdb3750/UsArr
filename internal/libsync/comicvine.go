package libsync

import (
	"net/url"
	"regexp"
	"strings"
)

// ComicVineConfidence is the confidence EVERY ComicVine identity claim UsArr
// derives from Kavita is written at, and it is deliberately below 1.0.
//
// ARCHITECTURE.md §6.4 amendment 3: "Identity parsed out of a free-text field is
// never strong … those get confidence 0.90, never 1.00, because a
// confidence-1.00 write hits ux_extid_work_strong and MERGES WORKS — a mistyped
// link must not be able to do that."
//
// Every ComicVine id Kavita can report is exactly that. Verified against
// Kavita's source at tag v0.9.0.2 — the owner's version (ADR-0035 §2a) — where
// `Series.ComicVineId` has exactly THREE writers in the whole tree and not one
// of them is a licensed matcher:
//
//	Kavita.Services/Scanner/ProcessSeries.cs:162
//	Kavita.Services/Scanner/ProcessSeries.cs:365
//	Kavita.Server/Helpers/ExternalMetadataIdHelper.cs:38
//
// The first two are the PLAIN SCANNER reading ComicInfo.xml's <Web> element; the
// third is whatever a user typed into the Edit Series dialog, stored after a
// whitespace check and nothing else. Free text, all three.
//
// v0.1 has no work_merge table and no un-merge, so a wrong strong claim is a
// one-way door. 0.90 keeps every ComicVine row out of ux_extid_work_strong
// (UNIQUE(source, value) WHERE work_id IS NOT NULL AND confidence >= 1.0) and
// out of the tier-1 reuse lookup in store.ApplyCatalogueBatch, which skips
// anything below 1.0.
const ComicVineConfidence = 0.90

// ComicVineVolumeSource is the external_id.source for a ComicVine VOLUME (4050)
// id — ComicVine's own word for what UsArr calls a comic work.
//
// It is namespaced rather than a bare "comicvine" precisely because
// SeriesDto.comicVineId erases the kind: 4050 and 4000 ids are drawn from
// DIFFERENT ComicVine number spaces, so storing both under one source string
// would let volume 159233 and issue 159233 collide on (source, value).
const ComicVineVolumeSource = "comicvine_volume"

// ComicVineKind is which ComicVine object an id names.
//
// ComicVine's URLs carry the kind as a numeric prefix on the id segment, and
// Kavita's own source documents the mapping
// (Kavita.Common/Helpers/WeblinkParser.cs:23-31, v0.9.0.2):
//
//	"The 4050 implies this is a Series (TPB/Series) and 4000 implies single issue"
//	https://comicvine.gamespot.com/batman-the-caped-crusader/4050-112794/       (Series)
//	https://comicvine.gamespot.com/batman-the-caped-crusader-6-volume-6/4000-907546/ (Issue)
type ComicVineKind string

const (
	// ComicVineVolume is a 4050 id. ComicVine's "volume" is a whole series, and
	// it is the ONLY ComicVine id shape that names a UsArr comic work.
	ComicVineVolume ComicVineKind = "volume"

	// ComicVineIssue is a 4000 id: ONE ISSUE. It is never written against a
	// work. §6.4 amendment 4 refuses an ISBN as a work id for the same reason —
	// an identifier one level below the work "silently claims" two different
	// things are the same, and this one is worse than an ISBN because the
	// collision is across number spaces rather than within one.
	ComicVineIssue ComicVineKind = "issue"
)

// ComicVineRef is one ComicVine identifier whose KIND IS PROVEN, because the
// text it came from still carried the 4050/4000 discriminator.
type ComicVineRef struct {
	Kind ComicVineKind
	ID   string
}

// comicVineHosts are the hosts a ComicVine link may use. Kavita matches the
// literal prefix "https://comicvine.gamespot.com/" and nothing else; UsArr also
// accepts http and a www. prefix, because the string here may have been pasted
// by a user rather than written by ComicTagger.
var comicVineHosts = map[string]bool{
	"comicvine.gamespot.com":     true,
	"www.comicvine.gamespot.com": true,
}

// comicVineIDSegment matches the "4050-112794" / "4000-907546" path segment.
//
// It is anchored to a whole path segment on both sides so that a slug which
// happens to contain the digits — /some-4050-thing/4000-1/ — cannot be read as
// the id.
var comicVineIDSegment = regexp.MustCompile(`(?:^|/)(4050|4000)-(\d+)(?:/|$)`)

// comicVineBareToken matches an UNLINKED "4050-112794" token: the shape a user
// gets by copying the id out of a ComicVine URL without the rest of it.
var comicVineBareToken = regexp.MustCompile(`^(4050|4000)-(\d+)$`)

// ParseComicVineWebLinks extracts every ComicVine reference from a raw webLinks
// string, PRESERVING THE VOLUME/ISSUE DISTINCTION.
//
// # Why this exists rather than a read of SeriesDto.comicVineId
//
// Kavita throws the distinction away. Kavita.Common/Helpers/WeblinkParser.cs:55-61
// at v0.9.0.2 reads, verbatim:
//
//	var extractedId = ExtractId<string?>(weblinks, ComicVineWeblinkWebsite);
//	if (string.IsNullOrEmpty(extractedId)) return Tuple.Create<string?, bool>(null, false);
//	return Tuple.Create<string?, bool>(extractedId.Split('-')[1], extractedId.StartsWith("4050"));
//
// `.Split('-')[1]` is the id with the 4050/4000 PREFIX REMOVED, and Kavita's own
// unit test pins it: Kavita.Common.Tests/Helpers/WeblinkParserTests.cs:24 asserts
// that ".../chew-1-taster-s-choice-part-1-of-5/4000-159233/" yields "159233".
// The Item2 bool is the only surviving discriminator, and ProcessSeries.cs:365
// DISCARDS IT. So `comicVineId` on the wire is a bare number whose number space
// is unrecoverable — while the URL it was parsed from stated it plainly.
//
// # Tokenisation
//
// Kavita stores chapter.WebLinks comma-joined: ProcessSeries.cs:886-893 splits
// ComicInfo.xml's <Web> on ',' when a comma is present and on ' ' otherwise, then
// rejoins with ','. This function splits on BOTH, which is a superset of what
// Kavita produces and also survives a raw <Web> value that never went through
// Kavita.
//
// Unparseable tokens are skipped rather than reported: a webLinks field
// routinely carries AniList, MAL and MangaBaka links too, and those are not
// errors here.
func ParseComicVineWebLinks(raw string) []ComicVineRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var out []ComicVineRef
	seen := map[ComicVineRef]bool{}
	appendRef := func(r ComicVineRef) {
		if seen[r] {
			return
		}
		seen[r] = true
		out = append(out, r)
	}

	for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}

		// The unlinked "4050-112794" form first: it is not a URL and url.Parse
		// would happily accept it as a relative reference.
		if m := comicVineBareToken.FindStringSubmatch(tok); m != nil {
			appendRef(comicVineRef(m[1], m[2]))
			continue
		}

		u, err := url.Parse(tok)
		if err != nil || u.Host == "" {
			continue
		}
		if !comicVineHosts[strings.ToLower(u.Host)] {
			continue
		}
		if m := comicVineIDSegment.FindStringSubmatch(u.Path); m != nil {
			appendRef(comicVineRef(m[1], m[2]))
		}
	}
	return out
}

func comicVineRef(prefix, id string) ComicVineRef {
	if prefix == "4050" {
		return ComicVineRef{Kind: ComicVineVolume, ID: id}
	}
	return ComicVineRef{Kind: ComicVineIssue, ID: id}
}

// VolumeRef returns the single VOLUME reference in refs, if there is exactly
// one.
//
// "Exactly one" is the same conservatism Kavita applies on its own side —
// ProcessSeries.cs:159-163 sets series.ComicVineId only when the distinct
// ComicVineSeriesId count across every parsed file is 1. Two different volume
// ids in one webLinks field describe two different ComicVine volumes, and
// picking one of them is a guess this code refuses to make.
func VolumeRef(refs []ComicVineRef) (ComicVineRef, bool) {
	var found ComicVineRef
	n := 0
	for _, r := range refs {
		if r.Kind != ComicVineVolume {
			continue
		}
		if n > 0 && r.ID != found.ID {
			return ComicVineRef{}, false
		}
		found, n = r, n+1
	}
	return found, n > 0
}
