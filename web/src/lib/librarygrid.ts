/**
 * THE PER-TYPE LIBRARY GRID, CLIENT SIDE — `GET /api/v1/library`.
 *
 * ARCHITECTURE.md §17.2's per-type grid and §17.8's library scope, over the one
 * endpoint that serves both. `docs/reference/http-api.md` §7 is the contract and
 * this module is written against it rather than against a memory of it.
 *
 * TWO SCREENS, ONE CLIENT. `routes/library/[type]` is one media type at a time
 * and `routes/library` is all six at once, and they are the SAME read with
 * `media_type` present or omitted — so `BrowseQuery.mediaType` is optional and
 * every rule below is written over that optionality rather than duplicated. The
 * axes are unchanged (§8.1 of `docs/design/DESIGN-DIRECTION.md`, closing OQ-2):
 * media type is the navigation axis, so `/library/movies` is a place, and a
 * library is a SCOPE carried as `?lib=` rather than a place of its own.
 *
 * ⚠️ `GET /api/v1/library/recent` IS STILL A DIFFERENT ENDPOINT AND STILL HAS A
 * CLIENT — `$lib/library`'s — because Home's Block C is closed at one table, one
 * order and no filters (§17.2, ADR-0028). What changed is which screen uses
 * which: `/library` now reads the BROWSE endpoint, because a screen that sorts
 * and scopes cannot be served by an endpoint that parses `limit` and `cursor`
 * and nothing else. At the default the two agree row for row and order for
 * order — same six kinds, same `added_at DESC, id DESC`, same undated-tail
 * handoff, same row on the wire — which is what made the switch a switch rather
 * than a rewrite. `TestBrowseWorksUnfilteredIsBlockCsCorpus`
 * (`internal/store/browse_test.go`) is that equality, executed. The two share a
 * row shape and a paging rule and share NO cursor (§7.5), so `$lib/library`'s
 * row parsing, availability rendering and feed algebra are imported here and
 * the query half is new.
 *
 * DOM-free and deterministic, like `$lib/library` and `$lib/libraryscreen`, so
 * the node-environment vitest run can pin every rule in it. `vitest.config.ts`
 * is `environment: 'node'` with no Svelte plugin, so a rule that lives inside an
 * `{#if}` in `routes/library/[type]/+page.svelte` is a rule nothing can test —
 * which is why the sort vocabulary, the cursor-reset rule and the query builder
 * are functions here rather than markup there.
 *
 * IT IS A LOCAL READ (principle 1). `internal/httpapi/library.go` says so at
 * `handleBrowseWorks`: one SQLite statement per page plus at most one small
 * statement to resolve `?lib=` slugs, no *Arr, no metadata provider, no image
 * fetch.
 *
 * ⚠️ FIVE PROPERTIES OF THIS ENDPOINT BREAK A CLIENT SILENTLY, AND EACH ONE IS
 * HELD BY A FUNCTION BELOW RATHER THAN BY A COMMENT.
 *
 *   1. `sort=sort_title` IS REFUSED MORE BROADLY THAN "not music".
 *      `browseWorksSQL` refuses it when `len(kinds) != 1`, and no `media_type`
 *      at all is six kinds, so the refusal covers `music` AND the unfiltered
 *      case — which is now a SHIPPING screen rather than a hypothetical, since
 *      `/library` sends no `media_type` at all. `browseSortsFor` derives
 *      availability from the kind COUNT rather than from a hard-coded "not
 *      music", and `browseSortNote` states the absence in UsArr's own words
 *      rather than letting the server's 400 text reach a reader.
 *   2. `sort=year` IS LEGAL IN THE SCHEMA AND REFUSED ON THE WIRE. Migration
 *      0005's CHECK admits it as a `library.default_sort`; `work.year` has no
 *      index, so this read refuses it by name. `readBrowseSort` can never
 *      return it, whatever a URL or a stored default says.
 *   3. THE CURSOR BINDS TO `sort` ALONE — not to `media_type` and not to `lib`.
 *      Replaying one under a different sort is a 400, but replaying it under a
 *      different TYPE or a different SCOPE is a 200 that starts partway into a
 *      different corpus. Nothing server-side catches it. `browseFeedFor` drops
 *      the whole feed whenever any of the three changes.
 *   4. THE ECHOED `limit` IS AUTHORITATIVE (§1.2, which governs both endpoints).
 *      The server clamps at 200 silently. `nextBrowsePage` asks for the size the
 *      server said it applied, never for the one this module asked for first.
 *   5. AN UNKNOWN QUERY PARAMETER IS IGNORED, SILENTLY. `handleBrowseWorks`
 *      reads five names off `r.URL.Query()` and never enumerates it, so a typo
 *      answers 200 with an unfiltered page one, which reads as "your library
 *      has everything in it". ⚠️ THAT IS A DELIBERATE WIRE CONTRACT AND NOT AN
 *      OVERSIGHT, so no amount of asking will make the server refuse:
 *      `http-api.md`'s preamble makes ignore-unknown API-wide and
 *      `TestUnrecognisedQueryParametersAreIgnoredNotRefused`
 *      (`internal/httpapi/library_browse_test.go`) pins it. The client has to
 *      notice for itself, and it notices from the ROUND TRIP rather than from a
 *      list of names it holds: `browseParams` is the only builder, and
 *      `browseEchoDrift` compares what it meant to send against the
 *      `media_type`, `sort` and `lib` the envelope says were applied.
 *
 * AND ONE ASYMMETRY THAT IS EASY TO GET BACKWARDS: an EMPTY `media_type` is
 * silently unfiltered, an EMPTY `lib` is a 400 (§7.3 — "a scope was asked for
 * and nothing was named"). So `lib` is emitted only when there is something to
 * put in it, and never as `?lib=`.
 */

