<script lang="ts">
	/**
	 * REQUESTS — ARCHITECTURE.md §17.5.
	 *
	 * The sidebar entry is `Requests` and stays that way across milestones; the
	 * sub-caption below closes the gap between the label and what v0.1 actually
	 * holds, which is the Prowlarr free-text path and nothing else. There is no
	 * request list, no approval queue and no `pending → approved → routed →
	 * available` here, because the request model is v0.2 (§16 is authoritative)
	 * — and Recent grabs is NOT it and must not be presented as it. It is a
	 * local read over rows v0.1 already writes.
	 *
	 * FOUR BLOCKS, AND ONE OF THEM IS DELIBERATELY NOT HERE YET:
	 *
	 *   1  the search toolbar          built
	 *   2  the fan-out status          built
	 *   3  the release results table   NOT BUILT — see the marked seam below
	 *   4  Recent grabs                built
	 *
	 * Block 3 is arriving from the /search takeover, which owns sortable
	 * results, the flag chips and indexer scoping. Building a second results
	 * table here would guarantee two of them disagreeing, so the region is a
	 * marked seam with an honest placeholder rather than a half-table.
	 *
	 * WHAT THE COPY RULES ARE, because they are correctness rather than tone.
	 * A grab is irreversible from UsArr's side: the release goes to a download
	 * client UsArr deliberately stops observing after handoff, so a mis-click
	 * cannot be detected, reported or reversed. Two consequences run through
	 * this file. Nothing may claim bytes are moving — "sent" is the strongest
	 * true word for every handed-over state, the successful one included. And
	 * nothing may assert that a grab did not happen: Prowlarr adds a release to
	 * the download client BEFORE the step that failed for the owner, and never
	 * rolls back, so a 5xx can cover an operation that already partly
	 * succeeded. The vocabulary lives in `$lib/requests` where a test can read
	 * it, not in this template.
	 *
	 * ADR-0038 — a list freezes its order while a user is aiming at it, by
	 * pointer or by focus — is not implemented here and that is not an
	 * omission: it governs a list that reorders under an engaged user,
	 * and neither list on this screen does. The release results are the list it
	 * was written for and they arrive with block 3, which is where it belongs.
	 */
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import List from '$lib/List.svelte';
	import type { ListColumn } from '$lib/list';
	import { NOTHING } from '$lib/list';
	import { prefs } from '$lib/prefs.svelte';
	import {
		ApiError,
		fetchRecentGrabs,
		openEventStream,
		problemsFrom,
		startSearch,
		type IndexerOutcome,
		type RecentGrab,
		type Release,
		type SearchReport,
		type StreamHandles
	} from '$lib/api';
	import { formatSize } from '$lib/format';
	import {
		DEFAULT_SEARCH_TYPE,
		SEARCH_TYPES,
		SEARCH_TYPE_NOTE,
		THIN_COVERAGE_NOTE,
		KNOWLEDGE_STOPS_NOTE,
		NOT_SENT_NOTE,
		RECENT_GRAB_ROW_INTRINSIC,
		fanoutSummary,
		formatWhen,
		grabOutcome,
		isFreeTextOnly,
		searchTypeParam
	} from '$lib/requests';

	/* ── search state ─────────────────────────────────────────────────────── */

	let query = $state('');
	let searchType = $state(DEFAULT_SEARCH_TYPE);
	let submitted = $state('');
	let searchId = $state<string | undefined>(undefined);
	let searching = $state(false);
	let finished = $state(false);
	let searchError = $state('');
	/** The server's own code. `no_indexer_service` is not a failure to retry —
	 * it means nothing is configured — so it gets the link that fixes it. */
	let searchErrorCode = $state('');
	let streamGap = $state('');
	let streamConnected = $state(false);

	/**
	 * The de-duplicated candidate set.
	 *
	 * It is held even though no results table renders yet, because block 2's
	 * sentence is a claim about de-duplication and the only honest source for
	 * "10 after de-duplication" is the merge actually having been done. Counting
	 * frames instead would report the raw number twice.
	 */
	let releases = $state<Release[]>([]);
	let outcomes = $state<IndexerOutcome[]>([]);
	let report = $state<SearchReport | undefined>(undefined);

	/* ── recent grabs state ───────────────────────────────────────────────── */

	const RECENT_LIMIT = 10;
	let grabs = $state<RecentGrab[]>([]);
	let grabsLimit = $state(RECENT_LIMIT);
	let grabsLoaded = $state(false);
	let grabsError = $state('');

	/**
	 * The clock the relative column is measured against, refreshed once a
	 * minute. Ten rows re-rendering every 60 s is not a cost worth optimising,
	 * and a block whose whole job is "did I already grab this an hour ago?" may
	 * not sit there saying "1 minute ago" forty minutes later.
	 */
	let now = $state(new Date());

	let stream: StreamHandles | undefined;

	/* ── derived ──────────────────────────────────────────────────────────── */

	// The closing report supersedes the running tally: it is the server's own
	// accounting and it includes indexers skipped without ever starting.
	const problems = $derived(problemsFrom(report ? report.indexers : outcomes));
	const answered = $derived(report ? report.answered : outcomes.filter((o) => o.answered).length);
	const totalIndexers = $derived(report?.totalIndexers);
	const rawReleases = $derived(
		report ? report.results : outcomes.reduce((sum, o) => sum + o.count, 0)
	);
	const fanout = $derived(
		fanoutSummary({
			answered,
			total: totalIndexers,
			releases: rawReleases,
			deduped: releases.length,
			finished,
			// Block 3 is not on this screen, so nothing is "shown". Flip this
			// with the results table and the sentence becomes §17.5's verbatim.
			rendered: false
		})
	);

	const grabColumns: ListColumn[] = [
		{ id: 'when', header: 'When', width: '132px' },
		{ id: 'release', header: 'Release', width: 'minmax(0, 3fr)' },
		{ id: 'indexer', header: 'Indexer', width: 'minmax(0, 1fr)' },
		{ id: 'protocol', header: 'Protocol', width: '92px' },
		{ id: 'size', header: 'Size', width: '10ch', align: 'end' },
		{ id: 'outcome', header: 'Outcome', width: 'minmax(0, 1.9fr)' }
	];

	/**
	 * `aria-rowcount` is only allowed to be a number when it is the truth. A
	 * short page is the whole set; a full one means the server had at least as
	 * many as we asked for and we were not told how many more, which is exactly
	 * the case ARIA defines -1 for.
	 */
	const grabTotal = $derived(grabs.length < grabsLimit ? grabs.length : undefined);
	const rowIntrinsic = $derived(RECENT_GRAB_ROW_INTRINSIC[prefs.density] ?? 44);

	/* ── behaviour ────────────────────────────────────────────────────────── */

	// Results stream as indexers answer, so the cross-indexer dedupe cannot be
	// done up front: a later, higher-priority indexer names the candidate it
	// replaces (internal/httpapi/search.go).
	function mergeReleases(incoming: Release[]) {
		let next = releases;
		for (const release of incoming) {
			if (next.some((r) => r.candidateId === release.candidateId)) continue;
			if (release.supersedesCandidateId !== undefined) {
				next = next.filter((r) => r.candidateId !== release.supersedesCandidateId);
			}
			next = [...next, release];
		}
		releases = next;
	}

	function noteIndexer(outcome: IndexerOutcome, phase: string) {
		if (phase !== 'indexer_done') return;
		outcomes = [...outcomes.filter((o) => o.indexerId !== outcome.indexerId), outcome];
	}

	async function loadGrabs() {
		try {
			const recent = await fetchRecentGrabs(RECENT_LIMIT);
			grabs = recent.grabs;
			grabsLimit = recent.limit;
			grabsError = '';
		} catch (error) {
			// The block reports its own failure rather than rendering as empty.
			// An empty Recent grabs and an unreadable Recent grabs mean opposite
			// things — "you have grabbed nothing" against "UsArr cannot tell you
			// what you grabbed" — and only one of them is reassuring.
			grabsError = error instanceof ApiError ? error.detail : String(error);
		} finally {
			grabsLoaded = true;
		}
	}

	onMount(() => {
		void loadGrabs();

		const tick = setInterval(() => (now = new Date()), 60_000);

		stream = openEventStream(
			(event) => {
				if (event.kind === 'unknown') return;
				if (event.kind === 'missed') {
					streamGap = event.action ? `${event.message} — ${event.action}` : event.message;
					return;
				}
				// A frame from an older search is ignored, never rendered late.
				if (searchId && event.searchId && event.searchId !== searchId) return;

				switch (event.kind) {
					case 'started':
						// The stream can beat the 202 that carries the same id.
						if (!searchId) searchId = event.searchId;
						break;
					case 'results':
						mergeReleases(event.releases);
						break;
					case 'indexer':
						noteIndexer(event.indexer, event.phase);
						break;
					case 'done':
						report = event.report;
						searching = false;
						finished = true;
						break;
					case 'failed':
						searchError = event.action ? `${event.message} — ${event.action}` : event.message;
						searching = false;
						break;
				}
			},
			(connected) => (streamConnected = connected)
		);

		return () => {
			clearInterval(tick);
			stream?.close();
		};
	});

	async function runSearch(event: SubmitEvent) {
		event.preventDefault();
		const trimmed = query.trim();
		if (trimmed === '') return;

		submitted = trimmed;
		searchId = undefined;
		releases = [];
		outcomes = [];
		report = undefined;
		searchError = '';
		searchErrorCode = '';
		streamGap = '';
		finished = false;
		searching = true;

		try {
			const accepted = await startSearch(trimmed, searchTypeParam(searchType));
			if (accepted.searchId) searchId = accepted.searchId;
		} catch (error) {
			searching = false;
			searchError = error instanceof ApiError ? error.detail : String(error);
			searchErrorCode = error instanceof ApiError ? error.code : '';
		}
	}
