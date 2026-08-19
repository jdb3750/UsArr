/*
 * THE PER-TYPE LIBRARY GRID, GUARDED — `$lib/librarygrid` and the screen over
 * it.
 *
 * WHY SOME OF IT READS SOURCE. `vitest.config.ts` is `environment: 'node'` with
 * no Svelte plugin, so a component cannot be imported and compiled in a test at
 * all. The repo's answer is already in the tree three times — `home.test.ts`,
 * `havecell.test.ts` and `libraryscreen.test.ts` all read a template through
 * Vite's `?raw` and run a ban list over it — so the rules that live in markup
 * are held that way and everything else is imported and called.
 *
 * ⚠️ EVERY GUARD BELOW ASSERTS THE POSITIVE CASE AS WELL AS THE ABSENCE IT IS
 * ABOUT. The failure mode of a rule that forbids something is finding nothing at
 * all: "A to Z is not offered for music" passes just as well when no sort is
 * offered for anything, and "year is never emitted" passes on a builder that
 * emits no sort. So each one proves it looked.
 */

import { describe, expect, it, vi } from 'vitest';
import { ApiError } from './api';
import { userFacingMarkup } from './copyguard';
import { cursorRejected, MEDIA_TYPES, type MediaType, type RecentPage } from './library';
import { findContentSizedTracks } from './list';
import {
	appendBrowsePage,
	BROWSE_AZ_UNAVAILABLE,
	BROWSE_AZ_UNAVAILABLE_MULTI_KIND,
	browseEchoDrift,
	browseEmptyState,
	browseFeedFor,
	BROWSE_SORTS,
	browseKindCount,
	browseParams,
	browseRequestUrl,
	browseRoute,
	browseSortNote,
	browseSortsFor,
	DEFAULT_BROWSE_SORT,
	emptyBrowseFeed,
	fetchBrowsePage,
	libraryNames,
	libraryScopeLine,
	LIBRARY_BROWSE_URL,
	MAX_LIBRARY_SLUGS,
	nextBrowsePage,
	readBrowseEcho,
	readBrowseSort,
	readLibraryNames,
	readLibraryScope,
	REFUSED_BROWSE_SORT,
	sameBrowseQuery,
	type BrowseFeed,
	type BrowseQuery
} from './librarygrid';
import ROUTE_SOURCE from '../routes/library/[type]/+page.svelte?raw';
import LAYOUT_SOURCE from '../routes/+layout.svelte?raw';
import RECENT_ROUTE_SOURCE from '../routes/library/+page.svelte?raw';
import HOME_SOURCE from '../routes/+page.svelte?raw';

const ROUTE = 'routes/library/[type]/+page.svelte';
const MARKUP = userFacingMarkup(ROUTE_SOURCE);

/**
 * The whole file with every comment blanked, length and line numbers preserved.
 *
 * ⚠️ A RULE CANNOT BE ALLOWED TO FIRE ON ITS OWN DOCUMENTATION, which is the
 * lesson `designrules.test.ts` records and the reason it strips before it scans:
 * the paragraph explaining why `item.kind` must never be read contains the
 * literal `work.kind`. `userFacingMarkup` is the wrong instrument here on its
 * own, because the code this bans lives in the `<script>` it removes.
 */
function stripComments(text: string): string {
	const blank = (m: string) => m.replace(/[^\n]/g, ' ');
	return text
		.replace(/<!--[\s\S]*?-->/g, blank)
		.replace(/\/\*[\s\S]*?\*\//g, blank)
		.replace(/(^|[^:'"\\])\/\/[^\n]*/g, (m, p: string) => p + blank(m.slice(p.length)));
}

const ROUTE_CODE = stripComments(ROUTE_SOURCE);

const query = (over: Partial<BrowseQuery> = {}): BrowseQuery => ({
	mediaType: 'movies',
	sort: 'added_at',
	libraries: [],
	...over
});

const page = (over: Partial<RecentPage> = {}): RecentPage => ({ items: [], limit: 50, ...over });

/** Answers every request from one scripted body. Scoped to the echo guard, which
 * is the only rule in this file that needs a round trip to observe at all. */
function stubFetch(script: () => Response) {
	vi.stubGlobal('fetch', () => Promise.resolve(script()));
}

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'content-type': 'application/json' }
	});
}

/** One `<Tag ... />` off the stripped markup, or a failure that says which. */
function selfClosingTag(markup: string, tag: string): string {
	const open = markup.indexOf(`<${tag}`);
	expect(open, `${ROUTE} no longer renders <${tag}`).toBeGreaterThanOrEqual(0);
	const end = markup.indexOf('/>', open);
	expect(end, `${ROUTE}'s <${tag}> is not self-closing any more`).toBeGreaterThan(open);
	return markup.slice(open, end);
}

/** The `const NAME: … = [ … ];` initialiser, comments and all. */
function declaration(source: string, name: string): string {
	const open = source.indexOf(`const ${name}`);
	expect(open, `${ROUTE} no longer declares ${name}`).toBeGreaterThanOrEqual(0);
	const end = source.indexOf('];', open);
	expect(end, `${ROUTE}'s ${name} is not an array literal any more`).toBeGreaterThan(open);
	return source.slice(open, end);
}

const LIST_TAG = selfClosingTag(MARKUP, 'List');
const COLUMNS = declaration(ROUTE_SOURCE, 'COLUMNS');