import { getJson } from './api';
import type { HomeMode } from './home';
import {
	isMediaType,
	mediaTypeLabel,
	toRecentPage,
	type MediaType,
	type RecentItem,
	type RecentPage
} from './library';
import { recentEmptyState, type RecentEmptyState } from './libraryscreen';

/** The endpoint, as `internal/httpapi/server.go` routes it. It is NOT a
 * superset of `/library/recent` and shares no cursor with it (§7 preamble). */
export const LIBRARY_BROWSE_URL = '/api/v1/library';

/* ── 1. the three orders, and the fourth that is refused ──────────────────── */

/**
 * §7.6's three orders, in the server's OWN words.
 *
 * The vocabulary is `library.default_sort`'s verbatim — migration 0005 spells
 * it `CHECK (default_sort IN ('sort_title','added_at','year','popularity'))` —
 * so a client that reads a library's stored default can hand it straight back.
 * Inventing wire words ("newest", "az") would be a second spelling for one set
 * of states, which is the drift the endpoint was named against.
 */
export const BROWSE_SORTS = ['added_at', 'sort_title', 'popularity'] as const;

export type BrowseSort = (typeof BROWSE_SORTS)[number];

/** `added_at` is the server's default and is what an absent `?sort=` means. */
export const DEFAULT_BROWSE_SORT: BrowseSort = 'added_at';

/**
 * ⚠️ THE FOURTH MEMBER OF THE SCHEMA'S CHECK, AND IT MUST NEVER BE SENT.
 *
 * `year` is a legal `library.default_sort` value and is refused by this
 * endpoint with its own message: `work.year` has no index of any kind, so the
 * order would be a temp b-tree over the whole filtered corpus on every page. It
 * is named here rather than merely absent from `BROWSE_SORTS`, because the
 * likeliest way a client sends it is by reading a library's own default back —
 * not by typing it — and a constant is what a guard can assert against.
 */
export const REFUSED_BROWSE_SORT = 'year';

const BROWSE_SORT_LABEL: Record<BrowseSort, string> = {
	added_at: 'Newest first',
	sort_title: 'A to Z',
	popularity: 'Most popular'
};

export function browseSortLabel(sort: BrowseSort): string {
	return BROWSE_SORT_LABEL[sort];
}

export function isBrowseSort(value: string): value is BrowseSort {
	return (BROWSE_SORTS as readonly string[]).includes(value);
}

/**
 * HOW MANY `work.kind` VALUES A MEDIA TYPE RESOLVES TO. `browseKinds` in
 * `internal/store/browse.go`, mirrored — and mirrored as a COUNT because the
 * count is the only part of it a client needs, and the kinds themselves are a
 * server vocabulary this screen must never navigate by (§7.2).
 *
 *   movies → movie · tv → series · comics → comic       one kind
 *   ebooks, audiobooks → book                           one kind, split by
 *                                                       edition.format
 *   music → artist AND album                            two kinds
 *
 * ⚠️ THIS IS WHAT DECIDES WHETHER A→Z IS OFFERED, and it is a count rather than
 * a `!== 'music'` test on purpose: the store's condition is `len(kinds) != 1`,
 * so the refusal already covers a second two-kind type the day one is added,
 * and a hard-coded exception for music would not.
 */
export function browseKindCount(mediaType: MediaType | undefined): number {
	if (mediaType === undefined) return ALL_TYPES_KIND_COUNT;
	return mediaType === 'music' ? 2 : 1;
}

/**
 * HOW MANY KINDS THE UNFILTERED CORPUS IS, and it is a number rather than
 * `MEDIA_TYPES.length` because the two are not the same quantity and only
 * coincidentally the same digit. `browseKinds("")` in `internal/store/browse.go`
 * returns `recentWorkKinds`, which is `movie, series, artist, album, book,
 * comic` — six KINDS over six MEDIA TYPES, where music is two kinds and books
 * are one kind split by `edition.format`. Counting the nav enum instead would be
 * right today and wrong the moment either mapping moves.
 */
