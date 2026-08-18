<script lang="ts">
	/**
	 * LIBRARIES — the row view. ARCHITECTURE.md §17.8, over
	 * `GET /api/v1/libraries` (`docs/reference/http-api.md` §2).
	 *
	 * §17.8 splits this screen from Services in one sentence: *"Services answers
	 * 'is the pipe up, and how do I fix it?'. Libraries answers 'what is in it,
	 * what is it called, and where do requests go?'"*. The endpoint answers the
	 * first two thirds; the third is deferred by §17.8 itself and the screen says
	 * so once, in its own copy.
	 *
	 * WHAT IS RENDERED IS WHAT EXISTS, AND THE READ IS ALL THAT EXISTS. There is
	 * one libraries route in `internal/httpapi/server.go` and it is a GET, so
	 * this screen has no Accept, no Decline, no `Add library`, no rename, no
	 * reorder handle and no detail view. Every one of those is specified in
	 * §17.8; none of them has an endpoint, and a control that calls nothing is
	 * the invented status CLAUDE.md forbids. The copy under the table says which
	 * ones are missing rather than leaving the reader to notice.
	 *
	 * ⚠️ AND THERE IS NO PROPOSAL UI, WHICH IS A DECISION RATHER THAN AN OMISSION.
	 * [ADR-0048](../../../../docs/DECISIONS.md#adr-0048) settles that a library
	 * proposal is NOT a row in `library`: it lives in the connect probe's
	 * response, is never persisted, and a row exists only once the user has
	 * accepted one. So this endpoint serves accepted libraries and cannot serve a
	 * proposal, and drawing §17.8's pre-checked Accept step here would be drawing
	 * a screen over a value that does not travel on this wire.
	 *
	 * TIER 0, SO THERE IS NO LOADING AFFORDANCE AT ALL. DESIGN-DIRECTION §7.2:
	 * *"Show nothing at all. No skeleton, no spinner, no fade-in."* The endpoint
	 * is two SQLite statements with no *Arr call and no capability probe, so the
	 * table simply is not in the DOM until the read has answered — which is a
	 * different thing from rendering an empty table, and the reason the `empty`
	 * state is gated on `loaded`.
	 *
	 * THE THREE WAYS THIS SCREEN SHOWS NOTHING, AND THEY READ DIFFERENTLY:
	 *   · the read FAILED — an error banner with the server's verbatim text.
	 *   · the SESSION ENDED — a prompt, not an error. Nothing is broken and there
	 *     is no upstream text to quote, so there is no verbatim block.
	 *   · there are genuinely NO LIBRARIES — the list's own `empty` state (§9.6):
	 *     one sentence naming why, and the one link that leads to the fix.
	 * `describeFailure` in `$lib/libraries` owns the first two so a node-run test
	 * can pin them; a rule that lives inside an `{#if}` here is untestable.
	 *
	 * ⚠️ NOTHING ON THIS SCREEN CLAIMS A SOURCE IS HEALTHY. `missing_since` has no
	 * writer anywhere in the tree (`http-api.md` §2.4), so its absence is silence
	 * and not a pass. There is no green tick in the State column and no `.st--ok`
	 * anywhere below; see `libraryStates` for the argument.
	 */
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import { NOTHING, type ListColumn } from '$lib/list';
	import {
		describeFailure,
		fetchLibraries,
		itemCountText,
		kindLabel,
		libraryStates,
		sourceChips,
		type LibrariesFailure,
		type Library
	} from '$lib/libraries';
	// One §9.1 timestamp formatter in the product, and it lives beside the screen
	// that needed it first. Duplicating it here would be a second implementation
	// of a rule the design states once.
	import { stampOf } from '$lib/services';

	/**
	 * The columns, declared. §9.1: `table-layout: auto` has to measure every cell
	 * in every row, which is the one layout mode no containment can help.
	 *
	 * ⚠️ NOT ONE OF THESE TRACKS IS CONTENT-SIZED, AND THAT IS A CORRECTNESS RULE.
	 * ADR-0029 makes every row its own `display: grid`, so a track sized by
	 * `auto`, `max-content`, `min-content` or `fit-content()` — in either position
	 * of a `minmax()` — resolves against the contents of its OWN row and has
	 * nothing to line up with. `$lib/list`'s `findContentSizedTracks` warns in
	 * DEV; the reserve below is a declared `fr` with a 0 floor or a fixed length,
	 * everywhere.
	 *
	 * The two fixed tracks are sized to the widest string each can print rather
	 * than to a guess. `Kind` prints one of §17.8's five labels plus, for the two
	 * schema kinds §17.8 gives no product word to, the schema value itself; the
	 * widest of those is `Comics`. `Items` prints a grouped integer, and 110px is
	 * the reserve the Services screen's own `Items` column already carries, on the
	 * same face at the same density.
	 *
	 * §9.1's overflow policy is honoured by letting the CELLS wrap — the chips in
	 * `Sources` and the marks in `State` both stack — rather than by an unbounded
	 * track, which is the correction `$lib/list`'s header records.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'library', header: 'Library', width: 'minmax(0, 1.6fr)' },
		{ id: 'kind', header: 'Kind', width: '110px' },
		{ id: 'items', header: 'Items', width: '110px', align: 'end' },
		{ id: 'sources', header: 'Sources', width: 'minmax(0, 1.8fr)' },
		{ id: 'state', header: 'State', width: 'minmax(0, 1.3fr)' }
	];

	const SERVICES = resolve('/services');
	const LOGIN = resolve('/login');

	/**
	 * The Services row that owns one source, as a path plus a fragment.
	 *
	 * The path half is `resolve()`'s, so a configured base path is carried; the
	 * fragment half cannot be, because a `ResolvedPathname` does not take one and
	 * the fragment is the whole point — it is what makes this a link to the ROW
	 * rather than to the top of an eight-row screen, which §17.8 names as a
	 * defect in as many words. Home's Block A does the same join at its own
	 * `Open in Services` link, and `svelte/no-navigation-without-resolve` is
	 * suppressed at the one call site there and here — the rule fires on anything
	 * that is not a bare `resolve()`, a function call included, which is the rule
	 * being strict rather than wrong. What this function adds is that the join and
	 * the `#service-<id>` contract with `routes/services/+page.svelte` have one
	 * definition on this screen rather than one per chip.
	 */
	function servicesAnchor(serviceInstanceId: number): string {
		return `${SERVICES}#service-${String(serviceInstanceId)}`;
	}

	let libraries = $state<Library[]>([]);
	let loaded = $state(false);
	let failure = $state<LibrariesFailure | undefined>(undefined);

	/** Re-read on a timer so a relative time stays true rather than freezing. */
	let now = $state(new Date());

	async function load() {
		try {
			libraries = await fetchLibraries();
			failure = undefined;
		} catch (error) {
			libraries = [];
			failure = describeFailure(error);
		} finally {
			loaded = true;
		}
	}

	onMount(() => {
		void load();
		// A cheap re-read of local SQLite rather than an upstream call, and it also
		// keeps a relative stamp from freezing at whatever it said on arrival.
		const timer = setInterval(() => {
			now = new Date();
			void load();
		}, 60_000);
		return () => clearInterval(timer);
	});

	const count = $derived(libraries.length);
	const showTable = $derived(loaded && failure === undefined);
