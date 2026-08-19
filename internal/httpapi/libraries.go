package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// GET /api/v1/libraries is §17.8's Libraries screen, row view.
//
// §17.8 splits it from Services in one sentence — *"Services answers 'is the
// pipe up, and how do I fix it?'. Libraries answers 'what is in it, what is it
// called, and where do requests go?'"* — and this endpoint answers the first two
// thirds. The third, the request destination, is absent by §17.8's own rule; see
// libraryResponse.
//
// IT IS A LOCAL READ (principle 1). Two SQLite statements, no *Arr, no metadata
// provider, no capability probe, no image fetch. In particular it is NOT the
// connect probe: ADR-0048 puts the proposal set in the probe's response and says
// a `library` row exists only once the user has accepted one, so this endpoint
// serves accepted libraries and cannot serve a proposal.
//
// THE WIRE CONTRACT IS WRITTEN DOWN WHERE A CONSUMER WILL FIND IT:
// docs/reference/http-api.md §2. A doc comment is not reachable from a browser
// tab, which is the lesson /library/recent's entry records.
//
// ⚠️ WHAT THIS DOES NOT ANSWER, AND THE ARCHITECTURE DISSOLVED THE QUESTION
// RATHER THAN THIS ENDPOINT DUCKING IT. A consumer asked for a field
// distinguishing a library the user has never been shown from one they have
// acted on. There is no such field, and there is nothing for one to carry:
// [ADR-0048](../../docs/DECISIONS.md#adr-0048) decides, in its own words,
//
//	"A library proposal lives in the connect probe's response. It is never
//	persisted. A `library` row is created only when the user accepts one."
//
// so "once no row exists before Accept, every row is an accepted row by
// construction". Every library this endpoint returns has therefore been acted
// on; a proposal has no row to return. ⚠️ ADR-0048 is equally explicit that the
// removal is not done — §17.8: *"Libraries come into existence, on a first
// successful connect to a Kavita, with no screen involved … The Accept step
// below does not gate it, because the Accept step does not exist"* — and
// ADR-0048 clause 4 answers exactly that case: existing `managed_by = 'auto'`
// rows are DECLARED accepted, with no migration and no backfill. So the
// invariant holds by declaration today and by construction once Accept lands,
// and either way the wire field would be a constant.
//
// `managed_by` is not on the wire for the same reason, stated separately because
// it is a different argument: ADR-0048 Fact 1 and §17.8 both MEASURE that
// `'user'` has never been written by any code path, so the column is `'auto'` on
// every row. §17.4 rule 5 — a column whose value is identical for every row is
// not data — applies to a wire field as much as to a table cell.

// librarySourceResponse is one source chip as it crosses to a browser.
//
// A FIELD-BY-FIELD ALLOWLIST, never a denylist, and the struct IS the allowlist:
// store.LibrarySource is copied into this by hand.
//
// THIS IS THE STRUCT THAT MATTERS FOR §14. A library row joins to
// `service_instance`, which carries `api_key_enc` — a full-admin *Arr credential
// — and `base_url`, an internal host the user typed. The store read allowlists
// two columns off that table and this allowlists the same two onto the wire:
// the instance's NAME, which is the chip's label, and its KIND, which is the
// icon. §17.8 states the rule for the whole screen: *"No credential field ever
// appears on this screen"*, and §12.1 keeps API keys behind Services plus sudo.
// `base_url` is absent on top of that, because the address of an internal
// service is §17.3's to render and not this screen's.
type librarySourceResponse struct {
	ID int64 `json:"id"`

	// ServiceInstanceID is what §17.8's cross-link needs: *"a degraded source
	// on a library row links to that instance's Services row"*. It is the id
	// the Services screen already routes by, so publishing it adds no fact the
	// browser does not have once it has loaded /api/v1/services.
	ServiceInstanceID int64  `json:"service_instance_id"`
	ServiceName       string `json:"service_name"`
	ServiceKind       string `json:"service_kind"`

	// ContainerKind is migration 0005's five-value enum and ContainerRef is the
	// container the upstream itself reported, verbatim. Together they are the
	// binding §17.8's definition sentence describes — *"a name you own over
	// containers your services already computed"* — and a library whose source
	// cannot be traced back to a container is not meaningfully editable.
	ContainerKind string `json:"container_kind"`
	ContainerRef  string `json:"container_ref"`

	// ContainerName is `library_source.container_identity`, the container's own
	// name as the upstream reported it at bind time. §17.8's detail view calls
	// for it as *"upstream's own name beneath it, greyed and non-editable"*.
	// Absent rather than empty when the column is NULL: `""` reads as a
	// container whose name is blank.
	//
	// ⚠️ IT IS NOT A PATH IN v0.1. The only writer is catalogue.go's
	// insertLibrarySource, which hardcodes `container_kind = 'remote_library'`
	// and writes the Kavita library's name. A `root_folder` source would carry
	// a real upstream path, and §17.8 already draws that as data the screen
	// renders (its `Sonarr HD-1080p · /media/tv` cell); what it forbids is
	// UsArr TYPING a path, and neither this nor that is UsArr typing one.
	ContainerName *string `json:"container_name,omitempty"`

	// IsMetadataAuthority is §17.8's per-source radio. It is on the wire even
	// though the detail view *"suppresses [the control] entirely below two
	// sources"* — the suppression is a rendering rule about a radio group with
	// one option, not a reason to withhold which source won.
	IsMetadataAuthority bool `json:"is_metadata_authority"`

	// MissingSince is §17.8's per-source health, absent when the source is
	// healthy.
	//
	// ⚠️ NOTHING SETS IT — see store.LibrarySource.MissingSince for the
	// measurement. The two statements that touch the column both CLEAR it, so
	// "no source is missing" is currently a statement about the writer rather
	// than about the upstream. A renderer must not present the absence of this
	// field as a positive health check.
	MissingSince *time.Time `json:"missing_since,omitempty"`
}