const ALL_TYPES_KIND_COUNT = 6;

/**
 * The orders this media type can actually be served in.
 *
 * §7.6: `sort_title` walks `ix_work_kind_sort`, which is `(kind, sort_title,
 * id)`, and SQLite cannot supply `ORDER BY` from an index whose leading column
 * is constrained by `IN` — so it needs the kind as an equality and is refused
 * for anything that is not exactly one kind. Offering the control anyway would
 * put a 400 behind a chip that looks like the others.
 */
export function browseSortsFor(mediaType: MediaType | undefined): BrowseSort[] {
	return BROWSE_SORTS.filter((s) => s !== 'sort_title' || browseKindCount(mediaType) === 1);
}

/**
 * WHY A TO Z IS MISSING FROM THE ALL-TYPES SCREEN'S SORT CONTROL, IN UsArr's
 * OWN WORDS.
 *
 * ⚠️ THE SERVER'S OWN 400 TEXT MUST NEVER REACH THIS SCREEN, and that is the
 * whole reason this string exists rather than a `catch` that renders
 * `ApiError.action`. `handleBrowseWorks` answers an unservable sort with
 * *"sort_title needs a media_type of one kind — not music — and there is no
 * index behind year at all"*: it is one shared sentence for two different
 * refusals, it names a parameter and a column rather than anything on screen,
 * and it talks about music and about year to a reader who asked for neither.
 * Correct for a wire consumer, wrong for this audience.
 *
 * ⚠️ AND IT IS STATED RATHER THAN OFFERED-THEN-REFUSED. `browseSortsFor` keeps
 * the option out of the control entirely, so this note is the only place the
 * order is mentioned; a control that listed A to Z and answered with a banner
 * would be spending a round trip to say something knowable before it was sent.
 *
 * `browseSortNote` is what decides when it prints, and it decides from the kind
 * COUNT — see `browseKindCount` — never from the library scope. A scope narrows
 * the rows and changes no index, so `?lib=` has nothing to do with which orders
 * are servable, and keying the note on it would be a rule that fires on the
 * wrong condition and reads as if it were right.
 */
export const BROWSE_AZ_UNAVAILABLE =
	'This view spans every media type, and A to Z is not available across all of them.';

/**
 * WHY A TO Z IS MISSING FROM ONE MEDIA TYPE'S SORT CONTROL, IN UsArr's OWN
 * WORDS. Music is the only type that reaches it today, and it was dropped
 * SILENTLY there until this string existed — two options and no reason, on the
 * one screen where the absence is most surprising, because the five per-type
 * screens beside it all offer three.
 *
 * ⚠️ IT DOES NOT SAY "MUSIC", AND THAT IS THE POINT. `browseKinds` in
 * `internal/store/browse.go` maps `music` onto `artist` AND `album`, and the
 * store's refusal is `len(kinds) != 1` rather than a test on the type's name. A
 * sentence naming artists and albums would be a second hard-coded exception to
 * hold in step with that map, and would be a lie on the day a second two-kind
 * type is added; the COUNT is the fact this note and `browseSortsFor` are both
 * derived from.
 *
 * ⚠️ AND IT IS NOT THE ALL-TYPES SENTENCE. That one opens "this view spans
 * every media type", which is false on a view that spans exactly one. Same
 * refusal, two screens, two sentences — and the server's own 400 text is used
 * for neither, for the reason `BROWSE_AZ_UNAVAILABLE` gives above.
 */
export const BROWSE_AZ_UNAVAILABLE_MULTI_KIND =
	'This media type covers more than one kind of item, and A to Z is not available across them.';

/**
 * The one-line reason to print beside the sort control, or `undefined` when
 * every order this module knows about is on the control.
 *
 * ⚠️ WHETHER A NOTE PRINTS AT ALL IS THE KIND COUNT'S ANSWER AND NEVER THE
 * MEDIA TYPE'S NAME: the guard below is `browseSortsFor`, which is
 * `browseKindCount(…) === 1` and so is the store's own `len(kinds) != 1`. WHICH
 * of the two sentences prints is then a question about the screen rather than
 * about the refusal — the all-types view is the one with no media type at all —
 * so a second two-kind type gets a true sentence here without an edit.
 */
export function browseSortNote(query: BrowseQuery): string | undefined {
	if (browseSortsFor(query.mediaType).includes('sort_title')) return undefined;
	return query.mediaType === undefined ? BROWSE_AZ_UNAVAILABLE : BROWSE_AZ_UNAVAILABLE_MULTI_KIND;
}

export function browseSortAvailable(
	sort: string,
	mediaType: MediaType | undefined
): sort is BrowseSort {
	return isBrowseSort(sort) && browseSortsFor(mediaType).includes(sort);
}