</script>

<svelte:head><title>Libraries · UsArr</title></svelte:head>

<!--
	NO <h2> RESTATING THE PAGE NAME. The shell's toolbar already renders it and
	the same string is the h1 inside main. Standing rule on every screen.

	THE DEFINITION IS SHIPPING COPY, NOT A NOTE (§17.8). It is the newest concept
	in the product, nothing else in the ecosystem works this way, and strip it and
	the screen contains no definition of a library anywhere. Verbatim from §17.8.
-->
<div class="pagehead">
	<p class="pagehead__meta">
		A library is a name you own over containers your services already computed: a whole instance, a
		root folder, an upstream library id, or an *Arr tag.
	</p>
</div>

{#if failure}
	<div class="section">
		{#if failure.k === 'session'}
			<!--
				A PROMPT, NOT AN ERROR (§10, and §17.3.3's shape for the neighbouring
				case). Nothing is broken and nothing was lost, so there is no verbatim
				block: the server did not fail, it declined an unauthenticated read.

				⚠️ MEASURED, AND IT DOES NOT REACH THE SCREEN TODAY. `$lib/api` hands
				every 401 to the shell's `onUnauthorized` hook, which clears the
				session — and `+layout.svelte` renders `!session.authenticated` as its
				own `Not signed in. Taking you to the sign-in page.` INSTEAD of this
				route's children, then navigates. Driven in Chromium against the built
				SPA with a late 401 and the /login route chunk delayed, this banner
				appeared in 0 of 400 polled frames while the path went /libraries →
				/login. So it is not "what the user sees in the gap", and saying so
				would be inventing a frame nobody has been shown. What it IS: this
				screen's own correct reading of a 401, which is why the else-arm below
				cannot render one as a failed local read with a Try again the user
				would press forever. It is one branch, and it is the branch that keeps
				the other one honest.
			-->
			<div class="banner banner--warn" role="alert">
				<Icon name="alert" />
				<div class="banner__body">
					<div class="banner__title">{failure.title}</div>
					<p class="banner__text">{failure.text}</p>
				</div>
				<div class="banner__actions">
					<a class="btn btn--primary" href={LOGIN}>Sign in</a>
				</div>
			</div>
		{:else}
			<!--
				§10's `error` state: the verbatim upstream text plus a retry. The
				endpoint reads local SQLite, so the sentence deliberately does not send
				the user to Services — a service being down cannot cause this, and a
				fix that cannot work is worse than none.
			-->
			<div class="banner banner--err" role="alert">
				<Icon name="x-circle" />
				<div class="banner__body">
					<div class="banner__title">{failure.title}</div>
					<p class="banner__text">{failure.text}</p>
					<p class="verbatim">{failure.verbatim}</p>
				</div>
				<div class="banner__actions">
					<button type="button" class="btn btn--primary" onclick={() => void load()}>
						Try again
					</button>
				</div>
			</div>
		{/if}
	</div>
{/if}

{#if showTable}
	<div class="toolbar">
		<span class="toolbar__label">
			{#if count === 0}
				No library exists yet.
			{:else}
				{count}
				{count === 1 ? 'library' : 'libraries'}. This table renders from the replica and never waits
				on a service.
			{/if}
		</span>
		<span class="toolbar__spacer"></span>
		<!--
			§9.1: a fact identical on every row is stated ONCE, above the table, and
			its column is dropped. Both of these are exactly that shape, and the first
			is §17.8's own required sentence for the deferred column. A third note,
			about what this screen cannot DO, sits under the table instead.
		-->
		<p class="toolbar__note toolbar__label">
			No request destination can be set yet: no connected service accepts requests. Indexer search
			still works, and the grab ends in your download client.
		</p>
		<p class="toolbar__note toolbar__label">
			Per-source health is not reported yet, so a source with no warning beside it has not been
			checked.
		</p>
	</div>

	<List
		label="Your libraries"
		columns={COLUMNS}
		rows={libraries}
		key={(library) => String(library.id)}
		total={count}
		state={count === 0 ? 'empty' : 'default'}
		emptyTitle="No libraries yet"
		emptyText="A library appears here once a service with a catalogue is connected and its first import has run. Prowlarr alone will not produce one, because an indexer has no library to bind to."
		{cell}
		{emptyActions}
	/>

	<!--
		A FOOTNOTE, NOT A TOOLBAR NOTE, AND THE SPLIT IS BY WHAT THE SENTENCE IS
		FOR. The two lines above are about how to READ the table — one explains a
		column §17.8 defers, the other stops an absent warning being read as a
		health pass — so they sit with the table, which is §9.1's rule for a fact
		identical on every row. This one is about what the screen cannot DO, which
		is the question a reader arrives with only after they have looked, and it
		is principle 3's honest degradation rather than a caption.
	-->
	<div class="section">
		<p class="screennote">
			Renaming a library, joining sources into one, and reordering this list are not built yet.
			UsArr creates a library for each container a service reports, on that service's first import.
		</p>
	</div>
{/if}

{#snippet emptyActions()}
	<a class="btn btn--primary" href={SERVICES}>Services</a>
{/snippet}

{#snippet cell(library: Library, column: ListColumn)}
	{#if column.id === 'library'}
		<!--
			§9.1 tier 1: an identity field truncates at the cell and carries the full
			string in `title`. The SLUG IS NOT RENDERED — §17.8 is emphatic that the
			row's identifier is not drawn as a path, because a self-hoster who reads
			`/movies` under a library concludes UsArr scans `/movies`, on the screen
			whose whole point is that it never reads a filesystem.
		-->
		<span class="trunc" title={library.name}>{library.name}</span>
	{:else if column.id === 'kind'}
		{kindLabel(library.kind)}
	{:else if column.id === 'items'}
		{itemCountText(library)}
	{:else if column.id === 'sources'}
		{@const chips = sourceChips(library)}
		{#if chips.total === 0}
			<span class="muted">{NOTHING.empty}</span>
		{:else}
			<div class="cell-chips">
				{#each chips.shown as chip (chip.key)}
					<!--
						§17.8's cross-link: *"a degraded source on a library row links to
						that instance's Services row"*. It is LABELLED AS NAVIGATION and
						performs navigation, which is §17.8's own rule about a control
						labelled as an operation. The Services screen anchors by id.
					-->
					{@const href = servicesAnchor(chip.serviceInstanceId)}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- see servicesAnchor -->
					<a class="chip" class:chip--pending={chip.missing} {href} title={chip.title}>
						<span class="trunc">{chip.label}</span>
					</a>
				{/each}
				{#if chips.more > 0}
					<span class="muted">+{chips.more} more</span>
				{/if}
			</div>
		{/if}
	{:else if column.id === 'state'}
		{@const marks = libraryStates(library)}
		{#if marks.length === 0}
			<span class="muted">{NOTHING.empty}</span>
		{:else}
			{#each marks as mark (mark.key)}
				<div class="statemark">
					<span class={`st st--${mark.tone}`}>
						<Icon name={mark.tone === 'warn' ? 'alert' : 'dash-circle'} size="sm" />{mark.word}
					</span>
					{#if mark.at}
						<span class="cell-sub">{stampOf(mark.at, now)}</span>
					{/if}
				</div>
			{/each}
		{/if}
	{/if}
{/snippet}

<style>
	/*
	 * The chips wrap rather than overflowing, which is how a declared track keeps
	 * §9.1's overflow policy: a genuine overflow degrades to a wrap, never to a
	 * clip, and never to a content-sized track that would break row alignment.
	 */
	.cell-chips {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-3);
		min-width: 0;
	}

	/* `.chip` is `white-space: nowrap`, so a long instance name would push past
	 * the track without these two. With them the chip truncates inside itself and
	 * the full string stays on `title`, which is §9.1 tier 1 applied to a chip. */
	.cell-chips :global(.chip) {
		max-width: 100%;
		min-width: 0;
	}

	/* Muted body text outside a table and outside an empty state, at the same
	 * size and colour `.pagehead__meta` uses, because it is the same kind of
	 * sentence in the same region. Page-scoped rather than promoted to app.css:
	 * §9.8 promotes on the second consumer, and this is the first. */
	.screennote {
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		max-width: 78ch;
	}

	/* One mark per line. A row can carry several — a library can be turned off,
	 * kept out of search and empty at the same time — and a precedence that
	 * printed one would drop facts the user has no other way to see. */
	.statemark {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2) var(--space-3);
		min-width: 0;
	}
</style>
