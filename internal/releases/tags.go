package releases

import (
	"sort"

	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/servarr/mapping"
)

// Tag is one derived tag, stored decomposed into namespace and value — never as
// one "namespace:value" string (tags.md §2).
type Tag struct {
	Namespace string
	Value     string
}

// String renders the display form.
func (t Tag) String() string { return t.Namespace + ":" + t.Value }

// DeriveTags computes the system tags a Prowlarr result carries immediately, with
// no library behind it (ARCHITECTURE.md §8.5). This is the source-tagging
// differentiator working with zero library, which is the point of the mode.
//
// Derived tags are never user-editable. Nothing here is inference: `source:` comes
// from a first-class protocol enum asserted by the indexer definition, and `type:`
// from the Newznab parent category, which is an always-present independent signal.
//
// ix may be nil when the owning indexer is unknown; the indexer-privacy tag is
// then omitted rather than guessed.
func DeriveTags(r servarr.ReleaseResource, ix *servarr.IndexerResource) []Tag {
	var tags []Tag
	add := func(ns, v string) {
		if v != "" {
			tags = append(tags, Tag{Namespace: ns, Value: v})
		}
	}

	// source: from DownloadProtocol. Byte-identical across the Prowlarr, Sonarr and
	// Radarr specs, which is what makes it assertable rather than guessed.
	add("source", mapping.SourceValue(r.Protocol))

	// type: (and sometimes format:) from the raw category array. Single-valued.
	if t, f, ok := mapping.MediaType(r.CategoryIDs()); ok {
		add("type", t)
		add("format", f)
	}

	// indexer: from the result's own indexer name, falling back to the resource.
	name := r.Indexer
	if name == "" && ix != nil {
		name = ix.Name
	}
	add("indexer", mapping.Slug(name))

	if ix != nil {
		add("indexer-privacy", mapping.PrivacyValue(ix.Privacy))
	}

	// flag: from indexerFlags, which is string[] on Prowlarr (an int bitmask on
	// Radarr's MovieFileResource, and untyped in Radarr's ReleaseResource — hence
	// one mapper per (app, resource) rather than one per app).
	seen := make(map[string]bool, len(r.IndexerFlags))
	flags := make([]string, 0, len(r.IndexerFlags))
	for _, f := range r.IndexerFlags {
		s := mapping.Slug(f)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		flags = append(flags, s)
	}
	// Sorted so the tag set of a given release is stable across searches; the
	// upstream array order is not guaranteed.
	sort.Strings(flags)
	for _, f := range flags {
		add("flag", f)
	}

	return tags
}
