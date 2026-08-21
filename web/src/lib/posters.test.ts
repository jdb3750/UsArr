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
 * that still spells it.
 *
 * ⚠️ AND THE REASON IS THAT THEY WOULD HAVE FAILED, NOT THAT THEY WOULD HAVE
 * PASSED VACUOUSLY. This paragraph used to claim the repointed assertions "would
 * have gone green over a corpus that cannot fail them", and that is measurably
 * false: restoring the pre-move version of this file over the moved tree fails
 * loudly in four places — `BLOCK_C` has no `<img`, no `<PosterGrid` arm to find,
 * no `.postercard__art` and no tile to reserve. They follow their subject into
 * `PosterGrid.svelte?raw` because the subject moved, which is the ordinary
 * reason, and a guard that goes red when its subject leaves is the guard working.
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
	chooseLibraryView,
	libraryViewKey,
	LIBRARY_VIEWS,
	LIBRARY_VIEW_DEFAULT,
	parseLibraryView,
	readLibraryView,
	type LibraryViewChoice,
	type LibraryViewStorage
} from './librarygrid';
import { sectionsMarkup, userFacingMarkup } from './copyguard';
// Home AS TEXT. The `{#if}` that swaps the two panels is markup, and markup is
// the one thing this suite cannot execute.
import HOME_SOURCE from '../routes/+page.svelte?raw';
// The card AS TEXT, for the same reason and off the file that now draws it.
import GRID_SOURCE from './PosterGrid.svelte?raw';
// ⚠️ AND THE TWO CATALOGUE SCREENS, WHICH GREW THE SAME TOGGLE AND HAD NONE OF
// THIS ASSERTED OVER THEM. Home's toggle block was the only one, so replacing
// both of these toggles with keyboard-unreachable `<div role="button">`s and
// dropping `role="group"` and `aria-label` left all 1,025 tests green.
import LIBRARY_SOURCE from '../routes/library/+page.svelte?raw';
import LIBRARY_TYPE_SOURCE from '../routes/library/[type]/+page.svelte?raw';

const HOME = 'routes/+page.svelte';
const GRID = 'lib/PosterGrid.svelte';
const LIBRARY = 'routes/library/+page.svelte';
const LIBRARY_TYPE = 'routes/library/[type]/+page.svelte';

/** The component's markup, with its script, style and comments stripped. */
const GRID_MARKUP = userFacingMarkup(GRID_SOURCE);

/** Block C, sliced to its own `</section>`. Throws when the id moves. */
const BLOCK_C = sectionsMarkup(userFacingMarkup(HOME_SOURCE), 'id="home-recent"').join('\n');

/** The two catalogue screens, stripped of script, style and comments. */
const LIBRARY_MARKUP = userFacingMarkup(LIBRARY_SOURCE);
const LIBRARY_TYPE_MARKUP = userFacingMarkup(LIBRARY_TYPE_SOURCE);

/**
 * EVERY SCREEN THAT DRAWS THE VIEW TOGGLE, WITH THE CORPUS AND THE HOLDER EACH
 * ONE SPELLS.
 *
 * ⚠️ A LIST RATHER THAN THREE COPIES OF ONE BLOCK, because the defect this
 * replaces was not a weak assertion — it was a MISSING one. The toggle rules
 * were asserted over Home alone, so the two catalogue screens grew the same
 * control with none of it held: swapping both of their `<button type="button">`s
 * for `<div role="button" tabindex="-1">` and dropping `role="group"` and
 * `aria-label="View mode"` left the whole suite green. A fourth screen that
 * grows this toggle is added here, and gets every rule at once.
 *
 * THE HOLDER DIFFERS AND THE RULE DOES NOT. Home reads `homeView` because it has
 * one key for the all-types view; the catalogue screens read `view`, a
 * `createLibraryView` keyed per media type. What is asserted is that the pressed
 * state is bound to the screen's OWN holder, not that they share a name.
 */
