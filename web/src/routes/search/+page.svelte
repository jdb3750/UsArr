<script lang="ts">
	/**
	 * FREE-TEXT INDEXER SEARCH (ARCHITECTURE §17.5, §8.5).
	 *
	 * This screen COMPOSES three modules and contains as little as it can:
	 *
	 *   $lib/frozenorder.svelte  ADR-0038, the freeze-while-aimed rule
	 *   $lib/rowstate.svelte     ADR-0038 clause 4, identity-keyed row state
	 *   $lib/sortspec            ADR-0038 clause 6, sort key and direction in the URL
	 *   $lib/indexerscope.svelte the indexer picker's selection and its catalogue
	 *
	 * None of those is specific to this route. The parts left in this file are
	 * the ones that genuinely are: the SSE plumbing, the column set, the cell
	 * rendering and the grab affordance.
	 *
	 * THE SORT CONTROL SITS IN THE TOOLBAR, NOT IN THE COLUMN HEADERS, and that
	 * is a positive choice rather than an omission. §17.5 clause 3 defines the
	 * results region as "the result rows and their header", and says of the
	 * toolbar's sort control that it "sits outside it, so operating it is always
	 * an explicit re-sort". A header inside the frozen region would work, but it
	 * needs a sort affordance in `$lib/List.svelte`, which is a shared primitive
	 * another thread is building against right now.
	 */
	import { onMount } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import List from '$lib/List.svelte';
	import { NOTHING, capChips, type ListColumn, type ListState } from '$lib/list';
	import { createFrozenOrder, settle } from '$lib/frozenorder.svelte';
	import { createRowState } from '$lib/rowstate.svelte';
	import { createIndexerScope, parseIds } from '$lib/indexerscope.svelte';
	import {
		compareBy,
		nextDir,
		readSort,
		writeSort,
		type SortColumn,
		type SortSpec
	} from '$lib/sortspec';
	import {
		ApiError,
		grabRelease,
		openEventStream,
		problemsFrom,
		startSearch,
		type IndexerOutcome,
		type Release,
		type SearchReport,
		type StreamHandles
	} from '$lib/api';
	import { formatAge, formatSize } from '$lib/format';

	/** Per-row grab state, keyed by candidate id. Never by index — see
	 * `$lib/rowstate.svelte`, and ADR-0038 clause 4. */
	type GrabState = {
		state: 'grabbing' | 'grabbed' | 'sent-unknown' | 'failed';
		message: string;
		code: string;
		action: string;
	};

	/**
	 * ⚠️ THE ONLY CODES THAT GET A BUTTON, AND WHY THE SET IS AN ALLOWLIST.
	 *
	 * The grab error codes are a wire contract that is still being added to
	 * (internal/httpapi/errorcodes.go), so a code this build does not recognise
	 * is SHOWN TO THE USER and offered nothing. Guessing at an unfamiliar code
	 * guesses wrong in the dangerous direction.
	 *
	 *   grab_failed            asserts the release did NOT reach the download
	 *                          client, so retrying cannot duplicate anything.
	 *   expired /
	 *   no_longer_offered /
	 *   search_failed          nothing was sent, and the release may well still
	 *                          be there — the action that can succeed is a fresh
	 *                          search, never a retry of an opaque release id
	 *                          that will return the same error for ever.
	 *
	 * ⚠️ `grab_outcome_unknown` IS ON NEITHER LIST, DELIBERATELY. It means the
	 * release WAS handed to Prowlarr and neither UsArr nor Prowlarr can say what
	 * the download client did with it — the download may well be running.
	 * Retrying is precisely what produces two copies of a multi-gigabyte
	 * release, and UsArr cannot tell whether the first one is already going. The
	 * server's own action is "Check your download client", which is a thing the
	 * user does elsewhere, so it renders as text and not as a control.
	 */
	const RETRYABLE = new Set(['grab_failed']);
	const RESEARCHABLE = new Set(['expired', 'no_longer_offered', 'search_failed']);

	/**
	 * The two flags that get one step of visual emphasis.
	 *
	 * ⚠️ THIS IS NOT AN ALLOWLIST AND MUST NEVER BECOME ONE. Every flag string
	 * that arrives is rendered; these two are drawn heavier because they are the
	 * only ones DERIVED rather than indexer-supplied — Prowlarr computes them
	 * from an exact download factor of 0.0 and 0.5 — and the only ones that
	 * change what a download costs your ratio. Matching flags against a known
	 * list would drop every future indexer's flags invisibly. See
	 * `Release.indexerFlags`.
	 */
	const EMPHASISED_FLAGS = new Set(['freeleech', 'halfleech']);

	/** What the toolbar can sort by. `id` matches the column it sorts. */
	const SORT_COLUMNS: SortColumn<Release>[] = [
		{ id: 'age', defaultDir: 'asc', value: (r) => r.ageDays },
		{ id: 'size', defaultDir: 'desc', value: (r) => r.sizeBytes },
		{ id: 'grabs', defaultDir: 'desc', value: (r) => r.grabs },
		{ id: 'peers', defaultDir: 'desc', value: (r) => r.seeders }
	];

	/** Newest first, which is what the design's own toolbar copy states. */
	const DEFAULT_SORT = { key: 'age', dir: 'asc' } as const;

	const SORT_LABELS: Record<string, string> = {
		age: 'Age',
		size: 'Size',
		grabs: 'Grabs',
		peers: 'Seeders'
	};

	/**
	 * `width` is required on every column: `table-layout: auto` has to measure
	 * every cell in every row, which is the one layout mode no containment can
	 * help. See `$lib/list.ts`.
	 *
	 * `stackLine` is the TWO-LINE phone fork, which is the right degradation for
	 * a list that is scanned rather than read one row at a time.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'protocol', header: 'Protocol', width: '88px', stackLine: 2 },
		{ id: 'age', header: 'Age', width: '72px', align: 'end', stackLine: 2 },
		{
			id: 'title',
			header: 'Title',
			width: 'minmax(0, 3fr)',
			stackLabel: false,
			stackLine: 1
		},
		{ id: 'indexer', header: 'Indexer', width: 'minmax(0, 1fr)', stackLine: 2 },
		{ id: 'size', header: 'Size', width: '88px', align: 'end', stackLine: 2 },
		{ id: 'grabs', header: 'Grabs', width: '76px', align: 'end', stackLine: 'hidden' },
		// 112px, not 88px: `Not applicable` is the widest thing this cell can hold
		// (a usenet release has no seeder count at all) and at 88px it wrapped onto
		// a second line, which set the row height for every cell beside it — the
		// tallest-cell failure §9.1 names. Measured in the browser, not guessed.
		{ id: 'peers', header: 'Peers', width: '112px', align: 'end', stackLine: 'hidden' },
		{ id: 'flags', header: 'Indexer flags', width: 'minmax(0, 1fr)', stackLine: 'hidden' },
		// minmax(max-content, auto), never a fixed track: §9.1's overflow policy.
		// A fixed track shears the buttons attached to exactly the rows that are
		// in trouble, with no scrollbar and no way to reach what was cut.
		{ id: 'actions', header: 'Actions', width: 'minmax(max-content, auto)', stackLine: 1 }
	];

	const scope = createIndexerScope();
	const grabs = createRowState<number, GrabState>();

	let query = $state('');
	let submitted = $state('');
	let searchId = $state<string | undefined>(undefined);
	let searching = $state(false);
	let finished = $state(false);
	let searchError = $state('');
	// The server's own error code. `no_indexer_service` is not a failure the user
	// can retry — it means nothing is configured — so it gets the link that fixes
	// it rather than the generic banner.
	let searchErrorCode = $state('');
	let streamGap = $state('');
	let streamConnected = $state(false);
	let pickerOpen = $state(false);

	/**
	 * THE ARRIVAL BUFFER, not the rendered list. Results land here in whatever
	 * order the indexers answer; what the user sees is `order.rows`, which moves
	 * only when ADR-0038 allows it to.
	 */
	let releases = $state<Release[]>([]);
	// Every indexer the server has told us about, and the ones that have reported
	// a final outcome. Until the closing report lands these are the only honest
	// answer to "how much of this search is in".
	let indexersSeen = $state<number[]>([]);
	let outcomes = $state<IndexerOutcome[]>([]);
	let report = $state<SearchReport | undefined>(undefined);

	// The report supersedes the running tally once it arrives: it is the server's
	// own accounting, including indexers that were skipped without ever starting.
	const problems = $derived(problemsFrom(report ? report.indexers : outcomes));
	const indexersTotal = $derived(report?.totalIndexers ?? (indexersSeen.length || undefined));
	const indexersDone = $derived(report ? report.totalIndexers : outcomes.length);

	/**
	 * ADR-0038 clause 6: the sort key and direction live in the URL, so a sorted
	 * search is linkable and survives a reload exactly rather than approximately.
	 *
	 * ⚠️ THE URL IS SEEDED FROM AND MIRRORED TO, NOT DERIVED FROM, and that is a
	 * measured correction rather than a preference. Deriving `sort` from
	 * `page.url` meant an explicit re-sort read the OLD comparator: `replaceState`
	 * does not update `page.url` synchronously, so `order.resort()` on the next
	 * line re-applied the order it already had. In the browser that showed as the
	 * URL changing to `?sort=peers&dir=desc` while the rows did not move at all —
	 * the silent kind of failure, since the control looked as though it had
	 * worked. Local state is authoritative from here on and `writeUrl` mirrors it.
	 */
	let sort = $state<SortSpec>(readSort(page.url.searchParams, SORT_COLUMNS, DEFAULT_SORT));
	const releaseKey = (release: Release) => String(release.candidateId);
	const compare = $derived(
		compareBy(
			SORT_COLUMNS.find((c) => c.id === sort.key),
			sort.dir,
			releaseKey
		)
	);

	const order = createFrozenOrder<Release>({
		source: () => releases,
		key: releaseKey,
		compare: () => compare,
		complete: () => finished
	});

	const listState: ListState = $derived.by(() => {
		if (order.rows.length === 0) return searching ? 'loading' : 'empty';
		return problems.length > 0 ? 'partial' : 'default';
	});

	/** Aliased so the `use:` directive names a plain identifier. The action is
	 * bound to nothing, so detaching it from the object is safe. */
	const resultsRegion = order.region;

	let stream: StreamHandles | undefined;

	// Results stream as indexers answer, so the cross-indexer dedupe cannot be
	// done up front: a later, higher-priority indexer names the candidate it
	// replaces and this drops the superseded row (internal/httpapi/search.go).
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
		// Every frame names an indexer, and the started phase names it before it
		// has answered — which is what makes the picker's catalogue the full set
		// rather than only the indexers that returned a row.
		scope.learn(outcome.indexerId, outcome.name);
		if (!indexersSeen.includes(outcome.indexerId)) {
			indexersSeen = [...indexersSeen, outcome.indexerId];
		}
		if (phase !== 'indexer_done') return;
		outcomes = [...outcomes.filter((o) => o.indexerId !== outcome.indexerId), outcome];
	}

	// One stream for the whole page, opened once. Results are appended as they
	// arrive per indexer — that progressive render is the point of the SSE design.
	onMount(() => {
		stream = openEventStream(
			(event) => {
				if (event.kind === 'unknown') return;
				if (event.kind === 'missed') {
					streamGap = event.action ? `${event.message} — ${event.action}` : event.message;
					return;
				}
				// A frame from an older search is ignored, not rendered late.
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
						for (const outcome of event.report?.indexers ?? []) {
							scope.learn(outcome.indexerId, outcome.name);
						}
						searching = false;
						finished = true;
						// ADR-0038 clause 2's completion edge. It is a call rather than
						// an effect because only this handler knows that the last batch
						// of rows has already landed; see `settle`.
						settle(order);
						break;
					case 'failed':
						searchError = event.action ? `${event.message} — ${event.action}` : event.message;
						searching = false;
						break;
				}
			},
			(connected) => (streamConnected = connected)
		);

		const params = page.url.searchParams;
		// A link that names indexers wins over the sticky selection: whoever sent
		// it meant that scope, and adopting it makes the link do what it says.
		const linked = parseIds(params.getAll('indexer').join(','));
		if (linked.length > 0) scope.adopt(linked);

		const linkedQuery = (params.get('q') ?? '').trim();
		if (linkedQuery !== '') {
			query = linkedQuery;
			void runSearch();
		}

		return () => stream?.close();
	});

	/**
	 * THE ONE PLACE THE URL IS WRITTEN.
	 *
	 * `replaceState` from `$app/navigation`, not `goto`: this is shallow
	 * routing. The screen is not going anywhere — it is recording the state it
	 * is already in — so there is no load to re-run, no scroll to reset and no
	 * focus to move, and `page.url` still updates so `sort` re-derives. Replace
	 * rather than push, because a Back button that walks sort directions is not
	 * what Back is for.
	 *
	 * eslint's `no-navigation-without-resolve` wants the argument to be a
	 * `resolve()` call, whose type is a bare `ResolvedPathname`. A resolved
	 * pathname cannot carry a query string, and the query string is the entire
	 * point here — ADR-0038 clause 6 puts the sort in it — so the URL is built
	 * from `page.url`, which is already resolved, and the rule is suppressed at
	 * the single call site rather than turned off for the file.
	 */
	function writeUrl(params: URLSearchParams) {
		const search = params.toString();
		// `page.url.pathname` already carries any configured base path, so this is
		// a resolved path with a query string appended and nothing more.
		const href = search === '' ? page.url.pathname : `${page.url.pathname}?${search}`;
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- see above: a ResolvedPathname cannot carry ?sort=
		replaceState(href, page.state);
	}

	/** Write the query, the sort and the scope into the URL. */
	function syncUrl(text: string) {
		const params = new SvelteURLSearchParams();
		if (text) params.set('q', text);
		writeSort(params, sort);
		for (const id of scope.selected) params.append('indexer', String(id));
		writeUrl(params);
	}

	/**
	 * AN EXPLICIT RE-SORT. The only thing that may reorder a frozen list, which
	 * is why every path that changes the order goes through here.
	 */
	function setSort(id: string) {
		// Local state first, so `compare` has already re-derived by the time
		// `resort()` reads it. See the note on `sort`.
		sort = { key: id, dir: nextDir(SORT_COLUMNS, sort, id) };
		syncUrl(submitted);
		order.resort();
	}

	async function runSearch(event?: SubmitEvent) {
		event?.preventDefault();
		const trimmed = query.trim();
		if (trimmed === '') return;

		submitted = trimmed;
		searchId = undefined;
		releases = [];
		order.reset();
		indexersSeen = [];
		outcomes = [];
		report = undefined;
		grabs.clear();
		searchError = '';
		searchErrorCode = '';
		streamGap = '';
		finished = false;
		searching = true;

		syncUrl(trimmed);

		try {
			// The scope narrows the fan-out server-side, before the legs are
			// planned, so an unselected indexer spends none of its rate limit.
			const accepted = await startSearch(trimmed, undefined, { indexerIds: scope.selected });
			if (accepted.searchId) searchId = accepted.searchId;
		} catch (error) {
			searching = false;
			searchError = error instanceof ApiError ? error.detail : String(error);
			searchErrorCode = error instanceof ApiError ? error.code : '';
		}
	}

	function toggleIndexer(id: number) {
		scope.toggle(id);
		syncUrl(submitted);
	}

	function clearScope() {
		scope.clear();
		syncUrl(submitted);
	}

	async function grab(release: Release) {
		const id = release.candidateId;
		grabs.set(id, { state: 'grabbing', message: '', code: '', action: '' });
		try {
			const result = await grabRelease(id);
			grabs.set(id, { state: 'grabbed', message: result.message, code: '', action: '' });
		} catch (error) {
			if (!(error instanceof ApiError)) {
				grabs.set(id, { state: 'failed', message: String(error), code: '', action: '' });
				return;
			}
			grabs.set(id, {
				// The one code that is not a failure. See RETRYABLE above.
				state: error.code === 'grab_outcome_unknown' ? 'sent-unknown' : 'failed',
				message: error.detail,
				code: error.code,
				action: error.action
			});
		}
	}

	function flagChips(release: Release) {
		return capChips(release.indexerFlags ?? []);
	}
