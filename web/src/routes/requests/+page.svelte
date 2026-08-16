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
	 * FOUR BLOCKS, AND THIS IS NOW THE SCREEN THEY ALL LIVE ON:
	 *
	 *   1  the search toolbar          the query, the type, and the indexer scope
	 *   2  the fan-out status          real counts, never a progress bar
	 *   3  the release results table   MOVED HERE FROM /search
	 *   4  Recent grabs                the local record a grab leaves
	 *
	 * ⚠️ THERE IS EXACTLY ONE RELEASE-SEARCH SURFACE IN UsArr AND THIS IS IT.
	 * /search was the other one; it is now an honest account of a screen that is
	 * not built — §17.4 is search over your OWN library, and there is no local
	 * index to search yet — plus a pointer here. Two results tables built from
	 * one SSE stream is how they end up disagreeing about what a de-duplicated
	 * row is, and this screen already states the counts such a table must match.
	 *
	 * ⚠️ THIS SCREEN COMPOSES; IT DOES NOT REIMPLEMENT. Four modules arrived with
	 * the table and every one of them is portable by design:
	 *
	 *   $lib/frozenorder.svelte   ADR-0038, the freeze-while-aimed rule
	 *   $lib/rowstate.svelte      ADR-0038 clause 4, identity-keyed row state
	 *   $lib/sortspec             ADR-0038 clause 6, sort key and dir in the URL
	 *   $lib/indexerscope.svelte  the indexer picker's selection and catalogue
	 *
	 * Read `frozenorder.svelte`'s header before touching anything about ordering.
	 * Those rules took four rounds across three threads to settle and every
	 * clause is load-bearing — freeze while the pointer OR focus is in the
	 * results region, identity rather than position, a late straggler is not a
	 * special case, one explicit control carrying its own count, 0 ms and never
	 * animated.
	 *
	 * ⚠️ WHAT CHANGED IN THE MOVE, AND WHY. Two things, both deliberate:
	 *
	 *   THE SORT MOVED FROM THE TOOLBAR ONTO THE COLUMN HEADERS, and the toolbar
	 *   segment is REMOVED rather than kept beside it. The previous screen said
	 *   in as many words why it was in the toolbar: a header affordance needed a
	 *   new prop on `$lib/List.svelte` while another thread was live-editing that
	 *   file. It is not any more, the prop exists, and the sort is now where a
	 *   person looks for it. Keeping both would be two controls for one setting —
	 *   two things to keep in agreement for no gain (CLAUDE.md, "cut before you
	 *   add"). The header sits INSIDE the frozen results region, which is not a
	 *   contradiction: the freeze forbids the order changing without the user
	 *   asking, and pressing a header IS asking. §9.1a names a column header as
	 *   an explicit sort control in the same breath as the toolbar's.
	 *
	 *   RETRY IS GONE FROM EVERY STATE. See `liveGrabCopy` in `$lib/requests` for
	 *   the argument: once `grab_failed` narrowed, nothing left under it is fixed
	 *   by pressing the same button again, and the transient cases a retry would
	 *   help are exactly the ambiguous code that must never offer one.
	 *
	 * WHAT THE COPY RULES ARE, because they are correctness rather than tone.
	 * A grab is irreversible from UsArr's side: the release goes to a download
	 * client UsArr deliberately stops observing after handoff, so a mis-click
	 * cannot be detected, reported or reversed. Two consequences run through
	 * this file. Nothing may claim bytes are moving — "sent" is the strongest
	 * true word for every handed-over state, the successful one included. And
	 * nothing may assert that a grab did not happen where UsArr cannot know:
	 * Prowlarr adds a release to the download client BEFORE the step that failed
	 * for the owner, and never rolls back. The vocabulary lives in
	 * `$lib/requests` where a test can read it, not in this template.
	 */
	import { onMount } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import List from '$lib/List.svelte';
	import { NOTHING, capChips, type ListColumn, type ListState } from '$lib/list';
	import { prefs } from '$lib/prefs.svelte';
	import { createFrozenOrder, settle } from '$lib/frozenorder.svelte';
	import { createRowState } from '$lib/rowstate.svelte';
	import { createIndexerScope, parseIds } from '$lib/indexerscope.svelte';
	import { compareBy, nextDir, readSort, writeSort, type SortColumn } from '$lib/sortspec';
	import {
		ApiError,
		fetchRecentGrabs,
		grabRelease,
		openEventStream,
		problemsFrom,
		startSearch,
		type IndexerOutcome,
		type RecentGrab,
		type Release,
		type SearchReport,
		type StreamHandles
	} from '$lib/api';
	import { formatAge, formatSize } from '$lib/format';
	import {
		CODE_OUTCOME_UNKNOWN,
		DEFAULT_SEARCH_TYPE,
		FROZEN_ORDER_NOTE,
		GRAB_ROW_STALE_NOTE,
		KNOWLEDGE_STOPS_NOTE,
		LIVE_GRAB_SENT_LABEL,
		NOT_SENT_NOTE,
		RECENT_GRAB_ROW_INTRINSIC,
		RELEASE_ROW_INTRINSIC,
		SEARCH_TYPES,
		SEARCH_TYPE_NOTE,
		THIN_COVERAGE_NOTE,
		categoryLabel,
		fanoutSummary,
		flagsAbsence,
		formatWhen,
		grabOutcome,
		grabWindow,
		isEmphasisedFlag,
		isFreeTextOnly,
		liveGrabCopy,
		searchTypeParam
	} from '$lib/requests';

	/** Per-row grab state, keyed by candidate id. Never by index — ADR-0038
	 * clause 4, and `$lib/rowstate.svelte` for why this is the one of clause 4's
	 * four states that has no other home. */
	type GrabState = {
		state: 'grabbing' | 'grabbed' | 'sent-unknown' | 'not-sent';
		message: string;
		code: string;
		action: string;
	};

	/** What the results table can be sorted by. `id` matches the `ListColumn` it
	 * sorts, which is what lets a header hand its own id back. */
	const SORT_COLUMNS: SortColumn<Release>[] = [
		{ id: 'age', defaultDir: 'asc', value: (r) => r.ageDays },
		{ id: 'size', defaultDir: 'desc', value: (r) => r.sizeBytes },
		{ id: 'grabs', defaultDir: 'desc', value: (r) => r.grabs },
		{ id: 'peers', defaultDir: 'desc', value: (r) => r.seeders }
	];

	/** Newest first. `age_days` is the only sortable field the server sends on
	 * EVERY release, so it is the one default that orders a usenet-only install
	 * rather than collapsing straight to the tiebreak. */
	const DEFAULT_SORT = { key: 'age', dir: 'asc' } as const;

	/**
	 * `width` is required on every column: `table-layout: auto` has to measure
	 * every cell in every row, which is the one layout mode no containment can
	 * help. See `$lib/list.ts`.
	 *
	 * `sortable` is the half this move added — the header becomes the sort
	 * control for the four columns `SORT_COLUMNS` can compare, and stays plain
	 * text for the rest, which is also what leaves Services untouched.
	 *
	 * `stackLine` is the TWO-LINE phone fork, which §9.1 gives to a list that is
	 * scanned rather than read one row at a time.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'protocol', header: 'Protocol', width: '88px', stackLine: 2 },
		{ id: 'age', header: 'Age', width: '76px', align: 'end', sortable: true, stackLine: 2 },
		{
			id: 'title',
			header: 'Title',
			width: 'minmax(0, 3fr)',
			stackLabel: false,
			stackLine: 1
		},
		{ id: 'indexer', header: 'Indexer', width: 'minmax(0, 1fr)', stackLine: 2 },
		{ id: 'category', header: 'Category', width: 'minmax(0, 0.8fr)', stackLine: 'hidden' },
		{ id: 'size', header: 'Size', width: '92px', align: 'end', sortable: true, stackLine: 2 },
		{
			id: 'grabs',
			header: 'Grabs',
			width: '80px',
			align: 'end',
			sortable: true,
			stackLine: 'hidden'
		},
		// 112px, not 88px: `Not applicable` is the widest thing this cell can hold
		// (a usenet release has no seeder count at all) and at 88px it wrapped onto
		// a second line, which set the row height for every cell beside it — the
		// tallest-cell failure §9.1 names. Measured in the browser, not guessed.
		{
			id: 'peers',
			header: 'Peers',
			width: '112px',
			align: 'end',
			sortable: true,
			stackLine: 'hidden'
		},
		{ id: 'flags', header: 'Indexer flags', width: 'minmax(0, 1fr)', stackLine: 'hidden' },
		// minmax(max-content, auto), never a fixed track: §9.1's overflow policy.
		// A fixed track shears the buttons attached to exactly the rows that are
		// in trouble, with no scrollbar and no way to reach what was cut.
		{ id: 'actions', header: 'Actions', width: 'minmax(max-content, auto)', stackLine: 1 }
	];

	const grabColumns: ListColumn[] = [
		{ id: 'when', header: 'When', width: '132px' },
		{ id: 'release', header: 'Release', width: 'minmax(0, 3fr)' },
		{ id: 'indexer', header: 'Indexer', width: 'minmax(0, 1fr)' },
		{ id: 'protocol', header: 'Protocol', width: '92px' },
		{ id: 'size', header: 'Size', width: '10ch', align: 'end' },
		{ id: 'outcome', header: 'Outcome', width: 'minmax(0, 1.9fr)' }
	];

	const scope = createIndexerScope();
	const rowGrabs = createRowState<number, GrabState>();

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
	let pickerOpen = $state(false);

	/**
	 * THE ARRIVAL BUFFER, not the rendered list. Results land here in whatever
	 * order the indexers answer; what the user sees is `order.rows`, which moves
	 * only when ADR-0038 allows it to.
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

	/**
	 * A separate, faster clock for the grab window.
	 *
	 * Fifteen seconds, because §17.5 requires the countdown to announce at 5, 2
	 * and 1 minutes remaining and a 60-second clock can step straight over the
	 * one-minute mark. It does NOT make the live region chatty: `grabWindow`
	 * returns the same string for every instant inside a step, and an unchanged
	 * `role="status"` announces nothing.
	 */
	let windowNow = $state(new Date());

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

	/**
	 * ADR-0038 clause 6: the sort key and direction live in the URL, so a sorted
	 * search is linkable and survives a reload exactly rather than approximately.
	 *
	 * ⚠️ THE URL IS SEEDED FROM AND MIRRORED TO, NOT DERIVED FROM, and that is a
	 * measured correction rather than a preference. Deriving `sort` from
	 * `page.url` meant an explicit re-sort read the OLD comparator:
	 * `replaceState` does not update `page.url` synchronously, so `order.resort()`
	 * on the next line re-applied the order it already had. In the browser that
	 * showed as the URL changing to `?sort=peers&dir=desc` while the rows did not
	 * move at all — the silent kind of failure, since the control looked as
	 * though it had worked. Local state is authoritative and `syncUrl` mirrors it.
	 */
	let sort = $state(readSort(page.url.searchParams, SORT_COLUMNS, DEFAULT_SORT));
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

	/** Aliased so the `use:` directive names a plain identifier. The action is
	 * bound to nothing, so detaching it from the object is safe. */
	const resultsRegion = order.region;

	const fanout = $derived(
		fanoutSummary({
			answered,
			total: totalIndexers,
			releases: rawReleases,
			deduped: releases.length,
			finished,
			// ⚠️ THE TABLE IS ON THIS SCREEN NOW, so `shown` is a true claim about
			// it and §17.5's sentence renders verbatim — "112 releases, 10 shown
			// after de-duplication". This flag was the seam; this is it flipped.
			rendered: true
		})
	);

	const listState: ListState = $derived.by(() => {
		if (order.rows.length === 0) return searching ? 'loading' : 'empty';
		return problems.length > 0 ? 'partial' : 'default';
	});

	/**
	 * The grab window, measured from the EARLIEST expiry on screen.
	 *
	 * Earliest rather than latest, because the promise being kept is "an expired
	 * release is never offered as grabbable" and one release being past it is
	 * enough to break the promise. Prowlarr's cache is a non-rolling 30 minutes,
	 * so in practice every candidate of one search expires within seconds of the
	 * others and the distinction rarely shows.
	 */
	const earliestExpiry = $derived(
		order.rows
			.map((r) => r.expiresAt)
			.filter((v): v is string => v !== undefined && !Number.isNaN(Date.parse(v)))
			.sort((a, b) => Date.parse(a) - Date.parse(b))[0]
	);
	const grabWin = $derived(grabWindow(earliestExpiry, windowNow));

	/**
	 * `aria-rowcount` is only allowed to be a number when it is the truth. A
	 * short page is the whole set; a full one means the server had at least as
	 * many as we asked for and we were not told how many more, which is exactly
	 * the case ARIA defines -1 for.
	 */
	const grabTotal = $derived(grabs.length < grabsLimit ? grabs.length : undefined);
	const rowIntrinsic = $derived(RECENT_GRAB_ROW_INTRINSIC[prefs.density] ?? 44);
	const releaseIntrinsic = $derived(RELEASE_ROW_INTRINSIC[prefs.density] ?? 45);

	/* ── behaviour ────────────────────────────────────────────────────────── */

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

	// One stream for the whole page, opened once. Results are appended as they
	// arrive per indexer — that progressive render is the point of the SSE design.
	onMount(() => {
		void loadGrabs();

		const tick = setInterval(() => (now = new Date()), 60_000);
		const windowTick = setInterval(() => (windowNow = new Date()), 15_000);

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

		const linkedType = params.get('type') ?? '';
		if (SEARCH_TYPES.some((t) => t.id === linkedType)) searchType = linkedType;

		const linkedQuery = (params.get('q') ?? '').trim();
		if (linkedQuery !== '') {
			query = linkedQuery;
			void runSearch();
		}

		return () => {
			clearInterval(tick);
			clearInterval(windowTick);
			stream?.close();
		};
	});

	/**
	 * THE ONE PLACE THE URL IS WRITTEN.
	 *
	 * `replaceState` from `$app/navigation`, not `goto`: this is shallow routing.
	 * The screen is not going anywhere — it is recording the state it is already
	 * in — so there is no load to re-run, no scroll to reset and no focus to
	 * move. Replace rather than push, because a Back button that walks sort
	 * directions is not what Back is for.
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

	/** The query, the type, the sort and the scope — all of it linkable. */
	function syncUrl(text: string) {
		const params = new SvelteURLSearchParams();
		if (text) params.set('q', text);
		if (searchType !== DEFAULT_SEARCH_TYPE) params.set('type', searchType);
		writeSort(params, sort);
		for (const id of scope.selected) params.append('indexer', String(id));
		writeUrl(params);
	}

	/**
	 * AN EXPLICIT RE-SORT, arriving from a column header.
	 *
	 * With the re-sort control it is one of exactly two things that may reorder a
	 * frozen list, and it is allowed precisely because the user operated a sort
	 * control — §9.1a's own words. The header being inside the frozen region does
	 * not change that: the freeze forbids the order moving without being asked.
	 */
	function onsort(id: string) {
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
		outcomes = [];
		report = undefined;
		rowGrabs.clear();
		searchError = '';
		searchErrorCode = '';
		streamGap = '';
		finished = false;
		searching = true;
		windowNow = new Date();

		syncUrl(trimmed);

		try {
			// The scope narrows the fan-out server-side, before the legs are
			// planned, so an unselected indexer spends none of its rate limit.
			const accepted = await startSearch(trimmed, {
				type: searchTypeParam(searchType),
				indexerIds: scope.selected
			});
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

	/**
	 * Send one release to Prowlarr's download client.
	 *
	 * §17.5's three-state rule is applied off the server's ERROR CODE and never
	 * off its prose. `$lib/requests.liveGrabCopy` owns the mapping and the words;
	 * this owns the request and where the answer is put.
	 */
	async function grab(release: Release) {
		const id = release.candidateId;
		if (grabWin.expired || rowGrabs.get(id)?.state === 'grabbing') return;

		rowGrabs.set(id, { state: 'grabbing', message: '', code: '', action: '' });
		try {
			const result = await grabRelease(id);
			rowGrabs.set(id, { state: 'grabbed', message: result.message, code: '', action: '' });
		} catch (error) {
			if (!(error instanceof ApiError)) {
				rowGrabs.set(id, { state: 'not-sent', message: String(error), code: '', action: '' });
			} else {
				rowGrabs.set(id, {
					// The one code that is not a failure. See `liveGrabCopy`.
					state: error.code === CODE_OUTCOME_UNKNOWN ? 'sent-unknown' : 'not-sent',
					message: error.detail,
					code: error.code,
					action: error.action
				});
			}
		}
		// Both arms refresh the record: the ambiguous state writes a provenance
		// row too, precisely so the infohash join key survives (internal/releases).
		void loadGrabs();
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

<div class="section">
	<form class="searchbar" onsubmit={runSearch}>
		<label class="sr" for="req-query">Search terms</label>
		<input
			id="req-query"
			name="query"
			type="search"
			class="searchbar__input"
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

	<p class="req-note" id="req-type-note">
		{SEARCH_TYPE_NOTE}
		{#if isFreeTextOnly(searchType)}
			<!-- SW-08, at the point of use: a search that runs correctly against
			     healthy indexers and returns one row looks identical to a broken
			     one, and for these two types the reason is the indexer ecosystem
			     rather than the query. -->
			<span class="req-coverage">{THIN_COVERAGE_NOTE}</span>
		{/if}
	</p>

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
		line: "3 indexers are not answering" is not something a user can act on,
		and the three reasons are usually three different problems.
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
	THE RESULTS REGION: the result rows and their header, and nothing else. While
	the pointer is inside it OR focus is within it, the order is frozen — and the
	`use:` action is where both halves of that condition are wired. The section
	head, the countdown and the re-sort control sit inside it too, which is
	correct: a hand on the re-sort button is a hand aimed at this list.
-->
<section class="section" id="release-results" use:resultsRegion>
	<div class="section__head">
		<h2>Release results</h2>
		{#if order.rows.length > 0}
			<span class="section__count num">
				{order.rows.length}
				{order.rows.length === 1 ? 'result' : 'results'}
			</span>
		{/if}
		<span class="section__actions">
			{#if order.hasPending}
				<!--
					ADR-0038's ONE EXPLICIT CONTROL, carrying its own count. A late
					straggler is not a separate case and gets no separate surface: it is
					another thing that would have reordered, it is counted here, and it
					does not enter the rendered list until this is pressed. There is no
					append-below-marked-late mechanism, and that is a rejection rather
					than an omission.
				-->
				<button type="button" class="btn" onclick={() => order.resort()}>
					{order.pendingLabel}
				</button>
			{/if}
			{#if grabWin.expired && submitted}
				<button type="button" class="btn btn--primary" onclick={() => runSearch()}>
					Search again
				</button>
			{/if}
		</span>
	</div>

	{#if order.hasPending}
		<p class="req-note">{FROZEN_ORDER_NOTE}</p>
	{/if}

	<!--
		THE GRAB WINDOW. §17.5 makes this a requirement rather than a nicety: the
		screen states that an expired release is never offered as grabbable, and
		that is only true if the client acts on it. Otherwise a user who read
		"closes in 18 minutes", worked through the list and pressed Grab receives a
		400 they were promised could not happen.

		TWO ELEMENTS, ON PURPOSE. The always-visible reading ticks; the
		`role="status"` beneath it carries a string that CHANGES ONLY at 5, 2 and 1
		minutes remaining and at zero, so it announces three times rather than
		every fifteen seconds. An unchanged live region announces nothing.
	-->
	{#if order.rows.length > 0 && grabWin.label}
		<p class="req-note req-window">{grabWin.label}</p>
	{/if}
	<p class="req-note req-window" role="status">{grabWin.notice}</p>

	<!--
		NO `onactivate`. The primitive fires Enter or Space on a focused row if it
		is given a handler, and the only thing this row does is irreversible — so
		walking the list with the arrow keys and pressing Enter would send a
		multi-gigabyte release to a download client. Grab is a button the user aims
		at, and nothing else on this row is worth a whole-row activation.
	-->
	<List
		label="Release results"
		columns={COLUMNS}
		rows={order.rows}
		key={releaseKey}
		stack="two-line"
		state={listState}
		rowIntrinsic={releaseIntrinsic}
		sortKey={sort.key}
		sortDir={sort.dir}
		{onsort}
		loadingNote="Waiting for the first indexer to report."
		partialNote={report?.summary ??
			'Some indexers have not answered, so these results are partial.'}
		emptyTitle={submitted ? 'No releases matched' : 'Nothing searched yet'}
		emptyText={submitted
			? `Indexer search matches what the indexer itself matches, and UsArr does not rewrite the query. Nothing came back for “${submitted}”. The line above says how many indexers answered, which is what separates “nothing matched” from “nobody answered”.`
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
				<!-- An identity string: truncated at the cell with the full value in
				     `title`, and never `overflow-wrap: anywhere`, which renders x264
				     as x26 / 4 and destroys the thing the reader is scanning for. -->
				<span class="mono trunc" title={release.title}>{release.title}</span>
				{@const grabState = rowGrabs.get(release.candidateId)}
				{#if grabState && grabState.state !== 'grabbing'}
					{@const copy = liveGrabCopy(grabState.code)}
					<div class="grabstate">
						{#if grabState.state === 'grabbed'}
							<!--
								The strongest true word. Prowlarr's 200 means Prowlarr accepted
								the release, not that a download is running, and UsArr
								deliberately stops observing at handoff — so it will never
								learn more than this.

								⚠️ A PLAIN CHIP, NOT `chip--done`. The previous screen painted
								this one green and it should not be: §9.5 reserves chroma for
								what is WRONG, and Recent grabs already renders the SAME state
								— a stored `sent` row — neutral for exactly that reason. Two
								renderings of one fact in two colours is the disagreement this
								move exists to remove. The green also spelled a word §17.5
								bans on this screen, which is how it was noticed.
							-->
							<span class="chip">{LIVE_GRAB_SENT_LABEL}</span>
						{:else if grabState.state === 'sent-unknown'}
							<!--
								⚠️ NOT A FAILURE, AND NEVER RENDERED AS ONE. The release reached
								Prowlarr; what the download client did with it is unknown, and
								the owner's own book downloaded end to end in Deluge while
								UsArr reported an error. The chip sits beside the confirmed one
								verbally, and the server's action renders as TEXT — the only
								button that would fit here is one that sends the release again,
								and that is what produces two copies of a 68 GB release.
							-->
							<span class="chip chip--pending">{copy.label}</span>
							<span class="cell-sub">{grabState.message}</span>
							{#if grabState.action}<span class="cell-sub">{grabState.action}</span>{/if}
						{:else}
							<span class="chip chip--failed">{copy.label}</span>
							<span class="cell-sub">{grabState.message}</span>
							{#if grabState.action && !copy.offersSearchAgain}
								<span class="cell-sub">{grabState.action}</span>
							{/if}
						{/if}
					</div>
				{/if}
			{:else if column.id === 'indexer'}
				<span class="trunc" title={release.indexerName ?? ''}>
					{release.indexerName ?? NOTHING.empty}
				</span>
			{:else if column.id === 'category'}
				<!-- The server's own `type:` / `format:` tags, not a client-side
				     re-derivation of the Newznab tree: mapping.MediaType runs two
				     passes so [3000, 3030] reads `book · audiobook` rather than
				     `music`, which is exactly what category 3030 exists to prevent. -->
				{@const label = categoryLabel(release.tags, release.categories)}
				{#if label}
					<span class="trunc" title={label}>{label}</span>
				{:else}
					<span class="muted">{NOTHING.empty}</span>
				{/if}
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
				<!--
					⚠️ AN OPEN VOCABULARY. Prowlarr's IndexerFlag is a subclassable
					class, not an enum — PassThePopcorn adds `golden` and `approved` on
					top of the base type's seven — so a chip renders whatever string
					arrives and nothing here matches a known list. An allowlist would
					drop every future indexer's flags INVISIBLY, the row simply showing
					fewer chips than the indexer sent.

					The one step of emphasis on `freeleech` and `halfleech` is weight and
					fill, never hue: chroma marks what is wrong, and a free download is
					not wrong. Three chips plus `+N more` (§9.1).

					And an empty cell is UNKNOWN, never "not freeleech" — $lib/requests
					owns which of the two absences it is, and neither of them is "None".
				-->
				{#if (release.indexerFlags ?? []).length === 0}
					<span class="muted">{flagsAbsence(release.protocol)}</span>
				{:else}
					{@const chips = capChips(release.indexerFlags ?? [])}
					<span class="chips">
						{#each chips.shown as flag, i (i)}
							<span class="chip" class:chip--leech={isEmphasisedFlag(flag)}>{flag}</span>
						{/each}
						{#if chips.more > 0}<span class="chip">+{chips.more} more</span>{/if}
					</span>
				{/if}
			{:else if column.id === 'actions'}
				{@const grabState = rowGrabs.get(release.candidateId)}
				{@const copy = liveGrabCopy(grabState?.code ?? '')}
				<div class="cell-actions">
					{#if grabState?.state === 'grabbed' || grabState?.state === 'sent-unknown'}
						<!-- Nothing to offer. The release is handed over, and an unknown
						     outcome is resolved in the download client, not here. -->
					{:else if grabState?.state === 'not-sent' && copy.offersSearchAgain}
						<!-- The cache dropped this listing, so the same opaque id answers
						     the same 4xx for ever. A fresh search is the one action that
						     can work. -->
						<button type="button" class="btn btn--sm" onclick={() => runSearch()}>
							Search again
						</button>
					{:else if grabState?.state === 'not-sent'}
						<!-- ⚠️ NO BUTTON, AND THIS IS THE NARROWING THE MOVE MADE. Every
						     code left under here is a bad API key, an open breaker, an
						     SSRF refusal, a Prowlarr 400 or 409, a corrupt stored blob or
						     a body Prowlarr would not bind — not one of which is fixed by
						     pressing the same button again. The server's own sentence is
						     beside the title, where it says why. -->
					{:else}
						<!--
							A VISIBLE TEXT LABEL, NEVER A BARE ICON. §17.5 and
							DESIGN-DIRECTION §13: an irreversible multi-gigabyte action may
							not be an unlabelled glyph eight pixels from a benign one —
							particularly a download arrow, which means "download this file
							to my computer" everywhere else and here means "send this to
							your download client via Prowlarr".

							`aria-disabled` rather than `disabled` once the window has
							closed, so the control keeps its place in the tab order and can
							still be read. The handler returns early either way.
						-->
						<button
							type="button"
							class="btn btn--sm"
							onclick={() => grab(release)}
							aria-disabled={grabWin.expired || grabState?.state === 'grabbing'
								? 'true'
								: undefined}
						>
							{grabState?.state === 'grabbing' ? 'Sending' : 'Grab'}
						</button>
					{/if}

					{#if release.infoUrl}
						<!--
							THE INDEXER'S OWN PAGE, AND NEVER A DOWNLOAD URL. Prowlarr
							rewrites downloadUrl and magnetUrl into proxy links carrying its
							FULL ADMIN API KEY, so neither is on the wire at all
							(internal/httpapi/search.go). This is `info_url`, which has been
							through the query-parameter deny-list because a private tracker
							puts its passkey there.

							`resolve()` is inapplicable and the rule is suppressed rather
							than satisfied: this is an ABSOLUTE URL AT ANOTHER ORIGIN, chosen
							by the indexer, and `resolve()` builds an internal path from a
							route id. `rel="noreferrer noopener"` is the part that actually
							matters here — a private tracker must not receive UsArr's URL as
							a Referer, and the opened tab must not get a handle on this one.
						-->
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- an indexer-supplied absolute URL at another origin; resolve() builds internal paths -->
						<a class="btn btn--sm" href={release.infoUrl} target="_blank" rel="noreferrer noopener"
							>Indexer page</a
						>
					{/if}
				</div>
				{#if grabWin.expired && grabState?.state !== 'grabbed'}
					<!-- The row-level note §17.5 requires beside a disabled control. The
					     sentence that explains it is above the table, once. -->
					<div class="cell-sub">{GRAB_ROW_STALE_NOTE}</div>
				{/if}
			{/if}
		{/snippet}
	</List>
</section>

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

			`key` hands the id straight through rather than through String(): it is
			already an opaque string, and nothing on this screen may sort on it,
			compare it numerically or read anything out of its shape.
		-->
		<List
			label="Recent grabs"
			columns={grabColumns}
			rows={grabs}
			key={(g: RecentGrab) => g.id}
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
					<span class="mono trunc" title={grab.releaseTitle}>{grab.releaseTitle}</span>
				{:else if column.id === 'indexer'}
					{#if grab.indexerName}
						<span class="trunc" title={grab.indexerName}>{grab.indexerName}</span>
					{:else}
						<span class="muted">{NOTHING.empty}</span>
					{/if}
				{:else if column.id === 'protocol'}
					{#if grab.protocol}
						<span class="proto">{grab.protocol}</span>
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
	 *
	 * And nothing here carries a transition. §9.1a: sort is 0 ms and a reorder is
	 * never animated anywhere, because an animation widens the window in which
	 * the row under the pointer is neither where it was nor where it is going.
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

	/* A block-level note: the sentences §17.5 requires around each block. Left
	 * aligned in flow at the same content edge as the table it sits above, never
	 * a centred box. */
	.req-note {
		max-width: 78ch;
		margin: var(--space-2) 0 0;
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.req-note + .req-note {
		margin-bottom: var(--space-4);
	}

	.req-window {
		margin-top: var(--space-1);
	}

	/* An empty live region stays IN FLOW and is never display:none — a region
	 * hidden that way is not announced when it later fills. It simply takes no
	 * vertical space until it has something to say. */
	.req-window:empty {
		margin: 0;
	}
</style>
