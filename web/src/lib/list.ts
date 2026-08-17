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
 * The value is a single CSS grid track: `minmax(0, 2fr)`, `12ch`, `198px`.
 *
 * ⚠️ IT MUST NOT BE CONTENT-SIZED. `auto`, `max-content`, `min-content`,
 * `fit-content()`, and any `minmax()` built out of them are all forbidden here,
 * and `findContentSizedTracks` below says why and enforces it in dev. This
 * comment used to recommend `minmax(max-content, auto)` for an action column,
 * citing §9.1's overflow policy, and that recommendation shipped the bug the
 * guard now closes. §9.1's policy is honoured by letting the CELL wrap
 * (`.tbl .cell-actions { flex-wrap: wrap }`), not by an unbounded track.
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
	if (import.meta.env.DEV) warnContentSized(columns);
	if (columns.length === 0) return 'minmax(0, 1fr)';
	return columns.map((c) => c.width).join(' ');
}

/**
 * The three CSS-wide keywords that size a track to its own grid's content.
 *
 * Matched case-insensitively because CSS keywords are ASCII case-insensitive,
 * so `MAX-CONTENT` is the same declaration and the same bug.
 */
const CONTENT_SIZED_KEYWORD = /^(?:auto|max-content|min-content)$/i;

/** `name(args)` as one whole token — `minmax(0, 1fr)`, `fit-content(200px)`. */
const TRACK_FUNCTION = /^([a-zA-Z-]+)\((.*)\)$/s;

/**
 * Split on a top-level separator, ignoring anything inside parentheses.
 *
 * The nesting is the whole reason this is not `String.split`: the comma in
 * `minmax(fit-content(200px), 1fr)` that matters is the outer one, and the
 * whitespace in `minmax( max-content , auto )` is not a track boundary.
 */
function splitTopLevel(input: string, on: 'comma' | 'space'): string[] {
	const parts: string[] = [];
	let depth = 0;
	let start = 0;
	for (let i = 0; i < input.length; i++) {
		const ch = input[i];
		if (ch === '(') depth++;
		else if (ch === ')') depth--;
		else if (depth === 0 && (on === 'comma' ? ch === ',' : /\s/.test(ch))) {
			parts.push(input.slice(start, i));
			start = i + 1;
		}
	}
	parts.push(input.slice(start));
	return parts.map((p) => p.trim()).filter((p) => p !== '');
}

/**
 * Whether one grid track sizes against the content of its own grid.
 *
 * ⚠️ THIS IS THE PREDICATE THE WHOLE GUARD RESTS ON, SO ITS SCOPE IS EXACT.
 * Content-sized: the bare keywords `auto` / `max-content` / `min-content`, any
 * `fit-content()`, and any `minmax()` with a content-sized argument in EITHER
 * position. Not content-sized: lengths (`80px`, `10ch`, `4rem`), percentages,
 * `<flex>` (`1fr`, `2.4fr`), `calc()`, and the `minmax(0, 1fr)` /
 * `minmax(80px, 1fr)` forms every well-behaved column in this repo uses.
 *
 * ⚠️ EITHER POSITION, NOT JUST THE FLOOR, AND THAT IS DELIBERATE. CSS Grid
 * §7.2.3 says "auto as a maximum is identical to max-content", so
 * `minmax(80px, auto)` is every bit as row-local as `minmax(max-content, 1fr)`
 * — it just fails less dramatically because the floor holds the short rows
 * together. Checking only the floor would let the same bug back in through the
 * second argument. No legitimate track in this repo has a content-sized
 * maximum, so the wider rule costs nothing.
 */