describe('A to Z is offered exactly where the store can serve it', () => {
	/*
	 * ⚠️ THE STORE'S CONDITION IS `len(kinds) != 1`, WHICH IS BROADER THAN
	 * "not music". `browseWorksSQL` refuses `sort_title` whenever the media type
	 * does not resolve to exactly one `work.kind`, because `ix_work_kind_sort` is
	 * `(kind, sort_title, id)` and SQLite cannot supply ORDER BY from an index
	 * whose leading column is constrained by `IN`. Music is artists AND albums;
	 * no media type at all is six. This screen never sends the second case — the
	 * route is always one type — so the reachable half of the rule is music, and
	 * it is derived from the kind count rather than from the word.
	 */
	it('withholds it from music and offers it to every single-kind type', () => {
		expect(
			browseSortsFor('music'),
			'A to Z is offered for music, where sort_title is a 400: music is two kinds ' +
				'(artist AND album) and the index needs the kind as an equality'
		).not.toContain('sort_title');

		const single = MEDIA_TYPES.filter((t) => t !== 'music');
		expect(
			single.length,
			'the single-kind list is empty, so the positive half proves nothing'
		).toBe(5);
		for (const type of single) {
			expect(
				browseSortsFor(type),
				`A to Z is missing for ${type}, which is one kind and can be served in that order`
			).toContain('sort_title');
		}
	});

	it('counts kinds the way internal/store/browse.go does', () => {
		expect(browseKindCount('music'), 'music is artist AND album').toBe(2);
		for (const type of MEDIA_TYPES.filter((t) => t !== 'music')) {
			expect(browseKindCount(type), `${type} is not one kind any more`).toBe(1);
		}
	});

	it('never resolves a URL to an order this type cannot be served in', () => {
		expect(
			readBrowseSort(new URLSearchParams('sort=sort_title'), 'music'),
			'?sort=sort_title survived on music, which is a 400 the screen would render as an error'
		).toBe(DEFAULT_BROWSE_SORT);
		expect(
			readBrowseSort(new URLSearchParams('sort=sort_title'), 'comics'),
			'?sort=sort_title was dropped on comics, where it is legal — this guard would pass ' +
				'on a function that dropped every sort'
		).toBe('sort_title');
	});
});

describe('the absence of A to Z is stated on this screen too, in UsArr own words', () => {
	/*
	 * ⚠️ MUSIC USED TO DROP IT SILENTLY. `browseSortsFor` has always kept
	 * `sort_title` off this control for music — the store refuses it whenever the
	 * media type is not exactly one `work.kind`, and `browseKinds` maps `music`
	 * onto `artist` AND `album` — but nothing said so, so the screen showed two
	 * options where its five siblings show three and gave no reason for the
	 * difference. The all-types screen states the same refusal; this one did not.
	 */
	it('gives music a non-empty note and renders it', () => {
		const note = browseSortNote(query({ mediaType: 'music' }));
		expect(
			note,
			'music has no A to Z note again, so the option is missing from the control with ' +
				'nothing on screen to say why'
		).toBeDefined();
		expect(
			(note ?? '').length,
			'the music A to Z note is empty or a stub, so every absence assertion over it ' +
				'passes over nothing'
		).toBeGreaterThan(20);
		expect(
			note,
			'the music note is not the exported string, so a test cannot hold its wording'
		).toBe(BROWSE_AZ_UNAVAILABLE_MULTI_KIND);
		expect(
			MARKUP,
			`${ROUTE} no longer renders the note, so the missing option is unexplained on screen`
		).toContain('{sortNote}');
	});

	/*
	 * ⚠️ THE SERVER'S 400 TEXT MUST NOT BE THE THING THE READER SEES, here for the
	 * same reason as on the all-types screen. `handleBrowseWorks` answers an
	 * unservable sort with ONE shared sentence for TWO refusals: "sort_title needs
	 * a media_type of one kind — not music — and there is no index behind year at
	 * all; added_at and popularity work everywhere". It names a wire parameter and
	 * a column with no index to a reader who asked about neither.
	 *
	 * ⚠️ THE ABSENCE ASSERTIONS ARE PRECEDED BY A PRESENCE ONE, because an absence
	 * assertion over an empty string passes and proves nothing.
	 */
	it('renders none of the words the server 400 uses', () => {
		expect(
			BROWSE_AZ_UNAVAILABLE_MULTI_KIND.length,
			'the music A to Z note is empty, so every absence assertion below passes over nothing'
		).toBeGreaterThan(20);

		// Fragments of `internal/httpapi/library.go`'s ErrUnservableSort arm, quoted
		// from it rather than paraphrased.
		for (const leak of [
			'sort_title',
			'media_type',
			'no index',
			'music',
			'year',
			'added_at',
			'popularity'
		]) {
			expect(
				BROWSE_AZ_UNAVAILABLE_MULTI_KIND.toLowerCase(),
				`the music A to Z note contains "${leak}", which is the server's own 400 wording ` +
					'reaching a reader. That sentence answers two refusals at once and names a wire ' +
					'parameter, a media type and a column to somebody who asked about none of them.'
			).not.toContain(leak);
		}
	});

	/*
	 * ⚠️ TWO SCREENS, TWO SENTENCES. The all-types note opens "this view spans
	 * every media type", which is false of a view that spans exactly one, so
	 * reusing it here would print a claim the screen contradicts. Neither may
	 * collapse into the other.
	 */
	it('does not reuse the all-types sentence', () => {
		expect(
			BROWSE_AZ_UNAVAILABLE.length,
			'the all-types note is empty, so the comparison below proves nothing'
		).toBeGreaterThan(20);
		expect(
			browseSortNote(query({ mediaType: 'music' })),
			'music now prints the all-types sentence, which tells a reader on one media type ' +
				'that this view spans every media type'
		).not.toBe(BROWSE_AZ_UNAVAILABLE);
		expect(
			browseSortNote({ sort: 'added_at', libraries: [] }),
			'the all-types view stopped printing its own sentence'
		).toBe(BROWSE_AZ_UNAVAILABLE);
	});

	/*
	 * ⚠️ THE POSITIVE HALF. A note that printed everywhere would pass every
	 * assertion above, so the five single-kind types are checked to have all three
	 * orders and nothing to explain.
	 */
	it('leaves every single-kind type with three sorts and no note', () => {
		const single = MEDIA_TYPES.filter((t) => t !== 'music');
		expect(single.length, 'the single-kind list is empty, so this guard proves nothing').toBe(5);
		for (const type of single) {
			expect(
				browseSortsFor(type),
				`${type} no longer offers all three orders, and this screen would need a note it ` +
					'does not have'
			).toEqual([...BROWSE_SORTS]);
			expect(
				browseSortNote(query({ mediaType: type })),
				`${type} prints an A to Z note while offering A to Z, so the note fires on the ` +
					'wrong condition'
			).toBeUndefined();
		}
		expect(
			browseSortsFor('music').length,
			'music offers three orders now, so the note above explains an absence that is not there'
		).toBe(2);
	});
});

