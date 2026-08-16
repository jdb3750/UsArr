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

	it('is the measured one-line content box, which on this primitive IS --row-h', () => {
		// MEASURED, scripts/list-bench.mjs, 2,000 rendered one-line rows per
		// density: computed row padding 0px, row border 1px, content box exactly
		// 28 / 32 / 36 — one distinct height each, no spread at all.
		//
		// It equals --row-h because the padding lives on the <td> and not on the
		// row, so ADR-0029's "padding and border are added on top" correction —
		// measured on a row that DID carry its own padding — does not apply to
		// this primitive. Adding a padding term here would over-count and make
		// every placeholder too tall, which is the same drift in the other
		// direction. The test is written against the measurement so that a future
		// change putting padding back on the row fails here rather than silently
		// re-introducing the error.
		expect(ROW_INTRINSIC).toEqual({ compact: 28, standard: 32, relaxed: 36 });
	});
});