/**
 * `?sort=` off a URL, and it can only ever answer with an order this media type
 * can be served in.
 *
 * An unknown, refused or unavailable key falls back to the default rather than
 * erroring, which is `$lib/sortspec.readSort`'s rule for the same reason: a link
 * that names an order this build cannot serve should still open the screen, and
 * the screen's own control then says what it is actually sorted by. `year` and
 * `sort_title`-on-music both land here, so neither can reach the wire.
 */
export function readBrowseSort(
	params: URLSearchParams,
	mediaType: MediaType | undefined
): BrowseSort {
	const raw = (params.get('sort') ?? '').trim();
	return browseSortAvailable(raw, mediaType) ? raw : DEFAULT_BROWSE_SORT;
}

/* ── 2. the library scope ─────────────────────────────────────────────────── */

/**
 * §7.3's bound on `?lib=`, and it is a REFUSAL rather than a clamp.
 *
 * Silently dropping slugs WIDENS the page — fewer scope terms means more rows —
 * which is the one direction a filter must never fail in. So a client that
 * holds more than this many does not send a truncated list; it says so.
 */
export const MAX_LIBRARY_SLUGS = 32;

/**
 * The `?lib=` slugs a URL carries, in order, with the empty spellings removed.
 *
 * ⚠️ AN EMPTY RESULT MEANS "NO SCOPE" AND MUST NEVER BE SENT AS `?lib=`. The
 * server tests PRESENCE rather than emptiness, so `?lib=`, `?lib=%20` and
 * `?lib=,,` are all 400 "lib was sent with no library in it" — while an empty
 * `media_type` is silently unfiltered. The two parameters fail in opposite
 * directions and only one of them is safe to pass through blind.
 */
export function readLibraryScope(params: URLSearchParams): string[] {
	return (params.get('lib') ?? '')
		.split(',')
		.map((s) => s.trim())
		.filter((s) => s !== '');
}

/* ── 3. one query, and the route that produces it ─────────────────────────── */

/**
 * EVERYTHING THE SERVER HAS TO BE TOLD AGAIN ON EVERY PAGE (§7.5: "send
 * `media_type`, `lib` and `sort` again on every page, unchanged. The server does
 * not remember them"), and therefore everything a cursor is only valid within.
 */
export interface BrowseQuery {
	/**
	 * ⚠️ ABSENT MEANS EVERY MEDIA TYPE AT ONCE, AND IT IS AN OPTIONAL FIELD
	 * RATHER THAN AN `'all'` MEMBER OF THE ENUM. §17.2's enum is closed at six
	 * and the wire's `media_type` is closed at the same six, so a seventh value
	 * spelled `all` would be a vocabulary this client holds and the server does
	 * not — and `browseParams` would have to translate it back to an omission on
	 * every page. Omission is what the endpoint actually understands (§7.2: an
	 * absent `media_type` is every type), so it is what this field spells.
	 */
	mediaType?: MediaType;
	sort: BrowseSort;
	/** Library slugs. Empty is "every library", never `?lib=`. */
	libraries: string[];
}

/**
 * WHAT A `/library/[type]` URL RESOLVES TO, INCLUDING THE TWO WAYS IT CAN FAIL
 * BEFORE ANY REQUEST IS MADE.
 *
 * Both failures are refusals the client can make on its own, and making them
 * here is the point: §7.2 and §7.3 tell us exactly what the server will do with
 * an unknown media type and with a 33rd slug, so sending either would be
 * spending a round trip to be told something already known — and would put the
 * server's wording on a mistake the URL bar made.
 */
export type BrowseRoute =
	| { k: 'ok'; query: BrowseQuery }
	| { k: 'unknown-type'; value: string }
	| { k: 'too-many-libraries'; count: number };

/**
 * A query that is KNOWN to name a media type, and the route that produces one.
 *
 * ⚠️ IT EXISTS BECAUSE `BrowseQuery.mediaType` BECAME OPTIONAL AND THE PER-TYPE
 * SCREEN'S GUARANTEE DID NOT. `browseRoute` refuses any segment that is not one
 * of the six before it builds anything, so on that screen the field is always
 * set — and that screen renders a heading, a page title and an empty state off
 * it. Without this narrowing those three would each need a fallback for a case
 * the route has already made unreachable, and a fallback for an unreachable case
 * is a second answer nobody can ever check.
 */
export interface TypedBrowseQuery extends BrowseQuery {
	mediaType: MediaType;
}

export type TypedBrowseRoute =
	| { k: 'ok'; query: TypedBrowseQuery }
	| { k: 'unknown-type'; value: string }
	| { k: 'too-many-libraries'; count: number };