export function isContentSized(track: string): boolean {
	const t = track.trim();
	if (t === '') return false;

	// A `width` is documented as ONE track, but nothing enforces that, and
	// `'minmax(0, 1fr) auto'` in a single field must not slip through on a
	// technicality. Recurse into whitespace-separated tracks first.
	const tracks = splitTopLevel(t, 'space');
	if (tracks.length > 1) return tracks.some(isContentSized);

	if (CONTENT_SIZED_KEYWORD.test(t)) return true;

	const fn = TRACK_FUNCTION.exec(t);
	if (!fn) return false;
	const name = fn[1].toLowerCase();
	// `fit-content(<length-percentage>)` is `min(max-content, max(auto, arg))`:
	// content-sized by construction whatever the argument is.
	if (name === 'fit-content') return true;
	if (name === 'minmax') return splitTopLevel(fn[2], 'comma').some(isContentSized);
	return false;
}

/**
 * THE TRACKS IN `widths` THAT CANNOT ALIGN ACROSS ROWS, IN INPUT ORDER.
 *
 * ⚠️ THE BUG CLASS THIS CLOSES, BECAUSE IT IS INVISIBLE OTHERWISE. ADR-0029
 * makes every row of a list an INDEPENDENT `display: grid`; header and body
 * rows line up only because both resolve the same `--cols`. A content-sized
 * track therefore has nothing to agree with — it sizes against the contents of
 * its own row, so the header row sizes to the column NAME and each body row
 * sizes to whatever that row happens to hold. There is no grid algorithm in
 * play that could reconcile them, which is why this is a rule and not a
 * preference.
 *
 * It shipped once. The Requests release table declared Actions as
 * `minmax(max-content, auto)`; measured in Chromium at 1440 px that track came
 * out 61 px in the header and 155.02 px in a body row, and the 94.02 px
 * difference was handed to the header row's `fr` tracks alone, drifting every
 * column right of Indexer 37–94 px off its own header. Rows sheared against
 * EACH OTHER too — a release with no `info_url` renders no "Indexer page" link,
 * and its track measured 63 px against its siblings' 155.02 px. Fixed on `main`
 * at 9bcb547 by giving the column a measured 198 px reserve.
 *
 * Nothing failed. It rendered, it just misaligned — no exception, no failing
 * assertion, no visual that reads as broken rather than as slightly ugly. A
 * guard is the only instrument that catches it, so `gridTemplate` calls this on
 * every dev render and `console.error`s the offending column ids.
 *
 * PURE, AND EXPORTED FOR ITS OWN SAKE. It takes strings rather than columns so
 * `list.test.ts` can fire it at every legitimate form as well as every
 * offending one — a validator that flags everything passes a suite that only
 * feeds it bad input.
 */
export function findContentSizedTracks(widths: readonly string[]): string[] {
	return widths.filter(isContentSized);
}

/**
 * The dev-only half: name the columns, say why, and DO NOT THROW.
 *
 * Throwing would take down a screen over a cosmetic defect, and a guard that
 * can break the app is a guard people delete. `import.meta.env.DEV` is a
 * literal `false` in a production build, so Rollup drops this call and the
 * function with it — verified by grepping the built bundle for the message.
 */
function warnContentSized(columns: readonly ListColumn[]): void {
	// Through `findContentSizedTracks` rather than straight to the predicate, so
	// the exported entry point is the one the running app depends on. A
	// validator only the tests call is a validator that can rot without failing
	// anything. The second pass is what maps a width back to the column id the
	// message has to name; it is two passes over at most a dozen columns, in dev.
	if (findContentSizedTracks(columns.map((c) => c.width)).length === 0) return;
	const named = columns
		.filter((c) => isContentSized(c.width))
		.map((c) => `${c.id} (width: ${c.width})`)
		.join(', ');
	console.error(
		`[UsArr list] content-sized column track(s): ${named}. ` +
			'Every list row is an independent grid (ADR-0029), so a track sized by ' +
			'`auto`, `max-content`, `min-content`, `fit-content()` or a `minmax()` ' +
			'built from them resolves against its OWN row and cannot line up with ' +
			'the header row or with the other body rows. Declare a fixed reserve ' +
			'(measure the widest state) or an `fr` track with a 0 floor, and let ' +
			'the cell wrap if its contents can outgrow the reserve.'
	);
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
