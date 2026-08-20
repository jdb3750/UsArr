/*
 * THE SETTINGS TOGGLE ACTUALLY REACHES THE SHELL'S `/`.
 *
 * The defect this guards against is not a crash and not a wrong pixel: it is a
 * Settings control that reported a state it did not have. `prefs.shortcuts`
 * existed, `List.svelte` passed it to `roving.ts` as `letterKeys`, and the
 * shell's `/` never read it — so turning single-key shortcuts OFF left `/`
 * navigating. DESIGN-DIRECTION's grep rule is explicit: no single-character
 * keyboard shortcut without the Settings toggle that turns all of them off
 * (SC 2.1.4, Level A).
 *
 * The decision is pure (`opensSearch`) so it can be tested directly. The
 * WIRING is not, and a green `opensSearch` says nothing about whether the
 * layout passes the real preference into it — `vitest.config.ts` is
 * `environment: 'node'` with no Svelte plugin, so the component cannot be
 * compiled, and the repo's answer is to read the source through Vite's `?raw`
 * and assert over the text. Both halves are below because neither subsumes the
 * other.
 *
 * ⚠️ EVERY SLICE PROVES IT FOUND SOMETHING BEFORE IT ASSERTS AN ABSENCE.
 * `expect('').not.toContain(anything)` passes, so a slice that silently stops
 * matching turns a rule into a rumour that reports green for ever. This repo
 * has shipped a vacuous guard before.
 */

import { describe, expect, it } from 'vitest';
import { opensSearch, type SearchKeyContext, type SearchKeyEvent } from './shellkeys';
import LAYOUT_SOURCE from '../routes/+layout.svelte?raw';
import SETTINGS_SOURCE from '../routes/settings/+page.svelte?raw';

const LAYOUT = 'routes/+layout.svelte';
const SETTINGS = 'routes/settings/+page.svelte';

/** Comments blanked, line count preserved: a rule must never fire on its own
 * documentation, and the paragraphs in both files name every symbol below. */
