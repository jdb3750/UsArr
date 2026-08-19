package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// Home's Block C: the one unified recently-added table across every media type,
// newest first (ARCHITECTURE.md §17.2 as amended by ADR-0028).
//
// WHICH SIDE OF THE ACCESS-SCOPE RULE THIS IS ON. catalogue.go states that every
// function in it is a replication WRITE with no acting user, and that "the reads
// that render any of this — the library grid, the item page, search — are the
// NEXT commit and every one of them takes a Scope in its signature". This is the
// first of those reads, and it does.
//
// IT IS A LOCAL READ AND NOTHING ELSE (principle 1). One statement — two on the
// one boundary described at recentWorksSQL — against the local file, no upstream
// call, no metadata provider, no image fetch.

// The media-type vocabulary ARCHITECTURE.md §17.2 closes at six. It is NOT
// work.kind: kind has eleven members, the navigation enum has six, and for two
// of the six the media type is a (kind, formats) PAIR because §6.1 makes an
// audiobook an edition of a book work rather than a kind of its own.
//
// These strings are a PUBLISHED vocabulary — httpapi hands them to the browser
// verbatim as the Type column's value — so they are constants here rather than
// spelled at the call site.
const (
	MediaTypeMovies     = "movies"
	MediaTypeTV         = "tv"
	MediaTypeMusic      = "music"
	MediaTypeEbooks     = "ebooks"
	MediaTypeAudiobooks = "audiobooks"
	MediaTypeComics     = "comics"
)

// editionFormatAudiobook is the one edition.format value that moves a `book`
// work from the Ebooks media type to the Audiobooks one (§17.2 rows 4 and 5).
const editionFormatAudiobook = "audiobook"

// recentWorkKinds is Block C's corpus, and it is an ALLOWLIST rather than a
// "everything except person" denylist. A kind added to the schema later is
// invisible here until someone maps it onto a media type, which is the failure
// direction to want: the alternative renders a row whose Type cell has nothing
// to put in it.
//
// WHAT IS ABSENT, AND WHY EACH ONE IS:
//
//   - 'person'. Excluded from the navigation enum, from the Tier 1 client prefix
//     index and from the FTS corpus (ADR-0033, §17.2, §4.5, schema.md §7
//     invariant 3). An author is a creative CREDIT, not a thing the user has a
//     library of, and there is no person screen in any milestone — so a person
//     row here is a row with nowhere to go. Migration 0007 made this reachable
//     rather than theoretical: importing one Kavita library creates hundreds of
//     person works, and every one of them carries an added_at from the same
//     import, so without this line the first Home screen after an import is
//     nothing but authors.
//   - 'season', 'episode', 'track', 'comic_issue'. The child kinds, excluded for
//     ADR-0030's reason: a large manga library's chapters would swamp every
//     query, and Block C is the screen where that shows first. A recently-added
//     table of 12,000 chapters is not "what did I just get".
//   - 'game'. §17.2's enum has six rows and none of them is Games, so a game
//     work has no Type cell to render into. Nothing writes a game work in v0.1.
//     Adding one is a one-line change HERE plus a seventh row in §17.2 — in that
//     order, because the enum is what this list is derived from.
var recentWorkKinds = []string{"movie", "series", "artist", "album", "book", "comic"}

// Bounds on ListRecentWorks.
//
// The default is 50 rather than §17.5's 10, because Block C is an inventory
// table rather than a status block: §17.1's own floor is 25 items above the
// fold, and a first page under that floor is the failure the block exists to
// avoid. The cap is what stops a query parameter turning a render read into a
// walk of the whole catalogue; §4.5's keyset window is ~100 and 200 leaves the
// client room to ask for two.
const (
	RecentWorksDefaultLimit = 50
	RecentWorksMaxLimit     = 200
)

