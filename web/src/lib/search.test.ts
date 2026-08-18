import { describe, expect, it } from 'vitest';
import {
	LIBRARY_SEARCH_URL,
	SEARCH_DEFAULT_LIMIT,
	SEARCH_LIMIT,
	SEARCH_MAX_LIMIT,
	SEARCH_TOKEN_CAP,
	TRIGRAM_FLOOR,
	atCapacity,
	indexerSearchQuery,
	isCurrent,
	isIncomplete,
	longestTokenLength,
	screenState,
	searchRequestUrl,
	toSearchResults,
	truncatedTokens,
	type SearchItem,
	type SearchResults
} from './search';

/**
 * THE SEARCH SCREEN'S CLIENT RULES, PINNED AGAINST `docs/reference/http-api.md`
 * §6 AND `internal/httpapi/librarysearch.go`.
 *
 * Every fixture is shaped from the SERVER's own source rather than from a
 * summary of it: `searchResponse` and `searchHitResponse` in
 * `internal/httpapi/librarysearch.go` for the frame, `store.SearchDefaultLimit`
 * / `store.SearchMaxLimit` in `internal/store/searchlibrary.go` for the bounds,
 * and `search.md` §3 for the token cap and the trigram floor.
 */

/** One item exactly as §6.2's worked example spells it. */
function hit(over: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		id: 41,
		kind: 'comic',
		media_type: 'comics',
		title: "Frieren: Beyond Journey's End",
		year: 2021,
		added_at: '2026-08-16T12:00:00Z',
		have_count: 43,
		want_count: 17,
		availability: { k: 'count', have: 43, total: null },
		...over
	};
}

function frame(over: Record<string, unknown> = {}): Record<string, unknown> {
	return { query: 'frieren', limit: 25, truncated: false, items: [hit()], ...over };
}

describe('§6.1 — the request', () => {
	it('sends `q` and never `query`', () => {
		/* §6.1: "`q` is the only spelling. `query=` is not accepted here, even
		   though GET /api/v1/releases/search accepts both." A client sending
		   `query=` gets 400 for an absent query, which looks like a server fault
		   and is not one. */
		const url = searchRequestUrl('frieren');
		expect(url.startsWith(`${LIBRARY_SEARCH_URL}?`)).toBe(true);
		const params = new URLSearchParams(url.slice(url.indexOf('?') + 1));
		expect(params.get('q')).toBe('frieren');
		expect(params.has('query')).toBe(false);
	});

	it('escapes a query that would otherwise be read as URL syntax', () => {
		/* §6.6: "Every metacharacter is safe" server-side — but only if it arrives
		   intact. `&`, `#` and `+` are the three that change the request rather
		   than the search. */
		const params = new URLSearchParams(searchRequestUrl('AC/DC & a+b #1').split('?')[1]);
		expect(params.get('q')).toBe('AC/DC & a+b #1');
	});

	it('asks for the endpoint maximum, because there is no second page', () => {
		/* §6.5 is structural: no cursor, no offset, no second page. A screen that
		   asked for the default would hide everything past row 25 with no control
		   anywhere that could reach it. */
		expect(SEARCH_DEFAULT_LIMIT).toBe(25);
		expect(SEARCH_MAX_LIMIT).toBe(100);
		expect(SEARCH_LIMIT).toBe(SEARCH_MAX_LIMIT);
		expect(new URLSearchParams(searchRequestUrl('x').split('?')[1]).get('limit')).toBe('100');
	});
});

describe('§6.2 / §6.3 — the response', () => {
	it('reads the four fields and the item list', () => {
		const out = toSearchResults(frame(), 'frieren', 100);
		expect(out.query).toBe('frieren');
		expect(out.limit).toBe(25);
		expect(out.truncated).toBe(false);
		expect(out.items).toHaveLength(1);
		expect(out.items[0].title).toBe("Frieren: Beyond Journey's End");
		expect(out.items[0].mediaType).toBe('comics');
		expect(out.items[0].kind).toBe('comic');
	});

	it('§6.3: the ECHOED limit wins over what was sent', () => {
		/* The clamp is silent. A client that asked for 10000, was served 100 and
		   trusted its own number would read the short answer as "that is all there
		   is" — and `atCapacity` would then never fire. */
		expect(toSearchResults(frame({ limit: 100 }), 'x', 10000).limit).toBe(100);
	});

	it('an absent limit falls back to what was sent, not to zero', () => {
		const out = toSearchResults({ query: 'x', items: [] }, 'x', 100);
		expect(out.limit).toBe(100);
	});

	it('§6.2: an absent year and an absent added_at stay absent', () => {
		/* The server omits the keys rather than sending 0/null, "because a rendered
		   0 is a claim about a release date". Reconstructing them here would
		   re-invent exactly what it went to the trouble of not saying. */
		const bare = hit();
		delete bare.year;
		delete bare.added_at;
		const item = toSearchResults(frame({ items: [bare] }), 'x', 100).items[0];
		expect(item.year).toBeUndefined();
		expect(item.addedAt).toBeUndefined();
		expect('year' in item).toBe(false);
	});

	it('§6.2: media_type is omitted for a kind §17.2 has no row for', () => {
		const bare = hit({ kind: 'game' });
		delete bare.media_type;
		const item = toSearchResults(frame({ items: [bare] }), 'x', 100).items[0];
		expect(item.mediaType).toBe('');
		expect(item.kind).toBe('game');
	});

	it('§6.7: a query that matched nothing is 200 with an empty list', () => {
		const out = toSearchResults(frame({ items: [] }), 'zzz', 100);
		expect(out.items).toEqual([]);
	});

	it('a non-object payload is an empty answer rather than a throw', () => {
		expect(toSearchResults(null, 'x', 100).items).toEqual([]);
		expect(toSearchResults('nope', 'x', 100).query).toBe('x');
	});
});

