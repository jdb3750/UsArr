/*
 * BLOCK C'S POSTERS VIEW, GUARDED — THE DATA HALF AND THE MARKUP HALF.
 *
 * WHAT CAN GO WRONG HERE, in the order it would go wrong silently:
 *
 *   1. The tile stops drawing `posterUrl`'s output. An `<img>` built out of
 *      `item.posterKey` by hand — or out of an empty string when the key is
 *      absent — renders a broken tile on every row of an install that has no
 *      artwork, and re-requests the current document while it is at it.
 *   2. The width stops being an allowlisted one. `GET /img/{key}` refuses an
 *      arbitrary `?w=` as a cache-poisoning DoS (ARCHITECTURE §4.4), so an
 *      invented width is a 400 and a broken image rather than a resized one.
 *   3. The toggle stops switching. Two panels drawn at once is Block C as two
 *      regions, which is exactly what §17.2 and ADR-0028 rule out; neither
 *      drawn at all is a heading over nothing.
 *
 * WHY IT IS SPLIT IN TWO. `vitest.config.ts` is `environment: 'node'` with no
 * Svelte plugin, so a component cannot be compiled, imported or rendered here
 * at all. Defect 1 and defect 2 are therefore pushed into `posterTile`, a plain
 * function this file calls FOR REAL over a fixture; defect 3 lives in an
 * `{#if}` and can only be read as text, which is `services-screen.test.ts`'s
 * idiom — one named template through Vite's `?raw`, sliced to the region that
 * renders.
 *
 * ⚠️ THE FIXTURE IS DELIBERATELY NOT EMPTY, and that is not a detail. The
 * failure mode of a guard over a collection is the empty collection: a grid
 * with no items renders no `<img>`, so "every tile draws posterUrl's output"
 * is vacuously true over nothing and a guard that can only pass is no guard.
 * `ITEMS` carries a work WITH artwork and a work WITHOUT it, the count is
 * asserted before anything is asserted about the tiles, and both arms are
 * checked — the second is the one an install with no covers actually sees.
 *
 * ⚠️ AND THE MARKUP SLICE THROWS WHEN IT MATCHES NOTHING, for the same reason.
 * `sectionsMarkup` fails rather than returning an empty corpus, because an
 * empty string satisfies every `not.toContain` there is and a renamed section
 * id would otherwise turn this file green instead of red.
 */

import { describe, expect, it } from 'vitest';
import { IMAGE_WIDTHS, POSTER_GRID_WIDTH, posterTile, posterUrl, type RecentItem } from './library';
import { HOME_VIEWS, HOME_VIEW_DEFAULT, HOME_VIEW_KEY, parseHomeView } from './home';
import { sectionsMarkup, userFacingMarkup } from './copyguard';
// Home AS TEXT. The `{#if}` that swaps the two panels is markup, and markup is
// the one thing this suite cannot execute.
import HOME_SOURCE from '../routes/+page.svelte?raw';

const HOME = 'routes/+page.svelte';

/** Block C, sliced to its own `</section>`. Throws when the id moves. */
const BLOCK_C = sectionsMarkup(userFacingMarkup(HOME_SOURCE), 'id="home-recent"').join('\n');

function item(over: Partial<RecentItem> = {}): RecentItem {
	return {
		id: 1,
		mediaType: 'comics',
		kind: 'comic',
		title: 'Berserk',
		haveCount: 3,
		wantCount: 3,
		...over
	};
}

/** A work with artwork, and a work without. Both states are ordinary. */
const ITEMS: RecentItem[] = [
	item({ id: 1, title: 'Berserk', year: 1989, posterKey: '0123456789abcdef' }),
	item({ id: 2, title: 'Vinland Saga', year: 2005 })
];