// recentWorksSQLMaxRows is the clamp recentWorksSQL applies, and it is the page
// cap PLUS ONE. The one is ListRecentWorks's has-a-next-page probe row: without
// the +1 here, a caller asking for the maximum page would have its probe clamped
// away and would be told there is no next page whenever the last page is exactly
// full. The public bound stays RecentWorksMaxLimit; this is the statement's.
const recentWorksSQLMaxRows = RecentWorksMaxLimit + 1

// RecentWork is one row of Block C, as the store reads it.
//
// It is deliberately NOT a whole work record. §4.5's rule is "never load full
// item records", and the columns here are the ones §17.2 says the row renders:
// the Type column, the title, the year, the time, and the availability grammar
// the Have column uses. overview, remote_path, content_hash and size_on_disk are
// absent because nothing on this screen reads them — and remote_path is a host
// filesystem path on somebody else's box, which must never reach a browser.
type RecentWork struct {
	ID int64

	// Kind is work.kind verbatim. MediaType is what §17.2's Type column
	// renders. Both travel because they answer different questions: kind is the
	// schema's word and is what an item route needs, MediaType is the
	// navigation enum's word and is what the user reads.
	Kind      string
	MediaType string

	Title   string
	Year    sql.NullInt64
	AddedAt sql.NullString

	// The denormalised rollups §17.2's Have column renders, plus the
	// polymorphic availability blob it renders them THROUGH (schema.md §1,
	// "The availability blob, per medium"). Availability is carried as the raw
	// column: this layer does not parse it, because its shape is discriminated
	// by a "k" key and a renderer switches on that.
	HaveCount    int64
	WantCount    int64
	Availability sql.NullString

	// PosterKey is `image_asset.cache_key` for the work's poster — the key
	// `GET /img/{key}` addresses — and it is INVALID when the work has no
	// poster asset. ⚠️ That used to be every work, because nothing wrote
	// `image_asset`; imagewrite.go does, so an invalid PosterKey now means what
	// it says rather than describing the tree.
	//
	// It is here rather than on a second read for the reason §4.5's "never load
	// full item records" rule does NOT forbid: it is one correlated primary-key
	// lookup per row, on a read that already returns the row, against a second
	// round trip per cover. See PosterKeyExpr for why it is the cache_key and
	// not the asset id, and for why publishing it discloses nothing the caller
	// could not already see.
	PosterKey sql.NullString
}

// RecentWorksCursor is one keyset position in Block C's ordering.
//
// THE ORDER IS `added_at DESC, id DESC` AND added_at IS NULLABLE, which is the
// whole reason this is a struct with three states rather than a pair.
// ix_work_added is `(added_at DESC, id DESC) WHERE deleted_at IS NULL`, and
// SQLite sorts NULL below every value, so in a DESC index the rows with no
// added_at form a TAIL after every dated row. A plain `(added_at, id) < (?, ?)`
// row-value comparison yields NULL for such a row and therefore EXCLUDES it —
// so a naive keyset walk would page through every dated work and then stop,
// and a work whose upstream reported no creation date would be unreachable on
// every page but the first. Kavita reaching that state costs one absent
// `created` field (internal/libsync/kavita.go writes AddedAt from it, and
// nullTime stores a zero time as NULL).
//
// The three states:
//
//	{}                        the first page: no cursor predicate at all.
//	{AddedAt valid, ID: n}    inside the dated range.
//	{NullTail: true, ID: n}   inside the undated tail.
type RecentWorksCursor struct {
	AddedAt sql.NullString
	ID      int64

	// NullTail selects the undated tail. It is a separate field rather than
	// "AddedAt is not valid" so that the zero cursor stays unambiguously "the
	// first page" — a caller that forgets to set it asks for page one, which is
	// wrong-but-visible, rather than for the tail, which is wrong-and-silent.
	NullTail bool
}