/**
 * The route parameter and the query string, resolved together.
 *
 * ⚠️ THE TYPE SEGMENT IS VALIDATED AGAINST THE SIX AND IS NEVER PASSED THROUGH.
 * `media_type` is a closed enum server-side and an unrecognised value is a 400,
 * never an unfiltered page — so a request built from an unvalidated segment is
 * a request whose answer is already known. §17.2's enum is `$lib/library`'s
 * `MEDIA_TYPES` and this function is the only door into a `BrowseQuery`.
 */
export function browseRoute(rawType: string, params: URLSearchParams): TypedBrowseRoute {
	const value = rawType.trim();
	if (!isMediaType(value)) return { k: 'unknown-type', value };
	const resolved = browseQueryFor(value, params);
	// The narrowing is STRUCTURAL rather than an assertion: the field is written
	// again from the value `isMediaType` just accepted, so nothing here claims a
	// property the code does not establish two lines up.
	if (resolved.k !== 'ok') return resolved;
	return { k: 'ok', query: { ...resolved.query, mediaType: value } };
}

/**
 * THE SAME RESOLUTION FOR THE ALL-TYPES SCREEN, which has no `[type]` segment
 * to validate and therefore cannot fail the first way.
 *
 * ⚠️ IT IS THE SAME FUNCTION UNDERNEATH RATHER THAN A SECOND COPY OF THE SCOPE
 * BOUND, and that matters more than it looks: the 32-slug bound is a REFUSAL
 * rather than a clamp (dropping slugs widens the page), so a screen that
 * resolved its own `?lib=` and forgot the bound would silently show more than
 * was asked for on exactly the address the bound exists for.
 */
export function browseAllTypesRoute(params: URLSearchParams): BrowseRoute {
	return browseQueryFor(undefined, params);
}

/** The half both resolvers share: the scope, its bound, and the sort that this
 * media type can actually be served in. */
function browseQueryFor(mediaType: MediaType | undefined, params: URLSearchParams): BrowseRoute {
	const libraries = readLibraryScope(params);
	if (libraries.length > MAX_LIBRARY_SLUGS) {
		return { k: 'too-many-libraries', count: libraries.length };
	}
	const query: BrowseQuery = { sort: readBrowseSort(params, mediaType), libraries };
	if (mediaType !== undefined) query.mediaType = mediaType;
	return { k: 'ok', query };
}

/** Whether two queries are the same corpus in the same order. Identity, not
 * equality of object references: it is what decides whether a cursor is still
 * meaningful. */
export function sameBrowseQuery(a: BrowseQuery, b: BrowseQuery): boolean {
	return (
		a.mediaType === b.mediaType &&
		a.sort === b.sort &&
		a.libraries.length === b.libraries.length &&
		a.libraries.every((slug, i) => slug === b.libraries[i])
	);
}

/* ── 4. the request, and the guard on what came back ──────────────────────── */

/** What one page request carries. `cursor` absent is "the start of this order". */
export interface BrowsePageRequest {
	limit: number;
	cursor?: string;
}

/**
 * One page's query string.
 *
 * `media_type` and `sort` go out on EVERY page, unchanged, because the server
 * does not remember them (§7.5) and because they are part of what the cursor
 * means. `lib` goes out only when there is a slug to put in it: see
 * `readLibraryScope` for why an empty one is a 400 rather than "no scope".
 *
 * It is the ONLY builder for this endpoint, and that is what `browseEchoDrift`
 * below is able to assume when it treats these three as "what the client meant".
 */
export function browseParams(query: BrowseQuery, request: BrowsePageRequest): URLSearchParams {
	const params = new URLSearchParams({ sort: query.sort, limit: String(request.limit) });
	// ⚠️ OMITTED, NEVER SENT EMPTY, FOR THE ALL-TYPES VIEW. `?media_type=` with
	// nothing in it happens to be silently unfiltered today (the handler trims
	// and tests for `!== ""`), so it would work — but it would be a client
	// leaning on a parameter's empty spelling meaning the same as its absence,
	// which is exactly the reading that makes `?lib=` a 400. One rule for both:
	// a filter with nothing to say does not appear in the query string.
	if (query.mediaType !== undefined) params.set('media_type', query.mediaType);
	if (query.libraries.length > 0) params.set('lib', query.libraries.join(','));
	// The cursor is set through URLSearchParams rather than concatenated: it is
	// base64url over a payload that can embed a SQLite datetime with a literal
	// space in it, and `$lib/library` records the unescaped form arriving as a
	// panic rather than as a wrong answer.
	if (request.cursor !== undefined) params.set('cursor', request.cursor);
	return params;
}

/** The URL for one page. Pure, and deliberately so: the guard is on what comes
 * BACK (`browseEchoDrift`), not on what goes out. */
export function browseRequestUrl(query: BrowseQuery, request: BrowsePageRequest): string {
	return `${LIBRARY_BROWSE_URL}?${browseParams(query, request).toString()}`;
}