function stripComments(text: string): string {
	const blank = (m: string) => m.replace(/[^\n]/g, ' ');
	return text
		.replace(/<!--[\s\S]*?-->/g, blank)
		.replace(/\/\*[\s\S]*?\*\//g, blank)
		.replace(/(^|[^:'"\\])\/\/[^\n]*/g, (m, p: string) => p + blank(m.slice(p.length)));
}

const LAYOUT_CODE = stripComments(LAYOUT_SOURCE);
const SETTINGS_CODE = stripComments(SETTINGS_SOURCE);

/**
 * The source from `open` up to `close`, with the offsets asserted rather than
 * trusted. `indexOf` returning -1 feeds `slice` a NEGATIVE index, which yields
 * the tail of the file rather than nothing — a slice that quietly stops meaning
 * what it says instead of failing loudly.
 */
function between(source: string, where: string, open: string, close: string): string {
	const from = source.indexOf(open);
	expect(from, `${where} no longer contains ${open}`).toBeGreaterThanOrEqual(0);
	const to = source.indexOf(close, from);
	expect(to, `${where} has no ${close} after ${open}`).toBeGreaterThan(from);
	return source.slice(from, to);
}

function ev(over: Partial<SearchKeyEvent> = {}): SearchKeyEvent {
	return {
		key: '/',
		ctrlKey: false,
		altKey: false,
		metaKey: false,
		repeat: false,
		defaultPrevented: false,
		isComposing: false,
		...over
	};
}

/** Shortcuts on, nav showing, not already on Search — the case that navigates. */
function ctx(over: Partial<SearchKeyContext> = {}): SearchKeyContext {
	return { shortcuts: true, showNav: true, onSearchScreen: false, ...over };
}

describe('the Settings toggle governs the shell', () => {
	it('opens Search on `/` while shortcuts are on', () => {
		expect(opensSearch(ev(), ctx())).toBe(true);
	});

	it('declines `/` once shortcuts are off', () => {
		const event = ev();
		// The positive half FIRST: this exact event is the one that navigates, so
		// the refusal below is a refusal of something and not of a typo.
		expect(opensSearch(event, ctx({ shortcuts: true })), 'the `/` case stopped firing').toBe(true);
		expect(opensSearch(event, ctx({ shortcuts: false }))).toBe(false);
	});

	/*
	 * ⚠️ SHIFT IS NOT A REFUSAL. On the German and French layouts `/` is a
	 * shifted key, so the shortcut arrives with `shiftKey` true; excluding it
	 * would delete the shortcut on those keyboards. `shiftKey` is not even in
	 * `SearchKeyEvent`, and this is the guard that says so on purpose.
	 */
	it('still opens Search when `/` was typed with Shift', () => {
		expect(opensSearch({ ...ev(), ...{ shiftKey: true } }, ctx())).toBe(true);
	});

	it.each([
		['a repeat, so holding the key is one navigation', { repeat: true }],
		['a key something ahead already claimed', { defaultPrevented: true }],
		['Ctrl+/, which means something to somebody', { ctrlKey: true }],
		['Alt+/', { altKey: true }],
		['Meta+/', { metaKey: true }],
		['a composition in progress, where the key is the IME\u2019s', { isComposing: true }]
	])('refuses %s', (_why, over) => {
		expect(opensSearch(ev(), ctx()), 'the `/` case stopped firing').toBe(true);
		expect(opensSearch(ev(over), ctx())).toBe(false);
	});

	it.each([
		['the nav that reaches Search is not rendered', { showNav: false }],
		['Search is already the screen', { onSearchScreen: true }]
	])('refuses `/` when %s', (_why, over) => {
		expect(opensSearch(ev(), ctx()), 'the `/` case stopped firing').toBe(true);
		expect(opensSearch(ev(), ctx(over))).toBe(false);
	});

	/*
	 * ⚠️ `l` IS DELIBERATELY ABSENT. DESIGN-DIRECTION specifies `l` for the scope
	 * popover, and §8.1's scope chip is unbuilt — the key would open nothing.
	 * Shipping it would repeat the defect this file guards rather than fix it.
	 */
	it.each(['l', 'j', 'k', '?', 'Escape'])('is not a handler for %o', (key) => {
		expect(opensSearch(ev(), ctx()), 'the `/` case stopped firing').toBe(true);
		expect(opensSearch(ev({ key }), ctx())).toBe(false);
	});
});

describe('the shell is wired to the preference, not just capable of reading one', () => {
	it('imports prefs by name rather than for its side effect alone', () => {
		expect(LAYOUT_CODE, `${LAYOUT} no longer imports $lib/prefs.svelte at all`).toContain(
			'$lib/prefs.svelte'
		);
		expect(LAYOUT_CODE).toContain("import { prefs } from '$lib/prefs.svelte';");
		// The bare form is the defect itself: the module ran, the value was never
		// read, and the toggle did nothing.
		expect(LAYOUT_CODE).not.toContain("import '$lib/prefs.svelte';");
	});

	it('passes the live preference into the decision', () => {
		const call = between(LAYOUT_CODE, LAYOUT, 'opensSearch(', '});');
		expect(call, `${LAYOUT}'s opensSearch call is not an object literal any more`).toContain(
			'showNav'
		);
		expect(call).toContain('shortcuts: prefs.shortcuts');
	});

	it('leaves the shell no second, ungated `/` branch', () => {
		const handler = between(LAYOUT_CODE, LAYOUT, 'function onWindowKeydown', '</script>');
		expect(handler, `${LAYOUT} no longer defines onWindowKeydown`).toContain('openSearchKey');
		// Escape is a named key and no scroll key is a character, so `/` is the
		// shell's only SC 2.1.4 shortcut; any other literal here is a new one.
		expect(handler).not.toContain("'/'");
	});

	it('is fed by a Settings control that writes the same preference', () => {
		const field = between(SETTINGS_CODE, SETTINGS, 'id="pref-shortcuts"', '</label>');
		expect(field, `${SETTINGS} no longer renders the shortcuts checkbox`).toContain('checked=');
		expect(field).toContain('{prefs.shortcuts}');
		expect(SETTINGS_CODE, `${SETTINGS} no longer writes the preference`).toContain(
			'prefs.setShortcuts('
		);
	});

	/*
	 * The control is named "Single-key shortcuts" and `/` is a key, not a letter.
	 * The help text used to say "single-letter", which read as a narrower promise
	 * than its own label made — and than the shell now keeps.
	 */
	it('describes the set in the same words its label uses', () => {
		expect(SETTINGS_SOURCE, `${SETTINGS} no longer labels the toggle`).toContain(
			'Single-key shortcuts'
		);
		expect(SETTINGS_SOURCE).toContain('single-key shortcuts as a set');
		expect(SETTINGS_SOURCE).not.toContain('single-letter');
	});
});