// workVisibilityPredicate renders the access scope as a predicate over one
// work-id column, plus its arguments.
//
// WHY A work READ NEEDS ONE AT ALL. `work` carries no user_id — a catalogue row
// is not user-scoped property, a LIBRARY is (schema.md §13, principle 4). What
// scopes a work is which service instance replicated it, and that reaches the
// row through service_item_link. So the predicate is an EXISTS over the link
// table rather than a column comparison, and it is the same three branches
// Scope.instancePredicate has, for the same reasons:
//
//   - AllInstances — v0.1's owner — is `1=1` and adds nothing to the plan. That
//     is what keeps Home's read a single index walk on the machine everybody is
//     actually running.
//   - An empty visible set FAILS CLOSED at `1=0`. "No visible instances" must
//     mean no rows, never all rows. A rollup computed across instances a user
//     cannot see is an existence oracle (store.go rule 2), and Block C is a
//     rollup of exactly that shape: it says what arrived and when.
//   - Otherwise, EXISTS over the live links on the visible instances.
//     `deleted_at IS NULL` is on the inner query because ix_sil_work is partial
//     on it; a tombstoned link is not visibility.
func (s Scope) workVisibilityPredicate(column string) (string, []any) {
	if s.AllInstances {
		return "1=1", nil
	}
	if len(s.InstanceIDs) == 0 {
		return "1=0", nil
	}
	args := make([]any, 0, len(s.InstanceIDs))
	for _, id := range s.InstanceIDs {
		args = append(args, id)
	}
	return `EXISTS (SELECT 1 FROM service_item_link sil
	                 WHERE sil.work_id = ` + column + `
	                   AND sil.deleted_at IS NULL
	                   AND sil.service_instance_id IN (` + placeholders(len(args)) + `))`, args
}

// recentWorksSQL renders the ListRecentWorks statement and its arguments.
//
// It is a function rather than a literal inlined below so the query-plan test
// can EXPLAIN THE STATEMENT THAT SHIPS (docs/DEVELOPMENT.md §11 rule 1: a guard
// that asserts against a hand-copied lookalike is probing a proxy). The limit is
// clamped here, beside the statement it bounds, for the same reason
// recentProvenanceSQL clamps there — a second caller must not be able to render
// this query with an unbounded LIMIT.
//
// ⚠️ THE UNARY PLUS ON `+w.kind` IS LOAD-BEARING AND IS NOT A TYPO. Written as a
// plain `w.kind IN (…)`, SQLite prefers ix_work_kind_sort for the IN and then
// sorts the whole result. Measured on the real schema, SQLite 3.53.4:
//
//	w.kind IN (…)   →  SEARCH w USING INDEX ix_work_kind_sort (kind=?)
//	                   | USE TEMP B-TREE FOR ORDER BY
//	+w.kind IN (…)  →  SEARCH w USING INDEX ix_work_added (added_at<?)
//
// The first is a full sort of every non-deleted work of those six kinds on every
// page of Home — i.e. the recently-added view degrading exactly as schema.md §1
// warns the partial predicate exists to prevent, and it degrades WORSE as the
// catalogue grows. The unary plus is SQLite's documented way to say "this
// expression is not usable as an index key" (<https://sqlite.org/optoverview.html>,
// "the unary + operator … disqualifies the expression from use with indexes");
// it changes no value, and `+kind` and `kind` compare identically. Removing it is
// pinned by TestRecentWorksPlanGuardFires.
func recentWorksSQL(scope Scope, cur RecentWorksCursor, limit int) (string, []any) {
	switch {
	case limit <= 0:
		limit = RecentWorksDefaultLimit
	case limit > recentWorksSQLMaxRows:
		limit = recentWorksSQLMaxRows
	}

	args := make([]any, 0, len(recentWorkKinds)+8)

	// The Ebooks/Audiobooks split, resolved SERVER-SIDE because it has to be:
	// §17.2 states the Tier 1 client index ships no format at all, so a client
	// cannot separate the two. MIN over a boolean gives the whole answer in ONE
	// correlated subquery: 1 when every edition of the work is an audiobook,
	// 0 when at least one is not, NULL when the work has no edition rows.
	args = append(args, editionFormatAudiobook)

	for _, k := range recentWorkKinds {
		args = append(args, k)
	}

	scopePred, scopeArgs := scope.workVisibilityPredicate("w.id")
	args = append(args, scopeArgs...)

	cursorPred := ""
	switch {
	case cur.NullTail:
		// Inside the undated tail. `added_at IS NULL` is an equality on the
		// index's leading column as far as the planner is concerned, so this
		// stays a seek: SEARCH w USING INDEX ix_work_added (added_at=? AND id<?).
		cursorPred = "\n\t\t   AND w.added_at IS NULL AND w.id < ?"
		args = append(args, cur.ID)
	case cur.AddedAt.Valid:
		cursorPred = "\n\t\t   AND (w.added_at, w.id) < (?, ?)"
		args = append(args, cur.AddedAt.String, cur.ID)
	}

	args = append(args, limit)

	return `
		SELECT w.id, w.kind, w.title, w.year, w.added_at,
		       w.have_count, w.want_count, w.availability,
		       (SELECT MIN(e.format = ?) FROM edition e WHERE e.work_id = w.id),
		       ` + PosterKeyExpr + `
		  FROM work w
		 WHERE w.deleted_at IS NULL
		   AND +w.kind IN (` + placeholders(len(recentWorkKinds)) + `)
		   AND ` + scopePred + cursorPred + `
		 ORDER BY w.added_at DESC, w.id DESC
		 LIMIT ?`, args
}