/**
 * WHAT THE SERVER SAID IT ACTUALLY APPLIED, read off one browse envelope (§7.4).
 *
 * ⚠️ EACH FIELD IS ABSENT EXACTLY WHEN THE SERVER APPLIED NOTHING, AND THE
 * GUARD BELOW IS ONLY SOUND BECAUSE OF THAT. It is a property of the handler
 * rather than a convenience of this parser: `handleBrowseWorks` fills
 * `media_type` and `sort` only from its own closed allowlists and marks both
 * `omitempty`, and `resolveBrowseLibraries` returns a nil slice ONLY when the
 * request carried no `lib` at all, every other outcome being a 400 or a fully
 * resolved non-empty list. So an absent key cannot mean "you asked and I
 * declined". `TestBrowseEnvelopeOmitsLibOnlyWhenNoScopeWasApplied`
 * (`internal/httpapi/library_browse_test.go`) fails if a third outcome ever
 * appears, which is the day `browseEchoDrift` stops being sound rather than
 * merely noisy.
 */
export interface BrowseEcho {
	mediaType?: string;
	sort?: string;
	/** The `?lib=` slugs the server resolved, in the order sent. Absent means no
	 * library scope was applied, and there is no second reading (§7.4). */
	libraries?: string[];
}

/** The three echoed fields, read defensively: this runs on a response body, so
 * a wrong shape must read as "the server applied nothing" and be reported, not
 * throw on a render path. */
export function readBrowseEcho(payload: unknown): BrowseEcho {
	const echo: BrowseEcho = {};
	if (typeof payload !== 'object' || payload === null) return echo;
	const raw = payload as Record<string, unknown>;
	if (typeof raw.media_type === 'string') echo.mediaType = raw.media_type;
	if (typeof raw.sort === 'string') echo.sort = raw.sort;
	// A non-string inside the list is DROPPED rather than coerced, so a malformed
	// echo comes out unequal to the intent and drifts loudly, instead of being
	// smoothed into something that matches.
	if (Array.isArray(raw.lib)) {
		echo.libraries = raw.lib.filter((s): s is string => typeof s === 'string');
	}
	return echo;
}

/**
 * WHERE THE QUERY THIS CLIENT MEANT AND THE QUERY THE SERVER APPLIED DISAGREE,
 * one sentence per disagreement, empty when they agree.
 *
 * ⚠️ THE SOURCE OF TRUTH IS THE SERVER'S OWN ANSWER, AND WHAT THIS REPLACED WAS
 * NOT. A `BROWSE_PARAMS` allowlist plus an `unknownBrowseParams` filter used to
 * compare the outgoing query string against a hard-coded list of parameter
 * names. Both are gone, and the list was NOT kept as a cheaper second check,
 * because it was not a second check:
 *
 *   It caught nothing this misses. §7.1 makes a recognised name with an
 *   unrecognised value a 400, so the only thing a typo can silently lose is a
 *   parameter WHOLE. Every such loss arrives here as an echo that is absent or
 *   unequal. There was no drift the name list saw and this does not.
 *
 *   It went stale in the direction that gets guards deleted. The list rots when
 *   the SERVER grows a parameter, not when this module does, so its next wrong
 *   answer would have been a `console.error` on a perfectly correct query. A
 *   guard that cries wolf on valid input is uninstalled by whoever meets it
 *   third.
 *
 *   The hazard it was written against is structural now. `browseParams` is the
 *   only builder, its keys are literals in one function in this file, and a
 *   test asserts the whole query string it produces. An allowlist over input
 *   that is itself a constant guards its own author.
 *
 * WHAT IS GIVEN UP, named rather than glossed: this fires one round trip later
 * than the list did, and it says nothing about `limit` or `cursor`. Neither can
 * be compared to an intent. `limit` is clamped by design (§1.2), so an unequal
 * echo there is the contract rather than a defect; a dropped `cursor` is page
 * one over again, which `nextBrowsePage`'s stop rule already reasons about.
 *
 * Pure and exported for its own sake so a test can fire it at a disagreeing
 * echo as well as an agreeing one: a guard that only ever sees the happy case
 * passes a suite that proves nothing.
 */
