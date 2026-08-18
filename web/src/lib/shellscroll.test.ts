/*
 * THE NARROWNESS IS THE FEATURE, so it is what gets asserted.
 *
 * `scrollDeltaFor` is the whole decision the shell's key forwarding makes, and
 * it is pure so that the decision can be tested without a browser. What a
 * browser is still needed for — that the amounts match the platform's, that
 * focus does not move, that the skip link is still the first tab stop, that a
 * back-navigation restores the region — is measured by `pnpm probe:shell`, and
 * neither check subsumes the other: this file proves the handler declines, the
 * probe proves the forwarding lands.
 */
import { describe, expect, it } from 'vitest';
import { scrollDeltaFor, type RegionMetrics } from './shellscroll';

/** A region 800 tall over 4000 of content, with room to move on both axes. */
const REGION: RegionMetrics = {
	scrollTop: 1000,
	scrollLeft: 100,
	clientHeight: 800,
	scrollHeight: 4000,
	clientWidth: 1200,
	scrollWidth: 2000
};

type Ev = Parameters<typeof scrollDeltaFor>[0];

function ev(key: string, mods: Partial<Ev> = {}): Ev {
	return {
		key,
		shiftKey: false,
		ctrlKey: false,
		altKey: false,
		metaKey: false,
		defaultPrevented: false,
		...mods
	};
}

describe('scrollDeltaFor — the amounts the platform uses', () => {
	/* Chromium 141, measured by scripts/shell-geometry-probe.mjs against a
	 * document that really does scroll: an arrow is 40px on both axes at every
	 * viewport height probed, and a page key is trunc(0.875 * clientHeight) —
	 * NOT clientHeight - 40, which disagreed at all six heights. */
	it('scrolls one line per arrow key, in the right direction, on both axes', () => {
		expect(scrollDeltaFor(ev('ArrowDown'), REGION)).toEqual({ top: 40, left: 0 });
		expect(scrollDeltaFor(ev('ArrowUp'), REGION)).toEqual({ top: -40, left: 0 });
		expect(scrollDeltaFor(ev('ArrowRight'), REGION)).toEqual({ top: 0, left: 40 });
		expect(scrollDeltaFor(ev('ArrowLeft'), REGION)).toEqual({ top: 0, left: -40 });
	});

	it('scrolls trunc(0.875 * clientHeight) per page key', () => {
		expect(scrollDeltaFor(ev('PageDown'), REGION)).toEqual({ top: 700, left: 0 });
		expect(scrollDeltaFor(ev('PageUp'), REGION)).toEqual({ top: -700, left: 0 });
		// The truncation is the measured behaviour, not a rounding preference:
		// a 300px scrollport stepped by 262, and 300 * 0.875 is 262.5.
		expect(scrollDeltaFor(ev('PageDown'), { ...REGION, clientHeight: 300 })).toEqual({
			top: 262,
			left: 0
		});
		expect(scrollDeltaFor(ev('PageDown'), { ...REGION, clientHeight: 500 })).toEqual({
			top: 437,
			left: 0
		});
	});

	it('treats Space as a page key and Shift+Space as its opposite', () => {
		expect(scrollDeltaFor(ev(' '), REGION)).toEqual({ top: 700, left: 0 });
		expect(scrollDeltaFor(ev(' ', { shiftKey: true }), REGION)).toEqual({ top: -700, left: 0 });
	});

	it('sends Home and End to the ends, as deltas from where the region is', () => {
		expect(scrollDeltaFor(ev('Home'), REGION)).toEqual({ top: -1000, left: -100 });
		// 4000 - 800 = 3200 of travel, less the 1000 already scrolled.
		expect(scrollDeltaFor(ev('End'), REGION)).toEqual({ top: 2200, left: 700 });
	});

	it('asks for nothing when there is nowhere to go', () => {
		const flat = { ...REGION, scrollTop: 0, scrollLeft: 0, scrollHeight: 800, scrollWidth: 1200 };
		expect(scrollDeltaFor(ev('Home'), flat)).toEqual({ top: 0, left: 0 });
		expect(scrollDeltaFor(ev('End'), flat)).toEqual({ top: 0, left: 0 });
	});
});

describe('scrollDeltaFor — what it declines', () => {
	it('never touches a Ctrl, Alt or Meta combination', () => {
		for (const mod of ['ctrlKey', 'altKey', 'metaKey'] as const) {
			for (const key of ['PageDown', 'PageUp', 'Home', 'End', 'ArrowDown', ' ']) {
				expect(scrollDeltaFor(ev(key, { [mod]: true }), REGION)).toBeNull();
			}
		}
	});

	it('leaves Shift alone on every key but Space, where it means "back a page"', () => {
		for (const key of ['PageDown', 'Home', 'End', 'ArrowDown', 'ArrowUp']) {
			expect(scrollDeltaFor(ev(key, { shiftKey: true }), REGION)).toBeNull();
		}
		expect(scrollDeltaFor(ev(' ', { shiftKey: true }), REGION)).not.toBeNull();
	});

	it('stands down for a key something ahead of it already handled', () => {
		expect(scrollDeltaFor(ev('PageDown', { defaultPrevented: true }), REGION)).toBeNull();
	});

	it('stands down mid-composition, where the key belongs to the IME', () => {
		expect(scrollDeltaFor(ev('PageDown', { isComposing: true }), REGION)).toBeNull();
	});

	it('ignores every key that is not a scroll key', () => {
		for (const key of ['a', 'Enter', 'Tab', 'Escape', 'F5', 'Backspace', 'x']) {
			expect(scrollDeltaFor(ev(key), REGION)).toBeNull();
		}
	});
});