describe('the order named `year` never reaches the wire', () => {
	/*
	 * `year` is a legal `library.default_sort` value — migration 0005's CHECK
	 * admits it — and it is refused by this endpoint with a message of its own,
	 * because `work.year` has no index and the order would be a temp b-tree over
	 * the whole filtered corpus on every page. A client sending it is most likely
	 * reading a library's own stored default back, so the refusal has to happen
	 * here rather than being reported as a typo.
	 */
	it('is not in the vocabulary', () => {
		expect(BROWSE_SORTS.length, 'the sort vocabulary is empty').toBe(3);
		expect(BROWSE_SORTS as readonly string[], 'year is offered as an order').not.toContain(
			REFUSED_BROWSE_SORT
		);
		expect(REFUSED_BROWSE_SORT, 'the refused order is no longer year').toBe('year');
	});

	it('is dropped from a URL rather than passed through', () => {
		for (const type of MEDIA_TYPES) {
			expect(
				readBrowseSort(new URLSearchParams('sort=year'), type),
				`?sort=year survived on ${type}`
			).toBe(DEFAULT_BROWSE_SORT);
		}
	});

	it('cannot be emitted, while a legal order can', () => {
		for (const type of MEDIA_TYPES) {
			const params = browseParams(
				{
					mediaType: type,
					sort: readBrowseSort(new URLSearchParams('sort=year'), type),
					libraries: []
				},
				{ limit: 50 }
			);
			expect(params.get('sort'), `sort=year was emitted for ${type}`).not.toBe('year');
		}
		expect(
			browseParams(query({ mediaType: 'comics', sort: 'sort_title' }), { limit: 50 }).get('sort'),
			'no sort is emitted at all, so the ban above is asserting nothing'
		).toBe('sort_title');
	});
});

