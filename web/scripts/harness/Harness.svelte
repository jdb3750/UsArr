<script lang="ts">
	/**
	 * The measurement harness's only screen: one List of fabricated release
	 * rows, at whatever row count the driver asks for.
	 *
	 * The column set is deliberately the Search-and-Grab release table's, since
	 * that is the widest list v0.1 ships and the one ADR-0029's row-height
	 * finding was measured against.
	 */
	import List from '../../src/lib/List.svelte';
	import { NOTHING, capChips, type ListColumn } from '../../src/lib/list';
	import { harness, type Release } from './state.svelte';

	const columns: ListColumn[] = [
		{ id: 'release', header: 'Release', width: 'minmax(0, 3fr)', stackLine: 1, stackLabel: false },
		{ id: 'indexer', header: 'Indexer', width: 'minmax(0, 1fr)', stackLine: 2 },
		{ id: 'size', header: 'Size', width: '9ch', align: 'end', stackLine: 2 },
		{ id: 'peers', header: 'Peers', width: '8ch', align: 'end' },
		{ id: 'age', header: 'Age', width: '9ch', align: 'end' },
		{ id: 'flags', header: 'Flags', width: 'minmax(0, 1fr)' },
		// The same 220 px reserve the real Search-and-Grab table declares, for the
		// same reason this file copies its column set: a content-sized track cannot
		// align across rows under ADR-0029, and one here made the harness misreport
		// the primitive — measured, the actions header sat 103.03 px right of every
		// body row and Indexer through Flags drifted 61.82-82.41 px with it.
		// Measured in the harness's OWN build, where /fonts/*.woff2 404 and
		// `document.fonts.check('600 13px "IBM Plex Sans"')` is false, this cell's
		// three controls span 143.20 px; 220 - 2 × --row-pad-x leaves 196 px, so
		// nothing wraps and no row height the bench publishes moves.
		{ id: 'actions', header: 'Actions', width: '220px' }
	];

	/** One short string per column, for the one-line row shape. */
	function simpleText(row: Release, column: ListColumn): string {
		switch (column.id) {
			case 'release':
				return row.release;
			case 'indexer':
				return row.indexer;
			case 'size':
				return row.size;
			case 'peers':
				return String(row.seeders);
			case 'age':
				return row.age;
			case 'flags':
				return row.flags[0] ?? NOTHING.empty;
			default:
				return NOTHING.empty;
		}
	}
</script>

<div class="section">
	<List
		label="Release candidates"
		{columns}
		rows={harness.rows}
		key={(r: Release) => r.id}
		total={harness.total}
		state={harness.state}
		hasMore={harness.hasMore}
		stack={harness.stack}
		rowIntrinsic={harness.rowIntrinsic}
		emptyTitle="No search results found"
		emptyText="Run a search below to see what your indexers have."
		filteredEmptyTitle="All results are hidden by the applied filter"
		filteredEmptyText="Clear the filter to see the other results."
		partialNote="4 of 9 indexers responded. The rest may still arrive."
		staleNote="Showing cached data from 11:47 on 15 Aug, 1 day ago."
		onloadmore={() => harness.loadMore()}
	>
		{#snippet cell(row: Release, column: ListColumn)}
			{#if harness.simple}
				<!-- One line of text, nothing else. See state.svelte.ts. -->
				<span class="trunc">{simpleText(row, column)}</span>
			{:else if column.id === 'release'}
				<span class="trunc mono" title={row.release}>{row.release}</span>
			{:else if column.id === 'indexer'}
				<span class="trunc" title={row.indexer}>{row.indexer}</span>
			{:else if column.id === 'size'}
				{row.size}
			{:else if column.id === 'peers'}
				<span>{row.seeders} / {row.leechers}</span>
				<span class="sr">{row.seeders} seeders, {row.leechers} leechers</span>
			{:else if column.id === 'age'}
				{row.age}
			{:else if column.id === 'flags'}
				{#if row.flags.length === 0}
					{NOTHING.empty}
				{:else}
					{@const capped = capChips(row.flags)}
					{#each capped.shown as flag (flag)}
						<span class="chip">{flag}</span>
					{/each}
					{#if capped.more > 0}<span class="chip">+{capped.more} more</span>{/if}
				{/if}
			{:else}
				<span class="cell-actions">
					<button type="button" class="btn btn--sm">Grab</button>
					<input type="checkbox" aria-label="Select {row.release}" />
					<select aria-label="Destination for {row.release}">
						<option>Movies</option>
						<option>Series</option>
					</select>
				</span>
			{/if}
		{/snippet}
	</List>
</div>