export function browseEchoDrift(query: BrowseQuery, echo: BrowseEcho): string[] {
	const drift: string[] = [];
	// `browseParams` sends both of these on every page, so intent is never
	// "unset" and an absent echo is always a disagreement.
	// The comparison is `!==` over two `string | undefined`s and holds in BOTH
	// directions without a special case: an all-types query means `undefined`,
	// and the handler omits the key exactly when it applied no type filter, so
	// agreement on the unfiltered view is `undefined === undefined`. Only the
	// WORDS need to know which side is which.
	if (echo.mediaType !== query.mediaType) {
		drift.push(
			`media_type: this client asked for ${query.mediaType ?? 'every media type'}, ` +
				`the server applied ${echo.mediaType ?? 'no type filter at all'}`
		);
	}
	if (echo.sort !== query.sort) {
		drift.push(
			`sort: this client asked for ${query.sort}, the server applied ` +
				`${echo.sort ?? 'its own default order'}`
		);
	}
	const wanted = query.libraries;
	const applied = echo.libraries;
	if (wanted.length === 0) {
		// An empty scope is sent as NO `lib` at all, so an echo here means the
		// server scoped a page this client meant to be over every library.
		if (applied !== undefined) {
			drift.push(
				`lib: this client asked for every library, the server scoped to ` +
					`${applied.join(', ') || 'an empty list'}`
			);
		}
	} else if (applied === undefined) {
		drift.push(
			`lib: this client asked for ${wanted.join(', ')}, the server applied no library scope`
		);
	} else if (applied.length !== wanted.length || applied.some((s, i) => s !== wanted[i])) {
		drift.push(
			`lib: this client asked for ${wanted.join(', ')}, the server applied ${applied.join(', ')}`
		);
	}
	return drift;
}

/**
 * The dev-only half: name the disagreement, say why it matters, and DO NOT
 * THROW.
 *
 * Throwing would take down a screen over a query-string defect, and a guard
 * that can break the app is a guard people delete. `import.meta.env.DEV` is a
 * literal `false` in a production build, so Rollup drops this call and the
 * function with it, and no user-facing error path is added by any of it.
 */
function warnBrowseEchoDrift(query: BrowseQuery, payload: unknown): void {
	const drift = browseEchoDrift(query, readBrowseEcho(payload));
	if (drift.length === 0) return;
	console.error(
		`[UsArr library] GET ${LIBRARY_BROWSE_URL} answered 200 without applying the query ` +
			`this screen asked for, so the grid below is a corpus nobody selected: ` +
			`${drift.join('; ')}. An unrecognised parameter is IGNORED by this endpoint ` +
			'rather than refused, so a misspelt name looks exactly like this: a full page ' +
			'under the wrong heading. The echoed fields are docs/reference/http-api.md §7.4.'
	);
}

/** One page of the grid, with the dev-only echo guard on the way back in. A
 * local SQLite read behind the endpoint: it makes no upstream call and is never
 * on a path that waits for a service. */
export async function fetchBrowsePage(
	query: BrowseQuery,
	request: BrowsePageRequest
): Promise<RecentPage> {
	const payload = await getJson(browseRequestUrl(query, request));
	if (import.meta.env.DEV) warnBrowseEchoDrift(query, payload);
	return toRecentPage(payload, request.limit);
}

/* ── 5. the feed, and the cursor-reset rule ───────────────────────────────── */

/**
 * EVERY PAGE READ SO FAR, THE QUERY THEY WERE READ UNDER, AND THE PAGE SIZE THE
 * SERVER SAID IT APPLIED.
 *
 * It is `$lib/library`'s `RecentFeed` plus those two fields, and both are here
 * because of a property of THIS endpoint that Block C does not have:
 *
 *   `query`  a cursor is minted under one `sort` and refused under another, and
 *            is minted under one `media_type` and one `lib` and SILENTLY
 *            ACCEPTED under others (§7.5). The query therefore travels with the
 *            cursor, so the reset can be decided rather than remembered.
 *   `limit`  the server clamps the page size and echoes what it applied (§1.2,
 *            which governs both endpoints). A client that keeps asking for its
 *            own number instead of the echoed one is paging against a size the
 *            server never agreed to.
 *
 * There is deliberately no `hasMore` boolean: "is there more" is
 * `cursor !== undefined` and nothing else, and two fields that must agree are
 * two fields that can disagree.
 */
export interface BrowseFeed {
	items: RecentItem[];
	/** The cursor for the NEXT page. Absent = this is the last page. */
	cursor?: string;
	/** Whether the server has answered at least once for THIS query. */
	loaded: boolean;
	/** The page size the SERVER applied, once it has said. */
	limit?: number;
	/** The query every item here was read under. */
	query: BrowseQuery;
}

export function emptyBrowseFeed(query: BrowseQuery): BrowseFeed {
	return { items: [], loaded: false, query };
}

/**
 * ⚠️ THE CURSOR-RESET RULE, AND NOTHING SERVER-SIDE ENFORCES IT.
 *
 * A cursor carries its sort and is refused under another one — that failure is
 * loud and is a 400. It carries NEITHER the media type NOR the library scope,
 * so replaying it after the user changes either is a `200 OK` whose page starts
 * partway into a different corpus: rows are skipped, the count is wrong, and
 * nothing anywhere reports a problem. The endpoint cannot catch it, because
 * from its side the request is well formed.
 *
 * So the feed is dropped whole — items, cursor and echoed limit — the moment
 * any part of the query changes, and this function is the only thing that says
 * so. Keeping the rows while dropping the cursor would be worse than either:
 * the screen would show one type's rows under another type's heading.
 */
