<script lang="ts">
	import { onMount } from 'svelte';
	import {
		ApiError,
		grabRelease,
		openEventStream,
		startSearch,
		type IndexerProblem,
		type Release,
		type StreamHandles
	} from '$lib/api';
	import { formatSize, protocolClass } from '$lib/format';

	type GrabState = { state: 'idle' | 'grabbing' | 'grabbed' | 'failed'; detail: string };

	let query = $state('');
	let submitted = $state('');
	let searchId = $state<string | undefined>(undefined);
	let searching = $state(false);
	let finished = $state(false);
	let searchError = $state('');
	let streamConnected = $state(false);

	let releases = $state<Release[]>([]);
	let problems = $state<IndexerProblem[]>([]);
	let indexersTotal = $state<number | undefined>(undefined);
	let indexersDone = $state<number | undefined>(undefined);
	let grabs = $state<Record<string, GrabState>>({});

	let stream: StreamHandles | undefined;

	// One stream for the whole page, opened once. Results are appended as they
	// arrive per indexer — that progressive render is the point of the SSE design.
	onMount(() => {
		stream = openEventStream(
			(event) => {
				// A frame from an older search is ignored, not rendered late.
				if (event.kind !== 'unknown' && searchId && event.searchId && event.searchId !== searchId) {
					return;
				}
				if (event.kind === 'result') {
					if (releases.some((r) => r.id === event.release.id)) return;
					releases = [...releases, event.release];
				} else if (event.kind === 'status') {
					if (event.problems.length > 0) problems = event.problems;
					indexersTotal = event.indexersTotal ?? indexersTotal;
					indexersDone = event.indexersDone ?? indexersDone;
				} else if (event.kind === 'done') {
					searching = false;
					finished = true;
				}
			},
			(connected) => (streamConnected = connected)
		);

		return () => stream?.close();
	});

	async function runSearch(event: SubmitEvent) {
		event.preventDefault();
		const trimmed = query.trim();
		if (trimmed === '') return;

		submitted = trimmed;
		searchId = undefined;
		releases = [];
		problems = [];
		indexersTotal = undefined;
		indexersDone = undefined;
		grabs = {};
		searchError = '';
		finished = false;
		searching = true;

		try {
			const started = await startSearch(trimmed);
			searchId = started.searchId;
			if (started.releases.length > 0) releases = started.releases;
			if (started.problems.length > 0) problems = started.problems;
			indexersTotal = started.indexersTotal;
			indexersDone = started.indexersDone;
		} catch (error) {
			searching = false;
			searchError = error instanceof ApiError ? error.message : String(error);
		}
	}

	async function grab(release: Release) {
		grabs = { ...grabs, [release.id]: { state: 'grabbing', detail: '' } };
		try {
			await grabRelease(release.id);
			grabs = { ...grabs, [release.id]: { state: 'grabbed', detail: '' } };
		} catch (error) {
			grabs = {
				...grabs,
				[release.id]: {
					state: 'failed',
					detail: error instanceof ApiError ? error.message : String(error)
				}
			};
		}
	}
</script>

<svelte:head><title>Search — UsArr</title></svelte:head>

<h2>Search indexers</h2>

<form class="search-form" onsubmit={runSearch}>
	<label class="visually-hidden" for="query">Search terms</label>
	<input
		id="query"
		name="query"
		type="search"
		autocomplete="off"
		placeholder="Free-text indexer search"
		bind:value={query}
	/>
	<button type="submit" disabled={query.trim() === ''}>Search</button>
</form>

<!--
	Every banner below is non-modal and sits above the list. The results list is
	never greyed out and never replaced — ARCHITECTURE.md §17.7.
-->

{#if searchError}
	<div class="banner banner-error" role="alert">
		The search could not be started.
		<p class="banner-detail">{searchError}</p>
	</div>
{/if}

{#if problems.length > 0}
	<div class="banner banner-warn">
		{problems.length}
		{problems.length === 1 ? 'indexer is' : 'indexers are'} not answering. These results are partial.
		{#each problems as problem (problem.indexer)}
			<p class="banner-detail">{problem.indexer}: {problem.error}</p>
		{/each}
	</div>
{/if}

{#if submitted && !streamConnected && !searchError}
	<div class="banner banner-warn">
		The event stream at <code>/api/events</code> is not connected, so results cannot arrive progressively.
		Anything already listed below is still valid.
	</div>
{/if}

{#if submitted}
	<div class="status-line">
		<span>Query: {submitted}</span>
		<span>{releases.length} {releases.length === 1 ? 'result' : 'results'}</span>
		{#if indexersTotal !== undefined}
			<span>{indexersDone ?? 0} of {indexersTotal} indexers reported</span>
		{/if}
		<span>{searching ? 'Searching' : finished ? 'Finished' : 'Idle'}</span>
	</div>
{/if}

{#if releases.length > 0}
	<ul class="results">
		{#each releases as release (release.id)}
			{@const grabState = grabs[release.id] ?? { state: 'idle', detail: '' }}
			<li class="result">
				<div class="result-main">
					<div class="result-title">{release.title}</div>
					<div class="result-meta">
						{#if release.protocol}<span class={protocolClass(release.protocol)}
								>{release.protocol}</span
							>{/if}
						{#if release.indexer}<span>{release.indexer}</span>{/if}
						{#if release.category}<span>{release.category}</span>{/if}
						{#if release.size !== undefined}<span>{formatSize(release.size)}</span>{/if}
						{#if release.seeders !== undefined}<span>{release.seeders} seeders</span>{/if}
						{#if release.age}<span>{release.age}</span>{/if}
					</div>
				</div>
				<div class="result-action">
					{#if grabState.state === 'grabbed'}
						<span class="grab-state grab-state-ok">Sent to Prowlarr</span>
					{:else}
						<button
							type="button"
							onclick={() => grab(release)}
							disabled={grabState.state === 'grabbing'}
						>
							{grabState.state === 'grabbing' ? 'Grabbing' : 'Grab'}
						</button>
					{/if}
				</div>
			</li>
			{#if grabState.state === 'failed'}
				<li class="result">
					<span class="grab-state grab-state-error">Grab failed — {grabState.detail}</span>
				</li>
			{/if}
		{/each}
	</ul>
{:else if submitted && !searching && !searchError}
	<p class="empty">
		No releases for “{submitted}”. Indexer search matches what the indexer itself matches; UsArr
		does not rewrite the query.
	</p>
{:else if submitted && searching}
	<p class="empty">Waiting for the first indexer to report.</p>
{:else if !submitted}
	<p class="empty">
		Free-text search across every indexer configured in Prowlarr. Results stream in per indexer as
		they answer, and Grab posts the release back to Prowlarr.
	</p>
{/if}

<style>
	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
</style>