describe('posterTile', () => {
	it('has something to be wrong about', () => {
		// The floor under every assertion below: a fixture that lost its rows
		// would satisfy all of them without drawing anything.
		expect(ITEMS.length, 'the poster fixture is empty, so nothing below is a check').toBe(2);
		expect(
			ITEMS.filter((i) => i.posterKey !== undefined).length,
			'no fixture work has artwork, so the src assertions never see a URL'
		).toBe(1);
	});

	it('draws its src from posterUrl and from nothing else', () => {
		const tiles = ITEMS.map(posterTile);
		expect(tiles[0]?.src, 'the tile with artwork drew no image at all').toBe(
			posterUrl(ITEMS[0]!, POSTER_GRID_WIDTH)
		);
		expect(tiles[0]?.src, 'the tile stopped addressing the image route').toContain('/img/');
		expect(tiles[0]?.src).toContain(ITEMS[0]!.posterKey!);
	});

	/*
	 * `posterUrl` returns `undefined` for an absent key deliberately — "the
	 * absent case is the caller's to draw" — and a caller that turns that into
	 * `''` gets `<img src="">`, which re-requests the current document.
	 */
	it('leaves src absent for a work with no artwork, never empty', () => {
		const tile = posterTile(ITEMS[1]!);
		expect(tile.src, 'a work with no artwork was given an image to draw').toBeUndefined();
		expect(tile.src, 'the absent case became an empty string').not.toBe('');
	});

	it('asks for a width the server will serve', () => {
		expect(
			IMAGE_WIDTHS.includes(POSTER_GRID_WIDTH),
			`the grid asks for ?w=${POSTER_GRID_WIDTH}, which is not on §4.4's allowlist; the ` +
				'server answers 400 and the tile is broken rather than resized'
		).toBe(true);
		expect(posterTile(ITEMS[0]!).src).toContain(`?w=${POSTER_GRID_WIDTH}`);
	});

	/*
	 * DESIGN-DIRECTION §9.2 puts the full title in a native `title` on both the
	 * art and the title line, because the title renders as one ellipsised line.
	 * An empty title gets no tooltip: `title=""` is a tooltip promising nothing.
	 */
	it('carries the full title as a tooltip, and none where there is no title', () => {
		expect(posterTile(ITEMS[0]!).tooltip).toBe('Berserk');
		expect(posterTile(item({ title: '' })).tooltip).toBeUndefined();
	});
});

describe('the Home view preference', () => {
	/*
	 * Two modes ship and only two: DESIGN-DIRECTION §9.1 names table and
	 * posters, and defers "overview" — the wide row with a thumbnail, which is
	 * the shape that would put a cover in a list row.
	 */
	it('ships table and posters, and no third mode', () => {
		expect([...HOME_VIEWS]).toEqual(['table', 'posters']);
		expect(HOME_VIEW_DEFAULT).toBe('table');
	});

	it('falls back rather than throwing on anything it did not write', () => {
		expect(parseHomeView('posters')).toBe('posters');
		expect(parseHomeView('table')).toBe('table');
		expect(parseHomeView('overview')).toBe(HOME_VIEW_DEFAULT);
		expect(parseHomeView(null)).toBe(HOME_VIEW_DEFAULT);
		expect(parseHomeView('')).toBe(HOME_VIEW_DEFAULT);
	});

	/* Storage keys are contract: renaming one silently resets every browser
	 * that has already stored a choice. */
	it('keeps its storage key', () => {
		expect(HOME_VIEW_KEY).toBe('usarr.home.view');
	});
});