const SCREENS = [
	{
		name: HOME,
		markup: BLOCK_C,
		holder: 'homeView',
		views: HOME_VIEWS,
		// Home writes one button per view by hand, so every view is named here.
		pressed: HOME_VIEWS.map((view) => `homeView.current === '${view}'`),
		over: undefined
	},
	{
		name: LIBRARY,
		markup: LIBRARY_MARKUP,
		holder: 'view',
		views: LIBRARY_VIEWS,
		pressed: ['view.current === choice'],
		over: 'LIBRARY_VIEWS'
	},
	{
		name: LIBRARY_TYPE,
		markup: LIBRARY_TYPE_MARKUP,
		holder: 'view',
		views: LIBRARY_VIEWS,
		pressed: ['view.current === choice'],
		over: 'LIBRARY_VIEWS'
	}
] as const;

/**
 * EVERY `aria-pressed={…}` EXPRESSION IN `markup`, IN SOURCE ORDER.
 *
 * Read as text rather than matched against a literal per view, because the three
 * screens spell the same rule two ways and both are correct: Home writes one
 * button per view, and the catalogue screens iterate `LIBRARY_VIEWS`, so one
 * binding there covers every view and a third view would grow a button without
 * an edit to the template. What has to hold on all three is that the expression
 * is the screen's OWN holder — a button bound to Home's `homeView` on a
 * catalogue screen would make choosing posters on one change the other.
 */
function pressedBindings(markup: string): string[] {
	return [...markup.matchAll(/aria-pressed=\{([^}]*)\}/g)].map((m) => m[1]!.trim());
}

/**
 * THE TAG NAME OF EVERY ELEMENT IN `markup` THAT CARRIES `attr`.
 *
 * ⚠️ THIS REPLACES `expect(corpus).toContain('<button')`, WHICH WAS NOT A CHECK
 * ON THE TOGGLE AT ALL. Block C also draws the error banner's restart button, so
 * that assertion passed with the toggle's buttons deleted outright — it only
 * ever proved that the section contained *some* button. `aria-pressed` is the
 * toggle's own attribute, so asking what tag carries it asks about the toggle
 * and nothing else: a `<div role="button">` answers `div` and fails, and it
 * fails for the reason that matters, which is Tab, Space and Enter.
 */
/** The `<PosterGrid …/>` tag a screen draws, or `undefined` if it draws none. */
function posterGridTag(markup: string): string | undefined {
	const at = markup.indexOf('<PosterGrid');
	if (at < 0) return undefined;
	const end = markup.indexOf('>', at);
	return end < 0 ? undefined : markup.slice(at, end + 1);
}

function tagsCarrying(markup: string, attr: string): string[] {
	const tags: string[] = [];
	for (let from = 0; ;) {
		const at = markup.indexOf(attr, from);
		if (at < 0) return tags;
		const open = markup.lastIndexOf('<', at);
		tags.push(open < 0 ? '' : (/^<\s*([A-Za-z][-\w]*)/.exec(markup.slice(open))?.[1] ?? ''));
		from = at + attr.length;
	}
}

/**
 * A CORPUS THAT STARTS AT `marker`, OR A FAILURE — NEVER A ONE-CHARACTER STRING.
 *
 * ⚠️ THIS EXISTS BECAUSE THE BARE IDIOM IS A TRAP AND THIS FILE FELL INTO IT.
 * `text.slice(text.indexOf(marker))` on a MISS is `slice(-1)`: the last
 * character of the file. Every `not.toContain` over that passes, and so does
 * `expect(corpus.length).toBeGreaterThan(0)` — a one-character string is not
 * empty. The anti-animation guard below was therefore satisfied by renaming
 * `.postergrid`, which nothing else pins any more, and a grid carrying
 * `animation: shimmer 2s infinite` went green through the whole gate.
 *
 * So the index is asserted BEFORE it is used, and the message names the file and
 * the marker, because the thing that goes wrong here is a rename rather than a
 * regression in what the CSS says.
 */
