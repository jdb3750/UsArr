/*
 * THE SCOPE SELECT, GUARDED — `$lib/scopeselect` and the two toolbars over it.
 *
 * ⚠️ THIS IS NOT §8.1's SCOPE CHIP AND NOTHING BELOW CLAIMS IT IS. The chip is
 * a shell-level multi-select popover with a top-bar hoist; this is the
 * screen-local single-select `<select>` that changes `?lib=` on `/library` and
 * `/library/[type]`. A guard here going green says nothing whatever about the
 * chip, which remains unbuilt.
 *
 * WHY SOME OF IT READS SOURCE. `vitest.config.ts` is `environment: 'node'` with
 * no Svelte plugin, so a component cannot be imported and compiled at all. The
 * repo's answer is already in the tree — `librarygrid.test.ts`,
 * `libraryscreen.test.ts` and `home.test.ts` all read a template through Vite's
 * `?raw` and assert over the text — so the rules that live in markup are held
 * that way and every decision that could be a function is one.
 *
 * ⚠️ EVERY GUARD BELOW PROVES IT FOUND SOMETHING BEFORE IT ASSERTS AN ABSENCE.
 * That is not ceremony: `expect('').not.toContain(anything)` passes, so a slice
 * that silently stops matching turns a rule into a rumour that reports green for
 * ever. This repo has shipped a vacuous guard before.
 */

import { describe, expect, it } from 'vitest';
import { libraryNames, readLibraryScope, type LibraryNames } from './librarygrid';
import {
	distinctSlugs,
	EVERY_LIBRARY,
	scopeSelectOptions,
	scopeSelectSearch,
	scopeSelectValue,
	scopeSelectWorthShowing
} from './scopeselect';
import TYPE_ROUTE_SOURCE from '../routes/library/[type]/+page.svelte?raw';
import LIBRARY_ROUTE_SOURCE from '../routes/library/+page.svelte?raw';

const SCREENS: readonly (readonly [string, string])[] = [
	['routes/library/[type]/+page.svelte', TYPE_ROUTE_SOURCE],
	['routes/library/+page.svelte', LIBRARY_ROUTE_SOURCE]
];

const names = (...pairs: readonly (readonly [string, string])[]): LibraryNames =>
	libraryNames(pairs.map(([slug, name]) => ({ slug, name })));

const KIDS = ['kids', 'Kids'] as const;
const VINYL = ['vinyl-rips', 'Vinyl rips'] as const;

describe('the scope select never writes an address the endpoint refuses', () => {
	/*
	 * ⚠️ THE ONE WIRE FACT IN THIS FILE. `resolveBrowseLibraries` tests
	 * `query.Has("lib")` — PRESENCE, not emptiness — so `?lib=`, `?lib=%20` and
	 * `?lib=,,` are all `400 "lib was sent with no library in it"`, while an
	 * empty `media_type` is silently unfiltered. The two parameters fail in
	 * opposite directions and this is the one that is easy to get backwards.
	 */
	it.each(['', ' ', ',,', ' , '])('deletes lib rather than emptying it for %o', (value) => {
		const params = new URLSearchParams('sort=sort_title&lib=kids');
		const search = scopeSelectSearch(params, value);
		// The positive half FIRST: prove the function is still writing an address
		// at all, so the absence below is an absence from something.
		expect(search, 'the rest of the address was dropped along with the scope').toContain(
			'sort=sort_title'
		);
		expect(search, `${value || '(empty)'} produced a lib parameter, which is a 400`).not.toMatch(
			/(^|&)lib=/
		);
	});

	it('sets lib when a real slug is chosen, and leaves the sort alone', () => {
		const search = scopeSelectSearch(new URLSearchParams('sort=popularity'), 'kids');
		expect(search).toContain('lib=kids');
		expect(search, 'switching library lost the order the user was reading').toContain(
			'sort=popularity'
		);
	});

	/*
	 * THE TRAP THIS CONTROL IS REQUIRED TO AVOID. `readLibraryScope` does not
	 * dedupe, so `?lib=a,a` reads as two slugs: it counts twice against
	 * MAX_LIBRARY_SLUGS and renders as "Scoped to Kids, Kids". The reader is left
	 * alone deliberately; what is settled here is that this control cannot be the
	 * thing that writes one.
	 */
	it('never writes the same slug twice', () => {
		const search = scopeSelectSearch(new URLSearchParams(), 'kids,kids');
		expect(search, 'the scope was dropped entirely rather than deduped').toContain('lib=kids');
		expect(readLibraryScope(new URLSearchParams(search))).toEqual(['kids']);
	});

	/*
	 * THE PROPERTY THAT MATTERS MORE THAN ANY SINGLE CASE: whatever this control
	 * writes, the reader on the other side of the address reads back as the same
	 * scope. The two functions are in different modules and nothing but this ties
	 * them together.
	 */
	it.each([
		['', []],
		['kids', ['kids']],
		['kids,kids', ['kids']],
		['kids,vinyl-rips', ['kids', 'vinyl-rips']]
	] as const)('round-trips %o through readLibraryScope', (value, expected) => {
		const search = scopeSelectSearch(new URLSearchParams(), value);
		expect(readLibraryScope(new URLSearchParams(search))).toEqual([...expected]);
	});

	it('trims, drops empties and dedupes in first-seen order', () => {
		expect(distinctSlugs([' b ', 'a', 'b', '', '  ', 'a'])).toEqual(['b', 'a']);
	});
});

