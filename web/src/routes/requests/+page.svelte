<script lang="ts">
	/**
	 * Placeholder. The sidebar is the fixed six-entry set ADR-0027 commits to,
	 * so the row exists from the first shell commit; the screen behind it does
	 * not. A row that leads to a SvelteKit 404 is worse than a screen that
	 * states its own status, which is what this does until the real one lands.
	 */
	import { resolve } from '$app/paths';
</script>

<svelte:head><title>Requests — UsArr</title></svelte:head>

<div class="section">
	<div class="empty">
		<h2 class="empty__title">Requests has not been built yet</h2>
		<p class="empty__text">
			This screen is specified in <code class="mono">docs/ARCHITECTURE.md</code> §17.5 and covers both
			paths: the Prowlarr free-text indexer search-and-grab path, and the *Arr-backed request flow. Only
			the first of those is in v0.1, and it currently lives on Search.
		</p>
		<div class="empty__actions">
			<a class="btn btn--primary" href={resolve('/search')}>Go to Search</a>
		</div>
	</div>
</div>
