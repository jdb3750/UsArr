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
	/**
	 * WHETHER THIS COLUMN'S HEADER IS A SORT CONTROL.
	 *
	 * Default false, and that default is what keeps the primitive's other
	 * consumers byte-identical: a column that does not opt in renders exactly the
	 * plain-text `<th>` it always did — no button, no glyph, and no `aria-sort`.
	 * The Services screen declares none and is unchanged by this field existing.
	 *
	 * Opting in also requires the list to be given `sortKey`, `sortDir` and
	 * `onsort`; without a handler there is nothing to activate, so the header
	 * stays plain rather than rendering a dead button. `$lib/sortspec` owns the
	 * comparator, the direction toggle and the URL half — the primitive decides
	 * nothing about what a sort MEANS, which is what lets ADR-0038's freeze rule
	 * stay on the screen where the irreversible action is.
	 *
	 * DESIGN-DIRECTION §9.1a names "a column header, the toolbar's sort, or the
	 * control in the next rule" as the three explicit sort controls, so this is a
	 * specified affordance rather than an added one.
	 */
	sortable?: boolean;
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
 * ⚠️ NAME THE BOX OR THE FIGURE IS UNUSABLE. ADR-0029's point (c) — that
 * `contain-intrinsic-size` sizes the CONTENT box, so padding and border are
 * added on top — applies here. What does not apply is its padding term: the
 * padding lives on the `<td>`, so `.tbl tbody tr`'s own computed padding is
 * 0 px, and the row carries a 1 px bottom border. The arithmetic is written out
 * rather than described, because it has been got wrong twice by reading a
 * border-box figure into a content-box property:
 *
 *     border box, as rendered   = --row-h         = 28 / 32 / 36
 *     content box, as rendered  = --row-h − 1 px  = 27 / 31 / 35  ← belongs here
 *
 * ⚠️ AND NAMING THE BOX IS STILL NOT ENOUGH FOR THE MIDDLE FIGURE, SO THE FLOOR
 * CONDITION IS NAMED WITH IT. `27 / 31 / 35` is the rendered content box WITH
 * THE FLOOR LIVE and it is also the NATURAL BORDER box with the floor forced
 * off — same digits, two quantities, and no box label separates them. The value
 * below is the first: rendered content box, floor binding. (REVIEW-LOG RH-01.)
 *
 * MEASURED ON BOTH STACK FORKS, at 1440×900, one-line rows of the real
 * component, 500 rows sampled per cell, every row forced to lay out so no
 * placeholder height is in the sample. "natural" is the same row with
 * `min-height` forced to 0:
 *
 *     fork        border, rendered  content, rendered  content, natural  floor
 *     'two-line'  28 / 32 / 36      27 / 31 / 35       26 / 30 / 34      binds
 *     'labels'    28 / 32 / 36      27 / 31 / 35       26 / 30 / 34      binds
 *
 * The two forks agree to the pixel, which is why this is one constant and not
 * one per fork, and zero spread within each cell. The floor BINDS on both:
 * `min-height: var(--row-h)` is what sets these heights, not the content, and
 * `scripts/list-bench.mjs` re-establishes that on every run by perturbing it
 * rather than leaving the claim here where nothing can check it.
 *
 * ⚠️ AND PERTURB IT ON THE RIGHT ELEMENT. `List.svelte` stamps `data-density`
 * on the LIST CONTAINER and app.css re-declares the density tokens at that
 * scope, so a `--row-h` override set on `<html>` is overridden by the container
 * and nothing moves — a null from a probe that could not fire. Override on the
 * list, or perturb `min-height` on the row with `!important`, which is what the
 * numbers above did: forcing `min-height: 0` moved every row, on both forks at
 * all three densities, so the probe demonstrably fired.
 *
 * ⚠️ THIS USED TO READ 28 / 32 / 36 AND THE SIX DIGITS DID NOT CHANGE MEANING
 * BY ACCIDENT. Before the `.stacksep` margin exemption landed, the stray 2 px
 * pushed the `two-line` fork's natural height clear of the floor and its
 * CONTENT box really was 28 / 32 / 36 (border box 29 / 33 / 37, floor inert) —
 * re-measured here by restoring that margin, not taken on report. The `labels`
 * fork never renders a `.stacksep`, never had the bug, and was 27 / 31 / 35
 * content box throughout, so the constant was already wrong for one of the two
 * forks. The same digits therefore name different boxes on either side of that
 * commit, which is how a stale number survives being re-read and re-approved.
 *
 * ⚠️ AND NOTHING WOULD HAVE FAILED IF IT HAD BEEN LEFT WRONG. `auto` in front
 * of the length means the browser discards this estimate for the row's own size
 * the moment the row has been laid out once, so 1 px too tall per row buys a
 * little scrollbar drift on rows that have never been on screen and trips no
 * assertion anywhere. The only instrument that catches it is a measurement,
 * which is why the bench prints the recommendation this constant must match.
 *
 * 🔍 The bench serves the harness from its own Vite root with no `publicDir`,
 * so IBM Plex 404s there and every figure above is nominally on the fallback
 * face. Re-measured with the real `web/static/fonts` served, the six numbers
 * come back byte-identical, and the control has teeth — the same probe shows
 * the cell's advance width moving 169.87 px → 153 px between the two
 * conditions. `line-height` on the cell is a fixed 18 px length rather than a
 * unitless multiple, and the floor binds anyway, so the face cannot move it.
 *
 * A list whose rows are taller than one line passes its own measured value in
 * `rowIntrinsic`; a constant would be wrong by ~50% on a design where a
 * services row is six lines and a search row is one. Measured on the release
 * row the harness renders — chips, a button, a checkbox and a `<select>` — that
 * row is NOT one height: two distinct content boxes, 44 px ×1308 and 48 px ×692
 * at compact, so mode 44 / 48 / 52 and mean 45.4 / 49.4 / 53.4, floor SLACK
 * (the content sets these, not `--row-h`).
 *
 * 🚩 WHICH MAKES 45 / 49 / 53 ANOTHER SAME-DIGITS-TWO-QUANTITIES TRAP: it is
 * both the MEAN CONTENT box and the MODAL BORDER box of that row. Anything
 * carrying it — `RELEASE_ROW_INTRINSIC` in `requests.ts` — wants the content
 * reading, and is right by the mean rather than by the mode. Do not "correct"
 * it to the other reading without re-measuring.
 */
export const ROW_INTRINSIC: Record<string, number> = {
	compact: 27,
	standard: 31,
	relaxed: 35
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
