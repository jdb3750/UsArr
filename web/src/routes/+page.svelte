<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { ApiError, checkReady } from '$lib/api';

	type Backend = { state: 'checking' | 'ready' | 'unavailable'; detail: string };

	let backend = $state<Backend>({ state: 'checking', detail: '' });

	onMount(async () => {
		try {
			await checkReady();
			backend = { state: 'ready', detail: '' };
		} catch (error) {
			backend = {
				state: 'unavailable',
				detail: error instanceof ApiError ? error.message : String(error)
			};
		}
	});
</script>

<svelte:head><title>UsArr</title></svelte:head>

<h2>UsArr is running</h2>

<p>
	The frontend was built, embedded into the binary and served from it. This is the shell, not the
	product: the real screens (home, services, search, requests) are specified in
	<code>docs/ARCHITECTURE.md</code> §17 and have not been built yet.
</p>

<p>
	<a href={resolve('/search')}>Search and grab</a> is the one working path — free-text indexer search
	over Prowlarr, with results streaming in per indexer.
</p>

<p>
	Start on <a href={resolve('/services')}>Services</a>. A fresh install has nothing configured, so
	search answers <code>no_indexer_service</code> until a Prowlarr instance is added there.
</p>

{#if backend.state === 'checking'}
	<div class="status-line">Checking the backend.</div>
{:else if backend.state === 'ready'}
	<div class="status-line">Backend ready. <code>/api/health/ready</code> answered 200.</div>
{:else}
	<div class="banner banner-warn">
		The backend is not answering <code>/api/health/ready</code>. Nothing that needs the API will
		work until it does; this page is served from the embedded build and does not depend on it.
		<p class="banner-detail">{backend.detail}</p>
	</div>
{/if}
