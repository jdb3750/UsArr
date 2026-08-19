/**
 * SEARCH OVER YOUR OWN LIBRARY, CLIENT SIDE — `GET /api/v1/search`.
 *
 * ARCHITECTURE.md §17.4 is the screen; `docs/reference/http-api.md` §6 is the
 * wire, and §6 is what this file is written against rather than against the Go
 * doc comment, which a browser tab cannot reach.
 *
 * ⚠️ THIS IS NOT THE PROWLARR FAN-OUT AND THE PATH NO LONGER SAYS SO. Until
 * `4a51bd4` the indexer fan-out answered on `/api/v1/search`; it now answers on
 * `GET /api/v1/releases/search`, and `$lib/api.startSearch` is the client for
 * THAT one. The two are different questions over different corpora: this one
 * asks what you HAVE and answers in its body, that one asks indexers what
 * EXISTS and answers `202` with results on the SSE stream. Nothing in this file
 * opens a stream, and nothing in it belongs on the Requests screen.
 *
 * IT IS A TIER 0 LOCAL READ (principle 1). `internal/httpapi/librarysearch.go`
 * says so at its own declaration: two SQLite statements, no *Arr call, no
 * metadata provider, no image fetch. So there is no upstream to wait for, no
 * partial state to model, and — §10 — no loading state to invent.
 *
 * DOM-FREE AND DETERMINISTIC, like `$lib/library` and `$lib/list`, because
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin: a rule
 * that lives inside an `{#if}` in `routes/search/+page.svelte` is a rule
 * nothing can test. Every decision the screen makes about what state it is in,
 * what it asks the server for, and what it may claim about the answer is a
 * function here.
 *
 * ⚠️ THE ITEM SHAPE IS `$lib/library`'s AND IS NOT RE-DECLARED. §6.2: "The item
 * keys are §1.3's item keys plus `score`, deliberately, so one row component
 * renders both Home's recently-added table and a search result", and
 * `TestSearchResponseKeysAreTheAllowlist` in `internal/httpapi` pins that on
 * the server side. `score` (§6.2.1) is the one key Home has no analogue for, it
 * is READ BY NOTHING HERE, and that is correct rather than an omission: it is
 * for the GROUPED presentation §17.4 rule 2 specifies, which is not built, and
 * §6.2.1's first forbidden use is a client re-sorting the rows by it — the
 * server's order already carries a media-type diversity guarantee that sorting
 * by score would silently undo. So `RecentItem`, `toRecentItem`, `haveCell`,
 * `availabilityMark` and `mediaTypeLabel` are IMPORTED here, not copied and not
 * extracted into a third module: extraction would have moved a tested type out
 * from under `library.test.ts` to express a relationship that already holds.
 * The one thing added is a name — `SearchItem` — so a reader of the Search
 * screen is not left wondering why its rows are called "recent".
 */

import { getJson } from './api';
import { haveCell, isMediaType, type MediaType, type RecentItem, toRecentItem } from './library';

/** The endpoint, as `internal/httpapi/server.go:300` routes it. */
export const LIBRARY_SEARCH_URL = '/api/v1/search';

/**
 * §6.3's bounds, which are `store.SearchDefaultLimit` and `store.SearchMaxLimit`
 * (`internal/store/searchlibrary.go:108-109`).
 *
 * They are here as constants rather than as behaviour: the CLAMP lives on the
 * server and the client never re-implements it. See `SEARCH_LIMIT` for what is
 * actually sent, and `SearchResults.limit` for what is believed afterwards.
 */
export const SEARCH_DEFAULT_LIMIT = 25;
export const SEARCH_MAX_LIMIT = 100;

/**
 * ⚠️ THE SCREEN ASKS FOR THE MAXIMUM, AND THAT IS A CORRECTNESS CHOICE RATHER
 * THAN A GREEDY ONE.
 *
 * §6.5: there is no cursor, no offset and no second page, because the store
 * fuses at most 200 candidates and re-ranks the whole of that set in Go, so
 * the ordering is a property of the set and there is no keyset position a
 * cursor could name. "A caller asks for up to 100 results and that is every
 * result this endpoint has."
 *
 * That sentence is what decides this. A screen that asked for the default 25
 * would hide rows 26 upward with NO control anywhere that could reach them —
 * silent truncation, which is the one failure Baymard's finding in §17.4 rule 3
 * names by name. Asking for the documented maximum makes the endpoint's own
 * ceiling the only ceiling, and `atCapacity()` below is what says so out loud
 * on the one query where it binds.
 *
 * The cost is bounded and small: 100 rows is half `LOAD_MORE_PAGE_SIZE`, the
 * measured DOM-row figure `$lib/list` carries, and every row is contained by
 * the list primitive.
 */