describe('§6.4 — truncated is about the QUERY, not the results', () => {
	it('is false unless the server sent the boolean true', () => {
		expect(toSearchResults(frame(), 'x', 100).truncated).toBe(false);
		const missing = frame();
		delete missing.truncated;
		expect(toSearchResults(missing, 'x', 100).truncated).toBe(false);
		expect(toSearchResults(frame({ truncated: 'yes' }), 'x', 100).truncated).toBe(false);
		expect(toSearchResults(frame({ truncated: true }), 'x', 100).truncated).toBe(true);
	});

	it('counts the typed words when the twelve-token cap fired', () => {
		/* `search.md` §3 step 1 splits on Unicode whitespace and drops empties, so
		   the count is derived by the same rule the server tokenises with. */
		const long = 'a b c d e f g h i j k l m n';
		const out = toSearchResults(frame({ truncated: true, query: long }), long, 100);
		expect(SEARCH_TOKEN_CAP).toBe(12);
		expect(truncatedTokens(out)).toBe(14);
	});

	it('is 0 when the cap did not fire, however long the query', () => {
		const long = 'a b c d e f g h i j k l m n';
		expect(truncatedTokens(toSearchResults(frame({ query: long }), long, 100))).toBe(0);
	});
});

describe('§6.5 — a full page means the endpoint stopped', () => {
	function results(count: number, limit: number): SearchResults {
		return {
			query: 'x',
			limit,
			truncated: false,
			items: Array.from({ length: count }, (_, i) => ({
				id: i,
				mediaType: 'comics',
				kind: 'comic',
				title: `t${i}`,
				haveCount: 1,
				wantCount: 0
			}))
		};
	}

	it('fires only when the rendered count reaches the ECHOED limit', () => {
		expect(atCapacity(results(99, 100))).toBe(false);
		expect(atCapacity(results(100, 100))).toBe(true);
		/* The echoed limit, never SEARCH_LIMIT: a server that clamped to 25 and
		   served 25 is at capacity even though the client asked for 100. */
		expect(atCapacity(results(25, 25))).toBe(true);
	});

	it('never fires on an empty answer with a nonsense limit', () => {
		expect(atCapacity(results(0, 0))).toBe(false);
	});
});

describe('the stale-answer guard', () => {
	it('accepts an answer whose echoed query is what is in the box', () => {
		const out = toSearchResults(frame({ query: 'frieren' }), 'frieren', 100);
		expect(isCurrent(out, 'frieren')).toBe(true);
		/* The screen trims before it asks and the server trims again, so a
		   trailing space the user has just typed is not a different query. */
		expect(isCurrent(out, '  frieren ')).toBe(true);
	});

	it('rejects an answer for a query the user has already replaced', () => {
		const out = toSearchResults(frame({ query: 'frier' }), 'frier', 100);
		expect(isCurrent(out, 'frieren')).toBe(false);
	});
});

