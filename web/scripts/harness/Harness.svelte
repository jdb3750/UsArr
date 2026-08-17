<script lang="ts">
	/**
	 * The measurement harness's only screen: one List of fabricated release
	 * rows, at whatever row count the driver asks for.
	 *
	 * The column set MODELS the Search-and-Grab release table — the widest list
	 * v0.1 ships, and the one ADR-0029's row-height finding was measured against
	 * — but it is NOT a copy of it, and the gap is written down here rather than
	 * left for the next reader to find in a number.
	 *
	 * ⚠️ SEVEN COLUMNS WHERE THE TABLE IS NOW ELEVEN. `routes/requests` has since
	 * gained Protocol, Category and Grabs, and gives Age / Size / Peers MEASURED
	 * px tracks (68 / 88 / 112) where this carries 9ch / 9ch / 8ch. That matters
	 * for exactly one thing and not for the rest: a row's height is set by its
	 * TALLEST cell, and the bimodality this harness measures — 44px×1308 and
	 * 48px×692 at compact — is the FLAGS cell wrapping its chips, not the
	 * controls and not any text cell, every one of which is `trunc`/nowrap and
	 * cannot grow. So the intrinsic figures published from here are the right
	 * statistic for a chip-and-controls row, and they are NOT an instrument for
	 * the 112px Peers track, which the real table widened from 88px precisely
	 * because `Not applicable` wrapped there. Matching these tracks would move
	 * width out of the `fr` pool and shift where the chips wrap — i.e. it would
	 * move the published mean, for a case this harness still could not render.
	 * Stated, therefore, rather than papered over.
	 */
	import List from '../../src/lib/List.svelte';
	import { NOTHING, capChips, type ListColumn } from '../../src/lib/list';
	// The APP'S OWN formatters, never a local copy: Size and Age are the two
	// cells whose text the product derives rather than receives, and a harness
	// that spelled them itself would be measuring its own spelling. `formatSize`
	// (not `sizeParts`) is what the release table renders today.
	import { formatAge, formatSize } from '../../src/lib/format';
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
				return formatSize(row.sizeBytes);
			case 'peers':
				return String(row.seeders);
			case 'age':
				return formatAge(row.ageDays);
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
				{formatSize(row.sizeBytes)}
			{:else if column.id === 'peers'}
				<span>{row.seeders} / {row.leechers}</span>
				<span class="sr">{row.seeders} seeders, {row.leechers} leechers</span>
			{:else if column.id === 'age'}
				{formatAge(row.ageDays)}
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
