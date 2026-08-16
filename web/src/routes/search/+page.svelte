<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import {
		ApiError,
		grabRelease,
		openEventStream,
		problemsFrom,
		startSearch,
		type IndexerOutcome,
		type Release,
		type SearchReport,
		type StreamHandles
	} from '$lib/api';
	import { formatAge, formatSize, protocolClass } from '$lib/format';

	type GrabState = { state: 'idle' | 'grabbing' | 'grabbed' | 'failed'; detail: string };

	let query = $state('');
	let submitted = $state('');
	let searchId = $state<string | undefined>(undefined);
	let searching = $state(false);
	let finished = $state(false);
	let searchError = $state('');
	// The server's own error code. `no_indexer_service` is not a failure the user
	// can retry — it means nothing is configured — so it gets the link that fixes
	// it rather than the generic banner.
	let searchErrorCode = $state('');
	let streamGap = $state('');
	let streamConnected = $state(false);

	let releases = $state<Release[]>([]);
	// Every indexer the server has told us about, and the ones that have reported
	// a final outcome. Until the closing report lands these are the only honest
	// answer to "how much of this search is in".
	let indexersSeen = $state<number[]>([]);
	let outcomes = $state<IndexerOutcome[]>([]);
	let report = $state<SearchReport | undefined>(undefined);
	let grabs = $state<Record<number, GrabState>>({});

	// The report supersedes the running tally once it arrives: it is the server's
	// own accounting, including indexers that were skipped without ever starting.
	const problems = $derived(problemsFrom(report ? report.indexers : outcomes));
	const indexersTotal = $derived(report?.totalIndexers ?? (indexersSeen.length || undefined));
	const indexersDone = $derived(report ? report.totalIndexers : outcomes.length);

	let stream: StreamHandles | undefined;

	// Results stream as indexers answer, so the cross-indexer dedupe cannot be
	// done up front: a later, higher-priority indexer names the candidate it
	// replaces and this drops the superseded row (internal/httpapi/search.go).
	function mergeReleases(incoming: Release[]) {
		let next = releases;
		for (const release of incoming) {
			if (next.some((r) => r.candidateId === release.candidateId)) continue;
			if (release.supersedesCandidateId !== undefined) {
				next = next.filter((r) => r.candidateId !== release.supersedesCandidateId);
			}
			next = [...next, release];
		}
		releases = next;
	}

	function noteIndexer(outcome: IndexerOutcome, phase: string) {
		if (!indexersSeen.includes(outcome.indexerId)) {
			indexersSeen = [...indexersSeen, outcome.indexerId];
		}
		if (phase !== 'indexer_done') return;
		outcomes = [...outcomes.filter((o) => o.indexerId !== outcome.indexerId), outcome];
	}

	// One stream for the whole page, opened once. Results are appended as they
	// arrive per indexer — that progressive render is the point of the SSE design.
	onMount(() => {
		stream = openEventStream(
			(event) => {
				if (event.kind === 'unknown') return;
				if (event.kind === 'missed') {
					streamGap = event.action ? `${event.message} — ${event.action}` : event.message;
					return;
				}
				// A frame from an older search is ignored, not rendered late.
				if (searchId && event.searchId && event.searchId !== searchId) return;

				switch (event.kind) {
					case 'started':
						// The stream can beat the 202 that carries the same id.
						if (!searchId) searchId = event.searchId;
						break;
					case 'results':
						mergeReleases(event.releases);
						break;
					case 'indexer':
						noteIndexer(event.indexer, event.phase);
						break;
					case 'done':
						report = event.report;
						searching = false;
						finished = true;
						break;
					case 'failed':
						searchError = event.action ? `${event.message} — ${event.action}` : event.message;
						searching = false;
						break;
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
		indexersSeen = [];
		outcomes = [];
		report = undefined;
		grabs = {};
		searchError = '';
		searchErrorCode = '';
		streamGap = '';
		finished = false;
		searching = true;

		try {
			const accepted = await startSearch(trimmed);
			if (accepted.searchId) searchId = accepted.searchId;
		} catch (error) {
			searching = false;
			searchError = error instanceof ApiError ? error.detail : String(error);
			searchErrorCode = error instanceof ApiError ? error.code : '';
		}
	}

	async function grab(release: Release) {
		const id = release.candidateId;
		grabs = { ...grabs, [id]: { state: 'grabbing', detail: '' } };
		try {
			await grabRelease(id);
			grabs = { ...grabs, [id]: { state: 'grabbed', detail: '' } };
		} catch (error) {
			grabs = {
				...grabs,
				[id]: {
					state: 'failed',
					detail: error instanceof ApiError ? error.message : String(error)
				}
			};
		}
	}
</script>

<svelte:head><title>Search · UsArr</title></svelte:head>

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

{#if searchErrorCode === 'no_indexer_service'}
	<!--
		Not a failure to retry: nothing is configured. The fix is one click away
		rather than a sentence describing an API call.
	-->
	<div class="banner banner-error" role="alert">
		There is no indexer service to search. Add a Prowlarr instance on
		<a href={resolve('/services')}>Services</a> and this screen will start working.
		<p class="banner-detail">{searchError}</p>
	</div>
{:else if searchError}
	<div class="banner banner-error" role="alert">
		The search did not complete.
		<p class="banner-detail">{searchError}</p>
	</div>
{/if}

{#if streamGap}
	<div class="banner banner-warn">
		Some events were missed while the connection was down, so this list may be incomplete.
		<p class="banner-detail">{streamGap}</p>
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
			<span>{indexersDone} of {indexersTotal} indexers reported</span>
		{/if}
		<span>{searching ? 'Searching' : finished ? 'Finished' : 'Idle'}</span>
	</div>
	{#if report?.summary}
		<!--
			Prowlarr answers 200 with `[]` for "every indexer failed", "all
			rate-limited" and "nothing matched" alike, so this sentence is the only
			thing that tells them apart — ARCHITECTURE.md §17.7.
		-->
		<p class="status-line">{report.summary}</p>
	{/if}
{/if}

{#if releases.length > 0}
	<ul class="results">
		{#each releases as release (release.candidateId)}
			{@const grabState = grabs[release.candidateId] ?? { state: 'idle', detail: '' }}
			<li class="result">
				<div class="result-main">
					<div class="result-title">{release.title}</div>
					<div class="result-meta">
						{#if release.protocol}<span class={protocolClass(release.protocol)}
								>{release.protocol}</span
							>{/if}
						{#if release.indexerName}<span>{release.indexerName}</span>{/if}
						{#if release.sizeBytes !== undefined}<span>{formatSize(release.sizeBytes)}</span>{/if}
						{#if release.seeders !== undefined}<span>{release.seeders} seeders</span>{/if}
						{#if formatAge(release.ageDays)}<span>{formatAge(release.ageDays)}</span>{/if}
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