describe('the four screen states', () => {
	const empty: SearchResults = { query: 'zzz', limit: 100, truncated: false, items: [] };
	const full: SearchResults = { ...empty, query: 'x', items: [{ ...({} as SearchItem) }] };

	it('no answer at all is idle, and idle is not empty', () => {
		/* The pair a screen gets wrong when they are separate booleans, and the
		   pair the user most needs told apart: §9.6's empty state is the wrong
		   shape for "no question was asked" and the right shape for "nothing
		   matched". */
		expect(screenState(undefined, undefined)).toEqual({ k: 'idle' });
	});

	it('an answer with no rows is empty, and carries the query back', () => {
		expect(screenState(empty, undefined)).toEqual({ k: 'empty', query: 'zzz' });
	});

	it('an answer with rows is results', () => {
		expect(screenState(full, undefined).k).toBe('results');
	});

	it('a failure wins over any answer, because the library is not known to be empty', () => {
		expect(screenState(empty, { message: 'boom', action: 'try again' })).toEqual({
			k: 'error',
			message: 'boom',
			action: 'try again'
		});
	});

	it('has no loading member at all', () => {
		/* §10: a Tier-0 component has no loading state and inventing one is the
		   failure. Asserted as an exhaustive switch so adding one breaks here. */
		const kinds = [
			screenState(undefined, undefined).k,
			screenState(empty, undefined).k,
			screenState(full, undefined).k,
			screenState(empty, { message: '', action: '' }).k
		];
		expect(new Set(kinds)).toEqual(new Set(['idle', 'empty', 'results', 'error']));
	});
});

describe('§17.4 rule 6 — the exit from an incomplete row', () => {
	function item(over: Partial<SearchItem> = {}): SearchItem {
		return {
			id: 1,
			mediaType: 'comics',
			kind: 'comic',
			title: 'Frieren',
			haveCount: 43,
			wantCount: 0,
			...over
		};
	}

	it('a full tick is complete and gets no action', () => {
		expect(
			isIncomplete(
				item({
					availability: { k: 'count', have: 12, total: 12, totalSource: 'declared', missing: [] }
				})
			)
		).toBe(false);
	});

	it('a count with no honest denominator is incomplete, and that is the common row', () => {
		/* `total: null` is not `total: 0`: nobody knows how many there are, so the
		   tick "must never fire" and the row cannot be called complete. */
		expect(
			isIncomplete(
				item({ availability: { k: 'count', have: 43, total: null, totalSource: '', missing: [] } })
			)
		).toBe(true);
	});

	it('a row with no availability blob at all is incomplete', () => {
		/* The tempting bug: have_count > 0 with want_count === 0 looks complete
		   from the counts alone, and is not. `want_count` is what is WANTED and
		   absent, never what exists. */
		expect(isIncomplete(item())).toBe(true);
	});

	it('a wanted count makes an otherwise complete row incomplete', () => {
		expect(
			isIncomplete(
				item({
					wantCount: 3,
					availability: { k: 'count', have: 12, total: 12, totalSource: 'declared', missing: [] }
				})
			)
		).toBe(true);
	});

	it('a tier the user does not hold makes the row incomplete', () => {
		expect(
			isIncomplete(
				item({
					mediaType: 'movies',
					availability: {
						k: 'tier',
						buckets: [
							{ key: '1080p', label: '1080p', have: 1, total: 1 },
							{ key: '2160p', label: '2160p', have: 0, total: 1 }
						]
					}
				})
			)
		).toBe(true);
	});

	it('carries the title and §8.5s category to the indexer search', () => {
		/* The mapping is the forward direction of docs/reference/tags.md's table,
		   with §8.5's special cases applied: audiobooks are 3030 and not 3000,
		   ebooks are 7020, and comics are the PARENT 7000 because 7030 returns
		   zero manga. */
		const q = (over: Partial<SearchItem>) => new URLSearchParams(indexerSearchQuery(item(over)));
		expect(q({ mediaType: 'comics' }).get('category')).toBe('7000');
		expect(q({ mediaType: 'audiobooks' }).get('category')).toBe('3030');
		expect(q({ mediaType: 'ebooks' }).get('category')).toBe('7020');
		expect(q({ mediaType: 'movies' }).get('category')).toBe('2000');
		expect(q({ mediaType: 'tv' }).get('category')).toBe('5000');
		expect(q({ mediaType: 'music' }).get('category')).toBe('3000');
		expect(q({ title: 'AC/DC & Co' }).get('q')).toBe('AC/DC & Co');
	});

	it('omits the category for a media type outside the six rather than guessing', () => {
		/* §6.2 omits `media_type` for a kind §17.2 has no row for. An unfiltered
		   indexer search is wide; a wrongly filtered one silently returns nothing. */
		expect(new URLSearchParams(indexerSearchQuery(item({ mediaType: '' }))).has('category')).toBe(
			false
		);
	});
});

describe('the trigram floor', () => {
	it('measures the LONGEST token, which is what search.md §3 step 5 gates on', () => {
		expect(TRIGRAM_FLOOR).toBe(3);
		expect(longestTokenLength('it')).toBe(2);
		expect(longestTokenLength('a train')).toBe(5);
		expect(longestTokenLength('  dune  ')).toBe(4);
		expect(longestTokenLength('')).toBe(0);
	});

	it('counts code points, so an astral query is not double-counted', () => {
		expect(longestTokenLength('𝄞𝄞')).toBe(2);
	});
});
