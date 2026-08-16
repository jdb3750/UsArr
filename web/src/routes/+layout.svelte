<script lang="ts">
	/**
	 * The shell's one guard.
	 *
	 * Every /api/v1 route except the auth bootstrap sits behind
	 * `authenticated` (internal/httpapi/server.go), so a signed-out browser gets
	 * 401 from search, from /api/events and from the services screen alike. The
	 * layout resolves the session ONCE, before any child route runs its own
	 * fetches, and registers the 401 hook so a session that expires mid-visit
	 * lands on the sign-in page instead of on a page of error banners.
	 *
	 * Nothing here renders an empty screen: while the bootstrap call is in
	 * flight the page says it is checking, and a backend that does not answer
	 * gets a banner naming the endpoint that failed.
	 */
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { ApiError, logout, onUnauthorized } from '$lib/api';
	import { session } from '$lib/session.svelte';

	let { children } = $props();

	let unreachable = $state('');
	let signingOut = $state(false);

	const loginPath = resolve('/login');
	const onLoginRoute = $derived(page.url.pathname === loginPath);

	onMount(() => {
		onUnauthorized(() => {
			session.clear();
			if (page.url.pathname !== loginPath) void goto(loginPath);
		});

		void (async () => {
			try {
				const state = await session.refresh();
				if (!state.authenticated && page.url.pathname !== loginPath) {
					await goto(loginPath);
				}
			} catch (error) {
				unreachable = error instanceof ApiError ? error.message : String(error);
			}
		})();

		return () => onUnauthorized(undefined);
	});

	async function signOut() {
		signingOut = true;
		try {
			await logout();
		} catch {
			// A logout that fails still ends this browser's session as far as the
			// shell is concerned; the server-side row expires on its own.
		} finally {
			session.clear();
			signingOut = false;
			await goto(loginPath);
		}
	}
</script>

<header class="app-header">
	<h1 class="app-title">UsArr</h1>
	{#if session.authenticated}
		<nav class="app-nav" aria-label="Main">
			<a href={resolve('/')} aria-current={page.url.pathname === '/' ? 'page' : undefined}>Home</a>
			<a
				href={resolve('/search')}
				aria-current={page.url.pathname === resolve('/search') ? 'page' : undefined}>Search</a
			>
		</nav>
		<div class="app-account">
			{#if session.current.username}<span class="app-user">{session.current.username}</span>{/if}
			<button type="button" onclick={signOut} disabled={signingOut}>
				{signingOut ? 'Signing out' : 'Sign out'}
			</button>
		</div>
	{/if}
</header>

<main>
	{#if unreachable}
		<h2>UsArr cannot reach its own backend</h2>
		<div class="banner banner-error" role="alert">
			<code>/api/v1/auth/session</code> did not answer, so the shell cannot tell whether you are
			signed in. This page is served from the embedded build and loads without the API; nothing else
			will work until the backend answers.
			<p class="banner-detail">{unreachable}</p>
		</div>
	{:else if !session.loaded}
		<p class="empty">Checking your session.</p>
	{:else if !session.authenticated && !onLoginRoute}
		<p class="empty">Not signed in. Taking you to the sign-in page.</p>
	{:else}
		{@render children()}
	{/if}
</main>

<style>
	.app-account {
		display: flex;
		align-items: center;
		gap: var(--space-5);
		margin-left: auto;
	}

	.app-user {
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}
</style>