export const SEARCH_LIMIT = SEARCH_MAX_LIMIT;

/**
 * One search result.
 *
 * A type ALIAS rather than an extending interface: §6.2 fixes the two key sets
 * as identical, so a second declaration could only ever drift from the first.
 */
export type SearchItem = RecentItem;

/** One answer from `GET /api/v1/search`, as `searchResponse` spells it in
 * `internal/httpapi/librarysearch.go:104-122`. */
export interface SearchResults {
	/**
	 * ⚠️ THE TEXT THE SERVER ACTUALLY SEARCHED, AND IT IS HOW A STALE ANSWER IS
	 * RECOGNISED. `librarysearch.go:107` echoes it "so a client can tell a stale
	 * response from a current one when the user is typing fast". `isCurrent()`
	 * below is the whole of that check; without it two in-flight reads can land
	 * out of order and the screen renders the older one under the newer query.
	 */
	query: string;
	/**
	 * ⚠️ THE CAP THE SERVER APPLIED, AND IT IS AUTHORITATIVE OVER WHAT WAS SENT.
	 * §6.3 is a clamp, not a validated range: `limit=10000` is served as 100 and
	 * echoed as 100. A client that trusted its own number would read the short
	 * answer as "that is all there is".
	 */
	limit: number;
	/**
	 * ⚠️ THE QUERY WAS CUT, NOT THE RESULTS, AND THE DIFFERENCE IS THE WHOLE
	 * POINT OF THE FLAG.
	 *
	 * §6.4, verbatim: it says `search.md` §3 step 1's twelve-token cap fired —
	 * the query had more than twelve whitespace-separated tokens and the search
	 * answered a PREFIX of what was typed — and "it says nothing about how many
	 * results came back."
	 *
	 * So copy of the form "there are more matches than shown" is FALSE on this
	 * field and must never be attached to it. What is true, and what
	 * `truncatedNote()` renders, is that part of the query was not searched.
	 */
	truncated: boolean;
	items: SearchItem[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function num(value: unknown): number | undefined {
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

/**
 * The response, parsed.
 *
 * `sent` is the fallback for `limit` alone, and only for a frame that omitted
 * it: the server sends the key unconditionally, so an absent one is a broken
 * frame rather than a fact, and the least-wrong stand-in is what was asked for.
 * `query` falls back to the text sent for the same reason — an echo that never
 * arrived must not make every later answer look stale.
 */
export function toSearchResults(payload: unknown, sentQuery: string, sent: number): SearchResults {
	if (!isRecord(payload)) return { query: sentQuery, limit: sent, truncated: false, items: [] };
	const raw = Array.isArray(payload.items) ? payload.items : [];
	return {
		query: typeof payload.query === 'string' ? payload.query : sentQuery,
		limit: num(payload.limit) ?? sent,
		// Anything that is not the boolean `true` is not a fired cap. An absent
		// key is the ordinary case and means the query was searched whole.
		truncated: payload.truncated === true,
		items: raw.map(toRecentItem).filter((i): i is SearchItem => i !== undefined)
	};
}

/**
 * The URL for one search.
 *
 * ⚠️ `q` IS THE ONLY SPELLING. §6.1: "`query=` is not accepted here, even though
 * `GET /api/v1/releases/search` accepts both; two spellings of one parameter is
 * a contract a client has to guess at."  `handleLibrarySearch` reads `q` and
 * nothing else (`internal/httpapi/librarysearch.go:135`), so a client sending
 * `query=` gets `400 bad_request` for an absent query — which looks like a
 * server fault and is not one.
 */
export function searchRequestUrl(query: string, limit = SEARCH_LIMIT): string {
	const params = new URLSearchParams({ q: query, limit: String(limit) });
	return `${LIBRARY_SEARCH_URL}?${params.toString()}`;
}

/**
 * One search. A local SQLite read behind the endpoint: it makes no upstream
 * call and is never on a path that waits for a service.
 *
 * `signal` is the as-you-type path's, and it is passed rather than owned:
 * `$lib/livesearch` decides when a read has been superseded, this function only
 * carries the decision to `fetch`. An aborted read rejects with `$lib/api`'s
 * `ApiError` at status 0 — see `getJson` — so a caller that cancels has to know
 * it cancelled and must not draw that as a failure.
 */
export async function fetchSearch(
	query: string,
	limit = SEARCH_LIMIT,
	signal?: AbortSignal
): Promise<SearchResults> {
	return toSearchResults(await getJson(searchRequestUrl(query, limit), signal), query, limit);
}

/**
 * WHETHER AN ANSWER STILL DESCRIBES WHAT IS IN THE BOX.
 *
 * Compared on the trimmed text, because that is what was sent: the screen
 * trims before it asks, and the server trims again (`strings.TrimSpace` at
 * `librarysearch.go:135`), so a trailing space the user has just typed is not
 * a different query and must not discard a good answer.
 */
export function isCurrent(results: SearchResults, typed: string): boolean {
	return results.query === typed.trim();
}

/**
 * THE FOUR STATES THE SCREEN CAN BE IN, AS ONE VALUE.
 *
 * A discriminated union rather than three booleans on the component, for the
 * reason `$lib/library`'s `RecentFeed` gives about `hasMore`: fields that must
 * agree are fields that can disagree, and "no query yet" versus "a query that
 * matched nothing" is exactly the pair a screen gets wrong when they are
 * separate flags. They are also the pair the user most needs told apart —
 * §9.6's empty state is the wrong shape for the first and the right shape for
 * the second.
 *
 * There is deliberately NO `loading` member. This is a Tier 0 local read and
 * §10 is explicit that a Tier-0 component has no loading state at all;
 * inventing one is the failure. While a second read is in flight the screen
 * keeps showing the answer it has, which is both true and still.
 */
export type SearchScreenState =
	/** Nothing has been asked yet. Not an empty result: no question was put. */
	| { k: 'idle' }
	/** The server answered and had nothing. A fact about the library. */
	| { k: 'empty'; query: string }
	/** The server answered with rows. */
	| { k: 'results'; results: SearchResults }
	/** The read failed. The library is not known to be empty; it is unknown. */
	| { k: 'error'; message: string; action: string };

/**
 * The state, derived from the two things the screen actually holds: the last
 * answer (or failure) and nothing else.
 *
 * `results === undefined` is "never answered", which is `idle` whatever the
 * user has typed — a half-typed query with no answer behind it is not an empty
 * library, and drawing §9.6's empty state under it would say it was.
 */
export function screenState(
	results: SearchResults | undefined,
	failure: { message: string; action: string } | undefined
): SearchScreenState {
	if (failure !== undefined) return { k: 'error', ...failure };
	if (results === undefined) return { k: 'idle' };
	if (results.items.length === 0) return { k: 'empty', query: results.query };
	return { k: 'results', results };
}

/**
 * WHETHER THE ANSWER FILLED THE SERVER'S CAP.
 *
 * §6.5 again: there is no second page. A full page therefore means the endpoint
 * has stopped, not that the library has — the fused candidate set runs to 200
 * and the cap is 100 — so the screen says so rather than letting the list end
 * on a row that looks like the last one.
 *
 * Compared against the ECHOED limit, never against `SEARCH_LIMIT`: §6.3 clamps
 * silently, so the echoed value is the only one that describes the answer in
 * hand. `>=` rather than `===` because a server that ever served one more than
 * it echoed would still be at capacity, and reading that as "not full" is the
 * failure direction.
 */
export function atCapacity(results: SearchResults): boolean {
	return results.limit > 0 && results.items.length >= results.limit;
}

/**
 * §6.4'S CAP, AS A NUMBER THE SCREEN CAN PUT IN A SENTENCE.
 *
 * `search.md` §3 step 1 caps the tokeniser at twelve whitespace-separated
 * tokens and the search then answers a prefix of what was typed. The wire
 * carries a boolean, so the count is derived here — by the same split the
 * server tokenises with, so the two agree by construction — and the SENTENCE
 * stays in the route.
 *
 * ⚠️ THAT DIVISION IS DELIBERATE AND IT IS ABOUT THE GUARD, not about tidiness.
 * `designrules.test.ts` rule 9 reads `web/src/routes/search/**` and says so:
 * "if Search's strings move into a `$lib` module the way Requests' did, this
 * rule stops seeing them and says nothing about it." A note phrased here would
 * be Search copy outside Search's corpus, which is the one way to leave rule 7
 * unenforced without ever writing an exception. Numbers travel; words do not.
 */
export const SEARCH_TOKEN_CAP = 12;

/** How many whitespace-separated tokens the user typed. 0 when the cap did not
 * fire, so a caller can branch on the same value it renders. */
export function truncatedTokens(results: SearchResults): number {
	if (!results.truncated) return 0;
	return results.query.split(/\s+/).filter((t) => t !== '').length;
}

/* ── §17.4 rule 6: the exit from a row that is missing something ──────────── */

/**
 * WHETHER THIS ROW IS SHORT OF SOMETHING, WHICH IS WHAT DECIDES RULE 6's ACTION.
 *
 * §17.4 rule 6 defines "incomplete" as "anything that is not a full ✓: a cross,
 * a partial fraction, or a tier the user does not hold". So the test is over
 * the SAME model the Have cell renders — `$lib/library.haveCell` — rather than
 * over the raw counts, because the two would disagree in exactly the case that
 * matters: `have_count > 0` with `want_count === 0` looks complete from the
 * counts and is not, since `want_count` is what is WANTED and absent rather
 * than what exists. `haveCell`'s header spells that trap out at length.
 *
 * A row with capped buckets (`more > 0`) is incomplete by default: the lines
 * that were not rendered were not inspected, and claiming completeness on
 * unexamined data is the tick that §6.3 forbids.
 */
export function isIncomplete(item: SearchItem): boolean {
	const cell = haveCell(item);
	if (cell.more > 0 || cell.missing !== '' || cell.gaps !== '') return true;
	return !cell.lines.every((line) => line.mark.k === 'complete');
}

/**
 * THE NEWZNAB CATEGORY EACH MEDIA TYPE SEARCHES UNDER.
 *
 * §17.4 rule 6 asks for the category to be "preselected from the row's media
 * type (§8.5's five special cases decide the mapping)". This is the forward
 * direction of the table `docs/reference/tags.md` gives — verified there
 * against Prowlarr's own `NewznabStandardCategory.cs` rather than inferred —
 * with §8.5's five special cases applied on top, and each divergence from the
 * parent id is one of those cases:
 *
 *   audiobooks → 3030, NOT 3000. Its parent is `Audio`, so the parent rule
 *                tags every audiobook as music. §8.5 lists this first.
 *   ebooks     → 7020, "the ebook filter proper" in §8.5's words. The parent
 *                7000 is the comics filter below, so the two must not share it.
 *   comics     → 7000, the PARENT, and this one is deliberately not 7030:
 *                §8.5 is explicit that "there is no manga category in the
 *                Newznab standard at all" and that a search filtered on 7030
 *                "returns zero manga", because Nyaa files its Literature
 *                categories under 7000.
 *
 * 🔍 One value per type is this screen's simplification rather than §8.5's
 * rule. §8.5 also accepts 7000/7040/7060 for ebooks and treats 7030 as a
 * ranking signal for comics; both are about how a RESULT is classified, and
 * this is about what a search is filtered to. Sending the extra parents here
 * would widen an ebook search to include every comic.
 */
const NEWZNAB_CATEGORY: Record<MediaType, number> = {
	movies: 2000,
	tv: 5000,
	music: 3000,
	ebooks: 7020,
	audiobooks: 3030,
	comics: 7000
};

/**
 * The query string for §17.5's free-text indexer search, pre-filled from one row.
 *
 * A query string and not a URL: the route composes it with `resolve('/requests')`
 * so the configured base path is applied once, in the place SvelteKit applies
 * it. `q` and `category` are the parameter names the Requests screen already
 * reads on mount, so this is an existing entry point rather than a new one.
 *
 * The category is omitted for a media type outside the six — §6.2 omits
 * `media_type` for a `kind` §17.2 has no row for — rather than guessed at. An
 * unfiltered indexer search is a wide search; a wrongly filtered one silently
 * returns nothing.
 */
export function indexerSearchQuery(item: SearchItem): string {
	const params = new URLSearchParams({ q: item.title });
	if (isMediaType(item.mediaType)) {
		params.set('category', String(NEWZNAB_CATEGORY[item.mediaType]));
	}
	return params.toString();
}

/* ── the short-query floor ────────────────────────────────────────────────── */

/**
 * The longest whitespace-separated token, in characters.
 *
 * §6.6's last paragraph is the reason this is measured at all: "Queries shorter
 * than three characters use only the full-text leg, because SQLite's trigram
 * tokenizer indexes nothing shorter. A query that is a single one-character
 * token matches nothing at all — both legs decline it — and answers 200 with an
 * empty `items`." `search.md` §3 step 5 puts the same floor at the longest
 * token rather than at the whole string, which is what this counts.
 *
 * Measured in code points rather than UTF-16 units, so a query made of astral
 * characters is not double-counted into looking longer than it is.
 */
export function longestTokenLength(query: string): number {
	let longest = 0;
	for (const token of query.trim().split(/\s+/)) {
		longest = Math.max(longest, [...token].length);
	}
	return longest;
}

/** The trigram tokenizer's floor, `search.md` §3 step 5. Below it, only the
 * full-text leg runs and a substring in the middle of a title is not found. */
export const TRIGRAM_FLOOR = 3;