// libraryResponse is one §17.8 row as it crosses to a browser.
//
// A FIELD-BY-FIELD ALLOWLIST. store.Library is copied into this by hand, so a
// column added to the store read cannot reach the wire by default, and
// TestLibrariesResponseKeysAreTheAllowlist pins the key set so growing it is a
// deliberate edit.
//
// WHAT IS ABSENT AND WHY, beyond the two arguments in this file's header:
//
//   - The four `sink_*` columns. §17.8: *"The `Request destination` column does
//     not render in v0.1, and it returns with the first service that can be a
//     destination"*, and separately *"the four `sink_*` columns are written by
//     nothing in the tree"*. Serving four nulls per row would make the deferral
//     look cosmetic when it is complete.
//   - `icon`, `default_sort`, and the corrections. §17.8's DETAIL view, which is
//     a different screen and a different read.
type libraryResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`

	// Slug is §17.8's URL identity, and the section is emphatic about how it is
	// NOT rendered: *"The row's identifier is not rendered as a path … Drop the
	// slash and the mono face"*. It travels because the chip is `?lib=ebooks`;
	// it is a query value, not a filesystem path, and a renderer that prefixes
	// it with a slash is making the mistake §17.8 names.
	Slug string `json:"slug"`

	// Kind is `library.kind` verbatim — the schema's word. §17.8 requires the
	// LABEL to be the product's — *"Label it `Movies · TV · Music · Books ·
	// Comics`, let the schema value be the value"* — so the mapping is the
	// renderer's. Note this is NOT §17.2's six-value media-type enum: that one
	// answers "what is this work" and is resolved server-side because the
	// client has no format; this one answers "what is this library of", has
	// seven members, and is a stored, user-editable property of the row.
	Kind string `json:"kind"`

	// Formats is `library.formats` forwarded verbatim as JSON: an array over
	// `edition.format`, and (kind, formats) is §17.2's media-type PAIR. Absent
	// when NULL, which means "any" — and which is EVERY row today, because
	// nothing writes the column. See store.Library.Formats.
	Formats json.RawMessage `json:"formats,omitempty"`

	// SortOrder is the reorder handle's value. It is served rather than left
	// implicit in the array order so a client that re-sorts locally can still
	// send back a correct reorder.
	SortOrder int64 `json:"sort_order"`

	Enabled         bool `json:"enabled"`
	IncludeInSearch bool `json:"include_in_search"`

	// ItemCount is §17.8's item-count column: `library_member` rows in this
	// library that the caller's scope admits. It is edition-grained by the
	// table's key, and equals a count of distinct works today because the only
	// writer files every work under the `edition_id = 0` sentinel.
	ItemCount int64 `json:"item_count"`

	// OrphanedAt is §6.5 rule 5's retained-with-a-reason state, absent when the
	// library has its sources. ⚠️ NOTHING WRITES IT — see store.Library.
	OrphanedAt *time.Time `json:"orphaned_at,omitempty"`

	// Sources is always present, and it is `[]` rather than absent for an
	// orphaned library: an absent key reads as "unknown", and "this library has
	// no sources" is precisely the fact §17.8's orphaned state renders.
	Sources []librarySourceResponse `json:"sources"`

	// Completeness is what the last import measured about how much of this
	// library's upstream containers UsArr's credential could see.
	//
	// ⚠️ ABSENT MEANS NOTHING WAS MEASURED, AND A CLIENT MUST NOT READ IT AS
	// "COMPLETE". It is absent for every library whose source runs no
	// completeness check — today that is every Kavita library — and for every
	// library that has never been imported. This is the ONE optional key on this
	// response whose absence is a claim about UsArr rather than about the
	// library, which is why it is `omitempty` on a pointer rather than a
	// zero-valued struct: an object with `state: ""` would be a fourth state
	// nobody defined.
	Completeness *libraryCompletenessResponse `json:"completeness,omitempty"`
}

