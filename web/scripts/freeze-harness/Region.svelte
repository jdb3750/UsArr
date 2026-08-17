<script lang="ts">
	/**
	 * ONE results region wired to `$lib/frozenorder.svelte`, and nothing else.
	 *
	 * ⚠️ THE SHAPE IS THE PRODUCTION ONE, DELIBERATELY, AND A HAND-ROLLED
	 * `{#each}` OF DIVS WILL NOT DO. That was tried first, and it is why this
	 * note exists. `set()` in svelte/src/internal/client/reactivity/sources.js
	 * raises `state_unsafe_mutation` only when the reaction ACTIVE AT THE INSTANT
	 * OF THE WRITE carries DERIVED, BLOCK_EFFECT, ASYNC or EAGER_EFFECT — and
	 * which reaction that is, when the browser dispatches an event out of a node
	 * move, depends on how deeply the moved node sits in the block structure
	 * Svelte is reconciling. A flat `{#each}` of `<div>`s moved its rows in a
	 * context where the write was LEGAL, and the pre-fix code passed every
	 * scenario below. `$lib/List.svelte` — the real primitive, a keyed `{#each}`
	 * of rows each holding its own keyed `{#each}` of cells — moves them in a
	 * context where the write THROWS. That is what the Requests screen renders,
	 * so it is what the negative control has to render.
	 *
	 * So: the region element carries `use:order.region`, `$lib/List.svelte`
	 * renders inside it, and the pending control sits OUTSIDE it (ADR-0038
	 * clause 3 — a control that reorders must not itself be something you can
	 * aim at, or pointing at it would freeze the list it is offering to
	 * unfreeze). That is the arrangement in src/routes/requests/+page.svelte,
	 * rebuilt here over fabricated rows because the route itself is a product
	 * surface and DESIGN-DIRECTION §9.6 keeps fabricated data off those.
	 */
	import List from '$lib/List.svelte';
	import type { ListColumn } from '$lib/list';
	import { createFrozenOrder, settle, type FrozenOrder } from '$lib/frozenorder.svelte';
	import { feed, type FeedRow } from './state.svelte';

	const order: FrozenOrder<FeedRow> = createFrozenOrder<FeedRow>({
		source: () => feed.rows,
		key: (row) => row.id,
		// Descending by rank by default, so a higher-ranked arrival sorts to
		// index 0 and pushes every rendered row down one position.
		compare: () => {
			const sign = feed.descending ? 1 : -1;
			return (a, b) => sign * (b.rank - a.rank);
		},
		complete: () => feed.complete
	});

	const region = order.region;

	const columns: ListColumn[] = [
		{ id: 'label', header: 'Release', width: 'minmax(0, 3fr)' },
		{ id: 'rank', header: 'Rank', width: '10ch', align: 'end' },
		{ id: 'actions', header: 'Actions', width: 'minmax(max-content, auto)' }
	];

	/**
	 * DIAGNOSTIC ONLY — no assertion depends on these. They record when the
	 * browser dispatched each boundary event relative to the driver's flush, so
	 * a run that passes because NOTHING FIRED shows up in the output rather than
	 * hiding behind a green tick. Every assertion is on rendered order.
	 */
	type Observed = { type: string; duringFlush: boolean };
	const observed: Observed[] = [];

	function observe(node: HTMLElement) {
		const types = ['pointerenter', 'pointerleave', 'pointercancel', 'focusin', 'focusout'];
		const handlers = types.map(
			(type) =>
				[
					type,
					() => observed.push({ type, duringFlush: window.__freeze?.inFlush === true })
				] as const
		);
		for (const [t, h] of handlers) node.addEventListener(t, h);
		return {
			destroy() {
				for (const [t, h] of handlers) node.removeEventListener(t, h);
			}
		};
	}

	export function handle() {
		return {
			order,
			settle: () => settle(order),
			observed,
			drainObserved: () => observed.splice(0, observed.length)
		};
	}
</script>

<h1>freeze-while-aimed harness</h1>

<!-- OUTSIDE the region on purpose. See the header. -->
<p class="pending" data-testid="pending">
	{order.hasPending ? order.pendingLabel : 'no pending arrivals'}
</p>
<button type="button" data-testid="resort" onclick={() => order.resort()}>re-sort</button>
<button type="button" data-testid="outside">outside the region</button>

<div class="region" data-testid="region" use:region use:observe>
	<List
		label="Release candidates"
		{columns}
		rows={order.rows}
		key={(r: FeedRow) => r.id}
		total={order.rows.length}
		hasMore={false}
		emptyTitle="No results yet"
		emptyText="Nothing has arrived."
	>
		{#snippet cell(row: FeedRow, column: ListColumn)}
			{#if column.id === 'label'}
				<span class="trunc">{row.label}</span>
			{:else if column.id === 'rank'}
				{row.rank}
			{:else}
				<button type="button" class="btn btn--sm">Grab</button>
			{/if}
		{/snippet}
	</List>
</div>

<div class="tail">below the region</div>

<style>
	h1 {
		font-size: 14px;
		margin: 0;
	}
	.pending {
		margin: 0;
	}
	.region {
		width: 700px;
	}
	/* The driver parks a REAL pointer at a REAL coordinate, and it needs
	 * somewhere below the region to park it that is not the viewport edge. */
	.tail {
		height: 2000px;
		background: #ddd;
	}
</style>
