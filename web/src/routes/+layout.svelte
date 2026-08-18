<script lang="ts">
	/**
	 * The application shell: sidebar · toolbar · content, and nothing else
	 * (DESIGN-DIRECTION §8.3).
	 *
	 * TWO AXES, HELD APART (ADR-0027). Media type is the NAVIGATION axis — one
	 * sidebar entry per type that has content, bounded at six by construction.
	 * A library is a SCOPE, not a place: a multi-select chip above the nav,
	 * carried in the URL as `?lib=`, never a nav list. Neither is rendered yet
	 * and both are deliberately absent rather than faked:
	 *
	 *   - Media-type rows are data-driven and there is no library data to drive
	 *     them. ARCHITECTURE §17.2's hard rule is that a type the user does not
	 *     have is not shown AT ALL, so six placeholder rows would break that rule
	 *     in the one place it is most visible.
	 *   - The scope chip renders nothing at 0 or 1 library, exactly as
	 *     Navidrome's LibrarySelector returns null. At zero libraries the correct
	 *     rendering is no control, so the seam is here and the widget is not.
	 *
	 * What ships is the fixed entry set — Home · Search · Requests · Services ·
	 * Libraries · Settings — grouped as the mockup groups them: content nouns
	 * first, configuration last.
	 *
	 * THE SESSION GUARD IS UNCHANGED. Every /api/v1 route except the auth
	 * bootstrap sits behind `authenticated` (internal/httpapi/server.go), so the
	 * layout resolves the session ONCE before any child route runs its own
	 * fetches, and registers the 401 hook so a session that expires mid-visit
	 * lands on the sign-in page rather than on a page of error banners.
	 *
	 * NO ANIMATION ON NAVIGATION, and no startViewTransition. A view transition
	 * inserts a rendering pause plus an animation between the click and the
	 * usable screen; when the data is local and the render is ~8 ms that makes
	 * the application measurably slower in exchange for looking designed.
	 */
	import '../app.css';
	// Imported for its side effect: reading the stored theme and density and
	// stamping data-theme / data-density on <html> before anything renders. The
	// CSP forbids the inline <script> that would normally do this in app.html,
	// so it happens here, at module evaluation, which is still before first
	// paint on an SPA whose served document has an empty body.
	import '$lib/prefs.svelte';
	import { onMount, tick } from 'svelte';
	import { afterNavigate, beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { ApiError, logout, onUnauthorized } from '$lib/api';
	import { session } from '$lib/session.svelte';
	import { createScrollMemory, forwardScrollKey, type ScrollMemory } from '$lib/shellscroll';

	let { children } = $props();

	let unreachable = $state('');
	let signingOut = $state(false);

	/** True while the viewport is narrow enough that the sidebar is a drawer. */
	let narrow = $state(false);
	/** Only meaningful while `narrow`: a wide viewport has no drawer to open. */
	let drawerOpen = $state(false);

	let announcement = $state('');
	/**
	 * False until the session bootstrap has answered and any redirect it causes
	 * has settled. Everything up to that point is still ARRIVAL, however many
	 * client-side navigations it takes to get there.
	 */
	let arrived = $state(false);
	let headingEl = $state<HTMLHeadingElement | null>(null);
	let toggleEl = $state<HTMLButtonElement | null>(null);
	let navEl = $state<HTMLElement | null>(null);
	/**
	 * The application's scroll container and the box inside it whose height
	 * changes as a screen's data arrives. Both belong to `$lib/shellscroll`, one
	 * as the thing that scrolls and one as the signal that there is finally
	 * something to scroll. Plain `let`, not `$state`: nothing rendered reads
	 * them, so making them reactive would only cost an invalidation.
	 */
	let mainEl: HTMLElement | null = null;
	let mainInnerEl: HTMLElement | null = null;
	let scrollMemory: ScrollMemory | null = null;

	const loginPath = resolve('/login');
	const onLoginRoute = $derived(page.url.pathname === loginPath);

	/**
	 * The fixed sidebar entries, in three groups. The media types belong between
	 * groups one and two once there is anything to count. Settings is last,
	 * which is the shape Sonarr, Radarr and Prowlarr already trained
	 * self-hosters on.
	 */
	type NavRoute = '/' | '/search' | '/requests' | '/services' | '/libraries' | '/settings';

	const NAV_GROUPS: { id: NavRoute; label: string }[][] = [
		[{ id: '/', label: 'Home' }],
		[
			{ id: '/search', label: 'Search' },
			{ id: '/requests', label: 'Requests' }
		],
		[
			{ id: '/services', label: 'Services' },
			{ id: '/libraries', label: 'Libraries' },
			{ id: '/settings', label: 'Settings' }
		]
	];

	/**
	 * One string per route, read by the toolbar title, the h1 and the live
	 * region alike, so the three cannot disagree.
	 *
	 * /login is deliberately absent: it is two screens on one path and its name
	 * is state, not a constant. See `loginTitle` below.
	 */
	const TITLES = new Map<string, string>([
		[resolve('/'), 'Home'],
		[resolve('/search'), 'Search'],
		[resolve('/requests'), 'Requests'],
		[resolve('/services'), 'Services'],
		[resolve('/libraries'), 'Libraries'],
		[resolve('/settings'), 'Settings']
	]);

	/**
	 * The screens still rendering against the scaffolding class vocabulary that
	 * app.css's TRANSITIONAL block carries. The set shrinks to nothing as each
	 * one is rebuilt, and the block goes with it.
	 */
	const TRANSITIONAL = new Set<string>([resolve('/login')]);

	/**
	 * /login is setup on a fresh install and sign-in on every install after it,
	 * and the server decides which — `setup_required` on GET /auth/session, held
	 * in `session` and updated by the sign-in screen when it learns otherwise.
	 * Naming it "Sign in" unconditionally meant a fresh install read "Sign in"
	 * in the toolbar, in the h1 and in the tab, over a body that said "Create
	 * the owner account". The screen has one name and this is where it is
	 * computed, so the toolbar, the h1 and the live region cannot disagree.
	 */
	const loginTitle = $derived(
		session.current.setupRequired ? 'Create the owner account' : 'Sign in'
	);
	const title = $derived(onLoginRoute ? loginTitle : (TITLES.get(page.url.pathname) ?? 'UsArr'));
	const showNav = $derived(session.authenticated && !onLoginRoute && !unreachable);
	const isDrawer = $derived(showNav && narrow && drawerOpen);
	const sidebarState = $derived(!showNav ? 'collapsed' : drawerOpen ? 'open' : 'closed');
	const transitional = $derived(TRANSITIONAL.has(page.url.pathname));

	onMount(() => {
		onUnauthorized(() => {
			session.clear();
			if (page.url.pathname !== loginPath) void goto(loginPath);
		});

		// 900px is where the sidebar stops taking a column and starts overlaying,
		// and it is the same breakpoint app.css uses. matchMedia rather than a
		// resize handler, so nothing reads layout on every resize frame.
		const mq = window.matchMedia('(max-width: 900px)');
		const sync = () => {
			narrow = mq.matches;
			if (!narrow) drawerOpen = false;
		};
		sync();
		mq.addEventListener('change', sync);

		void (async () => {
			try {
				const state = await session.refresh();
				if (!state.authenticated && page.url.pathname !== loginPath) {
					await goto(loginPath);
				}
			} catch (error) {
				unreachable = error instanceof ApiError ? error.message : String(error);
			} finally {
				// Only now is the user looking at the screen they arrived on, so
				// only now does a navigation mean they asked for one.
				arrived = true;
			}
		})();

		// Constructed here rather than at module scope: it reads history.state and
		// sessionStorage, neither of which exists until the browser does.
		scrollMemory = createScrollMemory(
			() => mainEl,
			() => mainInnerEl
		);

		return () => {
			onUnauthorized(undefined);
			mq.removeEventListener('change', sync);
			scrollMemory?.dispose();
			scrollMemory = null;
		};
	});

	/**
	 * SCROLL RESTORATION BELONGS TO THE REGION THAT SCROLLS ($lib/shellscroll).
	 *
	 * SvelteKit remembers a position per history entry and restores it with
	 * `scrollTo` on the window; the window is no longer the scroller, so every
	 * back-navigation landed at the top of the screen. The record is kept for
	 * `.main` instead, against the same history index SvelteKit keys its own by.
	 *
	 * `beforeNavigate` is the last moment the outgoing entry's offset is still
	 * readable, and `afterNavigate` is the first moment the incoming screen
	 * exists to be scrolled.
	 */
	beforeNavigate(() => scrollMemory?.remember());

	/**
	 * FOCUS FOLLOWS NAVIGATION (DESIGN-DIRECTION §11).
	 *
	 * Activating a nav link replaces the whole main region and leaves
	 * document.activeElement on the nav link with nothing announced, so a
	 * keyboard user has to tab past every remaining nav row to reach the screen
	 * they just opened. Focus moves to the new main's h1 instead, and the page
	 * name goes to a polite live region.
	 *
	 * `enter` is the initial load, where focus belongs wherever the browser put
	 * it — moving it there would fight the browser rather than help.
	 *
	 * AND SO IS AN ARRIVAL REDIRECT, which is the case this used to get wrong.
	 * `/` is not a screen for a signed-out user: the bootstrap resolves the
	 * session and sends them to /login, and that lands as a `goto` rather than
	 * an `enter`. Focusing the h1 there put the user PAST the skip link on the
	 * very first screen they see — the skip link measured as the fourth tab
	 * stop, behind both password fields — which is SC 2.4.1, Level A, failing
	 * on arrival. So until `arrived` flips, focus is left where SvelteKit's own
	 * navigation reset put it, which is the top of the document, and the skip
	 * link is the first thing Tab reaches. After that every navigation is one
	 * the user asked for, and the heading is the right place to send them:
	 * they are already past the nav they would have skipped.
	 *
	 * The announcement is skipped with the focus move, deliberately. A live
	 * region firing before the user has done anything announces a page they did
	 * not navigate to.
	 */
	afterNavigate(async (nav) => {
		if (narrow) drawerOpen = false;
		const movesFocus = nav.type !== 'enter' && arrived;
		await tick();
		if (movesFocus) {
			// `preventScroll` because the offset is decided one line below and not
			// by a side effect of focusing. The h1 is a 1px clipped box at the very
			// top of the region, so focusing it scrolls the region to the top — on a
			// forward navigation that is the right answer by luck, and on a back
			// navigation it is exactly the wrong one, undoing the restore. Focus
			// still lands on the heading; only its scrolling is declined.
			headingEl?.focus({ preventScroll: true });
			announcement = title;
		}
		scrollMemory?.settle(nav.type === 'popstate');
	});

	/**
	 * Opening a modal layer moves focus INTO it. Leaving focus on the toggle is
	 * what makes an `aria-modal` drawer a lie: everything behind it is inert, so
	 * a keyboard user standing outside it has one control and no way in except
	 * Tab, and a screen reader is still reading the toolbar.
	 */
	async function openDrawer() {
		drawerOpen = true;
		await tick();
		navEl?.querySelector<HTMLAnchorElement>('.nav__link')?.focus();
	}

	function closeDrawer() {
		if (!drawerOpen) return;
		drawerOpen = false;
		toggleEl?.focus();
	}

	/**
	 * Escape closes the drawer and returns focus to the control that opened it.
	 *
	 * The listener is on the window rather than on the drawer, and that is the
	 * fix for a measured bug: with it bound to the <nav>, pressing Escape while
	 * focus sat on the toggle — which is where focus is the instant the drawer
	 * opens, and the toggle is deliberately OUTSIDE the inert region so it can
	 * still close it — did nothing at all. A modal's dismiss key belongs to the
	 * modal, not to one subtree of it.
	 *
	 * IT ALSO CARRIES THE SCROLL KEYS, and that is the second half of moving the
	 * shell's scroll off the document ($lib/shellscroll). The browser gives a
	 * scroll key to the nearest scrollable ancestor of the FOCUSED element, and
	 * on the first paint the focused element is `<body>`, which no longer has
	 * one — so PageDown, the arrows, Home, End and Space did nothing until the
	 * first click, Tab or navigation. This supplies the ancestor the body lost,
	 * and does it WITHOUT MOVING FOCUS: the skip link stays the first tab stop.
	 *
	 * `forwardScrollKey` declines the moment focus is on anything real, so this
	 * is inert for the whole rest of the session; and it is not reached at all
	 * while the drawer is open, since the region behind a modal layer is inert
	 * and scrolling it would be scrolling something the user cannot see into.
	 */
	function onWindowKeydown(event: KeyboardEvent) {
		if (isDrawer) {
			if (event.key !== 'Escape') return;
			event.preventDefault();
			closeDrawer();
			return;
		}
		if (mainEl) forwardScrollKey(event, mainEl);
	}

	/**
	 * The skip link moves focus rather than relying on the fragment. Engines
	 * differ on whether a fragment navigation focuses a tabindex="-1" target,
	 * and this is SC 2.4.1 rather than a nicety. The href stays real so the link
	 * still works with scripting unavailable and still middle-clicks.
	 */
	function skipToContent(event: MouseEvent) {
		event.preventDefault();
		headingEl?.focus();
	}

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

<!--
	The first tab stop on every screen. A sidebar and a toolbar put a long,
	identical run of tab stops before the first row of every screen, and
	landmarks only help the people who have a screen reader. SC 2.4.1, Level A.
-->
<svelte:window onkeydown={onWindowKeydown} />

<a class="skip" href="#usarr-main" onclick={skipToContent}>Skip to content</a>

<div class="app" data-sidebar={sidebarState}>
	<header class="topbar">
		{#if showNav && narrow}
			<button
				bind:this={toggleEl}
				type="button"
				class="btn btn--sm sidebar-toggle"
				aria-expanded={drawerOpen}
				aria-controls="usarr-nav"
				onclick={drawerOpen ? closeDrawer : openDrawer}
			>
				{drawerOpen ? 'Hide navigation' : 'Show navigation'}
			</button>
		{/if}

		<!--
			While the drawer is open everything behind it is inert, which is what
			makes it a real modal layer rather than a panel that happens to sit on
			top: focus cannot reach the toolbar or the content underneath, and the
			only ways out are the toggle, the scrim and Escape (§9.4).
		-->
		<a class="topbar__brand" href={resolve('/')} inert={isDrawer}>UsArr</a>

		<!--
			The page title, at 20px, which is the largest type anywhere in the
			application. It is aria-hidden because the same string is the h1 inside
			main a few lines down; without that, the page name is in the
			accessibility tree twice, once as furniture and once as a heading.
		-->
		<span class="topbar__title" aria-hidden="true">{title}</span>

		<span class="topbar__spacer"></span>

		{#if session.authenticated}
			<div class="topbar__right" inert={isDrawer}>
				<!--
					THE USERNAME IS A LABEL, NOT CONTENT, so it truncates with an
					ellipsis and carries the whole string as a tooltip. It does not
					wrap: a name long enough to wrap grows the bar by a whole line, and
					before this it did not even do that — `.topbar` only wraps below
					560px, so a long name pushed the document sideways instead
					(measured: 175px of horizontal overflow on a 200px viewport).

					`.trunc` plus `title` is the pattern already in the tree for exactly
					this — release titles, indexer names, service addresses — rather
					than a second way of saying the same thing.
				-->
				{#if session.current.username}
					<span class="topbar__user trunc" title={session.current.username}
						>{session.current.username}</span
					>
				{/if}
				<button type="button" class="btn btn--sm" onclick={signOut} disabled={signingOut}>
					{signingOut ? 'Signing out' : 'Sign out'}
				</button>
			</div>
		{/if}
	</header>

	{#if showNav}
		<!--
			The scrim is a click target, not a control. Everything it does is also
			on the toggle and on Escape, both of which are keyboard-reachable, so it
			carries no role, no name and no tab stop.
		-->
		<div class="scrim" onclick={closeDrawer} aria-hidden="true"></div>

		<nav
			id="usarr-nav"
			class="sidebar"
			aria-label={isDrawer ? 'Navigation' : 'Main'}
			role={isDrawer ? 'dialog' : undefined}
			aria-modal={isDrawer ? 'true' : undefined}
			bind:this={navEl}
		>
			{#each NAV_GROUPS as group, groupIndex (groupIndex)}
				<ul class="nav__group">
					{#each group as item (item.id)}
						<li>
							<a
								class="nav__link"
								href={resolve(item.id)}
								aria-current={page.url.pathname === resolve(item.id) ? 'page' : undefined}
							>
								{item.label}
							</a>
						</li>
					{/each}
				</ul>
			{/each}
		</nav>
	{/if}

	<main class="main" id="usarr-main" tabindex="-1" inert={isDrawer} bind:this={mainEl}>
		<h1 class="sr pagetitle" bind:this={headingEl} tabindex="-1">{title}</h1>

		<!--
			.main is the application's scroll container, and a sticky element pins
			inside its scroll container's PADDING rather than against its border
			box — so the region's padding lives one box in, on .main__inner, and the
			sticky table header pins flush under the top bar. app.css carries the
			measurement. The h1 stays outside this box because
			`.main:has(> .pagetitle:focus)` reads it as a direct child of .main.
		-->
		<div class="main__inner" bind:this={mainInnerEl}>
			{#if unreachable}
				<div class="section">
					<div class="banner banner--err" role="alert">
						<div class="banner__body">
							<div class="banner__title">UsArr cannot reach its own backend</div>
							<div class="banner__text">
								<code class="mono">/api/v1/auth/session</code> did not answer, so the shell cannot tell
								whether you are signed in. This page is served from the embedded build and loads without
								the API; nothing else works until the backend answers.
							</div>
							<p class="verbatim shell-verbatim">{unreachable}</p>
						</div>
					</div>
				</div>
			{:else if !session.loaded}
				<div class="section"><p class="empty">Checking your session.</p></div>
			{:else if !session.authenticated && !onLoginRoute}
				<div class="section">
					<p class="empty">Not signed in. Taking you to the sign-in page.</p>
				</div>
			{:else}
				<div class:transitional-scope={transitional}>
					{@render children()}
				</div>
			{/if}
		</div>
	</main>
</div>

<!--
	One live region for the whole shell: atomic, carrying the whole string rather
	than a fragment of one, and never nested inside another live region.
-->
<div class="sr" role="status" aria-live="polite" aria-atomic="true">{announcement}</div>

<style>
	/*
	 * Shell-only rules. Anything with a design-system name lives in app.css;
	 * what is here belongs to this component and to nothing else.
	 */
	/* Colour and size only. The nowrap, the overflow and the ellipsis all come
	 * from `.trunc` in app.css, so the truncation here is the same truncation
	 * every other label in the application gets rather than a local copy of it. */
	.topbar__user {
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.shell-verbatim {
		margin-top: var(--space-3);
	}
</style>