describe('the request carries what the server does not remember', () => {
	it('sends media_type and sort on every page', () => {
		const params = browseParams(query({ mediaType: 'ebooks', sort: 'popularity' }), {
			limit: 200,
			cursor: 'MXR2My5iZXJzZXJr'
		});
		expect(params.get('media_type')).toBe('ebooks');
		expect(params.get('sort')).toBe('popularity');
		expect(params.get('limit')).toBe('200');
		expect(params.get('cursor')).toBe('MXR2My5iZXJzZXJr');
	});

	/*
	 * ⚠️ THE ONE ASYMMETRY ON THIS ENDPOINT. An empty `media_type` is silently
	 * unfiltered; an empty `lib` is a 400, because the server tests PRESENCE
	 * rather than emptiness — `?lib=`, `?lib=%20` and `?lib=,,` are all "a scope
	 * was asked for and nothing was named". So the parameter is omitted, never
	 * emitted empty.
	 */
	it('omits lib entirely rather than sending an empty one', () => {
		const none = browseParams(query({ libraries: [] }), { limit: 50 });
		expect(none.has('lib'), '?lib= was emitted with nothing in it, which is a 400').toBe(false);
		const some = browseParams(query({ libraries: ['kids', 'vinyl-rips'] }), { limit: 50 });
		expect(some.get('lib'), 'a real scope is not emitted, so the guard above proves nothing').toBe(
			'kids,vinyl-rips'
		);
	});

	it('reads only the empty spellings out of ?lib=', () => {
		for (const raw of ['lib=', 'lib=%20', 'lib=,,', 'lib=+,+']) {
			expect(readLibraryScope(new URLSearchParams(raw)), `${raw} produced a scope`).toEqual([]);
		}
		expect(
			readLibraryScope(new URLSearchParams('lib=kids,%20vinyl-rips')),
			'a real ?lib= produced no slugs, so the empty cases above prove nothing'
		).toEqual(['kids', 'vinyl-rips']);
	});

	/*
	 * ⚠️ THIS REPLACED A GUARD THAT COMPARED THE OUTGOING QUERY AGAINST A
	 * HARD-CODED `BROWSE_PARAMS` LIST. The list could only ever go stale in the
	 * direction that makes a guard fire on a CORRECT query, and it caught nothing
	 * the echo does not: §7.1 makes a recognised name with an unrecognised value
	 * a 400, so the only thing a typo can silently lose is a whole parameter, and
	 * a whole parameter lost is exactly an echo that comes back absent.
	 * `browseEchoDrift`'s own header carries the argument.
	 */
	it('reads the three echoed fields, and reads an absent one as absent', () => {
		expect(readBrowseEcho({ media_type: 'comics', sort: 'sort_title', lib: ['manga'] })).toEqual({
			mediaType: 'comics',
			sort: 'sort_title',
			libraries: ['manga']
		});
		/*
		 * §7.4: OMITTED MEANS THE SERVER APPLIED NOTHING, with no second reading,
		 * and that is the property the whole comparison rests on. Server-side it
		 * is pinned by TestBrowseEnvelopeOmitsLibOnlyWhenNoScopeWasApplied — for
		 * any request that carried `lib`, the answer is a 400 or a 200 that
		 * echoes it, and there is no third outcome.
		 */
		expect(readBrowseEcho({ items: [], limit: 50 })).toEqual({});
		// A body that is not an envelope at all reads as "nothing was applied",
		// so it drifts loudly rather than throwing on a render path.
		expect(readBrowseEcho('not an envelope')).toEqual({});
		expect(readBrowseEcho(null)).toEqual({});
	});

	it('says nothing when the server applied the query this client sent', () => {
		const q = query({ mediaType: 'comics', sort: 'sort_title', libraries: ['manga', 'kids'] });
		expect(
			browseEchoDrift(q, {
				mediaType: 'comics',
				sort: 'sort_title',
				libraries: ['manga', 'kids']
			}),
			'the guard accused a server that applied exactly what was asked of it'
		).toEqual([]);
		// The unscoped case agrees by ABSENCE, which is the half a naive
		// comparison gets wrong: no `lib` was sent, so no `lib` must come back.
		expect(
			browseEchoDrift(query(), { mediaType: 'movies', sort: 'added_at' }),
			'an unscoped page was accused of losing a scope it never asked for'
		).toEqual([]);
	});

	it('names each parameter the server did not apply, which is what a typo looks like', () => {
		// A misspelt `media_type` is ignored, so the page comes back unfiltered
		// and the envelope echoes no type at all.
		const typeOnly = browseEchoDrift(query({ mediaType: 'comics' }), { sort: 'added_at' });
		expect(
			typeOnly,
			'an unapplied media_type was not reported, so a typo in it is invisible'
		).toHaveLength(1);
		const [typeDrift] = typeOnly;
		expect(typeDrift).toContain('media_type');
		expect(typeDrift).toContain('comics');
		expect(typeDrift).toContain('no type filter at all');

		// A misspelt `sort` leaves the server on its own default order, which is
		// a different corpus order under the same heading.
		const sortOnly = browseEchoDrift(query({ sort: 'popularity' }), { mediaType: 'movies' });
		expect(sortOnly, 'an unapplied sort was not reported').toHaveLength(1);
		const [sortDrift] = sortOnly;
		expect(sortDrift).toContain('sort');
		expect(sortDrift).toContain('popularity');

		// A misspelt `lib` is the dangerous one: the scope WIDENS to the whole
		// catalogue and the screen looks like it worked.
		const libOnly = browseEchoDrift(query({ libraries: ['kids'] }), {
			mediaType: 'movies',
			sort: 'added_at'
		});
		expect(
			libOnly,
			'a dropped library scope was not reported, and a dropped scope WIDENS the page'
		).toHaveLength(1);
		const [libDrift] = libOnly;
		expect(libDrift).toContain('lib');
		expect(libDrift).toContain('kids');
		expect(libDrift).toContain('no library scope');

		// Wrong is not the same as missing: a scope that came back as a DIFFERENT
		// set is drift too, and so is one that arrived on an unscoped request.
		expect(
			browseEchoDrift(query({ libraries: ['kids'] }), {
				mediaType: 'movies',
				sort: 'added_at',
				libraries: ['adults']
			})
		).toHaveLength(1);
		expect(
			browseEchoDrift(query(), { mediaType: 'movies', sort: 'added_at', libraries: ['kids'] })
		).toHaveLength(1);

		// All three at once are named at once, not one at a time.
		expect(
			browseEchoDrift(query({ mediaType: 'comics', sort: 'popularity', libraries: ['kids'] }), {})
		).toHaveLength(3);
	});

	/*
	 * ⚠️ THE GUARD IS FIRED HERE, NOT JUST DECLARED. `import.meta.env.DEV` is
	 * true under vitest, so `fetchBrowsePage` really runs `warnBrowseEchoDrift`
	 * in this suite; spying on console.error is what proves the WIRING exists
	 * rather than only the predicate. `$lib/list`'s content-sized-track guard is
	 * pinned the same way and for the same reason.
	 */
	it('shouts in dev when the answer disagrees with the request, and stays quiet when it agrees', async () => {
		const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
		try {
			// Agreement first, so the noisy case below cannot be a guard that has
			// no off switch: a guard that cannot go quiet is as broken as one that
			// cannot speak.
			stubFetch(() =>
				jsonResponse({ items: [], limit: 50, media_type: 'comics', sort: 'added_at' })
			);
			await fetchBrowsePage(query({ mediaType: 'comics' }), { limit: 50 });
			expect(
				spy,
				'the guard fired on a response that applied the whole query'
			).not.toHaveBeenCalled();

			// Now the typo case: the envelope echoes nothing, because the server
			// ignored a name it did not recognise and served page one unfiltered.
			stubFetch(() => jsonResponse({ items: [], limit: 50 }));
			await fetchBrowsePage(query({ mediaType: 'comics', libraries: ['kids'] }), { limit: 50 });
			expect(spy, 'the guard is not wired into fetchBrowsePage at all').toHaveBeenCalledTimes(1);
			const message = String(spy.mock.calls[0][0]);
			expect(message).toContain(LIBRARY_BROWSE_URL);
			expect(message).toContain('media_type');
			expect(message).toContain('lib');
			expect(message).toContain('kids');
			// It must say WHY a 200 is the symptom, or the reader has no lead.
			expect(message).toContain('IGNORED');
		} finally {
			spy.mockRestore();
			vi.unstubAllGlobals();
		}
	});

	it('builds the endpoint the route table registers', () => {
		expect(LIBRARY_BROWSE_URL).toBe('/api/v1/library');
		expect(browseRequestUrl(query(), { limit: 50 }).startsWith(`${LIBRARY_BROWSE_URL}?`)).toBe(
			true
		);
	});
});

describe('an address this endpoint would refuse never becomes a request', () => {
	/*
	 * `media_type` is a closed enum server-side and an unrecognised value is a
	 * 400, never a silently unfiltered page — so the answer to `/library/films`
	 * is already known and asking would spend a round trip to be told it.
	 */
	it('refuses a segment outside the six, and admits all six', () => {
		const bad = browseRoute('films', new URLSearchParams());
		expect(bad.k, '/library/films resolved to a servable query').toBe('unknown-type');
		if (bad.k === 'unknown-type') expect(bad.value).toBe('films');
		expect(browseRoute('', new URLSearchParams()).k, 'an empty segment resolved to a query').toBe(
			'unknown-type'
		);
		for (const type of MEDIA_TYPES) {
			expect(browseRoute(type, new URLSearchParams()).k, `/library/${type} was refused`).toBe('ok');
		}
	});

	/*
	 * §7.3's bound is a REFUSAL and not a clamp, which is the opposite of
	 * `limit`'s rule: dropping slugs WIDENS the page, so a client that quietly
	 * sent the first 32 would show more than was asked for and look correct.
	 */
	it('refuses more than the endpoint accepts rather than truncating', () => {
		const slugs = Array.from({ length: MAX_LIBRARY_SLUGS + 1 }, (_, i) => `lib-${i}`);
		const over = browseRoute('movies', new URLSearchParams(`lib=${slugs.join(',')}`));
		expect(over.k, 'a 33-slug scope was sent rather than refused').toBe('too-many-libraries');
		if (over.k === 'too-many-libraries') expect(over.count).toBe(MAX_LIBRARY_SLUGS + 1);

		const at = browseRoute('movies', new URLSearchParams(`lib=${slugs.slice(0, -1).join(',')}`));
		expect(at.k, `a scope of exactly ${MAX_LIBRARY_SLUGS} was refused, and it is legal`).toBe('ok');
	});
});