// ListRecentWorks reads one keyset page of Home's Block C, newest first.
//
// SCOPE IS IN THE SIGNATURE AND IN THE SQL, both halves. A scope a query accepts
// and never filters on is indistinguishable from no scope at all, which is why
// TestRecentWorksScopeActuallyFilters breaks the predicate and watches the
// assertion catch it rather than trusting that the parameter is threaded.
//
// `deleted_at IS NULL` is in the query AND in ix_work_added's own partial
// predicate. That is not belt-and-braces: without the WHERE clause the read
// returns tombstoned rows, and without the partial index it reads them from the
// index and filters afterwards, so the 7-day tombstone window degrades exactly
// this view (schema.md §1).
//
// THE TWO-STATEMENT BOUNDARY. Ordering is `added_at DESC, id DESC` and added_at
// is nullable, so the undated rows form a tail the dated cursor predicate cannot
// reach — see RecentWorksCursor. When a dated page comes back short, the tail is
// where the rest of the page is, so this issues the tail query for the
// remainder. It happens at most once per page and only at the one boundary; a
// caller never sees it, and both statements have a pinned plan.
//
// The second return value is the cursor for the NEXT page, and it is invalid
// (ok=false) when this page is the last one.
func (s *Store) ListRecentWorks(
	ctx context.Context, scope Scope, cur RecentWorksCursor, limit int,
) ([]RecentWork, RecentWorksCursor, error) {
	switch {
	case limit <= 0:
		limit = RecentWorksDefaultLimit
	case limit > RecentWorksMaxLimit:
		limit = RecentWorksMaxLimit
	}

	// One extra row is read so "is there a next page" is an observation rather
	// than a guess. A cursor minted from a full page that happened to be the
	// last one sends the client back for an empty response, and an empty
	// response after a "Load more" is indistinguishable from a broken one.
	out, err := s.recentWorksPage(ctx, scope, cur, limit+1)
	if err != nil {
		return nil, RecentWorksCursor{}, err
	}

	// The dated range is exhausted; the rest of the page is in the undated tail.
	// The first page (a zero cursor) needs no handoff: with no cursor predicate
	// the ordered index walk crosses into the tail on its own. A tail cursor
	// needs none either — it is already there.
	if len(out) <= limit && cur.AddedAt.Valid && !cur.NullTail {
		tail, err := s.recentWorksPage(ctx, scope,
			RecentWorksCursor{NullTail: true, ID: recentWorksTailStart}, limit+1-len(out))
		if err != nil {
			return nil, RecentWorksCursor{}, err
		}
		out = append(out, tail...)
	}

	if len(out) <= limit {
		return out, RecentWorksCursor{}, nil
	}
	out = out[:limit]
	last := out[len(out)-1]
	next := RecentWorksCursor{AddedAt: last.AddedAt, ID: last.ID}
	if !last.AddedAt.Valid {
		next = RecentWorksCursor{NullTail: true, ID: last.ID}
	}
	return out, next, nil
}

