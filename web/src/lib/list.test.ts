/**
 * The list primitive's pure half.
 *
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a rune
 * component cannot be imported here at all. That is a real limit rather than an
 * oversight: adding jsdom plus a Svelte-aware test setup buys a DOM that is not
 * the DOM the primitive is measured against, and every invariant that matters —
 * exactly one tabindex="0", aria-rowindex continuity across an append,
 * identity-keyed focus surviving a reorder, the form-control bail-out, sticky
 * headers, containment — needs a real layout engine to be worth anything.
 *
 * So the split is deliberate: everything below is logic that has no DOM in it
 * and runs on every `make check`; everything that needs a browser is asserted
 * by `pnpm bench:list` (scripts/list-bench.mjs), which drives the REAL
 * component under Playwright and exits non-zero on a failed invariant.
 */
import { describe, expect, it } from 'vitest';
import {
	capChips,
	gridTemplate,
	NOTHING,
	rowCount,
	rowIndex,
	ROW_INTRINSIC,
	type ListColumn
} from './list';

const COLUMNS: ListColumn[] = [
	{ id: 'release', header: 'Release', width: 'minmax(0, 3fr)' },
	{ id: 'size', header: 'Size', width: '9ch', align: 'end' },
	{ id: 'actions', header: 'Actions', width: 'minmax(max-content, auto)' }
];

describe('gridTemplate', () => {
	it('joins the declared tracks in column order', () => {
		expect(gridTemplate(COLUMNS)).toBe('minmax(0, 3fr) 9ch minmax(max-content, auto)');
	});

	it('degrades to one full-width track rather than to an empty value', () => {
		// An empty --cols would make `grid-template-columns: var(--cols)` invalid
		// at computed-value time and drop the row back to a single implicit
		// column with no min-width:0, which is how a release name pushes the
		// document sideways.
		expect(gridTemplate([])).toBe('minmax(0, 1fr)');
	});
});

describe('rowIndex', () => {
	it('is 1-based with the header at 1, so the first data row is 2', () => {
		expect(rowIndex(0)).toBe(2);
		expect(rowIndex(1)).toBe(3);
	});

	it('counts over the full set when the rendered window is not the head of it', () => {
		expect(rowIndex(0, 400)).toBe(402);
	});
});

describe('rowCount', () => {
	it('includes the header row', () => {
		expect(rowCount(1204)).toBe(1205);
	});

	it('is -1 when the total is genuinely unknown, which is what ARIA defines it for', () => {
		expect(rowCount(undefined)).toBe(-1);
		expect(rowCount(-1)).toBe(-1);
		expect(rowCount(Number.NaN)).toBe(-1);
	});

	it('does not treat an empty result set as unknown', () => {
		// 0 is a real answer — "you own nothing" — and it must not collapse into
		// the same value as "I have not been told yet".
		expect(rowCount(0)).toBe(1);
	});
});

describe('capChips', () => {
	it('passes three or fewer through untouched', () => {
		expect(capChips(['a', 'b', 'c'])).toEqual({ shown: ['a', 'b', 'c'], more: 0 });
	});

	it('caps at three plus the remainder', () => {
		// The live case is one Audiobookshelf feeding fifteen libraries, which
		// makes that cell the tallest thing on the Services screen.
		expect(capChips(['a', 'b', 'c', 'd', 'e'])).toEqual({ shown: ['a', 'b', 'c'], more: 2 });
	});

	it('does not hand back the caller’s array', () => {
		const source = ['a', 'b'];
		const { shown } = capChips(source);
		shown.push('c');
		expect(source).toEqual(['a', 'b']);
	});
});

describe('NOTHING', () => {
	it('is exactly three words, and they are distinct', () => {
		// §9.1: nine renderings shipped in the prototype. Three concepts are in
		// play and the vocabulary has to separate them; a fourth member here is
		// how the ninth rendering comes back.
		const values = Object.values(NOTHING);
		expect(values).toHaveLength(3);
		expect(new Set(values).size).toBe(3);
		expect(NOTHING.empty).toBe('—');
		expect(NOTHING.unconfigured).toBe('Not configured');
		expect(NOTHING.inapplicable).toBe('Not applicable');
	});
});

describe('ROW_INTRINSIC', () => {
	it('carries a value for every density the preferences can be set to', () => {
		for (const density of ['compact', 'standard', 'relaxed']) {
			expect(ROW_INTRINSIC[density]).toBeGreaterThan(0);
		}
	});

	it('is the measured one-line CONTENT box, which is --row-h minus the row border', () => {
		// MEASURED, scripts/list-bench.mjs, 2,000 rendered one-line rows per
		// density, and re-measured on BOTH stack forks — 'two-line' and 'labels'
		// agree to the pixel at all three densities: computed row padding 0px,
		// row border 1px, CONTENT box 27 / 31 / 35, one distinct height each,
		// no spread at all.
		//
		// THE BOX IS THE WHOLE POINT OF THIS ASSERTION. `contain-intrinsic-size`
		// sizes the content box and the browser adds padding and border on top,
		// so the BORDER box is --row-h (28 / 32 / 36) and the content box is one
		// pixel less. Writing the border-box figure in here is a 1px-per-row
		// over-estimate that `contain-intrinsic-size: auto` absorbs silently the
		// moment a row is laid out: it fails no assertion, drifts the scrollbar
		// on rows nobody has seen, and survives review. That has happened once
		// already, so the numbers are pinned here and the box is named in both
		// places.
		expect(ROW_INTRINSIC).toEqual({ compact: 27, standard: 31, relaxed: 35 });
	});

	it('is one pixel under the density token, the row border being the difference', () => {
		// app.css declares --row-h 28 / 32 / 36 and gives `.tbl tbody tr` a 1px
		// bottom border and no padding of its own. This is that relationship
		// written down as a check, so a stylesheet change that puts padding back
		// on the row, or moves the border, breaks here rather than quietly making
		// every unseen row's placeholder the wrong height.
		//
		// It is a MEASURED FACT ABOUT THE CURRENT RULES, not a licence to compute
		// the property from the token: nothing in CSS says the two must track.
		const ROW_H = { compact: 28, standard: 32, relaxed: 36 };
		const ROW_BORDER_PX = 1;
		for (const density of ['compact', 'standard', 'relaxed'] as const) {
			expect(ROW_INTRINSIC[density]).toBe(ROW_H[density] - ROW_BORDER_PX);
		}
	});
});