// libraryCompletenessResponse is one library's completeness verdict on the wire.
//
// A FIELD-BY-FIELD ALLOWLIST like every other struct in this file, and a
// deliberately narrow one: `containers` is on store.LibraryCompleteness and is
// NOT here, because the count of folded verdicts is a fact about the fold rather
// than about the library, and no rendering asks for it. Add it when a screen
// does.
type libraryCompletenessResponse struct {
	// State is `complete`, `shortfall` or `unverified`, and there is no fourth
	// member. `unverified` is the state that keeps the other two honest: the
	// check rests on an upstream route being unguarded, and if that changes
	// every verdict becomes this one rather than silently becoming `complete`.
	// See internal/store/completeness.go.
	State string `json:"state"`

	// Total, Visible and Hidden are ITEM COUNTS AS THE UPSTREAM COUNTED THEM,
	// and they are NOT `item_count` above — that one counts library_member rows
	// UsArr wrote, these count what the upstream said it holds. The two can
	// legitimately differ for reasons that have nothing to do with a filter (a
	// comic BookOrbit holds and UsArr does not map, for one), so a client must
	// not subtract one from the other.
	//
	// ⚠️ ALL THREE ARE ABSENT WHEN State IS `unverified`. In that state they are
	// not small or stale, they are UNKNOWN, and serving 0 would put "the library
	// is empty" and "we did not look" in the same value.
	Total   int64 `json:"total_items,omitempty"`
	Visible int64 `json:"visible_items,omitempty"`
	Hidden  int64 `json:"hidden_items,omitempty"`

	// Reason is UsArr's own sentence about why the check could not be made,
	// present only with `unverified`. ⚠️ NEVER UPSTREAM TEXT — the upstream's own
	// error stays in the process log, which reference/security.md §5 requires and
	// which is also why there is no status code or provider message here.
	Reason string `json:"reason,omitempty"`

	// CheckedAt is when the verdict was recorded. It answers "how old is this
	// claim", which is the question any stored measurement raises, and it is what
	// lets a client show a shortfall as a measurement rather than as a live fact.
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

type librariesResponse struct {
	// Items is always present, `[]` on an empty install. §17.8's screen has a
	// zero state — a first run before any service is connected — and an absent
	// key would make it indistinguishable from a failure.
	Items []libraryResponse `json:"items"`
}

// handleListLibraries serves §17.8's Libraries list.
//
// NO PAGING AND NO QUERY PARAMETERS, matching the store read: a user's libraries
// are a set they created by hand, and the row view is a reorderable list, which
// a keyset window cannot express. There is nothing here to clamp, so there is no
// 400 path.
func (s *Server) handleListLibraries(w http.ResponseWriter, r *http.Request) error {
	a, ok := sessionFrom(r)
	if !ok {
		return errStatus(http.StatusUnauthorized, CodeUnauthorized, "this request has no session")
	}

	// SCOPE. storeScope derives it from the session and nothing else — the
	// caller cannot widen it with a query parameter, which is the whole reason
	// it is not one. v0.1's owner gets the full scope; a non-owner gets an
	// empty visible set, which the store renders as `1=0` and which therefore
	// returns no libraries rather than every library.
	rows, err := s.store.ListLibraries(r.Context(), storeScope(a))
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"your libraries could not be read").wrapping(err)
	}

	out := librariesResponse{Items: make([]libraryResponse, 0, len(rows))}
	for _, row := range rows {
		out.Items = append(out.Items, s.toLibraryResponse(row))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// toLibraryResponse is the allowlist applied. Every field is named; nothing is
// copied by reflection or embedded.
func (s *Server) toLibraryResponse(l store.Library) libraryResponse {
	out := libraryResponse{
		ID:              l.ID,
		Name:            l.Name,
		Slug:            l.Slug,
		Kind:            l.Kind,
		SortOrder:       l.SortOrder,
		Enabled:         l.Enabled,
		IncludeInSearch: l.IncludeInSearch,
		ItemCount:       l.ItemCount,
		Sources:         make([]librarySourceResponse, 0, len(l.Sources)),
	}
	out.Formats = s.libraryFormatsFor(l)
	out.OrphanedAt = libraryTime(l.OrphanedAt)
	out.Completeness = libraryCompletenessFor(l)
	for _, src := range l.Sources {
		out.Sources = append(out.Sources, librarySourceResponse{
			ID:                  src.ID,
			ServiceInstanceID:   src.ServiceInstanceID,
			ServiceName:         src.ServiceName,
			ServiceKind:         src.ServiceKind,
			ContainerKind:       src.ContainerKind,
			ContainerRef:        src.ContainerRef,
			ContainerName:       nullableString(src.ContainerIdentity),
			IsMetadataAuthority: src.IsMetadataAuthority,
			MissingSince:        libraryTime(src.MissingSince),
		})
	}
	return out
}

// libraryCompletenessFor renders the verdict, or nothing at all.
//
// NOTHING IS INVENTED HERE AND NOTHING IS DEFAULTED. A nil verdict means the
// last import ran no completeness check for this library — or no import has ever
// run — and the key is simply absent. The one transformation is the numeric
// suppression under `unverified`: `omitempty` would already drop three zeroes,
// and this makes the zeroing explicit so that a later edit adding a non-zero
// default cannot quietly publish a count nobody measured.
func libraryCompletenessFor(l store.Library) *libraryCompletenessResponse {
	c := l.Completeness
	if c == nil {
		return nil
	}
	out := &libraryCompletenessResponse{State: string(c.State)}
	if c.State == store.CompletenessUnverified {
		out.Reason = c.Reason
	} else {
		out.Total, out.Visible, out.Hidden = c.Total, c.Visible, c.Hidden
	}
	if at, err := store.ParseTime(c.CheckedAt); err == nil {
		out.CheckedAt = &at
	}
	return out
}

// libraryFormatsFor decides what the `formats` key carries, and it separates
// "absent because the library holds any format" from "absent because what is
// stored is not the shape the column promises".
//
// Migration 0005 declares the column *"JSON array over edition.format, NULL for
// 'any'"*. NULL is omitted silently — it is the honest majority state and
// logging it would drown the log. Anything that will not decode as a JSON ARRAY
// is omitted and LOGGED, on availabilityFor's reasoning: forwarding it would
// hand a renderer a value its type says is a list and is not, and dropping it
// silently would make a writer bug byte-identical to the honest case.
//
// The log carries the library id, which is what makes the row findable. It does
// not carry the value: that text came out of the database and may be
// arbitrarily long.
func (s *Server) libraryFormatsFor(l store.Library) json.RawMessage {
	if !l.Formats.Valid {
		return nil
	}
	var probe []string
	if err := json.Unmarshal([]byte(l.Formats.String), &probe); err != nil {
		s.log.Warn("library.formats is not a JSON array of strings and was dropped from the response",
			"library_id", l.ID, "err", redactText(err.Error()))
		return nil
	}
	return json.RawMessage(l.Formats.String)
}

// libraryTime renders a stored timestamp, or nothing.
//
// A stored timestamp that will not parse is DROPPED rather than guessed at,
// exactly as toRecentWorkResponse drops an unparseable added_at. Both fields
// this serves — orphaned_at and missing_since — are claims about when something
// broke, and a wrong date on either is worse than a missing one.
func libraryTime(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	at, err := store.ParseTime(v.String)
	if err != nil {
		return nil
	}
	return &at
}

// nullableString renders a nullable text column as an omittable string.
func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