describe('the cursor is dropped whenever the corpus changes', () => {
	/*
	 * ⚠️ NOTHING SERVER-SIDE CATCHES THIS. A cursor from this endpoint binds to
	 * `sort` ALONE. Replaying one under a different sort is a loud 400; replaying
	 * it under a different media type or a different library scope is a 200 whose
	 * page starts partway into a DIFFERENT corpus — rows skipped or repeated,
	 * nothing reported. The feed is therefore dropped whole, because keeping the
	 * rows would put one type's items under another type's heading.
	 */
	const loaded = (q: BrowseQuery): BrowseFeed => ({
		items: [],
		cursor: 'MXR2My5iZXJzZXJr',
		loaded: true,
		limit: 50,
		query: q
	});

	it('survives an identical query, so the reset is not just "always"', () => {
		const feed = loaded(query({ libraries: ['kids'] }));
		const kept = browseFeedFor(feed, query({ libraries: ['kids'] }));
		expect(kept, 'the feed was rebuilt for an unchanged query').toBe(feed);
		expect(kept.cursor, 'the cursor was dropped on an unchanged query').toBe('MXR2My5iZXJzZXJr');
	});

	it('is dropped when the media type changes', () => {
		const reset = browseFeedFor(loaded(query()), query({ mediaType: 'comics' }));
		expect(
			reset.cursor,
			'a movies cursor was replayed against comics. The server answers 200 and starts ' +
				'partway into a different corpus.'
		).toBeUndefined();
		expect(reset.loaded, 'the reset feed claims to have been read').toBe(false);
	});

	it('is dropped when the library scope changes', () => {
		for (const libraries of [['kids'], ['kids', 'vinyl-rips'], ['vinyl-rips']]) {
			const reset = browseFeedFor(loaded(query({ libraries: ['kids'] })), query({ libraries }));
			if (libraries.length === 1 && libraries[0] === 'kids') continue;
			expect(
				reset.cursor,
				`a cursor minted under lib=kids was replayed under lib=${libraries.join(',')}`
			).toBeUndefined();
		}
		const reset = browseFeedFor(loaded(query({ libraries: [] })), query({ libraries: ['kids'] }));
		expect(reset.cursor, 'adding a scope kept the unscoped cursor').toBeUndefined();
	});

	it('is dropped when the sort changes', () => {
		const reset = browseFeedFor(loaded(query()), query({ sort: 'popularity' }));
		expect(reset.cursor, 'an added_at cursor was replayed under popularity').toBeUndefined();
	});

	it('compares scopes by content and by order, not by reference', () => {
		expect(
			sameBrowseQuery(query({ libraries: ['a', 'b'] }), query({ libraries: ['a', 'b'] }))
		).toBe(true);
		expect(
			sameBrowseQuery(query({ libraries: ['a', 'b'] }), query({ libraries: ['b', 'a'] }))
		).toBe(false);
		expect(sameBrowseQuery(query({ libraries: ['a'] }), query({ libraries: ['a', 'b'] }))).toBe(
			false
		);
	});
});

describe('paging asks for the size the server said it applied', () => {
	/*
	 * 🚩 http-api.md §1.2, which governs both library endpoints: "a client pages
	 * against the `limit` in the RESPONSE, never against the one it sent". The
	 * server clamps at 200 SILENTLY, so a client that keeps sending its own
	 * number is asking for a page size the server never agreed to.
	 */
	it('carries the echoed limit into the next request', () => {
		const first = appendBrowsePage(emptyBrowseFeed(query()), page({ limit: 50, nextCursor: 'c1' }));
		expect(first.limit, 'the echoed page size was not kept').toBe(50);
		const next = nextBrowsePage(first, 200);
		expect(
			next?.limit,
			'the next page asks for the size this client wanted rather than the one the server ' +
				'applied. The server clamps silently, so the two can differ for ever.'
		).toBe(50);
		expect(next?.cursor).toBe('c1');
	});

	it('falls back to the caller size only before the server has answered', () => {
		expect(nextBrowsePage(emptyBrowseFeed(query()), 200)?.limit).toBe(200);
	});

	/*
	 * ⚠️ THE STOP RULE IS `next_cursor`'s ABSENCE AND NEVER A SHORT PAGE. Under
	 * `added_at` the server issues a second statement for the undated tail, so a
	 * page can legitimately come back shorter than the limit with rows still to
	 * come.
	 */
	it('stops on an absent cursor and not on a short page', () => {
		const short = appendBrowsePage(emptyBrowseFeed(query()), page({ limit: 50, nextCursor: 'c1' }));
		expect(nextBrowsePage(short, 200), 'a short page ended the walk').not.toBeUndefined();
		const last = appendBrowsePage(short, page({ limit: 50 }));
		expect(nextBrowsePage(last, 200), 'an absent next_cursor did not end the walk').toBeUndefined();
	});

	it('keeps the query across an appended page', () => {
		const q = query({ mediaType: 'comics', libraries: ['kids'] });
		expect(appendBrowsePage(emptyBrowseFeed(q), page()).query).toBe(q);
	});
});