function corpusFrom(text: string, marker: string): string {
	const at = text.indexOf(marker);
	expect(
		at,
		`${GRID} no longer spells ${marker}, so every rule asserted over it would read the last ` +
			'character of the file and pass over anything'
	).toBeGreaterThanOrEqual(0);
	return text.slice(at);
}

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
		expect(posterArtSrc(tile, {})).toBe(tile.src);
	});

	/*
	 * The defect: `GET /img/{key}` is a cache read and answers `404 not_cached`
	 * for a key whose bytes have not been rendered yet, which is ordinary rather
	 * than a fault. Without this the browser draws its own broken-image glyph.
	 */
	it('falls back to the empty tile once the image has failed', () => {
		const tile = posterTile(ITEMS[0]!);
		expect(
			posterArtSrc(tile, { [tile.id]: true }),
			'a cover that 404d is still being handed to the <img>, so the browser draws its ' +
				'own broken-image glyph rather than the tile the absent-key case draws'
		).toBeUndefined();
	});

	/*
	 * One failure is one card. A record keyed by anything coarser would blank the
	 * whole grid the first time any single cover was missing.
	 *
	 * The second work takes the FIXTURE'S key rather than a fresh literal: what
	 * distinguishes the two cards here is the work id, which is what the record is
	 * keyed on, and a second sixteen-hex string in a test file is a secret
	 * scanner's finding waiting to happen.
	 */
	it('blanks only the work that failed', () => {
		const failed: Record<number, true> = { [ITEMS[0]!.id]: true };
		const other = posterTile(item({ id: 99, posterKey: ITEMS[0]!.posterKey! }));
		expect(posterArtSrc(other, failed)).toBe(other.src);
	});

	/* The absent-key case is unchanged and still absent: a failure set that does
	 * not name it must not turn `undefined` into anything else. */
	it('leaves a work with no artwork absent either way', () => {
		const tile = posterTile(ITEMS[1]!);
		expect(posterArtSrc(tile, {})).toBeUndefined();
		expect(posterArtSrc(tile, { [tile.id]: true })).toBeUndefined();
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

/*
 * ⚠️ THE KEY IS READ AT CLICK TIME, AND THIS IS THE BLOCK THAT MAKES THAT A RULE
 * RATHER THAN A COMMENT.
 *
 * SvelteKit reuses `routes/library/[type]/+page.svelte` across
 * `/library/movies` → `/library/ebooks` rather than remounting it, so a key
 * resolved once at construction follows the reader into the next media type and
 * is then written back under the one they left. `$lib/libraryview.svelte.ts`
 * marked that MUST in its own header and nothing held it: the file carries runes,
 * so this node-environment suite cannot import it, and replacing the getter with
 * a closure over a captured value left the whole gate green. The decision moved
 * into `$lib/librarygrid` — a plain module — so it could be run here for real.
 *
 * A FAKE STORAGE RATHER THAN A localStorage SHIM, because which key is read and
 * which key is WRITTEN is the entire subject, and a map answers that directly.
 */
describe('readLibraryView / chooseLibraryView', () => {
	const MOVIES = libraryViewKey('movies');
	const EBOOKS = libraryViewKey('ebooks');

	function storage(seed: Record<string, string> = {}): LibraryViewStorage & {
		written: Map<string, string>;
	} {
		const written = new Map(Object.entries(seed));
		return {
			written,
			read: (key) => written.get(key) ?? null,
			write: (key, value) => void written.set(key, value)
		};
	}

	function choice(): LibraryViewChoice {
		return { key: undefined, view: undefined };
	}

	it('has two distinct keys to be wrong about', () => {
		// The floor under every assertion below: one key for both media types
		// would make "the choice did not follow the reader" vacuously true.
		expect(MOVIES, 'two media types share one stored view').not.toBe(EBOOKS);
	});

	it('opens as a table when storage has never been written', () => {
		expect(readLibraryView(() => MOVIES, choice(), storage())).toBe(LIBRARY_VIEW_DEFAULT);
	});

	it('reads back what storage holds for this key', () => {
		const store = storage({ [MOVIES]: 'posters' });
		expect(readLibraryView(() => MOVIES, choice(), store)).toBe('posters');
	});

	it('falls back rather than throwing on a value it did not write', () => {
		const store = storage({ [MOVIES]: 'overview' });
		expect(readLibraryView(() => MOVIES, choice(), store)).toBe(LIBRARY_VIEW_DEFAULT);
	});

	it('serves the choice made in this page view, under the key it was made for', () => {
		const chosen = choice();
		const store = storage();
		chooseLibraryView('posters', () => MOVIES, chosen, store);
		expect(readLibraryView(() => MOVIES, chosen, store)).toBe('posters');
		expect(store.written.get(MOVIES), 'the choice was never persisted').toBe('posters');
	});

	/*
	 * ⚠️ THE DEFECT, IN ITS OBSERVABLE FORM. The reader chooses posters on
	 * `/library/movies`, then navigates to `/library/ebooks`, which SvelteKit
	 * serves from the same component instance. Ebooks was never chosen and ebooks
	 * has nothing stored, so it must open as a table.
	 */
	it('does not follow the reader into a media type they never chose it for', () => {
		let key = MOVIES;
		const at = () => key;
		const chosen = choice();
		const store = storage();

		chooseLibraryView('posters', at, chosen, store);
		key = EBOOKS;

		expect(
			readLibraryView(at, chosen, store),
			'a choice made under one media type is being served under another, so the key was ' +
				'captured rather than read at access time'
		).toBe(LIBRARY_VIEW_DEFAULT);
	});

	/* And the write half of the same defect: nothing may land under a key the
	 * reader has not chosen anything for. */
	it('writes under the key that is current at the moment of the click', () => {
		let key = MOVIES;
		const at = () => key;
		const chosen = choice();
		const store = storage();

		chooseLibraryView('posters', at, chosen, store);
		key = EBOOKS;
		chooseLibraryView('table', at, chosen, store);

		expect(store.written.get(MOVIES), 'the movies choice was overwritten from another screen').toBe(
			'posters'
		);
		expect(store.written.get(EBOOKS), 'the ebooks choice landed under the wrong key').toBe('table');
	});

	/* Coming back is the other half: the choice made under movies is still
	 * movies\u2019, and storage answers for it once the in-memory pairing has moved on. */
	it('still answers for a media type the reader returns to', () => {
		let key = MOVIES;
		const at = () => key;
		const chosen = choice();
		const store = storage();

		chooseLibraryView('posters', at, chosen, store);
		key = EBOOKS;
		chooseLibraryView('table', at, chosen, store);
		key = MOVIES;

		expect(readLibraryView(at, chosen, store)).toBe('posters');
	});
});

/*
 * AND THE CALL SITES, WHICH ARE THE OTHER PLACE THE GETTER CAN BE DEFEATED.
 * `createLibraryView(() => capturedKey)` satisfies the `key: () => string`
 * signature perfectly and reintroduces the whole defect, so what is asserted is
 * that the arrow RECOMPUTES the key rather than closing over one.
 */
describe('the catalogue screens pass a key that is recomputed, not captured', () => {
	for (const [name, source, arg] of [
		[LIBRARY, LIBRARY_SOURCE, 'libraryViewKey(undefined)'],
		[LIBRARY_TYPE, LIBRARY_TYPE_SOURCE, 'libraryViewKey(query?.mediaType)']
	] as const) {
		it(name, () => {
			expect(
				source,
				`${name} passes createLibraryView something other than a getter that recomputes the ` +
					'key. A captured string type-checks and then writes the reader\u2019s choice under ' +
					'the media type they just left'
			).toContain(`createLibraryView(() => ${arg})`);
		});
	}
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
});

/*
 * THE VIEW TOGGLE, ON EVERY SCREEN THAT DRAWS ONE.
 *
 * The control is app.css's `.segment`, which shipped with its
 * `[aria-pressed='true']` state already styled and no consumer. Native buttons
 * in a named group: `aria-pressed` is what a toggle's state means, and the group
 * needs a name because "table / posters" says nothing on its own.
 */
describe('the view toggle', () => {
	it('has three screens to be wrong about', () => {
		// The floor under the loop: a SCREENS list that lost an entry would run
		// every rule below over the screens that remain and report nothing.
		expect(SCREENS.length, 'a screen that draws the toggle is no longer checked').toBe(3);
		for (const screen of SCREENS) {
			expect(screen.markup.length, `${screen.name} sliced to nothing`).toBeGreaterThan(0);
		}
	});

	for (const { name, markup, holder, views, pressed, over } of SCREENS) {
		describe(name, () => {
			it('draws a named group of real buttons', () => {
				expect(markup, `${name} dropped the .segment button group`).toContain('class="segment"');
				expect(markup, `${name}'s button group lost its role`).toContain('role="group"');
				expect(markup, `${name}'s button group lost its accessible name`).toContain(
					'aria-label="View mode"'
				);
			});

			it('binds every pressed state to this screen\u2019s own holder', () => {
				const bound = pressedBindings(markup);
				expect(
					bound,
					`${name}'s toggle no longer reports its pressed state the way this screen spells ` +
						`it. Whatever replaced it must still read ${holder}, because a catalogue screen ` +
						"bound to Home's holder makes choosing posters on one change the other"
				).toEqual([...pressed]);
				for (const expr of bound) {
					expect(expr, `${name}'s toggle reads a holder that is not its own`).toContain(
						`${holder}.current ===`
					);
				}
			});

			/*
			 * ONE BUTTON PER VIEW, however the screen spells it. An unrolled toggle
			 * has to name every view; a looped one has to loop over the canonical
			 * list rather than a hand-written array that can fall behind it.
			 */
			it('offers a control for every view this screen ships', () => {
				if (over === undefined) {
					expect(
						pressed.length,
						`${name} writes its buttons out by hand and no longer writes one per view`
					).toBe(views.length);
					return;
				}
				expect(
					markup,
					`${name}'s toggle no longer iterates ${over}, so its buttons and the view list ` +
						'can fall out of step'
				).toContain(`{#each ${over} as `);
			});

			/*
			 * ⚠️ ASKED OF THE TAG THAT CARRIES `aria-pressed`, NOT OF THE CORPUS.
			 * `toContain('<button')` over a whole screen is satisfied by any other
			 * button on it — Block C's error banner has one — so it passed with the
			 * toggle's buttons deleted. This cannot.
			 */
			it('makes the toggle out of real buttons, so it keeps Tab, Space and Enter', () => {
				const tags = tagsCarrying(markup, 'aria-pressed=');
				expect(tags.length, `${name} draws no aria-pressed control at all`).toBeGreaterThan(0);
				expect(
					[...new Set(tags)],
					`${name}'s toggle is no longer made of real buttons, so it has lost Tab, Space and ` +
						'Enter'
				).toEqual(['button']);
			});
		});
	}
});

describe('the catalogue screens\u2019 posters arm', () => {
	for (const { name, markup, holder } of SCREENS.filter((s) => s.name !== HOME)) {
		describe(name, () => {
			/*
			 * The same component Home draws, rather than a second copy of the card.
			 * A screen that re-inlined the grid would have its own geometry, its own
			 * `?w=` and its own broken-image behaviour to keep in step.
			 */
			it('draws the shared grid rather than a card of its own', () => {
				expect(
					markup,
					`${name} no longer draws <PosterGrid>. The grid, the card and the empty tile are ` +
						`${GRID}'s, and a screen that inlines its own copy drifts from it`
				).toContain('<PosterGrid');
			});

			/*
			 * §17.2's rule as ADR-0028 amended it, and Block C's rule applies to
			 * these screens for the same reason: the table and the grid are the two
			 * arms of ONE `{#if}`, so exactly one is in the DOM. Both drawn at once
			 * is the screen as two regions; neither is a toolbar over nothing.
			 */
			it('swaps the table for the grid rather than drawing both', () => {
				expect(
					markup,
					`${name} no longer gates its table on the view preference, so either both panels ` +
						'are drawn at once or neither is'
				).toContain(`{#if ${holder}.current === 'table'}`);
				const table = markup.indexOf('<List');
				const grid = markup.indexOf('<PosterGrid');
				expect(table, `${name} draws no table arm`).toBeGreaterThanOrEqual(0);
				expect(grid, `${name} draws no posters arm`).toBeGreaterThanOrEqual(0);
				expect(
					markup.slice(table, grid),
					'the table and the grid are no longer alternatives of one {#if}'
				).toContain('{:else');
			});
		});
	}
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
		const art = corpusFrom(style, '.postercard__art');
		expect(art, 'the tile reserves no box, so a decoded image shifts the grid').toContain(
			'aspect-ratio'
		);
		expect(art, 'the tile stopped falling back to a colour fill').toContain('var(--dc,');
		const grid = corpusFrom(style, '.postergrid');
		for (const banned of ['animation', 'transition', '@keyframes']) {
			expect(
				grid,
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

/*
 * §9.2's AVAILABILITY RULE, AND THE PROP THAT KEEPS IT OFF HOME.
 *
 * DESIGN-DIRECTION §9.2: *"Availability renders per §6.3's rollup rule: `have ==
 * total && total > 0` → ✓; `have == 0` → ✗; otherwise the fraction"*. Both
 * catalogue screens draw a Have column in their TABLE arm, so a posters view
 * without it drops a column the same screen showed a moment ago.
 *
 * ⚠️ AND IT IS OFF BY DEFAULT, WHICH IS THE HALF A TEST HAS TO HOLD. Home's
 * Block C is ARCHITECTURE §17.2's and ADR-0028's shape, not this component's, so
 * its rendered output must not move because a card gained a capability. The
 * default and the two call sites are asserted separately, because a default that
 * flipped would change Home without changing Home.
 */
describe('availability on the poster card', () => {
	it('renders the rollup through the cell the table arm already uses', () => {
		expect(
			GRID_MARKUP,
			`${GRID} draws no <HaveCell>, so §9.2's availability rule is unrendered on the card`
		).toContain('<HaveCell');
		expect(
			GRID_SOURCE,
			`${GRID} reimplements the have/total rollup instead of taking $lib/library.haveCell's. ` +
				'schema.md is explicit that markup must not reconstruct that comparison'
		).not.toContain('haveCount');
	});

	it('draws it only when the caller asks, and defaults to not asking', () => {
		expect(
			GRID_SOURCE,
			`${GRID} no longer defaults availability off, so Home's Block C would start drawing a ` +
				'rollup §17.2 never asked it for'
		).toContain('availability = false');

		const gate = GRID_MARKUP.indexOf('{#if availability}');
		const cell = GRID_MARKUP.indexOf('<HaveCell');
		expect(gate, `${GRID} no longer gates the rollup on the prop`).toBeGreaterThanOrEqual(0);
		expect(
			cell,
			`${GRID}'s <HaveCell> is drawn outside {#if availability}, so every caller gets it`
		).toBeGreaterThan(gate);
		expect(
			GRID_MARKUP.slice(gate, cell),
			'something else opened a block between the gate and the cell'
		).not.toContain('{/if}');
	});

	it("leaves Home's Block C exactly as §17.2 and ADR-0028 specify it", () => {
		const tag = posterGridTag(BLOCK_C);
		expect(tag, `${HOME} draws no <PosterGrid>`).toBeDefined();
		expect(
			tag,
			`${HOME} turned the rollup on for Block C. That is a §17.2 decision about what that ` +
				'section renders, not a side effect of a change to the card'
		).not.toContain('availability');
	});

	for (const [name, source] of [
		[LIBRARY, LIBRARY_MARKUP],
		[LIBRARY_TYPE, LIBRARY_TYPE_MARKUP]
	] as const) {
		it(`turns it on for ${name}, which draws a Have column in its table arm`, () => {
			expect(
				source,
				`${name}'s table arm draws <HaveCell> and its posters arm must not silently drop it`
			).toContain('<HaveCell');
			const tag = posterGridTag(source);
			expect(tag, `${name} draws no <PosterGrid>`).toBeDefined();
			expect(
				tag,
				`${name} toggles to a posters view that drops the Have column the table arm shows`
			).toContain('availability');
		});
	}
});