describe("Block C's posters panel", () => {
	it('has a corpus', () => {
		expect(BLOCK_C.length, `${HOME}'s Block C sliced to nothing`).toBeGreaterThan(0);
	});

	/*
	 * The whole point of the view: an <img> in the grid, whose src is the tile
	 * built by `posterTile`. A hand-rolled `/img/` string here would pass a test
	 * that only looked for `<img`, so both halves are asserted.
	 */
	it('renders an image whose src comes from posterTile', () => {
		expect(BLOCK_C, `${HOME}'s Block C draws no <img> at all`).toContain('<img');
		expect(
			BLOCK_C,
			`${HOME} builds a poster src by hand. posterUrl owns the width allowlist and the ` +
				'key check, and a second copy of either is a second thing to get wrong'
		).toContain('src={tile.src}');
		expect(BLOCK_C, `${HOME} no longer builds its tiles with posterTile`).toContain(
			'posterTile(item)'
		);
		expect(
			BLOCK_C,
			`${HOME} spells an /img path in its own markup rather than taking posterUrl's`
		).not.toContain('/img/');
	});

	/*
	 * §17.2 as amended by ADR-0028: Block C is ONE region. The two views are the
	 * two arms of one `{#if}`, so exactly one is in the DOM — a grid drawn
	 * beside the table is the second region the section exists to avoid.
	 */
	it('swaps the table for the grid rather than drawing both', () => {
		expect(
			BLOCK_C,
			`${HOME} no longer gates Block C's panels on the view preference, so either both ` +
				'are drawn at once or neither is'
		).toContain("{#if homeView.current === 'table'}");
		const table = BLOCK_C.indexOf('<List');
		const grid = BLOCK_C.indexOf('class="postergrid"');
		expect(table, `${HOME} draws no table arm`).toBeGreaterThanOrEqual(0);
		expect(grid, `${HOME} draws no posters arm`).toBeGreaterThanOrEqual(0);
		expect(
			BLOCK_C.slice(table, grid),
			'the table and the grid are no longer alternatives of one {#if}'
		).toContain('{:else');
	});

	/*
	 * The toggle is app.css's `.segment`, which shipped with its
	 * `[aria-pressed='true']` state already styled and no consumer. Native
	 * buttons in a named group: `aria-pressed` is what a toggle's state means,
	 * and the group needs a name because "table / posters" says nothing on its
	 * own.
	 */
	it('offers a native, named, pressed-state toggle', () => {
		expect(BLOCK_C, `${HOME} dropped the .segment button group`).toContain('class="segment"');
		expect(BLOCK_C, 'the button group lost its role').toContain('role="group"');
		expect(BLOCK_C, 'the button group lost its accessible name').toContain(
			'aria-label="View mode"'
		);
		for (const view of HOME_VIEWS) {
			expect(
				BLOCK_C,
				`the toggle has no button that reports whether ${view} is the current view`
			).toContain(`aria-pressed={homeView.current === '${view}'}`);
		}
		expect(
			BLOCK_C,
			'the toggle is no longer made of real buttons, so it has lost Tab, Space and Enter'
		).toContain('<button');
	});

	/*
	 * §4.4.1 rule 3 and §17.1: the empty tile is a `dominant_color` fill, never
	 * a grey box and never a shimmer, and `aspect-ratio` reserves the box so a
	 * decoded image shifts nothing. DESIGN-DIRECTION §13 adds "it never pulses".
	 */
	it('reserves the tile and never animates it', () => {
		const style = /<style>([\s\S]*)<\/style>/.exec(HOME_SOURCE)?.[1] ?? '';
		expect(style.length, `${HOME} has no <style> block to read`).toBeGreaterThan(0);
		const art = style.slice(style.indexOf('.postercard__art'));
		expect(art.length, `${HOME} no longer styles .postercard__art`).toBeGreaterThan(0);
		expect(art, 'the tile reserves no box, so a decoded image shifts the grid').toContain(
			'aspect-ratio'
		);
		expect(art, 'the tile stopped falling back to a colour fill').toContain('var(--dc,');
		for (const banned of ['animation', 'transition', '@keyframes']) {
			expect(
				style.slice(style.indexOf('.postergrid')),
				`${HOME}'s poster grid carries ${banned}. §17.1 bans animation on any list, grid ` +
					'or navigation transition, and a placeholder that pulses is a shimmer'
			).not.toContain(banned);
		}
	});

	/*
	 * DESIGN-DIRECTION §9.2, the rule that replaced the contrast solver: the
	 * title sits BELOW the tile, on the chrome's own ground. Inside the tile it
	 * would need a runtime WCAG solve against a single averaged colour, which
	 * is the subsystem that section deleted rather than improved.
	 */
	it('sets the title below the tile and not over the art', () => {
		const art = BLOCK_C.indexOf('postercard__art');
		const title = BLOCK_C.indexOf('postercard__title');
		expect(art, 'the card no longer draws a tile').toBeGreaterThanOrEqual(0);
		expect(title, 'the card no longer draws a title line').toBeGreaterThanOrEqual(0);
		expect(
			title,
			'the title moved above or inside the tile. §9.2 puts it below, on the chrome, ' +
				'because no contrast solve over an averaged colour can make text on cover art safe'
		).toBeGreaterThan(art);
		expect(BLOCK_C, 'the title line is no longer ellipsised to one line').toContain(
			'postercard__title trunc'
		);
	});
});
