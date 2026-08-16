<script lang="ts">
	/**
	 * A pointer from /search to /requests, and NOT a redirect.
	 *
	 * ARCHITECTURE.md §17.5 makes Requests the home of the Prowlarr free-text
	 * path, and this screen predates it — but it is a door the owner has been
	 * using, and a redirect would take a working bookmark away before the new
	 * screen carries the release results table that is still being finished
	 * here. So both stay reachable and this says which is which.
	 *
	 * It is a LAYOUT rather than an edit to `+page.svelte` on purpose: that file
	 * is being rewritten in another thread with sortable results, the flag chips
	 * and indexer scoping, and a one-line insertion into it is a merge conflict
	 * for no benefit. A layout is additive, and it is deleted whole when the
	 * results table moves to Requests and this route folds into it.
	 */
	import { resolve } from '$app/paths';

	let { children } = $props();
</script>

<div class="banner" role="status">
	<div class="banner__body">
		<div class="banner__title">Indexer search and grabbing now live on Requests</div>
		<div class="banner__text">
			This screen still works and still grabs. The per-release results table has not moved yet, so
			it is the one place a release can be grabbed from — everything around it, including your
			recent grabs, is on Requests.
		</div>
	</div>
	<div class="banner__actions">
		<a class="btn" href={resolve('/requests')}>Open Requests</a>
	</div>
</div>

{@render children()}
