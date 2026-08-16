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
		class: extraClass = ''
	}: Props = $props();

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
	$effect(() => {
		const el = tableEl;
		if (!el) return;
		el.style.setProperty('--cols', cols);
		el.style.setProperty('--row-ci', `${intrinsic}px`);
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
			aria-rowcount={rowCount(total)}
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
					{#each columns as column (column.id)}
						<th
							role="columnheader"
							scope="col"
							class:is-num={column.align === 'end'}
							data-col={column.id}>{column.header}</th
						>
					{/each}
				</tr>
			</thead>
			<!-- svelte-ignore a11y_no_redundant_roles -->
			<tbody role="rowgroup">
				{#each rows as row, i (key(row))}
					<!--
						tabindex is written by the roving action rather than here, and
						that is deliberate: exactly one row carries 0 at any moment and
						the component does not know which until the action has run. A
						literal tabindex={0} in the markup is the "eight tab stops on a
						seven-row table" bug.
					-->
					<!-- svelte-ignore a11y_no_redundant_roles -->
					<tr role="row" aria-rowindex={rowIndex(i, offset)} data-key={key(row)}>
						{#each columns as column (column.id)}
							<td
								role="cell"
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
