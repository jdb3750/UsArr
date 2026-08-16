<script lang="ts">
	/**
	 * Setup and sign-in, on one route.
	 *
	 * The server decides which of the two this is: GET /api/v1/auth/session
	 * reports setup_required until an owner exists, and POST /api/v1/auth/setup
	 * closes permanently the moment one does — a second call answers 409
	 * already_setup. So the screen asks first, and it also handles being wrong:
	 * if setup 409s because another tab got there first, it switches to signing
	 * in and says so rather than showing an error the user cannot act on.
	 *
	 * Both posts go through $lib/api's postJson, which is what supplies the
	 * Content-Type and the X-CSRF-Token that csrfProtected demands.
	 */
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, fetchSession, login, setupOwner } from '$lib/api';
	import { session } from '$lib/session.svelte';

	// The minimum internal/httpapi/auth.go enforces on the owner password.
	const MIN_PASSWORD = 12;

	let mode = $state<'asking' | 'setup' | 'login'>('asking');
	let username = $state('');
	let password = $state('');
	let busy = $state(false);
	let problem = $state('');
	let notice = $state('');
	let unreachable = $state('');

	const canSubmit = $derived(
		!busy && username.trim() !== '' && password !== '' && mode !== 'asking'
	);

	/**
	 * What this screen is called, computed from the same session state the shell
	 * computes its title from, so the tab and the toolbar say one thing. While
	 * the bootstrap is still asking, the shell has already resolved the session
	 * — the layout renders no child until it has — so this is right from the
	 * first paint rather than settling a beat later.
	 */
	const screenName = $derived(
		session.current.setupRequired ? 'Create the owner account' : 'Sign in'
	);

	onMount(async () => {
		try {
			const state = await fetchSession();
			session.set(state);
			mode = state.setupRequired ? 'setup' : 'login';
			if (state.authenticated) await goto(resolve('/'));
		} catch (error) {
			unreachable = error instanceof ApiError ? error.message : String(error);
		}
	});

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!canSubmit) return;
		busy = true;
		problem = '';
		notice = '';
		try {
			const state =
				mode === 'setup'
					? await setupOwner(username.trim(), password)
					: await login(username.trim(), password);
			session.set(state);
			password = '';
			await goto(resolve('/'));
		} catch (error) {
			if (error instanceof ApiError && error.status === 409 && mode === 'setup') {
				// Someone completed setup between the bootstrap call and this submit.
				// The shared session state carries the screen's name (the shell reads
				// setupRequired for the toolbar title, the h1 and the tab), so it is
				// corrected here too rather than left one refresh behind.
				mode = 'login';
				session.set({ ...session.current, setupRequired: false });
				notice = 'An owner account already exists on this install. Sign in instead.';
			} else {
				problem = error instanceof ApiError ? error.message : String(error);
			}
			password = '';
		} finally {
			busy = false;
		}
	}
</script>

<!--
	The tab reads what the screen is, not what the route is called. On a fresh
	install this is owner creation, and `setup_required` from the server is the
	one thing that decides it — the same value the shell reads for the toolbar
	title and the h1, so the three cannot drift apart. The visible heading is
	the toolbar's, which is where this shell puts every page title
	(DESIGN-DIRECTION §8.3); repeating it as an h2 here printed the page name
	twice and put two identical headings in one document.
-->
<svelte:head><title>{screenName} — UsArr</title></svelte:head>

{#if unreachable}
	<div class="banner banner-error" role="alert">
		The backend did not answer <code>/api/v1/auth/session</code>, so there is nothing to sign in to
		yet. Nothing is wrong with your account; the server is not reachable.
		<p class="banner-detail">{unreachable}</p>
	</div>
{:else if mode === 'asking'}
	<p class="empty">Checking whether this install has an owner account.</p>
{:else}
	{#if mode === 'setup'}
		<p>
			This install has no owner yet. The account you create here is the one administrator, and this
			screen closes permanently once it exists.
		</p>
	{:else}
		<p>UsArr needs a session before it can show your library, search, or grab anything.</p>
	{/if}

	{#if notice}
		<div class="banner banner-warn">{notice}</div>
	{/if}

	{#if problem}
		<div class="banner banner-error" role="alert">
			{mode === 'setup' ? 'The owner account was not created.' : 'That did not sign you in.'}
			<p class="banner-detail">{problem}</p>
		</div>
	{/if}

	<form class="auth-form" onsubmit={submit}>
		<div class="field">
			<label for="username">Username</label>
			<input
				id="username"
				name="username"
				type="text"
				autocomplete="username"
				required
				bind:value={username}
			/>
		</div>
		<div class="field">
			<label for="password">Password</label>
			<input
				id="password"
				name="password"
				type="password"
				autocomplete={mode === 'setup' ? 'new-password' : 'current-password'}
				minlength={mode === 'setup' ? MIN_PASSWORD : undefined}
				required
				bind:value={password}
			/>
			{#if mode === 'setup'}
				<p class="field-hint">At least {MIN_PASSWORD} characters.</p>
			{/if}
		</div>
		<div class="field">
			<button type="submit" disabled={!canSubmit}>
				{#if busy}
					{mode === 'setup' ? 'Creating' : 'Signing in'}
				{:else}
					{mode === 'setup' ? 'Create owner account' : 'Sign in'}
				{/if}
			</button>
		</div>
	</form>
{/if}

<style>
	.auth-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		max-width: 360px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.field label {
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		color: var(--fg-secondary);
	}

	.field-hint {
		margin: 0;
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}
</style>
