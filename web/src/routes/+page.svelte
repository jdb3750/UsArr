<script lang="ts">
	/**
	 * HOME — ARCHITECTURE.md §17.2 as amended by ADR-0028, plus §17.7's first-run
	 * and error states. DESIGN-DIRECTION §8.4, §9.6, §10.
	 *
	 * ADR-0028 fixes Home at three blocks whose combined height is O(1) in the
	 * number of media types:
	 *
	 *   Block A   media-type summary    ≤6 rows          NOT DRAWN — no source
	 *   Block B   attention             hidden when empty      DRAWN
	 *   Block C   recently added        one unified table      NOT DRAWN — no source
	 *
	 * AND TWO THINGS THAT ARE NOT BLOCKS, added because the three above leave
	 * this screen with nothing to do on the install the owner actually has:
	 *
	 *   Search      a release-search entry point, drawn when an indexer exists
	 *   Recent grabs  GET /api/v1/grabs/recent, hidden when empty
	 *
	 * ⚠️ RECENT GRABS IS NOT BLOCK C AND MUST NEVER BE LABELLED AS IT. Block C
	 * is `Recently added` — one unified table over the catalogue, sorted by
	 * `added_at DESC`, with a Type column — and a grab is not an item that
	 * arrived. `$lib/requests`' own KNOWLEDGE_STOPS_NOTE is the whole reason:
	 * UsArr stops watching at the moment Prowlarr accepts the release, so it
	 * does not know whether a single byte followed. A list of grabs under the
	 * words "recently added" would assert exactly the thing UsArr has gone to
	 * some trouble not to claim. It occupies the SLOT Block C will occupy, under
	 * its own honest heading, and it vacates that slot when a catalogue exists.
	 *
	 * NEITHER IS A FOURTH BLOCK IN THE SENSE ADR-0028 BOUNDS. The decision fixes
	 * Home's height at O(1) in the number of MEDIA TYPES, which is what six
	 * per-type strips broke; both of these are single regions whose height is
	 * independent of how many types exist, and the search entry point is not a
	 * region at all — it is one control inside the state block that was already
	 * drawn here, replacing the `Search indexers` button that stood in the same
	 * place. Nothing was added beside the three blocks that could not be put
	 * inside something already on the screen.
	 *
	 * WHY TWO OF THE THREE ARE ABSENT, AND WHY THAT IS THE CORRECT RENDERING
	 * RATHER THAN AN UNFINISHED ONE. The `work` / `edition` / `media_file`
	 * tables and the sync channels that fill them are unbuilt, so there is no
	 * catalogue: every count in Block A and every row in Block C would have to
	 * be invented. DESIGN-DIRECTION §9.6 closes that off in as many words —
	 * never fabricated data in a shipped product surface — and a zeroed table
	 * and a skeleton shimmer are the same fabrication with different
	 * punctuation. §17.2's own hard rule reaches the same place from the other
	 * side: a media type the user does not have is not shown AT ALL, not in
	 * Block A, not in the sidebar, not as a search group. With no catalogue,
	 * the honest number of media-type rows on this screen is zero, which is
	 * what `routes/+layout.svelte` already does with the sidebar.
	 *
	 * ⚠️ §17.2 DOES specify Block A's four sourceless rows for a v0.1 install —
	 * `Comics · no catalogue source · Kavita · after v0.1 · Add` — and that is
	 * NOT what is missing here. Those four rows are the shape for an install
	 * where the OTHER two rows carry real counts, which requires the Sonarr and
	 * Radarr catalogue sync §16 puts in v0.1 and which is not built. Rendering
	 * six rows of which six are `no catalogue source` is not §17.2's screen; it
	 * is a table with no data in it, and rule 13's own bound — the ban is on a
	 * region that says NOTHING — does not rescue a block whose every row says
	 * the same nothing. The block arrives with the first catalogue source, and
	 * §17.7's `partial` and `stale` states arrive with it, because both of them
	 * are statements about an import that cannot run yet.
	 *
	 * WHAT IS LEFT IS THE STATE THE OWNER IS ACTUALLY IN, and §8.5 names it
	 * rather than leaving it as an implicit empty app: Prowlarr configured, no
	 * library-bearing service, therefore SEARCH-AND-GRAB MODE. The mode is
	 * derived from the health response's `role` — §8.5's own test, "no
	 * configured instance advertises LibrarySync" — so the day a build accepts
	 * a Sonarr this screen changes without a line here being edited. See
	 * `$lib/home`, which holds the derivation so a test can read it.
	 *
	 * THE THREE STATES THAT ARE DRAWN, all three from real API data:
	 *
	 *   unconfigured     `setup_required`, or no services. §17.7: this goes to
	 *                    the first-run path and NEVER to an empty home page.
	 *   search-and-grab  services configured, none library-bearing (§8.5).
	 *   library          at least one library-bearing service. Unreachable in
	 *                    this build; see $lib/home for why it is still derived.
	 *
	 * AND THE FOUR THAT ARE NOT, each for the same reason. `partial` (an import
	 * in progress) and `stale` (an instance degraded, "showing cached data from
	 * 11:47") are both claims about a catalogue and a per-instance sync clock,
	 * and neither exists: §17.7's degraded banner is a sentence about how old
	 * the cached data is, and there is no cached data for it to be about. The
	 * unreachable-instance FACT is real and is reported, in Block B, where it
	 * has a source. `scope-empty` needs a library scope; there is no `library`
	 * table and no scope chip, and Navidrome's own discipline — which §17.2
	 * adopts — renders no chip at all below two libraries, so the state is
	 * unreachable rather than unimplemented. `filtered-empty` needs a filter.
	 *
	 * BLOCK B COMES FIRST, and §17.2 supplies the argument for it: "Block B is
	 * hidden when empty, so it costs nothing when nothing is wrong — which is
	 * exactly why it can go first." The rule is written for the phone fork,
	 * where a screen of counts above the block that reports a rejected API key
	 * is the wrong order; with no counts to scroll past it holds at every
	 * width, so there is one order rather than two.
	 *
	 * A PROBLEM IS STATED CANONICALLY ONCE PER SCREEN (§17.3). Block B says
	 * WHAT is wrong and links to that instance's row on Services; it does not
	 * grow a second copy of the button that fixes it. There is one place the
	 * fix is pressed.
	 *
	 * THAT RULE IS ALSO WHY THERE IS NO SERVICES SUMMARY HERE. "Show services
	 * only when something is wrong" is a block this screen already has: Block B
	 * IS that block, it is hidden when empty by ADR-0028, and its rows already
	 * carry the instance, UsArr's own word for the state, the upstream's
	 * verbatim text and a link to the row that owns the fix. A second region
	 * over the same predicate would be the green "all good" panel arriving by
	 * another door. Nothing was added for it.
	 *
	 * AND THE SAME RULE BOUNDS WHAT A RECENT-GRABS ROW MAY OFFER HERE: nothing.
	 * `Search again` and `Open Services` are actions §17.5 puts on the Requests
	 * block, which is the canonical record; Home shows the ten most recent as a
	 * summary and links to it. That is a deliberate narrowing rather than an
	 * omission — see the columns below, and the omitted `actions` prop on
	 * `$lib/RecentGrabs.svelte`, which is the whole of how it is expressed.
	 *
	 * THE CSP FORBIDS INLINE STYLE ATTRIBUTES, so nothing here writes one. The
	 * list primitive sets its custom properties through the CSSOM; everything
	 * else on this screen is a class.
	 */
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, fetchRecentGrabs, fetchServicesHealth, type RecentGrab } from '$lib/api';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import RecentGrabs from '$lib/RecentGrabs.svelte';
	import { type ListColumn } from '$lib/list';
	import {
		attention,
		hasIndexer,
		headline,
		homeMode,
		HOME_SEARCH_SCOPE_NOTE,
		type AttentionRow,
		type HomeMode
	} from '$lib/home';
	import { KNOWLEDGE_STOPS_NOTE, requestsSearchHref } from '$lib/requests';
	import { firstLine, rollupCount } from '$lib/services';

	/**
	 * Block B's columns, and they are NOT the Services screen's six.
	 *
	 * `State` is UsArr's own plain-language word, straight from `stateLabel()`
	 * so the two screens cannot drift. `What is wrong` carries the instance and
	 * the qualifying clause, with the upstream's verbatim text on the muted
	 * second line (§10's `error` row: the verbatim upstream text, never a
	 * paraphrase). `Action` is a link to the row that owns the fix.
	 *
	 * ⚠️ THE ACTION TRACK IS A FIXED RESERVE AND MUST NEVER BE CONTENT-SIZED.
	 * It read `minmax(max-content, auto)`, on the argument that a fixed action
	 * track shears the buttons attached to exactly the rows that are broken.
	 * ADR-0029 makes EVERY ROW ITS OWN GRID, so that argument does not hold: a
	 * content-sized track resolves against its own row's contents, and the
	 * header row's contents are the word "Action". Measured in Chromium at
	 * 1440 px with the Plex faces served, the track came out 56 px in the header
	 * and 130 px in every body row — so the whole body sat 74 px left of its own
	 * header, and `What is wrong` sat 22 px left of its own.
	 *
	 * THE RESERVE IS MEASURED, NOT CHOSEN. This cell has exactly ONE state — an
	 * unconditional `Open in Services` link, on every row, with no optional
	 * element to make one row disagree with the next. It measures 106.00 px with
	 * `document.fonts.check('600 13px "IBM Plex Sans"')` true and 115.23 px with
	 * it false; the Plex faces are `font-display: block`, so a build that cannot
	 * serve them renders on fallback metrics forever and the wider number is the
	 * one that has to fit. 115.23 + 2 × --row-pad-x (12 px at all three
	 * densities) = 139.23, plus one --space-4 of headroom rounded up to the next
	 * --space-4 = 152 px.
	 *
	 * AND IT DOES NOT GET `.cell-actions`, WHICH WAS CHECKED RATHER THAN
	 * ASSUMED. That class exists to give a cell holding SEVERAL controls somewhere
	 * to put the ones that do not fit. Forced to a 40 px reserve, this cell's
	 * lone anchor spilled 78 px past the cell edge and was clipped by
	 * `.tablewrap`'s `overflow-x: clip` — and every one of the Services screen's
	 * `.cell-actions` buttons spilled 75-149 px in exactly the same way, because
	 * `flex-wrap` cannot wrap a single item and `.btn` is `white-space: nowrap`.
	 * The class would change nothing here. What keeps this cell safe is that its
	 * label is a compile-time constant, so the reserve above is its widest state
	 * by construction. A second control added to this cell needs the class.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'state', header: 'State', width: 'minmax(0, 1.1fr)' },
		{ id: 'what', header: 'What is wrong', width: 'minmax(0, 2.6fr)' },
		{ id: 'action', header: 'Action', width: '152px' }
	];

	/**
	 * A Block B row is two lines — the instance and its qualifier, then the
	 * upstream's own text — so `contain-intrinsic-size` gets a two-line value.
	 * ROW_INTRINSIC's default is measured on a ONE-line row and would be wrong
	 * by half, which shows as scroll-height jitter. Same shape and therefore
	 * the same number as the Services table's rows.
	 */
	const ROW_INTRINSIC_ATTENTION = 44;

	/**
	 * RECENT GRABS ON HOME IS THREE COLUMNS, NOT THE REQUESTS BLOCK'S SIX, and
	 * the three that are dropped were dropped for a stated reason each rather
	 * than to make the table fit.
	 *
	 *   Indexer   identifies WHICH tracker answered, which is what you need when
	 *             you are deciding whether to trust or re-run a result. Home is
	 *             not where that decision gets made.
	 *   Protocol  usenet or torrent, read at the same moment and for the same
	 *             purpose as Indexer.
	 *   Size      the one figure on the row that is about the download rather
	 *             than about the grab, and the column §9.1 gives a reserved
	 *             `.unit` box and a 3ch unit slot to. Carrying that machinery for
	 *             a summary would be the copy this file is trying not to make.
	 *
	 * What survives is the three that answer Home's question — "what did I send,
	 * and did it get sent" — which is `when`, `release` and `outcome`. §17.4's
	 * slot-order invariant holds across the two: identity first, then the
	 * verdict, in the same places.
	 *
	 * ⚠️ EVERY TRACK IS FIXED OR `fr`, WHICH THE DEV GUARD IN `gridTemplate()`
	 * ENFORCES. ADR-0029 makes every row its own grid, so a content-sized track
	 * resolves against its own row's contents and the columns cannot line up —
	 * the failure Block B's `Action` track documents in detail above. `132px` is
	 * the Requests block's own Time width, and it is the same content: an
	 * absolute time over a relative one.
	 */
	const GRAB_COLUMNS: ListColumn[] = [
		{ id: 'when', header: 'Time', width: '132px' },
		{ id: 'release', header: 'Release', width: 'minmax(0, 3fr)' },
		{ id: 'outcome', header: 'Outcome', width: 'minmax(0, 1.9fr)' }
	];

	/** §17.5's own limit for the block, and the endpoint's default. */
	const RECENT_LIMIT = 10;

	const servicesPath = resolve('/services');
	const requestsPath = resolve('/requests');

	let mode = $state<HomeMode | undefined>(undefined);
	let services = $state(0);
	let indexers = $state(false);
	let rows = $state<AttentionRow[]>([]);
	let loadError = $state('');

	/**
	 * WHAT THE USER TYPES, AND IT NEVER LEAVES THIS SCREEN AS A QUERY. Home does
	 * not search: it navigates. `submit()` hands the string to Requests through
	 * `?q=`, which is the parameter that screen's `onMount` already reads.
	 */
	let query = $state('');

	/**
	 * Recent grabs. `grabsLoaded` is separate from `grabs.length` on purpose —
	 * an empty list and an unread list mean opposite things, and the Requests
	 * block draws the same distinction for the same reason. Before the read
	 * lands, neither the section nor an empty state is drawn.
	 */
	let grabs = $state<RecentGrab[]>([]);
	let grabsLoaded = $state(false);
	let grabsError = $state('');

	/**
	 * Re-read on a timer so the relative clauses stay true rather than freezing
	 * at whatever they said on arrival. `paused` carries "retrying 14:19, in 4
	 * minutes", and a number that goes on saying "in 4 minutes" for an hour is
	 * a worse answer than no number. This is a local SQLite read behind the
	 * endpoint, not an upstream call — principle 1 holds: no render path here
	 * blocks on a service.
	 */
	let now = $state(new Date());

	const count = $derived(rollupCount(rows));
	const meta = $derived(headline(mode, services, count));

	/**
	 * The href the submit button would follow with scripting off, and the one
	 * `submit()` navigates to with it on. Built from `resolve('/requests')` plus
	 * the one parameter that makes the link work — §17.4's mechanism, through
	 * `$lib/requests`' `requestsSearchHref` rather than a second copy of it.
	 */
	const searchHref = $derived(requestsSearchHref(requestsPath, query));

	async function load() {
		try {
			const health = await fetchServicesHealth();
			mode = homeMode(health);
			services = health.services.length;
			indexers = hasIndexer(health);
			rows = attention(health.services, now);
			loadError = '';
		} catch (error) {
			loadError = error instanceof ApiError ? error.detail : String(error);
		}
	}

	/**
	 * A LOCAL SQLITE READ, which is what lets Home make it at all. Principle 1
	 * forbids a render path that blocks on an *Arr; `GET /api/v1/grabs/recent`
	 * reads UsArr's own `provenance` and `audit_log` rows and touches no
	 * upstream, which `$lib/api` says in as many words at its declaration.
	 *
	 * It is deliberately NOT on the 60-second timer that re-reads health. That
	 * timer exists because Block B's relative clauses go stale — "retrying in 4
	 * minutes" is wrong a minute later — and a grab list has no such clause: the
	 * absolute time is fixed and the relative one is recomputed from `now`,
	 * which the timer already moves. Polling it would be a repeated read for a
	 * table that only changes when the user is on another screen.
	 */
	async function loadGrabs() {
		try {
			const recent = await fetchRecentGrabs(RECENT_LIMIT);
			grabs = recent.grabs;
			grabsError = '';
		} catch (error) {
			grabsError = error instanceof ApiError ? error.detail : String(error);
		} finally {
			grabsLoaded = true;
		}
	}

	/**
	 * Enter or the button. `goto` rather than a full navigation, because the
	 * destination is a route in this app and reloading the shell to reach it
	 * would throw away the SPA for no gain.
	 *
	 * ⚠️ THE NATIVE SUBMIT IS PREVENTED ONLY BECAUSE THE `<a>` BESIDE IT IS
	 * REAL. The button's own `formaction` is the same href, so a build with
	 * scripting off still reaches Requests with the query on it; this handler
	 * replaces a full page load with a client navigation and nothing else. An
	 * empty query goes to `/requests` bare, which is what the `Search indexers`
	 * button on this screen has always done.
	 */
	function submit(event: SubmitEvent) {
		event.preventDefault();
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- a resolve()'d path plus ?q=, which a ResolvedPathname cannot carry; see $lib/requests requestsSearchHref
		void goto(searchHref);
	}

	onMount(() => {
		void load();
		void loadGrabs();
		const timer = setInterval(() => {
			now = new Date();
			void load();
		}, 60_000);
		return () => clearInterval(timer);
	});