describe('a rejected cursor is told apart from a rejected filter', () => {
	/*
	 * ⚠️ `400 bad_request` IS ONE CODE OVER SEVERAL CAUSES ON THIS ENDPOINT: a
	 * bad media_type, a bad sort, an unknown lib slug, a malformed limit AND a
	 * stale cursor (http-api.md §7.7). Deciding from the status and the code
	 * alone therefore tells a user whose FILTER is wrong that their bookmark has
	 * gone stale, and offers a restart that fails identically. A request that
	 * carried no cursor cannot have had one rejected.
	 */
	const badRequest = new ApiError('bad', 400, LIBRARY_BROWSE_URL, 'bad_request', 'fix it', 'bad');

	it('is not consulted for a request that carried no cursor', () => {
		expect(
			cursorRejected(badRequest, undefined),
			'a cursorless 400 was read as a stale bookmark. On this endpoint that is what a ' +
				'mistyped media_type, sort or lib slug looks like.'
		).toBe(false);
	});

	it('still fires for a request that did carry one', () => {
		expect(
			cursorRejected(badRequest, 'MXR2My5iZXJzZXJr'),
			'a rejected cursor is no longer recognised, so the restart control never appears'
		).toBe(true);
	});

	it('is not fooled by another status or another code', () => {
		expect(cursorRejected(new ApiError('x', 500, '/', 'internal'), 'c')).toBe(false);
		expect(cursorRejected(new ApiError('x', 400, '/', 'unauthorized'), 'c')).toBe(false);
		expect(cursorRejected(new Error('boom'), 'c')).toBe(false);
	});

	/*
	 * Both existing callers pass the cursor they actually sent. Home's Block C is
	 * the one that must not regress: its first page carries no cursor, so a 400
	 * there is not a stale bookmark either.
	 */
	it('is called with the sent cursor at every call site', () => {
		for (const [name, source] of [
			[ROUTE, ROUTE_SOURCE],
			['routes/library/+page.svelte', RECENT_ROUTE_SOURCE],
			['routes/+page.svelte', HOME_SOURCE]
		] as const) {
			const calls = [...source.matchAll(/cursorRejected\(([^)]*)\)/g)].map((m) => m[1]);
			expect(calls.length, `${name} no longer calls cursorRejected`).toBeGreaterThan(0);
			for (const args of calls) {
				expect(
					args,
					`${name} calls cursorRejected(${args}) without the cursor it sent, so a bad ` +
						'filter would be reported as a stale bookmark'
				).toContain('.cursor');
			}
		}
	});
});

describe('the empty state says which fact it is stating', () => {
	/*
	 * Two different facts wear the same empty table. "No comics" is about a
	 * filter; "no services connected" is about the install, and telling the
	 * second story in the first's words answers a question about the library with
	 * a fact about a filter. The three non-`library` modes are
	 * `$lib/libraryscreen`'s words verbatim so the two screens cannot drift.
	 */
	it('defers to the services read when nothing library-bearing is connected', () => {
		for (const mode of ['unconfigured', 'search-and-grab', undefined] as const) {
			const state = browseEmptyState(query({ mediaType: 'comics' }), mode);
			expect(
				state.title.toLowerCase(),
				`the ${String(mode)} empty state blames the comics filter for a missing service`
			).not.toContain('comics');
		}
	});

	it('names the type when a library-bearing service IS connected', () => {
		const state = browseEmptyState(query({ mediaType: 'comics' }), 'library');
		expect(
			state.title.toLowerCase(),
			'the connected-library empty state does not say which type is empty'
		).toContain('comics');
		expect(state.text.length).toBeGreaterThan(0);
	});

	it('says the scope is part of the answer when there is one', () => {
		const scoped = browseEmptyState(query({ libraries: ['kids'] }), 'library');
		const all = browseEmptyState(query(), 'library');
		expect(
			scoped.title,
			'a scoped empty view and an unscoped one tell the same story, and only one of them ' +
				'is about the catalogue'
		).not.toBe(all.title);
	});
});

describe('the per-type route renders the shared cells and no others', () => {
	it('draws the shared Have cell', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer imports the shared Have cell from $lib/HaveCell.svelte`
		).toContain("from '$lib/HaveCell.svelte'");
		expect(
			MARKUP,
			`${ROUTE} imports the shared Have cell but never renders <HaveCell />. An import ` +
				'with no element is a column drawn by something else.'
		).toContain('<HaveCell');
	});

	it('has no inline availability chain of its own', () => {
		const inlined = ['complete', 'none', 'fraction', 'partial'].filter((m) =>
			MARKUP.includes(`mark.k === '${m}'`)
		);
		expect(
			inlined,
			`${ROUTE} tests the availability mark inline (${inlined.join(', ')}). That is the ` +
				'copy-paste $lib/HaveCell.svelte exists to end.'
		).toEqual([]);
		expect(
			MARKUP,
			`${ROUTE} calls haveCell() in its own markup. The component owns that call.`
		).not.toContain('haveCell(');
	});

	/*
	 * ⚠️ THE MEDIA TYPE IS NEVER DERIVED FROM `kind`. §17.2 states the Tier 1
	 * client index carries no format, so a browser holding `kind: 'book'` cannot
	 * tell an ebook from an audiobook: both are `book` and what separates them is
	 * `edition.format`, which is not on this wire. Deriving the type here would
	 * silently collapse two of the six into one.
	 */
	it('never reads item.kind', () => {
		expect(
			ROUTE_CODE.length,
			'the stripped route is empty, so this ban is reading nothing'
		).toBeGreaterThan(1000);
		expect(
			ROUTE_CODE,
			`${ROUTE} reads a row's kind. The Ebooks/Audiobooks split is server-side; kind ` +
				'cannot tell them apart.'
		).not.toContain('.kind');
		expect(
			ROUTE_CODE,
			`${ROUTE} no longer renders a media-type label at all, so the ban above is asserting ` +
				'nothing'
		).toContain('mediaTypeLabel(');
	});

	it('declares no content-sized track', () => {
		const widths = [...COLUMNS.matchAll(/width:\s*'([^']+)'/g)].map((m) => m[1]);
		expect(
			widths.length,
			`no column widths were found in ${ROUTE}'s COLUMNS, so this check asserts nothing`
		).toBe(4);
		expect(
			findContentSizedTracks(widths),
			`${ROUTE} declares a content-sized track. Every row is its own grid (ADR-0029), so ` +
				'the header sizes to the column name and each body row to its own contents.'
		).toEqual([]);
	});

	it('tells ARIA the truth about its size', () => {
		const total = /total=\{([^}]*)\}/.exec(LIST_TAG);
		expect(total, `${ROUTE} no longer passes total to <List>`).not.toBeNull();
		expect(
			total?.[1],
			`${ROUTE} passes total={${total?.[1] ?? ''}} unconditionally. While a cursor is ` +
				'outstanding the total is unknown, and the honest value is `undefined`.'
		).toContain('undefined');
	});

	it('reads "is there more" off the cursor rather than off a short page', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer asks nextBrowsePage() for the next page. The stop rule and the ` +
				'echoed page size both live there.'
		).toContain('nextBrowsePage(');
		expect(
			ROUTE_SOURCE,
			`${ROUTE} compares a page length against the page size. A short page is legal under ` +
				'added_at: the server issues a second statement for the undated tail.'
		).not.toMatch(/length\s*[<>]=?\s*LOAD_MORE_PAGE_SIZE/);
	});

	/*
	 * The route must not build a query string for this endpoint by hand: the
	 * builder is the only thing that knows an empty `lib` is a 400 and that a
	 * misspelt parameter is ignored rather than refused.
	 */
	it('builds no query for the endpoint of its own', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} names the browse endpoint directly. $lib/librarygrid owns the query string.`
		).not.toContain('/api/v1/library?');
		expect(ROUTE_SOURCE, `${ROUTE} no longer fetches a page through $lib/librarygrid`).toContain(
			'fetchBrowsePage('
		);
	});
});

