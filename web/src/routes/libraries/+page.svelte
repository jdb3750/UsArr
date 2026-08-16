<script lang="ts">
	/**
	 * Placeholder. See the note in ../requests/+page.svelte — the sidebar row is
	 * part of the fixed set, the screen behind it is a later slice.
	 */
	import { resolve } from '$app/paths';
</script>

<svelte:head><title>Libraries · UsArr</title></svelte:head>

<div class="section">
	<div class="empty">
		<h2 class="empty__title">Libraries has not been built yet</h2>
		<p class="empty__text">
			This screen is specified in <code class="mono">docs/ARCHITECTURE.md</code> §17.8. A library binds
			the upstream containers a service already named to a request destination, so nothing can be configured
			here until a service exists.
		</p>
		<div class="empty__actions">
			<a class="btn btn--primary" href={resolve('/services')}>Go to Services</a>
		</div>
	</div>
</div>
