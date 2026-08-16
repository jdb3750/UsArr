/**
 * The list primitive's types and its pure helpers.
 *
 * Everything in this file is deliberately DOM-free so it can be tested by the
 * node-environment vitest run. The DOM half lives in `roving.ts` (an action)
 * and `List.svelte` (the markup), and is exercised by the Playwright harness in
 * `scripts/list-bench.mjs`, because `vitest.config.ts` is `environment: 'node'`
 * with no Svelte plugin and therefore cannot import a rune component at all.
 */

/** Which way a column's values are read (DESIGN-DIRECTION §9.1). */
export type ColumnAlign = 'start' | 'end';

/**
 * One column of a list.
 *
 * `width` IS THE POINT OF THIS TYPE, and it is required rather than optional.
 * §9.1: "Declare column widths per group rather than letting auto layout derive
 * them. `table-layout: auto` must measure every cell in every row to compute
 * column widths — the one layout mode that is inherently O(all rows) and that
 * no containment can help." The measured saving was 1,199 ms → 547 ms at 5,000
 * rows. Making the field optional would let a caller opt back into the cost by
 * omission, which is the failure mode a required field forecloses.
 *
 * The value is a single CSS grid track: `minmax(0, 2fr)`, `12ch`,
 * `minmax(max-content, auto)` for an action column (§9.1's overflow policy —
 * a fixed track shears the buttons attached to the rows that are broken).
 */
export interface ListColumn {
	/** Stable id. Also written to `data-col`, which the stacked fork keys on. */
	id: string;
	/** The `columnheader`'s text. It survives the stacked view as the label. */
	header: string;
	/** One CSS grid track. Required; see the type comment. */
	width: string;
	/**
	 * `end` right-aligns the cell AND its header, and turns on `tabular-nums`
	 * at the cell so composite values inherit it. §9.1: "Header alignment
	 * matches its column's data alignment"; a left header over right numbers is
	 * a persistent low-grade scanning cost.
	 */
	align?: ColumnAlign;
	/**
	 * Whether to render the stacked label below 760 px. Default true. Set false
	 * where the value IS the row's identity and the label only restates it —
	 * §9.1's `.tbl--2up` case, where dropping the `Type` label is what buys the
	 * second line back on a 390 px phone.
	 *
	 * The label is a real `<span aria-hidden="true">`, never `::before`
	 * generated content; see List.svelte for the measured reason.
	 */
	stackLabel?: boolean;
	/**
	 * Where this column lands in the TWO-LINE phone fork, which is the right
	 * degradation for a results list (§9.1: a results list is scanned, so five
	 * labelled lines per result puts three results in an 844 px viewport).
	 * Ignored when the list stacks to label/value pairs. Default `hidden`: a
	 * two-line row shows the title and the two most identifying secondary
	 * fields, and everything else goes behind the row.
	 */
	stackLine?: 1 | 2 | 'hidden';
}

/**
 * THE THREE WORDS FOR "THERE IS NOTHING HERE", AND NO OTHERS (§9.1).
 *
 * Nine renderings shipped in the prototype — `None`, `No action needed`,
 * `Never`, `not applicable`, `none`, `n/a` twice, `no file` — with one screen
 * rendering the same fact two different ways in adjacent columns. Three
 * concepts are in play and the vocabulary has to separate them, so they are
 * exported as constants rather than left as string literals at each call site.
 */
export const NOTHING = {
	/** The value is genuinely empty and that is unremarkable. */
	empty: '—',
	/** This exists as a concept and you have not set it up. */
	unconfigured: 'Not configured',
	/** This concept does not exist for this row. */
	inapplicable: 'Not applicable'
} as const;

export type NothingKind = keyof typeof NOTHING;

/**
 * The `--cols` value for a column set: one grid track per column, in order.
 *
 * Set through `element.style.setProperty()` and never through a `style`
 * attribute. The server sends `style-src 'self'` with no `'unsafe-inline'`
 * (internal/httpapi/middleware.go), and an inline style ATTRIBUTE is refused
 * under that header — the attribute stays in the DOM and applies nothing, which
 * is invisible in the source and invisible in a screenshot. A custom property
 * written through the CSSOM is not covered by the directive.
 */
export function gridTemplate(columns: readonly ListColumn[]): string {
	if (columns.length === 0) return 'minmax(0, 1fr)';
	return columns.map((c) => c.width).join(' ');
}

/**
 * `aria-rowindex` for the nth rendered data row.
 *
 * 1-BASED OVER THE FULL RESULT SET, NOT OVER THE RENDERED WINDOW (§11). The
 * header row is index 1, so the first data row is 2. `offset` is how many rows
 * of the full set precede the rendered window; for "Load more" it is 0 by
 * construction, because the DOM holds a prefix.
 */
export function rowIndex(n: number, offset = 0): number {
	return offset + n + 2;
}

