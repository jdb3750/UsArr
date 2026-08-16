<script lang="ts">
	import '../app.css';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';

	let { children } = $props();
</script>

<header class="app-header">
	<h1 class="app-title">UsArr</h1>
	<nav class="app-nav" aria-label="Main">
		<a href={resolve('/')} aria-current={page.url.pathname === '/' ? 'page' : undefined}>Home</a>
		<a href={resolve('/search')} aria-current={page.url.pathname === '/search' ? 'page' : undefined}
			>Search</a
		>
	</nav>
</header>

<main>
	{@render children()}
</main>
