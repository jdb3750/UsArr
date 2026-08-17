<script lang="ts" generics="T">
	/**
	 * THE LIST PRIMITIVE. Services, Requests and the library grid all sit on it.
	 *
	 * ADR-0029 names it rather than leaving it to whoever writes the first
	 * table, and the reason is a correctness one: `content-visibility: auto` is
	 * defined entirely in terms of size, layout and paint containment, and CSS
	 * Containment Level 2 excludes internal table boxes from all three. On a
	 * `display: table-row` element the declaration parses, reads back as
	 * `auto`, and does nothing — invisible in the source and invisible in a
	 * screenshot. So a row is a `display: grid` element carrying explicit ARIA.
	 *
	 * The ELEMENTS stay `<table>/<thead>/<tbody>/<tr>/<th>/<td>`, because the
	 * ported stylesheet is written against them and because the markup still
	 * reads as a table to anyone opening devtools. Their computed display is
	 * block/grid, which drops the implicit table semantics, so every one of them
	 * carries an explicit role. That is not belt-and-braces; without it the
	 * accessibility tree has a stack of generic containers.
	 *
	 * WHAT THIS COMPONENT OWNS, and what it deliberately does not:
	 *
	 *   owns   the grid + ARIA structure, aria-rowcount/aria-rowindex over the
	 *          FULL set, roving tabindex, "Load more", declared column widths,
	 *          the scoped density attribute, contain-intrinsic-size, the
	 *          responsive fork, and the six states of §10 that a list can be in
	 *          by itself (default, loading, empty, filtered-empty, partial,
	 *          stale).
	 *
	 *   not    what a cell contains. §9.1's cell rules — the three nothing-words,
	 *          the chip cap, ellipsis-plus-`title` on identity fields, no prose
	 *          in a cell — cannot be enforced by a component that is handed a
	 *          snippet. `$lib/list.ts` exports NOTHING and capChips() so the
	 *          call sites share one implementation, and `.trunc`, `.cell-scroll`
	 *          and `.chip` in app.css are the classes they use.
	 *
	 * THE CSP CONSTRAINT, because it is the one that bites silently. The server
	 * sends `style-src 'self'` with no `'unsafe-inline'`, so a `style` attribute
	 * is refused: it stays in the DOM and applies nothing. `--cols` and
	 * `--row-ci` are therefore written with `element.style.setProperty()`, which
	 * is CSSOM rather than an inline style attribute and is not covered by the
	 * directive. Verified in a real browser against the real header, not assumed
	 * — see the report in scripts/list-bench.mjs.
	 */
	import type { Snippet } from 'svelte';
	import { gridTemplate, rowCount, rowIndex, ROW_INTRINSIC } from '$lib/list';
	import type { ListColumn, ListState } from '$lib/list';
	import { prefs } from '$lib/prefs.svelte';
	import { roving } from '$lib/roving';

	interface Props {
		/** The `role="table"`'s accessible name. Required: an ARIA table with no
		 * name is a table a screen-reader user cannot tell apart from the next
		 * one on the same screen. */
		label: string;
		columns: ListColumn[];
		rows: T[];
		/**
		 * THE IDENTITY ACCESSOR, and the reason it is required.
		 *
		 * Focus, hover, selection and pending state all key to this rather than
		 * to a positional index. A list that reorders under a positional key
		 * silently teleports the keyboard user to a different row, and on a
		 * screen whose action is an irreversible grab that is the sharpest
		 * failure available. It is also what `{#each}` keys on, so a reorder
		 * moves DOM nodes rather than rewriting their contents.
		 */
		key: (row: T) => string;
		/** One cell's contents, per row per column. */
		cell: Snippet<[T, ListColumn]>;
		/**
		 * The size of the FULL result set, not of `rows`. Omit it — or pass a
		 * negative number — while the total is genuinely unknown, and
		 * `aria-rowcount` becomes -1, which is what ARIA defines for that case.
		 * Passing `rows.length` here would be the "row 3 of 26 when the truth is
		 * row 3 of 1,204" bug.
		 */
		total?: number;
		/** How many rows of the full set precede the rendered window. 0 for
		 * "Load more", which holds a prefix by construction. */
		offset?: number;
		state?: ListState;
		/** `empty`: you own nothing. One sentence, §9.6. */
		emptyTitle?: string;
		emptyText?: string;
		emptyActions?: Snippet;
		/** `filtered-empty`: a DIFFERENT message — the filter is responsible. */
		filteredEmptyTitle?: string;
		filteredEmptyText?: string;
		filteredEmptyActions?: Snippet;
		/** `partial`: what arrived, what did not. "4 of 9 indexers responded". */
		partialNote?: string;
		/** `stale`: the data is real but old, with the timestamp. Not greyed. */
		staleNote?: string;
		/** `loading`: only where it genuinely applies. A Tier-0 component has no
		 * loading state at all and inventing one is the failure (§10). */
		loadingNote?: string;
		hasMore?: boolean;
		loadingMore?: boolean;
		onloadmore?: () => void;
		/**
		 * How the list degrades below 760 px.
		 *   `labels`    label/value pairs — right for a record you read one at a
		 *               time (Services, Libraries).
		 *   `two-line`  title then two identifying fields joined by `·` — right
		 *               for a results list, which is scanned. Five labelled
		 *               lines per result puts three results in an 844 px
		 *               viewport.
		 */
		stack?: 'labels' | 'two-line';
		/**
		 * The measured content-box height of a one-line row, in CSS px, for
		 * `contain-intrinsic-size`. Defaults to the measured per-density value
		 * in `$lib/list.ts`; a list whose rows are taller passes its own. See
		 * ROW_INTRINSIC for why this is not `--row-h`.
		 */
		rowIntrinsic?: number;
		/** A declared reason, which turns the roving model off and says why. */
		rovingOptOut?: string;
		/** Enter or Space on a focused row. */
		onactivate?: (row: T) => void;
		/** Extra classes on the `<table>`, for a per-screen variant. */
		class?: string;
		/**
		 * THE ROW EXPANDER, which §17.3 makes a requirement rather than a nicety:
		 * the breaker state, the *Arr's own health warnings and the verbatim
		 * upstream text all live behind it, because §9.1's truncation policy is
		 * explicit that an explanation is not a cell value at all.
		 *
		 * It is rendered as a second `<tr>` beneath its row, spanning every
		 * column, and ONLY when `expanded(row)` is true — a collapsed list costs
		 * nothing in the DOM, which is what keeps "Load more" cheap on a screen
		 * whose rows are otherwise one line.
		 *
		 * The extra row carries `role="row"` and its own `aria-rowindex` because
		 * a `rowgroup`'s owned elements are rows and nothing else, so it cannot be
		 * a `region` however much better that would read. It carries NO
		 * `data-key`, which is what keeps it out of the roving model: the list
		 * stays one tab stop and arrowing walks services, not services and their
		 * expanders alternately.
		 */
		rowExtra?: Snippet<[T]>;
		/** Whether `rowExtra` renders for this row. Ignored without `rowExtra`. */
		expanded?: (row: T) => boolean;
		/**
		 * THE SORT, ON THE COLUMN HEADERS.
		 *
		 * `sortKey` is the id of the column currently sorted, `sortDir` its
		 * direction, and `onsort` is handed a column id when its header is
		 * activated. The primitive decides NOTHING about what that means: the
		 * comparator, the direction toggle, the URL and — on a list that carries
		 * an irreversible action — ADR-0038's freeze rule all stay with the
		 * screen. This renders the affordance and reports the press.
		 *
		 * ALL THREE ARE OPTIONAL AND OMITTING THEM IS THE OLD BEHAVIOUR. A list
		 * that passes none, and columns that do not set `sortable`, render the
		 * same plain `<th>` text they always did. `aria-sort` is emitted ONLY on
		 * columns that declared `sortable`, because the attribute marks a column
		 * the user can sort — putting `none` on a column that can never be sorted
		 * announces an affordance that is not there.
		 */
		sortKey?: string;
		sortDir?: 'asc' | 'desc';
		onsort?: (columnId: string) => void;
	}

	let {
		label,
		columns,
		rows,
		key,
		cell,
		total,
		offset = 0,
		state: listState = 'default',
		emptyTitle = 'Nothing here yet',
		emptyText = '',
		emptyActions,
		filteredEmptyTitle = 'All results are hidden by the applied filter',
		filteredEmptyText = '',
		filteredEmptyActions,
		partialNote = '',
		staleNote = '',
		loadingNote = '',
		hasMore = false,
		loadingMore = false,
		onloadmore,
		stack = 'labels',
		rowIntrinsic,
		rovingOptOut,
		onactivate,
		class: extraClass = '',
		rowExtra,
		expanded,
		sortKey,
		sortDir,
		onsort
	}: Props = $props();

	/** A column is a sort control only if it asked to be AND there is something
	 * to call. A button that does nothing is worse than a plain header. */
	function sortsOn(column: ListColumn): boolean {
		return column.sortable === true && onsort !== undefined;
	}

	/**
	 * `aria-sort` for one header. ARIA spells the directions out — `asc` is not a
	 * value it accepts — and at most one column may carry anything other than
	 * `none`, which falls out of `sortKey` being a single id.
	 *
	 * `$lib/sortspec` exports the same function over a `SortSpec`; this one takes
	 * the two props apart because the primitive deliberately does not import the
	 * sort module. The two agree by having one rule each rather than by sharing a
	 * type the primitive would then be coupled to.
	 */
	function ariaSortFor(column: ListColumn): 'ascending' | 'descending' | 'none' | undefined {
		if (!sortsOn(column)) return undefined;
		if (column.id !== sortKey) return 'none';
		return sortDir === 'desc' ? 'descending' : 'ascending';
	}

	let tableEl = $state<HTMLTableElement | null>(null);

	/**
	 * The roved row's identity. Not an index: see the `key` prop.
	 *
	 * It survives a reorder because it is compared against `data-key`, and it
	 * survives an append because the assignment is re-run and finds the same
	 * key still present.
	 */
	let activeKey = $state<string | null>(null);

	const columnCount = $derived(columns.length);
	const cols = $derived(gridTemplate(columns));

	/**
	 * The `aria-rowindex` of every rendered row, computed once rather than from
	 * the loop counter.
	 *
	 * An open expander is a real row in the grid, so it consumes an index — and
	 * the moment one does, `offset + i + 2` is wrong for every row beneath it.
	 * §11's whole point is that a confidently wrong position arriving through the
	 * accessibility tree is worse than none, so the arithmetic is done here where
	 * it can account for them instead of in the template where it cannot.
	 */
	const laidOut = $derived.by(() => {
		let consumed = 0;
		return rows.map((row) => {
			const index = rowIndex(consumed, offset);
			consumed += 1;
			const open = rowExtra !== undefined && expanded?.(row) === true;
			if (open) consumed += 1;
			return { row, index, open };
		});
	});

	/** Open expanders are rows too, so the total the AT is told includes them. */
	const openCount = $derived(laidOut.filter((r) => r.open).length);
	const declaredTotal = $derived(total === undefined || total < 0 ? total : total + openCount);
	/**
	 * DENSITY IS SCOPED TO THIS LIST, not to `<html>`.
	 *
	 * ADR-0029 names this as one of three mitigations for the density toggle,
	 * which is the expensive operation on a long list rather than scrolling —
	 * measured at 153 ms / 1,199 ms / 6,508 ms at 1k / 5k / 25k rows against
	 * §7.2's 100 ms Tier-0 hard fail. Reading the preference here and stamping
	 * it on the table means the density tokens resolve on this subtree rather
	 * than being inherited from the root, so a list can in principle be
	 * restyled without the rest of the document.
	 *
	 * ⚠️ HOW MUCH THAT BUYS IS MEASURED IN scripts/list-bench.mjs AND IT IS NOT
	 * WHAT THE ADR ASSUMES. Bounding the invalidation to the list bounds it to
	 * the part of the document that has 25,000 elements in it; the rest of the
	 * page is a toolbar and a sidebar. The number is in the report.
	 */
	const density = $derived(prefs.density);
	const intrinsic = $derived(rowIntrinsic ?? ROW_INTRINSIC[density] ?? ROW_INTRINSIC.compact);

	const showTable = $derived(listState !== 'empty' && listState !== 'filtered-empty');

	/**
	 * Which columns render on which line of the two-line phone fork, and which
	 * of the second-line columns is first — so the `·` separator is a real
	 * element the component places, rather than `::before` generated content
	 * that would land inside the cell's accessible name.
	 */
	const firstSecondLine = $derived(
		stack === 'two-line' ? (columns.find((c) => c.stackLine === 2)?.id ?? null) : null
	);

	function stackLine(column: ListColumn): string | undefined {
		if (stack !== 'two-line') return undefined;
		return String(column.stackLine ?? 'hidden');
	}

	/**
	 * `--cols` and `--row-ci` go through the CSSOM. See the file header: a
	 * `style` attribute is refused by `style-src 'self'` and applies nothing,
	 * which is the kind of failure that survives review because the attribute is
	 * still visible in the DOM inspector.
	 */
	/**
	 * The density this list last rendered at. `undefined` until the table has a
	 * box, so the first render seeds it rather than counting as a change — a
	 * freshly mounted row has nothing remembered and needs no invalidation.
	 */
	let renderedDensity: string | undefined;

	$effect(() => {
		const el = tableEl;
		// `density` is read unconditionally so this effect depends on it even on
		// the pass where the table is not mounted. Reading it after an early
		// return would make the subscription conditional, and the invalidation
		// would silently stop happening for any list that starts empty.
		const nextDensity = density;
		if (!el) return;
		el.style.setProperty('--cols', cols);
		el.style.setProperty('--row-ci', `${intrinsic}px`);

		/*
		 * §7.4's INVALIDATION RULE, WHICH IS REQUIRED RATHER THAN ADVISORY.
		 *
		 * The rule and the three mechanisms that do NOT satisfy it are written
		 * out at `.tbl--remeasure` in app.css. In short: `contain-intrinsic-size:
		 * auto` remembers each row's last real height, the keyed `{#each}` below
		 * reuses row nodes across a density change, and the two together leave
		 * every off-screen row placeheld at the PREVIOUS density until the user
		 * scrolls it back into view. §7.4 permits rebuilding the rows or forcing
		 * re-measurement; this is the second, and the measurement that chose it
		 * is in scripts/measurements/2026-08-17-density-invalidation.md.
		 *
		 * ⚠️ THE ORDER OF THE THREE LINES BELOW IS THE WHOLE MECHANISM.
		 *
		 *   1. add the class, so every row stops skipping its contents;
		 *   2. force style and layout SYNCHRONOUSLY, here, in this task, so the
		 *      rows lay out at the density that was just applied. This is also
		 *      why `--row-ci` is set above rather than in an effect of its own —
		 *      an effect ordering that put the forced layout before the new value
		 *      would measure the old one;
		 *   3. take the class off after TWO animation frames.
		 *
		 * ⚠️ TWO FRAMES, NOT ONE, AND THE DIFFERENCE IS A FIX THAT MEASURES AS NO
		 * FIX. A rAF callback scheduled from inside an event handler runs in the
		 * SAME frame's rendering steps, BEFORE style and layout, so a single rAF
		 * can take the class off before the frame that was supposed to do the
		 * recording ever renders. Measured on the shipped primitive, 5,000 rich
		 * rows, compact -> relaxed: with one frame, the document's scrollHeight
		 * was already correct at 271,870 px synchronously after step (2) and had
		 * fallen back to 232,198 px two frames later — 14.59% error, which is the
		 * 14.57% of doing nothing at all. The forced layout in (2) gives correct
		 * GEOMETRY immediately; it does not reliably cause the browser to RECORD
		 * that geometry as the row's last remembered size, and only a completed
		 * rendering opportunity does. Two frames guarantee one.
		 *
		 * The second frame is nearly free: the rows laid out in the first one and
		 * the layout is cached, so it costs a style pass rather than a relayout.
		 */
		if (renderedDensity !== undefined && renderedDensity !== nextDensity) {
			el.classList.add('tbl--remeasure');
			void el.offsetHeight;
			let inner = 0;
			const frame = requestAnimationFrame(() => {
				inner = requestAnimationFrame(() => el.classList.remove('tbl--remeasure'));
			});
			renderedDensity = nextDensity;
			return () => {
				cancelAnimationFrame(frame);
				cancelAnimationFrame(inner);
				el.classList.remove('tbl--remeasure');
			};
		}
		renderedDensity = nextDensity;
	});

	const rovingParams = $derived({
		activeKey,
		enabled: rovingOptOut === undefined,
		letterKeys: prefs.shortcuts,
		// Anything that changes when the rendered rows change. This is what makes
		// the assignment re-run after a "Load more" append, which ADR-0029 makes
		// the primary interaction — a tabindex set once at init is wrong within
		// one click.
		revision: rows.length,
		onmove: (k: string) => {
			activeKey = k;
		},
		onactivate: (k: string) => {
			const row = rows.find((r) => key(r) === k);
			if (row !== undefined) onactivate?.(row);
		}
	});