export function browseFeedFor(feed: BrowseFeed, query: BrowseQuery): BrowseFeed {
	return sameBrowseQuery(feed.query, query) ? feed : emptyBrowseFeed(query);
}

/**
 * THE NEXT PAGE TO ASK FOR, OR `undefined` WHEN THERE IS NONE.
 *
 * ⚠️ THE STOP RULE IS `next_cursor`'s ABSENCE AND NEVER A SHORT PAGE.
 * `ListWorks` walks the dated range and, at the one boundary where that range
 * runs out, issues a second statement for the undated tail — so under
 * `added_at` a page can legitimately come back shorter than the limit with rows
 * still to come. A client that stopped on `items.length < limit` would truncate
 * the grid at exactly the row whose upstream reported no creation date.
 *
 * ⚠️ AND THE PAGE SIZE IS THE ECHOED ONE. `fallback` is only ever used before
 * the server has said anything; after that the request carries what the server
 * itself applied, so a clamp cannot leave the client asking for a size it will
 * not get.
 */
export function nextBrowsePage(feed: BrowseFeed, fallback: number): BrowsePageRequest | undefined {
	const limit = feed.limit ?? fallback;
	if (!feed.loaded) return { limit };
	if (feed.cursor === undefined) return undefined;
	return { limit, cursor: feed.cursor };
}

/** Whether "Load more" has anything to press for. Derived, never stored. */
export function browseHasMore(feed: BrowseFeed): boolean {
	return feed.cursor !== undefined;
}

/** The feed with one page appended, carrying the server's echoed page size
 * forward. Immutable, so a Svelte `$state` assignment is what re-renders rather
 * than a mutation nothing observes. */
export function appendBrowsePage(feed: BrowseFeed, page: RecentPage): BrowseFeed {
	const next: BrowseFeed = {
		items: [...feed.items, ...page.items],
		loaded: true,
		limit: page.limit,
		query: feed.query
	};
	if (page.nextCursor !== undefined) next.cursor = page.nextCursor;
	return next;
}

/* ── 6. the empty state ───────────────────────────────────────────────────── */

/**
 * WHY THIS GRID IS EMPTY, AND THE ANSWER IS NOT ALWAYS ABOUT THIS TYPE.
 *
 * Two different facts wear the same empty table, and telling them apart is the
 * whole job:
 *
 *   nothing is catalogued at all   the reason is upstream and belongs to
 *                                  `$lib/libraryscreen`, which already words it
 *                                  four ways from the services read. Saying "no
 *                                  comics" to a user with no service connected
 *                                  would answer a question about the library
 *                                  with a fact about a filter.
 *   the catalogue has none of THIS the filter is responsible, and the words say
 *                                  which filter — the type alone, or the type
 *                                  inside a library scope.
 *
 * `mode === 'library'` is the only case where a library-bearing service is
 * connected and an empty answer is therefore genuinely about this type; the
 * other three are `recentEmptyState`'s, verbatim, so the two screens cannot
 * drift into two stories about one install.
 */
export function browseEmptyState(query: BrowseQuery, mode: HomeMode | undefined): RecentEmptyState {
	if (mode !== 'library') return recentEmptyState(mode);
	// ⚠️ THE ALL-TYPES VIEW HAS NO TYPE TO BLAME, so an empty answer under a
	// scope is about the SCOPE and an empty answer without one is about the
	// catalogue — which is `recentEmptyState`'s question, verbatim, rather than a
	// fifth wording of it. Saying "no items of this type" on a view that filters
	// by no type would invent a filter to explain an absence that has none.
	if (query.mediaType === undefined) {
		if (query.libraries.length === 0) return recentEmptyState(mode);
		return {
			title: 'Nothing in this scope',
			text:
				'UsArr has catalogued nothing in the libraries this view is scoped to. The scope is part ' +
				'of the address, so clearing it searches every library instead. A library also holds ' +
				'only what its own sources reported.'
		};
	}
	const type = mediaTypeLabel(query.mediaType).toLowerCase();
	if (query.libraries.length > 0) {
		return {
			title: `No ${type} in this scope`,
			text:
				`UsArr has catalogued no ${type} in the libraries this view is scoped to. The scope is ` +
				'part of the address, so clearing it searches every library instead. A library also ' +
				'holds only what its own sources reported.'
		};
	}
	return {
		title: `No ${type} catalogued yet`,
		text:
			`A library-bearing service is connected and UsArr has written no ${type} rows from it. ` +
			'Either nothing it reports is of this type, or its first full import has not finished: ' +
			'that import starts on its own the first time UsArr connects to a service that has never ' +
			'completed one, and on a large library it runs for minutes rather than seconds.'
	};
}
