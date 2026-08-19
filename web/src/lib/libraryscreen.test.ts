/*
 * THE `/library` SCREEN, GUARDED.
 *
 * WHY IT READS SOURCE. `vitest.config.ts` is `environment: 'node'` with no
 * Svelte plugin, so a component cannot be imported and compiled in a test at
 * all. The repo's answer is already in the tree twice — `home.test.ts` reads a
 * template through Vite's `?raw` and runs a ban list over it, and
 * `havecell.test.ts` does the same for the Have column — so the same idiom is
 * used here for the rules that live in markup, and `recentEmptyState` is
 * imported and called directly for the one rule that does not.
 *
 * ⚠️ EVERY SLICE BELOW ASSERTS THAT IT FOUND SOMETHING BEFORE IT ASSERTS
 * ANYTHING ABOUT IT. The failure mode of a text guard is matching nothing at
 * all: an empty string passes every `not.toContain` there is, so a renamed
 * constant would turn each of these green rather than red.
 */

import { describe, expect, it } from 'vitest';
import { userFacingMarkup } from './copyguard';
import { findContentSizedTracks } from './list';
import { recentEmptyState } from './libraryscreen';
import ROUTE_SOURCE from '../routes/library/+page.svelte?raw';
import HOME_SOURCE from '../routes/+page.svelte?raw';
import LAYOUT_SOURCE from '../routes/+layout.svelte?raw';

const ROUTE = 'routes/library/+page.svelte';
const MARKUP = userFacingMarkup(ROUTE_SOURCE);

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

describe('recentEmptyState', () => {
	/*
	 * An empty catalogue means three different things depending on what is
	 * connected, and a fourth when UsArr could not find out. The screen renders
	 * ONE of these into `<List emptyTitle>`, so a branch that returned the wrong
	 * one would tell a user with no service at all that an import is on its way,
	 * which is §17.7's own failure mode written as copy.
	 */
	it('gives each mode its own words', () => {
		const titles = [
			recentEmptyState('library').title,
			recentEmptyState('search-and-grab').title,
			recentEmptyState('unconfigured').title,
			recentEmptyState(undefined).title
		];
		expect(
			new Set(titles).size,
			`two modes share an empty-state title: ${titles.join(' / ')}`
		).toBe(4);
		for (const state of [
			recentEmptyState('library'),
			recentEmptyState('search-and-grab'),
			recentEmptyState('unconfigured'),
			recentEmptyState(undefined)
		]) {
			expect(state.title.length, 'an empty state has no title').toBeGreaterThan(0);
			expect(state.text.length, `${state.title} has no explanation under it`).toBeGreaterThan(0);
		}
	});

	/*
	 * The unknown case is NOT the "nothing catalogued yet" case, and the
	 * distinction is the whole reason it exists: a failed services read means
	 * UsArr does not know why the list is empty, and saying "an import has not
	 * run" on the strength of a read that did not answer is a claim about the
	 * library made out of a claim about the network.
	 */
	it('does not assert an import is pending when the services read failed', () => {
		const unknown = recentEmptyState(undefined);
		expect(unknown.title, 'the unknown state borrowed the connected-library title').not.toBe(
			recentEmptyState('library').title
		);
		expect(
			unknown.text,
			'the unknown state claims a library-bearing service is connected, which is exactly ' +
				'the fact the failed read did not establish'
		).not.toContain('is connected and');
	});

	/*
	 * ⚠️ THE DRIFT GUARD, AND IT IS THE POINT OF THIS FILE. Home's Block C and
	 * this screen draw the same table over the same endpoint. An install with a
	 * connected library and no rows yet must not be told two different stories
	 * depending on which screen it is read from, and nothing else in the tree
	 * would notice if it were: the two strings are four files apart.
	 */
	it('is word for word Home Block C, for the mode Home draws', () => {
		const title = /emptyTitle="([^"]+)"/.exec(HOME_SOURCE);
		const text = /emptyText="([^"]+)"/.exec(HOME_SOURCE);
		expect(
			title,
			'routes/+page.svelte no longer passes a literal emptyTitle to Block C, so this ' +
				'guard has nothing to compare against and must be rewritten rather than deleted'
		).not.toBeNull();
		expect(
			text,
			'routes/+page.svelte no longer passes a literal emptyText to Block C'
		).not.toBeNull();

		const state = recentEmptyState('library');
		expect(
			state.title,
			"Home's Block C and /library disagree about what an empty catalogue is called"
		).toBe(title?.[1]);
		expect(
			state.text,
			"Home's Block C and /library explain an empty catalogue differently. One of the " +
				'two was reworded; reconcile them rather than relaxing this test.'
		).toBe(text?.[1]);
	});
});

describe('the /library route draws the shared Have cell', () => {
	it('imports it and renders it', () => {
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
				'copy-paste $lib/HaveCell.svelte exists to end: two screens had already come ' +
				'apart on it once. Pass the row to <HaveCell /> and let it decide.'
		).toEqual([]);
		expect(
			MARKUP,
			`${ROUTE} calls haveCell() in its own markup. The component owns that call.`
		).not.toContain('haveCell(');
	});
});