// recentWorksTailStart is the id the undated tail is entered at: above every
// possible work.id, so `w.id < ?` admits the whole tail. SQLite's INTEGER
// PRIMARY KEY is a signed 64-bit rowid, so this is one past its maximum.
const recentWorksTailStart int64 = 1<<63 - 1

func (s *Store) recentWorksPage(
	ctx context.Context, scope Scope, cur RecentWorksCursor, limit int,
) ([]RecentWork, error) {
	if limit <= 0 {
		return nil, nil
	}
	query, args := recentWorksSQL(scope, cur, limit)
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read recently added: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]RecentWork, 0, limit)
	for rows.Next() {
		var w RecentWork
		// allAudiobook is the MIN() above: NULL for a work with no editions.
		var allAudiobook sql.NullInt64
		if err := rows.Scan(&w.ID, &w.Kind, &w.Title, &w.Year, &w.AddedAt,
			&w.HaveCount, &w.WantCount, &w.Availability, &allAudiobook,
			&w.PosterKey); err != nil {
			return nil, fmt.Errorf("read recently added: scan: %w", err)
		}
		w.MediaType = mediaTypeOf(w.Kind, allAudiobook)
		if w.MediaType == "" {
			// Unreachable: the kind allowlist in the query and the switch in
			// mediaTypeOf are the same six kinds. It is an error rather than a
			// dropped row because the two lists drifting apart is precisely the
			// bug that would otherwise ship a Type column with a blank cell.
			return nil, fmt.Errorf(
				"read recently added: work %d has kind %q, which recentWorkKinds admits and "+
					"mediaTypeOf does not map; the two lists have drifted apart", w.ID, w.Kind)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read recently added: %w", err)
	}
	return out, nil
}

// mediaTypeOf maps a work onto §17.2's six-value navigation enum.
//
// allAudiobook is `MIN(edition.format = 'audiobook')` over the work's editions:
// 1 when every edition is an audiobook, 0 when at least one is not, and invalid
// when the work has no edition rows at all.
//
// ⚠️ A BOOK WITH BOTH AN EPUB AND AN M4B RENDERS AS Ebooks, and the choice is
// forced rather than preferred. §17.2's "has content" predicates for rows 4 and
// 5 are two independent EXISTS queries, so such a book legitimately makes BOTH
// media types present in Block A — but Block C's row has ONE Type cell and
// cannot hold two values. Ebooks is the side that keeps the row honest: its
// format list is the longer one ('print','ebook','cbz','cbr','pdf'), so the
// claim "this is in your ebooks" is true of the mixed work, whereas filing it
// under Audiobooks hides an EPUB the user has. The seam for doing better is
// already in the schema and costs nothing to keep: library_member is
// EDITION-grained, so an edition-grained Block C row is a change to this
// function's inputs and not to the tables.
//
// A book with NO editions is Ebooks for the same reason — 'book' is the kind and
// an absent edition is not evidence of an audiobook.
func mediaTypeOf(kind string, allAudiobook sql.NullInt64) string {
	switch kind {
	case "movie":
		return MediaTypeMovies
	case "series":
		return MediaTypeTV
	case "artist", "album":
		return MediaTypeMusic
	case "comic":
		return MediaTypeComics
	case "book":
		if allAudiobook.Valid && allAudiobook.Int64 == 1 {
			return MediaTypeAudiobooks
		}
		return MediaTypeEbooks
	}
	return ""
}