describe('all six media types are reachable from the shell', () => {
	/*
	 * A screen with no route into it is a screen that does not exist.
	 *
	 * ⚠️ ALL SIX SHIP, INCLUDING THE ONES AN INSTALL HAS NOTHING IN, and that is a
	 * recorded trade-off rather than an oversight: §17.2 and DESIGN-DIRECTION
	 * §8.1 want a type with no content hidden, and doing that honestly needs a
	 * per-type COUNT that http-api.md §7.1 states is not on the wire. Hiding a
	 * type on a count nobody measured would hide a library that is really there.
	 */
	it('has an entry and a title for every one of the six', () => {
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte no longer builds the media-type nav from MEDIA_TYPES, so a ' +
				'seventh type would arrive with no entry'
		).toContain('MEDIA_TYPES.map(');
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte no longer resolves the per-type route, so the six entries ' +
				'have no href'
		).toContain("resolve('/library/[type]', { type })");
		expect(
			LAYOUT_SOURCE,
			'the six type titles are no longer derived from the same list as the six entries, ' +
				'so a screen can have a link and an empty toolbar'
		).toContain('TYPE_NAV.map(');
	});

	/*
	 * ⚠️ THE LABEL IS `Library` AND IT USED TO BE `Recently added`. The old one
	 * was right for the old screen: `/library` read `/library/recent`, which is
	 * hard-ordered `added_at DESC, id DESC`. It now reads the browse endpoint and
	 * carries a sort control, so a view a user has put in `popularity` order is
	 * not a recently-added list, and the old label would contradict the screen.
	 *
	 * ⚠️ HOME'S BLOCK C KEEPS THE OLD WORDS, and that is not an inconsistency:
	 * Block C really is recently added, being closed at one order and no filters
	 * (§17.2, ADR-0028). `libraryscreen.test.ts` pins that the two screens still
	 * agree about the EMPTY state, which is the fact they genuinely share.
	 */
	it('labels the all-types view Library, not Recently added', () => {
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte dropped the /library entry. §17.2 keeps that view: it is the ' +
				'one table across ALL six types and is not superseded by the per-type screens.'
		).toContain("id: '/library', label: 'Library'");
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte calls the /library entry Recently added again. The screen ' +
				'sorts by popularity as well, so that label names a list the user may not be ' +
				'looking at.'
		).not.toContain("label: 'Recently added'");
	});

	it('links with a real href rather than a click handler', () => {
		const nav = LAYOUT_SOURCE.slice(LAYOUT_SOURCE.indexOf('class="nav__link"'));
		expect(
			nav.slice(0, 200),
			'a nav entry is no longer a real link, so it cannot be ' +
				'middle-clicked, Ctrl-clicked or copied'
		).toContain('href={item.href}');
	});
});

describe('the type navigation shows no count it cannot measure', () => {
	/*
	 * http-api.md §7.1: "There are no facet counts beside the chips; each is its
	 * own aggregate and its own read." Six one-row probes on every navigation is
	 * the render-path cost principle 1 refuses, and a count invented from the
	 * rows already loaded would be a count of the keyset prefix.
	 */
	it('puts no per-type number in the sidebar', () => {
		const nav = LAYOUT_SOURCE.slice(
			LAYOUT_SOURCE.indexOf('const TYPE_NAV'),
			LAYOUT_SOURCE.indexOf('const NAV_GROUPS')
		);
		expect(nav.length, 'TYPE_NAV was not found, so this guard is reading nothing').toBeGreaterThan(
			0
		);
		expect(nav, 'the media-type nav renders a count, and no endpoint serves one').not.toMatch(
			/count|itemCount/i
		);
		expect(nav, 'the media-type nav no longer renders a label').toContain('mediaTypeLabel(');
	});
});

describe('the media-type vocabulary is one list', () => {
	it('is $lib/library’s six, in its order', () => {
		const types: MediaType[] = [...MEDIA_TYPES];
		expect(types).toEqual(['movies', 'tv', 'music', 'ebooks', 'audiobooks', 'comics']);
	});
});

/* ── the scope line ─────────────────────────────────────────────────────────
 *
 * A scoped view used to print `Scoped to <slug>`, and ARCHITECTURE §17.8
 * deliberately renders a slug NOWHERE on the Libraries screen — so a user could
 * be scoped by a value the UI had never shown them. These guards hold the four
 * halves of the fix: the name is used when it resolves, the slug stands when it
 * does not, an unscoped view says nothing at all, and the libraries read is
 * never something the grid waits on.
 *
 * ⚠️ THE ABSENCE GUARD ASSERTS THE PRESENCE FIRST. "Nothing renders when
 * unscoped" passes just as well on a function that returns `undefined` for every
 * input, which would delete the line from the scoped view too.
 */

