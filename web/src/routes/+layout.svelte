<script lang="ts">
	/**
	 * The application shell: sidebar · toolbar · content, and nothing else
	 * (DESIGN-DIRECTION §8.3).
	 *
	 * TWO AXES, HELD APART (ADR-0027). Media type is the NAVIGATION axis — six
	 * entries, bounded at six by construction, each one a real place at
	 * `/library/{type}`. A library is a SCOPE, not a place: carried in the URL as
	 * `?lib=` on the routes that already exist, never a nav list.
	 *
	 * ⚠️ ALL SIX TYPES ARE SHOWN, INCLUDING THE ONES THIS INSTALL HAS NOTHING IN,
	 * AND THAT IS A DELIBERATE DEPARTURE FROM §17.2'S DATA-DRIVEN RULE RATHER
	 * THAN AN OVERSIGHT. §17.2 and DESIGN-DIRECTION §8.1 both want a type the
	 * user does not have hidden entirely, and doing that honestly needs a per-type
	 * COUNT BESIDE THESE CHIPS. `docs/reference/http-api.md` §7.1 states that
	 * there are *"no facet counts beside the chips; each is its own aggregate and
	 * its own read"*, and this header used to say the rows were absent because
	 * nothing could drive them. Six one-row probes on every navigation is exactly
	 * the render-path cost principle 1 exists to refuse, and hiding a type on a
	 * count nobody measured would hide a library that is really there — the worse
	 * of the two failures, because it is silent. So all six ship and an empty type
	 * says so on its own screen, where the words can be true.
	 *
	 * ⚠️ THIS ENDED *"The rule comes back the day a facet count does"*, AND ONE
	 * LANDED WITHOUT BRINGING IT BACK. `GET /api/v1/library/facets` ships six
	 * counts and Home's Block A is drawn off them, so the premise "there is none
	 * on the wire" is dead — but §7.1's rule is about a count BESIDE A CHIP, on a
	 * component that renders on every navigation, and ADR-0053's reopening
	 * condition is not this number: ADR-0059 refined it to an independent EXISTS
	 * over `edition.format`. So NOTHING HERE CHANGES. All six entries still ship,
	 * unconditionally, and none of them carries a number.
	 *
	 * THE SCOPE CHIP IS STILL NOT HERE. It renders nothing at 0 or 1 library,
	 * exactly as Navidrome's LibrarySelector returns null, so at zero libraries
	 * the correct rendering is no control. `?lib=` is honoured by the screens that
	 * read it; the chip that WRITES it is a separate commit and is not faked here.
	 *
	 * The entry set is Home · the six types · Library · Search · Requests ·
	 * Services · Libraries · Settings, grouped as the mockup groups them: content
	 * nouns first, configuration last. `Library` heads the second content
	 * group because it is the one view that is not per-type: one unified table
	 * across all six, which is the six rows above it with the filter taken off.
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
	import type { ResolvedPathname } from '$app/types';
	import { ApiError, logout, onUnauthorized } from '$lib/api';
	import { MEDIA_TYPES, mediaTypeLabel } from '$lib/library';
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
	const searchPath = resolve('/search');
	const onLoginRoute = $derived(page.url.pathname === loginPath);

	/**
	 * The sidebar entries, in four groups. Settings is last, which is the shape
	 * Sonarr, Radarr and Prowlarr already trained self-hosters on.
	 *
	 * `id` is the route id and `href` is the resolved path. They are two fields
	 * because the media-type rows are SIX entries over ONE route — `/library/
	 * [type]` — so the route id no longer identifies the row, and `resolve` needs
	 * the parameter to build a real link. Every entry keeps a real `href`, which
	 * is what makes middle-click, Ctrl-click and "copy link address" work; a
	 * click handler that called `goto` would look identical and break all three.
	 *
	 * `href` is typed `ResolvedPathname` rather than `string`, and that is what
	 * keeps `svelte/no-navigation-without-resolve` live over these links. The rule
	 * cannot see a `resolve()` call through an array literal, so a `string` here
	 * would need the rule suppressed at the one `<a>` that draws every entry —
	 * which would suppress it for the whole sidebar. The type is only inhabited by
	 * `resolve()`'s own return, so it holds the same property the rule does.
	 */
	type NavItem = { id: string; label: string; href: ResolvedPathname };

	/** ⚠️ ALL SIX, NOT THE ONES THIS INSTALL HAS. The header carries the whole
	 * argument: §7.1 puts no facet count beside a chip, and hiding a type on a
	 * count nobody measured hides a library that is really there. ⚠️ THIS SAID
	 * *"there is no facet count on the wire"*, WHICH IS NO LONGER TRUE — Home's
	 * Block A is drawn off `GET /api/v1/library/facets` — and it does not change
	 * this list; the header says why. The order is `$lib/library`'s
	 * `MEDIA_TYPES`, which is §17.2's own. */
	const TYPE_NAV: NavItem[] = MEDIA_TYPES.map((type) => ({
		id: `/library/${type}`,
		label: mediaTypeLabel(type),
		href: resolve('/library/[type]', { type })
	}));

	const NAV_GROUPS: NavItem[][] = [
		[{ id: '/', label: 'Home', href: resolve('/') }],
		TYPE_NAV,
		[
			/**
			 * ⚠️ `Library`, AND IT USED TO BE `Recently added`. The old label was
			 * right and the screen moved out from under it: `/library` renders the
			 * catalogue across every media type in one of the orders the browse
			 * endpoint serves, inside a `?lib=` scope, so a view a user has sorted by
			 * `popularity` is not a recently-added list and calling it one would be a
			 * label that contradicts what is on screen. It is the unfiltered parent of
			 * the six type rows above it, which is what the word now names.
			 *
			 * ⚠️ AND IT SITS FOUR ROWS ABOVE `Libraries`, WHICH IS A DIFFERENT NOUN.
			 * §17.2's own axes table assigns the singular to the SCOPE — *"Library | a
			 * user-defined grouping (§6.5)"* — so the collision is real and is
			 * recorded here rather than glossed. It is accepted rather than resolved:
			 * the two are in different nav GROUPS (content nouns versus
			 * configuration), the alternative label is now false rather than merely
			 * ambiguous, and a false label is worse than an ambiguous one. If it ever
			 * reads badly in use, the fix is a third word for this screen and not a
			 * return to the old one.
			 */
			{ id: '/library', label: 'Library', href: resolve('/library') },
			{ id: '/search', label: 'Search', href: resolve('/search') },
			{ id: '/requests', label: 'Requests', href: resolve('/requests') }
		],
		[
			{ id: '/services', label: 'Services', href: resolve('/services') },
			{ id: '/libraries', label: 'Libraries', href: resolve('/libraries') },
			{ id: '/settings', label: 'Settings', href: resolve('/settings') }
		]
	];

	/**
	 * One string per route, read by the toolbar title, the h1 and the live
	 * region alike, so the three cannot disagree.
	 *
	 * The six type screens take their entry from the same list the nav does, so a
	 * seventh type could not arrive with a link and no title. `/login` is
	 * deliberately absent: it is two screens on one path and its name is state,
	 * not a constant. See `loginTitle` below.
	 */
	const TITLES = new Map<string, string>([
		[resolve('/'), 'Home'],
		...TYPE_NAV.map((item): [string, string] => [item.href, item.label]),
		[resolve('/library'), 'Library'],
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
	 * TRUE WHEN THE KEYBOARD BELONGS TO SOMETHING THAT TAKES TEXT, and the one
	 * precondition `/` has that a scroll key does not.
	 *
	 * `focusIsUnclaimed` ($lib/shellscroll) is the wrong test here and the
	 * difference is the whole point: it is true ONLY on `<body>`, because a
	 * scroll key must never be second-guessed away from a control that would
	 * have handled it. §17.4 wants Search *reachable from every screen*, so `/`
	 * has to keep working with focus on a nav link, a button or an `<h1>` — the
	 * places focus actually sits after one Tab or one navigation. The test is
	 * therefore the narrow one: is this element eating characters?
	 *
	 * EVERY `<input>` COUNTS, INCLUDING A CHECKBOX, and that is deliberate. The
	 * alternative is an allowlist of the non-text `type=` values, which has to
	 * track the HTML spec forever to buy a shortcut on a control the user is
	 * about to press Space on anyway. `<select>` is in for a reason of its own:
	 * a closed select does typeahead, so a swallowed `/` is a swallowed jump.
	 *
	 * `isContentEditable` is read rather than the attribute, because it is the
	 * computed value — an editor's inner nodes inherit editability from an
	 * ancestor that carries the attribute, and the attribute test misses them.
	 */
	function focusTakesText(node: EventTarget | null): boolean {
		if (!(node instanceof HTMLElement)) return false;
		if (node.isContentEditable) return true;
		const tag = node.tagName;
		return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';
	}

	/**
	 * `/` OPENS SEARCH (ARCHITECTURE §17.4), and it opens the SCREEN.
	 *
	 * §17.4 specifies the key and the destination — Search is "reachable from
	 * every screen (and from `/` with a keyboard shortcut)" — and says nothing
	 * about focus. So this does exactly what clicking the Search nav row does
	 * and not one thing more: `goto`, and let `afterNavigate` put focus on the
	 * new screen's h1. IT DOES NOT FOCUS THE SEARCH FIELD, and that is the
	 * cautious reading rather than the conventional one. Jumping focus into the
	 * input would land the user PAST the skip link on a screen they have not
	 * seen yet, which is the SC 2.4.1 failure `afterNavigate` above already
	 * documents itself having fixed once. `reachable` is satisfied by arriving;
	 * spending the shell's one promise about focus order to save a Tab is not a
	 * trade this file gets to make twice.
	 *
	 * IT IS GATED ON `showNav`, not on the key alone. Search is reachable where
	 * the nav that reaches it is: signed out, on /login, or on the unreachable
	 * screen, `/` is an ordinary character and stays one.
	 *
	 * WHAT IT REFUSES TO SWALLOW:
	 *
	 *   · anything already claimed (`defaultPrevented`), on the same reasoning
	 *     as `scrollDeltaFor`;
	 *   · Ctrl, Alt and Meta combinations, which are browser and OS shortcuts —
	 *     Meta+/ and Ctrl+/ both mean something to somebody;
	 *   · a composition in progress, where the key belongs to the IME;
	 *   · a repeat, so holding the key is one navigation and not a history run;
	 *   · the field cases, via `focusTakesText`.
	 *
	 * ⚠️ SHIFT IS DELIBERATELY NOT IN THAT LIST, and it is the one modifier that
	 * would be wrong to exclude. On a US layout Shift+/ produces `?` and never
	 * reaches this; on the German and French layouts `/` IS a shifted key, so
	 * `event.key === '/'` arrives with `shiftKey` true and excluding it would
	 * delete the shortcut on those keyboards. The test is the character the
	 * layout produced, which is what `key` reports, not the keys pressed to
	 * produce it. This is the opposite call from `scrollDeltaFor`, which drops
	 * Shift because Shift is a selection modifier on a NAMED key; here the key
	 * is a character and Shift may be part of typing it.
	 *
	 * Already on Search, it returns false rather than preventing: there is
	 * nothing to navigate to, and swallowing the key to do nothing is worse
	 * than letting it through.
	 */
	function openSearchKey(event: KeyboardEvent): boolean {
		if (event.key !== '/' || event.defaultPrevented || event.repeat) return false;
		if (event.ctrlKey || event.altKey || event.metaKey || event.isComposing) return false;
		if (!showNav || page.url.pathname === searchPath) return false;
		if (focusTakesText(event.target)) return false;
		event.preventDefault();
		void goto(searchPath);
		return true;
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
	 *
	 * AND IT CARRIES `/`, §17.4's shortcut to Search, for the third time the
	 * same reason applies: a shortcut that has to work from every screen has to
	 * be listened for somewhere that is on every screen. `openSearchKey` runs
	 * FIRST and its precondition is narrower than the scroll keys' — see its own
	 * note — but the two cannot collide regardless, since `/` is not a scroll
	 * key and `scrollDeltaFor` returns null for it. It sits behind the drawer
	 * branch with everything else: while a modal layer is open, Escape is the
	 * only key this handler has.
	 */
	function onWindowKeydown(event: KeyboardEvent) {
		if (isDrawer) {
			if (event.key !== 'Escape') return;
			event.preventDefault();
			closeDrawer();
			return;
		}
		if (openSearchKey(event)) return;
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
								href={item.href}
								aria-current={page.url.pathname === item.href ? 'page' : undefined}
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