describe('the scope select can always state the scope the address carries', () => {
	it('offers every library plus the unscoped entry', () => {
		const options = scopeSelectOptions(names(KIDS, VINYL), []);
		expect(options).toEqual([
			{ value: '', label: EVERY_LIBRARY },
			{ value: 'kids', label: 'Kids' },
			{ value: 'vinyl-rips', label: 'Vinyl rips' }
		]);
	});

	it('selects a known library through its own entry rather than a second one', () => {
		const options = scopeSelectOptions(names(KIDS, VINYL), ['kids']);
		expect(options.filter((o) => o.value === 'kids')).toHaveLength(1);
		expect(scopeSelectValue(['kids'])).toBe('kids');
	});

	/*
	 * ⚠️ A SLUG THE LIST DOES NOT HAVE IS NOT DROPPED. `?lib=ghost` after a
	 * rename or a delete, or any scope read before `GET /api/v1/libraries`
	 * answers. Dropping it would leave the control claiming a scope the screen is
	 * not in — and the `<select>` would fall back to rendering its first option,
	 * which says "every library" over a page that is scoped.
	 */
	it('keeps an unresolved slug, labelled with the slug', () => {
		const options = scopeSelectOptions(names(KIDS), ['ghost']);
		const current = options.find((o) => o.value === 'ghost');
		expect(current, 'the address named a library the control cannot state').toBeDefined();
		expect(current?.label).toBe('ghost');
		expect(scopeSelectValue(['ghost'])).toBe('ghost');
	});

	/*
	 * `?lib=a,b` is expressible in the URL although no control writes one. A
	 * single-select cannot express it either, so it appears as ONE entry naming
	 * both — and picking a library REPLACES it rather than adding to it.
	 */
	it('states a two-library scope as one entry naming both', () => {
		const options = scopeSelectOptions(names(KIDS, VINYL), ['kids', 'vinyl-rips']);
		const current = options.find((o) => o.value === 'kids,vinyl-rips');
		expect(current, 'a two-library scope vanished from the control').toBeDefined();
		expect(current?.label).toBe('Kids, Vinyl rips');
		expect(scopeSelectValue(['kids', 'vinyl-rips'])).toBe('kids,vinyl-rips');
	});

	it('dedupes the entry it builds from a duplicated address', () => {
		const options = scopeSelectOptions(names(KIDS), ['kids', 'kids']);
		expect(options.map((o) => o.value)).toEqual(['', 'kids']);
		expect(scopeSelectValue(['kids', 'kids'])).toBe('kids');
	});

	it('gives the select a value that matches one of its own options', () => {
		for (const scope of [[], ['kids'], ['ghost'], ['kids', 'vinyl-rips'], ['kids', 'kids']]) {
			const options = scopeSelectOptions(names(KIDS, VINYL), scope);
			expect(
				options.map((o) => o.value),
				`no option carries the value for ${JSON.stringify(scope)}, so the control would ` +
					'render its first entry and misstate the scope'
			).toContain(scopeSelectValue(scope));
		}
	});
});

describe('a control that can do nothing is not on the screen', () => {
	/*
	 * ⚠️ ZERO LIBRARIES IS AN ABSENT CONTROL, NOT AN EMPTY ONE. A dropdown whose
	 * only entry is the state you are already in reads as a broken screen. A read
	 * that failed answers HAS NOT ANSWERED rather than an empty map — the two are
	 * distinguishable now, because `libraryScopeLine` has to tell them apart — and
	 * this function still gives both the same answer, which is why both are
	 * asserted below rather than only one.
	 */
	it('hides at zero libraries and while the read is in flight', () => {
		expect(scopeSelectWorthShowing(undefined)).toBe(false);
		expect(scopeSelectWorthShowing(names())).toBe(false);
	});

	/* ONE LIBRARY SHIPS IT: that library and every library are two states a user
	 * can be in and can move between, which is work for a switcher to do. */
	it('ships at one library', () => {
		expect(scopeSelectWorthShowing(names(KIDS))).toBe(true);
		expect(scopeSelectOptions(names(KIDS), []).map((o) => o.value)).toEqual(['', 'kids']);
	});
});