/**
 * `aria-rowcount`: the full set including the header row.
 *
 * ARIA defines -1 for "the total is genuinely unknown", and that is the honest
 * answer while a keyset page is in flight and the server has not said how many
 * there are. Returning the rendered count instead is what makes a screen reader
 * say "row 3 of 26" when the truth is "row 3 of 1,204" — a confidently wrong
 * number arriving through the accessibility tree.
 */
export function rowCount(total: number | undefined): number {
	if (total === undefined || total < 0 || !Number.isFinite(total)) return -1;
	return total + 1;
}

/**
 * A cell that renders one chip per related object caps at three plus `+N more`
 * (§9.1). The live case is one Audiobookshelf feeding fifteen libraries, which
 * makes that cell the tallest thing on the Services screen.
 */
export function capChips<C>(items: readonly C[], max = 3): { shown: C[]; more: number } {
	if (items.length <= max) return { shown: [...items], more: 0 };
	return { shown: items.slice(0, max), more: items.length - max };
}

/**
 * The measured content-box height, in CSS pixels, that
 * `contain-intrinsic-size: auto <n>px` should use for a ONE-LINE row at each
 * density.
 *
 * MEASURED, NOT ASSUMED. `scripts/list-bench.mjs` reads
 * `getBoundingClientRect().height` off 2,000 rendered one-line rows of the real
 * primitive at each density and subtracts the computed padding and border to
 * get the content box. `auto` in front of the length means the browser replaces
 * this estimate with the row's own last-rendered size once it has seen it, so
 * the value only has to be right for rows that have never been on screen.
 *
 * ⚠️ ONE OF ADR-0029'S THREE CORRECTIONS DOES NOT APPLY TO THIS PRIMITIVE, AND
 * THE MEASUREMENT SAYS SO. The ADR's point (c) is that `contain-intrinsic-size`
 * sizes the content box "so padding and border are added on top" — a 24 px row
 * with `auto 28px` produced a 37 px placeholder. That was measured on a row
 * whose padding was on the row. HERE THE ROW HAS NO PADDING OF ITS OWN: the
 * padding lives on the `<td>`, and `.tbl tbody tr` carries only a 1 px bottom
 * border. Measured, at all three densities: computed row padding 0 px, border
 * 1 px, and a one-line row's content box comes out at EXACTLY `--row-h` —
 * 28 / 32 / 36. So the placeholder border box is `--row-h + 1`, which is what a
 * real one-line row measures, and the ADR's arithmetic would have over-counted.
 * Points (a) and (b) stand unchanged.
 *
 * A list whose rows are taller than one line passes its own measured value in
 * `rowIntrinsic`; a constant would be wrong by ~50% on a design where a
 * services row is six lines and a search row is one. Measured on the release
 * row the harness renders — chips, a button, a checkbox and a `<select>` — the
 * same statistic is 45 / 49 / 53 px, i.e. 1.6× this default.
 */
export const ROW_INTRINSIC: Record<string, number> = {
	compact: 28,
	standard: 32,
	relaxed: 36
};

/** The states the primitive itself owns (DESIGN-DIRECTION §10). */
export type ListState = 'default' | 'loading' | 'empty' | 'filtered-empty' | 'partial' | 'stale';

/**
 * THE DEFAULT "LOAD MORE" PAGE SIZE.
 *
 * ADR-0029 guessed 100–300, or 300–600 "with `table-layout: fixed` and a
 * working containment path", and said the real threshold is set by the density
 * control rather than by scrolling. The second half is confirmed; the numbers
 * are not, and they are low by an order of magnitude for the primitive as it
 * now stands.
 *
 * MEASURED, desktop x86 Chromium, `scripts/list-bench.mjs`, density toggle as
 * the mean of four changes, forcing style recalculation AND layout:
 *
 *   as shipped (containment live, declared columns, density scoped to the list)
 *       0.0146 ms/row + 6.4 ms fixed  →  100 ms at ~6,400 rows
 *   worst case (containment forced off, attribute on <html>)
 *       0.2136 ms/row − 6.7 ms fixed  →  100 ms at ~500 rows
 *
 * The worst-case line reproduces ADR-0029's own condition and lands on ~500
 * rows against its 523-row extrapolation, which is what makes the shipped line
 * believable rather than a different benchmark measuring a different thing.
 *
 * At ADR-0029's 3–5× for a Pi 5 that is **1,280–2,133 rows in the DOM as
 * shipped**, against 100–167 in the worst case. A 200-row page therefore leaves
 * six presses of "Load more" before the density control reaches the Tier-0 hard
 * fail even on the pessimistic Pi estimate, and it matches ARCHITECTURE §4.5's
 * existing "keyset windows of ~100, prefetching ±2 pages" without needing that
 * to change.
 *
 * 🔍 INFERENCE, marked as one: the Pi 5 figure is ADR-0029's multiplier applied
 * to a desktop measurement, not a measurement on a Pi. Nothing in this repo has
 * run on the reference hardware yet.
 */
export const LOAD_MORE_PAGE_SIZE = 200;