</script>

<!--
	`overflow-x: clip`, not `auto`. `auto` computes to `auto` on BOTH axes, which
	makes this box a scroll container; the sticky header then pins to the
	wrapper's scrollport instead of the viewport, and the wrapper never scrolls
	vertically, so the header does not pin at all. Measured on the mockup: one
	header pinned at 1440 px and zero at 1000 px, a broken band from 761 px to
	1,099 px. A cell that genuinely overflows scrolls inside itself
	(`.cell-scroll`), which is what keeps the wrapper out of the scroll-container
	business while still degrading to a scroll rather than to an amputation.
-->
<div class="tablewrap">
	{#if listState === 'partial' && partialNote}
		<div class="banner banner--warn" role="status">
			<div class="banner__body"><div class="banner__text">{partialNote}</div></div>
		</div>
	{/if}
	{#if listState === 'stale' && staleNote}
		<div class="banner" role="status">
			<div class="banner__body"><div class="banner__text">{staleNote}</div></div>
		</div>
	{/if}

	{#if listState === 'empty'}
		<div class="empty">
			<h2 class="empty__title">{emptyTitle}</h2>
			{#if emptyText}<p class="empty__text">{emptyText}</p>{/if}
			{#if emptyActions}<div class="empty__actions">{@render emptyActions()}</div>{/if}
		</div>
	{:else if listState === 'filtered-empty'}
		<div class="empty">
			<h2 class="empty__title">{filteredEmptyTitle}</h2>
			{#if filteredEmptyText}<p class="empty__text">{filteredEmptyText}</p>{/if}
			{#if filteredEmptyActions}
				<div class="empty__actions">{@render filteredEmptyActions()}</div>
			{/if}
		</div>
	{/if}

	{#if showTable}
		<!--
			Every role here is explicit because the computed display is grid, which
			drops the implicit table semantics these elements would otherwise carry.
			aria-rowcount/aria-rowindex are 1-based over the FULL result set, not
			over the rendered window (§11) — the DOM holds a keyset prefix by
			construction, and content-visibility skips the contents of off-screen
			rows, so neither the browser nor the AT can compute a row's position in
			the whole set without being told.
			THE ROLES ARE NOT REDUNDANT AND THE WARNING IS WRONG HERE. svelte-check
			flags role="table" on a <table> because a <table> already has it — which
			is true only while the element generates a table box. `display: grid`
			removes it, and with it every implicit role in the subtree, so without
			these attributes the accessibility tree carries a stack of generic
			containers instead of a table. The whole point of ADR-0029 is that this
			element is not laid out as a table.
		-->
		<!-- svelte-ignore a11y_no_redundant_roles -->
		<table
			bind:this={tableEl}
			class="tbl {extraClass}"
			class:tbl--stack={stack === 'labels'}
			class:tbl--2line={stack === 'two-line'}
			role="table"
			aria-label={label}
			aria-rowcount={rowCount(declaredTotal)}
			aria-colcount={columnCount}
			data-density={density}
			data-roving={rovingOptOut === undefined ? '' : undefined}
			data-roving-optout={rovingOptOut}
			use:roving={rovingParams}
		>
			<!-- svelte-ignore a11y_no_redundant_roles -->
			<thead role="rowgroup">
				<!-- svelte-ignore a11y_no_redundant_roles -->
				<tr role="row" aria-rowindex={1}>
					{#each columns as column, c (column.id)}
						<th
							role="columnheader"
							scope="col"
							aria-colindex={c + 1}
							aria-sort={ariaSortFor(column)}
							class:is-num={column.align === 'end'}
							data-col={column.id}
						>
							{#if sortsOn(column)}
								<!--
									A real <button> INSIDE the <th>, never a click handler on the
									<th> itself: `columnheader` is not a focusable role, so a
									handler on the cell is a control no keyboard can reach.

									The direction glyph is aria-hidden because `aria-sort` on the
									<th> carries the same fact to the accessibility tree already —
									announcing both reads it twice.

									NO TRANSITION HERE OR ON THE REORDER IT CAUSES.
									DESIGN-DIRECTION §6 puts sort at 0 ms on the critical path and
									§9.1a extends it to say a reorder is never animated anywhere,
									because an animation widens the window in which the row under
									the pointer is neither where it was nor where it is going.
								-->
								<button
									type="button"
									class="th__sort"
									class:th__sort--on={column.id === sortKey}
									onclick={() => onsort?.(column.id)}
								>
									<span class="th__sortlabel">{column.header}</span>
									<span class="th__arrow" aria-hidden="true"
										>{column.id !== sortKey ? '' : sortDir === 'desc' ? '▾' : '▴'}</span
									>
								</button>
							{:else}
								{column.header}
							{/if}
						</th>
					{/each}
				</tr>
			</thead>
			<!-- svelte-ignore a11y_no_redundant_roles -->
			<tbody role="rowgroup">
				{#each laidOut as entry (key(entry.row))}
					{@const row = entry.row}
					<!--
						tabindex is written by the roving action rather than here, and
						that is deliberate: exactly one row carries 0 at any moment and
						the component does not know which until the action has run. A
						literal tabindex={0} in the markup is the "eight tab stops on a
						seven-row table" bug.
					-->
					<!-- svelte-ignore a11y_no_redundant_roles -->
					<tr role="row" aria-rowindex={entry.index} data-key={key(row)}>
						{#each columns as column, c (column.id)}
							<!--
								aria-colindex is 1-based and explicit for the same reason the
								roles are: the computed display is grid, so nothing in this
								subtree carries an implicit table position for the
								accessibility tree to derive one from.
							-->

							<td
								role="cell"
								aria-colindex={c + 1}
								class:is-num={column.align === 'end'}
								data-col={column.id}
								data-line={stackLine(column)}
							>
								{#if stack === 'two-line' && column.stackLine === 2 && column.id !== firstSecondLine}
									<!-- A real element, aria-hidden, never ::before content:
									     generated content lands inside the cell's accessible
									     name, so a screen reader announces the separator. It
									     sits between the second-line fields, so the first of
									     them does not get one. -->
									<span class="stacksep" aria-hidden="true">·</span>
								{/if}
								<!--
									The phone label is a real element marked aria-hidden, never
									`td[data-label]::before { content: attr(...) }`. That pattern
									puts the column name inside the cell's accessible name, so a
									screen reader in table-navigation mode announces the header
									and then the same word again on every cell of every row —
									"Service. Service, Sonarr…" — and it duplicates the header
									string into an attribute most translation pipelines will not
									pick up.
								-->
								{#if column.stackLabel !== false && column.stackLine !== 2}
									<span class="stacklabel" aria-hidden="true">{column.header}</span>
								{/if}
								{@render cell(row, column)}
							</td>
						{/each}
					</tr>
					{#if entry.open && rowExtra}
						<!--
							The expander. `colspan` means nothing to a grid, so the cell
							says so in grid terms via `.tbl td[colspan]` and repeats the
							fact to the accessibility tree with aria-colspan. No data-key,
							so roving walks services rather than services and expanders
							alternately.
						-->
						<!-- svelte-ignore a11y_no_redundant_roles -->
						<tr role="row" class="is-expand" aria-rowindex={entry.index + 1}>
							<td role="cell" colspan={columnCount} aria-colspan={columnCount}>
								{@render rowExtra(row)}
							</td>
						</tr>
					{/if}
				{/each}
			</tbody>
		</table>

		{#if listState === 'loading' && loadingNote}
			<p class="list__note" role="status">{loadingNote}</p>
		{/if}

		{#if hasMore}
			<!--
				"Load more" over keyset pages, never infinite scroll and never
				virtualization (ADR-0029). Appending must not reset focus, scroll
				position or the roving assignment: the keyed {#each} above moves the
				existing DOM nodes rather than rewriting them, and the action's
				`revision` re-runs an assignment that is idempotent, so the row the
				user was on keeps both its focus and its tab stop.
			-->
			<div class="list__more">
				<button
					type="button"
					class="btn"
					onclick={() => onloadmore?.()}
					aria-disabled={loadingMore ? 'true' : undefined}
				>
					{loadingMore ? 'Loading' : 'Load more'}
				</button>
				{#if total !== undefined && total >= 0}
					<span class="list__count">{rows.length} of {total}</span>
				{/if}
			</div>
		{/if}
	{/if}
</div>