describe('both browse toolbars carry the control', () => {
	/** The `{#if showScopeSelect}` block, which is the whole of the markup. */
	const sliceOf = (route: string, source: string) => {
		const open = source.indexOf('{#if showScopeSelect}');
		expect(
			open,
			`${route} no longer renders the scope select, so that screen cannot change its library`
		).toBeGreaterThanOrEqual(0);
		const end = source.indexOf('{/if}', open);
		expect(end, `${route}'s scope select is not a block`).toBeGreaterThan(open);
		return source.slice(open, end);
	};

	it.each(SCREENS)('%s renders a real select over scopeOptions', (route, source) => {
		const slice = sliceOf(route, source);
		expect(slice, `${route} renders no <select>`).toContain('<select');
		expect(slice, `${route}'s options are not built from scopeOptions`).toContain(
			'{#each scopeOptions as option (option.value)}'
		);
		expect(slice, `${route}'s select does not write the address`).toContain('onchange={onScope}');
		expect(slice, `${route}'s select does not show the scope it is in`).toContain(
			'value={scopeValue}'
		);
	});

	/*
	 * ⚠️ THE SAME CLASSES AS THE Sort SELECT BESIDE IT. One toolbar, one idiom,
	 * no new CSS: a second control styled its own way in the same row is the
	 * invented pattern DESIGN-DIRECTION rules out, and `.select`/`.toolbar__label`
	 * already exist in `app.css` for exactly this.
	 */
	it.each(SCREENS)('%s styles it as the toolbar already styles Sort', (route, source) => {
		const slice = sliceOf(route, source);
		expect(slice, `${route}'s scope select is not a .select`).toContain('class="select"');
		expect(slice, `${route}'s scope label is not a .toolbar__label`).toContain(
			'class="toolbar__label"'
		);
	});

	/*
	 * ⚠️ THE CONTROL MUST NOT BE MISTAKEN FOR §8.1's CHIP. If this ships wearing
	 * the chip's name, the chip reads as discharged while the sidebar, the
	 * `scope-empty` state and the top-bar hoist are all untouched. The markup is
	 * required to say what it is NOT.
	 */
	it.each(SCREENS)('%s says in the markup that it is not the chip', (route, source) => {
		const slice = sliceOf(route, source);
		expect(
			slice.length,
			`${route}'s scope-select slice is empty, so this guard reads nothing`
		).toBeGreaterThan(200);
		expect(
			slice,
			`${route}'s scope select no longer disclaims §8.1's chip, so a reader can mistake ` +
				'this subset for the control that is still unbuilt'
		).toContain('§8.1');
	});

	/*
	 * ⚠️ THE WORD ITSELF IS NOT BANNED, AND MUST NOT BE. Both screens carry prose
	 * that DISCLAIMS the chip — that prose is required by the guard above and by
	 * the one on the scope line, which has said "this is not §8.1's ScopeChip"
	 * since before this control existed. What must not appear is a chip-named
	 * BINDING: a `const scopeChip` or an `onChip` handler would make this subset
	 * grep as the chip and read as having discharged it.
	 */
	it.each(SCREENS)('%s names no binding after the chip', (route, source) => {
		expect(source, `${route} was not read at all`).toContain('scopeSelectOptions');
		expect(
			source,
			`${route} declares a chip-named binding, which is the confusion this control is ` +
				'required to avoid'
		).not.toMatch(/(?:const|let|var|function)\s+\w*[Cc]hip\w*/);
	});

	/*
	 * ⚠️ DELETED, NEVER EMPTIED, ALL THE WAY TO THE `pushState`. The handler must
	 * cope with a search string that is now empty — clearing the only parameter
	 * on the address — rather than pushing a bare `?`.
	 */
	it.each(SCREENS)('%s pushes a path when the address empties out', (route, source) => {
		const open = source.indexOf('function onScope');
		expect(open, `${route} no longer has a scope handler`).toBeGreaterThanOrEqual(0);
		const body = source.slice(open, source.indexOf('\n\t}', open));
		expect(
			body,
			`${route}'s handler does not build the address through $lib/scopeselect`
		).toContain('scopeSelectSearch(');
		expect(body, `${route}'s handler does not push the address`).toContain('pushState(');
		expect(body, `${route} would push a bare "?" once the last parameter is cleared`).toContain(
			"search === '' ? page.url.pathname"
		);
	});

	/*
	 * ⚠️ THE LIBRARIES READ IS NO LONGER GATED ON THE ADDRESS CARRYING A SCOPE.
	 * It used to be, correctly, while the names only enriched a scope line the
	 * unscoped view never renders. The unscoped view is where switching INTO a
	 * library starts, and a control cannot offer libraries nobody read.
	 */
	it.each(SCREENS)('%s reads the libraries on the unscoped view too', (route, source) => {
		expect(source, `${route} no longer reads the library names at all`).toContain(
			'readLibraryNames()'
		);
		expect(
			source,
			`${route} asks for the libraries only when the address already carries a scope, so ` +
				'the unscoped view offers no way into one'
		).not.toContain('(query?.libraries.length ?? 0) === 0 || namesAsked');
	});
});
