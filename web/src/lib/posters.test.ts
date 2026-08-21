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
 *
 * ⚠️ THE CARD'S MARKUP MOVED OUT OF HOME AND SO DID THE ASSERTIONS OVER IT, AND
 * THAT SPLIT IS THE POINT RATHER THAN AN INCONVENIENCE. `$lib/PosterGrid.svelte`
 * now holds the grid, the card and their CSS, because the catalogue screens are
 * the second and third consumers the old route-scoped CSS comment said were the
 * condition for lifting it. So there are two corpora here and each rule is
 * asserted over the file that can actually violate it: what the CARD draws is
 * read off the component, and what BLOCK C does — swapping one panel for the
 * other, off Home's own preference — is read off Home, which is the only file
 * that still spells it. Leaving the card rules pointed at Home would have been
 * the weakening, not the split: `BLOCK_C` no longer contains `<img` at all, so
 * every one of them would have gone green over a corpus that cannot fail them.
 */

import { describe, expect, it } from 'vitest';
import {
	IMAGE_WIDTHS,
	POSTER_GRID_WIDTH,
	posterArtSrc,
	posterTile,
	posterUrl,
	type RecentItem
} from './library';
import { HOME_VIEWS, HOME_VIEW_DEFAULT, HOME_VIEW_KEY, parseHomeView } from './home';
import { MEDIA_TYPES } from './library';
import {
	libraryViewKey,
	LIBRARY_VIEWS,
	LIBRARY_VIEW_DEFAULT,
	parseLibraryView
} from './librarygrid';
import { sectionsMarkup, userFacingMarkup } from './copyguard';
// Home AS TEXT. The `{#if}` that swaps the two panels is markup, and markup is
// the one thing this suite cannot execute.
import HOME_SOURCE from '../routes/+page.svelte?raw';
// The card AS TEXT, for the same reason and off the file that now draws it.
import GRID_SOURCE from './PosterGrid.svelte?raw';

const HOME = 'routes/+page.svelte';
const GRID = 'lib/PosterGrid.svelte';

/** The component's markup, with its script, style and comments stripped. */
const GRID_MARKUP = userFacingMarkup(GRID_SOURCE);

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

describe('posterArtSrc — the tile a broken image falls back to', () => {
	/*
	 * ⚠️ THE FIXTURE FOR THIS ONE IS THE WORK THAT HAS A KEY, and it has to be:
	 * the whole defect is a work whose key is present and whose bytes are not, so
	 * running it over the keyless work would assert nothing — that tile is already
	 * empty for the other reason. Asserted before the rest, so a fixture that lost
	 * its key cannot make the rest of this block vacuous.
	 */
	it('has something to be wrong about', () => {
		expect(
			posterTile(ITEMS[0]!).src,
			'the fixture work carries no artwork, so nothing below can fail to load'
		).toBeDefined();
	});

	it('draws the art while nothing has failed', () => {
		const tile = posterTile(ITEMS[0]!);
		expect(posterArtSrc(tile, new Set())).toBe(tile.src);
	});

	/*
	 * The defect: `GET /img/{key}` is a cache read and answers `404 not_cached`
	 * for a key whose bytes have not been rendered yet, which is ordinary rather
	 * than a fault. Without this the browser draws its own broken-image glyph.
	 */
	it('falls back to the empty tile once the image has failed', () => {
		const tile = posterTile(ITEMS[0]!);
		expect(
			posterArtSrc(tile, new Set([tile.id])),
			'a cover that 404d is still being handed to the <img>, so the browser draws its ' +
				'own broken-image glyph rather than the tile the absent-key case draws'
		).toBeUndefined();
	});

	/*
	 * One failure is one card. A set keyed by anything coarser would blank the
	 * whole grid the first time any single cover was missing.
	 *
	 * The second work takes the FIXTURE'S key rather than a fresh literal: what
	 * distinguishes the two cards here is the work id, which is what the set holds,
	 * and a second sixteen-hex string in a test file is a secret scanner's finding
	 * waiting to happen.
	 */
	it('blanks only the work that failed', () => {
		const failed = new Set([ITEMS[0]!.id]);
		const other = posterTile(item({ id: 99, posterKey: ITEMS[0]!.posterKey! }));
		expect(posterArtSrc(other, failed)).toBe(other.src);
	});

	/* The absent-key case is unchanged and still absent: a failure set that does
	 * not name it must not turn `undefined` into anything else. */
	it('leaves a work with no artwork absent either way', () => {
		const tile = posterTile(ITEMS[1]!);
		expect(posterArtSrc(tile, new Set())).toBeUndefined();
		expect(posterArtSrc(tile, new Set([tile.id]))).toBeUndefined();
	});
});