describe('the /library table offers no filter and no sort', () => {
	/*
	 * NOT A PREFERENCE, AND NOT A GAP TO BE FILLED BY THE NEXT PASS. What this
	 * screen holds is a keyset PREFIX of the catalogue — the newest N rows, for
	 * whatever N `Load more` has been pressed up to — because the endpoint is
	 * keyset-paginated and hard-ordered `added_at DESC, id DESC`. A control that
	 * filtered or sorted the rows in the DOM would present itself as operating on
	 * the library and would in fact operate on the prefix, so "no comics found"
	 * would mean "no comics in the newest 200 rows". A confidently wrong answer
	 * is worse than no control, so the controls wait for the browse read that can
	 * apply them server-side.
	 */
	it('declares no sortable column', () => {
		expect(
			COLUMNS,
			`${ROUTE} declares a sortable column. A header sort would sort the keyset prefix ` +
				'in the DOM and claim to have sorted the library.'
		).not.toContain('sortable');
	});

	it('wires no sort state into the list', () => {
		for (const prop of ['sortKey', 'sortDir', 'onsort']) {
			expect(
				LIST_TAG,
				`${ROUTE} passes ${prop} to <List>. The primitive draws a sort affordance from ` +
					'it, and there is nothing behind that affordance but a prefix.'
			).not.toContain(prop);
		}
	});

	it('ships no filter control and no filtered-empty state', () => {
		for (const control of ['<input', '<select', 'filteredEmpty']) {
			expect(
				MARKUP,
				`${ROUTE} renders ${control}. A filter over a keyset prefix answers a question ` +
					'about the newest N rows in the words of a question about the library.'
			).not.toContain(control);
		}
	});
});

describe('the /library table tells ARIA the truth about its size', () => {
	/*
	 * ARIA defines `aria-rowcount="-1"` for a total that is genuinely unknown,
	 * and it is unknown here by construction: a keyset endpoint never says how
	 * many rows exist. Passing the rendered count while a cursor is outstanding
	 * is what makes a screen reader say "row 3 of 200" when the truth is "row 3
	 * of 4,000", and it is what would put "200 of 200" under a Load-more button
	 * with thousands of rows left to fetch. `List.svelte` says so at the prop.
	 */
	it('passes total only once the feed is exhausted', () => {
		const total = /total=\{([^}]*)\}/.exec(LIST_TAG);
		expect(total, `${ROUTE} no longer passes total to <List>`).not.toBeNull();
		expect(
			total?.[1],
			`${ROUTE} passes total={${total?.[1] ?? ''}} unconditionally. While a cursor is ` +
				'outstanding the total is unknown, and the honest value is `undefined`.'
		).toContain('undefined');
	});
});

describe('the /library table pages to the end of the catalogue', () => {
	it('reads "is there more" off the cursor rather than off a short page', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer asks nextRequest() for the next page. The stop rule lives ` +
				'there and nowhere else: the server issues a second statement for the undated ' +
				'tail, so a short page is legal and is not the end of the list.'
		).toContain('nextRequest(');
		expect(
			ROUTE_SOURCE,
			`${ROUTE} compares a page length against the page size. That is the stop rule ` +
				'$lib/library exists to keep out of a template: it truncates the table at the ' +
				'first row whose upstream reported no creation date, silently and for ever.'
		).not.toMatch(/length\s*[<>]=?\s*LOAD_MORE_PAGE_SIZE/);
	});
});

describe('the /library table can line its columns up', () => {
	/*
	 * ADR-0029 makes every row an independent grid, so a content-sized track
	 * resolves against its OWN row's contents and the header cannot agree with
	 * the body. It shipped once and nothing failed: it rendered, it just
	 * misaligned. `findContentSizedTracks` is the running app's own validator,
	 * called here on the widths this route actually declares.
	 */
	it('declares no content-sized track', () => {
		const widths = [...COLUMNS.matchAll(/width:\s*'([^']+)'/g)].map((m) => m[1]);
		expect(
			widths.length,
			`no column widths were found in ${ROUTE}'s COLUMNS, so this check is asserting ` +
				'nothing at all'
		).toBe(5);
		expect(
			findContentSizedTracks(widths),
			`${ROUTE} declares a content-sized track. Every row is its own grid, so the header ` +
				'sizes to the column name and each body row sizes to its own contents.'
		).toEqual([]);
	});
});

describe('the /library screen is reachable', () => {
	/*
	 * A screen with no route into it is a screen that does not exist. THIS entry
	 * is hand-written rather than derived — unlike the six media-type rows, which
	 * `TYPE_NAV` builds from `MEDIA_TYPES` and titles from the same list — so its
	 * entry and its title are two facts that must agree; a row with no title
	 * renders an empty toolbar and an empty h1, which is the shape of the bug
	 * this pair catches.
	 */
	it('has a sidebar entry and a title', () => {
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte has no /library entry in NAV_GROUPS, so nothing in the ' +
				'application links to the screen'
		).toContain("id: '/library'");
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte has no /library title, so the toolbar and the h1 render empty'
		).toContain("[resolve('/library')");
	});
});
