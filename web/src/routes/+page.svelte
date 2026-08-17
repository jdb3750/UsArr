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
	 * THE CSP FORBIDS INLINE STYLE ATTRIBUTES, so nothing here writes one. The
	 * list primitive sets its custom properties through the CSSOM; everything
	 * else on this screen is a class.
	 */
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { ApiError, fetchServicesHealth } from '$lib/api';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import type { ListColumn } from '$lib/list';
	import { attention, headline, homeMode, type AttentionRow, type HomeMode } from '$lib/home';
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

	const servicesPath = resolve('/services');
	const requestsPath = resolve('/requests');

	let mode = $state<HomeMode | undefined>(undefined);
	let services = $state(0);
	let rows = $state<AttentionRow[]>([]);
	let loadError = $state('');

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

	async function load() {
		try {
			const health = await fetchServicesHealth();
			mode = homeMode(health);
			services = health.services.length;
			rows = attention(health.services, now);
			loadError = '';
		} catch (error) {
			loadError = error instanceof ApiError ? error.detail : String(error);
		}
	}

	onMount(() => {
		void load();
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
			<div class="empty__actions">
				<a class="btn btn--primary" href={requestsPath}>Search indexers</a>
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
</style>