const RECENT_ROUTE = 'routes/library/+page.svelte';
const RECENT_ROUTE_CODE = stripComments(RECENT_ROUTE_SOURCE);
const RECENT_MARKUP = userFacingMarkup(RECENT_ROUTE_SOURCE);

/** Both screens share this toolbar block, so every rule below runs over both. */
const SCOPED_SCREENS: [string, string, string][] = [
	[ROUTE, ROUTE_CODE, MARKUP],
	[RECENT_ROUTE, RECENT_ROUTE_CODE, RECENT_MARKUP]
];

/** The `const NAME = …;` initialiser, up to the statement's end. */
function binding(source: string, where: string, name: string): string {
	const open = source.indexOf(`const ${name} =`);
	expect(open, `${where} no longer declares ${name}`).toBeGreaterThanOrEqual(0);
	const end = source.indexOf(';', open);
	expect(end, `${where}'s ${name} is not a statement any more`).toBeGreaterThan(open);
	return source.slice(open, end);
}

describe('a scoped view states its scope in words a user can read', () => {
	const EBOOKS = { slug: 'ebooks', name: 'Ebooks' };
	const AUDIO = { slug: 'audiobooks', name: 'Audiobooks' };

	it('names the library when GET /api/v1/libraries resolved the slug', () => {
		const names = libraryNames([EBOOKS, AUDIO]);
		expect(names.size, 'the map is empty, so this guard proves nothing').toBe(2);
		expect(libraryScopeLine(['ebooks'], names)).toBe('Scoped to Ebooks');
	});

	it('falls back to the slug when the read has not answered', () => {
		// `undefined` is the value the screen holds from its first frame until the
		// read lands, and for ever if it never does.
		const line = libraryScopeLine(['ebooks']);
		expect(line, 'a scoped view rendered nothing, which is the one outcome forbidden').toBeTruthy();
		expect(line).toBe('Scoped to ebooks');
	});

	it('falls back to the slug when the read answered without that library', () => {
		const line = libraryScopeLine(['kids'], libraryNames([EBOOKS]));
		expect(line).toBe('Scoped to kids');
	});

	it('names every library in a multi-slug scope, in the address’s order', () => {
		const names = libraryNames([EBOOKS, AUDIO]);
		expect(libraryScopeLine(['audiobooks', 'ebooks'], names)).toBe('Scoped to Audiobooks, Ebooks');
	});

	it('resolves per slug, so one known name sits beside one unknown slug', () => {
		const line = libraryScopeLine(['ebooks', 'kids'], libraryNames([EBOOKS]));
		expect(line).toBe('Scoped to Ebooks, kids');
	});

	it('never stores a blank name, because a blank label states no scope at all', () => {
		const names = libraryNames([{ slug: 'ebooks', name: '   ' }]);
		expect(names.has('ebooks'), 'a whitespace name was stored and would render blank').toBe(false);
		expect(libraryScopeLine(['ebooks'], names)).toBe('Scoped to ebooks');
	});

	it('says nothing on an unscoped view, and the scoped case proves it looked', () => {
		expect(libraryScopeLine(['ebooks']), 'the scoped case is empty too').toBeTruthy();
		expect(libraryScopeLine([])).toBeUndefined();
		expect(libraryScopeLine([], libraryNames([EBOOKS]))).toBeUndefined();
	});
});

describe('the libraries read is an enrichment, never a precondition', () => {
	it('resolves to no names when the read fails, rather than rejecting', async () => {
		vi.stubGlobal('fetch', () => Promise.reject(new TypeError('connection refused')));
		try {
			const names = await readLibraryNames();
			expect(names.size, 'a failed read produced names, so the stub did not bite').toBe(0);
			// The line the screen still renders over that failure.
			expect(libraryScopeLine(['ebooks'], names)).toBe('Scoped to ebooks');
		} finally {
			vi.unstubAllGlobals();
		}
	});

	it('resolves to no names on a 500, rather than rejecting', async () => {
		stubFetch(() => jsonResponse({ detail: 'nope' }, 500));
		try {
			await expect(readLibraryNames()).resolves.toHaveProperty('size', 0);
		} finally {
			vi.unstubAllGlobals();
		}
	});

	it('reads the names it is given, so the two guards above are not vacuous', async () => {
		stubFetch(() =>
			jsonResponse({
				items: [
					{
						id: 2,
						name: 'Ebooks',
						slug: 'ebooks',
						kind: 'book',
						sort_order: 5,
						enabled: true,
						include_in_search: false,
						item_count: 3,
						sources: []
					}
				]
			})
		);
		try {
			const names = await readLibraryNames();
			expect(names.get('ebooks')).toBe('Ebooks');
		} finally {
			vi.unstubAllGlobals();
		}
	});

	it.each(SCOPED_SCREENS)('%s does not gate its table on the names', (where, code) => {
		// `drawList` is the one gate on whether the catalogue is in the DOM at all.
		const gate = binding(code, where, 'drawList');
		expect(gate, `${where}'s drawList reads nothing`).toContain('feed');
		expect(gate, `${where} gates its table on the libraries read`).not.toMatch(/\bnames\b/);
		const page = code.slice(code.indexOf('async function loadPage'));
		expect(page.length, `${where} no longer has a loadPage`).toBeGreaterThan(0);
		expect(
			page.slice(0, page.indexOf('\n\t}')),
			`${where} reads the library names on the paging path`
		).not.toMatch(/\bnames\b/);
	});

	it.each(SCOPED_SCREENS)(
		'%s renders the resolved line, never a raw slug join',
		(where, code, markup) => {
			expect(markup, `${where} no longer renders a scope line`).toContain('{scopeLine}');
			expect(markup, `${where} still prints the slug list verbatim`).not.toContain(
				'query.libraries.join'
			);
			// Nothing renders unscoped: the whole block is behind the scope test.
			const label = markup.indexOf('{scopeLine}');
			const guard = markup.lastIndexOf('{#if query.libraries.length > 0}', label);
			expect(guard, `${where}'s scope line is not behind the scoped test`).toBeGreaterThanOrEqual(
				0
			);
			expect(code, `${where} asks for names on an unscoped view`).toContain(
				'query?.libraries.length ?? 0) === 0'
			);
		}
	);
});