// EncodeRecentWorksCursor renders a keyset position as one opaque-looking token.
//
// IT IS AN ENCODING, NOT A SECRET, and saying so here is the point: it carries
// added_at and work.id, both of which the row it came from already published.
// The token exists so the pair travels as ONE parameter with ONE parse — a
// client that assembled `?after_added_at=&after_id=` by hand would be writing
// the ordering into the frontend, and the ordering is the server's.
//
// work.id is publishable, unlike provenance.id. §4.5's Tier 1 client prefix
// index ships `{id, title, year, kind, availability_state}` for every top-level
// work by design, and item routes are `/library/movies/{id}` — the catalogue is
// shared, and its ids say nothing about what another user acquired. That is the
// exact property provenance.id does NOT have, which is why grabs.go publishes an
// HMAC there and this does not.
//
// The version prefix is what lets the format change without a client that pinned
// the old one silently paging wrongly: an unknown prefix is a parse failure, not
// a best effort.
//
// BASE64URL IS FOR TRANSPORT, NOT FOR CONCEALMENT. The payload embeds an
// added_at, and store.go's timeLayout is "2006-01-02 15:04:05" — WITH A SPACE,
// deliberately, because it must match SQLite's own datetime() byte for byte. A
// raw space is not something a client can put in a query string unescaped, and
// the first test that walked two pages of this endpoint caught it as a panic
// rather than as a wrong answer. Encoding the whole token makes the parameter
// safe by construction instead of by every caller remembering to escape it.
func EncodeRecentWorksCursor(c RecentWorksCursor) string {
	payload := "1d" + strconv.FormatInt(c.ID, 10) + "." + c.AddedAt.String
	if c.NullTail {
		payload = "1n" + strconv.FormatInt(c.ID, 10)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// DecodeRecentWorksCursor parses a token minted by EncodeRecentWorksCursor.
//
// STRICT, AND ON PURPOSE. A cursor that fails to parse is a client error the
// caller renders as a 400; it is never silently treated as "start from the
// beginning", because that turns a stale bookmark into an infinite Load-more
// loop over page one. The timestamp half is NOT re-validated as a date: it is
// compared as text, exactly as the column is (see store.go's timeLayout note),
// and it is bound as a parameter, so an unparseable value selects no rows rather
// than doing anything.
func DecodeRecentWorksCursor(token string) (RecentWorksCursor, error) {
	if token == "" {
		return RecentWorksCursor{}, nil
	}
	// The token is never echoed into the error. It is caller-supplied text and
	// the caller renders the message; reflecting it back is how a query
	// parameter ends up in a page.
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return RecentWorksCursor{}, fmt.Errorf("store: cursor is not base64url: %w", err)
	}
	payload := string(raw)
	if len(payload) < 3 || payload[0] != '1' {
		return RecentWorksCursor{}, fmt.Errorf("store: cursor is not a recently-added cursor")
	}
	switch payload[1] {
	case 'n':
		id, err := strconv.ParseInt(payload[2:], 10, 64)
		if err != nil || id <= 0 {
			return RecentWorksCursor{}, fmt.Errorf("store: cursor has no usable id")
		}
		return RecentWorksCursor{NullTail: true, ID: id}, nil
	case 'd':
		rest := payload[2:]
		dot := strings.IndexByte(rest, '.')
		if dot <= 0 || dot == len(rest)-1 {
			return RecentWorksCursor{}, fmt.Errorf("store: cursor is malformed")
		}
		id, err := strconv.ParseInt(rest[:dot], 10, 64)
		if err != nil || id <= 0 {
			return RecentWorksCursor{}, fmt.Errorf("store: cursor has no usable id")
		}
		return RecentWorksCursor{
			AddedAt: sql.NullString{String: rest[dot+1:], Valid: true},
			ID:      id,
		}, nil
	}
	return RecentWorksCursor{}, fmt.Errorf("store: cursor is not a recently-added cursor")
}