</script>

<svelte:head><title>Home · UsArr</title></svelte:head>

<!--
	No <h2> restating the toolbar title. The shell already renders `Home` in the
	top bar and as the page's h1; a second one would cost a density row and make
	the heading structure claim a section boundary that does not exist.

	The meta line is DERIVED. A constant here is the failure the mockup records:
	a fixed "Last delta sync 14:02, 6 minutes ago" sat above "No services
	configured" on the very first screen a new user saw, first in reading order
	and first in the accessibility tree.
-->
<div class="pagehead">
	<span class="pagehead__meta">{meta}</span>
</div>

{#if loadError}
	<div class="section">
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">UsArr could not read what is connected</div>
				<div class="banner__text">
					<code class="mono">/api/v1/services/health</code> did not answer, so this screen cannot say
					what is configured or whether anything is broken.
				</div>
				<p class="verbatim">{loadError}</p>
			</div>
		</div>
	</div>
{/if}

<!--
	BLOCK B. Hidden entirely when empty (§17.2, ADR-0028) — the green "all good"
	panel is the thing it must never become — which is why this is an {#if} on
	the row count and not a List with an empty state.
-->
{#if rows.length > 0}
	<section class="section" id="home-attention">
		<div class="section__head">
			<h2>Needs attention</h2>
			<span class="section__count">{count}</span>
		</div>
		<List
			label="Services needing attention"
			columns={COLUMNS}
			{rows}
			key={(row) => String(row.id)}
			total={rows.length}
			rowIntrinsic={ROW_INTRINSIC_ATTENTION}
			stack="labels"
			{cell}
		/>
	</section>
{/if}

{#snippet cell(row: AttentionRow, column: ListColumn)}
	{#if column.id === 'state'}
		<span class={`st st--${row.tone}`}>
			<Icon name={row.icon} size="sm" />
			{row.state}
		</span>
	{:else if column.id === 'what'}
		<div class="trunc" title={row.name}>
			<span class="cell-title">{row.name}</span>
			{#if row.detail}<span>{row.detail}</span>{/if}
		</div>
		<!--
			The upstream's own words, VERBATIM after redaction. The first line is
			what fits a cell and the whole of it rides on `title`; it is never a
			paraphrase, because the one column a user scans for what is broken is
			the one column UsArr may not rewrite (§17.3, §10).
		-->
		{#if row.problem}
			<div class="cell-sub trunc" title={row.problem}>{firstLine(row.problem)}</div>
		{/if}
	{:else}
		<!--
			§17.3: a problem is stated canonically once per screen. This is a link
			TO the row that owns the fix, never a second copy of the fix.

			`servicesPath` IS `resolve('/services')`, computed once above, so this
			carries any configured base path. eslint's rule wants the href to be a
			`resolve()` call directly, and a `ResolvedPathname` cannot carry a
			fragment — the fragment is the whole point here, since it is what makes
			this a link to the ROW rather than to the top of a table. Suppressed at
			the one call site rather than turned off for the file, which is the
			precedent the Search screen set for the same limitation with `?sort=`.
		-->
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry #service-<id> -->
		<a class="btn btn--sm" href={`${servicesPath}#service-${String(row.id)}`}>Open in Services</a>
	{/if}
{/snippet}

<!--
	THE SEARCH ENTRY POINT.

	⚠️ IT IS A ROUTE, NOT A SEARCH FIELD, AND THAT IS THE WHOLE DESIGN.
	DESIGN-DIRECTION §8.3 holds library search and release search apart —
	"Merging them is how a 0 ms local query ends up waiting on a 30 s indexer" —
	so nothing here fans out to an indexer, opens an SSE stream or renders a
	result. Submitting navigates to Requests carrying `?q=`, which is the
	mechanism §17.4 already fixes for the `Search indexers →` action on a result
	row. Home stays a local read and Requests stays the one release-search
	surface in UsArr.

	IT SAYS WHAT IT SEARCHES, in the note under it, because an unlabelled input
	on a home screen reads as a search of your own library by default — and on
	this install a search of your own library is the one thing that cannot work.

	DRAWN OFF `hasIndexer`, NOT OFF THE MODE. See $lib/home: the precondition is
	that something can answer the query, which is true in `library` mode too and
	false on a fresh install. A box with nothing behind it is invented status
	wearing a control.

	NOT A HERO, and §1.5's table is the checklist it is written against: no
	oversized centred input, no illustration, no "Get started", no stat banner.
	It is an <h2> at --text-lg, a native <input type="search"> at the same
	content edge as the table it sits beside, and a submit button.
-->
{#if indexers}
	<section class="section" id="home-search">
		<div class="section__head">
			<h2>Search your indexers</h2>
		</div>
		<!--
			A REAL FORM WITH A REAL DESTINATION. The button's `formaction` is the
			same href `submit()` navigates to, so the control works without
			scripting and the handler only upgrades a page load to a client
			navigation. `type="search"` is the native control; nothing here is a
			styled <div> pretending to be one.
		-->
		<form class="homesearch" onsubmit={submit}>
			<!--
				A LABEL, NOT A PLACEHOLDER-AS-LABEL: the placeholder is gone the moment
				the field is typed into, and it is the only other string here.

				⚠️ IT DELIBERATELY DOES NOT REPEAT THE HEADING. `Search your indexers`
				is two elements away in the accessibility tree, and naming the field
				with it too makes a screen reader announce the same four words twice in
				a row — heading, then edit field — which is noise rather than context.
				The heading supplies the scope for anyone navigating by heading; the
				label supplies what goes IN the box, which is the thing the heading
				does not say. Requests draws the same distinction with `Search terms`.
			-->
			<label class="sr" for="home-query">Release name</label>
			<input
				id="home-query"
				name="q"
				type="search"
				class="searchfield homesearch__input"
				autocomplete="off"
				placeholder="Release name, or part of one"
				aria-describedby="home-search-note"
				bind:value={query}
			/>
			<button type="submit" class="btn btn--primary" formaction={searchHref}>Search indexers</button
			>
		</form>
		<p class="note" id="home-search-note">{HOME_SEARCH_SCOPE_NOTE}</p>
	</section>
{/if}

<!--
	RECENT GRABS, AND IT IS NOT BLOCK C. Block C is `Recently added` over a
	catalogue that does not exist; this is the local record a grab leaves, read
	from GET /api/v1/grabs/recent. It occupies the slot Block C will occupy and
	says what it actually is at the top of it.

	HIDDEN WHEN EMPTY, which is Block B's discipline applied to the one other
	region on this screen that can be empty for an honest reason. A user who has
	connected Prowlarr and grabbed nothing yet is told what to do by the search
	box above; a "No grabs recorded yet" table under it would be a region whose
	only content is the observation that it has none. Komga's `v-if="count > 0"`
	and Navidrome's `LibrarySelector` returning null are the same rule, and §17.2
	adopts both.

	AN UNREADABLE LIST IS NOT AN EMPTY ONE, so the error arm draws where the
	empty one does not. This is a local SQLite read: it failing says something
	about UsArr rather than about your grabs, and silence there would let the
	screen imply nothing has ever been grabbed.
-->
{#if grabsError}
	<section class="section" id="home-grabs">
		<div class="section__head"><h2>Recent grabs</h2></div>
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">Your recent grabs could not be read</div>
				<div class="banner__text">
					This is a local read from UsArr’s own database, so it failing is not an upstream problem.
					Nothing is missing here because a grab did not happen — the list simply could not be
					loaded.
				</div>
				<p class="verbatim">{grabsError}</p>
			</div>
		</div>
	</section>
{:else if grabsLoaded && grabs.length > 0}
	<section class="section" id="home-grabs">
		<div class="section__head">
			<h2>Recent grabs</h2>
			<span class="section__count num">
				{#if grabs.length < RECENT_LIMIT}
					{grabs.length}
					{grabs.length === 1 ? 'grab' : 'grabs'}
				{:else}
					the {RECENT_LIMIT} most recent
				{/if}
			</span>
			<!--
				§17.4's show-all template: the link names its type and its parent
				scope, so it still says what it does when a screen reader reads it out
				of context in a links list. It is the route to the canonical block —
				the one that carries the size, the indexer, the protocol and the two
				actions a row may offer.
			-->
			<div class="section__actions">
				<a class="btn btn--sm" href={requestsPath}>All recent grabs on Requests</a>
			</div>
		</div>

		<!--
			WHERE UsArr'S KNOWLEDGE STOPS, and it is above the table because it
			governs how every row below is read rather than footnoting them. It is
			$lib/requests' own constant, so Home and Requests cannot drift into
			describing the same rows differently.

			⚠️ NOT_SENT_NOTE IS DELIBERATELY NOT CARRIED HERE. It exists to stop the
			Requests block reading as a complete register of every way a grab can go
			wrong. This block states its own bound in the count beside the heading —
			`the 10 most recent` — and links to the block that carries the sentence,
			so the inference it guards against is already blocked. Repeating a
			three-clause paragraph about audit-log routing on a summary would cost
			more rows than the summary has.
		-->
		<p class="note home-grabnote">{KNOWLEDGE_STOPS_NOTE}</p>

		<!--
			THE SAME COMPONENT THE REQUESTS BLOCK DRAWS, under a narrower projection.
			`$lib/RecentGrabs.svelte` owns the row markup — the three states, the
			conditional sub-lines, the labelled stacking below 760 px — so the two
			screens cannot end up wording the same row differently, which is the one
			thing that would make this summary worse than no summary.

			NO `actions` PROP, AND ITS ABSENCE IS THE DECISION. §17.3 states a problem
			canonically once per screen: `Search again` starts a fresh fan-out only
			Requests can run, and `Open Services` is Block B's job above when the
			fault is UsArr's own configuration. `outcome.nonAction` still renders,
			because naming the non-action is a statement about the row rather than a
			control, and dropping it would leave a bare error chip.
		-->
		<RecentGrabs
			{grabs}
			columns={GRAB_COLUMNS}
			{now}
			total={grabs.length < RECENT_LIMIT ? grabs.length : undefined}
		/>
	</section>
{/if}

<!--
	§17.7's `unconfigured`, and §9.6's four constraints applied literally: the
	heading is an <h2> at --text-lg, everything is left-aligned at the same
	content edge the block it replaces would use, there is no container, and the
	state itself is one sentence. The extra explanation first run genuinely needs
	goes BELOW the buttons at --text-base rather than as a wider measure above
	them (§9.6 rule 4).
-->
{#if mode === 'unconfigured'}
	<div class="section">
		<div class="empty">
			<h2 class="empty__title">No services configured</h2>
			<!--
				Only Prowlarr is named, and that is a correctness call rather than
				brevity. `internal/httpapi.serviceKinds` accepts exactly one kind, so
				a sentence offering Sonarr, Radarr or a media server here would send
				a brand-new user to a dialog that refuses all three. The mockup makes
				the same call in its own comment for the same reason: naming a
				service the install cannot read a library from is this screen
				promising, on the very first thing a new user sees, something the
				milestone does not ship.
			-->
			<p class="empty__text">
				UsArr talks to the services you already run, and none of them is connected yet.
			</p>
			<div class="empty__actions">
				<a class="btn btn--primary" href={servicesPath}>Add a service</a>
			</div>
			<p class="note home-note">
				The service this build connects is Prowlarr, which gives you free-text search across your
				indexers and a grab that goes to Prowlarr's own download client. Adding one takes four
				things: which application it is, a name for it, its base URL and an API key. The connection
				is tested before anything is saved, and a service that fails its test is never stored.
			</p>
		</div>
	</div>
{/if}

<!--
	§8.5's NAMED CONFIGURATION, activated when no configured instance advertises
	LibrarySync. It is not an empty screen and it is not a stage of setting one
	up, and the copy has to say the second part as plainly as the first or the
	user reads the whole screen as a loading state for a library that is coming.
-->
{#if mode === 'search-and-grab'}
	<div class="section">
		<div class="empty">
			<h2 class="empty__title">Search-and-Grab mode</h2>
			<!--
				§8.5's own sentence, minus its opening clause, which the heading
				above already carries: "UsArr is running in Search-and-Grab mode:
				search your indexers and send grabs to your download client."
			-->
			<p class="empty__text">
				No library-bearing service is configured, so there is no library to show: what UsArr can do
				today is search your indexers and send grabs to your download client.
			</p>
			<!--
				⚠️ THE `Search indexers` BUTTON THAT STOOD HERE IS GONE, AND IT WAS
				REPLACED RATHER THAN DELETED. It navigated to Requests with no query;
				the search section above navigates to the same screen WITH one, from
				the same content edge, under a heading that says what it searches. Two
				controls to the same place, one of which is strictly weaker, is the
				second copy of the fix §17.3 bans. `Open Services` stays: it is the
				route to the thing the paragraph below is about, and it is the only
				action left on this block.

				⚠️ AND IT LOSES `btn--primary`, WHICH IS A HIERARCHY FIX RATHER THAN A
				DEMOTION. It carried the accent while it was the second of two buttons
				on an otherwise empty screen. The screen now has a primary action —
				`Search indexers`, on the form above — and §3.3a gives that accent to
				ONE control: two filled buttons on one screen point at two different
				things and so point at neither, and the one they would fight over here
				is the box the user came to type in.
			-->
			<div class="empty__actions">
				<a class="btn" href={servicesPath}>Open Services</a>
			</div>
			<!--
				§8.5 ends this state "Add a Sonarr, Radarr or media server to get a
				library", and that instruction is NOT shipped here, deliberately.
				The API accepts one kind, so the sentence would be an action the
				product refuses one click later — the "no invented status" failure
				reached by offering rather than by asserting. What replaces it says
				the same thing truthfully: a library is what those services would
				give you, and this build cannot connect them yet.
			-->
			<p class="note home-note">
				A Sonarr, a Radarr or a media server is what would give UsArr a library to replicate, and
				this build does not accept those kinds yet. Prowlarr is the only one it can connect, so this
				is the configuration rather than a stage on the way to another one.
			</p>
		</div>
	</div>
{/if}

<!--
	The third arm. It is unreachable in this build — every kind the API accepts
	is an indexer — and it is written rather than left as a blank screen,
	because the alternative is that the first build to connect a Sonarr renders
	Home as nothing at all. It says only what would be true of it: a
	library-bearing service is configured and there is no catalogue behind it.
	It does not draw Block A, Block C or §17.7's `partial` banner, because all
	three would need the import that has not been built.
-->
{#if mode === 'library'}
	<div class="section">
		<div class="empty">
			<h2 class="empty__title">Nothing catalogued yet</h2>
			<p class="empty__text">
				A library-bearing service is configured and UsArr has not read a catalogue from it, because
				the library sync is not built in this version.
			</p>
			<div class="empty__actions">
				<a class="btn btn--primary" href={servicesPath}>Open Services</a>
			</div>
		</div>
	</div>
{/if}

<style>
	/*
	 * §9.6 rule 4's below-the-buttons paragraph. `.note` is the class and the
	 * only thing this adds is the gap that keeps it off the button row; it is
	 * deliberately not a container, a panel or a background step.
	 */
	.home-note {
		margin-top: var(--space-5);
	}

	/*
	 * The search ROW — placement only. The field's own appearance moved to
	 * app.css's `.searchfield`, which the input below carries as its first
	 * class; what is left here is where the row puts it, which is the half no
	 * shared class may own.
	 *
	 * THESE FOUR LINES ARE NOT A DUPLICATE OF THE REQUESTS TOOLBAR'S FOUR even
	 * though they read identically today, and that is why they did not go with
	 * the appearance. Every one of them is flex participation: a caller that
	 * stacked this field under a label, or dropped it into a grid cell, would
	 * have to undo all four. The mockup's own `@media (max-width: 700px)` block
	 * is the recorded case — it turns `form.toolbar` into a column and resets
	 * `flex` on every child to do it.
	 *
	 * `1 1 24rem` rather than a width: the input takes the row on a phone and
	 * shares it with the button on a desktop, with no breakpoint of its own. It
	 * is 24rem here and 20rem on Requests, whose row carries a label, a select
	 * and two more buttons — the two numbers disagreed from the day they were
	 * written, which is on its own the argument against a shared basis.
	 *
	 * ⚠️ AND IT IS CAPPED, WHICH IS THE §1.5 FIX RATHER THAN A TASTE CALL.
	 * Uncapped it grew to 1,180 px at 1440 — a full-bleed input above the fold
	 * with nothing beside it, which is the hero-search bar §1.5's table sends
	 * back to the landing page. 42rem holds roughly the longest scene release
	 * name a user would actually type and leaves the row reading as a control on
	 * a screen rather than as the screen's subject. Below that width it still
	 * takes the whole row, so the phone case is unchanged.
	 *
	 * `min-width: 0` is the flex automatic-minimum-size override and stays with
	 * the row for the same reason: see `.searchfield`'s own note, which carries
	 * the measurement.
	 */
	.homesearch {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}

	.homesearch__input {
		flex: 1 1 24rem;
		max-width: 42rem;
		min-width: 0;
	}

	/* The note sits between the section head and the table, so it needs the gap
	 * on the table side rather than the paragraph spacing `.note` carries for a
	 * paragraph following a button row. */
	.home-grabnote {
		margin-bottom: var(--space-4);
	}
</style>