</script>

<svelte:head><title>Search · UsArr</title></svelte:head>

<!--
	NO <h2> HERE, DELIBERATELY. The shell's toolbar already renders the page name
	and the same string is the h1 inside main, so a heading here would put it on
	screen three times. Standing rule: a heading that only restates the toolbar
	title is removed.
-->

<div class="section">
	<form class="searchbar" onsubmit={runSearch}>
		<label class="sr" for="query">Search terms</label>
		<input
			id="query"
			name="query"
			type="search"
			class="searchbar__input"
			autocomplete="off"
			placeholder="Free-text indexer search"
			bind:value={query}
		/>
		<button type="submit" class="btn btn--primary" disabled={query.trim() === ''}>Search</button>
		<button
			type="button"
			class="btn"
			aria-expanded={pickerOpen}
			aria-controls="indexer-picker"
			onclick={() => (pickerOpen = !pickerOpen)}
		>
			{scope.isAll ? 'Indexers' : `Indexers (${scope.selected.length})`}
		</button>
	</form>

	<!--
		THE SCOPE IS STATED IN WORDS WHENEVER IT IS NOT THE DEFAULT, next to the
		search box rather than inside a picker somebody has to open. A filter that
		persists silently across sessions is how a user concludes an indexer is
		broken when they simply left it deselected weeks ago.
	-->
	{#if scope.summary}
		<p class="scopeline" role="status">
			<span>{scope.summary}</span>
			<span class="muted">This selection is remembered between searches.</span>
			<button type="button" class="linkish" onclick={clearScope}>Search all indexers</button>
		</p>
	{/if}

	{#if pickerOpen}
		<div class="picker" id="indexer-picker">
			<fieldset class="picker__set">
				<legend class="picker__legend">Indexers to search</legend>
				{#if scope.known.length === 0}
					<p class="picker__note">
						No indexer has reported yet. Run one search and every indexer this Prowlarr asked will
						be listed here.
					</p>
				{:else}
					<div class="picker__grid">
						{#each scope.known as indexer (indexer.id)}
							<label class="check">
								<input
									type="checkbox"
									checked={scope.isSelected(indexer.id)}
									onchange={() => toggleIndexer(indexer.id)}
								/>
								<span>{indexer.name}</span>
							</label>
						{/each}
					</div>
				{/if}
				<!--
					⚠️ AN HONEST LABEL ON A FALLBACK. There is no endpoint that lists a
					Prowlarr instance's indexers, so this catalogue is built from what
					searches have already reported. Implying it is Prowlarr's configured
					list would be an invented status.
				-->
				<p class="picker__note muted">
					UsArr has no endpoint that reads Prowlarr's own indexer list, so this is built from the
					indexers that searches have reported so far. An indexer that has never answered here will
					not appear until it does. Selecting none searches all of them.
				</p>
			</fieldset>
		</div>
	{/if}
</div>

<!--
	Every banner below is non-modal and sits above the list. The results list is
	never greyed out and never replaced — ARCHITECTURE.md §17.7.
-->

{#if searchErrorCode === 'no_indexer_service'}
	<!--
		Not a failure to retry: nothing is configured. The fix is one click away
		rather than a sentence describing an API call.
	-->
	<div class="banner banner--err" role="alert">
		<div class="banner__body">
			<div class="banner__text">
				There is no indexer service to search. Add a Prowlarr instance on
				<a href={resolve('/services')}>Services</a> and this screen will start working.
			</div>
			<p class="verbatim">{searchError}</p>
		</div>
	</div>
{:else if searchError}
	<div class="banner banner--err" role="alert">
		<div class="banner__body">
			<div class="banner__text">The search did not complete.</div>
			<p class="verbatim">{searchError}</p>
		</div>
	</div>
{/if}

{#if streamGap}
	<div class="banner banner--warn">
		<div class="banner__body">
			<div class="banner__text">
				Some events were missed while the connection was down, so this list may be incomplete.
			</div>
			<p class="verbatim">{streamGap}</p>
		</div>
	</div>
{/if}

{#if submitted && !streamConnected && !searchError}
	<div class="banner banner--warn">
		<div class="banner__body">
			<div class="banner__text">
				The event stream at <code class="mono">/api/events</code> is not connected, so results cannot
				arrive progressively. Anything already listed below is still valid.
			</div>
		</div>
	</div>
{/if}

<!--
	THE TOOLBAR SITS OUTSIDE THE RESULTS REGION, which is what makes every
	control on it an explicit re-sort (ADR-0038 clause 3). The re-sort control
	lives here for the same reason.
-->
<div class="toolbar">
	<span class="toolbar__label" id="sortlabel">Sort by</span>
	<div class="segment" role="group" aria-labelledby="sortlabel">
		{#each SORT_COLUMNS as column (column.id)}
			<button
				type="button"
				aria-pressed={sort.key === column.id}
				onclick={() => setSort(column.id)}
			>
				{SORT_LABELS[column.id] ?? column.id}
				{#if sort.key === column.id}
					<span aria-hidden="true">{sort.dir === 'asc' ? '↑' : '↓'}</span>
					<span class="sr">, sorted {sort.dir === 'asc' ? 'ascending' : 'descending'}</span>
				{/if}
			</button>
		{/each}
	</div>

	{#if order.hasPending}
		<!--
			ONE CONTROL, CARRYING ITS OWN COUNT (ADR-0038 clause 3). A late
			straggler is not a separate case and there is no
			append-below-marked-late mechanism: the pending rows do not enter the
			list until this is pressed.
		-->
		<button type="button" class="btn" onclick={() => order.resort()}>{order.pendingLabel}</button>
	{/if}

	<div class="toolbar__spacer"></div>

	{#if submitted}
		<span class="toolbar__label" role="status">
			{order.rows.length}
			{order.rows.length === 1 ? 'result' : 'results'}{#if indexersTotal !== undefined}, {indexersDone}
				of {indexersTotal} indexers reported{/if}. {searching ? 'Searching' : 'Finished'}.
		</span>
	{/if}
</div>

{#if problems.length > 0}
	<div class="banner banner--warn">
		<div class="banner__body">
			<div class="banner__text">
				{problems.length}
				{problems.length === 1 ? 'indexer is' : 'indexers are'} not answering, so these results are partial.
			</div>
			{#each problems as problem (problem.indexer)}
				<p class="verbatim">{problem.indexer}: {problem.error}</p>
			{/each}
		</div>
	</div>
{/if}

{#if report?.summary}
	<!--
		Prowlarr answers 200 with `[]` for "every indexer failed", "all
		rate-limited" and "nothing matched" alike, so this sentence is the only
		thing that tells them apart — ARCHITECTURE.md §17.7.
	-->
	<p class="reportline">{report.summary}</p>
{/if}

<!--
	THE RESULTS REGION: the result rows and their header, and nothing else. While
	the pointer is inside it OR focus is within it, the order is frozen.
-->
<div class="resultregion" use:resultsRegion>
	<List
		label="Release results"
		columns={COLUMNS}
		rows={order.rows}
		key={releaseKey}
		stack="two-line"
		state={listState}
		loadingNote="Waiting for the first indexer to report."
		partialNote={report?.summary ??
			'Some indexers have not answered, so these results are partial.'}
		emptyTitle={submitted ? 'No releases matched' : 'Nothing searched yet'}
		emptyText={submitted
			? `Indexer search matches what the indexer itself matches, and UsArr does not rewrite the query. Nothing came back for “${submitted}”.`
			: 'Free-text search across the indexers configured in Prowlarr. Results stream in per indexer as they answer, and Grab posts the release back to Prowlarr.'}
	>
		{#snippet cell(release: Release, column: ListColumn)}
			{#if column.id === 'protocol'}
				{#if release.protocol}
					<span class="proto">{release.protocol}</span>
				{:else}
					<span class="muted">{NOTHING.empty}</span>
				{/if}
			{:else if column.id === 'age'}
				{formatAge(release.ageDays) || NOTHING.empty}
			{:else if column.id === 'title'}
				<span class="mono trunc" title={release.title}>{release.title}</span>
				{@const state = grabs.get(release.candidateId)}
				{#if state && state.state !== 'grabbing'}
					<div class="grabstate">
						{#if state.state === 'grabbed'}
							<span class="chip chip--done">Sent to Prowlarr</span>
						{:else if state.state === 'sent-unknown'}
							<!--
								⚠️ NOT A FAILURE, AND NEVER RENDERED AS ONE. The release
								reached Prowlarr; what the download client did with it is
								unknown. A "grab failed, try again" here is what produces two
								copies of a multi-gigabyte release.
							-->
							<span class="chip chip--pending">Sent, outcome unknown</span>
							<span class="cell-sub">{state.message}</span>
							{#if state.action}<span class="cell-sub">{state.action}</span>{/if}
						{:else}
							<span class="chip chip--failed">
								{RESEARCHABLE.has(state.code) ? 'Not sent, the listing went stale' : 'Not sent'}
							</span>
							<span class="cell-sub">{state.message}</span>
							{#if state.action && !RESEARCHABLE.has(state.code)}
								<span class="cell-sub">{state.action}</span>
							{/if}
						{/if}
					</div>
				{/if}
			{:else if column.id === 'indexer'}
				<span class="trunc" title={release.indexerName ?? ''}>
					{release.indexerName ?? NOTHING.empty}
				</span>
			{:else if column.id === 'size'}
				{formatSize(release.sizeBytes) || NOTHING.empty}
			{:else if column.id === 'grabs'}
				{release.grabs === undefined ? NOTHING.empty : release.grabs.toLocaleString('en-GB')}
			{:else if column.id === 'peers'}
				<!-- `Peers` is Prowlarr's own word, so the header keeps it and the CELL
				     says what its parts are — §9.1's composite-numeric rule. -->
				{#if release.seeders === undefined}
					<span class="muted">
						{release.protocol === 'usenet' ? NOTHING.inapplicable : NOTHING.empty}
					</span>
				{:else}
					{release.seeders.toLocaleString('en-GB')} / {(release.leechers ?? 0).toLocaleString(
						'en-GB'
					)}
					<span class="sr">
						— {release.seeders} seeders, {release.leechers ?? 0} leechers
					</span>
				{/if}
			{:else if column.id === 'flags'}
				{#if (release.indexerFlags ?? []).length === 0}
					<!-- An absent flag means UNKNOWN, never "not freeleech". Usenet
					     results never carry flags at all, whatever the indexer offers,
					     and neither case may read as the ratio-safe claim it is not. -->
					<span class="muted">
						{release.protocol === 'usenet' ? 'Not reported' : 'None reported'}
					</span>
				{:else}
					{@const chips = flagChips(release)}
					<span class="chips">
						{#each chips.shown as flag, i (i)}
							<span class="chip" class:chip--leech={EMPHASISED_FLAGS.has(flag)}>{flag}</span>
						{/each}
						{#if chips.more > 0}<span class="chip">+{chips.more} more</span>{/if}
					</span>
				{/if}
			{:else if column.id === 'actions'}
				{@const state = grabs.get(release.candidateId)}
				<div class="cell-actions">
					{#if state?.state === 'grabbed' || state?.state === 'sent-unknown'}
						<!-- Nothing to offer. Grabbed is done, and the unknown outcome is
						     resolved in the download client, not here. -->
					{:else if state?.state === 'failed' && RESEARCHABLE.has(state.code)}
						<button type="button" class="btn btn--sm" onclick={() => runSearch()}>
							Search again
						</button>
					{:else if state?.state === 'failed' && !RETRYABLE.has(state.code)}
						<!-- An unrecognised code is shown, never retried. -->
					{:else}
						<button
							type="button"
							class="btn btn--sm"
							onclick={() => grab(release)}
							aria-disabled={state?.state === 'grabbing' ? 'true' : undefined}
						>
							{state?.state === 'grabbing'
								? 'Grabbing'
								: state?.state === 'failed'
									? 'Retry'
									: 'Grab'}
						</button>
					{/if}
				</div>
			{/if}
		{/snippet}
	</List>
</div>

<style>
	/*
	 * Screen-only rules. Anything with a design-system name lives in app.css;
	 * what is here belongs to this route and to nothing else.
	 */
	.searchbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}

	.searchbar__input {
		flex: 1 1 20rem;
		min-width: 0;
		min-height: var(--control-h);
		padding: 0 var(--space-4);
		background: var(--bg-inset);
		color: var(--fg);
		border: 1px solid var(--border-strong);
		border-radius: var(--radius-sm);
		font-size: var(--text-md);
	}

	.scopeline {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2) var(--space-4);
		margin: var(--space-4) 0 0;
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.picker {
		margin-top: var(--space-4);
	}

	.picker__set {
		margin: 0;
		padding: var(--space-4) var(--space-5);
		border: 1px solid var(--border-strong);
		border-radius: var(--radius-sm);
		background: var(--bg-raised);
	}

	.picker__legend {
		padding: 0 var(--space-2);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		font-weight: var(--weight-semibold);
	}

	.picker__grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(14rem, 1fr));
		gap: var(--space-2) var(--space-5);
	}

	.picker__note {
		margin: var(--space-4) 0 0;
		max-width: 78ch;
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.reportline {
		margin: var(--space-4) var(--space-6) 0;
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		color: var(--fg-muted);
	}

	/* The chips of one cell wrap together rather than each finding its own line. */
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}

	.grabstate {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2) var(--space-3);
	}
</style>