describe("the catalogue screens' view preference", () => {
	/* The same two modes as Home, and for the same reason: DESIGN-DIRECTION §9.1
	 * names table and posters and defers "overview". A third value here would be
	 * a view one screen has and the other does not. */
	it('ships table and posters, and no third mode', () => {
		expect([...LIBRARY_VIEWS]).toEqual(['table', 'posters']);
	});

	/*
	 * ⚠️ TABLE IS THE DEFAULT AND IT IS A RULE. DESIGN-DIRECTION §5.4: rows and
	 * tables are the default container, a card is the exception. Flipping this
	 * would be a design change, not a preference change.
	 */
	it('opens as a table, never as a grid of art', () => {
		expect(LIBRARY_VIEW_DEFAULT).toBe('table');
		expect(parseLibraryView(null)).toBe('table');
	});

	it('falls back rather than throwing on anything it did not write', () => {
		expect(parseLibraryView('posters')).toBe('posters');
		expect(parseLibraryView('table')).toBe('table');
		expect(parseLibraryView('overview')).toBe(LIBRARY_VIEW_DEFAULT);
		expect(parseLibraryView('')).toBe(LIBRARY_VIEW_DEFAULT);
	});

	/*
	 * ⚠️ ONE KEY PER MEDIA TYPE, which is the sentence Home could not satisfy —
	 * §9.1 persists the toggle *"per media type"* and Home is the all-types view.
	 * Six distinct keys plus the all-types one, and none of them is Home's.
	 */
	it("keys the choice on the media type, and never on Home's key", () => {
		const keys = MEDIA_TYPES.map(libraryViewKey);
		expect(new Set(keys).size, 'two media types share one stored view').toBe(MEDIA_TYPES.length);
		expect(libraryViewKey('movies')).toBe('usarr.library.view.movies');
		expect(
			keys.includes(HOME_VIEW_KEY),
			"a catalogue screen writes Home's stored view, so choosing posters on one changes the " +
				'other'
		).toBe(false);
	});

	/* The all-types screen has no media type and still needs somewhere to store a
	 * choice. `all` is not a MediaType, so it cannot collide with one. */
	it('gives the all-types screen its own key, distinct from every type and from Home', () => {
		const all = libraryViewKey(undefined);
		expect(all).toBe('usarr.library.view.all');
		expect(all).not.toBe(HOME_VIEW_KEY);
		expect(MEDIA_TYPES.map(libraryViewKey).includes(all)).toBe(false);
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
		const grid = BLOCK_C.indexOf('<PosterGrid');
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
});

describe('the poster card', () => {
	it('has a corpus', () => {
		expect(GRID_MARKUP.length, `${GRID} stripped to nothing`).toBeGreaterThan(0);
	});

	/*
	 * The whole point of the view: an <img> in the grid, whose src is the tile
	 * built by `posterTile`. A hand-rolled `/img/` string here would pass a test
	 * that only looked for `<img`, so both halves are asserted.
	 */
	it('renders an image whose src comes from posterTile', () => {
		expect(GRID_MARKUP, `${GRID} draws no <img> at all`).toContain('<img');
		expect(
			GRID_MARKUP,
			`${GRID} builds a poster src by hand. posterUrl owns the width allowlist and the ` +
				'key check, and a second copy of either is a second thing to get wrong'
		).toContain('{src}');
		expect(GRID_MARKUP, `${GRID} no longer builds its tiles with posterTile`).toContain(
			'posterTile(item)'
		);
		expect(
			GRID_MARKUP,
			`${GRID} spells an /img path in its own markup rather than taking posterUrl's`
		).not.toContain('/img/');
	});

	/*
	 * ⚠️ THE `<img>` MUST HAVE AN ERROR HANDLER, AND IT IS THE MARKUP HALF OF THE
	 * RULE `posterArtSrc` HOLDS. A key is not a promise that bytes exist —
	 * `GET /img/{key}` is a cache read that answers `404 not_cached` — so without
	 * this the browser draws its own broken-image glyph on every such tile. The
	 * function decides what to draw AFTER a failure; nothing but this attribute
	 * can tell it one happened.
	 */
	it('reports a failed image rather than leaving the browser to draw one', () => {
		expect(
			GRID_MARKUP,
			`${GRID}'s <img> has no onerror, so a key whose bytes are not cached renders the ` +
				"browser's own broken-image glyph instead of the empty tile"
		).toContain('onerror=');
		expect(GRID_MARKUP, `${GRID} no longer routes its src through posterArtSrc`).toContain(
			'posterArtSrc(tile,'
		);
	});

	/*
	 * The browser's own deferral, which costs no script and no observer. §17.1
	 * bans animation on a grid, and an image faded in by a load handler is that
	 * effect under another name.
	 */
	it('defers decode and load to the browser', () => {
		expect(GRID_MARKUP, `${GRID} stopped lazy-loading its art`).toContain('loading="lazy"');
		expect(GRID_MARKUP, `${GRID} stopped decoding its art off the main thread`).toContain(
			'decoding="async"'
		);
	});

	/*
	 * §4.4.1 rule 3 and §17.1: the empty tile is a `dominant_color` fill, never
	 * a grey box and never a shimmer, and `aspect-ratio` reserves the box so a
	 * decoded image shifts nothing. DESIGN-DIRECTION §13 adds "it never pulses".
	 */
	it('reserves the tile and never animates it', () => {
		const style = /<style>([\s\S]*)<\/style>/.exec(GRID_SOURCE)?.[1] ?? '';
		expect(style.length, `${GRID} has no <style> block to read`).toBeGreaterThan(0);
		const art = style.slice(style.indexOf('.postercard__art'));
		expect(art.length, `${GRID} no longer styles .postercard__art`).toBeGreaterThan(0);
		expect(art, 'the tile reserves no box, so a decoded image shifts the grid').toContain(
			'aspect-ratio'
		);
		expect(art, 'the tile stopped falling back to a colour fill').toContain('var(--dc,');
		for (const banned of ['animation', 'transition', '@keyframes']) {
			expect(
				style.slice(style.indexOf('.postergrid')),
				`${GRID}'s poster grid carries ${banned}. §17.1 bans animation on any list, grid ` +
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
		const art = GRID_MARKUP.indexOf('postercard__art');
		const title = GRID_MARKUP.indexOf('postercard__title');
		expect(art, 'the card no longer draws a tile').toBeGreaterThanOrEqual(0);
		expect(title, 'the card no longer draws a title line').toBeGreaterThanOrEqual(0);
		expect(
			title,
			'the title moved above or inside the tile. §9.2 puts it below, on the chrome, ' +
				'because no contrast solve over an averaged colour can make text on cover art safe'
		).toBeGreaterThan(art);
		expect(GRID_MARKUP, 'the title line is no longer ellipsised to one line').toContain(
			'postercard__title trunc'
		);
	});

	/*
	 * ⚠️ THE CARDS ARE INERT AND THAT IS THE CORRECT STATE TODAY, so it is
	 * asserted rather than left to be noticed. §17.1 requires anything that
	 * navigates to be a real `<a href>` that middle-clicks; the item route
	 * `/library/{type}/{id}` does not exist in `routes/`, so a card wrapped in a
	 * link would be a link to nowhere and a card wrapped in a click handler would
	 * be the `<div>` that rule bans. When that route lands, this expectation is
	 * what has to be rewritten, which is the point of writing it down.
	 */
	it('navigates nowhere, because there is nowhere to navigate to', () => {
		expect(
			GRID_MARKUP,
			`${GRID} made its cards clickable without a real href. §17.1: navigation is an <a>, ` +
				'never a click handler on a div'
		).not.toContain('onclick=');
		expect(
			GRID_MARKUP,
			`${GRID} grew a link — if /library/{type}/{id} now exists, this ` +
				'expectation is the thing to update rather than delete'
		).not.toContain('<a ');
	});
});