</script>

<svelte:head><title>Requests · UsArr</title></svelte:head>

<div class="pagehead">
	<p class="pagehead__meta">
		Free-text indexer search through Prowlarr, a grab sent to Prowlarr’s own download client, and
		your recent grabs.
	</p>
</div>

<!-- ══ BLOCK 1 — the search toolbar ═══════════════════════════════════════ -->

<form class="toolbar" onsubmit={runSearch}>
	<label class="toolbar__label" for="req-query">Query</label>
	<input
		id="req-query"
		class="input req-query"
		name="query"
		type="search"
		autocomplete="off"
		placeholder="Release name, or part of one"
		bind:value={query}
	/>

	<label class="toolbar__label" for="req-type">Search type</label>
	<!--
		Native <select>. The five values are Prowlarr's own, and the five ids map
		to the `type=` parameter in $lib/requests rather than here — a label that
		reads "TV" travels as `tvsearch`, and getting that wrong is invisible:
		Prowlarr does not error on an unrecognised type, it quietly runs a basic
		search instead.
	-->
	<select id="req-type" class="select" bind:value={searchType} aria-describedby="req-type-note">
		{#each SEARCH_TYPES as type (type.id)}
			<option value={type.id}>{type.label}</option>
		{/each}
	</select>

	<button type="submit" class="btn btn--primary" disabled={query.trim() === ''}>Search</button>

	<!--
		SEAM — the indexer and category multi-selects.

		They arrive with the /search takeover, alongside the sortable results
		table and the flag chips, and they are deliberately absent rather than
		stubbed: a scope control that does not scope anything is worse than no
		control, because the user reads the search as narrowed when it was not.
	-->

	<span class="toolbar__label toolbar__note" id="req-type-note">
		{SEARCH_TYPE_NOTE}
		{#if isFreeTextOnly(searchType)}
			<!-- SW-08, at the point of use: a search that runs correctly against
			     healthy indexers and returns one row looks identical to a broken
			     one, and for these two types the reason is the indexer ecosystem
			     rather than the query. -->
			<span class="req-coverage">{THIN_COVERAGE_NOTE}</span>
		{/if}
	</span>
</form>

<!--
	Every banner below is non-modal and sits above the content. Nothing is greyed
	out and nothing is replaced by a failure — ARCHITECTURE.md §17.7.
-->

{#if searchErrorCode === 'no_indexer_service'}
	<div class="banner banner--err" role="alert">
		<div class="banner__body">
			<div class="banner__title">There is no indexer service to search</div>
			<div class="banner__text">
				This screen searches your indexers through Prowlarr, and UsArr never talks to an indexer
				directly, so without one there is nothing to query.
			</div>
			<p class="verbatim">{searchError}</p>
		</div>
		<div class="banner__actions">
			<a class="btn btn--primary" href={resolve('/services')}>Add Prowlarr</a>
		</div>
	</div>
{:else if searchError}
	<div class="banner banner--err" role="alert">
		<div class="banner__body">
			<div class="banner__title">The search did not complete</div>
			<p class="verbatim">{searchError}</p>
		</div>
	</div>
{/if}

{#if streamGap}
	<div class="banner banner--warn" role="status">
		<div class="banner__body">
			<div class="banner__title">Some events were missed while the connection was down</div>
			<div class="banner__text">The counts below may be short by whatever arrived in that gap.</div>
			<p class="verbatim">{streamGap}</p>
		</div>
	</div>
{/if}

{#if submitted && !streamConnected && !searchError}
	<div class="banner banner--warn" role="status">
		<div class="banner__body">
			<div class="banner__title">The event stream is not connected</div>
			<div class="banner__text">
				Results arrive on <code class="mono">/api/events</code> as each indexer answers, so nothing can
				arrive progressively until it reconnects. Anything already counted is still valid.
			</div>
		</div>
	</div>
{/if}

<!-- ══ BLOCK 2 — fan-out status ═══════════════════════════════════════════ -->

<!--
	REAL COUNTS, NEVER A PROGRESS BAR. UsArr cannot time an outstanding indexer
	leg, so a bar would be drawing a number it does not have; the two figures it
	does have are how many answered and how many releases came back. The live
	region is atomic because the sentence is only true as a whole — announcing
	"9" without "of 9" is worse than announcing nothing.
-->
{#if submitted}
	<div class="section">
		<p class="req-fanout" role="status" aria-atomic="true">
			<span class="req-fanout__query">{submitted}</span>
			<span class="num">{fanout}</span>
		</p>
		{#if searching && answered === 0}
			<!--
				The window between pressing Search and the first indexer answering,
				which on a cold fan-out is seconds rather than instant. Without this
				the line above reads "0 indexers responded · 0 releases so far",
				which is true and looks like a search that finished badly.

				It is a sentence rather than a spinner or a bar: UsArr cannot time
				an outstanding leg, so the only honest thing to draw is words.

				`searching` deliberately does NOT disable the Search button. The
				flag is cleared by a `done` or `failed` frame off the SSE stream, so
				a stream that drops mid-search would leave a disabled button and no
				way to try again — trading a double-submit for a dead screen.
			-->
			<p class="req-note">Searching. No indexer has answered yet.</p>
		{/if}
		{#if report?.summary}
			<!--
				Prowlarr answers 200 with `[]` for "every indexer failed", "all
				rate-limited" and "nothing matched" alike, so this sentence from the
				server is the only thing that tells them apart (§17.7).
			-->
			<p class="req-note">{report.summary}</p>
		{/if}
	</div>
{/if}

{#each problems as problem (problem.indexer)}
	<!--
		One banner per indexer that did not answer, non-modal, with the upstream's
		own words verbatim (§9.5, §17.3). Per-indexer rather than one aggregate
		line: "3 indexers failed" is not something a user can act on, and the
		three reasons are usually three different problems.
	-->
	<div class="banner banner--warn" role="status">
		<div class="banner__body">
			<div class="banner__title">
				{problem.indexer} did not answer, so its releases are missing from these counts
			</div>
			<div class="banner__text">
				Every other indexer’s results are real. The count above says how many answered rather than
				implying the search was whole.
			</div>
			<p class="verbatim">{problem.error}</p>
		</div>
	</div>
{/each}

<!-- ══ BLOCK 3 — release results ══════════════════════════════════════════ -->

<!--
	INTEGRATION POINT, NOT AN OMISSION.

	The per-release results table — sortable columns, indexer flag chips, the
	grab window countdown, the Grab control and ADR-0038's ordering rule, which
	freezes a list while a user is aiming at it by pointer or by focus — arrives
	here from the /search takeover, which is landing that work on its own screen
	first. It is not duplicated here in the meantime: two
	results tables built from the same SSE stream by two threads is how they end
	up disagreeing about what a de-duplicated row is, and this screen already
	states the counts that table would have to match.

	When it lands: render it inside this region, and set `rendered: true` on the
	fanoutSummary() call above so its sentence says "shown after de-duplication"
	rather than "after de-duplication". That flag is the seam.
-->
<div class="section" data-block="release-results">
	<div class="section__head"><h2>Release results</h2></div>
	<p class="req-note">
		The per-release results table is not on this screen yet — it is being finished on
		<a href={resolve('/search')}>Search</a>, and it moves here when it is. The counts above are real
		and come from the search you just ran; a release can only be grabbed from Search until then.
	</p>
</div>

<!-- ══ BLOCK 4 — recent grabs ═════════════════════════════════════════════ -->

<!--
	A grab leaves a record, and that is the whole point of this block. UsArr's
	only write path in v0.1 produces a multi-gigabyte download, and before this
	the confirmation lived in a chip inside a search-result row — one navigation
	away and there was no UsArr-side record that anything had happened.

	This is NOT the request model. No approval queue, no pending → approved →
	routed → available, no quota, no `request` table. It is a local read over
	rows v0.1 already writes.
-->
<section class="section" id="recent-grabs">
	<div class="section__head">
		<h2>Recent grabs</h2>
		{#if grabsLoaded && !grabsError}
			<span class="section__count num">
				{#if grabTotal !== undefined}
					{grabTotal}
					{grabTotal === 1 ? 'grab' : 'grabs'}
				{:else}
					the {grabsLimit} most recent
				{/if}
			</span>
		{/if}
	</div>

	<!-- Where UsArr's knowledge stops. It is above the table because it governs
	     how every row below is read, not a footnote to them. -->
	<p class="req-note">{KNOWLEDGE_STOPS_NOTE}</p>
	<!-- And which grabs are missing, so the block does not read as "every grab
	     worked". A provenance row is written only after the request was
	     dispatched (internal/httpapi/grabs.go). -->
	<p class="req-note">{NOT_SENT_NOTE}</p>

	{#if grabsError}
		<div class="banner banner--err" role="alert">
			<div class="banner__body">
				<div class="banner__title">Your recent grabs could not be read</div>
				<div class="banner__text">
					This is a local read from UsArr’s own database, so it failing is not an upstream problem.
					Nothing below is missing because a grab did not happen — the list simply could not be
					loaded.
				</div>
				<p class="verbatim">{grabsError}</p>
			</div>
		</div>
	{:else}
		<!--
			`labels`, not `two-line`, below 760 px. §9.1 gives a scanned results
			list the two-line treatment and a record you read one at a time the
			labelled one, and this is the second: the question a row answers is
			"did that one work?", so Outcome may not be one of the fields the
			two-line fork drops. Ten rows bounds what that costs.
		-->
		<List
			label="Recent grabs"
			columns={grabColumns}
			rows={grabs}
			key={(g) => String(g.id)}
			total={grabTotal}
			{rowIntrinsic}
			stack="labels"
			state={grabsLoaded ? (grabs.length === 0 ? 'empty' : 'default') : 'loading'}
			loadingNote="Reading your recent grabs."
			emptyTitle="No grabs recorded yet"
			emptyText="A grab you send from a release result is recorded here, newest first, and the record survives a restart — which is what makes it possible to answer “did I already grab that?” an hour later."
		>
			{#snippet cell(grab: RecentGrab, column: ListColumn)}
				{#if column.id === 'when'}
					{@const when = formatWhen(grab.grabbedAt, now)}
					{#if when.absolute}
						<!-- Absolute AND relative, per §17.3's rule: one identifies the
						     moment, the other answers "how long ago" without arithmetic. -->
						<span class="num">{when.absolute}</span>
						<div class="cell-sub">{when.relative}</div>
					{:else}
						<span class="muted">{NOTHING.empty}</span>
					{/if}
				{:else if column.id === 'release'}
					<!-- An identity string: truncated at the cell with the full value in
					     `title`, and never `overflow-wrap: anywhere`, which renders x264
					     as x26 / 4 and destroys the thing the reader is scanning for. -->
					<span class="mono trunc" title={grab.releaseTitle}>{grab.releaseTitle}</span>
				{:else if column.id === 'indexer'}
					{#if grab.indexerName}
						<span class="trunc" title={grab.indexerName}>{grab.indexerName}</span>
					{:else}
						<span class="muted">{NOTHING.empty}</span>
					{/if}
				{:else if column.id === 'protocol'}
					{#if grab.protocol}
						<!-- Achromatic swatch: the words `torrent` and `usenet` carry the
						     distinction, because a torrent green one column from a status
						     green is the one collision this ramp cannot afford. -->
						<span class="proto"
							><span class="proto__dot" aria-hidden="true"></span>{grab.protocol}</span
						>
					{:else}
						<span class="muted">{NOTHING.empty}</span>
					{/if}
				{:else if column.id === 'size'}
					{#if grab.sizeBytes !== undefined}
						{formatSize(grab.sizeBytes)}
					{:else}
						<!-- Not every indexer reports a size, and 0 would be a lie rather
						     than an absence. -->
						<span class="muted">{NOTHING.empty}</span>
					{/if}
				{:else if column.id === 'outcome'}
					{@const outcome = grabOutcome(grab.outcome)}
					<!--
						The chip's word is "sent" on every state, and the ambiguous row
						sits BESIDE the ordinary one rather than beside a failure: both
						were handed to the download client, and they differ only in
						whether an error came back afterwards. The warn role is on the
						ambiguous one because chroma marks what is wrong — or here, what
						is unknown and worth checking — not what is fine (§9.5). Removing
						the colour still leaves the two legible, which is why the labels
						differ rather than only the tone.

						There is no Retry on any row, on any state, permanently.

						THE SUB-LINE IS CONDITIONAL, and that is §9.1 rather than a
						nicety: a clause identical on every row of a state is not data,
						and "the client accepted it" under every `sent` chip restated
						the chip at the cost of a second line on every confirmed row.
						The two states that keep a clause carry an instruction the chip
						does not. $lib/requests decides which; this renders what it is
						given, and renders nothing when it is given nothing.
					-->
					<span class="chip" class:chip--pending={outcome.tone === 'warn'}>{outcome.label}</span>
					{#if outcome.detail}
						<div class="cell-sub">{outcome.detail}</div>
					{/if}
				{/if}
			{/snippet}
		</List>
	{/if}
</section>

<style>
	/*
	 * Screen-only rules. Everything with a design-system name is in app.css, and
	 * every value here resolves to a token — a literal is a review failure.
	 * Nothing is set from a `style` attribute: the server sends
	 * `style-src 'self'` with no 'unsafe-inline', so an inline style stays in
	 * the DOM and applies nothing.
	 */

	/* The query field is the control the screen is for, so it takes the slack in
	 * the toolbar's flex row rather than sitting at its intrinsic width. */
	.req-query {
		flex: 1 1 280px;
		min-width: 0;
	}

	/* The type-specific coverage sentence takes its own line under the standing
	 * note rather than extending it, so the standing text does not reflow when
	 * the selector changes. */
	.req-coverage {
		display: block;
		margin-top: var(--space-2);
	}

	/* The fan-out line. Real counts, in tabular figures so the numbers do not
	 * jitter horizontally as legs land — the one place on this screen where a
	 * value changes while the user is reading it. */
	.req-fanout {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2) var(--space-4);
		font-size: var(--text-base);
		line-height: var(--leading-base);
	}

	.req-fanout__query {
		font-weight: var(--weight-semibold);
		overflow-wrap: anywhere;
		min-width: 0;
	}

	/* A block-level note: the sentences §17.5 requires around Recent grabs, and
	 * the placeholder in the block-3 seam. Left-aligned in flow at the same
	 * content edge as the table it sits above, never a centred box. */
	.req-note {
		max-width: 78ch;
		margin-top: var(--space-2);
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.req-note + .req-note {
		margin-bottom: var(--space-4);
	}
</style>
